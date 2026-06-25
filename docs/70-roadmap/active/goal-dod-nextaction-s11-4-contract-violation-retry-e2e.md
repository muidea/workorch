# S11.4 Contract Violation Retry E2E

Status: completed

Owner: e2e

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [Contract Violation Recovery Loop Design](../../30-module-architecture/contract-violation-recovery-loop-design.md)
- [S7.4 Contract Retry Inherits Stage Contract](./goal-dod-nextaction-s7-4-contract-retry-inherits-stage-contract.md)
- [S7.5 Contract Retry Budget And Signature Escalation](./goal-dod-nextaction-s7-5-contract-retry-budget-signature-escalation.md)
- [S7.6 Contract Violation Recovery Projection](./goal-dod-nextaction-s7-6-contract-violation-recovery-projection.md)
- [S11.1 Managed E2E Harness](./goal-dod-nextaction-s11-1-managed-e2e-harness.md)

## Contract Audit

- Scope: deployed or managed WorkOrch service handling a recoverable LLM contract violation and retrying with the same runtime contract.
- Observed state: S7.4-S7.6 already cover core retry input inheritance, retry budget/signature escalation, and projection of structured contract violation recovery. `scripts/verify-contract-violation-recovery-e2e.sh` existed and passed manually, but it was not included in `verify-e2e-full`, used a fixed mock provider port from Makefile, and did not purge its temporary workspace when run against a deployed service.
- Expected state: a recoverable LLM output violation is rejected before side effects, recorded through contract violation events, retried boundedly with `contractViolationRecovery` in the next LLM request, and completes without entering manual decision flow. The deployed e2e target should be repeatable under the managed harness and should not leak workspace state or require a globally fixed mock port.
- Deviation: the all-e2e gate did not exercise the existing contract violation recovery deployed script, so S11 full verification could pass without proving rejected diagnostic plus bounded retry in a managed service. The script harness also had avoidable runtime coupling to a static port and left cleanup to the temporary managed data root.
- Owner: e2e. This slice validates existing business-neutral core behavior and improves e2e harness repeatability. It does not change runtime state machine, template, Soul, TUI, Web, or LLM prompt rendering.
- Event path: mock LLM first returns a completed assessment that intentionally violates the evidence-ref contract -> orchestrator rejects it before side effects -> contract violation recorded and retry requested events are persisted -> retry LLM request includes `contractViolationRecovery` -> mock LLM returns a valid assessment -> run reaches completed -> snapshot/events expose the structured violation rule and no `blocked_on_decision` state.
- Ledger impact: rejected diagnostic remains rejected. It must not become accepted evidence, result, artifact, or long-term high-value memory.
- Stage Packet impact: none in this slice; S7.4 owns retry StagePacket inheritance. This e2e only proves the deployed service path receives the recovery payload and completes from a corrected output.
- Contract gap: missing S11-level managed/deployed regression that contract violation recovery works end to end and is included in the full e2e target.
- Proposed change: document S11.4, include `verify-contract-violation-recovery-e2e` in `verify-e2e-full`, expose it as a phony/help target, and make the script choose a free mock port plus purge its temporary workspace.
- Boundary check: no production core code is changed. The e2e fixture uses generic-governed and neutral contract text only; assertions consume structured events, run state, snapshot data, and mock-provider request observations.
- Verification: shell syntax check, managed harness execution for S11.4, focused Go tests for contract violation recovery, `git diff --check`, and boundary search over changed files.

## E2E Contract

- The script runs against `WORKORCH_BASE_URL` or the managed harness service URL.
- The script starts a local OpenAI-compatible mock provider and temporarily updates WorkOrch LLM configuration, restoring the previous configuration on exit.
- The script creates a temporary generic-governed workspace and purges it during cleanup.
- The first mock response intentionally violates the neutral DoD assessment evidence-ref contract.
- The runtime must reject that output before side effects, publish `contract_violation/recorded` and `contract_violation/retry_requested`, and retry with `contractViolationRecovery` in the LLM-visible request.
- The corrected mock response must complete the run without requiring decision or approval.

## Acceptance

- `verify-contract-violation-recovery-e2e` passes against the managed harness.
- `verify-e2e-full` includes `verify-contract-violation-recovery-e2e`.
- The script can run repeatedly without a hard-coded mock provider port.
- The script purges the temporary workspace after success or failure.
- Existing production contract violation recovery behavior is validated without production code changes.

## Verification

- `bash -n scripts/verify-contract-violation-recovery-e2e.sh`
- `WORKORCH_MANAGED_VERIFY_TARGET=verify-contract-violation-recovery-e2e WORKORCH_MANAGED_MAX_CONCURRENT_RUNS=1 WORKORCH_MANAGED_SKIP_PREFLIGHT=1 ./scripts/verify-workorch-managed-full.sh`
- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestContractViolation|TestRuntimeRepairInputIncludesContractViolationRecovery|TestValidateLLMExecutionOutputContractRejectsAssessmentOnlyExecutableOutputs' -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/services/workorch/biz/backends ./internal/services/workorch/service/tui -run 'ContractViolation|Projection' -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|mcp-lsp-bridge" scripts/verify-contract-violation-recovery-e2e.sh Makefile docs/70-roadmap/active/goal-dod-nextaction-s11-4-contract-violation-retry-e2e.md`
