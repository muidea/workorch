# workorchd Application Module

## 功能说明

`application/workorchd` 是服务端应用模块，负责把 HTTP/Web API、CLI facade、runtime session、scheduler、context registry、read model、projection 和 SSE 接入统一生命周期。它组合多个 owner 的能力形成应用用例，但不拥有 Workspace、Run、Result、Decision、Approval、Memory、MCP、Skill 或 Soul 的正式状态。

## 组件交互及合同

- 模块只使用 framework 注入的 magicCommon `event.Hub`；HTTP、CLI、scheduler、projection 和 owner command 共用同一个 Hub。
- 对 Workspace、Run、Orchestrator、Result、Decision、Approval、Memory、MCP Registry、Skill、Soul、Governance 和 Eventstream 的读写必须发送投递方 `pkg/events` 定义的具体 command，并校验具体 result。
- `pkg/events.DoneCommand` / `DoneCommandResult` 用于查询服务退出通道；`WorkspaceRunRequestedCommand` / `WorkspaceRunRequestedResult` 用于 scheduler 请求；`CLICommand` / `CLIResult` 用于 embedded CLI facade。
- `WorkspaceProjectionChangedData` 由本模块发布；workspace projection 只消费 Eventstream 的 `EventRecordAppendedData` 和 owner 快照。
- HTTP、SSE 和 remote adapter 使用应用层 DTO；组件间 EventHub 传输不得复用 HTTP DTO。
- runtime session 只持有应用基础设施和唯一 EventHub；生产 bootstrap 不暴露 owner repository。

## 相关约束

- application 只负责入口、用例编排、facade 和投影，不得成为正式状态 owner。
- HTTP、CLI、TUI、scheduler 和 projection 不得直接访问 owner repository 或 service。
- projection 只能读取和投影事件，不得修复 owner 状态或重建业务状态机。
- 跨组件 command/data/result 必须使用具体类型，不得引入 Envelope、通用 map payload、JSON 类型桥、reflect 或双 Hub 兼容层。
- repository builder、owner module fixture 和 repository 直写只能存在于 `biz/testkit` 或测试文件。
