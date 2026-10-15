# Aurora embedded-shell composition boundary

HQ Stage181 requires incremental integration with both existing BCC surfaces.
The standalone preview previously owned document language/direction, the URL
hash and global CSS selectors. Importing it directly into BCC would change the
host's appearance or routing. This prerequisite isolates those ownership rules;
it does not activate an Aurora route, feature flag or real provider.

The default `mountAurora(root, demo)` keeps the existing standalone behavior.
An explicitly selected composition mode supports a future opt-in DEMO island:

```ts
const destroy = mountAurora(root, createMockProvider(), { host: 'embedded' });
```

Both modes still refuse real providers and mislabeled real responses. The
embedded mode always starts at its own overview. Language, direction, theme
and styling attributes belong to the supplied root. Local navigation prevents
the hash change and focuses its local content; unrelated host route changes
do not repaint Aurora or take focus. Content uses a labelled section rather
than a second main landmark. Teardown aborts pending work, removes listeners,
empties its owned root and restores the previous root attributes. It does not
restore or change document appearance in embedded mode. The caller must give
Aurora exclusive ownership of that root and use only one shell per document;
multiple embedded shells and third-party writes into the owned root are outside
this contract.

Every selector in `preview/shell.css`, including media, focus and reduced-motion
rules, is restricted to `[data-aurora-root]`. The font face uses a separate
`AuroraVazirmatn` family; its original local source and font-display are retained.
All 94 original declaration blocks remain unchanged except that family name.
The two added rules bound positioning/stacking and remove standalone viewport
height in embedded mode. The standalone document margin/background reset moves
to `preview/page.css`; **never import page.css into the BCC host**. Existing
tokens are still preview tokens, not an approved replacement for BCC's theme.
Embedding/serving and font-path validation remain future integration work.

Eight added DOM/CSS cases exercise all four preference combinations, local
navigation/skip and focus, ignored host routing, pending completion, abort/late
data, root attribute restoration, remount ownership, rejected runtime modes,
recursive selector scoping and host computed-style sentinels. Negative inputs
append unscoped top-level and media rules and must fail. The original seven
isolation regressions fail on unchanged PR208 compiled source before this patch;
the existing standalone suite stays required. Strict type tests also forbid
real providers in embedded mode and unknown host modes. No dependency, lockfile
or byte budget changes are needed.

Implementation verification caught an intermediate CSSOM serialization dropping
the font source/display and a vendor interaction declaration. That unpublished
candidate was replaced with a declaration-preserving edit and source guards.
A verification helper initially compared an already-renamed font twice; its
normalization was corrected and all 94 original declaration blocks verified.
Passing DOM computed styles does not prove fonts, layout or browser appearance.
All failed baseline/implementation attempts remain in the linked HQ evidence.

The local environment has no browser executable. One Playwright browser install
attempt failed on truncated/empty ZIP extraction and a stale lock update; no
additional retry or rendered acceptance is claimed. The FA/EN × dark/light ×
320/390/768/1440 layout, keyboard traversal, accessibility and performance gates
remain open, as do native owner-deferred lab tests, real read mounting, reviewed
auth/session/CSRF integration and a reversible product feature flag. No BCC
handler/route/auth/asset embedding, default/classic product control, operational
write or live data path changes in this slice. Stop mounting and call `destroy`
to undo an experimental island; no backend or persisted state migration exists.

Exact source SHA, local/CI results and all limitations are recorded in #67.
The product epic #199 remains open. No main merge, release or deploy is granted.
