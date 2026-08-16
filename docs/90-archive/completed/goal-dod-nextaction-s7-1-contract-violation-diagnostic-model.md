# S7.1 ContractViolation Diagnostic Model

Status: completed

Type: roadmap-slice

Owner: core

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [Contract Violation Recovery Loop Design](../superseded/contract-violation-recovery-loop-design.md)

## Contract Audit

- Scope: `internal/pkg/workorchruntimecontract/contract_violation.go`, `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`.
- Observed state: WorkOrch already emits `ContractViolation` payloads and retry/escalation events, but the runtime contract package only exposes a passive struct without normalization or model-level tests. The payload has `violatedRule` and `failureSignature`, while S7.1 requires an explicit diagnostic model with rule/signature/recoverability.
- Expected state: contract violation diagnostics have a stable, normalized model that carries violation type, rule, signature and recoverability before later parse/schema gate, retry and projection slices consume it.
- Deviation: different code paths can rely on ad hoc maps or output fields, and the model package does not independently validate required diagnostic fields or recoverability values.
- Owner: core.
- Event path: failed LLM/runtime output -> contract violation classification -> normalized `ContractViolation` -> rejected diagnostic evidence/event/projection -> later bounded retry or escalation.
- Contract gap: missing normalized model contract for `violationType`, stable `signature`, and recoverability validation.
- Proposed change: extend `ContractViolation` with neutral `violationType` and `signature`, add `Normalized`, `HasScope`, `Recoverable` helpers and package tests, then populate/map the new fields from orchestrator classification.
- Boundary check: the change is business-neutral and only models runtime diagnostic metadata; no template, language, command, provider or scenario-specific rule is introduced.
- Verification: runtime contract package tests, orchestrator contract violation tests, `git diff --check`, and boundary search.

## Slice Scope

S7.1 only defines the common diagnostic model. It does not change retry budget, parse/schema gate behavior, side-effect blocking, projection rendering or escalation policy.

The minimal model fields are:

- `violationType`
- `violatedRule`
- `signature`
- `failureSignature`
- `recoverability`
- `expectedContract`
- `actualOutputSummary`
- `sourceRefs`

## Acceptance

- `ContractViolation.Normalized()` trims string fields and defaults unknown recoverability to `retry_same_contract`.
- `ContractViolation.Signature` is populated from failure signature when an explicit signature is absent.
- `ContractViolation.Recoverable()` is true only for retryable recoverability values.
- Orchestrator-produced violation maps include `violationType` and `signature`.
- No existing map payload consumers lose backward-compatible fields.

## Verification

- `go test ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -run 'TestContractViolation' -count=1`
- `go test ./internal/pkg/workorchport ./internal/pkg/workorchclient -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/pkg/workorchruntimecontract internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go docs/70-roadmap/active/goal-dod-nextaction-s7-1-contract-violation-diagnostic-model.md`
