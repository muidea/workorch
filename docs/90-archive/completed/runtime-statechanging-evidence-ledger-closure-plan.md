# Runtime State-Changing Evidence Ledger Closure Plan

Status: code-closed

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `goal-dod-nextaction-s3-2-state-change-evidence-write.md`
- `goal-dod-nextaction-s4-3-accepted-evidence-coverage-update.md`
- `goal-dod-nextaction-s5-5-missing-state-change-nextaction.md`

## Purpose

本文基于最新 live workspace 核对结果，补齐 WorkOrch runtime 在
`stateChanging` 执行结果入账、coverage required kind 闭合与 NextAction
重算之间的确定性闭环。

目标不是继续增加 prompt 约束，也不是为某个 workspace、Git 命令、软件开发模板或
version control snapshot 加特判。目标是让所有已经通过合同准入、带结构化
`stateChanging=true` 与 coverage scope 的 runtime step，在执行成功后形成可追溯的
accepted `state_change` evidence，并让 coverage ledger 只依据
`acceptedEvidenceByKind` 关闭 `missingEvidenceKinds`，再重新推导唯一 NextAction。

当前阶段不考虑兼容性。若现有实现、测试或投影与本文冲突，应按本文逐项收口。

## Contract Audit

- Scope: `ws-7ced70e98ea50c43/run-006` 的
  `version_control_snapshot / cwi-088` 收敛循环，以及对应的 runtime evidence
  recorder、coverage ledger 和 NextAction 重算路径。
- Observed state:
  - `run-006/graph-007/001-record-version-control-snapshot-state-change-final`
    的 step input 明确包含 `stateChanging=true`、`coverageFocusId=focus-cwi-088`、
    `coverageWorkItemIds=["cwi-088"]`、`coverageTargets=["version_control_snapshot"]`。
  - 该 step 执行成功，output 显示 `exitCode=0`，并产生了可审计的状态变更结果。
  - 后续 EvidenceRecord 只生成 accepted `validation`，没有 accepted
    `state_change`；EvidenceRecord 中 `coverageFocusId`、
    `coverageWorkItemIds`、`coverageTargets` 为空。
  - `coverage-work-items/run-006-cwi-088.json` 中 `acceptedEvidenceRefs` 已累计多条
    refs，但 `acceptedEvidenceByKind` 没有 `state_change`，item 仍保持 `failed`，
    后续 NextAction 继续围绕同一 `state_change` gap 派发重复修复。
- Expected state:
  - 已通过合同准入且成功执行的 `stateChanging=true` runtime step 必须创建 accepted
    `state_change` EvidenceRecord。
  - EvidenceRecord 必须保留来自 step input / current authority snapshot 的 coverage
    scope：`coverageFocusId`、`coverageWorkItemIds`、`coverageTargets`、
    `targetRefs`。
  - Coverage ledger 必须按 `acceptedEvidenceByKind` 关闭对应 required kind；当
    `state_change` 与 `validation` 都满足时，当前 item 不得继续驱动
    `state_change` NextAction。
- Deviation:
  - step input 的 structured state-changing/focus scope 没有贯穿到 output/evidence
    record。
  - accepted refs 被追加到 coverage item，但没有按 evidence kind 写入
    `acceptedEvidenceByKind.state_change`。
  - NextAction 仍从未闭合的 `missing state_change` 派生，导致重复生成等价修复图。
- Owner: WorkOrch core。问题位于中立 runtime candidate gate、runtime step evidence
  recorder、coverage ledger recompute 和 deterministic NextAction，不属于
  workspaceTemplate、Soul、TUI/Web、provider 或具体命令策略。
- Event path:
  - LLM candidate executionGraph -> contract gate/admission -> runtime step input with
    stateChanging scope -> block/runtime execution -> step output -> EvidenceRecord
    writer -> coverage ledger recompute -> convergence authority refresh -> NextAction
    -> projection/ViewModel。
- Contract gap:
  - 非 `patch/apply` 的 admitted state-changing runtime step 没有统一生成
    `state_change` evidence。
  - evidence writer 没有把 input/authority 的 coverage scope 复制到 EvidenceRecord。
  - coverage ledger 可见 accepted refs，但无法按 required evidence kind 闭合。
