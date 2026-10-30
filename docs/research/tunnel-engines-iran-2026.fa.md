<div dir="rtl" align="right" lang="fa">

# در بحران، لیست سفید برنده است، نه رمزنگاری

**پاسخ کوتاه:** در هر سه کشور ایران، روسیه و چین، آنچه در بحران تونل IR↔EX را می‌کشد **لیست سفید IP/CIDR، SNI و پروتکل** است، نه کیفیت رمزنگاری. پس جعبه‌ابزار BCC باید حول این محورها چیده شود: یک carrier تی‌سی‌پی روی ۴۴۳ با SNI مجاز (VLESS REALITY یا XHTTP پشت وب‌سرور واقعی)، مسیرهای CDN، و حالت «IR به‌عنوان رله‌ی داخلی». موتورهای reverse مثل FRP، GOST v3، Backhaul و Rathole برای شرایط عادی می‌مانند، و ابزارهای UDP (AmneziaWG و Hysteria2) فقط «ابزار روزهای عادی» حساب می‌شوند. در شرایط عادی، **Xray (REALITY/XHTTP، به‌علاوه‌ی bridge/portal)** و **FRP/GOST v3** در صدر قرار می‌گیرند. در اختناق شدید و لیست سفید، **TLS با SNI مجاز، CDN، چرخش اتصال‌های کوتاه و زنجیره از طریق رله‌ی داخلی** در صدرند. در قطع تقریباً کامل، هیچ تونل IP بین‌المللی قابل‌اتکایی وجود ندارد. آنجا فقط **تونل DNS (dnstt/slipstream)** با موفقیت متغیر کار می‌کند و IR در عمل نقش سرویس‌دهنده‌ی داخلی روی شبکه‌ی ملی را دارد. برای موتور بومی BAFT هم سه پیام اصلی هست. Noise داخل یک لایه‌ی TLS بیرونی درست است، به این شرط که **TLS-in-TLS نسازد**. شکل ترافیک باید با شکافتن uplink/downlink و چرخش اتصال پنهان شود. و SNI و مقصد باید قابل‌پیکربندی روی مقادیر مجاز باشند. ادعای «قطع حدود ۸ کیلوبایتی TCP خارجی در دیتاسنترهای ایران» **تأیید نشد**. نزدیک‌ترین داده‌ی سنجیده‌شده دو مورد است: قطع پس از حدود ۶ بسته برای اتصال‌های «خاکستری» در ایران ۲۰۲۶، و فریز حدود ۱۵–۲۰ کیلوبایت (حدود ۲۵ بسته) در روسیه.

> **یادداشت روش‌شناسی و محدودیت منابع:** در جلسه‌های پژوهش، **GitHub API مسدود بود (HTTP 403)**. به همین دلیل مجوزها، تاریخ انتشارها و آخرین commitها از صفحه‌های وب GitHub یا از aggregatorها گرفته شده‌اند و سال انتشار اغلب نمایش داده نمی‌شد. **arxiv.org، alphaxiv.org، filter.watch، factnameh، corpus.lantern.io (در بخشی از جلسه‌ها)، amnezia.org، cvefeed.io، osv.dev و NVD** هم در DNS یا fetch در دسترس نبودند. یافته‌های مربوط به آن‌ها بر خلاصه‌ی موتور جستجو تکیه دارد و باید از منبع اصلی دوباره خوانده شود. هرجا عبارت «ادعا» یا «نیازمند بازبینی» آمده، آن مورد از منبع اولیه تأیید نشده است.

## سه رژیم فیلترینگ، یک الگو: لیست سفید، شمارنده‌ی هر اتصال، و حذف UDP

