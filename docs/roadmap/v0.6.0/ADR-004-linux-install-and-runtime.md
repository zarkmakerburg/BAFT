<div dir="rtl" align="right" lang="fa">

# ADR-004: نصب و محیط اجرای لینوکس، طراحی از صفر (v0.6)

- **وضعیت:** پیشنهادی، منتظر تصمیم مالک (تصمیم‌های باز در §۱۹)
- **تاریخ:** ۲۰۲۶-۱۱-۰۲
- **دامنه:** نصب، آپدیت، rollback، uninstall و چیدمان فایل‌ها و سرویس‌های BAFT روی سرور لینوکس: پنل (BCC)، agent و تونل
- **مرتبط:** [ADR-003](../v0.5.0/ADR-003-real-host-public-ingress.md)، [طراحی Update Center](../../design/update-center.md)، [REAL_HOSTS_LAB](../v0.5.0/REAL_HOSTS_LAB.md)، PR #305 (آپدیت یکپارچهٔ موقت برای v0.5.1)

## ۱. مسئله

وضعیت فعلی (`main`، `2c8e14ef`):

- **نصب‌کننده یک اسکریپت bash با ۴۰۵۰ خط است.** Plan، rollback، جلوگیری از downgrade، تأیید امضا، هم‌زیستی با سرویس‌های دیگر، گواهی پنل و تنظیم شبکه همه در bash نوشته شده‌اند.
- **چهار باینری جدا به‌اضافهٔ خود نصب‌کننده:**

  | باینری (amd64، v0.4.0) | حجم |
  |---|---|
  | `baft` | ۱۱ MiB |
  | `baft-pair` | ۴ MiB |
  | `baft-agent` | ۸ MiB |
  | `baft-bcc` | ۱۸ MiB |

  هرکدام حالت نصب جدا دارند (`--role`، `--agent-only`، `--bcc-only`) و نسخه‌هایشان روی یک سرور می‌توانند از هم جدا بیفتند.
- **وضعیت نصب جایی ثبت نشده است.** تنظیمات یک سرور فقط داخل سطر `ExecStart` فایل‌های systemd است. آپدیت باید آن را تجزیه کند؛ #305 همین کار را به‌عنوان راه موقت می‌کند.
- **ردپای پراکنده روی دیسک:**
  - پوشه‌ها: `/opt/baft`، `/etc/baft`، `/var/lib/baft`، `/etc/baft-agent`، `/var/lib/baft-agent`، `/etc/baft-bcc`، `/var/lib/baft-bcc`، `/usr/local/lib/baft`، `/root/.acme.sh`
  - فایل‌های تنها: `/etc/sysctl.d/99-baft-net.conf`
  - اسکریپت‌های تولیدشده: `baft-panel` و `baft-net`
  - unitها: `baft.service`، `baft-<instance>.service`، `baft-agent.service`، `baft-bcc.service`، `baft-bcc-cert.{service,timer}`، `baft-recover.service`، `baft-ingress-chain.service`
- **Uninstall دقیق نیست.** در LAB واقعی، پس از `baft uninstall --full` این‌ها باقی ماندند: `/etc/baft`، `/var/lib/baft`، `/var/lib/baft-agent`، `/usr/local/lib/baft` و `baft-net`.
- **وابستگی‌ها:** python3، openssl، curl، acme.sh و گاهی apt. در حالت source، خود Go هم لازم است.
- **سه مسیر آپدیت جدا:** نصب‌کننده، `baft-panel update`، و job `update_baft` که تا v0.5.0 فقط `baft` و `baft-pair` را عوض می‌کرد.
- **سرور ایران معمولاً به GitHub دسترسی ندارد.** راه فعلی بستهٔ آفلاین است که باید دستی منتقل شود.

## ۲. هدف‌ها و غیرهدف‌ها

**هدف‌ها:**
1. یک دستور برای نصب و یک دستور برای آپدیت هر سرور، و یک کلیک در پنل برای آپدیت هر سرور، از جمله خود سرور پنل.
2. یک منبع حقیقت برای «چه چیزی با چه تنظیماتی روی این سرور نصب است».
3. هر تغییر، Plan قابل‌نمایش، برگشت خودکار، و بازیابی پس از قطع برق وسط کار دارد.
4. Uninstall دقیقاً همان چیزهایی را پاک می‌کند که BAFT ساخته است.
5. همان زنجیرهٔ اعتماد فعلی (ریشهٔ ثابت، گواهی کلید انتشار، فهرست ابطال، ضد downgrade) بدون هیچ تضعیفی.
6. نصب و آپدیت سرور ایران بدون دسترسی به GitHub.
7. منطق در Go با تست واحد؛ bash فقط برای راه‌اندازی.

