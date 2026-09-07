"""Progress checks do not need a deployed service or write any runtime state."""

import ast
import copy
from pathlib import Path
import unittest

from observe_runtime_progress import evaluate_workspace_progress, runtime_progress_facts


def snapshot(state="active"):
    return {"workspace": {"id": "ws", "activeRunId": "run-2"},
            "run": {"id": "run-2", "workspaceId": "ws", "state": state},
            "results": [], "acceptance": {"audit": {"workspaceId": "ws", "runId": "run-2",
                "stateRevision": 1, "coverage": [{"id": "cwi-1", "state": "open"}]}}}


def summary(value):
    return {"workspaceId": "ws", "workspace": value["workspace"], "run": value["run"],
            "runtimeProgress": runtime_progress_facts(value), "heartbeat": {"latestEffectiveProgressAt": "t1"}}


class RuntimeProgressTest(unittest.TestCase):
    def test_shell_observer_summary_uses_scoped_facts_and_two_samples(self):
        # Load only pure helpers, never source/run the live script main body.
        script = Path(__file__).with_name("observe-workorch-runtime-closure.sh").read_text(encoding="utf-8")
        block = script.split('python3 - "$TMP_DIR" "$WORKSPACE_IDS"', 1)[1].split("<<'PY'\n", 1)[1].split("\nPY\n", 1)[0]
        names = {"workspace_live_summary", "list_value", "state_counts", "compact_item_refs", "latest_by_time", "bounded_text"}
        tree = ast.parse(block)
        pure = ast.Module(body=[node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name in names], type_ignores=[])
        namespace = {"runtime_progress_facts": runtime_progress_facts}
        exec(compile(pure, "observer-pure-helpers", "exec"), namespace)
        value = snapshot()
        value["results"] = [{"id": "old", "runId": "run-1", "workspaceId": "ws", "state": "accepted"}]
        value["decisions"] = [{"id": "old-ticket", "runId": "run-1", "workspaceId": "ws", "state": "open"}]
        value["approvals"] = [{"id": "old-approval", "runId": "run-1", "workspaceId": "ws", "state": "pending"}]
        observed = namespace["workspace_live_summary"]("ws", {"snapshot": value})
        self.assertEqual(observed["counts"]["acceptedResults"], 1)
        self.assertEqual(observed["counts"]["currentRunAcceptedResults"], 0)
        self.assertEqual(observed["counts"]["openDecisions"], 0)
        self.assertEqual(observed["counts"]["pendingApprovals"], 0)
        self.assertFalse(observed["latestResult"]["id"])
        self.assertFalse(evaluate_workspace_progress(observed, observed)["positiveProgress"])
        self.assertIn("evaluate_workspace_progress(live_summary, before_summary, min_goal_accepted_results)", block)
        self.assertLess(script.index('snapshot-before-$workspace_id.json'), script.index('sse_pid=$!'))

    def test_active_heartbeat_revision_and_llm_are_activity_not_progress(self):
        before = summary(snapshot())
        after = copy.deepcopy(before)
        after["heartbeat"]["latestEffectiveProgressAt"] = "t2"
        after["heartbeat"]["latestLLMUsageAt"] = "t2"
        after["runtimeProgress"]["stateRevision"] = 10
        result = evaluate_workspace_progress(after, before)
        self.assertTrue(result["activityObserved"])
        self.assertFalse(result["positiveProgress"])
        self.assertFalse(result["pass"])
        self.assertEqual(result["mode"], "active_without_observed_progress")

    def test_owner_effect_evidence_and_cwi_deltas_before_final_result(self):
        before = summary(snapshot())
        after = snapshot()
        audit = after["acceptance"]["audit"]
        audit["acceptedEvidence"] = [{"id": "evidence-1"}]
        audit["operationReceipts"] = [{"executionKey": "exec-1", "outcome": "accepted", "changed": True,
                                        "observedEffectScope": "workspace_durable", "fence": 1}]
        audit["coverage"][0]["state"] = "satisfied"
        result = evaluate_workspace_progress(summary(after), before)
        self.assertTrue(result["pass"])
        self.assertTrue(result["positiveProgress"])
        self.assertFalse(result["completed"])
        self.assertEqual(result["delta"]["satisfiedCWIIds"], ["cwi-1"])
        self.assertEqual(result["delta"]["durableEffectExecutionKeys"], ["exec-1"])
        audit["operationReceipts"][0]["fence"] = 2
        replay = evaluate_workspace_progress(summary(after), summary(after))
        self.assertFalse(replay["observedProgress"])

    def test_historical_results_wrong_scope_and_run_replacement_are_not_deltas(self):
        before = summary(snapshot())
        for field in ("runId", "workspaceId"):
            value = snapshot("completed")
            value["results"] = [{"id": "result-old", "state": "accepted", "workspaceId": "ws", "runId": "run-2"}]
            value["results"][0][field] = "another"
            self.assertEqual(runtime_progress_facts(value)["acceptedResultIds"], [])
            self.assertFalse(evaluate_workspace_progress(summary(value), before)["pass"])
        after = summary(snapshot())
        after["run"]["id"] = "run-3"
        after["runtimeProgress"]["acceptedEvidenceIds"] = ["evidence-1"]
        self.assertFalse(evaluate_workspace_progress(after, before)["comparisonAvailable"])
        after = copy.deepcopy(before)
        after["runtimeProgress"]["stateRevision"] = 0
        after["runtimeProgress"]["acceptedEvidenceIds"] = ["stale-evidence"]
        self.assertFalse(evaluate_workspace_progress(after, before)["comparisonAvailable"])
        value = snapshot()
        value["acceptance"]["audit"]["runId"] = "run-1"
        self.assertFalse(runtime_progress_facts(value)["auditScopeValid"])

    def test_repair_effect_without_validation_or_cwi_delta_is_not_positive(self):
        before = summary(snapshot())
        value = snapshot()
        audit = value["acceptance"]["audit"]
        audit["stateRevision"] = 2
        audit["acceptedEvidence"] = [{"id": "repair", "kind": "state_change", "coverageWorkItemIds": []}]
        audit["operationReceipts"] = [{"executionKey": "repair-key", "outcome": "accepted", "changed": True,
                                       "observedEffectScope": "workspace_durable"}]
        result = evaluate_workspace_progress(summary(value), before)
        self.assertTrue(result["activityObserved"])
        self.assertTrue(result["durableEffectObserved"])
        self.assertFalse(result["acceptanceProgressObserved"])
        self.assertFalse(result["positiveProgress"])
        self.assertEqual(result["mode"], "effects_without_acceptance_progress")
        audit["acceptedEvidence"].append({"id": "validation", "kind": "validation", "coverageWorkItemIds": ["cwi-1"]})
        result = evaluate_workspace_progress(summary(value), before)
        self.assertTrue(result["acceptanceProgressObserved"])
        self.assertTrue(result["positiveProgress"])

    def test_effects_do_not_override_failure_or_stall(self):
        before = summary(snapshot())
        for state in ("failed", "cancelled", "superseded", "active"):
            after = summary(snapshot(state))
            after["runtimeProgress"]["acceptedEvidenceIds"] = ["evidence-1"]
            if state == "active":
                after["heartbeat"]["ineffectiveRetry"] = True
            result = evaluate_workspace_progress(after, before)
            self.assertTrue(result["observedProgress"])
            self.assertFalse(result["positiveProgress"])
            self.assertFalse(result["pass"])

    def test_completion_requires_current_run_accepted_result_and_coverage(self):
        value = snapshot("completed")
        value["results"] = [{"id": "result-2", "workspaceId": "ws", "runId": "run-2", "state": "accepted"}]
        self.assertFalse(evaluate_workspace_progress(summary(value))["completed"])
        value["acceptance"]["audit"]["coverage"][0]["state"] = "satisfied"
        result = evaluate_workspace_progress(summary(value))
        self.assertTrue(result["completed"])
        self.assertTrue(result["pass"])

    def test_governed_blocker_is_not_positive_progress(self):
        after = summary(snapshot("blocked_on_decision"))
        after["counts"] = {"openDecisions": 1}
        result = evaluate_workspace_progress(after, after)
        self.assertTrue(result["pass"])
        self.assertFalse(result["positiveProgress"])
        after["convergenceStatus"] = {"decisionEligibilityDenied": True}
        self.assertFalse(evaluate_workspace_progress(after, after)["pass"])

    def test_read_only_rejected_and_temporary_receipts_do_not_count_as_effects(self):
        value = snapshot()
        receipts = [{"executionKey": "exec", "outcome": "accepted", "changed": False, "observedEffectScope": "workspace_durable"},
                    {"executionKey": "exec", "outcome": "rejected", "changed": True, "observedEffectScope": "workspace_durable"},
                    {"executionKey": "exec", "outcome": "accepted", "changed": True, "observedEffectScope": "temporary"}]
        value["acceptance"]["audit"]["operationReceipts"] = receipts
        self.assertEqual(runtime_progress_facts(value)["durableEffectExecutionKeys"], [])


if __name__ == "__main__":
    unittest.main()