**ایران** در دوره‌های قطع از فیلترینگ مبتنی بر لیست سیاه به لیست سفید پروتکل، SNI و IP رسید. در «خاموشی پنهان» ژوئن ۲۰۲۵، اعلان‌های BGP برقرار ماندند اما **فقط DNS (UDP/53)، HTTP (80) و HTTPS (443)** از مرز عبور کردند. OpenVPN و SSH بی‌صدا حذف شدند ([Aryapour، arXiv 2507.14183، از طریق خلاصه‌ی جستجو](https://arxiv.org/pdf/2507.14183); [Lantern](https://corpus.lantern.io/papers/2025-aryapour-stealth-blackout/)). گزارش‌های میدانی همان دوره سه نکته‌ی مهم دارند. دیتاسنترها «فقط دسترسی ایران» شدند و ساخت تونل v2ray ناممکن شد. در یک ISP، ClientHello عبور می‌کرد اما ServerHello مسدود می‌شد. IPv6 و QUIC هم روی همه‌ی ISPها خاموش شدند ([net4people #484](https://github.com/net4people/bbs/issues/484)). در همان ژوئن تقریباً **همه‌ی پروتکل‌های UDP جز UDP/53** مسدود بودند، از جمله WireGuard، AmneziaWG، QUIC و OpenVPN ([Nym](https://www.nym.com/blog/nym-iran-report-2025)).

قطع‌های ۲۰۲۶ از این هم سخت‌تر بودند. قطع ژانویه/فوریه حدود **۹۸٪ افت از طریق برداشتن مسیرهای BGP** بود و در آن هیچ تونل مبتنی بر دسترسی IP کار نمی‌کرد ([Lantern: gusgustavo](https://corpus.lantern.io/findings/2026-gusgustavo-iran-internet-shutdown__iran-bgp-route-withdrawal-ip-collapse/)). منابع درباره‌ی مدت آن متناقض‌اند: ویکی‌پدیا ۸ ژانویه تا ۲۶ مه را ذکر می‌کند و Bloomberg حدود ۱۰ روز قطع تقریباً کامل را ([Wikipedia](https://en.wikipedia.org/wiki/2026_Internet_blackout_in_Iran); [Bloomberg](https://www.bloomberg.com/features/2026-iran-internet/)). در فازهای آوریل تا ژوئن ۲۰۲۶ اتفاقات زیر گزارش شد ([net4people #626](https://github.com/net4people/bbs/issues/626)):

- برخی IPهای Cloudflare دست‌دهی TCP را کامل می‌کردند ولی داده‌ای عبور نمی‌دادند.
- سپس لیست سفید SNI آمد و اتصال‌های غیرمجاز **پس از حدود ۶ بسته قطع** شدند.
- سقف throughput حدود **۱۰۰ KB/s** بود و اتصال‌ها اغلب حدود یک دقیقه‌ای قطع می‌شدند.
- ترفند ClientHello جعلی با TTL پایین حدود ۴ روز کار کرد. بعد NAT اعتبارسنج checksum و sequence (حوالی ۱۵ آوریل) آن را از کار انداخت.
- **میزبان‌های خارجی نمی‌توانستند اتصال به داخل ایران آغاز کنند.**

نکته‌ی آخر برای BCC تعیین‌کننده است، چون حالت reverse (EX dials IR) را در بحران بی‌اثر می‌کند. Amnezia ادعا می‌کند رویکرد لیست سفید از آغاز جنگ با آمریکا به‌طور دائم و سراسری اعمال شده است. این ادعای فروشنده است و صفحه‌ی اصلی آن fetch نشد ([Amnezia](https://amnezia.org/en/blog/internet-censorship-in-iran-q1-2026-and-forecast)).

**روسیه** نزدیک‌ترین الگوی مستند برای معماری BAFT است. از میانه‌ی ۲۰۲۵ اپراتورهای موبایل اتصال را وقتی سه شرط با هم برقرار باشد بی‌صدا فریز می‌کنند: اتصال TLS روی TCP است، مقصد ASN میزبانی خارجی است، و سرور بیش از حدود **۱۵–۲۰ KB (حدود ۲۵ بسته)** فرستاده است. هیچ RST ارسال نمی‌شود، و یک تلاش UDP سرور را حدود ۱۰ دقیقه مسدود می‌کند. شکستن در سطح TCP/IP یا چند درخواست داخل یک اتصال کمکی نمی‌کند، چون **شمارش برای هر اتصال جداگانه** است. SSH/SFTP بنا بر گزارش‌ها معاف بوده است ([net4people #490](https://github.com/net4people/bbs/issues/490)). Cloudflare هم تأیید کرده که از ۹ ژوئن ۲۰۲۵ سایت‌های پشت Cloudflare فقط **۱۶ KB نخست** هر asset را می‌گیرند ([Cloudflare](https://blog.cloudflare.com/ru-ru/russian-internet-users-are-unable-to-access-the-open-internet)). الگوی کاری گزارش‌شده یک **آبشار است: رله‌ی داخلی با IP مجاز به سرور خارجی**. این همان معماری IR→EX است ([Qubes forum](https://forum.qubes-os.org/t/installation-of-amnezia-vpn-and-amnezia-wg-effective-tools-against-internet-blocks-via-dpi-for-china-russia-belarus-turkmenistan-iran-vpn-with-vless-xray-reality-best-obfuscation-for-wireguard-easy-self-hosted-vpn-bypass/39005); [hxehex](https://github.com/hxehex/russia-mobile-internet-whitelist)). روسیه فیلتر QUIC بر اساس SNI را بین ۲۰۲۲ و ۲۰۲۳ در مقیاس گسترده اجرا کرد ([FOCI 2026](https://www.petsymposium.org/foci/2026/foci-2026-0010.php)).

**چین** همچنان از سال ۲۰۲۱ ترافیک «کاملاً رمزشده» با ظاهر تصادفی را با heuristic آنتروپی و ASCII مسدود می‌کند ([USENIX'23](https://www.usenix.org/conference/usenixsecurity23/presentation/wu-mingshi)). از ۷ آوریل ۲۰۲۴ هم Initialهای QUIC را رمزگشایی می‌کند و بر اساس SNI به مدت ۱۸۰ ثانیه drop می‌کند ([USENIX'25](https://www.usenix.org/conference/usenixsecurity25/presentation/zohaib)). در ۲۰ اوت ۲۰۲۵ به مدت ۷۴ دقیقه **همه‌ی TCP/443** را بی‌قیدوشرط RST کرد ([gfw.report](https://gfw.report/blog/gfw_unconditional_rst_20250820/en/)). نشت Geedge در سپتامبر ۲۰۲۵ هم از طبقه‌بندی ML روی فراداده‌ی جریان‌های رمزشده خبر می‌دهد ([HRIC](https://news.hrichina.org/p/largest-ever-data-leak-exposes-chinas)). مهم‌ترین نتیجه‌ی پژوهشی برای موتور BAFT مقاله‌ی USENIX'24 است. آشکارساز مستقل از پروتکلِ آن **دست‌دهی TLS کپسوله‌شده (TLS-in-TLS)** را با آسیب جانبی کم کشف می‌کند، حتی با padding تصادفی و چند لایه کپسوله‌سازی. نویسندگان هشدار می‌دهند که مالتی‌پلکس به‌همراه padding به‌تنهایی کافی نیست ([USENIX'24](https://www.usenix.org/conference/usenixsecurity24/presentation/xue)).

**درباره‌ی فرض «~۸ KB»:** هیچ منبعی پیدا نشد که قطع TCP خارجی از دیتاسنترهای ایران پس از حدود ۸ (یا ۸ تا ۱۶) کیلوبایت را تأیید کند. داده‌های موجود «حدود ۶ بسته» برای اتصال‌های خاکستری ایران (گزارش میدانی) و «۱۵–۲۰ KB» برای روسیه است. این فرض باید با سنجش مستقیم از چند دیتاسنتر ایرانی بازبینی شود و BCC آن را به‌صورت probe پیاده کند، نه به‌صورت ثابت کدنویسی‌شده. همچنین هیچ منبعی پیدا نشد که مشخص کند کدام دیتاسنترهای ایرانی پروتکل ۴۱ (6to4/6in4) یا ۴۷ (GRE) را فیلتر می‌کنند.

## کاتالوگ موتورها: موتورهای reverse برای روزهای عادی، Xray برای روزهای سخت

جهت در همه‌ی موتورهای port-forward فقط به این بستگی دارد که نقش «server» (listener) روی کدام میزبان باشد. **FRP و Rathole ذاتاً reverse هستند**: سرور روی IR است و EX آن را dial می‌کند. Chisel، GOST و SSH هر دو جهت را دارند ([rathole](https://github.com/rathole-org/rathole); [chisel](https://github.com/jpillora/chisel); [Backhaul](https://github.com/Musixal/Backhaul)). با توجه به گزارش «میزبان خارجی نمی‌تواند به داخل ایران اتصال آغاز کند» ([#626](https://github.com/net4people/bbs/issues/626))، حالت reverse در بحران شکننده‌تر از direct است. این برخلاف عادت رایج اسکریپت‌های ایرانی است که تقریباً همه‌چیز را «ریورس تانل» می‌نامند ([Azumi67/Backhaul_script](https://github.com/Azumi67/Backhaul_script)).

### جدول مقایسه‌ی موتورهای TCP/port-forward

| Engine | License | Maintenance (Oct 2026) | Direct (IR→EX) | Reverse (EX→IR) | TCP/UDP | Mux | Transports | Notes / evidence status |
|---|---|---|---|---|---|---|---|---|
| FRP | Apache-2.0 (not re-verified) | Very active, ~monthly; v0.71.1 (14 Oct 2026) | No (reverse design) | Yes | Both | tcpMux, pool | tcp/kcp/quic/ws/wss (list not primary-sourced) | Wire protocol v2 with AEAD from v0.69; v1 removed by v0.78; several 2026 security fixes ([releases](https://github.com/fatedier/frp/releases)) |
| GOST v3 | MIT | Active; v3.3.1 (21 Oct 2026) | Yes | Yes | Both | Yes | ws, h2, grpc, quic, kcp, dtls, ssh, icmp, tun/tap | Web API + Prometheus; v3.3.0 fixed a destination-policy bypass ([gost](https://github.com/go-gost/gost/releases)) |
| Backhaul | **AGPL-3.0** | Not archived; 17 open issues; release dates not visible | Yes, if server on EX | Yes (usual IR deployment) | Both (accept_udp) | tcpmux/wsmux/wssmux | tcp, udp, ws, wss | No documented checksums; root unit without hardening ([Backhaul](https://github.com/Musixal/Backhaul)) |
| Rathole | Apache-2.0 | Uncertain; last tag v0.5.0 (Oct 2023, third-party claim) | No | Yes | Both | — | TCP, TLS, Noise, WS | ~500 KiB binary (README claim); hot reload ([rathole](https://github.com/rathole-org/rathole); [dev.to](https://dev.to/instatunnel/rathole-vs-frp-vs-ngrok-what-the-benchmarks-actually-show-244o)) |
| Chisel | MIT | Quiet; changelog to v1.12 | Yes | Yes (`--reverse`, `R:`) | Both (UDP via SOCKS) | SSH channels | SSH over HTTP/WS | CVE-2024-43798 fixed in 1.10.0 ([NVD](https://nvd.nist.gov/vuln/detail/CVE-2024-43798)); CVE-2026-48113 unverified ([corgea](https://corgea.com/advisories/packages/go/github.com/jpillora/chisel)) |
| OpenSSH (+autossh) | BSD | Mature | `-L`/`-D` | `-R` | TCP | SSH channels | SSH | Exempt from the Russian freeze (field report), but easy to fingerprint and dropped in Iran's June 2025 shutdown ([#490](https://github.com/net4people/bbs/issues/490); [#484](https://github.com/net4people/bbs/issues/484)) |

هیچ داده‌ی کارایی سنجیده‌شده‌ای برای این موتورها روی مسیر IR–EX پیدا نشد. ستون کارایی عمداً حذف شده تا عدد ساختگی در جدول نیاید. استنتاج مهم این است: فریزی که برای هر اتصال جداگانه می‌شمارد، mux طولانی‌مدت روی یک اتصال (tcpmux، wsmux، yamux و tcpMux در FRP) را از کار می‌اندازد. در مقابل، poolهای کوتاه‌عمر و WSS با SNI مجاز پشت CDN با افت نرم‌تری خراب می‌شوند. چند نکته‌ی مجوز و نسخه هم مهم است. AGPL در Backhaul یعنی اگر BAFT نسخه‌ی تغییریافته‌ای را به‌عنوان سرویس شبکه عرضه کند، باید سورس را منتشر کند. منسوخ‌شدن پروتکل v1 در FRP یعنی BCC باید نسخه‌ی هر دو طرف را قفل و همگام کند.

### جدول Xray، sing-box و CDN

| Engine / transport | License | Maintenance | Direction | Mux | Detectability (evidence) | Ops complexity |
|---|---|---|---|---|---|---|
| Xray VLESS + Vision + REALITY | MPL-2.0 (not re-verified) | Very active; v26.7.11 stable confirmed, v26.8.x only via fork mirror ([releasealert](https://releasealert.dev/github/XTLS/Xray-core)) | IR→EX native | mux.cool / XUDP | Best normal-times resilience; SNI degrades per IP after ~2 weeks ([#253](https://github.com/net4people/bbs/issues/253)); Vision does not hide traffic shape ([note](https://publish.obsidian.md/zapret/Privacy/xray/xtls-vision)) | High: x25519, dest/serverNames, shortIds |
| Xray XHTTP | MPL-2.0 | Same | IR→EX, can split up/down | XMUX | Allows TLS 1.2 behind real nginx; not compatible with Vision ([Habr 990208](https://habr.com/en/articles/990208)) | High, schema changes often |
| Xray reverse (bridge/portal) | MPL-2.0 | Docs still say "testing phase" | Bridge dials portal (either side) | Mandatory mux | Same as underlying transport ([Habr 774838](https://habr.com/en/articles/774838); [internals](https://xray-internals.hidandelion.com/advanced/reverse.html)) | High |
| sing-box | GPL-3.0+ with naming clause (not re-verified) | v1.13.19 / 1.14.0-beta.17 (third-party mirror) | Both | smux/yamux/h2mux, padding | Depends on transport | Medium–high; GPL means a separate binary |
| CDN-fronted WS/gRPC/XHTTP | — | — | IR→CDN→EX | Per transport | Whitelisted IP space; Cloudflare capped at 16 KB in Russia ([Cloudflare](https://blog.cloudflare.com/ru-ru/russian-internet-users-are-unable-to-access-the-open-internet)); some CF IPs passed no data in Iran 2026 ([#626](https://github.com/net4people/bbs/issues/626)) | Medium; CDN account risk |
| Psiphon tunnel-core | GPL-3.0 | Binaries published | Client-oriented | — | Multi-protocol; not a natural fit for S2S ([pkg.go.dev](https://pkg.go.dev/github.com/psiphon-labs/psiphon-tunnel-core@v2.0.9+incompatible)) | High; value is in ideas, not integration |

REALITY در شرایط عادی ایران کار می‌کند، اما با آمدن لیست سفید IP یا CIDR کارایی‌اش را از دست می‌دهد ([#618](https://github.com/net4people/bbs/issues/618)). یک گزارش هم آن را روی همراه اول مسدود دانسته است ([Xray-core #2778](https://github.com/XTLS/Xray-core/issues/2778)). ادعای RaccoonLine دو بخش دارد: WireGuard و OpenVPN «در عرض چند ساعت» کشف می‌شوند و VLESS+REALITY مقاوم‌ترین گزینه است. این ادعای نزدیک به فروشنده است ([Security Ledger](https://securityledger.com/2026/05/raccoonline-report-vless-and-p2p-architecture-defeat-irans-2026-machine-learning-dpi/)). پرچم‌های مهم supply chain: mirrorها و forkهای بسته‌بندی‌شده‌ی Xray و sing-box زیاد دست‌به‌دست می‌شوند، از جمله Jolymmiles و enpioodada. بسته‌های سیستم‌عامل هم عقب‌اند؛ FreeBSD هنوز 25.12.8 را دارد ([FreshPorts](https://freshports.org/security/xray-core)). مرجع باید فقط releaseهای رسمی XTLS و SagerNet با checksum باشد.

## UDP و لایه‌ی ۳ سریع‌اند، ولی اولین قربانی بحران‌اند

| Engine | License | Maintenance | TCP/UDP | Detectability (evidence) | Ops |
|---|---|---|---|---|---|
| WireGuard | GPLv2 kernel / MIT go | Mature | UDP | Handshake detected then IP blackholed in Iran 2026 (community, via [Lantern](https://corpus.lantern.io/findings/2026-pooribitwise-iran-advanced-dpi__iran-wireguard-blackholed-handshake/)) | Low; kernel ≥5.6 |
| AmneziaWG 2.0 | GPLv2 / MIT (doc-knowledge) | 2.0 released mid-2025 ([TechRadar](https://www.techradar.com/vpn/vpn-services/amnezia-vpn-drops-new-amneziawg-2-0-protocol-as-censorship-tactics-grow-smarter)) | UDP | Mimics DNS/QUIC/SIP; tested mainly vs Russian DPI ([Habr](https://habr.com/ru/amp/publications/822785)); fails under whitelists; "targeted attack" July 2026 ([Amnezia](https://amnezia.org/blog/amnezia-shutdown-digest-july-2026-russia)) | Medium; Jc/Jmin/Jmax/S1/S2/H1–H4 must match (2.0 names unverified) |
| OpenVPN (+XOR) | GPLv2 | Mature; XOR is an out-of-tree patch | Both | Widely detected (secondary) | Medium |
| Hysteria2 | MIT | Active; v2.12.0 adds "mimic" (TCP disguise) ([newreleases](https://newreleases.io/project/github/apernet/hysteria/release/app%2Fv2.12.0)) | UDP (QUIC) | Brutal CC is a fingerprint; BBR matches it below 20% loss ([Lantern](https://corpus.lantern.io/findings/2025-wang-custom__custom-cca-fixed-rate-fingerprint/)) | Medium; manages iptables/nftables for port ranges |
| TUIC v5 | — | Original repo abandoned (2023); lives on in forks and sing-box (community) | UDP | Same QUIC risks | Fork-dependent |
| kcptun | MIT | Not verified | UDP | Detectable UDP | Low |
| udp2raw | MIT (not re-verified) | Last upstream 20230206.0 ([Arch](https://www.archlinux.org/packages/extra/x86_64/udp2raw)) | FakeTCP/UDP/ICMP | Converts UDP to FakeTCP | Needs root + iptables |
| Paqet | Not verified | **Alpha**; protocol may change ([OnWorks](https://www.onworks.net/software/linux/app-paqet)) | KCP over raw TCP with custom flags | New; no evidence | CAP_NET_RAW, RST suppression; managers: paqctl, Paqet-Tunnel-Manager ([samnet](https://www.samnet.dev/learn/guides/paqctl-setup/)) |
| GRE/IPIP/SIT/VXLAN | Kernel | — | L3 | No encryption; proto 41/47 dropped by firewalls that pass only 1/6/17 ([LWN](https://lwn.net/Articles/690455/)) | Low, but needs public IPs on both ends, no NAT, and MSS clamping |

تنها داده‌ی کارایی مستقل این است که BBR تا ۲۰٪ loss تقریباً به پای Hysteria/Brutal می‌رسد و Brutal فقط بالای این آستانه برتری معنادار دارد. نویسندگان همان مطالعه این برتری را به هزینه‌ی fingerprint نمی‌ارزند ([Lantern](https://corpus.lantern.io/findings/2025-wang-custom__bbr-viable-alternative-below-20pct-loss/)). BCC باید در Hysteria پهنای باند واقعی یا BBR را پیش‌فرض بگذارد، چون پهنای باند اغراق‌آمیز فقط fingerprint را تقویت می‌کند. GRE، IPIP و 6to4 در عمل لایه‌ی **آدرس‌دهی** هستند، نه لایه‌ی امنیت: نه رمزنگاری دارند و نه پنهان‌کاری. BCC باید آن‌ها را فقط به‌عنوان زیرلایه‌ی یک موتور رمزدار و با محاسبه‌ی MTU پشته‌ای ارائه کند (GRE 24، IPIP 20 و VXLAN 50 بایت سربار، و WireGuard با MTU پیش‌فرض 1420). در ضمن 6to4 anycast relay با RFC 7526 منسوخ شده است ([oneuptime](https://oneuptime.com/blog/post/2026-03-20-ipv6-tunneling-overview/markdown)).

## تونل DNS تنها بازمانده است و همیشه هم دوام نمی‌آورد

در قطع ژانویه/فوریه ۲۰۲۶، **dnstt روی UDP/53 «تنها بازمانده»** گزارش شد. اما همان دوره، zone دامنه‌ی .ir داخل کشور از دسترس خارج شد و resolution به یک nameserver در آمستردام هدایت شد. تحلیلی دیگر نتیجه می‌گیرد که ابزارهای DNS-tunneling در آن وضعیت شکست می‌خورند ([Lantern: gusgustavo](https://corpus.lantern.io/papers/2026-gusgustavo-iran-internet-shutdown/); [Lantern: free-the-internet](https://corpus.lantern.io/papers/2026-free-the-internet-iran-internet-shutdown/)). این تناقض حل‌نشده است. در فازهای بعدی هم resolverهای خارجی مسدود شدند، ترافیک به resolver محلی اجبار شد، و DoQ پس از نخستین بسته‌ی بعد از handshake قطع شد ([#626](https://github.com/net4people/bbs/issues/626)).

slipstream یک کانال QUIC چندمسیره روی DNS است. ادعا می‌شود حدود ۶۰٪ سربار هدر کمتری از dnstt دارد و در حالت resolver به حدود **۶۳ KB/s** و در حالت authoritative به حدود **۳٫۹ MB/s** می‌رسد. این ارقام شخص ثالث و تأییدنشده‌اند ([slipstream](https://zff.dev/aronson/slipstream); [undercodetesting](https://undercodetesting.com/slipstream-high-performance-dns-tunneling-with-quic-in-rust-a-deep-dive-into-covert-channels-video/)). ابزارهای جدید ۲۰۲۵–۲۰۲۶ عبارت‌اند از **SlipGate** (استقرار dnstt، NoizDNS، VayDNS و slipstream با احراز گواهی) و **MoaV v1.4.0** ([slipgate](https://pkg.go.dev/github.com/anonvector/slipgate@v1.6.2); [MoaV](https://newreleases.io/project/github/MotherofallVPNs/MoaV/release/v1.4.0)). تونل‌های ICMP (pingtunnel و icmptunnel) و WebRTC در ایران ۲۰۲۶ مسدود گزارش شدند ([#626](https://github.com/net4people/bbs/issues/626)). تونل‌زنی از طریق سرویس‌های تماس تصویری داخلیِ مجاز مثل Bale Meet از ۲۰۲۴ استفاده شده است ([#618](https://github.com/net4people/bbs/issues/618)). Spoof-Tunnel کار کرده، اما به سروری داخل ایران نیاز دارد ([#626](https://github.com/net4people/bbs/issues/626)). وضعیت نگهداری iodine، icmptunnel و pingtunnel در ۲۰۲۵–۲۰۲۶ بررسی نشد.

## رتبه‌بندی برای سه وضعیت ایران

| Rank | Normal filtering | Heavy throttling / whitelisting | Near-total shutdown (domestic only) |
|---|---|---|---|
| 1 | BAFT native (TLS/H2 + Noise, IR→EX) or Xray VLESS REALITY/XHTTP, direct | TCP/443 TLS with **whitelisted SNI** plus connection rotation (Xray XHTTP/REALITY or BAFT native) | **IR as domestic relay/service on the NIN**; international leg parked |
| 2 | FRP (v2 wire, ≥0.71.x) or GOST v3 over WSS/mux | CDN-fronted WS/gRPC/XHTTP (whitelisted IP space) | DNS tunnel: slipstream/dnstt via SlipGate-style manager (variable success) |
| 3 | Backhaul wssmux/tcpmux (mind AGPL) / Rathole TLS/Noise | Cascade via a domestic relay with whitelisted IP (Russian pattern) | Domestic WebRTC/messenger carriers (experimental, not in BCC core) |
| 4 | AmneziaWG 2.0 / Hysteria2 (BBR) for UDP throughput | Fallback ports and plain HTTP transports; Hysteria "mimic" / udp2raw FakeTCP / Paqet (experimental) | — |
| 5 | WireGuard/GRE only as underlay inside an encrypted engine | SSH (reportedly exempt in Russia, but dropped in Iran) | — |
| Avoid | Plain WireGuard/OpenVPN as carriers | Any UDP, reverse mode (EX→IR), long single-connection mux | Anything that needs international IP reachability |

این رتبه‌بندی از ترکیب منابع **استنتاج** شده و با هیچ آزمایش کنترل‌شده‌ای زیر رژیم لیست سفید ایران سنجیده نشده است. اگر برای «۸ KB» یا «۶ بسته» عدد دقیقی وجود داشته باشد، باید با probe میدانی تعیین شود.

## تغییرات مشخص برای BAFT و BCC

**موتور بومی BAFT.** معماری فعلی، یعنی carrier بیرونی TLS/HTTP2 یا WebSocket با Noise در payload و IR dials EX، با ادبیات کشف هم‌خوان است، به این شرط که چند اصلاح انجام شود. اول، **هرگز TLS ترافیک کاربر را دوباره داخل TLS بیرونی حمل نکند** بدون آنکه شکل دست‌دهی داخلی پنهان شود ([USENIX'24](https://www.usenix.org/conference/usenixsecurity24/presentation/xue)). دوم، اولین بایت‌ها باید شبیه پروتکل شناخته‌شده باشند، نه داده‌ی تصادفی ([USENIX'23](https://www.usenix.org/conference/usenixsecurity23/presentation/wu-mingshi)). سوم، fingerprint سمت سرور باید واقعی باشد: یک وب‌سرور واقعی هم‌محل، و پشتیبانی از TLS 1.2 در کنار 1.3، به همان دلیلی که XHTTP به وجود آمد ([Habr 990208](https://habr.com/en/articles/990208)). چهارم، uplink و downlink باید بتوانند روی اتصال‌های جدا بروند. پنجم، **چرخش اتصال با سقف بایت/بسته‌ی قابل‌تنظیم** لازم است، چون شمارنده‌ها برای هر اتصال جداگانه کار می‌کنند ([#490](https://github.com/net4people/bbs/issues/490)). ششم، **SNI و مقصد** باید روی مقادیر مجاز، CDN یا دامنه‌ی داخلی قابل‌تنظیم باشند. هفتم، حالت **دوپرشی (two-hop) از طریق رله‌ی داخلی** لازم است. هشتم، به‌جای پیش‌فرض ۴۴۳، باید فهرست پورت‌های جایگزین وجود داشته باشد ([gfw.report](https://gfw.report/blog/gfw_unconditional_rst_20250820/en/)). نهم، شکل ترافیک باید شکل داده شود: جریان‌های طولانی، متقارن و پرحجم دقیقاً همان چیزی است که طبقه‌بند ML هدف می‌گیرد. دهم، یک fallback غیر-IP لازم است: یک ماژول DNS-tunnel سبک برای کانال کنترل agent، حتی اگر داده را حمل نکند.

**BCC به‌عنوان جعبه‌ابزار.** اسکریپت‌های رایج، مانند Azumi67 و Backhaul_script، این الگو را دارند: کد از `refs/heads/main` اجرا می‌شود، pip بدون پین نصب می‌شود، باینری از «latest» گرفته می‌شود، هیچ checksumی چک نمی‌شود، و هر تونل یک unit با دسترسی root و یک «reset timer» دارد ([Backhaul_script](https://github.com/Azumi67/Backhaul_script); [Backhaul](https://github.com/Musixal/Backhaul)). نصب‌کننده‌ی 3x-ui بهتر است: sha256 را چک می‌کند و secretهای تصادفی با mode 600 و sandbox systemd دارد. ولی وقتی فایل hash وجود ندارد فقط هشدار می‌دهد و امضا هم ندارد ([3x-ui install.sh](https://raw.githubusercontent.com/MHSanaei/3x-ui/main/install.sh)). موج advisoryهای 3x-ui در ۲۰۲۶ مستقیماً به سطح حمله‌ی BCC مربوط است. این موج شامل admin پیش‌فرض، تزریق PostUp در WireGuard/AmneziaWG، node-sync بدون امضا و دور زدن محدودیت مسیر است ([advisories](https://github.com/MHSanaei/3x-ui/security/advisories)). CVE-2025-29331 (9.8) هم به‌روزرسانی بدون بررسی TLS بود که به RCE می‌رسید ([cvefeed، fetch نشد](https://cvefeed.io/vuln/detail/CVE-2025-29331)). پیامدهای این برای BCC:

- **Manifest امضاشده.** ساختار آن `{engine, version, arch, url, sha256}` باشد. agent هر artifact خارج از manifest را رد کند و هیچ مسیر warn-and-continue وجود نداشته باشد. mirror داخلی هم لازم است، چون GitHub از ایران قابل‌اتکا نیست.
- **قفل نسخه.** Chisel ≥1.10.0، FRP ≥0.71.x با wire v2 روی هر دو طرف، GOST ≥3.3.0، و Xray و sing-box فقط از release رسمی.
- **Unit قالبی.** از `baft-tunnel@<id>.service` با `DynamicUser`، `ProtectSystem=strict`، `NoNewPrivileges` و `SystemCallFilter=@system-service` استفاده شود، و `Restart=on-failure` جای reset timer را بگیرد.
- **رندر کانفیگ از schema نوع‌دار.** newline و کاراکترهای کنترلی رد شوند (درس PostUp). مقادیری که agent یا node گزارش می‌دهد غیرقابل‌اعتماد حساب شوند. اعتبارسنجی با خود موتور انجام شود، نوشتن اتمیک باشد، و اگر probe سلامت ظرف N ثانیه شکست خورد، به آخرین کانفیگ سالم برگشت خودکار انجام شود.
- **بستن داشبوردها و APIها به 127.0.0.1.** این شامل frps 7500، `web_port` در Backhaul و API در GOST است. imageهای جامعه‌ی FRP با admin/1234 دیده شده‌اند ([نمونه](https://hub.docker.com/r/hieupth/frpserver)). pprof در FRP خاموش بماند (گزارش دور زدن Basic Auth، تأییدنشده: [hackmd](https://hackmd.io/@1ExmmukzRMWN7B4gQ4W-4Q/HyZWPT3jfe)).
- **قوانین iptables برچسب‌دار.** با `-m comment` برچسب بخورند، دفتر نصب (ledger) نگه داشته شود، و uninstall فقط مسیرهای ثبت‌شده را حذف کند. auto-revert timer هم لازم است تا دسترسی SSH از دست نرود.
- **محاسبه‌ی MTU پشته و MSS clamping همیشگی.**
- **UI جهت را صریح نشان دهد.** به‌جای کلمه‌ی «reverse»، بنویسد «سرور ایران dial می‌کند» یا «سرور خارج dial می‌کند»، و هشدار دهد که حالت EX→IR در بحران از کار می‌افتد.
- **Fallback زنجیره‌ای مبتنی بر probe.** نمونه: BAFT native یا XHTTP، سپس CDN، سپس wssmux، سپس پورت جایگزین. probeها باید شامل انتقال بیش از ۶۴ KB، شمارش بسته تا قطع، و تست UDP باشند.

<div dir="ltr" align="left">

```ini
# Example hardened template unit for any engine instance
[Unit]
Description=BAFT tunnel %i
After=network-online.target
StartLimitIntervalSec=300
StartLimitBurst=5

[Service]
DynamicUser=yes
ExecStartPre=/usr/lib/baft/verify-artifact %i
ExecStart=/var/lib/baft/engines/%i/bin/engine -c /etc/baft/tunnels/%i/config
Restart=on-failure
RestartSec=5
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/baft/tunnels/%i
SystemCallFilter=@system-service
AmbientCapabilities=
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
```

</div>

ابزارهای raw-socket مثل Paqet و udp2raw، و ابزارهای L3، به `CAP_NET_RAW` یا `CAP_NET_ADMIN` نیاز دارند. برای این‌ها باید یک unit جداگانه با capability محدود ساخت، نه root کامل.

## ادعاهایی که باید دوباره راستی‌آزمایی شوند

موارد زیر از منبع اولیه تأیید نشده‌اند:

- قطع حدود ۸ KB در دیتاسنترهای ایران.
- مجوز FRP (Apache-2.0)، Xray (MPL-2.0) و sing-box (GPL-3.0+ با بند نام‌گذاری)، و مجوز Paqet.
- تاریخ آخرین release برای Rathole، Chisel، Backhaul، wireguard-go، OpenVPN، forkهای TUIC و kcptun (به‌دلیل مسدود بودن GitHub API).
- انتشار upstream نسخه‌ی Xray v26.8.x.
- نام پارامترهای AmneziaWG 2.0.
- CVE-2026-48113 در Chisel و CVE-2026-79315 و CVE-2026-79316 در x-ui.
- آسیب‌پذیری pprof در FRP.
- اعداد throughput در slipstream.
- ادعای RaccoonLine.
- دوام dnstt در وضعیت هدایت .ir به آمستردام (منابع متناقض‌اند).
- مدت قطع ژانویه ۲۰۲۶ (ویکی‌پدیا و Bloomberg متناقض‌اند).
- ادعای «لیست سفید دائمی» در Amnezia.
- اینکه کدام دیتاسنترهای ایرانی پروتکل‌های ۴۱ و ۴۷ را فیلتر می‌کنند.

خلاصه‌های Aryapour (arXiv) و Filterwatch و مقاله‌ی alphaxiv 2603.28753 هم از طریق خلاصه‌ی جستجو آمده‌اند و باید مستقیم خوانده شوند. منابع جامعه‌ی ایرانی (تلگرام و Azumi67)، Habr و ntc.party، و V2EX و issueهای چینی عمدتاً بازیابی نشدند. هیچ benchmark مستقلی هم برای مقایسه‌ی موتورها روی مسیر IR–EX وجود نداشت.

## نتیجه‌گیری

پرسش اصلی دیگر این نیست که «کدام پروتکل نامرئی‌تر است». پرسش این است که **«تونل با کدام آدرس، کدام SNI و از کدام سمت شروع می‌شود»**. در شرایط عادی، تقریباً هر موتور مدرنی کار می‌کند، و مزیت BCC در امنیت عملیاتی است: manifest امضاشده، unit سخت‌شده و برگشت خودکار. هیچ رقیبی در اکوسیستم این‌ها را ندارد. در بحران، مزیت به طراحی جابه‌جاپذیر منتقل می‌شود: SNI و IP مجاز، چرخش اتصال زیر آستانه‌ی شمارنده، رله‌ی داخلی، و کانال کنترل‌ای که با یک DNS-tunnel زنده می‌ماند. پس BCC باید **شرایط شبکه را اندازه بگیرد و موتور را بر اساس آن انتخاب کند**، و موتور بومی BAFT باید برای همین کار ساخته شود: carrier انعطاف‌پذیر با شکل ترافیک قابل‌تنظیم، نه یک رمزنگاری بهتر.

در قطع کامل، ارزش واقعی IR دیگر «پل به خارج» نیست. ارزش آن **حضور روی شبکه‌ی داخلی** است، یعنی ادامه‌ی سرویس به کاربران داخلی تا وقتی مسیر بین‌المللی برگردد. نقشه‌ی راه BAFT باید این حالت را یک وضعیت رسمی محصول بداند، نه یک شکست.

</div>
