# Runtime Terminal Ledger NextAction Dispatch Contract Hardening Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-07

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-graph-terminal-continuation-closure-plan.md`
- `runtime-ledger-authority-dispatch-hardening-final-closure-plan.md`
- `runtime-target-snapshot-statechange-preflight-closure-plan.md`
- `runtime-validation-required-output-authority-closure-plan.md`

## Purpose

本文用于固化以下主链路，避免 workspace 在 terminal step 后出现 `run=active + currentStepId=null + executable NextAction`，或因 target/context 不足导致 LLM 持续输出合同违例：

```text
terminal step result
  -> ledger recompute
  -> deterministic NextAction
  -> target/context sufficiency gate
  -> canonical StagePacket
  -> dispatch
```

本计划只处理 WorkOrch core 通用合同，不引入 workspace、template、Soul、语言、文件名、命令、provider 或错误文本特判。当前 `.workorch-workspaces` 中可能存在冲突、半成品或历史残留代码，但 core 只能通过结构化 snapshot、coverage/evidence ledger、target context 和 requiredOutput 合同消费这些事实。

## Contract Audit

- Scope:
  - `ws-db040d24a190f6e5/run-096`
  - `ws-705249a8d7816330/run-093-replan-001`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_graph.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_dispatch_specs.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`

- Observed state:
  - `run-093-replan-001` 当前仍在 LLM provider 执行，contract audit 显示 `NextAction=state_change`、`convergenceContract=need_state_change`、`requiredOutput` 已允许 `patch/apply`、`shell/command`、`mcp/tool_call`、`git/write`，未再出现 `need_state_change requires state-changing requiredOutput`。
  - `run-096` 已完成 `git/write`、`patch/apply`，并在 `shell/command` 验证失败后成功触发 runtime repair LLM step。
  - `run-096` 的 coverage ledger 已有 `state_change` accepted evidence，当前 focus `cwi-036` 仍缺 `validation`。
  - `run-096` 的 owner state 已推导 `NextAction=validate`、`convergenceContract=need_validation`、validation requiredOutput 已生成，但 run 保持 `active` 且 `currentStepId=null`。

- Expected state:
  - 任一 terminal step result 必须触发 evidence ledger 写账与 coverage ledger recompute。
  - ledger recompute 后必须产生唯一 current focus 与 deterministic NextAction。
  - 如果 NextAction 可执行，runtime 必须通过 canonical StagePacket 渲染并 dispatch 下一步。
  - 如果 target/context 不足，runtime 必须先 dispatch runtime-managed acquisition 或进入明确 blocker/terminal，不能让 LLM 在缺少当前事实时硬生成 patch/validation。
  - 不允许保留 `run=active + currentStepId=null + executable NextAction + no blocker`。

- Deviation:
  - terminal graph/repair step succeeded 后，ledger 已重新推导出 `validate`，但 continuation dispatch 未发生。
  - `NextAction -> dispatch` 的主路径没有对 `active + no current step` 做硬门禁。
  - state-changing/validation dispatch 前的 context sufficiency 仍需固化，避免 LLM 仅凭片面上下文输出 read-only graph、错误 patch 或 validation evidence 缺字段。

- Owner:
  - WorkOrch core runtime。
  - 不属于 LLM provider、workspaceTemplate、Soul 或具体 workspace 业务特判问题。

- Event path:

```text
step terminal result
  -> record accepted/rejected/diagnostic evidence
  -> coverage ledger recompute
  -> refresh convergenceAuthority / gapBaseline
  -> BuildConvergenceContract
  -> target/context sufficiency gate
  -> attach StagePacket / currentActionContract
  -> dispatch spec
  -> EventHub DispatchStepCommand
  -> run.currentStepId readback
```

- Contract gap:
  - Terminal continuation 缺少“executable NextAction 必须 dispatch”的硬约束。
  - Ledger recompute 与 continuation dispatch 之间存在可静默断开的状态。
  - Target/context sufficiency 仍未成为 state-changing 与 validation dispatch 的前置合同。
  - StagePacket mirror 一致性已有门禁，但需要覆盖 terminal continuation 入口。

- Proposed change:
  - 先固化 terminal continuation dispatch gate。
  - 再固化 state-changing target snapshot sufficiency gate。
  - 再固化 validation context sufficiency gate。
  - 最后补齐 canonical StagePacket 在 terminal continuation 入口的镜像一致性测试。

- Boundary check:
  - 只消费 `NextAction`、`convergenceContract.state`、`coverageWorkItemIds`、`structuredTargetRefs`、`targetSnapshots`、`requiredOutput`、`evidence kind` 等结构化字段。
  - 禁止在 core 中根据 workspace id、template id、Soul id、文件名、命令、provider、错误文本 marker 做分支。

