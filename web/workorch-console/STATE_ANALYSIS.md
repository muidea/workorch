# WorkOrch Console 状态管理分析

## 📊 统计概览

- **总 useState 数量**: 121 个
- **当前状态管理**: 全部使用 React useState
- **目标**: 迁移全局状态到 Zustand，保留局部 UI 状态

## 🗂️ 状态分类

### 1. 全局应用状态（需要迁移到 Zustand）

#### 1.1 工作空间相关（WorkspaceStore）
```typescript
// 行 2391-2397, 2450-2459
const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
const [workspaceId, setWorkspaceId] = useState<string | undefined>(undefined);
const [snapshot, setSnapshot] = useState<WorkspaceSnapshot | undefined>(undefined);
const [goals, setGoals] = useState<Goal[]>([]);
const [dods, setDoDs] = useState<DoD[]>([]);
const [souls, setSouls] = useState<Soul[]>([]);
const [soulBindings, setSoulBindings] = useState<WorkspaceSoulBindings | undefined>(undefined);
const [workspaceDeleteOpen, setWorkspaceDeleteOpen] = useState(false);
const [workspaceListReady, setWorkspaceListReady] = useState(false);
const [setupStep, setSetupStep] = useState(0);
const [workspaceSetupMode, setWorkspaceSetupMode] = useState<WorkspaceSetupMode>('manual');
```

**迁移优先级**: 🔴 高
**原因**: 跨多个页面共享，SSE 事件更新频繁

#### 1.2 工作空间看板（BoardStore）
```typescript
// 行 1800-1801
const [boardItems, setBoardItems] = useState<WorkspaceBoardItem[]>([]);
const [loading, setLoading] = useState(false);
```

**迁移优先级**: 🔴 高
**原因**: 独立页面，但数据量大，需要优化

#### 1.3 决策队列（DecisionStore）
```typescript
// 行 1926-1932
const [selectedDecisionKeys, setSelectedDecisionKeys] = useState<React.Key[]>([]);
const [decisionTarget, setDecisionTarget] = useState<{ item: Decision; resolutionType: string } | undefined>(undefined);
const [decisionSubmitting, setDecisionSubmitting] = useState(false);
const [decisionRecords, setDecisionRecords] = useState<Decision[]>([]);
const [decisionCursorAfterId, setDecisionCursorAfterId] = useState<string | undefined>(undefined);
const [decisionHasMore, setDecisionHasMore] = useState(false);
const [decisionLoading, setDecisionLoading] = useState(false);
```

**迁移优先级**: 🟡 中
**原因**: 独立页面，分页逻辑可复用

#### 1.4 审批队列（ApprovalStore）
```typescript
// 行 2174-2180
const [selectedApprovalKeys, setSelectedApprovalKeys] = useState<React.Key[]>([]);
const [approvalTarget, setApprovalTarget] = useState<{ item: Approval; action: 'approve' | 'reject'; relatedDecision?: Decision } | undefined>(undefined);
const [approvalSubmitting, setApprovalSubmitting] = useState(false);
const [approvalRecords, setApprovalRecords] = useState<Approval[]>([]);
const [approvalCursorAfterId, setApprovalCursorAfterId] = useState<string | undefined>(undefined);
const [approvalHasMore, setApprovalHasMore] = useState(false);
const [approvalLoading, setApprovalLoading] = useState(false);
```

**迁移优先级**: 🟡 中
**原因**: 独立页面，与 DecisionStore 类似

#### 1.5 MCP 运行时（RuntimeStore）
```typescript
// 行 5017-5027
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
```

**迁移优先级**: 🟡 中
**原因**: 独立页面，但状态较多

#### 1.6 配置管理（ConfigStore）
```typescript
// 行 5589-5605
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
```

**迁移优先级**: 🟡 中
**原因**: 配置页面，状态复杂但更新不频繁

#### 1.7 视图路由（ViewStore）
```typescript
// 行 6360-6361
const [view, setView] = useState<ViewKey>('board');
const [selectedWorkspaceId, setSelectedWorkspaceId] = useState<string | undefined>(undefined);
```

