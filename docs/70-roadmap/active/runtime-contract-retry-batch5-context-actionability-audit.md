# Runtime Contract Retry Batch 5 Context And Actionability Audit

## Purpose

Batch 5 audits whether the LLM request is complete, ordered, actionable, cache-aware, and bounded enough to let the model continue from the current Goal/DoD gap without inventing missing context.

This batch consumes the Gap/Todo, Contract, Retry, Context/Actionability, and Resource dimensions from `runtime-contract-retry-full-chain-audit-plan.md`.

The audit remains business-neutral. It may cite live workspace files as evidence samples, but core behavior must use structured fields such as target snapshots, content hashes, current-action, coverage targets, rejected evidence refs, and contract violation metadata.

## Scope

In scope:

- Prompt section ordering and C0/C1/C2/C3/C4/C5 partition.
- Current-action payload.
- Repair action package and target snapshot surfacing.
- Snapshot compaction and prompt safety flags.
- Context budget and cache-plan metadata.
- Actionability gap when complete target snapshots are present but LLM output claims they are missing.

Out of scope:

- Provider-specific cache private fields.
- Template-specific development instructions.
- Changing retry correction profiles from Batch 4.
- Web/TUI projection display.

## Evidence Used

| Evidence | Meaning |
| --- | --- |
| `ev:ws7c/run4/ctr-statechange-005/request/001` | Complete current target snapshots existed in the request. |
| `ev:ws7c/run4/ctr-statechange-005/output/001` | LLM still returned failed assessment claiming missing snapshots. |
| `ev:ws7c/run4/ctr-statechange-005/request-prompt/001` | Prompt sections included C3/C4 contracts, target snapshots, and cache plan. |
| `ev:code/context/prompt-partition/001` | Prompt is built as ordered C0/C1/C2/C3/C4/C5 sections. |
| `ev:code/context/current-action/001` | Current-action section states state-changing requirements and snapshot usage rules. |
| `ev:code/context/repair-snapshots/001` | Repair input projects patch/failure target snapshots and repairActionPackage target snapshots. |
| `ev:code/context/snapshot-compaction/001` | Prompt snapshot compaction marks truncated content unsafe for complete file_write. |
| `ev:code/context/cache-plan/001` | Cache plan keeps provider-neutral stable prefix and volatile tail separation. |

## Context Findings

### B5-CTX-001 Prompt Partitioning Is Directionally Correct

- Claim: The LLM request already has a cache-aware ordered section model.
- Expected behavior: Stable runtime/schema language belongs in C0, Goal/DoD in C1, capabilities in C2, stage contract in C3, current action/evidence in C4, and large trace refs/budget in C5.
- Actual behavior: `buildProviderPromptMessages` appends C0, C1, C2, C3, C4, and C5 sections in deterministic order. Prompt metadata assigns cacheability by section group and marks raw Goal/DoD sections protected.
- Evidence: `ev:code/context/prompt-partition/001`, `ev:ws7c/run4/ctr-statechange-005/request-prompt/001`.
- Counter-evidence: None for section ownership. Later findings show content/actionability gaps inside C4/current-action.
- Deviation type: none.
- Owner: core LLM prompt rendering.
- Closure batch: 5.
- Required code/doc/test action: Preserve section partitioning while adding missing current-action fields.

### B5-CTX-002 Complete Target Snapshots Exist But Are Not Elevated Enough

