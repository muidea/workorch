# Runtime Recoverable Wait Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-21

## Purpose And Ownership

本文定义无需人类业务选择、但暂时无法安全推进时的 bounded recoverable runtime wait。它不是重试循环、治理
Decision/Approval、SM-1 lifecycle state，也不是 projection long-poll。

- SM-2/SM-3 在 exact authority/request/dispatch transaction 中发现恢复条件，并由 Orchestrator private reducer seal wait。
- SM-4 持有 due-trigger、singleflight、owner probe、re-drive、deadline 和 fresh-process recovery。
- Run owner 只保持原 eligible nonterminal lifecycle，并 CAS 持久化 opaque aggregate；它不解释、解除或调度 wait。
- receiving owner 继续拥有具体 feasibility、admissibility、request recovery、receipt reconciliation 或 catalog acquisition fact。
- Web/TUI/workorchd 只显示投影；普通 Resume、HTTP long-poll、SSE replay 和 projection refresh 均不是恢复 trigger。

## Stable Value Contract

每个 wait 必须以以下稳定值封存；缺少任何 required 字段都必须 fail-closed，不能退化为无期限 `active`：

```text
RuntimeRecoverableWaitRecord
  WaitID
  WorkspaceID
  RunID
  DefinitionRevision
  AuthorityTransactionID
  StateRevisionOpened
  ReasonCode
  ProbeKind
  ProbeOwner
  ProbeRequestHash
  RecoveryConditionHash
  OpenedAt
  NextEligibleAt
  Deadline
  Attempt
  MaxAttempts
  BudgetDimension
  BudgetPolicyRevision
  BudgetAllocationID
  Status
  LastProbeResultHash
```

`Status` 只允许：

```text
waiting | probe_claimed | condition_satisfied | expired | exhausted | retired
```

`ReasonCode`、`ProbeKind` 与 owner/probe 组合必须来自受版本约束的闭集。`ProbeRequestHash` 是即将发送的 exact concrete
command 的 canonical hash；`RecoveryConditionHash` 封存预期的 revision/fence/readiness/outcome 谓词，不保存可由错误文本
解释的自由条件。时间均为 reducer 封存的绝对时间；process-local timer 只负责唤醒，不能修改这些值。

同一 authority transaction、reason、probe request 和 recovery condition 只能产生一个 `WaitID`。`Attempt` 只在 matching
probe result 被 aggregate CAS 接纳时递增；唤醒、发送失败、duplicate result 和 CAS reload 本身不递增。

## Closed Probe Mapping

wait 不定义通用 `ProbeRecoveryCommand`。`ProbeKind` 必须选择 receiving owner 已定义的 exact concrete command/result：

| ProbeKind | Required owner contract | Recovery meaning |
| --- | --- | --- |
| `provider_request_session` | 原 sealed `RequestRuntimeCandidateCommand/Result` 的 request-session recovery/reconciliation | 同一 logical request 的 whole-document、submission 或 outcome recovery |
| `provider_ceiling` | LLM owner `AcquireProviderCeilingCommand/Result` | exact provider/model ceiling 再确认 |
| `operation_feasibility` | capability owner `ProbeRuntimeOperationFeasibilityCommand/Result` | 当前 catalog/revision 下 operation 是否可行 |
| `semantic_proof_feasibility` | capability owner `ProbeRuntimeSemanticProofFeasibilityCommand/Result` | 当前 proof shape 是否可行 |
| `operation_admissibility` | capability owner `AcquireRuntimeOperationAdmissibilityCommand/Result` | effect 前 admissibility/fence 再确认 |
| `cwi_reconciliation` | capability owner `QueryRuntimeCWIReconciliationCommand/Result` | 已执行 effect、receipt 或 CWI outcome 的精确 reconciliation |
| `capability_acquisition` | capability owner `AcquireRuntimeCapabilitiesCommand/Result` | owner catalog/readiness snapshot acquisition |

只有实际实现相应 concrete contract 的 owner 才能用于该组合。例如未提供 CWI reconciliation 的 owner 不能声明
`cwi_reconciliation` wait。owner result 必须携带足以验证 condition 的 typed revision/fence/status；只有 diagnostic message
或建议稍后重试不构成 recovery condition。

## Private Events And Legal Transitions

