# S5.3 Open Decision NextAction

Status: completed

Type: roadmap-slice

Owner: core/orchestrator

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md)
- [S5.1 NextAction Minimal Model](./goal-dod-nextaction-s5-1-nextaction-minimal-model.md)
- [S5.2 Pending Approval NextAction](./goal-dod-nextaction-s5-2-pending-approval-nextaction.md)

## Contract Audit

- Scope: `RuntimeOrchestrator.NextActionForRun` and open decision interpretation.
- Observed state: runs can enter `blocked_on_decision` with `BlockingTicketID`, and resume correctly refuses open tickets, but there was no core `NextAction` for this owner state.
- Expected state: an open blocking decision must deterministically produce `request_decision` before state-changing work is considered.
- Deviation: scheduler/projection consumers could not distinguish "wait for decision" from "schedule state_change" using the S5 NextAction contract.
- Owner: core/orchestrator.
- Event path: decision ticket owner state + run blocking reference -> orchestrator NextAction interpreter -> scheduler/projection consumers.
- Contract gap: missing deterministic NextAction rule for open decision tickets.
- Proposed change: extend `NextActionForRun` with open decision handling and add `deriveOpenDecisionNextAction`.
- Boundary check: the rule only reads structured run/ticket state and scope; it does not inspect ticket text, template, language, provider, command output or business content.
- Verification: unit tests prove an open blocking ticket returns `request_decision`, resolved tickets do not, and unrelated open tickets do not affect a run.

## Design

S5.3 introduces the second deterministic rule:

```text
if no pending approval action applies
and run.state == blocked_on_decision
and run.blockingTicketId points to ticket.state == open
then NextAction.type = request_decision
```

The generated action:

- carries workspace/run scope;
- carries `subjectType=decision` and `subjectId=<ticketId>` for idempotency;
- uses `stage=decision`;
- uses `stageMode=governance`;
- includes `ticketId` in `requiredOutput`;
- allows decision resolution;
- forbids `state_change` while the decision is open.

The rule does not decide how a ticket should be resolved. TUI/Web/API remain command producers, and the decision owner remains the authority for resolution state.

## Acceptance

- Open blocking decision returns `request_decision`.
- Open decision action forbids `state_change`.
- Resolved or missing blocking decision does not return `request_decision`.
- An open ticket not referenced by `run.blockingTicketId` does not affect the run.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'TestNextActionForRunOpenDecision|TestDeriveOpenDecisionNextAction' -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" \
  internal/modules/kernel/orchestrator/biz/runtime_next_action.go \
  internal/modules/kernel/orchestrator/biz/runtime_next_action_test.go \
  docs/70-roadmap/active/goal-dod-nextaction-s5-3-open-decision-nextaction.md \
  docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md
```
