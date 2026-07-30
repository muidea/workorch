# Runtime V2 单 Owner 与合同归一收口方案

Status: completed

Type: implementation closure plan

Plan ID: `RUNTIME-SMO`

Last Updated: 2026-07-29

## 1. 目的、范围与唯一性

本文记录已完成的 `RUNTIME-SMO` 收口。它曾统一处理以下结构性问题：

1. 同一 Runtime 决策由 `ConvergenceState`、`RuntimeAction`、`RequiredOutput.Kind` 重复表达；
2. `internal/pkg/workorchruntimecontract` 中的 failure disposition、candidate correction 和 owner-code
   映射实际参与状态迁移；
3. capability、candidate、drive、delivery 的详细失败被压缩为通用 error/class 后，Orchestrator 可能选择错误的
   retry、repair、governance、CAS reload 或 terminal 路径；
4. 部分转换只保留归一后的 code/message，原 owner/provider 的 audit-safe diagnostic、字段违例、scope 和 revision
   lineage 不完整；
5. Run、Workspace、Result 与 capability result 重复解释 Orchestrator action 或 operation outcome，形成跨 owner
   双权威。

完成时的唯一目标是：

> 状态机只消费少量、稳定、owner-neutral 的事实；每个事实同时保留产生者提供的完整、安全原始诊断。归一分类
> 只能新增，不能覆盖原始反馈。所有迁移只由状态机 owner 的内部事件和 reducer 决定。

该轮未增加 Module/Block，未恢复 V1，未增加兼容双写，未引入 EventHub Envelope，未使用
`map[string]any`/JSON/reflect bridge，不把状态迁移、CAS、retry 或 I/O 放入共享 `pkg`。

当时不保留历史 Runtime aggregate 兼容路径。合同结构变更必须原子更新 producer、consumer、fixture 与
持久化 schema/revision；旧 schema 只能 typed fail-fast，开发/live 验证使用 fresh data，禁止双读、双写或
按字段存在性路由。

权威顺序：

1. `docs/00-core/runtime-stage-contract.md`：四状态机与禁止迁移；
2. `docs/00-core/runtime-request-cache-note.md`：provider-safe feedback 与 C0～C5；
3. `internal/modules/kernel/orchestrator/readme.md`：Orchestrator 实现边界；
4. `internal/pkg/workorchruntimecontract`：完成收口后的纯合同实现；
5. 本文：completed 验证记录；不再定义活动实施队列。

历史 live workspace、过程卡、旧 SMO/C7/C8 只作为测试反例来源，不继续占用 active 计划，也不能约束新实现。

## 2. 评审结论与当前偏差

### 2.1 分类粒度的正确判断

当前不是“所有合同都过细”，而是两种粒度被错误混用：

| 层 | 当前问题 | 目标 |
| --- | --- | --- |
| 状态机消费层 | failure/action/disposition 重复且过细，owner 私有码参与迁移 | 每个结果域只保留少量稳定 factual category |
| 诊断与审计层 | 原始 code/message/scope/details 在转换中被裁剪或覆盖 | 保留产生者安全原始反馈及完整 lineage |

不得通过删除详细 owner code 来简化状态机；也不得通过继续增加全局 `FailureClass` 来保留诊断。正确模型是
“稳定分类 + 原始诊断”并存。

### 2.2 已确认的重复权威

`RuntimeDecision` 当前同时持久化：

```text
ConvergenceState
RuntimeAction
RequiredOutput.Kind
```

三者存在固定一一映射，`RuntimeDecision.Validate()` 只是在防止三份权威漂移。收口后保留一个 canonical
`ConvergenceState`；`RuntimeAction` 和 output kind 仅可作为临时派生投影，不进入 aggregate、authority snapshot、
transaction、terminal delivery 或 owner notification 的正式 identity。

### 2.3 已确认的跨 owner 策略

以下对象是本轮审计的迁移清单；已标记完成的项不得重新回流到共享合同包：

- `FailureClassDisposition`、`FailureClassIsBusinessTerminal`：已从 pkg 迁至 Orchestrator owner-local failure policy；
- `RuntimeCandidateRejectionCodeDisposition`、`RuntimeCandidateRejectionTerminalCode`、
  `SealRuntimeCandidateRejection` 中的处置矩阵：已从 pkg 迁至 SM-3 owner-local policy；
- `RuntimeOwnerFeasibilityCategoryFor` 的集中式 `owner + code -> category` 表；
- `DeriveRuntimeRepairHints` 对 failure code/message 的字符串匹配：已删除，LLM/projection 只读取 sealed directive、typed gap 和原始安全诊断；
- `classifyRuntimeV2DriveFailure` 将所有 `common.ErrConflict` 解释为 CAS conflict：已改为只接受 typed CAS conflict；
- terminal delivery command/acknowledgement 用 `RuntimeAction` 指示 complete/fail；
- capability adapter 的 `Status` 与 `RuntimeOperationReceipt.Outcome` 双重结果表达。

这些逻辑不能继续作为共享 DTO helper 或跨 owner 隐式协议。

### 2.4 不得错误合并的独立状态

以下合同虽然都叫 state/disposition/outcome，但属于不同状态机或事务，必须保持独立：

