# WorkOrch MVVP Workspace ViewModel Architecture

Status: active

Type: module-architecture

Last Updated: 2026-06-09

## Purpose

本文固化 WorkOrch 在 `magicCommon/framework` 与 `EventHub` 规范下的
MVVP 架构设计，作为后续 workorchd、Web、TUI 和相关 read-model
代码收口的依据。

本文重点回答：

- WorkOrch 的 Model、View、ViewModel、Presenter 分别是什么
- workspace 状态投影的 owner 是谁
- workorchd 启动后应如何全量加载并持续维护所有 workspace 状态
- Web 和 TUI 如何消费统一 ViewModel
- REST、SSE、EventHub、BackgroundRoutine 的职责边界
- 当前实现收口状态与仍需遵守的边界
- 后续代码收口的验收清单

## Design Goal

WorkOrch 的最终设计预期是：

- `workorchd` 是 workspace 状态投影和实时状态管理的唯一服务端 owner。
- `workorchd` 启动完成后自动全量加载所有 workspace，并维护可直接展示的
  ViewModel 投影。
- `web` 和 `tui` 只展示 `workorchd` 整理后的状态信息，不自行聚合业务状态。
- `web` 通过 REST API 交互。
- `tui` 通过流式事件和轻量 REST/read-model 查询交互。
- 所有正式业务状态写入、状态机转换和 evidence 接受仍由 kernel owner module
  通过 EventHub command 完成。

## MVVP Definition

WorkOrch 中的 MVVP 定义为：

| Layer | WorkOrch 含义 | 代码归属 |
| --- | --- | --- |
| Model | 正式业务状态 owner，包括 workspace、run、decision、approval、result、memory、soul、skill、mcp、eventstream 等 | `internal/modules/kernel/*` |
| ViewModel | 服务端只读状态投影，面向 Web/TUI/CLI 展示，启动时全量构建并由事件增量维护 | `internal/modules/application/workorchd/biz/appservice/*view*` 与 server runtime |
| View | 产品展示层，只持有 UI 局部状态和 ViewModel 副本 | `web/workorch-console`、`internal/services/workorch/service/tui` |
| Presenter | 输入输出适配层，把用户操作转换为 command，把 ViewModel 映射为 REST/SSE/TUI 输出 | `internal/modules/application/workorchd/service/server`、`internal/pkg/workorchclient`、TUI command adapter |

### Model

Model 层是正式业务状态权威。规则：

- 每个业务模型由对应 owner module 管理。
- owner module 拥有自己的 repository/service/state machine。
- 跨 module 读写正式状态必须通过 EventHub command/response。
- 业务事件 topic 由 owner module 的 `pkg/events` 定义。
- Model 不依赖 Web、TUI、REST client 或 process service。
- Model 不为具体视图维护 UI 状态。

典型 Model owner：

- `internal/modules/kernel/workspace`
- `internal/modules/kernel/run`
- `internal/modules/kernel/decision`
- `internal/modules/kernel/approval`
- `internal/modules/kernel/result`
- `internal/modules/kernel/memory`
- `internal/modules/kernel/eventstream`
- `internal/modules/kernel/orchestrator`

### ViewModel

ViewModel 是服务端投影，不是正式业务状态。规则：

- ViewModel 可以由 Model 全量重建。
- ViewModel 可以缓存、失效、重建、降级。
- ViewModel 只读，不直接驱动状态机。
- ViewModel 必须带 revision、updatedAt、health/degraded 信息，便于端侧判断刷新状态。
- ViewModel 维护逻辑归 `workorchd`，不归 Web/TUI。
- ViewModel 读取路径必须快速、稳定，不能在端侧请求时重新做重型全量聚合。

### View

View 只负责展示和用户输入体验。规则：

- Web/TUI 不直接 import kernel owner service/repository。
- Web/TUI 不自行扫描所有 workspace。
- Web/TUI 不自行推导正式业务状态。
- Web/TUI 可以持有 UI 局部状态，例如 cursor、tab、viewport、input buffer。
- Web/TUI 可以持有最近一次 ViewModel 副本。
- Web/TUI 收到事件后只刷新 ViewModel 副本或请求轻量投影。

### Presenter

Presenter 是适配层。规则：

- REST handler 只做参数解析、校验、调用 use-case、输出 response。
- TUI command adapter 只把输入转为 backend port command。
- REST client 只负责 HTTP path、request/response 编解码和 error mapping。
- Presenter 不直接修复 Model 状态。
- Presenter 不把 workspace 投影逻辑复制到端侧。

## Implementation Status

截至 2026-06-09，本轮 MVVP 收口已经完成以下核心落点：

- `workorchd` 在 HTTP server 启动阶段创建 workspace projection manager。
- projection manager 启动时全量加载 workspace baseline，并维护
  workspace overview/detail projection store。
- projection manager 通过 `events.SubscribeWorkorch` 显式订阅 workspace/run/
  decision/approval/result/runtime 等 WorkOrch 业务事件；不依赖通配符订阅。
