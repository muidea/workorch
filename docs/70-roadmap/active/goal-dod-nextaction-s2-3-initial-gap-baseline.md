# Goal/DoD NextAction S2.3 Initial Gap Baseline

Status: active

Type: contract-audit

Last Updated: 2026-06-23

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S2.1 Workspace Definition Revision Model](./goal-dod-nextaction-s2-1-definition-revision-model.md)
- [S2.2 Current Definition Revision Binding](./goal-dod-nextaction-s2-2-current-definition-revision-binding.md)

## Contract Audit

- Scope: first dispatch gap baseline with no historical accepted/rejected evidence.
- Observed state: `RuntimeOrchestrator.ensureGapBaselineBeforeDispatch` already runs before runtime dispatch specs/events. It ensures coverage ledger state, builds `gapBaseline` from `coverageWorkItemContext`, stores open work items, todo items, accepted/rejected evidence refs when present, and persists the run execution context before dispatch.
- Expected state: the first run dispatch must have a `gapBaseline` even when no evidence exists yet. The baseline should still include workspace/run scope, current open coverage/todo items, definition revision when available, and `nextAction=schedule_step`.
- Deviation: no code gap found for the S2.3 behavior. Existing tests already cover initial baseline creation and persistence before dispatch.
- Owner: core orchestrator coverage/gap baseline. This is neutral runtime state, not template/Soul/policy.
- Event path: run start/resume -> dispatch specs/event -> `ensureGapBaselineBeforeDispatch` -> coverage ledger -> `run.executionContext.gapBaseline` -> step input.
- Contract gap: documentation did not explicitly close S2.3 against the current implementation and regression tests.
- Proposed change: no runtime code change for S2.3; record the audit and bind existing tests as acceptance evidence.
- Boundary check: existing baseline code derives from coverage ledger structure and run state only. It does not inspect template IDs, languages, providers, file names, or command output markers.
- Verification: existing orchestrator gap baseline tests, affected package tests, boundary search, and `git diff --check`.

## Closure Items

1. Done: first dispatch path calls `ensureGapBaselineBeforeDispatch`.
2. Done: initial baseline is written to both step input and persisted run execution context.
3. Done: no-evidence initial baseline still produces `todoItems` and `nextAction=schedule_step`.
4. Done: accepted/rejected evidence refs are included only when already present in coverage work items.

## Verification

Commands:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'RuntimeOrchestrator_DispatchRequestedStepHydratesGapBaseline|NextDispatchSpecsPersistsGapBaselineBeforeInitialStep|GapBaselineFromRunPreservesAcceptedAndRejectedEvidenceRefs' -count=1
go test ./internal/modules/application/workorchd/biz/appservice ./internal/services/workorch/biz/backends ./internal/services/workorch/service/tui ./internal/modules/application/workorchd/service/server ./internal/pkg/workorchport ./internal/pkg/workorchclient -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" docs/70-roadmap/active/goal-dod-nextaction-s2-3-initial-gap-baseline.md internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items_test.go internal/modules/kernel/orchestrator/biz/runtime_graph_test.go -S
git diff --check
```
