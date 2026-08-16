# Runtime Target Snapshot Authority And Projection Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-29

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-current-action-projection-convergence-closure-plan.md`
- `runtime-ledger-binding-projection-authority-closure-plan.md`
- `runtime-current-live-convergence-improvement-plan.md`

## Purpose

本文用于收口当前 live 验证中暴露的两个直接阻塞：

1. runtime 在缺少当前目标快照时仍选择 `state_change`，并渲染必须直接 `patch/apply` 的合同，导致 LLM 无法在不猜测文件内容的情况下继续推进。
2. owner state 已经是 `waiting_decision` / `blocked_on_decision`，但 workspace list/detail ViewModel 仍显示 `promptMode=workspace` 或旧 workspace 状态，导致 TUI/Web 展示与权威状态不一致。

目标是按 Goal/DoD NextAction 状态机和 convergence authority 简化设计，将当前偏差收口为业务中立的 runtime 规则：`NextAction` 必须由 authority snapshot 决定，Stage Packet 只能表达当前可满足合同；Projection/ViewModel 只能展示 owner snapshot，不维护第二套业务推断。

## Contract Audit

- Scope:
  - live workspaces `ws-705249a8d7816330/run-010` and `ws-db040d24a190f6e5/run-009`
  - runtime convergence / initial step input / target snapshot acquisition
  - workorchd projection / WorkspaceViewModel prompt mode
- Observed state:
  - Both active runs are `blocked_on_decision`.
  - Current LLM request passed context budget gate: `contextGateAction=pass`, no integrity warnings.
  - `currentActionContract.nextAction.kind=state_change`.
  - `stateChangingOutputRequired=true`.
  - `requiredOutput.acceptedKinds=["patch/apply","shell/command","mcp/tool_call"]`.
  - `requiredOutput.forbiddenKinds` includes `read_only_graph`, `context_rehydration_graph`, and `same_graph_file_read_before_patch`.
  - `repairActionPackage.targetSnapshots` is empty and `stagePacket.targetSnapshots` is empty.
  - LLM returned `blocked_on_decision` because it lacked `targetRef`, `baseHash`, and current file content.
  - The actual workspace directories are not empty and contain source files, but the rendered step input carried `emptyWorkspaceEvidenceEscalatedToImplementation=true`.
  - Detail snapshot endpoint exposes `workspace.state=waiting_decision`, `run.state=blocked_on_decision`, and open decision tickets, while workspace list/detail ViewModel still reports `promptMode=workspace` in the observed response.
- Expected state:
  - If a coverage focus requires state change but runtime lacks a current target snapshot or concrete state-changing target, `NextAction` must be `acquire_target_snapshot` / evidence acquisition, not direct `state_change`.
  - A direct `state_change` contract may be rendered only when the runtime has enough concrete target evidence to make the contract satisfiable.
  - Projection list, detail, TUI prompt, and Web prompt must derive from the same normalized owner snapshot and show `decision` when the active run is blocked by an open decision ticket.
- Deviation:
  - Current runtime can produce an impossible contract: it forbids read-only acquisition and simultaneously requires direct patch output without a current target snapshot.
  - Current projection overlay can expose stale or incomplete ViewModel fields even when owner detail already shows the correct governance blocker.
- Owner:
  - Primary owner is WorkOrch core runtime convergence and appservice projection.
  - This is not a workspaceTemplate, language, file-name, provider, or live workspace special case.
- Event path:
  - `restart/replan/resume command -> owner run activation -> initialStepInput -> refresh convergence authority -> BuildConvergenceContract -> ApplyConvergenceContract -> Stage Packet -> LLM request -> contract gate -> decision ticket -> owner state -> projection/ViewModel -> TUI/Web`.
- Contract gap:
  - Target snapshot availability is not part of the direct `state_change` eligibility gate unless `mustUseCurrentTargetSnapshot` is already set.
  - Existing workspace manifest / target availability can appear later in `executionContextPackage`, but initial state-change escalation may have already classified the step as direct implementation.
  - Projection list/detail ViewModel can preserve stale projected `promptMode` instead of re-deriving display state from normalized owner snapshot.
- Boundary check:
  - The fix must be expressed through generic `state_change`, `target_snapshot`, `coverage focus`, `requiredOutput`, `workspace manifest`, and `governance blocker` concepts.
  - Do not add checks for template IDs, Soul IDs, Go/Python/Node, file names, command names, provider names, or specific workspace IDs.

## Design Decision

### 1. Direct State Change Requires Concrete Target Authority

`state_change` is only eligible when at least one of the following is true:

- The current action is creating a new bounded artifact/file and does not require existing target content.
- The current `repairActionPackage` or stage packet carries a current, untruncated target snapshot with `targetRef` and either full content or a safe base hash.
- Accepted target snapshot evidence exists for the same workspace, run, definition revision, and current target, and it has not been made stale by a later applied patch.
- The current action uses a runtime-managed state-changing block whose input already declares auditable target refs and does not depend on reading unknown current text.

If none of these are true, runtime must not render a direct `state_change` contract. It must render target snapshot acquisition.

### 2. Target Snapshot Acquisition Is A First-Class NextAction

When the ledger says the focus needs `state_change` but target evidence is insufficient, the deterministic NextAction becomes:

```text
type: acquire_target_snapshot
stage: acquire_target_snapshot
stageMode: target_snapshot_acquisition
repairObjective: acquire_concrete_failure_evidence
requiredOutput:
  mustAcquireCurrentTargetSnapshot: true
  mustUseReadOnlySnapshotAcquisition: true
  mustNotMutateWorkspace: true
  acceptedKinds: file/list, file/read, git/read, mcp/resource_read, shell/command when declared read-only
  forbiddenKinds: patch/apply, file/write, state-changing shell/tool calls
