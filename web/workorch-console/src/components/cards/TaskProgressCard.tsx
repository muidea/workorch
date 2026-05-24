import React from 'react';
import { Space, Steps, Descriptions, Empty } from 'antd';
import { ProTable } from '../proCompat';
import type { TaskProgress, RepairLoop, ProgressBlocker, ProgressItem } from '../../types';
import {
  stateLabel,
  taskProgressStepStatus,
  numberText,
  compactJSON,
  asStringArray
} from '../../utils';
import { StateTag, BoundedParagraph, CompactTags } from '../common';

interface TaskProgressCardProps {
  progress?: TaskProgress;
  repairLoopOverride?: RepairLoop;
}

// 辅助函数
function recoveryModeLabel(mode?: string) {
  const labels: Record<string, string> = {
    auto: '自动恢复',
    manual: '手动恢复',
    checkpoint: '检查点恢复',
    unknown: '未知'
  };
  return labels[mode || ''] || mode || '-';
}

function recoveryActionLabel(action?: string) {
  const labels: Record<string, string> = {
    retry: '重试',
    rollback: '回滚',
    escalate: '升级',
    wait: '等待',
    unknown: '未知'
  };
  return labels[action || ''] || action || '-';
}

function repairLoopStateLabel(state?: string) {
  const labels: Record<string, string> = {
    active: '活跃',
    paused: '暂停',
    completed: '完成',
    failed: '失败',
    escalated: '已升级'
  };
  return labels[state || ''] || state || '-';
}

function progressKindLabel(kind: string) {
  const labels: Record<string, string> = {
    task: '任务',
    subtask: '子任务',
    milestone: '里程碑',
    blocker: '阻塞项',
    decision: '决策',
    approval: '审批'
  };
  return labels[kind] || kind;
}

export const TaskProgressCard = React.memo(function TaskProgressCard({ progress, repairLoopOverride }: TaskProgressCardProps) {
  if (!progress) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无任务进度投影" />;
  }

  const blockers = progress.blockers ?? [];
  const recovery = progress.recovery ?? { mode: 'unknown' };
  const repairLoop = repairLoopOverride ?? progress.repairLoop;
  const repairAttempt = repairLoop?.attemptCount ?? 0;
  const repairFailure = [repairLoop?.lastFailureClass, repairLoop?.lastFailureSignature].filter(Boolean).join(' / ');
  const repairEscalation = repairLoop?.escalated
    ? [repairLoop.escalationTicketId, repairLoop.escalationReason].filter(Boolean).join(' / ')
    : '-';
  const repairSignals = repairLoop?.structuredFailureSignals;
  const repairSignalTargets = asStringArray(repairSignals?.targetRefs);
  const repairSignalIdentifiers = asStringArray(repairSignals?.identifiers);
  const repairCompactionFields = asStringArray(repairLoop?.repairInputCompaction?.omittedFields);

  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Steps
        size="small"
        items={(progress.milestones ?? []).map((item) => ({
          title: item.title,
          description: stateLabel(item.state),
          status: taskProgressStepStatus(item.state)
        }))}
      />
      <Descriptions
        column={{ xs: 1, md: 2 }}
        size="small"
        items={[
          { key: 'state', label: '任务状态', children: <StateTag state={progress.state} /> },
          { key: 'currentMilestone', label: '当前里程碑', children: progress.currentMilestone || '-' },
          { key: 'summary', label: '进度摘要', children: <BoundedParagraph text={progress.summary} rows={2} /> },
          { key: 'recoveryMode', label: '恢复模式', children: recoveryModeLabel(recovery.mode) },
          { key: 'nextAction', label: '建议动作', children: recoveryActionLabel(recovery.nextAction) },
          { key: 'checkpoint', label: '恢复检查点', children: recovery.checkpointId ? <BoundedParagraph text={recovery.checkpointId} rows={1} copyText={recovery.checkpointId} /> : '-' },
          { key: 'blockingTicket', label: '阻塞决策', children: recovery.blockingTicketId || '-' },
          { key: 'blockingApproval', label: '阻塞审批', children: recovery.blockingApprovalId || '-' },
          { key: 'repairState', label: '修复状态', children: repairLoop ? repairLoopStateLabel(repairLoop.state) : '-' },
          { key: 'repairAttempt', label: '修复轮次', children: repairAttempt || '-' },
          { key: 'repairFailure', label: '最近失败', children: repairFailure || '-' },
          { key: 'repairTargetScope', label: '失败目标', children: repairLoop?.lastFailureTargetScope || '-' },
          { key: 'repairInputBytes', label: '修复输入字节', children: repairLoop?.lastRepairInputBytes ? numberText(repairLoop.lastRepairInputBytes) : '-' },
          { key: 'repairRepeatedReason', label: '重复失败原因', children: repairLoop?.repeatedFailureReason || '-' },
          { key: 'repairNextAction', label: '修复下一步', children: repairLoop?.nextAction || '-' },
          { key: 'repairEscalation', label: '升级信息', children: repairEscalation },
          { key: 'repairSignalTargets', label: '失败信号目标', children: <CompactTags tags={repairSignalTargets} maxVisible={4} /> },
          { key: 'repairSignalIdentifiers', label: '失败信号标识符', children: <CompactTags tags={repairSignalIdentifiers} maxVisible={4} /> },
          { key: 'repairCompaction', label: '修复输入裁剪', children: repairCompactionFields.length ? <CompactTags tags={repairCompactionFields} maxVisible={4} /> : '-' },
          { key: 'repairEvidence', label: '修复证据', children: <CompactTags tags={repairLoop?.evidenceRefs ?? []} maxVisible={4} /> }
        ]}
      />
      {repairSignals || repairLoop?.repairInputCompaction ? (
        <Descriptions
          column={{ xs: 1, md: 2 }}
          size="small"
          items={[
            {
              key: 'signals',
              label: 'Structured Failure Signals',
              children: repairSignals ? <pre className="json-panel">{compactJSON(repairSignals)}</pre> : '-'
            },
            {
              key: 'compaction',
              label: 'Repair Input Compaction',
              children: repairLoop?.repairInputCompaction ? <pre className="json-panel">{compactJSON(repairLoop.repairInputCompaction)}</pre> : '-'
            }
          ]}
        />
      ) : null}
      {blockers.length ? (
        <ProTable<ProgressBlocker>
          rowKey="id"
          search={false}
          options={false}
          pagination={false}
          size="small"
          columns={[
            { title: '阻塞项', dataIndex: 'id', copyable: true, ellipsis: true },
            { title: '类型', dataIndex: 'kind', width: 120, render: (_, item) => progressKindLabel(String(item.kind)) },
            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => <StateTag state={String(item.state)} /> },
            { title: '原因', dataIndex: 'reason', ellipsis: true },
            { title: '证据', dataIndex: 'evidenceRef', ellipsis: true }
          ]}
          dataSource={blockers}
        />
      ) : null}
      {(progress.items?.length ?? 0) > 0 ? (
        <ProTable<ProgressItem>
          rowKey="id"
          search={false}
          options={false}
          pagination={{ pageSize: 6 }}
          size="small"
          columns={[
            { title: '项目', dataIndex: 'id', copyable: true, ellipsis: true },
            { title: '类型', dataIndex: 'kind', width: 140, render: (_, item) => progressKindLabel(String(item.kind)) },
            { title: '名称', dataIndex: 'title', ellipsis: true },
            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => <StateTag state={String(item.state)} /> }
          ]}
          dataSource={progress.items ?? []}
        />
      ) : null}
    </Space>
  );
});
