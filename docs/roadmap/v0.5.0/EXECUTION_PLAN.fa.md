<div dir="rtl" align="right" lang="fa">

# BAFT v0.5.0 — سند اجرایی و نقشه راه مهندسی

> وضعیت سند: **پیشنهاد اجرایی مبنا / Draft 0.1**  
> نسخه هدف: **v0.5.0**  
> شاخه برنامه‌ریزی: `codex/v0.5.0-foundation-20261031`  
> مخزن مرجع: `zarkmakerburg/BAFT`  
> شاخه مبنا: `main`  
> SHA مبنا در زمان ایجاد شاخه: `9c32ba369fe2be2c549f2878d1de5dbf092c8f64`  
> تاریخ: 2026-10-31  
> تصمیم انتشار: **NO-GO تا عبور از گیت‌های این سند و رفع وابستگی‌های نسخه قبلی**

---

## 0. خلاصه تصمیم و اصول حاکم

هدف v0.5.0 ساخت یک محصول یکپارچه و قابل استفاده برای مدیریت اتصال امن بین سرورهاست؛ نه ادغام سطحی دو کدبیس و نه انتقال بی‌قیدوشرط تمام پروتکل‌های C-BAFT.

**BAFT مرجع امنیت، هویت، مجوز، کنترل عملیات، مالکیت منابع، Journal، Recovery و انتشار باقی می‌ماند.** قابلیت‌های منتخب C-BAFT در قالب Driverهای مستقل و تحت قرارداد BAFT بررسی و در صورت اثبات ارزش، بازپیاده‌سازی یا اقتباس می‌شوند.

خروجی مورد انتظار: کاربر از یک پنل واحد بتواند Node اضافه کند، آمادگی را بررسی کند، مسیرهای Direct/Reverse را آزمایش کند، روش سازگار را انتخاب کند، اتصال را برقرار کند، سلامت و مصرف را ببیند و عملیات ناموفق را ایمن بازیابی کند؛ بدون نیاز به شناخت جزئیات Kernel و Firewall.

### اصول غیرقابل مذاکره

1. BCC مرجع نهایی سیاست، تصمیم و صدور Job است.
2. Agent فقط عملیات معتبر، امضاشده، مجاز و محدود به منابع مشخص را اجرا می‌کند.
3. هیچ Transport Driver مرجع مستقل مجوز یا مالکیت منابع نیست.
4. تمام اثرهای سیستمی باید قبل/بعد قابل ثبت و قابل بازیابی باشند.
5. منابع خارج از مالکیت اثبات‌شده BAFT هرگز حذف یا بازنویسی نمی‌شوند.
6. موفقیت صرفاً با پاسخ موفق یک دستور یا بازشدن یک پورت اعلام نمی‌شود؛ Health Check کاربردی لازم است.
7. شبیه‌سازی، تست محلی، تست LAB ایزوله و تست روی میزبان فیزیکی در گزارش‌ها جداگانه برچسب می‌خورند.
8. ویژگی جدید نباید امنیت یا مسیر پیش‌فرض ساده را تضعیف کند.
9. قابلیت‌های پرریسک یا کم‌ارزش به‌خاطر تعداد پروتکل‌ها وارد نسخه نمی‌شوند.
10. Merge، Tag، Release و Deploy فقط با مجوز مربوط و پس از عبور از گیت‌های همان SHA انجام می‌شوند.

---

## 1. خط مبنا و وابستگی نسخه‌ای

### 1.1 مبنای مخزن

این شاخه از `main` در SHA بالا ایجاد شده است. ایجاد شاخه و سند به‌معنای تغییر `main`، ادغام PRها، بستن Issueها یا انتشار نیست.

### 1.2 کارهای نسخه قبلی که نباید گم شوند

در زمان تدوین سند، PRهای مرتبط با چرخه ingress و lifecycle در مخزن باز بودند؛ از جمله:

- PR #292 — durable generation fence / committed port release identity.
- PR #293 — retry/recovery برای signed Retire پس از شکست تحویل receipt؛ وابسته به #292.
- PR #294 — lifecycle shutdown و بستن handlerها / WebSocket teardown.
- Issue #269 — در متن شواهد PR #293 هنوز OPEN معرفی شده بود.

این فهرست snapshot زمان تدوین است و باید در شروع هر مرحله دوباره از GitHub بررسی شود. وضعیت‌ها، SHAها و CIهای PRها ممکن است تغییر کرده باشند.

**قانون وابستگی:** کار v0.5.0 نباید P0های باز نسخه قبلی را پنهان کند یا با کپی‌کردن کد از شاخه‌های ناتمام، آن‌ها را به‌عنوان قابلیت پایدار فرض کند. قبل از شروع هر تغییر runtime که به ingress، lease، listener، firewall، receipt یا recovery وابسته است:
- آخرین main، Issueهای P0، PRهای مرتبط و CI دقیقاً روی SHA فعلی بررسی شوند.
- فقط تغییرات merge‌شده یا تغییراتی که به‌طور رسمی در شاخه کاری v0.5.0 وارد و مستقل اعتبارسنجی شده‌اند مبنا باشند.
- تست‌های موفق والد یا PR دیگر به SHA جدید منتقل‌شده فرض نشوند.
- شواهد شکست و محدودیت محیطی حفظ شوند.