```

This action records accepted `target_snapshot` or diagnostic evidence. After evidence is accepted, authority refresh recomputes the next action. If the focus still needs state change and a usable snapshot now exists, the next action becomes direct `state_change`.

### 3. Existing Workspace Manifest Must Be Available Before Eligibility

Initial step input and replan/restart activation must classify the workspace using the same authority snapshot used for dispatch. `workspaceFileManifest` may be carried as top-level step input or inside `executionContextPackage`, but the state-change eligibility check must observe the same manifest view before choosing between:

- target snapshot acquisition for existing workspace content
- direct implementation only for a genuinely empty or new-target flow

The runtime must not set `emptyWorkspaceEvidenceEscalatedToImplementation=true` when a non-empty manifest is already available in the same dispatch input.

### 4. Projection ViewModel Must Be Owner-Snapshot Derived

Projection must normalize workspace governance first, then compute ViewModel from that normalized snapshot. If the active run is `blocked_on_decision` with an open or inferred blocker ticket, list/detail ViewModel must report:

```text
workspace.state: waiting_decision
workspace.blockingTicketId: <ticket>
viewModel.promptMode: decision
```

If projection cannot prove the blocker state, it may expose degraded/stale health, but TUI/Web must not synthesize an alternate business state.

## Closure Tasks

### P0. State-Change Eligibility Gate

- Add a single core-neutral helper for state-change target readiness.
- Inputs:
  - current action / convergence contract
  - repair action package
  - stage packet target snapshots
  - accepted target snapshot evidence metadata
  - workspace manifest availability
- Output:
  - `ready_for_state_change`
  - `requires_target_snapshot_acquisition`
  - diagnostic reason for audit and tests
- Acceptance:
  - A run with missing `state_change` evidence, non-empty manifest, and no target snapshots selects `acquire_target_snapshot`.
  - A run with a current target snapshot keeps `state_change`.
  - A new bounded creation flow can still use direct `state_change` when no existing target content is required.

### P0. Initial Step Input And Authority Alignment

- Ensure `initialStepInput` observes the same manifest and authority data that will be used by Stage Packet rendering.
- Prevent `applyFirstRunGapBaselineStateChangingContract` from escalating to direct implementation when a manifest exists only in `executionContextPackage`.
- Ensure `BuildConvergenceContract` and `ApplyConvergenceContract` preserve `acquire_target_snapshot` after authority refresh and do not overwrite it with stale `stateChangingOutputRequired`.
- Acceptance:
  - Live-shaped input with `executionContextPackage.workspaceFileManifest` and no target snapshots produces `repairObjective=acquire_concrete_failure_evidence`, not `produce_state_changing_output`.
  - `currentActionContract.nextAction.kind` matches `acquire_target_snapshot`.
  - `requiredOutput.forbiddenKinds` forbids mutation and does not forbid read-only acquisition.

### P0. Stage Packet RequiredOutput Rebinding

- Stage Packet must be derived from the selected NextAction, not from stale repair lineage.
- For `acquire_target_snapshot`, `repairActionPackage.requiredOutput`, top-level `requiredOutput`, `stagePacket.requiredOutput`, and `currentActionContract.requiredOutput` must all express evidence acquisition.
- For `state_change`, all four must express direct state-changing output and include usable target snapshot references when required.
- Acceptance:
  - No request may contain `nextAction=state_change` plus empty target snapshots plus forbidden read-only acquisition.
  - No request may contain `nextAction=acquire_target_snapshot` plus `mustIncludeStateChangingNode=true`.

### P0. Decision Gate Eligibility

- `blocked_on_decision` from LLM must be accepted only when the runtime truly lacks information it cannot acquire through available read-only capabilities or governance policy.
- Missing target snapshots in a readable workspace are not human-only by default; they are a runtime acquisition action.
- Acceptance:
  - The two current live blockers would not create human decision tickets; they would route to target snapshot acquisition.
  - If read-only acquisition is impossible because of permissions, missing workspace, credentials, or external dependency, a decision ticket remains valid and includes structured missing fields.

### P0. Projection / ViewModel Same-Source Fix

- Normalize governance projection before computing list and detail ViewModel.
- Ensure list view, detail view, REST detail overlay, SSE projection, and TUI prompt use the same `WorkspaceViewModel.promptMode`.
- Do not keep a stale `promptMode=workspace` when normalized owner snapshot says `waiting_decision`.
- Acceptance:
  - Seeded workspace with `RunStateBlockedOnDecision` and blocking ticket projects list/detail `promptMode=decision`.
  - Detail endpoint and workspace list return the same prompt mode and compatible workspace state for the same projection revision.

### P1. Accepted Target Snapshot Evidence Loop

- When target snapshot acquisition succeeds, record accepted `target_snapshot` evidence with:
  - workspaceId, runId, stepId, definitionRevision
  - targetRef, baseHash/contentHash, capturedAt
  - coverageWorkItemIds and structuredTargetRefs when available
- Authority refresh must hydrate current target snapshots from accepted evidence before choosing direct `state_change`.
- Acceptance:
  - `acquire_target_snapshot -> accepted target_snapshot -> state_change` works without user decision.
  - A later applied patch invalidates stale snapshot evidence for the same target.

### P1. Tests And Regression Fixtures

Required tests:

- `NextActionForRun` routes missing state-change target to `acquire_target_snapshot`.
- Initial step input with non-empty manifest but no snapshots renders evidence acquisition.
- Direct state-change remains selected when target snapshot exists.
- Current action contract cannot be `state_change` with empty target snapshots and read-only acquisition forbidden.
- Projection list/detail prompt mode is `decision` for blocked decision owner state.
- Decision gate does not escalate missing target snapshot when runtime can acquire it.

### P1. Live Verification

After code closure and redeploy:

1. Restart/replan a workspace with existing files and no current target snapshot.
2. Confirm first new LLM request has:
   - `currentActionContract.nextAction.kind=acquire_target_snapshot`
   - `repairObjective=acquire_concrete_failure_evidence`
   - mutation forbidden
   - read-only acquisition allowed
3. Confirm target snapshot evidence is accepted.
4. Confirm next request switches to `state_change` with current target snapshots.
5. Confirm no immediate human decision ticket is created for missing target snapshot.
6. Confirm TUI/Web prompt mode follows projection: `decision` only when owner state is blocked on an open decision; otherwise `running` or `workspace` as appropriate.

## Code Access Inventory

Likely code areas:

- Runtime NextAction / convergence:
  - `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_current_action_contract.go`
- Initial input / Stage Packet:
  - `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- Decision eligibility / contract violation:
  - `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- Evidence recording and reuse:
  - `internal/modules/kernel/orchestrator/biz/runtime_evidence_records.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`
- Projection/ViewModel:
  - `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/store.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_governance_projection.go`
  - `internal/modules/application/workorchd/service/server/workspace_projection.go`
  - `internal/modules/application/workorchd/service/server/http_workspace.go`

## Non-Goals

- Do not relax the state-changing contract to allow same-graph file reads before patch.
- Do not let LLM decide whether to switch from `state_change` to evidence acquisition.
- Do not add software-development-specific, language-specific, file-name-specific, provider-specific, or workspace-ID-specific logic in core.
- Do not use watchdog or periodic recovery to paper over wrong NextAction selection.
- Do not make TUI/Web infer decision/running state locally.

## Verification Commands

Targeted tests after implementation:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'NextAction|Convergence|CurrentAction|TargetSnapshot|StagePacket|DecisionGate' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore ./internal/modules/application/workorchd/service/server -run 'ViewModel|Prompt|Decision|Projection' -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|main.go|handler.go|lsp|provider|ws-705249|ws-db040" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
```

