#!/usr/bin/env python3
"""Fail closed unless both named combined-path cases really pass without skips."""
import hashlib
import json
import os
import pathlib
import sys

TESTS = (
    "TestBCCSignedSharedSameIROperationalIngressLifecycle",
    "TestBCCSignedSharedSameIROperationalIngressPartialAddFailure",
)


def verify(root):
    errors = []
    meta = json.loads((root / "metadata.json").read_text())
    if not meta.get("source_stable"):
        errors.append("source changed during build/run")
    expected = os.environ.get("EXPECT_SHA")
    if expected and meta.get("head") != expected:
        errors.append("source HEAD differs from EXPECT_SHA")
    if os.environ.get("SHAREDOP_REQUIRE_CLEAN") == "1" and meta.get("dirty"):
        errors.append("source checkout was dirty")
    hashes = meta["evidence_sha256"]
    required = {"source-before.json", "source-after.json", "binaries.sha256", "go-version.txt", "namespaces.txt"}
    required.update(name + suffix for name in TESTS for suffix in (".log", ".json", ".exit"))
    if not required.issubset(hashes):
        errors.append("missing sealed evidence files")
    for name, expected_hash in hashes.items():
        if pathlib.Path(name).name != name:
            raise ValueError("invalid evidence filename")
        actual = hashlib.sha256((root / name).read_bytes()).hexdigest()
        if actual != expected_hash:
            errors.append("evidence changed: " + name)
    before = json.loads((root / "source-before.json").read_text())
    after = json.loads((root / "source-after.json").read_text())
    fingerprint = hashlib.sha256(json.dumps(before["files"], sort_keys=True).encode()).hexdigest()
    if before != after or not meta["source_stable"]:
        errors.append("source snapshots differ")
    if (before["head"] != meta["head"] or before["committed_tree"] != meta["committed_tree"]
            or bool(before["status"]) != meta["dirty"]
            or fingerprint != before["files_sha256"] or fingerprint != meta["source_files_sha256"]):
        errors.append("source identity does not match snapshots")
    for name in TESTS:
        if (root / (name + ".exit")).read_text().strip() != "0":
            errors.append(name + ": process exited unsuccessfully")
        path = root / (name + ".json")
        if path.stat().st_size > 32 * 1024 * 1024:
            raise ValueError("oversized event log")
        events = [json.loads(line) for line in path.read_text().splitlines() if line.strip()]
        for event in events:
            if event.get("Package") != "./internal/agent":
                errors.append(name + ": wrong/missing package")
            test = event.get("Test", "")
            if test and test.split("/", 1)[0] != name:
                errors.append(name + ": unexpected test " + test)
            if event.get("Action") in ("fail", "skip"):
                errors.append(name + ": " + event["Action"] + " " + test)
        for action in ("run", "pass"):
            if sum(e.get("Test") == name and e.get("Action") == action for e in events) != 1:
                errors.append(name + ": expected one top-level " + action)
        if sum(not e.get("Test") and e.get("Action") == "pass" for e in events) != 1:
            errors.append(name + ": missing/duplicate package PASS")
    return {"pass": not errors, "tests": list(TESTS), "head": meta["head"], "dirty": meta["dirty"], "errors": errors}


def main():
    if sys.argv[1:] == ["--list-tests"]:
        print("\n".join(TESTS))
        return
    root = pathlib.Path(sys.argv[1])
    try:
        result = verify(root)
    except (OSError, ValueError, KeyError, TypeError) as exc:
        result = {"pass": False, "errors": [str(exc)]}
    root.mkdir(parents=True, exist_ok=True)
    (root / "verdict.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))
    raise SystemExit(0 if result["pass"] else 1)


if __name__ == "__main__":
    main()
