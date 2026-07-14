# S4.5 Completion Gate Reads Coverage Ledger

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: runtime completion gate before accepted DoD result recording.
- Observed state: runtime dispatch and graph completion paths update coverage from the validated DoD assessment, then call `coverageLedger().validateReadyForAcceptance` before recording accepted completion evidence. The gate fails when coverage work items are unresolved, missing evidence, pending assessment, or satisfied without accepted evidence refs.
- Expected state: completion must read the current coverage ledger and only allow formal accepted result recording when all coverage items are satisfied, blocked, or skipped with accepted evidence trace where required.
- Deviation: existing implementation and tests were present, but S4.5 did not have a dedicated contract audit linking completion gate behavior to the canonical coverage ledger.
- Owner: core orchestrator coverage subsystem and runtime completion flow.
- Event path: LLM DoD assessment -> validation -> coverage update -> `validateReadyForAcceptance` -> accepted result/artifact/memory write or completion-gate failure.
- Contract gap: missing explicit S4.5 documentation that completion gate reads coverage work item state as the authoritative ledger before accepted output writes.
- Proposed change: document the existing canonical path and bind it to the current test coverage.
- Boundary check: the gate only reads structured coverage status, missing evidence kinds, accepted evidence refs, and item IDs. It does not inspect template IDs, Soul IDs, languages, commands, providers, file names, or business-specific text.
- Verification: existing targeted coverage gate tests, orchestrator/biz package tests, and kernel package tests.

## Design

S4.5 is implemented by the existing completion flow:

1. Runtime extracts and validates a DoD assessment.
2. `coverageLedger().updateFromAssessment` updates the coverage ledger from accepted/rejected assessment evidence.
3. `coverageLedger().validateReadyForAcceptance` checks current coverage work items.
4. If validation fails, runtime enters the completion-gate failure path and records diagnostics for repair.
5. If validation passes, formal accepted result/artifact/memory writes proceed through the result subsystem.

The accepted output write guard from S3.6 remains a separate protection: S4.5 decides whether the coverage ledger is ready; S3.6 prevents known rejected evidence refs from backing accepted outputs.

## Acceptance

- Completion gate rejects unresolved coverage items.
- Completion gate rejects items missing required evidence kinds.
- Completion gate rejects satisfied items with no accepted evidence refs.
- Completion gate allows satisfied items with accepted evidence refs.
- Accepted completion result is recorded only after coverage gate passes.

## Verification

Executed on 2026-06-24:

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestCoverageWorkItemsAcceptanceGateRequiresSatisfiedItems|TestCoverageWorkItemsAcceptanceGateAllowsSatisfiedEvidence|TestCoverageWorkItemsAssessmentClosesEvidenceReadyItemWithoutNewRefs|TestGraphCompletionGateFailurePersistsCoverageWorkItemDiagnostics' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./internal/modules/kernel/... -count=1`

Boundary result: the slice documents and verifies the structured completion gate path only. It does not add template-specific policy or business-specific branching.