- candidate request session state；
- candidate document framing disposition（complete/incomplete/empty 的纯边界 observation）；
- candidate attempt state；
- sealed dispatch state；
- operation execution state；
- assessment disposition；
- repair-surface disposition；
- target-selection disposition；
- governance binding state；
- delivery acknowledgement state。
- SM-1 `RuntimeTerminationRecord.Kind`（cancelled/superseded 的 owner-local intent/record）。

本计划只消除重复权威和跨域处置，不建立一个全局 `Disposition` 枚举。

## 3. 不可变设计规则

### 3.1 单一 owner 与内部事件

| 状态域 | 唯一 owner | 可消费的外部输入 | 禁止 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run`（owner reducer 位于 `biz`） | activation/continuation intent、terminal delivery fact、governance continuation fact | 外部指定 Run 目标 state、选择 Runtime decision |
| SM-2 authority/focus | `kernel/orchestrator`（owner state/reducer 位于 `biz/runtimev2`） | typed prerequisite、feasibility、receipt、assessment、governance fact | capability/adapter 选择下一 decision |
| SM-3 candidate/dispatch | `kernel/orchestrator`（owner state/reducer 位于 `biz/runtimev2`） | request outcome、complete document、candidate diagnostic、owner feasibility/receipt | adapter 关闭 attempt、seal dispatch 或选择 retry |
| SM-4 drive/recovery/delivery | `kernel/orchestrator`（drive coordinator 位于 `biz`，owner state/reducer 位于 `biz/runtimev2`） | Run fact、typed recovery fact、delivery acknowledgement | 重建 SM-2 authority、写 Run/Workspace/Result state |
| capability 技术状态 | 各 capability Block Biz | sealed typed operation/probe | 修改 aggregate/focus/coverage/terminal |
| Result/Workspace 资源状态 | 各自 Block Biz | sealed terminal outcome | 裁决 Runtime terminal 或 evidence |

跨 owner 仍使用 owner 在自身 `pkg/events` 定义的具体 `Command/Data/Result`。收到外部结果后，Orchestrator 必须
先转换为 owner-local internal event，再由唯一 transition coordinator CAS；internal event 不得成为新的 EventHub
通用消息。

### 3.2 三类字段必须分开

每份 Runtime 合同字段只能属于以下一类：

1. **Canonical authority**：唯一可决定状态或幂等 identity 的字段；必须持久化；
2. **Derived projection**：由 canonical authority 纯派生；不得独立持久化或参与 CAS/hash；
3. **Original diagnostic**：产生者的安全原始反馈；用于审计、排障和有界 provider correction；不得选择迁移。

同一字段不得同时承担两个类别。任何新增字段必须在代码评审中标明所属类别。

### 3.3 原始反馈保真边界

“不丢失原始信息”指：进入 owner typed result 边界后，所有允许跨边界的安全事实都必须原样保留，不得被
归一 category、通用 error 或新 summary 覆盖。它不授权持久化 credential、endpoint、raw prompt、raw
provider body、未脱敏 filesystem 内容或其他 core 明确禁止的数据。

大型且允许持久化的 owner 诊断可使用 owner-controlled、不可变、受 scope 约束的 `DetailsRef + DetailsHash`；
禁止内容只能保存既有 request/session audit 允许的 hash、byte count、disposition、redacted/omitted 标记，不能
建立指向禁止内容的 Runtime ref。

## 4. 目标合同模型

### 4.1 唯一 Runtime decision

收口后使用 `ConvergenceState` 作为 SM-2 唯一 canonical decision：

| Canonical `ConvergenceState` | 派生执行语义 | 派生 output 语义 |
| --- | --- | --- |
| `need_prerequisite` | acquire prerequisite | prerequisite |
| `need_state_change` | request state-change candidate | durable state change |
| `need_validation` | request validation candidate | validation |
| `need_assessment` | request assessment candidate | assessment |
| `need_governance` | bind/request governance | governance dependency |
| `complete` | terminal complete | none |
| `failed` | terminal fail | none |

具体要求：

- `RuntimeDecision` 不再持久化 `Action`；
- `RequiredOutputContract` 不再持久化与 state 一一对应的 `Kind`，但继续保留 accepted kinds、effect scope、
  evidence kinds、coverage、target、prerequisite 和 required operation sequence；
- `AuthoritySnapshot`、`AuthorityTransaction`、candidate request identity、repair directive 使用 canonical state；
- provider renderer 可生成 `requestedOperationMode` 等只读文案/DTO，但该值必须从 sealed state/output 派生；
- `RuntimeDecision` 内只保存绑定 canonical state 的 terminal code/summary，不再保存第二个 terminal kind；SM-4
  seal 跨 owner delivery fact 时才生成 `TerminalOutcome{Kind, Code, Summary}`；
- sealed `TerminalOutcome.Kind` 只有 `completed | failed | cancelled | superseded`。`completed | failed` 从 SM-2
  terminal state 派生，`cancelled | superseded` 只能来自 SM-1 typed administrative intent/fact；一旦 seal，它是该
  delivery transaction 的 canonical identity，下游不得再读取 `ConvergenceState` 推导；现有 action 与独立
  `AdministrativeKind` 均删除；
- 状态机测试只断言 canonical state 和具体 output constraints，不再组合断言三份字段一致。

### 4.2 统一反馈值，但不建立通用消息

`internal/pkg/workorchruntimecontract` 可定义 owner-neutral 的纯值 DTO，供 concrete Result 嵌入：

```text
RuntimeFeedback
  Domain                 // request | candidate | feasibility | operation | drive | delivery
  StableCategory         // 对应 Domain 的稳定事实类别
  WorkspaceID
  RunID
  AuthoritySnapshotID    // 适用时必填
  DispatchID             // 适用时必填
  OriginalDiagnostic
    SchemaVersion
    OwnerModuleID
    OriginalClass
    OriginalCode
    OriginalMessage
    Signature
    OperationID
    TargetRefs
    ExpectedRevision
    ObservedRevision
    FieldViolations[]
    DetailsRef
    DetailsHash
    ContentType
    Truncated
    Redacted
