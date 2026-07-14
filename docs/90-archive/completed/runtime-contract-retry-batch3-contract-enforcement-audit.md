# Runtime Contract Retry Batch 3 Contract Enforcement Audit

## Purpose

Batch 3 audits whether WorkOrch runtime contract wording, requiredOutput, and enforcement gates are internally consistent. It focuses on contract enforcement only. It does not decide concrete software-development strategy, and it must not move LSP, provider gateway, Go, or live workspace-specific rules into core.

This batch consumes the Contract, Evidence, and Artifact dimensions from `runtime-contract-retry-full-chain-audit-plan.md`.

## Scope

In scope:

- Controlled substitute validation evidence.
- Validation shell safety.
- State-changing output requirement.
- Patch/apply target and snapshot rules.
- Same-graph read-before-patch constraints.
- Contract violation classification boundaries that are required before Batch 4 retry correction.

Out of scope:

- TUI/Web rendering.
- Provider-specific cache behavior.
- Template-specific verification strategy.
- Concrete deliverable implementation for either live workspace.

## Evidence Used

| Evidence | Meaning |
| --- | --- |
| `ev:ws4e/run4/ctr-validation-003/output/001` | Live controlled-substitute validation graph was rejected as shell mutation. |
| `ev:ws7c/run4/ctr-statechange-005/output/001` | Live state-changing retry returned failed DoD assessment instead of state-changing output. |
| `ev:ws7c/run4/ctr-statechange-005/request/001` | Complete target snapshots existed in the request despite the output claiming missing snapshots. |
| `ev:code/contract/concrete-validation/001` | Runtime concrete validation evidence gate. |
| `ev:code/contract/shell-safety/001` | Shell validation safety gate and temporary-target heuristic. |
| `ev:code/contract/state-changing/001` | Runtime state-changing output gate. |
| `ev:code/contract/patch-granularity/001` | Patch/apply existing-file and baseHash enforcement. |
| `ev:code/contract/current-snapshot/001` | RequiredOutput contract requiring direct patch from current snapshots. |
| `ev:code/contract/prompt-controlled-substitute/001` | LLM-facing contract text that asks the model to try controlled substitutes. |
| `ev:code/contract/graph-state-changing/001` | State-changing graph must include ungated patch/apply or accepted auditable state-changing operation. |
| `ev:code/contract/violation-feedback/001` | Current violation feedback hints are broad and prepare Batch 4 audit. |

## Contract Findings

### B3-CE-001 Controlled Substitute Contract Mismatch

- Claim: Controlled substitute validation is allowed by prompt contract but is not represented by a precise runtime scratch boundary.
- Expected behavior: If concrete validation requires controlled substitute evidence, runtime should provide a bounded scratch boundary and accept writes only inside that boundary, while still rejecting workspace deliverable mutation through validation shell commands.
- Actual behavior: Runtime uses command-line heuristics for temporary targets. A live graph with `validationEvidence.mode=controlled_substitute`, `observationType=controlled_substitute_observation`, and `/tmp/workorch-cwi-012-*` substitute fixtures was rejected as workspace mutation.
- Evidence: `ev:ws4e/run4/ctr-validation-003/output/001`, `ev:code/contract/concrete-validation/001`, `ev:code/contract/shell-safety/001`, `ev:code/contract/prompt-controlled-substitute/001`.
- Counter-evidence: No runtime-owned `validationScratchDir` contract is visible in the enforcement path cited by the evidence records.
- Deviation type: `D-CONTROLLED-SUBSTITUTE-SCRATCH-BOUNDARY`.
- Owner: core runtime contract.
- Closure batch: 3.
- Required code/doc/test action: Add an explicit validation scratch boundary to the runtime contract and enforce it structurally instead of relying only on command-line temporary-target heuristics.

### B3-CE-002 Validation Shell Safety Is Correct In Direction But Too Heuristic

