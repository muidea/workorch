import { Alert, Descriptions, List, Space, Tag, Typography } from 'antd';
import type { DefinitionAuthoringView } from '../../types';
import { authoringSourceLabel } from '../../workspaceAuthoring';

export function AuthoringCandidateProposal({ proposal }: { proposal: NonNullable<DefinitionAuthoringView['proposal']> }) {
  return <Space direction="vertical" style={{ width: '100%' }}>
    <Alert type="info" message="候选按 all_of 逐项合取；DECLARED 仅为验证能力声明，不是运行验证已通过。六审 PASS 也不等于用户确认。" />
    <Descriptions column={1} items={[
      { key: 'version', label: '候选版本', children: proposal.version },
      { key: 'goal', label: 'Goal 来源', children: `${proposal.goalItemRef || '-'} · ${proposal.goalTextHash || '-'}` },
      { key: 'proposal', label: 'proposalHash', children: proposal.proposalHash || '尚未生成' },
      { key: 'spec', label: 'specificationHash', children: proposal.specificationHash || '尚未具备完整已审身份' },
      { key: 'permit', label: '派生 permit', children: proposal.derivationPermitId },
    ]} />
    <Typography.Title level={5}>候选 Goal 原文（required）</Typography.Title>
    <pre style={{ whiteSpace: 'pre-wrap' }}>{proposal.goalText}</pre>
    <Typography.Title level={5}>源承诺清单</Typography.Title>
    <List dataSource={proposal.items} renderItem={item => <List.Item><div><Tag>{authoringSourceLabel(item.sourceKind)}</Tag>{item.ref}<pre style={{ whiteSpace: 'pre-wrap' }}>{item.text}</pre></div></List.Item>} />
    <Typography.Title level={5}>维度 → 要求 → 义务（all_of）</Typography.Title>
    <List aria-label="完整维度" dataSource={proposal.dimensions || []} renderItem={item => <List.Item>{item.id} · {item.required ? 'required' : 'optional'} · {item.activated ? 'activated' : 'inactive'} · resources: {item.resourceIds?.join('、') || '无'}</List.Item>} />
    <List aria-label="完整要求" dataSource={proposal.requirements || []} renderItem={item => <List.Item><div>{item.id} → dimension {item.dimensionId} · {item.required ? 'required' : 'optional'} · {authoringSourceLabel(item.sourceKind)}<div>来源：{item.sourceItemRefs?.join('、')}；资源：{item.resourceIds?.join('、') || '无'}</div><pre style={{ whiteSpace: 'pre-wrap' }}>{item.text}</pre></div></List.Item>} />
    <List aria-label="完整义务" dataSource={proposal.obligations || []} renderItem={item => <List.Item><div>{item.id} → requirement {item.requirementId}<div>DoD 来源：{item.dodItemRefs?.join('、')}</div><pre style={{ whiteSpace: 'pre-wrap' }}>{item.text}</pre></div></List.Item>} />
    <Typography.Title level={5}>资源快照（不包含文件正文）</Typography.Title>
    <List dataSource={proposal.resources || []} renderItem={item => <List.Item>{item.id} · {item.ref} · {item.snapshotHash} · {item.required ? 'required' : 'optional'} · {item.available ? 'available' : 'unavailable'}</List.Item>} />
    <Typography.Title level={5}>逐义务验证声明</Typography.Title>
    <List aria-label="逐义务验证声明" dataSource={proposal.verifications || []} renderItem={item => <List.Item><div>{item.id} → obligation {item.obligationId} · {item.status}<pre style={{ whiteSpace: 'pre-wrap' }}>{item.declaration}</pre><div>Profile：{item.profileId || '未绑定'}；evidenceRequirement：{item.evidenceRequirementId || '未绑定'}；scope：{item.scope || '未绑定'}</div></div></List.Item>} />
    <Typography.Title level={5}>六类独立审查（{proposal.reviews?.length || 0}/6）</Typography.Title>
    <List aria-label="独立审查明细" dataSource={proposal.reviews || []} renderItem={item => <List.Item><div><Tag color={item.status === 'PASS' ? 'green' : 'orange'}>{item.status}</Tag>{item.kind}<pre style={{ whiteSpace: 'pre-wrap' }}>{item.reason}</pre><div>required 覆盖：{item.requirementIds?.join('、') || '未提供'}</div><div>依据引用：{item.evidenceRefs?.join('、') || '未提供'}</div><div>审查绑定 proposalHash（字段 specificationHash）：{item.specificationHash}</div></div></List.Item>} />
    {(proposal.executionProofMappings || []).map((item, index) => <div key={`execution-${index}`}>{item.consumerItemRef} · {item.ruleRef} → {item.sourceItemRef} · {item.contractRef}<pre style={{ whiteSpace: 'pre-wrap' }}>{item.consumerGoalText}</pre></div>)}
    {(proposal.evidencePurposeMappings || []).map((item, index) => <div key={`purpose-${index}`}>{item.historicalItemRef} → {item.acceptanceItemRef} · {item.contractRef}<div>{item.historicalProfileId} · {item.historicalDeliverableId} · {item.historicalProfileHash}</div><pre style={{ whiteSpace: 'pre-wrap' }}>{item.historicalText}</pre></div>)}
  </Space>;
}
