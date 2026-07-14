# Goal/DoD NextAction S0.5 Projection ViewModel TUI Prompt Audit

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [MVVP Workspace ViewModel Architecture](../../30-module-architecture/mvvp-workspace-viewmodel-architecture.md)
- [S8.1 Workspace ViewModel Revision Fields](./goal-dod-nextaction-s8-1-workspace-viewmodel-revision-fields.md)
- [S8.2 TUI Prompt Mode Projection](./goal-dod-nextaction-s8-2-tui-prompt-mode-projection.md)
- [S8.3 TUI List Main Same Revision](./goal-dod-nextaction-s8-3-tui-list-main-same-revision.md)
- [S8.4 Command Pending And Accepted Display](./goal-dod-nextaction-s8-4-command-pending-accepted-display.md)
- [S8.5 SSE Reconnect Last Event ID](./goal-dod-nextaction-s8-5-sse-reconnect-last-event-id.md)
- [S8.6 Decision Approval View Regression](./goal-dod-nextaction-s8-6-decision-approval-view-regression.md)
- [S11.6 TUI/Web Projection E2E](./goal-dod-nextaction-s11-6-tui-web-projection-e2e.md)

## Contract Audit

- Scope: workorchd projection/ViewModel ownership, TUI workspace list, main view, prompt mode, command display and Web/TUI refresh.
- Observed state: WorkspaceViewModel revision fields are projected by workorchd. TUI prompt mode, list and main view consume projection state instead of maintaining a second runtime state machine. Web/TUI e2e covers projection scope, switch refresh, prompt/list/main/status consistency and decision/approval view regression.
- Expected state: TUI/Web only submit commands and render projected state. workorchd is the ViewModel owner and state source for prompt mode, command status, governance blockers and coverage summary.
- Deviation: S0.5 originally had no standalone data-source audit, so the later S8/S11.6 slices carried the closure evidence without a single baseline.
- Owner: workorchd projection and TUI/Web port consumers.
- Event path: owner state event -> workorchd projection -> workspace snapshot/list/SSE -> port/client DTO -> TUI/Web render.
- Contract gap: documentation gap only. Later S8 and S11.6 slices provide the implementation and tests.
- Proposed change: record this projection/ViewModel inventory as the S0.5 audit baseline and link the completed implementation slices.
- Boundary check: UI state is derived from neutral projection fields. No template, language, command-output, or workspace-specific state inference is added.
- Verification: linked S8/S11.6 tests plus roadmap coverage audit.

## Data Source Inventory

| UI Surface | Authoritative Source | Verified By |
| --- | --- | --- |
| Workspace list | projected workspace summary/revision | S8.1, S8.3 |
| Main view | projected workspace snapshot/revision | S8.3 |
| Prompt mode | projected run/governance state | S8.2, S8.6 |
| Command status | projected command accepted/rejected state | S8.4 |
| Web stream refresh | SSE reconnect and last event id | S8.5 |
| Cross-client e2e | service/Web/TUI projection contracts | S11.6 |

## Verification

- S8.1-S8.6 completed and document ViewModel revision, prompt mode, same-revision render, command display, SSE reconnect and governance view regression.
- S11.6 completed and groups service/Web/TUI projection e2e.
- `python3` roadmap coverage audit reports S0.5 as covered by this document.
