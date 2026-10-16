# llm：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页12个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

LLM只提供配置/catalog、provider调用、完整/失败/不完整文档与usage事实，不拥有planning/semantic/terminal权威。CompleteProviderDocument精确绑定RequestIdentity、Target、RequestHash、ExpectedConfigurationRevision；RequestIdentity长度≤256、非空、规范空白且无NUL，Request.Model等于Target.ModelID。

输出Document/Failure/Incomplete必须恰一臂，RequestIdentity/Hash/Target/config revision和submitted/usage相符。incomplete.output_limit、stream_ended_before_completion等是失败事实；不得补猜JSON或将空usage作0。Runtime调用顺序是preflight→fresh current intent→session CAS→physical provider；authoring同样先建立持久request身份。submitted-but-outcome-unknown不能换ID补发；必须以原request对账并保留unknown消费。配置查询不制造execution budget，真实provider调用服从封存预算。

## 逐topic合同

<a id="ct-llm-acquireproviderceiling"></a>

### CT-llm-AcquireProviderCeiling

- 边界：`TopicAcquireProviderCeiling` = `/llm/runtime/acquire-provider-ceiling`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L754)
- 接收provider：llm；Command [AcquireProviderCeilingCommand](#schema-acquireproviderceilingcommand) → Result [AcquireProviderCeilingResult](#schema-acquireproviderceilingresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleProviderCeilingPrerequisite`（llm）
- 接收入口：[handleProviderCeilingPrerequisite](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L846)
- 已观察外部引用：orchestrator [AcquireAndCommitProviderCeilingPrerequisite](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L241)
- 本topic入口/具体类型分支的Source审查索引：[runtimeCommandHasExactOrchestratorSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/runtime_physical_submission_guard.go#L174)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[runtimeCommandHasExactOrchestratorSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/runtime_physical_submission_guard.go#L174)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || command.WorkspaceID == "" || command.RunID == "" || command.AuthoritySnapshotID == "" || command.PrerequisiteID == ""`；`err := selection.Validate(); err != nil`；`command.MinimumBytes > 0 && ceiling.MaxBytes < command.MinimumBytes`；`err := ceiling.Validate(); err != nil`；`err := observation.Validate(); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-llm-acquireruntimecapabilities"></a>

### CT-llm-AcquireRuntimeCapabilities

- 边界：`TopicAcquireRuntimeCapabilities` = `/llm/runtime-prerequisite/acquire-capabilities`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L746)
- 接收provider：llm；Command [AcquireRuntimeCapabilitiesCommand](#schema-acquireruntimecapabilitiescommand) → Result [AcquireRuntimeCapabilitiesResult](#schema-acquireruntimecapabilitiesresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleRuntimeCapabilities`（llm）
- 接收入口：[handleRuntimeCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L211)
- 已观察外部引用：orchestrator [sendRuntimeCapabilityOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/prerequisite_event_adapter.go#L323)
- 本topic入口/具体类型分支的Source审查索引：[runtimeCommandHasExactOrchestratorSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/runtime_physical_submission_guard.go#L174)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[runtimeCommandHasExactOrchestratorSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/runtime_physical_submission_guard.go#L174)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-llm-llmcommandcompleteprovider"></a>

### CT-llm-LLMCommandCompleteProvider

- 边界：`TopicLLMCommandCompleteProvider` = `/llm/command/complete-provider`；CF03；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L5)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L748)
- 接收provider：llm；Command [CompleteProviderCommand](#schema-completeprovidercommand) → Result [CompleteProviderResult](#schema-completeproviderresult)；target合法caller：application/workorchd/appservice（setupdraft）、application/workorchd（只允许specificationCompletionKindAllowed集合且Request.WorkspaceID、ExpectedConfigurationRevision非空）
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleCommand`（llm）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L922)
- 已观察外部引用：workorchd [completeDefinitionDocument](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_specification.go#L64)；workorchd [GenerateWorkspaceSetupDraft](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/setupdraft/generator.go#L23)
- 本topic入口/具体类型分支的Source审查索引：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`ev == nil`；`ev.Source() == llmSetupDraftAppserviceSource`；`ev.Source() != llmApplicationOwnerSource`；`err = response.ValidateFor(cmd); err == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-llm-llmcommandcompleteproviderdocument"></a>

### CT-llm-LLMCommandCompleteProviderDocument

- 边界：`TopicLLMCommandCompleteProviderDocument` = `/llm/command/complete-provider-document`；CF05；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L4)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L747)
- 接收provider：llm；Command [CompleteProviderDocumentCommand](#schema-completeproviderdocumentcommand) → Result [CompleteProviderDocumentResult](#schema-completeproviderdocumentresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleCommand`（llm）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L922)
- 已观察外部引用：workorchd [completeAuthoringDocument](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_document.go#L17)
- 本topic入口/具体类型分支的Source审查索引：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：仅application/workorchd的authoring文档入口；Runtime不冒用该Source，使用独立runtime candidate/preflight合同。Document/Failure/Incomplete恰一臂，exact request/config/target hash与submitted/usage均匹配。
- 验收适用集：[CP03](../Workorch_组件固定合同与分层验收规范.md#cp03)

<a id="ct-llm-llmcommandgenerateembedding"></a>

### CT-llm-LLMCommandGenerateEmbedding

- 边界：`TopicLLMCommandGenerateEmbedding` = `/llm/command/generate-embedding`；CF03；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L749)
- 接收provider：llm；Command [GenerateEmbeddingCommand](#schema-generateembeddingcommand) → Result [GenerateEmbeddingResult](#schema-generateembeddingresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleCommand`（llm）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L922)
- 已观察外部引用：workorchd [GenerateEmbedding](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/activationports/embedding.go#L19)
- 本topic入口/具体类型分支的Source审查索引：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || strings.TrimSpace(cmd.Input) == ""`；`err = response.ValidateFor(cmd); err == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-llm-llmcommandgetactiveconfiguration"></a>

### CT-llm-LLMCommandGetActiveConfiguration

- 边界：`TopicLLMCommandGetActiveConfiguration` = `/llm/command/get-active-configuration`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L750)
- 接收provider：llm；Command [GetActiveConfigurationCommand](#schema-getactiveconfigurationcommand) → Result [GetActiveConfigurationResult](#schema-getactiveconfigurationresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleCommand`（llm）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L922)
- 已观察外部引用：workorchd [activeDefinitionProviderConfiguration](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/definition_authoring_specification.go#L135)；workorchd [GetActiveLLMConfiguration](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/configcore/llm_catalog.go#L84)
- 本topic入口/具体类型分支的Source审查索引：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-llm-llmcommandgetprovidermodelcatalog"></a>

### CT-llm-LLMCommandGetProviderModelCatalog

- 边界：`TopicLLMCommandGetProviderModelCatalog` = `/llm/command/get-provider-model-catalog`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L751)
- 接收provider：llm；Command [GetProviderModelCatalogCommand](#schema-getprovidermodelcatalogcommand) → Result [GetProviderModelCatalogResult](#schema-getprovidermodelcatalogresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleCommand`（llm）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L922)
- 已观察外部引用：workorchd [GetLLMModelCatalog](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/configcore/llm_catalog.go#L21)
- 本topic入口/具体类型分支的Source审查索引：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-llm-llmcommandrefreshprovidermodelcatalog"></a>

### CT-llm-LLMCommandRefreshProviderModelCatalog

- 边界：`TopicLLMCommandRefreshProviderModelCatalog` = `/llm/command/refresh-provider-model-catalog`；CF03；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L752)
- 接收provider：llm；Command [RefreshProviderModelCatalogCommand](#schema-refreshprovidermodelcatalogcommand) → Result [RefreshProviderModelCatalogResult](#schema-refreshprovidermodelcatalogresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleCommand`（llm）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L922)
- 已观察外部引用：workorchd [RefreshLLMModelCatalog](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/configcore/llm_catalog.go#L45)
- 本topic入口/具体类型分支的Source审查索引：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-llm-llmcommandvalidateinferenceselection"></a>

### CT-llm-LLMCommandValidateInferenceSelection

- 边界：`TopicLLMCommandValidateInferenceSelection` = `/llm/command/validate-inference-selection`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L753)
- 接收provider：llm；Command [ValidateProviderInferenceSelectionCommand](#schema-validateproviderinferenceselectioncommand) → Result [ValidateProviderInferenceSelectionResult](#schema-validateproviderinferenceselectionresult)；target合法caller：application/workorchd
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleCommand`（llm）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L922)
- 已观察外部引用：workorchd [EnsureProviderInferenceSelection](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/activationports/provider_target_readiness.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-llm-llmusagerecorded"></a>

### CT-llm-LLMUsageRecorded

- 边界：`TopicLLMUsageRecorded` = `/llm/usage/recorded`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L19)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L757)
- 唯一事实owner/发布者：llm；Data [LLMUsageRecordedData](#schema-llmusagerecordeddata)；同步Command/Result：不适用；允许订阅者：eventstream、workorchd
- 实际注册：[subscribeLLMNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L351) → `subscribeLLMNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：[publishLLMUsageRecorded](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L623)
- 已观察外部引用：workorchd [isLLMUsageEventRecordForRun](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/readmodelprojection/projection_contract_views.go#L468)；workorchd [RunLLMUsageEventRecordOwnerQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/readmodelcore/query.go#L39)；workorchd [WorkspaceLLMUsageEventRecordOwnerQuery](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/readmodelcore/query.go#L53)；eventstream [subscribeLLMNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L351)；eventstream [llmUsageRecord](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L1010)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-llm-preflightruntimecandidate"></a>

### CT-llm-PreflightRuntimeCandidate

- 边界：`TopicPreflightRuntimeCandidate` = `/llm/runtime-candidate/preflight`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L755)
- 接收provider：llm；Command [PreflightRuntimeCandidateCommand](#schema-preflightruntimecandidatecommand) → Result [PreflightRuntimeCandidateResult](#schema-preflightruntimecandidateresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleRuntimeCandidatePreflight`（llm）
- 接收入口：[handleRuntimeCandidatePreflight](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L289)
- 已观察外部引用：orchestrator [PreflightRuntimeCandidate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_candidate_event_adapter.go#L22)
- 本topic入口/具体类型分支的Source审查索引：[runtimeCommandHasExactOrchestratorSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/runtime_physical_submission_guard.go#L174)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[runtimeCommandHasExactOrchestratorSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/runtime_physical_submission_guard.go#L174)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || !runtimeCandidateCommandScopeValid(command.DocumentKind, command.WorkspaceID, command.RunID, command.Decision, command.SemanticContext)`；`command.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-llm-requestruntimecandidate"></a>

### CT-llm-RequestRuntimeCandidate

- 边界：`TopicRequestRuntimeCandidate` = `/llm/runtime-candidate/request`；CF03；source existing / target retain；CommandTopic / Execute/Execute
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L756)
- 接收provider：llm；Command [RequestRuntimeCandidateCommand](#schema-requestruntimecandidatecommand) → Result [RequestRuntimeCandidateResult](#schema-requestruntimecandidateresult)；target合法caller：kernel/orchestrator
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L109) → `handleRuntimeCandidate`（llm）
- 接收入口：[handleRuntimeCandidate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L388)
- 已观察外部引用：orchestrator [sendRuntimeCandidateRequest](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_candidate_event_adapter.go#L136)
- 本topic入口/具体类型分支的Source审查索引：[runtimeCommandHasExactOrchestratorSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/runtime_physical_submission_guard.go#L174)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[runtimeCommandHasExactOrchestratorSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/runtime_physical_submission_guard.go#L174)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`!ok || m.candidateProvider == nil || m.physicalSubmissionGuard == nil || !runtimeCandidateCommandScopeValid(command.DocumentKind, command.WorkspaceID, command.RunID, command.Decision, command.SemanticContext)`；`err := command.RequestIdentity.Validate(); err != nil`；`command.RequestIdentity.Normalized() != command.RequestIdentity`；`validateErr := candidateResult.Validate(); validateErr != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-acquireproviderceilingcommand"></a>

### AcquireProviderCeilingCommand

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:130](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L130)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`PrerequisiteID string`；`Selection runtimecontract.ProviderInferenceSelection`；`MinimumBytes int`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)

<a id="schema-acquireproviderceilingresult"></a>

### AcquireProviderCeilingResult

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:139](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L139)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`AuthoritySnapshotID string`；`Ceiling runtimecontract.ProviderCeiling`；`Observation runtimecontract.RuntimePrerequisiteObservation`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ProviderCeiling（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1616)；[RuntimePrerequisiteObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_prerequisite_freshness.go#L101)

<a id="schema-acquireruntimecapabilitiescommand"></a>

### AcquireRuntimeCapabilitiesCommand

- 源码：[internal/modules/blocks/llm/pkg/events/runtime_capability.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/runtime_capability.go#L5)
- 声明：`type AcquireRuntimeCapabilitiesCommand runtimecontract.RuntimeCapabilityAcquisitionRequest`
- 字段引用类型：[RuntimeCapabilityAcquisitionRequest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L15)

<a id="schema-acquireruntimecapabilitiesresult"></a>

### AcquireRuntimeCapabilitiesResult

- 源码：[internal/modules/blocks/llm/pkg/events/runtime_capability.go:6](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/runtime_capability.go#L6)
- 字段：`Acquisition runtimecontract.RuntimeCapabilityAcquisition`
- 字段引用类型：[RuntimeCapabilityAcquisition（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_capability_acquisition.go#L27)

<a id="schema-completeprovidercommand"></a>

### CompleteProviderCommand

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:21](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L21)
- 字段：`Target runtimecontract.ProviderModelTarget`；`Request ProviderRequest`；`ExpectedConfigurationRevision string`
- 字段引用类型：[ProviderModelTarget（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_model_target.go#L16)；[ProviderRequest](#schema-providerrequest)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L519)
- 空值/identity/版本/范围原式：`err := target.Validate(); err != nil`；`r.Target != target`；`requested.ProviderID != "" || requested.ModelID != ""`；`err := requested.Validate(); err != nil || !target.Same(requested)`；`model := strings.TrimSpace(command.Request.Model); model != "" && target.ModelID != model`；`strings.TrimSpace(r.Completion.Content) == "" || !utf8.ValidString(r.Completion.Content)`；`err := validateGenericProviderUsage(r.Completion.Usage); err != nil`

<a id="schema-completeproviderdocumentcommand"></a>

### CompleteProviderDocumentCommand

- 源码：[internal/modules/blocks/llm/pkg/events/provider_document.go:18](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L18)
- 字段：`RequestIdentity string`；`Target runtimecontract.ProviderModelTarget`；`Request ProviderRequest`；`ExpectedConfigurationRevision string`
- 字段引用类型：[ProviderModelTarget（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_model_target.go#L16)；[ProviderRequest](#schema-providerrequest)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L25)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L76)；[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L134)
- 空值/identity/版本/范围原式：`c.RequestIdentity == "" || len(c.RequestIdentity) > 256 || strings.TrimSpace(c.RequestIdentity) != c.RequestIdentity || strings.ContainsRune(c.RequestIdentity, 0) || c.Target.Validate() != nil || c.Target != c.Target.Normalized() || c.ExpectedConfigurationRevision == ""`；`_, err := c.Request.ToModel(); err != nil`；`c.Request.Model != c.Target.ModelID`；`err != nil || r.RequestIdentity != c.RequestIdentity || r.RequestHash != hash || r.Target != c.Target || r.ConfigurationRevision != c.ExpectedConfigurationRevision`；`r.PhysicalResponseMode.Validate() != nil || r.PhysicalResponseMode != r.PhysicalResponseMode.Normalized() || validateGenericProviderUsage(r.Usage) != nil || r.Wire.Validate() != nil || r.ObservedDocument.Validate() != nil`；`r.TransportCause != nil && (r.TransportCause.Validate() != nil || !r.TransportCause.Observed || !r.TransportCause.Complete || r.Failure == nil)`；`r.AuditPublicationFailureClass != "" && r.AuditPublicationFailureClass != "audit_publication_failed"`；`r.Document != nil`；`!r.ProviderSubmitted || !r.Document.Content.Observed || !r.Document.Content.Complete || r.Document.Content.Validate() != nil || !r.ObservedDocument.Complete || !reflect.DeepEqual(r.Document.Content, r.ObservedDocument)`；`r.Failure != nil`；`!r.ProviderSubmitted || !providerDocumentCodeCanonical(r.Failure.Code) || len(r.Failure.Cause) == 0`；`r.Incomplete != nil`；`!r.ProviderSubmitted || (r.Incomplete.Code != "output_limit" && r.Incomplete.Code != "stream_ended_before_completion") || len(r.Incomplete.Cause) == 0 || r.ObservedDocument.Complete`；`arms != 1`

<a id="schema-completeproviderdocumentresult"></a>

### CompleteProviderDocumentResult

- 源码：[internal/modules/blocks/llm/pkg/events/provider_document.go:117](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L117)
- 字段：`RequestIdentity string`；`RequestHash string`；`Target runtimecontract.ProviderModelTarget`；`ConfigurationRevision string`；`Document *ProviderCompleteDocument`；`Failure *ProviderDocumentFailure`；`Incomplete *ProviderIncompleteDocument`；`ObservedDocument models.ProviderObservedContent`；`Wire models.ProviderObservedContent`；`TransportCause *models.ProviderObservedContent`；`Usage *llmaudit.ProviderUsage`；`ProviderSubmitted bool`；`PhysicalResponseMode llmaudit.PhysicalResponseMode`；`AuditPublicationFailureClass string`
- 字段引用类型：[ProviderModelTarget（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_model_target.go#L16)；[ProviderCompleteDocument](#schema-providercompletedocument)；[ProviderDocumentFailure](#schema-providerdocumentfailure)；[ProviderIncompleteDocument](#schema-providerincompletedocument)；[ProviderObservedContent](#schema-providerobservedcontent)；[ProviderUsage（shared/llmaudit声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/llmaudit/audit.go#L8)；[PhysicalResponseMode（shared/llmaudit声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/llmaudit/physical_response_mode.go#L11)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L134)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L185)
- 空值/identity/版本/范围原式：`err != nil || r.RequestIdentity != c.RequestIdentity || r.RequestHash != hash || r.Target != c.Target || r.ConfigurationRevision != c.ExpectedConfigurationRevision`；`r.PhysicalResponseMode.Validate() != nil || r.PhysicalResponseMode != r.PhysicalResponseMode.Normalized() || validateGenericProviderUsage(r.Usage) != nil || r.Wire.Validate() != nil || r.ObservedDocument.Validate() != nil`；`r.TransportCause != nil && (r.TransportCause.Validate() != nil || !r.TransportCause.Observed || !r.TransportCause.Complete || r.Failure == nil)`；`r.AuditPublicationFailureClass != "" && r.AuditPublicationFailureClass != "audit_publication_failed"`；`r.Document != nil`；`!r.ProviderSubmitted || !r.Document.Content.Observed || !r.Document.Content.Complete || r.Document.Content.Validate() != nil || !r.ObservedDocument.Complete || !reflect.DeepEqual(r.Document.Content, r.ObservedDocument)`；`r.Failure != nil`；`!r.ProviderSubmitted || !providerDocumentCodeCanonical(r.Failure.Code) || len(r.Failure.Cause) == 0`；`r.Incomplete != nil`；`!r.ProviderSubmitted || (r.Incomplete.Code != "output_limit" && r.Incomplete.Code != "stream_ended_before_completion") || len(r.Incomplete.Cause) == 0 || r.ObservedDocument.Complete`；`arms != 1`

<a id="schema-completeproviderresult"></a>

### CompleteProviderResult

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:514](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L514)
- 字段：`Target runtimecontract.ProviderModelTarget`；`Completion ProviderCompletionData`
- 字段引用类型：[ProviderModelTarget（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_model_target.go#L16)；[ProviderCompletionData](#schema-providercompletiondata)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L519)
- 空值/identity/版本/范围原式：`err := target.Validate(); err != nil`；`r.Target != target`；`requested.ProviderID != "" || requested.ModelID != ""`；`err := requested.Validate(); err != nil || !target.Same(requested)`；`model := strings.TrimSpace(command.Request.Model); model != "" && target.ModelID != model`；`strings.TrimSpace(r.Completion.Content) == "" || !utf8.ValidString(r.Completion.Content)`；`err := validateGenericProviderUsage(r.Completion.Usage); err != nil`

<a id="schema-generateembeddingcommand"></a>

### GenerateEmbeddingCommand

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:88](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L88)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID string`；`Target runtimecontract.ProviderModelTarget`；`Input string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ProviderModelTarget（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_model_target.go#L16)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L553)
- 空值/identity/版本/范围原式：`err := target.Validate(); err != nil`；`r.Target != target`；`err := requested.Validate(); err != nil || !target.Same(requested)`；`input == "" || r.Embedding.ProviderID != target.ProviderID || r.Embedding.Model != target.ModelID || r.Embedding.Input != input`；`len(r.Embedding.Vector) == 0`；`math.IsNaN(value) || math.IsInf(value, 0)`；`r.Embedding.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || !r.Embedding.ProviderSubmitted || r.Embedding.RequestBytes <= 0 || r.Embedding.ResponseBytes <= 0`；`err := validateGenericProviderUsage(r.Embedding.Usage); err != nil`

<a id="schema-generateembeddingresult"></a>

### GenerateEmbeddingResult

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:548](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L548)
- 字段：`Target runtimecontract.ProviderModelTarget`；`Embedding llmmodel.ProviderEmbeddingResult`
- 字段引用类型：[ProviderModelTarget（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_model_target.go#L16)；[ProviderEmbeddingResult](#schema-providerembeddingresult)
- 实际方法：[ValidateFor](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L553)
- 空值/identity/版本/范围原式：`err := target.Validate(); err != nil`；`r.Target != target`；`err := requested.Validate(); err != nil || !target.Same(requested)`；`input == "" || r.Embedding.ProviderID != target.ProviderID || r.Embedding.Model != target.ModelID || r.Embedding.Input != input`；`len(r.Embedding.Vector) == 0`；`math.IsNaN(value) || math.IsInf(value, 0)`；`r.Embedding.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || !r.Embedding.ProviderSubmitted || r.Embedding.RequestBytes <= 0 || r.Embedding.ResponseBytes <= 0`；`err := validateGenericProviderUsage(r.Embedding.Usage); err != nil`

<a id="schema-getactiveconfigurationcommand"></a>

### GetActiveConfigurationCommand

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:99](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L99)
- 声明：`type GetActiveConfigurationCommand struct{}`

<a id="schema-getactiveconfigurationresult"></a>

### GetActiveConfigurationResult

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:101](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L101)
- 字段：`ProviderID string`；`ConfigurationRevision string`；`DefaultChat runtimecontract.ProviderModelTarget`；`DefaultEmbedding *runtimecontract.ProviderModelTarget`
- 字段引用类型：[ProviderModelTarget（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_model_target.go#L16)

<a id="schema-getprovidermodelcatalogcommand"></a>

### GetProviderModelCatalogCommand

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:108](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L108)
- 声明：`type GetProviderModelCatalogCommand struct{}`

<a id="schema-getprovidermodelcatalogresult"></a>

### GetProviderModelCatalogResult

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:110](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L110)
- 字段：`Catalog llmmodel.ProviderModelCatalog`
- 字段引用类型：[ProviderModelCatalog](#schema-providermodelcatalog)

<a id="schema-llmusagerecordeddata"></a>

### LLMUsageRecordedData

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:608](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L608)
- 字段：`RequestID string`；`LogicalRequestID string`；`PhysicalCallID string`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID string`；`Target runtimecontract.ProviderModelTarget`；`Operation string`；`CapabilityRevision string`；`ConfigurationRevision string`；`StablePrefixHash string`；`Kind string`；`APISurface string`；`Provider string`；`PhysicalResponseMode llmaudit.PhysicalResponseMode`；`ProviderSubmitted bool`；`Status string`；`FailureClass string`；`StartedAt string`；`CompletedAt string`；`DurationMs int`；`RequestBytes int`；`ResponseBytes int`；`InputTokens int`；`OutputTokens int`；`TotalTokens int`；`Estimated bool`；`UsageSource string`；`ContextBudget llmaudit.ContextBudget`；`CacheUsage llmaudit.CacheUsage`；`OccurredAt string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ProviderModelTarget（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_model_target.go#L16)；[PhysicalResponseMode（shared/llmaudit声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/llmaudit/physical_response_mode.go#L11)；[ContextBudget（shared/llmaudit声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/llmaudit/audit.go#L23)；[CacheUsage（shared/llmaudit声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/llmaudit/audit.go#L135)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L642)；[validateLLMUsageRequestIdentity](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L706)
- 空值/identity/版本/范围原式：`!canonicalNonEmptyAuditString(d.RequestID) || !canonicalNonEmptyAuditString(d.Status)`；`d.Status != "completed" && d.Status != "failed"`；`d.Kind != strings.TrimSpace(d.Kind)`；`string(d.WorkspaceID) != strings.TrimSpace(string(d.WorkspaceID)) || string(d.RunID) != strings.TrimSpace(string(d.RunID)) || (d.RunID != "" && d.WorkspaceID == "")`；`err := d.PhysicalResponseMode.Validate(); err != nil`；`d.PhysicalResponseMode != d.PhysicalResponseMode.Normalized()`；`err := validateLLMUsageRequestIdentity(d); err != nil`；`value < 0`；`d.TotalTokens < d.InputTokens || d.TotalTokens < d.OutputTokens`；`!d.ContextBudget.Empty()`；`d.ContextBudget.ProviderSubmitted != d.ProviderSubmitted`；`err := validateLLMContextBudgetMeasurements(d.ContextBudget); err != nil`；`d.ContextBudget.ActualTokens != d.TotalTokens`；`!d.ProviderSubmitted`；`d.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || d.ResponseBytes != 0 || d.InputTokens != 0 || d.OutputTokens != 0 || d.TotalTokens != 0 || d.ContextBudget.ActualTokens != 0 || !llmCacheUsageHasNoTokens(d.CacheUsage)`；`d.RequestBytes <= 0`；`logicalID != data.LogicalRequestID || physicalID != data.PhysicalCallID`；`runtimeRequest && (logicalID == "" || physicalID == "")`；`(logicalID == "") != (physicalID == "")`；`physicalID != "" && data.RequestID != physicalID`

<a id="schema-normalizationmanifest"></a>

### NormalizationManifest

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:64](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L64)
- 字段：`NormalizedSections []string`；`ProtectedSections []string`；`EstimatedTokensSaved int`；`SemanticLoss bool`

<a id="schema-preflightruntimecandidatecommand"></a>

### PreflightRuntimeCandidateCommand

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:168](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L168)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DocumentKind runtimecontract.RuntimeProviderDocumentKind`；`Decision runtimecontract.RuntimeDecision`；`Transaction runtimecontract.AuthorityTransaction`；`CandidateAttempt int`；`CandidateCorrectionObservation *runtimecontract.RuntimeCandidateCorrectionObservation`；`CandidateCorrection *llmmodel.RuntimeCandidateCorrectionInput`；`StablePrefixHash string`；`CandidateManifest runtimecontract.RuntimeCandidateContractManifest`；`DefinitionContext runtimecontract.RuntimeDefinitionContext`；`CandidateContext runtimecontract.RuntimeCandidateContext`；`SemanticManifest runtimecontract.RuntimeSemanticPlanManifest`；`SemanticContext runtimecontract.RuntimeSemanticPlanRequestContext`；`SemanticCorrectionObservation *runtimecontract.RuntimeSemanticCorrectionObservation`；`SemanticCorrectionDirective *runtimecontract.RuntimeSemanticCorrectionDirective`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeProviderDocumentKind（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_contract.go#L26)；[RuntimeDecision（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1741)；[AuthorityTransaction（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1750)；[RuntimeCandidateCorrectionObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_correction_observation.go#L35)；[RuntimeCandidateCorrectionInput](#schema-runtimecandidatecorrectioninput)；[RuntimeCandidateContractManifest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/candidate_contract_manifest.go#L71)；[RuntimeDefinitionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_definition.go#L253)；[RuntimeCandidateContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate.go#L17)；[RuntimeSemanticPlanManifest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_contract.go#L186)；[RuntimeSemanticPlanRequestContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_contract.go#L604)；[RuntimeSemanticCorrectionObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_correction_observation.go#L242)；[RuntimeSemanticCorrectionDirective（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_correction.go#L203)

<a id="schema-preflightruntimecandidateresult"></a>

### PreflightRuntimeCandidateResult

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:187](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L187)
- 字段：`Transaction runtimecontract.AuthorityTransaction`；`DocumentKind runtimecontract.RuntimeProviderDocumentKind`；`Feedback runtimecontract.RuntimeFeedback`；`Audit *runtimecontract.ProviderRequestAuditView`
- 字段引用类型：[AuthorityTransaction（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1750)；[RuntimeProviderDocumentKind（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_contract.go#L26)；[RuntimeFeedback（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_feedback.go#L63)；[ProviderRequestAuditView（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_request_audit.go#L7)

<a id="schema-promptcontext"></a>

### PromptContext

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:36](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L36)
- 字段：`SchemaVersion string`；`Sections []PromptSection`；`Turns []PromptTurn`；`Normalization *NormalizationManifest`
- 字段引用类型：[PromptSection](#schema-promptsection)；[PromptTurn](#schema-promptturn)；[NormalizationManifest](#schema-normalizationmanifest)

<a id="schema-promptsection"></a>

### PromptSection

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:43](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L43)
- 字段：`ID string`；`Name string`；`Role string`；`Policy string`；`Cacheability string`；`Hash string`；`SourceRefs []string`；`DefinitionRevision string`；`DisplayOrder int`；`Content string`；`Normalized bool`；`Protected bool`

<a id="schema-promptturn"></a>

### PromptTurn

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:58](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L58)
- 字段：`Role string`；`Content string`；`SectionID string`

<a id="schema-provideradaptercachehint"></a>

### ProviderAdapterCacheHint

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:167](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L167)
- 字段：`Enabled bool`；`Mode string`；`ProviderMapping string`；`CacheablePrefix []string`；`VolatileSections []string`；`CacheBreakers []string`

<a id="schema-providercachebreaker"></a>

### ProviderCacheBreaker

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:148](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L148)
- 字段：`SectionID string`；`Reason string`

<a id="schema-providercacheplan"></a>

### ProviderCachePlan

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:120](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L120)
- 字段：`SchemaVersion string`；`Strategy string`；`CacheKey string`；`StablePrefixHash string`；`RenderedStablePrefixHash string`；`WorkspaceDefinitionHash string`；`CapabilityContractHash string`；`StageBundleHash string`；`VolatileTailHash string`；`LargeRefsHash string`；`SectionHashes []ProviderCacheSectionHash`；`CacheBreakers []ProviderCacheBreaker`；`Canonicalization ProviderCanonicalizationPlan`；`ObservedRequestShape ProviderObservedRequestShape`；`AdapterHint ProviderAdapterCacheHint`
- 字段引用类型：[ProviderCacheSectionHash](#schema-providercachesectionhash)；[ProviderCacheBreaker](#schema-providercachebreaker)；[ProviderCanonicalizationPlan](#schema-providercanonicalizationplan)；[ProviderObservedRequestShape](#schema-providerobservedrequestshape)；[ProviderAdapterCacheHint](#schema-provideradaptercachehint)

<a id="schema-providercachesectionhash"></a>

### ProviderCacheSectionHash

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:138](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L138)
- 字段：`SectionID string`；`Name string`；`Cacheability string`；`Hash string`；`SizeBytes int`；`SourceRefs []string`；`DefinitionRevision string`

<a id="schema-providercanonicalizationplan"></a>

### ProviderCanonicalizationPlan

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:153](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L153)
- 字段：`SectionOrderPolicy string`；`SectionOrder []string`；`OrderedDataPolicy string`；`MapKeyPolicy string`；`ProviderMapping string`

<a id="schema-providercapabilities"></a>

### ProviderCapabilities

- 源码：[internal/modules/blocks/llm/pkg/models/provider_config.go:37](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_config.go#L37)
- 字段：`Responses bool`；`Streaming bool`；`JSONSchemaResponse bool`；`NonChatOperations []string`

<a id="schema-providercatalogstate"></a>

### ProviderCatalogState

- 源码：[internal/modules/blocks/llm/pkg/models/provider_model_catalog.go:18](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_model_catalog.go#L18)
- 声明：`type ProviderCatalogState string`
- 源码枚举值：`ProviderCatalogStateFresh = "fresh"`；`ProviderCatalogStateStale = "stale"`；`ProviderCatalogStateUnavailable = "unavailable"`

<a id="schema-providercompletedocument"></a>

### ProviderCompleteDocument

- 源码：[internal/modules/blocks/llm/pkg/events/provider_document.go:101](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L101)
- 字段：`Content models.ProviderObservedContent`
- 字段引用类型：[ProviderObservedContent](#schema-providerobservedcontent)

<a id="schema-providercompletiondata"></a>

### ProviderCompletionData

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:505](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L505)
- 字段：`Content string`；`Usage *llmaudit.ProviderUsage`
- 字段引用类型：[ProviderUsage（shared/llmaudit声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/llmaudit/audit.go#L8)

<a id="schema-providercontextbudget"></a>

### ProviderContextBudget

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:71](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L71)
- 字段：`ProviderID string`；`ModelID string`；`CapabilityRevision string`；`ConfigurationRevision string`；`ContextWindowTokens int`；`ProviderMaxOutputTokens int`；`EffectiveMaxOutputTokens int`；`MaxBytes int`；`ActualBytes int`；`MaxTokens int`；`EstimatedInputTokens int`；`EstimatedOutputTokens int`；`ActualTokens int`；`MinimumProtectedBytes int`；`ShrinkLevel int`；`Truncated bool`；`Rules []string`；`SourceRefs []string`；`IncludedSections []string`；`NormalizedSections []string`；`CompressedSections []string`；`OmittedSections []string`；`GoalDoDCompressed bool`；`OverBudget bool`；`P0Overflow bool`；`ProfileMode string`；`CapabilitySource string`；`ContextUtilization float64`；`MaxRequestBytes int`；`StageBudgetRatio float64`；`StageBudgetSource string`；`TracePolicy string`；`PromptCaptured bool`；`Estimator string`；`RedactionApplied bool`；`RedactedPaths []string`；`ProtectedFields []string`；`IntegrityWarnings []string`；`ContextGateAction string`；`ContextGateReason string`；`BudgetFailureClass string`；`ProviderSubmitted bool`；`CachePlan *ProviderCachePlan`；`Profile ProviderContextProfile`
- 字段引用类型：[ProviderCachePlan](#schema-providercacheplan)；[ProviderContextProfile](#schema-providercontextprofile)

<a id="schema-providercontextprofile"></a>

### ProviderContextProfile

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:176](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L176)
- 字段：`ProfileMode string`；`CapabilitySource string`；`ContextWindowTokens int`；`MaxInputTokens int`；`ReservedOutputTokens int`；`SafetyRatio float64`；`ContextUtilizationRatio float64`；`BytesHardLimit int`；`MaxRequestBytes int`；`EstimatedTokensPerByte float64`

<a id="schema-providerdocumentfailure"></a>

### ProviderDocumentFailure

- 源码：[internal/modules/blocks/llm/pkg/events/provider_document.go:105](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L105)
- 字段：`Code string`；`Cause []byte`

<a id="schema-providerembeddingresult"></a>

### ProviderEmbeddingResult

- 源码：[internal/modules/blocks/llm/pkg/models/provider_config.go:55](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_config.go#L55)
- 字段：`ProviderID string`；`Model string`；`Input string`；`Vector []float64`；`Usage *llmaudit.ProviderUsage`；`PhysicalResponseMode llmaudit.PhysicalResponseMode`；`ProviderSubmitted bool`；`RequestBytes int`；`ResponseBytes int`
- 字段引用类型：[ProviderUsage（shared/llmaudit声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/llmaudit/audit.go#L8)；[PhysicalResponseMode（shared/llmaudit声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/llmaudit/physical_response_mode.go#L11)

<a id="schema-providerincompletedocument"></a>

### ProviderIncompleteDocument

- 源码：[internal/modules/blocks/llm/pkg/events/provider_document.go:110](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go#L110)
- 字段：`Code string`；`Cause []byte`

<a id="schema-providerinferenceprofile"></a>

### ProviderInferenceProfile

- 源码：[internal/modules/blocks/llm/pkg/models/provider_config.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_config.go#L31)
- 字段：`Temperature *float64`；`MaxOutputTokens int`；`ThinkingBudgetTokens int`

<a id="schema-providermodelcatalog"></a>

### ProviderModelCatalog

- 源码：[internal/modules/blocks/llm/pkg/models/provider_model_catalog.go:116](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_model_catalog.go#L116)
- 字段：`ProviderID string`；`Models []ProviderModelDescriptor`；`Revision string`；`FetchedAt time.Time`；`ExpiresAt time.Time`；`Stale bool`；`Source string`；`State ProviderCatalogState`；`ErrorCode string`；`ErrorMessage string`
- 字段引用类型：[ProviderModelDescriptor](#schema-providermodeldescriptor)；[ProviderCatalogState](#schema-providercatalogstate)

<a id="schema-providermodeldescriptor"></a>

### ProviderModelDescriptor

- 源码：[internal/modules/blocks/llm/pkg/models/provider_model_catalog.go:27](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_model_catalog.go#L27)
- 字段：`Target runtimecontract.ProviderModelTarget`；`RouteOwner string`；`ContextWindowTokens int`；`MaxOutputTokens int`；`SupportedEndpoints []string`；`Operations []string`；`Reasoning ProviderReasoningCapability`；`CapabilityReady bool`；`UnavailableReason string`；`CapabilityRevision string`
- 字段引用类型：[ProviderModelTarget（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_model_target.go#L16)；[ProviderReasoningCapability](#schema-providerreasoningcapability)

<a id="schema-providerobservedcontent"></a>

### ProviderObservedContent

- 源码：[internal/modules/blocks/llm/pkg/models/provider_document.go:12](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_document.go#L12)
- 字段：`Content []byte`；`ObservedBytes uint64`；`SHA256 string`；`Complete bool`；`Observed bool`；`Saved bool`；`Reason string`
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_document.go#L34)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_document.go#L41)
- 空值/identity/版本/范围原式：`!o.Observed`；`o.Content != nil || o.ObservedBytes != 0 || o.SHA256 != "" || o.Complete || o.Saved || o.Reason == ""`；`err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != o.SHA256`；`!o.Complete && o.Reason == ""`；`o.Saved`；`o.Content == nil || o.ObservedBytes != uint64(len(o.Content)) || len(o.Content) > ProviderDocumentObservationLimit`；`hex.EncodeToString(hash[:]) != o.SHA256`；`o.Content != nil || o.ObservedBytes <= ProviderDocumentObservationLimit || o.Reason != "observation_storage_limit"`

<a id="schema-providerobservedrequestshape"></a>

### ProviderObservedRequestShape

- 源码：[internal/modules/blocks/llm/pkg/models/decision.go:161](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/decision.go#L161)
- 字段：`ProviderFacingMode string`；`TurnsHaveSectionIDs bool`；`TypedEventTape bool`

<a id="schema-providerreasoningcapability"></a>

### ProviderReasoningCapability

- 源码：[internal/modules/blocks/llm/pkg/models/provider_model_catalog.go:46](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_model_catalog.go#L46)
- 字段：`Supported bool`；`DefaultEffort string`；`Efforts []string`；`Valid bool`；`InvalidReason string`
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_model_catalog.go#L54)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_model_catalog.go#L109)
- 空值/identity/版本/范围原式：`!c.Supported`；`effort == ""`；`_, ok := seen[effort]; ok`；`len(c.Efforts) == 0`；`c.DefaultEffort != ""`；`_, ok := seen[c.DefaultEffort]; !ok`

<a id="schema-providerrequest"></a>

### ProviderRequest

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:32](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L32)
- 字段：`WorkspaceID string`；`RunID string`；`StepID string`；`Kind string`；`Model string`；`ReasoningEffort string`；`SystemLocale string`；`DialogLanguage string`；`Schema string`；`SchemaFormat string`；`MaxOutputTokens int`；`RawTextOutput bool`；`TraceRefs []TraceReference`；`Config llmmodel.RedactedProviderConfig`；`PromptContext *llmmodel.PromptContext`；`ContextBudget *llmmodel.ProviderContextBudget`
- 字段引用类型：[TraceReference](#schema-tracereference)；[RedactedProviderConfig](#schema-redactedproviderconfig)；[PromptContext](#schema-promptcontext)；[ProviderContextBudget](#schema-providercontextbudget)

<a id="schema-redactedproviderconfig"></a>

### RedactedProviderConfig

- 源码：[internal/modules/blocks/llm/pkg/models/provider_config.go:69](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/provider_config.go#L69)
- 字段：`ID string`；`APISurface string`；`BaseURL string`；`Model string`；`Timeout string`；`Disabled bool`；`Ready bool`；`Reason string`；`Capabilities ProviderCapabilities`；`InferenceProfile ProviderInferenceProfile`；`ContextProfile ProviderContextProfile`
- 字段引用类型：[ProviderCapabilities](#schema-providercapabilities)；[ProviderInferenceProfile](#schema-providerinferenceprofile)；[ProviderContextProfile](#schema-providercontextprofile)

<a id="schema-refreshprovidermodelcatalogcommand"></a>

### RefreshProviderModelCatalogCommand

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:114](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L114)
- 声明：`type RefreshProviderModelCatalogCommand struct{}`

<a id="schema-refreshprovidermodelcatalogresult"></a>

### RefreshProviderModelCatalogResult

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:116](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L116)
- 字段：`Catalog llmmodel.ProviderModelCatalog`
- 字段引用类型：[ProviderModelCatalog](#schema-providermodelcatalog)

<a id="schema-requestruntimecandidatecommand"></a>

### RequestRuntimeCandidateCommand

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:147](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L147)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`DocumentKind runtimecontract.RuntimeProviderDocumentKind`；`RequestIdentity runtimecontract.RuntimeCandidateRequestIdentity`；`ExpectedDocument *runtimecontract.RuntimeExpectedDocumentContract`；`Decision runtimecontract.RuntimeDecision`；`Transaction runtimecontract.AuthorityTransaction`；`CandidateAttempt int`；`CandidateCorrectionObservation *runtimecontract.RuntimeCandidateCorrectionObservation`；`CandidateCorrection *llmmodel.RuntimeCandidateCorrectionInput`；`StablePrefixHash string`；`CandidateManifest runtimecontract.RuntimeCandidateContractManifest`；`DefinitionContext runtimecontract.RuntimeDefinitionContext`；`CandidateContext runtimecontract.RuntimeCandidateContext`；`SemanticManifest runtimecontract.RuntimeSemanticPlanManifest`；`SemanticContext runtimecontract.RuntimeSemanticPlanRequestContext`；`SemanticCorrectionObservation *runtimecontract.RuntimeSemanticCorrectionObservation`；`SemanticCorrectionDirective *runtimecontract.RuntimeSemanticCorrectionDirective`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[RuntimeProviderDocumentKind（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_contract.go#L26)；[RuntimeCandidateRequestIdentity（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate_request_session.go#L127)；[RuntimeExpectedDocumentContract（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_expected_document.go#L17)；[RuntimeDecision（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1741)；[AuthorityTransaction（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1750)；[RuntimeCandidateCorrectionObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_correction_observation.go#L35)；[RuntimeCandidateCorrectionInput](#schema-runtimecandidatecorrectioninput)；[RuntimeCandidateContractManifest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/candidate_contract_manifest.go#L71)；[RuntimeDefinitionContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_definition.go#L253)；[RuntimeCandidateContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate.go#L17)；[RuntimeSemanticPlanManifest（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_contract.go#L186)；[RuntimeSemanticPlanRequestContext（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_contract.go#L604)；[RuntimeSemanticCorrectionObservation（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_correction_observation.go#L242)；[RuntimeSemanticCorrectionDirective（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_correction.go#L203)

<a id="schema-requestruntimecandidateresult"></a>

### RequestRuntimeCandidateResult

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:220](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L220)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`Transaction runtimecontract.AuthorityTransaction`；`DocumentKind runtimecontract.RuntimeProviderDocumentKind`；`RequestIdentity runtimecontract.RuntimeCandidateRequestIdentity`；`ExpectedDocumentHash string`；`DocumentOutcome *RuntimeProviderDocumentOutcome`；`Feedback *runtimecontract.RuntimeFeedback`；`Stale *runtimecontract.RuntimePrerequisiteStale`；`Audit *runtimecontract.ProviderRequestAuditView`；`Usage *llmaudit.ProviderUsage`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[AuthorityTransaction（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/canonical_runtime.go#L1750)；[RuntimeProviderDocumentKind（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_semantic_contract.go#L26)；[RuntimeCandidateRequestIdentity（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate_request_session.go#L127)；[RuntimeProviderDocumentOutcome](#schema-runtimeproviderdocumentoutcome)；[RuntimeFeedback（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_feedback.go#L63)；[RuntimePrerequisiteStale（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_prerequisite_freshness.go#L252)；[ProviderRequestAuditView（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_request_audit.go#L7)；[ProviderUsage（shared/llmaudit声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/llmaudit/audit.go#L8)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L238)；[validateRuntimeCandidateResultAudit](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L341)
- 空值/identity/版本/范围原式：`r.WorkspaceID == "" || r.RunID == "" || string(r.WorkspaceID) != strings.TrimSpace(string(r.WorkspaceID)) || string(r.RunID) != strings.TrimSpace(string(r.RunID))`；`err := r.RequestIdentity.Validate(); err != nil`；`r.RequestIdentity != r.RequestIdentity.Normalized()`；`strings.TrimSpace(r.ExpectedDocumentHash) == "" || r.ExpectedDocumentHash != strings.TrimSpace(r.ExpectedDocumentHash)`；`err := validateRuntimeCandidateResultAudit(r); err != nil`；`r.Stale != nil`；`err := stale.Validate(); err != nil`；`r.Feedback != nil`；`err := ValidateRuntimeProviderRequestFeedback(*r.Feedback); err != nil`；`r.DocumentOutcome != nil`；`err := r.DocumentOutcome.Validate(); err != nil`；`expected := strings.TrimSpace(r.ExpectedDocumentHash); expected == "" || expected != outcome.Framing.ExpectedDocumentHash`；`variants != 1`；`r.Audit.ProviderSubmitted || r.Usage != nil || strings.TrimSpace(r.Audit.Status) != "failed" || strings.TrimSpace(r.Audit.FailureClass) != string(runtimecontract.RuntimeCandidateFailureClassProviderConfiguration)`；`r.Feedback.WorkspaceID != r.WorkspaceID || r.Feedback.RunID != r.RunID || strings.TrimSpace(r.Audit.Status) != "failed"`；`err := validateRuntimeCandidateFeedbackAudit(*r.Feedback, *r.Audit); err != nil`；`!r.Audit.ProviderSubmitted`；`strings.TrimSpace(r.Audit.Status) != llmmodel.RuntimeCandidateStatusCompleted || strings.TrimSpace(r.Audit.FailureClass) != ""`；`audit == nil`；`!canonicalNonEmptyAuditString(audit.RequestID) || !canonicalNonEmptyAuditString(audit.LogicalRequestID) || !canonicalNonEmptyAuditString(audit.PhysicalCallID) || audit.RequestID != identity.PhysicalCallID || audit.LogicalRequestID != identity.LogicalRequestID || audit.PhysicalCallID != identity.PhysicalCallID`；`!canonicalNonEmptyAuditString(audit.WorkspaceID) || !canonicalNonEmptyAuditString(audit.RunID) || audit.WorkspaceID != string(result.WorkspaceID) || audit.RunID != string(result.RunID)`；`!canonicalNonEmptyAuditString(audit.Operation) || !canonicalNonEmptyAuditString(audit.StablePrefixHash)`；`result.DocumentKind == runtimecontract.RuntimeProviderDocumentSemanticPlan`；`audit.Kind != wantKind`；`err := audit.Target.Validate(); err != nil`；`audit.Target != audit.Target.Normalized()`；`err := audit.PhysicalResponseMode.Validate(); err != nil`；`audit.PhysicalResponseMode != audit.PhysicalResponseMode.Normalized()`；`audit.DurationMs < 0 || audit.RequestBytes < 0 || audit.ResponseBytes < 0`；`status != audit.Status || strings.TrimSpace(audit.FailureClass) != audit.FailureClass`；`status != "completed" && status != "failed"`；`status == "completed" && strings.TrimSpace(audit.FailureClass) != ""`；`status == "failed" && strings.TrimSpace(audit.FailureClass) == "" && result.Stale == nil`；`audit.ContextBudget.Empty() || audit.ContextBudget.ProviderSubmitted != audit.ProviderSubmitted`；`audit.ContextBudget.ProviderID != audit.Target.ProviderID || audit.ContextBudget.ModelID != audit.Target.ModelID`；`audit.ContextBudget.CapabilityRevision != audit.CapabilityRevision || audit.ContextBudget.ConfigurationRevision != audit.ConfigurationRevision`；`hash := audit.ContextBudget.StablePrefixHash; hash != "" && hash != audit.StablePrefixHash`；`value < 0`；`!audit.ProviderSubmitted`；`audit.PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || audit.ResponseBytes != 0 || result.Usage != nil || audit.ContextBudget.ActualTokens != 0`；`audit.RequestBytes <= 0`；`err := validateRuntimeCandidateResultUsage(result.Usage, audit.ContextBudget); err != nil`
- 显式分支集合（只表列源码case，default语义见validator）：`runtimecontract.RuntimeProviderDocumentOperations, runtimecontract.RuntimeProviderDocumentSemanticPlan`

<a id="schema-runtimecandidatecorrectioninput"></a>

### RuntimeCandidateCorrectionInput

- 源码：[internal/modules/blocks/llm/pkg/models/runtime_candidate.go:15](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/runtime_candidate.go#L15)
- 字段：`ManifestHash string`；`Objectives []runtimecontract.RuntimeCandidateCorrectionObjective`；`Witness runtimecontract.RuntimeCandidateCorrectionWitness`
- 字段引用类型：[RuntimeCandidateCorrectionObjective（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate.go#L209)；[RuntimeCandidateCorrectionWitness（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_candidate_evaluator.go#L56)
- 实际方法：[ValidateAgainstObservation](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/runtime_candidate_correction.go#L19)；[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/runtime_candidate.go#L21)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/runtime_candidate.go#L29)；[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/runtime_candidate.go#L37)；[StableRequestCorrectionHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/models/runtime_candidate.go#L48)
- 空值/identity/版本/范围原式：`manifest.Revision != runtimecontract.RuntimeCandidateContractManifestRevision || manifest.Hash == "" || manifest.Hash != runtimecontract.StableRuntimeCandidateContractManifestHash(manifest)`；`err := runtimecontract.ValidateRuntimeCandidateOperationCorrectionMetadata(manifest); err != nil`；`err := observation.Validate(); err != nil`；`observation.Diagnostic.WireRule != nil`；`err := runtimecontract.ValidateRuntimeCandidateFailureDiagnosticWireRule(manifest, observation.Diagnostic); err != nil`；`err := c.Validate(manifest.Hash); err != nil`；`err := runtimecontract.ValidateRuntimeCandidateCorrectionAgainstObservation( observation, c.ManifestHash, c.Objectives, c.Witness, ); err != nil`；`c.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessSchemaField`；`c.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessParameterEntryEncoding`；`c.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessOwnerConstraint`；`!found || constraint.ParameterName != runtimeCandidateCorrectionPathField(c.Witness.Path) || string(constraint.RequiredShape) != c.Witness.AlternativeValue`；`c.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessOperationValueRule`；`!found || len(rule.ParameterNames) == 0 || rule.ParameterNames[len(rule.ParameterNames)-1] != runtimeCandidateCorrectionPathField(c.Witness.Path) || !rule.AcceptsViolationShapes(runtimecontract.RuntimeOperationValueActualShape(c.Witness.RejectedValue), runtimecontract.RuntimeOperationValueRequiredShape(c.Witness.AlternativeValue))`；`c.Witness.Dimension == runtimecontract.RuntimeCandidateCorrectionWitnessOperationSetCardinality`；`manifest.OperationSetPolicy.MaxOperationsPerOwnerTarget != 1 || violation == nil || violation.Validate() != nil || violation.Maximum != manifest.OperationSetPolicy.MaxOperationsPerOwnerTarget`；`operation.OwnerModuleID == violation.OwnerModuleID`；`!ownerKnown`；`c.ManifestHash == "" || c.ManifestHash != strings.TrimSpace(manifestHash) || len(c.Objectives) == 0`；`c == nil`

<a id="schema-runtimeproviderdocumentoutcome"></a>

### RuntimeProviderDocumentOutcome

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:198](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L198)
- 字段：`Framing runtimecontract.RuntimeProviderDocumentFraming`；`Content []byte`
- 字段引用类型：[RuntimeProviderDocumentFraming（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_expected_document.go#L173)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L203)；[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L209)
- 空值/identity/版本/范围原式：`framing != o.Framing`；`err := runtimecontract.ValidateRuntimeProviderDocumentFramingContent(framing, o.Content); err != nil`

<a id="schema-tracereference"></a>

### TraceReference

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:27](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L27)
- 字段：`Name string`；`Ref string`

<a id="schema-validateproviderinferenceselectioncommand"></a>

### ValidateProviderInferenceSelectionCommand

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:120](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L120)
- 字段：`Selection runtimecontract.ProviderInferenceSelection`；`Operation string`
- 字段引用类型：[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)

<a id="schema-validateproviderinferenceselectionresult"></a>

### ValidateProviderInferenceSelectionResult

- 源码：[internal/modules/blocks/llm/pkg/events/messages.go:125](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/messages.go#L125)
- 字段：`Selection runtimecontract.ProviderInferenceSelection`；`CapabilityRevision string`
- 字段引用类型：[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [runtimeCommandHasExactOrchestratorSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/runtime_physical_submission_guard.go#L174)：来源集合由switch/返回表达式判定，见完整函数
  - helper完整分支：`func runtimeCommandHasExactOrchestratorSource(event interface{ Source() string }) bool { return event != nil && event.Source() == runtimecontract.RuntimeModuleOrchestrator }`
  - 当前来源常量：`runtimecontract.RuntimeModuleOrchestrator = "kernel/orchestrator"`
- [llmCommandHasExactApplicationSource](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/biz/owner.go#L1010)：`ev.Source() == llmSetupDraftAppserviceSource`；`ev.Source() != llmApplicationOwnerSource`
  - helper完整分支：`func llmCommandHasExactApplicationSource(ev magicevent.Event) bool { if ev == nil { return false } if ev.ID() == llmevents.TopicLLMCommandCompleteProvider { if ev.Source() == llmSetupDraftAppserviceSource { return true } if ev.Source() != llmApplicationOwnerSource { return false } cmd, ok := ev.Data().(llmevents.CompleteProviderCommand) return ok && specificationCompletionKindAllowed(cmd.Request.Kind) && cmd.Request.WorkspaceID != "" && cmd.ExpectedConfigurationRevision != "" } return ev.Source() == llmApplicationOwnerSource }`
  - 当前来源常量：`llmSetupDraftAppserviceSource = "application/workorchd/appservice"`；`llmApplicationOwnerSource = "application/workorchd"`