# BAFT Adversarial Resilience & Intrusion Report

**Verdict: ✅ PASS** · generated 2026-10-30 10:28 UTC · commit `9c32ba369fe2` · profile `bounded`

This report is produced automatically by the periodic resilience tester (`tests/adversary/`). It launches a real BAFT EX/IR pair on loopback and, acting as a censor / DoS attacker and as an on-path intruder, tries to break or penetrate it with real traffic. All scenarios are loopback only and never target any third-party host.

## 1. Destruction / DoS resilience

| scenario | result | heal | EX rss MB | IR rss MB | EX fd | IR fd | note |
|---|---|---|---|---|---|---|---|
| connflood | ✅ | ok | 20 | 20 | 14 | 14 | — |
| garbage | ✅ | ok | 20 | 20 | 14 | 14 | — |
| slowloris | ✅ | ok | 22 | 20 | 14 | 14 | — |
| probe | ✅ | ok | 19 | 20 | 14 | 14 | — |
| manyflows | ✅ | ok | 24 | 22 | 186 | 186 | — |

## 2. Intrusion / penetration attempts

| intrusion | boundary held | detail |
|---|---|---|
| pairing_replay | ✅ | replayed one-time pairing reply rejected |
| rogue_key | ✅ | rogue carrier rejected; EX up; legit traffic intact |
| onpath_tamper | ✅ | on-path tamper detected and failed closed (no corrupted delivery) |

## 3. Findings

- No findings: every destruction scenario survived within the resource bounds and every intrusion attempt was contained.

## 4. Environment

```
profile=bounded seconds_per=6 workers=16 conns=300 slowloris=200 noise=1
sha=9c32ba369fe2be2c549f2878d1de5dbf092c8f64
go version go1.27.3 linux/amd64
Linux runnervm48cm3 7.0.0-1017-azure #17~24.04.1-Ubuntu SMP PREEMPT Thu Sep 24 18:46:57 UTC 2026 x86_64 x86_64 x86_64 GNU/Linux
baseline rss: ex=19MB ir=20MB limit=512MB fd_limit=1024
probe fingerprints (evidence): {'http_get': (352, False), 'random_256': (103, True), 'empty': (0, False), 'baft_hello_prefix': (0, False)}
probe deterministic: identical input gave the same response (0, False)
```

---

Zero-day fuzzing of attacker-reachable parsers runs separately in the `fuzz` workflow; any crasher it finds is uploaded there and seeded back into the corpus as a regression.
