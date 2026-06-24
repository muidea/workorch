# S4.3 Accepted Evidence Coverage Update

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: core coverage ledger evidence update rules.
- Observed state: `coverageApplyWorkItemEvidence` already separates accepted and rejected evidence into `acceptedEvidenceByKind` / `acceptedEvidenceRefs` and `rejectedEvidenceByKind` / `rejectedEvidenceRefs`. Derived missing evidence is computed only from accepted evidence.
- Expected state: accepted evidence can satisfy required evidence kinds and move a coverage item toward `evidence_ready` or `satisfied`; rejected evidence remains diagnostic/provenance and must not satisfy coverage.
- Deviation: the implementation path existed, but S4.3 did not have a dedicated contract audit and direct tests proving rejected evidence cannot close `missingEvidenceKinds`.
- Owner: core orchestrator coverage subsystem.
- Event path: completed/failed step or assessment -> `coverageWorkItemUpdate` -> `coverageApplyWorkItemEvidence` -> coverage work item status/missing evidence -> context/repository projection.
- Contract gap: missing explicit S4.3 verification for accepted-vs-rejected coverage update semantics.
- Proposed change: add direct tests for accepted evidence satisfaction and rejected evidence non-satisfaction, and document the canonical update contract.
- Boundary check: tests and contract use only structured evidence kind, evidence refs, status, missing evidence and rejection reason. No template, Soul, language, command, provider, file name, or business-specific branch is introduced.
- Verification: targeted coverage evidence tests, orchestrator/biz package tests, and kernel package tests.

## Design

S4.3 is implemented by the existing coverage evidence update path:

1. Successful runtime evidence calls `coverageApplyWorkItemEvidence(..., accepted=true, ...)`.
2. Failed or rejected runtime evidence calls `coverageApplyWorkItemEvidence(..., accepted=false, ...)`.
3. Accepted evidence is stored under `acceptedEvidenceByKind` and flattened into `acceptedEvidenceRefs`.
4. Rejected evidence is stored under `rejectedEvidenceByKind` and flattened into `rejectedEvidenceRefs`.
5. `coverageMissingEvidenceKinds` and derived coverage readiness count only accepted evidence.

Rejected evidence may change status to `failed` when no accepted evidence exists, or keep an item in progress when partial accepted evidence already exists, but it never reduces `missingEvidenceKinds`.

## Acceptance

- Accepted evidence for a required kind removes that kind from `missingEvidenceKinds`.
- Accepted evidence for all required kinds moves an open item to `evidence_ready` when assessment is still required.
- Rejected evidence for a required kind is recorded in rejected evidence fields.
- Rejected evidence does not populate accepted evidence fields.
- Rejected evidence does not remove the required kind from `missingEvidenceKinds`.

## Verification

Executed on 2026-06-24:

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestCoverageApplyWorkItemEvidence_AcceptedEvidenceSatisfiesRequiredKind|TestCoverageApplyWorkItemEvidence_RejectedEvidenceDoesNotSatisfyRequiredKind|TestCoverageApplyWorkItemEvidence_RejectionUsesRequiredEvidenceCounts' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./internal/modules/kernel/... -count=1`

Boundary result: the slice strengthens core coverage ledger tests only. It does not add template-specific policy or business-specific branching.
