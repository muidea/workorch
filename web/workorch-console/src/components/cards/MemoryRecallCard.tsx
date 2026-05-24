import React from 'react';
import { Space, Empty, Tag } from 'antd';
import { CardGrid, ProTable, ProCard } from '../proCompat';
import type { MemoryRecallTrace } from '../../types';
import { compactJSON } from '../../utils';
import { StateTag, BoundedParagraph, CompactTags, MetricGrid } from '../common';
import { modelLabels } from '../../utils/constants';

interface MemoryRecallCardProps {
  items?: MemoryRecallTrace[];
}

type MemoryRecallGroup = {
  key: string;
  source: string;
  stage: string;
  status: string;
  traces: MemoryRecallTrace[];
  memoryIds: string[];
};

function memoryRecallGroups(items: MemoryRecallTrace[]) {
  const grouped = new Map<string, MemoryRecallTrace[]>();
  for (const item of items) {
    const source = item.source || 'unknown';
    const stage = item.stage || 'unknown';
    const key = `${source}/${stage}`;
    grouped.set(key, [...(grouped.get(key) ?? []), item]);
  }
  return Array.from(grouped.entries())
    .map(([key, traces]) => {
      const [source, stage] = key.split('/');
      const status = traces.some((item) => item.status === 'degraded') ? 'degraded' : traces.some((item) => item.status === 'selected') ? 'selected' : traces[0]?.status || 'unknown';
      const memoryIds = Array.from(new Set(traces.flatMap((item) => item.retrievedMemoryIds ?? [])));
      return { key, source, stage, status, traces, memoryIds };
    })
    .sort((a, b) => a.key.localeCompare(b.key));
}

function memoryRecallQueryPanel(item: MemoryRecallTrace) {
  return compactJSON({
    policy: item.policy,
    providerId: item.providerId,
    operation: item.operation,
    queryTextPreview: item.queryTextPreview,
    queryTextSha256: item.queryTextSha256,
    minScore: item.minScore,
    limit: item.limit,
    embeddingModel: item.embeddingModel,
    vectorDimension: item.vectorDimension,
    occurredAt: item.occurredAt,
    evidenceRef: item.evidenceRef
  });
}

export const MemoryRecallCard = React.memo(function MemoryRecallCard({ items }: MemoryRecallCardProps) {
  const traces = items ?? [];

  if (traces.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}未记录${modelLabels.memoryRecall}证据`} />;
  }

  const groups = memoryRecallGroups(traces);
  const memoryIds = new Set(traces.flatMap((item) => item.retrievedMemoryIds ?? [])).size;
  const degraded = traces.filter((item) => item.status === 'degraded').length;
  const selected = traces.filter((item) => item.status === 'selected').length;

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <MetricGrid
        items={[
          { key: 'traces', label: modelLabels.memoryRecallTrace, value: traces.length, color: '#1668dc' },
          { key: 'groups', label: '阶段分组', value: groups.length, color: '#08979c' },
          { key: 'selected', label: '已选中', value: selected, color: '#389e0d' },
          { key: 'degraded', label: '降级', value: degraded, color: degraded ? '#cf1322' : undefined },
          { key: 'memoryIds', label: modelLabels.memory, value: memoryIds, color: '#531dab' }
        ]}
      />
      <ProTable<MemoryRecallGroup>
        rowKey="key"
        search={false}
        options={false}
        pagination={false}
        size="small"
        columns={[
          { title: '分组', dataIndex: 'key', render: (_, item) => <Space><Tag color="blue">{item.source}</Tag><Tag>{item.stage}</Tag></Space> },
          { title: '状态', dataIndex: 'status', width: 110, render: (_, item) => <StateTag state={item.status} /> },
          { title: modelLabels.memoryRecallTrace, width: 140, render: (_, item) => item.traces.length },
          { title: `命中${modelLabels.memory}`, dataIndex: 'memoryIds', ellipsis: true, render: (_, item) => <CompactTags tags={item.memoryIds} maxVisible={5} /> }
        ]}
        expandable={{
          expandedRowRender: (group) => (
            <ProTable<MemoryRecallTrace>
              rowKey="id"
              search={false}
              options={false}
              pagination={false}
              size="small"
              columns={[
                { title: modelLabels.memoryRecallTrace, dataIndex: 'id', copyable: true, ellipsis: true },
                { title: '策略', dataIndex: 'policy', ellipsis: true },
                { title: '状态', dataIndex: 'status', width: 100, render: (_, item) => <StateTag state={item.status} /> },
                { title: `命中${modelLabels.memory}`, dataIndex: 'retrievedMemoryIds', ellipsis: true, render: (_, item) => <CompactTags tags={item.retrievedMemoryIds ?? []} maxVisible={5} /> },
                { title: '原因', dataIndex: 'reason', ellipsis: true, render: (_, item) => <BoundedParagraph text={item.reason || item.queryTextPreview || '-'} rows={2} /> }
              ]}
              dataSource={group.traces}
              expandable={{
                expandedRowRender: (item) => (
                  <CardGrid>
                    <ProCard title="召回查询" colSpan={{ xs: 24, lg: 12 }}>
                      <pre className="json-panel json-panel-large">{memoryRecallQueryPanel(item)}</pre>
                    </ProCard>
                    <ProCard title={`命中${modelLabels.memory}详情`} colSpan={{ xs: 24, lg: 12 }}>
                      <pre className="json-panel json-panel-large">{compactJSON(item.retrievedMemoryHits ?? [])}</pre>
                    </ProCard>
                  </CardGrid>
                )
              }}
            />
          )
        }}
        dataSource={groups}
      />
    </Space>
  );
});
