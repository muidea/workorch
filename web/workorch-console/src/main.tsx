import React, { useEffect, useRef, useState } from 'react';
import ReactDOM from 'react-dom/client';
import {
  CheckCircleOutlined,
  ClusterOutlined,
  DashboardOutlined,
  ExceptionOutlined,
  ProfileOutlined,
  SettingOutlined
} from '@ant-design/icons';
import { PageContainer, ProCard, ProLayout, ProTable } from '@ant-design/pro-components';
import { App as AntApp, Badge, Button, ConfigProvider, Descriptions, Empty, Form, Input, InputNumber, List, Modal, Select, Space, Steps, Tabs, Tag, Typography, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import type { Approval, Artifact, Decision, DoD, Goal, LLMProviderTemplate, MCPPrompt, MCPResource, MCPServer, MCPTool, MemoryItem, Patch, Result, Run, RunObservability, RunSnapshot, RuntimeCheckpoint, RuntimeTimelineItem, ServiceConfiguration, Skill, Soul, StepView, ToolCall, UpdateLLMConfigurationRequest, UpdateLLMProviderConfiguration, ValidationResult, Workspace, WorkspaceExecutionActivationRequest, WorkspaceSnapshot } from './types';
import { workorchApi } from './api';
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

function stateLabel(state?: string) {
  if (!state) {
    return '未知';
  }
  return stateLabels[state] ? `${stateLabels[state]} (${state})` : state;
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
  if (!snapshot) {
    return undefined;
  }
  const activeRun = snapshot.runs?.find((item) => item.id === snapshot.workspace.activeRunId);
  return activeRun ?? runStateSummary(snapshot.runs).latest;
}

function workspaceProcessItems(snapshot?: WorkspaceSnapshot, goals?: Goal[], dods?: DoD[]) {
  const currentRun = activeOrLatestRun(snapshot);
  const hasDefinition = Boolean((goals?.length ?? 0) > 0 && (dods?.length ?? 0) > 0);
  const missingDefinition = !goals?.length ? '需要定义 Goal' : !dods?.length ? '需要定义 DoD' : '需要补齐 Goal / DoD';
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
      : currentRun?.state === 'validating' ? '正在执行 DoD 验收'
        : currentRun ? '等待运行证据闭环' : '尚未启动 Run';
  return [
    { title: '工作空间', description: snapshot?.workspace ? stateLabel(snapshot.workspace.state) : '未选择', status: snapshot?.workspace ? 'finish' as const : 'wait' as const },
    { title: '目标与验收', description: hasDefinition ? 'Goal / DoD 已定义' : missingDefinition, status: hasDefinition ? 'finish' as const : 'process' as const },
    { title: '启动执行', description: currentRun ? `${currentRun.id} / ${stateLabel(currentRun.state)}` : '尚未启动 Run', status: currentRun ? 'finish' as const : 'wait' as const },
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

function skillScopeLabel(skill: Skill) {
  if (skill.source === 'skill-hub') {
    return skill.scope === 'workspace' ? '工作区 SkillHub' : '全局 SkillHub';
  }
  return '内置 Skill';
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
    evidenceRefs: asStringArray(asRecord(item)?.evidenceRefs)
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

function goalOutcomeSummary(snapshot?: RunSnapshot) {
	const summary = primaryResult(snapshot)?.summary || snapshot?.run.failureReason;
	return summary ? String(summary) : '尚未形成可确认的目标成果';
}

function goalOutcomeState(snapshot?: RunSnapshot) {
	const result = primaryResult(snapshot);
	if (result?.state === 'accepted') {
		return <Tag color="green">Result 已接受</Tag>;
	}
	if (result) {
		return stateTag(result.state);
	}
	const assessment = dodAssessment(snapshot);
	if (assessment?.satisfied === true) {
		return <Tag color="green">DoD 已满足</Tag>;
  }
  if (assessment?.satisfied === false) {
    return <Tag color="red">DoD 未满足</Tag>;
  }
  return snapshot ? stateTag(snapshot.run.state) : <Tag>未选择 Run</Tag>;
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
    run_state: 'Run 状态',
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

type ControlPlaneProps = {
  selectedWorkspaceId?: string;
  onOpenWorkspace: (workspaceId: string) => void;
};

type WorkspaceBoardItem = {
  snapshot: WorkspaceSnapshot;
  goals: Goal[];
  dods: DoD[];
};

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

  const trackWorkspaceActivationInBackground = (workspaceId: string) => {
    void workorchApi.activateWorkspaceExecution(workspaceId)
      .then(() => {
        message.success('工作空间执行已在后台启动');
        void loadBoard();
      })
      .catch((error: unknown) => {
        message.error(`工作空间执行后台启动失败：${errorMessage(error)}`);
        void loadBoard();
      });
  };

  const activateWorkspaceExecution = async (workspaceId: string) => {
    message.info('工作空间执行已提交，正在后台启动');
    trackWorkspaceActivationInBackground(workspaceId);
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
          {boardItems.map(({ snapshot, goals, dods }) => {
            const activeRun = snapshot.runs?.find((item) => item.id === snapshot.workspace.activeRunId);
            const currentRun = activeRun ?? runStateSummary(snapshot.runs).latest;
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
                extra={stateTag(snapshot.workspace.state)}
              >
                <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                  <div className="workorch-card-summary">
                    <div>
                      <Typography.Text type="secondary">最近 Run</Typography.Text>
                      <div>{runSummary.latest ? <Space>{shortId(runSummary.latest.id)}{stateTag(runSummary.latest.state)}</Space> : '-'}</div>
                    </div>
                    <div>
                      <Typography.Text type="secondary">治理阻塞</Typography.Text>
                      <div><Badge count={pendingDecisions + pendingApprovals} showZero color={pendingDecisions + pendingApprovals > 0 ? '#faad14' : '#52c41a'} /></div>
                    </div>
                  </div>
                  {metricGrid([
                    { key: 'goal', label: 'Goal', value: goals.length },
                    { key: 'dod', label: 'DoD', value: dods.length },
                    { key: 'run', label: 'Run', value: runSummary.total },
                    { key: 'completed', label: '完成', value: runSummary.completed, color: '#389e0d' },
                    { key: 'failed', label: '失败', value: runSummary.failed, color: '#cf1322' }
                  ])}
                  <Descriptions
                    column={1}
                    size="small"
                    items={[
                      { key: 'workingDirectory', label: '工作目录', children: boundedParagraph(snapshot.workspace.workingDirectory, { rows: 1, copyText: snapshot.workspace.workingDirectory }) },
                      { key: 'activeRun', label: '活跃运行', children: activeRun ? `${activeRun.id} / ${stateLabel(activeRun.state)}` : '-' },
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
                  {canStartRun ? <Button onClick={() => void activateWorkspaceExecution(snapshot.workspace.id)}>启动执行</Button> : null}
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

  const resumeRunInBackground = (runId: string) => {
    void workorchApi.resumeRun(runId)
      .then(() => {
        message.success(`Run ${runId} 已在后台继续执行`);
        actionRef.current?.reload();
      })
      .catch((error: unknown) => {
        message.error(`Run ${runId} 后台继续失败：${errorMessage(error)}`);
        actionRef.current?.reload();
      });
  };

  const resolve = async (item: Decision, resolutionType: string, payload?: Record<string, unknown>) => {
    try {
      await workorchApi.resolveDecision(item.id, resolutionType, payload);
      message.success(`决策已按 ${resolutionType} 处理，Run 已转入后台继续`);
      actionRef.current?.reload();
      resumeRunInBackground(item.runId);
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
      message.success(`决策已按 ${target.resolutionType} 处理，Run 已转入后台继续`);
      actionRef.current?.reload();
      resumeRunInBackground(target.item.runId);
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
      const decisions = await workorchApi.listDecisions();
      const selectedDecisions = decisions.filter((item) => ticketIds.includes(item.id));
      await workorchApi.batchResolveDecisions(ticketIds, resolutionType, { operator: 'console', mode: 'batch' });
      message.success(`${ticketIds.length} 个决策工单已按 ${resolutionType} 处理，相关 Run 已转入后台继续`);
      setSelectedDecisionKeys([]);
      actionRef.current?.reload();
      Array.from(new Set(selectedDecisions.map((item) => item.runId))).forEach(resumeRunInBackground);
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
      subTitle="处理阻塞 Run 的 Decision Ticket，支持补充信息、重新规划和拒绝当前路径"
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
                { key: 'run', label: '关联 Run', children: decisionTarget.item.runId },
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

  const resumeRunInBackground = (runId: string) => {
    void workorchApi.resumeRun(runId)
      .then(() => {
        message.success(`Run ${runId} 已在后台继续执行`);
        actionRef.current?.reload();
      })
      .catch((error: unknown) => {
        message.error(`Run ${runId} 后台继续失败：${errorMessage(error)}`);
        actionRef.current?.reload();
      });
  };

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
      message.success(`审批${actionLabel(target.action)}，Run 已转入后台继续`);
      actionRef.current?.reload();
      resumeRunInBackground(target.item.runId);
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
      const approvals = await workorchApi.listApprovals();
      const selectedApprovals = approvals.filter((item) => approvalIds.includes(item.id));
      await workorchApi.batchResolveApprovals(approvalIds, action);
      message.success(`${approvalIds.length} 个审批项${actionLabel(action)}，相关 Run 已转入后台继续`);
      setSelectedApprovalKeys([]);
      actionRef.current?.reload();
      Array.from(new Set(selectedApprovals.map((item) => item.runId))).forEach(resumeRunInBackground);
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
      subTitle="处理阻塞 Run 的 Approval，批准后 Run 可继续推进，拒绝后 Run 失败收口"
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
                { key: 'run', label: '关联 Run', children: approvalTarget.item.runId },
                { key: 'step', label: '步骤', children: approvalTarget.item.stepId || '-' },
                { key: 'reason', label: '审批原因', children: approvalTarget.item.reason || '-' }
              ]}
            />
            <Typography.Text type="secondary">已补充的决策信息</Typography.Text>
            <pre className="json-panel">{compactJSON(approvalTarget.relatedDecision?.resolutionPayload)}</pre>
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
      subTitle="统一处理人工决策与审批，确保阻塞 Run 能按正式业务语义继续、失败或重新规划"
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
  const [skills, setSkills] = useState<Skill[]>([]);
  const [mcpServers, setMCPServers] = useState<MCPServer[]>([]);
  const [memoryItems, setMemoryItems] = useState<MemoryItem[]>([]);
  const [configuration, setConfiguration] = useState<ServiceConfiguration | undefined>(undefined);
  const [runSnapshot, setRunSnapshot] = useState<RunSnapshot | undefined>(undefined);
  const [runObservability, setRunObservability] = useState<RunObservability | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [activationOpen, setActivationOpen] = useState(false);
  const [activationLoading, setActivationLoading] = useState(false);
  const [workspaceSetupOpen, setWorkspaceSetupOpen] = useState(false);
  const [goalDefinitionOpen, setGoalDefinitionOpen] = useState(false);
  const [dodDefinitionOpen, setDoDDefinitionOpen] = useState(false);
  const [businessSubmitting, setBusinessSubmitting] = useState(false);
  const [editingGoal, setEditingGoal] = useState<Goal | undefined>(undefined);
  const [editingDoD, setEditingDoD] = useState<DoD | undefined>(undefined);
  const [activationTracking, setActivationTracking] = useState(false);
  const [workspaceListReady, setWorkspaceListReady] = useState(false);
  const [setupStep, setSetupStep] = useState(0);
  const [createdWorkspace, setCreatedWorkspace] = useState<Workspace | undefined>(undefined);
  const workspaceListRequestSeq = useRef(0);
  const workspaceContextRequestSeq = useRef(0);
  const [activationForm] = Form.useForm();
  const [workspaceForm] = Form.useForm();
  const [setupGoalForm] = Form.useForm();
  const [setupDoDForm] = Form.useForm();
  const [goalForm] = Form.useForm();
  const [dodForm] = Form.useForm();

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
    if (!workspaceId || !activationTracking) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      void loadWorkspaceContext(workspaceId).catch((error: unknown) => {
        message.error(errorMessage(error));
      });
    }, 3000);
    return () => window.clearInterval(timer);
  }, [workspaceId, activationTracking]);

  const readyDoDs = dods.filter((item) => item.state === 'ready' || item.state === 'satisfied');
  const llmProviders = configuration?.llm.providers ?? [];

  function clearWorkspaceContext() {
    setSnapshot(undefined);
    setGoals([]);
    setDoDs([]);
    setMemoryItems([]);
    setRunSnapshot(undefined);
    setRunObservability(undefined);
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
    const [nextSnapshot, nextGoals, nextDoDs, nextSouls, nextSkills, nextMCPServers, nextMemoryItems, nextConfiguration] = await Promise.all([
      workorchApi.readWorkspaceSnapshot(targetWorkspaceId),
      workorchApi.listGoals(targetWorkspaceId),
      workorchApi.listDoDs(targetWorkspaceId),
      workorchApi.listSouls(),
      workorchApi.listSkills(targetWorkspaceId),
      workorchApi.listMCPServers(),
      workorchApi.listMemory(targetWorkspaceId),
      workorchApi.readConfiguration()
    ]);
    if (requestSeq !== workspaceContextRequestSeq.current) {
      return;
    }
    setSnapshot(nextSnapshot);
    setGoals(nextGoals);
    setDoDs(nextDoDs);
    setSouls(nextSouls);
    setSkills(nextSkills);
    setMCPServers(nextMCPServers);
    setMemoryItems(nextMemoryItems);
    setConfiguration(nextConfiguration);
    const latestRun = runStateSummary(nextSnapshot.runs).latest;
    if (latestRun) {
      const [nextRunSnapshot, nextRunObservability] = await Promise.all([
        workorchApi.readRunSnapshot(latestRun.id),
        workorchApi.readRunObservability(latestRun.id)
      ]);
      if (requestSeq !== workspaceContextRequestSeq.current) {
        return;
      }
      setRunSnapshot(nextRunSnapshot);
      setRunObservability(nextRunObservability);
    } else {
      setRunSnapshot(undefined);
      setRunObservability(undefined);
    }
  }

  const refreshWorkspace = async () => {
    if (!workspaceId) {
      return;
    }
    await loadWorkspaceContext(workspaceId);
  };

  const closeWorkspaceSetup = () => {
    setWorkspaceSetupOpen(false);
    setSetupStep(0);
    setCreatedWorkspace(undefined);
    workspaceForm.resetFields();
    setupGoalForm.resetFields();
    setupDoDForm.resetFields();
  };

  const activateWorkspaceExecution = (request: WorkspaceExecutionActivationRequest) => {
    if (!workspaceId) {
      return;
    }
    const targetWorkspaceId = workspaceId;
    setActivationLoading(true);
    setActivationOpen(false);
    setActivationTracking(true);
    activationForm.resetFields();
    message.info('工作空间执行已提交，已转入后台跟踪');
    void workorchApi.activateWorkspaceExecution(targetWorkspaceId, request)
      .then(async (activation) => {
        setSnapshot(activation.snapshot);
        setRunSnapshot(undefined);
        setRunObservability(undefined);
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
      soulId: souls[0]?.id,
      skillIds: [],
      mcpServerIds: [],
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
    { title: 'Run ID', dataIndex: 'id', copyable: true },
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
  const outcomeResult = primaryResult(runSnapshot);
  const outcomeArtifacts = outcomeResult
    ? (runSnapshot?.artifacts ?? []).filter((item) => item.resultId === outcomeResult.id || !item.resultId)
    : (runSnapshot?.artifacts ?? []);
  const outcomeMemoryRefs = memoryRefIDs(runSnapshot);
  const outcomeMemories = memoryItems.filter((item) => item.runId === outcomeRun?.id || outcomeMemoryRefs.has(item.id));
  const outcomeDoDItems = dodAssessmentItems(runSnapshot);
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
      subTitle="围绕 Workspace 管理目标、验收标准、执行过程、治理阻塞和运行证据"
      extra={
        <Space wrap>
          <Button onClick={() => setWorkspaceSetupOpen(true)}>创建工作空间</Button>
          <Button disabled={!workspaceId} onClick={() => openGoalDefinition()}>定义目标</Button>
          <Button disabled={!workspaceId} onClick={() => openDoDDefinition()}>定义验收标准</Button>
          <Button type="primary" disabled={!workspaceId} onClick={requestWorkspaceActivation}>启动执行</Button>
          {activationTracking ? <Tag color="processing">后台跟踪中</Tag> : null}
          <Select
            style={{ width: 360 }}
            placeholder="选择工作空间"
            value={workspaceId}
            onChange={setWorkspaceId}
            options={workspaces.map((item) => ({ value: item.id, label: `${item.title || item.id} (${item.id})` }))}
          />
        </Space>
      }
    >
      {snapshot ? (
        <>
          <ProCard loading={loading} title={snapshot.workspace.title || snapshot.workspace.id} extra={stateTag(snapshot.workspace.state)}>
            <Descriptions
              column={2}
              items={[
                { key: 'id', label: '工作空间 ID', children: snapshot.workspace.id },
                { key: 'activeRun', label: '活跃运行', children: snapshot.workspace.activeRunId || '-' },
                { key: 'description', label: '描述', children: boundedParagraph(snapshot.workspace.description, { rows: 2 }) },
                { key: 'workingDirectory', label: '工作目录', children: boundedParagraph(snapshot.workspace.workingDirectory, { rows: 1, copyText: snapshot.workspace.workingDirectory }) }
              ]}
            />
          </ProCard>
          <ProCard title="执行过程" className="workorch-section">
            <Steps items={workspaceProcessItems(snapshot, goals, dods)} />
          </ProCard>
          <ProCard title="主要业务模型" className="workorch-section">
            {metricGrid([
              { key: 'workspace', label: 'Workspace', value: 1, color: '#1668dc' },
              { key: 'goal', label: 'Goal', value: goals.length },
              { key: 'dod', label: 'DoD', value: dods.length },
              { key: 'run', label: 'Run', value: snapshot.runs?.length ?? 0, color: '#0958d9' },
              { key: 'decision', label: 'Decision', value: snapshot.decisions?.length ?? 0 },
              { key: 'approval', label: 'Approval', value: snapshot.approvals?.length ?? 0 },
              { key: 'soul', label: 'Soul', value: souls.length },
              { key: 'skill', label: 'Skill', value: skills.length },
              { key: 'mcp', label: 'MCP Server', value: mcpServers.length },
              { key: 'memory', label: 'Memory', value: memoryItems.length },
              { key: 'step', label: 'Step', value: runSnapshot?.steps?.length ?? 0 },
              { key: 'result', label: 'Result', value: snapshot.results?.length ?? runSnapshot?.results?.length ?? 0, color: '#389e0d' },
              { key: 'artifact', label: 'Artifact', value: snapshot.artifacts?.length ?? runSnapshot?.artifacts?.length ?? 0 },
              { key: 'tool', label: 'ToolCall', value: runSnapshot?.toolCalls?.length ?? 0 },
              { key: 'patch', label: 'Patch', value: runSnapshot?.patches?.length ?? 0 },
              { key: 'validation', label: 'Validation', value: runSnapshot?.validationResults?.length ?? 0 },
              { key: 'checkpoint', label: 'Checkpoint', value: runSnapshot?.runtimeCheckpoints?.length ?? 0 }
            ])}
          </ProCard>
          <ProCard title="运行摘要" className="workorch-section">
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              {metricGrid([
                { key: 'runs', label: 'Run 总数', value: runSummary.total },
                { key: 'created', label: '已创建', value: runSummary.created },
                { key: 'active', label: '运行中', value: runSummary.active, color: '#0958d9' },
                { key: 'blocked', label: '阻塞中', value: runSummary.blocked, color: '#d48806' },
                { key: 'validating', label: '验收中', value: runSummary.validating, color: '#08979c' },
                { key: 'completed', label: '已完成', value: runSummary.completed, color: '#389e0d' },
                { key: 'failed', label: '失败/终止', value: runSummary.failed, color: '#cf1322' },
                { key: 'decisions', label: '待处理决策', value: pendingCount(snapshot.decisions), color: pendingCount(snapshot.decisions) ? '#d48806' : undefined },
                { key: 'approvals', label: '待处理审批', value: pendingCount(snapshot.approvals), color: pendingCount(snapshot.approvals) ? '#d48806' : undefined }
              ])}
              <Descriptions
                column={1}
                size="small"
                items={[
                  { key: 'latestRun', label: '最近运行', children: runSummary.latest ? `${runSummary.latest.id} / ${stateLabel(runSummary.latest.state)}` : '-' }
                ]}
              />
            </Space>
          </ProCard>
          <ProCard gutter={16} wrap className="workorch-section">
            <ProCard title="目标定义（Goal）" colSpan={{ xs: 24, lg: 12 }} extra={<Tag>{goals.length} 项</Tag>}>
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
            <ProCard title="验收标准（DoD）" colSpan={{ xs: 24, lg: 12 }} extra={<Tag>{dodDefinitionRows.length} 项</Tag>}>
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
                    { key: 'run', label: '关联 Run', children: `${runSnapshot.run.id} / ${stateLabel(runSnapshot.run.state)}` },
                    { key: 'result', label: '正式 Result', children: outcomeResult ? boundedParagraph(outcomeResult.id, { rows: 1, copyText: outcomeResult.id }) : '-' },
                    { key: 'goal', label: '目标', children: boundedParagraph(outcomeGoal?.text || runSnapshot.run.goalId, { rows: 2 }) },
                    { key: 'dod', label: '验收标准', children: outcomeDoD?.items?.length ? compactTags(outcomeDoD.items) : runSnapshot.run.doDId || '-' },
                    { key: 'summary', label: '成果结论', children: boundedParagraph(goalOutcomeSummary(runSnapshot), { rows: 3, className: 'goal-outcome-summary' }) },
                    { key: 'source', label: '结果来源', children: outcomeResult ? `${outcomeResult.sourceType || '-'} / ${outcomeResult.sourceRef || '-'}` : '-' },
                    { key: 'evidence', label: '结果证据', children: outcomeResult?.evidenceRefs?.length ? compactTags(outcomeResult.evidenceRefs) : '-' }
                  ]}
                />
                <ProCard gutter={16} wrap>
                  <ProCard title="验收项确认" colSpan={{ xs: 24, lg: 12 }}>
                    {outcomeDoDItems.length ? (
                      <ProTable
                        rowKey="key"
                        search={false}
                        options={false}
                        pagination={false}
                        size="small"
                        columns={[
                          { title: '验收项', dataIndex: 'item', ellipsis: true },
                          { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(String(item.state)) },
                          { title: '证据', dataIndex: 'evidenceRefs', ellipsis: true, render: (_, item) => (item.evidenceRefs as string[]).join(', ') || '-' }
                        ]}
                        dataSource={outcomeDoDItems}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Run 尚未输出 DoD 评估" />
                    )}
                  </ProCard>
                  <ProCard title="正式交付物（Artifact）" colSpan={{ xs: 24, lg: 12 }}>
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
                          { title: '状态', dataIndex: 'state', width: 110, render: (_, item) => stateTag(item.state) },
                          { title: '摘要', dataIndex: 'summary', ellipsis: true }
                        ]}
                        dataSource={outcomeArtifacts}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Run 尚未形成正式 Artifact" />
                    )}
                  </ProCard>
                </ProCard>
                <ProCard gutter={16} wrap>
                  <ProCard title="工具调用成果" colSpan={{ xs: 24, lg: 12 }}>
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
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Run 未产生工具调用" />
                    )}
                  </ProCard>
                  <ProCard title="Memory 输出" colSpan={{ xs: 24, lg: 12 }}>
                    {outcomeMemories.length ? (
                      <ProTable<MemoryItem>
                        rowKey="id"
                        search={false}
                        options={false}
                        pagination={{ pageSize: 5 }}
                        size="small"
                        columns={[
                          { title: '记忆', dataIndex: 'summary', ellipsis: true },
                          { title: '类型', dataIndex: 'memoryType', width: 130 },
                          { title: '来源', dataIndex: 'sourceType', width: 100 },
                          { title: '向量', width: 90, render: (_, item) => item.vectorReady ? <Tag color="cyan">{item.vectorDimension}</Tag> : '-' }
                        ]}
                        dataSource={outcomeMemories}
                      />
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Run 未产生或引用 Memory 成果" />
                    )}
                  </ProCard>
                </ProCard>
              </Space>
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请先启动或选择一个 Run 查看目标成果" />
            )}
          </ProCard>
          <ProCard title="运行记录（Run）" className="workorch-section">
            <ProTable<Run> rowKey="id" search={false} options={false} pagination={false} columns={runColumns} dataSource={snapshot.runs ?? []} />
          </ProCard>
          <ProCard gutter={16} wrap className="workorch-section">
            <ProCard title="决策工单（Decision）" colSpan="50%">
              <ProTable<Decision> rowKey="id" search={false} options={false} pagination={false} columns={[
                { title: '工单', dataIndex: 'id', copyable: true },
                { title: '类型', dataIndex: 'decisionKind', width: 150 },
                { title: '问题', dataIndex: 'question', ellipsis: true, render: (_, item) => boundedParagraph(item.question, { rows: 2 }) },
                { title: '原因', dataIndex: 'reason', ellipsis: true },
                { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
                { title: '解决方式', dataIndex: 'resolutionType' }
              ]} dataSource={snapshot.decisions ?? []} expandable={{
                expandedRowRender: (item) => (
                  <ProCard gutter={16} wrap ghost>
                    <ProCard title="上下文" colSpan={{ xs: 24, lg: 12 }}><pre className="json-panel json-panel-large">{compactJSON(item.context)}</pre></ProCard>
                    <ProCard title="解决内容" colSpan={{ xs: 24, lg: 12 }}><pre className="json-panel json-panel-large">{compactJSON(item.resolutionPayload)}</pre></ProCard>
                  </ProCard>
                )
              }} />
            </ProCard>
            <ProCard title="审批工单（Approval）" colSpan="50%">
              <ProTable<Approval> rowKey="id" search={false} options={false} pagination={false} columns={[
                { title: '审批', dataIndex: 'id', copyable: true },
                { title: '运行', dataIndex: 'runId', copyable: true, ellipsis: true },
                { title: '步骤', dataIndex: 'stepId', copyable: true, ellipsis: true },
                { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
                { title: '原因', dataIndex: 'reason' }
              ]} dataSource={snapshot.approvals ?? []} />
            </ProCard>
          </ProCard>
          <ProCard title="运行过程与证据" className="workorch-section" extra={runSnapshot ? <Tag>{evidenceCount(runSnapshot)} 条证据</Tag> : null}>
            {runSnapshot ? (
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <Steps
                  size="small"
                  items={[
                    { title: '调度', description: stateLabel(runSnapshot.run.state), status: 'finish' },
                    { title: '步骤', description: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0}`, status: runObservability?.progress.stepFailed ? 'error' : 'finish' },
                    { title: '工具调用', description: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runSnapshot.toolCalls?.length ?? 0}`, status: 'finish' },
                    { title: '校验', description: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? runSnapshot.validationResults?.length ?? 0}`, status: runSnapshot.run.state === 'failed' ? 'error' : 'finish' },
                    { title: '终态', description: runObservability?.progress.terminal ? '已进入终态' : '尚未终态', status: runObservability?.progress.terminal ? 'finish' : 'process' }
                  ]}
                />
                {metricGrid([
                  { key: 'steps', label: '步骤完成', value: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0}`, color: runObservability?.progress.stepFailed ? '#cf1322' : '#0958d9' },
                  { key: 'tools', label: '工具完成', value: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runSnapshot.toolCalls?.length ?? 0}` },
                  { key: 'validations', label: '校验通过', value: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? runSnapshot.validationResults?.length ?? 0}`, color: runObservability?.evidence.validationFailed ? '#cf1322' : '#389e0d' },
                  { key: 'patches', label: 'Patch 应用', value: `${runObservability?.evidence.patchApplied ?? 0}/${runObservability?.evidence.patchTotal ?? runSnapshot.patches?.length ?? 0}` },
                  { key: 'results', label: 'Result', value: runObservability?.evidence.resultTotal ?? runSnapshot.results?.length ?? 0, color: '#389e0d' },
                  { key: 'artifacts', label: 'Artifact', value: runObservability?.evidence.artifactTotal ?? runSnapshot.artifacts?.length ?? 0 }
                ])}
                <Descriptions
                  column={3}
                  items={[
                    { key: 'run', label: '运行', children: `${runSnapshot.run.id} / ${stateLabel(runSnapshot.run.state)}` },
                    { key: 'currentStep', label: '当前步骤', children: runObservability?.progress.currentStepId || runSnapshot.steps?.at(-1)?.id || '-' },
                    { key: 'terminal', label: '是否终态', children: runObservability ? (runObservability.progress.terminal ? '是' : '否') : '-' },
                    { key: 'steps', label: '步骤', children: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0} 已完成` },
                    { key: 'toolCalls', label: '工具调用', children: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runSnapshot.toolCalls?.length ?? 0} 已完成` },
                    { key: 'patches', label: 'Patch', children: `${runObservability?.evidence.patchApplied ?? 0}/${runObservability?.evidence.patchTotal ?? runSnapshot.patches?.length ?? 0} 已应用` },
                    { key: 'validations', label: '校验结果', children: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? runSnapshot.validationResults?.length ?? 0} 通过` },
                    { key: 'checkpoints', label: '运行检查点', children: runObservability?.evidence.runtimeCheckpointTotal ?? runSnapshot.runtimeCheckpoints?.length ?? 0 },
                    { key: 'results', label: '正式 Result', children: runObservability?.evidence.resultTotal ?? runSnapshot.results?.length ?? 0 },
                    { key: 'artifacts', label: '正式 Artifact', children: runObservability?.evidence.artifactTotal ?? runSnapshot.artifacts?.length ?? 0 },
                    { key: 'events', label: '事件', children: runObservability?.evidence.eventTotal ?? 0 },
                    { key: 'context', label: '执行上下文', children: <pre className="json-panel">{compactJSON(runSnapshot.run.executionContext)}</pre> }
                  ]}
                />
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
                        { key: 'state', label: 'Run 状态', children: stateLabel(runObservability.latestCheckpoint.runStateSnapshot) },
                        { key: 'step', label: '当前步骤', children: boundedParagraph(runObservability.latestCheckpoint.currentStepId, { rows: 1, copyText: runObservability.latestCheckpoint.currentStepId }) },
                        { key: 'blocked', label: '阻塞信息', children: runObservability.latestCheckpoint.blockingTicketId || runObservability.latestCheckpoint.blockingApprovalId || '无阻塞' }
                      ]}
                    />
                  ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前 Run 尚未记录检查点" />
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
                            { title: 'Run 状态', dataIndex: 'runState', width: 130, render: (_, item) => stateLabel(item.runState) },
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
                      label: `工具 ${runSnapshot.toolCalls?.length ?? 0}`,
                      children: (
                        <ProTable<ToolCall>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: '工具调用', dataIndex: 'id', copyable: true, ellipsis: true },
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
                      label: `校验 ${runSnapshot.validationResults?.length ?? 0}`,
                      children: (
                        <ProTable<ValidationResult>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: '校验', dataIndex: 'id', copyable: true, ellipsis: true },
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
                      label: `Patch ${runSnapshot.patches?.length ?? 0}`,
                      children: (
                        <ProTable<Patch>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: 'Patch', dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '类型', dataIndex: 'patchType', width: 140 },
                            { title: '目标', dataIndex: 'targetRef', ellipsis: true },
                            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                            { title: '摘要', dataIndex: 'summary', ellipsis: true }
                          ]}
                          dataSource={runSnapshot.patches ?? []}
                        />
                      )
                    },
                    {
                      key: 'checkpoints',
                      label: `检查点 ${runSnapshot.runtimeCheckpoints?.length ?? 0}`,
                      children: (
                        <ProTable<RuntimeCheckpoint>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: '检查点', dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '类型', dataIndex: 'checkpointType', width: 180 },
                            { title: 'Run 状态', dataIndex: 'runStateSnapshot', width: 140, render: (_, item) => stateTag(item.runStateSnapshot) },
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
                      label: `Result ${runSnapshot.results?.length ?? 0}`,
                      children: (
                        <ProTable<Result>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: 'Result', dataIndex: 'id', copyable: true, ellipsis: true },
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
                      label: `Artifact ${runSnapshot.artifacts?.length ?? 0}`,
                      children: (
                        <ProTable<Artifact>
                          rowKey="id"
                          search={false}
                          options={false}
                          pagination={{ pageSize: 8 }}
                          size="small"
                          columns={[
                            { title: 'Artifact', dataIndex: 'id', copyable: true, ellipsis: true },
                            { title: '类型', dataIndex: 'artifactType', width: 140 },
                            { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                            { title: '名称/URI', render: (_, item) => item.name || item.uri || '-', ellipsis: true },
                            { title: '摘要', dataIndex: 'summary', ellipsis: true }
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
        title="创建工作空间"
        open={workspaceSetupOpen}
        confirmLoading={businessSubmitting}
        onCancel={closeWorkspaceSetup}
        onOk={() => {
          if (setupStep === 0) {
            workspaceForm.submit();
          } else if (setupStep === 1) {
            setupGoalForm.submit();
          } else {
            setupDoDForm.submit();
          }
        }}
        okText={setupStep === 0 ? '创建工作空间' : setupStep === 1 ? '定义目标' : '定义验收标准'}
      >
        <Steps
          current={setupStep}
          className="workorch-section"
          items={[
            { title: '工作空间' },
            { title: '目标' },
            { title: '验收标准' }
          ]}
        />
        <Form
          form={workspaceForm}
          layout="vertical"
          style={{ display: setupStep === 0 ? 'block' : 'none' }}
          onFinish={async (values: { title: string; description?: string; workingDirectory?: string }) => {
            setBusinessSubmitting(true);
            try {
              const workspace = await workorchApi.createWorkspace(values.title, values.description, values.workingDirectory);
              setCreatedWorkspace(workspace);
              await loadWorkspaceList(workspace.id);
              setSetupStep(1);
              message.success('工作空间已创建，请继续定义目标');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item name="title" label="标题" rules={[{ required: true }]}>
            <Input placeholder="本地 workorch 工作空间" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea rows={3} placeholder="描述工作空间目标、范围或当前交付预期" />
          </Form.Item>
          <Form.Item name="workingDirectory" label="工作目录">
            <Input placeholder="留空时自动使用服务工作路径 / workspaces / 生成的工作空间 ID" />
          </Form.Item>
        </Form>
        <Form
          form={setupGoalForm}
          layout="vertical"
          style={{ display: setupStep === 1 ? 'block' : 'none' }}
          onFinish={async (values: { text: string }) => {
            if (!createdWorkspace) {
              message.error('工作空间尚未创建');
              return;
            }
            setBusinessSubmitting(true);
            try {
              await workorchApi.defineGoal(createdWorkspace.id, values.text);
              await loadWorkspaceContext(createdWorkspace.id);
              setSetupStep(2);
              message.success('目标已定义，请继续定义验收标准');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item label="工作空间">
            <Input value={createdWorkspace?.id || ''} disabled />
          </Form.Item>
          <Form.Item name="text" label="目标" rules={[{ required: true }]}>
            <Input.TextArea rows={4} placeholder="描述该工作空间要达成的业务结果" />
          </Form.Item>
        </Form>
        <Form
          form={setupDoDForm}
          layout="vertical"
          style={{ display: setupStep === 2 ? 'block' : 'none' }}
          onFinish={async (values: { items: string }) => {
            if (!createdWorkspace) {
              message.error('工作空间尚未创建');
              return;
            }
            setBusinessSubmitting(true);
            try {
              const items = values.items.split('\n').map((item) => item.trim()).filter(Boolean);
              await workorchApi.defineDoD(createdWorkspace.id, items);
              await loadWorkspaceContext(createdWorkspace.id);
              closeWorkspaceSetup();
              message.success('工作空间目标和验收标准已就绪');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item label="工作空间">
            <Input value={createdWorkspace?.id || ''} disabled />
          </Form.Item>
          <Form.Item name="items" label="验收项" rules={[{ required: true }]}>
            <Input.TextArea rows={5} placeholder={'每行一个验收标准\n示例：服务已完成本地部署并可访问'} />
          </Form.Item>
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
          <Form.Item name="soulId" label="执行人格（Soul）">
            <Select allowClear options={souls.map((item) => ({ value: item.id, label: `${item.name || item.id} (${item.id})` }))} />
          </Form.Item>
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

function RuntimeControlPage() {
  const { message } = AntApp.useApp();
  const [serverForm] = Form.useForm<MCPServerFormValues>();
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [selectedServerId, setSelectedServerId] = useState<string | undefined>(undefined);
  const [tools, setTools] = useState<MCPTool[]>([]);
  const [resources, setResources] = useState<MCPResource[]>([]);
  const [prompts, setPrompts] = useState<MCPPrompt[]>([]);
  const [metrics, setMetrics] = useState('');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const selectedServer = servers.find((item) => item.id === selectedServerId);

  const loadRuntime = async () => {
    setLoading(true);
    try {
      const [nextServers, nextMetrics] = await Promise.all([
        workorchApi.listMCPServers(),
        workorchApi.metrics()
      ]);
      setServers(nextServers);
      setMetrics(nextMetrics);
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
      const [nextTools, nextResources, nextPrompts] = await Promise.all([
        workorchApi.listMCPServerTools(serverId),
        workorchApi.listMCPServerResources(serverId),
        workorchApi.listMCPServerPrompts(serverId)
      ]);
      setTools(nextTools);
      setResources(nextResources);
      setPrompts(nextPrompts);
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
      endpoint: server.endpoint,
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

  const createServer = async (values: MCPServerFormValues) => {
    setSaving(true);
    try {
      const server = await workorchApi.createMCPServer(mcpServerPayload(values));
      message.success(`MCP 服务 ${server.id} 已创建`);
      await loadRuntime();
      setSelectedServerId(server.id);
      serverForm.resetFields();
      resetServerDraft();
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

  return (
    <PageContainer
      title="运行控制"
      subTitle="管理 MCP 运行能力，并观察本地运行队列与并发压力"
      extra={<Button onClick={() => void loadRuntime()} loading={loading}>刷新</Button>}
    >
      <ProCard gutter={16} wrap loading={loading}>
        <ProCard title="运行调度" colSpan={{ xs: 24, lg: 8 }}>
          <Descriptions
            column={1}
            items={[
              { key: 'max', label: '最大并发 Run', children: metricValue(metrics, 'workorch_server_max_concurrent_runs') },
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
            <Tag color="blue">{servers.reduce((total, item) => total + (item.allowedTools?.length ?? 0), 0)} 个工具</Tag>
            <Tag color="cyan">{servers.reduce((total, item) => total + (item.allowedResources?.length ?? 0), 0)} 个资源</Tag>
            <Tag color="geekblue">{servers.reduce((total, item) => total + (item.allowedPrompts?.length ?? 0), 0)} 个提示词</Tag>
            <Tag color="purple">{servers.filter((item) => item.sessionPolicy === 'long_lived').length} 个长会话</Tag>
          </Space>
        </ProCard>
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
                  { key: 'endpoint', label: '端点', children: selectedServer.endpoint || '-' },
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
          title="创建 MCP 服务"
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
            onFinish={(values) => void createServer(values)}
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
            <Button type="primary" htmlType="submit" loading={saving}>创建 MCP 服务</Button>
          </Form>
        </ProCard>
      </ProCard>
    </PageContainer>
  );
}

function ConfigurationPage() {
  const { message } = AntApp.useApp();
  const [providerEditorForm] = Form.useForm<UpdateLLMProviderConfiguration>();
  const [configuration, setConfiguration] = useState<ServiceConfiguration | undefined>(undefined);
  const [defaultProviderId, setDefaultProviderId] = useState<string | undefined>(undefined);
  const [defaultEmbeddingProviderId, setDefaultEmbeddingProviderId] = useState<string | undefined>(undefined);
  const [providerDrafts, setProviderDrafts] = useState<UpdateLLMProviderConfiguration[]>([]);
  const [selectedProviderIndex, setSelectedProviderIndex] = useState(0);
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  const loadConfiguration = async () => {
    setLoading(true);
    try {
      const nextConfiguration = await workorchApi.readConfiguration();
      const nextFormValues = llmConfigurationFormValues(nextConfiguration);
      setConfiguration(nextConfiguration);
      setDefaultProviderId(nextFormValues.defaultProviderId);
      setDefaultEmbeddingProviderId(nextFormValues.defaultEmbeddingProviderId);
      setProviderDrafts(nextFormValues.providers);
      setSelectedProviderIndex(0);
      setSelectedTemplateId(nextConfiguration.llm.providerTemplates?.[0]?.templateId);
      providerEditorForm.setFieldsValue(nextFormValues.providers[0] ?? nextProviderDraft([]));
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
  return (
    <PageContainer
      title="系统配置"
      subTitle="管理服务配置、仓储配置和运行期生效的 LLM Provider"
      extra={<Button onClick={() => void loadConfiguration()} loading={loading}>刷新</Button>}
    >
      <ProCard gutter={16} wrap loading={loading}>
        <ProCard title="服务参数" colSpan="50%">
          <Descriptions
            column={1}
            items={[
              { key: 'addr', label: '监听地址', children: configuration?.server.addr || '-' },
              { key: 'readHeaderTimeout', label: '读 Header 超时', children: configuration?.server.readHeaderTimeout || '-' },
              { key: 'readTimeout', label: '读超时', children: configuration?.server.readTimeout || '-' },
              { key: 'writeTimeout', label: '写超时', children: configuration?.server.writeTimeout || '-' },
              { key: 'idleTimeout', label: '空闲超时', children: configuration?.server.idleTimeout || '-' },
              { key: 'shutdownTimeout', label: '关闭超时', children: configuration?.server.shutdownTimeout || '-' },
              { key: 'maxConcurrentRuns', label: '最大并发 Run', children: configuration?.server.maxConcurrentRuns ?? '-' }
            ]}
          />
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
        title="LLM 服务商"
        className="workorch-section"
        extra={<Button type="primary" onClick={() => void providerEditorForm.validateFields().then(() => saveLLMConfiguration())} loading={saving}>保存 LLM 服务商</Button>}
      >
        <Typography.Paragraph type="secondary">
          API 密钥读取时会被隐藏。编辑时留空表示保留已有密钥；保存后的服务商配置会由 workorchd 运行期重新加载，并用于后续 LLM 步骤。
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
              配置模板按 provider 模式生成草稿：OpenAI 与 Anthropic 使用官方 SDK；compatible 用于 DeepSeek、MiniMax、Ollama 或本地兼容服务。
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
    <PageContainer title="服务状态" subTitle="查看 workorchd 健康状态、就绪状态和运行指标">
      <ProCard gutter={16} wrap>
        <ProCard title="健康检查" colSpan="50%">
          <Descriptions items={Object.entries(health).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
        </ProCard>
        <ProCard title="就绪检查" colSpan="50%">
          <Descriptions items={Object.entries(ready).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
        </ProCard>
      </ProCard>
      <ProCard title="运行指标（Metrics）" className="workorch-section">
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
