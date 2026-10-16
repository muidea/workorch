# file：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页19个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

File的runtime查询只返回文件owner事实；authoring资源读取是pre-Run能力，不能强制伪Run。Execute的唯一payload是SealedOperation.Operation，FileList/FileRead/FileEdit/FileWrite/PatchApply Data只由owner解码，调用方不能另传第二份Data。ContentPresent/NewTextPresent使合法空文本与未提供不同；BaseHash/Occurrence参与写前校验。路径必须在获准deliverable/source范围，符号链接/目录穿越/源目录保留按VS-V40验证。

File执行端核对operation kind/ID、target与CWI集合、execution authority及PayloadHash。当前SealRuntimeOperation使用JSON做detached clone；目标明确改为typed深拷贝，保留原规范hash编码。receipt经Orchestrator query/commit合同持久；query Found=false仅在完整identity/fence准入成立时有意义。读/preview也走sealed execute lane，不能因为无durable effect省掉准入。query同族并不等价：target-selection不能把ProspectiveTargetRef当已存在；repair confirmation回显RequestID/IssuedAtStateRevision/ExpectedSubjectRevision并与fresh manifest比较。

## 逐topic合同

<a id="ct-file-acquireauthoringresources"></a>

### CT-file-AcquireAuthoringResources

- 边界：`TopicAcquireAuthoringResources` = `/file/authoring/acquire-resources`；CF01；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L17)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L183)
- 接收provider：file；Command [AcquireAuthoringResourceCommand](#schema-acquireauthoringresourcecommand) → Result [AcquireAuthoringResourceResult](#schema-acquireauthoringresourceresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleAcquireAuthoringResource`（file）
- 接收入口：[handleAcquireAuthoringResource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L48)
- 已观察外部引用：workorchd [acquireDefinitionAuthoringResources](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_resources.go#L24)
- 本topic入口/具体类型分支的Source审查索引：[handleVerifyAuthoringResource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L23)；[handleAcquireAuthoringResource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L48)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != authoringApplicationSource` 分支谓词（[handleAcquireAuthoringResource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L48)）
- 特化承诺：pre-Run authoring用AcquireAuthoringResourceCommand/Result（topic是复数Resources，类型是单数Resource）；资源ID/owner revision/content/hash需绑exact authoring request，不授execution permission。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event == nil || event.Source() != authoringApplicationSource`；`!ok || command.Validate() != nil`；`command.Validate() != nil || m == nil || m.runtimeAdapter == nil`；`!runtimeFileDeliverableTargetEligible(command.ResourceRef)`；`err != nil || filepath.ToSlash(ref) != command.ResourceRef`；`err != nil || observed != target || filepath.ToSlash(currentRef) != command.ResourceRef`；`err := response.ValidateFor(command); err != nil`；`!ok || view.ValidateFor(query) != nil`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-file-acquireruntimecapabilities"></a>

### CT-file-AcquireRuntimeCapabilities

- 边界：`TopicAcquireRuntimeCapabilities` = `/file/runtime-prerequisite/acquire-capabilities`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L172)
- 接收provider：file；Command [AcquireRuntimeCapabilitiesCommand](#schema-acquireruntimecapabilitiescommand) → Result [AcquireRuntimeCapabilitiesResult](#schema-acquireruntimecapabilitiesresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleRuntimeCapabilities`（file）
- 接收入口：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L639)
- 已观察外部引用：orchestrator [sendRuntimeCapabilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L323)
- 本topic入口/具体类型分支的Source审查索引：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS11](../Workorch_组件固定合同与分层验收规范.md#cs11)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-acquireruntimeoperationadmissibility"></a>

### CT-file-AcquireRuntimeOperationAdmissibility

- 边界：`TopicAcquireRuntimeOperationAdmissibility` = `/file/runtime-prerequisite/acquire-operation-admissibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L173)
- 接收provider：file；Command [AcquireRuntimeOperationAdmissibilityCommand](#schema-acquireruntimeoperationadmissibilitycommand) → Result [AcquireRuntimeOperationAdmissibilityResult](#schema-acquireruntimeoperationadmissibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleRuntimeOperationAdmissibility`（file）
- 接收入口：[handleRuntimeOperationAdmissibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L658)
- 已观察外部引用：orchestrator [sendRuntimeOperationAdmissibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L64)
- 本topic入口/具体类型分支的Source审查索引：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS07](../Workorch_组件固定合同与分层验收规范.md#cs07)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || m.runtimeAdapter == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.OwnerModuleID != ModuleID || command.WorkspaceDeliverableDirectory == "" || len(command.RequiredKinds) == 0`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-acquireruntimetargetselection"></a>

### CT-file-AcquireRuntimeTargetSelection

- 边界：`TopicAcquireRuntimeTargetSelection` = `/file/runtime-prerequisite/acquire-target-selection`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L180)
- 接收provider：file；Command [AcquireRuntimeTargetSelectionCommand](#schema-acquireruntimetargetselectioncommand) → Result [AcquireRuntimeTargetSelectionResult](#schema-acquireruntimetargetselectionresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleAcquireRuntimeTargetSelection`（file）
- 接收入口：[handleAcquireRuntimeTargetSelection](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1083)
- 已观察外部引用：orchestrator [AcquireRuntimeTargetSelectionPrerequisite](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L84)
- 本topic入口/具体类型分支的Source审查索引：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS11](../Workorch_组件固定合同与分层验收规范.md#cs11)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || m.runtimeAdapter == nil || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" || command.TargetRole == "" || command.TargetSelectorRef == "" || command.MaxItems < 1 || command.MaxItems > runtimecontract.RuntimeOwnerInventoryItemsMaxCount`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-acquiretargetsnapshot"></a>

### CT-file-AcquireTargetSnapshot

- 边界：`TopicAcquireTargetSnapshot` = `/file/runtime-prerequisite/acquire-target-snapshot`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L179)
- 接收provider：file；Command [AcquireTargetSnapshotCommand](#schema-acquiretargetsnapshotcommand) → Result [AcquireTargetSnapshotResult](#schema-acquiretargetsnapshotresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleAcquireTargetSnapshot`（file）
- 接收入口：[handleAcquireTargetSnapshot](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1063)
- 已观察外部引用：orchestrator [AcquireTargetSnapshotPrerequisite](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L28)
- 本topic入口/具体类型分支的Source审查索引：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS11](../Workorch_组件固定合同与分层验收规范.md#cs11)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == "" || command.TargetRef == ""`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-acquireworkspacemanifest"></a>

### CT-file-AcquireWorkspaceManifest

- 边界：`TopicAcquireWorkspaceManifest` = `/file/runtime-prerequisite/acquire-workspace-manifest`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L15)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L181)
- 接收provider：file；Command [AcquireWorkspaceManifestCommand](#schema-acquireworkspacemanifestcommand) → Result [AcquireWorkspaceManifestResult](#schema-acquireworkspacemanifestresult)；target合法caller：kernel/orchestrator、blocks/git
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleAcquireWorkspaceManifest`（file）
- 接收入口：[handleAcquireWorkspaceManifest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L936)
- 已观察外部引用：orchestrator [AcquireAndCommitWorkspaceManifestPrerequisite](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L167)；git [acquireFileWorkspaceTreeRevision](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L291)
- 本topic入口/具体类型分支的Source审查索引：[requireWorkspaceManifestSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1122)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireWorkspaceManifestSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1122)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS11](../Workorch_组件固定合同与分层验收规范.md#cs11)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == "" || command.WorkspaceDeliverableDirectory == ""`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-confirmruntimerepairsurface"></a>

### CT-file-ConfirmRuntimeRepairSurface

- 边界：`TopicConfirmRuntimeRepairSurface` = `/file/runtime-query/confirm-repair-surface`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L16)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L182)
- 接收provider：file；Command [ConfirmRuntimeRepairSurfaceCommand](#schema-confirmruntimerepairsurfacecommand) → Result [ConfirmRuntimeRepairSurfaceResult](#schema-confirmruntimerepairsurfaceresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleConfirmRuntimeRepairSurface`（file）
- 接收入口：[handleConfirmRuntimeRepairSurface](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1011)
- 已观察外部引用：orchestrator [ConfirmFileRuntimeRepairSurface](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L120)
- 本topic入口/具体类型分支的Source审查索引：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：RequestID、IssuedAtStateRevision、ExpectedSubjectRevision须回显，owner基于fresh WorkspaceManifest判断；不同revision返回stale/unavailable事实，Core重观察而非临时修复。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || m.runtimeAdapter == nil || strings.TrimSpace(command.RequestID) == "" || strings.TrimSpace(command.RequestID) != command.RequestID || command.IssuedAtStateRevision == 0 || strings.TrimSpace(command.ExpectedSubjectRevision) == "" || strings.TrimSpace(command.ExpectedSubjectRevision) != command.ExpectedSubjectRevision || command.WorkspaceID == "" || command.RunID == "" || strings.TrimSpace(command.WorkspaceDeliverableDirectory) == ""`；`observation.WorkspaceID != command.WorkspaceID || observation.RunID != command.RunID || observation.Validate() != nil`；`manifest.Revision != command.ExpectedSubjectRevision`；`err := confirmation.Validate(); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-executefileeditoperation"></a>

### CT-file-ExecuteFileEditOperation

- 边界：`TopicExecuteFileEditOperation` = `/file/runtime-operation/edit`；CF06；source existing / target change；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L22)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L187)
- 接收provider：file；Command [ExecuteFileEditOperationCommand](#schema-executefileeditoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleFileEditOperation`（file）
- 接收入口：[handleFileEditOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L276)
- 已观察外部引用：orchestrator [executeFileEdit](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L236)
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event == nil`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`；`command.Transaction.DefinitionRevision == "" || command.Transaction.DefinitionRevision != strings.TrimSpace(command.Transaction.DefinitionRevision)`；`err := validateFileReplayProof(command.Execution.ReplayProof, command.OperationID, data.Path); err != nil`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-file-executefilelistoperation"></a>

### CT-file-ExecuteFileListOperation

- 边界：`TopicExecuteFileListOperation` = `/file/runtime-operation/list`；CF06；source existing / target change；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L20)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L185)
- 接收provider：file；Command [ExecuteFileListOperationCommand](#schema-executefilelistoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleFileListOperation`（file）
- 接收入口：[handleFileListOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L154)
- 已观察外部引用：orchestrator [executeFileList](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L86)
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event == nil`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-file-executefilereadoperation"></a>

### CT-file-ExecuteFileReadOperation

- 边界：`TopicExecuteFileReadOperation` = `/file/runtime-operation/read`；CF06；source existing / target change；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L21)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L186)
- 接收provider：file；Command [ExecuteFileReadOperationCommand](#schema-executefilereadoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleFileReadOperation`（file）
- 接收入口：[handleFileReadOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L209)
- 已观察外部引用：orchestrator [executeFileRead](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L103)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeFileReadDependenciesQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/file_read_dependency_query.go#L15)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event == nil`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`；`command.Transaction.DefinitionRevision == "" || command.Transaction.DefinitionRevision != strings.TrimSpace(command.Transaction.DefinitionRevision)`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-file-executefilewriteoperation"></a>

### CT-file-ExecuteFileWriteOperation

- 边界：`TopicExecuteFileWriteOperation` = `/file/runtime-operation/write`；CF06；source existing / target change；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L23)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L188)
- 接收provider：file；Command [ExecuteFileWriteOperationCommand](#schema-executefilewriteoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleFileWriteOperation`（file）
- 接收入口：[handleFileWriteOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L355)
- 已观察外部引用：orchestrator [executeFileWrite](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L253)
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event == nil`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`；`command.Transaction.DefinitionRevision == "" || command.Transaction.DefinitionRevision != strings.TrimSpace(command.Transaction.DefinitionRevision)`；`err := validateFileReplayProof(command.Execution.ReplayProof, command.OperationID, data.TargetRef); err != nil`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-file-executepatchapplyoperation"></a>

### CT-file-ExecutePatchApplyOperation

- 边界：`TopicExecutePatchApplyOperation` = `/file/runtime-operation/patch-apply`；CF06；source existing / target change；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L24)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L189)
- 接收provider：file；Command [ExecutePatchApplyOperationCommand](#schema-executepatchapplyoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handlePatchApplyOperation`（file）
- 接收入口：[handlePatchApplyOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L434)
- 已观察外部引用：orchestrator [executePatchApply](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L270)
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event == nil`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`；`command.Transaction.DefinitionRevision == "" || command.Transaction.DefinitionRevision != strings.TrimSpace(command.Transaction.DefinitionRevision)`；`err := validateFileReplayProof(command.Execution.ReplayProof, command.OperationID, data.TargetRef); err != nil`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-file-executepatchpreviewoperation"></a>

### CT-file-ExecutePatchPreviewOperation

- 边界：`TopicExecutePatchPreviewOperation` = `/file/runtime-operation/patch-preview`；CF06；source existing / target change；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L25)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L190)
- 接收provider：file；Command [ExecutePatchPreviewOperationCommand](#schema-executepatchpreviewoperationcommand) → Result [ExecuteOperationResult](#schema-executeoperationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handlePatchPreviewOperation`（file）
- 接收入口：[handlePatchPreviewOperation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L557)
- 已观察外部引用：orchestrator [executePatchPreview](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_executor.go#L287)
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS02](../Workorch_组件固定合同与分层验收规范.md#cs02)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event == nil`；`receipt, found, err := m.queryRuntimeOperationReceipt(event.Context(), command.WorkspaceID, command.RunID, command.DispatchID, command.OperationID, command.Execution); err != nil`
- 验收适用集：[CP02](../Workorch_组件固定合同与分层验收规范.md#cp02)

<a id="ct-file-proberuntimeoperationfeasibility"></a>

### CT-file-ProbeRuntimeOperationFeasibility

- 边界：`TopicProbeRuntimeOperationFeasibility` = `/file/runtime-operation/probe-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L175)
- 接收provider：file；Command [ProbeRuntimeOperationFeasibilityCommand](#schema-proberuntimeoperationfeasibilitycommand) → Result [ProbeRuntimeOperationFeasibilityResult](#schema-proberuntimeoperationfeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleRuntimeOperationFeasibility`（file）
- 接收入口：[handleRuntimeOperationFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L679)
- 已观察外部引用：orchestrator [sendRuntimeOperationFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_operation_admissibility_event_adapter.go#L127)
- 本topic入口/具体类型分支的Source审查索引：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`err := fileevents.ValidateRuntimeOperationFeasibilityCommand(command, ModuleID); err != nil`；`repair := command.OperationAuthorization.InputRepair; repair != nil && failure == nil && stale == nil && !repair.MatchesReplayProof(replayProof)`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-proberuntimesemanticprooffeasibility"></a>

### CT-file-ProbeRuntimeSemanticProofFeasibility

- 边界：`TopicProbeRuntimeSemanticProofFeasibility` = `/file/runtime-semantic/probe-proof-feasibility`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L176)
- 接收provider：file；Command [ProbeRuntimeSemanticProofFeasibilityCommand](#schema-proberuntimesemanticprooffeasibilitycommand) → Result [ProbeRuntimeSemanticProofFeasibilityResult](#schema-proberuntimesemanticprooffeasibilityresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleRuntimeSemanticProofFeasibility`（file）
- 接收入口：[handleRuntimeSemanticProofFeasibility](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L766)
- 已观察外部引用：orchestrator [sendRuntimeSemanticProofFeasibilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_semantic_feasibility_event_adapter.go#L61)
- 本topic入口/具体类型分支的Source审查索引：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS15](../Workorch_组件固定合同与分层验收规范.md#cs15)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`_, err := common.BindRuntimeWorkspaceContext(event.Context(), command.WorkspaceID); err != nil`；`err := fileevents.ValidateRuntimeSemanticProofFeasibilityCommand(command, ModuleID); err != nil`；`manifest.Revision != command.OwnerCatalog.ObservedRevision`；`command.Proposal.Normalized().Target.Policy == runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget`；`strings.TrimSpace(command.Proposal.ValidationRecipeID) != "" || len(command.Proposal.RequiredCheckRefs) != 0`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-queryruntimecwireconciliation"></a>

### CT-file-QueryRuntimeCWIReconciliation

- 边界：`TopicQueryRuntimeCWIReconciliation` = `/file/reconciliation/query-cwi`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L177)
- 接收provider：file；Command [QueryRuntimeCWIReconciliationCommand](#schema-queryruntimecwireconciliationcommand) → Result [QueryRuntimeCWIReconciliationResult](#schema-queryruntimecwireconciliationresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleRuntimeCWIReconciliationQuery`（file）
- 接收入口：[handleRuntimeCWIReconciliationQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L960)
- 已观察外部引用：orchestrator [sendRuntimeCWIReconciliationOwnerQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_reconciliation.go#L185)
- 本topic入口/具体类型分支的Source审查索引：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS05](../Workorch_组件固定合同与分层验收规范.md#cs05)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || m == nil || m.runtimeAdapter == nil || command.ValidateForOwner(ModuleID) != nil`；`err := observation.Validate(); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-queryruntimefilereaddependencies"></a>

### CT-file-QueryRuntimeFileReadDependencies

- 边界：`TopicQueryRuntimeFileReadDependencies` = `/file/runtime-query/read-dependencies`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L178)
- 接收provider：file；Command [QueryRuntimeFileReadDependenciesCommand](#schema-queryruntimefilereaddependenciescommand) → Result [QueryRuntimeFileReadDependenciesResult](#schema-queryruntimefilereaddependenciesresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleRuntimeFileReadDependenciesQuery`（file）
- 接收入口：[handleRuntimeFileReadDependenciesQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/file_read_dependency_query.go#L15)
- 已观察外部引用：orchestrator [queryRuntimeFileReadDependencies](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_file_read_dependency_event_adapter.go#L40)
- 本topic入口/具体类型分支的Source审查索引：[handleRuntimeFileReadDependenciesQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/file_read_dependency_query.go#L15)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 源caller准入：`Source != runtimecontract.RuntimeModuleOrchestrator` 分支谓词（[handleRuntimeFileReadDependenciesQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/file_read_dependency_query.go#L15)）
- 特化承诺：source新增只读：只重观察原sealed file/read选择器与SourceReceiptHash绑定的OriginalDependencies；回显RequestID/IssuedAtStateRevision/receipt，observed必须携CurrentDependencies，missing不能伪造新revision。owner提供事实，不执行、不改变旧receipt、不自行授credit。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`；`!ok || m.runtimeAdapter == nil || command.RequestID == "" || command.RequestID != strings.TrimSpace(command.RequestID) || command.IssuedAtStateRevision == 0 || command.SourceReceiptHash == "" || command.SourceReceiptHash != strings.TrimSpace(command.SourceReceiptHash) || command.Transaction.DefinitionRevision == "" || command.Transaction.DefinitionRevision != strings.TrimSpace(command.Transaction.DefinitionRevision)`；`!reflect.DeepEqual(original, original.Normalized())`；`err := original.Validate(); err != nil || original.DefinitionRevision != command.Transaction.DefinitionRevision || len(original.Entries) != 1`；`entry.OwnerKind != "capability" || entry.OwnerInstanceID != ModuleID+"/"+string(command.WorkspaceID) || entry.ResourceKind != string(runtimecontract.RuntimeResourceWorkspaceTarget) || entry.ResourceID != data.Path`；`!found || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != command.SourceReceiptHash || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectNone || !reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, &original) || !reflect.DeepEqual(receipt.TargetRefs, operation.TargetRefs)`
- 验收适用集：[CP10](../Workorch_组件固定合同与分层验收规范.md#cp10)

<a id="ct-file-validateruntimeoperationcandidateconformance"></a>

### CT-file-ValidateRuntimeOperationCandidateConformance

- 边界：`TopicValidateRuntimeOperationCandidateConformance` = `/file/runtime-operation/validate-candidate-conformance`；CF04；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L174)
- 接收provider：file；Command [ValidateRuntimeOperationCandidateConformanceCommand](#schema-validateruntimeoperationcandidateconformancecommand) → Result [ValidateRuntimeOperationCandidateConformanceResult](#schema-validateruntimeoperationcandidateconformanceresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleRuntimeOperationCandidateConformance`（file）
- 接收入口：[handleRuntimeOperationCandidateConformance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L740)
- 已观察外部引用：orchestrator [ValidateRuntimeOperationCandidateConformance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_candidate_conformance_event_adapter.go#L16)
- 本topic入口/具体类型分支的Source审查索引：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS07](../Workorch_组件固定合同与分层验收规范.md#cs07)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`err := fileevents.ValidateRuntimeOperationCandidateConformanceCommandValue(command, ModuleID); err != nil`
- 验收适用集：[CP07](../Workorch_组件固定合同与分层验收规范.md#cp07)

<a id="ct-file-verifyauthoringresource"></a>

### CT-file-VerifyAuthoringResource

- 边界：`TopicVerifyAuthoringResource` = `/file/authoring/verify-resource`；CF01；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/topics.go#L18)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L184)
- 接收provider：file；Command [VerifyAuthoringResourceCommand](#schema-verifyauthoringresourcecommand) → Result [VerifyAuthoringResourceResult](#schema-verifyauthoringresourceresult)；target合法caller：blocks/workspace
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L62) → `handleVerifyAuthoringResource`（file）
- 接收入口：[handleVerifyAuthoringResource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L23)
- 已观察外部引用：workspace [verifyAuthoringDerivationResources](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_authoring_resource_verification.go#L19)
- 本topic入口/具体类型分支的Source审查索引：[handleVerifyAuthoringResource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L23)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceevents.CommandDestination` 分支谓词（[handleVerifyAuthoringResource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L23)）
- 特化承诺：再次读取真实资源并比较authoring资源事实，不以caller自报路径/hash替代owner验证；源资源不可被当deliverable随Workspace purge删除。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`event == nil || event.Source() != workspaceevents.CommandDestination`；`!ok || command.Validate() != nil`；`fact.WorkspaceRevision != command.WorkspaceRevision || fact.ContentHash != command.ContentHash || fact.Content != command.Content`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-acquireauthoringresourcecommand"></a>

### AcquireAuthoringResourceCommand

- 源码：[internal/modules/blocks/file/pkg/events/authoring_resources.go:18](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/authoring_resources.go#L18)
- 字段：`WorkspaceID common.WorkspaceID`；`AuthoringInputHash string`；`ResourceRef string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/authoring_resources.go#L24)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/authoring_resources.go#L41)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || len(c.WorkspaceID) > 256 || c.AuthoringInputHash == "" || len(c.AuthoringInputHash) > 256 || c.ResourceRef == "" || len(c.ResourceRef) > 1024 || !utf8.ValidString(c.ResourceRef) || strings.ContainsRune(c.ResourceRef, 0)`；`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.AuthoringInputHash != c.AuthoringInputHash || r.ResourceRef != c.ResourceRef || len(r.Content) > AuthoringResourceMaxBytes || int64(len(r.Content)) != r.Size || !utf8.ValidString(r.Content) || strings.ContainsRune(r.Content, 0)`；`r.ContentHash != "sha256:"+hex.EncodeToString(checksum[:])`

<a id="schema-acquireauthoringresourceresult"></a>

### AcquireAuthoringResourceResult

- 源码：[internal/modules/blocks/file/pkg/events/authoring_resources.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/authoring_resources.go#L31)
- 字段：`WorkspaceID common.WorkspaceID`；`WorkspaceRevision uint64`；`AuthoringInputHash string`；`ResourceRef string`；`Content string`；`ContentHash string`；`Size int64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/authoring_resources.go#L41)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.AuthoringInputHash != c.AuthoringInputHash || r.ResourceRef != c.ResourceRef || len(r.Content) > AuthoringResourceMaxBytes || int64(len(r.Content)) != r.Size || !utf8.ValidString(r.Content) || strings.ContainsRune(r.Content, 0)`；`r.ContentHash != "sha256:"+hex.EncodeToString(checksum[:])`

<a id="schema-acquireruntimecapabilitiescommand"></a>

### AcquireRuntimeCapabilitiesCommand

- 源码：[internal/modules/blocks/file/pkg/events/runtime_capability.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_capability.go#L5)
- 声明：`type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest`
- 字段引用类型：[RuntimeCapabilityAcquisitionRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L15)

<a id="schema-acquireruntimecapabilitiesresult"></a>

### AcquireRuntimeCapabilitiesResult

- 源码：[internal/modules/blocks/file/pkg/events/runtime_capability.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_capability.go#L7)
- 字段：`Acquisition runtimecontract.RuntimeCapabilityAcquisition`
- 字段引用类型：[RuntimeCapabilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L27)

<a id="schema-acquireruntimeoperationadmissibilitycommand"></a>

### AcquireRuntimeOperationAdmissibilityCommand

- 源码：[internal/modules/blocks/file/pkg/events/runtime_admissibility.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L5)
- 声明：`type AcquireRuntimeOperationAdmissibilityCommand runtimecontract.RuntimeOperationAdmissibilityRequest`
- 字段引用类型：[RuntimeOperationAdmissibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L14)
- 实际方法：[ValidateRuntimeOperationAdmissibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L88)

<a id="schema-acquireruntimeoperationadmissibilityresult"></a>

### AcquireRuntimeOperationAdmissibilityResult

- 源码：[internal/modules/blocks/file/pkg/events/runtime_admissibility.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L7)
- 字段：`Acquisition runtimecontract.RuntimeOperationAdmissibilityAcquisition`
- 字段引用类型：[RuntimeOperationAdmissibilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L83)

<a id="schema-acquireruntimetargetselectioncommand"></a>

### AcquireRuntimeTargetSelectionCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L31)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`PrerequisiteID string`；`WorkspaceDeliverableDirectory string`；`TargetRole string`；`TargetSelectorRef string`；`TargetSelectorOwnerRef string`；`TargetSelectorSubjectRef string`；`TargetSelectorCandidateRefs []string`；`ProspectiveTargetAllOfConstraintRefs []string`；`ProspectiveTargetRef string`；`MaxItems int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-acquireruntimetargetselectionresult"></a>

### AcquireRuntimeTargetSelectionResult

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:47](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L47)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`Selection runtimecontract.RuntimeTargetSelection`；`Observation runtimecontract.RuntimePrerequisiteObservation`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeTargetSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1470)；[RuntimePrerequisiteObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_prerequisite_freshness.go#L101)

<a id="schema-acquiretargetsnapshotcommand"></a>

### AcquireTargetSnapshotCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L11)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`PrerequisiteID string`；`WorkspaceDeliverableDirectory string`；`TargetRef string`；`MaxBytes int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-acquiretargetsnapshotresult"></a>

### AcquireTargetSnapshotResult

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:21](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L21)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`Snapshot runtimecontract.TargetSnapshot`；`Observation runtimecontract.RuntimePrerequisiteObservation`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[TargetSnapshot（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1431)；[RuntimePrerequisiteObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_prerequisite_freshness.go#L101)

<a id="schema-acquireworkspacemanifestcommand"></a>

### AcquireWorkspaceManifestCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:55](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L55)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`PrerequisiteID string`；`WorkspaceDeliverableDirectory string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-acquireworkspacemanifestresult"></a>

### AcquireWorkspaceManifestResult

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:63](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L63)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`Manifest runtimecontract.WorkspaceManifest`；`Observation runtimecontract.RuntimePrerequisiteObservation`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[WorkspaceManifest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1416)；[RuntimePrerequisiteObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_prerequisite_freshness.go#L101)

<a id="schema-confirmruntimerepairsurfacecommand"></a>

### ConfirmRuntimeRepairSurfaceCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:77](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L77)
- 字段：`RequestID string`；`IssuedAtStateRevision uint64`；`ExpectedSubjectRevision string`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`WorkspaceDeliverableDirectory string`；`Observation runtimecontract.RuntimeValidationFailureObservation`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeValidationFailureObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_repair_surface.go#L46)

<a id="schema-confirmruntimerepairsurfaceresult"></a>

### ConfirmRuntimeRepairSurfaceResult

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:87](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L87)
- 字段：`Confirmation runtimecontract.RuntimeRepairSurfaceConfirmation`
- 字段引用类型：[RuntimeRepairSurfaceConfirmation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_repair_surface.go#L142)

<a id="schema-executefileeditoperationcommand"></a>

### ExecuteFileEditOperationCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:146](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L146)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)

<a id="schema-executefilelistoperationcommand"></a>

### ExecuteFileListOperationCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:136](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L136)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)

<a id="schema-executefilereadoperationcommand"></a>

### ExecuteFileReadOperationCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:141](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L141)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)

<a id="schema-executefilewriteoperationcommand"></a>

### ExecuteFileWriteOperationCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:151](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L151)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)

<a id="schema-executeoperationresult"></a>

### ExecuteOperationResult

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:166](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L166)
- 字段：`Receipt runtimecontract.RuntimeOperationReceipt`
- 字段引用类型：[RuntimeOperationReceipt（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_receipt.go#L139)

<a id="schema-executepatchapplyoperationcommand"></a>

### ExecutePatchApplyOperationCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:156](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L156)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)

<a id="schema-executepatchpreviewoperationcommand"></a>

### ExecutePatchPreviewOperationCommand

- 源码：[internal/modules/blocks/file/pkg/events/messages.go:161](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go#L161)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`SealedOperation SealedRuntimeOperation`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)

<a id="schema-proberuntimeoperationfeasibilitycommand"></a>

### ProbeRuntimeOperationFeasibilityCommand

- 源码：[internal/modules/blocks/file/pkg/events/runtime_admissibility.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L11)
- 声明：`type ProbeRuntimeOperationFeasibilityCommand runtimecontract.RuntimeOperationFeasibilityRequest`
- 字段引用类型：[RuntimeOperationFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L253)
- 实际方法：[ValidateRuntimeOperationFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L47)

<a id="schema-proberuntimeoperationfeasibilityresult"></a>

### ProbeRuntimeOperationFeasibilityResult

- 源码：[internal/modules/blocks/file/pkg/events/runtime_admissibility.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L13)
- 声明：`type ProbeRuntimeOperationFeasibilityResult runtimecontract.RuntimeOperationFeasibilityObservation`
- 字段引用类型：[RuntimeOperationFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_admissibility_acquisition.go#L270)

<a id="schema-proberuntimesemanticprooffeasibilitycommand"></a>

### ProbeRuntimeSemanticProofFeasibilityCommand

- 源码：[internal/modules/blocks/file/pkg/events/runtime_admissibility.go:19](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L19)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityCommand runtimecontract.RuntimeSemanticProofFeasibilityRequest`
- 字段引用类型：[RuntimeSemanticProofFeasibilityRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L13)
- 实际方法：[ValidateRuntimeSemanticProofFeasibilityCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L23)

<a id="schema-proberuntimesemanticprooffeasibilityresult"></a>

### ProbeRuntimeSemanticProofFeasibilityResult

- 源码：[internal/modules/blocks/file/pkg/events/runtime_admissibility.go:21](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L21)
- 声明：`type ProbeRuntimeSemanticProofFeasibilityResult runtimecontract.RuntimeSemanticProofFeasibilityObservation`
- 字段引用类型：[RuntimeSemanticProofFeasibilityObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_contract.go#L24)

<a id="schema-queryruntimecwireconciliationcommand"></a>

### QueryRuntimeCWIReconciliationCommand

- 源码：[internal/modules/blocks/file/pkg/events/runtime_reconciliation.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_reconciliation.go#L5)
- 声明：`type QueryRuntimeCWIReconciliationCommand runtimecontract.RuntimeCWIOwnerReconciliationRequest`
- 字段引用类型：[RuntimeCWIOwnerReconciliationRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L371)
- 实际方法：[ValidateForOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_reconciliation.go#L11)

<a id="schema-queryruntimecwireconciliationresult"></a>

### QueryRuntimeCWIReconciliationResult

- 源码：[internal/modules/blocks/file/pkg/events/runtime_reconciliation.go:7](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_reconciliation.go#L7)
- 字段：`Observation runtimecontract.RuntimeCWIReconciliationObservation`
- 字段引用类型：[RuntimeCWIReconciliationObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_cwi_reconciliation.go#L467)

<a id="schema-queryruntimefilereaddependenciescommand"></a>

### QueryRuntimeFileReadDependenciesCommand

- 源码：[internal/modules/blocks/file/pkg/events/read_dependencies.go:6](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/read_dependencies.go#L6)
- 字段：`<embedded> runtimecontract.RuntimeOperationExecutionContext`；`RequestID string`；`IssuedAtStateRevision uint64`；`SealedOperation SealedRuntimeOperation`；`SourceReceiptHash string`；`OriginalDependencies runtimecontract.RuntimeOwnerRevisionSet`
- 字段引用类型：[RuntimeOperationExecutionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_execution.go#L31)；[SealedRuntimeOperation](#schema-sealedruntimeoperation)；[RuntimeOwnerRevisionSet（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_result_closeout_formal.go#L46)
- 实际方法：[ValidateRuntimeFileReadDependenciesResult](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/read_dependencies_validation.go#L12)
- 空值/identity/版本/范围原式：`command.RequestID == "" || command.RequestID != strings.TrimSpace(command.RequestID) || command.IssuedAtStateRevision == 0 || command.SourceReceiptHash == "" || command.SourceReceiptHash != strings.TrimSpace(command.SourceReceiptHash) || command.OriginalDependencies.DefinitionRevision != command.Transaction.DefinitionRevision || len(command.OriginalDependencies.Entries) != 1`；`!reflect.DeepEqual(command.OriginalDependencies, command.OriginalDependencies.Normalized())`；`err := command.OriginalDependencies.Validate(); err != nil`；`result.RequestID != command.RequestID || result.IssuedAtStateRevision != command.IssuedAtStateRevision || result.SourceReceiptHash != command.SourceReceiptHash`；`result.CurrentDependencies != nil`；`set == nil || set.DefinitionRevision != command.Transaction.DefinitionRevision || len(set.Entries) != 1`；`!reflect.DeepEqual(*set, set.Normalized())`；`err := set.Validate(); err != nil`；`len(command.OriginalDependencies.Entries) != 1`；`!reflect.DeepEqual(original, current)`
- 显式分支集合（只表列源码case，default语义见validator）：`RuntimeFileReadDependencyMissing`；`RuntimeFileReadDependencyObserved`

<a id="schema-queryruntimefilereaddependenciesresult"></a>

### QueryRuntimeFileReadDependenciesResult

- 源码：[internal/modules/blocks/file/pkg/events/read_dependencies.go:22](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/read_dependencies.go#L22)
- 字段：`RequestID string`；`IssuedAtStateRevision uint64`；`SourceReceiptHash string`；`Status RuntimeFileReadDependencyObservationStatus`；`CurrentDependencies *runtimecontract.RuntimeOwnerRevisionSet`
- 字段引用类型：[RuntimeFileReadDependencyObservationStatus](#schema-runtimefilereaddependencyobservationstatus)；[RuntimeOwnerRevisionSet（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_result_closeout_formal.go#L46)
- 实际方法：[ValidateRuntimeFileReadDependenciesResult](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/read_dependencies_validation.go#L12)
- 空值/identity/版本/范围原式：`command.RequestID == "" || command.RequestID != strings.TrimSpace(command.RequestID) || command.IssuedAtStateRevision == 0 || command.SourceReceiptHash == "" || command.SourceReceiptHash != strings.TrimSpace(command.SourceReceiptHash) || command.OriginalDependencies.DefinitionRevision != command.Transaction.DefinitionRevision || len(command.OriginalDependencies.Entries) != 1`；`!reflect.DeepEqual(command.OriginalDependencies, command.OriginalDependencies.Normalized())`；`err := command.OriginalDependencies.Validate(); err != nil`；`result.RequestID != command.RequestID || result.IssuedAtStateRevision != command.IssuedAtStateRevision || result.SourceReceiptHash != command.SourceReceiptHash`；`result.CurrentDependencies != nil`；`set == nil || set.DefinitionRevision != command.Transaction.DefinitionRevision || len(set.Entries) != 1`；`!reflect.DeepEqual(*set, set.Normalized())`；`err := set.Validate(); err != nil`；`len(command.OriginalDependencies.Entries) != 1`；`!reflect.DeepEqual(original, current)`
- 显式分支集合（只表列源码case，default语义见validator）：`RuntimeFileReadDependencyMissing`；`RuntimeFileReadDependencyObserved`

<a id="schema-runtimefilereaddependencyobservationstatus"></a>

### RuntimeFileReadDependencyObservationStatus

- 源码：[internal/modules/blocks/file/pkg/events/read_dependencies.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/read_dependencies.go#L15)
- 声明：`type RuntimeFileReadDependencyObservationStatus string`
- 源码枚举值：`RuntimeFileReadDependencyObserved = "observed"`；`RuntimeFileReadDependencyMissing = "missing"`

<a id="schema-sealedruntimeoperation"></a>

### SealedRuntimeOperation

- 源码：[internal/modules/blocks/file/pkg/events/operation.go:20](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/operation.go#L20)
- 字段：`Operation runtimecontract.RuntimeOperation`；`PayloadHash string`
- 字段引用类型：[RuntimeOperation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate.go#L395)
- 实际方法：[ValidateForExecution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/operation.go#L49)
- 空值/identity/版本/范围原式：`err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(context); err != nil`；`operation.Kind != strings.TrimSpace(expectedKind)`；`runtimecontract.RuntimeModuleForOperationKind(operation.Kind) != runtimecontract.RuntimeModuleFile`；`err := runtimecontract.ValidateRuntimeOperationContract(operation); err != nil`；`operation.ID != strings.TrimSpace(context.OperationID)`；`!sameExecutionStrings(operation.TargetRefs, context.TargetRefs) || !sameExecutionCoverage(operation.CoverageWorkItemIDs, context.CoverageWorkItemIDs)`；`err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, context.Execution, nil); err != nil`；`err != nil`；`len(actualHash) != len(expectedHash) || subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1`

<a id="schema-validateruntimeoperationcandidateconformancecommand"></a>

### ValidateRuntimeOperationCandidateConformanceCommand

- 源码：[internal/modules/blocks/file/pkg/events/runtime_admissibility.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L15)
- 声明：`type ValidateRuntimeOperationCandidateConformanceCommand runtimecontract.RuntimeOperationCandidateConformanceRequest`
- 字段引用类型：[RuntimeOperationCandidateConformanceRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_candidate_conformance.go#L277)
- 实际方法：[ValidateRuntimeOperationCandidateConformanceCommandValue](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L51)；[NewValidateRuntimeOperationCandidateConformanceResult](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L55)
- 空值/identity/版本/范围原式：`err != nil`；`err := observation.ValidateForRequest(request, request.OperationAuthorization.OwnerModuleID); err != nil`

<a id="schema-validateruntimeoperationcandidateconformanceresult"></a>

### ValidateRuntimeOperationCandidateConformanceResult

- 源码：[internal/modules/blocks/file/pkg/events/runtime_admissibility.go:17](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L17)
- 声明：`type ValidateRuntimeOperationCandidateConformanceResult runtimecontract.RuntimeOperationCandidateConformanceObservation`
- 字段引用类型：[RuntimeOperationCandidateConformanceObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_operation_candidate_conformance.go#L319)
- 实际方法：[NewValidateRuntimeOperationCandidateConformanceResult](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/runtime_admissibility.go#L55)
- 空值/identity/版本/范围原式：`err != nil`；`err := observation.ValidateForRequest(request, request.OperationAuthorization.OwnerModuleID); err != nil`

<a id="schema-verifyauthoringresourcecommand"></a>

### VerifyAuthoringResourceCommand

- 源码：[internal/modules/blocks/file/pkg/events/verify_authoring_resource.go:12](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/verify_authoring_resource.go#L12)
- 字段：`WorkspaceID common.WorkspaceID`；`WorkspaceRevision uint64`；`AuthoringInputHash string`；`ResourceRef string`；`Content string`；`ContentHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/verify_authoring_resource.go#L21)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/verify_authoring_resource.go#L35)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision != c.WorkspaceRevision || r.AuthoringInputHash != c.AuthoringInputHash || r.ResourceRef != c.ResourceRef || r.ContentHash != c.ContentHash`

<a id="schema-verifyauthoringresourceresult"></a>

### VerifyAuthoringResourceResult

- 源码：[internal/modules/blocks/file/pkg/events/verify_authoring_resource.go:27](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/verify_authoring_resource.go#L27)
- 字段：`WorkspaceID common.WorkspaceID`；`WorkspaceRevision uint64`；`AuthoringInputHash string`；`ResourceRef string`；`ContentHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/verify_authoring_resource.go#L35)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision != c.WorkspaceRevision || r.AuthoringInputHash != c.AuthoringInputHash || r.ResourceRef != c.ResourceRef || r.ContentHash != c.ContentHash`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [handleVerifyAuthoringResource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L23)：`event == nil || event.Source() != workspaceevents.CommandDestination`
  - helper完整分支：`func (m *Biz) handleVerifyAuthoringResource(event magicevent.Event, result magicevent.Result) { if result == nil { return } if event == nil || event.Source() != workspaceevents.CommandDestination { setPrerequisiteError(result, common.ErrInvalidArgument) return } command, ok := event.Data().(fileevents.VerifyAuthoringResourceCommand) if !ok || command.Validate() != nil { setPrerequisiteError(result, common.ErrInvalidArgument) return } query := fileevents.AcquireAuthoringResourceCommand{WorkspaceID: command.WorkspaceID, AuthoringInputHash: command.AuthoringInputHash, ResourceRef: command.ResourceRef} fact, err := m.acquireAuthoringResource(event, query) if setPrerequisiteError(result, err) { return } if fact.WorkspaceRevision != command.WorkspaceRevision || fact.ContentHash != command.ContentHash || fact.Content != command.Content { setPrerequisiteError(result, common.ErrConflict) return } result.Set(fileevents.VerifyAuthoringResourceResult{WorkspaceID: fact.WorkspaceID, WorkspaceRevision: fact.WorkspaceRevision, AuthoringInputHash: fact.AuthoringInputHash, ResourceRef: fact.ResourceRef, ContentHash: fact.ContentHash}, nil) }`
  - 当前来源常量：`workspaceevents.CommandDestination = "blocks/workspace"`
- [handleAcquireAuthoringResource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L48)：`event == nil || event.Source() != authoringApplicationSource`
  - helper完整分支：`func (m *Biz) handleAcquireAuthoringResource(event magicevent.Event, result magicevent.Result) { if result == nil { return } if event == nil || event.Source() != authoringApplicationSource { setPrerequisiteError(result, fmt.Errorf("%w: authoring File query requires the exact Application source", common.ErrInvalidArgument)) return } command, ok := event.Data().(fileevents.AcquireAuthoringResourceCommand) if !ok || command.Validate() != nil { setPrerequisiteError(result, common.ErrInvalidArgument) return } response, err := m.acquireAuthoringResource(event, command) if setPrerequisiteError(result, err) { return } result.Set(response, nil) }`
  - 当前来源常量：`authoringApplicationSource = "application/workorchd"`
- [requireOrchestratorRuntimeSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1114)：`event != nil && event.Source() == runtimecontract.RuntimeModuleOrchestrator`
  - helper完整分支：`func requireOrchestratorRuntimeSource(event magicevent.Event, result magicevent.Result) bool { if event != nil && event.Source() == runtimecontract.RuntimeModuleOrchestrator { return true } setPrerequisiteError(result, fmt.Errorf("%w: runtime File command requires the exact Orchestrator source", common.ErrInvalidArgument)) return false }`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`
- [requireWorkspaceManifestSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1122)：`event != nil && (event.Source() == runtimecontract.RuntimeModuleOrchestrator || event.Source() == runtimecontract.RuntimeModuleGit)`
  - helper完整分支：`func requireWorkspaceManifestSource(event magicevent.Event, result magicevent.Result) bool { if event != nil && (event.Source() == runtimecontract.RuntimeModuleOrchestrator || event.Source() == runtimecontract.RuntimeModuleGit) { return true } setPrerequisiteError(result, fmt.Errorf("%w: workspace manifest command requires the exact Orchestrator or Git owner source", common.ErrInvalidArgument)) return false }`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`；`runtimecontract.RuntimeModuleGit = "blocks/git"`
- [handleRuntimeFileReadDependenciesQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/file_read_dependency_query.go#L15)：`event == nil || event.Source() != runtimecontract.RuntimeModuleOrchestrator`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`；`ModuleID = "blocks/file"`