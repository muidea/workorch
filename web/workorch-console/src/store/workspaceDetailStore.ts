import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import type {
  WorkspaceTemplate,
  MCPServer,
  MCPCapability,
  MemoryItem,
  ServiceConfiguration,
  RunSnapshot,
  RunObservability,
  ContextBundle,
  EventRecord,
  StepView,
  ToolCall,
  RuntimeCheckpoint,
  Result,
  Patch,
  ValidationResult,
  Artifact,
  WorkspaceSetupDraftSession,
  Goal,
  DoD
} from '../types';

interface WorkspaceDetailState {
  // 基础数据
  workspaceTemplates: WorkspaceTemplate[];
  mcpServers: MCPServer[];
  mcpCapabilities: MCPCapability[];
  memoryItems: MemoryItem[];
  configuration: ServiceConfiguration | undefined;

  // 运行时数据
  runSnapshot: RunSnapshot | undefined;
  runObservability: RunObservability | undefined;
  contextBundle: ContextBundle | undefined;

  // 运行事件分页
  runEventRecords: EventRecord[];
  runEventCursorAfterId: string | undefined;
  runEventHasMore: boolean;
  runEventLoading: boolean;

  // 运行步骤分页
  runStepRecords: StepView[];
  runStepCursorAfterId: string | undefined;
  runStepHasMore: boolean;
  runStepLoading: boolean;

  // 工具调用分页
  runToolCallRecords: ToolCall[];
  runToolCallCursorAfterId: string | undefined;
  runToolCallHasMore: boolean;
  runToolCallLoading: boolean;

  // 检查点分页
  runCheckpointRecords: RuntimeCheckpoint[];
  runCheckpointCursorAfterId: string | undefined;
  runCheckpointHasMore: boolean;
  runCheckpointLoading: boolean;

  // 结果分页
  runResultRecords: Result[];
  runResultCursorAfterId: string | undefined;
  runResultHasMore: boolean;
  runResultLoading: boolean;

  // 补丁分页
  runPatchRecords: Patch[];
  runPatchCursorAfterId: string | undefined;
  runPatchHasMore: boolean;
  runPatchLoading: boolean;

  // 验证结果分页
  runValidationRecords: ValidationResult[];
  runValidationCursorAfterId: string | undefined;
  runValidationHasMore: boolean;
  runValidationLoading: boolean;

  // 工件分页
  runArtifactRecords: Artifact[];
  runArtifactCursorAfterId: string | undefined;
  runArtifactHasMore: boolean;
  runArtifactLoading: boolean;

  // UI 状态
  loading: boolean;
  activationOpen: boolean;
  activationLoading: boolean;
  workspaceSetupOpen: boolean;
  workspaceSetupDraftSession: WorkspaceSetupDraftSession | undefined;
  workspaceSetupDraftInput: string;
  workspaceSetupDraftLoading: boolean;
  workspaceSetupDraftApplied: boolean;
  goalDefinitionOpen: boolean;
  dodDefinitionOpen: boolean;
  soulBindingEditorOpen: boolean;
  businessSubmitting: boolean;
  workspaceDeleting: boolean;
  soulBindingSaving: boolean;
  editingGoal: Goal | undefined;
  editingDoD: DoD | undefined;
  activationTracking: boolean;

  // Setters - 基础数据
  setWorkspaceTemplates: (templates: WorkspaceTemplate[]) => void;
  setMCPServers: (servers: MCPServer[]) => void;
  setMCPCapabilities: (capabilities: MCPCapability[]) => void;
  setMemoryItems: (items: MemoryItem[]) => void;
  setConfiguration: (config: ServiceConfiguration | undefined) => void;

  // Setters - 运行时数据
  setRunSnapshot: (snapshot: RunSnapshot | undefined) => void;
  setRunObservability: (observability: RunObservability | undefined) => void;
  setContextBundle: (bundle: ContextBundle | undefined) => void;

  // Setters - 运行事件
  setRunEventRecords: (records: EventRecord[] | ((prev: EventRecord[]) => EventRecord[])) => void;
  setRunEventCursorAfterId: (id: string | undefined) => void;
  setRunEventHasMore: (hasMore: boolean) => void;
  setRunEventLoading: (loading: boolean) => void;

  // Setters - 运行步骤
  setRunStepRecords: (records: StepView[] | ((prev: StepView[]) => StepView[])) => void;
  setRunStepCursorAfterId: (id: string | undefined) => void;
  setRunStepHasMore: (hasMore: boolean) => void;
  setRunStepLoading: (loading: boolean) => void;

