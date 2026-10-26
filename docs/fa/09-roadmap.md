<div dir="rtl" align="right" lang="fa">

# 09 — نقشه راه و مراحل A تا H

## فلسفه مراحل

هر Stage یک خروجی فنی و یک gate دارد. هدف این است که قابلیت‌های پرریسک یا performance work قبل از اثبات correctness/security وارد core نشوند.

## Stage A — قرارداد و spike

خروجی‌ها:

- ADRهای TLS/HTTP؛
- golden vector پروتکل؛
- config schema؛
- H2 full-duplex واقعی؛
- mTLS؛
- cancellation؛
- اثبات اتصال مستقل Shardها.

**وضعیت:** انجام‌شده برای scope تعریف‌شده.

## Stage B — انتقال امن پایه

خروجی‌ها:

- parser؛
- Route ثابت؛
- OPEN/DATA/ACK/WINDOW/FIN؛
- خطاهای ثابت؛
- identity/authorization؛
- active revocation؛
- CLI؛
- COR-01 یک GiB.

**وضعیت:** complete برای vertical slice تعریف‌شده.

## Stage C — کنترل منابع و چند Flow

خروجی‌های هدف:

- allocator سراسری؛
- receive/replay reservation؛
- DRR؛
- bounded control queue؛
- multi-Flow و multi-Shard؛
- metrics لازم؛
- receiver کند بدون OOM.

**وضعیت:** در حال اجرا و تثبیت. بخش‌های اصلی وارد data path شده‌اند، اما slow-receiver gate هنوز کامل سبز نیست.

## Stage D — resume دقیق

خروجی‌های طراحی:

- boot/session/epoch fencing؛
- snapshot state؛
- replay؛
- tombstone؛
- duplicate-free carrier replacement؛
- idempotency کامل OPEN.

نکته مهم: resume فقط تا زمانی معنا دارد که Process و state حافظه‌ای لازم زنده باشند، مگر اینکه بعداً persistence جداگانه طراحی شود. Blueprint فعلی resume بعد از process restart را وعده نمی‌دهد.

## Stage D2 — endpoint pool / relay

پس از core resume:

- چند endpoint محدود و ازپیش‌تعریف‌شده؛
- direct/relay health؛
- hysteresis و cooldown؛
- relay با upstream ثابت؛
- در صورت پژوهش Worker، مسیر experimental جدا از core.

هیچ relay نباید arbitrary destination بپذیرد.

## Stage E — عملکرد

- benchmark تکرارپذیر؛
- profiler؛
- baseline comparison؛
- گزارش صریح pass/fail؛
- raw results و manifest.

هدف performance بدون داده واقعی «قبولی» محسوب نمی‌شود.

## Stage F — عملیات

- systemd؛
- نصب idempotent؛
- config transaction؛
- drain/stop؛
- certificate rotation؛
- rollback؛
- support bundle بدون secret/payload.

## Stage G — پژوهش حامل/رفتار اختیاری

- H3 فقط تحت flag؛
- A/B measurement؛
- resource-bounded policy؛
- مطالعه detectability با دامنه تعریف‌شده.

این مرحله مجوز ادعای «undetectable» همگانی نمی‌دهد.

## Stage H — پایلوت واقعی

- نصب کنار مسیر موجود؛
- Route آزمایشی محدود؛
- health واقعی؛
- چند بازه زمانی؛
- مسیر rollback؛
- ثبت محدودیت‌ها و نتیجه بدون اغراق.

## پس از v0.4.0: multiplex مشترک با مدیریت BCC

در v0.4.0، multiplex مشترک فقط از مسیر خط فرمان عرضه می‌شود (`baft-pair ex-accept --join` و reload زنده؛ KNOWN-LIMITATIONS را ببینید). نسخه‌ی بعدی طرح کامل را با تست‌ها و LAB مخصوص خودش پیاده می‌کند:

- نوع تازه‌ای از تراکنش tunnel که یک tenant را با reload (نه restart) به listener در حال اجرای EX اضافه یا از آن حذف می‌کند، با پشتیبان، بررسی سلامت و rollback جداگانه که به tenantهای دیگر دست نمی‌زند؛
- مدل مالکیتی که در آن یک instance (marker، generation، unit و metrics claim) چند tunnel را حمل می‌کند تا شناسایی، drift، retire و rollback دقیق بمانند؛
- برنامه‌ریزی BCC، ویزارد و drift برای tenantهای listener مشترک، فقط وقتی هر دو طرف این قابلیت را گزارش کنند؛
- چند tenant در یک session و قابلیت wire به نام `multiplex-v1` که در HELLO مذاکره می‌شود؛
- reload زنده برای listenerهای فقط mTLS، یا رد صریح آن؛
- LAB دو هاست: افزودن و حذف tenant هنگام عبور ترافیک بقیه، crash یک tenant و reboot.

</div>
