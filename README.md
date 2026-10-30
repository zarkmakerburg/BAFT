بستهٔ نهایی امضای BAFT v0.4.0 برای macOS آماده است. حجم آرشیو ۴۱٬۸۴۱٬۷۰۱بایت، حدود۴۰MiB است و Intel/Apple Silicon را پوشش می‌دهد. این بسته بدون کلید خصوصی است و خودش چیزی منتشر یا Deploy نمی‌کند.

۱. فایل `BAFT-v0.4.0-macOS-signing.tar.gz` را در Downloads بگذارید و در Terminal صحت دانلود را بررسی و باز کنید:

```sh
cd "$HOME/Downloads"
printf '%s\n' '814d09d6fe141f502eae1b3be8862d9cb51d57aecb93a487326ea096df6e16b4  BAFT-v0.4.0-macOS-signing.tar.gz' | shasum -a 256 -c -
tar -xzf BAFT-v0.4.0-macOS-signing.tar.gz
```

فقط پس از پیام `OK` ادامه دهید. اگر Go و Python3 ندارید، با Homebrew نصب کنید: `brew install go python`.

۲. دستور دقیق زیر، tag/SHA/workflow و هر۱۰فایل را بدون کلید بررسی و بازسازی می‌کند؛ این بخش اینترنت لازم دارد:

```sh
cd "$HOME/Downloads/BAFT-v0.4.0-macOS-signing"
bash ./Sign-BAFT.command 9c32ba369fe2be2c549f2878d1de5dbf092c8f64
```

۳. پس از پیام موفقیت بررسی عمومی، شبکه را قطع کنید و Enter بزنید. مسیر کامل `release.key` موجود و گواهی معتبر `release-key.cert.json` را بدون کوتیشن وارد کنید. پس از موفقیت sign و verify، فقط فایل کوچک `signed-public-only.tar.gz` چاپ‌شده را برای ادامهٔ انتشار تحویل بدهید. این فایل شامل metadata عمومیِ امضاشده و رسیدهاست؛ ده باینری ثابت در HQ موجودند و دوباره ارسال نمی‌شوند. کلید روی Mac می‌ماند.

مرجع نسخه: tag `v0.4.0`، main SHA `9c32ba369fe2be2c549f2878d1de5dbf092c8f64` و [workflow رسمی](https://github.com/zarkmakerburg/BAFT/actions/runs/41046337283). همهٔ۱۳گیت لازم و۲۸job موفق و هر۱۰asset رسمی با بازسازی مستقل برابرند. امضای واقعی روی دستگاه شما و بررسی مجدد خروجی در HQ هنوز باقی است؛ این تحویل به معنی انتشار انجام‌شده نیست.

SHA-256 آرشیو:

`814d09d6fe141f502eae1b3be8862d9cb51d57aecb93a487326ea096df6e16b4`
