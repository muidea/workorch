# workorch TUI Display Closure Tracking

## Purpose

This document tracks the current TUI display closure scope confirmed during the
May 3, 2026 review. It is used as the delivery checklist and acceptance record
before code changes are implemented.

The scope is limited to the default `workorch` TUI agent experience. It does
not replace `docs/workorch-cli-tui-agent-requirements.md`; it records the
focused display and workspace-selection closure items for this development
round.

## Confirmed Product Decisions

- The default TUI must be result-first and display-converged.
- Detailed run, step, decision, approval, and provider stream events remain in
  backend records only.
- The TUI must not provide a detailed event-stream or debug-event view in this
  round.
- Running progress may be shown only as a compact progress/status summary.
- The input area must be fixed near the bottom of the terminal, above the help
  bar.
- Workspace startup selection must prefer the current directory binding first.
- If the current directory has no bound Workspace, TUI may reuse the last active
  Workspace.
- If neither current directory binding nor last active Workspace is usable, TUI
  may create or bind a Workspace for the current directory.
- The TUI agent loop only maintains the user message loop and must not fork or
  replace backend runtime behavior.
- Local TUI interaction must continue to use the shared backend and
  EventHub-driven callbacks, with no direct HTTP client path and no polling.

## Current Problems To Close

Observed from `./tui.png` and current TUI implementation:

- The main area shows internal process noise such as `Ready.`, `goal:`,
  `run:`, `step:`, decision creation, user answers, and backend error events.
- Error text can be shown twice: once as stream content and once as `Error:`.
- The top input line exposes a raw Workspace ID and places typing at the top of
  the screen.
- The Workspace panel renders many raw `ws-*` IDs and truncates states into
  unreadable fragments.
- The active Workspace is not understandable enough for a user to confirm where
  commands will run.
- Repeated TUI startup may create too many Workspaces instead of reusing the
  current directory binding.
- Terminal Result display is minimal and Artifact context is not represented.
- ANSI and CJK width handling can produce unstable alignment and truncation.

## Task Status Legend

- `pending`: confirmed requirement, not implemented in this round.
- `in_progress`: implementation underway.
- `implemented`: code has landed, verification not complete.
- `verified`: acceptance checks have passed.
- `blocked`: waiting for design or dependency decision.
- `deferred`: intentionally postponed.

## Delivery Checklist

### TUI-DISPLAY-01. Result-First Main View

Status: verified

Requirement:

- The main view must default to final Result, failure reason, current blocking
  item, or a concise empty state.
- It must not render raw internal event lines such as `run:`, `step:`,
  `goal:`, `answer:`, or `Ready.` by default.
- Detailed event history must remain backend-owned and must not be surfaced as
  a TUI debug view in this round.

Acceptance:

- Initial TUI screen does not contain `Ready.`.
- Submitting a goal does not permanently add `goal:` or raw run IDs to the main
  result area.
- Step events update progress summary only.
- Completed runs show final result content in the main area.

### TUI-DISPLAY-02. Compact Progress Summary

Status: verified

Requirement:

- Replace the visible event stream with a compact status/progress summary.
- Supported states include idle, submitting, running, waiting for decision,
  waiting for approval, completed, failed, and interrupted.
- The summary may include the current step kind or blocking reason, but not a
  scrolling event log.

Acceptance:

- Run state callbacks update a single compact progress area.
- Decision and approval callbacks update the progress state without appending
  noisy history.
- Failed runs show the failure reason once.

### TUI-DISPLAY-03. Bottom Input Area

Status: verified

Requirement:

- Move the input area from the top row to a bottom-fixed area above the help
  bar.
- The input area must remain visible across normal terminal sizes.
- Input context must change clearly for normal goals, decision answers, and
  approval responses.

Acceptance:

- First screen shows the input area near the bottom.
- Window resize does not push the input out of view.
- Decision and approval prompts are handled through the bottom input context.
- The old top-row raw `workspaceID -> input` pattern is gone.