- projection rebuild 由 framework 注入的 `BackgroundRoutine` 调度；事件 observer
  只做轻量分类和 rebuild 调度。
- `workorchd` HTTP server、startup projection scheduling、watchdog timer、governance continuation、snapshot cache
  warmup 与 projection rebuild 都不再提供无 `BackgroundRoutine` 的生产 fallback。
  缺失 framework 注入的 `BackgroundRoutine` 应作为装配错误暴露。
- `/readyz` 输出 projection state，`ready/stale/degraded` 表示服务端 ViewModel 可服务。
- `GET /api/v1/workorch/workspaces` 读取服务端 projection store。
- `GET /api/v1/workorch/workspaces/{workspaceId}/snapshot` 返回服务端 detail
  projection，不要求 Web/TUI 自行 fan-out owner 状态。
- workspace detail projection 中已包含 `taskProgress`、`contextBundle`、
  `taskDecompositions`、governance、runtime observability 与 LLM usage summary。
- workspace LLM usage summary 会合并 workspace-scope 与 run-scope
  `TopicRunLLMUsageRecorded` event records，避免只看 workspace eventstream 时漏掉
  run-local usage。
- TUI 启动、workspace 切换和 projection-changed 事件处理统一消费 workspace
  list/detail projection；不再自行组合 run/status/goal/DoD/decision/approval。
- Web workspace board/detail 统一消费 projection list/detail；SSE 只作为 projection
  refresh signal。
- Web workspace detail 在 workspace 切换时只用 detail projection 填充主视图；
  run 级 snapshot/observability/page endpoints 仅作为用户显式点击“查看成果与过程”
  或分页按钮后的 drill-down 读取，不参与 workspace 主视图自动刷新。
- `internal/pkg/workorchclient`、`internal/pkg/workorchport`、remote/embedded backend
  已纳入同一 projection contract。
- project-local skill 已刷新 framework/EventHub/MVVP ViewModel 规则。

仍需长期遵守的审计规则：

- 新增 Web/TUI/CLI 主视图不得重新引入 request-time heavy fan-out。
- 新增正式状态读写必须通过 owner module 的 EventHub command/response。
- 新增业务 event topic 必须定义在被操作业务模型 owner 的 `pkg/events`。
- projection manager 只能维护可重建的 ViewModel，不拥有正式状态机。
- projection gap repair 可以发生在服务端 projection 层，不能下沉到端侧自行聚合。

## Target Architecture

目标架构如下：

```text
                       User Input
                           |
                    Web / TUI View
                           |
                      Presenter
       REST client / TUI backend / HTTP handlers / SSE mapper
                           |
                  Workorchd ViewModel
        workspace projection store + projection manager
                           |
             EventHub command/query and notifications
                           |
                         Model
        kernel workspace/run/decision/approval/result/...
                           |
                       Repository
```

### Ownership

| Object | Owner |
| --- | --- |
| Workspace formal state | `kernel/workspace` |
| Run formal state | `kernel/run` / `kernel/orchestrator` |
| Decision formal state | `kernel/decision` |
| Approval formal state | `kernel/approval` |
| Result formal state | `kernel/result` |
| Event records | `kernel/eventstream` |
| Workspace display state | `workorchd ViewModel projection manager` |
| TUI cursor/input/viewport | TUI View |
| Web tabs/table local filters | Web View |

## Workorchd ViewModel Projection Manager

### Responsibility

新增或完善 `workorchd` 内部 ViewModel projection manager，职责为：

- 启动时全量加载所有 workspace。
- 为每个 workspace 构建可展示的 overview projection。
- 为 active/selected workspace 构建 detail projection 或可快速补齐 detail projection。
- 订阅 EventHub 事件并增量更新投影。
- 检测事件 revision gap，并触发 workspace 级或全量 rebuild。
- 对外提供快速只读接口给 REST handler 和 TUI backend。
- 输出 SSE 事件，通知 Web/TUI 投影已变化。

### Non-Responsibility

projection manager 不负责：

- 创建 workspace
- 启动 run
- resolve decision / approval
- 接受 result
- 修改 kernel owner repository
- 直接调用 provider、LLM、MCP、shell 等 runtime block
- 承担 UI 布局逻辑

### Suggested Package Layout

后续代码收口建议使用以下落点：

```text
internal/modules/application/workorchd/biz/appservice/workspaceviewcore
  ViewModel DTO、projection store、revision、health/degraded 状态

internal/modules/application/workorchd/biz/appservice/workspaceviewports
  从 owner modules / readmodel dependencies 加载 baseline 的窄接口

internal/modules/application/workorchd/biz/appservice/workspaceviewprojection
  纯投影函数，复用 readmodelprojection 中已有映射能力

internal/modules/application/workorchd/service/server
  projection manager lifecycle wiring、REST/SSE route adapter
```

如果实现过程中发现 package 过细，应优先合并到一个 focused package，保持单一职责：

- `workspaceviewcore` 可以同时包含 DTO、store、manager interface。
- 纯函数稳定后再下沉到 `workspaceviewprojection`。
- server 层只保留 route、request/response、manager 启停接线。