- Claim: Runtime correctly rejects detached processes, background processes, process termination, and shell-based workspace mutation for validation, but the current mutation detector cannot reliably distinguish runtime-owned scratch writes from workspace writes.
- Expected behavior: Shell validation safety should be business-neutral and path-bound. It should reject destructive or unbounded shell shapes independent of template, and it should evaluate write destinations against workspace deliverable root and runtime scratch root.
- Actual behavior: The gate evaluates command-line markers such as `touch`, `mkdir`, `cp`, `tee`, redirection, and temporary aliases. It allows some isolated temp forms only when it recognizes temporary-target patterns.
- Evidence: `ev:code/contract/shell-safety/001`.
- Counter-evidence: The live controlled-substitute output proves the heuristic can reject an intended bounded substitute even when the output declares validation evidence metadata.
- Deviation type: `D-CONTROLLED-SUBSTITUTE-SCRATCH-BOUNDARY`.
- Owner: core runtime contract.
- Closure batch: 3.
- Required code/doc/test action: Keep destructive shell-shape rejection, but require explicit runtime scratch scope for write-capable validation nodes.

### B3-CE-003 State-Changing Output Gate Is Necessary And Mostly Correct

- Claim: Runtime should reject DoD-only assessment when a state-changing repair contract requires executable state-changing output.
- Expected behavior: If `stateChangingOutputRequired` is true, output must contain a valid state-changing executionGraph, accepted state-changing LLM output, incremental artifact output for the applicable phase, decision/approval blocker, or another explicitly accepted state-changing form.
- Actual behavior: Runtime returns `repair_no_state_changing_output` when a state-changing repair returns failed `dodAssessment` or no state-changing output.
- Evidence: `ev:code/contract/state-changing/001`, `ev:ws7c/run4/ctr-statechange-005/output/001`.
- Counter-evidence: The live request also contained complete target snapshots, so the repeated failure indicates retry correction/actionability weakness rather than a reason to weaken this gate.
- Deviation type: `D-AVAILABLE-SNAPSHOT-IGNORED`, `D-RETRY-CORRECTION-TOO-BROAD`.
- Owner: core runtime contract and retry correction.
- Closure batch: 3 for enforcement confirmation, Batch 4 and Batch 5 for correction/actionability.
- Required code/doc/test action: Keep the gate strict. Do not accept failed DoD assessment as state-changing output. Add targeted retry and current-action improvements in later batches.

### B3-CE-004 Patch Existing-File Rule Is More Nuanced Than Earlier Narrative

- Claim: Existing-file `file_write` is not universally forbidden; it is allowed only when the current repair context has a complete, safe snapshot and the patch request carries a matching `baseHash`.
- Expected behavior: `file_write` for an existing target must be rejected if it lacks complete/safe current context, if `baseHash` is missing or mismatched, if content includes redaction/template markers, or if a narrower patch form is required. `text_replace` and `unified_diff` remain valid when they satisfy required fields and relevance checks.
- Actual behavior: `validateRepairPatchRequestGranularity` enforces `file_write` by checking existing target context, `contentComplete`, `safeForFileWrite`, redaction/truncation/binary flags, matching `contentHash`/`BaseHash`, and unresolved markers.
- Evidence: `ev:code/contract/patch-granularity/001`, `ev:code/contract/current-snapshot/001`.
- Counter-evidence: Live `ws7c` still returned no patch, so the problem is not that patch/apply is impossible; the problem is that the retry request did not force the model back to the supplied current snapshot strongly enough.
- Deviation type: `D-PATCH-CONTRACT-RECOVERY-GAP`.
- Owner: core patch contract and retry correction.
- Closure batch: 3 for rule confirmation, Batch 4 for recovery contract.
- Required code/doc/test action: Document and test the safe `file_write` exception. Retry correction must cite allowed patch modes and baseHash requirements instead of only saying "state-changing output required".

### B3-CE-005 Same-Graph Read Before Patch Is Correctly Forbidden When Snapshot Exists

