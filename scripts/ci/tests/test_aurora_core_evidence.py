"""Real Git fixtures prove evidence can be recovered without the CI object store."""

import json
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "aurora_core_evidence.py"


class MergeEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.repo = self.directory / "repo"
        self.repo.mkdir()
        self.git("init", "-b", "source")
        self.git("config", "user.name", "Fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        (self.repo / "classic").write_text("keep classic BCC\n")
        self.git("add", ".")
        self.git("commit", "-m", "common")
        self.git("checkout", "-b", "aurora")
        (self.repo / "aurora").write_text("integrate Aurora\n")
        self.git("add", ".")
        self.git("commit", "-m", "incoming")
        self.incoming = self.git("rev-parse", "HEAD")
        self.git("checkout", "source")
        (self.repo / "core").write_text("preserve core\n")
        self.git("add", ".")
        self.git("commit", "-m", "source")
        self.source = self.git("rev-parse", "HEAD")
        self.git("merge", "--no-commit", "--no-ff", self.incoming)
        self.output = self.directory / "evidence"

    def git(self, *args, cwd=None):
        return subprocess.check_output(["git", *args], cwd=cwd or self.repo,
                                       text=True, stderr=subprocess.PIPE).strip()

    def run_script(self, *args, ok=True):
        result = subprocess.run(["python3", str(SCRIPT), *map(str, args)], cwd=self.repo,
                                text=True, capture_output=True)
        if ok:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0)
        return result

    def prepare(self):
        self.run_script("prepare", self.output, self.source, self.incoming)
        return json.loads((self.output / "manifest.json").read_text())

    def test_self_contained_bundle_restores_exact_commit_tree_and_parents(self):
        manifest = self.prepare()
        self.assertFalse(manifest["combined_tests_passed"])
        self.assertEqual(manifest["state"], "PREPARED")
        restored = self.directory / "restored"
        self.git("init", str(restored))
        self.git("bundle", "verify", str(self.output / "combined.bundle"), cwd=restored)
        self.git("fetch", str(self.output / "combined.bundle"),
                 manifest["bundle_ref"] + ":" + manifest["bundle_ref"], cwd=restored)
        restored_commit = self.git("rev-parse", manifest["bundle_ref"], cwd=restored)
        self.assertEqual(restored_commit, manifest["combined_commit"])
        self.assertEqual(self.git("show", "-s", "--format=%T", restored_commit, cwd=restored),
                         manifest["combined_tree"])
        self.assertEqual(self.git("show", "-s", "--format=%P", restored_commit, cwd=restored).split(),
                         [self.source, self.incoming])
        self.assertEqual(self.git("show", restored_commit + ":classic", cwd=restored), "keep classic BCC")
        self.assertEqual(self.git("show", restored_commit + ":aurora", cwd=restored), "integrate Aurora")
        self.assertEqual(self.git("rev-parse", "HEAD"), self.source)
        self.run_script("record", self.output, "success", "success", "success")
        self.assertTrue(json.loads((self.output / "manifest.json").read_text())["combined_tests_passed"])

    def test_same_parents_and_tree_produce_same_commit(self):
        first = self.prepare()
        other = self.directory / "second"
        self.run_script("prepare", other, self.source, self.incoming)
        second = json.loads((other / "manifest.json").read_text())
        self.assertEqual(first["combined_commit"], second["combined_commit"])

    def test_failed_skipped_cancelled_checks_cannot_be_accepted(self):
        for index, outcome in enumerate(("failure", "skipped", "cancelled")):
            with self.subTest(outcome=outcome):
                output = self.directory / str(index)
                self.run_script("prepare", output, self.source, self.incoming)
                self.run_script("record", output, "success", outcome, "success", ok=False)
                manifest = json.loads((output / "manifest.json").read_text())
                self.assertEqual(manifest["state"], "FAILED")
                self.assertFalse(manifest["combined_tests_passed"])

    def test_source_or_incoming_mismatch_refused_before_evidence(self):
        self.run_script("prepare", self.output, self.incoming, self.source, ok=False)
        self.assertFalse(self.output.exists())
        self.run_script("prepare", self.output, self.source, "0" * 40, ok=False)
        self.assertFalse(self.output.exists())

    def test_dirty_tracked_source_is_refused(self):
        (self.repo / "classic").write_text("unexpected change\n")
        self.run_script("prepare", self.output, self.source, self.incoming, ok=False)
        self.assertFalse(self.output.exists())

    def test_post_test_worktree_change_cannot_be_accepted(self):
        self.prepare()
        (self.repo / "classic").write_text("test rewrote source\n")
        self.run_script("record", self.output, "success", "success", "success", ok=False)
        self.assertFalse(json.loads((self.output / "manifest.json").read_text())["combined_tests_passed"])

    def test_bundle_tampering_cannot_be_accepted(self):
        self.prepare()
        with (self.output / "combined.bundle").open("ab") as bundle:
            bundle.write(b"changed")
        self.run_script("record", self.output, "success", "success", "success", ok=False)
        self.assertEqual(json.loads((self.output / "manifest.json").read_text())["state"], "FAILED")


if __name__ == "__main__":
    unittest.main()
