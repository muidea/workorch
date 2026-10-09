#!/usr/bin/env python3
"""Synthetic negative tests for typed owner joins; no evidence here is live."""
from __future__ import annotations

import copy
import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parent))
import verify_lsp_bridge_prd_acceptance_link as link  # noqa: E402
import verify_lsp_bridge_prd_evidence as lint  # noqa: E402
from test_verify_lsp_bridge_prd_evidence import EvidenceVerifierTests  # noqa: E402


class TypedOwnerJoinTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.report_path = self.root / "report.json"
        self.binding = {
            "workspaceId": "ws-synthetic",
            "runId": "run-synthetic",
            "startupIdentity": "sha256:" + "a" * 64,
            "deliveryCommit": "b" * 40,
            "serviceSourceIdentity": "sha256:" + "c" * 64,
        }
        self.report, self.owner_export = self._fixtures()

    def _fixtures(self):
        report = {
            "schemaVersion": lint.SCHEMA_VERSION,
            **self.binding,
            "producerCoverage": "unknown",
            "requirements": [],
            "artifacts": [],
        }
        files_by_ref = {}
        for req_id in lint.FACT_FIELDS:
            ref = f"artifact-{req_id.lower()}"
            payload = f"synthetic artifact body {req_id}".encode()
            path = self.root / f"{ref}.txt"
            path.write_bytes(payload)
            digest = hashlib.sha256(payload).hexdigest()
            files_by_ref[ref] = digest
            report["artifacts"].append({
                **self.binding,
                "ref": ref,
                "kind": "command_output" if req_id == "LSP-12" else "test_report",
                "path": path.name,
                "sha256": "sha256:" + digest,
            })
            report["requirements"].append({
                "id": req_id,
                "status": "passed",
                "evidence": [{
                    **self.binding,
                    "checkId": req_id,
                    "command": f"synthetic {req_id}",
                    "exitCode": 0,
                    "result": "passed",
                    "execution": "real",
                    "artifactRefs": [ref],
                    "facts": EvidenceVerifierTests.facts_for(req_id),
                }],
            })
        report["requirements"][11]["evidence"][0]["facts"]["restart"]["killEvidenceRef"] = "artifact-lsp-12"

        outcome = {"Kind": "completed", "Code": "runtime.completed"}
        evidence_refs = [f"evidence-{req.lower()}" for req in lint.FACT_FIELDS]
        artifact_ids = [f"artifact-owner-{req.lower()}" for req in lint.FACT_FIELDS]
        dod_items = [
            {"Item": req_id, "State": "satisfied", "EvidenceRefs": [f"evidence-{req_id.lower()}"]}
            for req_id in lint.FACT_FIELDS
        ]
        closeout = {
            "CloseoutID": "closeout-synthetic",
            "StartupInputHash": self.binding["startupIdentity"],
            "DefinitionRevision": "definition-revision-synthetic",
            "RuntimeStateRevision": 11,
            "TerminalOutcome": outcome,
            "AcceptedEvidenceRefs": evidence_refs,
            "RequirementOutcomes": [{"CoverageWorkItemID": f"cwi-{index}", "AssessmentState": "satisfied"} for index, _ in enumerate(lint.FACT_FIELDS)],
            "CloseoutHash": "closeout-hash-synthetic",
        }
        material = {
            "WorkspaceID": self.binding["workspaceId"],
            "RunID": self.binding["runId"],
            "GoalID": "goal-synthetic",
            "DoDID": "dod-synthetic",
            "EvidenceRefs": evidence_refs,
            "ArtifactRefs": artifact_ids,
            "DoDAssessment": {"Satisfied": True, "Items": dod_items},
            "RuntimeCloseout": closeout,
        }
        result = {
            "ID": "result-synthetic",
            "WorkspaceID": self.binding["workspaceId"],
            "RunID": self.binding["runId"],
            "GoalID": "goal-synthetic",
            "DoDID": "dod-synthetic",
            "State": "accepted",
            "EvidenceRefs": evidence_refs,
            "ArtifactRefs": artifact_ids,
            "DoDAssessment": material["DoDAssessment"],
            "RuntimeCloseout": closeout,
            "TerminalMaterialHash": "result-material-sha256:synthetic",
        }
        delivery = {
            "TerminalDeliveryID": "terminal-synthetic",
            "WorkspaceID": self.binding["workspaceId"],
            "RunID": self.binding["runId"],
            "AuthoritySnapshotID": "authority-synthetic",
            "DefinitionRevision": closeout["DefinitionRevision"],
            "EvidenceRevision": "evidence-revision-synthetic",
            "DispatchID": "dispatch-synthetic",
            "Outcome": outcome,
        }
        artifacts = []
        content_dtos = []
        report_bytes_by_ref = {}
        for req_id, artifact_id in zip(lint.FACT_FIELDS, artifact_ids):
            ref = f"artifact-{req_id.lower()}"
            artifacts.append({
                "ID": artifact_id,
                "WorkspaceID": self.binding["workspaceId"],
                "RunID": self.binding["runId"],
                "ResultID": result["ID"],
                "ValidationResultID": f"validation-{req_id.lower()}",
                "ArtifactType": "runtime_reference",
                "State": "available",
                "URI": ref,
                "SourceRef": ref,
                "TerminalDeliveryID": delivery["TerminalDeliveryID"],
                "MaterialHash": "artifact-material-sha256:synthetic",
            })
            report_bytes_by_ref[ref] = (self.root / f"{ref}.txt").read_bytes()
            content_dtos.append({
                "ArtifactID": artifact_id,
                "ResultID": result["ID"],
                "WorkspaceID": self.binding["workspaceId"],
                "RunID": self.binding["runId"],
                "URI": ref,
                "Content": list(report_bytes_by_ref[ref]),
                "ContentSHA256": "sha256:" + files_by_ref[ref],
            })
        validations = [{
            "ID": f"validation-{req_id.lower()}",
            "WorkspaceID": self.binding["workspaceId"],
            "RunID": self.binding["runId"],
            "ValidationKind": "managed",
            "State": "passed",
            "EvidenceRef": f"evidence-{req_id.lower()}",
            "EvidenceQuality": "valid",
            "ZeroWork": False,
        } for req_id in lint.FACT_FIELDS]
        owner_export = {
            "result": result,
            "resultMaterial": material,
            "canonicalResultMaterialHash": result["TerminalMaterialHash"],
            "runtimeCloseoutHash": closeout["CloseoutHash"],
            "terminalDelivery": delivery,
            "artifacts": artifacts,
            "artifactContents": content_dtos,
            "validationResults": validations,
        }
        return report, owner_export

    def verify(self, owner_export=None, expected=None):
        self.report_path.write_text(json.dumps(self.report), encoding="utf-8")
        return link.verify_link(
            self.report,
            self.owner_export if owner_export is None else owner_export,
            self.report_path,
            self.binding if expected is None else expected,
        )

    def test_complete_typed_join_remains_pending_without_producer_and_managed_check(self):
        outcome = self.verify()
        self.assertEqual(outcome["status"], "PENDING_UNVERIFIED")
        self.assertFalse(outcome["acceptanceEstablished"])
        typed = outcome["typedOwnerJoin"]
        self.assertEqual(typed["missingDoDRequirementIds"], [])
        self.assertEqual(typed["unsatisfiedDoDRequirementIds"], [])
        self.assertEqual(typed["unmatchedReportArtifactRefs"], [])
        self.assertEqual(len(typed["reportArtifactToOwnerArtifactIds"]), 20)
        self.assertFalse(typed["formalValidationJoinEstablished"])
        self.assertTrue(any("producer entrypoint" in value for value in outcome["missingRequiredFacts"]))

    def test_external_identity_pin_is_mandatory_and_exact(self):
        with self.assertRaisesRegex(link.LinkError, "must exactly match"):
            self.verify(expected={"runId": self.binding["runId"]})
        drifted = dict(self.binding, deliveryCommit="d" * 40)
        with self.assertRaisesRegex(lint.EvidenceError, "deliveryCommit differs"):
            self.verify(expected=drifted)

    def test_result_and_material_scope_or_terminal_hash_drift_rejected(self):
        for selector, mutation, expected_error in (
            (lambda value: value["result"], lambda row: row.__setitem__("RunID", "other-run"), "Result owner workspace/run"),
            (lambda value: value["result"], lambda row: row.__setitem__("TerminalMaterialHash", "changed"), "terminal material hash"),
            (lambda value: value["resultMaterial"], lambda row: row.__setitem__("GoalID", "other-goal"), "Goal/DoD lineage"),
        ):
            export = copy.deepcopy(self.owner_export)
            mutation(selector(export))
            with self.subTest(expected_error=expected_error), self.assertRaisesRegex(link.LinkError, expected_error):
                self.verify(export)

    def test_terminal_closeout_scope_hash_and_outcome_are_joined_exactly(self):
        cases = (
            ("StartupInputHash", "sha256:" + "d" * 64, "StartupInputHash"),
            ("CloseoutHash", "drift", "closeout hash"),
            ("DefinitionRevision", "other-definition", "completed lineage"),
            ("TerminalOutcome", {"Kind": "failed", "Code": "failed"}, "completed lineage"),
        )
        for key, value, message in cases:
            export = copy.deepcopy(self.owner_export)
            export["resultMaterial"]["RuntimeCloseout"][key] = value
            export["result"]["RuntimeCloseout"][key] = value
            if key == "TerminalOutcome":
                export["terminalDelivery"]["Outcome"] = value
            if key == "DefinitionRevision":
                export["terminalDelivery"]["DefinitionRevision"] = value
                export["terminalDelivery"]["Outcome"]["Kind"] = "failed"
            with self.subTest(key=key), self.assertRaisesRegex(link.LinkError, message):
                self.verify(export)

    def test_artifact_scope_lineage_and_report_content_hash_must_match(self):
        for field, value, message in (
            ("WorkspaceID", "other-workspace", "Artifact.WorkspaceID"),
            ("ResultID", "other-result", "Artifact.ResultID"),
            ("State", "deleted", "not available"),
        ):
            export = copy.deepcopy(self.owner_export)
            export["artifacts"][0][field] = value
            with self.subTest(field=field), self.assertRaisesRegex(link.LinkError, message):
                self.verify(export)
        export = copy.deepcopy(self.owner_export)
        export["artifactContents"][0]["Content"] = "not-bytes"
        with self.assertRaisesRegex(link.LinkError, "exact byte array"):
            self.verify(export)
        export = copy.deepcopy(self.owner_export)
        export["artifactContents"][0]["ContentSHA256"] = "sha256:" + "f" * 64
        with self.assertRaisesRegex(link.LinkError, "content bytes/hash mismatch"):
            self.verify(export)

    def test_validation_result_scope_state_and_evidence_link_required(self):
        mutations = (
            ("RunID", "other-run", "scope mismatch"),
            ("State", "failed", "current passed"),
            ("EvidenceRef", "orphan", "not in Result evidence"),
        )
        for field, value, message in mutations:
            export = copy.deepcopy(self.owner_export)
            export["validationResults"][0][field] = value
            with self.subTest(field=field), self.assertRaisesRegex(link.LinkError, message):
                self.verify(export)

    def test_missing_or_non_satisfied_lsp_dod_items_do_not_link(self):
        export = copy.deepcopy(self.owner_export)
        export["resultMaterial"]["DoDAssessment"]["Items"][0]["State"] = "passed"
        export["result"]["DoDAssessment"]["Items"][0]["State"] = "passed"
        outcome = self.verify(export)
        self.assertIn("LSP-01", outcome["typedOwnerJoin"]["unsatisfiedDoDRequirementIds"])
        export["resultMaterial"]["DoDAssessment"]["Items"].pop()
        export["result"]["DoDAssessment"]["Items"].pop()
        outcome = self.verify(export)
        self.assertEqual(len(outcome["typedOwnerJoin"]["missingDoDRequirementIds"]), 2)
        self.assertEqual(outcome["typedOwnerJoin"]["noncanonicalDoDItemIds"], [])

    def test_result_and_closeout_evidence_ref_drift_rejected(self):
        export = copy.deepcopy(self.owner_export)
        export["validationResults"] = []
        export["result"]["EvidenceRefs"] = export["result"]["EvidenceRefs"][:-1]
        with self.assertRaisesRegex(link.LinkError, "Result / ResultMaterial evidence refs drift"):
            self.verify(export)
        export = copy.deepcopy(self.owner_export)
        export["validationResults"] = []
        export["resultMaterial"]["RuntimeCloseout"]["AcceptedEvidenceRefs"] = export["resultMaterial"]["RuntimeCloseout"]["AcceptedEvidenceRefs"][:-1]
        with self.assertRaisesRegex(link.LinkError, "RuntimeCloseout accepted evidence"):
            self.verify(export)

    def test_result_and_material_artifact_ref_drift_rejected(self):
        export = copy.deepcopy(self.owner_export)
        export["result"]["ArtifactRefs"] = export["result"]["ArtifactRefs"][:-1]
        with self.assertRaisesRegex(link.LinkError, "Result / ResultMaterial artifact refs drift"):
            self.verify(export)

    def test_report_artifact_missing_owner_link_stays_explicit_pending(self):
        export = copy.deepcopy(self.owner_export)
        export["artifacts"][0]["URI"] = "owner-uri-different"
        export["artifacts"][0]["SourceRef"] = "owner-uri-different"
        outcome = self.verify(export)
        self.assertEqual(outcome["status"], "PENDING_UNVERIFIED")
        self.assertIn("artifact-lsp-01", outcome["typedOwnerJoin"]["unmatchedReportArtifactRefs"])


if __name__ == "__main__":
    unittest.main()
