# S4.2 Definition Revision Coverage Ledger Initialization

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: coverage ledger initialization from the current workspace definition revision.
- Observed state: `coverageSubsystem.ensure` builds coverage work items from the run execution context and persists the projection through `CoverageWorkItemRepository`. `coverageDefinitionRevision` reads explicit `definitionRevision` / goal/dod revision fields before deriving a stable fallback hash.
- Expected state: every initialized coverage item must bind to the active definition revision, and each current DoD leaf source item must have an active coverage work item before runtime asks LLM to proceed.
- Deviation: existing implementation was present, but the active roadmap slice did not have an explicit audit document and the neutral baseline test did not assert definition revision or DoD source binding.
- Owner: core orchestrator coverage subsystem.
- Event path: run execution context definition -> `coverageSubsystem.ensure` -> `buildCoverageWorkItemsForRun` -> coverage work items/context -> optional repository projection sync.
- Contract gap: missing explicit S4.2 verification that initialized items carry definition revision and DoD leaf-bound active coverage entries.
- Proposed change: document the existing canonical path and strengthen the neutral baseline test to assert revision propagation and active DoD item creation.
- Boundary check: implementation and test only use structured `definitionRevision`, `goal.id`, `dod.id`, and coverage source fields. No template, Soul, language, command, provider, file name, or business-specific branch is introduced.
- Verification: targeted coverage tests and kernel package tests.

## Design

The S4.2 initialization path is already implemented by the coverage subsystem:

1. `coverageSubsystem.ensure` creates or refreshes `run.ExecutionContext["coverageWorkItems"]`.
2. `buildCoverageWorkItemsForRun` derives source items from structured Goal/DoD/profile contract fields.
3. `coverageDefinitionRevision` binds every generated item to the explicit definition revision when present, otherwise to a deterministic hash of definition-relevant structured data.
4. `coverageWorkItemContextFromItems` builds the current focus and ledger summary from the same items.
5. `syncRepositoryProjection` persists the owner projection to `CoverageWorkItemRepository` when the repository is available.

No second coverage model is introduced. S4.2 continues to use the S4.1 canonical `CoverageWorkItem` model.

## Acceptance

- Coverage initialization propagates the active definition revision to every coverage item.
- Structured DoD leaf items generate active coverage items; container-only DoD sources remain excluded from the ledger.
- The generated coverage context and focus are built from the same ledger items.
- Repository-backed coverage sync remains unchanged and keeps run-scoped IDs.

## Verification

Executed on 2026-06-24:

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestEnsureRunCoverageWorkItemsBuildsNeutralBaseline|TestCoverageWorkItemsSyncProjectionToRepository|TestCoverageWorkItemsTrackEvidenceByKind' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./internal/modules/kernel/... -count=1`

Boundary result: the slice verifies structured definition-to-ledger propagation only. It does not add template-specific policy or a second coverage state owner.
