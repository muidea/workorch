# S8.6 Decision And Approval View Regression

Status: completed

Type: roadmap-slice

Owner: TUI/Web

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S8.2 TUI Prompt Mode From Projection](./goal-dod-nextaction-s8-2-tui-prompt-mode-projection.md)
- [S8.3 TUI List And Main View Same Revision](./goal-dod-nextaction-s8-3-tui-list-main-same-revision.md)
- [S8.4 Command Pending And Accepted Display](./goal-dod-nextaction-s8-4-command-pending-accepted-display.md)
- [S8.5 SSE Reconnect And Last Event ID](./goal-dod-nextaction-s8-5-sse-reconnect-last-event-id.md)
- [MVVP Workspace ViewModel Architecture](../../30-module-architecture/mvvp-workspace-viewmodel-architecture.md)

## Contract Audit

- Scope: TUI/Web decision and approval views after decision resolution, approval approve, or approval reject.
- Observed state: TUI decision resolution clears local input/blocker and explicitly batches workspace context refresh plus decision list refresh. TUI approval resolution clears local input/blocker and refreshes active workspace state, but does not explicitly refresh approval list/detail in the same way. Web detail and governance pages already call their owner APIs after resolve and also refresh through SSE projection changes.
- Expected state: after decision or approval resolution, TUI/Web must show only owner-projected governance state. Local resolved markers may be optimistic, but they must be followed by workspace context and governance list refresh so stale prompts or stale pending rows cannot persist.
- Deviation: TUI approval resolution has a weaker refresh chain than decision resolution, which can leave approval list state stale until a later projection/list refresh.
- Owner: workorchd owns governance state and workspace projection; TUI/Web only submit scoped commands and consume projection/port DTOs.
- Event path: TUI/Web resolve command -> workorchd decision/approval owner -> workspace projection and governance records -> TUI/Web context/list refresh -> prompt/list/main view.
- Ledger impact: none.
- Stage Packet impact: none.
- Contract gap: approval resolution does not explicitly refresh the approval projection/list after local resolution.
- Proposed change: make TUI `ApprovalResolvedMsg` batch the same owner projection refreshes as decision resolution: active workspace state, workspace context for the resolved workspace, and approval list refresh.
- Boundary check: the change only uses generic workspace/approval scope and owner-projected DTOs. It does not inspect template ID, Soul ID, provider, command output, programming language, goal text, or business content.
- Verification: focused TUI regression test for approval resolved refresh command contents, full TUI package test, `git diff --check`, and diff-only boundary search.

## Slice Scope

In scope:

- TUI approval resolution refresh parity with decision resolution.
- TUI regression tests for approval prompt/list refresh behavior.
- Web path audit confirming existing detail/governance pages refresh after resolve and subscribe to projection stream.

Out of scope:

- Changing governance owner state or workorchd resolution semantics.
- Changing decision/approval REST contracts.
- Changing runtime scheduling after replan/approve.
- Adding Web e2e automation; covered later by S11.6.

## Acceptance

- TUI approval approve/reject clears local input/blocker and waits for server continuation.
- TUI approval resolution explicitly refreshes workspace context and approval list for the resolved workspace.
- Existing TUI decision resolution behavior remains unchanged.
- Web continues to refresh detail/governance pages from owner APIs and SSE projection events.
- The change does not add business/template/language/provider special casing.

## Verification

- Passed: `go test ./internal/services/workorch/service/tui -run 'DecisionResolved|ApprovalResolved|DecisionApproval|ApprovalListClears' -count=1`
- Passed: `go test ./internal/services/workorch/service/tui -count=1`
- Passed: `git diff --check`
- Passed: diff-only boundary search found no newly added business/template/language/provider special casing in touched TUI files.