下列名称表示 Orchestrator aggregate 内的 private facts，不是 EventHub 公共状态 topic：

```text
recoverable_wait_opened
recoverable_wait_probe_due
recoverable_wait_probe_observed
recoverable_wait_condition_satisfied
recoverable_wait_expired
recoverable_wait_exhausted
recoverable_wait_retired
```

合法迁移为：

```text
no wait
  -> waiting
waiting
  -> probe_claimed
  -> expired
  -> exhausted
probe_claimed
  -> waiting
  -> condition_satisfied
  -> expired
  -> exhausted
condition_satisfied
  -> retired
expired | exhausted
  -> retired only after matching typed terminal is sealed
```

- `waiting -> probe_claimed` 必须先 CAS；只有 claim 成功的 SM-4 drive 可以发送 exact owner command。
- result 满足 sealed condition 时，`probe_claimed -> condition_satisfied` 与恢复后的 next authority fact 在同一 CAS 中提交。
- typed result 表示条件尚未满足时，只有在 deadline、attempt 和 allocation 仍允许的情况下才回到 `waiting`，并封存新的
  `NextEligibleAt`；否则进入 `expired`/`exhausted` 并 seal typed terminal。
- stale/mismatched result 只形成 bounded diagnostic，不改变 wait；duplicate result hash 为零 revision。
- wait 不能被替换为另一个 reason/probe。authority 条件变化时必须先 `retired`，再由新 transaction 独立判断是否开新 wait。

## Trigger And Recovery Rules

SM-4 只接受以下 trigger：sealed `NextEligibleAt` 到期、receiving owner 的 matching typed revision/readiness notification、或
fresh-process inventory 发现一个已到期/已满足通知条件的 persisted wait。trigger 只安排 bounded drive；不得直接把状态改为
`condition_satisfied`。

fresh-process reload 必须：

1. 从 opaque aggregate 读取 exact wait、budget allocation 和 authority state；
2. 对 `probe_claimed` 使用同一 request identity 先 reconciliation，不新建 logical call/effect；
3. 对尚未到期的 `waiting` 重新注册 process-local timer；
4. 对已到 deadline/guard 的 wait 通过 reducer seal typed terminal；
5. 不从 Eventstream、projection、Step/checkpoint 或 wall-clock timer memory 恢复 authority。

crash 位于 owner command 发送后、result commit 前时，re-drive 必须复用 sealed probe identity。owner 不能证明该 probe/effect
未发生时，状态保持 unknown 并受 deadline/budget约束，禁止盲目再次产生 physical call/effect。
matching reconciliation 仍不能确定消费时必须持久化 `still_unknown` owner result；deadline/guard 终态失败时原子封存
`exhausted` allocation 与 `failed` reconciliation，并继续在 budget projection 中保守计入 unknown amount。

## Projection

只读投影至少包含 `WaitID`、safe reason、status、probe kind/owner、recovery condition summary、opened/next-eligible/deadline、
attempt/max-attempts、budget dimension/allocation summary 和 aggregate state revision。raw command、credential、provider body、
未脱敏 diagnostic 与 owner-private fence 不得投影。

projection 必须明确区分 recoverable wait、governance wait 与 terminal failure；不得提供 clear、retry-now、extend deadline、
top-up 或修改 condition 的通用 mutation。

## Invariants And Verification

- 每个非终态 active aggregate 要么存在可执行 next transition，要么存在本合同定义的 persisted wait/governance wait；不能无后继。
- wait 必须同时具有 concrete owner contract、condition、trigger、deadline、attempt guard 和 budget allocation。
- wait 不创建 SM-1 state、Decision、Approval、provider request attempt 或 capability effect。
- probe claim、owner request、result join、condition satisfaction 和 retirement 对 duplicate/restart 均幂等。
- deadline、guard 或 budget exhaustion 必须形成 typed terminal，不能无限续期或改换 reason。
- owner/probe 组合不受支持、result 无 typed condition fact或identity/revision不匹配时 fail-closed。

必测场景包括：duplicate due trigger、通知早于/晚于 timer、claim 前后 crash、owner commit 后 response loss、stale result、
fresh-process reload、condition satisfied 与 deadline 竞态、budget exhaustion，以及投影无法解除 wait。执行预算见
[Execution Budget](execution-budget.md)，状态机总规则见 [Four State Machines](state-machines.md)。
