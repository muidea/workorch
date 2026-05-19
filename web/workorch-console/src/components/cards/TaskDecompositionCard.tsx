import React from 'react';
import { Space, Descriptions, Empty, Tabs } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { TaskDecomposition, TaskDecompositionNode } from '../../types';
import { stateLabel, compactJSON } from '../../utils';
import { StateTag, BoundedParagraph, CompactTags, MetricGrid } from '../common';

interface TaskDecompositionCardProps {
  items?: TaskDecomposition[];
}

export const TaskDecompositionCard = React.memo(function TaskDecompositionCard({ items }: TaskDecompositionCardProps) {
  const decompositions = items ?? [];

  if (decompositions.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无任务拆分图" />;
  }

  const latest = decompositions.at(-1);
  const total = decompositions.reduce((sum, item) => sum + item.nodeTotal, 0);
  const completed = decompositions.reduce((sum, item) => sum + item.nodeCompleted, 0);
  const failed = decompositions.reduce((sum, item) => sum + item.nodeFailed, 0);
  const running = decompositions.reduce((sum, item) => sum + item.nodeRunning, 0);

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Descriptions
        column={{ xs: 1, md: 4 }}
        size="small"
        items={[
          { key: 'graph', label: '拆分图', children: decompositions.length },
          { key: 'node', label: '节点', children: `${completed}/${total} 已完成` },
          { key: 'running', label: '执行中', children: running },
          { key: 'failed', label: '失败', children: failed },
          { key: 'latest', label: '最新根步骤', children: latest?.rootStepId ? <BoundedParagraph text={latest.rootStepId} rows={1} copyText={latest.rootStepId} /> : '-' },
          { key: 'state', label: '最新状态', children: latest ? <StateTag state={latest.state} /> : '-' },
          { key: 'branch', label: '分支', children: latest?.graphBranch || '-' },
          { key: 'summary', label: '摘要', children: <BoundedParagraph text={latest?.summary} rows={2} /> }
        ]}
      />
      <Tabs
        items={decompositions.map((item, index) => ({
          key: item.id || `${item.rootStepId}-${index}`,
          label: `拆分图 ${index + 1} / ${stateLabel(item.state)}`,
          children: (
            <Space direction="vertical" size="small" style={{ width: '100%' }}>
              <MetricGrid
                items={[
                  { key: 'nodes', label: '节点总数', value: item.nodeTotal },
                  { key: 'completed', label: '已完成', value: item.nodeCompleted, color: '#389e0d' },
                  { key: 'running', label: '执行中', value: item.nodeRunning, color: '#0958d9' },
                  { key: 'failed', label: '失败', value: item.nodeFailed, color: '#cf1322' }
                ]}
              />
              <ProTable<TaskDecompositionNode>
                rowKey={(node, nodeIndex) => `${item.id}-${node.id}-${nodeIndex}`}
                search={false}
                options={false}
                pagination={{ pageSize: 8 }}
                size="small"
                columns={[
                  { title: '节点', dataIndex: 'id', copyable: true, ellipsis: true },
                  { title: '类型', dataIndex: 'kind', width: 150 },
                  { title: '标题', dataIndex: 'title', ellipsis: true, render: (_, node) => <BoundedParagraph text={node.title} rows={2} /> },
                  { title: '状态', dataIndex: 'state', width: 110, render: (_, node) => <StateTag state={node.state} /> },
                  { title: '依赖', dataIndex: 'dependsOn', width: 180, render: (_, node) => <CompactTags tags={node.dependsOn ?? []} /> },
                  { title: '执行步骤', dataIndex: 'stepId', copyable: true, ellipsis: true }
                ]}
                expandable={{
                  expandedRowRender: (node) => (
                    <Descriptions
                      column={1}
                      size="small"
                      items={[
                        { key: 'stepState', label: '步骤状态', children: node.stepState ? stateLabel(node.stepState) : '-' },
                        { key: 'branch', label: '分支', children: node.branch || '-' },
                        { key: 'evidence', label: '证据', children: <CompactTags tags={node.evidenceRefs ?? []} maxVisible={6} /> },
                        { key: 'input', label: '输入', children: <pre className="json-panel">{compactJSON(node.input)}</pre> }
                      ]}
                    />
                  )
                }}
                dataSource={item.nodes ?? []}
              />
            </Space>
          )
        }))}
      />
    </Space>
  );
});
