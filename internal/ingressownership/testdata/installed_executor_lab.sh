#!/bin/bash
# Run only on a disposable Linux LAB host with systemd PID1. Every file under
# the fixed /etc and /var/lib targets is masked by private disposable mounts.
set -euo pipefail
LAB_BINARY=${1:?provide an absolute Linux ingressownership test binary}
test -f "$LAB_BINARY"
LAB_BASE=$(mktemp -d /tmp/baft-delta-bootstrap.XXXXXX)
LAB_NS=baft-delta-bootstrap-$$
cleanup() { sudo ip netns del "$LAB_NS" 2>/dev/null || true; sudo rm -rf "$LAB_BASE"; }
trap cleanup EXIT
sudo mkdir -p "$LAB_BASE/etc/baft" "$LAB_BASE/etc/baft-agent" "$LAB_BASE/varlib/baft-agent" "$LAB_BASE/baft" "$LAB_BASE/baft-agent" "$LAB_BASE/state"
for fixture in baft baft-agent state; do printf 'disposable-executor-bootstrap-lab' | sudo tee "$LAB_BASE/$fixture/.executor-bootstrap-lab" >/dev/null; done
sudo chmod 700 "$LAB_BASE" "$LAB_BASE/etc" "$LAB_BASE/varlib" "$LAB_BASE/baft" "$LAB_BASE/baft-agent" "$LAB_BASE/state"
sudo cp "$LAB_BINARY" "$LAB_BASE/executor.test"
sudo chmod 755 "$LAB_BASE/executor.test"
sudo ip netns add "$LAB_NS"
sudo ip netns exec "$LAB_NS" ip link set lo up
sudo unshare --mount --propagation private /bin/bash -c 'set -euo pipefail; mount --bind "$1/etc" /etc; mount --bind "$1/baft" /etc/baft; mount --bind "$1/baft-agent" /etc/baft-agent; mount --bind "$1/varlib" /var/lib; mount --bind "$1/state" /var/lib/baft-agent; exec ip netns exec "$2" env BAFT_EXECUTOR_BOOTSTRAP_LAB=1 "$1/executor.test" -test.run "^TestInstalledManagedExecutorBootstrapLAB$" -test.v -test.count=1' bash "$LAB_BASE" "$LAB_NS"