- Claim: Complete target snapshots are present, but the prompt does not provide a concise top-level available snapshot summary that the retry classifier can enforce.
- Expected behavior: When complete snapshots exist, current-action should expose a short `availableTargetSnapshots` summary with targetRef, contentComplete, safeForFileWrite, allowed patch modes, contentHash, source path, and prompt safety. It should explicitly say the model must not claim those snapshots are missing.
- Actual behavior: Target snapshots exist in `failureTargetRepairContext`, `patchRepairContext`, and `repairActionPackage.targetSnapshots`. Current-action includes general requirements such as "when snapshots exist, do not return file/read"; however, the available snapshot set is nested in large C3/C4 structures and not exposed as a compact authoritative list.
- Evidence: `ev:ws7c/run4/ctr-statechange-005/request/001`, `ev:ws7c/run4/ctr-statechange-005/request-prompt/001`, `ev:code/context/current-action/001`, `ev:code/context/repair-snapshots/001`.
- Counter-evidence: The LLM output claimed missing snapshots despite the request containing complete `main.go` and `main_test.go` snapshots.
- Deviation type: `D-AVAILABLE-SNAPSHOT-IGNORED`.
- Owner: core LLM current-action / retry classifier.
- Closure batch: 5.
- Required code/doc/test action: Add `availableTargetSnapshots` to C4.current-action and classify missing-snapshot claims as `available_snapshot_ignored` when complete snapshots are present.

### B5-CTX-003 Snapshot Prompt Safety Is Correct But Needs Actionability Binding

- Claim: Prompt compaction correctly downgrades truncated snapshots so they cannot be used as complete file_write sources.
- Expected behavior: Complete snapshots should remain complete and safe; truncated snapshots should set prompt-specific incomplete/unsafe flags and force exact read or targeted extraction before full file_write.
- Actual behavior: `compactRepairSnapshotsForPrompt` sets `contentComplete=false`, `safeForFileWrite=false`, `promptContentComplete=false`, `promptSafeForFileWrite=false`, and `promptExtractionRequired=true` when content is truncated. `promptRequiredOutputForRepairPackage` then disables direct complete file_write from incomplete prompt snapshots.
- Evidence: `ev:code/context/snapshot-compaction/001`.
- Counter-evidence: `ws7c` request had complete small snapshots, so the live issue is not unsafe truncation. It is that complete snapshots were not made actionable enough.
- Deviation type: `D-AVAILABLE-SNAPSHOT-IGNORED`.
- Owner: core prompt rendering / context budget.
- Closure batch: 5.
- Required code/doc/test action: Preserve truncation safety while adding complete-snapshot actionability summary.

### B5-CTX-004 Current-Action Has Rules But No Enforceable Missing-Snapshot Guard

- Claim: Current-action already says state-changing output is required and forbids read-only graphs, but it does not produce an enforceable guard against false "snapshot missing" assessments.
- Expected behavior: If current-action supplies complete snapshots, output that says snapshots are missing should be rejected with a specific violation and retried with the exact available target summary.
- Actual behavior: Current-action includes requirements to use snapshots and not return file/read for those targets. Runtime enforcement rejects missing state-changing output, but does not separately classify "available snapshot ignored" from generic state-changing-missing output.
- Evidence: `ev:code/context/current-action/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`.
- Counter-evidence: Batch 3 and Batch 4 show the generic contract rejection worked, but did not improve correction quality.
- Deviation type: `D-AVAILABLE-SNAPSHOT-IGNORED`, `D-RETRY-CORRECTION-TOO-BROAD`.
- Owner: core LLM request assembly / contract violation classifier.
- Closure batch: 5.
- Required code/doc/test action: Add available-snapshot-aware classifier and retry feedback; integrate with Batch 4 correction profile.

### B5-CTX-005 Cache Plan Is Correctly Provider-Neutral

- Claim: Cache plan is provider-neutral and does not place dynamic repair fields in the stable prefix.
- Expected behavior: C0/C1/C2 can be cacheable; C3/C4/C5 should stay volatile or large-ref sections. Provider-private cache fields should be applied by adapters from a neutral plan.
- Actual behavior: Cache plan hashes C0, C1, C2, C3, C4, C5 separately, sets stable cache key from C0/C1/C2, marks C4 as volatile breakers, and exposes adapter hints without core provider-private fields.
- Evidence: `ev:code/context/cache-plan/001`, `ev:ws7c/run4/ctr-statechange-005/request-prompt/001`.
- Counter-evidence: Low cache hit rates may still occur due to large/variable C1/C2 or provider behavior, but this audit did not find a C0 contamination issue.
- Deviation type: none.
- Owner: core LLM prompt/cache planning.
- Closure batch: 5.
- Required code/doc/test action: Preserve stable-prefix boundary when adding availableTargetSnapshots; it belongs in C4/current-action, not C0/C1/C2.

