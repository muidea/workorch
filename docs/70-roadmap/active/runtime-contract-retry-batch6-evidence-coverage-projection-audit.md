# Runtime Contract Retry Batch 6 Evidence Coverage Projection Audit

## Purpose

Batch 6 audits whether runtime outputs become the correct evidence/result/artifact records, whether accepted evidence moves the coverage ledger, and whether workorchd projects those states as ViewModel data without requiring TUI/Web to infer backend business state.

This batch consumes the Evidence, Coverage, Artifact, and Projection dimensions from `runtime-contract-retry-full-chain-audit-plan.md`.

The audit remains business-neutral. Live workspace files and git state may be cited as evidence samples, but core behavior must use structured evidence refs, result state, artifact state, coverage work items, acceptance slices, and projection ViewModel fields.

## Scope

In scope:

- Accepted result recording from validated DoD assessment.
- Rejected diagnostic result handling for failed completion gates.
- Accepted slice result recording before final completion.
- Artifact creation and idempotent artifact update.
- Coverage work item ledger movement from step, assessment, and governance evidence.
- Final acceptance gate based on unresolved coverage work items and accepted evidence refs.
- workorchd projection of coverage summary, gap baseline, result quality, runtime evidence summary, results, and artifacts.

Out of scope:

- Weakening DoD coverage semantics to accept unverifiable output.
- Template-specific evidence policy content.
- TUI/Web client-side business inference.
- Provider-specific usage, cache, or streaming behavior.

## Evidence Used

| Evidence | Meaning |
| --- | --- |
| `ev:code/evidence/result-recording/001` | Accepted final result is recorded from validated DoD assessment with evidence refs and artifact refs. |
| `ev:code/evidence/slice-result-recording/001` | Coverage-progress assessments can record accepted slice results before final completion. |
| `ev:code/evidence/rejected-result-boundary/001` | Rejected DoD assessments remain rejected results and can be superseded by recovery. |
| `ev:code/evidence/artifact-recording/001` | Artifacts are scoped by workspace/run/result/step and carry evidence refs. |
| `ev:code/evidence/memory-boundary/001` | Diagnostic-only and rejected contract outputs are filtered from long-lived memory; validated run summary memory carries evidence refs. |
| `ev:code/coverage/work-item-ledger/001` | Coverage work items are updated from completed/failed steps, assessments, and governance owner evidence. |
| `ev:code/coverage/acceptance-gate/001` | Final acceptance is blocked if coverage work items are unresolved or not linked to accepted evidence. |
| `ev:code/projection/coverage-summary/001` | Projection derives coverage summary, gap baseline, accepted/rejected evidence refs, and todo items. |
| `ev:code/projection/dod-evidence-trace/001` | Projection labels DoD item evidence refs as linked, partial, or missing. |
| `ev:code/projection/result-artifact-owner-read/001` | Read model result/artifact pages use EventHub owner commands with workspace/run scope. |
| `ev:code/projection/result-artifact-view/001` | Projection exposes artifact refs, result quality, and runtime evidence totals. |
| `ev:ws4e/artifact/git/001` | Live sample with produced commits/artifacts. |
| `ev:ws7c/artifact/git/001` | Live sample with untracked skeleton files and no accepted final version snapshot. |

## Audit Questions

1. Which runtime outputs become accepted evidence, rejected diagnostic evidence, result records, artifacts, or memory?
2. Does accepted evidence move the coverage ledger only when it matches the active coverage work item contract?
3. Does rejected evidence preserve diagnostic lineage without being counted as accepted coverage?
4. Does final acceptance require all unresolved coverage work items to be satisfied or explicitly blocked/skipped?
5. Are artifacts complete enough to support final closure, or only intermediate diagnostic records?
6. Does workorchd projection expose enough state for TUI/Web to render coverage, evidence, artifacts, retry, and decision state without separate client inference?

## Audit Findings

### B6-ECP-001 Accepted Final Result Path Exists

- Claim: Runtime has a path for recording accepted final results from validated DoD assessments.
- Expected behavior: Accepted final result should include workspace/run/goal/DoD scope, evidence refs, artifact refs, summary detail, and accepted state.
- Actual behavior: Code creates accepted result records and publishes result accepted events after summary/manifest/patch/web/git artifact refs are collected.
- Evidence: `ev:code/evidence/result-recording/001`.
- Counter-evidence: Live workspaces did not reach this path because contract retry and coverage blockers occurred earlier.
- Deviation type: none.
- Owner: core result subsystem.
- Closure batch: 6.
- Required code/doc/test action: Preserve accepted-result scope and evidence requirements. Do not weaken the accepted result path to force live closure while upstream contract/retry/context gates remain open.

