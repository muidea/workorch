import { Alert, List, Typography } from 'antd';
import type { DefinitionAuthoringView } from '../../types';

export function AuthoringRequiredEvidencePurposes({ proposal }: { proposal: NonNullable<DefinitionAuthoringView['proposal']> }) {
  const required = proposal.requiredEvidencePurposes || [];
  if (!required.length) return null;
  return <section aria-label="Profile 必需验收来源">
    <Typography.Title level={5}>Profile 必需验收来源</Typography.Title>
    <Alert type={required.some(item => !item.acceptanceItemRef) ? 'warning' : 'info'} message="每个必需历史来源须选择唯一原 DoD；缺项阻止确认。关系完整仅是声明，后续仍须独立普通验收，不代表工作或终验完成。" />
    <List dataSource={required} renderItem={item => <List.Item><div>
      <div>{item.historicalItemRef} → {item.acceptanceItemRef || '缺少原 DoD 验收来源'}</div>
      <div>{item.contractRef} · Profile {item.historicalProfileId} · {item.historicalDeliverableId}</div>
      <div>快照：{item.historicalProfileHash}</div>
      <pre style={{ whiteSpace: 'pre-wrap' }}>{item.historicalText}</pre>
      {item.acceptanceItemRef ? <pre style={{ whiteSpace: 'pre-wrap' }}>{proposal.items.find(source => source.ref === item.acceptanceItemRef)?.text}</pre> : null}
    </div></List.Item>} />
  </section>;
}
