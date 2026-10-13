# Security policy

BAFT carries other people's traffic and manages servers, so we take security reports seriously. Thank you for reporting responsibly.

## Report a vulnerability privately

**Do not open a public issue, discussion or pull request for a vulnerability.**

Use GitHub's private vulnerability reporting: open the repository's **[Security tab](https://github.com/zarkmakerburg/baft/security)** and choose **Report a vulnerability** (direct link: <https://github.com/zarkmakerburg/baft/security/advisories/new>). Only the maintainers can see the report.

Please include, in English or Persian:

- the affected component (installer, `baft`, `baft-pair`, `baft-agent`, BCC panel, `baft-panel`, `baft-net`, release signing) and version (`baft version`);
- what an attacker can do, and what they need first (network position, a panel login, a server account);
- steps to reproduce, or a proof of concept;
- any idea for a fix.

Never include real pairing codes, panel passwords, agent tokens, private keys or users' traffic. Redact IP addresses you do not want to share.

## What to expect

- We aim to acknowledge a report within 7 days and to agree on a fix and disclosure date with you.
- Fixes ship as a signed release; the changelog and a GitHub security advisory describe the issue after users have had a chance to update.
- We credit reporters who want to be credited.

## Scope

In scope:

- the tunnel (IR/EX nodes): authentication, mutual TLS, pairing, Route authorization, frame parsing, memory bounds, recovery;
- `install.sh`, release verification (pinned root key, signatures, revocation, anti-downgrade) and the offline bundle;
- the BCC web panel: login (Argon2id, rate limits), sessions and CSRF, the API, SSH bootstrap, backups and restore;
- `baft-agent` and signed jobs (any way to run commands beyond the fixed job list);
- `baft-panel`, `baft-net`, `baft uninstall` and other management commands, including privilege or file-ownership mistakes;
- secrets leaking into logs, support bundles, backups or the audit log.

Out of scope:

- the fact that a network can detect or block BAFT traffic (BAFT makes no undetectability claim); report measurable fingerprinting weaknesses as normal issues;
- weaknesses in x-ui, Xray or other software you carry through BAFT;
- attacks that need root on a server BAFT runs on;
- denial of service by flooding a public port, unless a small input causes a large or unbounded cost;
- missing best-practice headers or reports from automated scanners without a demonstrated impact.

## Supported versions

Only the latest release receives security fixes. Update with the installer (see the [user guide](docs/en/guide/05-update-backup-uninstall.md)).
