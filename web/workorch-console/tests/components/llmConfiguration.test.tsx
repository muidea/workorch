import { App as AntApp, ConfigProvider } from 'antd';
import { StyleProvider } from '@ant-design/cssinjs';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { workorchApi } from '../../src/api';
import { activationReadyChatTarget, chatTargetOptions, llmModelOptions, readyCatalogTargetOptions } from '../../src/helpers';
import { ConfigurationPage } from '../../src/pages/ConfigurationPage';
import type { ProviderModelDescriptorView, ServiceConfiguration } from '../../src/types';

vi.mock('../../src/api', () => ({ workorchApi: {
  readConfiguration: vi.fn(), listSouls: vi.fn(), listWorkspaceTemplates: vi.fn(), updateLLMConfiguration: vi.fn()
} }));

// jsdom 不检验布局；隔离样式容器，保留真实表单与 Select 的行为。
const styleContainer = document.createElement('div');

const endpointOnlyModels: ProviderModelDescriptorView[] = [
  'intfloat/multilingual-e5-small', 'jina-embeddings-v2-base-code'
].map(modelId => ({ providerId: 'ai-proxy', modelId, operations: ['embeddings'], capabilityReady: true, embeddingReady: true }));

function configuration(models = endpointOnlyModels): ServiceConfiguration {
  return {
    server: {}, repository: {}, runtime: {}, webSearch: {},
    governance: {
      autoTicket: { enabled: false, minConfidence: 1, maxPerRun: 1 },
      autoApproval: { enabled: false, minConfidence: 1, maxPerRun: 1, allowReadOnly: false, allowWorkspaceWrite: false },
      budget: { maxConsecutiveFailures: 1, maxRepeatedSamePrompt: 1 }
    },
    llm: {
      defaultChat: { providerId: 'ai-proxy', modelId: 'chat' },
      budget: { contextUtilizationRatio: 0.75, estimatedTokensPerByte: 0.25 }, restartRequired: false,
      catalog: { providerId: 'ai-proxy', state: 'fresh', models: [
        { providerId: 'ai-proxy', modelId: 'chat', operations: ['responses'], capabilityReady: true, contextWindowTokens: 8192, maxOutputTokens: 1024 },
        ...models
      ] }
    }
  };
}

