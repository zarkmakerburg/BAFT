<div dir="rtl" align="right" lang="fa">

# 40 — یادداشت‌های مهندسی

این صفحه مطالب مهندسی‌ای را نگه می‌دارد که قبلاً در صفحهٔ اول (`README.fa.md`) بود. صفحهٔ اول حالا برای کاربر و اپراتور نوشته شده و این صفحه برای بازبین‌ها و مشارکت‌کننده‌هاست. چیزی حذف نشده، فقط جابه‌جا شده است.

English: [../en/40-engineering-notes.md](../en/40-engineering-notes.md)

<p dir="ltr" align="left">
  <strong>Bounded · Authenticated · Fail-safe · Transactional</strong>
</p>

## نشان‌های داخلی

این نشان‌ها قبلاً بالای README فارسی بودند. وضعیت گیت‌های مهندسی داخلی را نشان می‌دهند، نه قابلیت کاربری:

<p dir="ltr" align="left">
  <img alt="ECRL" src="https://img.shields.io/badge/ECRL-Runtime-f3bd45?style=flat-square&labelColor=050505">
  <img alt="RTL Safe" src="https://img.shields.io/badge/RTL-Safe-f3bd45?style=flat-square&labelColor=050505">
  <img alt="Logo Preserved" src="https://img.shields.io/badge/Logo-Preserved-f3bd45?style=flat-square&labelColor=050505">
</p>

- **ECRL Runtime**: بازیابی ECRL به runtime وصل شده است.
- **RTL Safe**: تست راست‌چین بودن مستندات فارسی (`tests/docs/rtl_test.go`).
- **Logo Preserved**: لوگوی اصلی BAFT مستقل از تم BCC باقی می‌ماند و تغییر نمی‌کند ([سیستم بصری BCC](28-bcc-visual-system.md)).

> **وضعیت پروژه:** BAFT هنوز به‌عنوان نرم‌افزار production-ready اعلام نشده است. هر ادعای عملکرد، امنیت یا بازیابی فقط در محدوده‌ای معتبر است که کد، تست و شواهد CI آن را پشتیبانی کنند.

## BAFT از نگاه فنی

BAFT یک **transport fabric کنترل‌شده** برای جابه‌جایی جریان‌های TCP میان نودهای تحت مدیریت همان اپراتور است.

در معماری پایه:

- نود **IR** اتصال Carrier را آغاز می‌کند.
- نود **EX** اتصال را می‌پذیرد.
- دادهٔ برنامه می‌تواند در هر دو جهت منتقل شود.
- peer مقصد دلخواه تعیین نمی‌کند؛ فقط یک **Route ID** مجاز درخواست می‌کند.
- مقصد واقعی از پیکربندی محلی و allowlist همان نود انتخاب می‌شود.

این طراحی عمداً سطح اختیار peer را محدود می‌کند و کنترل مسیر، هویت، Release، Agent و تغییرات عملیاتی را در اختیار اپراتور نگه می‌دارد.

## قابلیت‌ها و مرزها (از README قبلی)

**قابلیت‌های موجود:**

- Release امضاشده با manifest، certificate، revocation list و SHA-256
- Installer با verify اجباری و anti-downgrade
- BCC Control Plane
- Secure Agent با jobهای امضاشده و allowlist
- SSH bootstrap با host-key pinning
- Native Tunnel Builder
- rollback دوطرفه در failure
- Launch-1 end-to-end
- ECRL same-process recovery با bounded replay

**هنوز خارج از محدودهٔ تأییدشده:**

- process-restart resume
- machine-reboot resume
- durable ECRL snapshots
- production readiness
- تضمین سرعت یا اتصال در شبکهٔ عمومی
- ادعای universal undetectability

## معماری

### Control Plane

BCC مسئول مدیریت fleet، صدور jobهای امضاشده، ثبت state، audit و کنترل تغییرات است. Agent روی هر نود فقط actionهای ازپیش‌تعریف‌شده را اجرا می‌کند و مسیر shell آزاد ندارد.

### Data Plane

<div dir="ltr" align="left">

```text
Local Application
       |
       v
IR Route Listener
       |
       v
    BAFT IR
       ||
       || authenticated carrier
       || bounded flow control
       || recovery-aware session
       v
    BAFT EX
       |
       v
Fixed Authorized Route
       |
       v
 Target Service
```

</div>

### اصل Route ثابت

peer اجازه ندارد host/port دلخواه را داخل فریم تعیین کند. IR یک Route ID می‌فرستد و EX مقصد واقعی را از پیکربندی محلی و allowlist خودش انتخاب می‌کند.

## مدل اعتماد

BAFT چند مرز اعتماد را از هم جدا می‌کند:

- هویت Node
- peer allowlist
- Route allowlist
- release trust
- control-plane authorization
- local destination policy

اعتماد به یک CA به‌تنهایی به‌معنای اجازهٔ دسترسی به همه Routeها نیست.

## زنجیرهٔ انتشار امن

Installer فقط artifactهایی را می‌پذیرد که زنجیرهٔ اعتماد Release را پاس کنند:

<div dir="ltr" align="left">

```text
Pinned Root
    |
    v
Release Key Certificate
    |
    +-- not revoked
    +-- valid time window
    v
Signed Manifest
    |
    +-- version
    +-- exact commit
    +-- artifact size
    +-- SHA-256
    v
Downloaded Binaries
```

