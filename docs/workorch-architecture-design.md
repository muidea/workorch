# workorch 架构与设计实现文档

## 1. 项目总览

### 1.1 顶层结构

```
workorch/
├── application/workorchd/     # 应用入口点
│   └── main.go                # 解析 -config-dir，启动 magicCommon 生命周期
├── config/
│   └── application.toml       # 运行时配置
├── internal/
│   ├── initiators/            # 插件初始化器（运行时启动器）
│   ├── modules/               # 所有模块
│   │   ├── kernel/            # 核心领域模块（10 个）
│   │   ├── blocks/            # 运行时适配器模块（5 个）
│   │   └── application/       # 应用层模块（workorchd）
│   └── pkg/                   # 共享库
│       ├── common/            # 基础类型（ID、状态、错误）
│       ├── events/            # 事件系统
│       ├── repository/        # 仓库接口 + 文件/内存实现
│       └── workorchconfig/    # 配置管理
├── web/workorch-console/      # React SPA 前端
├── scripts/                   # 验证与压测脚本
├── docs/                      # 设计文档
├── bin/                       # 构建输出
└── go.mod                     # Go 1.24.13，模块 github.com/muidea/workorch
```

### 1.2 核心依赖

| 依赖 | 用途 |
|------|------|
| magicCommon v1.5.7 | 应用框架（启动、关闭、插件生命周期） |
| magicEngine v1.4.9 | HTTP 路由与中间件 |
| anthropic-sdk-go v1.38.0 | Anthropic LLM 适配器 |
| openai-go v1.12.0 | OpenAI LLM 适配器 |
| antd 5 + ProComponents 2.8 | Web Console UI |

---

## 2. 模块架构模式

每个模块遵循统一的 4 层结构：

```
module/
├── plugin.go          # 注册权重 + 模块工厂到 pluginmodule.Register()
├── module.go          # Setup(RunTimeHelper) / Run() / Teardown()
├── pkg/
│   ├── models/        # 领域模型结构体
│   └── common/        # 模块级常量/类型
├── service/
│   └── service.go     # 公开接口 + 函数指针包装的 Module 结构体
└── biz/               # 业务逻辑实现
```

模块通过 `plugin.go` 中的 `init()` 函数注册到框架的 `pluginmodule.Register()`，权重决定初始化顺序。

---

## 3. 核心模块（Kernel）

### 3.1 Workspace 模块（权重 20）

**Workspace**：{ID, Title, Description, WorkingDirectory, State, ActiveRunID, DefaultSoulID, SoulBindings}

Workspace 是最高层级的组织单元。包含工作目录、激活的 Run、以及按阶段绑定的 Soul 映射。

**Goal**：{ID, WorkspaceID, Text} — 自然语言描述的执行目标

**DoD**（Definition of Done）：{ID, WorkspaceID, Items（多项目标检查项）, State}

DoD 定义了验收条件，其状态从 Ready→Satisfied，是 Run 完成验收（accepted Result）的前置条件。

**Workspace 状态机**：
- `active` / `running` / `waiting_decision` / `blocked` / `validating` / `completed` / `failed`
- 状态与 Run 同步：workspace 状态始终反映其活跃 Run 的当前阶段

**服务接口**：
```go
CreateWorkspace / EnsureWorkspace / DeleteWorkspace
DefineGoal / UpdateGoal
DefineDoD / UpdateDoD
```

### 3.2 Run 模块（权重 40，最高内核权重）

**Run**：{ID, WorkspaceID, GoalID, DoDID, InitialStepKind, ExecutionContext, State, Priority, BlockingTicketID, BlockingApprovalID, FailureReason}

**Step**：{ID, RunID, Kind, State, Input, Output}

Step 是 Run 的基本执行单元。每个 Step 由 LLM 或工具调用驱动，输出包含下一步指令或治理信号。

**证据模型**：
| 模型 | 用途 |
|------|------|
| ToolCall | 记录每一步的 runtime adapter 调用（started/completed/failed） |
| Patch | 记录文件系统修改（file_write / unified_diff） |
| ValidationResult | 记录步骤级校验（passed/failed） |
| RuntimeCheckpoint | 记录恢复点（step_dispatched/step_completed/decision_blocked/approval_blocked） |

