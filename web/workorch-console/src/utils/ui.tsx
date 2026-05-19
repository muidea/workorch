import React from 'react';
import { Tag, Typography } from 'antd';
import { stateLabel } from './format';

export function stateTag(state?: string) {
  const value = state || 'unknown';
  const color = value.includes('failed') || value.includes('reject') ? 'red' : value.includes('block') || value.includes('pending') ? 'gold' : 'green';
  return <Tag color={color}>{stateLabel(value)}</Tag>;
}

export type MetricItem = {
  key: string;
  label: string;
  value: React.ReactNode;
  color?: string;
};

export function metricGrid(items: MetricItem[]) {
  return (
    <div className="workorch-metric-grid">
      {items.map((item) => (
        <div key={item.key} className="workorch-metric-item">
          <Typography.Text type="secondary">{item.label}</Typography.Text>
          <Typography.Title level={4} style={{ margin: 0, color: item.color }}>{item.value}</Typography.Title>
        </div>
      ))}
    </div>
  );
}

export function boundedParagraph(value?: React.ReactNode, options?: { rows?: number; copyText?: string; className?: string }) {
  return (
    <Typography.Paragraph
      className={['workorch-bounded-text', options?.className].filter(Boolean).join(' ')}
      copyable={options?.copyText ? { text: options.copyText } : false}
      ellipsis={{ rows: options?.rows ?? 2, expandable: true, symbol: '展开' }}
    >
      {value || '-'}
    </Typography.Paragraph>
  );
}
