#!/usr/bin/env bash
# Disposable root filesystem only: never bind-mount host /etc or policy state.
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
command -v go >/dev/null
command -v docker >/dev/null
case "$(docker info --format '{{.Architecture}}')" in
  arm64|aarch64) lab_arch=arm64 ;;
  amd64|x86_64) lab_arch=amd64 ;;
  *) echo 'Unsupported Docker architecture for managed ingress LAB' >&2; exit 1 ;;
esac
lab_dir="$(mktemp -d "${TMPDIR:-/tmp}/baft-managed-policy.XXXXXX")"
lab_name="$(basename "$lab_dir" | tr '[:upper:]' '[:lower:]')"
tests_image="baft-managed-policy-lab:${lab_name}-tests"
cli_image="baft-managed-policy-lab:${lab_name}-cli"
cleanup() {
  docker rm -f "${lab_name}-authority" "${lab_name}-inventory" "${lab_name}-cli" >/dev/null 2>&1 || true
  docker image rm "$cli_image" "$tests_image" >/dev/null 2>&1 || true
  rm -rf "$lab_dir"
}
trap cleanup EXIT
CGO_ENABLED=0 GOOS=linux GOARCH="$lab_arch" go test -c ./internal/ingressauthority -o "$lab_dir/authority.test"
CGO_ENABLED=0 GOOS=linux GOARCH="$lab_arch" go test -c ./internal/ingressinventory -o "$lab_dir/inventory.test"
CGO_ENABLED=0 GOOS=linux GOARCH="$lab_arch" go build -o "$lab_dir/baft-agent" ./cmd/baft-agent
# A private host umask must not prevent the UID501 subprocess from running
# the root-owned test binary inside the image.
chmod 0755 "$lab_dir/authority.test" "$lab_dir/inventory.test" "$lab_dir/baft-agent"
mkdir -p "$lab_dir/rootfs/tmp" "$lab_dir/rootfs/root" "$lab_dir/cli-rootfs/etc/systemd/system" "$lab_dir/cli-rootfs/usr/lib/systemd/system"
chmod 0755 "$lab_dir/rootfs"
find "$lab_dir/cli-rootfs" -type d -exec chmod 0755 {} +
chmod 1777 "$lab_dir/rootfs/tmp"
chmod 0700 "$lab_dir/rootfs/root"
printf 'isolated-policy-lab' > "$lab_dir/rootfs/baft-policy-lab"
chmod 0600 "$lab_dir/rootfs/baft-policy-lab"
cat > "$lab_dir/Dockerfile" <<'DOCKERFILE'
FROM scratch AS tests
COPY authority.test /authority.test
COPY inventory.test /inventory.test
COPY baft-agent /baft-agent
COPY rootfs/ /
ENV BAFT_ISOLATED_POLICY_LAB=1
ENTRYPOINT ["/authority.test"]
FROM tests AS cli
COPY cli-rootfs/ /
DOCKERFILE
docker build --quiet --target tests -t "$tests_image" "$lab_dir"
docker build --quiet --target cli -t "$cli_image" "$lab_dir"
docker run --rm --network none --name "${lab_name}-authority" "$tests_image" \
  -test.run '^Test(ProvisionInstalledPolicyLAB|InstalledPolicyBindingLAB|CommandIntent|ExpiredRecordedCommandLAB|DurableLifecycle|Lifecycle)' -test.count=1 -test.v
docker run --rm --network none --name "${lab_name}-inventory" --entrypoint /inventory.test "$tests_image" \
  -test.run '^Test' -test.count=1 -test.v
docker run --rm --network none --name "${lab_name}-cli" --entrypoint /baft-agent "$cli_image" \
  --provision-ingress-policy --node-id ir-lab-bootstrap
