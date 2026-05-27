# WorkOrch TUI Display And Refresh Consistency Design

## Background

Recent workspace checks showed that backend workspace state, Web state projection,
and TUI local projection can become hard to compare when an old run has terminal
outcome or open blocker records while a newer active run is already running.

The TUI must remain a presentation and interaction surface. It must not introduce
business-specific runtime decisions, must not use local HTTP client polling, and
must consume the shared backend/event/read-model contract.

## Goals

1. Show necessary information without turning the TUI into a raw event dump.
2. Keep main viewport, runtime status bar, input area, and workspace list refreshed
   from the same active workspace/run projection.
3. Preserve good terminal usability: native copy must continue to work, mouse
   reporting remains disabled, and long input/output must not break layout.
4. Make Web/TUI state comparison straightforward by using the same state source
   semantics.

## Non-Goals

- Do not add business-specific rules to the WorkOrch core runtime.
- Do not change runtime state transitions or workspace output artifacts.
- Do not replace EventHub-driven TUI refresh with HTTP polling.
- Do not make TUI responsible for full trace review; detailed trace remains in
  Web/API/files.

## Projection Contract

The TUI projection must use these sources:

- Current workspace state comes from `WorkspaceInfo.State`.
- Current active run comes from `WorkspaceInfo.ActiveRunID`.
- Run status and blocker display are scoped to the active workspace and active run.
- Outcome/result can be displayed as result evidence, but must not override the
  current workspace state.
- Superseded, failed, or completed old runs may appear only as history/summary;
  they must not drive the current status bar, prompt mode, or active blocker.

## Display Contract

### Main Viewport

Default sections:

- Active workspace and template/soul binding summary.
- Goal/conversation and current DoD.
- Compact task decomposition, memory recall, LLM usage/context, runtime events,
  and governance sections.
- Result, artifacts, sources, and patches.
- Last error when present.

Main viewport must remain scrollable with keyboard PgUp/PgDn. Workspace panel must
remain fixed on wide terminals.

### Runtime Status Bar

The status bar sits immediately above the input area. It should include:

- Progress label and progress text.
- Active workspace short id.
- Active run short id when present.
- Current blocker id when waiting for decision/approval.
- New update marker when the user has manually scrolled away from the tail.

The status bar must be single-line and truncated to terminal width.

### Input Area

The input area stays at the bottom. It must show the input semantic mode:

- `goal` when no active run context exists.
- `continue` when normal input will continue/resume an active run.
- `decision` when answering a decision ticket.
- `approval` when resolving approval.

Large multiline paste must be folded in display while preserving the original
submitted content. Markdown-like paste should show both a head preview and a tail
preview.

Plain letter keys and control keys that can be emitted during terminal/input
handling must not quit the default agent session. The TUI must not use bare `q`,
`ctrl+q`, or double `ctrl+c` as quit paths, because Chinese IME composition and
normal text input can emit intermediate runes or control sequences before
committing Chinese text. The only supported quit path is the slash command:
`/exit` (with `/quit` and `/q` aliases accepted by the same command parser).

### Workspace List

Workspace list remains a monitoring sidebar on wide terminals and compact section
on narrow terminals. It should show:

- Active/selected marker.
- Workspace name, disambiguating id when needed.
- Workspace state.
- Active run short id when present.
- Current active workspace blocker counts with an explicit scope label.

## Refresh Contract

TUI refresh remains event-driven:

- Workspace create/delete/list events refresh the workspace list.
- Active workspace events update active status and fetch context when necessary.
- Non-active workspace events update the workspace monitor only.
- Run, step, patch, LLM usage, memory recall, governance, and result events append
  compact progress/runtime-event information.

Viewport follow behavior:

- If the user has not manually scrolled, runtime updates should follow the latest
  content.
- If the user has manually scrolled, runtime updates must not steal the viewport;
  the status bar shows a new update marker.
- Terminal result events may force the viewport to the result/tail so the user can
  see completion.
- `/clear`, workspace switch, and run restart reset the viewport follow state.

## Implementation Tasks

| ID | Priority | Task | Status |
| --- | --- | --- | --- |
| TUI-DRC-01 | P0 | Enforce active workspace/run scoped blocker projection | completed |
| TUI-DRC-02 | P0 | Ensure outcome/result does not override current state display | completed |
| TUI-DRC-03 | P1 | Add viewport follow-vs-user-scroll state and new update marker | completed |
| TUI-DRC-04 | P1 | Structure status bar with workspace/run/blocker context | completed |
| TUI-DRC-05 | P2 | Add active run marker and explicit blocker scope in workspace panel | completed |
| TUI-DRC-06 | P2 | Improve input prompt mode and folded paste preview | completed |
| TUI-DRC-07 | P0 | Add regression tests for stale blocker/outcome and display refresh | completed |
| TUI-DRC-08 | P0 | Prevent bare `q`, `ctrl+q`, and double `ctrl+c` from quitting during normal/Chinese input | completed |

## Verification

Required local verification:

```bash
go test ./internal/modules/application/workorch/service/tui -count=1
```

When frontend assets are also touched in the same change set, run:

```bash
make console-build
```

## Change Log

| Date | Change |
| --- | --- |
| 2026-05-27 | Initial design from Web/TUI state inconsistency review. |
| 2026-05-27 | Implemented TUI projection, viewport follow, status bar, workspace panel, folded input, and bare-key input safety closure. |