**Run 状态机**：
```
Created → Running → BlockedOnDecision → Running → ... → Validating → Completed
        → Running → BlockedOnApproval  → Running → ... → Validating → Completed
        → Failed / Cancelled / Superseded
```

- `isTerminalRunState()`：Completed、Failed、Cancelled、Superseded
- `canCancelRun()`：Created、Running、BlockedOnDecision、BlockedOnApproval、Validating
- `canSupersedeRun()`：Running、BlockedOnDecision、BlockedOnApproval、Validating
- `canFailRun()`：Created、Running、BlockedOnDecision、BlockedOnApproval、Validating

**服务接口**（16 个方法）：
```go
CreateRun / EnsureRun / StartRun / EnsureStarted
ResumeRun / EnsureResumed
BlockOnDecision / EnsureBlockedOnDecision
BlockOnApproval / EnsureBlockedOnApproval
ValidateRun / CompleteRun
CancelRun / SupersedeRun / FailRun
```

### 3.3 Soul 模块（权重 20）

**Soul**：{ID, Name, Description, BusinessDomains, SoulMD, AllowedSkillRefs, AllowedMCPServerRefs, AllowedCapabilityRefs, AllowedCapabilityCategories, AllowedToolKinds}

Soul 是治理实体，定义了对 workspace 执行运行的约束。可按 workspace 按阶段（planning / execution / decision / approval / validation / artifact）绑定不同的 Soul。验证规则确保所有被选中的 skill、MCP server、capability 都在 Soul 的允许列表中。

**服务接口**：
```go
CreateSoul / UpdateSoul / DeleteSoul / GetSoul / ListSouls
ValidateCapabilitySelection(SoulID, SkillIDs, MCPServerIDs, CapabilityIDs, Categories, ToolKinds) → error
```

### 3.4 Skill 模块（权重 20）

**Skill**：{ID, Name, Version, Description, TriggerRules, Instruction, ReferenceRefs, RequiredToolKinds, RequiredMCPServerRefs, RequiredCapabilityRefs, Constraints}

Skill 有两个来源：
1. **仓库管理型**（存储在 workorch 自身的数据库中）
2. **Skill-Hub 管理型**（外部 CLI 工具 `skill-hub`，支持 global 和 workspace 两种 scope）

Skill 引用使用 `skill-hub://` URI 格式，自动从对应 scope 解析。

### 3.5 Decision 模块（权重 20）

**Ticket**：{ID, WorkspaceID, RunID, StepID, Reason, DecisionKind, Question, Context, State}

**Resolution**：{ID, TicketID, Type, Payload}

Resolution 类型：
- `provide_missing_info`：将附加信息注入 LLM 执行的上下文
- `reject_current_path`：终止运行并标记为失败
- `replan`：废弃运行（supersede）以便重新规划

**服务接口**：
```go
CreateTicket / EnsureTicket
ResolveTicket / EnsureResolvedTicket / EnsureResolvedTicketAndReturn
```

### 3.6 MCP 模块（权重 28）

**Server**：{ID, Name, Transport, Endpoint, Enabled, AllowedTools, AllowedResources, AllowedPrompts, DefaultApprovalPolicy, SessionPolicy, TimeoutMillis}

**Capability**：{ID, ServerID, Kind, Name, DisplayName, Description, Category, Tags, UsageHint, InputSummary, OutputSummary, StageHints, RiskLevel, Schema, URI, Enabled, ApprovalPolicy, Status}

Transport 支持 `stdio`（子进程 stdin/stdout）和 `http`（JSON-RPC 2.0）。创建 Server 时会根据 AllowedTools/Resources/Prompts 自动生成 Default Capability。

### 3.7 Memory 模块（权重 20）

**MemoryItem**：{ID, WorkspaceID, RunID, SourceType, SourceRef, MemoryType, Content, Summary, Tags, Importance, Confidence, Visibility, EmbeddingProviderID, EmbeddingModel, EmbeddingText, Vector, VectorScore}

支持语义向量搜索（通过 embedding provider）。在 workspace 激活时通过 `selectedMemoryItems()` 检索相关记忆，并在 run 关闭时写入技能反馈记忆（skill feedback memory）。

### 3.8 Result 模块

**Result**：{ID, WorkspaceID, RunID, GoalID, DoDID, State, Title, Summary, SourceType, SourceRef, EvidenceRefs, ArtifactRefs, DoDAssessment, SummaryDetail}