```

约束：

- `RuntimeFeedback` 是值对象，不是 EventHub Envelope；每个 owner 继续返回自己的 concrete Result；
- `StableCategory` 只能由产生事实的 owner 或明确的 Orchestrator boundary adapter 创建；
- workspace-scoped feedback 必须携带 workspace/run，authority/dispatch/operation identity 按所处边界必填；
- `OriginalDiagnostic` 仅在产生者确实没有 diagnostic（例如普通 accepted fact）时可为空；一旦 producer 提供了
  允许跨边界的安全 diagnostic，adapter 不得丢弃、替换或伪造；
- `OriginalClass/Code/Message` 一经进入合同不得被覆盖；派生 summary 使用独立字段；
- `FieldViolations` 保留 path、actual、allowed、required、forbidden，不能只留下拼接后的 message；
- 有业务顺序的 output/event/violation 保持原顺序；仅无顺序语义的 canonical comparison 副本允许排序去重；
- clone/normalize 不得修改 original 字段。hash helper 对 canonical copy 计算，不能反写原值；
- `Cause error` 不跨 EventHub；adapter 必须把安全的 cause chain 映射为 structured diagnostic，禁止内容只记录
  redaction/audit 元数据。

### 4.3 分域稳定分类

不得建立全局 failure disposition。每个边界只暴露本状态机所需的最小 factual category。

#### Candidate request session

```text
document_complete
document_incomplete
provider_temporarily_unavailable
provider_authentication_required
provider_configuration_required
provider_protocol_rejected
provider_submission_unknown
prerequisite_stale
continuation_exhausted
internal_contract_defect
```

以上只是 provider/request session 事实，不创建或失败 candidate attempt。随后 complete document 才进入
candidate domain。

#### Candidate evaluation

```text
accepted
contract_rejected
internal_contract_defect
```

`correctable_once` 不是 owner failure class，而是 SM-3 在读取 persisted candidate diagnostic、manifest 和 attempt
ledger 后产生的 owner-local transition result。是否 retry、terminal 不出现在 LLM Result 或共享 pkg mapping 中。

#### Owner feasibility

```text
feasible
stale
prerequisite_required
infeasible
```

`infeasible` 可附带 owner-neutral factual category，例如 resource absent、environment unavailable、policy
disallowed、target invalid、validation recipe unavailable、observation unknown。owner 私有 code 到 factual category
的映射归各 capability Biz/owner contract；共享 pkg 不维护全 owner 词汇表。

#### Operation receipt

```text
accepted
rejected
no_op
stale
prerequisite_required
governance_required
outcome_unknown
```

`RuntimeOperationReceipt.Outcome` 是唯一 canonical operation result。adapter 的 `Status`、`FailureCode`、
`ErrorCode` 不再形成第二结果权威；它们迁入 `OriginalDiagnostic` 或 owner-private 技术结果。`no_op` 只表达
期望 durable effect 但 owner 观测到零变化；read-only/validation 的正常零 durable effect 仍为 `accepted`。
删除与 `Outcome=no_op` 重复的跨边界 `NoOp bool`；`Changed` 只保留为 observed effect metadata，并受
outcome/effect-scope invariant 约束。

#### Drive/recovery/delivery

```text
concurrent_state_changed
temporarily_unavailable
operation_outcome_unknown
delivery_failed
internal_contract_defect
```

只有 repository expected revision 与 actual revision 不一致可产生 `concurrent_state_changed`。语义拒绝、stale
identity、非法 transition、candidate session conflict 不能使用 `common.ErrConflict` 进入 CAS reload。

真实权限、配置或外部依赖继续使用独立 `ExternalDependency/GovernanceRequirement`，不压入 failure category。

### 4.4 provider-safe 与审计反馈分离

LLM C4 只能消费：

- manifest-bound field violations；
- bounded correction objectives；
- rejected shape hash；
- required/allowed/forbidden 的 provider-safe projection；
- current-run、current-focus 的安全 evidence identity。

LLM 不得收到 owner raw message、stderr/stdout、provider body、Runtime retry/terminal 决策、CAS/delivery 状态或
未封存 target。`OriginalDiagnostic` 的存在不意味着可以全部回灌 provider。

`RuntimeCandidateFailureDiagnosticFromPayloadDiagnostic` 等转换必须形成两条输出：

1. persisted/audit-safe original diagnostic；
2. provider-safe correction projection。

任何一条都不能覆盖另一条。

## 5. 合同保留、合并、下沉、删除清单

| 当前合同/逻辑 | 处理 | 最终 owner/形态 |
| --- | --- | --- |
| `ConvergenceState` | 保留并提升为唯一权威 | Orchestrator aggregate |
| `RuntimeAction` | 从持久化与跨 owner identity 删除 | 仅局部纯派生；完成后评估是否删除类型 |
| `RequiredOutput.Kind` | 删除重复字段 | 从 canonical state 派生 |
| `FailureFact` 原始 class/code/message | 重构并完整保留 | `OriginalDiagnostic` |
| `FailureClassDisposition` | 从 pkg 删除 | Orchestrator owner-local reducer/policy |
| candidate rejection code/disposition matrix | 从 pkg 删除 | SM-3 internal-event reducer |
| `RuntimeOwnerFeasibilityCategoryFor` 全局表 | 从 pkg 删除 | 各 capability owner 本地 total mapping |
| `DeriveRuntimeRepairHints` 字符串分流 | 删除 | Orchestrator typed correction objective；LLM 只渲染 |
| `RuntimeOperationReceipt` | 保留 | 唯一 operation outcome/effect/evidence receipt |
| receipt `NoOp bool` | 删除重复字段 | 使用 `Outcome=no_op`；`Changed` 仅为 effect metadata |
| capability adapter `Status + ErrorCode/FailureCode` | 合并 | owner-private 技术 DTO 或 `OriginalDiagnostic` |
| prerequisite stale/resource revision | 保留并独立于 failure | SM-2 factual input |
| governance dependency/binding | 保留并独立于 failure | Governance/Decision owner fact + Orchestrator binding |
| `RuntimeTerminalDeliveryIdentity.Action`、独立 `AdministrativeKind` | 合并替换 | SM-4 sealed `TerminalOutcome.Kind`（completed/failed/cancelled/superseded）；不回写为 SM-2 decision 字段 |
| Run state notification 的 Runtime state/action/focus/repair 镜像 | 缩减 | SM-1 state + opaque runtime revision/ref；详细 Runtime projection 归 Orchestrator |
| delivery `Diagnostic string` | 结构化 | concrete Result 内的 typed feedback |
| `common.ErrConflict -> CASConflict` | 删除泛化映射 | typed concurrency/semantic result |
| candidate/dispatch/execution/assessment/governance/delivery 独立 state | 保留 | 各自 owner/事务，不全局合并 |
| candidate document framing disposition | 保留 | LLM owner 的纯 request observation；不选择 continuation/retry/attempt |
| SM-1 `RuntimeTerminationRecord.Kind` | 保留 | Run owner-local administrative intent/record；delivery 时映射为 sealed outcome，不跨 owner 重复持久化 |

## 6. 组件收口范围

### 6.1 核心必改组件

| 组件 | 收口内容 | 影响 SM |
| --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | 增加无损 feedback 值合同；删除 transition disposition/全 owner code 表/raw-message repair；合并 decision/output identity | SM-2/3/4 合同 |
| `kernel/orchestrator/biz/runtimev2` | 按 internal event + current persisted state 选择 retry/repair/governance/terminal；维护唯一 decision、candidate、receipt policy | SM-2/3 |
| `kernel/orchestrator/biz` drive/mainline/adapters | typed failure boundary；区分 CAS、semantic reject、transport、delivery；禁止字符串解析 | SM-4 |
| `blocks/run` | SM-1 notification 去 Runtime 语义镜像；aggregate opaque persistence；terminal outcome typed delivery | SM-1/4 |
| `blocks/result`、`blocks/workspace` | terminal outcome/ack 去 `RuntimeAction`；保留 owner-local transition diagnostic | SM-4 |

### 6.2 capability owner

`blocks/file`、`blocks/git`、`blocks/shell`、`blocks/mcp`、`blocks/ssh`、`blocks/web` 必须逐个完成：

1. 本 owner emitted code total inventory；
2. owner-private code 到 owner-neutral factual category 的本地映射；
3. concrete feasibility/operation Result 嵌入完整 original diagnostic；
4. `Receipt.Outcome` 唯一化，移除跨边界 `Status` 双权威；
5. unknown code 在 owner boundary 返回 typed owner-contract defect，同时保留原 code/message；
6. duplicate/restart exact receipt 不改变 original diagnostic；
7. DTO 不含 Runtime action、retry、terminal、repair directive 或 aggregate mutation。

### 6.3 LLM owner

`blocks/llm` 负责：

- 原始 provider transport/protocol 反馈转换为 safe structured diagnostic；
- request/session factual category，不选择 retry/terminal；
- strict candidate DTO decode 和 provider-safe field violation；
- C4 只渲染 Orchestrator sealed correction projection；
- stable prefix 不因 original diagnostic、owner code 或迁移结果漂移。

### 6.4 投影与协议组件

`application/workorchd`、TUI、Web console、`blocks/eventstream` 只做：

- 展示 stable category、safe original message、owner code、signature 与 details ref；
- 按 workspace/run/authority/dispatch scope 查询；
- 不根据 message/code 自动 Resume、创建 ticket、选择 action 或修改 owner state；
- 不把完整 raw diagnostic 写入 provider request、memory 或无界 projection。

### 6.5 其余运行单元的全量复核

`kernel/governance`、`kernel/system`、`blocks/decision`、`blocks/approval`、`blocks/memory`、`blocks/skill`、
`blocks/soul`、`blocks/mcpregistry`、`initiators/workorchruntime` 不一定需要 Runtime DTO 修改，但必须通过扫描证明：

- 没有用通用 error/message 选择其他 owner 状态；
- 没有 duplicated action/status/outcome authority；
- concrete EventHub Result 保留 owner error code/message/scope；
- `pkg` 不包含 transition/retry/terminal 选择；
- Initiator 不持有业务反馈或 Runtime policy。

## 7. 唯一串行实施队列

同一时刻只有一张卡可以 `in_progress`。每张卡必须同时完成代码、producer/consumer 测试、静态门禁与文档同步。

| 顺序 | 卡片 | 状态 | 主要内容 | 退出条件 |
| --- | --- | --- | --- | --- |
| 1 | SMO-C1 无损反馈基础合同 | completed | `RuntimeFeedback/OriginalDiagnostic/FieldViolation`、clone/normalize/hash、安全边界 | exact round-trip；normalization 不覆盖 original；禁止字段不进入 aggregate/audit |
| 2 | SMO-C2 capability result 唯一化 | completed | 六个 capability owner 本地分类、receipt outcome 唯一化、unknown code typed fail-fast | owner emitted-code total matrix；跨 EventHub 原始诊断不丢失；无 Status/Outcome 漂移 |
| 3 | SMO-C3 candidate/request 收拢 | completed | LLM factual result、SM-3 correction policy owner 化、audit-safe/provider-safe 双投影 | pkg 无 correction disposition matrix；attempt 0/1、incomplete/provider/contract 路径完整 |
| 4 | SMO-C4 failure/recovery 收拢 | completed | disposition 迁入 Orchestrator、typed drive error、CAS 与 semantic conflict 分离 | pkg 无 retry/terminal policy；semantic conflict 零 CAS retry；unknown 不默认 retry |
| 5 | SMO-C5 decision/terminal 唯一化 | completed | 删除持久化 Action/Output.Kind；terminal outcome；Run/Result/Workspace contract 更新 | aggregate 仅一份 decision；terminal owner 不理解 RuntimeAction；旧 schema fail-fast |
| 6 | SMO-C6 projection、22 组件与静态门禁 | completed | UI/eventstream safe projection；全 22 单元 pkg/contract scan；文档权威同步 | 无 error-text routing、跨 owner action、通用 Envelope 或原始反馈丢失 |
| 7 | SMO-C7 restart/concurrency/fresh live | completed | fresh data、crash points、多 workspace、正向收敛、terminal delivery | 第 10 节全部通过，无人工 Resume/数据修改/兼容 fallback |

不得跳过 C1/C2 直接删除旧分类，否则会在迁移期间再次丢失 owner 原始反馈；不得在 C5 前通过 UI/renderer 适配
掩盖三份 decision 权威。

## 8. 每张卡的具体修改与测试

### SMO-C1：无损反馈基础合同

修改：

- 在 `workorchruntimecontract` 定义 owner-neutral 纯值 DTO，不定义 topic/Command/Result；
- `OriginalDiagnostic.Normalized()` 返回 canonical comparison copy，原值由 `CloneOriginal()` 或等价路径保留；
- 明确 field violation、target/revision、details ref/hash、redaction/truncation 规则；
- 为 candidate payload、contract diagnostic、validation diagnostic、owner failure 建立无损转换。

测试：

- JSON/persistence round-trip 前后 original class/code/message/order 一致；
- allowed/required/forbidden、field path、actual 不因 summary 生成丢失；
- truncated 必有 byte/hash/ref 或明确 omitted reason；
- credential、endpoint、raw provider body、raw prompt 不进入 aggregate/audit DTO；
- clone 后 slice/map 不共享底层数据。

### SMO-C2：capability result 唯一化

修改：

- 每个 owner 建立私有 emitted-code registry/total mapping；
- feasibility Result 直接携带 disposition/category/diagnostic；
- operation Result 以 receipt outcome 为唯一正式结果；
- adapter 技术字段留在 owner-private DTO，跨 EventHub 只返回 typed receipt/feedback；
- 删除共享全 owner mapping。

测试：

- File/Git/Shell/MCP/SSH/Web 每个 emitted code 恰有一个 factual category；
- unknown owner/code、跨 owner code typed reject，原 code/message 仍可审计；
- accepted receipt 不携带 failure，rejected receipt 必有 original diagnostic；
- `Outcome=no_op` 不再携带第二个 NoOp 布尔；read-only accepted 与 durable no-op 可区分；
- `Status=accepted + Receipt.Outcome=rejected` 等状态结构上不可表达；
- duplicate/restart receipt hash 与 diagnostic hash 稳定。

### SMO-C3：candidate/request 收拢

修改：

- Candidate evaluator 仅输出完整结构诊断，不输出 retry/terminal disposition；
- SM-3 internal reducer 根据 manifest、attempt ledger、diagnostic category 选择唯一 correction 或 terminal；
- incomplete document/provider transport 不创建 candidate attempt；
- owner infeasible 不进入 candidate correction；
- request audit 保存 permitted hashes/disposition，C4 只使用 provider-safe projection。

测试：

- strict wire、unknown field、parameter、target、effect、coverage、replay first-failure 顺序；
- correctable attempt 最多一次，第二次 complete rejection typed terminal；
- owner infeasible 零 attempt-1；
- provider 401/config 与 transient/incomplete 使用各自 factual path；
- attempt 0/1 stable prefix 不漂移；原始安全 diagnostic 不因 provider projection 丢失。

### SMO-C4：failure/recovery 收拢

修改：

- 删除共享 `RuntimeFailureDisposition`；
- Orchestrator 为不同 internal event 定义 owner-local transition table；
- repository CAS 返回携带 expected/observed revision 的 typed conflict；
- semantic transition reject、stale identity、duplicate/no-op 使用各自 typed result；
- repair 只消费 typed gap/category，删除 message substring 匹配。

测试：

- 只有 expected/observed revision 漂移进入 CAS reload；
- permanent contract reject、stale candidate、delivery mismatch 不消耗 CAS budget；
- transport、operation recovery、delivery 各用独立 ledger；
- unknown category/internal defect fail-fast，零 provider/ticket/effect；
- 相同 revision/signature 无进展在下一 provider call 前终止，不在进程内空转。

### SMO-C5：decision/terminal 唯一化

完成证据（2026-07-29）：

- 已完成 SM-4 terminal delivery identity 的收拢：`Result -> Workspace -> Run` 的 concrete command、delivery identity 与 acknowledgement 只携带同一份 `TerminalOutcome{Kind, Code, Summary}`；下游不再接收或解释 `RuntimeAction`、`AdministrativeKind`、分散的 terminal code/summary。
- 已完成 Run state notification 的收拢：`blocks/run` 仅发布 SM-1 lifecycle 字段、opaque `RuntimeStateRevision/RuntimeStateRef` 与 causation；Workspace projection、Eventstream 和 process metrics 不再从该 notification 消费 convergence/action/focus/repair/authority/dispatch 语义。
- 已完成 canonical decision 迁移：aggregate、authority snapshot、authority transaction、candidate manifest/context 只持久化 `ConvergenceState` 与 output constraints；`RuntimeAction` 与 `RequiredOutput.Kind` 已从正式 identity 删除。
- 已完成 schema fence：Runtime aggregate/activation schema 已升级为 v3；v2 及其他非当前 schema 在 contract validation、bootstrap 与 Run-owner load 边界统一返回 typed `ErrUnsupportedRuntimeStateSchema`，不做双读、双写、默认补字段或 in-place migration。
- 已补齐 C5 回归：序列化断言 aggregate 不含 `Decision.Action`、`RequiredOutput.Kind` 或 delivery-only terminal outcome；authority identity 对 canonical state/semantic output 变化敏感、对 derived focus wording 不敏感；`make verify-boundary-contract` 已同步至 canonical convergence-state/effect source。

修改：

- aggregate、authority、transaction、manifest context 改用 canonical `ConvergenceState`；
- `RuntimeAction` 与 `RequiredOutput.Kind` 从 persisted identity 删除；
- `RuntimeDecision` 只保留 canonical state + terminal code/summary；SM-4 seal 的 `TerminalOutcome.Kind` 替代
  terminal action 与独立 administrative kind；
- Run notification 只保留 SM-1 state、runtime opaque revision/ref 和 causation；
- Run recovery inventory 同样只保留 SM-1 state、RuntimeStateRecord presence 与 opaque revision/ref；SM-4 recovery
  category 由 Orchestrator typed load 后 owner-local 推导；
- Orchestrator projection 单独提供 focus/decision/repair 只读详情；
- bump schema/revision，旧 aggregate typed reject。

测试：

- 每个 state 的合法 output constraints 和禁止 executable/effect 矩阵；
- Runtime aggregate 序列化结果不存在独立 Action/Output.Kind/terminal Kind；delivery ledger 只存在一份 sealed
  `TerminalOutcome.Kind`；
- Authority ID 对真实 decision/output change 敏感，对派生文案不敏感；
- Result→Workspace→Run terminal delivery 使用完整 outcome identity，四种 Kind 的 duplicate/stale 均幂等；
- Run 不根据 Orchestrator decision 字段选择 SM-1 以外状态。

### SMO-C6：projection、22 组件与门禁

当前进度（2026-07-29）：

- 已将 Runtime V2 读模型的伪 `CurrentAction`/`NextActionKind` 收拢为 canonical
  `ConvergenceState`；遗留 coverage 视图仅能以 `SuggestedNextAction` 作为展示提示，TUI/Web/API
  不再把该字段表述为可执行动作。
- heartbeat 已删除对 `Run.FailureReason` 的文本前缀路由，改为只读取持久化 Runtime V2 的
  `FailureClass` 与 sealed `TerminalCode`；原始 message 只展示，不参与 retry 语义。
- Runtime V2 audit 已投影 `RuntimeOperationReceipt.Feedback` 的稳定分类和完整安全
  `OriginalDiagnostic` 元数据（owner/code/message/signature/scope/details ref/hash），不投影 raw
  provider/tool body，也不允许 audit 回写 Runtime。
- `verify-boundary-contract.sh` 已对 1 Initiator、4 Modules、17 Blocks 的全部 `pkg/` 运行 transition
  purity scan，并新增 projection 的 duplicate-action、error-text routing、reverse-drive 与 feedback-chain
  静态门禁。

修改：

- TUI/Web/API 展示 stable category 与 safe original diagnostic；
- eventstream 保存有界 typed projection，不保存禁止 raw payload；
- `verify-boundary-contract.sh` 增加第 9 节扫描；
- 同步 core、Orchestrator readme、request-cache note、组件矩阵、active overview 和 current-unclosed matrix。

测试：

- TUI/Web 不从 message/code 自动 Resume、ticket、action；
- 原始安全 feedback 在 owner→EventHub→aggregate/audit→projection 全链路一致；
- details ref 受 workspace/run/owner scope 限制；
- projection 丢失/失败不改变 Runtime state；
- 22 个单元逐个通过 pkg purity 与 concrete Result scan。

### SMO-C7：restart、concurrency 与 fresh live

当前进度（2026-07-29）：

- file-backed crash/restart matrix 与 managed fresh-data `verify-ci-mvp-file` 已覆盖 current-schema 的
  candidate/receipt/terminal-delivery 恢复，不以 Resume、运行数据修改或兼容 fallback 继续已持久化 aggregate。
- Runtime V2 fixture 已只从 sealed C3 `requiredState` 派生候选 payload；provider fixture 不再读取退休
  `RuntimeAction`/`CurrentAction`/`NextActionKind`。
- `verify-runtime-v2-c7-scripted.sh` 将 mock-provider request、fresh fixture、terminal detail、receipt、audit
  与 safe diagnostic 固定保存在 managed artifact root 的 `llm-verification/`；不得再把本轮 C7 证据散落到
  无关联的 `mktemp` 目录。fixture 仍只以 sealed C3 manifest 生成 operation，健康探针只使用独立的无副作用
  response，不能成为正式 candidate 的 contract fallback。
- `verify_runtime_v2_c7_restart_harness.py` 提供隔离的 deployed crash producer：同一 fresh data round 只经
  公共 activation API 生成三个 Run，依次停在 C4 provider-submitted/no-dispatch、SM-3 sealed-and-claimed
  operation 与 SM-4 terminal-result-delivery-pending。同步 provider/owner 调用会占用一个驱动 lane，因此每类
  边界使用独立 crash/restart cycle，不能人为堆叠成一个 hub 饱和场景；三个 cycle 仍共享本轮 fresh data、binary、
  provider 和 artifact root。它只控制 disposable provider/process 及本轮 Result
  目录的短暂外部 I/O 故障，不写入 runtime record、不加入 production test hook；恢复后调用既有只读
  `verify-runtime-v2-restart-recovery-live.sh`，并比较 provider request ledger，禁止已知 request 重放。
- `active_no_dispatch` 恢复只能形成 `provider_submission_unknown` 的安全终态：candidate attempt 与 dispatch
  计数均保持不变；`active_sealed_dispatch` 必须保留 crash 前的 exact `ExecutionKey + Fence`，并以
  `outcome_unknown` 收敛，禁止在 release 后重放 claimed Shell side effect；`terminal_delivery_pending` 必须保留
  sealed authority、terminal code、candidate/dispatch 计数，并完成 Result → Workspace → Run acknowledgement。
- `make verify-runtime-v2-c7-scripted` 已在 fresh managed data 中通过：先以两个 workspace 的并发 activation
  跑通 state-change、validation、assessment 与 terminal delivery，再运行上述三个独立 crash/restart cycle。
  该脚本保留每轮的 scoped request ledger、safe diagnostic、receipt、terminal 与 acknowledgement；活动文档只保留
  可复现命令，不记录临时 artifact 路径。
- 同一最新构建的 managed `verify-ci-mvp-file`、`make verify-boundary-contract` 与 runtime-isolation gate 已通过。
- `observe-workorch-runtime-closure.sh` 已只读取 `convergenceState` 做只读 preflight audit，不再将
  `nextActionKind` 作为推进行为或断言依据；原始反馈仍仅用于审计展示。
- 已从活动 Make 验收入口删除 V1 `executionGraph`/`artifactPlan`/`artifactChunk` 的 incremental-artifact
  verifier。其协议不能作为 Runtime V2 candidate/dispatch 成功标准。

## 9. 静态门禁

`scripts/verify-boundary-contract.sh` 最终必须检查：

1. `internal/pkg/workorchruntimecontract` 不存在 failure→retry/repair/governance/CAS/terminal mapping；
2. 不存在集中式全 owner `owner/code -> category` 表；
3. 不存在按 error/failure message substring 选择 action、target、retry、ticket 或 terminal；
4. capability EventHub Result 不携带 Runtime action/terminal/retry/aggregate mutation；
5. operation 正式结果不存在 `Status` 与 `Receipt.Outcome` 双权威；
6. Runtime aggregate/authority/transaction 不持久化重复 Action/Output.Kind；
7. Run/Workspace/Result terminal delivery 使用 typed outcome，不解释 Runtime action；
8. owner notification/projection 不反向驱动 Runtime；
9. original diagnostic 转换不得只留下 summary 或覆盖 original code/message；
10. EventHub 仍为 owner-defined concrete Command/Data/Result，无 Envelope/map/any/JSON/reflect bridge；
11. 所有 workspace-scoped feedback 携带 workspace/run 及适用 authority/dispatch/operation identity；
12. provider raw body、credential、endpoint、raw prompt 不进入 aggregate/audit/projection/memory。

禁止用注释白名单长期跳过规则。确属 owner-private adapter 字段必须通过目录/类型边界证明不会进入 EventHub。

## 10. 强制验证矩阵

### 10.1 合同与原始信息保真

- stable category 与 original class/code/message 同时存在；
- normalization、clone、hash、persistence、EventHub round-trip 不改变 original；
- unknown code 保留原始反馈并 fail-fast，不默认 CAS/retry；
- provider-safe projection 删除敏感字段，但 persisted audit-safe diagnostic 仍完整；
- truncated/omitted/redacted 均有明确元数据，不伪装为完整内容；
- details ref/hash 在 duplicate/restart 后稳定且 scope 可验证。

### 10.2 四状态机

- SM-1 不读取派生 Runtime action 选择 lifecycle；
- SM-2 只根据 typed fact/category、coverage/evidence 和 current persisted state 选择 decision；
- SM-3 只根据 persisted session/attempt/manifest/diagnostic seal correction/dispatch；
- SM-4 只根据 typed recovery/delivery fact 处理 reload/re-drive/ack，不重建 authority；
- 每个外部结果都先成为 owner-local internal event，再发生至多一次 CAS；
- duplicate、stale、乱序、CAS 前中断、CAS 后响应丢失均零重复副作用。

### 10.3 多 workspace 与恢复

- singleflight、background context、diagnostic/details ref 均包含 workspace/run scope；
- 两个 workspace 同时 candidate、receipt、recovery、delivery 不串 authority/diagnostic；
- restart 后 exact request/receipt/ack 只继续未完成提交，不重发已知 provider/durable effect；
- `active_no_dispatch`、`active_sealed_dispatch`、`terminal_delivery_pending` 自动恢复；
- 不出现 `active + nonterminal + no scheduled drive`。

### 10.4 验证命令

按卡片先执行受影响包，整轮至少执行：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/blocks/{file,git,shell,mcp,ssh,web,llm}/... -count=1
go test ./internal/modules/blocks/{run,result,workspace}/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
make verify-boundary-contract
go test -race ./internal/modules/kernel/orchestrator/... ./internal/modules/blocks/{llm,file,shell,run,workspace,result}/... -count=1
go test -p 1 ./... -count=1
go vet ./...
make verify-ci-mvp-file
```

