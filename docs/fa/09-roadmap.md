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

## افزونه پذیرفته‌شده پس از v0.3.2 (2026-10-16)

نسخه [v0.3.2](https://github.com/zarkmakerburg/BAFT/releases/tag/v0.3.2) به‌صورت عمومی منتشر و بررسی امضا شده است؛ **آزمون میدانی IR↔EX و استقرار عملیاتی جداگانه و به تصمیم HQ موکول است.** جزییات [PLAN.md](../../PLAN.md)، [#199](https://github.com/zarkmakerburg/BAFT/issues/199) و [پرامپت جامع اصلاحی #67](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6223076976) معتبرند.

- **P0:** تکمیل Link Test/Network Doctor با تازه‌بودن و عدم‌قطعیت شواهد، Failover چندآدرسی **همان peer** و آزمون جداگانه حفظ **همان TCP Flow** با ECRL در برابر صرفاً اتصال مجدد.
- **P1:** راهنمای Transport در BCC/Aurora Hybrid با انتخاب دستی و مشخصات رمزنگاری واقعی؛ سقف پهنای باند و اتصال هم‌زمان به تفکیک تونل؛ پروفایل‌های Quiet/Balanced/Boost مبتنی بر اندازه‌گیری.
- **P2:** تحقیق ایزوله KCP/QUIC/UDP/WireGuard/SOCKS5 و Packet/ICMP/Raw Socket با threat model و بدون فعال‌سازی عملیاتی؛ Benchmark تکرارپذیر Stage E و R-003.

**قرارداد:** Multi-Address ≠ Multi-Exit. موارد جدید نباید امنیت، تخصیص محدود حافظه، صحت replay و هویت/ایزولاسیون تونل را نقض کنند. برای هر مرحله شاخه/PR مستقل و Gate دقیق لازم است؛ بدون مجوز HQ هیچ Merge/Release/Deploy صورت نگیرد.

</div>
