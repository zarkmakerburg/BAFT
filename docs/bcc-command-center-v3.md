# BAFT Command Center — UI V3 frozen specification

Status: DESIGN FROZEN; implementation in isolated branch `ui/bcc-command-center-v3`.

## Brand and layout
- Use the user-supplied BAFT gold-and-black woven B emblem as the canonical logo. Never replace it with a generated substitute. The image asset must be imported from the owner's approved source before production release.
- Premium, restrained Swiss-style hierarchy; spacious layout, no noisy decorative cards.
- Persian (fa-IR) and English (en-US), proper RTL/LTR direction and localized labels/numerals where appropriate.
- Dark: graphite/near-black, subdued metallic gold, cool neutral text. Light: warm ivory, soft sand, muted gold, low-contrast borders; no neon or harsh saturation.
- Persist theme and language preferences client-side; default to OS theme if no saved preference. Honor reduced motion.

## Interactive globe
- Actual WGS84 coordinates for every server node. No country flag floating at an arbitrary map position.
- Projection to a real 3D globe (WebGL) with camera orbit, zoom and deterministic fallback. For a known country but unknown city, show a country-level approximate marker explicitly labeled as approximate, never claim exact host location.
- Connections join real source and destination coordinates as elevated geodesic arcs.
- Healthy: subtle moving gold particle halo indicating direction and relative flow; degraded: amber; disconnected: red with restrained pulsing warning; unknown: neutral gray (never claim healthy).
- Hover/focus: highlight arc, smoothly frame endpoints, show connection-specific live metrics: source/destination, tunnel ID, transport, health, current up/down throughput, RTT/jitter/loss, last handshake, last error, last update timestamp. Never substitute country facts.
- Mouse leave restores view; keyboard focus/touch selection supported; zoom must not jerk or fight manual camera control.
- No fake realtime values. Stale/absent telemetry displays Unknown / last observed with timestamp.

## Product navigation
Overview, Servers, Tunnels, Monitoring, Security, SSL, Backups & Restore, Updates, Settings. Preserve current BCC authorization, APIs and audit.
- Desktop sidebar and mobile responsive navigation.
- Avoid overloaded overview: globe primary, compact KPIs, selected-link inspector and recent incidents; detailed tables and graphs in their own pages.
- Danger operations require explicit confirmation and ownership checks; never trigger on hover.

## Engineering gates
1. Inventory existing BCC API/data fields; classify supported vs missing live link telemetry.
2. Implement visual shell, theme/i18n and accessible components without changing existing data-plane behavior.
3. Build globe against explicit mock fixtures then connect to real BCC endpoints; do not invent live telemetry.
4. Tests: Persian/English RTL, dark/light, keyboard/touch, disconnected/stale state, coordinates, multiple nodes in one country, responsive layout, reduced motion.
5. Independent review and screenshots before any merge or production deployment.
6. Do not touch PNL↔AS or Arno↔Main live services. No release or merge without review.

## Implementation (October 2026)
The Command Center now follows the owner's three reference images (logo, landing, dashboard in dark and light).

- Routes under the secret path: `/` is the landing page, or the dashboard once signed in; `login` is the sign-in form; `classic/` keeps the previous single-page dashboard with its own sign-in. Old `v3/` bookmarks redirect to `/`.
- Files: `web/bcc-app.html` (template, served only through `webui.Page`), `bcc-app.css`, `bcc-app.js` (shell, pages, fa/en strings), `bcc-map.js` (2D world map with tunnel arcs), and `bcc-operations-v3.js`, which embeds the classic dashboard's tools into the new pages.
- Data: `GET api/overview?window=24h|7d` gives counts, latency, traffic rates from exit nodes only and a 96-bucket series; values without telemetry are `null` and shown as "—". `POST api/nodes/location` (admin, audited) sets a server's country so it appears on the map; the list lives in `internal/bcc/countries.go` and must match `COUNTRIES` in `bcc-app.js`.
- The sidebar stays on the left in both languages; the content follows RTL in Persian. Theme and language are saved in the browser and applied before the first paint.
- Opened as a static file the page shows a demo labelled DEMO; served by BCC it never shows invented numbers.
- Checks: `node --test web/tests/*.test.cjs` (string parity, country list, escaping, embedded files) and `web/qa/bcc-app-chrome.cjs` for screenshots of every page in both languages and themes.
- Maps: the landing globe and the dashboard map use bundled NASA imagery (Blue Marble day, Black Marble night, ocean mask and relief, via the three-globe example set) rendered locally; nothing is fetched from outside BCC. The globe turns slowly and can be dragged.
- Dashboard extras, all from BCC's own records: a details panel when a server or tunnel arc is clicked (status, latency, uptime, current traffic, tunnels, certificate expiry where BCC issued the certificate, and a Route Doctor run); recent events from alerts and the audit log; system health (last backup, next certificate expiry, rotations, agents not connected, BCC version); this month's cost, revenue, profit and cost per GB of exit traffic; quick actions; and a data freshness note that turns amber when updates stop.
