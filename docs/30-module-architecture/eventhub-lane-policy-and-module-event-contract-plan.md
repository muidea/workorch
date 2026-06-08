# EventHub Lane Policy and Module Event Contract Plan

Status: active

Type: migration-plan

Last Updated: 2026-06-08

## Purpose

本文用于固化 WorkOrch 在 `github.com/muidea/magicCommon/framework` 与 `github.com/muidea/magicCommon/event` 规范下的 EventHub lane 使用规则、module/block 事件契约边界和后续逐项收口计划。

当前阶段不考虑兼容性。若现有 topic、payload、lane 或 helper 设计与本文规则冲突，应以本文作为后续代码收口依据。

本文补充并修正 `docs/60-verification/framework-eventhub-module-boundary-checklist.md` 中旧的 lane 口径。旧口径要求 run-scoped 事件携带 `RunID` 时优先进入 `run/<runID>` lane；该规则会把读查询、状态观察、CLI facade、block 执行等不属于同一状态一致性边界的事件错误串行化。新规则要求 lane 表达一致性边界，而不是直接表达业务对象 ID。

## Problem Statement

WorkOrch 之前在 `internal/pkg/events.MagicHub` 中通过 `Envelope.RunID` / `Envelope.WorkspaceID` 推断 lane：

- 有 `RunID` 时进入 `run/<runID>` lane。
- 没有 `RunID` 但有 `WorkspaceID` 时进入 `workspace/<workspaceID>` lane。
- 否则退回 topic lane。

这个策略在早期能保持 run 状态机串行，但它把所有带 run/workspace scope 的事件都视为同一类状态写入，导致以下问题：

- snapshot、status、list steps、read model 等查询在长时间 run 执行 lane 后排队。
- TUI / CLI facade 消息携带 run/workspace scope 时被错误绑定到 run lane。
- block/module 的无状态执行或只读能力被不必要地串行化。
- owner command handler 内部同步查询其他 owner 时，容易出现同 lane 等待同 lane 的阻塞形态。
- Hub 通过字符串包含关系推断业务语义，使业务契约散落在通用派发层。

本轮要修正的核心误区是：lane 不是业务作用域，lane 是一致性边界。`RunID`、`WorkspaceID`、`StepID` 是事件 scope，不应天然决定串行队列。

## Reference Pattern

参照 `/home/rangh/codespace/magicRunner/internal/modules/kernel/panel/biz`：

- module 在自身构造和运行过程中注册需要消费的事件。
- `SubscribeFunc` 由模块内部调用，订阅哪些 topic 属于模块自己的契约。
- 事件发布由模块方法内部通过 `SendEvent` / `PostEvent` 完成。
- topic、payload、header、destination、sync/async 语义由业务模块自己约定。
- EventHub 负责传递事件，不负责理解每个业务 topic 的状态语义。

WorkOrch 可以继续保留自身更严格的 owner-state command/response 规则，但要采用同样的边界原则：模块负责定义自己的事件契约，Hub 只负责按契约派发。

## Unified Rules

### EventHub

- Hub 只负责事件分发、订阅、同步/异步映射、基础 envelope 准备和 lane 执行。
- Hub 不应通过 `RunID` / `WorkspaceID` 自动判断事件是否需要状态串行。
- Hub 不应长期保留基于 topic 字符串包含关系的业务推断，例如 `contains("/command/get")`。
- Hub 可以提供 lane policy registry 或 contract resolver，但 policy 应来自明确注册的事件契约。
- 缺失契约时可以采用保守 fallback，但 fallback 只能作为迁移期保护，不能作为长期业务规则来源。
- `internal/pkg/events` 只保留通用 EventHub wrapper、Envelope、Response、通用 payload helper 和 contract registry 聚合；业务 topic 不再由该包定义。

### Module / Block

- 每个 owner module 在 module 内部订阅自己的 command topic，并在 `Teardown` / `Stop` 释放订阅。
- event command topic、notification topic、command DTO、query DTO、response/data view DTO 必须由业务模型 owner module/block 的 `pkg` 包定义。
- 每个 command topic 只表达一个清晰意图，例如 get/list/create/update/transition/execute/notify。
- 跨 owner 读写正式状态必须走同步 EventHub command，并要求返回 `events.Response`。
- owner module 在 handler 内把公开 payload 翻译为自身 service/repository 操作；调用方不能直接注入另一个 owner service 或 repository。
- block 只执行 runtime 技术能力或外部系统交互，不拥有正式 WorkOrch 状态。
- 无状态 block/module 执行不应因为携带 `RunID` 就进入 run 状态 lane。
- 只有状态 owner 写入、状态机转换、formal evidence 接受/拒绝、同一资源互斥操作等需要严格串行。

