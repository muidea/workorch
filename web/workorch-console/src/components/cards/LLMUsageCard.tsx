import React from 'react';
import { Space, Empty, Tag, Descriptions, Tabs } from 'antd';
import { ProTable } from '../proCompat';
import type { LLMUsageSummary, LLMUsageTrace, LLMUsageProviderModelBreakdown } from '../../types';
import { numberText, costText } from '../../utils';
import { StateTag, BoundedParagraph, CompactTags, MetricGrid } from '../common';
import { modelLabels } from '../../utils/constants';

interface LLMUsageCardProps {
  summary?: LLMUsageSummary;
  traces?: LLMUsageTrace[];
}

function llmUsageCostSummary(summary?: LLMUsageSummary) {
  if (!summary || summary.entryTotal === 0) {
    return '-';
  }
  if (summary.currency && summary.estimatedCost) {
    return costText(summary.estimatedCost, summary.currency);
  }
  const breakdown = summary.costBreakdown ?? [];
  if (breakdown.length === 0) {
    return '-';
  }
  return breakdown.map((item) => costText(item.estimatedCost, item.currency)).join(' / ');
}

function llmUsageProviderModelKey(item: LLMUsageProviderModelBreakdown, index?: number) {
  return [item.providerId, item.provider, item.model, item.currency, index].filter(Boolean).join('|');
}

export const LLMUsageCard = React.memo(function LLMUsageCard({ summary, traces }: LLMUsageCardProps) {
  const usageSummary = summary ?? {
    entryTotal: 0,
    inputTokens: 0,
    outputTokens: 0,
    totalTokens: 0,
    estimatedEntries: 0,
    providerNativeEntries: 0,
    costEstimatedEntries: 0,
    failedEntries: 0
  };
  const usageTraces = traces ?? [];

  if (usageSummary.entryTotal === 0 && usageTraces.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}未记录${modelLabels.llmUsage}`} />;
  }

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <MetricGrid
        items={[
          { key: 'calls', label: 'LLM 调用', value: numberText(usageSummary.entryTotal), color: '#1668dc' },
          { key: 'tokens', label: 'Token 总量', value: numberText(usageSummary.totalTokens), color: '#0958d9' },
          { key: 'input', label: '输入 Token', value: numberText(usageSummary.inputTokens) },
          { key: 'output', label: '输出 Token', value: numberText(usageSummary.outputTokens) },
          { key: 'cost', label: '估算费用', value: llmUsageCostSummary(usageSummary), color: usageSummary.costEstimatedEntries ? '#d48806' : undefined },
          { key: 'failed', label: '失败调用', value: numberText(usageSummary.failedEntries), color: usageSummary.failedEntries ? '#cf1322' : undefined }
        ]}
      />
      <Descriptions
        column={{ xs: 1, md: 3 }}
        size="small"
        items={[
          { key: 'native', label: 'Provider 原生 usage', children: numberText(usageSummary.providerNativeEntries) },
          { key: 'estimated', label: '估算 usage', children: numberText(usageSummary.estimatedEntries) },
          { key: 'costEstimated', label: '估算费用条目', children: numberText(usageSummary.costEstimatedEntries) },
          { key: 'costBreakdown', label: '币种拆分', children: (usageSummary.costBreakdown ?? []).length ? <CompactTags tags={(usageSummary.costBreakdown ?? []).map((item) => `${costText(item.estimatedCost, item.currency)} (${item.entryTotal})`)} maxVisible={4} /> : '-' },
          { key: 'costSource', label: '费用口径', children: usageSummary.costEstimatedEntries ? '基于本地 provider profile pricing 估算，不代表实际账单' : '未配置价格或未产生费用估算' },
          { key: 'providerModels', label: 'Provider/Model', children: numberText(usageSummary.providerModelBreakdown?.length ?? 0) }
        ]}
      />
      <Tabs
        items={[
          {
            key: 'providerModel',
            label: `Provider/Model ${usageSummary.providerModelBreakdown?.length ?? 0}`,
            children: (
              <ProTable<LLMUsageProviderModelBreakdown>
                rowKey={llmUsageProviderModelKey}
                search={false}
                options={false}
                pagination={false}
                size="small"
                columns={[
                  { title: 'Provider', render: (_, item) => `${item.providerId || '-'} / ${item.provider || '-'}`, ellipsis: true },
                  { title: 'Model', dataIndex: 'model', ellipsis: true },
                  { title: '币种', dataIndex: 'currency', width: 90, render: (_, item) => item.currency || '-' },
                  { title: '调用', dataIndex: 'entryTotal', width: 90, render: (_, item) => numberText(item.entryTotal) },
                  { title: '输入/输出', width: 150, render: (_, item) => `${numberText(item.inputTokens)} / ${numberText(item.outputTokens)}` },
                  { title: 'Token', dataIndex: 'totalTokens', width: 120, render: (_, item) => numberText(item.totalTokens) },
                  { title: '估算费用', width: 130, render: (_, item) => costText(item.estimatedCost, item.currency) },
                  { title: '估算/失败', width: 120, render: (_, item) => `${numberText(item.estimatedEntries)} / ${numberText(item.failedEntries)}` }
                ]}
                dataSource={usageSummary.providerModelBreakdown ?? []}
              />
            )
          },
          {
            key: 'traces',
            label: `调用明细 ${usageTraces.length}`,
            children: (
              <ProTable<LLMUsageTrace>
                rowKey="id"
                search={false}
                options={false}
                pagination={{ pageSize: 8 }}
                size="small"
                columns={[
                  { title: '调用', dataIndex: 'id', copyable: true, ellipsis: true },
                  { title: 'Provider', render: (_, item) => `${item.providerId || '-'} / ${item.model || '-'}`, ellipsis: true },
                  { title: '操作', dataIndex: 'operation', ellipsis: true },
                  { title: '状态', dataIndex: 'status', width: 110, render: (_, item) => <StateTag state={item.status || 'unknown'} /> },
                  { title: 'usage 来源', dataIndex: 'usageSource', width: 140, render: (_, item) => item.usageSource || (item.estimated ? 'estimated' : 'provider_native') },
                  { title: 'Token', width: 150, render: (_, item) => `${numberText(item.inputTokens)} / ${numberText(item.outputTokens)} / ${numberText(item.totalTokens)}` },
                  { title: '估算费用', width: 130, render: (_, item) => costText(item.estimatedCost, item.currency) },
                  { title: '耗时', dataIndex: 'durationMs', width: 100, render: (_, item) => item.durationMs ? `${numberText(item.durationMs)} ms` : '-' },
                  { title: '流式', dataIndex: 'streaming', width: 80, render: (_, item) => item.streaming ? <Tag color="blue">stream</Tag> : '-' }
                ]}
                expandable={{
                  expandedRowRender: (item) => (
                    <Descriptions
                      size="small"
                      column={1}
                      items={[
                        { key: 'step', label: '步骤', children: item.stepId ? <BoundedParagraph text={item.stepId} rows={1} copyText={item.stepId} /> : '-' },
                        { key: 'bytes', label: '请求/响应字节', children: `${numberText(item.requestBytes)} / ${numberText(item.responseBytes)}` },
                        { key: 'time', label: '开始/结束时间', children: `${item.startedAt || '-'} / ${item.completedAt || '-'}` },
                        { key: 'failureClass', label: '失败类型', children: item.failureClass || '-' },
                        { key: 'evidenceRef', label: '证据引用', children: item.evidenceRef || '-' }
                      ]}
                    />
                  )
                }}
                dataSource={usageTraces}
              />
            )
          }
        ]}
      />
    </Space>
  );
});
