# decision：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页19个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Decision管理ticket、原因/选项/用户选择、resolution及其提交历史；不拥有Workspace Definition adoption。runtime/pre-Run clarification有不同concrete合同，WorkspaceID与最终candidate/input/lineage分别匹配，不把空Run误作运行期批准。

目标NC04严格为D2A exact选择→D2B Workspace definition-scope preflight→D2R仅runtime-bound时Orchestrator matching→D2C Decision formal commit/ACK→D3 Workspace adoption→D4 journal→D5 materialized seal。无Run/合法terminal prospective successor采用definition-scope receipt。新RegisterContractChangeProposal只完成有版本登记和最终候选反查；Prepare/QueryDefinitionChange只完成独立候选准备/查询，尚无完整批准/应用链。ResolveExisting接口不自动证明post-seal目标完整实现。

## 逐topic合同

<a id="ct-decision-acquireruntimecapabilities"></a>

### CT-decision-AcquireRuntimeCapabilities

- 边界：`TopicAcquireRuntimeCapabilities` = `/decision/runtime-prerequisite/acquire-capabilities`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L19)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L197)
- 接收provider：decision；Command [AcquireRuntimeCapabilitiesCommand](#schema-acquireruntimecapabilitiescommand) → Result [AcquireRuntimeCapabilitiesResult](#schema-acquireruntimecapabilitiesresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleRuntimeCapabilities`（decision）
- 接收入口：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L183)
- 已观察外部引用：orchestrator [sendRuntimeCapabilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L323)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L183)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimecontract.RuntimeModuleOrchestrator` 分支谓词（[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L183)）
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`；`_, err := common.BindRuntimeWorkspaceContext(event.Context(), request.WorkspaceID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-decision-decisioncommandensureresolved"></a>

### CT-decision-DecisionCommandEnsureResolved

- 边界：`TopicDecisionCommandEnsureResolved` = `/decision/command/ensure-resolved`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L15)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L204)
- 接收provider：decision；Command [EnsureResolvedCommand](#schema-ensureresolvedcommand) → Result [EnsureResolvedResult](#schema-ensureresolvedresult)；target合法caller：application/workorchd/appservice、kernel/governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleCommand`（decision）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)
- 已观察外部引用：workorchd [ResolveDecision](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/decision_resolution_commands.go#L15)；governance [eventResolveDecision](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_commands.go#L23)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)；[decisionResolutionIngressSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/resolution_commands.go#L18)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[decisionResolutionIngressSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/resolution_commands.go#L18)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == ""`；`cmd.DecisionKind == "runtime_governance"`；`currentTicket.DecisionKind != cmd.DecisionKind`；`currentTicket.FailureClass == runtimecontract.RuntimeValidationInputReviewFailureClass && cmd.Type == modulecommon.ResolutionTypeProvideMissingInfo`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-decision-decisioncommandgetresolution"></a>

### CT-decision-DecisionCommandGetResolution

- 边界：`TopicDecisionCommandGetResolution` = `/decision/command/get-resolution`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L202)
- 接收provider：decision；Command [GetResolutionCommand](#schema-getresolutioncommand) → Result [GetResolutionResult](#schema-getresolutionresult)；target合法caller：workorchd、orchestrator、workspace、governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleCommand`（decision）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)
- 已观察外部引用：workorchd [ReadDecisionResolutionByTicketOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/decision_resolution_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.TicketID == ""`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-decision-decisioncommandgetticket"></a>

### CT-decision-DecisionCommandGetTicket

- 边界：`TopicDecisionCommandGetTicket` = `/decision/command/get-ticket`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L198)
- 接收provider：decision；Command [GetTicketCommand](#schema-getticketcommand) → Result [GetTicketResult](#schema-getticketresult)；target合法caller：workorchd、orchestrator、workspace、governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleCommand`（decision）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)
- 已观察外部引用：workorchd [RegisterReviewedDefinitionChange](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_change.go#L63)；workorchd [ReadDecisionTicketByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/decision_ticket_read.go#L14)；governance [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_readers.go#L27)；workspace [observeDefinitionChangeProposal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_change_provenance.go#L18)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.TicketID == ""`；`!ticketMatchesDecisionScope(item, decisionCommandScope{WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: cmd.StepID})`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-decision-decisioncommandlisttickets"></a>

### CT-decision-DecisionCommandListTickets

- 边界：`TopicDecisionCommandListTickets` = `/decision/command/list-tickets`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L199)
- 接收provider：decision；Command [ListTicketsCommand](#schema-listticketscommand) → Result [ListTicketsResult](#schema-listticketsresult)；target合法caller：workorchd、orchestrator、workspace、governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleCommand`（decision）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)
- 已观察外部引用：workorchd [ListDecisionTicketsOwnerPage](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/decision_reads.go#L33)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.Scope != decisionevents.TicketListScopeAll`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-decision-decisioncommandlistticketsbyrun"></a>

### CT-decision-DecisionCommandListTicketsByRun

- 边界：`TopicDecisionCommandListTicketsByRun` = `/decision/command/list-tickets-by-run`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L200)
- 接收provider：decision；Command [ListTicketsByRunCommand](#schema-listticketsbyruncommand) → Result [ListTicketsByRunResult](#schema-listticketsbyrunresult)；target合法caller：workorchd、orchestrator、workspace、governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleCommand`（decision）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)
- 已观察外部引用：workorchd [ListDecisionTicketsByRunOwnerPage](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/decision_reads.go#L47)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.RunID == ""`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-decision-decisioncommandlistticketsbyws"></a>

### CT-decision-DecisionCommandListTicketsByWS

- 边界：`TopicDecisionCommandListTicketsByWS` = `/decision/command/list-tickets-by-workspace`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L201)
- 接收provider：decision；Command [ListTicketsByWorkspaceCommand](#schema-listticketsbyworkspacecommand) → Result [ListTicketsByWorkspaceResult](#schema-listticketsbyworkspaceresult)；target合法caller：workorchd、orchestrator、workspace、governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleCommand`（decision）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)
- 已观察外部引用：workorchd [ListDecisionTicketsByWorkspaceOwnerPage](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/decision_reads.go#L19)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == ""`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-decision-decisioncommandpurgeworkspace"></a>

### CT-decision-DecisionCommandPurgeWorkspace

- 边界：`TopicDecisionCommandPurgeWorkspace` = `/decision/command/purge-workspace`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L16)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L206)
- 接收provider：decision；Command [PurgeWorkspaceDecisionsCommand](#schema-purgeworkspacedecisionscommand) → Result [PurgeWorkspaceDecisionsResult](#schema-purgeworkspacedecisionsresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleCommand`（decision）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextPurge](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L304)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)；[executePurgeWorkspaceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/workspace_deletion_internal_event.go#L87)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDeletionCommandSource` 分支谓词（[executePurgeWorkspaceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/workspace_deletion_internal_event.go#L87)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-decision-decisioncommandsealworkspacedeletion"></a>

### CT-decision-DecisionCommandSealWorkspaceDeletion

- 边界：`TopicDecisionCommandSealWorkspaceDeletion` = `/decision/command/seal-workspace-deletion`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L17)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L205)
- 接收provider：decision；Command [SealWorkspaceDecisionDeletionCommand](#schema-sealworkspacedecisiondeletioncommand) → Result [SealWorkspaceDecisionDeletionResult](#schema-sealworkspacedecisiondeletionresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleCommand`（decision）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextSeal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L221)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)；[executeSealWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/workspace_deletion_internal_event.go#L66)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDeletionCommandSource` 分支谓词（[executeSealWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/workspace_deletion_internal_event.go#L66)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-decision-openruntimedefinitionclarification"></a>

### CT-decision-OpenRuntimeDefinitionClarification

- 边界：`TopicOpenRuntimeDefinitionClarification` = `/decision/definition-clarification/runtime/open`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L24)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L211)
- 接收provider：decision；Command [OpenRuntimeDefinitionClarificationRequestCommand](#schema-openruntimedefinitionclarificationrequestcommand) → Result [OpenRuntimeDefinitionClarificationRequestResult](#schema-openruntimedefinitionclarificationrequestresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleOpenRuntimeDefinitionClarification`（decision）
- 接收入口：[handleOpenRuntimeDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L106)
- 已观察外部引用：orchestrator [OpenRuntimeDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_definition_clarification_event_adapter.go#L19)
- 本topic入口/具体类型分支的Source审查索引：[handleOpenRuntimeDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L106)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimeDefinitionClarificationSource` 分支谓词（[handleOpenRuntimeDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L106)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != runtimeDefinitionClarificationSource || m == nil || m.tickets == nil`；`err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-decision-openruntimegovernancerequest"></a>

### CT-decision-OpenRuntimeGovernanceRequest

- 边界：`TopicOpenRuntimeGovernanceRequest` = `/decision/runtime-governance/open-request`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L18)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L207)
- 接收provider：decision；Command [OpenRuntimeGovernanceRequestCommand](#schema-openruntimegovernancerequestcommand) → Result [OpenRuntimeGovernanceRequestResult](#schema-openruntimegovernancerequestresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleRuntimeGovernanceRequest`（decision）
- 接收入口：[handleRuntimeGovernanceRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/runtime_governance_adapter.go#L19)
- 已观察外部引用：orchestrator [OpenRuntimeGovernanceRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_request_adapter.go#L51)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeGovernanceRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/runtime_governance_adapter.go#L19)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimecontract.RuntimeModuleOrchestrator` 分支谓词（[handleRuntimeGovernanceRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/runtime_governance_adapter.go#L19)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`；`!ok || m == nil || m.service == nil || command.TicketID == "" || command.WorkspaceID == "" || command.RunID == "" || command.DispatchID == "" || command.Transaction.AuthoritySnapshotID == "" || command.Dependency.Kind == "" || command.Dependency.Reason == "" || !modulecommon.IsSupportedResolutionType(command.ContinueResolutionType) || !modulecommon.IsSupportedResolutionType(command.FailResolutionType)`；`err := m.rejectFencedWorkspace(ctx, command.WorkspaceID); err != nil`；`command.Dependency.RequestedAction == runtimecontract.RuntimeValidationInputReviewInformationAction`；`command.Dependency.Kind != string(runtimecontract.GovernanceDecision) || command.Dependency.SubjectRef == ""`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-decision-openworkspacedefinitionclarification"></a>

### CT-decision-OpenWorkspaceDefinitionClarification

- 边界：`TopicOpenWorkspaceDefinitionClarification` = `/decision/definition-clarification/workspace/open`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L20)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L208)
- 接收provider：decision；Command [OpenWorkspaceDefinitionClarificationRequestCommand](#schema-openworkspacedefinitionclarificationrequestcommand) → Result [OpenWorkspaceDefinitionClarificationRequestResult](#schema-openworkspacedefinitionclarificationrequestresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleOpenWorkspaceDefinitionClarification`（decision）
- 接收入口：[handleOpenWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L74)
- 已观察外部引用：workorchd [OpenWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/definition_clarification.go#L17)
- 本topic入口/具体类型分支的Source审查索引：[handleOpenWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L74)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDefinitionClarificationSource` 分支谓词（[handleOpenWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L74)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil`；`err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil`；`err := cmd.Validate(); err != nil`；`err := ticketRecord.Validate(); err != nil || !workspaceDefinitionClarificationTicketSemanticsMatch(ticket, requestCommand) || !workspaceDefinitionClarificationBindingMatchesCommand(binding, requestCommand)`；`err := response.ValidateFor(cmd); err != nil`；`err := record.Validate(); err != nil`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-decision-queryworkspacedefinitionclarificationfact"></a>

### CT-decision-QueryWorkspaceDefinitionClarificationFact

- 边界：`TopicQueryWorkspaceDefinitionClarificationFact` = `/decision/definition-clarification/workspace/query-persisted-fact`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L23)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L203)
- 接收provider：decision；Command [QueryWorkspaceDefinitionClarificationFactCommand](#schema-queryworkspacedefinitionclarificationfactcommand) → Result [QueryWorkspaceDefinitionClarificationFactResult](#schema-queryworkspacedefinitionclarificationfactresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleQueryWorkspaceDefinitionClarificationFact`（decision）
- 接收入口：[handleQueryWorkspaceDefinitionClarificationFact](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L37)
- 已观察外部引用：workspace [verifyDefinitionClarificationProvenance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_admission_provenance.go#L20)
- 本topic入口/具体类型分支的Source审查索引：[handleQueryWorkspaceDefinitionClarificationFact](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L37)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDefinitionFactQuerySource` 分支谓词（[handleQueryWorkspaceDefinitionClarificationFact](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L37)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceDefinitionFactQuerySource || m == nil || m.tickets == nil || m.resolutions == nil`；`err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil`；`err := cmd.Validate(); err != nil`；`err := ticketRecord.Validate(); err != nil || !workspaceDefinitionClarificationTicketSemanticsMatch(ticket, requestCommand) || !workspaceDefinitionClarificationBindingMatchesCommand(binding, requestCommand)`；`err := response.ValidateFor(cmd); err != nil`；`err := record.Validate(); err != nil`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-decision-reconcileworkspacedefinitionclarificationcandidate"></a>

### CT-decision-ReconcileWorkspaceDefinitionClarificationCandidate

- 边界：`TopicReconcileWorkspaceDefinitionClarificationCandidate` = `/decision/definition-clarification/workspace/reconcile-candidate`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L21)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L209)
- 接收provider：decision；Command [ReconcileWorkspaceDefinitionClarificationCandidateCommand](#schema-reconcileworkspacedefinitionclarificationcandidatecommand) → Result [ReconcileWorkspaceDefinitionClarificationCandidateResult](#schema-reconcileworkspacedefinitionclarificationcandidateresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleReconcileWorkspaceDefinitionClarification`（decision）
- 接收入口：[handleReconcileWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L53)
- 已观察外部引用：workorchd [ReconcileWorkspaceDefinitionClarificationCandidate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/definition_clarification.go#L59)
- 本topic入口/具体类型分支的Source审查索引：[handleReconcileWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L53)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDefinitionClarificationSource` 分支谓词（[handleReconcileWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L53)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil || m.resolutions == nil`；`err := cmd.Validate(); err != nil`；`err := m.rejectFencedWorkspace(ev.Context(), cmd.NewRequest.WorkspaceID); err != nil`；`identity.Validate() != nil || binding.ResolutionBindingHash != identity.BindingHash(expectedID) || !definitionClarificationResolutionMatches(resolution, expected)`；`reconcile.Validate() != nil || binding.ResolutionBindingHash != reconcile.BindingHash(expectedID) || !definitionClarificationResolutionMatches(resolution, expected)`；`command.Validate() != nil || binding.ResolutionBindingHash != command.BindingHash(expectedID, decisionevents.DefinitionClarificationResolved, decisionevents.RuntimeDefinitionClarificationNoConflict, "", "") || !definitionClarificationResolutionMatches(resolution, expected)`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-decision-registercontractchangeproposal"></a>

### CT-decision-RegisterContractChangeProposal

- 边界：`TopicRegisterContractChangeProposal` = `/decision/contract-change/register-proposal`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L196)
- 接收provider：decision；Command [RegisterContractChangeProposalCommand](#schema-registercontractchangeproposalcommand) → Result [RegisterContractChangeProposalResult](#schema-registercontractchangeproposalresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleRegisterContractChangeProposal`（decision）
- 接收入口：[handleRegisterContractChangeProposal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/contract_change.go#L14)
- 已观察外部引用：workorchd [RegisterContractChangeProposal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/contract_change.go#L19)
- 本topic入口/具体类型分支的Source审查索引：[handleRegisterContractChangeProposal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/contract_change.go#L14)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDefinitionClarificationSource` 分支谓词（[handleRegisterContractChangeProposal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/contract_change.go#L14)）
- 特化承诺：source新增：TicketID/RequestID、ExpectedRevision/ExpectedHash成对、Proposal.Validate与Hash、结果revision=expected+1及registered/duplicate互斥处置；FinalCandidate需反查Workspace持久reviewed identity。登记不是最终批准、formal ACK或adoption。target NC04完整链仍未由此完成。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`ev == nil`；`!ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.service == nil`；`err := cmd.Validate(); err != nil`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-decision-resolveruntimedefinitionclarificationbyreplacement"></a>

### CT-decision-ResolveRuntimeDefinitionClarificationByReplacement

- 边界：`TopicResolveRuntimeDefinitionClarificationByReplacement` = `/decision/definition-clarification/runtime/resolve-by-replacement`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L25)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L212)
- 接收provider：decision；Command [ResolveRuntimeDefinitionClarificationByReplacementCommand](#schema-resolveruntimedefinitionclarificationbyreplacementcommand) → Result [ResolveRuntimeDefinitionClarificationByReplacementResult](#schema-resolveruntimedefinitionclarificationbyreplacementresult)；target合法caller：blocks/run
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleResolveRuntimeDefinitionClarification`（decision）
- 接收入口：[handleResolveRuntimeDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L122)
- 已观察外部引用：run [bindRuntimeClarificationReplacement](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_execution_intent.go#L1005)
- 本topic入口/具体类型分支的Source审查索引：[handleResolveRuntimeDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L122)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimeDefinitionReplacementSource` 分支谓词（[handleResolveRuntimeDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L122)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != runtimeDefinitionReplacementSource || m == nil || m.tickets == nil || m.resolutions == nil`；`err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil`；`identity.Validate() != nil || binding.ResolutionBindingHash != identity.BindingHash(expectedID) || !definitionClarificationResolutionMatches(resolution, expected)`；`reconcile.Validate() != nil || binding.ResolutionBindingHash != reconcile.BindingHash(expectedID) || !definitionClarificationResolutionMatches(resolution, expected)`；`command.Validate() != nil || binding.ResolutionBindingHash != command.BindingHash(expectedID, decisionevents.DefinitionClarificationResolved, decisionevents.RuntimeDefinitionClarificationNoConflict, "", "") || !definitionClarificationResolutionMatches(resolution, expected)`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-decision-resolveworkspacedefinitionclarificationbyadmission"></a>

### CT-decision-ResolveWorkspaceDefinitionClarificationByAdmission

- 边界：`TopicResolveWorkspaceDefinitionClarificationByAdmission` = `/decision/definition-clarification/workspace/resolve-by-admission`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L22)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L210)
- 接收provider：decision；Command [ResolveWorkspaceDefinitionClarificationByAdmissionCommand](#schema-resolveworkspacedefinitionclarificationbyadmissioncommand) → Result [ResolveWorkspaceDefinitionClarificationByAdmissionResult](#schema-resolveworkspacedefinitionclarificationbyadmissionresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L58) → `handleResolveWorkspaceDefinitionClarification`（decision）
- 接收入口：[handleResolveWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L90)
- 已观察外部引用：workorchd [ResolveWorkspaceDefinitionClarificationByAdmission](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/governanceports/definition_clarification.go#L38)
- 本topic入口/具体类型分支的Source审查索引：[handleResolveWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L90)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDefinitionClarificationSource` 分支谓词（[handleResolveWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L90)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil || m.resolutions == nil`；`err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-decision-ticketcreated"></a>

### CT-decision-TicketCreated

- 边界：`TopicTicketCreated` = `/ticket/created`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L217)
- 唯一事实owner/发布者：decision；Data [TicketChangedData](#schema-ticketchangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、governance、orchestrator、process/workorch、workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L148) → `Start`（governance）；[subscribeDecisionNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L531) → `subscribeDecisionNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[publishTicketCreated](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/decision_notification.go#L15)
- 已观察外部引用：workorchd [enrichSSEEnvelope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/sse_enrichment.go#L15)；workorchd [ValidWorkorchEventRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/observed/event_record.go#L15)；governance [Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L148)；eventstream [subscribeDecisionNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L531)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-decision-ticketresolved"></a>

### CT-decision-TicketResolved

- 边界：`TopicTicketResolved` = `/ticket/resolved`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L220)
- 唯一事实owner/发布者：decision；Data [TicketResolvedData](#schema-ticketresolveddata)；同步Command/Result：不适用；允许订阅者：eventstream、governance、orchestrator、process/workorch、workorchd
- 实际注册：[subscribeDecisionNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L531) → `subscribeDecisionNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[recordAndPublishResolvedTicket](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/decision_notification.go#L45)
- 已观察外部引用：workorchd [ValidWorkorchEventRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/observed/event_record.go#L15)；workorchd [normalizeDecisionTicketSnapshot](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/observed/workorch_contract.go#L158)；eventstream [subscribeDecisionNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L531)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := m.withWorkspaceActiveDelivery(ctx, ticket.WorkspaceID, func(guarded context.Context) error { event := magicevent.NewEventWithContext(decisionevents.TopicTicketResolved, modulecommon.ModuleID, decisionevents.NotificationDestination, magicevent.NewHeader(), guarded, data) m.bindDecisionNotificationLane(event, ticket) m.PostEvent(event) return nil }); err != nil`
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-acquireruntimecapabilitiescommand"></a>

### AcquireRuntimeCapabilitiesCommand

- 源码：[internal/modules/blocks/decision/pkg/events/runtime_capability.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/runtime_capability.go#L5)
- 声明：`type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest`
- 字段引用类型：[RuntimeCapabilityAcquisitionRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L15)

<a id="schema-acquireruntimecapabilitiesresult"></a>

### AcquireRuntimeCapabilitiesResult

- 源码：[internal/modules/blocks/decision/pkg/events/runtime_capability.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/runtime_capability.go#L7)
- 字段：`Acquisition runtimecontract.RuntimeCapabilityAcquisition`
- 字段引用类型：[RuntimeCapabilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L27)

<a id="schema-contractchangecandidate"></a>

### ContractChangeCandidate

- 源码：[internal/modules/blocks/decision/pkg/models/contract_change.go:72](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L72)
- 字段：`ID string`；`Revision uint64`；`Hash string`

<a id="schema-contractchangedifference"></a>

### ContractChangeDifference

- 源码：[internal/modules/blocks/decision/pkg/models/contract_change.go:55](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L55)
- 字段：`ItemID string`；`Kind string`；`Action string`；`OriginalText string`；`ProposedText string`；`SemanticChange string`

<a id="schema-contractchangeimpact"></a>

### ContractChangeImpact

- 源码：[internal/modules/blocks/decision/pkg/models/contract_change.go:65](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L65)
- 字段：`SubjectID string`；`Kind string`；`ProposedHandling string`；`Reason string`

<a id="schema-contractchangeproposal"></a>

### ContractChangeProposal

- 源码：[internal/modules/blocks/decision/pkg/models/contract_change.go:24](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L24)
- 字段：`SchemaVersion int`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`BaseDefinitionRevision string`；`BaseStartupInputHash string`；`BaseInputRevision uint64`；`BaseInputHash string`；`ProposedBy string`；`TriggerFacts string`；`Discovery string`；`RootCauseOrUnknown string`；`WhyChange string`；`EvidenceRefs []string`；`EvidenceGaps string`；`Differences []ContractChangeDifference`；`Alternatives string`；`NoChangeConsequences string`；`UnaffectedContinuationConditions string`；`Impacts []ContractChangeImpact`；`EffectsAndReceipts string`；`OutstandingCosts string`；`ReservedBudget string`；`AdditionalCostDurationRisk string`；`RequiredAuthorization string`；`DecisionScope string`；`ApplicationConditions string`；`RecoveryRestrictions string`；`FinalCandidate *ContractChangeCandidate`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ContractChangeDifference](#schema-contractchangedifference)；[ContractChangeImpact](#schema-contractchangeimpact)；[ContractChangeCandidate](#schema-contractchangecandidate)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L87)；[Hash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L115)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L124)
- 空值/identity/版本/范围原式：`err != nil`；`p.SchemaVersion != ContractChangeSchemaVersion || p.WorkspaceID == "" || ((p.BaseDefinitionRevision == "") != (p.BaseStartupInputHash == "")) || ((p.BaseInputRevision == 0) != (p.BaseInputHash == "")) || (p.BaseDefinitionRevision == "" && p.BaseInputRevision == 0)`；`p.BaseDefinitionRevision != "" && (strings.TrimSpace(p.BaseDefinitionRevision) == "" || strings.TrimSpace(p.BaseStartupInputHash) == "") || p.BaseInputRevision != 0 && strings.TrimSpace(p.BaseInputHash) == "" || p.RunID != "" && p.BaseDefinitionRevision == ""`；`strings.TrimSpace(value) == "" || !utf8.ValidString(value)`；`len(p.EvidenceRefs) == 0 && strings.TrimSpace(p.EvidenceGaps) == ""`；`len(p.EvidenceRefs) > 128 || len(p.Differences) == 0 || len(p.Differences) > 128 || len(p.Impacts) == 0 || len(p.Impacts) > 256`；`strings.TrimSpace(ref) == "" || !utf8.ValidString(ref)`；`strings.TrimSpace(d.ItemID) == "" || seen[d.ItemID] || strings.TrimSpace(d.SemanticChange) == ""`；`d.OriginalText != "" || strings.TrimSpace(d.ProposedText) == ""`；`strings.TrimSpace(d.OriginalText) == "" || d.ProposedText != ""`；`strings.TrimSpace(d.OriginalText) == "" || strings.TrimSpace(d.ProposedText) == "" || d.OriginalText == d.ProposedText`；`strings.TrimSpace(impact.SubjectID) == "" || seen[key] || strings.TrimSpace(impact.Reason) == ""`；`c := p.FinalCandidate; c != nil && (strings.TrimSpace(c.ID) == "" || c.Revision == 0 || strings.TrimSpace(c.Hash) == "")`；`err != nil || len(body) > ContractChangeDocumentByteLimit`；`!utf8.ValidString(p.EvidenceGaps) || !utf8.ValidString(string(p.WorkspaceID)) || !utf8.ValidString(string(p.RunID)) || !utf8.ValidString(p.BaseDefinitionRevision) || !utf8.ValidString(p.BaseStartupInputHash) || !utf8.ValidString(p.BaseInputHash)`；`!utf8.ValidString(text)`；`!utf8.ValidString(i.SubjectID) || !utf8.ValidString(i.Reason)`；`c := p.FinalCandidate; c != nil && (!utf8.ValidString(c.ID) || !utf8.ValidString(c.Hash))`
- 显式分支集合（只表列源码case，default语义见validator）：`"goal", "criterion", "constraint"`；`"add"`；`"remove"`；`"modify"`；`"requirement", "obligation", "criterion", "verification_policy", "review_mapping", "definition_run_binding", "plan_cwi", "resource_environment", "artifact_evidence_closure", "delivery_regression"`；`"retain", "recheck", "invalidate_reopen", "unknown"`

<a id="schema-contractchangerevision"></a>

### ContractChangeRevision

- 源码：[internal/modules/blocks/decision/pkg/models/contract_change.go:79](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L79)
- 字段：`RequestID string`；`Revision uint64`；`Hash string`；`RegisteredAt string`；`Proposal ContractChangeProposal`
- 字段引用类型：[ContractChangeProposal](#schema-contractchangeproposal)
- 实际方法：[CloneContractChangeRevisions](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L104)

<a id="schema-definitionclarificationbinding"></a>

### DefinitionClarificationBinding

- 源码：[internal/modules/blocks/decision/pkg/models/definition_clarification.go:17](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/definition_clarification.go#L17)
- 字段：`Class DefinitionClarificationClass`；`SchemaVersion uint32`；`OpenBindingHash string`；`RequiredActionKind string`；`CandidateID string`；`CandidateRevision uint64`；`CandidateHash string`；`ViolationOrGapCode string`；`CanonicalSignatureVersion string`；`CanonicalSignature string`；`CanonicalSignatureHash string`；`OffendingRefs []string`；`DefinitionRevision string`；`StartupInputHash string`；`AuthoritySnapshotID string`；`ClarificationDispatchID string`；`FocusRef string`；`LogicalRequestID string`；`SemanticDocumentHash string`；`ResolutionID common.ResolutionID`；`ResolutionBindingHash string`；`ResolutionCandidateID string`；`ResolutionCandidateRevision uint64`；`ResolutionCandidateHash string`；`AdmissionResultHash string`；`ReplacementIntentID string`；`ReplacementIntentHash string`；`SuccessorDefinitionRevision string`；`SuccessorStartupInputHash string`
- 字段引用类型：[DefinitionClarificationClass](#schema-definitionclarificationclass)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/definition_clarification.go#L49)

<a id="schema-definitionclarificationclass"></a>

### DefinitionClarificationClass

- 源码：[internal/modules/blocks/decision/pkg/models/definition_clarification.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/definition_clarification.go#L7)
- 声明：`type DefinitionClarificationClass string`
- 源码枚举值：`DefinitionClarificationWorkspacePreSeal = "workspace_definition_pre_seal"`；`DefinitionClarificationRuntime = "runtime_definition"`

<a id="schema-definitionclarificationdisposition"></a>

### DefinitionClarificationDisposition

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:25](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L25)
- 声明：`type DefinitionClarificationDisposition string`
- 源码枚举值：`DefinitionClarificationCreated = DefinitionClarificationDisposition(definitioncontract.DefinitionClarificationCreated)`；`DefinitionClarificationDuplicate = DefinitionClarificationDisposition(definitioncontract.DefinitionClarificationDuplicate)`；`DefinitionClarificationResolved = DefinitionClarificationDisposition(definitioncontract.DefinitionClarificationResolved)`；`DefinitionClarificationStale = DefinitionClarificationDisposition(definitioncontract.DefinitionClarificationStale)`
- 实际方法：[BindingHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L437)

<a id="schema-ensureresolvedcommand"></a>

### EnsureResolvedCommand

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:28](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L28)
- 字段：`ResolutionID common.ResolutionID`；`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`DecisionKind string`；`Type string`；`Payload []workorcheventpayload.Field`；`ContinuationAction string`；`ResumeRunID common.RunID`；`SuccessorRunID common.RunID`
- 字段引用类型：[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[Field（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L27)

<a id="schema-ensureresolvedresult"></a>

### EnsureResolvedResult

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:155](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L155)
- 字段：`Ticket *decisionmodel.Ticket`；`Resolution *decisionmodel.Resolution`
- 字段引用类型：[Ticket](#schema-ticket)；[Resolution](#schema-resolution)

<a id="schema-getresolutioncommand"></a>

### GetResolutionCommand

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:113](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L113)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`TicketID common.TicketID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)

<a id="schema-getresolutionresult"></a>

### GetResolutionResult

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:154](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L154)
- 字段：`Resolution *decisionmodel.Resolution`
- 字段引用类型：[Resolution](#schema-resolution)

<a id="schema-getticketcommand"></a>

### GetTicketCommand

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:60](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L60)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`TicketID common.TicketID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)

<a id="schema-getticketresult"></a>

### GetTicketResult

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:141](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L141)
- 字段：`Ticket *decisionmodel.Ticket`
- 字段引用类型：[Ticket](#schema-ticket)

<a id="schema-listticketsbyruncommand"></a>

### ListTicketsByRunCommand

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:73](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L73)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Cursor string`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-listticketsbyrunresult"></a>

### ListTicketsByRunResult

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:146](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L146)
- 字段：`Tickets []*decisionmodel.Ticket`；`NextCursor string`
- 字段引用类型：[Ticket](#schema-ticket)

<a id="schema-listticketsbyworkspacecommand"></a>

### ListTicketsByWorkspaceCommand

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:80](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L80)
- 字段：`WorkspaceID common.WorkspaceID`；`Cursor string`；`Limit int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-listticketsbyworkspaceresult"></a>

### ListTicketsByWorkspaceResult

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:150](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L150)
- 字段：`Tickets []*decisionmodel.Ticket`；`NextCursor string`
- 字段引用类型：[Ticket](#schema-ticket)

<a id="schema-listticketscommand"></a>

### ListTicketsCommand

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:67](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L67)
- 字段：`Scope TicketListScope`；`Cursor string`；`Limit int`
- 字段引用类型：[TicketListScope](#schema-ticketlistscope)

<a id="schema-listticketsresult"></a>

### ListTicketsResult

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:142](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L142)
- 字段：`Tickets []*decisionmodel.Ticket`；`NextCursor string`
- 字段引用类型：[Ticket](#schema-ticket)

<a id="schema-openruntimedefinitionclarificationrequestcommand"></a>

### OpenRuntimeDefinitionClarificationRequestCommand

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:270](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L270)
- 字段：`SchemaVersion uint32`；`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DefinitionRevision string`；`StartupInputHash string`；`AuthoritySnapshotID string`；`ClarificationDispatchID string`；`FocusRef string`；`LogicalRequestID string`；`SemanticDocumentHash string`；`GapKind RuntimeDefinitionGapKind`；`OffendingItemRefs []string`；`CanonicalGapSignatureVersion string`；`CanonicalGapSignature string`；`CanonicalGapSignatureHash string`；`RequiredActionKind string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeDefinitionGapKind](#schema-runtimedefinitiongapkind)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L376)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L393)
- 空值/identity/版本/范围原式：`c.SchemaVersion != DefinitionClarificationSchemaVersionCurrent || c.TicketID == "" || c.WorkspaceID == "" || c.RunID == "" || c.DefinitionRevision == "" || c.StartupInputHash == "" || c.AuthoritySnapshotID == "" || c.ClarificationDispatchID == "" || c.FocusRef == "" || c.LogicalRequestID == "" || c.SemanticDocumentHash == "" || len(c.OffendingItemRefs) == 0 || c.CanonicalGapSignatureVersion == "" || c.CanonicalGapSignature == "" || c.CanonicalGapSignatureHash == "" || c.RequiredActionKind != DefinitionClarificationActionReviseAndRestart`；`c.CanonicalGapSignatureHash != clarificationHash("runtime-signature", c.CanonicalGapSignatureVersion, c.CanonicalGapSignature) || c.TicketID != StableRuntimeDefinitionClarificationTicketID(c)`
- 显式分支集合（只表列源码case，default语义见validator）：`RuntimeDefinitionGapImplicitOneOf, RuntimeDefinitionGapUndeclaredPrerequisite, RuntimeDefinitionGapSemanticAmbiguity`

<a id="schema-openruntimedefinitionclarificationrequestresult"></a>

### OpenRuntimeDefinitionClarificationRequestResult

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:290](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L290)
- 字段：`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`ClarificationDispatchID string`；`Disposition DefinitionClarificationDisposition`；`ClarificationRequestBindingHash string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[DefinitionClarificationDisposition](#schema-definitionclarificationdisposition)

<a id="schema-openruntimegovernancerequestcommand"></a>

### OpenRuntimeGovernanceRequestCommand

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:41](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L41)
- 字段：`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DispatchID string`；`Transaction runtimecontract.AuthorityTransaction`；`Dependency runtimecontract.ExternalDependency`；`ContinueResolutionType string`；`FailResolutionType string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[AuthorityTransaction（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1750)；[ExternalDependency（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate.go#L463)

<a id="schema-openruntimegovernancerequestresult"></a>

### OpenRuntimeGovernanceRequestResult

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:52](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L52)
- 字段：`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DispatchID string`；`AuthoritySnapshotID string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-openworkspacedefinitionclarificationrequestcommand"></a>

### OpenWorkspaceDefinitionClarificationRequestCommand

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:46](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L46)
- 声明：`type OpenWorkspaceDefinitionClarificationRequestCommand definitioncontract.WorkspaceDefinitionClarificationRequestIdentity`
- 字段引用类型：[WorkspaceDefinitionClarificationRequestIdentity（shared/workorchdefinitioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdefinitioncontract/clarification.go#L36)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L149)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L153)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L170)

<a id="schema-openworkspacedefinitionclarificationrequestresult"></a>

### OpenWorkspaceDefinitionClarificationRequestResult

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:47](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L47)
- 字段：`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`CandidateID string`；`CandidateRevision uint64`；`Disposition DefinitionClarificationDisposition`；`ClarificationRequestBindingHash string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionClarificationDisposition](#schema-definitionclarificationdisposition)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L161)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L170)

<a id="schema-purgeworkspacedecisionscommand"></a>

### PurgeWorkspaceDecisionsCommand

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:99](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L99)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L106)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L109)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/workspace_deletion.go#L41)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableDecisionDeletionFenceID(s) || r.PurgeID != StableDecisionDeletionPurgeID(s) || r.Revision != 2 || r.AckHash != StableDecisionDeletionPurgeAckHash(s) || r.PurgeProofHash != StableDecisionDeletionPurgeProofHash(s)`

<a id="schema-purgeworkspacedecisionsresult"></a>

### PurgeWorkspaceDecisionsResult

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:167](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L167)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`PurgeID string`；`Revision uint64`；`AckHash string`；`PurgeProofHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/workspace_deletion.go#L41)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableDecisionDeletionFenceID(s) || r.PurgeID != StableDecisionDeletionPurgeID(s) || r.Revision != 2 || r.AckHash != StableDecisionDeletionPurgeAckHash(s) || r.PurgeProofHash != StableDecisionDeletionPurgeProofHash(s)`

<a id="schema-queryworkspacedefinitionclarificationfactcommand"></a>

### QueryWorkspaceDefinitionClarificationFactCommand

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:80](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L80)
- 字段：`WorkspaceID common.WorkspaceID`；`TicketID common.TicketID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L91)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L110)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.TicketID == ""`；`err := c.Validate(); err != nil`；`!r.Found`；`r.Ticket != nil || r.Resolution != nil`；`r.Ticket == nil`；`err := r.Ticket.Validate(); err != nil`；`request.WorkspaceID != c.WorkspaceID || request.TicketID != c.TicketID`；`r.Resolution == nil`；`err := r.Resolution.Validate(); err != nil`；`resolution.WorkspaceID != request.WorkspaceID || resolution.TicketID != request.TicketID || resolution.OldCandidateID != request.CandidateID || resolution.OldCandidateRevision != request.CandidateRevision || resolution.OldCandidateHash != request.CandidateHash`

<a id="schema-queryworkspacedefinitionclarificationfactresult"></a>

### QueryWorkspaceDefinitionClarificationFactResult

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:85](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L85)
- 字段：`Found bool`；`Ticket *definitioncontract.WorkspaceDefinitionClarificationTicketRecord`；`Resolution *definitioncontract.WorkspaceDefinitionClarificationResolutionRecord`
- 字段引用类型：[WorkspaceDefinitionClarificationTicketRecord（shared/workorchdefinitioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdefinitioncontract/clarification.go#L104)；[WorkspaceDefinitionClarificationResolutionRecord（shared/workorchdefinitioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdefinitioncontract/clarification.go#L113)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L98)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L110)
- 空值/identity/版本/范围原式：`r.Ticket != nil`；`r.Resolution != nil`；`err := c.Validate(); err != nil`；`!r.Found`；`r.Ticket != nil || r.Resolution != nil`；`r.Ticket == nil`；`err := r.Ticket.Validate(); err != nil`；`request.WorkspaceID != c.WorkspaceID || request.TicketID != c.TicketID`；`r.Resolution == nil`；`err := r.Resolution.Validate(); err != nil`；`resolution.WorkspaceID != request.WorkspaceID || resolution.TicketID != request.TicketID || resolution.OldCandidateID != request.CandidateID || resolution.OldCandidateRevision != request.CandidateRevision || resolution.OldCandidateHash != request.CandidateHash`

<a id="schema-reconcileworkspacedefinitionclarificationcandidatecommand"></a>

### ReconcileWorkspaceDefinitionClarificationCandidateCommand

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:57](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L57)
- 字段：`OldRequest OpenWorkspaceDefinitionClarificationRequestCommand`；`NewRequest OpenWorkspaceDefinitionClarificationRequestCommand`
- 字段引用类型：[OpenWorkspaceDefinitionClarificationRequestCommand](#schema-openworkspacedefinitionclarificationrequestcommand)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L209)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L216)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L233)

<a id="schema-reconcileworkspacedefinitionclarificationcandidateresult"></a>

### ReconcileWorkspaceDefinitionClarificationCandidateResult

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:62](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L62)
- 字段：`WorkspaceID common.WorkspaceID`；`OldTicketID common.TicketID`；`OldResolutionID common.ResolutionID`；`NewTicketID common.TicketID`；`NewTicketDisposition DefinitionClarificationDisposition`；`NewRequestBindingHash string`；`ReconciliationBindingHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[DefinitionClarificationDisposition](#schema-definitionclarificationdisposition)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L224)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L233)

<a id="schema-registercontractchangeproposalcommand"></a>

### RegisterContractChangeProposalCommand

- 源码：[internal/modules/blocks/decision/pkg/events/contract_change.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/contract_change.go#L13)
- 字段：`TicketID common.TicketID`；`RequestID string`；`ExpectedRevision uint64`；`ExpectedHash string`；`Proposal decisionmodel.ContractChangeProposal`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ContractChangeProposal](#schema-contractchangeproposal)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/contract_change.go#L31)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/contract_change.go#L39)
- 空值/identity/版本/范围原式：`c.TicketID == "" || strings.TrimSpace(c.RequestID) == "" || !utf8.ValidString(c.RequestID) || len(c.RequestID) > 256 || ((c.ExpectedRevision == 0) != (c.ExpectedHash == "")) || c.ExpectedRevision >= decisionmodel.ContractChangeRevisionLimit`；`err := c.Validate(); err != nil`；`r.TicketID != c.TicketID || r.WorkspaceID != c.Proposal.WorkspaceID || r.RunID != c.Proposal.RunID || r.RequestID != c.RequestID || r.Revision != c.ExpectedRevision+1 || r.Hash != c.Proposal.Hash() || (r.Disposition != "registered" && r.Disposition != "duplicate")`

<a id="schema-registercontractchangeproposalresult"></a>

### RegisterContractChangeProposalResult

- 源码：[internal/modules/blocks/decision/pkg/events/contract_change.go:21](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/contract_change.go#L21)
- 字段：`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`RequestID string`；`Revision uint64`；`Hash string`；`Disposition string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/contract_change.go#L39)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.TicketID != c.TicketID || r.WorkspaceID != c.Proposal.WorkspaceID || r.RunID != c.Proposal.RunID || r.RequestID != c.RequestID || r.Revision != c.ExpectedRevision+1 || r.Hash != c.Proposal.Hash() || (r.Disposition != "registered" && r.Disposition != "duplicate")`

<a id="schema-resolution"></a>

### Resolution

- 源码：[internal/modules/blocks/decision/pkg/models/resolution.go:8](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/resolution.go#L8)
- 字段：`ID common.ResolutionID`；`TicketID common.TicketID`；`ResolvedAt string`；`Type string`；`Payload workorcheventpayload.Payload`；`ContinuationAction string`；`ResumeRunID common.RunID`；`SuccessorRunID common.RunID`
- 字段引用类型：[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)
- 实际方法：[CloneResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/resolution.go#L19)；[CloneResolutions](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/resolution.go#L28)

<a id="schema-resolveruntimedefinitionclarificationbyreplacementcommand"></a>

### ResolveRuntimeDefinitionClarificationByReplacementCommand

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:308](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L308)
- 字段：`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`PredecessorRunID common.RunID`；`ReplacementIntentID string`；`PredecessorDefinitionRevision string`；`PredecessorStartupInputHash string`；`SuccessorDefinitionRevision string`；`SuccessorStartupInputHash string`；`ReplacementIntentHash string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L346)；[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L419)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L429)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.TicketID != c.TicketID || r.WorkspaceID != c.WorkspaceID || r.PredecessorRunID != c.PredecessorRunID || r.ReplacementIntentID != c.ReplacementIntentID || r.ResolutionType != DefinitionClarificationResolutionReplacementBound`；`r.ResolutionID == "" || r.CurrentReplacementIntentID != "" || r.CurrentReplacementIntentHash != "" || r.ResolutionBindingHash != c.BindingHash(r.ResolutionID, DefinitionClarificationResolved, RuntimeDefinitionClarificationNoConflict, "", "")`；`r.CurrentReplacementIntentID != "" || r.CurrentReplacementIntentHash != "" || r.ResolutionBindingHash != c.BindingHash("", DefinitionClarificationStale, r.ConflictKind, "", "")`；`r.CurrentReplacementIntentID == "" || r.CurrentReplacementIntentHash == "" || r.ResolutionBindingHash != c.BindingHash("", DefinitionClarificationStale, r.ConflictKind, r.CurrentReplacementIntentID, r.CurrentReplacementIntentHash)`；`c.TicketID == "" || c.WorkspaceID == "" || c.PredecessorRunID == "" || c.ReplacementIntentID == "" || c.PredecessorDefinitionRevision == "" || c.PredecessorStartupInputHash == "" || c.SuccessorDefinitionRevision == "" || c.SuccessorStartupInputHash == "" || c.ReplacementIntentHash == "" || c.PredecessorDefinitionRevision == c.SuccessorDefinitionRevision || c.PredecessorStartupInputHash == c.SuccessorStartupInputHash`
- 显式分支集合（只表列源码case，default语义见validator）：`r.ConflictKind == RuntimeDefinitionClarificationNoConflict && (r.Disposition == DefinitionClarificationResolved || r.Disposition == DefinitionClarificationDuplicate)`；`r.Disposition == DefinitionClarificationStale && r.ResolutionID == "" && r.ConflictKind == RuntimeDefinitionClarificationScopeMismatch`；`r.Disposition == DefinitionClarificationStale && r.ResolutionID == "" && r.ConflictKind == RuntimeDefinitionClarificationTicketAlreadyBound`

<a id="schema-resolveruntimedefinitionclarificationbyreplacementresult"></a>

### ResolveRuntimeDefinitionClarificationByReplacementResult

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:320](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L320)
- 字段：`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`PredecessorRunID common.RunID`；`ReplacementIntentID string`；`ResolutionID common.ResolutionID`；`ResolutionType string`；`Disposition DefinitionClarificationDisposition`；`ConflictKind RuntimeDefinitionClarificationConflictKind`；`CurrentReplacementIntentID string`；`CurrentReplacementIntentHash string`；`ResolutionBindingHash string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[DefinitionClarificationDisposition](#schema-definitionclarificationdisposition)；[RuntimeDefinitionClarificationConflictKind](#schema-runtimedefinitionclarificationconflictkind)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L334)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L346)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.TicketID != c.TicketID || r.WorkspaceID != c.WorkspaceID || r.PredecessorRunID != c.PredecessorRunID || r.ReplacementIntentID != c.ReplacementIntentID || r.ResolutionType != DefinitionClarificationResolutionReplacementBound`；`r.ResolutionID == "" || r.CurrentReplacementIntentID != "" || r.CurrentReplacementIntentHash != "" || r.ResolutionBindingHash != c.BindingHash(r.ResolutionID, DefinitionClarificationResolved, RuntimeDefinitionClarificationNoConflict, "", "")`；`r.CurrentReplacementIntentID != "" || r.CurrentReplacementIntentHash != "" || r.ResolutionBindingHash != c.BindingHash("", DefinitionClarificationStale, r.ConflictKind, "", "")`；`r.CurrentReplacementIntentID == "" || r.CurrentReplacementIntentHash == "" || r.ResolutionBindingHash != c.BindingHash("", DefinitionClarificationStale, r.ConflictKind, r.CurrentReplacementIntentID, r.CurrentReplacementIntentHash)`
- 显式分支集合（只表列源码case，default语义见validator）：`r.ConflictKind == RuntimeDefinitionClarificationNoConflict && (r.Disposition == DefinitionClarificationResolved || r.Disposition == DefinitionClarificationDuplicate)`；`r.Disposition == DefinitionClarificationStale && r.ResolutionID == "" && r.ConflictKind == RuntimeDefinitionClarificationScopeMismatch`；`r.Disposition == DefinitionClarificationStale && r.ResolutionID == "" && r.ConflictKind == RuntimeDefinitionClarificationTicketAlreadyBound`

