# S3.4 Decision And Approval Governance Evidence Write

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: kernel decision/approval owner services and framework/bootstrap dependency wiring.
- Observed state: decision tickets, decision resolutions, and approvals are persisted by their owner modules, but the EvidenceRecord ledger has no governance evidence for accepted decision resolutions or approval outcomes.
- Expected state: every accepted governance resolution must be traceable through EvidenceRecord with workspace/run/step scope, governance kind, sourceRef, subject, and sanitized metadata.
- Deviation: coverage/NextAction cannot later distinguish governance evidence from plain owner state or UI projection.
- Owner: core/workorchd. Kernel decision/approval modules own the authoritative governance state; workorchd/bootstrap provides the shared EvidenceRecord repository.
- Event path: TUI/Web/REST/EventHub command -> decision/approval owner -> repository state transition -> EvidenceRecordRepository governance record -> owner event/projection.
- Contract gap: governance resolution state is durable, but accepted/rejected governance evidence is not mirrored into the evidence ledger.
- Proposed change: after a decision resolution or approval resolution is durably accepted by its owner, create or update one deterministic EvidenceRecord.
- Boundary check: implementation only consumes structured decision/approval model fields. It does not inspect template IDs, Soul IDs, programming languages, filenames, commands, provider names, or business goal text.
- Verification: owner service tests assert decision, approved approval, and rejected approval evidence records; boundary search verifies no business-specific core logic is introduced.

## Scope

In scope:

- Decision ticket resolved -> accepted `decision` EvidenceRecord.
- Approval approved -> accepted `approval` EvidenceRecord.
- Approval rejected -> rejected `approval` EvidenceRecord.
- Idempotent create/update behavior for repeated ensure commands.
- Framework/bootstrap/plugin wiring so owner modules receive the shared EvidenceRecord repository.

Out of scope:

- Pending ticket/approval creation evidence.
- Coverage ledger updates from governance evidence.
- TUI/Web projection changes.
- Diagnostic rejected evidence for LLM contract violations.

## Design

Decision resolution record:

- `Kind=decision`
- `Status=accepted`
- `SourceType=decision_resolution`
- `SourceRef=<resolution.ID>`
- `SubjectType=decision_ticket`
- `SubjectID=<ticket.ID>`

Approval resolution record:

- `Kind=approval`
- `Status=accepted` for approved approvals
- `Status=rejected` for rejected approvals
- `SourceType=approval`
- `SourceRef=<approval.ID>`
- `SubjectType=approval`
- `SubjectID=<approval.ID>`

EvidenceRecord IDs are deterministic and file-repository-safe. If a record already exists, the owner updates it in place and preserves the original `CreatedAt`.

## Acceptance

- Decision resolution writes accepted governance evidence.
- Approval approval writes accepted governance evidence.
- Approval rejection writes rejected governance evidence.
- Repeated ensure commands do not create duplicate evidence records.
- Existing decision/approval state transitions and events remain unchanged.

## Verification

- `go test ./internal/modules/blocks/decision/biz ./internal/modules/blocks/approval/biz -count=1`
- `go test ./internal/modules/blocks/decision ./internal/modules/blocks/approval ./internal/modules/application/workorchd/biz/bootstrap ./internal/initiators/workorchruntime -count=1`
- `go test ./internal/pkg/repository ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/kernel/... -count=1`