**غیرهدف‌ها:**
- تغییر پروتکل تونل یا رفتار BCC.
- پشتیبانی از توزیع‌های بدون systemd.
- پکیج‌های `.deb` و مخزن apt. اینها ممکن است بعداً از همین چیدمان ساخته شوند (§۱۸).

## ۳. خلاصهٔ تصمیم

| موضوع | امروز | پیشنهاد |
|---|---|---|
| باینری | ۴ باینری + اسکریپت | **یک باینری `baft` با چند فرمان**؛ نام‌های قدیمی به آن لینک می‌شوند |
| منبع حقیقت | سطرهای `ExecStart` | **`/etc/baft/node.json`** |
| منطق نصب | bash، ۴۰۵۰ خط | **`baft setup` / `baft apply` در Go** |
| راه‌انداز | همان اسکریپت ۴۰۵۰ خطی | **`install.sh` حدود ۱۵۰ خط**: دانلود، تأیید امضا، تحویل |
| نسخه‌ها | جایگزینی در `/usr/local/bin` و نگه‌داشتن `.prev` | **`/opt/baft/versions/<v>/`** و symlink اتمی `current` |
| آپدیت از پنل | job داخل خود agent | **سرویس موقت `baft update`** بیرون از agent، با بررسی سلامت و rollback |
| سرور پنل | از پنل قابل آپدیت نیست | **همیشه یک agent محلی دارد**، پس از پنل آپدیت می‌شود |
| ایران | بستهٔ آفلاین دستی | **پنل آینهٔ Releaseهای تأییدشده است** |
| Uninstall | فهرست مسیرهای ثابت | **دفتر مالکیت**: فقط چیزهایی که ثبت شده‌اند پاک می‌شوند |
| گواهی پنل | acme.sh در `/root` | **ACME داخل خود پنل (Go)** |

## ۴. یک باینری با چند فرمان

<div dir="ltr" align="left">

```
baft run            # tunnel node            (was: baft run)
baft agent          # BCC agent              (was: baft-agent)
baft panel          # Command Center         (was: baft-bcc)
baft pair ...       # pairing codes          (was: baft-pair)
baft setup          # interactive wizard -> writes desired state
baft apply          # plan + apply desired state  [--plan] [--json] [--yes]
baft update         # [--version vX.Y.Z] [--offline DIR] [--mirror URL] [--plan] [--yes]
baft rollback       # switch back to the previous installed version
baft uninstall      # [--keep-data] ... (exists today, moves onto the ledger)
baft status | doctor | logs | support-bundle | net | dns-failover
sudo baft           # menu (fa/en)
```

</div>

- **سازگاری با نام‌های قدیمی:** اگر باینری با نام `baft-agent`، `baft-bcc` یا `baft-pair` اجرا شود (از روی `argv[0]`، مثل busybox)، مستقیم همان حالت را اجرا می‌کند. پس unitها، اسکریپت‌ها و agentهای قدیمی بدون تغییر کار می‌کنند.
- **سیاست artifact در فاز ۱ (اندازه‌گیری مقدماتی):** برای حفظ سازگاری v0.5.x و محدود نگه‌داشتن دانلودها، `scripts/release/build.sh` فعلاً artifactهای جداگانهٔ `baft`، `baft-agent`، `baft-bcc`، `baft-pair` و نصب‌کننده را برای هر معماری می‌سازد. entrypointهای قدیمی wrapperهای نازک Go هستند، اما باینری‌های خروجی **کپی بایت‌به‌بایت `baft` نیستند**؛ هرکدام فقط کد و وابستگی‌های جزء مربوط را لینک می‌کنند. بنابراین Update Center قدیمی هنوز artifact مورد انتظارش را می‌گیرد و مجبور نیست همیشه باینری کامل را دانلود کند. این تفاوت با حالت نهایی «یک artifact اصلی + لینک‌های محلی» باید هنگام تصمیم‌گیری فاز ۲ دوباره بررسی شود.
- **اندازه‌گیری واقعی Release build:** روی شاخهٔ `codex/v0.6.0-multicall`، SHA `455f1e49fece5572bd3d1e340052a23acc9db801`، از اجرای موفق `scripts/release/dry_run.sh`:
  
  | Artifact | amd64 | arm64 |
  |---|---:|---:|
  | `baft` | 24,785,020 B (23.64 MiB) | 23,134,332 B (22.06 MiB) |
  | `baft-agent` | 11,276,412 B (10.75 MiB) | 10,223,740 B (9.75 MiB) |
  | `baft-bcc` | 19,079,292 B (18.20 MiB) | 18,022,524 B (17.19 MiB) |
  | `baft-pair` | 5,091,452 B (4.86 MiB) | 4,653,180 B (4.44 MiB) |
  | `baft-install` | 201,338 B (0.19 MiB) | 201,338 B (0.19 MiB) |
  
  این اعداد اندازهٔ فایل‌های Release با flags ساخت فعلی‌اند؛ هنوز مقایسهٔ apples-to-apples با artifactهای امضاشدهٔ v0.4.0 یا سنجش دانلود واقعی همهٔ نقش‌ها نیستند. باینری اصلی `baft` اکنون حدود 23.64 MiB است؛ معیار فاز ۱ «حجم اندازه‌گیری‌شده» ثبت شد، اما تصمیم نهایی دربارهٔ یک artifact مشترک در برابر artifactهای انتقالی هنوز باز است.
