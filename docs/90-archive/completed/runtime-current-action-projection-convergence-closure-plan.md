# Runtime Current Action Projection Convergence Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-29

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-ledger-binding-projection-authority-closure-plan.md`
- `runtime-current-live-convergence-improvement-plan.md`
- `runtime-validation-nextaction-convergence-improvement-plan.md`

## Purpose

本文件承接 2026-06-29 live verification 结果，专门收口“经过多轮完善后仍然存在偏差”的根因。

当前目标不是继续叠加局部门禁，而是把实现重新收束到两份长期设计文档要求的主线：

```text
Coverage/Evidence Ledger
  -> Deterministic NextAction
  -> single currentActionContract
  -> narrow LLM candidate
  -> Candidate Gate
  -> accepted/rejected evidence
  -> Projection/ViewModel from same owner snapshot
```

当前阶段不考虑兼容性。若现有字段、测试或端侧行为与本文冲突，应按本文逐项收口。

## Contract Audit

- Scope:
  - live workspaces `ws-705249a8d7816330` / `run-007` and `ws-db040d24a190f6e5` / `run-007`
  - runtime convergence authority, LLM retry contract, workspace projection, ViewModel prompt mode, list/detail snapshot consistency
- Observed state:
  - deployed binary is `ef2efd89 fix: align runtime ledger projection authority`.
  - `ws-db040d24a190f6e5` detail snapshot shows `workspace.state=waiting_decision`, `run.state=blocked_on_decision`, and `blockingTicketId=run-007-contract-retry-001-of-planning-001-3-contract-violation-decision`.
  - The same detail response has `viewModel.promptMode=workspace`, and workspace list shows `state=active`, `blockingTicketId=null`, `promptMode=workspace`.
  - `ws-705249a8d7816330` produced files (`go.mod`, `main.go`) but deliverable quality is placeholder-level and does not satisfy Goal/DoD.
  - `ws-db040d24a190f6e5` retry request required `state_change`, but LLM returned read-only `file/read` plus `shell/command` verification graph and was rejected by runtime.
  - latest retry request payload was about 945 KB; provider usage showed about 30k input tokens for one turn.
- Expected state:
  - list/detail/main view/prompt must be generated from the same normalized owner snapshot and same ViewModel revision.
  - blocked decision owner state must project as `promptMode=decision` everywhere.
  - LLM should see one narrow current action contract, not multiple competing derived contracts.
  - rejected LLM output should become diagnostic evidence and bounded retry input, but retry input must be smaller and more actionable than the original context.
  - accepted state change must be tied to coverage/evidence ledger before further validation or completion.
- Deviation:
  - projection overlay can mix a fresh detail owner state with a stale projected ViewModel.
  - runtime still exposes many equivalent derived fields to LLM: `convergenceAuthority`, `gapBaseline`, `convergenceContract`, `coverageFocus`, `repairActionPackage`, top-level `requiredOutput`, `stagePacket.requiredOutput`, and `contractViolationRecovery.expectedContract`.
  - retry context remains large and noisy, so LLM continues returning read/inspect/verify graphs under a state-changing contract.
  - tests and closure docs marked several slices `code-closed`, but live chain still violates MVVP and narrow-action expectations.
- Owner:
  - primary owner is WorkOrch core runtime and workorchd projection.
  - this is not a template-specific or provider-specific problem, although software-dev template policy may later provide better task granularity.
- Event path:
  - TUI/Web/REST command -> workorchd owner state -> runtime scheduler -> convergence authority -> LLM input -> contract gate -> evidence ledger -> projection store -> REST/SSE -> TUI/Web.
- Contract gap:
  - missing single provider-facing current action contract.
  - projection detail/list/ViewModel are not forced to be same-source.
  - retry compaction does not enforce “current focus + target snapshot + last violation + one output shape”.
  - accepted state-change quality is not strong enough before runtime moves to further validation.
- Boundary:
  - closure must remain business-neutral.
  - core may reason about generic state change, validation, evidence, projection and governance.
  - core must not branch on template ID, Soul ID, language, command, provider, workspace title, file names, or live workspace IDs.

## Root Cause

The repeated deviation is caused by implementation drift from the simplification design:

1. The code added more guardrails around the old multi-field contract instead of replacing it with one provider-facing current action contract.
2. Projection added normalization and overlay behavior, but did not enforce a single owner snapshot source for workspace, run, blocker and ViewModel.
3. Retry feedback records violations, but the next prompt still carries a large historical context and many duplicated contract surfaces.
4. Local tests cover many helper-level invariants but do not yet fail on the exact live chain: blocked owner state -> projected list/detail -> TUI prompt, or state-change violation -> narrow retry -> accepted state change.

## Design Direction

### 1. Single Current Action Contract

Runtime must produce one provider-facing `currentActionContract` immediately before provider/block dispatch.

`currentActionContract` is derived from current owner authority:

```text
definition revision
  + coverage/evidence ledger
  + current focus
  + deterministic NextAction
  + accepted/rejected evidence refs
  + latest violation diagnostic
  -> currentActionContract
