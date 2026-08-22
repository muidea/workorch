import { useEffect, useState } from 'react';
import { App as AntApp, Button, Descriptions, Empty, Form, Input, InputNumber, Modal, Select, Space, Tag } from 'antd';
import { CardGrid, PageContainer, ProCard, ProTable } from '../components/proCompat';
import type { ProColumns } from '../components/proCompat';
import type { MCPCapability, MCPCapabilityMutation, MCPPrompt, MCPResource, MCPServer, MCPTool } from '../types';
import { workorchApi } from '../api';
import { modelLabels } from '../utils/constants';
import {
  joinLines,
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

function parseEnvironmentReferenceLines(value?: string) {
  const references: Record<string, string> = {};
  for (const line of splitLines(value)) {
    const index = line.indexOf('=');
    const key = (index >= 0 ? line.slice(0, index) : line).trim();
    if (!key) {
      continue;
    }
    references[key] = index >= 0 ? line.slice(index + 1).trim() : '';
  }
  return Object.keys(references).length > 0 ? references : undefined;
}

function joinEnvironmentReferences(references?: Record<string, string>) {
  return Object.entries(references ?? {})
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, reference]) => `${name}=${reference}`)
    .join('\n');
}

function mcpServerPayload(values: MCPServerFormValues): MCPServer {
  const payload: MCPServer = {
    id: values.id.trim(),
    name: values.name.trim(),
    transport: values.transport,
    endpoint: values.endpoint?.trim(),
    command: values.command?.trim(),
    args: splitLines(values.args),
    enabled: values.enabled === true,
    allowedTools: splitLines(values.allowedTools),
    allowedResources: splitLines(values.allowedResources),
    allowedPrompts: splitLines(values.allowedPrompts),
    defaultApprovalPolicy: values.defaultApprovalPolicy?.trim(),
    sessionPolicy: values.sessionPolicy?.trim(),
    timeoutMillis: values.timeoutMillis
  };
  if (values.environmentRefsMode === 'clear') {
    payload.environmentRefs = {};
  } else if (values.environmentRefsMode === 'set') {
    payload.environmentRefs = parseEnvironmentReferenceLines(values.environmentRefs) ?? {};
  }
  // keep intentionally omits environmentRefs so the backend preserves the owner value.
  return payload;
}

interface MCPServerFormValues {
  id: string;
  name: string;
  transport: string;
  endpoint?: string;
  command?: string;
  args?: string;
  environmentRefsMode?: 'keep' | 'set' | 'clear';
  environmentRefs?: string;
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
  id: string;
  serverId: string;
  kind: 'tool' | 'resource' | 'prompt' | string;
  name: string;
  displayName: string;
  description?: string;
  category?: string;
  tags?: string[];
  stageHints?: string[];
  usageHint?: string;
  inputSummary?: string;
  outputSummary?: string;
  riskLevel?: string;
  effectScope?: string;
  idempotencyClass?: string;
  validationProofApplicability?: string;
  approvalPolicy?: string;
  schema?: string;
  uri?: string;
  enabled: boolean;
}

function parseCapabilitySchema(value?: string) {
  const text = value?.trim();
  if (!text) {
    return undefined;
  }
  const parsed = JSON.parse(text) as unknown;
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('schema must be a JSON object');
  }
  return parsed as Record<string, unknown>;
}

