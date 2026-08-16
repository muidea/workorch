# S5.2 Pending Approval NextAction

Status: completed

Type: roadmap-slice

Owner: core/orchestrator

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [S5.1 NextAction Minimal Model](./goal-dod-nextaction-s5-1-nextaction-minimal-model.md)

## Contract Audit

- Scope: `RuntimeOrchestrator.NextActionForRun` and pending approval interpretation.
- Observed state: runs can enter `blocked_on_approval` with `BlockingApprovalID`, and resume correctly refuses pending approval, but there was no core `NextAction` projection for this owner state.
- Expected state: pending approval must deterministically produce `request_approval` before any state-changing action is considered.
- Deviation: later scheduler/projection work had no reusable core contract to distinguish "wait for approval" from "schedule state_change".
- Owner: core/orchestrator.
- Event path: approval owner state + run blocking reference -> orchestrator NextAction interpreter -> scheduler/projection consumers.
- Contract gap: missing first NextAction rule for pending approval.
- Proposed change: add a scoped `NextActionForRun` entry point and derive `request_approval` when the run is blocked on a pending approval.
- Boundary check: the rule only reads structured run and approval state; it does not inspect approval text, template, language, provider, command output or business content.
- Verification: unit tests prove pending approval returns `request_approval`, approved approval does not return it, and unrelated pending approvals do not affect a blocked run.

## Design

S5.2 introduces only the first deterministic rule:

```text
if run.state == blocked_on_approval
and run.blockingApprovalId points to approval.state == pending
then NextAction.type = request_approval
```

The generated action:

- carries workspace/run scope;
- carries `subjectType=approval` and `subjectId=<approvalId>` for idempotency;
- uses `stage=approval`;
- uses `stageMode=governance`;
- includes `approvalId` in `requiredOutput`;
- allows approval resolution actions;
- forbids `state_change` while approval is pending.

The rule intentionally does not auto-approve, reject, resume, replan, validate or inspect coverage gaps. Those remain separate owner flows or later S5 slices.

## Acceptance

- Pending blocking approval returns `request_approval`.
- Pending approval action forbids `state_change`.
- Approved or missing blocking approval does not return `request_approval`.
- A pending approval not referenced by `run.blockingApprovalId` does not affect the run.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestNextActionForRunPendingApproval|TestDerivePendingApprovalNextAction' -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" \
  internal/modules/kernel/orchestrator/biz/runtime_next_action.go \
  internal/modules/kernel/orchestrator/biz/runtime_next_action_test.go \
  docs/70-roadmap/active/goal-dod-nextaction-s5-2-pending-approval-nextaction.md \
  docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md
```
