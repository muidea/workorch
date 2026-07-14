# S5.7 Recoverable Diagnostic NextAction

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

## Contract Audit

- Scope: `RuntimeOrchestrator.NextActionForRun` and rejected diagnostic evidence records.
- Observed state: recoverable contract violations are already recorded as rejected diagnostic evidence with structured `recoverability`, but the S5 `NextAction` interpreter did not yet expose a deterministic `repair` action for those diagnostics.
- Expected state: after governance gates, an active run with recoverable rejected diagnostic evidence must produce `repair` before gap/evidence actions instead of falling through to no action or a human decision.
- Deviation: downstream scheduler/projection consumers could miss the recoverable diagnostic and route to ad hoc waiting decision behavior.
- Owner: core/orchestrator.
- Event path: rejected diagnostic evidence record -> structured recoverability check -> orchestrator NextAction interpreter -> scheduler/projection consumers.
- Contract gap: missing deterministic NextAction rule for recoverable rejected diagnostics.
- Proposed change: read run-scoped evidence records, select the latest non-superseded rejected diagnostic with retryable recoverability, and return a scoped `repair` NextAction.
- Boundary check: the rule only reads structured evidence kind/status/recoverability and scope; it does not inspect template, language, provider, command output, failure text or business content.
- Verification: unit tests prove recoverable rejected diagnostic evidence returns `repair`, decision-required diagnostics do not, superseded diagnostics are ignored, and evidence gaps keep priority over diagnostic repair.

## Design

S5.7 introduces the third deterministic rule in the canonical priority order:

```text
if no pending approval action applies
and no open decision action applies
and latest non-superseded rejected diagnostic evidence is recoverable
then NextAction.type = repair
```

Recoverable values are limited to the runtime contract enum:

- `retry_same_contract`
- `narrow_contract`
- `normalize_then_retry`

The generated action:

- carries workspace/run scope;
- carries `subjectType=diagnostic` and `subjectId=<evidenceRecordId>` for idempotency;
- uses `stage=repair`;
- uses `stageMode=diagnostic_retry`;
- requires the repair to bind the rejected diagnostic evidence;
- forbids `request_decision` and `complete` while the recoverable diagnostic remains unresolved.

This slice does not execute the repair. Later slices can schedule and render the action through Stage Packet and LLM context.

## Acceptance

- Active run with recoverable rejected diagnostic evidence returns `repair`.
- `decision_required` or `fail_fast` diagnostics do not return `repair`.
- Superseded rejected diagnostics do not return `repair`.
- Recoverable rejected diagnostics keep priority over gap baseline and missing evidence actions, while governance approval/decision gates keep priority over diagnostic repair.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestNextActionForRunRecoverableDiagnostic|TestDeriveRecoverableDiagnosticNextAction' -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" \
  internal/modules/kernel/orchestrator/biz/runtime_next_action.go \
  internal/modules/kernel/orchestrator/biz/runtime_next_action_test.go \
  docs/70-roadmap/active/goal-dod-nextaction-s5-7-recoverable-diagnostic-nextaction.md \
  docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md
```