### 1.3 C-BAFT به‌عنوان مرجع قابلیت

C-BAFT یک کدبیس مستقل مشتق‌شده از C-Tunnel است و نباید با BAFT به‌عنوان یک مخزن واحد تلقی شود. بررسی قبلی، قابلیت‌های ارزشمند UX و شبکه‌ای را نشان داده، اما چند ریسک مهم نیز شناسایی شده است:

- Panel ممکن است روی تمام Interfaceها و بدون TLS اجباری در دسترس باشد.
- پاک‌سازی Interface در برخی مسیرها بر نام تکیه می‌کند، نه اثبات مالکیت.
- تغییر `net.ipv4.ip_forward` بدون بازیابی مقدار قبلی مشاهده شده است.
- Installer پیش‌فرض به upstream اشاره می‌کرد، نه لزوماً fork.
- Checksum از همان منبع Release دریافت می‌شود و به‌تنهایی اصالت ناشر را اثبات نمی‌کند.
- نصب چند فایل، تراکنش کامل و rollback جامع نداشت.
- WireGuard tools برای نصب عمومی اجباری بود، حتی وقتی Transport انتخابی به آن نیاز نداشت.
- بعضی نام‌های پروتکل، پیاده‌سازی کامل استاندارد مربوطه را تضمین نمی‌کردند.

بنابراین سیاست این است: **ایده و رفتار مفید اقتباس شود؛ پیاده‌سازی و مدل امنیتی بدون ممیزی منتقل نشود.**

---

## 2. تعریف محصول و محدوده نسخه

### 2.1 کاربرهای هدف

1. کاربر عادی: دو یا چند سرور را ثبت و اتصال پیشنهادی را بدون دانش شبکه برقرار کند.
2. مدیر سرور: وضعیت، کیفیت، مصرف، رخدادها و بازیابی را مدیریت کند.
3. کاربر حرفه‌ای: Transport و پارامترهای پشتیبانی‌شده را با کنترل و شواهد فنی تنظیم کند.

### 2.2 تجربه نهایی کاربر

1. نصب BCC یا ورود به پنل موجود.
2. نصب/ثبت Agent روی Nodeها.
3. بررسی سلامت، نسخه، سازگاری و پیش‌نیازها.
4. انتخاب اتصال خودکار یا مسیر دستی Direct/Reverse.
5. اجرای Connection Test واقعی.
6. مشاهده مقایسه روش‌های قابل پشتیبانی و علت انتخاب.
7. مرور Plan و اثرهای موردنیاز.
8. تأیید و اجرای Job امضاشده.
9. Health Check کاربردی و ثبت وضعیت.
10. مانیتورینگ و عیب‌یابی.
11. بازیابی کنترل‌شده در شکست یا افت کیفیت.
12. Upgrade، Rollback و Uninstall امن.

### 2.3 خارج از محدوده مگر با تصمیم جداگانه

- تبدیل BAFT به پنل فروش اشتراک یا مدیریت کاربران سرویس‌های ثالث.
- انتقال تمام ۲۸ روش ادعاشده C-BAFT بدون نیازسنجی و اعتبارسنجی.
- ادعای سازگاری کامل DTLS/QUIC صرفاً بر اساس شباهت قالب بسته.
- پیاده‌سازی هم‌زمان تمام قابلیت‌های L2 overlay، FEC و multipath در اولین برش.
- تغییر زیرساخت production یا مهاجرت سرویس‌های فعال در جریان توسعه.
- هرگونه تغییر در GoldApp یا سرویس‌های بیرونی بدون Issue و مجوز جداگانه.

---

## 3. معماری هدف

### 3.1 لایه‌ها و مالکیت مسئولیت

| لایه | مسئولیت | ممنوعیت |
|---|---|---|
| BCC Control Plane | سیاست، هویت، مجوز، plan، انتخاب مسیر، صدور Job | اجرای مستقیم shell یا دورزدن Agent |
| API / Protocol | schema، version negotiation، validation | پذیرش schema ناشناخته یا downgrade ناامن |
| Agent | اجرای Job معتبر، گزارش نتیجه و وضعیت | اجرای دستور آزاد یا تغییر خارج از scope |
| Journal | ثبت intent، checkpoint، effect، receipt و terminal state | ثبت موفقیت پیش از اثبات اثر |
| Ownership Registry | شناسه و شواهد مالکیت منابع | مالکیت بر اساس نام یا comment به‌تنهایی |
| Firewall / Network Manager | تغییر منابع متعلق به BAFT | حذف rule/interface/route نامرتبط |
| Transport Drivers | آماده‌سازی، اعمال، probe، health و cleanup روش انتقال | اختیار مستقل برای صدور مجوز |
| Path Health | سنجش کیفیت و روند | تبدیل داده stale به وضعیت live |
| Recovery Manager | retry، reconciliation، failover و rollback | پاک‌کردن evidence برای ساختن موفقیت ظاهری |
| BCC UI | نمایش قابل فهم و عملیات کاربر | Backend موازی یا تصمیم امنیتی در مرورگر |
| Installer / Updater | نصب، اعتبارسنجی، upgrade و rollback | اجرای artifact تأییدنشده با root |

### 3.2 قرارداد Transport Driver

