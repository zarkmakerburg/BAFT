# 5 — Update, backup and uninstall

[Guide index](README.md) · [فارسی](../../fa/guide/05-update-backup-uninstall.md)

## Update

First check whether a newer signed release exists. This only reads; it changes nothing on the server:

```bash
baft update check                                     # exit 0: current, 3: update available, 1: unknown
baft update check --save-installer ./baft-install.sh  # also keep the verified, signed installer
```

The check verifies the release against the pinned root key, the revocation list and this server's release state (no downgrade, no re-tag). If it cannot fetch or verify, it says `UNKNOWN`, never "up to date". It does **not** apply the update; apply it with the installer, for example `sudo BAFT_VERSION=vX.Y.Z bash ./baft-install.sh --plan` to preview and the same command without `--plan` to apply. It is also in the `baft` menu (item 11).

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