```

The provider-facing input must expose only this contract as the active contract. Existing fields such as `gapBaseline`, `convergenceContract`, `repairActionPackage`, `stagePacket`, and `contractViolationRecovery.expectedContract` may remain as internal audit records or trace refs, but they must not be duplicated as active LLM instructions.

Required invariant:

- `currentActionContract.nextAction.kind` is the only action selector visible as active instruction.
- `currentActionContract.requiredOutput` is the only active required output.
- old repair packages, old replan continuation, rejected output, and governance history can only appear under diagnostic lineage.

### 2. Projection Same-Source ViewModel

Projection must emit list, detail, workspace metadata and prompt mode from one normalized owner snapshot.

Required invariant:

- If detail says `workspace.state=waiting_decision` or `run.state=blocked_on_decision` with a blocker ID, ViewModel must be `promptMode=decision`.
- Workspace list item must carry the same blocker and prompt mode as the detail summary for the same projected source revision.
- HTTP detail handlers must not overlay a stale projected ViewModel on a freshly read detail snapshot.
- Projection can surface `authorityDiagnostics`, but cannot hide or locally repair owner-state conflicts.

### 3. Narrow Retry Context

Contract violation retry must not reuse the full large context when the next action is already known.

Provider-facing retry input should be:

```text
currentActionContract
target snapshots or snapshot refs for the current focus
last rejected diagnostic
accepted evidence refs for this focus
explicit forbidden repeat pattern
```

Required invariant:

- Retry request omits historical large sections unless explicitly referenced by compact refs.
- Retry request does not include multiple equivalent required-output surfaces.
- When action is `state_change`, the retry must choose one output mode:
  - `patch/apply`
  - `state_changing_tool`
  - `real_governance_blocker`
- Read-only graph is rejected before side effects and should not cause further broad context growth.

### 4. State-Change Quality Gate

Accepted state-changing output must be strong enough to support evidence ledger progress.

Required invariant:

- Placeholder file writes or no-op patches cannot satisfy `state_change`.
- Accepted state change must bind to current coverage work item IDs and target refs.
- If the state change is too small to satisfy the target, runtime records partial accepted evidence but still derives the next gap deterministically.
- Validation cannot be considered satisfied until it binds to the same focus or a declared supporting focus.

## Code Scope

### P0 Projection Same-Source

Target files:

- `internal/modules/application/workorchd/service/server/http_workspace.go`
- `internal/modules/application/workorchd/service/server/workspace_projection.go`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_governance_projection.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_summary.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_detail.go`
- `internal/services/workorch/service/tui/workspace_state.go`

Tasks:

