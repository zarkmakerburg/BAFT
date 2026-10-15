# BAFT Android/Termux offline release signing

This workflow allows the HQ key holder to sign on an Android phone without a Mac. **No private key is uploaded to GitHub, pasted into chat, or stored in GitHub Actions.** Do not use this workflow to bypass HQ release approval.

## Preconditions

1. Android phone with Termux from F-Droid and adequate storage. Install tools: `pkg update && pkg install git golang coreutils unzip curl`. A Go version compatible with the repository's `go.mod` is needed; `GOTOOLCHAIN=auto` may download a newer toolchain.
2. An **existing** authorized release private key `release.key`, its matching **root-signed** `release-key.cert.json`, and the committed trust files `release/keys/root.pub` and `release/keys/revocations.json`. Never create a replacement root key just to sign: deployed installers pin the existing root.
3. An HQ-approved release **tag** pointing to the reviewed commit on main. A green PR alone is not release authorization. The release workflow creates an **unsigned-release-candidate** Actions artifact after the approved tag is pushed.
4. Download the unsigned artifact from the exact GitHub Actions run, checking tag and commit against the HQ-approved values using a separately obtained GitHub commit reference. Treat the artifact as untrusted until independently checked.

## Phone preparation

Run in Termux:

```sh
pkg update
pkg install git golang coreutils unzip
umask 077
mkdir -p "$HOME/baft-signing/keys" "$HOME/baft-signing/work"
chmod 700 "$HOME/baft-signing/keys"
git clone https://github.com/zarkmakerburg/BAFT.git "$HOME/baft-signing/source"
cd "$HOME/baft-signing/source"
go build -o "$HOME/baft-signing/baft-release" ./cmd/baft-release
```

Move the authorized `release.key` and `release-key.cert.json` into `$HOME/baft-signing/keys/` using a trusted local transfer; never place them in shared storage or a repository. Run `chmod 600 "$HOME/baft-signing/keys/"*`. Prefer a separate, disconnected signing device; if using an internet-connected daily phone, this is **phone-isolated signing**, not a true offline hardware-security boundary. Disconnect networking during the signing step.

Unzip the exact downloaded GitHub artifact into `$HOME/baft-signing/work/candidate` such that the directory contains `CANDIDATE`, `dist/`, `root.pub`, `revocations.json`, and `install.sh`. Run the repository script:

```sh
cd "$HOME/baft-signing/source"
bash scripts/release/mobile_sign_termux.sh \
  "$HOME/baft-signing/work/candidate" \
  "vX.Y.Z" \
  "FULL_40_CHARACTER_APPROVED_COMMIT_SHA" \
  "$HOME/baft-signing/keys/release.key" \
  "$HOME/baft-signing/keys/release-key.cert.json"
```

The script refuses mismatched tag/commit, already signed or unexpected candidate contents, missing files, mismatched installer copies, untrusted root, and signing key/certificate mismatches (checked by `baft-release sign`). It signs using the existing BAFT signer, verifies the output using the pinned root and revocations, and produces `SHA256SUMS`, `manifest.json`, and `release-key.cert.json` under `candidate/dist/`.

**Important:** This does not prove a GitHub artifact was built from the approved source. Confirm the approved tag, workflow run SHA, CI and artifact provenance independently before signing. Do not run arbitrary scripts bundled in the downloaded artifact. The local signer is built from the independently reviewed repository source.

## After signing

Keep `release.key` on the phone only. The **public** signed release assets under `candidate/dist/` can be transferred for separate verification and authorized publication; private keys must never be included. Do not publish, tag, merge or deploy until HQ expressly approves. Production signing is distinct from CI's throwaway-key tests.

If no valid release private key or root-signed certificate is available on the phone, **stop**. Recover the existing authorized key through a secure process; do not generate a new key pair and assume existing installations will trust it.
