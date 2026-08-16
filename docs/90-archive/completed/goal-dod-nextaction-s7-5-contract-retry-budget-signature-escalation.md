# S7.5 Contract Retry Budget And Signature Escalation

Status: completed

Type: roadmap-slice

Owner: core

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Contract Violation Recovery Loop Design](../superseded/contract-violation-recovery-loop-design.md)
- [S7.1 ContractViolation Diagnostic Model](./goal-dod-nextaction-s7-1-contract-violation-diagnostic-model.md)
- [S7.4 Contract Retry Inherits Stage Contract](./goal-dod-nextaction-s7-4-contract-retry-inherits-stage-contract.md)

## Contract Audit

- Scope: `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`, `runtime_repair.go`, step repository summaries, and contract violation retry tests.
- Observed state: contract violation retry already has per-step, per-run and repeated-signature counters, and repeated violations can switch correction mode to `forced_narrow`. Existing tests cover per-step exhaustion and forced-narrow retry, but do not independently prove run-level exhaustion or repeated-signature escalation. The available snapshot ignored rule also contains a natural-language scan over LLM output text. Step summaries do not preserve `contractViolationRecovery` or top-level `contractViolation`, so summary-based budget counters can under-count retries.
- Expected state: bounded retry must stop deterministically when per-step, per-run or repeated-signature budgets are exhausted. Repeated signatures may get one narrower retry while budget remains. Contract violation classification must use structured runtime fields, not output prose in any language.
- Deviation: S7.5 lacks explicit test coverage for two budget dimensions. The natural-language snapshot classification can turn unstable LLM prose into a core contract rule, which weakens core/template neutrality and can misclassify outputs.
- Owner: core.
- Event path: failed LLM step -> contract violation diagnostic -> `recoveryActionForFailedStep` -> budget/signature counters -> retry dispatch or exhaustion decision.
- Ledger impact: exhausted retry remains rejected diagnostic/governance evidence; it must not create accepted evidence or side effects.
- Stage Packet impact: remaining retries continue through S7.4 final StagePacket; exhausted retries do not render another LLM step.
- Contract gap: missing budget regression tests, missing structured retry fields in summary reads, and one text-driven rule branch.
- Proposed change: add focused run-level and repeated-signature exhaustion tests; preserve contract violation retry fields in step summaries; remove prose scanning from `available_snapshot_ignored` classification and require structured `contractViolationType`.
- Boundary check: the change is business-neutral. It keeps retry decisions based on structured fields, counters and policy values, not template ID, Soul ID, language, file type, provider, command output or free-form LLM text.
- Verification: focused contract violation budget tests, existing available snapshot tests adjusted to structured input, orchestrator package tests, `git diff --check`, and diff-only boundary search.

## Slice Scope

In scope:

- Per-step retry budget remains enforced.
- Per-run retry budget has explicit regression coverage.
- Repeated signature budget has explicit regression coverage.
- Repeated signature before exhaustion still produces `forced_narrow` correction.
- Available snapshot ignored classification uses structured violation type only.

Out of scope:

- Projection display of recovery/exhaustion state.
- Provider retry policy.
- TUI/Web rendering.
- Template/Soul strategy.

## Acceptance

- Contract violation retry returns retry while all budgets remain available.
- Exhausted per-step, per-run or repeated-signature budget returns `contract_violation_exhausted`.
- Exhaustion context still includes attempts, run attempts and signature repeat count.
- No core classifier scans LLM natural-language output to infer `available_snapshot_ignored`.
- Tests prove natural-language snapshot claims alone do not trigger that rule.

## Verification

- Passed: `go test ./internal/modules/kernel/orchestrator/biz -run 'TestContractViolationRecoveryAction|TestContractViolationRetry|TestContractViolationClassifiesAvailableSnapshotIgnored|TestContractViolationDoesNotClassify' -count=1`
- Passed: `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Passed: `go test ./internal/pkg/repository/... -count=1`
- Passed: `git diff --check`
- Passed: diff-only boundary search found no added core business/template/language/command special casing:
  `git diff --unified=0 -- internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go internal/modules/kernel/orchestrator/biz/runtime_repair_input.go internal/pkg/repository/memory/memory.go internal/pkg/repository/file/file.go | rg -n '^\\+.*(software-dev|weather-query|code-review|go test|npm test|city|ssh|server)'`
- Passed: diff-only natural-language classifier search found no added snapshot prose trigger:
  `git diff --unified=0 -- internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go | rg -n '^\\+.*(缺少|缺失|未提供|需要读取|snapshot is missing|must read|not provided|cannot proceed)'`
