#!/usr/bin/env python3
"""live 验证器本地合同回归，不发送 HTTP 或调用 provider。"""
import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("authoring_live", Path(__file__).with_name("verify-workspace-authoring-live.py"))
assert spec is not None and spec.loader is not None
live = importlib.util.module_from_spec(spec)
spec.loader.exec_module(live)


def ready_view():
    return {"state": "ready_to_confirm", "templateCurrent": True, "inputHash": "input", "derivationPermitId": "permit",
            "proposal": {"derivationPermitId": "permit", "proposalHash": "proposal", "specificationHash": "spec",
                         "items": [{"ref": "item"}], "reviews": [{"kind": kind, "status": "PASS", "specificationHash": "proposal"}
                         for kind in ("dimension_coverage", "negative_path", "regression", "resource_environment", "traceability", "verifier_feasibility")]}}


class AuthoringLiveContractTest(unittest.TestCase):
    def test_valid_ready_proposal(self):
        view = ready_view()
        self.assertEqual(live.require_ready_proposal(view), view["proposal"])

    def test_missing_runs_is_not_zero_runs(self):
        live.require_zero_runs({"runs": []})
        for response in ({}, {"runs": None}, {"runs": {}}, {"runs": [{"id": "unexpected"}]}):
            with self.subTest(response=response), self.assertRaises(RuntimeError):
                live.require_zero_runs(response)

    def test_fail_closed_for_incomplete_or_stale_identity(self):
        for field in ("inputHash", "derivationPermitId"):
            view = ready_view()
            del view[field]
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                live.require_ready_proposal(view)
        for field in ("proposalHash", "specificationHash", "derivationPermitId", "items"):
            view = ready_view()
            del view["proposal"][field]
            with self.subTest(field=field), self.assertRaises(RuntimeError):
                live.require_ready_proposal(view)
        view = ready_view()
        view["templateCurrent"] = False
        with self.assertRaises(RuntimeError):
            live.require_ready_proposal(view)

    def test_reviews_require_canonical_kinds_and_exact_proposal(self):
        base = ready_view()
        for field, value in (("kind", "invented"), ("status", "UNKNOWN"), ("status", "FAIL"), ("specificationHash", "stale")):
            view = copy.deepcopy(base)
            view["proposal"]["reviews"][0][field] = value
            with self.subTest(field=field, value=value), self.assertRaises(RuntimeError):
                live.require_ready_proposal(view)


if __name__ == "__main__":
    unittest.main()
