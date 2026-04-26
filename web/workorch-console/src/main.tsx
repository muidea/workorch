import React, { useEffect, useRef, useState } from 'react';
import ReactDOM from 'react-dom/client';
import {
  CheckCircleOutlined,
  DashboardOutlined,
  ExceptionOutlined,
  ProfileOutlined,
  SettingOutlined
} from '@ant-design/icons';
import { PageContainer, ProCard, ProLayout, ProTable } from '@ant-design/pro-components';
import { App as AntApp, Button, ConfigProvider, Descriptions, Empty, Form, Input, Modal, Select, Space, Steps, Tag, Typography, theme } from 'antd';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import type { Approval, Decision, DoD, Goal, MCPServer, MemoryItem, Run, RunObservability, RunSnapshot, RuntimeTimelineItem, ServiceConfiguration, Skill, Soul, StepView, UpdateLLMConfigurationRequest, UpdateLLMProviderConfiguration, Workspace, WorkspaceExecutionActivationRequest, WorkspaceSnapshot } from './types';
import { workorchApi } from './api';
import './styles.css';

type ViewKey = 'board' | 'decisionQueue' | 'workspaceDetail' | 'configuration' | 'service';

const routes = [
  { path: '/board', name: 'Workspace Overview', icon: <DashboardOutlined /> },
  { path: '/decisionQueue', name: 'Decision Queue', icon: <ExceptionOutlined /> },
  { path: '/workspaceDetail', name: 'Workspace Console', icon: <ProfileOutlined /> },
  { path: '/configuration', name: 'Configuration', icon: <SettingOutlined /> },
  { path: '/service', name: 'Service Health', icon: <CheckCircleOutlined /> }
];

