# Orchestrator State Machine Contract Hardening Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-09

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-terminal-ledger-nextaction-dispatch-contract-hardening-plan.md`
- `runtime-validation-command-contract-authority-closure-plan.md`
- `runtime-ledger-authority-dispatch-hardening-final-closure-plan.md`

## Purpose

本文先完整冻结 WorkOrch orchestrator 相关状态机，再基于该状态机进行合同固化与状态迁移代码收口。

近期 live 验证已经证明，继续在 `runtime_gap_baseline.go`、`runtime_convergence.go`、`runtime_repair.go`、`runtime_current_action_contract.go` 等局部入口追加补丁，会形成循环修复：每轮修复一个 stale mirror 或 recovery path，下一轮又从另一个派生字段把旧合同带回 dispatch。

后续代码收口必须先满足本文状态机，不再以单个 live workspace 现象直接追加局部 gate。

## Contract Audit

- Scope:
  - `kernel/orchestrator` run lifecycle、step lifecycle、coverage/evidence ledger、convergence authority、NextAction、StagePacket、repair/retry/governance、contract violation recovery、projection readback。
- Observed state:
  - `ws-db040d24a190f6e5/run-105` 在重新部署后可以继续调度，从 12 个 step 推进到 21 个 step，但最终进入 `blocked_on_decision`。
  - 当前 focus 仍缺 `state_change` evidence，但部分 dispatch input 仍派生为 `convergenceContract.state=need_validation`、`stagePacket.nextAction=validate`，且 `requiredOutput.forbiddenKinds` 禁止 `patch/apply` / `git/write`。
  - 最终 blocker 为 `executionContext.stagePacket.requiredOutput does not match current authority requiredOutput`，说明派生合同仍可从不同镜像入口漂移。
- Expected state:
  - `terminal step result -> ledger recompute -> NextAction -> target/context sufficiency gate -> canonical StagePacket -> dispatch` 必须是单向、单入口、同源状态迁移。
  - `coverage/evidence ledger` 与 `convergenceAuthority` 是当前收敛权威。
  - `convergenceContract`、`stagePacket`、`repairActionPackage.requiredOutput`、top-level `requiredOutput`、`currentActionContract` 只能从同一次 authority snapshot 派生。
  - 旧 failed step input、旧 repair package、旧 contract violation recovery、旧 decision/replan lineage 只能进入 diagnostic lineage，不能反向驱动当前 NextAction。
- Deviation:
  - 当前实现把“权威状态”和“派生镜像”混用，导致 validation failure / repair retry / contract retry / graph continuation 仍可能各自重建局部合同。
  - 局部修复不断补“rebinding / reject / preserve”门禁，但没有把状态迁移收敛到唯一生成点。
- Owner:
  - WorkOrch core orchestrator。该问题不是 workspaceTemplate、Soul、LLM provider、语言或具体文件名特判。
- Event path:
  - command/event -> run owner state -> orchestrator scheduler -> coverage/evidence ledger -> convergence authority -> NextAction -> StagePacket/requiredOutput -> block/LLM candidate -> contract gate -> evidence ledger -> run/step/result/governance state -> projection。
- Contract gap:
  - 缺少完整状态机表驱动的单入口迁移约束。
  - 缺少“派生字段不得作为 authority source”的硬规则。
  - 缺少 NextAction 与 ConvergenceContract / StagePacket / RequiredOutput 的全矩阵一致性校验。
- Boundary check:
  - 本文只使用状态、证据类型、合同、owner、scope、幂等、治理对象，不引入模板、语言、命令、文件名或 provider 特判。

## State Machine Layers

Orchestrator 实际涉及多层状态机。后续收口必须明确当前修改属于哪一层，不能跨层直接改状态。

| Layer | State owner | Role | May drive NextAction | May mutate workspace |
| --- | --- | --- | --- | --- |
| Workspace lifecycle | workspace owner | workspace 可用性与当前 active run | no | no |
| Run lifecycle | run owner / orchestrator | run 执行、治理、终止状态 | no, only gates scheduling | no |
| Step lifecycle | orchestrator runtime | 单个 runtime/LLM/block step 的执行状态 | indirectly through evidence result | only through admitted block |
| Coverage ledger | runtime coverage owner | Goal/DoD item gap 权威 | yes | no |
| Evidence ledger | runtime evidence owner | accepted/rejected evidence 权威 | yes, through coverage recompute | no |
| Convergence authority | orchestrator convergence owner | current focus + deterministic NextAction | yes | no |
| Stage contract | derived renderer | 当前 step input 合同 | no | no |
| Contract gate | orchestrator/block adapter | 候选输出准入 | no, records evidence | no unless admitted |
| Governance | decision/approval owner | 外部信息/审批阻塞 | gates scheduling | no |
| Projection/ViewModel | workorchd projection | 展示态 | no | no |

## Canonical Object States

### WorkspaceState

Source: `internal/pkg/common/states.go`

| State | Meaning | Allowed owner transition |
| --- | --- | --- |
| `active` | workspace 可执行，可有 active run | create / restart / resume readback |
| `blocked_on_approval` | workspace 当前 run 等待 approval | run blocked on approval projects to workspace |
| `superseded` | workspace 被替代或关闭 | supersede command |

Workspace state 不表达 coverage 进度，不参与 NextAction 推导。

### RunState

Source: `internal/pkg/common/states.go`

| State | Meaning | Allowed next states |
| --- | --- | --- |
| `pending` | run 已创建但未启动，允许在调度前因治理门禁进入 blocked | `active`, `blocked_on_decision`, `blocked_on_approval`, `cancelled`, `failed` |
| `active` | runtime 可调度 step | `blocked_on_decision`, `blocked_on_approval`, `completed`, `failed`, `cancelled`, `superseded` |
| `blocked_on_decision` | 等待 decision ticket resolution | `active`, `failed`, `cancelled`, `superseded` |
| `blocked_on_approval` | 等待 approval resolution | `active`, `failed`, `cancelled`, `superseded` |
| `completed` | completion gate 接受终态 | terminal |
| `failed` | fail-fast 或不可恢复失败 | terminal |
| `cancelled` | 用户或系统取消 | terminal |
| `superseded` | 被 replacement run 替代 | terminal |

Hard rules:

- `completed` 只能由 final coverage/result completion gate 写入。
- `blocked_on_decision` 只能由 governance owner 创建 open ticket 后写入。
- `blocked_on_approval` 只能由 approval owner 创建 pending approval 后写入。
- `active` 不代表正向推进，只代表可调度。
- `blocked_on_decision` 恢复时必须先确认 ticket 已 resolved，且 resolution 只作为 diagnostic/governance input，不覆盖 convergence authority。

### StepState

Source: `internal/pkg/common/states.go`

| State | Meaning | Allowed next states |
| --- | --- | --- |
| `pending` | step 已创建，尚未进入 runtime | `executing`, `skipped`, `cancelled`, `failed` |
| `executing` | runtime worker 已接收 | `succeeded`, `failed`, `cancelled` |
| `succeeded` | step output 已通过对应合同并完成记录 | terminal |
| `failed` | step output/执行/合同失败 | terminal for this step; may trigger repair/governance |
| `skipped` | 被当前 authority 判定不需执行 | terminal |
| `cancelled` | 被 run cancel/supersede 中断 | terminal |

Hard rules:

- step 不允许先 `succeeded` 再因 decision/contract gate 改 `failed`。
- LLM/block 输出未过合同门禁时，step 只能失败并记录 rejected diagnostic evidence，不能生成 accepted result/artifact/evidence。
- state-changing side effect 必须在准入门禁通过后执行；验证/assessment 不能伪造 state-changing evidence。

### CoverageWorkItem Status

Source: `internal/pkg/workorchruntimecontract/coverage_work_item.go` and `runtime_coverage_state.go`

| Status | Meaning | Derived next action |
| --- | --- | --- |
| `open` | 尚未有足够证据 | `need_evidence` |
| `in_progress` | 当前 focus 正在收敛 | depends on missing evidence |
| `evidence_ready` | required evidence 已满足，等待 assessment | `need_assessment` |
| `satisfied` | accepted evidence + accepted assessment 满足 | none |
| `failed` | 当前 focus 有 rejected evidence 或失败面 | depends on missing evidence / governance |
| `blocked` | 需要外部信息/权限/决策 | `need_decision` |
| `skipped` | 已明确跳过 | none |
| `superseded` | 被新 definition/focus 替代 | none |

Hard rules:

- `satisfied` 只能由 accepted evidence + assessment gate 触发。
- `state_change` 缺失时，不能进入 completion。
- `validation` accepted 不能替代 `state_change` accepted。
- rejected diagnostic evidence 只影响 repair/context，不满足 missing evidence。

### EvidenceRecord

Source: `internal/pkg/workorchruntimecontract/evidence_record.go`

Kinds:

- `state_change`
- `validation`
- `assessment`
- `artifact`
- `target_snapshot`
- `decision`
- `approval`
- `diagnostic`

Statuses:

- `accepted`
- `rejected`
- `superseded`

Hard rules:

- `diagnostic/rejected` 不进入 coverage satisfied 计算。
- `target_snapshot` 只证明当前资源可安全 patch，不等价于 state_change。
- `decision` / `approval` 只解除治理阻塞，不证明 Goal/DoD 满足。

### DecisionTicketState

| State | Meaning | Allowed next states |
| --- | --- | --- |
| `open` | 等待用户或治理输入 | `resolved`, `superseded` |
| `resolved` | 已提供 resolution | terminal for ticket; run may resume |
| `superseded` | 被 replacement/replan/cancel 替代 | terminal |

Decision resolution 不拥有 NextAction，只能作为 governance evidence 或 diagnostic context。

### ApprovalState

| State | Meaning | Allowed next states |
| --- | --- | --- |
| `pending` | 等待批准 | `approved`, `rejected`, `superseded` |
| `approved` | 准许某个明确 side effect | terminal for approval |
| `rejected` | 拒绝 side effect | terminal for approval |
| `superseded` | 被 replacement/cancel 替代 | terminal |

Approval 只能批准已声明、已 scope、已绑定 target 的动作，不能改变 coverage 状态。

### ResultState

| State | Meaning | Rule |
| --- | --- | --- |
| `draft` | 候选 result | 不能完成 run |
| `accepted` | 通过 final coverage authority | 可触发 run completed |
| `rejected` | 不满足 DoD 或合同 | 只作为 diagnostic evidence |
| `superseded` | 被新 result 替代 | 不参与完成 |

## Runtime Stage Machine

Source: `internal/modules/kernel/orchestrator/biz/runtime_stage.go`

| RuntimeStage | Mode | Meaning | NextAction class |
| --- | --- | --- | --- |
| `exploration` | `open_judgment` | 探索上下文 | `assess_gap`, `plan` |
| `planning` | `open_judgment` | 生成计划或首轮图 | `plan` |
| `execution` | `strict_convergence` | 执行 state-changing/action graph | `state_change`, `repair` |
| `verification` | `strict_convergence` | 产生 validation evidence | `validate`, `assessment` |
| `acceptance` | `strict_convergence` | final DoD/result gate | `assessment`, `complete`, `fail` |
| `repair` | `strict_convergence` | 对失败 step 进行有界修复 | `acquire_target_snapshot`, `state_change`, `validate`, `repair` |
| `replan` | `structured_governance` by intent | 替换策略或新 run lineage | `plan` or `assess_gap` after refresh |
| `decision` | `structured_governance` | 等待/处理 decision | `request_decision` |
| `approval` | `structured_governance` | 等待/处理 approval | `request_approval` |

Hard rules:

- `open_judgment` 只允许规划/探索，不允许直接接受 completion。
- `strict_convergence` 必须有 current focus、requiredOutput、evidence binding。
- `structured_governance` 只能阻塞/释放执行，不可替代 coverage/evidence ledger。

## ConvergenceContract State Machine

Source: `runtime_convergence.go`

| Contract state | Meaning | Canonical NextAction | RequiredOutput rule |
| --- | --- | --- | --- |
| `need_evidence` | 缺可定位证据/目标快照/失败面 | `acquire_target_snapshot` or evidence acquisition | read-only / target snapshot; no workspace mutation |
| `need_state_change` | 缺 state-changing evidence | `state_change` | must include state-changing kind, e.g. `patch/apply` or other accepted mutating kind |
| `need_validation` | 缺 validation evidence | `validate` | validation-only; workspace mutation forbidden except scratch |
| `need_assessment` | evidence ready, 缺 assessment | `assessment` | assessment-only, no executable mutation |
| `need_governance` | 缺外部信息/权限/审批 | `request_decision` / `request_approval` | no LLM mutation |
| `blocked` | 无可自动继续路径 | `request_decision` or `fail` | governance/terminal only |

Forbidden combinations:

- `nextAction=state_change` with `contract.state=need_validation`.
- `nextAction=validate` when current focus only misses `state_change`.
- `contract.state=need_state_change` while `requiredOutput.forbiddenKinds` contains all state-changing kinds.
- `need_validation` with `mustIncludeStateChangingNode=true`.
- `need_assessment` with executable `patch/apply`, `git/write`, `file/write`, mutating shell.

## NextAction State Machine

Source: `internal/pkg/workorchruntimecontract/next_action.go`

| NextAction | Input authority condition | Output |
| --- | --- | --- |
| `assess_gap` | definition/ledger changed or no current focus | recompute focus and coverage gaps |
| `plan` | no executable plan exists under current definition | LLM plan / execution graph seed |
| `acquire_target_snapshot` | state_change needed but patch-safe target missing | target_snapshot evidence |
| `state_change` | current focus missing `state_change` and target/context sufficient | accepted state_change evidence or rejected diagnostic |
| `validate` | current focus missing `validation` and no state_change preemption | accepted validation evidence or rejected diagnostic |
| `assessment` | required evidence complete and needs judgement | accepted/rejected assessment |
| `repair` | failed step has recoverable error but must remain under current authority | new narrow candidate or diagnostic |
| `request_decision` | missing external information/user choice | open decision ticket |
| `request_approval` | side effect requires explicit approval | pending approval |
| `complete` | all coverage satisfied and result accepted | run completed |
| `fail` | no recoverable path or fail-fast contract | run failed |

Precedence for current focus:

1. Terminal/gov gate: cancelled/superseded/failed/completed remains terminal.
2. Open approval/decision blocks scheduling.
3. Unsatisfied current focus drives NextAction.
4. If `state_change` is missing, state-change must preempt validation-only loops unless the next action is explicitly `acquire_target_snapshot`.
5. If target snapshot is required and unavailable, `acquire_target_snapshot` preempts `state_change`.
6. If required evidence is complete, `assessment` preempts more evidence acquisition.
7. If all coverage is satisfied, `complete`.

## Canonical Delivery Loop

The only valid forward loop is:

```text
Run active
  -> refresh authority from definition + coverage/evidence ledger
  -> choose current focus
  -> derive deterministic NextAction
  -> derive ConvergenceContract
  -> derive RequiredOutput
  -> derive StagePacket
  -> dispatch runtime/LLM/block
  -> contract gate
  -> accepted evidence or rejected diagnostic
  -> ledger recompute
  -> run state or next dispatch