**迁移优先级**: 🔴 高
**原因**: 全局路由状态，影响整个应用

### 2. 局部 UI 状态（保留 useState）

#### 2.1 编辑模态框状态
```typescript
// 行 1458-1460 (Soul 编辑)
const [editingSoul, setEditingSoul] = useState<Soul | undefined>(undefined);
const [saving, setSaving] = useState(false);
const [deletingSoulId, setDeletingSoulId] = useState<string | undefined>(undefined);

// 行 1602-1604 (Template 编辑)
const [editingTemplate, setEditingTemplate] = useState<WorkspaceTemplate | undefined>(undefined);
const [saving, setSaving] = useState(false);
const [deletingTemplateId, setDeletingTemplateId] = useState<string | undefined>(undefined);

// 行 2454-2455 (Goal/DoD 编辑)
const [editingGoal, setEditingGoal] = useState<Goal | undefined>(undefined);
const [editingDoD, setEditingDoD] = useState<DoD | undefined>(undefined);
```

**保留原因**: 仅在组件内部使用，不需要跨组件共享

#### 2.2 提交/加载状态
```typescript
const [businessSubmitting, setBusinessSubmitting] = useState(false);
const [workspaceDeleting, setWorkspaceDeleting] = useState(false);
const [soulBindingSaving, setSoulBindingSaving] = useState(false);
const [activationTracking, setActivationTracking] = useState(false);
```

**保留原因**: 临时 UI 状态，生命周期短

## 🎯 Zustand Store 设计

### Store 1: workspaceStore
```typescript
interface WorkspaceState {
  // 数据
  workspaces: Workspace[];
  selectedId: string | undefined;
  snapshot: WorkspaceSnapshot | undefined;
  goals: Goal[];
  dods: DoD[];
  souls: Soul[];
  soulBindings: WorkspaceSoulBindings | undefined;
  
  // UI 状态
  deleteOpen: boolean;
  listReady: boolean;
  setupStep: number;
  setupMode: WorkspaceSetupMode;
  
  // Actions
  setWorkspaces: (workspaces: Workspace[]) => void;
  selectWorkspace: (id: string | undefined) => void;
  updateSnapshot: (snapshot: WorkspaceSnapshot) => void;
  mergeWorkspaces: (newWorkspaces: Workspace[]) => void;
  addGoal: (goal: Goal) => void;
  updateGoal: (id: string, updates: Partial<Goal>) => void;
  deleteGoal: (id: string) => void;
  // ... 其他 actions
}
```

### Store 2: boardStore
```typescript
interface BoardState {
  items: WorkspaceBoardItem[];
  loading: boolean;
  
  setItems: (items: WorkspaceBoardItem[]) => void;
  mergeItems: (newItems: WorkspaceBoardItem[]) => void;
  setLoading: (loading: boolean) => void;
}
```

### Store 3: decisionStore
```typescript
interface DecisionState {
  records: Decision[];
  selectedKeys: React.Key[];
  target: { item: Decision; resolutionType: string } | undefined;
  submitting: boolean;
  cursorAfterId: string | undefined;
  hasMore: boolean;
  loading: boolean;
  
  loadMore: () => Promise<void>;
  reset: () => void;
  selectDecisions: (keys: React.Key[]) => void;
  setTarget: (target: DecisionState['target']) => void;
  submitResolution: (payload: any) => Promise<void>;
}
```

### Store 4: approvalStore
```typescript
interface ApprovalState {
  records: Approval[];
  selectedKeys: React.Key[];
  target: { item: Approval; action: 'approve' | 'reject'; relatedDecision?: Decision } | undefined;
  submitting: boolean;
  cursorAfterId: string | undefined;
  hasMore: boolean;
  loading: boolean;
  
  loadMore: () => Promise<void>;
  reset: () => void;
  selectApprovals: (keys: React.Key[]) => void;
  setTarget: (target: ApprovalState['target']) => void;
  submitAction: (action: 'approve' | 'reject') => Promise<void>;
}
```

