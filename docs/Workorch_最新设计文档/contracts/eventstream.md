# eventstream：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页6个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Eventstream拥有审计记录与position/high-watermark。Append是同步持久事实能力；RecordAppended是异步通知，二者不可互换。workspace scope、有限过滤/分页、record typed value和redaction按其真实schema。

事件不是恢复authority：日志丢失、重复、乱序、SSE断线都不能驱动业务reducer或重做effect。删除时position floor/tombstone不复用；WorkspaceDeleted在最终deletion fence之后仅给展示传输，不回灌审计造成复生。

## 逐topic合同

<a id="ct-eventstream-eventstreamcommandhighwatermarks"></a>

### CT-eventstream-EventStreamCommandHighWatermarks

- 边界：`TopicEventStreamCommandHighWatermarks` = `/eventstream/command/high-watermarks`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L236)
- 接收provider：eventstream；Command [HighWatermarksCommand](#schema-highwatermarkscommand) → Result [HighWatermarksResult](#schema-highwatermarksresult)；target合法caller：application/workorchd
- 实际注册：[subscribeOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/owner_lifecycle.go#L40) → `subscribeOwnerCommand`（eventstream）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L135) → `handleCommand`（eventstream）
- 接收入口：[subscribeOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/owner_lifecycle.go#L40)；[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L176)
- 已观察外部引用：workorchd [readWorkspaceProjectionHighWatermarks](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L339)
- 本topic入口/具体类型分支的Source审查索引：[executeCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L245)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != applicationCommandSource` 分支谓词（[executeCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L245)）
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`ev == nil || result == nil`；`result == nil`；`ev.Source() != applicationCommandSource`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-eventstream-eventstreamcommandlist"></a>

### CT-eventstream-EventStreamCommandList

- 边界：`TopicEventStreamCommandList` = `/eventstream/command/list`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/topics.go#L5)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L235)
- 接收provider：eventstream；Command [ListEventsCommand](#schema-listeventscommand) → Result [ListEventsResult](#schema-listeventsresult)；target合法caller：application/workorchd
- 实际注册：[subscribeOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/owner_lifecycle.go#L40) → `subscribeOwnerCommand`（eventstream）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L135) → `handleCommand`（eventstream）
- 接收入口：[subscribeOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/owner_lifecycle.go#L40)；[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L176)
- 已观察外部引用：workorchd [ListEventRecordsByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/eventstream_reads.go#L16)
- 本topic入口/具体类型分支的Source审查索引：[executeCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L245)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != applicationCommandSource` 分支谓词（[executeCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L245)）
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`ev == nil || result == nil`；`result == nil`；`ev.Source() != applicationCommandSource`；`query.WorkspaceID != ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-eventstream-eventstreamcommandpurgeworkspace"></a>

### CT-eventstream-EventStreamCommandPurgeWorkspace

- 边界：`TopicEventStreamCommandPurgeWorkspace` = `/eventstream/command/purge-workspace`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L238)
- 接收provider：eventstream；Command [PurgeWorkspaceCommand](#schema-purgeworkspacecommand) → Result [PurgeWorkspaceResult](#schema-purgeworkspaceresult)；target合法caller：workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L135) → `handleCommand`（eventstream）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L176)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextPurge](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L304)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/workspace_deletion_internal_event.go#L44)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-eventstream-eventstreamcommandrecordapprovalresolution"></a>

### CT-eventstream-EventStreamCommandRecordApprovalResolution

- 边界：`TopicEventStreamCommandRecordApprovalResolution` = `/eventstream/command/record-approval-resolution`；CF11；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L239)
- 接收provider：eventstream；Command [RecordApprovalResolutionCommand](#schema-recordapprovalresolutioncommand) → Result [RecordApprovalResolutionResult](#schema-recordapprovalresolutionresult)；target合法caller：blocks/approval
- 实际注册：[subscribeOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/owner_lifecycle.go#L40) → `subscribeOwnerCommand`（eventstream）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L135) → `handleCommand`（eventstream）
- 接收入口：[subscribeOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/owner_lifecycle.go#L40)；[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L176)
- 已观察外部引用：approval [recordApprovalResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/approval_notification.go#L66)
- 本topic入口/具体类型分支的Source审查索引：[executeRecordApprovalResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L200)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != approvalcommon.ModuleID` 分支谓词（[executeRecordApprovalResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L200)）
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`ev == nil || result == nil`；`result == nil`；`s == nil || s.repo == nil || ev == nil || ev.Source() != approvalcommon.ModuleID`；`!ok || command.Data.Validate() != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-eventstream-eventstreamcommandsealworkspacedeletion"></a>

### CT-eventstream-EventStreamCommandSealWorkspaceDeletion

- 边界：`TopicEventStreamCommandSealWorkspaceDeletion` = `/eventstream/command/seal-workspace-deletion`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L237)
- 接收provider：eventstream；Command [SealWorkspaceEventStreamDeletionCommand](#schema-sealworkspaceeventstreamdeletioncommand) → Result [SealWorkspaceEventStreamDeletionResult](#schema-sealworkspaceeventstreamdeletionresult)；target合法caller：workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L135) → `handleCommand`（eventstream）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L176)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextSeal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L221)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/workspace_deletion_internal_event.go#L44)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-eventstream-eventstreamrecordappended"></a>

### CT-eventstream-EventStreamRecordAppended

- 边界：`TopicEventStreamRecordAppended` = `/eventstream/record/appended`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L240)
- 唯一事实owner/发布者：eventstream；Data [EventRecordAppendedData](#schema-eventrecordappendeddata)；同步Command/Result：不适用；允许订阅者：process/workorch、workorchd
- 实际注册：[newEventCollector](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/runtimecore/event_collector.go#L29) → `newEventCollector`（workorchd）；[Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53) → `handleEventRecordAppended`（workorchd）
- 已读事实来源引用（含发布/分派路径）：[publishRecordAppended](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L1102)
- 已观察外部引用：workorchd [Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53)；workorchd [newEventCollector](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/runtimecore/event_collector.go#L29)；workorchd [Close](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/runtimecore/event_collector.go#L65)；process/workorch [Subscribe](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L85)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-approvalresolutionfact"></a>

### ApprovalResolutionFact

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:192](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L192)
- 字段：`NotificationID string`；`ResolutionID common.ResolutionID`；`ApprovalID common.ApprovalID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`Reason string`；`Action string`；`Context []workorcheventpayload.Field`；`ResolutionContext []workorcheventpayload.Field`；`PrincipalKind string`；`PrincipalSubjectID string`；`State common.ApprovalState`
- 字段引用类型：[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[Field（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L27)；[ApprovalState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L13)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L208)
- 空值/identity/版本/范围原式：`strings.TrimSpace(d.NotificationID) == "" || d.ResolutionID == "" || d.ApprovalID == "" || d.WorkspaceID == "" || d.RunID == "" || strings.TrimSpace(d.PrincipalSubjectID) == ""`；`d.State != common.ApprovalStateApproved && d.State != common.ApprovalStateRejected`
- 显式分支集合（只表列源码case，default语义见validator）：`"human_operator", "auto_governance"`

<a id="schema-eventquery"></a>

### EventQuery

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L31)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Topic string`；`ExcludeTopics []string`；`SubjectType string`；`SubjectID string`；`CursorAfterID common.EventRecordID`；`OccurredAfter string`；`OccurredBefore string`；`Limit int`；`IncludePayload bool`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[EventRecordID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L20)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L45)
- 空值/identity/版本/范围原式：`len(value) > MaxQueryStringBytes || !utf8.ValidString(value)`；`len(q.ExcludeTopics) > MaxExcludeTopics`；`len(rawTopic) > MaxQueryStringBytes || !utf8.ValidString(rawTopic)`；`topic == ""`；`_, ok := seen[topic]; ok`；`err != nil`；`q.Limit == 0`；`q.Limit < 1 || q.Limit > MaxListLimit`

<a id="schema-eventrecordappendeddata"></a>

### EventRecordAppendedData

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:185](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L185)
- 字段：`Record EventRecordData`
- 字段引用类型：[EventRecordData](#schema-eventrecorddata)

<a id="schema-eventrecorddata"></a>

### EventRecordData

- 源码：[internal/modules/blocks/eventstream/pkg/events/record_value.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/record_value.go#L15)
- 声明：`type EventRecordData workorcheventrecordcontract.Record`
- 字段引用类型：[Record（shared/workorcheventrecordcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventrecordcontract/record.go#L32)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/record_value.go#L17)
- 空值/identity/版本/范围原式：`item.ID == "" || strings.TrimSpace(item.Version) == "" || strings.TrimSpace(item.Topic) == "" || strings.TrimSpace(item.OccurredAt) == "" || strings.TrimSpace(item.SourceModule) == "" || strings.TrimSpace(item.SubjectType) == "" || strings.TrimSpace(item.SubjectID) == ""`；`item.WorkspaceID != "" && item.Revision < 1`；`item.StreamPosition < 1`；`err != nil`；`canonicalOccurredAt != item.OccurredAt`；`!recordpayload.SanitizeAuditPayload(item.Payload).Equal(item.Payload)`；`!recordpayload.SanitizeAuditPayload(item.PayloadSummary).Equal(item.PayloadSummary)`

<a id="schema-highwatermarkscommand"></a>

### HighWatermarksCommand

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:103](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L103)
- 字段：`WorkspaceIDs []common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L110)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L147)
- 空值/identity/版本/范围原式：`len(c.WorkspaceIDs) == 0 || len(c.WorkspaceIDs) > MaxHighWatermarkScopes`；`len(raw) > MaxHighWatermarkWorkspaceIDBytes || !utf8.ValidString(raw)`；`workspaceID == ""`；`_, ok := seen[workspaceID]; ok`；`err != nil`；`len(r.HighWatermarks) != len(command.WorkspaceIDs)`；`raw == "" || raw != strings.TrimSpace(raw) || len(raw) > MaxHighWatermarkWorkspaceIDBytes || !utf8.ValidString(raw) || item.Revision < 0`；`_, ok := expected[item.WorkspaceID]; !ok`；`_, ok := seen[item.WorkspaceID]; ok`；`index > 0 && raw <= previous`

