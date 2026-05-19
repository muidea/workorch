# WorkOrch Runtime Repair Convergence Optimization Design

## 1. Background

Workspace `ws-5823c555519e601c` exposed repeated non-convergent behavior across `run-849b12c224dad205` and `run-849b12c224dad205-replan-001`:

1. Repair loops repeatedly failed with `llm_decision_invalid` due to invalid decision JSON.
2. Patch apply repeatedly failed with `unified diff hunk context mismatch`.
3. Replan produced state changes, but added test artifacts that introduced new compile blockers before existing blockers were cleared.
4. Final validation never reached accepted `Result` / `Artifact` closure.

This document defines optimization scope before code changes.

## 2. Scope And Constraints

1. Keep WorkOrch core business-neutral.
2. Do not hardcode software-domain semantics in reusable runtime core.
3. Keep domain-specific policy in `workspaceTemplate` / profile contract / Soul.
4. Do not directly patch workspace deliverable outputs as a "fix strategy"; improve core runtime and template guidance.
5. Current stage is non-compat mode: prioritize correct target behavior over backward compatibility.

## 3. Problem Decomposition

### P1. LLM Decision Schema Instability

Symptoms:
- `invalid llm decision json: invalid character '}' after array element`
- Repair budget consumed by parse failures instead of effective state change.

Root gap:
- Runtime relies on one-shot strict structured parse without enough deterministic salvage path.

### P2. Patch Grounding Mismatch

Symptoms:
- `patch/apply` failed with `unified diff hunk context mismatch`.

Root gap:
- Generated diff not always grounded on the latest exact target snapshot/hash used by dispatcher.

### P3. Wrong Repair Priority

Symptoms:
- Compile blockers existed (`undefined`, type redeclare), but repair path added new tests/files first.

Root gap:
- Repair objective does not strongly enforce "clear active blockers first" before expansion changes.

### P4. Replan Noise Propagation

Symptoms:
- Replan continued from failed context but still carried noisy action patterns (new file expansion) before stabilization.

Root gap:
- Replan continuation lacks stricter gating for failure-target-first changes.

### P5. Weak Convergence Observability

Symptoms:
- Hard to distinguish "making progress" vs "same failure replay" in run-level closure checks.

Root gap:
- Existing metrics/events are rich but no explicit convergence score/gate for repair-of-repair behavior.

## 4. Target Behavior

For failure class `shell_command_failed` + concrete compile blockers:

1. First repair action must target failure files/symbols.
2. Repair output must be valid structured decision JSON.
3. Patch must be grounded to latest target snapshot/hash.
4. Verification must rerun and either:
   - pass and continue DoD assessment, or
   - fail with new actionable blockers and bounded repair attempts.
5. Replan must not expand scope until blocker class is reduced or resolved.

## 5. Optimization Design

## 5.1 Core Runtime (Neutral)

1. Decision parse hardening:
- Add deterministic extraction boundary for structured JSON payload from stream output.
- Add one-step contract-correction retry for schema-invalid decisions using neutral schema prompt.
- Preserve failure lineage (`failureClass`, `failureSignature`, `schemaViolationPath`) for next repair input.

2. Patch grounding contract:
- `patch/apply` node must carry `targetRef` + `baseHash` + `groundingRef`.
- Dispatcher verifies live hash before apply.
- On hash mismatch, runtime emits explicit `patch_grounding_stale` failure class and forces read-refresh step before next patch generation.

3. Failure-target-first execution gate:
- For active compile/runtime blocker failures, runtime enforces first state-changing node to target declared failure targets (or declared related unblocking target).
- Expansion-only patch (new test/new file) is rejected while blocker unresolved.

4. Repair budget partition:
- Separate counters by failure class:
  - schema-invalid budget
  - patch-grounding budget
  - execution-blocker budget
- Prevent one noisy class from exhausting total repair budget.

5. Convergence duplicate detection:
- If same `failureSignature` repeats without file hash change, escalate to governance/replan earlier with structured diagnosis.

## 5.2 Software-Dev Template / Soul (Domain Layer)

1. Repair priority policy:
- Always fix active compile blockers first.
- Allow new coverage test/file only after compile passes baseline.

2. Patch strategy policy:
- Prefer minimal diff to existing failure files.
- Avoid full-file replacement unless explicitly required.

