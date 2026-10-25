<div dir="rtl" align="right" lang="fa">

# محدودیت‌های شناخته‌شده

> English: [KNOWN-LIMITATIONS.en.md](KNOWN-LIMITATIONS.en.md)

- BAFT هنوز research software است و production-ready اعلام نشده است.
- Stage-C soak فعلی برای multi-Flow/slow-receiver و race sample سبز است؛ با این حال benchmark عمومی، پایلوت واقعی و production readiness هنوز اثبات نشده‌اند.
- COR-01 پاس‌شده correctness روی runner/local networking است و benchmark اینترنت عمومی نیست.
- allocator، TWRL، PADL و control scheduling وارد data path شده‌اند؛ shared-budget چند Shard و soak تکرارشونده فعلی تست شده‌اند.
- تعویض Carrier در همان process، epoch fencing در همان process و bounded replay در Step 5.7 پیاده‌سازی و تست شده‌اند.
- snapshot پایدار ECRL، resume بعد از process restart و resume بعد از reboot ماشین پیاده‌سازی یا ادعا نشده‌اند.
- endpoint pool/relay production path کامل نشده است.
- H3 و Worker جزو مسیر پایه فعال نیستند.
- benchmark رسمی 60s × 5 و profiler campaign کامل نشده است.
- نصب‌کنندهٔ قابل اجرای مجدد (بررسی، plan، اعمال، وارسی و بازگرداندن config، باینری‌ها و وضعیت سرویس systemd؛ سند 33)، حذف امن (سند 34)، بستهٔ پشتیبانی بدون راز (`baft support-bundle`) و transactional certificate rotation در A4 پیاده‌سازی و در CI روی systemd تست شده‌اند؛ اما روی مجموعهٔ بزرگی از سرورهای واقعی آزموده نشده‌اند. Update و Repair در منوی `baft` هنوز برنامه‌ریزی‌شده‌اند (از نصب‌کننده استفاده کنید).
- **فقط TCP.** بافت جریان TCP را عبور می‌دهد و UDP را عبور نمی‌دهد. VLESS/VMess/Trojan روی TCP، WebSocket، gRPC یا HTTPUpgrade کار می‌کنند (حتی UDPای که داخل آن‌ها حمل می‌شود)؛ Hysteria2، TUIC، WireGuard و دیگر پروتکل‌های UDP خام کار نمی‌کنند.
- هیچ پایلوت واقعی بزرگی بین ایران و EX اجرا نشده است؛ بافت تازه است و در میدان و در مقیاس بزرگ ثابت نشده. دست‌کم از یک دیتاسنتر ایرانی، TCP به خارج روی پورت‌هایی غیر از ۲۲ پس از حدود ۸٫۷ کیلوبایت قطع می‌شد، پس هر مسیر باید تست شود (`sudo baft-net`).
- هیچ ادعای عمومی درباره سرعت تضمینی، تشخیص‌ناپذیری یا دسترسی در همه شرایط اثبات نشده است.
- listener عمومی IR به‌صورت خودکار کاربر نهایی را authenticate نمی‌کند؛ اگر از loopback خارج شود، امنیت لایه سرویس جداگانه لازم است.


## وضعیت qualification Step 5.7

Baseline `29125392566021a286af20ff8f3a4907c28dbee8` پنج Recovery Soak مستقل، Stage-C soak و Full CI را پاس کرده است. این وضعیت freeze candidate است و ACCEPT/FREEZE رسمی با HQ است؛ محدودیت‌های scope زیر بدون تغییر باقی می‌مانند.

## محدودیت‌های Public Ingress در v0.4.0
adopt کردن ingress زنده پس از reboot پیاده‌سازی نشده است (پس از reboot به Apply یا Retire امضاشدهٔ جدید نیاز است)، تغییرات جزئیِ نامشخص نیاز به reconciliation دارند، و نتایج LAB روی loopback یا namespace دسترس‌پذیری بیرونی را تأیید نمی‌کنند. کلاینتی که ابتدا چیزی نمی‌فرستد تا ۵۰ میلی‌ثانیه تأخیر اضافه در برقراری اتصال روی پورت ingress می‌بیند. «Update Center» داخل محصول وجود ندارد؛ برای ارتقا نصب‌کنندهٔ تأییدشده را دوباره اجرا کنید. بازگشت به نسخهٔ قدیمی‌تر از schema 14/15 در SQLite یا schema 3/4 در backup فقط با snapshot تأییدشدهٔ پیش از ارتقا ممکن است.

## محدوده Recovery در Step 5.7
اتصال ECRL به Runtime در این مرحله فقط برای **تعویض Carrier در همان process و همان Session زنده** است. epoch fencing همان process و bounded replay بر اساس Plan اعتبارسنجی‌شده پیاده‌سازی و تست شده‌اند. Commit recovery شامل آماده‌سازی کامل پیش از commit، Plan Digest canonical، barrier دوطرفه readiness/commit، هویت idempotent برای commit و مدیریت جداگانه خطاهای post-commit است. readiness نسل replacement نیز monotonic و generation-explicit است؛ application waiter فقط بعد از finalization، activation و reconciliation کامل همان generation آزاد می‌شود و stale generation حق rollback یا wake جعلی ندارد. snapshot بازیابی بین restart پردازه یا reboot ماشین پایدار نمی‌شود و هیچ ادعایی برای process-restart resume وجود ندارد. تغییر BootID سمت peer هنگام recovery به‌صورت fail-closed رد می‌شود. Subscription Engine خارج از scope است و Record Shaping / Morphing در Step 5.7 تغییری نکرده‌اند.

</div>