هر Driver باید قرارداد مشترک و نسخه‌بندی‌شده داشته باشد. نام متدها در طراحی اولیه:

- `Capabilities`: قابلیت‌ها و محدودیت‌ها.
- `Preflight`: بررسی فقط‌خواندنی پیش‌نیازها.
- `Plan`: فهرست اثرها و منابع موردنیاز.
- `Prepare`: آماده‌سازی بدون فعال‌سازی نهایی.
- `Apply`: اجرای اثرهای مجاز و محدود.
- `Probe`: اثبات عبور داده کاربردی.
- `Health`: وضعیت عملیاتی و متریک‌ها.
- `Rollback`: بازگردانی effectهای متعلق به همان operation.
- `Cleanup`: پاک‌سازی پس از اثبات مالکیت و نبود وابستگی.
- `UpgradeCompatibility`: اعلام سازگاری با نسخه Agent/peer.

هر Driver باید موارد زیر را اعلام کند:
- شناسه و نسخه Driver.
- خانواده انتقال و توپولوژی‌های پشتیبانی‌شده.
- OS، Kernel و ابزارهای موردنیاز.
- پورت‌ها و پروتکل‌های شبکه.
- نیازمندی privilege/capability.
- وضعیت رمزنگاری و احراز هویت؛ صریحاً مشخص شود اگر خودش رمزنگاری ندارد.
- محدودیت MTU، encapsulation و سربار.
- منابعی که ایجاد یا تغییر می‌دهد.
- روش اثبات مالکیت هر منبع.
- روش recovery و محدودیت rollback.
- ماتریس سازگاری و سطح بلوغ.

**شرط طراحی:** Driver نباید shell آزاد دریافت کند. ورودی باید typed، محدود، validate شده و به عملیات مشخص تبدیل شود.

### 3.3 چرخه عمر اتصال

وضعیت‌های پیشنهادی، با تطبیق نهایی با state machine موجود:

`Draft → Preflighted → Planned → Prepared → Applying → Verifying → Active`

حالت‌های شکست و نگهداری:

`ApplyFailed`, `VerificationFailed`, `Degraded`, `Recovering`, `RollingBack`, `Retiring`, `Retired`, `Unknown`.

قواعد:
- هر transition باید guard و event ثبت‌شده داشته باشد.
- `Active` فقط پس از probe کاربردی و تأیید اثرهای لازم صادر شود.
- timeout به‌معنای شکست قطعی effect نیست؛ reconciliation لازم است.
- `Unknown` باید در UI نامشخص نمایش داده شود، نه سالم یا قطع.
- عملیات تکراری باید idempotent و به شناسه operation متصل باشند.
- هر effect باید correlation ID و ownership evidence داشته باشد.

---

## 4. قابلیت‌های منتخب C-BAFT و نحوه اقتباس

| قابلیت مرجع | ارزش برای BAFT | نحوه ورود | اولویت |
|---|---|---|---|
| Connection Test | اثبات مسیر پیش از Apply | بازطراحی در BCC/Agent با probe کاربردی | P0 |
| Direct / Reverse | انتخاب توپولوژی مناسب | اتصال به Connection Plan و lifecycle | P0 |
| Deploy Best Direct/Reverse | کاهش پیچیدگی کاربر | Auto Select با توضیح دلیل و confidence | P0 |
| Tunnel filters/search | مدیریت چند اتصال | UI واحد بر اساس API BAFT | P1 |
| Traffic/host metrics | مشاهده وضعیت | متریک واقعی، timestamp و منبع مشخص | P1 |
| MTU discovery | کاهش fragmentation | آزمایش کنترل‌شده و پیشنهاد؛ اعمال با Plan | P1 |
| Multi-Lane | افزایش ظرفیت در شرایط مناسب | آزمایش مستقل، سپس Driver extension | P2 |
| FEC | تحمل loss در شرایط خاص | benchmark و اندازه‌گیری سربار | P2 |
| GRE/IPIP | سناریوهای L3 خاص | Driver اختیاری، بدون ادعای رمزنگاری | P2 |
| VXLAN/GENEVE | L2 overlay | فقط پس از threat model و نیاز واقعی | P2 |
| UDP-Crypt | انتقال UDP رمزگذاری‌شده | طراحی/ممیزی رمزنگاری مستقل؛ نه کپی کور | P2 / gated |

### 4.1 Connection Test Engine

حداقل سنجش‌ها:
- کنترل reachability دوطرفه.
- کنترل DNS و آدرس مقصد در صورت کاربرد.
- بررسی پورت و نوع transport.
- RTT، jitter و packet loss با روش و نمونه‌گیری ثبت‌شده.
- MTU قابل استفاده و شناسایی fragmentation.
- throughput با محدودیت مدت و حجم.
- پایداری و reconnect.
- probe کاربردی که واقعاً داده را از مسیر مورد نظر عبور دهد.
- مقایسه Direct و Reverse.
- ثبت نسخه، شرایط، timestamp، نمونه‌ها و نتیجه خام.

هر نتیجه باید برچسب روش داشته باشد: `synthetic`, `loopback`, `isolated-lab`, `physical-host`. مقایسه نتایج ناهم‌شرایط ممنوع است.

### 4.2 Auto Select

