import { Button, Space, Typography } from 'antd';
import { workorchApi } from '../api';
import type { RuntimeAuditOperationReceipt } from '../types';

export function RuntimeValidationOutputLinks({ outputs }: { outputs?: RuntimeAuditOperationReceipt['validationOutputs'] }) {
  return (
    <Space direction="vertical" size={2}>
      {(outputs ?? []).map((output) => (
        <Space key={`${output.stageId}:${output.stream}`} size={4} wrap>
          {output.ref ? (
            <Button size="small" type="link" href={workorchApi.runtimePayloadURL(output.ref)} target="_blank" rel="noopener noreferrer">
              {output.stageId} / {output.stream}
            </Button>
          ) : <Typography.Text>{output.stageId} / {output.stream}：未保存</Typography.Text>}
          <Typography.Text type="secondary" title={`原始 SHA256：${output.sha256}`}>
            已保存 {output.savedBytes} / {output.bytes} 字节
            {!output.complete || output.savedBytes !== output.bytes ? '（不完整）' : ''}
            {output.omittedReason ? ` · ${output.omittedReason}` : ''}
            {output.storageOmittedReason ? ` · ${output.storageOmittedReason}` : ''}
          </Typography.Text>
        </Space>
      ))}
    </Space>
  );
}
