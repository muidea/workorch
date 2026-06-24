# S6.1 StagePacket Minimal Model

Status: completed

Type: roadmap-slice

Owner: core

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [S5.1 NextAction Minimal Model](./goal-dod-nextaction-s5-1-nextaction-minimal-model.md)

## Contract Audit

- Scope: `internal/pkg/workorchruntimecontract`, `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`.
- Observed state: P5 can derive deterministic `NextAction`, and step input carries scattered `definitionRevision`, `coverageWorkItemContext`, `repairActionPackage.requiredOutput`, `runtimeStage` and scope fields.
- Expected state: each runtime step input can reconstruct a minimal Stage Packet with workspace/run scope, definition identity, current focus and required output before later P6 slices render C3/C4/C5.
- Deviation: Stage Packet is only described in design docs; no reusable contract object or step-input attachment exists, so downstream LLM context rendering still depends on scattered maps.
- Owner: core.
- Event path: run execution context + convergence contract + current step input -> StagePacket builder -> step input `stagePacket` -> later LLM context rendering.
- Contract gap: missing neutral StagePacket model and minimal reconstruction path; S6.1 does not yet inject accepted/rejected evidence refs, target snapshots, or context budget protection.
- Proposed change: add a business-neutral `StagePacket` contract type and attach a minimal packet to initial and follow-up step input using existing run scope, definition revision, current focus and requiredOutput fields.
- Boundary check: core only copies structured fields already owned by runtime; no template ID, language, command, provider or domain-specific rule is introduced.
- Verification: package tests for normalization/map rendering; orchestrator tests for initial and follow-up step inputs containing definition/currentFocus/requiredOutput; boundary search for scenario strings.

## Slice Scope

S6.1 introduces only the minimal Stage Packet structure:

- `schemaVersion`
- `workspaceId`, `runId`, optional `stepId`
- `definition.revision`, `definition.goalId`, `definition.doDId`
- `activeStage`, `activeFocus`
- optional `nextAction`
- `requiredOutput`
- `allowedActions`, `forbiddenActions`, `requiredEvidenceKinds`
- `idempotencyKey`

Evidence refs, target snapshot integrity, compact summaries, memory refs and process evidence timeline remain S6.2-S6.6.

## Acceptance

- The runtime contract package exposes a reusable `StagePacket` model.
- Initial LLM step input includes `stagePacket.definition`, `stagePacket.activeFocus` and `stagePacket.requiredOutput` when those source fields exist.
- Follow-up step input rebuilds `stagePacket` after inherited `nextInput` mutates the current focus or required output.
- The packet is JSON-map renderable for current map-based step input and later LLM rendering.

## Verification

- `go test ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/pkg/workorchruntimecontract internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