3. Verification strategy policy:
- Required sequence:
  - targeted compile/test command for blocker
  - then broader verification command
- Report failure targets in structured form for runtime reuse.

## 5.3 Memory And Context Policy

1. Persist high-value repair memory only:
- accepted blocker diagnosis
- accepted fix strategy
- verified command-result mapping

2. Exclude low-value repetitive memory:
- repeated generic "repair lesson captured" without new actionable delta.

3. In repair input, always include:
- latest failure target snapshot refs
- latest compile stderr excerpt
- last accepted patch metadata

## 6. Implementation Plan

### Phase A: Runtime Guardrails

1. Decision schema correction path.
2. Patch grounding stale detection + enforced refresh.
3. Failure-target-first gate.

### Phase B: Template Convergence Policy

1. Update software-dev Soul repair instructions.
2. Update software-dev profile repair guidance for compile-first.

### Phase C: Observability And Verification

1. Add convergence metrics:
- `repair_attempt_by_class_total`
- `repair_duplicate_failure_total`
- `patch_grounding_stale_total`
- `repair_schema_invalid_total`

2. Extend verification scripts to assert:
- no repeated schema-invalid loops beyond configured threshold
- compile blockers addressed before expansion patches
- accepted result closure reached or governance escalated with explicit diagnosis

## 7. Acceptance Criteria

1. For the same failure profile as `ws-5823c555519e601c`, repair loop should not repeat schema-invalid failure for 10 attempts.
2. `patch/apply` context mismatch should trigger refresh-and-repair path, not repeated blind retries.
3. Compile blocker runs should prioritize fixing blocker files before adding new test artifacts.
4. Run must end in one of:
- accepted closure with verification evidence, or
- explicit governance/escalation closure with non-ambiguous blocker diagnosis.

## 8. Task Tracking

| ID | Task | Layer | Priority | Status |
|---|---|---|---|---|
| RCO-01 | Decision schema correction retry | Core runtime | P0 | Completed |
| RCO-02 | Patch grounding stale detection and failure class | Core runtime | P0 | Completed |
| RCO-03 | Failure-target-first patch gate | Core runtime | P0 | Completed |
| RCO-04 | Repair budget partition by failure class | Core runtime | P1 | Completed |
| RCO-05 | Duplicate failure early escalation | Core runtime | P1 | Completed |
| RCO-06 | Compile-first repair policy update | software-dev template/Soul | P0 | Completed |
| RCO-07 | Verification sequence policy update | software-dev template/Soul | P1 | Completed |
| RCO-08 | High-value memory write filter | Core runtime | P1 | Completed |
| RCO-09 | Convergence metrics and script assertions | Core runtime + scripts | P1 | Completed |

## 8.1 Progress Log

### 2026-05-19 (Round 1)

Completed:
1. `RCO-01`:
   - Added deterministic local JSON salvage in `ParseLLMExecutionDecision`: trailing-comma repair in addition to malformed shell args repair.
   - Added unit test `TestParseLLMExecutionDecisionRepairsTrailingCommaJSON`.
2. `RCO-02`:
   - Added new runtime failure class `patch_grounding_stale`.
   - Classification now maps `patch_apply` diagnostics with `patch_basehash_mismatch` / `unified_diff_hunk_context_mismatch` / `unified_diff_hunk_removal_mismatch` to `patch_grounding_stale`.
   - Added unit test `TestClassifyRuntimeFailurePatchGroundingStale`.
3. `RCO-03`:
   - Added compile/API-blocker direct-target gate in patch relevance validation:
     compile-shape blocker repairs cannot bypass failure target scope via `relatedFailureTargets`.
   - Added unit test `TestValidateLLMExecutionOutputContractRejectsCompileBlockerBypassViaRelatedFailureTarget`.
4. `RCO-06`:
   - Updated `software-dev` template DoD policy to require fixing compile/interface blockers before expansion/test-only changes.

Verification:
1. `go test ./internal/modules/blocks/llm/biz`
2. `go test ./internal/modules/kernel/orchestrator/biz`

### 2026-05-19 (Round 2)

Completed:
1. `RCO-04`:
   - Added class-partitioned runtime repair ceilings:
     - `llm_decision_invalid`: max 4 attempts
     - `patch_apply_failed` / `patch_grounding_stale`: max 6 attempts
     - others follow policy max
   - Applied to exhaustion escalation, repair follow-up maxAttempts contract, and auto-resume checks.