## Involved Module Inventory

本次收口不考虑兼容性，所有涉及模块按以下职责重新固化。表中“目标职责”是
后续代码调整的验收依据。

| Path | Layer | Target responsibility | Current gap to check | Required closure |
| --- | --- | --- | --- | --- |
| `internal/modules/kernel/workspace` | Model owner | workspace 正式状态、workspace command topic、workspace notification topic | 确认所有 workspace create/update/delete/state transition 只由 owner command 完成 | 补齐 owner `pkg/events` topic，投影层只读或通过 command 读取 |
| `internal/modules/kernel/run` | Model owner | run 正式状态、run command/read topic、run state notification | 确认 run ID 读取有 workspace scope，避免跨 workspace `run-001` 串扰 | 所有 run get/list/status command payload 必须携带 workspace scope 或唯一 open 约束 |
| `internal/modules/kernel/orchestrator` | Model/governance owner | run 编排、continuation、state-machine 推进 | 确认无状态执行能力不被绑定到全局串行 lane | 只对同一 workspace/run 使用 lane；长耗时 continuation 交给 BackgroundRoutine |
| `internal/modules/kernel/decision` | Model owner | decision ticket/resolution 正式状态 | 确认 TUI/HTTP 不直接 resolve repository | resolve/list/read 只走 owner command，返回 `events.Response` |
| `internal/modules/kernel/approval` | Model owner | approval 正式状态 | 同上 | 同上 |
| `internal/modules/kernel/result` | Model owner | result/artifact/patch/validation 正式状态 | 确认目录规范、topic 归属和 scope | result/artifact event 归 owner `pkg/events`，投影只消费通知或 command read |
| `internal/modules/kernel/memory` | Model owner | memory recall/index 状态 | 确认 ViewModel 只展示摘要 | heavy recall/evidence 只在 detail/evidence 级按需加载 |
| `internal/modules/kernel/eventstream` | Model owner | event record、revision、replay checkpoint | 确认 projection startup 有 replay/gap 策略 | 提供 projection baseline/replay 所需 command/read topic |
| `internal/modules/kernel/soul` | Model owner | soul catalog/binding 正式状态 | 确认投影只读取展示字段 | owner events 定义 topic，appservice 只通过 command/read port |
| `internal/modules/kernel/skill` | Model owner | skill catalog/selection 正式状态 | 同上 | 同上 |
| `internal/modules/kernel/mcp` | Model owner | MCP server/capability 正式状态 | 同上 | 同上 |
| `internal/modules/application/workorchd/module.go` | Application module bridge | 接入 framework plugin lifecycle，暴露 workorchd 进程级 done command | 不承载 ViewModel 业务实现 | 保持窄桥接，不新增跨 owner service registry |
| `internal/modules/application/workorchd/biz/appservice/readmodelcore` | Appservice DTO | 现有 workspace/run snapshot/read-model DTO | DTO 与新 ViewModel contract 部分重叠 | 保留正式 read-model DTO，新增 projection DTO 时明确复用或替代关系 |
| `internal/modules/application/workorchd/biz/appservice/readmodelprojection` | Pure projection functions | owner model 到 read-model DTO 的纯映射 | 已复用为 projection baseline/detail 映射 | 继续保持纯函数，不拥有 projection store/lifecycle |
| `internal/modules/application/workorchd/biz/appservice/readmodelports` | EventHub-backed reader | owner command/read fan-out 和 snapshot baseline load | 已作为服务端 projection rebuild 内部 loader 使用 | HTTP/Web/TUI 不直接调用 heavy fan-out |
| `internal/modules/application/workorchd/biz/appservice/workspaceviewcore` | ViewModel core | `WorkspaceOverview`、`WorkspaceDetail`、projection health、store/manager | 已新增 projection store/manager，并订阅 WorkOrch 事件 | 后续只承载 ViewModel core，不扩展正式状态 owner 能力 |
| `internal/modules/application/workorchd/biz/appservice/workspaceviewports` | ViewModel baseline ports | 从 owner/readmodel 加载 baseline、workspace rebuild、detail warmup 的窄接口 | 第一轮由 `workspaceviewcore.Loader` + server adapter 承接 | 后续如继续拆分，只做窄 loader contract，不引入 facade |
| `internal/modules/application/workorchd/biz/appservice/workspaceviewprojection` | Pure projection | read-model snapshot 到 ViewModel overview/detail 的纯函数 | 第一轮复用 `readmodelprojection`，未单独拆包 | 只有出现稳定复用边界时再拆，避免 wrapper-only package |
| `internal/modules/application/workorchd/service/server` | Presenter + lifecycle adapter | HTTP handler、SSE mapper、projection manager 启停接线 | 已改为 list/detail handler 读 projection store | server 只保留 route/response/lifecycle wiring |
| `internal/pkg/workorchclient` | REST client | HTTP path、query、request/response decode、error mapping | 需要跟随新 ViewModel response envelope | 新增/调整 `ListWorkspaceOverviews`、`GetWorkspaceDetail` 等方法 |
| `internal/pkg/workorchport` | Adapter port contract | CLI/TUI/agent 使用的稳定 interface 和 DTO alias | 已增加 workspace list/detail projection contract | 保留 command port，禁止 adapter 直接依赖 backend 实现 |
| `internal/services/workorch/biz/backends` | Backend implementation | remote 调 REST client，embedded 调 EventHub-backed appservice/port | 需要统一返回服务端 ViewModel | remote/embedded 都实现同一 `workorchport` ViewModel interface |
| `internal/services/workorch/service/tui` | View + Presenter | 局部 UI 状态、输入、SSE/port message 处理 | 已改为消费 overview/detail projection | 不再新增端侧 owner fan-out 聚合 |
| `web/workorch-console` | View + Presenter | REST/SSE 展示服务端 ViewModel | 已统一走 projection list/detail | SSE 只触发 projection refresh |
| `scripts/verify-*`、`tests/e2e` | Verification | 自动管理 `./bin/workorch` 并验证 REST/SSE/TUI contract | 需要覆盖 startup projection 和多 workspace 并发 | 增加 projection ready、list/detail revision、TUI refresh e2e |
| `.agents/skills` / skill-hub 管理 skill | Development guidance | 指导后续应用按 framework/EventHub/MVVP 建模 | 已补充 service-side ViewModel/projection owner 规则 | 后续根据回归问题继续反馈到 skill |

