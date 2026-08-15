# Eventstream Block-class Owner

## 组件类型

`blocks/eventstream` 按职责属于 Block：它只拥有 EventRecord 审计流、workspace
revision high-watermark 与查询/清理合同，不组合多个 owner 推进业务闭环。目录、
module ID 和 EventHub 合同身份均为 `blocks/eventstream`。

## 功能说明

`blocks/eventstream` 聚合多个组件发布的具体通知，规范化为按 workspace 有序的 EventRecord 审计流，并提供列表、workspace revision high-watermark 与 workspace purge 合同。EventRecord 是审计与投影状态，不是业务 owner 的修复入口。

## 组件交互及合同

- 分 topic 订阅 Workspace、Run 状态、Orchestrator Patch/ArtifactContinuation、Result、Decision、Approval、Memory 和 Governance 的具体 data 类型。
- 输入通知转换为 `EventRecordData` 后写入 EventRecord repository，并发布 `EventRecordAppendedData`。
- `ListEventRecordsCommand`、`HighWatermarksCommand`、`PurgeWorkspaceEventsCommand` 及对应 `EventStreamCommandResult` 由 `blocks/eventstream/pkg/events` 定义；EventRecord 只能由已订阅的 owner notification 追加，不暴露外部 append command。
- `EventRecordQuery` 是 Eventstream owner DTO；module 内部映射 repository query 并 clone `ExcludeTopics`，EventHub 不暴露 repository query/entity。
- application projection readiness 必须通过 typed `HighWatermarksCommand` 比较 EventStream owner 的真实 persisted workspace revision；不得仅依赖 projection 自己收到的 `EventRecordAppendedData` 自证 ready。
- application projection、SSE、Observed 和 remote 展示只消费 `EventRecordData`，不反向修改 owner 状态。
- 审计 wire 的 version、字段 key、typed payload encoder/decoder 由
  `pkg/recordpayload` 唯一定义；共享 `internal/pkg/events` 只保留 owner-neutral
  EventHub lane helper，不保存业务 payload mapper 或全局字段索引。
- workspace purge 必须携带 WorkspaceID，并只删除该 workspace 的 EventRecord。

## 当前内聚收口切片（2026-08-16）

- `pkg/recordpayload` 仍是唯一 audit wire codec package；原 842 行 payload 文件已按 workspace/run/definition、
  decision/approval/governance、result/artifact/continuation、memory/recall、LLM usage 事件族分成纯 typed codec 文件。
  字段 key 仍只在 `payload.go` 定义，wire version 仍只在 `version.go` 定义。
- `biz/event_stream_service.go`、唯一 repository append authority、workspace revision/high-watermark/index、query/purge 与
  `EventRecordAppendedData` 发布顺序均未修改。codec 文件不持有 Hub/repository，不依据审计 payload 修复任何 owner。
- C22-06 Eventstream 切片完成后，非测试 Go 为 16 个文件/1,884 LOC，测试保持 5 个文件/1,002 LOC，package
  保持 6；最大 codec 文件从 `pkg/recordpayload/payload.go` 842 行降为
  `pkg/recordpayload/governance_payload.go` 211 行，组件最大生产文件变为未修改的 append owner
  `biz/event_stream_service.go` 796 行；至少 500 行生产文件从 2 个降为 1 个，生产 `CC>30` 继续为 0。

## 相关约束

- Eventstream 不得把投影或审计记录当成 owner state，也不得据此修复 Workspace、Run、Result 或治理状态。
- 每种订阅必须显式校验投递方具体 data 类型；禁止 Envelope、通用 map payload、JSON/reflect 类型转换桥。
- EventRecord payload 使用 Eventstream 自有递归具体值表达；无法安全转换的值必须拒绝或降级为有界摘要。
- 无生产发布方的 禁止的 Step、LLM progress 和 contract-violation notification 不注册消息合同或 typed subscription；历史 EventRecord 仅作为通用只读审计数据保留。
- repository append 失败（包括 conflict）必须向同步发送方返回错误，且不得发布不存在的 `EventRecordAppendedData`。
- 事件顺序和 revision 必须在 workspace scope 内稳定、可重放且可清理。
- high-watermark 查询是只读 owner contract，只返回 workspace/revision，不暴露 repository，也不允许 projection 反向修复 Eventstream。
