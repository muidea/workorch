import React, { useState } from 'react';
import ReactDOM from 'react-dom/client';
import {
  CheckCircleOutlined,
  ClusterOutlined,
  DashboardOutlined,
  ExceptionOutlined,
  ProfileOutlined,
  SettingOutlined
} from '@ant-design/icons';
import { ProCard, ProLayout, ProTable } from '@ant-design/pro-components';
import { App as AntApp, Button, ConfigProvider, Descriptions, Empty, Form, Input, List, Select, Space, Steps, Tabs, Tag, Typography, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import type { ContextBundle, ContextCompressionCandidate, ContextEvidence, ContextSection, Decision, DoD, EventRecord, ExecutionSection, Goal, LLMContextBudgetTrace, LLMModelCapabilityConfiguration, LLMProviderTemplate, LLMUsageProviderModelBreakdown, LLMUsageSummary, LLMUsageTrace, MemoryRecallTrace, ProgressBlocker, ProgressItem, RepairLoop, Result, Run, RunObservability, RunSnapshot, RuntimeCapability, RuntimeTimelineItem, ServiceConfiguration, Skill, Soul, TaskDecomposition, TaskDecompositionNode, TaskProgress, UpdateLLMConfigurationRequest, UpdateLLMProviderConfiguration, UpdateWebSearchConfigurationRequest, UpdateWebSearchProviderConfiguration, WebSearchProviderTemplate, WebSourceTrace, WorkspaceExecutionActivationRequest, WorkspaceSetupDraft, WorkspaceSnapshot, WorkspaceSoulBindings, WorkspaceTemplate } from './types';
import { workorchApi } from './api';
import { ServicePage } from './pages/ServicePage';
import { GovernanceQueuePage } from './pages/GovernanceQueuePage';
import { WorkspaceBoardPage } from './pages/WorkspaceBoardPage';
import { RuntimeControlPage } from './pages/RuntimeControlPage';
import { ConfigurationPage } from './pages/ConfigurationPage';
import { WorkspaceDetailPage } from './pages/WorkspaceDetailPage';
import './styles.css';

export type ViewKey = 'board' | 'workspaceDetail' | 'governance' | 'runtimeControl' | 'configuration' | 'service';

export const runtimeEventPageLimit = 100;
export const runtimeStepPageLimit = 100;
export const runtimeToolCallPageLimit = 100;
export const runtimeCheckpointPageLimit = 100;
export const runtimeResultPageLimit = 100;
export const runtimePatchPageLimit = 100;
export const runtimeValidationPageLimit = 100;
export const runtimeArtifactPageLimit = 100;

export function mergeById<T extends { id: string }>(current: T[], items: T[], append: boolean) {
  if (!append) {
    return items;
  }
  const seen = new Set(current.map((item) => item.id));
  return [
    ...current,
    ...items.filter((item) => {
      if (seen.has(item.id)) {
        return false;
      }
      seen.add(item.id);
      return true;
    })
  ];
}

export const routes = [
  { path: '/board', name: '业务总览', icon: <DashboardOutlined /> },
  { path: '/workspaceDetail', name: '工作空间', icon: <ProfileOutlined /> },
  { path: '/governance', name: '治理队列', icon: <ExceptionOutlined /> },
  { path: '/runtimeControl', name: '运行控制', icon: <ClusterOutlined /> },
  { path: '/configuration', name: '系统配置', icon: <SettingOutlined /> },
  { path: '/service', name: '服务状态', icon: <CheckCircleOutlined /> }
];

export const stateLabels: Record<string, string> = {
  unknown: '未知',
  active: '活跃',
  created: '已创建',
  running: '运行中',
  validating: '验收中',
  completed: '已完成',
  accepted: '已接受',
  failed: '失败',
  cancelled: '已取消',
  superseded: '已替代',
  blocked_on_decision: '等待决策',
  blocked_on_approval: '等待审批',
  waiting_decision: '等待决策',
  waiting_approval: '等待审批',
  open: '待处理',
  resolved: '已解决',
  pending: '待审批',
  approved: '已批准',
  rejected: '已拒绝',
  ready: '就绪',
  satisfied: '已满足'
};

export const progressKindLabels: Record<string, string> = {
  step: '步骤',
  tool_call: '工具调用',
  patch: '补丁（Patch）',
  validation_result: '校验结果',
  result: '结果（Result）',
  artifact: '交付物（Artifact）',
  decision: '决策工单',
  approval: '审批工单'
};

export const recoveryModeLabels: Record<string, string> = {
  idle: '待启动',
  continue: '可继续',
  wait_decision: '等待决策',
  wait_approval: '等待审批',
  inspect_failure: '排查失败',
  restart: '可重新启动',
  none: '无需恢复'
};

export const recoveryActionLabels: Record<string, string> = {
  start_workspace_execution: '启动工作空间执行',
  inspect_active_run: '查看活跃运行',
  continue_run: '继续运行',
  continue_validation: '继续验收',
  resolve_decision_ticket: '补充或处理决策工单',
  resolve_approval: '处理审批工单',
  inspect_result: '查看结果（Result）/交付物（Artifact）',
  inspect_failure_reason_and_evidence: '查看失败原因和运行证据',
  start_new_run: '启动新的运行'
};

export const repairLoopStateLabels: Record<string, string> = {
  inactive: '未进入修复',
  repairing: '自动修复中',
  awaiting_decision: '等待决策',
  awaiting_approval: '等待审批',
  completed_after_repair: '修复后完成',
  escalated_decision: '已升级决策',
  terminated: '修复终止'
};

export const defaultSoulBindingStages = ['planning', 'execution', 'decision', 'approval', 'validation', 'artifact'];

export const soulBindingStageLabels: Record<string, string> = {
  planning: '规划',
  execution: '执行',
  decision: '决策',
  approval: '审批',
  validation: '验收',
  artifact: '交付物'
};

export const modelLabels = {
  workspace: '工作空间（Workspace）',
  goal: '目标（Goal）',
  dod: '验收标准（DoD）',
  doDAssessment: 'DoD 验收评估',
  doDItem: 'DoD 验收项',
  run: '运行（Run）',
  decision: '决策工单（Decision）',
  approval: '审批工单（Approval）',
  result: '结果（Result）',
  artifact: '交付物（Artifact）',
  memory: '记忆（Memory）',
  memoryRecall: '记忆召回（Memory Recall）',
  memoryRecallTrace: '记忆召回轨迹',
  executionSection: '执行分区（Execution Section）',
  soul: '执行人格（Soul）',
  skill: '技能（Skill）',
  mcpServer: 'MCP 服务（MCP Server）',
  mcpCapability: 'MCP 能力（Tool / Resource / Prompt）',
  mcpTool: '工具（Tool）',
  mcpResource: '资源（Resource）',
  mcpPrompt: '提示词（Prompt）',
  step: '步骤（Step）',
  patch: '补丁（Patch）',
  toolCall: '工具调用（Tool Call）',
  validationResult: '校验结果（Validation Result）',
  runtimeCheckpoint: '运行检查点（Runtime Checkpoint）',
  llmProvider: 'LLM 服务商（Provider）',
  llmUsage: 'LLM 使用量 / 费用',
  metrics: '运行指标（Metrics）'
};

export function stateLabel(state?: string) {
  if (!state) {
    return '未知';
  }
  return stateLabels[state] ? `${stateLabels[state]} (${state})` : state;
}

export function soulBindingStageLabel(stage: string) {
  return soulBindingStageLabels[stage] ? `${soulBindingStageLabels[stage]} (${stage})` : stage;
}

export function progressKindLabel(kind?: string) {
  if (!kind) {
    return '未知';
  }
  return progressKindLabels[kind] ?? kind;
}

export function recoveryModeLabel(mode?: string) {
  if (!mode) {
    return '未知';
  }
  return recoveryModeLabels[mode] ? `${recoveryModeLabels[mode]} (${mode})` : mode;
}

export function recoveryActionLabel(action?: string) {
  if (!action) {
    return '-';
  }
  return recoveryActionLabels[action] ?? action;
}

export function repairLoopStateLabel(state?: string) {
  if (!state) {
    return '未知';
  }
  return repairLoopStateLabels[state] ? `${repairLoopStateLabels[state]} (${state})` : state;
}

export function stateTag(state?: string) {
  const value = state || 'unknown';
  const color = value.includes('failed') || value.includes('reject') ? 'red' : value.includes('block') || value.includes('pending') ? 'gold' : 'green';
  return <Tag color={color}>{stateLabel(value)}</Tag>;
}

export function shortId(value?: string, keep = 10) {
  if (!value) {
    return '-';
  }
  return value.length > keep ? `${value.slice(0, keep)}…` : value;
}

export function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : '操作失败';
}

export function isNotFoundError(error: unknown) {
  return /(^|\s)404(\s|$)|not found|不存在/i.test(errorMessage(error));
}

export function pendingCount(items?: Array<{ state: string }>) {
  return (items ?? []).filter((item) => item.state === 'open' || item.state === 'pending').length;
}

export function runStateSummary(runs?: Run[]) {
  const items = runs ?? [];
  return {
    total: items.length,
    created: items.filter((item) => item.state === 'created').length,
    active: items.filter((item) => item.state === 'running').length,
    blocked: items.filter((item) => item.state === 'blocked_on_decision' || item.state === 'blocked_on_approval').length,
    validating: items.filter((item) => item.state === 'validating').length,
    completed: items.filter((item) => item.state === 'completed').length,
    failed: items.filter((item) => item.state === 'failed' || item.state === 'cancelled' || item.state === 'superseded').length,
    latest: items[items.length - 1]
  };
}

export function workspaceProjectionRun(snapshot?: WorkspaceSnapshot) {
  if (!snapshot) {
    return undefined;
  }
  const activeRun = snapshot.runs?.find((item) => item.id === snapshot.workspace.activeRunId);
  if (activeRun) {
    return activeRun;
  }
  const outcomeRun = snapshot.runs?.find((item) => item.id === snapshot.outcome?.runId);
  if (outcomeRun) {
    return outcomeRun;
  }
  const progressRun = snapshot.runs?.find((item) => item.id === snapshot.taskProgress?.runId);
  if (progressRun) {
    return progressRun;
  }
  return runStateSummary(snapshot.runs).latest;
}

export function workspaceProjectionRunLabel(snapshot?: WorkspaceSnapshot) {
  const run = workspaceProjectionRun(snapshot);
  return run ? `${run.id} / ${stateLabel(run.state)}` : '-';
}

export function isBlockedRunState(state?: string) {
  return state === 'blocked_on_decision' || state === 'blocked_on_approval';
}

export function isCompletedRunState(state?: string) {
  return state === 'completed';
}

export function isFailedRunState(state?: string) {
  return state === 'failed' || state === 'cancelled' || state === 'superseded';
}

export function isTerminalRunState(state?: string) {
  return isCompletedRunState(state) || isFailedRunState(state);
}

export function activeOrLatestRun(snapshot?: WorkspaceSnapshot) {
  return workspaceProjectionRun(snapshot);
}

