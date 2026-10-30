#!/usr/bin/env python3
"""STEP02 verifier pure contracts; no process, HTTP or live provider calls."""
import importlib.util
import copy
import json
from pathlib import Path
import tomllib
import unittest

spec = importlib.util.spec_from_file_location("step02", Path(__file__).with_name("verify-workspace-candidate-step02.py"))
step02 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(step02)


class Step02VerifierTest(unittest.TestCase):
    def source_payload(self):
        expected = {'derived-dod:template:000003': 'terminal_owner_operation_receipt',
                    'derived-dod:template:000004': 'terminal_owner_snapshot',
                    'derived-dod:template:000013': 'terminal_result_evidence_index',
                    'derived-dod:user:000001': None}
        return {'candidateAcceptance': [{'CandidateItemRef': ref} for ref in expected],
                'sourceApplicableVerificationDeclarations': [
                    {'candidateItemRef': ref, 'sourceScope': 'dodItem', 'sourceId': f'source-{i}',
                     'declarationBindingMode': 'one_applicable_reference',
                     'declarations': [{'profileId': p, 'evidenceRequirementId': '*', 'scope': 'dodItem'} for p in ('first', 'second')],
                     'mergedRequiredEvidenceKinds': ['validation'],
                     'mergedRequiredConsumers': [] if kind is None else [{'Kind': kind, 'ConsumptionPoint': 'post_terminal_delivery', 'TerminalContractRef': 'terminal-v1'}]}
                    for i, (ref, kind) in enumerate(expected.items())]}

    def test_consumers_apply_to_source_even_with_multiple_declaration_choices(self):
        payload = self.source_payload()
        before = copy.deepcopy(payload)
        step02.validate_source_consumers(payload)
        self.assertEqual(payload, before)
        mutations = (
            lambda p: p['sourceApplicableVerificationDeclarations'][0].update(mergedRequiredConsumers=[]),
            lambda p: p['sourceApplicableVerificationDeclarations'][-1].update(mergedRequiredConsumers=[{'Kind': 'terminal_owner_snapshot'}]),
            lambda p: p['sourceApplicableVerificationDeclarations'][0].update(candidateItemRef='other'),
            lambda p: p['sourceApplicableVerificationDeclarations'][0].update(declarationBindingMode='all_references'),
            lambda p: p['sourceApplicableVerificationDeclarations'][0]['declarations'][0].update(consumers=[]),
            lambda p: p['sourceApplicableVerificationDeclarations'].pop(),
            lambda p: p['sourceApplicableVerificationDeclarations'].__setitem__(1, copy.deepcopy(p['sourceApplicableVerificationDeclarations'][0])),
        )
        for mutate in mutations:
            bad = copy.deepcopy(payload)
            mutate(bad)
            with self.subTest(mutate=mutate), self.assertRaises(RuntimeError):
                step02.validate_source_consumers(bad)

    def test_review_blocked_is_not_settled_until_all_six_critics_finish(self):
        view = {"state": "review_blocked", "proposal": {"reviews": []}}
        for kind in step02.KINDS:
            self.assertFalse(step02.compilation_settled(view))
            view["proposal"]["reviews"].append({"kind": kind, "status": "FAIL"})
        self.assertTrue(step02.compilation_settled(view))
        view["proposal"]["reviews"][-1]["kind"] = step02.KINDS[0]
        self.assertFalse(step02.compilation_settled(view))
        self.assertTrue(step02.compilation_settled({"state": "failed"}))
        self.assertFalse(step02.compilation_settled({"state": "outcome_unresolved"}))

    def test_config_round_trip_preserves_credential_bytes_without_logging(self):
        config = {"endpointName": "测试", "llm": {"aiProxy": {"baseURL": "http://127.0.0.1", "apiKey": 'synthetic-"\\\n-key'}, "modelOverrides": {"model.with.dot": {"contextWindowTokens": 128000, "operations": ["responses"]}}}, "runtime": {"enabled": True}}
        self.assertEqual(tomllib.loads(step02.toml_document(config)), config)

    def test_exact_prompt_payload_excludes_contract_examples(self):
        payload = {"candidateAcceptance": [{"CandidateItemRef": "source", "Text": "原文"}], "goal": {"Text": "原目标"}}
        self.assertEqual(step02.prompt_payload('contract {"shape":true}\n' + json.dumps(payload)), payload)

    def test_ambiguous_or_truncated_prompt_fails_closed(self):
        for text in ('{}', '{"outputShape":', '{"outputShape":{}}\n{"candidateAcceptance":[]}'):
            with self.subTest(text=text), self.assertRaises(RuntimeError):
                step02.prompt_payload(text)

    def test_repository_envelope_requires_exact_workspace(self):
        record = {"ID": "ws", "DefinitionAuthoring": {"Revision": 1}}
        self.assertEqual(step02.workspace_record({"ws": record}, "ws"), record)
        for document in (record, {}, {"other": record}, {"ws": record, "other": record}, {"ws": {"ID": "other", "DefinitionAuthoring": {"Revision": 1}}}):
            with self.subTest(document=document), self.assertRaises(RuntimeError):
                step02.workspace_record(document, "ws")

    def test_candidate_success_requires_nonblank_declaration_even_with_six_pass(self):
        source = {"input": {"goalText": "固定目标", "acceptanceRequirements": ["原始要求"]}}
        view = {"input": copy.deepcopy(source["input"]), "state": "ready_to_confirm", "proposal": {
            "goalText": "固定目标", "proposalHash": "hash",
            "requirements": [{"id": "goal", "text": "固定目标", "required": True}, {"id": "required", "text": "原始要求", "required": True}],
            "obligations": [],
            "reviews": [{"kind": kind, "status": "PASS", "reason": "具体审查依据", "evidenceRefs": ["goal"], "requirementIds": ["goal", "required"], "specificationHash": "hash"} for kind in step02.KINDS],
            "verifications": [{"status": "DECLARED", "declaration": "已发布的适用能力，尚未执行。", "profileId": "p", "evidenceRequirementId": "*", "scope": "dodItem"}],
        }}
        step02.validate_candidate(view, source)
        for declaration in (None, "", " \t\n", "\u0085\u3000"):
            invalid = copy.deepcopy(view)
            verification = invalid["proposal"]["verifications"][0]
            if declaration is None:
                del verification["declaration"]
            else:
                verification["declaration"] = declaration
            with self.subTest(declaration=declaration), self.assertRaises(RuntimeError):
                step02.validate_candidate(invalid, source)


if __name__ == "__main__":
    unittest.main()