- Verification:
  - 每个 P0 改动必须有失败样本单测。
  - targeted tests、orchestrator package tests、`make fmt-check` 必须通过。
  - live 验证必须证明不再出现 active run 无 current step 且 NextAction 可执行的状态。

## Closure Principles

1. 不放松合同。
   - `state_change` 不能由 read-only graph、assessment 或 validation evidence 替代。
   - `validation` 不能由 supporting-only evidence 关闭。

2. 不做下游兜底。
   - 不用 watchdog/recovery finalization 把 active run 强行补状态。
   - 修复点必须在 terminal result、ledger recompute、NextAction continuation 或 dispatch gate 主路径。

3. LLM 上下文不足时不让 LLM 硬写。
   - 缺 target snapshot 时先 acquire snapshot。
   - 缺 validation route/context 时先补 validation context 或进入 governed blocker。

4. old lineage 只做诊断。
   - failed step、old graph、old repair package、old requiredOutput、old decision text 不得覆盖 current authority。

## P0: Terminal Continuation Dispatch Gate

- [x] Add failing test: `active + currentStepId=null + NextAction=validate` dispatches a validation step.
  - Scenario:
    - Run is active.
    - Current step has terminal succeeded.
    - Coverage ledger current focus has accepted `state_change` and missing `validation`.
    - Current authority says `NextAction=validate`.
  - Expected:
    - Runtime creates one validation dispatch spec.
    - Run current step is updated or EventHub dispatch command is emitted.
    - No silent `active + null currentStepId` remains.

- [x] Add failing test: `active + currentStepId=null + NextAction=state_change` dispatches state-changing LLM step or target snapshot prerequisite.

- [x] Add failing test: executable NextAction dispatch failure becomes structured blocker/failure.
  - Expected:
    - EventHub publish / owner readback failure cannot leave run active without blocker.

- [ ] Implement a single continuation gate:

```text
continueAfterTerminalResult(run)
  -> ensure ledger recompute
  -> refresh convergenceAuthority
  -> classify NextAction
  -> if executable: build dispatch spec
  -> if governance: block on ticket/approval
  -> if complete/fail: finalize terminal
```

- [ ] Route graph terminal, repair succeeded, validation succeeded/failed, provider retry terminal and restart continuation through this same gate.

Acceptance:

- No active run may have `currentStepId=null` while `NextAction` is `validate`、`state_change`、`assessment`、`acquire_target_snapshot` or another executable action.
- `run-096`-style state must immediately continue to validation dispatch or a precise blocker.

## P0: Ledger Recompute Contract

- [ ] Add test: `patch/apply` and `git/write` terminal success write accepted `state_change` evidence only to exact current focus.

- [ ] Add test: `shell/command` terminal success without `observedClaims` is supporting-only and does not close validation.

- [ ] Add test: rejected validation evidence remains rejected diagnostic and cannot close required validation.

- [ ] Ensure recompute result contains:
  - current focus id
  - missing evidence kinds
  - accepted/rejected evidence refs
  - next action kind
  - budget scope

Acceptance:

- `state_change` and `validation` cannot satisfy each other.
- supporting-only evidence cannot close required evidence.
- ledger recompute always produces enough data for NextAction interpreter.

## P0: Deterministic NextAction To Dispatch Contract

- [ ] Add test matrix for NextAction mapping:
  - `state_change` -> state-changing dispatch or target snapshot acquisition.
  - `validate` -> bounded validation dispatch.
  - `assessment` -> assessment dispatch/gate.
  - `request_decision` / `request_approval` -> governed blocker.
  - `complete` -> run terminal completed.
  - `fail` -> run terminal failed with structured reason.

- [ ] Centralize NextAction-to-dispatch selection.
  - Avoid separate graph/retry/restart/replan paths each making partial decisions.

- [x] Make dispatch idempotency include workspace/run/step/kind/action scope.

Acceptance:

- Every deterministic NextAction has exactly one owner outcome.
- No path can consume NextAction without producing dispatch, blocker, terminal, or typed error.

## P0: Target/Context Sufficiency Gate

### State-Changing Sufficiency

- [x] Add failing test: `NextAction=state_change` without safe current target snapshot cannot dispatch direct patch LLM.

- [ ] Required state-changing preflight:
  - current focus has target refs.
  - target snapshot exists for patch/file write target, or explicit acquire target snapshot is dispatched.
  - snapshot is complete, current, and safe for file write.
  - redacted/truncated snapshots are not safe for file write.
  - stale snapshots cannot seed patch.

