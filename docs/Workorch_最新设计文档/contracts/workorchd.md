# workorchd：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页51个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

workorchd是Web/CLI协议到typed owner合同的应用协调者，拥有应用任务、projection和少量准入入口，不拥有Workspace/Run/Core状态。接收RunRequested后仍经Workspace/Run正式合同；RuntimeView/CLI查询只投影。HTTP adapter不能绕Biz去repository。

CLI FacadeTopics的现有集合没有WorkspaceCreate，也没有Goal/DoD写命令；其他资产Create和读/运维能力可保留，不做“所有CLI都删除”。Web-only创建限制针对创建入口和NC00链。CLI source未全读到的调用点列为未验证；Facade存在不能充当Web全环证据。projection变化、service stop及owner通知均不触发私有authority写入。

## 逐topic合同

<a id="ct-workorchd-cliapprovalapprove"></a>

### CT-workorchd-CLIApprovalApprove

- 边界：`TopicCLIApprovalApprove` = `/cli/approval/approve`；CF07；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L26)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L289)
- 接收provider：workorchd；Command [CLIApprovalApproveCommand](#schema-cliapprovalapprovecommand) → Result [CLIApprovalApproveResult](#schema-cliapprovalapproveresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIGovernance`（workorchd）
- 接收入口：[handleTypedCLIGovernance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_governance.go#L11)
- 已观察外部引用：process/workorch [Approve](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L656)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-workorchd-cliapprovalbatchresolve"></a>

### CT-workorchd-CLIApprovalBatchResolve

- 边界：`TopicCLIApprovalBatchResolve` = `/cli/approval/batch-resolve`；CF07；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L28)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L291)
- 接收provider：workorchd；Command [CLIApprovalBatchResolveCommand](#schema-cliapprovalbatchresolvecommand) → Result [CLIApprovalBatchResolveResult](#schema-cliapprovalbatchresolveresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIGovernance`（workorchd）
- 接收入口：[handleTypedCLIGovernance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_governance.go#L11)
- 已观察外部引用：process/workorch [BatchResolve](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L688)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-workorchd-cliapprovalget"></a>

### CT-workorchd-CLIApprovalGet

- 边界：`TopicCLIApprovalGet` = `/cli/approval/get`；CF07；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L30)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L293)
- 接收provider：workorchd；Command [CLIApprovalGetCommand](#schema-cliapprovalgetcommand) → Result [CLIApprovalGetResult](#schema-cliapprovalgetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIGovernance`（workorchd）
- 接收入口：[handleTypedCLIGovernance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_governance.go#L11)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L719)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-workorchd-cliapprovallist"></a>

### CT-workorchd-CLIApprovalList

- 边界：`TopicCLIApprovalList` = `/cli/approval/list`；CF07；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L29)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L292)
- 接收provider：workorchd；Command [CLIApprovalListCommand](#schema-cliapprovallistcommand) → Result [CLIApprovalListResult](#schema-cliapprovallistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIGovernance`（workorchd）
- 接收入口：[handleTypedCLIGovernance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_governance.go#L11)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L710)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-workorchd-cliapprovalreject"></a>

### CT-workorchd-CLIApprovalReject

- 边界：`TopicCLIApprovalReject` = `/cli/approval/reject`；CF07；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L27)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L290)
- 接收provider：workorchd；Command [CLIApprovalRejectCommand](#schema-cliapprovalrejectcommand) → Result [CLIApprovalRejectResult](#schema-cliapprovalrejectresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIGovernance`（workorchd）
- 接收入口：[handleTypedCLIGovernance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_governance.go#L11)
- 已观察外部引用：process/workorch [Reject](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L672)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-workorchd-cliconfigurationget"></a>

### CT-workorchd-CLIConfigurationGet

- 边界：`TopicCLIConfigurationGet` = `/cli/configuration/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L55)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L318)
- 接收provider：workorchd；Command [CLIConfigurationGetCommand](#schema-cliconfigurationgetcommand) → Result [CLIConfigurationGetResult](#schema-cliconfigurationgetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIConfiguration`（workorchd）
- 接收入口：[handleTypedCLIConfiguration](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_configuration.go#L9)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L1048)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cliconfigurationrefreshchatmodels"></a>

### CT-workorchd-CLIConfigurationRefreshChatModels

- 边界：`TopicCLIConfigurationRefreshChatModels` = `/cli/configuration/refresh-chat-models`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L56)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L319)
- 接收provider：workorchd；Command [CLIConfigurationRefreshChatModelsCommand](#schema-cliconfigurationrefreshchatmodelscommand) → Result [CLIConfigurationRefreshChatModelsResult](#schema-cliconfigurationrefreshchatmodelsresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIConfiguration`（workorchd）
- 接收入口：[handleTypedCLIConfiguration](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_configuration.go#L9)
- 已观察外部引用：process/workorch [RefreshChatModelTargets](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L1069)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clidecisionbatchresolve"></a>

### CT-workorchd-CLIDecisionBatchResolve

- 边界：`TopicCLIDecisionBatchResolve` = `/cli/decision/batch-resolve`；CF07；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L23)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L286)
- 接收provider：workorchd；Command [CLIDecisionBatchResolveCommand](#schema-clidecisionbatchresolvecommand) → Result [CLIDecisionBatchResolveResult](#schema-clidecisionbatchresolveresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIGovernance`（workorchd）
- 接收入口：[handleTypedCLIGovernance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_governance.go#L11)
- 已观察外部引用：process/workorch [BatchResolve](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L616)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-workorchd-clidecisionget"></a>

### CT-workorchd-CLIDecisionGet

- 边界：`TopicCLIDecisionGet` = `/cli/decision/get`；CF07；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L25)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L288)
- 接收provider：workorchd；Command [CLIDecisionGetCommand](#schema-clidecisiongetcommand) → Result [CLIDecisionGetResult](#schema-clidecisiongetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIGovernance`（workorchd）
- 接收入口：[handleTypedCLIGovernance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_governance.go#L11)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L638)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-workorchd-clidecisionlist"></a>

### CT-workorchd-CLIDecisionList

- 边界：`TopicCLIDecisionList` = `/cli/decision/list`；CF07；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L24)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L287)
- 接收provider：workorchd；Command [CLIDecisionListCommand](#schema-clidecisionlistcommand) → Result [CLIDecisionListResult](#schema-clidecisionlistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIGovernance`（workorchd）
- 接收入口：[handleTypedCLIGovernance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_governance.go#L11)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L629)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-workorchd-clidecisionresolve"></a>

### CT-workorchd-CLIDecisionResolve

- 边界：`TopicCLIDecisionResolve` = `/cli/decision/resolve`；CF07；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L22)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L285)
- 接收provider：workorchd；Command [CLIDecisionResolveCommand](#schema-clidecisionresolvecommand) → Result [CLIDecisionResolveResult](#schema-clidecisionresolveresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIGovernance`（workorchd）
- 接收入口：[handleTypedCLIGovernance](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_governance.go#L11)
- 已观察外部引用：process/workorch [Resolve](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L601)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS04](../Workorch_组件固定合同与分层验收规范.md#cs04)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP05](../Workorch_组件固定合同与分层验收规范.md#cp05)

<a id="ct-workorchd-clidodget"></a>

### CT-workorchd-CLIDoDGet

- 边界：`TopicCLIDoDGet` = `/cli/dod/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L18)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L281)
- 接收provider：workorchd；Command [CLIDoDGetCommand](#schema-clidodgetcommand) → Result [CLIDoDGetResult](#schema-clidodgetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIDefinition`（workorchd）
- 接收入口：[handleTypedCLIDefinition](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_definition.go#L9)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L478)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clidodlist"></a>

### CT-workorchd-CLIDoDList

- 边界：`TopicCLIDoDList` = `/cli/dod/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L17)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L280)
- 接收provider：workorchd；Command [CLIDoDListCommand](#schema-clidodlistcommand) → Result [CLIDoDListResult](#schema-clidodlistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIDefinition`（workorchd）
- 接收入口：[handleTypedCLIDefinition](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_definition.go#L9)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L469)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cligoalget"></a>

### CT-workorchd-CLIGoalGet

- 边界：`TopicCLIGoalGet` = `/cli/goal/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L16)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L279)
- 接收provider：workorchd；Command [CLIGoalGetCommand](#schema-cligoalgetcommand) → Result [CLIGoalGetResult](#schema-cligoalgetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIDefinition`（workorchd）
- 接收入口：[handleTypedCLIDefinition](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_definition.go#L9)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L448)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cligoallist"></a>

### CT-workorchd-CLIGoalList

- 边界：`TopicCLIGoalList` = `/cli/goal/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L15)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L278)
- 接收provider：workorchd；Command [CLIGoalListCommand](#schema-cligoallistcommand) → Result [CLIGoalListResult](#schema-cligoallistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIDefinition`（workorchd）
- 接收入口：[handleTypedCLIDefinition](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_definition.go#L9)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L439)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climcpcapabilitylist"></a>

### CT-workorchd-CLIMCPCapabilityList

- 边界：`TopicCLIMCPCapabilityList` = `/cli/mcp/capability/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L43)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L306)
- 接收provider：workorchd；Command [CLIMCPCapabilityListCommand](#schema-climcpcapabilitylistcommand) → Result [CLIMCPCapabilityListResult](#schema-climcpcapabilitylistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [ListCapabilities](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L892)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climcpservercreate"></a>

### CT-workorchd-CLIMCPServerCreate

- 边界：`TopicCLIMCPServerCreate` = `/cli/mcp/server/create`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L39)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L302)
- 接收provider：workorchd；Command [CLIMCPServerCreateCommand](#schema-climcpservercreatecommand) → Result [CLIMCPServerCreateResult](#schema-climcpservercreateresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [CreateServer](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L841)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climcpserverget"></a>

### CT-workorchd-CLIMCPServerGet

- 边界：`TopicCLIMCPServerGet` = `/cli/mcp/server/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L40)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L303)
- 接收provider：workorchd；Command [CLIMCPServerGetCommand](#schema-climcpservergetcommand) → Result [CLIMCPServerGetResult](#schema-climcpservergetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [GetServer](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L854)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climcpserverlist"></a>

### CT-workorchd-CLIMCPServerList

- 边界：`TopicCLIMCPServerList` = `/cli/mcp/server/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L41)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L304)
- 接收provider：workorchd；Command [CLIMCPServerListCommand](#schema-climcpserverlistcommand) → Result [CLIMCPServerListResult](#schema-climcpserverlistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [ListServers](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L867)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climcpserverupdate"></a>

### CT-workorchd-CLIMCPServerUpdate

- 边界：`TopicCLIMCPServerUpdate` = `/cli/mcp/server/update`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L42)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L305)
- 接收provider：workorchd；Command [CLIMCPServerUpdateCommand](#schema-climcpserverupdatecommand) → Result [CLIMCPServerUpdateResult](#schema-climcpserverupdateresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [UpdateServer](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L879)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climemorycreate"></a>

### CT-workorchd-CLIMemoryCreate

- 边界：`TopicCLIMemoryCreate` = `/cli/memory/create`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L44)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L307)
- 接收provider：workorchd；Command [CLIMemoryCreateCommand](#schema-climemorycreatecommand) → Result [CLIMemoryCreateResult](#schema-climemorycreateresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIMemory`（workorchd）
- 接收入口：[handleTypedCLIMemory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_memory.go#L10)
- 已观察外部引用：process/workorch [Create](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L907)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climemorydelete"></a>

### CT-workorchd-CLIMemoryDelete

- 边界：`TopicCLIMemoryDelete` = `/cli/memory/delete`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L47)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L310)
- 接收provider：workorchd；Command [CLIMemoryDeleteCommand](#schema-climemorydeletecommand) → Result [CLIMemoryDeleteResult](#schema-climemorydeleteresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIMemory`（workorchd）
- 接收入口：[handleTypedCLIMemory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_memory.go#L10)
- 已观察外部引用：process/workorch [Delete](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L943)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climemoryget"></a>

### CT-workorchd-CLIMemoryGet

- 边界：`TopicCLIMemoryGet` = `/cli/memory/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L45)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L308)
- 接收provider：workorchd；Command [CLIMemoryGetCommand](#schema-climemorygetcommand) → Result [CLIMemoryGetResult](#schema-climemorygetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIMemory`（workorchd）
- 接收入口：[handleTypedCLIMemory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_memory.go#L10)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L919)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climemorylist"></a>

### CT-workorchd-CLIMemoryList

- 边界：`TopicCLIMemoryList` = `/cli/memory/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L48)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L311)
- 接收provider：workorchd；Command [CLIMemoryListCommand](#schema-climemorylistcommand) → Result [CLIMemoryListResult](#schema-climemorylistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIMemory`（workorchd）
- 接收入口：[handleTypedCLIMemory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_memory.go#L10)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L952)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climemorysearch"></a>

### CT-workorchd-CLIMemorySearch

- 边界：`TopicCLIMemorySearch` = `/cli/memory/search`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L49)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L312)
- 接收provider：workorchd；Command [CLIMemorySearchCommand](#schema-climemorysearchcommand) → Result [CLIMemorySearchResult](#schema-climemorysearchresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIMemory`（workorchd）
- 接收入口：[handleTypedCLIMemory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_memory.go#L10)
- 已观察外部引用：process/workorch [Search](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L961)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`query.WorkspaceID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-climemoryupdate"></a>

### CT-workorchd-CLIMemoryUpdate

- 边界：`TopicCLIMemoryUpdate` = `/cli/memory/update`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L46)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L309)
- 接收provider：workorchd；Command [CLIMemoryUpdateCommand](#schema-climemoryupdatecommand) → Result [CLIMemoryUpdateResult](#schema-climemoryupdateresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIMemory`（workorchd）
- 接收入口：[handleTypedCLIMemory](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_memory.go#L10)
- 已观察外部引用：process/workorch [Update](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L931)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clirunget"></a>

### CT-workorchd-CLIRunGet

- 边界：`TopicCLIRunGet` = `/cli/run/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L19)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L282)
- 接收provider：workorchd；Command [CLIRunGetCommand](#schema-clirungetcommand) → Result [CLIRunGetResult](#schema-clirungetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIRun`（workorchd）
- 接收入口：[handleTypedCLIRun](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_run.go#L11)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L563)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clirunlist"></a>

### CT-workorchd-CLIRunList

- 边界：`TopicCLIRunList` = `/cli/run/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L20)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L283)
- 接收provider：workorchd；Command [CLIRunListCommand](#schema-clirunlistcommand) → Result [CLIRunListResult](#schema-clirunlistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIRun`（workorchd）
- 接收入口：[handleTypedCLIRun](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_run.go#L11)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L576)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clirunstatus"></a>

### CT-workorchd-CLIRunStatus

- 边界：`TopicCLIRunStatus` = `/cli/run/status`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L21)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L284)
- 接收provider：workorchd；Command [CLIRunStatusCommand](#schema-clirunstatuscommand) → Result [CLIRunStatusResult](#schema-clirunstatusresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIRun`（workorchd）
- 接收入口：[handleTypedCLIRun](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_run.go#L11)
- 已观察外部引用：process/workorch [Status](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L585)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cliskillcreate"></a>

### CT-workorchd-CLISkillCreate

- 边界：`TopicCLISkillCreate` = `/cli/skill/create`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L36)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L299)
- 接收provider：workorchd；Command [CLISkillCreateCommand](#schema-cliskillcreatecommand) → Result [CLISkillCreateResult](#schema-cliskillcreateresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [Create](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L805)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cliskillget"></a>

### CT-workorchd-CLISkillGet

- 边界：`TopicCLISkillGet` = `/cli/skill/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L37)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L300)
- 接收provider：workorchd；Command [CLISkillGetCommand](#schema-cliskillgetcommand) → Result [CLISkillGetResult](#schema-cliskillgetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L817)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cliskilllist"></a>

### CT-workorchd-CLISkillList

- 边界：`TopicCLISkillList` = `/cli/skill/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L38)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L301)
- 接收provider：workorchd；Command [CLISkillListCommand](#schema-cliskilllistcommand) → Result [CLISkillListResult](#schema-cliskilllistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L829)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clisoulcreate"></a>

### CT-workorchd-CLISoulCreate

- 边界：`TopicCLISoulCreate` = `/cli/soul/create`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L31)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L294)
- 接收provider：workorchd；Command [CLISoulCreateCommand](#schema-clisoulcreatecommand) → Result [CLISoulCreateResult](#schema-clisoulcreateresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [Create](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L748)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clisouldelete"></a>

### CT-workorchd-CLISoulDelete

- 边界：`TopicCLISoulDelete` = `/cli/soul/delete`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L35)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L298)
- 接收provider：workorchd；Command [CLISoulDeleteCommand](#schema-clisouldeletecommand) → Result [CLISoulDeleteResult](#schema-clisouldeleteresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [Delete](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L793)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clisoulget"></a>

### CT-workorchd-CLISoulGet

- 边界：`TopicCLISoulGet` = `/cli/soul/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L32)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L295)
- 接收provider：workorchd；Command [CLISoulGetCommand](#schema-clisoulgetcommand) → Result [CLISoulGetResult](#schema-clisoulgetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L760)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clisoullist"></a>

### CT-workorchd-CLISoulList

- 边界：`TopicCLISoulList` = `/cli/soul/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L33)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L296)
- 接收provider：workorchd；Command [CLISoulListCommand](#schema-clisoullistcommand) → Result [CLISoulListResult](#schema-clisoullistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L772)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clisoulupdate"></a>

### CT-workorchd-CLISoulUpdate

- 边界：`TopicCLISoulUpdate` = `/cli/soul/update`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L34)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L297)
- 接收provider：workorchd；Command [CLISoulUpdateCommand](#schema-clisoulupdatecommand) → Result [CLISoulUpdateResult](#schema-clisoulupdateresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIContextRegistry`（workorchd）
- 接收入口：[handleTypedCLIContextRegistry](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_context_registry.go#L10)
- 已观察外部引用：process/workorch [Update](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L781)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clitemplatecreate"></a>

### CT-workorchd-CLITemplateCreate

- 边界：`TopicCLITemplateCreate` = `/cli/template/create`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L50)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L313)
- 接收provider：workorchd；Command [CLITemplateCreateCommand](#schema-clitemplatecreatecommand) → Result [CLITemplateCreateResult](#schema-clitemplatecreateresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLITemplate`（workorchd）
- 接收入口：[handleTypedCLITemplate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_template.go#L10)
- 已观察外部引用：process/workorch [Create](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L977)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clitemplatedelete"></a>

### CT-workorchd-CLITemplateDelete

- 边界：`TopicCLITemplateDelete` = `/cli/template/delete`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L54)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L317)
- 接收provider：workorchd；Command [CLITemplateDeleteCommand](#schema-clitemplatedeletecommand) → Result [CLITemplateDeleteResult](#schema-clitemplatedeleteresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLITemplate`（workorchd）
- 接收入口：[handleTypedCLITemplate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_template.go#L10)
- 已观察外部引用：process/workorch [Delete](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L1036)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clitemplateget"></a>

### CT-workorchd-CLITemplateGet

- 边界：`TopicCLITemplateGet` = `/cli/template/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L51)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L314)
- 接收provider：workorchd；Command [CLITemplateGetCommand](#schema-clitemplategetcommand) → Result [CLITemplateGetResult](#schema-clitemplategetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLITemplate`（workorchd）
- 接收入口：[handleTypedCLITemplate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_template.go#L10)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L994)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clitemplatelist"></a>

### CT-workorchd-CLITemplateList

- 边界：`TopicCLITemplateList` = `/cli/template/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L52)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L315)
- 接收provider：workorchd；Command [CLITemplateListCommand](#schema-clitemplatelistcommand) → Result [CLITemplateListResult](#schema-clitemplatelistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLITemplate`（workorchd）
- 接收入口：[handleTypedCLITemplate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_template.go#L10)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L1007)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-clitemplateupdate"></a>

### CT-workorchd-CLITemplateUpdate

- 边界：`TopicCLITemplateUpdate` = `/cli/template/update`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L53)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L316)
- 接收provider：workorchd；Command [CLITemplateUpdateCommand](#schema-clitemplateupdatecommand) → Result [CLITemplateUpdateResult](#schema-clitemplateupdateresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLITemplate`（workorchd）
- 接收入口：[handleTypedCLITemplate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_template.go#L10)
- 已观察外部引用：process/workorch [Update](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L1019)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cliworkspacedelete"></a>

### CT-workorchd-CLIWorkspaceDelete

- 边界：`TopicCLIWorkspaceDelete` = `/cli/workspace/delete`；CF12；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L13)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L276)
- 接收provider：workorchd；Command [CLIWorkspaceDeleteCommand](#schema-cliworkspacedeletecommand) → Result [CLIWorkspaceDeleteResult](#schema-cliworkspacedeleteresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIWorkspace`（workorchd）
- 接收入口：[handleTypedCLIWorkspace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_workspace.go#L12)
- 已观察外部引用：process/workorch [Delete](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L424)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS06](../Workorch_组件固定合同与分层验收规范.md#cs06)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`command.WorkspaceID == ""`
- 验收适用集：[CP12](../Workorch_组件固定合同与分层验收规范.md#cp12)

<a id="ct-workorchd-cliworkspaceget"></a>

### CT-workorchd-CLIWorkspaceGet

- 边界：`TopicCLIWorkspaceGet` = `/cli/workspace/get`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L273)
- 接收provider：workorchd；Command [CLIWorkspaceGetCommand](#schema-cliworkspacegetcommand) → Result [CLIWorkspaceGetResult](#schema-cliworkspacegetresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIWorkspace`（workorchd）
- 接收入口：[handleTypedCLIWorkspace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_workspace.go#L12)
- 已观察外部引用：process/workorch [Get](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L339)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`command.WorkspaceID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cliworkspacelist"></a>

### CT-workorchd-CLIWorkspaceList

- 边界：`TopicCLIWorkspaceList` = `/cli/workspace/list`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L11)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L274)
- 接收provider：workorchd；Command [CLIWorkspaceListCommand](#schema-cliworkspacelistcommand) → Result [CLIWorkspaceListResult](#schema-cliworkspacelistresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIWorkspace`（workorchd）
- 接收入口：[handleTypedCLIWorkspace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_workspace.go#L12)
- 已观察外部引用：process/workorch [List](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L352)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cliworkspacesnapshot"></a>

### CT-workorchd-CLIWorkspaceSnapshot

- 边界：`TopicCLIWorkspaceSnapshot` = `/cli/workspace/snapshot`；CF11；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L12)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L275)
- 接收provider：workorchd；Command [CLIWorkspaceSnapshotCommand](#schema-cliworkspacesnapshotcommand) → Result [CLIWorkspaceSnapshotResult](#schema-cliworkspacesnapshotresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIWorkspace`（workorchd）
- 接收入口：[handleTypedCLIWorkspace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_workspace.go#L12)
- 已观察外部引用：process/workorch [Snapshot](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L372)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`command.WorkspaceID == "" || d.viewReader == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-cliworkspaceupdatellmselection"></a>

### CT-workorchd-CLIWorkspaceUpdateLLMSelection

- 边界：`TopicCLIWorkspaceUpdateLLMSelection` = `/cli/workspace/update-llm-selection`；CF03；source existing / target retain；FacadeTopic / Facade/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L14)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L277)
- 接收provider：workorchd；Command [CLIWorkspaceUpdateLLMSelectionCommand](#schema-cliworkspaceupdatellmselectioncommand) → Result [CLIWorkspaceUpdateLLMSelectionResult](#schema-cliworkspaceupdatellmselectionresult)；target合法caller：process/workorch的CLI backend（wire Source=workorch-cli，排除Workspace创建）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedCLIWorkspace`（workorchd）
- 接收入口：[handleTypedCLIWorkspace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_workspace.go#L12)
- 已观察外部引用：process/workorch [UpdateLLMSelection](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L410)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS08](../Workorch_组件固定合同与分层验收规范.md#cs08)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`command.WorkspaceID == ""`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-workorchdcommandreadworkspaceruntimeview"></a>

### CT-workorchd-WorkorchDCommandReadWorkspaceRuntimeView

- 边界：`TopicWorkorchDCommandReadWorkspaceRuntimeView` = `/workorchd/command/workspace-runtime-view/read`；CF11；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L84)
- 接收provider：workorchd；Command [ReadWorkspaceRuntimeViewCommand](#schema-readworkspaceruntimeviewcommand) → Result [ReadWorkspaceRuntimeViewResult](#schema-readworkspaceruntimeviewresult)；target合法caller：workorchd公开Web/execution gateway、process/workorch现有typed查询/执行backend（不授创建或绕过Workspace/Run准入）
- 实际注册：[runTypedCLI](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_dispatcher.go#L10) → `handleTypedWorkspaceRuntimeView`（workorchd）
- 接收入口：[handleTypedWorkspaceRuntimeView](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/cli_typed_workspace.go#L93)
- 已观察外部引用：process/workorch [RuntimeView](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L386)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`query.WorkspaceID == "" || d.viewReader == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-workorchd-workorchdcommandworkspacerunrequested"></a>

### CT-workorchd-WorkorchDCommandWorkspaceRunRequested

- 边界：`TopicWorkorchDCommandWorkspaceRunRequested` = `/workorchd/command/workspace-run/requested`；CF02；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L83)
- 接收provider：workorchd；Command [WorkspaceRunRequestedCommand](#schema-workspacerunrequestedcommand) → Result [WorkspaceRunRequestedResult](#schema-workspacerunrequestedresult)；target合法caller：workorchd公开Web/execution gateway、process/workorch现有typed查询/执行backend（不授创建或绕过Workspace/Run准入）
- 实际注册：[Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53) → `handleWorkspaceRunRequested`（workorchd）
- 接收入口：[handleWorkspaceRunRequested](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L392)
- owner内构造/发送静态引用：[AdmitWorkspaceExecution](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/execution_admission_gateway.go#L35)
- 已观察外部引用：process/workorch [Command](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L496)
- Source静态证据边界：本轮未定位本topic专属Source拒绝；不把router其他family的guard移植到此topic，target allowlist仍须双端准入验收
- 特化承诺：[CS10](../Workorch_组件固定合同与分层验收规范.md#cs10)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`command.WorkspaceID == ""`；`command.Intent != "resume" && command.Intent != "restart"`；`strings.TrimSpace(command.RequestID) == ""`；`err := outcome.Validate(); err != nil`；`outcome.WorkspaceID != command.WorkspaceID || strings.TrimSpace(outcome.RequestID) != strings.TrimSpace(command.RequestID) || string(outcome.Kind) != command.Intent`
- 验收适用集：[CP06](../Workorch_组件固定合同与分层验收规范.md#cp06)

<a id="ct-workorchd-workorchdservicestopped"></a>

### CT-workorchd-WorkorchDServiceStopped

- 边界：`TopicWorkorchDServiceStopped` = `/workorchd/service/stopped`；CF00；source notification / target retain；DataTopic / Notify/Topic
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L82)
- 唯一事实owner/发布者：workorchd；Data [ServiceStoppedData](#schema-servicestoppeddata)；同步Command/Result：不适用；允许订阅者：process/workorch、workorchd
- 已读事实来源引用（含发布/分派路径）：[publishServiceStopped](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L123)
- 已观察外部引用：process/workorch [subscribeServerStopped](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/runtime.go#L58)；process/workorch [Shutdown](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/runtime.go#L77)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-workorchd-workspaceprojectionchanged"></a>

### CT-workorchd-WorkspaceProjectionChanged

- 边界：`TopicWorkspaceProjectionChanged` = `/workorchd/workspace/projection/changed`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L85)
- 唯一事实owner/发布者：workorchd；Data [WorkspaceProjectionChangedData](#schema-workspaceprojectionchangeddata)；同步Command/Result：不适用；允许订阅者：process/workorch、workorchd
- 实际注册：[Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53) → `handleWorkspaceProjectionChanged`（workorchd）
- 已读事实来源引用（含发布/分派路径）：[Setup](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L53)；[publishWorkspaceProjectionChanged](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L372)
- 已观察外部引用：process/workorch [SubscribeWorkspaceProjection](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L158)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 本topic入口/具体类型分支的Source审查索引：[workspaceProjectionChangedFromOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L291)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`event == nil || event.Source() != ModuleID`
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-cliapprovalapprovecommand"></a>

### CLIApprovalApproveCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:126](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L126)
- 字段：`RequestID string`；`WorkspaceID common.WorkspaceID`；`ApprovalID common.ApprovalID`；`Payload workorcheventpayload.Payload`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-cliapprovalapproveresult"></a>

### CLIApprovalApproveResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:132](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L132)
- 字段：`Approval projectioncontract.Approval`
- 字段引用类型：[Approval（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L66)

<a id="schema-cliapprovalbatchresolvecommand"></a>

### CLIApprovalBatchResolveCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:140](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L140)
- 字段：`RequestID string`；`WorkspaceID common.WorkspaceID`；`ApprovalIDs []common.ApprovalID`；`Action string`；`Payload workorcheventpayload.Payload`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-cliapprovalbatchresolveresult"></a>

### CLIApprovalBatchResolveResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:147](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L147)
- 字段：`Approvals []projectioncontract.Approval`
- 字段引用类型：[Approval（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L66)

<a id="schema-cliapprovalgetcommand"></a>

### CLIApprovalGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:150](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L150)
- 字段：`WorkspaceID common.WorkspaceID`；`ApprovalID common.ApprovalID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)

<a id="schema-cliapprovalgetresult"></a>

### CLIApprovalGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:154](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L154)
- 字段：`Approval projectioncontract.Approval`
- 字段引用类型：[Approval（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L66)

<a id="schema-cliapprovallistcommand"></a>

### CLIApprovalListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:148](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L148)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-cliapprovallistresult"></a>

### CLIApprovalListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:149](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L149)
- 字段：`Approvals []projectioncontract.Approval`
- 字段引用类型：[Approval（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L66)

<a id="schema-cliapprovalrejectcommand"></a>

### CLIApprovalRejectCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:133](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L133)
- 字段：`RequestID string`；`WorkspaceID common.WorkspaceID`；`ApprovalID common.ApprovalID`；`Payload workorcheventpayload.Payload`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-cliapprovalrejectresult"></a>

### CLIApprovalRejectResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:139](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L139)
- 字段：`Approval projectioncontract.Approval`
- 字段引用类型：[Approval（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L66)

<a id="schema-cliartifactcontinuationinfo"></a>

### CLIArtifactContinuationInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_artifact_dto.go:30](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_artifact_dto.go#L30)
- 字段：`ID common.ArtifactContinuationID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`RootStepID common.StepID`；`ArtifactID string`；`TargetRef string`；`Operation string`；`State common.ArtifactContinuationState`；`ExpectedSequence int`；`ReceivedSequences []int`；`FinalReceived bool`；`BytesWritten int64`；`RollingHash string`；`LastChunkHash string`；`CoverageTargets []string`；`EvidenceRefs []string`；`FailureReason string`；`CreatedAt string`；`UpdatedAt string`；`Progress workorcheventpayload.Payload`
- 字段引用类型：[ArtifactContinuationID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L17)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[ArtifactContinuationState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L21)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-cliartifactinfo"></a>

### CLIArtifactInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_artifact_dto.go:8](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_artifact_dto.go#L8)
- 字段：`ID common.ArtifactID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`ResultID common.ResultID`；`StepID common.StepID`；`ToolCallID common.ToolCallID`；`PatchID common.PatchID`；`ValidationResultID common.ValidationResultID`；`ArtifactType string`；`Category string`；`Name string`；`Summary string`；`URI string`；`ContentType string`；`State common.ArtifactState`；`EvidenceRefs []string`；`Metadata workorcheventpayload.Payload`；`CreatedAt string`；`UpdatedAt string`
- 字段引用类型：[ArtifactID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L16)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L15)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[ToolCallID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L12)；[PatchID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L13)；[ValidationResultID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L14)；[ArtifactState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L20)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-cliconfigurationgetcommand"></a>

### CLIConfigurationGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:266](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L266)
- 声明：`type CLIConfigurationGetCommand struct{}`

<a id="schema-cliconfigurationgetresult"></a>

### CLIConfigurationGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:267](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L267)
- 字段：`Configuration workorchport.ServiceConfig`
- 字段引用类型：[ServiceConfig（shared/workorchport声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchport/config_events.go#L22)

<a id="schema-cliconfigurationrefreshchatmodelscommand"></a>

### CLIConfigurationRefreshChatModelsCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:268](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L268)
- 声明：`type CLIConfigurationRefreshChatModelsCommand struct{}`

<a id="schema-cliconfigurationrefreshchatmodelsresult"></a>

### CLIConfigurationRefreshChatModelsResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:269](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L269)
- 字段：`Configuration workorchport.ServiceConfig`
- 字段引用类型：[ServiceConfig（shared/workorchport声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchport/config_events.go#L22)

<a id="schema-clidecisionbatchresolvecommand"></a>

### CLIDecisionBatchResolveCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:110](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L110)
- 字段：`RequestID string`；`WorkspaceID common.WorkspaceID`；`TicketIDs []common.TicketID`；`ResolutionType string`；`Payload workorcheventpayload.Payload`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-clidecisionbatchresolveresult"></a>

### CLIDecisionBatchResolveResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:117](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L117)
- 字段：`Decisions []projectioncontract.Decision`
- 字段引用类型：[Decision（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L6)

<a id="schema-clidecisiongetcommand"></a>

### CLIDecisionGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:120](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L120)
- 字段：`WorkspaceID common.WorkspaceID`；`TicketID common.TicketID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[TicketID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L8)

<a id="schema-clidecisiongetresult"></a>

### CLIDecisionGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:124](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L124)
- 字段：`Decision projectioncontract.Decision`
- 字段引用类型：[Decision（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L6)

<a id="schema-clidecisionlistcommand"></a>

### CLIDecisionListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:118](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L118)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-clidecisionlistresult"></a>

### CLIDecisionListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:119](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L119)
- 字段：`Decisions []projectioncontract.Decision`
- 字段引用类型：[Decision（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L6)

<a id="schema-clidecisionresolvecommand"></a>

### CLIDecisionResolveCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:105](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L105)
- 字段：`RequestID string`；`Request workorchport.ResolveDecisionRequest`
- 字段引用类型：[ResolveDecisionRequest（shared/workorchport声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchport/governance.go#L18)

<a id="schema-clidecisionresolveresult"></a>

### CLIDecisionResolveResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:109](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L109)
- 字段：`Decision projectioncontract.Decision`
- 字段引用类型：[Decision（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L6)

<a id="schema-clidodgetcommand"></a>

### CLIDoDGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:76](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L76)
- 字段：`WorkspaceID common.WorkspaceID`；`DoDID common.DoDID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[DoDID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L5)

<a id="schema-clidodgetresult"></a>

### CLIDoDGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:80](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L80)
- 字段：`DoD workorchmanagementcontract.DoD`
- 字段引用类型：[DoD（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L37)

<a id="schema-clidodlistcommand"></a>

### CLIDoDListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:72](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L72)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-clidodlistresult"></a>

### CLIDoDListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:73](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L73)
- 字段：`DoDs []workorchmanagementcontract.DoD`
- 字段引用类型：[DoD（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L37)

<a id="schema-cligapbaselineinfo"></a>

### CLIGapBaselineInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_gap_dto.go:29](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_gap_dto.go#L29)
- 字段：`WorkspaceID string`；`RunID string`；`DefinitionRevision string`；`BaselineRevision string`；`CurrentFocus workorcheventpayload.Payload`；`OpenCoverageWorkItems []workorcheventpayload.Payload`；`AcceptedEvidenceRefs []string`；`RejectedEvidenceRefs []string`；`TodoItems []CLIGapTodoItemInfo`；`Diagnostics []projectioncontract.GapDiagnostic`
- 字段引用类型：[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[CLIGapTodoItemInfo](#schema-cligaptodoiteminfo)；[GapDiagnostic（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/gap.go#L32)

<a id="schema-cligaptodoiteminfo"></a>

### CLIGapTodoItemInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_gap_dto.go:17](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_gap_dto.go#L17)
- 字段：`ID string`；`Source string`；`SourceRef string`；`CoverageWorkItemIDs []string`；`RequiredEvidenceKinds []string`；`Status string`；`Priority int`；`Reason string`；`Metadata workorcheventpayload.Payload`
- 字段引用类型：[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-cligoalgetcommand"></a>

### CLIGoalGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:49](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L49)
- 字段：`WorkspaceID common.WorkspaceID`；`GoalID common.GoalID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[GoalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L4)

<a id="schema-cligoalgetresult"></a>

### CLIGoalGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:53](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L53)
- 字段：`Goal workorchmanagementcontract.Goal`
- 字段引用类型：[Goal（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L27)

<a id="schema-cligoallistcommand"></a>

### CLIGoalListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:45](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L45)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-cligoallistresult"></a>

### CLIGoalListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:46](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L46)
- 字段：`Goals []workorchmanagementcontract.Goal`
- 字段引用类型：[Goal（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L27)

<a id="schema-climcpcapability"></a>

### CLIMCPCapability

- 源码：[internal/modules/application/workorchd/pkg/events/cli_mcp_dto.go:29](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_mcp_dto.go#L29)
- 字段：`ID string`；`ServerID string`；`Kind string`；`Name string`；`DisplayName string`；`Description string`；`Category string`；`Tags []string`；`UsageHint string`；`InputSummary string`；`OutputSummary string`；`StageHints []string`；`RiskLevel string`；`EffectScope string`；`IdempotencyClass string`；`ValidationProofApplicability string`；`Schema workorcheventpayload.Payload`；`URI string`；`Enabled bool`；`ApprovalPolicy string`；`Source string`；`SourceRef string`；`Revision uint64`
- 字段引用类型：[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-climcpcapabilitylistcommand"></a>

### CLIMCPCapabilityListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:209](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L209)
- 声明：`type CLIMCPCapabilityListCommand struct{}`

<a id="schema-climcpcapabilitylistresult"></a>

### CLIMCPCapabilityListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:210](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L210)
- 字段：`Capabilities []CLIMCPCapability`
- 字段引用类型：[CLIMCPCapability](#schema-climcpcapability)

<a id="schema-climcpserver"></a>

### CLIMCPServer

- 源码：[internal/modules/application/workorchd/pkg/events/cli_mcp_dto.go:27](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_mcp_dto.go#L27)
- 声明：`type CLIMCPServer CLIMCPServerMutation`
- 字段引用类型：[CLIMCPServerMutation](#schema-climcpservermutation)

<a id="schema-climcpservercreatecommand"></a>

### CLIMCPServerCreateCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:198](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L198)
- 字段：`Server CLIMCPServerMutation`
- 字段引用类型：[CLIMCPServerMutation](#schema-climcpservermutation)

<a id="schema-climcpservercreateresult"></a>

### CLIMCPServerCreateResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:199](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L199)
- 字段：`Server CLIMCPServer`
- 字段引用类型：[CLIMCPServer](#schema-climcpserver)

<a id="schema-climcpservergetcommand"></a>

### CLIMCPServerGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:200](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L200)
- 字段：`ServerID common.MCPServerID`
- 字段引用类型：[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)

<a id="schema-climcpservergetresult"></a>

### CLIMCPServerGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:201](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L201)
- 字段：`Server CLIMCPServer`
- 字段引用类型：[CLIMCPServer](#schema-climcpserver)

<a id="schema-climcpserverlistcommand"></a>

### CLIMCPServerListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:202](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L202)
- 声明：`type CLIMCPServerListCommand struct{}`

<a id="schema-climcpserverlistresult"></a>

### CLIMCPServerListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:203](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L203)
- 字段：`Servers []CLIMCPServer`
- 字段引用类型：[CLIMCPServer](#schema-climcpserver)

<a id="schema-climcpservermutation"></a>

### CLIMCPServerMutation

- 源码：[internal/modules/application/workorchd/pkg/events/cli_mcp_dto.go:10](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_mcp_dto.go#L10)
- 字段：`ID string`；`Name string`；`Transport string`；`Endpoint string`；`Command string`；`Args []string`；`EnvironmentRefs []CLIStringEntry`；`Enabled bool`；`AllowedTools []string`；`AllowedResources []string`；`AllowedPrompts []string`；`DefaultApprovalPolicy string`；`SessionPolicy string`；`TimeoutMillis int`
- 字段引用类型：[CLIStringEntry](#schema-clistringentry)

<a id="schema-climcpserverupdatecommand"></a>

### CLIMCPServerUpdateCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:204](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L204)
- 字段：`ServerID common.MCPServerID`；`Server CLIMCPServerMutation`
- 字段引用类型：[MCPServerID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L26)；[CLIMCPServerMutation](#schema-climcpservermutation)

<a id="schema-climcpserverupdateresult"></a>

### CLIMCPServerUpdateResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:208](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L208)
- 字段：`Server CLIMCPServer`
- 字段引用类型：[CLIMCPServer](#schema-climcpserver)

<a id="schema-climemorycreatecommand"></a>

### CLIMemoryCreateCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:214](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L214)
- 字段：`WorkspaceID common.WorkspaceID`；`Memory workorchmanagementcontract.MemoryMutation`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryMutation（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L296)

<a id="schema-climemorycreateresult"></a>

### CLIMemoryCreateResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:218](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L218)
- 字段：`Memory workorchmanagementcontract.MemoryItem`
- 字段引用类型：[MemoryItem（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L275)

<a id="schema-climemorydeletecommand"></a>

### CLIMemoryDeleteCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:236](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L236)
- 字段：`WorkspaceID common.WorkspaceID`；`MemoryID common.MemoryID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)

<a id="schema-climemorydeleteresult"></a>

### CLIMemoryDeleteResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:240](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L240)
- 字段：`Deleted bool`

<a id="schema-climemorygetcommand"></a>

### CLIMemoryGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:221](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L221)
- 字段：`WorkspaceID common.WorkspaceID`；`MemoryID common.MemoryID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)

<a id="schema-climemorygetresult"></a>

### CLIMemoryGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:225](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L225)
- 字段：`Memory workorchmanagementcontract.MemoryItem`
- 字段引用类型：[MemoryItem（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L275)

<a id="schema-climemorylistcommand"></a>

### CLIMemoryListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:241](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L241)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-climemorylistresult"></a>

### CLIMemoryListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:242](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L242)
- 字段：`Memories []workorchmanagementcontract.MemoryItem`
- 字段引用类型：[MemoryItem（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L275)

<a id="schema-climemoryrecalltraceinfo"></a>

### CLIMemoryRecallTraceInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go:115](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go#L115)
- 字段：`ID common.MemoryRecallTraceID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`GraphNodeID string`；`Stage string`；`Source string`；`Policy string`；`Status string`；`ProviderID string`；`ModelID string`；`Operation string`；`QueryTextPreview string`；`QueryTextSha256 string`；`MinScore float64`；`Limit int`；`EmbeddingModel string`；`VectorDimension int`；`RefreshSource string`；`SelectionPolicyVersion string`；`CandidateMemoryIDs []common.MemoryID`；`SelectedMemoryIDs []common.MemoryID`；`ExcludedMemoryIDs []common.MemoryID`；`TypeQuota []workorchmemorycontract.TypeQuotaEntry`；`SelectionScores []workorchmemorycontract.RecallSelectionScore`；`RetrievedMemoryIDs []common.MemoryID`；`RetrievedMemoryHits []workorchmemorycontract.RecallHit`；`ContextBudget workorchmemorycontract.RecallContextBudget`；`SemanticRecall *workorchmemorycontract.RecallSelectionEvidence`；`FallbackFrom string`；`Reason string`；`OccurredAt string`；`EvidenceRef string`
- 字段引用类型：[MemoryRecallTraceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L23)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)；[TypeQuotaEntry（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall.go#L24)；[RecallSelectionScore（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall.go#L17)；[RecallHit（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall.go#L9)；[RecallContextBudget（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall_selection.go#L20)；[RecallSelectionEvidence（shared/workorchmemorycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmemorycontract/recall_selection.go#L41)

<a id="schema-climemorysearchcommand"></a>

### CLIMemorySearchCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:245](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L245)
- 字段：`Query workorchquerycontract.MemorySearchQuery`
- 字段引用类型：[MemorySearchQuery（shared/workorchquerycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchquerycontract/query.go#L36)

<a id="schema-climemorysearchresult"></a>

### CLIMemorySearchResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:248](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L248)
- 字段：`Memories []workorchmanagementcontract.MemoryItem`
- 字段引用类型：[MemoryItem（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L275)

<a id="schema-climemoryupdatecommand"></a>

### CLIMemoryUpdateCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:228](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L228)
- 字段：`WorkspaceID common.WorkspaceID`；`MemoryID common.MemoryID`；`Memory workorchmanagementcontract.MemoryMutation`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[MemoryID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L22)；[MemoryMutation（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L296)

<a id="schema-climemoryupdateresult"></a>

### CLIMemoryUpdateResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:233](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L233)
- 字段：`Memory workorchmanagementcontract.MemoryItem`
- 字段引用类型：[MemoryItem（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L275)

<a id="schema-clipatchinfo"></a>

### CLIPatchInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_artifact_dto.go:53](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_artifact_dto.go#L53)
- 字段：`ID common.PatchID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`ToolCallID common.ToolCallID`；`ApprovalID common.ApprovalID`；`RevertApprovalID common.ApprovalID`；`PatchType string`；`TargetRef string`；`Summary string`；`State common.PatchState`；`AddedLines int`；`DeletedLines int`；`BeforeHash string`；`AfterHash string`；`AffectedFiles []string`；`RiskLevel string`；`RiskReasons []string`；`Diagnostics []workorcheventpayload.Payload`；`CreatedAt string`；`AppliedAt string`；`RevertedAt string`
- 字段引用类型：[PatchID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L13)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[ToolCallID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L12)；[ApprovalID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L10)；[PatchState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L15)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-clirepairloopinfo"></a>

### CLIRepairLoopInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_progress_dto.go:10](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_progress_dto.go#L10)
- 字段：`State string`；`AttemptCount int`；`LastAttempt int`；`LastRepairStepID string`；`LastFailureStepID string`；`LastFailureClass string`；`LastFailureSignature string`；`LastFailureTargetScope string`；`LastFailureAt string`；`StructuredFailureSignals workorcheventpayload.Payload`；`RepairInputCompaction workorcheventpayload.Payload`；`LastRepairInputBytes int`；`RepeatedFailureReason string`；`MissingDoDItems []workorcheventpayload.Payload`；`RecentAttempts []workorcheventpayload.Payload`；`Escalated bool`；`EscalationReason string`；`EscalationTicketID string`；`EvidenceRefs []string`
- 字段引用类型：[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-clirungetcommand"></a>

### CLIRunGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:92](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L92)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-clirungetresult"></a>

### CLIRunGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:96](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L96)
- 字段：`Run projectioncontract.Run`
- 字段引用类型：[Run（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/resource.go#L104)

<a id="schema-clirunlistcommand"></a>

### CLIRunListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:97](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L97)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-clirunlistresult"></a>

### CLIRunListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:98](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L98)
- 字段：`Runs []projectioncontract.Run`
- 字段引用类型：[Run（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/resource.go#L104)

<a id="schema-clirunstatuscommand"></a>

### CLIRunStatusCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:99](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L99)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)

<a id="schema-clirunstatusresult"></a>

### CLIRunStatusResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:103](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L103)
- 字段：`Status projectioncontract.RunStatus`
- 字段引用类型：[RunStatus（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/run.go#L13)

<a id="schema-cliskillcreatecommand"></a>

### CLISkillCreateCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:180](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L180)
- 字段：`Skill workorchmanagementcontract.SkillMutation`
- 字段引用类型：[SkillMutation（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L163)

<a id="schema-cliskillcreateresult"></a>

### CLISkillCreateResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:183](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L183)
- 字段：`Skill workorchmanagementcontract.Skill`
- 字段引用类型：[Skill（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L129)

<a id="schema-cliskillgetcommand"></a>

### CLISkillGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:186](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L186)
- 字段：`WorkspaceID common.WorkspaceID`；`SkillID common.SkillID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[SkillID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L25)

<a id="schema-cliskillgetresult"></a>

### CLISkillGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:190](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L190)
- 字段：`Skill workorchmanagementcontract.Skill`
- 字段引用类型：[Skill（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L129)

<a id="schema-cliskilllistcommand"></a>

### CLISkillListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:193](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L193)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-cliskilllistresult"></a>

### CLISkillListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:194](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L194)
- 字段：`Skills []workorchmanagementcontract.Skill`
- 字段引用类型：[Skill（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L129)

<a id="schema-clisoulcreatecommand"></a>

### CLISoulCreateCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:156](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L156)
- 字段：`Soul workorchmanagementcontract.SoulMutation`
- 字段引用类型：[SoulMutation（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L116)

<a id="schema-clisoulcreateresult"></a>

### CLISoulCreateResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:159](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L159)
- 字段：`Soul workorchmanagementcontract.Soul`
- 字段引用类型：[Soul（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L98)

<a id="schema-clisouldeletecommand"></a>

### CLISoulDeleteCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:177](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L177)
- 字段：`SoulID common.SoulID`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-clisouldeleteresult"></a>

### CLISoulDeleteResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:178](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L178)
- 字段：`Deleted bool`

<a id="schema-clisoulgetcommand"></a>

### CLISoulGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:162](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L162)
- 字段：`SoulID common.SoulID`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-clisoulgetresult"></a>

### CLISoulGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:163](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L163)
- 字段：`Soul workorchmanagementcontract.Soul`
- 字段引用类型：[Soul（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L98)

<a id="schema-clisoullistcommand"></a>

### CLISoulListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:166](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L166)
- 声明：`type CLISoulListCommand struct{}`

<a id="schema-clisoullistresult"></a>

### CLISoulListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:167](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L167)
- 字段：`Souls []workorchmanagementcontract.Soul`
- 字段引用类型：[Soul（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L98)

<a id="schema-clisoulupdatecommand"></a>

### CLISoulUpdateCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:170](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L170)
- 字段：`SoulID common.SoulID`；`Soul workorchmanagementcontract.SoulMutation`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)；[SoulMutation（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L116)

<a id="schema-clisoulupdateresult"></a>

### CLISoulUpdateResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:174](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L174)
- 字段：`Soul workorchmanagementcontract.Soul`
- 字段引用类型：[Soul（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L98)

<a id="schema-clistepviewinfo"></a>

### CLIStepViewInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_gap_dto.go:8](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_gap_dto.go#L8)
- 字段：`ID string`；`Kind string`；`State string`；`Input workorcheventpayload.Payload`；`Output workorcheventpayload.Payload`；`ContractAudit workorcheventpayload.Payload`
- 字段引用类型：[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-clistringentry"></a>

### CLIStringEntry

- 源码：[internal/modules/application/workorchd/pkg/events/cli_mcp_dto.go:5](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_mcp_dto.go#L5)
- 字段：`Key string`；`Value string`

<a id="schema-clitaskdecompositioninfo"></a>

### CLITaskDecompositionInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_task_dto.go:28](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_task_dto.go#L28)
- 字段：`ID string`；`WorkspaceID string`；`RunID string`；`RootStepID string`；`State string`；`Summary string`；`GraphBranch string`；`NodeTotal int`；`NodeCompleted int`；`NodeFailed int`；`NodeRunning int`；`Nodes []CLITaskDecompositionNodeInfo`；`EvidenceRefs []string`
- 字段引用类型：[CLITaskDecompositionNodeInfo](#schema-clitaskdecompositionnodeinfo)

<a id="schema-clitaskdecompositionnodeinfo"></a>

### CLITaskDecompositionNodeInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_task_dto.go:8](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_task_dto.go#L8)
- 字段：`ID string`；`Kind string`；`Title string`；`State string`；`DependsOn []string`；`Branch string`；`StepID string`；`StepState string`；`RetryOf string`；`RetryAttempt int`；`FailureClass string`；`FailureSignature string`；`TargetScope string`；`BlockingTicketID string`；`EvidenceRefs []string`；`CoverageTargets []string`；`Input workorcheventpayload.Payload`
- 字段引用类型：[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-clitaskplaninfo"></a>

### CLITaskPlanInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_task_dto.go:44](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_task_dto.go#L44)
- 字段：`ID string`；`WorkspaceID string`；`RunID string`；`AnchorStepID string`；`RevisionStepID string`；`Revision int`；`State string`；`Stage string`；`SourceOutputState string`；`Summary string`；`GraphBranch string`；`NodeTotal int`；`NodeCompleted int`；`NodeFailed int`；`NodeRunning int`；`Nodes []CLITaskDecompositionNodeInfo`；`RetryOf string`；`RetryAttempt int`；`FailureClass string`；`FailureSignature string`；`TargetScope string`；`BlockingTicketID string`；`EvidenceRefs []string`；`OccurredAt string`；`SourceProjection string`；`SealedLLMSelection *runtimecontract.ProviderInferenceSelection`
- 字段引用类型：[CLITaskDecompositionNodeInfo](#schema-clitaskdecompositionnodeinfo)；[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)

<a id="schema-clitaskprogressinfo"></a>

### CLITaskProgressInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_progress_dto.go:32](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_progress_dto.go#L32)
- 字段：`WorkspaceID string`；`RunID string`；`State string`；`Summary string`；`CurrentStepID string`；`CurrentStepKind string`；`CurrentStepState string`；`DiagnosticLineage string`；`CurrentMilestone string`；`BlockingTicketID string`；`BlockingApprovalID string`；`Milestones []projectioncontract.ProgressMilestone`；`Items []projectioncontract.ProgressItem`；`Blockers []projectioncontract.ProgressBlocker`；`Recovery projectioncontract.RecoveryPlan`；`RepairLoop CLIRepairLoopInfo`；`AssessmentOutcome projectioncontract.AssessmentOutcome`；`EvidenceRefs []string`；`Runtime *projectioncontract.RuntimeProgress`
- 字段引用类型：[ProgressMilestone（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/progress.go#L56)；[ProgressItem（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/task_progress.go#L25)；[ProgressBlocker（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/task_progress.go#L33)；[RecoveryPlan（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/progress.go#L19)；[CLIRepairLoopInfo](#schema-clirepairloopinfo)；[AssessmentOutcome（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/progress.go#L43)；[RuntimeProgress（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/runtime_progress.go#L7)

<a id="schema-clitemplatecreatecommand"></a>

### CLITemplateCreateCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:252](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L252)
- 字段：`Template CLITemplateInfo`
- 字段引用类型：[CLITemplateInfo](#schema-clitemplateinfo)

<a id="schema-clitemplatecreateresult"></a>

### CLITemplateCreateResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:253](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L253)
- 字段：`Template CLITemplateInfo`
- 字段引用类型：[CLITemplateInfo](#schema-clitemplateinfo)

<a id="schema-clitemplatedeletecommand"></a>

### CLITemplateDeleteCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:263](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L263)
- 字段：`TemplateID string`

<a id="schema-clitemplatedeleteresult"></a>

### CLITemplateDeleteResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:264](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L264)
- 字段：`Deleted bool`

<a id="schema-clitemplategetcommand"></a>

### CLITemplateGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:254](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L254)
- 字段：`TemplateID string`

<a id="schema-clitemplategetresult"></a>

### CLITemplateGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:255](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L255)
- 字段：`Template CLITemplateInfo`
- 字段引用类型：[CLITemplateInfo](#schema-clitemplateinfo)

<a id="schema-clitemplateinfo"></a>

### CLITemplateInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go:151](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go#L151)
- 字段：`ID string`；`Name string`；`Description string`；`GoalText string`；`DoDItems []string`；`DefaultSoulID common.SoulID`；`SoulBindings []soulbinding.Binding`；`ContextHints workorcheventpayload.Payload`；`EvidencePolicies []workorcheventpayload.Payload`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)；[Binding（shared/soulbinding声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/soulbinding/binding.go#L13)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)

<a id="schema-clitemplatelistcommand"></a>

### CLITemplateListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:256](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L256)
- 声明：`type CLITemplateListCommand struct{}`

<a id="schema-clitemplatelistresult"></a>

### CLITemplateListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:257](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L257)
- 字段：`Templates []CLITemplateInfo`
- 字段引用类型：[CLITemplateInfo](#schema-clitemplateinfo)

<a id="schema-clitemplateupdatecommand"></a>

### CLITemplateUpdateCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:258](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L258)
- 字段：`TemplateID string`；`Template CLITemplateInfo`
- 字段引用类型：[CLITemplateInfo](#schema-clitemplateinfo)

<a id="schema-clitemplateupdateresult"></a>

### CLITemplateUpdateResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:262](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L262)
- 字段：`Template CLITemplateInfo`
- 字段引用类型：[CLITemplateInfo](#schema-clitemplateinfo)

<a id="schema-cliworkspacedeletecommand"></a>

### CLIWorkspaceDeleteCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:26](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L26)
- 字段：`WorkspaceID common.WorkspaceID`；`Purge bool`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-cliworkspacedeleteresult"></a>

### CLIWorkspaceDeleteResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:30](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L30)
- 字段：`Deleted bool`

<a id="schema-cliworkspacegetcommand"></a>

### CLIWorkspaceGetCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:18](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L18)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-cliworkspacegetresult"></a>

### CLIWorkspaceGetResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:19](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L19)
- 字段：`Workspace CLIWorkspaceInfo`
- 字段引用类型：[CLIWorkspaceInfo](#schema-cliworkspaceinfo)

<a id="schema-cliworkspaceinfo"></a>

### CLIWorkspaceInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go:33](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go#L33)
- 字段：`Revision uint64`；`ID common.WorkspaceID`；`Title string`；`Description string`；`DeliverableDirectory string`；`State common.WorkspaceState`；`ActiveRunID common.RunID`；`TemplateID string`；`ProfileID string`；`DefaultLLMSelection *runtimecontract.ProviderInferenceSelection`；`DefaultSoulID common.SoulID`；`SoulBindings []soulbinding.Binding`；`ContextHints workorcheventpayload.Payload`；`EvidencePolicies []workorcheventpayload.Payload`；`Projection projectioncontract.ProjectionStatus`；`ViewModel CLIWorkspaceViewModelInfo`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[WorkspaceState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)；[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)；[Binding（shared/soulbinding声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/soulbinding/binding.go#L13)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[ProjectionStatus（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/resource.go#L70)；[CLIWorkspaceViewModelInfo](#schema-cliworkspaceviewmodelinfo)

<a id="schema-cliworkspacelistcommand"></a>

### CLIWorkspaceListCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:20](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L20)
- 声明：`type CLIWorkspaceListCommand struct{}`

<a id="schema-cliworkspacelistresult"></a>

### CLIWorkspaceListResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:21](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L21)
- 字段：`Workspaces []CLIWorkspaceInfo`
- 字段引用类型：[CLIWorkspaceInfo](#schema-cliworkspaceinfo)

<a id="schema-cliworkspaceruntimeviewinfo"></a>

### CLIWorkspaceRuntimeViewInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go:92](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go#L92)
- 字段：`WorkspaceID common.WorkspaceID`；`ActiveRunID common.RunID`；`DisplayRunID common.RunID`；`Projection projectioncontract.ProjectionStatus`；`Revision projectioncontract.WorkspaceRefreshRevision`；`PromptMode string`；`RunState common.RunState`；`TaskProgress CLITaskProgressInfo`；`RunHeartbeat projectioncontract.RunHeartbeat`；`ConvergenceStatus *projectioncontract.ConvergenceStatus`；`Acceptance projectioncontract.AcceptanceProjection`；`RunCommand *projectioncontract.RunCommandStatus`；`BlockingDecision *projectioncontract.Decision`；`BlockingApproval *projectioncontract.Approval`；`DisplayOutcome projectioncontract.WorkspaceDisplayOutcome`；`GovernanceDisplay projectioncontract.GovernanceDisplay`；`ConsistencyDiagnostic *projectioncontract.ConsistencyDiagnostic`；`Activity projectioncontract.WorkspaceActivityFeed`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[ProjectionStatus（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/resource.go#L70)；[WorkspaceRefreshRevision（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/revision.go#L7)；[RunState（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go#L7)；[CLITaskProgressInfo](#schema-clitaskprogressinfo)；[RunHeartbeat（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/run_heartbeat.go#L5)；[ConvergenceStatus（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/run.go#L33)；[AcceptanceProjection（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/acceptance.go#L7)；[RunCommandStatus（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/workspace_view.go#L90)；[Decision（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L6)；[Approval（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L66)；[WorkspaceDisplayOutcome（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/outcome.go#L21)；[GovernanceDisplay（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/display_consistency.go#L6)；[ConsistencyDiagnostic（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/display_consistency.go#L15)；[WorkspaceActivityFeed（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/workspace_view.go#L75)

<a id="schema-cliworkspacesnapshotcommand"></a>

### CLIWorkspaceSnapshotCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:22](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L22)
- 字段：`WorkspaceID common.WorkspaceID`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-cliworkspacesnapshotinfo"></a>

### CLIWorkspaceSnapshotInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go:52](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go#L52)
- 字段：`Workspace CLIWorkspaceInfo`；`ViewModel CLIWorkspaceViewModelInfo`；`Projection projectioncontract.ProjectionStatus`；`Run *projectioncontract.Run`；`Step *CLIStepViewInfo`；`Result *projectioncontract.Result`；`DisplayOutcome projectioncontract.WorkspaceDisplayOutcome`；`GovernanceDisplay projectioncontract.GovernanceDisplay`；`ConsistencyDiagnostic *projectioncontract.ConsistencyDiagnostic`；`TaskProgress CLITaskProgressInfo`；`RunHeartbeat projectioncontract.RunHeartbeat`；`ConvergenceStatus *projectioncontract.ConvergenceStatus`；`RunCommand *projectioncontract.RunCommandStatus`；`ContextBundle workorcheventpayload.Payload`；`CoverageSummary workorcheventpayload.Payload`；`GapBaseline *CLIGapBaselineInfo`；`AuthorityDiagnostics []projectioncontract.GapDiagnostic`；`Acceptance projectioncontract.AcceptanceProjection`；`TaskDecompositions []CLITaskDecompositionInfo`；`TaskPlans []CLITaskPlanInfo`；`ExecutionSections []projectioncontract.ExecutionSection`；`Goals []workorchmanagementcontract.Goal`；`DoDs []workorchmanagementcontract.DoD`；`Runs []projectioncontract.Run`；`Results []projectioncontract.Result`；`Artifacts []CLIArtifactInfo`；`ArtifactContinuations []CLIArtifactContinuationInfo`；`Patches []CLIPatchInfo`；`MemoryRecallTraces []CLIMemoryRecallTraceInfo`；`WebSources []projectioncontract.WebSourceTrace`；`LLMContextBudgets []projectioncontract.LLMContextBudgetTrace`；`LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary`；`LLMUsageTraces []projectioncontract.LLMUsageTrace`；`LLMUsageSummary projectioncontract.LLMUsageSummary`；`EventRecords []projectioncontract.EventRecord`；`Decisions []projectioncontract.Decision`；`Approvals []projectioncontract.Approval`
- 字段引用类型：[CLIWorkspaceInfo](#schema-cliworkspaceinfo)；[CLIWorkspaceViewModelInfo](#schema-cliworkspaceviewmodelinfo)；[ProjectionStatus（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/resource.go#L70)；[Run（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/resource.go#L104)；[CLIStepViewInfo](#schema-clistepviewinfo)；[Result（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/result.go#L8)；[WorkspaceDisplayOutcome（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/outcome.go#L21)；[GovernanceDisplay（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/display_consistency.go#L6)；[ConsistencyDiagnostic（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/display_consistency.go#L15)；[CLITaskProgressInfo](#schema-clitaskprogressinfo)；[RunHeartbeat（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/run_heartbeat.go#L5)；[ConvergenceStatus（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/run.go#L33)；[RunCommandStatus（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/workspace_view.go#L90)；[Payload（shared/workorcheventpayload声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorcheventpayload/payload.go#L43)；[CLIGapBaselineInfo](#schema-cligapbaselineinfo)；[GapDiagnostic（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/gap.go#L32)；[AcceptanceProjection（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/acceptance.go#L7)；[CLITaskDecompositionInfo](#schema-clitaskdecompositioninfo)；[CLITaskPlanInfo](#schema-clitaskplaninfo)；[ExecutionSection（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/execution_section.go#L3)；[Goal（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L27)；[DoD（shared/workorchmanagementcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchmanagementcontract/definition.go#L37)；[CLIArtifactInfo](#schema-cliartifactinfo)；[CLIArtifactContinuationInfo](#schema-cliartifactcontinuationinfo)；[CLIPatchInfo](#schema-clipatchinfo)；[CLIMemoryRecallTraceInfo](#schema-climemoryrecalltraceinfo)；[WebSourceTrace（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/web_git_trace.go#L3)；[LLMContextBudgetTrace（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/llm_observability.go#L42)；[LLMContextBudgetSummary（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/llm_observability.go#L86)；[LLMUsageTrace（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/llm_observability.go#L108)；[LLMUsageSummary（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/llm_observability.go#L141)；[EventRecord（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/llm_observability.go#L11)；[Decision（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L6)；[Approval（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/governance.go#L66)

<a id="schema-cliworkspacesnapshotresult"></a>

### CLIWorkspaceSnapshotResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:23](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L23)
- 字段：`Snapshot CLIWorkspaceSnapshotInfo`
- 字段引用类型：[CLIWorkspaceSnapshotInfo](#schema-cliworkspacesnapshotinfo)

<a id="schema-cliworkspaceupdatellmselectioncommand"></a>

### CLIWorkspaceUpdateLLMSelectionCommand

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L31)
- 字段：`WorkspaceID common.WorkspaceID`；`Selection *runtimecontract.ProviderInferenceSelection`；`ExpectedRevision uint64`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[ProviderInferenceSelection（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/provider_inference_selection.go#L12)

<a id="schema-cliworkspaceupdatellmselectionresult"></a>

### CLIWorkspaceUpdateLLMSelectionResult

- 源码：[internal/modules/application/workorchd/pkg/events/cli_contracts.go:36](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_contracts.go#L36)
- 字段：`Workspace CLIWorkspaceInfo`
- 字段引用类型：[CLIWorkspaceInfo](#schema-cliworkspaceinfo)

<a id="schema-cliworkspaceviewmodelinfo"></a>

### CLIWorkspaceViewModelInfo

- 源码：[internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/cli_workspace_dto.go#L13)
- 字段：`WorkspaceID common.WorkspaceID`；`State string`；`PromptMode string`；`Revision int64`；`StreamGeneration string`；`GlobalRevision int64`；`WorkspaceRevision int64`；`SourceEventRevision int64`；`ProjectedSourceRevision int64`；`UpdatedAt string`；`RebuiltAt string`；`Degraded bool`；`StaleReason string`；`Error string`；`FailedWorkspaceIds []string`；`DegradedSince string`；`Sections []projectioncontract.WorkspaceViewModelSection`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[WorkspaceViewModelSection（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/workspace_view.go#L25)

<a id="schema-readworkspaceruntimeviewcommand"></a>

### ReadWorkspaceRuntimeViewCommand

- 源码：[internal/modules/application/workorchd/pkg/events/messages.go:72](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L72)
- 字段：`Query workorchquerycontract.WorkspaceRuntimeViewQuery`
- 字段引用类型：[WorkspaceRuntimeViewQuery（shared/workorchquerycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchquerycontract/query.go#L49)

<a id="schema-readworkspaceruntimeviewresult"></a>

### ReadWorkspaceRuntimeViewResult

- 源码：[internal/modules/application/workorchd/pkg/events/messages.go:76](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L76)
- 字段：`View CLIWorkspaceRuntimeViewInfo`
- 字段引用类型：[CLIWorkspaceRuntimeViewInfo](#schema-cliworkspaceruntimeviewinfo)

<a id="schema-servicestoppeddata"></a>

### ServiceStoppedData

- 源码：[internal/modules/application/workorchd/pkg/events/messages.go:20](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L20)
- 字段：`ServiceName string`；`State string`；`ErrorCode string`；`ErrorMessage string`；`StoppedAt time.Time`

<a id="schema-workspaceprojectionchangeddata"></a>

### WorkspaceProjectionChangedData

- 源码：[internal/modules/application/workorchd/pkg/events/messages.go:49](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L49)
- 字段：`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StreamGeneration string`；`GlobalRevision int64`；`WorkspaceRevision int64`；`SourceEventRevision int64`；`ProjectedSourceRevision int64`；`RuntimeStateRevision uint64`；`EvidenceSequence uint64`；`ActivityRevision int64`；`WorkspaceTotal int`；`ChangeKind string`；`ChangedSections []projectioncontract.WorkspaceProjectionSection`；`ProjectionState string`；`UpdatedAt string`；`RebuiltAt string`；`StaleReason string`；`Error string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[WorkspaceProjectionSection（shared/workorchprojectioncontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchprojectioncontract/workspace_view.go#L161)

<a id="schema-workspacerunrequestedcommand"></a>

### WorkspaceRunRequestedCommand

- 源码：[internal/modules/application/workorchd/pkg/events/messages.go:28](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L28)
- 字段：`WorkspaceID common.WorkspaceID`；`Intent string`；`Reason string`；`RequestID string`；`Source string`
- 字段引用类型：[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)

<a id="schema-workspacerunrequestedresult"></a>

### WorkspaceRunRequestedResult

- 源码：[internal/modules/application/workorchd/pkg/events/messages.go:40](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L40)
- 声明：`type WorkspaceRunRequestedResult runtimecontract.RuntimeExecutionIntentAdmission`
- 字段引用类型：[RuntimeExecutionIntentAdmission（shared/workorchruntimecontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_execution_intent.go#L60)
- 实际方法：[Validate](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/events/messages.go#L42)
- 空值/identity/版本/范围原式：`err := (runtimecontract.RuntimeExecutionIntentAdmission(r)).Validate(); err != nil`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [workspaceProjectionChangedFromOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L291)：`event == nil || event.Source() != ModuleID`
  - 当前来源常量：`ModuleID = "application/workorchd"`