- **جداسازی دسترسی** با خود unitها حفظ می‌شود، نه با جدا بودن فایل‌ها:
  - تونل با کاربر `baft` اجرا می‌شود.
  - پنل با کاربر خودش و فقط `CAP_NET_BIND_SERVICE`.
  - agent با root، به همان دلیل امروز: nft و systemd و نوشتن فایل‌های تونل.

  کدی که روی دیسک هست ولی اجرا نمی‌شود، سطح حمله را بیشتر نمی‌کند.

## ۵. چیدمان فایل‌ها

<div dir="ltr" align="left">

```
/opt/baft/versions/v0.6.0/baft        # verified, read-only (0555, root)
/opt/baft/versions/v0.5.1/baft        # previous (kept: 2)
/opt/baft/current -> versions/v0.6.0  # one atomic symlink switch
/usr/local/bin/baft -> /opt/baft/current/baft
/usr/local/bin/baft-agent|baft-bcc|baft-pair -> baft   (compat, phase-out in v0.7)

/etc/baft/node.json                   # desired state (§6), root 0600
/etc/baft/release-state.json          # anti-downgrade trust state (monotonic, separate on purpose)
/etc/baft/tunnel/<name>/              # tunnel configs, keys, PKI
/etc/baft/agent/                      # token, pinned BCC job key, release root
/etc/baft/panel/                      # admin token, access, job key, backup key, TLS
/var/lib/baft/{tunnel,agent,panel}/   # runtime state (BCC database under panel/)
/var/lib/baft/install/ledger.json     # ownership ledger (§11)
/var/lib/baft/install/journal/        # apply journal for crash recovery (§7)

/usr/lib/systemd/system/baft-*.service  # BAFT's units (package-style)
/etc/systemd/system/baft-*.service.d/   # the operator's overrides: never written by BAFT
```

</div>

- **unitها در `/usr/lib/systemd/system`:** این جای unitهای «بسته» است. `systemctl edit` تنظیمات اپراتور را در `/etc/.../*.d/` می‌نویسد و آپدیت هیچ‌وقت به آن دست نمی‌زند. امروز هر تغییر دستی در unit با اجرای دوباره پاک می‌شود.
- **نصب‌های قدیمی:** مسیرهای موجود جابه‌جا نمی‌شوند (§۱۴). `node.json` مسیر واقعی هر جزء را ثبت می‌کند، پس یک نصب v0.5 با پوشه‌های `/etc/baft-bcc` و `/var/lib/baft-agent` همان‌جا می‌ماند. چیدمان بالا فقط برای نصب تازه است.

## ۶. فایل وضعیت: `/etc/baft/node.json`

<div dir="ltr" align="left">