### TUI-DISPLAY-04. Clear Workspace Identity

Status: verified

Requirement:

- The active Workspace must be understandable to the user.
- Prefer title and working directory display, with short ID only as supporting
  context.
- Avoid showing long raw Workspace IDs as the primary label.

Acceptance:

- The active Workspace display includes title or working directory when
  available.
- The active Workspace can be distinguished from other Workspaces without
  reading a full `ws-*` ID.
- Submitted goals use the selected active Workspace.

### TUI-DISPLAY-05. Simplified Workspace Monitor

Status: verified

Requirement:

- The Workspace monitor must not fill the screen with raw `ws-*` IDs.
- Show a bounded, readable Workspace summary list.
- Use clear state labels or icons that do not truncate into meaningless
  fragments.
- Non-active Workspace events update monitor state but must not replace the
  active session result/progress content.

Acceptance:

- The Workspace panel remains readable with many Workspaces.
- Active Workspace is visibly marked.
- Non-active Workspace updates do not pollute the active main view.
- Narrow terminals can degrade to a simpler layout without broken alignment.

### TUI-DISPLAY-06. Workspace Startup Reuse Strategy

Status: verified

Requirement:

- TUI startup must not create a new Workspace unconditionally.
- Startup active Workspace selection priority:
  1. Existing Workspace bound to the current working directory.
  2. Last active Workspace, only when current directory has no binding.
  3. Create or bind a current-directory Workspace only when neither is usable.
- TUI Workspace switching must record the selected Workspace as last active.
- If the last active Workspace was deleted or is inaccessible, fallback must be
  deterministic and visible.

Acceptance:

- Starting `workorch` twice in the same directory does not create duplicate
  Workspaces.
- When a current directory binding exists, it wins over last active Workspace.
- When no current directory binding exists, last active Workspace can be
  restored.
- Switching Workspace in TUI affects the next submitted goal and the next
  startup fallback.

### TUI-DISPLAY-07. Error Display Deduplication

Status: verified

Requirement:

- Errors must be rendered in one clear area.
- Do not append backend errors into a stream and also show them as `Error:`.
- Distinguish command failure, run failure, invalid user input, and blocking
  state.

Acceptance:

- A backend command error appears once.
- A run failure appears once with its failure reason.
- Decision/approval waiting state is not styled as a generic error.
- Clearing or submitting follow-up input removes stale non-terminal errors
  where appropriate.

### TUI-DISPLAY-08. Decision And Approval Current-State Handling

Status: verified

Requirement:

- Decision and Approval must render as current blocking states, not historical
  event lines.
- The prompt/reason must be visible enough for the user to answer inline.
- After resolution, the current blocking state must clear.

Acceptance:

- Decision-created callback changes the input context to decision answer mode.
- Approval-created callback changes the input context to approval mode.
- Resolution callback clears the blocking prompt and resumes normal progress.
- User answers are not kept as visible history noise.

### TUI-DISPLAY-09. Terminal Result And Artifact Context

Status: verified

Requirement:

- Completed runs must show final Result summary in the main area.
- If backend events expose Artifact references, names, or paths, the TUI must
  show concise Artifact context.
- If no Result exists yet, show a concise empty state rather than process
  history.

Acceptance:

- Result-accepted callback updates the main result area.
- Completed state remains visible after run completion.
- Artifact references are visible when present in event payloads.
- Missing Artifact data does not produce misleading placeholder output.

### TUI-DISPLAY-10. Layout Stability

Status: verified

Requirement:

- Avoid ANSI-width and CJK-width layout breakage.
- Use stable sizing for header, progress, result, Workspace monitor, bottom
  input, and help bar.
- Provide a narrow-width fallback layout when the right panel cannot fit.

Acceptance:

- Chinese text truncates cleanly without broken layout.
- Colored text does not shift the Workspace panel alignment.
- Input and help remain visible at common terminal sizes.
- Workspace list truncation preserves meaningful labels.

### TUI-DISPLAY-11. Documentation Sync

Status: verified

