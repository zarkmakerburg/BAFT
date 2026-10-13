# 4 — Network, BBR and path tests

[Guide index](README.md) · [فارسی](../../fa/guide/04-network-and-bbr.md)

`sudo baft-net` is installed on every BAFT server. Run it without arguments for a menu, or give the item name.

| # | Command | What it does |
|---|---|---|
| 1 | `sudo baft-net status` | shows whether BBR and the tuning are on, the qdisc and the kernel |
| 2 | `sudo baft-net on` | turns on BBR (if the kernel has it) and TCP buffer tuning |
| 3 | `sudo baft-net off` | turns them off and returns to the system defaults |
| 4 | `sudo baft-net speed` | download and upload test; also reports when a foreign TCP transfer is cut after a few KB |
| 5 | `sudo baft-net latency` | connect time to a few well-known hosts |
| 6 | `sudo baft-net route` | route to a host (mtr, installed if missing) |
| 7 | `sudo baft-net port` | is a port open on a host, and how long the connect takes |

## BBR and tuning

The installer offers to turn the tuning on once. `BAFT_TUNE_NETWORK=1` turns it on without asking, `0` never asks. The settings are written to `/etc/sysctl.d/99-baft-net.conf` and survive reboots. They are: BBR with the `fq` qdisc, larger TCP read/write buffers (up to 32 MB), MTU probing, no slow start after idle, and larger connection backlogs. `sudo baft-net off` removes the file.

BBR usually helps on long, lossy paths such as Iran ↔ abroad. If something gets worse, turn it off; it is safe to switch either way.

## Testing the path from Iran

Some Iranian datacenters let a foreign TCP connection open and then cut it after a few kilobytes (in one case, about 8.7 KB on every port except 22). A simple "port open" check does not show this. Before you rely on a port:

1. On the **Iran server**, run `sudo baft-net speed`. If it says *"the transfer stopped after N bytes: foreign TCP looks cut on this server"*, the datacenter is cutting foreign traffic.
2. Run `sudo baft-net port` and enter the outside server's IP and the BAFT port, then the carried ports.
3. If one port is cut, try another one, or another Iran datacenter.

The installer runs its own Iran → outside path test before pairing and stops with a clear message if the path does not carry data.
