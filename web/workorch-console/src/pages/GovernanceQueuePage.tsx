import { PageContainer } from '@ant-design/pro-components';
import { Tabs } from 'antd';
import { modelLabels } from '../utils/constants';
import { DecisionQueuePage } from './DecisionQueuePage';
import { ApprovalQueuePage } from './ApprovalQueuePage';

export function GovernanceQueuePage() {
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
            children: <DecisionQueuePage />
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
