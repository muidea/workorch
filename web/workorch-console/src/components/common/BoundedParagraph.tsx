import { Typography } from 'antd';

const { Paragraph } = Typography;

interface BoundedParagraphProps {
  text?: string;
  rows?: number;
  copyText?: string;
}

export function BoundedParagraph({ text, rows = 3, copyText }: BoundedParagraphProps) {
  if (!text) {
    return <Typography.Text type="secondary">-</Typography.Text>;
  }

  return (
    <Paragraph
      ellipsis={{ rows, expandable: true, symbol: '展开' }}
      copyable={copyText ? { text: copyText } : false}
      className="workorch-bounded-text"
    >
      {text}
    </Paragraph>
  );
}
