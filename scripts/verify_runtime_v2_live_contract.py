#!/usr/bin/env python3
"""Pure Runtime V2 live-verifier assertions with hermetic regressions."""

from __future__ import annotations

import sys
from typing import Any


def external_provider_unavailable_reason(failure_reason: Any) -> str:
    """Classify the terminal transport boundary without accepting the run.

    A real-provider verifier must preserve the first failed contract boundary.
    Once the Runtime V2 transport ledger is exhausted, absence of a candidate,
    dispatch, evidence, or assessment is expected and must not replace the
    external provider diagnosis with derived acceptance noise.
    """

    reason = str(failure_reason or "").strip()
    if not reason.startswith("provider_transport_exhausted:"):
        return ""
    detail = reason.split(":", 1)[1].strip()
    return detail or "provider transport retry budget exhausted"


def bounded_git_sequence_errors(operations: list[dict[str, Any]]) -> list[str]:
    """Validate accepted Git effects, excluding invalidated-dispatch history."""

    git_operations = [item for item in operations if item.get("kind") == "git/write"]
    accepted = [item for item in git_operations if item.get("outcome") == "accepted"]
    expected_variants = ["init", "add", "commit"]
    errors: list[str] = []

    got_variants = [item.get("variantId") for item in accepted]
    if got_variants != expected_variants:
        errors.append(
            f"accepted git variants={got_variants}, want ordered {expected_variants}"
        )
    for variant in expected_variants:
        count = sum(1 for item in accepted if item.get("variantId") == variant)
        if count != 1:
            errors.append(f"accepted git sequence variant {variant} count={count}, want 1")
    for item in accepted:
        if not item.get("requiredOperationRef"):
            errors.append(
                f"accepted git operation {item.get('operationId')} lacks requiredOperationRef"
            )

    hashes = [item.get("capabilityStateHash") for item in accepted]
    if any(not value for value in hashes):
        errors.append("accepted git operation lacks capabilityStateHash")
    elif len(set(hashes)) != len(expected_variants):
        errors.append(
            "accepted git operations must use one fresh capabilityStateHash per durable effect; "
            f"got {hashes}"
        )
    return errors


