#!/usr/bin/env python3
"""Fail-closed verifier for deployed Runtime convergence evidence manifests.

The verifier deliberately does not infer scenario coverage from a successful
shell command or from its filename.  A scenario counts only after its manifest
passes the checked JSON schema, every referenced artifact resolves and matches
its digest/data-root identity, and every scenario-mandatory assertion is
present with concrete evidence references.
"""

from __future__ import annotations

import argparse
import dataclasses
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import tempfile
from typing import Any, Iterable


SCHEMA_VERSION = "workorch.deployed-convergence-evidence/v1"
AGGREGATE_SCHEMA_VERSION = "workorch.deployed-convergence-evidence-aggregate/v1"
DEFAULT_SCHEMA = Path(__file__).with_name("deployed-convergence-evidence-manifest.schema.json")


REFERENCE_FIELDS: dict[str, str] = {
    "requestSessionRefs": "request_session",
    "dispatchRefs": "sealed_dispatch",
    "ownerReceiptRefs": "owner_receipt",
    "evidenceRefs": "accepted_evidence",
    "waitRefs": "recoverable_wait",
    "governanceRefs": "governance",
    "definitionCandidateRefs": "definition_candidate",
    "definitionClarificationRefs": "definition_clarification",
    "replacementIntentRefs": "replacement_intent",
    "resultRefs": "result",
    "terminalDeliveryRefs": "terminal_delivery",
}

HASH_FIELDS: dict[str, str] = {
    "ownerRevisionSetHashes": "owner_revision_set",
    "budgetViewHashes": "budget_view",
}


@dataclasses.dataclass(frozen=True)
class ScenarioRule:
    assertions: frozenset[str]
    required_refs: frozenset[str] = frozenset()
    required_hashes: frozenset[str] = frozenset()
    min_workspaces: int = 1
    min_runs: int = 1
    min_definitions: int = 1
    min_runtime_revisions: int = 1
    min_restart_epochs: int = 1


