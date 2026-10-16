# result：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页14个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Result接收Orchestrator的immutable terminal material，owner自己派生ResultID/ArtifactID、状态和时间。caller不能指定accepted等生命周期字段。带Artifact子项的delivery可先持久draft，全部children durable后完成terminal CAS；同delivery/material hash重放返回原ID，不重新验收。

顺序是Result→Workspace→Run；有效terminal seal、三条exact ack以及post_terminal_delivery lineage assertions全部成立才收口。Artifact内容读取有scope/大小/完整性边界，不通过metadata任意读取源文件；Result读到accepted也不能直接解锁Workspace新Start。

## 逐topic合同

<a id="ct-result-artifactcommandlistbyresult"></a>

### CT-result-ArtifactCommandListByResult

- 边界：`TopicArtifactCommandListByResult` = `/artifact/command/list-by-result`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L19)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L354)
- 接收provider：result；Command [ListArtifactsByResultCommand](#schema-listartifactsbyresultcommand) → Result [ListArtifactsByResultResult](#schema-listartifactsbyresultresult)；target合法caller：workorchd、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleCommand`（result）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L227)
- 已观察外部引用：workorchd [ListArtifactsByResultOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/artifact_result_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.ResultID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-result-artifactcommandlistbyrun"></a>

### CT-result-ArtifactCommandListByRun

- 边界：`TopicArtifactCommandListByRun` = `/artifact/command/list-by-run`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L18)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L353)
- 接收provider：result；Command [ListArtifactsByRunCommand](#schema-listartifactsbyruncommand) → Result [ListArtifactsByRunResult](#schema-listartifactsbyrunresult)；target合法caller：workorchd、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleCommand`（result）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L227)
- 已观察外部引用：workorchd [ListArtifactsByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/artifact_reads.go#L30)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.RunID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-result-artifactcommandlistbyworkspace"></a>

### CT-result-ArtifactCommandListByWorkspace

- 边界：`TopicArtifactCommandListByWorkspace` = `/artifact/command/list-by-workspace`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L17)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L352)
- 接收provider：result；Command [ListArtifactsByWorkspaceCommand](#schema-listartifactsbyworkspacecommand) → Result [ListArtifactsByWorkspaceResult](#schema-listartifactsbyworkspaceresult)；target合法caller：workorchd、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleCommand`（result）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L227)
- 已观察外部引用：workorchd [ListArtifactsByWorkspaceOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/artifact_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-result-artifactcommandreadcontent"></a>

### CT-result-ArtifactCommandReadContent

- 边界：`TopicArtifactCommandReadContent` = `/artifact/command/read-content`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L20)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L355)
- 接收provider：result；Command [ReadArtifactContentCommand](#schema-readartifactcontentcommand) → Result [ReadArtifactContentResult](#schema-readartifactcontentresult)；target合法caller：workorchd、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleCommand`（result）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L227)
- 已观察外部引用：workorchd [ReadArtifactContentByResultOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/artifact_content_read.go#L13)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.RunID == "" || strings.TrimSpace(cmd.URI) == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-result-resultaccepted"></a>

### CT-result-ResultAccepted

- 边界：`TopicResultAccepted` = `/result/accepted`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L365)
- 唯一事实owner/发布者：result；Data [ResultChangedData](#schema-resultchangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeResultNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L369) → `subscribeResultNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：workorchd [enrichSSEEnvelope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/sse_enrichment.go#L15)；eventstream [subscribeResultNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L369)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-result-resultcommandget"></a>

### CT-result-ResultCommandGet

- 边界：`TopicResultCommandGet` = `/result/command/get`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L349)
- 接收provider：result；Command [GetResultCommand](#schema-getresultcommand) → Result [GetResultResult](#schema-getresultresult)；target合法caller：workorchd、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleCommand`（result）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L227)
- 已观察外部引用：workorchd [ReadResultByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/result_read.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.ResultID == ""`；`item.WorkspaceID != cmd.WorkspaceID`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-result-resultcommandlistbyrun"></a>

### CT-result-ResultCommandListByRun

- 边界：`TopicResultCommandListByRun` = `/result/command/list-by-run`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L351)
- 接收provider：result；Command [ListResultsByRunCommand](#schema-listresultsbyruncommand) → Result [ListResultsByRunResult](#schema-listresultsbyrunresult)；target合法caller：workorchd、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleCommand`（result）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L227)
- 已观察外部引用：workorchd [ListResultsByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/result_reads.go#L30)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.RunID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-result-resultcommandlistbyworkspace"></a>

### CT-result-ResultCommandListByWorkspace

- 边界：`TopicResultCommandListByWorkspace` = `/result/command/list-by-workspace`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L350)
- 接收provider：result；Command [ListResultsByWorkspaceCommand](#schema-listresultsbyworkspacecommand) → Result [ListResultsByWorkspaceResult](#schema-listresultsbyworkspaceresult)；target合法caller：workorchd、orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleCommand`（result）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L227)
- 已观察外部引用：workorchd [ListResultsByWorkspaceOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/result_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-result-resultcommandpurgeworkspace"></a>

### CT-result-ResultCommandPurgeWorkspace

- 边界：`TopicResultCommandPurgeWorkspace` = `/result/command/purge-workspace`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L23)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L357)
- 接收provider：result；Command [PurgeWorkspaceResultsCommand](#schema-purgeworkspaceresultscommand) → Result [PurgeWorkspaceResultsResult](#schema-purgeworkspaceresultsresult)；target合法caller：workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleLifecycleCommand`（result）
- 接收入口：[handleLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/lifecycle_handler.go#L30)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextPurge](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L304)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/workspace_deletion_internal_event.go#L60)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-result-resultcommandsealworkspacedeletion"></a>

### CT-result-ResultCommandSealWorkspaceDeletion

- 边界：`TopicResultCommandSealWorkspaceDeletion` = `/result/command/seal-workspace-deletion`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L22)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L356)
- 接收provider：result；Command [SealWorkspaceResultDeletionCommand](#schema-sealworkspaceresultdeletioncommand) → Result [SealWorkspaceResultDeletionResult](#schema-sealworkspaceresultdeletionresult)；target合法caller：workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleLifecycleCommand`（result）
- 接收入口：[handleLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/lifecycle_handler.go#L30)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextSeal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L221)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/workspace_deletion_internal_event.go#L60)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-result-resultcreated"></a>

### CT-result-ResultCreated

- 边界：`TopicResultCreated` = `/result/created`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L365)
- 唯一事实owner/发布者：result；Data [ResultChangedData](#schema-resultchangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、workorchd
- 实际注册：[subscribeResultNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L369) → `subscribeResultNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeResultNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L369)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-result-resultfactterminaldelivery"></a>

### CT-result-ResultFactTerminalDelivery

- 边界：`TopicResultFactTerminalDelivery` = `/result/fact/terminal-delivery`；CF10；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L15)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L367)
- 接收provider：result；Command [RuntimeTerminalDeliveryFact](#schema-runtimeterminaldeliveryfact) → Result [ResultLifecycleCommandResult](#schema-resultlifecyclecommandresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L67) → `handleLifecycleCommand`（result）
- 接收入口：[handleLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/lifecycle_handler.go#L30)
- 已观察外部引用：orchestrator [deliverRuntimeResult](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_result_projection.go#L135)
- 本topic入口/具体类型分支的Source审查索引：[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/lifecycle_handler.go#L84)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != "kernel/orchestrator"` 分支谓词（[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/lifecycle_handler.go#L84)）
- 特化承诺：只kernel/orchestrator提供RuntimeTerminalDeliveryFact；根据Outcome派生Result状态，Material不携带owner生命周期字段；same delivery/material重放返回原ResultID，child Artifact未durable不ack终态。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != "kernel/orchestrator"`；`err := cmd.Validate(); err != nil`
- 验收适用集：[CP04](../Workorch_组件固定合同与分层验收规范.md#cp04)

<a id="ct-result-resultrejected"></a>

### CT-result-ResultRejected

- 边界：`TopicResultRejected` = `/result/rejected`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L365)
- 唯一事实owner/发布者：result；Data [ResultChangedData](#schema-resultchangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeResultNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L369) → `subscribeResultNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeResultNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L369)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-result-resultsuperseded"></a>

### CT-result-ResultSuperseded

- 边界：`TopicResultSuperseded` = `/result/superseded`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L365)
- 唯一事实owner/发布者：result；Data [ResultChangedData](#schema-resultchangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeResultNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L369) → `subscribeResultNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeResultNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L369)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-artifact"></a>

### Artifact

- 源码：[internal/modules/blocks/result/pkg/models/artifact.go:8](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/models/artifact.go#L8)
- 字段：`ID common.ArtifactID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`ResultID common.ResultID`；`StepID common.StepID`；`ToolCallID common.ToolCallID`；`PatchID common.PatchID`；`ValidationResultID common.ValidationResultID`；`ArtifactType string`；`Category string`；`Name string`；`Summary string`；`URI string`；`ContentType string`；`State common.ArtifactState`；`EvidenceRefs []string`；`Metadata workorcheventpayload.Payload`；`SourceRef string`；`TerminalDeliveryID string`；`MaterialHash string`；`CreatedAt string`；`UpdatedAt string`
- 字段引用类型：[ArtifactID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L16)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L15)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[ToolCallID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L12)；[PatchID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L13)；[ValidationResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L14)；[ArtifactState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L20)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-getresultcommand"></a>

### GetResultCommand

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:26](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L26)
- 字段：`WorkspaceID common.WorkspaceID`；`ResultID common.ResultID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L15)

<a id="schema-getresultresult"></a>

### GetResultResult

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:155](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L155)
- 字段：`Result *resultmodel.Result`
- 字段引用类型：[Result](#schema-result)

<a id="schema-listartifactsbyresultcommand"></a>

### ListArtifactsByResultCommand

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:57](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L57)
- 字段：`WorkspaceID common.WorkspaceID`；`ResultID common.ResultID`；`Cursor string`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L15)

<a id="schema-listartifactsbyresultresult"></a>

### ListArtifactsByResultResult

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:172](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L172)
- 字段：`Artifacts []*resultmodel.Artifact`；`NextCursor string`
- 字段引用类型：[Artifact](#schema-artifact)

<a id="schema-listartifactsbyruncommand"></a>

### ListArtifactsByRunCommand

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:50](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L50)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Cursor string`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-listartifactsbyrunresult"></a>

### ListArtifactsByRunResult

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:168](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L168)
- 字段：`Artifacts []*resultmodel.Artifact`；`NextCursor string`
- 字段引用类型：[Artifact](#schema-artifact)

<a id="schema-listartifactsbyworkspacecommand"></a>

### ListArtifactsByWorkspaceCommand

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:44](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L44)
- 字段：`WorkspaceID common.WorkspaceID`；`Cursor string`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-listartifactsbyworkspaceresult"></a>

### ListArtifactsByWorkspaceResult

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:164](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L164)
- 字段：`Artifacts []*resultmodel.Artifact`；`NextCursor string`
- 字段引用类型：[Artifact](#schema-artifact)

<a id="schema-listresultsbyruncommand"></a>

### ListResultsByRunCommand

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:37](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L37)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Cursor string`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-listresultsbyrunresult"></a>

### ListResultsByRunResult

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:160](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L160)
- 字段：`Results []*resultmodel.Result`；`NextCursor string`
- 字段引用类型：[Result](#schema-result)

<a id="schema-listresultsbyworkspacecommand"></a>

### ListResultsByWorkspaceCommand

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L31)
- 字段：`WorkspaceID common.WorkspaceID`；`Cursor string`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-listresultsbyworkspaceresult"></a>

