# S6.3 StagePacket Target Snapshot Integrity

Status: completed

Type: roadmap-slice

Owner: core

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S6.1 StagePacket Minimal Model](./goal-dod-nextaction-s6-1-stage-packet-minimal-model.md)
- [S6.2 StagePacket Evidence Refs](./goal-dod-nextaction-s6-2-stage-packet-evidence-refs.md)

## Contract Audit

- Scope: `internal/pkg/workorchruntimecontract/stage_packet.go`, `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`.
- Observed state: target snapshot metadata exists in `repairActionPackage.targetSnapshots`, `failureTargetRepairContext`, `patchRepairContext` and related step input fields, but S6.2 StagePacket does not expose snapshot integrity.
- Expected state: each StagePacket can reconstruct whether current target snapshots are full, partial, truncated or redacted before later LLM rendering decides whether exact file output is admissible.
- Deviation: downstream prompt/context code must still inspect scattered target snapshot maps to know if a snapshot can back exact complete file output.
- Owner: core.
- Event path: existing target snapshot metadata in step input -> StagePacket builder -> `stagePacket.targetSnapshots[]` metadata -> later LLM C3/C5 rendering and budget gates.
- Contract gap: missing top-level target snapshot integrity state; S6.3 does not change patch execution, prompt wording or context budget policy.
- Proposed change: add a metadata-only `targetSnapshots` view to StagePacket, normalized with `integrityState` and `safeForExactFileWrite`.
- Boundary check: core only interprets neutral snapshot integrity flags; no template, language, command, provider or business-specific rule is introduced.
- Verification: StagePacket package tests, orchestrator tests for full/truncated/redacted snapshot metadata, LLM block regression, boundary search.

## Slice Scope

S6.3 adds StagePacket target snapshot metadata:

- `targetRef`
- `exists`
- `contentHash`
- `contentComplete`
- `safeForFileWrite`
- `safeForExactFileWrite`
- `redacted`
- `truncated`
- `isBinary`
- `size`
- `source`
- `contextSource`
- `error`
- `integrityState`: `full`, `partial`, `truncated` or `redacted`

The StagePacket metadata view does not carry snapshot `content` or `contentPreview`. Exact patch/file-write admission remains owned by existing runtime and LLM context gates.

## Acceptance

- `StagePacket` serializes target snapshot metadata with integrity state.
- Truncated snapshots are marked `integrityState=truncated` and `safeForExactFileWrite=false`.
- Redacted snapshots are marked `integrityState=redacted` and `safeForExactFileWrite=false`.
- Full snapshots are marked only when complete, safe for file write, non-redacted, non-truncated and non-binary.
- No repository or filesystem scan is added for StagePacket construction.

## Verification

- `go test ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- `go test ./internal/modules/blocks/llm/biz -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/pkg/workorchruntimecontract internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
