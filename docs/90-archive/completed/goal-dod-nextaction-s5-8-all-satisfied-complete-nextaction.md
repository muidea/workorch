# S5.8 All Satisfied Complete NextAction

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
- [S5.5 Missing State Change NextAction](./goal-dod-nextaction-s5-5-missing-state-change-nextaction.md)
- [S5.6 Missing Validation NextAction](./goal-dod-nextaction-s5-6-missing-validation-nextaction.md)
- [S5.7 Recoverable Diagnostic NextAction](./goal-dod-nextaction-s5-7-recoverable-diagnostic-nextaction.md)

## Contract Audit

- Scope: `RuntimeOrchestrator.NextActionForRun` and completion readiness from coverage work items.
- Observed state: coverage work items already expose structured status, required evidence kinds and accepted evidence refs, but the S5 `NextAction` interpreter did not yet produce a deterministic `complete` action when all coverage items are satisfied.
- Expected state: after governance, recoverable diagnostics, gap baseline and missing evidence checks, an active run whose coverage work items are all satisfied with accepted evidence must produce `complete`.
- Deviation: downstream scheduler/projection consumers could see no NextAction even when the ledger is already ready for completion.
- Owner: core/orchestrator.
- Event path: coverage work item repository/projection -> satisfied evidence check -> orchestrator NextAction interpreter -> scheduler/projection consumers.
- Contract gap: missing deterministic NextAction rule for completion-ready coverage ledger.
- Proposed change: reuse the coverage work item projection used by S5.5/S5.6, require a non-empty all-satisfied ledger with accepted evidence, and return a scoped `complete` NextAction.
- Boundary check: the rule only reads structured coverage status, evidence refs and required evidence kinds; it does not inspect template, language, provider, command output, target text or business content.
- Verification: unit tests prove satisfied coverage returns `complete`, open/evidence-ready/no-evidence/terminal runs do not, and recoverable diagnostics keep priority over completion.

## Design

S5.8 introduces the final P5 deterministic completion rule:

```text
if no pending approval action applies
and no open decision action applies
and no recoverable rejected diagnostic applies
and gapBaseline exists
and no active coverage work item is missing state_change evidence
and no active coverage work item is missing validation evidence
and every coverage work item is satisfied/closed/completed with accepted evidence
then NextAction.type = complete
```

The generated action:

- carries workspace/run scope;
- carries `subjectType=run` and `subjectId=<runId>` for idempotency;
- uses `stage=complete`;
- uses `stageMode=accepted_evidence`;
- includes focused coverage work item IDs;
- includes the union of required evidence kinds from the satisfied ledger;
- requires completion to freeze accepted evidence;
- forbids state-changing, validation, repair and governance request actions while the completion ledger remains valid.

This slice does not finalize the run. Later slices can schedule and execute completion through the same owner/EventHub path.

## Acceptance

- Active run with `gapBaseline` and all coverage items satisfied with accepted evidence returns `complete`.
- Empty coverage ledger does not return `complete`.
- Open, evidence-ready, blocked, skipped or evidence-less items do not return `complete`.
- Terminal runs do not return `complete`.
- Recoverable rejected diagnostics keep priority over completion.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestNextActionForRunAllSatisfied|TestDeriveAllSatisfiedCompleteNextAction' -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" \
  internal/modules/kernel/orchestrator/biz/runtime_next_action.go \
  internal/modules/kernel/orchestrator/biz/runtime_next_action_test.go \
  docs/70-roadmap/active/goal-dod-nextaction-s5-8-all-satisfied-complete-nextaction.md \
  docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md
```