### ListResultsByWorkspaceResult

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:156](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L156)
- 字段：`Results []*resultmodel.Result`；`NextCursor string`
- 字段引用类型：[Result](#schema-result)

<a id="schema-purgeworkspaceresultscommand"></a>

### PurgeWorkspaceResultsCommand

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:88](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L88)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L96)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L100)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/workspace_deletion.go#L51)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableResultDeletionFenceID(scope) || r.PurgeID != StableResultDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash != StableResultDeletionPurgeProofHash(scope) || r.AckHash != StableResultDeletionPurgeAckHash(scope)`

<a id="schema-purgeworkspaceresultsresult"></a>

### PurgeWorkspaceResultsResult

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:225](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L225)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`PurgeID string`；`Revision uint64`；`AckHash string`；`PurgeProofHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/workspace_deletion.go#L51)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableResultDeletionFenceID(scope) || r.PurgeID != StableResultDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash != StableResultDeletionPurgeProofHash(scope) || r.AckHash != StableResultDeletionPurgeAckHash(scope)`

<a id="schema-readartifactcontentcommand"></a>

### ReadArtifactContentCommand

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:67](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L67)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`URI string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-readartifactcontentresult"></a>

### ReadArtifactContentResult

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:180](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L180)
- 字段：`ArtifactID common.ArtifactID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`URI string`；`FileName string`；`ContentType string`；`Content []byte`；`ContentSHA256 string`
- 字段引用类型：[ArtifactID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L16)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L191)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L196)
- 空值/identity/版本/范围原式：`r.ArtifactID == "" || r.WorkspaceID == "" || r.RunID == "" || strings.TrimSpace(r.URI) == "" || strings.TrimSpace(r.FileName) == "" || strings.TrimSpace(r.ContentType) == "" || len(r.Content) == 0 || len(r.Content) > MaxArtifactContentBytes || len(strings.TrimSpace(r.ContentSHA256)) != sha256.Size*2`；`strings.Contains(r.FileName, "/") || strings.Contains(r.FileName, "\\") || r.FileName == "." || r.FileName == ".."`；`err != nil || len(hash) != sha256.Size`；`!strings.EqualFold(hex.EncodeToString(want[:]), r.ContentSHA256)`

