# workorch TUI Agent Acceptance

## Purpose

This document defines acceptance criteria for the workorch TUI agent
experience. It is intended for product confirmation first and then for
converting the accepted scenarios into repeatable E2E tests.

Requirement source:

- `docs/workorch-cli-tui-agent-requirements.md`

## Acceptance Scope

The acceptance target is the default local `workorch` TUI mode.

In scope:

- `workorch` with no subcommand opens the TUI agent session.
- Local mode embeds workorch runtime capability.
- Local mode uses magicCommon EventHub command/event/callback interaction.
- TUI state changes are driven by EventHub callbacks, not polling.
- Current shell directory is the default Workspace working directory.
- The TUI can start runs, observe progress, handle Decision Tickets and
  Approvals, switch Workspace, and display terminal Result/Artifact context.
- The default TUI display is result-first: detailed runtime event history stays
  in backend records and is not shown as a debug/event-stream view.

Out of scope for this TUI acceptance document:

- Web Console behavior.
- Deployed `workorchd` HTTP service acceptance.
- Capacity/load testing.
- Remote TUI transport beyond proving it is isolated behind the backend.

## Hard Acceptance Rules

These rules are binary. Any failure blocks TUI acceptance.

- Local TUI must not import `internal/modules/application/workorch/biz/client`.
- Local TUI must not use fixed-interval polling for state refresh.
- Local TUI must not start or require a `workorchd` child process.
- Local TUI must not require a local HTTP listener.
- Agent interaction must not bypass the shared backend contract.
- Event updates must be delivered through callback/subscription messages.
- All user-visible state changes must be reproducible through tests.
- The default TUI must not render raw internal event lines such as `run:`,
  `step:`, `goal:`, or `answer:`.
- The input area must remain near the bottom of the terminal, above the help
  bar.

## Verification Targets

Expected final verification commands:

```bash
make cli-test
make verify-cli
make verify-tui-agent
go test ./internal/modules/application/workorch/... --count 1
```

New target to add:

- `make verify-tui-agent`

Expected script to add:

- `scripts/verify-tui-agent.sh`

Expected focused test packages:

- `internal/modules/application/workorch/service/tui`
- `internal/modules/application/workorch/biz/backend`
- `internal/modules/application/workorch/biz/eventhub`
- `internal/modules/application/workorch/biz/agent`

## Test Layers

### Layer 1. Static Architecture Guards

Purpose:

- Prevent forbidden local-mode paths from returning.

Checks:

- TUI package has no `biz/client` import.
- TUI package has no `tea.Tick`, `time.Tick`, `time.Ticker`, or fixed polling
  state refresh.
- TUI package does not call HTTP client APIs directly.
- Production CLI code does not import `biz/local`.
- Agent local flow does not directly call
  `appservice.NewRuntimeSessionFromInitiator` for normal business interaction.

Suggested command:

```bash
scripts/verify-tui-agent.sh --static-only
```

### Layer 2. TUI Model Tests With Fake Backend

Purpose:

- Verify TUI behavior deterministically without a terminal, HTTP server, or
  real LLM.

Test method:

- Instantiate TUI model with a fake backend.
- Inject keyboard messages and backend event callbacks.
- Assert rendered model state and outgoing backend commands.

Must cover:

- Initial model state.
- Current directory Workspace binding.
- Current directory Workspace reuse before last active fallback.
- Submit goal.
- Workspace switch.
- Compact progress update callback.
- Decision prompt and resolution.
- Approval prompt and resolution.
- Terminal completed result.
- Terminal failed result.
- Interrupt/cancel behavior.
- Stale event ignore.
- Async error display.

Suggested command:

```bash
go test ./internal/modules/application/workorch/service/tui --count 1
```

### Layer 3. Backend/EventHub Integration Tests

Purpose:

- Verify the local backend can drive TUI-visible state through EventHub.

Test method:

- Use embedded runtime/EventHub test setup.
- Publish command events through the local backend.
- Assert command result events and subscribed state callbacks.

Must cover:

