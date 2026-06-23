# Runtime Coverage / Convergence State Machine Closure Plan

## Purpose

Recent Batch 7 live verification exposed a repeated local-patch pattern around coverage ledger, convergence contract, repair action package, output contract, and evidence recording. This document fixes the closure direction before additional code changes.

The target is a single business-neutral state machine:

1. Coverage ledger owns effective requirement state and accepted/rejected evidence.
2. Convergence contract maps that state to one next runtime action.
3. Repair action package renders only the selected action contract.
4. Output contract validates the selected action, not historical alternatives.
5. Evidence recording feeds back into the ledger and recomputes the next state.

Core must remain template-neutral. It may consume structured `requiredEvidenceKinds`, `requiredEvidenceCounts`, `evidenceRole`, `coverageFocus`, and runtime step metadata. It must not infer requirements from workspace title, template ID, Soul ID, natural-language business text, file names, language names, provider names, or specific tool output markers.

## Current Loop Evidence

The current loop is documented in `runtime-contract-retry-batch7-regression-live-verification-audit.md` around `20260623-035` through `20260623-044`.

Observed pattern:

- A focused live blocker is fixed.
- The successor run advances one layer.
- A neighboring path in the same state-machine chain interprets the same ledger state differently.
- Runtime opens another repair/decision loop.

High-risk areas:

- `runtime_repair_focus_contract.go`: focus binding is normalized in several places.
- `runtime_coverage_evidence.go`: required evidence kinds and missing evidence are recomputed from mixed source types.
- `runtime_coverage_work_items.go`: `status`, `missingEvidenceKinds`, and `currentGap` are derived together but consumed independently.
- `runtime_convergence.go` / `runtime_graph.go`: multiple predicates decide whether a coverage item is actionable.
- `runtime_repair_action_package.go` / `runtime_execution_contract.go`: required output can drift from the ledger state that selected it.

## Authoritative Coverage States

Coverage work item state is authoritative only after this tuple is evaluated together:

- `status`
- `requiredEvidenceKinds`
- `requiredEvidenceCounts`
- `acceptedEvidenceByKind`
- `rejectedEvidenceByKind`
- `missingEvidenceKinds`
- `currentGap`
- `evidenceRole`

No consumer should independently reinterpret `currentGap` without first considering `status` and `missingEvidenceKinds`.

| State | Required Evidence | Accepted Evidence | Meaning | Next Runtime Action |
| --- | --- | --- | --- | --- |
| `open` | Present or defaulted | None or insufficient | Evidence must be acquired. | `need_evidence` |
| `in_progress` | Present | Partial | Continue acquiring missing required evidence kinds. | `need_evidence` |
| `evidence_ready` | All required kinds satisfied | Complete but not finally assessed | Do not request more mechanical evidence for this item. | `need_assessment` |
| `satisfied` | All required kinds satisfied | Complete and accepted by assessment/final gate | Closed. | `complete` |
| `blocked` | Any | Any | Runtime lacks external dependency, permission, credential, approval, or information. | `need_decision` or `need_approval` |
| `skipped` | Any | Any | Explicitly excluded by structured policy or scope. | `complete` for this item |
| `failed` | Any | Rejected or no usable evidence | Needs retry only if there is an actionable missing evidence kind or structured current failure. | `need_evidence`, `need_repair`, or `need_decision` |

### State Invariants

- `missingEvidenceKinds` is the only mechanical evidence-acquisition gap.
- `currentGap == "required evidence kinds present; coverage assessment required"` is not an evidence-acquisition gap.
- `evidence_ready` with empty `missingEvidenceKinds` must not select `mustProduceConcreteValidationEvidence`.
- `satisfied` must have traceable accepted evidence refs unless explicitly skipped or policy-exempt.
- Governance evidence (`decision`, `approval`) is required only when declared through structured source data or owner evidence policy.
- Existing owner repository items are lineage, not schema authority; effective required kinds are recomputed from the current definition and structured source.