export function workspaceProcessItems(snapshot?: WorkspaceSnapshot, goals?: Goal[], dods?: DoD[]) {
  const currentRun = activeOrLatestRun(snapshot);
  const hasDefinition = Boolean((goals?.length ?? 0) > 0 && (dods?.length ?? 0) > 0);
  const missingDefinition = !goals?.length ? `需要定义${modelLabels.goal}` : !dods?.length ? `需要定义${modelLabels.dod}` : `需要补齐${modelLabels.goal} / ${modelLabels.dod}`;
  const pendingDecisions = pendingCount(snapshot?.decisions);
  const pendingApprovals = pendingCount(snapshot?.approvals);
  const blocked = isBlockedRunState(currentRun?.state) || pendingDecisions > 0 || pendingApprovals > 0;
  const terminal = isTerminalRunState(currentRun?.state);
  const failed = isFailedRunState(currentRun?.state);
  const completed = isCompletedRunState(currentRun?.state);
  const governanceDescription = pendingDecisions > 0 || pendingApprovals > 0
    ? `待处理：决策 ${pendingDecisions} / 审批 ${pendingApprovals}`
    : blocked ? stateLabel(currentRun?.state) : '无待处理阻塞';
  const evidenceDescription = completed
    ? stateLabel(currentRun?.state)
    : failed ? stateLabel(currentRun?.state)
      : currentRun?.state === 'validating' ? `正在执行${modelLabels.doDAssessment}`
        : currentRun ? '等待运行证据闭环' : `尚未启动${modelLabels.run}`;
  return [
    { title: '工作空间', description: snapshot?.workspace ? stateLabel(snapshot.workspace.state) : '未选择', status: snapshot?.workspace ? 'finish' as const : 'wait' as const },
    { title: '目标与验收', description: hasDefinition ? `${modelLabels.goal} / ${modelLabels.dod} 已定义` : missingDefinition, status: hasDefinition ? 'finish' as const : 'process' as const },
    { title: '启动执行', description: currentRun ? `${currentRun.id} / ${stateLabel(currentRun.state)}` : `尚未启动${modelLabels.run}`, status: currentRun ? 'finish' as const : 'wait' as const },
    { title: '治理处理', description: governanceDescription, status: blocked ? 'process' as const : currentRun ? 'finish' as const : 'wait' as const },
    { title: '证据验收', description: evidenceDescription, status: failed ? 'error' as const : completed ? 'finish' as const : terminal ? 'finish' as const : currentRun ? 'process' as const : 'wait' as const }
  ];
}

export function runtimeSummaryTags(runs?: Run[]) {
  const summary = runStateSummary(runs);
  return (
    <Space wrap>
      <Tag>总数 {summary.total}</Tag>
      <Tag color="blue">运行中 {summary.active}</Tag>
      <Tag color="gold">阻塞 {summary.blocked}</Tag>
      <Tag color="cyan">验收中 {summary.validating}</Tag>
      <Tag color="green">完成 {summary.completed}</Tag>
      <Tag color="red">失败 {summary.failed}</Tag>
    </Space>
  );
}

export function taskProgressStepStatus(state?: string) {
  if (state === 'failed') {
    return 'error' as const;
  }
  if (state === 'completed') {
    return 'finish' as const;
  }
  if (state === 'running' || state === 'blocked') {
    return 'process' as const;
  }
  return 'wait' as const;
}

export function taskProgressCard(progress?: TaskProgress, repairLoopOverride?: RepairLoop) {
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
          { key: 'state', label: '任务状态', children: stateTag(progress.state) },
          { key: 'currentMilestone', label: '当前里程碑', children: progress.currentMilestone || '-' },
          { key: 'summary', label: '进度摘要', children: boundedParagraph(progress.summary, { rows: 2 }) },
          { key: 'recoveryMode', label: '恢复模式', children: recoveryModeLabel(recovery.mode) },
          { key: 'nextAction', label: '建议动作', children: recoveryActionLabel(recovery.nextAction) },
          { key: 'checkpoint', label: '恢复检查点', children: recovery.checkpointId ? boundedParagraph(recovery.checkpointId, { rows: 1, copyText: recovery.checkpointId }) : '-' },
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
          { key: 'repairSignalTargets', label: '失败信号目标', children: compactTags(repairSignalTargets, 4) },
          { key: 'repairSignalIdentifiers', label: '失败信号标识符', children: compactTags(repairSignalIdentifiers, 4) },
          { key: 'repairCompaction', label: '修复输入裁剪', children: repairCompactionFields.length ? compactTags(repairCompactionFields, 4) : '-' },
          { key: 'repairEvidence', label: '修复证据', children: compactTags(repairLoop?.evidenceRefs ?? [], 4) }
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
            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(String(item.state)) },
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
            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(String(item.state)) }
          ]}
          dataSource={progress.items ?? []}
        />
      ) : null}
    </Space>
  );
}

export function taskDecompositionCard(items?: TaskDecomposition[]) {
  const decompositions = items ?? [];
  if (decompositions.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无任务拆分图" />;
  }
  const latest = decompositions.at(-1);
  const total = decompositions.reduce((sum, item) => sum + item.nodeTotal, 0);
  const completed = decompositions.reduce((sum, item) => sum + item.nodeCompleted, 0);
  const failed = decompositions.reduce((sum, item) => sum + item.nodeFailed, 0);
  const running = decompositions.reduce((sum, item) => sum + item.nodeRunning, 0);
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      <Descriptions
        column={{ xs: 1, md: 4 }}
        size="small"
        items={[
          { key: 'graph', label: '拆分图', children: decompositions.length },
          { key: 'node', label: '节点', children: `${completed}/${total} 已完成` },
          { key: 'running', label: '执行中', children: running },
          { key: 'failed', label: '失败', children: failed },
          { key: 'latest', label: '最新根步骤', children: latest?.rootStepId ? boundedParagraph(latest.rootStepId, { rows: 1, copyText: latest.rootStepId }) : '-' },
          { key: 'state', label: '最新状态', children: latest ? stateTag(latest.state) : '-' },
          { key: 'branch', label: '分支', children: latest?.graphBranch || '-' },
          { key: 'summary', label: '摘要', children: boundedParagraph(latest?.summary, { rows: 2 }) }
        ]}
      />
      <Tabs
        items={decompositions.map((item, index) => ({
          key: item.id || `${item.rootStepId}-${index}`,
          label: `拆分图 ${index + 1} / ${stateLabel(item.state)}`,
          children: (
            <Space direction="vertical" size="small" style={{ width: '100%' }}>
              {metricGrid([
                { key: 'nodes', label: '节点总数', value: item.nodeTotal },
                { key: 'completed', label: '已完成', value: item.nodeCompleted, color: '#389e0d' },
                { key: 'running', label: '执行中', value: item.nodeRunning, color: '#0958d9' },
                { key: 'failed', label: '失败', value: item.nodeFailed, color: '#cf1322' }
              ])}
              <ProTable<TaskDecompositionNode>
                rowKey={(node, nodeIndex) => `${item.id}-${node.id}-${nodeIndex}`}
                search={false}
                options={false}
                pagination={{ pageSize: 8 }}
                size="small"
                columns={[
                  { title: '节点', dataIndex: 'id', copyable: true, ellipsis: true },
                  { title: '类型', dataIndex: 'kind', width: 150 },
                  { title: '标题', dataIndex: 'title', ellipsis: true, render: (_, node) => boundedParagraph(node.title, { rows: 2 }) },
                  { title: '状态', dataIndex: 'state', width: 110, render: (_, node) => stateTag(node.state) },
                  { title: '依赖', dataIndex: 'dependsOn', width: 180, render: (_, node) => compactTags(node.dependsOn ?? []) },
                  { title: '执行步骤', dataIndex: 'stepId', copyable: true, ellipsis: true }
                ]}
                expandable={{
                  expandedRowRender: (node) => (
                    <Descriptions
                      column={1}
                      size="small"
                      items={[
                        { key: 'stepState', label: '步骤状态', children: node.stepState ? stateLabel(node.stepState) : '-' },
                        { key: 'branch', label: '分支', children: node.branch || '-' },
                        { key: 'evidence', label: '证据', children: compactTags(node.evidenceRefs ?? [], 6) },
                        { key: 'input', label: '输入', children: <pre className="json-panel">{compactJSON(node.input)}</pre> }
                      ]}
                    />
                  )
                }}
                dataSource={item.nodes ?? []}
              />
            </Space>
          )
        }))}
      />
    </Space>
  );
}

export function executionSectionsCard(items?: ExecutionSection[]) {
  const sections = items ?? [];
  if (sections.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`暂无${modelLabels.executionSection}`} />;
  }
  const completed = sections.filter((item) => item.state === 'completed').length;
  const failed = sections.filter((item) => item.state === 'failed').length;
  const running = sections.filter((item) => item.state === 'running').length;
  const memoryRecallRefs = new Set(sections.flatMap((item) => item.memoryRecallRefs ?? [])).size;
  const memoryIds = new Set(sections.flatMap((item) => item.memoryIds ?? [])).size;
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {metricGrid([
        { key: 'total', label: '分区总数', value: sections.length, color: '#1668dc' },
        { key: 'completed', label: '已完成', value: completed, color: '#389e0d' },
        { key: 'running', label: '执行中', value: running, color: '#0958d9' },
        { key: 'failed', label: '失败', value: failed, color: '#cf1322' },
        { key: 'memoryRecallRefs', label: modelLabels.memoryRecall, value: memoryRecallRefs, color: '#08979c' },
        { key: 'memoryIds', label: `${modelLabels.memory}引用`, value: memoryIds, color: '#531dab' }
      ])}
      <ProTable<ExecutionSection>
        rowKey="id"
        search={false}
        options={false}
        pagination={{ pageSize: 8 }}
        size="small"
        columns={[
          { title: '分区', dataIndex: 'title', ellipsis: true, render: (_, item) => boundedParagraph(item.title || item.id, { rows: 2 }) },
          { title: '状态', dataIndex: 'state', width: 110, render: (_, item) => stateTag(item.state) },
          { title: '图节点', dataIndex: 'graphNodeId', copyable: true, ellipsis: true },
          { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
          { title: modelLabels.memoryRecall, dataIndex: 'memoryRecallRefs', width: 180, render: (_, item) => compactTags(item.memoryRecallRefs ?? [], 3) },
          { title: modelLabels.memory, dataIndex: 'memoryIds', width: 160, render: (_, item) => compactTags(item.memoryIds ?? [], 3) },
          { title: '证据', dataIndex: 'actualEvidenceRefs', width: 220, render: (_, item) => compactTags(item.actualEvidenceRefs ?? [], 4) }
        ]}
        expandable={{
          expandedRowRender: (item) => (
            <Descriptions
              column={1}
              size="small"
              items={[
                { key: 'summary', label: '摘要', children: boundedParagraph(item.summary, { rows: 2 }) },
                { key: 'dod', label: '关联验收项', children: compactTags(item.relatedDoDItems ?? [], 6) },
                { key: 'planned', label: '计划引用', children: compactTags(item.plannedStepRefs ?? [], 6) },
                { key: 'governance', label: '治理工单', children: compactTags([...(item.decisionRefs ?? []), ...(item.approvalRefs ?? [])], 6) },
                { key: 'runtime', label: '运行证据', children: compactTags([...(item.toolCallRefs ?? []), ...(item.patchRefs ?? []), ...(item.validationRefs ?? [])], 6) },
                { key: 'outputs', label: `${modelLabels.result}/${modelLabels.artifact}`, children: compactTags([...(item.resultRefs ?? []), ...(item.artifactRefs ?? [])], 6) },
                { key: 'memoryRecall', label: `${modelLabels.memoryRecall}证据`, children: compactTags(item.memoryRecallRefs ?? [], 8) },
                { key: 'memoryIds', label: `${modelLabels.memory}引用`, children: compactTags(item.memoryIds ?? [], 8) }
              ]}
            />
          )
        }}
        dataSource={sections}
      />
    </Space>
  );
}

export type MemoryRecallGroup = {
  key: string;
  source: string;
  stage: string;
  status: string;
  traces: MemoryRecallTrace[];
  memoryIds: string[];
};