```

Every dispatchable step must carry this single lineage:

```text
authoritySnapshotId / baselineRevision
definitionRevision
currentFocus.id
currentFocus.missingEvidenceKinds
nextAction.type
convergenceContract.state
stagePacket.idempotencyKey
requiredOutput.acceptedKinds
requiredOutput.forbiddenKinds
acceptedEvidenceRefs
rejectedEvidenceRefs
```

Any field not derived from this lineage is diagnostic only.

## Dispatch Contract Matrix

| NextAction | Contract state | Stage | Must include | Must forbid |
| --- | --- | --- | --- | --- |
| `acquire_target_snapshot` | `need_evidence` | `repair` or `verification` | read-only file/list/file/read/git/read or runtime-managed snapshot | `patch/apply`, `git/write`, `file/write`, mutating shell |
| `state_change` | `need_state_change` | `execution` or `repair` | one accepted state-changing kind and focus binding | assessment-only completion, read-only-only graph |
| `validate` | `need_validation` | `verification` | concrete validation evidence binding | workspace deliverable mutation, `patch/apply`, `git/write` |
| `assessment` | `need_assessment` | `acceptance` or `verification` | evidence refs and coverage item mapping | executable state-changing output |
| `request_decision` | `need_governance` or `blocked` | `decision` | decision ticket context | side effects |
| `request_approval` | `need_governance` | `approval` | approval scope and target | unscoped side effects |
| `complete` | deterministic complete | `acceptance` | accepted final result + all coverage satisfied | open coverage |
| `fail` | blocked/fail-fast | terminal | failure reason and evidence refs | new dispatch |

## Contract Violation State Machine

Candidate output can violate schema or contract. Violation handling is a state machine, not an ad hoc retry.

| Recoverability | Meaning | Allowed transition |
| --- | --- | --- |
| `retry_same_contract` | same requiredOutput, same authority | contract retry step |
| `narrow_contract` | same authority, narrower output shape | contract retry step |
| `normalize_then_retry` | normalize candidate without side effect then retry | retry step |
| `recompute_authority` | ledger changed or stale authority detected | authority refresh, then dispatch or governed wait |
| `rebuild_validation_required_output_from_authority` | validation requiredOutput render drift | rebuild from authority |
| `decision_required` | real external decision needed | open decision ticket |
| `fail_fast` | unsafe or impossible | fail step/run |

Hard rules:

- Contract violation output is rejected diagnostic evidence.
- It cannot write accepted evidence, result, artifact, or long-term high-value memory.
- If recoverability is `recompute_authority`, LLM must not be asked to recompute authority; runtime owns the recompute.
- Retry must keep the same `definitionRevision`, `currentFocus`, `NextAction`, `ConvergenceContract`, and `RequiredOutput` unless authority recompute changes them.

## Derived Field Authority Rules

The following fields may be persisted for readback or diagnostics, but must be overwritten from current authority before dispatch:

- `gapBaseline`
- `coverageFocus`
- `convergenceContract`
- `stagePacket`
- `repairActionPackage`
- `repairActionPackage.requiredOutput`
- top-level `requiredOutput`
- `currentActionContract`
- `acceptance.currentFocus`
- `executionContext.stagePacket`
- `contractViolationRecovery.expectedContract`

Rules:

1. Only `coverage/evidence ledger + definition + governance state` can select current focus and NextAction.
2. `convergenceAuthority` is a snapshot of that selection, not an independent mutable strategy object.
3. Any old field from failed step, replan continuation, decision resolution, provider retry, graph follow-up or contract retry is diagnostic lineage.
4. Dispatch input must be generated from canonical authority, not by merging old input.
5. If old input must be shown to LLM, it must be under diagnostic/history keys that cannot be read by contract renderers.

## Required Migration

## Implementation Progress

- 2026-07-09:
  - Added code-level `RunState` / `StepState` transition tables.
  - Added table-driven `NextAction -> ConvergenceContract.state` validation.
  - Added dispatch requiredOutput shape validation for `need_evidence` / `need_state_change` / `need_validation` / `need_assessment`.
  - Added current authority snapshot lineage sealing for dispatch-derived mirrors: `convergenceContract`, `repairActionPackage`, `currentActionContract`, `stagePacket`, and `stagePacket.nextAction`.
  - Consolidated dispatch derived-field rendering through `renderCurrentAuthorityDispatchInput` so initial step, next-step continuation, graph-local authority, target-snapshot realignment, and authority refresh all attach `stagePacket`, synchronize `requiredOutput`, attach `currentActionContract`, and seal `authoritySnapshotId` through the same path.
  - Routed runtime repair, contract retry, provider retry, resolved decision retry, known-readable decision repair, and context-integrity retry through `finalizeCurrentAuthorityDispatchInputForRun` before dispatch.
  - Hardened local validation repair preservation so it cannot override a latest run authority focus that still lacks `state_change`.
  - Added regression tests proving retry/resume inputs carry a sealed current authority snapshot and cannot preserve stale validation-only requiredOutput when the latest authority requires state-changing output.

### Phase 1: Freeze the table-driven state machine

- Add code constants or validator tables for:
  - RunState transitions.
  - StepState transitions.
  - CoverageWorkItem status -> derived action.
  - NextAction -> ConvergenceContract state.
  - NextAction -> RequiredOutput shape.
- Acceptance:
  - Unit tests reject all forbidden combinations listed in this document.

### Phase 2: Single authority refresh output

- Introduce or consolidate one canonical function that returns:

```text
ConvergenceAuthoritySnapshot
  definitionRevision
  coverageBaselineRevision
  currentFocus
  nextAction
  convergenceContract
  requiredOutput
  stagePacketSeed
  budgetScope
  acceptedEvidenceRefs
  rejectedEvidenceRefs
