# Runtime Contract Retry Batch 4 Retry Correction Audit

## Purpose

Batch 4 audits how WorkOrch turns a rejected runtime contract output into the next retry, and whether repeated violations receive a narrower correction contract before the run is escalated to decision.

This batch consumes the Contract, Retry, and Decision dimensions from `runtime-contract-retry-full-chain-audit-plan.md`.

The audit is business-neutral. Live workspace examples are evidence samples only. Core retry behavior must be driven by structured violation type, rule, target scope, contract metadata, and bounded evidence refs, not by workspace title, language, file names, or provider details.

## Scope

In scope:

- Contract violation taxonomy.
- Recoverability classification.
- Retry step creation.
- Retry input construction.
- Per-step, per-run, and repeated-signature retry budget.
- Escalation to decision after targeted retry is exhausted.
- Rejected diagnostic evidence and memory boundaries.
- Projection/event payload surface needed by Web/TUI ViewModel.

Out of scope:

- Implementing scratch directory enforcement from Batch 3.
- Rewriting LLM context payload shape from Batch 5.
- Changing template/Soul-specific delivery strategy.
- Accepting invalid LLM output as successful progress.

## Evidence Used

| Evidence | Meaning |
| --- | --- |
| `ev:ws4e/events/all-runs/001` | Live `ws4e` latest run had repeated contract violations and retry requests before decision. |
| `ev:ws7c/events/all-runs/001` | Live `ws7c` latest run had repeated state-changing contract violations and retry requests before decision. |
| `ev:code/retry/violation-taxonomy/001` | Current mapping from violation type to violated rule, recoverability, retryHint, and forbiddenRepeat. |
| `ev:code/retry/recovery-routing/001` | Failed step recovery routes contract violations to retry, dispatch, or exhaustion. |
| `ev:code/retry/recovery-projection/001` | Retry input receives `contractViolationRecovery` and generic repair guidance. |
| `ev:code/retry/retry-budget/001` | Retry budget uses per-step, per-run, and repeated-signature counters. |
| `ev:code/retry/escalation-decision/001` | Exhausted contract violation retry creates a decision ticket and publishes escalated event. |
| `ev:code/retry/events/001` | Contract violation recorded/retry/escalated events are published with workspace/run/step scope. |
| `ev:code/retry/memory-boundary/001` | Contract violation diagnostics and retry details are blocked from long-term memory writes. |
| `ev:code/retry/projection-payload/001` | Projection payload currently allows only a compact subset of contract violation fields. |

## Retry Correction Findings

### B4-RC-001 Violation Taxonomy Exists But Is Too Coarse For Correction

- Claim: Runtime records structured contract violation metadata, but several distinct retry needs collapse into broad rules.
- Expected behavior: Each recurring contract violation should map to a correction profile that preserves the original contract and adds precise, rule-specific requirements.
- Actual behavior: `state_changing_required_missing`, `concrete_validation_contract_missing`, coverage/evidence assessment errors, and unparseable output have basic rule-specific hints. Patch/apply failures such as missing target, missing content, graph depends on LLM before patch, and file edit conflict are grouped as `runtime_contract_violation` for retry hint purposes.
- Evidence: `ev:code/retry/violation-taxonomy/001`, `ev:ws7c/steps/summary/001`.
- Counter-evidence: The taxonomy is sufficient for basic retry budgeting and event projection; it is not sufficient to force a targeted correction shape.
- Deviation type: `D-RETRY-CORRECTION-TOO-BROAD`, `D-PATCH-CONTRACT-RECOVERY-GAP`.
- Owner: core retry correction.
- Closure batch: 4.
- Required code/doc/test action: Introduce violation correction profiles keyed by violated rule and normalized violation type. Patch/apply contract failures need their own correction profile instead of the generic runtime-contract hint.

### B4-RC-002 Retry Routing Is Event-Driven And Budgeted