### B6-ECP-002 Rejected Diagnostic Results Are Distinct From Accepted Results

- Claim: Runtime records rejected completion diagnostics without counting them as accepted closure evidence.
- Expected behavior: A rejected DoD assessment should remain a rejected result with source/evidence lineage until a recovery step supersedes it; final accepted completion must require accepted state, matching scope, satisfied assessment, and evidence refs.
- Actual behavior: `recordRunResultFromRejectedDoDAssessment` writes `ResultStateRejected`, keeps source/evidence refs, and `acceptedCompletionResultMatchesRun` rejects anything that is not accepted/satisfied/evidence-linked.
- Evidence: `ev:code/evidence/rejected-result-boundary/001`.
- Counter-evidence: Earlier code used the internal helper name `publishResultAccepted` for all result states. This naming clarity issue is now closed; the helper is `publishResultEvent`, and the topic is still selected from the persisted result state.
- Deviation type: none for persisted evidence semantics; possible projection/event naming clarity follow-up.
- Owner: core result subsystem / projection.
- Closure batch: 6.
- Required code/doc/test action: Add or confirm tests that rejected result state is not counted as final accepted completion and can be superseded by recovery.

### B6-ECP-003 Coverage Ledger Has A Strict Acceptance Gate

- Claim: Coverage work item acceptance is strict and evidence-linked.
- Expected behavior: Final acceptance should fail if any coverage work item is unresolved, missing required evidence, or marked satisfied without accepted evidence refs.
- Actual behavior: `validateReadyForAcceptance` rejects unresolved work items and satisfied items without accepted evidence refs.
- Evidence: `ev:code/coverage/acceptance-gate/001`.
- Counter-evidence: Strictness can slow closure when upstream context/retry fails to produce targeted evidence, but that is a Batch 3-5 issue rather than a reason to weaken Batch 6 acceptance.
- Deviation type: none.
- Owner: core coverage ledger.
- Closure batch: 6.
- Required code/doc/test action: Preserve strictness. If live workspaces remain blocked, trace whether the missing evidence is due to contract/retry/context gaps before changing coverage logic.

### B6-ECP-004 Coverage Ledger Moves From Multiple Owner Evidence Sources

- Claim: Coverage can be updated by completed/failed steps, validated assessments, and governance owner evidence.
- Expected behavior: The ledger should accept only structured evidence refs that match work item targets/kinds and should preserve rejected lineage for failed or unmatched evidence.
- Actual behavior: `coverageSubsystem` updates work items from step outputs, failed steps, assessment items, and decision/approval owner refs, then synchronizes coverageWorkItemContext and acceptance projection.
- Evidence: `ev:code/coverage/work-item-ledger/001`.
- Counter-evidence: No evidence in this audit shows accepted evidence being incorrectly ignored or rejected evidence being counted as accepted.
- Deviation type: none.
- Owner: core coverage ledger.
- Closure batch: 6.
- Required code/doc/test action: Maintain tests for accepted evidence movement, rejected evidence preservation, and governance owner evidence sync.

### B6-ECP-005 Memory Boundary Keeps Diagnostic-Only Outputs Out Of Long-Lived Recall

- Claim: Runtime memory does not persist rejected contract output or transient provider diagnostics as high-value memory.
- Expected behavior: Memory should contain accepted run summaries, accepted skill/tool/profile feedback, or reusable lessons with provenance. Rejected diagnostic output should remain current recovery evidence, not long-lived recall.
- Actual behavior: Runtime memory filters transient tags/text, diagnostic-only text, rejected_output, and contract_violation. Run-summary memory is only written from validated DoD assessment with scope, sourceRef, tags, importance, and confidence.
- Evidence: `ev:code/evidence/memory-boundary/001`.
- Counter-evidence: This audit did not inspect every autogenerated repair lesson path; Batch 5 still owns context/noise issues.
- Deviation type: none for result-summary memory; keep repair-lesson memory under Batch 5/7 regression.
- Owner: core memory / result subsystem.
- Closure batch: 6.
- Required code/doc/test action: Preserve rejected diagnostic memory filter and include memory boundary in Batch 7 regression if contract retry code changes memory paths.

### B6-ECP-006 Projection Exposes Coverage, Evidence, And Task State

