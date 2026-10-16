# Exact-source CI admission for release acceptance

`scripts/release/accept.sh` checks the current remote tag and main history,
then reads GitHub Actions workflow runs for that exact commit. A successful
job name alone cannot qualify a release. The latest run number/attempt for
each intended workflow file decides; an older green run cannot hide a newer
failure, cancellation, unfinished run or missing conclusion.

Required evidence is:

- `.github/workflows/ci.yml`: overall `completed/success` and all 15 current
  mandatory jobs, including license checks, installer faults, BCC/schema
  migration, offline install, SSH install and uninstall.
- `.github/workflows/static-analysis.yml`: overall `completed/success` and
  the `gosec` job successful on the same commit.
- `.github/workflows/r2-noise.yml`: overall `completed/success` and the `r3`
  job successful on the same commit. If branch/path filters do not start it,
  dispatch this workflow on the frozen candidate ref and verify the run SHA.
  Absence is a failed admission gate, not an informational skip.

The overall main-workflow check also catches failure of a newly added job
before that job is added to the explicit mandatory-job list. Missing named
jobs fail closed even if the overall run is reported successful.

The regression fixtures execute the real acceptance script against disposable
Git remotes and a local fake GitHub CLI. They falsify whole-run success,
missing mandatory jobs, missing/wrong-source/wrong-workflow security and R3
evidence, and later failures despite green named jobs. Existing tests retain
remote-tag freshness, workflow identity and latest-run selection checks.
The qualified-CI fixture still fails on absent published assets: admission
does not replace signature, manifest, provenance or offline-bundle acceptance.
Fixtures neither contact production nodes nor sign/publish releases.

This tightens the acceptance policy for the current release program. Historical
tags without all of this evidence may now fail this script; that does not
retroactively establish a defect in their signatures or binaries. Requalify
the intended exact source rather than relaxing gates or borrowing another
commit's green result.

This change does not authorize a tag, main merge, production signature,
release or deployment. HQ must still approve the final scope and decision;
native Iran–Germany acceptance remains explicitly deferred. Aurora remains
an additive BCC workstream and is not activated by this release-gate change.