## Convergence Action Matrix

Convergence must produce one primary next action.

| Coverage State | Additional Condition | Convergence State | `repairObjective` | Required Output |
| --- | --- | --- | --- | --- |
| `open` / `in_progress` | Missing `validation` only | `need_validation` | `concrete_validation_evidence` | bounded `shell/command` or `mcp/tool_call` validation graph |
| `open` / `in_progress` | Missing `state_change` | `need_state_change` | `state_changing_output` | `patch/apply`, artifact/write, git/write, or other state-changing evidence-producing output allowed by contract |
| `open` / `in_progress` | Missing governance owner kind | `need_decision` / `need_approval` only when owner evidence cannot be produced automatically | scoped ticket / approval | owner-scoped resolution |
| `failed` | Structured retryable contract violation with actionable target | `need_repair` | violation-specific repair | violation-specific required output |
| `failed` | No actionable target or external dependency missing | `need_decision` | escalation | scoped decision |
| `evidence_ready` | No missing required kind | `need_assessment` | `complete_dod_goal_coverage_assessment` | assessment-only JSON with concrete evidenceRefs |
| `satisfied` / `skipped` | None | `complete` | none | none |

### Required Output Ownership

`repairActionPackage.requiredOutput` must be generated from the selected convergence state, not patched by downstream consumers.

- `need_validation` owns `mustProduceConcreteValidationEvidence=true`.
- `need_state_change` owns `stateChangingOutputRequired=true` or equivalent structured required output.
- `need_assessment` strips executable required-output fields and sets `assessmentOnly=true`.
- `need_decision` / `need_approval` must include structured external dependency, permission, credential, or exhausted automation reason.

## LLM Contract Violation Tolerance

LLM output is not expected to be perfectly stable. Runtime convergence must not depend on the LLM eventually choosing the correct state-machine transition by itself.

The runtime owns all deterministic state transitions:

- Coverage ledger state classification.
- Missing evidence calculation.
- Assessment-ready detection.
- Focus binding.
- Required-output rendering.
- Accepted/rejected evidence recording.
- Decision/approval gate eligibility.
- Completion gate evaluation.

The LLM owns only candidate content generation:

- Proposed state-changing output.
- Proposed validation evidence graph.
- Proposed assessment with evidence refs.
- Proposed governance blocker with structured external dependency.

When an LLM output violates the runtime contract, the recovery path must be selected by deterministic runtime classification before any retry prompt is built.

| Violation Class | Runtime Action | Retry Use |
| --- | --- | --- |
| Parse failure with no safely extractable structure | Record rejected diagnostic evidence and retry within budget using the same selected action. | Yes, bounded. |
| Wrong output shape but no side effects executed | Record rejected diagnostic evidence and retry with the selected action's narrow required output. | Yes, bounded. |
| Semantically useful assessment with safe evidence refs but wrong wrapper | Normalize only the safe assessment fields, then run normal assessment gate. | Retry only if normalization cannot prove safety. |
| Required evidence already present but LLM asks for more mechanical evidence | Ignore the LLM-selected action and route to `need_assessment`. | No evidence retry. |
| Continuable work returned as `blocked_on_decision` | Run the decision ticket gate; only structured external dependency may create a ticket. | Retry or continue automatically when no external dependency exists. |
| Duplicate/stale patch or stale target snapshot | Reject as diagnostic evidence and rebuild the selected state-changing contract from the latest target snapshot. | Yes, bounded if current target is available. |
| Provider timeout or incomplete stream | Retry provider call only when no side effects were executed and lineage is idempotent. | Yes, bounded by provider retry policy. |

Runtime must never execute, accept, or persist invalid LLM output as accepted result/artifact/evidence. Invalid output may be persisted only as rejected diagnostic evidence with provenance.

### Stable Convergence Rule

A retry is allowed only when the selected next action still requires LLM-generated content. If the next action can be derived from the ledger without new LLM content, runtime must advance deterministically instead of asking the LLM to restate the same contract.

