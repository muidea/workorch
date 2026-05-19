import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import type {
  ServiceConfiguration,
  Soul,
  WorkspaceTemplate,
  LLMModelCapabilityConfiguration,
  UpdateLLMProviderConfiguration,
  UpdateWebSearchProviderConfiguration
} from '../types';
import { workorchApi } from '../api';

interface ConfigState {
  // 数据
  configuration: ServiceConfiguration | undefined;
  souls: Soul[];
  workspaceTemplates: WorkspaceTemplate[];

  // LLM 配置
  defaultProviderId: string | undefined;
  defaultEmbeddingProviderId: string | undefined;
  modelCapabilityDrafts: LLMModelCapabilityConfiguration[];
  providerDrafts: UpdateLLMProviderConfiguration[];
  selectedProviderIndex: number;
  selectedTemplateId: string | undefined;

  // Web Search 配置
  defaultWebSearchProviderId: string | undefined;
  webSearchProviderDrafts: UpdateWebSearchProviderConfiguration[];
  selectedWebSearchProviderIndex: number;
  selectedWebSearchTemplateId: string | undefined;

  // 加载和保存状态
  loading: boolean;
  serverSaving: boolean;
  saving: boolean;
  webSearchSaving: boolean;

  // Actions
  setConfiguration: (configuration: ServiceConfiguration | undefined) => void;
  setSouls: (souls: Soul[]) => void;
  setWorkspaceTemplates: (templates: WorkspaceTemplate[]) => void;
  setDefaultProviderId: (id: string | undefined) => void;
  setDefaultEmbeddingProviderId: (id: string | undefined) => void;
  setModelCapabilityDrafts: (drafts: LLMModelCapabilityConfiguration[] | ((prev: LLMModelCapabilityConfiguration[]) => LLMModelCapabilityConfiguration[])) => void;
  setProviderDrafts: (drafts: UpdateLLMProviderConfiguration[] | ((prev: UpdateLLMProviderConfiguration[]) => UpdateLLMProviderConfiguration[])) => void;
  setSelectedProviderIndex: (index: number) => void;
  setSelectedTemplateId: (id: string | undefined) => void;
  setDefaultWebSearchProviderId: (id: string | undefined) => void;
  setWebSearchProviderDrafts: (drafts: UpdateWebSearchProviderConfiguration[] | ((prev: UpdateWebSearchProviderConfiguration[]) => UpdateWebSearchProviderConfiguration[])) => void;
  setSelectedWebSearchProviderIndex: (index: number) => void;
  setSelectedWebSearchTemplateId: (id: string | undefined) => void;
  setLoading: (loading: boolean) => void;
  setServerSaving: (saving: boolean) => void;
  setSaving: (saving: boolean) => void;
  setWebSearchSaving: (saving: boolean) => void;

  // 异步操作
  loadConfiguration: () => Promise<void>;
}

export const useConfigStore = create<ConfigState>()(
  devtools(
    (set) => ({
      // 初始状态
      configuration: undefined,
      souls: [],
      workspaceTemplates: [],
      defaultProviderId: undefined,
      defaultEmbeddingProviderId: undefined,
      modelCapabilityDrafts: [],
      providerDrafts: [],
      selectedProviderIndex: 0,
      selectedTemplateId: undefined,
      defaultWebSearchProviderId: undefined,
      webSearchProviderDrafts: [],
      selectedWebSearchProviderIndex: 0,
      selectedWebSearchTemplateId: undefined,
      loading: false,
      serverSaving: false,
      saving: false,
      webSearchSaving: false,

      // Setters
      setConfiguration: (configuration) => set({ configuration }),
      setSouls: (souls) => set({ souls }),
      setWorkspaceTemplates: (workspaceTemplates) => set({ workspaceTemplates }),
      setDefaultProviderId: (defaultProviderId) => set({ defaultProviderId }),
      setDefaultEmbeddingProviderId: (defaultEmbeddingProviderId) => set({ defaultEmbeddingProviderId }),
      setModelCapabilityDrafts: (drafts) =>
        set((state) => ({
          modelCapabilityDrafts: typeof drafts === 'function' ? drafts(state.modelCapabilityDrafts) : drafts
        })),
      setProviderDrafts: (drafts) =>
        set((state) => ({
          providerDrafts: typeof drafts === 'function' ? drafts(state.providerDrafts) : drafts
        })),
      setSelectedProviderIndex: (selectedProviderIndex) => set({ selectedProviderIndex }),
      setSelectedTemplateId: (selectedTemplateId) => set({ selectedTemplateId }),
      setDefaultWebSearchProviderId: (defaultWebSearchProviderId) => set({ defaultWebSearchProviderId }),
      setWebSearchProviderDrafts: (drafts) =>
        set((state) => ({
          webSearchProviderDrafts: typeof drafts === 'function' ? drafts(state.webSearchProviderDrafts) : drafts
        })),
      setSelectedWebSearchProviderIndex: (selectedWebSearchProviderIndex) => set({ selectedWebSearchProviderIndex }),
      setSelectedWebSearchTemplateId: (selectedWebSearchTemplateId) => set({ selectedWebSearchTemplateId }),
      setLoading: (loading) => set({ loading }),
      setServerSaving: (serverSaving) => set({ serverSaving }),
      setSaving: (saving) => set({ saving }),
      setWebSearchSaving: (webSearchSaving) => set({ webSearchSaving }),

      // 异步操作
      loadConfiguration: async () => {
        set({ loading: true });
        try {
          const [nextConfiguration, nextSouls, nextWorkspaceTemplates] = await Promise.all([
            workorchApi.readConfiguration(),
            workorchApi.listSouls(),
            workorchApi.listWorkspaceTemplates()
          ]);

          set({
            configuration: nextConfiguration,
            souls: nextSouls,
            workspaceTemplates: nextWorkspaceTemplates,
            loading: false
          });

          return nextConfiguration;
        } catch (error) {
          set({ loading: false });
          throw error;
        }
      }
    }),
    { name: 'ConfigStore' }
  )
);
