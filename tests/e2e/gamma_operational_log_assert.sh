#!/usr/bin/env bash
set -euo pipefail
python3 - "${1:?usage: gamma_operational_log_assert.sh <go-test-json>}" <<'PY'
import json, sys
package = "github.com/zarkmakerburg/baft/internal/bcc"
name = "TestOperationalIngressFullLifecycleLAB"
counts = {"run": 0, "pass": 0}
package_pass = 0
try:
    with open(sys.argv[1]) as stream:
        for line in stream:
            event = json.loads(line)
            if event.get("Package") != package:
                continue
            test, action = event.get("Test", ""), event.get("Action")
            if action == "fail" or (action == "skip" and (test == name or test.startswith(name + "/"))):
                raise ValueError("positive operational LAB failed or skipped")
            if test == name and action in counts:
                counts[action] += 1
            if not test and action == "pass":
                package_pass += 1
    if counts != {"run": 1, "pass": 1} or package_pass != 1:
        raise ValueError("missing exact executed positive operational test and package PASS")
except (OSError, ValueError, TypeError) as error:
    print("BLOCKED: " + str(error), file=sys.stderr)
    sys.exit(1)
print("PASS: named positive operational LAB executed without skips")
PY
