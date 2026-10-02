"""共享 authoring mock 的本地合同测试，不连接服务。"""
import copy
import importlib.util
import json
from pathlib import Path
import unittest

module_spec = importlib.util.spec_from_file_location('authoring_mock', Path(__file__).with_name('verify_authoring_mock.py'))
assert module_spec is not None and module_spec.loader is not None
mock = importlib.util.module_from_spec(module_spec)
module_spec.loader.exec_module(mock)
AuthoringMockError = mock.AuthoringMockError
authoring_response = mock.authoring_response


def prompt(value, section='C3.specification-request'):
    return '### C0.specification-contract\n合同\n### ' + section + '\n' + json.dumps(value, ensure_ascii=False)


class AuthoringMockTest(unittest.TestCase):
    def candidate(self):
        return {'goal': {'CandidateItemRef': 'authoring:goal', 'Text': '目标'},
                'candidateAcceptance': [{'CandidateItemRef': 'derived-dod:user:1', 'Text': '用户约束'},
                                        {'CandidateItemRef': 'derived-dod:template:1', 'Text': '模板约束'}],
                'outputShape': {'Version': 1, 'GoalItemRef': 'authoring:goal', 'GoalTextHash': 'hash'},
                'expectedResources': [{'ID': 'resource', 'Ref': 'file', 'SnapshotHash': 'snapshot', 'Required': True, 'Available': True}]}

    def test_non_authoring_is_not_intercepted(self):
        self.assertIsNone(authoring_response('### C3.semantic-authority\n{}'))

    def test_derivation_echoes_identity_without_inventing_items(self):
        value = {'outputShape': {'AuthoringInputHash': 'input', 'TemplateSnapshotHash': 'template'}}
        self.assertEqual(authoring_response(prompt(value, 'C3.derivation-input')),
                         {**value['outputShape'], 'AdditionalItems': []})

    def test_proposal_preserves_sources_resources_and_unverified_status(self):
        value = self.candidate()
        original = copy.deepcopy(value)
        spec = authoring_response(prompt(value))['specification']
        self.assertEqual(value, original)
        self.assertEqual(spec['Resources'], value['expectedResources'])
        self.assertEqual([r['SourceKind'] for r in spec['Requirements']], ['USER_EXPLICIT', 'USER_EXPLICIT', 'TEMPLATE_REQUIRED'])
        self.assertEqual([r['Text'] for r in spec['Requirements']], ['目标', '用户约束', '模板约束'])
        self.assertTrue(all(v['Status'] == 'UNVERIFIED' for v in spec['Verifications']))
        self.assertEqual(spec['Reviews'], [])

    def test_feasibility_is_unknown_without_profile_declaration(self):
        candidate = self.candidate()
        spec = authoring_response(prompt(candidate))['specification']
        value = {**candidate, 'kind': 'verifier_feasibility', 'proposalHash': 'proposal', 'specification': spec}
        review = authoring_response(prompt(value))['review']
        self.assertEqual(review['Status'], 'UNKNOWN')
        self.assertEqual(review['SpecificationHash'], 'proposal')
        for verification in spec['Verifications']:
            verification['Status'] = 'DECLARED'
        self.assertEqual(authoring_response(prompt(value))['review']['Status'], 'UNKNOWN')
        spec['Reviews'] = [{'Status': 'PASS'}]
        with self.assertRaises(AuthoringMockError):
            authoring_response(prompt(value))

    def test_exact_profile_binding_and_forged_declaration(self):
        value = self.candidate()
        value['profiles'] = [{'ID': 'fixture', 'EvidenceRequirements': [
            {'ID': '*', 'Scope': 'dodItem', 'RequiredEvidenceKinds': ['validation'],
             'AcceptedStepKinds': ['shell/run']}]}]
        spec = authoring_response(prompt(value))['specification']
        self.assertTrue(all(v['Status'] == 'DECLARED' and v['ProfileID'] == 'fixture' for v in spec['Verifications']))
        review = {**value, 'kind': 'verifier_feasibility', 'proposalHash': 'exact', 'specification': spec}
        self.assertEqual(authoring_response(prompt(review))['review']['Status'], 'PASS')
        for field in ('ProfileID', 'EvidenceRequirementID', 'Scope', 'Status'):
            forged = copy.deepcopy(review)
            forged['specification']['Verifications'][0][field] = 'invented'
            self.assertEqual(authoring_response(prompt(forged))['review']['Status'], 'UNKNOWN')
        value['profiles'].append(copy.deepcopy(value['profiles'][0]))
        self.assertTrue(all(v['Status'] == 'UNVERIFIED' for v in authoring_response(prompt(value))['specification']['Verifications']))
        value['profiles'] = [None]
        with self.assertRaises(AuthoringMockError):
            authoring_response(prompt(value))

    def test_invalid_contract_fails_closed(self):
        for value in ({}, {**self.candidate(), 'candidateAcceptance': []},
                      {**self.candidate(), 'kind': 'invented'}):
            with self.subTest(value=value), self.assertRaises(AuthoringMockError):
                authoring_response(prompt(value))


if __name__ == '__main__':
    unittest.main()