<a id="schema-result"></a>

### Result

- 源码：[internal/modules/blocks/result/pkg/models/result.go:9](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/models/result.go#L9)
- 字段：`Revision uint64`；`ID common.ResultID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`GoalID common.GoalID`；`DoDID common.DoDID`；`State common.ResultState`；`Title string`；`Summary string`；`SourceType string`；`SourceRef string`；`EvidenceRefs []string`；`ArtifactRefs []common.ArtifactID`；`DoDAssessment workorchresultcontract.DoDAssessment`；`SummaryDetail workorchresultcontract.ResultSummary`；`AcceptedAt string`；`RejectedAt string`；`AcceptedBy string`；`CreatedAt string`；`UpdatedAt string`；`RuntimeCloseout *runtimecontract.RuntimeResultCloseout`；`TerminalMaterialHash string`；`TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity`
- 字段引用类型：[ResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L15)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[ResultState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L19)；[ArtifactID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L16)；[DoDAssessment（shared/workorchresultcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchresultcontract/result.go#L24)；[ResultSummary（shared/workorchresultcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchresultcontract/result.go#L11)；[RuntimeResultCloseout（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_result_closeout.go#L13)；[RuntimeTerminalDeliveryIdentity（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go#L35)

<a id="schema-resultchangeddata"></a>

### ResultChangedData

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:104](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L104)
- 字段：`NotificationID string`；`TerminalMaterialHash string`；`ResultID common.ResultID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`GoalID common.GoalID`；`DoDID common.DoDID`；`State common.ResultState`；`Title string`；`Summary string`；`SourceType string`；`SourceRef string`；`EvidenceRefs []string`；`ArtifactRefs []common.ArtifactID`
- 字段引用类型：[ResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L15)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[ResultState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L19)；[ArtifactID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L16)

<a id="schema-resultlifecyclecommandresult"></a>

### ResultLifecycleCommandResult

- 源码：[internal/modules/blocks/result/pkg/events/lifecycle_messages.go:118](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L118)
- 字段：`Result *resultmodel.Result`；`Transitioned bool`；`DeliveryAck *runtimecontract.RuntimeDeliveryAcknowledgement`
- 字段引用类型：[Result](#schema-result)；[RuntimeDeliveryAcknowledgement（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go#L160)

<a id="schema-resultmaterial"></a>

### ResultMaterial

- 源码：[internal/modules/blocks/result/pkg/events/lifecycle_messages.go:21](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L21)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`GoalID common.GoalID`；`DoDID common.DoDID`；`Title string`；`Summary string`；`SourceType string`；`SourceRef string`；`EvidenceRefs []string`；`ArtifactRefs []common.ArtifactID`；`DoDAssessment workorchresultcontract.DoDAssessment`；`SummaryDetail workorchresultcontract.ResultSummary`；`RuntimeCloseout *runtimecontract.RuntimeResultCloseout`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[ArtifactID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L16)；[DoDAssessment（shared/workorchresultcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchresultcontract/result.go#L24)；[ResultSummary（shared/workorchresultcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchresultcontract/result.go#L11)；[RuntimeResultCloseout（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_result_closeout.go#L13)
- 实际方法：[CloneRaw](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L48)；[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L52)；[ValidateRaw](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L56)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L60)；[CanonicalResultMaterialHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L69)

<a id="schema-runtimeterminaldeliveryfact"></a>

### RuntimeTerminalDeliveryFact

- 源码：[internal/modules/blocks/result/pkg/events/lifecycle_messages.go:76](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L76)
- 字段：`<embedded> runtimecontract.RuntimeTerminalDeliveryFact`；`Material ResultMaterial`
- 字段引用类型：[RuntimeTerminalDeliveryFact（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go#L74)；[ResultMaterial](#schema-resultmaterial)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L81)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L87)；[Identity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go#L113)
- 空值/identity/版本/范围原式：`err := c.Material.Validate(); err != nil`；`c.SourceOwner != "kernel/orchestrator"`；`err := c.RuntimeTerminalDeliveryFact.Validate(); err != nil`；`c.Material.WorkspaceID != c.WorkspaceID || c.Material.RunID != c.RunID`；`c.Material.RuntimeCloseout != nil && c.Material.RuntimeCloseout.CloseoutID != ""`；`closeout.DefinitionRevision != c.DefinitionRevision || closeout.TerminalOutcome != c.Outcome`

<a id="schema-sealworkspaceresultdeletioncommand"></a>

### SealWorkspaceResultDeletionCommand

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:73](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L73)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`DataDispositionIntent deletioncontract.Mode`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L82)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L86)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/workspace_deletion.go#L39)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableResultDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableResultDeletionSealAckHash(scope)`

<a id="schema-sealworkspaceresultdeletionresult"></a>

### SealWorkspaceResultDeletionResult

- 源码：[internal/modules/blocks/result/pkg/events/messages.go:216](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/messages.go#L216)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`Revision uint64`；`AckHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/workspace_deletion.go#L39)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableResultDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableResultDeletionSealAckHash(scope)`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/workspace_deletion_internal_event.go#L60)：`ev == nil || ev.Source() != workspaceDeletionCommandSource || m == nil || m.workspaceDeletions == nil || m.service == nil`
  - 当前来源常量：`workspaceDeletionCommandSource = "blocks/workspace"`
- [executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/lifecycle_handler.go#L84)：`!ok || ev.Source() != "kernel/orchestrator"`