- Claim: workorchd projection exposes coverage summary, gap baseline, result quality, runtime evidence totals, results, artifacts, DoD evidence linkage state, and first-class task/retry state.
- Expected behavior: TUI/Web should consume this projection and not reconstruct business state from raw event streams.
- Actual behavior: Projection derives coverage and evidence views from run snapshots and owner reads, marks DoD item evidence as linked/partial/missing, and now carries `TaskPlans` / `TaskDecompositions` with rejected/retrying/blocked lifecycle metadata through run, workspace, remote, port and TUI surfaces.
- Evidence: `ev:code/projection/coverage-summary/001`, `ev:code/projection/dod-evidence-trace/001`, `ev:code/projection/result-artifact-owner-read/001`, `ev:code/projection/result-artifact-view/001`, `ev:code/projection/progress-mixed-items/001`.
- Counter-evidence: `TaskProgress.Items` remains a raw activity surface by design; clients must prefer `TaskPlans` for todo/retry rendering.
- Deviation type: none after Batch 2 closure; keep projection-only invariant under Batch 7.
- Owner: projection / ViewModel.
- Closure batch: 2 and 6.
- Required code/doc/test action: Preserve `TaskPlans` as the primary planned-task ViewModel and keep raw activity separate.

### B6-ECP-007 Artifact Snapshot Completeness Is Not Equivalent To File Existence

- Claim: Produced workspace files, untracked files, commits, and result artifacts are different evidence classes and must not be conflated.
- Expected behavior: Final closure requires accepted result/artifact evidence linked to coverage, not merely files present in the deliverable directory.
- Actual behavior: Live samples show one workspace with commits/artifacts and another with untracked skeleton files; neither is sufficient by itself to prove final Goal/DoD closure.
- Evidence: `ev:ws4e/artifact/git/001`, `ev:ws7c/artifact/git/001`.
- Counter-evidence: File outputs can still be valid intermediate state-changing evidence when linked through runtime evidence refs and coverage targets.
- Deviation type: `D-ARTIFACT-SNAPSHOT-INCOMPLETE`.
- Owner: core evidence / projection / template policy boundary.
- Closure batch: 6.
- Required code/doc/test action: Define artifact completeness as generic structured linkage: workspace/run/result/step scope, artifact state/category, evidenceRefs, coverage linkage, and accepted result relation. Do not infer final closure from file names, git status, or language-specific deliverables.

## Evidence Acceptance Matrix

| Source | Accepted Evidence When | Rejected Evidence When | Projection Requirement |
| --- | --- | --- | --- |
| Runtime step | Step completes and output maps to coverage targets or accepted runtime evidence. | Step fails, violates contract, or produces unmatched assessment evidence. | Show step evidence refs and state. |
| DoD assessment | Assessment passes validation, evidence quality, and coverage work item gate. | Completion gate rejects assessment or assessment is synthetic diagnostic output. | Show result quality and DoD evidence trace. |
| Patch / state change | Patch/tool result is accepted by runtime contract and linked to coverage targets. | Patch contract rejects unsafe output or missing target linkage. | Show artifact/result refs and rejected diagnostic lineage. |
| Decision / approval | Owner evidence resolves required governance work item. | Missing or unresolved governance owner evidence. | Show open/resolved decision/approval state and coverage evidence refs. |
| Artifact | Created by result subsystem with workspace/run/result/step scope and evidence refs. | File exists without accepted runtime evidence linkage. | Show artifact scope, state, category, URI, and evidence refs. |

## Closure Gates

### B6-G1 Accepted And Rejected Evidence Ownership

- Chain node: runtime result/evidence recording.
- Deviation type: `D-ARTIFACT-SNAPSHOT-INCOMPLETE` if artifact/file state is confused with accepted evidence.
- Evidence refs: `ev:code/evidence/result-recording/001`, `ev:code/evidence/slice-result-recording/001`, `ev:code/evidence/rejected-result-boundary/001`, `ev:code/evidence/artifact-recording/001`, `ev:code/evidence/memory-boundary/001`.
- Owner: core result/evidence subsystem.
- Expected behavior: Accepted evidence, rejected diagnostic evidence, results, artifacts, and memory have distinct ownership and states.
- Actual behavior: Code refs show distinct accepted, rejected, slice result, artifact, and memory boundaries. Live workspaces need verification after upstream retry fixes, but Batch 6 does not justify weakening those boundaries.
- Proposed change: No code change unless a regression test proves rejected diagnostic state is projected or counted as accepted evidence.
- Why this is not a business-specific core rule: It uses generic result/artifact/evidence state and refs.
- Required tests: Accepted result and rejected diagnostic result cannot be projected or counted as the same evidence state.