انتخاب بر اساس کمترین ping به‌تنهایی ممنوع است. الگوریتم باید:
- ابتدا روش‌های ناسازگار یا ناامن را حذف کند.
- سپس availability و probe واقعی را الزام کند.
- loss، jitter، latency، throughput، پایداری و هزینه منابع را در نظر بگیرد.
- حداقل نمونه و confidence تعریف کند.
- tie-break deterministic داشته باشد.
- پیشنهاد اصلی و جایگزین را همراه علت انتخاب/رد نشان دهد.
- امکان override دستی را برای نقش مجاز فراهم کند.
- از تغییر مسیر مکرر با hysteresis، cooldown و minimum dwell time جلوگیری کند.
- تصمیم را ثبت کند تا قابل بازبینی باشد.

وزن‌ها باید با benchmark و سناریو تعیین شوند، نه با اعداد سلیقه‌ای ثابت. معیارها و نسخه الگوریتم باید همراه نتیجه ذخیره شوند.

### 4.3 Path Health و Failover

Health Monitor متریک‌ها را جمع می‌کند؛ اما اختیار تغییر مسیر را به‌تنهایی ندارد. BCC policy را ارزیابی و Job صادر می‌کند. Failover فقط پس از آماده‌سازی و probe مسیر جایگزین انجام می‌شود و در شکست به وضعیت قبلی برمی‌گردد. از split-brain، دو مالک هم‌زمان و مسیرهای متداخل جلوگیری شود.

---

## 5. تجربه کاربری و پنل واحد

### 5.1 اصل محصول

دو پنل موازی نسازیم. قابلیت‌های مفید پنل C-BAFT به پنل BCC منتقل می‌شوند؛ UI از API رسمی BAFT داده می‌گیرد و هر تصمیم امنیتی در backend باقی می‌ماند.

### 5.2 بخش‌های پنل

1. Dashboard — وضعیت Nodeها، اتصال‌ها، هشدارها و مصرف.
2. Nodes — افزودن، Pair، وضعیت Agent، نسخه و قابلیت‌ها.
3. Connections — فهرست، جست‌وجو، فیلتر، جزئیات و lifecycle.
4. Connection Wizard — ساخت اتصال مرحله‌ای.
5. Connection Test — مقایسه مسیر و روش انتقال.
6. Health — latency، jitter، loss، throughput و reconnect.
7. Operations / Journal — عملیات، مراحل، actor، زمان و نتیجه.
8. Security — هویت، enrollment، مجوزها و revoke.
9. Updates — نسخه، سازگاری، upgrade و rollback.
10. Settings — زبان، اعلان‌ها و تنظیمات سطح مجاز.

### 5.3 سطوح کاربری

- ساده: افزودن Node، اتصال خودکار، تست، تأیید، وضعیت.
- پیشرفته: انتخاب Direct/Reverse، مشاهده Transport، تنظیمات قابل پشتیبانی.
- حرفه‌ای: پارامترهای Driver، MTU، مسیر، diagnostics و سیاست‌ها.

سطح ساده نباید کنترل امنیتی ضعیف‌تری داشته باشد.

### 5.4 Wizard

1. **افزودن سرورها:** enrollment، سازگاری، نسخه، دسترسی و capability.
2. **انتخاب اتصال:** auto یا انتخاب دستی مجاز.
3. **آزمایش:** اجرای probe و نمایش مقایسه همراه علت.
4. **اعمال:** نمایش plan، اثرها و هشدارها؛ تأیید کاربر؛ اجرای signed Job؛ verification و recovery.

### 5.5 الزامات UX

- فارسی RTL و انگلیسی LTR از ابتدا.
- Responsive واقعی برای موبایل، نه فقط CSS بدون آزمون.
- پیام خطا شامل «چه شد / چه چیزی تحت تأثیر است / اقدام بعدی».
- جزئیات فنی در بخش بازشونده.
- نمایش timestamp و تازگی داده.
- وضعیت نامشخص در قطع telemetry.
- progress مرحله‌ای عملیات طولانی.
- تأیید صریح عملیات مخرب و نمایش منابع هدف.
- عدم افشای token، private key یا secret در UI و log.
- accessibility: keyboard، focus، labels، contrast و screen-reader.
- هیچ دکمه‌ای نباید صرفاً ظاهری یا بدون action واقعی باشد.

---

## 6. امنیت، مالکیت و تهدیدها

### 6.1 تهدیدهای اصلی

- جعل Node یا Agent.
- replay یا اجرای دوباره Job.
- استفاده از Job منقضی یا لغوشده.
- دست‌کاری plan بین تأیید و Apply.
- TOCTOU در بررسی socket، interface، route و firewall.
- حذف منابع متعلق به سرویس دیگر.
- نشت secret در log، error، URL یا backup.
- downgrade یا نصب artifact جعلی.
- CSRF، session theft و دسترسی مدیریتی ناامن.
- resource exhaustion از probe یا تست throughput.
- failover loop و oscillation.
- ناسازگاری نسخه و schema.

### 6.2 کنترل‌های لازم