def runtime_v2_audit_errors(
    audit: Any, workspace_id: str, run_id: str, require_terminal: bool
) -> list[str]:
    """Check the metadata-only Runtime V2 audit chain retained by live runs."""

    if not isinstance(audit, dict):
        return ["missing Runtime V2 audit projection"]
    errors: list[str] = []
    if audit.get("workspaceId") != workspace_id or audit.get("runId") != run_id:
        errors.append(
            "audit scope mismatch "
            f"workspace/run={audit.get('workspaceId')}/{audit.get('runId')}, want {workspace_id}/{run_id}"
        )
    if not audit.get("stateRevision") or not audit.get("definitionRevision") or not audit.get("evidenceRevision"):
        errors.append("audit lacks persisted state, definition, or evidence revision")

    focus = audit.get("focus") or {}
    if not focus.get("id") or not (focus.get("coverageWorkItemIds") or []):
        errors.append("audit lacks current focus or coverage identity")
    coverage = audit.get("coverage") or []
    if not coverage or any(not item.get("id") or not item.get("state") for item in coverage):
        errors.append("audit lacks complete coverage state")

    authority = audit.get("authority") or {}
    for field in (
        "snapshotId",
        "convergenceState",
        "focusId",
        "requiredOutputHash",
        "definitionRevision",
        "evidenceRevision",
        "operationContractRevision",
    ):
        if not authority.get(field):
            errors.append(f"audit authority lacks {field}")
    if authority.get("action"):
        errors.append("audit authority retains retired action field")
    if authority.get("definitionRevision") and authority.get("definitionRevision") != audit.get("definitionRevision"):
        errors.append("audit authority definition revision differs from aggregate definition revision")
    if authority.get("evidenceRevision") and authority.get("evidenceRevision") != audit.get("evidenceRevision"):
        errors.append("audit authority evidence revision differs from aggregate evidence revision")

    attempts = audit.get("candidateAttempts") or []
    if not attempts:
        errors.append("audit has no candidate attempt")
    seen_attempts: set[tuple[str, int]] = set()
    for item in attempts:
        if not item.get("authorityContractId") or not item.get("authoritySnapshotId") or not item.get("state"):
            errors.append("audit candidate attempt lacks authority identity or state")
        if not item.get("stablePrefixHash"):
            errors.append("audit candidate attempt lacks stablePrefixHash")
        authority_snapshot_id = item.get("authoritySnapshotId")
        # Attempt zero is omitted by the read-model's omitempty JSON tag.
        attempt = item.get("attempt", 0)
        if not isinstance(authority_snapshot_id, str) or not authority_snapshot_id or not isinstance(attempt, int) or attempt < 0:
            errors.append("audit candidate attempt lacks authority snapshot or valid numeric attempt")
            continue
        attempt_key = (authority_snapshot_id, attempt)
        if attempt_key in seen_attempts:
            errors.append(
                "audit has duplicate candidate attempt "
                f"authoritySnapshotId/attempt={authority_snapshot_id}/{attempt}"
            )
        seen_attempts.add(attempt_key)

    dispatches = audit.get("dispatches") or []
    seen_dispatches: set[str] = set()
    for item in dispatches:
        dispatch_id = item.get("id")
        if not dispatch_id or not item.get("authoritySnapshotId") or not item.get("state"):
            errors.append("audit dispatch lacks identity, authority snapshot, or state")
            continue
        if dispatch_id in seen_dispatches:
            errors.append(f"audit has duplicate dispatch {dispatch_id}")
        seen_dispatches.add(dispatch_id)

    grounding = audit.get("grounding") or {}
    observations = grounding.get("prerequisiteObservations") or []
    if not observations:
        errors.append("audit lacks owner prerequisite observations")
    for item in observations:
        if not item.get("prerequisiteId") or not item.get("ownerModuleId") or not item.get("observationId"):
            errors.append("audit prerequisite observation lacks owner identity")
        for dependency in item.get("dependencies") or []:
            if not all(dependency.get(field) for field in ("domain", "ownerModuleId", "subjectRef", "revision")):
                errors.append("audit prerequisite freshness dependency is incomplete")
    selection = grounding.get("targetSelection")
    if selection is not None:
        for field in ("snapshotId", "ownerModuleId", "targetRole", "selectorRef", "selectorOwnerRef", "observedRevision"):
            if not selection.get(field):
                errors.append(f"audit target selection lacks {field}")
    for item in grounding.get("targetSnapshots") or []:
        if not item.get("targetRef") or not item.get("contentHash") or not item.get("contentState"):
            errors.append("audit target snapshot lacks identity or content state")

    ceiling = grounding.get("providerCeiling")
    if not isinstance(ceiling, dict):
        errors.append("audit lacks sealed provider ceiling")
    else:
        for field in (
            "providerId", "modelId", "contextWindowTokens", "providerMaxOutputTokens",
            "effectiveMaxOutputTokens", "maxInputTokens", "maxBytes", "capabilityRevision", "configurationRevision",
        ):
            if not ceiling.get(field):
                errors.append(f"audit provider ceiling lacks {field}")
        if (
            isinstance(ceiling.get("contextWindowTokens"), int)
            and isinstance(ceiling.get("effectiveMaxOutputTokens"), int)
            and isinstance(ceiling.get("maxInputTokens"), int)
            and ceiling["maxInputTokens"] + ceiling["effectiveMaxOutputTokens"] > ceiling["contextWindowTokens"]
        ):
            errors.append("audit provider ceiling input plus output exceeds context window")

    repair = audit.get("repair") or {}
    for field in ("repairAttempts", "validationOnlyRetries", "noProgressCount", "terminalDeliveryAttempts"):
        if not isinstance(repair.get(field), int) or repair.get(field) < 0:
            errors.append(f"audit repair counter {field} is invalid")

    if require_terminal:
        terminal = audit.get("terminal")
        if not isinstance(terminal, dict):
            errors.append("completed Run lacks terminal audit record")
        elif not terminal.get("code"):
            errors.append("terminal audit lacks terminal code")
        assessments = audit.get("assessments") or []
        if not assessments:
            errors.append("completed Run lacks assessment audit lineage")
        elif not any(item.get("disposition") == "satisfied" for item in assessments):
            errors.append("completed Run has no satisfied assessment audit delta")
    return errors


