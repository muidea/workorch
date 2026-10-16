# workspace：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页51个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

源码已在kernel/workspace，当前角色为Module；wire ModuleID仍为blocks/workspace。此次目录迁移保留workspace能力/事实owner身份、topic和存储identity，不能因为搬目录改变历史scope。Workspace拥有Definition创作、精确确认/物化、active-slot和唯一删除journal；不直接改Run/Result。

现有Create/Authoring/Specification/Admission消息属于NC00的源基础，不证明完整Web-only创建或M3已实现。外部创建只能由Web经workorchd进入Workspace，TUI/CLI创建入口移除；已有后台typed创建能力不向任意caller开放。M3 readiness与auto grant/撤销、initialactivation、post-seal变更、安全handoff仅在NC00–NC05定义，本页逐CT列source与target差分。

已封存义务不得通过UpdateGoal/DoD等旧CRUD原位变更；这些入口对sealed Definition必须拒绝并引导NC04 ticket。新候选改变后旧confirm/approve失效。source自有revision/CAS/input hash只证明各owner局部承诺；整组materialized seal、active-slot和Run generation不能从单次Save成功推断。

## 逐topic合同

<a id="ct-workspace-doddefined"></a>

### CT-workspace-DoDDefined

- 边界：`TopicDoDDefined` = `/dod/defined`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L471)
- 唯一事实owner/发布者：workspace；Data [DoDDefinedData](#schema-doddefineddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeWorkspaceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L400) → `subscribeWorkspaceNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[publishDoDEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/goal_dod_manager.go#L551)
- 已观察外部引用：eventstream [subscribeWorkspaceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L400)；eventstream [dodRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L947)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-workspace-goaldefined"></a>

### CT-workspace-GoalDefined

- 边界：`TopicGoalDefined` = `/goal/defined`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L470)
- 唯一事实owner/发布者：workspace；Data [GoalDefinedData](#schema-goaldefineddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeWorkspaceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L400) → `subscribeWorkspaceNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[publishGoalEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/goal_dod_manager.go#L397)
- 已观察外部引用：eventstream [subscribeWorkspaceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L400)；eventstream [goalRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L933)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-workspace-workspaceauthoringadmit"></a>

### CT-workspace-WorkspaceAuthoringAdmit

- 边界：`TopicWorkspaceAuthoringAdmit` = `/workspace/definition-authoring/admit`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_admission.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L459)
- 接收provider：workspace；Command [AdmitDefinitionAuthoringCommand](#schema-admitdefinitionauthoringcommand) → Result [AdmitDefinitionAuthoringResult](#schema-admitdefinitionauthoringresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [sendAuthoringAdmission](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_admission.go#L192)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspaceauthoringconfirm"></a>

### CT-workspace-WorkspaceAuthoringConfirm

- 边界：`TopicWorkspaceAuthoringConfirm` = `/workspace/definition-authoring/confirm`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_confirmation.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L458)
- 接收provider：workspace；Command [ConfirmDefinitionAuthoringCommand](#schema-confirmdefinitionauthoringcommand) → Result [ConfirmDefinitionAuthoringResult](#schema-confirmdefinitionauthoringresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [ConfirmAndAdmitDefinitionAuthoring](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_admission.go#L23)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspaceauthoringdiagnosticquery"></a>

### CT-workspace-WorkspaceAuthoringDiagnosticQuery

- 边界：`TopicWorkspaceAuthoringDiagnosticQuery` = `/workspace/definition-authoring/diagnostic/query`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L447)
- 接收provider：workspace；Command [QueryAuthoringDiagnosticCommand](#schema-queryauthoringdiagnosticcommand) → Result [QueryAuthoringDiagnosticResult](#schema-queryauthoringdiagnosticresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [QueryAuthoringDiagnostic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring.go#L176)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`command.Validate() != nil || m == nil || m.workspaces == nil || m.manager == nil || ctx == nil`；`w.ID != command.WorkspaceID || w.ValidateDefinitionAuthoringHistory() != nil`；`w.DefinitionAuthoring.Input.WorkspaceID != command.WorkspaceID`；`request.PermitID != command.PermitID`；`err := result.ValidateFor(command); err != nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspaceauthoringresourcescopequery"></a>

### CT-workspace-WorkspaceAuthoringResourceScopeQuery

- 边界：`TopicWorkspaceAuthoringResourceScopeQuery` = `/workspace/definition-authoring/resource-scope/query`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L45)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L418)
- 接收provider：workspace；Command [QueryAuthoringResourceScopeCommand](#schema-queryauthoringresourcescopecommand) → Result [QueryAuthoringResourceScopeResult](#schema-queryauthoringresourcescoperesult)；target合法caller：blocks/file
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：file [authoringWorkspaceScope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/authoring_resources.go#L136)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != "blocks/file"` 分支谓词（[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)）
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != "blocks/file"`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspaceauthoringspecificationcomplete"></a>

### CT-workspace-WorkspaceAuthoringSpecificationComplete

- 边界：`TopicWorkspaceAuthoringSpecificationComplete` = `/workspace/definition-authoring/specification/complete`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L453)
- 接收provider：workspace；Command [CompleteAuthoringSpecificationCommand](#schema-completeauthoringspecificationcommand) → Result [AuthoringSpecificationRequestResult](#schema-authoringspecificationrequestresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [settleAuthoringSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_settlement.go#L83)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspaceauthoringspecificationreserve"></a>

### CT-workspace-WorkspaceAuthoringSpecificationReserve

- 边界：`TopicWorkspaceAuthoringSpecificationReserve` = `/workspace/definition-authoring/specification/reserve`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L452)
- 接收provider：workspace；Command [ReserveAuthoringSpecificationCommand](#schema-reserveauthoringspecificationcommand) → Result [AuthoringSpecificationRequestResult](#schema-authoringspecificationrequestresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [completeAuthoringSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_specification.go#L232)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspaceauthoringsuccessauditquery"></a>

### CT-workspace-WorkspaceAuthoringSuccessAuditQuery

- 边界：`TopicWorkspaceAuthoringSuccessAuditQuery` = `/workspace/definition-authoring/success-byte-audit/query`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_success_audit.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L446)
- 接收provider：workspace；Command [QueryAuthoringSuccessAuditCommand](#schema-queryauthoringsuccessauditcommand) → Result [QueryAuthoringSuccessAuditResult](#schema-queryauthoringsuccessauditresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [QueryAuthoringSuccessAudit](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_success_audit.go#L15)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != definitionSpecificationCommandSource` 分支谓词（[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)）
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != definitionSpecificationCommandSource`；`c.Validate() != nil || m == nil || m.workspaces == nil || m.manager == nil || ctx == nil`；`w.ID != c.WorkspaceID || w.DefinitionAuthoring == nil || w.DefinitionAuthoring.Validate() != nil || w.ValidateDefinitionAuthoringHistory() != nil`；`r.ValidateFor(c) != nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacecommandcreate"></a>

### CT-workspace-WorkspaceCommandCreate

- 边界：`TopicWorkspaceCommandCreate` = `/workspace/command/create`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L416)
- 接收provider：workspace；Command [CreateWorkspaceCommand](#schema-createworkspacecommand) → Result [WorkspaceResult](#schema-workspaceresult)；target合法caller：workorchd（Web创建/已授权定义和执行用例）
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [CreateWorkspaceRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/workspaceports/workspace.go#L31)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：target change：仅workorchd的公开Web创建用例可进入，source存在Create不是NC00完整实现；必须建立稳定request journal，M3前后Run=0。其他caller无创建授权。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := m.manager.CreateWorkspace(ctx, workspaceevents.CreateWorkspaceCommand{ WorkspaceID: cmd.WorkspaceID, Title: cmd.Title, Description: cmd.Description, DeliverableDirectory: cmd.DeliverableDirectory, InternalDirectory: cmd.InternalDirectory, }); err != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacecommanddefinedod"></a>

### CT-workspace-WorkspaceCommandDefineDoD

- 边界：`TopicWorkspaceCommandDefineDoD` = `/workspace/command/define-dod`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L23)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L428)
- 接收provider：workspace；Command [DefineDoDCommand](#schema-definedodcommand) → Result [DoDResult](#schema-dodresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [DefineDoD](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/definitionports/dod_commands.go#L23)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS03](../Workorch_组件固定合同与分层验收规范.md#cs03)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || m.dods == nil`；`err := m.manager.DefineDoD(scopedCtx, cmd); err != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacecommanddefinegoal"></a>

### CT-workspace-WorkspaceCommandDefineGoal

- 边界：`TopicWorkspaceCommandDefineGoal` = `/workspace/command/define-goal`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L18)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L423)
- 接收provider：workspace；Command [DefineGoalCommand](#schema-definegoalcommand) → Result [GoalResult](#schema-goalresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [DefineGoal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/definitionports/goal_commands.go#L23)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS03](../Workorch_组件固定合同与分层验收规范.md#cs03)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || m.goals == nil`；`err := m.manager.DefineGoal(scopedCtx, cmd); err != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacecommanddelete"></a>

### CT-workspace-WorkspaceCommandDelete

- 边界：`TopicWorkspaceCommandDelete` = `/workspace/command/delete`；CF12；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L17)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L422)
- 接收provider：workspace；Command [DeleteWorkspaceCommand](#schema-deleteworkspacecommand) → Result [DeleteWorkspaceResult](#schema-deleteworkspaceresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [DeleteWorkspaceRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/workspaceports/workspace.go#L108)
- 本topic入口/具体类型分支的Source审查索引：[deleteWorkspace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/workspace_commands.go#L215)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceDeletionIngressSource` 分支谓词（[deleteWorkspace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/workspace_commands.go#L215)）
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`ev == nil || ev.Source() != workspaceDeletionIngressSource`；`err := cmd.Validate(); err != nil`；`err := response.ValidateFor(cmd); err != nil`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-workspace-workspacecommanddeletedod"></a>

### CT-workspace-WorkspaceCommandDeleteDoD

- 边界：`TopicWorkspaceCommandDeleteDoD` = `/workspace/command/delete-dod`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L25)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L430)
- 接收provider：workspace；Command [DeleteDoDCommand](#schema-deletedodcommand) → Result [DeleteDoDResult](#schema-deletedodresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [DeleteDoD](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/definitionports/dod_commands.go#L68)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS03](../Workorch_组件固定合同与分层验收规范.md#cs03)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.DoDID == "" || m.dods == nil`；`err := m.manager.DeleteDoD(scopedCtx, cmd); err != nil`；`!ok || response.ValidateFor(command) != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacecommanddeletegoal"></a>

### CT-workspace-WorkspaceCommandDeleteGoal

- 边界：`TopicWorkspaceCommandDeleteGoal` = `/workspace/command/delete-goal`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L20)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L425)
- 接收provider：workspace；Command [DeleteGoalCommand](#schema-deletegoalcommand) → Result [DeleteGoalResult](#schema-deletegoalresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [DeleteGoal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/definitionports/goal_commands.go#L68)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS03](../Workorch_组件固定合同与分层验收规范.md#cs03)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.GoalID == "" || m.goals == nil`；`err := m.manager.DeleteGoal(scopedCtx, cmd); err != nil`；`!ok || response.ValidateFor(command) != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacecommandexecutionintent"></a>

### CT-workspace-WorkspaceCommandExecutionIntent

- 边界：`TopicWorkspaceCommandExecutionIntent` = `/workspace/command/execution-intent`；CF02；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L41)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L439)
- 接收provider：workspace；Command [WorkspaceExecutionIntentCommand](#schema-workspaceexecutionintentcommand) → Result [WorkspaceExecutionIntentResult](#schema-workspaceexecutionintentresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleExecutionIntentCommand`（workspace）
- 接收入口：[handleExecutionIntentCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/execution_intent.go#L35)
- 已观察外部引用：workorchd [submit](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/workspace_command_gateway.go#L25)
- 本topic入口/具体类型分支的Source审查索引：[prepareWorkspaceExecutionIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/execution_intent.go#L87)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceExecutionIngressSource` 分支谓词（[prepareWorkspaceExecutionIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/execution_intent.go#L87)）
- 特化承诺：[CS10](../Workorch_组件固定合同与分层验收规范.md#cs10)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`err := response.Validate(); err != nil`；`!ok || ev.Source() != workspaceExecutionIngressSource`；`err := cmd.Validate(); err != nil`；`cmd.Kind == runtimecontract.RuntimeExecutionIntentResume`；`err := validateRestartExecutionDefinition(cmd, goal, dod, definitionRevision); err != nil`；`cmd.Kind != runtimecontract.RuntimeExecutionIntentRestart`
- 验收适用集：[CP06](../Workorch_组件固定合同与分层验收规范.md#cp06)

<a id="ct-workspace-workspacecommandget"></a>

### CT-workspace-WorkspaceCommandGet

- 边界：`TopicWorkspaceCommandGet` = `/workspace/command/get`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L417)
- 接收provider：workspace；Command [GetWorkspaceCommand](#schema-getworkspacecommand) → Result [WorkspaceResult](#schema-workspaceresult)；target合法caller：workorchd、orchestrator、run、system、blocks/shell
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [ReadWorkspaceByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/workspace_reads.go#L15)；orchestrator [Read](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/state_event_adapter.go#L56)；run [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_event_reader.go#L21)；shell [handleRuntimeValidationRawOutput](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/validation_raw_output_query.go#L17)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || m.workspaces == nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacecommandgetdod"></a>

### CT-workspace-WorkspaceCommandGetDoD

- 边界：`TopicWorkspaceCommandGetDoD` = `/workspace/command/get-dod`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L26)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L431)
- 接收provider：workspace；Command [GetDoDCommand](#schema-getdodcommand) → Result [DoDResult](#schema-dodresult)；target合法caller：workorchd、orchestrator、run、system、kernel/governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [ReadDoDByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/workspace_dod_reads.go#L14)；governance [GetDoD](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_readers.go#L154)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.DoDID == "" || m.dods == nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacecommandgetgoal"></a>

### CT-workspace-WorkspaceCommandGetGoal

- 边界：`TopicWorkspaceCommandGetGoal` = `/workspace/command/get-goal`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L21)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L426)
- 接收provider：workspace；Command [GetGoalCommand](#schema-getgoalcommand) → Result [GoalResult](#schema-goalresult)；target合法caller：workorchd、orchestrator、run、system、kernel/governance
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [ReadGoalByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/workspace_definition_reads.go#L14)；governance [GetGoal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/event_readers.go#L136)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || cmd.GoalID == "" || m.goals == nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacecommandlist"></a>

### CT-workspace-WorkspaceCommandList

- 边界：`TopicWorkspaceCommandList` = `/workspace/command/list`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L419)
- 接收provider：workspace；Command [ListWorkspacesCommand](#schema-listworkspacescommand) → Result [WorkspaceListResult](#schema-workspacelistresult)；target合法caller：workorchd、orchestrator、run、system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [ListWorkspacesByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/workspace_reads.go#L32)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || response.ValidateFor(command) != nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacecommandlistdodsbyws"></a>

### CT-workspace-WorkspaceCommandListDoDsByWS

- 边界：`TopicWorkspaceCommandListDoDsByWS` = `/workspace/command/list-dods-by-workspace`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L27)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L432)
- 接收provider：workspace；Command [ListDoDsByWorkspaceCommand](#schema-listdodsbyworkspacecommand) → Result [DoDListResult](#schema-dodlistresult)；target合法caller：workorchd、orchestrator、run、system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [ListDoDsByWorkspaceOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/workspace_dod_reads.go#L36)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || m.dods == nil`；`!ok || response.ValidateFor(command) != nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacecommandlistgoalsbyws"></a>

### CT-workspace-WorkspaceCommandListGoalsByWS

- 边界：`TopicWorkspaceCommandListGoalsByWS` = `/workspace/command/list-goals-by-workspace`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L22)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L427)
- 接收provider：workspace；Command [ListGoalsByWorkspaceCommand](#schema-listgoalsbyworkspacecommand) → Result [GoalListResult](#schema-goallistresult)；target合法caller：workorchd、orchestrator、run、system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [ListGoalsByWorkspaceOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/workspace_definition_reads.go#L36)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || m.goals == nil`；`!ok || response.ValidateFor(command) != nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacecommandupdate"></a>

### CT-workspace-WorkspaceCommandUpdate

- 边界：`TopicWorkspaceCommandUpdate` = `/workspace/command/update`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L15)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L420)
- 接收provider：workspace；Command [UpdateWorkspaceCommand](#schema-updateworkspacecommand) → Result [WorkspaceResult](#schema-workspaceresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [UpdateWorkspaceRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/workspaceports/workspace.go#L51)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || m.workspaces == nil || soulbinding.Validate(cmd.SoulBindings) != nil`；`cmd.TemplateBindingHash != "" && (cmd.TemplateID != current.TemplateID || cmd.TemplateBindingHash != current.TemplateBindingHash)`；`workspacemodel.WorkspaceTemplateSnapshotHash(template) != cmd.TemplateBindingHash`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacecommandupdatedod"></a>

### CT-workspace-WorkspaceCommandUpdateDoD

- 边界：`TopicWorkspaceCommandUpdateDoD` = `/workspace/command/update-dod`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L24)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L429)
- 接收provider：workspace；Command [UpdateDoDCommand](#schema-updatedodcommand) → Result [DoDResult](#schema-dodresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [UpdateDoD](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/definitionports/dod_commands.go#L44)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS03](../Workorch_组件固定合同与分层验收规范.md#cs03)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || m.dods == nil`；`err := m.manager.UpdateDoD(scopedCtx, cmd); err != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacecommandupdategoal"></a>

### CT-workspace-WorkspaceCommandUpdateGoal

- 边界：`TopicWorkspaceCommandUpdateGoal` = `/workspace/command/update-goal`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L19)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L424)
- 接收provider：workspace；Command [UpdateGoalCommand](#schema-updategoalcommand) → Result [GoalResult](#schema-goalresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [UpdateGoal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/definitionports/goal_commands.go#L44)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS03](../Workorch_组件固定合同与分层验收规范.md#cs03)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || m.goals == nil`；`err := m.manager.UpdateGoal(scopedCtx, cmd); err != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacecommandupdatellmselection"></a>

### CT-workspace-WorkspaceCommandUpdateLLMSelection

- 边界：`TopicWorkspaceCommandUpdateLLMSelection` = `/workspace/command/update-llm-selection`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L16)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L421)
- 接收provider：workspace；Command [UpdateWorkspaceLLMSelectionCommand](#schema-updateworkspacellmselectioncommand) → Result [WorkspaceLLMSelectionResult](#schema-workspacellmselectionresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [UpdateWorkspaceLLMSelection](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/workspaceports/workspace.go#L79)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.WorkspaceID == "" || m.workspaces == nil`；`cmd.ExpectedRevision == 0 || item.Revision != cmd.ExpectedRevision`；`cmd.Selection == nil`；`err := selection.Validate(); err != nil`；`err := m.workspaces.CompareAndSwap(ctx, item, cmd.ExpectedRevision); err != nil`；`!ok || response.ValidateFor(command) != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacecreated"></a>

### CT-workspace-WorkspaceCreated

- 边界：`TopicWorkspaceCreated` = `/workspace/created`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L466)
- 唯一事实owner/发布者：workspace；Data [WorkspaceChangedData](#schema-workspacechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeWorkspaceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L400) → `subscribeWorkspaceNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：workorchd [isEventStreamCriticalTopic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/server_stream.go#L74)；workorchd [ValidWorkorchEventRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/observed/event_record.go#L15)；eventstream [subscribeWorkspaceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L400)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-workspace-workspacedefinitionadmissionintent"></a>

### CT-workspace-WorkspaceDefinitionAdmissionIntent

- 边界：`TopicWorkspaceDefinitionAdmissionIntent` = `/workspace/definition-admission/intent`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L43)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L441)
- 接收provider：workspace；Command [DefinitionAdmissionIntentCommand](#schema-definitionadmissionintentcommand) → Result [DefinitionAdmissionIntentResult](#schema-definitionadmissionintentresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionAdmissionCommand`（workspace）
- 接收入口：[handleDefinitionAdmissionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_admission.go#L24)
- 已观察外部引用：workorchd [SubmitDefinitionAdmissionIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/workspaceports/definition_admission.go#L21)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionAdmissionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_admission.go#L24)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != definitionAdmissionCommandSource` 分支谓词（[handleDefinitionAdmissionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_admission.go#L24)）
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`m == nil || m.workspaces == nil || ev == nil || ev.Source() != definitionAdmissionCommandSource`；`current == nil || candidate.WorkspaceID != current.ID`；`err := ledger.Validate(current.ID); err != nil`；`!command.CurrentTemplate`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacedefinitionauthoringcreate"></a>

### CT-workspace-WorkspaceDefinitionAuthoringCreate

- 边界：`TopicWorkspaceDefinitionAuthoringCreate` = `/workspace/definition-authoring/create`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L46)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L457)
- 接收provider：workspace；Command [CreateDefinitionAuthoringCommand](#schema-createdefinitionauthoringcommand) → Result [CreateDefinitionAuthoringResult](#schema-createdefinitionauthoringresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [CreateDefinitionAuthoringView](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_creation.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：target change：NC00 exact输入→authoring journal入口；stable request/input hash、workspace revision和失败恢复均成立后才生成候选。不能以Create成功等同M3或自动Start。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`cmd.Validate() != nil || m == nil || m.manager == nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacedefinitionauthoringquery"></a>

### CT-workspace-WorkspaceDefinitionAuthoringQuery

- 边界：`TopicWorkspaceDefinitionAuthoringQuery` = `/workspace/definition-authoring/query`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L44)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L445)
- 接收provider：workspace；Command [QueryDefinitionAuthoringCommand](#schema-querydefinitionauthoringcommand) → Result [QueryDefinitionAuthoringResult](#schema-querydefinitionauthoringresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [QueryDefinitionAuthoring](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring.go#L15)；workorchd [EnsureWorkspaceDefinitionAdmission](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/activationports/definition_admission.go#L38)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || m.manager == nil || m.workspaces == nil || ctx == nil || cmd.WorkspaceID == ""`；`cmd.InputHash != ""`；`w.DefinitionAuthoring != nil && w.DefinitionAuthoring.InputHash == cmd.InputHash`；`c := w.DefinitionChangeCandidate; c != nil && c.Record.InputHash == cmd.InputHash`；`err := result.ValidateFor(cmd); err != nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacedefinitionauthoringsave"></a>

### CT-workspace-WorkspaceDefinitionAuthoringSave

- 边界：`TopicWorkspaceDefinitionAuthoringSave` = `/workspace/definition-authoring/save`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L47)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L444)
- 接收provider：workspace；Command [SaveDefinitionAuthoringCommand](#schema-savedefinitionauthoringcommand) → Result [SaveDefinitionAuthoringResult](#schema-savedefinitionauthoringresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [SaveDefinitionAuthoring](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring.go#L74)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacedefinitionchangeprepare"></a>

### CT-workspace-WorkspaceDefinitionChangePrepare

- 边界：`TopicWorkspaceDefinitionChangePrepare` = `/workspace/definition-change/prepare`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L442)
- 接收provider：workspace；Command [PrepareDefinitionChangeCommand](#schema-preparedefinitionchangecommand) → Result [PrepareDefinitionChangeResult](#schema-preparedefinitionchangeresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [PrepareDefinitionChange](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_change.go#L19)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：source新增独立候选准备：共享handler顶端Source guard对此Prepare化简为只允许application/workorchd；DefinitionChangeQuery、ResourceScopeQuery、DiagnosticQuery的例外均不适用。再从Decision exact proposal身份反查原goal/criterion差异；保留原seal/正式Goal/DoD，expected revision+1，重复请求返回原candidate；拒绝Run历史/未结算permit，继承permit、unknown、失败及有界预算。target尚需NC04 final approve、runtime matching、formal ACK、adoption与materialized seal，不将prepare冒充采纳。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`inputHash == ""`；`c == nil || c.Record.InputHash != inputHash`；`c.Validate(w) != nil || workspaceDeletionRejectsMutation(w) != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacedefinitionchangequery"></a>

### CT-workspace-WorkspaceDefinitionChangeQuery

- 边界：`TopicWorkspaceDefinitionChangeQuery` = `/workspace/definition-change/query`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L443)
- 接收provider：workspace；Command [QueryDefinitionChangeCommand](#schema-querydefinitionchangecommand) → Result [QueryDefinitionChangeResult](#schema-querydefinitionchangeresult)；target合法caller：application/workorchd、blocks/decision
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [QueryDefinitionChange](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_change.go#L100)；decision [verifyContractChangeFinalCandidate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/contract_change_candidate.go#L16)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：source新增只读：Candidate可空；Candidate空时Reviewed必须空；候选存在则Query响应按原Prepare身份再校验，Reviewed须与owner持久候选计算的reviewed identity相等。查询不批准、不应用、不产生Run。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`m == nil || m.manager == nil || m.workspaces == nil || ctx == nil || cmd.WorkspaceID == ""`；`c.Validate(w) != nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacedeleted"></a>

### CT-workspace-WorkspaceDeleted

- 边界：`TopicWorkspaceDeleted` = `/workspace/deleted`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L469)
- 唯一事实owner/发布者：workspace；Data [WorkspaceDeletedData](#schema-workspacedeleteddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53) → `handleWorkspaceDeleted`（workorchd）
- 已读事实来源引用（含发布/分派路径）：[publishWorkspaceDeleted](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/goal_dod_manager.go#L373)
- 已观察外部引用：workorchd [workspaceDeletedStreamEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/event_subscriptions.go#L121)；workorchd [isEventStreamCriticalTopic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/server_stream.go#L74)；workorchd [Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53)；process/workorch [Subscribe](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L85)；process/workorch [workspaceDeletedBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L250)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-workspace-workspacederivationcomplete"></a>

### CT-workspace-WorkspaceDerivationComplete

- 边界：`TopicWorkspaceDerivationComplete` = `/workspace/definition-derivation/complete`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L451)
- 接收provider：workspace；Command [CompleteDefinitionDerivationCommand](#schema-completedefinitionderivationcommand) → Result [DefinitionDerivationRequestResult](#schema-definitionderivationrequestresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [settleDerivation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_settlement.go#L16)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacederivationreserve"></a>

### CT-workspace-WorkspaceDerivationReserve

- 边界：`TopicWorkspaceDerivationReserve` = `/workspace/definition-derivation/reserve`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L450)
- 接收provider：workspace；Command [ReserveDefinitionDerivationCommand](#schema-reservedefinitionderivationcommand) → Result [DefinitionDerivationRequestResult](#schema-definitionderivationrequestresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [deriveDefinitionAcceptance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_derivation.go#L23)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacefactterminaldelivery"></a>

### CT-workspace-WorkspaceFactTerminalDelivery

- 边界：`TopicWorkspaceFactTerminalDelivery` = `/workspace/fact/terminal-delivery`；CF10；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L42)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L440)
- 接收provider：workspace；Command [RuntimeTerminalDeliveryFact](#schema-runtimeterminaldeliveryfact) → Result [WorkspaceLifecycleCommandResult](#schema-workspacelifecyclecommandresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleLifecycleCommand`（workspace）
- 接收入口：[handleLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/lifecycle_handler.go#L22)
- 已观察外部引用：orchestrator [deliverRuntimeWorkspace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_result_projection.go#L163)
- 本topic入口/具体类型分支的Source审查索引：[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/lifecycle_handler.go#L38)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != "kernel/orchestrator"` 分支谓词（[executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/lifecycle_handler.go#L38)）
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || ev.Source() != "kernel/orchestrator"`
- 验收适用集：[CP04](../Workorch_组件固定合同与分层验收规范.md#cp04)

<a id="ct-workspace-workspacepreparationdiagnosticquery"></a>

### CT-workspace-WorkspacePreparationDiagnosticQuery

- 边界：`TopicWorkspacePreparationDiagnosticQuery` = `/workspace/definition-authoring/preparation-diagnostic/query`；CF01；source query / target change；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L449)
- 接收provider：workspace；Command [QueryPreparationDiagnosticCommand](#schema-querypreparationdiagnosticcommand) → Result [QueryPreparationDiagnosticResult](#schema-querypreparationdiagnosticresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [QueryPreparationDiagnostic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring.go#L149)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`cmd.Validate() != nil || m == nil || m.manager == nil || m.workspaces == nil || ctx == nil`；`w.ID != cmd.WorkspaceID`；`result.ValidateFor(cmd) != nil`
- 验收适用集：[CP08](../Workorch_组件固定合同与分层验收规范.md#cp08)

<a id="ct-workspace-workspacepreparationdiagnosticrecord"></a>

### CT-workspace-WorkspacePreparationDiagnosticRecord

- 边界：`TopicWorkspacePreparationDiagnosticRecord` = `/workspace/definition-authoring/preparation-diagnostic/record`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L448)
- 接收provider：workspace；Command [RecordPreparationDiagnosticCommand](#schema-recordpreparationdiagnosticcommand) → Result [RecordPreparationDiagnosticResult](#schema-recordpreparationdiagnosticresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [RecordPreparationDiagnostic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring.go#L122)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacespecificationbegin"></a>

### CT-workspace-WorkspaceSpecificationBegin

- 边界：`TopicWorkspaceSpecificationBegin` = `/workspace/definition-specification/begin`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L48)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L454)
- 接收provider：workspace；Command [BeginDefinitionSpecificationCommand](#schema-begindefinitionspecificationcommand) → Result [BeginDefinitionSpecificationResult](#schema-begindefinitionspecificationresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [BeginDefinitionSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_specification_ledger.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacespecificationcomplete"></a>

### CT-workspace-WorkspaceSpecificationComplete

- 边界：`TopicWorkspaceSpecificationComplete` = `/workspace/definition-specification/complete`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L50)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L456)
- 接收provider：workspace；Command [CompleteDefinitionSpecificationRequestCommand](#schema-completedefinitionspecificationrequestcommand) → Result [DefinitionSpecificationRequestResult](#schema-definitionspecificationrequestresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [persistDefinitionSpecificationCompletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_specification_ledger.go#L85)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := cmd.Validate(); err != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacespecificationreserve"></a>

### CT-workspace-WorkspaceSpecificationReserve

- 边界：`TopicWorkspaceSpecificationReserve` = `/workspace/definition-specification/reserve`；CF01；source existing / target change；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L49)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L455)
- 接收provider：workspace；Command [ReserveDefinitionSpecificationRequestCommand](#schema-reservedefinitionspecificationrequestcommand) → Result [DefinitionSpecificationRequestResult](#schema-definitionspecificationrequestresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleDefinitionSpecificationCommand`（workspace）
- 接收入口：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)
- 已观察外部引用：workorchd [reserveDefinitionSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_specification_ledger.go#L55)
- 本topic入口/具体类型分支的Source审查索引：[handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS12](../Workorch_组件固定合同与分层验收规范.md#cs12)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := cmd.Validate(); err != nil`
- 验收适用集：[CP01](../Workorch_组件固定合同与分层验收规范.md#cp01)

<a id="ct-workspace-workspacestatechanged"></a>

### CT-workspace-WorkspaceStateChanged

- 边界：`TopicWorkspaceStateChanged` = `/workspace/state/changed`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L466)
- 唯一事实owner/发布者：workspace；Data [WorkspaceChangedData](#schema-workspacechangeddata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeWorkspaceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L400) → `subscribeWorkspaceNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[publishWorkspaceUpdated](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L272)
- 已观察外部引用：workorchd [enrichSSEEnvelope](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/sse_enrichment.go#L15)；workorchd [isEventStreamCriticalTopic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/service/server/server_stream.go#L74)；workorchd [ValidWorkorchEventRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/observed/event_record.go#L15)；eventstream [subscribeWorkspaceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L400)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-workspace-workspacetemplatecommandcreate"></a>

### CT-workspace-WorkspaceTemplateCommandCreate

- 边界：`TopicWorkspaceTemplateCommandCreate` = `/workspace-template/command/create`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L29)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L433)
- 接收provider：workspace；Command [CreateWorkspaceTemplateCommand](#schema-createworkspacetemplatecommand) → Result [WorkspaceTemplateResult](#schema-workspacetemplateresult)；target合法caller：workorchd（Web创建/已授权定义和执行用例）
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [CreateWorkspaceTemplateRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/workspaceports/template_records.go#L25)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workspace-workspacetemplatecommanddelete"></a>

### CT-workspace-WorkspaceTemplateCommandDelete

- 边界：`TopicWorkspaceTemplateCommandDelete` = `/workspace-template/command/delete`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L33)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L437)
- 接收provider：workspace；Command [DeleteWorkspaceTemplateCommand](#schema-deleteworkspacetemplatecommand) → Result [DeleteWorkspaceTemplateResult](#schema-deleteworkspacetemplateresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [DeleteWorkspaceTemplateRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/workspaceports/template_records.go#L53)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.TemplateID == "" || m.templates == nil`；`parentID == cmd.TemplateID`；`err := m.templates.Delete(ctx, cmd.TemplateID); err != nil`；`!ok || response.ValidateFor(command) != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workspace-workspacetemplatecommandensuredefaults"></a>

### CT-workspace-WorkspaceTemplateCommandEnsureDefaults

- 边界：`TopicWorkspaceTemplateCommandEnsureDefaults` = `/workspace-template/command/ensure-defaults`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L34)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L438)
- 接收provider：workspace；Command [EnsureWorkspaceTemplateDefaultsCommand](#schema-ensureworkspacetemplatedefaultscommand) → Result [EnsureWorkspaceTemplateDefaultsResult](#schema-ensureworkspacetemplatedefaultsresult)；target合法caller：kernel/system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleSystemDefaultsCommand`（workspace）
- 接收入口：[handleSystemDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L146)
- 已观察外部引用：system [ensureWorkspaceTemplateDefaults](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/biz/system.go#L87)
- 本topic入口/具体类型分支的Source审查索引：[handleSystemDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L146)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != workspaceTemplateDefaultsCommandSource` 分支谓词（[handleSystemDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L146)）
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`ev == nil || ev.Source() != workspaceTemplateDefaultsCommandSource || m == nil || m.templates == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workspace-workspacetemplatecommandget"></a>

### CT-workspace-WorkspaceTemplateCommandGet

- 边界：`TopicWorkspaceTemplateCommandGet` = `/workspace-template/command/get`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L30)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L434)
- 接收provider：workspace；Command [GetWorkspaceTemplateCommand](#schema-getworkspacetemplatecommand) → Result [WorkspaceTemplateResult](#schema-workspacetemplateresult)；target合法caller：workorchd、orchestrator、run、system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [ReadWorkspaceTemplateByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/workspace_template_reads.go#L15)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || cmd.TemplateID == "" || m.templates == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workspace-workspacetemplatecommandlist"></a>

### CT-workspace-WorkspaceTemplateCommandList

- 边界：`TopicWorkspaceTemplateCommandList` = `/workspace-template/command/list`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L31)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L435)
- 接收provider：workspace；Command [ListWorkspaceTemplatesCommand](#schema-listworkspacetemplatescommand) → Result [WorkspaceTemplateListResult](#schema-workspacetemplatelistresult)；target合法caller：workorchd、orchestrator、run、system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [ListWorkspaceTemplatesByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/workspace_template_reads.go#L32)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`!ok || response.ValidateFor(command) != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workspace-workspacetemplatecommandquerybuiltindefaults"></a>

### CT-workspace-WorkspaceTemplateCommandQueryBuiltinDefaults

- 边界：`TopicWorkspaceTemplateCommandQueryBuiltinDefaults` = `/workspace-template/command/query-builtin-defaults`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/builtin_defaults.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/builtin_defaults.go#L39)
- 接收provider：workspace；Command [QueryBuiltinWorkspaceTemplateDefaultsCommand](#schema-querybuiltinworkspacetemplatedefaultscommand) → Result [QueryBuiltinWorkspaceTemplateDefaultsResult](#schema-querybuiltinworkspacetemplatedefaultsresult)；target合法caller：application/workorchd/appservice
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleBuiltinTemplateDefaultsCommand`（workspace）
- 接收入口：[handleBuiltinTemplateDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/builtin_defaults.go#L19)
- 已观察外部引用：workorchd [queryBuiltinDefaults](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/builtin_defaults_service.go#L27)
- 本topic入口/具体类型分支的Source审查索引：[handleBuiltinTemplateDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/builtin_defaults.go#L19)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[handleBuiltinTemplateDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/builtin_defaults.go#L19)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workspace-workspacetemplatecommandupdate"></a>

### CT-workspace-WorkspaceTemplateCommandUpdate

- 边界：`TopicWorkspaceTemplateCommandUpdate` = `/workspace-template/command/update`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/topics.go#L32)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L436)
- 接收provider：workspace；Command [UpdateWorkspaceTemplateCommand](#schema-updateworkspacetemplatecommand) → Result [WorkspaceTemplateResult](#schema-workspacetemplateresult)；target合法caller：workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleCommand`（workspace）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/command_router.go#L93)
- 已观察外部引用：workorchd [UpdateWorkspaceTemplateRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/workspaceports/template_records.go#L39)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workspace-workspacetemplatecommandupgradebuiltindefault"></a>

### CT-workspace-WorkspaceTemplateCommandUpgradeBuiltinDefault

- 边界：`TopicWorkspaceTemplateCommandUpgradeBuiltinDefault` = `/workspace-template/command/upgrade-builtin-default`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/builtin_defaults.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/builtin_defaults.go#L40)
- 接收provider：workspace；Command [UpgradeBuiltinWorkspaceTemplateDefaultCommand](#schema-upgradebuiltinworkspacetemplatedefaultcommand) → Result [UpgradeBuiltinWorkspaceTemplateDefaultResult](#schema-upgradebuiltinworkspacetemplatedefaultresult)；target合法caller：application/workorchd/appservice
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L86) → `handleBuiltinTemplateDefaultsCommand`（workspace）
- 接收入口：[handleBuiltinTemplateDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/builtin_defaults.go#L19)
- 已观察外部引用：workorchd [upgradeBuiltinDefault](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/builtin_defaults_service.go#L77)
- 本topic入口/具体类型分支的Source审查索引：[handleBuiltinTemplateDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/builtin_defaults.go#L19)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[handleBuiltinTemplateDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/builtin_defaults.go#L19)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-admitdefinitionauthoringcommand"></a>

### AdmitDefinitionAuthoringCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_admission.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_admission.go#L15)
- 字段：`Candidate workspace.DefinitionAdmissionCandidate`；`ExpectedWorkspaceRevision uint64`；`ExpectedConfigurationHash string`；`ClarificationTicket *WorkspaceDefinitionClarificationTicketFact`；`ClarificationResolution *WorkspaceDefinitionClarificationResolutionFact`
- 字段引用类型：[DefinitionAdmissionCandidate](#schema-definitionadmissioncandidate)；[WorkspaceDefinitionClarificationTicketFact](#schema-workspacedefinitionclarificationticketfact)；[WorkspaceDefinitionClarificationResolutionFact](#schema-workspacedefinitionclarificationresolutionfact)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_admission.go#L23)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_admission.go#L49)
- 空值/identity/版本/范围原式：`c.ExpectedWorkspaceRevision == 0 || c.ExpectedConfigurationHash == "" || c.Candidate.AuthoringRef == nil || c.Candidate.CompilationRef != nil`；`err := c.Candidate.Validate(); err != nil`；`c.ClarificationTicket != nil && c.ClarificationResolution != nil`；`c.ClarificationTicket != nil && c.ClarificationTicket.Validate() != nil || c.ClarificationResolution != nil && c.ClarificationResolution.Validate() != nil`；`c.Validate() != nil || r.Candidate.CandidateHash != c.Candidate.CandidateHash || r.WorkspaceRevision < c.ExpectedWorkspaceRevision || r.AdmissionResult.ValidateFor(r.Candidate) != nil`；`r.PriorClarification != nil`；`r.PriorClarification.Validate() != nil || r.PriorClarification.Candidate.WorkspaceID != r.Candidate.WorkspaceID || r.PriorClarification.Candidate.CandidateID != r.Candidate.CandidateID || r.PriorClarification.Candidate.CandidateRevision >= r.Candidate.CandidateRevision`；`c.ClarificationTicket != nil && r.PriorClarification == nil || c.ClarificationResolution != nil && r.PriorClarification == nil`；`r.AdmissionResult.Disposition == workspace.DefinitionAdmissionAdmittedAllOf`；`r.BlockingTicketID != ""`；`r.StartupInput == nil`；`r.PriorClarification == nil || r.ActivationAdmission != nil || c.ClarificationTicket != nil || c.ClarificationResolution != nil`；`r.ActivationAdmission == nil || r.ActivationAdmission.ValidateCandidate(*r.StartupInput, r.Candidate) != nil`；`r.PriorClarification == nil && r.ActivationAdmission.State != workspace.WorkspaceActivationEligible || r.PriorClarification != nil && (r.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution && r.ActivationAdmission.State != workspace.WorkspaceActivationEligible || r.ActivationAdmission.BlockingTicketID == "")`；`err != nil || r.ActivationAdmission.BlockingTicketID != identity.TicketID`；`c.ClarificationTicket != nil && (r.PriorClarification == nil || c.ClarificationTicket.ValidateFor(*r.PriorClarification) != nil || c.ClarificationTicket.TicketBinding.TicketID != r.ActivationAdmission.BlockingTicketID)`；`r.ActivationAdmission.State == workspace.WorkspaceActivationEligible && r.ActivationAdmission.BlockingTicketID != "" && c.ClarificationResolution != nil && c.ClarificationResolution.ValidateForEligible(r.Candidate, r.AdmissionResult, *r.StartupInput, *r.ActivationAdmission) != nil`；`r.ActivationAdmission.State == workspace.WorkspaceActivationPendingClarificationResolution && c.ClarificationResolution != nil`；`r.StartupInput != nil || r.ActivationAdmission != nil`；`err != nil || r.BlockingTicketID != identity.TicketID`

<a id="schema-admitdefinitionauthoringresult"></a>

### AdmitDefinitionAuthoringResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_admission.go:39](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_admission.go#L39)
- 字段：`Candidate workspace.DefinitionAdmissionCandidate`；`AdmissionResult workspace.DefinitionAdmissionResult`；`PriorClarification *DefinitionAdmissionClarificationBinding`；`BlockingTicketID common.TicketID`；`StartupInput *workspace.WorkspaceStartupInputContract`；`ActivationAdmission *workspace.WorkspaceActivationAdmission`；`WorkspaceRevision uint64`
- 字段引用类型：[DefinitionAdmissionCandidate](#schema-definitionadmissioncandidate)；[DefinitionAdmissionResult](#schema-definitionadmissionresult)；[DefinitionAdmissionClarificationBinding](#schema-definitionadmissionclarificationbinding)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[WorkspaceStartupInputContract](#schema-workspacestartupinputcontract)；[WorkspaceActivationAdmission](#schema-workspaceactivationadmission)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_admission.go#L49)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.Candidate.CandidateHash != c.Candidate.CandidateHash || r.WorkspaceRevision < c.ExpectedWorkspaceRevision || r.AdmissionResult.ValidateFor(r.Candidate) != nil`；`r.PriorClarification != nil`；`r.PriorClarification.Validate() != nil || r.PriorClarification.Candidate.WorkspaceID != r.Candidate.WorkspaceID || r.PriorClarification.Candidate.CandidateID != r.Candidate.CandidateID || r.PriorClarification.Candidate.CandidateRevision >= r.Candidate.CandidateRevision`；`c.ClarificationTicket != nil && r.PriorClarification == nil || c.ClarificationResolution != nil && r.PriorClarification == nil`；`r.AdmissionResult.Disposition == workspace.DefinitionAdmissionAdmittedAllOf`；`r.BlockingTicketID != ""`；`r.StartupInput == nil`；`r.PriorClarification == nil || r.ActivationAdmission != nil || c.ClarificationTicket != nil || c.ClarificationResolution != nil`；`r.ActivationAdmission == nil || r.ActivationAdmission.ValidateCandidate(*r.StartupInput, r.Candidate) != nil`；`r.PriorClarification == nil && r.ActivationAdmission.State != workspace.WorkspaceActivationEligible || r.PriorClarification != nil && (r.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution && r.ActivationAdmission.State != workspace.WorkspaceActivationEligible || r.ActivationAdmission.BlockingTicketID == "")`；`err != nil || r.ActivationAdmission.BlockingTicketID != identity.TicketID`；`c.ClarificationTicket != nil && (r.PriorClarification == nil || c.ClarificationTicket.ValidateFor(*r.PriorClarification) != nil || c.ClarificationTicket.TicketBinding.TicketID != r.ActivationAdmission.BlockingTicketID)`；`r.ActivationAdmission.State == workspace.WorkspaceActivationEligible && r.ActivationAdmission.BlockingTicketID != "" && c.ClarificationResolution != nil && c.ClarificationResolution.ValidateForEligible(r.Candidate, r.AdmissionResult, *r.StartupInput, *r.ActivationAdmission) != nil`；`r.ActivationAdmission.State == workspace.WorkspaceActivationPendingClarificationResolution && c.ClarificationResolution != nil`；`r.StartupInput != nil || r.ActivationAdmission != nil`；`err != nil || r.BlockingTicketID != identity.TicketID`

<a id="schema-authoringspecificationrequestresult"></a>

### AuthoringSpecificationRequestResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go:84](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L84)
- 字段：`Request workspace.DefinitionAuthoringSpecificationRequest`；`FreshPermitID string`；`Consumed uint32`；`Remaining uint32`
- 字段引用类型：[DefinitionAuthoringSpecificationRequest](#schema-definitionauthoringspecificationrequest)
- 实际方法：[validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L91)；[ValidateReserve](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L104)；[ValidateCompletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L117)
- 空值/identity/版本/范围原式：`err := r.Request.ValidateView(); err != nil`；`r.Consumed == 0 || r.Consumed > workspace.DefinitionAuthoringSpecificationRequestLimit || r.Remaining != workspace.DefinitionAuthoringSpecificationRequestLimit-r.Consumed`；`r.FreshPermitID != "" && (r.FreshPermitID != r.Request.PermitID || r.Request.Proposal != nil || r.Request.Review != nil || r.Request.FailureCode != "")`；`err := c.Validate(); err != nil`；`err := r.validate(); err != nil`；`r.Request.RequestID != c.RequestID || r.Request.PermitID != c.Request().ExpectedPermitID()`；`r.FreshPermitID != "" || r.Request.PermitID != c.PermitID`；`c.Mapping != nil`；`r.Request.Phase != workspace.DefinitionSpecificationProposal || r.Request.CompletionIdentity != workspace.MappingCompletionIdentity(c.PermitID, *c.Mapping, *c.SuccessAudit)`；`r.Request.FailureCode == "invalid_proposal"`；`r.Request.Proposal != nil || d == nil || d.ProviderCode != "" || d.PayloadBytes != c.SuccessAudit.Document.ObservedBytes || d.PayloadSHA256 != c.SuccessAudit.Document.SHA256 || d.ProviderDocument == nil || d.ProviderWire == nil || !reflect.DeepEqual(*d.ProviderDocument, c.SuccessAudit.Metadata().Document) || !reflect.DeepEqual(*d.ProviderWire, c.SuccessAudit.Metadata().Wire)`；`r.Request.Proposal == nil || !reflect.DeepEqual(r.Request.Mapping, c.Mapping) || r.Request.SuccessAudit == nil || !reflect.DeepEqual(*r.Request.SuccessAudit, c.SuccessAudit.Metadata()) || c.Proposal != nil && !reflect.DeepEqual(r.Request.Proposal, c.Proposal)`；`c.Diagnostic != nil`；`r.Request.FailureCode != c.FailureCode || !reflect.DeepEqual(r.Request.Review, c.Review) || !reflect.DeepEqual(r.Request.Diagnostic, diagnostic)`

<a id="schema-authoringsummary"></a>

### AuthoringSummary

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:61](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L61)
- 字段：`State string`；`FailurePhase string`；`FailureCode string`；`ReviewCount int`；`InputRevision uint64`

<a id="schema-begindefinitionspecificationcommand"></a>

### BeginDefinitionSpecificationCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_specification.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L13)
- 字段：`Candidate workspacemodel.DefinitionAdmissionCandidate`；`Resources []workspacemodel.GoalSpecificationResource`；`Dimensions []workspacemodel.GoalSpecificationDimension`；`ExpectedWorkspaceRevision uint64`；`ExpectedConfigurationHash string`
- 字段引用类型：[DefinitionAdmissionCandidate](#schema-definitionadmissioncandidate)；[GoalSpecificationResource](#schema-goalspecificationresource)；[GoalSpecificationDimension](#schema-goalspecificationdimension)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L21)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L28)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L48)
- 空值/identity/版本/范围原式：`c.Candidate.Specification != nil || c.Candidate.CompilationRef != nil || c.ExpectedWorkspaceRevision == 0 || c.ExpectedConfigurationHash == "" || c.Candidate.Validate() != nil || workspacemodel.ValidateDefinitionSpecificationInputs(c.Resources, c.Dimensions) != nil`；`err := command.Validate(); err != nil`；`r.WorkspaceID != command.Candidate.WorkspaceID || r.CandidateHash != command.Candidate.CandidateHash || r.Snapshot.Validate() != nil || r.Snapshot.CandidateHash != r.CandidateHash || r.GoalRevision == "" || r.DoDRevision == "" || r.WorkspaceRevision == 0 || r.CompilationInputHash != workspacemodel.DefinitionSpecificationCompilationInputHash(command.Candidate.CandidateHash, command.Resources, command.Dimensions) || r.CompilationInputHash != workspacemodel.DefinitionSpecificationCompilationInputHash(r.Snapshot.CandidateHash, r.Resources, r.Dimensions)`

<a id="schema-begindefinitionspecificationresult"></a>

### BeginDefinitionSpecificationResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_specification.go:36](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L36)
- 字段：`WorkspaceID common.WorkspaceID`；`CandidateHash string`；`CompilationInputHash string`；`GoalRevision string`；`DoDRevision string`；`Snapshot workspacemodel.DefinitionAdmissionCandidate`；`Resources []workspacemodel.GoalSpecificationResource`；`Dimensions []workspacemodel.GoalSpecificationDimension`；`WorkspaceRevision uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAdmissionCandidate](#schema-definitionadmissioncandidate)；[GoalSpecificationResource](#schema-goalspecificationresource)；[GoalSpecificationDimension](#schema-goalspecificationdimension)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L48)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != command.Candidate.WorkspaceID || r.CandidateHash != command.Candidate.CandidateHash || r.Snapshot.Validate() != nil || r.Snapshot.CandidateHash != r.CandidateHash || r.GoalRevision == "" || r.DoDRevision == "" || r.WorkspaceRevision == 0 || r.CompilationInputHash != workspacemodel.DefinitionSpecificationCompilationInputHash(command.Candidate.CandidateHash, command.Resources, command.Dimensions) || r.CompilationInputHash != workspacemodel.DefinitionSpecificationCompilationInputHash(r.Snapshot.CandidateHash, r.Resources, r.Dimensions)`

<a id="schema-builtinworkspacetemplatedefault"></a>

### BuiltinWorkspaceTemplateDefault

- 源码：[internal/modules/kernel/workspace/pkg/events/builtin_defaults.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/builtin_defaults.go#L13)
- 字段：`TemplateID string`；`Present bool`；`CurrentContentHash string`；`CurrentBuiltinRevision string`；`BuiltinContentHash string`；`UpgradeAvailable bool`

<a id="schema-candidateitemtoitemrefmapping"></a>

### CandidateItemToItemRefMapping

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:122](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L122)
- 字段：`CandidateItemRef string`；`ItemRef string`
- 实际方法：[normalizeItemMappings](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L836)；[stableItemMappingHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L846)

<a id="schema-completeauthoringspecificationcommand"></a>

### CompleteAuthoringSpecificationCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go:41](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L41)
- 字段：`WorkspaceID common.WorkspaceID`；`PermitID string`；`Proposal *workspace.GoalSpecification`；`Mapping *workspace.DefinitionAuthoringMapping`；`SuccessAudit *workspace.DefinitionAuthoringSuccessByteAudit`；`Review *workspace.GoalSpecificationCompletenessReview`；`FailureCode string`；`Diagnostic *workspace.DefinitionAuthoringDiagnostic`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[GoalSpecification](#schema-goalspecification)；[DefinitionAuthoringMapping](#schema-definitionauthoringmapping)；[DefinitionAuthoringSuccessByteAudit](#schema-definitionauthoringsuccessbyteaudit)；[GoalSpecificationCompletenessReview](#schema-goalspecificationcompletenessreview)；[DefinitionAuthoringDiagnostic](#schema-definitionauthoringdiagnostic)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L52)；[ValidateCompletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L117)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.PermitID == "" || len(c.PermitID) > 256`；`c.Mapping != nil`；`c.SuccessAudit == nil || c.SuccessAudit.Validate() != nil`；`c.Proposal != nil || c.SuccessAudit != nil`；`c.Review != nil`；`c.FailureCode != ""`；`c.Diagnostic != nil && (c.Diagnostic.Validate() != nil || c.FailureCode == "provider_failed" && c.Diagnostic.ProviderCode == "")`；`c.Diagnostic != nil`；`outcomes != 1`；`err := c.Validate(); err != nil`；`err := r.validate(); err != nil`；`r.FreshPermitID != "" || r.Request.PermitID != c.PermitID`；`r.Request.Phase != workspace.DefinitionSpecificationProposal || r.Request.CompletionIdentity != workspace.MappingCompletionIdentity(c.PermitID, *c.Mapping, *c.SuccessAudit)`；`r.Request.FailureCode == "invalid_proposal"`；`r.Request.Proposal != nil || d == nil || d.ProviderCode != "" || d.PayloadBytes != c.SuccessAudit.Document.ObservedBytes || d.PayloadSHA256 != c.SuccessAudit.Document.SHA256 || d.ProviderDocument == nil || d.ProviderWire == nil || !reflect.DeepEqual(*d.ProviderDocument, c.SuccessAudit.Metadata().Document) || !reflect.DeepEqual(*d.ProviderWire, c.SuccessAudit.Metadata().Wire)`；`r.Request.Proposal == nil || !reflect.DeepEqual(r.Request.Mapping, c.Mapping) || r.Request.SuccessAudit == nil || !reflect.DeepEqual(*r.Request.SuccessAudit, c.SuccessAudit.Metadata()) || c.Proposal != nil && !reflect.DeepEqual(r.Request.Proposal, c.Proposal)`；`r.Request.FailureCode != c.FailureCode || !reflect.DeepEqual(r.Request.Review, c.Review) || !reflect.DeepEqual(r.Request.Diagnostic, diagnostic)`

<a id="schema-completedefinitionderivationcommand"></a>

### CompleteDefinitionDerivationCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_derivation.go:30](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L30)
- 字段：`WorkspaceID common.WorkspaceID`；`PermitID string`；`Proposal *workspace.DefinitionDerivedAcceptance`；`FailureCode string`；`Diagnostic *workspace.DefinitionAuthoringDiagnostic`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionDerivedAcceptance](#schema-definitionderivedacceptance)；[DefinitionAuthoringDiagnostic](#schema-definitionauthoringdiagnostic)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L38)；[ValidateCompletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L85)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.PermitID == "" || len(c.PermitID) > 256 || (c.Proposal == nil) == (c.FailureCode == "")`；`c.FailureCode != "" && c.FailureCode != "provider_failed" && c.FailureCode != "invalid_proposal"`；`c.Diagnostic != nil && (c.FailureCode != "invalid_proposal" && c.FailureCode != "provider_failed" || c.Diagnostic.Validate() != nil || c.FailureCode == "provider_failed" && c.Diagnostic.ProviderCode == "")`；`err := c.Validate(); err != nil`；`err := r.validate(); err != nil`；`c.Diagnostic != nil`；`r.FreshPermitID != "" || r.Request.Input.WorkspaceID != c.WorkspaceID || r.Request.PermitID != c.PermitID || r.Request.FailureCode != c.FailureCode || !reflect.DeepEqual(r.Request.Proposal, c.Proposal) || !reflect.DeepEqual(r.Request.Diagnostic, diagnostic)`

<a id="schema-completedefinitionspecificationrequestcommand"></a>

### CompleteDefinitionSpecificationRequestCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_specification.go:121](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L121)
- 字段：`Identity workspacemodel.DefinitionSpecificationRequestIdentity`；`PermitID string`；`Completion workspacemodel.DefinitionSpecificationRequestCompletion`
- 字段引用类型：[DefinitionSpecificationRequestIdentity](#schema-definitionspecificationrequestidentity)；[DefinitionSpecificationRequestCompletion](#schema-definitionspecificationrequestcompletion)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L127)
- 空值/identity/版本/范围原式：`c.Identity.Validate() != nil || strings.TrimSpace(c.PermitID) != c.Identity.PermitID()`

<a id="schema-confirmdefinitionauthoringcommand"></a>

### ConfirmDefinitionAuthoringCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_confirmation.go:16](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_confirmation.go#L16)
- 字段：`WorkspaceID common.WorkspaceID`；`InputHash string`；`DerivationPermitID string`；`ProposalHash string`；`SpecificationHash string`；`ExpectedWorkspaceRevision uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_confirmation.go#L25)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_confirmation.go#L44)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.InputHash == "" || len(c.InputHash) > 256 || c.DerivationPermitID == "" || len(c.DerivationPermitID) > 256 || c.ProposalHash == "" || len(c.ProposalHash) > 256 || c.SpecificationHash == "" || len(c.SpecificationHash) > 256 || c.ExpectedWorkspaceRevision == 0`；`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.InputHash != c.InputHash || r.ProposalHash != c.ProposalHash || r.SpecificationHash != c.SpecificationHash || r.Goal.WorkspaceID != c.WorkspaceID || r.DoD.WorkspaceID != c.WorkspaceID || r.Goal.ID == "" || r.DoD.ID == "" || r.WorkspaceRevision == 0`

<a id="schema-confirmdefinitionauthoringresult"></a>

### ConfirmDefinitionAuthoringResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_confirmation.go:34](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_confirmation.go#L34)
- 字段：`WorkspaceID common.WorkspaceID`；`InputHash string`；`ProposalHash string`；`SpecificationHash string`；`Goal workspace.Goal`；`DoD workspace.DoD`；`WorkspaceRevision uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Goal](#schema-goal)；[DoD](#schema-dod)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_confirmation.go#L44)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.InputHash != c.InputHash || r.ProposalHash != c.ProposalHash || r.SpecificationHash != c.SpecificationHash || r.Goal.WorkspaceID != c.WorkspaceID || r.DoD.WorkspaceID != c.WorkspaceID || r.Goal.ID == "" || r.DoD.ID == "" || r.WorkspaceRevision == 0`

<a id="schema-createdefinitionauthoringcommand"></a>

### CreateDefinitionAuthoringCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_creation.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_creation.go#L13)
- 字段：`RequestID string`；`WorkspaceID common.WorkspaceID`；`Title string`；`Description string`；`DeliverableDirectory string`；`InternalDirectory string`；`Input workspace.DefinitionAuthoringInput`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAuthoringInput](#schema-definitionauthoringinput)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_creation.go#L23)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_creation.go#L49)
- 空值/identity/版本/范围原式：`(workspace.DefinitionAuthoringCreation{RequestID: c.RequestID, InputHash: c.CreationInputHash()}).Validate(c.WorkspaceID) != nil || c.Input.Validate() != nil || c.Input.WorkspaceID != c.WorkspaceID || c.Input.Revision != 1 || strings.TrimSpace(c.Title) == "" || len(c.Title) > 1024 || len(c.Description) > 32*1024 || c.DeliverableDirectory == "" || c.InternalDirectory == ""`；`strings.ContainsRune(value, 0) || len(value) > 32*1024`；`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.RequestID != c.RequestID || r.InputHash != c.CreationInputHash()`

<a id="schema-createdefinitionauthoringresult"></a>

### CreateDefinitionAuthoringResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_creation.go:43](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_creation.go#L43)
- 字段：`WorkspaceID common.WorkspaceID`；`RequestID string`；`InputHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_creation.go#L49)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.RequestID != c.RequestID || r.InputHash != c.CreationInputHash()`

<a id="schema-createworkspacecommand"></a>

### CreateWorkspaceCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:24](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L24)
- 字段：`WorkspaceID common.WorkspaceID`；`Title string`；`Description string`；`DeliverableDirectory string`；`InternalDirectory string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-createworkspacetemplatecommand"></a>

### CreateWorkspaceTemplateCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:236](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L236)
- 字段：`TemplateID string`；`ParentTemplateIDs []string`；`Name string`；`Description string`；`GoalText string`；`DoDItems []string`；`DefaultSoulID string`；`SoulBindings []soulbinding.Binding`；`ContextHints workorcheventpayload.Payload`；`EvidencePolicies []workorcheventpayload.Payload`
- 字段引用类型：[Binding（shared/soulbinding声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/soulbinding/binding.go#L13)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-definedodcommand"></a>

### DefineDoDCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:222](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L222)
- 字段：`DoDID common.DoDID`；`WorkspaceID common.WorkspaceID`；`Items []string`
- 字段引用类型：[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[doDResultIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L41)

<a id="schema-definegoalcommand"></a>

### DefineGoalCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:208](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L208)
- 字段：`GoalID common.GoalID`；`WorkspaceID common.WorkspaceID`；`Text string`
- 字段引用类型：[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[goalResultIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L25)

<a id="schema-definitionadmissioncandidate"></a>

### DefinitionAdmissionCandidate

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:78](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L78)
- 字段：`SchemaVersion uint32`；`WorkspaceID common.WorkspaceID`；`CandidateID string`；`CandidateRevision uint64`；`BaseDefinitionRevision string`；`BaseStartupInputHash string`；`GoalInput DefinitionCandidateItem`；`DoDInputs []DefinitionCandidateItem`；`WorkspaceTemplateSnapshot DefinitionTemplateSnapshot`；`AdmissionPolicyBindings []DefinitionAdmissionPolicyBinding`；`RuntimeProfileContracts []runtimecontract.RuntimeProfileContract`；`RequiredOwnerFactBindings []DefinitionRequiredOwnerFactBinding`；`OwnerFactConvergenceBindings []DefinitionOwnerFactConvergenceBinding`；`RuntimeProfileContractsHash string`；`AdmissionCompilerVersion string`；`ResolvedProjectWorkspaceDirectory string`；`ProposedRelationInputs []DefinitionProposedRelationInput`；`Specification *GoalSpecification`；`CompilationRef *DefinitionSpecificationCompilationRef`；`AuthoringRef *DefinitionAuthoringAdmissionRef`；`CandidateHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionCandidateItem](#schema-definitioncandidateitem)；[DefinitionTemplateSnapshot](#schema-definitiontemplatesnapshot)；[DefinitionAdmissionPolicyBinding](#schema-definitionadmissionpolicybinding)；[RuntimeProfileContract（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_profile_contract.go#L10)；[DefinitionRequiredOwnerFactBinding](#schema-definitionrequiredownerfactbinding)；[DefinitionOwnerFactConvergenceBinding](#schema-definitionownerfactconvergencebinding)；[DefinitionProposedRelationInput](#schema-definitionproposedrelationinput)；[GoalSpecification](#schema-goalspecification)；[DefinitionSpecificationCompilationRef](#schema-definitionspecificationcompilationref)；[DefinitionAuthoringAdmissionRef](#schema-definitionauthoringadmissionref)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L167)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L200)；[WithHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L288)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L294)；[computeHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L325)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L381)；[ValidateCandidate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L459)；[candidateSpecificationAdmissionHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L550)；[ValidateCandidate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L590)；[ValidateActivationAdmissionPersistence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L610)；[ValidateActivationAdmissionView](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L615)；[validateActivationAdmissionPersistence](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L618)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L191)
- 空值/identity/版本/范围原式：`c.Specification != nil`；`c.CompilationRef != nil`；`c.AuthoringRef != nil`；`c.SchemaVersion != DefinitionAdmissionCandidateSchemaVersionCurrent || c.WorkspaceID == "" || c.CandidateID == "" || c.CandidateRevision == 0`；`(c.BaseDefinitionRevision == "") != (c.BaseStartupInputHash == "")`；`c.GoalInput.CandidateItemRef == "" || c.GoalInput.Text == "" || c.GoalInput.SourceIdentity == "" || len(c.DoDInputs) == 0`；`item.CandidateItemRef == "" || item.Text == "" || item.SourceIdentity == ""`；`_, duplicate := seen[item.CandidateItemRef]; duplicate`；`c.WorkspaceTemplateSnapshot.TemplateRef == "" || c.WorkspaceTemplateSnapshot.Revision == "" || c.WorkspaceTemplateSnapshot.SnapshotHash == "" || c.RuntimeProfileContractsHash == "" || c.ResolvedProjectWorkspaceDirectory == ""`；`runtimecontract.StableRuntimeProfileContractsHash(c.RuntimeProfileContracts) != c.RuntimeProfileContractsHash`；`c.AdmissionCompilerVersion != DefinitionAdmissionCompilerVersionCurrent`；`binding.PolicyRef == "" || binding.Revision == "" || binding.SnapshotHash == ""`；`_, duplicate := policyRefs[binding.PolicyRef]; duplicate`；`binding.SourceRef == ""`；`err := binding.Requirement.Validate(); err != nil`；`err := binding.Convergence.Requirement.Validate(); err != nil`；`relation.RelationRef == "" || relation.RelationKind == "" || len(relation.SourceItemRefs) == 0 || len(relation.TargetItemRefs) == 0`；`_, ok := seen[ref]; !ok`；`err := c.Specification.Validate(c.GoalInput, c.DoDInputs); err != nil`；`c.Specification == nil || c.CompilationRef.Validate() != nil`；`c.Specification == nil || c.CompilationRef != nil || c.AuthoringRef.Validate() != nil || c.Specification.AdmissionHash() != c.AuthoringRef.SpecificationHash`；`c.CandidateHash == "" || c.CandidateHash != want`；`err := candidate.Validate(); err != nil`；`r.WorkspaceID != candidate.WorkspaceID || r.CandidateID != candidate.CandidateID || r.CandidateRevision != candidate.CandidateRevision || r.CandidateHash != candidate.CandidateHash || r.AdmissionCompilerVersion != candidate.AdmissionCompilerVersion`；`r.CanonicalAdmittedInputHash == "" || r.ViolationCode != "" || r.CanonicalViolation != "" || r.CanonicalViolationHash != "" || len(r.OffendingRefs) != 0`；`r.ViolationCode == "" || r.CanonicalViolation == "" || r.CanonicalViolationHash == "" || len(r.OffendingRefs) == 0 || r.CanonicalAdmittedInputHash != ""`；`r.AdmissionResultHash == "" || r.AdmissionResultHash != r.computeHash()`；`err := s.Validate(); err != nil`；`s.WorkspaceID != candidate.WorkspaceID || s.GoalTextHash != StableDefinitionTextHash(candidate.GoalInput.Text) || s.WorkspaceTemplateRef != candidate.WorkspaceTemplateSnapshot.TemplateRef || s.TemplateRevision != candidate.WorkspaceTemplateSnapshot.Revision || s.TemplateSnapshotHash != candidate.WorkspaceTemplateSnapshot.SnapshotHash || s.RuntimeProfileContractsHash != candidate.RuntimeProfileContractsHash || s.SpecificationHash != candidateSpecificationAdmissionHash(candidate) || s.ResolvedProjectWorkspaceDirectory != candidate.ResolvedProjectWorkspaceDirectory`；`dodSourceIdentity == ""`；`dodSourceIdentity != item.SourceIdentity`；`s.DefinitionRevision != candidate.GoalInput.SourceIdentity+"|"+dodSourceIdentity || s.DoDItemSetHash != StableDefinitionStringSetHash(dodTexts)`；`mapping.CandidateItemRef == "" || mapping.ItemRef == ""`；`_, duplicate := mappings[mapping.CandidateItemRef]; duplicate`；`_, duplicate := itemRefs[mapping.ItemRef]; duplicate`；`len(mappings) != 1+len(candidate.DoDInputs) || mappings[candidate.GoalInput.CandidateItemRef] != s.GoalDefinitionRef`；`ref == ""`；`!equalDefinitionStrings(dodRefs, s.DoDDefinitionRefs)`；`candidate.Specification == nil`；`err := a.Validate(startup); err != nil`；`err := startup.ValidateCandidate(candidate); err != nil`；`a.WorkspaceID != candidate.WorkspaceID || a.CandidateID != candidate.CandidateID || a.CandidateRevision != candidate.CandidateRevision || a.CandidateHash != candidate.CandidateHash`；`w == nil`；`w.DefinitionAuthoringConfirmation != nil`；`sequence == 0 && confirmation.PreviousInputHash == ""`；`sequence != uint64(len(w.DefinitionAuthoringHistory)+1)`；`w.StartupInput == nil && w.ActivationAdmission == nil`；`len(w.DefinitionAuthoringHistory) > 0`；`err := w.validateDefinitionAuthoringHistory(view); err != nil`；`w.DefinitionAuthoringConfirmation == nil`；`len(w.DefinitionAdmissionCandidates) != len(w.DefinitionAuthoringHistory)`；`last.Validate() != nil || last.AuthoringRef == nil || last.CandidateID != w.DefinitionAdmissionCandidates[0].CandidateID`；`len(w.DefinitionAdmissionCandidates) == len(w.DefinitionAuthoringHistory)`；`w.DefinitionAuthoringConfirmation.validateMode(w.DefinitionAuthoring, w.ID, view) != nil || w.DefinitionAuthoringConfirmation.ValidateSuccessor(w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1].Confirmation) != nil`；`w.DefinitionAuthoringConfirmation.Phase != DefinitionAuthoringConfirmationMaterialized || len(w.DefinitionAdmissionCandidates) != len(w.DefinitionAuthoringHistory)+1`；`candidate.Validate() != nil || candidate.AuthoringRef == nil || candidate.CandidateID != w.DefinitionAdmissionCandidates[0].CandidateID || candidate.CandidateRevision != uint64(len(w.DefinitionAdmissionCandidates))`；`err != nil || *candidate.AuthoringRef != ref || w.DefinitionAuthoringConfirmation.ValidateSuccessor(w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1].Confirmation) != nil`；`candidate.GoalInput != goal || !reflect.DeepEqual(candidate.DoDInputs, DefinitionAdmissionCandidate{DoDInputs: dod}.Normalized().DoDInputs) || candidate.Specification == nil || !reflect.DeepEqual(candidate.Specification.Normalized(), reviewed.Normalized())`；`w.DefinitionAuthoringConfirmation != nil && len(w.DefinitionAdmissionCandidates) > 0`；`w.DefinitionAuthoring == nil || w.DefinitionAuthoringConfirmation.Phase != DefinitionAuthoringConfirmationMaterialized || len(w.DefinitionAdmissionCandidates) != 1`；`err := candidate.Validate(); err != nil || candidate.AuthoringRef == nil`；`err != nil || *candidate.AuthoringRef != ref`；`w.StartupInput == nil || w.ActivationAdmission == nil`；`w.DefinitionAuthoringConfirmation != nil && w.DefinitionAuthoringConfirmation.Phase != DefinitionAuthoringConfirmationMaterialized`；`len(w.DefinitionAuthoringHistory) > 0 && w.validateDefinitionAuthoringHistory(view) != nil`；`candidate.CandidateID != admission.CandidateID || candidate.CandidateRevision != admission.CandidateRevision || candidate.CandidateHash != admission.CandidateHash`；`matched != nil`；`matched == nil`；`err := admission.ValidateCandidate(w.StartupInput.Normalized(), *matched); err != nil`；`w.DefinitionAuthoring == nil || matched.AuthoringRef == nil || len(w.DefinitionAdmissionCandidates) != len(w.DefinitionAuthoringHistory)+1 || matched.CandidateRevision != uint64(len(w.DefinitionAdmissionCandidates)) || len(w.DefinitionAuthoringHistory) > 0 && (matched.CandidateID != w.DefinitionAdmissionCandidates[0].CandidateID || w.DefinitionAuthoringConfirmation.ValidateSuccessor(w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1].Confirmation) != nil)`；`err != nil || *matched.AuthoringRef != ref`；`matched.GoalInput != goal || !reflect.DeepEqual(matched.DoDInputs, DefinitionAdmissionCandidate{DoDInputs: dod}.Normalized().DoDInputs) || matched.Specification == nil || !reflect.DeepEqual(matched.Specification.Normalized(), reviewed.Normalized())`；`matched.AuthoringRef != nil`；`c.FailureCode != ""`；`c.Proposal != nil || c.Review != nil || len(c.FailureCode) > 96`；`(r < 'a' || r > 'z') && r != '_' && (r < '0' || r > '9')`；`c.Proposal == nil || c.Review != nil || len(c.Proposal.Reviews) != 0 || c.Proposal.Version != GoalSpecificationVersionCurrent || c.Proposal.GoalItemRef != candidate.GoalInput.CandidateItemRef || c.Proposal.GoalTextHash != StableDefinitionTextHash(candidate.GoalInput.Text) || !sameSpecificationResources(c.Proposal.Resources, resources) || !sameSpecificationDimensions(c.Proposal.Dimensions, dimensions)`；`c.Proposal != nil || c.Review == nil || proposal == nil || c.Review.Kind != id.ReviewKind || c.Review.SpecificationHash != id.ProposalHash || proposal.ContentHash() != id.ProposalHash`；`kind == c.Review.Kind`
- 显式分支集合（只表列源码case，default语义见validator）：`DefinitionAdmissionAdmittedAllOf`；`DefinitionAdmissionClarificationRequired`；`identifier") } } return nil } switch id.Phase { case DefinitionSpecificationProposal`；`DefinitionSpecificationCritic`

<a id="schema-definitionadmissioncandidatedisposition"></a>

### DefinitionAdmissionCandidateDisposition

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:303](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L303)
- 声明：`type DefinitionAdmissionCandidateDisposition string`
- 源码枚举值：`DefinitionAdmissionCandidateCreated = "created"`；`DefinitionAdmissionCandidateDuplicate = "duplicate"`

<a id="schema-definitionadmissionclarificationbinding"></a>

### DefinitionAdmissionClarificationBinding

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:321](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L321)
- 字段：`Candidate workspacemodel.DefinitionAdmissionCandidate`；`AdmissionResult workspacemodel.DefinitionAdmissionResult`
- 字段引用类型：[DefinitionAdmissionCandidate](#schema-definitionadmissioncandidate)；[DefinitionAdmissionResult](#schema-definitionadmissionresult)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L49)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L55)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L62)；[DecisionRequestIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L76)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L121)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L154)
- 空值/identity/版本/范围原式：`err := b.AdmissionResult.ValidateFor(b.Candidate); err != nil`；`b.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired`；`err := b.Validate(); err != nil`；`err := command.Validate(); err != nil`；`err := f.Validate(); err != nil`；`err := binding.Validate(); err != nil`；`err != nil`；`!f.RequestIdentity.EqualNormalized(expected)`；`err := prior.Validate(); err != nil`；`err := admissionResult.ValidateFor(candidate); err != nil`；`f.ResolutionIdentity != expected`
- 显式分支集合（只表列源码case，default语义见validator）：`workspacemodel.DefinitionAdmissionProfileContractInvalid, workspacemodel.DefinitionAdmissionProfilePrerequisiteUnresolved`

<a id="schema-definitionadmissiondisposition"></a>

### DefinitionAdmissionDisposition

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:20](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L20)
- 声明：`type DefinitionAdmissionDisposition string`
- 源码枚举值：`DefinitionAdmissionAdmittedAllOf = "admitted_all_of"`；`DefinitionAdmissionClarificationRequired = "definition_clarification_required"`

<a id="schema-definitionadmissionintentcommand"></a>

### DefinitionAdmissionIntentCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:292](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L292)
- 字段：`Candidate workspacemodel.DefinitionAdmissionCandidate`；`ExpectedCandidateRevision uint64`；`CurrentTemplate bool`；`ExpectedWorkspaceRevision uint64`；`ExpectedConfigurationHash string`；`ClarificationTicket *WorkspaceDefinitionClarificationTicketFact`；`ClarificationResolution *WorkspaceDefinitionClarificationResolutionFact`
- 字段引用类型：[DefinitionAdmissionCandidate](#schema-definitionadmissioncandidate)；[WorkspaceDefinitionClarificationTicketFact](#schema-workspacedefinitionclarificationticketfact)；[WorkspaceDefinitionClarificationResolutionFact](#schema-workspacedefinitionclarificationresolutionfact)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L12)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L25)
- 空值/identity/版本/范围原式：`c.ClarificationTicket != nil`；`c.ClarificationResolution != nil`；`err := c.Candidate.Validate(); err != nil`；`c.CurrentTemplate && (c.ExpectedWorkspaceRevision == 0 || c.ExpectedConfigurationHash == "")`；`c.ClarificationTicket != nil && c.ClarificationResolution != nil`；`err := c.ClarificationTicket.Validate(); err != nil`；`err := c.ClarificationResolution.Validate(); err != nil`

<a id="schema-definitionadmissionintentdisposition"></a>

### DefinitionAdmissionIntentDisposition

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:310](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L310)
- 声明：`type DefinitionAdmissionIntentDisposition string`
- 源码枚举值：`DefinitionAdmissionIntentClarificationRequired = "clarification_required"`；`DefinitionAdmissionIntentClarificationTicketRequired = "clarification_ticket_required"`；`DefinitionAdmissionIntentClarificationResolutionRequired = "clarification_resolution_required"`；`DefinitionAdmissionIntentEligible = "eligible"`

<a id="schema-definitionadmissionintentresult"></a>

### DefinitionAdmissionIntentResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:346](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L346)
- 字段：`Candidate workspacemodel.DefinitionAdmissionCandidate`；`AdmissionResult workspacemodel.DefinitionAdmissionResult`；`CandidateDisposition DefinitionAdmissionCandidateDisposition`；`Disposition DefinitionAdmissionIntentDisposition`；`PriorClarification *DefinitionAdmissionClarificationBinding`；`StartupInput *workspacemodel.WorkspaceStartupInputContract`；`ActivationAdmission *workspacemodel.WorkspaceActivationAdmission`
- 字段引用类型：[DefinitionAdmissionCandidate](#schema-definitionadmissioncandidate)；[DefinitionAdmissionResult](#schema-definitionadmissionresult)；[DefinitionAdmissionCandidateDisposition](#schema-definitionadmissioncandidatedisposition)；[DefinitionAdmissionIntentDisposition](#schema-definitionadmissionintentdisposition)；[DefinitionAdmissionClarificationBinding](#schema-definitionadmissionclarificationbinding)；[WorkspaceStartupInputContract](#schema-workspacestartupinputcontract)；[WorkspaceActivationAdmission](#schema-workspaceactivationadmission)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L206)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L224)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L276)
- 空值/identity/版本/范围原式：`r.PriorClarification != nil`；`r.StartupInput != nil`；`r.ActivationAdmission != nil`；`err := r.AdmissionResult.ValidateFor(r.Candidate); err != nil`；`err := r.PriorClarification.Validate(); err != nil`；`r.PriorClarification.Candidate.WorkspaceID != r.Candidate.WorkspaceID || r.PriorClarification.Candidate.CandidateID != r.Candidate.CandidateID || r.PriorClarification.Candidate.CandidateRevision >= r.Candidate.CandidateRevision`；`(r.StartupInput == nil) != (r.ActivationAdmission == nil)`；`err := r.StartupInput.ValidateCandidate(r.Candidate); err != nil`；`err := r.ActivationAdmission.ValidateCandidate(*r.StartupInput, r.Candidate); err != nil`；`r.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || r.StartupInput != nil`；`r.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf || r.PriorClarification == nil || r.StartupInput != nil`；`r.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf || r.PriorClarification == nil || r.ActivationAdmission == nil || r.ActivationAdmission.State != workspacemodel.WorkspaceActivationPendingClarificationResolution`；`r.AdmissionResult.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf || r.ActivationAdmission == nil || r.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible`；`err := r.Validate(); err != nil`；`r.Candidate.CandidateID != candidate.CandidateID || r.Candidate.CandidateRevision != candidate.CandidateRevision || r.Candidate.CandidateHash != candidate.CandidateHash || r.Candidate.WorkspaceID != candidate.WorkspaceID`
- 显式分支集合（只表列源码case，default语义见validator）：`DefinitionAdmissionCandidateCreated, DefinitionAdmissionCandidateDuplicate`；`DefinitionAdmissionIntentClarificationRequired`；`DefinitionAdmissionIntentClarificationTicketRequired`；`DefinitionAdmissionIntentClarificationResolutionRequired`；`DefinitionAdmissionIntentEligible`

<a id="schema-definitionadmissionpolicybinding"></a>

### DefinitionAdmissionPolicyBinding

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:45](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L45)
- 字段：`PolicyRef string`；`Revision string`；`SnapshotHash string`
- 实际方法：[normalizeAdmissionPolicyBindings](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L747)

<a id="schema-definitionadmissionresult"></a>

### DefinitionAdmissionResult

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:107](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L107)
- 字段：`WorkspaceID common.WorkspaceID`；`CandidateID string`；`CandidateRevision uint64`；`CandidateHash string`；`Disposition DefinitionAdmissionDisposition`；`ViolationCode string`；`OffendingRefs []string`；`CanonicalViolation string`；`CanonicalViolationHash string`；`AdmissionCompilerVersion string`；`CanonicalAdmittedInputHash string`；`AdmissionResultHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAdmissionDisposition](#schema-definitionadmissiondisposition)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L367)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L381)；[WithHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L408)；[computeHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L414)
- 空值/identity/版本/范围原式：`err := candidate.Validate(); err != nil`；`r.WorkspaceID != candidate.WorkspaceID || r.CandidateID != candidate.CandidateID || r.CandidateRevision != candidate.CandidateRevision || r.CandidateHash != candidate.CandidateHash || r.AdmissionCompilerVersion != candidate.AdmissionCompilerVersion`；`r.CanonicalAdmittedInputHash == "" || r.ViolationCode != "" || r.CanonicalViolation != "" || r.CanonicalViolationHash != "" || len(r.OffendingRefs) != 0`；`r.ViolationCode == "" || r.CanonicalViolation == "" || r.CanonicalViolationHash == "" || len(r.OffendingRefs) == 0 || r.CanonicalAdmittedInputHash != ""`；`r.AdmissionResultHash == "" || r.AdmissionResultHash != r.computeHash()`
- 显式分支集合（只表列源码case，default语义见validator）：`DefinitionAdmissionAdmittedAllOf`；`DefinitionAdmissionClarificationRequired`

<a id="schema-definitionauthoringadmissionref"></a>

### DefinitionAuthoringAdmissionRef

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_admission_ref.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_admission_ref.go#L11)
- 字段：`InputHash string`；`DerivationPermitID string`；`ProposalHash string`；`SpecificationHash string`
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_admission_ref.go#L18)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_admission_ref.go#L26)
- 空值/identity/版本/范围原式：`field == "" || len(field) > 256`

<a id="schema-definitionauthoringconfirmation"></a>

### DefinitionAuthoringConfirmation

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go:24](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L24)
- 字段：`Phase DefinitionAuthoringConfirmationPhase`；`InputHash string`；`DerivationPermitID string`；`ProposalHash string`；`SpecificationHash string`；`TemplateSnapshotHash string`；`ResourceSnapshotHash string`；`ConfigurationHash string`；`Goal Goal`；`DoD DoD`；`PreviousInputHash string`；`FormalSequence uint64`
- 字段引用类型：[DefinitionAuthoringConfirmationPhase](#schema-definitionauthoringconfirmationphase)；[Goal](#schema-goal)；[DoD](#schema-dod)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L39)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L59)；[validateMode](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L64)；[ValidateView](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L68)；[validatedSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L72)；[validatedSpecificationMode](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L75)；[ValidateSuccessor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L116)
- 空值/identity/版本/范围原式：`record == nil || record.Input.WorkspaceID != workspaceID || record.validate(view) != nil || workspaceID == "" || c.InputHash != record.InputHash || c.TemplateSnapshotHash != record.TemplateSnapshotHash || c.ConfigurationHash == "" || (c.Phase != DefinitionAuthoringConfirmationPrepared && c.Phase != DefinitionAuthoringConfirmationMaterialized)`；`err != nil || specification.AdmissionHash() != c.SpecificationHash || c.ResourceSnapshotHash != derivation.ResourceSnapshotHash`；`err != nil || len(items) != len(c.DoD.Items) || len(items) == 0`；`c.DoD.Items[n] != item.Text`；`sequence == 0 && c.PreviousInputHash == ""`；`sequence == 0 || sequence == 1 && c.PreviousInputHash != "" || sequence > 1 && c.PreviousInputHash == "" || sequence > 1 && c.PreviousInputHash == c.InputHash`；`sequence > 1`；`previousGoal == "" || previousDoD == "" || c.Goal.PreviousDefinitionHash == "" || c.DoD.PreviousDefinitionHash == ""`；`c.Goal.PreviousGoalID != "" || c.DoD.PreviousDoDID != "" || c.Goal.PreviousDefinitionHash != "" || c.DoD.PreviousDefinitionHash != ""`；`c.Goal.WorkspaceID != workspaceID || c.DoD.WorkspaceID != workspaceID || c.Goal.ID != common.GoalID("authoring-goal-"+StableDefinitionAuthoringConfirmationID(workspaceID, c.InputHash, c.DerivationPermitID, c.SpecificationHash)) || c.DoD.ID != common.DoDID("authoring-dod-"+StableDefinitionAuthoringConfirmationID(workspaceID, c.InputHash, c.DerivationPermitID, c.SpecificationHash)) || c.Goal.Text != strings.TrimSpace(record.Input.GoalText) || c.Goal.DefinitionRevision != authoringFormalRevisionAt("goal", sequence, c.Goal.Text) || c.DoD.DefinitionRevision != authoringFormalRevisionAt("dod", sequence, strings.Join(c.DoD.Items, "\n")) || c.Goal.LineageState != "active" || c.DoD.LineageState != "active" || c.DoD.State != common.DoDStateReady`；`c.FormalSequence < 2 || predecessor.Phase != DefinitionAuthoringConfirmationMaterialized || c.PreviousInputHash != predecessor.InputHash || c.Goal.PreviousGoalID != predecessor.Goal.ID || c.DoD.PreviousDoDID != predecessor.DoD.ID || c.Goal.PreviousDefinitionHash != authoringFormalHash(predecessor.Goal.DefinitionRevision) || c.DoD.PreviousDefinitionHash != authoringFormalHash(predecessor.DoD.DefinitionRevision)`；`previousSequence == 0`；`c.FormalSequence != previousSequence+1`

<a id="schema-definitionauthoringconfirmationphase"></a>

### DefinitionAuthoringConfirmationPhase

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go:14](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L14)
- 声明：`type DefinitionAuthoringConfirmationPhase string`
- 源码枚举值：`DefinitionAuthoringConfirmationPrepared = "prepared"`；`DefinitionAuthoringConfirmationMaterialized = "materialized"`

<a id="schema-definitionauthoringconfirmedrevision"></a>

### DefinitionAuthoringConfirmedRevision

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go:35](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go#L35)
- 字段：`Record DefinitionAuthoringRecord`；`Confirmation workspace.DefinitionAuthoringConfirmation`
- 字段引用类型：[DefinitionAuthoringRecord](#schema-definitionauthoringrecord)；[DefinitionAuthoringConfirmation](#schema-definitionauthoringconfirmation)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go#L164)

<a id="schema-definitionauthoringdiagnostic"></a>

### DefinitionAuthoringDiagnostic

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go:24](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go#L24)
- 字段：`ParseError string`；`ParseErrorSHA256 string`；`ParseErrorBytes uint64`；`ParseErrorSaved bool`；`Payload []byte`；`PayloadSHA256 string`；`PayloadBytes uint64`；`PayloadSaved bool`；`NotSavedReason string`；`ProviderCode string`；`ProviderDocument *DefinitionProviderObservation`；`ProviderWire *DefinitionProviderObservation`；`ProviderCause *DefinitionProviderObservation`
- 字段引用类型：[DefinitionProviderObservation](#schema-definitionproviderobservation)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go#L62)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go#L81)；[ValidateMetadata](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_success_audit.go#L86)
- 空值/identity/版本/范围原式：`d.ProviderCode != "" || d.ProviderDocument != nil || d.ProviderWire != nil || d.ProviderCause != nil`；`len(d.ProviderCode) > 128 || d.ProviderDocument == nil || d.ProviderWire == nil || d.ProviderDocument.Validate() != nil || d.ProviderWire.Validate() != nil || d.ProviderCause != nil && (d.ProviderCode == "" || d.ProviderCause.Validate() != nil || !d.ProviderCause.Observed || !d.ProviderCause.Complete)`；`d.ProviderCode == "" && (!d.ProviderDocument.Observed || !d.ProviderDocument.Complete || !d.ProviderDocument.Saved)`；`!utf8.ValidString(d.ParseError)`；`d.ParseErrorBytes == 0 || d.ParseErrorSHA256 == "" || len(d.ParseErrorSHA256) != sha256.Size*2`；`strings.ToLower(d.ParseErrorSHA256) != d.ParseErrorSHA256`；`_, err := hex.DecodeString(d.ParseErrorSHA256); err != nil`；`d.PayloadSHA256 == "" || len(d.PayloadSHA256) != sha256.Size*2`；`strings.ToLower(d.PayloadSHA256) != d.PayloadSHA256`；`_, err := hex.DecodeString(d.PayloadSHA256); err != nil`；`d.ParseErrorSaved`；`d.ParseErrorBytes > DefinitionAuthoringDiagnosticErrorLimit || uint64(len(d.ParseError)) != d.ParseErrorBytes`；`hex.EncodeToString(h[:]) != d.ParseErrorSHA256`；`d.ParseError != "" || d.ParseErrorBytes <= DefinitionAuthoringDiagnosticErrorLimit`；`d.PayloadSaved`；`d.PayloadBytes > DefinitionAuthoringDiagnosticDocumentLimit || uint64(len(d.Payload)) != d.PayloadBytes || d.Payload == nil`；`hex.EncodeToString(h[:]) != d.PayloadSHA256`；`d.Payload != nil || d.PayloadBytes <= DefinitionAuthoringDiagnosticDocumentLimit`；`d.ParseErrorSaved && d.PayloadSaved`；`d.NotSavedReason != ""`；`d.NotSavedReason != definitionAuthoringDiagnosticNotSavedReason`；`d.ProviderCode == "" && d.ProviderDocument != nil && (d.ProviderDocument.ObservedBytes != d.PayloadBytes || d.ProviderDocument.SHA256 != d.PayloadSHA256)`；`d.ParseError != "" || d.Payload != nil`；`d.ParseErrorBytes == 0 || !validHash(d.ParseErrorSHA256) || !validHash(d.PayloadSHA256)`；`d.ParseErrorSaved != (d.ParseErrorBytes <= DefinitionAuthoringDiagnosticErrorLimit) || d.PayloadSaved != (d.PayloadBytes <= DefinitionAuthoringDiagnosticDocumentLimit)`；`len(d.ProviderCode) > 128 || d.ProviderDocument == nil || d.ProviderWire == nil || d.ProviderDocument.ValidateMetadata() != nil || d.ProviderWire.ValidateMetadata() != nil || d.ProviderCause != nil && (d.ProviderCode == "" || d.ProviderCause.ValidateMetadata() != nil || !d.ProviderCause.Observed || !d.ProviderCause.Complete)`；`d.ProviderCode == "" && (!d.ProviderDocument.Observed || !d.ProviderDocument.Complete || !d.ProviderDocument.Saved || d.ProviderDocument.ObservedBytes != d.PayloadBytes || d.ProviderDocument.SHA256 != d.PayloadSHA256)`

<a id="schema-definitionauthoringdimensionmapping"></a>

### DefinitionAuthoringDimensionMapping

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go:30](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go#L30)
- 字段：`Ref string`；`ResourceIDs []string`

<a id="schema-definitionauthoringgoalmapping"></a>

### DefinitionAuthoringGoalMapping

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go:26](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go#L26)
- 字段：`DimensionRef string`；`ResourceIDs []string`

<a id="schema-definitionauthoringinput"></a>

### DefinitionAuthoringInput

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring.go:178](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring.go#L178)
- 字段：`Version uint32`；`WorkspaceID common.WorkspaceID`；`Revision uint64`；`GoalText string`；`AcceptanceRequirements []string`；`TemplateID string`；`ResourceRefs []string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring.go#L251)；[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring.go#L257)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring.go#L270)；[InputHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring.go#L307)；[validateDerivationRequests](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_derivation.go#L84)；[validateDerivationRequestsMode](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_derivation.go#L87)；[ValidateDefinitionAuthoringResources](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_resources.go#L31)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_derived_acceptance.go#L19)；[ValidateAuthoringSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification.go#L54)；[DefinitionAuthoringSaveRequestHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_save_receipt.go#L21)
- 空值/identity/版本/范围原式：`i.Version != DefinitionAuthoringInputVersionCurrent || i.WorkspaceID == "" || i.Revision == 0`；`!validText(string(i.WorkspaceID), 256) || !validText(i.GoalText, 32*1024) || !validText(i.TemplateID, 256)`；`len(i.AcceptanceRequirements) > 128 || len(i.ResourceRefs) > DefinitionAuthoringResourceLimit`；`!validText(requirement, 4096)`；`!validText(ref, 1024) || seen[ref]`；`total > 128*1024`；`len(records) > DefinitionDerivationRequestLimit`；`err := record.validate(view); err != nil`；`record.Input.WorkspaceID != input.WorkspaceID || record.Input.Revision > input.Revision || seen[record.RequestID]`；`attempts[key] > DefinitionDerivationAttemptLimit`；`input.Validate() != nil || len(resources) != len(input.ResourceRefs) || len(resources) > DefinitionAuthoringResourceLimit`；`resource.Ref != normalized.ResourceRefs[index] || len(resource.Content) > DefinitionAuthoringResourceMaxBytes || !utf8.ValidString(resource.Content) || strings.ContainsRune(resource.Content, 0)`；`resource.ContentHash != "sha256:"+hex.EncodeToString(checksum[:])`；`total > DefinitionAuthoringResourceTotal`；`err := input.Validate(); err != nil`；`template == nil || template.ID != input.Normalized().TemplateID || p.AuthoringInputHash != input.InputHash() || p.TemplateSnapshotHash != WorkspaceTemplateSnapshotHash(template)`；`len(p.AdditionalItems) > 128 || len(p.AdditionalItems)+len(input.AcceptanceRequirements)+len(template.DoDItems) == 0`；`strings.TrimSpace(item) == "" || len(item) > 4096 || !utf8.ValidString(item) || strings.ContainsRune(item, 0)`；`err != nil`；`directory == nil`

<a id="schema-definitionauthoringmapping"></a>

### DefinitionAuthoringMapping

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go:16](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go#L16)
- 字段：`Protocol string`；`InputHash string`；`TemplateSnapshotHash string`；`ResourceSnapshotHash string`；`GoalRequirement DefinitionAuthoringGoalMapping`；`Dimensions []DefinitionAuthoringDimensionMapping`；`Requirements []DefinitionAuthoringRequirementMapping`；`Obligations []DefinitionAuthoringObligationMapping`
- 字段引用类型：[DefinitionAuthoringGoalMapping](#schema-definitionauthoringgoalmapping)；[DefinitionAuthoringDimensionMapping](#schema-definitionauthoringdimensionmapping)；[DefinitionAuthoringRequirementMapping](#schema-definitionauthoringrequirementmapping)；[DefinitionAuthoringObligationMapping](#schema-definitionauthoringobligationmapping)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go#L54)；[MappingCompletionIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping_completion.go#L13)
- 空值/identity/版本/范围原式：`err != nil`

<a id="schema-definitionauthoringobligationmapping"></a>

### DefinitionAuthoringObligationMapping

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go:41](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go#L41)
- 字段：`CandidateItemRef string`；`RequirementRef string`；`Verification DefinitionAuthoringVerificationMapping`
- 字段引用类型：[DefinitionAuthoringVerificationMapping](#schema-definitionauthoringverificationmapping)

<a id="schema-definitionauthoringpreparationdiagnostic"></a>

### DefinitionAuthoringPreparationDiagnostic

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go:30](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go#L30)
- 字段：`DiagnosticID string`；`WorkspaceID common.WorkspaceID`；`InputHash string`；`TemplateSnapshotHash string`；`ObservedWorkspaceRevision uint64`；`Stage DefinitionAuthoringPreparationStage`；`TargetRequestID string`；`DerivationPermitID string`；`ReviewKind string`；`ProposalHash string`；`PhaseCode string`；`ResourceRef string`；`ProviderBindingHash string`；`Cause []byte`；`CauseSHA256 string`；`CauseBytes uint64`；`CauseSaved bool`；`NotSavedReason string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAuthoringPreparationStage](#schema-definitionauthoringpreparationstage)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go#L75)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go#L86)；[ValidateMetadata](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go#L89)；[validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go#L94)；[ValidatePreparationDiagnosticTarget](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go#L285)；[validatePreparationTarget](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go#L292)
- 空值/identity/版本/范围原式：`view && d.Cause != nil`；`!preparationIdentityText(string(d.WorkspaceID), 256, true) || !preparationIdentityText(d.InputHash, 256, true) || !preparationIdentityText(d.TemplateSnapshotHash, 256, true) || d.ObservedWorkspaceRevision == 0 || !preparationIdentityText(d.TargetRequestID, 128, true) || !preparationIdentityText(d.PhaseCode, 128, true) || !preparationIdentityText(d.ResourceRef, 1024, false) || !preparationIdentityText(d.ProviderBindingHash, 256, false) || !preparationIdentityText(d.DerivationPermitID, 256, false) || !preparationIdentityText(d.ProposalHash, 256, false)`；`d.DerivationPermitID != "" || d.ReviewKind != "" || d.ProposalHash != ""`；`d.DerivationPermitID == "" || d.ReviewKind != "" || d.ProposalHash != ""`；`d.DerivationPermitID == "" || !GoalSpecificationReviewKind(d.ReviewKind) || d.ProposalHash == ""`；`d.Stage != DefinitionAuthoringPreparationDerivation`；`len(d.CauseSHA256) != sha256.Size*2 || strings.ToLower(d.CauseSHA256) != d.CauseSHA256`；`_, err := hex.DecodeString(d.CauseSHA256); err != nil`；`d.CauseSaved`；`d.CauseBytes > DefinitionAuthoringPreparationCauseLimit || d.NotSavedReason != "" || !view && (d.Cause == nil || uint64(len(d.Cause)) != d.CauseBytes)`；`!view`；`hex.EncodeToString(h[:]) != d.CauseSHA256`；`d.Cause != nil || d.CauseBytes <= DefinitionAuthoringPreparationCauseLimit || d.NotSavedReason != DefinitionAuthoringPreparationCauseNotSavedReason`；`d.DiagnosticID != d.ExpectedDiagnosticID()`；`d.InputHash != r.InputHash || d.TemplateSnapshotHash != r.TemplateSnapshotHash`；`request.RequestID == d.TargetRequestID`；`d.ResourceRef != "" && !slices.Contains(r.Input.Normalized().ResourceRefs, d.ResourceRef)`；`err != nil || derivation.Input.InputHash() != r.InputHash || WorkspaceTemplateSnapshotHash(&derivation.Template) != r.TemplateSnapshotHash || d.PhaseCode != "config" && d.ProviderBindingHash != "" && d.ProviderBindingHash != derivation.ProviderBindingHash`；`d.Stage == DefinitionAuthoringPreparationReview && r.completedSpecificationProposal(d.DerivationPermitID, d.ProposalHash) == nil`；`d.PhaseCode == "request_budget" || d.PhaseCode == "attempt_budget"`；`d.ProviderBindingHash == ""`；`d.Stage == DefinitionAuthoringPreparationDerivation`；`request.Input.InputHash() == r.InputHash && WorkspaceTemplateSnapshotHash(&request.Template) == r.TemplateSnapshotHash`；`d.Stage == DefinitionAuthoringPreparationReview`；`request.DerivationPermitID == d.DerivationPermitID && request.Phase == phase && request.ReviewKind == d.ReviewKind`；`d.PhaseCode == "request_budget" && count < limit || d.PhaseCode == "attempt_budget" && (count >= limit || attempts < attemptLimit)`
- 显式分支集合（只表列源码case，default语义见validator）：`DefinitionAuthoringPreparationDerivation`；`DefinitionAuthoringPreparationProposal`；`DefinitionAuthoringPreparationReview`；`"resources"`；`"config", "render", "request_budget", "attempt_budget"`

<a id="schema-definitionauthoringpreparationdiagnosticmetadata"></a>

### DefinitionAuthoringPreparationDiagnosticMetadata

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L15)
- 字段：`DiagnosticID string`；`WorkspaceID common.WorkspaceID`；`InputHash string`；`TemplateSnapshotHash string`；`ObservedWorkspaceRevision uint64`；`Stage workspace.DefinitionAuthoringPreparationStage`；`TargetRequestID string`；`DerivationPermitID string`；`ReviewKind string`；`ProposalHash string`；`PhaseCode string`；`ResourceRef string`；`ProviderBindingHash string`；`Cause []byte`；`CauseSHA256 string`；`CauseBytes uint64`；`CauseSaved bool`；`NotSavedReason string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAuthoringPreparationStage](#schema-definitionauthoringpreparationstage)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L45)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L51)

<a id="schema-definitionauthoringpreparationstage"></a>

### DefinitionAuthoringPreparationStage

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go:20](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_preparation_diagnostic.go#L20)
- 声明：`type DefinitionAuthoringPreparationStage string`
- 源码枚举值：`DefinitionAuthoringPreparationDerivation = "derivation"`；`DefinitionAuthoringPreparationProposal = "proposal"`；`DefinitionAuthoringPreparationReview = "review"`

<a id="schema-definitionauthoringrecord"></a>

### DefinitionAuthoringRecord

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go:24](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go#L24)
- 字段：`Input workspace.DefinitionAuthoringInput`；`InputHash string`；`TemplateSnapshotHash string`；`TemplateSnapshot WorkspaceTemplate`；`DerivationRequests []DefinitionDerivationRequest`；`SpecificationRequests []workspace.DefinitionAuthoringSpecificationRequest`；`SaveReceipts []workspace.DefinitionAuthoringSaveReceipt`；`PreparationDiagnostics []DefinitionAuthoringPreparationDiagnosticMetadata`
- 字段引用类型：[DefinitionAuthoringInput](#schema-definitionauthoringinput)；[WorkspaceTemplate](#schema-workspacetemplate)；[DefinitionDerivationRequest](#schema-definitionderivationrequest)；[DefinitionAuthoringSpecificationRequest](#schema-definitionauthoringspecificationrequest)；[DefinitionAuthoringSaveReceipt](#schema-definitionauthoringsavereceipt)；[DefinitionAuthoringPreparationDiagnosticMetadata](#schema-definitionauthoringpreparationdiagnosticmetadata)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go#L104)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go#L156)
- 空值/identity/版本/范围原式：`err != nil`

<a id="schema-definitionauthoringrequirementmapping"></a>

### DefinitionAuthoringRequirementMapping

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go:34](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go#L34)
- 字段：`Ref string`；`DimensionRef string`；`SourceItemRefs []string`；`ResourceIDs []string`；`Text string`

<a id="schema-definitionauthoringresourcesnapshot"></a>

### DefinitionAuthoringResourceSnapshot

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_resources.go:21](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_resources.go#L21)
- 字段：`Ref string`；`Content string`；`ContentHash string`
- 实际方法：[CloneDefinitionAuthoringResources](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_resources.go#L27)；[ValidateDefinitionAuthoringResources](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_resources.go#L31)；[DefinitionAuthoringResourcesHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_resources.go#L53)；[ValidateAuthoringSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification.go#L54)
- 空值/identity/版本/范围原式：`input.Validate() != nil || len(resources) != len(input.ResourceRefs) || len(resources) > DefinitionAuthoringResourceLimit`；`resource.Ref != normalized.ResourceRefs[index] || len(resource.Content) > DefinitionAuthoringResourceMaxBytes || !utf8.ValidString(resource.Content) || strings.ContainsRune(resource.Content, 0)`；`resource.ContentHash != "sha256:"+hex.EncodeToString(checksum[:])`；`total > DefinitionAuthoringResourceTotal`；`err != nil`

<a id="schema-definitionauthoringsavereceipt"></a>

### DefinitionAuthoringSaveReceipt

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_save_receipt.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_save_receipt.go#L13)
- 字段：`RequestID string`；`RequestHash string`；`InputHash string`；`TemplateSnapshotHash string`；`WorkspaceRevision uint64`
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_save_receipt.go#L35)
- 空值/identity/版本/范围原式：`strings.TrimSpace(r.RequestID) != r.RequestID || r.RequestID == "" || len(r.RequestID) > 256 || strings.ContainsRune(r.RequestID, 0) || !validAuthoringSaveHash(r.RequestHash, "authoring-save-sha256:") || !validAuthoringSaveHash(r.InputHash, "definition-authoring-sha256:") || !validAuthoringSaveHash(r.TemplateSnapshotHash, "template-snapshot-sha256:") || r.WorkspaceRevision == 0`

<a id="schema-definitionauthoringspecificationrequest"></a>

### DefinitionAuthoringSpecificationRequest

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go:148](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go#L148)
- 字段：`RequestID string`；`DerivationPermitID string`；`Phase DefinitionSpecificationPhase`；`ReviewKind string`；`ProposalHash string`；`ProviderBindingHash string`；`PermitID string`；`Proposal *GoalSpecification`；`Mapping *DefinitionAuthoringMapping`；`SuccessAudit *DefinitionAuthoringSuccessByteAudit`；`CompletionIdentity string`；`Review *GoalSpecificationCompletenessReview`；`FailureCode string`；`Diagnostic *DefinitionAuthoringDiagnostic`
- 字段引用类型：[DefinitionSpecificationPhase](#schema-definitionspecificationphase)；[GoalSpecification](#schema-goalspecification)；[DefinitionAuthoringMapping](#schema-definitionauthoringmapping)；[DefinitionAuthoringSuccessByteAudit](#schema-definitionauthoringsuccessbyteaudit)；[GoalSpecificationCompletenessReview](#schema-goalspecificationcompletenessreview)；[DefinitionAuthoringDiagnostic](#schema-definitionauthoringdiagnostic)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go#L165)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go#L197)；[ValidateView](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go#L200)；[validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go#L201)
- 空值/identity/版本/范围原式：`r.Proposal != nil`；`r.Mapping == nil || r.SuccessAudit == nil`；`view`；`r.SuccessAudit.ValidateMetadata() != nil`；`r.SuccessAudit.Validate() != nil`；`r.Mapping != nil || r.SuccessAudit != nil`；`r.Diagnostic != nil`；`r.Diagnostic.ValidateMetadata() != nil`；`r.Diagnostic.Validate() != nil`；`r.RequestID == "" || len(r.RequestID) > 128 || r.DerivationPermitID == "" || len(r.DerivationPermitID) > 256 || r.ProviderBindingHash == "" || len(r.ProviderBindingHash) > 256 || r.PermitID != r.ExpectedPermitID()`；`r.ReviewKind != "" || r.ProposalHash != ""`；`!GoalSpecificationReviewKind(r.ReviewKind) || r.ProposalHash == "" || len(r.ProposalHash) > 256`；`r.CompletionIdentity != "" && (r.Phase != DefinitionSpecificationProposal || r.Proposal == nil && (r.FailureCode != "invalid_proposal" || r.Diagnostic == nil))`；`r.Proposal != nil && r.CompletionIdentity != MappingCompletionIdentity(r.PermitID, *r.Mapping, *r.SuccessAudit)`；`r.FailureCode != ""`；`r.Diagnostic != nil && (r.FailureCode == "provider_failed" && r.Diagnostic.ProviderCode == "")`；`r.FailureCode != "provider_failed" && r.FailureCode != "invalid_proposal" && r.FailureCode != "invalid_review" || r.Proposal != nil || r.Review != nil`；`r.Phase == DefinitionSpecificationProposal && r.FailureCode == "invalid_review" || r.Phase == DefinitionSpecificationCritic && r.FailureCode == "invalid_proposal"`；`r.Diagnostic != nil || r.Proposal != nil && r.Review != nil || r.Proposal != nil && (r.Phase != DefinitionSpecificationProposal || len(r.Proposal.Reviews) != 0) || r.Review != nil && (r.Phase != DefinitionSpecificationCritic || r.Review.Kind != r.ReviewKind || r.Review.SpecificationHash != r.ProposalHash)`
- 显式分支集合（只表列源码case，default语义见validator）：`DefinitionSpecificationProposal`；`DefinitionSpecificationCritic`

<a id="schema-definitionauthoringsuccessbyteaudit"></a>

### DefinitionAuthoringSuccessByteAudit

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_success_audit.go:12](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_success_audit.go#L12)
- 字段：`Document DefinitionProviderObservation`；`Wire DefinitionProviderObservation`
- 字段引用类型：[DefinitionProviderObservation](#schema-definitionproviderobservation)
- 实际方法：[MappingCompletionIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping_completion.go#L13)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_success_audit.go#L17)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_success_audit.go#L22)；[ValidateMetadata](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_success_audit.go#L33)
- 空值/identity/版本/范围原式：`err != nil`；`a.Document.Validate() != nil || !a.Document.Observed || !a.Document.Complete || !a.Document.Saved || a.Document.ObservedBytes == 0 || a.Wire.Validate() != nil`；`a.Document.ValidateMetadata() != nil || !a.Document.Observed || !a.Document.Complete || !a.Document.Saved || a.Document.ObservedBytes == 0 || a.Wire.ValidateMetadata() != nil`

<a id="schema-definitionauthoringverificationmapping"></a>

### DefinitionAuthoringVerificationMapping

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go:46](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_mapping.go#L46)
- 字段：`Declaration string`；`Status string`；`ProfileID string`；`EvidenceRequirementID string`；`Scope string`

<a id="schema-definitioncandidateitem"></a>

### DefinitionCandidateItem

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:33](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L33)
- 字段：`CandidateItemRef string`；`Text string`；`SourceIdentity string`
- 实际方法：[validatedSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L72)；[validatedSpecificationMode](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L75)；[normalizeCandidateItem](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L731)；[normalizeCandidateItems](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L738)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L251)
- 空值/identity/版本/范围原式：`record == nil || record.Input.WorkspaceID != workspaceID || record.validate(view) != nil || workspaceID == "" || c.InputHash != record.InputHash || c.TemplateSnapshotHash != record.TemplateSnapshotHash || c.ConfigurationHash == "" || (c.Phase != DefinitionAuthoringConfirmationPrepared && c.Phase != DefinitionAuthoringConfirmationMaterialized)`；`err != nil || specification.AdmissionHash() != c.SpecificationHash || c.ResourceSnapshotHash != derivation.ResourceSnapshotHash`；`err != nil || len(items) != len(c.DoD.Items) || len(items) == 0`；`c.DoD.Items[n] != item.Text`；`sequence == 0 && c.PreviousInputHash == ""`；`sequence == 0 || sequence == 1 && c.PreviousInputHash != "" || sequence > 1 && c.PreviousInputHash == "" || sequence > 1 && c.PreviousInputHash == c.InputHash`；`sequence > 1`；`previousGoal == "" || previousDoD == "" || c.Goal.PreviousDefinitionHash == "" || c.DoD.PreviousDefinitionHash == ""`；`c.Goal.PreviousGoalID != "" || c.DoD.PreviousDoDID != "" || c.Goal.PreviousDefinitionHash != "" || c.DoD.PreviousDefinitionHash != ""`；`c.Goal.WorkspaceID != workspaceID || c.DoD.WorkspaceID != workspaceID || c.Goal.ID != common.GoalID("authoring-goal-"+StableDefinitionAuthoringConfirmationID(workspaceID, c.InputHash, c.DerivationPermitID, c.SpecificationHash)) || c.DoD.ID != common.DoDID("authoring-dod-"+StableDefinitionAuthoringConfirmationID(workspaceID, c.InputHash, c.DerivationPermitID, c.SpecificationHash)) || c.Goal.Text != strings.TrimSpace(record.Input.GoalText) || c.Goal.DefinitionRevision != authoringFormalRevisionAt("goal", sequence, c.Goal.Text) || c.DoD.DefinitionRevision != authoringFormalRevisionAt("dod", sequence, strings.Join(c.DoD.Items, "\n")) || c.Goal.LineageState != "active" || c.DoD.LineageState != "active" || c.DoD.State != common.DoDStateReady`；`s.Version != GoalSpecificationVersionCurrent || s.GoalItemRef != goal.CandidateItemRef || s.GoalTextHash != StableDefinitionTextHash(goal.Text)`；`len(s.Resources) > 128 || len(s.Dimensions) > 64 || len(s.Requirements) > 256 || len(s.Obligations) > 512 || len(s.Verifications) > 512 || len(s.Reviews) != len(goalReviewKinds)`；`id == "" || identities[id]`；`!claimID(r.ID) || r.Ref == "" || len(r.Ref) > 1024 || len(r.SnapshotHash) > 256 || len(r.ID) > 128`；`!claimID(d.ID) || len(d.ID) > 128 || !specRefsExist(d.ResourceIDs, func(ref string) bool { _, ok := resources[ref]; return ok })`；`r.Required && r.SourceKind == GoalRequirementUserExplicit && slices.Equal(r.SourceItemRefs, []string{goal.CandidateItemRef}) && r.Text == goal.Text`；`!claimID(r.ID) || len(r.ID) > 128 || !hasDimension || len(r.Text) > textLimit || len(r.SourceItemRefs) > 32 || len(r.ResourceIDs) > 128 || !specRefsExist(r.SourceItemRefs, func(ref string) bool { return items[ref] }) || !specRefsExist(r.ResourceIDs, func(ref string) bool { _, ok := resources[ref]; return ok })`；`!claimID(o.ID) || len(o.ID) > 128 || !hasRequirement || len(o.Text) > 4096 || len(o.DoDItemRefs) > 32 || !specRefsExist(o.DoDItemRefs, func(ref string) bool { return dodRefs[ref] })`；`!claimID(v.ID) || len(v.ID) > 128 || !hasObligation || len(v.Declaration) > 4096`；`v.ProfileID == "" || v.EvidenceRequirementID == "" || v.Scope == ""`；`v.ProfileID != "" || v.EvidenceRequirementID != "" || v.Scope != ""`；`!allowedReview[r.Kind] || reviews[r.Kind]`；`err := context.validate(r); err != nil`
- 显式分支集合（只表列源码case，default语义见validator）：`GoalRequirementUserExplicit, GoalRequirementTemplateRequired, GoalRequirementTemplateConditional, GoalRequirementDerived, GoalRequirementDiscovered`；`GoalVerificationDeclared`；`GoalVerificationUnverified`

<a id="schema-definitionchangecandidate"></a>

### DefinitionChangeCandidate

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_change.go:33](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L33)
- 字段：`RequestID string`；`ExpectedWorkspaceRevision uint64`；`RecordedWorkspaceRevision uint64`；`TicketID common.TicketID`；`ProposalRevision uint64`；`ProposalHash string`；`BaseDefinitionRevision string`；`BaseStartupInputHash string`；`BaseInputHash string`；`Record DefinitionAuthoringRecord`
- 字段引用类型：[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[DefinitionAuthoringRecord](#schema-definitionauthoringrecord)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L46)

<a id="schema-definitionchangereviewedidentity"></a>

### DefinitionChangeReviewedIdentity

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_change_reviewed.go:12](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_change_reviewed.go#L12)
- 字段：`ID string`；`Revision uint64`；`Hash string`；`InputHash string`；`TemplateSnapshotHash string`；`DerivationPermitID string`；`ProposalHash string`；`SpecificationHash string`
- 实际方法：[ReviewedIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_change_reviewed.go#L23)；[ReviewedIdentityView](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_change_reviewed.go#L31)；[reviewedIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_change_reviewed.go#L35)
- 空值/identity/版本/范围原式：`c.Validate(w) != nil`；`c.Record.Input.WorkspaceID != id`；`request.Input.InputHash() != c.Record.InputHash`；`request.Proposal == nil || request.FailureCode != "" || derivation != nil`；`derivation == nil`；`request.DerivationPermitID != derivation.PermitID`；`request.FailureCode != "" || request.Proposal == nil && request.Review == nil`；`request.Proposal != nil`；`proposalHash != ""`；`view`；`err != nil`

<a id="schema-definitionderivationrequest"></a>

### DefinitionDerivationRequest

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go:11](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go#L11)
- 字段：`RequestID string`；`Input workspace.DefinitionAuthoringInput`；`Template WorkspaceTemplate`；`Resources []workspace.DefinitionAuthoringResourceSnapshot`；`ResourceSnapshotHash string`；`ProviderBindingHash string`；`PermitID string`；`Proposal *workspace.DefinitionDerivedAcceptance`；`FailureCode string`；`Diagnostic *workspace.DefinitionAuthoringDiagnostic`
- 字段引用类型：[DefinitionAuthoringInput](#schema-definitionauthoringinput)；[WorkspaceTemplate](#schema-workspacetemplate)；[DefinitionAuthoringResourceSnapshot](#schema-definitionauthoringresourcesnapshot)；[DefinitionDerivedAcceptance](#schema-definitionderivedacceptance)；[DefinitionAuthoringDiagnostic](#schema-definitionauthoringdiagnostic)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go#L65)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go#L96)
- 空值/identity/版本/范围原式：`err != nil`

<a id="schema-definitionderivationrequestresult"></a>

### DefinitionDerivationRequestResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_derivation.go:52](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L52)
- 字段：`Request DefinitionDerivationRequest`；`FreshPermitID string`；`Consumed uint32`；`Remaining uint32`
- 字段引用类型：[DefinitionDerivationRequest](#schema-definitionderivationrequest)
- 实际方法：[validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L59)；[ValidateReserve](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L72)；[ValidateCompletion](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L85)
- 空值/identity/版本/范围原式：`err := r.Request.Validate(); err != nil`；`r.Consumed == 0 || r.Consumed > workspace.DefinitionDerivationRequestLimit || r.Remaining != workspace.DefinitionDerivationRequestLimit-r.Consumed`；`r.FreshPermitID != "" && (r.FreshPermitID != r.Request.PermitID || r.Request.Proposal != nil || r.Request.FailureCode != "")`；`err := c.Validate(); err != nil`；`err := r.validate(); err != nil`；`r.Request.Input.WorkspaceID != c.WorkspaceID || r.Request.Input.InputHash() != c.InputHash || r.Request.RequestID != c.RequestID || r.Request.ProviderBindingHash != c.ProviderBindingHash || r.Request.ResourceSnapshotHash != c.ResourceSnapshotHash || !reflect.DeepEqual(r.Request.Resources, c.Resources)`；`c.Diagnostic != nil`；`r.FreshPermitID != "" || r.Request.Input.WorkspaceID != c.WorkspaceID || r.Request.PermitID != c.PermitID || r.Request.FailureCode != c.FailureCode || !reflect.DeepEqual(r.Request.Proposal, c.Proposal) || !reflect.DeepEqual(r.Request.Diagnostic, diagnostic)`

<a id="schema-definitionderivedacceptance"></a>

### DefinitionDerivedAcceptance

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_derived_acceptance.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_derived_acceptance.go#L13)
- 字段：`AuthoringInputHash string`；`TemplateSnapshotHash string`；`AdditionalItems []string`
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_derived_acceptance.go#L19)；[ContentHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_derived_acceptance.go#L61)；[ValidateAuthoringSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification.go#L54)
- 空值/identity/版本/范围原式：`err := input.Validate(); err != nil`；`template == nil || template.ID != input.Normalized().TemplateID || p.AuthoringInputHash != input.InputHash() || p.TemplateSnapshotHash != WorkspaceTemplateSnapshotHash(template)`；`len(p.AdditionalItems) > 128 || len(p.AdditionalItems)+len(input.AcceptanceRequirements)+len(template.DoDItems) == 0`；`strings.TrimSpace(item) == "" || len(item) > 4096 || !utf8.ValidString(item) || strings.ContainsRune(item, 0)`；`total > 128*1024`；`err != nil`

<a id="schema-definitionownerfactconvergencebinding"></a>

### DefinitionOwnerFactConvergenceBinding

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:62](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L62)
- 字段：`SourceRef string`；`Convergence runtimecontract.RuntimeOwnerFactConvergence`
- 字段引用类型：[RuntimeOwnerFactConvergence（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_owner_fact.go#L40)
- 实际方法：[normalizeDefinitionOwnerFactConvergenceBindings](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L775)
- 空值/identity/版本/范围原式：`ret[i].SourceRef != ret[j].SourceRef`

<a id="schema-definitionproposedrelationinput"></a>

### DefinitionProposedRelationInput

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:67](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L67)
- 字段：`RelationRef string`；`RelationKind string`；`SourceItemRefs []string`；`TargetItemRefs []string`
- 实际方法：[normalizeProposedRelations](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L792)

<a id="schema-definitionproviderobservation"></a>

### DefinitionProviderObservation

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_provider_observation.go:10](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_provider_observation.go#L10)
- 字段：`Content []byte`；`ObservedBytes uint64`；`SHA256 string`；`Observed bool`；`Complete bool`；`Saved bool`；`Reason string`
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_provider_observation.go#L20)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_provider_observation.go#L27)；[ValidateMetadata](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_success_audit.go#L39)
- 空值/identity/版本/范围原式：`!o.Observed`；`o.Content != nil || o.ObservedBytes != 0 || o.SHA256 != "" || o.Complete || o.Saved || o.Reason == ""`；`err != nil || len(hash) != sha256.Size || hex.EncodeToString(hash) != o.SHA256`；`!o.Complete && o.Reason == ""`；`o.Saved`；`o.Content == nil || uint64(len(o.Content)) != o.ObservedBytes || len(o.Content) > DefinitionAuthoringDiagnosticDocumentLimit`；`hex.EncodeToString(sum[:]) != o.SHA256`；`o.Content != nil || o.ObservedBytes <= DefinitionAuthoringDiagnosticDocumentLimit || o.Reason != "observation_storage_limit"`；`o.Content != nil`；`o.ObservedBytes != 0 || o.SHA256 != "" || o.Complete || o.Saved || o.Reason == ""`；`o.ObservedBytes > DefinitionAuthoringDiagnosticDocumentLimit`；`o.ObservedBytes <= DefinitionAuthoringDiagnosticDocumentLimit || o.Reason != "observation_storage_limit"`

<a id="schema-definitionrequiredownerfactbinding"></a>

### DefinitionRequiredOwnerFactBinding

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:54](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L54)
- 字段：`SourceRef string`；`Requirement runtimecontract.RuntimeOwnerFactRequirement`
- 字段引用类型：[RuntimeOwnerFactRequirement（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_owner_fact.go#L28)
- 实际方法：[normalizeDefinitionRequiredOwnerFactBindings](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L758)
- 空值/identity/版本/范围原式：`ret[i].SourceRef != ret[j].SourceRef`

<a id="schema-definitionspecificationcompilationref"></a>

### DefinitionSpecificationCompilationRef

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go:250](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L250)
- 字段：`InputCandidateHash string`；`CompilationInputHash string`；`ProposalPermitID string`；`ProposalHash string`；`CriticPermitIDs [6]string`
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L258)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L268)
- 空值/identity/版本/范围原式：`r.InputCandidateHash == "" || r.CompilationInputHash == "" || r.ProposalPermitID == "" || r.ProposalHash == "" || len(r.InputCandidateHash) > 256 || len(r.CompilationInputHash) > 256 || len(r.ProposalPermitID) > 256 || len(r.ProposalHash) > 256`；`permit == "" || len(permit) > 256 || seen[permit]`

<a id="schema-definitionspecificationphase"></a>

### DefinitionSpecificationPhase

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go:19](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L19)
- 声明：`type DefinitionSpecificationPhase string`
- 源码枚举值：`DefinitionSpecificationProposal = "proposal"`；`DefinitionSpecificationCritic = "critic"`

<a id="schema-definitionspecificationrequestcompletion"></a>

### DefinitionSpecificationRequestCompletion

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go:153](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L153)
- 字段：`Proposal *GoalSpecification`；`Review *GoalSpecificationCompletenessReview`；`FailureCode string`
- 字段引用类型：[GoalSpecification](#schema-goalspecification)；[GoalSpecificationCompletenessReview](#schema-goalspecificationcompletenessreview)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L159)；[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L172)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L191)
- 空值/identity/版本/范围原式：`c.Proposal != nil`；`c.Review != nil`；`c.FailureCode != ""`；`c.Proposal != nil || c.Review != nil || len(c.FailureCode) > 96`；`(r < 'a' || r > 'z') && r != '_' && (r < '0' || r > '9')`；`c.Proposal == nil || c.Review != nil || len(c.Proposal.Reviews) != 0 || c.Proposal.Version != GoalSpecificationVersionCurrent || c.Proposal.GoalItemRef != candidate.GoalInput.CandidateItemRef || c.Proposal.GoalTextHash != StableDefinitionTextHash(candidate.GoalInput.Text) || !sameSpecificationResources(c.Proposal.Resources, resources) || !sameSpecificationDimensions(c.Proposal.Dimensions, dimensions)`；`c.Proposal != nil || c.Review == nil || proposal == nil || c.Review.Kind != id.ReviewKind || c.Review.SpecificationHash != id.ProposalHash || proposal.ContentHash() != id.ProposalHash`；`kind == c.Review.Kind`
- 显式分支集合（只表列源码case，default语义见validator）：`identifier") } } return nil } switch id.Phase { case DefinitionSpecificationProposal`；`DefinitionSpecificationCritic`

<a id="schema-definitionspecificationrequestdisposition"></a>

### DefinitionSpecificationRequestDisposition

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_specification.go:69](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L69)
- 声明：`type DefinitionSpecificationRequestDisposition string`
- 源码枚举值：`DefinitionSpecificationPermitted = "permitted"`；`DefinitionSpecificationAlreadyReserved = "already_reserved"`；`DefinitionSpecificationAlreadyCompleted = "already_completed"`

<a id="schema-definitionspecificationrequestidentity"></a>

### DefinitionSpecificationRequestIdentity

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go:35](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L35)
- 字段：`WorkspaceID common.WorkspaceID`；`CandidateID string`；`CandidateRevision uint64`；`CandidateHash string`；`CompilationInputHash string`；`Phase DefinitionSpecificationPhase`；`ReviewKind string`；`LogicalRequestID string`；`ProposalHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionSpecificationPhase](#schema-definitionspecificationphase)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L47)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L57)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L191)
- 空值/identity/版本/范围原式：`i.WorkspaceID == "" || i.CandidateID == "" || i.CandidateRevision == 0 || i.CandidateHash == "" || i.CompilationInputHash == "" || i.LogicalRequestID == "" || len(i.CandidateID) > 128 || len(i.CandidateHash) > 256 || len(i.CompilationInputHash) > 256 || len(i.LogicalRequestID) > 128 || len(i.ProposalHash) > 256`；`i.ReviewKind != "" || i.ProposalHash != ""`；`!GoalSpecificationReviewKind(i.ReviewKind) || i.ProposalHash == ""`；`c.FailureCode != ""`；`c.Proposal != nil || c.Review != nil || len(c.FailureCode) > 96`；`(r < 'a' || r > 'z') && r != '_' && (r < '0' || r > '9')`；`c.Proposal == nil || c.Review != nil || len(c.Proposal.Reviews) != 0 || c.Proposal.Version != GoalSpecificationVersionCurrent || c.Proposal.GoalItemRef != candidate.GoalInput.CandidateItemRef || c.Proposal.GoalTextHash != StableDefinitionTextHash(candidate.GoalInput.Text) || !sameSpecificationResources(c.Proposal.Resources, resources) || !sameSpecificationDimensions(c.Proposal.Dimensions, dimensions)`；`c.Proposal != nil || c.Review == nil || proposal == nil || c.Review.Kind != id.ReviewKind || c.Review.SpecificationHash != id.ProposalHash || proposal.ContentHash() != id.ProposalHash`；`kind == c.Review.Kind`
- 显式分支集合（只表列源码case，default语义见validator）：`DefinitionSpecificationProposal`；`DefinitionSpecificationCritic`；`identifier") } } return nil } switch id.Phase { case DefinitionSpecificationProposal`

<a id="schema-definitionspecificationrequestresult"></a>

### DefinitionSpecificationRequestResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_specification.go:79](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L79)
- 字段：`Identity workspacemodel.DefinitionSpecificationRequestIdentity`；`Disposition DefinitionSpecificationRequestDisposition`；`PermitID string`；`Completed *workspacemodel.DefinitionSpecificationRequestCompletion`；`Consumed uint8`；`Remaining uint8`
- 字段引用类型：[DefinitionSpecificationRequestIdentity](#schema-definitionspecificationrequestidentity)；[DefinitionSpecificationRequestDisposition](#schema-definitionspecificationrequestdisposition)；[DefinitionSpecificationRequestCompletion](#schema-definitionspecificationrequestcompletion)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L88)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L113)
- 空值/identity/版本/范围原式：`identity.Validate() != nil || r.Identity != identity.Normalized() || r.Consumed == 0 || r.Consumed > workspacemodel.DefinitionSpecificationRequestLimit || r.Remaining != workspacemodel.DefinitionSpecificationRequestLimit-r.Consumed`；`r.PermitID != identity.PermitID() || r.Completed != nil`；`r.PermitID != "" || r.Completed != nil`；`r.PermitID != "" || r.Completed == nil`
- 显式分支集合（只表列源码case，default语义见validator）：`DefinitionSpecificationPermitted`；`DefinitionSpecificationAlreadyReserved`；`DefinitionSpecificationAlreadyCompleted`

<a id="schema-definitiontemplatesnapshot"></a>

### DefinitionTemplateSnapshot

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:39](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L39)
- 字段：`TemplateRef string`；`Revision string`；`SnapshotHash string`

<a id="schema-deletedodcommand"></a>

### DeleteDoDCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:232](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L232)
- 字段：`WorkspaceID common.WorkspaceID`；`DoDID common.DoDID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L200)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID) || !validDoDResultID(command.DoDID) || !validWorkspaceResultID(r.WorkspaceID) || !validDoDResultID(r.DoDID)`；`r.WorkspaceID != command.WorkspaceID || r.DoDID != command.DoDID`

<a id="schema-deletedodresult"></a>

### DeleteDoDResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:406](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L406)
- 字段：`WorkspaceID common.WorkspaceID`；`DoDID common.DoDID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L200)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID) || !validDoDResultID(command.DoDID) || !validWorkspaceResultID(r.WorkspaceID) || !validDoDResultID(r.DoDID)`；`r.WorkspaceID != command.WorkspaceID || r.DoDID != command.DoDID`

<a id="schema-deletegoalcommand"></a>

### DeleteGoalCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:218](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L218)
- 字段：`WorkspaceID common.WorkspaceID`；`GoalID common.GoalID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L156)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID) || !validGoalResultID(command.GoalID) || !validWorkspaceResultID(r.WorkspaceID) || !validGoalResultID(r.GoalID)`；`r.WorkspaceID != command.WorkspaceID || r.GoalID != command.GoalID`

<a id="schema-deletegoalresult"></a>

### DeleteGoalResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:400](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L400)
- 字段：`WorkspaceID common.WorkspaceID`；`GoalID common.GoalID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L156)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID) || !validGoalResultID(command.GoalID) || !validWorkspaceResultID(r.WorkspaceID) || !validGoalResultID(r.GoalID)`；`r.WorkspaceID != command.WorkspaceID || r.GoalID != command.GoalID`

<a id="schema-deleteworkspacecommand"></a>

### DeleteWorkspaceCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:103](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L103)
- 字段：`WorkspaceID common.WorkspaceID`；`RequestID string`；`Purge bool`；`ExpectedRevision uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L110)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L116)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L192)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.RequestID == ""`；`err := command.Validate(); err != nil`；`r.WorkspaceID != command.WorkspaceID || strings.TrimSpace(r.DeletionID) != StableWorkspaceDeletionID(command.WorkspaceID, command.RequestID) || r.RequestID != command.RequestID || r.Mode != command.Mode() || r.DeletionRevision == 0 || r.WorkspaceRevision == 0 || !r.Completed`
- 显式分支集合（只表列源码case，default语义见validator）：`DeleteWorkspaceCompleted, DeleteWorkspaceDuplicate`

<a id="schema-deleteworkspacedisposition"></a>

### DeleteWorkspaceDisposition

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:137](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L137)
- 声明：`type DeleteWorkspaceDisposition string`
- 源码枚举值：`DeleteWorkspaceCompleted = "completed"`；`DeleteWorkspaceDuplicate = "duplicate"`

<a id="schema-deleteworkspaceresult"></a>

### DeleteWorkspaceResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:146](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L146)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`RequestID string`；`Mode deletioncontract.Mode`；`Disposition DeleteWorkspaceDisposition`；`DeletionRevision uint64`；`WorkspaceRevision uint64`；`Completed bool`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)；[DeleteWorkspaceDisposition](#schema-deleteworkspacedisposition)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L192)
- 空值/identity/版本/范围原式：`err := command.Validate(); err != nil`；`r.WorkspaceID != command.WorkspaceID || strings.TrimSpace(r.DeletionID) != StableWorkspaceDeletionID(command.WorkspaceID, command.RequestID) || r.RequestID != command.RequestID || r.Mode != command.Mode() || r.DeletionRevision == 0 || r.WorkspaceRevision == 0 || !r.Completed`
- 显式分支集合（只表列源码case，default语义见validator）：`DeleteWorkspaceCompleted, DeleteWorkspaceDuplicate`

<a id="schema-deleteworkspacetemplatecommand"></a>

### DeleteWorkspaceTemplateCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:250](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L250)
- 字段：`TemplateID string`
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L238)
- 空值/identity/版本/范围原式：`!validTemplateResultID(command.TemplateID) || !validTemplateResultID(r.TemplateID)`；`r.TemplateID != command.TemplateID`

<a id="schema-deleteworkspacetemplateresult"></a>

### DeleteWorkspaceTemplateResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:412](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L412)
- 字段：`TemplateID string`
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L238)
- 空值/identity/版本/范围原式：`!validTemplateResultID(command.TemplateID) || !validTemplateResultID(r.TemplateID)`；`r.TemplateID != command.TemplateID`

<a id="schema-dod"></a>

### DoD

- 源码：[internal/modules/kernel/workspace/pkg/models/dod.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/dod.go#L5)
- 字段：`ID common.DoDID`；`WorkspaceID common.WorkspaceID`；`Items []string`；`State common.DoDState`；`DefinitionRevision string`；`PreviousDefinitionHash string`；`PreviousDoDID common.DoDID`；`LineageState string`
- 字段引用类型：[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DoDState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L5)
- 实际方法：[CloneDoD](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/dod.go#L16)；[CloneDoDs](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/dod.go#L25)

<a id="schema-doddefineddata"></a>

### DoDDefinedData

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:378](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L378)
- 字段：`DoDID common.DoDID`；`WorkspaceID common.WorkspaceID`；`Items []string`；`State common.DoDState`；`DefinitionRevision string`；`PreviousDefinitionHash string`；`PreviousDoDID common.DoDID`；`LineageState string`
- 字段引用类型：[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DoDState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L5)

<a id="schema-dodlistresult"></a>

### DoDListResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:405](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L405)
- 字段：`DoDs []*workspacemodel.DoD`
- 字段引用类型：[DoD](#schema-dod)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L180)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID)`；`item == nil || !validWorkspaceResultID(item.WorkspaceID) || !validDoDResultID(item.ID)`；`item.WorkspaceID != command.WorkspaceID`；`_, exists := seen[item.ID]; exists`

<a id="schema-dodresult"></a>

### DoDResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:404](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L404)
- 字段：`DoD *workspacemodel.DoD`
- 字段引用类型：[DoD](#schema-dod)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L166)
- 空值/identity/版本/范围原式：`!ok || r.DoD == nil`；`!validWorkspaceResultID(workspaceID) || !validDoDResultID(doDID) || !validWorkspaceResultID(r.DoD.WorkspaceID) || !validDoDResultID(r.DoD.ID)`；`r.DoD.WorkspaceID != workspaceID || r.DoD.ID != doDID`

<a id="schema-ensureworkspacetemplatedefaultscommand"></a>

### EnsureWorkspaceTemplateDefaultsCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:281](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L281)
- 声明：`type EnsureWorkspaceTemplateDefaultsCommand struct{}`
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L248)
- 空值/identity/版本/范围原式：`!r.Ensured`

<a id="schema-ensureworkspacetemplatedefaultsresult"></a>

### EnsureWorkspaceTemplateDefaultsResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:283](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L283)
- 字段：`Ensured bool`
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L248)
- 空值/identity/版本/范围原式：`!r.Ensured`

<a id="schema-getdodcommand"></a>

### GetDoDCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:267](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L267)
- 字段：`WorkspaceID common.WorkspaceID`；`DoDID common.DoDID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)
- 实际方法：[doDResultIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L45)

<a id="schema-getgoalcommand"></a>

### GetGoalCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:258](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L258)
- 字段：`WorkspaceID common.WorkspaceID`；`GoalID common.GoalID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)
- 实际方法：[goalResultIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L29)

<a id="schema-getworkspacecommand"></a>

### GetWorkspaceCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:252](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L252)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-getworkspacetemplatecommand"></a>

### GetWorkspaceTemplateCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:276](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L276)
- 字段：`TemplateID string`

<a id="schema-goal"></a>

### Goal

- 源码：[internal/modules/kernel/workspace/pkg/models/goal.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal.go#L5)
- 字段：`ID common.GoalID`；`WorkspaceID common.WorkspaceID`；`Text string`；`DefinitionRevision string`；`PreviousDefinitionHash string`；`PreviousGoalID common.GoalID`；`LineageState string`
- 字段引用类型：[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[CloneGoal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal.go#L15)；[CloneGoals](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal.go#L23)

<a id="schema-goaldefineddata"></a>

### GoalDefinedData

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:368](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L368)
- 字段：`GoalID common.GoalID`；`WorkspaceID common.WorkspaceID`；`Text string`；`DefinitionRevision string`；`PreviousDefinitionHash string`；`PreviousGoalID common.GoalID`；`LineageState string`
- 字段引用类型：[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-goallistresult"></a>

### GoalListResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:399](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L399)
- 字段：`Goals []*workspacemodel.Goal`
- 字段引用类型：[Goal](#schema-goal)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L136)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID)`；`item == nil || !validWorkspaceResultID(item.WorkspaceID) || !validGoalResultID(item.ID)`；`item.WorkspaceID != command.WorkspaceID`；`_, exists := seen[item.ID]; exists`

<a id="schema-goalresult"></a>

### GoalResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:398](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L398)
- 字段：`Goal *workspacemodel.Goal`
- 字段引用类型：[Goal](#schema-goal)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L122)
- 空值/identity/版本/范围原式：`!ok || r.Goal == nil`；`!validWorkspaceResultID(workspaceID) || !validGoalResultID(goalID) || !validWorkspaceResultID(r.Goal.WorkspaceID) || !validGoalResultID(r.Goal.ID)`；`r.Goal.WorkspaceID != workspaceID || r.Goal.ID != goalID`

<a id="schema-goalspecification"></a>

### GoalSpecification

- 源码：[internal/modules/kernel/workspace/pkg/models/goal_specification.go:36](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L36)
- 字段：`Version uint32`；`GoalItemRef string`；`GoalTextHash string`；`Resources []GoalSpecificationResource`；`Dimensions []GoalSpecificationDimension`；`Requirements []GoalSpecificationRequirement`；`Obligations []GoalSpecificationObligation`；`Verifications []GoalSpecificationVerification`；`Reviews []GoalSpecificationCompletenessReview`
- 字段引用类型：[GoalSpecificationResource](#schema-goalspecificationresource)；[GoalSpecificationDimension](#schema-goalspecificationdimension)；[GoalSpecificationRequirement](#schema-goalspecificationrequirement)；[GoalSpecificationObligation](#schema-goalspecificationobligation)；[GoalSpecificationVerification](#schema-goalspecificationverification)；[GoalSpecificationCompletenessReview](#schema-goalspecificationcompletenessreview)
- 实际方法：[validatedSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L72)；[validatedSpecificationMode](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go#L75)；[ValidateReviewCoverage](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification_review_coverage.go#L42)；[ValidateAuthoringSpecification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification.go#L54)；[validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification.go#L62)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L191)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L104)；[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L128)；[ContentHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L204)；[AdmissionHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L237)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L251)
- 空值/identity/版本/范围原式：`record == nil || record.Input.WorkspaceID != workspaceID || record.validate(view) != nil || workspaceID == "" || c.InputHash != record.InputHash || c.TemplateSnapshotHash != record.TemplateSnapshotHash || c.ConfigurationHash == "" || (c.Phase != DefinitionAuthoringConfirmationPrepared && c.Phase != DefinitionAuthoringConfirmationMaterialized)`；`err != nil || specification.AdmissionHash() != c.SpecificationHash || c.ResourceSnapshotHash != derivation.ResourceSnapshotHash`；`err != nil || len(items) != len(c.DoD.Items) || len(items) == 0`；`c.DoD.Items[n] != item.Text`；`sequence == 0 && c.PreviousInputHash == ""`；`sequence == 0 || sequence == 1 && c.PreviousInputHash != "" || sequence > 1 && c.PreviousInputHash == "" || sequence > 1 && c.PreviousInputHash == c.InputHash`；`sequence > 1`；`previousGoal == "" || previousDoD == "" || c.Goal.PreviousDefinitionHash == "" || c.DoD.PreviousDefinitionHash == ""`；`c.Goal.PreviousGoalID != "" || c.DoD.PreviousDoDID != "" || c.Goal.PreviousDefinitionHash != "" || c.DoD.PreviousDefinitionHash != ""`；`c.Goal.WorkspaceID != workspaceID || c.DoD.WorkspaceID != workspaceID || c.Goal.ID != common.GoalID("authoring-goal-"+StableDefinitionAuthoringConfirmationID(workspaceID, c.InputHash, c.DerivationPermitID, c.SpecificationHash)) || c.DoD.ID != common.DoDID("authoring-dod-"+StableDefinitionAuthoringConfirmationID(workspaceID, c.InputHash, c.DerivationPermitID, c.SpecificationHash)) || c.Goal.Text != strings.TrimSpace(record.Input.GoalText) || c.Goal.DefinitionRevision != authoringFormalRevisionAt("goal", sequence, c.Goal.Text) || c.DoD.DefinitionRevision != authoringFormalRevisionAt("dod", sequence, strings.Join(c.DoD.Items, "\n")) || c.Goal.LineageState != "active" || c.DoD.LineageState != "active" || c.DoD.State != common.DoDStateReady`；`err != nil`；`len(missing) > 0`；`len(spec.Resources) != len(resources)`；`expected, ok := expectedByID[actual.ID]; !ok || seen[actual.ID] || actual != expected`；`len(withReviews.Reviews) == 0`；`err := withReviews.Validate(goal, items); err != nil`；`len(spec.Dimensions) == 0 || len(spec.Requirements) == 0`；`dimension.Required && !dimension.Activated`；`!requirement.Required || requirement.Text == "" || len(requirement.SourceItemRefs) == 0`；`!slices.Equal(requirement.SourceItemRefs, []string{DefinitionAuthoringGoalItemRef}) && !authoringRequirementVerified(requirement, spec.Obligations, spec.Verifications)`；`!authoringGoalRequirementPresent(goal.Text, spec.Requirements)`；`!authoringItemObligationPresent(item, spec.Requirements, spec.Obligations)`；`obligation.Text == "" || len(obligation.DoDItemRefs) == 0`；`ref == DefinitionAuthoringGoalItemRef`；`requirement.SourceKind != GoalRequirementUserExplicit`；`item.CandidateItemRef == ref && requirement.SourceKind != authoringItemKind(item)`；`c.FailureCode != ""`；`c.Proposal != nil || c.Review != nil || len(c.FailureCode) > 96`；`(r < 'a' || r > 'z') && r != '_' && (r < '0' || r > '9')`；`c.Proposal == nil || c.Review != nil || len(c.Proposal.Reviews) != 0 || c.Proposal.Version != GoalSpecificationVersionCurrent || c.Proposal.GoalItemRef != candidate.GoalInput.CandidateItemRef || c.Proposal.GoalTextHash != StableDefinitionTextHash(candidate.GoalInput.Text) || !sameSpecificationResources(c.Proposal.Resources, resources) || !sameSpecificationDimensions(c.Proposal.Dimensions, dimensions)`；`c.Proposal != nil || c.Review == nil || proposal == nil || c.Review.Kind != id.ReviewKind || c.Review.SpecificationHash != id.ProposalHash || proposal.ContentHash() != id.ProposalHash`；`kind == c.Review.Kind`；`s.Version != GoalSpecificationVersionCurrent || s.GoalItemRef != goal.CandidateItemRef || s.GoalTextHash != StableDefinitionTextHash(goal.Text)`；`len(s.Resources) > 128 || len(s.Dimensions) > 64 || len(s.Requirements) > 256 || len(s.Obligations) > 512 || len(s.Verifications) > 512 || len(s.Reviews) != len(goalReviewKinds)`；`id == "" || identities[id]`；`!claimID(r.ID) || r.Ref == "" || len(r.Ref) > 1024 || len(r.SnapshotHash) > 256 || len(r.ID) > 128`；`!claimID(d.ID) || len(d.ID) > 128 || !specRefsExist(d.ResourceIDs, func(ref string) bool { _, ok := resources[ref]; return ok })`；`r.Required && r.SourceKind == GoalRequirementUserExplicit && slices.Equal(r.SourceItemRefs, []string{goal.CandidateItemRef}) && r.Text == goal.Text`；`!claimID(r.ID) || len(r.ID) > 128 || !hasDimension || len(r.Text) > textLimit || len(r.SourceItemRefs) > 32 || len(r.ResourceIDs) > 128 || !specRefsExist(r.SourceItemRefs, func(ref string) bool { return items[ref] }) || !specRefsExist(r.ResourceIDs, func(ref string) bool { _, ok := resources[ref]; return ok })`；`!claimID(o.ID) || len(o.ID) > 128 || !hasRequirement || len(o.Text) > 4096 || len(o.DoDItemRefs) > 32 || !specRefsExist(o.DoDItemRefs, func(ref string) bool { return dodRefs[ref] })`；`!claimID(v.ID) || len(v.ID) > 128 || !hasObligation || len(v.Declaration) > 4096`；`v.ProfileID == "" || v.EvidenceRequirementID == "" || v.Scope == ""`；`v.ProfileID != "" || v.EvidenceRequirementID != "" || v.Scope != ""`；`!allowedReview[r.Kind] || reviews[r.Kind]`；`err := context.validate(r); err != nil`
- 显式分支集合（只表列源码case，default语义见validator）：`identifier") } } return nil } switch id.Phase { case DefinitionSpecificationProposal`；`DefinitionSpecificationCritic`；`GoalRequirementUserExplicit, GoalRequirementTemplateRequired, GoalRequirementTemplateConditional, GoalRequirementDerived, GoalRequirementDiscovered`；`GoalVerificationDeclared`；`GoalVerificationUnverified`

<a id="schema-goalspecificationcompletenessreview"></a>

### GoalSpecificationCompletenessReview

- 源码：[internal/modules/kernel/workspace/pkg/models/goal_specification.go:93](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L93)
- 字段：`Kind string`；`Status string`；`SpecificationHash string`；`Reason string`；`EvidenceRefs []string`；`RequirementIDs []string`
- 实际方法：[ValidateReviewCoverage](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification_review_coverage.go#L42)；[validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L363)
- 空值/identity/版本/范围原式：`err != nil`；`len(missing) > 0`；`!GoalSpecificationReviewKind(r.Kind) || r.SpecificationHash != c.contentHash || r.Status == "" || len(r.Reason) > 4096 || len(r.EvidenceRefs) > 64 || len(r.RequirementIDs) > 256 || !specRefsExist(r.EvidenceRefs, func(ref string) bool { return c.evidence[ref] }) || !specRefsExist(r.RequirementIDs, func(ref string) bool { _, ok := c.requirements[ref]; return ok })`；`r.Reason == "" || len(r.EvidenceRefs) == 0 || len(r.RequirementIDs) == 0`；`err := scope.ValidateReviewCoverage(r); err != nil`
- 显式分支集合（只表列源码case，default语义见validator）：`GoalReviewPass`；`GoalReviewFail, GoalReviewUnknown`

<a id="schema-goalspecificationdimension"></a>

### GoalSpecificationDimension

- 源码：[internal/modules/kernel/workspace/pkg/models/goal_specification.go:56](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L56)
- 字段：`ID string`；`Required bool`；`Activated bool`；`ResourceIDs []string`
- 实际方法：[DefinitionSpecificationCompilationInputHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L95)；[ValidateDefinitionSpecificationInputs](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L113)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L191)
- 空值/identity/版本/范围原式：`len(seed.Resources) > 128 || len(seed.Dimensions) > 64`；`resource.ID == "" || len(resource.ID) > 128 || seen[resource.ID] || resource.Ref == "" || len(resource.Ref) > 1024 || len(resource.SnapshotHash) > 256`；`dimension.ID == "" || len(dimension.ID) > 128 || seen[dimension.ID] || len(dimension.ResourceIDs) > 128 || !specRefsExist(dimension.ResourceIDs, func(ref string) bool { return resourceIDs[ref] })`；`c.FailureCode != ""`；`c.Proposal != nil || c.Review != nil || len(c.FailureCode) > 96`；`(r < 'a' || r > 'z') && r != '_' && (r < '0' || r > '9')`；`c.Proposal == nil || c.Review != nil || len(c.Proposal.Reviews) != 0 || c.Proposal.Version != GoalSpecificationVersionCurrent || c.Proposal.GoalItemRef != candidate.GoalInput.CandidateItemRef || c.Proposal.GoalTextHash != StableDefinitionTextHash(candidate.GoalInput.Text) || !sameSpecificationResources(c.Proposal.Resources, resources) || !sameSpecificationDimensions(c.Proposal.Dimensions, dimensions)`；`c.Proposal != nil || c.Review == nil || proposal == nil || c.Review.Kind != id.ReviewKind || c.Review.SpecificationHash != id.ProposalHash || proposal.ContentHash() != id.ProposalHash`；`kind == c.Review.Kind`
- 显式分支集合（只表列源码case，default语义见validator）：`identifier") } } return nil } switch id.Phase { case DefinitionSpecificationProposal`；`DefinitionSpecificationCritic`

<a id="schema-goalspecificationobligation"></a>

### GoalSpecificationObligation

- 源码：[internal/modules/kernel/workspace/pkg/models/goal_specification.go:73](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L73)
- 字段：`ID string`；`RequirementID string`；`DoDItemRefs []string`；`Text string`

<a id="schema-goalspecificationrequirement"></a>

### GoalSpecificationRequirement

- 源码：[internal/modules/kernel/workspace/pkg/models/goal_specification.go:63](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L63)
- 字段：`ID string`；`DimensionID string`；`Required bool`；`SourceKind string`；`SourceItemRefs []string`；`ResourceIDs []string`；`Text string`

<a id="schema-goalspecificationresource"></a>

### GoalSpecificationResource

- 源码：[internal/modules/kernel/workspace/pkg/models/goal_specification.go:48](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L48)
- 字段：`ID string`；`Ref string`；`SnapshotHash string`；`Required bool`；`Available bool`
- 实际方法：[DefinitionSpecificationCompilationInputHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L95)；[ValidateDefinitionSpecificationInputs](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L113)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_specification_ledger.go#L191)
- 空值/identity/版本/范围原式：`len(seed.Resources) > 128 || len(seed.Dimensions) > 64`；`resource.ID == "" || len(resource.ID) > 128 || seen[resource.ID] || resource.Ref == "" || len(resource.Ref) > 1024 || len(resource.SnapshotHash) > 256`；`dimension.ID == "" || len(dimension.ID) > 128 || seen[dimension.ID] || len(dimension.ResourceIDs) > 128 || !specRefsExist(dimension.ResourceIDs, func(ref string) bool { return resourceIDs[ref] })`；`c.FailureCode != ""`；`c.Proposal != nil || c.Review != nil || len(c.FailureCode) > 96`；`(r < 'a' || r > 'z') && r != '_' && (r < '0' || r > '9')`；`c.Proposal == nil || c.Review != nil || len(c.Proposal.Reviews) != 0 || c.Proposal.Version != GoalSpecificationVersionCurrent || c.Proposal.GoalItemRef != candidate.GoalInput.CandidateItemRef || c.Proposal.GoalTextHash != StableDefinitionTextHash(candidate.GoalInput.Text) || !sameSpecificationResources(c.Proposal.Resources, resources) || !sameSpecificationDimensions(c.Proposal.Dimensions, dimensions)`；`c.Proposal != nil || c.Review == nil || proposal == nil || c.Review.Kind != id.ReviewKind || c.Review.SpecificationHash != id.ProposalHash || proposal.ContentHash() != id.ProposalHash`；`kind == c.Review.Kind`
- 显式分支集合（只表列源码case，default语义见validator）：`identifier") } } return nil } switch id.Phase { case DefinitionSpecificationProposal`；`DefinitionSpecificationCritic`

<a id="schema-goalspecificationverification"></a>

### GoalSpecificationVerification

- 源码：[internal/modules/kernel/workspace/pkg/models/goal_specification.go:83](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go#L83)
- 字段：`ID string`；`ObligationID string`；`Declaration string`；`Status string`；`ProfileID string`；`EvidenceRequirementID string`；`Scope string`

<a id="schema-listdodsbyworkspacecommand"></a>

### ListDoDsByWorkspaceCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:272](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L272)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L180)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID)`；`item == nil || !validWorkspaceResultID(item.WorkspaceID) || !validDoDResultID(item.ID)`；`item.WorkspaceID != command.WorkspaceID`；`_, exists := seen[item.ID]; exists`

<a id="schema-listgoalsbyworkspacecommand"></a>

### ListGoalsByWorkspaceCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:263](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L263)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L136)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID)`；`item == nil || !validWorkspaceResultID(item.WorkspaceID) || !validGoalResultID(item.ID)`；`item.WorkspaceID != command.WorkspaceID`；`_, exists := seen[item.ID]; exists`

<a id="schema-listworkspacetemplatescommand"></a>

### ListWorkspaceTemplatesCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:280](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L280)
- 声明：`type ListWorkspaceTemplatesCommand struct{}`
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L224)
- 空值/identity/版本/范围原式：`item == nil || !validTemplateResultID(item.ID)`；`_, exists := seen[item.ID]; exists`

<a id="schema-listworkspacescommand"></a>

### ListWorkspacesCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:256](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L256)
- 声明：`type ListWorkspacesCommand struct{}`
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L75)
- 空值/identity/版本/范围原式：`item == nil || !validWorkspaceResultID(item.ID)`；`_, exists := seen[item.ID]; exists`

<a id="schema-preparedefinitionchangecommand"></a>

### PrepareDefinitionChangeCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_change.go:16](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L16)
- 字段：`WorkspaceID common.WorkspaceID`；`RequestID string`；`ExpectedWorkspaceRevision uint64`；`TicketID common.TicketID`；`ProposalRevision uint64`；`ProposalHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L25)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L57)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.TicketID == "" || strings.TrimSpace(c.RequestID) == "" || len(c.RequestID) > 256 || !utf8.ValidString(c.RequestID) || c.ExpectedWorkspaceRevision == 0 || c.ExpectedWorkspaceRevision == ^uint64(0) || c.ProposalRevision == 0 || c.ProposalHash == ""`；`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision < v.RecordedWorkspaceRevision || v.RequestID != c.RequestID || v.ExpectedWorkspaceRevision != c.ExpectedWorkspaceRevision || v.RecordedWorkspaceRevision != c.ExpectedWorkspaceRevision+1 || v.TicketID != c.TicketID || v.ProposalRevision != c.ProposalRevision || v.ProposalHash != c.ProposalHash || v.Record.Input.WorkspaceID != c.WorkspaceID || v.BaseInputHash == "" || v.Record.InputHash == v.BaseInputHash`；`err != nil || record.ValidateView() != nil`

<a id="schema-preparedefinitionchangeresult"></a>

### PrepareDefinitionChangeResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_change.go:51](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L51)
- 字段：`WorkspaceID common.WorkspaceID`；`WorkspaceRevision uint64`；`Candidate DefinitionChangeCandidate`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionChangeCandidate](#schema-definitionchangecandidate)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L57)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision < v.RecordedWorkspaceRevision || v.RequestID != c.RequestID || v.ExpectedWorkspaceRevision != c.ExpectedWorkspaceRevision || v.RecordedWorkspaceRevision != c.ExpectedWorkspaceRevision+1 || v.TicketID != c.TicketID || v.ProposalRevision != c.ProposalRevision || v.ProposalHash != c.ProposalHash || v.Record.Input.WorkspaceID != c.WorkspaceID || v.BaseInputHash == "" || v.Record.InputHash == v.BaseInputHash`；`err != nil || record.ValidateView() != nil`

<a id="schema-queryauthoringdiagnosticcommand"></a>

### QueryAuthoringDiagnosticCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring.go:14](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L14)
- 字段：`WorkspaceID common.WorkspaceID`；`PermitID string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L32)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L39)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.PermitID == "" || len(c.PermitID) > 256`；`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.PermitID != c.PermitID || r.Diagnostic.Validate() != nil`；`r.FailureCode == "provider_failed" && r.Diagnostic.ProviderCode != "" && (r.Phase == AuthoringDiagnosticDerivation || r.Phase == workspace.DefinitionSpecificationProposal || r.Phase == workspace.DefinitionSpecificationCritic)`；`(r.Phase == AuthoringDiagnosticDerivation || r.Phase == workspace.DefinitionSpecificationProposal) && r.FailureCode == "invalid_proposal" || r.Phase == workspace.DefinitionSpecificationCritic && r.FailureCode == "invalid_review"`

<a id="schema-queryauthoringdiagnosticresult"></a>

### QueryAuthoringDiagnosticResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring.go:24](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L24)
- 字段：`WorkspaceID common.WorkspaceID`；`PermitID string`；`Phase workspace.DefinitionSpecificationPhase`；`FailureCode string`；`Diagnostic workspace.DefinitionAuthoringDiagnostic`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionSpecificationPhase](#schema-definitionspecificationphase)；[DefinitionAuthoringDiagnostic](#schema-definitionauthoringdiagnostic)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L39)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.PermitID != c.PermitID || r.Diagnostic.Validate() != nil`；`r.FailureCode == "provider_failed" && r.Diagnostic.ProviderCode != "" && (r.Phase == AuthoringDiagnosticDerivation || r.Phase == workspace.DefinitionSpecificationProposal || r.Phase == workspace.DefinitionSpecificationCritic)`；`(r.Phase == AuthoringDiagnosticDerivation || r.Phase == workspace.DefinitionSpecificationProposal) && r.FailureCode == "invalid_proposal" || r.Phase == workspace.DefinitionSpecificationCritic && r.FailureCode == "invalid_review"`

<a id="schema-queryauthoringresourcescopecommand"></a>

### QueryAuthoringResourceScopeCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go#L15)
- 字段：`WorkspaceID common.WorkspaceID`；`InputHash string`；`ResourceRef string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go#L21)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go#L36)；[AuthoringResourceScopeFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go#L43)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || len(c.WorkspaceID) > 256 || c.InputHash == "" || len(c.InputHash) > 256 || c.ResourceRef == "" || len(c.ResourceRef) > 1024 || !utf8.ValidString(c.ResourceRef) || strings.ContainsRune(c.ResourceRef, 0)`；`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.InputHash != c.InputHash || r.ResourceRef != c.ResourceRef || r.DeliverableDirectory == ""`；`command.Validate() != nil || view.ValidateFor(QueryDefinitionAuthoringCommand{WorkspaceID: command.WorkspaceID}) != nil || view.Record == nil || view.Record.InputHash != command.InputHash || !view.CurrentTemplateAvailable || view.CurrentTemplateSnapshotHash != view.Record.TemplateSnapshotHash || view.DeliverableDirectory == "" || !slices.Contains(view.Record.Input.ResourceRefs, command.ResourceRef)`

<a id="schema-queryauthoringresourcescoperesult"></a>

### QueryAuthoringResourceScopeResult

- 源码：[internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go:28](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go#L28)
- 字段：`WorkspaceID common.WorkspaceID`；`WorkspaceRevision uint64`；`InputHash string`；`ResourceRef string`；`DeliverableDirectory string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go#L36)；[AuthoringResourceScopeFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go#L43)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.InputHash != c.InputHash || r.ResourceRef != c.ResourceRef || r.DeliverableDirectory == ""`；`command.Validate() != nil || view.ValidateFor(QueryDefinitionAuthoringCommand{WorkspaceID: command.WorkspaceID}) != nil || view.Record == nil || view.Record.InputHash != command.InputHash || !view.CurrentTemplateAvailable || view.CurrentTemplateSnapshotHash != view.Record.TemplateSnapshotHash || view.DeliverableDirectory == "" || !slices.Contains(view.Record.Input.ResourceRefs, command.ResourceRef)`

<a id="schema-queryauthoringsuccessauditcommand"></a>

### QueryAuthoringSuccessAuditCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_success_audit.go:10](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_success_audit.go#L10)
- 字段：`WorkspaceID common.WorkspaceID`；`PermitID string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_success_audit.go#L15)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_success_audit.go#L29)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.PermitID == "" || len(c.PermitID) > 256`；`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.PermitID != c.PermitID || r.CanonicalHash == "" || r.Audit.Validate() != nil`

<a id="schema-queryauthoringsuccessauditresult"></a>

### QueryAuthoringSuccessAuditResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_success_audit.go:22](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_success_audit.go#L22)
- 字段：`WorkspaceID common.WorkspaceID`；`PermitID string`；`CanonicalHash string`；`Audit workspace.DefinitionAuthoringSuccessByteAudit`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAuthoringSuccessByteAudit](#schema-definitionauthoringsuccessbyteaudit)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_success_audit.go#L29)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_success_audit.go#L35)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.PermitID != c.PermitID || r.CanonicalHash == "" || r.Audit.Validate() != nil`

<a id="schema-querybuiltinworkspacetemplatedefaultscommand"></a>

### QueryBuiltinWorkspaceTemplateDefaultsCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/builtin_defaults.go:22](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/builtin_defaults.go#L22)
- 声明：`type QueryBuiltinWorkspaceTemplateDefaultsCommand struct{}`

<a id="schema-querybuiltinworkspacetemplatedefaultsresult"></a>

### QueryBuiltinWorkspaceTemplateDefaultsResult

- 源码：[internal/modules/kernel/workspace/pkg/events/builtin_defaults.go:23](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/builtin_defaults.go#L23)
- 字段：`Defaults []BuiltinWorkspaceTemplateDefault`
- 字段引用类型：[BuiltinWorkspaceTemplateDefault](#schema-builtinworkspacetemplatedefault)

<a id="schema-querydefinitionauthoringcommand"></a>

### QueryDefinitionAuthoringCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring.go:81](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L81)
- 字段：`WorkspaceID common.WorkspaceID`；`InputHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L102)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.CurrentTemplateAvailable != (r.CurrentTemplateSnapshotHash != "")`；`c.InputHash != "" && (len(c.InputHash) > 256 || r.Record == nil || r.Record.InputHash != c.InputHash || r.Confirmation != nil || r.AdmissionCandidate != nil || r.ActivationAdmission != nil || len(r.History) != 0)`；`r.Record != nil && r.Record.Input.WorkspaceID != c.WorkspaceID`；`r.Record != nil`；`err != nil`；`(&workspace.Workspace{ID: c.WorkspaceID, Revision: r.WorkspaceRevision, DefinitionAuthoring: record, DefinitionAuthoringHistory: history}).ValidatePreparationDiagnosticHistoryView() != nil`；`r.Confirmation != nil`；`sequence == 0 && r.Confirmation.PreviousInputHash == ""`；`sequence != uint64(len(r.History)+1)`；`r.AdmissionCandidate != nil`；`r.Confirmation.ValidateView(record, c.WorkspaceID) != nil`；`err := record.ValidateView(); err != nil`；`len(r.History) > 0`；`r.Record == nil || r.Record.Input.Revision <= r.History[len(r.History)-1].Record.Input.Revision`；`archived.Record.Input.Revision <= previousRevision || archived.Confirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || archived.Confirmation.ValidateView(&archived.Record, c.WorkspaceID) != nil`；`index == 0 && archived.Confirmation.FormalSequence > 1 || index > 0 && archived.Confirmation.FormalSequence != uint64(index+1)`；`index > 0 && archived.Confirmation.ValidateSuccessor(r.History[index-1].Confirmation) != nil`；`r.Confirmation != nil && r.Confirmation.ValidateSuccessor(r.History[len(r.History)-1].Confirmation) != nil`；`r.PriorClarification != nil`；`r.PriorClarification.Validate() != nil || r.PriorClarification.Candidate.WorkspaceID != c.WorkspaceID || r.PriorClarification.Candidate.CandidateRevision > uint64(len(r.History))`；`r.Confirmation == nil || r.Confirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || r.AdmissionCandidate.Validate() != nil || r.AdmissionCandidate.AuthoringRef == nil || r.AdmissionCandidate.WorkspaceID != c.WorkspaceID || r.AdmissionResult == nil || r.AdmissionResult.ValidateFor(*r.AdmissionCandidate) != nil`；`r.ActivationAdmission == nil && r.AdmissionResult.Disposition != workspace.DefinitionAdmissionClarificationRequired && r.PriorClarification == nil`；`r.PriorClarification != nil && (r.PriorClarification.Candidate.CandidateID != r.AdmissionCandidate.CandidateID || r.PriorClarification.Candidate.CandidateRevision >= r.AdmissionCandidate.CandidateRevision)`；`r.AdmissionCandidate.CandidateRevision != uint64(len(r.History)+1)`；`*r.AdmissionCandidate.AuthoringRef != ref`；`r.AdmissionCandidate.GoalInput != goal || !reflect.DeepEqual(r.AdmissionCandidate.DoDInputs, workspace.DefinitionAdmissionCandidate{DoDInputs: dod}.Normalized().DoDInputs) || r.AdmissionCandidate.Specification == nil || !reflect.DeepEqual(r.AdmissionCandidate.Specification.Normalized(), spec.Normalized())`；`r.ActivationAdmission != nil && (r.AdmissionResult.Disposition != workspace.DefinitionAdmissionAdmittedAllOf || r.ActivationAdmission.CandidateHash != r.AdmissionCandidate.CandidateHash || r.ActivationAdmission.State != workspace.WorkspaceActivationEligible && r.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution || r.ActivationAdmission.State == workspace.WorkspaceActivationPendingClarificationResolution && r.PriorClarification == nil)`；`r.ActivationAdmission != nil && r.PriorClarification != nil`；`err != nil || r.ActivationAdmission.BlockingTicketID != identity.TicketID`；`r.AdmissionResult != nil || r.ActivationAdmission != nil`

<a id="schema-querydefinitionauthoringresult"></a>

### QueryDefinitionAuthoringResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring.go:87](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L87)
- 字段：`WorkspaceID common.WorkspaceID`；`WorkspaceRevision uint64`；`Record *DefinitionAuthoringRecord`；`History []DefinitionAuthoringConfirmedRevision`；`Confirmation *workspace.DefinitionAuthoringConfirmation`；`PriorClarification *DefinitionAdmissionClarificationBinding`；`AdmissionCandidate *workspace.DefinitionAdmissionCandidate`；`AdmissionResult *workspace.DefinitionAdmissionResult`；`ActivationAdmission *workspace.WorkspaceActivationAdmission`；`DeliverableDirectory string`；`CurrentTemplateSnapshotHash string`；`CurrentTemplateAvailable bool`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAuthoringRecord](#schema-definitionauthoringrecord)；[DefinitionAuthoringConfirmedRevision](#schema-definitionauthoringconfirmedrevision)；[DefinitionAuthoringConfirmation](#schema-definitionauthoringconfirmation)；[DefinitionAdmissionClarificationBinding](#schema-definitionadmissionclarificationbinding)；[DefinitionAdmissionCandidate](#schema-definitionadmissioncandidate)；[DefinitionAdmissionResult](#schema-definitionadmissionresult)；[WorkspaceActivationAdmission](#schema-workspaceactivationadmission)
- 实际方法：[AuthoringResourceScopeFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/authoring_resource_scope.go#L43)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L102)
- 空值/identity/版本/范围原式：`command.Validate() != nil || view.ValidateFor(QueryDefinitionAuthoringCommand{WorkspaceID: command.WorkspaceID}) != nil || view.Record == nil || view.Record.InputHash != command.InputHash || !view.CurrentTemplateAvailable || view.CurrentTemplateSnapshotHash != view.Record.TemplateSnapshotHash || view.DeliverableDirectory == "" || !slices.Contains(view.Record.Input.ResourceRefs, command.ResourceRef)`；`c.WorkspaceID == "" || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0 || r.CurrentTemplateAvailable != (r.CurrentTemplateSnapshotHash != "")`；`c.InputHash != "" && (len(c.InputHash) > 256 || r.Record == nil || r.Record.InputHash != c.InputHash || r.Confirmation != nil || r.AdmissionCandidate != nil || r.ActivationAdmission != nil || len(r.History) != 0)`；`r.Record != nil && r.Record.Input.WorkspaceID != c.WorkspaceID`；`r.Record != nil`；`err != nil`；`(&workspace.Workspace{ID: c.WorkspaceID, Revision: r.WorkspaceRevision, DefinitionAuthoring: record, DefinitionAuthoringHistory: history}).ValidatePreparationDiagnosticHistoryView() != nil`；`r.Confirmation != nil`；`sequence == 0 && r.Confirmation.PreviousInputHash == ""`；`sequence != uint64(len(r.History)+1)`；`r.AdmissionCandidate != nil`；`r.Confirmation.ValidateView(record, c.WorkspaceID) != nil`；`err := record.ValidateView(); err != nil`；`len(r.History) > 0`；`r.Record == nil || r.Record.Input.Revision <= r.History[len(r.History)-1].Record.Input.Revision`；`archived.Record.Input.Revision <= previousRevision || archived.Confirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || archived.Confirmation.ValidateView(&archived.Record, c.WorkspaceID) != nil`；`index == 0 && archived.Confirmation.FormalSequence > 1 || index > 0 && archived.Confirmation.FormalSequence != uint64(index+1)`；`index > 0 && archived.Confirmation.ValidateSuccessor(r.History[index-1].Confirmation) != nil`；`r.Confirmation != nil && r.Confirmation.ValidateSuccessor(r.History[len(r.History)-1].Confirmation) != nil`；`r.PriorClarification != nil`；`r.PriorClarification.Validate() != nil || r.PriorClarification.Candidate.WorkspaceID != c.WorkspaceID || r.PriorClarification.Candidate.CandidateRevision > uint64(len(r.History))`；`r.Confirmation == nil || r.Confirmation.Phase != workspace.DefinitionAuthoringConfirmationMaterialized || r.AdmissionCandidate.Validate() != nil || r.AdmissionCandidate.AuthoringRef == nil || r.AdmissionCandidate.WorkspaceID != c.WorkspaceID || r.AdmissionResult == nil || r.AdmissionResult.ValidateFor(*r.AdmissionCandidate) != nil`；`r.ActivationAdmission == nil && r.AdmissionResult.Disposition != workspace.DefinitionAdmissionClarificationRequired && r.PriorClarification == nil`；`r.PriorClarification != nil && (r.PriorClarification.Candidate.CandidateID != r.AdmissionCandidate.CandidateID || r.PriorClarification.Candidate.CandidateRevision >= r.AdmissionCandidate.CandidateRevision)`；`r.AdmissionCandidate.CandidateRevision != uint64(len(r.History)+1)`；`*r.AdmissionCandidate.AuthoringRef != ref`；`r.AdmissionCandidate.GoalInput != goal || !reflect.DeepEqual(r.AdmissionCandidate.DoDInputs, workspace.DefinitionAdmissionCandidate{DoDInputs: dod}.Normalized().DoDInputs) || r.AdmissionCandidate.Specification == nil || !reflect.DeepEqual(r.AdmissionCandidate.Specification.Normalized(), spec.Normalized())`；`r.ActivationAdmission != nil && (r.AdmissionResult.Disposition != workspace.DefinitionAdmissionAdmittedAllOf || r.ActivationAdmission.CandidateHash != r.AdmissionCandidate.CandidateHash || r.ActivationAdmission.State != workspace.WorkspaceActivationEligible && r.ActivationAdmission.State != workspace.WorkspaceActivationPendingClarificationResolution || r.ActivationAdmission.State == workspace.WorkspaceActivationPendingClarificationResolution && r.PriorClarification == nil)`；`r.ActivationAdmission != nil && r.PriorClarification != nil`；`err != nil || r.ActivationAdmission.BlockingTicketID != identity.TicketID`；`r.AdmissionResult != nil || r.ActivationAdmission != nil`

<a id="schema-querydefinitionchangecommand"></a>

### QueryDefinitionChangeCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_change.go:72](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L72)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L83)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0`；`r.Candidate == nil`；`r.Reviewed != nil`；`err != nil`；`err != nil || identity != *r.Reviewed`

<a id="schema-querydefinitionchangeresult"></a>

### QueryDefinitionChangeResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_change.go:76](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L76)
- 字段：`WorkspaceID common.WorkspaceID`；`WorkspaceRevision uint64`；`Candidate *DefinitionChangeCandidate`；`Reviewed *workspace.DefinitionChangeReviewedIdentity`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionChangeCandidate](#schema-definitionchangecandidate)；[DefinitionChangeReviewedIdentity](#schema-definitionchangereviewedidentity)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_change.go#L83)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || r.WorkspaceID != c.WorkspaceID || r.WorkspaceRevision == 0`；`r.Candidate == nil`；`r.Reviewed != nil`；`err != nil`；`err != nil || identity != *r.Reviewed`

<a id="schema-querypreparationdiagnosticcommand"></a>

### QueryPreparationDiagnosticCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go:93](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L93)
- 字段：`WorkspaceID common.WorkspaceID`；`DiagnosticID string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L98)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L114)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.DiagnosticID == "" || len(c.DiagnosticID) > 256`；`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.Diagnostic.WorkspaceID != c.WorkspaceID || r.Diagnostic.DiagnosticID != c.DiagnosticID || r.Diagnostic.Validate() != nil`

<a id="schema-querypreparationdiagnosticresult"></a>

### QueryPreparationDiagnosticResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go:105](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L105)
- 字段：`WorkspaceID common.WorkspaceID`；`Diagnostic workspace.DefinitionAuthoringPreparationDiagnostic`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAuthoringPreparationDiagnostic](#schema-definitionauthoringpreparationdiagnostic)
- 实际方法：[DeepClone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L110)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L114)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.WorkspaceID != c.WorkspaceID || r.Diagnostic.WorkspaceID != c.WorkspaceID || r.Diagnostic.DiagnosticID != c.DiagnosticID || r.Diagnostic.Validate() != nil`

<a id="schema-recordpreparationdiagnosticcommand"></a>

### RecordPreparationDiagnosticCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go:55](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L55)
- 字段：`Diagnostic workspace.DefinitionAuthoringPreparationDiagnostic`
- 字段引用类型：[DefinitionAuthoringPreparationDiagnostic](#schema-definitionauthoringpreparationdiagnostic)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L59)；[DeepClone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L60)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L76)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.Diagnostic.Validate() != nil || r.WorkspaceID != c.Diagnostic.WorkspaceID || r.Diagnostic.WorkspaceID != r.WorkspaceID || r.WorkspaceRevision == 0 || r.Diagnostic.ObservedWorkspaceRevision > r.WorkspaceRevision`；`r.Duplicate`；`c.Diagnostic.ObservedWorkspaceRevision == ^uint64(0) || r.WorkspaceRevision != c.Diagnostic.ObservedWorkspaceRevision+1`；`!reflect.DeepEqual(actual, expected)`

<a id="schema-recordpreparationdiagnosticresult"></a>

### RecordPreparationDiagnosticResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go:65](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L65)
- 字段：`WorkspaceID common.WorkspaceID`；`WorkspaceRevision uint64`；`Diagnostic DefinitionAuthoringPreparationDiagnosticMetadata`；`Duplicate bool`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAuthoringPreparationDiagnosticMetadata](#schema-definitionauthoringpreparationdiagnosticmetadata)
- 实际方法：[DeepClone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L72)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_preparation_diagnostic.go#L76)
- 空值/identity/版本/范围原式：`c.Validate() != nil || r.Diagnostic.Validate() != nil || r.WorkspaceID != c.Diagnostic.WorkspaceID || r.Diagnostic.WorkspaceID != r.WorkspaceID || r.WorkspaceRevision == 0 || r.Diagnostic.ObservedWorkspaceRevision > r.WorkspaceRevision`；`r.Duplicate`；`c.Diagnostic.ObservedWorkspaceRevision == ^uint64(0) || r.WorkspaceRevision != c.Diagnostic.ObservedWorkspaceRevision+1`；`!reflect.DeepEqual(actual, expected)`

<a id="schema-reserveauthoringspecificationcommand"></a>

### ReserveAuthoringSpecificationCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go:14](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L14)
- 字段：`WorkspaceID common.WorkspaceID`；`InputHash string`；`RequestID string`；`DerivationPermitID string`；`ReviewKind string`；`ProposalHash string`；`ProviderBindingHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L32)；[ValidateReserve](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_specification.go#L104)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.InputHash == "" || len(c.InputHash) > 256`；`err := c.Validate(); err != nil`；`err := r.validate(); err != nil`；`r.Request.RequestID != c.RequestID || r.Request.PermitID != c.Request().ExpectedPermitID()`

<a id="schema-reservedefinitionderivationcommand"></a>

### ReserveDefinitionDerivationCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_derivation.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L13)
- 字段：`WorkspaceID common.WorkspaceID`；`WorkspaceRevision uint64`；`InputHash string`；`RequestID string`；`Resources []workspace.DefinitionAuthoringResourceSnapshot`；`ResourceSnapshotHash string`；`ProviderBindingHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DefinitionAuthoringResourceSnapshot](#schema-definitionauthoringresourcesnapshot)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L23)；[ValidateReserve](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_derivation.go#L72)
- 空值/identity/版本/范围原式：`c.WorkspaceID == "" || c.InputHash == "" || len(c.InputHash) > 256 || c.RequestID == "" || len(c.RequestID) > 128 || c.ProviderBindingHash == "" || len(c.ProviderBindingHash) > 256 || len(c.Resources) > workspace.DefinitionAuthoringResourceLimit || c.ResourceSnapshotHash != workspace.DefinitionAuthoringResourcesHash(c.Resources) || (len(c.Resources) > 0 && c.WorkspaceRevision == 0)`；`err := c.Validate(); err != nil`；`err := r.validate(); err != nil`；`r.Request.Input.WorkspaceID != c.WorkspaceID || r.Request.Input.InputHash() != c.InputHash || r.Request.RequestID != c.RequestID || r.Request.ProviderBindingHash != c.ProviderBindingHash || r.Request.ResourceSnapshotHash != c.ResourceSnapshotHash || !reflect.DeepEqual(r.Request.Resources, c.Resources)`

<a id="schema-reservedefinitionspecificationrequestcommand"></a>

### ReserveDefinitionSpecificationRequestCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_specification.go:63](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L63)
- 字段：`Identity workspacemodel.DefinitionSpecificationRequestIdentity`
- 字段引用类型：[DefinitionSpecificationRequestIdentity](#schema-definitionspecificationrequestidentity)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_specification.go#L67)

<a id="schema-runtimeterminaldeliveryfact"></a>

### RuntimeTerminalDeliveryFact

- 源码：[internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go:74](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L74)
- 字段：`<embedded> runtimecontract.RuntimeTerminalDeliveryFact`
- 字段引用类型：[RuntimeTerminalDeliveryFact（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go#L74)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L78)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L83)；[Identity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L94)
- 空值/identity/版本/范围原式：`c.SourceOwner != "kernel/orchestrator"`；`err := c.RuntimeTerminalDeliveryFact.Validate(); err != nil`

<a id="schema-savedefinitionauthoringcommand"></a>

### SaveDefinitionAuthoringCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring.go:52](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L52)
- 字段：`RequestID string`；`ExpectedInputHash string`；`Input workspace.DefinitionAuthoringInput`；`ExpectedWorkspaceRevision uint64`；`DeliverableDirectory *string`
- 字段引用类型：[DefinitionAuthoringInput](#schema-definitionauthoringinput)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L60)
- 空值/identity/版本/范围原式：`err := c.Input.Validate(); err != nil`；`c.DeliverableDirectory != nil && (strings.TrimSpace(*c.DeliverableDirectory) == "" || len(*c.DeliverableDirectory) > 32*1024 || strings.ContainsRune(*c.DeliverableDirectory, 0))`；`strings.TrimSpace(c.RequestID) != c.RequestID || c.RequestID == "" || len(c.RequestID) > 256 || strings.ContainsRune(c.RequestID, 0)`；`c.ExpectedWorkspaceRevision == 0`

<a id="schema-savedefinitionauthoringresult"></a>

### SaveDefinitionAuthoringResult

- 源码：[internal/modules/kernel/workspace/pkg/events/definition_authoring.go:76](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go#L76)
- 字段：`Receipt workspace.DefinitionAuthoringSaveReceipt`；`Record DefinitionAuthoringRecord`
- 字段引用类型：[DefinitionAuthoringSaveReceipt](#schema-definitionauthoringsavereceipt)；[DefinitionAuthoringRecord](#schema-definitionauthoringrecord)

<a id="schema-templatesource"></a>

### TemplateSource

- 源码：[internal/modules/kernel/workspace/pkg/models/template_inheritance.go:23](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/template_inheritance.go#L23)
- 字段：`TemplateID string`；`ContentHash string`
- 实际方法：[inheritedTemplateSourceHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/template_inheritance.go#L225)

<a id="schema-updatedodcommand"></a>

### UpdateDoDCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:227](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L227)
- 字段：`WorkspaceID common.WorkspaceID`；`DoDID common.DoDID`；`Items []string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)
- 实际方法：[doDResultIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L49)

<a id="schema-updategoalcommand"></a>

### UpdateGoalCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:213](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L213)
- 字段：`WorkspaceID common.WorkspaceID`；`GoalID common.GoalID`；`Text string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)
- 实际方法：[goalResultIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L33)

<a id="schema-updateworkspacecommand"></a>

### UpdateWorkspaceCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:83](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L83)
- 字段：`WorkspaceID common.WorkspaceID`；`Title string`；`Description string`；`DeliverableDirectory string`；`InternalDirectory string`；`TemplateID string`；`TemplateBindingHash string`；`ProfileID string`；`DefaultLLMSelection *runtimecontract.ProviderInferenceSelection`；`DefaultSoulID common.SoulID`；`SoulBindings []soulbinding.Binding`；`ContextHints workorcheventpayload.Payload`；`EvidencePolicies []workorcheventpayload.Payload`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)；[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)；[Binding（shared/soulbinding声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/soulbinding/binding.go#L13)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-updateworkspacellmselectioncommand"></a>

### UpdateWorkspaceLLMSelectionCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:98](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L98)
- 字段：`WorkspaceID common.WorkspaceID`；`Selection *runtimecontract.ProviderInferenceSelection`；`ExpectedRevision uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L89)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID) || command.ExpectedRevision == 0 || r.Workspace == nil || !validWorkspaceResultID(r.Workspace.ID)`；`r.Workspace.ID != command.WorkspaceID`；`r.Changed`；`expectedRevision == ^uint64(0)`；`r.Workspace.Revision != expectedRevision`；`command.Selection == nil`；`r.Workspace.DefaultLLMSelection != nil`；`err := expectedSelection.Validate(); err != nil`；`r.Workspace.DefaultLLMSelection == nil || !r.Workspace.DefaultLLMSelection.Normalized().Same(expectedSelection)`

<a id="schema-updateworkspacetemplatecommand"></a>

### UpdateWorkspaceTemplateCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:249](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L249)
- 实际alias/同形类型：[CreateWorkspaceTemplateCommand](#schema-createworkspacetemplatecommand)
- 字段引用类型：[CreateWorkspaceTemplateCommand](#schema-createworkspacetemplatecommand)

<a id="schema-upgradebuiltinworkspacetemplatedefaultcommand"></a>

### UpgradeBuiltinWorkspaceTemplateDefaultCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/builtin_defaults.go:27](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/builtin_defaults.go#L27)
- 字段：`TemplateID string`；`ExpectedContentHash string`；`TargetContentHash string`

<a id="schema-upgradebuiltinworkspacetemplatedefaultresult"></a>

### UpgradeBuiltinWorkspaceTemplateDefaultResult

- 源码：[internal/modules/kernel/workspace/pkg/events/builtin_defaults.go:32](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/builtin_defaults.go#L32)
- 字段：`Default BuiltinWorkspaceTemplateDefault`；`Upgraded bool`
- 字段引用类型：[BuiltinWorkspaceTemplateDefault](#schema-builtinworkspacetemplatedefault)

<a id="schema-workspace"></a>

### Workspace

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:35](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L35)
- 字段：`Revision uint64`；`ID common.WorkspaceID`；`Title string`；`Description string`；`DeliverableDirectory string`；`InternalDirectory string`；`State common.WorkspaceState`；`ActiveRunID common.RunID`；`TemplateID string`；`TemplateBindingHash string`；`ProfileID string`；`DefaultLLMSelection *runtimecontract.ProviderInferenceSelection`；`DefaultSoulID common.SoulID`；`SoulBindings []soulbinding.Binding`；`ContextHints workorcheventpayload.Payload`；`EvidencePolicies []workorcheventpayload.Payload`；`DefinitionAdmissionCandidates []workspacemodel.DefinitionAdmissionCandidate`；`StartupInput *workspacemodel.WorkspaceStartupInputContract`；`ActivationAdmission *workspacemodel.WorkspaceActivationAdmission`；`TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity`；`AuthoringSummary *AuthoringSummary`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[WorkspaceState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)；[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)；[Binding（shared/soulbinding声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/soulbinding/binding.go#L13)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[DefinitionAdmissionCandidate](#schema-definitionadmissioncandidate)；[WorkspaceStartupInputContract](#schema-workspacestartupinputcontract)；[WorkspaceActivationAdmission](#schema-workspaceactivationadmission)；[RuntimeTerminalDeliveryIdentity（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go#L35)；[AuthoringSummary](#schema-authoringsummary)

<a id="schema-workspaceactivationadmission"></a>

### WorkspaceActivationAdmission

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:154](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L154)
- 字段：`WorkspaceID common.WorkspaceID`；`CandidateID string`；`CandidateRevision uint64`；`CandidateHash string`；`DefinitionRevision string`；`StartupInputHash string`；`State WorkspaceActivationAdmissionState`；`BlockingTicketID common.TicketID`；`ClarificationResolutionID common.ResolutionID`；`ClarificationResolutionBindingHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[WorkspaceActivationAdmissionState](#schema-workspaceactivationadmissionstate)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[ResolutionID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L9)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L557)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L567)；[ValidateCandidate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L590)
- 空值/identity/版本/范围原式：`a.WorkspaceID == "" || a.CandidateID == "" || a.CandidateRevision == 0 || a.CandidateHash == "" || a.DefinitionRevision == "" || a.StartupInputHash == "" || a.WorkspaceID != startup.WorkspaceID || a.DefinitionRevision != startup.DefinitionRevision || a.StartupInputHash != startup.StartupInputHash`；`a.BlockingTicketID != "" && (a.ClarificationResolutionID == "" || a.ClarificationResolutionBindingHash == "")`；`a.BlockingTicketID == "" || a.ClarificationResolutionID != "" || a.ClarificationResolutionBindingHash != ""`；`err := a.Validate(startup); err != nil`；`err := startup.ValidateCandidate(candidate); err != nil`；`a.WorkspaceID != candidate.WorkspaceID || a.CandidateID != candidate.CandidateID || a.CandidateRevision != candidate.CandidateRevision || a.CandidateHash != candidate.CandidateHash`
- 显式分支集合（只表列源码case，default语义见validator）：`WorkspaceActivationEligible`；`WorkspaceActivationPendingClarificationResolution`

<a id="schema-workspaceactivationadmissionstate"></a>

### WorkspaceActivationAdmissionState

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:147](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L147)
- 声明：`type WorkspaceActivationAdmissionState string`
- 源码枚举值：`WorkspaceActivationEligible = "eligible"`；`WorkspaceActivationPendingClarificationResolution = "pending_clarification_resolution"`

<a id="schema-workspacechangeddata"></a>

### WorkspaceChangedData

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:356](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L356)
- 字段：`WorkspaceID common.WorkspaceID`；`Title string`；`Description string`；`DeliverableDirectory string`；`InternalDirectory string`；`State common.WorkspaceState`；`ActiveRunID common.RunID`；`RunID common.RunID`；`DefaultLLMSelection *runtimecontract.ProviderInferenceSelection`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[WorkspaceState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)

<a id="schema-workspacedefinitionclarificationresolutionfact"></a>

### WorkspaceDefinitionClarificationResolutionFact

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:338](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L338)
- 字段：`ResolutionIdentity definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity`；`ResolutionBinding definitioncontract.WorkspaceDefinitionClarificationResolutionBinding`
- 字段引用类型：[WorkspaceDefinitionClarificationAdmissionResolutionIdentity（shared/workorchdefinitioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdefinitioncontract/clarification.go#L60)；[WorkspaceDefinitionClarificationResolutionBinding（shared/workorchdefinitioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdefinitioncontract/clarification.go#L91)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L140)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L146)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L154)；[ValidateForEligible](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L186)
- 空值/identity/版本/范围原式：`err := f.ResolutionBinding.ValidateFor(f.ResolutionIdentity); err != nil`；`err := f.Validate(); err != nil`；`err := prior.Validate(); err != nil`；`err := admissionResult.ValidateFor(candidate); err != nil`；`f.ResolutionIdentity != expected`；`command.TicketID != admission.BlockingTicketID || command.WorkspaceID != candidate.WorkspaceID || command.NewCandidateID != candidate.CandidateID || command.NewCandidateRevision != candidate.CandidateRevision || command.NewCandidateHash != candidate.CandidateHash || command.AdmissionResultHash != admissionResult.AdmissionResultHash || command.DefinitionRevision != startup.DefinitionRevision || command.StartupInputHash != startup.StartupInputHash || result.ResolutionID != admission.ClarificationResolutionID || result.ResolutionBindingHash != admission.ClarificationResolutionBindingHash`

<a id="schema-workspacedefinitionclarificationticketfact"></a>

### WorkspaceDefinitionClarificationTicketFact

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:330](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L330)
- 字段：`RequestIdentity definitioncontract.WorkspaceDefinitionClarificationRequestIdentity`；`TicketBinding definitioncontract.WorkspaceDefinitionClarificationTicketBinding`
- 字段引用类型：[WorkspaceDefinitionClarificationRequestIdentity（shared/workorchdefinitioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdefinitioncontract/clarification.go#L36)；[WorkspaceDefinitionClarificationTicketBinding（shared/workorchdefinitioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdefinitioncontract/clarification.go#L51)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L107)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L113)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_admission.go#L121)
- 空值/identity/版本/范围原式：`err := f.TicketBinding.ValidateFor(f.RequestIdentity); err != nil`；`err := f.Validate(); err != nil`；`err := binding.Validate(); err != nil`；`err != nil`；`!f.RequestIdentity.EqualNormalized(expected)`

<a id="schema-workspacedeleteddata"></a>

### WorkspaceDeletedData

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:163](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L163)
- 字段：`WorkspaceID common.WorkspaceID`；`DeletionID string`；`RequestID string`；`Mode deletioncontract.Mode`；`Disposition DeleteWorkspaceDisposition`；`DeletionRevision uint64`；`WorkspaceRevision uint64`；`CompletedAt time.Time`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[Mode（shared/workorchdeletioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchdeletioncontract/contract.go#L12)；[DeleteWorkspaceDisposition](#schema-deleteworkspacedisposition)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L174)
- 空值/identity/版本/范围原式：`d.WorkspaceID == "" || d.RequestID == "" || d.DeletionRevision == 0 || d.WorkspaceRevision == 0 || d.CompletedAt.IsZero()`；`d.DeletionID != StableWorkspaceDeletionID(d.WorkspaceID, d.RequestID) || d.Disposition != DeleteWorkspaceCompleted`
- 显式分支集合（只表列源码case，default语义见validator）：`deletioncontract.ModeRetain, deletioncontract.ModePurge`

<a id="schema-workspaceexecutionintentcommand"></a>

### WorkspaceExecutionIntentCommand

- 源码：[internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go:16](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L16)
- 字段：`WorkspaceID common.WorkspaceID`；`RequestID string`；`Kind runtimecontract.RuntimeExecutionIntentKind`；`Bootstrap *runtimecontract.RuntimeBootstrap`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RuntimeExecutionIntentKind（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_execution_intent.go#L12)；[RuntimeBootstrap（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_bootstrap.go#L12)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L23)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L33)
- 空值/identity/版本/范围原式：`c.Bootstrap != nil`；`err := runtimecontract.ValidateRuntimeBootstrapRaw(*c.Bootstrap); err != nil`；`c.WorkspaceID == "" || c.RequestID == ""`；`c.Bootstrap == nil`；`err := c.Bootstrap.Validate(); err != nil`
- 显式分支集合（只表列源码case，default语义见validator）：`runtimecontract.RuntimeExecutionIntentResume`；`runtimecontract.RuntimeExecutionIntentRestart`

<a id="schema-workspaceexecutionintentresult"></a>

### WorkspaceExecutionIntentResult

- 源码：[internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go:63](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L63)
- 声明：`type WorkspaceExecutionIntentResult runtimecontract.RuntimeExecutionIntentAdmission`
- 字段引用类型：[RuntimeExecutionIntentAdmission（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_execution_intent.go#L60)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L65)
- 空值/identity/版本/范围原式：`err := (runtimecontract.RuntimeExecutionIntentAdmission(r)).Validate(); err != nil`

<a id="schema-workspacellmselectionresult"></a>

### WorkspaceLLMSelectionResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:394](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L394)
- 字段：`Workspace *Workspace`；`Changed bool`
- 字段引用类型：[Workspace](#schema-workspace)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L89)
- 空值/identity/版本/范围原式：`!validWorkspaceResultID(command.WorkspaceID) || command.ExpectedRevision == 0 || r.Workspace == nil || !validWorkspaceResultID(r.Workspace.ID)`；`r.Workspace.ID != command.WorkspaceID`；`r.Changed`；`expectedRevision == ^uint64(0)`；`r.Workspace.Revision != expectedRevision`；`command.Selection == nil`；`r.Workspace.DefaultLLMSelection != nil`；`err := expectedSelection.Validate(); err != nil`；`r.Workspace.DefaultLLMSelection == nil || !r.Workspace.DefaultLLMSelection.Normalized().Same(expectedSelection)`

<a id="schema-workspacelifecyclecommandresult"></a>

### WorkspaceLifecycleCommandResult

- 源码：[internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go:100](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/lifecycle_messages.go#L100)
- 字段：`Workspace *Workspace`；`Transitioned bool`；`DeliveryAck *runtimecontract.RuntimeDeliveryAcknowledgement`
- 字段引用类型：[Workspace](#schema-workspace)；[RuntimeDeliveryAcknowledgement（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lifecycle_delivery.go#L160)

<a id="schema-workspacelistresult"></a>

### WorkspaceListResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:393](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L393)
- 字段：`Workspaces []*Workspace`
- 字段引用类型：[Workspace](#schema-workspace)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L75)
- 空值/identity/版本/范围原式：`item == nil || !validWorkspaceResultID(item.ID)`；`_, exists := seen[item.ID]; exists`

<a id="schema-workspaceresult"></a>

### WorkspaceResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:392](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L392)
- 字段：`Workspace *Workspace`
- 字段引用类型：[Workspace](#schema-workspace)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L61)
- 空值/identity/版本/范围原式：`!ok || r.Workspace == nil`；`!validWorkspaceResultID(expected) || !validWorkspaceResultID(r.Workspace.ID)`；`r.Workspace.ID != expected`

<a id="schema-workspacestartupinputcontract"></a>

### WorkspaceStartupInputContract

- 源码：[internal/modules/kernel/workspace/pkg/models/definition_admission.go:127](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L127)
- 字段：`WorkspaceID common.WorkspaceID`；`DefinitionRevision string`；`GoalDefinitionRef string`；`GoalTextHash string`；`DoDDefinitionRefs []string`；`DoDItemSetHash string`；`WorkspaceTemplateRef string`；`TemplateRevision string`；`TemplateSnapshotHash string`；`RuntimeProfileContractsHash string`；`SpecificationHash string`；`ResolvedProjectWorkspaceDirectory string`；`CandidateItemToItemRefMappings []CandidateItemToItemRefMapping`；`CandidateItemToItemRefMappingHash string`；`StartupInputHash string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[CandidateItemToItemRefMapping](#schema-candidateitemtoitemrefmapping)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L423)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L441)；[ValidateCandidate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L459)；[WithHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L524)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L531)；[computeHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L538)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L567)；[ValidateCandidate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go#L590)
- 空值/identity/版本/范围原式：`s.WorkspaceID == "" || s.DefinitionRevision == "" || s.GoalDefinitionRef == "" || s.GoalTextHash == "" || len(s.DoDDefinitionRefs) == 0 || s.DoDItemSetHash == "" || s.WorkspaceTemplateRef == "" || s.TemplateRevision == "" || s.TemplateSnapshotHash == "" || s.RuntimeProfileContractsHash == "" || s.ResolvedProjectWorkspaceDirectory == "" || len(s.CandidateItemToItemRefMappings) == 0`；`s.CandidateItemToItemRefMappingHash != stableItemMappingHash(s.CandidateItemToItemRefMappings)`；`s.StartupInputHash == "" || s.StartupInputHash != s.computeHash()`；`err := s.Validate(); err != nil`；`err := candidate.Validate(); err != nil`；`s.WorkspaceID != candidate.WorkspaceID || s.GoalTextHash != StableDefinitionTextHash(candidate.GoalInput.Text) || s.WorkspaceTemplateRef != candidate.WorkspaceTemplateSnapshot.TemplateRef || s.TemplateRevision != candidate.WorkspaceTemplateSnapshot.Revision || s.TemplateSnapshotHash != candidate.WorkspaceTemplateSnapshot.SnapshotHash || s.RuntimeProfileContractsHash != candidate.RuntimeProfileContractsHash || s.SpecificationHash != candidateSpecificationAdmissionHash(candidate) || s.ResolvedProjectWorkspaceDirectory != candidate.ResolvedProjectWorkspaceDirectory`；`dodSourceIdentity == ""`；`dodSourceIdentity != item.SourceIdentity`；`s.DefinitionRevision != candidate.GoalInput.SourceIdentity+"|"+dodSourceIdentity || s.DoDItemSetHash != StableDefinitionStringSetHash(dodTexts)`；`mapping.CandidateItemRef == "" || mapping.ItemRef == ""`；`_, duplicate := mappings[mapping.CandidateItemRef]; duplicate`；`_, duplicate := itemRefs[mapping.ItemRef]; duplicate`；`len(mappings) != 1+len(candidate.DoDInputs) || mappings[candidate.GoalInput.CandidateItemRef] != s.GoalDefinitionRef`；`ref == ""`；`!equalDefinitionStrings(dodRefs, s.DoDDefinitionRefs)`；`s.SpecificationHash != ""`；`a.WorkspaceID == "" || a.CandidateID == "" || a.CandidateRevision == 0 || a.CandidateHash == "" || a.DefinitionRevision == "" || a.StartupInputHash == "" || a.WorkspaceID != startup.WorkspaceID || a.DefinitionRevision != startup.DefinitionRevision || a.StartupInputHash != startup.StartupInputHash`；`a.BlockingTicketID != "" && (a.ClarificationResolutionID == "" || a.ClarificationResolutionBindingHash == "")`；`a.BlockingTicketID == "" || a.ClarificationResolutionID != "" || a.ClarificationResolutionBindingHash != ""`；`err := a.Validate(startup); err != nil`；`err := startup.ValidateCandidate(candidate); err != nil`；`a.WorkspaceID != candidate.WorkspaceID || a.CandidateID != candidate.CandidateID || a.CandidateRevision != candidate.CandidateRevision || a.CandidateHash != candidate.CandidateHash`
- 显式分支集合（只表列源码case，default语义见validator）：`WorkspaceActivationEligible`；`WorkspaceActivationPendingClarificationResolution`

<a id="schema-workspacetemplate"></a>

### WorkspaceTemplate

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:69](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L69)
- 字段：`ID string`；`ParentTemplateIDs []string`；`EffectiveSources []workspacemodel.TemplateSource`；`Name string`；`Description string`；`GoalText string`；`DoDItems []string`；`DefaultSoulID string`；`SoulBindings []soulbinding.Binding`；`ContextHints workorcheventpayload.Payload`；`EvidencePolicies []workorcheventpayload.Payload`
- 字段引用类型：[TemplateSource](#schema-templatesource)；[Binding（shared/soulbinding声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/soulbinding/binding.go#L13)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)
- 实际方法：[cloneAuthoringTemplate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring_dto.go#L40)

<a id="schema-workspacetemplatelistresult"></a>

### WorkspaceTemplateListResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:411](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L411)
- 字段：`Templates []*WorkspaceTemplate`
- 字段引用类型：[WorkspaceTemplate](#schema-workspacetemplate)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L224)
- 空值/identity/版本/范围原式：`item == nil || !validTemplateResultID(item.ID)`；`_, exists := seen[item.ID]; exists`

<a id="schema-workspacetemplateresult"></a>

### WorkspaceTemplateResult

- 源码：[internal/modules/kernel/workspace/pkg/events/messages.go:410](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/messages.go#L410)
- 字段：`Template *WorkspaceTemplate`
- 字段引用类型：[WorkspaceTemplate](#schema-workspacetemplate)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/management_result_validation.go#L210)
- 空值/identity/版本/范围原式：`!ok || r.Template == nil`；`!validTemplateResultID(expected) || !validTemplateResultID(r.Template.ID)`；`r.Template.ID != expected`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [handleDefinitionSpecificationCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_specification.go#L22)：`ev == nil || m == nil || m.workspaces == nil || (ev.Source() != definitionSpecificationCommandSource && !(ev.ID() == workspaceevents.TopicWorkspaceDefinitionChangeQuery && ev.Source() == "blocks/decision") && ((ev.ID() != workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery || ev.Source() != "blocks/file") && ev.ID() != workspaceevents.TopicWorkspaceAuthoringDiagnosticQuery))`
  - 当前来源常量：`definitionSpecificationCommandSource = "application/workorchd"`；`workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery = "/workspace/definition-authoring/resource-scope/query"`
- [deleteWorkspace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/workspace_commands.go#L215)：`ev == nil || ev.Source() != workspaceDeletionIngressSource`
  - 当前来源常量：`workspaceDeletionIngressSource = "application/workorchd"`
- [prepareWorkspaceExecutionIntent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/execution_intent.go#L87)：`!ok || ev.Source() != workspaceExecutionIngressSource`
  - 当前来源常量：`workspaceExecutionIngressSource = "application/workorchd"`
- [handleDefinitionAdmissionCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_admission.go#L24)：`m == nil || m.workspaces == nil || ev == nil || ev.Source() != definitionAdmissionCommandSource`
  - 当前来源常量：`definitionAdmissionCommandSource = "application/workorchd"`
- [executeLifecycleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/lifecycle_handler.go#L38)：`!ok || ev.Source() != "kernel/orchestrator"`
- [handleSystemDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L146)：`ev == nil || ev.Source() != workspaceTemplateDefaultsCommandSource || m == nil || m.templates == nil`
  - 当前来源常量：`workspaceTemplateDefaultsCommandSource = "kernel/system"`
- [handleBuiltinTemplateDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/builtin_defaults.go#L19)：`m == nil || m.templates == nil || ev == nil || ev.Source() != "application/workorchd/appservice"`