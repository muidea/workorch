# Runtime Validation Contract Gate And Managed Session Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-nextaction-validation-to-statechange-closure-plan.md`
- `runtime-validation-evidence-context-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`

## Purpose

This document captures the next closure scope after live verification showed that
the convergence authority flow can advance one workspace while another
workspace is blocked by repeated validation contract rejections.

The goal is to keep the WorkOrch core business-neutral while making the runtime
validation gate deterministic, precise and recoverable:

```text
Current Focus
  -> NextAction(validate)
  -> requiredOutput(concrete validation evidence)
  -> LLM candidate validation graph
  -> contract gate
  -> managed validation or safe shell admission
  -> accepted/rejected evidence
  -> ledger recompute
```

This plan does not add software-development-specific rules. It improves the
neutral contract gate, validation route selection, retry classification and
runtime IO/resource controls.

## Contract Audit

- Scope:
  - `ws-7ced70e98ea50c43`
  - `run-007-replan-001-replan-001`
  - latest failed step:
    `run-007-replan-001-replan-001/contract-retry-001-of-planning-001-3`
  - open decision:
    `run-007-replan-001-replan-001-contract-retry-001-of-planning-001-3-contract-violation-decision`
- Observed state:
  - Workspace state: `waiting_decision`
  - Run state: `blocked_on_decision`
  - Current focus: `cwi-014`
  - NextAction: `request_decision`
  - Latest contract rule: `concrete_validation_contract_missing`
  - Retry path attempted contract retry three times after the initial step.
  - Latest response declared `validationEvidence.mode=controlled_substitute`,
    `validationEvidence.observationType=controlled_substitute_observation` and
    wrote to a child path under `/tmp/workorch-validation-scratch/...`.
  - Runtime still rejected the candidate with:
    `concrete validation evidence graph must not mutate workspace files or write outside validationScratchDir through shell/command`.
  - `managedValidationAvailable=true`, but the retry still used a large
    `shell/command` here-doc validation harness.
  - Process resource observation during live run: `workorch` RSS approached
    1GB and `.workorch-data` reached about 3.6GB.
- Expected state:
  - A candidate that declares concrete validation evidence and confines writes
    to runtime validation scratch should either be admitted or rejected with a
    precise reason that can be corrected deterministically.
  - If shell safety cannot be proven for a complex validation harness, runtime
    should route the next retry to managed validation instead of repeatedly
    asking the LLM to rewrite large shell here-docs.
  - Repeated same-focus validation contract failures should produce a narrowed
    route change such as `managed_validation_required`, not only
    `retry_same_contract`.
- Deviation:
  - The shell safety gate scans the whole `sh -c` script as flat text and can
    treat here-doc body content as shell syntax.
  - Embedded source code inside here-docs can contain tokens such as `>`,
    `rm`, `mkdir`, `cp` or `kill` that are not shell operations, causing false
    `write_outside_validationScratchDir` rejections.
  - The retry hint remains too broad, so the LLM returns another large shell
    harness rather than switching to a managed validation session.
- Owner:
  - Primary: WorkOrch core contract gate and runtime validation route
    selection.
  - Secondary: projection/event IO retention policy for resource control.
  - Not owner: workspaceTemplate-specific software development policy,
    provider behavior, TUI/Web local state.
- Event path:
  - LLM step -> candidate `executionGraph` -> `validateLLMExecutionOutputContract`
    -> `validateConcreteValidationGraphShellSafety` -> rejected diagnostic
    evidence -> focus-scoped contract retry -> budget exhaustion -> decision
    projection.
- Contract gap:
  - Shell safety inspection lacks a neutral parser/preprocessor that ignores
    here-doc body text.
  - Required output does not force managed validation after repeated complex
    shell safety failures.
  - Contract violation classes do not distinguish missing validation metadata,
    true scratch escape, unsafe process control and scanner uncertainty.
  - Resource retention does not yet enforce the long-session IO strategy
    strongly enough for live multi-round execution.

## Design Alignment

### Goal/DoD NextAction Design

The current live state confirms that coverage ledger, current focus and
NextAction are active. One workspace progressed through multiple validation
focuses. The gap is inside the candidate gate:

- Contract gate must stay strict.
- Invalid candidates must remain rejected diagnostic evidence.
- But a validly declared candidate must not be rejected by a known false-positive
  scanner.
- Runtime must use deterministic route changes when a recoverable violation
  repeats.

### Convergence Authority Simplification

The issue is not stale authority or projection drift. The authority path
correctly escalated after repeated same-rule failures. The remaining problem is
that the rejected diagnostic is too coarse, so deterministic NextAction cannot
choose a better validation route.

The closure must preserve the simplified authority model:

- `Coverage/Evidence Ledger` remains the authority.
- `requiredOutput` is derived from current `NextAction`.
- Old failed outputs stay diagnostic-only.
- Projection exposes the blocker but does not repair owner state.

## Required Changes

### P0: Shell Safety Scanner Must Ignore Here-Doc Bodies

Current risk:

- `runtime_execution_contract.go` treats the whole shell command line as flat
  text.
- Here-doc body content is scanned as if it were shell syntax.
- A source-code comparison such as `idx > 0` can be classified as shell output
  redirection.

Target behavior:

- Before applying mutation/redirection markers, normalize shell command text by
  removing or masking here-doc bodies.
- Preserve here-doc opener and terminator lines enough to keep syntax boundaries
  visible.
- Continue detecting real shell redirections outside scratch.
- Continue rejecting actual process termination, detached processes and writes
  outside validation scratch.

Implementation scope:

- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract_integration_test.go`

Acceptance:

- A shell command with `cat > "$scratch/probe.go" <<'EOF'` and Go source body
  containing `idx > 0` is allowed when all write targets are under
  `validationScratchDir`.
- A shell command with `printf ok > validation.log` is still rejected.
- A here-doc that writes to a non-scratch target remains rejected.
- Existing process termination and detached process tests still pass.

### P0: Add Focused Regression Fixture From Live Failure Shape

Target behavior:

- The regression test should model the live failure class without hard-coding
  workspace IDs, business names or files.
- The fixture should use neutral names and a generic controlled substitute
  harness.

Implementation scope:

- `runtime_execution_contract_integration_test.go`

Acceptance:

- Test fails on the current false-positive scanner.
- Test passes after here-doc body masking.
- The fixture contains no template ID, language-specific policy branch or live
  workspace ID condition.

### P1: Route Repeated Complex Shell Validation To Managed Session

Current risk:

- `managedValidationAvailable=true` is exposed, but the retry contract still
  accepts a large raw shell harness.
- LLM repeatedly returns shell scripts that are hard to statically prove safe.

Target behavior:

- After the same focus receives repeated shell safety contract violations,
  runtime narrows required output to `validation/managed_session`.
- When `mustUseManagedHarness=true`, plain `shell/command` candidates are
  rejected with `managed_validation_required`.
- The route change is focus-scoped and does not affect unrelated validation
  steps.

Implementation scope:

- `runtime_contract_violation.go`
- `runtime_repair_action_package.go`
- `runtime_execution_contract.go`
- `runtime_next_action.go` or the current equivalent NextAction-to-contract
  renderer

Acceptance:

- First recoverable validation contract violation may retry the same contract.
- Second repeated shell safety failure for the same focus switches retry hint
  and requiredOutput to managed validation.
- Plain shell candidate under managed route is rejected with a precise
  `managed_validation_required` reason.
- `validation/managed_session` normalizes to runtime-managed shell execution
  with scratch, timeout, cleanup and optional port allocation metadata.

### P1: Split Validation Contract Violation Classes

Current risk:

- Multiple distinct failures collapse into `concrete_validation_contract_missing`.
- Retry hints become too generic.

Target classes:

- `validation_evidence_metadata_missing`
- `validation_scratch_required`
- `validation_scratch_escape`
- `validation_process_control_forbidden`
- `validation_detached_process_forbidden`
- `validation_shell_safety_unproven`
- `managed_validation_required`

Rules:

- Missing `validationEvidence.mode` or `observationType` maps to
  `validation_evidence_metadata_missing`.
- A concrete write candidate without runtime scratch maps to
  `validation_scratch_required`.
- A proven write outside scratch maps to `validation_scratch_escape`.
- `kill`, `pkill`, `killall`, `fuser -k` maps to
  `validation_process_control_forbidden`.
- `nohup`, `setsid` or background `&` maps to
  `validation_detached_process_forbidden`.
- Parser uncertainty for a complex shell should prefer
  `validation_shell_safety_unproven` and route to managed validation, not a
  misleading scratch escape.

Implementation scope:

- `runtime_execution_contract.go`
- `runtime_contract_violation.go`
- `runtime_validation_diagnostic.go`
- `internal/pkg/workorchruntimecontract/validation_diagnostic.go`
- Workorch client/port DTOs if exposed in projection.

Acceptance:

- Rejected diagnostic evidence contains the specific class.
- Projection can show the specific class without local TUI/Web inference.
- Retry hint is class-specific.

### P1: Managed Validation RequiredOutput Contract

Target behavior:

- Required output must make the managed route obvious and narrow.
- The LLM should no longer need to generate a full validation harness when
  runtime can provide scratch, timeout, cleanup and port allocation.

RequiredOutput additions:

```json
{
  "validationRoute": "managed_harness",
  "mustUseManagedHarness": true,
  "acceptedKinds": ["validation/managed_session"],
  "managedValidation": {
    "scratchRequired": true,
    "timeoutRequired": true,
    "cleanupPolicy": "process_group_on_timeout_or_completion",
    "portAllocation": "optional"
  }
}
```

Rules:

- These fields are neutral runtime capability fields.
- Template/profile may influence when managed validation is preferred, but core
  should route based on repeated safety failures and required evidence type.
- Existing `shell/command` remains valid for simple bounded read-only or
  scratch-provable validation.

Acceptance:

- LLM context C3/C4 includes the narrowed managed route only when current
  NextAction requires it.
- C0 stable prefix remains unchanged by managed route selection.
- Cache partition tests prove route changes stay in volatile/stage sections.

### P2: Resource And IO Retention Guardrails

Current risk:

- Live `.workorch-data` reached about 3.6GB.
- `workorch` RSS approached 1GB during active multi-round runs.
- LLM request/response, checkpoint and event payloads are likely retained at a
  granularity that is too heavy for long-session execution.

Target behavior:

- Projection default reads metadata-only payloads.
- LLM raw request/response payloads are retained by reference and hydrated only
  for explicit observability/debug endpoints.
- Stream progress remains an in-memory/progress signal and does not force full
  projection fan-out.
- Event timelines use recent complete data plus historical summaries and
  evidence ledger refs.
- Large checkpoint payload retention has a bounded policy.

Implementation scope:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/pkg/repository/file/**`
- LLM call persistence/read APIs
- Existing resource closure plan:
  `projection-event-io-resource-closure-plan.md`