Requirement:

- Update the relevant requirement and acceptance documents after the final
  display contract is implemented.
- Remove or revise contract text that still describes the default TUI as an
  event-stream-first view.

Documents:

- `docs/workorch-cli-tui-agent-requirements.md`
- `docs/workorch-tui-interaction-contract.md`
- `docs/workorch-tui-agent-acceptance.md`
- This tracking document.

Acceptance:

- The documents consistently describe result-first default display.
- Acceptance scenarios no longer require detailed event stream lines in the
  default TUI.
- The final implementation status is recorded in this document.

### TUI-DISPLAY-12. Test And Verification Coverage

Status: verified

Requirement:

- Add or update deterministic TUI model tests for the closure scope.
- Keep architecture guards passing.
- Keep verification based on backend callbacks, not polling.

Required coverage:

- Bottom input rendering.
- Compact progress state.
- Result-first main view.
- Workspace startup reuse priority.
- Workspace switching.
- Error deduplication.
- Decision and approval current-state handling.
- Terminal result and Artifact context.
- Stale event ignore.
- Narrow layout behavior.

Expected verification:

```bash
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui --count 1
GOCACHE=/tmp/workorch-go-build make verify-tui-agent
GOCACHE=/tmp/workorch-go-build make cli-test
```

## Out Of Scope For This Round

- A visible detailed event stream or debug event view in TUI.
- A separate TUI business implementation that bypasses the shared backend.
- Fixed-interval polling for state refresh.
- Direct HTTP client usage from TUI, CLI command handlers, or agent flow.
- Reintroducing `--local` daemon-launch mode.
- Rebuilding the web console.

## Acceptance Record

Implementation status: verified

Code locations changed:

- `application/workorch/main.go`
- `application/workorch/main_test.go`
- `internal/modules/application/workorch/biz/config/config.go`
- `internal/modules/application/workorch/service/cli/repl.go`
- `internal/modules/application/workorch/service/tui/model.go`
- `internal/modules/application/workorch/service/tui/model_test.go`
- `docs/workorch-cli-tui-agent-requirements.md`
- `docs/workorch-tui-interaction-contract.md`
- `docs/workorch-tui-agent-acceptance.md`
- `docs/workorch-tui-display-closure-tracking.md`

Tests added or updated:

- TUI model tests now cover result-first rendering, bottom input, decision
  input context, Workspace switch callback, error deduplication, stale
  non-active Workspace events, terminal result, and Artifact context.
- Application startup tests now cover current directory binding priority, last
  active Workspace fallback, and create-only-when-needed behavior.

Verification commands run:

```bash
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/service/tui --count 1
GOCACHE=/tmp/workorch-go-build go test ./application/workorch --count 1
GOCACHE=/tmp/workorch-go-build go test ./internal/modules/application/workorch/... --count 1
GOCACHE=/tmp/workorch-go-build go test ./application/workorch ./internal/modules/application/workorch/... --count 1
GOCACHE=/tmp/workorch-go-build make cli-build
GOCACHE=/tmp/workorch-go-build make verify-tui-agent
GOCACHE=/tmp/workorch-go-build make cli-test
GOCACHE=/tmp/workorch-go-build make verify-cli
```

Known residual risks:

- `make verify-tui-agent` scripted E2E still uses stable marker assertions; it
  does not yet capture a full terminal screenshot diff.
- Artifact display depends on Artifact references being present in backend
  result event payloads.

Follow-up fix 2026-05-03:

- The TUI now renders the submitted input as `Task: ...` context in the main
  view so the current user request remains visible while the bottom input is
  available for follow-up, Decision, or Approval input.
- EventHub backend conversion now copies Decision Ticket and Approval IDs from
  event envelope `SubjectID` into TUI event payloads. This prevents inline
  Decision/Approval responses from submitting empty IDs and failing with
  `invalid argument`.
- Additional tests cover submitted task context rendering, Decision
  `blockingTicketId` parsing, and EventHub subject ID enrichment.