<a id="schema-highwatermarksresult"></a>

### HighWatermarksResult

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:142](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L142)
- 字段：`HighWatermarks []WorkspaceHighWatermark`
- 字段引用类型：[WorkspaceHighWatermark](#schema-workspacehighwatermark)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L147)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L181)
- 空值/identity/版本/范围原式：`err != nil`；`len(r.HighWatermarks) != len(command.WorkspaceIDs)`；`raw == "" || raw != strings.TrimSpace(raw) || len(raw) > MaxHighWatermarkWorkspaceIDBytes || !utf8.ValidString(raw) || item.Revision < 0`；`_, ok := expected[item.WorkspaceID]; !ok`；`_, ok := seen[item.WorkspaceID]; ok`；`index > 0 && raw <= previous`

<a id="schema-listeventscommand"></a>

### ListEventsCommand

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:99](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L99)
- 字段：`Query EventQuery`
- 字段引用类型：[EventQuery](#schema-eventquery)

<a id="schema-listeventsresult"></a>

### ListEventsResult

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:140](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L140)
- 字段：`Events []EventRecordData`
- 字段引用类型：[EventRecordData](#schema-eventrecorddata)

<a id="schema-purgeworkspacecommand"></a>

### PurgeWorkspaceCommand

- 源码：[internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go:39](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L39)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L47)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L54)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L108)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableEventStreamDeletionFenceID(scope) || r.PurgeID != StableEventStreamDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash != StableEventStreamDeletionPurgeProofHash(scope) || r.AckHash != StableEventStreamDeletionPurgeAckHash(scope)`

<a id="schema-purgeworkspaceresult"></a>

### PurgeWorkspaceResult

- 源码：[internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go:58](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L58)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`PurgeID string`；`Revision uint64`；`AckHash string`；`PurgeProofHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L108)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableEventStreamDeletionFenceID(scope) || r.PurgeID != StableEventStreamDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash != StableEventStreamDeletionPurgeProofHash(scope) || r.AckHash != StableEventStreamDeletionPurgeAckHash(scope)`

<a id="schema-recordapprovalresolutioncommand"></a>

### RecordApprovalResolutionCommand

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:223](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L223)
- 字段：`Data ApprovalResolutionFact`
- 字段引用类型：[ApprovalResolutionFact](#schema-approvalresolutionfact)

<a id="schema-recordapprovalresolutionresult"></a>

### RecordApprovalResolutionResult

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:225](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L225)
- 字段：`NotificationID string`；`ResolutionID common.ResolutionID`；`ApprovalID common.ApprovalID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`
- 字段引用类型：[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-sealworkspaceeventstreamdeletioncommand"></a>

### SealWorkspaceEventStreamDeletionCommand

- 源码：[internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go:12](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L12)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`DataDispositionIntent deletioncontract.Mode`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L21)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L28)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L96)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableEventStreamDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableEventStreamDeletionSealAckHash(scope)`

