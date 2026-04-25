import React, { useEffect, useRef, useState } from 'react';
import ReactDOM from 'react-dom/client';
import {
  ApartmentOutlined,
  CheckCircleOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  ExceptionOutlined,
  PlayCircleOutlined,
  SafetyCertificateOutlined
} from '@ant-design/icons';
import { PageContainer, ProCard, ProLayout, ProTable } from '@ant-design/pro-components';
import { App as AntApp, Button, ConfigProvider, Descriptions, Form, Input, Modal, Space, Statistic, Steps, Tag, theme } from 'antd';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import type { Approval, Decision, DoD, Goal, Run, SliceDescriptor, SliceStatus, Workspace } from './types';
import { workorchApi } from './api';
import './styles.css';

type ViewKey = 'overview' | 'workspaces' | 'runs' | 'decisions' | 'approvals' | 'slices' | 'service';

const routes = [
  { path: '/overview', name: 'Overview', icon: <DashboardOutlined /> },
  { path: '/workspaces', name: 'Workspaces', icon: <ApartmentOutlined /> },
  { path: '/runs', name: 'Runs', icon: <PlayCircleOutlined /> },
  { path: '/decisions', name: 'Decisions', icon: <ExceptionOutlined /> },
  { path: '/approvals', name: 'Approvals', icon: <SafetyCertificateOutlined /> },
  { path: '/slices', name: 'Slices', icon: <DeploymentUnitOutlined /> },
  { path: '/service', name: 'Service', icon: <CheckCircleOutlined /> }
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

function OverviewPage() {
  const [health, setHealth] = useState('loading');
  const [ready, setReady] = useState('loading');
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [decisions, setDecisions] = useState<Decision[]>([]);
  const [approvals, setApprovals] = useState<Approval[]>([]);

  useEffect(() => {
    void Promise.all([
      workorchApi.healthz().then((item) => setHealth(item.status)).catch((err: Error) => setHealth(err.message)),
      workorchApi.readyz().then((item) => setReady(item.status)).catch((err: Error) => setReady(err.message)),
      workorchApi.listWorkspaces().then(setWorkspaces),
      workorchApi.listRuns().then(setRuns),
      workorchApi.listDecisions().then(setDecisions),
      workorchApi.listApprovals().then(setApprovals)
    ]);
  }, []);

  return (
    <PageContainer title="workorch management overview" subTitle="Local MVP service control plane">
      <ProCard gutter={16} wrap>
        <ProCard colSpan="25%">
          <Statistic title="Health" value={health} />
        </ProCard>
        <ProCard colSpan="25%">
          <Statistic title="Readiness" value={ready} />
        </ProCard>
        <ProCard colSpan="25%">
          <Statistic title="Workspaces" value={workspaces.length} />
        </ProCard>
        <ProCard colSpan="25%">
          <Statistic title="Runs" value={runs.length} />
        </ProCard>
      </ProCard>
      <ProCard gutter={16} wrap className="workorch-section">
        <ProCard title="Pending decisions" colSpan="50%">
          <Statistic value={decisions.filter((item) => item.state === 'open').length} suffix={`/ ${decisions.length}`} />
        </ProCard>
        <ProCard title="Pending approvals" colSpan="50%">
          <Statistic value={approvals.filter((item) => item.state === 'pending').length} suffix={`/ ${approvals.length}`} />
        </ProCard>
      </ProCard>
    </PageContainer>
  );
}

function WorkspacesPage() {
  const { message } = AntApp.useApp();
  const workspaceActionRef = useRef<ActionType | undefined>(undefined);
  const goalActionRef = useRef<ActionType | undefined>(undefined);
  const dodActionRef = useRef<ActionType | undefined>(undefined);
  const [modalOpen, setModalOpen] = useState(false);
  const [goalModalOpen, setGoalModalOpen] = useState(false);
  const [dodModalOpen, setDoDModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [wizardStep, setWizardStep] = useState(0);
  const [createdWorkspace, setCreatedWorkspace] = useState<Workspace | undefined>(undefined);
  const [selectedWorkspace, setSelectedWorkspace] = useState<Workspace | undefined>(undefined);
  const [workspaceGoals, setWorkspaceGoals] = useState<Goal[]>([]);
  const [workspaceDoDs, setWorkspaceDoDs] = useState<DoD[]>([]);
  const [definitionLoading, setDefinitionLoading] = useState(false);
  const [workspaceForm] = Form.useForm();
  const [wizardGoalForm] = Form.useForm();
  const [wizardDoDForm] = Form.useForm();
  const [goalForm] = Form.useForm();
  const [dodForm] = Form.useForm();

  const closeWorkspaceWizard = () => {
    setModalOpen(false);
    setWizardStep(0);
    setCreatedWorkspace(undefined);
    workspaceForm.resetFields();
    wizardGoalForm.resetFields();
    wizardDoDForm.resetFields();
  };

  const selectWorkspace = async (workspace?: Workspace) => {
    setSelectedWorkspace(workspace);
    if (!workspace) {
      setWorkspaceGoals([]);
      setWorkspaceDoDs([]);
      return;
    }
    setDefinitionLoading(true);
    try {
      const [goals, dods] = await Promise.all([
        workorchApi.listGoals(workspace.id),
        workorchApi.listDoDs(workspace.id)
      ]);
      setWorkspaceGoals(goals);
      setWorkspaceDoDs(dods);
    } catch (error) {
      setWorkspaceGoals([]);
      setWorkspaceDoDs([]);
      message.error(errorMessage(error));
    } finally {
      setDefinitionLoading(false);
    }
  };

  const reloadWorkspaceDefinitions = () => {
    void selectWorkspace(selectedWorkspace ?? createdWorkspace);
  };

  const columns: ProColumns<Workspace>[] = [
    { title: 'Workspace ID', dataIndex: 'id', copyable: true },
    { title: 'Title', dataIndex: 'title' },
    { title: 'Description', dataIndex: 'description' },
    { title: 'Working Directory', dataIndex: 'workingDirectory', copyable: true },
    { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    { title: 'Active Run', dataIndex: 'activeRunId', copyable: true },
    {
      title: 'Actions',
      valueType: 'option',
      render: (_, item) => (
        <Button size="small" onClick={() => void selectWorkspace(item)}>
          Manage
        </Button>
      )
    }
  ];
  const goalColumns: ProColumns<Goal>[] = [
    { title: 'Goal ID', dataIndex: 'id', copyable: true },
    { title: 'Goal', dataIndex: 'text' }
  ];
  const dodColumns: ProColumns<DoD>[] = [
    { title: 'DoD ID', dataIndex: 'id', copyable: true },
    { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    { title: 'Items', dataIndex: 'items', render: (_, item) => item.items.join(' / ') }
  ];

  return (
    <PageContainer
      title="Workspaces"
      subTitle="Manage each workspace with its Goal and Definition of Done"
      extra={<Button type="primary" onClick={() => setModalOpen(true)}>Create workspace</Button>}
    >
      <ProTable<Workspace>
        actionRef={workspaceActionRef}
        rowKey="id"
        search={false}
        columns={columns}
        request={async () => {
          const items = await workorchApi.listWorkspaces();
          if (!selectedWorkspace && items[0]) {
            void selectWorkspace(items[0]);
          }
          return tableResult(items);
        }}
        rowSelection={{
          type: 'radio',
          selectedRowKeys: selectedWorkspace ? [selectedWorkspace.id] : [],
          onChange: (_, rows) => {
            void selectWorkspace(rows[0]);
          }
        }}
        onRow={(item) => ({
          onClick: () => void selectWorkspace(item)
        })}
      />
      <ProCard
        title={selectedWorkspace ? `Workspace: ${selectedWorkspace.title}` : 'Workspace definitions'}
        subTitle={selectedWorkspace?.id || 'Select a workspace to manage Goal and DoD'}
        className="workorch-section"
        extra={
          <Space>
            <Button disabled={!selectedWorkspace} onClick={() => setGoalModalOpen(true)}>Define goal</Button>
            <Button type="primary" disabled={!selectedWorkspace} onClick={() => setDoDModalOpen(true)}>Define DoD</Button>
          </Space>
        }
      >
        {selectedWorkspace ? (
          <>
            <Descriptions
              column={2}
              items={[
                { key: 'id', label: 'Workspace ID', children: selectedWorkspace.id },
                { key: 'state', label: 'State', children: stateTag(selectedWorkspace.state) },
                { key: 'description', label: 'Description', children: selectedWorkspace.description || '-' },
                { key: 'workingDirectory', label: 'Working Directory', children: selectedWorkspace.workingDirectory }
              ]}
            />
            <ProCard gutter={16} wrap className="workorch-section">
              <ProCard title="Goals" colSpan="50%">
                <ProTable<Goal>
                  actionRef={goalActionRef}
                  rowKey="id"
                  search={false}
                  columns={goalColumns}
                  dataSource={workspaceGoals}
                  loading={definitionLoading}
                  pagination={false}
                />
              </ProCard>
              <ProCard title="Definition of Done" colSpan="50%">
                <ProTable<DoD>
                  actionRef={dodActionRef}
                  rowKey="id"
                  search={false}
                  columns={dodColumns}
                  dataSource={workspaceDoDs}
                  loading={definitionLoading}
                  pagination={false}
                />
              </ProCard>
            </ProCard>
          </>
        ) : (
          <Tag>No workspace selected</Tag>
        )}
      </ProCard>
      <Modal
        title="Create workspace"
        open={modalOpen}
        confirmLoading={submitting}
        onCancel={closeWorkspaceWizard}
        onOk={() => {
          if (wizardStep === 0) {
            workspaceForm.submit();
          } else if (wizardStep === 1) {
            wizardGoalForm.submit();
          } else {
            wizardDoDForm.submit();
          }
        }}
        okText={wizardStep === 0 ? 'Create workspace' : wizardStep === 1 ? 'Define goal' : 'Define DoD'}
      >
        <Steps
          current={wizardStep}
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
          style={{ display: wizardStep === 0 ? 'block' : 'none' }}
          onFinish={async (values: { title: string; description?: string; workingDirectory?: string }) => {
            setSubmitting(true);
            try {
              const workspace = await workorchApi.createWorkspace(values.title, values.description, values.workingDirectory);
              message.success('Workspace created, continue defining Goal');
              setCreatedWorkspace(workspace);
              void selectWorkspace(workspace);
              setWizardStep(1);
              workspaceActionRef.current?.reload();
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setSubmitting(false);
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
          form={wizardGoalForm}
          layout="vertical"
          style={{ display: wizardStep === 1 ? 'block' : 'none' }}
          onFinish={async (values: { text: string }) => {
            if (!createdWorkspace) {
              message.error('Workspace is not created');
              return;
            }
            setSubmitting(true);
            try {
              await workorchApi.defineGoal(createdWorkspace.id, values.text);
              message.success('Goal defined, continue defining DoD');
              setWizardStep(2);
              void selectWorkspace(createdWorkspace);
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setSubmitting(false);
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
          form={wizardDoDForm}
          layout="vertical"
          style={{ display: wizardStep === 2 ? 'block' : 'none' }}
          onFinish={async (values: { items: string }) => {
            if (!createdWorkspace) {
              message.error('Workspace is not created');
              return;
            }
            setSubmitting(true);
            try {
              const items = values.items.split('\n').map((item) => item.trim()).filter(Boolean);
              await workorchApi.defineDoD(createdWorkspace.id, items);
              message.success('Workspace Goal and DoD are ready');
              void selectWorkspace(createdWorkspace);
              closeWorkspaceWizard();
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setSubmitting(false);
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
      <Modal title="Define goal" open={goalModalOpen} confirmLoading={submitting} onCancel={() => setGoalModalOpen(false)} onOk={() => goalForm.submit()}>
        <Form
          form={goalForm}
          layout="vertical"
          onFinish={async (values: { text: string }) => {
            if (!selectedWorkspace) {
              message.error('Select a workspace first');
              return;
            }
            setSubmitting(true);
            try {
              await workorchApi.defineGoal(selectedWorkspace.id, values.text);
              message.success('Goal defined');
              setGoalModalOpen(false);
              goalForm.resetFields();
              reloadWorkspaceDefinitions();
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setSubmitting(false);
            }
          }}
        >
          <Form.Item name="text" label="Goal" rules={[{ required: true }]}>
            <Input.TextArea rows={4} placeholder="Describe the target outcome for this workspace" />
          </Form.Item>
        </Form>
      </Modal>
      <Modal title="Define DoD" open={dodModalOpen} confirmLoading={submitting} onCancel={() => setDoDModalOpen(false)} onOk={() => dodForm.submit()}>
        <Form
          form={dodForm}
          layout="vertical"
          onFinish={async (values: { items: string }) => {
            if (!selectedWorkspace) {
              message.error('Select a workspace first');
              return;
            }
            setSubmitting(true);
            try {
              const items = values.items.split('\n').map((item) => item.trim()).filter(Boolean);
              await workorchApi.defineDoD(selectedWorkspace.id, items);
              message.success('DoD defined');
              setDoDModalOpen(false);
              dodForm.resetFields();
              reloadWorkspaceDefinitions();
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setSubmitting(false);
            }
          }}
        >
          <Form.Item name="items" label="DoD Items" rules={[{ required: true }]}>
            <Input.TextArea rows={5} placeholder={'One acceptance criterion per line\nExample: service deployed locally'} />
          </Form.Item>
        </Form>
      </Modal>
    </PageContainer>
  );
}

function RunsPage() {
  const { message } = AntApp.useApp();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [goals, setGoals] = useState<Goal[]>([]);
  const [dods, setDoDs] = useState<DoD[]>([]);
  const [modalOpen, setModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    void workorchApi.listWorkspaces().then(setWorkspaces);
  }, []);

  const loadDefinitions = async (workspaceId?: string) => {
    if (!workspaceId) {
      setGoals([]);
      setDoDs([]);
      return;
    }
    const [nextGoals, nextDoDs] = await Promise.all([
      workorchApi.listGoals(workspaceId),
      workorchApi.listDoDs(workspaceId)
    ]);
    setGoals(nextGoals);
    setDoDs(nextDoDs);
  };

  const columns: ProColumns<Run>[] = [
    { title: 'Run ID', dataIndex: 'id', copyable: true },
    { title: 'Workspace', dataIndex: 'workspaceId', copyable: true },
    { title: 'Goal', dataIndex: 'goalId', copyable: true },
    { title: 'DoD', dataIndex: 'doDId', copyable: true },
    { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    { title: 'Blocking Ticket', dataIndex: 'blockingTicketId', copyable: true },
    { title: 'Blocking Approval', dataIndex: 'blockingApprovalId', copyable: true },
    {
      title: 'Actions',
      valueType: 'option',
      render: (_, item) => (
        <Space>
          <Button
            size="small"
            onClick={async () => {
              try {
                await workorchApi.startRun(item.id);
                message.success('Run started');
                actionRef.current?.reload();
              } catch (error) {
                message.error(errorMessage(error));
              }
            }}
          >
            Start
          </Button>
          <Button
            size="small"
            onClick={async () => {
              try {
                await workorchApi.resumeRun(item.id);
                message.success('Run resumed');
                actionRef.current?.reload();
              } catch (error) {
                message.error(errorMessage(error));
              }
            }}
          >
            Resume
          </Button>
        </Space>
      )
    }
  ];

  return (
    <PageContainer
      title="Runs"
      extra={<Button type="primary" onClick={() => {
        setModalOpen(true);
        void loadDefinitions(workspaces[0]?.id);
      }}>Create run</Button>}
    >
      <ProTable<Run>
        actionRef={actionRef}
        rowKey="id"
        search={false}
        columns={columns}
        request={async () => tableResult(await workorchApi.listRuns())}
      />
      <Modal title="Create run" open={modalOpen} confirmLoading={submitting} onCancel={() => setModalOpen(false)} onOk={() => form.submit()}>
        <Form
          form={form}
          layout="vertical"
          onValuesChange={(changed: { workspaceId?: string }) => {
            if (changed.workspaceId !== undefined) {
              void loadDefinitions(changed.workspaceId);
            }
          }}
          onFinish={async (values: { workspaceId: string; runId: string; goalId?: string; doDId?: string }) => {
            setSubmitting(true);
            try {
              await workorchApi.createRun(values.workspaceId, values.runId, values.goalId, values.doDId);
              message.success('Run created');
              setModalOpen(false);
              form.resetFields();
              actionRef.current?.reload();
            } catch (error) {
              message.error(errorMessage(error));
            } finally {
              setSubmitting(false);
            }
          }}
        >
          <Form.Item name="workspaceId" label="Workspace ID" rules={[{ required: true }]}>
            <Input list="workspace-options" placeholder={workspaces[0]?.id || 'ws-local'} />
          </Form.Item>
          <datalist id="workspace-options">
            {workspaces.map((item) => <option key={item.id} value={item.id} />)}
          </datalist>
          <Form.Item name="runId" label="Run ID" rules={[{ required: true }]}>
            <Input placeholder="run-local" />
          </Form.Item>
          <Form.Item name="goalId" label="Goal ID" rules={[{ required: true }]}>
            <Input list="goal-options" placeholder={goals[0]?.id || 'Define a goal first'} />
          </Form.Item>
          <datalist id="goal-options">
            {goals.map((item) => <option key={item.id} value={item.id}>{item.text}</option>)}
          </datalist>
          <Form.Item name="doDId" label="DoD ID" rules={[{ required: true }]}>
            <Input list="dod-options" placeholder={dods[0]?.id || 'Define a DoD first'} />
          </Form.Item>
          <datalist id="dod-options">
            {dods.map((item) => <option key={item.id} value={item.id}>{item.items.join(' / ')}</option>)}
          </datalist>
        </Form>
      </Modal>
    </PageContainer>
  );
}

function DecisionsPage() {
  const { message } = AntApp.useApp();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const columns: ProColumns<Decision>[] = [
    { title: 'Ticket ID', dataIndex: 'id', copyable: true },
    { title: 'Workspace', dataIndex: 'workspaceId', copyable: true },
    { title: 'Run', dataIndex: 'runId', copyable: true },
    { title: 'Reason', dataIndex: 'reason' },
    { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    { title: 'Resolution', dataIndex: 'resolutionType' },
    {
      title: 'Actions',
      valueType: 'option',
      render: (_, item) => (
        <Space>
          <Button
            size="small"
            onClick={async () => {
              try {
                await workorchApi.resolveDecision(item.id);
                message.success('Decision resolved');
                actionRef.current?.reload();
              } catch (error) {
                message.error(errorMessage(error));
              }
            }}
          >
            Resolve
          </Button>
          <Button
            size="small"
            danger
            onClick={async () => {
              try {
                await workorchApi.rejectDecision(item.id);
                message.success('Decision rejected');
                actionRef.current?.reload();
              } catch (error) {
                message.error(errorMessage(error));
              }
            }}
          >
            Reject
          </Button>
        </Space>
      )
    }
  ];
  return (
    <PageContainer title="Decisions">
      <ProTable<Decision> actionRef={actionRef} rowKey="id" search={false} columns={columns} request={async () => tableResult(await workorchApi.listDecisions())} />
    </PageContainer>
  );
}

function ApprovalsPage() {
  const { message } = AntApp.useApp();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const columns: ProColumns<Approval>[] = [
    { title: 'Approval ID', dataIndex: 'id', copyable: true },
    { title: 'Workspace', dataIndex: 'workspaceId', copyable: true },
    { title: 'Run', dataIndex: 'runId', copyable: true },
    { title: 'Reason', dataIndex: 'reason' },
    { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    {
      title: 'Actions',
      valueType: 'option',
      render: (_, item) => (
        <Space>
          <Button
            size="small"
            onClick={async () => {
              try {
                await workorchApi.approveApproval(item.id);
                message.success('Approval approved');
                actionRef.current?.reload();
              } catch (error) {
                message.error(errorMessage(error));
              }
            }}
          >
            Approve
          </Button>
          <Button
            size="small"
            danger
            onClick={async () => {
              try {
                await workorchApi.rejectApproval(item.id);
                message.success('Approval rejected');
                actionRef.current?.reload();
              } catch (error) {
                message.error(errorMessage(error));
              }
            }}
          >
            Reject
          </Button>
        </Space>
      )
    }
  ];
  return (
    <PageContainer title="Approvals">
      <ProTable<Approval> actionRef={actionRef} rowKey="id" search={false} columns={columns} request={async () => tableResult(await workorchApi.listApprovals())} />
    </PageContainer>
  );
}

function SlicesPage() {
  const { message } = AntApp.useApp();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [catalog, setCatalog] = useState<SliceDescriptor[]>([]);

  useEffect(() => {
    void workorchApi.listSlices().then(setCatalog);
  }, []);

  const columns: ProColumns<SliceStatus>[] = [
    { title: 'Slice ID', dataIndex: 'id', copyable: true },
    { title: 'Title', dataIndex: 'title' },
    { title: 'Workspace', dataIndex: 'workspaceId', copyable: true },
    { title: 'Run', dataIndex: 'runId', copyable: true },
    { title: 'Run State', dataIndex: 'runState', render: (_, item) => stateTag(item.runState) },
    { title: 'Steps', dataIndex: 'stepCount' },
    { title: 'Observed', dataIndex: 'observedCount' },
    {
      title: 'Actions',
      valueType: 'option',
      render: (_, item) => (
        <Button
          size="small"
          onClick={async () => {
            try {
              await workorchApi.runSlice(item.id);
              message.success('Slice executed');
              actionRef.current?.reload();
            } catch (error) {
              message.error(errorMessage(error));
            }
          }}
        >
          Run slice
        </Button>
      )
    }
  ];

  return (
    <PageContainer title="Slices">
      <ProCard title="Available slice catalog" className="workorch-section">
        <Space wrap>
          {catalog.map((item) => <Tag key={item.id}>{item.id} - {item.title}</Tag>)}
        </Space>
      </ProCard>
      <ProTable<SliceStatus> actionRef={actionRef} rowKey="id" search={false} columns={columns} request={async () => tableResult(await workorchApi.listSliceStatuses())} />
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
    <PageContainer title="Service">
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
  const [view, setView] = useState<ViewKey>('overview');
  const renderContent = () => {
    switch (view) {
      case 'workspaces':
        return <WorkspacesPage />;
      case 'runs':
        return <RunsPage />;
      case 'decisions':
        return <DecisionsPage />;
      case 'approvals':
        return <ApprovalsPage />;
      case 'slices':
        return <SlicesPage />;
      case 'service':
        return <ServicePage />;
      default:
        return <OverviewPage />;
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
                setView((item.path?.replace('/', '') as ViewKey) || 'overview');
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