```json
{
  "schema": 1,
  "release": { "channel": "stable", "pinned": "" },
  "components": {
    "panel": {
      "unit": "baft-panel.service",
      "listen": "0.0.0.0:2087",
      "address": { "kind": "domain", "host": "panel.example.com" },
      "tls": { "mode": "acme", "renew": "auto" },
      "paths": { "config": "/etc/baft/panel", "state": "/var/lib/baft/panel" }
    },
    "agent": {
      "unit": "baft-agent.service",
      "bcc_url": "https://panel.example.com:2087",
      "node_id": "ir-1",
      "interval": "30s",
      "metrics_listen": "127.0.0.1:9191",
      "ingress": { "chain": true, "profile": "host-v1" },
      "release_mirror": "https://panel.example.com:2087/releases",
      "paths": { "config": "/etc/baft/agent", "state": "/var/lib/baft/agent" }
    },
    "tunnels": [
      { "name": "main", "role": "ex", "managed_by": "installer",
        "unit": "baft@main.service", "config": "/etc/baft/tunnel/main/baft.yaml" }
    ]
  },
  "host": { "network_tuning": "bbr", "dns_failover": false }
}
```

</div>

قواعد:
- **بدون راز:** فقط مسیر رازها در فایل است. توکن، کلید و گذرواژه هیچ‌وقت داخل `node.json` نوشته نمی‌شوند.
- **مالکیت تونل‌ها:**
  - تونل‌هایی که BCC می‌سازد مال agent هستند (`managed_by: "agent"`). فهرست آن‌ها در وضعیت خود agent است، که امروز هم هست.
  - `node.json` فقط می‌گوید agent مالک تونل‌های instance است. نصب‌کننده هرگز فایل‌های آن‌ها را نمی‌نویسد.
- **نوشتن و قفل:** فقط `baft setup`، `baft apply`، `baft update` و `baft migrate` این فایل را می‌نویسند، زیر قفل `flock /run/baft/install.lock` و به‌صورت اتمی (فایل موقت، fsync، rename).
- **`release-state.json` جدا می‌ماند:** این فایل امنیتی و فقط رو به جلوست (ضد downgrade و ضد تکرار فهرست ابطال)، و ادغامش با یک فایل تنظیمات قابل ویرایش ریسک دارد.

## ۷. موتور Plan و Apply

**وضعیت مطلوب** (`node.json`) + **وضعیت مشاهده‌شده** (دیسک، systemd، nft) ← Plan ← Apply ← Verify ← در صورت شکست، Undo.

- **قالب گام‌ها:** Plan فهرستی از گام‌های نوع‌دار است، مثل:
  - `file.write`، `dir.ensure`، `user.ensure`
  - `unit.write`، `unit.enable`، `unit.restart`
  - `symlink.switch`
  - `nft.chain.ensure`، `sysctl.write`

  هر گام چهار بخش دارد: **پیش‌شرط** (چیزی که باید هنوز درست باشد)، **اجرا**، **بررسی** و **Undo**.
- **دفترچهٔ اجرا (journal):** پیش از هر گام، نیت آن در `/var/lib/baft/install/journal/` نوشته و fsync می‌شود. اگر برق وسط کار قطع شود، اجرای بعدی یا کار را تمام می‌کند یا آن را برمی‌گرداند. هیچ‌وقت نصب را نیمه‌کاره رها نمی‌کند. این همان الگوی journal و fence در ingress است، که روی سرور واقعی آزموده شده است.
- **همان قراردادهای امروز:**
  - `--plan` بدون root، بدون شبکه و بدون هیچ نوشتنی کار می‌کند.
  - خروجی `--json` پایدار است و در تست‌ها به‌عنوان golden مقایسه می‌شود.
  - اجرای دوباره روی سرور سالم هیچ کاری نمی‌کند و هیچ سرویسی را restart نمی‌کند.
  - رازها و pairing هرگز خودبه‌خود عوض نمی‌شوند.
- **حالت‌های نصب امروز** (FRESH، CURRENT، UPGRADE_AVAILABLE، REPAIR_REQUIRED و بقیه) از مقایسهٔ دو وضعیت به دست می‌آیند، نه از حدس زدن روی فایل‌ها.

## ۸. آپدیت و rollback

