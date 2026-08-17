# Eventstream Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-17

## 组件类型

`blocks/eventstream` 按职责属于 Block：它只拥有 EventRecord 审计流、workspace
revision high-watermark 与查询/清理合同，不组合多个 owner 推进业务闭环。目录、
module ID 和 EventHub 合同身份均为 `blocks/eventstream`。

## 功能范围与边界

`blocks/eventstream` 聚合多个组件发布的具体通知，规范化为按 workspace 有序的 EventRecord 审计流，并提供列表、workspace revision high-watermark 与 workspace purge 合同。EventRecord 是审计与投影状态，不是业务 owner 的修复入口。

## 组件交互及合同

- 分 topic 订阅 Workspace、Run、Result、Decision、Approval、Memory、Governance 和 LLM usage 的具体 data 类型；当前不直接订阅 Orchestrator topic。
- 输入通知转换为 `EventRecordData` 后写入 EventRecord repository，并发布 `EventRecordAppendedData`。
- `ListEventRecordsCommand`、`HighWatermarksCommand`、`PurgeWorkspaceEventsCommand` 及对应 `EventStreamCommandResult` 由 `blocks/eventstream/pkg/events` 定义；EventRecord 只能由已订阅的 owner notification 追加，不暴露外部 append command。
- `EventRecordQuery` 是 Eventstream owner DTO；module 内部映射 repository query 并 clone `ExcludeTopics`，EventHub 不暴露 repository query/entity。
- application projection readiness 必须通过 typed `HighWatermarksCommand` 比较 EventStream owner 的真实 persisted workspace revision；不得仅依赖 projection 自己收到的 `EventRecordAppendedData` 自证 ready。
- application projection、SSE、Observed 和 remote 展示只消费 `EventRecordData`，不反向修改 owner 状态。
- 审计 wire 的 version、字段 key、typed payload encoder/decoder 由
  `pkg/recordpayload` 唯一定义；共享 `internal/pkg/events` 只保留 owner-neutral
  EventHub lane helper，不保存业务 payload mapper 或全局字段索引。
- workspace purge 必须携带 WorkspaceID，并只删除该 workspace 的 EventRecord。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| Workspace、Run、Result、Decision、Approval、Memory、Governance、LLM | owner → Eventstream | 各 owner 的具体 typed notification | 完整；逐 topic/type 显式订阅 |
| `application/workorchd` | Application → Eventstream | list、high-watermarks、workspace purge | 完整 |
| application projection | 双向只读链路 | 消费 `EventRecordAppendedData`，并以 persisted high-watermark 判定 readiness | 完整；projection 不回写 owner |
| `kernel/orchestrator` | 无直接订阅 | Runtime 权威状态经 owner 正式事实进入审计 | 完整；不从审计重建 authority |

## 相关约束

- Eventstream 不得把投影或审计记录当成 owner state，也不得据此修复 Workspace、Run、Result 或治理状态。
- 每种订阅必须显式校验投递方具体 data 类型；禁止 Envelope、通用 map payload、JSON/reflect 类型转换桥。
- EventRecord payload 使用 Eventstream 自有递归具体值表达；无法安全转换的值必须拒绝或降级为有界摘要。
- 无生产发布方的 禁止的 Step、LLM progress 和 contract-violation notification 不注册消息合同或 typed subscription；历史 EventRecord 仅作为通用只读审计数据保留。
- repository append 失败（包括 conflict）必须向同步发送方返回错误，且不得发布不存在的 `EventRecordAppendedData`。
- 事件顺序和 revision 必须在 workspace scope 内稳定、可重放且可清理。
- high-watermark 查询是只读 owner contract，只返回 workspace/revision，不暴露 repository，也不允许 projection 反向修复 Eventstream。

## 待完善测试点

- [ ] 为每个生产订阅冻结 topic 与 data 类型矩阵，覆盖 wrong type、nil data 和无生产发布方 topic 不注册。
- [ ] 覆盖 append conflict/repository failure，确认同步发送方收到错误且不发布虚假 appended 通知。
- [ ] 覆盖 workspace 内 revision 单调性、重启后的 high-watermark 恢复和不同 workspace 隔离。
- [ ] 覆盖 purge 仅清理目标 workspace，重复 purge 幂等且不影响其他审计流。
- [ ] 覆盖 Post 通知 handler 的失败隔离、有界 payload 降级以及 projection lag 后的 readiness 判定。
