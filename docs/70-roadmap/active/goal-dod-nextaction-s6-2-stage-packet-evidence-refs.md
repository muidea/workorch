# S6.2 StagePacket Evidence Refs

Status: completed

Type: roadmap-slice

Owner: core

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [S6.1 StagePacket Minimal Model](./goal-dod-nextaction-s6-1-stage-packet-minimal-model.md)

## Contract Audit

- Scope: `internal/pkg/workorchruntimecontract/stage_packet.go`, `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`.
- Observed state: S6.1 attaches `stagePacket` with scope, definition, current focus and required output, while accepted/rejected evidence refs remain embedded inside `coverageWorkItemContext`, `repairActionPackage` or active focus maps.
- Expected state: each LLM step input can reconstruct the current Stage Packet and directly see accepted/rejected evidence refs without relying on old natural-language history or a fresh evidence repository scan.
- Deviation: top-level Stage Packet lacks accepted/rejected evidence refs, so later C3/C4/C5 rendering cannot consistently protect trace refs from context compaction.
- Owner: core.
- Event path: coverage/evidence ledger projection in run execution context -> step input coverage context/repair package -> StagePacket builder -> step input `stagePacket.acceptedEvidenceRefs` and `stagePacket.rejectedEvidenceRefs`.
- Contract gap: missing top-level trace refs in Stage Packet; this slice does not load full evidence records or render prompt sections.
- Proposed change: extend StagePacket with accepted/rejected evidence refs and derive them from already-present structured input sources only.
- Boundary check: core only copies neutral evidence IDs/refs; no template, language, command, provider or business-specific interpretation is introduced.
- Verification: StagePacket package tests, orchestrator tests for initial/follow-up evidence refs, LLM block regression, boundary search.

## Slice Scope

S6.2 adds only top-level evidence refs:

- `acceptedEvidenceRefs`
- `rejectedEvidenceRefs`

The builder reads refs from current structured input in priority order:

- active focus / coverage work item context
- current focus items
- repair action package
- ledger summary

This slice intentionally does not:

- fetch full evidence records from repository;
- render C0/C1/C2/C3/C4/C5 prompt sections;
- add target snapshot integrity;
- change context budget behavior.

## Acceptance

- `StagePacket` serializes accepted/rejected evidence refs as top-level JSON fields.
- Initial step input preserves accepted/rejected evidence refs when available in current action package.
- Follow-up step input rebuilds evidence refs after `nextInput` changes current focus.
- No additional evidence repository scan is introduced for StagePacket construction.

## Verification

- `go test ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- `go test ./internal/modules/blocks/llm/biz -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/pkg/workorchruntimecontract internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
