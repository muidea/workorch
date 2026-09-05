#!/usr/bin/env python3
"""Pure Runtime live-verifier assertions with hermetic regressions."""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path
from typing import Any


def external_provider_unavailable_reason(failure_reason: Any) -> str:
    """Classify the terminal transport boundary without accepting the run.

    A real-provider verifier must preserve the first failed contract boundary.
    Once the Runtime transport ledger is exhausted, absence of a candidate,
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


def has_canonical_satisfied_assessment(assessments: Any) -> bool:
    """Return whether the owner-derived assessment outcome closes coverage.

    `claimedDisposition` is preserved producer diagnostic.  The Runtime
    reducer alone derives `canonicalDisposition`; the verifier must not revive
    the retired unqualified `disposition` field or treat an LLM claim as a
    completion authority.
    """

    return any(
        isinstance(item, dict) and item.get("canonicalDisposition") == "satisfied"
        for item in (assessments or [])
    )


ROOT_DIR = Path(__file__).resolve().parent.parent


def coverage_binding_chain_errors(audit: Any, require_complete: bool) -> list[str]:
    """Delegate proof semantics to the canonical Go Runtime contract.

    Python retains transport and projection-shape assertions only. It must not
    implement target authorization, validation attestation, variant, metric, or
    artifact proof rules independently from the SM-2 receipt-credit matcher.
    """

    request = json.dumps(
        {"audit": audit, "requireComplete": require_complete},
        separators=(",", ":"),
    )
    process = subprocess.run(
        ["go", "run", "./internal/tools/runtimeauditcheck"],
        cwd=ROOT_DIR,
        input=request,
        text=True,
        capture_output=True,
        check=False,
    )
    if process.returncode != 0:
        detail = (process.stderr or process.stdout).strip()
        return [f"canonical Runtime audit check failed: {detail or process.returncode}"]
    try:
        result = json.loads(process.stdout)
    except json.JSONDecodeError as exc:
        return [f"canonical Runtime audit check returned invalid JSON: {exc}"]
    errors = result.get("errors") if isinstance(result, dict) else None
    if not isinstance(errors, list) or any(not isinstance(item, str) for item in errors):
        return ["canonical Runtime audit check returned an invalid error list"]
    return errors


def runtime_audit_errors(
    audit: Any, workspace_id: str, run_id: str, require_terminal: bool
) -> list[str]:
    """Check the metadata-only Runtime audit chain retained by live runs."""

    if not isinstance(audit, dict):
        return ["missing Runtime audit projection"]
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
    for field in ("repairAttempts", "validationOnlyRetries", "noProgressCount"):
        if not isinstance(repair.get(field), int) or repair.get(field) < 0:
            errors.append(f"audit repair counter {field} is invalid")
    if "terminalDeliveryAttempts" in repair:
        errors.append("audit repair contains SM-4 terminal delivery state")

    if require_terminal:
        terminal = audit.get("terminal")
        if not isinstance(terminal, dict):
            errors.append("completed Run lacks terminal audit record")
        elif not terminal.get("code"):
            errors.append("terminal audit lacks terminal code")
        elif not isinstance(terminal.get("terminalDeliveryAttempts", 0), int) or terminal.get("terminalDeliveryAttempts", 0) < 0:
            errors.append("terminal audit delivery attempts are invalid")
        assessments = audit.get("assessments") or []
        if not assessments:
            errors.append("completed Run lacks assessment audit lineage")
        else:
            if any(not isinstance(item, dict) or not item.get("claimedDisposition") for item in assessments):
                errors.append("completed Run assessment audit lacks original claimed disposition")
            if not has_canonical_satisfied_assessment(assessments):
                errors.append("completed Run has no canonical satisfied assessment audit delta")
        errors.extend(coverage_binding_chain_errors(audit, True))
    return errors


def provider_ceiling_trace_errors(audit: Any, traces: Any) -> list[str]:
    """Prove at least one provider request used the sealed model ceiling."""

    if not isinstance(audit, dict):
        return ["missing Runtime audit projection"]
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


def runtime_completion_sequence_errors(audit: Any, require_focus_advance: bool) -> list[str]:
    """Check persisted evidence/assessment lineage, never inferred UI activity."""

    if not isinstance(audit, dict):
        return ["missing Runtime audit projection"]
    evidence = audit.get("acceptedEvidence") or []
    kinds = {item.get("kind") for item in evidence if isinstance(item, dict)}
    errors: list[str] = []
    if "state_change" not in kinds:
        errors.append("completion audit lacks accepted durable state-change evidence")
    if "validation" not in kinds:
        errors.append("completion audit lacks accepted validation evidence")
    if not has_canonical_satisfied_assessment(audit.get("assessments")):
        errors.append("completion audit lacks canonical satisfied assessment delta")
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
        "coverage": [{"id": "coverage-1", "state": "satisfied", "evidenceRefs": ["evidence-write", "evidence-validate"]}],
        "coverageBindings": [{
            "id": "binding/coverage-1", "coverageWorkItemId": "coverage-1", "focusId": "focus-1",
            "definitionRevision": "definition-1", "hash": "f514949145af69343b199ff648d441d81bc4fbb0022d7db5ba7dd1e1002bb04b", "targetRefs": ["target.txt"],
            "declarationRef": "dod/coverage-1",
            "proofs": [
                {"evidenceKind": "state_change", "requiredEffectScopes": ["workspace_durable"], "allowedOperationKinds": ["file/write"]},
                {"evidenceKind": "validation", "requiredEffectScopes": ["none"], "allowedOperationKinds": ["validation/managed_session"], "validationRecipeId": "recipe-1", "validationProofRequirementHash": "runtime-validation-proof-requirement-check-1", "requiredCheckRefs": ["check-1"], "validationCheckAttestations": [{"requiredCheckRef": "check-1", "observationProtocolRef": "shell/go-test-json/v1", "nativeCheckRef": "TestCheck1"}]},
            ],
        }],
        "authority": {
            "snapshotId": "authority-1", "convergenceState": "complete", "focusId": "focus-1", "requiredOutputHash": "output-1",
            "definitionRevision": "definition-1", "evidenceRevision": "evidence-2", "operationContractRevision": "operations-1",
        },
        "candidateAttempts": [{"authorityContractId": "contract-1", "authoritySnapshotId": "authority-1", "state": "accepted", "stablePrefixHash": "prefix-1"}],
        "dispatches": [{"id": "dispatch-1", "authoritySnapshotId": "authority-1", "state": "committed", "operations": [
            {"id": "write-1", "kind": "file/write", "executionKey": "exec/ws-1/run-1/dispatch-1/write-1", "fence": 1, "bindingRefs": ["binding/coverage-1"], "targetRefs": ["target.txt"]},
            {"id": "validate-1", "kind": "validation/managed_session", "executionKey": "exec/ws-1/run-1/dispatch-1/validate-1", "fence": 1, "bindingRefs": ["binding/coverage-1"], "targetRefs": ["target.txt"]},
        ]}],
        "grounding": {"prerequisiteObservations": [{"prerequisiteId": "manifest", "ownerModuleId": "blocks/file", "observationId": "obs-1", "dependencies": [{"domain": "workspace_tree", "ownerModuleId": "blocks/file", "subjectRef": "workspace", "revision": "tree-1"}]}], "providerCeiling": {"providerId": "ai-proxy", "modelId": "model-1", "contextWindowTokens": 128000, "providerMaxOutputTokens": 16384, "effectiveMaxOutputTokens": 16384, "maxInputTokens": 111616, "maxBytes": 446464, "capabilityRevision": "capability-1", "configurationRevision": "configuration-1"}},
        "repair": {"repairAttempts": 0, "validationOnlyRetries": 0, "noProgressCount": 0},
        "assessments": [{"id": "assessment-1", "claimedDisposition": "satisfied", "canonicalDisposition": "satisfied"}],
        "acceptedEvidence": [
            {"id": "evidence-write", "kind": "state_change", "dispatchId": "dispatch-1", "operationId": "write-1", "bindingRefs": ["binding/coverage-1"], "coverageWorkItemIds": ["coverage-1"], "targetRefs": ["target.txt"], "evidenceRefs": ["artifact/write"]},
            {"id": "evidence-validate", "kind": "validation", "dispatchId": "dispatch-1", "operationId": "validate-1", "bindingRefs": ["binding/coverage-1"], "coverageWorkItemIds": ["coverage-1"], "targetRefs": ["target.txt"], "evidenceRefs": ["artifact/validate"], "validationRecipeId": "recipe-1", "observationProtocolRef": "shell/go-test-json/v1", "executedChecks": ["TestCheck1"], "passedChecks": ["TestCheck1"]},
        ],
        "operationReceipts": [
            {"executionKey": "exec/ws-1/run-1/dispatch-1/write-1", "fence": 1, "operationId": "write-1", "outcome": "accepted", "observedEffectScope": "workspace_durable", "changed": True, "targetRefs": ["target.txt"], "evidenceRefs": ["artifact/write"]},
            {"executionKey": "exec/ws-1/run-1/dispatch-1/validate-1", "fence": 1, "operationId": "validate-1", "outcome": "accepted", "observedEffectScope": "none", "targetRefs": ["target.txt"], "evidenceRefs": ["artifact/validate"], "validationRecipeId": "recipe-1", "observationProtocolRef": "shell/go-test-json/v1", "executedChecks": ["TestCheck1"], "passedChecks": ["TestCheck1"]},
        ],
        "terminal": {"code": "runtime_completed", "terminalDeliveryAttempts": 0},
    }
    if errors := runtime_audit_errors(valid_audit, "ws-1", "run-1", True):
        raise AssertionError(f"valid Runtime audit rejected: {errors}")
    exact_target_subset = json.loads(json.dumps(valid_audit))
    exact_target_subset["coverageBindings"][0]["targetRefs"] = ["support.txt", "target.txt"]
    exact_target_subset["coverageBindings"][0]["hash"] = "c4874722eba1a0e1955955491e5406a80d2b08ffeedc5475d5babab72958342f"
    if errors := runtime_audit_errors(exact_target_subset, "ws-1", "run-1", True):
        raise AssertionError(f"exact-target operation subset rejected by canonical audit: {errors}")
    managed_audit = json.loads(json.dumps(valid_audit))
    managed_proof = managed_audit["coverageBindings"][0]["proofs"][1]
    managed_proof["allowedOperationKinds"] = ["validation/managed_session"]
    managed_proof["validationProofRequirementHash"] = "runtime-validation-proof-requirement-check-1"
    managed_proof["validationCheckAttestations"] = [{
        "requiredCheckRef": "check-1",
        "observationProtocolRef": "shell/go-test-json/v1",
        "nativeCheckRef": "TestCheck1",
    }]
    managed_audit["dispatches"][0]["operations"][1]["kind"] = "validation/managed_session"
    for observation in (
        managed_audit["acceptedEvidence"][1],
        managed_audit["operationReceipts"][1],
    ):
        observation["observationProtocolRef"] = "shell/go-test-json/v1"
        observation["executedChecks"] = ["TestCheck1"]
        observation["passedChecks"] = ["TestCheck1"]
    if errors := runtime_audit_errors(managed_audit, "ws-1", "run-1", True):
        raise AssertionError(f"valid managed-validation audit rejected: {errors}")
    logical_check_receipt = json.loads(json.dumps(managed_audit))
    for observation in (
        logical_check_receipt["acceptedEvidence"][1],
        logical_check_receipt["operationReceipts"][1],
    ):
        observation["executedChecks"] = ["check-1"]
        observation["passedChecks"] = ["check-1"]
    if not runtime_audit_errors(logical_check_receipt, "ws-1", "run-1", True):
        raise AssertionError("managed receipt bypassed its native check attestation")
    missing_attestation = json.loads(json.dumps(managed_audit))
    missing_attestation["coverageBindings"][0]["proofs"][1]["validationCheckAttestations"] = []
    if not runtime_audit_errors(missing_attestation, "ws-1", "run-1", True):
        raise AssertionError("managed receipt without projected check attestation was accepted")
    wrong_protocol = json.loads(json.dumps(managed_audit))
    wrong_protocol["acceptedEvidence"][1]["observationProtocolRef"] = "shell/plain-output/v1"
    wrong_protocol["operationReceipts"][1]["observationProtocolRef"] = "shell/plain-output/v1"
    if not runtime_audit_errors(wrong_protocol, "ws-1", "run-1", True):
        raise AssertionError("managed receipt with a drifted observation protocol was accepted")
    governed_audit = json.loads(json.dumps(valid_audit))
    governed_operation = governed_audit["dispatches"][0]["operations"][1]
    governed_operation["executionKey"] = "exec/ws-1/run-1/original-dispatch/validate-1"
    governed_operation["fence"] = 2
    governed_receipt = governed_audit["operationReceipts"][1]
    governed_receipt["executionKey"] = governed_operation["executionKey"]
    governed_receipt["fence"] = governed_operation["fence"]
    if errors := runtime_audit_errors(governed_audit, "ws-1", "run-1", True):
        raise AssertionError(f"governance-resumed execution identity rejected: {errors}")
    valid_trace = {"providerId": "ai-proxy", "model": "model-1", "contextBudget": {"providerId": "ai-proxy", "modelId": "model-1", "capabilityRevision": "capability-1", "configurationRevision": "configuration-1", "maxBytes": 446464, "maxRequestBytes": 446464, "maxTokens": 111616, "estimatedOutputTokens": 16384, "overBudget": False}}
    if errors := provider_ceiling_trace_errors(valid_audit, [valid_trace]):
        raise AssertionError(f"matching sealed provider ceiling trace rejected: {errors}")
    invalid_trace = dict(valid_trace)
    invalid_trace["contextBudget"] = dict(valid_trace["contextBudget"])
    invalid_trace["contextBudget"]["estimatedOutputTokens"] = 4096
    if not provider_ceiling_trace_errors(valid_audit, [invalid_trace]):
        raise AssertionError("fixed output-cap trace was accepted")
    valid_audit["completedFocusIds"] = ["focus-1"]
    valid_audit["remainingFocusIds"] = []
    if errors := runtime_completion_sequence_errors(valid_audit, True):
        raise AssertionError(f"valid completion sequence rejected: {errors}")
    invalid_audit = dict(valid_audit)
    invalid_audit["candidateAttempts"] = [{"authorityContractId": "contract-1"}]
    if not runtime_audit_errors(invalid_audit, "ws-1", "run-1", True):
        raise AssertionError("incomplete Runtime audit was accepted")
    legacy_action_audit = dict(valid_audit)
    legacy_action_audit["authority"] = dict(valid_audit["authority"])
    legacy_action_audit["authority"]["action"] = "complete"
    if not runtime_audit_errors(legacy_action_audit, "ws-1", "run-1", True):
        raise AssertionError("Runtime audit retaining retired action was accepted")
    legacy_assessment_audit = dict(valid_audit)
    legacy_assessment_audit["assessments"] = [{"id": "assessment-1", "disposition": "satisfied"}]
    if not runtime_audit_errors(legacy_assessment_audit, "ws-1", "run-1", True):
        raise AssertionError("Runtime audit retaining retired assessment disposition was accepted")
    duplicate_attempt_audit = dict(valid_audit)
    duplicate_attempt_audit["candidateAttempts"] = valid_audit["candidateAttempts"] * 2
    if not runtime_audit_errors(duplicate_attempt_audit, "ws-1", "run-1", True):
        raise AssertionError("duplicate Runtime candidate attempt was accepted")
    missing_binding = dict(valid_audit)
    missing_binding["coverageBindings"] = []
    if not runtime_audit_errors(missing_binding, "ws-1", "run-1", True):
        raise AssertionError("completed CWI without immutable binding was accepted")
    missing_sealed_operation = dict(valid_audit)
    missing_sealed_operation["dispatches"] = [{"id": "dispatch-1", "authoritySnapshotId": "authority-1", "state": "committed"}]
    if not runtime_audit_errors(missing_sealed_operation, "ws-1", "run-1", True):
        raise AssertionError("completed CWI without sealed operation was accepted")
    missing_receipt = dict(valid_audit)
    missing_receipt["operationReceipts"] = []
    if not runtime_audit_errors(missing_receipt, "ws-1", "run-1", True):
        raise AssertionError("completed CWI without owner receipt was accepted")
    missing_validation_proof = dict(valid_audit)
    missing_validation_proof["operationReceipts"] = [dict(item) for item in valid_audit["operationReceipts"]]
    missing_validation_proof["operationReceipts"][1]["passedChecks"] = []
    if not runtime_audit_errors(missing_validation_proof, "ws-1", "run-1", True):
        raise AssertionError("non-empty but incomplete validation proof was accepted")
    missing_proof_kind = json.loads(json.dumps(valid_audit))
    missing_proof_kind["coverage"][0]["evidenceRefs"] = ["evidence-write"]
    missing_proof_kind["acceptedEvidence"] = missing_proof_kind["acceptedEvidence"][:1]
    missing_proof_kind["operationReceipts"] = missing_proof_kind["operationReceipts"][:1]
    if not runtime_audit_errors(missing_proof_kind, "ws-1", "run-1", True):
        raise AssertionError("satisfied CWI without every sealed proof kind was accepted")

    print("runtime live contract self-test passed")


def audit_from_snapshot(value: Any) -> dict[str, Any] | None:
    """Extract the read-only Runtime audit from a run snapshot response."""

    if not isinstance(value, dict):
        return None
    snapshot = value.get("snapshot")
    if not isinstance(snapshot, dict):
        return None
    acceptance = snapshot.get("acceptance")
    if not isinstance(acceptance, dict):
        return None
    audit = acceptance.get("audit")
    return audit if isinstance(audit, dict) else None


def verify_snapshot(path: str, workspace_id: str, run_id: str, require_terminal: bool) -> None:
    with open(path, encoding="utf-8") as handle:
        audit = audit_from_snapshot(json.load(handle))
    errors = runtime_audit_errors(audit, workspace_id, run_id, require_terminal)
    if errors:
        raise SystemExit("\n".join(errors))


if __name__ == "__main__":
    args = sys.argv[1:]
    if args == ["--self-test"]:
        _self_test()
    elif len(args) == 7 and args[0] == "--snapshot" and args[2] == "--workspace-id" and args[4] == "--run-id" and args[6] == "--require-terminal":
        verify_snapshot(args[1], args[3], args[5], True)
    else:
        raise SystemExit("usage: verify_runtime_live_contract.py --self-test | --snapshot PATH --workspace-id ID --run-id ID --require-terminal")
