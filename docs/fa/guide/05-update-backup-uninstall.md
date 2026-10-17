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

بررسی، release را در برابر کلید ریشهٔ پین‌شده، فهرست ابطال و release state همین سرور تأیید می‌کند (بدون downgrade و بدون re-tag). اگر نتواند دریافت یا تأیید کند `UNKNOWN` می‌گوید، هرگز «به‌روز است» نه. هر جزء نصب‌شده (سرویس انتقال، agent، BCC) را هم با **باینری‌های خودش** می‌سنجد: بایت‌به‌بایت با artifactهای release امضاشده مقایسه می‌شوند. release state بین Core و BCC مشترک است؛ پس پنلی که بعد از آپدیت Core روی باینری قدیمی مانده، `OUTDATED` نشان داده می‌شود و بررسی «آپدیت هست» می‌گوید، حتی اگر release state از قبل نسخهٔ تازه را ثبت کرده باشد. در منوی `baft` هم هست (گزینهٔ ۱۱).

بعد همان CLI اعمالش می‌کند. نصب‌کنندهٔ امضاشدهٔ خودِ release را برای هر جزئی که عقب است اجرا می‌کند؛ پس این یک نصب‌کنندهٔ دوم نیست:

<div dir="ltr" align="left">

```bash
baft update apply --plan   # no root: shows exactly what would change, changes nothing
sudo baft update apply     # shows the plan, asks you to type `update`, then applies
```

</div>

- **چه چیزی و به چه ترتیب:** BCC، بعد agent، بعد سرویس انتقال (اول صفحهٔ کنترل، تا شکست پیش از رسیدن به سرویس خودِ تونل‌ها متوقف شود). هر کدام در همان حالتی اجرا می‌شود که نصب شده: `--bcc-only`، `--agent-only` یا `--role ir|ex` که از config نود خوانده می‌شود. ورودی‌های agent (آدرس BCC، شناسهٔ نود، فایل توکن، کلید job پین‌شده) از unit خودش بازیابی می‌شود، پس چیزی پرسیده یا دوباره ساخته نمی‌شود. `--only transport,agent,bcc` بعضی را انتخاب می‌کند.
- **چه چیزی را رد می‌کند:** نصبی که سالم نیست (اول تعمیر کنید) و هر قدم نصب‌کننده که به کلید، گواهی، config، pairing، توکن، فایل دسترسی یا unit دست بزند. قدمی که این `baft` نمی‌شناسد هم رد می‌شود، نه اینکه رد شود برود؛ در آن حالت دستی اعمال کنید. برای BCC یک backup تازه هم لازم است (`*.baftbak` جدیدتر از ۲۶ ساعت در پوشهٔ backup خود BCC) یا عمداً `--skip-backup-check`.
- **ایمنی حین اجرا:** نصب‌کننده همانی است که تأیید و امضا شده (sha256 آن درست پیش از هر اجرا دوباره چک می‌شود) و نسخه روی همان نسخهٔ تأییدشده پین می‌شود. در session جدا و با خروجی در `/opt/baft/update-logs/` اجرا می‌شود؛ پس قطع SSH یا Ctrl-C نصب نیمه‌کاره نمی‌گذارد و خودش تمام می‌شود (یا rollback می‌کند). با `tail -f` روی log دنبالش کنید. شکست، اجرا را پیش از جزء بعدی متوقف می‌کند.
- **بعد از آن:** plan دوم باید چیزی برای تغییر نداشته باشد و باینری‌ها باید همان release امضاشده باشند؛ وگرنه همین را می‌گوید و با کد ۱ خارج می‌شود. سرویسی که باینری‌اش عوض شود restart می‌شود و لحظه‌ای قطع می‌شود.

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