- Boundary check:
  - 收口只能基于结构化字段和通用 step/admission 状态。
  - core 不得根据 template ID、Soul ID、workspace title、语言、文件名、命令文本、
    Git ref、provider 或 live workspace ID 推断 state_change。
- Verification:
  - 增加失败样本测试：成功的 `shell/command` step，input 带
    `stateChanging=true` 与 `coverageWorkItemIds`，必须生成 accepted
    `state_change` evidence 并关闭 `state_change` missing。
  - 增加对照测试：普通 validation-only `shell/command` 不得误生成
    `state_change`。
  - 增加 coverage recompute/NextAction 测试：同一 item 已有 accepted
    `state_change + validation` 后，不得继续派发 `state_change`。

## Closure Scope

### C1: Preserve Runtime Step Coverage Scope

Problem:

- live step input 已带有 `coverageFocusId`、`coverageWorkItemIds`、
  `coverageTargets` 和 `stateChanging`，但 step output、tool-call output 与
  EvidenceRecord 没有保留这些字段。

Target:

- runtime step completion path 必须能从 step input 或 dispatch authority snapshot 读取
  scope，并传递给 evidence writer。
- output 可以保持 block 原始输出的中立性，但 evidence writer 必须记录结构化 scope。
- scope 解析必须支持至少：
  - `coverageFocusId`
  - `coverageWorkItemIds`
  - `coverageTargets`
  - `targetRef` / `targetRefs`
  - `stateChanging`

Candidate code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_mainline_integration_test.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
- runtime step completion / evidence recording path in `internal/modules/kernel/orchestrator/biz`

Acceptance:

- A completed runtime step can produce EvidenceRecord metadata containing the same coverage scope that was present in input.
- Missing scope in output does not erase scope that was present in input.
- No branch checks command text, file names or template identifiers.

### C2: Record Accepted State-Change Evidence For Admitted Runtime Steps

Problem:

- S3.2 already records accepted `state_change` for successful `patch/apply`, but live flow shows a successful admitted `shell/command` with `stateChanging=true` only produces validation evidence.

Target:

- If a runtime step is terminal success and its input declares `stateChanging=true`, the evidence writer must create an accepted `state_change` EvidenceRecord.
- This rule applies to any admitted step kind capable of side effects; it is driven by the structured `stateChanging` flag and admission result, not by natural language or command text.
- If the same step also produces validation metadata, `validation` evidence may also be recorded. `validation` must not replace `state_change`.

Candidate code areas:

- runtime evidence recording after step completion.
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items_test.go`

Acceptance:

- Successful `stateChanging=true` runtime step creates exactly one deterministic accepted
  `state_change` record for the step/sourceRef.
- Re-running/replaying the same event is idempotent and does not create duplicate evidence.
- Failed, rejected or non-admitted state-changing candidates do not create accepted `state_change`.
- Validation-only shell/tool steps do not create `state_change`.

### C3: Apply Accepted Evidence By Required Kind

Problem:

- `acceptedEvidenceRefs` can contain many refs while `acceptedEvidenceByKind` remains empty or lacks `state_change`, so `missingEvidenceKinds` is not closed.

Target:

- Coverage updates must write accepted evidence into `acceptedEvidenceByKind[kind]` and then derive flattened `acceptedEvidenceRefs`.
- Required evidence satisfaction must be computed from `acceptedEvidenceByKind`, not from untyped refs alone.
- Rejected diagnostics must remain under rejected fields and must not satisfy required kinds.

Candidate code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_coverage_state.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`

Acceptance:

- For a coverage item requiring `["state_change", "validation"]`, applying accepted
  `state_change` removes only `state_change` from `missingEvidenceKinds`.
- Applying accepted `validation` after accepted `state_change` leaves no missing kind.
- Rejected evidence does not remove missing kinds.
- Existing accepted evidence is preserved when later rejected diagnostics are recorded.

### C4: Recompute Authority And Stop Repeating Closed Gaps

Problem:

- After repeated accepted refs, the current focus still reports
  `missing accepted evidence binding for required evidence kind: state_change`, and NextAction keeps generating equivalent state-changing snapshot attempts.

Target:

- After evidence ledger write and coverage recompute, convergence authority refresh must observe the closed required kind and choose the next deterministic action.
- If all required kinds for the focus are satisfied, the next action must move to assessment, next coverage item, complete, or governed block/fail. It must not remain `state_change`.