## Final Package Organization

最终落点以职责为准，不以当前文件所在目录为准。

```text
internal/modules/application/workorchd/biz/appservice/workspaceviewcore
  types.go              # WorkspaceOverview, WorkspaceDetail, ProjectionStatus
  store.go              # in-memory projection store and revision rules
  manager.go            # manager interface, lifecycle-neutral core contract

internal/modules/application/workorchd/biz/appservice/workspaceviewports
  baseline.go           # full baseline load through EventHub-backed readers
  rebuild.go            # workspace/detail rebuild entrypoints
  events.go             # owner event extraction helpers

internal/modules/application/workorchd/biz/appservice/workspaceviewprojection
  overview.go           # read-model snapshot -> overview
  detail.go             # read-model snapshot -> detail
  health.go             # degraded/stale mapping

internal/modules/application/workorchd/service/server
  http_workspace.go     # route adapter only
  http_events.go        # SSE adapter only
  projection_runtime.go # framework BackgroundRoutine/EventHub wiring only
```

允许在第一轮收口中合并 `workspaceviewports` 与 `workspaceviewprojection`，但必须满足：

- appservice 包内拥有 DTO/store/rebuild 核心。
- server 包不拥有核心 projection store。
- server 包不直接调用多个 owner reader 拼装字段。
- package 名称能用一句话说明职责，不能成为新的 catch-all。

## Communication Rules By Owner

业务 event 的定义按被操作业务模型 owner 拆分。

| Owner | Command topic lives in | Notification topic lives in | Projection usage |
| --- | --- | --- | --- |
| Workspace | `kernel/workspace/pkg/events` | `kernel/workspace/pkg/events` | baseline/rebuild 使用 command read；created/updated/deleted 触发 overview patch/rebuild |
| Run | `kernel/run/pkg/events` | `kernel/run/pkg/events` | active run、run state、usage、progress 字段来自 command read 或 notification |
| Orchestrator | `kernel/orchestrator/pkg/events` | `kernel/orchestrator/pkg/events` | continuation/progress 事件只更新对应 workspace/run lane |
| Decision | `kernel/decision/pkg/events` | `kernel/decision/pkg/events` | unresolved/resolved blocker 更新 workspace blocker summary |
| Approval | `kernel/approval/pkg/events` | `kernel/approval/pkg/events` | approval blocker summary 同上 |
| Result | `kernel/result/pkg/events` | `kernel/result/pkg/events` | result/artifact/patch summary 更新 outcome |
| Memory | `kernel/memory/pkg/events` | `kernel/memory/pkg/events` | memory recall count/summary 更新 detail |
| EventStream | `kernel/eventstream/pkg/events` | `kernel/eventstream/pkg/events` | revision checkpoint、replay、gap repair |

基础 `internal/pkg/events` 只允许保留 envelope、hub、response、payload key、helper
和非业务通用 contract。不得新增业务 `Topic* = "/..."` alias。

## Lane And Runtime Rules

- 需要状态一致性的同一 workspace/run/step 事件绑定稳定 lane key。
- 无状态技术能力、只读投影读取、不同 workspace 的 rebuild 不绑定到同一全局 lane。
- projection manager 的 rebuild/detail warmup 通过 framework 注入的
  `BackgroundRoutine` 执行。
- EventHub observer 只做轻量分类、store patch、提交 rebuild task。
- 同步 command 必须检查 `events.Response`，缺失 response 是装配错误。
- shutdown 后 projection manager 关闭订阅并停止提交新的 background task。

## REST And Stream Final Contract

### `GET /api/v1/workorch/workspaces`

语义：读取服务端 projection store 的 workspace overview list。

禁止：

