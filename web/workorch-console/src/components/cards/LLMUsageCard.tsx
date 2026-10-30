import { llmCacheTokenValue, llmCacheReadRatio, nativeCacheSummaryItems } from '../../llmUsagePresentation';
import React from 'react';
import { Space, Empty, Tag, Descriptions, Tabs, Alert } from 'antd';
import { ProTable } from '../proCompat';
import type { LLMUsageSummary, LLMUsageTrace, LLMUsageProviderModelBreakdown } from '../../types';
import { numberText, percentText } from '../../utils';
import { StateTag, BoundedParagraph, MetricGrid } from '../common';

interface LLMUsageCardProps {
  summary?: LLMUsageSummary;
  traces?: LLMUsageTrace[];
}

function llmUsageProviderModelKey(item: LLMUsageProviderModelBreakdown, index?: number) {
  return [item.providerId, item.provider, item.model, index].filter(Boolean).join('|');
}

export const LLMUsageCard = React.memo(function LLMUsageCard({ summary, traces }: LLMUsageCardProps) {
  if (!summary || summary.availability === 'not_loaded') {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="LLM 用量未加载；未加载不代表零调用" />;
  }
  const usageSummary = summary;
  const usageTraces = traces ?? [];
  const scopeLabel = summary.scope === 'workspace' ? 'Workspace' : 'Run';
  const incomplete = summary.availability !== 'complete';
  const queryDescription = `仅显示已查询的 ${scopeLabel} 用量窗口（上限 ${summary.recordLimit} 条，返回 ${summary.observedRecords} 条）；不代表全部历史调用。`;

  if (usageSummary.entryTotal === 0 && usageTraces.length === 0) {
    if (incomplete) {
      return <Alert type="warning" showIcon message="LLM 用量数据不完整" description={`${queryDescription} 返回记录尚不能形成完整用量汇总，不能视为零调用。`} />;
    }
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`已查询的 ${scopeLabel} 用量窗口内没有记录；不代表全部历史调用`} />;
  }

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Alert type={incomplete ? 'warning' : 'info'} showIcon message={incomplete ? 'LLM 用量数据不完整' : 'LLM 用量查询范围'} description={queryDescription} />
      <MetricGrid
        items={[
          { key: 'calls', label: 'LLM 调用', value: numberText(usageSummary.entryTotal), color: '#1668dc' },
          { key: 'tokens', label: 'Token 总量', value: numberText(usageSummary.totalTokens), color: '#0958d9' },
          { key: 'input', label: '输入 Token', value: numberText(usageSummary.inputTokens) },
          { key: 'output', label: '输出 Token', value: numberText(usageSummary.outputTokens) },
          { key: 'cacheRead', label: 'Cache Read Token', value: llmCacheTokenValue(usageSummary, 'read') },
          { key: 'cacheWrite', label: 'Cache Write Token', value: llmCacheTokenValue(usageSummary, 'write') },
          { key: 'cacheReadRatio', label: 'Cache Read/Input', value: llmCacheReadRatio(usageSummary), color: usageSummary.cacheReadInputRatio ? '#389e0d' : undefined },
          { key: 'failed', label: '失败调用', value: numberText(usageSummary.failedEntries), color: usageSummary.failedEntries ? '#cf1322' : undefined }
        ]}
      />
      <Descriptions
        column={{ xs: 1, md: 3 }}
        size="small"
        items={[
          { key: 'native', label: 'Provider 原生 usage', children: numberText(usageSummary.providerNativeEntries) },
          { key: 'estimated', label: '估算 usage', children: numberText(usageSummary.estimatedEntries) },
          { key: 'cacheable', label: 'Cacheable/Non-cacheable 输入', children: `${numberText(usageSummary.cacheableInputTokens)} / ${numberText(usageSummary.nonCacheableInputTokens)}` },
          { key: 'cacheRatio', label: 'Cacheable/Non-cacheable 比例', children: `${percentText(usageSummary.cacheableInputRatio)} / ${percentText(usageSummary.nonCacheableInputRatio)}` },
          { key: 'cacheEntries', label: 'Cache 口径', children: `${numberText(usageSummary.cacheProviderEntries)} provider / ${numberText(usageSummary.cacheEstimatedEntries)} estimated` },
          ...nativeCacheSummaryItems(usageSummary.nativeCache),
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
                  { title: '调用', dataIndex: 'entryTotal', width: 90, render: (_, item) => numberText(item.entryTotal) },
                  { title: '输入/输出', width: 150, render: (_, item) => `${numberText(item.inputTokens)} / ${numberText(item.outputTokens)}` },
                  { title: 'Token', dataIndex: 'totalTokens', width: 120, render: (_, item) => numberText(item.totalTokens) },
                  { title: 'Cache R/W', width: 130, render: (_, item) => `${llmCacheTokenValue(item, 'read')} / ${llmCacheTokenValue(item, 'write')}` },
                  { title: 'Cache Read/Input', width: 140, render: (_, item) => llmCacheReadRatio(item) },
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
                  { title: '耗时', dataIndex: 'durationMs', width: 100, render: (_, item) => item.durationMs ? `${numberText(item.durationMs)} ms` : '-' },
                  { title: '物理响应', dataIndex: 'physicalResponseMode', width: 110, render: (_, item) => <Tag color={item.physicalResponseMode === 'streaming' ? 'blue' : 'default'}>{item.physicalResponseMode}</Tag> },
                  { title: 'Provider 提交', dataIndex: 'providerSubmitted', width: 110, render: (_, item) => item.providerSubmitted ? <Tag color="green">submitted</Tag> : <Tag>not submitted</Tag> }
                ]}
                expandable={{
                  expandedRowRender: (item) => (
                    <Descriptions
                      size="small"
                      column={1}
                      items={[
                        { key: 'logicalRequestId', label: '逻辑请求 ID', children: item.logicalRequestId ? <BoundedParagraph text={item.logicalRequestId} rows={1} copyText={item.logicalRequestId} /> : '-' },
                        { key: 'physicalCallId', label: '物理调用 ID', children: item.physicalCallId ? <BoundedParagraph text={item.physicalCallId} rows={1} copyText={item.physicalCallId} /> : '-' },
                        { key: 'step', label: '步骤', children: item.stepId ? <BoundedParagraph text={item.stepId} rows={1} copyText={item.stepId} /> : '-' },
                        { key: 'bytes', label: '请求/响应字节', children: `${numberText(item.requestBytes)} / ${numberText(item.responseBytes)}` },
                        { key: 'time', label: '开始/结束时间', children: `${item.startedAt || '-'} / ${item.completedAt || '-'}` },
                        { key: 'failureClass', label: '失败类型', children: item.failureClass || '-' },
                        { key: 'cacheUsage', label: 'Cache Usage', children: <pre className="json-panel">{JSON.stringify(item.cacheUsage ?? null, null, 2)}</pre> },
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