export function memoryRecallGroups(items: MemoryRecallTrace[]) {
  const grouped = new Map<string, MemoryRecallTrace[]>();
  for (const item of items) {
    const source = item.source || 'unknown';
    const stage = item.stage || 'unknown';
    const key = `${source}/${stage}`;
    grouped.set(key, [...(grouped.get(key) ?? []), item]);
  }
  return Array.from(grouped.entries())
    .map(([key, traces]) => {
      const [source, stage] = key.split('/');
      const status = traces.some((item) => item.status === 'degraded') ? 'degraded' : traces.some((item) => item.status === 'selected') ? 'selected' : traces[0]?.status || 'unknown';
      const memoryIds = Array.from(new Set(traces.flatMap((item) => item.retrievedMemoryIds ?? [])));
      return { key, source, stage, status, traces, memoryIds };
    })
    .sort((a, b) => a.key.localeCompare(b.key));
}

export function memoryRecallQueryPanel(item: MemoryRecallTrace) {
  return compactJSON({
    policy: item.policy,
    providerId: item.providerId,
    operation: item.operation,
    queryTextPreview: item.queryTextPreview,
    queryTextSha256: item.queryTextSha256,
    minScore: item.minScore,
    limit: item.limit,
    embeddingModel: item.embeddingModel,
    vectorDimension: item.vectorDimension,
    occurredAt: item.occurredAt,
    evidenceRef: item.evidenceRef
  });
}

export function memoryRecallCard(items?: MemoryRecallTrace[]) {
  const traces = items ?? [];
  if (traces.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}未记录${modelLabels.memoryRecall}证据`} />;
  }
  const groups = memoryRecallGroups(traces);
  const memoryIds = new Set(traces.flatMap((item) => item.retrievedMemoryIds ?? [])).size;
  const degraded = traces.filter((item) => item.status === 'degraded').length;
  const selected = traces.filter((item) => item.status === 'selected').length;
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {metricGrid([
        { key: 'traces', label: modelLabels.memoryRecallTrace, value: traces.length, color: '#1668dc' },
        { key: 'groups', label: '阶段分组', value: groups.length, color: '#08979c' },
        { key: 'selected', label: '已选中', value: selected, color: '#389e0d' },
        { key: 'degraded', label: '降级', value: degraded, color: degraded ? '#cf1322' : undefined },
        { key: 'memoryIds', label: modelLabels.memory, value: memoryIds, color: '#531dab' }
      ])}
      <ProTable<MemoryRecallGroup>
        rowKey="key"
        search={false}
        options={false}
        pagination={false}
        size="small"
        columns={[
          { title: '分组', dataIndex: 'key', render: (_, item) => <Space><Tag color="blue">{item.source}</Tag><Tag>{item.stage}</Tag></Space> },
          { title: '状态', dataIndex: 'status', width: 110, render: (_, item) => stateTag(item.status) },
          { title: modelLabels.memoryRecallTrace, width: 140, render: (_, item) => item.traces.length },
          { title: `命中${modelLabels.memory}`, dataIndex: 'memoryIds', ellipsis: true, render: (_, item) => compactTags(item.memoryIds, 5) }
        ]}
        expandable={{
          expandedRowRender: (group) => (
            <ProTable<MemoryRecallTrace>
              rowKey="id"
              search={false}
              options={false}
              pagination={false}
              size="small"
              columns={[
                { title: modelLabels.memoryRecallTrace, dataIndex: 'id', copyable: true, ellipsis: true },
                { title: '策略', dataIndex: 'policy', ellipsis: true },
                { title: '状态', dataIndex: 'status', width: 100, render: (_, item) => stateTag(item.status) },
                { title: `命中${modelLabels.memory}`, dataIndex: 'retrievedMemoryIds', ellipsis: true, render: (_, item) => compactTags(item.retrievedMemoryIds ?? [], 5) },
                { title: '原因', dataIndex: 'reason', ellipsis: true, render: (_, item) => boundedParagraph(item.reason || item.queryTextPreview || '-', { rows: 2 }) }
              ]}
              dataSource={group.traces}
              expandable={{
                expandedRowRender: (item) => (
                  <ProCard gutter={16} wrap ghost>
                    <ProCard title="召回查询" colSpan={{ xs: 24, lg: 12 }}>
                      <pre className="json-panel json-panel-large">{memoryRecallQueryPanel(item)}</pre>
                    </ProCard>
                    <ProCard title={`命中${modelLabels.memory}详情`} colSpan={{ xs: 24, lg: 12 }}>
                      <pre className="json-panel json-panel-large">{compactJSON(item.retrievedMemoryHits ?? [])}</pre>
                    </ProCard>
                  </ProCard>
                )
              }}
            />
          )
        }}
        dataSource={groups}
      />
    </Space>
  );
}

export type WebSourceRow = {
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

export function webSourceRows(items?: WebSourceTrace[]) {
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

export function webSourceCard(items?: WebSourceTrace[]) {
  const rows = webSourceRows(items);
  if (rows.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前运行未记录 Web 来源证据" />;
  }
  const fetched = rows.filter((item) => item.kind === 'fetch').length;
  const approvals = new Set(rows.map((item) => item.approvalId).filter(Boolean)).size;
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {metricGrid([
        { key: 'sources', label: '来源总数', value: rows.length, color: '#1668dc' },
        { key: 'fetched', label: '已读取来源', value: fetched, color: fetched ? '#389e0d' : undefined },
        { key: 'approvals', label: modelLabels.approval, value: approvals, color: approvals ? '#d48806' : undefined }
      ])}
      <ProTable<WebSourceRow>
        rowKey="key"
        search={false}
        options={false}
        pagination={{ pageSize: 6 }}
        size="small"
        columns={[
          { title: '类型', dataIndex: 'kind', width: 90, render: (_, item) => <Tag color={item.kind === 'fetch' ? 'green' : 'blue'}>{item.kind === 'fetch' ? 'Fetch' : 'Search'}</Tag> },
          { title: '来源', render: (_, item) => boundedParagraph(item.title || item.source || item.url, { rows: 2, copyText: item.url }) },
          { title: 'URL', render: (_, item) => <Typography.Link href={item.finalUrl || item.url} target="_blank" rel="noreferrer">{item.finalUrl || item.url}</Typography.Link>, ellipsis: true },
          { title: '证据', dataIndex: 'evidenceRef', ellipsis: true, render: (_, item) => item.evidenceRef ? compactTags([item.evidenceRef], 1) : '-' },
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
                { key: 'url', label: '原始 URL', children: boundedParagraph(item.url, { rows: 1, copyText: item.url }) }
              ]}
            />
          )
        }}
        dataSource={rows}
      />
    </Space>
  );
}

export function numberText(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
}

export function costText(value?: number, currency?: string) {
  if (!value || value <= 0) {
    return '-';
  }
  const formatted = new Intl.NumberFormat('zh-CN', {
    minimumFractionDigits: 0,
    maximumFractionDigits: value < 0.01 ? 8 : 4
  }).format(value);
  return currency ? `${formatted} ${currency}` : formatted;
}

export function llmUsageCostSummary(summary?: LLMUsageSummary) {
  if (!summary || summary.entryTotal === 0) {
    return '-';
  }
  if (summary.currency && summary.estimatedCost) {
    return costText(summary.estimatedCost, summary.currency);
  }
  const breakdown = summary.costBreakdown ?? [];
  if (breakdown.length === 0) {
    return '-';
  }
  return breakdown.map((item) => costText(item.estimatedCost, item.currency)).join(' / ');
}

export function llmUsageProviderModelKey(item: LLMUsageProviderModelBreakdown, index?: number) {
  return [item.providerId, item.provider, item.model, item.currency, index].filter(Boolean).join('|');
}

export function llmUsageCard(summary?: LLMUsageSummary, traces?: LLMUsageTrace[]) {
  const usageSummary = summary ?? {
    entryTotal: 0,
    inputTokens: 0,
    outputTokens: 0,
    totalTokens: 0,
    estimatedEntries: 0,
    providerNativeEntries: 0,
    costEstimatedEntries: 0,
    failedEntries: 0
  };
  const usageTraces = traces ?? [];
  if (usageSummary.entryTotal === 0 && usageTraces.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}未记录${modelLabels.llmUsage}`} />;
  }
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {metricGrid([
        { key: 'calls', label: 'LLM 调用', value: numberText(usageSummary.entryTotal), color: '#1668dc' },
        { key: 'tokens', label: 'Token 总量', value: numberText(usageSummary.totalTokens), color: '#0958d9' },
        { key: 'input', label: '输入 Token', value: numberText(usageSummary.inputTokens) },
        { key: 'output', label: '输出 Token', value: numberText(usageSummary.outputTokens) },
        { key: 'cost', label: '估算费用', value: llmUsageCostSummary(usageSummary), color: usageSummary.costEstimatedEntries ? '#d48806' : undefined },
        { key: 'failed', label: '失败调用', value: numberText(usageSummary.failedEntries), color: usageSummary.failedEntries ? '#cf1322' : undefined }
      ])}
      <Descriptions
        column={{ xs: 1, md: 3 }}
        size="small"
        items={[
          { key: 'native', label: 'Provider 原生 usage', children: numberText(usageSummary.providerNativeEntries) },
          { key: 'estimated', label: '估算 usage', children: numberText(usageSummary.estimatedEntries) },
          { key: 'costEstimated', label: '估算费用条目', children: numberText(usageSummary.costEstimatedEntries) },
          { key: 'costBreakdown', label: '币种拆分', children: (usageSummary.costBreakdown ?? []).length ? compactTags((usageSummary.costBreakdown ?? []).map((item) => `${costText(item.estimatedCost, item.currency)} (${item.entryTotal})`), 4) : '-' },
          { key: 'costSource', label: '费用口径', children: usageSummary.costEstimatedEntries ? '基于本地 provider profile pricing 估算，不代表实际账单' : '未配置价格或未产生费用估算' },
          { key: 'providerModels', label: 'Provider/Model', children: numberText(usageSummary.providerModelBreakdown?.length ?? 0) }
        ]}
      />
      <Tabs
        items={[
          {
            key: 'providerModel',
            label: `Provider/Model ${usageSummary.providerModelBreakdown?.length ?? 0}`,
            children: (
              <ProTable<LLMUsageProviderModelBreakdown>
                rowKey={llmUsageProviderModelKey}
                search={false}
                options={false}
                pagination={false}
                size="small"
                columns={[
                  { title: 'Provider', render: (_, item) => `${item.providerId || '-'} / ${item.provider || '-'}`, ellipsis: true },
                  { title: 'Model', dataIndex: 'model', ellipsis: true },
                  { title: '币种', dataIndex: 'currency', width: 90, render: (_, item) => item.currency || '-' },
                  { title: '调用', dataIndex: 'entryTotal', width: 90, render: (_, item) => numberText(item.entryTotal) },
                  { title: '输入/输出', width: 150, render: (_, item) => `${numberText(item.inputTokens)} / ${numberText(item.outputTokens)}` },
                  { title: 'Token', dataIndex: 'totalTokens', width: 120, render: (_, item) => numberText(item.totalTokens) },
                  { title: '估算费用', width: 130, render: (_, item) => costText(item.estimatedCost, item.currency) },
                  { title: '估算/失败', width: 120, render: (_, item) => `${numberText(item.estimatedEntries)} / ${numberText(item.failedEntries)}` }
                ]}
                dataSource={usageSummary.providerModelBreakdown ?? []}
              />
            )
          },
          {
            key: 'traces',
            label: `调用明细 ${usageTraces.length}`,
            children: (
              <ProTable<LLMUsageTrace>
                rowKey="id"
                search={false}
                options={false}
                pagination={{ pageSize: 8 }}
                size="small"
                columns={[
                  { title: '调用', dataIndex: 'id', copyable: true, ellipsis: true },
                  { title: 'Provider', render: (_, item) => `${item.providerId || '-'} / ${item.model || '-'}`, ellipsis: true },
                  { title: '操作', dataIndex: 'operation', ellipsis: true },
                  { title: '状态', dataIndex: 'status', width: 110, render: (_, item) => stateTag(item.status || 'unknown') },
                  { title: 'usage 来源', dataIndex: 'usageSource', width: 140, render: (_, item) => item.usageSource || (item.estimated ? 'estimated' : 'provider_native') },
                  { title: 'Token', width: 150, render: (_, item) => `${numberText(item.inputTokens)} / ${numberText(item.outputTokens)} / ${numberText(item.totalTokens)}` },
                  { title: '估算费用', width: 130, render: (_, item) => costText(item.estimatedCost, item.currency) },
                  { title: '耗时', dataIndex: 'durationMs', width: 100, render: (_, item) => item.durationMs ? `${numberText(item.durationMs)} ms` : '-' },
                  { title: '流式', dataIndex: 'streaming', width: 80, render: (_, item) => item.streaming ? <Tag color="blue">stream</Tag> : '-' }
                ]}
                expandable={{
                  expandedRowRender: (item) => (
                    <Descriptions
                      size="small"
                      column={1}
                      items={[
                        { key: 'step', label: '步骤', children: item.stepId ? boundedParagraph(item.stepId, { rows: 1, copyText: item.stepId }) : '-' },
                        { key: 'bytes', label: '请求/响应字节', children: `${numberText(item.requestBytes)} / ${numberText(item.responseBytes)}` },
                        { key: 'cost', label: '费用语义', children: `${item.costEstimated ? '估算' : '未估算'} / ${item.costSource || '-'}` },
                        { key: 'failure', label: '失败分类', children: item.failureClass || '-' },
                        { key: 'time', label: '时间', children: [item.startedAt, item.completedAt || item.occurredAt].filter(Boolean).join(' → ') || '-' },
                        { key: 'budget', label: 'Context Budget', children: <pre className="json-panel">{compactJSON(item.contextBudget)}</pre> }
                      ]}
                    />
                  )
                }}
                dataSource={usageTraces}
              />
            )
          }
        ]}
      />
    </Space>
  );
}