Acceptance:

- Normal workspace list and snapshot projection do not hydrate raw LLM payloads.
- Observability endpoint can still fetch full request/response by ref.
- Long-run projection test proves timeline/event reads are metadata-only by
  default.
- Live verification records RSS and IO before/after.

## Work Items

### S1: Here-Doc Scanner Closure

- Add shell here-doc body masking helper.
- Apply it before mutation/redirection scanning.
- Keep process-control scanning on executable shell lines.
- Add regression tests for scratch-safe here-doc, unsafe non-scratch redirect and
  process-control rejection.

### S2: Validation Violation Class Split

- Introduce specific violation classes.
- Map current contract gate errors to specific classes.
- Preserve existing public behavior where necessary through generic summaries,
  but expose the specific class in diagnostics.

### S3: Managed Validation Narrowing

- Add retry classification that detects repeated shell safety failures under the
  same focus.
- Narrow requiredOutput to managed validation.
- Reject plain shell when the managed route is mandatory.
- Add focus-scoped budget tests.

### S4: Context And Cache Boundary Tests

- Verify managed route fields render in C3/C4 only.
- Verify C0 stable prefix hash does not change when validation route changes.
- Verify rejected diagnostics remain diagnostic-only and do not enter accepted
  evidence or memory.

### S5: Resource Retention Follow-Up

- Audit normal projection read paths for raw payload hydration.
- Add metadata-only default tests where missing.
- Define retention/compaction policy for LLM request/response and checkpoints.
- Record live resource verification after code closure.

## Non-Goals

- Do not weaken contract gate to accept unbounded shell.
- Do not execute rejected LLM output.
- Do not add branches for a template, programming language, command, file name,
  provider or live workspace ID.
- Do not use projection to repair owner state.
- Do not make TUI/Web infer recovery actions locally.

## Verification Plan

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|main.go|main_test.go|LSP|provider routing|local proxy" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "validation_scratch_escape|validation_shell_safety_unproven|managed_validation_required|validation/managed_session" internal/modules/kernel internal/pkg docs/70-roadmap/active/runtime-validation-contract-gate-managed-session-closure-plan.md
rg -n "Cache-Control|cache_control|stablePrefixHash" internal/modules/blocks/llm internal/modules/kernel
```

Live verification:

1. Start workorchd and record PID, start time and git commit.
2. Resume or replan a workspace that reaches validation-only focus.
3. Confirm scratch-bounded validation here-doc is admitted or routed to managed
   validation.
4. Confirm repeated shell safety failures switch to managed validation before
   decision escalation.
5. Confirm rejected outputs remain diagnostic-only.
6. Confirm projection revisions remain aligned.
7. Record RSS, CPU, IO and `.workorch-data` growth before and after execution.

## Done Criteria

- Scratch-safe here-doc validation is not falsely rejected.
- True writes outside `validationScratchDir` remain rejected.
- Repeated complex shell validation failures route to managed validation.
- Contract violation diagnostics expose specific neutral classes.
- Managed validation route does not alter C0 stable prompt/cache prefix.
- Projection and IO paths remain read-model/metadata-first by default.
- Core remains business-neutral.
