#!/usr/bin/env bash
# Make the JSON bundle the BCC Update Center imports from an unpacked offline
# bundle (scripts/release/offline_bundle.sh output). Metadata only: the
# binaries stay out of it (BCC never holds them); each node verifies every byte
# itself. Nothing in the output is trusted: BCC verifies it against its pinned
# release root.
#
#   scripts/release/update_center_bundle.sh <unpacked baft-offline-vX.Y.Z dir> > update-bundle.json
#   scripts/release/update_center_bundle.sh --revocations-only <revocations.json> > update-bundle.json
set -euo pipefail

if [[ "${1:-}" == "--revocations-only" ]]; then
  rev=${2:?usage: update_center_bundle.sh --revocations-only <revocations.json>}
  python3 -I - "$rev" <<'PY'
import json, sys
print(json.dumps({"schema_version": 1, "revocations": json.load(open(sys.argv[1]))}, separators=(",", ":")))
PY
  exit 0
fi

dir=${1:?usage: update_center_bundle.sh <unpacked baft-offline dir>}
for f in release/manifest.json release/release-key.cert.json release/SHA256SUMS revocations.json; do
  [[ -s "$dir/$f" ]] || { echo "update_center_bundle: $dir/$f is missing" >&2; exit 1; }
done
python3 -I - "$dir" <<'PY'
import json, os, sys
d = sys.argv[1]
j = lambda p: json.load(open(os.path.join(d, p)))
out = {
    "schema_version": 1,
    "manifest": j("release/manifest.json"),
    "cert": j("release/release-key.cert.json"),
    "revocations": j("revocations.json"),
    "sha256sums": open(os.path.join(d, "release/SHA256SUMS")).read(),
}
s = json.dumps(out, separators=(",", ":"))
if len(s) > 262144:
    sys.exit("update_center_bundle: bundle is larger than the 256 KiB import limit")
print(s)
PY
