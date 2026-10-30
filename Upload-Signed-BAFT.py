#!/usr/bin/env python3
"""Upload verified public files to the existing BAFT draft; never publish it.

No signing key or other private file is needed or accessed. GitHub authentication
uses the owner's existing gh configuration. The agent performs independent
published-release verification and acceptance after the upload is complete.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

REPO = 'zarkmakerburg/BAFT'
VERSION = 'v0.4.0'
SOURCE = '9c32ba369fe2be2c549f2878d1de5dbf092c8f64'
TAG_OBJECT = '69e77c655caa0fd0d18ca87226f20c23a2af2b35'
RELEASE_ID = 429023788
INVENTORY_SHA256 = 'e01b023d3780b992e0e36dc0878f86e516d9bf4f014460aab187f59b2f90d2f9'
NAMES = {f'{n}-linux-{a}' for n in ('baft', 'baft-pair', 'baft-bcc', 'baft-agent', 'baft-install')
         for a in ('amd64', 'arm64')} | {
    'manifest.json', 'SHA256SUMS', 'release-key.cert.json',
    'baft-offline-v0.4.0.tar.gz', 'baft-offline-v0.4.0.tar.gz.sha256',
    'LICENSE', 'COPYRIGHT', 'THIRD_PARTY_LICENSES.md'}


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def gh_api(path):
    result = subprocess.run(['gh', 'api', f'repos/{REPO}/{path}'],
                            capture_output=True, text=True)
    require(result.returncode == 0, 'GitHub API ناموفق بود؛ دسترسی حساب به این Draft را بررسی کن: ' + path)
    return json.loads(result.stdout)


def validate_binding():
    ref = gh_api('git/ref/tags/' + VERSION)
    require(ref['object']['type'] == 'tag' and ref['object']['sha'] == TAG_OBJECT,
            'tag object تغییر کرده؛ بارگذاری متوقف شد.')
    tag = gh_api('git/tags/' + TAG_OBJECT)
    require(tag['object']['type'] == 'commit' and tag['object']['sha'] == SOURCE,
            'tag به commit پذیرفته‌شده اشاره نمی‌کند.')
    require(gh_api('branches/main')['commit']['sha'] == SOURCE,
            'main تغییر کرده؛ بررسی مجدد ایجنت لازم است.')
    release = gh_api('releases/' + str(RELEASE_ID))
    require(release.get('id') == RELEASE_ID and release.get('tag_name') == VERSION
            and release.get('target_commitish') == SOURCE
            and release.get('draft') is True and release.get('published_at') is None,
            'شناسه یا وضعیت Draft تغییر کرده؛ بارگذاری متوقف شد.')
    return release


def remote_assets():
    assets = gh_api(f'releases/{RELEASE_ID}/assets?per_page=100')
    require(isinstance(assets, list) and len(assets) <= len(NAMES), 'تعداد assetها نامعتبر است.')
    found = {}
    for asset in assets:
        name = asset.get('name')
        require(name in NAMES and name not in found and asset.get('state') == 'uploaded',
                'asset اضافی، تکراری یا نیمه‌کاره در Draft وجود دارد.')
        require(type(asset.get('id')) is int and asset['id'] > 0, 'شناسه asset نامعتبر است.')
        found[name] = asset
    return found


def curl_fetch(url, target, max_bytes):
    """Use curl's normal CA trust, with HTTPS-only redirects and bounded output."""
    command = ['curl', '--disable', '--fail', '--silent', '--show-error', '--location',
               '--proto', '=https', '--proto-redir', '=https',
               '--connect-timeout', '30', '--max-time', '600', url]
    digest = hashlib.sha256()
    count = 0
    # Keeping stderr off a pipe prevents an error response from blocking stdout.
    # No CA overrides, TLS-disable flags or credential/header injection are used.
    with tempfile.TemporaryFile() as errors, target.open('xb') as output:
        with subprocess.Popen(command, stdout=subprocess.PIPE, stderr=errors) as response:
            try:
                while True:
                    chunk = response.stdout.read(min(1024 * 1024, max_bytes - count + 1))
                    if not chunk:
                        break
                    count += len(chunk)
                    require(count <= max_bytes, 'اندازه دانلود بیشتر از مقدار ثابت است.')
                    digest.update(chunk)
                    output.write(chunk)
                status = response.wait()
                if status != 0:
                    errors.seek(0)
                    detail = errors.read(4096).decode('utf-8', errors='replace').strip()
                    raise RuntimeError('دانلود معتبر HTTPS با curl ناموفق بود: ' + detail)
            except BaseException:
                if response.poll() is None:
                    response.kill()
                response.wait()
                raise
    return count, digest.hexdigest()


def download(url, target, expected):
    count, digest = curl_fetch(url, target, expected['bytes'])
    require(count == expected['bytes'] and digest == expected['sha256'],
            'هش یا اندازه فایل دانلودشده مطابقت ندارد: ' + target.name)