- Workspace create or current-directory Workspace ensure.
- Run activation command result.
- Runtime progress event delivery.
- Decision resolve command result.
- Approval approve/reject command result.
- Run resume command result.
- Concurrent command correlation.
- Subscription cleanup on context cancel.

Suggested command:

```bash
go test ./internal/modules/application/workorch/biz/eventhub ./internal/modules/application/workorch/biz/backend --count 1
```

### Layer 4. Scripted TUI E2E

Purpose:

- Verify the installed `workorch` binary behaves like a TUI agent from the
  operator perspective.

Test method:

- Build `./bin/workorch`.
- Launch it in a temporary project directory.
- Drive input through a PTY-compatible scripted runner.
- Use deterministic fake backend or deterministic local mock provider.
- Capture terminal output snapshots.
- Assert visible state transitions and terminal result text.

Suggested command:

```bash
make verify-tui-agent
```

## Acceptance Scenarios

### TUI-E2E-01. Opens Agent TUI By Default

Status: pending

Given:

- `./bin/workorch` is built.
- No `--remote` flag is provided.
- The command is started inside a temporary project directory.

When:

- The user runs `workorch`.

Then:

- The first screen is an agent session TUI.
- The screen includes an input area.
- The screen includes active Workspace identity.
- The screen includes all-Workspace status context.
- No HTTP listener is started.
- No `workorchd` child process is started.

E2E assertion:

- Captured TUI output contains stable markers for agent input, active
  Workspace, and status panel.
- Static/process checks show no child `workorchd` process.

### TUI-E2E-02. Current Directory Workspace Binding

Status: pending

Given:

- The user starts `workorch` in `/tmp/workorch-tui-e2e/project-a`.

When:

- The TUI initializes.

Then:

- The active Workspace is bound to that directory.
- Starting a run without explicit Workspace selection uses that Workspace.
- The binding is visible enough for the user to confirm where agent actions
  will apply.

E2E assertion:

- Backend receives a Workspace ensure/create command with
  `workingDirectory=/tmp/workorch-tui-e2e/project-a`.
- The TUI renders the active Workspace as current.

### TUI-E2E-03. Submit Goal Starts Run

Status: pending

Given:

- The TUI is open in a current-directory Workspace.

When:

- The user types a goal and submits it.

Then:

- The TUI publishes a run activation command through the backend.
- The TUI shows that a Run was created or started.
- The TUI does not poll to discover the Run.

E2E assertion:

- Fake backend records exactly one activation command.
- TUI updates after receiving the matching callback event.

### TUI-E2E-04. Runtime Progress Updates From Callback

Status: pending

Given:

- A Run is active.

When:

- Backend emits step, tool call, or provider stream progress events.

Then:

- The TUI updates the compact progress summary.
- Progress is associated with the active Run.
- Detailed progress events remain backend records and are not rendered as a
  default event stream.
- No periodic refresh is required.

E2E assertion:

- Injected progress events update stable progress summary markers in terminal
  output.
- No `TickMsg` or fixed poll refresh is used by the TUI model.

### TUI-E2E-05. Decision Ticket Inline Resolution

Status: pending

Given:

- A Run becomes `blocked_on_decision`.

When:

- Backend emits an open Decision Ticket event.
- The user enters an answer in the TUI.

Then:

- The TUI shows the Decision question.
- The user's answer is sent through the backend decision resolve command.
- The TUI receives a command result callback.
- The Run can continue through the same event-driven path.

E2E assertion:

- Fake backend records one decision resolution command with the expected
  `ticketId` and payload.
- TUI renders the ticket as resolved after callback.

### TUI-E2E-06. Approval Inline Approve And Reject

Status: pending

Given:

- A Run becomes `blocked_on_approval`.

When:

- Backend emits a pending Approval event.
- The user approves or rejects in the TUI.

Then:

- The TUI shows the Approval reason.
- The selected action is sent through the backend.
- The TUI updates after the command result callback.

E2E assertion:

- Approve path records `approve`.
- Reject path records `reject`.
- No direct HTTP endpoint is called.