- هویت Node مستقل و قابل revoke.
- mutual authentication روی کانال کنترل.
- امضای Job و اتصال آن به Node، operation، plan hash، expiry و nonce.
- anti-replay پایدار و idempotency.
- قفل/lease در تمام بازه اثرهای حساس، مطابق مدل authority موجود.
- ثبت ownership manifest شامل resource identity و شواهد سیستم‌عامل.
- verify-before-delete برای هر resource.
- حفظ مقدار قبلی sysctl، route و firewall و بازیابی دقیق آن.
- secret storage مناسب و عدم log کردن secret.
- TLS اجباری برای پنل راه دور؛ secure cookie، CSRF protection، session expiry و revoke.
- installer مبتنی بر release ثابت، manifest و امضای مستقل مورد اعتماد.
- systemd hardening با حداقل privilege سازگار با نیاز.
- rate limit و سقف حجم/مدت تست.
- security review مستقل و regression tests برای هر مورد.

### 6.3 Ownership Registry

مالکیت نباید صرفاً از نام Interface یا Comment در Firewall نتیجه شود. حداقل باید ترکیبی از:
- شناسه operation و Node.
- resource type و identity پایدار.
- metadata ثبت‌شده در Journal.
- شواهد readback از سیستم‌عامل.
- owner generation / lifecycle state.
- dependency reference و tombstone در حذف.
استفاده شود. اگر مالکیت قابل اثبات نیست، عملیات حذف باید fail-closed شود.

---

## 7. Installer، Upgrade، Rollback و Uninstall

### 7.1 نصب

1. تشخیص distro، architecture، kernel و init system.
2. بررسی پشتیبانی رسمی.
3. بررسی منابع، فضای دیسک و پورت‌ها.
4. تشخیص سرویس‌ها و resource conflict.
5. نمایش plan قبل از تغییر.
6. دریافت manifest نسخه ثابت.
7. اعتبارسنجی امضای release با trust root از پیش تعریف‌شده.
8. بررسی hash هر asset.
9. نصب dependency فقط برای Driver انتخاب‌شده.
10. نصب فایل‌ها در staging.
11. backup و ثبت state قبلی.
12. اعمال اتمیک و transactional.
13. نصب unitهای systemd با hardening.
14. راه‌اندازی Agent.
15. enrollment امن و ثبت Node.
16. health check کاربردی.
17. گزارش نتیجه، مسیر log و recovery instructions.

### 7.2 Upgrade

- سازگاری نسخه BCC/Agent/Driver و schema.
- رد downgrade ناامن.
- دریافت artifact امضاشده و verify پیش از اجرا.
- snapshot تنظیمات و state مورد نیاز.
- migrationهای نسخه‌ای و idempotent.
- نصب staged و atomic.
- restart کنترل‌شده.
- verify واقعی.
- rollback خودکار در شکست قابل بازیابی.
- حفظ audit trail.

### 7.3 Uninstall

- preview منابعی که قرار است حذف شوند.
- توقف کنترل‌شده.
- حذف فقط منابع با مالکیت اثبات‌شده و بدون dependent.
- حفظ یا حذف تنظیمات طبق انتخاب روشن کاربر.
- نگهداری evidence و گزارش.
- فهرست منابعی که به‌دلیل عدم اثبات مالکیت باقی مانده‌اند.
- عدم اجرای پاک‌سازی گسترده بر اساس prefix یا نام تنها.

### 7.4 پشتیبانی سیستم‌عامل

در هر release matrix دقیقاً OS، نسخه، معماری، kernel، systemd و Driverهای پشتیبانی‌شده مشخص شود. پشتیبانی از distro جدید فقط با اجرای نصب/upgrade/rollback/uninstall در محیط متناظر اعلام شود.

---

## 8. برنامه اجرایی مرحله‌ای

هر مرحله باید Issue/PR مستقل، owner مشخص، تست RED قبل از fix برای regression، تست GREEN پس از fix و معیار خروج داشته باشد.

### Stage 0 — Baseline و وابستگی‌ها (P0)

- بررسی آخرین main و release/tag.
- بررسی Issueهای P0 و PRهای باز v0.4.x.
- ثبت exact SHA، tree، CI و وضعیت تست.
- استخراج معماری فعلی BCC/Agent/Journal/Firewall/Wizard.
- inventory قابلیت‌های موجود؛ جلوگیری از بازنویسی.
- تهیه threat model و risk register.
- ثبت feature matrix از C-BAFT با evidence.
- تعیین مواردی که باید به v0.5.1 موکول شوند.

**خروجی:** Baseline report، dependency graph و تصمیم scope.

### Stage 1 — Architecture Contracts (P0)

- Transport interface و versioning.
- Capability schema.
- Connection Plan و hash.
- lifecycle state machine.
- ownership registry contract.
- Job/Journal event model.
- error taxonomy.
- API compatibility.
- test fixtures و contract tests.

**خروجی:** قراردادهای قابل تست و review؛ هنوز Driver جدید به production behavior متصل نشود.

### Stage 2 — Connection Test Engine (P0)

- preflight دوطرفه.
- probe کاربردی.
- metrics و raw evidence.
- bounded throughput test.
- MTU discovery read-only.
- result schema و persistence.
- تست false-positive و stale telemetry.

**خروجی:** تست مسیر قابل تکرار بدون Apply ناخواسته.

### Stage 3 — Driver Framework و Direct/Reverse (P0)

- انتقال قابلیت‌های موجود BAFT به contract جدید بدون regression.
- پیاده‌سازی/سازگارسازی Direct.
- پیاده‌سازی/سازگارسازی Reverse.
- prepare/apply/verify/rollback.
- resource ownership.
- lifecycle/restart/recovery.
- version compatibility.

