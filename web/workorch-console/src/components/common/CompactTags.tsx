import { Tag, Typography } from 'antd';

const { Text } = Typography;

interface CompactTagsProps {
  tags: string[];
  maxVisible?: number;
}

/**
 * 紧凑标签组件
 * 显示标签列表，超出部分显示数量
 */
export function CompactTags({ tags, maxVisible = 5 }: CompactTagsProps) {
  if (!tags || tags.length === 0) {
    return <Text type="secondary">-</Text>;
  }

  const visible = tags.slice(0, maxVisible);
  const remaining = tags.length - maxVisible;

  return (
    <div className="workorch-compact-tags">
      {visible.map((tag, index) => (
        <Tag key={index}>{tag}</Tag>
      ))}
      {remaining > 0 && (
        <Text type="secondary" style={{ fontSize: 12 }}>
          +{remaining} 更多
        </Text>
      )}
    </div>
  );
}
