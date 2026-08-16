# S7.6 Contract Violation Recovery Projection

Status: completed

Type: roadmap-slice

Owner: workorchd appservice

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [MVVP Workspace ViewModel Architecture](../superseded/mvvp-workspace-viewmodel-architecture.md)
- [S7.4 Contract Retry Inherits Stage Contract](./goal-dod-nextaction-s7-4-contract-retry-inherits-stage-contract.md)
- [S7.5 Contract Retry Budget And Signature Escalation](./goal-dod-nextaction-s7-5-contract-retry-budget-signature-escalation.md)

## Contract Audit

- Scope: read model projection, port snapshot/status DTOs, remote backend decoding and TUI-visible runtime execution/task plan projection for contract violation recovery.
- Observed state: `RuntimeExecutionState` already exposes `ContractViolation`; `MapRuntimeExecutionState` reads it from retry input `contractViolationRecovery`, failed step output `contractViolation`, and runtime checkpoints. Task plan projection builds a `contract_violation_retry` revision from retry input. Backend event conversion treats contract violation recorded/retry/escalated events as projection changes, and remote decode preserves runtime execution contract violation.
- Expected state: when contract violation recovery is active or exhausted, workorchd projection should expose the violation/recovery state so TUI/Web can display it from ViewModel data without local inference.
- Deviation: no new code gap found in the current implementation. The remaining gap is documentation and focused verification that the existing projection chain covers S7.6.
- Owner: workorchd appservice, with port/backend display adapters consuming the projection.
- Event path: orchestrator contract violation event -> workspace projection changed event -> readmodel snapshot/status -> `runtimeExecution.contractViolation` and retry task plan -> backend/TUI display data.
- Ledger impact: projection only displays rejected diagnostic/retry state; it does not mark evidence accepted or mutate coverage.
- Stage Packet impact: none; this slice consumes S7.4/S7.5 outputs.
- Contract gap: S7.6 needed an explicit slice record and regression command set; existing code already maps the structured data.
- Proposed change: no behavior change unless verification fails. Record the projection chain and run focused tests.
- Boundary check: projection consumes structured runtime contract fields only. It does not parse template ID, Soul ID, workspace title, provider, command output, programming language or business text.
- Verification: readmodel projection tests, backend event/decode tests, TUI contract violation view tests, and boundary search.

## Slice Scope

In scope:

- Verify `RuntimeExecutionState.ContractViolation`.
- Verify retry task plan projection uses `contractViolationRecovery`.
- Verify contract violation events trigger projection refresh.
- Verify remote/backend/TUI decode/display paths preserve structured violation data.

Out of scope:

- New TUI layout work.
- Web visual changes.
- Retry budget logic.
- StagePacket rendering.

## Acceptance

- Run snapshot projects contract violation without requiring runtime progress.
- Retry without new graph appears as a retry plan revision with lineage and violated rule.
- Backend converts contract violation recorded/retry/escalated events into projection refresh events.
- Remote snapshot decode preserves `runtimeExecution.contractViolation`.
- No endpoint or client infers contract violation state from prose.

## Verification

- Passed: `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'TestRunSnapshotFromModelsProjectsContractViolationWithoutRuntimeProgress|TestRunSnapshotProjectsContractRetryWithoutNewGraphAsPlanRevision' -count=1`
- Passed: `go test ./internal/services/workorch/biz/backends -run 'TestDecodeSnapshotInfoPreservesRuntimeExecutionContractViolation|TestConvertEnvelopeTreatsContractViolationAsProjectionChange' -count=1`
- Passed: `go test ./internal/services/workorch/service/tui -run 'ContractViolation|Projection' -count=1`
- Passed: `git diff --check`
- Passed: diff-only boundary search found no added core business/template/language/command special casing:
  `git diff --unified=0 -- docs/70-roadmap/active/goal-dod-nextaction-s7-6-contract-violation-recovery-projection.md | rg -n '^\\+.*(software-dev|weather-query|code-review|go test|npm test|city|ssh|server)'`
