<div dir="rtl" align="right" lang="fa">

<p align="left" dir="ltr"><a href="README.md"><strong>English →</strong></a></p>

<p align="center">
  <img src="docs/assets/baft-readme-poster-fa.webp" alt="BAFT — بافت" width="100%">
</p>

<h1 align="center">BAFT — بافت</h1>

<p align="center" dir="ltr">
  <a href="https://github.com/zarkmakerburg/baft/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/zarkmakerburg/baft?style=flat-square&label=release"></a>
  <a href="https://github.com/zarkmakerburg/baft/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/zarkmakerburg/baft/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <img alt="License: AGPL-3.0" src="https://img.shields.io/badge/license-AGPL--3.0-blue?style=flat-square">
</p>

<p align="center">
  <a href="README.md">English</a> ·
  <a href="docs/fa/guide/README.md">راهنمای کاربر</a> ·
  <a href="https://github.com/zarkmakerburg/baft/releases/latest">آخرین نسخه</a> ·
  <a href="CHANGELOG.md">تغییرات نسخه‌ها</a>
</p>

**بافت یک تونل امن و پایدار از سرور داخل ایران به سرور خارج است، برای کسانی که روی سرور خارج x-ui یا 3x-ui (یا هر سرویس TCP دیگری) دارند؛ همراه با یک پنل وب برای مدیریت چند سرور.**

بافت پنل فروش کاربر و اشتراک (مثل مرزبان) نیست. سرویسی را که همین الان دارید از ایران به خارج می‌رساند و به x-ui شما دست نمی‌زند.

## ویژگی‌های اصلی

- **نصب با یک خط** روی سرور خارج؛ سرور ایران را هم می‌تواند با SSH از همان‌جا نصب کند.
- **عبور پورت‌های x-ui**: سرور ایران روی همان پورت‌ها گوش می‌دهد و آن‌ها را به سرور خارج می‌رساند. کاربر فقط آدرس سرور را عوض می‌کند.
- **تست مسیر پیش از جفت‌شدن**: بررسی می‌کند مسیر ایران به خارج واقعاً داده رد می‌کند، نه فقط این‌که پورت باز است.
- **پنل وب (BCC)** برای چند سرور، روی دامنه یا IP با گواهی Let's Encrypt.
- **تنظیم شبکه**: روشن و خاموش کردن BBR و بافرهای TCP با یک دستور، به‌همراه تست سرعت، مسیر و پورت.
- **امنیت**: نسخه‌های امضاشده که با کلید ریشهٔ آفلاین وارسی می‌شوند، TLS دوطرفه بین سرورها، ورود پنل با Argon2id، و jobهای امضاشدهٔ agent که هرگز shell اجرا نمی‌کنند.
- **اجرای دوباره و حذف بی‌خطر**: اجرای دوباره اول برنامهٔ تغییرات را نشان می‌دهد و اگر خطا بخورد برمی‌گرداند؛ حذف فقط چیزهایی را پاک می‌کند که ثابت شود مال بافت است.

## وضعیت انتشار و توسعه

