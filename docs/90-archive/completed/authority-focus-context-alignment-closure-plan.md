# Authority Focus Context Alignment Closure Plan

Status: implemented

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-deterministic-convergence-closure-plan.md`
- `focus-scoped-assessment-managed-validation-context-closure-plan.md`

## Purpose

本文记录 2026-06-27 live verification 后暴露的收敛偏差，并将后续代码收口拆成可逐项验收的任务。目标是继续落实 Goal/DoD NextAction 状态机与 Convergence Authority 简化设计，避免继续围绕单个 LLM 输出、某个 workspace 或某条 prompt 做局部补丁。

本文当前阶段不考虑兼容性。若现有 runtime input、projection、template policy 或测试与本文冲突，应按本文逐项收口。

## Contract Audit

- Scope:
  - `ws-4e83810542ac4d65`
    - run `run-006-replan-001-replan-001-replan-001-replan-001-replan-001`
    - failed step `contract-retry-001-of-planning-001-2`
  - `ws-7ced70e98ea50c43`
    - run `run-005-replan-001-replan-001-replan-001-replan-001-replan-001`
    - failed validation step `graph-002/001-validate-cwi-013-sensitive-header-redaction`
    - active repair step `repair-001-of-graph-002-001-validate-cwi-013-sensitive-header-redaction`
- Observed state:
  - `ws-4e...` reached `failed`; runtime rejected DoD assessment with `focus-scoped dodAssessment item is outside current focus`.
  - `ws-4e...` current authority focus was `cwi-001`, `NextAction=assessment`, but LLM assessment still produced a satisfied item outside current focus. Runtime recorded `coverage_map_incomplete` and terminated after bounded retry.
  - `ws-7...` remained `running`; current focus was `cwi-013`, `NextAction=validate`, and validation failed because generated Go validation harness invoked `go build` without disabling VCS stamping in a scratch copy.
  - `ws-7...` validation input contained mixed focus-bearing fields: `stagePacket.activeFocus` / `coverageWorkItemContext.currentFocus` pointed at `cwi-013`, while `acceptance.activeSlice/currentFocus` still contained `cwi-085`.
  - Projection source and projected revisions were aligned; TUI/Web projection was not the immediate blocker.
  - `workorch` process CPU remained high during live execution, and full `/api/v1/workorch/runs` was expensive.
- Expected state:
  - One refreshed convergence authority snapshot drives current focus, deterministic NextAction, Stage Packet, requiredOutput and all focus-bearing C3/C4 fields before provider or block dispatch.
  - C3 Stage Packet is the only current action/focus contract visible to LLM. Old acceptance focus, old coverage focus, old repair package and old lineage may be included only as diagnostic lineage.
  - Focus-scoped assessment can only assess current `coverageWorkItemIds`; if input contains contradictory focus-bearing fields, runtime fails fast as internal contract misalignment before calling LLM.
  - Template-specific validation environment behavior is provided by workspaceTemplate/profile/Soul/policy, while core only executes neutral validation capabilities.
- Deviation:
  - Dispatch input can still expose multiple focus-bearing current signals.
  - Assessment retry still leaves enough global or stale context for LLM to select out-of-focus coverage items.
  - Software-development validation environment requirements are not fully represented as template policy consumed by generic runtime validation.
  - Runtime/read model still has high-cost paths that can amplify long-session CPU/IO.
- Owner:
  - Primary: WorkOrch core runtime convergence authority and LLM context rendering.
  - Secondary: software-dev workspaceTemplate/profile/Soul validation policy.
  - Secondary: workorchd projection/read model resource usage.
- Event path:
  - TUI/Web/REST command -> workorchd owner -> scheduler -> convergence authority refresh -> Stage Packet/input rendering -> LLM/block output -> contract gate -> accepted/rejected evidence -> ledger recompute -> projection/ViewModel.
- Contract gap:
  - Focus-bearing derived fields are not yet fully single-sourced at dispatch.
  - Assessment requiredOutput does not yet enforce a sufficiently narrow current focus visible surface.
  - Validation environment guidance for software-dev is not fully data-bound through template policy.
  - Heavy all-run reads remain available on the hot service path.
- Boundary check:
  - Core changes must stay business-neutral and use structured focus, evidence, stage, NextAction, requiredOutput and diagnostic lineage fields.
  - Go-specific validation behavior belongs in software-dev template/profile/Soul/policy or e2e fixtures, not `kernel/orchestrator`.

## Closure Principles

- `stagePacket.activeFocus` is the only current focus contract for provider/block dispatch.
- Any field named or shaped like current focus must either be rebound from the same authority snapshot or explicitly marked diagnostic-only.
- LLM contract violations remain rejected diagnostic evidence; they do not satisfy coverage, write result/artifact, or become high-value memory.
- Runtime should deterministically compute any assessment that can be computed from accepted evidence. LLM assessment is candidate explanation, not coverage authority.
- Template-specific validation environment knobs are declarative data consumed by generic validation rendering.

## Scope

### Core Runtime

- Dispatch input rendering and pre-dispatch validation.
- Stage Packet / requiredOutput / repairActionPackage focus binding.
- LLM context C3/C4 partition rendering.
- Contract violation retry input construction.
- Assessment gate and completion gate.

### Template / Policy

- `software-dev` validation environment policy.
- Template-bound validation harness guidance for scratch builds, isolated caches, and managed validation sessions.

### Projection / Resource

- Read model endpoints that may hydrate all runs or all history.
- Authority diagnostics projection for focus drift.

## Work Items

### P0. Single Current Focus Visible Surface

Status: completed

Problem:

- Live input showed `stagePacket.activeFocus=cwi-013` while `acceptance.currentFocus=cwi-085`.
- This violates the design requirement that C3 Stage Packet is the only current focus/action authority.

Required changes:

- Before provider/block dispatch, rebuild all focus-bearing fields from the refreshed authority snapshot.
- If an old focus-bearing field must remain for traceability, move it under a diagnostic lineage container and remove it from current action paths.
- Treat contradictory current focus fields after refresh as `runtime_internal_contract_misalignment`.

Affected areas:

- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair*.go`