def provider_ceiling_trace_errors(audit: Any, traces: Any) -> list[str]:
    """Prove at least one provider request used the sealed model ceiling."""

    if not isinstance(audit, dict):
        return ["missing Runtime V2 audit projection"]
    ceiling = ((audit.get("grounding") or {}).get("providerCeiling"))
    if not isinstance(ceiling, dict):
        return ["missing sealed provider ceiling"]
    if not isinstance(traces, list) or not traces:
        return ["missing provider usage audit trace"]
    for trace in traces:
        if not isinstance(trace, dict):
            continue
        budget = trace.get("contextBudget") or {}
        if not isinstance(budget, dict):
            continue
        provider_id = budget.get("providerId") or trace.get("providerId")
        model_id = budget.get("modelId") or trace.get("model")
        if provider_id != ceiling.get("providerId") or model_id != ceiling.get("modelId"):
            continue
        if budget.get("capabilityRevision") != ceiling.get("capabilityRevision") or budget.get("configurationRevision") != ceiling.get("configurationRevision"):
            continue
        if budget.get("maxBytes") != ceiling.get("maxBytes") or budget.get("maxRequestBytes") != ceiling.get("maxBytes"):
            continue
        if budget.get("maxTokens") != ceiling.get("maxInputTokens") or budget.get("estimatedOutputTokens") != ceiling.get("effectiveMaxOutputTokens"):
            continue
        if budget.get("overBudget"):
            return ["provider context-budget trace submitted over the sealed ceiling"]
        return []
    return ["no provider context-budget trace matches the sealed model ceiling"]


def runtime_v2_completion_sequence_errors(audit: Any, require_focus_advance: bool) -> list[str]:
    """Check persisted evidence/assessment lineage, never inferred UI activity."""

    if not isinstance(audit, dict):
        return ["missing Runtime V2 audit projection"]
    evidence = audit.get("acceptedEvidence") or []
    kinds = {item.get("kind") for item in evidence if isinstance(item, dict)}
    errors: list[str] = []
    if "state_change" not in kinds:
        errors.append("completion audit lacks accepted durable state-change evidence")
    if "validation" not in kinds:
        errors.append("completion audit lacks accepted validation evidence")
    if not any(item.get("disposition") == "satisfied" for item in (audit.get("assessments") or []) if isinstance(item, dict)):
        errors.append("completion audit lacks satisfied assessment delta")
    if require_focus_advance and not (audit.get("completedFocusIds") or []):
        errors.append("bounded completion audit lacks persisted focus advance")
    if audit.get("remainingFocusIds"):
        errors.append("completed Run retains remaining focus queue")
    return errors


def _accepted(variant: str, snapshot_hash: str, operation_id: str) -> dict[str, Any]:
    return {
        "kind": "git/write",
        "variantId": variant,
        "outcome": "accepted",
        "capabilityStateHash": snapshot_hash,
        "requiredOperationRef": operation_id,
        "operationId": f"op-{variant}",
    }