- Claim: When current snapshots are already supplied, a state-changing repair graph should not spend the same graph on read-before-patch.
- Expected behavior: The model must patch directly from the provided snapshot, return a real governance blocker, or wait for an explicit evidence-acquisition contract if the snapshot is insufficient.
- Actual behavior: `applyCurrentSnapshotDirectPatchContract` sets `mustUseCurrentTargetSnapshot`, `mustPatchDirectlyFromProvidedSnapshot`, `sameGraphReadPatchRule`, `patchApplyFieldRule`, and `readBeforePatchRecoveryRule`.
- Evidence: `ev:code/contract/current-snapshot/001`, `ev:code/contract/graph-state-changing/001`.
- Counter-evidence: `ws7c` output claimed snapshots were missing even though request evidence shows complete target snapshots. That is an actionability/retry problem, not a reason to permit read loops by default.
- Deviation type: `D-AVAILABLE-SNAPSHOT-IGNORED`.
- Owner: core runtime contract, LLM request assembly, retry correction.
- Closure batch: 3 for rule confirmation, Batch 5 for request/actionability.
- Required code/doc/test action: Surface an explicit available-target snapshot summary close to current-action and classify "snapshot missing" claims as a violation when complete snapshots are present.

## Contract Enforcement Matrix

| Contract Area | Current Rule | Design Assessment | Required Closure |
| --- | --- | --- | --- |
| Controlled substitute validation | Requires concrete validation graph and evidence metadata; shell safety may allow temp writes through heuristics. | Direction is correct, boundary is incomplete. | Add runtime scratch boundary and path-scoped enforcement. |
| Validation shell safety | Rejects process termination, detached/background process, and detected shell mutation. | Direction is correct and business-neutral. | Keep destructive-shape rejection; replace temp-write ambiguity with scratch path checks. |
| State-changing repair | Rejects failed DoD assessment when state-changing output is required. | Correct. | Keep strict; improve retry feedback rather than weakening. |
| Patch/apply existing file | Allows safe `file_write` only with complete/safe snapshot and matching baseHash; otherwise prefer exact patch forms. | Correct but easy for retry prompt to under-specify. | Add tests and targeted retry instructions. |
| Same-graph read-before-patch | Forbidden when current snapshots are supplied. | Correct. | Improve available snapshot visibility and violation classification. |

## Closure Gates

### B3-G1 Controlled Substitute Scratch Boundary

- Chain node: Runtime validation contract gate.
- Deviation type: `D-CONTROLLED-SUBSTITUTE-SCRATCH-BOUNDARY`.
- Evidence refs: `ev:ws4e/run4/ctr-validation-003/output/001`, `ev:code/contract/concrete-validation/001`, `ev:code/contract/shell-safety/001`, `ev:code/contract/prompt-controlled-substitute/001`.
- Owner: core runtime contract.
- Expected behavior: Controlled substitute validation may write only into runtime-provided validation scratch space and must never mutate workspace deliverable files through shell validation.
- Actual behavior: Runtime relies on command-line temp heuristics and rejects a declared substitute fixture path under `/tmp`.
- Proposed change: Add a runtime-owned `validationScratchDir` contract to validation repair input and enforce shell write destinations against workspace root and scratch root.
- Why this is not a business-specific core rule: Scratch-bound validation is a generic safety boundary and does not encode language, tool, template, or live workspace behavior.
- Required tests: Accept controlled substitute shell write inside scratch; reject write inside workspace; reject write outside scratch; reject detached/background/process-kill validation.

### B3-G2 State-Changing Output Gate Confirmation

