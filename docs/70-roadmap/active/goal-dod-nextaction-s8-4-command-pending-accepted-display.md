# S8.4 Command Pending And Accepted Display

Status: completed

Type: roadmap-slice

Owner: TUI/Web

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S8.1 Workspace ViewModel Revision Fields](./goal-dod-nextaction-s8-1-workspace-viewmodel-revision-fields.md)
- [S8.2 TUI Prompt Mode From Projection](./goal-dod-nextaction-s8-2-tui-prompt-mode-projection.md)
- [S8.3 TUI List And Main View Same Revision](./goal-dod-nextaction-s8-3-tui-list-main-same-revision.md)
- [MVVP Workspace ViewModel Architecture](../../30-module-architecture/mvvp-workspace-viewmodel-architecture.md)

## Contract Audit

- Scope: TUI and Web display of workspace run command projection after `/run resume`, `/run replan`, `/run restart`, or equivalent Web/REST command submission.
- Observed state: workorchd already projects `runCommand` accepted/rejected metadata into the workspace snapshot. Web renders a run command section, but its client type and helper do not expose all command states. TUI clears stale `submitting` in some snapshot paths, but it does not retain or render the projected command ACK in the runtime status bar.
- Expected state: after command submission, TUI/Web show local submitting only until the next owner projection arrives. Once `runCommand` is projected, the displayed command state must come from the snapshot projection, including accepted or rejected state, request id, intent, source, revision, and rejection error when present.
- Deviation: TUI can show only generic progress without the command ACK, and Web can hide rejected/error fields because the client DTO is narrower than the owner projection.
- Owner: workorchd owns command state and projection; TUI/Web only consume the projected ViewModel/snapshot fields.
- Event path: TUI/Web command -> backend command -> workorchd scheduler command accepted/rejected -> workspace snapshot `runCommand` -> TUI/Web display.
- Ledger impact: none.
- Stage Packet impact: none.
- Contract gap: endpoint consumers do not render the full owner-projected command status consistently, so users cannot distinguish "submitted locally" from "accepted/rejected by owner projection".
- Proposed change: store the active snapshot `RunCommandStatusInfo` in TUI display state, render a neutral command status segment in the runtime status bar, clear stale local submitting after command projection, and align Web/client `RunCommandStatus` fields with the owner projection.
- Boundary check: the change only consumes generic command metadata. It does not inspect template ID, Soul ID, provider, command output, programming language, goal text, or business content.
- Verification: focused TUI regression tests for accepted/rejected command projection, Web/client type check or build where available, `git diff --check`, and diff-only boundary search.

## Slice Scope

In scope:

- TUI runtime status bar displays the projected command state.
- TUI no longer leaves local `submitting` after accepted/rejected command projection.
- Web/client type definitions include the complete projected command status fields.
- Web command descriptions render state and rejection error from projection.

Out of scope:

- Changing command acceptance, idempotency, scheduler queueing, or EventHub behavior.
- Inferring run state from command ACK.
- SSE reconnect behavior.
- Decision/approval full view regression.

## Acceptance

- TUI displays `command accepted <intent>` when owner projection accepts a command.
- TUI displays `command rejected <intent>` and rejection error when owner projection rejects a command.
- Accepted/rejected command projection does not mark a workspace as running by itself.
- Web can render accepted and rejected command status without dropping projected fields.
- The change does not add business/template/language/provider special casing.

## Verification

- Passed: `go test ./internal/services/workorch/service/tui -run 'RunCommand|CommandProjection|Submitting' -count=1`
- Passed: `go test ./internal/services/workorch/service/tui -count=1`
- Passed: `go test ./internal/pkg/workorchclient ./internal/services/workorch/biz/backends -count=1`
- Passed: `npm run build` in `web/workorch-console`
- Passed: `git diff --check`
- Passed: diff-only boundary search found no newly added business/template/language/provider special casing in touched source files.
