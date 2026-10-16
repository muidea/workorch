# orchestrator：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页18个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Orchestrator是SM-2/3/4与Runtime aggregate的唯一语义owner。当前Drive只接受owner内部受控来源，外部执行意图不能直接指定next action/state；普通Start/Resume的RecoveryLeaseGeneration=0，只有持久SM-4恢复凭据可驱动wait probe。各逐edge完整写集引用Core和Runtime规格，不在这里复制。

本版明确保留OrchestratorCommandResult有限结构；五个list查询分别仅允许ToolCalls、Patches、ValidationResults、CoverageWorkItems、ArtifactContinuations一臂，其他slice必须nil；selected slice的nil与空均为零结果。新增五个query-specific validator，并由handler输出和caller都检查；不造通用envelope、不继续“必要时拆分”悬项。

Query/CommitRuntimeOperationReceipt向真实能力owner提供持久receipt服务，能力owner只写自己authored fact；Found必须等价于Receipt!=nil，receipt ExecutionKey/Fence/OperationID一致；commit返回必须SameRuntimeOperationReceipt。working receipt接纳及相关CWI/no-credit效果由同aggregate单CAS协调；现有ImplementationWork production admission继续closed。

## 逐topic合同

<a id="ct-orchestrator-orchestratorcommandbeginworkspaceruntimequiescence"></a>

### CT-orchestrator-OrchestratorCommandBeginWorkspaceRuntimeQuiescence