- Chain node: Runtime repair output contract.
- Deviation type: `D-AVAILABLE-SNAPSHOT-IGNORED`.
- Evidence refs: `ev:ws7c/run4/ctr-statechange-005/output/001`, `ev:ws7c/run4/ctr-statechange-005/request/001`, `ev:code/contract/state-changing/001`.
- Owner: core runtime contract and retry correction.
- Expected behavior: Failed DoD assessment cannot satisfy state-changing repair when executable output is required.
- Actual behavior: Runtime correctly rejects the failed assessment, but retry does not make the already available snapshots actionable enough.
- Proposed change: Keep gate strict; route implementation work to Batch 4 and Batch 5 for targeted retry/current-action improvements.
- Why this is not a business-specific core rule: The gate is based on generic requiredOutput and state-changing contract flags.
- Required tests: A state-changing step returning only failed DoD assessment is rejected; decision/approval blockers remain accepted only when structurally valid.

### B3-G3 Patch/Apply Snapshot Rule Confirmation

- Chain node: Patch/apply graph node contract.
- Deviation type: `D-PATCH-CONTRACT-RECOVERY-GAP`.
- Evidence refs: `ev:code/contract/patch-granularity/001`, `ev:code/contract/current-snapshot/001`.
- Owner: core patch contract.
- Expected behavior: Existing target `file_write` is valid only with complete/safe snapshot and matching baseHash; exact patch forms are preferred for bounded repair.
- Actual behavior: Current code implements this rule, but retry feedback does not consistently force the model to use the supplied snapshot and allowed patch mode.
- Proposed change: Keep enforcement. Add regression tests and feed the rule into Batch 4 violation-specific correction.
- Why this is not a business-specific core rule: It is a generic artifact mutation safety rule.
- Required tests: Safe existing-file `file_write` passes with matching baseHash; missing/mismatched baseHash fails; redacted/truncated/binary snapshots fail; non-substantive unified diff fails.

### B3-G4 Validation Shell Safety Policy

- Chain node: Execution graph shell validation gate.
- Deviation type: `D-CONTROLLED-SUBSTITUTE-SCRATCH-BOUNDARY`.
- Evidence refs: `ev:code/contract/shell-safety/001`.
- Owner: core runtime contract.
- Expected behavior: Shell validation must remain bounded, foreground, non-destructive, and path-scoped.
- Actual behavior: Foreground/destructive checks are explicit; write path safety is command-line heuristic based.
- Proposed change: Preserve command-shape rejection and add path-scoped scratch/workspace checks for write-capable validation commands.
- Why this is not a business-specific core rule: It protects runtime integrity independent of workspace type.
- Required tests: Reject `nohup`, `setsid`, background `&`, process kill, and workspace path write attempts.

## Closure Records

### 2026-06-21 B3-G1 / B3-G4 Runtime Validation Scratch Boundary

- Closed gates: `B3-G1`, `B3-G4`.
- Closed deviation: `D-CONTROLLED-SUBSTITUTE-SCRATCH-BOUNDARY`.
- Implementation refs:
  - `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_contracts.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_execution_contract_integration_test.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_validation_diagnostic_test.go`
- Actual change:
  - Runtime now injects a neutral `validationScratchDir` into concrete validation repair contracts when `mustProduceConcreteValidationEvidence=true`.
  - `repairActionPackage`, `requiredOutput`, and step input carry the same scratch boundary so LLM current-action and runtime enforcement share one contract.
  - Write-capable validation modes (`controlled_substitute`, `dynamic_execution`, `executable_contract`, `runtime_behavior`) may write only under `validationScratchDir`.
  - Validation shell still rejects workspace deliverable mutation, detached/background process launch, process termination, and shell writes outside the runtime scratch boundary.
  - The previous broad `/tmp` / `mktemp` temporary-target heuristic is no longer the authority for accepting validation writes.
- Boundary result:
  - This is a core runtime safety contract. It does not encode workspace template IDs, Soul IDs, programming languages, live workspace IDs, provider names, LSP behavior, or concrete deliverable filenames.
  - Template/Soul may still choose a concrete validation strategy, but core only enforces the declared scratch boundary and shell safety shape.
  - Boundary search found existing hits in defaults/Soul data, MCP `serverId` fields, and historical test fixtures. The new production code only adds neutral `validationScratchDir` scratch-bound enforcement.