Result 是 DoD 支持的验收交付物。只有通过 validated DoD assessment 后才能创建 accepted Result。State 可以是 draft / accepted / superseded。

**Artifact**：{ID, WorkspaceID, RunID, ResultID, StepID, ArtifactType, Category, Name, Summary, URI, ContentType, State, EvidenceRefs}

Artifact 是具体的交付物文件。Category 分为：source、configuration、documentation、test_report、runtime_evidence、final_summary。内部 URI 使用 `workorch://` 前缀，不暴露本地绝对路径。

### 3.9 EventStream 模块（权重 20）

**EventRecord**：{ID, Version, Topic, OccurredAt, SourceModule, SubjectType, SubjectID, WorkspaceID, RunID, StepID, Payload}

EventStream 是唯一具有 `Run()/Stop()` 生命周期的内核模块。它通过 `events.SubscribeWorkorch()` 订阅所有 18 个 workorch topic，将每个事件持久化为 EventRecord，支持按 topic、workspace、run、时间范围查询。

### 3.10 Orchestrator 模块（权重 30）

Orchestrator 是 workorch 的核心编排引擎。

**RuntimeAdapter 接口**：
```go
type RuntimeAdapter interface {
    ExecuteRuntimeStep(ctx, StepExecutionRequest) (StepExecutionResult, error)
}
```

**RuntimeOrchestrator 结构体**持有所有仓库和事件 Hub 的引用，负责完整的执行生命周期。

**Step 执行流程**（`dispatchStepWithWorkspace()`）：
1. 创建或更新 Step 记录
2. 使用执行上下文包丰富 LLM Step 输入（workspace manifest、results、deliverables、failures、governance history）
3. 发布 `TopicStepDispatched` 事件
4. 记录 ToolCall 证据（并行：ToolCall + RuntimeCheckpoint）
5. 通过 `runtime.ExecuteRuntimeStep()` 执行实际的 runtime adapter
6. 应用运行时 patches（file_write、unified_diff）
7. 记录内存写入（最多 8 个 MemoryItem 并行写入）
8. 将 Step 标记为 completed（并行：Step state + step_completed RuntimeCheckpoint）
9. 发布 `TopicStepStateChanged` 事件
10. 检查 Decision block → 创建 Ticket，阻塞 Run
11. 检查 Approval block → 创建 Approval，阻塞 Run
12. 调度 next step / graph follow-up / LLM completion follow-up / 关闭

**执行上下文包**（`execution_context_package.go`）：
每个 LLM Step 的上下文包含：
- `workspaceFileManifest`：workspace 目录中的文件列表
- `latestAcceptedResult`：最近一次已接受的 Result
- `currentDeliverables`：可用 Artifact（最多 8 个）
- `recentFailureSummary`：最近 Step 失败信息（最多 6 个）
- `selectedContext`：Skill、MCP Server、Capability、Memory 上下文
- `governanceHistory`：最近的 Decision Ticket 和 Resolution（最多 6 个）

上下文包版本化为 `workorch.execution_context.v1`。稳定上下文（Result、Deliverables、Workspace History）跨 Step 复用，避免重复 I/O。

**失败分类**（`failure_classification.go`）：
| 类别 | 说明 |
|------|------|
| `provider_unavailable` | LLM 服务不可用 |
| `provider_schema_drift` | LLM 输出格式不符合 schema |
| `provider_timeout` | LLM 调用超时 |
| `workspace_path_conflict` | 文件操作路径冲突 |
| `shell_command_failed` | Shell 命令执行失败 |
| `patch_apply_failed` | 补丁应用失败 |
| `mcp_capability_denied` | MCP 能力被拒绝 |
| `mcp_runtime_failed` | MCP 运行时失败 |

重复相同失败达到阈值（maxRuntimeRepairAttempts = 3）时触发 Decision 阻塞。

**已注册的 Step Kind**：
- `llm/workspace_execution`（默认初始 kind）
- `shell/command`、`ssh/command`
- `mcp/tool_call`、`mcp/resource_read`、`mcp/prompt_get`
- `patch/apply`

---

## 4. 运行时适配器模块（Blocks）

Blocks 模块为特定 Step Kind 实现 RuntimeAdapter 接口。

### 4.1 LLM Block（权重 16）

