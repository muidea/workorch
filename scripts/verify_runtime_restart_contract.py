#!/usr/bin/env python3
"""Pure contract checks for the deployed Runtime SM-4 restart verifier."""

from __future__ import annotations

import sys
from typing import Any


EXPECTED_FIXTURE_CLASSES = frozenset(
    {
        "active_no_dispatch",
        "active_sealed_dispatch",
        "terminal_delivery_pending",
    }
)
TERMINAL_RUN_STATES = frozenset({"completed", "failed", "cancelled", "superseded"})
GOVERNED_WAIT_STATES = frozenset({"blocked_on_decision", "blocked_on_approval"})


def fixture_manifest_errors(manifest: Any) -> list[str]:
    """Return contract errors without reading a deployed instance.

    Each recovery class has to identify a distinct workspace.  The deployed
    snapshot endpoint is workspace-scoped, so allowing the same workspace in
    multiple classes would not prove independent recovery paths.
    """

    if not isinstance(manifest, dict):
        return ["fixture manifest must be a JSON object"]
    fixtures = manifest.get("fixtures")
    if not isinstance(fixtures, list):
        return ["fixture manifest fixtures must be an array"]

    errors: list[str] = []
    classes: list[str] = []
    seen_workspaces: set[str] = set()
    seen_runs: set[tuple[str, str]] = set()
    for index, item in enumerate(fixtures):
        if not isinstance(item, dict):
            errors.append(f"fixture[{index}] must be an object")
            continue
        fixture_class = item.get("class")
        if not isinstance(fixture_class, str) or not fixture_class.strip():
            errors.append(f"fixture[{index}] requires non-empty class")
        else:
            classes.append(fixture_class)
        workspace_id = item.get("workspaceId")
        run_id = item.get("runId")
        if not isinstance(workspace_id, str) or not workspace_id.strip():
            errors.append(f"fixture[{index}] requires non-empty workspaceId")
            continue
        if not isinstance(run_id, str) or not run_id.strip():
            errors.append(f"fixture[{index}] requires non-empty runId")
            continue
        if workspace_id in seen_workspaces:
            errors.append(f"fixture[{index}] reuses workspaceId {workspace_id!r}")
        seen_workspaces.add(workspace_id)
        identity = (workspace_id, run_id)
        if identity in seen_runs:
            errors.append(f"fixture[{index}] duplicates workspaceId/runId {workspace_id!r}/{run_id!r}")
        seen_runs.add(identity)
        errors.extend(_fixture_pre_restart_errors(index, item, fixture_class))

    if len(fixtures) != len(EXPECTED_FIXTURE_CLASSES) or set(classes) != EXPECTED_FIXTURE_CLASSES:
        errors.append(
            "fixture manifest must contain exactly active_no_dispatch, "
            "active_sealed_dispatch, terminal_delivery_pending"
        )
    if len(classes) != len(set(classes)):
        errors.append("fixture manifest contains duplicate recovery classes")
    return errors


