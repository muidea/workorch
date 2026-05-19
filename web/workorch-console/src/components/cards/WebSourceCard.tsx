import React from 'react';
import { Space, Empty, Tag, Typography, Descriptions } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { WebSourceTrace } from '../../types';
import { BoundedParagraph, CompactTags, MetricGrid } from '../common';
import { modelLabels } from '../../utils/constants';

interface WebSourceCardProps {
  items?: WebSourceTrace[];
}

type WebSourceRow = {
  key: string;
  kind: 'search' | 'fetch';
  providerId?: string;
  query?: string;
  title?: string;
  url: string;
  finalUrl?: string;
  source?: string;
  occurredAt?: string;
  contentHash?: string;
  truncated?: boolean;
  evidenceRef?: string;
  approvalId?: string;
  policyDecision?: string;
};

function webSourceRows(items?: WebSourceTrace[]) {
  const rows: WebSourceRow[] = [];
  for (const trace of items ?? []) {
    for (const source of trace.sources ?? []) {
      rows.push({
        key: `search-${source.evidenceRef || source.url}-${rows.length}`,
        kind: 'search',
        providerId: trace.providerId,
        query: trace.query,
        title: source.title,
        url: source.url,
        source: source.source,
        occurredAt: trace.searchedAt || source.publishedAt,
        evidenceRef: source.evidenceRef || trace.evidenceRefs?.[0],
        approvalId: trace.approvalId,
        policyDecision: trace.policyDecision
      });
    }
    for (const source of trace.fetchedSources ?? []) {
      rows.push({
        key: `fetch-${source.evidenceRef || source.finalUrl || source.url}-${rows.length}`,
        kind: 'fetch',
        providerId: trace.providerId,
        title: source.title,
        url: source.url,
        finalUrl: source.finalUrl,
        source: source.source,
        occurredAt: source.retrievedAt,
        contentHash: source.contentHash,
        truncated: source.truncated,
        evidenceRef: source.evidenceRef || trace.evidenceRefs?.[0],
        approvalId: trace.approvalId,
        policyDecision: trace.policyDecision
      });
    }
  }
  return rows;
}

export const WebSourceCard = React.memo(function WebSourceCard({ items }: WebSourceCardProps) {
  const rows = webSourceRows(items);

  if (rows.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前运行未记录 Web 来源证据" />;
  }

  const fetched = rows.filter((item) => item.kind === 'fetch').length;
  const approvals = new Set(rows.map((item) => item.approvalId).filter(Boolean)).size;

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <MetricGrid
        items={[
          { key: 'sources', label: '来源总数', value: rows.length, color: '#1668dc' },
          { key: 'fetched', label: '已读取来源', value: fetched, color: fetched ? '#389e0d' : undefined },
          { key: 'approvals', label: modelLabels.approval, value: approvals, color: approvals ? '#d48806' : undefined }
        ]}
      />
      <ProTable<WebSourceRow>
        rowKey="key"
        search={false}
        options={false}
        pagination={{ pageSize: 6 }}
        size="small"
        columns={[
          { title: '类型', dataIndex: 'kind', width: 90, render: (_, item) => <Tag color={item.kind === 'fetch' ? 'green' : 'blue'}>{item.kind === 'fetch' ? 'Fetch' : 'Search'}</Tag> },
          { title: '来源', render: (_, item) => <BoundedParagraph text={item.title || item.source || item.url} rows={2} copyText={item.url} /> },
          { title: 'URL', render: (_, item) => <Typography.Link href={item.finalUrl || item.url} target="_blank" rel="noreferrer">{item.finalUrl || item.url}</Typography.Link>, ellipsis: true },
          { title: '证据', dataIndex: 'evidenceRef', ellipsis: true, render: (_, item) => item.evidenceRef ? <CompactTags tags={[item.evidenceRef]} maxVisible={1} /> : '-' },
          { title: '策略', dataIndex: 'policyDecision', width: 110, render: (_, item) => item.policyDecision || '-' },
          { title: '时间', dataIndex: 'occurredAt', width: 180, ellipsis: true },
          { title: 'Hash', dataIndex: 'contentHash', ellipsis: true, render: (_, item) => item.contentHash || '-' }
        ]}
        expandable={{
          expandedRowRender: (item) => (
            <Descriptions
              size="small"
              column={1}
              items={[
                { key: 'query', label: '查询', children: item.query || '-' },
                { key: 'provider', label: 'Provider', children: item.providerId || '-' },
                { key: 'approval', label: modelLabels.approval, children: item.approvalId || '-' },
                { key: 'truncated', label: '是否截断', children: item.truncated ? '是' : '否' },
                { key: 'url', label: '原始 URL', children: <BoundedParagraph text={item.url} rows={1} copyText={item.url} /> }
              ]}
            />
          )
        }}
        dataSource={rows}
      />
    </Space>
  );
});