支持三种 Provider 类型：
- **OpenAI SDK**：通过 openai-go 调用 OpenAI 兼容 API
- **Anthropic SDK**：通过 anthropic-sdk-go 调用 Anthropic API
- **HTTP Provider**：直接的 HTTP/JSON 调用

核心执行流程：
1. 根据配置选择 Provider
2. 从 execution context 构建 system prompt
3. 序列化 LLM decision schema（`workorch.llmExecutionDecision.v1`）
4. 调用 Provider 完成
5. 解析 JSON 响应，验证 schema
6. 自动修复格式错误的决策（markdown 代码块剥离、JSON 修复）
7. 验证 shell 命令（runtime capability 边界、不可用命令列表）

Provider 配置在每次执行时加载，通过 revision 缓存避免重复构建。仅构造被选中的 Provider 客户端（lazy construction）。

**LLM Decision Schema**（`workorch.llmExecutionDecision.v1`）：
```json
{
  "completionState": "continue | blocked_on_decision | blocked_on_approval | completed | failed",
  "thoughtSummary": "...",
  "nextStep": { "kind": "...", "input": {...} },
  "requiredDecision": { "reason": "...", "decisionKind": "...", "question": "...", "context": {...} },
  "requiredApproval": { "reason": "...", "action": "...", "context": {...} },
  "executionGraph": { "nodes": [{ "id": "...", "kind": "...", "input": {...} }] },
  "dodAssessment": { "satisfied": true, "items": [...], "summary": "..." },
  "patches": [{ "targetRef": "...", "content": "...", "diff": "..." }],
  "memoryWrites": [{ "memoryType": "...", "content": "...", "tags": [...] }]
}
```

### 4.2 MCP Block（权重 29）

**Transport**：支持 stdio（子进程 stdin/stdout JSON-RPC 2.0）和 HTTP。

**Session 管理**：
- `transient`：每个请求新建子进程
- `long_lived`：子进程持久化连接，跨请求复用

**操作**：tool_call、resource_read、prompt_get，均通过 JSON-RPC 2.0 协议交互。

### 4.3 Approval Block（权重 20）

**Approval**：{ID, WorkspaceID, RunID, StepID, Reason, State}

State 转换：pending → approved / rejected / resolved

服务接口：`CreateApproval` / `Approve` / `Reject` / `EnsureApprovedAndReturn`，支持幂等操作。

### 4.4 Shell Block（权重 15）

通过 `os/exec` 在 workspace working directory 中执行 shell 命令。验证命令是否在允许列表中，防止执行禁用命令。

### 4.5 SSH Block（权重 16）

通过 SSH 在远程主机上执行命令。配置包含 host、port、user、auth 方式。

---

## 5. 应用层

### 5.1 启动流程

1. `main.go` 解析 `-config-dir` 参数
2. 调用 `app.Run()` 启动 magicCommon 生命周期
3. 框架按权重顺序初始化所有模块
4. `bootstrap.NewFromEnv()` 构建所有依赖（仓库、服务、事件 Hub）
5. HTTP Server 启动并监听 `:8080`

### 5.2 App Service 层

| 服务文件 | 职责 |
|----------|------|
| `runtime_session.go` | 运行时会话生命周期管理 |
| `workspace_execution_activation_service.go` | Workspace 执行激活（选择 Goal/DoD、构建执行上下文、创建启动 Run） |
| `workspace_soul_routing.go` | Soul 路由与阶段绑定 |
| `skill_selection_service.go` | Skill 自动选择（关键词评分算法） |
| `skill_hub_service.go` | 与外部 `skill-hub` CLI 集成 |
| `context_registry_service.go` | 统一 CRUD（Soul、Skill、MCP Server/Capability、Memory） |
| `read_model_service.go` | Run/Workspace 快照、状态、可观察性、任务进度 |
| `run_service.go` | Run 创建、启动、恢复 |
| `decision_service.go` | Decision Ticket 创建、批量解析、列表 |
| `approval_service.go` | Approval 创建、批量解析、列表 |
| `workspace_service.go` | Workspace 列表、详情、模板管理 |
| `run_recovery_service.go` | Run 恢复与失败处理 |

**Runtime 接口层次**：
```
SnapshotReadRuntime  → GetWorkspaceByID / GetRun / GetStep / GetTicket / GetApproval
ActionRuntime        → EnsureWorkspace / EnsureRun / EnsureTicket / EnsureApproval / ...
Runtime              → SnapshotReadRuntime | ActionRuntime
```

