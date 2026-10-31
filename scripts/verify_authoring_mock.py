"""本地 authoring mock：只回显候选身份，不伪造 Profile 或运行证据。"""
import hashlib
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


def _source_kind(ref):
    for prefix, kind in (('derived-dod:user:', 'USER_EXPLICIT'),
                         ('derived-dod:template:', 'TEMPLATE_REQUIRED'),
                         ('derived-dod:generated:', 'DERIVED')):
        if ref.startswith(prefix):
            return kind
    raise AuthoringMockError('未知候选来源')


def _candidates(request):
    items = request['candidateAcceptance']
    if not isinstance(items, list) or not items or any(not isinstance(item, dict) for item in items):
        raise AuthoringMockError('候选验收项必须为非空对象列表')
    seen = set()
    for item in items:
        ref = _text(item['CandidateItemRef'])
        _text(item['Text'])
        _source_kind(ref)
        if ref in seen:
            raise AuthoringMockError('候选来源重复')
        seen.add(ref)
    return items


def _rows(spec, key):
    values = spec.get(key) or []
    if not isinstance(values, list) or any(not isinstance(value, dict) for value in values):
        raise AuthoringMockError('规格字段必须为对象列表')
    return values


def _node_ids(values):
    ids = [value.get('ID') for value in values]
    valid = all(isinstance(value, str) and bool(value) for value in ids)
    return ids, valid and len(set(ids)) == len(ids)


def _review(request):
    kind = request['kind']
    if kind not in KINDS:
        raise AuthoringMockError('未知审查类别')
    spec = request['specification']
    if not isinstance(spec, dict):
        raise AuthoringMockError('审查规格必须为对象')
    if spec.get('Reviews'):
        raise AuthoringMockError('独立审查不得读取其他结论')
    items = _candidates(request)
    goal = request['goal']
    goal_ref, goal_text = _text(goal['CandidateItemRef']), _text(goal['Text'])
    requirements = _rows(spec, 'Requirements')
    obligations = _rows(spec, 'Obligations')
    verifications = _rows(spec, 'Verifications')
    dimensions = _rows(spec, 'Dimensions')
    resources = _rows(spec, 'Resources')
    if not requirements:
        raise AuthoringMockError('审查必须包含可追溯需求')
    requirement_ids, requirements_valid = _node_ids(requirements)
    dimension_values, dimensions_valid = _node_ids(dimensions)
    _, obligations_valid = _node_ids(obligations)
    _, verifications_valid = _node_ids(verifications)
    resource_values, resources_valid = _node_ids(resources)
    # 身份损坏先返回失败，不能让 None/重复 ID 获得正面审查结论。
    if not all((requirements_valid, dimensions_valid, obligations_valid, verifications_valid, resources_valid)):
        return {'review': {'Kind': kind, 'Status': 'FAIL',
                           'SpecificationHash': _text(request['proposalHash']),
                           'Reason': '规格节点身份缺失或重复；不提供实际执行证据或模型语义质量证明',
                           'EvidenceRefs': [goal_ref], 'RequirementIDs': []}}
    dimension_ids = set(dimension_values)
    source_requirements = {}
    valid = (bool(dimensions)
             and all(value.get('Required') is True and value.get('Activated') is True for value in dimensions)
             and spec.get('GoalItemRef') == goal_ref
             and spec.get('GoalTextHash') == 'definition-text-sha256:' + hashlib.sha256(goal_text.strip().encode('utf-8')).hexdigest())
    for requirement in requirements:
        refs = requirement.get('SourceItemRefs')
        if not isinstance(refs, list) or not refs or any(not isinstance(ref, str) for ref in refs):
            valid = False
            continue
        valid = valid and requirement.get('Required') is True and requirement.get('DimensionID') in dimension_ids
        for ref in refs:
            source_requirements.setdefault(ref, []).append(requirement)
    expected = {goal_ref, *(item['CandidateItemRef'] for item in items)}
    valid = valid and set(source_requirements) == expected and all(len(values) == 1 for values in source_requirements.values())
    goal_requirement = source_requirements.get(goal_ref, [{}])[0]
    valid = valid and (goal_requirement.get('Text') == goal_text
                       and goal_requirement.get('SourceKind') == 'USER_EXPLICIT'
                       and goal_requirement.get('SourceItemRefs') == [goal_ref])
    candidate_refs = {item['CandidateItemRef'] for item in items}
    valid = valid and len(obligations) == len(items)
    valid = valid and all(value.get('DoDItemRefs') and all(ref in candidate_refs for ref in value['DoDItemRefs']) for value in obligations)
    for item in items:
        ref = item['CandidateItemRef']
        requirement = source_requirements.get(ref, [{}])[0]
        matches = [value for value in obligations if ref in (value.get('DoDItemRefs') or [])]
        valid = valid and (requirement.get('SourceKind') == _source_kind(ref) and len(matches) == 1
                           and matches[0].get('RequirementID') == requirement.get('ID')
                           and matches[0].get('Text') == item['Text'] and matches[0].get('DoDItemRefs') == [ref])
    valid = valid and len(verifications) == len(obligations) and all(
        len([value for value in verifications if value.get('ObligationID') == obligation.get('ID')]) == 1
        for obligation in obligations)
    resource_ids = set(resource_values)
    valid = valid and all(
        isinstance(value.get('ResourceIDs'), list)
        and all(ref in resource_ids for ref in value['ResourceIDs'])
        for value in dimensions + requirements)
    if 'expectedResources' in request:
        valid = valid and resources == request['expectedResources']
    elif 'resources' in request:
        snapshots = request['resources']
        if not isinstance(snapshots, list) or any(not isinstance(value, dict) for value in snapshots):
            raise AuthoringMockError('封存资源必须为对象列表')
        expected_resources = [{'ID': f'authoring-resource:{index + 1:06d}', 'Ref': value.get('Ref'),
                               'SnapshotHash': value.get('ContentHash'), 'Required': True, 'Available': True}
                              for index, value in enumerate(snapshots)]
        valid = valid and resources == expected_resources
    else:
        raise AuthoringMockError('审查缺少封存资源事实')
    binding = _binding(request)
    verified = bool(binding) and bool(verifications) and all(
        isinstance(value.get('Declaration'), str) and bool(value['Declaration'].strip())
        and value.get('Status') == 'DECLARED'
        and (value.get('ProfileID'), value.get('EvidenceRequirementID'), value.get('Scope')) == binding
        for value in verifications)
    status = 'PASS' if valid else 'FAIL'
    if valid and kind == 'verifier_feasibility' and not verified:
        status = 'UNKNOWN'
    if kind == 'resource_environment' and any(value.get('Required') and not value.get('Available') for value in resources):
        status = 'FAIL'
    return {'review': {'Kind': kind, 'Status': status,
                       'SpecificationHash': _text(request['proposalHash']),
                       'Reason': '本地来源与合同夹具核对；不提供实际执行证据或模型语义质量证明',
                       'EvidenceRefs': [goal_ref], 'RequirementIDs': requirement_ids}}