- Claim: Contract violation retry is on the main runtime recovery path and is not a lost side channel.
- Expected behavior: A recoverable contract violation should create a bounded retry step, publish retry-requested event, and then dispatch through the normal orchestrator path.
- Actual behavior: `runtimeRecoveryActionForFailedStep` selects `contract_violation_retry` while within budget. `dispatchRuntimeRepairFollowUp` builds a `contract-retry-*` step, publishes `/orchestrator/contract_violation/retry_requested`, and calls `requestDispatchStep`.
- Evidence: `ev:code/retry/recovery-routing/001`, `ev:code/retry/events/001`.
- Counter-evidence: Live workspaces still reached decision, which indicates routing worked but correction content did not become more effective.
- Deviation type: `D-RETRY-CORRECTION-TOO-BROAD`.
- Owner: core orchestrator / EventHub runtime.
- Closure batch: 4.
- Required code/doc/test action: Keep this event-driven route. Add tests that retry events include correction profile id / correction mode after implementation.

### B4-RC-003 Retry Input Preserves Contract But Uses Generic Guidance

- Claim: Retry input carries the rejected contract and expected contract, but the human/model-facing guidance is generic.
- Expected behavior: Retry input should preserve the original `runtimeStage`, `repairObjective`, `stateChangingOutputRequired`, `requiredOutput`, source refs, and actual rejected shape, then add a narrow correction contract for the violated rule.
- Actual behavior: `applyContractViolationRecoveryProjection` copies `contractViolationRecovery`, marks `contractViolationRetry=true`, sets lineage, and appends generic guidance: use expected contract, do not repeat actual output summary, return one valid JSON decision.
- Evidence: `ev:code/retry/recovery-projection/001`, `ev:code/contract/violation-feedback/001`.
- Counter-evidence: Existing tests prove some important fields are preserved, including state-changing expected contract and gap baseline. The missing piece is not preservation; it is narrow correction.
- Deviation type: `D-RETRY-CORRECTION-TOO-BROAD`.
- Owner: core retry correction / LLM current-action.
- Closure batch: 4.
- Required code/doc/test action: Add `contractViolationRecovery.correctionContract` with rule-specific required output, forbidden shapes, target scope, allowed patch modes, and escalation instructions.

### B4-RC-004 Same-Type Repetition Is Budgeted, Not Narrowed

- Claim: Repeated contract violations are counted and eventually escalated, but repetition does not change the retry contract before exhaustion.
- Expected behavior: After the first same-rule or same-target retry fails, the next retry should switch to a forced correction profile rather than restating the same broad contract.
- Actual behavior: Retry budget counts per failed source step, per run, and repeated signature/rule. Default policy is 2 retries per step, 8 per run, and 1 repeated signature. The second retry still receives the same generic recovery projection unless other repair input logic changes it.
- Evidence: `ev:code/retry/retry-budget/001`, `ev:code/retry/recovery-projection/001`, `ev:ws4e/events/all-runs/001`, `ev:ws7c/events/all-runs/001`.
- Counter-evidence: Budget exhaustion is useful and prevents infinite retry. It does not by itself improve output quality.
- Deviation type: `D-RETRY-CORRECTION-TOO-BROAD`.
- Owner: core retry correction.
- Closure batch: 4.
- Required code/doc/test action: Add `correctionMode=forced_narrow` or equivalent when same-rule/same-target repeats. Tests must prove attempt 2 has stricter correction content than attempt 1.

### B4-RC-005 Escalation To Decision Exists And Is Properly Scoped

- Claim: When retry budget is exhausted, runtime opens a decision ticket instead of silently spinning.
- Expected behavior: Escalation should include structured context, rejected result policy, recovery actions, failed step, violation, attempts, and budget limits.
- Actual behavior: `blockOnContractViolationRecoveryExhaustion` creates a decision ticket with `runtimeRepairLoopState=contract_violation_recovery_exhausted`, `allowedRecoveryActions=["replan","terminate"]`, violation context, attempts, runAttempts, signatureRepeatCount, correction profile context, and budget settings; it blocks the run and publishes escalated event.
- Evidence: `ev:code/retry/escalation-decision/001`, `ev:code/retry/events/001`.
- Counter-evidence: Live workspaces escalated while still lacking effective progress, so escalation is necessary but should happen after targeted correction has genuinely been attempted.
- Deviation type: `D-RETRY-CORRECTION-TOO-BROAD`.
- Owner: core governance / retry correction.
- Closure batch: 4.
- Required code/doc/test action: Keep escalation path. Add evidence that forced correction was attempted before escalation for retryable rules.

### B4-RC-006 Rejected Contract Violation Output Is Kept Out Of Accepted Memory