**آپدیت محلی** (`baft update`):
1. Release به `/opt/baft/versions/<v>.partial/` دانلود می‌شود. منبع به ترتیب: آینهٔ پنل، `--mirror`، GitHub یا `--offline`.
2. تأیید کامل: گواهی، فهرست ابطال، امضای manifest، SHA256SUMS و ضد downgrade و re-tag. اگر تأیید شکست بخورد، پوشهٔ `.partial` حذف می‌شود و **هیچ چیز** عوض نمی‌شود.
3. `rename(.partial → <v>)`، سپس `symlink.switch current → <v>`.
4. restart به ترتیب امن: **پنل ← agent ← تونل‌هایی که نصب‌کننده ساخته**. بعد از هر restart، سلامت همان جزء بررسی می‌شود:
   - **پنل:** خود پنل روی آدرسش جواب بدهد.
   - **agent:** active باشد و ظرف ۶۰ ثانیه یک poll احرازشده به BCC داشته باشد. agent زمان آخرین poll موفق را در `/run/baft/agent.status` می‌نویسد.
   - **تونل:** `baft doctor`.
5. اگر هرکدام شکست بخورد: symlink به نسخهٔ قبل برمی‌گردد، همان سرویس‌ها restart می‌شوند و نتیجه ثبت می‌شود.
6. `release-state.json` فقط پس از موفقیت جلو می‌رود.

**تونل‌هایی که BCC ساخته** به‌طور پیش‌فرض restart نمی‌شوند، چون اتصال کاربرها قطع می‌شود. آن‌ها نسخهٔ جدید را در restart بعدی می‌گیرند. `--restart-tunnels` یا دکمهٔ جدا در پنل آن‌ها را restart می‌کند. (تصمیم باز ۳)

**آپدیت از پنل:**
- agent فقط یک **سرویس موقت** می‌سازد: `systemd-run --unit baft-update-<v> baft update --version <v> --yes --from-agent`، و پاسخ می‌دهد که آپدیت شروع شد.
- خود آپدیت، بررسی سلامت و rollback در همان سرویس موقت انجام می‌شود، **بیرون از پروسهٔ agent**. پس restart شدن agent وسط کار مشکلی ایجاد نمی‌کند.
- نتیجهٔ نهایی را agent جدید (یا agent برگشته) در poll بعدی گزارش می‌کند.
- این طرح جای «نگهبان + marker» را می‌گیرد که در #305 برای v0.5.1 ساخته شد. آن راه موقت و درست است، ولی این طرح یک مسیر برای آپدیت محلی و آپدیت از پنل دارد.

**سرور پنل:**
- `baft setup` روی سرور پنل همیشه یک agent محلی هم نصب می‌کند که خودکار به همان پنل وصل می‌شود.
- در نتیجه سرور پنل هم مثل بقیه از صفحهٔ Updates آپدیت می‌شود. پنل نمی‌تواند خودش را مستقیم آپدیت کند، چون sandbox است و پروسه‌ای که باید rollback کند خودِ پنل است. اینجا agent محلی این کار را از بیرون انجام می‌دهد. (تصمیم باز ۲)

**rollback دستی:** `baft rollback` symlink را به نسخهٔ قبلیِ نگه‌داشته‌شده برمی‌گرداند و سلامت را دوباره بررسی می‌کند. در state، downgrade فقط با `--allow-downgrade` و پیام صریح ممکن است، مثل امروز.

## ۹. راه‌انداز: `install.sh` حدود ۱۵۰ خط

<div dir="ltr" align="left">

```bash
bash <(curl -Ls https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh)
bash <(curl -Ls https://<panel>/install.sh) --mirror https://<panel>/releases   # Iran
```

</div>

کار راه‌انداز:
1. بررسی root، systemd، معماری و توزیع (§۱۵).
2. دانلود `manifest.json`، `release-key.cert.json`، `SHA256SUMS`، `baft-linux-<arch>` و فهرست ابطال.
3. تأیید با `openssl pkeyutl -verify -rawin` (Ed25519، OpenSSL 3) در برابر کلید ریشه‌ای که **داخل خود اسکریپت** ثابت شده است. بعد تأیید SHA-256 باینری. شکست یعنی توقف، بدون هیچ تغییری.
4. قرار دادن باینری در `/opt/baft/versions/<v>/` و `exec baft setup "$@"`.

- **وابستگی‌ها:** پس از این مرحله دیگر python3، acme.sh، apt و Go لازم نیست. فقط `curl` و `openssl` برای خود راه‌انداز، که روی Ubuntu و Debian پشتیبانی‌شده نصب‌اند.
- **اعتماد به خود اسکریپت:** اسکریپتی که از `main` گرفته شود فقط به TLS گیت‌هاب متکی است، مثل همهٔ اسکریپت‌های `curl | bash`. برای همین:
  - اسکریپت هر Release هم به‌عنوان asset امضاشده منتشر می‌شود.
  - مستندات نسخهٔ pin‌شده (`/releases/download/vX/install.sh`) را هم ارائه می‌دهند.
  - کلید ریشهٔ داخل اسکریپت با کلید داخل باینری یکی است. هرکدام عوض شود، دیگری آن را رد می‌کند.
