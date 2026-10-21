<div dir="rtl" align="right" lang="fa">

# برنامه توسعه

> نسخه انگلیسی: [PLAN.en.md](PLAN.en.md)

## Stage A — قرارداد و spike

- [x] ثبت مرجع Blueprint/Master Prompt
- [x] ساخت repository و فایل‌های وضعیت
- [x] pin نسخه Go
- [x] BAFT/1 parser و golden vectors
- [x] schema پیکربندی سخت‌گیرانه
- [x] test PKI
- [x] H2 full-duplex + mTLS + cancellation
- [x] اثبات چهار اتصال مستقل برای چهار Shard
- [x] CI روی Go 1.27.1

## Stage B — secure vertical slice

- [x] HELLO / HELLO_ACK / READY
- [x] Route ثابت allowlisted
- [x] OPEN / DATA / ACK / WINDOW / FIN
- [x] half-close و hash end-to-end
- [x] negative certificate tests
- [x] duplicate key / target injection rejection
- [x] duplicate DATA و invalid ACK/WINDOW tests
- [x] idempotent OPEN
- [x] active peer/certificate revocation
- [x] RESET و error code ثابت
- [x] COR-01 یک GiB دوطرفه
- [x] strict YAML loader و dependency lock
- [x] CLI واقعی IR/EX

## Stage C — منابع، fairness و چند Flow

- [x] allocator سراسری data memory
- [x] pool مستقل receive/replay
- [x] per-flow reservation
- [x] آزادسازی replay با ACK
- [x] backpressure قبل از read اضافی source
- [x] DRR برحسب byte در data path
- [x] bounded control queue
- [x] control priority + data anti-starvation burst cap
- [x] multi-Flow integration test
- [x] تثبیت slow-receiver liveness gate با TWRL و تست deterministic
- [ ] تست طولانی slow receiver / soak
- [x] تست فشار چند Shard با memory budget مشترک و reuse بدون leak
- [x] conservation metrics برای allocator/Flow بدون label حساس و پرکاردینالیتی
- [ ] بستن Stage C بعد از PASS شدن workflow مستقل Stage-C soak بدون OOM/race/deadlock

## Stage D — resume دقیق

- [x] prior-art review اجباری Raft / QUIC / TLS 1.3
- [x] threat model مجزا از Epoch پایه
- [x] invariant رسمی ECRL (I0–I6)
- [x] falsification criteria (F01–F10)
- [ ] property/fault-injection suite برای I0–I6 و F01–F09
- [ ] boot/session/epoch fencing
- [ ] ownership دقیق Carrier
- [ ] snapshot state
- [ ] replay روی Carrier جایگزین
- [ ] tombstone
- [ ] duplicate-free replacement
- [ ] state-machine fuzz
- [ ] COR-04 تا COR-11

## Stage D2 — endpoint pool / relay

- [ ] endpoint model با dial address جدا از identity
- [ ] health / retry / hysteresis / cooldown محدود
- [ ] fixed-upstream relay
- [ ] عدم پذیرش arbitrary destination
- [ ] Worker path فقط experimental و جدا از core

## Stage E — performance

- [ ] benchmark manifest
- [ ] 60 ثانیه × 5 تکرار
- [ ] profiler
- [ ] baseline comparison
- [ ] raw results
- [ ] گزارش صریح pass/fail

## Stage F — operations

- [ ] systemd hardening
- [ ] installer idempotent
- [ ] config transaction + rollback
- [ ] drain/stop semantics
- [x] certificate rotation (A4: docs/en/35-cert-rotation.md)
- [ ] support bundle بدون payload/secret
- [ ] packaging

## Stage G — research

- [ ] H3 تحت flag
- [ ] A/B experiments
- [ ] resource-bounded research policy
- [ ] detectability study با ادعای محدود و قابل‌اندازه‌گیری

## Stage H — پایلوت واقعی

