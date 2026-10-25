# R1 — slow-receiver liveness gate (2026-10-25)

Scope: fixture-level correction to `tests/integration`. **No BAFT protocol change,
no Stage-C gate closure, no release/tag/deploy.** #269 remains OPEN / NO-GO.

## Symptom

The exact-head full Linux suite on `hq/release-completion-20261025` (`5e31b1e`)
failed once in `tests/integration` at `TestNoiseMorphingBackpressureBounded`:

```
backpressure_test.go:291: source did not resume after receiver drain: stalled=688128 now=688128
```

The identical failure reproduces on `main` `94c55dc`, so it is **not** introduced
by the ingress branch.

## Measurements (local linux/amd64, Go 1.27.1, unprivileged, no network namespaces)

| Fixture | Failures |
|---|---|
| branch `5e31b1e`, stock 1 KiB buffers | 4/10 |
| `main` `94c55dc`, stock 1 KiB buffers | 2/6 |
| target receive buffer raised to 64 KiB only | 1/8 |
| both fixture socket buffers raised to 64 KiB | 0/8 |
| both fixture socket buffers raised to 8 KiB (**shipped**) | 0/10, 0/3 under `-race` |

Every failure reports the same stalled byte count, which is the expected
backpressure stop point.

## Diagnosis

The failing goroutine dump (temporary instrumentation, not committed) shows the
source blocked in the user-socket write, IR blocked in `reserveReadCapacity`
waiting for replay credit, and EX blocked in `writeConnFull` towards a target
socket whose peer goroutine is already draining with `io.Copy(io.Discard, c)`.
That is a kernel zero-window interaction produced by 1 KiB socket buffers, not a
demonstrated BAFT flow-control defect.

## Change and preserved assertions

The test's target receive buffer and source write buffer move from 1 KiB to
8 KiB. Nothing else changes. The gate still requires:

- the source to stall before 16 MiB while the target is not reading
  (`stalledAt > 0 && stalledAt < totalSource`);
- IR/EX allocator caps (`Total <= 128 KiB`, receive/replay `<= 64 KiB`);
- exactly one stable 64 KiB EX receive reservation;
- the source to resume after the drain within the 3 s deadline;
- `go vet ./tests/integration` clean, and the whole `tests/integration` package
  green (64.9 s) on the exact tree.

## Not claimed

The attribution is fixture-level. Whether a smaller buffer would expose a real
BAFT liveness defect under a different kernel or provider path is untested here.
Exact-head hosted CI is still required before any release decision.
