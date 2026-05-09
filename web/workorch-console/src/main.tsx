import React, { useEffect, useRef, useState } from 'react';
import ReactDOM from 'react-dom/client';
import {
  CheckCircleOutlined,
  ClusterOutlined,
  DashboardOutlined,
  DownOutlined,
  ExceptionOutlined,
  ProfileOutlined,
  SettingOutlined
} from '@ant-design/icons';
import { PageContainer, ProCard, ProLayout, ProTable } from '@ant-design/pro-components';
import { App as AntApp, Badge, Button, Checkbox, ConfigProvider, Descriptions, Dropdown, Empty, Form, Input, InputNumber, List, Modal, Select, Space, Steps, Tabs, Tag, Typography, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import type { Approval, Artifact, ContextBundle, ContextCompressionCandidate, ContextEvidence, ContextSection, Decision, DoD, ExecutionSection, Goal, LLMContextBudgetTrace, LLMProviderTemplate, LLMUsageProviderModelBreakdown, LLMUsageSummary, LLMUsageTrace, MCPCapability, MCPPrompt, MCPResource, MCPServer, MCPTool, MemoryItem, MemoryRecallTrace, Patch, ProgressBlocker, ProgressItem, RepairLoop, Result, Run, RunObservability, RunSnapshot, RuntimeCapability, RuntimeCheckpoint, RuntimeTimelineItem, ServiceConfiguration, Skill, Soul, StepView, TaskDecomposition, TaskDecompositionNode, TaskProgress, ToolCall, UpdateLLMConfigurationRequest, UpdateLLMProviderConfiguration, UpdateServerConfigurationRequest, UpdateWebSearchConfigurationRequest, UpdateWebSearchProviderConfiguration, ValidationResult, WebSearchProviderTemplate, WebSourceTrace, Workspace, WorkspaceExecutionActivationRequest, WorkspaceSnapshot, WorkspaceSoulBindings, WorkspaceTemplate } from './types';
import { createWorkorchEventStream, workorchApi } from './api';
import './styles.css';

type ViewKey = 'board' | 'workspaceDetail' | 'governance' | 'runtimeControl' | 'configuration' | 'service';

const routes = [
  { path: '/board', name: '业务总览', icon: <DashboardOutlined /> },
  { path: '/workspaceDetail', name: '工作空间', icon: <ProfileOutlined /> },
  { path: '/governance', name: '治理队列', icon: <ExceptionOutlined /> },
  { path: '/runtimeControl', name: '运行控制', icon: <ClusterOutlined /> },
  { path: '/configuration', name: '系统配置', icon: <SettingOutlined /> },
  { path: '/service', name: '服务状态', icon: <CheckCircleOutlined /> }
];

const stateLabels: Record<string, string> = {
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

const progressKindLabels: Record<string, string> = {
  step: '步骤',
  tool_call: '工具调用',
  patch: '补丁（Patch）',
  validation_result: '校验结果',
  result: '结果（Result）',
  artifact: '交付物（Artifact）',
  decision: '决策工单',
  approval: '审批工单'
};

const recoveryModeLabels: Record<string, string> = {
  idle: '待启动',
  continue: '可继续',
  wait_decision: '等待决策',
  wait_approval: '等待审批',
  inspect_failure: '排查失败',
  restart: '可重新启动',
  none: '无需恢复'
};

const recoveryActionLabels: Record<string, string> = {
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

const repairLoopStateLabels: Record<string, string> = {
  inactive: '未进入修复',
  repairing: '自动修复中',
  awaiting_decision: '等待决策',
  awaiting_approval: '等待审批',
  completed_after_repair: '修复后完成',
  escalated_decision: '已升级决策',
  terminated: '修复终止'
};

const defaultSoulBindingStages = ['planning', 'execution', 'decision', 'approval', 'validation', 'artifact'];

const soulBindingStageLabels: Record<string, string> = {
  planning: '规划',
  execution: '执行',
  decision: '决策',
  approval: '审批',
  validation: '验收',
  artifact: '交付物'
};

const modelLabels = {
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

function stateLabel(state?: string) {
  if (!state) {
    return '未知';
  }
  return stateLabels[state] ? `${stateLabels[state]} (${state})` : state;
}

function soulBindingStageLabel(stage: string) {
  return soulBindingStageLabels[stage] ? `${soulBindingStageLabels[stage]} (${stage})` : stage;
}

function progressKindLabel(kind?: string) {
  if (!kind) {
    return '未知';
  }
  return progressKindLabels[kind] ?? kind;
}

function recoveryModeLabel(mode?: string) {
  if (!mode) {
    return '未知';
  }
  return recoveryModeLabels[mode] ? `${recoveryModeLabels[mode]} (${mode})` : mode;
}

function recoveryActionLabel(action?: string) {
  if (!action) {
    return '-';
  }
  return recoveryActionLabels[action] ?? action;
}

function repairLoopStateLabel(state?: string) {
  if (!state) {
    return '未知';
  }
  return repairLoopStateLabels[state] ? `${repairLoopStateLabels[state]} (${state})` : state;
}

function stateTag(state?: string) {
  const value = state || 'unknown';
  const color = value.includes('failed') || value.includes('reject') ? 'red' : value.includes('block') || value.includes('pending') ? 'gold' : 'green';
  return <Tag color={color}>{stateLabel(value)}</Tag>;
}

function shortId(value?: string, keep = 10) {
  if (!value) {
    return '-';
  }
  return value.length > keep ? `${value.slice(0, keep)}…` : value;
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : '操作失败';
}

function isNotFoundError(error: unknown) {
  return /(^|\s)404(\s|$)|not found|不存在/i.test(errorMessage(error));
}

function tableResult<T>(data: T[]) {
  return {
    data,
    success: true,
    total: data.length
  };
}

function pendingCount(items?: Array<{ state: string }>) {
  return (items ?? []).filter((item) => item.state === 'open' || item.state === 'pending').length;
}

function runStateSummary(runs?: Run[]) {
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

function workspaceProjectionRun(snapshot?: WorkspaceSnapshot) {
  if (!snapshot) {
    return undefined;
  }
  const activeRun = snapshot.runs?.find((item) => item.id === snapshot.workspace.activeRunId);
  if (activeRun) {
    return activeRun;
  }
  const progressRun = snapshot.runs?.find((item) => item.id === snapshot.taskProgress?.runId);
  if (progressRun) {
    return progressRun;
  }
  return runStateSummary(snapshot.runs).latest;
}

function isBlockedRunState(state?: string) {
  return state === 'blocked_on_decision' || state === 'blocked_on_approval';
}

function isCompletedRunState(state?: string) {
  return state === 'completed';
}

function isFailedRunState(state?: string) {
  return state === 'failed' || state === 'cancelled' || state === 'superseded';
}

function isTerminalRunState(state?: string) {
  return isCompletedRunState(state) || isFailedRunState(state);
}

function activeOrLatestRun(snapshot?: WorkspaceSnapshot) {
  return workspaceProjectionRun(snapshot);
}

function workspaceProcessItems(snapshot?: WorkspaceSnapshot, goals?: Goal[], dods?: DoD[]) {
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

function runtimeSummaryTags(runs?: Run[]) {
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

function taskProgressStepStatus(state?: string) {
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

function taskProgressCard(progress?: TaskProgress, repairLoopOverride?: RepairLoop) {
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
          { key: 'repairNextAction', label: '修复下一步', children: repairLoop?.nextAction || '-' },
          { key: 'repairEscalation', label: '升级信息', children: repairEscalation },
          { key: 'repairEvidence', label: '修复证据', children: compactTags(repairLoop?.evidenceRefs ?? [], 4) }
        ]}
      />
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

function taskDecompositionCard(items?: TaskDecomposition[]) {
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

function executionSectionsCard(items?: ExecutionSection[]) {
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

type MemoryRecallGroup = {
  key: string;
  source: string;
  stage: string;
  status: string;
  traces: MemoryRecallTrace[];
  memoryIds: string[];
};

function memoryRecallGroups(items: MemoryRecallTrace[]) {
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

function memoryRecallQueryPanel(item: MemoryRecallTrace) {
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

function memoryRecallCard(items?: MemoryRecallTrace[]) {
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

function webSourceCard(items?: WebSourceTrace[]) {
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

function numberText(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
}

function costText(value?: number, currency?: string) {
  if (!value || value <= 0) {
    return '-';
  }
  const formatted = new Intl.NumberFormat('zh-CN', {
    minimumFractionDigits: 0,
    maximumFractionDigits: value < 0.01 ? 8 : 4
  }).format(value);
  return currency ? `${formatted} ${currency}` : formatted;
}

function llmUsageCostSummary(summary?: LLMUsageSummary) {
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

function llmUsageProviderModelKey(item: LLMUsageProviderModelBreakdown, index?: number) {
  return [item.providerId, item.provider, item.model, item.currency, index].filter(Boolean).join('|');
}

function llmUsageCard(summary?: LLMUsageSummary, traces?: LLMUsageTrace[]) {
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

function llmContextBudgetCard(traces?: LLMContextBudgetTrace[]) {
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

function contextBundleCard(bundle?: ContextBundle) {
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

function skillScopeLabel(skill: Skill) {
  if (skill.source === 'skill-hub') {
    return skill.scope === 'workspace' ? '工作空间 SkillHub 技能' : '全局 SkillHub 技能';
  }
  return '内置技能';
}

function skillDisplayLabel(skill: Skill) {
  const name = skill.name || skill.sourceSkillId || skill.id;
  const version = skill.version ? `@${skill.version}` : '';
  const status = skill.status ? ` / ${skill.status}` : '';
  return `${name}${version} / ${skillScopeLabel(skill)}${status}`;
}

type MetricItem = {
  key: string;
  label: string;
  value: React.ReactNode;
  color?: string;
};

function metricGrid(items: MetricItem[]) {
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

function runtimeCapabilityTable(items: RuntimeCapability[] | undefined, emptyText: string) {
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

function capabilityKindTag(kind?: string) {
  if (kind === 'tool') return <Tag color="blue">{modelLabels.mcpTool}</Tag>;
  if (kind === 'resource') return <Tag color="cyan">{modelLabels.mcpResource}</Tag>;
  if (kind === 'prompt') return <Tag color="geekblue">{modelLabels.mcpPrompt}</Tag>;
  return kind ? <Tag>{kind}</Tag> : '-';
}

function evidenceCount(snapshot?: RunSnapshot) {
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

function timelineLabel(item: RuntimeTimelineItem) {
  return item.topic || item.type || item.source;
}

function compactJSON(value: unknown) {
  if (value === undefined || value === null) {
    return '-';
  }
  return JSON.stringify(value, null, 2);
}

function secondaryCellText(value: string) {
  return <Typography.Text type="secondary">{value}</Typography.Text>;
}

function timelineStreamContent(item: RuntimeTimelineItem) {
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

function timelineOccurredAtContent(item: RuntimeTimelineItem) {
  return item.occurredAt || secondaryCellText('未记录');
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

type DecisionPayloadField = {
  name: string;
  label: string;
  required: boolean;
  initialValue?: unknown;
};

function stringListFromValue(value: unknown) {
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

function primitiveSuggestedPayload(context: Record<string, unknown> | undefined) {
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

function decisionRequiredFields(item: Decision) {
  const context = recordValue(item.context);
  const requiredFields = [
    ...stringListFromValue(context?.requiredFields),
    ...stringListFromValue(context?.fields)
  ];
  const suggestedFields = Object.keys(primitiveSuggestedPayload(context));
  return Array.from(new Set([...requiredFields, ...suggestedFields]));
}

function decisionPayloadFields(item: Decision): DecisionPayloadField[] {
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

function decisionInitialPayload(item: Decision) {
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

function decisionFieldLabel(field: string) {
  return field;
}

function parseDecisionExtraPayload(text: unknown) {
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

function normalizeDecisionResolutionPayload(values: Record<string, unknown>) {
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

function outputText(output: Record<string, unknown> | undefined, key: string) {
  const value = output?.[key];
  if (value === undefined || value === null || value === '') {
    return '-';
  }
  return typeof value === 'string' ? value : compactJSON(value);
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function asStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.map(String).filter(Boolean) : [];
}

function dodAssessment(snapshot?: RunSnapshot) {
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

function dodAssessmentItems(snapshot?: RunSnapshot) {
  const assessmentItems = dodAssessment(snapshot)?.items;
  return Array.isArray(assessmentItems) ? assessmentItems.map((item, index) => ({
    key: `dod-${index}`,
    item: asRecord(item)?.item ? String(asRecord(item)?.item) : compactJSON(item),
    state: asRecord(item)?.state ? String(asRecord(item)?.state) : '-',
    evidenceRefs: asStringArray(asRecord(item)?.evidenceRefs),
    failureReason: asRecord(item)?.failureReason ? String(asRecord(item)?.failureReason) : ''
  })) : [];
}

function memoryRefIDs(snapshot?: RunSnapshot) {
	const refs = new Set<string>();
	for (const step of snapshot?.steps ?? []) {
		for (const ref of asStringArray(step.output?.memoryRefs)) {
			refs.add(ref);
    }
	}
	return refs;
}

function resultTimeValue(result: Result) {
	return result.updatedAt || result.createdAt || result.id;
}

function primaryResult(snapshot?: RunSnapshot) {
	const results = snapshot?.results ?? [];
	if (!results.length) {
		return undefined;
	}
	return [...results].sort((left, right) => resultTimeValue(right).localeCompare(resultTimeValue(left)))[0];
}

function primaryAcceptedResult(snapshot?: RunSnapshot) {
	const results = (snapshot?.results ?? []).filter((item) => item.state === 'accepted');
	if (!results.length) {
		return undefined;
	}
	return [...results].sort((left, right) => resultTimeValue(right).localeCompare(resultTimeValue(left)))[0];
}

function goalOutcomeSummary(snapshot?: RunSnapshot) {
	const result = primaryAcceptedResult(snapshot) ?? primaryResult(snapshot);
	const summary = snapshot?.outcome?.summary || result?.summaryDetail?.goalOutcome || result?.doDAssessment?.summary || result?.summary || snapshot?.run.failureReason;
	return summary ? String(summary) : '尚未形成可确认的目标成果';
}

function goalOutcomeState(snapshot?: RunSnapshot) {
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

function runOutcomeState(snapshot?: RunSnapshot, observability?: RunObservability) {
  return observability?.outcome?.state || snapshot?.outcome?.state || snapshot?.run.state || 'unknown';
}

function runRawState(snapshot?: RunSnapshot, observability?: RunObservability) {
  return observability?.outcome?.rawState || snapshot?.outcome?.rawState || snapshot?.run.state || 'unknown';
}

function boundedParagraph(value?: React.ReactNode, options?: { rows?: number; copyText?: string; className?: string }) {
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

function compactTags(items: string[], limit = 3) {
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

function splitLines(value?: string) {
  return (value || '').split('\n').map((item) => item.trim()).filter(Boolean);
}

function joinLines(values?: string[]) {
  return (values ?? []).join('\n');
}

function splitDefinitionList(value?: string) {
  return (value || '').split(/[\n,]/).map((item) => item.trim()).filter(Boolean);
}

type SoulBindingFormValues = {
  defaultSoulId?: string;
  bindings?: Record<string, string | undefined>;
};

type WorkspaceSetupFormValues = {
  templateId?: string;
  title: string;
  description?: string;
  workingDirectory?: string;
};

type GoalSetupFormValues = {
  text: string;
};

type DoDSetupFormValues = {
  items: string;
};

type WorkspaceDeleteFormValues = {
  purge?: boolean;
};

function normalizeSoulBindingFormValues(values: SoulBindingFormValues): WorkspaceSoulBindings {
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

function stageBindings(stages: string[], soulId?: string) {
  const value = soulId?.trim();
  if (!value) {
    return {};
  }
  return Object.fromEntries(stages.map((stage) => [stage, value]));
}

function setupDoDItems(value?: string) {
  return (value || '').split('\n').map((item) => item.trim()).filter(Boolean);
}

type SoulDefinitionFormValues = {
  id?: string;
  name?: string;
  description?: string;
  businessDomains?: string;
  soulMd?: string;
  allowedSkillRefs?: string;
  allowedMCPServerRefs?: string;
  allowedToolKinds?: string;
};

function soulDefinitionPayload(values: SoulDefinitionFormValues, soulID: string): Soul {
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

type WorkspaceTemplateFormValues = {
  id?: string;
  name?: string;
  description?: string;
  goalText?: string;
  doDItems?: string;
  defaultSoulId?: string;
  soulBindings?: Record<string, string | undefined>;
};

function workspaceTemplatePayload(values: WorkspaceTemplateFormValues, templateID: string): WorkspaceTemplate {
  const bindings = normalizeSoulBindingFormValues({
    defaultSoulId: values.defaultSoulId,
    bindings: values.soulBindings
  });
  return {
    id: templateID,
    name: (values.name || '').trim(),
    description: (values.description || '').trim(),
    goalText: (values.goalText || '').trim(),
    doDItems: splitLines(values.doDItems),
    defaultSoulId: bindings.defaultSoulId,
    soulBindings: bindings.bindings
  };
}

function metricValue(metrics: string, name: string) {
  const line = metrics.split('\n').find((item) => item.startsWith(`${name} `));
  if (!line) {
    return '-';
  }
  const value = line.trim().split(/\s+/).at(-1);
  return value || '-';
}

function runtimeTarget(output?: Record<string, unknown>) {
  if (!output) {
    return '-';
  }
  return outputText(output, 'tool') !== '-' ? outputText(output, 'tool') :
    outputText(output, 'uri') !== '-' ? outputText(output, 'uri') :
      outputText(output, 'prompt');
}

function checkpointTypeLabel(type?: string) {
  const labels: Record<string, string> = {
    run_state: '运行状态',
    runtime_step: '运行步骤',
    graph_state: '执行图',
    validation: '验收校验'
  };
  return type ? labels[type] ?? type : '-';
}

function runPriority(item: Run) {
  const direct = item.priority;
  if (direct !== undefined && direct !== null) {
    return direct;
  }
  const contextPriority = item.executionContext?.priority;
  return typeof contextPriority === 'number' ? contextPriority : '-';
}

function numericRunPriority(item: Run) {
  const priority = runPriority(item);
  return typeof priority === 'number' ? priority : undefined;
}

function llmConfigurationFormValues(configuration: ServiceConfiguration): UpdateLLMConfigurationRequest {
  return {
    defaultProviderId: configuration.llm.defaultProviderId,
    defaultEmbeddingProviderId: configuration.llm.defaultEmbeddingProviderId,
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
      nonChatOperations: item.nonChatOperations ?? []
    }))
  };
}

function providerDraftFromTemplate(template: LLMProviderTemplate, existing: UpdateLLMProviderConfiguration[], current?: UpdateLLMProviderConfiguration): UpdateLLMProviderConfiguration {
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

function nextProviderDraft(existing: UpdateLLMProviderConfiguration[], template?: LLMProviderTemplate): UpdateLLMProviderConfiguration {
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

type WebSearchProviderFormValues = Omit<Partial<UpdateWebSearchProviderConfiguration>, 'allowDomains' | 'denyDomains'> & {
  allowDomains?: string;
  denyDomains?: string;
};

function webSearchConfigurationFormValues(configuration: ServiceConfiguration): UpdateWebSearchConfigurationRequest {
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

function webSearchProviderFormValues(provider?: UpdateWebSearchProviderConfiguration): WebSearchProviderFormValues {
  return {
    ...provider,
    allowDomains: joinLines(provider?.allowDomains),
    denyDomains: joinLines(provider?.denyDomains)
  };
}

function webSearchProviderDraftFromTemplate(template: WebSearchProviderTemplate, existing: UpdateWebSearchProviderConfiguration[], current?: UpdateWebSearchProviderConfiguration): UpdateWebSearchProviderConfiguration {
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

function nextWebSearchProviderDraft(existing: UpdateWebSearchProviderConfiguration[], template?: WebSearchProviderTemplate): UpdateWebSearchProviderConfiguration {
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

function SoulDefinitionManager({ souls, onSoulsChange }: { souls: Soul[]; onSoulsChange: (souls: Soul[]) => void }) {
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

function WorkspaceTemplateManager({ souls, templates, onTemplatesChange }: { souls: Soul[]; templates: WorkspaceTemplate[]; onTemplatesChange: (templates: WorkspaceTemplate[]) => void }) {
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

type ControlPlaneProps = {
  selectedWorkspaceId?: string;
  onOpenWorkspace: (workspaceId: string) => void;
};

type WorkspaceBoardItem = {
  snapshot: WorkspaceSnapshot;
  goals: Goal[];
  dods: DoD[];
};

function readyDoDs(dods: DoD[]) {
  return dods.filter((item) => item.state === 'ready' || item.state === 'satisfied');
}

function workspaceBoardActivationRequest(item: WorkspaceBoardItem): WorkspaceExecutionActivationRequest | undefined {
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

function workspaceBoardActivationHint(item: WorkspaceBoardItem) {
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

function WorkspaceBoardPage({ onOpenWorkspace }: ControlPlaneProps) {
  const { message } = AntApp.useApp();
  const [boardItems, setBoardItems] = useState<WorkspaceBoardItem[]>([]);
  const [loading, setLoading] = useState(false);

  const loadBoard = async () => {
    setLoading(true);
    try {
      const workspaces = await workorchApi.listWorkspaces();
      const nextBoardItems = await Promise.all(workspaces.map(async (item) => {
        const [snapshot, goals, dods] = await Promise.all([
          workorchApi.readWorkspaceSnapshot(item.id),
          workorchApi.listGoals(item.id),
          workorchApi.listDoDs(item.id)
        ]);
        return { snapshot, goals, dods };
      }));
      setBoardItems(nextBoardItems);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadBoard();
  }, []);

  const trackWorkspaceActivationInBackground = (workspaceId: string, request: WorkspaceExecutionActivationRequest) => {
    void workorchApi.activateWorkspaceExecution(workspaceId, request)
      .then(() => {
        message.success('工作空间执行已在后台启动');
        void loadBoard();
      })
      .catch((error: unknown) => {
        message.error(`工作空间执行后台启动失败：${errorMessage(error)}`);
        void loadBoard();
      });
  };

  const activateWorkspaceExecution = async (item: WorkspaceBoardItem) => {
    const request = workspaceBoardActivationRequest(item);
    if (!request) {
      message.warning(workspaceBoardActivationHint(item));
      return;
    }
    message.info('工作空间执行已提交，正在后台启动');
    trackWorkspaceActivationInBackground(item.snapshot.workspace.id, request);
  };

  return (
    <PageContainer
      title="业务总览"
      subTitle="从工作空间视角查看目标、运行、治理阻塞和交付状态"
      extra={<Button onClick={() => void loadBoard()} loading={loading}>刷新</Button>}
    >
      {boardItems.length === 0 && !loading ? (
        <Empty description="暂无工作空间" />
      ) : (
        <ProCard gutter={16} wrap loading={loading}>
          {boardItems.map((item) => {
            const { snapshot, goals, dods } = item;
            const activeRun = snapshot.runs?.find((item) => item.id === snapshot.workspace.activeRunId);
            const currentRun = workspaceProjectionRun(snapshot);
            const runSummary = runStateSummary(snapshot.runs);
            const pendingDecisions = pendingCount(snapshot.decisions);
            const pendingApprovals = pendingCount(snapshot.approvals);
            const canStartRun = !currentRun || isTerminalRunState(currentRun.state);
            return (
              <ProCard
                key={snapshot.workspace.id}
                title={snapshot.workspace.title || snapshot.workspace.id}
                subTitle={snapshot.workspace.id}
                colSpan={{ xs: 24, md: 12, xl: 8 }}
                bordered
                extra={stateTag(snapshot.outcome?.state ?? snapshot.workspace.state)}
              >
                <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                  <div className="workorch-card-summary">
                    <div>
                      <Typography.Text type="secondary">最近运行（Run）</Typography.Text>
                      <div>{runSummary.latest ? <Space>{shortId(runSummary.latest.id)}{stateTag(runSummary.latest.state)}</Space> : '-'}</div>
                    </div>
                    <div>
                      <Typography.Text type="secondary">治理阻塞</Typography.Text>
                      <div><Badge count={pendingDecisions + pendingApprovals} showZero color={pendingDecisions + pendingApprovals > 0 ? '#faad14' : '#52c41a'} /></div>
                    </div>
                  </div>
                  {metricGrid([
                    { key: 'goal', label: modelLabels.goal, value: goals.length },
                    { key: 'dod', label: modelLabels.dod, value: dods.length },
                    { key: 'run', label: modelLabels.run, value: runSummary.total },
                    { key: 'completed', label: '完成', value: runSummary.completed, color: '#389e0d' },
                    { key: 'failed', label: '失败', value: runSummary.failed, color: '#cf1322' }
                  ])}
                  <Descriptions
                    column={1}
                    size="small"
                    items={[
                      { key: 'workingDirectory', label: '工作目录', children: boundedParagraph(snapshot.workspace.workingDirectory, { rows: 1, copyText: snapshot.workspace.workingDirectory }) },
                      { key: 'activeRun', label: '活跃运行', children: activeRun ? `${activeRun.id} / ${stateLabel(activeRun.state)}` : '-' },
                      { key: 'outcome', label: '业务结果', children: snapshot.outcome?.runId ? `${snapshot.outcome.runId} / ${stateLabel(snapshot.outcome.state)}` : stateLabel(snapshot.outcome?.state ?? snapshot.workspace.state) },
                      { key: 'runtimeSummary', label: '运行摘要', children: runtimeSummaryTags(snapshot.runs) }
                    ]}
                  />
                </Space>
                <Steps
                  size="small"
                  className="workorch-section"
                  items={workspaceProcessItems(snapshot, goals, dods)}
                />
                <Space className="workorch-section" wrap>
                  <Button type="primary" onClick={() => onOpenWorkspace(snapshot.workspace.id)}>进入工作空间</Button>
                  {canStartRun ? <Button onClick={() => void activateWorkspaceExecution(item)}>{currentRun ? '重新启动执行' : '启动执行'}</Button> : null}
                  {currentRun && !isTerminalRunState(currentRun.state) ? <Button onClick={() => void workorchApi.resumeRun(currentRun.id).then(loadBoard).catch((error: unknown) => message.error(errorMessage(error)))}>继续运行</Button> : null}
                </Space>
              </ProCard>
            );
          })}
        </ProCard>
      )}
    </PageContainer>
  );
}

function DecisionQueuePage() {
  const { message } = AntApp.useApp();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [selectedDecisionKeys, setSelectedDecisionKeys] = useState<React.Key[]>([]);
  const [decisionTarget, setDecisionTarget] = useState<{ item: Decision; resolutionType: string } | undefined>(undefined);
  const [decisionSubmitting, setDecisionSubmitting] = useState(false);
  const [decisionForm] = Form.useForm();

  const resolve = async (item: Decision, resolutionType: string, payload?: Record<string, unknown>) => {
    try {
      await workorchApi.resolveDecision(item.id, resolutionType, payload);
      message.success(`决策已按 ${resolutionType} 处理，${modelLabels.run} 已由服务端转入后台继续`);
      actionRef.current?.reload();
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const openDecisionResolution = (item: Decision, resolutionType: string) => {
    decisionForm.resetFields();
    if (resolutionType === 'provide_missing_info') {
      decisionForm.setFieldsValue(decisionInitialPayload(item));
    } else {
      decisionForm.setFieldsValue({ operator: 'console' });
    }
    setDecisionTarget({ item, resolutionType });
  };

  const submitDecisionResolution = async () => {
    if (!decisionTarget) {
      return;
    }
    setDecisionSubmitting(true);
    try {
      const values = await decisionForm.validateFields();
      const payload = normalizeDecisionResolutionPayload(values);
      const target = decisionTarget;
      await workorchApi.resolveDecision(target.item.id, target.resolutionType, payload);
      setDecisionTarget(undefined);
      decisionForm.resetFields();
      message.success(`决策已按 ${target.resolutionType} 处理，${modelLabels.run} 已由服务端转入后台继续`);
      actionRef.current?.reload();
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setDecisionSubmitting(false);
    }
  };

  const batchResolve = async (resolutionType: string) => {
    const ticketIds = selectedDecisionKeys.map(String);
    if (ticketIds.length === 0) {
      message.warning('请先选择待处理决策工单');
      return;
    }
    try {
      await workorchApi.batchResolveDecisions(ticketIds, resolutionType, { operator: 'console', mode: 'batch' });
      message.success(`${ticketIds.length} 个决策工单已按 ${resolutionType} 处理，相关${modelLabels.run}已由服务端转入后台继续`);
      setSelectedDecisionKeys([]);
      actionRef.current?.reload();
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const columns: ProColumns<Decision>[] = [
    { title: '工单 ID', dataIndex: 'id', copyable: true },
    { title: '工作空间', dataIndex: 'workspaceId', copyable: true },
    { title: '运行', dataIndex: 'runId', copyable: true },
    { title: '类型', dataIndex: 'decisionKind' },
    { title: '问题', dataIndex: 'question', ellipsis: true, render: (_, item) => boundedParagraph(item.question, { rows: 2 }) },
    { title: '原因', dataIndex: 'reason', ellipsis: true },
    { title: '自动治理', dataIndex: 'autoGovernance', render: (_, item) => item.autoGovernance ? stateTag(`${item.autoGovernance.status}:${item.autoGovernance.action}`) : '-' },
    { title: '解决方式', dataIndex: 'resolutionType', width: 140, render: (_, item) => item.resolutionType ? stateTag(item.resolutionType) : '-' },
    { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    {
      title: '操作',
      valueType: 'option',
      render: (_, item) => item.state === 'open' ? (
        <Space wrap>
          <Button size="small" type="primary" onClick={() => openDecisionResolution(item, 'provide_missing_info')}>补充信息</Button>
          <Button size="small" onClick={() => void resolve(item, 'replan', { operator: 'console' })}>重新规划并继续</Button>
          <Button size="small" danger onClick={() => void resolve(item, 'reject_current_path', { operator: 'console' })}>拒绝路径并继续</Button>
        </Space>
      ) : stateTag(item.resolutionType)
    }
  ];

  return (
    <PageContainer
      title="决策工单"
      subTitle={`处理阻塞${modelLabels.run}的${modelLabels.decision}，支持补充信息、重新规划和拒绝当前路径`}
      extra={(
        <Space wrap>
          <Button disabled={selectedDecisionKeys.length === 0} onClick={() => void batchResolve('replan')}>批量重新规划</Button>
          <Button danger disabled={selectedDecisionKeys.length === 0} onClick={() => void batchResolve('reject_current_path')}>批量拒绝路径</Button>
        </Space>
      )}
    >
      <ProTable<Decision>
        actionRef={actionRef}
        rowKey="id"
        search={false}
        options={false}
        rowSelection={{
          selectedRowKeys: selectedDecisionKeys,
          onChange: setSelectedDecisionKeys,
          getCheckboxProps: (record) => ({ disabled: record.state !== 'open' })
        }}
        columns={columns}
        request={async () => {
          const items = await workorchApi.listDecisions();
          return tableResult(items.sort((left, right) => Number(left.state !== 'open') - Number(right.state !== 'open')));
        }}
        expandable={{
          expandedRowRender: (item) => (
            <ProCard gutter={16} wrap ghost>
              <ProCard title="决策上下文" colSpan={{ xs: 24, lg: 12 }}>
                <pre className="json-panel json-panel-large">{compactJSON(item.context)}</pre>
              </ProCard>
              <ProCard title="解决内容" colSpan={{ xs: 24, lg: 12 }}>
                <pre className="json-panel json-panel-large">{compactJSON(item.resolutionPayload)}</pre>
              </ProCard>
              <ProCard title="自动治理摘要" colSpan={{ xs: 24, lg: 12 }}>
                <pre className="json-panel json-panel-large">{compactJSON(item.autoGovernance)}</pre>
              </ProCard>
            </ProCard>
          )
        }}
      />
      <Modal
        title="补充决策信息"
        open={Boolean(decisionTarget)}
        confirmLoading={decisionSubmitting}
        onCancel={() => setDecisionTarget(undefined)}
        onOk={() => void submitDecisionResolution()}
        okText="提交并转入后台"
        destroyOnHidden
      >
        {decisionTarget ? (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions
              column={1}
              size="small"
              items={[
                { key: 'ticket', label: '决策工单', children: decisionTarget.item.id },
                { key: 'run', label: `关联${modelLabels.run}`, children: decisionTarget.item.runId },
                { key: 'question', label: '问题', children: decisionTarget.item.question || '-' },
                { key: 'reason', label: '原因', children: decisionTarget.item.reason || '-' }
              ]}
            />
            <Form form={decisionForm} layout="vertical">
              {decisionPayloadFields(decisionTarget.item).map((field) => (
                <Form.Item
                  key={field.name}
                  name={['structuredPayload', field.name]}
                  label={field.label}
                  rules={field.required ? [{ required: true, message: `请补充${field.label}` }] : undefined}
                >
                  <Input />
                </Form.Item>
              ))}
              {decisionPayloadFields(decisionTarget.item).length === 0 ? (
                <Form.Item name="answer" label="补充信息" rules={[{ required: true, message: '请填写补充信息' }]}>
                  <Input.TextArea rows={4} placeholder="按决策问题补充业务信息，例如目标对象、执行范围、路径选择或缺失参数。" />
                </Form.Item>
              ) : (
                <Form.Item name="answer" label="补充说明">
                  <Input.TextArea rows={3} placeholder="可选。用于补充字段之外的业务判断、执行约束或上下文说明。" />
                </Form.Item>
              )}
              <Form.Item name="note" label="处理备注">
                <Input.TextArea rows={2} placeholder="可选。记录本次处理依据，供后续审计和审批查看。" />
              </Form.Item>
              <Form.Item
                name="extraPayloadText"
                label="高级 JSON 载荷"
                tooltip="可选。填写 JSON 对象，会合并到 resolutionPayload，用于复杂结构化信息。"
                rules={[
                  {
                    validator: (_, value) => {
                      try {
                        parseDecisionExtraPayload(value);
                        return Promise.resolve();
                      } catch (error) {
                        return Promise.reject(error instanceof Error ? error : new Error('JSON 格式不正确'));
                      }
                    }
                  }
                ]}
              >
                <Input.TextArea rows={4} placeholder='例如：{"target":"业务对象","method":"执行方式","constraints":["约束条件"]}' />
              </Form.Item>
              <Form.Item name="operator" label="处理人" rules={[{ required: true, message: '请填写处理人' }]}>
                <Input />
              </Form.Item>
            </Form>
            <Typography.Text type="secondary">上下文</Typography.Text>
            <pre className="json-panel">{compactJSON(decisionTarget.item.context)}</pre>
          </Space>
        ) : null}
      </Modal>
    </PageContainer>
  );
}

function ApprovalQueuePage() {
  const { message } = AntApp.useApp();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [selectedApprovalKeys, setSelectedApprovalKeys] = useState<React.Key[]>([]);
  const [approvalTarget, setApprovalTarget] = useState<{ item: Approval; action: 'approve' | 'reject'; relatedDecision?: Decision } | undefined>(undefined);
  const [approvalSubmitting, setApprovalSubmitting] = useState(false);
  const [approvalForm] = Form.useForm<{ operator?: string; comment?: string }>();
  const actionLabel = (action: 'approve' | 'reject') => action === 'approve' ? '已批准' : '已拒绝';

  const openApprovalResolution = async (item: Approval, action: 'approve' | 'reject') => {
    approvalForm.resetFields();
    approvalForm.setFieldsValue({ operator: 'console' });
    try {
      const decisions = await workorchApi.listDecisions();
      const relatedDecision = decisions
        .filter((decision) => decision.runId === item.runId && decision.state === 'resolved')
        .at(-1);
      setApprovalTarget({ item, action, relatedDecision });
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const submitApprovalResolution = async () => {
    if (!approvalTarget) {
      return;
    }
    setApprovalSubmitting(true);
    try {
      await approvalForm.validateFields();
      if (approvalTarget.action === 'approve') {
        await workorchApi.approveApproval(approvalTarget.item.id);
      } else {
        await workorchApi.rejectApproval(approvalTarget.item.id);
      }
      const target = approvalTarget;
      setApprovalTarget(undefined);
      approvalForm.resetFields();
      message.success(`审批${actionLabel(target.action)}，${modelLabels.run} 已由服务端转入后台继续`);
      actionRef.current?.reload();
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setApprovalSubmitting(false);
    }
  };

  const batchResolve = async (action: 'approve' | 'reject') => {
    const approvalIds = selectedApprovalKeys.map(String);
    if (approvalIds.length === 0) {
      message.warning('请先选择待审批项');
      return;
    }
    try {
      await workorchApi.batchResolveApprovals(approvalIds, action);
      message.success(`${approvalIds.length} 个审批项${actionLabel(action)}，相关${modelLabels.run}已由服务端转入后台继续`);
      setSelectedApprovalKeys([]);
      actionRef.current?.reload();
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const columns: ProColumns<Approval>[] = [
    { title: '审批 ID', dataIndex: 'id', copyable: true },
    { title: '工作空间', dataIndex: 'workspaceId', copyable: true },
    { title: '运行', dataIndex: 'runId', copyable: true },
    { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
    { title: '原因', dataIndex: 'reason', ellipsis: true },
    { title: '自动治理', dataIndex: 'autoGovernance', render: (_, item) => item.autoGovernance ? stateTag(`${item.autoGovernance.status}:${item.autoGovernance.action}`) : '-' },
    { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    {
      title: '操作',
      valueType: 'option',
      render: (_, item) => item.state === 'pending' ? (
        <Space wrap>
          <Button size="small" type="primary" onClick={() => void openApprovalResolution(item, 'approve')}>审批确认</Button>
          <Button size="small" danger onClick={() => void openApprovalResolution(item, 'reject')}>拒绝确认</Button>
        </Space>
      ) : stateTag(item.state)
    }
  ];

  return (
    <PageContainer
      title="审批队列"
      subTitle={`处理阻塞${modelLabels.run}的${modelLabels.approval}，批准后运行可继续推进，拒绝后运行失败收口`}
      extra={(
        <Space wrap>
          <Button disabled={selectedApprovalKeys.length === 0} onClick={() => void batchResolve('approve')}>批量批准</Button>
          <Button danger disabled={selectedApprovalKeys.length === 0} onClick={() => void batchResolve('reject')}>批量拒绝</Button>
        </Space>
      )}
    >
      <ProTable<Approval>
        actionRef={actionRef}
        rowKey="id"
        search={false}
        options={false}
        rowSelection={{
          selectedRowKeys: selectedApprovalKeys,
          onChange: setSelectedApprovalKeys,
          getCheckboxProps: (record) => ({ disabled: record.state !== 'pending' })
        }}
        columns={columns}
        request={async () => {
          const items = await workorchApi.listApprovals();
          return tableResult(items.sort((left, right) => Number(left.state !== 'pending') - Number(right.state !== 'pending')));
        }}
      />
      <Modal
        title={approvalTarget?.action === 'reject' ? '拒绝审批确认' : '审批确认'}
        open={Boolean(approvalTarget)}
        confirmLoading={approvalSubmitting}
        onCancel={() => setApprovalTarget(undefined)}
        onOk={() => void submitApprovalResolution()}
        okText={approvalTarget?.action === 'reject' ? '拒绝并转入后台' : '批准并转入后台'}
        okButtonProps={{ danger: approvalTarget?.action === 'reject' }}
        destroyOnHidden
      >
        {approvalTarget ? (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions
              column={1}
              size="small"
              items={[
                { key: 'approval', label: '审批 ID', children: approvalTarget.item.id },
                { key: 'run', label: `关联${modelLabels.run}`, children: approvalTarget.item.runId },
                { key: 'step', label: '步骤', children: approvalTarget.item.stepId || '-' },
                { key: 'reason', label: '审批原因', children: approvalTarget.item.reason || '-' }
              ]}
            />
            <Typography.Text type="secondary">已补充的决策信息</Typography.Text>
            <pre className="json-panel">{compactJSON(approvalTarget.relatedDecision?.resolutionPayload)}</pre>
            <Typography.Text type="secondary">自动治理摘要</Typography.Text>
            <pre className="json-panel">{compactJSON(approvalTarget.item.autoGovernance)}</pre>
            <Form form={approvalForm} layout="vertical">
              <Form.Item name="operator" label="审批人" rules={[{ required: true, message: '请填写审批人' }]}>
                <Input />
              </Form.Item>
              <Form.Item name="comment" label="审批意见">
                <Input.TextArea rows={3} placeholder="记录本次审批判断依据，当前版本用于界面确认，不写入审批模型" />
              </Form.Item>
            </Form>
          </Space>
        ) : null}
      </Modal>
    </PageContainer>
  );
}

function GovernanceQueuePage() {
  return (
    <PageContainer
      title="治理队列"
      subTitle={`统一处理人工决策与审批，确保阻塞${modelLabels.run}能按正式业务语义继续、失败或重新规划`}
    >
      <Tabs
        items={[
          {
            key: 'decisions',
            label: '决策工单',
            children: <DecisionQueuePage />
          },
          {
            key: 'approvals',
            label: '审批处理',
            children: <ApprovalQueuePage />
          }
        ]}
      />
    </PageContainer>
  );
}

function WorkspaceDetailPage({ selectedWorkspaceId }: Pick<ControlPlaneProps, 'selectedWorkspaceId'>) {
  const { message } = AntApp.useApp();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspaceId, setWorkspaceId] = useState<string | undefined>(undefined);
  const [snapshot, setSnapshot] = useState<WorkspaceSnapshot | undefined>(undefined);
  const [goals, setGoals] = useState<Goal[]>([]);
  const [dods, setDoDs] = useState<DoD[]>([]);
  const [souls, setSouls] = useState<Soul[]>([]);
  const [soulBindings, setSoulBindings] = useState<WorkspaceSoulBindings | undefined>(undefined);
  const [workspaceTemplates, setWorkspaceTemplates] = useState<WorkspaceTemplate[]>([]);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [mcpServers, setMCPServers] = useState<MCPServer[]>([]);
  const [mcpCapabilities, setMCPCapabilities] = useState<MCPCapability[]>([]);
  const [memoryItems, setMemoryItems] = useState<MemoryItem[]>([]);
  const [configuration, setConfiguration] = useState<ServiceConfiguration | undefined>(undefined);
  const [runSnapshot, setRunSnapshot] = useState<RunSnapshot | undefined>(undefined);
  const [runObservability, setRunObservability] = useState<RunObservability | undefined>(undefined);
  const [contextBundle, setContextBundle] = useState<ContextBundle | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [activationOpen, setActivationOpen] = useState(false);
  const [activationLoading, setActivationLoading] = useState(false);
  const [workspaceSetupOpen, setWorkspaceSetupOpen] = useState(false);
  const [goalDefinitionOpen, setGoalDefinitionOpen] = useState(false);
  const [dodDefinitionOpen, setDoDDefinitionOpen] = useState(false);
  const [soulBindingEditorOpen, setSoulBindingEditorOpen] = useState(false);
  const [workspaceDeleteOpen, setWorkspaceDeleteOpen] = useState(false);
  const [businessSubmitting, setBusinessSubmitting] = useState(false);
  const [workspaceDeleting, setWorkspaceDeleting] = useState(false);
  const [soulBindingSaving, setSoulBindingSaving] = useState(false);
  const [editingGoal, setEditingGoal] = useState<Goal | undefined>(undefined);
  const [editingDoD, setEditingDoD] = useState<DoD | undefined>(undefined);
  const [activationTracking, setActivationTracking] = useState(false);
  const [workspaceListReady, setWorkspaceListReady] = useState(false);
  const [setupStep, setSetupStep] = useState(0);
  const workspaceListRequestSeq = useRef(0);
  const workspaceContextRequestSeq = useRef(0);
  const currentWorkspaceIdRef = useRef<string | undefined>(undefined);
  const workspaceEventRefreshTimerRef = useRef<number | undefined>(undefined);
  const [activationForm] = Form.useForm();
  const [soulBindingsForm] = Form.useForm();
  const [setupSoulBindingsForm] = Form.useForm<SoulBindingFormValues>();
  const [workspaceForm] = Form.useForm<WorkspaceSetupFormValues>();
  const [setupGoalForm] = Form.useForm<GoalSetupFormValues>();
  const [setupDoDForm] = Form.useForm<DoDSetupFormValues>();
  const [workspaceDeleteForm] = Form.useForm<WorkspaceDeleteFormValues>();
  const [goalForm] = Form.useForm();
  const [dodForm] = Form.useForm();

  useEffect(() => {
    currentWorkspaceIdRef.current = workspaceId;
  }, [workspaceId]);

  useEffect(() => {
    setWorkspaceListReady(false);
    setLoading(true);
    void loadWorkspaceList(selectedWorkspaceId)
      .catch((error: unknown) => {
        message.error(errorMessage(error));
      })
      .finally(() => setLoading(false));
  }, [selectedWorkspaceId]);

  useEffect(() => {
    if (!workspaceListReady) {
      return;
    }
    if (!workspaceId) {
      clearWorkspaceContext();
      return;
    }
    setLoading(true);
    void loadWorkspaceContext(workspaceId).catch((error: unknown) => {
      if (isNotFoundError(error)) {
        message.warning('当前工作空间不存在，已刷新可用工作空间列表');
        void loadWorkspaceList(undefined, workspaceId).catch((refreshError: unknown) => {
          message.error(errorMessage(refreshError));
        });
      } else {
        message.error(errorMessage(error));
      }
    }).finally(() => setLoading(false));
  }, [workspaceListReady, workspaceId]);

  useEffect(() => {
    if (!workspaceListReady) {
      return undefined;
    }
    const stream = createWorkorchEventStream();
    const scheduleRefresh = (refreshList: boolean, targetWorkspaceId?: string) => {
      if (workspaceEventRefreshTimerRef.current !== undefined) {
        window.clearTimeout(workspaceEventRefreshTimerRef.current);
      }
      workspaceEventRefreshTimerRef.current = window.setTimeout(() => {
        workspaceEventRefreshTimerRef.current = undefined;
        const selected = currentWorkspaceIdRef.current;
        const nextWorkspaceId = targetWorkspaceId || selected;
        if (refreshList) {
          void loadWorkspaceList(nextWorkspaceId).catch((error: unknown) => {
            message.error(errorMessage(error));
          });
        }
        if (nextWorkspaceId) {
          void loadWorkspaceContext(nextWorkspaceId).catch((error: unknown) => {
            if (isNotFoundError(error)) {
              void loadWorkspaceList(undefined, nextWorkspaceId).catch((refreshError: unknown) => {
                message.error(errorMessage(refreshError));
              });
              return;
            }
            message.error(errorMessage(error));
          });
        }
      }, 150);
    };
    const handleEvent = (event: MessageEvent<string>) => {
      try {
        const envelope = JSON.parse(event.data) as { topic?: string; workspaceId?: string };
        const topic = String(envelope.topic ?? '');
        const eventWorkspaceId = envelope.workspaceId;
        const refreshList = topic.includes('workspace');
        if (refreshList || !eventWorkspaceId || eventWorkspaceId === currentWorkspaceIdRef.current) {
          scheduleRefresh(refreshList, eventWorkspaceId);
        }
      } catch {
        scheduleRefresh(false, currentWorkspaceIdRef.current);
      }
    };
    stream.addEventListener('workorch', handleEvent);
    stream.onerror = () => {
      stream.close();
    };
    return () => {
      stream.removeEventListener('workorch', handleEvent);
      stream.close();
      if (workspaceEventRefreshTimerRef.current !== undefined) {
        window.clearTimeout(workspaceEventRefreshTimerRef.current);
        workspaceEventRefreshTimerRef.current = undefined;
      }
    };
  }, [workspaceListReady]);

  const readyDoDs = dods.filter((item) => item.state === 'ready' || item.state === 'satisfied');
  const llmProviders = configuration?.llm.providers ?? [];
  const soulBindingStages = soulBindings?.availableStages?.length ? soulBindings.availableStages : defaultSoulBindingStages;
  const soulOptions = souls.map((item) => ({
    value: item.id,
    label: `${item.name || item.id} (${item.id})`
  }));
  const soulNameById = new Map(souls.map((item) => [item.id, item.name ? `${item.name} (${item.id})` : item.id]));
  const workspaceDefaultSoulLabel = soulBindings?.defaultSoulId
    ? soulNameById.get(soulBindings.defaultSoulId) || soulBindings.defaultSoulId
    : '未配置';
  const stageBindingTags = soulBindingStages
    .filter((stage) => soulBindings?.bindings?.[stage])
    .map((stage) => `${soulBindingStageLabel(stage)}: ${soulNameById.get(soulBindings?.bindings?.[stage] || '') || soulBindings?.bindings?.[stage]}`);

  function clearWorkspaceContext() {
    setSnapshot(undefined);
    setGoals([]);
    setDoDs([]);
    setSoulBindings(undefined);
    setSkills([]);
    setMCPServers([]);
    setMCPCapabilities([]);
    setMemoryItems([]);
    setRunSnapshot(undefined);
    setRunObservability(undefined);
    setContextBundle(undefined);
  }

  async function loadWorkspaceList(preferredWorkspaceId?: string, ignoredWorkspaceId?: string) {
    const requestSeq = ++workspaceListRequestSeq.current;
    const items = await workorchApi.listWorkspaces();
    if (requestSeq !== workspaceListRequestSeq.current) {
      return workspaceId;
    }
    setWorkspaces(items);
    const selectableItems = ignoredWorkspaceId ? items.filter((item) => item.id !== ignoredWorkspaceId) : items;
    const selectableIDs = new Set(selectableItems.map((item) => item.id));
    const nextWorkspaceId = preferredWorkspaceId && selectableIDs.has(preferredWorkspaceId)
      ? preferredWorkspaceId
      : workspaceId && selectableIDs.has(workspaceId)
        ? workspaceId
        : selectableItems[0]?.id;
    setWorkspaceListReady(true);
    setWorkspaceId(nextWorkspaceId);
    if (!nextWorkspaceId) {
      clearWorkspaceContext();
    }
    return nextWorkspaceId;
  }

  async function loadWorkspaceContext(targetWorkspaceId: string) {
    const requestSeq = ++workspaceContextRequestSeq.current;
    const [nextSnapshot, nextContextBundle, nextGoals, nextDoDs, nextSouls, nextSoulBindings, nextSkills, nextMCPServers, nextMCPCapabilities, nextMemoryItems, nextConfiguration] = await Promise.all([
      workorchApi.readWorkspaceSnapshot(targetWorkspaceId),
      workorchApi.readWorkspaceContextBundle(targetWorkspaceId),
      workorchApi.listGoals(targetWorkspaceId),
      workorchApi.listDoDs(targetWorkspaceId),
      workorchApi.listSouls(),
      workorchApi.readWorkspaceSoulBindings(targetWorkspaceId),
      workorchApi.listSkills(targetWorkspaceId),
      workorchApi.listMCPServers(),
      workorchApi.listMCPCapabilities({ enabled: true }),
      workorchApi.listMemory(targetWorkspaceId),
      workorchApi.readConfiguration()
    ]);
    if (requestSeq !== workspaceContextRequestSeq.current) {
      return;
    }
    setSnapshot(nextSnapshot);
    setContextBundle(nextContextBundle);
    setGoals(nextGoals);
    setDoDs(nextDoDs);
    setSouls(nextSouls);
    setSoulBindings(nextSoulBindings);
    soulBindingsForm.setFieldsValue({
      defaultSoulId: nextSoulBindings.defaultSoulId,
      bindings: nextSoulBindings.bindings ?? {}
    });
    setSkills(nextSkills);
    setMCPServers(nextMCPServers);
    setMCPCapabilities(nextMCPCapabilities);
    setMemoryItems(nextMemoryItems);
    setConfiguration(nextConfiguration);
    const projectedRun = workspaceProjectionRun(nextSnapshot);
    if (projectedRun) {
      const [nextRunSnapshot, nextRunObservability, nextRunContextBundle] = await Promise.all([
        workorchApi.readRunSnapshot(projectedRun.id),
        workorchApi.readRunObservability(projectedRun.id),
        workorchApi.readRunContextBundle(projectedRun.id)
      ]);
      if (requestSeq !== workspaceContextRequestSeq.current) {
        return;
      }
      setRunSnapshot(nextRunSnapshot);
      setRunObservability(nextRunObservability);
      setContextBundle(nextRunContextBundle);
    } else {
      setRunSnapshot(undefined);
      setRunObservability(undefined);
      setContextBundle(nextContextBundle);
    }
  }

  const refreshWorkspace = async () => {
    if (!workspaceId) {
      return;
    }
    await loadWorkspaceContext(workspaceId);
  };

  const saveWorkspaceSoulBindings = async (values: { defaultSoulId?: string; bindings?: Record<string, string> }) => {
    if (!workspaceId) {
      message.error('请先选择工作空间');
      return;
    }
    setSoulBindingSaving(true);
    try {
      const nextBindings = await workorchApi.updateWorkspaceSoulBindings(workspaceId, {
        defaultSoulId: values.defaultSoulId,
        bindings: values.bindings
      });
      setSoulBindings(nextBindings);
      soulBindingsForm.setFieldsValue({
        defaultSoulId: nextBindings.defaultSoulId,
        bindings: nextBindings.bindings ?? {}
      });
      await loadWorkspaceContext(workspaceId);
      setSoulBindingEditorOpen(false);
      message.success('执行人格配置已保存');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSoulBindingSaving(false);
    }
  };

  const closeWorkspaceSetup = () => {
    setWorkspaceSetupOpen(false);
    setSetupStep(0);
    workspaceForm.resetFields();
    setupGoalForm.resetFields();
    setupDoDForm.resetFields();
    setupSoulBindingsForm.resetFields();
  };

  const openWorkspaceSetup = async () => {
    setLoading(true);
    try {
      const [nextSouls, nextTemplates] = await Promise.all([
        workorchApi.listSouls(),
        workorchApi.listWorkspaceTemplates()
      ]);
      setSouls(nextSouls);
      setWorkspaceTemplates(nextTemplates);
      workspaceForm.resetFields();
      setupGoalForm.resetFields();
      setupDoDForm.resetFields();
      setupSoulBindingsForm.resetFields();
      setSetupStep(0);
      setWorkspaceSetupOpen(true);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  };

  const applyWorkspaceTemplate = (templateID?: string) => {
    const template = workspaceTemplates.find((item) => item.id === templateID);
    if (!template) {
      setupGoalForm.resetFields();
      setupDoDForm.resetFields();
      setupSoulBindingsForm.resetFields();
      return;
    }
    setupGoalForm.setFieldsValue({ text: template.goalText || '' });
    setupDoDForm.setFieldsValue({ items: joinLines(template.doDItems) });
    setupSoulBindingsForm.setFieldsValue({
      defaultSoulId: template.defaultSoulId,
      bindings: template.soulBindings ?? {}
    });
  };

  const validateWorkspaceSetupStep = async (step: number) => {
    if (step === 0) {
      await workspaceForm.validateFields();
      return;
    }
    if (step === 1) {
      await setupGoalForm.validateFields();
      return;
    }
    if (step === 2) {
      const values = await setupDoDForm.validateFields();
      if (setupDoDItems(values.items).length === 0) {
        throw new Error('请至少填写一个验收项');
      }
      return;
    }
    if (step === 3) {
      await setupSoulBindingsForm.validateFields();
    }
  };

  const goNextWorkspaceSetupStep = async () => {
    try {
      await validateWorkspaceSetupStep(setupStep);
      setSetupStep((current) => Math.min(current + 1, 4));
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const goPreviousWorkspaceSetupStep = () => {
    setSetupStep((current) => Math.max(current - 1, 0));
  };

  const submitWorkspaceSetup = async () => {
    setBusinessSubmitting(true);
    try {
      await validateWorkspaceSetupStep(0);
      await validateWorkspaceSetupStep(1);
      await validateWorkspaceSetupStep(2);
      await validateWorkspaceSetupStep(3);

      const workspaceValues = workspaceForm.getFieldsValue();
      const goalValues = setupGoalForm.getFieldsValue();
      const dodValues = setupDoDForm.getFieldsValue();
      const soulBindingValues = setupSoulBindingsForm.getFieldsValue();
      const workspace = await workorchApi.createWorkspace(workspaceValues.title, workspaceValues.description, workspaceValues.workingDirectory);
      await workorchApi.defineGoal(workspace.id, goalValues.text);
      await workorchApi.defineDoD(workspace.id, setupDoDItems(dodValues.items));
      await workorchApi.updateWorkspaceSoulBindings(workspace.id, normalizeSoulBindingFormValues(soulBindingValues));
      await loadWorkspaceList(workspace.id);
      await loadWorkspaceContext(workspace.id);
      closeWorkspaceSetup();
      message.success('工作空间、目标、验收标准和执行人格配置已提交');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setBusinessSubmitting(false);
    }
  };

  const openSoulBindingEditor = () => {
    soulBindingsForm.setFieldsValue({
      defaultSoulId: soulBindings?.defaultSoulId,
      bindings: soulBindings?.bindings ?? {}
    });
    setSoulBindingEditorOpen(true);
  };

  const openWorkspaceDelete = () => {
    workspaceDeleteForm.setFieldsValue({ purge: false });
    setWorkspaceDeleteOpen(true);
  };

  const submitWorkspaceDelete = async () => {
    if (!workspaceId) {
      return;
    }
    const targetWorkspaceId = workspaceId;
    const values = workspaceDeleteForm.getFieldsValue();
    setWorkspaceDeleting(true);
    try {
      await workorchApi.deleteWorkspace(targetWorkspaceId, values.purge);
      setWorkspaceDeleteOpen(false);
      workspaceDeleteForm.resetFields();
      clearWorkspaceContext();
      await loadWorkspaceList(undefined, targetWorkspaceId);
      message.success(values.purge ? '工作空间及关联数据已删除' : '工作空间已删除');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setWorkspaceDeleting(false);
    }
  };

  const activateWorkspaceExecution = (request: WorkspaceExecutionActivationRequest) => {
    if (!workspaceId) {
      return;
    }
    const selectedCapabilityServerIds = (request.capabilityIds ?? [])
      .map((capabilityId) => mcpCapabilities.find((item) => item.id === capabilityId)?.serverId)
      .filter((value): value is string => Boolean(value));
    const requestWithCapabilityServers: WorkspaceExecutionActivationRequest = {
      ...request,
      mcpServerIds: Array.from(new Set([...(request.mcpServerIds ?? []), ...selectedCapabilityServerIds]))
    };
    const targetWorkspaceId = workspaceId;
    setActivationLoading(true);
    setActivationOpen(false);
    setActivationTracking(true);
    activationForm.resetFields();
    message.info('工作空间执行已提交，已转入后台跟踪');
    void workorchApi.activateWorkspaceExecution(targetWorkspaceId, requestWithCapabilityServers)
      .then(async (activation) => {
        setSnapshot(activation.snapshot);
        setRunSnapshot(undefined);
        setRunObservability(undefined);
        setContextBundle(undefined);
        await loadWorkspaceContext(targetWorkspaceId);
        message.success('工作空间执行已完成一次状态刷新');
      })
      .catch((error: unknown) => {
        message.error(errorMessage(error));
      })
      .finally(() => {
        setActivationLoading(false);
        setActivationTracking(false);
      });
  };

  const requestWorkspaceActivation = () => {
    activationForm.setFieldsValue({
      goalId: goals[0]?.id,
      doDId: readyDoDs[0]?.id,
      llmProviderId: configuration?.llm.defaultProviderId,
      soulId: undefined,
      soulBindings: {},
      skillIds: [],
      mcpServerIds: [],
      capabilityIds: [],
      memoryIds: [],
      priority: 0
    });
    setActivationOpen(true);
  };

  const openGoalDefinition = (goal?: Goal) => {
    setEditingGoal(goal);
    goalForm.setFieldsValue({ text: goal?.text || '' });
    setGoalDefinitionOpen(true);
  };

  const openDoDDefinition = (dod?: DoD) => {
    setEditingDoD(dod);
    dodForm.setFieldsValue({ items: dod?.items?.join('\n') || '' });
    setDoDDefinitionOpen(true);
  };

  const runColumns: ProColumns<Run>[] = [
    { title: '运行 ID（Run ID）', dataIndex: 'id', copyable: true },
    { title: '目标', dataIndex: 'goalId', copyable: true },
    { title: '验收标准', dataIndex: 'doDId', copyable: true },
    { title: '优先级', render: (_, item) => runPriority(item), width: 100 },
    { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    { title: '阻塞工单', dataIndex: 'blockingTicketId', copyable: true },
    {
      title: '操作',
      valueType: 'option',
      render: (_, item) => (
        <Space>
          <Button
            size="small"
            onClick={() => void Promise.all([
              workorchApi.readRunSnapshot(item.id),
              workorchApi.readRunObservability(item.id)
            ]).then(([nextSnapshot, nextObservability]) => {
              setRunSnapshot(nextSnapshot);
              setRunObservability(nextObservability);
            }).catch((error: unknown) => message.error(errorMessage(error)))}
          >
            查看成果与过程
          </Button>
          <Button
            size="small"
            onClick={() => void workorchApi.resumeRun(item.id, numericRunPriority(item))
              .then(refreshWorkspace)
              .catch((error: unknown) => message.error(errorMessage(error)))}
          >
            继续运行
          </Button>
        </Space>
      )
    }
  ];
  const runSummary = runStateSummary(snapshot?.runs);
  const outcomeRun = runSnapshot?.run;
  const outcomeGoal = goals.find((item) => item.id === outcomeRun?.goalId);
  const outcomeDoD = dods.find((item) => item.id === outcomeRun?.doDId);
  const outcomeResult = primaryAcceptedResult(runSnapshot) ?? primaryResult(runSnapshot);
  const outcomeSummary = outcomeResult?.summaryDetail;
  const outcomeArtifacts = outcomeResult
    ? (runSnapshot?.artifacts ?? []).filter((item) => item.resultId === outcomeResult.id || !item.resultId)
    : (runSnapshot?.artifacts ?? []);
  const outcomeMemoryRefs = memoryRefIDs(runSnapshot);
  const outcomeMemories = memoryItems.filter((item) => item.runId === outcomeRun?.id || outcomeMemoryRefs.has(item.id));
  const outcomeDoDItems = dodAssessmentItems(runSnapshot);
  const runWebSources = runObservability?.webSources ?? runSnapshot?.webSources ?? snapshot?.webSources;
  const webSourceCount = webSourceRows(runWebSources).length;
  const runLLMContextBudgets = runObservability?.llmContextBudgets ?? runSnapshot?.llmContextBudgets ?? snapshot?.llmContextBudgets;
  const runLLMUsageSummary = runObservability?.llmUsageSummary ?? runSnapshot?.llmUsageSummary ?? snapshot?.llmUsageSummary;
  const runLLMUsageTraces = runObservability?.llmUsageTraces ?? runSnapshot?.llmUsageTraces ?? snapshot?.llmUsageTraces;
  const dodDefinitionRows = dods.flatMap((dod) => dod.items.map((text, index) => ({
    key: `${dod.id}-${index}`,
    id: dod.id,
    state: dod.state,
    index: index + 1,
    text
  })));

  return (
    <PageContainer
      title="工作空间"
      subTitle="围绕工作空间（Workspace）管理目标、验收标准、执行过程、治理阻塞和运行证据"
      extra={
        <div className="workspace-page-header-actions">
          <div className="workspace-header-context">
            <Typography.Text type="secondary" className="workspace-header-context-label">当前工作空间</Typography.Text>
            <Select
              className="workspace-header-select"
              placeholder="选择工作空间"
              value={workspaceId}
              onChange={setWorkspaceId}
              options={workspaces.map((item) => ({ value: item.id, label: `${item.title || item.id} (${item.id})` }))}
            />
            {activationTracking ? <Tag color="processing">后台跟踪中</Tag> : null}
          </div>
          <div className="workspace-header-actions-main">
            <Button type="primary" disabled={!workspaceId} onClick={requestWorkspaceActivation}>启动执行</Button>
            <Button onClick={() => void openWorkspaceSetup()}>创建工作空间</Button>
            <Dropdown
              menu={{
                items: [
                  { key: 'goal', label: '定义目标', disabled: !workspaceId },
                  { key: 'dod', label: '定义验收标准', disabled: !workspaceId },
                  { key: 'soul', label: '调整执行人格', disabled: !workspaceId }
                ],
                onClick: ({ key }) => {
                  if (key === 'goal') {
                    openGoalDefinition();
                  } else if (key === 'dod') {
                    openDoDDefinition();
                  } else if (key === 'soul') {
                    openSoulBindingEditor();
                  }
                }
              }}
            >
              <Button disabled={!workspaceId}>完善定义 <DownOutlined /></Button>
            </Dropdown>
            <Dropdown
              menu={{
                items: [
                  { key: 'delete', label: '删除工作空间', danger: true, disabled: !workspaceId }
                ],
                onClick: ({ key }) => {
                  if (key === 'delete') {
                    openWorkspaceDelete();
                  }
                }
              }}
            >
              <Button disabled={!workspaceId}>更多操作 <DownOutlined /></Button>
            </Dropdown>
          </div>
        </div>
      }
    >
      {snapshot ? (
        <>
          <ProCard loading={loading} title={snapshot.workspace.title || snapshot.workspace.id} extra={stateTag(snapshot.outcome?.state ?? snapshot.workspace.state)}>
            <Descriptions
              column={2}
              items={[
                { key: 'id', label: '工作空间 ID', children: snapshot.workspace.id },
                { key: 'activeRun', label: '活跃运行', children: snapshot.workspace.activeRunId || '-' },
                { key: 'outcome', label: '业务结果', children: snapshot.outcome?.runId ? `${snapshot.outcome.runId} / ${stateLabel(snapshot.outcome.state)}` : stateLabel(snapshot.outcome?.state ?? snapshot.workspace.state) },
                { key: 'description', label: '描述', children: boundedParagraph(snapshot.workspace.description, { rows: 2 }) },
                { key: 'workingDirectory', label: '工作目录', children: boundedParagraph(snapshot.workspace.workingDirectory, { rows: 1, copyText: snapshot.workspace.workingDirectory }) },
                { key: 'defaultSoul', label: '默认执行人格', children: workspaceDefaultSoulLabel },
                { key: 'stageSouls', label: '阶段执行人格', children: compactTags(stageBindingTags, 4) }
              ]}
            />
          </ProCard>
          <ProCard title="执行过程" className="workorch-section">
            <Steps items={workspaceProcessItems(snapshot, goals, dods)} />
          </ProCard>
          <ProCard title="主要业务模型" className="workorch-section">
            {metricGrid([
              { key: 'workspace', label: modelLabels.workspace, value: 1, color: '#1668dc' },
              { key: 'goal', label: modelLabels.goal, value: goals.length },
              { key: 'dod', label: modelLabels.dod, value: dods.length },
              { key: 'run', label: modelLabels.run, value: snapshot.runs?.length ?? 0, color: '#0958d9' },
              { key: 'decision', label: modelLabels.decision, value: snapshot.decisions?.length ?? 0 },
              { key: 'approval', label: modelLabels.approval, value: snapshot.approvals?.length ?? 0 },
              { key: 'soul', label: modelLabels.soul, value: souls.length },
              { key: 'skill', label: modelLabels.skill, value: skills.length },
              { key: 'mcp', label: modelLabels.mcpServer, value: mcpServers.length },
              { key: 'memory', label: modelLabels.memory, value: memoryItems.length },
              { key: 'step', label: modelLabels.step, value: runSnapshot?.steps?.length ?? 0 },
              { key: 'result', label: modelLabels.result, value: snapshot.results?.length ?? runSnapshot?.results?.length ?? 0, color: '#389e0d' },
              { key: 'artifact', label: modelLabels.artifact, value: snapshot.artifacts?.length ?? runSnapshot?.artifacts?.length ?? 0 },
              { key: 'webSources', label: 'Web 来源', value: webSourceCount, color: webSourceCount ? '#08979c' : undefined },
              { key: 'llmContext', label: 'LLM Context', value: runLLMContextBudgets?.length ?? 0, color: (runLLMContextBudgets?.length ?? 0) > 0 ? '#1677ff' : undefined },
              { key: 'llmUsage', label: modelLabels.llmUsage, value: runLLMUsageSummary?.entryTotal ?? 0, color: runLLMUsageSummary?.entryTotal ? '#d48806' : undefined },
              { key: 'tool', label: modelLabels.toolCall, value: runSnapshot?.toolCalls?.length ?? 0 },
              { key: 'patch', label: modelLabels.patch, value: runSnapshot?.patches?.length ?? 0 },
              { key: 'validation', label: modelLabels.validationResult, value: runSnapshot?.validationResults?.length ?? 0 },
              { key: 'checkpoint', label: modelLabels.runtimeCheckpoint, value: runSnapshot?.runtimeCheckpoints?.length ?? 0 }
            ])}
          </ProCard>
          <ProCard title="运行摘要" className="workorch-section">
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              {metricGrid([
                { key: 'runs', label: '运行（Run）总数', value: runSummary.total },
                { key: 'created', label: '已创建', value: runSummary.created },
                { key: 'active', label: '运行中', value: runSummary.active, color: '#0958d9' },
                { key: 'blocked', label: '阻塞中', value: runSummary.blocked, color: '#d48806' },
                { key: 'validating', label: '验收中', value: runSummary.validating, color: '#08979c' },
                { key: 'completed', label: '已完成', value: runSummary.completed, color: '#389e0d' },
                { key: 'failed', label: '失败/终止', value: runSummary.failed, color: '#cf1322' },
                { key: 'decisions', label: '待处理决策', value: pendingCount(snapshot.decisions), color: pendingCount(snapshot.decisions) ? '#d48806' : undefined },
                { key: 'approvals', label: '待处理审批', value: pendingCount(snapshot.approvals), color: pendingCount(snapshot.approvals) ? '#d48806' : undefined },
                { key: 'llmCost', label: 'LLM 估算费用', value: llmUsageCostSummary(runLLMUsageSummary), color: runLLMUsageSummary?.costEstimatedEntries ? '#d48806' : undefined }
              ])}
              <Descriptions
                column={1}
                size="small"
                items={[
                  { key: 'latestRun', label: '最近运行', children: runSummary.latest ? `${runSummary.latest.id} / ${stateLabel(runSummary.latest.state)}` : '-' },
                  { key: 'outcome', label: '业务结果', children: snapshot.outcome?.runId ? `${snapshot.outcome.runId} / ${stateLabel(snapshot.outcome.state)}` : '-' }
                ]}
              />
            </Space>
          </ProCard>
          <ProCard gutter={16} wrap className="workorch-section">
            <ProCard title="目标定义（Goal）" colSpan={{ xs: 24, lg: 9 }} extra={<Tag>{goals.length} 项</Tag>}>
              {goals.length ? (
                <List
                  className="workorch-definition-list"
                  pagination={goals.length > 4 ? { pageSize: 4, size: 'small' } : false}
                  dataSource={goals}
                  renderItem={(item) => (
                    <List.Item
                      actions={[<Button key="edit" type="link" size="small" onClick={() => openGoalDefinition(item)}>修改</Button>]}
                    >
                      <List.Item.Meta
                        title={<Space><Typography.Text copyable>{item.id}</Typography.Text></Space>}
                        description={boundedParagraph(item.text, { rows: 3 })}
                      />
                    </List.Item>
                  )}
                />
              ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无目标" />}
            </ProCard>
            <ProCard title="验收标准（DoD）" colSpan={{ xs: 24, lg: 9 }} extra={<Tag>{dodDefinitionRows.length} 项</Tag>}>
              {dodDefinitionRows.length ? (
                <List
                  className="workorch-definition-list"
                  pagination={dodDefinitionRows.length > 5 ? { pageSize: 5, size: 'small' } : false}
                  dataSource={dodDefinitionRows}
                  renderItem={(item) => {
                    const dod = dods.find((candidate) => candidate.id === item.id);
                    return (
                      <List.Item
                        actions={[<Button key="edit" type="link" size="small" disabled={!dod} onClick={() => dod && openDoDDefinition(dod)}>修改</Button>]}
                      >
                        <List.Item.Meta
                          title={<Space><Tag>{item.index}</Tag>{stateTag(item.state)}<Typography.Text copyable>{item.id}</Typography.Text></Space>}
                          description={boundedParagraph(item.text, { rows: 2 })}
                        />
                      </List.Item>
                    );
                  }}
                />
              ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无验收标准" />}
            </ProCard>
            <ProCard title="执行配置（Soul）" colSpan={{ xs: 24, lg: 6 }} extra={<Button size="small" onClick={openSoulBindingEditor}>调整</Button>}>
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <Typography.Text type="secondary">
                  执行人格（Soul）作为工作空间执行配置参与规划、执行、决策、审批、验收和交付物阶段，不改变目标（Goal）/验收标准（DoD）主体定义。
                </Typography.Text>
                <Descriptions
                  size="small"
                  column={1}
                  items={[
                    { key: 'defaultSoul', label: '默认执行人格', children: workspaceDefaultSoulLabel },
                    { key: 'stageSoul', label: '阶段覆盖', children: compactTags(stageBindingTags, 3) },
                    { key: 'priority', label: '启用优先级', children: compactTags(soulBindings?.priorityPolicy ?? []) }
                  ]}
                />
              </Space>
            </ProCard>
          </ProCard>
          <ProCard
            title="目标成果"
            className="workorch-section"
            extra={goalOutcomeState(runSnapshot)}
          >
            {runSnapshot ? (
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <Descriptions
                  column={2}
                  items={[
                    { key: 'run', label: `关联${modelLabels.run}`, children: `${runSnapshot.run.id} / ${stateLabel(runOutcomeState(runSnapshot))}` },
                    { key: 'rawRunState', label: '原始运行状态', children: stateLabel(runRawState(runSnapshot)) },
                    { key: 'result', label: modelLabels.result, children: outcomeResult ? boundedParagraph(outcomeResult.id, { rows: 1, copyText: outcomeResult.id }) : '-' },
                    { key: 'goal', label: '目标', children: boundedParagraph(outcomeGoal?.text || runSnapshot.run.goalId, { rows: 2 }) },
                    { key: 'dod', label: '验收标准', children: outcomeDoD?.items?.length ? compactTags(outcomeDoD.items) : runSnapshot.run.doDId || '-' },
                    { key: 'summary', label: '成果结论', children: boundedParagraph(goalOutcomeSummary(runSnapshot), { rows: 3, className: 'goal-outcome-summary' }) },
                    { key: 'implementation', label: '实现摘要', children: outcomeSummary?.implementationSummary ? boundedParagraph(outcomeSummary.implementationSummary, { rows: 3 }) : '-' },
                    { key: 'verification', label: '验证摘要', children: outcomeSummary?.verificationSummary ? boundedParagraph(outcomeSummary.verificationSummary, { rows: 3 }) : '-' },
                    { key: 'risks', label: '剩余风险', children: outcomeSummary?.residualRisks?.length ? compactTags(outcomeSummary.residualRisks, 4) : '-' },
                    { key: 'nextActions', label: '后续动作', children: outcomeSummary?.nextActions?.length ? compactTags(outcomeSummary.nextActions, 4) : '-' },
                    { key: 'source', label: '结果来源', children: outcomeResult ? `${outcomeResult.sourceType || '-'} / ${outcomeResult.sourceRef || '-'}` : '-' },
                    { key: 'evidence', label: '结果证据', children: outcomeResult?.evidenceRefs?.length ? compactTags(outcomeResult.evidenceRefs) : '-' }
                  ]}
                />
                <ProCard gutter={16} wrap>
                  <ProCard title={modelLabels.doDAssessment} colSpan={{ xs: 24, lg: 12 }}>
                    {outcomeDoDItems.length ? (
                      <ProTable
                        rowKey="key"
                        search={false}
                        options={false}
                        pagination={false}
                        size="small"
                        columns={[
                          { title: modelLabels.doDItem, dataIndex: 'item', ellipsis: true },
                          { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(String(item.state)) },
                          { title: '证据', dataIndex: 'evidenceRefs', ellipsis: true, render: (_, item) => compactTags((item.evidenceRefs as string[]) ?? [], 4) },
                          { title: '未满足原因', dataIndex: 'failureReason', ellipsis: true, render: (_, item) => item.failureReason || '-' }
                        ]}
                        dataSource={outcomeDoDItems}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}尚未输出${modelLabels.doDAssessment}`} />
                    )}
                  </ProCard>
                  <ProCard title={modelLabels.artifact} colSpan={{ xs: 24, lg: 12 }}>
                    {outcomeArtifacts.length > 0 ? (
                      <ProTable<Artifact>
                        rowKey="id"
                        search={false}
                        options={false}
                        pagination={false}
                        size="small"
                        columns={[
                          { title: '名称/URI', ellipsis: true, render: (_, item) => item.name || item.uri || item.id },
                          { title: '类型', dataIndex: 'artifactType', width: 120 },
                          { title: '分类', dataIndex: 'category', width: 130, render: (_, item) => item.category || '-' },
                          { title: '状态', dataIndex: 'state', width: 110, render: (_, item) => stateTag(item.state) },
                          { title: '摘要', dataIndex: 'summary', ellipsis: true },
                          {
                            title: '操作',
                            valueType: 'option',
                            render: (_, item) => item.uri ? (
                              <Button size="small" type="link" href={workorchApi.artifactContentURL(item.uri)} target="_blank">
                                下载
                              </Button>
                            ) : null
                          }
                        ]}
                        dataSource={outcomeArtifacts}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}尚未形成${modelLabels.artifact}`} />
                    )}
                  </ProCard>
                </ProCard>
                <ProCard gutter={16} wrap>
                  <ProCard title={modelLabels.toolCall} colSpan={{ xs: 24, lg: 12 }}>
                    {(runSnapshot.toolCalls?.length ?? 0) > 0 ? (
                      <ProTable<ToolCall>
                        rowKey="id"
                        search={false}
                        options={false}
                        pagination={{ pageSize: 5 }}
                        size="small"
                        columns={[
                          { title: '工具', dataIndex: 'toolName', ellipsis: true },
                          { title: '状态', dataIndex: 'state', width: 110, render: (_, item) => stateTag(item.state) },
                          { title: '结果摘要', dataIndex: 'exitSummary', ellipsis: true },
                          { title: '错误', dataIndex: 'error', ellipsis: true }
                        ]}
                        dataSource={runSnapshot.toolCalls ?? []}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}未产生${modelLabels.toolCall}`} />
                    )}
                  </ProCard>
                  <ProCard title={modelLabels.memory} colSpan={{ xs: 24, lg: 12 }}>
                    {outcomeMemories.length ? (
                      <ProTable<MemoryItem>
                        rowKey="id"
                        search={false}
                        options={false}
                        pagination={{ pageSize: 5 }}
                        size="small"
                        columns={[
                          { title: '记忆摘要', dataIndex: 'summary', ellipsis: true },
                          { title: '记忆类型', dataIndex: 'memoryType', width: 130 },
                          { title: '来源类型', dataIndex: 'sourceType', width: 110 },
                          { title: '向量状态', width: 100, render: (_, item) => item.vectorReady ? <Tag color="cyan">{item.vectorDimension}</Tag> : '-' }
                        ]}
                        dataSource={outcomeMemories}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}未产生或引用${modelLabels.memory}`} />
                    )}
                  </ProCard>
                  <ProCard title={modelLabels.memoryRecall} colSpan={{ xs: 24, lg: 12 }}>
                    {memoryRecallCard(runSnapshot.memoryRecallTraces)}
                  </ProCard>
                  <ProCard title="Web 来源" colSpan={{ xs: 24, lg: 12 }}>
                    {webSourceCard(runWebSources)}
                  </ProCard>
                  <ProCard title="LLM Context Budget" colSpan={{ xs: 24, lg: 12 }}>
                    {llmContextBudgetCard(runLLMContextBudgets)}
                  </ProCard>
                  <ProCard title={modelLabels.llmUsage} colSpan={{ xs: 24, lg: 12 }}>
                    {llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)}
                  </ProCard>
                </ProCard>
              </Space>
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请先启动或选择一个运行（Run）查看目标成果" />
            )}
          </ProCard>
          <ProCard title="运行记录（Run）" className="workorch-section">
            <ProTable<Run> rowKey="id" search={false} options={false} pagination={false} columns={runColumns} dataSource={snapshot.runs ?? []} />
          </ProCard>
          <ProCard
            title="任务进度与恢复"
            className="workorch-section"
            extra={stateTag((runObservability?.taskProgress ?? runSnapshot?.taskProgress ?? snapshot.taskProgress)?.state)}
          >
            {taskProgressCard(
              runObservability?.taskProgress ?? runSnapshot?.taskProgress ?? snapshot.taskProgress,
              runObservability?.repairLoop ?? runSnapshot?.repairLoop
            )}
          </ProCard>
          <ProCard
            title="任务拆分图"
            className="workorch-section"
            extra={<Tag>{(runObservability?.taskDecompositions ?? runSnapshot?.taskDecompositions ?? snapshot.taskDecompositions ?? []).length} 个图</Tag>}
          >
            {taskDecompositionCard(runObservability?.taskDecompositions ?? runSnapshot?.taskDecompositions ?? snapshot.taskDecompositions)}
          </ProCard>
          <ProCard
            title="执行分区"
            className="workorch-section"
            extra={<Tag>{(runObservability?.executionSections ?? runSnapshot?.executionSections ?? snapshot.executionSections ?? []).length} 个分区</Tag>}
          >
            {executionSectionsCard(runObservability?.executionSections ?? runSnapshot?.executionSections ?? snapshot.executionSections)}
          </ProCard>
          <ProCard
            title="上下文恢复包"
            className="workorch-section"
            extra={<Tag>{contextBundle?.mode || '未生成'}</Tag>}
          >
            {contextBundleCard(contextBundle)}
          </ProCard>
          <ProCard gutter={16} wrap className="workorch-section">
            <ProCard title="决策工单（Decision）" colSpan="50%">
              <ProTable<Decision>
                rowKey="id"
                search={false}
                options={false}
                pagination={false}
                columns={[
                  { title: '工单', dataIndex: 'id', copyable: true },
                  { title: '类型', dataIndex: 'decisionKind', width: 150 },
                  { title: '问题', dataIndex: 'question', ellipsis: true, render: (_, item) => boundedParagraph(item.question, { rows: 2 }) },
                  { title: '原因', dataIndex: 'reason', ellipsis: true },
                  { title: '自动治理', dataIndex: 'autoGovernance', render: (_, item) => item.autoGovernance ? stateTag(`${item.autoGovernance.status}:${item.autoGovernance.action}`) : '-' },
                  { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
                  { title: '解决方式', dataIndex: 'resolutionType' }
                ]}
                dataSource={snapshot.decisions ?? []}
                expandable={{
                  expandedRowRender: (item) => (
                    <ProCard gutter={16} wrap ghost>
                      <ProCard title="上下文" colSpan={{ xs: 24, lg: 12 }}>
                        <pre className="json-panel json-panel-large">{compactJSON(item.context)}</pre>
                      </ProCard>
                      <ProCard title="解决内容" colSpan={{ xs: 24, lg: 12 }}>
                        <pre className="json-panel json-panel-large">{compactJSON(item.resolutionPayload)}</pre>
                      </ProCard>
                      <ProCard title="自动治理摘要" colSpan={{ xs: 24, lg: 12 }}>
                        <pre className="json-panel json-panel-large">{compactJSON(item.autoGovernance)}</pre>
                      </ProCard>
                    </ProCard>
                  )
                }}
              />
            </ProCard>
            <ProCard title="审批工单（Approval）" colSpan="50%">
              <ProTable<Approval>
                rowKey="id"
                search={false}
                options={false}
                pagination={false}
                columns={[
                  { title: '审批', dataIndex: 'id', copyable: true },
                  { title: '运行', dataIndex: 'runId', copyable: true, ellipsis: true },
                  { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
                  { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
                  { title: '原因', dataIndex: 'reason' },
                  { title: '自动治理', dataIndex: 'autoGovernance', render: (_, item) => item.autoGovernance ? stateTag(`${item.autoGovernance.status}:${item.autoGovernance.action}`) : '-' }
                ]}
                dataSource={snapshot.approvals ?? []}
              />
            </ProCard>
          </ProCard>
          <ProCard title="运行过程与证据" className="workorch-section" extra={runSnapshot ? <Tag>{evidenceCount(runSnapshot)} 条证据</Tag> : null}>
            {runSnapshot ? (
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <Steps
                  size="small"
                  items={[
                    { title: '调度', description: stateLabel(runOutcomeState(runSnapshot, runObservability)), status: 'finish' },
                    { title: '步骤', description: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0}`, status: runObservability?.progress.stepFailed ? 'error' : 'finish' },
                    { title: '工具调用', description: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runSnapshot.toolCalls?.length ?? 0}`, status: 'finish' },
                    { title: '校验', description: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? runSnapshot.validationResults?.length ?? 0}`, status: runOutcomeState(runSnapshot, runObservability) === 'failed' ? 'error' : 'finish' },
                    { title: '终态', description: runObservability?.progress.terminal ? '已进入终态' : '尚未终态', status: runObservability?.progress.terminal ? 'finish' : 'process' }
                  ]}
                />
                {metricGrid([
                  { key: 'steps', label: '步骤完成', value: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0}`, color: runObservability?.progress.stepFailed ? '#cf1322' : '#0958d9' },
                  { key: 'tools', label: '工具完成', value: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runSnapshot.toolCalls?.length ?? 0}` },
                  { key: 'validations', label: '校验通过', value: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? runSnapshot.validationResults?.length ?? 0}`, color: runObservability?.evidence.validationFailed ? '#cf1322' : '#389e0d' },
                  { key: 'patches', label: '补丁（Patch）应用', value: `${runObservability?.evidence.patchApplied ?? 0}/${runObservability?.evidence.patchTotal ?? runSnapshot.patches?.length ?? 0}` },
                  { key: 'results', label: modelLabels.result, value: runObservability?.evidence.resultTotal ?? runSnapshot.results?.length ?? 0, color: '#389e0d' },
                  { key: 'artifacts', label: modelLabels.artifact, value: runObservability?.evidence.artifactTotal ?? runSnapshot.artifacts?.length ?? 0 },
                  { key: 'webSources', label: 'Web 来源', value: webSourceCount, color: webSourceCount ? '#08979c' : undefined },
                  { key: 'llmContextBudgets', label: 'LLM Context', value: runLLMContextBudgets?.length ?? 0, color: (runLLMContextBudgets?.length ?? 0) > 0 ? '#1677ff' : undefined },
                  { key: 'llmUsage', label: modelLabels.llmUsage, value: runLLMUsageSummary?.entryTotal ?? 0, color: runLLMUsageSummary?.entryTotal ? '#d48806' : undefined }
                ])}
                <Descriptions
                  column={3}
                  items={[
                    { key: 'run', label: '运行', children: `${runSnapshot.run.id} / ${stateLabel(runOutcomeState(runSnapshot, runObservability))}` },
                    { key: 'rawRunState', label: '原始运行状态', children: stateLabel(runRawState(runSnapshot, runObservability)) },
                    { key: 'currentStep', label: '当前步骤', children: runObservability?.progress.currentStepId || runSnapshot.steps?.at(-1)?.id || '-' },
                    { key: 'terminal', label: '是否终态', children: runObservability ? (runObservability.progress.terminal ? '是' : '否') : '-' },
                    { key: 'steps', label: '步骤', children: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0} 已完成` },
                    { key: 'toolCalls', label: '工具调用', children: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runSnapshot.toolCalls?.length ?? 0} 已完成` },
                    { key: 'patches', label: modelLabels.patch, children: `${runObservability?.evidence.patchApplied ?? 0}/${runObservability?.evidence.patchTotal ?? runSnapshot.patches?.length ?? 0} 已应用` },
                    { key: 'validations', label: modelLabels.validationResult, children: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? runSnapshot.validationResults?.length ?? 0} 通过` },
                    { key: 'checkpoints', label: modelLabels.runtimeCheckpoint, children: runObservability?.evidence.runtimeCheckpointTotal ?? runSnapshot.runtimeCheckpoints?.length ?? 0 },
                    { key: 'results', label: modelLabels.result, children: runObservability?.evidence.resultTotal ?? runSnapshot.results?.length ?? 0 },
                    { key: 'artifacts', label: modelLabels.artifact, children: runObservability?.evidence.artifactTotal ?? runSnapshot.artifacts?.length ?? 0 },
                    { key: 'webSources', label: 'Web 来源', children: webSourceCount },
                    { key: 'llmContextBudgets', label: 'LLM Context', children: runLLMContextBudgets?.length ?? 0 },
                    { key: 'llmUsage', label: modelLabels.llmUsage, children: `${numberText(runLLMUsageSummary?.entryTotal)} 次 / ${numberText(runLLMUsageSummary?.totalTokens)} tokens / ${llmUsageCostSummary(runLLMUsageSummary)}` },
                    { key: 'selectedCapabilities', label: '选择能力', children: runObservability?.selectedCapabilities?.length ?? runSnapshot.selectedCapabilities?.length ?? 0 },
                    { key: 'usedCapabilities', label: '使用能力', children: runObservability?.usedCapabilities?.length ?? runSnapshot.usedCapabilities?.length ?? 0 },
                    { key: 'events', label: '事件', children: runObservability?.evidence.eventTotal ?? 0 },
                    { key: 'context', label: '执行上下文', children: <pre className="json-panel">{compactJSON(runSnapshot.run.executionContext)}</pre> }
                  ]}
                />
                <ProCard gutter={16} wrap>
                  <ProCard title="已选择 MCP 能力" colSpan={{ xs: 24, lg: 12 }}>
                    {runtimeCapabilityTable(runObservability?.selectedCapabilities ?? runSnapshot.selectedCapabilities, `当前${modelLabels.run}未显式选择${modelLabels.mcpCapability}`)}
                  </ProCard>
                  <ProCard title="实际使用 MCP 能力" colSpan={{ xs: 24, lg: 12 }}>
                    {runtimeCapabilityTable(runObservability?.usedCapabilities ?? runSnapshot.usedCapabilities, `当前${modelLabels.run}未产生${modelLabels.mcpCapability}使用证据`)}
                  </ProCard>
                </ProCard>
                <ProCard
                  size="small"
                  title="最新检查点"
                  className="workorch-checkpoint-card"
                  extra={runObservability?.latestCheckpoint ? stateTag(runObservability.latestCheckpoint.runStateSnapshot) : <Tag>暂无</Tag>}
                >
                  {runObservability?.latestCheckpoint ? (
                    <Descriptions
                      size="small"
                      column={{ xs: 1, sm: 2, md: 4 }}
                      items={[
                        { key: 'type', label: '检查点类型', children: checkpointTypeLabel(runObservability.latestCheckpoint.checkpointType) },
                        { key: 'state', label: '运行状态', children: stateLabel(runObservability.latestCheckpoint.runStateSnapshot) },
                        { key: 'step', label: '当前步骤', children: boundedParagraph(runObservability.latestCheckpoint.currentStepId, { rows: 1, copyText: runObservability.latestCheckpoint.currentStepId }) },
                        { key: 'blocked', label: '阻塞信息', children: runObservability.latestCheckpoint.blockingTicketId || runObservability.latestCheckpoint.blockingApprovalId || '无阻塞' }
                      ]}
                    />
                  ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`当前${modelLabels.run}尚未记录${modelLabels.runtimeCheckpoint}`} />
                  )}
                </ProCard>
                <Tabs
                  items={[
                    {
                      key: 'timeline',
                      label: `时间线 ${runObservability?.timeline?.length ?? 0}`,
                      children: (
                        <ProTable<RuntimeTimelineItem>
                          rowKey={(item, index) => `${item.source}-${item.id || item.topic || index}-${index}`}
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: '来源', dataIndex: 'source', width: 110 },
                            { title: '事件', render: (_, item) => timelineLabel(item), ellipsis: true },
                            { title: '运行状态', dataIndex: 'runState', width: 130, render: (_, item) => stateLabel(item.runState) },
                            { title: '步骤', dataIndex: 'stepId', ellipsis: true },
                            { title: '摘要', dataIndex: 'summary', ellipsis: true },
                            { title: '流式事件', render: (_, item) => timelineStreamContent(item), ellipsis: true },
                            { title: '发生时间', dataIndex: 'occurredAt', width: 210, render: (_, item) => timelineOccurredAtContent(item) }
                          ]}
                          expandable={{
                            expandedRowRender: (item) => <pre className="json-panel json-panel-large">{compactJSON(item.output ?? item.data)}</pre>
                          }}
                          dataSource={runObservability?.timeline ?? []}
                        />
                      )
                    },
                    {
                      key: 'webSources',
                      label: `Web 来源 ${webSourceCount}`,
                      children: webSourceCard(runWebSources)
                    },
                    {
                      key: 'llmContextBudgets',
                      label: `LLM Context ${runLLMContextBudgets?.length ?? 0}`,
                      children: llmContextBudgetCard(runLLMContextBudgets)
                    },
                    {
                      key: 'llmUsage',
                      label: `LLM 使用量 ${runLLMUsageSummary?.entryTotal ?? 0}`,
                      children: llmUsageCard(runLLMUsageSummary, runLLMUsageTraces)
                    },
                    {
                      key: 'steps',
                      label: `步骤 ${runSnapshot.steps?.length ?? 0}`,
                      children: (
                        <ProTable<StepView>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: '步骤', dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '类型', dataIndex: 'kind', width: 180 },
                            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                            { title: '操作', render: (_, item) => outputText(item.output, 'operation'), ellipsis: true },
                            { title: '服务', render: (_, item) => outputText(item.output, 'serverId'), ellipsis: true },
                            { title: '目标', render: (_, item) => runtimeTarget(item.output), ellipsis: true },
                            { title: '阶段', render: (_, item) => outputText(item.output, 'stage'), ellipsis: true },
                            { title: '下一类型', render: (_, item) => outputText(item.output, 'nextKind'), ellipsis: true }
                          ]}
                          expandable={{
                            expandedRowRender: (item) => <pre className="json-panel json-panel-large">{compactJSON(item.output)}</pre>
                          }}
                          dataSource={runSnapshot.steps ?? []}
                        />
                      )
                    },
                    {
                      key: 'toolCalls',
                      label: `工具调用 ${runSnapshot.toolCalls?.length ?? 0}`,
                      children: (
                        <ProTable<ToolCall>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: modelLabels.toolCall, dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '工具', dataIndex: 'toolName', ellipsis: true },
                            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                            { title: '摘要', dataIndex: 'exitSummary', ellipsis: true },
                            { title: '错误', dataIndex: 'error', ellipsis: true }
                          ]}
                          dataSource={runSnapshot.toolCalls ?? []}
                        />
                      )
                    },
                    {
                      key: 'validations',
                      label: `校验结果 ${runSnapshot.validationResults?.length ?? 0}`,
                      children: (
                        <ProTable<ValidationResult>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: modelLabels.validationResult, dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '类型', dataIndex: 'validationKind', width: 160 },
                            { title: '目标', dataIndex: 'targetRef', ellipsis: true },
                            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                            { title: '摘要', dataIndex: 'summary', ellipsis: true }
                          ]}
                          dataSource={runSnapshot.validationResults ?? []}
                        />
                      )
                    },
                    {
                      key: 'patches',
                      label: `${modelLabels.patch} ${runSnapshot.patches?.length ?? 0}`,
                      children: (
                        <ProTable<Patch>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: modelLabels.patch, dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '类型', dataIndex: 'patchType', width: 140 },
                            { title: '目标', dataIndex: 'targetRef', ellipsis: true },
                            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                            { title: '增删行', width: 100, render: (_, item) => `+${item.addedLines ?? 0}/-${item.deletedLines ?? 0}` },
                            { title: '审批', dataIndex: 'approvalId', copyable: true, ellipsis: true, render: (_, item) => item.approvalId || item.revertApprovalId || '-' },
                            { title: '摘要', dataIndex: 'summary', ellipsis: true }
                          ]}
                          dataSource={runSnapshot.patches ?? []}
                        />
                      )
                    },
                    {
                      key: 'checkpoints',
                      label: `${modelLabels.runtimeCheckpoint} ${runSnapshot.runtimeCheckpoints?.length ?? 0}`,
                      children: (
                        <ProTable<RuntimeCheckpoint>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: modelLabels.runtimeCheckpoint, dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '类型', dataIndex: 'checkpointType', width: 180 },
                            { title: '运行状态', dataIndex: 'runStateSnapshot', width: 140, render: (_, item) => stateTag(item.runStateSnapshot) },
                            { title: '当前步骤', dataIndex: 'currentStepId', ellipsis: true },
                            { title: '决策', dataIndex: 'blockingTicketId', ellipsis: true },
                            { title: '审批', dataIndex: 'blockingApprovalId', ellipsis: true }
                          ]}
                          dataSource={runSnapshot.runtimeCheckpoints ?? []}
                        />
                      )
                    },
                    {
                      key: 'results',
                      label: `结果 ${runSnapshot.results?.length ?? 0}`,
                      children: (
                        <ProTable<Result>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: modelLabels.result, dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                            { title: '标题', dataIndex: 'title', ellipsis: true },
                            { title: '摘要', dataIndex: 'summary', ellipsis: true },
                            { title: '来源', render: (_, item) => `${item.sourceType || '-'} / ${item.sourceRef || '-'}`, ellipsis: true }
                          ]}
                          dataSource={runSnapshot.results ?? []}
                        />
                      )
                    },
                    {
                      key: 'artifacts',
                      label: `交付物 ${runSnapshot.artifacts?.length ?? 0}`,
                      children: (
                        <ProTable<Artifact>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: modelLabels.artifact, dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '类型', dataIndex: 'artifactType', width: 140 },
                            { title: '分类', dataIndex: 'category', width: 140, render: (_, item) => item.category || '-' },
                            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                            { title: '名称/URI', render: (_, item) => item.name || item.uri || '-', ellipsis: true },
                            { title: '摘要', dataIndex: 'summary', ellipsis: true },
                            {
                              title: '操作',
                              valueType: 'option',
                              render: (_, item) => item.uri ? (
                                <Button size="small" type="link" href={workorchApi.artifactContentURL(item.uri)} target="_blank">
                                  下载
                                </Button>
                              ) : null
                            }
                          ]}
                          dataSource={runSnapshot.artifacts ?? []}
                        />
                      )
                    }
                  ]}
                />
              </Space>
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择一个运行记录查看过程和证据" />
            )}
          </ProCard>
        </>
      ) : (
        <Empty description={loading ? '正在加载工作空间详情' : '未选择工作空间'} />
      )}
      <Modal
        title="调整工作空间执行人格（Soul）"
        open={soulBindingEditorOpen}
        confirmLoading={soulBindingSaving}
        onCancel={() => setSoulBindingEditorOpen(false)}
        onOk={() => soulBindingsForm.submit()}
        okText="保存执行配置"
        cancelText="取消"
        width={760}
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Typography.Paragraph type="secondary">
            执行人格（Soul）是工作空间的执行配置。先维护目标（Goal）和验收标准（DoD），再按需设置默认执行人格与阶段覆盖；启动执行时仍可做单次覆盖。
          </Typography.Paragraph>
          <Descriptions
            size="small"
            column={1}
            items={[
              { key: 'workspace', label: '工作空间', children: snapshot?.workspace ? `${snapshot.workspace.title || snapshot.workspace.id} (${snapshot.workspace.id})` : '-' },
              { key: 'priority', label: '启用优先级', children: compactTags(soulBindings?.priorityPolicy ?? []) }
            ]}
          />
          {souls.length ? (
            <Form
              form={soulBindingsForm}
              layout="vertical"
              onFinish={(values) => void saveWorkspaceSoulBindings(values as { defaultSoulId?: string; bindings?: Record<string, string> })}
            >
              <Form.Item name="defaultSoulId" label="工作空间默认执行人格（Soul）">
                <Select allowClear placeholder="未绑定时不注入默认执行人格" options={soulOptions} />
              </Form.Item>
              <Space className="workorch-section">
                <Typography.Text type="secondary">阶段执行人格绑定</Typography.Text>
                <Button
                  size="small"
                  onClick={() => soulBindingsForm.setFieldsValue({ bindings: stageBindings(soulBindingStages, soulBindingsForm.getFieldValue('defaultSoulId')) })}
                >
                  默认执行人格应用到全部阶段
                </Button>
              </Space>
              <div className="soul-binding-grid">
                {soulBindingStages.map((stage) => (
                  <Form.Item key={stage} name={['bindings', stage]} label={soulBindingStageLabel(stage)}>
                    <Select allowClear placeholder="继承默认执行人格" options={soulOptions} />
                  </Form.Item>
                ))}
              </div>
            </Form>
          ) : (
            <Empty description="暂无可用执行人格，可先到系统配置创建执行人格" />
          )}
        </Space>
      </Modal>
      <Modal
        title="创建工作空间"
        open={workspaceSetupOpen}
        confirmLoading={businessSubmitting}
        onCancel={closeWorkspaceSetup}
        width={820}
        footer={[
          <Button key="cancel" onClick={closeWorkspaceSetup}>取消</Button>,
          setupStep > 0 ? <Button key="previous" onClick={goPreviousWorkspaceSetupStep}>上一步</Button> : null,
          setupStep < 4 ? <Button key="next" type="primary" onClick={() => void goNextWorkspaceSetupStep()}>下一步</Button> : null,
          setupStep === 4 ? <Button key="submit" type="primary" loading={businessSubmitting} onClick={() => void submitWorkspaceSetup()}>确认提交</Button> : null
        ].filter(Boolean)}
      >
        <Steps
          current={setupStep}
          className="workorch-section"
          items={[
            { title: '工作空间' },
            { title: '目标' },
            { title: '验收标准' },
            { title: '执行人格' },
            { title: '确认提交' }
          ]}
        />
        <Form
          form={workspaceForm}
          layout="vertical"
          style={{ display: setupStep === 0 ? 'block' : 'none' }}
        >
          <Form.Item name="templateId" label="工作空间模板（Workspace Template）">
            <Select
              allowClear
              placeholder="可选。选择后自动带入目标、验收标准和执行人格绑定"
              onChange={applyWorkspaceTemplate}
              options={workspaceTemplates.map((item) => ({ value: item.id, label: `${item.name || item.id} (${item.id})` }))}
            />
          </Form.Item>
          <Form.Item name="title" label="标题" rules={[{ required: true }]}>
            <Input placeholder="本地 workorch 工作空间" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} placeholder="描述工作空间目标、范围或当前交付预期" />
          </Form.Item>
          <Form.Item name="workingDirectory" label="工作目录">
            <Input placeholder="留空时自动使用 repository.dataDir/workspaces/工作空间 ID/working" />
          </Form.Item>
        </Form>
        <Form
          form={setupGoalForm}
          layout="vertical"
          style={{ display: setupStep === 1 ? 'block' : 'none' }}
        >
          <Form.Item label="工作空间">
            <Input value={workspaceForm.getFieldValue('title') || '待创建，最终确认提交后生成'} disabled />
          </Form.Item>
          <Form.Item name="text" label="目标" rules={[{ required: true }]}>
            <Input.TextArea rows={4} placeholder="描述该工作空间要达成的业务结果" />
          </Form.Item>
        </Form>
        <Form
          form={setupDoDForm}
          layout="vertical"
          style={{ display: setupStep === 2 ? 'block' : 'none' }}
        >
          <Form.Item label="工作空间">
            <Input value={workspaceForm.getFieldValue('title') || '待创建，最终确认提交后生成'} disabled />
          </Form.Item>
          <Form.Item name="items" label="验收项" rules={[{ required: true }]}>
            <Input.TextArea rows={5} placeholder={'每行一个验收标准\n示例：服务已完成本地部署并可访问'} />
          </Form.Item>
        </Form>
        <Form
          form={setupSoulBindingsForm}
          layout="vertical"
          style={{ display: setupStep === 3 ? 'block' : 'none' }}
        >
          <Form.Item label="工作空间">
            <Input value={workspaceForm.getFieldValue('title') || '待创建，最终确认提交后生成'} disabled />
          </Form.Item>
          <Typography.Paragraph type="secondary">
            可选。用于设置该工作空间默认执行人格（Soul）和各任务阶段执行人格；后续可在工作空间详情中修改。
          </Typography.Paragraph>
          {souls.length ? (
            <>
              <Form.Item name="defaultSoulId" label="工作空间默认执行人格（Soul）">
                <Select allowClear placeholder="未绑定时不注入默认执行人格" options={soulOptions} />
              </Form.Item>
              <Space className="workorch-section">
                <Typography.Text type="secondary">阶段执行人格绑定</Typography.Text>
                <Button
                  size="small"
                  onClick={() => setupSoulBindingsForm.setFieldsValue({ bindings: stageBindings(soulBindingStages, setupSoulBindingsForm.getFieldValue('defaultSoulId')) })}
                >
                  默认执行人格应用到全部阶段
                </Button>
              </Space>
              <div className="soul-binding-grid">
                {soulBindingStages.map((stage) => (
                  <Form.Item key={stage} name={['bindings', stage]} label={soulBindingStageLabel(stage)}>
                    <Select allowClear placeholder="继承默认执行人格" options={soulOptions} />
                  </Form.Item>
                ))}
              </div>
            </>
          ) : (
            <Empty description="暂无执行人格，可先到系统配置创建，或直接完成后续配置" />
          )}
        </Form>
        <div style={{ display: setupStep === 4 ? 'block' : 'none' }}>
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Typography.Paragraph type="secondary">
              请确认以下信息。点击“确认提交”后，workorch 会按顺序创建工作空间（Workspace）、写入目标（Goal）、写入验收标准（DoD），并保存工作空间执行人格（Soul）配置。
            </Typography.Paragraph>
            <Descriptions
              bordered
              column={1}
              size="small"
              items={[
                { key: 'title', label: '工作空间', children: workspaceForm.getFieldValue('title') || '-' },
                { key: 'description', label: '描述', children: boundedParagraph(workspaceForm.getFieldValue('description'), { rows: 2 }) },
                { key: 'workingDirectory', label: '工作目录', children: workspaceForm.getFieldValue('workingDirectory') || '自动生成' },
                { key: 'goal', label: '目标', children: boundedParagraph(setupGoalForm.getFieldValue('text'), { rows: 3 }) },
                { key: 'dod', label: '验收标准', children: compactTags(setupDoDItems(setupDoDForm.getFieldValue('items')), 6) },
                {
                  key: 'defaultSoul',
                  label: '默认执行人格',
                  children: setupSoulBindingsForm.getFieldValue('defaultSoulId')
                    ? soulNameById.get(setupSoulBindingsForm.getFieldValue('defaultSoulId')) || setupSoulBindingsForm.getFieldValue('defaultSoulId')
                    : '未配置'
                },
                {
                  key: 'stageSouls',
                  label: '阶段执行人格',
                  children: compactTags(
                    Object.entries(setupSoulBindingsForm.getFieldValue('bindings') ?? {})
                      .filter(([, soulId]) => Boolean(soulId))
                      .map(([stage, soulId]) => `${soulBindingStageLabel(stage)}: ${soulNameById.get(String(soulId)) || soulId}`),
                    6
                  )
                }
              ]}
            />
          </Space>
        </div>
      </Modal>
      <Modal
        title="删除工作空间"
        open={workspaceDeleteOpen}
        okText="确认删除"
        okButtonProps={{ danger: true }}
        confirmLoading={workspaceDeleting}
        onCancel={() => setWorkspaceDeleteOpen(false)}
        onOk={() => void submitWorkspaceDelete()}
      >
        <Form form={workspaceDeleteForm} layout="vertical" initialValues={{ purge: false }}>
          <Typography.Paragraph>
            确认删除「{snapshot?.workspace.title || workspaceId}」？默认仅删除工作空间实体记录，不会清理运行数据或工作目录。
          </Typography.Paragraph>
          <Form.Item name="purge" valuePropName="checked">
            <Checkbox>
              同时删除该工作空间的目标、验收标准、运行数据、治理记录、交付物索引、记忆（Memory）和工作目录
            </Checkbox>
          </Form.Item>
          <Typography.Paragraph type="secondary">
            勾选后会清理 file backend 下该工作空间的数据目录，并删除工作空间（Workspace）的工作目录。该操作不可恢复。
          </Typography.Paragraph>
        </Form>
      </Modal>
      <Modal
        title={editingGoal ? '修改目标' : '定义目标'}
        open={goalDefinitionOpen}
        confirmLoading={businessSubmitting}
        onCancel={() => {
          setGoalDefinitionOpen(false);
          setEditingGoal(undefined);
          goalForm.resetFields();
        }}
        onOk={() => goalForm.submit()}
      >
        <Form
          form={goalForm}
          layout="vertical"
          onFinish={async (values: { text: string }) => {
            if (!workspaceId) {
              message.error('请先选择工作空间');
              return;
            }
            setBusinessSubmitting(true);
            try {
              if (editingGoal) {
                await workorchApi.updateGoal(editingGoal.id, values.text);
              } else {
                await workorchApi.defineGoal(workspaceId, values.text);
              }
              await loadWorkspaceContext(workspaceId);
              setGoalDefinitionOpen(false);
              setEditingGoal(undefined);
              goalForm.resetFields();
              message.success(editingGoal ? '目标已修改' : '目标已定义');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item name="text" label="目标" rules={[{ required: true }]}>
            <Input.TextArea rows={4} placeholder="描述该工作空间要达成的业务结果" />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title={editingDoD ? '修改验收标准' : '定义验收标准'}
        open={dodDefinitionOpen}
        confirmLoading={businessSubmitting}
        onCancel={() => {
          setDoDDefinitionOpen(false);
          setEditingDoD(undefined);
          dodForm.resetFields();
        }}
        onOk={() => dodForm.submit()}
      >
        <Form
          form={dodForm}
          layout="vertical"
          onFinish={async (values: { items: string }) => {
            if (!workspaceId) {
              message.error('请先选择工作空间');
              return;
            }
            setBusinessSubmitting(true);
            try {
              const items = values.items.split('\n').map((item) => item.trim()).filter(Boolean);
              if (editingDoD) {
                await workorchApi.updateDoD(editingDoD.id, items);
              } else {
                await workorchApi.defineDoD(workspaceId, items);
              }
              await loadWorkspaceContext(workspaceId);
              setDoDDefinitionOpen(false);
              setEditingDoD(undefined);
              dodForm.resetFields();
              message.success(editingDoD ? '验收标准已修改' : '验收标准已定义');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item name="items" label="验收项" rules={[{ required: true }]}>
            <Input.TextArea rows={5} placeholder={'每行一个验收标准\n示例：服务已完成本地部署并可访问'} />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title="启动工作空间执行"
        open={activationOpen}
        confirmLoading={activationLoading}
        onCancel={() => setActivationOpen(false)}
        onOk={() => activationForm.submit()}
        okText="提交后台执行"
      >
        <Form
          form={activationForm}
          layout="vertical"
          onFinish={(values: WorkspaceExecutionActivationRequest) => void activateWorkspaceExecution(values)}
        >
          <Form.Item name="goalId" label="目标" rules={[{ required: true }]}>
            <Select options={goals.map((item) => ({ value: item.id, label: `${item.text} (${item.id})` }))} />
          </Form.Item>
          <Form.Item name="doDId" label="验收标准" rules={[{ required: true }]}>
            <Select options={readyDoDs.map((item) => ({ value: item.id, label: `${item.items.join(' / ')} (${item.id})` }))} />
          </Form.Item>
          <Form.Item name="llmProviderId" label="LLM 服务商">
            <Select
              allowClear
              placeholder="使用系统默认 LLM 服务商"
              options={llmProviders.map((item) => ({
                value: item.id,
                label: `${item.default ? '[默认] ' : ''}${item.id} / ${item.model || item.provider || '未配置'}${item.ready ? '' : ` / ${item.reason || '不可用'}`}`
              }))}
            />
          </Form.Item>
          <Form.Item name="soulId" label="本次默认执行人格（Soul）">
            <Select allowClear placeholder="继承工作空间默认和阶段执行人格配置" options={soulOptions} />
          </Form.Item>
          <Typography.Text type="secondary">本次执行阶段执行人格覆盖</Typography.Text>
          <div className="soul-binding-grid soul-binding-grid-compact">
            {soulBindingStages.map((stage) => (
              <Form.Item key={stage} name={['soulBindings', stage]} label={soulBindingStageLabel(stage)}>
                <Select allowClear placeholder="继承工作空间配置" options={soulOptions} />
              </Form.Item>
            ))}
          </div>
          <Form.Item name="skillIds" label="可用技能（Skill）">
            <Select
              mode="multiple"
              allowClear
              options={skills.map((item) => ({
                value: item.id,
                label: skillDisplayLabel(item)
              }))}
            />
          </Form.Item>
          <Form.Item name="mcpServerIds" label="MCP 服务">
            <Select
              mode="multiple"
              allowClear
              options={mcpServers.map((item) => ({
                value: item.id,
                label: `${item.name || item.id} / ${item.transport}${item.enabled ? '' : ' / 已禁用'}`
              }))}
            />
          </Form.Item>
          <Form.Item name="capabilityIds" label="MCP 能力（Tool / Resource / Prompt）">
            <Select
              mode="multiple"
              allowClear
              showSearch
              optionFilterProp="label"
              placeholder="可按业务能力细粒度限制本次执行可用范围"
              options={mcpCapabilities.map((item) => ({
                value: item.id,
                label: `${item.displayName || item.name} / ${item.kind} / ${item.name}`
              }))}
            />
          </Form.Item>
          <Form.Item name="memoryIds" label="记忆（Memory）">
            <Select
              mode="multiple"
              allowClear
              placeholder="留空时由 workorch 自动召回工作空间记忆"
              options={memoryItems.map((item) => ({
                value: item.id,
                label: `${item.memoryType}: ${item.summary}${item.vectorReady ? ` / vector:${item.vectorDimension ?? 0}` : ''}`
              }))}
            />
          </Form.Item>
          <Form.Item name="priority" label="运行队列优先级">
            <InputNumber style={{ width: '100%' }} placeholder="0" />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
}

type MCPServerFormValues = {
  id: string;
  name: string;
  transport: string;
  endpoint?: string;
  enabled: boolean;
  allowedTools?: string;
  allowedResources?: string;
  allowedPrompts?: string;
  defaultApprovalPolicy?: string;
  sessionPolicy?: string;
  timeoutMillis?: number;
};

type MCPCapabilityFilterValues = {
  q?: string;
  serverId?: string;
  kind?: string;
  category?: string;
  stage?: string;
  riskLevel?: string;
  enabled?: boolean;
};

type MCPCapabilityFormValues = {
  displayName?: string;
  description?: string;
  category?: string;
  tags?: string[];
  usageHint?: string;
  inputSummary?: string;
  outputSummary?: string;
  stageHints?: string[];
  riskLevel?: string;
  enabled?: boolean;
  approvalPolicy?: string;
  status?: string;
};

function mcpServerPayload(values: MCPServerFormValues): MCPServer {
  return {
    id: values.id.trim(),
    name: values.name.trim(),
    transport: values.transport,
    endpoint: values.endpoint?.trim(),
    enabled: values.enabled !== false,
    allowedTools: splitLines(values.allowedTools),
    allowedResources: splitLines(values.allowedResources),
    allowedPrompts: splitLines(values.allowedPrompts),
    defaultApprovalPolicy: values.defaultApprovalPolicy?.trim(),
    sessionPolicy: values.sessionPolicy?.trim(),
    timeoutMillis: values.timeoutMillis
  };
}

function displayMCPEndpoint(endpoint?: string) {
  const value = endpoint?.trim() ?? '';
  const marker = '.workorch-data/system/mcp-endpoints/';
  const index = value.indexOf(marker);
  return index >= 0 ? value.slice(index) : value;
}

function RuntimeControlPage() {
  const { message } = AntApp.useApp();
  const [serverForm] = Form.useForm<MCPServerFormValues>();
  const [capabilityFilterForm] = Form.useForm<MCPCapabilityFilterValues>();
  const [capabilityForm] = Form.useForm<MCPCapabilityFormValues>();
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [selectedServerId, setSelectedServerId] = useState<string | undefined>(undefined);
  const [tools, setTools] = useState<MCPTool[]>([]);
  const [resources, setResources] = useState<MCPResource[]>([]);
  const [prompts, setPrompts] = useState<MCPPrompt[]>([]);
  const [capabilities, setCapabilities] = useState<MCPCapability[]>([]);
  const [editingCapability, setEditingCapability] = useState<MCPCapability | undefined>(undefined);
  const [capabilityModalOpen, setCapabilityModalOpen] = useState(false);
  const [metrics, setMetrics] = useState('');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const selectedServer = servers.find((item) => item.id === selectedServerId);

  const loadCapabilityDirectory = async (values?: MCPCapabilityFilterValues) => {
    const query = values ?? capabilityFilterForm.getFieldsValue();
    setCapabilities(await workorchApi.listMCPCapabilities({
      serverId: query.serverId,
      kind: query.kind,
      category: query.category,
      stage: query.stage,
      riskLevel: query.riskLevel,
      enabled: query.enabled,
      q: query.q
    }));
  };

  const loadRuntime = async () => {
    setLoading(true);
    try {
      const [nextServers, nextMetrics] = await Promise.all([
        workorchApi.listMCPServers(),
        workorchApi.metrics()
      ]);
      setServers(nextServers);
      setMetrics(nextMetrics);
      await loadCapabilityDirectory();
      if (!selectedServerId && nextServers[0]) {
        setSelectedServerId(nextServers[0].id);
      }
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  };

  const loadCapabilities = async (serverId: string) => {
    try {
      const [nextTools, nextResources, nextPrompts, nextCapabilities] = await Promise.all([
        workorchApi.listMCPServerTools(serverId),
        workorchApi.listMCPServerResources(serverId),
        workorchApi.listMCPServerPrompts(serverId),
        workorchApi.listMCPCapabilities({ serverId })
      ]);
      setTools(nextTools);
      setResources(nextResources);
      setPrompts(nextPrompts);
      setCapabilities((previous) => {
        const retained = previous.filter((item) => item.serverId !== serverId);
        return [...retained, ...nextCapabilities].sort((left, right) => left.id.localeCompare(right.id));
      });
    } catch (error) {
      setTools([]);
      setResources([]);
      setPrompts([]);
      message.error(errorMessage(error));
    }
  };

  useEffect(() => {
    void loadRuntime();
  }, []);

  useEffect(() => {
    if (!selectedServerId) {
      setTools([]);
      setResources([]);
      setPrompts([]);
      return;
    }
    void loadCapabilities(selectedServerId);
  }, [selectedServerId]);

  const selectServer = (server: MCPServer) => {
    setSelectedServerId(server.id);
    serverForm.setFieldsValue({
      id: server.id,
      name: server.name,
      transport: server.transport,
      endpoint: displayMCPEndpoint(server.endpoint),
      enabled: server.enabled,
      allowedTools: joinLines(server.allowedTools),
      allowedResources: joinLines(server.allowedResources),
      allowedPrompts: joinLines(server.allowedPrompts),
      defaultApprovalPolicy: server.defaultApprovalPolicy,
      sessionPolicy: server.sessionPolicy,
      timeoutMillis: server.timeoutMillis
    });
  };

  const resetServerDraft = () => {
    const nextIndex = servers.length + 1;
    serverForm.setFieldsValue({
      id: `mcp-runtime-${nextIndex}`,
      name: `Runtime MCP ${nextIndex}`,
      transport: 'stdio',
      enabled: true,
      sessionPolicy: 'transient',
      timeoutMillis: 60000,
      allowedTools: '',
      allowedResources: '',
      allowedPrompts: ''
    });
  };

  const saveServer = async (values: MCPServerFormValues) => {
    setSaving(true);
    try {
      const payload = mcpServerPayload(values);
      const exists = servers.some((item) => item.id === payload.id);
      const server = exists
        ? await workorchApi.updateMCPServer(payload.id, payload)
        : await workorchApi.createMCPServer(payload);
      message.success(`MCP 服务 ${server.id} 已${exists ? '更新' : '创建'}`);
      await loadRuntime();
      setSelectedServerId(server.id);
      await loadCapabilities(server.id);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const editCapability = (item: MCPCapability) => {
    setEditingCapability(item);
    capabilityForm.setFieldsValue({
      ...item,
      tags: item.tags ?? [],
      stageHints: item.stageHints ?? [],
      enabled: item.enabled !== false
    });
    setCapabilityModalOpen(true);
  };

  const saveCapability = async () => {
    if (!editingCapability) {
      return;
    }
    setSaving(true);
    try {
      const values = await capabilityForm.validateFields();
      await workorchApi.updateMCPCapability(editingCapability.id, {
        ...editingCapability,
        ...values,
        id: editingCapability.id,
        serverId: editingCapability.serverId,
        kind: editingCapability.kind,
        name: editingCapability.name,
        tags: values.tags ?? [],
        stageHints: values.stageHints ?? [],
        enabled: values.enabled !== false
      });
      message.success('MCP 能力已更新');
      setCapabilityModalOpen(false);
      setEditingCapability(undefined);
      await loadCapabilityDirectory();
      if (selectedServerId) {
        await loadCapabilities(selectedServerId);
      }
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const serverColumns: ProColumns<MCPServer>[] = [
    { title: '服务 ID', dataIndex: 'id', copyable: true },
    { title: '名称', dataIndex: 'name' },
    { title: '传输', dataIndex: 'transport', width: 120 },
    { title: '会话', dataIndex: 'sessionPolicy', width: 130, render: (_, item) => item.sessionPolicy || 'transient' },
    { title: '启用', dataIndex: 'enabled', width: 100, render: (_, item) => item.enabled ? <Tag color="green">已启用</Tag> : <Tag color="red">已禁用</Tag> },
    { title: '工具', render: (_, item) => item.allowedTools?.length ?? 0, width: 90 },
    { title: '资源', render: (_, item) => item.allowedResources?.length ?? 0, width: 110 },
    { title: '提示词', render: (_, item) => item.allowedPrompts?.length ?? 0, width: 100 },
    {
      title: '操作',
      valueType: 'option',
      render: (_, item) => <Button size="small" onClick={() => selectServer(item)}>查看</Button>
    }
  ];

  const capabilityColumns: ProColumns<MCPCapability>[] = [
    { title: '能力名称', dataIndex: 'displayName', copyable: true },
    {
      title: '类型',
      dataIndex: 'kind',
      width: 110,
      render: (_, item) => capabilityKindTag(item.kind)
    },
    { title: '技术标识', dataIndex: 'name', copyable: true, ellipsis: true },
    { title: '所属服务', dataIndex: 'serverId', ellipsis: true },
    { title: '分类', dataIndex: 'category', width: 120, render: (_, item) => item.category || '-' },
    {
      title: '阶段',
      dataIndex: 'stageHints',
      render: (_, item) => <Space wrap>{(item.stageHints ?? []).map((stage) => <Tag key={stage}>{stage}</Tag>)}</Space>
    },
    { title: '风险', dataIndex: 'riskLevel', width: 120, render: (_, item) => item.riskLevel || '-' },
    { title: '状态', dataIndex: 'enabled', width: 100, render: (_, item) => item.enabled ? <Tag color="green">已启用</Tag> : <Tag color="red">已禁用</Tag> },
    {
      title: '操作',
      valueType: 'option',
      width: 90,
      render: (_, item) => <Button size="small" onClick={() => editCapability(item)}>编辑</Button>
    }
  ];

  const selectedCapabilities = selectedServerId
    ? capabilities.filter((item) => item.serverId === selectedServerId)
    : capabilities;

  return (
    <PageContainer
      title="运行控制"
      subTitle={`管理${modelLabels.mcpCapability}，并观察本地运行队列与并发压力`}
      extra={<Button onClick={() => void loadRuntime()} loading={loading}>刷新</Button>}
    >
      <ProCard gutter={16} wrap loading={loading}>
        <ProCard title="运行调度" colSpan={{ xs: 24, lg: 8 }}>
          <Descriptions
            column={1}
            items={[
              { key: 'max', label: '最大并发运行（Run）', children: metricValue(metrics, 'workorch_server_max_concurrent_runs') },
              { key: 'inFlight', label: '执行中', children: metricValue(metrics, 'workorch_run_requests_in_flight') },
              { key: 'waiting', label: '队列等待', children: metricValue(metrics, 'workorch_run_queue_waiting') },
              { key: 'total', label: '队列总量', children: metricValue(metrics, 'workorch_run_queue_total') }
            ]}
          />
        </ProCard>
        <ProCard title="MCP 能力摘要" colSpan={{ xs: 24, lg: 16 }}>
          <Space wrap>
            <Tag>{servers.length} 个服务</Tag>
            <Tag color="green">{servers.filter((item) => item.enabled).length} 个已启用</Tag>
            <Tag color="blue">{capabilities.filter((item) => item.kind === 'tool').length} 个工具</Tag>
            <Tag color="cyan">{capabilities.filter((item) => item.kind === 'resource').length} 个资源</Tag>
            <Tag color="geekblue">{capabilities.filter((item) => item.kind === 'prompt').length} 个提示词</Tag>
            <Tag color="purple">{servers.filter((item) => item.sessionPolicy === 'long_lived').length} 个长会话</Tag>
          </Space>
        </ProCard>
      </ProCard>

      <ProCard
        title="MCP 能力目录"
        subTitle="以业务名称统一识别工具（Tool）、资源（Resource）和提示词（Prompt），同时保留技术标识"
        className="workorch-section"
      >
        <Form
          form={capabilityFilterForm}
          layout="inline"
          className="workorch-inline-filter"
          onFinish={(values) => void loadCapabilityDirectory(values)}
        >
          <Form.Item name="q" label="搜索">
            <Input allowClear placeholder="业务名称、技术标识、说明、标签" style={{ width: 260 }} />
          </Form.Item>
          <Form.Item name="serverId" label="服务">
            <Select
              allowClear
              placeholder="全部服务"
              style={{ width: 220 }}
              options={servers.map((item) => ({ value: item.id, label: `${item.name} / ${item.id}` }))}
            />
          </Form.Item>
          <Form.Item name="kind" label="类型">
            <Select
              allowClear
              placeholder="全部"
              style={{ width: 140 }}
              options={[
                { value: 'tool', label: modelLabels.mcpTool },
                { value: 'resource', label: modelLabels.mcpResource },
                { value: 'prompt', label: modelLabels.mcpPrompt }
              ]}
            />
          </Form.Item>
          <Form.Item name="category" label="分类">
            <Input allowClear placeholder="分类" style={{ width: 150 }} />
          </Form.Item>
          <Form.Item name="stage" label="阶段">
            <Input allowClear placeholder="阶段" style={{ width: 150 }} />
          </Form.Item>
          <Form.Item name="riskLevel" label="风险">
            <Select
              allowClear
              placeholder="全部"
              style={{ width: 150 }}
              options={[
                { value: 'read_only', label: '只读' },
                { value: 'operator', label: '需操作员确认' },
                { value: 'high', label: '高风险' }
              ]}
            />
          </Form.Item>
          <Form.Item name="enabled" label="状态">
            <Select
              allowClear
              placeholder="全部"
              style={{ width: 130 }}
              options={[
                { value: true, label: '已启用' },
                { value: false, label: '已禁用' }
              ]}
            />
          </Form.Item>
          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit">筛选</Button>
              <Button onClick={() => { capabilityFilterForm.resetFields(); void loadCapabilityDirectory({}); }}>重置</Button>
            </Space>
          </Form.Item>
        </Form>
        <ProTable<MCPCapability>
          rowKey="id"
          search={false}
          options={false}
          pagination={{ pageSize: 8 }}
          columns={capabilityColumns}
          dataSource={capabilities}
          expandable={{
            expandedRowRender: (item) => (
              <Descriptions
                column={1}
                size="small"
                items={[
                  { key: 'id', label: '能力 ID', children: item.id },
                  { key: 'description', label: '说明', children: item.description || '-' },
                  { key: 'usageHint', label: '使用建议', children: item.usageHint || '-' },
                  { key: 'input', label: '输入摘要', children: item.inputSummary || '-' },
                  { key: 'output', label: '输出摘要', children: item.outputSummary || '-' },
                  { key: 'uri', label: '资源 URI', children: item.uri || '-' },
                  { key: 'approval', label: '审批策略', children: item.approvalPolicy || '-' },
                  { key: 'tags', label: '标签', children: (item.tags ?? []).length > 0 ? <Space wrap>{item.tags?.map((tag) => <Tag key={tag}>{tag}</Tag>)}</Space> : '-' }
                ]}
              />
            )
          }}
        />
      </ProCard>

      <ProCard title="MCP 服务" className="workorch-section">
        <ProTable<MCPServer>
          rowKey="id"
          search={false}
          options={false}
          pagination={{ pageSize: 6 }}
          columns={serverColumns}
          dataSource={servers}
          rowClassName={(item) => item.id === selectedServerId ? 'workorch-selected-row' : ''}
        />
      </ProCard>

      <ProCard gutter={16} wrap className="workorch-section">
        <ProCard title="已选 MCP 服务" colSpan={{ xs: 24, lg: 12 }}>
          {selectedServer ? (
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <Descriptions
                column={1}
                items={[
                  { key: 'id', label: '服务 ID', children: selectedServer.id },
                  { key: 'endpoint', label: '端点', children: displayMCPEndpoint(selectedServer.endpoint) || '-' },
                  { key: 'approval', label: '默认审批策略', children: selectedServer.defaultApprovalPolicy || '-' },
                  { key: 'timeout', label: '超时毫秒', children: selectedServer.timeoutMillis ?? '-' }
                ]}
              />
              <ProTable<MCPTool>
                rowKey={(item) => `${item.serverId}-${item.name}`}
                search={false}
                options={false}
                pagination={false}
                size="small"
                columns={[{ title: '允许的工具', dataIndex: 'name' }]}
                dataSource={tools}
              />
              <ProTable<MCPCapability>
                rowKey="id"
                search={false}
                options={false}
                pagination={false}
                size="small"
                columns={[
                  { title: '业务能力', dataIndex: 'displayName' },
                  { title: '类型', dataIndex: 'kind', width: 100 },
                  { title: '技术标识', dataIndex: 'name', ellipsis: true },
                  { title: '状态', dataIndex: 'enabled', width: 100, render: (_, item) => item.enabled ? <Tag color="green">已启用</Tag> : <Tag color="red">已禁用</Tag> }
                ]}
                dataSource={selectedCapabilities}
              />
              <ProTable<MCPResource>
                rowKey={(item) => `${item.serverId}-${item.uri}`}
                search={false}
                options={false}
                pagination={false}
                size="small"
                columns={[{ title: '允许的资源', dataIndex: 'uri' }]}
                dataSource={resources}
              />
              <ProTable<MCPPrompt>
                rowKey={(item) => `${item.serverId}-${item.name}`}
                search={false}
                options={false}
                pagination={false}
                size="small"
                columns={[{ title: '允许的提示词', dataIndex: 'name' }]}
                dataSource={prompts}
              />
            </Space>
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择 MCP 服务" />
          )}
        </ProCard>

        <ProCard
          title="MCP 服务配置"
          colSpan={{ xs: 24, lg: 12 }}
          extra={<Button onClick={resetServerDraft}>新建草稿</Button>}
        >
          <Form
            form={serverForm}
            layout="vertical"
            initialValues={{
              transport: 'stdio',
              enabled: true,
              sessionPolicy: 'transient',
              timeoutMillis: 60000
            }}
            onFinish={(values) => void saveServer(values)}
          >
            <Form.Item name="id" label="服务 ID" rules={[{ required: true }]}>
              <Input placeholder="mcp-filesystem" />
            </Form.Item>
            <Form.Item name="name" label="名称" rules={[{ required: true }]}>
              <Input placeholder="Filesystem MCP" />
            </Form.Item>
            <Form.Item name="transport" label="传输方式" rules={[{ required: true }]}>
              <Select options={[
                { value: 'stdio', label: 'stdio' },
                { value: 'http', label: 'http' },
                { value: 'https', label: 'https' }
              ]} />
            </Form.Item>
            <Form.Item name="endpoint" label="端点">
              <Input placeholder="stdio 命令路径，或 HTTP endpoint URL" />
            </Form.Item>
            <Form.Item name="enabled" label="是否启用">
              <Select options={[
                { value: true, label: '是' },
                { value: false, label: '否' }
              ]} />
            </Form.Item>
            <Form.Item name="sessionPolicy" label="会话策略">
              <Select options={[
                { value: 'transient', label: '短会话（transient）' },
                { value: 'long_lived', label: '长会话（long_lived）' }
              ]} />
            </Form.Item>
            <Form.Item name="timeoutMillis" label="超时毫秒">
              <InputNumber style={{ width: '100%' }} min={0} />
            </Form.Item>
            <Form.Item name="defaultApprovalPolicy" label="默认审批策略">
              <Input placeholder="可选审批策略" />
            </Form.Item>
            <Form.Item name="allowedTools" label="允许的工具（Tool）">
              <Input.TextArea rows={3} placeholder="每行一个工具名称" />
            </Form.Item>
            <Form.Item name="allowedResources" label="允许的资源（Resource）">
              <Input.TextArea rows={3} placeholder="每行一个资源 URI" />
            </Form.Item>
            <Form.Item name="allowedPrompts" label="允许的提示词（Prompt）">
              <Input.TextArea rows={3} placeholder="每行一个提示词名称" />
            </Form.Item>
            <Button type="primary" htmlType="submit" loading={saving}>保存 MCP 服务</Button>
          </Form>
        </ProCard>
      </ProCard>
      <Modal
        title="编辑 MCP 能力"
        open={capabilityModalOpen}
        onCancel={() => {
          setCapabilityModalOpen(false);
          setEditingCapability(undefined);
        }}
        onOk={() => void saveCapability()}
        confirmLoading={saving}
        width={760}
        destroyOnHidden
      >
        <Form form={capabilityForm} layout="vertical">
          <Descriptions
            size="small"
            column={1}
            items={[
              { key: 'id', label: '能力 ID', children: editingCapability?.id || '-' },
              { key: 'technical', label: '技术标识', children: `${editingCapability?.kind || '-'} / ${editingCapability?.serverId || '-'} / ${editingCapability?.name || '-'}` }
            ]}
          />
          <Form.Item name="displayName" label="业务名称" rules={[{ required: true }]}>
            <Input placeholder="便于用户识别的能力名称" />
          </Form.Item>
          <Form.Item name="description" label="能力说明">
            <Input.TextArea rows={3} placeholder="说明该工具、资源或提示词做什么、何时使用" />
          </Form.Item>
          <Form.Item name="category" label="业务分类">
            <Input placeholder="例如：代码仓库、验收、数据查询" />
          </Form.Item>
          <Form.Item name="tags" label="标签">
            <Select mode="tags" placeholder="输入后回车生成标签" options={[]} />
          </Form.Item>
          <Form.Item name="stageHints" label="适用阶段">
            <Select mode="tags" placeholder="例如：planning、execution、validation" options={[]} />
          </Form.Item>
          <Form.Item name="usageHint" label="使用建议">
            <Input.TextArea rows={2} placeholder="给工作空间（Workspace）或技能（Skill）选择该能力时的提示" />
          </Form.Item>
          <Form.Item name="inputSummary" label="输入摘要">
            <Input.TextArea rows={2} placeholder="说明需要哪些输入" />
          </Form.Item>
          <Form.Item name="outputSummary" label="输出摘要">
            <Input.TextArea rows={2} placeholder="说明会产生哪些输出" />
          </Form.Item>
          <Form.Item name="riskLevel" label="风险等级">
            <Select
              allowClear
              options={[
                { value: 'read_only', label: '只读' },
                { value: 'operator', label: '需操作员确认' },
                { value: 'high', label: '高风险' }
              ]}
            />
          </Form.Item>
          <Form.Item name="approvalPolicy" label="审批策略">
            <Input placeholder="例如：operator、auto、manual" />
          </Form.Item>
          <Form.Item name="status" label="管理状态">
            <Select
              allowClear
              options={[
                { value: 'draft', label: '草稿' },
                { value: 'active', label: '已确认' },
                { value: 'deprecated', label: '已废弃' }
              ]}
            />
          </Form.Item>
          <Form.Item name="enabled" label="是否启用">
            <Select options={[
              { value: true, label: '启用' },
              { value: false, label: '禁用' }
            ]} />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
}

function ConfigurationPage() {
  const { message } = AntApp.useApp();
  const [serverForm] = Form.useForm<UpdateServerConfigurationRequest>();
  const [providerEditorForm] = Form.useForm<UpdateLLMProviderConfiguration>();
  const [webSearchProviderEditorForm] = Form.useForm<WebSearchProviderFormValues>();
  const [configuration, setConfiguration] = useState<ServiceConfiguration | undefined>(undefined);
  const [souls, setSouls] = useState<Soul[]>([]);
  const [workspaceTemplates, setWorkspaceTemplates] = useState<WorkspaceTemplate[]>([]);
  const [defaultProviderId, setDefaultProviderId] = useState<string | undefined>(undefined);
  const [defaultEmbeddingProviderId, setDefaultEmbeddingProviderId] = useState<string | undefined>(undefined);
  const [providerDrafts, setProviderDrafts] = useState<UpdateLLMProviderConfiguration[]>([]);
  const [selectedProviderIndex, setSelectedProviderIndex] = useState(0);
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | undefined>(undefined);
  const [defaultWebSearchProviderId, setDefaultWebSearchProviderId] = useState<string | undefined>(undefined);
  const [webSearchProviderDrafts, setWebSearchProviderDrafts] = useState<UpdateWebSearchProviderConfiguration[]>([]);
  const [selectedWebSearchProviderIndex, setSelectedWebSearchProviderIndex] = useState(0);
  const [selectedWebSearchTemplateId, setSelectedWebSearchTemplateId] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [serverSaving, setServerSaving] = useState(false);
  const [saving, setSaving] = useState(false);
  const [webSearchSaving, setWebSearchSaving] = useState(false);

  const loadConfiguration = async () => {
    setLoading(true);
    try {
      const [nextConfiguration, nextSouls, nextWorkspaceTemplates] = await Promise.all([
        workorchApi.readConfiguration(),
        workorchApi.listSouls(),
        workorchApi.listWorkspaceTemplates()
      ]);
      const nextFormValues = llmConfigurationFormValues(nextConfiguration);
      const nextWebSearchFormValues = webSearchConfigurationFormValues(nextConfiguration);
      setConfiguration(nextConfiguration);
      setSouls(nextSouls);
      setWorkspaceTemplates(nextWorkspaceTemplates);
      setDefaultProviderId(nextFormValues.defaultProviderId);
      setDefaultEmbeddingProviderId(nextFormValues.defaultEmbeddingProviderId);
      setProviderDrafts(nextFormValues.providers);
      setSelectedProviderIndex(0);
      setSelectedTemplateId(nextConfiguration.llm.providerTemplates?.[0]?.templateId);
      setDefaultWebSearchProviderId(nextWebSearchFormValues.defaultProviderId);
      setWebSearchProviderDrafts(nextWebSearchFormValues.providers);
      setSelectedWebSearchProviderIndex(0);
      setSelectedWebSearchTemplateId(nextConfiguration.webSearch.providerTemplates?.[0]?.templateId);
      serverForm.setFieldsValue({ defaultDialogLanguage: nextConfiguration.server.defaultDialogLanguage || '' });
      providerEditorForm.setFieldsValue(nextFormValues.providers[0] ?? nextProviderDraft([]));
      webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(nextWebSearchFormValues.providers[0] ?? nextWebSearchProviderDraft([])));
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadConfiguration();
  }, []);

  const saveLLMConfiguration = async () => {
    setSaving(true);
    try {
      const nextConfiguration = await workorchApi.updateLLMConfiguration({
        defaultProviderId,
        defaultEmbeddingProviderId,
        providers: providerDrafts.map((item) => ({
          id: item.id?.trim(),
          provider: item.provider?.trim(),
          baseUrl: item.baseUrl?.trim(),
          apiKey: item.apiKey?.trim(),
          model: item.model?.trim(),
          timeout: item.timeout?.trim(),
          disabled: item.disabled === true,
          chatCompletions: item.chatCompletions !== false,
          streaming: item.streaming === true,
          nonChatOperations: item.nonChatOperations ?? []
        }))
      });
      const nextFormValues = llmConfigurationFormValues(nextConfiguration);
      setConfiguration(nextConfiguration);
      setDefaultProviderId(nextFormValues.defaultProviderId);
      setDefaultEmbeddingProviderId(nextFormValues.defaultEmbeddingProviderId);
      setProviderDrafts(nextFormValues.providers);
      setSelectedProviderIndex(0);
      providerEditorForm.setFieldsValue(nextFormValues.providers[0] ?? nextProviderDraft([]));
      message.success('LLM 配置已更新');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  const saveServerConfiguration = async (values: UpdateServerConfigurationRequest) => {
    setServerSaving(true);
    try {
      const nextConfiguration = await workorchApi.updateServerConfiguration({
        defaultDialogLanguage: values.defaultDialogLanguage || ''
      });
      setConfiguration(nextConfiguration);
      serverForm.setFieldsValue({ defaultDialogLanguage: nextConfiguration.server.defaultDialogLanguage || '' });
      message.success('服务配置已更新');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setServerSaving(false);
    }
  };

  const saveWebSearchConfiguration = async () => {
    setWebSearchSaving(true);
    try {
      const nextConfiguration = await workorchApi.updateWebSearchConfiguration({
        defaultProviderId: defaultWebSearchProviderId,
        providers: webSearchProviderDrafts.map((item) => ({
          id: item.id?.trim(),
          provider: item.provider?.trim(),
          searchEndpoint: item.searchEndpoint?.trim(),
          fetchEndpoint: item.fetchEndpoint?.trim(),
          apiKey: item.apiKey?.trim(),
          timeout: item.timeout?.trim(),
          maxResults: item.maxResults,
          maxFetchBytes: item.maxFetchBytes,
          maxRedirects: item.maxRedirects,
          allowDomains: item.allowDomains ?? [],
          denyDomains: item.denyDomains ?? [],
          disabled: item.disabled === true
        }))
      });
      const nextFormValues = webSearchConfigurationFormValues(nextConfiguration);
      setConfiguration(nextConfiguration);
      setDefaultWebSearchProviderId(nextFormValues.defaultProviderId);
      setWebSearchProviderDrafts(nextFormValues.providers);
      setSelectedWebSearchProviderIndex(0);
      webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(nextFormValues.providers[0] ?? nextWebSearchProviderDraft([])));
      message.success('Web Search 配置已更新');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setWebSearchSaving(false);
    }
  };

  const selectProvider = (index: number) => {
    const item = providerDrafts[index];
    if (!item) {
      return;
    }
    setSelectedProviderIndex(index);
    providerEditorForm.setFieldsValue(item);
  };

  const providerTemplates = configuration?.llm.providerTemplates ?? [];
  const providerTemplateByID = new Map(providerTemplates.map((item) => [item.templateId, item]));

  const addProvider = () => {
    const draft = nextProviderDraft(providerDrafts, selectedTemplateId ? providerTemplateByID.get(selectedTemplateId) : undefined);
    setProviderDrafts((items) => [...items, draft]);
    setSelectedProviderIndex(providerDrafts.length);
    providerEditorForm.setFieldsValue(draft);
    if (!defaultProviderId) {
      setDefaultProviderId(draft.id);
    }
  };

  const removeSelectedProvider = () => {
    if (providerDrafts.length <= 1) {
      return;
    }
    const nextDrafts = providerDrafts.filter((_, index) => index !== selectedProviderIndex);
    const nextIndex = Math.min(selectedProviderIndex, nextDrafts.length - 1);
    setProviderDrafts(nextDrafts);
    setSelectedProviderIndex(nextIndex);
    providerEditorForm.setFieldsValue(nextDrafts[nextIndex]);
    if (!nextDrafts.some((item) => item.id === defaultProviderId)) {
      setDefaultProviderId(nextDrafts[0]?.id);
    }
    if (!nextDrafts.some((item) => item.id === defaultEmbeddingProviderId)) {
      setDefaultEmbeddingProviderId(undefined);
    }
  };

  const updateSelectedProvider = (changedValues: Partial<UpdateLLMProviderConfiguration>, values: UpdateLLMProviderConfiguration) => {
    if (changedValues.provider) {
      const template = providerTemplates.find((item) => item.provider === changedValues.provider);
      if (template) {
        const templated = providerDraftFromTemplate(template, providerDrafts, { ...values, id: values.id, apiKey: values.apiKey });
        providerEditorForm.setFieldsValue(templated);
        setProviderDrafts((items) => {
          const nextItems = [...items];
          nextItems[selectedProviderIndex] = templated;
          return nextItems;
        });
        return;
      }
    }
    setProviderDrafts((items) => {
      const previousID = items[selectedProviderIndex]?.id;
      const nextItems = [...items];
      nextItems[selectedProviderIndex] = {
        ...nextItems[selectedProviderIndex],
        ...values
      };
      const nextID = values.id?.trim();
      if (defaultProviderId === previousID && nextID && nextID !== previousID) {
        setDefaultProviderId(nextID);
      }
      if (defaultEmbeddingProviderId === previousID && nextID && nextID !== previousID) {
        setDefaultEmbeddingProviderId(nextID);
      }
      return nextItems;
    });
  };

  const statusByProviderID = new Map((configuration?.llm.providers ?? []).map((item) => [item.id, item]));
  const selectedProvider = providerDrafts[selectedProviderIndex];
  const selectedProviderStatus = selectedProvider ? statusByProviderID.get(selectedProvider.id) : undefined;
  const webSearchProviderTemplates = configuration?.webSearch.providerTemplates ?? [];
  const webSearchProviderTemplateByID = new Map(webSearchProviderTemplates.map((item) => [item.templateId, item]));
  const webSearchStatusByProviderID = new Map((configuration?.webSearch.providers ?? []).map((item) => [item.id, item]));
  const selectedWebSearchProvider = webSearchProviderDrafts[selectedWebSearchProviderIndex];
  const selectedWebSearchProviderStatus = selectedWebSearchProvider ? webSearchStatusByProviderID.get(selectedWebSearchProvider.id) : undefined;

  const selectWebSearchProvider = (index: number) => {
    const item = webSearchProviderDrafts[index];
    if (!item) {
      return;
    }
    setSelectedWebSearchProviderIndex(index);
    webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(item));
  };

  const addWebSearchProvider = () => {
    const draft = nextWebSearchProviderDraft(webSearchProviderDrafts, selectedWebSearchTemplateId ? webSearchProviderTemplateByID.get(selectedWebSearchTemplateId) : undefined);
    setWebSearchProviderDrafts((items) => [...items, draft]);
    setSelectedWebSearchProviderIndex(webSearchProviderDrafts.length);
    webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(draft));
    if (!defaultWebSearchProviderId) {
      setDefaultWebSearchProviderId(draft.id);
    }
  };

  const removeSelectedWebSearchProvider = () => {
    if (webSearchProviderDrafts.length <= 1) {
      return;
    }
    const nextDrafts = webSearchProviderDrafts.filter((_, index) => index !== selectedWebSearchProviderIndex);
    const nextIndex = Math.min(selectedWebSearchProviderIndex, nextDrafts.length - 1);
    setWebSearchProviderDrafts(nextDrafts);
    setSelectedWebSearchProviderIndex(nextIndex);
    webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(nextDrafts[nextIndex]));
    if (!nextDrafts.some((item) => item.id === defaultWebSearchProviderId)) {
      setDefaultWebSearchProviderId(nextDrafts[0]?.id);
    }
  };

  const updateSelectedWebSearchProvider = (changedValues: Partial<WebSearchProviderFormValues>, values: WebSearchProviderFormValues) => {
    const currentDraft = webSearchProviderDrafts[selectedWebSearchProviderIndex];
    if (changedValues.provider) {
      const template = webSearchProviderTemplates.find((item) => item.provider === changedValues.provider);
      if (template) {
        const templated = webSearchProviderDraftFromTemplate(template, webSearchProviderDrafts, {
          ...currentDraft,
          ...values,
          allowDomains: splitLines(values.allowDomains),
          denyDomains: splitLines(values.denyDomains),
          apiKey: values.apiKey
        });
        webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(templated));
        setWebSearchProviderDrafts((items) => {
          const nextItems = [...items];
          nextItems[selectedWebSearchProviderIndex] = templated;
          return nextItems;
        });
        return;
      }
    }
    setWebSearchProviderDrafts((items) => {
      const previousID = items[selectedWebSearchProviderIndex]?.id;
      const nextItems = [...items];
      nextItems[selectedWebSearchProviderIndex] = {
        ...nextItems[selectedWebSearchProviderIndex],
        ...values,
        allowDomains: splitLines(values.allowDomains),
        denyDomains: splitLines(values.denyDomains)
      };
      const nextID = values.id?.trim();
      if (defaultWebSearchProviderId === previousID && nextID && nextID !== previousID) {
        setDefaultWebSearchProviderId(nextID);
      }
      return nextItems;
    });
  };

  return (
    <PageContainer
      title="系统配置"
      subTitle="管理服务配置、执行人格（Soul）、工作空间模板（Workspace Template）、LLM 服务商和 Web Search Provider"
      extra={<Button onClick={() => void loadConfiguration()} loading={loading}>刷新</Button>}
    >
      <ProCard gutter={16} wrap loading={loading}>
        <ProCard title="服务参数" colSpan="50%">
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions
              column={1}
              items={[
                { key: 'addr', label: '监听地址', children: configuration?.server.addr || '-' },
                { key: 'readHeaderTimeout', label: '读 Header 超时', children: configuration?.server.readHeaderTimeout || '-' },
                { key: 'readTimeout', label: '读超时', children: configuration?.server.readTimeout || '-' },
                { key: 'writeTimeout', label: '写超时', children: configuration?.server.writeTimeout || '-' },
                { key: 'idleTimeout', label: '空闲超时', children: configuration?.server.idleTimeout || '-' },
                { key: 'shutdownTimeout', label: '关闭超时', children: configuration?.server.shutdownTimeout || '-' },
                { key: 'maxConcurrentRuns', label: '最大并发运行（Run）', children: configuration?.server.maxConcurrentRuns ?? '-' },
                { key: 'defaultSoulId', label: '系统默认执行人格（Soul）', children: configuration?.runtime.defaultSoulId || '-' }
              ]}
            />
            <Form form={serverForm} layout="inline" onFinish={(values) => void saveServerConfiguration(values)}>
              <Form.Item name="defaultDialogLanguage" label="默认交互语言">
                <Select
                  style={{ width: 180 }}
                  options={[
                    { value: '', label: '跟随系统语言' },
                    { value: 'zh-CN', label: '中文' },
                    { value: 'en', label: 'English' }
                  ]}
                />
              </Form.Item>
              <Form.Item>
                <Button type="primary" htmlType="submit" loading={serverSaving}>保存服务配置</Button>
              </Form.Item>
            </Form>
          </Space>
        </ProCard>
        <ProCard title="仓储" colSpan="50%">
          <Descriptions
            column={1}
            items={[
              { key: 'backend', label: '后端', children: configuration?.repository.backend || '-' },
              { key: 'dataDir', label: '数据目录', children: configuration?.repository.dataDir || '-' }
            ]}
          />
        </ProCard>
      </ProCard>
      <ProCard
        title="Web Search Provider"
        className="workorch-section"
        extra={<Button type="primary" onClick={() => void webSearchProviderEditorForm.validateFields().then(() => saveWebSearchConfiguration())} loading={webSearchSaving}>保存 Web Search Provider</Button>}
      >
        <Typography.Paragraph type="secondary">
          Web Search Provider 由 workorch 运行期统一治理，提供 web/search 与 web/fetch；API 密钥读取时隐藏，编辑时留空表示保留已有密钥。
        </Typography.Paragraph>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <ProCard
            title="Provider 列表"
            extra={(
              <Space>
                <Select
                  style={{ width: 240 }}
                  value={selectedWebSearchTemplateId}
                  onChange={setSelectedWebSearchTemplateId}
                  placeholder="选择配置模板"
                  options={webSearchProviderTemplates.map((item) => ({ value: item.templateId, label: item.name }))}
                />
                <Button onClick={addWebSearchProvider}>按模板新增</Button>
              </Space>
            )}
          >
            <Form.Item label="默认 Web Search Provider">
              <Select
                value={defaultWebSearchProviderId}
                onChange={setDefaultWebSearchProviderId}
                placeholder="选择默认 Web Search Provider"
                options={webSearchProviderDrafts
                  .map((item) => item.id?.trim())
                  .filter(Boolean)
                  .map((id) => ({ value: id, label: id }))}
              />
            </Form.Item>
            <Space direction="vertical" size="small" className="llm-provider-list">
              {webSearchProviderDrafts.map((item, index) => {
                const status = webSearchStatusByProviderID.get(item.id);
                const selected = index === selectedWebSearchProviderIndex;
                return (
                  <button
                    key={`${item.id || 'web-provider'}-${index}`}
                    type="button"
                    className={`llm-provider-list-item${selected ? ' llm-provider-list-item-selected' : ''}`}
                    onClick={() => selectWebSearchProvider(index)}
                  >
                    <span className="llm-provider-list-main">
                      <Typography.Text strong>{item.id || '新 Provider'}</Typography.Text>
                      <Typography.Text type="secondary">{item.searchEndpoint || item.provider || '搜索端点未配置'}</Typography.Text>
                    </span>
                    <span className="llm-provider-list-tags">
                      {item.id === defaultWebSearchProviderId ? <Tag color="blue">默认</Tag> : null}
                      {status?.apiKeyConfigured ? <Tag color="green">密钥</Tag> : null}
                      {status?.ready ? <Tag color="green">就绪</Tag> : <Tag color="red">{status?.reason || '未保存'}</Tag>}
                    </span>
                  </button>
                );
              })}
            </Space>
          </ProCard>
          <ProCard
            title={selectedWebSearchProvider ? `编辑 Provider：${selectedWebSearchProvider.id || '新 Provider'}` : '编辑 Provider'}
            extra={<Button danger onClick={removeSelectedWebSearchProvider} disabled={webSearchProviderDrafts.length <= 1}>移除</Button>}
          >
            {selectedWebSearchProvider ? (
              <Form
                form={webSearchProviderEditorForm}
                layout="vertical"
                onValuesChange={updateSelectedWebSearchProvider}
              >
                <Form.Item name="id" label="Provider 标识" rules={[{ required: true }]}>
                  <Input placeholder="web-http-main" />
                </Form.Item>
                <Form.Item name="provider" label="Provider 类型" rules={[{ required: true }]}>
                  <Select
                    options={webSearchProviderTemplates.map((item) => ({ value: item.provider, label: `${item.provider} / ${item.name}` }))}
                  />
                </Form.Item>
                <Form.Item name="searchEndpoint" label="Search Endpoint" rules={[{ required: true }]}>
                  <Input placeholder="https://search-gateway.example/api/search" />
                </Form.Item>
                <Form.Item name="fetchEndpoint" label="Fetch Endpoint">
                  <Input placeholder="https://search-gateway.example/api/fetch" />
                </Form.Item>
                <Form.Item name="timeout" label="超时">
                  <Input placeholder="20s" />
                </Form.Item>
                <Form.Item name="maxResults" label="最大搜索结果">
                  <InputNumber min={1} max={20} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name="maxFetchBytes" label="最大读取字节数">
                  <InputNumber min={1024} max={1048576} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name="maxRedirects" label="最大重定向次数">
                  <InputNumber min={0} max={5} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name="allowDomains" label="允许域名">
                  <Input.TextArea rows={2} placeholder="每行一个域名，留空表示不限制" />
                </Form.Item>
                <Form.Item name="denyDomains" label="拒绝域名">
                  <Input.TextArea rows={2} placeholder="每行一个域名" />
                </Form.Item>
                <Form.Item name="disabled" label="是否禁用">
                  <Select
                    options={[
                      { value: false, label: '否' },
                      { value: true, label: '是' }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="apiKey" label="API 密钥">
                  <Input.Password placeholder="留空表示保留已有 API 密钥" autoComplete="new-password" />
                </Form.Item>
                <Descriptions
                  column={1}
                  items={[
                    { key: 'apiKey', label: '已保存 API 密钥', children: selectedWebSearchProviderStatus?.apiKeyConfigured ? <Tag color="green">已配置</Tag> : <Tag>未配置</Tag> },
                    { key: 'status', label: '已保存状态', children: selectedWebSearchProviderStatus?.ready ? <Tag color="green">就绪</Tag> : <Tag color="red">{selectedWebSearchProviderStatus?.reason || '未保存'}</Tag> },
                    { key: 'fetch', label: '读取端点', children: selectedWebSearchProviderStatus?.fetchEndpoint || '-' }
                  ]}
                />
              </Form>
            ) : (
              <Empty description="请选择或新增 Web Search Provider" />
            )}
          </ProCard>
        </Space>
      </ProCard>
      <SoulDefinitionManager souls={souls} onSoulsChange={setSouls} />
      <WorkspaceTemplateManager souls={souls} templates={workspaceTemplates} onTemplatesChange={setWorkspaceTemplates} />
      <ProCard
        title="LLM 服务商"
        className="workorch-section"
        extra={<Button type="primary" onClick={() => void providerEditorForm.validateFields().then(() => saveLLMConfiguration())} loading={saving}>保存 LLM 服务商</Button>}
      >
        <Typography.Paragraph type="secondary">
          API 密钥读取时会被隐藏。编辑时留空表示保留已有密钥；保存后的服务商配置会由 workorch 运行期重新加载，并用于后续 LLM 步骤。
        </Typography.Paragraph>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <ProCard
            title="服务商列表"
            extra={(
              <Space>
                <Select
                  style={{ width: 220 }}
                  value={selectedTemplateId}
                  onChange={setSelectedTemplateId}
                  placeholder="选择配置模板"
                  options={providerTemplates.map((item) => ({ value: item.templateId, label: item.name }))}
                />
                <Button onClick={addProvider}>按模板新增</Button>
              </Space>
            )}
          >
            <Typography.Paragraph type="secondary">
              配置模板按 provider 模式生成草稿：OpenAI 与 Anthropic 使用官方 SDK；DeepSeek、MiniMax 等支持 OpenAI SDK 的服务请选择 OpenAI 并配置 baseURL，compatible 仅用于轻量 HTTP 兼容端点。
            </Typography.Paragraph>
            <Form.Item label="默认 LLM 服务商">
              <Select
                value={defaultProviderId}
                onChange={setDefaultProviderId}
                placeholder="选择默认 LLM 服务商"
                options={providerDrafts
                  .map((item) => item.id?.trim())
                  .filter(Boolean)
                  .map((id) => ({ value: id, label: id }))}
              />
            </Form.Item>
            <Form.Item label="默认 Embedding 服务商">
              <Select
                allowClear
                value={defaultEmbeddingProviderId}
                onChange={setDefaultEmbeddingProviderId}
                placeholder="选择用于向量记忆召回的服务商"
                options={providerDrafts
                  .filter((item) => (item.nonChatOperations ?? []).includes('embeddings'))
                  .map((item) => item.id?.trim())
                  .filter(Boolean)
                  .map((id) => ({ value: id, label: id }))}
              />
            </Form.Item>
            <Space direction="vertical" size="small" className="llm-provider-list">
              {providerDrafts.map((item, index) => {
                const status = statusByProviderID.get(item.id);
                const selected = index === selectedProviderIndex;
                return (
                  <button
                    key={`${item.id || 'provider'}-${index}`}
                    type="button"
                    className={`llm-provider-list-item${selected ? ' llm-provider-list-item-selected' : ''}`}
                    onClick={() => selectProvider(index)}
                  >
                    <span className="llm-provider-list-main">
                      <Typography.Text strong>{item.id || '新服务商'}</Typography.Text>
                      <Typography.Text type="secondary">{item.model || item.provider || '模型未配置'}</Typography.Text>
                    </span>
                    <span className="llm-provider-list-tags">
                      {item.id === defaultProviderId ? <Tag color="blue">默认</Tag> : null}
                      {item.id === defaultEmbeddingProviderId ? <Tag color="cyan">embedding</Tag> : null}
                      {status?.apiKeyConfigured ? <Tag color="green">密钥</Tag> : <Tag color="red">密钥</Tag>}
                      {status?.ready ? <Tag color="green">就绪</Tag> : <Tag color="red">{status?.reason || '未保存'}</Tag>}
                    </span>
                  </button>
                );
              })}
            </Space>
          </ProCard>
          <ProCard
            title={selectedProvider ? `编辑服务商：${selectedProvider.id || '新服务商'}` : '编辑服务商'}
            extra={<Button danger onClick={removeSelectedProvider} disabled={providerDrafts.length <= 1}>移除</Button>}
          >
            {selectedProvider ? (
              <Form
                form={providerEditorForm}
                layout="vertical"
                onValuesChange={updateSelectedProvider}
              >
                <Form.Item name="id" label="服务商标识" rules={[{ required: true }]}>
                  <Input placeholder="local" />
                </Form.Item>
                <Form.Item name="provider" label="服务商类型" rules={[{ required: true }]}>
                  <Select
                    options={providerTemplates.map((item) => ({ value: item.provider, label: `${item.provider} / ${item.name}` }))}
                  />
                </Form.Item>
                <Form.Item name="model" label="模型" rules={[{ required: true }]}>
                  <Input placeholder="gpt-4.1-mini" />
                </Form.Item>
                <Form.Item name="baseUrl" label="基础 URL">
                  <Input placeholder="https://api.openai.com/v1 或本地兼容端点" />
                </Form.Item>
                <Form.Item name="timeout" label="超时">
                  <Input placeholder="60s" />
                </Form.Item>
                <Form.Item name="disabled" label="是否禁用">
                  <Select
                    options={[
                      { value: false, label: '否' },
                      { value: true, label: '是' }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="chatCompletions" label="对话补全">
                  <Select
                    options={[
                      { value: true, label: '启用' },
                      { value: false, label: '禁用' }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="streaming" label="流式输出">
                  <Select
                    options={[
                      { value: false, label: '禁用' },
                      { value: true, label: '启用' }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="nonChatOperations" label="非对话能力">
                  <Select mode="tags" placeholder="embeddings, responses" options={[]} />
                </Form.Item>
                <Form.Item name="apiKey" label="API 密钥">
                  <Input.Password placeholder="留空表示保留已有 API 密钥" autoComplete="new-password" />
                </Form.Item>
                <Descriptions
                  column={1}
                  items={[
                    { key: 'apiKey', label: '已保存 API 密钥', children: selectedProviderStatus?.apiKeyConfigured ? <Tag color="green">已配置</Tag> : <Tag color="red">缺失</Tag> },
                    { key: 'status', label: '已保存状态', children: selectedProviderStatus?.ready ? <Tag color="green">就绪</Tag> : <Tag color="red">{selectedProviderStatus?.reason || '未保存'}</Tag> },
                    { key: 'capabilities', label: '已保存能力', children: <Space wrap>{selectedProviderStatus?.chatCompletions ? <Tag color="blue">chat</Tag> : null}{selectedProviderStatus?.streaming ? <Tag color="cyan">streaming</Tag> : null}{(selectedProviderStatus?.nonChatOperations ?? []).map((item) => <Tag key={item}>{item}</Tag>)}</Space> }
                  ]}
                />
              </Form>
            ) : (
              <Empty description="请选择或新增 LLM 服务商" />
            )}
          </ProCard>
        </Space>
      </ProCard>
    </PageContainer>
  );
}

function ServicePage() {
  const [health, setHealth] = useState<Record<string, string>>({});
  const [ready, setReady] = useState<Record<string, string>>({});
  const [metrics, setMetrics] = useState('');

  useEffect(() => {
    void workorchApi.healthz().then(setHealth);
    void workorchApi.readyz().then(setReady);
    void fetch('/metrics').then((response) => response.text()).then(setMetrics);
  }, []);

  return (
    <PageContainer title="服务状态" subTitle="查看 workorch 健康状态、就绪状态和运行指标">
      <ProCard gutter={16} wrap>
        <ProCard title="健康检查" colSpan="50%">
          <Descriptions items={Object.entries(health).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
        </ProCard>
        <ProCard title="就绪检查" colSpan="50%">
          <Descriptions items={Object.entries(ready).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
        </ProCard>
      </ProCard>
      <ProCard title={modelLabels.metrics} className="workorch-section">
        <pre className="metrics-panel">{metrics}</pre>
      </ProCard>
    </PageContainer>
  );
}

function ConsoleApp() {
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