1. Remove or narrow `mergeWorkspaceProjectionOverlay` so it cannot override a freshly normalized detail ViewModel with stale projected ViewModel.
2. Make list and summary snapshots use the same `normalizedEntrySnapshot` path as detail.
3. Add a projection invariant helper that derives workspace state, blocker fields and prompt mode together.
4. Add tests for:
   - detail blocked decision -> prompt mode decision.
   - workspace list blocked decision -> prompt mode decision.
   - list/detail same projected source revision -> same blocker and prompt mode.
   - stale ViewModel cannot override newer owner state.

Acceptance:

- Live `ws-db040d24a190f6e5`-class state cannot project as `workspace` prompt while run is `blocked_on_decision`.

### P0 Provider-Facing Current Action Contract

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`

Tasks:

1. Introduce a neutral `CurrentActionContract` model.
2. Build it from current convergence authority immediately before dispatch.
3. Render only `currentActionContract` as active provider-facing contract.
4. Move duplicated legacy surfaces into internal diagnostics or trace refs:
   - `gapBaseline`
   - `convergenceContract`
   - `repairActionPackage`
   - `stagePacket.requiredOutput`
   - `contractViolationRecovery.expectedContract`
5. Add a dispatch integrity gate that fails if provider-facing input contains more than one active required output.

Acceptance:

- Request inspection shows exactly one active required output path.
- old lineage can be present only under `diagnosticLineage` and never as active instruction.

### P0 Narrow Contract Retry

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_output_mode_selector.go`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`

Tasks:

1. Add retry compaction mode for known current action.
2. For `state_change` retry, keep only:
   - current action contract.
   - current focus target refs.
   - target snapshots or explicit snapshot refs.
   - last rejected diagnostic.
   - accepted/rejected evidence refs for current focus.
3. Remove large historical DoD failure lists from active retry instruction; keep them as compact refs.
4. Add context budget test proving retry request is materially smaller than original request for the same run.

Acceptance:

- State-change retry no longer produces 900KB-class prompt when the target action is already known.
- LLM retry prompt clearly forbids returning read-only graphs and offers one preferred state-changing output shape.

### P1 State-Change Output Quality Gate

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_graph.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_quality.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`

Tasks:

1. Reject placeholder state changes when they do not bind to target refs or do not materially change a target.
2. Preserve partial accepted evidence only when a real state change occurred.
3. Recompute next action after partial evidence instead of treating one patch as broad DoD progress.
4. Add tests for placeholder file writes and low-quality state changes.

Acceptance:

- Placeholder proxy-like output cannot mark `source_change` satisfied.
- Real but partial state change is accepted as partial evidence and leads to the next deterministic gap.

### P1 Live Chain Regression

Target files:

