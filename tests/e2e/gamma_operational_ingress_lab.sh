#!/usr/bin/env bash
# Positive acceptance only. The named test owns its disposable Linux LAB;
# this runner never grants authority or substitutes the negative receipt test.
set -euo pipefail
cd "$(dirname "$0")/../.."
log="$(mktemp "${TMPDIR:-/tmp}/baft-operational-ingress.XXXXXX")"
trap 'rm -f "$log"' EXIT
go test -json ./internal/bcc -run '^TestOperationalIngressFullLifecycleLAB$' -count=1 -timeout=15m | tee "$log"
bash tests/e2e/gamma_operational_log_assert.sh "$log"
