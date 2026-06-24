# S3.1 EvidenceRecord Model And Repository

Status: implemented

Type: contract-audit

Roadmap: `P3 Evidence Ledger / S3.1`

## Contract Audit

- Scope: core/repository EvidenceRecord minimal persistence and query model.
- Observed state: accepted/rejected evidence refs are currently distributed across result, artifact, validation result, runtime checkpoint, memory trace, coverage work item and read model projection fields.
- Expected state: runtime can persist a single authority record for evidence with workspace/run/step scope, kind, status and sourceRef before coverage and next action consume it.
- Deviation: existing evidence refs do not have a common status owner, so accepted evidence, rejected diagnostics and governance evidence cannot be queried uniformly.
- Owner: core/repository.
- Event path: future runtime output or governance resolution -> evidence recorder -> EvidenceRecordRepository -> coverage ledger and projection.
- Contract gap: no minimal persistent EvidenceRecord model exists; S3.2-S3.6 cannot write accepted/rejected evidence without coupling directly to result/artifact/memory/checkpoint stores.
- Proposed change: add neutral `EvidenceRecord` schema, repository interface, memory repository, file repository and workspace-scoped file repository wiring.
- Boundary check: model fields are generic (`kind`, `status`, `sourceRef`, `subject`, `metadata`) and contain no template, language, command or business-scene branches.
- Verification: repository contract tests cover memory/file create/get/list/update/delete, clone isolation and bundle reload.

## Model

`EvidenceRecord` is a scoped runtime ledger item:

- `workspaceId`, `runId`, optional `stepId`
- `kind`: `state_change`, `validation`, `assessment`, `artifact`, `decision`, `approval`, `diagnostic`
- `status`: `accepted`, `rejected`, `superseded`
- `sourceType`, `sourceRef`
- optional `subjectType`, `subjectId`
- optional `coverageItemRefs`
- optional `supersedes`
- timestamps and metadata

## Repository Contract

The repository supports:

- `Create`
- `Get`
- `ListByRun`
- `ListByStep`
- `ListBySourceRef`
- `Update`
- `Delete`

All writes require `id`, `workspaceId`, `runId`, `kind`, `status` and `sourceRef`.

## Non-Goals

S3.1 does not write runtime evidence from orchestrator paths. That belongs to:

- S3.2 state_change accepted evidence
- S3.3 validation accepted/rejected evidence
- S3.4 decision/approval governance evidence
- S3.5 diagnostic rejected evidence
- S3.6 rejected evidence side-effect blocking