### TUI-E2E-07. Workspace Switch

Status: pending

Given:

- The TUI has at least two Workspaces.

When:

- The user opens the Workspace switcher and selects another Workspace.

Then:

- The active Workspace changes.
- Subsequent submitted goals use the selected Workspace.
- All-Workspace status remains visible.
- Events from non-active Workspaces update the monitor but do not replace the
  active result/progress view.

E2E assertion:

- Fake backend receives the next activation command with the selected
  Workspace ID.
- Stale active-Workspace scoped events are ignored by the main session view.

### TUI-E2E-08. Terminal Result And Artifact Display

Status: pending

Given:

- A Run reaches a terminal completed state with accepted Result and linked
  Artifact references.

When:

- Backend emits terminal result events.

Then:

- The TUI shows completed state.
- The TUI shows final Result summary.
- The TUI shows Artifact references or names.
- The completed session remains visible after the Run finishes.

E2E assertion:

- Captured output includes completed state, result summary, and artifact
  reference.

### TUI-E2E-09. Failure Display

Status: pending

Given:

- A Run fails.

When:

- Backend emits a failure event with failure reason.

Then:

- The TUI shows failed state.
- The failure reason is visible.
- The user can submit a follow-up or start another Run without restarting the
  TUI.

E2E assertion:

- Captured output includes failed state and failure reason.
- Subsequent submit sends a new backend command.

### TUI-E2E-10. Interrupt And Cleanup

Status: pending

Given:

- A Run is active or a command is waiting for result.

When:

- The user presses Ctrl-C or quits the TUI.

Then:

- Active command waits are cancelled.
- EventHub subscriptions are unsubscribed.
- Runtime shutdown is clean.
- No goroutine/process leaks remain in the E2E harness.

E2E assertion:

- Fake backend receives cancellation/unsubscribe calls.
- Process exits with expected status.

### TUI-E2E-11. Remote Mode Isolation

Status: pending

Given:

- `workorch --remote http://127.0.0.1:8080` is used.

When:

- The TUI starts and the user submits a command.

Then:

- TUI uses the shared backend interface.
- Remote backend handles transport.
- TUI and agent code do not import HTTP client directly.
- Local EventHub command dispatcher is not used for remote business writes.

E2E assertion:

- Fake remote backend records the command.
- Static checks confirm no direct TUI HTTP client import.

## Manual Acceptance Checklist

Before marking the TUI as accepted, a reviewer should confirm:

- The first screen feels like an agent workspace, not a resource dashboard.
- The active Workspace and working directory are always understandable.
- The user can submit a goal without learning internal workorch objects first.
- The input area is near the bottom and remains visible during updates.
- The default main view does not show detailed raw run/step/event history.
- Decision and Approval interruptions are handled inline.
- Runtime progress is compact, live, and event-driven.
- Terminal success and failure states are clear.
- Workspace switching is discoverable and does not corrupt the active session.
- The TUI remains usable on a normal terminal size.

## E2E Implementation Notes

Recommended test harness design:

- Prefer a fake backend for deterministic TUI E2E tests.
- Add a small scripted PTY runner only for binary-level acceptance.
- Keep real LLM/provider behavior out of TUI acceptance.
- Use backend event injection to simulate progress, blocking, and terminal
  states.
- Keep terminal assertions based on stable text markers, not color escape
  sequences.
- Run static guards before scripted E2E so architecture regressions fail fast.

Suggested `make verify-tui-agent` sequence:

```bash
go test ./internal/modules/application/workorch/service/tui --count 1
go test ./internal/modules/application/workorch/biz/backend ./internal/modules/application/workorch/biz/eventhub --count 1
./scripts/verify-tui-agent.sh --static-only
./scripts/verify-tui-agent.sh --scripted
```

## Promotion Rule

A scenario can move from `pending` to `verified` only when:

- The corresponding automated test exists.
- The test is included in `make verify-tui-agent` or `make cli-test`.
- The test passes locally.
- Any required manual acceptance note is recorded in this document.
