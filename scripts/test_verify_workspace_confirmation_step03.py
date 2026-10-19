#!/usr/bin/env python3
"""STEP03 verifier rules only; no external provider or process."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("step03", Path(__file__).with_name("verify-workspace-confirmation-step03.py"))
step03 = importlib.util.module_from_spec(spec)
spec.loader.exec_module(step03)


class Step03VerifierTest(unittest.TestCase):
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
