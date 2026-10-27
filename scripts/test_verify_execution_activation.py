"""Admission assertions and shell failure propagation without a deployed service."""

import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import tempfile
import unittest

from verify_execution_activation import validate_accepted_activation


ROOT = Path(__file__).resolve().parent.parent


def response(**changes):
    result = {
        "accepted": True, "workspaceId": "ws-test", "intent": "restart",
        "requestId": "request-test", "requestReason": "e2e_restart", "source": "e2e",
        "state": "accepted", "outcome": "replacement_pending",
        "reason": "terminal_predecessor_successor_requested",
    }
    result.update(changes)
    return result


def shell_function(script, name):
    # Extract a function without sourcing a live verifier's mutating main body.
    text = (ROOT / "scripts" / script).read_text(encoding="utf-8")
    match = re.search(rf"(?ms)^{name}\(\) \{{\n.*?^\}}\n", text)
    if not match:
        raise AssertionError(f"missing function {name} in {script}")
    return match.group(0)


class ActivationContractTest(unittest.TestCase):
    def validate(self, data):
        validate_accepted_activation(data, intent="restart", workspace_id="ws-test",
                                     request_id="request-test", request_reason="e2e_restart",
                                     require_source=True)

    def test_all_accepted_owner_reasons(self):
        pairs = {
            "reconciliation_requested": (
                "current_run_continuation_accepted", "pending_run_activated", "new_run_activated",
            ),
            "replacement_pending": (
                "duplicate_restart", "predecessor_supersede_requested", "terminal_predecessor_successor_requested",
            ),
        }
        for outcome, reasons in pairs.items():
            for reason in reasons:
                with self.subTest(outcome=outcome, reason=reason):
                    self.validate(response(outcome=outcome, reason=reason))
        validate_accepted_activation(response(intent="resume", outcome="reconciliation_requested",
                                               reason="current_run_continuation_accepted"), intent="resume")

    def test_scope_identity_and_owner_rejections_fail_closed(self):
        invalid = {
            "accepted": (False, 1, None), "workspaceId": ("ws-other", "", 1),
            "requestId": ("another-request", "", 7), "intent": ("resume", "continue", None),
            "requestReason": ("other", "", None), "state": ("queued", "rejected", None),
            "source": ("", None, 7), "reason": ("new_run_activated", "unknown", "", [], None),
            "outcome": ("reconciliation_requested", "restart_required", "fresh_activation_required",
                        "definition_changed_requires_restart", "governance_required", "unknown", [], None),
        }
        for field, values in invalid.items():
            for value in values:
                with self.subTest(field=field, value=value):
                    with self.assertRaises(ValueError):
                        self.validate(response(**{field: value}))
        for value in (None, [], "response"):
            with self.assertRaises(ValueError):
                self.validate(value)

    def test_verifier_wrappers_do_not_poll_after_invalid_admission(self):
        invocations = {
            "verify-llm-workspace-execution.sh": (
                "wait_activation_run", 'wait_activation_run "$response_file" "$output_file"'),
            "verify-mcp-stdio-workspace-execution.sh": (
                "wait_activation_run", 'wait_activation_run ws-test "$response_file" "$before_file" "$runs_file" "$output_file"'),
            "verify-dual-workspace-resume-replan-e2e.sh": (
                "validate_command_response", 'validate_command_response "$response_file" ws-test restart request-test e2e_restart'),
            "verify-run-resume-gap-confirmation-e2e.sh": (
                "validate_command_response", 'validate_command_response "$response_file" restart request-test e2e_restart'),
            "verify-real-weather-query.sh": (
                "validate_activation_command_response", 'validate_activation_command_response "$response_file" request-test e2e_restart'),
        }
        with tempfile.TemporaryDirectory(prefix="workorch-activation-test-") as directory:
            path = Path(directory) / "response.json"
            path.write_text(json.dumps(response(reason="unknown")), encoding="utf-8")
            env = dict(os.environ, ROOT_DIR=str(ROOT), response_file=str(path), workspace_id="ws-test",
                       output_file=directory + "/out.json", before_file=directory + "/before.json",
                       runs_file=directory + "/runs.json", BASE_URL="http://unused.invalid",
                       ACTIVATION_INTENT="restart", ACTIVATION_RUN_WAIT_SECONDS="0")
            for script, (name, invocation) in invocations.items():
                for conditional in (False, True):
                    with self.subTest(script=script, conditional=conditional):
                        # Both command substitution (errexit cleared) and an if
                        # condition (errexit ignored) must preserve the failure.
                        call = (f'if run_id="$({invocation})"; then exit 0; else exit $?; fi'
                                if conditional else f'run_id="$({invocation})"')
                        command = ('set -Eeuo pipefail\n'
                                   'curl() { echo UNEXPECTED_POLL >&2; exit 90; }\n'
                                   'get_json() { echo UNEXPECTED_POLL >&2; exit 90; }\n'
                                   'sleep() { echo UNEXPECTED_POLL >&2; exit 90; }\n'
                                   + shell_function(script, name) + call)
                        result = subprocess.run(["bash", "-c", command], env=env, capture_output=True,
                                                text=True, timeout=5)
                        self.assertEqual(result.returncode, 1, result.stderr)
                        self.assertEqual(result.stdout, "")
                        self.assertIn("owner admission outcome/reason", result.stderr)
                        self.assertNotIn("UNEXPECTED_POLL", result.stderr)

    def test_managed_failure_preserves_first_error_and_exit_code(self):
        with tempfile.TemporaryDirectory(prefix="workorch-step-test-") as directory:
            env = dict(os.environ, STEP_LOG_DIR=directory, STEP_LOG_INDEX="0", VERBOSE_LOGS="0",
                       FAILURE_LOG_HEAD_LINES="3", FAILURE_LOG_TAIL_LINES="3")
            payload = 'print("FIRST_CONTRACT_ERROR"); print("snapshot line\\n" * 500); raise SystemExit(23)'
            command = ('set -Eeuo pipefail\n' + shell_function("verify-workorch-managed-full.sh", "run_step")
                       + "run_step python3 -c " + shlex.quote(payload))
            result = subprocess.run(["bash", "-c", command], env=env, capture_output=True, text=True, timeout=5)
            self.assertEqual(result.returncode, 23, result.stderr)
            self.assertIn("--- command output head ---\nFIRST_CONTRACT_ERROR", result.stderr)
            self.assertIn("--- command output tail ---", result.stderr)
            self.assertLess(result.stderr.count("snapshot line\n"), 10)
            logs = list(Path(directory).glob("*.log"))
            self.assertEqual(len(logs), 1)
            self.assertEqual(logs[0].read_text().count("snapshot line\n"), 500)

    def test_managed_failure_indexes_go_errors_between_head_and_tail(self):
        lines = (["head"] + ["unrelated fixture output"] * 500 + [
            "    --- FAIL: TestPublicFullflow (3.86s)",
            "        fullflow_test.go:155: missing sealed implementation work policy",
            "FAIL\tgithub.com/example/integration\t3.873s",
            "panic: test timed out after 15m0s",
            "\trunning tests:",
            "\t\tTestOwnerRepair/after_receipt (7s)",
            "fatal error: concurrent map writes",
            "# github.com/example/build [github.com/example/build.test]",
            "build_test.go:12: undefined: missingContract",
        ] + ["ok github.com/example/other"] * 500 + ["tail"])
        raw = "\n".join(lines) + "\n"
        with tempfile.TemporaryDirectory(prefix="workorch-failure-index-") as directory:
            env = dict(os.environ, STEP_LOG_DIR=directory, STEP_LOG_INDEX="0", VERBOSE_LOGS="0",
                       FAILURE_LOG_HEAD_LINES="1", FAILURE_LOG_TAIL_LINES="1")
            payload = "import sys; sys.stdout.write(" + repr(raw) + "); raise SystemExit(23)"
            command = ('set -Eeuo pipefail\n' + shell_function("verify-workorch-managed-full.sh", "run_step")
                       + "run_step python3 -c " + shlex.quote(payload))
            result = subprocess.run(["bash", "-c", command], env=env, capture_output=True, text=True, timeout=5)
            self.assertEqual(result.returncode, 23, result.stderr)
            self.assertIn("--- command failure index (complete log line numbers) ---", result.stderr)
            for index in range(501, 510):
                self.assertIn(f"{index + 1}:{lines[index]}\n", result.stderr)
            self.assertNotIn("unrelated fixture output\n", result.stderr)
            logs = list(Path(directory).glob("*.log"))
            self.assertEqual(len(logs), 1)
            self.assertEqual(logs[0].read_text(), raw)

    def test_managed_failure_index_is_bounded_without_truncating_log(self):
        raw = "head\n" + "".join(f"--- FAIL: TestFailure{i} (0.00s)\n" for i in range(100)) + "tail\n"
        with tempfile.TemporaryDirectory(prefix="workorch-failure-index-bound-") as directory:
            env = dict(os.environ, STEP_LOG_DIR=directory, STEP_LOG_INDEX="0", VERBOSE_LOGS="0",
                       FAILURE_LOG_HEAD_LINES="1", FAILURE_LOG_TAIL_LINES="1")
            payload = "import sys; sys.stdout.write(" + repr(raw) + "); raise SystemExit(19)"
            command = ('set -Eeuo pipefail\n' + shell_function("verify-workorch-managed-full.sh", "run_step")
                       + "run_step python3 -c " + shlex.quote(payload))
            result = subprocess.run(["bash", "-c", command], env=env, capture_output=True, text=True, timeout=5)
            self.assertEqual(result.returncode, 19, result.stderr)
            self.assertEqual(result.stderr.count(":--- FAIL:"), 80)
            self.assertIn("20 additional failure markers; see complete command log", result.stderr)
            logs = list(Path(directory).glob("*.log"))
            self.assertEqual(len(logs), 1)
            self.assertEqual(logs[0].read_text(), raw)

    def test_failed_verifiers_keep_evidence_without_json_console_dump(self):
        for script in ("verify-llm-workspace-execution.sh", "verify-mcp-stdio-workspace-execution.sh"):
            with self.subTest(script=script), tempfile.TemporaryDirectory(prefix="workorch-evidence-test-") as directory:
                path = Path(directory) / "snapshot.json"
                path.write_text('{"snapshot":"MUST_STAY_IN_ARTIFACT"}', encoding="utf-8")
                env = dict(os.environ, TMP_DIR=directory, KEEP_TMP="0", WATCHDOG_PID="", MOCK_PID="",
                           MOCK_LOG=directory + "/absent.log", BASE_URL="http://unused.invalid")
                command = ('set -Eeuo pipefail\n'
                           + shell_function(script, "cleanup") + shell_function(script, "report_failure")
                           + 'trap report_failure ERR\ntrap cleanup EXIT\nexit_with_failure() { return 19; }\nexit_with_failure')
                result = subprocess.run(["bash", "-c", command], env=env, capture_output=True, text=True, timeout=5)
                self.assertEqual(result.returncode, 19, result.stderr)
                self.assertTrue(path.is_file())
                self.assertIn("keeping verification temp dir:", result.stderr)
                self.assertNotIn("MUST_STAY_IN_ARTIFACT", result.stderr)


if __name__ == "__main__":
    unittest.main()
