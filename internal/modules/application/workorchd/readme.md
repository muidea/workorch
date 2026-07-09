# workorchd Application Module

## 功能定位

`application/workorchd` 是 WorkOrch 服务端 application module。它把 HTTP/Web API、CLI command dispatcher、projection/read model、配置接口、context registry 与 runtime session 接入 magicCommon module 生命周期。

## 拥有边界

- 拥有产品入口、API 用例编排、projection DTO、read model、SSE/event refresh 与配置/context registry 应用面。
- 不拥有 kernel 正式状态；Workspace、Run、Result、Decision、Approval、Memory、MCP、Skill、Soul 等状态权威保留在对应 kernel module。
- 不直接修复 owner state；跨 owner 读写必须走 EventHub command/response 或稳定 port。

## 主要交互

- 从 `workorch/runtime` initiator 获取 repository exports、EventHub 与配置加载能力。
- 通过 owner command/read ports 与 `kernel/workspace`、`kernel/run`、`kernel/orchestrator`、`kernel/result`、`kernel/decision`、`kernel/approval`、`kernel/eventstream`、`kernel/memory`、`kernel/mcp`、`kernel/skill`、`kernel/soul` 交互。
- 通过 `internal/services/workorch` 支撑本地/远程 CLI、TUI、server mode。
- 为 `web/workorch-console` 和 `internal/pkg/workorchclient` 暴露 HTTP 合同。

## 组件交互合同

- Input contract: API/TUI/CLI 写请求必须转换为 scoped command 或正式 owner port 调用。
- Output contract: 对外只返回 bounded projection DTO、command result、raw/detail ref 或配置 DTO。
- Owner interaction contract: 访问 kernel owner state 必须通过 EventHub command/response 或稳定 read/command port。
- Projection contract: projection 只能读取 owner 快照和事件，不修复 owner state，不重建业务状态机。
- Runtime session contract: borrowed exports 必须包含 appservice 所需的完整 repository 与 EventHub 依赖。

## 已知缺口

- `ExportsFromInitiator` 当前未填充 `CoverageWorkItems`，`runtimecore.HasRuntimeExports` 只检查 EventHub，导致缺依赖可能在下游表现为泛化 `invalid argument`。
- `service/server` 存在 background submit failure 行为与测试期望不一致的问题，需要在 server lifecycle 合同中单独收口。

## 边界约束

- application module 只做入口、facade、用例编排和投影，不成为业务状态 owner。
- borrowed runtime exports 必须完整校验，避免下游出现泛化 `invalid argument`。
- HTTP/TUI/CLI 不能绕过 owner module 直接修改正式状态。
