# Application Workorchd Architecture

Status: active

Type: module-architecture

Last Updated: 2026-05-30

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

### `internal/modules/application/workorchd/service/server`

职责：

- route registration
- HTTP handler surface
- request validation / response output

server 层不应复制业务逻辑；其职责应止于：

- 请求解析与参数校验
- 调用 appservice
- 输出 HTTP response / SSE

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

## Source Documents

- `repository-structure.md`
- `../50-product-interface/service-api.md`
