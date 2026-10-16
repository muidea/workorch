# git：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页7个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Git拥有repository/worktree状态与Git效果。AcquireCapabilityState不是纯零写查询：以AcquisitionKey查/存owner capability-state receipt，绑定File owner的WorkspaceTreeRevision；同key遇不同文件树revision返回conflict，不能重写旧快照。其余observation不授coverage credit。

QueryRuntimeCWIReconciliation只能证明实际repo存在/revision；无Git proof、缺仓库或绑定要求Git无法独立验证的check/metric/artifact，分别返回unverifiable/unsatisfied，不能给satisfied假绿。Execute的kind、工作树、base revision、pathspec、remote/ref与grant受seal约束；commit/push的权限不互相推出。远端unknown不盲重做。

## 逐topic合同

<a id="ct-git-acquirecapabilitystate"></a>

### CT-git-AcquireCapabilityState

- 边界：`TopicAcquireCapabilityState` = `/git/runtime-prerequisite/acquire-capability-state`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L60)
- 接收provider：git；Command [AcquireCapabilityStateCommand](#schema-acquirecapabilitystatecommand) → Result [AcquireCapabilityStateResult](#schema-acquirecapabilitystateresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L81) → `handleGitCapabilityState`（git）
- 接收入口：[handleGitCapabilityState](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L236)
- 已观察外部引用：orchestrator [AcquireGitCapabilityStatePrerequisite](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L382)
- 本topic入口/具体类型分支的Source审查索引：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：AcquisitionKey非空且幂等；先取得File WorkspaceTreeRevision，命中receipt必须同revision，缺receipt才StoreIfAbsent。返回QueryReceiptRef/AcquisitionBinding不是新execution授权。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || m.runtimeAdapter == nil || m.capabilityReceipts == nil || command.WorkspaceID == "" || command.PrerequisiteID == "" || command.AcquisitionKey == "" || command.WorkspaceDeliverableDirectory == "" || command.CapabilityStateKind != "repository_state" || command.SubjectRef == ""`；`snapshot, found, err := m.capabilityReceipts.Load(command.WorkspaceID, command.AcquisitionKey); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-git-acquireruntimecapabilities"></a>

### CT-git-AcquireRuntimeCapabilities

- 边界：`TopicAcquireRuntimeCapabilities` = `/git/runtime-prerequisite/acquire-capabilities`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/topics.go#L4)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L55)
- 接收provider：git；Command [AcquireRuntimeCapabilitiesCommand](#schema-acquireruntimecapabilitiescommand) → Result [AcquireRuntimeCapabilitiesResult](#schema-acquireruntimecapabilitiesresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L81) → `handleRuntimeCapabilities`（git）
- 接收入口：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L986)
- 已观察外部引用：orchestrator [sendRuntimeCapabilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L323)
- 本topic入口/具体类型分支的Source审查索引：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS11](../Workorch_组件固定合同与分层验收规范.md#cs11)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-git-acquireruntimeoperationadmissibility"></a>

### CT-git-AcquireRuntimeOperationAdmissibility

- 边界：`TopicAcquireRuntimeOperationAdmissibility` = `/git/runtime-prerequisite/acquire-operation-admissibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/topics.go#L5)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L56)
- 接收provider：git；Command [AcquireRuntimeOperationAdmissibilityCommand](#schema-acquireruntimeoperationadmissibilitycommand) → Result [AcquireRuntimeOperationAdmissibilityResult](#schema-acquireruntimeoperationadmissibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L81) → `handleRuntimeOperationAdmissibility`（git）
- 接收入口：[handleRuntimeOperationAdmissibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L338)
- 已观察外部引用：orchestrator [sendRuntimeOperationAdmissibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L64)
- 本topic入口/具体类型分支的Source审查索引：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS07](../Workorch_组件固定合同与分层验收规范.md#cs07)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := gitevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil`；`err := manifestBinding.Validate(); err != nil`；`err := inventory.Validate(); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-git-executeoperation"></a>

### CT-git-ExecuteOperation

- 边界：`TopicExecuteOperation` = `/git/runtime-operation/execute`；CF06；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L61)
- 接收provider：git；Command [ExecuteOperationCommand](#schema-executeoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L81) → `handler`（git）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L81) → `handleGitOperation`（git）
- 接收入口：[handleGitOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L677)
- 已观察外部引用：orchestrator [executeGit](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L181)
- 本topic入口/具体类型分支的Source审查索引：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := validateGitOperationTargetBinding(operation, data); err != nil`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`；`stale, staleErr := m.runtimePrerequisiteStaleReceipt(event.Context(), command, operation.RequiredOperationRef); staleErr != nil`；`data.Operation == modulecommon.OperationCommit`；`err := validateGitCommitDependencyScope(command.RuntimeOperationExecutionContext); err != nil`；`after, afterErr := m.runtimeAdapter.CapabilityState(event.Context(), "runtime-receipt/"+command.OperationID, "workspace_deliverable", command.WorkspaceDeliverableDirectory); afterErr == nil`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-git-proberuntimeoperationfeasibility"></a>

### CT-git-ProbeRuntimeOperationFeasibility

- 边界：`TopicProbeRuntimeOperationFeasibility` = `/git/runtime-operation/probe-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L57)
- 接收provider：git；Command [ProbeRuntimeOperationFeasibilityCommand](#schema-proberuntimeoperationfeasibilitycommand) → Result [ProbeRuntimeOperationFeasibilityResult](#schema-proberuntimeoperationfeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L81) → `handleRuntimeOperationFeasibility`（git）
- 接收入口：[handleRuntimeOperationFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L453)
- 已观察外部引用：orchestrator [sendRuntimeOperationFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L127)
- 本topic入口/具体类型分支的Source审查索引：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := gitevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil`；`stale == nil && failure != nil && failure.Code == "operation_admissibility_stale" && command.Observation != nil`；`observed, err := m.runtimeAdapter.CapabilityState(event.Context(), command.Admissibility.PrerequisiteID, "workspace_deliverable", command.WorkspaceDeliverableDirectory); err == nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-git-proberuntimesemanticprooffeasibility"></a>

### CT-git-ProbeRuntimeSemanticProofFeasibility

- 边界：`TopicProbeRuntimeSemanticProofFeasibility` = `/git/runtime-semantic/probe-proof-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L58)
- 接收provider：git；Command [ProbeRuntimeSemanticProofFeasibilityCommand](#schema-proberuntimesemanticprooffeasibilitycommand) → Result [ProbeRuntimeSemanticProofFeasibilityResult](#schema-proberuntimesemanticprooffeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L81) → `handleRuntimeSemanticProofFeasibility`（git）
- 接收入口：[handleRuntimeSemanticProofFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L551)
- 已观察外部引用：orchestrator [sendRuntimeSemanticProofFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_semantic_feasibility_event_adapter.go#L61)
- 本topic入口/具体类型分支的Source审查索引：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := gitevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil`；`snapshot.ObservedRevision != command.OwnerCatalog.ObservedRevision`；`strings.TrimSpace(command.Proposal.ValidationRecipeID) != "" || len(command.Proposal.RequiredCheckRefs) != 0`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-git-queryruntimecwireconciliation"></a>

### CT-git-QueryRuntimeCWIReconciliation

- 边界：`TopicQueryRuntimeCWIReconciliation` = `/git/reconciliation/query-cwi`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L59)
- 接收provider：git；Command [QueryRuntimeCWIReconciliationCommand](#schema-queryruntimecwireconciliationcommand) → Result [QueryRuntimeCWIReconciliationResult](#schema-queryruntimecwireconciliationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L81) → `handleRuntimeCWIReconciliationQuery`（git）
- 接收入口：[handleRuntimeCWIReconciliationQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L161)
- 已观察外部引用：orchestrator [sendRuntimeCWIReconciliationOwnerQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_reconciliation.go#L185)
- 本topic入口/具体类型分支的Source审查索引：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS05](../Workorch_组件固定合同与分层验收规范.md#cs05)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil`；`!runtimeCWIReconciliationBindingHasGitProof(command.Binding)`；`status == runtimecontract.RuntimeCWIReconciliationObservationSatisfied && (len(command.Binding.RequiredCheckRefs) != 0 || len(command.Binding.MetricConstraints) != 0 || len(command.Binding.ArtifactRefs) != 0)`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-acquirecapabilitystatecommand"></a>

### AcquireCapabilityStateCommand

- 源码：[internal/modules/blocks/git/pkg/events/messages.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L11)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`PrerequisiteID string`；`WorkspaceDeliverableDirectory string`；`CapabilityStateKind string`；`SubjectRef string`；`AcquisitionKey string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-acquirecapabilitystateresult"></a>

### AcquireCapabilityStateResult

- 源码：[internal/modules/blocks/git/pkg/events/messages.go:22](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L22)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`Snapshot runtimecontract.RuntimeCapabilityStateSnapshot`；`Observation runtimecontract.RuntimePrerequisiteObservation`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeCapabilityStateSnapshot（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1391)；[RuntimePrerequisiteObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_prerequisite_freshness.go#L101)

<a id="schema-acquireruntimecapabilitiescommand"></a>

### AcquireRuntimeCapabilitiesCommand

- 源码：[internal/modules/blocks/git/pkg/events/runtime_capability.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_capability.go#L5)
- 声明：`type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest`
- 字段引用类型：[RuntimeCapabilityAcquisitionRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L15)

<a id="schema-acquireruntimecapabilitiesresult"></a>

### AcquireRuntimeCapabilitiesResult

- 源码：[internal/modules/blocks/git/pkg/events/runtime_capability.go:6](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_capability.go#L6)
- 字段：`Acquisition runtimecontract.RuntimeCapabilityAcquisition`
- 字段引用类型：[RuntimeCapabilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L27)

<a id="schema-acquireruntimeoperationadmissibilitycommand"></a>

### AcquireRuntimeOperationAdmissibilityCommand

- 源码：[internal/modules/blocks/git/pkg/events/runtime_admissibility.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_admissibility.go#L5)
- 声明：`type AcquireRuntimeOperationAdmissibilityCommand runtimecontract.RuntimeOperationAdmissibilityRequest`
- 字段引用类型：[RuntimeOperationAdmissibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L14)
- 实际方法：[ValidateRuntimeOperationAdmissibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_admissibility.go#L43)

<a id="schema-acquireruntimeoperationadmissibilityresult"></a>

### AcquireRuntimeOperationAdmissibilityResult

- 源码：[internal/modules/blocks/git/pkg/events/runtime_admissibility.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_admissibility.go#L7)
- 字段：`Acquisition runtimecontract.RuntimeOperationAdmissibilityAcquisition`
- 字段引用类型：[RuntimeOperationAdmissibilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L83)

<a id="schema-executeoperationcommand"></a>

### ExecuteOperationCommand

- 源码：[internal/modules/blocks/git/pkg/events/messages.go:41](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L41)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`；`CapabilityStates []runtimecontract.RuntimeCapabilityStateSnapshot`；`WorkspaceTreeObservation *runtimecontract.RuntimePrerequisiteObservation`；`CompletedRequiredOperationReceipts []runtimecontract.RuntimeRequiredOperationReceiptBinding`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)；[RuntimeCapabilityStateSnapshot（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1391)；[RuntimePrerequisiteObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_prerequisite_freshness.go#L101)；[RuntimeRequiredOperationReceiptBinding（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_receipt.go#L171)

<a id="schema-executeoperationresult"></a>

### ExecuteOperationResult

- 源码：[internal/modules/blocks/git/pkg/events/messages.go:49](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/messages.go#L49)
- 字段：`Receipt runtimecontract.RuntimeOperationReceipt`
- 字段引用类型：[RuntimeOperationReceipt（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_receipt.go#L139)

<a id="schema-proberuntimeoperationfeasibilitycommand"></a>

### ProbeRuntimeOperationFeasibilityCommand

- 源码：[internal/modules/blocks/git/pkg/events/runtime_admissibility.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_admissibility.go#L11)
- 声明：`type ProbeRuntimeOperationFeasibilityCommand runtimecontract.RuntimeOperationFeasibilityRequest`
- 字段引用类型：[RuntimeOperationFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L253)
- 实际方法：[ValidateRuntimeOperationFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_admissibility.go#L31)

<a id="schema-proberuntimeoperationfeasibilityresult"></a>

### ProbeRuntimeOperationFeasibilityResult

- 源码：[internal/modules/blocks/git/pkg/events/runtime_admissibility.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_admissibility.go#L13)
- 声明：`type ProbeRuntimeOperationFeasibilityResult runtimecontract.RuntimeOperationFeasibilityObservation`
- 字段引用类型：[RuntimeOperationFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L270)

<a id="schema-proberuntimesemanticprooffeasibilitycommand"></a>

### ProbeRuntimeSemanticProofFeasibilityCommand

- 源码：[internal/modules/blocks/git/pkg/events/runtime_admissibility.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_admissibility.go#L15)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityCommand runtimecontract.RuntimeSemanticProofFeasibilityRequest`
- 字段引用类型：[RuntimeSemanticProofFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L13)
- 实际方法：[ValidateRuntimeSemanticProofFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_admissibility.go#L19)

<a id="schema-proberuntimesemanticprooffeasibilityresult"></a>

### ProbeRuntimeSemanticProofFeasibilityResult

- 源码：[internal/modules/blocks/git/pkg/events/runtime_admissibility.go:17](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_admissibility.go#L17)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityResult runtimecontract.RuntimeSemanticProofFeasibilityObservation`
- 字段引用类型：[RuntimeSemanticProofFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L24)

<a id="schema-queryruntimecwireconciliationcommand"></a>

### QueryRuntimeCWIReconciliationCommand

- 源码：[internal/modules/blocks/git/pkg/events/runtime_reconciliation.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_reconciliation.go#L5)
- 声明：`type QueryRuntimeCWIReconciliationCommand runtimecontract.RuntimeCWIOwnerReconciliationRequest`
- 字段引用类型：[RuntimeCWIOwnerReconciliationRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L371)
- 实际方法：[ValidateForOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_reconciliation.go#L11)

<a id="schema-queryruntimecwireconciliationresult"></a>

### QueryRuntimeCWIReconciliationResult

- 源码：[internal/modules/blocks/git/pkg/events/runtime_reconciliation.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/runtime_reconciliation.go#L7)
- 字段：`Observation runtimecontract.RuntimeCWIReconciliationObservation`
- 字段引用类型：[RuntimeCWIReconciliationObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L467)

<a id="schema-sealedruntimeoperation"></a>

### SealedRuntimeOperation

- 源码：[internal/modules/blocks/git/pkg/events/operation.go:20](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/operation.go#L20)
- 字段：`Operation runtimecontract.RuntimeOperation`；`PayloadHash string`
- 字段引用类型：[RuntimeOperation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate.go#L395)
- 实际方法：[ValidateForExecution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/events/operation.go#L47)
- 空值/identity/版本/范围原式：`err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil`；`runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleGit`；`err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil`；`operation.ID != strings.TrimSpace(context.OperationID)`；`!sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs)`；`err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, nil); err != nil`；`err != nil`；`len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [gitRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1003)：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`
  - helper完整分支：`func gitRuntimeIngressCommand[T any](event magicevent.Event, result magicevent.Result) (T, bool) { var zero T if result == nil { return zero, false } if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator { setGitRuntimeIngressInvalid(result, "runtime Git ingress requires exact Orchestrator source") return zero, false } command, ok := event.Data().(T) if !ok { setGitRuntimeIngressInvalid(result, "runtime Git ingress payload has an invalid concrete type") return zero, false } return command, true }`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`