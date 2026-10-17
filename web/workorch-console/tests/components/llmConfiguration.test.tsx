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
].map(modelId => ({ providerId: 'ai-proxy', modelId, operations: ['embeddings'], capabilityReady: false, unavailableReason: 'capability_incomplete' }));

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

  it('endpoint-only Embedding 仍可见、禁用并解释单条输入容量，不进入执行候选', () => {
    const cfg = configuration();
    const options = llmModelOptions(cfg, 'embeddings', undefined);
    expect(options).toHaveLength(2);
    for (const option of options) {
      expect(option).toMatchObject({ source: 'catalog', ready: false, disabled: true });
      expect(option.label).toContain('contextWindowTokens（单条输入 token 上限）');
      expect(option.label).not.toContain('maxOutputTokens');
    }
    expect(readyCatalogTargetOptions(cfg, 'embeddings')).toEqual([]);
    expect(chatTargetOptions(cfg).map(item => item.modelId)).toEqual(['chat']);
    expect(activationReadyChatTarget(cfg)).toEqual({ providerId: 'ai-proxy', modelId: 'chat' });
    expect(cfg.llm.defaultEmbedding).toBeUndefined();
  });

  it('owner ready 的 Embedding 不要求输出容量；owner 拒绝仍不可选', () => {
    const cfg = configuration([
      { ...endpointOnlyModels[0], contextWindowTokens: 512, capabilityReady: true, unavailableReason: undefined },
      { ...endpointOnlyModels[1], contextWindowTokens: 8192, unavailableReason: 'owner_unavailable' }
    ]);
    const options = llmModelOptions(cfg, 'embeddings', undefined);
    expect(options[0]).toMatchObject({ ready: true, label: endpointOnlyModels[0].modelId });
    expect(options[1]).toMatchObject({ ready: false, disabled: true });
    expect(options[1].label).toContain('owner_unavailable');
    expect(readyCatalogTargetOptions(cfg, 'embeddings').map(item => item.modelId)).toEqual([endpointOnlyModels[0].modelId]);
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

  it('真实配置表单显示禁用原因，保存不自动设置默认 Embedding', async () => {
    render(<StyleProvider container={styleContainer}><ConfigProvider theme={{ hashed: false, token: { motion: false } }} wave={{ disabled: true }}><AntApp><ConfigurationPage /></AntApp></ConfigProvider></StyleProvider>);
    await act(async () => {});
    await act(async () => { fireEvent.click(screen.getByRole('tab', { name: 'LLM' })); });
    expect(screen.getByText(/已枚举不等于可调用/)).toBeTruthy();
    const select = screen.getByLabelText('默认 Embedding 模型');
    await act(async () => { fireEvent.mouseDown(select.closest('.ant-select')!.querySelector('.ant-select-selector')!); });
    for (const model of endpointOnlyModels) {
      const label = screen.getByText(`${model.modelId}（不可选：缺少有效 contextWindowTokens（单条输入 token 上限））`, { selector: '.ant-select-item-option-content' });
      expect(label.closest('.ant-select-item-option')?.classList.contains('ant-select-item-option-disabled')).toBe(true);
    }
    expect(workorchApi.updateLLMConfiguration).not.toHaveBeenCalled();
    await act(async () => { fireEvent.click(screen.getByRole('button', { name: '保存 LLM 配置' })); });
    expect(workorchApi.updateLLMConfiguration).toHaveBeenCalledWith(expect.objectContaining({
      defaultChatModelId: 'chat', defaultEmbeddingModelId: undefined, clearDefaultEmbedding: true
    }));
  });
});