  // Setters - 工具调用
  setRunToolCallRecords: (records: ToolCall[] | ((prev: ToolCall[]) => ToolCall[])) => void;
  setRunToolCallCursorAfterId: (id: string | undefined) => void;
  setRunToolCallHasMore: (hasMore: boolean) => void;
  setRunToolCallLoading: (loading: boolean) => void;

  // Setters - 检查点
  setRunCheckpointRecords: (records: RuntimeCheckpoint[] | ((prev: RuntimeCheckpoint[]) => RuntimeCheckpoint[])) => void;
  setRunCheckpointCursorAfterId: (id: string | undefined) => void;
  setRunCheckpointHasMore: (hasMore: boolean) => void;
  setRunCheckpointLoading: (loading: boolean) => void;

  // Setters - 结果
  setRunResultRecords: (records: Result[] | ((prev: Result[]) => Result[])) => void;
  setRunResultCursorAfterId: (id: string | undefined) => void;
  setRunResultHasMore: (hasMore: boolean) => void;
  setRunResultLoading: (loading: boolean) => void;

  // Setters - 补丁
  setRunPatchRecords: (records: Patch[] | ((prev: Patch[]) => Patch[])) => void;
  setRunPatchCursorAfterId: (id: string | undefined) => void;
  setRunPatchHasMore: (hasMore: boolean) => void;
  setRunPatchLoading: (loading: boolean) => void;

  // Setters - 验证结果
  setRunValidationRecords: (records: ValidationResult[] | ((prev: ValidationResult[]) => ValidationResult[])) => void;
  setRunValidationCursorAfterId: (id: string | undefined) => void;
  setRunValidationHasMore: (hasMore: boolean) => void;
  setRunValidationLoading: (loading: boolean) => void;

  // Setters - 工件
  setRunArtifactRecords: (records: Artifact[] | ((prev: Artifact[]) => Artifact[])) => void;
  setRunArtifactCursorAfterId: (id: string | undefined) => void;
  setRunArtifactHasMore: (hasMore: boolean) => void;
  setRunArtifactLoading: (loading: boolean) => void;

  // Setters - UI 状态
  setLoading: (loading: boolean) => void;
  setActivationOpen: (open: boolean) => void;
  setActivationLoading: (loading: boolean) => void;
  setWorkspaceSetupOpen: (open: boolean) => void;
  setWorkspaceSetupDraftSession: (session: WorkspaceSetupDraftSession | undefined) => void;
  setWorkspaceSetupDraftInput: (input: string) => void;
  setWorkspaceSetupDraftLoading: (loading: boolean) => void;
  setWorkspaceSetupDraftApplied: (applied: boolean) => void;
  setGoalDefinitionOpen: (open: boolean) => void;
  setDoDDefinitionOpen: (open: boolean) => void;
  setSoulBindingEditorOpen: (open: boolean) => void;
  setBusinessSubmitting: (submitting: boolean) => void;
  setWorkspaceDeleting: (deleting: boolean) => void;
  setSoulBindingSaving: (saving: boolean) => void;
  setEditingGoal: (goal: Goal | undefined) => void;
  setEditingDoD: (dod: DoD | undefined) => void;
  setActivationTracking: (tracking: boolean) => void;

  // 重置方法
  resetRunData: () => void;
}

