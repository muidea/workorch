# WorkOrch TUI Requirements

Status: active

Type: product-interface

Last Verified: 2026-08-16

## Product Shape

- TUI is the default binary mode and provides a result-first local agent experience.
- local mode embeds the WorkOrch modules and uses the shared backend/EventHub; it does not launch or call a local HTTP server.
- `--remote <base-url>` uses the HTTP/SSE remote backend behind the same port interfaces.
- TUI submits Workspace, execution and governance intents; it never owns Runtime or resource state.

## Workspace Selection

At startup the process tries, in order:

1. a Workspace whose deliverable directory equals the current working directory;
2. the configured last-active Workspace;
3. no selection, after which the user selects or creates a Workspace.

The TUI must not silently create a Workspace merely because neither lookup succeeds.

## Display Requirements

- active Workspace identity, Goal/DoD and current Run/blocker are always scoped together;
- main content prioritizes current progress, formal Result, artifacts, failures and actionable governance;
- all-Workspace monitor remains bounded and cannot overwrite the active Workspace view;
- detailed steps/tool/provider/audit records are opt-in inspection views, not the default screen;
- long content is scrollable and preserves native terminal copy behavior.

## Input Requirements

- bottom input supports slash commands and context-sensitive bare text;
- Decision text resolves the current ticket; Approval text must parse unambiguously as approve/reject;
- bare text with an active Run requests Resume without turning the text into an authority patch;
- bare text with a selected Workspace and no active Run is a new Goal;
- explicit `/run restart` is required for a fresh activation.

## Refresh Requirements

- local updates use backend/EventHub notifications; remote updates use SSE plus scoped projection refresh;
- no fixed interval `time.Ticker`, `time.Tick` or `tea.Tick` state polling;
- command admission is not final state; owner projection revision is the display fact;
- stale responses from a previously selected Workspace are discarded.

## Interrupt Boundary

`Ctrl+C` while the TUI marks work as running only interrupts the local display/wait state. It does not send Run cancellation,
supersede or termination to any owner. When not locally running, it instructs the user to use `/exit`. Formal Run terminal
states can only arrive from owner facts.

## Non-Goals

- no daemon-launching `--local` mode;
- no CLI-only lifecycle implementation;
- no raw event-stream-first main UI;
- no client-selected target Run state or execution-context patch.

Acceptance is defined in [tui-agent-acceptance.md](../../60-verification/tui/tui-agent-acceptance.md).
