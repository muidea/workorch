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
import { App as AntApp, Button, ConfigProvider, Descriptions, Empty, Form, Input, InputNumber, Modal, Select, Space, Steps, Tabs, Tag, Typography, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import type { Approval, Decision, DoD, Goal, MCPPrompt, MCPResource, MCPServer, MCPTool, MemoryItem, Run, RunObservability, RunSnapshot, RuntimeTimelineItem, ServiceConfiguration, Skill, Soul, StepView, UpdateLLMConfigurationRequest, UpdateLLMProviderConfiguration, Workspace, WorkspaceExecutionActivationRequest, WorkspaceSnapshot } from './types';
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

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : '操作失败';
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

function workspaceProcessItems(snapshot?: WorkspaceSnapshot, goals?: Goal[], dods?: DoD[]) {
  const latestRun = runStateSummary(snapshot?.runs).latest;
  const hasDefinition = Boolean((goals?.length ?? 0) > 0 && (dods?.length ?? 0) > 0);
  const blocked = latestRun?.state === 'blocked_on_decision' || latestRun?.state === 'blocked_on_approval';
  const terminal = latestRun?.state === 'completed' || latestRun?.state === 'failed' || latestRun?.state === 'cancelled' || latestRun?.state === 'superseded';
  return [
    { title: '工作空间', description: snapshot?.workspace ? stateLabel(snapshot.workspace.state) : '未选择', status: snapshot?.workspace ? 'finish' as const : 'wait' as const },
    { title: '目标与验收', description: hasDefinition ? 'Goal / DoD 已定义' : '需要补齐 Goal / DoD', status: hasDefinition ? 'finish' as const : 'process' as const },
    { title: '启动执行', description: latestRun ? `${latestRun.id} / ${stateLabel(latestRun.state)}` : '尚未启动 Run', status: latestRun ? 'finish' as const : 'wait' as const },
    { title: '治理处理', description: blocked ? stateLabel(latestRun?.state) : '无待处理阻塞', status: blocked ? 'process' as const : latestRun ? 'finish' as const : 'wait' as const },
    { title: '证据验收', description: terminal ? stateLabel(latestRun?.state) : '等待运行证据闭环', status: latestRun?.state === 'failed' ? 'error' as const : terminal ? 'finish' as const : 'wait' as const }
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

function evidenceCount(snapshot?: RunSnapshot) {
  if (!snapshot) {
    return 0;
  }
  return (snapshot.toolCalls?.length ?? 0) +
    (snapshot.patches?.length ?? 0) +
    (snapshot.validationResults?.length ?? 0) +
    (snapshot.runtimeCheckpoints?.length ?? 0);
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

function outputText(output: Record<string, unknown> | undefined, key: string) {
  const value = output?.[key];
  if (value === undefined || value === null || value === '') {
    return '-';
  }
  return typeof value === 'string' ? value : compactJSON(value);
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

function nextProviderDraft(existing: UpdateLLMProviderConfiguration[]): UpdateLLMProviderConfiguration {
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

function WorkspaceBoardPage({ onOpenWorkspace }: ControlPlaneProps) {
  const { message } = AntApp.useApp();
  const [snapshots, setSnapshots] = useState<WorkspaceSnapshot[]>([]);
  const [loading, setLoading] = useState(false);

  const loadBoard = async () => {
    setLoading(true);
    try {
      const workspaces = await workorchApi.listWorkspaces();
      const nextSnapshots = await Promise.all(workspaces.map((item) => workorchApi.readWorkspaceSnapshot(item.id)));
      setSnapshots(nextSnapshots);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadBoard();
  }, []);

  const activateWorkspaceExecution = async (workspaceId: string) => {
    try {
      await workorchApi.activateWorkspaceExecution(workspaceId);
      message.success('工作空间执行已启动');
      await loadBoard();
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  return (
    <PageContainer
      title="业务总览"
      subTitle="从工作空间视角查看目标、运行、治理阻塞和交付状态"
      extra={<Button onClick={() => void loadBoard()} loading={loading}>刷新</Button>}
    >
      {snapshots.length === 0 && !loading ? (
        <Empty description="暂无工作空间" />
      ) : (
        <ProCard gutter={16} wrap loading={loading}>
          {snapshots.map((snapshot) => {
            const activeRun = snapshot.runs?.find((item) => item.id === snapshot.workspace.activeRunId);
            const runSummary = runStateSummary(snapshot.runs);
            const pendingDecisions = pendingCount(snapshot.decisions);
            const pendingApprovals = pendingCount(snapshot.approvals);
            return (
              <ProCard
                key={snapshot.workspace.id}
                title={snapshot.workspace.title || snapshot.workspace.id}
                subTitle={snapshot.workspace.id}
                colSpan={{ xs: 24, md: 12, xl: 8 }}
                bordered
                extra={stateTag(snapshot.workspace.state)}
              >
                <Descriptions
                  column={1}
                  items={[
                    { key: 'workingDirectory', label: '工作目录', children: snapshot.workspace.workingDirectory },
                    { key: 'activeRun', label: '活跃运行', children: activeRun ? `${activeRun.id} / ${stateLabel(activeRun.state)}` : '-' },
                    { key: 'runtimeSummary', label: '运行摘要', children: runtimeSummaryTags(snapshot.runs) },
                    { key: 'latestRun', label: '最近运行', children: runSummary.latest ? `${runSummary.latest.id} / ${stateLabel(runSummary.latest.state)}` : '-' },
                    { key: 'decisions', label: '待处理决策', children: pendingDecisions },
                    { key: 'approvals', label: '待处理审批', children: pendingApprovals }
                  ]}
                />
                <Steps
                  size="small"
                  className="workorch-section"
                  items={workspaceProcessItems(snapshot)}
                />
                <Space className="workorch-section" wrap>
                  <Button type="primary" onClick={() => onOpenWorkspace(snapshot.workspace.id)}>进入工作空间</Button>
                  {!activeRun ? <Button onClick={() => void activateWorkspaceExecution(snapshot.workspace.id)}>启动执行</Button> : null}
                  {activeRun ? <Button onClick={() => void workorchApi.resumeRun(activeRun.id).then(loadBoard).catch((error: unknown) => message.error(errorMessage(error)))}>继续运行</Button> : null}
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

  const resolve = async (item: Decision, resolutionType: string, payload?: Record<string, unknown>) => {
    try {
      await workorchApi.resolveDecision(item.id, resolutionType, payload);
      message.success(`决策已按 ${resolutionType} 处理`);
      actionRef.current?.reload();
    } catch (error) {
      message.error(errorMessage(error));
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
      message.success(`${ticketIds.length} 个决策工单已按 ${resolutionType} 处理`);
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
    { title: '问题', dataIndex: 'question', ellipsis: true },
    { title: '原因', dataIndex: 'reason', ellipsis: true },
    { title: '上下文', dataIndex: 'context', render: (_, item) => <pre className="json-panel">{compactJSON(item.context)}</pre> },
    { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    {
      title: '操作',
      valueType: 'option',
      render: (_, item) => item.state === 'open' ? (
        <Space wrap>
          <Button size="small" type="primary" onClick={() => void resolve(item, 'provide_missing_info', { operator: 'console' })}>补充信息</Button>
          <Button size="small" onClick={() => void resolve(item, 'replan', { operator: 'console' })}>重新规划</Button>
          <Button size="small" danger onClick={() => void resolve(item, 'reject_current_path', { operator: 'console' })}>拒绝路径</Button>
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
          <Button disabled={selectedDecisionKeys.length === 0} onClick={() => void batchResolve('provide_missing_info')}>批量补充信息</Button>
          <Button disabled={selectedDecisionKeys.length === 0} onClick={() => void batchResolve('replan')}>批量重新规划</Button>
          <Button danger disabled={selectedDecisionKeys.length === 0} onClick={() => void batchResolve('reject_current_path')}>批量拒绝路径</Button>
        </Space>
      )}
    >
      <ProTable<Decision>
        actionRef={actionRef}
        rowKey="id"
        search={false}
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
      />
    </PageContainer>
  );
}

function ApprovalQueuePage() {
  const { message } = AntApp.useApp();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [selectedApprovalKeys, setSelectedApprovalKeys] = useState<React.Key[]>([]);
  const actionLabel = (action: 'approve' | 'reject') => action === 'approve' ? '已批准' : '已拒绝';

  const resolve = async (item: Approval, action: 'approve' | 'reject') => {
    try {
      if (action === 'approve') {
        await workorchApi.approveApproval(item.id);
      } else {
        await workorchApi.rejectApproval(item.id);
      }
      message.success(`审批${actionLabel(action)}`);
      actionRef.current?.reload();
    } catch (error) {
      message.error(errorMessage(error));
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
      message.success(`${approvalIds.length} 个审批项${actionLabel(action)}`);
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
    { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    {
      title: '操作',
      valueType: 'option',
      render: (_, item) => item.state === 'pending' ? (
        <Space wrap>
          <Button size="small" type="primary" onClick={() => void resolve(item, 'approve')}>批准</Button>
          <Button size="small" danger onClick={() => void resolve(item, 'reject')}>拒绝</Button>
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
  const [workspaceId, setWorkspaceId] = useState<string | undefined>(selectedWorkspaceId);
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
  const [setupStep, setSetupStep] = useState(0);
  const [createdWorkspace, setCreatedWorkspace] = useState<Workspace | undefined>(undefined);
  const [activationForm] = Form.useForm();
  const [workspaceForm] = Form.useForm();
  const [setupGoalForm] = Form.useForm();
  const [setupDoDForm] = Form.useForm();
  const [goalForm] = Form.useForm();
  const [dodForm] = Form.useForm();

  useEffect(() => {
    void workorchApi.listWorkspaces().then((items) => {
      setWorkspaces(items);
      if (!workspaceId && items[0]) {
        setWorkspaceId(items[0].id);
      }
    });
  }, []);

  useEffect(() => {
    if (selectedWorkspaceId) {
      setWorkspaceId(selectedWorkspaceId);
    }
  }, [selectedWorkspaceId]);

  useEffect(() => {
    if (!workspaceId) {
      return;
    }
    setLoading(true);
    void loadWorkspaceContext(workspaceId).catch((error: unknown) => {
      message.error(errorMessage(error));
    }).finally(() => setLoading(false));
  }, [workspaceId]);

  const readyDoDs = dods.filter((item) => item.state === 'ready' || item.state === 'satisfied');
  const llmProviders = configuration?.llm.providers ?? [];

  async function loadWorkspaceList(nextWorkspaceId?: string) {
    const items = await workorchApi.listWorkspaces();
    setWorkspaces(items);
    if (nextWorkspaceId) {
      setWorkspaceId(nextWorkspaceId);
    } else if (!workspaceId && items[0]) {
      setWorkspaceId(items[0].id);
    }
  }

  async function loadWorkspaceContext(targetWorkspaceId: string) {
    const [nextSnapshot, nextGoals, nextDoDs, nextSouls, nextSkills, nextMCPServers, nextMemoryItems, nextConfiguration] = await Promise.all([
      workorchApi.readWorkspaceSnapshot(targetWorkspaceId),
      workorchApi.listGoals(targetWorkspaceId),
      workorchApi.listDoDs(targetWorkspaceId),
      workorchApi.listSouls(),
      workorchApi.listSkills(),
      workorchApi.listMCPServers(),
      workorchApi.listMemory(targetWorkspaceId),
      workorchApi.readConfiguration()
    ]);
    setSnapshot(nextSnapshot);
    setGoals(nextGoals);
    setDoDs(nextDoDs);
    setSouls(nextSouls);
    setSkills(nextSkills);
    setMCPServers(nextMCPServers);
    setMemoryItems(nextMemoryItems);
    setConfiguration(nextConfiguration);
    setRunSnapshot(undefined);
    setRunObservability(undefined);
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

  const activateWorkspaceExecution = async (request: WorkspaceExecutionActivationRequest) => {
    if (!workspaceId) {
      return;
    }
    setActivationLoading(true);
    try {
      const activation = await workorchApi.activateWorkspaceExecution(workspaceId, request);
      setSnapshot(activation.snapshot);
      setRunSnapshot(undefined);
      setRunObservability(undefined);
      setActivationOpen(false);
      activationForm.resetFields();
      message.success('工作空间执行已启动');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setActivationLoading(false);
    }
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
            查看过程
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

  return (
    <PageContainer
      title="工作空间"
      subTitle="围绕 Workspace 管理目标、验收标准、执行过程、治理阻塞和运行证据"
      extra={
        <Space wrap>
          <Button onClick={() => setWorkspaceSetupOpen(true)}>创建工作空间</Button>
          <Button disabled={!workspaceId} onClick={() => setGoalDefinitionOpen(true)}>定义目标</Button>
          <Button disabled={!workspaceId} onClick={() => setDoDDefinitionOpen(true)}>定义验收标准</Button>
          <Button type="primary" disabled={!workspaceId} onClick={requestWorkspaceActivation}>启动执行</Button>
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
                { key: 'description', label: '描述', children: snapshot.workspace.description || '-' },
                { key: 'workingDirectory', label: '工作目录', children: snapshot.workspace.workingDirectory }
              ]}
            />
          </ProCard>
          <ProCard title="执行过程" className="workorch-section">
            <Steps items={workspaceProcessItems(snapshot, goals, dods)} />
          </ProCard>
          <ProCard title="主要业务模型" className="workorch-section">
            <Space wrap>
              <Tag color="blue">Workspace 1</Tag>
              <Tag>Goal {goals.length}</Tag>
              <Tag>DoD {dods.length}</Tag>
              <Tag color="geekblue">Run {snapshot.runs?.length ?? 0}</Tag>
              <Tag>Decision {snapshot.decisions?.length ?? 0}</Tag>
              <Tag>Approval {snapshot.approvals?.length ?? 0}</Tag>
              <Tag>Soul {souls.length}</Tag>
              <Tag>Skill {skills.length}</Tag>
              <Tag>MCP Server {mcpServers.length}</Tag>
              <Tag>Memory {memoryItems.length}</Tag>
              <Tag color="cyan">Step {runSnapshot?.steps?.length ?? 0}</Tag>
              <Tag color="purple">ToolCall {runSnapshot?.toolCalls?.length ?? 0}</Tag>
              <Tag>Patch {runSnapshot?.patches?.length ?? 0}</Tag>
              <Tag>Validation {runSnapshot?.validationResults?.length ?? 0}</Tag>
              <Tag>Checkpoint {runSnapshot?.runtimeCheckpoints?.length ?? 0}</Tag>
            </Space>
          </ProCard>
          <ProCard title="运行摘要" className="workorch-section">
            <Descriptions
              column={3}
              items={[
                { key: 'runs', label: 'Run 总数', children: runSummary.total },
                { key: 'created', label: '已创建', children: runSummary.created },
                { key: 'active', label: '运行中', children: runSummary.active },
                { key: 'blocked', label: '阻塞中', children: runSummary.blocked },
                { key: 'validating', label: '验收中', children: runSummary.validating },
                { key: 'completed', label: '已完成', children: runSummary.completed },
                { key: 'failed', label: '失败/终止', children: runSummary.failed },
                { key: 'decisions', label: '待处理决策', children: pendingCount(snapshot.decisions) },
                { key: 'approvals', label: '待处理审批', children: pendingCount(snapshot.approvals) },
                { key: 'latestRun', label: '最近运行', children: runSummary.latest ? `${runSummary.latest.id} / ${stateLabel(runSummary.latest.state)}` : '-' }
              ]}
            />
          </ProCard>
          <ProCard gutter={16} wrap className="workorch-section">
            <ProCard title="目标定义（Goal）" colSpan="50%">
              {goals.length ? goals.map((item) => <Typography.Paragraph key={item.id}>{item.text}</Typography.Paragraph>) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无目标" />}
            </ProCard>
            <ProCard title="验收标准（DoD）" colSpan="50%">
              {dods.length ? dods.flatMap((item) => item.items).map((item) => <Tag key={item}>{item}</Tag>) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无验收标准" />}
            </ProCard>
          </ProCard>
          <ProCard title="运行记录（Run）" className="workorch-section">
            <ProTable<Run> rowKey="id" search={false} pagination={false} columns={runColumns} dataSource={snapshot.runs ?? []} />
          </ProCard>
          <ProCard gutter={16} wrap className="workorch-section">
            <ProCard title="决策工单（Decision）" colSpan="50%">
              <ProTable<Decision> rowKey="id" search={false} pagination={false} columns={[
                { title: '工单', dataIndex: 'id', copyable: true },
                { title: '状态', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
                { title: '解决方式', dataIndex: 'resolutionType' }
              ]} dataSource={snapshot.decisions ?? []} />
            </ProCard>
            <ProCard title="审批工单（Approval）" colSpan="50%">
              <ProTable<Approval> rowKey="id" search={false} pagination={false} columns={[
                { title: '审批', dataIndex: 'id', copyable: true },
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
                    { key: 'events', label: '事件', children: runObservability?.evidence.eventTotal ?? 0 },
                    { key: 'context', label: '执行上下文', children: <pre className="json-panel">{compactJSON(runSnapshot.run.executionContext)}</pre> }
                  ]}
                />
                {runObservability?.latestCheckpoint ? (
                  <Tag color="blue">最新检查点：{runObservability.latestCheckpoint.checkpointType} / {stateLabel(runObservability.latestCheckpoint.runStateSnapshot)}</Tag>
                ) : null}
                <ProTable<RuntimeTimelineItem>
                  rowKey={(item, index) => `${item.source}-${item.id || item.topic || index}-${index}`}
                  search={false}
                  pagination={{ pageSize: 5 }}
                  size="small"
                  columns={[
                    { title: '来源', dataIndex: 'source', width: 110 },
                    { title: '事件', render: (_, item) => timelineLabel(item), ellipsis: true },
                    { title: 'Run 状态', dataIndex: 'runState', width: 130, render: (_, item) => stateLabel(item.runState) },
                    { title: '步骤', dataIndex: 'stepId', ellipsis: true },
                    { title: '摘要', dataIndex: 'summary', ellipsis: true },
                    { title: '流式事件', render: (_, item) => outputText(item.output ?? item.data, 'streamEvents') !== '-' ? outputText(item.output ?? item.data, 'streamEvents') : outputText(item.data, 'delta'), ellipsis: true },
                    { title: '发生时间', dataIndex: 'occurredAt', width: 210 }
                  ]}
                  dataSource={runObservability?.timeline ?? []}
                />
                <ProTable<StepView>
                  rowKey="id"
                  search={false}
                  pagination={{ pageSize: 5 }}
                  size="small"
                  columns={[
                    { title: '步骤', dataIndex: 'id', copyable: true, ellipsis: true },
                    { title: '类型', dataIndex: 'kind', width: 180 },
                    { title: '状态', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                    { title: '操作', render: (_, item) => outputText(item.output, 'operation'), ellipsis: true },
                    { title: '服务', render: (_, item) => outputText(item.output, 'serverId'), ellipsis: true },
                    { title: '目标', render: (_, item) => runtimeTarget(item.output), ellipsis: true },
                    { title: '阶段', render: (_, item) => outputText(item.output, 'stage'), ellipsis: true },
                    { title: '下一类型', render: (_, item) => outputText(item.output, 'nextKind'), ellipsis: true },
                    { title: '流式事件', render: (_, item) => outputText(item.output, 'streamEvents'), ellipsis: true },
                    { title: 'Memory 引用', render: (_, item) => outputText(item.output, 'memoryRefs'), ellipsis: true },
                    { title: 'DoD 评估', render: (_, item) => outputText(item.output, 'dodAssessment'), ellipsis: true }
                  ]}
                  dataSource={runSnapshot.steps ?? []}
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
      <Modal title="定义目标" open={goalDefinitionOpen} confirmLoading={businessSubmitting} onCancel={() => setGoalDefinitionOpen(false)} onOk={() => goalForm.submit()}>
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
              await workorchApi.defineGoal(workspaceId, values.text);
              await loadWorkspaceContext(workspaceId);
              setGoalDefinitionOpen(false);
              goalForm.resetFields();
              message.success('目标已定义');
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
      <Modal title="定义验收标准" open={dodDefinitionOpen} confirmLoading={businessSubmitting} onCancel={() => setDoDDefinitionOpen(false)} onOk={() => dodForm.submit()}>
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
              await workorchApi.defineDoD(workspaceId, items);
              await loadWorkspaceContext(workspaceId);
              setDoDDefinitionOpen(false);
              dodForm.resetFields();
              message.success('验收标准已定义');
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
        okText="启动执行"
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
                label: `${item.name || item.id}${item.version ? `@${item.version}` : ''}`
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
                pagination={false}
                size="small"
                columns={[{ title: '允许的工具', dataIndex: 'name' }]}
                dataSource={tools}
              />
              <ProTable<MCPResource>
                rowKey={(item) => `${item.serverId}-${item.uri}`}
                search={false}
                pagination={false}
                size="small"
                columns={[{ title: '允许的资源', dataIndex: 'uri' }]}
                dataSource={resources}
              />
              <ProTable<MCPPrompt>
                rowKey={(item) => `${item.serverId}-${item.name}`}
                search={false}
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

  const addProvider = () => {
    const draft = nextProviderDraft(providerDrafts);
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

  const updateSelectedProvider = (_changedValues: Partial<UpdateLLMProviderConfiguration>, values: UpdateLLMProviderConfiguration) => {
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
            extra={<Button onClick={addProvider}>新增服务商</Button>}
          >
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
                    options={[
                      { value: 'compatible', label: 'compatible' },
                      { value: 'openai', label: 'openai' }
                    ]}
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
