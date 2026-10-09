#!/usr/bin/env python3
"""只读联结软件开发验收报告与现有 WorkOrch typed owner DTO。"""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import sys
from typing import Any

import verify_lsp_bridge_prd_evidence as evidence_lint


class LinkError(ValueError):
    """输入类型错误、证据 lineage 不匹配或 owner DTO 缺失。"""


def _load_json(path: Path) -> Any:
    value = json.loads(
        path.read_text(encoding="utf-8"),
        object_pairs_hook=evidence_lint._no_duplicate_pairs,
        parse_constant=evidence_lint._reject_constant,
        parse_float=evidence_lint._reject_overflow,
    )
    evidence_lint._reject_surrogates(value)
    return value


def _object(value: Any, where: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        raise LinkError(f"{where} 必须是对象")
    return value


def _string(value: Any, where: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise LinkError(f"{where} 缺少非空字符串")
    return value


def _array(value: Any, where: str) -> list[Any]:
    if not isinstance(value, list):
        raise LinkError(f"{where} 必须是数组")
    return value


def _ref_array(value: Any, where: str) -> set[str]:
    values = _array(value, where)
    if any(not isinstance(item, str) or not item for item in values):
        raise LinkError(f"{where} 必须包含非空字符串")
    if len(set(values)) != len(values):
        raise LinkError(f"{where} 存在重复项")
    return set(values)


def _index(rows: list[Any], where: str, id_key: str) -> dict[str, dict[str, Any]]:
    output: dict[str, dict[str, Any]] = {}
    for position, value in enumerate(rows):
        row = _object(value, f"{where}[{position}]")
        identity = _string(row.get(id_key), f"{where}[{position}].{id_key}")
        if identity in output:
            raise LinkError(f"{where} 重复 id {identity}")
        output[identity] = row
    return output


def _sha256(raw: bytes) -> str:
    return "sha256:" + hashlib.sha256(raw).hexdigest()


def _decode_owner_content(content: dict[str, Any], artifact_ref: str) -> bytes:
    """Decode only reversible byte encodings returned by Result owner content read."""
    raw = content.get("Content")
    if isinstance(raw, str):
        # ReadArtifactContentResult.Content is a JSON byte array on the HTTP
        # boundary; accepting a base64 string would invent an undocumented DTO.
        raise LinkError(f"owner content bytes must be an exact byte array for {artifact_ref}")
    if not isinstance(raw, list) or any(type(byte) is not int or byte < 0 or byte > 255 for byte in raw):
        raise LinkError(f"owner content bytes unavailable or malformed for report artifact {artifact_ref}")
    return bytes(raw)


def _verified_report(report: Any, report_path: Path, expected: dict[str, str]) -> dict[str, Any]:
    document = _object(report, "LSP evidence report")
    lint = evidence_lint.validate(document, report_path, "live", expected)
    if lint["status"] != "PENDING_UNVERIFIED":
        raise LinkError("report verifier 返回意外状态")
    return lint


def _join_typed_owner_export(export: Any, report: dict[str, Any], report_path: Path) -> dict[str, Any]:
    """Join the current typed DTO wire fields, without interpreting projection aliases."""
    root = _object(export, "WorkOrch typed owner export")
    result = _object(root.get("result"), "blocks/result Result model")
    material = _object(root.get("resultMaterial"), "blocks/result ResultMaterial")
    closeout = _object(material.get("RuntimeCloseout"), "ResultMaterial.RuntimeCloseout")
    delivery = _object(root.get("terminalDelivery"), "RuntimeTerminalDeliveryIdentity")
    artifacts_by_id = _index(_array(root.get("artifacts"), "Result owner Artifact DTOs"), "artifacts", "ID")
    contents_by_uri = {
        _string(row.get("URI"), f"artifactContents[{index}].URI"): _object(row, f"artifactContents[{index}]")
        for index, row in enumerate(_array(root.get("artifactContents"), "Result owner artifact content DTOs"))
    }
    validations_by_id = _index(_array(root.get("validationResults"), "Orchestrator ValidationResult DTOs"), "validationResults", "ID")

    expected = {key: report[key] for key in evidence_lint.BINDING}
    workspace_id = _string(result.get("WorkspaceID"), "Result.WorkspaceID")
    run_id = _string(result.get("RunID"), "Result.RunID")
    result_id = _string(result.get("ID"), "Result.ID")
    if workspace_id != expected["workspaceId"] or run_id != expected["runId"]:
        raise LinkError("Result owner workspace/run scope 与 report 不一致")
    if result.get("State") != "accepted":
        raise LinkError("Result owner state 不是 current accepted")
    if result.get("GoalID") != material.get("GoalID") or result.get("DoDID") != material.get("DoDID"):
        raise LinkError("Result 与 ResultMaterial Goal/DoD lineage 不一致")
    if result.get("WorkspaceID") != material.get("WorkspaceID") or result.get("RunID") != material.get("RunID"):
        raise LinkError("Result 与 ResultMaterial workspace/run lineage 不一致")
    result_hash = _string(result.get("TerminalMaterialHash"), "Result.TerminalMaterialHash")
    material_hash = _string(root.get("canonicalResultMaterialHash"), "canonicalResultMaterialHash")
    if not result_hash.startswith("result-material-sha256:") or result_hash != material_hash:
        raise LinkError("Result terminal material hash 与导出的 sealed ResultMaterial hash 不一致")

    expected_delivery = {
        "WorkspaceID": workspace_id,
        "RunID": run_id,
        "DefinitionRevision": closeout.get("DefinitionRevision"),
    }
    for key, expected_value in expected_delivery.items():
        if delivery.get(key) != expected_value:
            raise LinkError(f"RuntimeTerminalDeliveryIdentity.{key} 与 closeout/Result 不一致")
    if delivery.get("DefinitionRevision") != closeout.get("DefinitionRevision"):
        raise LinkError("terminal delivery / RuntimeCloseout definition revision 不一致")
    if closeout.get("StartupInputHash") != expected["startupIdentity"]:
        raise LinkError("RuntimeCloseout.StartupInputHash 与 report startup identity 不一致")
    if closeout.get("DefinitionRevision") != delivery.get("DefinitionRevision"):
        raise LinkError("RuntimeCloseout.DefinitionRevision 与 terminal delivery 不一致")
    if result.get("RuntimeCloseout") != closeout:
        raise LinkError("Result.RuntimeCloseout 与 ResultMaterial.RuntimeCloseout 不一致")
    if not closeout.get("CloseoutHash") or closeout.get("CloseoutHash") != root.get("runtimeCloseoutHash"):
        raise LinkError("RuntimeCloseout hash 缺失或与导出的 closeout hash 不一致")
    if not closeout.get("DefinitionRevision") or delivery.get("DefinitionRevision") != closeout.get("DefinitionRevision"):
        raise LinkError("RuntimeCloseout / terminal delivery definition revision 缺失或不一致")
    terminal_outcome = _object(closeout.get("TerminalOutcome"), "RuntimeCloseout.TerminalOutcome")
    if terminal_outcome.get("Kind") != "completed" or delivery.get("Outcome") != terminal_outcome:
        raise LinkError("RuntimeCloseout 与 terminal delivery outcome 不是 exact completed lineage")

    result_evidence_refs = _ref_array(result.get("EvidenceRefs"), "Result.EvidenceRefs")
    material_evidence_refs = _ref_array(material.get("EvidenceRefs"), "ResultMaterial.EvidenceRefs")
    if result_evidence_refs != material_evidence_refs:
        raise LinkError("Result / ResultMaterial evidence refs drift")
    closeout_evidence_refs = _ref_array(closeout.get("AcceptedEvidenceRefs"), "RuntimeCloseout.AcceptedEvidenceRefs")
    if closeout_evidence_refs != result_evidence_refs:
        raise LinkError("ResultMaterial evidence refs drift from RuntimeCloseout accepted evidence")
    result_artifact_ids = _ref_array(result.get("ArtifactRefs"), "Result.ArtifactRefs")
    material_artifact_refs = _ref_array(material.get("ArtifactRefs"), "ResultMaterial.ArtifactRefs")
    if result_artifact_ids != material_artifact_refs:
        raise LinkError("Result / ResultMaterial artifact refs drift")

    assessment = _object(material.get("DoDAssessment"), "ResultMaterial.DoDAssessment")
    if result.get("DoDAssessment") != assessment:
        raise LinkError("Result.DoDAssessment 与 ResultMaterial.DoDAssessment 不一致")
    if assessment.get("Satisfied") is not True:
        raise LinkError("ResultMaterial.DoDAssessment.Satisfied is not true")
    dod_items = _index(_array(assessment.get("Items"), "DoDAssessment.Items"), "DoDAssessment.Items", "Item")
    required_ids = set(evidence_lint.FACT_FIELDS)
    missing_dod_ids = sorted(required_ids - dod_items.keys())
    noncanonical_dod_ids = sorted(dod_items.keys() - required_ids)
    unsatisfied_ids = sorted(
        req_id for req_id in required_ids.intersection(dod_items)
        if dod_items[req_id].get("State") != "satisfied"
    )

    artifact_ids_in_result: set[str] = set()
    artifact_ref_to_owner_id: dict[str, str] = {}
    for artifact_id in result_artifact_ids:
        artifact = artifacts_by_id.get(artifact_id)
        if artifact is None:
            raise LinkError(f"Result.ArtifactRefs 引用未在 Result owner artifacts 导出：{artifact_id}")
        for key, wanted in (("ID", artifact_id), ("WorkspaceID", workspace_id), ("RunID", run_id), ("ResultID", result_id)):
            if artifact.get(key) != wanted:
                raise LinkError(f"Artifact.{key} scope/lineage mismatch: {artifact_id}")
        if artifact.get("State") != "available":
            raise LinkError(f"Artifact not available: {artifact_id}")
        uri = _string(artifact.get("URI"), f"Artifact[{artifact_id}].URI")
        artifact_ref_to_owner_id[uri] = artifact_id
        artifact_ref_to_owner_id[_string(artifact.get("SourceRef"), f"Artifact[{artifact_id}].SourceRef")] = artifact_id
        artifact_ids_in_result.add(artifact_id)

    report_artifacts = _index(_array(report.get("artifacts"), "report.artifacts"), "report.artifacts", "ref")
    report_artifact_refs = set(report_artifacts)
    artifact_content_joins: dict[str, str] = {}
    missing_artifact_links: list[str] = []
    for artifact_ref, report_artifact in report_artifacts.items():
        owner_artifact_id = artifact_ref_to_owner_id.get(artifact_ref)
        if owner_artifact_id is None:
            missing_artifact_links.append(artifact_ref)
            continue
        content = contents_by_uri.get(artifact_ref)
        if content is None:
            raise LinkError(f"owner content bytes metadata missing for report artifact ref {artifact_ref}")
        if content.get("WorkspaceID") != workspace_id or content.get("RunID") != run_id or content.get("URI") != artifact_ref:
            raise LinkError(f"artifact content scope mismatch: {artifact_ref}")
        content_hash = _string(content.get("ContentSHA256"), f"artifactContents[{artifact_ref}].ContentSHA256")
        if content.get("ArtifactID") != owner_artifact_id:
            raise LinkError(f"artifact content owner ID mismatch: {artifact_ref}")
        content_bytes = _decode_owner_content(content, artifact_ref)
        if _sha256(content_bytes) != content_hash:
            raise LinkError(f"owner artifact content bytes/hash mismatch: {artifact_ref}")
        report_file, errors = evidence_lint.dce._artifact_path_errors(report_path, report_artifact.get("path"))
        if errors or report_file is None:
            raise LinkError("; ".join(errors) or f"missing report artifact {artifact_ref}")
        actual_hash = _sha256(report_file.read_bytes())
        expected_hash = report_artifact.get("sha256")
        if content_hash != expected_hash or actual_hash != expected_hash:
            raise LinkError(f"report bytes / Result owner content hash mismatch: {artifact_ref}")
        artifact_content_joins[artifact_ref] = owner_artifact_id

    validations = list(validations_by_id.values())
    for validation_id, validation in validations_by_id.items():
        if validation.get("WorkspaceID") != workspace_id or validation.get("RunID") != run_id:
            raise LinkError(f"ValidationResult scope mismatch: {validation_id}")
        if validation.get("State") != "passed":
            raise LinkError(f"ValidationResult is not current passed: {validation_id}")
        if validation.get("EvidenceRef") and validation.get("EvidenceRef") not in result_evidence_refs:
            raise LinkError(f"ValidationResult EvidenceRef is not in Result evidence: {validation_id}")

    dod_evidence_missing = []
    for req_id in required_ids.intersection(dod_items):
        item = dod_items[req_id]
        refs = _ref_array(item.get("EvidenceRefs", []), f"DoDAssessment[{req_id}].EvidenceRefs")
        if not refs or not refs.issubset(result_evidence_refs):
            dod_evidence_missing.append(req_id)
    # Existing exports do not carry a typed linkage from ValidationResult to
    # RuntimeValidationObservation, nor report checks to sealed Shell native
    # attestation. Expose exact missing contract instead of inferring it.
    return {
        "resultId": result_id,
        "workspaceId": workspace_id,
        "runId": run_id,
        "goalId": result.get("GoalID"),
        "doDId": result.get("DoDID"),
        "resultState": result.get("State"),
        "terminalDeliveryId": delivery.get("TerminalDeliveryID"),
        "terminalMaterialHash": result_hash,
        "runtimeCloseoutHash": closeout.get("CloseoutHash"),
        "closeoutRuntimeStateRevision": closeout.get("RuntimeStateRevision"),
        "closeoutStartupInputHash": closeout.get("StartupInputHash"),
        "closeoutDefinitionRevision": closeout.get("DefinitionRevision"),
        "resultEvidenceRefs": sorted(result_evidence_refs),
        "resultArtifactIds": sorted(result_artifact_ids),
        "reportArtifactToOwnerArtifactIds": artifact_content_joins,
        "unmatchedReportArtifactRefs": sorted(missing_artifact_links),
        "validationResultIds": sorted(validations_by_id),
        "missingDoDRequirementIds": missing_dod_ids,
        "noncanonicalDoDItemIds": noncanonical_dod_ids,
        "unsatisfiedDoDRequirementIds": unsatisfied_ids,
        "DoDRequirementEvidenceMissing": sorted(dod_evidence_missing),
        "formalValidationJoinEstablished": False,
        "formalValidationJoinMissing": [
            "the owner export must bind each LSP report check to a sealed Shell managed-validation recipe and native-check attestation",
            "ValidationResult does not carry RuntimeValidationObservation recipe/protocol/check facts or report artifact hash",
            "no existing typed DTO binds deliveryCommit/serviceSourceIdentity to the report bytes; use deployment-side external expected pin only",
        ],
    }


def verify_link(report: Any, owner_export: Any, report_path: Path, expected_identities: Any) -> dict[str, Any]:
    expected = _object(expected_identities, "external expected identities")
    if set(expected) != evidence_lint.BINDING:
        raise LinkError("external expected identities keys must exactly match report BINDING")
    report_lint = _verified_report(report, report_path, expected)
    linked = _join_typed_owner_export(owner_export, report, report_path)
    missing = list(linked["formalValidationJoinMissing"])
    if linked["missingDoDRequirementIds"]:
        missing.append("ResultMaterial.DoDAssessment does not declare every fixed LSP requirement id")
    if linked["unsatisfiedDoDRequirementIds"]:
        missing.append("current DoDAssessment has non-satisfied LSP requirement items")
    if linked["noncanonicalDoDItemIds"]:
        missing.append("DoDAssessment includes unexpected item IDs: " + ", ".join(linked["noncanonicalDoDItemIds"]))
    if linked["DoDRequirementEvidenceMissing"]:
        missing.append("LSP DoD item evidence does not resolve through Result evidence refs")
    if linked["unmatchedReportArtifactRefs"]:
        missing.append("report artifact refs do not exactly match Result owner Artifact URI/SourceRef")
    missing.extend([
        "there is no existing LSP-specific producer entrypoint registry/producer implementation; a registry string is not a producer",
        "WorkOrch Result DTO is an owner-persisted terminal outcome, not a producer attestation for this external service report",
    ])
    return {
        "status": "PENDING_UNVERIFIED",
        "acceptanceEstablished": False,
        "reportLint": report_lint["status"],
        "typedOwnerJoin": linked,
        "missingRequiredFacts": missing,
        "limitation": "This is a software-dev acceptance-layer typed join only. It never approves acceptance without a real adapter producer, exact managed native checks, and externally pinned deployment/source identities.",
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("report", type=Path, help="read-only LSP report")
    parser.add_argument("owner_export", type=Path, help="typed Result/Artifact/ValidationResult DTO export")
    parser.add_argument("--expected-identities", type=Path, required=True, help="deployment-side external identity pins")
    args = parser.parse_args(argv)
    try:
        report = _load_json(args.report)
        owner_export = _load_json(args.owner_export)
        expected = _load_json(args.expected_identities)
        result = verify_link(report, owner_export, args.report.resolve(), expected)
    except (OSError, json.JSONDecodeError, ValueError, LinkError, evidence_lint.EvidenceError) as exc:
        print(json.dumps({"status": "PENDING_UNVERIFIED", "acceptanceEstablished": False, "error": f"typed join pending: {exc}"}, ensure_ascii=False), file=sys.stderr)
        return 2
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
