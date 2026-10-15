#!/usr/bin/env python3
"""Preserve a tested merge as local Git objects, never as a remote ref update."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess


def git(*args: str, env=None) -> str:
    return subprocess.check_output(["git", *args], text=True, env=env).strip()


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def write_manifest(directory: Path, manifest: dict) -> None:
    temporary = directory / "manifest.json.tmp"
    temporary.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    temporary.replace(directory / "manifest.json")


def prepare(directory: Path, source: str, incoming: str) -> None:
    for value in (source, incoming):
        require(re.fullmatch(r"[0-9a-f]{40}", value) is not None, "expected full Git SHA")
    require(git("rev-parse", "HEAD") == source, "checkout must be the exact PR source")
    require(git("rev-parse", "MERGE_HEAD") == incoming, "expected pending pinned merge")
    require(not git("diff", "--name-only", "--diff-filter=U"), "unresolved merge")
    require(not git("diff", "--name-only"), "unstaged tracked changes")
    git("diff", "--cached", "--check")
    tree = git("write-tree")
    stamp = git("show", "-s", "--format=%cI", source)
    env = dict(os.environ, GIT_AUTHOR_NAME="BAFT preflight evidence",
               GIT_COMMITTER_NAME="BAFT preflight evidence",
               GIT_AUTHOR_EMAIL="preflight@users.noreply.github.com",
               GIT_COMMITTER_EMAIL="preflight@users.noreply.github.com",
               GIT_AUTHOR_DATE=stamp, GIT_COMMITTER_DATE=stamp)
    commit = git("commit-tree", tree, "-p", source, "-p", incoming,
                 "-m", "Isolated Aurora x Core preflight evidence; not mainline acceptance", env=env)
    directory.mkdir(parents=True, exist_ok=False)
    reference = "refs/baft-preflight/" + commit
    # Only the disposable local object store/ref is touched; HEAD/index stay put.
    git("update-ref", reference, commit)
    try:
        git("bundle", "create", str(directory / "combined.bundle"), reference)
        git("bundle", "verify", str(directory / "combined.bundle"))
    finally:
        git("update-ref", "-d", reference, commit)
    digest = hashlib.sha256((directory / "combined.bundle").read_bytes()).hexdigest()
    (directory / "combined.bundle.sha256").write_text(digest + "  combined.bundle\n")
    manifest = {
        "schema": 1, "state": "PREPARED", "combined_tests_passed": False,
        "source_sha": source, "incoming_sha": incoming,
        "combined_commit": commit, "combined_tree": tree,
        "parents": [source, incoming], "bundle_ref": reference,
        "bundle_sha256": digest, "bundle_self_contained": True,
        "run_id": os.environ.get("GITHUB_RUN_ID"),
        "run_attempt": os.environ.get("GITHUB_RUN_ATTEMPT"),
        "limits": ["Source regression evidence only", "No browser or native lab acceptance",
                   "No mainline merge, release, deployment or live BCC activation"],
    }
    write_manifest(directory, manifest)
    print(json.dumps(manifest, sort_keys=True))


def record(directory: Path, outcomes: list[str]) -> None:
    require(all(value in {"success", "failure", "cancelled", "skipped"} for value in outcomes),
            "unknown test outcome")
    manifest = json.loads((directory / "manifest.json").read_text())
    require(manifest["state"] == "PREPARED", "evidence was already finalized")
    commit = manifest["combined_commit"]
    unchanged = (git("rev-parse", "HEAD") == manifest["source_sha"]
                 and git("write-tree") == manifest["combined_tree"]
                 and not git("diff", "--name-only")
                 and git("show", "-s", "--format=%T", commit) == manifest["combined_tree"]
                 and git("show", "-s", "--format=%P", commit).split() == manifest["parents"])
    digest = hashlib.sha256((directory / "combined.bundle").read_bytes()).hexdigest()
    unchanged = unchanged and digest == manifest["bundle_sha256"]
    passed = unchanged and all(value == "success" for value in outcomes)
    manifest.update(state="TESTED" if passed else "FAILED", combined_tests_passed=passed,
                    tracked_source_unchanged=unchanged,
                    outcomes=dict(zip(("aurora", "core", "classic"), outcomes)))
    write_manifest(directory, manifest)
    print(json.dumps(manifest, sort_keys=True))
    require(passed, "combined tests did not pass on unchanged bundled source")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    first = commands.add_parser("prepare")
    first.add_argument("directory", type=Path)
    first.add_argument("source")
    first.add_argument("incoming")
    last = commands.add_parser("record")
    last.add_argument("directory", type=Path)
    last.add_argument("outcomes", nargs=3)
    args = parser.parse_args()
    if args.command == "prepare":
        prepare(args.directory.resolve(), args.source, args.incoming)
    else:
        record(args.directory.resolve(), args.outcomes)


if __name__ == "__main__":
    main()
