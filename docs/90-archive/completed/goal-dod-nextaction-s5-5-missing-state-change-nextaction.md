# S5.5 Missing State Change NextAction

Status: completed

Type: roadmap-slice

Owner: core/orchestrator

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [S5.1 NextAction Minimal Model](./goal-dod-nextaction-s5-1-nextaction-minimal-model.md)
- [S5.2 Pending Approval NextAction](./goal-dod-nextaction-s5-2-pending-approval-nextaction.md)
- [S5.3 Open Decision NextAction](./goal-dod-nextaction-s5-3-open-decision-nextaction.md)
- [S5.4 Missing Gap Baseline NextAction](./goal-dod-nextaction-s5-4-missing-gap-nextaction.md)

## Contract Audit

- Scope: `RuntimeOrchestrator.NextActionForRun` and coverage work item evidence gaps.
- Observed state: coverage work items already carry structured `requiredEvidenceKinds`, accepted evidence refs and derived `missingEvidenceKinds`, but the S5 `NextAction` interpreter did not yet turn a missing `state_change` evidence gap into a deterministic action.
- Expected state: after governance gates and gap baseline readiness, an active coverage work item missing `state_change` evidence must produce a scoped `state_change` NextAction before validation, assessment or completion can be selected.
- Deviation: downstream scheduler/projection consumers could continue without an explicit state-changing focus even when the ledger says state-changing evidence is missing.
- Owner: core/orchestrator.
- Event path: coverage work item repository/projection -> derived missing evidence state -> orchestrator NextAction interpreter -> scheduler/projection consumers.
- Contract gap: missing deterministic NextAction rule for `state_change` evidence gaps.
- Proposed change: hydrate coverage projection from the coverage work item repository when available, derive the first active work item missing `state_change`, and return a scoped `state_change` NextAction.
- Boundary check: the rule only reads structured coverage item status, ID and evidence kinds; it does not inspect template, language, provider, command output, target text or business content.
- Verification: unit tests prove missing `state_change` evidence returns a scoped action, completed/blocked/superseded work items are ignored, and repository-backed coverage can be used when the run projection is empty.

## Design

S5.5 introduces the fourth deterministic rule:

```text
if no pending approval action applies
and no open decision action applies
and gapBaseline exists
and an active coverage work item is missing state_change evidence
then NextAction.type = state_change
```

The generated action:

- carries workspace/run scope;
- carries `subjectType=coverage` and `subjectId=<coverageWorkItemId>` for idempotency;
- sets `focusCoverageItemIds` to the selected coverage work item;
- sets `requiredEvidenceKinds=["state_change"]`;
- uses `stage=state_change`;
- uses `stageMode=evidence`;
- requires output to bind the selected coverage work item and evidence kind;
- forbids `complete`, `validate` and `assessment` while the state-changing evidence gap remains open.

This slice does not execute the action. Later slices can schedule and render the action through Stage Packet and LLM context.

## Acceptance

- Active run with `gapBaseline` and an active coverage work item missing `state_change` returns `state_change`.
- The action includes a single focused coverage work item and required evidence kind.
- Terminal runs and inactive coverage work items do not return `state_change`.
- Repository-backed coverage work items can drive the action when the run execution context has no coverage projection.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestNextActionForRunMissingStateChange|TestDeriveMissingStateChangeNextAction' -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" \
  internal/modules/kernel/orchestrator/biz/runtime_next_action.go \
  internal/modules/kernel/orchestrator/biz/runtime_next_action_test.go \
  docs/70-roadmap/active/goal-dod-nextaction-s5-5-missing-state-change-nextaction.md \
  docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md
```