- `internal/modules/application/workorchd/service/server/http_test.go`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/manager_test.go`
- `internal/modules/kernel/orchestrator/biz/*_integration_test.go`
- `internal/services/workorch/service/tui/model_test.go`

Tasks:

1. Add e2e-style fixture for owner blocked decision -> projection list/detail/prompt.
2. Add fixture for state-change contract violation -> narrow retry -> accepted patch or governed blocker.
3. Add fixture proving TUI does not locally infer prompt mode when ViewModel is present.
4. Add fixture proving accepted command/projection revision remains scoped per workspace.

Acceptance:

- Tests fail on the current live deviation and pass after P0/P1 closure.

### P2 Resource Guard

Target files:

- `internal/modules/application/workorchd/biz/appservice/read_model_workspace*.go`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/*`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_*context*.go`

Tasks:

1. Keep projection refresh metadata-only for high-frequency paths.
2. Ensure full snapshot/detail hydration is only used by explicit detail or observability views.
3. Add metrics/test for retry request bytes and projection metadata scan count.
4. Keep old validation scratch cleanup as managed resource lifecycle, not projection/watchdog compensation.

Acceptance:

- Idle or governed-blocked workspaces do not trigger repeated full snapshot hydration.
- Narrow retry reduces request bytes and CPU pressure during repeated contract violations.

## Closure Progress

| Item | Status | Evidence |
| --- | --- | --- |
| P0 Projection Same-Source | code-closed | Added shared prompt-mode derivation in `readmodelprojection.WorkspacePromptMode`; `workspaceviewcore` now uses the shared derivation; HTTP detail projection no longer lets stale projected ViewModel override fresh detail owner state and rebinds prompt mode from normalized detail state. Added `TestWorkspaceSnapshotEndpointDoesNotOverlayStalePromptMode`. |
| P0 Provider-Facing Current Action Contract | code-closed-first-slice | Added neutral `currentActionContract` model and dispatch attachment. LLM prompt rendering now emits `C3.current-action-contract` and suppresses legacy active `C3.stage-contract`, `C3.stage-packet-contract`, and `C3.repair-action-package` sections when the contract exists. Current action text now treats `currentActionContract.requiredOutput` as the active contract. Added runtime and prompt regression tests. Legacy runtime input fields remain for internal execution and audit until later slices can remove them safely. |
| P0 Narrow Contract Retry | code-closed-second-slice | When `currentActionContract` is present, prompt current-progress no longer expands legacy `repairActionPackage` progress, target set, and requiredOutput as active prompt payload. Provider context budgeting now treats `currentActionContract` as P0/protected runtime contract and applies current-action input narrowing before hard-limit compaction, dropping broad P2 history from provider input while preserving current focus, required output and compact diagnostic lineage. |
| P1 State-Change Output Quality Gate | code-closed-first-slice | State-change evidence now goes through coverage evidence quality sufficiency before satisfying `state_change`: invalid/inconclusive/zero-work or target-unaligned state changes are retained only as supporting lineage and cannot close required state-change evidence. State-change target refs now include structured coverage targets and coverage work item IDs. Added coverage ledger regressions for zero-work and target-unaligned state changes. Existing patch contract regressions cover placeholder/non-substantive patch rejection before side effects. |
| P1 Live Chain Regression | partial | Projection same-source live-deviation class now has HTTP regression coverage; state-change violation -> narrow retry -> accepted patch/governed blocker e2e fixture remains pending. |
| P2 Resource Guard | partial | Added request-byte regression for current-action narrowing: large graph/history inputs with `currentActionContract` are materially reduced before hard-limit compaction and no longer retain broad `graphEvidence` / `processEvidenceTimeline` in provider input. Projection metadata-only paths and scan-count regression remain pending. |

## Verification Plan

Run targeted tests:

```bash
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1
```

Run boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|provider|ws-[0-9a-f]" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg -g '*.go'
rg -n "requiredOutput|repairActionPackage|convergenceContract|gapBaseline|stagePacket|currentActionContract" internal/modules/kernel internal/modules/blocks/llm -g '*.go'
```

Live verification:

1. Confirm deployed commit and binary.
2. Trigger two workspaces.
3. Verify workspace list/detail/prompt mode share blocker and projection revision.
4. Verify state-change retry request has one active required output.
5. Verify read-only graph violation produces bounded retry or governed blocker.
6. Verify accepted state change produces evidence refs bound to current focus.
7. Verify request bytes and event metadata scan counters do not grow continuously in blocked/idle states.

## Done Criteria

- Detail/list/TUI prompt cannot disagree on decision/approval/running state when consuming the same projection.
- Provider-facing input contains one active current action contract.
- Retry prompt is narrow, focus-scoped and materially smaller than the original broad context.
- Contract violation output remains rejected diagnostic evidence only.
- Accepted state change must bind to coverage/evidence ledger and pass quality gate.
- Projection does not repair owner state or overlay stale ViewModel onto fresh detail state.
- Tests cover the exact live deviation class.
- Core remains business-neutral and contains no template, language, command, provider or live workspace-specific branches.