### Event Definition Ownership

事件定义按“被操作业务模型的 owner”归属，而不是按发布方归属。发布方只能 import owner 的 `pkg/events`、`pkg/contracts`、`pkg/models`，不能在 `internal/pkg/events` 新增业务 topic。

| 业务模型 / 能力 | owner | topic / command / view 定义落点 |
| --- | --- | --- |
| Workspace / Goal / DoD / WorkspaceTemplate | `kernel/workspace` | `internal/modules/kernel/workspace/pkg/events` + `pkg/contracts` + `pkg/models` |
| Run / Step / RuntimeCheckpoint | `kernel/run` | `internal/modules/kernel/run/pkg/events` + `pkg/contracts` + `pkg/models` |
| Result / Artifact | `kernel/result` | `internal/modules/kernel/result/pkg/events` + `pkg/contracts` + `pkg/models` |
| Decision Ticket / Resolution | `kernel/decision` | `internal/modules/kernel/decision/pkg/events` + `pkg/contracts` + `pkg/models` |
| Approval | `kernel/approval` | `internal/modules/kernel/approval/pkg/events` + `pkg/contracts` + `pkg/models` |
| Memory / MemoryRecallTrace | `kernel/memory` | `internal/modules/kernel/memory/pkg/events` + `pkg/contracts` + `pkg/models` |
| Soul / Skill / MCP catalog | `kernel/soul`, `kernel/skill`, `kernel/mcp` | 各 owner 的 `pkg/events` + `pkg/contracts` + `pkg/models` |
| EventStream / Governance / Orchestrator evidence | 对应 kernel owner | 各 owner 的 `pkg/events` + `pkg/contracts` / `pkg/models` |
| Runtime block execute | `blocks/pkg/runtimeevent` and each block | `internal/modules/blocks/pkg/runtimeevent` + block `pkg` |
| LLM provider command | `blocks/llm` | `internal/modules/blocks/llm/pkg/events` + `pkg/models` |
| CLI/TUI facade | `application/workorchd` or process port | `internal/modules/application/workorchd/pkg/events` or `internal/pkg/workorchport` |

数据视图规则：

- owner module 的 `pkg/models` 定义正式业务模型和 read view。
- owner module 的 `pkg/contracts` 定义 command DTO、query DTO、response DTO。
- owner module 的 `pkg/events` 定义 topic 常量、event contract metadata、owner 专属 payload helper。
- 通用 `Envelope`、`Response`、`PayloadKeyCommand`、`PayloadKeyItems` 等机制字段可以留在 `internal/pkg/events`。
- 跨 owner 聚合 view 应落在聚合 owner 或 application read model focused package，不能反向塞回 EventHub 基础包。

### Sync / Async

- 需要结果、错误、状态机裁决或强一致反馈的 command 使用同步 `Send` / `Publish`。
- 纯通知、progress、UI refresh、observability stream 使用异步 `Post` / `PublishAsync`。
- 长耗时操作不应直接在 EventHub observer 中执行；应转交 framework 统一管理的 `BackgroundRoutine` 或运行期 executor。
- 同步 command handler 内不得等待同一 lane 上的新同步事件完成。

## Lane Taxonomy

lane key 必须表达一致性或资源竞争边界。建议统一使用以下命名：