- [ ] If state-changing target context is insufficient, convert to:

```text
NextAction=acquire_target_snapshot
convergenceContract=need_evidence
requiredOutput=read-only target snapshot acquisition
```

Acceptance:

- LLM is not asked to produce patch/apply without target snapshot sufficiency.
- Dirty/conflicting workspace facts are exposed as structured target context, not inferred from prompt prose.

### Validation Sufficiency

- [ ] Add failing test: `NextAction=validate` with missing validation evidence route cannot dispatch ambiguous validation.

- [ ] Required validation preflight:
  - current focus binding exists.
  - validation requiredOutput contains accepted validation kinds.
  - validation scratch/resource lifecycle is declared when needed.
  - concrete validation must include `validationEvidence.mode`、`observationType`、`observedClaims`、`targetRefs` and coverage binding.

- [ ] If validation context is insufficient, dispatch context acquisition or return governed blocker/fail-fast with typed reason.

Acceptance:

- Validation success without observed claims remains supporting-only.
- Bounded validation dispatch always contains enough route/context for concrete validation evidence.

## P1: Canonical StagePacket Coverage For Terminal Continuation

- [x] Add test: terminal continuation input rebuilds all mirrors from current authority:
  - `convergenceContract`
  - `stagePacket.requiredOutput`
  - `stagePacket.nextAction.requiredOutput`
  - `repairActionPackage.requiredOutput`
  - top-level `requiredOutput`
  - `currentActionContract.requiredOutput`

- [ ] Ensure old failed step input/output only enters:
  - `diagnosticLineage`
  - `latestRejectedDiagnostic`
  - `processEvidenceTimeline`
  - `currentFailure`

- [ ] Dispatch preflight must reject mirror mismatch before provider call.

Acceptance:

- Terminal continuation cannot use old graph-local or failed-step requiredOutput as current contract.
- `stale_required_output_mirror_rebound` remains diagnostic only and does not prevent dispatch when canonical mirrors are aligned.

## P1: Projection And Readback Contract

- [x] Projection must show the same owner NextAction as run execution context.

- [ ] After EventHub dispatch publish, owner readback must observe:
  - step exists or queued command exists.
  - run current step is updated or recoverability is explicitly pending.

- [x] If readback does not advance, record structured scheduler diagnostic and prevent silent active/no-current-step state.

Acceptance:

- TUI/Web/API cannot display a run as merely active when owner state has executable NextAction but no dispatch.

## P2: Resource And Context Noise Control

- [ ] Ensure terminal continuation does not repeatedly hydrate full snapshots for polling views.

- [ ] Ensure target/context acquisition is bounded and focused on current coverage target, not entire workspace dump.

- [ ] Ensure LLM context includes enough current facts but does not include stale old graph packages as competing authority.

Acceptance:

- Context is complete enough for current action, but remains structured and bounded.
- CPU/IO does not grow due to repeated full snapshot JSON conversion or unbounded workspace reads.

## Verification Plan

### Unit Tests

Run:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TerminalContinuation|NextActionDispatch|TargetSnapshotSufficiency|ValidationSufficiency|StagePacket|LedgerRecompute' -count=1
```

Required coverage:

- terminal succeeded -> ledger recompute -> validate dispatch
- terminal succeeded -> ledger recompute -> state_change dispatch
- terminal failed -> rejected diagnostic -> repair/validate/state_change dispatch
- target snapshot missing -> acquire_target_snapshot, not direct patch
- validation route/context missing -> context acquisition or governed blocker
- mirror mismatch rejected before LLM provider call

### Package Tests

Run:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
make fmt-check
```

### Live Verification

Run:

```bash
./scripts/verify-authority-live-e2e-closure.sh
```

Then inspect active runs:

- No active run with `currentStepId=null` and executable NextAction.
- No `need_state_change requires state-changing requiredOutput`.
- No state-changing LLM dispatch without target/context sufficiency.
- No validation completion accepted without observed claims and coverage binding.
- If a run cannot continue, it must be terminal or governed blocked with precise reason.

## Done Criteria

- `terminal step result -> ledger recompute -> NextAction -> target/context sufficiency gate -> canonical StagePacket -> dispatch` is covered by tests.
- `run=active + currentStepId=null + executable NextAction` is impossible by contract.
- LLM receives a current, authority-derived, bounded actionable package before every state-changing or validation dispatch.
- Old failed-step/graph/repair lineage cannot override current authority.
- Live validation shows either positive continuation, governed blocker, terminal success/failure, or typed insufficiency action; no silent active stall.
