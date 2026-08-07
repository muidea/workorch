import { useEffect, useMemo, useState } from 'react';
import { App as AntApp, Button, Checkbox, Descriptions, Empty, Form, Input, InputNumber, Select, Space, Table, Tabs, Tag, Typography } from 'antd';
import { CardGrid, PageContainer, ProCard } from '../components/proCompat';
import type {
  ProviderModelDescriptorView,
  ServiceConfiguration,
  Soul,
  UpdateLLMConfigurationRequest,
  UpdateServerConfigurationRequest,
  UpdateWebSearchProviderConfiguration,
  WorkspaceTemplate
} from '../types';
import { workorchApi } from '../api';
import {
  configurationModelTargetOptions,
  errorMessage,
  llmConfigurationFormValues,
  nextWebSearchProviderDraft,
  parseProviderModelTargetKey,
  providerModelTargetKey,
  splitHeaderLines,
  SoulDefinitionManager,
  validateHeaderLines,
  webSearchConfigurationFormValues,
  webSearchProviderDraftFromTemplate,
  webSearchProviderFormValues,
  WorkspaceTemplateManager,
  type LLMConfigurationFormValues,
  type WebSearchProviderFormValues
} from '../helpers';

function configNumberText(value: number | undefined) {
  return typeof value === 'number' && Number.isFinite(value) ? value.toLocaleString() : '-';
}

const maxAIProxyTimeoutMillis = 10 * 60 * 1000;

