# workorchd Application Module

## 功能定位

`application/workorchd` 是 WorkOrch 服务端 application module。它把 HTTP/Web API、CLI command dispatcher、projection/read model、配置接口、context registry 与 runtime session 接入 magicCommon module 生命周期。

## 拥有边界

- 拥有产品入口、API 用例编排、projection DTO、read model、SSE/event refresh 与配置/context registry 应用面。
- 不拥有 kernel 正式状态；Workspace、Run、Result、Decision、Approval、Memory、MCP、Skill、Soul 等状态权威保留在对应 kernel module。
- 不直接修复 owner state；跨 owner 读写必须走 EventHub command/response 或稳定 port。

## 主要交互

- 直接使用 framework 注入或组件自建的 magicCommon `event.Hub` 作为唯一 CommandHub，并从配置包获取加载能力；不再构造独立 projection Hub，也不通过 initiator 获取或转发 Hub/owner repository。
- 通过 owner command/read ports 与 `blocks/workspace`、`kernel/run`、`kernel/orchestrator`、`blocks/result`、`blocks/decision`、`blocks/approval`、`kernel/eventstream`、`blocks/memory`、`blocks/mcp-registry`、`blocks/skill`、`blocks/soul` 交互。
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
- Workspace projection event contract: `workspaceviewcore.Manager` 直接订阅 Eventstream owner 定义的 `EventRecordAppendedData`，并直接读取其中的 `EventRecordData` 更新 stale/runtime-progress 投影；禁止先恢复为 `events.Envelope` 再分发。投影变化由本组件定义并发布 `WorkspaceProjectionChangedData`，Server 直接订阅该具体类型。
- Runtime session contract: borrowed exports 只包含 application-owned infrastructure；必须提供唯一 framework `CommandHub`，不得携带 projection `events.Hub` 或 owner repository。
- Test fixture contract: file/memory repository builder、owner module 直接实例化和 repository fixture 直写只允许存在于 `biz/testkit` 与 `*_test.go`，不得回流到生产 bootstrap/session 合同。
- Test lifecycle contract: testkit 的 repository/module 生命周期由 test-only magicCommon CommandHub wrapper 显式持有；禁止再通过 projection `events.Hub` wrapper 携带 repository exports、owner modules 或 teardown 行为。
- EventHub export contract: bootstrap/runtime session/infrastructure exports 只保留 framework `CommandHub`；新建 command port 直接接收该 Hub，不得新增 `RequiredEventHub()`、`RawHub()`、`rawMagicHubProvider` 或双 Hub 兼容接口。
- Runtime action port contract: `runtimeports.EventHubActionPort` 与 managed runtime factory 只接收 framework `CommandHub`；projection、scheduler、owner command 与 typed notification 共用该 Hub。
- LLM command contract: workspace setup draft 与 activation embedding/run command 直接使用 session `CommandHub`；CLI governance continuation 也从 session 获取 `RequiredCommandHub()`。
- Server command contract: production server 只接收 `CommandHub`；runtime policy update、projection watcher 与 scheduler command/watcher 直接使用该 Hub。未注入时独立 Server 测试/构造会创建自己的 magicCommon Hub，并从该 Hub 建立 RuntimeSession。
- Bootstrap/context contract: bootstrap、testkit、HTTP server、CLI dispatcher 与 backend session 均只传递 `CommandHub`；`MagicHub.RawHub()`、`ContextWithEventHubs`、`EventHubFromContext` 已删除，请求与后台 context 统一使用 `ContextWithCommandHub`。
- Activation memory recall contract: activation 只发送 Memory owner 定义的 recall upsert command；持久化成功后的 `MemoryRecallRecordedData` 由 Memory block 单点发布，activation 不再接收 projection Hub 或重复发布 Envelope。

## 已知缺口

- 当前盘点未发现 application module 职责或 owner command/result 交互合同缺失。
- X04 已关闭：生产 bootstrap/session 已删除完整 owner repository exports，runtime session 只校验 EventHub/CommandHub；测试 repository fixture 已迁入 `biz/testkit`。
- server scheduler 与 runtime block execute 链路已改用投递方定义的具体 command/data/result；后续新增跨组件事件时不得恢复 Envelope payload map、通用 `map[string]any` 或 JSON 类型转换桥。
- Managed runtime 的已发布事件来源只允许是 Eventstream 的 typed `EventRecordAppendedData` collector；旧 projection Hub `Published() []Envelope` 回读、去重合并兼容链已删除。
- HTTP/SSE 展示与回放已使用 Server 自有 `streamEvent` DTO，不再复用 `events.Envelope`；Workspace projection、Managed runtime collector、Server watcher、Observed snapshot 与组件间 EventHub 传输均不使用该类型。Observed collector、runtime session、read-model readers 与 snapshot projection 全链路直接传递 Eventstream 定义的 `EventRecordData`；历史 `WorkorchEvent`、Snapshot/Stream Envelope 双向转换及对应 decoder/normalize 兼容模型已删除。
- workorchd application 与 `internal/services/workorch` 的生产及测试 Go 代码中 `events.Envelope`、legacy `events.Hub`、`MagicHub`、双 Hub 装配参数与上下文透传扫描均已清零；HTTP/SSE/remote 测试使用本地 DTO，Observed/ManagedRuntime/projection 测试直接使用 `EventRecordData` fixture。
- `internal/pkg/events` 仅保留 lane/message contract、typed runtime message alias、审计 payload 映射与版本校验；legacy Envelope/Hub/MemoryHub/MagicHub 实现已作为无调用方失效代码删除。

## 边界约束

- application module 只做入口、facade、用例编排和投影，不成为业务状态 owner。
- borrowed runtime exports 必须校验 application-owned infrastructure，避免下游出现泛化 `invalid argument`；禁止重新加入 owner repository 字段。
- HTTP/TUI/CLI 不能绕过 owner module 直接修改正式状态。