2. `RCO-05`:
   - Extended duplicate-failure blocking classes to include `llm_decision_invalid` and `patch_grounding_stale`.
   - Added early escalation path for these noisy non-convergent classes before generic max-attempt exhaustion.
3. `RCO-07`:
   - Added explicit verification sequencing policy (targeted blocker verification first, then broader regression) in software-dev template DoD rules and runtime repair guidance.
4. `RCO-08`:
   - Added high-value auto-memory filter for runtime repair lessons:
     - keep only actionable/coverage-gap/critical-failure-class lessons
     - drop generic low-signal repair lesson writes.

In Progress (closed in Round 3):
1. `RCO-09`:
   - Added repair convergence telemetry fields into runtime repair follow-up input:
     - `repairAttemptByClass`
     - `repairDuplicateFailureCount`
   - Added CI verify-script schema-invalid loop threshold assertion (`assert_run_schema_invalid_threshold`).
   - Service-level exported counters are pending.

### 2026-05-19 (Round 3)

Completed:
1. `RCO-09`:
   - Added service-level convergence counters in `/metrics`:
     - `repair_attempt_by_class_total{failure_class=...}`
     - `repair_duplicate_failure_total`
     - `patch_grounding_stale_total`
     - `repair_schema_invalid_total`
   - Added runtime event-driven aggregation from Step events (`/step/dispatched`, `/step/state/changed`) without domain-specific hardcoding.
   - Added HTTP metrics test coverage for convergence counters and default zero-value export lines.

Verification:
1. `go test ./internal/modules/application/workorchd/service/server -run 'TestMetricsEndpointReportsCounters|TestMetricsEndpointReportsRepairConvergenceCounters'`

### 2026-05-19 (Round 4)

Completed:
1. Runtime duplicate-failure escalation now carries structured recovery context:
   - duplicate-failure decision tickets now include:
     - `recoveryPolicy=runtime_repair_exhausted_requires_strategy`
     - `allowedRecoveryActions=["replan"]`
   - this lets auto-governance resolve repeated `llm_decision_invalid` loops via deterministic replan path instead of parking on human-confirmation by default.
2. LLM decision JSON local repair extended:
   - added structural repair for malformed payloads where array closing `]` is missing before object close `}`.
3. Convergence metrics ingestion aligned with runtime event payload shape:

### 2026-05-19 (Round 6)

Completed:
1. Repair failure diagnostics hardening (core-neutral):
   - runtime now records structured `lastFailureDiagnostics` on failed steps, including:
     - `failureClass`, `failureSignature`, `stage`, `error`
     - optional `command`, `args`, `exitCode`, `failureTargets`
     - `stderrPreview`/`stdoutPreview`
   - runtime repair follow-up input now carries `lastFailureDiagnostics` for subsequent LLM repair decisions.

2. State-changing repair input contract tightening:
   - for runtime repair steps that require state change and include concrete failed-step lineage (`failedStepId` / `repairOfStepId`), runtime validates presence of actionable failure diagnostics:
     - `lastFailureDiagnostics`, or equivalent diagnostics from `failedStepOutput`.
   - prevents state-changing repair from running with opaque `exit status 1`-only context.

3. Retry routing improvement for LLM output-contract failures:
   - retryable LLM repair-generation failures now prefer provider-retry lane before consuming runtime-repair budget when failure is contract/output-generation type (e.g. malformed patch decision payload / redacted marker patch payload).
   - this keeps runtime-repair budget focused on actionable code-level remediation.

4. Duplicate loop breaker with evidence awareness:
   - duplicate-failure escalation now supports runtime contract-loop failures (`runtime_failed` with repeated LLM graph-contract signature).
   - if repeated signature occurs without newly completed action evidence, runtime escalates earlier instead of continuing same repair loop.
   - if new completed action evidence exists, runtime allows continued repair.

Verification:
1. `go test ./internal/modules/kernel/orchestrator/biz`
   - metrics watcher now reads repair fields from event `input.summary` / `output.summary` as well as top-level maps.
   - deduplicates attempt counting by `stepId` to avoid double increments on dispatched/failed dual events.
4. Repair loop observability normalized:
   - `lastAttempt` projection is floored by `attemptCount` to prevent regressive displays such as `attemptCount=4` with `lastAttempt=1`.