export function llmContextBudgetCard(traces?: LLMContextBudgetTrace[]) {
  const items = traces ?? [];
  if (items.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前运行未记录 LLM Context Budget" />;
  }
  const truncatedCount = items.filter((item) => item.truncated).length;
  const p0OverflowCount = items.filter((item) => item.p0Overflow).length;
  const goalDoDCompressedCount = items.filter((item) => item.goalDoDCompressed).length;
  return (
    <Space direction="vertical" size="middle" style={{ width: '100%' }}>
      {metricGrid([
        { key: 'traceCount', label: '预算轨迹', value: numberText(items.length), color: '#1668dc' },
        { key: 'truncatedCount', label: '触发压缩', value: numberText(truncatedCount), color: truncatedCount ? '#d48806' : undefined },
        { key: 'p0OverflowCount', label: 'P0 Overflow', value: numberText(p0OverflowCount), color: p0OverflowCount ? '#cf1322' : undefined },
        { key: 'goalDoDCompressedCount', label: 'Goal/DoD 被压缩', value: numberText(goalDoDCompressedCount), color: goalDoDCompressedCount ? '#cf1322' : '#389e0d' }
      ])}
      <ProTable<LLMContextBudgetTrace>
        rowKey={(item, index) => `${item.stepId}-${index}`}
        search={false}
        options={false}
        pagination={{ pageSize: 8 }}
        size="small"
        columns={[
          { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
          { title: 'Provider/Model', render: (_, item) => `${item.providerId || '-'} / ${item.model || '-'}`, ellipsis: true },
          { title: '字节预算', width: 150, render: (_, item) => `${numberText(item.actualBytes)} / ${numberText(item.maxBytes)}` },
          { title: 'Token 预算', width: 150, render: (_, item) => `${numberText(item.estimatedInputTokens)} / ${numberText(item.maxTokens)}` },
          { title: '压缩级别', dataIndex: 'shrinkLevel', width: 90, render: (_, item) => numberText(item.shrinkLevel) },
          { title: '状态', width: 180, render: (_, item) => compactTags([item.truncated ? 'truncated' : 'full', item.p0Overflow ? 'p0_overflow' : '', item.goalDoDCompressed ? 'goal_dod_compressed' : 'goal_dod_protected'].filter(Boolean), 4) }
        ]}
        expandable={{
          expandedRowRender: (item) => (
            <Descriptions
              size="small"
              column={1}
              items={[
                { key: 'rules', label: '预算规则', children: compactTags(item.rules ?? [], 8) },
                { key: 'sections', label: '分区处理', children: compactTags([`included:${(item.includedSections ?? []).join('|') || '-'}`, `normalized:${(item.normalizedSections ?? []).join('|') || '-'}`, `compressed:${(item.compressedSections ?? []).join('|') || '-'}`, `omitted:${(item.omittedSections ?? []).join('|') || '-'}`], 8) },
                { key: 'refs', label: '来源引用', children: compactTags(item.sourceRefs ?? [], 8) },
                { key: 'policy', label: 'trace 策略', children: `${item.tracePolicy || '-'} / estimator: ${item.estimator || '-'}` },
                { key: 'profile', label: 'Profile', children: <pre className="json-panel">{compactJSON(item.profile)}</pre> }
              ]}
            />
          )
        }}
        dataSource={items}
      />
    </Space>
  );
}

export function contextBundleCard(bundle?: ContextBundle) {
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
          { key: 'summary', label: '摘要', children: boundedParagraph(bundle.summary, { rows: 2 }) },
          { key: 'recovery', label: '恢复建议', children: recoveryActionLabel(bundle.recovery?.nextAction) },
          { key: 'checkpoint', label: '恢复检查点', children: bundle.latestCheckpoint?.id ? boundedParagraph(bundle.latestCheckpoint.id, { rows: 1, copyText: bundle.latestCheckpoint.id }) : '-' },
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
                  { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => item.state ? stateTag(item.state) : '-' },
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
                  { key: 'summary', label: '压缩摘要', children: boundedParagraph(bundle.compressedContext?.summary, { rows: 3 }) },
                  { key: 'retention', label: '保留策略', children: boundedParagraph(bundle.compressedContext?.retentionPolicy, { rows: 2 }) },
                  { key: 'sourceSections', label: '来源分区', children: compactTags(bundle.compressedContext?.sourceSectionRefs ?? []) },
                  { key: 'sourceEvidence', label: '来源证据', children: compactTags(bundle.compressedContext?.sourceEvidenceRefs ?? []) }
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
                  { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => item.state ? stateTag(item.state) : '-' },
                  { title: '摘要', dataIndex: 'summary', ellipsis: true },
                  { title: '时间', dataIndex: 'occurredAt', width: 190 }
                ]}
                expandable={{
                  expandedRowRender: (item) => <pre className="json-panel json-panel-large">{compactJSON(item.data)}</pre>
                }}
                dataSource={bundle.recentEvidence ?? []}
              />
            )
          },
          {
            key: 'compression',
            label: `压缩候选 ${bundle.compressionCandidates?.length ?? 0}`,
            children: (
              <ProTable<ContextCompressionCandidate>
                rowKey={(item, index) => `${item.sectionId}-${index}`}
                search={false}
                options={false}
                pagination={false}
                size="small"
                columns={[
                  { title: '分区', dataIndex: 'sectionId', width: 180 },
                  { title: '建议模式', dataIndex: 'suggestedMode', width: 150 },
                  { title: '原因', dataIndex: 'reason', ellipsis: true },
                  { title: '保留策略', dataIndex: 'retention', ellipsis: true }
                ]}
                dataSource={bundle.compressionCandidates ?? []}
              />
            )
          }
        ]}
      />
    </Space>
  );
}

export function skillScopeLabel(skill: Skill) {
  if (skill.source === 'skill-hub') {
    return skill.scope === 'workspace' ? '工作空间 SkillHub 技能' : '全局 SkillHub 技能';
  }
  return '内置技能';
}

export function skillDisplayLabel(skill: Skill) {
  const name = skill.name || skill.sourceSkillId || skill.id;
  const version = skill.version ? `@${skill.version}` : '';
  const status = skill.status ? ` / ${skill.status}` : '';
  return `${name}${version} / ${skillScopeLabel(skill)}${status}`;
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

export function runtimeCapabilityTable(items: RuntimeCapability[] | undefined, emptyText: string) {
  if (!items?.length) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} />;
  }
  return (
    <ProTable<RuntimeCapability>
      rowKey="id"
      search={false}
      options={false}
      pagination={false}
      size="small"
      columns={[
        { title: '业务能力', dataIndex: 'displayName', ellipsis: true },
        { title: '类型', dataIndex: 'kind', width: 100, render: (_, item) => capabilityKindTag(item.kind) },
        { title: '技术标识', dataIndex: 'name', ellipsis: true },
        { title: '服务', dataIndex: 'serverId', ellipsis: true },
        { title: '分类', dataIndex: 'category', width: 120, render: (_, item) => item.category || '-' },
        { title: '风险', dataIndex: 'riskLevel', width: 110, render: (_, item) => item.riskLevel || '-' },
        { title: '使用次数', dataIndex: 'useCount', width: 100, render: (_, item) => item.useCount ?? '-' },
        { title: '证据', dataIndex: 'evidenceRefs', ellipsis: true, render: (_, item) => (item.evidenceRefs ?? []).join(', ') || '-' }
      ]}
      dataSource={items}
    />
  );
}

export function capabilityKindTag(kind?: string) {
  if (kind === 'tool') return <Tag color="blue">{modelLabels.mcpTool}</Tag>;
  if (kind === 'resource') return <Tag color="cyan">{modelLabels.mcpResource}</Tag>;
  if (kind === 'prompt') return <Tag color="geekblue">{modelLabels.mcpPrompt}</Tag>;
  return kind ? <Tag>{kind}</Tag> : '-';
}

export function evidenceCount(snapshot?: RunSnapshot) {
  if (!snapshot) {
    return 0;
  }
  return (snapshot.toolCalls?.length ?? 0) +
    (snapshot.patches?.length ?? 0) +
    (snapshot.validationResults?.length ?? 0) +
    (snapshot.runtimeCheckpoints?.length ?? 0) +
    (snapshot.results?.length ?? 0) +
    (snapshot.artifacts?.length ?? 0);
}

export function timelineLabel(item: RuntimeTimelineItem) {
  return item.topic || item.type || item.source;
}

export function compactJSON(value: unknown) {
  if (value === undefined || value === null) {
    return '-';
  }
  return JSON.stringify(value, null, 2);
}

export function secondaryCellText(value: string) {
  return <Typography.Text type="secondary">{value}</Typography.Text>;
}

export function timelineStreamContent(item: RuntimeTimelineItem) {
  if (item.source === 'step_stream') {
    const delta = outputText(item.data, 'delta');
    if (delta !== '-') {
      return delta;
    }
    return item.type ? `事件类型：${item.type}` : '已记录流式片段';
  }
  const raw = (item.output ?? item.data)?.streamEvents;
  if (Array.isArray(raw) && raw.length > 0) {
    return <Tag color="cyan">已记录 {raw.length} 条</Tag>;
  }
  return secondaryCellText('不适用');
}

export function timelineOccurredAtContent(item: RuntimeTimelineItem) {
  return item.occurredAt || secondaryCellText('未记录');
}

export function recordValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

export type DecisionPayloadField = {
  name: string;
  label: string;
  required: boolean;
  initialValue?: unknown;
};

export function stringListFromValue(value: unknown) {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.map((item) => {
    if (typeof item === 'string') {
      return item;
    }
    const record = recordValue(item);
    return String(record?.name ?? record?.key ?? record?.field ?? '');
  }).map((item) => item.trim()).filter(Boolean);
}