def _fixture_pre_restart_errors(index: int, fixture: dict[str, Any], fixture_class: Any) -> list[str]:
    """Validate the persisted-before-restart facts required for each class."""

    prefix = f"fixture[{index}].preRestart"
    pre_restart = fixture.get("preRestart")
    if not isinstance(pre_restart, dict):
        return [f"{prefix} must be an object"]
    errors: list[str] = []
    for field in ("stateRevision", "candidateAttemptCount", "dispatchCount"):
        value = pre_restart.get(field)
        if not isinstance(value, int) or value < 0:
            errors.append(f"{prefix}.{field} must be a non-negative integer")
    authority_snapshot_id = pre_restart.get("authoritySnapshotId")
    if not isinstance(authority_snapshot_id, str) or not authority_snapshot_id.strip():
        errors.append(f"{prefix}.authoritySnapshotId must be non-empty")

    if fixture_class == "active_no_dispatch" and pre_restart.get("dispatchCount") != 0:
        errors.append(f"{prefix}.dispatchCount must be 0 for active_no_dispatch")
    if fixture_class == "active_sealed_dispatch":
        dispatch_id = pre_restart.get("sealedDispatchId")
        if not isinstance(dispatch_id, str) or not dispatch_id.strip():
            errors.append(f"{prefix}.sealedDispatchId must be non-empty for active_sealed_dispatch")
        executions = pre_restart.get("operationExecutions")
        if (
            not isinstance(executions, list)
            or not executions
            or any(
                not isinstance(value, dict)
                or not isinstance(value.get("executionKey"), str)
                or not value["executionKey"].strip()
                or not isinstance(value.get("fence"), int)
                or value["fence"] < 1
                or value.get("executionState") not in {"sealed", "claimed"}
                for value in executions
            )
            or len({value["executionKey"] for value in executions if isinstance(value, dict) and isinstance(value.get("executionKey"), str)}) != len(executions)
        ):
            errors.append(
                f"{prefix}.operationExecutions must be unique sealed/claimed executionKey/fence records for active_sealed_dispatch"
            )
    if fixture_class == "terminal_delivery_pending":
        terminal_code = pre_restart.get("terminalCode")
        if not isinstance(terminal_code, str) or not terminal_code.strip():
            errors.append(f"{prefix}.terminalCode must be non-empty for terminal_delivery_pending")
    return errors


def audit_transition_identity_errors(audit: Any) -> list[str]:
    """Check the identity invariants observable after SM-4 restart recovery."""

    if not isinstance(audit, dict):
        return ["missing Runtime audit projection"]
    errors: list[str] = []
    seen_attempts: set[tuple[str, int]] = set()
    for item in audit.get("candidateAttempts") or []:
        if not isinstance(item, dict):
            errors.append("audit candidate attempt must be an object")
            continue
        authority_snapshot_id = item.get("authoritySnapshotId")
        # Attempt zero is omitted by the read-model's omitempty JSON tag.
        attempt = item.get("attempt", 0)
        if not isinstance(authority_snapshot_id, str) or not authority_snapshot_id.strip() or not isinstance(attempt, int) or attempt < 0:
            errors.append("audit candidate attempt lacks authority snapshot or valid numeric attempt")
            continue
        key = (authority_snapshot_id, attempt)
        if key in seen_attempts:
            errors.append(f"duplicate candidate attempt authoritySnapshotId/attempt={key[0]}/{key[1]}")
        seen_attempts.add(key)
    seen_dispatches: set[str] = set()
    for item in audit.get("dispatches") or []:
        if not isinstance(item, dict) or not isinstance(item.get("id"), str) or not item["id"].strip():
            errors.append("audit dispatch lacks id")
            continue
        if item["id"] in seen_dispatches:
            errors.append(f"duplicate audit dispatch {item['id']}")
        seen_dispatches.add(item["id"])
    return errors