- 边界：`TopicOrchestratorCommandBeginWorkspaceRuntimeQuiescence` = `/orchestrator/command/begin-workspace-runtime-quiescence`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L320)
- 接收provider：orchestrator；Command [BeginWorkspaceRuntimeQuiescenceCommand](#schema-beginworkspaceruntimequiescencecommand) → Result [BeginWorkspaceRuntimeQuiescenceResult](#schema-beginworkspaceruntimequiescenceresult)；target合法caller：workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleWorkspaceRuntimeQuiescence`（orchestrator）
- 接收入口：[handleWorkspaceRuntimeQuiescence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L20)
- 已观察外部引用：workspace [driveWorkspaceDeletionQuiescence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L149)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[handleWorkspaceRuntimeQuiescence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L20)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || command.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-orchestrator-orchestratorcommandconfirmworkspaceruntimequiescence"></a>

### CT-orchestrator-OrchestratorCommandConfirmWorkspaceRuntimeQuiescence

- 边界：`TopicOrchestratorCommandConfirmWorkspaceRuntimeQuiescence` = `/orchestrator/command/confirm-workspace-runtime-quiescence`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L321)
- 接收provider：orchestrator；Command [ConfirmWorkspaceRuntimeQuiescenceCommand](#schema-confirmworkspaceruntimequiescencecommand) → Result [ConfirmWorkspaceRuntimeQuiescenceResult](#schema-confirmworkspaceruntimequiescenceresult)；target合法caller：workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleWorkspaceRuntimeQuiescence`（orchestrator）
- 接收入口：[handleWorkspaceRuntimeQuiescence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L20)
- 已观察外部引用：workspace [driveWorkspaceDeletionQuiescence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L149)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[handleWorkspaceRuntimeQuiescence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L20)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || command.Validate() != nil`；`err := intent.Validate(); err != nil || m == nil || m.workspaceDeletions == nil || m.runtimeController == nil || m.EventHub() == nil`；`m.workspaceRuntimeActivityLocked(intent.WorkspaceID)`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-orchestrator-orchestratorcommandlistartifactcontinuationsbyrun"></a>

### CT-orchestrator-OrchestratorCommandListArtifactContinuationsByRun

- 边界：`TopicOrchestratorCommandListArtifactContinuationsByRun` = `/orchestrator/command/list-artifact-continuations-by-run`；CF11；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L17)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L330)
- 接收provider：orchestrator；Command [ListArtifactContinuationsByRunCommand](#schema-listartifactcontinuationsbyruncommand) → Result [OrchestratorCommandResult](#schema-orchestratorcommandresult)；target合法caller：workorchd、run、workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleMagicCommand`（orchestrator）
- 接收入口：[handleMagicCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L1211)
- 已观察外部引用：workorchd [ListArtifactContinuationsByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/orchestrator_artifact_continuation_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：target change：只接受OrchestratorCommandResult.ArtifactContinuations查询臂；其他四臂必须nil，selected nil/空均为合法零结果；query-specific validator需双端执行，scope绑定Workspace/Run，不返回可变内部对象。
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-orchestrator-orchestratorcommandlistcoverageworkitemsbyrun"></a>

### CT-orchestrator-OrchestratorCommandListCoverageWorkItemsByRun

- 边界：`TopicOrchestratorCommandListCoverageWorkItemsByRun` = `/orchestrator/command/list-coverage-work-items-by-run`；CF11；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L16)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L329)
- 接收provider：orchestrator；Command [ListCoverageWorkItemsByRunCommand](#schema-listcoverageworkitemsbyruncommand) → Result [OrchestratorCommandResult](#schema-orchestratorcommandresult)；target合法caller：workorchd、run、workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleMagicCommand`（orchestrator）
- 接收入口：[handleMagicCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L1211)
- 已观察外部引用：workorchd [ListCoverageWorkItemsByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/orchestrator_coverage_work_item_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：target change：只接受OrchestratorCommandResult.CoverageWorkItems查询臂；其他四臂必须nil，selected nil/空均为合法零结果；query-specific validator需双端执行，scope绑定Workspace/Run，不返回可变内部对象。
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-orchestrator-orchestratorcommandlistpatchesbyrun"></a>

### CT-orchestrator-OrchestratorCommandListPatchesByRun

- 边界：`TopicOrchestratorCommandListPatchesByRun` = `/orchestrator/command/list-patches-by-run`；CF11；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L327)
- 接收provider：orchestrator；Command [ListPatchesByRunCommand](#schema-listpatchesbyruncommand) → Result [OrchestratorCommandResult](#schema-orchestratorcommandresult)；target合法caller：workorchd、run、workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleMagicCommand`（orchestrator）
- 接收入口：[handleMagicCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L1211)
- 已观察外部引用：workorchd [ListPatchesByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/orchestrator_reads.go#L28)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：target change：只接受OrchestratorCommandResult.Patches查询臂；其他四臂必须nil，selected nil/空均为合法零结果；query-specific validator需双端执行，scope绑定Workspace/Run，不返回可变内部对象。
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-orchestrator-orchestratorcommandlisttoolcallsbyrun"></a>

### CT-orchestrator-OrchestratorCommandListToolCallsByRun

- 边界：`TopicOrchestratorCommandListToolCallsByRun` = `/orchestrator/command/list-tool-calls-by-run`；CF11；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L326)
- 接收provider：orchestrator；Command [ListToolCallsByRunCommand](#schema-listtoolcallsbyruncommand) → Result [OrchestratorCommandResult](#schema-orchestratorcommandresult)；target合法caller：workorchd、run、workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleMagicCommand`（orchestrator）
- 接收入口：[handleMagicCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L1211)
- 已观察外部引用：workorchd [ListToolCallsByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/orchestrator_reads.go#L14)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：target change：只接受OrchestratorCommandResult.ToolCalls查询臂；其他四臂必须nil，selected nil/空均为合法零结果；query-specific validator需双端执行，scope绑定Workspace/Run，不返回可变内部对象。
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-orchestrator-orchestratorcommandlistvalidationresultsbyrun"></a>

### CT-orchestrator-OrchestratorCommandListValidationResultsByRun

- 边界：`TopicOrchestratorCommandListValidationResultsByRun` = `/orchestrator/command/list-validation-results-by-run`；CF11；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L15)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L328)
- 接收provider：orchestrator；Command [ListValidationResultsByRunCommand](#schema-listvalidationresultsbyruncommand) → Result [OrchestratorCommandResult](#schema-orchestratorcommandresult)；target合法caller：workorchd、run、workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleMagicCommand`（orchestrator）
- 接收入口：[handleMagicCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L1211)
- 已观察外部引用：workorchd [ListValidationResultsByRunOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/orchestrator_reads.go#L42)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：target change：只接受OrchestratorCommandResult.ValidationResults查询臂；其他四臂必须nil，selected nil/空均为合法零结果；query-specific validator需双端执行，scope绑定Workspace/Run，不返回可变内部对象。
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-orchestrator-orchestratorcommandpurgeworkspaceevidence"></a>

### CT-orchestrator-OrchestratorCommandPurgeWorkspaceEvidence

- 边界：`TopicOrchestratorCommandPurgeWorkspaceEvidence` = `/orchestrator/command/purge-workspace-evidence`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L323)
- 接收provider：orchestrator；Command [PurgeWorkspaceOrchestratorEvidenceCommand](#schema-purgeworkspaceorchestratorevidencecommand) → Result [PurgeWorkspaceOrchestratorEvidenceResult](#schema-purgeworkspaceorchestratorevidenceresult)；target合法caller：blocks/workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleWorkspaceDeletion`（orchestrator）
- 接收入口：[handleWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L57)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextPurge](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L304)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_deletion.go#L12)；[handleWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L57)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementOrchestratorCommandSource` 分支谓词（[handleWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L57)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event.Source() != workspaceRetirementOrchestratorCommandSource || m == nil`；`!ok || command.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-orchestrator-orchestratorcommandrecordapprovalevidence"></a>

### CT-orchestrator-OrchestratorCommandRecordApprovalEvidence

- 边界：`TopicOrchestratorCommandRecordApprovalEvidence` = `/orchestrator/command/record-approval-evidence`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L325)
- 接收provider：orchestrator；Command [RecordApprovalEvidenceCommand](#schema-recordapprovalevidencecommand) → Result [RecordApprovalEvidenceResult](#schema-recordapprovalevidenceresult)；target合法caller：blocks/approval
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleApprovalEvidenceCommand`（orchestrator）
- 接收入口：[handleApprovalEvidenceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/governance_evidence_command.go#L105)
- 已观察外部引用：approval [recordApprovalGovernanceEvidence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/evidence_event.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[handleApprovalEvidenceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/governance_evidence_command.go#L105)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != "blocks/approval"` 分支谓词（[handleApprovalEvidenceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/governance_evidence_command.go#L105)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || ev == nil || ev.Source() != "blocks/approval"`；`m == nil || m.evidenceRecords == nil || command.EvidenceRecordID == "" || command.WorkspaceID == "" || command.RunID == "" || command.ApprovalID == "" || command.ResolutionID == "" || command.Principal.Validate() != nil`；`command.Reason != ""`；`command.Action != ""`；`err := m.withOrchestratorEvidenceMutation(ctx, command.WorkspaceID, func(guarded context.Context) error { return repository.CreateOrVerifyImmutableEvidenceRecord(guarded, m.evidenceRecords, record) }); err != nil`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-orchestrator-orchestratorcommandrecorddecisionevidence"></a>

### CT-orchestrator-OrchestratorCommandRecordDecisionEvidence

- 边界：`TopicOrchestratorCommandRecordDecisionEvidence` = `/orchestrator/command/record-decision-evidence`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L324)
- 接收provider：orchestrator；Command [RecordDecisionEvidenceCommand](#schema-recorddecisionevidencecommand) → Result [RecordDecisionEvidenceResult](#schema-recorddecisionevidenceresult)；target合法caller：blocks/decision
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleDecisionEvidenceCommand`（orchestrator）
- 接收入口：[handleDecisionEvidenceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/governance_evidence_command.go#L19)
- 已观察外部引用：decision [recordDecisionResolutionEvidence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/evidence_event.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[handleDecisionEvidenceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/governance_evidence_command.go#L19)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != "blocks/decision"` 分支谓词（[handleDecisionEvidenceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/governance_evidence_command.go#L19)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || ev == nil || ev.Source() != "blocks/decision"`；`m == nil || m.evidenceRecords == nil || command.EvidenceRecordID == "" || command.WorkspaceID == "" || command.RunID == "" || command.TicketID == "" || command.ResolutionID == ""`；`command.Reason != ""`；`command.Question != ""`；`command.ContinuationAction != ""`；`command.ResumeRunID != ""`；`command.SuccessorRunID != ""`；`err := m.withOrchestratorEvidenceMutation(ctx, command.WorkspaceID, func(guarded context.Context) error { return repository.CreateOrVerifyImmutableEvidenceRecord(guarded, m.evidenceRecords, record) }); err != nil`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-orchestrator-orchestratorcommandsealworkspacedeletion"></a>

### CT-orchestrator-OrchestratorCommandSealWorkspaceDeletion

- 边界：`TopicOrchestratorCommandSealWorkspaceDeletion` = `/orchestrator/command/seal-workspace-deletion`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L322)
- 接收provider：orchestrator；Command [SealWorkspaceOrchestratorDeletionCommand](#schema-sealworkspaceorchestratordeletioncommand) → Result [SealWorkspaceOrchestratorDeletionResult](#schema-sealworkspaceorchestratordeletionresult)；target合法caller：blocks/workspace
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleWorkspaceDeletion`（orchestrator）
- 接收入口：[handleWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L57)
- 已观察外部引用：workspace [driveWorkspaceDeletionNextSeal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L221)；workspace [bindWorkspaceDeletionOwnerLane](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go#L543)
- 本topic入口/具体类型分支的Source审查索引：[executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_deletion.go#L12)；[handleWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L57)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceRetirementOrchestratorCommandSource` 分支谓词（[handleWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L57)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event.Source() != workspaceRetirementOrchestratorCommandSource || m == nil`；`!ok || command.Validate() != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-orchestrator-orchestratorcommitruntimegovernanceapproval"></a>

### CT-orchestrator-OrchestratorCommitRuntimeGovernanceApproval

- 边界：`TopicOrchestratorCommitRuntimeGovernanceApproval` = `/orchestrator/runtime-governance/commit-approval`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L22)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L336)
- 接收provider：orchestrator；Command [CommitRuntimeGovernanceApprovalResolutionCommand](#schema-commitruntimegovernanceapprovalresolutioncommand) → Result [CommitRuntimeGovernanceResolutionResult](#schema-commitruntimegovernanceresolutionresult)；target合法caller：blocks/approval
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleRuntimeGovernanceApprovalResolution`（orchestrator）
- 接收入口：[handleRuntimeGovernanceApprovalResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L24)
- 已观察外部引用：approval [commitRuntimeApprovalProposal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/resolution_commands.go#L68)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeGovernanceApprovalResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L24)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != approvalcommon.ModuleID` 分支谓词（[handleRuntimeGovernanceApprovalResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L24)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || event == nil || event.Source() != approvalcommon.ModuleID`；`!ok || m.runtimeController == nil || command.ResolutionID == "" || command.ApprovalID == "" || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.Summary) == ""`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-orchestrator-orchestratorcommitruntimegovernanceticket"></a>

### CT-orchestrator-OrchestratorCommitRuntimeGovernanceTicket

- 边界：`TopicOrchestratorCommitRuntimeGovernanceTicket` = `/orchestrator/runtime-governance/commit-ticket-resolution`；CF07；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L23)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L335)
- 接收provider：orchestrator；Command [CommitRuntimeGovernanceTicketResolutionCommand](#schema-commitruntimegovernanceticketresolutioncommand) → Result [CommitRuntimeGovernanceResolutionResult](#schema-commitruntimegovernanceresolutionresult)；target合法caller：blocks/decision
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleRuntimeGovernanceTicketResolution`（orchestrator）
- 接收入口：[handleRuntimeGovernanceTicketResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L55)
- 已观察外部引用：decision [commitRuntimeGovernanceTicketProposal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/runtime_governance_adapter.go#L124)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeGovernanceTicketResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L55)；[handleRuntimeGovernanceTicketResolutionLocked](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L65)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != decisioncommon.ModuleID` 分支谓词（[handleRuntimeGovernanceTicketResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L55)）
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || event == nil || event.Source() != decisioncommon.ModuleID`
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-orchestrator-orchestratorcommitruntimeoperationreceipt"></a>

### CT-orchestrator-OrchestratorCommitRuntimeOperationReceipt

- 边界：`TopicOrchestratorCommitRuntimeOperationReceipt` = `/orchestrator/runtime-operation/receipt/commit`；CF06；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L21)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L333)
- 接收provider：orchestrator；Command [CommitRuntimeOperationReceiptCommand](#schema-commitruntimeoperationreceiptcommand) → Result [CommitRuntimeOperationReceiptResult](#schema-commitruntimeoperationreceiptresult)；target合法caller：与封存operation owner精确匹配的file/git/shell/ssh/web/mcp（不得互代）
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleRuntimeOperationReceipt`（orchestrator）
- 接收入口：[handleRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_receipt_event_adapter.go#L25)
- 已观察外部引用：file [commitRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/runtime_operation_receipt.go#L60)；ssh [commitRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/ssh/biz/runtime_operation_receipt.go#L52)；git [commitRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/runtime_operation_receipt.go#L52)；shell [commitRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/runtime_operation_receipt.go#L52)；web [commitRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/runtime_operation_receipt.go#L52)；mcp [commitRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/runtime_operation_receipt.go#L52)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_receipt_event_adapter.go#L25)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：
- 特化承诺：author是能力owner、persist是Orchestrator。返回receipt必须与提交SameRuntimeOperationReceipt；StateRevision仅提供提交事实，能力方不解释Runtime语义。working/no-credit sibling写集单CAS，ImplementationWork admission仍closed。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`event.Source() == "" || event.Source() != strings.TrimSpace(event.Source())`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-orchestrator-orchestratordriveruntime"></a>

### CT-orchestrator-OrchestratorDriveRuntime

- 边界：`TopicOrchestratorDriveRuntime` = `/orchestrator/runtime/drive`；CF09；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L19)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L331)
- 接收provider：orchestrator；Command [DriveRuntimeCommand](#schema-driveruntimecommand) → Result [DriveRuntimeResult](#schema-driveruntimeresult)；target合法caller：kernel/orchestrator（owner内部受控drive）
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleRuntimeDrive`（orchestrator）
- 接收入口：[handleRuntimeDrive](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_drive.go#L172)
- owner内构造/发送静态引用：[DriveRuntime](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_drive.go#L137)
- usage：未在本次已读调用端找到外部引用；不是已证明unused。target仅允许上列caller/订阅者；需双端VS补证
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeDrive](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_drive.go#L172)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != runtimeInternalDriveSource` 分支谓词（[handleRuntimeDrive](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_drive.go#L172)）
- 特化承诺：[CS13](../Workorch_组件固定合同与分层验收规范.md#cs13)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || m == nil || event.Source() != runtimeInternalDriveSource`
- 验收适用集：[CP11](../Workorch_组件固定合同与分层验收规范.md#cp11)

<a id="ct-orchestrator-orchestratorqueryruntimeoperationreceipt"></a>

### CT-orchestrator-OrchestratorQueryRuntimeOperationReceipt

- 边界：`TopicOrchestratorQueryRuntimeOperationReceipt` = `/orchestrator/runtime-operation/receipt/query`；CF06；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L20)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L332)
- 接收provider：orchestrator；Command [QueryRuntimeOperationReceiptCommand](#schema-queryruntimeoperationreceiptcommand) → Result [QueryRuntimeOperationReceiptResult](#schema-queryruntimeoperationreceiptresult)；target合法caller：与封存operation owner精确匹配的file/git/shell/ssh/web/mcp（不得互代）
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleRuntimeOperationReceipt`（orchestrator）
- 接收入口：[handleRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_receipt_event_adapter.go#L25)
- 已观察外部引用：file [queryRuntimeOperationReceiptWithSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/runtime_operation_receipt.go#L22)；ssh [queryRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/ssh/biz/runtime_operation_receipt.go#L18)；git [queryRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/runtime_operation_receipt.go#L18)；shell [queryRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/runtime_operation_receipt.go#L18)；shell [handleRuntimeValidationRawOutput](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/validation_raw_output_query.go#L17)；web [queryRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/runtime_operation_receipt.go#L18)；mcp [queryRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/runtime_operation_receipt.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_receipt_event_adapter.go#L25)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：Found==(Receipt!=nil)；found时Validate并比ExecutionKey/Fence/OperationID，不匹配ErrConflict；not-found不独立授权新effect，仍检查seal/fence。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`command.ReadSource != nil`；`source.WorkspaceID != command.WorkspaceID || source.RunID != command.RunID || source.DispatchID != command.DispatchID || source.OperationID != command.OperationID || source.Execution.ExecutionKey != command.ExecutionKey || source.Execution.Fence != command.Fence || !reflect.DeepEqual(source.Execution.GovernanceGrant, command.GovernanceGrant)`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-orchestrator-orchestratorrequestruntimetermination"></a>

### CT-orchestrator-OrchestratorRequestRuntimeTermination

- 边界：`TopicOrchestratorRequestRuntimeTermination` = `/orchestrator/runtime-lifecycle/request-termination`；CF10；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L24)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L334)
- 接收provider：orchestrator；Command [RequestRuntimeTerminationCommand](#schema-requestruntimeterminationcommand) → Result [RequestRuntimeTerminationResult](#schema-requestruntimeterminationresult)；target合法caller：blocks/run
- 实际注册：[subscribeRuntimeIngress](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L269) → `handleRuntimeTerminationRequest`（orchestrator）
- 接收入口：[handleRuntimeTerminationRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L341)
- 已观察外部引用：run [deliverClaimedReplacementTermination](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_execution_intent.go#L800)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeTerminationRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L341)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != "blocks/run"` 分支谓词（[handleRuntimeTerminationRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L341)）
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != "blocks/run" || m == nil || cmd.Validate() != nil || cmd.RequestedBy != "blocks/run"`；`!runtimeTerminationCommandMatchesAggregate(cmd, fresh)`；`retired := aggregate.Facts.Recovery.Normalized().RetiredTerminalDeliveryLease; retired.Validate() == nil && retired.State == runtimecontract.RuntimeDriveLeaseRunning`
- 验收适用集：[CP04](../Workorch_组件固定合同与分层验收规范.md#cp04)

<a id="ct-orchestrator-orchestratorruntimedriveobserved"></a>

### CT-orchestrator-OrchestratorRuntimeDriveObserved

- 边界：`TopicOrchestratorRuntimeDriveObserved` = `/orchestrator/runtime/drive-observed`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/topics.go#L29)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L339)
- 唯一事实owner/发布者：orchestrator；Data [RuntimeDriveObservedData](#schema-runtimedriveobserveddata)；同步Command/Result：不适用；允许订阅者：eventstream、workorchd
- 实际注册：[Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53) → `handleRuntimeDriveObserved`（workorchd）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：workorchd [Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || m.runtimeController == nil || event.Scope.WorkspaceID == "" || event.Scope.RunID == ""`；`_, quiescing := m.quiescingWorkspaces[event.Scope.WorkspaceID]; quiescing`；`err == nil && result.Aggregate.Facts.WorkspaceID != ""`
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-artifactcontinuation"></a>

### ArtifactContinuation

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:182](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L182)
- 字段：`ID common.ArtifactContinuationID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`RootStepID common.StepID`；`CurrentStepID common.StepID`；`ArtifactID string`；`TargetRef string`；`Operation string`；`OutputMode string`；`AuthorityTransaction runtimecontract.AuthorityTransaction`；`CandidateAttempt int`；`StablePrefixHash string`；`MaxChunks int`；`TailBytes int`；`CoverageTargets []string`；`State common.ArtifactContinuationState`；`ExpectedSequence int`；`ReceivedSequences []int`；`ReceivedChunkHashes []ArtifactContinuationChunkHash`；`FinalReceived bool`；`BytesWritten int64`；`RollingHash string`；`LastChunkHash string`；`PreviousChunkTail string`；`RetryCounters []ArtifactContinuationRetryCounter`；`EvidenceRefs []string`；`CreatedAt string`；`UpdatedAt string`；`Metadata workorcheventpayload.Payload`
- 字段引用类型：[ArtifactContinuationID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L17)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[AuthorityTransaction（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1750)；[ArtifactContinuationState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L21)；[ArtifactContinuationChunkHash](#schema-artifactcontinuationchunkhash)；[ArtifactContinuationRetryCounter](#schema-artifactcontinuationretrycounter)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-artifactcontinuationchunkhash"></a>

### ArtifactContinuationChunkHash

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:169](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L169)
- 字段：`Sequence int`；`Hash string`
- 实际方法：[artifactContinuationChunkHashes](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L267)；[artifactContinuationChunkHashMap](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L276)
- 空值/identity/版本/范围原式：`_, exists := ret[item.Sequence]; exists`

<a id="schema-artifactcontinuationretrycounter"></a>

### ArtifactContinuationRetryCounter

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:174](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L174)
- 字段：`Key string`；`Count int`

<a id="schema-beginworkspaceruntimequiescencecommand"></a>

### BeginWorkspaceRuntimeQuiescenceCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go:12](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L12)
- 字段：`Intent deletioncontract.WorkspaceRuntimeQuiescenceIntent`
- 字段引用类型：[WorkspaceRuntimeQuiescenceIntent（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/runtime_quiescence.go#L127)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L16)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L25)
- 空值/identity/版本/范围原式：`err := intent.Validate(); err != nil`；`r.WorkspaceID != intent.WorkspaceID || r.DeletionID != intent.DeletionID || r.GateID != StableWorkspaceRuntimeQuiescingGateID(intent) || r.OrchestratorQuiescenceRevision != 1`

<a id="schema-beginworkspaceruntimequiescenceresult"></a>

### BeginWorkspaceRuntimeQuiescenceResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go:18](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L18)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`GateID string`；`OrchestratorQuiescenceRevision uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L25)
- 空值/identity/版本/范围原式：`err := intent.Validate(); err != nil`；`r.WorkspaceID != intent.WorkspaceID || r.DeletionID != intent.DeletionID || r.GateID != StableWorkspaceRuntimeQuiescingGateID(intent) || r.OrchestratorQuiescenceRevision != 1`

<a id="schema-commitruntimegovernanceapprovalresolutioncommand"></a>

### CommitRuntimeGovernanceApprovalResolutionCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:101](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L101)
- 字段：`ResolutionID common.ResolutionID`；`ApprovalID common.ApprovalID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Outcome runtimecontract.RuntimeGovernanceResolutionOutcome`；`Summary string`；`EvidenceRefs []string`
- 字段引用类型：[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeGovernanceResolutionOutcome（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_governance.go#L14)

<a id="schema-commitruntimegovernanceresolutionresult"></a>

### CommitRuntimeGovernanceResolutionResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:111](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L111)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`ResolutionID common.ResolutionID`；`Disposition RuntimeGovernanceResolutionDisposition`；`StateRevision uint64`；`EvidenceRevision string`；`AuthoritySnapshotID string`；`State runtimecontract.ConvergenceState`；`Terminal *runtimecontract.RuntimeTerminalDecision`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[RuntimeGovernanceResolutionDisposition](#schema-runtimegovernanceresolutiondisposition)；[ConvergenceState（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L14)；[RuntimeTerminalDecision（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1713)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L125)
- 空值/identity/版本/范围原式：`r.WorkspaceID == "" || r.RunID == "" || r.ResolutionID == ""`；`r.StateRevision == 0 || r.AuthoritySnapshotID == "" || r.State == ""`；`r.Terminal == nil`；`r.Terminal != nil`
- 显式分支集合（只表列源码case，default语义见validator）：`RuntimeGovernanceResolutionDispositionCommitted, RuntimeGovernanceResolutionDispositionStale, RuntimeGovernanceResolutionDispositionDuplicate`；`runtimecontract.ConvergenceStateComplete, runtimecontract.ConvergenceStateFailed`；`runtimecontract.ConvergenceStateNeedPrerequisite, runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation, runtimecontract.ConvergenceStateNeedAssessment, runtimecontract.ConvergenceStateNeedGovernance`

<a id="schema-commitruntimegovernanceticketresolutioncommand"></a>

### CommitRuntimeGovernanceTicketResolutionCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:91](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L91)
- 字段：`ResolutionID common.ResolutionID`；`TicketID common.TicketID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Outcome runtimecontract.RuntimeGovernanceResolutionOutcome`；`Summary string`；`EvidenceRefs []string`
- 字段引用类型：[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeGovernanceResolutionOutcome（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_governance.go#L14)

<a id="schema-commitruntimeoperationreceiptcommand"></a>

### CommitRuntimeOperationReceiptCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:70](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L70)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DispatchID string`；`Fence uint64`；`Receipt runtimecontract.RuntimeOperationReceipt`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeOperationReceipt（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_receipt.go#L139)

<a id="schema-commitruntimeoperationreceiptresult"></a>

### CommitRuntimeOperationReceiptResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:78](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L78)
- 字段：`Receipt runtimecontract.RuntimeOperationReceipt`；`StateRevision uint64`
- 字段引用类型：[RuntimeOperationReceipt（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_receipt.go#L139)
- 实际方法：[ValidateCommitRuntimeOperationReceiptResult](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/owner_receipt.go#L40)
- 空值/identity/版本/范围原式：`err := expected.Validate(); err != nil`；`err := receipt.Validate(); err != nil`；`!runtimecontract.SameRuntimeOperationReceipt(receipt, expected)`

<a id="schema-confirmworkspaceruntimequiescencecommand"></a>

### ConfirmWorkspaceRuntimeQuiescenceCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go:36](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L36)
- 字段：`Intent deletioncontract.WorkspaceRuntimeQuiescenceIntent`
- 字段引用类型：[WorkspaceRuntimeQuiescenceIntent（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/runtime_quiescence.go#L127)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L40)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L47)
- 空值/identity/版本/范围原式：`err := r.Token.ValidateFor(command.Intent); err != nil || r.OrchestratorQuiescenceRevision != 2`；`err != nil`

<a id="schema-confirmworkspaceruntimequiescenceresult"></a>

### ConfirmWorkspaceRuntimeQuiescenceResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go:42](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L42)
- 字段：`Token deletioncontract.WorkspaceRuntimeQuiescenceToken`；`OrchestratorQuiescenceRevision uint64`
- 字段引用类型：[WorkspaceRuntimeQuiescenceToken（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/runtime_quiescence.go#L65)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L47)
- 空值/identity/版本/范围原式：`err := r.Token.ValidateFor(command.Intent); err != nil || r.OrchestratorQuiescenceRevision != 2`；`err != nil`

<a id="schema-driveruntimecommand"></a>

### DriveRuntimeCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:19](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L19)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`RecoveryLeaseGeneration uint64`；`ReconciliationRequestID string`；`ReconciliationGeneration uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-driveruntimeresult"></a>

### DriveRuntimeResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:34](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L34)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StateRevision uint64`；`EvidenceRevision string`；`DefinitionRevision string`；`AuthoritySnapshotID string`；`DispatchID string`；`State runtimecontract.ConvergenceState`；`BlockingTicketID common.TicketID`；`BlockingApprovalID common.ApprovalID`；`RecoverableWait *runtimecontract.RuntimeRecoverableWaitView`；`Terminal *runtimecontract.RuntimeTerminalDecision`；`AdministrativeTermination *runtimecontract.RuntimeTerminationRecord`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ConvergenceState（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L14)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[RuntimeRecoverableWaitView（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_recoverable_wait.go#L263)；[RuntimeTerminalDecision（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1713)；[RuntimeTerminationRecord（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_repair_directive.go#L181)

<a id="schema-governanceevidenceprincipal"></a>

### GovernanceEvidencePrincipal

- 源码：[internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go:14](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go#L14)
- 字段：`Kind string`；`SubjectID string`
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go#L19)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go#L25)
- 空值/identity/版本/范围原式：`p.SubjectID == ""`
- 显式分支集合（只表列源码case，default语义见validator）：`"human_operator", "auto_governance"`

<a id="schema-listartifactcontinuationsbyruncommand"></a>

### ListArtifactContinuationsByRunCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:167](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L167)
- 实际alias/同形类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)
- 字段引用类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)

<a id="schema-listcoverageworkitemsbyruncommand"></a>

### ListCoverageWorkItemsByRunCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:166](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L166)
- 实际alias/同形类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)
- 字段引用类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)

<a id="schema-listpatchesbyruncommand"></a>

### ListPatchesByRunCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:164](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L164)
- 实际alias/同形类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)
- 字段引用类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)

<a id="schema-listrunevidencecommand"></a>

### ListRunEvidenceCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:158](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L158)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-listtoolcallsbyruncommand"></a>

### ListToolCallsByRunCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:163](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L163)
- 实际alias/同形类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)
- 字段引用类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)

<a id="schema-listvalidationresultsbyruncommand"></a>

### ListValidationResultsByRunCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:165](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L165)
- 实际alias/同形类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)
- 字段引用类型：[ListRunEvidenceCommand](#schema-listrunevidencecommand)

<a id="schema-orchestratorcommandresult"></a>

### OrchestratorCommandResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:310](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L310)
- 字段：`ToolCalls []*runtimeevidencemodel.ToolCall`；`Patches []*runtimeevidencemodel.Patch`；`ValidationResults []*runtimeevidencemodel.ValidationResult`；`CoverageWorkItems []*runtimecontract.CoverageWorkItem`；`ArtifactContinuations []*ArtifactContinuation`
- 字段引用类型：[ToolCall](#schema-toolcall)；[Patch](#schema-patch)；[ValidationResult](#schema-validationresult)；[CoverageWorkItem（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/coverage_work_item.go#L10)；[ArtifactContinuation](#schema-artifactcontinuation)

<a id="schema-patch"></a>

### Patch

- 源码：[internal/modules/kernel/orchestrator/pkg/models/evidence.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L31)
- 字段：`ID common.PatchID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`ToolCallID common.ToolCallID`；`ApprovalID common.ApprovalID`；`PatchType string`；`TargetRef string`；`Content string`；`Diff string`；`Summary string`；`CoverageTargets []PatchCoverageTarget`；`State common.PatchState`；`BeforeHash string`；`AfterHash string`；`AffectedFiles []string`；`AddedLines int`；`DeletedLines int`；`RiskLevel string`；`RiskReasons []string`；`Diagnostics []workorcheventpayload.Payload`；`OriginalContent string`；`OriginalExists bool`；`ReverseDiff string`；`RevertApprovalID common.ApprovalID`；`RevertedAt string`；`CreatedAt string`；`AppliedAt string`
- 字段引用类型：[PatchID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L13)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[ToolCallID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L12)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[PatchCoverageTarget](#schema-patchcoveragetarget)；[PatchState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L15)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L86)；[ClonePatches](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L127)

<a id="schema-patchcoveragetarget"></a>

### PatchCoverageTarget

- 源码：[internal/modules/kernel/orchestrator/pkg/models/evidence.go:26](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L26)
- 字段：`GoalRequirementID string`；`TargetText string`

<a id="schema-purgeworkspaceorchestratorevidencecommand"></a>

### PurgeWorkspaceOrchestratorEvidenceCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go:94](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L94)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L102)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L109)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L168)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableOrchestratorDeletionFenceID(scope) || r.PurgeID != StableOrchestratorDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash == "" || r.AckHash != StableOrchestratorDeletionPurgeAckHash(scope, r.PurgeProofHash)`

<a id="schema-purgeworkspaceorchestratorevidenceresult"></a>

### PurgeWorkspaceOrchestratorEvidenceResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go:111](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L111)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`PurgeID string`；`Revision uint64`；`AckHash string`；`PurgeProofHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L168)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableOrchestratorDeletionFenceID(scope) || r.PurgeID != StableOrchestratorDeletionPurgeID(scope) || r.Revision != 2 || r.PurgeProofHash == "" || r.AckHash != StableOrchestratorDeletionPurgeAckHash(scope, r.PurgeProofHash)`

<a id="schema-queryruntimeoperationreceiptcommand"></a>

### QueryRuntimeOperationReceiptCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:53](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L53)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DispatchID string`；`OperationID string`；`ExecutionKey string`；`Fence uint64`；`GovernanceGrant *runtimecontract.RuntimeOperationGovernanceGrant`；`ReadSource *runtimecontract.RuntimeOperationExecutionContext`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeOperationGovernanceGrant（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_governance.go#L249)；[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)

<a id="schema-queryruntimeoperationreceiptresult"></a>

### QueryRuntimeOperationReceiptResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:65](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L65)
- 字段：`Found bool`；`Receipt *runtimecontract.RuntimeOperationReceipt`
- 字段引用类型：[RuntimeOperationReceipt（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_receipt.go#L139)
- 实际方法：[ValidateQueryRuntimeOperationReceiptResult](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/owner_receipt.go#L15)
- 空值/identity/版本/范围原式：`executionKey == "" || fence == 0 || operationID == ""`；`result.Found != (result.Receipt != nil)`；`!result.Found`；`err := receipt.Validate(); err != nil`；`receipt.ExecutionKey != executionKey || receipt.Fence != fence || receipt.OperationID != operationID`

<a id="schema-recordapprovalevidencecommand"></a>

### RecordApprovalEvidenceCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go:60](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go#L60)
- 字段：`EvidenceRecordID common.EvidenceRecordID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`ApprovalID common.ApprovalID`；`ApprovalState common.ApprovalState`；`ResolutionID common.ResolutionID`；`Principal GovernanceEvidencePrincipal`；`Reason string`；`Action string`；`Context []workorcheventpayload.Field`；`ResolutionContext []workorcheventpayload.Field`；`Timestamp string`
- 字段引用类型：[EvidenceRecordID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L19)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[ApprovalState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L13)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[GovernanceEvidencePrincipal](#schema-governanceevidenceprincipal)；[Field（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L27)

<a id="schema-recordapprovalevidenceresult"></a>

### RecordApprovalEvidenceResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go:76](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go#L76)
- 字段：`EvidenceRecordID common.EvidenceRecordID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`ApprovalID common.ApprovalID`；`ResolutionID common.ResolutionID`；`ApprovalState common.ApprovalState`
- 字段引用类型：[EvidenceRecordID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L19)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[ApprovalState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L13)

<a id="schema-recorddecisionevidencecommand"></a>

### RecordDecisionEvidenceCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go:38](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go#L38)
- 字段：`EvidenceRecordID common.EvidenceRecordID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`TicketID common.TicketID`；`ResolutionID common.ResolutionID`；`ResolutionType string`；`DecisionKind string`；`Reason string`；`Question string`；`ResolutionPayload []workorcheventpayload.Field`；`ContinuationAction string`；`ResumeRunID common.RunID`；`SuccessorRunID common.RunID`；`Timestamp string`
- 字段引用类型：[EvidenceRecordID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L19)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)；[Field（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L27)

<a id="schema-recorddecisionevidenceresult"></a>

### RecordDecisionEvidenceResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go:56](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/governance_evidence_messages.go#L56)
- 字段：`EvidenceRecordID common.EvidenceRecordID`
- 字段引用类型：[EvidenceRecordID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L19)

<a id="schema-requestruntimeterminationcommand"></a>

### RequestRuntimeTerminationCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/lifecycle_messages.go:52](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/lifecycle_messages.go#L52)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Kind runtimecontract.RuntimeTerminationKind`；`Reason string`；`RequestedBy string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeTerminationKind（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_repair_directive.go#L168)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/lifecycle_messages.go#L70)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.RunID == "" || !runtimecontract.ValidRuntimeTerminationKind(c.Kind) || strings.TrimSpace(c.Reason) == "" || strings.TrimSpace(c.RequestedBy) == ""`

<a id="schema-requestruntimeterminationresult"></a>

### RequestRuntimeTerminationResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/lifecycle_messages.go:64](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/lifecycle_messages.go#L64)
- 字段：`TerminalDeliveryID string`；`WorkspaceReleased bool`；`StateRevision uint64`

<a id="schema-runtimedriveobservationkind"></a>

### RuntimeDriveObservationKind

- 源码：[internal/modules/kernel/orchestrator/pkg/events/runtime_drive_observation.go:14](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/runtime_drive_observation.go#L14)
- 声明：`type RuntimeDriveObservationKind string`
- 源码枚举值：`RuntimeDriveObservationScheduled = "drive_scheduled"`；`RuntimeDriveObservationRunning = "drive_running"`；`RuntimeDriveObservationRetryTimerScheduled = "retry_timer_scheduled"`；`RuntimeDriveObservationRetryTimerFired = "retry_timer_fired"`；`RuntimeDriveObservationCASReload = "cas_reload"`；`RuntimeDriveObservationSemanticRejection = "semantic_rejection"`；`RuntimeDriveObservationDeliveryPending = "delivery_pending"`；`RuntimeDriveObservationStateCommitted = "state_committed"`
- 实际方法：[Valid](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/runtime_drive_observation.go#L27)
- 显式分支集合（只表列源码case，default语义见validator）：`RuntimeDriveObservationScheduled, RuntimeDriveObservationRunning, RuntimeDriveObservationRetryTimerScheduled, RuntimeDriveObservationRetryTimerFired, RuntimeDriveObservationCASReload, RuntimeDriveObservationSemanticRejection, RuntimeDriveObservationDeliveryPending`；`RuntimeDriveObservationStateCommitted`

<a id="schema-runtimedriveobserveddata"></a>

### RuntimeDriveObservedData

- 源码：[internal/modules/kernel/orchestrator/pkg/events/runtime_drive_observation.go:49](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/runtime_drive_observation.go#L49)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Kind RuntimeDriveObservationKind`；`AggregateRevision uint64`；`RecoveryGeneration uint64`；`FailureCategory string`；`LeaseState workorchruntimecontract.RuntimeDriveLeaseState`；`Source string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeDriveObservationKind](#schema-runtimedriveobservationkind)；[RuntimeDriveLeaseState（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_drive_lease.go#L13)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/runtime_drive_observation.go#L60)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/runtime_drive_observation.go#L68)
- 空值/identity/版本/范围原式：`d.WorkspaceID == "" || d.RunID == "" || !d.Kind.Valid() || d.Source == ""`

<a id="schema-runtimegovernanceresolutiondisposition"></a>

### RuntimeGovernanceResolutionDisposition

- 源码：[internal/modules/kernel/orchestrator/pkg/events/messages.go:83](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go#L83)
- 声明：`type RuntimeGovernanceResolutionDisposition string`
- 源码枚举值：`RuntimeGovernanceResolutionDispositionCommitted = "committed"`；`RuntimeGovernanceResolutionDispositionStale = "stale_diagnostic"`；`RuntimeGovernanceResolutionDispositionDuplicate = "duplicate"`

<a id="schema-sealworkspaceorchestratordeletioncommand"></a>

### SealWorkspaceOrchestratorDeletionCommand

- 源码：[internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go:64](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L64)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`DataDispositionIntent deletioncontract.Mode`；`QuiescenceID string`；`RequestHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)
- 实际方法：[Scope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L73)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L80)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L156)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableOrchestratorDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableOrchestratorDeletionSealAckHash(scope)`

<a id="schema-sealworkspaceorchestratordeletionresult"></a>

### SealWorkspaceOrchestratorDeletionResult

- 源码：[internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go:82](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L82)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`DeletionGeneration uint64`；`FenceID string`；`Revision uint64`；`AckHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/workspace_retirement.go#L156)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != scope.WorkspaceID || r.DeletionID != scope.DeletionID || r.DeletionGeneration != scope.DeletionGeneration || r.FenceID != StableOrchestratorDeletionFenceID(scope) || r.Revision != 1 || r.AckHash != StableOrchestratorDeletionSealAckHash(scope)`

<a id="schema-toolcall"></a>

### ToolCall

- 源码：[internal/modules/kernel/orchestrator/pkg/models/evidence.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L11)
- 字段：`ID common.ToolCallID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`ToolName string`；`Arguments workorcheventpayload.Payload`；`State common.ToolCallState`；`StartedAt string`；`EndedAt string`；`ExitSummary string`；`Output workorcheventpayload.Payload`；`Error string`
- 字段引用类型：[ToolCallID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L12)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[ToolCallState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L14)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L80)；[CloneToolCalls](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L113)

<a id="schema-validationresult"></a>

### ValidationResult

- 源码：[internal/modules/kernel/orchestrator/pkg/models/evidence.go:62](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L62)
- 字段：`ID common.ValidationResultID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`ValidationKind string`；`TargetRef string`；`State common.ValidationResultState`；`Summary string`；`EvidenceRef string`；`EvidenceQuality common.EvidenceQuality`；`EvidenceReason string`；`ZeroWork bool`；`EvidenceTags []string`；`StartedAt string`；`EndedAt string`
- 字段引用类型：[ValidationResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L14)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[ValidationResultState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L16)；[EvidenceQuality（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L17)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L106)；[CloneValidationResults](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/models/evidence.go#L141)

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [handleWorkspaceRuntimeQuiescence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L20)：`event.Source() != workspaceRetirementOrchestratorCommandSource || m == nil`
  - 当前来源常量：`workspaceRetirementOrchestratorCommandSource = "blocks/workspace"`
- [executeWorkspaceDeletionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_deletion.go#L12)：`event == nil || event.Source() != workspaceRetirementOrchestratorCommandSource || m == nil || m.workspaceDeletions == nil`
  - 当前来源常量：`workspaceRetirementOrchestratorCommandSource = "blocks/workspace"`
- [handleWorkspaceDeletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/workspace_retirement.go#L57)：`event.Source() != workspaceRetirementOrchestratorCommandSource || m == nil`
  - 当前来源常量：`workspaceRetirementOrchestratorCommandSource = "blocks/workspace"`
- [handleApprovalEvidenceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/governance_evidence_command.go#L105)：`m == nil || ev == nil || ev.Source() != "blocks/approval"`
- [handleDecisionEvidenceCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/governance_evidence_command.go#L19)：`m == nil || ev == nil || ev.Source() != "blocks/decision"`
- [handleRuntimeGovernanceApprovalResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L24)：`m == nil || event == nil || event.Source() != approvalcommon.ModuleID`
  - 当前来源常量：`approvalcommon.ModuleID = "blocks/approval"`
- [handleRuntimeGovernanceTicketResolution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L55)：`m == nil || event == nil || event.Source() != decisioncommon.ModuleID`
  - 当前来源常量：`decisioncommon.ModuleID = "blocks/decision"`
- [handleRuntimeGovernanceTicketResolutionLocked](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_governance_event_adapter.go#L65)：来源集合由switch/返回表达式判定，见完整函数
- [handleRuntimeOperationReceipt](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_receipt_event_adapter.go#L25)：`source.WorkspaceID != command.WorkspaceID || source.RunID != command.RunID || source.DispatchID != command.DispatchID || source.OperationID != command.OperationID || source.Execution.ExecutionKey != command.ExecutionKey || source.Execution.Fence != command.Fence || !reflect.DeepEqual(source.Execution.GovernanceGrant, command.GovernanceGrant)`
- [handleRuntimeDrive](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_drive.go#L172)：`!ok || m == nil || event.Source() != runtimeInternalDriveSource`
  - 当前来源常量：`runtimeInternalDriveSource = "kernel/orchestrator"`
- [handleRuntimeTerminationRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L341)：`!ok || ev.Source() != "blocks/run" || m == nil || cmd.Validate() != nil || cmd.RequestedBy != "blocks/run"`