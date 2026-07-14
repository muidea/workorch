# S7.4 Contract Retry Inherits Stage Contract

Status: completed

Type: roadmap-slice

Owner: core

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [Contract Violation Recovery Loop Design](../../30-module-architecture/contract-violation-recovery-loop-design.md)
- [S6.1 StagePacket Minimal Model](./goal-dod-nextaction-s6-1-stage-packet-minimal-model.md)
- [S6.2 StagePacket Evidence Refs](./goal-dod-nextaction-s6-2-stage-packet-evidence-refs.md)
- [S7.3 Contract Gate Blocks Side Effects](./goal-dod-nextaction-s7-3-contract-gate-blocks-side-effects.md)

## Contract Audit

- Scope: `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`, contract violation retry input and StagePacket rendering.
- Observed state: `runtimeRepairFollowUpStepInput` builds repair/retry input from run execution context and failed step input, applies missing coverage, repair action package, contract violation recovery, convergence and output mode selection, then returns the mutated input directly. Initial and normal follow-up inputs rebuild `stagePacket`, but runtime repair follow-up does not rebuild it after final mutations.
- Expected state: contract violation retry must inherit the same effective runtime stage, required output, definition revision, coverage/gap context and rejected diagnostic refs in the final LLM-visible StagePacket. Retry may add C4 recovery diagnostics, but it must not lose or replace the current C1/C3 contract.
- Deviation: the top-level retry input can contain inherited fields, but `stagePacket` may be absent or stale, so downstream prompt rendering cannot rely on a single current packet for `definition`, `activeFocus` and `requiredOutput`.
- Owner: core.
- Event path: failed LLM step -> rejected diagnostic evidence -> `runtimeRepairFollowUpStepInput` -> contract violation recovery projection -> convergence/output mode mutation -> final StagePacket -> LLM C3/C4 retry input.
- Contract gap: runtime repair follow-up does not finalize StagePacket after contract retry mutation, leaving retry inheritance implicit and harder to verify.
- Proposed change: finalize runtime repair follow-up input by rebuilding StagePacket after all repair/convergence mutations, and add a contract violation retry test that verifies definition revision, runtime stage, requiredOutput and rejected refs are preserved.
- Boundary check: the change is business-neutral. It only reuses structured runtime fields and does not inspect template ID, Soul ID, workspace title, programming language, command output, provider name or business text.
- Verification: focused contract violation retry test, stage packet tests, orchestrator package targeted test, `git diff --check`, and boundary search.

## Slice Scope

S7.4 only covers retry input inheritance and final StagePacket consistency.

In scope:

- Rebuild StagePacket for runtime repair follow-up after all input mutations.
- Ensure contract violation retry carries `contractViolationRecovery`.
- Ensure StagePacket carries current definition revision, runtime stage, active focus, required output and rejected evidence refs.

Out of scope:

- Retry budget and repeated signature escalation.
- Projection rendering of recovery state.
- Provider retry policy.
- Template/Soul prompt strategy.

## Acceptance

- Contract violation retry input contains `contractViolationRecovery`.
- StagePacket definition revision matches the current run/coverage definition revision.
- StagePacket active stage matches the final retry runtime stage.
- StagePacket requiredOutput matches the final repairActionPackage required output.
- StagePacket rejected evidence refs include the current rejected diagnostic refs.
- The change does not add business, template, language, command, provider or scenario-specific logic to core.

## Verification

- Passed: `go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeRepairInputIncludesContractViolationRecovery|TestInitialStepInputAttachesMinimalStagePacket|TestFollowUpStepInputRebuildsStagePacketAfterNextInputMutation' -count=1`
- Passed: `go test ./internal/modules/kernel/orchestrator/... -run 'TestRuntimeRepairInputIncludesContractViolationRecovery|TestInitialStepInputAttachesMinimalStagePacket|TestFollowUpStepInputRebuildsStagePacketAfterNextInputMutation' -count=1`
- Passed: `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Passed: `git diff --check`
- Passed: diff-only boundary search found no added core business/template/language/command special casing:
  `git diff --unified=0 -- internal/modules/kernel/orchestrator/biz/runtime_repair_input.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go | rg -n '^\\+.*(software-dev|weather-query|code-review|go test|npm test|city|ssh|server)'`