- 在 handler 内逐 workspace 调用 snapshot/result/evidence heavy loader。
- 在 handler 内直接访问 kernel owner repository/service。
- projection 未 ready 时长时间阻塞 HTTP 请求。

Response shape：

```json
{
  "service": "workorch",
  "backend": "file",
  "dataRoot": ".workorch-data",
  "projection": {
    "state": "ready",
    "globalRevision": 12,
    "updatedAt": "2026-06-09T00:00:00Z",
    "rebuiltAt": "2026-06-09T00:00:00Z",
    "workspaceTotal": 2,
    "degraded": false,
    "error": ""
  },
  "workspaces": []
}
```

### `GET /api/v1/workorch/workspaces/{workspaceId}/snapshot`

当前阶段可以继续使用 `snapshot` path，但返回语义调整为 workspace detail
projection。后续命名可改为 `/projection` 或 `/detail`，本阶段不做兼容承诺。

Response shape：

```json
{
  "service": "workorch",
  "backend": "file",
  "dataRoot": ".workorch-data",
  "projection": {
    "state": "ready",
    "workspaceRevision": 7,
    "globalRevision": 12,
    "updatedAt": "2026-06-09T00:00:00Z",
    "sourceEventRevision": 88
  },
  "snapshot": {}
}
```

如果 detail 尚未 ready：

- 可以返回 overview-backed partial detail，并标记 `projection.state=building`。
- 或返回 HTTP 409/503，并包含 projection state 与 error summary。
- 不能退回 TUI/Web 端自行聚合。

### `GET /api/v1/workorch/events/stream`

SSE event 必须包含 projection change 信号：

```json
{
  "type": "workspace_projection_changed",
  "workspaceId": "ws-...",
  "globalRevision": 12,
  "workspaceRevision": 7,
  "changeKind": "overview",
  "projectionState": "ready"
}
```

端侧收到事件后只做两件事：

- revision 变化时重新读取 list/detail projection。
- active workspace 不匹配时只更新 workspace monitor，不更新主视图。

## Code Closure Order And Hard Stops

### Step 0 - Stop Request-Time Ownership Drift

目标：先防止新的 View/TUI/Web 聚合继续扩散。

检查：

- `service/server` handler 不新增 owner repository/service 访问。
- TUI 不新增新的 `FetchWorkspaceContext` 类聚合。
- Web 不新增多个 heavy endpoint 拼 list 状态。

Hard stop：

- 如果某个 endpoint 必须临时使用旧聚合，必须在代码中限制为 rebuild/baseline
  内部路径，不能由 View 直接触发。

### Step 1 - Define ViewModel Core

目标：新增 `workspaceviewcore`，固化 DTO、store、revision、health。

验收：

- store 单测覆盖 upsert/list/get/delete/revision/degraded。
- DTO JSON 字段稳定。
- 无 framework、HTTP、TUI 依赖。

### Step 2 - Move Projection Core Out Of Server

目标：把当前 `service/server` projection 原型下沉到 appservice focused package。

验收：

- server 只依赖 manager interface。
- appservice projection core 不 import HTTP/router。
- `go test ./internal/modules/application/workorchd/biz/appservice/...` 通过。

### Step 3 - Baseline Load And Startup Ready

目标：`workorchd` 启动后自动全量加载所有 workspace projection。

验收：

- `HTTPServer.Start()` 或 server runtime 在 Run 阶段启动 projection manager。
- manager 使用 framework BackgroundRoutine。
- `/readyz` 能表达 projection ready/stale/degraded。
- 启动期间事件 gap 有 rebuild/replay 策略。

### Step 4 - REST Switch

目标：workspace list/detail handler 改读 projection store。

验收：

- `GET /workspaces` 不调用 `ListProjectedWorkspaces` heavy 聚合。
- `GET /workspaces/{id}/snapshot` 优先读 detail projection。
- command routes 仍通过 appservice/owner command 写 Model。

### Step 5 - Backend Port And Client Switch

目标：统一 remote/embedded backend 的 ViewModel 返回。

验收：

- `internal/pkg/workorchclient` decode 新 response envelope。
- `internal/pkg/workorchport` 暴露 overview/detail DTO 或明确 alias。
- `biz/backends/remote` 和 `biz/backends/eventhub` contract 一致。

### Step 6 - TUI Switch

目标：TUI 只消费 overview/detail projection。

验收：

- 启动读取 projection list。
- workspace switch 读取 projection detail。
- SSE/projection changed event 驱动 refresh。
- 删除或封存 TUI 侧 run/status/goal/dod/decision/approval 拼装。

### Step 7 - Web Switch

目标：Web console 使用同一 ViewModel API。

验收：

- Workspace board 用 overview list。
- Workspace detail 用 detail projection。
- SSE 只作为 refresh signal。

### Step 8 - Cleanup And Skill Refresh

目标：清理旧入口并沉淀规则。

验收：

- 删除失效 request-time list/detail 聚合入口或降级为 projection rebuild 内部函数。
- 文档、e2e、skill 同步更新。
- 全量 test/e2e 通过。

## ViewModel Data Contract