describe('LLM 配置目录发现与调用资格', () => {
  beforeEach(() => {
    vi.mocked(workorchApi.readConfiguration).mockResolvedValue(configuration());
    vi.mocked(workorchApi.listSouls).mockResolvedValue([]);
    vi.mocked(workorchApi.listWorkspaceTemplates).mockResolvedValue([]);
    vi.mocked(workorchApi.updateLLMConfiguration).mockClear().mockResolvedValue(configuration());
  });

  it('保存的模型与运行进程模型分开展示，不能用保存选择冒充生效事实', async () => {
    const cfg = configuration();
    cfg.llm.defaultChat.modelId = 'saved-chat';
    cfg.llm.restartRequired = true;
    cfg.llm.active = { defaultChat: { providerId: 'ai-proxy', modelId: 'active-chat' } };
    vi.mocked(workorchApi.readConfiguration).mockResolvedValue(cfg);
    render(<StyleProvider container={styleContainer}><ConfigProvider theme={{ hashed: false, token: { motion: false } }} wave={{ disabled: true }}><AntApp><ConfigurationPage /></AntApp></ConfigProvider></StyleProvider>);
    await act(async () => {});
    fireEvent.click(screen.getByRole('tab', { name: 'LLM' }));
    expect(await screen.findByText('ai-proxy/active-chat')).toBeTruthy();
    expect(screen.getByText('保存的 LLM 配置尚未确认生效，需要重启 WorkOrch。')).toBeTruthy();
    expect(screen.queryByText('ai-proxy/saved-chat')).toBeNull();
    expect(workorchApi.updateLLMConfiguration).not.toHaveBeenCalled();
  });

  it('未取得运行配置时不回填保存的默认模型', async () => {
    render(<StyleProvider container={styleContainer}><ConfigProvider theme={{ hashed: false, token: { motion: false } }} wave={{ disabled: true }}><AntApp><ConfigurationPage /></AntApp></ConfigProvider></StyleProvider>);
    await act(async () => {});
    fireEvent.click(screen.getByRole('tab', { name: 'LLM' }));
    expect(await screen.findByText('未取得运行配置，当前模型未知')).toBeTruthy();
    expect(screen.queryByText('ai-proxy/chat')).toBeNull();
  });

  it('owner 接纳 endpoint-only Embedding，容量保持未知，不进入对话候选或自动设为默认', () => {
    const cfg = configuration();
    const options = llmModelOptions(cfg, 'embeddings', undefined);
    expect(options).toHaveLength(2);
    for (const option of options) {
      expect(option).toMatchObject({ source: 'catalog', ready: true });
      expect(option.disabled).not.toBe(true);
    }
    expect(readyCatalogTargetOptions(cfg, 'embeddings').map(item => item.modelId)).toEqual(endpointOnlyModels.map(item => item.modelId));
    expect(cfg.llm.catalog?.models?.slice(1).every(item => item.contextWindowTokens === undefined)).toBe(true);
    expect(chatTargetOptions(cfg).map(item => item.modelId)).toEqual(['chat']);
    expect(activationReadyChatTarget(cfg)).toEqual({ providerId: 'ai-proxy', modelId: 'chat' });
    expect(cfg.llm.defaultEmbedding).toBeUndefined();
  });

  it('owner ready 的 Embedding 不要求输出容量；owner 拒绝仍不可选', () => {
    const cfg = configuration([
      { ...endpointOnlyModels[0], contextWindowTokens: 512, capabilityReady: true, unavailableReason: undefined },
      { ...endpointOnlyModels[1], contextWindowTokens: 8192, capabilityReady: false, embeddingReady: false, unavailableReason: 'owner_unavailable' }
    ]);
    const options = llmModelOptions(cfg, 'embeddings', undefined);
    expect(options[0]).toMatchObject({ ready: true, label: endpointOnlyModels[0].modelId });
    expect(options[1]).toMatchObject({ ready: false, disabled: true });
    expect(options[1].label).toContain('owner_unavailable');
    expect(readyCatalogTargetOptions(cfg, 'embeddings').map(item => item.modelId)).toEqual([endpointOnlyModels[0].modelId]);
  });

  it('混合模型仅 Embedding ready 时不提升 Responses 资格', () => {
    const cfg = configuration([{ ...endpointOnlyModels[0], operations: ['responses', 'embeddings'], capabilityReady: false, embeddingReady: true, unavailableReason: 'capability_incomplete' }]);
    expect(readyCatalogTargetOptions(cfg, 'embeddings').map(item => item.modelId)).toEqual([endpointOnlyModels[0].modelId]);
    expect(chatTargetOptions(cfg).map(item => item.modelId)).toEqual(['chat']);
    expect(llmModelOptions(cfg, 'responses', undefined)[1]).toMatchObject({ disabled: true, ready: false });
  });

  it('未 ready 的 Responses 同样可见但不能泄漏到激活，且过滤其他 provider/operation', () => {
    const cfg = configuration([
      { providerId: 'ai-proxy', modelId: 'bad-chat', operations: ['responses'], capabilityReady: false, contextWindowTokens: 8192, unavailableReason: 'capability_incomplete' },
      { ...endpointOnlyModels[0], providerId: 'other' },
      { ...endpointOnlyModels[1], operations: [] }
    ]);
    expect(llmModelOptions(cfg, 'embeddings', undefined)).toEqual([]);
    expect(llmModelOptions(cfg, 'responses', undefined)[1].label).toContain('maxOutputTokens');
    expect(chatTargetOptions(cfg).map(item => item.modelId)).toEqual(['chat']);
  });

  it('保留已配置但不可用的身份，目录诊断不重复且未知模型不被替代', () => {
    expect(llmModelOptions(configuration(), 'embeddings', endpointOnlyModels[0].modelId)).toHaveLength(2);
    const cfg = configuration();
    cfg.llm.catalog = undefined;
    expect(llmModelOptions(cfg, 'embeddings', 'configured-model')).toEqual([
      expect.objectContaining({ modelId: 'configured-model', source: 'configured', ready: false, disabled: true })
    ]);
  });

  it('真实配置表单可选容量未知的 Embedding，保存不自动设置默认', async () => {
    render(<StyleProvider container={styleContainer}><ConfigProvider theme={{ hashed: false, token: { motion: false } }} wave={{ disabled: true }}><AntApp><ConfigurationPage /></AntApp></ConfigProvider></StyleProvider>);
    await act(async () => {});
    await act(async () => { fireEvent.click(screen.getByRole('tab', { name: 'LLM' })); });
    expect(screen.getByText(/已枚举不等于可调用/)).toBeTruthy();
    const select = screen.getByLabelText('默认 Embedding 模型');
    await act(async () => { fireEvent.mouseDown(select.closest('.ant-select')!.querySelector('.ant-select-selector')!); });
    for (const model of endpointOnlyModels) {
      const label = screen.getByText(model.modelId, { selector: '.ant-select-item-option-content' });
      expect(label.closest('.ant-select-item-option')?.classList.contains('ant-select-item-option-disabled')).toBe(false);
    }
    expect(workorchApi.updateLLMConfiguration).not.toHaveBeenCalled();
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '保存 LLM 配置' })); });
    expect(workorchApi.updateLLMConfiguration).toHaveBeenCalledWith(expect.objectContaining({
      defaultChatModelId: 'chat', defaultEmbeddingModelId: undefined, clearDefaultEmbedding: true
    }));
  });

  it('显式选择容量未知的 Embedding 后提交精确模型身份', async () => {
    render(<StyleProvider container={styleContainer}><ConfigProvider theme={{ hashed: false, token: { motion: false } }} wave={{ disabled: true }}><AntApp><ConfigurationPage /></AntApp></ConfigProvider></StyleProvider>);
    await act(async () => {});
    await act(async () => { fireEvent.click(screen.getByRole('tab', { name: 'LLM' })); });
    const select = screen.getByLabelText('默认 Embedding 模型');
    await act(async () => { fireEvent.mouseDown(select.closest('.ant-select')!.querySelector('.ant-select-selector')!); });
    await act(async () => { fireEvent.click(screen.getByText(endpointOnlyModels[0].modelId, { selector: '.ant-select-item-option-content' })); });
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '保存 LLM 配置' })); });
    expect(workorchApi.updateLLMConfiguration).toHaveBeenCalledWith(expect.objectContaining({
      defaultEmbeddingModelId: endpointOnlyModels[0].modelId, clearDefaultEmbedding: false
    }));
  });
});
