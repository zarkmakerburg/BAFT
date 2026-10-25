#!/usr/bin/env bash
# Positive acceptance only, in private mount+network namespaces. The compiled
# test never sees host BAFT configuration and no host firewall is changed.
set -euo pipefail
cd "$(dirname "$0")/../.."
work="$(mktemp -d "${TMPDIR:-/tmp}/baft-operational-build.XXXXXX")"
trap 'rm -rf "$work"' EXIT
CGO_ENABLED=0 go test -c -o "$work/bcc.test" ./internal/bcc
log="$work/evidence.json"
bash tests/e2e/gamma_operational_namespace_lab.sh "$work/bcc.test" | go tool test2json -p github.com/zarkmakerburg/baft/internal/bcc -t | tee "$log"
bash tests/e2e/gamma_operational_log_assert.sh "$log"
