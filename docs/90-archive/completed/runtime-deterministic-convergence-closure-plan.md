# Runtime Deterministic Convergence Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-validation-evidence-context-closure-plan.md`
- `runtime-decision-eligibility-narrow-retry-closure-plan.md`
- `current-action-contract-authority-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`

## Purpose

本文基于最新 live workspace 核对结果，继续收口 WorkOrch runtime 收敛链路。核心目标是让 runtime owner 基于 `coverage/evidence ledger -> deterministic NextAction -> Stage Packet -> contract gate -> accepted/rejected evidence -> ledger recompute` 确定性推进，而不是在 LLM 输出不稳定、assessment 不完整或 validation graph 违例时反复进入 decision/replan。

本文只定义下一轮代码收口范围。后续每项代码修改必须引用本文对应条目，并保留 WorkOrch core 业务中立：core 只能实现通用状态机、ledger、NextAction、contract gate、managed validation 和 projection/ViewModel 能力；软件开发相关的交付策略、验证偏好、版本控制快照和服务验证习惯必须通过 workspaceTemplate/profile/Soul/policy 数据进入。

当前阶段不考虑兼容性。若现有实现、测试或端侧行为与本文冲突，应按本文逐项收口。

## Contract Audit

- Scope: `ws-4e83810542ac4d65`、`ws-7ced70e98ea50c43` 的 runtime convergence、contract violation retry、deterministic assessment、validation route、projection/ViewModel 展示链路。
- Observed state:
  - workorchd projection status is `ready`; observed `sourceEventRevision == projectedSourceRevision == 13104` and `workspaceTotal=2`.
  - `ws-4e83810542ac4d65` is `waiting_decision` with active run `run-006-replan-001-replan-001-replan-001` and blocking ticket `run-006-replan-001-replan-001-replan-001-repair-003-of-planning-001-decision`.
  - `ws-4e83810542ac4d65` previously showed all coverage items as evidence-ready, but LLM assessment still produced a decision path that asked for engineering deliverables such as `patch/apply`, while the current action contract was not supposed to allow that direction.
  - `ws-7ced70e98ea50c43` is `waiting_decision` with active run `run-005-replan-001-replan-001-replan-001` and blocking ticket `run-005-replan-001-replan-001-replan-001-contract-retry-001-of-graph-009-003-setup-and-build-2-contract-violation-decision`.
  - `ws-7ced70e98ea50c43` repeatedly produced validation shell graphs using background process and process termination patterns, for example service launch with `&` followed by `kill`. Runtime correctly rejected them as concrete validation contract violations, but recovery still escalated into decision instead of deriving a safer validation route.
  - `GET /api/v1/workorch/workspaces/:workspaceId` currently returns workspace metadata and an empty `workspace.viewModel`; detailed runtime fields are expected from `/snapshot`. This can confuse consumers if Web/TUI mix metadata and projected snapshot sources.
- Expected state:
  - When ledger already has enough accepted evidence for a focus, runtime should deterministically derive `assessment` or `complete` without letting stale LLM assessment request state-changing output.
  - When validation requires a managed service lifecycle, runtime should route by neutral policy to `managed_harness` / `runtime_action` / `deferred` / governed decision before asking LLM to invent unsafe shell lifecycle commands.
  - Contract violations should become rejected diagnostic evidence and bounded retry only when the current NextAction still needs LLM candidate content. If runtime can derive the next action from ledger/policy, it must continue deterministically.
  - TUI/Web should consume a single projected ViewModel revision for workspace list, main view, prompt and details; metadata endpoints must not be treated as business-state authority.
- Deviation:
  - Deterministic coverage/evidence state is still not sufficient in some paths to prevent LLM assessment output from redirecting the flow.
  - Validation route policy exists in code shape, but live flow still allows repeated unsafe LLM shell validation to exhaust into decision.
  - Decision eligibility is still too broad for recoverable contract violations and deterministic validation gaps.
  - Projection is healthy, but client-facing detail access still has two visible shapes: metadata-only workspace detail and projected snapshot detail.
- Owner: WorkOrch core for deterministic convergence, validation routing, contract violation recovery, decision eligibility and ViewModel contract. WorkspaceTemplate/profile/Soul/policy owns software-development-specific validation and deliverable preferences. TUI/Web owns only command submission and projected ViewModel rendering.
- Event path:
  - TUI/Web/REST command -> workorchd EventHub command -> owner state -> runtime authority refresh -> deterministic NextAction -> Stage Packet / LLM or runtime action -> contract gate -> accepted/rejected evidence -> coverage recompute -> projection/ViewModel -> TUI/Web display.