### B5-CTX-006 Prompt Volume Is High But Structured

- Claim: The request has many sections and large C4 current-action/current-progress payloads, which can hide the immediate actionable target even though the context is structurally complete.
- Expected behavior: The most immediate action should be short and front-loaded in C4.current-action, with detailed coverage/history available as supporting evidence.
- Actual behavior: The live request C4.current-action is large, includes nested contractViolationRecovery and repairActionPackage content, and current-progress/coverage sections are also large. The prompt is ordered, but the actionable snapshot summary is not concise.
- Evidence: `ev:ws7c/run4/ctr-statechange-005/request-prompt/001`.
- Counter-evidence: Context budget includes section hashes and typed event tape, so the issue is not unstructured raw dumping.
- Deviation type: `D-AVAILABLE-SNAPSHOT-IGNORED`.
- Owner: core LLM current-action / context budget.
- Closure batch: 5.
- Required code/doc/test action: Add a short current-action `primaryNextAction` and `availableTargetSnapshots` before nested diagnostics.

## Actionability Matrix

| Area | Current State | Required Closure |
| --- | --- | --- |
| Prompt partition | C0/C1/C2/C3/C4/C5 exists and is ordered. | Preserve. |
| Goal/DoD protection | C1 raw Goal/DoD sections are protected and definition hash exists. | Preserve. |
| Target snapshots | Present in multiple nested fields. | Add compact C4 `availableTargetSnapshots` summary. |
| Snapshot safety | Truncated prompt snapshots are marked unsafe. | Preserve and include prompt safety in summary. |
| Missing snapshot claims | Rejected only as generic state-changing output failure. | Add `available_snapshot_ignored` classifier and retry feedback. |
| Cache boundary | Stable prefix excludes volatile C4. | Add new fields only to C4/current-action. |
| Prompt noise | Large but structured current-action and coverage sections. | Front-load primary next action and concise target summary. |

## Closure Gates

### B5-G1 Available Target Snapshot Summary

- Chain node: LLM request current-action.
- Deviation type: `D-AVAILABLE-SNAPSHOT-IGNORED`.
- Evidence refs: `ev:ws7c/run4/ctr-statechange-005/request/001`, `ev:ws7c/run4/ctr-statechange-005/request-prompt/001`, `ev:code/context/current-action/001`, `ev:code/context/repair-snapshots/001`.
- Owner: core LLM current-action.
- Expected behavior: Complete target snapshots are listed in a short authoritative C4 field close to the requested action.
- Actual behavior: Complete snapshots are present but nested in large repair/context structures.
- Proposed change: Add `availableTargetSnapshots` and `primaryNextAction` to C4.current-action.
- Why this is not a business-specific core rule: It summarizes generic snapshot metadata and allowed output forms.
- Required tests: Prompt context for state-changing repair with complete snapshots includes summary with targetRef, contentHash, contentComplete, safeForFileWrite, allowedPatchModes, and source path.

### B5-G2 Available Snapshot Ignored Classifier

- Chain node: Contract violation classifier and retry correction.
- Deviation type: `D-AVAILABLE-SNAPSHOT-IGNORED`.
- Evidence refs: `ev:ws7c/run4/ctr-statechange-005/request/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`.
- Owner: core contract violation classifier / retry correction.
- Expected behavior: If output claims target snapshot is missing while request contains complete safe snapshot, classify as `available_snapshot_ignored`.
- Actual behavior: The failure becomes generic `state_changing_required_missing`.
- Proposed change: Add classifier that compares rejected output language/diagnostic fields to structured available snapshot summary.
- Why this is not a business-specific core rule: It compares structured availability against generic missing-data claims.
- Required tests: Complete snapshot + missing snapshot output yields `available_snapshot_ignored` and retry feedback cites available targets.