def fixture_final_errors(
    fixture: dict[str, Any],
    run_state: Any,
    active_run_id: Any,
    audit: Any,
    operations: Any,
) -> list[str]:
    """Validate class-specific recovery outcomes from metadata-only projections."""

    fixture_class = fixture.get("class")
    state = str(run_state or "").lower()
    active = str(active_run_id or "")
    errors = audit_transition_identity_errors(audit)
    terminal = state in TERMINAL_RUN_STATES
    governed_wait = state in GOVERNED_WAIT_STATES
    if not (terminal or governed_wait):
        errors.append(f"runState={run_state!r}, want terminal or governed wait")
    if terminal and active:
        errors.append(f"terminal run still owns workspace activeRunId={active}")
    if not isinstance(audit, dict):
        return errors
    pre_restart = fixture["preRestart"]

    if fixture_class == "active_sealed_dispatch":
        expected_dispatch_id = pre_restart["sealedDispatchId"]
        dispatches = audit.get("dispatches") or []
        if not any(isinstance(item, dict) and item.get("id") == expected_dispatch_id for item in dispatches):
            errors.append(f"sealed dispatch {expected_dispatch_id!r} is absent after recovery")
        if not isinstance(operations, list):
            errors.append("sealed-dispatch recovery lacks operation projection")
        else:
            for prior in pre_restart["operationExecutions"]:
                execution_key = prior["executionKey"]
                fence = prior["fence"]
                matches = [
                    item for item in operations
                    if isinstance(item, dict)
                    and item.get("executionKey") == execution_key
                    and item.get("fence") == fence
                ]
                if len(matches) != 1:
                    errors.append(f"sealed executionKey/fence {execution_key!r}/{fence} has {len(matches)} projected records, want 1")
                    continue
                recovered = matches[0]
                # SIGKILL occurred after the owner claim but before a receipt.
                # The sealed idempotency policy provides no replay authority,
                # therefore recovery must retain the exact execution identity
                # and converge it to outcome_unknown rather than execute the
                # released command a second time.
                if recovered.get("executionState") != "outcome_unknown" or recovered.get("outcomeUnknown") is not True:
                    errors.append(
                        f"sealed executionKey/fence {execution_key!r}/{fence} was not preserved as outcome_unknown"
                    )
    elif fixture_class == "active_no_dispatch":
        if not terminal:
            errors.append("active_no_dispatch must converge to a terminal Run, not governance wait")
        if len(audit.get("candidateAttempts") or []) != pre_restart["candidateAttemptCount"]:
            errors.append("active_no_dispatch replayed or recomputed candidate attempts")
        if len(audit.get("dispatches") or []) != pre_restart["dispatchCount"]:
            errors.append("active_no_dispatch replayed or recomputed dispatches")
    elif fixture_class == "terminal_delivery_pending":
        if not terminal:
            errors.append("terminal_delivery_pending must converge to a terminal Run, not governance wait")
        if audit.get("authority", {}).get("snapshotId") != pre_restart["authoritySnapshotId"]:
            errors.append("terminal delivery changed the sealed authority snapshot")
        terminal_audit = audit.get("terminal") or {}
        if terminal_audit.get("code") != pre_restart["terminalCode"]:
            errors.append("terminal delivery changed the sealed terminal code")
        if len(audit.get("candidateAttempts") or []) != pre_restart["candidateAttemptCount"]:
            errors.append("terminal delivery recomputed candidate attempts")
        if len(audit.get("dispatches") or []) != pre_restart["dispatchCount"]:
            errors.append("terminal delivery recomputed dispatches")
    return errors


