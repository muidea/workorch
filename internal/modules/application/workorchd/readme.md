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
- Done command contract: `internal/services/workorch` 发送 `pkg/events.DoneCommand`，本 module 直接通过 magicCommon EventHub 返回 `DoneCommandResult{Done <-chan error}`；禁止使用 Envelope payload 传递 channel。
- Scheduler command contract: HTTP、CLI 与 embedded backend 发送 `pkg/events.WorkspaceRunRequestedCommand`，server scheduler 返回 `WorkspaceRunRequestedResult`；workspace/run scope、intent、activation selection 与 continuation patch 均由具体 command 字段承载，禁止使用 `map[string]any` 作为 EventHub command/result。
- CLI facade contract: `internal/services/workorch` 直接通过 CommandHub 发送本组件 `pkg/events.CLICommand`，dispatcher 返回具体 `CLIResult`；`/cli/result`、Envelope payload map、`TypedResource any` 与 map 编解码兼容链已删除。
- Projection contract: projection 只能读取 owner 快照和事件，不修复 owner state，不重建业务状态机。
- Runtime session contract: borrowed exports 必须包含 appservice 所需的完整 repository 与 EventHub 依赖。
- EventHub export contract: runtime session 同时要求 projection `EventHub` 与 framework `CommandHub`；新建 command port 必须依赖 `RequiredCommandHub()`，不得新增 `RawHub()`、`rawMagicHubProvider` 或单 Hub 反查兼容接口。
- Runtime action port contract: `runtimeports.EventHubActionPort` 直接接收 framework `CommandHub`，managed runtime factory 必须同时接收 projection Hub 与 CommandHub。
- LLM command contract: workspace setup draft 与 activation embedding/run command 直接使用 session `CommandHub`；CLI governance continuation 也从 session 获取 `RequiredCommandHub()`。
- Server command contract: production server 显式接收 projection Hub 与 CommandHub，runtime policy update 和 scheduler command/watcher 不再自行调用 `RawHub()`。
- Bootstrap/context contract: bootstrap、testkit、HTTP server、CLI dispatcher 与 backend session 均显式传递双 Hub；`MagicHub.RawHub()` 已删除，请求与后台 context 必须使用 `ContextWithEventHubs`。

## 已知缺口

- 当前盘点未发现 application module 职责或 owner command/result 交互合同缺失。
- initiator exports 已补齐 `CoverageWorkItems`，runtime session 会逐项校验全部 repository exports 与 EventHub，并对缺失项返回具名错误；后续增加 borrowed export 时必须同步更新边界校验。
- server scheduler 与 runtime block execute 链路已改用投递方定义的具体 command/data/result；后续新增跨组件事件时不得恢复 Envelope payload map、通用 `map[string]any` 或 JSON 类型转换桥。
- projection/SSE 通知仍使用 `events.Envelope` 作为展示与回放载体；该链路不再承担 owner command/data/result 交互，也不能作为重新引入单 Hub 兼容层的理由。

## 边界约束

- application module 只做入口、facade、用例编排和投影，不成为业务状态 owner。
- borrowed runtime exports 必须完整校验，避免下游出现泛化 `invalid argument`。
- HTTP/TUI/CLI 不能绕过 owner module 直接修改正式状态。