- **ثبت سرور از پنل با SSH:** پنل باینری تأییدشده را خودش از طریق SSH می‌فرستد و `baft setup --agent ... --offline` را اجرا می‌کند. سرور ایران برای ثبت شدن به GitHub نیاز ندارد.

## ۱۰. پنل به‌عنوان آینهٔ Release

- **چه چیزی سرو می‌شود:** فقط Releaseهایی که در Update Center وارد و تأیید شده‌اند، روی `https://<panel>/releases/<v>/` با همان فایل‌های امضاشده. پنل فایل خودش را امضا نمی‌کند و هر سرور همه‌چیز را دوباره با کلید ریشه تأیید می‌کند.
- **نتیجه:** دسترسی به پنل جای دسترسی به GitHub را می‌گیرد، ولی اعتماد همچنان از امضاست، نه از منبع دانلود.
- **دیگر منابع:**
  - `--mirror URL` برای هر آینهٔ دیگری.
  - `--offline DIR` (بستهٔ آفلاین امروز) برای سرور بی‌اینترنت.

## ۱۱. دفتر مالکیت و Uninstall دقیق

- **ثبت هنگام ساخت:** هر گام apply که چیزی بسازد (فایل، پوشه، unit، کاربر، شیء nft، فایل sysctl)، آن را با مسیر، نوع و SHA-256 در `/var/lib/baft/install/ledger.json` ثبت می‌کند.
- **پاک‌سازی:** `baft uninstall` فقط اقلام همین دفتر را پاک می‌کند:
  - اگر محتوای یک فایل از زمان ساخت تغییر کرده باشد، آن را نشان می‌دهد و بدون تأیید پاکش نمی‌کند.
  - پوشه فقط وقتی حذف می‌شود که خالی شود.
  - دسته‌بندی دادهٔ امروز (نگه داشتن کانفیگ، گواهی، پشتیبان و audit) و پشتیبان اضطراری پنل حفظ می‌شود.
- **نتیجه:** باقی‌مانده‌هایی که LAB واقعی نشان داد دیگر ممکن نیست. چیزی که BAFT نساخته هرگز حذف نمی‌شود، و این همان تضمینی است که هم‌زیستی با BAFT تولیدی روی سرور خارج به آن نیاز داشت.

## ۱۲. unitهای systemd

| unit | کاربر و دسترسی | توضیح |
|---|---|---|
| `baft@<name>.service` | `baft`، فقط پورت‌های خودش | تونل CLI؛ instanceهای BCC از همین الگو |
| `baft-agent.service` | root، `ProtectSystem=strict` با مسیرهای نوشتنی مشخص | همان سخت‌گیری امروز |
| `baft-panel.service` | کاربر `baft-panel`، `CAP_NET_BIND_SERVICE` | ACME داخلی، بدون نیاز به `/root` |
| `baft-recover.service` | oneshot هنگام بوت | بازیابی تغییرهای نیمه‌کاره |
| `baft-ingress-chain.service` | oneshot | فقط وقتی ingress فعال است |
| `baft-update-<v>.service` | موقت (`systemd-run`) | فقط هنگام آپدیت از پنل |
| `baft.target` | — | همهٔ سرویس‌های BAFT را با هم start و stop می‌کند |

**نام‌گذاری در گذار:** نام unitهای فعلی (`baft-bcc.service`، `baft.service`) در نصب‌های migrate‌شده حفظ می‌شوند و نام جدید فقط در نصب تازه است. `node.json` نام واقعی هر unit را ثبت می‌کند.

## ۱۳. گواهی پنل و ابزارهای کمکی