Live verification evidence:

- request log for the acquisition step
- accepted target snapshot evidence record

## Closure Progress

Status after code closure:

- P0 state-change eligibility is implemented through the shared target snapshot acquisition gate. A non-empty workspace manifest without a usable target snapshot now keeps the next action on `acquire_target_snapshot` instead of rendering an impossible direct `state_change` contract.
- P0 initial step input alignment is implemented in LLM step input enrichment. If the execution context package adds a manifest after the initial input was built, the step input is rechecked and rebound to the selected authority action before Stage Packet and Current Action Contract are attached.
- P0 requiredOutput rebinding is implemented for target snapshot acquisition. `repairActionPackage.requiredOutput`, Stage Packet, and Current Action Contract preserve read-only acquisition flags and do not carry direct state-changing requirements.
- P0 projection same-source behavior is implemented. Workspace metadata/detail projection derives prompt mode from normalized owner governance state and no longer preserves a stale projected `promptMode=workspace` when owner state is waiting on a decision blocker.
- P1 accepted target snapshot evidence hydration already exists in `NextActionForRun`; targeted regression tests cover acquisition first, accepted snapshot hydration, and stale snapshot invalidation after a later patch.

Remaining validation:

- Live verification must confirm a real workspace moves `acquire_target_snapshot -> accepted target_snapshot -> state_change` without opening a human decision for missing target content.
- Live projection verification must confirm workspace list, detail, TUI prompt, and Web prompt report the same prompt mode for the same owner snapshot revision.
- following state-change request log with populated target snapshots
- projection/list/detail prompt mode snapshots
- metrics showing no queue pressure or projection rebuild loop

## Completion Criteria

This closure is complete only when:

- Current live-shaped cases no longer produce impossible `state_change` contracts.
- Missing target snapshot becomes deterministic acquisition, not a human decision, when runtime has read-only capability.
- Direct `state_change` remains strict and auditable once target snapshot evidence exists.
- Projection list/detail/TUI/Web prompt mode all come from the same owner-derived ViewModel.
- All changes remain business-neutral and covered by targeted tests plus boundary search.