## 11. Fresh deployed live 退出条件

使用最新 build、fresh data、至少两个并发 workspace。不得人工修改 runtime data、Decision、Approval、schema，
不得以重复 Resume、提高 budget 或放宽 decoder 作为补偿。

必须连续证明：

1. activation 自动进入 active 并封存 ready provider/model；
2. candidate request 的 stable prefix 稳定，complete/incomplete/provider failure 分流正确；
3. candidate contract violation 最多一次有界 correction，original diagnostic 在 UI/audit 可追溯；
4. owner infeasible/stale/prerequisite/operation reject 使用 owner factual category，不进入错误 correction/CAS；
5. durable operation 具有唯一 receipt outcome、ExecutionKey/Fence，重启不重复副作用；
6. semantic conflict 不消耗 CAS reload，真实 revision conflict 自动 reload/re-drive；
7. validation/assessment/repair 只按 typed gap 推进，不解析 stderr/error text；
8. governance 仅由真实 typed dependency 产生，不出现无信息 ticket；
9. terminal 以 typed outcome 完成 Result→Workspace→Run acknowledgement；
10. TUI/Web/eventstream 展示 safe original feedback，但不反向驱动；
11. 两个 workspace 的 state、diagnostic、request、receipt、delivery 完全隔离；
12. workspace 能持续推进到 completed、typed business terminal 或真实 governed wait，不出现合同违例循环、
    revision-only 空转或 active 静止。

## 12. 完成定义与文档纪律

`RUNTIME-SMO` 只有同时满足以下条件才可关闭：

- SMO-C1～C7 全部完成，不存在部分 producer/consumer 双合同；
- `workorchruntimecontract` 只剩 DTO、合同、纯无状态 helper；
- 四状态机每个 transition 只有一个 owner-local reducer/CAS 入口；
- 状态机分类已收拢，所有安全原始合同反馈全链路保真；
- 22 个运行单元的 concrete EventHub 合同、pkg purity、scope 与错误传播均通过；
- targeted、race、全仓、CI-safe deployed 与 fresh live 验证通过；
- core contract、request-cache note、Orchestrator readme、组件矩阵、current-state/unclosed 与 active overview
  对同一最终模型无冲突。

任一卡完成时必须记录：影响 SM、owner、canonical field、derived field、original diagnostic、合法/禁止迁移、
测试命令和 fresh/restart 证据。已完成过程移入 `docs/90-archive/`，active 文档只保留当前未关闭队列，不保留
workspace-specific 日记或临时补丁说明。
