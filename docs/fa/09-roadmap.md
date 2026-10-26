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


## پس از v0.4.0: موتورهای تونل دیگر (مستقیم و معکوس)

امروز BCC فقط یک موتور را مستقر می‌کند، یعنی بافت بومی، و فقط در جهت **مستقیم** (IR به EX وصل می‌شود). کاتالوگ موتورها (`internal/tunnelengine/catalog.go`) موتورهای جدول زیر را «در دسترس نیست» نشان می‌دهد و در plan و deploy رد می‌کند. هر tunnel در BCC با jobهای امضاشده‌ی agent ساخته می‌شود: `tunnel_prepare_ex`/`tunnel_prepare_ir`، `tunnel_commit_ex`/`tunnel_commit_ir`، `tunnel_health`، `tunnel_finalize`، `tunnel_rollback`، `tunnel_retire` و `tunnel_discover`. هر job یک تراکنش `tunnelnode` اجرا می‌کند که config، unit و marker مالکیت یک instance را می‌نویسد و می‌تواند آن را برگرداند. باینری‌ها فقط از release امضاشده می‌آیند (`update_baft` فقط `baft` و `baft-pair` را عوض می‌کند).

اصطلاحات. **مستقیم**: سرور ایران اتصال را به سرور خارج باز می‌کند؛ کاربران به ایران وصل می‌شوند و ترافیک از خارج خارج می‌شود. **معکوس**: سرور خارج اتصال را به سرور ایران باز می‌کند و ایران برای کاربران گوش می‌دهد؛ برای وقتی که ایران نمی‌تواند به بیرون وصل شود ولی خارج به ایران می‌رسد.

| موتور | مجوز upstream (بررسی‌شده ۲۰۲۶-۱۰-۲۶) | مستقیم (ایران به خارج) | معکوس (خارج به ایران) | ترنسپورت اتصال | برنامه |
|---|---|---|---|---|---|
| FRP (`fatedier/frp`) | Apache-2.0 | frps روی خارج، frpc روی خارج برای مقصد، و frpc از نوع stcp/xtcp visitor روی ایران | frps روی ایران، frpc روی خارج مقصد خارج را منتشر می‌کند | TCP، KCP، QUIC، WebSocket/WSS | درایور P0 |
| Rathole (`rathole-org/rathole`) | Apache-2.0 | طبیعی نیست (سرور پورت عمومی را روی خود میزبان باز می‌کند) | سرور روی ایران پورت کاربران را باز می‌کند و کلاینت روی خارج به مقصد می‌فرستد | TCP، TLS، Noise (پیش‌فرض `Noise_NK_25519_ChaChaPoly_BLAKE2s`)، WebSocket | درایور P0، فقط معکوس |
| Backhaul (`Musixal/Backhaul`) | AGPL-3.0 | مستند نشده | سرور روی ایران (در مثال‌هایش «Local, IRAN»)، کلاینت روی خارج | tcp، tcpmux، udp، udp روی tcp، ws، wsmux، wss، wssmux | درایور P1، فقط معکوس |
| WireGuard (کرنل، `wireguard-tools`، `wireguard-go`) | ماژول کرنل در Linux 5.6 به بعد؛ `wireguard-go` با MIT؛ مجوز tools تأیید نشد (صفحه در دسترس نبود) | هر طرف می‌تواند شروع کند؛ ایران پورت‌های کاربران را با nft DNAT وارد tunnel می‌کند | همان، با خارج به‌عنوان آغازگر | فقط UDP (لایه ۳) | درایور P1، با rollback مسیر و فایروال |
| SSH | OpenSSH (بسته‌ی سیستم) | `ssh -L` از ایران، یا SOCKS پویا (مدخل کاتالوگ `ssh-socks`) | `ssh -R` از خارج؛ برای bind عمومی، sshd ایران به `GatewayPorts` نیاز دارد | TCP روی SSH | P1، با کلید محدود، هرگز حساب اپراتور |
| GOST v3 (`go-gost/gost`) | MIT | listener فوروارد روی ایران با relay/TLS/WS به خارج | handler از نوع `rtcp`/tunnel (معکوس از v3.0.0-rc) | TCP، TLS، WS، WSS، QUIC، KCP و غیره | نامزد، هنوز در کاتالوگ نیست |
| Chisel (`jpillora/chisel`) | MIT | کلاینت روی ایران یک پورت محلی به سرور روی خارج باز می‌کند | سرور روی ایران با remoteهای `R:`، کلاینت روی خارج | HTTP/WebSocket، رمزشده با SSH | نامزد، هنوز در کاتالوگ نیست |
| WSS و QUIC | (حامل‌های بافت) | حامل‌های موتور بومی‌اند، نه موتور جداگانه | | | کار حامل P1 |

هر موتور پیش از فعال شدن مدخلش در کاتالوگ به این‌ها نیاز دارد (همان gateهای `docs/en/38-smart-tunnel-ports-and-drivers.md` و `docs/en/41-engine-inventory-evidence-contract.md`):

1. **منشأ باینری.** نسخه‌ی upstream ثابت و SHA-256 برای هر معماری، که دوباره ساخته یا تأیید شود و داخل release امضاشده‌ی بافت همراه فایل مجوز و پیشنهاد کد منبع عرضه شود. agent هرگز موتور را مستقیم از upstream دانلود نمی‌کند.
2. **نصب و موجودی.** یک job امضاشده که باینری موتور را نصب یا تأیید می‌کند، به‌علاوه‌ی گزارش فقط‌خواندنی موجودی (سند ۴۱)، تا BCC موتور را فقط روی دو نودی بپذیرد که هر دو نسخه و digest مورد انتظار را گزارش کنند.
3. **قرارداد درایور.** برای هر موتور یک درایور که config را از plan بازبینی‌شده می‌سازد (بدون فیلد آزاد)، با نقش‌های وابسته به جهت (کدام طرف گوش می‌دهد و کدام وصل می‌شود)، unit سیستم‌دی برای هر instance، رمزهایی که روی خود نود ساخته و هرگز نمایش داده نمی‌شوند، و رزرو پورت از راه بررسی‌های تعارض موجود.
4. **تراکنش‌ها.** همان چرخه‌ی prepare، commit، health، finalize، rollback، retire و discover بافت بومی، با marker مالکیت دقیق، پایداری پس از restart و حذف امن.
5. **BCC و ویزارد.** ویزارد جهت (مستقیم یا معکوس)، موتور و ترنسپورت را فقط وقتی نشان می‌دهد که مدخل کاتالوگ فعال باشد و هر دو نود موتور را گزارش کنند؛ plan hash، drift، metrics و health برای هر موتور.
6. **شواهد.** تست واحد و تست خطا، سپس LAB دو نودی برای هر موتور و هر جهت (ترافیک، restart، reboot، rollback، حذف) پیش از فعال کردن گزینه.

منابع: https://github.com/fatedier/frp، https://github.com/rathole-org/rathole (و `docs/transport.md` آن)، https://github.com/Musixal/Backhaul، https://github.com/go-gost/gost، https://gost.run/en/blog/2023/reverse-proxy/، https://github.com/jpillora/chisel، https://github.com/WireGuard/wireguard-go، https://wiki.gentoo.org/wiki/WireGuard.

</div>