</div>

اگر verify شکست بخورد، نصب متوقف می‌شود.

## Native Tunnel Builder

ساخت تونل به‌صورت تراکنشی انجام می‌شود:

<div dir="ltr" align="left">

```text
PLAN
  |
  v
VALIDATE
  |
  v
PREPARE EX + PREPARE IR
  |
  v
COMMIT
  |
  v
HEALTH CHECK
  |\
  | \__ failure --> ROLLBACK BOTH SIDES
  |
  +---- success --> ACTIVE
```

</div>

هدف این است که failure میانی، یک سمت را در وضعیت نیمه‌فعال رها نکند.

## BCC — مرکز کنترل BAFT

قابلیت‌های اصلی:

- دسترسی وب با session و CSRF protection
- rate limit و lock موقت بعد از login failure
- SQLite state
- audit log
- monitoring
- finance ledger
- node revocation
- token rotation
- SSH bootstrap
- Tunnel Builder

هویت بصری BCC در مستند [سیستم بصری BCC](28-bcc-visual-system.md) ثبت شده است؛ **لوگوی اصلی BAFT مستقل از تم BCC باقی می‌ماند و تغییر نمی‌کند.**

## نصب مرحله‌به‌مرحله با پارامتر

### دریافت installer

<div dir="ltr" align="left">

```bash
curl -fsSLO https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh
less install.sh
chmod +x install.sh
```

</div>

### نصب EX

<div dir="ltr" align="left">

```bash
sudo bash install.sh \
  --role ex \
  --public-address EX_HOST_OR_IP \
  --version v0.1.0
```

</div>

EX یک pairing code با prefix زیر تولید می‌کند:

<div dir="ltr" align="left">

```text
BAFTPAIR1:...
```

</div>

### نصب IR

<div dir="ltr" align="left">

```bash
sudo bash install.sh \
  --role ir \
  --pairing-code 'BAFTPAIR1:...' \
  --version v0.1.0
```

</div>

راهنمای کامل: [اجرای IR و EX](06-running-ir-ex.md)

## Recovery

در محدودهٔ فعلی، BAFT روی recovery در همان process/session تمرکز دارد.

**تأییدشده:**

- Carrier replacement
- epoch fencing
- bounded replay
- حفظ Flow فعال
- multi-flow continuity
- FIN / FIN_ACK recovery
- commit validation پیش از authority change

**تأییدنشده:**

- resume بعد از restart پردازه
- resume بعد از reboot ماشین
- snapshot پایدار session

## روش نوآوری

تصمیم‌های مهم BAFT در دو مسیر جدا سنجیده می‌شوند: **Baseline** (کوچک‌ترین سازوکار امن، محدود و تست‌پذیر با مسیر rollback) و **Research** (سازوکار مفهوماً متفاوت برای یک شکاف مستند، با آزمایش ابطال‌پذیر و معیار شکست صریح). قاعدهٔ 10x/100x فشار طراحی است، نه ادعای عددی. نمونه‌ها: [TWRL](13-stage-c-twrl.md) و [PADL](14-stage-c-padl.md). جزئیات: [روش‌شناسی نوآوری](12-innovation-method.md).

## CI و شواهد مهندسی

گیت‌های اصلی پروژه شامل موارد زیر هستند:

- unit / integration tests
- race detector
- `go vet`
- protocol fuzz smoke
- installer E2E
- signed-release installer E2E
- Agent enrollment E2E
- SSH bootstrap روی sshd واقعی
- Launch-1 end-to-end
- rollback tests
- multi-flow / slow-receiver soak
- recovery-specific soak

عبور از این گیت‌ها اثبات correctness در محدودهٔ تست‌شده است و به‌تنهایی به‌معنای benchmark عمومی یا production readiness نیست.

## اسناد وضعیت

- [STATUS.md](../../STATUS.md)
- [PLAN.md](../../PLAN.md)
- [TEST-RESULTS.md](../../TEST-RESULTS.md)
- [KNOWN-LIMITATIONS.md](../../KNOWN-LIMITATIONS.md)
- [BLOCKERS.md](../../BLOCKERS.md)

## ساختار مخزن

<div dir="ltr" align="left">

```text
cmd/                 command-line binaries
internal/            runtime, protocol, BCC, agent, recovery
configs/             example configurations
tests/               integration, correctness and E2E tests
scripts/             release/build/test automation
release/             release trust material
docs/fa/             Persian documentation
docs/en/             English documentation
reports/             engineering and research reports
.github/workflows/   CI, soak, release and security workflows
```

</div>

## توسعه از سورس

<div dir="ltr" align="left">

```bash
git clone https://github.com/zarkmakerburg/baft.git
cd baft

go build ./cmd/baft
go test ./...
go test -race ./...
go vet ./...
```

</div>

نسخهٔ مرجع Go در حال حاضر **1.27.1** است.

## چیزی که BAFT نیست

BAFT در وضعیت فعلی:

- VPN عمومی چندکاربره نیست.
- reverse proxy مقصد-دلخواه نیست.
- جایگزین ACL یا PKI سرویس مقصد نیست.
- کیفیت شبکه عمومی را تضمین نمی‌کند.
- universal connectivity یا undetectability را ادعا نمی‌کند.
- process-restart recovery را ارائه نمی‌کند.
- production-ready اعلام نشده است.

</div>
