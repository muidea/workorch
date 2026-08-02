# Application Workorchd Architecture

Status: active

Type: module-architecture

Last Updated: 2026-07-29

## Purpose

本文件说明 `application/workorch` 与 `internal/modules/application/workorchd` 的应用层职责。

生产主链为 **Runtime only**：application 负责入口、用例组合、typed bootstrap 构造与向 Run owner 的
Start/Resume **intent 提交**；不拥有 drive loop、continuation queue，也不在 Decision resolve 后 Resume。

权威说明：`internal/modules/application/workorchd/readme.md`。

## Main Responsibilities

应用层负责：

- 进程入口与 lifecycle
- runtime bootstrap 组装（application-owned infrastructure only）
- request-scoped runtime session
- HTTP / CLI / SSE surface
- appservice use-case 组合
- 构造 typed `RuntimeBootstrap` 并经 EventHub 创建 Run
- 将 Start / Resume scheduler command 转发为 Run owner typed activation / continuation intent
- workspace ViewModel projection 管理与输出适配

应用层不负责：

- 拥有 Workspace / Run / Result / Decision / Approval / Memory / MCP / Skill / Soul 正式状态
- 执行 Runtime drive / reducer / sealed dispatch
- 维护 run pending/started continuation queue 或 governance recovery scheduler
- Decision / Approval resolve 后发布 Resume
- 禁止的 inactive-running recovery、stale-Step watchdog、orphan Step / runtime-gap 恢复 API
- `executionContextPatch` 作为 activation、恢复或跨组件 payload

## Main Components

### `application/workorch`

- 主可执行入口
- 显式加载 initiator、kernel、blocks
- 启动 TUI 或 `--server`

### `internal/initiators/workorchruntime`

- 以 repository provider 形式为 kernel owner 装配依赖
- 不聚合导出 runtime policy 双权威；EventHub 来自 framework application runtime

### `internal/modules/application/workorchd/biz/bootstrap`

- 组装 application-owned infrastructure：projection EventHub、framework CommandHub、BackgroundRoutine、配置加载
- **不** 暴露 owner repository 到生产 bootstrap/session
- repository builder / owner fixture / 直写仅允许 `biz/testkit`

### `internal/modules/application/workorchd/biz/appservice`

- workspace activation / restart 用例
- run-scoped Start / Resume intent 提交
- decision / approval HTTP resolve（只提交 owner command + 刷新投影）
- read-model / ViewModel 装配

### `internal/modules/application/workorchd/service/server`

- route registration、HTTP handler、SSE
- 请求解析 → 调用 appservice → 输出 response
- 不持有 projection core，不直接访问 owner repository/service

## Runtime Activation

1. workspace activation 使用 cumulative effective Goal/DoD 与 profile capability policy 构造 typed `RuntimeBootstrap`
   Definition seed；它包含稳定 CWI identity、presentation slice 和 policy refs，不包含预制 semantic binding/focus queue。
2. bootstrap 随 Run create command 交给 Run owner 持久化；definition / activation 快照在初始化前不可变。
3. activation 为 DoD/profile requirement 生成稳定 seed item；逐 CWI target/recipe/check 与 focus 只由后续
   Orchestrator semantic planning/owner feasibility/SM-2 seal 形成。
4. RuntimeBootstrap 直接保存 dialog language、Soul 内容快照、Skill 指令、MCP 选择、Memory、evidence policy，以及 state-change / capability-usage / profile acceptance / template stage policy 等 execution contract。
5. Start/Resume：Run owner消费 scoped intent、内部迁移并发布 `RunStateChanged`；Orchestrator 再以该事实触发唯一 drive。

application **不** 维护 background queue；workspace activation/restart 同步执行用例，通过 Workspace / Run / Orchestrator typed EventHub command 完成。

## Start / Resume Boundary

| 入口 | application 行为 |
| --- | --- |
| Start scheduler command | 直接发送 Run owner typed activation intent |
| Resume scheduler command | 直接发送 Run owner typed continuation intent（仅 scope） |
| Decision / Approval HTTP resolve | 只提交对应 owner command 并刷新投影 |
| `runtime_governance` ticket resolve | Decision owner 直接提交 Orchestrator resolution；**不** 由 application 发 Resume |

约束：

- 请求只有在 typed owner command 成功接收后才返回 accepted
- 无 execution pending/started map、无 owner readback retry、无 scheduler recovery map
- 重启后 Runtime 推进由 Run owner 的 SM-1 state + opaque RuntimeStateRecord inventory/notification 触发
  Orchestrator internal drive；SM-4 typed load 后 owner-local 分类并幂等恢复

## Lifecycle

magicCommon context-first lifecycle：

- `Startup(ctx, ...)` / `Run(ctx)` / `Shutdown(ctx)`
- `Setup` / `Run` / `Teardown(ctx)`

服务启动只启动 workspace projection 与 HTTP 生命周期；**不** 运行 禁止的 inactive-running recovery，**不** 注册 stale-Step watchdog（watchdog interval audit = `0`）。

## Request-Scoped Runtime Session

- session 只持有应用基础设施与唯一 EventHub
- `ManagedRuntime` 仅订阅 Eventstream typed record 做进程内观测；不提供 owner action / snapshot facade / 跨 owner service adapter
- session 结束只做资源释放，不做业务状态裁决

## Read Model Boundary

| 层 | 职责 |
| --- | --- |
| observed | 从 Eventstream / owner 快照提取语义投影输入 |
| presenter | 包装为 API / Web / Console / TUI 输出 |
| appservice | 组织 read query、装配 DTO |

约束：

- projection 只读与投影，不修复 owner 状态、不重建业务状态机
- workspace list/detail 优先读启动后维护的 ViewModel projection
- 旧 request-time snapshot 聚合仅作 baseline/rebuild 内部细节

详见 `mvvp-workspace-viewmodel-architecture.md`、`projection-first-readmodel-io-closure-design.md`。

## Explicit Non-Goals

- `executionContextPatch` / `Run.ExecutionContext` / resume patch API
- application 订阅 Decision/Approval resolved 以恢复 Run
- `scheduleWatchdog`、stale dispatched recovery、stuck Run recovery helper
- 旧主链的 repair/output policy 缓存于 bootstrap/session
- orphan Step / runtime-gap 恢复 API

## Source Documents

- `architecture-overview.md`
- `kernel-orchestrator-governance.md`
- `mvvp-workspace-viewmodel-architecture.md`
- `projection-first-readmodel-io-closure-design.md`
- `internal/modules/application/workorchd/readme.md`
- `../00-core/runtime-stage-contract.md`
- `../50-product-interface/service-api.md`
