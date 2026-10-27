# EX failover on the IR (`baft failover`)

An IR paired with more than one EX (one tunnel instance per EX) runs
`baft failover` in front of the per-EX route listeners. Clients connect to the
failover listener; each new connection goes to the first EX whose path is
healthy, in the configured preference order.

## Why an end-to-end probe

On a real Iran path (2026-10-27, #67) the connection to one EX opened
normally but stopped carrying data after about 8 KiB; TCP connects and small
pings still succeeded. A dial check cannot see that. Each target therefore has
an optional `probe`: an echo endpoint reached through the same tunnel. A probe
sends `--probe-bytes` (default 32 KiB, above the observed cut-off) and needs the
same bytes back within `--probe-timeout`.

- A target is skipped after `--fail-after` (default 2) failed probes in a row
  and preferred again after `--recover-after` (default 3) good ones.
- When every target is unhealthy, all are still tried in order: a broken
  prober never blocks all traffic.
- A target without `probe` is judged only by whether its route listener
  accepts the connection.
- Existing connections stay on their path; only new connections move.

## Setup

EX side: run the echo endpoint and carry its port through the tunnel
(installer `--ports`, e.g. the service port plus 2444):

```bash
baft echo --listen 127.0.0.1:2444
```

IR side:

```bash
baft failover --listen 127.0.0.1:1080 \
  --target ex2=127.0.0.1:1443,probe=127.0.0.1:2444 \
  --target ex1=127.0.0.1:1543,probe=127.0.0.1:2544
```

## Limits

- Failover moves new connections only; a connection already on a path that
  starts dropping data is lost.
- Probing costs `probe-bytes x 2` per target per interval (about 13 KB/s per
  target at the defaults).
