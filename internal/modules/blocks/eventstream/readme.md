# Eventstream Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/eventstream` 是不可变 `EventRecord` 审计流、全局 append `StreamPosition`、workspace-local record revision /
high-watermark，以及永久 workspace Eventstream deletion tombstone 的唯一 owner。它把 owner notification 规范化为有界
审计记录，并提供有界 list/high-watermark query。

EventRecord、SSE 和 projection 都是非权威派生面。本组件不拥有 Workspace、Run、Result、Governance 或 SM-1～SM-4
状态，不从 audit replay 重建 authority，也不向 Orchestrator 发送 drive/evidence/lifecycle command。

## 组件交互及合同

| 交互方 | 方向 | typed EventHub 合同 | 调用/授权语义 |
| --- | --- | --- | --- |
| Workspace、Run、Result、Decision、Approval、Memory、Governance、LLM | owner → Eventstream | 各 producer 定义的 concrete notification data | 校验 concrete payload 与 exact producer source |
| Approval | Approval → Eventstream | `RecordApprovalResolutionCommand/Result` | exact Approval source；同步 durable checkpoint |
| `application/workorchd` | Application → Eventstream | `ListEventsCommand/Result`、`HighWatermarksCommand/Result` | 两个 query topic 均只接受 exact owner source `application/workorchd` |
| Workspace | Workspace → Eventstream | `SealWorkspaceEventStreamDeletionCommand/Result`、`PurgeWorkspaceCommand/Result` | exact Workspace source 与完整 deletion scope |
| Application projection/SSE | Eventstream → Application | `EventRecordAppendedData` | append 成功后的只读 fact，不可回写 owner |
| repository backend | Eventstream → repository | EventRecord、stream-position、deletion repositories | owner-private persistence |

Eventstream 自有 topic/result 位于 [`pkg/events`](pkg/events)。public `EventQuery` 不暴露 repository query；字符串最多
4096 bytes、exclude topic 最多 128、limit 默认 100/最大 5000。owner 在 append 前把来源时间规范为 canonical UTC
RFC3339Nano；repository 拒绝非当前形态，读取损坏时间时 fail-closed。查询边界同样规范化，要求
`OccurredAfter <= OccurredBefore`，并以时间语义执行开区间过滤。
`HighWatermarksCommand` 原始 scope 数量为 1～500；每个 workspace ID 必须是最多 4096 bytes 的 UTF-8，owner
先完成全量校验再做任何 repository/deletion I/O。ID 经 trim 形成 canonical identity，空值 fail-closed，canonical
duplicate 按首次出现折叠，result 再按 workspace ID 排序。

### 生命周期合同

- 普通 command 与所有 matched notification 逐成功登记 topic/observer identity；部分 Start 失败保留 repository/订阅，未清理前不重复注册，退订成功才移除对应项。
- BeginShutdown 非等待关闭新删除命令；审计通知、Approval durable checkpoint 及只读 query 保留 drain，仍执行原 exact source/payload/scope/tombstone 校验，不从通知重建业务权威。
- handler 票覆盖真实 append/query 和后续通知；Post 的 nil Result 不影响真实完成记账。Quiesce 超时保留依赖；最终 StopChecked 封全部入站并再次排空，逆序 checked 退订全部成功才清 owner/Module 引用。
- Configure 不能覆盖未释放 owner；plugin guard 保留 cause，未知 panic 不吞且释放清理 claim。调用方串行 Configure/Start/Stop；本地排空不替代共享任务与 Hub 屏障，不更改 SM-1～SM-4。
- `biz/owner_lifecycle_test.go`、`module_lifecycle_test.go` 覆盖普通/matched 全部切点、失败退订、真实 Send/Post 排空、超时重试与 framework guard。

## 不变量与非职责

- `StreamPosition` 是跨 workspace 的全局单调 replay identity；预留失败可留空洞，但重启或 purge 后不得复用。
- record `Revision` 是 workspace-local append generation；canonical `OccurredAt` 只是来源时间，不参与 cursor 顺序。
- Result、Decision、Approval 复用 producer `NotificationID`；Run、Memory recall、LLM usage 由 Eventstream 以
  workspace/run scope、topic 与 producer causation/trace/request identity 签发有界确定性 record ID。Run 的状态专用 topic 与
  `/run/state/changed` 即使 causation 相同也具有不同 ID。