def _self_test() -> None:
    valid = {
        "fixtures": [
            {"class": "active_no_dispatch", "workspaceId": "ws-a", "runId": "run-a", "preRestart": {"stateRevision": 1, "candidateAttemptCount": 0, "dispatchCount": 0, "authoritySnapshotId": "authority-a"}},
            {"class": "active_sealed_dispatch", "workspaceId": "ws-b", "runId": "run-b", "preRestart": {"stateRevision": 2, "candidateAttemptCount": 1, "dispatchCount": 1, "authoritySnapshotId": "authority-b", "sealedDispatchId": "dispatch-b", "operationExecutions": [{"executionKey": "exec-b", "fence": 1, "executionState": "claimed"}]}},
            {"class": "terminal_delivery_pending", "workspaceId": "ws-c", "runId": "run-c", "preRestart": {"stateRevision": 3, "candidateAttemptCount": 1, "dispatchCount": 1, "authoritySnapshotId": "authority-c", "terminalCode": "runtime_completed"}},
        ]
    }
    if errors := fixture_manifest_errors(valid):
        raise AssertionError(f"valid restart fixture manifest rejected: {errors}")

    duplicate_workspace = {
        "fixtures": [
            {"class": "active_no_dispatch", "workspaceId": "ws-a", "runId": "run-a", "preRestart": {"stateRevision": 1, "candidateAttemptCount": 0, "dispatchCount": 0, "authoritySnapshotId": "authority-a"}},
            {"class": "active_sealed_dispatch", "workspaceId": "ws-a", "runId": "run-b", "preRestart": {"stateRevision": 2, "candidateAttemptCount": 1, "dispatchCount": 1, "authoritySnapshotId": "authority-b", "sealedDispatchId": "dispatch-b", "operationExecutionKeys": ["exec-b"]}},
            {"class": "terminal_delivery_pending", "workspaceId": "ws-c", "runId": "run-c", "preRestart": {"stateRevision": 3, "candidateAttemptCount": 1, "dispatchCount": 1, "authoritySnapshotId": "authority-c", "terminalCode": "runtime_completed"}},
        ]
    }
    if not fixture_manifest_errors(duplicate_workspace):
        raise AssertionError("duplicate restart fixture workspace was accepted")

    missing_class = {
        "fixtures": [
            {"class": "active_no_dispatch", "workspaceId": "ws-a", "runId": "run-a", "preRestart": {"stateRevision": 1, "candidateAttemptCount": 0, "dispatchCount": 0, "authoritySnapshotId": "authority-a"}},
            {"class": "active_no_dispatch", "workspaceId": "ws-b", "runId": "run-b", "preRestart": {"stateRevision": 2, "candidateAttemptCount": 0, "dispatchCount": 0, "authoritySnapshotId": "authority-b"}},
            {"class": "terminal_delivery_pending", "workspaceId": "ws-c", "runId": "run-c", "preRestart": {"stateRevision": 3, "candidateAttemptCount": 1, "dispatchCount": 1, "authoritySnapshotId": "authority-c", "terminalCode": "runtime_completed"}},
        ]
    }
    if not fixture_manifest_errors(missing_class):
        raise AssertionError("incomplete restart fixture classes were accepted")

    terminal_fixture = valid["fixtures"][2]
    terminal_audit = {
        "authority": {"snapshotId": "authority-c"},
        "terminal": {"code": "runtime_completed"},
        "candidateAttempts": [{"authoritySnapshotId": "authority-c", "attempt": 0}],
        "dispatches": [{"id": "dispatch-c"}],
    }
    if errors := fixture_final_errors(terminal_fixture, "completed", "", terminal_audit, []):
        raise AssertionError(f"valid terminal delivery recovery rejected: {errors}")
    replayed_terminal_audit = dict(terminal_audit)
    replayed_terminal_audit["candidateAttempts"] = terminal_audit["candidateAttempts"] + [{"authoritySnapshotId": "authority-next", "attempt": 0}]
    if not fixture_final_errors(terminal_fixture, "completed", "", replayed_terminal_audit, []):
        raise AssertionError("terminal delivery candidate replay was accepted")

    sealed_fixture = valid["fixtures"][1]
    sealed_audit = {"candidateAttempts": [{"authoritySnapshotId": "authority-b", "attempt": 0}], "dispatches": [{"id": "dispatch-b"}]}
    sealed_operations = [{"executionKey": "exec-b", "fence": 1, "executionState": "outcome_unknown", "outcomeUnknown": True}]
    if errors := fixture_final_errors(sealed_fixture, "blocked_on_decision", "run-b", sealed_audit, sealed_operations):
        raise AssertionError(f"sealed dispatch outcome-unknown recovery rejected: {errors}")
    if not fixture_final_errors(sealed_fixture, "blocked_on_decision", "run-b", sealed_audit, []):
        raise AssertionError("missing sealed execution projection was accepted")

    print("runtime restart contract self-test passed")


if __name__ == "__main__":
    if sys.argv[1:] != ["--self-test"]:
        raise SystemExit("usage: verify_runtime_restart_contract.py --self-test")
    _self_test()
