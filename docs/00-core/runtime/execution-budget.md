# Runtime Execution Budget Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-09-10

## Purpose

本文定义长期 Run 的 execution-budget authority、分配、消费、恢复与产品投影。它把分散的 provider、candidate、
semantic proof、operation、repair 和 recovery budget 纳入一个确定性视图，但不建立跨 owner 的第二份消费总账。

## Ownership Model

- 配置来源由 Workspace/Run policy 和对应 capability/provider owner 管理；fresh activation 将适用 policy revision 封存进
  `RuntimeBootstrap` 或 owner snapshot。
- Orchestrator 只拥有 SM-2/3/4 内的 allocation、reservation、consumption reference、remaining view 和 guard transition。
- LLM/capability owner 只报告自身 physical call/effect 的 typed consumption fact，不修改 Runtime remaining budget。
- `RuntimeExecutionBudgetView` 是 Runtime sub-ledger 的确定性汇总；它不是独立 repository、owner、CAS 或可被 UI 写入的总账。
- monetary cost 只有 provider 能返回可靠 currency/unit/amount 时才记为 confirmed；未知成本不得由 Runtime 猜测。

## Stable Value Contracts

policy、allocation、consumption、reconciliation 与 projection 分为五个值对象；它们都是 aggregate fact/value，均不新增
repository 或 owner：

```text
RuntimeExecutionBudgetPolicy
  PolicyRevision
  Scope                 workspace | run
  Dimensions[]          RuntimeBudgetDimensionPolicy
  RestartDisposition    fresh_from_successor_policy | zero | explicit_limit
  PolicyHash

RuntimeBudgetDimensionPolicy
  Dimension
  Unit
  Limit
  MaxIncrement
  ExhaustionDisposition recoverable_wait_if_reconcilable | terminal

RuntimeBudgetAllocation
  AllocationID
  Dimension
  Unit
  LogicalConsumptionID
  ScopeRef
  PolicyRevision
  LimitAtAllocation
  ReservedAmount
  MaxIncrement
  State                 reserved | confirmed | released | unknown | exhausted
  OwnerFactRef
  AllocationHash

RuntimeBudgetConsumptionFact
  ConsumptionID
  AllocationID
  Owner
  LogicalConsumptionID
  PhysicalConsumptionID
  Unit
  ConfirmedAmount
  UnknownAmount
  Currency
  MonetaryAmount
  OwnerResultRef
  OwnerResultHash
  ConsumptionHash

RuntimeBudgetReconciliationRecord
  ReconciliationID
  AllocationID
  ProbeKind
  ProbeOwner
  ProbeRequestHash
  ExpectedOwnerFence
  Status                 pending | confirmed | released | still_unknown | failed
  OwnerResultHash
  ReconciliationHash
```

所有整数数量均非负；`Limit`/`LimitAtAllocation`、token、call、attempt、operation 和时间单位必须是精确整数。
monetary amount 使用 typed decimal minor-unit + currency，不允许 binary float；provider 未返回可靠 unit/currency 时保持
unknown。`Dimensions[]`、allocation 和 consumption 按 canonical identity 排序后 hash；重复 dimension、unit mismatch、空 logical
identity、跨 policy revision join 或 amount overflow 均 fail-closed。

`LogicalConsumptionID` 表示一次被 Runtime 授权的语义消费；`PhysicalConsumptionID` 表示 owner 实际调用/effect。一个 logical
consumption 可因合同允许的 recovery 关联多个 physical facts，但每个 physical identity 只能归属于一个 allocation，且只能
confirmed 一次。

## Budget Dimensions

至少独立治理：

| Dimension | Authoritative fact |
| --- | --- |
| provider logical/physical calls | persisted request session + physical-call ledger |
| provider input/output tokens and known cost | LLM owner typed usage fact bound to physical call |
| whole-document recovery | request-recovery ledger |
| complete candidate correction | SM-3 attempt 0/1 ledger |
| semantic proof generation/correction | semantic generation ledger |
| capability operation/effect | sealed operation + owner receipt consumption |
| operation outcome recovery | SM-4 recovery ledger and exact fence |
| same-focus repair/CWI reopen | SM-2 focus repair/reopen ledger |
| reasoning effort | sealed request identity and selected provider ceiling |

Context inventory、Definition partition、active window 和 semantic batch capacity 仍由
[Definition Context Strategy](../definition-convergence/context-strategy.md) 管理。容量上限和 execution consumption 不得复用
同一字段或错误码。

