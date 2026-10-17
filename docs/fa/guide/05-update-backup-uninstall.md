<div dir="rtl" align="right" lang="fa">

# ۵ — به‌روزرسانی، پشتیبان و حذف

[فهرست راهنما](README.md) · [English](../../en/guide/05-update-backup-uninstall.md)

## به‌روزرسانی

اول بررسی کنید که release امضاشدهٔ تازه‌تری هست یا نه. این فقط می‌خواند و چیزی روی سرور تغییر نمی‌دهد:

<div dir="ltr" align="left">

```bash
baft update check                                     # exit 0: current, 3: update available, 1: unknown
baft update check --save-installer ./baft-install.sh  # also keep the verified, signed installer
```

</div>

بررسی، release را در برابر کلید ریشهٔ پین‌شده، فهرست ابطال و release state همین سرور تأیید می‌کند (بدون downgrade و بدون re-tag). اگر نتواند دریافت یا تأیید کند `UNKNOWN` می‌گوید، هرگز «به‌روز است» نه. به‌روزرسانی را **اعمال نمی‌کند**؛ با نصب‌کننده اعمال کنید، مثلاً `sudo BAFT_VERSION=vX.Y.Z bash ./baft-install.sh --plan` برای پیش‌نمایش و همان دستور بدون `--plan` برای اعمال. در منوی `baft` هم هست (گزینهٔ ۱۱).

همان خط نصب را روی هر سرور دوباره اجرا کنید (سرور خارج و ایران را با هم به‌روز کنید تا هر دو طرف یک نسخه داشته باشند):

<div dir="ltr" align="left">

```bash
bash <(curl -Ls https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh)
```

</div>

اجرای دوباره **نصب از نو نیست**. سرور را بررسی می‌کند، برنامهٔ تغییرات را نشان می‌دهد، فقط همان‌ها را تغییر می‌دهد و نتیجه را وارسی می‌کند؛ اگر وارسی رد شود، فایل‌های قبلی را برمی‌گرداند و هر چه در حال اجرا بود دوباره روشن می‌کند. هیچ‌وقت کلید یا گواهی را عوض نمی‌کند، config معتبر را بازنویسی نمی‌کند، دوباره جفت نمی‌کند و نسخهٔ قدیمی‌تر را نصب نمی‌کند (جلوگیری از downgrade).

با نسخهٔ دانلودشدهٔ نصب‌کننده (`curl -fsSLO https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh`):

<div dir="ltr" align="left">

```bash
sudo bash install.sh --plan          # show what would change, change nothing
sudo bash install.sh --yes           # apply without asking (scripts)
sudo bash install.sh --re-pair --yes # deliberately start a new pairing
```

</div>

اگر سرور ایران به گیت‌هاب دسترسی ندارد، با [بستهٔ آفلاین](01-install.md#offline-install) به‌روزش کنید.

پنل: `sudo baft-panel` ← ۸ (به‌روزرسانی).

جزئیات: [نصب‌کنندهٔ قابل اجرای مجدد](../33-rerunnable-installer.md).

## پشتیبان

- **پنل**: اگر `BAFT_BCC_BACKUP_KEY` تنظیم شده باشد، پنل پشتیبان رمزنگاری‌شده می‌نویسد (۷ روزانه و ۴ هفتگی نگه داشته می‌شوند). `baft-bcc restore-preview` بدون تغییر چیزی نشان می‌دهد بازگردانی چه چیزهایی را عوض می‌کند، و `baft-bcc restore` به‌صورت آفلاین بازمی‌گرداند. ببینید: [پشتیبان BCC](../29-bcc-backup-restore-preview.md).
- **سرورها**: فایل‌های مهم در `/etc/baft` (config، کلیدها، گواهی‌ها) و release state هستند. حذف استاندارد این‌ها را نگه می‌دارد، پس نصب دوباره همان سرور را بدون جفت‌شدن دوباره برمی‌گرداند.
- پیش از این‌که اجرای دوباره فایلی را عوض کند، نصب‌کننده آن را در `/opt/baft/backups/rerun-<time>/` کپی می‌کند (سه اجرای آخر نگه داشته می‌شود).

## حذف

<div dir="ltr" align="left">

```bash
baft uninstall --preview   # the exact plan; changes nothing
baft uninstall             # remove BAFT services and binaries, KEEP data
baft uninstall --full      # everything; each data class is asked separately (default NO)
```

</div>

همین کار از منوی `baft` (گزینهٔ ۱۳) هم انجام می‌شود. حذف فقط چیزهایی را پاک می‌کند که ثابت شود مال بافت است، تونل در حال اجرا را بدون پرسیدن متوقف نمی‌کند، پیش از پاک کردن دادهٔ پنل یک پشتیبان اضطراری وارسی‌شده می‌گیرد، و اگر وسط کار قطع شود می‌شود آن را برگرداند یا تمام کرد (`baft uninstall --restore` یا `--resume`). `sudo baft-net off` تنظیمات شبکه را برمی‌دارد.

جزئیات: [حذف امن](../34-safe-uninstall.md).

</div>
