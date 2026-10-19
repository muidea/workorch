"""发布打包不得绕过缺失的部署收敛证据。"""

from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest


# Independent expectation of all readiness obligations, not derived from the
# Makefile under test. None may disappear when new checks are added.
READINESS_CHECKS = (
    ("./internal/pkg/workorchruntimecontract ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/pkg/definitionadmission", "^TestGoalExecution", "3m"),
    ("./internal/modules/application/workorchd/biz/appservice/activationcore", "^TestBootstrapGoalExecution", "3m"),
    ("./internal/modules/kernel/orchestrator/biz", "^TestDriveRuntimeGoalExecutionSourceRealRepairAcrossRestart$", None),
    ("./internal/pkg/workorchruntimecontract ./internal/modules/kernel/orchestrator/biz/runtime ./internal/modules/kernel/orchestrator/biz", "Test(RuntimeImplementationWork|ImplementationWork|RuntimeSemantic.*Implementation|RuntimeSemantic.*Purpose)", None),
    ("./internal/pkg/workorchruntimecontract ./internal/modules/kernel/workspace/pkg/models ./internal/modules/kernel/workspace/pkg/definitionadmission", "Test.*Profile.*EvidencePurpose", None),
    ("./internal/modules/kernel/orchestrator/biz", "^TestDriveRuntimeProfileDeliverableHistoricalAcceptance(OwnerCASRecovery|ProcessRecovery)$", None),
    ("./internal/modules/kernel/orchestrator/biz/runtime", "^TestCWISuspension", "3m"),
    ("./internal/modules/kernel/orchestrator/biz", "^TestDriveRuntime(CWISuspension(RevalidatesOrdinaryPredecessorWithinOriginalBudget|ProcessRecovery)|DurableDispatchBudgetRejectsUndeclaredPredecessorRevalidation|AssertionFeedbackCannotInventPredecessorRevalidationAcrossRestart)$", None),
    ("./internal/pkg/workorchruntimecontract", "^Test(CoverageEvidenceBindingCannotConjoinIndependentValidationRecipes|ValidationObservationCannotBorrowPassedChecksAcrossRecipes)$", "3m"),
    ("./internal/integration", "^TestImplementationWork(PublicAuthoringFullFlow|FullflowUnsupportedCombinationStaysClosed|CommitEvidence.*)$", None),
)


class ReleaseConvergenceGateTest(unittest.TestCase):
    def readiness_dry_run(self, timeout="15m", target="verify-runtime-implementation-work-readiness"):
        return subprocess.run(
            ["make", "--no-print-directory", "-n", target, f"GOTEST_TIMEOUT={timeout}"],
            cwd=Path(__file__).resolve().parent.parent, capture_output=True, text=True, timeout=10, check=True,
        ).stdout

    def assert_readiness_commands(self, output, timeout="15m"):
        expected = [
            ["bash", "scripts/lib/go-test-must-run.sh", "--each-package", *packages.split(),
             "-run", selector, "-count=1", f"-timeout={fixed_timeout or timeout}"]
            for packages, selector, fixed_timeout in READINESS_CHECKS
        ]
        # Compare shell tokens, not quote style or brittle three-line offsets.
        # Exact tokens also reject masks, skipped packages/checks and extra ops.
        self.assertEqual([shlex.split(line) for line in output.splitlines()], expected)

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
                        f"go test -p=1 {race}-timeout={timeout} -count=1 ./internal/pkg/workorchruntimecontract",
                        command.stdout,
                    )

    def test_implementation_work_readiness_requires_public_fullflow_and_each_package(self):
        for timeout in ("15m", "20m"):
            with self.subTest(timeout=timeout):
                output = self.readiness_dry_run(timeout)
                self.assert_readiness_commands(output, timeout)
                dependency = self.readiness_dry_run(timeout, "verify-ci-mvp")
                self.assertTrue(dependency.startswith(output), "CI 验收不得移除或后置完整 readiness")

    def test_readiness_contract_rejects_removing_any_check_or_duplicating_one(self):
        commands = self.readiness_dry_run().splitlines()
        self.assert_readiness_commands("\n".join(commands))
        for index in range(len(commands)):
            with self.subTest(removed=index), self.assertRaises(AssertionError):
                self.assert_readiness_commands("\n".join(commands[:index] + commands[index + 1:]))
            with self.subTest(duplicated=index), self.assertRaises(AssertionError):
                self.assert_readiness_commands("\n".join([*commands, commands[index]]))

    def test_readiness_contract_rejects_weakening_guards_or_public_fullflow(self):
        output = self.readiness_dry_run()
        for name, before, after in (
            ("each package", "--each-package", ""),
            ("uncached tests", "-count=1", "-count=0"),
            ("bounded deadline", "-timeout=15m", "-timeout=0"),
            ("fixed deadline", "-timeout=3m", "-timeout=15m"),
            ("required package", "./internal/pkg/workorchruntimecontract ", ""),
            ("public fullflow", "PublicAuthoringFullFlow|", ""),
            ("fail closed", "FullflowUnsupportedCombinationStaysClosed|", ""),
            ("commit evidence", "|CommitEvidence.*", ""),
            ("failure mask", "-count=1", "-count=1 || true"),
        ):
            with self.subTest(name=name), self.assertRaises(AssertionError):
                self.assert_readiness_commands(output.replace(before, after, 1))

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
