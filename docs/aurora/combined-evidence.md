# Recoverable Aurora × Core source evidence

HQ Stage181 requires reproducible evidence of the combined tested source. The
preflight checks out the exact PR source, checks the live Aurora pin, then performs
the existing three-way merge in a disposable workspace. It never pushes or changes
main, source branches, BCC routes, production services or workflow permissions.

`scripts/ci/aurora_core_evidence.py prepare` creates a deterministic two-parent
commit from the staged merge tree. Its ordered parents are the PR source and the
pinned Aurora source; its timestamp comes from the first parent. This is an
evidence commit in the runner's object store, **not a published integration ref**.
The helper bundles the commit, tree, blobs and complete reachable history, verifies
the bundle, and removes its temporary local ref without changing HEAD or index.

The workflow uploads `combined.bundle`, its SHA-256 checksum, `manifest.json` and
combined test logs as a run-specific artifact with requested **90-day retention**.
Repository retention policy may shorten availability. Download/archive the artifact
before expiry; it is not a permanent GitHub commit URL or a release artifact.
No additional third-party dependency or write token is admitted. Only committed
repository source and synthetic test logs are included, not runner environment
dumps, credentials or production data.

The manifest starts at `PREPARED`, with `combined_tests_passed: false`. The final
record becomes `TESTED` only when Aurora, Core and classic BCC checks all succeeded,
the worktree/index/parents/tree still match, and the bundle checksum is unchanged.
Failure, cancellation, skipped checks or source changes cannot be accepted. An
interrupted runner can leave only PREPARED evidence; that is never a passing gate.
Evidence is uploaded on failing test paths too, whenever preparation succeeded.

## Recovery without the CI object store

Download the named artifact from its exact workflow run. In a new directory:

```sh
sha256sum -c combined.bundle.sha256
git init restored
git -C restored bundle verify ../combined.bundle
```

Read `bundle_ref`, `combined_commit`, `combined_tree` and `parents` in
`manifest.json`. Fetch the explicit `bundle_ref` into the new repository (plain
clone does not import custom refs automatically):

```sh
git -C restored fetch ../combined.bundle '<bundle_ref>:<bundle_ref>'
git -C restored show -s --format='%H %T %P' '<combined_commit>'
git -C restored switch --detach '<combined_commit>'
```

Compare all four hashes with the manifest, then reproduce the documented commands:
`npm --prefix web/aurora ci --ignore-scripts --no-audit --no-fund`,
`npm --prefix web/aurora test`,
`go test ./internal/bcc ./internal/incidentclass -count=1`, and
`node --test web/tests/*.test.cjs`. The workflow log links exact source/pin/run and
the final manifest records each test outcome. Dependency versions come from the
committed Go and npm metadata. No success claim follows merely from fetching a
bundle or reproducing the textual merge.

## Regression and acceptance boundaries

Real temporary Git repositories exercise independent bundle recovery, exact tree
and ordered parent identity, deterministic commit identity, unchanged HEAD,
wrong-source/pin refusal, dirty-source refusal, post-test source mutation, bundle
tampering, and failure/skipped/cancelled outcomes. Run:

```sh
python3 -m unittest discover -s scripts/ci/tests -p test_aurora_core_evidence.py -v
```

Development attempt one: six cases passed; the recovery fixture incorrectly
expected clone to import a custom ref. Corrected the fixture to initialize an
empty repository, verify the self-contained bundle and fetch its explicit ref.
An initial read guessed `core-reconciliation-preflight.yml`; inventory found the
existing `core-reconcile-preflight.yml`. No existing file or guard was removed.

This records source regression evidence only. Full current-head CI/race/vet/fuzz,
review, rendered FA/EN × dark/light × 320/390/768/1440 parity, keyboard/accessibility,
classic feature retention and secure feature-flagged route integration remain
separate gates. Native two-node acceptance remains owner-deferred. Aurora must
integrate with classic BCC, preserving its widgets, effects and reversible fallback.
No merge/release/deploy approval is granted by this artifact.
