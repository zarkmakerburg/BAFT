# DNS failover for IR entry servers (free Cloudflare DNS)

Clients connect to one name (for example `t.example.com`). `baft dns-failover`
keeps that name's A/AAAA records equal to the IR servers that answer, using the
free Cloudflare DNS API. No paid load balancer is needed.

## Behaviour

- Every `interval_seconds` (default 10) each target's `probe` (host:port) gets
  a TCP connect with `probe_timeout_ms` (default 3000).
- A target is withdrawn after `fail_after` (default 3) failed probes in a row
  and published again after `recover_after` (default 3) good ones.
- Records are DNS-only (not proxied) with `ttl` (default 60 s).
- New records are added before old ones are removed, so the name never
  resolves to nothing.
- When **no** target answers, nothing is withdrawn: the prober's own network is
  the likelier fault.
- Only records carrying the comment `managed-by:baft-dns-failover` are touched.
  Any other A/AAAA/CNAME record with the same name stops reconciliation.
- A restart starts with every target healthy and never withdraws before probing.

## Config

```json
{
  "zone_id": "<Cloudflare zone id>",
  "record": "t.example.com",
  "token_file": "/etc/baft/cloudflare-dns.token",
  "targets": [
    {"name": "ir-a", "address": "193.201.66.33",  "probe": "193.201.66.33:443"},
    {"name": "ir-b", "address": "194.110.24.20",  "probe": "194.110.24.20:443"},
    {"name": "ir-c", "address": "194.110.24.123", "probe": "194.110.24.123:443"}
  ]
}
```

The token needs only `Zone.DNS: Edit` for that zone. The token file must be
mode 0600 (group/other access is refused).

```bash
baft dns-failover --config /etc/baft/dns-failover.json --once   # one probe + reconcile, prints JSON
baft dns-failover --config /etc/baft/dns-failover.json          # long-running (systemd)
```

## Limits

- The probe runs from wherever this command runs (an EX). If that host cannot
  reach Iran while clients can, it sees every IR as down and changes nothing.
  Run it on more than one EX only with distinct record names; two writers on
  one name are not coordinated.
- DNS caching means clients move within roughly `ttl` plus resolver behaviour,
  not instantly; open connections to a dead IR are lost.