**خروجی:** اتصال واقعی و قابل بازیابی در topologyهای رسماً پشتیبانی‌شده.

### Stage 4 — Auto Select (P0)

- eligibility filtering.
- scoring قابل توضیح.
- confidence و حداقل sample.
- deterministic tie-break.
- hysteresis/cooldown.
- fallback و manual override.
- decision audit.

**خروجی:** انتخاب مسیر بر اساس داده واقعی، نه ادعای سرعت یا ping.

### Stage 5 — Health و Failover (P0 پایه)

- health telemetry.
- threshold policy.
- stale-data detection.
- degraded state.
- alternate path probe.
- signed failover job.
- recovery و anti-flap.

**خروجی:** تشخیص افت و جابه‌جایی کنترل‌شده با rollback.

### Stage 6 — BCC Unified UX (P0)

- dashboard.
- node management.
- connection list/details.
- wizard.
- test comparison.
- health view.
- operations/journal.
- FA/RTL و EN/LTR.
- responsive/mobile و accessibility.

**خروجی:** کاربر بتواند end-to-end بدون CLI اتصال را ایجاد و مدیریت کند.

### Stage 7 — Installer و lifecycle بسته‌ها (P0)

- secure installer.
- signed manifest و trust validation.
- dependency selection.
- transactional install.
- upgrade/migration.
- rollback.
- uninstall.
- diagnostics bundle با redaction.

**خروجی:** نصب و نگهداری تکرارپذیر در OSهای اعلام‌شده.

### Stage 8 — قابلیت‌های بهینه‌سازی (P1/P2)

به ترتیب و فقط بر اساس شواهد:
1. MTU tuning با rollback.
2. Multi-Lane benchmark و سپس implementation.
3. FEC benchmark و سپس implementation.
4. L3 Driverهای منتخب.
5. L2 overlay در صورت نیاز و threat model.
6. UDP-Crypt فقط پس از طراحی و ممیزی رمزنگاری.

**خروجی:** هر قابلیت دارای benchmark و cost/benefit مشخص.

### Stage 9 — LAB و Release Acceptance (P0)

- ماتریس چند Node.
- شرایط شبکه کنترل‌شده.
- نصب/upgrade/rollback/uninstall.
- failover و process crash.
- ownership adversarial tests.
- security review.
- UI browser acceptance.
- docs و packaging.
- final SHA validation.

**خروجی:** release dossier قابل بازبینی.

---

## 9. برنامه LAB

### 9.1 توپولوژی پایه

شش Node مستقل:
- IR-01 و IR-02: دو Node ایران.
- EU-01 تا EU-04: چهار Node خارجی از کشورها/دیتاسنترهای متفاوت، در صورت فراهم بودن محیط واقعی.

کشور، provider و latency باید از محیط واقعی ثبت شوند؛ مقدار جغرافیایی یا پینگ از پیش فرض نشود.

### 9.2 طبقه‌بندی محیط

- Unit/in-process: فقط منطق محدود.
- Network namespace/container: رفتار kernel/network در محیط ایزوله.
- VM/LAB node: lifecycle و سرویس واقعی در محیط آزمایش.
- Physical host: آزمون واقعی روی میزبان مستقل.
- Network emulation: تأخیر/loss/bandwidth تزریقی؛ به‌عنوان شبیه‌سازی گزارش شود.

### 9.3 سناریوها

1. نصب و enrollment هر Node.
2. pair و revoke.
3. تمام pairهای هدفمند و topologyهای مجاز.
4. Direct و Reverse.
5. connection probe و auto-select.
6. RTT/jitter/loss/throughput.
7. MTU و fragmentation.
8. restart Agent و BCC.
9. reboot Node.
10. قطع مسیر و failover.
11. قطع هنگام Prepare، Apply و Verify.
12. crash بین durable writeها.
13. retry و reconciliation.
14. duplicate/replay/expired job.
15. ownership collision با منابع غیر BAFT.
16. Firewall و sysctl restore.
17. upgrade از آخرین release پشتیبانی‌شده.
18. rollback بعد از failure injection.
19. uninstall با dependent و بدون dependent.
20. تست هم‌زمان چند connection و load.
21. تست پنل روی موبایل/دسکتاپ و FA/EN.
22. backup/restore و redaction.

### 9.4 شواهد اجباری

برای هر run:
- exact commit SHA و tree.
- نسخه Binary و Driver.
- OS/kernel و topology.
- تنظیمات و seed سناریو.
- زمان شروع/پایان.
- command/test identifier.
- PASS/FAIL/SKIP با علت.
- log و artifact checksum.
- منبع داده و طبقه‌بندی واقعی/شبیه‌سازی.
- نتیجه cleanup و سلامت بعد از تست.

هیچ SKIP به‌عنوان PASS گزارش نشود. محدودیت محیطی از failure محصول تفکیک شود اما پنهان نشود.

---

## 10. تست و کیفیت

### 10.1 سطوح تست