- Contract gap:
  - Missing deterministic assessment/complete fast path from accepted evidence ledger.
  - Missing or incomplete policy-driven validation route enforcement before LLM graph generation and after repeated validation contract violation.
  - Decision gate does not always ask whether a deterministic next action is available before creating/opening a ticket.
  - Workspace detail metadata endpoint and snapshot projection endpoint are not documented or enforced strongly enough as separate contracts.
- Proposed change:
  - Add deterministic assessment/complete gate before LLM assessment can trigger repair/decision.
  - Strengthen validation NextAction route and managed harness/runtime action/deferred decision semantics.
  - Narrow decision eligibility for contract violation exhaustion.
  - Align projection/ViewModel access contract across REST client, TUI backend and Web.
- Boundary check:
  - No core branch may reference template ID, Soul ID, workspace title, language, command name, file name, provider, port, or live workspace ID.
  - Software development validation preferences must be data-driven through template/profile/Soul/policy and consumed by neutral route interpreters.
- Verification:
  - Add failure-sample tests for evidence-ready assessment, repeated validation shell violation, decision eligibility, and ViewModel/detail source consistency.
  - Run boundary searches for business/template/language/command special cases in core.
  - Run focused orchestrator/appservice/TUI backend tests, then full `go test ./...` when feasible.

## Closure Scope

### C1: Deterministic Assessment And Complete Gate

Problem:

- `coverage/evidence ledger` can already prove that a focus is evidence-ready or satisfied, but runtime still asks LLM for assessment in a way that can return unrelated state-changing or repair direction.

Target:

- Runtime owns item-level assessment readiness and completion eligibility.
- LLM assessment is only a candidate explanation/evidence mapping. It cannot override accepted evidence, current focus, required evidence kinds or completion gate.
- If all required evidence kinds are accepted and linked to active coverage items, runtime either:
  - records deterministic assessment evidence and advances to `complete`, or
  - derives a precise missing evidence gap if ledger is incomplete.

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_result_recording.go`

Acceptance:

- A fixture with all coverage items carrying accepted evidence refs cannot produce `patch/apply` required output through assessment.
- `evidence_ready` cannot route to `need_state_change` unless ledger explicitly reports missing `state_change` evidence.
- Deterministic completion records traceable evidence refs and does not use LLM self-claim as the only authority.

### C2: Policy-Driven Validation Route Enforcement

Problem:

- For service, proxy, streaming or long-running validation, LLM repeatedly emits unsafe shell lifecycle commands. Runtime rejects them correctly, but then escalates to decision instead of deriving a safer validation route.

Target:

- Validation NextAction route is chosen before dispatch from neutral policy data:
  - `llm_graph`: LLM may generate bounded validation evidence graph.
  - `managed_harness`: runtime requires `validation/managed_session` or equivalent managed validation metadata.
  - `runtime_action`: runtime performs deterministic validation action from structured policy.
  - `deferred`: runtime records deferred evidence gap or external validation requirement.
  - `decision`: governance only when external user input, permission, credential, environment or manual confirmation is truly required.
  - `fail`: validation cannot be run automatically and no allowed non-LLM route exists.
- Repeated unsafe shell graph violation should cause route narrowing or managed-harness requirement before decision.

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- template/profile/Soul policy data under WorkOrch defaults.

Acceptance:

- Repeated validation graph violation with process background/termination and an available managed route does not directly open decision.
- `validation/managed_session` remains a neutral capability; core only normalizes lifecycle metadata, timeout, scratch and cleanup.
- Software-development-specific guidance is present only in template/profile/Soul/policy data, not in core branch conditions.

### C3: Contract Violation Recovery Decision Eligibility

Problem:

- Contract violation exhaustion can create decision tickets even when runtime has enough structured information to retry with a narrower contract or derive another deterministic action.

Target:

- Decision is allowed only when runtime lacks required external information, permission, approval, credential, configuration or manual choice.
- Rejected LLM output remains diagnostic-only and cannot become accepted evidence, result, artifact or high-value memory.
- Bounded retry is allowed only when current NextAction still needs LLM candidate content.
- If current NextAction is deterministic, runtime refreshes authority and continues without asking LLM to decide state transition.

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_memory.go`

Acceptance:

- Same-rule contract violation writes rejected diagnostic evidence with signature and retry hint.
- Repeated recoverable violation narrows required output or route before decision.
- Decision ticket creation includes a structured eligibility reason; auto-continuable validation/repair gaps cannot create decision.

### C4: Single ViewModel Detail Contract

Problem:

- Workspace list projection is ready, but `GET /api/v1/workorch/workspaces/:workspaceId` returns metadata-only workspace content with empty `viewModel`, while `/snapshot` contains projected runtime detail. Web/TUI must not mix these as equivalent state sources.

Target:

