# S5.4 Missing Gap Baseline NextAction

Status: completed

Type: roadmap-slice

Owner: core/orchestrator

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S5.1 NextAction Minimal Model](./goal-dod-nextaction-s5-1-nextaction-minimal-model.md)
- [S5.2 Pending Approval NextAction](./goal-dod-nextaction-s5-2-pending-approval-nextaction.md)
- [S5.3 Open Decision NextAction](./goal-dod-nextaction-s5-3-open-decision-nextaction.md)

## Contract Audit

- Scope: `RuntimeOrchestrator.NextActionForRun` and active run gap baseline gate.
- Observed state: `gapBaseline` is built before dispatch, but the S5 `NextAction` interpreter did not yet expose a deterministic `assess_gap` action when a run lacks baseline state.
- Expected state: an active run without `gapBaseline` must produce `assess_gap` before any `state_change` action can be selected.
- Deviation: downstream scheduler/projection consumers could see no NextAction and might continue ad hoc dispatch decisions without an explicit gap baseline contract.
- Owner: core/orchestrator.
- Event path: run execution context -> gap baseline presence check -> orchestrator NextAction interpreter -> scheduler/projection consumers.
- Contract gap: missing deterministic NextAction rule for absent gap baseline.
- Proposed change: extend `NextActionForRun` with `deriveMissingGapBaselineNextAction` after governance gates and before later coverage rules.
- Boundary check: the rule only reads structured run state and `gapBaseline`; it does not inspect template, language, provider, command output or business content.
- Verification: unit tests prove active runs without baseline return `assess_gap`, existing baseline suppresses it, and terminal runs do not enter gap assessment.

## Design

S5.4 introduces the third deterministic rule:

```text
if no pending approval action applies
and no open decision action applies
and run is active
and run.executionContext.gapBaseline is missing
then NextAction.type = assess_gap
```

The generated action:

- carries workspace/run scope;
- carries `subjectType=run` and `subjectId=<runId>` for idempotency;
- uses `stage=gap_analysis`;
- uses `stageMode=baseline`;
- requires a gap baseline;
- forbids `state_change` while the baseline is missing.

This slice does not build the baseline. Existing baseline construction stays in the dispatch preparation path. Later slices can reuse this action to schedule or project gap assessment explicitly.

## Acceptance

- Active run without `gapBaseline` returns `assess_gap`.
- `assess_gap` forbids `state_change`.
- Active run with a persisted `gapBaseline` does not return `assess_gap`.
- Terminal runs do not return `assess_gap`.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestNextActionForRunMissingGapBaseline|TestDeriveMissingGapBaselineNextAction' -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" \
  internal/modules/kernel/orchestrator/biz/runtime_next_action.go \
  internal/modules/kernel/orchestrator/biz/runtime_next_action_test.go \
  docs/70-roadmap/active/goal-dod-nextaction-s5-4-missing-gap-nextaction.md \
  docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md
```
