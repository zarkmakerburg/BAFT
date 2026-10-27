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

## Optional, from the installer

DNS failover is opt-in. On the EX the installer asks for a Cloudflare domain
(Enter skips it) or takes `--cloudflare-domain example.com`; with a domain it
also needs the IR public IPs (`--dns-targets 1.2.3.4,5.6.7.8`) and an API token
(`BAFT_CLOUDFLARE_TOKEN`, or asked hidden; never a command-line argument). It
then generates `baft-xxxxxx.example.com` and starts the service. Without a
domain nothing changes: clients use the IR addresses directly, exactly as
before. A failure here only warns, because the tunnel is already working; a
rerun keeps an existing setup.

## One-step setup

The operator gives only the Cloudflare API token and the name; the zone is
found automatically and the service is installed and started:

```bash
echo "$CF_TOKEN" | sudo baft dns-failover setup --domain example.com \
  --target ir-a=193.201.66.33 --target ir-b=194.110.24.20 --target ir-c=109.70.76.143
```

`--domain` generates a random `baft-xxxxxx.<domain>` name (printed at the
end); `--record` uses a name you choose instead. The token is read from stdin (never a command-line argument), checked against
Cloudflare before anything is written, and stored as `/etc/baft/cloudflare-dns.token`
(0600). The token needs `Zone:Read` and `DNS:Edit` on that zone. `--probe-port`
(default 443) or `name=IP:port` sets what is probed.

## Config

```json
{
  "zone_id": "<optional; found from record when empty>",
  "record": "t.example.com",
  "token_file": "/etc/baft/cloudflare-dns.token",
  "targets": [
    {"name": "ir-a", "address": "193.201.66.33",  "probe": "193.201.66.33:443"},
    {"name": "ir-b", "address": "194.110.24.20",  "probe": "194.110.24.20:443"},
    {"name": "ir-c", "address": "194.110.24.123", "probe": "194.110.24.123:443"}
  ]
}
```

The token needs `Zone:Read` (to find the zone) and `DNS:Edit` for that zone. The token file must be
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