- The metadata endpoint remains metadata-only or is explicitly documented as such.
- Runtime state, prompt mode, current focus, NextAction, governance blockers and coverage summary come from projected snapshot/ViewModel.
- REST client, TUI backend and Web detail page use `/snapshot` for business-state display.
- If a metadata endpoint exposes `viewModel`, it must either be populated from projection or omitted to avoid false authority.

Code areas:

- `internal/modules/application/workorchd/service/server/http_workspace.go`
- `internal/modules/application/workorchd/service/server/workspace_projection.go`
- `internal/pkg/workorchclient/workspace.go`
- `internal/services/workorch/biz/backends/remote.go`
- `internal/services/workorch/service/tui/**`
- `web/workorch-console/src/pages/WorkspaceDetailPage.tsx`

Acceptance:

- Workspace list, detail page, TUI main view and prompt are based on the same projected revision when rendering runtime state.
- Tests fail if TUI/Web uses metadata-only workspace detail as current run/decision/coverage authority.
- Missing projection shows degraded/loading status, not local business-state inference.

### C5: Dispatch-Time Authority Alignment Guard

Problem:

- Multiple derived fields still exist: `coverageFocus`, `gapBaseline`, `convergenceContract`, `stagePacket`, `repairActionPackage.requiredOutput`, `acceptance.currentFocus`. They can drift if any path carries old lineage into dispatch.

Target:

- Before every provider/block/runtime dispatch, one refreshed authority snapshot must drive:
  - current focus
  - deterministic NextAction
  - gapBaseline
  - convergenceContract
  - stagePacket
  - requiredOutput
  - budget scope
- Contradictory derived fields are runtime internal contract errors, not LLM/provider errors.
- Old repair/replan/violation lineage can be diagnostic context only.

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_specs.go`

Acceptance:

- Dispatch rejects or rebuilds stale focus-bearing fields before external execution.
- Replan/resume/provider retry/contract retry/follow-up all pass through the same alignment guard.
- Tests cover stale `repairActionPackage.requiredOutput`, stale `acceptance.currentFocus`, stale `stagePacket.requiredOutput` and stale budget scope.

## Implementation Order

1. C1 deterministic assessment/complete gate.
2. C2 policy-driven validation route enforcement.
3. C3 decision eligibility narrowing for contract violations.
4. C5 dispatch-time authority alignment hardening.
5. C4 ViewModel/detail source alignment.

Rationale:

- C1 and C2 directly target the current live blockers.
- C3 prevents repeated LLM violations from turning into unnecessary governance loops.
- C5 prevents future lineage drift from reintroducing the same class of bug.
- C4 ensures TUI/Web/Web detail views faithfully show owner state after runtime fixes.

## Verification Plan

Focused tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchclient/... ./internal/pkg/workorchport/... -count=1
```

Boundary searches:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|ws-4e838|ws-7ced|LSP-Bridge|LLM 本地代理" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg -g "*.go"
rg -n "nohup|setsid|pkill|killall|fuser -k| kill | kill -| &" internal/modules/kernel/orchestrator/biz internal/pkg/workorchdefaults web docs/70-roadmap/active -g "*.go" -g "*.md"
rg -n "repairActionPackage|requiredOutput|stagePacket|convergenceContract|gapBaseline|coverageFocus|acceptance.currentFocus|convergenceBudget" internal/modules/kernel/orchestrator/biz -g "*.go"
```

Live verification after code closure:

1. Record git commit, binary path, PID, service start time and projection revision.
2. Pick the two active workspaces and record workspace ID, active run ID, blocking ticket/approval, latest event revision and coverage summary.
3. Trigger decision `replan` or `/run resume` only after recording pre-state.
4. Confirm command accepted -> authority refresh -> deterministic NextAction -> dispatch/runtime action/governed wait.
5. Confirm contract violations become rejected diagnostics and bounded retry, not accepted output.
6. Confirm TUI prompt, TUI main view, workspace list and Web detail page display the same projected revision and state.
7. Confirm CPU/IO does not spike due to projection full snapshot hydration on normal state display.

## Done Criteria

- Runtime can complete or derive exact missing gaps from accepted coverage/evidence ledger without relying on LLM self-assessment as authority.
- Validation route is policy-driven and can require managed validation or deterministic runtime action before unsafe LLM shell output repeats into decision.
- Decision tickets are only created for true governance blockers, not auto-continuable contract violations.
- Dispatch receives one aligned authority-derived contract; stale lineage is diagnostic-only.
- Web/TUI consume projected ViewModel for business state and do not infer state from metadata-only workspace detail.
- Core remains business-neutral. Any software development behavior is represented as template/profile/Soul/policy data and consumed through neutral interpreters.
