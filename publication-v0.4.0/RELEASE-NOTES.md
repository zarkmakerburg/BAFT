# BAFT v0.4.0

Source: [`9c32ba369fe2be2c549f2878d1de5dbf092c8f64`](https://github.com/zarkmakerburg/BAFT/commit/9c32ba369fe2be2c549f2878d1de5dbf092c8f64) on `main`, tag `v0.4.0`.

## فارسی

این نسخه، نرم‌افزار پژوهشی با پذیرش در LAB بازتولیدپذیر است. شواهد، پیکربندی‌ها و خطاهای آزموده‌شده را پوشش می‌دهند؛ تضمین دسترس‌پذیری WAN یا تأیید آمادگی production نیستند.

تغییرهای اصلی:

- مسیر معمول BCC اکنون Plan بازبینی‌شده، job امضاشدهٔ Agent ثبت‌شده، public ingress با مالکیت دقیق و nftables، Receipt احرازشده و کارت Verified ingress را در یک جریان پذیرش می‌آزماید. اجرای اثر همچنان به capability، policy و authority معتبر نیاز دارد؛ خارج از profile پذیرفته‌شده، عملیات held/refused باقی می‌ماند.
- tenantهای Noise احرازشده می‌توانند listener و process مالک در EX را شریک شوند. join بازبینی‌شده، instanceهای مستقل IR و حذف امضاشده دارند. restart tenant، مالک را restart نمی‌کند؛ رکوردهای مالک و siblingها حفظ می‌شوند و جایگزینی/rotation مالک دارای tenant وابسته رد می‌شود.
- cleanup پس از restart، بررسی authority، socket inventory تازه، readback دقیق rule و حذف آن را زیر یک guard نگه می‌دارد. مالکیت نامعلوم مجوز cleanup ایجاد نمی‌کند.
- آزمون ترکیبی، انتقال payload به دو مقصد متمایز را همراه join، restart و retire tenant، خطای پاسخ پس از Add واقعی nft، rollback و حفظ اتصال و منابع owner می‌سنجد. manifest قدیمی ۲۸موردی تغییر نکرده است؛ گیت مشترک و recovery نیز در پذیرش انتشار الزامی‌اند.
- fixtureهای مرورگر logout و startup آزمون uninstall اصلاح شده‌اند؛ guard محصول و assertionهای حفظ سرویس ضعیف نشده‌اند.

محدودیت‌های مؤثر در انتخاب نسخه:

- **مرگ یا restart کل Agent، public listenerها و اتصال‌های آن‌ها را از بین می‌برد.** adopt این listenerها پس از راه‌اندازی مجدد پیاده‌سازی نشده است. موفقیت restart یک tenant یا بازسازی مؤلفه‌های durable به معنی حفظ اتصال پس از مرگ کل Agent نیست.
- پس از Apply شکست‌خورده، reconciliation می‌تواند با شواهد کامل و tombstone نسل، claim محلی را آزاد کند؛ اما BCC همچنان Receipt شکست و رزرو desired همان پورت را نگه می‌دارد. **استفادهٔ مجدد خودکار BCC از این پورت ارائه نشده است.** رکورد مالکیت یا رزرو را برای دورزدن این شرط دستی پاک نکنید.
- کارت Verified ingress سلامت کامل برنامهٔ مقصد یا 3X-UI را تأیید نمی‌کند. payloadهای مستقل LAB، رسیدن داده به مقصد را ثابت کرده‌اند. مدیریت subscription، کاربران و UUID با 3X-UI است؛ اختلاف client port و destination port به تنظیم خارجی subscription نیاز دارد.
- shared Noise peerها با multiplex-v1 داخل یک session یکسان نیستند. reload زندهٔ mTLS-only، adopt خودکار tenantهای CLI، UDP و engineهای خارجی غیرفعال، قابلیت‌های این نسخه نیستند. [Issue #269](https://github.com/zarkmakerburg/BAFT/issues/269) برای الزامات کامل خود باز می‌ماند.
- Update Center بستهٔ امضاشده را با واردکردن operator می‌پذیرد و خودکار apply نمی‌کند. `update_baft` فقط `baft` و `baft-pair` را عوض می‌کند؛ برای سایر نقش‌ها از مسیر installer تأییدشده استفاده کنید. rollback از schemaهای جدید SQLite/backup به snapshot تأییدشدهٔ پیش از upgrade نیاز دارد.

۱۳ workflow لازم و ۲۸ job پذیرش روی همین SHA موفق‌اند. public ingress برابر ۲۸/۲۸، recovery برابر ۱۳/۱۳ و shared operational برابر ۲/۲ هستند، همگی با صفر SKIP. هر ۱۰ artifact در دو build همین SHA یکسان‌اند و بررسی provenance از clean archive موفق بوده است. SKIPهای helper/privileged در مجموعهٔ عمومی، جایگزین این گیت‌های zero-SKIP نیستند.

LAB مشترک از kernel واقعی و `node.Runtime` واقعی با نظارت goroutine استفاده می‌کند؛ این آزمون، اجرای systemd روی میزبان فیزیکی، مرگ process کل Agent یا آزمون WAN مستقل نیست. حفظ table خارجی TCP/2053 فقط حفظ همان table آزموده‌شده را ثابت می‌کند. جزئیات در [قرارداد پذیرش مشترک](https://github.com/zarkmakerburg/BAFT/blob/9c32ba369fe2be2c549f2878d1de5dbf092c8f64/docs/release/v040-shared-ingress-acceptance.md) و [سیاست پذیرش LAB](https://github.com/zarkmakerburg/BAFT/blob/9c32ba369fe2be2c549f2878d1de5dbf092c8f64/docs/release/v040-lab-acceptance-policy.md) آمده است.

برای نصب، از assetهای انتشار و مسیر installer با بررسی manifest، certificate، checksumها و revocation در برابر ریشهٔ ثابت BAFT استفاده کنید. artifact با نام `unsigned-release-candidate` برای نصب به‌عنوان انتشار امضاشده نیست. انتشار این نسخه مجوز تغییر روی میزبان‌های production ایجاد نمی‌کند.

## English

v0.4.0 is a research release qualified by reproducible LAB acceptance on the exact source commit above. Its evidence covers the tested configurations and failure modes; it does not establish WAN availability or production readiness.

- The normal BCC path joins a reviewed Plan, an enrolled Agent's signed job, ownership-checked public ingress and nftables, an authenticated Receipt, and the matching Verified ingress card. Effects still require admitted capabilities, policy and authority.
- Authenticated Noise tenants can share the owner's EX listener and process, with reviewed joins, independent IR instances and signed retirement. Tenant Runtime restart preserves the owner. Owner and sibling records are retained, and owner replacement/rotation is refused while dependent tenants remain.
- Restart cleanup holds authority checks, fresh socket inventory, exact rule readback and removal under one guard. Unknown ownership grants no cleanup authority.
- Combined acceptance sends distinct payloads to real targets while exercising tenant join/restart/retirement, an error returned after a real nft Add, exact rollback, and owner connection/resource preservation. The existing 28-case ingress manifest is unchanged; shared operational ingress and recovery gates are also required.

Availability and upgrade limits:

- **Whole-Agent death or restart loses its public listeners and their connections.** Restart adoption is not implemented. Tenant Runtime restart and reconstruction of durable components do not prove whole-process continuity.
- After failed Apply, complete local evidence can retire a local claim with a generation tombstone. BCC still retains the failed receipt and desired port reservation. **Automatic BCC reuse of that failed port is unavailable.** Do not clear ownership or reservation records to bypass admission.
- Verified ingress is not full application or 3X-UI health verification. Independent marker payloads establish target delivery in the LAB. Subscription, user and UUID management remain external to BAFT.
- Shared Noise peers are not multiplex-v1 within one session. mTLS-only live reload, automatic adoption of CLI tenants, UDP and disabled external engines are outside this release. [#269](https://github.com/zarkmakerburg/BAFT/issues/269) remains open.
- Update Center requires an operator-imported signed bundle and does not apply it automatically. `update_baft` replaces only `baft` and `baft-pair`; other roles use the verified installer path. Rollback across newer SQLite/backup schemas requires a verified pre-upgrade snapshot.

All 13 required workflow paths and 28 acceptance jobs succeeded on this SHA. Operational ingress passed 28/28 cases, recovery 13/13 and shared operational ingress 2/2, with zero SKIPs in those gates. All ten assets matched two clean builds of this source; clean-archive provenance checks passed. General helper/privileged skips do not replace the zero-SKIP LAB gates.

The combined LAB uses a real kernel and production `node.Runtime` goroutines. It does not exercise physical-host systemd supervision or whole-Agent process death. A preserved foreign TCP/2053 table proves that table's preservation, not an arbitrary production service. See the [combined acceptance contract](https://github.com/zarkmakerburg/BAFT/blob/9c32ba369fe2be2c549f2878d1de5dbf092c8f64/docs/release/v040-shared-ingress-acceptance.md) for the exact boundaries.

Use release assets through the installer verification path, including manifest, certificate, checksums and current revocations against the pinned BAFT root. Do not install the `unsigned-release-candidate` as a signed release. Publication authorizes no production-host changes.

## Evidence

The source gate links below belong to the exact `main` SHA above. The tag workflow produces the unsigned candidate; it is not evidence of offline signing or published-asset acceptance.

| Gate | Evidence |
| --- | --- |
| CI, installation and release dry-run | [ci.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008208691) |
| Static analysis | [static-analysis.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41044986750) |
| Noise/R3 | [r2-noise.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008248316) |
| Operational ingress, 28/28 | [operational-ingress-v1.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008208796) |
| Operational recovery, 13/13 | [operational-recovery.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008208504) |
| Combined shared ingress, 2/2 | [shared-operational-ingress.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008208454) |
| P0 ingress LAB | [p0-ingress-lab.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008430432) |
| Gamma two-node LAB | [gamma-two-node-lab.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008251038) |
| v0.3.7 upgrade/rollback LAB | [gamma-v037-upgrade-lab.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008253886) |
| v0.3.7 tunnel upgrade LAB | [gamma-v037-tunnel-upgrade-lab.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008256944) |
| BCC UI/unit/browser | [bcc-v3-ui-tests.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008433301) |
| Vulnerability analysis | [vulncheck.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008259495) |
| Ingress pin checks | [delta-ingress-pin.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41008261984) |
| Official unsigned candidate | [release.yml](https://github.com/zarkmakerburg/BAFT/actions/runs/41046337283) |

Release decisions and verification evidence are tracked in [HQ #67](https://github.com/zarkmakerburg/BAFT/issues/67).

Offline signing verified: the Mac-signed manifest names this exact source commit, and all ten assets verify under release key `41f8a2dccb57be62f7a917fb2cc929f2`, the pinned BAFT root and current revocations. The offline bundle was built twice with byte-identical output.

Offline bundle SHA-256: `2c22268b7d1750dbf0b8a9bf4eea4158e89c14207dbf626ad7d85de23428102d`.