### B5-G3 Snapshot Compaction Safety Guard

- Chain node: Prompt snapshot compaction and requiredOutput mutation.
- Deviation type: none; guardrail gate.
- Evidence refs: `ev:code/context/snapshot-compaction/001`.
- Owner: core prompt/context budget.
- Expected behavior: Truncated/redacted/binary snapshots cannot be used as complete file_write sources.
- Actual behavior: Current code downgrades prompt flags and requiredOutput when snapshots are incomplete for prompt.
- Proposed change: Preserve this behavior while adding available target summary.
- Why this is not a business-specific core rule: It protects generic artifact mutation safety.
- Required tests: Large truncated snapshot sets prompt extraction required and does not appear as safe complete file_write target in availableTargetSnapshots.

### B5-G4 Cache Boundary Preservation

- Chain node: Prompt context and provider cache plan.
- Deviation type: none; guardrail gate.
- Evidence refs: `ev:code/context/prompt-partition/001`, `ev:code/context/cache-plan/001`.
- Owner: core LLM prompt/cache planning.
- Expected behavior: Available snapshot and retry fields remain volatile and do not change C0 stable prefix.
- Actual behavior: Current C0/C1/C2/C4 split supports this.
- Proposed change: Add fields only to C4.current-action and update cache tests if needed.
- Why this is not a business-specific core rule: It preserves provider-neutral request partitioning.
- Required tests: Adding/changing availableTargetSnapshots changes volatileTailHash but not stablePrefixHash.

## Closure Records

### 2026-06-21 B5-G1 / B5-G2 / B5-G3 / B5-G4 Context Actionability

- Closed gates: `B5-G1`, `B5-G2`.
- Guardrails verified: `B5-G3`, `B5-G4`.
- Closed deviation: `D-AVAILABLE-SNAPSHOT-IGNORED`.
- Implementation refs:
  - `internal/modules/blocks/llm/biz/prompt_messages.go`
  - `internal/modules/blocks/llm/biz/prompt_messages_test.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- Actual change:
  - Added C4 `availableTargetSnapshots` summary beside the current requested action. The summary is bounded and carries `targetRef`, `contentHash`, `baseHash`, prompt safety flags, `sourcePath`, and allowed generic patch modes without embedding file content.
  - Added C4 `primaryNextAction` for state-changing repair so the immediate action is visible before nested diagnostics.
  - Preserved prompt snapshot safety: redacted/truncated/binary or prompt-truncated snapshots are marked `promptSafeForFileWrite=false` and `promptExtractionRequired=true`.
  - Added neutral contract rule `available_snapshot_ignored` when the rejected output claims snapshots/current target content are unavailable while the failed step input already contains complete safe target snapshots.
  - Added retry correction profile `available_snapshot_direct_action`, including available target snapshot summaries and forbidden repeat shapes such as `missing_snapshot_claim`.
- Boundary result:
  - The new classifier uses structured runtime snapshot availability plus generic missing-snapshot/current-target claims.
  - No branch depends on workspace template ID, Soul ID, live workspace ID, provider, programming language, LSP, concrete project file names, or business-specific text.
  - New prompt fields are rendered only in `C4.current-action`; `C0` stable prompt remains unchanged.
- Tests run:
  - `gofmt -w internal/modules/blocks/llm/biz/prompt_messages.go internal/modules/blocks/llm/biz/prompt_messages_test.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go internal/modules/kernel/orchestrator/biz/runtime_repair.go`
  - `go test ./internal/modules/blocks/llm/biz -run 'TestCurrentActionIncludesAvailableTargetSnapshots|TestCurrentActionMarksTruncatedAvailableSnapshotUnsafe|TestProviderCachePlanKeepsStablePrefixForAvailableTargetSnapshots|TestContractViolationRecoveryRendersInCurrentActionOnly' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestContractViolationClassifiesAvailableSnapshotIgnored|TestContractViolationDoesNotClassifyMissingSnapshotWithoutAvailableSnapshot|TestContractViolationCorrectionProfilesCoverPrimaryRules|TestRuntimeRepairInputIncludesContractViolationRecovery' -count=1`
  - `go test ./internal/modules/blocks/llm/biz -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1`
  - `go test ./... -count=1`