- [ ] نصب کنار مسیر موجود
- [ ] یک Route آزمایشی
- [ ] health واقعی
- [ ] چند بازه آزمایش
- [ ] rollback
- [ ] گزارش محدودیت و نتیجه


## R2 v0.1 — وضعیت اجرا

- [x] تشخیص Debian/Ubuntu در installer
- [x] تشخیص amd64/arm64
- [x] نصب/تطبیق Go 1.27.1 با checksum manifest
- [x] build با `-trimpath -ldflags="-s -w"`
- [x] نصب `/usr/local/bin/baft`
- [x] ساخت system user `baft`
- [x] systemd hardening شامل `NoNewPrivileges=true` و `PrivateTmp=true`
- [x] ابزار `baft-pair`
- [x] descriptor `BAFTPAIR1`
- [x] apply اتمیک pairing state در IR
- [x] Noise Pattern IK با `github.com/flynn/noise v1.1.0`
- [x] enrollment اولیه با `IKpsk0`
- [x] تست Noise روی HTTP/2 در حضور TLS-terminating intermediary
- [x] race test
- [ ] اتصال SecurityInternal به `node.Runtime` production data path
- [ ] حذف اتمیک PSK یک‌بارمصرف پس از pin موفق در runtime واقعی
- [ ] تست نصب کامل روی VM تازه Debian و Ubuntu
- [ ] Stage D Replay Engine — **PAUSED**

### Non-goal فعلی

probabilistic timing/packet morphing برای دورزدن traffic analysis یا فیلترینگ در R2 v0.1 پیاده‌سازی نشده است.


## R3 safe — برنامه

- [x] `internal/recordshape` با bucket padding deterministic
- [x] اعمال shaping بعد از Noise encryption و قبل از wire
- [x] propagation گزینه از EX داخل Pairing descriptor
- [x] `--enable-record-shaping` در installer
- [x] تست bounded wire sizes و round-trip
- [x] Integration Noise/H2 intermediary با shaping روشن
- [ ] production wiring به `node.Runtime`
- [ ] VM install test روی Debian/Ubuntu
- [ ] R4 public distribution — **BLOCKED تا بعد از review**
- [ ] Stage D Replay — **PAUSED**

### خارج از scope

random packet-size morphing و timing jitter برای شکست تحلیل آماری یا سامانه‌های فیلترینگ در این شاخه پیاده‌سازی نمی‌شوند.


## R3.1 — نامزد بررسی v0.2-Pro

Padding احتمالی نرمال/لاپلاس، jitter قابل تنظیم و پاسخ HTML پیش از ورود به Session پیاده شد. حالت صریح Noise با peer pin‌شده اکنون به Runtime متصل است. آزمون‌های محلی و Differential مدل/موتور ECRL پاس شدند؛ Stage D و recovery همچنان متوقف‌اند.

[گزارش سه‌بخشی هوشا و شواهد](reports/HOOSHA-R3.1.md). این بخش وضعیت فعلی شاخه R3.1 است و توضیحات R2/R3 بالا سوابق تاریخی‌اند. نسخه نهایی v0.2-Pro، نصب VM تازه، حفظ سرعت با jitter و اثبات مقاومت فیلترینگ هنوز تأیید نشده‌اند.

## افزونه نقشه راه پس از انتشار v0.3.2 — ارزیابی Arange-tun (2026-10-16)

