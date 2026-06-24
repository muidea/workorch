# S4.4 Missing Evidence Kind Calculation

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: core coverage ledger missing evidence calculation.
- Observed state: `coverageMissingEvidenceKinds` delegates to `deriveCoverageWorkItemState`, which computes missing kinds from `requiredEvidenceKinds`, `requiredEvidenceCounts`, and `acceptedEvidenceByKind`.
- Expected state: each required evidence kind is independently satisfied only by accepted evidence of the same kind. `state_change`, `validation`, and `assessment` evidence must not satisfy each other.
- Deviation: existing calculation was present, but S4.4 lacked a dedicated test that exercises mixed required kinds in one coverage item.
- Owner: core orchestrator coverage subsystem.
- Event path: coverage work item required kinds + accepted evidence by kind -> `coverageMissingEvidenceKinds` -> coverage state/current gap -> next action selection.
- Contract gap: missing explicit S4.4 verification that mixed evidence kinds stay independent.
- Proposed change: add a direct coverage evidence test that applies accepted `state_change`, `validation`, and `assessment` evidence sequentially and asserts missing kinds close one by one.
- Boundary check: the test uses only structured evidence kinds and refs. No template, Soul, language, command, provider, file name, or business-specific branch is introduced.
- Verification: targeted coverage evidence tests, orchestrator/biz package tests, and kernel package tests.

## Design

S4.4 uses the existing derived coverage state:

1. Required evidence kinds are read from `requiredEvidenceKinds`.
2. Required counts are read from `requiredEvidenceCounts`, defaulting to one per kind.
3. Accepted evidence is counted from `acceptedEvidenceByKind`.
4. Missing kinds are recomputed by comparing required counts with accepted counts per kind.
5. Rejected evidence is not counted for satisfaction.

Flat accepted refs are only used for legacy single-kind or terminal compact summaries. Mixed-kind coverage items require by-kind accepted evidence to avoid cross-kind satisfaction.

## Acceptance

- Accepted `state_change` evidence does not satisfy `validation` or `assessment`.
- Accepted `validation` evidence does not satisfy `state_change` or `assessment`.
- Accepted `assessment` evidence does not satisfy `state_change` or `validation`.
- A mixed-kind item becomes evidence-ready only after all required kinds have accepted evidence.

## Verification

Executed on 2026-06-24:

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestCoverageMissingEvidenceKindsAreIndependentByEvidenceKind|TestCoverageApplyWorkItemEvidence_AcceptedEvidenceSatisfiesRequiredKind|TestCoverageApplyWorkItemEvidence_RejectedEvidenceDoesNotSatisfyRequiredKind' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./internal/modules/kernel/... -count=1`

Boundary result: the slice strengthens structured missing-evidence tests only. It does not add template-specific policy or business-specific branching.
