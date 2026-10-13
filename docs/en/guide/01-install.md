# 1 — Install

[Guide index](README.md) · [فارسی](../../fa/guide/01-install.md)

## What you need

- Two servers with Debian or Ubuntu (amd64 or arm64): one **outside Iran** (where x-ui runs) and one **inside Iran**.
- Root access on both (`sudo -i`).
- If you want the installer to set up the Iran server for you: SSH from the outside server to the Iran server (root, or a user with passwordless `sudo`), with a password or key.
- For the panel with SSL: a domain pointed at the server, or just its public IP, and port 80 free during certificate issuance.

## Step 1: the outside server

Run, as root:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh)
```

Choose **1 (Outside Iran)**. The installer:

1. checks the system, then downloads the signed release and **verifies** it against the pinned root key (an unverified or older release is refused);
2. asks for the public address of this server (the detected one is the default);
3. asks which **ports to carry**. It suggests the ports the server already listens on, such as your x-ui inbounds. Enter them comma-separated, for example `443,2053`;
4. asks how to set up the Iran server: **over SSH from here** (recommended), or by hand with a pairing code;
5. **tests the Iran → outside path** before pairing, so you learn early if your Iran datacenter cuts foreign connections;
6. pairs the two servers and starts the tunnel;
7. offers to install the **web panel** on this server (domain or IP, with a Let's Encrypt certificate), see [Panel](03-panel.md);
8. offers to turn on **BBR** and TCP tuning, see [Network and BBR](04-network-and-bbr.md).

The last screen shows the panel link, username and a one-time password. Save the password: it is not shown again (you can make a new login with `sudo baft-panel`).

## Step 2: the Iran server

### Option A: over SSH from the outside server (recommended)

Give the Iran server's `user@host` (and port if not 22) and a password or key. The installer shows the Iran server's SSH host key fingerprint for you to confirm, then installs and pairs it. You never log in to the Iran server, and the Iran server does not need to reach GitHub, which is often blocked from Iran.

The SSH password or key is used only for this step and is not stored.

### Option B: by hand with a pairing code

The outside server prints a one-time code that starts with `BAFTPAIR1:` and waits. On the Iran server run the same install line, choose **2 (Inside Iran)** and paste the code. The Iran server prints a reply code that starts with `BAFTREPLY1:`; paste it back into the outside server.

The pairing code is valid for **15 minutes** and only once. If it expires, see [Troubleshooting](06-troubleshooting.md#pairing-code-expired).

If the Iran server cannot download from GitHub, use Option A or the [offline install](#offline-install).

## Check that it works

```bash
baft            # status menu
baft doctor     # full health check
baft version    # installed version
```

On the Iran server, `ss -ltn` should show the carried ports listening. Then follow [Use with x-ui](02-use-with-x-ui.md).

## Scripts and automation

All questions can be answered with flags (use `bash -s --` after a piped installer, or download it first):

```bash
curl -fsSLO https://raw.githubusercontent.com/zarkmakerburg/baft/main/install.sh
less install.sh     # read it first if you like
sudo bash install.sh --role ex --public-address EX_IP --ports 443,2053 \
  --ir-ssh root@IR_IP --ir-ssh-key /root/.ssh/id_ed25519 \
  --ir-ssh-fingerprint SHA256:... --with-panel
```

Useful flags: `--role ex|ir`, `--public-address`, `--ports`, `--pairing-code 'BAFTPAIR1:...'` (IR), `--ir-ssh`, `--ir-ssh-key`, `--ir-ssh-fingerprint` (required without a terminal), `--with-panel` / `--no-panel`, `--bcc-domain`, `--bcc-port`, `--version vX.Y.Z`, `--plan` (show what would change, change nothing), `--yes`. Without a terminal (or with `BAFT_NONINTERACTIVE=1`) nothing is asked and `--role` is required. `BAFT_TUNE_NETWORK=1` turns on BBR without asking. Run `bash install.sh --help` for the full list.

## Offline install

For a server that cannot reach GitHub, every release has a `baft-offline-<version>.tar.gz` bundle. Copy it to the server, then:

```bash
sha256sum -c baft-offline-vX.Y.Z.tar.gz.sha256
tar -xzf baft-offline-vX.Y.Z.tar.gz && cd baft-offline-vX.Y.Z
sudo bash install.sh --offline . --role ir --pairing-code 'BAFTPAIR1:...'
```

`--offline` uses no network and no `apt`; `python3`, `openssl` and `sha256sum` must already be installed. The release inside is verified exactly as an online install. See [Signed releases](../23-p1a-signed-releases.md).

## Running the installer again

Running the same line again is safe: it inspects the server, shows a plan, changes only what is listed and rolls back on failure. It never rotates keys or re-pairs on its own. See [Update, backup, uninstall](05-update-backup-uninstall.md) and [The rerunnable installer](../33-rerunnable-installer.md).

More detail: [Running IR and EX](../06-running-ir-ex.md), [SSH bootstrap](../26-p1d-ssh-bootstrap.md).