- 同一稳定 ID 仅在 persisted source、topic、scope、subject 与 payload 全等时视为 exact replay；任一不等保留 conflict 并记录错误。
  producer source/topic/payload validation 仍可在 append 前更早 fail-closed。
  `workspace_definition_clarification` 的 pre-seal 通知保持 workspace-only scope（RunID/StepID 均空）：created 必须
  为 open，resolved 必须为 resolved，结束类型仅 `definition_candidate_adjusted` 或
  `definition_candidate_revision_superseded`，不得携带 Run continuation/resume/successor。两类通知均校验 exact
  workspace/ticket/resolution notification identity；普通 run-scoped ticket 仍要求 RunID。审计不授予准入或执行权威。
  未声明稳定 identity 的其它 notification 保持 append-per-delivery，不伪装成已去重。
- payload 在 persistence 前执行 typed validation、secret removal、string/slice/aggregate bound；list 默认不 hydrate payload。
- deletion tombstone 永久阻止该 workspace 再 append；purge 不能删除其它 workspace，也不能重置全局 position。
- append 成功后才发布 `EventRecordAppendedData`；repository failure 不得发布伪成功。
- `Start` 幂等，`Stop` 释放全部订阅；无 `Result` 的 deletion write 不得改变 tombstone。
- Application query source 在 normalization 和 repository/deletion I/O 前校验；producer notification 对每个已订阅 topic
  校验 exact owner source。

## 能力验收

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| typed notification → append | [`biz/event_stream_service_test.go`](biz/event_stream_service_test.go) 的 Decision/Approval/Governance/Run/Workspace/Result/Memory/LLM 用例 | 覆盖 |
| query bounds / RFC3339 canonical range | [`pkg/events/messages_test.go`](pkg/events/messages_test.go)、[`TestEventRecordRepositoryContract_Memory`](../../../pkg/repository/repository_contract_test.go)、[`TestEventRecordRepositoryContract_File`](../../../pkg/repository/repository_contract_test.go) | 覆盖 query 与双后端当前形态拒绝 |
| persisted record shape / payload sanitization | `TestEventRecordDataRejectsInvalidOrNonCanonicalOccurredAt`、`TestServiceCanonicalizesOccurredAtBeforePersistence`、[`pkg/recordpayload`](pkg/recordpayload) tests | 覆盖 |
| stable replay / drift | `TestServiceStableOwnerNotificationReplayRejectsSourceTopicAndPayloadDrift`、`TestRunStableNotificationIdentitySeparatesSpecificAndCanonicalTopics`、`TestServiceDeduplicatesStableResultNotificationReplay`、`TestServiceDeduplicatesStableDecisionNotificationReplay`、`TestServiceDurablyAcknowledgesExactApprovalResolutionReplay` | 覆盖已声明稳定 identity 的 exact replay、Run 双 topic 分离及 source/topic/payload conflict |
| source / negative / append failure | `TestServiceRejectsForgedSourceForEverySubscribedProducerTopic`、`TestServiceRejectsForgedOwnerNotificationSource`、`TestServiceDoesNotPublishAppendedWhenRepositoryAppendFails` | 覆盖全部已订阅 producer-topic wrong-source 矩阵、exact Workspace 正向代表项及 append failure |
| deletion stale/replay/concurrency/effect | [`biz/workspace_deletion_test.go`](biz/workspace_deletion_test.go) | 覆盖 |
| restart / global position | repository memory/file contract、file purge-swap tests、`TestServicePurgePermanentlyFencesWorkspaceAndPreservesGlobalPosition` | 覆盖 |
| lifecycle | `TestServiceStartIsIdempotentAndStopReleasesSubscription`、`TestModuleRunIsIdempotentAndTeardownUnsubscribes` | 以 Run notification 代表项覆盖共享订阅 fence 的重复 Start、Stop 后零投递与 module teardown |
| high-watermark input bounds | `TestHighWatermarksCommandNormalizedCanonicalizesAndDeduplicates`、`TestHighWatermarksCommandNormalizedRejectsInvalidItemsAndCount`、`TestHighWatermarksFailsClosedBeforeOwnerIOAndDeduplicatesCanonicalIDs` | 覆盖 count、canonical trim、blank/UTF-8/item byte bound、duplicate collapse 与 owner I/O 前 fail-closed |
| Application caller authorization | `TestApplicationQueriesRequireExactSourceBeforeOwnerIO`、`TestListEventRecordsUsesCanonicalApplicationOwnerSource` | 覆盖合法 caller 使用 canonical owner source，以及 list/high-watermark forged/noncanonical source 在 owner I/O 前 fail-closed |
| provider / external lifecycle | 不适用 | 不适用 |
