# Run / Process Evidence / Result / Artifact Domain Model

Status: active

Type: domain-model

Last Verified: 2026-08-16

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

- owner-local `Revision`、Run/Workspace/Goal/DoD identity 与 `DefinitionRevision`；
- `RuntimeBootstrap`、`ActivationRequestID`、priority 与 lineage；
- `RuntimeState` 和 opaque lifecycle fence；
- `ReconciliationRequestID` 与 reconciliation scope；
- `RuntimeActivationStatus`、SM-1 `State`、blocking ticket/approval；
- failure/timestamps 与 immutable `TerminalDelivery`；
- restart predecessor 上的 durable `ReplacementIntent`。

SM-1 states：`pending`、`active`、`blocked_on_decision`、`blocked_on_approval`、`completed`、`failed`、
`cancelled`、`superseded`。只有 Run owner 的 reducer 可以转换这些状态。

## Replacement Intent

`ReplacementIntent` 是 predecessor Run 的 owner-local restart transaction：

1. `pending`：已封存 immutable successor input，尚未创建 successor；
2. `successor_created`：终态投递 identity 已知且 successor 已创建；
3. `completed`：Run owner已确认 matching terminal delivery 和 aggregate revision。

它不包含 Workspace 目标状态或 Orchestrator reducer 指令。request ID、definition/bootstrap、predecessor/successor lineage
和 terminal delivery identity 共同保证重试、崩溃恢复与去重。

## Runtime Aggregate

Run 只负责 CAS 持久化/返回 opaque record；Orchestrator解释其中的 facts、focus、authority transaction、candidate
session、sealed dispatch、operation receipts、evidence ledger、governance binding、budgets 和 terminal outcome。

恢复时从 aggregate 重新 drive。Eventstream、Step、checkpoint 或 projection 不得被用于重建这些字段。

## Process Evidence

- Step 是可分页过程记录，不表示被授权的下一步。
- ToolCall/Patch/ValidationResult 是 operation 事实的审计/投影视图；正式接纳状态来自 Orchestrator ledger。
- RuntimeCheckpoint 是诊断快照，不是恢复源。
- ArtifactContinuation 支持受限的分块/组装过程；其状态不能绕过 candidate、sealed dispatch 或 evidence gate。
- stale、duplicate 和 revision-mismatched 回执不得产生第二次副作用或重复 evidence credit。

## Result

Result owner model 包含 owner revision、Workspace/Run/Goal/DoD identity、state、title/summary、source refs、
evidence/artifact refs、DoD assessment、summary detail、audit timestamps、`RuntimeCloseout` 与 immutable
`TerminalDelivery`。

Result states 为 `draft`、`accepted`、`rejected`、`superseded`。Result owner不判断下一步或重新评估 DoD；它验证
matching sealed delivery 后幂等保存 Orchestrator 已裁决的 closeout。

## Artifact

Artifact 是 Result owner 管理的正式产物元数据。内容可以位于文件、对象存储或其他受管位置；跨组件只传递
typed/logical reference。Artifact 可支持 Result，但 artifact 存在本身不等于 CWI evidence 已接纳或 DoD 已满足。

## Core Invariants

- process success 不等于 business acceptance。
- capability receipt 不等于 accepted evidence。
- projection outcome 不等于 formal Result。
- terminal delivery 顺序固定为 Orchestrator → Result → Workspace → Run。
- restart successor 只能由 Run owner transaction 创建。
- 新 definition 必须进入新 Run，不能 patch 当前 aggregate。