function capabilitySchemaText(schema?: Record<string, unknown>) {
  return schema ? JSON.stringify(schema, null, 2) : '';
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
      const nextServers = await workorchApi.listMCPServers();
      setServers(nextServers);
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
      command: server.command,
      args: joinLines(server.args),
      environmentRefsMode: 'keep',
      environmentRefs: '',
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
    setSelectedServerId(undefined);
    setTools([]);
    setResources([]);
    setPrompts([]);
    serverForm.setFieldsValue({
      id: `mcp-runtime-${nextIndex}`,
      name: `Runtime MCP ${nextIndex}`,
      transport: 'stdio',
      enabled: true,
      sessionPolicy: 'transient',
      timeoutMillis: 60000,
      endpoint: '',
      command: '',
      args: '',
      environmentRefsMode: 'keep',
      environmentRefs: '',
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
      schema: capabilitySchemaText(item.schema),
      enabled: item.enabled !== false
    });
    setCapabilityModalOpen(true);
  };

  const createCapability = () => {
    setEditingCapability(undefined);
    capabilityForm.setFieldsValue({
      id: '',
      serverId: selectedServerId || '',
      kind: 'tool',
      name: '',
      displayName: '',
      description: '',
      category: '',
      tags: [],
      stageHints: [],
      usageHint: '',
      inputSummary: '',
      outputSummary: '',
      riskLevel: undefined,
      approvalPolicy: '',
      effectScope: 'unknown',
      idempotencyClass: 'unknown',
      validationProofApplicability: 'not_declared',
      schema: '',
      uri: '',
      enabled: true
    });
    setCapabilityModalOpen(true);
  };

  const capabilityPayload = (values: MCPCapabilityFormValues): MCPCapabilityMutation => ({
    id: (editingCapability?.id || values.id || '').trim(),
    serverId: (editingCapability?.serverId || values.serverId || '').trim(),
    kind: (editingCapability?.kind || values.kind || '').trim(),
    name: (editingCapability?.name || values.name || '').trim(),
    displayName: values.displayName?.trim(),
    description: values.description?.trim(),
    category: values.category?.trim(),
    tags: values.tags ?? [],
    stageHints: values.stageHints ?? [],
    usageHint: values.usageHint?.trim(),
    inputSummary: values.inputSummary?.trim(),
    outputSummary: values.outputSummary?.trim(),
    riskLevel: values.riskLevel?.trim(),
    effectScope: values.effectScope?.trim(),
    idempotencyClass: values.idempotencyClass?.trim(),
    validationProofApplicability: values.validationProofApplicability?.trim(),
    approvalPolicy: values.approvalPolicy?.trim(),
    schema: parseCapabilitySchema(values.schema),
    uri: values.uri?.trim(),
    enabled: values.enabled !== false
  });

  const saveCapability = async () => {
    setSaving(true);
    try {
      const values = await capabilityForm.validateFields();
      const payload = capabilityPayload(values);
      if (editingCapability) {
        await workorchApi.updateMCPCapability(editingCapability.id, payload);
      } else {
        await workorchApi.createMCPCapability(payload);
      }
      message.success(`MCP 能力已${editingCapability ? '更新' : '创建'}`);
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
    { title: '启动', width: 120, render: (_, item) => item.endpoint?.trim() ? 'endpoint' : item.command?.trim() ? 'command' : '-' },
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
      subTitle={`管理${modelLabels.mcpCapability}目录与服务接入状态`}
      extra={<Button onClick={() => void loadRuntime()} loading={loading}>刷新</Button>}
    >
      <CardGrid loading={loading}>
        <ProCard title="MCP 能力摘要" colSpan={24}>
          <Space wrap>
            <Tag>{servers.length} 个服务</Tag>
            <Tag color="green">{servers.filter((item) => item.enabled).length} 个已启用</Tag>
            <Tag color="blue">{capabilities.filter((item) => item.kind === 'tool').length} 个工具</Tag>
            <Tag color="cyan">{capabilities.filter((item) => item.kind === 'resource').length} 个资源</Tag>
            <Tag color="geekblue">{capabilities.filter((item) => item.kind === 'prompt').length} 个提示词</Tag>
            <Tag color="purple">{servers.filter((item) => item.sessionPolicy === 'long_lived').length} 个长会话</Tag>
          </Space>
        </ProCard>
      </CardGrid>

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
              <Button onClick={createCapability}>新建能力</Button>
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
                  { key: 'effectScope', label: '影响范围', children: item.effectScope || '-' },
                  { key: 'idempotencyClass', label: '幂等类别', children: item.idempotencyClass || '-' },
                  { key: 'proofApplicability', label: '验证证明适用性', children: item.validationProofApplicability || '-' },
                  { key: 'provenance', label: '来源', children: `${item.source || '-'} / ${item.sourceRef || '-'} / revision ${item.revision ?? '-'}` },
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

      <CardGrid className="workorch-section">
        <ProCard title="已选 MCP 服务" colSpan={{ xs: 24, lg: 12 }}>
          {selectedServer ? (
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <Descriptions
                column={1}
                items={[
                  { key: 'id', label: '服务 ID', children: selectedServer.id },
                  { key: 'endpoint', label: '端点', children: displayMCPEndpoint(selectedServer.endpoint) || '-' },
                  { key: 'command', label: '命令', children: selectedServer.command || '-' },
                  { key: 'args', label: '参数', children: joinLines(selectedServer.args) || '-' },
                  { key: 'environmentRefs', label: '环境引用', children: joinEnvironmentReferences(selectedServer.environmentRefs) || '-' },
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
              <Input disabled={Boolean(selectedServerId)} placeholder="mcp-filesystem" />
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
            <Form.Item name="command" label="命令">
              <Input placeholder="endpoint 为空时使用，例如 lsp-bridge" />
            </Form.Item>
            <Form.Item name="args" label="参数">
              <Input.TextArea rows={2} placeholder="每行一个参数；不使用 shell 字符串" />
            </Form.Item>
            <Form.Item name="environmentRefs" label="环境引用">
              <Input.TextArea rows={3} placeholder="每行 KEY=env://ENV_NAME；不得填写密钥明文" />
            </Form.Item>
            <Form.Item name="environmentRefsMode" label="环境引用提交方式">
              <Select options={[
                { value: 'keep', label: '保留已有环境引用' },
                { value: 'set', label: '使用上方输入覆盖' },
                { value: 'clear', label: '清空环境引用' }
              ]} />
            </Form.Item>
            <Form.Item name="enabled" label="是否启用" rules={[{ required: true }]}>
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
      </CardGrid>
      <Modal
        title={editingCapability ? '编辑 MCP 能力' : '新建 MCP 能力'}
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
          <Form.Item name="id" label="能力 ID" rules={[{ required: true }]}>
            <Input disabled={Boolean(editingCapability)} placeholder="server-tool-name" />
          </Form.Item>
          <Form.Item name="serverId" label="所属服务" rules={[{ required: true }]}>
            <Select
              disabled={Boolean(editingCapability)}
              placeholder="选择 MCP 服务"
              options={servers.map((item) => ({ value: item.id, label: `${item.name} / ${item.id}` }))}
            />
          </Form.Item>
          <Form.Item name="kind" label="能力类型" rules={[{ required: true }]}>
            <Select
              disabled={Boolean(editingCapability)}
              options={[
                { value: 'tool', label: modelLabels.mcpTool },
                { value: 'resource', label: modelLabels.mcpResource },
                { value: 'prompt', label: modelLabels.mcpPrompt }
              ]}
            />
          </Form.Item>
          <Form.Item name="name" label="技术标识" rules={[{ required: true }]}>
            <Input disabled={Boolean(editingCapability)} placeholder="tool/resource/prompt name" />
          </Form.Item>
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
          <Form.Item name="uri" label="资源 URI">
            <Input placeholder="仅资源能力需要填写" />
          </Form.Item>
          <Form.Item
            name="schema"
            label="输入 Schema"
            rules={[
              {
                validator: async (_, value?: string) => {
                  parseCapabilitySchema(value);
                }
              }
            ]}
          >
            <Input.TextArea rows={5} placeholder={'{\n  "type": "object"\n}'} />
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
          <Form.Item name="effectScope" label="影响范围">
            <Select
              options={[
                { value: 'unknown', label: '未知' },
                { value: 'none', label: '无持久影响' },
                { value: 'ephemeral_session', label: '临时会话' },
                { value: 'workspace_durable', label: 'Workspace 持久写入' },
                { value: 'remote_durable', label: '远端持久写入' },
                { value: 'governance_only', label: '仅治理' }
              ]}
            />
          </Form.Item>
          <Form.Item name="idempotencyClass" label="幂等类别">
            <Select
              options={[
                { value: 'unknown', label: '未知' },
                { value: 'safe_to_replay', label: '可安全重放' },
                { value: 'at_most_once', label: '至多一次' },
                { value: 'owner_managed', label: 'Owner 管理' }
              ]}
            />
          </Form.Item>
          <Form.Item name="validationProofApplicability" label="验证证明适用性">
            <Select
              options={[
                { value: 'not_declared', label: '未声明' },
                { value: 'direct', label: '直接证明' },
                { value: 'auxiliary', label: '辅助证明' }
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
