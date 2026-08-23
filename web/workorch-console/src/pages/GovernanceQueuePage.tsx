import { PageContainer } from '../components/proCompat';
import { Tabs } from 'antd';
import { modelLabels } from '../utils/constants';
import { DecisionQueuePage } from './DecisionQueuePage';
import { ApprovalQueuePage } from './ApprovalQueuePage';
import type { ControlPlaneProps } from '../types';

export function GovernanceQueuePage({ onOpenWorkspace }: Pick<ControlPlaneProps, 'onOpenWorkspace'>) {
  return (
    <PageContainer
      title="治理队列"
      subTitle={`统一处理人工决策与审批，确保阻塞${modelLabels.run}能按正式业务语义继续、失败或重新规划`}
    >
      <Tabs
        items={[
          {
            key: 'decisions',
            label: '决策工单',
            children: <DecisionQueuePage onOpenWorkspace={onOpenWorkspace} />
          },
          {
            key: 'approvals',
            label: '审批处理',
            children: <ApprovalQueuePage />
          }
        ]}
      />
    </PageContainer>
  );
}
