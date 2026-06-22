import React from 'react';
import { Space, Empty, Descriptions } from 'antd';
import { ProTable } from '../proCompat';
import type { LLMContextBudgetTrace } from '../../types';
import { numberText, compactJSON, percentText } from '../../utils';
import { MetricGrid, CompactTags } from '../common';

interface LLMContextBudgetCardProps {
  traces?: LLMContextBudgetTrace[];
}

const shortHash = (value: unknown) => {
  const text = String(value ?? '').trim();
  if (!text) return '-';
  return text.startsWith('sha256:') ? `sha256:${text.slice(7, 19)}` : text.slice(0, 18);
};

const cachePlanTags = (plan?: Record<string, unknown>) => {
  if (!plan) return [];
  return [
    `C0:${shortHash(plan.stablePrefixHash)}`,
    `C1:${shortHash(plan.workspaceDefinitionHash)}`,
    `C2:${shortHash(plan.capabilityContractHash)}`,
    `C3:${shortHash(plan.stageBundleHash)}`,
    `C4:${shortHash(plan.volatileTailHash)}`
  ];
};

const cacheBreakerTags = (plan?: Record<string, unknown>) => {
  const breakers = Array.isArray(plan?.cacheBreakers) ? plan.cacheBreakers : [];
  return breakers.map((item) => {
    const value = item as Record<string, unknown>;
    return `${value.sectionId ?? '-'}:${value.reason ?? '-'}`;
  });
};

export const LLMContextBudgetCard = React.memo(function LLMContextBudgetCard({ traces }: LLMContextBudgetCardProps) {
  const items = traces ?? [];

  if (items.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前运行未记录 LLM Context Budget" />;
  }

  const truncatedCount = items.filter((item) => item.truncated).length;
  const overBudgetCount = items.filter((item) => item.overBudget).length;
  const p0OverflowCount = items.filter((item) => item.p0Overflow).length;
  const goalDoDCompressedCount = items.filter((item) => item.goalDoDCompressed).length;
  const latest = items[items.length - 1];

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <MetricGrid
        items={[
          { key: 'traceCount', label: '预算轨迹', value: numberText(items.length), color: '#1668dc' },
          { key: 'truncatedCount', label: '触发压缩', value: numberText(truncatedCount), color: truncatedCount ? '#d48806' : undefined },
          { key: 'overBudgetCount', label: '超预算', value: numberText(overBudgetCount), color: overBudgetCount ? '#cf1322' : undefined },
          { key: 'p0OverflowCount', label: 'P0 Overflow', value: numberText(p0OverflowCount), color: p0OverflowCount ? '#cf1322' : undefined },
          { key: 'goalDoDCompressedCount', label: 'Goal/DoD 被压缩', value: numberText(goalDoDCompressedCount), color: goalDoDCompressedCount ? '#cf1322' : '#389e0d' },
          { key: 'latestUtilization', label: '最新利用率', value: percentText(latest?.contextUtilizationRatio), color: latest?.contextUtilizationRatio && latest.contextUtilizationRatio > 0.85 ? '#d48806' : undefined },
          { key: 'profileMode', label: 'Profile Mode', value: latest?.profileMode || '-' },
          { key: 'capabilitySource', label: '能力来源', value: latest?.capabilitySource || '-' }
        ]}
      />
      <ProTable<LLMContextBudgetTrace>
        rowKey={(item, index) => `${item.stepId}-${index}`}
        search={false}
        options={false}
        pagination={{ pageSize: 8 }}
        size="small"
        columns={[
          { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
          { title: 'Provider/Model', render: (_, item) => `${item.providerId || '-'} / ${item.model || '-'}`, ellipsis: true },
          { title: '字节预算', width: 150, render: (_, item) => `${numberText(item.actualBytes)} / ${numberText(item.maxBytes)}` },
          { title: 'Token 预算', width: 150, render: (_, item) => `${numberText(item.estimatedInputTokens)} / ${numberText(item.maxTokens)}` },
          { title: '压缩级别', dataIndex: 'shrinkLevel', width: 90, render: (_, item) => numberText(item.shrinkLevel) },
          { title: '利用率', width: 100, render: (_, item) => percentText(item.contextUtilizationRatio) },
          { title: '状态', width: 220, render: (_, item) => <CompactTags tags={[item.truncated ? 'truncated' : 'full', item.overBudget ? 'over_budget' : '', item.p0Overflow ? 'p0_overflow' : '', item.goalDoDCompressed ? 'goal_dod_compressed' : 'goal_dod_protected'].filter(Boolean)} maxVisible={5} /> }
        ]}
        expandable={{
          expandedRowRender: (item) => (
            <Descriptions
              size="small"
              column={1}
              items={[
                { key: 'rules', label: '预算规则', children: <CompactTags tags={item.rules ?? []} maxVisible={8} /> },
                { key: 'sections', label: '分区处理', children: <CompactTags tags={[`included:${(item.includedSections ?? []).join('|') || '-'}`, `normalized:${(item.normalizedSections ?? []).join('|') || '-'}`, `compressed:${(item.compressedSections ?? []).join('|') || '-'}`, `omitted:${(item.omittedSections ?? []).join('|') || '-'}`]} maxVisible={8} /> },
                { key: 'refs', label: '来源引用', children: <CompactTags tags={item.sourceRefs ?? []} maxVisible={8} /> },
                { key: 'cachePlan', label: 'Cache Plan', children: <CompactTags tags={cachePlanTags(item.cachePlan)} maxVisible={8} /> },
                { key: 'cacheBreakers', label: 'Cache Breakers', children: <CompactTags tags={cacheBreakerTags(item.cachePlan)} maxVisible={8} /> },
                { key: 'profileMode', label: 'Profile', children: `${item.profileMode || '-'} / source: ${item.capabilitySource || '-'}` },
                { key: 'stageBudget', label: 'Stage Budget', children: `${percentText(item.stageBudgetRatio)} / source: ${item.stageBudgetSource || '-'}` },
                { key: 'requestBytes', label: '请求字节上限', children: numberText(item.maxRequestBytes) },
                { key: 'policy', label: 'trace 策略', children: `${item.tracePolicy || '-'} / estimator: ${item.estimator || '-'}` },
                { key: 'profile', label: 'Profile', children: <pre className="json-panel">{compactJSON(item.profile)}</pre> }
              ]}
            />
          )
        }}
        dataSource={items}
      />
    </Space>
  );
});
