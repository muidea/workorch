import { Alert, Space, Tag, Typography } from 'antd';
import type { WorkspaceAuthoringSummary } from '../../types';
import { authoringSummaryPresentation } from '../../workspaceAuthoringSummary';

type Props = {
  summary?: WorkspaceAuthoringSummary;
  onOpenSpecification?: () => void;
  compact?: boolean;
};

export function AuthoringSummary({ summary, onOpenSpecification, compact = false }: Props) {
  const presentation = authoringSummaryPresentation(summary);
  if (!presentation) return null;

  const content = (
    <Space direction="vertical" size={4}>
      <Space wrap>
        <Tag color={presentation.color}>{presentation.label}</Tag>
        {presentation.failurePhase ? <Typography.Text>失败阶段：{presentation.failurePhase}</Typography.Text> : null}
        {presentation.failureCode ? <Typography.Text type="secondary">原因代码：{presentation.failureCode}</Typography.Text> : null}
        {presentation.reviewCount !== undefined ? <Typography.Text>已完成审查：{presentation.reviewCount} 项</Typography.Text> : null}
      </Space>
      <Typography.Text type="secondary">创作状态仅供查看；请进入规格查看完整内容和后续处理方式，不代表执行（Run）状态。</Typography.Text>
    </Space>
  );

  return compact ? (
    <div className="workorch-section" aria-label="规格创作摘要">
      {content}
      {onOpenSpecification ? <Typography.Link onClick={onOpenSpecification}>进入规格查看</Typography.Link> : null}
    </div>
  ) : (
    <Alert
      className="workorch-section"
      type={presentation.type}
      showIcon
      message="规格创作摘要"
      description={(
        <Space direction="vertical" size="small">
          {content}
          {onOpenSpecification ? <Typography.Link onClick={onOpenSpecification}>进入规格查看</Typography.Link> : null}
        </Space>
      )}
    />
  );
}