export function primitiveSuggestedPayload(context: Record<string, unknown> | undefined) {
  const payload: Record<string, unknown> = {};
  if (!context) {
    return payload;
  }
  for (const [key, value] of Object.entries(context)) {
    const lowerKey = key.toLowerCase();
    const shouldUse = lowerKey.startsWith('suggested') ||
      lowerKey.startsWith('default') ||
      lowerKey === 'candidate' ||
      lowerKey === 'payload';
    const record = shouldUse ? recordValue(value) : undefined;
    if (!record) {
      continue;
    }
    for (const [field, fieldValue] of Object.entries(record)) {
      if (fieldValue === null || fieldValue === undefined) {
        continue;
      }
      if (typeof fieldValue === 'string' || typeof fieldValue === 'number' || typeof fieldValue === 'boolean') {
        payload[field] = fieldValue;
      }
    }
  }
  return payload;
}

export function decisionRequiredFields(item: Decision) {
  const context = recordValue(item.context);
  const requiredFields = [
    ...stringListFromValue(context?.requiredFields),
    ...stringListFromValue(context?.fields)
  ];
  const suggestedFields = Object.keys(primitiveSuggestedPayload(context));
  return Array.from(new Set([...requiredFields, ...suggestedFields]));
}

export function decisionPayloadFields(item: Decision): DecisionPayloadField[] {
  const context = recordValue(item.context);
  const suggested = primitiveSuggestedPayload(context);
  const required = new Set([
    ...stringListFromValue(context?.requiredFields),
    ...stringListFromValue(context?.fields)
  ]);
  return decisionRequiredFields(item).map((field) => ({
    name: field,
    label: decisionFieldLabel(field),
    required: required.has(field),
    initialValue: suggested[field]
  }));
}

export function decisionInitialPayload(item: Decision) {
  const structuredPayload: Record<string, unknown> = {};
  for (const field of decisionPayloadFields(item)) {
    structuredPayload[field.name] = field.initialValue ?? '';
  }
  return {
    operator: 'console',
    answer: '',
    note: '',
    extraPayloadText: '',
    structuredPayload
  };
}

export function decisionFieldLabel(field: string) {
  return field;
}

export function parseDecisionExtraPayload(text: unknown) {
  const content = typeof text === 'string' ? text.trim() : '';
  if (!content) {
    return {};
  }
  const parsed = JSON.parse(content) as unknown;
  const record = recordValue(parsed);
  if (!record) {
    throw new Error('JSON 高级载荷必须是对象');
  }
  return record;
}

export function normalizeDecisionResolutionPayload(values: Record<string, unknown>) {
  const structuredPayload = recordValue(values.structuredPayload) ?? {};
  const payload: Record<string, unknown> = {
    operator: typeof values.operator === 'string' && values.operator.trim() ? values.operator.trim() : 'console'
  };
  for (const [key, value] of Object.entries(structuredPayload)) {
    if (typeof value === 'string') {
      payload[key] = value.trim();
      continue;
    }
    payload[key] = value;
  }
  if (typeof values.answer === 'string' && values.answer.trim()) {
    payload.answer = values.answer.trim();
  }
  if (typeof values.note === 'string' && values.note.trim()) {
    payload.note = values.note.trim();
  }
  return {
    ...payload,
    ...parseDecisionExtraPayload(values.extraPayloadText)
  };
}

export function outputText(output: Record<string, unknown> | undefined, key: string) {
  const value = output?.[key];
  if (value === undefined || value === null || value === '') {
    return '-';
  }
  return typeof value === 'string' ? value : compactJSON(value);
}

export function eventRecordSummary(item: EventRecord) {
  const payload = item.payload;
  for (const key of ['summary', 'reason', 'error', 'action', 'status', 'state']) {
    const value = outputText(payload, key);
    if (value !== '-') {
      return value;
    }
  }
  return item.subjectId || item.subjectType || '-';
}

export function eventRecordState(item: EventRecord) {
  const state = outputText(item.payload, 'state');
  if (state !== '-') {
    return stateLabel(state);
  }
  const status = outputText(item.payload, 'status');
  return status !== '-' ? status : '-';
}

export function eventRecordSubject(item: EventRecord) {
  return [item.subjectType, item.subjectId].filter(Boolean).join(' / ') || '-';
}

export function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

export const runtimeRawRefFields = [
  { key: 'requestRef', label: 'Request' },
  { key: 'responseRef', label: 'Response' },
  { key: 'streamRef', label: 'Stream' },
  { key: 'usageRef', label: 'Usage' },
  { key: 'repairResponseRef', label: 'Repair' }
];

export function runtimeRawRefActions(value: unknown) {
  const root = asRecord(value);
  const refs = asRecord(root?.llmCallRefs) ?? (runtimeRawRefFields.some((field) => typeof root?.[field.key] === 'string') ? root : undefined);
  if (!refs) {
    return null;
  }
  const actions = runtimeRawRefFields.flatMap((field) => {
    const ref = refs[field.key];
    if (typeof ref !== 'string' || !ref) {
      return [];
    }
    return [{ ...field, ref }];
  });
  if (actions.length === 0) {
    return null;
  }
  return (
    <Space size={4} wrap className="runtime-raw-actions">
      <Typography.Text type="secondary">LLM Raw</Typography.Text>
      {actions.map((action) => (
        <Button
          key={`${action.key}:${action.ref}`}
          size="small"
          type="link"
          href={workorchApi.runtimePayloadURL(action.ref)}
          target="_blank"
        >
          {action.label}
        </Button>
      ))}
    </Space>
  );
}

export function runtimeRawItemSafe(value: string) {
  return btoa(value).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

export function runtimePayloadRef(workspaceId: string, runId: string, collection: string, itemId: string, fileName: string) {
  return `workorch://workspaces/${workspaceId}/runs/${runId}/${collection}/${runtimeRawItemSafe(itemId)}/${fileName}`;
}

export function runtimeDetailActions(workspaceId: string, runId: string, collection: string, itemId: string, actions: Array<{ fileName: string; label: string }>) {
  if (!workspaceId || !runId || !collection || !itemId || actions.length === 0) {
    return null;
  }
  return (
    <Space size={4} wrap className="runtime-raw-actions">
      <Typography.Text type="secondary">Raw</Typography.Text>
      {actions.map((action) => {
        const ref = runtimePayloadRef(workspaceId, runId, collection, itemId, action.fileName);
        return (
          <Button
            key={`${collection}:${itemId}:${action.fileName}`}
            size="small"
            type="link"
            href={workorchApi.runtimePayloadURL(ref)}
            target="_blank"
          >
            {action.label}
          </Button>
        );
      })}
    </Space>
  );
}

export function asStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.map(String).filter(Boolean) : [];
}

export function dodAssessment(snapshot?: RunSnapshot) {
  const resultAssessment = (primaryAcceptedResult(snapshot) ?? primaryResult(snapshot))?.doDAssessment;
  if (resultAssessment) {
    return resultAssessment as Record<string, unknown>;
  }
  const steps = snapshot?.steps ?? [];
  for (let index = steps.length - 1; index >= 0; index -= 1) {
    const assessment = asRecord(steps[index].output?.dodAssessment);
    if (assessment) {
      return assessment;
    }
  }
  return undefined;
}

export function dodAssessmentItems(snapshot?: RunSnapshot) {
  const assessmentItems = dodAssessment(snapshot)?.items;
  return Array.isArray(assessmentItems) ? assessmentItems.map((item, index) => ({
    key: `dod-${index}`,
    item: asRecord(item)?.item ? String(asRecord(item)?.item) : compactJSON(item),
    state: asRecord(item)?.state ? String(asRecord(item)?.state) : '-',
    evidenceRefs: asStringArray(asRecord(item)?.evidenceRefs),
    failureReason: asRecord(item)?.failureReason ? String(asRecord(item)?.failureReason) : ''
  })) : [];
}

export function memoryRefIDs(snapshot?: RunSnapshot) {
	const refs = new Set<string>();
	for (const step of snapshot?.steps ?? []) {
		for (const ref of asStringArray(step.output?.memoryRefs)) {
			refs.add(ref);
    }
	}
	return refs;
}

export function resultTimeValue(result: Result) {
	return result.updatedAt || result.createdAt || result.id;
}

export function primaryResult(snapshot?: RunSnapshot) {
	const results = snapshot?.results ?? [];
	if (!results.length) {
		return undefined;
	}
	return [...results].sort((left, right) => resultTimeValue(right).localeCompare(resultTimeValue(left)))[0];
}

export function primaryAcceptedResult(snapshot?: RunSnapshot) {
	const results = (snapshot?.results ?? []).filter((item) => item.state === 'accepted');
	if (!results.length) {
		return undefined;
	}
	return [...results].sort((left, right) => resultTimeValue(right).localeCompare(resultTimeValue(left)))[0];
}

export function goalOutcomeSummary(snapshot?: RunSnapshot) {
	const result = primaryAcceptedResult(snapshot) ?? primaryResult(snapshot);
	const summary = snapshot?.outcome?.summary || result?.summaryDetail?.goalOutcome || result?.doDAssessment?.summary || result?.summary || snapshot?.run.failureReason;
	return summary ? String(summary) : '尚未形成可确认的目标成果';
}

export function goalOutcomeState(snapshot?: RunSnapshot) {
	if (snapshot?.outcome?.accepted || snapshot?.outcome?.state === 'accepted') {
		return <Tag color="green">结果已接受</Tag>;
	}
	if (snapshot?.outcome?.state === 'failed') {
		return <Tag color="red">验收未通过</Tag>;
	}
	const result = primaryResult(snapshot);
	if (result?.state === 'accepted') {
		return <Tag color="green">结果已接受</Tag>;
	}
	if (result) {
		return stateTag(result.state);
	}
	const assessment = dodAssessment(snapshot);
	if (assessment?.satisfied === true) {
		return <Tag color="green">验收标准（DoD）已满足</Tag>;
  }
  if (assessment?.satisfied === false) {
    return <Tag color="red">验收标准（DoD）未满足</Tag>;
  }
  return snapshot ? stateTag(snapshot.run.state) : <Tag>未选择运行（Run）</Tag>;
}

export function runOutcomeState(snapshot?: RunSnapshot, observability?: RunObservability) {
  return observability?.outcome?.state || snapshot?.outcome?.state || snapshot?.run.state || 'unknown';
}