| Lane | 用途 | 示例 |
| --- | --- | --- |
| `state/run/<workspaceId>/<runId>` | run owner 正式 Run 记录写入和 formal evidence 绑定；`runId` 是 workspace-local，必须携带 workspace scope | create/update/delete run |
| `state/orchestrator/workspace/<workspaceId>/run/<runId>` | orchestrator 执行推进、resume/recovery/closeout 等编排状态；不得与 run owner lane 共队列，否则 handler 内同步回写 run owner 会自锁 | start/resume/fail/complete run |
| `state/workspace/<workspaceId>` | workspace owner 状态写入、goal/dod/workspace 变更 | create workspace, update goal |
| `state/<owner>/workspace/<workspaceId>/<subject|run>/<id>` | 其他 owner 正式状态写入；workspace-local subject 必须带 workspace scope | `state/approval/workspace/ws-a/subject/approval-001` |
| `query/<topic>/<scope>` | owner read/query，不阻塞状态写 lane；读取 workspace-local run/step 时必须带 workspace scope | `query/run/command/list-steps-by-run/workspace/ws-a/run/run-001` |
| `exec/block/<kind>/workspace/<workspaceId>/step/<stepId>` | block 执行互斥或按 step 独立执行；`stepId` 也是 workspace-local 时必须携带 workspace scope | `exec/block/mcp-tool-call/workspace/ws-a/step/run-001-step-2` |
| `resource/<type>/<id>` | 外部资源互斥，例如文件、patch、workspace deliverable | `resource/file/<workspaceId>/<path>` |
| `topic/<topic>` | facade、通知、progress、无需业务串行的 topic | CLI result/progress |
| `global/<owner>` | 全局单例状态或启动治理 | runtime config update |

### 禁止规则

- 禁止把 `run/<runId>` 作为所有 run-scoped 事件的默认 lane。
- 禁止把 `workspace/<workspaceId>` 作为所有 workspace-scoped 事件的默认 lane。
- 禁止在 run/orchestrator state lane 中只使用 workspace-local `runId`；必须使用 `workspaceId + runId`，否则不同 workspace 的 `run-001` 会被错误串行。
- 禁止让 orchestrator 执行推进 command 与 run owner 写 command 共用同一个 lane；orchestrator 可按 workspace/run 串行，但 lane owner 必须是 `orchestrator`。
- 禁止在 query/execute lane 中只使用 workspace-local `runId` 或 `stepId`；只要 envelope 有 `workspaceId`，lane 必须带 workspace scope。
- 禁止只靠 topic 字符串片段推断 read/write/execute/notify。
- 禁止让 CLI/TUI facade 消息与 run 状态机写入共用同一 lane。

## Event Contract Shape

每个跨 module/block topic 都应能登记或描述以下信息：

| 字段 | 说明 |
| --- | --- |
| `topic` | 稳定 topic，例如 `/run/command/get` |
| `owner` | 负责订阅和处理该 topic 的 module/block |
| `publisher` | 允许发布该 topic 的模块、adapter 或 helper |
| `deliveryMode` | `sync` 或 `async` |
| `commandClass` | `read` / `write` / `execute` / `notify` |
| `lanePolicy` | lane 计算策略，不由裸 scope 自动推断 |
| `requiredScope` | 必须携带的 `workspaceId`、`runId`、`stepId`、subject 等 |
| `payloadContract` | payload key 或 DTO 约定 |
| `responseContract` | 同步 response payload、错误语义、缺失 response 处理 |
| `lifecycle` | owner 在 `Run` 订阅，在 `Teardown` 释放 |

推荐逐步在 `internal/pkg/events` 中引入显式契约注册，例如：

```text
TopicRunCommandGet
  owner: kernel/run
  deliveryMode: sync
  commandClass: read
  lanePolicy: query by run scope
  requiredScope: workspaceId optional for scoped verification, runId required
  response: run model or not found error

TopicOrchestratorCommandStartRun
  owner: kernel/orchestrator
  deliveryMode: sync
  commandClass: write
  lanePolicy: state/orchestrator/workspace/<workspaceId>/run/<runId>
  requiredScope: workspaceId, runId
  response: orchestrator command response
```

## Adjustment Scope

### `internal/pkg/events`

- 引入 event contract / lane policy registry。
- 将 lane 计算从隐式 `RunID` / `WorkspaceID` fallback 迁移到显式 contract。
- 保留迁移期 fallback，但增加测试和 TODO，确保新增 topic 必须声明契约。
- 移除长期存在的 `strings.Contains(topic, "/command/get|list|search")` read 推断。

### `internal/pkg/events/topics_*`

- 当前这些文件是待迁移旧结构。
- topic 常量应移动到 owner module/block 的 `pkg/events` 包。
- 迁移期可以在 `internal/pkg/events` 中保留 alias 以降低单轮改动风险；最终 `internal/pkg/events/topics_*.go` 应删除或只保留非业务通用 topic。
- contract registry 可以聚合各 owner `pkg/events.Contracts()` 的结果，但不能直接定义 owner topic。

