# Eventstream Module

## 功能说明

`kernel/eventstream` 聚合多个组件发布的具体通知，规范化为按 workspace 有序的 EventRecord 审计流，并提供列表、追加和 workspace purge 合同。EventRecord 是审计与投影状态，不是业务 owner 的修复入口。

## 组件交互及合同

- 分 topic 订阅 Workspace、Run 状态、Orchestrator Patch/ArtifactContinuation、Result、Decision、Approval、Memory 和 Governance 的具体 data 类型。
- 输入通知转换为 `EventRecordData` 后写入 EventRecord repository，并发布 `EventRecordAppendedData`。
- `ListEventRecordsCommand`、`AppendEventRecordCommand`、`PurgeWorkspaceEventsCommand` 及对应 `EventStreamCommandResult` 由 `kernel/eventstream/pkg/events` 定义。
- application projection、SSE、Observed 和 remote 展示只消费 `EventRecordData`，不反向修改 owner 状态。
- workspace purge 必须携带 WorkspaceID，并只删除该 workspace 的 EventRecord。

## 相关约束

- Eventstream 不得把投影或审计记录当成 owner state，也不得据此修复 Workspace、Run、Result 或治理状态。
- 每种订阅必须显式校验投递方具体 data 类型；禁止 Envelope、通用 map payload、JSON/reflect 类型转换桥。
- EventRecord payload 使用 Eventstream 自有递归具体值表达；无法安全转换的值必须拒绝或降级为有界摘要。
- 无生产发布方的 V1 Step、LLM progress/usage 和 contract-violation notification 不注册消息合同或 typed subscription；历史 EventRecord 仅作为通用只读审计数据保留。
- 事件顺序和 revision 必须在 workspace scope 内稳定、可重放且可清理。