export function runRawState(snapshot?: RunSnapshot, observability?: RunObservability) {
  return observability?.outcome?.rawState || snapshot?.outcome?.rawState || snapshot?.run.state || 'unknown';
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

export function compactTags(items: string[], limit = 3) {
  if (items.length === 0) {
    return '-';
  }
  return (
    <Space wrap size={[4, 4]} className="workorch-compact-tags">
      {items.slice(0, limit).map((item) => <Tag key={item}>{item}</Tag>)}
      {items.length > limit ? <Tag color="blue">+{items.length - limit}</Tag> : null}
    </Space>
  );
}

export function workspaceSetupDraftStateLabel(state?: string) {
  switch ((state || '').trim()) {
    case 'draft_ready':
      return '草案可用';
    case 'clarifying':
      return '需要澄清';
    case 'confirmed':
      return '已确认';
    case 'expired':
      return '已过期';
    default:
      return '未开始';
  }
}

export function workspaceSetupDraftStateColor(state?: string) {
  switch ((state || '').trim()) {
    case 'draft_ready':
      return 'green';
    case 'clarifying':
      return 'orange';
    case 'confirmed':
      return 'blue';
    case 'expired':
      return 'red';
    default:
      return undefined;
  }
}

export function workspaceSetupDraftConfidenceLabel(value?: string) {
  switch ((value || '').trim()) {
    case 'high':
      return '高';
    case 'medium':
      return '中';
    case 'low':
      return '低';
    default:
      return '-';
  }
}

export function workspaceSetupDraftTextBlock(value?: string) {
  const text = (value || '').trim();
  return (
    <Typography.Paragraph className="workspace-draft-text-block">
      {text || '-'}
    </Typography.Paragraph>
  );
}

export function workspaceSetupDraftDoDList(items?: string[]) {
  const normalized = (items ?? []).map((item) => item.trim()).filter(Boolean);
  if (normalized.length === 0) {
    return <Typography.Text type="secondary">-</Typography.Text>;
  }
  return (
    <ol className="workspace-draft-dod-list">
      {normalized.map((item, index) => (
        <li key={`${index}-${item}`}>
          <Typography.Text>{item}</Typography.Text>
        </li>
      ))}
    </ol>
  );
}

export function workspaceSetupDraftInfoList(items?: string[]) {
  const normalized = (items ?? []).map((item) => item.trim()).filter(Boolean);
  if (normalized.length === 0) {
    return null;
  }
  return (
    <ul className="workspace-draft-info-list">
      {normalized.map((item, index) => (
        <li key={`${index}-${item}`}>
          <Typography.Text>{item}</Typography.Text>
        </li>
      ))}
    </ul>
  );
}

export function hasWorkspaceSetupDraftInfoItems(items?: string[]) {
  return (items ?? []).some((item) => item.trim());
}

export function workspaceSetupDraftReadyForApply(draft?: WorkspaceSetupDraft) {
  return Boolean(draft?.goalText?.trim() && (draft.doDItems?.some((item) => item.trim()) ?? false));
}

export function splitLines(value?: string) {
  return (value || '').split('\n').map((item) => item.trim()).filter(Boolean);
}

export function joinLines(values?: string[]) {
  return (values ?? []).join('\n');
}

export function splitDefinitionList(value?: string) {
  return (value || '').split(/[\n,]/).map((item) => item.trim()).filter(Boolean);
}

export type DoDListContext = {
  indent: number;
  text: string;
};

export function splitDoDRawLines(value?: string) {
  return (value || '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .split('\n')
    .flatMap((line) => {
      if (!shouldSplitDoDInline(line)) {
        return [line];
      }
      return line.split(/[;；]/);
    });
}

export function shouldSplitDoDInline(line: string) {
  if (!/[;；]/.test(line)) {
    return false;
  }
  const trimmed = line.trim();
  if (!trimmed || /^#{1,6}\s+/.test(trimmed)) {
    return false;
  }
  return !stripDoDListMarker(trimmed).isListItem;
}

export function stripDoDListMarker(line: string): { text: string; isListItem: boolean } {
  const trimmed = line.trim();
  if (/^[-*+•]\s+/.test(trimmed)) {
    return { text: stripDoDCheckbox(trimmed.slice(2).trim()), isListItem: true };
  }
  const ordered = trimmed.match(/^\d+[\.)、]\s+/);
  if (ordered) {
    return { text: stripDoDCheckbox(trimmed.slice(ordered[0].length).trim()), isListItem: true };
  }
  if (/^\[\s\]/.test(trimmed) || /^\[[xX]\]/.test(trimmed)) {
    return { text: stripDoDCheckbox(trimmed), isListItem: true };
  }
  return { text: trimmed, isListItem: false };
}

export function stripDoDCheckbox(line: string) {
  return line.replace(/^\[\s\]\s*/, '').replace(/^\[[xX]\]\s*/, '').trim();
}

export function cleanDoDInlineMarkdown(value: string) {
  return value.replace(/\*\*|__|`/g, '').trim().split(/\s+/).filter(Boolean).join(' ');
}

export function stripDoDOrdinal(value: string) {
  return value.replace(/^\d+(?:\.\d+)*(?:[\.)、])?\s+/, '').trim();
}

export function trimDoDTrailingColon(value: string) {
  return value.replace(/[\s:：]+$/, '').trim();
}

export function hasDoDInlineLabel(value: string) {
  const index = value.search(/[:：]/);
  if (index <= 0 || index >= value.length - 1) {
    return false;
  }
  return value.slice(0, index).trim().length > 0 && Array.from(value.slice(0, index).trim()).length <= 40;
}

export function isDoDSameLevelContinuation(isListItem: boolean, value: string) {
  return isListItem && !/[:：]$/.test(value) && !hasDoDInlineLabel(value);
}

export function doDLeadingIndent(line: string) {
  let indent = 0;
  for (const char of line) {
    if (char === ' ') {
      indent += 1;
    } else if (char === '\t') {
      indent += 4;
    } else {
      break;
    }
  }
  return indent;
}

export function doDWithContext(context: string[], item: string) {
  const cleanItem = item.trim();
  if (context.length === 0) {
    return cleanItem;
  }
  const prefix = context.join(' / ');
  if (cleanItem.startsWith(prefix)) {
    return cleanItem;
  }
  return `${prefix}： ${cleanItem}`;
}

export function setupDoDItems(value?: string) {
  const headings: string[] = [];
  const listContexts: DoDListContext[] = [];
  const seen = new Set<string>();
  const items: string[] = [];

  for (const line of splitDoDRawLines(value)) {
    const trimmed = line.trim();
    if (!trimmed || /^[-_*]{3,}$/.test(trimmed)) {
      continue;
    }

    const heading = trimmed.match(/^(#{1,6})\s+(.+)$/);
    if (heading) {
      const level = heading[1].length;
      const text = stripDoDOrdinal(cleanDoDInlineMarkdown(heading[2]));
      if (text) {
        headings[level - 1] = text;
        headings.length = level;
        listContexts.length = 0;
      }
      continue;
    }

    const indent = doDLeadingIndent(line);
    const stripped = stripDoDListMarker(trimmed);
    const content = cleanDoDInlineMarkdown(stripped.text);
    if (!content) {
      continue;
    }
    const keepSameLevel = isDoDSameLevelContinuation(stripped.isListItem, content);
    while (
      listContexts.length > 0 &&
      (listContexts[listContexts.length - 1].indent > indent ||
        (listContexts[listContexts.length - 1].indent === indent && !keepSameLevel))
    ) {
      listContexts.pop();
    }
    if (stripped.isListItem && /[:：]$/.test(content)) {
      const parent = trimDoDTrailingColon(content);
      if (parent) {
        listContexts.push({ indent, text: parent });
      }
      continue;
    }
    const context = [
      ...headings.filter(Boolean),
      ...listContexts.map((item) => item.text).filter(Boolean)
    ];
    const normalized = doDWithContext(context, content);
    if (!normalized || seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    items.push(normalized);
  }

  return items;
}

export function workspaceSetupDraftFromForms(goalText?: string, doDText?: string): WorkspaceSetupDraft {
  return {
    goalText: (goalText || '').trim(),
    doDItems: setupDoDItems(doDText)
  };
}

export function hasWorkspaceSetupDraftContent(draft?: WorkspaceSetupDraft) {
  return Boolean(
    draft &&
    (
      draft.goalText?.trim() ||
      (draft.doDItems?.length ?? 0) > 0 ||
      (draft.assumptions?.length ?? 0) > 0 ||
      (draft.clarifyingQuestions?.length ?? 0) > 0 ||
      (draft.warnings?.length ?? 0) > 0
    )
  );
}

export function workspaceSetupDraftMessageLabel(role?: string) {
  switch ((role || '').trim()) {
    case 'assistant':
      return '草案助手';
    case 'system':
      return '系统';
    default:
      return '用户';
  }
}

export const workspaceSetupSmartDraftStep = 5;

export type SoulBindingFormValues = {
  defaultSoulId?: string;
  bindings?: Record<string, string | undefined>;
};

export type WorkspaceSetupMode = 'manual' | 'smart';

export type WorkspaceSetupFormValues = {
  setupMode?: WorkspaceSetupMode;
  templateId?: string;
  title: string;
  description?: string;
  workingDirectory?: string;
};

export type GoalSetupFormValues = {
  text: string;
};

export type DoDSetupFormValues = {
  items: string;
};

export type WorkspaceDeleteFormValues = {
  purge?: boolean;
};

export function normalizeSoulBindingFormValues(values: SoulBindingFormValues): WorkspaceSoulBindings {
  const defaultSoulId = values.defaultSoulId?.trim() || undefined;
  const bindings = Object.fromEntries(
    Object.entries(values.bindings ?? {})
      .map(([stage, soulId]) => [stage, soulId?.trim() || ''])
      .filter(([, soulId]) => Boolean(soulId))
  );
  return {
    defaultSoulId,
    bindings
  };
}

export function stageBindings(stages: string[], soulId?: string) {
  const value = soulId?.trim();
  if (!value) {
    return {};
  }
  return Object.fromEntries(stages.map((stage) => [stage, value]));
}

export type SoulDefinitionFormValues = {
  id?: string;
  name?: string;
  description?: string;
  businessDomains?: string;
  soulMd?: string;
  allowedSkillRefs?: string;
  allowedMCPServerRefs?: string;
  allowedToolKinds?: string;
};

export function soulDefinitionPayload(values: SoulDefinitionFormValues, soulID: string): Soul {
  return {
    id: soulID,
    name: (values.name || '').trim(),
    description: (values.description || '').trim(),
    businessDomains: splitDefinitionList(values.businessDomains),
    soulMd: (values.soulMd || '').trim(),
    allowedSkillRefs: splitDefinitionList(values.allowedSkillRefs),
    allowedMCPServerRefs: splitDefinitionList(values.allowedMCPServerRefs),
    allowedToolKinds: splitDefinitionList(values.allowedToolKinds)
  };
}

export type WorkspaceTemplateFormValues = {
  id?: string;
  name?: string;
  description?: string;
  goalText?: string;
  doDItems?: string;
  defaultSoulId?: string;
  soulBindings?: Record<string, string | undefined>;
};

export function workspaceTemplatePayload(values: WorkspaceTemplateFormValues, templateID: string): WorkspaceTemplate {
  const bindings = normalizeSoulBindingFormValues({
    defaultSoulId: values.defaultSoulId,
    bindings: values.soulBindings
  });
  return {
    id: templateID,
    name: (values.name || '').trim(),
    description: (values.description || '').trim(),
    goalText: (values.goalText || '').trim(),
    doDItems: setupDoDItems(values.doDItems),
    defaultSoulId: bindings.defaultSoulId,
    soulBindings: bindings.bindings
  };
}

export function metricValue(metrics: string, name: string) {
  const line = metrics.split('\n').find((item) => item.startsWith(`${name} `));
  if (!line) {
    return '-';
  }
  const value = line.trim().split(/\s+/).at(-1);
  return value || '-';
}

export function runtimeTarget(output?: Record<string, unknown>) {
  if (!output) {
    return '-';
  }
  return outputText(output, 'tool') !== '-' ? outputText(output, 'tool') :
    outputText(output, 'uri') !== '-' ? outputText(output, 'uri') :
      outputText(output, 'prompt');
}

export function checkpointTypeLabel(type?: string) {
  const labels: Record<string, string> = {
    run_state: '运行状态',
    runtime_step: '运行步骤',
    graph_state: '执行图',
    validation: '验收校验'
  };
  return type ? labels[type] ?? type : '-';
}

export function runPriority(item: Run) {
  const direct = item.priority;
  if (direct !== undefined && direct !== null) {
    return direct;
  }
  const contextPriority = item.executionContext?.priority;
  return typeof contextPriority === 'number' ? contextPriority : '-';
}

export function numericRunPriority(item: Run) {
  const priority = runPriority(item);
  return typeof priority === 'number' ? priority : undefined;
}

export function llmConfigurationFormValues(configuration: ServiceConfiguration): UpdateLLMConfigurationRequest {
  return {
    defaultProviderId: configuration.llm.defaultProviderId,
    defaultEmbeddingProviderId: configuration.llm.defaultEmbeddingProviderId,
    modelCapabilities: configuration.llm.modelCapabilities ?? [],
    providers: (configuration.llm.providers ?? []).map((item) => ({
      id: item.id,
      provider: item.provider,
      baseUrl: item.baseUrl,
      apiKey: '',
      model: item.model,
      timeout: item.timeout,
      disabled: item.disabled,
      chatCompletions: item.chatCompletions,
      streaming: item.streaming,
      nonChatOperations: item.nonChatOperations ?? [],
      failoverEnabled: item.failoverEnabled,
      failoverProviders: item.failoverProviders ?? [],
      inferenceProfile: item.inferenceProfile,
      contextProfile: item.contextProfile
    }))
  };
}

export function providerDraftFromTemplate(template: LLMProviderTemplate, existing: UpdateLLMProviderConfiguration[], current?: UpdateLLMProviderConfiguration): UpdateLLMProviderConfiguration {
  let id = current?.id?.trim() || template.id;
  const currentID = current?.id?.trim();
  const existingIDs = new Set(existing.map((item) => item.id?.trim()).filter((item) => item && item !== currentID));
  if (existingIDs.has(id)) {
    let index = existing.length + 1;
    id = `${template.provider}-${index}`;
    while (existingIDs.has(id)) {
      index += 1;
      id = `${template.provider}-${index}`;
    }
  }
  return {
    ...current,
    id,
    provider: template.provider,
    baseUrl: template.baseUrl,
    model: template.model,
    timeout: template.timeout || '60s',
    disabled: template.disabled,
    chatCompletions: template.chatCompletions,
    streaming: template.streaming,
    nonChatOperations: template.nonChatOperations ?? [],
    apiKey: current?.apiKey ?? ''
  };
}

export function nextProviderDraft(existing: UpdateLLMProviderConfiguration[], template?: LLMProviderTemplate): UpdateLLMProviderConfiguration {
  if (template) {
    return providerDraftFromTemplate(template, existing);
  }
  let index = existing.length + 1;
  let id = `provider-${index}`;
  const existingIDs = new Set(existing.map((item) => item.id));
  while (existingIDs.has(id)) {
    index += 1;
    id = `provider-${index}`;
  }
  return {
    id,
    provider: 'compatible',
    timeout: '60s',
    disabled: false,
    chatCompletions: true,
    streaming: false,
    nonChatOperations: []
  };
}

export function nextModelCapabilityDraft(existing: LLMModelCapabilityConfiguration[]): LLMModelCapabilityConfiguration {
  let index = existing.length + 1;
  let id = `model-capability-${index}`;
  const existingIDs = new Set(existing.map((item) => item.id));
  while (existingIDs.has(id)) {
    index += 1;
    id = `model-capability-${index}`;
  }
  return {
    id,
    modelPattern: '',
    contextWindowTokens: 128000
  };
}

export function cleanedInferenceProfile(item: UpdateLLMProviderConfiguration) {
  const profile = item.inferenceProfile;
  if (!profile) {
    return undefined;
  }
  return {
    temperature: profile.temperature,
    maxOutputTokens: profile.maxOutputTokens,
    reasoningEffort: profile.reasoningEffort?.trim(),
    thinkingBudgetTokens: profile.thinkingBudgetTokens
  };
}

export function cleanedContextProfile(item: UpdateLLMProviderConfiguration) {
  const profile = item.contextProfile;
  if (!profile) {
    return undefined;
  }
  return {
    profileMode: profile.profileMode?.trim(),
    contextWindowTokens: profile.contextWindowTokens,
    maxInputTokens: profile.maxInputTokens,
    reservedOutputTokens: profile.reservedOutputTokens,
    safetyRatio: profile.safetyRatio,
    contextUtilizationRatio: profile.contextUtilizationRatio,
    bytesHardLimit: profile.bytesHardLimit,
    maxRequestBytes: profile.maxRequestBytes,
    estimatedTokensPerByte: profile.estimatedTokensPerByte,
    inputTokenPrice: profile.inputTokenPrice,
    outputTokenPrice: profile.outputTokenPrice,
    currency: profile.currency?.trim()
  };
}

export type WebSearchProviderFormValues = Omit<Partial<UpdateWebSearchProviderConfiguration>, 'allowDomains' | 'denyDomains'> & {
  allowDomains?: string;
  denyDomains?: string;
};

export function webSearchConfigurationFormValues(configuration: ServiceConfiguration): UpdateWebSearchConfigurationRequest {
  return {
    defaultProviderId: configuration.webSearch.defaultProviderId,
    providers: (configuration.webSearch.providers ?? []).map((item) => ({
      id: item.id,
      provider: item.provider,
      searchEndpoint: item.searchEndpoint,
      fetchEndpoint: item.fetchEndpoint,
      apiKey: '',
      timeout: item.timeout,
      maxResults: item.maxResults,
      maxFetchBytes: item.maxFetchBytes,
      maxRedirects: item.maxRedirects,
      allowDomains: item.allowDomains ?? [],
      denyDomains: item.denyDomains ?? [],
      disabled: item.disabled
    }))
  };
}

export function webSearchProviderFormValues(provider?: UpdateWebSearchProviderConfiguration): WebSearchProviderFormValues {
  return {
    ...provider,
    allowDomains: joinLines(provider?.allowDomains),
    denyDomains: joinLines(provider?.denyDomains)
  };
}

export function webSearchProviderDraftFromTemplate(template: WebSearchProviderTemplate, existing: UpdateWebSearchProviderConfiguration[], current?: UpdateWebSearchProviderConfiguration): UpdateWebSearchProviderConfiguration {
  let id = current?.id?.trim() || template.id;
  const currentID = current?.id?.trim();
  const existingIDs = new Set(existing.map((item) => item.id?.trim()).filter((item) => item && item !== currentID));
  if (existingIDs.has(id)) {
    let index = existing.length + 1;
    id = `${template.provider}-${index}`;
    while (existingIDs.has(id)) {
      index += 1;
      id = `${template.provider}-${index}`;
    }
  }
  return {
    ...current,
    id,
    provider: template.provider,
    searchEndpoint: template.searchEndpoint ?? current?.searchEndpoint,
    fetchEndpoint: template.fetchEndpoint ?? current?.fetchEndpoint,
    timeout: template.timeout || current?.timeout || '20s',
    maxResults: template.maxResults ?? current?.maxResults ?? 8,
    maxFetchBytes: template.maxFetchBytes ?? current?.maxFetchBytes ?? 262144,
    maxRedirects: template.maxRedirects ?? current?.maxRedirects ?? 3,
    disabled: current?.disabled === true,
    apiKey: current?.apiKey ?? ''
  };
}

export function nextWebSearchProviderDraft(existing: UpdateWebSearchProviderConfiguration[], template?: WebSearchProviderTemplate): UpdateWebSearchProviderConfiguration {
  if (template) {
    return webSearchProviderDraftFromTemplate(template, existing);
  }
  let index = existing.length + 1;
  let id = `web-provider-${index}`;
  const existingIDs = new Set(existing.map((item) => item.id));
  while (existingIDs.has(id)) {
    index += 1;
    id = `web-provider-${index}`;
  }
  return {
    id,
    provider: 'http',
    timeout: '20s',
    maxResults: 8,
    maxFetchBytes: 262144,
    maxRedirects: 3,
    disabled: false
  };
}

export function SoulDefinitionManager({ souls, onSoulsChange }: { souls: Soul[]; onSoulsChange: (souls: Soul[]) => void }) {
  const { message, modal } = AntApp.useApp();
  const [form] = Form.useForm<SoulDefinitionFormValues>();
  const [editingSoul, setEditingSoul] = useState<Soul | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [deletingSoulId, setDeletingSoulId] = useState<string | undefined>(undefined);

  const editSoul = (soul?: Soul) => {
    setEditingSoul(soul);
    form.setFieldsValue({
      id: soul?.id || '',
      name: soul?.name || '',
      description: soul?.description || '',
      businessDomains: joinLines(soul?.businessDomains),
      soulMd: soul?.soulMd || '',
      allowedSkillRefs: joinLines(soul?.allowedSkillRefs),
      allowedMCPServerRefs: joinLines(soul?.allowedMCPServerRefs),
      allowedToolKinds: joinLines(soul?.allowedToolKinds)
    });
  };

  const saveSoul = async (values: SoulDefinitionFormValues) => {
    const soulID = (editingSoul?.id || values.id || '').trim();
    if (!soulID) {
      message.error('请填写执行人格 ID');
      return;
    }
    setSaving(true);
    try {
      const payload = soulDefinitionPayload(values, soulID);
      const saved = editingSoul
        ? await workorchApi.updateSoul(soulID, payload)
        : await workorchApi.createSoul(payload);
      const nextSouls = await workorchApi.listSouls();
      onSoulsChange(nextSouls);
      editSoul(saved);
      message.success(editingSoul ? '执行人格定义已更新' : '执行人格定义已创建');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const deleteSoul = (soul: Soul) => {
    modal.confirm({
      title: `删除${modelLabels.soul}`,
      content: `确认删除「${soul.name || soul.id}」？删除后，仍引用该执行人格的工作空间或模板需要重新调整执行配置。`,
      okText: '删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        setDeletingSoulId(soul.id);
        try {
          await workorchApi.deleteSoul(soul.id);
          const nextSouls = await workorchApi.listSouls();
          onSoulsChange(nextSouls);
          if (editingSoul?.id === soul.id) {
            editSoul(undefined);
          }
          message.success('执行人格定义已删除');
        } catch (error) {
          message.error(errorMessage(error));
          throw error;
        } finally {
          setDeletingSoulId(undefined);
        }
      }
    });
  };

  return (
    <ProCard title={modelLabels.soul} className="workorch-section">
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text type="secondary">
          执行人格（Soul）是系统级执行配置，定义人格、阶段策略和输出契约；技能（Skill）、MCP 服务和工具（Tool）权限必须通过结构化字段显式维护。
        </Typography.Text>
        <ProCard split="vertical" bordered={false}>
          <ProCard colSpan={{ xs: 24, lg: 8 }} ghost>
            <List
              size="small"
              dataSource={souls}
              locale={{ emptyText: <Empty description="暂无执行人格定义" /> }}
              renderItem={(item) => (
                <List.Item
                  actions={[
                    <Button key="edit" type="link" size="small" onClick={() => editSoul(item)}>编辑</Button>,
                    <Button key="delete" type="link" danger size="small" loading={deletingSoulId === item.id} onClick={() => deleteSoul(item)}>删除</Button>
                  ]}
                >
                  <List.Item.Meta
                    title={<Typography.Text strong>{item.name || item.id}</Typography.Text>}
                    description={
                      <Space direction="vertical" size={2}>
                        <Typography.Text type="secondary">{item.id}</Typography.Text>
                        {compactTags(item.businessDomains ?? [], 2)}
                      </Space>
                    }
                  />
                </List.Item>
              )}
            />
            <Button style={{ marginTop: 12 }} onClick={() => editSoul(undefined)}>新增执行人格</Button>
          </ProCard>
          <ProCard colSpan={{ xs: 24, lg: 16 }} ghost>
            <Form form={form} layout="vertical" onFinish={(values) => void saveSoul(values)}>
              <Form.Item name="id" label="执行人格 ID" rules={[{ required: !editingSoul, message: '请输入执行人格 ID' }]}>
                <Input disabled={Boolean(editingSoul)} placeholder="例如 soul-engineering-delivery" />
              </Form.Item>
              <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}>
                <Input placeholder="例如 Engineering Delivery" />
              </Form.Item>
              <Form.Item name="description" label="描述">
                <Input.TextArea rows={2} placeholder="说明该执行人格的适用场景" />
              </Form.Item>
              <Form.Item name="businessDomains" label="业务域">
                <Input.TextArea rows={2} placeholder="每行或逗号分隔，例如 engineering, validation" />
              </Form.Item>
              <Form.Item name="soulMd" label="soul.md" rules={[{ required: true, message: '请输入执行人格（Soul）定义内容' }]}>
                <Input.TextArea rows={8} placeholder="# Soul\n\n定义人格、阶段策略和输出契约" />
              </Form.Item>
              <Form.Item name="allowedSkillRefs" label="允许技能（Skill）">
                <Input.TextArea rows={2} placeholder="每行或逗号分隔" />
              </Form.Item>
              <Form.Item name="allowedMCPServerRefs" label="允许 MCP 服务（MCP Server）">
                <Input.TextArea rows={2} placeholder="每行或逗号分隔" />
              </Form.Item>
              <Form.Item name="allowedToolKinds" label="允许工具类型（Tool Kind）">
                <Input.TextArea rows={2} placeholder="每行或逗号分隔，例如 llm/chat" />
              </Form.Item>
              <Space>
                <Button type="primary" htmlType="submit" loading={saving}>
                  {editingSoul ? '保存执行人格' : '创建执行人格'}
                </Button>
                <Button onClick={() => editSoul(undefined)}>清空</Button>
              </Space>
            </Form>
          </ProCard>
        </ProCard>
      </Space>
    </ProCard>
  );
}

export function WorkspaceTemplateManager({ souls, templates, onTemplatesChange }: { souls: Soul[]; templates: WorkspaceTemplate[]; onTemplatesChange: (templates: WorkspaceTemplate[]) => void }) {
  const { message, modal } = AntApp.useApp();
  const [form] = Form.useForm<WorkspaceTemplateFormValues>();
  const [editingTemplate, setEditingTemplate] = useState<WorkspaceTemplate | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [deletingTemplateId, setDeletingTemplateId] = useState<string | undefined>(undefined);
  const stages = defaultSoulBindingStages;
  const soulOptions = souls.map((item) => ({ value: item.id, label: `${item.name || item.id} (${item.id})` }));

  const editTemplate = (template?: WorkspaceTemplate) => {
    setEditingTemplate(template);
    form.setFieldsValue({
      id: template?.id || '',
      name: template?.name || '',
      description: template?.description || '',
      goalText: template?.goalText || '',
      doDItems: joinLines(template?.doDItems),
      defaultSoulId: template?.defaultSoulId,
      soulBindings: template?.soulBindings ?? {}
    });
  };

  const applyDefaultSoulToStages = () => {
    const defaultSoulId = form.getFieldValue('defaultSoulId');
    form.setFieldsValue({ soulBindings: stageBindings(stages, defaultSoulId) });
  };

  const saveTemplate = async (values: WorkspaceTemplateFormValues) => {
    const templateID = (editingTemplate?.id || values.id || '').trim();
    if (!templateID) {
      message.error('请填写模板 ID');
      return;
    }
    setSaving(true);
    try {
      const payload = workspaceTemplatePayload(values, templateID);
      const saved = editingTemplate
        ? await workorchApi.updateWorkspaceTemplate(templateID, payload)
        : await workorchApi.createWorkspaceTemplate(payload);
      const nextTemplates = await workorchApi.listWorkspaceTemplates();
      onTemplatesChange(nextTemplates);
      editTemplate(saved);
      message.success(editingTemplate ? '工作空间模板已更新' : '工作空间模板已创建');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const deleteTemplate = (template: WorkspaceTemplate) => {
    modal.confirm({
      title: '删除工作空间模板',
      content: `确认删除「${template.name || template.id}」？删除模板不会删除已创建的工作空间。`,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: async () => {
        setDeletingTemplateId(template.id);
        try {
          await workorchApi.deleteWorkspaceTemplate(template.id);
          const nextTemplates = await workorchApi.listWorkspaceTemplates();
          onTemplatesChange(nextTemplates);
          if (editingTemplate?.id === template.id) {
            editTemplate(undefined);
          }
          message.success('工作空间模板已删除');
        } catch (error) {
          message.error(errorMessage(error));
        } finally {
          setDeletingTemplateId(undefined);
        }
      }
    });
  };

  return (
    <ProCard title="工作空间模板（Workspace Template）" className="workorch-section">
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text type="secondary">
          模板用于复用新建工作空间时的目标（Goal）、验收标准（DoD）、默认执行人格（Soul）与阶段执行人格绑定；实际工作空间仍通过正式业务 API 创建和写入定义。
        </Typography.Text>
        <ProCard split="vertical" bordered={false}>
          <ProCard colSpan={{ xs: 24, lg: 8 }} ghost>
            <List
              size="small"
              dataSource={templates}
              locale={{ emptyText: <Empty description="暂无工作空间模板" /> }}
              renderItem={(item) => (
                <List.Item actions={[
                  <Button key="edit" type="link" size="small" onClick={() => editTemplate(item)}>编辑</Button>,
                  <Button key="delete" type="link" danger size="small" loading={deletingTemplateId === item.id} onClick={() => deleteTemplate(item)}>删除</Button>
                ]}>
                  <List.Item.Meta
                    title={<Typography.Text strong>{item.name || item.id}</Typography.Text>}
                    description={
                      <Space direction="vertical" size={2}>
                        <Typography.Text type="secondary">{item.id}</Typography.Text>
                        <Space wrap size={[4, 4]}>
                          {item.goalText ? <Tag color="blue">{modelLabels.goal}</Tag> : null}
                          {(item.doDItems?.length ?? 0) > 0 ? <Tag color="green">{modelLabels.dod} {item.doDItems?.length}</Tag> : null}
                          {item.defaultSoulId ? <Tag color="cyan">默认执行人格</Tag> : null}
                        </Space>
                      </Space>
                    }
                  />
                </List.Item>
              )}
            />
            <Button style={{ marginTop: 12 }} onClick={() => editTemplate(undefined)}>新增模板</Button>
          </ProCard>
          <ProCard colSpan={{ xs: 24, lg: 16 }} ghost>
            <Form form={form} layout="vertical" onFinish={(values) => void saveTemplate(values)}>
              <Form.Item name="id" label="模板 ID" rules={[{ required: !editingTemplate, message: '请输入模板 ID' }]}>
                <Input disabled={Boolean(editingTemplate)} placeholder="例如 workspace-template-delivery" />
              </Form.Item>
              <Form.Item name="name" label="模板名称" rules={[{ required: true, message: '请输入模板名称' }]}>
                <Input placeholder="例如 软件交付验证模板" />
              </Form.Item>
              <Form.Item name="description" label="描述">
                <Input.TextArea rows={2} placeholder="说明模板适用场景" />
              </Form.Item>
              <Form.Item name="goalText" label="默认目标（Goal）">
                <Input.TextArea rows={3} placeholder="创建工作空间时自动带入，可在向导中修改" />
              </Form.Item>
              <Form.Item name="doDItems" label="默认验收标准（DoD）">
                <Input.TextArea rows={4} placeholder={'每行一个验收标准\n创建工作空间时自动带入，可在向导中修改'} />
              </Form.Item>
              <Form.Item name="defaultSoulId" label="默认执行人格（Soul）">
                <Select allowClear placeholder="不设置默认执行人格" options={soulOptions} />
              </Form.Item>
              <Space className="workorch-section">
                <Typography.Text type="secondary">阶段执行人格绑定</Typography.Text>
                <Button size="small" onClick={applyDefaultSoulToStages}>默认执行人格应用到全部阶段</Button>
              </Space>
              <div className="soul-binding-grid">
                {stages.map((stage) => (
                  <Form.Item key={stage} name={['soulBindings', stage]} label={soulBindingStageLabel(stage)}>
                    <Select allowClear placeholder="继承默认执行人格" options={soulOptions} />
                  </Form.Item>
                ))}
              </div>
              <Space>
                <Button type="primary" htmlType="submit" loading={saving}>
                  {editingTemplate ? '保存模板' : '创建模板'}
                </Button>
                <Button onClick={() => editTemplate(undefined)}>清空</Button>
              </Space>
            </Form>
          </ProCard>
        </ProCard>
      </Space>
    </ProCard>
  );
}

export type ControlPlaneProps = {
  selectedWorkspaceId?: string;
  onOpenWorkspace: (workspaceId: string) => void;
};

export type WorkspaceBoardItem = {
  snapshot: WorkspaceSnapshot;
  goals: Goal[];
  dods: DoD[];
};

export function readyDoDs(dods: DoD[]) {
  return dods.filter((item) => item.state === 'ready' || item.state === 'satisfied');
}

export function workspaceBoardActivationRequest(item: WorkspaceBoardItem): WorkspaceExecutionActivationRequest | undefined {
  const projectedRun = workspaceProjectionRun(item.snapshot);
  if (projectedRun && isTerminalRunState(projectedRun.state) && projectedRun.goalId && projectedRun.doDId) {
    return { goalId: projectedRun.goalId, doDId: projectedRun.doDId };
  }

  const availableDoDs = readyDoDs(item.dods);
  if (item.goals.length === 1 && availableDoDs.length === 1) {
    return { goalId: item.goals[0].id, doDId: availableDoDs[0].id };
  }

  return undefined;
}

export function workspaceBoardActivationHint(item: WorkspaceBoardItem) {
  if (item.goals.length === 0) {
    return `当前工作空间尚未定义目标，请进入工作空间补齐${modelLabels.goal}后再启动执行`;
  }
  const availableDoDs = readyDoDs(item.dods);
  if (availableDoDs.length === 0) {
    return `当前工作空间尚无可用验收标准，请进入工作空间补齐${modelLabels.dod}后再启动执行`;
  }
  if (item.goals.length > 1 || availableDoDs.length > 1) {
    return `当前工作空间存在多个目标或验收标准，请进入工作空间选择明确的${modelLabels.goal}/${modelLabels.dod}后再启动执行`;
  }
  return '当前工作空间无法确定启动上下文，请进入工作空间核对后再启动执行';
}





export function ConsoleApp() {
  const [view, setView] = useState<ViewKey>('board');
  const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | undefined>(undefined);
  const renderContent = () => {
    switch (view) {
      case 'board':
        return <WorkspaceBoardPage onOpenWorkspace={(workspaceId) => {
          setSelectedWorkspaceId(workspaceId);
          setView('workspaceDetail');
        }} />;
      case 'governance':
        return <GovernanceQueuePage />;
      case 'workspaceDetail':
        return <WorkspaceDetailPage selectedWorkspaceId={selectedWorkspaceId} />;
      case 'runtimeControl':
        return <RuntimeControlPage />;
      case 'configuration':
        return <ConfigurationPage />;
      case 'service':
        return <ServicePage />;
      default:
        return <WorkspaceBoardPage onOpenWorkspace={(workspaceId) => {
          setSelectedWorkspaceId(workspaceId);
          setView('workspaceDetail');
        }} />;
    }
  };

  return (
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#1668dc',
          borderRadius: 8
        }
      }}
    >
      <AntApp>
        <ProLayout
          title="workorch 控制台"
          logo={false}
          route={{ path: '/', routes }}
          location={{ pathname: `/${view}` }}
          menuItemRender={(item, dom) => (
            <a
              onClick={(event) => {
                event.preventDefault();
                setView((item.path?.replace('/', '') as ViewKey) || 'board');
              }}
            >
              {dom}
            </a>
          )}
          layout="mix"
          contentWidth="Fluid"
        >
          {renderContent()}
        </ProLayout>
      </AntApp>
    </ConfigProvider>
  );
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConsoleApp />
  </React.StrictMode>
);
