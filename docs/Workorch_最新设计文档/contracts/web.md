# web：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页5个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

这里的Web是HTTP/网页能力owner，不是面向用户的Workorch Web控制台。查询/执行遵循URL、redirect、协议、网络目标和数据去向约束；API/HTTP 200只说明传输事实。配置变化不能替换旧dispatch封存target/provider语义。

Search/fetch实际返回的受限内容、artifact和截断信息与执行receipt一起校验。该owner当前无独立CWI reconciliation topic；不支持的写请求unknown和跨Run冲突不得当安全重试。

## 逐topic合同

<a id="ct-web-acquireruntimecapabilities"></a>

### CT-web-AcquireRuntimeCapabilities

- 边界：`TopicAcquireRuntimeCapabilities` = `/web/runtime-prerequisite/acquire-capabilities`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/topics.go#L4)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/messages.go#L31)
- 接收provider：web；Command [AcquireRuntimeCapabilitiesCommand](#schema-acquireruntimecapabilitiescommand) → Result [AcquireRuntimeCapabilitiesResult](#schema-acquireruntimecapabilitiesresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L54) → `handleRuntimeCapabilities`（web）
- 接收入口：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L251)
- 已观察外部引用：orchestrator [sendRuntimeCapabilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L323)
- 本topic入口/具体类型分支的Source审查索引：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS11](../Workorch_组件固定合同与分层验收规范.md#cs11)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-web-acquireruntimeoperationadmissibility"></a>

### CT-web-AcquireRuntimeOperationAdmissibility

- 边界：`TopicAcquireRuntimeOperationAdmissibility` = `/web/runtime-prerequisite/acquire-operation-admissibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/topics.go#L5)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/messages.go#L32)
- 接收provider：web；Command [AcquireRuntimeOperationAdmissibilityCommand](#schema-acquireruntimeoperationadmissibilitycommand) → Result [AcquireRuntimeOperationAdmissibilityResult](#schema-acquireruntimeoperationadmissibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L54) → `handleRuntimeOperationAdmissibility`（web）
- 接收入口：[handleRuntimeOperationAdmissibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L264)
- 已观察外部引用：orchestrator [sendRuntimeOperationAdmissibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L64)
- 本topic入口/具体类型分支的Source审查索引：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS07](../Workorch_组件固定合同与分层验收规范.md#cs07)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := webevents.ValidateRuntimeOperationAdmissibilityCommand(command, ModuleID); err != nil`；`err := inventory.Validate(); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-web-executeoperation"></a>

### CT-web-ExecuteOperation

- 边界：`TopicExecuteOperation` = `/web/runtime-operation/execute`；CF06；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/messages.go#L35)
- 接收provider：web；Command [ExecuteOperationCommand](#schema-executeoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[subscribeOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner_lifecycle.go#L37) → `subscribeOwner`（web）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L54) → `handleWebOperation`（web）
- 接收入口：[subscribeOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner_lifecycle.go#L37)；[handleWebOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L124)
- 已观察外部引用：orchestrator [executeWeb](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L140)
- 本topic入口/具体类型分支的Source审查索引：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`ev == nil || result == nil`；`err := orchestratorevents.ValidateRuntimeOperationScope(command.WorkspaceID, command.RunID, command.DispatchID, command.Transaction, command.OperationID, command.EvidenceKind, command.WorkspaceDeliverableDirectory); err != nil`；`strings.TrimSpace(command.WorkspaceInternalDirectory) == ""`；`err := command.Execution.Validate(); err != nil`；`command.Execution.ExpectedEffectScope != runtimecontract.RuntimeEffectNone`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-web-proberuntimeoperationfeasibility"></a>

### CT-web-ProbeRuntimeOperationFeasibility

- 边界：`TopicProbeRuntimeOperationFeasibility` = `/web/runtime-operation/probe-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/messages.go#L33)
- 接收provider：web；Command [ProbeRuntimeOperationFeasibilityCommand](#schema-proberuntimeoperationfeasibilitycommand) → Result [ProbeRuntimeOperationFeasibilityResult](#schema-proberuntimeoperationfeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L54) → `handleRuntimeOperationFeasibility`（web）
- 接收入口：[handleRuntimeOperationFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L318)
- 已观察外部引用：orchestrator [sendRuntimeOperationFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L127)
- 本topic入口/具体类型分支的Source审查索引：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := webevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil`；`revision != command.Admissibility.ObservedRevision`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-web-proberuntimesemanticprooffeasibility"></a>

### CT-web-ProbeRuntimeSemanticProofFeasibility

- 边界：`TopicProbeRuntimeSemanticProofFeasibility` = `/web/runtime-semantic/probe-proof-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/messages.go#L34)
- 接收provider：web；Command [ProbeRuntimeSemanticProofFeasibilityCommand](#schema-proberuntimesemanticprooffeasibilitycommand) → Result [ProbeRuntimeSemanticProofFeasibilityResult](#schema-proberuntimesemanticprooffeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L54) → `handleRuntimeSemanticProofFeasibility`（web）
- 接收入口：[handleRuntimeSemanticProofFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L369)
- 已观察外部引用：orchestrator [sendRuntimeSemanticProofFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_semantic_feasibility_event_adapter.go#L61)
- 本topic入口/具体类型分支的Source审查索引：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := webevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil`；`revision != command.OwnerCatalog.ObservedRevision`；`strings.TrimSpace(command.Proposal.ValidationRecipeID) != "" || len(command.Proposal.RequiredCheckRefs) != 0`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-acquireruntimecapabilitiescommand"></a>

### AcquireRuntimeCapabilitiesCommand

- 源码：[internal/modules/blocks/web/pkg/events/runtime_capability.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_capability.go#L5)
- 声明：`type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest`
- 字段引用类型：[RuntimeCapabilityAcquisitionRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L15)

<a id="schema-acquireruntimecapabilitiesresult"></a>

### AcquireRuntimeCapabilitiesResult

- 源码：[internal/modules/blocks/web/pkg/events/runtime_capability.go:6](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_capability.go#L6)
- 字段：`Acquisition runtimecontract.RuntimeCapabilityAcquisition`
- 字段引用类型：[RuntimeCapabilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L27)

<a id="schema-acquireruntimeoperationadmissibilitycommand"></a>

### AcquireRuntimeOperationAdmissibilityCommand

- 源码：[internal/modules/blocks/web/pkg/events/runtime_admissibility.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_admissibility.go#L5)
- 声明：`type AcquireRuntimeOperationAdmissibilityCommand runtimecontract.RuntimeOperationAdmissibilityRequest`
- 字段引用类型：[RuntimeOperationAdmissibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L14)
- 实际方法：[ValidateRuntimeOperationAdmissibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_admissibility.go#L43)

<a id="schema-acquireruntimeoperationadmissibilityresult"></a>

### AcquireRuntimeOperationAdmissibilityResult

- 源码：[internal/modules/blocks/web/pkg/events/runtime_admissibility.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_admissibility.go#L7)
- 字段：`Acquisition runtimecontract.RuntimeOperationAdmissibilityAcquisition`
- 字段引用类型：[RuntimeOperationAdmissibilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L83)

<a id="schema-executeoperationcommand"></a>

### ExecuteOperationCommand

- 源码：[internal/modules/blocks/web/pkg/events/messages.go:20](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/messages.go#L20)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)

<a id="schema-executeoperationresult"></a>

### ExecuteOperationResult

- 源码：[internal/modules/blocks/web/pkg/events/messages.go:25](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/messages.go#L25)
- 字段：`Receipt runtimecontract.RuntimeOperationReceipt`
- 字段引用类型：[RuntimeOperationReceipt（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_receipt.go#L139)

<a id="schema-proberuntimeoperationfeasibilitycommand"></a>

### ProbeRuntimeOperationFeasibilityCommand

- 源码：[internal/modules/blocks/web/pkg/events/runtime_admissibility.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_admissibility.go#L11)
- 声明：`type ProbeRuntimeOperationFeasibilityCommand runtimecontract.RuntimeOperationFeasibilityRequest`
- 字段引用类型：[RuntimeOperationFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L253)
- 实际方法：[ValidateRuntimeOperationFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_admissibility.go#L31)

<a id="schema-proberuntimeoperationfeasibilityresult"></a>

### ProbeRuntimeOperationFeasibilityResult

- 源码：[internal/modules/blocks/web/pkg/events/runtime_admissibility.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_admissibility.go#L13)
- 声明：`type ProbeRuntimeOperationFeasibilityResult runtimecontract.RuntimeOperationFeasibilityObservation`
- 字段引用类型：[RuntimeOperationFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L270)

<a id="schema-proberuntimesemanticprooffeasibilitycommand"></a>

### ProbeRuntimeSemanticProofFeasibilityCommand

- 源码：[internal/modules/blocks/web/pkg/events/runtime_admissibility.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_admissibility.go#L15)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityCommand runtimecontract.RuntimeSemanticProofFeasibilityRequest`
- 字段引用类型：[RuntimeSemanticProofFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L13)
- 实际方法：[ValidateRuntimeSemanticProofFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_admissibility.go#L19)

<a id="schema-proberuntimesemanticprooffeasibilityresult"></a>

### ProbeRuntimeSemanticProofFeasibilityResult

- 源码：[internal/modules/blocks/web/pkg/events/runtime_admissibility.go:17](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/runtime_admissibility.go#L17)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityResult runtimecontract.RuntimeSemanticProofFeasibilityObservation`
- 字段引用类型：[RuntimeSemanticProofFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L24)

<a id="schema-runtimeexecutionauthorization"></a>

### RuntimeExecutionAuthorization

- 源码：[internal/modules/blocks/web/pkg/events/operation.go:20](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/operation.go#L20)
- 字段：`CatalogSnapshotID string`；`AcquisitionBinding string`；`PrerequisiteID string`；`OwnerRevision string`；`CatalogObservation runtimecontract.RuntimePrerequisiteObservation`；`Policy runtimecontract.RuntimeOperationPolicyFact`
- 字段引用类型：[RuntimePrerequisiteObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_prerequisite_freshness.go#L101)；[RuntimeOperationPolicyFact（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_policy.go#L13)
- 实际方法：[normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/operation.go#L29)；[validateForOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/operation.go#L39)；[ValidateForExecution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/operation.go#L108)
- 空值/identity/版本/范围原式：`a.CatalogSnapshotID == "" || a.PrerequisiteID == "" || a.OwnerRevision == ""`；`err := a.CatalogObservation.Validate(); err != nil`；`a.CatalogObservation.PrerequisiteID != a.PrerequisiteID || a.CatalogObservation.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility || a.CatalogObservation.OwnerModuleID != runtimecontract.RuntimeModuleWeb || a.CatalogObservation.ContentHash != a.CatalogSnapshotID || !webObservationHasOwnerRevision(a.CatalogObservation, a.OwnerRevision)`；`err := a.Policy.Validate(); err != nil`；`!found || matched.ID != a.Policy.ID || a.Policy.OwnerModuleID != runtimecontract.RuntimeModuleWeb`；`err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil`；`!webOperationKind(operation.Kind) || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleWeb`；`err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil`；`operation.ID != strings.TrimSpace(context.OperationID)`；`!sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs)`；`err := authorization.validateForOperation(operation); err != nil`；`err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, []runtimecontract.RuntimeOperationPolicyFact{authorization.Policy}); err != nil`；`strings.TrimSpace(context.Execution.AdmissibilitySnapshotID) != authorization.CatalogSnapshotID || strings.TrimSpace(context.Execution.AcquisitionBinding) != authorization.AcquisitionBinding`；`err != nil`；`len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1`

<a id="schema-sealedruntimeoperation"></a>

### SealedRuntimeOperation

- 源码：[internal/modules/blocks/web/pkg/events/operation.go:67](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/operation.go#L67)
- 字段：`Operation runtimecontract.RuntimeOperation`；`Authorization RuntimeExecutionAuthorization`；`PayloadHash string`
- 字段引用类型：[RuntimeOperation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate.go#L395)；[RuntimeExecutionAuthorization](#schema-runtimeexecutionauthorization)
- 实际方法：[ValidateForExecution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/pkg/events/operation.go#L108)
- 空值/identity/版本/范围原式：`err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil`；`!webOperationKind(operation.Kind) || runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleWeb`；`err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil`；`operation.ID != strings.TrimSpace(context.OperationID)`；`!sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs)`；`err := authorization.validateForOperation(operation); err != nil`；`err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, []runtimecontract.RuntimeOperationPolicyFact{authorization.Policy}); err != nil`；`strings.TrimSpace(context.Execution.AdmissibilitySnapshotID) != authorization.CatalogSnapshotID || strings.TrimSpace(context.Execution.AcquisitionBinding) != authorization.AcquisitionBinding`；`err != nil`；`len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [webRuntimeIngressCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/web/biz/owner.go#L450)：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`
  - helper完整分支：`func webRuntimeIngressCommand[T any](event magicevent.Event, result magicevent.Result) (T, bool) { var zero T if result == nil { return zero, false } if event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator { setWebRuntimeIngressInvalid(result, "runtime Web ingress requires exact Orchestrator source") return zero, false } command, ok := event.Data().(T) if !ok { setWebRuntimeIngressInvalid(result, "runtime Web ingress payload has an invalid concrete type") return zero, false } return command, true }`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`