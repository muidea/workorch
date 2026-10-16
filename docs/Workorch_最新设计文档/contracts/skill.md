# skill：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页3个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Skill是技能资产owner。Create需要具体SkillID、内容/version及有限refs/tool/capability约束；Get按SkillID，List按CursorAfterID/Limit分页。Skill文本、RequiredToolKinds或MCP refs是输入资产，不是执行许可。

caller取得typed detached snapshot，并在activation封存版本/hash/provenance；更新默认资产不能热换旧Run。当前只有Create/Get/List三topic，不为文档完整性伪造Update/Delete能力。

## 逐topic合同

<a id="ct-skill-skillcommandcreate"></a>

### CT-skill-SkillCommandCreate

- 边界：`TopicSkillCommandCreate` = `/skill/command/create`；CF03；source existing / target retain；CommandTopic / Write/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/topics.go#L4)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L63)
- 接收provider：skill；Command [CreateSkillCommand](#schema-createskillcommand) → Result [CreateSkillResult](#schema-createskillresult)；target合法caller：application/workorchd/context-registry
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L51) → `handleSkillCommand`（skill）
- 接收入口：[handleSkillCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L117)
- 已观察外部引用：workorchd [CreateSkill](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/contextregistryports/skill_commands.go#L18)
- 本topic入口/具体类型分支的Source审查索引：[handleSkillCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L117)；[skillCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L174)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[skillCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L174)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS01](../Workorch_组件固定合同与分层验收规范.md#cs01)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`err := validateSkillCommand(cmd); err != nil`；`validateSkillID(cmd.SkillID) != nil || strings.TrimSpace(cmd.Name) == "" || strings.TrimSpace(cmd.Version) == "" || strings.TrimSpace(cmd.Instruction) == ""`；`hasBlank(cmd.TriggerRules) || hasBlank(cmd.ReferenceRefs) || hasBlank(cmd.RequiredToolKinds) || hasBlank(cmd.RequiredMCPServerRefs) || hasBlank(cmd.RequiredCapabilityRefs) || hasBlank(cmd.Constraints)`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-skill-skillcommandget"></a>

### CT-skill-SkillCommandGet

- 边界：`TopicSkillCommandGet` = `/skill/command/get`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/topics.go#L5)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L64)
- 接收provider：skill；Command [GetSkillCommand](#schema-getskillcommand) → Result [GetSkillResult](#schema-getskillresult)；target合法caller：application/workorchd/context-registry、application/workorchd/appservice
- 实际注册：[subscribeOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner_lifecycle.go#L37) → `subscribeOwner`（skill）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L51) → `handleSkillCommand`（skill）
- 接收入口：[subscribeOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner_lifecycle.go#L37)；[handleSkillCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L117)
- 已观察外部引用：workorchd [ReadSkillByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/context_soul_skill_reads.go#L39)
- 本topic入口/具体类型分支的Source审查索引：[handleSkillCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L117)；[skillCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L174)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[skillCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L174)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`ev == nil || result == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

<a id="ct-skill-skillcommandlist"></a>

### CT-skill-SkillCommandList

- 边界：`TopicSkillCommandList` = `/skill/command/list`；CF03；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L65)
- 接收provider：skill；Command [ListSkillsCommand](#schema-listskillscommand) → Result [ListSkillsResult](#schema-listskillsresult)；target合法caller：application/workorchd/context-registry、application/workorchd/appservice
- 实际注册：[subscribeOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner_lifecycle.go#L37) → `subscribeOwner`（skill）；[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L51) → `handleSkillCommand`（skill）
- 接收入口：[subscribeOwner](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner_lifecycle.go#L37)；[handleSkillCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L117)
- 已观察外部引用：workorchd [ListSkillsByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/context_soul_skill_reads.go#L55)
- 本topic入口/具体类型分支的Source审查索引：[handleSkillCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L117)；[skillCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L174)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 前置来源helper：[skillCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L174)；此处只索引实际helper，来源集合和逐分支语义以源码为准，不将target allowlist冒充已实现准入
- 特化承诺：[CS14](../Workorch_组件固定合同与分层验收规范.md#cs14)；仅应用该形状，不扩大本topic字段/准入
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`ev == nil || result == nil`
- 验收适用集：[CP13](../Workorch_组件固定合同与分层验收规范.md#cp13)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-createskillcommand"></a>

### CreateSkillCommand

- 源码：[internal/modules/blocks/skill/pkg/events/messages.go:13](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L13)
- 字段：`SkillID common.SkillID`；`Name string`；`Version string`；`Description string`；`TriggerRules []string`；`Instruction string`；`ReferenceRefs []string`；`RequiredToolKinds []string`；`RequiredMCPServerRefs []string`；`RequiredCapabilityRefs []string`；`Constraints []string`
- 字段引用类型：[SkillID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L25)

<a id="schema-createskillresult"></a>

### CreateSkillResult

- 源码：[internal/modules/blocks/skill/pkg/events/messages.go:36](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L36)
- 字段：`Skill *skillmodel.Skill`
- 字段引用类型：[Skill](#schema-skill)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L40)

<a id="schema-getskillcommand"></a>

### GetSkillCommand

- 源码：[internal/modules/blocks/skill/pkg/events/messages.go:27](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L27)
- 字段：`SkillID common.SkillID`
- 字段引用类型：[SkillID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L25)

<a id="schema-getskillresult"></a>

### GetSkillResult

- 源码：[internal/modules/blocks/skill/pkg/events/messages.go:44](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L44)
- 字段：`Skill *skillmodel.Skill`
- 字段引用类型：[Skill](#schema-skill)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L48)

<a id="schema-listskillscommand"></a>

### ListSkillsCommand

- 源码：[internal/modules/blocks/skill/pkg/events/messages.go:31](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L31)
- 字段：`CursorAfterID common.SkillID`；`Limit int`
- 字段引用类型：[SkillID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L25)

<a id="schema-listskillsresult"></a>

### ListSkillsResult

- 源码：[internal/modules/blocks/skill/pkg/events/messages.go:52](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L52)
- 字段：`Skills []*skillmodel.Skill`；`NextCursor common.SkillID`
- 字段引用类型：[Skill](#schema-skill)；[SkillID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L25)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/events/messages.go#L57)

<a id="schema-skill"></a>

### Skill

- 源码：[internal/modules/blocks/skill/pkg/models/skill.go:8](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/models/skill.go#L8)
- 字段：`ID common.SkillID`；`Name string`；`Version string`；`Description string`；`TriggerRules []string`；`Instruction string`；`ReferenceRefs []string`；`RequiredToolKinds []string`；`RequiredMCPServerRefs []string`；`RequiredCapabilityRefs []string`；`Constraints []string`；`Provenance workorchskillcontract.Provenance`
- 字段引用类型：[SkillID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L25)；[Provenance（shared/workorchskillcontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchskillcontract/skill.go#L28)
- 实际方法：[CloneSkill](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/models/skill.go#L36)；[CloneSkills](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/models/skill.go#L50)

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [handleSkillCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L117)：`ev == nil || !skillCommandSourceAllowed(ev.ID(), ev.Source())`
- [skillCommandSourceAllowed](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L174)：来源集合由switch/返回表达式判定，见完整函数
  - helper完整分支：`func skillCommandSourceAllowed(topic, source string) bool { switch topic { case skillevents.TopicSkillCommandCreate: return source == applicationContextRegistrySource case skillevents.TopicSkillCommandGet, skillevents.TopicSkillCommandList: return source == applicationContextRegistrySource || source == applicationOwnerReadSource default: return false } }`
  - 当前来源常量：`applicationContextRegistrySource = "application/workorchd/context-registry"`；`applicationOwnerReadSource = "application/workorchd/appservice"`