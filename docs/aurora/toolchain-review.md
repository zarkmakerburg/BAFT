# A1 compiler and dependency admission — 2026-10-15

Decision: one exact development dependency, `typescript@6.0.3`; no runtime
framework, graph, chart, CSS, polyfill, bundler, Node service or browser package.
Alternatives: JSDoc alone cannot meet the owner's strict-TypeScript contract;
React/bundler adoption would expand this foundation before a shell is selected.

Primary references inspected:

- [Official 6.0 documentation](https://www.typescriptlang.org/docs/handbook/release-notes/typescript-6-0.html).
- [Upstream v6.0.3 release](https://github.com/microsoft/TypeScript/releases/tag/v6.0.3), published 2026-04-16; verified public release, not a draft.
- [Exact registry package](https://registry.npmjs.org/typescript/6.0.3): Apache-2.0, Node >=14.17, no package dependencies. The lockfile records the exact tarball and SHA-512. Installed package metadata, LICENSE.txt and ThirdPartyNoticeText.txt were inspected.

6.0.3 is the deliberately pinned, tested A1 compiler, not a claim about the newest
upstream version or long-term support. Upstream is Microsoft's TypeScript
repository. Compiler upgrades require a lockfile review and generated-output
diff; the chosen compiler is not part of shipped browser modules. Preserve
upstream license/notices if distributing the build tool itself.

Installation and CI use `npm ci --ignore-scripts --no-audit --no-fund`. No package
lifecycle hooks are executed. A separate `npm audit --json` against this locked
graph on 2026-10-15 reported zero known advisories; its JSON is retained with A1
evidence. That is a point-in-time registry check, not a universal security verdict.
The existing workflow actions remain SHA-pinned with read-only contents access.

Local tools: Node 24.19.0/npm 11.9.0; CI uses the existing Node 22 action.
TypeScript target/module are ES2022, with strict null checks, unknown catch values,
unchecked-index/optional-property checks, isolated modules and no emit on error.
DOM declarations describe browser fetch/streams; Node tests use its native APIs.
There is no React/SSR runtime to certify. Browser rendering remains an A2 gate.

Current emitted foundation: four modules, 15,508 raw bytes and 5,106 bytes summed
per-file gzip(level9). The temporary rebuild compares every output byte and
detects orphans. Initial module-only guards: <=32,768 raw and <=12,288 summed
gzip bytes. These are regression guards after measuring A1, not Android runtime,
rendering, network-transfer or user-capacity acceptance. Actual current BCC
embedding/transfer delta is zero because these files are not loaded by the UI.

No Go dependency, backend API, session/CSRF behavior, default route, existing
feature or installer was changed. Native two-node/candidate testing remains
owner-deferred. Graph/chart/motion dependencies remain unselected and require
their own admission records.
