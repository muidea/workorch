# shell：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页8个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Shell执行和验证必须区分：命令成功、exit=0、真实测试数、验证目标/receipt binding是不同事实。sealed operation绑定cwd、environment、command/recipe、limits、target及治理grant，owner只能报告执行/验证事实，不能设置CWI或terminal。

Feasibility、semantic-proof feasibility、prospective-satisfaction分别是只读适用性/未来目标可验证性，不能替代真实execution与fresh proof。超时必须观察实际子进程退出；kill请求不等于取消完成。截断输出保留截断标记，0 tests exit0不准当required check通过。ImplementationWork仍closed，不能把工作伪装成validation/acquisition/repair开放。

## 逐topic合同

<a id="ct-shell-acquireruntimecapabilities"></a>

### CT-shell-AcquireRuntimeCapabilities

- 边界：`TopicAcquireRuntimeCapabilities` = `/shell/runtime-prerequisite/acquire-capabilities`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/topics.go#L4)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L43)
- 接收provider：shell；Command [AcquireRuntimeCapabilitiesCommand](#schema-acquireruntimecapabilitiescommand) → Result [AcquireRuntimeCapabilitiesResult](#schema-acquireruntimecapabilitiesresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L61) → `handleRuntimeCapabilities`（shell）
- 接收入口：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L723)
- 已观察外部引用：orchestrator [sendRuntimeCapabilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L323)
- 本topic入口/具体类型分支的Source审查索引：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS11](../Workorch_组件固定合同与分层验收规范.md#cs11)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-shell-acquireruntimeoperationadmissibility"></a>

### CT-shell-AcquireRuntimeOperationAdmissibility

- 边界：`TopicAcquireRuntimeOperationAdmissibility` = `/shell/runtime-prerequisite/acquire-operation-admissibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/topics.go#L5)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L44)
- 接收provider：shell；Command [AcquireRuntimeOperationAdmissibilityCommand](#schema-acquireruntimeoperationadmissibilitycommand) → Result [AcquireRuntimeOperationAdmissibilityResult](#schema-acquireruntimeoperationadmissibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L61) → `handleRuntimeOperationAdmissibility`（shell）
- 接收入口：[handleRuntimeOperationAdmissibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L736)
- 已观察外部引用：orchestrator [sendRuntimeOperationAdmissibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L64)
- 本topic入口/具体类型分支的Source审查索引：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS07](../Workorch_组件固定合同与分层验收规范.md#cs07)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := shellevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil`；`len(command.RequiredTargetRefs) > command.MaxItems`；`err := validateDeclaredValidationTargetScope(command.WorkspaceDeliverableDirectory, command.RequiredTargetRefs); err != nil`；`runtimeShellManagedValidationRequested(command.RequiredKinds) && len(command.ValidationRecipes) != 0`；`err := inventory.Validate(); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-shell-executeoperation"></a>

### CT-shell-ExecuteOperation

- 边界：`TopicExecuteOperation` = `/shell/runtime-operation/execute`；CF06；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L50)
- 接收provider：shell；Command [ExecuteOperationCommand](#schema-executeoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L61) → `handler`（shell）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L61) → `handleShellOperation`（shell）
- 接收入口：[handleShellOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L309)
- 已观察外部引用：orchestrator [executeShell](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L210)
- 本topic入口/具体类型分支的Source审查索引：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil`；`err := command.Execution.Validate(); err != nil`；`err := validateRuntimeShellOperationData(command.RuntimeOperationExecutionContext, data, authorization); err != nil`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`；`operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession && (command.Transaction.DefinitionRevision == "" || command.Transaction.DefinitionRevision != strings.TrimSpace(command.Transaction.DefinitionRevision))`；`observation := shellValidationFailureObservation(command.RuntimeOperationExecutionContext, authorization, diagnostic); observation != nil`；`undeclared := shellUndeclaredWorkspaceTargets(operationResult.WorkspaceObservedTargetChanges, data.TargetRefs); len(undeclared) > 0`；`status == runtimecontract.RuntimeOperationExecutionStatusAccepted && data.StateChanging`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-shell-proberuntimeoperationfeasibility"></a>

### CT-shell-ProbeRuntimeOperationFeasibility

- 边界：`TopicProbeRuntimeOperationFeasibility` = `/shell/runtime-operation/probe-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L45)
- 接收provider：shell；Command [ProbeRuntimeOperationFeasibilityCommand](#schema-proberuntimeoperationfeasibilitycommand) → Result [ProbeRuntimeOperationFeasibilityResult](#schema-proberuntimeoperationfeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L61) → `handleRuntimeOperationFeasibility`（shell）
- 接收入口：[handleRuntimeOperationFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L811)
- 已观察外部引用：orchestrator [sendRuntimeOperationFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L127)
- 本topic入口/具体类型分支的Source审查索引：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := shellevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-shell-proberuntimeownerfactprospectivesatisfaction"></a>

### CT-shell-ProbeRuntimeOwnerFactProspectiveSatisfaction

- 边界：`TopicProbeRuntimeOwnerFactProspectiveSatisfaction` = `/shell/runtime-owner-fact/probe-prospective-satisfaction`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L47)
- 接收provider：shell；Command [ProbeRuntimeOwnerFactProspectiveSatisfactionCommand](#schema-proberuntimeownerfactprospectivesatisfactioncommand) → Result [ProbeRuntimeOwnerFactProspectiveSatisfactionResult](#schema-proberuntimeownerfactprospectivesatisfactionresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L61) → `handleRuntimeOwnerFactProspectiveSatisfaction`（shell）
- 接收入口：[handleRuntimeOwnerFactProspectiveSatisfaction](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/prospective_satisfaction.go#L19)
- 已观察外部引用：orchestrator [ProbeRuntimeOwnerFactProspectiveSatisfaction](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_owner_fact_prospective_satisfaction_event_adapter.go#L17)
- 本topic入口/具体类型分支的Source审查索引：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`_, err := common.BindRuntimeWorkspaceContext(event.Context(), request.WorkspaceID); err != nil`；`err := shellevents.ValidateRuntimeOwnerFactProspectiveSatisfactionCommand(command, ModuleID, modulecommon.ProspectiveSatisfactionContractRef); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-shell-proberuntimesemanticprooffeasibility"></a>

### CT-shell-ProbeRuntimeSemanticProofFeasibility

- 边界：`TopicProbeRuntimeSemanticProofFeasibility` = `/shell/runtime-semantic/probe-proof-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L46)
- 接收provider：shell；Command [ProbeRuntimeSemanticProofFeasibilityCommand](#schema-proberuntimesemanticprooffeasibilitycommand) → Result [ProbeRuntimeSemanticProofFeasibilityResult](#schema-proberuntimesemanticprooffeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L61) → `handleRuntimeSemanticProofFeasibility`（shell）
- 接收入口：[handleRuntimeSemanticProofFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L835)
- 已观察外部引用：orchestrator [sendRuntimeSemanticProofFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_semantic_feasibility_event_adapter.go#L61)
- 本topic入口/具体类型分支的Source审查索引：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := shellevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil`；`current.ObservedRevision != command.OwnerCatalog.ObservedRevision`；`command.Proposal.ValidationRecipeID != ""`；`err := validateDeclaredValidationTargets(command.WorkspaceDeliverableDirectory, targetRefs); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-shell-queryruntimecwireconciliation"></a>

### CT-shell-QueryRuntimeCWIReconciliation

- 边界：`TopicQueryRuntimeCWIReconciliation` = `/shell/reconciliation/query-cwi`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L48)
- 接收provider：shell；Command [QueryRuntimeCWIReconciliationCommand](#schema-queryruntimecwireconciliationcommand) → Result [QueryRuntimeCWIReconciliationResult](#schema-queryruntimecwireconciliationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L61) → `handleRuntimeCWIReconciliationQuery`（shell）
- 接收入口：[handleRuntimeCWIReconciliationQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L141)
- 已观察外部引用：orchestrator [sendRuntimeCWIReconciliationOwnerQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_reconciliation.go#L185)
- 本topic入口/具体类型分支的Source审查索引：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS05](../Workorch_组件固定合同与分层验收规范.md#cs05)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil`；`!runtimeCWIReconciliationBindingHasShellProof(command.Binding)`；`len(recipes) != len(command.Binding.ValidationRecipeIDs)`；`len(command.Binding.RequiredCheckRefs) != 0`；`len(command.Binding.MetricConstraints) != 0 || len(command.Binding.ArtifactRefs) != 0`；`len(command.Binding.RequiredCheckRefs) == 0`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-shell-queryruntimevalidationrawoutput"></a>

### CT-shell-QueryRuntimeValidationRawOutput

- 边界：`TopicQueryRuntimeValidationRawOutput` = `/shell/validation/query-raw-output`；CF04；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L49)
- 接收provider：shell；Command [QueryRuntimeValidationRawOutputCommand](#schema-queryruntimevalidationrawoutputcommand) → Result [QueryRuntimeValidationRawOutputResult](#schema-queryruntimevalidationrawoutputresult)；target合法caller：kernel/orchestrator
- 已读取CI接线失败：[run38215841512/job114700248734](https://github.com/muidea/workorch/actions/runs/38215841512/job/114700248734)报告sender=false、receiver=true；此项不是本轮执行，也不由静态引用替代生产sender
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L61) → `handleRuntimeValidationRawOutput`（shell）
- 接收入口：[handleRuntimeValidationRawOutput](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/validation_raw_output_query.go#L17)
- usage：未在本次已读调用端找到外部引用；不是已证明unused。target仅允许上列caller/订阅者；需双端VS补证
- 本topic入口/具体类型分支的Source审查索引：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：source新增查询：只选择原receipt的StageID与stdout/stderr流，目录/ref/保存身份由owner反查；Workspace/Run/Dispatch/Operation/ExecutionKey/Fence/SourceReceiptHash匹配，Raw≤64KiB且SavedBytes/SavedSHA256一致，完整保存时原SHA256也一致。已有最新CI明确sender=false、receiver=true，当前未实现生产调用接线；目标需接入真实caller并双端验证，不能算SG2完成。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || m.runtimeAdapter == nil || command.Validate() != nil`；`!found || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != command.SourceReceiptHash || receipt.Evidence.Validation == nil || (receipt.ObservedEffectScope != runtimecontract.RuntimeEffectEphemeralSession && !(receipt.Outcome == runtimecontract.RuntimeOperationOutcomeRejected && receipt.ObservedEffectScope == runtimecontract.RuntimeEffectUnknown)) || (receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted && receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected) || receipt.Evidence.Validation.ObservationProtocolRef != shellProjectConjunctionProtocolRef || receipt.Evidence.Validation.ExecutionConjunction == nil`；`stage.Identity.ID == command.StageID`；`command.Stream == "stderr"`；`!ok || workspace.ValidateFor(workspaceCommand) != nil`；`err := output.ValidateFor(command); err != nil`
- 验收适用集：[CP14](../Workorch_组件固定合同与分层验收规范.md#cp14)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-acquireruntimecapabilitiescommand"></a>

### AcquireRuntimeCapabilitiesCommand

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_capability.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_capability.go#L5)
- 声明：`type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest`
- 字段引用类型：[RuntimeCapabilityAcquisitionRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L15)

<a id="schema-acquireruntimecapabilitiesresult"></a>

### AcquireRuntimeCapabilitiesResult

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_capability.go:6](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_capability.go#L6)
- 字段：`Acquisition runtimecontract.RuntimeCapabilityAcquisition`
- 字段引用类型：[RuntimeCapabilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L27)

<a id="schema-acquireruntimeoperationadmissibilitycommand"></a>

### AcquireRuntimeOperationAdmissibilityCommand

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_admissibility.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_admissibility.go#L5)
- 声明：`type AcquireRuntimeOperationAdmissibilityCommand runtimecontract.RuntimeOperationAdmissibilityRequest`
- 字段引用类型：[RuntimeOperationAdmissibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L14)
- 实际方法：[ValidateRuntimeOperationAdmissibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_admissibility.go#L43)

<a id="schema-acquireruntimeoperationadmissibilityresult"></a>

### AcquireRuntimeOperationAdmissibilityResult

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_admissibility.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_admissibility.go#L7)
- 字段：`Acquisition runtimecontract.RuntimeOperationAdmissibilityAcquisition`
- 字段引用类型：[RuntimeOperationAdmissibilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L83)

<a id="schema-executeoperationcommand"></a>

### ExecuteOperationCommand

- 源码：[internal/modules/blocks/shell/pkg/events/messages.go:29](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L29)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)

<a id="schema-executeoperationresult"></a>

### ExecuteOperationResult

- 源码：[internal/modules/blocks/shell/pkg/events/messages.go:37](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/messages.go#L37)
- 字段：`Receipt runtimecontract.RuntimeOperationReceipt`
- 字段引用类型：[RuntimeOperationReceipt（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_receipt.go#L139)

<a id="schema-proberuntimeoperationfeasibilitycommand"></a>

### ProbeRuntimeOperationFeasibilityCommand

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_admissibility.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_admissibility.go#L11)
- 声明：`type ProbeRuntimeOperationFeasibilityCommand runtimecontract.RuntimeOperationFeasibilityRequest`
- 字段引用类型：[RuntimeOperationFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L253)
- 实际方法：[ValidateRuntimeOperationFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_admissibility.go#L31)

<a id="schema-proberuntimeoperationfeasibilityresult"></a>

### ProbeRuntimeOperationFeasibilityResult

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_admissibility.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_admissibility.go#L13)
- 声明：`type ProbeRuntimeOperationFeasibilityResult runtimecontract.RuntimeOperationFeasibilityObservation`
- 字段引用类型：[RuntimeOperationFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L270)

<a id="schema-proberuntimeownerfactprospectivesatisfactioncommand"></a>

### ProbeRuntimeOwnerFactProspectiveSatisfactionCommand

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_prospective_satisfaction.go:10](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_prospective_satisfaction.go#L10)
- 声明：`type ProbeRuntimeOwnerFactProspectiveSatisfactionCommand runtimecontract.RuntimeOwnerFactProspectiveSatisfactionRequest`
- 字段引用类型：[RuntimeOwnerFactProspectiveSatisfactionRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_owner_fact_prospective_satisfaction.go#L266)
- 实际方法：[ValidateRuntimeOwnerFactProspectiveSatisfactionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_prospective_satisfaction.go#L16)
- 空值/identity/版本/范围原式：`ownerModuleID == "" || contractRef == "" || request.Requirement.OwnerModuleID != ownerModuleID || request.ProspectiveSatisfactionContractRef != contractRef`

<a id="schema-proberuntimeownerfactprospectivesatisfactionresult"></a>

### ProbeRuntimeOwnerFactProspectiveSatisfactionResult

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_prospective_satisfaction.go:12](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_prospective_satisfaction.go#L12)
- 字段：`Observation runtimecontract.RuntimeOwnerFactProspectiveSatisfactionObservation`
- 字段引用类型：[RuntimeOwnerFactProspectiveSatisfactionObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_owner_fact_prospective_satisfaction.go#L410)

<a id="schema-proberuntimesemanticprooffeasibilitycommand"></a>

### ProbeRuntimeSemanticProofFeasibilityCommand

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_admissibility.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_admissibility.go#L15)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityCommand runtimecontract.RuntimeSemanticProofFeasibilityRequest`
- 字段引用类型：[RuntimeSemanticProofFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L13)
- 实际方法：[ValidateRuntimeSemanticProofFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_admissibility.go#L19)

<a id="schema-proberuntimesemanticprooffeasibilityresult"></a>

### ProbeRuntimeSemanticProofFeasibilityResult

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_admissibility.go:17](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_admissibility.go#L17)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityResult runtimecontract.RuntimeSemanticProofFeasibilityObservation`
- 字段引用类型：[RuntimeSemanticProofFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L24)

<a id="schema-queryruntimecwireconciliationcommand"></a>

### QueryRuntimeCWIReconciliationCommand

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_reconciliation.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_reconciliation.go#L5)
- 声明：`type QueryRuntimeCWIReconciliationCommand runtimecontract.RuntimeCWIOwnerReconciliationRequest`
- 字段引用类型：[RuntimeCWIOwnerReconciliationRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L371)
- 实际方法：[ValidateForOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_reconciliation.go#L11)

<a id="schema-queryruntimecwireconciliationresult"></a>

### QueryRuntimeCWIReconciliationResult

- 源码：[internal/modules/blocks/shell/pkg/events/runtime_reconciliation.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/runtime_reconciliation.go#L7)
- 字段：`Observation runtimecontract.RuntimeCWIReconciliationObservation`
- 字段引用类型：[RuntimeCWIReconciliationObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L467)

<a id="schema-queryruntimevalidationrawoutputcommand"></a>

### QueryRuntimeValidationRawOutputCommand

- 源码：[internal/modules/blocks/shell/pkg/events/validation_raw_output.go:17](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/validation_raw_output.go#L17)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DispatchID string`；`OperationID string`；`ExecutionKey string`；`Fence uint64`；`SourceReceiptHash string`；`StageID string`；`Stream string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/validation_raw_output.go#L38)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/validation_raw_output.go#L54)
- 空值/identity/版本/范围原式：`value == "" || len(value) > 1024 || strings.TrimSpace(value) != value || !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) >= 0`；`c.Fence == 0 || c.Stream != "stdout" && c.Stream != "stderr"`；`err != nil || origin != c.DispatchID`；`c.Validate() != nil || r.SourceReceiptHash != c.SourceReceiptHash || r.StageID != c.StageID || r.Stream != c.Stream || r.Identity.Validate() != nil || r.Identity.Storage == nil || r.Identity.Storage.Reference == "" || len(r.Raw) > RuntimeValidationRawOutputMaxBytes || r.Identity.SavedBytes != int64(len(r.Raw))`；`r.SavedSHA256 != hex.EncodeToString(digest[:]) || r.Identity.Bytes == r.Identity.SavedBytes && r.Identity.SHA256 != r.SavedSHA256`

<a id="schema-queryruntimevalidationrawoutputresult"></a>

### QueryRuntimeValidationRawOutputResult

- 源码：[internal/modules/blocks/shell/pkg/events/validation_raw_output.go:29](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/validation_raw_output.go#L29)
- 字段：`SourceReceiptHash string`；`StageID string`；`Stream string`；`Identity runtimecontract.RuntimeValidationOutputIdentity`；`SavedSHA256 string`；`Raw []byte`
- 字段引用类型：[RuntimeValidationOutputIdentity（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_validation_execution_conjunction.go#L32)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/validation_raw_output.go#L54)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/validation_raw_output.go#L67)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.SourceReceiptHash != c.SourceReceiptHash || r.StageID != c.StageID || r.Stream != c.Stream || r.Identity.Validate() != nil || r.Identity.Storage == nil || r.Identity.Storage.Reference == "" || len(r.Raw) > RuntimeValidationRawOutputMaxBytes || r.Identity.SavedBytes != int64(len(r.Raw))`；`r.SavedSHA256 != hex.EncodeToString(digest[:]) || r.Identity.Bytes == r.Identity.SavedBytes && r.Identity.SHA256 != r.SavedSHA256`

<a id="schema-runtimeexecutionauthorization"></a>

### RuntimeExecutionAuthorization

- 源码：[internal/modules/blocks/shell/pkg/events/operation.go:34](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/operation.go#L34)
- 字段：`CatalogSnapshotID string`；`AcquisitionBinding string`；`PrerequisiteID string`；`OwnerRevision string`；`CatalogObservation runtimecontract.RuntimePrerequisiteObservation`；`Policy runtimecontract.RuntimeOperationPolicyFact`；`TargetRole string`；`ValidationRecipes []runtimecontract.RuntimeValidationRecipe`；`ValidationScopeTargetRefs []string`；`EvidencePolicies []runtimecontract.RuntimeEvidencePolicy`；`ValidationProofBinding runtimecontract.RuntimeValidationProofRequirementBinding`
- 字段引用类型：[RuntimePrerequisiteObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_prerequisite_freshness.go#L101)；[RuntimeOperationPolicyFact（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_policy.go#L13)；[RuntimeValidationRecipe（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_evidence_policy.go#L237)；[RuntimeEvidencePolicy（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_evidence_policy.go#L10)；[RuntimeValidationProofRequirementBinding（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_validation_surface.go#L316)
- 实际方法：[normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/operation.go#L48)；[validateForOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/operation.go#L63)；[ValidateForExecution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/operation.go#L205)
- 空值/identity/版本/范围原式：`err := runtimecontract.ValidateRuntimeEvidencePolicies(a.EvidencePolicies); err != nil`；`a.CatalogSnapshotID == "" || a.PrerequisiteID == "" || a.OwnerRevision == ""`；`err := a.CatalogObservation.Validate(); err != nil`；`a.CatalogObservation.PrerequisiteID != a.PrerequisiteID || a.CatalogObservation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility || a.CatalogObservation.OwnerModuleID != runtimecontract.RuntimeModuleShell || a.CatalogObservation.ContentHash != a.CatalogSnapshotID || !observationHasOwnerRevision(a.CatalogObservation, runtimecontract.RuntimeResourceOwnerCatalog, "shell_runtime_policy", a.OwnerRevision)`；`err := a.Policy.Validate(); err != nil`；`!found || matched.ID != a.Policy.ID || a.Policy.OwnerModuleID != runtimecontract.RuntimeModuleShell`；`err != nil`；`operation.Kind == runtimecontract.RuntimeBlockKindManagedValidationSession`；`_, found := runtimecontract.RuntimeValidationRecipeForID(a.ValidationRecipes, data.ValidationRecipeID); !found`；`err := a.ValidationProofBinding.Validate(); err != nil`；`a.ValidationProofBinding.ValidationRecipeID != data.ValidationRecipeID || len(operation.BindingRefs) != 1`；`len(a.ValidationScopeTargetRefs) != 0`；`!runtimeValidationProofBindingEmpty(a.ValidationProofBinding)`；`err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil`；`err := runtimecontract.ValidateRuntimeEvidencePolicies(s.Authorization.EvidencePolicies); err != nil`；`operation.Kind != runtimecontract.StepKindShellCommand && operation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession`；`err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil`；`operation.ID != strings.TrimSpace(context.OperationID)`；`!sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs)`；`err := authorization.validateForOperation(operation); err != nil`；`err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, []runtimecontract.RuntimeOperationPolicyFact{authorization.Policy}); err != nil`；`strings.TrimSpace(context.Execution.AdmissibilitySnapshotID) != authorization.CatalogSnapshotID || strings.TrimSpace(context.Execution.AcquisitionBinding) != authorization.AcquisitionBinding`；`len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1`

<a id="schema-sealedruntimeoperation"></a>

### SealedRuntimeOperation

- 源码：[internal/modules/blocks/shell/pkg/events/operation.go:113](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/operation.go#L113)
- 字段：`Operation runtimecontract.RuntimeOperation`；`Authorization RuntimeExecutionAuthorization`；`PayloadHash string`
- 字段引用类型：[RuntimeOperation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate.go#L395)；[RuntimeExecutionAuthorization](#schema-runtimeexecutionauthorization)
- 实际方法：[ValidateForExecution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/events/operation.go#L205)
- 空值/identity/版本/范围原式：`err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil`；`err := runtimecontract.ValidateRuntimeEvidencePolicies(s.Authorization.EvidencePolicies); err != nil`；`operation.Kind != runtimecontract.StepKindShellCommand && operation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession`；`err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil`；`operation.ID != strings.TrimSpace(context.OperationID)`；`!sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs)`；`err := authorization.validateForOperation(operation); err != nil`；`err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, []runtimecontract.RuntimeOperationPolicyFact{authorization.Policy}); err != nil`；`strings.TrimSpace(context.Execution.AdmissibilitySnapshotID) != authorization.CatalogSnapshotID || strings.TrimSpace(context.Execution.AcquisitionBinding) != authorization.AcquisitionBinding`；`err != nil`；`len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [shellRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L899)：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`
  - helper完整分支：`func shellRuntimeIngressCommand[T any](event magicevent.Event, result magicevent.Result) (T, bool) { var zero T if result == nil { return zero, false } if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator { setShellRuntimeIngressInvalid(result, "runtime Shell ingress requires exact Orchestrator source") return zero, false } command, ok := event.Data().(T) if !ok { setShellRuntimeIngressInvalid(result, "runtime Shell ingress payload has an invalid concrete type") return zero, false } return command, true }`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`