Acceptance:

- Fixture with `stagePacket.activeFocus=cwi-013` and `acceptance.currentFocus=cwi-085` is corrected before dispatch or fail-fast before provider call.
- No provider request contains parallel current focus authorities.

Implementation:

- `runtime_gap_baseline.go` now aligns `acceptance.currentFocus` and `acceptance.activeSlice` from convergence authority before dispatch.
- Stale acceptance focus/slice data is demoted into `diagnosticLineage.staleAcceptance`.
- Dispatch validation rejects stale top-level or executionContext acceptance focus/slice as `runtime_internal_contract_misalignment`.

### P1. Focus-Scoped Assessment Narrowing

Status: completed

Problem:

- `ws-4e...` failed because LLM returned a satisfied DoD assessment item outside current `cwi-001`.
- Runtime correctly rejected the output, but repeated retry still did not converge.

Required changes:

- Render assessment requiredOutput with explicit `coverageWorkItemIds` and `allowedTargetRefs` for only current focus.
- Remove or demote global unresolved Goal/DoD text from focus-scoped assessment C4 current-action sections.
- Prefer deterministic runtime assessment when current focus already has required accepted evidence kinds.
- Contract retry for assessment must not reintroduce old global gap text as current action authority.

Affected areas:

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_completion*.go`

Acceptance:

- Fixture based on `focus-scoped dodAssessment item is outside current focus` rejects the bad candidate and produces a next retry input that only exposes `cwi-001` as current focus.
- If accepted evidence is sufficient, runtime computes assessment/next state without asking LLM to restate global coverage.

Implementation:

- Assessment-state dispatch now renders a focus-scoped `requiredOutput` with `assessmentScope=current_focus`, `coverageWorkItemIds`, `acceptedEvidenceRefs`, `allowedTargetRefs`, and executable-output forbiddance.
- `stagePacket.requiredOutput`, top-level `requiredOutput`, and `repairActionPackage.requiredOutput` now use the same assessment contract.
- Existing coverage acceptance gate already accepts `evidence_ready` items with accepted evidence refs as deterministic readiness; this closure prevents LLM assessment retry from re-expanding the current action to global coverage.

### P2. Diagnostic Lineage Demotion

Status: completed

Problem:

- Old repair packages, old decision/replan lineage and compacted historical gaps can still appear next to current action data.
- This increases LLM confusion and risks stale authority reuse.

Required changes:

- Introduce or normalize a single `diagnosticLineage` section for old repair/replan/violation/governance history.
- Ensure C4 current-action renders only current authority-derived values.
- Ensure rejected diagnostics remain available by sourceRef but do not compete with current requiredOutput.

Affected areas:

- `internal/modules/kernel/orchestrator/biz/runtime_repair*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_prompt_context*.go`
- `internal/modules/blocks/llm/**`

Acceptance:

- Request log for assessment/repair shows stale lineage only in diagnostic lineage or trace refs.
- `stablePrefixHash` remains unaffected by runtime retry diagnostics.

Implementation:

- Stale acceptance current-action data is moved under `diagnosticLineage.staleAcceptance` instead of remaining next to C3/C4 current action fields.
- The current action surface remains authority-derived; rejected diagnostics and stale lineage are retained only for traceability.

### P3. Template-Bound Validation Environment Policy

Status: completed

Problem:

- `ws-7...` validation failed because generated scratch validation used `go build` in a context where VCS stamping failed.
- The fix is Go/software-dev specific and must not become core logic.

Required changes:

- Add software-dev template/profile/Soul guidance for validation environment defaults, such as scratch directory isolation, cache env, managed validation preference, and language-specific build flags.
- Represent this as template-bound policy data or Soul text that is bound into workspace/run context.
- Generic runtime only consumes structured validation policy and does not inspect command text or language names.

Affected areas:

- `internal/pkg/workorchdefaults/defaults/templates.json`
- `internal/pkg/workorchdefaults/defaults/souls/software-dev/validation.md`
- Workspace activation/template binding code if policy is not currently persisted into workspace/run context.

Acceptance:

- software-dev validation prompt/context includes the template-bound validation environment policy.
- Boundary search confirms no Go command or `-buildvcs=false` special case is added to `internal/modules/kernel/orchestrator`.
- Existing workspace activation or template-binding refresh can make the policy visible to new runs.

Implementation:

- `software-dev` default template now declares `profileContracts.software-dev.validationEnvironmentPolicy`.
- The policy includes scratch isolation, bounded foreground validation, managed-session preference, and Go scratch-build `GOFLAGS=-buildvcs=false`.
- `soul-software-dev-validation` now instructs validation to consume the policy from `executionContract.profileContracts`.
- Appservice activation tests verify the policy is bound into new software-dev run `executionContract`.

### P4. Managed Validation Preference for Long-Running/Service Checks

Status: completed

Problem:

- Software-dev validations for services and local proxies frequently require bounded foreground process management, temporary ports, scratch dirs and cleanup.
- Raw shell validation increases chances of brittle generated harnesses.

Required changes:

- Strengthen template policy that service/runtime behavior validation should prefer `validation/managed_session` when port allocation, timeout, process cleanup or scratch isolation is needed.
- Core remains neutral: it exposes managed validation capability and validates declared evidence metadata.

Affected areas:

- software-dev template/profile/Soul policy.
- Existing managed validation examples/tests if policy binding is incomplete.

Acceptance:

- LLM input for software-dev service validation advertises `validation/managed_session` as preferred when prerequisites match.
- Core tests remain template-neutral.

Implementation:

- The software-dev profile policy declares `preferManagedSessionFor` service/proxy/streaming/long-running/temporary-port/process-cleanup validation.
- The software-dev validation Soul repeats the preference while keeping core runtime neutral.

### P5. Projection and Resource Guardrails

Status: completed

Problem:

- Full `/api/v1/workorch/runs` was expensive and returned tens of MB before timing out.
- Long sessions with many request/input files still risk CPU/IO pressure.

Required changes:

- Ensure normal workspace list/snapshot paths read projection/read model and do not hydrate all run payloads.
- Add pagination, metadata-only default, or explicit observability mode for all-run endpoints if missing.
- Preserve complete observability through scoped/on-demand reads.

Affected areas:

- `internal/modules/application/workorchd/service/server/**`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`

Acceptance:

- Normal TUI/Web projection reads avoid full payload hydration.
- Full run list requires explicit bounded pagination or metadata-only behavior.
- CPU/IO live verification records lower idle and blocked-state cost.

Implementation:

- HTTP run list endpoints now return metadata-only runs by stripping `executionContext` from list responses.
- Scoped run get/snapshot endpoints remain the explicit detail paths.

## Verification Plan

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
```

Completed verification:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./internal/pkg/workorchdefaults -count=1
go test ./internal/modules/application/workorchd/service/server -run 'TestRunListAndGetEndpointsReturnPersistedRuns' -count=1
git diff --check
```

Boundary verification:

- `GOFLAGS=-buildvcs=false` appears only in software-dev default template/Soul/test data.
- No Go-specific validation branch was added to `internal/modules/kernel/orchestrator`.

Boundary search:

```bash
rg -n "go test|go build|-buildvcs|npm test|software-dev|weather-query|code-review" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "acceptance\\.currentFocus|coverageFocus|repairActionPackage|stagePacket|diagnosticLineage|requiredOutput" internal/modules/kernel/orchestrator/biz -g '*.go'
rg -n "stablePrefixHash|C0|C1|C2|C3|C4|C5" internal/modules/blocks/llm internal/modules/kernel -g '*.go'
```

Live verification:

1. Start latest `workorch` and record pid, git commit and start time.
2. Trigger resume/replan on two workspaces.
3. Verify provider/block dispatch input has one current focus authority.
4. Verify out-of-focus assessment is rejected before side effect and next retry remains focus-scoped.
5. Verify software-dev validation policy is visible in run context without core language special cases.
6. Verify TUI/Web ViewModel revisions remain aligned.
7. Verify CPU/IO drops when no provider/block call is active and full run listing is bounded.

## Done Criteria

- Provider/block dispatch cannot proceed with contradictory focus-bearing current fields.
- Focus-scoped assessment retry exposes only the current coverage work item as current action authority.
- Old repair/replan/violation lineage is diagnostic-only and cannot override current NextAction or requiredOutput.
- software-dev validation environment behavior is template-bound and traceable into run context.
- Core remains business-neutral with no template/language/command/provider/workspace-specific branches.
- Projection and REST list endpoints do not impose unbounded run/history hydration on normal UI paths.
