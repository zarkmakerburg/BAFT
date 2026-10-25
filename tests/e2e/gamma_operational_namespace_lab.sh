#!/usr/bin/env bash
# Linux-only execution wrapper, also usable in Colima with a cross-built test
# binary. Compilation and JSON conversion happen outside the private netns.
set -euo pipefail
binary="${1:?usage: gamma_operational_namespace_lab.sh <compiled-bcc-test>}"
test "$(uname -s)" = Linux
test -f "$binary"
for tool in ip unshare mount; do command -v "$tool" >/dev/null; done
test -x /usr/sbin/nft
privilege=()
if test "$(id -u)" != 0; then privilege=(sudo -n); fi
work="$(mktemp -d /tmp/baft-gamma-operational.XXXXXX)"
namespace="baft-gamma-operational-$$"
cleanup() {
  "${privilege[@]}" ip netns del "$namespace" 2>/dev/null || true
  "${privilege[@]}" rm -rf "$work"
}
trap cleanup EXIT
# Parent overlays provide mount targets without creating anything under host
# /etc or /var/lib. All contents below are discarded when this wrapper exits.
"${privilege[@]}" mkdir -p "$work/etc/baft" "$work/etc/baft-agent" "$work/varlib/baft-agent" "$work/baft" "$work/baft-agent" "$work/state"
for path in baft baft-agent state; do
  printf 'disposable-signed-lease-lab' | "${privilege[@]}" tee "$work/$path/.signed-lease-lab" >/dev/null
done
"${privilege[@]}" chown -R 0:0 "$work"
"${privilege[@]}" chmod 0700 "$work" "$work/etc" "$work/varlib" "$work/baft" "$work/baft-agent" "$work/state"
"${privilege[@]}" cp "$binary" "$work/bcc.test"
"${privilege[@]}" chmod 0755 "$work/bcc.test"
"${privilege[@]}" ip netns add "$namespace"
"${privilege[@]}" ip netns exec "$namespace" ip link set lo up
"${privilege[@]}" unshare --mount --propagation private /bin/bash -c '
  set -euo pipefail
  mount --bind "$1/etc" /etc
  mount --bind "$1/baft" /etc/baft
  mount --bind "$1/baft-agent" /etc/baft-agent
  mount --bind "$1/varlib" /var/lib
  mount --bind "$1/state" /var/lib/baft-agent
  exec ip netns exec "$2" env BAFT_OPERATIONAL_INGRESS_LAB=1 "$1/bcc.test" -test.run "^TestOperationalIngressFullLifecycleLAB$" -test.v -test.count=1 -test.timeout=3m
' bash "$work" "$namespace"
