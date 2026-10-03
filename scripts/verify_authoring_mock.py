"""本地 authoring mock：只回显候选身份，不伪造 Profile 或运行证据。"""
import copy
import json
import re


class AuthoringMockError(ValueError):
    pass


KINDS = {'dimension_coverage', 'negative_path', 'regression', 'resource_environment', 'traceability', 'verifier_feasibility'}


def _text(value):
    if not isinstance(value, str) or not value:
        raise AuthoringMockError('创作请求缺少非空身份')
    return value


def _binding(request):
    bindings = []
    profiles = request.get('profiles') or []
    if not isinstance(profiles, list):
        raise AuthoringMockError('Profile 声明必须为列表')
    for profile in profiles:
        if not isinstance(profile, dict):
            raise AuthoringMockError('Profile 声明必须为对象')
        requirements = profile.get('EvidenceRequirements') or []
        if not isinstance(requirements, list) or any(not isinstance(item, dict) for item in requirements):
            raise AuthoringMockError('证据声明必须为对象列表')
        for requirement in requirements:
            if requirement.get('ID') == '*' and requirement.get('Scope') == 'dodItem' and requirement.get('RequiredEvidenceKinds') and requirement.get('AcceptedStepKinds'):
                bindings.append((_text(profile['ID']), _text(requirement['ID']), 'dodItem'))
    # 多个可选声明不能由 mock 猜测选择。
    return bindings[0] if len(bindings) == 1 else None


def authoring_response(prompt):
    """非 authoring 请求返回 None；识别后的损坏请求明确拒绝。"""
    sections = dict(re.findall(r'(?ms)^###\s+([^\n]+)\n(.*?)(?=^###\s+|\Z)', prompt))
    section = next((key for key in ('C3.derivation-input', 'C3.specification-request') if key in sections), None)
    if section is None:
        return None
    try:
        request, _ = json.JSONDecoder().raw_decode(sections[section].lstrip())
        if not isinstance(request, dict):
            raise AuthoringMockError('创作输入必须为对象')
        if section == 'C3.derivation-input':
            shape = request['outputShape']
            return {'AuthoringInputHash': _text(shape['AuthoringInputHash']),
                    'TemplateSnapshotHash': _text(shape['TemplateSnapshotHash']),
                    'AdditionalItems': []}
        if 'kind' in request:
            kind = request['kind']
            if kind not in KINDS:
                raise AuthoringMockError('未知审查类别')
            spec = request['specification']
            if spec.get('Reviews'):
                raise AuthoringMockError('独立审查不得读取其他结论')
            requirements = spec.get('Requirements') or []
            if not isinstance(requirements, list) or not requirements:
                raise AuthoringMockError('审查必须包含可追溯需求')
            binding = _binding(request)
            verified = bool(binding) and bool(spec.get('Verifications')) and all(
                v.get('Status') == 'DECLARED' and (v.get('ProfileID'), v.get('EvidenceRequirementID'), v.get('Scope')) == binding
                for v in spec['Verifications'])
            status = 'UNKNOWN' if kind == 'verifier_feasibility' and not verified else 'PASS'
            return {'review': {'Kind': kind, 'Status': status,
                               'SpecificationHash': _text(request['proposalHash']),
                               'Reason': '本地合同夹具核对；不提供实际执行证据',
                               'EvidenceRefs': [_text(request['goal']['CandidateItemRef'])],
                               'RequirementIDs': [_text(r['ID']) for r in requirements]}}
        items = request['candidateAcceptance']
        if not isinstance(items, list) or not items:
            raise AuthoringMockError('候选验收项不能为空')
        goal = request['goal']
        spec = {'Version': 1, 'GoalItemRef': _text(goal['CandidateItemRef']),
                'GoalTextHash': _text(request['goalTextHash'])}
        spec.update(Dimensions=[{'ID': 'authoring', 'Required': True, 'Activated': True, 'ResourceIDs': []}],
                    Requirements=[], Obligations=[], Verifications=[], Reviews=[])
        sources = [(goal, 'USER_EXPLICIT', 'goal')]
        for index, item in enumerate(items):
            ref = _text(item['CandidateItemRef'])
            if ref.startswith('derived-dod:user:'):
                kind = 'USER_EXPLICIT'
            elif ref.startswith('derived-dod:template:'):
                kind = 'TEMPLATE_REQUIRED'
            elif ref.startswith('derived-dod:generated:'):
                kind = 'DERIVED'
            else:
                raise AuthoringMockError('未知候选来源')
            sources.append((item, kind, f'item-{index}'))
        binding = _binding(request)
        for item, kind, identity in sources:
            ref, text = _text(item['CandidateItemRef']), _text(item['Text'])
            spec['Requirements'].append({'ID': identity, 'DimensionID': 'authoring', 'Required': True,
                                         'SourceKind': kind, 'SourceItemRefs': [ref], 'ResourceIDs': [], 'Text': text})
            obligation = 'obligation-' + identity
            spec['Obligations'].append({'ID': obligation, 'RequirementID': identity,
                                        'DoDItemRefs': [items[0]['CandidateItemRef'] if identity == 'goal' else ref], 'Text': text})
            verification = {'ID': 'verify-' + identity, 'ObligationID': obligation,
                            'Declaration': '按封存 Profile 验证' if binding else '请求未提供可信 Profile 声明',
                            'Status': 'DECLARED' if binding else 'UNVERIFIED',
                            'ProfileID': '', 'EvidenceRequirementID': '', 'Scope': ''}
            if binding:
                verification.update(ProfileID=binding[0], EvidenceRequirementID=binding[1], Scope=binding[2])
            spec['Verifications'].append(verification)
        spec['Resources'] = copy.deepcopy(request['expectedResources'])
        return {'specification': spec}
    except (KeyError, TypeError, json.JSONDecodeError) as error:
        raise AuthoringMockError('创作 mock 输入合同无效') from error