### Owner Command / Read Publishers

涉及：

- `internal/modules/application/workorchd/biz/appservice/ownercommandports`
- `internal/modules/application/workorchd/biz/appservice/ownerreadports`
- 其他 EventHub-backed owner adapter

要求：

- 写入/状态机 helper 使用 state lane contract。
- read/list/search helper 使用 query lane contract。
- helper 应检查 `events.Response`，缺失 response 视为 lifecycle wiring failure。
- helper 不导入 owner service command 类型作为跨 module payload。

### Runtime Block Execution

涉及：

- `internal/modules/blocks/pkg/runtimeevent`
- `internal/modules/blocks/*`
- `internal/modules/kernel/orchestrator/*runtime*`

要求：

- block execution topic 使用 execute contract。
- 无状态 block 执行使用 `exec/block/<kind>/workspace/<workspaceId>/step/<stepId>` 或更精确资源 lane；当 `stepId` 不是全局唯一时必须携带 workspace scope。
- 需要外部资源互斥的 block 使用 `resource/<type>/<id>` lane。
- block 不进入 run state lane，除非它实际拥有 run 状态，这种情况原则上不应发生。

### Kernel Modules

涉及：

- `internal/modules/kernel/run`
- `internal/modules/kernel/workspace`
- `internal/modules/kernel/result`
- `internal/modules/kernel/decision`
- `internal/modules/kernel/approval`
- `internal/modules/kernel/governance`
- `internal/modules/kernel/orchestrator`
- `internal/modules/kernel/eventstream`
- `internal/modules/kernel/soul`
- `internal/modules/kernel/skill`
- `internal/modules/kernel/mcp`
- `internal/modules/kernel/memory`

要求：

- owner module 内部完成订阅和发布。
- owner 写入和状态机转换只在 owner state lane 串行。
- owner read/query 不与写 lane 共享队列。
- governance/orchestrator 的长耗时 continuation 使用 framework `BackgroundRoutine`，不阻塞 observer。

### Application / Facade

涉及：

- `internal/services/workorch`
- `internal/pkg/workorchport`
- `internal/pkg/workorchclient`
- `internal/modules/application/workorchd`

要求：

- CLI/TUI/remote/server facade 不直接拥有正式状态。
- CLI command/result/progress 使用 topic lane 或 facade contract，不因 run/workspace scope 进入 state lane。
- REST/CLI/TUI 参数必须提供足够 workspace/run scope；缺失 workspace 的 run 执行应直接失败并提示 workspace 缺失。

### Docs / Skills

迁移完成后同步：

- `docs/60-verification/framework-eventhub-module-boundary-checklist.md`
- `docs/30-module-architecture/framework-eventhub-structure-migration-plan.md`
- `go-application-event-runtime`
- `go-multi-module-dev`
- 必要时补充 `go-module-initiator-lifecycle`

## Migration Phases

### Phase 1: Document and Audit

目标：

- 完成本文档。
- 扫描所有 `Publish` / `PublishAsync` / `Subscribe` / runtime block execute 调用点。
- 输出 topic 分类清单：read/write/execute/notify/facade。
- 标记仍依赖隐式 lane 推断的 topic。

验收：

- 有一份可执行的 topic/lane 审计清单。
- 明确第一批必须调整的 topic。

### Phase 2: Event Contract and Lane Policy Registry

目标：

- 在 `internal/pkg/events` 引入显式 lane policy。
- topic contract 能根据 `Envelope` 计算 lane。
- Hub 调用 contract resolver，而不是直接按 `RunID` / `WorkspaceID` 推断。

验收：

- read command 不进入 state run lane。
- write command 进入对应 state lane。
- facade topic 进入 topic lane。
- 未声明 topic 的 fallback 有测试覆盖和日志/注释提醒。

### Phase 3: Publishing Helper Split

目标：

- owner read helper 与 owner write/helper 的 lane policy 明确分离。
- application/appservice 只通过稳定 EventHub-backed port 访问 owner state。
- command helper 统一检查 `events.Response`。

验收：

- `ownerreadports` 发布的 command 都是 query lane。
- `ownercommandports` 发布的状态变更都是 state lane。
- 生产路径不存在跨 owner service/repository 直接访问。

### Phase 4: Block Execution Policy

目标：