- Remaining risk:
  - This batch improves actionability for current target snapshots, but does not yet audit evidence/coverage projection completeness from Batch 6.
  - Full package and repository-wide tests still need to be rerun after Batch 5 and any subsequent closure batches are combined.
- Next gate:
  - Continue with Batch 6 evidence/coverage projection closure, especially whether rejected/accepted evidence, coverage items, and projection fields remain complete, bounded, and traceable.

### 2026-06-22 B5 Live Regression Supplemental Closure

- Reopened gate: `B5-G2`.
- Live evidence source:
  - `/tmp/workorch-b7-observe-summary-20260622111724/snapshot-ws-7ced70e98ea50c43.json`
- Observed deviation:
  - The failed step carried complete safe target snapshots in nested `failureTargetRepairContext` and `patchRepairContext`.
  - `input.availableTargetSnapshots` was absent and `repairActionPackage.targetSnapshots` was empty on the retry-visible step input.
  - The rejected output used a non-English missing-current-target-snapshot claim, so the classifier did not consistently route it to `available_snapshot_ignored`.
- Implementation refs:
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_input_test.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go`
- Actual change:
  - Added `ensureAvailableTargetSnapshotsForStateChangingRepair` to materialize generic available target snapshot summaries from nested repair contexts into `input.availableTargetSnapshots`.
  - The same helper synchronizes `repairActionPackage.targetSnapshots`, `currentTargetSnapshots`, and current-snapshot direct patch requirements when a state-changing repair already has complete safe target context.
  - Runtime repair input now reapplies this materialization after action-package creation, after convergence contract application, and before output-decision-contract refresh.
  - Contract violation recovery now also materializes the same fields before composing retry recovery context.
  - Missing-current-target-snapshot detection now recognizes generic Chinese missing/current-target/snapshot wording in addition to the existing English wording.
- Boundary result:
  - The change is still core-neutral: it only compares structured snapshot availability with generic missing-current-target claims.
  - No branch depends on workspace template ID, Soul ID, live workspace ID, provider, programming language, concrete filenames, LSP, or business-specific text.
  - Top-level `availableTargetSnapshots` remains metadata-only; full target content stays in the already-existing action package snapshot path when safe.
- Tests run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/runtime_repair_input.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go internal/modules/kernel/orchestrator/biz/runtime_repair_input_test.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(ApplyRuntimeRepairStateChangingActionPackageProjectionMaterializesAvailableTargetSnapshots|ContractViolationClassifiesAvailableSnapshotIgnored|ContractViolationClassifiesAvailableSnapshotIgnoredFromNestedChineseOutput|ContractViolationDoesNotClassifyMissingSnapshotWithoutAvailableSnapshot)$' -count=1 -timeout=240s`
- Remaining risk:
  - This closes the observed missing materialization/classification path.
  - It does not by itself prove deployed live goal convergence; Batch 7 live goal-convergence verification remains required.

## Batch 5 Decision

Batch 5 is code-closed for the audited context/actionability scope.

- `availableTargetSnapshots` and `primaryNextAction` are now surfaced in C4 current-action.
- Missing-snapshot claims against complete safe runtime snapshots are classified as `available_snapshot_ignored`.
- Snapshot truncation/redaction/binary prompt safety remains enforced.
- Dynamic current-action fields remain outside C0 stable prompt; cache-boundary regression tests cover this.

Future work should reopen Batch 5 only if a focused regression proves the C4 summary is absent from a state-changing repair request, the classifier misses a structured available-snapshot case, or cache partitioning regresses. Otherwise, remaining convergence work belongs to Batch 6 projection/evidence completeness or Batch 7 deployed/live verification.