```

- Acceptance:
  - No dispatch path builds `convergenceContract`, `stagePacket`, or `requiredOutput` from failed step input directly.

### Phase 3: Dispatch input becomes pure rendering

- Dispatch input builder may read historical input only as diagnostics.
- It must first strip all derived fields, then render fresh fields from authority snapshot.
- Acceptance:
  - A test with stale `executionContext.stagePacket.requiredOutput` cannot produce mismatch.

### Phase 4: Repair/retry becomes authority consumer

- `runtimeRepairFollowUpStepInput`, contract retry, provider retry, graph continuation must consume authority snapshot.
- They can add diagnostics, failure surface, target snapshots, retry hints, but cannot choose NextAction.
- Acceptance:
  - validation failure with focus missing `state_change` cannot dispatch validation-only retry unless NextAction is `acquire_target_snapshot`.

### Phase 5: Terminal and governance gates

- Completion requires all active coverage items satisfied and accepted final result.
- Decision/approval requires real governance reason.
- Recovery exhaustion should create blocker only after authority recompute confirms no automatic path.
- Acceptance:
  - No `blocked_on_decision` for continuable `state_change` or target snapshot path.

### Phase 6: Projection is read-only

- Projection may expose mismatch diagnostics.
- Projection must not infer or repair owner state.
- Acceptance:
  - Projection tests show stale derived fields as diagnostics, not altered state.

## Immediate Code Review Checklist

Before touching code, inspect these paths against the state machine:

- `runtime_gap_baseline.go`
  - Should stop being an accumulation of local rebinding rules and become a pure authority snapshot/rendering validator.
- `runtime_convergence.go`
  - Should own NextAction -> ConvergenceContract mapping, not merge stale continuation contracts.
- `runtime_repair_input.go`
  - Should provide diagnostics and repair context only; it must not choose current authority.
- `runtime_repair.go`
  - Should consume recoverability and authority result; no finalization watchdog-style fallback.
- `runtime_current_action_contract.go`
  - Should render from canonical requiredOutput only.
- `runtime_dispatch_event.go` / `runtime_dispatch_execution.go`
  - Should reject dispatch if snapshot lineage is missing or inconsistent.
- `runtime_stage_packet.go`
  - Should carry one active focus and one NextAction only.

## Verification Plan

P0 tests:

- Matrix test: every NextAction maps to exactly one ConvergenceContract state.
- Matrix test: every ConvergenceContract state maps to valid RequiredOutput.
- Regression: focus missing only `state_change` cannot render `need_validation`.
- Regression: stale `stagePacket.requiredOutput` cannot override current authority.
- Regression: contract violation recovery `recompute_authority` never asks LLM to recompute authority.
- Regression: validation failure diagnostic with concrete target either routes to `state_change` or `acquire_target_snapshot`, not validation-only loop.

P1 tests:

- RunState transition table.
- StepState finalization ordering.
- Decision/approval scope and resolved resume.
- Projection read-only mismatch diagnostics.

P2 live verification:

- Start one clean workspace and one dirty/conflicting workspace.
- Observe:
  - authority refresh revision,
  - current focus,
  - NextAction,
  - ConvergenceContract,
  - StagePacket,
  - RequiredOutput,
  - dispatch result,
  - evidence ledger update.
- Pass criteria:
  - No `stagePacket.requiredOutput does not match current authority requiredOutput`.
  - No `focus missing state_change` with `need_validation` dispatch.
  - No `run=active/running` with failed current step and no blocker.
  - No governance ticket while automatic state_change or target snapshot path is available.