Verification:
1. `go test ./internal/modules/blocks/llm/biz -run 'TestParseLLMExecutionDecisionRepairsTrailingCommaJSON|TestParseLLMExecutionDecisionRepairsMissingArrayCloserJSON'`
2. `go test ./internal/modules/application/workorchd/service/server -run 'TestMetricsEndpointReportsCounters|TestMetricsEndpointReportsRepairConvergenceCounters|TestMetricsEndpointRepairCountersReadSummaryPayload'`
3. `go test ./internal/modules/kernel/orchestrator/biz -run 'TestShouldBlockOnDuplicateRuntimeFailureEarlyForSchemaInvalid|TestClassifyRuntimeFailurePatchGroundingStale|TestRuntimeRepairFollowUpStepInputIncludesRepairClassAndDuplicateCounts'`
4. `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestMapTaskProgressProjectsRepairLoopState|TestMapTaskProgressRepairLoopSignalsPreferCurrentFailureStep|TestMapTaskProgressProjectsRepairLoopEscalation'`
5. `go test ./internal/modules/kernel/governance/biz -run 'TestServiceAutoReplansRuntimeRepairExhaustionTicket|TestServiceAutoResolvesReplanDecisionTicketWithoutMissingFields'`

Verification:
1. `go test ./internal/modules/kernel/orchestrator/biz -run 'TestMaxRuntimeRepairAttemptsForFailureClassUsesPartition|TestShouldBlockOnDuplicateRuntimeFailureEarlyForSchemaInvalid|TestRuntimeRepairFollowUpStepInputIncludesRepairClassAndDuplicateCounts|TestClassifyRuntimeFailurePatchGroundingStale|TestValidateLLMExecutionOutputContractRejectsCompileBlockerBypassViaRelatedFailureTarget'`
2. `go test ./internal/modules/blocks/llm/biz ./internal/modules/kernel/orchestrator/biz`

### 2026-05-19 (Round 5)

Completed:
1. Execution-quality gate: strategy reassessment contract enforcement in strict repair loops:
   - Added runtime gate in `validateLLMExecutionOutputContract`:
     - when `runtimeRepairFollowUp=true`, `stateChangingOutputRequired=true`, and `strategyReassessment.required=true`,
     - if output contains state-changing `patch/apply`, it must carry the required structured decision fields:
       - `failedSurface`
       - `rootCauseTarget`
       - `repairTargetDecision`
       - `forbiddenRepair`
       - `verificationPlan`
   - Gate accepts these fields from:
     - `patches[].*`
     - `executionGraph.nodes[].input` for `patch/apply`
     - `nextInput` when `nextKind=patch/apply`
   - `failedSurface` must align with the current reassessment failed surface.
2. Early circuit-breaker for repeated malformed LLM decision loops:
   - `llm_decision_invalid` is now treated as duplicate-failure blocker class.
   - Duplicate-block pre-budget bypass now includes `llm_decision_invalid`, so repeated schema-invalid loops can escalate earlier instead of consuming full repair budget.
3. Regression tests added:
   - `TestValidateLLMExecutionOutputContractRejectsMissingStrategyReassessmentDecisionFields`
   - `TestValidateLLMExecutionOutputContractAllowsStrategyReassessmentDecisionFieldsInPatchInput`
   - `TestShouldBlockOnDuplicateRuntimeFailureForRepeatedLLMDecisionInvalid`

Verification:
1. `go test ./internal/modules/kernel/orchestrator/biz -run 'TestShouldBlockOnDuplicateRuntimeFailureForRepeatedLLMDecisionInvalid|TestShouldNotBlockOnDuplicateRuntimeFailureBeforeScopedBudget|TestValidateLLMExecutionOutputContractRejectsMissingStrategyReassessmentDecisionFields|TestValidateLLMExecutionOutputContractAllowsStrategyReassessmentDecisionFieldsInPatchInput|TestRuntimeOrchestrator_DuplicateRuntimeFailureUsesDialogLanguage|TestRuntimeRepairFollowUpStepInputIncludesFailureContext'`

## 9. Non-Goals

1. No business-domain hardcoding in reusable runtime core.
2. No manual intervention on workspace output artifacts as primary fix.
3. No compatibility fallback for legacy ambiguous repair behavior in this phase.
