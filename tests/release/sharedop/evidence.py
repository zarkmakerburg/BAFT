#!/usr/bin/env python3
"""Record source identity, including dirty content, and bind saved LAB evidence."""
import hashlib
import json
import pathlib
import subprocess
import sys


def git(*args):
    return subprocess.check_output(["git", *args])


def digest(path):
    h = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def snapshot():
    # A dirty checkout's HEAD alone does not identify the executed source.
    # Capture every tracked/untracked file's digest, but never ignored files.
    names = git("ls-files", "--cached", "--others", "--exclude-standard", "-z")
    files = {}
    for raw in sorted(set(names.split(b"\0")) - {b""}):
        name = raw.decode("utf-8")
        path = pathlib.Path(name)
        if path.is_symlink():
            files[name] = {"symlink": str(path.readlink())}
        elif path.is_file():
            files[name] = {"sha256": digest(path), "executable": bool(path.stat().st_mode & 0o111)}
        else:
            files[name] = {"missing": True}
    return {
        "head": git("rev-parse", "HEAD").decode().strip(),
        "committed_tree": git("rev-parse", "HEAD^{tree}").decode().strip(),
        "status": git("status", "--porcelain=v1").decode(),
        "files_sha256": hashlib.sha256(json.dumps(files, sort_keys=True).encode()).hexdigest(),
        "files": files,
    }


def main():
    mode, target = sys.argv[1:]
    target = pathlib.Path(target)
    if mode == "snapshot":
        data = snapshot()
    elif mode == "seal":
        before = json.loads((target / "source-before.json").read_text())
        after = json.loads((target / "source-after.json").read_text())
        data = {
            "source_stable": before == after,
            "head": before["head"],
            "committed_tree": before["committed_tree"],
            "dirty": bool(before["status"]),
            "source_files_sha256": before["files_sha256"],
            "evidence_sha256": {
                path.name: digest(path) for path in sorted(target.iterdir())
                if path.is_file() and path.name not in {"metadata.json", "verdict.json"}
            },
        }
        target = target / "metadata.json"
    else:
        raise SystemExit("expected snapshot or seal")
    target.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