function goDurationMillis(raw: string | undefined): number | undefined {
  const value = raw?.trim();
  if (!value) return undefined;
  const parts = Array.from(value.matchAll(/(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g));
  if (parts.length === 0 || parts.map((part) => part[0]).join('') !== value) return undefined;
  const unitMillis: Record<string, number> = {
    ns: 1e-6,
    us: 1e-3,
    'µs': 1e-3,
    ms: 1,
    s: 1000,
    m: 60 * 1000,
    h: 60 * 60 * 1000
  };
  return parts.reduce((total, part) => total + Number(part[1]) * unitMillis[part[2]], 0);
}

function validateAIProxyTimeout(_: unknown, raw: string | undefined) {
  const timeoutMillis = goDurationMillis(raw);
  if (timeoutMillis === undefined || timeoutMillis <= 0) {
    return Promise.reject(new Error('请输入正数 Go duration，例如 180s 或 3m'));
  }
  if (timeoutMillis > maxAIProxyTimeoutMillis) {
    return Promise.reject(new Error('请求超时不得超过 10m'));
  }
  return Promise.resolve();
}

export function ConfigurationPage() {
  const { message } = AntApp.useApp();
  const [serverForm] = Form.useForm<UpdateServerConfigurationRequest>();
  const [llmForm] = Form.useForm<LLMConfigurationFormValues>();
  const [webSearchProviderEditorForm] = Form.useForm<WebSearchProviderFormValues>();
  const [configuration, setConfiguration] = useState<ServiceConfiguration | undefined>(undefined);
  const [souls, setSouls] = useState<Soul[]>([]);
  const [workspaceTemplates, setWorkspaceTemplates] = useState<WorkspaceTemplate[]>([]);
  const [defaultWebSearchProviderId, setDefaultWebSearchProviderId] = useState<string | undefined>(undefined);
  const [webSearchProviderDrafts, setWebSearchProviderDrafts] = useState<UpdateWebSearchProviderConfiguration[]>([]);
  const [selectedWebSearchProviderIndex, setSelectedWebSearchProviderIndex] = useState(0);
  const [selectedWebSearchTemplateId, setSelectedWebSearchTemplateId] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(false);
  const [serverSaving, setServerSaving] = useState(false);
  const [saving, setSaving] = useState(false);
  const [catalogRefreshing, setCatalogRefreshing] = useState(false);
  const [webSearchSaving, setWebSearchSaving] = useState(false);

  const applyConfiguration = (nextConfiguration: ServiceConfiguration) => {
    const nextFormValues = llmConfigurationFormValues(nextConfiguration);
    const nextWebSearchFormValues = webSearchConfigurationFormValues(nextConfiguration);
    setConfiguration(nextConfiguration);
    setDefaultWebSearchProviderId(nextWebSearchFormValues.defaultProviderId);
    setWebSearchProviderDrafts(nextWebSearchFormValues.providers);
    setSelectedWebSearchProviderIndex(0);
    setSelectedWebSearchTemplateId(nextConfiguration.webSearch.providerTemplates?.[0]?.templateId);
    serverForm.setFieldsValue({ defaultDialogLanguage: nextConfiguration.server.defaultDialogLanguage || '' });
    llmForm.setFieldsValue(nextFormValues);
    webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(nextWebSearchFormValues.providers[0] ?? nextWebSearchProviderDraft([])));
  };

  const loadConfiguration = async () => {
    setLoading(true);
    try {
      const [nextConfiguration, nextSouls, nextWorkspaceTemplates] = await Promise.all([
        workorchApi.readConfiguration(),
        workorchApi.listSouls(),
        workorchApi.listWorkspaceTemplates()
      ]);
      setSouls(nextSouls);
      setWorkspaceTemplates(nextWorkspaceTemplates);
      applyConfiguration(nextConfiguration);
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadConfiguration();
  }, []);

  const saveServerConfiguration = async (values: UpdateServerConfigurationRequest) => {
    setServerSaving(true);
    try {
      const nextConfiguration = await workorchApi.updateServerConfiguration(values);
      applyConfiguration(nextConfiguration);
      message.success('服务配置已更新');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setServerSaving(false);
    }
  };

  const saveLLMConfiguration = async (values: LLMConfigurationFormValues) => {
    setSaving(true);
    try {
      const selectableChatTargets = configurationModelTargetOptions(configuration, 'responses', configuration?.llm.defaultChat);
      const defaultChat = parseProviderModelTargetKey(values.defaultChatTargetKey);
      const currentChatTargetKey = providerModelTargetKey(
        configuration?.llm.defaultChat?.providerId,
        configuration?.llm.defaultChat?.modelId
      );
      // A credential rotation must remain possible when the old key has
      // already expired and the catalog therefore contains no ready target.
      // A changed target still has to be ready; the currently configured
      // target is allowed so the connection can be repaired first.
      if (!defaultChat || (!selectableChatTargets.some((item) => item.ready && item.value === values.defaultChatTargetKey) && values.defaultChatTargetKey !== currentChatTargetKey)) {
        message.error('请选择完整且 ready 的默认对话模型（Provider/Model）');
        return;
      }
      const selectableEmbeddingTargets = configurationModelTargetOptions(configuration, 'embeddings', configuration?.llm.defaultEmbedding);
      const defaultEmbedding = parseProviderModelTargetKey(values.defaultEmbeddingTargetKey);
      const currentEmbeddingTargetKey = providerModelTargetKey(
        configuration?.llm.defaultEmbedding?.providerId,
        configuration?.llm.defaultEmbedding?.modelId
      );
      if (values.defaultEmbeddingTargetKey && (
        !defaultEmbedding ||
        (!selectableEmbeddingTargets.some((item) => item.ready && item.value === values.defaultEmbeddingTargetKey) && values.defaultEmbeddingTargetKey !== currentEmbeddingTargetKey)
      )) {
        message.error('请选择完整且 ready 的默认 Embedding 模型（Provider/Model）');
        return;
      }
      const payload: UpdateLLMConfigurationRequest = {
        aiProxy: {
          baseURL: values.aiProxy?.baseURL?.trim(),
          apiKey: values.aiProxy?.apiKey?.trim() || undefined,
          timeout: values.aiProxy?.timeout?.trim(),
          catalogRequestTimeout: values.aiProxy?.catalogRequestTimeout?.trim(),
          catalogTTL: values.aiProxy?.catalogTTL?.trim(),
          catalogMaxStaleAge: values.aiProxy?.catalogMaxStaleAge?.trim(),
          maxCatalogResponseBytes: values.aiProxy?.maxCatalogResponseBytes,
          maxCatalogModels: values.aiProxy?.maxCatalogModels
        },
        defaultChat: {
          providerID: defaultChat.providerId,
          modelID: defaultChat.modelId
        },
        defaultEmbedding: defaultEmbedding ? {
          providerID: defaultEmbedding.providerId,
          modelID: defaultEmbedding.modelId
        } : undefined,
        clearDefaultEmbedding: !defaultEmbedding,
        clearAIProxyApiKey: Boolean(values.clearAIProxyApiKey),
        budget: values.budget
      };
      if (payload.clearAIProxyApiKey) {
        if (payload.aiProxy) {
          // Frontend fail-fast: clear and non-empty new secret cannot both be present.
          if (payload.aiProxy.apiKey) {
            message.error('清空 ai-proxy API 密钥时不能同时填写新密钥');
            return;
          }
          payload.aiProxy.apiKey = undefined;
        }
      }
      const nextConfiguration = await workorchApi.updateLLMConfiguration(payload);
      applyConfiguration(nextConfiguration);
      // Config reload is immediate on the server; refresh the effective
      // ai-proxy catalog so readiness reflects the new key without restart.
      await refreshCatalog();
      message.success('LLM 配置已更新');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

  async function refreshCatalog() {
    setCatalogRefreshing(true);
    try {
      // The refresh action is also a recovery path for an expired credential.
      // If the form contains a new connection value, persist that value before
      // issuing /v1/models; otherwise refresh would deterministically reuse the
      // old server-side key and report a misleading 401.
      const values = llmForm.getFieldsValue();
      const currentAIProxy = configuration?.llm.aiProxy;
      const pendingAPIKey = values.aiProxy?.apiKey?.trim();
      const pendingBaseURL = values.aiProxy?.baseURL?.trim();
      const pendingTimeout = values.aiProxy?.timeout?.trim();
      const pendingCatalogRequestTimeout = values.aiProxy?.catalogRequestTimeout?.trim();
      const pendingCatalogTTL = values.aiProxy?.catalogTTL?.trim();
      const pendingCatalogMaxStaleAge = values.aiProxy?.catalogMaxStaleAge?.trim();
      const connectionChanged = Boolean(
        pendingAPIKey ||
        (pendingBaseURL && pendingBaseURL !== currentAIProxy?.baseUrl) ||
        values.clearAIProxyApiKey ||
        pendingTimeout !== currentAIProxy?.timeout ||
        pendingCatalogRequestTimeout !== currentAIProxy?.catalogRequestTimeout ||
        pendingCatalogTTL !== currentAIProxy?.catalogTTL ||
        pendingCatalogMaxStaleAge !== currentAIProxy?.catalogMaxStaleAge ||
        values.aiProxy?.maxCatalogResponseBytes !== currentAIProxy?.maxCatalogResponseBytes ||
        values.aiProxy?.maxCatalogModels !== currentAIProxy?.maxCatalogModels
      );
      if (connectionChanged) {
        const nextConfiguration = await workorchApi.updateLLMConfiguration({
          aiProxy: {
            baseURL: pendingBaseURL,
            apiKey: pendingAPIKey || undefined,
            timeout: pendingTimeout,
            catalogRequestTimeout: pendingCatalogRequestTimeout,
            catalogTTL: pendingCatalogTTL,
            catalogMaxStaleAge: pendingCatalogMaxStaleAge,
            maxCatalogResponseBytes: values.aiProxy?.maxCatalogResponseBytes,
            maxCatalogModels: values.aiProxy?.maxCatalogModels
          },
          clearAIProxyApiKey: Boolean(values.clearAIProxyApiKey)
        });
        applyConfiguration(nextConfiguration);
      }
      const catalog = await workorchApi.refreshLLMModelCatalog('ai-proxy');
      setConfiguration((current) => current ? {
        ...current,
        llm: {
          ...current.llm,
          catalog
        }
      } : current);
      message.success('模型目录已刷新');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setCatalogRefreshing(false);
    }
  }

  const saveWebSearchConfiguration = async () => {
    setWebSearchSaving(true);
    try {
      const nextConfiguration = await workorchApi.updateWebSearchConfiguration({
        defaultProviderId: defaultWebSearchProviderId,
        providers: webSearchProviderDrafts
      });
      applyConfiguration(nextConfiguration);
      message.success('Web Search 配置已更新');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setWebSearchSaving(false);
    }
  };

  const webSearchProviderTemplates = configuration?.webSearch.providerTemplates ?? [];
  const webSearchStatusByProviderID = new Map((configuration?.webSearch.providers ?? []).map((item) => [item.id, item]));
  const selectedWebSearchProvider = webSearchProviderDrafts[selectedWebSearchProviderIndex];
  const selectedWebSearchProviderStatus = selectedWebSearchProvider ? webSearchStatusByProviderID.get(selectedWebSearchProvider.id) : undefined;
  const runtimeGit = configuration?.runtime.git;
  const llmOutput = configuration?.runtime.llmOutput;
  const catalog = configuration?.llm.catalog;
  const chatModelOptions = useMemo(() => configurationModelTargetOptions(configuration, 'responses', configuration?.llm.defaultChat), [configuration]);
  const embeddingModelOptions = useMemo(() => configurationModelTargetOptions(configuration, 'embeddings', configuration?.llm.defaultEmbedding), [configuration]);

  const selectWebSearchProvider = (index: number) => {
    setSelectedWebSearchProviderIndex(index);
    webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(webSearchProviderDrafts[index] ?? nextWebSearchProviderDraft(webSearchProviderDrafts)));
  };

  const addWebSearchProvider = () => {
    const template = webSearchProviderTemplates.find((item) => item.templateId === selectedWebSearchTemplateId) ?? webSearchProviderTemplates[0];
    const next = template ? webSearchProviderDraftFromTemplate(template, webSearchProviderDrafts) : nextWebSearchProviderDraft(webSearchProviderDrafts);
    const nextDrafts = [...webSearchProviderDrafts, next];
    setWebSearchProviderDrafts(nextDrafts);
    setSelectedWebSearchProviderIndex(nextDrafts.length - 1);
    webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(next));
  };

  const removeSelectedWebSearchProvider = () => {
    if (webSearchProviderDrafts.length <= 1) {
      return;
    }
    const nextDrafts = webSearchProviderDrafts.filter((_, index) => index !== selectedWebSearchProviderIndex);
    const nextIndex = Math.max(0, Math.min(selectedWebSearchProviderIndex, nextDrafts.length - 1));
    setWebSearchProviderDrafts(nextDrafts);
    setSelectedWebSearchProviderIndex(nextIndex);
    if (defaultWebSearchProviderId && !nextDrafts.some((item) => item.id === defaultWebSearchProviderId)) {
      setDefaultWebSearchProviderId(nextDrafts[0]?.id);
    }
    webSearchProviderEditorForm.setFieldsValue(webSearchProviderFormValues(nextDrafts[nextIndex] ?? nextWebSearchProviderDraft([])));
  };

  const updateSelectedWebSearchProvider = (_: Partial<WebSearchProviderFormValues>, values: WebSearchProviderFormValues) => {
    setWebSearchProviderDrafts((current) => {
      const nextItems = current.map((item, index) => {
        if (index !== selectedWebSearchProviderIndex) {
          return item;
        }
        const previousID = item.id;
        const nextID = values.id?.trim() || previousID;
        if (defaultWebSearchProviderId === previousID && nextID && nextID !== previousID) {
          setDefaultWebSearchProviderId(nextID);
        }
        return {
          ...item,
          id: nextID,
          provider: values.provider,
          searchEndpoint: values.searchEndpoint,
          fetchEndpoint: values.fetchEndpoint,
          headers: splitHeaderLines(values.headers),
          apiKey: values.apiKey,
          timeout: values.timeout,
          maxResults: values.maxResults,
          maxFetchBytes: values.maxFetchBytes,
          maxRedirects: values.maxRedirects,
          allowDomains: Array.isArray(values.allowDomains) ? values.allowDomains : item.allowDomains,
          denyDomains: Array.isArray(values.denyDomains) ? values.denyDomains : item.denyDomains,
          disabled: values.disabled
        } as UpdateWebSearchProviderConfiguration;
      });
      return nextItems;
    });
  };

  const catalogColumns = [
    { title: '模型 ID', dataIndex: 'modelId', key: 'modelId' },
    { title: 'Route Owner', dataIndex: 'routeOwner', key: 'routeOwner', render: (value?: string) => value || '-' },
    { title: 'Context Window', dataIndex: 'contextWindowTokens', key: 'contextWindowTokens', render: (value?: number) => configNumberText(value) },
    { title: 'Max Output', dataIndex: 'maxOutputTokens', key: 'maxOutputTokens', render: (value?: number) => configNumberText(value) },
    { title: 'Endpoints', dataIndex: 'supportedEndpoints', key: 'supportedEndpoints', render: (items?: string[]) => (items?.length ? items.join(', ') : '-') },
    {
      title: 'Operations',
      dataIndex: 'operations',
      key: 'operations',
      render: (ops?: string[]) => (ops?.length ? ops.join(', ') : '-')
    },
    {
      title: 'Readiness',
      dataIndex: 'capabilityReady',
      key: 'capabilityReady',
      render: (ready: boolean, record: ProviderModelDescriptorView) => ready
        ? <Tag color="green">ready</Tag>
        : <Tag color="red">{record.unavailableReason || 'incomplete'}</Tag>
    }
  ];

  return (
    <PageContainer
      title="系统配置"
      subTitle="管理服务配置、执行人格（Soul）、工作空间模板（Workspace Template）、ai-proxy 模型与 Web Search Provider"
      extra={<Button onClick={() => void loadConfiguration()} loading={loading}>刷新</Button>}
    >
      <Tabs
        className="configuration-tabs"
        destroyOnHidden={false}
        items={[
          {
            key: 'service',
            label: '基础服务',
            children: (
              <CardGrid loading={loading}>
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
                <ProCard title="运行期 Git 能力" colSpan="50%">
                  <Descriptions
                    column={1}
                    items={[
                      { key: 'readCapability', label: 'Git 读取能力', children: '核心能力默认启用' },
                      { key: 'writeCapability', label: 'Git 写入能力', children: '核心能力默认启用' },
                      { key: 'defaultTimeout', label: '默认超时', children: runtimeGit?.defaultTimeout || '-' }
                    ]}
                  />
                </ProCard>
                <ProCard title="LLM 输出预算" colSpan="100%">
                  <Descriptions
                    column={{ xs: 1, md: 2, xl: 3 }}
                    items={[
                      { key: 'maxPatchContentBytes', label: '单个补丁内嵌字节', children: configNumberText(llmOutput?.maxPatchContentBytes) },
                      { key: 'maxGraphBytes', label: '单次 Graph 字节', children: configNumberText(llmOutput?.maxGraphBytes) },
                      { key: 'maxGraphFileWrites', label: 'Graph 写入节点数', children: configNumberText(llmOutput?.maxGraphFileWrites) },
                      { key: 'schemaInvalidLargePreviewThreshold', label: '大预览降级阈值', children: configNumberText(llmOutput?.schemaInvalidLargePreviewThreshold) },
                      { key: 'maxArtifactChunkBytes', label: 'Artifact Chunk 字节', children: configNumberText(llmOutput?.maxArtifactChunkBytes) },
                      { key: 'previousChunkTailBytes', label: '上一 Chunk 尾部字节', children: configNumberText(llmOutput?.previousChunkTailBytes) },
                      { key: 'maxChunkRetryAttempts', label: 'Chunk 重试次数', children: configNumberText(llmOutput?.maxChunkRetryAttempts) },
                      { key: 'maxArtifactPlanAttempts', label: 'Artifact Plan 尝试次数', children: configNumberText(llmOutput?.maxArtifactPlanAttempts) },
                      { key: 'maxArtifactReplanAttempts', label: 'Artifact Replan 次数', children: configNumberText(llmOutput?.maxArtifactReplanAttempts) }
                    ]}
                  />
                </ProCard>
              </CardGrid>
            )
          },
          {
            key: 'llm',
            label: 'LLM / ai-proxy',
            children: (
              <ProCard
                title="ai-proxy 首选 LLM"
                className="workorch-section"
                extra={(
                  <Space>
                    <Button onClick={() => void llmForm.validateFields([['aiProxy', 'timeout']]).then(() => refreshCatalog())} loading={catalogRefreshing}>刷新模型</Button>
                    <Button type="primary" onClick={() => void llmForm.validateFields().then((values) => saveLLMConfiguration(values))} loading={saving}>保存 LLM 配置</Button>
                  </Space>
                )}
              >
                <Typography.Paragraph type="secondary">
                  WorkOrch 默认连接内置 ai-proxy。内置模型来自 `/v1/models` 精确目录，自定义模型来自高级 Provider 声明；默认目标按 Provider/Model 完整选择，不会静默匹配同名模型。
                </Typography.Paragraph>
                <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                  <ProCard title="连接与默认模型">
                    <Form form={llmForm} layout="vertical" onFinish={(values) => void saveLLMConfiguration(values)}>
                      <Form.Item name={['aiProxy', 'baseURL']} label="ai-proxy BaseURL" rules={[{ required: true, message: '请填写 ai-proxy BaseURL' }]}>
                        <Input placeholder="http://127.0.0.1:8080" />
                      </Form.Item>
                      <Form.Item name={['aiProxy', 'apiKey']} label="API 密钥">
                        <Input.Password placeholder={configuration?.llm.aiProxy?.apiKeyConfigured ? '已配置，留空表示保留' : '可选'} autoComplete="new-password" />
                      </Form.Item>
                      <Form.Item name="clearAIProxyApiKey" valuePropName="checked">
                        <Checkbox>显式清空 ai-proxy API 密钥</Checkbox>
                      </Form.Item>
                      <Form.Item name={['aiProxy', 'timeout']} label="请求超时" rules={[{ validator: validateAIProxyTimeout }]} extra="默认 180s，最大 10m；应大于 ai-proxy body-idle timeout 并小于 Runtime 恢复预算。">
                        <Input placeholder="180s" />
                      </Form.Item>
                      <Form.Item
                        name="defaultChatTargetKey"
                        label="默认对话模型"
                        rules={[
                          { required: true, message: '请选择默认对话模型' },
                          {
                            validator: (_, value?: string) => chatModelOptions.some((item) => item.ready && item.value === value)
                              ? Promise.resolve()
                              : Promise.reject(new Error('已配置目标当前未 ready，请选择 ready 的 Provider/Model'))
                          }
                        ]}
                      >
                        <Select
                          showSearch
                          placeholder={chatModelOptions.some((item) => item.ready) ? '选择 ready Responses 模型' : '请先刷新 ai-proxy 模型目录'}
                          options={chatModelOptions}
                        />
                      </Form.Item>
                      <Form.Item
                        name="defaultEmbeddingTargetKey"
                        label="默认 Embedding 模型"
                        rules={[
                          {
                            validator: (_, value?: string) => !value || embeddingModelOptions.some((item) => item.ready && item.value === value)
                              ? Promise.resolve()
                              : Promise.reject(new Error('已配置目标当前未 ready，请选择 ready 的 Provider/Model 或清空'))
                          }
                        ]}
                      >
                        <Select
                          allowClear
                          showSearch
                          placeholder="可选，仅展示 embeddings 能力模型"
                          options={embeddingModelOptions}
                        />
                      </Form.Item>
                      <Form.Item name={['budget', 'contextUtilizationRatio']} label="上下文利用率">
                        <InputNumber min={0.05} max={1} step={0.05} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['budget', 'estimatedTokensPerByte']} label="Token/Byte 估算">
                        <InputNumber min={0.01} step={0.01} style={{ width: '100%' }} />
                      </Form.Item>
                    </Form>
                  </ProCard>
                  <ProCard title="模型目录">
                    <Descriptions
                      column={{ xs: 1, md: 2, xl: 3 }}
                      items={[
                        { key: 'state', label: '目录状态', children: catalog?.state || '-' },
                        { key: 'revision', label: '目录 Revision', children: catalog?.revision || '-' },
                        { key: 'fetchedAt', label: '最近刷新', children: catalog?.fetchedAt || '-' },
                        { key: 'stale', label: 'Stale', children: catalog?.stale ? <Tag color="orange">stale</Tag> : <Tag color="green">fresh</Tag> },
                        { key: 'apiKey', label: 'API 密钥', children: configuration?.llm.aiProxy?.apiKeyConfigured ? <Tag color="green">已配置</Tag> : <Tag>未配置</Tag> },
                        { key: 'error', label: '刷新错误', children: catalog?.errorMessage || '-' }
                      ]}
                    />
                    <Table
                      style={{ marginTop: 16 }}
                      size="small"
                      rowKey={(row) => `${row.providerId}/${row.modelId}`}
                      pagination={{ pageSize: 10 }}
                      dataSource={catalog?.models ?? []}
                      columns={catalogColumns}
                      locale={{ emptyText: <Empty description="暂无模型，请点击刷新模型" /> }}
                    />
                  </ProCard>
                  {/* obsolete custom-provider editor removed; ai-proxy is the sole LLM provider.
                    title="高级：自定义 Provider"
                    extra={(
                      <Space>
                        <Button onClick={() => setShowAdvancedLLM((value) => !value)}>
                          {showAdvancedLLM ? '收起高级配置' : '展开高级配置'}
                        </Button>
                        <Button onClick={addCustomProvider}>新增 Provider</Button>
                      </Space>
                    )}
                  >
                    <Typography.Paragraph type="secondary">
                      普通模式只使用 ai-proxy 目录中的 ready 模型。自定义 Provider 为显式高级能力，需声明精确 context/output/operations；API 密钥留空表示保留已有密钥。
                    </Typography.Paragraph>
                    {showAdvancedLLM ? (
                      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                        <Space direction="vertical" size="small" className="llm-provider-list" style={{ width: '100%' }}>
                          {customProviderDrafts.length === 0 ? (
                            <Empty description="尚未配置自定义 Provider" />
                          ) : customProviderDrafts.map((item, index) => {
                            const selected = index === selectedCustomProviderIndex;
                            return (
                              <button
                                key={`${item.id || 'custom-provider'}-${index}`}
                                type="button"
                                className={`llm-provider-list-item${selected ? ' llm-provider-list-item-selected' : ''}`}
                                onClick={() => selectCustomProvider(index)}
                              >
                                <span className="llm-provider-list-main">
                                  <Typography.Text strong>{item.id || '新 Provider'}</Typography.Text>
                                  <Typography.Text type="secondary">{item.baseUrl || item.provider || 'endpoint 未配置'}</Typography.Text>
                                </span>
                                <span className="llm-provider-list-tags">
                                  <Tag>{(item.models ?? []).length} models</Tag>
                                  {configuration?.llm.customProviders?.find((provider) => provider.id === item.id)?.apiKeyConfigured ? <Tag color="green">密钥</Tag> : null}
                                </span>
                              </button>
                            );
                          })}
                        </Space>
                        {selectedCustomProvider ? (
                          <ProCard
                            title={`编辑 Provider：${selectedCustomProvider.id || '新 Provider'}`}
                            extra={<Button danger onClick={removeSelectedCustomProvider}>移除 Provider</Button>}
                          >
                            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                              <Form layout="vertical">
                                <Form.Item label="Provider 标识" required>
                                  <Input
                                    value={selectedCustomProvider.id}
                                    onChange={(event) => updateSelectedCustomProvider({ id: event.target.value })}
                                    placeholder="openai-direct"
                                  />
                                </Form.Item>
                                <Form.Item label="Provider 类型" required>
                                  <Select
                                    value={selectedCustomProvider.provider}
                                    onChange={(value) => updateSelectedCustomProvider({ provider: value })}
                                    options={[
                                      { value: 'openai-compatible', label: 'openai-compatible' },
                                      { value: 'anthropic', label: 'anthropic' },
                                      { value: 'custom', label: 'custom' }
                                    ]}
                                  />
                                </Form.Item>
                                <Form.Item label="API Surface">
                                  <Select
                                    value={selectedCustomProvider.apiSurface || 'openai_compatible_http'}
                                    onChange={(value) => updateSelectedCustomProvider({ apiSurface: value })}
                                    options={[
                                      { value: 'openai_compatible_http', label: 'openai_compatible_http' },
                                      { value: 'anthropic_messages_http', label: 'anthropic_messages_http' }
                                    ]}
                                  />
                                </Form.Item>
                                <Form.Item label="BaseURL" required>
                                  <Input
                                    value={selectedCustomProvider.baseUrl}
                                    onChange={(event) => updateSelectedCustomProvider({ baseUrl: event.target.value })}
                                    placeholder="https://api.openai.com/v1"
                                  />
                                </Form.Item>
                                <Form.Item label="API 密钥">
                                  <Input.Password
                                    value={selectedCustomProvider.apiKey}
                                    onChange={(event) => updateSelectedCustomProvider({ apiKey: event.target.value })}
                                    placeholder={configuration?.llm.customProviders?.find((provider) => provider.id === selectedCustomProvider.id)?.apiKeyConfigured ? '已配置，留空表示保留' : '可选'}
                                    autoComplete="new-password"
                                    disabled={Boolean(selectedCustomProvider.clearApiKey)}
                                  />
                                </Form.Item>
                                <Form.Item>
                                  <Checkbox
                                    checked={Boolean(selectedCustomProvider.clearApiKey)}
                                    onChange={(event) => updateSelectedCustomProvider({
                                      clearApiKey: event.target.checked,
                                      apiKey: event.target.checked ? '' : selectedCustomProvider.apiKey
                                    })}
                                  >
                                    显式清空此 Provider 的 API 密钥
                                  </Checkbox>
                                </Form.Item>
                                <Form.Item label="超时">
                                  <Input
                                    value={selectedCustomProvider.timeout}
                                    onChange={(event) => updateSelectedCustomProvider({ timeout: event.target.value })}
                                    placeholder="60s"
                                  />
                                </Form.Item>
                              </Form>
                              <ProCard
                                title="模型声明"
                                extra={(
                                  <Space>
                                    <Button onClick={addCustomModel}>新增模型</Button>
                                    <Button danger onClick={removeSelectedCustomModel} disabled={(selectedCustomProvider.models?.length ?? 0) <= 1}>移除模型</Button>
                                  </Space>
                                )}
                              >
                                <Space wrap>
                                  {(selectedCustomProvider.models ?? []).map((model, modelIndex) => (
                                    <Button
                                      key={`${model.id || 'model'}-${modelIndex}`}
                                      type={modelIndex === selectedCustomModelIndex ? 'primary' : 'default'}
                                      onClick={() => setSelectedCustomModelIndex(modelIndex)}
                                    >
                                      {model.id || `模型 ${modelIndex + 1}`}
                                    </Button>
                                  ))}
                                </Space>
                                {selectedCustomModel ? (
                                  <Form layout="vertical" style={{ marginTop: 16 }}>
                                    <Form.Item label="模型 ID" required>
                                      <Input
                                        value={selectedCustomModel.id}
                                        onChange={(event) => updateSelectedCustomModel({ id: event.target.value })}
                                        placeholder="gpt-4o"
                                      />
                                    </Form.Item>
                                    <Form.Item label="Context Window Tokens" required>
                                      <InputNumber
                                        min={1}
                                        style={{ width: '100%' }}
                                        value={selectedCustomModel.contextWindowTokens}
                                        onChange={(value) => updateSelectedCustomModel({ contextWindowTokens: typeof value === 'number' ? value : undefined })}
                                      />
                                    </Form.Item>
                                    <Form.Item label="Max Output Tokens" required>
                                      <InputNumber
                                        min={1}
                                        style={{ width: '100%' }}
                                        value={selectedCustomModel.maxOutputTokens}
                                        onChange={(value) => updateSelectedCustomModel({ maxOutputTokens: typeof value === 'number' ? value : undefined })}
                                      />
                                    </Form.Item>
                                    <Form.Item label="Operations" required>
                                      <Select
                                        mode="multiple"
                                        value={selectedCustomModel.operations}
                                        onChange={(value) => updateSelectedCustomModel({ operations: value })}
                                        options={[
                                          { value: 'chat_completions', label: 'chat_completions' },
                                          { value: 'embeddings', label: 'embeddings' }
                                        ]}
                                      />
                                    </Form.Item>
                                    <Form.Item label="Temperature">
                                      <InputNumber
                                        min={0}
                                        max={2}
                                        step={0.1}
                                        style={{ width: '100%' }}
                                        value={selectedCustomModel.temperature}
                                        onChange={(value) => updateSelectedCustomModel({ temperature: typeof value === 'number' ? value : undefined })}
                                      />
                                    </Form.Item>
                                    <Form.Item label="Reasoning Effort">
                                      <Select
                                        allowClear
                                        value={selectedCustomModel.reasoningEffort || undefined}
                                        onChange={(value) => updateSelectedCustomModel({ reasoningEffort: value })}
                                        options={[
                                          { value: 'low', label: 'low' },
                                          { value: 'medium', label: 'medium' },
                                          { value: 'high', label: 'high' }
                                        ]}
                                      />
                                    </Form.Item>
                                    <Form.Item label="Thinking Budget Tokens">
                                      <InputNumber
                                        min={0}
                                        style={{ width: '100%' }}
                                        value={selectedCustomModel.thinkingBudgetTokens}
                                        onChange={(value) => updateSelectedCustomModel({ thinkingBudgetTokens: typeof value === 'number' ? value : undefined })}
                                      />
                                    </Form.Item>
                                    <Form.Item label="Cache Mode">
                                      <Select
                                        allowClear
                                        value={selectedCustomModel.cacheMode || undefined}
                                        onChange={(value) => updateSelectedCustomModel({ cacheMode: value })}
                                        options={[
                                          { value: 'none', label: 'none' },
                                          { value: 'prompt_cache', label: 'prompt_cache' },
                                          { value: 'message_cache_control', label: 'message_cache_control' }
                                        ]}
                                      />
                                    </Form.Item>
                                    <Form.Item label="Cache Usage Fields">
                                      <Select
                                        mode="tags"
                                        value={selectedCustomModel.cacheUsageFields}
                                        onChange={(value) => updateSelectedCustomModel({ cacheUsageFields: value })}
                                        placeholder="cache_read_input_tokens"
                                      />
                                    </Form.Item>
                                  </Form>
                                ) : <Empty description="请新增模型声明" />}
                              </ProCard>
                            </Space>
                          </ProCard>
                        ) : null}
                      </Space>
                    ) : (
                      <Typography.Text type="secondary">
                        已配置 {customProviderDrafts.length} 个自定义 Provider。展开后可编辑 model/operation/inference/cache。
                      </Typography.Text>
                    )}
                  */}
                </Space>
              </ProCard>
            )
          },
          {
            key: 'web-search',
            label: 'Web Search',
            children: (
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
                        <Form.Item
                          name="headers"
                          label="请求头"
                          rules={[
                            {
                              validator: async (_, value?: string) => {
                                validateHeaderLines(value);
                              }
                            }
                          ]}
                        >
                          <Input.TextArea rows={3} placeholder={'Authorization: Bearer token\nX-Request-Source: workorch'} />
                        </Form.Item>
                        <Form.Item name="timeout" label="超时">
                          <Input placeholder="20s" />
                        </Form.Item>
                        <Form.Item name="maxResults" label="最大搜索结果">
                          <InputNumber min={1} max={20} style={{ width: '100%' }} />
                        </Form.Item>
                        <Form.Item name="maxFetchBytes" label="最大读取字节数">
                          <InputNumber min={1024} style={{ width: '100%' }} />
                        </Form.Item>
                        <Form.Item name="maxRedirects" label="最大重定向次数">
                          <InputNumber min={0} max={10} style={{ width: '100%' }} />
                        </Form.Item>
                        <Form.Item name="allowDomains" label="允许域名">
                          <Select mode="tags" placeholder="example.test" />
                        </Form.Item>
                        <Form.Item name="denyDomains" label="拒绝域名">
                          <Select mode="tags" placeholder="blocked.example.test" />
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
            )
          },
          {
            key: 'soul',
            label: '执行人格',
            children: <SoulDefinitionManager souls={souls} onSoulsChange={setSouls} />
          },
          {
            key: 'workspace-template',
            label: '工作空间模板',
            children: <WorkspaceTemplateManager souls={souls} templates={workspaceTemplates} onTemplatesChange={setWorkspaceTemplates} />
          }
        ]}
      />
    </PageContainer>
  );
}
