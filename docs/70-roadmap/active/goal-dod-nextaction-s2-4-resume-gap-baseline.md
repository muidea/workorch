# Goal/DoD NextAction S2.4 Resume Gap Baseline

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S2.3 Initial Gap Baseline](./goal-dod-nextaction-s2-3-initial-gap-baseline.md)

## Contract Audit

- Scope: `ResumeRunWithExecutionContextPatch` dispatch path for blocked and running runs.
- Observed state: normal resume paths that call `nextDispatchSpecs` or `nextDispatchSpecsAfterStep` already refresh `gapBaseline`. Resume-specific recovery branches can construct dispatch specs directly for provider retry, runtime repair, context gate retry, artifact continuation, or running-run recovery.
- Expected state: every resume dispatch must refresh or reuse the authoritative run-level `gapBaseline`, apply `gapBaselineDispatchGate`, and pass the resulting baseline into the next runtime step input before any side effect.
- Deviation: specialized resume recovery specs did not have a single pre-dispatch baseline gate. They could rely on later enrichment or event-driven dispatch, which is not an explicit owner-level contract for the direct dispatch path.
- Owner: core orchestrator. This is neutral runtime state derived from Goal/DoD coverage, not workspaceTemplate, Soul, provider, language, or tool policy.
- Event path: TUI/Web/REST command -> appservice scoped resume command -> orchestrator resume owner -> dispatch specs -> `prepareGapBaselineDispatchSpecs` -> coverage ledger/gap gate -> runtime step input -> projection.
- Contract gap: resume needed one common pre-dispatch gate for all spec sources, including direct recovery branches.
- Proposed change: add `prepareGapBaselineDispatchSpecs` and call it before resume dispatch for both running recovery and blocked decision/approval recovery paths. The helper refreshes persisted run baseline, applies the dispatch gate, and injects current coverage context and gap baseline into each spec.
- Boundary check: the helper uses only run state, coverage work item context, coverage focus, and gap baseline. It does not inspect template IDs, Soul IDs, file names, programming languages, command output markers, provider names, or business text.
- Verification: targeted resume contract test asserts stale baseline is refreshed before dispatch and persisted; affected orchestrator tests and boundary search must pass.

## Closure Items

1. Done: resume-specific dispatch specs now pass through a shared gap baseline preparation helper.
2. Done: the helper refreshes `run.executionContext.gapBaseline` from coverage ledger before dispatch.
3. Done: the helper applies `gapBaselineDispatchGate` so `resume` cannot bypass wait/no-action/fail-fast actions.
4. Done: runtime step input receives current `coverageWorkItemContext` and `gapBaseline`.

## Verification

Commands:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'ResumeRunClearsResolvedDecisionBeforeGapBaselineRefresh|ResumeRepairPlanRetryDispatchesRuntimeRepair|ResumeRunContinuesFromNextStepAfterRuntimeDecisionResolved' -count=1
go test ./internal/modules/kernel/orchestrator/biz -run 'RuntimeOrchestrator_DispatchRequestedStepHydratesGapBaseline|NextDispatchSpecsPersistsGapBaselineBeforeInitialStep|GapBaselineFromRunPreservesAcceptedAndRejectedEvidenceRefs' -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" docs/70-roadmap/active/goal-dod-nextaction-s2-4-resume-gap-baseline.md internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go internal/modules/kernel/orchestrator/biz/runtime_lifecycle.go internal/modules/kernel/orchestrator/biz/runtime_resume_contract_integration_test.go -S
git diff --check
```