- runtime block execution 使用 execute/resource lane。
- LLM/MCP/Shell/File/Git/Web/SSH 等 block 不默认进入 run state lane。
- orchestrator 只消费 block evidence，并由自身或 owner module 完成正式状态写入。

验收：

- block execution contract 有单元测试。
- 长耗时执行不会阻塞 status/snapshot/read command。

### Phase 5: Verification, Docs, and Skill Refresh

目标：

- 跑全量单元测试和 e2e。
- 更新 checklist 和 skill。
- 移除迁移期临时 topic 字符串推断。

验收命令：

```bash
go test ./... -count=1
make verify-managed-full
```

验收场景：

- 长时间 run 执行期间，snapshot/status/list/read query 不被 run state lane 阻塞。
- 同一个 run 的 start/resume/fail/complete 等状态机转换仍保持串行。
- block 执行可按 step/resource 并发或互斥，不与 run state owner 混用 lane。
- CLI/TUI facade 的 command/result/progress 不被 run state lane 卡住。
- 无 workspace 的 run 执行直接返回 workspace 缺失并失败。

### Phase 6: Owner-Owned Event Definition Split

目标：

- 按业务模型 owner module/block 建立 `pkg/events` 包。
- 将 `internal/pkg/events/topics_*.go` 中的业务 topic 常量移动到 owner `pkg/events`。
- owner `pkg/events` 暴露自身 topic contract 列表，供 `internal/pkg/events` 聚合注册。
- 发布方逐步改为 import owner event package，例如 `runevents.TopicCommandGet`、`workspaceevents.TopicCommandCreate`。
- `internal/pkg/events` 最终只保留 Hub/Envelope/Response 和 contract registry 聚合，不再承载业务 topic 定义。

验收：

- 新增业务 command/topic 时只能在 owner module/block 的 `pkg/events` 中定义。
- `rg "Topic[A-Za-z].*=\\s*\\\"/" internal/pkg/events -g 'topics_*.go'` 不再出现 owner 业务 topic 定义。
- owner module 的 topic、command DTO、read view DTO 在同一 owner `pkg` 族内可追踪。
- appservice、blocks、orchestrator 等调用方 import 业务 owner `pkg/events`，不再通过 `internal/pkg/events` 获取业务 topic。

## Initial Closure Checklist

- [x] 输出本迁移文档。
- [x] 扫描并生成 topic 分类清单。
- [x] 将 `internal/pkg/events` 的 lane 计算改为显式 contract registry。
- [x] 将 read/write/execute/notify/facade topic 逐项登记 contract。
- [x] 调整 owner read/write publisher helper。
- [x] 调整 runtime block execution lane policy。
- [x] 更新旧 checklist 中过期 lane 规则。
- [x] 更新相关 skill。
- [x] Phase 6: 按 owner module/block 拆分 event topic 定义。
- [x] Phase 6: 按 owner module/block 拆分 event contract metadata。
- [x] Phase 6: 调用方 imports 从 `internal/pkg/events.Topic*` 迁移到 owner `pkg/events.Topic*`。
- [x] Phase 6: 删除或清空 `internal/pkg/events/topics_*.go` 中的 owner 业务 topic 定义。
- [x] 执行 `go test ./... -count=1`。
- [x] 执行 `make verify-managed-full`。

## Current Closure Record

### 2026-06-08 Phase 1 / Phase 2

已完成：

- 新增 `internal/pkg/events/event_contract.go`，集中定义 `EventHubContract`、`EventCommandClass` 和 `EventLanePolicy`。
- `MagicHub` 已改为通过 `EventHubLaneKey(envelope)` 调用契约解析器，不再直接根据 `RunID` / `WorkspaceID` 推断 lane。
- runtime block execute prefix 由 contract resolver 识别，lane 在有 workspace scope 时进入 `exec/block/<kind>/workspace/<workspaceID>/step/<stepID>`。
- CLI facade topic 进入 topic lane。
- owner read command 进入 query lane。
- owner write/state command 进入 state lane。
- scoped notification 进入 topic lane。
- `docs/60-verification/framework-eventhub-module-boundary-checklist.md` 已替换旧的 run/workspace 自动 lane 规则。

直接验证：

```bash
go test ./internal/pkg/events -count=1
```

待继续：

- 完成全量 `go test ./... -count=1` 和 `make verify-managed-full` 后再更新 skill。

