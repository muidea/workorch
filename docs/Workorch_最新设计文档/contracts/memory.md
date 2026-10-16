# memory：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页14个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Memory管理知识条目及recall trace，不把历史知识当当前验证证据。Create/Update绑定Workspace scope；RunID仅在确有Run来源时提供，普通记忆管理不伪造Run。TraceID/WorkspaceID/RunID为trace必填；finite limits、非NaN/Inf、quota key唯一与recall selection evidence由真实validator检查。

item/trace写入口经workspace mutation lock及WithWorkspaceActiveFence；删除seal/purge是独立family。query按workspace与visibility/retrieval bounds，不返回mutable repository。候选/selected/retrieved/实际注入必须区分，不能以回忆打分覆盖Goal/DoD。

## 逐topic合同

<a id="ct-memory-memorycommandcreate"></a>

### CT-memory-MemoryCommandCreate

- 边界：`TopicMemoryCommandCreate` = `/memory/command/create`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L503)
- 接收provider：memory；Command [CreateMemoryCommand](#schema-creatememorycommand) → Result [CreateMemoryResult](#schema-creatememoryresult)；target合法caller：workorchd、system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：workorchd [PublishMemoryItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/memory_commands.go#L13)；workorchd [CreateMemoryItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/memory_commands.go#L37)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`err := validateMemoryCommand(cmd); err != nil`；`cmd.MemoryID == "" || cmd.WorkspaceID == "" || strings.TrimSpace(cmd.SourceType) == "" || strings.TrimSpace(cmd.MemoryType) == "" || strings.TrimSpace(cmd.Summary) == ""`；`!modulecommon.IsSupportedSourceType(cmd.SourceType) || !modulecommon.IsSupportedMemoryType(cmd.MemoryType)`；`!modulecommon.IsWritableMemoryTypeForSource(cmd.SourceType, cmd.MemoryType)`；`hasBlank(cmd.Tags)`；`len(cmd.Vector) > modulecommon.MemoryVectorMaxDimension`；`cmd.SourceType != modulecommon.MemorySourceUser && strings.TrimSpace(cmd.SourceRef) == ""`；`cmd.SourceType == modulecommon.MemorySourceUser && strings.TrimSpace(cmd.SourceRef) == "" && cmd.MemoryType != modulecommon.MemoryTypeProjectContext && cmd.MemoryType != modulecommon.MemoryTypeUserPreference`；`modulecommon.ContainsSensitiveMemoryText(cmd.Summary, cmd.Content, cmd.EmbeddingText, strings.Join(cmd.Tags, " "))`；其余分支见上述入口全函数及本页schema校验
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-memory-memorycommanddelete"></a>

### CT-memory-MemoryCommandDelete

- 边界：`TopicMemoryCommandDelete` = `/memory/command/delete`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L16)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L508)
- 接收provider：memory；Command [DeleteMemoryCommand](#schema-deletememorycommand) → Result [DeleteMemoryResult](#schema-deletememoryresult)；target合法caller：workorchd、system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：workorchd [DeleteMemoryItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/memory_commands.go#L61)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || cmd.WorkspaceID == "" || cmd.MemoryID == ""`；`err := m.service.deleteMemoryItem(ctx, cmd.MemoryID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-memory-memorycommandget"></a>

### CT-memory-MemoryCommandGet

- 边界：`TopicMemoryCommandGet` = `/memory/command/get`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L504)
- 接收provider：memory；Command [GetMemoryCommand](#schema-getmemorycommand) → Result [GetMemoryResult](#schema-getmemoryresult)；target合法caller：workorchd、system、orchestrator、run
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：workorchd [ReadMemoryItemByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/context_memory_reads.go#L15)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || cmd.WorkspaceID == "" || cmd.MemoryID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-memory-memorycommandlistbyworkspace"></a>

### CT-memory-MemoryCommandListByWorkspace

- 边界：`TopicMemoryCommandListByWorkspace` = `/memory/command/list-by-workspace`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L505)
- 接收provider：memory；Command [ListMemoryByWorkspaceCommand](#schema-listmemorybyworkspacecommand) → Result [ListMemoryByWorkspaceResult](#schema-listmemorybyworkspaceresult)；target合法caller：workorchd、system、orchestrator、run、kernel/governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：workorchd [ListMemoryItemsPageByWorkspaceOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/context_memory_reads.go#L41)；governance [ListByWorkspaceID](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_readers.go#L176)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || cmd.WorkspaceID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-memory-memorycommandpurgeworkspace"></a>

### CT-memory-MemoryCommandPurgeWorkspace

- 边界：`TopicMemoryCommandPurgeWorkspace` = `/memory/command/purge-workspace`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L20)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L512)
- 接收provider：memory；Command [PurgeWorkspaceMemoryCommand](#schema-purgeworkspacememorycommand) → Result [PurgeWorkspaceMemoryResult](#schema-purgeworkspacememoryresult)；target合法caller：workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextPurge](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L304)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/workspace_deletion_internal_event.go#L61)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-memory-memorycommandsealworkspacedeletion"></a>

### CT-memory-MemoryCommandSealWorkspaceDeletion

- 边界：`TopicMemoryCommandSealWorkspaceDeletion` = `/memory/command/seal-workspace-deletion`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L19)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L511)
- 接收provider：memory；Command [SealWorkspaceMemoryDeletionCommand](#schema-sealworkspacememorydeletioncommand) → Result [SealWorkspaceMemoryDeletionResult](#schema-sealworkspacememorydeletionresult)；target合法caller：workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextSeal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L221)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/workspace_deletion_internal_event.go#L61)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-memory-memorycommandsearch"></a>

### CT-memory-MemoryCommandSearch

- 边界：`TopicMemoryCommandSearch` = `/memory/command/search`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L506)
- 接收provider：memory；Command [SearchMemoryCommand](#schema-searchmemorycommand) → Result [SearchMemoryResult](#schema-searchmemoryresult)；target合法caller：workorchd、system、orchestrator、run
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：workorchd [SearchMemoryByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/context_memory_reads.go#L54)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || cmd.Query.WorkspaceID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-memory-memorycommandupdate"></a>

### CT-memory-MemoryCommandUpdate

- 边界：`TopicMemoryCommandUpdate` = `/memory/command/update`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L15)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L507)
- 接收provider：memory；Command [UpdateMemoryCommand](#schema-updatememorycommand) → Result [UpdateMemoryResult](#schema-updatememoryresult)；target合法caller：workorchd、system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：workorchd [PublishMemoryItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/memory_commands.go#L13)；workorchd [UpdateMemoryItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/memory_commands.go#L49)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`_, err := m.service.getMemoryItemScoped(ctx, cmd.WorkspaceID, cmd.MemoryID); err != nil`；`err := m.service.updateMemoryItem(ctx, memoryevents.CreateMemoryCommand(cmd)); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-memory-memorycreated"></a>

### CT-memory-MemoryCreated

- 边界：`TopicMemoryCreated` = `/memory/created`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L519)
- 唯一事实owner/发布者：memory；Data [MemoryChangedData](#schema-memorychangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeMemoryNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L319) → `subscribeMemoryNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[executeMemoryItemCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/item_commands.go#L11)
- 已观察外部引用：eventstream [subscribeMemoryNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L319)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-memory-memorydeleted"></a>

### CT-memory-MemoryDeleted

- 边界：`TopicMemoryDeleted` = `/memory/deleted`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L519)
- 唯一事实owner/发布者：memory；Data [MemoryChangedData](#schema-memorychangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeMemoryNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L319) → `subscribeMemoryNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[executeMemoryItemCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/item_commands.go#L11)
- 已观察外部引用：eventstream [subscribeMemoryNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L319)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-memory-memoryrecallrecorded"></a>

### CT-memory-MemoryRecallRecorded

- 边界：`TopicMemoryRecallRecorded` = `/memory/recall/recorded`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L521)
- 唯一事实owner/发布者：memory；Data [MemoryRecallRecordedData](#schema-memoryrecallrecordeddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeMemoryNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L319) → `subscribeMemoryNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[publishMemoryRecallRecorded](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/memory_notification.go#L26)
- 已观察外部引用：eventstream [subscribeMemoryNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L319)；eventstream [memoryRecallRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L997)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-memory-memoryrecalltracecommandlist"></a>

### CT-memory-MemoryRecallTraceCommandList

- 边界：`TopicMemoryRecallTraceCommandList` = `/memory/recall/command/list`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L18)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L510)
- 接收provider：memory；Command [ListMemoryRecallTracesCommand](#schema-listmemoryrecalltracescommand) → Result [ListMemoryRecallTracesResult](#schema-listmemoryrecalltracesresult)；target合法caller：workorchd、system、orchestrator、run
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：workorchd [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/readmodelports/observability_readers.go#L28)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || cmd.Query.WorkspaceID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-memory-memoryrecalltracecommandupsert"></a>

### CT-memory-MemoryRecallTraceCommandUpsert

- 边界：`TopicMemoryRecallTraceCommandUpsert` = `/memory/recall/command/upsert`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L17)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L509)
- 接收provider：memory；Command [UpsertMemoryRecallTraceCommand](#schema-upsertmemoryrecalltracecommand) → Result [UpsertMemoryRecallTraceResult](#schema-upsertmemoryrecalltraceresult)；target合法caller：blocks/run
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/owner.go#L47) → `handleCommand`（memory）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/command_router.go#L55)
- 已观察外部引用：run [publishRuntimeActivationMemoryRecall](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/activation_memory_recall.go#L29)
- 本topic入口/具体类型分支的Source审查索引：[executeMemoryRecallTraceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/recall_trace_commands.go#L12)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runcommon.ModuleID` 分支谓词（[executeMemoryRecallTraceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/recall_trace_commands.go#L12)）
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || ev.Source() != runcommon.ModuleID`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-memory-memoryupdated"></a>

### CT-memory-MemoryUpdated

- 边界：`TopicMemoryUpdated` = `/memory/updated`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L519)
- 唯一事实owner/发布者：memory；Data [MemoryChangedData](#schema-memorychangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeMemoryNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L319) → `subscribeMemoryNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[executeMemoryItemCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/item_commands.go#L11)
- 已观察外部引用：eventstream [subscribeMemoryNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L319)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-creatememorycommand"></a>

### CreateMemoryCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:20](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L20)
- 字段：`MemoryID common.MemoryID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`SourceType string`；`SourceRef string`；`MemoryType string`；`Content string`；`Summary string`；`Tags []string`；`Importance int`；`Confidence int`；`Visibility string`；`EmbeddingProviderID string`；`EmbeddingModel string`；`EmbeddingText string`；`Vector []float64`
- 字段引用类型：[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-creatememoryresult"></a>

### CreateMemoryResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:188](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L188)
- 字段：`WorkspaceID common.WorkspaceID`；`MemoryID common.MemoryID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)

<a id="schema-deletememorycommand"></a>

### DeleteMemoryCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:41](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L41)
- 字段：`WorkspaceID common.WorkspaceID`；`MemoryID common.MemoryID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)

<a id="schema-deletememoryresult"></a>

### DeleteMemoryResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:223](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L223)
- 字段：`WorkspaceID common.WorkspaceID`；`MemoryID common.MemoryID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)

<a id="schema-getmemorycommand"></a>

### GetMemoryCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:169](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L169)
- 字段：`WorkspaceID common.WorkspaceID`；`MemoryID common.MemoryID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)

<a id="schema-getmemoryresult"></a>

### GetMemoryResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:193](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L193)
- 字段：`Item *memorymodel.MemoryItem`
- 字段引用类型：[MemoryItem](#schema-memoryitem)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L197)

<a id="schema-listmemorybyworkspacecommand"></a>

### ListMemoryByWorkspaceCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:174](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L174)
- 字段：`WorkspaceID common.WorkspaceID`；`CursorAfterID common.MemoryID`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)

<a id="schema-listmemorybyworkspaceresult"></a>

### ListMemoryByWorkspaceResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:201](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L201)
- 字段：`Items []*memorymodel.MemoryItem`；`NextCursor common.MemoryID`
- 字段引用类型：[MemoryItem](#schema-memoryitem)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L206)

<a id="schema-listmemoryrecalltracescommand"></a>

### ListMemoryRecallTracesCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:184](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L184)
- 字段：`Query workorchquerycontract.MemoryRecallTraceQuery`
- 字段引用类型：[MemoryRecallTraceQuery（shared/workorchquerycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchquerycontract/query.go#L77)

<a id="schema-listmemoryrecalltracesresult"></a>

### ListMemoryRecallTracesResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:254](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L254)
- 字段：`Traces []*MemoryRecallTrace`；`NextCursor common.MemoryRecallTraceID`
- 字段引用类型：[MemoryRecallTrace](#schema-memoryrecalltrace)；[MemoryRecallTraceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L23)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L259)

<a id="schema-memorychangeddata"></a>

### MemoryChangedData

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:283](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L283)
- 字段：`MemoryID common.MemoryID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`MemoryType string`；`Summary string`；`Content string`；`SourceType string`；`SourceRef string`；`Tags []string`；`Importance int`；`Confidence int`；`Visibility string`
- 字段引用类型：[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-memoryitem"></a>

### MemoryItem

- 源码：[internal/modules/blocks/memory/pkg/models/memory_item.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/models/memory_item.go#L5)
- 字段：`ID common.MemoryID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`SourceType string`；`SourceRef string`；`MemoryType string`；`Content string`；`Summary string`；`Tags []string`；`Importance int`；`Confidence int`；`Visibility string`；`EmbeddingProviderID string`；`EmbeddingModel string`；`EmbeddingText string`；`Vector []float64`；`VectorScore float64`
- 字段引用类型：[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)
- 实际方法：[CloneMemoryItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/models/memory_item.go#L25)；[CloneMemoryItems](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/models/memory_item.go#L35)

<a id="schema-memoryrecallrecordeddata"></a>

### MemoryRecallRecordedData

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:342](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L342)
- 字段：`TraceID common.MemoryRecallTraceID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`Stage string`；`Source string`；`Policy string`；`Status string`；`ProviderID string`；`Operation string`；`RetrievedMemoryIDs []common.MemoryID`；`SelectionEvidence *workorchmemorycontract.RecallSelectionEvidence`；`Reason string`；`OccurredAt string`
- 字段引用类型：[MemoryRecallTraceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L23)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)；[RecallSelectionEvidence（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall_selection.go#L41)

<a id="schema-memoryrecalltrace"></a>

### MemoryRecallTrace

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:362](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L362)
- 声明：`type MemoryRecallTrace UpsertMemoryRecallTraceCommand`
- 字段引用类型：[UpsertMemoryRecallTraceCommand](#schema-upsertmemoryrecalltracecommand)
- 实际方法：[cloneMemoryRecallTrace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L407)；[cloneMemoryRecallTraces](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L418)

<a id="schema-memoryrecalltracewritedisposition"></a>

### MemoryRecallTraceWriteDisposition

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:228](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L228)
- 声明：`type MemoryRecallTraceWriteDisposition string`
- 源码枚举值：`MemoryRecallTraceRecorded = "recorded"`；`MemoryRecallTraceDuplicate = "duplicate"`

<a id="schema-purgeworkspacememorycommand"></a>

### PurgeWorkspaceMemoryCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:153](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L153)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L161)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L165)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/workspace_deletion.go#L51)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableMemoryDeletionFenceID(scope) || r.PurgeID != StableMemoryDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash != StableMemoryDeletionPurgeProofHash(scope) || r.AckHash != StableMemoryDeletionPurgeAckHash(scope)`

<a id="schema-purgeworkspacememoryresult"></a>

### PurgeWorkspaceMemoryResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:272](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L272)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`PurgeID string`；`Revision uint64`；`AckHash string`；`PurgeProofHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/workspace_deletion.go#L51)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableMemoryDeletionFenceID(scope) || r.PurgeID != StableMemoryDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash != StableMemoryDeletionPurgeProofHash(scope) || r.AckHash != StableMemoryDeletionPurgeAckHash(scope)`

<a id="schema-sealworkspacememorydeletioncommand"></a>

### SealWorkspaceMemoryDeletionCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:138](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L138)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`DataDispositionIntent deletioncontract.Mode`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L147)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L151)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/workspace_deletion.go#L39)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableMemoryDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableMemoryDeletionSealAckHash(scope)`

<a id="schema-sealworkspacememorydeletionresult"></a>

### SealWorkspaceMemoryDeletionResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:263](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L263)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`Revision uint64`；`AckHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/workspace_deletion.go#L39)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableMemoryDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableMemoryDeletionSealAckHash(scope)`

<a id="schema-searchmemorycommand"></a>

### SearchMemoryCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:180](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L180)
- 字段：`Query workorchquerycontract.MemorySearchQuery`
- 字段引用类型：[MemorySearchQuery（shared/workorchquerycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchquerycontract/query.go#L36)

<a id="schema-searchmemoryresult"></a>

### SearchMemoryResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:210](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L210)
- 字段：`Items []*memorymodel.MemoryItem`
- 字段引用类型：[MemoryItem](#schema-memoryitem)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L214)

<a id="schema-updatememorycommand"></a>

### UpdateMemoryCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:39](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L39)
- 实际alias/同形类型：[CreateMemoryCommand](#schema-creatememorycommand)
- 字段引用类型：[CreateMemoryCommand](#schema-creatememorycommand)

<a id="schema-updatememoryresult"></a>

### UpdateMemoryResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:218](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L218)
- 字段：`WorkspaceID common.WorkspaceID`；`MemoryID common.MemoryID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)

<a id="schema-upsertmemoryrecalltracecommand"></a>

### UpsertMemoryRecallTraceCommand

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:46](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L46)
- 字段：`TraceID common.MemoryRecallTraceID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`GraphNodeID string`；`Stage string`；`Source string`；`Policy string`；`Status string`；`ProviderID string`；`ModelID string`；`Operation string`；`QueryTextPreview string`；`QueryTextSha256 string`；`MinScore float64`；`Limit int`；`EmbeddingModel string`；`VectorDimension int`；`RefreshSource string`；`SelectionPolicyVersion string`；`CandidateMemoryIDs []common.MemoryID`；`SelectedMemoryIDs []common.MemoryID`；`ExcludedMemoryIDs []common.MemoryID`；`TypeQuota []workorchmemorycontract.TypeQuotaEntry`；`SelectionScores []workorchmemorycontract.RecallSelectionScore`；`RetrievedMemoryIDs []common.MemoryID`；`RetrievedMemoryHits []workorchmemorycontract.RecallHit`；`ContextBudget workorchmemorycontract.RecallContextBudget`；`SemanticRecall *workorchmemorycontract.RecallSelectionEvidence`；`FallbackFrom string`；`Reason string`；`OccurredAt string`；`EvidenceRef string`
- 字段引用类型：[MemoryRecallTraceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L23)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)；[TypeQuotaEntry（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall.go#L24)；[RecallSelectionScore（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall.go#L17)；[RecallHit（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall.go#L9)；[RecallContextBudget（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall_selection.go#L20)；[RecallSelectionEvidence（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall_selection.go#L41)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L82)
- 空值/identity/版本/范围原式：`c.TraceID == "" || c.WorkspaceID == "" || c.RunID == ""`；`len(c.CandidateMemoryIDs) > workorchmemorycontract.RecallSelectionMaxCandidateCount || len(c.SelectedMemoryIDs) > workorchmemorycontract.RecallSelectionMaxSelectedCount || len(c.ExcludedMemoryIDs) > workorchmemorycontract.RecallSelectionMaxCandidateCount || len(c.SelectionScores) > workorchmemorycontract.RecallSelectionMaxCandidateCount || len(c.RetrievedMemoryIDs) > workorchmemorycontract.RecallSelectionMaxSelectedCount || len(c.RetrievedMemoryHits) > workorchmemorycontract.RecallSelectionMaxSelectedCount || math.IsNaN(c.MinScore) || math.IsInf(c.MinScore, 0) || c.Limit < 0 || c.VectorDimension < 0`；`key == "" || entry.Quota < 0`；`_, ok := seen[key]; ok`；`score.ID == "" || strings.TrimSpace(score.Reason) == ""`；`hit.ID == "" || hit.VectorDimension < 0 || math.IsNaN(hit.VectorScore) || math.IsInf(hit.VectorScore, 0)`；`strings.TrimSpace(c.Source) == "activation"`；`err := evidence.Validate(); err != nil`

<a id="schema-upsertmemoryrecalltraceresult"></a>

### UpsertMemoryRecallTraceResult

- 源码：[internal/modules/blocks/memory/pkg/events/messages.go:235](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L235)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`TraceID common.MemoryRecallTraceID`；`Disposition MemoryRecallTraceWriteDisposition`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[MemoryRecallTraceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L23)；[MemoryRecallTraceWriteDisposition](#schema-memoryrecalltracewritedisposition)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/pkg/events/messages.go#L242)
- 空值/identity/版本/范围原式：`r.WorkspaceID != expectedWorkspaceID || r.RunID != expectedRunID || r.TraceID != expectedTraceID`
- 显式分支集合（只表列源码case，default语义见validator）：`MemoryRecallTraceRecorded, MemoryRecallTraceDuplicate`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/workspace_deletion_internal_event.go#L61)：`ev == nil || ev.Source() != workspaceDeletionCommandSource || m == nil || m.workspaceDeletions == nil || m.service == nil`
  - 当前来源常量：`workspaceDeletionCommandSource = "blocks/workspace"`
- [executeMemoryRecallTraceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/memory/biz/recall_trace_commands.go#L12)：`!ok || ev.Source() != runcommon.ModuleID`
  - 当前来源常量：`runcommon.ModuleID = "blocks/run"`