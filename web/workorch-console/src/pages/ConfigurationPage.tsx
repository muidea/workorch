import { useEffect, useState } from 'react';
import { App as AntApp, Button, Descriptions, Empty, Form, Input, InputNumber, Select, Space, Tabs, Tag, Typography } from 'antd';
import { PageContainer, ProCard } from '@ant-design/pro-components';
import type {
  LLMModelCapabilityConfiguration,
  ServiceConfiguration,
  Soul,
  UpdateLLMProviderConfiguration,
  UpdateServerConfigurationRequest,
  UpdateWebSearchProviderConfiguration,
  WorkspaceTemplate
} from '../types';
import { workorchApi } from '../api';
import {
  cleanedContextProfile,
  cleanedInferenceProfile,
  errorMessage,
  llmConfigurationFormValues,
  nextModelCapabilityDraft,
  nextProviderDraft,
  nextWebSearchProviderDraft,
  providerDraftFromTemplate,
  SoulDefinitionManager,
  splitLines,
  webSearchConfigurationFormValues,
  webSearchProviderDraftFromTemplate,
  webSearchProviderFormValues,
  WorkspaceTemplateManager,
  type WebSearchProviderFormValues
} from '../helpers';

export function ConfigurationPage() {
  const { message } = AntApp.useApp();
  const [serverForm] = Form.useForm<UpdateServerConfigurationRequest>();
  const [providerEditorForm] = Form.useForm<UpdateLLMProviderConfiguration>();
  const [webSearchProviderEditorForm] = Form.useForm<WebSearchProviderFormValues>();
  const [configuration, setConfiguration] = useState<ServiceConfiguration | undefined>(undefined);
  const [souls, setSouls] = useState<Soul[]>([]);
  const [workspaceTemplates, setWorkspaceTemplates] = useState<WorkspaceTemplate[]>([]);
  const [defaultProviderId, setDefaultProviderId] = useState<string | undefined>(undefined);
  const [defaultEmbeddingProviderId, setDefaultEmbeddingProviderId] = useState<string | undefined>(undefined);
  const [modelCapabilityDrafts, setModelCapabilityDrafts] = useState<LLMModelCapabilityConfiguration[]>([]);
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
      setModelCapabilityDrafts(nextFormValues.modelCapabilities ?? []);
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
        defaultEmbeddingProviderId: defaultEmbeddingProviderId?.trim() || '',
        modelCapabilities: modelCapabilityDrafts.map((item) => ({
          id: item.id.trim(),
          provider: item.provider?.trim() || undefined,
          modelPattern: item.modelPattern.trim(),
          contextWindowTokens: item.contextWindowTokens
        })),
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
          nonChatOperations: item.nonChatOperations ?? [],
          failoverEnabled: item.failoverEnabled === true,
          failoverProviders: item.failoverProviders ?? [],
          inferenceProfile: cleanedInferenceProfile(item),
          contextProfile: cleanedContextProfile(item)
        }))
      });
      const nextFormValues = llmConfigurationFormValues(nextConfiguration);
      setConfiguration(nextConfiguration);
      setDefaultProviderId(nextFormValues.defaultProviderId);
      setDefaultEmbeddingProviderId(nextFormValues.defaultEmbeddingProviderId);
      setModelCapabilityDrafts(nextFormValues.modelCapabilities ?? []);
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

  const updateModelCapabilityDraft = (index: number, patch: Partial<LLMModelCapabilityConfiguration>) => {
    setModelCapabilityDrafts((items) => {
      const nextItems = [...items];
      nextItems[index] = {
        ...nextItems[index],
        ...patch
      };
      return nextItems;
    });
  };

  const addModelCapability = () => {
    setModelCapabilityDrafts((items) => [...items, nextModelCapabilityDraft(items)]);
  };

  const removeModelCapability = (index: number) => {
    setModelCapabilityDrafts((items) => items.filter((_, itemIndex) => itemIndex !== index));
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
      <Tabs
        className="configuration-tabs"
        destroyOnHidden={false}
        items={[
          {
            key: 'service',
            label: '基础服务',
            children: (
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
          },
          {
            key: 'llm',
            label: 'LLM 服务商',
            children: (
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
            title="模型能力表"
            extra={<Button onClick={addModelCapability}>新增能力</Button>}
          >
            <Space direction="vertical" size="small" style={{ width: '100%' }}>
              {modelCapabilityDrafts.length ? modelCapabilityDrafts.map((item, index) => (
                <Space key={`${item.id || 'model-capability'}-${index}`} wrap align="start" style={{ width: '100%' }}>
                  <Input
                    style={{ width: 220 }}
                    value={item.id}
                    onChange={(event) => updateModelCapabilityDraft(index, { id: event.target.value })}
                    placeholder="能力标识"
                  />
                  <Select
                    allowClear
                    style={{ width: 180 }}
                    value={item.provider || undefined}
                    onChange={(value) => updateModelCapabilityDraft(index, { provider: value })}
                    placeholder="Provider 类型"
                    options={Array.from(new Set(providerTemplates.map((template) => template.provider).filter(Boolean))).map((provider) => ({ value: provider, label: provider }))}
                  />
                  <Input
                    style={{ width: 260 }}
                    value={item.modelPattern}
                    onChange={(event) => updateModelCapabilityDraft(index, { modelPattern: event.target.value })}
                    placeholder="模型匹配表达式"
                  />
                  <InputNumber
                    style={{ width: 180 }}
                    min={1}
                    value={item.contextWindowTokens}
                    onChange={(value) => updateModelCapabilityDraft(index, { contextWindowTokens: Number(value ?? 0) })}
                    placeholder="上下文 Tokens"
                  />
                  <Button danger onClick={() => removeModelCapability(index)}>移除</Button>
                </Space>
              )) : (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无模型能力配置" />
              )}
            </Space>
          </ProCard>
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
                <Form.Item name="failoverEnabled" label="故障转移">
                  <Select
                    options={[
                      { value: false, label: '禁用' },
                      { value: true, label: '启用' }
                    ]}
                  />
                </Form.Item>
                <Form.Item name="failoverProviders" label="故障转移服务商">
                  <Select mode="tags" placeholder="按顺序填写备用 provider id" options={providerDrafts.map((item) => ({ value: item.id, label: item.id }))} />
                </Form.Item>
                <Form.Item name={['inferenceProfile', 'temperature']} label="Temperature">
                  <InputNumber min={0} max={2} step={0.1} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name={['inferenceProfile', 'maxOutputTokens']} label="最大输出 Tokens">
                  <InputNumber min={0} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name={['inferenceProfile', 'reasoningEffort']} label="推理强度">
                  <Select
                    allowClear
                    options={[
                      { value: 'low', label: 'low' },
                      { value: 'medium', label: 'medium' },
                      { value: 'high', label: 'high' },
                      { value: 'xhigh', label: 'xhigh' }
                    ]}
                  />
                </Form.Item>
                <Form.Item name={['inferenceProfile', 'thinkingBudgetTokens']} label="Thinking Budget Tokens">
                  <InputNumber min={0} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name={['contextProfile', 'profileMode']} label="上下文配置模式">
                  <Select
                    allowClear
                    options={[
                      { value: 'auto', label: 'auto' },
                      { value: 'manual', label: 'manual' }
                    ]}
                  />
                </Form.Item>
                <Form.Item name={['contextProfile', 'contextWindowTokens']} label="上下文窗口 Tokens">
                  <InputNumber min={0} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item noStyle shouldUpdate={(previous, current) => previous.contextProfile?.profileMode !== current.contextProfile?.profileMode}>
                  {({ getFieldValue }) => getFieldValue(['contextProfile', 'profileMode']) === 'manual' ? (
                    <>
                      <Form.Item name={['contextProfile', 'maxInputTokens']} label="最大输入 Tokens">
                        <InputNumber min={0} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['contextProfile', 'reservedOutputTokens']} label="保留输出 Tokens">
                        <InputNumber min={0} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['contextProfile', 'safetyRatio']} label="安全比例">
                        <InputNumber min={0} max={1} step={0.05} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['contextProfile', 'contextUtilizationRatio']} label="上下文利用比例">
                        <InputNumber min={0} max={1} step={0.05} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['contextProfile', 'bytesHardLimit']} label="请求字节硬限制">
                        <InputNumber min={0} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['contextProfile', 'maxRequestBytes']} label="最大请求字节数">
                        <InputNumber min={0} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['contextProfile', 'estimatedTokensPerByte']} label="Token/Byte 估算">
                        <InputNumber min={0} step={0.01} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['contextProfile', 'inputTokenPrice']} label="输入 Token 单价">
                        <InputNumber min={0} step={0.000001} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['contextProfile', 'outputTokenPrice']} label="输出 Token 单价">
                        <InputNumber min={0} step={0.000001} style={{ width: '100%' }} />
                      </Form.Item>
                      <Form.Item name={['contextProfile', 'currency']} label="计费币种">
                        <Input placeholder="USD" />
                      </Form.Item>
                    </>
                  ) : null}
                </Form.Item>
                <Form.Item name="apiKey" label="API 密钥">
                  <Input.Password placeholder="留空表示保留已有 API 密钥" autoComplete="new-password" />
                </Form.Item>
                <Descriptions
                  column={1}
                  items={[
                    { key: 'apiKey', label: '已保存 API 密钥', children: selectedProviderStatus?.apiKeyConfigured ? <Tag color="green">已配置</Tag> : <Tag color="red">缺失</Tag> },
                    { key: 'status', label: '已保存状态', children: selectedProviderStatus?.ready ? <Tag color="green">就绪</Tag> : <Tag color="red">{selectedProviderStatus?.reason || '未保存'}</Tag> },
                    { key: 'capabilities', label: '已保存能力', children: <Space wrap>{selectedProviderStatus?.chatCompletions ? <Tag color="blue">chat</Tag> : null}{selectedProviderStatus?.streaming ? <Tag color="cyan">streaming</Tag> : null}{(selectedProviderStatus?.nonChatOperations ?? []).map((item) => <Tag key={item}>{item}</Tag>)}</Space> },
                    { key: 'contextProfileMode', label: '已保存上下文模式', children: selectedProviderStatus?.contextProfile?.profileMode || '-' },
                    { key: 'contextWindowTokens', label: '已保存上下文窗口', children: selectedProviderStatus?.contextProfile?.contextWindowTokens || '-' },
                    { key: 'contextCapabilitySource', label: '已保存能力来源', children: selectedProviderStatus?.contextProfile?.capabilitySource || '-' }
                  ]}
                />
              </Form>
            ) : (
              <Empty description="请选择或新增 LLM 服务商" />
            )}
          </ProCard>
        </Space>
              </ProCard>
            )
          }
        ]}
      />
    </PageContainer>
  );
}
