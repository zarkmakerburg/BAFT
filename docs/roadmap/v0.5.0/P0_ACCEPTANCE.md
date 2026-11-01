<div dir="rtl" align="right" lang="fa">

# تطبیق P0 مشاور با شواهد موجود

این سند ردیف‌های P0 گزارش مشاور (۱ نوامبر ۲۰۲۶) را با معیار پذیرش خودشان در برابر شواهد واقعی قرار می‌دهد. سه قاعده رعایت شده است:

- شواهد هر SHA فقط برای همان SHA معتبر است.
- شبیه‌سازی (namespace یا KVM) از اتصال واقعی ایران↔خارج جدا گزارش می‌شود.
- آنچه اجرا نشده «باز» می‌ماند.

| ردیف P0 | معیار پذیرش مشاور | شواهد موجود | فاصلهٔ باقی‌مانده | مرحله |
|---|---|---|---|---|
| Shared Listener برای چند Tunnel | چند Tunnel مستقل روی Listener مشترک، بدون تداخل | `shared operational ingress` (جریان‌های واقعی جداپروسه روی همان IR، `TestBCCSignedSharedSameIRRealFlowsPreserveOwner`) و تست‌های `tests/integration/shared_multiplex_test.go` | مسیر محصول (Wizard → BCC → Deploy) با چند Tunnel روی یک Listener مشترک بین دو میزبان هنوز اثبات نشده است | ۲ |
| Tenant Isolation | تست منفی Cross-Tenant و محدودیت منابع | `TestCrossTenantRouteAccessRefused` (نسخهٔ سخت‌گیرانهٔ #294: رد دقیق EX برای هر route) و `MultipleEXPeersOnOneIRWithoutInterruption` | محدودیت منابع برای هر tenant تست ندارد | ۲ |
| چرخهٔ کامل Join/Retire | حذف یک Tenant بدون قطع دیگران | `SiblingTunnelsSurviveInstallAndRollback`؛ fence برادرِ retired (`c13c4510`)؛ تونل دوم روی همان EX در guided flow (host) | Retire یک tenant روی Listener مشترک در حالی که بقیه ترافیک دارند | ۲ |
| بازیابی بعد از Crash | تست Crash در نقاط مختلف | `operational recovery`: kill -9 وسط Apply و Retire در kernel واقعی (namespace خصوصی)، صفر SKIP؛ #292 (crash بعد از tombstone) و #293 (شکست Receipt) در PR #301 | — (روی SHA ادغام #301 باید دوباره سبز بماند) | ۱ |
| بازیابی پس از Restart Agent | حفظ Listener و وضعیت پس از راه‌اندازی | **باگ P0 پیدا و رفع شد** (پایین را ببینید). گام ۶ `real_hosts.sh`: restart هر دو agent، Apply تازه، Ready و ترافیک | Listener داخل پروسهٔ agent است و حفظ نمی‌شود؛ پس از چند ثانیه **برمی‌گردد**. اجرا روی سرور واقعی باقی است | ۱ |
| تست Firewall Drop | پورت باز ولی مسیر مسدود، بدون Ready کاذب | **جدید:** گام ۷ `real_hosts.sh`. یک قانون drop اپراتور با اولویت بالاتر اضافه می‌شود؛ BCC باید از Ready خارج شود و پس از حذف قانون دوباره Ready شود | اجرا روی سرور واقعی | ۱ |
| تست واقعی 3X-UI | پورت 54619 و Refresh اشتراک | Detector فقط‌خواندنی Xray/3X-UI و رد پورت متعلق به Xray (`guided flow`، host) | سرور تست با 3X-UI نصب‌شده لازم است؛ inbound غیرفعال در `x-ui.db` هنوز تشخیص داده نمی‌شود | ۳ |
| تست Reboot Persistence | بازیابی پس از Restart ماشین | `operational recovery` (بازیابی boot در namespace، reboot واقعی نیست)؛ **جدید:** گام ۹ `real_hosts.sh` با `REBOOT=1`. در self-test یک reboot واقعی ماشین مجازی است | اجرا روی سرور واقعی | ۱ |

## باگ P0: Restart agent پورت عمومی را برای همیشه می‌بست

**چگونه پیدا شد:** self-test اسکریپت سرور واقعی (`real_hosts.sh` روی دو ماشین KVM، run 41307934189، SHA `1189cf45`) این مسیر را نشان داد:

1. پس از یک `systemctl restart baft-agent` ساده روی IR، بازیابی پس از restart ingress را با شواهد kernel درست rollback کرد.
2. BCC گزارش agent را «replay متعارض» دانست و رد کرد.
3. نتیجه: تونل ACTIVE ماند ولی پورت عمومی برای همیشه بسته ماند. Reboot همین اثر را دارد.

تست‌های قبلی (`VerifiedIngressAfterPlainAgentRestart`) فقط پاک‌سازی امن را بررسی می‌کردند، نه برگشتن سرویس را.

**رفع:** ADR-003 §4.4، بخش «As implemented»:

- **Agent:**
  - اول reservation را آزاد می‌کند و سپس `released_after_restart` را گزارش می‌دهد.
  - گزارش تحویل‌نشده در هر poll دوباره ارسال می‌شود.
- **BCC:** فقط همین شکل دقیق را می‌پذیرد و با یک Apply امضاشدهٔ تازه با generation بعدی پاسخ می‌دهد.
- **Agent (admission):** همان Plan بازبینی‌شده را فقط پس از آزادسازیِ ناشی از rollback اعمال می‌پذیرد، آن هم با generation بالاتر و شناسه‌های تازه. پس از Retire یا decommission، Plan همچنان باید تازه باشد.

## شواهد اجرا

| SHA | اجرا | نتیجه |
|---|---|---|
| `c8a0bec9` (ادغام #292، #293 و #294) | ۱۱ workflow روی PR #301 | همه سبز |
| `1189cf45` (اولین self-test) | 41307934189، `two-host (real-hosts-selftest)` | **شکست:** پورت عمومی پس از restart برنگشت (باگ بالا) |
| `73b68baf` (رفع BCC) | 41309036181 | **شکست:** Agent همان Plan را پس از release رد کرد |
| `7ff0ec5c` (رفع admission) | 41309696078 و 41309701659، `two-host (real-hosts-selftest)` | **PASS** |

اندازه‌گیری‌های self-test روی `7ff0ec5c` (دو ماشین مجازی روی یک runner؛ **مسیر واقعی ایران↔خارج نیست**):

| سناریو | نتیجه |
|---|---|
| Restart هر دو agent | Apply تازه صادر شد (`restaged: true`)؛ Ready پس از ۱۰ ثانیه؛ ترافیک سالم |
| Firewall Drop روی پورت عمومی | BCC پس از ۱۴ ثانیه از Ready خارج شد (Ready کاذب نداشت)؛ پس از حذف قانون، Ready پس از ۶ ثانیه |
| قطع ۲۰ ثانیه‌ای مسیر تونل | در زمان قطع ترافیک واقعاً شکست؛ بازیابی ۱ ثانیه پس از برگشت مسیر |
| Reboot سرور IR | پس از بالا آمدن Ready شد و ترافیک سالم بود |
| Decommission | پورت بسته شد و قانون متعلق به BAFT حذف شد |

## مشاهدهٔ جانبی: آنچه uninstall کامل باقی می‌گذارد

پس از `baft uninstall --full --delete-tunnel-configs --stop-active-tunnels --yes`، روی هر دو ماشین این مسیرها باقی ماندند: `/etc/baft`، `/etc/baft-agent`، `/var/lib/baft`، `/var/lib/baft-agent` و `/usr/local/bin/baft-net`. روی IR که reboot شده بود، یک unit هم با وضعیت `not-found/active` باقی ماند.

- **مسیرهای داده:** نگه‌داشتن گواهی، audit و وضعیت agent بدون `--delete-*` طراحی uninstaller است.
- **`baft-net` و unit فعال:** این دو احتمالاً کمبود uninstaller هستند. باید جداگانه بررسی شوند (P2، نه مانع انتشار).
- **اثر روی LAB:** ممکن بود وضعیت باقی‌ماندهٔ agent (tombstoneهای generation) اجرای دوم LAB روی همان سرورها را رد کند. برای همین `real_hosts.sh`:
  - همهٔ `--delete-*`ها را به uninstall می‌دهد؛
  - چون full mode فقط روی سرور بدون BAFT اجرا می‌شود، باقی‌ماندهٔ مسیرهای BAFT را خودش پاک می‌کند؛
  - آنچه uninstaller باقی گذاشته بود را در لاگ (`uninstaller-left`) ثبت می‌کند.

</div>