export const useWorkspaceDetailStore = create<WorkspaceDetailState>()(
  devtools(
    (set) => ({
      // 初始状态 - 基础数据
      workspaceTemplates: [],
      mcpServers: [],
      mcpCapabilities: [],
      memoryItems: [],
      configuration: undefined,

      // 初始状态 - 运行时数据
      runSnapshot: undefined,
      runObservability: undefined,
      contextBundle: undefined,

      // 初始状态 - 运行事件
      runEventRecords: [],
      runEventCursorAfterId: undefined,
      runEventHasMore: false,
      runEventLoading: false,

      // 初始状态 - 运行步骤
      runStepRecords: [],
      runStepCursorAfterId: undefined,
      runStepHasMore: false,
      runStepLoading: false,

      // 初始状态 - 工具调用
      runToolCallRecords: [],
      runToolCallCursorAfterId: undefined,
      runToolCallHasMore: false,
      runToolCallLoading: false,

      // 初始状态 - 检查点
      runCheckpointRecords: [],
      runCheckpointCursorAfterId: undefined,
      runCheckpointHasMore: false,
      runCheckpointLoading: false,

      // 初始状态 - 结果
      runResultRecords: [],
      runResultCursorAfterId: undefined,
      runResultHasMore: false,
      runResultLoading: false,

      // 初始状态 - 补丁
      runPatchRecords: [],
      runPatchCursorAfterId: undefined,
      runPatchHasMore: false,
      runPatchLoading: false,

      // 初始状态 - 验证结果
      runValidationRecords: [],
      runValidationCursorAfterId: undefined,
      runValidationHasMore: false,
      runValidationLoading: false,

      // 初始状态 - 工件
      runArtifactRecords: [],
      runArtifactCursorAfterId: undefined,
      runArtifactHasMore: false,
      runArtifactLoading: false,

      // 初始状态 - UI 状态
      loading: false,
      activationOpen: false,
      activationLoading: false,
      workspaceSetupOpen: false,
      workspaceSetupDraftSession: undefined,
      workspaceSetupDraftInput: '',
      workspaceSetupDraftLoading: false,
      workspaceSetupDraftApplied: false,
      goalDefinitionOpen: false,
      dodDefinitionOpen: false,
      soulBindingEditorOpen: false,
      businessSubmitting: false,
      workspaceDeleting: false,
      soulBindingSaving: false,
      editingGoal: undefined,
      editingDoD: undefined,
      activationTracking: false,

      // Setters - 基础数据
      setWorkspaceTemplates: (workspaceTemplates) => set({ workspaceTemplates }),
      setMCPServers: (mcpServers) => set({ mcpServers }),
      setMCPCapabilities: (mcpCapabilities) => set({ mcpCapabilities }),
      setMemoryItems: (memoryItems) => set({ memoryItems }),
      setConfiguration: (configuration) => set({ configuration }),

      // Setters - 运行时数据
      setRunSnapshot: (runSnapshot) => set({ runSnapshot }),
      setRunObservability: (runObservability) => set({ runObservability }),
      setContextBundle: (contextBundle) => set({ contextBundle }),

      // Setters - 运行事件
      setRunEventRecords: (records) =>
        set((state) => ({
          runEventRecords: typeof records === 'function' ? records(state.runEventRecords) : records
        })),
      setRunEventCursorAfterId: (runEventCursorAfterId) => set({ runEventCursorAfterId }),
      setRunEventHasMore: (runEventHasMore) => set({ runEventHasMore }),
      setRunEventLoading: (runEventLoading) => set({ runEventLoading }),

      // Setters - 运行步骤
      setRunStepRecords: (records) =>
        set((state) => ({
          runStepRecords: typeof records === 'function' ? records(state.runStepRecords) : records
        })),
      setRunStepCursorAfterId: (runStepCursorAfterId) => set({ runStepCursorAfterId }),
      setRunStepHasMore: (runStepHasMore) => set({ runStepHasMore }),
      setRunStepLoading: (runStepLoading) => set({ runStepLoading }),

      // Setters - 工具调用
      setRunToolCallRecords: (records) =>
        set((state) => ({
          runToolCallRecords: typeof records === 'function' ? records(state.runToolCallRecords) : records
        })),
      setRunToolCallCursorAfterId: (runToolCallCursorAfterId) => set({ runToolCallCursorAfterId }),
      setRunToolCallHasMore: (runToolCallHasMore) => set({ runToolCallHasMore }),
      setRunToolCallLoading: (runToolCallLoading) => set({ runToolCallLoading }),

      // Setters - 检查点
      setRunCheckpointRecords: (records) =>
        set((state) => ({
          runCheckpointRecords: typeof records === 'function' ? records(state.runCheckpointRecords) : records
        })),
      setRunCheckpointCursorAfterId: (runCheckpointCursorAfterId) => set({ runCheckpointCursorAfterId }),
      setRunCheckpointHasMore: (runCheckpointHasMore) => set({ runCheckpointHasMore }),
      setRunCheckpointLoading: (runCheckpointLoading) => set({ runCheckpointLoading }),

      // Setters - 结果
      setRunResultRecords: (records) =>
        set((state) => ({
          runResultRecords: typeof records === 'function' ? records(state.runResultRecords) : records
        })),
      setRunResultCursorAfterId: (runResultCursorAfterId) => set({ runResultCursorAfterId }),
      setRunResultHasMore: (runResultHasMore) => set({ runResultHasMore }),
      setRunResultLoading: (runResultLoading) => set({ runResultLoading }),

      // Setters - 补丁
      setRunPatchRecords: (records) =>
        set((state) => ({
          runPatchRecords: typeof records === 'function' ? records(state.runPatchRecords) : records
        })),
      setRunPatchCursorAfterId: (runPatchCursorAfterId) => set({ runPatchCursorAfterId }),
      setRunPatchHasMore: (runPatchHasMore) => set({ runPatchHasMore }),
      setRunPatchLoading: (runPatchLoading) => set({ runPatchLoading }),

      // Setters - 验证结果
      setRunValidationRecords: (records) =>
        set((state) => ({
          runValidationRecords: typeof records === 'function' ? records(state.runValidationRecords) : records
        })),
      setRunValidationCursorAfterId: (runValidationCursorAfterId) => set({ runValidationCursorAfterId }),
      setRunValidationHasMore: (runValidationHasMore) => set({ runValidationHasMore }),
      setRunValidationLoading: (runValidationLoading) => set({ runValidationLoading }),

      // Setters - 工件
      setRunArtifactRecords: (records) =>
        set((state) => ({
          runArtifactRecords: typeof records === 'function' ? records(state.runArtifactRecords) : records
        })),
      setRunArtifactCursorAfterId: (runArtifactCursorAfterId) => set({ runArtifactCursorAfterId }),
      setRunArtifactHasMore: (runArtifactHasMore) => set({ runArtifactHasMore }),
      setRunArtifactLoading: (runArtifactLoading) => set({ runArtifactLoading }),

      // Setters - UI 状态
      setLoading: (loading) => set({ loading }),
      setActivationOpen: (activationOpen) => set({ activationOpen }),
      setActivationLoading: (activationLoading) => set({ activationLoading }),
      setWorkspaceSetupOpen: (workspaceSetupOpen) => set({ workspaceSetupOpen }),
      setWorkspaceSetupDraftSession: (workspaceSetupDraftSession) => set({ workspaceSetupDraftSession }),
      setWorkspaceSetupDraftInput: (workspaceSetupDraftInput) => set({ workspaceSetupDraftInput }),
      setWorkspaceSetupDraftLoading: (workspaceSetupDraftLoading) => set({ workspaceSetupDraftLoading }),
      setWorkspaceSetupDraftApplied: (workspaceSetupDraftApplied) => set({ workspaceSetupDraftApplied }),
      setGoalDefinitionOpen: (goalDefinitionOpen) => set({ goalDefinitionOpen }),
      setDoDDefinitionOpen: (dodDefinitionOpen) => set({ dodDefinitionOpen }),
      setSoulBindingEditorOpen: (soulBindingEditorOpen) => set({ soulBindingEditorOpen }),
      setBusinessSubmitting: (businessSubmitting) => set({ businessSubmitting }),
      setWorkspaceDeleting: (workspaceDeleting) => set({ workspaceDeleting }),
      setSoulBindingSaving: (soulBindingSaving) => set({ soulBindingSaving }),
      setEditingGoal: (editingGoal) => set({ editingGoal }),
      setEditingDoD: (editingDoD) => set({ editingDoD }),
      setActivationTracking: (activationTracking) => set({ activationTracking }),

      // 重置方法
      resetRunData: () =>
        set({
          runSnapshot: undefined,
          runObservability: undefined,
          contextBundle: undefined,
          runEventRecords: [],
          runEventCursorAfterId: undefined,
          runEventHasMore: false,
          runStepRecords: [],
          runStepCursorAfterId: undefined,
          runStepHasMore: false,
          runToolCallRecords: [],
          runToolCallCursorAfterId: undefined,
          runToolCallHasMore: false,
          runCheckpointRecords: [],
          runCheckpointCursorAfterId: undefined,
          runCheckpointHasMore: false,
          runResultRecords: [],
          runResultCursorAfterId: undefined,
          runResultHasMore: false,
          runPatchRecords: [],
          runPatchCursorAfterId: undefined,
          runPatchHasMore: false,
          runValidationRecords: [],
          runValidationCursorAfterId: undefined,
          runValidationHasMore: false,
          runArtifactRecords: [],
          runArtifactCursorAfterId: undefined,
          runArtifactHasMore: false
        })
    }),
    { name: 'WorkspaceDetailStore' }
  )
);
