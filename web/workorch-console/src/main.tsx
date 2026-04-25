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
import { App as AntApp, Button, ConfigProvider, Descriptions, Form, Input, Modal, Space, Statistic, Tag, theme } from 'antd';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import type { Approval, Decision, Run, SliceDescriptor, SliceStatus, Workspace } from './types';
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
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [modalOpen, setModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm();
  const columns: ProColumns<Workspace>[] = [
    { title: 'Workspace ID', dataIndex: 'id', copyable: true },
    { title: 'Title', dataIndex: 'title' },
    { title: 'Description', dataIndex: 'description' },
    { title: 'Working Directory', dataIndex: 'workingDirectory', copyable: true },
    { title: 'State', dataIndex: 'state', render: (_, item) => stateTag(item.state) },
    { title: 'Active Run', dataIndex: 'activeRunId', copyable: true }
  ];

  return (
    <PageContainer title="Workspaces" extra={<Button type="primary" onClick={() => setModalOpen(true)}>Create workspace</Button>}>
      <ProTable<Workspace>
        actionRef={actionRef}
        rowKey="id"
        search={false}
        columns={columns}
        request={async () => tableResult(await workorchApi.listWorkspaces())}
      />
      <Modal
        title="Create workspace"
        open={modalOpen}
        confirmLoading={submitting}
        onCancel={() => setModalOpen(false)}
        onOk={() => form.submit()}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={async (values: { title: string; description?: string; workingDirectory?: string }) => {
            setSubmitting(true);
            try {
              await workorchApi.createWorkspace(values.title, values.description, values.workingDirectory);
              message.success('Workspace created');
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
      </Modal>
    </PageContainer>
  );
}

function RunsPage() {
  const { message } = AntApp.useApp();
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [modalOpen, setModalOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    void workorchApi.listWorkspaces().then(setWorkspaces);
  }, []);

  const columns: ProColumns<Run>[] = [
    { title: 'Run ID', dataIndex: 'id', copyable: true },
    { title: 'Workspace', dataIndex: 'workspaceId', copyable: true },
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
    <PageContainer title="Runs" extra={<Button type="primary" onClick={() => setModalOpen(true)}>Create run</Button>}>
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
          <Form.Item name="goalId" label="Goal ID">
            <Input placeholder="goal-local" />
          </Form.Item>
          <Form.Item name="doDId" label="DoD ID">
            <Input placeholder="dod-local" />
          </Form.Item>
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