- Unit: validation، scoring، state transition، ownership.
- Contract: Driver/API/Job schema.
- Integration: BCC → signed Job → Agent → OS effect → receipt.
- Fault injection: crash، timeout، partial apply، lost receipt.
- Race: concurrency در lease، lifecycle، Journal و UI backend.
- Security: replay، signature، privilege، CSRF، secret leakage، ownership.
- System: install، restart، reboot، upgrade، rollback، uninstall.
- Browser: wizard، accessibility، RTL/LTR، mobile.
- Performance: CPU، memory، throughput، latency، jitter، recovery p50/p95/p99.

### 10.2 قواعد کیفیت

- regression مهم باید ابتدا با تستی که قبل از fix شکست می‌خورد اثبات شود.
- هر تغییر باید test مربوطه را در همان SHA اجرا کند.
- تست‌های والد یا شاخه دیگر به head جدید منتقل‌شده فرض نمی‌شوند.
- CI سبز جای تست رفتاری یا security review را نمی‌گیرد.
- benchmark بدون baseline هم‌شرایط پذیرفته نیست.
- تست flaky باید ریشه‌یابی شود؛ افزایش retry به‌تنهایی درمان محسوب نمی‌شود.
- محدودیت محیطی و test skipها دقیقاً گزارش شوند.

---

## 11. Installer و تجربه کاربر: معیار پذیرش

یک کاربر تازه‌کار باید بتواند:
- بسته رسمی را تشخیص دهد و اصالت آن را verify کند.
- روی OS پشتیبانی‌شده نصب کند.
- Node را با enrollment امن اضافه کند.
- خطای پیش‌نیاز را با پیام قابل فهم حل کند.
- اتصال را با wizard بسازد.
- از پنل بفهمد اتصال واقعاً Active است یا نه.
- latency/loss و آخرین زمان نمونه‌گیری را ببیند.
- علت انتخاب مسیر را بفهمد.
- عملیات را بدون دانش shell متوقف یا بازیابی کند.
- upgrade و rollback را از مسیر مستند انجام دهد.

برای release candidate، حداقل یک آزمون usability با کاربران غیرتوسعه‌دهنده انجام شود؛ اگر کاربر برای تکمیل مسیر اصلی به توضیح شفاهی توسعه‌دهنده نیاز داشت، UX هنوز آماده نیست.

---

## 12. Release Gates

نسخه فقط زمانی قابل انتشار است که همه موارد مربوط به scope رسمی release پاس شده باشند:

| Gate | معیار |
|---|---|
| G0 Baseline | وضعیت P0 و dependencyهای قبلی روشن |
| G1 Architecture | قراردادها و state machine review شده |
| G2 Security | هیچ Critical/High حل‌نشده در scope |
| G3 Driver | lifecycle و ownership هر Driver اثبات‌شده |
| G4 Connection | probe کاربردی و مسیر واقعی موفق |
| G5 Auto Select | انتخاب تکرارپذیر و قابل توضیح |
| G6 Recovery | failure injection و rollback موفق |
| G7 UI | مسیر کامل کاربر در browser acceptance |
| G8 Installer | install/upgrade/rollback/uninstall موفق |
| G9 Compatibility | ماتریس نسخه و OS تأییدشده |
| G10 LAB | سناریوهای اجباری بدون skip پنهان |
| G11 CI | workflowهای لازم روی exact release SHA موفق |
| G12 Packaging | artifact، manifest، hash و signature معتبر |
| G13 Docs | راهنمای نصب، استفاده، عیب‌یابی و recovery کامل |

**شرط NO-GO:** هر نقصی که منجر به اجرای Job غیرمجاز، حذف منبع نامرتبط، نشت Secret، نصب artifact تأییدنشده، گزارش Active کاذب، rollback غیرقابل اعتماد یا شکست recovery شود.

---

## 13. ساختار پیشنهادی Issue / PR

شناسه‌های زیر پیشنهادند و تا زمان ایجاد واقعی Issue، شناسه رسمی GitHub نیستند:

| Workstream | خروجی | اولویت |
|---|---|---|
| W00 Baseline | inventory، P0 dependency، test baseline | P0 |
| W01 Contracts | Driver/API/Job/lifecycle schemas | P0 |
| W02 Ownership | resource identity و cleanup proof | P0 |
| W03 Probe Engine | connection test و evidence | P0 |
| W04 Direct Driver | قرارداد و lifecycle Direct | P0 |
| W05 Reverse Driver | قرارداد و lifecycle Reverse | P0 |
| W06 Auto Select | scoring و explainability | P0 |
| W07 Health | telemetry و degraded state | P0 |
| W08 Failover | policy، signed job، recovery | P0 |
| W09 BCC UX | dashboard، wizard، operations | P0 |
| W10 Installer | install/upgrade/rollback/uninstall | P0 |
| W11 Security | threat review و adversarial tests | P0 |
| W12 LAB | multi-node و fault matrix | P0 |
| W13 Optimization | MTU، Multi-Lane، FEC | P1/P2 |
| W14 Release | final acceptance و artifacts | P0 |

هر PR:
- یک مسئولیت اصلی.
- محدود و قابل review.
- تست و evidence در body.
- فایل‌های خارج از scope ممنوع.
- تغییر schema با migration و compatibility tests.
- هیچ merge یا release خودکار از این سند مجاز نمی‌شود.

---

## 14. گزارش‌دهی و کنترل تغییر

