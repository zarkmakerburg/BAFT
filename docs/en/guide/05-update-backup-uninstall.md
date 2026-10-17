# 5 — Update, backup and uninstall

[Guide index](README.md) · [فارسی](../../fa/guide/05-update-backup-uninstall.md)

## Update

First check whether a newer signed release exists. This only reads; it changes nothing on the server:

```bash
baft update check                                     # exit 0: current, 3: update available, 1: unknown
baft update check --save-installer ./baft-install.sh  # also keep the verified, signed installer
```

The check verifies the release against the pinned root key, the revocation list and this server's release state (no downgrade, no re-tag). If it cannot fetch or verify, it says `UNKNOWN`, never "up to date". It also judges each installed component (the transport service, the agent, BCC) by its **own binaries**: they are compared byte for byte with the signed release's artifacts. The release state is shared by Core and BCC, so a panel left on an old binary after Core was updated shows as `OUTDATED` and the check reports an update, even though the release state already names the newest version. It is also in the `baft` menu (item 11).

Then apply it from the same CLI. It runs the release's own signed installer for each component that is behind, so nothing here is a second installer:

```bash
baft update apply --plan   # no root: shows exactly what would change, changes nothing
sudo baft update apply     # shows the plan, asks you to type `update`, then applies
```

- **What runs, in what order:** BCC, then the agent, then the transport service (control plane first, so a failure stops before the tunnels' own service is touched). Each one is run in the mode it was installed in: `--bcc-only`, `--agent-only`, or `--role ir|ex` read from the node's config. The agent's inputs (BCC address, node id, token file, pinned job key) are recovered from its own unit, so nothing is asked and nothing is regenerated. `--only transport,agent,bcc` selects some.
- **What it refuses:** an install that is not healthy (repair it first), and any installer step that would touch keys, certificates, config, pairing, tokens, access files or units. An installer step this `baft` does not know is refused too, never waved through; apply by hand in that case. For BCC it also needs a recent backup (`*.baftbak` newer than 26 hours in BCC's backup directory), or `--skip-backup-check` on purpose.
- **Safety while it runs:** the installer is the verified, signed one (its sha256 is checked again right before every run) and the version is pinned to the one that verified. It runs in its own session with its output in `/opt/baft/update-logs/`, so a dropped SSH session or Ctrl-C does not leave a half-applied install: it finishes (or rolls back) on its own. `tail -f` the log to follow it. A failure stops the run before the next component.
- **After:** a second plan must have nothing left to change and the binaries must now be the signed release's; otherwise it says so and exits 1. A service is restarted when its binary changes, which briefly interrupts it.

Run the same install line again on each server (outside and Iran together, so both sides run the same version):

```bash
bash <(curl -Ls https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh)
```

A rerun is **not** a reinstall. It inspects the server, shows a plan, changes only what is listed and checks the result; if a check fails, it puts the previous files back and restarts what was running. It never rotates keys or certificates, never overwrites a valid config, never re-pairs and never installs an older release (anti-downgrade).

With a downloaded copy of the installer (`curl -fsSLO https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh`):

```bash
sudo bash install.sh --plan          # show what would change, change nothing
sudo bash install.sh --yes           # apply without asking (scripts)
sudo bash install.sh --re-pair --yes # deliberately start a new pairing
```

If the Iran server cannot reach GitHub, update it with the [offline bundle](01-install.md#offline-install).

The panel: `sudo baft-panel` → 8 (update).

Details: [The rerunnable installer](../33-rerunnable-installer.md).

## Backup

- **Panel**: the panel writes encrypted backups when `BAFT_BCC_BACKUP_KEY` is set (7 daily and 4 weekly are kept). `baft-bcc restore-preview` shows what a restore would change without changing anything, and `baft-bcc restore` restores offline. See [BCC backups](../29-bcc-backup-restore-preview.md).
- **Servers**: the important files are `/etc/baft` (config, keys, certificates) and the release state. A standard uninstall keeps them, so a reinstall brings back the same server without re-pairing.
- Before a rerun changes a file, the installer copies it to `/opt/baft/backups/rerun-<time>/` (the newest three are kept).

## Uninstall

```bash
baft uninstall --preview   # the exact plan; changes nothing
baft uninstall             # remove BAFT services and binaries, KEEP data
baft uninstall --full      # everything; each data class is asked separately (default NO)
```

Also in the `baft` menu (item 13). Uninstall only removes what is proven to be BAFT's, never stops a running tunnel without asking, takes a verified emergency backup before deleting panel data, and can be undone or finished after an interruption (`baft uninstall --restore` / `--resume`). `sudo baft-net off` removes the network tuning.

Details: [Safe uninstall](../34-safe-uninstall.md).
