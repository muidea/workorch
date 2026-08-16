import { useEffect, useMemo, useState } from 'react';
import { App as AntApp, Button, Descriptions, Empty, Form, Input, InputNumber, Select, Space, Tabs, Tag, Typography } from 'antd';
import { CardGrid, PageContainer, ProCard } from '../components/proCompat';
import type {
  ServiceConfiguration,
  Soul,
  UpdateLLMConfigurationRequest,
  UpdateServerConfigurationRequest,
  UpdateWebSearchProviderConfiguration,
  WorkspaceTemplate
} from '../types';
import { workorchApi } from '../api';
import {
  llmModelOptions,
  errorMessage,
  llmConfigurationFormValues,
  nextWebSearchProviderDraft,
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
    llmForm.resetFields();
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
      const defaultChatModelId = values.defaultChatModelId?.trim();
      const currentChatModelId = configuration?.llm.defaultChat?.modelId;
      // A credential rotation must remain possible when the old key has
      // already expired and the catalog therefore contains no ready target.
      // A changed model still has to be ready; the configured model is
      // allowed so its credential can be repaired first.
      if (!defaultChatModelId || (!chatModelOptions.some((item) => item.ready && item.value === defaultChatModelId) && defaultChatModelId !== currentChatModelId)) {
        message.error('请选择可用的默认对话模型');
        return;
      }
      const defaultEmbeddingModelId = values.defaultEmbeddingModelId?.trim();
      const currentEmbeddingModelId = configuration?.llm.defaultEmbedding?.modelId;
      if (defaultEmbeddingModelId && (
        !embeddingModelOptions.some((item) => item.ready && item.value === defaultEmbeddingModelId) && defaultEmbeddingModelId !== currentEmbeddingModelId
      )) {
        message.error('请选择可用的默认 Embedding 模型');
        return;
      }
      const payload: UpdateLLMConfigurationRequest = {
        apiKey: values.apiKey?.trim() || undefined,
        defaultChatModelId,
        defaultEmbeddingModelId: defaultEmbeddingModelId || undefined,
        clearDefaultEmbedding: !defaultEmbeddingModelId,
        budget: values.budget
      };
      const nextConfiguration = await workorchApi.updateLLMConfiguration(payload);
      applyConfiguration(nextConfiguration);
      message.success('LLM 配置已保存，重启 WorkOrch 后生效');
    } catch (error) {
      message.error(errorMessage(error));
    } finally {
      setSaving(false);
    }
  };

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
  const chatModelOptions = useMemo(
    () => llmModelOptions(configuration, 'responses', configuration?.llm.defaultChat?.modelId),
    [configuration]
  );
  const embeddingModelOptions = useMemo(
    () => llmModelOptions(configuration, 'embeddings', configuration?.llm.defaultEmbedding?.modelId),
    [configuration]
  );

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

  return (
    <PageContainer
      title="系统配置"
      subTitle="管理服务配置、执行人格（Soul）、工作空间模板（Workspace Template）、LLM 与 Web Search Provider"
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
                <ProCard title="候选文档恢复" colSpan="100%">
                  <Descriptions
                    column={1}
                    items={[
                      { key: 'maxDocumentRecoveryAttempts', label: '候选完整文档恢复次数', children: configNumberText(llmOutput?.maxDocumentRecoveryAttempts) }
                    ]}
                  />
                </ProCard>
              </CardGrid>
            )
          },
          {
            key: 'llm',
            label: 'LLM',
            children: (
              <ProCard
                title="LLM"
                className="workorch-section"
                extra={<Button type="primary" onClick={() => void llmForm.validateFields().then((values) => saveLLMConfiguration(values))} loading={saving}>保存 LLM 配置</Button>}
              >
                <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                  <ProCard title="模型与预算">
                    <Form form={llmForm} layout="vertical" onFinish={(values) => void saveLLMConfiguration(values)}>
                      <Form.Item name="apiKey" label="API 密钥">
                        <Input.Password placeholder="留空表示保留已有密钥" autoComplete="new-password" />
                      </Form.Item>
                      <Form.Item
                        name="defaultChatModelId"
                        label="默认对话模型"
                        rules={[
                          { required: true, message: '请选择默认对话模型' },
                          {
                            validator: (_, value?: string) => value === configuration?.llm.defaultChat?.modelId || chatModelOptions.some((item) => item.ready && item.value === value)
                              ? Promise.resolve()
                              : Promise.reject(new Error('已配置模型当前不可用，请选择可用模型'))
                          }
                        ]}
                      >
                        <Select
                          showSearch
                          placeholder={chatModelOptions.some((item) => item.ready) ? '选择可用模型' : '当前没有可用模型'}
                          options={chatModelOptions}
                        />
                      </Form.Item>
                      <Form.Item
                        name="defaultEmbeddingModelId"
                        label="默认 Embedding 模型"
                        rules={[
                          {
                            validator: (_, value?: string) => !value || value === configuration?.llm.defaultEmbedding?.modelId || embeddingModelOptions.some((item) => item.ready && item.value === value)
                              ? Promise.resolve()
                              : Promise.reject(new Error('已配置模型当前不可用，请选择可用模型或清空'))
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