### B6-G2 Coverage Ledger Movement

- Chain node: coverage work item ledger.
- Deviation type: new only if accepted evidence fails to move matching coverage or rejected evidence is counted as accepted.
- Evidence refs: `ev:code/coverage/work-item-ledger/001`, `ev:code/coverage/acceptance-gate/001`.
- Owner: core coverage ledger.
- Expected behavior: Accepted evidence closes matching coverage items; rejected evidence preserves lineage without closing required coverage.
- Actual behavior: Code is strict and evidence-linked. Current live failures in L1 are upstream contract/retry/context blockers rather than proven ledger movement defects.
- Proposed change: Preserve strict acceptance unless a precise ledger mismatch is found.
- Why this is not a business-specific core rule: Coverage work items are generic structured records.
- Required tests: Coverage item accepts required evidence refs, rejects unmatched evidence, and blocks final acceptance when unresolved.

### B6-G3 Artifact Snapshot Completeness

- Chain node: artifact recording and projection.
- Deviation type: `D-ARTIFACT-SNAPSHOT-INCOMPLETE`.
- Evidence refs: `ev:code/evidence/artifact-recording/001`, `ev:code/projection/result-artifact-view/001`, `ev:ws4e/artifact/git/001`, `ev:ws7c/artifact/git/001`.
- Owner: core evidence / projection / template policy boundary.
- Expected behavior: Artifact completeness is represented as structured artifact/result/evidence linkage, not inferred from raw files alone.
- Actual behavior: Artifact records include scope/state/category/evidence refs, and workspace snapshot merging now carries active run `Results` / `Artifacts` into the workspace ViewModel so clients can inspect accepted result-linked artifacts from projection rather than inferring from raw file/git state.
- Proposed change: Completed as a projection clarity gate: workspace views explicitly include active run result/artifact details and still keep raw activity separate.
- Why this is not a business-specific core rule: It describes generic artifact state and evidence linkage.
- Required tests: Projection distinguishes available artifact and accepted result-linked artifact through formal result/artifact fields; raw workspace file/git state is not used as final closure input.

### B6-G4 Projection ViewModel Completeness

- Chain node: workorchd read model projection.
- Deviation type: `D-PLANNED-TASK-PROJECTION-GAP` or new evidence projection gap.
- Evidence refs: `ev:code/projection/coverage-summary/001`, `ev:code/projection/dod-evidence-trace/001`, `ev:code/projection/result-artifact-owner-read/001`, `ev:code/projection/result-artifact-view/001`, `ev:code/projection/progress-mixed-items/001`.
- Owner: projection / ViewModel.
- Expected behavior: TUI/Web can render coverage, evidence, artifact, retry, and decision state from workorchd projection only.
- Actual behavior: Coverage/evidence/result/artifact projection exists and uses owner reads; Batch 2 task/retry projection is now carried through the same ViewModel surfaces.
- Proposed change: Do not add TUI/Web inference. Future clarity gaps should add generic ViewModel fields to workorchd projection.
- Why this is not a business-specific core rule: Projection exposes generic runtime state.
- Required tests: Snapshot projection includes coverage summary, accepted/rejected evidence refs, artifacts, results, and retry/decision state without client inference.

## Closure Records

### 2026-06-21 B6-G1 / B6-G2 / B6-G3 / B6-G4 Verification Closure

- Closed gates by verification: `B6-G1`, `B6-G2`, `B6-G3`, `B6-G4`.
- Code change: initial verification did not require core result/coverage changes.
- Closure reason:
  - The audit did not identify a focused code defect in accepted/rejected result ownership, coverage ledger movement, artifact linkage, or readmodel projection.
  - Existing tests cover rejected result boundaries, accepted slice/final result paths, coverage work item acceptance/rejection, artifact/result idempotency, and projection of coverage/result/artifact state.
  - Recent Batch 3-5 code changes did not weaken these paths; full repository tests passed after Batch 5.
- Verified invariants:
  - Rejected diagnostics are persisted as rejected/superseded result state and are not counted as accepted completion evidence.
  - Accepted completion still requires accepted result state, satisfied DoD assessment, evidence refs, and coverage work item readiness.
  - Coverage work items require matching accepted evidence and preserve rejected lineage without demoting already accepted evidence.
  - Projection exposes coverage/result/artifact/continuation state through readmodel fields, without requiring TUI/Web to infer final closure from raw workspace files.