### 2026-06-08 Phase 3 / Phase 4

已完成：

- `ownercommandports` 的 publish helper 增加 write contract 校验，只允许 `EventCommandClassWrite` topic。
- `ownerreadports` 不再复用 `ownercommandports` 发布函数，已独立发布 read command 并校验 `EventCommandClassRead`。
- 补充 read/write helper 误用回归测试。
- 补充 `runtimeevent.Executor` 回归测试，确认同一个 run 上 state write lane 阻塞时，runtime block execute 仍可通过 workspace-scoped execute lane 完成。

直接验证：

```bash
go test ./internal/modules/application/workorchd/biz/appservice/ownercommandports ./internal/modules/application/workorchd/biz/appservice/ownerreadports -count=1
go test ./internal/pkg/events ./internal/modules/blocks/pkg/runtimeevent -count=1
```

### 2026-06-08 Phase 6 Topic Definition Split

已完成：

- 为业务模型 owner 建立 `pkg/events/topics.go`：
  - `kernel/workspace`
  - `kernel/run`
  - `kernel/result`
  - `kernel/decision`
  - `kernel/approval`
  - `kernel/memory`
  - `kernel/soul`
  - `kernel/skill`
  - `kernel/mcp`
  - `kernel/eventstream`
  - `kernel/governance`
  - `kernel/orchestrator`
  - `blocks/llm`
  - `application/workorchd`
- runtime block execute topic 下沉到 `internal/modules/blocks/pkg/runtimeevent/eventdefs`，避免 `internal/pkg/events` 与 `runtimeevent` 循环依赖。
- `internal/pkg/events/topics_*.go` 已删除；`internal/pkg/events` 不再暴露 owner 业务 topic alias。

直接核对：

```bash
rg -n "Topic[A-Za-z0-9]+\\s*=\\s*\\\"/" internal/pkg/events -g 'topics_*.go'
find internal/modules -path '*/pkg/events/topics.go' -type f | sort
```

已完成：

- 调用方 import 已迁移到 owner `pkg/events`，生产代码和测试均不再通过 `internal/pkg/events.Topic*` 获取业务 topic。
- `internal/pkg/events/topics_*.go` alias 文件已删除。
- `internal/pkg/events` 内部的 WorkOrch event parser/version matrix 直接引用 owner `pkg/events`。
- runtime block execute topic 保持在 `internal/modules/blocks/pkg/runtimeevent/eventdefs`，由 `runtimeevent.RuntimeBlockExecuteTopic` 对外暴露，避免 runtimeevent 与基础 events 包循环依赖。

直接核对：

```bash
rg -n "\bevents\.Topic[A-Za-z0-9]+" internal application -g '*.go'
rg -n "Topic[A-Za-z0-9]+\s*=\s*\"/" internal/pkg/events -g '*.go'
go test ./... -count=1
```

结果：

- `events.Topic*` 引用清零。
- `internal/pkg/events` 中直接 topic 字符串定义清零。
- `go test ./... -count=1` 通过。
- `make verify-managed-full` 通过，managed WorkOrch service 自动启动在临时端口并完成 full e2e 验证后退出。

### 2026-06-08 Phase 6 Contract Metadata Split

已完成：

- 新增 `internal/pkg/eventcontract`，承载不依赖 EventHub 的 command class、lane policy 和 contract 基础类型。
- 各 owner `pkg/events` 已提供 `Contracts() []eventcontract.Contract`。
- `internal/pkg/events/event_contract.go` 已改为聚合 owner `Contracts()`，不再集中硬编码各业务 topic 分类。
- CLI facade topic 已下沉到 `internal/modules/application/workorchd/pkg/events`；通用 `cli_events.go` 只保留 CLI payload 类型和 command name 常量，不再暴露 topic alias。

直接核对：

```bash
rg -n "func Contracts\\(\\) \\[\\]eventcontract\\.Contract" internal/modules -g 'contracts.go'
rg -n "Topic[A-Za-z0-9]+\\s*=\\s*\\\"/" internal/pkg/events -g '*.go'
```

结果：

- owner `pkg/events` contract 聚合完成。
- `internal/pkg/events` 不再定义或 alias 业务 topic。
- `go-application-event-runtime` 与 `go-multi-module-dev` 已补充 owner-owned event definition、基础 events 包不得承载业务 topic、runtime block execute topic 归属等规则。