### WorkspaceOverview

workspace 列表视图必须使用轻量 overview：

```text
WorkspaceOverview
  id
  title
  description
  deliverableDirectory
  state
  activeRunId
  activeRunState
  activeRunPriority
  templateId
  profileId
  defaultSoulId
  blocker
    decisionCount
    approvalCount
    currentDecisionId
    currentApprovalId
  progress
    state
    label
    summary
    currentStepId
    latestEventSummary
  outcome
    resultState
    resultId
    artifactCount
  usage
    llmInputTokens
    llmOutputTokens
    cacheHitRate
  revision
  updatedAt
  projectionState
  projectionError
```

`projectionState` 至少包括：

- `ready`
- `building`
- `stale`
- `degraded`
- `failed`

### WorkspaceDetail

workspace detail 面向当前 active/selected workspace：

```text
WorkspaceDetail
  overview
  goals
  dods
  runs
  activeRun
  taskProgress
  contextBundle
  taskDecompositions
  decisions
  approvals
  memoryRecalls
  llmUsage
  llmContext
  runtimeEvents
  artifacts
  patches
  sources
  resultSummary
  failureReason
  failureHint
  revision
  updatedAt
  projectionState
```

Detail 可以分级：

- `summary`：启动和列表刷新必须快速返回。
- `active`：当前 workspace 主视图需要的 detail。
- `evidence`：较重的 run evidence、tool calls、checkpoints、artifacts，可按需分页。

### Revision Contract

所有 ViewModel response 必须带 revision：

- `globalRevision`：workspace 列表整体 revision。
- `workspaceRevision`：单 workspace projection revision。
- `sourceEventRevision`：投影已观察到的最新事件 revision；当前实现记录
  EventHub/EventStream envelope revision，后续 EventStream owner 提供 replay
  checkpoint 后可提升为严格 checkpoint。
- `rebuiltAt`：最近一次 full rebuild 时间。
- `updatedAt`：最近一次增量更新时间。

端侧只需要比较 revision，不自行判断内部 Model 状态。

## Startup Flow

目标启动流程：

1. `workorchd` framework service Startup 阶段完成依赖接线。
2. `Run` 阶段启动 HTTP server 和 projection manager。
3. projection manager 先订阅相关 EventHub notifications，避免启动窗口事件丢失。
4. projection manager 进入 `building` 状态。
5. projection manager 全量加载所有 workspace baseline。
6. projection manager 对 baseline 构建 overview store。
7. projection manager 读取 EventStream revision checkpoint。
8. 如启动期间存在事件 gap，执行 replay 或 workspace rebuild。
9. projection manager 标记 `ready`。
10. HTTP `/readyz` 应能体现 projection 是否 ready 或 degraded。
11. 后续事件通过 EventHub/SSE 增量推进 projection revision。

### Startup Consistency Rule

为了避免启动期间丢事件，必须满足以下之一：

- 订阅 EventHub 后加载 baseline，并通过 EventStream replay 补齐订阅前后的 revision gap。
- 或者加载 baseline 后立即执行一次 EventStream revision 对齐检查，发现 gap 后做 full rebuild。

不能仅依赖“启动时读取一次 repository”。

## Event Handling

projection manager 订阅通知类事件，不处理正式状态转换。

### Event Sources

至少需要消费：

- workspace created / updated / deleted / state changed
- goal defined / updated
- dod defined / updated
- run created / state changed / activated / completed / failed
- step dispatched / completed / failed
- decision created / resolved
- approval created / resolved
- result accepted / updated
- artifact changed
- patch changed
- memory recall recorded
- llm stream progress
- llm usage recorded
- eventstream revision gap / replay events

### EventHub Rules

- 投影更新使用 notification event。
- 需要重建某个 workspace 时，projection manager 可以通过 owner command 同步读取 Model。
- 同一 workspace 的投影更新必须按 workspace lane 保序。
- 无 workspace scope 的事件只能触发全局 refresh 或安全忽略。
- projection manager 不向 command payload 塞 owner service command 类型。
- command topic 直接 import owner module 的 `pkg/events`。

### Incremental Update Strategy

每个事件处理分为三类：

| 类型 | 策略 |
| --- | --- |
| 轻量字段事件 | 直接 patch overview/detail projection |
| 关联状态事件 | 触发 workspace rebuild |
| gap/unknown event | 标记 stale，并异步 rebuild workspace 或 full store |

事件处理必须幂等。

## REST API Contract

当前阶段不考虑兼容性，REST 语义应按 ViewModel 重新固化。

### Workspace List

`GET /api/v1/workorch/workspaces`

返回 `WorkspaceOverview[]`，来自 projection store。

要求：

- 不做请求时重型全量聚合。
- 不逐个 workspace 读取 result/evidence。
- 返回 global revision 和 projection health。
- projection 未 ready 时返回明确状态，不长时间阻塞。

建议 response：

```text
{
  "service": "workorch",
  "backend": "file",
  "dataRoot": ".workorch-data",
  "projection": {
    "state": "ready",
    "globalRevision": 123,
    "updatedAt": "...",
    "workspaceTotal": 10
  },
  "workspaces": []
}
```

