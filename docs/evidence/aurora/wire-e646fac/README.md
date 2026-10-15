# Shared BCC ↔ Aurora wire contract evidence (pending Go CI)

Source PR205: e646faceefea39cb09a45ad0cbf2c019d47de863
Source tree: dbef5d0d7720cf605b4ece1effcea862608141dd
Source parent / current PR204: de446b5df8f045c69ca388b3c500515143f7812a
Claim: #67/comment6192411548

This evidence-only branch does not change the source PR or accept the full roadmap.
It preserves the exact fetched-source local log and complete decoded UI job116433804135.
Local30 Aurora +12 DOM +14 classic tests PASS/0 FAIL/0 SKIP; strict/negative TypeScript,
generated8-module byte equality and size guards PASS. Runtime bytes/dependencies unchanged.

GitHub UI checkout actually used synthetic PR merge b22ed6484c7616d19c85c1818ba9b288c88d90ee.
Its fetched Git tree is dbef5d0d7720cf605b4ece1effcea862608141dd, identical to the source;
parents are de446b5 then e646fac. Local tests ran the head itself. No main merge occurred.

At this snapshot UI38738495407 and static38738495204 are SUCCESS; main38738495678
is IN_PROGRESS. Go producer compile/execution is PENDING, not inferred from Node.
No local Go/gofmt and no CI rerun. Later qualification must append, not erase this snapshot.

Fixtures compare selected consumed fields of actual in-process handlers and the same
Node-injected responses; they are neither full response captures nor an API version.
Synthetic temp state, no listening socket/live node. Legacy test-admin authentication
does not qualify cookie/CSRF, persistence or native access.

Browser/preview/screenshots/mobile320/rendered accessibility/performance OPEN/NOT RUN.
Native IR↔EX/candidate owner-DEFERRED/OPEN. No operational/merge/release acceptance.
See attempts.md and source docs/aurora/read-wire-contract.md for exact scope/limitations.