- Boundary searches run:
  - `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg`
  - `rg -n "stateChangingOutputRequired|repairObjective|providerTimeoutStateChangingRetry|assessmentOnly|runtimeRepairFollowUp|currentProgressDigest|memoryRecall|Cache-Control|cache_control" internal/modules/blocks/llm internal/modules/kernel`
  - `rg -n "definitionChangeImpact|coverageTargetSet|repairActionPackage|processEvidenceTimeline|requiredOutput|blocked_on_decision|contextGate" internal/modules/kernel internal/modules/blocks/llm`
  - `rg -n "validationScratchDir|controlledSubstituteScratchRule|validationScratchSetupRule|workorch-validation-test|workorch-validation-scratch|mktemp" internal/modules/kernel/orchestrator/biz internal/modules/blocks/llm/biz -g '*.go'`
- Tests run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go internal/modules/kernel/orchestrator/biz/runtime_execution_contract_integration_test.go internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go internal/modules/kernel/orchestrator/biz/runtime_repair_contracts.go internal/modules/kernel/orchestrator/biz/runtime_validation_diagnostic_test.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestValidateLLMExecutionOutputContract(AllowsControlledSubstitute|RejectsControlledSubstitute|AllowsDynamicExecutionTempCacheWrites|RejectsStateChangingGraphWithDetachedProcess|AllowsObservationModeAlias)|TestRepairActionPackageAddsValidationScratchDirForConcreteValidationEvidence' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1`
  - `go test ./internal/modules/kernel/orchestrator/... -count=1`
  - `go test ./internal/modules/blocks/llm/biz -count=1`
  - `go test ./... -count=1`
  - Evidence reference integrity check over `docs/70-roadmap/active/runtime-contract-retry-*.md` produced no missing definitions.
- Remaining risk:
  - This closure does not claim Batch 4 retry correction or Batch 5 current-action actionability is complete.
  - Live workspaces that already exhausted retry budget may still require a fresh replan/resume after the new scratch contract is present in later LLM requests.
- Next gate:
  - Continue with Batch 4 `B4-G1` / `B4-G2` so repeated same-rule contract violations receive narrow correction profiles instead of broad retry restatements.

### 2026-06-21 B3-G1 Supplemental Convergence Validation Scratch Closure

- Closed gap: convergence and continuation `need_validation` paths could rebuild `requiredOutput` from an existing repair action package without mirroring the runtime-owned `validationScratchDir`.
- Evidence:
  - Fresh bounded live observation at `/tmp/workorch-live-observe-20260621150205` showed `ws-4e83810542ac4d65` blocked on `concrete_validation_contract_missing`; its `contractViolationRecovery.expectedContract.repairActionPackage.requiredOutput` required controlled substitute validation but did not expose `validationScratchDir`.
  - Local code inspection showed `repairActionPackageFromRepairInput` injected scratch correctly, while convergence, coverage-work-item, concrete-validation continuation, and schema-invalid validation recovery paths could rebuild the validation `requiredOutput` without a shared scratch-injection helper.
- Implementation refs:
  - `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_graph.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_contracts.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_convergence_test.go`
- Actual change:
  - Added `concreteValidationEvidenceRequiredOutputWithScratch` so production code that rebuilds concrete validation `requiredOutput` can consistently mirror the runtime scratch boundary.
  - Coverage-work-item convergence, generic convergence `need_validation`, `ApplyConvergenceContract`, concrete-validation continuation inheritance, and schema-invalid concrete-validation recovery now use the shared helper.
  - The step input, repair action package, and validation `requiredOutput` carry the same `validationScratchDir` for concrete validation contracts.
  - The change is business-neutral: it only carries the runtime-owned scratch boundary for validation shell safety and does not inspect workspace IDs, template IDs, provider names, languages, target filenames, or business text.