def _self_test() -> None:
    if external_provider_unavailable_reason("target_grounding_unresolved: no target"):
        raise AssertionError("non-provider terminal was classified as external provider unavailable")
    if external_provider_unavailable_reason("provider_transport_exhausted: upstream 502") != "upstream 502":
        raise AssertionError("provider transport terminal was not classified")

    # Unexecuted add/commit operations from invalidated dispatches are retained
    # for lineage.  They are not duplicate durable effects.
    valid = [
        _accepted("init", "snapshot-init", "repo-init"),
        {"kind": "git/write", "variantId": "add", "capabilityStateHash": "snapshot-init"},
        {"kind": "git/write", "variantId": "commit", "capabilityStateHash": "snapshot-init"},
        _accepted("add", "snapshot-add", "stage-changes"),
        {"kind": "git/write", "variantId": "commit", "capabilityStateHash": "snapshot-add"},
        _accepted("commit", "snapshot-commit", "create-commit"),
    ]
    if errors := bounded_git_sequence_errors(valid):
        raise AssertionError(f"valid refreshed Git chain rejected: {errors}")

    duplicate = valid + [_accepted("commit", "snapshot-extra", "create-commit")]
    if not bounded_git_sequence_errors(duplicate):
        raise AssertionError("duplicate accepted Git commit was accepted")

    reused_snapshot = [
        _accepted("init", "snapshot-one", "repo-init"),
        _accepted("add", "snapshot-one", "stage-changes"),
        _accepted("commit", "snapshot-one", "create-commit"),
    ]
    if not bounded_git_sequence_errors(reused_snapshot):
        raise AssertionError("reused Git capability-state snapshot was accepted")

    valid_audit = {
        "workspaceId": "ws-1", "runId": "run-1", "stateRevision": 9,
        "definitionRevision": "definition-1", "evidenceRevision": "evidence-2",
        "focus": {"id": "focus-1", "coverageWorkItemIds": ["coverage-1"]},
        "coverage": [{"id": "coverage-1", "state": "satisfied"}],
        "authority": {
            "snapshotId": "authority-1", "convergenceState": "complete", "focusId": "focus-1", "requiredOutputHash": "output-1",
            "definitionRevision": "definition-1", "evidenceRevision": "evidence-2", "operationContractRevision": "operations-1",
        },
        "candidateAttempts": [{"authorityContractId": "contract-1", "authoritySnapshotId": "authority-1", "state": "accepted", "stablePrefixHash": "prefix-1"}],
        "dispatches": [{"id": "dispatch-1", "authoritySnapshotId": "authority-1", "state": "committed"}],
        "grounding": {"prerequisiteObservations": [{"prerequisiteId": "manifest", "ownerModuleId": "blocks/file", "observationId": "obs-1", "dependencies": [{"domain": "workspace_tree", "ownerModuleId": "blocks/file", "subjectRef": "workspace", "revision": "tree-1"}]}], "providerCeiling": {"providerId": "ai-proxy", "modelId": "model-1", "contextWindowTokens": 128000, "providerMaxOutputTokens": 16384, "effectiveMaxOutputTokens": 16384, "maxInputTokens": 111616, "maxBytes": 446464, "capabilityRevision": "capability-1", "configurationRevision": "configuration-1"}},
        "repair": {"repairAttempts": 0, "validationOnlyRetries": 0, "noProgressCount": 0, "terminalDeliveryAttempts": 0},
        "assessments": [{"id": "assessment-1", "disposition": "satisfied"}],
        "terminal": {"code": "runtime_completed"},
    }
    if errors := runtime_v2_audit_errors(valid_audit, "ws-1", "run-1", True):
        raise AssertionError(f"valid Runtime V2 audit rejected: {errors}")
    valid_trace = {"providerId": "ai-proxy", "model": "model-1", "contextBudget": {"providerId": "ai-proxy", "modelId": "model-1", "capabilityRevision": "capability-1", "configurationRevision": "configuration-1", "maxBytes": 446464, "maxRequestBytes": 446464, "maxTokens": 111616, "estimatedOutputTokens": 16384, "overBudget": False}}
    if errors := provider_ceiling_trace_errors(valid_audit, [valid_trace]):
        raise AssertionError(f"matching sealed provider ceiling trace rejected: {errors}")
    invalid_trace = dict(valid_trace)
    invalid_trace["contextBudget"] = dict(valid_trace["contextBudget"])
    invalid_trace["contextBudget"]["estimatedOutputTokens"] = 4096
    if not provider_ceiling_trace_errors(valid_audit, [invalid_trace]):
        raise AssertionError("fixed output-cap trace was accepted")
    valid_audit["acceptedEvidence"] = [{"kind": "state_change"}, {"kind": "validation"}]
    valid_audit["completedFocusIds"] = ["focus-1"]
    valid_audit["remainingFocusIds"] = []
    if errors := runtime_v2_completion_sequence_errors(valid_audit, True):
        raise AssertionError(f"valid completion sequence rejected: {errors}")
    invalid_audit = dict(valid_audit)
    invalid_audit["candidateAttempts"] = [{"authorityContractId": "contract-1"}]
    if not runtime_v2_audit_errors(invalid_audit, "ws-1", "run-1", True):
        raise AssertionError("incomplete Runtime V2 audit was accepted")
    legacy_action_audit = dict(valid_audit)
    legacy_action_audit["authority"] = dict(valid_audit["authority"])
    legacy_action_audit["authority"]["action"] = "complete"
    if not runtime_v2_audit_errors(legacy_action_audit, "ws-1", "run-1", True):
        raise AssertionError("Runtime V2 audit retaining retired action was accepted")
    duplicate_attempt_audit = dict(valid_audit)
    duplicate_attempt_audit["candidateAttempts"] = valid_audit["candidateAttempts"] * 2
    if not runtime_v2_audit_errors(duplicate_attempt_audit, "ws-1", "run-1", True):
        raise AssertionError("duplicate Runtime V2 candidate attempt was accepted")

    print("runtime-v2 live contract self-test passed")


if __name__ == "__main__":
    if sys.argv[1:] != ["--self-test"]:
        raise SystemExit("usage: verify_runtime_v2_live_contract.py --self-test")
    _self_test()