## Focus Binding Rules

Coverage focus binding is a contract between selected action and evidence recording.

Rules:

- A single function should derive the canonical focus token set from `coverageWorkItemContext.currentFocus`.
- Any step that can produce accepted evidence for the current focus must carry one of the canonical binding forms:
  - `coverageFocusId`
  - `focusId`
  - `coverageWorkItemId`
  - `coverageWorkItemIds`
  - `coverageTargets`
- `patch/apply` remains strict: it must not be silently rebound to a different target.
- Non-patch validation or state-changing evidence nodes may inherit an unambiguous current focus only when exactly one active focus exists.
- Evidence recording must reject focus-bound evidence that does not match the current canonical token set.

## Owner Boundaries

| Concern | Owner |
| --- | --- |
| Effective Goal/DoD definition and coverage source extraction | coverage ledger |
| Required evidence kind derivation | coverage ledger from structured data |
| Current focus and accepted/rejected evidence lineage | coverage ledger |
| Next runtime action | convergence contract |
| Required output fields | convergence contract / repair action package builder |
| LLM output validation | execution contract |
| Step evidence acceptance/rejection | coverage ledger |
| Workspace/TUI/Web state display | workorchd projection |

TUI/Web must display projection only. They must not infer coverage state or repair action.

## Current Implementation Gaps To Close

### G1: Predicate Duplication

Multiple predicates infer whether coverage needs more evidence. Examples include:

- `coverageWorkItemHasOpenGap`
- `coverageMissingEvidenceKinds`
- `coverageVerificationGapPresent`
- `convergenceNeedsValidation`
- `coverageAssessmentOnlyFailureHasActionableGap`

Closure:

- Introduce a neutral derived classification helper for coverage work items.
- Consumers ask the helper for `NextAction`, not individual fields.
- Keep small wrappers for compatibility only if they delegate to the helper.

### G2: `currentGap` Is Overloaded

`currentGap` is both display text and decision input. This caused `evidence_ready` to be treated as an actionable evidence gap.

Closure:

- Treat `currentGap` as display text only.
- Mechanical evidence selection must use derived `missingEvidenceKinds`.
- Assessment-only gap should be represented as `NextAction=need_assessment`.

### G3: Required Output Drift

`requiredOutput` can be rebuilt by later repair/output-mode projections and lose focus or required action fields.

Closure:

- Build `requiredOutput` only through a single convergence-state renderer.
- Reapply focus binding by recomputing from canonical focus state, not by appending fields in multiple downstream paths.

### G4: Legacy Owner Repository Fields Can Pollute Effective State

Persisted coverage work items may contain stale required kinds created by older derivation rules.

Closure:

- On hydration, treat repository entries as evidence lineage and previous status only.
- Recompute effective required kinds from current structured source.
- Preserve accepted/rejected evidence only for effective kinds.

### G5: Live Verification Still Uses Historical Decisions

After a local fix, live workspaces may remain on decisions produced by old binaries.

Closure:

- Live verification must record binary commit, process start time, active run ID, and whether the decision was created before or after the fix.
- Positive verification requires redeploy plus explicit `replan` / `resume` using the fixed binary.

## Code Closure Order

1. Add the derived coverage work-item action helper and tests. Completed locally in `runtime_coverage_state.go` and `runtime_coverage_state_test.go`.
2. Migrate high-risk predicates to delegate to the helper. Completed locally for coverage gap predicates, target predicates, DoD/gap baseline consumers, and acceptance slice matching.
3. Centralize convergence-state-to-required-output rendering. Completed locally for coverage work-item convergence renderers and follow-up application paths.
4. Migrate focus binding to one canonical token helper. Completed locally for state-changing, validation, and evidence acquisition required-output rebuilds.
5. Add integration tests that cover the full chain. Completed locally in `TestCoverageConvergenceFullChainActionMatrix` and focus-binding required-output tests:
   - coverage item with missing validation -> validation graph required
   - coverage item with missing state_change -> state-changing output required
   - evidence_ready item -> assessment-only required
   - satisfied item -> no repair required
   - structured decision/approval item -> owner evidence required
   - legacy text-inferred decision item -> normalized to validation only
