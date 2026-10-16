# soul：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页9个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Soul管理行为配置及内建默认版本。EnsureDefaults仅system启动协调；QueryBuiltinDefaults与UpgradeBuiltinDefault把当前记录、builtin版本及升级意图分开，升级不得覆盖用户已编辑内容而不做guard。

Create/Get/List/Update/Delete/ValidateCapabilitySelection都不授Runtime authority；能力配置校验通过不是actual owner可执行或目标满足。旧Run按封存snapshot恢复，不能用“当前默认值”重编译旧Definition。

## 逐topic合同

<a id="ct-soul-soulcommandcreate"></a>

### CT-soul-SoulCommandCreate

- 边界：`TopicSoulCommandCreate` = `/soul/command/create`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/topics.go#L4)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L118)
- 接收provider：soul；Command [CreateSoulCommand](#schema-createsoulcommand) → Result [CreateSoulResult](#schema-createsoulresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L54) → `handleSoulCommand`（soul）
- 接收入口：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)
- 已观察外部引用：workorchd [CreateSoul](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/soul_commands.go#L25)
- 本topic入口/具体类型分支的Source审查索引：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)；[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-soul-soulcommanddelete"></a>

### CT-soul-SoulCommandDelete

- 边界：`TopicSoulCommandDelete` = `/soul/command/delete`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L126)
- 接收provider：soul；Command [DeleteSoulCommand](#schema-deletesoulcommand) → Result [DeleteSoulResult](#schema-deletesoulresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L54) → `handleSoulCommand`（soul）
- 接收入口：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)
- 已观察外部引用：workorchd [DeleteSoul](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/soul_commands.go#L64)
- 本topic入口/具体类型分支的Source审查索引：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)；[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := m.service.deleteSoul(ctx, cmd.SoulID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-soul-soulcommandensuredefaults"></a>

### CT-soul-SoulCommandEnsureDefaults

- 边界：`TopicSoulCommandEnsureDefaults` = `/soul/command/ensure-defaults`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L130)
- 接收provider：soul；Command [EnsureDefaultsCommand](#schema-ensuredefaultscommand) → Result [EnsureDefaultsResult](#schema-ensuredefaultsresult)；target合法caller：kernel/system
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L54) → `handleSystemDefaultsCommand`（soul）
- 接收入口：[handleSystemDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L132)
- 已观察外部引用：system [ensureSoulDefaults](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/system/biz/system.go#L71)
- 本topic入口/具体类型分支的Source审查索引：[handleSystemDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L132)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != systemCommandSource` 分支谓词（[handleSystemDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L132)）
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`ev == nil || ev.Source() != systemCommandSource || m == nil || m.service == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-soul-soulcommandget"></a>

### CT-soul-SoulCommandGet

- 边界：`TopicSoulCommandGet` = `/soul/command/get`；CF03；source existing / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/topics.go#L5)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L120)
- 接收provider：soul；Command [GetSoulCommand](#schema-getsoulcommand) → Result [GetSoulResult](#schema-getsoulresult)；target合法caller：application/workorchd/context-registry、application/workorchd/appservice
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L54) → `handleSoulCommand`（soul）
- 接收入口：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)
- 已观察外部引用：workorchd [ReadSoul](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/soul_commands.go#L81)；workorchd [ReadSoulByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/context_soul_skill_reads.go#L23)
- 本topic入口/具体类型分支的Source审查索引：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)；[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-soul-soulcommandlist"></a>

### CT-soul-SoulCommandList

- 边界：`TopicSoulCommandList` = `/soul/command/list`；CF03；source existing / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L122)
- 接收provider：soul；Command [ListSoulsCommand](#schema-listsoulscommand) → Result [ListSoulsResult](#schema-listsoulsresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L54) → `handleSoulCommand`（soul）
- 接收入口：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)
- 已观察外部引用：workorchd [ListSouls](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/soul_commands.go#L98)
- 本topic入口/具体类型分支的Source审查索引：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)；[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-soul-soulcommandquerybuiltindefaults"></a>

### CT-soul-SoulCommandQueryBuiltinDefaults

- 边界：`TopicSoulCommandQueryBuiltinDefaults` = `/soul/command/query-builtin-defaults`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/builtin_defaults.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/builtin_defaults.go#L38)
- 接收provider：soul；Command [QueryBuiltinSoulDefaultsCommand](#schema-querybuiltinsouldefaultscommand) → Result [QueryBuiltinSoulDefaultsResult](#schema-querybuiltinsouldefaultsresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L54) → `handleBuiltinDefaultsCommand`（soul）
- 接收入口：[handleBuiltinDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/builtin_defaults.go#L15)
- 已观察外部引用：workorchd [queryBuiltinDefaults](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/builtin_defaults_service.go#L27)
- 本topic入口/具体类型分支的Source审查索引：[handleBuiltinDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/builtin_defaults.go#L15)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[handleBuiltinDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/builtin_defaults.go#L15)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-soul-soulcommandupdate"></a>

### CT-soul-SoulCommandUpdate

- 边界：`TopicSoulCommandUpdate` = `/soul/command/update`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L124)
- 接收provider：soul；Command [UpdateSoulCommand](#schema-updatesoulcommand) → Result [UpdateSoulResult](#schema-updatesoulresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L54) → `handleSoulCommand`（soul）
- 接收入口：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)
- 已观察外部引用：workorchd [UpdateSoul](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/soul_commands.go#L38)
- 本topic入口/具体类型分支的Source审查索引：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)；[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-soul-soulcommandupgradebuiltindefault"></a>

### CT-soul-SoulCommandUpgradeBuiltinDefault

- 边界：`TopicSoulCommandUpgradeBuiltinDefault` = `/soul/command/upgrade-builtin-default`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/builtin_defaults.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/builtin_defaults.go#L39)
- 接收provider：soul；Command [UpgradeBuiltinSoulDefaultCommand](#schema-upgradebuiltinsouldefaultcommand) → Result [UpgradeBuiltinSoulDefaultResult](#schema-upgradebuiltinsouldefaultresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L54) → `handleBuiltinDefaultsCommand`（soul）
- 接收入口：[handleBuiltinDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/builtin_defaults.go#L15)
- 已观察外部引用：workorchd [upgradeBuiltinDefault](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/builtin_defaults_service.go#L77)
- 本topic入口/具体类型分支的Source审查索引：[handleBuiltinDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/builtin_defaults.go#L15)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[handleBuiltinDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/builtin_defaults.go#L15)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-soul-soulcommandvalidatecapability"></a>

### CT-soul-SoulCommandValidateCapability

- 边界：`TopicSoulCommandValidateCapability` = `/soul/command/validate-capability`；CF03；source existing / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L128)
- 接收provider：soul；Command [ValidateCapabilitySelectionCommand](#schema-validatecapabilityselectioncommand) → Result [ValidateCapabilitySelectionResult](#schema-validatecapabilityselectionresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L54) → `handleSoulCommand`（soul）
- 接收入口：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)
- 已观察外部引用：workorchd [ValidateSoulCapabilitySelection](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/soul_capability_validation.go#L14)
- 本topic入口/具体类型分支的Source审查索引：[handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)；[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := m.service.validateCapabilitySelection(ctx, cmd); err != nil`；`err := validateSoulID(cmd.SoulID); err != nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-builtinsouldefault"></a>

### BuiltinSoulDefault

- 源码：[internal/modules/blocks/soul/pkg/events/builtin_defaults.go:14](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/builtin_defaults.go#L14)
- 字段：`SoulID common.SoulID`；`Present bool`；`CurrentContentHash string`；`CurrentBuiltinRevision string`；`BuiltinContentHash string`；`UpgradeAvailable bool`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-createsoulcommand"></a>

### CreateSoulCommand

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L13)
- 字段：`SoulID common.SoulID`；`Name string`；`Description string`；`BusinessDomains []string`；`SoulMD string`；`AllowedSkillRefs []string`；`AllowedMCPServerRefs []string`；`AllowedCapabilityRefs []string`；`AllowedCapabilityCategories []string`；`AllowedToolKinds []string`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-createsoulresult"></a>

### CreateSoulResult

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:55](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L55)
- 字段：`Soul *soulmodel.Soul`
- 字段引用类型：[Soul](#schema-soul)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L59)

<a id="schema-deletesoulcommand"></a>

### DeleteSoulCommand

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:28](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L28)
- 字段：`SoulID common.SoulID`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-deletesoulresult"></a>

### DeleteSoulResult

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:88](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L88)
- 字段：`SoulID common.SoulID`；`Deleted bool`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-ensuredefaultscommand"></a>

### EnsureDefaultsCommand

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:49](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L49)
- 声明：`type EnsureDefaultsCommand struct{}`

<a id="schema-ensuredefaultsresult"></a>

### EnsureDefaultsResult

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:51](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L51)
- 字段：`Ensured bool`

<a id="schema-getsoulcommand"></a>

### GetSoulCommand

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:41](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L41)
- 字段：`SoulID common.SoulID`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-getsoulresult"></a>

### GetSoulResult

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:63](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L63)
- 字段：`Soul *soulmodel.Soul`
- 字段引用类型：[Soul](#schema-soul)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L67)

<a id="schema-listsoulscommand"></a>

### ListSoulsCommand

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:45](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L45)
- 字段：`CursorAfterID common.SoulID`；`Limit int`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-listsoulsresult"></a>

### ListSoulsResult

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:71](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L71)
- 字段：`Souls []*soulmodel.Soul`；`NextCursor common.SoulID`
- 字段引用类型：[Soul](#schema-soul)；[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L76)

<a id="schema-querybuiltinsouldefaultscommand"></a>

### QueryBuiltinSoulDefaultsCommand

- 源码：[internal/modules/blocks/soul/pkg/events/builtin_defaults.go:23](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/builtin_defaults.go#L23)
- 声明：`type QueryBuiltinSoulDefaultsCommand struct{}`

<a id="schema-querybuiltinsouldefaultsresult"></a>

### QueryBuiltinSoulDefaultsResult

- 源码：[internal/modules/blocks/soul/pkg/events/builtin_defaults.go:24](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/builtin_defaults.go#L24)
- 字段：`Defaults []BuiltinSoulDefault`
- 字段引用类型：[BuiltinSoulDefault](#schema-builtinsouldefault)

<a id="schema-soul"></a>

### Soul

- 源码：[internal/modules/blocks/soul/pkg/models/soul.go:25](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/models/soul.go#L25)
- 字段：`ID common.SoulID`；`Revision uint64`；`ContentHash string`；`SourceProvenance string`；`Name string`；`Description string`；`BusinessDomains []string`；`SoulMD string`；`AllowedSkillRefs []string`；`AllowedMCPServerRefs []string`；`AllowedCapabilityRefs []string`；`AllowedCapabilityCategories []string`；`AllowedToolKinds []string`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)
- 实际方法：[StableSoulContentHash](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/models/soul.go#L41)；[CloneSoul](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/models/soul.go#L58)；[CloneSouls](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/models/soul.go#L72)
- 空值/identity/版本/范围原式：`item == nil`

<a id="schema-updatesoulcommand"></a>

### UpdateSoulCommand

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:26](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L26)
- 实际alias/同形类型：[CreateSoulCommand](#schema-createsoulcommand)
- 字段引用类型：[CreateSoulCommand](#schema-createsoulcommand)

<a id="schema-updatesoulresult"></a>

### UpdateSoulResult

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:80](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L80)
- 字段：`Soul *soulmodel.Soul`
- 字段引用类型：[Soul](#schema-soul)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L84)

<a id="schema-upgradebuiltinsouldefaultcommand"></a>

### UpgradeBuiltinSoulDefaultCommand

- 源码：[internal/modules/blocks/soul/pkg/events/builtin_defaults.go:26](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/builtin_defaults.go#L26)
- 字段：`SoulID common.SoulID`；`ExpectedContentHash string`；`TargetContentHash string`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-upgradebuiltinsouldefaultresult"></a>

### UpgradeBuiltinSoulDefaultResult

- 源码：[internal/modules/blocks/soul/pkg/events/builtin_defaults.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/builtin_defaults.go#L31)
- 字段：`Default BuiltinSoulDefault`；`Upgraded bool`
- 字段引用类型：[BuiltinSoulDefault](#schema-builtinsouldefault)

<a id="schema-validatecapabilityselectioncommand"></a>

### ValidateCapabilitySelectionCommand

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:32](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L32)
- 字段：`SoulID common.SoulID`；`SkillRefs []string`；`MCPServerRefs []string`；`CapabilityRefs []string`；`CapabilityCategories []string`；`ToolKinds []string`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

<a id="schema-validatecapabilityselectionresult"></a>

### ValidateCapabilitySelectionResult

- 源码：[internal/modules/blocks/soul/pkg/events/messages.go:93](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/events/messages.go#L93)
- 字段：`SoulID common.SoulID`；`Validated bool`
- 字段引用类型：[SoulID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L21)

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [handleSoulCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L160)：`ev == nil || !soulCommandSourceAllowed(ev.ID(), ev.Source())`
- [soulCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L249)：来源集合由switch/返回表达式判定，见完整函数
  - helper完整分支：`func soulCommandSourceAllowed(topic, source string) bool { switch topic { case soulevents.TopicSoulCommandCreate, soulevents.TopicSoulCommandList, soulevents.TopicSoulCommandUpdate, soulevents.TopicSoulCommandDelete, soulevents.TopicSoulCommandValidateCapability: return source == applicationContextRegistrySource case soulevents.TopicSoulCommandGet: return source == applicationContextRegistrySource || source == applicationOwnerReadSource default: return false } }`
  - 当前来源常量：`applicationContextRegistrySource = "application/workorchd/context-registry"`；`applicationOwnerReadSource = "application/workorchd/appservice"`
- [handleSystemDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L132)：`ev == nil || ev.Source() != systemCommandSource || m == nil || m.service == nil`
  - 当前来源常量：`systemCommandSource = "kernel/system"`
- [handleBuiltinDefaultsCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/builtin_defaults.go#L15)：`m == nil || m.service == nil || ev == nil || ev.Source() != applicationContextRegistrySource`
  - 当前来源常量：`applicationContextRegistrySource = "application/workorchd/context-registry"`