### Store 5: runtimeStore
```typescript
interface RuntimeState {
  servers: MCPServer[];
  selectedServerId: string | undefined;
  tools: MCPTool[];
  resources: MCPResource[];
  prompts: MCPPrompt[];
  capabilities: MCPCapability[];
  metrics: string;
  loading: boolean;
  saving: boolean;
  
  loadServers: () => Promise<void>;
  selectServer: (id: string | undefined) => void;
  loadServerDetails: (id: string) => Promise<void>;
  saveCapability: (capability: MCPCapability) => Promise<void>;
}
```

### Store 6: configStore
```typescript
interface ConfigState {
  configuration: ServiceConfiguration | undefined;
  souls: Soul[];
  workspaceTemplates: WorkspaceTemplate[];
  defaultProviderId: string | undefined;
  defaultEmbeddingProviderId: string | undefined;
  modelCapabilityDrafts: LLMModelCapabilityConfiguration[];
  providerDrafts: UpdateLLMProviderConfiguration[];
  selectedProviderIndex: number;
  selectedTemplateId: string | undefined;
  defaultWebSearchProviderId: string | undefined;
  webSearchProviderDrafts: UpdateWebSearchProviderConfiguration[];
  selectedWebSearchProviderIndex: number;
  selectedWebSearchTemplateId: string | undefined;
  loading: boolean;
  serverSaving: boolean;
  saving: boolean;
  webSearchSaving: boolean;
  
  loadConfiguration: () => Promise<void>;
  updateProvider: (index: number, updates: Partial<UpdateLLMProviderConfiguration>) => void;
  saveConfiguration: () => Promise<void>;
  // ... 其他 actions
}
```

### Store 7: viewStore
```typescript
interface ViewState {
  currentView: ViewKey;
  selectedWorkspaceId: string | undefined;
  
  setView: (view: ViewKey) => void;
  selectWorkspace: (id: string | undefined) => void;
}
```

## 📋 迁移计划

### 阶段 1: 创建 Stores（1-2 小时）
1. ✅ 创建 `src/store/workspaceStore.ts`（已存在，需扩展）
2. 创建 `src/store/boardStore.ts`
3. 创建 `src/store/decisionStore.ts`
4. 创建 `src/store/approvalStore.ts`
5. 创建 `src/store/runtimeStore.ts`
6. 创建 `src/store/configStore.ts`
7. 创建 `src/store/viewStore.ts`
8. 创建 `src/store/index.ts` 统一导出

### 阶段 2: 迁移高优先级状态（2-3 小时）
1. 迁移 viewStore（最简单，影响最大）
2. 扩展 workspaceStore（已有基础）
3. 迁移 boardStore

### 阶段 3: 迁移中优先级状态（2-3 小时）
1. 迁移 decisionStore
2. 迁移 approvalStore
3. 迁移 runtimeStore
4. 迁移 configStore

### 阶段 4: SSE 事件集成（1-2 小时）
1. 在 stores 中添加 SSE 事件处理
2. 实现增量更新逻辑
3. 添加防抖和节流

### 阶段 5: 验证和优化（1 小时）
1. 测试所有功能
2. 检查性能改进
3. 清理未使用的 useState

## 📈 预期收益

### 性能改进
- **减少不必要的重渲染**: 通过 Zustand 的选择器机制
- **更好的状态更新**: 使用 Immer 进行不可变更新
- **SSE 事件优化**: 统一的事件处理和防抖

### 代码质量
- **状态逻辑集中**: 从 121 个 useState 减少到约 30-40 个
- **更好的类型安全**: Zustand 提供完整的 TypeScript 支持
- **更容易测试**: Store 可以独立测试

### 开发体验
- **更清晰的数据流**: 单向数据流
- **更容易调试**: Zustand DevTools 支持
- **更好的代码组织**: 状态逻辑与 UI 分离

## 🎯 成功指标

- [ ] 121 个 useState 减少到 30-40 个
- [ ] 7 个 Zustand stores 创建完成
- [ ] SSE 事件处理优化完成
- [ ] 构建通过，无 TypeScript 错误
- [ ] 所有功能正常工作
- [ ] 性能测试通过（减少重渲染次数）

---

**生成时间**: 2026-05-19  
**分析文件**: src/main.tsx (6428 行)  
**useState 总数**: 121 个
