import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import type { MCPServer, MCPTool, MCPResource, MCPPrompt, MCPCapability } from '../types';
import { workorchApi } from '../api';

interface MCPCapabilityFilterValues {
  serverId?: string;
  kind?: string;
  category?: string;
  stage?: string;
  riskLevel?: string;
  enabled?: boolean;
  q?: string;
}

interface RuntimeState {
  servers: MCPServer[];
  selectedServerId: string | undefined;
  tools: MCPTool[];
  resources: MCPResource[];
  prompts: MCPPrompt[];
  capabilities: MCPCapability[];
  editingCapability: MCPCapability | undefined;
  capabilityModalOpen: boolean;
  metrics: string;
  loading: boolean;
  saving: boolean;

  setServers: (servers: MCPServer[]) => void;
  setSelectedServerId: (id: string | undefined) => void;
  setTools: (tools: MCPTool[]) => void;
  setResources: (resources: MCPResource[]) => void;
  setPrompts: (prompts: MCPPrompt[]) => void;
  setCapabilities: (capabilities: MCPCapability[]) => void;
  setEditingCapability: (capability: MCPCapability | undefined) => void;
  setCapabilityModalOpen: (open: boolean) => void;
  setMetrics: (metrics: string) => void;
  setLoading: (loading: boolean) => void;
  setSaving: (saving: boolean) => void;
  loadCapabilityDirectory: (filter?: MCPCapabilityFilterValues) => Promise<void>;
  loadRuntime: (currentFilter?: MCPCapabilityFilterValues) => Promise<void>;
  reset: () => void;
}

export const useRuntimeStore = create<RuntimeState>()(
  devtools(
    (set, get) => ({
      servers: [],
      selectedServerId: undefined,
      tools: [],
      resources: [],
      prompts: [],
      capabilities: [],
      editingCapability: undefined,
      capabilityModalOpen: false,
      metrics: '',
      loading: false,
      saving: false,

      setServers: (servers) => set({ servers }),
      setSelectedServerId: (id) => set({ selectedServerId: id }),
      setTools: (tools) => set({ tools }),
      setResources: (resources) => set({ resources }),
      setPrompts: (prompts) => set({ prompts }),
      setCapabilities: (capabilities) => set({ capabilities }),
      setEditingCapability: (capability) => set({ editingCapability: capability }),
      setCapabilityModalOpen: (open) => set({ capabilityModalOpen: open }),
      setMetrics: (metrics) => set({ metrics }),
      setLoading: (loading) => set({ loading }),
      setSaving: (saving) => set({ saving }),

      loadCapabilityDirectory: async (filter) => {
        const capabilities = await workorchApi.listMCPCapabilities({
          serverId: filter?.serverId,
          kind: filter?.kind,
          category: filter?.category,
          stage: filter?.stage,
          riskLevel: filter?.riskLevel,
          enabled: filter?.enabled,
          q: filter?.q,
        });
        set({ capabilities });
      },

      loadRuntime: async (currentFilter) => {
        set({ loading: true });
        try {
          const [nextServers, nextMetrics] = await Promise.all([
            workorchApi.listMCPServers(),
            workorchApi.metrics(),
          ]);
          set({ servers: nextServers, metrics: nextMetrics });
          await get().loadCapabilityDirectory(currentFilter);
          const state = get();
          if (!state.selectedServerId && nextServers[0]) {
            set({ selectedServerId: nextServers[0].id });
          }
        } finally {
          set({ loading: false });
        }
      },

      reset: () =>
        set({
          servers: [],
          selectedServerId: undefined,
          tools: [],
          resources: [],
          prompts: [],
          capabilities: [],
          editingCapability: undefined,
          capabilityModalOpen: false,
          metrics: '',
          loading: false,
          saving: false,
        }),
    }),
    { name: 'RuntimeStore' }
  )
);
