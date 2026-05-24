import React from 'react';
import { Space, Empty, Descriptions, Tabs } from 'antd';
import { ProTable } from '../proCompat';
import type { ContextBundle, ContextSection, ContextEvidence, ContextCompressionCandidate } from '../../types';
import { compactJSON } from '../../utils';
import { StateTag, BoundedParagraph, CompactTags } from '../common';
import { modelLabels, recoveryActionLabels } from '../../utils/constants';

interface ContextBundleCardProps {
  bundle?: ContextBundle;
}

function recoveryActionLabel(action?: string) {
  if (!action) return '-';
  return recoveryActionLabels[action] || action;
}

export const ContextBundleCard = React.memo(function ContextBundleCard({ bundle }: ContextBundleCardProps) {
  if (!bundle) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无上下文包" />;
  }

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Descriptions
        column={{ xs: 1, md: 3 }}
        size="small"
        items={[
          { key: 'mode', label: '上下文模式', children: bundle.mode },
          { key: 'compressedMode', label: '压缩模式', children: bundle.compressedContext?.mode || '-' },
          { key: 'run', label: `关联${modelLabels.run}`, children: bundle.runId || '-' },
          { key: 'summary', label: '摘要', children: <BoundedParagraph text={bundle.summary} rows={2} /> },
          { key: 'recovery', label: '恢复建议', children: recoveryActionLabel(bundle.recovery?.nextAction) },
          { key: 'checkpoint', label: '恢复检查点', children: bundle.latestCheckpoint?.id ? <BoundedParagraph text={bundle.latestCheckpoint.id} rows={1} copyText={bundle.latestCheckpoint.id} /> : '-' },
          { key: 'compression', label: '压缩候选', children: `${bundle.compressionCandidates?.length ?? 0} 项` }
        ]}
      />
      <Tabs
        items={[
          {
            key: 'sections',
            label: `上下文分区 ${bundle.sections?.length ?? 0}`,
            children: (
              <ProTable<ContextSection>
                rowKey="id"
                search={false}
                options={false}
                pagination={{ pageSize: 6 }}
                size="small"
                columns={[
                  { title: '分区', dataIndex: 'title', ellipsis: true },
                  { title: '类型', dataIndex: 'kind', width: 160 },
                  { title: '优先级', dataIndex: 'priority', width: 90 },
                  { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => item.state ? <StateTag state={item.state} /> : '-' },
                  { title: '摘要', dataIndex: 'summary', ellipsis: true }
                ]}
                expandable={{
                  expandedRowRender: (item) => <pre className="json-panel json-panel-large">{compactJSON(item.data)}</pre>
                }}
                dataSource={bundle.sections ?? []}
              />
            )
          },
          {
            key: 'compressed',
            label: '压缩上下文',
            children: (
              <Descriptions
                column={1}
                size="small"
                items={[
                  { key: 'version', label: '版本', children: bundle.compressedContext?.version || '-' },
                  { key: 'summary', label: '压缩摘要', children: <BoundedParagraph text={bundle.compressedContext?.summary} rows={3} /> },
                  { key: 'retention', label: '保留策略', children: <BoundedParagraph text={bundle.compressedContext?.retentionPolicy} rows={2} /> },
                  { key: 'sourceSections', label: '来源分区', children: <CompactTags tags={bundle.compressedContext?.sourceSectionRefs ?? []} /> },
                  { key: 'sourceEvidence', label: '来源证据', children: <CompactTags tags={bundle.compressedContext?.sourceEvidenceRefs ?? []} /> }
                ]}
              />
            )
          },
          {
            key: 'evidence',
            label: `最近证据 ${bundle.recentEvidence?.length ?? 0}`,
            children: (
              <ProTable<ContextEvidence>
                rowKey={(item, index) => `${item.kind}-${item.id}-${index}`}
                search={false}
                options={false}
                pagination={{ pageSize: 6 }}
                size="small"
                columns={[
                  { title: '证据', dataIndex: 'id', copyable: true, ellipsis: true },
                  { title: '类型', dataIndex: 'kind', width: 180 },
                  { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => item.state ? <StateTag state={item.state} /> : '-' },
                  { title: '摘要', dataIndex: 'summary', ellipsis: true },
                  { title: '时间', dataIndex: 'occurredAt', width: 190 }
                ]}
                dataSource={bundle.recentEvidence ?? []}
              />
            )
          },
          {
            key: 'candidates',
            label: `压缩候选 ${bundle.compressionCandidates?.length ?? 0}`,
            children: (
              <ProTable<ContextCompressionCandidate>
                rowKey="sectionId"
                search={false}
                options={false}
                pagination={{ pageSize: 6 }}
                size="small"
                columns={[
                  { title: '分区', dataIndex: 'sectionId', ellipsis: true },
                  { title: '原因', dataIndex: 'reason', ellipsis: true },
                  { title: '优先级', dataIndex: 'priority', width: 90 }
                ]}
                dataSource={bundle.compressionCandidates ?? []}
              />
            )
          }
        ]}
      />
    </Space>
  );
});