### Workspace Detail

`GET /api/v1/workorch/workspaces/{workspaceId}/snapshot`

返回 `WorkspaceDetail` 或 detail projection。

要求：

- 优先从 projection store 读取。
- 如果 detail 未构建，返回 summary projection 并异步构建 detail，或显式返回 `building`。
- 不让 Web/TUI 等待不可控的重型全量聚合。

### Event Stream

`GET /api/v1/workorch/events/stream`

SSE 必须能通知投影变化。

建议事件 payload：

```text
{
  "type": "workspace_projection_changed",
  "workspaceId": "ws-...",
  "globalRevision": 123,
  "workspaceRevision": 45,
  "changeKind": "overview|detail|deleted|rebuild|gap",
  "projectionState": "ready"
}
```

端侧收到事件后：

- workspace list revision 变化：刷新 list。
- active workspace revision 变化：刷新 detail。
- gap/rebuild：刷新 list 和 active detail。

## TUI Contract

TUI 是 View + Presenter，不是 ViewModel owner。

### TUI State

TUI 允许持有：

- `workspaces []WorkspaceOverview`
- `activeWorkspaceID`
- `selectedWorkspaceID`
- `cursor`
- `input`
- `transcript`
- `mainViewport`
- `lastError`
- command submission pending/error state
- 最近一次 active `WorkspaceDetail`

TUI 不允许：

- 扫描全部 owner repository
- 自行组合 run/status/goal/dod/decision/approval/result
- 以旧 workspace 的异步响应覆盖当前 active workspace
- 用 command submission pending/error 覆盖 workspace/run 的服务端业务状态
- 使用固定 interval polling 作为主刷新机制

### TUI Startup

目标流程：

1. 建立 backend。
2. 订阅 SSE/EventHub stream。
3. 读取 `GET /workspaces` projection list。
4. 选择 active workspace。
5. 读取 active workspace detail projection。
6. 渲染视图。
7. 后续由 projection changed event 驱动刷新。

### TUI Command

TUI 输入只转为 command：

- create workspace
- submit goal
- define DoD
- start/resume/restart run
- resolve decision
- approve/reject approval
- inspect/detail refresh

command 返回后，TUI 不直接假定最终状态；它等待 workorchd projection event 或读取新的 projection revision。提交中、提交失败、HTTP/EventHub command 被拒绝等状态只能显示为端侧 command feedback，不能把当前 workspace/run 改写成 running、failed、decision、submitting 等业务状态。业务状态只来自服务端 projection/detail。

### TUI Refresh

TUI 刷新规则：

- 不再通过 `FetchWorkspaceContext` 自行重建上下文。
- 使用 `FetchWorkspaceOverviewList` 和 `FetchWorkspaceDetailProjection`。
- 所有消息必须带 workspace scope。
- stale message 必须丢弃。
- 主视图切换 workspace 时显式 reset viewport。
- non-active workspace 事件只能更新 workspace panel/monitor，不污染 active 主视图。

## Web Contract

Web 也是 View + Presenter。

Web 必须：

- 使用同一 `WorkspaceOverview` list API。
- 使用同一 `WorkspaceDetail` projection API。
- 使用同一 SSE projection changed event。
- 不在前端重复推导 workspace state。
- 不通过多个重型 endpoint 拼出列表状态。

Web 可保留：

- table pagination/filter/sort UI state
- selected tab
- modal form state
- optimistic input pending 状态

## BackgroundRoutine Contract

projection manager 的后台任务必须由 application framework 统一管理的
`BackgroundRoutine` 调度。

`workorchd` 不允许在生产路径为缺失 `BackgroundRoutine` 自行创建 fallback
goroutine。服务入口必须通过 `framework/application` 注入同一套
`EventHub` 与 `BackgroundRoutine`；测试代码如需验证异步行为，也应显式使用
`magicCommon/task.NewBackgroundRoutine` 或等价 fixture，而不是依赖生产 fallback。

允许：

- startup full load
- workspace rebuild
- gap repair rebuild
- detail warmup
- event replay

禁止：

- module/block 自行裸起长期 goroutine
- shutdown 后继续提交 projection rebuild
- View/TUI 触发未受控后台扫描

Shutdown 时：

- 先停止 projection manager 接收新事件。
- 取消 rebuild context。
- 等待 BackgroundRoutine 中已提交任务完成或超时。
- 释放 EventHub subscription。

## Readiness And Health

服务 health 应区分：

- process live
- HTTP ready
- projection ready
- projection degraded

建议：

- `/livez` 只表示进程存活。
- `/readyz` 在 projection 初始构建成功、stale 可服务或明确 degraded 可服务后返回 ready。
- `/api/v1/workorch/workspaces` response 中包含 projection health。

如果 projection failed：

- REST 返回明确 `projection.state=failed` 和 error summary。
- TUI 显示服务端投影失败，不自行 fallback 聚合。

## Migration Plan

### Phase 1 - Contract And DTO