- Tests run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go internal/modules/kernel/orchestrator/biz/runtime_convergence.go internal/modules/kernel/orchestrator/biz/runtime_graph.go internal/modules/kernel/orchestrator/biz/runtime_repair_contracts.go internal/modules/kernel/orchestrator/biz/runtime_convergence_test.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- Verification note:
  - Re-deployed live services must be re-observed after replan/resume to prove new LLM requests carry `validationScratchDir`; old already-blocked decisions may still contain pre-fix contract payloads.

### 2026-06-21 B3-G2 / B3-G3 State-Changing And Patch Snapshot Contract Confirmation

- Closed gates: `B3-G2`, `B3-G3`.
- Closed deviations:
  - `D-AVAILABLE-SNAPSHOT-IGNORED` enforcement confirmation.
  - `D-PATCH-CONTRACT-RECOVERY-GAP` enforcement confirmation.
- Implementation refs:
  - `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_execution_contract_integration_test.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_patch_contract.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_patch_requests.go`
- Actual change:
  - Confirmed and protected the strict state-changing output gate: a state-changing repair that returns only `dodAssessment`, including a failed assessment, is rejected as `repair_no_state_changing_output`.
  - Tightened blocker escape semantics: a state-changing repair may return a governance blocker only when the blocker is structurally actionable. Decision blockers now require a reason and question; approval blockers now require a reason and action. Reason-only placeholders no longer satisfy the state-changing output contract.
  - Confirmed and protected the existing-file `file_write` exception at the LLM output contract entry: it is accepted only when the supplied target snapshot is complete, safe, unredacted, untruncated, non-binary, and the patch carries a `baseHash` equal to the snapshot `contentHash`.
  - Confirmed contract-layer rejection for missing baseHash, mismatched baseHash, incomplete snapshots, unsafe snapshots, redacted snapshots, truncated snapshots, binary snapshots, and non-substantive unified diffs.
- Boundary result:
  - The change is a core runtime contract enforcement rule based only on structured `requiredOutput`, blocker, snapshot, patch, and baseHash fields.
  - It does not inspect workspace IDs, template IDs, Soul IDs, provider names, programming languages, concrete filenames for branching, or business text.
  - It keeps enforcement strict and does not move Batch 4 retry correction or Batch 5 current-action actionability into this batch.
- Tests run:
  - `gofmt -w internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go internal/modules/kernel/orchestrator/biz/runtime_execution_contract_integration_test.go`
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestValidateLLMExecutionOutputContract(AllowsStructuredStateChangingBlockers|RejectsIncompleteStateChangingBlockers|RejectsStateChangingRepairSatisfiedAssessmentOnly|AllowsSafeRepairFileWriteForExistingContextTarget|RejectsRepairFileWriteForExistingContextTargetWithoutBaseHash|RejectsUnsafeRepairFileWriteForExistingContextTarget|RejectsNonSubstantiveRepairUnifiedDiff)' -count=1`
  - `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- Remaining risk:
  - This closure proves contract enforcement. It does not prove that retry prompts always make the supplied snapshots visible enough for the model to act correctly; that remains a Batch 4 and Batch 5 responsibility.
  - Live workspaces already in old waiting-decision states may require a fresh replan/resume to receive the tightened blocker and patch snapshot contract.

## Batch 3 Decision

Batch 3 is code-closed for contract enforcement.

- Validation scratch boundaries, strict state-changing output gates, structured blocker escape semantics, and safe existing-file `file_write` exceptions are covered by focused regressions.
- The implementation must not weaken strict runtime contracts.
- The follow-up dependency on retry correction generation has been closed by Batch 4; the follow-up dependency on current-action visibility has been closed by Batch 5.

Future work should reopen Batch 3 only if a focused regression proves contract enforcement is internally inconsistent, validation scratch boundaries are missing from a new concrete-validation path, or strict output gates reject/accept an output contrary to the canonical runtime contract.
