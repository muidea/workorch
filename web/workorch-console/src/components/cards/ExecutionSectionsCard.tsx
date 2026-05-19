import React from 'react';
import { Space, Empty } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { ExecutionSection } from '../../types';
import { StateTag, BoundedParagraph, CompactTags, MetricGrid } from '../common';
import { modelLabels } from '../../utils/constants';

interface ExecutionSectionsCardProps {
  items?: ExecutionSection[];
}

export const ExecutionSectionsCard = React.memo(function ExecutionSectionsCard({ items }: ExecutionSectionsCardProps) {
  const sections = items ?? [];

  if (sections.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`暂无${modelLabels.executionSection}`} />;
  }

  const completed = sections.filter((item) => item.state === 'completed').length;
  const failed = sections.filter((item) => item.state === 'failed').length;
  const running = sections.filter((item) => item.state === 'running').length;
  const memoryRecallRefs = new Set(sections.flatMap((item) => item.memoryRecallRefs ?? [])).size;
  const memoryIds = new Set(sections.flatMap((item) => item.memoryIds ?? [])).size;

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <MetricGrid
        items={[
          { key: 'total', label: '分区总数', value: sections.length, color: '#1668dc' },
          { key: 'completed', label: '已完成', value: completed, color: '#389e0d' },
          { key: 'running', label: '执行中', value: running, color: '#0958d9' },
          { key: 'failed', label: '失败', value: failed, color: '#cf1322' },
          { key: 'memoryRecallRefs', label: modelLabels.memoryRecall, value: memoryRecallRefs, color: '#08979c' },
          { key: 'memoryIds', label: `${modelLabels.memory}引用`, value: memoryIds, color: '#531dab' }
        ]}
      />
      <ProTable<ExecutionSection>
        rowKey="id"
        search={false}
        options={false}
        pagination={{ pageSize: 8 }}
        size="small"
        columns={[
          { title: '分区', dataIndex: 'title', ellipsis: true, render: (_, item) => <BoundedParagraph text={item.title || item.id} rows={2} /> },
          { title: '状态', dataIndex: 'state', width: 110, render: (_, item) => <StateTag state={item.state} /> },
          { title: '图节点', dataIndex: 'graphNodeId', copyable: true, ellipsis: true },
          { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
          { title: modelLabels.memoryRecall, dataIndex: 'memoryRecallRefs', width: 180, render: (_, item) => <CompactTags tags={item.memoryRecallRefs ?? []} maxVisible={3} /> },
          { title: modelLabels.memory, dataIndex: 'memoryIds', width: 160, render: (_, item) => <CompactTags tags={item.memoryIds ?? []} maxVisible={3} /> },
          { title: '证据', dataIndex: 'actualEvidenceRefs', width: 220, render: (_, item) => <CompactTags tags={item.actualEvidenceRefs ?? []} maxVisible={4} /> }
        ]}
        expandable={{
          expandedRowRender: (item) => (
            <div style={{ padding: '8px 0' }}>
              <div style={{ marginBottom: 8 }}>
                <strong>摘要：</strong>
                <BoundedParagraph text={item.summary} rows={2} />
              </div>
              <div style={{ marginBottom: 8 }}>
                <strong>关联验收项：</strong>
                <CompactTags tags={item.relatedDoDItems ?? []} maxVisible={6} />
              </div>
              <div style={{ marginBottom: 8 }}>
                <strong>计划引用：</strong>
                <CompactTags tags={item.plannedStepRefs ?? []} maxVisible={6} />
              </div>
            </div>
          )
        }}
        dataSource={sections}
      />
    </Space>
  );
});
