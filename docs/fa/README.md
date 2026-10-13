<div dir="rtl" align="right" lang="fa">

# راهنمای مستندات BAFT

[English documentation](../en/README.md) | [README اصلی فارسی](../../README.fa.md) | [راهنمای کاربر](guide/README.md)

## از این‌جا شروع کنید: راهنمای کاربر

اگر می‌خواهید بافت را نصب کنید و با x-ui به کار ببرید، اول **[راهنمای کاربر](guide/README.md)** را بخوانید: [نصب](guide/01-install.md)، [استفاده با x-ui](guide/02-use-with-x-ui.md)، [پنل](guide/03-panel.md)، [شبکه و BBR](guide/04-network-and-bbr.md)، [به‌روزرسانی، پشتیبان و حذف](guide/05-update-backup-uninstall.md)، [عیب‌یابی](guide/06-troubleshooting.md)، [پرسش‌های رایج](guide/07-faq.md).

## مستندات اپراتور

جزئیات رفتار نصب‌کننده، پنل و ابزارها:

- [06 — ساخت و اجرای IR/EX](06-running-ir-ex.md)
- [23 — نسخه‌های امضاشده و بستهٔ آفلاین](23-p1a-signed-releases.md)
- [24 — دسترسی وب به BCC و state در SQLite](24-p1c-bcc-access.md)
- [25 — jobهای امن agent](25-p1d-agent.md)
- [26 — فهرست سرورها و راه‌اندازی با SSH](26-p1d-ssh-bootstrap.md)
- [27 — سازندهٔ بومی تونل](27-p1e-tunnel-builder.md)
- [29 — پشتیبان BCC و پیش‌نمایش restore](29-bcc-backup-restore-preview.md)
- [30 — سلامت چندلایهٔ BCC](30-bcc-layered-health.md)
- [32 — منوی `baft`](32-baft-menu.md)
- [33 — نصب‌کنندهٔ قابل اجرای مجدد (نصب پنل، SSL، `baft-panel` و `baft-net`)](33-rerunnable-installer.md)
- [34 — حذف امن](34-safe-uninstall.md)
- [35 — چرخش تراکنشی گواهی](35-cert-rotation.md)
- [37 — اجرای بافت پشت Cloudflare (انگلیسی)](../en/37-cloudflare-websocket.md)

## مستندات فنی

این بخش برای خواننده‌ای نوشته شده که ممکن است هیچ شناخت قبلی از BAFT نداشته باشد. ترتیب پیشنهادی مطالعه:

1. [01 — معرفی، هدف و مرز پروژه](01-overview.md)
2. [02 — معماری، نقش‌ها و جریان داده](02-architecture.md)
3. [03 — مدل امنیت، هویت و مرز اعتماد](03-security-model.md)
4. [04 — پروتکل BAFT/1 و state machine](04-protocol-baft1.md)
5. [05 — پیکربندی و Routeها](05-configuration.md)
6. [06 — ساخت و اجرای IR/EX](06-running-ir-ex.md)
7. [07 — کنترل حافظه، backpressure و زمان‌بندی](07-resource-control.md)
8. [08 — آزمون‌ها، CI و معیار پذیرش](08-testing-and-ci.md)
9. [09 — نقشه راه و مراحل A تا H](09-roadmap.md)
10. [10 — ساختار کد و مسئولیت ماژول‌ها](10-repository-layout.md)
11. [11 — فرهنگ واژگان](11-glossary.md)
12. [12 — روش‌شناسی نوآوری](12-innovation-method.md)
13. [13 — TWRL در Stage C](13-stage-c-twrl.md)

## سلسله‌مراتب منابع

وقتی میان اسناد اختلاف وجود دارد، این ترتیب را رعایت کنید:

1. Blueprint محتوایی 1.4 و Implementation Master Prompt 1.2؛
2. ADRهای پذیرفته‌شده برای تصمیم‌های پیاده‌سازی؛
3. schema، wire tests و golden vectors برای قرارداد قابل‌اجرا؛
4. STATUS و TEST-RESULTS برای آنچه واقعاً ساخته و آزمایش شده؛
5. این راهنماها برای توضیح و آموزش.

هر قابلیتی که در Blueprint آمده ولی در STATUS به‌عنوان پیاده‌شده ثبت نشده، **برنامه یا قرارداد آینده** است و نباید قابلیت موجود فرض شود.

14. [14 — PADL در Stage C](14-stage-c-padl.md)

15. [15 — Conservation Telemetry و چند Shard](15-conservation-telemetry.md)

16. [16 — Metrics حفاظتی و conservation](16-conservation-metrics.md)

17. [17 — گیت‌های صحت single-flight](17-correctness-gates.md)

18. [18 — گیت Soak برای Stage C](18-stage-c-soak.md)

19. [19 — قرنطینه محدود وضعیت پایانی Flow](19-terminal-quarantine.md)

20. [20 — یادداشت قدیمی ECRL؛ superseded](20-stage-d-ecrl.md)

15D. [ECRL — prior-art، مدل تهدید و invariant رسمی](15-stage-d-ecrl.md)

16R. [R1/R2 — Reachability و SecurityInternal](16-reachability-innovation.md)

21. [21 — R3.1 Morphing / Stealth Pro (انگلیسی)](../en/21-stealth-pro.md)
22. [22 — نقشه‌راه Launch-1](22-launch-1-roadmap.md)
23. [23 — P1-A: آرتیفکت‌های امضاشدهٔ Release](23-p1a-signed-releases.md)
24. [24 — P1-C: دسترسی وب به BCC و state در SQLite](24-p1c-bcc-access.md)
25. [25 — P1-D: jobهای امن agent](25-p1d-agent.md)
26. [26 — P1-D: فهرست سرورها و راه‌اندازی با SSH](26-p1d-ssh-bootstrap.md)
27. [27 — P1-E: سازندهٔ بومی تونل](27-p1e-tunnel-builder.md)


28. [28 — سیستم بصری BCC](28-bcc-visual-system.md)
29. [29 — پشتیبان BCC و پیش‌نمایش restore](29-bcc-backup-restore-preview.md)
30. [30 — سلامت چندلایهٔ BCC با hysteresis](30-bcc-layered-health.md)
31. [31 — کشف تونل‌های موجود (فقط گزارش)](31-tunnel-discovery.md)
32. [32 — منوی `baft` و هدر ترمینال](32-baft-menu.md)
33. [33 — نصب‌کننده‌ی قابل اجرای مجدد](33-rerunnable-installer.md)
34. [34 — حذف امن](34-safe-uninstall.md)
35. [35 — چرخش تراکنشی گواهی](35-cert-rotation.md)
36. [36 — soak تاب‌آوری در برابر مهاجم (انگلیسی)](../en/36-adversarial-resilience.md)
37. [37 — اجرای بافت پشت Cloudflare (انگلیسی)](../en/37-cloudflare-websocket.md)
40. [40 — یادداشت‌های مهندسی (منتقل‌شده از صفحهٔ اول)](40-engineering-notes.md)

</div>
