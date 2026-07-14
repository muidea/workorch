# S3.2 State Change Accepted Evidence Write

Status: completed

Type: implementation-slice

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: `internal/modules/kernel/orchestrator/biz/runtime_patches.go` patch apply success path.
- Observed state: an applied patch updates `Patch.State=applied`, records before/after hash, annotates tool output with `evidenceRef=patch.ID`, and publishes `patch.applied`; the new EvidenceRecord ledger has no accepted record for the admitted side effect.
- Expected state: every admitted workspace state-changing side effect must create an accepted `state_change` EvidenceRecord with workspace/run/step scope, stable source reference, and trace metadata.
- Deviation: patch evidence and output annotation exist, but coverage/NextAction cannot later rely on the EvidenceRecord ledger to distinguish accepted state changes from rejected diagnostics or plain patch metadata.
- Owner: core/orchestrator.
- Event path: LLM output `patch/apply` -> contract/admission -> workspace write succeeds -> `PatchStateApplied` -> `EvidenceRecordRepository` accepted `state_change` -> later coverage/projection consumers.
- Contract gap: accepted side effects are not persisted in the authoritative evidence ledger introduced by S3.1.
- Proposed change: after `PatchStateApplied` is durably written, create or update one deterministic accepted `state_change` EvidenceRecord whose `SourceRef` is the patch ID.
- Verification: patch apply integration test asserts EvidenceRecord scope, kind/status, sourceRef, subject, and hash/tool metadata; boundary search checks no template/language/tool-specific core rule is introduced.

## Scope

This slice only records accepted state-change evidence for successfully applied patches.

In scope:

- Generic patch-applied side effects.
- Deterministic, idempotent EvidenceRecord ID.
- File-repository-safe EvidenceRecord ID while preserving the original patch ID in `SourceRef`.
- Patch metadata sufficient for traceability.

Out of scope:

- Validation accepted/rejected evidence.
- Decision or approval governance evidence.
- Diagnostic rejected evidence.
- Coverage ledger updates from accepted evidence.
- Template-specific evidence policy.

## Design

An EvidenceRecord is created only after all of the following are true:

1. Runtime patch output passed existing admission and patch request parsing.
2. Workspace write succeeded.
3. Before/after target hash changed.
4. Patch repository update to `PatchStateApplied` succeeded.

The record uses:

- `Kind=state_change`
- `Status=accepted`
- `SourceType=patch`
- `SourceRef=<patch.ID>`
- `SubjectType=workspace_target`
- `SubjectID=<patch.TargetRef>`
- `DefinitionRevision=<run.DefinitionRevision>`

The EvidenceRecord ID is deterministic from the patch ID and sanitized for file repositories. Re-creating the same record updates the existing ledger entry instead of creating duplicates.

## Boundary Check

This is core-neutral because the implementation only observes generic patch application state. It does not inspect template ID, Soul ID, programming language, command output, target filename semantics, or any workspace business goal text.

## Acceptance

- A successful patch apply creates one accepted `state_change` EvidenceRecord.
- Failed/no-op patch application does not create accepted evidence.
- The record is queryable by run and by sourceRef.
- Existing patch apply behavior and events remain unchanged.