function stateTag(state?: string) {
  const value = state || 'unknown';
  const color = value.includes('failed') || value.includes('reject') ? 'red' : value.includes('block') || value.includes('pending') ? 'gold' : 'green';
  return <Tag color={color}>{value}</Tag>;
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : 'Operation failed';
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

function runtimeSummaryTags(runs?: Run[]) {
  const summary = runStateSummary(runs);
  return (
    <Space wrap>
      <Tag>total {summary.total}</Tag>
      <Tag color="blue">active {summary.active}</Tag>
      <Tag color="gold">blocked {summary.blocked}</Tag>
      <Tag color="cyan">validating {summary.validating}</Tag>
      <Tag color="green">completed {summary.completed}</Tag>
      <Tag color="red">failed {summary.failed}</Tag>
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

function llmConfigurationFormValues(configuration: ServiceConfiguration): UpdateLLMConfigurationRequest {
  return {
    defaultProviderId: configuration.llm.defaultProviderId,
    providers: (configuration.llm.providers ?? []).map((item) => ({
      id: item.id,
      provider: item.provider,
      baseUrl: item.baseUrl,
      apiKey: '',
      model: item.model,
      timeout: item.timeout,
      disabled: item.disabled
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
    disabled: false
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
      message.success('Workspace execution activated');
      await loadBoard();
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  return (
    <PageContainer
      title="Workspace Overview"
      subTitle="Delivery and runtime progress across workspaces"
      extra={<Button onClick={() => void loadBoard()} loading={loading}>Refresh</Button>}
    >
      {snapshots.length === 0 && !loading ? (
        <Empty description="No workspace yet" />
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
                    { key: 'workingDirectory', label: 'Working Directory', children: snapshot.workspace.workingDirectory },
                    { key: 'activeRun', label: 'Active Run', children: activeRun ? `${activeRun.id} / ${activeRun.state}` : '-' },
                    { key: 'runtimeSummary', label: 'Runtime Summary', children: runtimeSummaryTags(snapshot.runs) },
                    { key: 'latestRun', label: 'Latest Run', children: runSummary.latest ? `${runSummary.latest.id} / ${runSummary.latest.state}` : '-' },
                    { key: 'decisions', label: 'Open Decisions', children: pendingDecisions },
                    { key: 'approvals', label: 'Pending Approvals', children: pendingApprovals }
                  ]}
                />
                <Space className="workorch-section" wrap>
                  <Button type="primary" onClick={() => onOpenWorkspace(snapshot.workspace.id)}>Open console</Button>
                  {!activeRun ? <Button onClick={() => void activateWorkspaceExecution(snapshot.workspace.id)}>Start execution</Button> : null}
                  {activeRun ? <Button onClick={() => void workorchApi.resumeRun(activeRun.id).then(loadBoard).catch((error: unknown) => message.error(errorMessage(error)))}>Resume active run</Button> : null}
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

  const resolve = async (item: Decision, resolutionType: string, payload?: Record<string, unknown>) => {
    try {
      await workorchApi.resolveDecision(item.id, resolutionType, payload);
      message.success(`Decision resolved as ${resolutionType}`);
      actionRef.current?.reload();
    } catch (error) {
      message.error(errorMessage(error));
    }
  };

  const columns: ProColumns<Decision>[] = [
    { title: 'Ticket ID', dataIndex: 'id', copyable: true },
    { title: 'Workspace', dataIndex: 'workspaceId', copyable: true },
    { title: 'Run', dataIndex: 'runId', copyable: true },
    { title: 'Kind', dataIndex: 'decisionKind' },
    { title: 'Question', dataIndex: 'question', ellipsis: true },
    { title: 'Reason', dataIndex: 'reason', ellipsis: true },
    { title: 'Context', dataIndex: 'context', render: (_, item) => <pre className="json-panel">{compactJSON(item.context)}</pre> },
    { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    {
      title: 'Actions',
      valueType: 'option',
      render: (_, item) => item.state === 'open' ? (
        <Space wrap>
          <Button size="small" type="primary" onClick={() => void resolve(item, 'provide_missing_info', { operator: 'console' })}>Provide info</Button>
          <Button size="small" onClick={() => void resolve(item, 'replan', { operator: 'console' })}>Replan</Button>
          <Button size="small" danger onClick={() => void resolve(item, 'reject_current_path', { operator: 'console' })}>Reject path</Button>
        </Space>
      ) : stateTag(item.resolutionType)
    }
  ];

  return (
    <PageContainer title="Decision Queue" subTitle="Process pending decision tickets with formal resolution types">
      <ProTable<Decision>
        actionRef={actionRef}
        rowKey="id"
        search={false}
        columns={columns}
        request={async () => {
          const items = await workorchApi.listDecisions();
          return tableResult(items.sort((left, right) => Number(left.state !== 'open') - Number(right.state !== 'open')));
        }}
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
      message.success('Workspace execution activated');
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
      memoryIds: []
    });
    setActivationOpen(true);
  };

  const runColumns: ProColumns<Run>[] = [
    { title: 'Run ID', dataIndex: 'id', copyable: true },
    { title: 'Goal', dataIndex: 'goalId', copyable: true },
    { title: 'DoD', dataIndex: 'doDId', copyable: true },
    { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    { title: 'Blocking Ticket', dataIndex: 'blockingTicketId', copyable: true },
    {
      title: 'Actions',
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
            Observability
          </Button>
          <Button
            size="small"
            onClick={() => void workorchApi.resumeRun(item.id)
              .then(refreshWorkspace)
              .catch((error: unknown) => message.error(errorMessage(error)))}
          >
            Resume
          </Button>
        </Space>
      )
    }
  ];
  const runSummary = runStateSummary(snapshot?.runs);

  return (
    <PageContainer
      title="Workspace Console"
      subTitle="Create workspace, define Goal and DoD, start execution, and observe runtime progress"
      extra={
        <Space wrap>
          <Button onClick={() => setWorkspaceSetupOpen(true)}>Create workspace</Button>
          <Button disabled={!workspaceId} onClick={() => setGoalDefinitionOpen(true)}>Define goal</Button>
          <Button disabled={!workspaceId} onClick={() => setDoDDefinitionOpen(true)}>Define DoD</Button>
          <Button type="primary" disabled={!workspaceId} onClick={requestWorkspaceActivation}>Start execution</Button>
          <Select
            style={{ width: 360 }}
            placeholder="Select workspace"
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
                { key: 'id', label: 'Workspace ID', children: snapshot.workspace.id },
                { key: 'activeRun', label: 'Active Run', children: snapshot.workspace.activeRunId || '-' },
                { key: 'description', label: 'Description', children: snapshot.workspace.description || '-' },
                { key: 'workingDirectory', label: 'Working Directory', children: snapshot.workspace.workingDirectory }
              ]}
            />
          </ProCard>
          <ProCard title="Runtime Summary" className="workorch-section">
            <Descriptions
              column={3}
              items={[
                { key: 'runs', label: 'Runs', children: runSummary.total },
                { key: 'created', label: 'Created', children: runSummary.created },
                { key: 'active', label: 'Active', children: runSummary.active },
                { key: 'blocked', label: 'Blocked', children: runSummary.blocked },
                { key: 'validating', label: 'Validating', children: runSummary.validating },
                { key: 'completed', label: 'Completed', children: runSummary.completed },
                { key: 'failed', label: 'Failed', children: runSummary.failed },
                { key: 'decisions', label: 'Open Decisions', children: pendingCount(snapshot.decisions) },
                { key: 'approvals', label: 'Pending Approvals', children: pendingCount(snapshot.approvals) },
                { key: 'latestRun', label: 'Latest Run', children: runSummary.latest ? `${runSummary.latest.id} / ${runSummary.latest.state}` : '-' }
              ]}
            />
          </ProCard>
          <ProCard gutter={16} wrap className="workorch-section">
            <ProCard title="Goal" colSpan="50%">
              {goals.length ? goals.map((item) => <Typography.Paragraph key={item.id}>{item.text}</Typography.Paragraph>) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No goal" />}
            </ProCard>
            <ProCard title="Definition of Done" colSpan="50%">
              {dods.length ? dods.flatMap((item) => item.items).map((item) => <Tag key={item}>{item}</Tag>) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No DoD" />}
            </ProCard>
          </ProCard>
          <ProCard title="Runs" className="workorch-section">
            <ProTable<Run> rowKey="id" search={false} pagination={false} columns={runColumns} dataSource={snapshot.runs ?? []} />
          </ProCard>
          <ProCard gutter={16} wrap className="workorch-section">
            <ProCard title="Decisions" colSpan="50%">
              <ProTable<Decision> rowKey="id" search={false} pagination={false} columns={[
                { title: 'Ticket', dataIndex: 'id', copyable: true },
                { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
                { title: 'Resolution', dataIndex: 'resolutionType' }
              ]} dataSource={snapshot.decisions ?? []} />
            </ProCard>
            <ProCard title="Approvals" colSpan="50%">
              <ProTable<Approval> rowKey="id" search={false} pagination={false} columns={[
                { title: 'Approval', dataIndex: 'id', copyable: true },
                { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
                { title: 'Reason', dataIndex: 'reason' }
              ]} dataSource={snapshot.approvals ?? []} />
            </ProCard>
          </ProCard>
          <ProCard title="Selected Run Observability" className="workorch-section" extra={runSnapshot ? <Tag>{evidenceCount(runSnapshot)} evidence records</Tag> : null}>
            {runSnapshot ? (
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <Descriptions
                  column={3}
                  items={[
                    { key: 'run', label: 'Run', children: `${runSnapshot.run.id} / ${runSnapshot.run.state}` },
                    { key: 'currentStep', label: 'Current Step', children: runObservability?.progress.currentStepId || runSnapshot.steps?.at(-1)?.id || '-' },
                    { key: 'terminal', label: 'Terminal', children: runObservability ? String(runObservability.progress.terminal) : '-' },
                    { key: 'steps', label: 'Steps', children: `${runObservability?.progress.stepCompleted ?? 0}/${runObservability?.progress.stepTotal ?? runSnapshot.steps?.length ?? 0} completed` },
                    { key: 'toolCalls', label: 'Tool Calls', children: `${runObservability?.evidence.toolCallCompleted ?? 0}/${runObservability?.evidence.toolCallTotal ?? runSnapshot.toolCalls?.length ?? 0} completed` },
                    { key: 'patches', label: 'Patches', children: `${runObservability?.evidence.patchApplied ?? 0}/${runObservability?.evidence.patchTotal ?? runSnapshot.patches?.length ?? 0} applied` },
                    { key: 'validations', label: 'Validation Results', children: `${runObservability?.evidence.validationPassed ?? 0}/${runObservability?.evidence.validationTotal ?? runSnapshot.validationResults?.length ?? 0} passed` },
                { key: 'checkpoints', label: 'Runtime Checkpoints', children: runObservability?.evidence.runtimeCheckpointTotal ?? runSnapshot.runtimeCheckpoints?.length ?? 0 },
                    { key: 'events', label: 'Events', children: runObservability?.evidence.eventTotal ?? 0 },
                    { key: 'context', label: 'Execution Context', children: <pre className="json-panel">{compactJSON(runSnapshot.run.executionContext)}</pre> }
                  ]}
                />
                {runObservability?.latestCheckpoint ? (
                  <Tag color="blue">Latest checkpoint: {runObservability.latestCheckpoint.checkpointType} / {runObservability.latestCheckpoint.runStateSnapshot}</Tag>
                ) : null}
                <ProTable<RuntimeTimelineItem>
                  rowKey={(item, index) => `${item.source}-${item.id || item.topic || index}-${index}`}
                  search={false}
                  pagination={{ pageSize: 5 }}
                  size="small"
                  columns={[
                    { title: 'Source', dataIndex: 'source', width: 110 },
                    { title: 'Event', render: (_, item) => timelineLabel(item), ellipsis: true },
                    { title: 'Run State', dataIndex: 'runState', width: 130 },
                    { title: 'Step', dataIndex: 'stepId', ellipsis: true },
                    { title: 'Occurred At', dataIndex: 'occurredAt', width: 210 }
                  ]}
                  dataSource={runObservability?.timeline ?? []}
                />
                <ProTable<StepView>
                  rowKey="id"
                  search={false}
                  pagination={{ pageSize: 5 }}
                  size="small"
                  columns={[
                    { title: 'Step', dataIndex: 'id', copyable: true, ellipsis: true },
                    { title: 'Kind', dataIndex: 'kind', width: 180 },
                    { title: 'State', dataIndex: 'state', width: 120, render: (_, item) => stateTag(item.state) },
                    { title: 'Stage', render: (_, item) => outputText(item.output, 'stage'), ellipsis: true },
                    { title: 'Next Kind', render: (_, item) => outputText(item.output, 'nextKind'), ellipsis: true },
                    { title: 'Memory Refs', render: (_, item) => outputText(item.output, 'memoryRefs'), ellipsis: true },
                    { title: 'DoD Assessment', render: (_, item) => outputText(item.output, 'dodAssessment'), ellipsis: true }
                  ]}
                  dataSource={runSnapshot.steps ?? []}
                />
              </Space>
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Select a run observability view" />
            )}
          </ProCard>
        </>
      ) : (
        <Empty description={loading ? 'Loading workspace detail' : 'No workspace selected'} />
      )}
      <Modal
        title="Create workspace"
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
        okText={setupStep === 0 ? 'Create workspace' : setupStep === 1 ? 'Define goal' : 'Define DoD'}
      >
        <Steps
          current={setupStep}
          className="workorch-section"
          items={[
            { title: 'Workspace' },
            { title: 'Goal' },
            { title: 'DoD' }
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
              message.success('Workspace created, continue defining Goal');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item name="title" label="Title" rules={[{ required: true }]}>
            <Input placeholder="Local workorch workspace" />
          </Form.Item>
          <Form.Item name="description" label="Description">
            <Input.TextArea rows={3} placeholder="Describe the workspace purpose, target scope, or current delivery expectation" />
          </Form.Item>
          <Form.Item name="workingDirectory" label="Working Directory">
            <Input placeholder="Leave empty to use service working path / workspaces / generated workspace ID" />
          </Form.Item>
        </Form>
        <Form
          form={setupGoalForm}
          layout="vertical"
          style={{ display: setupStep === 1 ? 'block' : 'none' }}
          onFinish={async (values: { text: string }) => {
            if (!createdWorkspace) {
              message.error('Workspace is not created');
              return;
            }
            setBusinessSubmitting(true);
            try {
              await workorchApi.defineGoal(createdWorkspace.id, values.text);
              await loadWorkspaceContext(createdWorkspace.id);
              setSetupStep(2);
              message.success('Goal defined, continue defining DoD');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item label="Workspace">
            <Input value={createdWorkspace?.id || ''} disabled />
          </Form.Item>
          <Form.Item name="text" label="Goal" rules={[{ required: true }]}>
            <Input.TextArea rows={4} placeholder="Describe the target outcome for this workspace" />
          </Form.Item>
        </Form>
        <Form
          form={setupDoDForm}
          layout="vertical"
          style={{ display: setupStep === 2 ? 'block' : 'none' }}
          onFinish={async (values: { items: string }) => {
            if (!createdWorkspace) {
              message.error('Workspace is not created');
              return;
            }
            setBusinessSubmitting(true);
            try {
              const items = values.items.split('\n').map((item) => item.trim()).filter(Boolean);
              await workorchApi.defineDoD(createdWorkspace.id, items);
              await loadWorkspaceContext(createdWorkspace.id);
              closeWorkspaceSetup();
              message.success('Workspace Goal and DoD are ready');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item label="Workspace">
            <Input value={createdWorkspace?.id || ''} disabled />
          </Form.Item>
          <Form.Item name="items" label="DoD Items" rules={[{ required: true }]}>
            <Input.TextArea rows={5} placeholder={'One acceptance criterion per line\nExample: service deployed locally'} />
          </Form.Item>
        </Form>
      </Modal>
      <Modal title="Define goal" open={goalDefinitionOpen} confirmLoading={businessSubmitting} onCancel={() => setGoalDefinitionOpen(false)} onOk={() => goalForm.submit()}>
        <Form
          form={goalForm}
          layout="vertical"
          onFinish={async (values: { text: string }) => {
            if (!workspaceId) {
              message.error('Select a workspace first');
              return;
            }
            setBusinessSubmitting(true);
            try {
              await workorchApi.defineGoal(workspaceId, values.text);
              await loadWorkspaceContext(workspaceId);
              setGoalDefinitionOpen(false);
              goalForm.resetFields();
              message.success('Goal defined');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item name="text" label="Goal" rules={[{ required: true }]}>
            <Input.TextArea rows={4} placeholder="Describe the target outcome for this workspace" />
          </Form.Item>
        </Form>
      </Modal>
      <Modal title="Define DoD" open={dodDefinitionOpen} confirmLoading={businessSubmitting} onCancel={() => setDoDDefinitionOpen(false)} onOk={() => dodForm.submit()}>
        <Form
          form={dodForm}
          layout="vertical"
          onFinish={async (values: { items: string }) => {
            if (!workspaceId) {
              message.error('Select a workspace first');
              return;
            }
            setBusinessSubmitting(true);
            try {
              const items = values.items.split('\n').map((item) => item.trim()).filter(Boolean);
              await workorchApi.defineDoD(workspaceId, items);
              await loadWorkspaceContext(workspaceId);
              setDoDDefinitionOpen(false);
              dodForm.resetFields();
              message.success('DoD defined');
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setBusinessSubmitting(false);
            }
          }}
        >
          <Form.Item name="items" label="DoD Items" rules={[{ required: true }]}>
            <Input.TextArea rows={5} placeholder={'One acceptance criterion per line\nExample: service deployed locally'} />
          </Form.Item>
        </Form>
      </Modal>
      <Modal
        title="Start workspace execution"
        open={activationOpen}
        confirmLoading={activationLoading}
        onCancel={() => setActivationOpen(false)}
        onOk={() => activationForm.submit()}
        okText="Start execution"
      >
        <Form
          form={activationForm}
          layout="vertical"
          onFinish={(values: WorkspaceExecutionActivationRequest) => void activateWorkspaceExecution(values)}
        >
          <Form.Item name="goalId" label="Goal" rules={[{ required: true }]}>
            <Select options={goals.map((item) => ({ value: item.id, label: `${item.text} (${item.id})` }))} />
          </Form.Item>
          <Form.Item name="doDId" label="Definition of Done" rules={[{ required: true }]}>
            <Select options={readyDoDs.map((item) => ({ value: item.id, label: `${item.items.join(' / ')} (${item.id})` }))} />
          </Form.Item>
          <Form.Item name="llmProviderId" label="LLM Provider ID">
            <Select
              allowClear
              placeholder="Use configured default provider"
              options={llmProviders.map((item) => ({
                value: item.id,
                label: `${item.default ? '[default] ' : ''}${item.id} / ${item.model || item.provider || 'not configured'}${item.ready ? '' : ` / ${item.reason || 'unavailable'}`}`
              }))}
            />
          </Form.Item>
          <Form.Item name="soulId" label="Soul">
            <Select allowClear options={souls.map((item) => ({ value: item.id, label: `${item.name || item.id} (${item.id})` }))} />
          </Form.Item>
          <Form.Item name="skillIds" label="Skills">
            <Select
              mode="multiple"
              allowClear
              options={skills.map((item) => ({
                value: item.id,
                label: `${item.name || item.id}${item.version ? `@${item.version}` : ''}`
              }))}
            />
          </Form.Item>
          <Form.Item name="mcpServerIds" label="MCP Servers">
            <Select
              mode="multiple"
              allowClear
              options={mcpServers.map((item) => ({
                value: item.id,
                label: `${item.name || item.id} / ${item.transport}${item.enabled ? '' : ' / disabled'}`
              }))}
            />
          </Form.Item>
          <Form.Item name="memoryIds" label="Memory">
            <Select
              mode="multiple"
              allowClear
              placeholder="Leave empty to let workorch recall workspace memory automatically"
              options={memoryItems.map((item) => ({
                value: item.id,
                label: `${item.memoryType}: ${item.summary}`
              }))}
            />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
}

function ConfigurationPage() {
  const { message } = AntApp.useApp();
  const [providerEditorForm] = Form.useForm<UpdateLLMProviderConfiguration>();
  const [configuration, setConfiguration] = useState<ServiceConfiguration | undefined>(undefined);
  const [defaultProviderId, setDefaultProviderId] = useState<string | undefined>(undefined);
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
        providers: providerDrafts.map((item) => ({
          id: item.id?.trim(),
          provider: item.provider?.trim(),
          baseUrl: item.baseUrl?.trim(),
          apiKey: item.apiKey?.trim(),
          model: item.model?.trim(),
          timeout: item.timeout?.trim(),
          disabled: item.disabled === true
        }))
      });
      const nextFormValues = llmConfigurationFormValues(nextConfiguration);
      setConfiguration(nextConfiguration);
      setDefaultProviderId(nextFormValues.defaultProviderId);
      setProviderDrafts(nextFormValues.providers);
      setSelectedProviderIndex(0);
      providerEditorForm.setFieldsValue(nextFormValues.providers[0] ?? nextProviderDraft([]));
      message.success('LLM configuration updated');
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
      return nextItems;
    });
  };

  const statusByProviderID = new Map((configuration?.llm.providers ?? []).map((item) => [item.id, item]));
  const selectedProvider = providerDrafts[selectedProviderIndex];
  const selectedProviderStatus = selectedProvider ? statusByProviderID.get(selectedProvider.id) : undefined;
  return (
    <PageContainer
      title="Configuration"
      extra={<Button onClick={() => void loadConfiguration()} loading={loading}>Refresh</Button>}
    >
      <ProCard gutter={16} wrap loading={loading}>
        <ProCard title="Server" colSpan="50%">
          <Descriptions
            column={1}
            items={[
              { key: 'addr', label: 'Address', children: configuration?.server.addr || '-' },
              { key: 'readHeaderTimeout', label: 'Read Header Timeout', children: configuration?.server.readHeaderTimeout || '-' },
              { key: 'readTimeout', label: 'Read Timeout', children: configuration?.server.readTimeout || '-' },
              { key: 'writeTimeout', label: 'Write Timeout', children: configuration?.server.writeTimeout || '-' },
              { key: 'idleTimeout', label: 'Idle Timeout', children: configuration?.server.idleTimeout || '-' },
              { key: 'shutdownTimeout', label: 'Shutdown Timeout', children: configuration?.server.shutdownTimeout || '-' },
              { key: 'maxConcurrentRuns', label: 'Max Concurrent Runs', children: configuration?.server.maxConcurrentRuns ?? '-' }
            ]}
          />
        </ProCard>
        <ProCard title="Repository" colSpan="50%">
          <Descriptions
            column={1}
            items={[
              { key: 'backend', label: 'Backend', children: configuration?.repository.backend || '-' },
              { key: 'dataDir', label: 'Data Directory', children: configuration?.repository.dataDir || '-' }
            ]}
          />
        </ProCard>
      </ProCard>
      <ProCard
        title="LLM Providers"
        className="workorch-section"
        extra={<Button type="primary" onClick={() => void providerEditorForm.validateFields().then(() => saveLLMConfiguration())} loading={saving}>Save LLM Providers</Button>}
      >
        <Typography.Paragraph type="secondary">
          Provider API keys are intentionally redacted. Leave API Key empty to keep the existing secret. Saved provider changes are reloaded by workorchd and used by the next LLM runtime step.
        </Typography.Paragraph>
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <ProCard
            title="Provider List"
            extra={<Button onClick={addProvider}>Add Provider</Button>}
          >
            <Form.Item label="Default Provider ID">
              <Select
                value={defaultProviderId}
                onChange={setDefaultProviderId}
                placeholder="Select the default LLM provider"
                options={providerDrafts
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
                      <Typography.Text strong>{item.id || 'new provider'}</Typography.Text>
                      <Typography.Text type="secondary">{item.model || item.provider || 'model not configured'}</Typography.Text>
                    </span>
                    <span className="llm-provider-list-tags">
                      {item.id === defaultProviderId ? <Tag color="blue">default</Tag> : null}
                      {status?.apiKeyConfigured ? <Tag color="green">key</Tag> : <Tag color="red">key</Tag>}
                      {status?.ready ? <Tag color="green">ready</Tag> : <Tag color="red">{status?.reason || 'unsaved'}</Tag>}
                    </span>
                  </button>
                );
              })}
            </Space>
          </ProCard>
          <ProCard
            title={selectedProvider ? `Edit Provider: ${selectedProvider.id || 'new provider'}` : 'Edit Provider'}
            extra={<Button danger onClick={removeSelectedProvider} disabled={providerDrafts.length <= 1}>Remove</Button>}
          >
            {selectedProvider ? (
              <Form
                form={providerEditorForm}
                layout="vertical"
                onValuesChange={updateSelectedProvider}
              >
                <Form.Item name="id" label="Provider ID" rules={[{ required: true }]}>
                  <Input placeholder="local" />
                </Form.Item>
                <Form.Item name="provider" label="Provider Type" rules={[{ required: true }]}>
                  <Select
                    options={[
                      { value: 'compatible', label: 'compatible' },
                      { value: 'openai', label: 'openai' }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="model" label="Model" rules={[{ required: true }]}>
                  <Input placeholder="gpt-4.1-mini" />
                </Form.Item>
                <Form.Item name="baseUrl" label="Base URL">
                  <Input placeholder="https://api.openai.com/v1 or local compatible endpoint" />
                </Form.Item>
                <Form.Item name="timeout" label="Timeout">
                  <Input placeholder="60s" />
                </Form.Item>
                <Form.Item name="disabled" label="Disabled">
                  <Select
                    options={[
                      { value: false, label: 'false' },
                      { value: true, label: 'true' }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="apiKey" label="API Key">
                  <Input.Password placeholder="Leave empty to keep existing API key" autoComplete="new-password" />
                </Form.Item>
                <Descriptions
                  column={1}
                  items={[
                    { key: 'apiKey', label: 'Saved API Key', children: selectedProviderStatus?.apiKeyConfigured ? <Tag color="green">configured</Tag> : <Tag color="red">missing</Tag> },
                    { key: 'status', label: 'Saved Status', children: selectedProviderStatus?.ready ? <Tag color="green">ready</Tag> : <Tag color="red">{selectedProviderStatus?.reason || 'unsaved'}</Tag> }
                  ]}
                />
              </Form>
            ) : (
              <Empty description="Select or add an LLM provider" />
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
    <PageContainer title="Service Health">
      <ProCard gutter={16} wrap>
        <ProCard title="Health" colSpan="50%">
          <Descriptions items={Object.entries(health).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
        </ProCard>
        <ProCard title="Readiness" colSpan="50%">
          <Descriptions items={Object.entries(ready).map(([key, value]) => ({ key, label: key, children: value }))} column={1} />
        </ProCard>
      </ProCard>
      <ProCard title="Metrics" className="workorch-section">
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
      case 'decisionQueue':
        return <DecisionQueuePage />;
      case 'workspaceDetail':
        return <WorkspaceDetailPage selectedWorkspaceId={selectedWorkspaceId} />;
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
          title="workorch"
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