输入评审 v2 的一次性信息补充以同一 Run 内原 CWI/check 为身份，从既有 semantic generation 的
`input_review_information` retirement 与 exact Decision resolution 反证；不新建计数 owner。subject 内容、路径、native、
recipe/catalog revision 和进程重启不能刷新额度。只允许把 resolution facts 投影到其原 exact subject；预算消费范围
不能被误用为把事实移植到新 subject 的权限。v1 保持封存的 exact-subject 预算，history 容量和 provider/request 预算仍独立。

## Allocate Before Call Or Effect

provider submission、semantic generation、candidate correction、capability dispatch 或 recovery replay 前必须能够从 persisted
facts 确定：

- exact dimension、scope、policy revision 和 limit；
- logical consumption identity；
- confirmed consumed、reserved/unknown 和 remaining；
- 本次允许的最大增量；
- exhaustion disposition。

无法在外部动作前完成 allocation 时保持零 physical call/effect 并 fail-closed。reservation 只授权一个 exact logical
consumption；不得作为其它 request/operation 的余额。

allocation 的合法迁移为：

```text
no allocation -> reserved
reserved -> confirmed | released | unknown
unknown -> confirmed | released | exhausted
confirmed | released | exhausted -> terminal value
```

- `reserved` 必须与授权 external call/effect 的 request/dispatch seal 在同一 aggregate CAS 中出现。
- owner 明确证明未发生 physical call/effect 时才可 `released`；transport error 本身不是该证明。
- owner 返回 matching typed usage/receipt 后才可 `confirmed`；确认量超过 reservation/max increment 必须 fail-closed 并保留
  owner fact供审计，不能静默截断。
- external outcome 或消费发生性未知时进入 `unknown`，随后只允许 exact reconciliation 或本合同定义的 bounded wait。
- exact reconciliation 明确证明 physical consumption 未发生时，必须在同一 aggregate value 中删除 unknown fact、把
  allocation 改为 `released` 并把 reconciliation 改为 `released`；reconciliation 仍无法确定时记录 `still_unknown`，不得
  释放余额；deadline/guard 终态失败时改为 `exhausted + failed` 并继续把 unknown amount 计入不可用余额。
- allocation 创建前发现 limit 不足时直接返回 typed exhaustion 且不创建 allocation；`exhausted` 不是把尚未执行的
  `reserved` reservation 伪造成已消费，而是 unknown consumption 无法再安全恢复的终态。
- allocation 不得在 confirmed 后 top-up、改 dimension、换 logical identity 或重新打开。

## Owner Fact Join Contract

本合同不发明通用 budget EventHub envelope。每个 external action 的 receiving owner 通过其现有 concrete typed result 返回
usage/receipt或稳定 owner result reference：

| Action | Join source | Required identity |
| --- | --- | --- |
| LLM/provider invocation | matching LLM-owner request result/audit + provider request-session physical-call ledger | workspace/run、logical request、physical call、explicit providerSubmitted、physical response mode、provider result/usage hash |
| semantic/candidate document | SM-3 complete-document evaluator fact | transaction、document kind、attempt、document hash |
| capability operation | matching `ExecuteRuntimeOperation` owner result/receipt | sealed dispatch、operation、ExecutionKey/Fence、receipt hash |
| feasibility/admissibility/acquisition | matching owner concrete command/result | authority/focus、catalog revision/fence、request hash |
| operation/request recovery | matching owner reconciliation result | original allocation、logical/physical identity、expected fence |

Orchestrator private reducer把matching fact规范化为 `RuntimeBudgetConsumptionFact` 并与 authority transition原子CAS。
owner result若没有足以确认消费的 typed amount/identity，只能产生 `unknown`，不得解析 message、账单页面或 metrics 推测。
result mismatch、stale、duplicate或来自另一Run/policy的fact只能进入bounded diagnostic，不能消费或释放当前allocation。
对 provider request，`providerSubmitted=false` 的 exact LLM audit 是 release authority；`providerSubmitted=true` 确认
physical call，usage 缺失时 token allocations 保守进入 unknown；audit 缺失/不可信则整个 physical settlement 为 unknown。
请求字节数、response mode、reservation 或 transport error 文本均不能替代 submission fact。settlement 必须与对应
session/recovery/terminal transition 同一 aggregate CAS，不能先记账再迁移或先迁移再记账。

## Consumption And Duplicate Semantics