**آخرین نسخهٔ عمومی منتشرشده: [v0.5.0](https://github.com/zarkmakerburg/BAFT/releases/tag/v0.5.0)** (انتشار در ۲۰۲۶-۱۱-۰۲). سرشاخهٔ فعلی `main` روی [`d4348c2ace51`](https://github.com/zarkmakerburg/BAFT/commit/d4348c2ace51e56469b10aee2806f3ad4869c8fa) است و تغییرهای پس از v0.5.0 را در بخش **[انتشارنیافته](CHANGELOG.md)** دارد؛ از جمله به‌روزرسانی یکپارچهٔ اجزای سرور، خودبه‌روزرسانی Agent و ابطال/پین‌کردن گواهی در سمت Dialer. این تغییرها هنوز **نسخهٔ منتشرشده نیستند**. تا انتشار رسمی نسخهٔ امضاشدهٔ بعدی، برای نصب عملیاتی از آخرین نسخهٔ عمومی امضاشده استفاده کنید.

**وضعیت P0:** [Issue #269](https://github.com/zarkmakerburg/BAFT/issues/269) همچنان باز است. نسخهٔ v0.5.0 راهنمای ساخت تونل، Public Ingress اختیاری و عملیات tenant روی مسیر Shared Transport را دارد، اما پذیرش کامل P0 بسته نشده است. به‌ویژه، وجود مسیر Shared Transport فعلی به معنی پیاده‌سازی یا پشتیبانی عمومی از `multiplex-v1` نیست. نامزد یکپارچه‌سازی Ingress در [PR #278](https://github.com/zarkmakerburg/BAFT/pull/278) هنوز Draft است و یافته‌های حل‌نشدهٔ fencing در [برد HQ](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6861034383) ثبت شده‌اند.

**وضعیت مهندسی:** Stage A و B در محدودهٔ تعریف‌شدهٔ خود کامل‌اند. workflow مربوط به soak مرحلهٔ C روی SHA فعلی `main` موفق شده است ([اجرای 43472903062](https://github.com/zarkmakerburg/BAFT/actions/runs/43472903062))؛ بستن رسمی Stage C همچنان تصمیم پذیرش جداگانه است. Stage D کامل نیست: ECRL فقط تعویض Carrier نشست زنده در همان process را پشتیبانی می‌کند؛ ادامهٔ نشست پس از restart فرایند، reboot ماشین و snapshot پایدار نشست پیاده نشده‌اند. بافت Production-ready اعلام نشده و SHA فعلی `main` به‌تنهایی شاهد پایلوت میدانی در مقیاس نیست.

۹ اجرای GitHub Actions که در [وضعیت پروژه](STATUS.md) فهرست شده‌اند، روی همین SHA دقیق `main` موفق شده‌اند. این گزاره به معنی اجرا یا موفقیت همهٔ workflowهای مخزن روی این SHA نیست.

- `install.sh`: نصب، به‌روزرسانی و ساخت ابزارهای مدیریتی `baft-panel` و `baft-net`.
- `internal/bcc/`: backend پنل، طرح‌های بازبینی‌شدهٔ تونل، سیاست Public Ingress و هماهنگی عملیات.
- `web/`: رابط مرورگری BCC، از جمله راهنمای ساخت تونل.
- `tests/installer/` و `tests/e2e/`: تست‌های قراردادی نصب‌کننده و workflowهای end-to-end.
- `docs/design/` و `docs/roadmap/`: تصمیم‌های طراحی، برنامهٔ پیاده‌سازی و معیارهای پذیرش؛ سند طراحی به‌تنهایی اثبات قابلیت منتشرشده نیست.

برای جزئیات ببینید: [وضعیت پروژه](STATUS.md)، [برنامهٔ توسعه](PLAN.md)، [محدودیت‌های شناخته‌شده](KNOWN-LIMITATIONS.md) و [تغییرات نسخه‌ها](CHANGELOG.md).



روی سرور **خارج** (Debian یا Ubuntu، با دسترسی root):

<div dir="ltr" align="left">

```bash
bash <(curl -Ls https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh)
```

</div>

گزینهٔ **۱ (خارج از ایران)** را بزنید و به پرسش‌ها جواب دهید. راهنمای کامل: [نصب](docs/fa/guide/01-install.md).

## نصب

## استفاده با x-ui در ۴ قدم

1. نصب‌کننده را روی سرور خارج (همان‌جا که x-ui هست) اجرا کنید، **۱** را بزنید و پورت‌هایی را که باید عبور کنند بدهید، مثلاً `443,2053`. خودش پورت‌هایی را که سرور الان روی آن‌ها گوش می‌دهد (مثل inboundهای x-ui) پیشنهاد می‌کند. در اسکریپت: `--ports 443,2053`.
2. بگذارید سرور ایران را با SSH نصب کند (یا همان خط را روی سرور ایران اجرا کنید و کد جفت‌شدن را وارد کنید). پیش از جفت‌شدن، مسیر ایران به خارج تست می‌شود.
3. حالا سرور ایران روی هر پورت عبوری گوش می‌دهد و آن را از داخل تونل به همان پورت روی سرور خارج می‌رساند.
4. در لینک‌های x-ui فقط آدرس سرور را به **IP سرور ایران** تغییر دهید. پورت همان می‌ماند.

جزئیات: [استفاده با x-ui](docs/fa/guide/02-use-with-x-ui.md).

## چه چیزی کار می‌کند و محدودیت‌ها

**کار می‌کند** (TCP): VLESS، VMess و Trojan روی TCP، WebSocket، gRPC و HTTPUpgrade؛ حتی UDPای که *داخل* همین پروتکل‌ها حمل می‌شود (مثل DNS یا بازی از داخل VLESS). هر سرویس TCP ساده‌ی دیگری هم کار می‌کند.

**کار نمی‌کند** (مبتنی بر UDP): Hysteria2، TUIC، WireGuard و هر چیزی که UDP خام لازم دارد. بافت فقط TCP را عبور می‌دهد.

**وضعیت، بی‌تعارف**: بافت تازه است. در CI و محیط آزمایشی تست شده، اما **هنوز در میدان و در مقیاس بزرگ ثابت نشده است**؛ هیچ آزمایش واقعی بزرگی بین ایران و خارج انجام نشده. شبکه در دیتاسنترهای ایران یکسان نیست: دست‌کم از یک دیتاسنتر ایرانی، اتصال TCP به خارج روی پورت‌هایی غیر از ۲۲ پس از حدود ۸٫۷ کیلوبایت قطع می‌شد. پیش از تکیه بر آن، مسیرتان را با `sudo baft-net` تست کنید. ببینید: [محدودیت‌های شناخته‌شده](KNOWN-LIMITATIONS.md).

## دستورهای مدیریت

| دستور | کجا | کار |
|---|---|---|
| `baft` | هر سرور | منوی وضعیت: وضعیت، doctor، لاگ، گواهی‌ها، بستهٔ پشتیبانی، حذف |
| `sudo baft-panel` | سرور پنل | لینک و نام کاربری پنل، ورود تازه، ری‌استارت، لاگ، دامنه و SSL، به‌روزرسانی |
| `sudo baft-net` | هر سرور | روشن و خاموش کردن BBR، تست سرعت، تأخیر، مسیر (mtr)، تست پورت |
| `baft doctor` / `baft version` | هر سرور | بررسی سلامت / نسخهٔ نصب‌شده |
| `journalctl -u baft -f` | هر سرور | لاگ زندهٔ تونل |

## مستندات

- **[راهنمای کاربر](docs/fa/guide/README.md)**: [نصب](docs/fa/guide/01-install.md)، [x-ui](docs/fa/guide/02-use-with-x-ui.md)، [پنل](docs/fa/guide/03-panel.md)، [شبکه و BBR](docs/fa/guide/04-network-and-bbr.md)، [به‌روزرسانی، پشتیبان و حذف](docs/fa/guide/05-update-backup-uninstall.md)، [عیب‌یابی](docs/fa/guide/06-troubleshooting.md)، [پرسش‌های رایج](docs/fa/guide/07-faq.md)
- [مستندات فنی](docs/fa/README.md) و [یادداشت‌های مهندسی](docs/fa/40-engineering-notes.md) (معماری، پروتکل، مراحل توسعه، شواهد CI)
- [تغییرات نسخه‌ها](CHANGELOG.md) · [محدودیت‌های شناخته‌شده](KNOWN-LIMITATIONS.md) · [وضعیت پروژه](STATUS.md)

## پشتیبانی و امنیت

- گزارش خطا و پرسش: [Issueهای گیت‌هاب](https://github.com/zarkmakerburg/baft/issues/new/choose). هرگز کد جفت‌شدن، رمز یا کلید را در آن‌جا نگذارید.
- آسیب‌پذیری امنیتی: به‌صورت خصوصی گزارش دهید، [SECURITY.md](SECURITY.md) را ببینید. لطفاً Issue عمومی باز نکنید.

## مجوز

بافت نرم‌افزار آزاد تحت مجوز [GNU AGPL-3.0](LICENSE) است. نام و لوگوی بافت علامت تجاری‌اند و زیر AGPL مجوز داده نمی‌شوند؛ [COPYRIGHT](COPYRIGHT) را ببینید. مجوز کتابخانه‌های دیگران: [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).

## حمایت از پروژه

اگر بافت برایتان مفید است، لطفاً **[در گیت‌هاب به آن ستاره بدهید](https://github.com/zarkmakerburg/baft/stargazers)**؛ این کار به دیده‌شدن پروژه کمک می‌کند.

☕ **یک قهوه مهمانم کنید**: حمایت مالی با ارز دیجیتال:

<div dir="ltr" align="left">

| Network | Wallet address |
|---|---|
| TRON (TRC-20) | `TP13BYbfbcFbtomvdLc9rKxAWbWyk195VH` |
| TON | `UQAN4TZrv29BRagXzFD_4_tkWJvx1wf8mIEeFHymwhlIatLa` |

</div>

پیش از ارسال، آدرس و شبکه را دوباره بررسی کنید؛ تراکنش‌ها برگشت‌پذیر نیستند.

</div>
