"""共享 authoring mock 的本地合同测试，不连接服务。"""
import copy
import hashlib
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
        return {'goal': {'CandidateItemRef': 'authoring-goal', 'Text': '目标'},
                'candidateAcceptance': [{'CandidateItemRef': 'derived-dod:user:1', 'Text': '用户约束'},
                                        {'CandidateItemRef': 'derived-dod:template:1', 'Text': '模板约束'}],
                'inputHash': 'input', 'templateHash': 'template', 'resourceSnapshotHash': 'resources',
                'expectedResources': [{'ID': 'resource', 'Ref': 'file', 'SnapshotHash': 'snapshot', 'Required': True, 'Available': True}]}

    def profile(self):
        return {'ID': 'fixture', 'EvidenceRequirements': [
            {'ID': '*', 'Scope': 'dodItem', 'RequiredEvidenceKinds': ['validation'],
             'AcceptedStepKinds': ['shell/run']}]}

    def canonical_review_fixture(self, value):
        # 仅构造 critic 输入，不能绕过 Go owner compiler 或声称正式准入。
        mapping = authoring_response(prompt(value))['mapping']
        goal = value['goal']
        spec = {'Version': 1, 'GoalItemRef': goal['CandidateItemRef'],
                'GoalTextHash': 'definition-text-sha256:' + hashlib.sha256(goal['Text'].encode()).hexdigest(),
                'Dimensions': [{'ID': 'dimension', 'Required': True, 'Activated': True, 'ResourceIDs': []}],
                'Requirements': [{'ID': 'goal', 'DimensionID': 'dimension', 'Required': True,
                                  'SourceKind': 'USER_EXPLICIT', 'SourceItemRefs': [goal['CandidateItemRef']],
                                  'ResourceIDs': [], 'Text': goal['Text']}],
                'Obligations': [], 'Verifications': [], 'Resources': copy.deepcopy(value['expectedResources']), 'Reviews': []}
        for index, item in enumerate(value['candidateAcceptance']):
            requirement, obligation = f'canonical-{index}', f'obligation-{index}'
            spec['Requirements'].append({'ID': requirement, 'DimensionID': 'dimension', 'Required': True,
                                         'SourceKind': mock._source_kind(item['CandidateItemRef']),
                                         'SourceItemRefs': [item['CandidateItemRef']], 'ResourceIDs': [], 'Text': item['Text']})
            spec['Obligations'].append({'ID': obligation, 'RequirementID': requirement,
                                        'DoDItemRefs': [item['CandidateItemRef']], 'Text': item['Text']})
            spec['Verifications'].append({'ID': f'verification-{index}', 'ObligationID': obligation,
                                          **mapping['Obligations'][index]['Verification']})
        return {**value, 'kind': 'verifier_feasibility', 'proposalHash': 'exact', 'specification': spec}

    def test_non_authoring_is_not_intercepted(self):
        self.assertIsNone(authoring_response('### C3.semantic-authority\n{}'))

    def test_derivation_echoes_identity_without_inventing_items(self):
        value = {'outputShape': {'AuthoringInputHash': 'input', 'TemplateSnapshotHash': 'template'}}
        self.assertEqual(authoring_response(prompt(value, 'C3.derivation-input')),
                         {**value['outputShape'], 'AdditionalItems': []})

    def test_proposal_only_maps_sealed_sources(self):
        value = self.candidate()
        original = copy.deepcopy(value)
        response = authoring_response(prompt(value))
        self.assertEqual(value, original)
        self.assertEqual(set(response), {'mapping'})
        mapping = response['mapping']
        self.assertEqual(mapping['Protocol'], 'workorch.authoring.sealed-ref-mapping/v1')
        self.assertEqual((mapping['InputHash'], mapping['TemplateSnapshotHash'], mapping['ResourceSnapshotHash']),
                         ('input', 'template', 'resources'))
        refs = [item['CandidateItemRef'] for item in value['candidateAcceptance']]
        self.assertEqual([r['SourceItemRefs'] for r in mapping['Requirements']], [[ref] for ref in refs])
        self.assertEqual([o['CandidateItemRef'] for o in mapping['Obligations']], refs)
        self.assertEqual(mapping['GoalRequirement'], {'DimensionRef': 'all', 'ResourceIDs': []})
        self.assertNotIn(value['goal']['CandidateItemRef'], refs)
        for row in mapping['Obligations']:
            self.assertEqual(set(row), {'CandidateItemRef', 'RequirementRef', 'Verification'})
            verification = row['Verification']
            self.assertEqual(verification['Status'], 'UNVERIFIED')
            self.assertTrue(verification['ProfileID'] == verification['EvidenceRequirementID'] == verification['Scope'] == '')
        for field in ('Resources', 'Reviews', 'GoalTextHash', 'Version', 'SourceKind', 'Required'):
            self.assertNotIn(field, mapping)

    def test_feasibility_is_unknown_without_profile_declaration(self):
        review = self.canonical_review_fixture(self.candidate())
        self.assertEqual(authoring_response(prompt(review))['review']['Status'], 'UNKNOWN')
        for verification in review['specification']['Verifications']:
            verification['Status'] = 'DECLARED'
        self.assertEqual(authoring_response(prompt(review))['review']['Status'], 'UNKNOWN')
        review['specification']['Reviews'] = [{'Status': 'PASS'}]
        with self.assertRaises(AuthoringMockError):
            authoring_response(prompt(review))

    def test_exact_profile_binding_and_forged_declaration(self):
        value = self.candidate()
        value['profiles'] = [self.profile()]
        mapping = authoring_response(prompt(value))['mapping']
        self.assertTrue(all(o['Verification']['Status'] == 'DECLARED' and o['Verification']['ProfileID'] == 'fixture'
                            for o in mapping['Obligations']))
        review = self.canonical_review_fixture(value)
        result = authoring_response(prompt(review))['review']
        self.assertEqual(result['Status'], 'PASS')
        self.assertEqual(result['SpecificationHash'], 'exact')
        self.assertEqual(result['RequirementIDs'], [r['ID'] for r in review['specification']['Requirements']])
        for field in ('ProfileID', 'EvidenceRequirementID', 'Scope', 'Status'):
            forged = copy.deepcopy(review)
            forged['specification']['Verifications'][0][field] = 'invented'
            self.assertEqual(authoring_response(prompt(forged))['review']['Status'], 'UNKNOWN')
        value['profiles'].append(copy.deepcopy(value['profiles'][0]))
        self.assertTrue(all(o['Verification']['Status'] == 'UNVERIFIED'
                            for o in authoring_response(prompt(value))['mapping']['Obligations']))
        value['profiles'] = [None]
        with self.assertRaises(AuthoringMockError):
            authoring_response(prompt(value))

    def test_full_37_candidates_and_six_independent_reviews(self):
        value = self.candidate()
        value['candidateAcceptance'] = [
            {'CandidateItemRef': f'derived-dod:{kind}:{index:06d}', 'Text': f'{kind}完整原文\n条目{index}'}
            for kind, count in (('user', 5), ('template', 13), ('generated', 19))
            for index in range(1, count + 1)]
        value['profiles'] = [self.profile()]
        mapping = authoring_response(prompt(value))['mapping']
        self.assertEqual(len(mapping['Obligations']), 37)
        review = self.canonical_review_fixture(value)
        original = copy.deepcopy(review['specification'])
        for kind in mock.KINDS:
            review['kind'] = kind
            self.assertEqual(authoring_response(prompt(review))['review']['Status'], 'PASS')
        self.assertEqual(review['specification'], original)

    def test_six_reviews_reject_omission_changed_text_and_wrong_source(self):
        value = self.candidate()
        value['profiles'] = [self.profile()]
        review = self.canonical_review_fixture(value)
        for kind in mock.KINDS:
            for defect in ('missing', 'changed', 'source', 'goal', 'hash', 'duplicate'):
                forged = copy.deepcopy(review)
                forged['kind'] = kind
                spec = forged['specification']
                if defect == 'missing':
                    spec['Obligations'].pop()
                elif defect == 'changed':
                    spec['Obligations'][0]['Text'] = '弱化后的文字'
                elif defect == 'source':
                    spec['Requirements'][1]['SourceKind'] = 'DERIVED'
                elif defect == 'goal':
                    spec['Requirements'][0]['Text'] = '改变目标'
                elif defect == 'hash':
                    spec['GoalTextHash'] = 'wrong'
                else:
                    spec['Requirements'].append(copy.deepcopy(spec['Requirements'][1]))
                with self.subTest(kind=kind, defect=defect):
                    self.assertEqual(authoring_response(prompt(forged))['review']['Status'], 'FAIL')

    def test_six_reviews_reject_invalid_canonical_node_identities(self):
        value = self.candidate()
        value['profiles'] = [self.profile()]
        review = self.canonical_review_fixture(value)
        for kind in mock.KINDS:
            for defect in ('duplicate_dimension', 'missing_dimension_id', 'missing_obligation_id',
                           'duplicate_verification_id', 'missing_verification_id'):
                forged = copy.deepcopy(review)
                forged['kind'] = kind
                spec = forged['specification']
                if defect == 'duplicate_dimension':
                    spec['Dimensions'].append(copy.deepcopy(spec['Dimensions'][0]))
                elif defect == 'missing_dimension_id':
                    spec['Dimensions'][0].pop('ID')
                elif defect == 'missing_obligation_id':
                    spec['Obligations'][0].pop('ID')
                    spec['Verifications'][0].pop('ObligationID')
                elif defect == 'duplicate_verification_id':
                    spec['Verifications'][1]['ID'] = spec['Verifications'][0]['ID']
                else:
                    spec['Verifications'][0].pop('ID')
                with self.subTest(kind=kind, defect=defect):
                    self.assertEqual(authoring_response(prompt(forged))['review']['Status'], 'FAIL')

    def test_resource_review_rejects_changed_sealed_facts(self):
        value = self.candidate()
        value['profiles'] = [self.profile()]
        review = self.canonical_review_fixture(value)
        review['kind'] = 'resource_environment'
        self.assertEqual(authoring_response(prompt(review))['review']['Status'], 'PASS')
        for field, changed in (('Ref', 'other'), ('SnapshotHash', 'other'), ('Required', False), ('Available', False)):
            forged = copy.deepcopy(review)
            forged['specification']['Resources'][0][field] = changed
            with self.subTest(field=field):
                self.assertEqual(authoring_response(prompt(forged))['review']['Status'], 'FAIL')
        review.pop('expectedResources')
        review['resources'] = [{'Ref': 'file', 'ContentHash': 'snapshot'}]
        review['specification']['Resources'][0]['ID'] = 'authoring-resource:000001'
        self.assertEqual(authoring_response(prompt(review))['review']['Status'], 'PASS')
        review['specification']['Resources'][0]['SnapshotHash'] = 'changed'
        self.assertEqual(authoring_response(prompt(review))['review']['Status'], 'FAIL')

    def test_review_requires_sealed_resources_and_nonempty_declaration(self):
        value = self.candidate()
        value['profiles'] = [self.profile()]
        review = self.canonical_review_fixture(value)
        for declaration in ('', '   ', None):
            forged = copy.deepcopy(review)
            forged['specification']['Verifications'][0]['Declaration'] = declaration
            with self.subTest(declaration=declaration):
                self.assertEqual(authoring_response(prompt(forged))['review']['Status'], 'UNKNOWN')
        review.pop('expectedResources')
        with self.assertRaises(AuthoringMockError):
            authoring_response(prompt(review))

    def test_review_accepts_grouping_without_replacing_obligation_text(self):
        value = self.candidate()
        value['profiles'] = [self.profile()]
        value['candidateAcceptance'].append({'CandidateItemRef': 'derived-dod:user:2', 'Text': '另一条独立原文'})
        review = self.canonical_review_fixture(value)
        requirements = review['specification']['Requirements']
        grouped = requirements[1]
        grouped['SourceItemRefs'].append('derived-dod:user:2')
        grouped['Text'] = '同来源的分组说明，不是任何一条义务原文'
        requirements.pop()
        review['specification']['Obligations'][-1]['RequirementID'] = grouped['ID']
        for kind in mock.KINDS:
            review['kind'] = kind
            with self.subTest(kind=kind):
                self.assertEqual(authoring_response(prompt(review))['review']['Status'], 'PASS')

    def test_invalid_contract_fails_closed(self):
        candidate = self.candidate()
        duplicate = copy.deepcopy(candidate)
        duplicate['candidateAcceptance'].append(copy.deepcopy(duplicate['candidateAcceptance'][0]))
        for value in ({}, {**candidate, 'candidateAcceptance': []}, {**candidate, 'kind': 'invented'},
                      {**candidate, 'candidateAcceptance': [None]}, duplicate):
            with self.subTest(value=value), self.assertRaises(AuthoringMockError):
                authoring_response(prompt(value))


if __name__ == '__main__':
    unittest.main()
