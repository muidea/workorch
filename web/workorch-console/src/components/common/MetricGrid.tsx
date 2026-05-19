import { Typography } from 'antd';

const { Text } = Typography;

export interface MetricItem {
  key: string;
  label: string;
  value: number | string;
  color?: string;
}

interface MetricGridProps {
  items: MetricItem[];
}

/**
 * 指标网格组件
 * 用于展示多个指标卡片
 */
export function MetricGrid({ items }: MetricGridProps) {
  return (
    <div className="workorch-metric-grid">
      {items.map((item) => (
        <div key={item.key} className="workorch-metric-item">
          <Text type="secondary" style={{ fontSize: 12 }}>
            {item.label}
          </Text>
          <div style={{ fontSize: 20, fontWeight: 600, color: item.color || '#1668dc', marginTop: 4 }}>
            {item.value}
          </div>
        </div>
      ))}
    </div>
  );
}
