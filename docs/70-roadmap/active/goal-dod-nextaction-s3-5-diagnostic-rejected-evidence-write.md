# S3.5 Diagnostic Rejected Evidence Write

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: kernel orchestrator runtime contract violation path.
- Observed state: LLM contract violations are classified, attached to failed step output, checkpointed, and published as orchestrator events, but the EvidenceRecord ledger has no rejected diagnostic evidence for the rejected LLM output.
- Expected state: every rejected runtime contract violation must be traceable as `Kind=diagnostic`, `Status=rejected`, with workspace/run/step scope, sourceRef, subject, failure classification, violated rule, recoverability, and rejected output ref.
- Deviation: later coverage/NextAction can see failed validation evidence but cannot distinguish the rejected LLM diagnostic output as a first-class ledger record.
- Owner: core. The orchestrator owns runtime contract validation and rejected LLM output classification.
- Event path: LLM step output -> runtime contract validation -> failed step output with contractViolation -> rejected diagnostic EvidenceRecord -> checkpoint/event/retry.
- Contract gap: rejected contract diagnostics are only embedded in step output/events and are not mirrored into the evidence ledger.
- Proposed change: after contract violation diagnostics are attached and before retry/event flow continues, create or update one deterministic rejected diagnostic EvidenceRecord.
- Boundary check: implementation only consumes structured runtime contract violation fields and failure metadata. It does not inspect template IDs, Soul IDs, programming languages, filenames, commands, provider names, or business goal text.
- Verification: orchestrator owner tests assert rejected diagnostic evidence is written idempotently and remains separate from result/artifact/memory writes; boundary search verifies no business-specific core logic is introduced.

## Scope

In scope:

- Runtime contract violation -> rejected `diagnostic` EvidenceRecord.
- Idempotent create/update behavior for repeated recording of the same failed step.
- Metadata for violated rule, violation type, failure class/signature, recoverability, retry hint, rejected output ref, and compact contract violation snapshot.

Out of scope:

- Coverage ledger consumption of rejected diagnostic evidence.
- Contract violation retry budget changes.
- TUI/Web projection changes.
- Provider-specific retry or cache behavior.

## Design

Diagnostic rejected record:

- `Kind=diagnostic`
- `Status=rejected`
- `SourceType=contract_violation`
- `SourceRef=<contractViolation.ID>`
- `SubjectType=runtime_step`
- `SubjectID=<failed step.ID>`

The record is written only after the owner path has classified the LLM output as a runtime contract violation. It must not execute rejected output, must not create accepted result/artifact, and must not write high-value memory.

EvidenceRecord IDs are deterministic and file-repository-safe. If a record already exists, the owner updates it in place and preserves the original `CreatedAt`.

## Acceptance

- Contract violation failure writes rejected diagnostic evidence.
- Repeated recording does not create duplicate evidence records.
- Rejected diagnostic evidence is scoped to workspace/run/step and references the rejected output.
- No result, artifact, or memory repository write is introduced by diagnostic evidence recording.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRecord(ValidationResultWritesAcceptedAndRejectedEvidenceRecords|ContractViolationWritesRejectedDiagnosticEvidenceRecord)|TestHandleRuntimeStepFailureRecordsRejectedDiagnosticEvidence' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./internal/modules/kernel/orchestrator -count=1`
- `go test ./internal/modules/kernel/... -count=1`