<a id="schema-resolveworkspacedefinitionclarificationbyadmissioncommand"></a>

### ResolveWorkspaceDefinitionClarificationByAdmissionCommand

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:55](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L55)
- 声明：`type ResolveWorkspaceDefinitionClarificationByAdmissionCommand definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity`
- 字段引用类型：[WorkspaceDefinitionClarificationAdmissionResolutionIdentity（shared/workorchdefinitioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdefinitioncontract/clarification.go#L60)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L190)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L194)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L254)

<a id="schema-resolveworkspacedefinitionclarificationbyadmissionresult"></a>

### ResolveWorkspaceDefinitionClarificationByAdmissionResult

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:71](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L71)
- 字段：`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`ResolutionID common.ResolutionID`；`ResolutionType string`；`Disposition DefinitionClarificationDisposition`；`ResolutionBindingHash string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[DefinitionClarificationDisposition](#schema-definitionclarificationdisposition)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L245)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L254)

<a id="schema-runtimedefinitionclarificationconflictkind"></a>

### RuntimeDefinitionClarificationConflictKind

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:300](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L300)
- 声明：`type RuntimeDefinitionClarificationConflictKind string`
- 源码枚举值：`RuntimeDefinitionClarificationNoConflict = "none"`；`RuntimeDefinitionClarificationTicketAlreadyBound = "ticket_already_bound"`；`RuntimeDefinitionClarificationScopeMismatch = "scope_mismatch"`
- 实际方法：[BindingHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L437)

