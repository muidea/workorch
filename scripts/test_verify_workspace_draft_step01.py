#!/usr/bin/env python3
"""Test STEP01 controlled-build rejection paths without running a service."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

SCRIPT = Path(__file__).with_name("verify-workspace-draft-step01.py")
spec = importlib.util.spec_from_file_location("step01", SCRIPT)
step01 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(step01)


class ControlledBuildTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="workorch-step01-builder-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / "source"
        self.source.mkdir()
        self.artifacts = self.root / "evidence"
        self.artifacts.mkdir()
        self.identity = {"sourceCommit": "a" * 40, "buildInputSha256": "b" * 64, "consoleTreeSha256": "c" * 64}

    def test_refuses_old_binary_before_build_or_service(self):
        (self.artifacts / "workorch").write_bytes(b"old binary")
        with mock.patch.object(step01.subprocess, "run") as run:
            with self.assertRaisesRegex(RuntimeError, "pre-existing"):
                step01.build_candidate(self.source, self.artifacts)
            run.assert_not_called()

    def test_cli_does_not_accept_caller_binary(self):
        result = subprocess.run(["python3", str(SCRIPT), "--binary", "/tmp/old-workorch"], capture_output=True)
        self.assertEqual(result.returncode, 2)
        self.assertIn(b"unrecognized arguments", result.stderr)

    def test_build_failure_never_produces_identity(self):
        with mock.patch.object(step01.build_identity, "capture", return_value=self.identity), \
             mock.patch.object(step01.subprocess, "run", return_value=subprocess.CompletedProcess([], 1)), \
             mock.patch.object(step01.build_identity, "produce") as produce:
            with self.assertRaisesRegex(RuntimeError, "fresh build failed"):
                step01.build_candidate(self.source, self.artifacts)
            produce.assert_not_called()

    def test_source_drift_between_build_commands_is_rejected(self):
        with mock.patch.object(step01.build_identity, "capture", side_effect=[self.identity, {**self.identity, "buildInputSha256": "d" * 64}]), \
             mock.patch.object(step01.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run:
            with self.assertRaisesRegex(RuntimeError, "source changed"):
                step01.build_candidate(self.source, self.artifacts)
            self.assertEqual(run.call_count, 1)

    def test_only_fresh_build_runs_fixed_commands_and_binds_preimage(self):
        produced = {**self.identity, "binarySha256": "d" * 64}
        with mock.patch.object(step01.build_identity, "capture", return_value=self.identity), \
             mock.patch.object(step01.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run, \
             mock.patch.object(step01.build_identity, "produce", return_value=produced) as produce:
            binary, identity = step01.build_candidate(self.source, self.artifacts)
            self.assertEqual(binary, self.artifacts / "workorch")
            self.assertEqual(identity, produced)
            self.assertEqual(json.loads((self.artifacts / "expected-build-identity.json").read_bytes()), produced)
            self.assertEqual(run.call_args_list[0].args[0], ["npm", "run", "build"])
            self.assertEqual(run.call_args_list[1].args[0], ["go", "build", "-buildvcs=true", "-o", str(binary), "./application/workorch"])
            self.assertEqual(json.loads((self.artifacts / "pre-go-build.json").read_text())["buildInputSha256"], self.identity["buildInputSha256"])
            produce.assert_called_once()


if __name__ == "__main__":
    unittest.main()
