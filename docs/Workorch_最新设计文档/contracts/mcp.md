# mcp：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页6个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

MCP是远端能力执行owner，MCPRegistry是注册事实owner；registry可发现不等于执行授权。OperationData有ServerID/Tool/URI/Prompt、有限OperationParameter列表、Summary/Stream；不得扩成map[string]any。sealed operation与target role、server/capability/schema快照一致后才能调用。

远端server未声明幂等或receipt查询，timeout-after-effect保持unknown。CWI reconciliation仅报告该owner可独立验证的绑定证据；unsupported/unverifiable不能折算credit。stdio/HTTP session清理与订阅排空属CF00。

## 逐topic合同

<a id="ct-mcp-acquireruntimecapabilities"></a>

### CT-mcp-AcquireRuntimeCapabilities

- 边界：`TopicAcquireRuntimeCapabilities` = `/mcp/runtime-prerequisite/acquire-capabilities`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/topics.go#L4)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/messages.go#L29)
- 接收provider：mcp；Command [AcquireRuntimeCapabilitiesCommand](#schema-acquireruntimecapabilitiescommand) → Result [AcquireRuntimeCapabilitiesResult](#schema-acquireruntimecapabilitiesresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L66) → `handleRuntimeCapabilities`（mcp）
- 接收入口：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L414)
- 已观察外部引用：orchestrator [sendRuntimeCapabilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L323)
- 本topic入口/具体类型分支的Source审查索引：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS11](../Workorch_组件固定合同与分层验收规范.md#cs11)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-mcp-acquireruntimeoperationadmissibility"></a>

### CT-mcp-AcquireRuntimeOperationAdmissibility

- 边界：`TopicAcquireRuntimeOperationAdmissibility` = `/mcp/runtime-prerequisite/acquire-operation-admissibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/topics.go#L5)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/messages.go#L30)
- 接收provider：mcp；Command [AcquireRuntimeOperationAdmissibilityCommand](#schema-acquireruntimeoperationadmissibilitycommand) → Result [AcquireRuntimeOperationAdmissibilityResult](#schema-acquireruntimeoperationadmissibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L66) → `handleRuntimeOperationAdmissibility`（mcp）
- 接收入口：[handleRuntimeOperationAdmissibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L427)
- 已观察外部引用：orchestrator [sendRuntimeOperationAdmissibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L64)
- 本topic入口/具体类型分支的Source审查索引：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS07](../Workorch_组件固定合同与分层验收规范.md#cs07)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := mcpevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-mcp-executeoperation"></a>

### CT-mcp-ExecuteOperation

- 边界：`TopicExecuteOperation` = `/mcp/runtime-operation/execute`；CF06；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/messages.go#L34)
- 接收provider：mcp；Command [ExecuteOperationCommand](#schema-executeoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L66) → `handler`（mcp）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L66) → `handleMCPOperation`（mcp）
- 接收入口：[handleMCPOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L253)
- 已观察外部引用：orchestrator [executeMCP](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L160)
- 本topic入口/具体类型分支的Source审查索引：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil`；`err := command.Execution.Validate(); err != nil`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`；`governance != nil && command.Execution.GovernanceGrant == nil`；`err := validateMCPApprovalGrant(command.Execution.GovernanceGrant, *governance); err != nil`；`err := prepareMCPEvidenceStore(command.WorkspaceInternalDirectory); err != nil`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-mcp-proberuntimeoperationfeasibility"></a>

### CT-mcp-ProbeRuntimeOperationFeasibility

- 边界：`TopicProbeRuntimeOperationFeasibility` = `/mcp/runtime-operation/probe-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/messages.go#L31)
- 接收provider：mcp；Command [ProbeRuntimeOperationFeasibilityCommand](#schema-proberuntimeoperationfeasibilitycommand) → Result [ProbeRuntimeOperationFeasibilityResult](#schema-proberuntimeoperationfeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L66) → `handleRuntimeOperationFeasibility`（mcp）
- 接收入口：[handleRuntimeOperationFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L552)
- 已观察外部引用：orchestrator [sendRuntimeOperationFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L127)
- 本topic入口/具体类型分支的Source审查索引：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := mcpevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil`；`revision != command.Admissibility.ObservedRevision`；`!containsExactMCPValue(serverIDs, data.ServerID)`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-mcp-proberuntimesemanticprooffeasibility"></a>

### CT-mcp-ProbeRuntimeSemanticProofFeasibility

- 边界：`TopicProbeRuntimeSemanticProofFeasibility` = `/mcp/runtime-semantic/probe-proof-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/messages.go#L32)
- 接收provider：mcp；Command [ProbeRuntimeSemanticProofFeasibilityCommand](#schema-proberuntimesemanticprooffeasibilitycommand) → Result [ProbeRuntimeSemanticProofFeasibilityResult](#schema-proberuntimesemanticprooffeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L66) → `handleRuntimeSemanticProofFeasibility`（mcp）
- 接收入口：[handleRuntimeSemanticProofFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L615)
- 已观察外部引用：orchestrator [sendRuntimeSemanticProofFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_semantic_feasibility_event_adapter.go#L61)
- 本topic入口/具体类型分支的Source审查索引：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := mcpevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil`；`revision != command.OwnerCatalog.ObservedRevision`；`!surfaceFound || surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady || surface.ProofApplicability != runtimecontract.RuntimeValidationProofDirect || surface.InputBindingRef == "" || !containsExactMCPValue(recipeRefs, command.Proposal.ValidationRecipeID)`；`probeErr := m.runtimeAdapter.ProbeOperation(event.Context(), RuntimeOperationRequest{ WorkspaceID: command.WorkspaceID, RunID: command.RunID, StepID: common.StepID("semantic-preflight/" + command.FeasibilityBatchID + "/" + string(command.Proposal.CoverageWorkItemID)), Kind: runtimecontract.StepKindMCPToolCall, WorkspaceDeliverableDirectory: command.WorkspaceDeliverableDirectory, TargetRefs: runtimecontract.RuntimeSemanticProposalTargetRefs(command.Proposal), Data: mcpevents.OperationData{ServerID: serverID, Tool: tool}, }); probeErr != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-mcp-queryruntimecwireconciliation"></a>

### CT-mcp-QueryRuntimeCWIReconciliation

- 边界：`TopicQueryRuntimeCWIReconciliation` = `/mcp/reconciliation/query-cwi`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/messages.go#L33)
- 接收provider：mcp；Command [QueryRuntimeCWIReconciliationCommand](#schema-queryruntimecwireconciliationcommand) → Result [QueryRuntimeCWIReconciliationResult](#schema-queryruntimecwireconciliationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L66) → `handleRuntimeCWIReconciliationQuery`（mcp）
- 接收入口：[handleRuntimeCWIReconciliationQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L150)
- 已观察外部引用：orchestrator [sendRuntimeCWIReconciliationOwnerQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_reconciliation.go#L185)
- 本topic入口/具体类型分支的Source审查索引：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS05](../Workorch_组件固定合同与分层验收规范.md#cs05)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil`；`!runtimeCWIReconciliationBindingHasMCPProof(command.Binding)`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-acquireruntimecapabilitiescommand"></a>

### AcquireRuntimeCapabilitiesCommand

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_capability.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_capability.go#L5)
- 声明：`type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest`
- 字段引用类型：[RuntimeCapabilityAcquisitionRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L15)

<a id="schema-acquireruntimecapabilitiesresult"></a>

### AcquireRuntimeCapabilitiesResult

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_capability.go:6](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_capability.go#L6)
- 字段：`Acquisition runtimecontract.RuntimeCapabilityAcquisition`
- 字段引用类型：[RuntimeCapabilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L27)

<a id="schema-acquireruntimeoperationadmissibilitycommand"></a>

### AcquireRuntimeOperationAdmissibilityCommand

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go#L5)
- 声明：`type AcquireRuntimeOperationAdmissibilityCommand runtimecontract.RuntimeOperationAdmissibilityRequest`
- 字段引用类型：[RuntimeOperationAdmissibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L14)
- 实际方法：[ValidateRuntimeOperationAdmissibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go#L43)

<a id="schema-acquireruntimeoperationadmissibilityresult"></a>

### AcquireRuntimeOperationAdmissibilityResult

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go#L7)
- 字段：`Acquisition runtimecontract.RuntimeOperationAdmissibilityAcquisition`
- 字段引用类型：[RuntimeOperationAdmissibilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L83)

<a id="schema-executeoperationcommand"></a>

### ExecuteOperationCommand

- 源码：[internal/modules/blocks/mcp/pkg/events/messages.go:17](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/messages.go#L17)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`；`TargetRole string`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)

<a id="schema-executeoperationresult"></a>

### ExecuteOperationResult

- 源码：[internal/modules/blocks/mcp/pkg/events/messages.go:23](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/messages.go#L23)
- 字段：`Receipt runtimecontract.RuntimeOperationReceipt`
- 字段引用类型：[RuntimeOperationReceipt（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_receipt.go#L139)

<a id="schema-proberuntimeoperationfeasibilitycommand"></a>

### ProbeRuntimeOperationFeasibilityCommand

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go#L11)
- 声明：`type ProbeRuntimeOperationFeasibilityCommand runtimecontract.RuntimeOperationFeasibilityRequest`
- 字段引用类型：[RuntimeOperationFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L253)
- 实际方法：[ValidateRuntimeOperationFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go#L31)

<a id="schema-proberuntimeoperationfeasibilityresult"></a>

### ProbeRuntimeOperationFeasibilityResult

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go#L13)
- 声明：`type ProbeRuntimeOperationFeasibilityResult runtimecontract.RuntimeOperationFeasibilityObservation`
- 字段引用类型：[RuntimeOperationFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L270)

<a id="schema-proberuntimesemanticprooffeasibilitycommand"></a>

### ProbeRuntimeSemanticProofFeasibilityCommand

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go#L15)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityCommand runtimecontract.RuntimeSemanticProofFeasibilityRequest`
- 字段引用类型：[RuntimeSemanticProofFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L13)
- 实际方法：[ValidateRuntimeSemanticProofFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go#L19)

<a id="schema-proberuntimesemanticprooffeasibilityresult"></a>

### ProbeRuntimeSemanticProofFeasibilityResult

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go:17](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go#L17)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityResult runtimecontract.RuntimeSemanticProofFeasibilityObservation`
- 字段引用类型：[RuntimeSemanticProofFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L24)

<a id="schema-queryruntimecwireconciliationcommand"></a>

### QueryRuntimeCWIReconciliationCommand

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_reconciliation.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_reconciliation.go#L5)
- 声明：`type QueryRuntimeCWIReconciliationCommand runtimecontract.RuntimeCWIOwnerReconciliationRequest`
- 字段引用类型：[RuntimeCWIOwnerReconciliationRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L371)
- 实际方法：[ValidateForOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_reconciliation.go#L11)

<a id="schema-queryruntimecwireconciliationresult"></a>

### QueryRuntimeCWIReconciliationResult

- 源码：[internal/modules/blocks/mcp/pkg/events/runtime_reconciliation.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/runtime_reconciliation.go#L7)
- 字段：`Observation runtimecontract.RuntimeCWIReconciliationObservation`
- 字段引用类型：[RuntimeCWIReconciliationObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L467)

<a id="schema-runtimeexecutionauthorization"></a>

### RuntimeExecutionAuthorization

- 源码：[internal/modules/blocks/mcp/pkg/events/operation.go:21](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/operation.go#L21)
- 字段：`CatalogSnapshotID string`；`AcquisitionBinding string`；`RegistryRevision string`；`Capability runtimecontract.RuntimeMCPCapabilityContext`；`Policy runtimecontract.RuntimeOperationPolicyFact`；`ValidationSurface *runtimecontract.RuntimeValidationSurface`
- 字段引用类型：[RuntimeMCPCapabilityContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_activation.go#L76)；[RuntimeOperationPolicyFact（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_policy.go#L13)；[RuntimeValidationSurface（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_validation_surface.go#L40)
- 实际方法：[normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/operation.go#L30)；[validateForOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/operation.go#L45)；[ValidateForExecution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/operation.go#L165)
- 空值/identity/版本/范围原式：`a.ValidationSurface != nil`；`a.CatalogSnapshotID == "" || a.RegistryRevision == ""`；`err := a.Policy.Validate(); err != nil`；`capability.ID == "" || capability.ServerID == "" || !capability.Enabled || capability.Status != "enabled" || capability.OperationKind != operation.Kind || a.Policy.OwnerModuleID != runtimecontract.RuntimeModuleMCP || a.Policy.OperationKind != operation.Kind || a.Policy.SubjectRef != string(capability.ID) || a.Policy.EffectScope != runtimecontract.RuntimeEffectScope(capability.EffectScope) || a.Policy.IdempotencyClass != runtimecontract.RuntimeOperationIdempotencyClass(capability.IdempotencyClass)`；`!found || matched.ID != a.Policy.ID`；`err != nil`；`string(capability.ServerID) != data.ServerID`；`capability.Kind != "tool" || capability.Name != data.Tool`；`err := a.ValidationSurface.Validate(); err != nil`；`a.ValidationSurface.OwnerModuleID != runtimecontract.RuntimeModuleMCP || a.ValidationSurface.OperationKind != operation.Kind || a.ValidationSurface.ID != runtimecontract.RuntimeMCPToolValidationRecipeID(data.ServerID, data.Tool)`；`capability.Kind != "resource" || firstNonEmpty(capability.URI, capability.Name) != data.URI`；`capability.Kind != "prompt" || capability.Name != data.Prompt`；`err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil`；`!mcpOperationKind(operation.Kind) || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleMCP`；`err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil`；`operation.ID != strings.TrimSpace(context.OperationID)`；`!sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs)`；`err := authorization.validateForOperation(operation); err != nil`；`err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, []runtimecontract.RuntimeOperationPolicyFact{authorization.Policy}); err != nil`；`strings.TrimSpace(context.Execution.AdmissibilitySnapshotID) != authorization.CatalogSnapshotID || strings.TrimSpace(context.Execution.AcquisitionBinding) != authorization.AcquisitionBinding`；`len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1`
- 显式分支集合（只表列源码case，default语义见validator）：`runtimecontract.StepKindMCPToolCall`；`runtimecontract.StepKindMCPResourceRead`；`runtimecontract.StepKindMCPPromptGet`

<a id="schema-sealedruntimeoperation"></a>

### SealedRuntimeOperation

- 源码：[internal/modules/blocks/mcp/pkg/events/operation.go:104](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/operation.go#L104)
- 字段：`Operation runtimecontract.RuntimeOperation`；`Authorization RuntimeExecutionAuthorization`；`PayloadHash string`
- 字段引用类型：[RuntimeOperation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate.go#L395)；[RuntimeExecutionAuthorization](#schema-runtimeexecutionauthorization)
- 实际方法：[ValidateForExecution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/events/operation.go#L165)
- 空值/identity/版本/范围原式：`err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil`；`!mcpOperationKind(operation.Kind) || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleMCP`；`err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil`；`operation.ID != strings.TrimSpace(context.OperationID)`；`!sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs)`；`err := authorization.validateForOperation(operation); err != nil`；`err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, []runtimecontract.RuntimeOperationPolicyFact{authorization.Policy}); err != nil`；`strings.TrimSpace(context.Execution.AdmissibilitySnapshotID) != authorization.CatalogSnapshotID || strings.TrimSpace(context.Execution.AcquisitionBinding) != authorization.AcquisitionBinding`；`err != nil`；`len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [mcpRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L875)：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`
  - helper完整分支：`func mcpRuntimeIngressCommand[T any](event magicevent.Event, result magicevent.Result) (T, bool) { var zero T if result == nil { return zero, false } if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator { setMCPRuntimeIngressInvalid(result, "runtime MCP ingress requires exact Orchestrator source") return zero, false } command, ok := event.Data().(T) if !ok { setMCPRuntimeIngressInvalid(result, "runtime MCP ingress payload has an invalid concrete type") return zero, false } return command, true }`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`