# Run / Process Evidence / Result / Artifact Domain Model

Status: normative

Type: domain-model

Last Verified: 2026-08-25

## Ownership Map

| Object | Sole owner / authority | Notes |
| --- | --- | --- |
| Run lifecycle | `blocks/run` | SM-1、bootstrap binding、resume/restart、terminal fence |
| Runtime aggregate | `kernel/orchestrator` | SM-2/3/4 semantic authority；opaque record由 Run repository物理持久化 |
| Step/checkpoint process records | Run-side persistence/read model | 诊断与观测，不是 scheduler authority |
| Operation evidence | `kernel/orchestrator` | receipt judgement、accepted/rejected ledger、coverage binding |
| Concrete operation receipt | executing capability owner | 只陈述事实 |
| Result / Artifact | `blocks/result` | formal outcome 与产物元数据 |
| Projection types | `workorchd` | 可重建展示对象 |

## Run

当前 Run owner model 的稳定字段组包括：

- owner-local `Revision`、Run/Workspace/Goal/DoD identity、`DefinitionRevision` 与 immutable `StartupInputHash`；
- `RuntimeBootstrap`、`ActivationRequestID` 与 lineage；caller-controlled Run priority 不属于 activation、replacement 或
  persisted Run identity，acceptance/context/Soul 等独立业务排序也不能成为执行调度权威；
- `RuntimeState` 和 opaque lifecycle fence；
- `ReconciliationRequestID` 与 reconciliation scope；
- SM-1 `State` 与 blocking ticket/approval；Run 不保存 Runtime schema-validity/activation status；
- failure/timestamps 与 immutable `TerminalDelivery`；
- restart predecessor 上的 active durable `ReplacementIntent` 与 immutable rejected-intent retirement history。

SM-1 states：`pending`、`active`、`blocked_on_decision`、`blocked_on_approval`、`completed`、`failed`、
`cancelled`、`superseded`。只有 Run owner 的 reducer 可以转换这些状态。

`recoverable_runtime_wait`不是SM-1 state。Run保持eligible nonterminal lifecycle；wait identity、reason、recovery
condition、trigger/deadline、budget和guard属于Orchestrator aggregate，由SM-4安排persisted re-drive。Run/projection不得
把它伪装为Decision/Approval blocker或要求普通Resume才能恢复。

## Replacement Intent

`ReplacementIntent` 是 predecessor Run 的 owner-local SM-1 subordinate restart transaction，不是第五个状态机：

1. `pending`：普通 restart 已封存 immutable successor input，尚未创建 successor；
2. `pending_clarification_resolution`：Definition-gap intent 已绑定 ticket 与 predecessor/successor startup-input lineage，
   等待 Decision owner acknowledgement；
3. `clarification_resolution_bound`：matching dedicated resolution 已按 exact binding hash 回绑，可以开始 administrative
   supersede/delivery；
4. `clarification_resolution_rejected`：Decision 返回 exact stale/scope conflict；该 intent 不可 delivery；
5. `successor_created`：终态投递 identity 已知且 successor 已创建；clarification intent 必须仍携带 bound resolution；
6. `completed`：Run owner已确认 matching terminal delivery 和 aggregate revision；clarification binding 不得在此时丢失。

它不包含 Workspace 目标状态或 Orchestrator reducer 指令。request ID、definition/bootstrap、predecessor/successor lineage
和 terminal delivery identity 共同保证重试、崩溃恢复与去重。只有不同 RequestID 的用户显式 Restart 可以把一个
scope-mismatch `clarification_resolution_rejected` intent 移入 predecessor 的 immutable retirement history 并封存新 intent；
`ticket_already_bound` rejection 是不可由 Restart 覆盖的 owner invariant breach。相同 RequestID重用、自动 supersede以及
替换bound/deliverable/completed intent均为typed conflict。

administrative cancelled/superseded 先由 Run-private event claim intent；Orchestrator 只在 SM-4 opaque aggregate 中保存并重投
exact `RuntimeTerminationRecord` handoff。Result、Workspace 与 Run 各自以 private event确认自身 delivery；最终 Run terminal
只能由 Run-private SM-1 delivery event持久化，handoff record本身不属于 Run lifecycle state。

## Runtime Aggregate

Run 只负责 CAS 持久化/返回 opaque record；Orchestrator解释其中的 facts、focus、authority transaction、candidate
session、sealed dispatch、operation receipts、evidence ledger、governance binding、budgets 和 terminal outcome。

aggregate中的execution-budget sub-ledgers分别拥有logical consumption；`RuntimeExecutionBudgetView`只从这些facts纯派生
limit、confirmed、unknown/reserved、remaining和exhausted，不拥有独立CAS或写入口。

恢复时从 aggregate 重新 drive。Eventstream、Step、checkpoint 或 projection 不得被用于重建这些字段。

## Process Evidence

- Step 是可分页过程记录，不表示被授权的下一步。
- ToolCall/Patch/ValidationResult 是 operation 事实的审计/投影视图；正式接纳状态来自 Orchestrator ledger。
- RuntimeCheckpoint 是诊断快照，不是恢复源。
- ArtifactContinuation 支持受限的分块/组装过程；其状态不能绕过 candidate、sealed dispatch 或 evidence gate。
- stale、duplicate 和 revision-mismatched 回执不得产生第二次副作用或重复 evidence credit。

## Result

Result owner model 包含 owner revision、Workspace/Run/execution-lineage、Definition revision、Goal/DoD identity、state、
title/summary、逐requirement/CWI coverage、accepted evidence refs、assessment、deliverable/artifact refs、适用owner
revision/snapshot/fence set及其hash、known limitations、summary detail、audit timestamps、`RuntimeCloseout` 与 immutable
`TerminalDelivery`。

Result states 为 `draft`、`accepted`、`rejected`、`superseded`。Result owner不判断下一步或重新评估 DoD；它验证
matching sealed delivery 后幂等保存 Orchestrator 已裁决的 closeout。

`RuntimeResultCloseout`、逐requirement/CWI三轴对象和`RuntimeOwnerRevisionSet`的唯一字段、hash及成功校验规则见
[Result Closeout And Requirement Projection](result-closeout.md)；本页不另定义兼容变体。

产品projection把coverage、assessment和evidence作为独立维度；Result owner不把`partially_satisfied`或
`indeterminate`重解释为成功，也不从global world counter判断freshness。

## Artifact

Artifact 是 Result owner 管理的正式产物元数据。内容可以位于文件、对象存储或其他受管位置；跨组件只传递
typed/logical reference。Artifact 可支持 Result，但 artifact 存在本身不等于 CWI evidence 已接纳或 DoD 已满足。

## Core Invariants

- process success 不等于 business acceptance。
- capability receipt 不等于 accepted evidence。
- projection outcome 不等于 formal Result。
- terminal delivery 顺序固定为 Orchestrator → Result → Workspace → Run。
- restart successor 只能由同一 Run owner 的 SM-1 subordinate transaction 创建。
- 新 definition 必须进入新 Run，不能 patch 当前 aggregate。
