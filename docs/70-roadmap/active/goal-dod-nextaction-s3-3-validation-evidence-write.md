# S3.3 Validation Accepted And Rejected Evidence Write

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go` validation result recording path.
- Observed state: runtime step completion and failure paths create `ValidationResult` records, but the EvidenceRecord ledger introduced by S3.1 has no corresponding accepted or rejected validation evidence.
- Expected state: every persisted validation result must be traceable through EvidenceRecord with workspace/run/step scope, validation kind, accepted/rejected status, sourceRef, subject, and quality metadata.
- Deviation: later coverage/NextAction stages cannot rely on a single evidence ledger to distinguish accepted validation evidence from rejected validation diagnostics.
- Owner: core/orchestrator.
- Event path: runtime step finalize/failure -> `recordValidationResult` -> `ValidationResultRepository` -> `EvidenceRecordRepository` validation accepted/rejected -> later coverage/projection consumers.
- Contract gap: validation results are stored in their legacy repository but not mirrored into the authoritative EvidenceRecord ledger.
- Proposed change: after `ValidationResult` create/update succeeds, create or update one deterministic `validation` EvidenceRecord whose `SourceRef` is the validation result ID.
- Boundary check: implementation uses only generic validation result state and quality fields; it does not inspect template IDs, Soul IDs, target filenames, programming languages, commands, or business goal text.
- Verification: unit test records passed and failed validation results and asserts accepted/rejected EvidenceRecord scope, sourceRef, subject, and metadata; boundary search checks no business-specific core logic is introduced.

## Scope

This slice only records EvidenceRecord entries for runtime `ValidationResult` persistence.

In scope:

- `ValidationResultStatePassed` -> accepted validation evidence.
- `ValidationResultStateFailed` and `ValidationResultStateWarning` -> rejected validation evidence.
- Deterministic, file-repository-safe EvidenceRecord ID.
- Trace metadata for validation kind, target, state, evidence quality, and linked evidenceRef.

Out of scope:

- Coverage ledger updates from validation EvidenceRecord.
- Completion gate changes.
- Result/artifact/memory write policy changes.
- Decision/approval governance evidence.
- Diagnostic rejected evidence for LLM contract violations.

## Design

EvidenceRecord is created only after `ValidationResultRepository.Create` or `Update` succeeds. If the EvidenceRecord already exists, it is updated in place while preserving the original `CreatedAt`.

The record uses:

- `Kind=validation`
- `Status=accepted` when validation state is `passed`
- `Status=rejected` when validation state is `failed` or `warning`
- `SourceType=validation_result`
- `SourceRef=<validationResult.ID>`
- `SubjectType=runtime_step`
- `SubjectID=<validationResult.TargetRef>`
- `DefinitionRevision=<run.DefinitionRevision>`

## Acceptance

- Passed validation creates accepted `validation` EvidenceRecord.
- Failed validation creates rejected `validation` EvidenceRecord.
- EvidenceRecord is queryable by run and sourceRef.
- Existing ValidationResult persistence behavior remains unchanged.
