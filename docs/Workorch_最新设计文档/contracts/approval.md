# approval：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页11个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Approval只管理风险许可和resolution，不管理业务目标变更的最终候选批准。当前runtime ingress要求真实RunID；pre-Run不能用dummy Run凑合同。scope/grant绑定exact运行、operation和有效期；resolve不授予目标验收credit。

OpenRuntimeApprovalRequest是风险申请事实入口；resolved通知仅供read projection/reconciliation。重复resolve必须匹配原request/outcome，冲突不覆盖历史。Workspace删除使用独立deletion scope/tombstone，与普通grant撤销不同。

## 逐topic合同

<a id="ct-approval-acquireruntimecapabilities"></a>

### CT-approval-AcquireRuntimeCapabilities

- 边界：`TopicAcquireRuntimeCapabilities` = `/approval/runtime-prerequisite/acquire-capabilities`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L17)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L191)
- 接收provider：approval；Command [AcquireRuntimeCapabilitiesCommand](#schema-acquireruntimecapabilitiescommand) → Result [AcquireRuntimeCapabilitiesResult](#schema-acquireruntimecapabilitiesresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleRuntimeCapabilities`（approval）
- 接收入口：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L150)
- 已观察外部引用：orchestrator [sendRuntimeCapabilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L323)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L150)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimecontract.RuntimeModuleOrchestrator` 分支谓词（[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L150)）
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`；`_, err := common.BindRuntimeWorkspaceContext(event.Context(), request.WorkspaceID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-approval-approvalcommandensureapproved"></a>

### CT-approval-ApprovalCommandEnsureApproved

- 边界：`TopicApprovalCommandEnsureApproved` = `/approval/command/ensure-approved`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L198)
- 接收provider：approval；Command [EnsureApprovedCommand](#schema-ensureapprovedcommand) → Result [EnsureApprovedResult](#schema-ensureapprovedresult)；target合法caller：application/workorchd/appservice（HumanOperator且SubjectID一致）、kernel/governance（AutoGovernance且SubjectID一致）
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleCommand`（approval）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L49)
- 已观察外部引用：workorchd [ApproveApprovalItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/approval_commands.go#L15)；workorchd [ResolveApprovalItemWithContext](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/approval_commands.go#L23)；workorchd [resolveApprovalItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/approval_commands.go#L30)；workorchd [resolveApproved](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/approval_commands.go#L48)；governance [eventResolveApproval](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_commands.go#L58)
- 前置来源helper：[validateApprovalResolutionSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L161)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-approval-approvalcommandensurerejected"></a>

### CT-approval-ApprovalCommandEnsureRejected

- 边界：`TopicApprovalCommandEnsureRejected` = `/approval/command/ensure-rejected`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L15)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L199)
- 接收provider：approval；Command [EnsureRejectedCommand](#schema-ensurerejectedcommand) → Result [EnsureRejectedResult](#schema-ensurerejectedresult)；target合法caller：application/workorchd/appservice（HumanOperator且SubjectID一致）、kernel/governance（AutoGovernance且SubjectID一致）
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleCommand`（approval）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L49)
- 已观察外部引用：workorchd [RejectApprovalItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/approval_commands.go#L19)；workorchd [ResolveApprovalItemWithContext](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/approval_commands.go#L23)；workorchd [resolveRejected](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/approval_commands.go#L54)
- 前置来源helper：[validateApprovalResolutionSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L161)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-approval-approvalcommandget"></a>

### CT-approval-ApprovalCommandGet

- 边界：`TopicApprovalCommandGet` = `/approval/command/get`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L192)
- 接收provider：approval；Command [GetApprovalCommand](#schema-getapprovalcommand) → Result [GetApprovalResult](#schema-getapprovalresult)；target合法caller：workorchd、orchestrator、governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleCommand`（approval）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L49)
- 已观察外部引用：governance [GetScoped](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_readers.go#L51)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.ApprovalID == ""`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-approval-approvalcommandlist"></a>

### CT-approval-ApprovalCommandList

- 边界：`TopicApprovalCommandList` = `/approval/command/list`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L193)
- 接收provider：approval；Command [ListAllApprovalsCommand](#schema-listallapprovalscommand) → Result [ListApprovalsResult](#schema-listapprovalsresult)；target合法caller：workorchd、orchestrator、governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleCommand`（approval）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L49)
- 已观察外部引用：workorchd [ListApprovalsByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/approval_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.Scope != approvalevents.ApprovalListScopeAll`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-approval-approvalcommandlistbyrun"></a>

### CT-approval-ApprovalCommandListByRun

- 边界：`TopicApprovalCommandListByRun` = `/approval/command/list-by-run`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L194)
- 接收provider：approval；Command [ListApprovalsByRunCommand](#schema-listapprovalsbyruncommand) → Result [ListApprovalsByRunResult](#schema-listapprovalsbyrunresult)；target合法caller：workorchd、orchestrator、governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleCommand`（approval）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L49)
- 已观察外部引用：workorchd [ListApprovalsByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/approval_reads.go#L26)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.RunID == ""`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-approval-approvalcommandlistbyworkspace"></a>

### CT-approval-ApprovalCommandListByWorkspace

- 边界：`TopicApprovalCommandListByWorkspace` = `/approval/command/list-by-workspace`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L195)
- 接收provider：approval；Command [ListApprovalsByWorkspaceCommand](#schema-listapprovalsbyworkspacecommand) → Result [ListApprovalsByWorkspaceResult](#schema-listapprovalsbyworkspaceresult)；target合法caller：workorchd、orchestrator、governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleCommand`（approval）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L49)
- 已观察外部引用：workorchd [ListApprovalsByWorkspaceOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/approval_reads.go#L42)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == ""`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-approval-approvalcommandpurgeworkspace"></a>

### CT-approval-ApprovalCommandPurgeWorkspace

- 边界：`TopicApprovalCommandPurgeWorkspace` = `/approval/command/purge-workspace`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L196)
- 接收provider：approval；Command [PurgeWorkspaceApprovalsCommand](#schema-purgeworkspaceapprovalscommand) → Result [PurgeWorkspaceApprovalsResult](#schema-purgeworkspaceapprovalsresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleCommand`（approval）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L49)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextPurge](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L304)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executePurgeWorkspaceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/workspace_deletion_internal_event.go#L97)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDeletionCommandSource` 分支谓词（[executePurgeWorkspaceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/workspace_deletion_internal_event.go#L97)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-approval-approvalcommandsealworkspacedeletion"></a>

### CT-approval-ApprovalCommandSealWorkspaceDeletion

- 边界：`TopicApprovalCommandSealWorkspaceDeletion` = `/approval/command/seal-workspace-deletion`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L197)
- 接收provider：approval；Command [SealWorkspaceApprovalDeletionCommand](#schema-sealworkspaceapprovaldeletioncommand) → Result [SealWorkspaceApprovalDeletionResult](#schema-sealworkspaceapprovaldeletionresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleCommand`（approval）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/command_router.go#L49)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextSeal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L221)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeSealWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/workspace_deletion_internal_event.go#L76)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDeletionCommandSource` 分支谓词（[executeSealWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/workspace_deletion_internal_event.go#L76)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-approval-approvalresolved"></a>

### CT-approval-ApprovalResolved

- 边界：`TopicApprovalResolved` = `/approval/resolved`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L205)
- 唯一事实owner/发布者：approval；Data [ApprovalChangedData](#schema-approvalchangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、governance、orchestrator、process/workorch、workorchd
- 实际注册：[subscribeApprovalNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L507) → `subscribeApprovalNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：workorchd [ValidWorkorchEventRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/observed/event_record.go#L15)；eventstream [executeRecordApprovalResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L200)；eventstream [subscribeApprovalNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L507)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || m.EventHub() == nil || data.Validate() != nil`；`result == nil`；`!ok || response.NotificationID != data.NotificationID || response.ResolutionID != data.ResolutionID || response.ApprovalID != data.ApprovalID || response.WorkspaceID != data.WorkspaceID || response.RunID != data.RunID`
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-approval-openruntimeapprovalrequest"></a>

### CT-approval-OpenRuntimeApprovalRequest

- 边界：`TopicOpenRuntimeApprovalRequest` = `/approval/runtime-governance/open-request`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/topics.go#L16)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L200)
- 接收provider：approval；Command [OpenRuntimeApprovalRequestCommand](#schema-openruntimeapprovalrequestcommand) → Result [OpenRuntimeApprovalRequestResult](#schema-openruntimeapprovalrequestresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L58) → `handleRuntimeApprovalRequest`（approval）
- 接收入口：[handleRuntimeApprovalRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/runtime_governance_adapter.go#L16)
- 已观察外部引用：orchestrator [OpenRuntimeApprovalRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_request_adapter.go#L20)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeApprovalRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/runtime_governance_adapter.go#L16)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimecontract.RuntimeModuleOrchestrator` 分支谓词（[handleRuntimeApprovalRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/runtime_governance_adapter.go#L16)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator || m == nil || m.service == nil`；`!ok || command.ApprovalID == "" || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.DispatchID) == "" || strings.TrimSpace(command.AuthoritySnapshotID) == "" || strings.TrimSpace(command.DependencyKind) != string(runtimecontract.GovernanceApproval) || strings.TrimSpace(command.SubjectRef) == "" || strings.TrimSpace(command.Reason) == "" || strings.TrimSpace(command.RequestedAction) == "" || len(command.AuthorizationContext) == 0`；`err := m.rejectFencedWorkspace(ctx, command.WorkspaceID); err != nil`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-acquireruntimecapabilitiescommand"></a>

### AcquireRuntimeCapabilitiesCommand

- 源码：[internal/modules/blocks/approval/pkg/events/runtime_capability.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/runtime_capability.go#L5)
- 声明：`type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest`
- 字段引用类型：[RuntimeCapabilityAcquisitionRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L15)

<a id="schema-acquireruntimecapabilitiesresult"></a>

### AcquireRuntimeCapabilitiesResult

- 源码：[internal/modules/blocks/approval/pkg/events/runtime_capability.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/runtime_capability.go#L7)
- 字段：`Acquisition runtimecontract.RuntimeCapabilityAcquisition`
- 字段引用类型：[RuntimeCapabilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L27)

<a id="schema-approval"></a>

### Approval

- 源码：[internal/modules/blocks/approval/pkg/models/approval.go:10](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/approval.go#L10)
- 字段：`ID common.ApprovalID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`Reason string`；`Action string`；`Context workorcheventpayload.Payload`；`ResolutionContext workorcheventpayload.Payload`；`State common.ApprovalState`；`AutoGovernance workorcheventpayload.Payload`；`ResolutionID common.ResolutionID`；`ResolutionPrincipal ApprovalResolutionPrincipal`；`RuntimeGovernance *RuntimeGovernanceBinding`；`ResolutionProposal *ResolutionProposal`；`EvidenceRecordedAt string`；`ResolutionNotificationPublishedAt string`；`ResolvedAt string`；`ResolvedBy string`；`CreatedAt string`；`UpdatedAt string`
- 字段引用类型：[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[ApprovalState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L13)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[ApprovalResolutionPrincipal](#schema-approvalresolutionprincipal)；[RuntimeGovernanceBinding](#schema-runtimegovernancebinding)；[ResolutionProposal](#schema-resolutionproposal)
- 实际方法：[CloneApproval](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/approval.go#L36)；[CloneApprovals](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/approval.go#L55)

<a id="schema-approvalchangeddata"></a>

### ApprovalChangedData

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:114](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L114)
- 字段：`NotificationID string`；`ResolutionID common.ResolutionID`；`ApprovalID common.ApprovalID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`Reason string`；`Action string`；`Context []workorcheventpayload.Field`；`ResolutionContext []workorcheventpayload.Field`；`Principal approvalmodel.ApprovalResolutionPrincipal`；`State common.ApprovalState`
- 字段引用类型：[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[Field（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L27)；[ApprovalResolutionPrincipal](#schema-approvalresolutionprincipal)；[ApprovalState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L13)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L129)
- 空值/identity/版本/范围原式：`strings.TrimSpace(d.NotificationID) == "" || d.ResolutionID == "" || d.ApprovalID == "" || d.WorkspaceID == "" || d.RunID == "" || d.Principal.Validate() != nil`；`d.State != common.ApprovalStateApproved && d.State != common.ApprovalStateRejected`

<a id="schema-approvallistscope"></a>

### ApprovalListScope

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:23](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L23)
- 声明：`type ApprovalListScope string`
- 源码枚举值：`ApprovalListScopeAll = "all"`

<a id="schema-approvalresolutionprincipal"></a>

### ApprovalResolutionPrincipal

- 源码：[internal/modules/blocks/approval/pkg/models/runtime_governance.go:18](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/runtime_governance.go#L18)
- 字段：`Kind ApprovalResolutionPrincipalKind`；`SubjectID string`
- 字段引用类型：[ApprovalResolutionPrincipalKind](#schema-approvalresolutionprincipalkind)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/runtime_governance.go#L23)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/runtime_governance.go#L29)
- 空值/identity/版本/范围原式：`p.SubjectID == ""`
- 显式分支集合（只表列源码case，default语义见validator）：`ApprovalResolutionPrincipalHumanOperator, ApprovalResolutionPrincipalAutoGovernance`

<a id="schema-approvalresolutionprincipalkind"></a>

### ApprovalResolutionPrincipalKind

- 源码：[internal/modules/blocks/approval/pkg/models/runtime_governance.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/runtime_governance.go#L11)
- 声明：`type ApprovalResolutionPrincipalKind string`
- 源码枚举值：`ApprovalResolutionPrincipalHumanOperator = "human_operator"`；`ApprovalResolutionPrincipalAutoGovernance = "auto_governance"`

<a id="schema-ensureapprovedcommand"></a>

### EnsureApprovedCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:37](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L37)
- 实际alias/同形类型：[ResolveApprovalCommand](#schema-resolveapprovalcommand)
- 字段引用类型：[ResolveApprovalCommand](#schema-resolveapprovalcommand)

<a id="schema-ensureapprovedresult"></a>

### EnsureApprovedResult

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:152](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L152)
- 字段：`Approval *approvalmodel.Approval`
- 字段引用类型：[Approval](#schema-approval)

<a id="schema-ensurerejectedcommand"></a>

### EnsureRejectedCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:38](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L38)
- 实际alias/同形类型：[ResolveApprovalCommand](#schema-resolveapprovalcommand)
- 字段引用类型：[ResolveApprovalCommand](#schema-resolveapprovalcommand)

<a id="schema-ensurerejectedresult"></a>

### EnsureRejectedResult

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:153](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L153)
- 字段：`Approval *approvalmodel.Approval`
- 字段引用类型：[Approval](#schema-approval)

<a id="schema-getapprovalcommand"></a>

### GetApprovalCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:40](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L40)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`ApprovalID common.ApprovalID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)

<a id="schema-getapprovalresult"></a>

### GetApprovalResult

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:139](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L139)
- 字段：`Approval *approvalmodel.Approval`
- 字段引用类型：[Approval](#schema-approval)

<a id="schema-listallapprovalscommand"></a>

### ListAllApprovalsCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:46](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L46)
- 字段：`Scope ApprovalListScope`；`Cursor string`；`Limit int`
- 字段引用类型：[ApprovalListScope](#schema-approvallistscope)

<a id="schema-listapprovalsbyruncommand"></a>

### ListApprovalsByRunCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:52](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L52)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Cursor string`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-listapprovalsbyrunresult"></a>

### ListApprovalsByRunResult

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:144](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L144)
- 字段：`Approvals []*approvalmodel.Approval`；`NextCursor string`
- 字段引用类型：[Approval](#schema-approval)

<a id="schema-listapprovalsbyworkspacecommand"></a>

### ListApprovalsByWorkspaceCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:59](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L59)
- 字段：`WorkspaceID common.WorkspaceID`；`Cursor string`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-listapprovalsbyworkspaceresult"></a>

### ListApprovalsByWorkspaceResult

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:148](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L148)
- 字段：`Approvals []*approvalmodel.Approval`；`NextCursor string`
- 字段引用类型：[Approval](#schema-approval)

<a id="schema-listapprovalsresult"></a>

### ListApprovalsResult

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:140](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L140)
- 字段：`Approvals []*approvalmodel.Approval`；`NextCursor string`
- 字段引用类型：[Approval](#schema-approval)

<a id="schema-openruntimeapprovalrequestcommand"></a>

### OpenRuntimeApprovalRequestCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:92](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L92)
- 字段：`ApprovalID common.ApprovalID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DispatchID string`；`AuthoritySnapshotID string`；`DependencyKind string`；`SubjectRef string`；`Reason string`；`RequestedAction string`；`RiskLevel string`；`AuthorizationContext []workorcheventpayload.Field`
- 字段引用类型：[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[Field（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L27)

<a id="schema-openruntimeapprovalrequestresult"></a>

### OpenRuntimeApprovalRequestResult

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:106](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L106)
- 字段：`ApprovalID common.ApprovalID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DispatchID string`；`AuthoritySnapshotID string`
- 字段引用类型：[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-purgeworkspaceapprovalscommand"></a>

### PurgeWorkspaceApprovalsCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:78](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L78)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L85)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L88)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/workspace_deletion.go#L41)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableApprovalDeletionFenceID(s) || r.PurgeID != StableApprovalDeletionPurgeID(s) || r.Revision != 2 || r.AckHash != StableApprovalDeletionPurgeAckHash(s) || r.PurgeProofHash != StableApprovalDeletionPurgeProofHash(s)`

<a id="schema-purgeworkspaceapprovalsresult"></a>

### PurgeWorkspaceApprovalsResult

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:162](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L162)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`PurgeID string`；`Revision uint64`；`AckHash string`；`PurgeProofHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/workspace_deletion.go#L41)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableApprovalDeletionFenceID(s) || r.PurgeID != StableApprovalDeletionPurgeID(s) || r.Revision != 2 || r.AckHash != StableApprovalDeletionPurgeAckHash(s) || r.PurgeProofHash != StableApprovalDeletionPurgeProofHash(s)`

<a id="schema-resolutionproposal"></a>

### ResolutionProposal

- 源码：[internal/modules/blocks/approval/pkg/models/runtime_governance.go:71](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/runtime_governance.go#L71)
- 字段：`ResolutionID common.ResolutionID`；`TargetState common.ApprovalState`；`Principal ApprovalResolutionPrincipal`；`Context workorcheventpayload.Payload`；`State ResolutionProposalState`；`FailureClass string`；`FailureReason string`；`ProposedAt string`；`CommittedAt string`；`EvidenceRecordedAt string`；`ResolutionNotificationPublishedAt string`
- 字段引用类型：[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[ApprovalState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L13)；[ApprovalResolutionPrincipal](#schema-approvalresolutionprincipal)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[ResolutionProposalState](#schema-resolutionproposalstate)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/runtime_governance.go#L85)

<a id="schema-resolutionproposalstate"></a>

### ResolutionProposalState

- 源码：[internal/modules/blocks/approval/pkg/models/runtime_governance.go:63](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/runtime_governance.go#L63)
- 声明：`type ResolutionProposalState string`
- 源码枚举值：`ResolutionProposalPending = "pending"`；`ResolutionProposalCommitted = "committed"`；`ResolutionProposalRejected = "rejected"`

<a id="schema-resolveapprovalcommand"></a>

### ResolveApprovalCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:27](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L27)
- 字段：`ApprovalID common.ApprovalID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Principal approvalmodel.ApprovalResolutionPrincipal`；`ResolutionContext []workorcheventpayload.Field`
- 字段引用类型：[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ApprovalResolutionPrincipal](#schema-approvalresolutionprincipal)；[Field（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L27)

<a id="schema-runtimegovernancebinding"></a>

### RuntimeGovernanceBinding

- 源码：[internal/modules/blocks/approval/pkg/models/runtime_governance.go:42](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/runtime_governance.go#L42)
- 字段：`AuthoritySnapshotID string`；`DispatchID string`；`DependencyKind string`；`SubjectRef string`；`RequestedAction string`；`RiskLevel string`；`AuthorizationHash string`
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/models/runtime_governance.go#L52)

<a id="schema-sealworkspaceapprovaldeletioncommand"></a>

### SealWorkspaceApprovalDeletionCommand

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:65](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L65)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`DataDispositionIntent deletioncontract.Mode`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L73)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L76)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/workspace_deletion.go#L31)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableApprovalDeletionFenceID(s) || r.Revision != 1 || r.AckHash != StableApprovalDeletionSealAckHash(s)`

<a id="schema-sealworkspaceapprovaldeletionresult"></a>

### SealWorkspaceApprovalDeletionResult

- 源码：[internal/modules/blocks/approval/pkg/events/messages.go:154](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/messages.go#L154)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`Revision uint64`；`AckHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/events/workspace_deletion.go#L31)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableApprovalDeletionFenceID(s) || r.Revision != 1 || r.AckHash != StableApprovalDeletionSealAckHash(s)`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L150)：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`；`ModuleID = "blocks/approval"`
- [executePurgeWorkspaceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/workspace_deletion_internal_event.go#L97)：`!ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil`
  - 当前来源常量：`workspaceDeletionCommandSource = "blocks/workspace"`
- [executeSealWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/workspace_deletion_internal_event.go#L76)：`!ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil`
  - 当前来源常量：`workspaceDeletionCommandSource = "blocks/workspace"`
- [handleRuntimeApprovalRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/runtime_governance_adapter.go#L16)：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator || m == nil || m.service == nil`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`