- Claim: Contract violation retry details are treated as diagnostic evidence, not durable reusable memory.
- Expected behavior: Rejected output and retry diagnostics should not become accepted result, artifact, or high-value memory.
- Actual behavior: Runtime memory code skips repair lesson memory for contract violation retries and tests reject explicit diagnostic-only contract violation memory writes.
- Evidence: `ev:code/retry/memory-boundary/001`.
- Counter-evidence before closure: The retry correction needed to use rejected output as current diagnostic context in C4/current-action, but not as long-term memory.
- Deviation type: none.
- Owner: core memory boundary.
- Closure batch: 4.
- Required code/doc/test action: Preserve this behavior while adding correction profiles.
- Closure status: code-closed by the 2026-06-21 Batch 4/5 records. `contractViolationRecovery` is rendered in C4 current-action only, correction profiles include compact rejected-output diagnostics, and memory tests preserve the diagnostic-only boundary.

### B4-RC-007 Projection Payload Is Too Small For Retry Debuggability

- Claim: Projection currently allows a compact set of contract violation event fields, but not enough to show why retry did or did not narrow.
- Expected behavior: ViewModel should expose business-neutral retry metadata such as correction profile id, correction mode, retry attempt, target scope, and escalation reason without exposing raw rejected payload.
- Actual behavior: Allowed projection keys include attempt, maxAttempts, recoverability, rejectedOutputRef, retryHint, retryStepId, signatureRepeatCount, sourceStepId, ticketId, and violatedRule.
- Evidence: `ev:code/retry/projection-payload/001`.
- Counter-evidence: The current surface is enough to show that retry happened; it is not enough to show whether the retry became targeted.
- Deviation type: `D-PLANNED-TASK-PROJECTION-GAP`.
- Owner: projection / ViewModel.
- Closure batch: 4 for event metadata requirement, Batch 6 for full projection model.
- Required code/doc/test action: Add bounded correction metadata to event payload and projection allowlist after correction profiles exist.

## Retry Correction Matrix

| Violated Rule / Type | Current Retry Behavior | Required Targeted Correction |
| --- | --- | --- |
| `state_changing_required_missing` | Retry same expected contract with generic "do not repeat read-only graph / DoD assessment" guidance. | Force one of: valid state-changing `patch/apply`, accepted state-changing tool/shell contract, incremental artifact state-changing output, or real governance blocker. Include available target snapshot summary when present. |
| `concrete_validation_contract_missing` | Retry same expected contract with generic validation evidence hint. | Require bounded validation graph, validationEvidence fields, allowed node kinds, scratch boundary from Batch 3, and explicit forbidden detached/background/workspace mutation shapes. |
| `coverage_map_incomplete` / `dod_assessment_missing_evidence_refs` | Retry assessment with evidence refs and coverage. | Require exact missing coverage refs, accepted/rejected evidence refs, and prohibit satisfied items without traceable evidence. |
| `patch_dispatch_invalid` / missing target/content / file edit conflict | Often maps to `runtime_contract_violation` and generic valid-output hint. | Require patch-specific profile with targetRef, patchType, baseHash/snapshot rules, allowed patch modes, and original invalid patch reason. |
| `unparseable_output` / wrong output shape | Retry one valid JSON decision. | Keep strict JSON-only retry; include schema/output-shape delta and no side effects. |
| `decision_blocked_when_actionable` | Rule exists but no observed live evidence in this batch. | Require executable continuation or real blocker with missing external input. |

## Closure Gates

### B4-G1 Violation Correction Profile Matrix

- Chain node: Contract violation classification and retry input construction.
- Deviation type: `D-RETRY-CORRECTION-TOO-BROAD`.
- Evidence refs: `ev:code/retry/violation-taxonomy/001`, `ev:code/retry/recovery-projection/001`, `ev:code/contract/violation-feedback/001`.
- Owner: core retry correction.
- Expected behavior: Every retryable violated rule maps to a correction profile with required output shape, forbidden repeats, target scope, and recovery mode.
- Actual behavior: Retry hint and repair guidance are broad strings; patch/apply failures collapse to generic runtime-contract feedback.
- Proposed change: Add structured `correctionContract` under `contractViolationRecovery`, generated from rule/type/scope.
- Why this is not a business-specific core rule: The profiles are based on neutral runtime contracts and output schemas.
- Required tests: One test per primary profile: state-changing, concrete validation, patch/apply, coverage/evidence, unparseable output.

