#!/usr/bin/env python3
"""STEP03 verifier rules only; no external provider or process."""
import importlib.util
import copy
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("step03", Path(__file__).with_name("verify-workspace-confirmation-step03.py"))
step03 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(step03)


class Step03VerifierTest(unittest.TestCase):
    def dod_fixture(self):
        items = [{"ref": "user:1", "text": "原文\r\n不截断"}, {"ref": "template:1", "text": "模板原准则"}]
        view = {"workspaceId": "ws", "proposal": {"items": items}}
        journal = {"DoD": {"ID": "dod", "WorkspaceID": "ws", "DefinitionRevision": "revision", "Items": [item["text"] for item in items]}}
        candidate = {"DoDInputs": [{"CandidateItemRef": item["ref"], "Text": item["text"]} for item in reversed(items)]}
        dod = {"id": "dod", "workspaceId": "ws", "definitionRevision": "revision", "items": list(journal["DoD"]["Items"])}
        return view, journal, candidate, dod

    def test_materialized_dod_preserves_exact_order_and_candidate_source_mapping(self):
        step03.validate_materialized_dod(*self.dod_fixture())

    def test_materialized_dod_rejects_content_order_scope_and_source_drift(self):
        mutations = {
            "formal text": lambda v, j, c, d: d["items"].__setitem__(0, "改写"),
            "formal order": lambda v, j, c, d: d["items"].reverse(),
            "journal text": lambda v, j, c, d: j["DoD"]["Items"].__setitem__(0, "改写"),
            "reviewed text": lambda v, j, c, d: v["proposal"]["items"][0].update(text="改写"),
            "missing item": lambda v, j, c, d: d["items"].pop(),
            "extra item": lambda v, j, c, d: d["items"].append("多余"),
            "formal scope": lambda v, j, c, d: d.update(workspaceId="other"),
            "journal scope": lambda v, j, c, d: j["DoD"].update(WorkspaceID="other"),
            "formal id": lambda v, j, c, d: d.update(id="other"),
            "formal revision": lambda v, j, c, d: d.update(definitionRevision="other"),
            "candidate text": lambda v, j, c, d: c["DoDInputs"][0].update(Text="改写"),
            "candidate ref": lambda v, j, c, d: c["DoDInputs"][0].update(CandidateItemRef="other"),
            "candidate duplicate": lambda v, j, c, d: c["DoDInputs"].append(copy.deepcopy(c["DoDInputs"][0])),
            "reviewed duplicate ref": lambda v, j, c, d: v["proposal"]["items"][0].update(ref="template:1"),
        }
        for name, mutate in mutations.items():
            with self.subTest(name=name), self.assertRaises(RuntimeError):
                data = self.dod_fixture()
                mutate(*data)
                step03.validate_materialized_dod(*data)

    def activation_fixture(self):
        view = {"workspaceId": "ws", "startupInputHash": "startup"}
        original = {"specificationHash": "specification-not-proposal"}
        candidate = {"WorkspaceID": "ws", "CandidateID": "candidate", "CandidateRevision": 2, "CandidateHash": "candidate-hash"}
        startup = {"WorkspaceID": "ws", "SpecificationHash": original["specificationHash"], "StartupInputHash": "startup", "DefinitionRevision": "definition"}
        activation = {**candidate, "State": "eligible", "StartupInputHash": "startup", "DefinitionRevision": "definition"}
        return view, original, candidate, startup, activation

    def test_sealed_activation_binds_original_specification_and_selected_candidate(self):
        step03.validate_sealed_activation(*self.activation_fixture())

    def test_sealed_activation_rejects_candidate_scope_specification_and_seal_drift(self):
        for position, field, value in [
            (2, "WorkspaceID", "other"), (3, "WorkspaceID", "other"), (4, "WorkspaceID", "other"),
            (4, "CandidateID", "other"), (4, "CandidateRevision", 3), (4, "CandidateHash", "other"),
            (4, "State", "pending_clarification_resolution"), (3, "SpecificationHash", "proposal"),
            (3, "StartupInputHash", "other"), (4, "StartupInputHash", "other"), (4, "DefinitionRevision", "other"),
        ]:
            with self.subTest(position=position, field=field), self.assertRaises(RuntimeError):
                data = self.activation_fixture()
                data[position][field] = value
                step03.validate_sealed_activation(*data)

    def test_blocked_template_is_standalone_not_a_conflicting_parent_override(self):
        template = step03.BLOCKED_TEMPLATE
        self.assertNotIn("parentTemplateIds", template)
        profile = template["contextHints"]["profileContracts"]["software-dev"]
        self.assertTrue(profile["deliverableExpectations"][0]["required"])
        self.assertEqual(profile["evidenceRequirements"][0]["id"], "*")

    def test_confirmation_uses_exactly_five_fields_not_candidate_body(self):
        view = {"workspaceId": "scope-in-url", "workspaceRevision": 8, "inputHash": "input", "derivationPermitId": "derive", "proposal": {"proposalHash": "proposal", "specificationHash": "specification", "goalText": "not submitted"}}
        self.assertEqual(step03.confirmation(view), {"inputHash": "input", "derivationPermitId": "derive", "proposalHash": "proposal", "specificationHash": "specification", "expectedWorkspaceRevision": 8})
        self.assertNotIn("confirmationRecovery", view)

    def test_incomplete_candidate_is_not_repaired_or_guessed(self):
        for view in ({}, {"inputHash": "input"}, {"inputHash": "input", "derivationPermitId": "derive", "proposal": {"proposalHash": "proposal"}}):
            with self.subTest(view=view), self.assertRaises(KeyError):
                step03.confirmation(view)


if __name__ == "__main__":
    unittest.main()