6. Run boundary verification and selected live verification. Local boundary verification is required before commit; live verification remains required after redeploy.

### Local Closure Notes

- `missingEvidenceKinds` is now read through the derived coverage state for convergence, target predicates, acceptance grouping, DoD/gap baseline, and current failure projection.
- `currentGap` remains display/diagnostic text and is not the mechanical evidence-acquisition source.
- `ApplyConvergenceContract` now preserves coverage focus binding when rebuilding `need_evidence` required output.
- Evidence acquisition required-output rebuilds that have access to `input` use the focus-aware builder so `coverageFocus`, `mustBindCoverageFocus`, `coverageFocusId`, and `coverageWorkItemIds` are not dropped.
- The legacy `coverageMissingEvidenceKinds` compatibility entry now delegates to the derived state machine. This prevents stale `evidence_ready` / `satisfied` status from closing missing evidence without enough structured or legacy traceable evidence refs.
- Terminal coverage states are no longer trusted unconditionally: `satisfied` / `closed` / `completed` still reopen to `need_evidence` when required evidence counts are not satisfied. `skipped` remains the explicit policy-exempt terminal state.

### Contract Audit: Legacy Missing Evidence Wrapper

- Scope: `internal/modules/kernel/orchestrator/biz/runtime_coverage_evidence.go`.
- Observed state: the derived coverage state helper enforced by-kind evidence semantics, but the legacy `coverageMissingEvidenceKinds` wrapper still returned no missing evidence for `evidence_ready` / `satisfied` items with empty `acceptedEvidenceByKind`; the derived helper also trusted terminal status before checking evidence completeness.
- Expected state: all missing-evidence consumers, including compatibility wrappers and ledger update helpers, use the same derived mechanical evidence gap.
- Deviation: a stale status field could bypass the derived state-machine rules and make old consumers treat ambiguous flat evidence as complete.
- Owner: core coverage ledger / convergence state machine.
- Event path: accepted evidence update -> missing evidence recompute -> coverage ledger state -> convergence contract -> repair action package.
- Contract gap: legacy helper did not delegate to the authoritative `deriveCoverageWorkItemState` classification, and terminal status could bypass required evidence count validation.
- Proposed change: make `coverageMissingEvidenceKinds` return `deriveCoverageWorkItemState(item).MissingEvidenceKinds`; make `deriveCoverageWorkItemState` validate required evidence before accepting `satisfied` / `closed` / `completed` as complete.
- Boundary check: the change consumes only structured evidence fields and status; it adds no template, language, provider, command, or workspace-specific rule.
- Verification: `TestCoverageMissingEvidenceKindsDelegatesToDerivedState` proves ambiguous flat evidence does not close multi-kind requirements through the legacy wrapper; `TestDeriveCoverageWorkItemState` covers stale satisfied items reopening to evidence acquisition.

## Verification Gates

Minimum local verification for code closure:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=300s
./scripts/verify-boundary-contract.sh
git diff --check
go build -o ./bin/workorch ./application/workorch
```

Live verification after redeploy:

1. Capture `git rev-parse --short HEAD`.
2. Capture workorch PID/start time and `/readyz`.
3. Resolve only scoped open decisions that allow `replan` or `resume`.
4. Observe active run until one of:
   - accepted result/artifact closure
   - structured decision/approval with external dependency
   - bounded failure with a new audited defect
5. Record coverage item states and latest step outputs in the Batch 7 audit.

## Non-Goals

- Do not weaken contract validation to make live runs pass.
- Do not add software-dev, language, file, provider, or workspace-specific rules to core.
- Do not auto-approve approvals.
- Do not use watchdog or recovery timers to hide lost events or invalid state transitions.
- Do not treat raw file existence as Goal/DoD closure without accepted evidence linkage.