SCENARIO_RULES: dict[str, ScenarioRule] = {
    "DC-01": ScenarioRule(
        assertions=frozenset(
            {
                "runtime_revision_monotonic",
                "binding_and_dispatch_sealed_before_operation",
                "concrete_owner_receipt",
                "result_terminal_delivery_hash_match",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset(
            {
                "requestSessionRefs",
                "dispatchRefs",
                "ownerReceiptRefs",
                "evidenceRefs",
                "resultRefs",
                "terminalDeliveryRefs",
            }
        ),
        required_hashes=frozenset({"ownerRevisionSetHashes"}),
        min_runtime_revisions=2,
    ),
    "DC-02": ScenarioRule(
        assertions=frozenset(
            {
                "submitted_marker_precedes_provider_call",
                "pre_terminal_cas_restart_has_zero_replay",
                "post_terminal_cas_readback_is_idempotent",
                "complete_document_consumes_one_attempt",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset({"requestSessionRefs"}),
        min_restart_epochs=2,
    ),
    "DC-03": ScenarioRule(
        assertions=frozenset(
            {
                "execution_key_and_fence_stable",
                "owner_effect_at_most_once",
                "receipt_precedes_runtime_credit",
                "stale_duplicate_receipt_zero_credit",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset({"dispatchRefs", "ownerReceiptRefs", "evidenceRefs"}),
        min_restart_epochs=2,
    ),
    "DC-04": ScenarioRule(
        assertions=frozenset(
            {
                "intersecting_evidence_becomes_stale",
                "unrelated_coverage_remains_current",
                "replacement_evidence_recloses_focus",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset({"dispatchRefs", "ownerReceiptRefs", "evidenceRefs"}),
        required_hashes=frozenset({"ownerRevisionSetHashes"}),
        min_runtime_revisions=2,
    ),
    "DC-05": ScenarioRule(
        assertions=frozenset(
            {
                "recoverable_wait_record_complete",
                "generic_resume_does_not_release_wait",
                "matching_trigger_is_idempotent",
                "condition_satisfied_continues_same_run",
                "deadline_and_guard_terminal_are_typed",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset({"waitRefs"}),
        min_restart_epochs=2,
    ),
    "DC-06": ScenarioRule(
        assertions=frozenset(
            {
                "allocation_precedes_physical_call_or_effect",
                "physical_identity_deduplicated",
                "budget_view_matches_subledger",
                "budget_survives_restart",
                "unknown_allocation_excluded_from_remaining",
                "exhaustion_has_typed_disposition",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset({"requestSessionRefs", "dispatchRefs", "ownerReceiptRefs"}),
        required_hashes=frozenset({"budgetViewHashes"}),
        min_restart_epochs=2,
    ),
    "DC-07": ScenarioRule(
        assertions=frozenset(
            {
                "ticket_has_external_dependency",
                "resolution_scope_and_revision_match",
                "stale_duplicate_resolution_deterministic",
                "system_and_budget_errors_create_no_ticket",
                "resolution_continues_same_run",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset({"waitRefs", "governanceRefs"}),
    ),
    "DC-08": ScenarioRule(
        assertions=frozenset(
            {
                "replacement_intent_precedes_delivery",
                "terminal_ack_order_result_workspace_run",
                "successor_is_unique",
                "predecessor_is_immutable",
                "successor_uses_fresh_policy_and_aggregate",
                "duplicate_restart_preserves_lineage",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset({"replacementIntentRefs", "resultRefs", "terminalDeliveryRefs"}),
        min_runs=2,
        min_restart_epochs=2,
    ),
    "DC-09": ScenarioRule(
        assertions=frozenset(
            {
                "required_cwi_three_axis_closeout",
                "accepted_refs_and_owner_revision_union_exact",
                "indeterminate_is_not_success",
                "closeout_view_hash_and_lineage_match",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset({"evidenceRefs", "resultRefs", "terminalDeliveryRefs"}),
        required_hashes=frozenset({"ownerRevisionSetHashes"}),
    ),
    "DC-10": ScenarioRule(
        assertions=frozenset(
            {
                "all_authority_records_are_workspace_scoped",
                "projection_lag_does_not_drive_authority",
                "workspace_recovery_is_independent",
                "sse_reconnect_preserves_scope",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset(
            {
                "requestSessionRefs",
                "dispatchRefs",
                "ownerReceiptRefs",
                "evidenceRefs",
                "waitRefs",
                "resultRefs",
            }
        ),
        required_hashes=frozenset({"budgetViewHashes"}),
        min_workspaces=2,
        min_runs=2,
    ),
    "DC-11": ScenarioRule(
        assertions=frozenset(
            {
                "invalid_candidate_has_no_promotion_run_provider_or_effect",
                "duplicate_invalid_candidate_reuses_ticket",
                "corrected_all_of_promotion_waits_for_dedicated_resolution",
                "workspace_ack_precedes_eligibility",
                "setup_does_not_require_llm",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset(
            {"definitionCandidateRefs", "definitionClarificationRefs", "governanceRefs"}
        ),
        min_runs=0,
        min_runtime_revisions=0,
    ),
    "DC-12": ScenarioRule(
        assertions=frozenset(
            {
                "runtime_gap_opens_dedicated_clarification",
                "affected_focus_dispatch_is_frozen",
                "generic_answer_and_resume_are_rejected",
                "new_definition_does_not_mutate_predecessor",
                "replacement_binding_and_delivery_are_restart_safe",
                "successor_is_unique",
                "bounded_terminal",
            }
        ),
        required_refs=frozenset(
            {
                "definitionClarificationRefs",
                "governanceRefs",
                "replacementIntentRefs",
                "terminalDeliveryRefs",
            }
        ),
        min_runs=2,
        min_definitions=2,
        min_restart_epochs=2,
    ),
}


@dataclasses.dataclass(frozen=True)
class ProfileRequirement:
    scenario_id: str
    scenario_variant: str
    provider_constraint: str = "any"
    require_non_generic_profile: bool = False


CI_SAFE_REQUIREMENTS = tuple(
    ProfileRequirement(scenario_id, "ci-safe", "non-real")
    for scenario_id in ("DC-01", "DC-03", "DC-05", "DC-06", "DC-07", "DC-08", "DC-09", "DC-10")
)
DEFINITION_REQUIREMENTS = tuple(
    ProfileRequirement(scenario_id, "definition") for scenario_id in ("DC-11", "DC-12")
)
REAL_PROVIDER_REQUIREMENTS = tuple(
    ProfileRequirement(scenario_id, "real-provider", "real") for scenario_id in ("DC-01", "DC-02")
)
PROFILE_SPECIFIC_REQUIREMENTS = (
    ProfileRequirement("DC-04", "profile", require_non_generic_profile=True),
)

# Requirements are manifest identities, not only scenario IDs. In particular,
# a full release needs two independent DC-01 proofs: reproducible CI-safe and
# real-provider. One variant can never stand in for the other.
PROFILE_REQUIREMENTS: dict[str, tuple[ProfileRequirement, ...]] = {
    "ci-safe": CI_SAFE_REQUIREMENTS,
    "definition": DEFINITION_REQUIREMENTS,
    "real-provider": REAL_PROVIDER_REQUIREMENTS,
    "profile": PROFILE_SPECIFIC_REQUIREMENTS,
    "full": (
        *CI_SAFE_REQUIREMENTS,
        *REAL_PROVIDER_REQUIREMENTS,
        *PROFILE_SPECIFIC_REQUIREMENTS,
        *DEFINITION_REQUIREMENTS,
    ),
}

# A release manifest is eligible only when its producer is reviewed and wired
# to emit the mandatory assertions from observed owner/read artifacts.  None of
# the historical verifier entrypoints currently emits the v1 manifest in full,
# so registering one based only on a successful exit code would be a false
# coverage claim.  Add exact entrypoints here only together with their producer
# implementation and regression fixtures.
REGISTERED_PRODUCERS: dict[tuple[str, str], frozenset[str]] = {
    (requirement.scenario_id, requirement.scenario_variant): frozenset()
    for requirements in PROFILE_REQUIREMENTS.values()
    for requirement in requirements
}


class DuplicateKeyError(ValueError):
    pass


def _object_without_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise DuplicateKeyError(f"duplicate JSON object key {key!r}")
        result[key] = value
    return result


def load_json(path: Path) -> Any:
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle, object_pairs_hook=_object_without_duplicate_keys)


def _json_type_matches(value: Any, expected: str) -> bool:
    if expected == "object":
        return isinstance(value, dict)
    if expected == "array":
        return isinstance(value, list)
    if expected == "string":
        return isinstance(value, str)
    if expected == "integer":
        return isinstance(value, int) and not isinstance(value, bool)
    if expected == "boolean":
        return isinstance(value, bool)
    return True


def schema_errors(value: Any, schema: dict[str, Any], path: str = "manifest") -> list[str]:
    """Validate the deliberately small JSON-Schema keyword set used by v1."""

    errors: list[str] = []
    expected_type = schema.get("type")
    if isinstance(expected_type, str) and not _json_type_matches(value, expected_type):
        return [f"{path} must be {expected_type}"]
    if "const" in schema and value != schema["const"]:
        errors.append(f"{path} must equal {schema['const']!r}")
    if "enum" in schema and value not in schema["enum"]:
        errors.append(f"{path} must be one of {schema['enum']!r}")

    if isinstance(value, dict):
        required = schema.get("required") or []
        for field in required:
            if field not in value:
                errors.append(f"{path}.{field} is required")
        properties = schema.get("properties") or {}
        if schema.get("additionalProperties") is False:
            for field in value:
                if field not in properties:
                    errors.append(f"{path}.{field} is not allowed by schema")
        for field, child in value.items():
            child_schema = properties.get(field)
            if isinstance(child_schema, dict):
                errors.extend(schema_errors(child, child_schema, f"{path}.{field}"))
    elif isinstance(value, list):
        minimum = schema.get("minItems")
        if isinstance(minimum, int) and len(value) < minimum:
            errors.append(f"{path} must contain at least {minimum} items")
        if schema.get("uniqueItems") is True:
            rendered = [json.dumps(item, ensure_ascii=False, sort_keys=True) for item in value]
            if len(rendered) != len(set(rendered)):
                errors.append(f"{path} must contain unique items")
        item_schema = schema.get("items")
        if isinstance(item_schema, dict):
            for index, child in enumerate(value):
                errors.extend(schema_errors(child, item_schema, f"{path}[{index}]"))
    elif isinstance(value, str):
        minimum = schema.get("minLength")
        if isinstance(minimum, int) and len(value) < minimum:
            errors.append(f"{path} must contain at least {minimum} characters")
        pattern = schema.get("pattern")
        if isinstance(pattern, str) and re.search(pattern, value) is None:
            errors.append(f"{path} does not match {pattern!r}")
    elif isinstance(value, int) and not isinstance(value, bool):
        minimum = schema.get("minimum")
        maximum = schema.get("maximum")
        if isinstance(minimum, int) and value < minimum:
            errors.append(f"{path} must be at least {minimum}")
        if isinstance(maximum, int) and value > maximum:
            errors.append(f"{path} must be at most {maximum}")
    return errors


RFC3339_TIMESTAMP = re.compile(
    r"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}"
    r"(?:\.[0-9]{1,9})?(?:Z|[+-][0-9]{2}:[0-9]{2})$"
)


def _parse_timestamp(value: Any, field: str, errors: list[str]) -> dt.datetime | None:
    if not isinstance(value, str):
        return None
    if RFC3339_TIMESTAMP.fullmatch(value) is None:
        errors.append(f"manifest.{field} must be an RFC3339 timestamp")
        return None
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        errors.append(f"manifest.{field} must be an RFC3339 timestamp")
        return None
    if parsed.tzinfo is None:
        errors.append(f"manifest.{field} must include a timezone")
        return None
    return parsed


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return f"sha256:{digest.hexdigest()}"


def _artifact_path_errors(manifest_path: Path, raw_path: str) -> tuple[Path | None, list[str]]:
    errors: list[str] = []
    relative = Path(raw_path)
    if relative.is_absolute() or ".." in relative.parts:
        return None, [f"artifact path {raw_path!r} must stay relative to the manifest directory"]
    root = manifest_path.parent.resolve()
    unresolved = manifest_path.parent / relative
    current = manifest_path.parent
    for part in relative.parts:
        current = current / part
        if current.is_symlink():
            errors.append(f"artifact path {raw_path!r} must not traverse a symlink")
            return None, errors
    try:
        resolved = unresolved.resolve(strict=True)
    except FileNotFoundError:
        return None, [f"artifact path {raw_path!r} does not exist"]
    try:
        resolved.relative_to(root)
    except ValueError:
        return None, [f"artifact path {raw_path!r} escapes the manifest directory"]
    if not resolved.is_file():
        return None, [f"artifact path {raw_path!r} is not a regular file"]
    return resolved, errors


@dataclasses.dataclass(frozen=True)
class ValidatedManifest:
    path: Path
    scenario_id: str
    scenario_variant: str
    profile_id: str
    provider_class: str
    invocation_id: str
    service_revision: str
    producer_entrypoint: str
    manifest_sha256: str
    assertion_ids: tuple[str, ...]


def validate_manifest(
    manifest: Any,
    manifest_path: Path,
    schema: dict[str, Any],
) -> tuple[ValidatedManifest | None, list[str]]:
    errors = schema_errors(manifest, schema)
    if errors or not isinstance(manifest, dict):
        return None, errors

    scenario_id = manifest["scenarioId"]
    rule = SCENARIO_RULES.get(scenario_id)
    if rule is None:
        errors.append(f"manifest.scenarioId {scenario_id!r} has no active scenario rule")
        return None, errors

    if scenario_id in {"DC-01", "DC-02", "DC-06", "DC-12"} and manifest["providerClass"] == "not_applicable":
        errors.append(f"{scenario_id} requires providerClass mock or real")

    started_at = _parse_timestamp(manifest["startedAt"], "startedAt", errors)
    completed_at = _parse_timestamp(manifest["completedAt"], "completedAt", errors)
    if started_at is not None and completed_at is not None:
        if completed_at < started_at:
            errors.append("manifest.completedAt precedes manifest.startedAt")
        elif (completed_at - started_at).total_seconds() > manifest["boundedDeadlineSeconds"]:
            errors.append("manifest elapsed time exceeds boundedDeadlineSeconds")

    for field, minimum in (
        ("workspaceIds", rule.min_workspaces),
        ("runIds", rule.min_runs),
        ("definitionRevisions", rule.min_definitions),
        ("runtimeStateRevisions", rule.min_runtime_revisions),
        ("restartEpochs", rule.min_restart_epochs),
    ):
        if len(manifest[field]) < minimum:
            errors.append(f"{scenario_id} requires at least {minimum} {field}")

    revisions = manifest["runtimeStateRevisions"]
    if any(right <= left for left, right in zip(revisions, revisions[1:])):
        errors.append("manifest.runtimeStateRevisions must be strictly increasing")

    for field in rule.required_refs:
        if not manifest[field]:
            errors.append(f"{scenario_id} requires non-empty manifest.{field}")
    for field in rule.required_hashes:
        if not manifest[field]:
            errors.append(f"{scenario_id} requires non-empty manifest.{field}")

    artifacts_by_ref: dict[str, dict[str, Any]] = {}
    artifact_hashes_by_kind: dict[str, set[str]] = {}
    all_manifest_refs: set[str] = set()
    for index, artifact in enumerate(manifest["artifacts"]):
        ref = artifact["ref"]
        if ref in artifacts_by_ref:
            errors.append(f"manifest.artifacts[{index}].ref duplicates {ref!r}")
            continue
        artifacts_by_ref[ref] = artifact
        artifact_hashes_by_kind.setdefault(artifact["kind"], set()).add(artifact["sha256"])
        if artifact["dataRootIdentity"] != manifest["dataRootIdentity"]:
            errors.append(f"artifact {ref!r} dataRootIdentity differs from manifest")
        if artifact["workspaceId"] and artifact["workspaceId"] not in manifest["workspaceIds"]:
            errors.append(f"artifact {ref!r} workspaceId is outside manifest scope")
        if artifact["runId"] and artifact["runId"] not in manifest["runIds"]:
            errors.append(f"artifact {ref!r} runId is outside manifest scope")
        resolved, path_errors = _artifact_path_errors(manifest_path, artifact["path"])
        errors.extend(f"artifact {ref!r}: {value}" for value in path_errors)
        if resolved is not None:
            actual = _sha256(resolved)
            if actual != artifact["sha256"]:
                errors.append(
                    f"artifact {ref!r} digest mismatch: manifest={artifact['sha256']} actual={actual}"
                )

    for field, expected_kind in REFERENCE_FIELDS.items():
        for ref in manifest[field]:
            all_manifest_refs.add(ref)
            artifact = artifacts_by_ref.get(ref)
            if artifact is None:
                errors.append(f"manifest.{field} ref {ref!r} has no artifact registry entry")
            elif artifact["kind"] != expected_kind:
                errors.append(
                    f"manifest.{field} ref {ref!r} has kind={artifact['kind']!r}, want {expected_kind!r}"
                )

    for field, expected_kind in HASH_FIELDS.items():
        available = artifact_hashes_by_kind.get(expected_kind, set())
        for digest in manifest[field]:
            if digest not in available:
                errors.append(
                    f"manifest.{field} digest {digest!r} has no matching {expected_kind!r} artifact"
                )

    if scenario_id == "DC-10":
        expected_workspaces = set(manifest["workspaceIds"])
        expected_runs = set(manifest["runIds"])
        for field in sorted(rule.required_refs):
            selected_artifacts = [
                artifacts_by_ref[ref] for ref in manifest[field] if ref in artifacts_by_ref
            ]
            observed_workspaces = {
                artifact["workspaceId"] for artifact in selected_artifacts if artifact["workspaceId"]
            }
            missing_workspaces = sorted(expected_workspaces - observed_workspaces)
            if missing_workspaces:
                errors.append(
                    f"DC-10 manifest.{field} lacks per-workspace evidence for: "
                    + ", ".join(missing_workspaces)
                )
            observed_runs = {
                artifact["runId"] for artifact in selected_artifacts if artifact["runId"]
            }
            missing_runs = sorted(expected_runs - observed_runs)
            if missing_runs:
                errors.append(
                    f"DC-10 manifest.{field} lacks per-run evidence for: "
                    + ", ".join(missing_runs)
                )
        for field in sorted(rule.required_hashes):
            expected_kind = HASH_FIELDS[field]
            selected_hashes = set(manifest[field])
            observed_workspaces = {
                artifact["workspaceId"]
                for artifact in manifest["artifacts"]
                if artifact["kind"] == expected_kind
                and artifact["sha256"] in selected_hashes
                and artifact["workspaceId"]
            }
            missing_workspaces = sorted(expected_workspaces - observed_workspaces)
            if missing_workspaces:
                errors.append(
                    f"DC-10 manifest.{field} lacks per-workspace evidence for: "
                    + ", ".join(missing_workspaces)
                )
            observed_runs = {
                artifact["runId"]
                for artifact in manifest["artifacts"]
                if artifact["kind"] == expected_kind
                and artifact["sha256"] in selected_hashes
                and artifact["runId"]
            }
            missing_runs = sorted(expected_runs - observed_runs)
            if missing_runs:
                errors.append(
                    f"DC-10 manifest.{field} lacks per-run evidence for: "
                    + ", ".join(missing_runs)
                )

    epoch_ids: set[str] = set()
    epoch_refs: set[str] = set()
    epoch_digests: set[str] = set()
    previous_epoch_started: dt.datetime | None = None
    for index, epoch in enumerate(manifest["restartEpochs"]):
        if epoch["epochId"] in epoch_ids:
            errors.append(f"manifest.restartEpochs[{index}] duplicates epochId {epoch['epochId']!r}")
        epoch_ids.add(epoch["epochId"])
        epoch_started = _parse_timestamp(epoch["startedAt"], f"restartEpochs[{index}].startedAt", errors)
        if epoch_started is not None and started_at is not None and epoch_started < started_at:
            errors.append(f"manifest.restartEpochs[{index}].startedAt precedes scenario start")
        if epoch_started is not None and completed_at is not None and epoch_started > completed_at:
            errors.append(f"manifest.restartEpochs[{index}].startedAt follows scenario completion")
        if (
            epoch_started is not None
            and previous_epoch_started is not None
            and epoch_started <= previous_epoch_started
        ):
            errors.append("manifest.restartEpochs must be strictly ordered by startedAt")
        if epoch_started is not None:
            previous_epoch_started = epoch_started
        ref = epoch["evidenceRef"]
        all_manifest_refs.add(ref)
        if ref in epoch_refs:
            errors.append(f"manifest.restartEpochs[{index}] reuses evidenceRef {ref!r}")
        epoch_refs.add(ref)
        artifact = artifacts_by_ref.get(ref)
        if artifact is None:
            errors.append(f"manifest.restartEpochs[{index}].evidenceRef has no artifact registry entry")
        elif artifact["kind"] != "restart_epoch":
            errors.append(f"manifest.restartEpochs[{index}].evidenceRef must reference restart_epoch")
        elif artifact["sha256"] in epoch_digests:
            errors.append(
                f"manifest.restartEpochs[{index}].evidenceRef reuses a prior restart artifact digest"
            )
        else:
            epoch_digests.add(artifact["sha256"])

    assertions_by_id: dict[str, dict[str, Any]] = {}
    for index, assertion in enumerate(manifest["assertions"]):
        assertion_id = assertion["id"]
        if assertion_id in assertions_by_id:
            errors.append(f"manifest.assertions[{index}].id duplicates {assertion_id!r}")
        assertions_by_id[assertion_id] = assertion
        if assertion["status"] != "passed":
            errors.append(f"assertion {assertion_id!r} did not pass")
        for ref in assertion["evidenceRefs"]:
            all_manifest_refs.add(ref)
            if ref not in artifacts_by_ref:
                errors.append(f"assertion {assertion_id!r} ref {ref!r} has no artifact registry entry")
    missing_assertions = sorted(rule.assertions - assertions_by_id.keys())
    if missing_assertions:
        errors.append(f"{scenario_id} missing mandatory assertions: {', '.join(missing_assertions)}")

    orphan_refs = sorted(set(artifacts_by_ref) - all_manifest_refs)
    if orphan_refs:
        errors.append(f"manifest has unreferenced artifacts: {', '.join(orphan_refs)}")

    if errors:
        return None, errors
    return (
        ValidatedManifest(
            path=manifest_path,
            scenario_id=scenario_id,
            scenario_variant=manifest["scenarioVariant"],
            profile_id=manifest["profileId"],
            provider_class=manifest["providerClass"],
            invocation_id=manifest["invocationId"],
            service_revision=manifest["serviceRevision"],
            producer_entrypoint=manifest["producerEntrypoint"],
            manifest_sha256=_sha256(manifest_path),
            assertion_ids=tuple(sorted(assertions_by_id)),
        ),
        [],
    )


def _candidate_matches_requirement(
    candidate: ValidatedManifest,
    requirement: ProfileRequirement,
    required_profile_id: str | None,
) -> bool:
    if candidate.scenario_id != requirement.scenario_id or candidate.scenario_variant != requirement.scenario_variant:
        return False
    if requirement.provider_constraint == "non-real" and candidate.provider_class == "real":
        return False
    if requirement.provider_constraint == "real" and candidate.provider_class != "real":
        return False
    if requirement.require_non_generic_profile:
        if candidate.profile_id == "generic":
            return False
        if required_profile_id and candidate.profile_id != required_profile_id:
            return False
    return True


def _requirement_qualifier(
    requirement: ProfileRequirement,
    required_profile_id: str | None,
) -> str:
    parts = [f"scenarioVariant={requirement.scenario_variant!r}"]
    if requirement.provider_constraint == "real":
        parts.append("providerClass=real")
    elif requirement.provider_constraint == "non-real":
        parts.append("non-real providerClass")
    if requirement.require_non_generic_profile:
        if required_profile_id:
            parts.append(f"profileId={required_profile_id!r}")
        else:
            parts.append("a non-generic profileId")
    return " with " + " and ".join(parts)


def aggregate_manifests(
    manifest_root: Path,
    schema: dict[str, Any],
    profile: str,
    required_profile_id: str | None = None,
    producer_registry: dict[tuple[str, str], frozenset[str]] | None = None,
) -> tuple[dict[str, Any], list[str]]:
    errors: list[str] = []
    validated: list[ValidatedManifest] = []
    registered_producers = REGISTERED_PRODUCERS if producer_registry is None else producer_registry
    if not manifest_root.exists() or not manifest_root.is_dir():
        errors.append(f"manifest root does not exist or is not a directory: {manifest_root}")
        paths: list[Path] = []
    else:
        paths = sorted(manifest_root.rglob("*.dce.json"))
        if not paths:
            errors.append(f"manifest root contains no *.dce.json files: {manifest_root}")

    for path in paths:
        if path.is_symlink():
            errors.append(f"{path}: manifest path must not be a symlink")
            continue
        try:
            document = load_json(path)
        except (OSError, json.JSONDecodeError, DuplicateKeyError) as exc:
            errors.append(f"{path}: cannot load manifest: {exc}")
            continue
        value, manifest_errors = validate_manifest(document, path, schema)
        if manifest_errors:
            errors.extend(f"{path}: {message}" for message in manifest_errors)
        elif value is not None:
            producer_identity = (value.scenario_id, value.scenario_variant)
            allowed_producers = registered_producers.get(producer_identity, frozenset())
            if value.producer_entrypoint not in allowed_producers:
                registered = ", ".join(sorted(allowed_producers)) or "none"
                errors.append(
                    f"{path}: producerEntrypoint {value.producer_entrypoint!r} is not a registered "
                    f"deployed producer for {value.scenario_id}/{value.scenario_variant} "
                    f"(registered: {registered})"
                )
            else:
                validated.append(value)

    identity_seen: dict[tuple[str, str, str, str], Path] = {}
    for item in validated:
        identity = (item.scenario_id, item.scenario_variant, item.profile_id, item.provider_class)
        previous = identity_seen.get(identity)
        if previous is not None:
            errors.append(
                "duplicate scenario evidence identity "
                f"{identity}: {previous} and {item.path}"
            )
        identity_seen[identity] = item.path

    requirements = PROFILE_REQUIREMENTS[profile]
    selected: list[ValidatedManifest] = []
    for requirement in requirements:
        candidates = [
            item
            for item in validated
            if _candidate_matches_requirement(item, requirement, required_profile_id)
        ]
        if not candidates:
            errors.append(
                f"release profile {profile!r} is missing {requirement.scenario_id}"
                f"{_requirement_qualifier(requirement, required_profile_id)}"
            )
            continue
        if len(candidates) > 1:
            errors.append(
                f"release profile {profile!r} has ambiguous {requirement.scenario_id}"
                f"/{requirement.scenario_variant} evidence: "
                + ", ".join(str(item.path) for item in candidates)
            )
            continue
        selected.append(candidates[0])

    service_revisions = {item.service_revision for item in selected}
    if len(service_revisions) > 1:
        errors.append(
            "selected release evidence spans multiple service revisions: "
            + ", ".join(sorted(service_revisions))
        )

    root_resolved = manifest_root.resolve()
    summaries: list[dict[str, Any]] = []
    for item in selected:
        try:
            relative_path = item.path.resolve().relative_to(root_resolved).as_posix()
        except ValueError:
            relative_path = item.path.name
        summaries.append(
            {
                "scenarioId": item.scenario_id,
                "scenarioVariant": item.scenario_variant,
                "profileId": item.profile_id,
                "providerClass": item.provider_class,
                "invocationId": item.invocation_id,
                "producerEntrypoint": item.producer_entrypoint,
                "serviceRevision": item.service_revision,
                "manifestPath": relative_path,
                "manifestSha256": item.manifest_sha256,
                "assertionIds": list(item.assertion_ids),
            }
        )
    aggregate = {
        "schemaVersion": AGGREGATE_SCHEMA_VERSION,
        "profile": profile,
        "requiredProfileId": required_profile_id or "",
        "requiredScenarioIds": [requirement.scenario_id for requirement in requirements],
        "requiredEvidenceIdentities": [
            {
                "scenarioId": requirement.scenario_id,
                "scenarioVariant": requirement.scenario_variant,
                "providerConstraint": requirement.provider_constraint,
                "requireNonGenericProfile": requirement.require_non_generic_profile,
            }
            for requirement in requirements
        ],
        "status": "failed" if errors else "passed",
        "validatedManifests": sorted(
            summaries,
            key=lambda item: (item["scenarioId"], item["scenarioVariant"], item["profileId"], item["providerClass"]),
        ),
        "errors": errors,
        "verifiedAt": dt.datetime.now(dt.timezone.utc).isoformat().replace("+00:00", "Z"),
    }
    return aggregate, errors


def write_json_atomic(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
            json.dump(value, handle, ensure_ascii=False, indent=2, sort_keys=True)
            handle.write("\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.replace(temporary_name, path)
    finally:
        if os.path.exists(temporary_name):
            os.unlink(temporary_name)


SUPPORTED_SCHEMA_KEYWORDS = frozenset(
    {
        "$schema",
        "$id",
        "title",
        "type",
        "additionalProperties",
        "required",
        "properties",
        "const",
        "enum",
        "pattern",
        "minLength",
        "minimum",
        "maximum",
        "items",
        "minItems",
        "uniqueItems",
    }
)


def _unsupported_schema_keywords(schema: dict[str, Any], path: str = "schema") -> list[str]:
    errors = [
        f"{path} uses unsupported keyword {key!r}"
        for key in schema
        if key not in SUPPORTED_SCHEMA_KEYWORDS
    ]
    properties = schema.get("properties")
    if isinstance(properties, dict):
        for field, child in properties.items():
            if isinstance(child, dict):
                errors.extend(_unsupported_schema_keywords(child, f"{path}.properties.{field}"))
    items = schema.get("items")
    if isinstance(items, dict):
        errors.extend(_unsupported_schema_keywords(items, f"{path}.items"))
    return errors


def _load_schema(path: Path) -> dict[str, Any]:
    value = load_json(path)
    if not isinstance(value, dict):
        raise ValueError("manifest schema must be a JSON object")
    if value.get("title") != "DeployedConvergenceEvidenceManifest":
        raise ValueError("manifest schema title is not DeployedConvergenceEvidenceManifest")
    unsupported = _unsupported_schema_keywords(value)
    if unsupported:
        raise ValueError("; ".join(unsupported))
    return value


def parse_args(argv: Iterable[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest-root", type=Path, required=True)
    parser.add_argument("--schema", type=Path, default=DEFAULT_SCHEMA)
    parser.add_argument("--profile", choices=tuple(PROFILE_REQUIREMENTS), default="ci-safe")
    parser.add_argument("--profile-id", help="required DC-04 profileId for profile/full gates")
    parser.add_argument("--output", type=Path, required=True)
    return parser.parse_args(list(argv))


def main(argv: Iterable[str] = sys.argv[1:]) -> int:
    args = parse_args(argv)
    try:
        schema = _load_schema(args.schema)
    except (OSError, json.JSONDecodeError, DuplicateKeyError, ValueError) as exc:
        print(f"cannot load deployed convergence manifest schema: {exc}", file=sys.stderr)
        return 2
    aggregate, errors = aggregate_manifests(
        args.manifest_root,
        schema,
        args.profile,
        args.profile_id,
    )
    write_json_atomic(args.output, aggregate)
    if errors:
        print("deployed convergence evidence gate failed:", file=sys.stderr)
        for error in errors:
            print(f"- {error}", file=sys.stderr)
        print(f"aggregate report: {args.output}", file=sys.stderr)
        return 1
    print(
        "deployed convergence evidence gate passed "
        f"profile={args.profile} scenarios={','.join(aggregate['requiredScenarioIds'])}"
    )
    print(f"aggregate report: {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