### 5.3 HTTP 路由（`/api/v1/workorch/`）

**系统路由**：`/healthz`、`/readyz`、`/metrics`、配置 CRUD

**读取路由**（50+ 端点）：
- `GET /workspaces`、`GET /workspaces/:id`、`GET /workspaces/:id/snapshot`
- `GET /runs/:id`、`GET /runs/:id/snapshot`、`GET /runs/:id/status`、`GET /runs/:id/observability`
- `GET /souls`、`GET /souls/:id`
- `GET /skills`、`GET /skills/:id`
- `GET /mcp/servers`、`GET /mcp/servers/:id`、`GET /mcp/capabilities`
- `GET /memory`、`GET /memory/:id`
- `GET /decisions`、`GET /decisions/:id`、`GET /approvals`、`GET /approvals/:id`
- `GET /events`、`GET /artifacts/content`

**写入路由**（30+ 端点）：
- `POST /workspaces`、`POST /workspaces/:id/goals`、`POST /workspaces/:id/dods`
- `POST /workspaces/:id/execution-activations`
- `POST /runs/:id/start`、`POST /runs/:id/resume`
- `POST /decisions/:id/resolve`、`POST /decisions/batch-resolve`
- `POST /approvals/:id/approve`、`POST /approvals/:id/reject`、`POST /approvals/batch-resolve`
- `PUT /configuration/llm`、`PUT /configuration/server`
- `DELETE /souls/:id`、`DELETE /skills/:id`、`DELETE /mcp/servers/:id`

**Run 状态等待路由**：`GET /runs/:id/status?waitState=running&waitTimeoutMillis=120000`，支持服务端等待（长轮询）以避免客户端轮询。

---

## 6. 仓库层

### 6.1 接口（22 个）

每个实体类型一个接口，遵循标准 CRUD 模式：
```go
type XxxRepository interface {
    Create(ctx, *model) error
    Get(ctx, ID) (*model, error)
    Update(ctx, *model) error
    Delete(ctx, ID) error
    List(ctx) ([]*model, error)
    // 实体特定查询方法
}
```

### 6.2 内存后端

基于 `map[ID]*Model` 的并发安全实现，使用 `sync.RWMutex`。主要用于测试。

### 6.3 文件后端

数据布局（`{dataRoot}/`）：
```
system/                         # 系统级实体
├── souls/{soulId}/soul.md     # Soul 的 markdown 定义
├── skills.json                # 系统级 Skill
├── mcp_servers.json           # MCP Server
├── mcp_capabilities.json      # MCP Capability
├── workspace_templates.json   # Workspace 模板
├── workspaces.json            # Workspace 索引
└── event_records.json         # 事件记录

workspaces/{workspaceId}/      # Workspace 级实体（按 workspace 隔离）
├── goals.json
├── dods.json
├── runs.json
├── steps.json
├── tool_calls.json
├── patches.json
├── validation_results.json
├── runtime_checkpoints.json
├── results.json
├── artifacts.json
├── tickets.json
├── resolutions.json
├── approvals.json
└── memory_items.json
```

**缓存**：`mapFileCache`（`sync.Map`）缓存加载的 JSON 文件。先从缓存读取（Rlock），命中则返回 clone；未命中则从磁盘加载（Lock），写入缓存。

### 6.4 性能优化（QC1-QC101）

所有独立仓库读取已并行化，详见 `docs/run-closure-quality-improvement-plan.md` 和 `docs/single-workspace-performance-optimization-record.md`。

---

## 7. 事件系统

**18 个 Topic**：
```
workspace.created / workspace.state_changed / workspace.deleted
goal.defined / dod.defined
run.created / run.state_changed
step.dispatched / step.state_changed
ticket.created / ticket.resolved
approval.created / approval.resolved
result.accepted / artifact.created
```

**Envelope 结构**：{ID, Version, Topic, OccurredAt, SourceModule, SubjectType, SubjectID, WorkspaceID, RunID, StepID, Payload}

**MemoryHub**：基于 `sync.RWMutex` 的内存发布/订阅实现，支持 `"*"` 通配符订阅。