<a id="schema-sealworkspaceeventstreamdeletionresult"></a>

### SealWorkspaceEventStreamDeletionResult

- 源码：[internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go:30](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L30)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`Revision uint64`；`AckHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/workspace_deletion.go#L96)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableEventStreamDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableEventStreamDeletionSealAckHash(scope)`

<a id="schema-workspacehighwatermark"></a>

### WorkspaceHighWatermark

- 源码：[internal/modules/blocks/eventstream/pkg/events/messages.go:135](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/pkg/events/messages.go#L135)
- 字段：`WorkspaceID common.WorkspaceID`；`Revision int64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [executeCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L245)：`ev.Source() != applicationCommandSource`
  - 当前来源常量：`applicationCommandSource = "application/workorchd"`
- [executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/workspace_deletion_internal_event.go#L44)：`s == nil || s.workspaceDeletions == nil || s.workspacePurger == nil || ev == nil || ev.Source() != workspaceDeletionCommandSource`
  - 当前来源常量：`workspaceDeletionCommandSource = "blocks/workspace"`
- [executeRecordApprovalResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L200)：`s == nil || s.repo == nil || ev == nil || ev.Source() != approvalcommon.ModuleID`
  - 当前来源常量：`approvalcommon.ModuleID = "blocks/approval"`