def responses_prompt_text(value):
    if isinstance(value, str):
        return value
    if not isinstance(value, list) or not value:
        raise AuthoringMockError('Responses input 必须为文本或消息列表')
    parts = []
    for message in value:
        if not isinstance(message, dict) or message.get('role') not in ('developer', 'user', 'assistant'):
            raise AuthoringMockError('Responses 消息角色无效')
        content = message.get('content')
        if not isinstance(content, list) or not content:
            raise AuthoringMockError('Responses 消息正文无效')
        for block in content:
            if not isinstance(block, dict) or block.get('type') != 'input_text' or not isinstance(block.get('text'), str):
                raise AuthoringMockError('Responses 消息必须保留文本')
            parts.append(block['text'])
    return '\n\n'.join(parts)


def authoring_prompt_payloads(prompt):
    sections = dict(re.findall(r'(?ms)^###\s+([^\n]+)\n(.*?)(?=^###\s+|\Z)', responses_prompt_text(prompt)))
    keys = [key for key in ('C1.specification-snapshot', 'C2.specification-profiles', 'C3.derivation-input', 'C3.specification-request') if key in sections]
    if not keys:
        return None, None
    request = {}
    for key in keys:
        value, _ = json.JSONDecoder().raw_decode(sections[key].lstrip())
        if not isinstance(value, dict) or request.keys() & value.keys():
            raise AuthoringMockError('创作输入必须为无重复字段对象')
        request.update(value)
    return request, 'C3.derivation-input' if 'C3.derivation-input' in keys else 'C3.specification-request'


def authoring_response(prompt):
    """非 authoring 请求返回 None；识别后的损坏请求明确拒绝。"""
    try:
        request, section = authoring_prompt_payloads(prompt)
        if section is None:
            return None
        if section == 'C3.derivation-input':
            if request['outputShape'] != {'AdditionalItems': []}:
                raise AuthoringMockError('派生请求必须使用当前语义输出合同')
            return {'AdditionalItems': []}
        if 'kind' in request:
            review = _review(request)
            review['review'].pop('Kind')
            review['review'].pop('SpecificationHash')
            return review
        items = _candidates(request)
        _text(request['goal']['CandidateItemRef'])
        _text(request['goal']['Text'])
        if not isinstance(request['expectedResources'], list):
            raise AuthoringMockError('封存资源必须为列表')
        binding = _binding(request)
        mapping = {'Protocol': 'workorch.authoring.sealed-ref-mapping/v1',
                   'InputHash': _text(request['inputHash']),
                   'TemplateSnapshotHash': _text(request['templateHash']),
                   'ResourceSnapshotHash': _text(request['resourceSnapshotHash']),
                   'GoalRequirement': {'DimensionRef': 'all', 'ResourceIDs': []},
                   'Dimensions': [{'Ref': 'all', 'ResourceIDs': []}],
                   'Requirements': [], 'Obligations': []}
        for index, item in enumerate(items):
            ref = item['CandidateItemRef']
            requirement = f'item-{index}'
            mapping['Requirements'].append({'Ref': requirement, 'DimensionRef': 'all',
                                            'SourceItemRefs': [ref], 'ResourceIDs': [],
                                            'Text': '封存候选分组：' + ref})
            verification = {'Declaration': '按封存 Profile 验证' if binding else '请求未提供可信 Profile 声明',
                            'Status': 'DECLARED' if binding else 'UNVERIFIED',
                            'ProfileID': '', 'EvidenceRequirementID': '', 'Scope': ''}
            if binding:
                verification.update(ProfileID=binding[0], EvidenceRequirementID=binding[1], Scope=binding[2])
            mapping['Obligations'].append({'CandidateItemRef': ref, 'RequirementRef': requirement,
                                          'Verification': verification})
        for field in ('Protocol', 'InputHash', 'TemplateSnapshotHash', 'ResourceSnapshotHash'):
            del mapping[field]
        return {'mapping': mapping}
    except (KeyError, TypeError, json.JSONDecodeError) as error:
        raise AuthoringMockError('创作 mock 输入合同无效') from error