- Tests run:
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_(RecordsRejectedResult|RejectedResult|DispatchNextClosesRunningRunWithRejectedResult|DuplicateCompletedRuntimeResultDoesNotRepeatResultArtifacts|CompleteRunRequiresAcceptedResultForDoDGatedRun|ResultArtifacts)|TestCoverageWorkItems(AcceptanceGate|Advance|Reject|Record|Governance|Assessment)|TestResultSubsystemRecordsAcceptedSliceResultWithoutFinalCompletion' -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'TestRunSnapshotFromModelsProjectsBlockingDecisionStepAndCoverage|TestRunSnapshotFromModelsProjectsContractViolationWithoutRuntimeProgress|TestWorkspaceSnapshotWithRunSnapshotMergesRunProjection|TestWorkspaceSnapshotFromModelsProjectsArtifactContinuations|TestProjectWorkspaceSnapshotDetailCompactsContextAndRunDerivedCollections' -count=1`
  - `go test ./... -count=1`
- Remaining risk:
  - Live workspace closure still needs Batch 7 regression/live verification after Batch 3-6 gates are combined.
- Next gate:
  - Continue with Batch 7 regression/live verification, then reconcile the convergence closure plan status across Batch 1-7.

### 2026-06-22 B6-G3 / B6-G4 Workspace Result Artifact Projection Closure

- Closed gates: `B6-G3`, `B6-G4`.
- Closed deviation: `D-ARTIFACT-SNAPSHOT-INCOMPLETE` projection clarity portion.
- Implementation refs:
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_result_artifact_merge.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_run_merge.go`
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_test.go`
- Actual change:
  - Workspace snapshot merge now carries active run `Results` and `Artifacts` into the workspace ViewModel, with run snapshot entries overriding stale workspace-base entries by ID.
  - Primary `Result` selection remains based on formal run-scoped results; `Artifacts` retain `resultId`, `state`, `category`, and `evidenceRefs`.
  - This keeps final closure evidence on structured result/artifact linkage instead of raw workspace file/git state.
- Tests run:
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'TestWorkspaceSnapshotWithRunSnapshotMergesRunProjection|TestRunSnapshotProjectsRejectedExecutionGraphAsTaskPlan|TestRunSnapshotProjectsContractRetryWithoutNewGraphAsPlanRevision' -count=1`
- Boundary result:
  - The change is projection/ViewModel plumbing only.
  - No branch depends on workspace template ID, Soul ID, live workspace ID, provider, programming language, filenames, or business-specific text.

### 2026-06-22 B6-G1 Result Event Naming Clarity Closure

- Closed gate: `B6-G1`.
- Closed follow-up: internal result event publisher naming no longer implies that rejected/superseded results are accepted.
- Implementation refs:
  - `internal/modules/kernel/orchestrator/biz/runtime_events.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_result_artifacts.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_result_recording.go`
- Actual change:
  - Renamed internal `publishResultAccepted` helper and interface method to `publishResultEvent`.
  - Kept existing state-based topic selection via `resultEventTopic`, so rejected results continue to publish `/result/rejected`, superseded results publish `/result/superseded`, and accepted results publish `/result/accepted`.
  - No payload, repository state, or event topic compatibility behavior changed.
- Boundary result:
  - The change is core event naming clarity only and remains business-neutral.
  - No branch depends on workspace template ID, Soul ID, live workspace ID, provider, programming language, filenames, or business-specific text.

## Batch 6 Decision

Batch 6 is audit-complete for the current evidence packet.

The audited result is:

- Core result/evidence mechanics already keep accepted result, rejected diagnostic result, slice result, artifact, and memory boundaries distinct.
- Coverage ledger strictness is correct and should not be weakened to accelerate live workspace closure.
- The live blockers remain upstream in runtime convergence and external verification, not in Batch 6 result/artifact ownership.
- Batch 6 projection clarity around artifact completeness is now code-closed for the current evidence packet: workspace views carry accepted result/evidence/artifact linkage and do not require raw file/git inference.

Future Batch 6 code closure should reopen only if a focused test proves that projection cannot distinguish accepted result-linked artifacts from unaccepted workspace file state, or that rejected diagnostics are being displayed/counted as accepted evidence.