- **گواهی:** ACME (Let's Encrypt برای دامنه و برای IP) داخل `baft panel` با Go انجام می‌شود، با تمدید خودکار و چالش http-01 روی پورت 80 فقط در لحظهٔ صدور. با این کار acme.sh، تایمر `baft-bcc-cert` و `/root/.acme.sh` حذف می‌شوند. نصب‌های فعلی تا تمدید بعدی همان گواهی موجود را نگه می‌دارند.
- **اسکریپت‌های کمکی:** `baft-net` و `baft-panel` (اسکریپت‌های shell تولیدشده) به `baft net` و منوی `sudo baft` منتقل می‌شوند.

## ۱۴. مهاجرت از v0.5

- **`baft migrate`** اولین بار به‌طور خودکار با `baft update` اجرا می‌شود، با Plan قابل‌نمایش و `--plan`:
  1. نصب فعلی را می‌خواند: unitها و `ExecStart`، فایل‌های agent و پنل، کانفیگ تونل و `release-state.json`. این تجزیه فقط یک بار و در یک جا انجام می‌شود، با تست‌های fixture از نصب‌های واقعی v0.4 و v0.5.
  2. `node.json` را با **همان مسیرهای موجود** می‌نویسد. هیچ داده‌ای جابه‌جا نمی‌شود: نه پایگاه دادهٔ پنل، نه رازها و نه کانفیگ تونل.
  3. دفتر مالکیت را از اقلامی که بی‌تردید مال BAFT هستند می‌سازد (unitهای دارای علامت `# baft-managed` و مسیرهای استاندارد). موارد مبهم «مالک نامعلوم» ثبت می‌شوند و uninstall فقط با تأیید صریح به آن‌ها دست می‌زند.
  4. باینری‌ها به چیدمان `versions/` منتقل می‌شوند و لینک‌های سازگاری در مسیرهای قدیمی ساخته می‌شوند.
  5. اگر unitها باید معادل‌سازی شوند، تفاوتشان نمایش داده می‌شود.
- **پنجرهٔ سازگاری:** در سراسر v0.6، `install.sh` همهٔ گزینه‌های امروز (`--role`، `--agent-only`، `--bcc-only`، `--update` و بقیه) را می‌پذیرد و به `baft setup` و `baft update` تبدیل می‌کند. v0.7 این لایه را حذف می‌کند.
- **agentهای قدیمی:** agentهای v0.5.x با job `update_baft` همچنان کار می‌کنند، چون نام‌های قدیمی artifact امضا می‌شوند (§۴). اولین آپدیت آن‌ها به v0.6، migrate را هم اجرا می‌کند.

## ۱۵. محیط‌های پشتیبانی‌شده

- **توزیع‌ها:** Ubuntu 22.04 و 24.04، و Debian 12. Debian 13 پس از آزمون اضافه می‌شود.
- **معماری‌ها:** amd64 و arm64.
- **نرم‌افزار پایه:** systemd ≥ 249 و OpenSSL 3. nftables فقط برای ingress لازم است.

هر محیط دیگر با پیام روشن رد می‌شود، نه با خطای نیمه‌راه.

## ۱۶. آزمون

1. **واحد (Go):**
   - موتور Plan با جدول حالت‌ها و خروجی golden `--plan --json`.
   - Undo هر گام.
   - تجزیهٔ migrate روی fixtureهای نصب واقعی v0.4 و v0.5.
2. **تزریق خطا:** قطع کار پس از هر گام apply. اجرای بعدی باید کار را تمام کند یا برگرداند، بدون هیچ اثر نیمه‌کاره. این گسترش `installer_faults.sh` امروز است.
3. **هم‌ارزی در دورهٔ گذار:** روی fixtureهای یکسان، Plan اسکریپت فعلی و `baft apply` مقایسه می‌شوند. هر تفاوت یا رفع آگاهانهٔ یک باگ است، یا خطا.
4. **ماتریس VM:** سه توزیع روی LAB دومیزبانهٔ KVM که الان داریم. سناریوها: نصب تازه، ارتقا از v0.4.0، v0.5.0 و v0.5.1، rollback، قطع برق وسط آپدیت (kill سخت VM) و uninstall با بررسی «هیچ باقی‌مانده‌ای نیست».
5. **LAB سرورهای واقعی:** کل مسیر، به‌اضافهٔ آپدیت از پنل و آپدیت سرور پنل از طریق agent محلی آن.

## ۱۷. ملاحظات امنیتی

- **زنجیرهٔ اعتماد:** بدون تغییر است: ریشهٔ ثابت، گواهی کلید انتشار، فهرست ابطال تاریخ‌دار، ضد downgrade و re-tag. فقط محل تأیید از bash و python به Go منتقل می‌شود. یک تأییدکننده باقی می‌ماند (`internal/release`) که امروز هم agent و پنل از آن استفاده می‌کنند.
- **جداسازی دسترسی:** باینری واحد دسترسی‌ها را یکی نمی‌کند. هر سرویس با کاربر و capability خودش اجرا می‌شود (§۱۲).
- **محدودیت آپدیت از پنل:** سرویس موقت آپدیت فقط `baft update` با نسخه‌ای را اجرا می‌کند که در job امضاشدهٔ BCC آمده است. پارامتر آزاد یا مسیر دلخواه از راه دور پذیرفته نمی‌شود، همان قاعدهٔ allowlist پارامترهای job.
- **دسترسی‌های root:** `--plan` و `status` نیازی به root ندارند. apply فقط با root اجرا می‌شود.

## ۱۸. گزینه‌های بررسی‌شده

| گزینه | چرا انتخاب نشد |
|---|---|
| ادامهٔ bash و بازسازی تدریجی | منطق تراکنشی، journal و تست واحد در bash عملی نیست. ریشهٔ بیشتر باگ‌های نصب همین است |
| پکیج `.deb` و مخزن apt | سرور ایران به مخزن خارجی دسترسی پایدار ندارد، و دو نظام اعتماد (GPG مخزن و ریشهٔ BAFT) کنار هم می‌مانند. ممکن است بعداً **از همین چیدمان** ساخته شود، چون `/usr/lib/systemd` و `/opt/baft` با ساختار پکیج سازگارند |
| Docker | با شبکهٔ میزبان، nft و ingress امن (ADR-003) تداخل دارد، و بسیاری از VPSها Docker ندارند |
| باینری‌های جدا با یک نصب‌کنندهٔ یکپارچه | جدا افتادن نسخه‌ها و سه مسیر آپدیت باقی می‌ماند |

## ۱۹. تصمیم‌های باز برای مالک

1. **محل unitها:** `/usr/lib/systemd/system` (پیشنهاد) یا ادامهٔ `/etc/systemd/system`.
2. **agent محلی روی سرور پنل:** همیشه نصب شود (پیشنهاد)، یا اختیاری باشد.
3. **restart تونل‌های BCC هنگام آپدیت:** پیش‌فرض نه (پیشنهاد)، با دکمهٔ جدا.
4. **ACME داخل پنل** به‌جای acme.sh.
5. **فهرست توزیع‌ها:** Ubuntu 22.04 و 24.04 و Debian 12؛ آیا Debian 11 یا Ubuntu 20.04 هم لازم است؟ (هر دو OpenSSL 3 ندارند.)
6. **پنجرهٔ سازگاری:** یک نسخهٔ کامل (v0.6) برای گزینه‌های `install.sh` و نام‌های قدیمی artifact.

## ۲۰. فازها و معیار پذیرش

| فاز | محتوا | معیار پذیرش |
|---|---|---|
| ۱ | باینری واحد و تشخیص حالت از `argv[0]`؛ artifactهای سازگاری | همهٔ LABهای فعلی بدون تغییر سبز؛ حجم اندازه‌گیری‌شده |
| ۲ | `node.json`، موتور Plan و Apply با journal، `baft migrate` | تست‌های واحد و تزریق خطا؛ هم‌ارزی Plan با اسکریپت فعلی؛ migrate روی fixtureهای v0.4 و v0.5 |
| ۳ | چیدمان `versions/`، `baft update` و `baft rollback`، سرویس موقت آپدیت، agent محلی پنل | ماتریس VM ارتقا و rollback و قطع برق؛ آپدیت سرور پنل از صفحهٔ Updates |
| ۴ | راه‌انداز ۱۵۰ خطی، آینهٔ پنل، ثبت SSH بدون GitHub، ACME داخلی | نصب سرور ایران فقط از طریق پنل در LAB واقعی |
| ۵ | دفتر مالکیت و uninstall دقیق | uninstall روی هر سه توزیع بدون باقی‌مانده؛ هم‌زیستی با BAFT تولیدی در LAB واقعی |
| ۶ (v0.7) | حذف لایهٔ سازگاری `install.sh` و نام‌های قدیمی | — |

**رابطه با کار جاری:** #305 (آپدیت یکپارچه با `install.sh --update` و آپدیت agent از پنل با نگهبان) مشکل امروز را برای v0.5.1 حل می‌کند و جایگزین این طرح نیست. در فاز ۳، مسیر نگهبان با سرویس موقت `baft update` جایگزین می‌شود.

</div>
