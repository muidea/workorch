# S4.1 CoverageItem Minimal Model Mapping

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: core coverage ledger model and repository contract.
- Observed state: coverage state is already represented by `workorchruntimecontract.CoverageWorkItem`, synchronized from orchestrator projection into `CoverageWorkItemRepository`, and hydrated back into run execution context.
- Expected state: S4.1 requires a minimal `CoverageItem` model that binds each item to the current definition, goal/dod source, required evidence kinds, and accepted/rejected evidence refs.
- Deviation: the roadmap used the name `CoverageItem`, while the implemented runtime model is named `CoverageWorkItem`. Without an explicit mapping, later slices could accidentally introduce a second coverage model and split owner state.
- Owner: core. The coverage ledger belongs to runtime contract, repository, and orchestrator coverage subsystem.
- Event path: definition/gap source -> `coverageSubsystem.ensure` -> `CoverageWorkItem` repository projection -> run execution context projection -> later evidence updates.
- Contract gap: documentation and contract tests did not explicitly state that `CoverageWorkItem` is the canonical S4 CoverageItem implementation.
- Proposed change: document the mapping and strengthen repository contract assertions for goal/dod binding and required evidence kinds.
- Boundary check: no template, Soul, language, command, provider, file name, or business-specific logic is introduced. The model only stores structured scope, definition, source, evidence, and status fields.
- Verification: repository contract tests cover memory/file implementations and bundle reload; kernel tests cover orchestrator coverage behavior.

## Canonical Mapping

S4 `CoverageItem` is implemented by `internal/pkg/workorchruntimecontract.CoverageWorkItem`.

Required S4.1 fields map as follows:

- item identity: `CoverageWorkItem.ID`
- workspace/run scope: `WorkspaceID`, `RunID`
- definition binding: `DefinitionRevision`
- source binding: `NormalizedSourceKey`, `SourceScope`, `SourceRequirementID`, `SourceGoalID`, `SourceDoDID`, `SourcePath`, `TargetText`
- required evidence kinds: `RequiredEvidenceKinds`
- evidence refs: `AcceptedEvidenceRefs`, `RejectedEvidenceRefs`
- state and gap: `Status`, `CurrentGap`
- update provenance: `UpdatedByStepID`, `Metadata`

The repository ID is derived as `<runID>-<contextCoverageWorkItemId>` by the orchestrator projection sync. The original context coverage item ID remains in metadata as `contextCoverageWorkItemId`, so UI/context projection can keep stable compact IDs without losing repository identity.

## Scope

In scope:

- Treat `CoverageWorkItem` as the canonical minimal CoverageItem model.
- Preserve memory/file repository parity.
- Verify goal/dod binding fields survive create/get/reload.
- Keep run execution context projection compatible with the repository model.

Out of scope:

- Coverage ledger initialization from definition revision. Covered by S4.2.
- Evidence-driven coverage updates. Covered by S4.3.
- Missing evidence kind calculation. Covered by S4.4.
- Completion gate reading the ledger. Covered by S4.5.
- Projection summary. Covered by S4.6.

## Acceptance

- `CoverageWorkItem` has workspace/run scope and definition revision.
- `CoverageWorkItem` binds to source goal/dod fields when available.
- `CoverageWorkItem` stores `RequiredEvidenceKinds`.
- `CoverageWorkItem` stores accepted/rejected evidence refs.
- Memory and file repositories preserve the fields through create/get/list/reload paths.

## Verification

Executed on 2026-06-24:

- `go test ./internal/pkg/repository -run 'TestCoverageWorkItemRepositoryContract_Memory|TestCoverageWorkItemRepositoryContract_File|TestRepositoryBundleContract_File' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestEnsureRunCoverageWorkItemsBuildsNeutralBaseline|TestCoverageWorkItemsSyncProjectionToRepository|TestCoverageWorkItemsTrackEvidenceByKind' -count=1`

Boundary result: the slice only records a canonical model mapping and strengthens structured repository assertions. It does not add a second coverage model or business-specific branching.