**类型化事件**：
```
WorkorchEvent 接口
├── WorkspaceEvent  { Created / StateChanged / Deleted }
├── RunEvent        { Created / StateChanged }
├── GoalEvent       { Defined }
├── DoDEvent        { Defined }
├── StepEvent       { Dispatched / StateChanged }
├── DecisionTicketEvent { Created / Resolved }
├── ApprovalEvent   { Created / Resolved }
├── ResultEvent     { Accepted }
└── ArtifactEvent   { Created }
```

---

## 8. 治理流程（Governance Flow）

### 8.1 Decision 流程

```
LLM Step 输出
  → requiredDecision: { reason, decisionKind, question, context }
  → Orchestrator 创建 DecisionTicket（state: open）
  → Run 转换为 BlockedOnDecision
  → 外部用户/系统提供 Resolution
  → Resolution 类型决定后续动作：
    - provide_missing_info: 注入上下文，继续执行
    - reject_current_path: 标记 Run 为 Failed
    - replan: 标记 Run 为 Superseded
```

### 8.2 Approval 流程

```
LLM Step 输出
  → requiredApproval: { reason, action, context }
  → Orchestrator 创建 Approval（state: pending）
  → Run 转换为 BlockedOnApproval
  → 外部用户/系统 Approve 或 Reject
  → Approved: 继续执行
  → Rejected: 标记 Run 为 Failed
```

### 8.3 Soul 治理

```
Workspace Activation
  → resolveWorkspaceSoulRouting()
  → 按阶段绑定 Soul（planning / execution / decision / approval / validation / artifact）
  → 验证所有选中的 Skill/MCP/Capability 是否符合 Soul 的允许列表
  → validateSoulCapabilitySelection() 检查每个选中项
```

---

## 9. Skill 自动选择

当未提供显式 Skill ID 时：
1. 列出所有可用 Skill（仓库 + skill-hub）
2. 如果绑定了 Soul，按 Soul.AllowedSkillRefs 过滤
3. 按关键词匹配 Goal + DoD 文本打分
4. 按分数降序排列，选前 5 个（maxAutoSelectedSkills）

MCP Server 和 Capability 通过 `skill_required_mcp_auto_bind` 策略从 Skill 的 RequiredMCPServerRefs 和 RequiredCapabilityRefs 自动绑定。

---

## 10. Web Console

基于 React 19 + antd 5 + ProComponents 2.8 的单页应用。

**6 个主视图**：
| 视图 | 功能 |
|------|------|
| `board` | 业务总览、Workspace 列表、运行状态 |
| `workspaceDetail` | Workspace 详情、Goal/DoD 管理、执行激活、Timeline |
| `governance` | Decision/Approval 队列处理 |
| `runtimeControl` | Run 控制（启动、恢复、取消） |
| `configuration` | 系统配置（LLM Provider、MCP Server、Capability、Soul、Skill、Memory） |
| `service` | 服务状态监控 |

---

## 11. 关键设计决策

### 11.1 内核与业务通用性

Kernel/Orchestrator 不解析具体 Goal 文本来决定行为。所有业务特定逻辑通过以下方式配置：
- Soul：治理约束（允许的 Skill/MCP/Tool）
- Skill：执行指令和工具要求
- DoD：验收条件

### 11.2 Result 与中间证据分离

ToolCall、Patch、ValidationResult 是运行时证据记录，不替代正式的 Result/Artifact。只有通过 DoD 评估门控后才能创建 accepted Result。历史失败的修复步骤仍可见于证据，但不会被 accepted Result 覆盖。

### 11.3 统一结果投影

RunOutcome 和 WorkspaceOutcome 投影统一用于 API 和 Web：
- 一个拥有最新失败 Run 但后续有 accepted Result 的 Workspace 不会显示最终结果为 failed
- 拥有 accepted Result 的 Run 即使之前有修复失败也显示为 accepted
- 已完成但没有 accepted Result 的 Run 明确显示为 incomplete

### 11.4 幂等性

所有 Ensure* 方法支持幂等操作，允许安全重试。例如 `EnsureResolvedTicketAndReturn` 对已解析的 ticket 返回已有 resolution，不报错。

### 11.5 LLM 决策 Schema 强制

所有 LLM 输出必须是严格的 JSON 对象（`workorch.llmExecutionDecision.v1`）。不允许 markdown、代码块、解释文本。自动修复层处理常见格式错误，但 schema 违规最终会被拒绝并触发修复流程。