- confirmed provider usage 绑定 `LogicalRequestID + PhysicalCallID + usage hash`；duplicate result不重复扣减。
- candidate attempt 只由 SM-3 对已结算且 framing 为 one complete JSON object 的 document 创建，再执行 strict local
  schema/contract/semantic admission。incomplete/empty/invalid framing、transport recovery 与 provider terminal 均不消费
  candidate attempt；no-candidate framing 只消费 activation-sealed whole-document recovery，complete-object rejection 共用
  同一 attempt 0/1 上限。
- manifest-declared owner candidate-conformance 是已存在 attempt 内的 side-effect-free read/restart fence，不分配独立
  provider、capability、repair 或 recovery budget；`nonconformant` 只消费该 attempt 已有的唯一 correction slot，duplicate/restart
  复用 exact `LogicalRequestID + operation + manifest` fact 且零重复消费。
- capability consumption绑定 sealed dispatch、operation、ExecutionKey/Fence 和 receipt；duplicate receipt不重复扣减。
- outcome unknown 保留为 reserved/unknown，不能假定未消费，也不能无条件按 confirmed 重复扣减。
- accepted owner resource change、projection refresh、duplicate drive、stale diagnostic 和纯 CAS reload不消费业务执行预算。
- 同一 CWI reopen/repair 的消费随 aggregate持久化；process restart不重置计数。

## Exhaustion And Recoverable Wait

budget exhaustion 本身不创建 Decision/Approval。处理只允许：

1. remaining足以执行 exact transition：继续并原子持久化消费；
2. consumption暂时 unknown，且存在 owner reconciliation/recovery command、deadline和重新 drive触发源：进入
   bounded recoverable runtime wait；
3. limit已耗尽、recovery guard耗尽或无法证明安全继续：typed fail-closed terminal。

waiting record必须符合 [Recoverable Wait](recoverable-wait.md)，并绑定本合同的 exact allocation、dimension、policy revision
和remaining/unknown fact。它属于SM-2/3/4 aggregate事实，不新增SM-1 lifecycle state。

## Recovery And Restart

- fresh-process recovery从aggregate的budget sub-ledgers重建同一确定性view，不读取projection、metrics或provider文本。
- crash发生在external submission/effect之后、consumption commit之前时，必须先使用exact owner reconciliation/fence确定结果；
  不得直接再执行。
- duplicate/stale/reordered consumption fact只能幂等join或进入diagnostic。
- Resume沿用当前Run封存的policy和已消费事实。
- Restart是否获得新budget由fresh Workspace/Run policy明确决定；successor不继承predecessor aggregate计数，但lineage/audit
  必须能分别解释两次Run的预算。

reconciliation 先持久化 `RuntimeBudgetReconciliationRecord{Status=pending}` 和对应 recoverable wait/probe claim，再发送
owner-specific concrete command。matching result 与 allocation状态、consumption fact、wait迁移在一个aggregate CAS中提交。
crash后复用 `ReconciliationID + ProbeRequestHash`；禁止建立新logical consumption来探测旧消费。

## Projection

稳定只读对象为：

```text
RuntimeExecutionBudgetView
  WorkspaceID
  RunID
  PolicyRevision
  StateRevision
  Dimensions[]
    Dimension
    Unit
    Limit
    Confirmed
    Reserved
    Unknown
    Remaining
    Exhausted
  ActiveWaitID
  TerminalReasonCode
  ViewHash
```

`Confirmed` 是该 dimension 已确认 physical facts 的去重和；`Reserved` 不含 unknown；`Unknown` 单独显示；
`Remaining=max(0, Limit-Confirmed-Reserved-Unknown)`，因此 UI 不能把 unknown 当作可用余额。view按dimension canonical排序并绑定
aggregate `StateRevision`；projection不得提供“补余额”“清零消费”或直接恢复Run的写入口。

## Invariants

- 每项消费只有一个authoritative sub-ledger和logical identity；
- 汇总值必须从sub-ledger纯派生且满足`confirmed + remaining <= limit`，unknown单独显示；
- policy/allocation/consumption/reconciliation/view identity与hash在clone、persistence、reload和projection中保持稳定；
- allocation先于external call/effect；
- duplicate/restart不重复消费；
- unknown consumption不被解释为zero；
- budget、retry、repair、recovery和context capacity保持独立；
- exhaustion不能伪造成governance dependency；
- projection/metrics/provider billing页面不回写Runtime authority。

## Verification

验证见 [Core Runtime Verification](verification.md)，provider request identity见
[Runtime Request Provider Contract](../runtime-request-provider-contract.md)。
