import { useEffect, useState } from 'react';
import { App as AntApp, Button, Descriptions, Empty, Form, Input, InputNumber, Modal, Select, Space, Tag } from 'antd';
import { PageContainer, ProCard, ProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import type { MCPCapability, MCPPrompt, MCPResource, MCPServer, MCPTool } from '../types';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import {
  joinLines,
  metricValue,
  capabilityKindTag,
  errorMessage,
  splitLines
} from '../helpers';

function displayMCPEndpoint(endpoint?: string) {
  const value = endpoint?.trim() ?? '';
  const marker = '.workorch-data/system/mcp-endpoints/';
  const index = value.indexOf(marker);
  return index >= 0 ? value.slice(index) : value;
}

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

interface MCPServerFormValues {
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
}

interface MCPCapabilityFilterValues {
  serverId?: string;
  kind?: string;
  category?: string;
  stage?: string;
  riskLevel?: string;
  enabled?: boolean;
  q?: string;
}

interface MCPCapabilityFormValues {
  displayName: string;
  description?: string;
  category?: string;
  tags?: string[];
  stageHints?: string[];
  usageHint?: string;
  inputSummary?: string;
  outputSummary?: string;
  riskLevel?: string;
  approvalPolicy?: string;
  status?: string;
  enabled: boolean;
}

export function RuntimeControlPage() {
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