- 定义 `WorkspaceOverview`、`WorkspaceDetail`、`ProjectionState`、revision contract。
- 明确 REST response envelope。
- 增加单元测试覆盖 DTO JSON。

### Phase 2 - Projection Store

- 实现 in-memory projection store。
- 支持 global revision、workspace revision、upsert/delete/list/get。
- 支持 projection health。
- 支持 stale/degraded 标记。

### Phase 3 - Baseline Loader

- 从 owner/readmodel dependencies 全量加载 workspace baseline。
- 构建 overview projection。
- 对 active/running/blocked workspace 构建 detail projection。
- 加入启动全量加载测试。

### Phase 4 - Event Subscription

- projection manager 订阅 EventHub notifications。
- 增量 patch overview。
- 对复杂事件触发 workspace rebuild。
- 检测 revision gap 并 rebuild。
- 加入事件驱动投影测试。

### Phase 5 - REST Switch

- `/workspaces` 改为读 projection store。
- `/workspaces/{id}/snapshot` 改为读 projection detail。
- 保留 command routes，不改变 Model owner 写入路径。
- 加入 HTTP handler 不触发重型聚合的测试。

### Phase 6 - TUI Switch

- TUI `FetchAll` 改为获取 `WorkspaceOverview[]`。
- TUI `FetchWorkspaceContext` 改为获取 `WorkspaceDetail`。
- 移除 TUI 侧 run/status/goal/dod/decision/approval 聚合。
- 保留 workspace scope 过滤和 viewport reset。
- 加入 TUI 端只消费投影的测试。

### Phase 7 - Web Alignment

- Web workspace list/detail 改为消费统一 ViewModel API。
- SSE 使用 projection changed event 驱动刷新。
- 移除前端重复推导 workspace state 的逻辑。

### Phase 8 - Cleanup

- 删除或降级旧的请求时重型聚合入口。
- 清理重复 DTO/mapper。
- 更新 service-api、TUI display 文档和 verification checklist。
- 更新对应 skill，确保后续应用开发按 MVVP + EventHub + framework 模式实现。

## Verification Plan

### Unit Tests

必须覆盖：

- projection store upsert/list/get/delete
- global revision 和 workspace revision 递增
- startup baseline load all workspaces
- event incremental update
- event gap rebuild
- deleted workspace 从 projection 删除
- degraded projection response
- stale event 幂等处理

### Server Tests

必须覆盖：

- `GET /workspaces` 读取 projection store，不触发重型聚合函数
- `GET /workspaces/{id}/snapshot` 读取 detail projection
- projection 未 ready 时返回明确状态
- startup 后 projection manager 调用 full load
- shutdown 释放 subscription/background tasks

### TUI Tests

必须覆盖：

- TUI 启动读取 projection list
- 切换 workspace 后读取 projection detail
- stale workspace message 丢弃
- projection changed event 触发 list/detail refresh
- non-active workspace update 只影响 workspace panel
- TUI 不调用旧的 context aggregation helper

### E2E

必须覆盖：

- 自动管理 `./bin/workorch` 实例。
- 启动后 `/readyz` ready。
- 创建多个 workspace。
- `/workspaces` 返回所有 workspace overview。
- 同时执行多个 workspace 后，list projection 中状态实时变化。
- TUI 或 TUI backend 测试能观察到 workspace list 全量和 active detail 更新。
- Web console 使用同一 API 能显示一致状态。

## Acceptance Criteria

代码收口完成时必须满足：

- workorchd 启动后自动全量加载所有 workspace。
- workorchd 有唯一 workspace ViewModel projection manager。
- `/api/v1/workorch/workspaces` 不再请求时重型聚合。
- Web 和 TUI 使用同一套 ViewModel contract。
- TUI 不再自行组合 workspace snapshot/context。
- Model state transition 仍由 kernel owner modules 通过 EventHub command 完成。
- projection manager 使用 framework 管理的 BackgroundRoutine。
- projection manager 的 EventHub subscription 在 shutdown 释放。
- 所有 workspace/run/decision/approval/result/artifact scope 明确。
- 所有端侧 async message 都携带 scope/revision。
- e2e 全量回归通过。

## Red Lines

- 不允许 Web/TUI 直接访问 kernel owner repository/service。
- 不允许 TUI 通过扫描所有 workspace 计算服务端状态。
- 不允许请求 `/workspaces` 时逐个 workspace 做重型 evidence 聚合。
- 不允许 ViewModel 直接修改 Model 正式状态。
- 不允许 projection manager 使用裸 goroutine 长期运行。
- 不允许缺少 workspace scope 的事件更新 active workspace 主视图。
- 不允许把业务 event topic 定义回共享基础 events 包。
- 不允许为了显示方便绕过 EventHub command/response。

## Source Documents

- `architecture-overview.md`
- `application-workorchd.md`
- `eventhub-lane-policy-and-module-event-contract-plan.md`
- `../50-product-interface/tui/display-refresh-consistency-design.md`
- `../50-product-interface/service-api.md`
- `../60-verification/framework-eventhub-module-boundary-checklist.md`
