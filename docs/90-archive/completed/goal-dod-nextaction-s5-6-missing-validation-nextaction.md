# S5.6 Missing Validation NextAction

Status: completed

Type: roadmap-slice

Owner: core/orchestrator

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S5.1 NextAction Minimal Model](./goal-dod-nextaction-s5-1-nextaction-minimal-model.md)
- [S5.2 Pending Approval NextAction](./goal-dod-nextaction-s5-2-pending-approval-nextaction.md)
- [S5.3 Open Decision NextAction](./goal-dod-nextaction-s5-3-open-decision-nextaction.md)
- [S5.4 Missing Gap Baseline NextAction](./goal-dod-nextaction-s5-4-missing-gap-nextaction.md)
- [S5.5 Missing State Change NextAction](./goal-dod-nextaction-s5-5-missing-state-change-nextaction.md)

## Contract Audit

- Scope: `RuntimeOrchestrator.NextActionForRun` and coverage work item validation evidence gaps.
- Observed state: coverage work items can report missing `validation` evidence, but the S5 `NextAction` interpreter did not yet turn that missing evidence into a deterministic `validate` action.
- Expected state: after governance gates, gap baseline readiness, and state-changing evidence readiness, an active coverage work item missing `validation` evidence must produce a scoped `validate` NextAction before assessment or completion can be selected.
- Deviation: downstream scheduler/projection consumers could continue without a validation focus even when the ledger says validation evidence is missing.
- Owner: core/orchestrator.
- Event path: coverage work item repository/projection -> derived missing evidence state -> orchestrator NextAction interpreter -> scheduler/projection consumers.
- Contract gap: missing deterministic NextAction rule for `validation` evidence gaps.
- Proposed change: reuse the coverage work item projection used by S5.5, derive the first active work item missing `validation`, and return a scoped `validate` NextAction after the `state_change` rule.
- Boundary check: the rule only reads structured coverage item status, ID and evidence kinds; it does not inspect template, language, provider, command output, target text or business content.
- Verification: unit tests prove validation gaps return a scoped action only after state-changing gaps are absent, and terminal/inactive work items do not trigger validation.

## Design

S5.6 introduces the fifth deterministic rule:

```text
if no pending approval action applies
and no open decision action applies
and gapBaseline exists
and no active coverage work item is missing state_change evidence
and an active coverage work item is missing validation evidence
then NextAction.type = validate
```

The generated action:

- carries workspace/run scope;
- carries `subjectType=coverage` and `subjectId=<coverageWorkItemId>` for idempotency;
- sets `focusCoverageItemIds` to the selected coverage work item;
- sets `requiredEvidenceKinds=["validation"]`;
- uses `stage=validate`;
- uses `stageMode=evidence`;
- requires output to bind the selected coverage work item and evidence kind;
- forbids `complete`, `assessment` and `state_change` while the validation evidence gap remains open.

This slice does not execute the action. Later slices can schedule and render the action through Stage Packet and LLM context.

## Acceptance

- Active run with `gapBaseline` and an active coverage work item missing `validation` returns `validate` after no `state_change` gap remains.
- The action includes a single focused coverage work item and required evidence kind.
- Terminal runs and inactive coverage work items do not return `validate`.
- If any active coverage work item still lacks `state_change`, S5.5 keeps priority over S5.6.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestNextActionForRunMissingValidation|TestDeriveMissingValidationNextAction' -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" \
  internal/modules/kernel/orchestrator/biz/runtime_next_action.go \
  internal/modules/kernel/orchestrator/biz/runtime_next_action_test.go \
  docs/70-roadmap/active/goal-dod-nextaction-s5-6-missing-validation-nextaction.md \
  docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md
```
