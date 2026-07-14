# Goal/DoD NextAction S2.5 Replan Definition And Baseline Inheritance

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S2.4 Resume Gap Baseline](./goal-dod-nextaction-s2-4-resume-gap-baseline.md)

## Contract Audit

- Scope: `RuntimeOrchestrator.replanLoadedRun` successor run creation after decision replan.
- Observed state: replan successor clones predecessor `ExecutionContext`, clears transient compressed/replan state, resets convergence budget, prepares coverage ledger, and starts the successor. The successor already inherits `GoalID`, `DoDID`, current execution context definition data, coverage work items, and gap baseline through dispatch. The run model `DefinitionRevision` field was not explicitly copied to the successor.
- Expected state: a replan successor must remain bound to the current effective Goal/DoD definition revision unless a separate definition-change command produced a new revision. Decision resolution text or payload is governance context only; it must not overwrite `GoalID`, `DoDID`, `DefinitionRevision`, `executionContext.definitionRevision`, coverage baseline, or gap baseline.
- Deviation: model-level `Run.DefinitionRevision` could be empty on the successor even when the predecessor had it set, while execution context still carried the revision. This creates a split between run metadata and context-derived coverage baseline.
- Owner: core orchestrator run state. This is neutral lifecycle state and does not belong to workspaceTemplate, Soul, TUI, Web, or provider-specific code.
- Event path: decision resolution -> resume owner -> `replanLoadedRun` -> successor run create -> coverage ledger `prepareReplanSuccessor` -> start run -> gap baseline dispatch.
- Contract gap: successor creation needed explicit model-level revision inheritance and regression coverage proving resolution payload does not override Goal/DoD definition fields.
- Proposed change: copy `run.DefinitionRevision` into the successor model and extend the running replan integration test to seed conflicting resolution payload fields while asserting predecessor definitions and gap baseline are retained.
- Boundary check: the change only copies structured run metadata and coverage state. It does not inspect template IDs, Soul IDs, languages, command output text, filenames, provider names, or business-specific content.
- Verification: targeted replan integration test, existing replan/gap baseline tests, affected orchestrator package tests, boundary search, and `git diff --check`.

## Closure Items

1. Done: replan successor now inherits `Run.DefinitionRevision`.
2. Done: replan successor keeps predecessor `GoalID` and `DoDID`.
3. Done: decision resolution payload is retained only under `replanContinuation` context and does not become authoritative definition state.
4. Done: existing coverage ledger preparation continues to rebuild current gap baseline for successor dispatch.

## Verification

Commands:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'RunningResumeReplansFromResolvedDecisionOnFailedNonTerminalStep|ResumeRunReplanSupersedesRun|ResumeRunLegacyRepairResolutionPayloadReplans' -count=1
go test ./internal/modules/kernel/orchestrator/biz -run 'CoverageWorkItemsReplanSuccessorPreservesCurrentRevisionLineage|RuntimeOrchestrator_RunningResumeReplansFromResolvedDecisionOnFailedNonTerminalStep' -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|pkg/lsp|mcp-lsp-bridge" docs/70-roadmap/active/goal-dod-nextaction-s2-5-replan-definition-baseline.md internal/modules/kernel/orchestrator/biz/runtime_run_state.go internal/modules/kernel/orchestrator/biz/runtime_repair_resume_integration_test.go -S
git diff --check
```
