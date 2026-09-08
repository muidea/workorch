"""The release packager cannot bypass missing deployed convergence evidence."""

from pathlib import Path
import subprocess
import tempfile
import unittest


class ReleaseConvergenceGateTest(unittest.TestCase):
    def test_managed_preflight_shares_bounded_make_test_deadline(self):
        repo = Path(__file__).resolve().parent.parent
        managed = (repo / "scripts" / "verify-workorch-managed-full.sh").read_text(encoding="utf-8")
        self.assertIn(
            'run_step go test "-timeout=$(make --no-print-directory -s go-test-timeout)" $GO_TEST_FLAGS $GO_TEST_PACKAGES',
            managed,
        )
        for timeout in ("15m", "20m"):
            with self.subTest(timeout=timeout):
                queried = subprocess.run(
                    ["make", "--no-print-directory", "-s", "go-test-timeout", f"GOTEST_TIMEOUT={timeout}"],
                    cwd=repo, capture_output=True, text=True, timeout=10, check=True,
                )
                self.assertEqual(queried.stdout.strip(), timeout)
                for target, race in (("test", ""), ("test-race", "-race ")):
                    command = subprocess.run(
                        ["make", "--no-print-directory", "-n", target, "GO=go", f"GOTEST_TIMEOUT={timeout}",
                         "GOTEST_FLAGS=-count=1", "PKGS=./internal/pkg/workorchruntimecontract"],
                        cwd=repo, capture_output=True, text=True, timeout=10, check=True,
                    )
                    self.assertIn(
                        f"go test {race}-timeout={timeout} -count=1 ./internal/pkg/workorchruntimecontract",
                        command.stdout,
                    )

    def test_missing_evidence_stops_before_packaging_or_dependency_install(self):
        for jobs in (1, 2):
            with self.subTest(jobs=jobs):
                self.assert_missing_evidence_blocks_release(jobs)

    def assert_missing_evidence_blocks_release(self, jobs):
        repo = Path(__file__).resolve().parent.parent
        with tempfile.TemporaryDirectory(prefix="workorch-release-gate-") as directory:
            root = Path(directory)
            output = root / "packages"
            output.mkdir()
            sentinel = output / "previous-release.txt"
            sentinel.write_text("preserve previous release", encoding="utf-8")
            result = subprocess.run(
                ["make", "--no-print-directory", f"-j{jobs}", "release", "GO=false", "NPM=false",
                 "DEPLOYED_CONVERGENCE_PROFILE=ci-safe",
                 f"DEPLOYED_CONVERGENCE_MANIFEST_ROOT={root}", f"RELEASE_OUT={output}",
                 f"DEPLOYED_CONVERGENCE_REPORT={root / 'report.json'}"],
                cwd=repo, capture_output=True, text=True, timeout=30,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("verify-deployed-convergence-release", result.stdout + result.stderr)
            self.assertIn('--profile "full"', result.stdout)
            self.assertNotIn("false mod download", result.stdout + result.stderr)
            self.assertEqual(sentinel.read_text(encoding="utf-8"), "preserve previous release")
            self.assertEqual(list(output.iterdir()), [sentinel], "unverified release touched existing packages")


if __name__ == "__main__":
    unittest.main()
