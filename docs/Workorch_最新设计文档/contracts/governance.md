# governance：实际组件合同卡

- 文档版本v0.3；日期2026-10-12；源码固定 `5360caf36e081717087557f34d2af3af1c16dc16`；本页5个真实topic，均为设计候选，未执行项目测试/服务/live
- Command provider是能力接收方；Data owner是事实发布者，订阅方不复制消息schema；[共同规则CC01–CC06](../Workorch_组件固定合同与分层验收规范.md#generic-contract)只在规范维护
- source/target/usage分别记录。observed reference只代表读取到的静态引用，不能推出调用已被运行验证；target合法caller是本版有界allowlist，新增caller需修改具体合同

## Owner局部规则

Governance协调Decision/Approval的自动政策和trace，不能直接写其store或自行改变Definition/Run。四个自动通知只表示evaluated/resolved/escalated/blocked的事实，不将其转换成无界授权。

ListAutoGovernanceTraces要求WorkspaceID、Limit 1…500；所有query string必须UTF-8、无前后空白、≤4 KiB。trace query只读，不含owner私有startup recovery inventory。自动治理结果须保存真实policy/principal与范围，不能伪装人工点击。

## 逐topic合同

<a id="ct-governance-governanceautoblocked"></a>

### CT-governance-GovernanceAutoBlocked

- 边界：`TopicGovernanceAutoBlocked` = `/governance/auto/blocked`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/topics.go#L9)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L70)
- 唯一事实owner/发布者：governance；Data [GovernanceData](#schema-governancedata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeGovernanceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L480) → `subscribeGovernanceNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeGovernanceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L480)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-governance-governanceautocommandlisttraces"></a>

### CT-governance-GovernanceAutoCommandListTraces

- 边界：`TopicGovernanceAutoCommandListTraces` = `/governance/auto/command/list-traces`；CF07；source query / target retain；CommandTopic / Read/Query
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/topics.go#L10)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L62)
- 接收provider：governance；Command [ListAutoGovernanceTracesCommand](#schema-listautogovernancetracescommand) → Result [GovernanceCommandResult](#schema-governancecommandresult)；target合法caller：application/workorchd/appservice
- 实际注册：[Start](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L148) → `handleCommand`（governance）
- 接收入口：[handleCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L386)
- 已观察外部引用：workorchd [ListAutoGovernanceTracesByOwnerCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/ownerreadports/governance_reads.go#L15)
- 本topic入口/具体类型分支的Source审查索引：[executeCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L398)；只列直接入口与显式类型分支及其来源helper，不合并router其他family；条件执行范围以完整分支为准
- 源caller准入：`Source != "application/workorchd/appservice"` 分支谓词（[executeCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L398)）
- 特化承诺：Query.WorkspaceID必填，Limit必须1…500；Workspace/Run/Step/type/id/status每串≤4096字节、UTF-8且无首尾空格。Run/Step可空只表示workspace级过滤，不能跨workspace。
- 源码分支条件索引（`if`原式；并非每条均为拒绝，true/false后果以对应入口为准）：`result == nil`；`s == nil || s.traces == nil || ev == nil || ev.Source() != "application/workorchd/appservice"`
- 验收适用集：[CP09](../Workorch_组件固定合同与分层验收规范.md#cp09)

<a id="ct-governance-governanceautoescalated"></a>

### CT-governance-GovernanceAutoEscalated

- 边界：`TopicGovernanceAutoEscalated` = `/governance/auto/escalated`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/topics.go#L8)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L70)
- 唯一事实owner/发布者：governance；Data [GovernanceData](#schema-governancedata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeGovernanceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L480) → `subscribeGovernanceNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeGovernanceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L480)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-governance-governanceautoevaluated"></a>

### CT-governance-GovernanceAutoEvaluated

- 边界：`TopicGovernanceAutoEvaluated` = `/governance/auto/evaluated`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/topics.go#L6)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L70)
- 唯一事实owner/发布者：governance；Data [GovernanceData](#schema-governancedata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeGovernanceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L480) → `subscribeGovernanceNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeGovernanceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L480)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

<a id="ct-governance-governanceautoresolved"></a>

### CT-governance-GovernanceAutoResolved

- 边界：`TopicGovernanceAutoResolved` = `/governance/auto/resolved`；CF11；source notification / target retain；DataTopic / Notify/State
- 源定义：[topic](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/topics.go#L7)；[MessageContracts](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L70)
- 唯一事实owner/发布者：governance；Data [GovernanceData](#schema-governancedata)；同步Command/Result：不适用；允许订阅者：eventstream、process/workorch、workorchd
- 实际注册：[subscribeGovernanceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L480) → `subscribeGovernanceNotifications`（eventstream）
- 已读事实来源引用（含发布/分派路径）：本轮未定位事实来源引用；保留schema，不据此宣称unused或已经发布
- 已观察外部引用：eventstream [subscribeGovernanceNotifications](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/eventstream/biz/event_stream_service.go#L480)；process/workorch [convertBackendEvent](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/biz/backends/eventhub.go#L269)
- 特化承诺：[CS09](../Workorch_组件固定合同与分层验收规范.md#cs09)；仅应用该形状，不扩大本topic字段/准入
- 验收适用集：[CP15](../Workorch_组件固定合同与分层验收规范.md#cp15)

## Schema、空值和校验索引

以下为本owner实际具体类型，每个只定义一次；跨owner neutral Runtime纯值只引用原类型，完整状态语义以Core/Runtime规格为准。类型缺少Validate不等于任意字段合法，须连同上方真实handler guard阅读；字段表是源码声明，目标新增只在明确change/NC条目中提出。

<a id="schema-autogovernancetrace"></a>

### AutoGovernanceTrace

- 源码：[internal/modules/kernel/governance/pkg/models/auto_governance_trace.go:18](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/models/auto_governance_trace.go#L18)
- 字段：`ID common.AutoGovernanceTraceID`；`WorkspaceID common.WorkspaceID`；`RunID common.RunID`；`StepID common.StepID`；`ItemType string`；`ItemID string`；`Action string`；`RiskLevel string`；`Confidence float64`；`PolicyMatched string`；`PolicyBlocked string`；`BudgetBefore int`；`BudgetAfter int`；`Reason string`；`EvidenceRefs []string`；`PromptFingerprint string`；`LLMProviderID string`；`LLMModel string`；`LLMDecisionPreview string`；`Status AutoGovernanceTraceState`；`OccurredAt string`
- 字段引用类型：[AutoGovernanceTraceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L24)；[WorkspaceID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L3)；[RunID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L6)；[StepID（shared/common声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/ids.go#L7)；[AutoGovernanceTraceState](#schema-autogovernancetracestate)
- 实际方法：[CloneAutoGovernanceTrace](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/models/auto_governance_trace.go#L42)；[CloneAutoGovernanceTraces](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/models/auto_governance_trace.go#L51)

<a id="schema-autogovernancetracestate"></a>

### AutoGovernanceTraceState

- 源码：[internal/modules/kernel/governance/pkg/models/auto_governance_trace.go:8](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/models/auto_governance_trace.go#L8)
- 声明：`type AutoGovernanceTraceState string`
- 源码枚举值：`AutoGovernanceTraceStateEvaluated = "evaluated"`；`AutoGovernanceTraceStateResolving = "resolving"`；`AutoGovernanceTraceStateResolved = "resolved"`；`AutoGovernanceTraceStateEscalated = "escalated"`；`AutoGovernanceTraceStateBlocked = "blocked"`

<a id="schema-governancecommandresult"></a>

### GovernanceCommandResult

- 源码：[internal/modules/kernel/governance/pkg/events/messages.go:52](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L52)
- 字段：`Traces []*governancemodel.AutoGovernanceTrace`
- 字段引用类型：[AutoGovernanceTrace](#schema-autogovernancetrace)
- 实际方法：[Clone](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L56)

<a id="schema-governancedata"></a>

### GovernanceData

- 源码：[internal/modules/kernel/governance/pkg/events/messages.go:47](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L47)
- 字段：`Trace governancemodel.AutoGovernanceTrace`；`EscalationReason string`
- 字段引用类型：[AutoGovernanceTrace](#schema-autogovernancetrace)

<a id="schema-listautogovernancetracescommand"></a>

### ListAutoGovernanceTracesCommand

- 源码：[internal/modules/kernel/governance/pkg/events/messages.go:23](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L23)
- 字段：`Query workorchquerycontract.AutoGovernanceTraceQuery`
- 字段引用类型：[AutoGovernanceTraceQuery（shared/workorchquerycontract声明）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchquerycontract/query.go#L99)
- 实际方法：[Normalized](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/events/messages.go#L31)
- 空值/identity/版本/范围原式：`len(value) > MaxAutoGovernanceTraceQueryStringBytes || !utf8.ValidString(value) || value != strings.TrimSpace(value)`；`c.Query.WorkspaceID == "" || c.Query.Limit < 1 || c.Query.Limit > MaxAutoGovernanceTraceQueryLimit`

## 源码来源准入索引

以下按真实函数去重。列举源码表达式，不将多个互斥/或条件拆成独立拒绝结论；详细return与副作用前后顺序以链接完整函数为准。

- [executeCommand](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L398)：`s == nil || s.traces == nil || ev == nil || ev.Source() != "application/workorchd/appservice"`