### B4-G2 Forced Narrow Retry After Repetition

- Chain node: Contract violation retry budget and retry input generation.
- Deviation type: `D-RETRY-CORRECTION-TOO-BROAD`.
- Evidence refs: `ev:code/retry/retry-budget/001`, `ev:ws4e/events/all-runs/001`, `ev:ws7c/events/all-runs/001`.
- Owner: core retry correction.
- Expected behavior: Same-rule or same-target repetition changes retry mode from broad correction to forced narrow correction before escalation.
- Actual behavior: Repetition is counted for budget; retry content does not explicitly narrow based on repeat count.
- Proposed change: Add repeat-aware correction mode using attempt/signatureRepeatCount/target scope.
- Why this is not a business-specific core rule: Repetition is measured through structured runtime metadata, not content-specific strings.
- Required tests: Attempt 1 has normal correction; attempt 2 with same rule/scope has forced-narrow correction; exhausted budget opens decision.

### B4-G3 Escalation Requires Targeted Retry Evidence

- Chain node: Contract violation exhaustion decision.
- Deviation type: `D-RETRY-CORRECTION-TOO-BROAD`.
- Evidence refs: `ev:code/retry/escalation-decision/001`, `ev:code/retry/events/001`.
- Owner: core governance / retry correction.
- Expected behavior: Decision escalation should show which targeted correction profile was attempted and why automation is exhausted.
- Actual behavior: Escalation includes violation and budget context but not targeted correction profile evidence.
- Proposed change: Include last correction profile/mode in escalation context and event payload.
- Why this is not a business-specific core rule: It records runtime recovery provenance.
- Required tests: Escalated decision context includes violatedRule, correctionProfileId, correctionMode, attempts, and rejectedOutputRef.

### B4-G4 Rejected Diagnostic Boundary

- Chain node: Contract violation retry memory/result/evidence boundary.
- Deviation type: none; guardrail gate.
- Evidence refs: `ev:code/retry/memory-boundary/001`.
- Owner: core memory/evidence boundary.
- Expected behavior: Rejected contract violation output can inform the immediate retry but must not be accepted as durable memory/result/artifact.
- Actual behavior: Memory write path already rejects diagnostic-only contract violation content and skips repair lesson memory for contract violation retry.
- Proposed change: Preserve this behavior while adding correction profiles.
- Why this is not a business-specific core rule: It is a generic evidence quality and memory provenance rule.
- Required tests: Existing memory tests remain green; new correction profile fields do not become memory writes.

## Closure Records

### 2026-06-21 B4-G1 / B4-G2 / B4-G3 Retry Correction Profiles

