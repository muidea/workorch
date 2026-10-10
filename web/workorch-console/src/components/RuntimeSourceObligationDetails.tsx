import { Descriptions, Empty, Table, Typography } from 'antd';
import type { RuntimeResultCloseout, RuntimeSourceObligationOutcome } from '../types';

export function RuntimeSourceObligationDetails({ closeout }: { closeout?: RuntimeResultCloseout }) {
  if (!closeout) return null;
  const rows = (closeout.RequirementOutcomes ?? []).flatMap((item) =>
    (item.SourceObligations ?? []).map((source) => ({ ...source, coverageId: item.CoverageWorkItemID }))
  );
  if (!rows.length) return null;
  return (
    <section aria-label="源义务验收">
      <Typography.Title level={5}>源义务验收</Typography.Title>
      <Typography.Paragraph>以下为服务端已封存的逐源结论；共享 CWI 不合并 A/B 的证明条件，历史证据不能恢复当前验收。</Typography.Paragraph>
      <Descriptions size="small" column={1} items={[
        { key: 'definition', label: '定义修订', children: closeout.DefinitionRevision },
        { key: 'startup', label: '启动输入', children: closeout.StartupInputHash },
        { key: 'hash', label: '结果身份', children: closeout.CloseoutHash }
      ]} />
      <Table<RuntimeSourceObligationOutcome & { coverageId: string }>
        size="small"
        pagination={false}
        rowKey={(row) => `${row.coverageId}/${row.Check.CheckRef}`}
        dataSource={rows}
        locale={{ emptyText: <Empty description="尚无源义务结论" /> }}
        columns={[
          { title: '源义务 / 验证', render: (_, row) => `${row.Check.ObligationID} / ${row.Check.VerificationID}` },
          { title: 'Requirement / CWI', render: (_, row) => `${row.Check.RequirementID} / ${row.coverageId}` },
          { title: '原判据', render: (_, row) => <span style={{ whiteSpace: 'pre-wrap' }}>{row.Check.Criterion}</span> },
          { title: '验证声明', render: (_, row) => <span style={{ whiteSpace: 'pre-wrap' }}>{row.Check.Declaration}</span> },
          { title: '覆盖 / 验收', render: (_, row) => `${row.CoverageState} / ${row.AssessmentState}` },
          { title: '当前 binding', render: (_, row) => row.CurrentBindingRef?.BindingID || '-' },
          { title: '已接受证据', render: (_, row) => row.AcceptedEvidenceRefs?.join('\n') || '-' },
          { title: '未满足原因', render: (_, row) => row.AssessmentReasonCode || '-' }
        ]}
      />
    </section>
  );
}
