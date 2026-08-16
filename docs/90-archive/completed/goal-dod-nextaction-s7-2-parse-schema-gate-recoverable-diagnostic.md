# S7.2 Parse/Schema Gate Recoverable Diagnostic

Status: completed

Type: roadmap-slice

Owner: core/llm

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [Contract Violation Recovery Loop Design](../superseded/contract-violation-recovery-loop-design.md)
- [S7.1 ContractViolation Diagnostic Model](./goal-dod-nextaction-s7-1-contract-violation-diagnostic-model.md)

## Contract Audit

- Scope: `internal/modules/blocks/llm/biz/runtime_adapter.go`, `internal/modules/kernel/orchestrator/biz/runtime_repair.go`, `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`.
- Observed state: LLM block already performs one provider repair attempt when `ParseLLMExecutionDecision` fails. If repair also fails, the result is `stage=llm_schema_invalid` with parse/repair previews. Orchestrator can classify the step as `llm_decision_invalid` and later derive `violatedRule=unparseable_output`, but the parse/schema gate does not consistently publish a stable `contractViolationType` and recoverable metadata at the failure output boundary.
- Expected state: terminal parse/schema gate failures are accepted only as rejected diagnostic evidence, carry a stable neutral violation type, resolve to `unparseable_output`, and are marked `retry_same_contract` so the next retry can inherit the same stage packet and required output contract.
- Deviation: downstream recovery and projection can still depend on stage/error text to infer that an LLM output was unparseable. The diagnostic summary does not explicitly preserve parse/repair error fields as current recovery material.
- Owner: core/llm.
- Event path: LLM provider response -> `ParseLLMExecutionDecision`/repair gate -> failed LLM step output -> orchestrator contract violation diagnostic -> rejected diagnostic evidence -> bounded retry with `contractViolationRecovery`.
- Contract gap: missing explicit parse/schema violation type and recoverability at the gate output, plus incomplete current diagnostic summary for parse/repair errors.
- Proposed change: define a neutral parse/schema contract violation type, attach it to terminal LLM schema failures, map it to `unparseable_output`, preserve parse/repair errors in `actualOutputSummary`, and add focused tests for LLM block output and orchestrator recovery diagnostics.
- Boundary check: the change is core business-neutral. It classifies generic JSON/schema contract failures and does not inspect template ID, Soul ID, programming language, command output, provider name, workspace title or target business text.
- Verification: focused LLM adapter test, orchestrator contract violation test, runtime contract package test, `git diff --check`, and boundary search over changed core files.

## Slice Scope

S7.2 only covers terminal parse/schema gate failures after the LLM block's single repair attempt fails.

In scope:

- Stable parse/schema violation type.
- `unparseable_output` rule mapping.
- `retry_same_contract` recoverability.
- Parse and repair error diagnostic summary.
- Tests proving the rejected diagnostic is recoverable.

Out of scope:

- Retry budget and repeated signature escalation.
- Side-effect admission gate behavior.
- TUI/Web projection rendering.
- Provider retry or provider transport failures.
- Template/Soul prompt strategy.

## Acceptance

- LLM block terminal parse/schema failures include `contractViolationType`, `contractViolationRule`, and `contractViolationRecoverability`.
- Orchestrator maps parse/schema failures to `violatedRule=unparseable_output` even when the input step only has stage/error diagnostics.
- `ContractViolation` for parse/schema failures is recoverable and uses `retry_same_contract`.
- `actualOutputSummary` preserves bounded `parseError` and `repairError` diagnostics.
- The change does not add business, template, language, command, provider or scenario-specific logic to core.

## Verification

- `go test ./internal/modules/blocks/llm/biz -run 'TestRuntimeAdapterReportsRepairFailure' -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -run 'TestContractViolation.*LLMSchema|TestContractViolationCorrectionProfilesCoverPrimaryRules' -count=1`
- `go test ./internal/pkg/workorchruntimecontract ./internal/modules/kernel/orchestrator/biz ./internal/modules/blocks/llm/biz -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/modules/blocks/llm/biz/runtime_adapter.go internal/modules/kernel/orchestrator/biz/runtime_repair.go internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go docs/70-roadmap/active/goal-dod-nextaction-s7-2-parse-schema-gate-recoverable-diagnostic.md`