<a id="schema-runtimedefinitiongapkind"></a>

### RuntimeDefinitionGapKind

- 源码：[internal/modules/blocks/decision/pkg/events/definition_clarification.go:34](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/definition_clarification.go#L34)
- 声明：`type RuntimeDefinitionGapKind string`
- 源码枚举值：`RuntimeDefinitionGapImplicitOneOf = "implicit_one_of"`；`RuntimeDefinitionGapUndeclaredPrerequisite = "undeclared_prerequisite"`；`RuntimeDefinitionGapSemanticAmbiguity = "semantic_ambiguity"`

<a id="schema-runtimegovernanceresolutionproposal"></a>

### RuntimeGovernanceResolutionProposal

- 源码：[internal/modules/blocks/decision/pkg/models/runtime_governance_proposal.go:12](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/runtime_governance_proposal.go#L12)
- 字段：`ResolutionID common.ResolutionID`；`ResolutionType string`；`Payload workorcheventpayload.Payload`；`ContinuationAction string`；`ResumeRunID common.RunID`；`SuccessorRunID common.RunID`；`PayloadHash string`；`State RuntimeGovernanceResolutionProposalState`；`FailureClass string`；`FailureReason string`；`CreatedAt string`；`UpdatedAt string`；`EvidenceRecordedAt string`；`ResolutionNotificationPublishedAt string`
- 字段引用类型：[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeGovernanceResolutionProposalState](#schema-runtimegovernanceresolutionproposalstate)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/runtime_governance_proposal.go#L37)

<a id="schema-runtimegovernanceresolutionproposalstate"></a>

### RuntimeGovernanceResolutionProposalState

- 源码：[internal/modules/blocks/decision/pkg/models/runtime_governance_proposal.go:29](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/runtime_governance_proposal.go#L29)
- 声明：`type RuntimeGovernanceResolutionProposalState string`
- 源码枚举值：`RuntimeGovernanceResolutionProposalPending = "pending"`；`RuntimeGovernanceResolutionProposalCommitted = "committed"`；`RuntimeGovernanceResolutionProposalRejected = "rejected"`

<a id="schema-sealworkspacedecisiondeletioncommand"></a>

### SealWorkspaceDecisionDeletionCommand

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:86](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L86)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`DataDispositionIntent deletioncontract.Mode`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L94)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L97)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/workspace_deletion.go#L31)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableDecisionDeletionFenceID(s) || r.Revision != 1 || r.AckHash != StableDecisionDeletionSealAckHash(s)`

<a id="schema-sealworkspacedecisiondeletionresult"></a>

### SealWorkspaceDecisionDeletionResult

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:159](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L159)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`Revision uint64`；`AckHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/workspace_deletion.go#L31)
- 空值/identity/版本/范围原式：`err := c.Validate(); err != nil`；`r.WorkspaceID != s.WorkspaceID || r.DeletionID != s.DeletionID || r.DeletionGeneration != s.DeletionGeneration || r.FenceID != StableDecisionDeletionFenceID(s) || r.Revision != 1 || r.AckHash != StableDecisionDeletionSealAckHash(s)`

<a id="schema-ticket"></a>

### Ticket

- 源码：[internal/modules/blocks/decision/pkg/models/ticket.go:8](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/ticket.go#L8)
- 字段：`ID common.TicketID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`Reason string`；`DecisionKind string`；`Question string`；`Context workorcheventpayload.Payload`；`State common.DecisionTicketState`；`AutoGovernance workorcheventpayload.Payload`；`FailureClass string`；`FailureSignature string`；`AllowedRecoveryActions []string`；`DefinitionClarification *DefinitionClarificationBinding`；`RuntimeGovernanceProposal *RuntimeGovernanceResolutionProposal`；`ContractChangeRevisions []ContractChangeRevision`；`ResolvedAt string`；`ResolvedBy string`；`CreatedAt string`；`UpdatedAt string`；`CreatedNotificationPublishedAt string`；`ResolutionNotificationPublishedAt string`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[DecisionTicketState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L11)；[DefinitionClarificationBinding](#schema-definitionclarificationbinding)；[RuntimeGovernanceResolutionProposal](#schema-runtimegovernanceresolutionproposal)；[ContractChangeRevision](#schema-contractchangerevision)
- 实际方法：[ValidateContractChangeAppend](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change_append.go#L10)；[ValidateContractChange](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/contract_change.go#L238)；[CloneTicket](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/ticket.go#L33)；[CloneTickets](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/ticket.go#L52)
- 空值/identity/版本/范围原式：`current == nil || next == nil || !current.IsContractChange() || !next.IsContractChange() || current.ValidateContractChange() != nil || next.ValidateContractChange() != nil || len(next.ContractChangeRevisions) != len(current.ContractChangeRevisions)+1 || !reflect.DeepEqual(current.ContractChangeRevisions, next.ContractChangeRevisions[:len(current.ContractChangeRevisions)])`；`!reflect.DeepEqual(before, after)`；`t == nil || t.ID == "" || t.WorkspaceID == "" || t.DecisionKind != ContractChangeDecisionKind || t.StepID != "" || t.State != common.DecisionTicketStateOpen || t.ResolvedAt != "" || t.ResolvedBy != "" || t.CreatedNotificationPublishedAt != "" || t.ResolutionNotificationPublishedAt != "" || t.DefinitionClarification != nil || t.RuntimeGovernanceProposal != nil || len(t.AllowedRecoveryActions) != 0 || len(t.Context.Fields) != 0 || len(t.AutoGovernance.Fields) != 0 || t.FailureClass != "" || t.FailureSignature != "" || t.Question != "" || len(t.ContractChangeRevisions) == 0 || len(t.ContractChangeRevisions) > ContractChangeRevisionLimit`；`t.Reason != base.WhyChange`；`strings.TrimSpace(r.RequestID) == "" || !utf8.ValidString(r.RequestID) || requests[r.RequestID] || r.Revision != uint64(n+1) || r.RegisteredAt == "" || r.Hash != r.Proposal.Hash() || r.Proposal.Validate() != nil || r.Proposal.WorkspaceID != t.WorkspaceID || r.Proposal.RunID != t.RunID || r.Proposal.BaseDefinitionRevision != base.BaseDefinitionRevision || r.Proposal.BaseStartupInputHash != base.BaseStartupInputHash || r.Proposal.BaseInputRevision != base.BaseInputRevision || r.Proposal.BaseInputHash != base.BaseInputHash`；`_, err := time.Parse(time.RFC3339Nano, r.RegisteredAt); err != nil`；`len(r.RequestID) > 256`；`err != nil || len(body) > ContractChangeHistoryByteLimit`

<a id="schema-ticketchangeddata"></a>

### TicketChangedData

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:119](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L119)
- 字段：`NotificationID string`；`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`Reason string`；`DecisionKind string`；`State common.DecisionTicketState`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[DecisionTicketState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L11)

<a id="schema-ticketlistscope"></a>

### TicketListScope

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:24](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L24)
- 声明：`type TicketListScope string`
- 源码枚举值：`TicketListScopeAll = "all"`

<a id="schema-ticketresolveddata"></a>

### TicketResolvedData

- 源码：[internal/modules/blocks/decision/pkg/events/messages.go:130](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go#L130)
- 字段：`NotificationID string`；`Ticket TicketChangedData`；`ResolutionID common.ResolutionID`；`ResolutionType string`；`ResolutionPayload []workorcheventpayload.Field`；`ContinuationAction string`；`ResumeRunID common.RunID`；`SuccessorRunID common.RunID`
- 字段引用类型：[TicketChangedData](#schema-ticketchangeddata)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[Field（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L27)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L183)：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`；`ModuleID = "blocks/decision"`
- [handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/command_router.go#L50)：来源集合由switch/返回表达式判定，见完整函数
- [decisionResolutionIngressSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/resolution_commands.go#L18)：来源集合由switch/返回表达式判定，见完整函数
  - helper完整分支：`func decisionResolutionIngressSourceAllowed(source string) bool { switch source { case decisionResolutionApplicationSource, decisionResolutionGovernanceSource: return true default: return false } }`
  - 当前来源常量：`decisionResolutionApplicationSource = "application/workorchd/appservice"`；`decisionResolutionGovernanceSource = "kernel/governance"`
- [executePurgeWorkspaceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/workspace_deletion_internal_event.go#L87)：`!ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil`
  - 当前来源常量：`workspaceDeletionCommandSource = "blocks/workspace"`
- [executeSealWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/workspace_deletion_internal_event.go#L66)：`!ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil`
  - 当前来源常量：`workspaceDeletionCommandSource = "blocks/workspace"`
- [handleOpenRuntimeDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L106)：`!ok || ev.Source() != runtimeDefinitionClarificationSource || m == nil || m.tickets == nil`
  - 当前来源常量：`runtimeDefinitionClarificationSource = "kernel/orchestrator"`
- [handleRuntimeGovernanceRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/runtime_governance_adapter.go#L19)：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`
- [handleOpenWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L74)：`!ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil`
  - 当前来源常量：`workspaceDefinitionClarificationSource = "application/workorchd"`
- [handleQueryWorkspaceDefinitionClarificationFact](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L37)：`!ok || ev.Source() != workspaceDefinitionFactQuerySource || m == nil || m.tickets == nil || m.resolutions == nil`
  - 当前来源常量：`workspaceDefinitionFactQuerySource = "blocks/workspace"`
- [handleReconcileWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L53)：`!ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil || m.resolutions == nil`
  - 当前来源常量：`workspaceDefinitionClarificationSource = "application/workorchd"`
- [handleRegisterContractChangeProposal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/contract_change.go#L14)：`!ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.service == nil`
  - 当前来源常量：`workspaceDefinitionClarificationSource = "application/workorchd"`
- [handleResolveRuntimeDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L122)：`!ok || ev.Source() != runtimeDefinitionReplacementSource || m == nil || m.tickets == nil || m.resolutions == nil`
  - 当前来源常量：`runtimeDefinitionReplacementSource = "blocks/run"`
- [handleResolveWorkspaceDefinitionClarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/definition_clarification.go#L90)：`!ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil || m.resolutions == nil`
  - 当前来源常量：`workspaceDefinitionClarificationSource = "application/workorchd"`