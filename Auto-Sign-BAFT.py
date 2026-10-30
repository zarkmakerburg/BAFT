#!/usr/bin/env python3
"""Find existing local BAFT key paths; run the unchanged reviewed Mac wrapper.

Discovery reads directory entries and file metadata only, never key contents.
The trusted signing wrapper still performs all verification, the manual offline
acknowledgement, signing and final verification. No keys are created or uploaded.
"""
import hashlib
import os
from pathlib import Path
import stat
import subprocess
import sys

SHA = "9c32ba369fe2be2c549f2878d1de5dbf092c8f64"
WRAPPER_SHA256 = "2541c1e8fd9631f53de4d4b01eb24470585d3e4205656cd206aeea60e29da5bb"
PACKAGE = "BAFT-v0.4.0-macOS-signing"
NAMES = ("release.key", "release-key.cert.json")

# Bash 3.2 compatible. Only substitute the two exact path reads; the network
# acknowledgement and all other reads retain the original interactive behavior.
BASH = r'''
set -euo pipefail
BAFT_AUTO_SCRIPT=$1
BAFT_AUTO_SHA=$2
BAFT_AUTO_KEY=$3
BAFT_AUTO_CERT=$4
read() {
    if [[ $# -eq 2 && "$1" == -r && "$2" == BAFT_KEY ]]; then
        BAFT_KEY=$BAFT_AUTO_KEY
        printf '%s\n' "$BAFT_KEY"
    elif [[ $# -eq 2 && "$1" == -r && "$2" == BAFT_CERT ]]; then
        BAFT_CERT=$BAFT_AUTO_CERT
        printf '%s\n' "$BAFT_CERT"
    else
        builtin read "$@"
    fi
}
source "$BAFT_AUTO_SCRIPT" "$BAFT_AUTO_SHA"
'''


def discover(home, base):
    found = {name: [] for name in NAMES}
    denied = []
    for current, dirs, files in os.walk(home, followlinks=False, onerror=denied.append):
        current = Path(current)
        dirs[:] = sorted(d for d in dirs if d not in (".git", ".Trash")
                         and not (current / d).is_symlink()
                         and current / d != base
                         and not (current == home and d.startswith("baft-signed-v0.4.0.")))
        for name in NAMES:
            if name not in files:
                continue
            path = current / name
            try:
                regular = stat.S_ISREG(path.lstat().st_mode)
            except OSError:
                continue
            if regular and "\n" not in str(path) and "\r" not in str(path):
                found[name].append(path)
    return {name: sorted(paths) for name, paths in found.items()}, len(denied)


def choose(name, paths, terminal):
    if len(paths) == 1:
        print(f"{name}: {paths[0]}", flush=True)
        return paths[0]
    print(f"چند فایل {name} پیدا شد؛ شمارهٔ فایل اصلی خودت را انتخاب کن:", flush=True)
    for number, path in enumerate(paths, 1):
        print(f"  {number}) {path}", flush=True)
    while True:
        print("شماره: ", end="", flush=True)
        answer = terminal.readline()
        if not answer:
            raise RuntimeError("انتخاب لغو شد.")
        try:
            number = int(answer.strip())
        except ValueError:
            continue
        if 1 <= number <= len(paths):
            return paths[number - 1]


def main():
    if sys.platform != "darwin":
        raise RuntimeError("این ابزار برای اجرا روی macOS است.")
    if os.environ.get("CI") == "true" or os.environ.get("GITHUB_ACTIONS") == "true":
        raise RuntimeError("امضا در CI مجاز نیست؛ روی مک نگهدارندهٔ کلید اجرا کن.")
    home = Path.home().resolve()
    base = home / "Downloads" / PACKAGE
    if base.is_symlink() or not base.is_dir():
        raise RuntimeError("بسته را در Downloads باز کن: " + str(base))
    base = base.resolve()
    wrapper = base / "Sign-BAFT.command"
    if wrapper.is_symlink() or not wrapper.is_file():
        raise RuntimeError("اسکریپت اصلی بسته پیدا نشد.")
    if hashlib.sha256(wrapper.read_bytes()).hexdigest() != WRAPPER_SHA256:
        raise RuntimeError("هش اسکریپت اصلی تغییر کرده؛ اجرای امضا متوقف شد.")
    print("در حال جستجوی فایل‌های موجود در پوشهٔ کاربر؛ محتوای کلید خوانده نمی‌شود…", flush=True)
    found, denied = discover(home, base)
    missing = [name for name, paths in found.items() if not paths]
    if missing:
        for name in NAMES:
            for path in found[name]:
                print(f"{name}: {path}", flush=True)
        raise RuntimeError("فایل با نام استاندارد پیدا نشد: " + ", ".join(missing)
                           + f". پوشه‌های غیرقابل دسترسی: {denied}. کلید جدید ساخته نشد؛ این خروجی را گزارش کن.")
    if denied:
        print(f"جستجو به {denied} پوشه دسترسی نداشت؛ ممکن است نسخه‌های دیگری آنجا باشند.", flush=True)
    with open("/dev/tty", "r") as terminal:
        key = choose(NAMES[0], found[NAMES[0]], terminal)
        cert = choose(NAMES[1], found[NAMES[1]], terminal)
        print("بررسی عمومی شروع می‌شود. در درخواست قطع شبکه، اینترنت را قطع کن و Enter بزن. مسیرها خودکار وارد می‌شوند.", flush=True)
        return subprocess.run(["/bin/bash", "-c", BASH, "baft-auto-sign",
                               str(wrapper), SHA, str(key), str(cert)],
                              stdin=terminal, cwd=base, check=False).returncode


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, RuntimeError) as error:
        print("متوقف شد: " + str(error), file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        print("\nلغو شد.", file=sys.stderr)
        sys.exit(130)