گزارش هر مرحله فقط شامل موارد زیر باشد:
1. Stage و وضعیت: DONE / IN PROGRESS / BLOCKED.
2. Branch و exact HEAD SHA.
3. تغییرات واقعی و فایل‌های تغییرکرده.
4. LOC اضافه/حذف، با روش محاسبه مشخص.
5. تست‌های اجراشده، نتیجه، SKIP و محدودیت.
6. CI run و لینک exact SHA.
7. ریسک و مانع باقی‌مانده.
8. تصمیم یا dependency مرحله بعد.

از گزارش برنامه به‌جای اجرای واقعی اجتناب شود. اگر دسترسی یا مجوز واقعی وجود ندارد، مانع صریح ثبت شود.

### حفاظت از مخزن

- `main` و شاخه‌های انتشار مستقیم نوشته نشوند.
- شاخه‌های موجود و stashها حذف یا reset نشوند.
- force-push ممنوع.
- previewهای LAB در GitHub منتشر نشوند.
- secret، key، raw PCAP و داده حساس در Issue/PR قرار نگیرد.
- merge، tag، release و deploy فقط با مجوز جداگانه انجام شوند.
- هر PR و commit باید به Issue و evidence مربوط پیوند داشته باشد.

---

## 15. ترتیب شروع کار

ترتیب اجرای عملی:

1. بازبینی وضعیت جاری GitHub و تأیید اینکه branch هنوز از main مبنای درست دارد.
2. تکمیل Stage 0 و تهیه baseline واقعی.
3. بررسی دقیق P0های نسخه قبلی و عدم تداخل با PRهای باز.
4. استخراج کد/قرارداد فعلی برای Driver، Job، Journal، ownership و Wizard.
5. نوشتن ADRهای لازم برای تصمیم‌های معماری.
6. اجرای contract tests قبل از پیاده‌سازی.
7. شروع کوچک‌ترین vertical slice: یک مسیر Direct end-to-end با plan، signed Job، ownership، Journal، probe و rollback.
8. سپس Reverse و Auto Select.
9. سپس UI، Installer و LAB.
10. در پایان قابلیت‌های بهینه‌سازی فقط بر اساس شواهد benchmark.

**اولین برش قابل تحویل (Vertical Slice):**
از پنل یک Connection Plan ساخته شود؛ BCC آن را اعتبارسنجی و امضا کند؛ Agent مجاز اجرا کند؛ منابع با ownership evidence ایجاد شوند؛ probe واقعی داده را عبور دهد؛ Journal وضعیت نهایی را ثبت کند؛ و failure injection ثابت کند rollback فقط منابع همان operation را حذف می‌کند.

این برش از افزودن چندین Transport بدون lifecycle کامل ارزشمندتر است و ریسک معماری را زود آشکار می‌کند.

---

## 16. تصمیم‌های باز که باید در Stage 0/1 با شواهد حل شوند

1. کدام Transport فعلی BAFT از نظر معماری و نگهداری بهترین پایه Driver است؟
2. کدام قابلیت C-BAFT واقعاً به نیاز کاربر BAFT پاسخ می‌دهد و کدام فقط feature count را زیاد می‌کند؟
3. مرز دقیق BCC و Agent در probeهای شبکه چیست؟
4. کدام metrics از قبل وجود دارند و کدام نیاز به instrumentation دارند؟
5. مدل مالکیت فعلی چه شکاف‌هایی در Interface، Route، nftables/iptables و sysctl دارد؟
6. نسخه‌بندی schema و compatibility window چگونه با releaseهای قبلی سازگار می‌شود؟
7. چه OS/Kernelهایی در v0.5.0 رسماً پشتیبانی می‌شوند؟
8. آیا Multi-Lane/FEC در داده benchmark بهبود معنی‌دار می‌دهند؟
9. آیا کاربر واقعاً به L2 overlay نیاز دارد یا scope محصول باید روی tunnelهای هدف فعلی متمرکز بماند؟
10. کدام عملیات باید approval انسانی داشته باشد و کدام با policy قابل خودکارسازی است؟

تا زمان پاسخ مستند به این پرسش‌ها، پیاده‌سازی قابلیت‌های پرریسک P2 آغاز نشود.

---

## 17. Definition of Done نسخه

v0.5.0 زمانی Done است که:
- مسیر اصلی کاربر از نصب تا اتصال و نگهداری بدون CLI تکمیل شود.
- حداقل Transportهای رسمی نسخه دارای lifecycle کامل و پشتیبانی مستند باشند.
- Direct/Reverse و Auto Select با شواهد واقعی کار کنند.
- BCC و Agent از یک مدل امن و واحد استفاده کنند.
- تمام effectهای سیستمی ownership و rollback قابل اثبات داشته باشند.
- telemetry و UI داده واقعی و timestamp معتبر نشان دهند.
- installer و updater امضای مستقل و recovery داشته باشند.
- تست‌های امنیتی، race، integration، LAB و browser acceptance روی exact SHA پاس شوند.
- هیچ P0 یا Critical/High حل‌نشده در محدوده release باقی نماند.
- مستندات فارسی و انگلیسی برای نصب، استفاده، ارتقا و بازیابی منتشر شوند.
- release dossier شامل source SHA، artifact hashes، signatures، workflow runs و محدودیت‌ها باشد.

**پایان سند — این سند مجوز merge، tag، release یا deploy نیست.**

</div>
