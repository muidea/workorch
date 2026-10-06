#!/usr/bin/env python3
"""仅使用临时 synthetic JSON 的回归测试。"""
from __future__ import annotations

import copy
import hashlib
import json
from pathlib import Path
import sys
import tempfile
import unittest
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parent))
import verify_lsp_bridge_prd_evidence as verifier  # noqa: E402


class EvidenceVerifierTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.manifest_path = self.root / "report.json"
        self.binding = {
            "workspaceId": "ws-synthetic", "runId": "run-synthetic",
            "startupIdentity": "sha256:" + "a" * 64,
            "deliveryCommit": "b" * 40,
            "serviceSourceIdentity": "sha256:" + "c" * 64,
        }
        self.document = {
            "schemaVersion": verifier.SCHEMA_VERSION, **self.binding,
            "producerCoverage": "unknown", "requirements": [], "artifacts": [],
        }
        for req_id in verifier.FACT_FIELDS:
            ref = f"synthetic-{req_id.lower()}"
            content = f"synthetic output {req_id}".encode()
            file_path = self.root / "artifacts" / f"{ref}.txt"
            file_path.parent.mkdir(exist_ok=True)
            file_path.write_bytes(content)
            self.document["artifacts"].append({
                **self.binding, "ref": ref, "kind": "command_output" if req_id == "LSP-12" else "test_report",
                "path": f"artifacts/{ref}.txt", "sha256": "sha256:" + hashlib.sha256(content).hexdigest(),
            })
            if req_id == "LSP-19":
                docs_content = b"synthetic documentation index"
                docs_path = self.root / "artifacts" / "docs.txt"
                docs_path.write_bytes(docs_content)
                self.document["artifacts"].append({
                    **self.binding, "ref": "synthetic-lsp-19-docs", "kind": "documentation_index",
                    "path": "artifacts/docs.txt", "sha256": "sha256:" + hashlib.sha256(docs_content).hexdigest(),
                })
            self.document["requirements"].append({
                "id": req_id, "status": "passed", "evidence": [{
                    **self.binding, "checkId": req_id, "command": f"synthetic {req_id}",
                    "exitCode": 0, "result": "passed", "execution": "fixture",
                    "artifactRefs": [ref] + (["synthetic-lsp-19-docs"] if req_id == "LSP-19" else []),
                    "facts": self.facts_for(req_id),
                }],
            })

    @staticmethod
    def facts_for(req_id):
        if req_id == "LSP-01":
            return {"tools": sorted(verifier.TOOLS)}
        if req_id == "LSP-02":
            return {"definitions": {lang: {"passed": True, "symbol": "Target", "path": f"src/{lang}/main", "line": 1, "execution": "real"} for lang in verifier.LANGUAGES}}
        if req_id == "LSP-03":
            return {"unsaved": {k: True for k in ("syncApplied", "discardRestored", "hoverLatest", "definitionLatest", "diagnosticsLatest", "unchangedDidChangeSkipped")}}
        if req_id == "LSP-04":
            return {"diagnostic": {"message": "synthetic error", "line": 1, "column": 1, "severity": "error"}}
        if req_id == "LSP-05":
            output = lambda total: {"limit": 10, "total": total, "returned": min(total, 10), "truncated": total > 10, "complete": total <= 10, "reason": "max_output_limit" if total > 10 else "within_limit"}
            return {"boundedOutputs": {
                "hover": output(3), "diagnostics": output(3), "references": output(3),
                "overflowProbe": {name: output(12) for name in ("hover", "diagnostics", "references")},
            }}
        if req_id == "LSP-06":
            return {"status": {"instanceState": "ready", "activity": "active", "restartCount": 0, "dependencyPresent": True, "dependencyRunning": True}}
        if req_id == "LSP-07":
            return {"shutdown": {"singleClosed": True, "allClosed": True, "reinitialized": True}}
        if req_id == "LSP-08":
            return {"repair": {"defaultIsAdvice": True, "applySafeOnly": True, "allowedAction": "restart_exited_instance"}}
        if req_id == "LSP-09":
            return {key: {"value": 20.0, "unit": "ms", "warm": True, "scope": "mcp_internal_excluding_lsp"} for key in ("definitionLatency", "hoverLatency")}
        if req_id == "LSP-10":
            return {"rss": {"value": 40.0, "unit": "MB", "scope": "mcp_process_excluding_children", "simultaneous": True}, "languages": ["Go", "Rust", "Python"]}
        if req_id == "LSP-11":
            return {"configuration": {"keys": ["defaultTimeout", "maxReferences", "maxDiagnostics", "maxHoverCharacters"], "runtimeEffective": True}}
        if req_id == "LSP-12":
            return {"restart": {"trigger": "manual_kill", "killEvidenceRef": "synthetic-lsp-12", "nextRequestRestarted": True, "initialized": True}}
        if req_id == "LSP-13":
            return {"lifecycle": {"idleTtlReclaimed": True, "capacityLruReclaimed": True, "onlyIdleReclaimed": True}}
        if req_id == "LSP-14":
            return {"pathUri": {"linuxPassed": True, "windowsClaimed": False, "windowsPassed": False}}
        if req_id == "LSP-15":
            return {"concurrency": {"goroutines": 10, "requestIdsMatched": True, "allCompleted": True}, "race": True}
        if req_id == "LSP-16":
            return {"build": ["go build -buildvcs=false ./..."], "test": ["go test ./..."], "raceTest": ["go test -race ./..."]}
        if req_id == "LSP-17":
            return {"realGoLsp": {"passed": True, "execution": "real", "fixture": False, "server": "gopls"}}
        if req_id == "LSP-18":
            return {"configExample": ["runtime", "performance", "languageCommands"]}
        if req_id == "LSP-19":
            return {"documents": ["integration", "development", "performance", "dod", "systemPrompt"]}
        if req_id == "LSP-20":
            return {"reproducibleDelivery": {"sourcePresent": True, "testsPresent": True, "buildReproducible": True}}
        raise AssertionError(req_id)

    def validate(self, doc=None, mode="selftest", expected=None):
        return verifier.validate(doc if doc is not None else self.document, self.manifest_path, mode, expected)

    def test_complete_synthetic_report_is_not_acceptance(self):
        result = self.validate()
        self.assertEqual(result["status"], "SYNTHETIC_SELFTEST_OK")
        self.assertFalse(result["acceptanceEstablished"])
        self.assertEqual(result["requirementsValidated"], 20)

    def test_live_requires_explicit_identity_and_stays_pending(self):
        with self.assertRaisesRegex(verifier.EvidenceError, "external trusted"):
            self.validate(mode="live")
        for row in self.document["requirements"]:
            row["evidence"][0]["execution"] = "real"
        result = self.validate(mode="live", expected=self.binding)
        self.assertEqual(result["status"], "PENDING_UNVERIFIED")
        self.assertFalse(result["acceptanceEstablished"])

    def test_live_rejects_fixtures(self):
        with self.assertRaisesRegex(verifier.EvidenceError, "fixture/mock"):
            self.validate(mode="live", expected=self.binding)

    def test_missing_duplicate_unknown_skipped_and_malformed_rejected(self):
        variants = []
        doc = copy.deepcopy(self.document); doc["requirements"].pop(); variants.append(doc)
        doc = copy.deepcopy(self.document); doc["requirements"].append(copy.deepcopy(doc["requirements"][0])); variants.append(doc)
        doc = copy.deepcopy(self.document); doc["requirements"][0]["id"] = "LSP-99"; variants.append(doc)
        doc = copy.deepcopy(self.document); doc["requirements"][0]["status"] = "skipped"; variants.append(doc)
        doc = copy.deepcopy(self.document); doc["requirements"][0]["evidence"][0]["facts"]["tools"].pop(); variants.append(doc)
        for case in variants:
            with self.subTest(index=variants.index(case)), self.assertRaises(verifier.EvidenceError):
                self.validate(case)

    def test_each_requirement_requires_typed_facts_not_command_label(self):
        for row in self.document["requirements"]:
            req_id = row["id"]
            doc = copy.deepcopy(self.document)
            selected = next(r for r in doc["requirements"] if r["id"] == req_id)
            selected["evidence"][0]["facts"] = {}
            with self.subTest(requirement=req_id), self.assertRaises(verifier.EvidenceError):
                self.validate(doc)

    def test_exact_tools_languages_and_unsaved_contract(self):
        doc = copy.deepcopy(self.document)
        doc["requirements"][0]["evidence"][0]["facts"]["tools"].append("invented")
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        doc = copy.deepcopy(self.document)
        del doc["requirements"][1]["evidence"][0]["facts"]["definitions"]["Shell"]
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        doc = copy.deepcopy(self.document)
        doc["requirements"][2]["evidence"][0]["facts"]["unsaved"]["hoverLatest"] = False
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)

    def test_rejects_bad_artifact_path_hash_and_scope(self):
        for key, value in (("path", "../outside"), ("sha256", "sha256:" + "f" * 64), ("runId", "other")):
            doc = copy.deepcopy(self.document); doc["artifacts"][0][key] = value
            with self.subTest(key=key), self.assertRaises(verifier.EvidenceError): self.validate(doc)
        target = self.root / "target"; target.write_text("synthetic")
        link = self.root / "artifacts" / "link"; link.symlink_to(target)
        doc = copy.deepcopy(self.document); doc["artifacts"][0]["path"] = "artifacts/link"
        doc["artifacts"][0]["sha256"] = "sha256:" + hashlib.sha256(b"synthetic").hexdigest()
        with self.assertRaisesRegex(verifier.EvidenceError, "symlink"): self.validate(doc)

    def test_rejects_mock_real_lsp_and_invalid_performance_values(self):
        doc = copy.deepcopy(self.document); doc["requirements"][16]["evidence"][0]["facts"]["realGoLsp"]["fixture"] = True
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        for value in (-1, True, float("nan"), float("inf"), 50):
            doc = copy.deepcopy(self.document); doc["requirements"][8]["evidence"][0]["facts"]["definitionLatency"]["value"] = value
            with self.subTest(value=value), self.assertRaises(verifier.EvidenceError): self.validate(doc)
        doc = copy.deepcopy(self.document); doc["requirements"][9]["evidence"][0]["facts"]["rss"]["scope"] = "including_children"
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)

    def test_cli_rejects_duplicate_keys_surrogates_and_overflow(self):
        path = self.root / "report.json"
        cases = (
            '{"schemaVersion":"one","schemaVersion":"two"}',
            '{"text":"\\ud800"}',
            '{"number":1e999}',
        )
        for content in cases:
            path.write_text(content, encoding="utf-8")
            with self.subTest(content=content):
                self.assertEqual(verifier.main([str(path), "--mode", "selftest"]), 1)

    def test_rejects_unhashable_identifiers_and_overflowing_numbers(self):
        doc = copy.deepcopy(self.document)
        doc["requirements"][0]["id"] = []
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        for value in (10**10000,):
            doc = copy.deepcopy(self.document)
            doc["requirements"][8]["evidence"][0]["facts"]["hoverLatency"]["value"] = value
            with self.assertRaises(verifier.EvidenceError): self.validate(doc)

    def test_bounded_outputs_require_real_limit_exhaustion(self):
        for mutation in (
            {"returned": 9, "truncated": True, "complete": False, "reason": "max_output_limit"},
            {"returned": 11, "truncated": True, "complete": False, "reason": "max_output_limit"},
            {"returned": 10, "truncated": True, "complete": False, "reason": "within_limit"},
        ):
            doc = copy.deepcopy(self.document)
            doc["requirements"][4]["evidence"][0]["facts"]["boundedOutputs"]["hover"].update(mutation)
            with self.assertRaises(verifier.EvidenceError): self.validate(doc)

    def test_restart_rss_language_constraints(self):
        doc = copy.deepcopy(self.document)
        doc["requirements"][11]["evidence"][0]["facts"]["restart"]["trigger"] = "child_exit_observed"
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        doc = copy.deepcopy(self.document)
        doc["requirements"][9]["evidence"][0]["facts"]["languages"] = ["Go", "Rust", "Java"]
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        doc = copy.deepcopy(self.document)
        doc["requirements"][9]["evidence"][0]["facts"]["rss"]["simultaneous"] = False
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)

    def test_lsp05_requires_overflow_probes_for_all_bounded_outputs(self):
        doc = copy.deepcopy(self.document)
        bounded = doc["requirements"][4]["evidence"][0]["facts"]["boundedOutputs"]
        bounded["overflowProbe"] = {name: {"limit": 10, "total": 3, "returned": 3, "truncated": False, "complete": True, "reason": "within_limit"} for name in ("hover", "diagnostics", "references")}
        with self.assertRaisesRegex(verifier.EvidenceError, "must exercise total > limit"):
            self.validate(doc)

    def test_lsp12_malformed_facts_raise_contract_error(self):
        for malformed in (None, [], {"restart": None}, {"restart": []}):
            doc = copy.deepcopy(self.document)
            doc["requirements"][11]["evidence"][0]["facts"] = malformed
            with self.subTest(value=malformed), self.assertRaises(verifier.EvidenceError):
                self.validate(doc)

    def test_repair_booleans_and_bad_collection_types_rejected(self):
        doc = copy.deepcopy(self.document)
        doc["requirements"][7]["evidence"][0]["facts"]["repair"]["defaultIsAdvice"] = []
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        for field in ("defaultIsAdvice", "applySafeOnly"):
            doc = copy.deepcopy(self.document)
            doc["requirements"][7]["evidence"][0]["facts"]["repair"][field] = False
            with self.subTest(field=field), self.assertRaises(verifier.EvidenceError): self.validate(doc)
        for field in ("defaultIsAdvice", "applySafeOnly"):
            doc = copy.deepcopy(self.document)
            doc["requirements"][7]["evidence"][0]["facts"]["repair"][field] = "true"
            with self.subTest(field=field), self.assertRaises(verifier.EvidenceError): self.validate(doc)
        doc = copy.deepcopy(self.document)
        doc["artifacts"][0]["kind"] = []
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        doc = copy.deepcopy(self.document)
        doc["requirements"][0]["evidence"][0]["execution"] = []
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        for refs in ([["nested"]], [{}]):
            doc = copy.deepcopy(self.document)
            doc["requirements"][0]["evidence"][0]["artifactRefs"] = refs
            with self.assertRaises(verifier.EvidenceError): self.validate(doc)

        doc = copy.deepcopy(self.document)
        doc["requirements"][0]["evidence"][0]["artifactRefs"] = ["missing"]
        with self.assertRaisesRegex(verifier.EvidenceError, "unknown artifact"): self.validate(doc)
        doc = copy.deepcopy(self.document)
        doc["requirements"][0]["evidence"][0]["artifactRefs"] = ["missing"]
        with self.assertRaisesRegex(verifier.EvidenceError, "unknown artifact"): self.validate(doc)
        for kind in ("source_snapshot", "documentation_index"):
            doc = copy.deepcopy(self.document)
            doc["requirements"][0]["evidence"][0]["artifactRefs"] = ["synthetic-lsp-19-docs"]
            with self.subTest(kind=kind):
                next(item for item in doc["artifacts"] if item["ref"] == "synthetic-lsp-19-docs")["kind"] = kind
                with self.assertRaisesRegex(verifier.EvidenceError, "source/docs alone"): self.validate(doc)

    def test_manual_kill_ref_must_resolve_scoped_artifact(self):
        doc = copy.deepcopy(self.document)
        doc["requirements"][11]["evidence"][0]["facts"]["restart"]["killEvidenceRef"] = "unknown"
        with self.assertRaisesRegex(verifier.EvidenceError, "listed in artifactRefs"): self.validate(doc)
        doc = copy.deepcopy(self.document)
        doc["requirements"][11]["evidence"][0]["facts"]["restart"]["killEvidenceRef"] = "unknown"
        doc["requirements"][11]["evidence"][0]["artifactRefs"].append("unknown")
        with self.assertRaisesRegex(verifier.EvidenceError, "unknown artifact"): self.validate(doc)
        doc = copy.deepcopy(self.document)
        kill = doc["artifacts"][11]
        kill["runId"] = "different-run"
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)
        doc = copy.deepcopy(self.document)
        doc["artifacts"][11]["kind"] = "test_report"
        with self.assertRaisesRegex(verifier.EvidenceError, "command output"): self.validate(doc)

    def test_malformed_artifact_paths_commits_and_direct_surrogates_rejected(self):
        for path in (None, [], {}, 42, True, ""):
            doc = copy.deepcopy(self.document)
            doc["artifacts"][0]["path"] = path
            with self.subTest(path=path), self.assertRaises(verifier.EvidenceError):
                self.validate(doc)
        for width in (39, 41, 63, 65):
            doc = copy.deepcopy(self.document)
            doc["deliveryCommit"] = "b" * width
            with self.subTest(width=width), self.assertRaises(verifier.EvidenceError):
                self.validate(doc)
        doc = copy.deepcopy(self.document)
        doc["requirements"][0]["evidence"][0]["command"] = chr(0xD800)
        with self.assertRaisesRegex(verifier.EvidenceError, "surrogate"):
            self.validate(doc)

    def test_bounded_output_requires_total_count_not_only_limit_hit(self):
        doc = copy.deepcopy(self.document)
        bounded = doc["requirements"][4]["evidence"][0]["facts"]["boundedOutputs"]["hover"]
        bounded.update(total=10, returned=10, truncated=True, complete=False, reason="max_output_limit")
        with self.assertRaises(verifier.EvidenceError): self.validate(doc)

    def test_schema_accepts_flat_closed_objects_and_rejects_extra_properties(self):
        schema = json.loads((Path(__file__).with_name("lsp-bridge-prd-evidence.schema.json")).read_text())
        defs = schema["$defs"]
        self.assertEqual(set(defs["evidence"]["properties"]), set(defs["evidence"]["required"]))
        self.assertEqual(set(defs["artifact"]["properties"]), set(defs["artifact"]["required"]))
        self.assertFalse(defs["evidence"]["additionalProperties"])
        self.assertFalse(defs["artifact"]["additionalProperties"])
        self.assertIn("facts", defs["evidence"]["properties"])
        self.assertNotIn("allOf", defs["evidence"])
        self.assertNotIn("allOf", defs["artifact"])
        valid_evidence: dict[str, Any] = {key: "synthetic" for key in defs["evidence"]["required"]}
        valid_evidence.update(exitCode=0, artifactRefs=["ref"], facts={})
        self.assertEqual(set(valid_evidence), set(defs["evidence"]["properties"]))
        self.assertFalse(defs["evidence"]["additionalProperties"])
        self.assertNotEqual(set({**valid_evidence, "extra": "forbidden"}), set(defs["evidence"]["properties"]))

    def test_cli_is_read_only_and_live_requires_explicit_expectations(self):
        path = self.root / "report.json"; path.write_text(json.dumps(self.document), encoding="utf-8")
        original = path.read_bytes()
        self.assertEqual(verifier.main([str(path), "--mode", "selftest"]), 0)
        self.assertEqual(path.read_bytes(), original)
        self.assertEqual(verifier.main([str(path), "--mode", "live"]), 1)


if __name__ == "__main__":
    unittest.main()
