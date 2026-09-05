"""Regression coverage for the provider-visible, bounded semantic proof contract."""

import copy
import json
import unittest

from verify_mcp_stdio_mock import MockContractError, self_test, semantic_plan_payload


class SemanticProofContractTest(unittest.TestCase):
    def setUp(self):
        self.authority = {
            "manifest": {
                "hash": "manifest-batch",
                "definitionSeedHash": "seed",
                "coverageItems": [{"id": "cwi-002"}, {"id": "cwi-001"}],
                # Deliberately not in batch order; IDs are not declaration refs.
                "proofObligations": [
                    {"coverageWorkItemId": "cwi-001", "declarationRef": "definition/dodItem/dod-007"},
                    {"coverageWorkItemId": "cwi-002", "declarationRef": "definition/dodItem/dod-003"},
                ],
                "ownerCatalogs": [{
                    "ownerModuleId": "blocks/mcp", "selectorRefs": ["mcp-test"],
                    "validationRecipeRefs": ["mcp/tool_call/mcp-test/repo.summary"],
                }],
            },
            "definitionSeed": {"hash": "seed"},
            "schema": {"properties": {
                "schemaVersion": {"const": "workorch.runtime_semantic_plan_document.v2"},
            }},
            "planStep": "proof_batch", "batchId": "batch-001", "batchSequence": 0,
            "batchCoverageWorkItemIds": ["cwi-002", "cwi-001"],
        }

    def payload(self, authority=None):
        # A diagnostic or original definition must never supply current identity.
        prompt = "### C3.semantic-authority\n" + json.dumps(authority or self.authority)
        prompt += '\n### C4.execution-tail\n{"declarationRef":"stale/diagnostic"}'
        return semantic_plan_payload(prompt, "mcp-test")

    def test_current_declarations_match_each_cwi_in_batch_order(self):
        proposals = self.payload()["proposals"]
        self.assertEqual(
            [(item["coverageWorkItemId"], item["declarationRef"]) for item in proposals],
            [("cwi-002", "definition/dodItem/dod-003"), ("cwi-001", "definition/dodItem/dod-007")],
        )

    def test_current_ref_is_opaque_not_reconstructed_from_definition(self):
        obligation = self.authority["manifest"]["proofObligations"][0]
        obligation["declarationRef"] = "repair/convergence-opaque"
        obligation["validationProofRequirement"] = {"declarationRef": "definition/dodItem/stale"}
        self.assertEqual(self.payload()["proposals"][1]["declarationRef"], "repair/convergence-opaque")

    def test_malformed_or_out_of_batch_obligations_fail_closed(self):
        original = self.authority["manifest"]["proofObligations"]
        for obligations in (None, {}, [], [original[0]], original + [original[0]],
                            original + [{"coverageWorkItemId": "cwi-foreign", "declarationRef": "foreign"}],
                            original + [None]):
            with self.subTest(obligations=obligations):
                authority = copy.deepcopy(self.authority)
                authority["manifest"]["proofObligations"] = obligations
                with self.assertRaises(MockContractError):
                    self.payload(authority)
        for field in ("coverageWorkItemId", "declarationRef"):
            for value in (None, "", " ", 7, [], {}):
                with self.subTest(field=field, value=value):
                    authority = copy.deepcopy(self.authority)
                    authority["manifest"]["proofObligations"][0][field] = value
                    with self.assertRaises(MockContractError):
                        self.payload(authority)

    def test_pascal_case_transport(self):
        obligations = self.authority["manifest"].pop("proofObligations")
        self.authority["manifest"]["ProofObligations"] = [
            {"CoverageWorkItemID": item["coverageWorkItemId"], "DeclarationRef": item["declarationRef"]}
            for item in obligations
        ]
        self.test_current_declarations_match_each_cwi_in_batch_order()

    def test_existing_candidate_wire_contract(self):
        self_test()


if __name__ == "__main__":
    unittest.main()