Candidate code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_state.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/coverage_summary.go`

Acceptance:

- Fixture with accepted `state_change + validation` for one focus cannot derive
  `state_change` NextAction for the same focus.
- Projection may expose historical rejected diagnostics, but current NextAction is derived from the refreshed coverage/evidence ledger.
- Repeated equivalent state-changing steps stop after the required kind is closed.

### C5: Resource Regression Guard

Problem:

- The live run wrote many checkpoint/event/evidence/LLM-call files while repeating the same unclosed gap. This creates CPU/RSS/IO pressure and masks real progress.

Target:

- Fixing the ledger closure should reduce repeated equivalent dispatches. Add regression checks around idempotent evidence write and NextAction transition to prevent unbounded retry loops for already-closed required kinds.

Candidate code areas:

- orchestrator tests around repeated completed step replay.
- projection/readmodel tests only if stale projection hides closed ledger state.

Acceptance:

- Replaying the same completed state-changing step does not increase evidence count unboundedly.
- Once the focus is closed, scheduler/NextAction does not dispatch another same-focus state-change action solely because stale untyped refs exist.

## Implementation Order

1. Add focused failing tests for C1/C2 using a neutral `shell/command` step with
   `stateChanging=true` and coverage scope.
2. Update runtime evidence recording to preserve step input coverage scope.
3. Generate deterministic accepted `state_change` EvidenceRecord for successful admitted
   state-changing runtime steps.
4. Update or confirm coverage ledger recompute writes and reads
   `acceptedEvidenceByKind`.
5. Add NextAction regression test for closed `state_change + validation` focus.
6. Run targeted tests, then boundary search.

## Verification Commands

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'StateChange|Coverage|NextAction|Evidence' -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server|version_control_snapshot|refs/workorch" internal/modules/kernel/orchestrator/biz -g '*.go'
rg -n "stateChanging|coverageFocusId|coverageWorkItemIds|acceptedEvidenceByKind|missingEvidenceKinds" internal/modules/kernel/orchestrator/biz -g '*.go'
```

Live verification after code closure:

1. Redeploy latest `workorch`.
2. Resume or replan a workspace that requires both `state_change` and `validation`.
3. Confirm a successful `stateChanging=true` runtime step writes accepted `state_change`
   EvidenceRecord with coverage scope.
4. Confirm the target coverage item shows `acceptedEvidenceByKind.state_change`.
5. Confirm current NextAction moves away from the closed `state_change` gap.
6. Observe CPU/RSS/IO for at least one follow-up cycle and confirm no repeated equivalent same-focus state-change loop.

## Code Closure Result

Completed on 2026-06-27.

- C1 completed: runtime step EvidenceRecord now preserves structured coverage scope
  from step input/output, including `coverageFocusId`, `coverageWorkItemIds`,
  `coverageTargets`, `targetRef` / `targetRefs` and `stateChanging`.
- C2 completed: successful completed runtime steps that declare `stateChanging=true`
  now write one deterministic accepted `state_change` EvidenceRecord. Dedicated apply
  paths such as patch/artifact apply remain owned by their existing apply evidence
  writers to avoid duplicate state-change records.
- C3 completed: coverage step updates now treat structured `stateChanging=true` as
  a state-change evidence source, and accepted EvidenceRecord reconciliation can bind
  by `coverageTargets` as well as coverage item refs.
- C4 covered by regression tests: closing required `state_change + validation`
  removes `missingEvidenceKinds` for the focus and leaves the item `evidence_ready`.
- C5 covered by idempotency tests: replaying the same completed runtime step keeps
  deterministic EvidenceRecord count stable and does not create duplicate
  state-change evidence.

Executed verification:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'StateChange|Coverage|NextAction|Evidence' -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
git diff --check
```

## Done Criteria

- Successful admitted `stateChanging=true` runtime steps produce accepted
  `state_change` evidence with workspace/run/step and coverage scope.
- Validation evidence and state-change evidence remain separate evidence kinds.
- Coverage ledger closes required kinds only through accepted evidence by kind.
- Deterministic NextAction no longer repeats `state_change` for a focus whose
  `state_change` kind is already accepted.
- The implementation remains core-neutral and contains no template, language, command text,
  file name, provider, workspace title or live workspace ID branch.