**وضعیت نسخه:** [v0.3.2 رسماً منتشر و امضایش تأیید شده است](https://github.com/zarkmakerburg/BAFT/releases/tag/v0.3.2). انتشار عمومی با استقرار یا پذیرش میدانی ایران↔خارج یکسان نیست؛ آزمایش نیتیو و Deploy همچنان نیازمند مجوز HQ است. مرجع اجرایی [پرامپت جامع افزوده‌شده به #67](https://github.com/zarkmakerburg/BAFT/issues/67#issuecomment-6223076976) و مرجع محصول [#199](https://github.com/zarkmakerburg/BAFT/issues/199) است؛ این بخش جایگزین سوابق تاریخی مراحل بالاتر نیست.

### P0 — ایمنی و شواهد قبل از گسترش
- [ ] **Network Doctor / Link Test هوشمند:** تکمیل ابزارهای موجود #145/#146/#147 برای اندازه‌گیری TCP/UDP reachability، RTT، jitter، loss، goodput، CPU/RAM با زمان، تازگی، دامنه اندازه‌گیری و confidence. گزارش خطای مستند و UNKNOWN برای فقدان شواهد؛ پیشنهاد Transport قابل‌توضیح بدون سوییچ یا Deploy خودکار.
- [ ] **Multi-Address Failover یک سرور:** اتصال‌های جایگزین فقط برای یک peer تأییدشده؛ جداسازی هویت از آدرس، retry/backoff/hysteresis/cooldown محدود، audit و rollback. تست blackhole/flap/timeout، خطای handshake، stale generation و ایزوله‌ماندن تونل‌های دیگر.
- [ ] **ECRL same-flow continuity:** جداگانه ثابت شود (۱) تونل دوباره وصل می‌شود و (۲) **همان TCP Flow قدیمی** بدون فقدان/تکرار بایت، با کنترل ACK/WINDOW/FIN و بودجه replay محدود ادامه می‌یابد. هیچ ادعای resume بعد از process restart/reboot نیست.

### P1 — تجربه اپراتور و کنترل مصرف
- [ ] **راهنمای انتخاب Transport در BCC/Aurora Hybrid:** پشتیبانی واقعی هر دو نود، تفاوت TLS/mTLS/Noise/plain، نیاز TCP/UDP، اثر انگشت قابل مشاهده و حدود شواهد، پیشنهاد بر مبنای Link Test، انتخاب دستی، پیش‌نمایش effective-config، تأیید امن، audit، rollback و FA/EN RTL/LTR Dark/Light/mobile.
- [ ] **سهمیه هر تونل:** سقف اتصال هم‌زمان و پهنای‌باند aggregate به تفکیک instance، سیاست پیش‌فرض بدون تغییر، فشار حافظه کنترل‌شده، fairness و جلوگیری از starvation صف کنترل؛ تست هم‌زمانی، محدودیت/عدم محدودیت و rollback تنظیمات.
- [ ] **Quiet/Balanced/Boost:** تکمیل مورد موجود #199 بر پایه اندازه‌گیری قبل/بعد، سقف منابع، fallback هنگام پسرفت؛ بدون کاهش امنیت یا تغییر Transport پنهانی.

### P2 — آزمایش‌های مجزا، غیرمسدودکننده انتشار
- [ ] بررسی KCP/QUIC/UDP و WireGuard/AmneziaWG/SOCKS5 تنها به عنوان Adapterهای اختیاری مستقل؛ QUIC/KCP بدون UDP باز مفید نیستند؛ هیچ‌کدام جایگزین ECRL خودکار نمی‌شوند.
- [ ] بررسی Packet/ICMP/Raw Socket/ Spoof فقط در آزمایشگاه مجزا همراه با threat model، نیاز privilege، سازگاری و ارزیابی ریسک؛ بدون ادعای «غیرقابل شناسایی» یا فعال‌سازی عملیاتی.
- [ ] Stage E benchmark: مقایسه BAFT، direct TCP و گزینه‌های هم‌رده با مسیر یکسان، 60s × 5، RTT/jitter/loss/goodput، بقای همان Flow، CPU/RAM، pprof و داده خام؛ پیوند به R-003.

**تفکیک الزام‌آور:** Multi-Address (چند مسیر به **همان peer**) با Multi-Exit (چند نود خارجی با هویت و Route مستقل) یکسان نیست. Merge/Release/Deploy/تغییر سرور زنده و فعال‌سازی Carrier آزمایشی نیازمند مجوز فعلی HQ است. اجرای هر آیتم در شاخه/PR جدا با تست exact SHA و گزارش شواهد در #67 انجام شود.

</div>