def verify_remote(asset, expected, work):
    require(asset.get('size') == expected['bytes'], 'اندازه asset موجود متفاوت است: ' + asset['name'])
    path = work / ('remote-' + str(asset['id']))
    # gh follows GitHub's supported authenticated asset download route. Contents
    # are public release data, never executed. No headers or tokens are logged.
    with path.open('xb') as output:
        result = subprocess.run(['gh', 'api', '-H', 'Accept: application/octet-stream',
                                 f'repos/{REPO}/releases/assets/{asset["id"]}'],
                                stdout=output, stderr=subprocess.DEVNULL)
    require(result.returncode == 0, 'دریافت asset موجود ناموفق بود: ' + asset['name'])
    require(path.stat().st_size == expected['bytes']
            and hashlib.sha256(path.read_bytes()).hexdigest() == expected['sha256'],
            'محتوای asset موجود متفاوت است؛ هیچ فایلی جایگزین نشد: ' + asset['name'])
    path.unlink()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--commit', required=True, help='immutable public delivery commit')
    args = parser.parse_args(argv)
    require(re.fullmatch('[0-9a-f]{40}', args.commit), 'commit کامل لازم است.')
    require(sys.platform == 'darwin', 'این ابزار را روی مک اجرا کن.')
    require(os.environ.get('CI') != 'true' and os.environ.get('GITHUB_ACTIONS') != 'true',
            'این ابزار برای GitHub Actions یا CI نیست.')
    require(shutil.which('gh'), 'ابتدا gh را نصب کن: brew install gh')
    require(shutil.which('curl'), 'ابزار curl پیدا نشد.')
    auth = subprocess.run(['gh', 'auth', 'status', '--hostname', 'github.com'],
                          stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    require(auth.returncode == 0,
            'ورود GitHub لازم است. اجرا کن: gh auth login --hostname github.com --web')
    validate_binding()
    work = Path(tempfile.mkdtemp(prefix='baft-public-upload-v0.4.0.', dir=Path.home()))
    print('فقط فایل‌های عمومی انتشار؛ پوشه: ' + str(work), flush=True)
    base = f'https://raw.githubusercontent.com/{REPO}/{args.commit}/publication-v0.4.0'
    inventory_path = work / 'ASSET-INVENTORY.json'
    # A hard limit applies before interpreting the pinned inventory.
    count, digest = curl_fetch(base + '/ASSET-INVENTORY.json', inventory_path, 65536)
    require(count <= 65536 and digest == INVENTORY_SHA256,
            'هش فهرست ثابت انتشار مطابقت ندارد.')
    inventory = json.loads(inventory_path.read_bytes())
    require(inventory.get('version') == VERSION and inventory.get('commit') == SOURCE
            and inventory.get('draft_release_id') == RELEASE_ID
            and set(inventory.get('assets', {})) == NAMES,
            'فهرست assetها یا اتصال نسخه نامعتبر است.')
    specs = inventory['assets']
    for name, spec in specs.items():
        require(type(spec['bytes']) is int and 0 < spec['bytes'] < 100 * 1024 * 1024
                and re.fullmatch('[0-9a-f]{64}', spec['sha256']), 'مشخصات فایل نامعتبر است: ' + name)
    verified = {}
    for number, name in enumerate(sorted(NAMES), 1):
        validate_binding()
        existing = remote_assets()
        if name in existing:
            verify_remote(existing[name], specs[name], work)
            print(f'[{number}/18] قبلاً بارگذاری و تطبیق شد: {name}', flush=True)
            verified[name] = existing[name]['id']
            continue
        path = work / name
        print(f'[{number}/18] دریافت و بارگذاری: {name}', flush=True)
        download(base + '/assets/' + name, path, specs[name])
        validate_binding()
        # No --clobber: a concurrent or conflicting asset is never overwritten.
        uploaded = subprocess.run(['gh', 'release', 'upload', VERSION, str(path), '--repo', REPO],
                                  stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
        require(uploaded.returncode == 0,
                'بارگذاری ناموفق بود: ' + name + '\n' + uploaded.stderr.strip())
        current = remote_assets()
        require(name in current, 'asset پس از بارگذاری پیدا نشد: ' + name)
        verify_remote(current[name], specs[name], work)
        verified[name] = current[name]['id']
    validate_binding()
    final = remote_assets()
    require(set(final) == NAMES and all(final[n]['id'] == verified[n] for n in NAMES),
            'assetها حین بارگذاری تغییر کردند؛ بررسی ایجنت لازم است.')
    receipt = {'version': VERSION, 'source_commit': SOURCE, 'release_id': RELEASE_ID,
               'draft': True, 'published': False, 'delivery_commit': args.commit,
               'inventory_sha256': INVENTORY_SHA256, 'asset_ids': verified,
               'all_18_uploaded_bytes_verified': True, 'private_key_accessed': False}
    receipt_path = work / 'UPLOAD-RECEIPT.json'
    receipt_path.write_text(json.dumps(receipt, indent=2) + '\n')
    print('PASS: هر ۱۸ فایل بارگذاری و بررسی شد. نسخه همچنان Draft است؛ انتشار با ایجنت است.', flush=True)
    print('تمام شد؛ در چت بنویس «آپلود تمام شد». رسید عمومی: ' + str(receipt_path), flush=True)
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, RuntimeError, ValueError, KeyError) as error:
        print('متوقف شد: ' + str(error), file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        print('\nلغو شد؛ اجرای دوباره فایل‌های یکسان موجود را بررسی و رد می‌کند.', file=sys.stderr)
        sys.exit(130)
