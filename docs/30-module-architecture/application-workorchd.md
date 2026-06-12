# Application Workorchd Architecture

Status: active

Type: module-architecture

Last Updated: 2026-06-12

## Purpose

本文件说明 `application/workorch` 和 `internal/modules/application/workorchd`
相关的应用层架构职责。

## Main Responsibilities

应用层负责：

- 进程入口与 lifecycle 触发
- runtime bootstrap 组装
- request-scoped runtime session
- HTTP service 暴露
- appservice 级 use-case 组合
- workspace ViewModel projection 的服务端管理和输出适配

应用层不负责：

- 拥有 workspace/run/decision/approval/result 等正式业务状态
- 绕过 owner module 直接执行状态机转换
- 让 Web/TUI 在端侧重建 workspace 状态投影

workspace ViewModel 的完整设计以
`mvvp-workspace-viewmodel-architecture.md` 为准。长期运行下的 projection-first
读路径、EventRecord payload、file backend IO 和 CPU/IO 收口设计以
`projection-first-readmodel-io-closure-design.md` 为准。

## Main Components

### `application/workorch`

职责：

- 作为主可执行入口
- 选择加载 initiator、kernel、blocks 模块
- 启动 TUI 或 `--server` 服务模式

### `internal/initiators/workorchruntime`

职责：

- 提供 repository、event hub、runtime policy、shared services
- 作为模块装配依赖源

### `internal/modules/application/workorchd/biz/bootstrap`

职责：

- 组装 grouped exports
- 构造应用层 Runtime shell
- 管理 request-scoped runtime module lifecycle

典型 exports 分组应至少包括：

- `Repositories`
- `Services`
- `Infrastructure`
- `Runtime`

分组边界：

- `Repositories`
  - workspace/run/result/event/memory 等正式仓储访问面
- `Services`
  - appservice 可直接组合的高层 service / policy / query face
- `Infrastructure`
  - event hub、config、logger、provider client、raw opener 等非业务设施
- `Runtime`
  - request-scoped runtime session、runtime use-case shell、stage-aware helper

### `internal/modules/application/workorchd/biz/appservice`

职责：

- execution activation
- workspace/run/decision/approval use-case
- read-model assembly
- runtime adapter/use-case composition

更细边界：

- activation / recovery / governance / read-model 等 use-case 组合应留在 appservice
- appservice 优先通过 module service 或 grouped runtime face 访问下层能力
- read-model DTO 与 read-model construction 逻辑应分离
- workspace overview/detail ViewModel core、projection store、baseline loader
  应落在 appservice focused package，而不是 HTTP server package

### `internal/modules/application/workorchd/service/server`

职责：

- route registration
- HTTP handler surface
- request validation / response output

server 层不应复制业务逻辑；其职责应止于：

- 请求解析与参数校验
- 调用 appservice
- 输出 HTTP response / SSE

server 层可以接线 projection manager lifecycle，但不拥有 projection core：

- 不在 handler 内逐 workspace 做 heavy snapshot/result/evidence 聚合
- 不直接访问 kernel owner repository/service
- 不把 projection store、baseline rebuild、field mapping 长期放在 server 包内
- 只把 projection store 的只读结果映射为 REST/SSE response

## Lifecycle Upgrade Note

当前 WorkOrch 已迁移到 magicCommon 的 context-first lifecycle：

- `Startup(ctx, ...)`
- `Run(ctx)`
- `Shutdown(ctx)`
- `Setup/Run/Teardown(ctx)`

该迁移属于应用层/框架接线问题，不属于 runtime core contract。

## Request-Scoped Runtime Session

应用层需要为 HTTP / backend / local TUI 等入口提供 request-scoped runtime session：

- session 只负责本次请求的 runtime access 与 teardown
- session 不能演化为新的业务状态拥有者
- request cancel / context cancel 应能安全传递到 appservice 与下游 runtime use-case

稳定方向：

- HTTP 请求进入后，应在 request scope 内绑定 runtime access / repositories / event hub / raw opener
- session 结束时只做资源释放，不做业务状态裁决
- session 既不替代 workspace/run state，也不替代 repository 持久化 contract

### Startup Recovery Boundary

workorchd 启动恢复属于应用层 use-case 组合，职责是把 repository 中无法由内存 executor
继续推进的状态重新交回 kernel runtime contract：

- `running + terminal dispatched step` 表示上次进程已派发但未形成 output；启动恢复应调用 run service resume，让 orchestrator 重新派发该 step。
- `running` 且没有 blocker、没有 continuation checkpoint、也没有可恢复 terminal dispatched/failed step 时，才能标记为 stuck failure。
- 应用层只能识别这些通用状态形态，不得根据 workspace template、具体文件、测试框架或业务目标做分支。
- 恢复后状态仍由 kernel run/orchestrator 决定，appservice 只返回恢复结果投影。

Run watchdog / continuation recovery 是该边界的运行期扩展：workorchd 可以通过 framework `BackgroundRoutine`
注册启动恢复任务和周期 timer，扫描 stale dispatched、resolved governance blocker、replan continuation
等通用运行时形态，但不得直接改写正式状态机。详细触发时机、影响范围和后续特性跟踪见
`../70-roadmap/active/workorchd-state-loop-and-projection-closure-plan.md#p24-run-watchdog--continuation-recovery`。

典型入口包括：

- HTTP read/write handler
- deployed verification / server-side request path
- local TUI / backend bridge 在应用层复用的 request scope

## Read Model Boundary

应用层读模型职责应继续分离为：

- observed: 事件/状态到 snapshot 的语义投影
- presenter: 输出渲染/包装
- appservice: 聚合读取入口与 DTO 装配

### Observed / Presenter / Read-Model Split

三层当前至少应保持：

| 层 | 当前职责 |
| --- | --- |
| observed | 从 repositories / eventstream / runtime state 提取语义投影输入 |
| presenter | 把 observed 结果包装成 API / Web / Console / TUI 需要的输出形态 |
| appservice | 组织 read query、调用 observed / presenter、输出 DTO |

约束：

- observed 不直接做 HTTP 级响应包装
- presenter 不重新执行业务查询或状态裁决
- appservice 不应把领域查询、投影语义、HTTP 输出全部混在一个函数里

### Read-Model Assembly Focus

应用层当前主要装配以下 read-model：

- workspace snapshot / detail
- run snapshot / status / observability
- decisions / approvals page
- taskProgress / contextBundle / taskDecompositions / memoryRecallTraces
- artifact content / runtime payload / event list 的读打开入口

这些装配都应：

- 优先复用 module/service/repository query face
- 与 `service-api.md` 的 response family 保持一致
- 与 `service-acceptance.md` 的 direct verification 证据保持一致

### Workspace ViewModel Boundary

workspace list/detail 面向 Web/TUI 时应优先读取 workorchd 启动后维护的
ViewModel projection：

- overview list: `GET /api/v1/workorch/workspaces`
- detail projection: `GET /api/v1/workorch/workspaces/:workspaceId/snapshot`
- refresh signal: `GET /api/v1/workorch/events`

旧的 request-time snapshot 聚合只能作为 projection baseline/rebuild 的内部实现
细节，不再作为 Web/TUI 的主刷新路径。

## Source Documents

- `repository-structure.md`
- `mvvp-workspace-viewmodel-architecture.md`
- `projection-first-readmodel-io-closure-design.md`
- `../50-product-interface/service-api.md`
