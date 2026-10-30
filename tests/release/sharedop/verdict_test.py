#!/usr/bin/env python3
"""Exercise missing, skipped, failed and stale evidence rejection with fixtures."""
import hashlib
import json
import os
import pathlib
import tempfile
import unittest
from unittest.mock import patch

import verdict


class VerdictFixtures(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)
        for name in ("binaries.sha256", "go-version.txt", "namespaces.txt"):
            (self.root / name).write_text("fixture\n")
        fingerprint = hashlib.sha256(json.dumps({}, sort_keys=True).encode()).hexdigest()
        source = {"head": "a" * 40, "committed_tree": "c" * 40, "status": "", "files": {}, "files_sha256": fingerprint}
        for name in ("source-before.json", "source-after.json"):
            (self.root / name).write_text(json.dumps(source))
        for test in verdict.TESTS:
            (self.root / (test + ".exit")).write_text("0\n")
            (self.root / (test + ".log")).write_text("fixture text\n")
            self.events(test, [
                {"Action": "run", "Test": test},
                {"Action": "pass", "Test": test},
                {"Action": "pass"},
            ])
        self.meta = {"source_stable": True, "head": "a" * 40, "committed_tree": "c" * 40,
                     "dirty": False, "source_files_sha256": fingerprint}
        self.seal()
        self.env = patch.dict(os.environ, {}, clear=True)
        self.env.start()
        self.addCleanup(self.env.stop)

    def events(self, name, events):
        for event in events:
            event.setdefault("Package", "./internal/agent")
        (self.root / (name + ".json")).write_text("".join(json.dumps(e) + "\n" for e in events))

    def seal(self):
        self.meta["evidence_sha256"] = {
            p.name: hashlib.sha256(p.read_bytes()).hexdigest()
            for p in self.root.iterdir() if p.name != "metadata.json"
        }
        (self.root / "metadata.json").write_text(json.dumps(self.meta))

    def assert_rejected(self):
        self.seal()
        self.assertFalse(verdict.verify(self.root)["pass"])

    def test_complete_fixture(self):
        self.assertTrue(verdict.verify(self.root)["pass"])

    def test_empty_and_run_only(self):
        test = verdict.TESTS[0]
        for events in ([], [{"Action": "run", "Test": test}, {"Action": "pass"}]):
            with self.subTest(events=events):
                self.events(test, events)
                self.assert_rejected()

    def test_skip_and_failure_cannot_hide_under_top_level_pass(self):
        test = verdict.TESTS[0]
        for action, name in (("skip", test + "/sub"), ("fail", test + "/sub"), ("fail", "")):
            with self.subTest(action=action, name=name):
                self.events(test, [
                    {"Action": "run", "Test": test}, {"Action": action, "Test": name},
                    {"Action": "pass", "Test": test}, {"Action": "pass"},
                ])
                self.assert_rejected()

    def test_nonzero_exit_with_pass_output(self):
        (self.root / (verdict.TESTS[0] + ".exit")).write_text("1\n")
        self.assert_rejected()

    def test_changed_source_or_wrong_head(self):
        self.meta["source_stable"] = False
        self.assert_rejected()
        self.meta["source_stable"] = True
        os.environ["EXPECT_SHA"] = "b" * 40
        self.assert_rejected()

    def test_dirty_release_checkout(self):
        os.environ["SHAREDOP_REQUIRE_CLEAN"] = "1"
        self.meta["dirty"] = True
        self.assert_rejected()

    def test_changed_log_after_seal(self):
        (self.root / (verdict.TESTS[0] + ".log")).write_text("changed")
        self.assertFalse(verdict.verify(self.root)["pass"])

    def test_missing_sealed_evidence(self):
        (self.root / "binaries.sha256").unlink()
        self.assert_rejected()

    def test_unexpected_test(self):
        self.events(verdict.TESTS[0], [{"Action": "pass", "Test": "TestOther"}])
        self.assert_rejected()

    def test_snapshots_checked_independently_of_stable_flag(self):
        path = self.root / "source-after.json"
        source = json.loads(path.read_text())
        source["head"] = "d" * 40
        path.write_text(json.dumps(source))
        self.assert_rejected()


if __name__ == "__main__":
    unittest.main()
