# WorkOrch TUI Interaction Contract

Status: active

Type: product-interface

Last Verified: 2026-08-16

## Layout

The screen contains an active Workspace header, compact progress/status, scrollable result/context area, bounded Workspace
monitor, bottom input and help. Decision/Approval change prompt context; ordinary Run states stay in the status area.

## Bare Text Semantics

Input is evaluated in this order:

| Current context | Bare text behavior |
| --- | --- |
| blocking Decision | resolve the current Ticket with the text |
| blocking Approval | parse approve/reject; keep waiting when ambiguous |
| no active Workspace | show an error |
| active Workspace with active Run | request `resume`; text remains transcript only and is not a Runtime patch |
| active Workspace without active Run | save/submit the text as a new Goal and begin execution flow |

Slash commands take precedence over bare text.

## Commands

Base commands implemented by the current TUI:

```text
/help                 /clear
/config               /model
/model show           /model clear
/model reasoning      /workspace
/workspace create     /goal
/dod                  /status
/coverage             /steps
/timeline             /patches
/artifacts            /memory
/result               /run resume
/run restart          /exit
```

Decision context adds `/decision answer`, `/decision terminate`, `/decision show`, `/decision list`. Approval context adds
`/approval approve`, `/approval reject`, `/approval show`, `/approval list`.

`/run resume` requests continuation of the Workspace-selected current Run. `/run restart` requests a fresh definition-bound
activation; it does not directly create or select a successor.

Normal TUI creation uses `/workspace create <title> --template <template-id> [--dir <path>]`. Template is required; when
`--dir` is omitted, the application resolves and creates the default `.workorch-workspaces/<workspace-id>` directory.

## Keyboard

- `Enter`: submit or confirm selection;
- `Alt+Enter`: insert newline;
- `Tab`: complete slash command;
- `Ctrl+P` / `Ctrl+N`: local input history;
- `PgUp` / `PgDn`: scroll main content;
- `Ctrl+W`: toggle Workspace selector;
- `j` / `k` or arrows: navigate selector/suggestions where applicable;
- `Esc`: close overlay/selector or clear input;
- `Ctrl+C`: interrupt local running display state only;
- `/exit`: quit cleanly.

## Workspace And Scope

- changing selector row does not change the active Workspace until confirmation;
- after confirmation, prompt, blocker, Run and content refresh all use the new Workspace scope;
- late asynchronous data for the old scope cannot update the main view;
- inactive Workspace events update only their cached monitor summary.

## Governance

- blocker prompt derives from the active Run projection and typed Ticket/Approval detail;
- resolution is sent once to the appropriate owner API/backend;
- successful resolution clears the prompt and waits for owner continuation facts;
- the TUI does not submit an extra Resume after resolution.

## Model Selection

`/model` selects an ai-proxy model for the Workspace; `/model reasoning` updates the complete selection with a reasoning effort;
`/model clear` restores system-default inheritance. Writes use expected Workspace revision. The active Run retains its sealed
selection; a changed selection applies to a fresh activation, not an in-place Resume.

## Result And Inspection

`/result`, `/artifacts`, `/patches`, `/steps`, `/timeline`, `/coverage`, `/memory`, `/status` expose compact projection views.
Artifact continuation content and raw provider/event payloads are not expanded by default. These views cannot trigger owner
state transitions.