- Closed gates: `B4-G1`, `B4-G2`, `B4-G3`.
- Guardrail verified: `B4-G4`.
- Closed deviations: `D-RETRY-CORRECTION-TOO-BROAD`, `D-PATCH-CONTRACT-RECOVERY-GAP`.
- Implementation refs:
  - `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
  - `internal/pkg/workorchruntimecontract/contract_violation.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/event_payload_contract_violation_allowed_keys.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/event_payload_contract_violation_test.go`
- Actual change:
  - Added structured `contractViolationRecovery.correctionContract`, `correctionProfileId`, `correctionMode`, and `targetScope`.
  - Added neutral correction profiles for state-changing output, concrete validation evidence, patch/apply contract failures, coverage/evidence assessment, and JSON/output-shape failures.
  - Split patch/apply contract failures into `patch_apply_contract_failed` so missing target/content, read-before-patch, dispatch invalid, unresolved template, and file edit conflict no longer collapse into a generic retry profile.
  - Added repeat-aware `forced_narrow` correction mode when retry attempt or same-signature/same-rule repeat indicates that the broad correction already failed.
  - Escalation context now carries the last attempted correction profile/mode/contract so decision tickets and escalated events can show which automated correction was exhausted.
  - Exhaustion decisions now allow both `replan` and `terminate`; appservice decision normalization preserves an explicit `terminate` as `reject_current_path` instead of downgrading it to the first allowed action.
  - Event projection now preserves bounded correction metadata (`correctionProfileId`, `correctionMode`, `targetScope`) while still omitting heavy raw `contractViolation` and `context` payloads.
- Boundary result:
  - Profiles are generated from structured runtime rule/type/scope, attempt, repeat count, requiredOutput, and snapshot metadata.
  - No profile branches on workspace template ID, Soul ID, live workspace ID, programming language, provider, LSP, concrete filename semantics, or natural-language business text.
  - `recoverability` remains the existing retry classification; narrowing is expressed by `correctionMode` and `correctionContract` to avoid changing budget semantics.
- Boundary searches run:
  - `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg`
  - `rg -n "correctionProfileId|correctionMode|correctionContract|patch_apply_contract_failed|forced_narrow|validationScratchDir|workorch-validation-test" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg internal/modules/blocks/llm -g '*.go'`
  - Business/template hits remained in defaults/Soul data and existing tests; new production fields are runtime-contract metadata only.
- Tests run:
  - `gofmt -w internal/pkg/workorchruntimecontract/contract_violation.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go internal/modules/kernel/orchestrator/biz/runtime_repair.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/event_payload_contract_violation_allowed_keys.go internal/modules/application/workorchd/biz/appservice/readmodelprojection/event_payload_contract_violation_test.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestContractViolation(Recovery|Retry|Correction|Events|Escalated|Escalation)|TestRuntimeRepairInputIncludesContractViolationRecovery' -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'TestProjectEventPayloadKeepsContractViolationSummaryFields|TestRunSnapshotFromModelsProjectsContractViolationWithoutRuntimeProgress|TestWorkspaceSnapshot' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1`
  - `go test ./internal/modules/kernel/orchestrator/... -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1`
  - `go test ./internal/modules/blocks/llm/biz -count=1`
  - `go test ./internal/modules/blocks/llm/biz -run 'TestContractViolationRecoveryRendersInCurrentActionOnly|TestPrompt|TestRuntimeAdapter' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_RuntimeMemoryWriteRejectsContractViolationDiagnostic|TestRuntimeOrchestrator_ContractViolationRetryDoesNotCreateRepairLessonMemory|TestContractViolationRecovery' -count=1`
  - `go test ./internal/modules/blocks/memory/... -count=1`
  - `go test ./... -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestResolveDecisionWithSession(InfersRepairExhaustionReplanFromContinueUnderPolicy|KeepsExplicitRepairExhaustionTerminateUnderReplanPolicy|MapsRepairExhaustionReplanAndTerminate)' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_.*Governance|TestRuntimeOrchestrator_.*Localization|Test.*Repair.*Exhaust|Test.*ContractViolation' -count=1`
  - `WORKORCH_MANAGED_VERIFY_TARGET=verify-llm-file WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_VERIFY_KEEP_TMP=1 WORKORCH_MANAGED_GO_TEST_PACKAGES="./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/orchestrator/biz" ./scripts/verify-workorch-managed-full.sh`
  - Evidence reference integrity check over `docs/70-roadmap/active/runtime-contract-retry-*.md` produced no missing definitions.
- Remaining risk:
  - Batch 4 does not rewrite LLM request partitioning or current-action snapshot placement; Batch 5 must verify that correction profiles are rendered near the current action without changing C0 stable prompt.
  - Live workspaces that already escalated may need a fresh replan/resume to receive the new correction profile fields.
- Next gate:
  - Continue with Batch 5 `B5-G1` / `B5-G2`: surface available target snapshot summaries and classify ignored snapshots in current-action/request assembly.

## Batch 4 Decision

Batch 4 is code-closed for structured retry correction.

- Structured `contractViolationRecovery` now carries correction profile, correction mode, correction contract, target scope, bounded rejected-output summary, and escalation provenance.
- Repeat-aware `forced_narrow` retry is covered before governance escalation.
- Rejected diagnostic output remains immediate retry context only; it is not accepted as result/artifact and does not become long-lived memory.
- Batch 5 has subsequently closed the current-action rendering gap for these correction fields and available target snapshots.

Future work should reopen Batch 4 only if a focused regression proves correction profile metadata is missing, repeat-aware narrowing does not trigger, or rejected diagnostic data crosses the memory/result/artifact boundary.
