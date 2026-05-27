# workorch TUI Agent Interaction Contract (CLI-AGENT-14)

## First-Screen Layout

```
╔══════════════════════════════════════════════════════════╦═════════════╗
║                    WorkOrch Agent                         ║  Workspaces ║
║  Workspace: project  /repo/project                       ║  ▸ project  ║
║  Running - llm/workspace_execution                       ║    ops      ║
║  ──────────────────────────────────────────────────────  ║    docs     ║
║                                                          ║             ║
║  ── Result ──                                            ║             ║
║  天气查询完成：多云 +21°C                                  ║             ║
║                                                          ║             ║
║  ws-project > _                                          ║  0 decisions║
╚══════════════════════════════════════════════════════════╩═════════════╝
```

### Key areas
1. **Top bar**: title + active Workspace title/path context
2. **Progress summary**: compact state such as idle, running, waiting,
   completed, or failed
3. **Result area**: terminal result and Artifact references after completion
4. **Right panel**: bounded all-Workspace monitor + alert counts
5. **Bottom input area**: slash command input; decision/approval answer context
6. **Bottom bar**: keyboard shortcuts help

## Input Area Behavior

- The normal prompt is neutral: `ws-xxx >`.
- Normal-state input must use explicit slash commands such as `/goal`,
  `/run continue`, `/run restart`, `/dod`, `/memory`, or `/result`.
- Plain text outside decision/approval blocker state is rejected with slash
  command guidance; it is not implicitly treated as a goal or run input.
- During decision blocker state the prompt changes to `ws-xxx decision >` and
  plain text is submitted as the current decision answer.
- During approval blocker state the prompt changes to `ws-xxx approval >` and
  plain text is parsed as approval/rejection intent; unclear text asks the user
  to use `/approval approve` or `/approval reject`.
- The input area is fixed near the bottom of the terminal, above the help bar.
- Press Enter to submit.
- Press Alt+Enter to insert a newline in the input buffer.
- Press Ctrl+P / Ctrl+N to navigate local input history.
- Press Tab while typing a slash command to complete the command name.
- Press Ctrl+C to interrupt the active Run.
- Press ESC to clear current input.
- `/` commands supported in the default TUI: `/help`, `/clear`,
  `/workspace`, `/workspace create`, `/goal`, `/dod`, `/result`, `/memory`,
  `/run continue`, `/run restart`, `/exit`.
- `/run continue [message]` resumes the current active Run.
- `/run restart [message]` starts a new Workspace execution with the existing
  cumulative Goal and DoD definitions. The optional message is injected as
  operator dialog input for the new Run.
- `/dod item1; item2` sets the pending DoD for the next submitted Goal.
  If no custom DoD is set, the next Goal uses the active Workspace template
  DoD when the Workspace was initialized from a matching template; otherwise
  it uses the generic default DoD.
- `/workspace create <title> --template <templateId> [--dir <path>]` creates
  a Workspace and applies the template only at creation time. After creation
  the TUI switches to that Workspace and refreshes the Workspace list.
- TUI does not provide a normal workspaceTemplate rebind command. Template
  selection is a Workspace creation-time concern; existing Workspace
  template migration must be handled as a separate advanced migration flow.

### Slash Command Contract

| Command | Usage |
|---------|-------|
| `/help` | Show the TUI help overlay. |
| `/clear` | Clear current TUI display state. |
| `/workspace` | Toggle workspace selection mode. |
| `/workspace create <title> [--template <id>] [--dir <path>]` | Create and switch to a workspace. |
| `/goal <text>` | Define a new goal and activate execution. |
| `/dod [items]` | Show current DoD when empty, otherwise persist DoD items. |
| `/result` or `/refresh` | Refresh the current workspace result/context. |
| `/memory` or `/mem` | Show memory recall summary. |
| `/run continue [message]` | Resume the selected active run, optionally with operator input. |
| `/run restart [message]` | Start a new run using the current cumulative Goal/DoD, optionally with operator input. |
| `/decision show` or `/decision list` | Show current/listed decision tickets. |
| `/decision answer <text>` | Resolve current decision with an answer. |
| `/decision replan [note]` | Resolve current decision by requesting replan. |
| `/decision terminate [note]` | Resolve current decision by terminating the current path. |
| `/approval show` or `/approval list` | Show current/listed approvals. |
| `/approval approve` | Approve current approval request. |
| `/approval reject` | Reject current approval request. |
| `/exit` | Quit the TUI. No bare `q`, Ctrl+Q, `/q`, or `/quit` exit path is supported. |

## Progress Summary Display

The default TUI does not render a detailed event stream. Detailed run, step,
decision, approval, and provider stream events remain backend records only.
The visible progress area is a compact summary:

| State | Display |
|-------|---------|
| Idle | `Idle` |
| Submitting | `Submitting - Submitting goal` |
| Running | `Running - <current step kind>` |
| Decision required | `Waiting for decision - <question>` |
| Approval required | `Waiting for approval - <reason>` |
| Completed | `Completed - Completed` |
| Failed | `Failed - <failure reason>` |
| Interrupted | `Interrupted - Interrupted` |

The progress summary is updated by EventHub callbacks and must not be refreshed
by polling.

## Decision/Approval Inline Handling

When a decision arrives, the current blocking prompt is shown in the main area
and the bottom input area changes context:
```
  Decision required
  How should I query the weather?

  Decision answer > _
```
User answer is sent via Enter. No modal dialog.

## Workspace Monitor (Right Panel)

- Lists all Workspaces with state icons.
- Active workspace highlighted.
- Up/Down arrows switch active workspace.
- Alert counts for blocking items.
- Workspace labels prefer title or working directory, with short ID only as
  supporting context.
- The monitor is bounded and must not fill the screen with long raw Workspace
  IDs.
- The main Workspace context may show default soul and stage binding summaries
  as read-only template initialization evidence.

## Keyboard Actions

| Key | Action |
|-----|--------|
| Enter | Submit input |
| Alt+Enter | Insert newline |
| Ctrl+C | Interrupt active Run |
| ESC | Clear input |
| Ctrl+W | Enter/leave Workspace selection |
| Ctrl+H | Toggle help |
| Ctrl+P / Ctrl+N | Previous/next input history |
| Tab | Complete slash command |
| PgUp / PgDn | Scroll main result area |
| `/exit` | Quit |

Mouse drag is reserved for the terminal emulator's native text selection and
copy behavior. The TUI must not enable Bubble Tea mouse reporting by default,
because that captures drag events and prevents selecting visible Result,
Conversation, and Workspace text.

## Interrupt Semantics

- Ctrl+C during a Run: cancel the Run context.
- Progress summary shows "Interrupted.".
- Result area shows partial output if available.
- Agent returns to input prompt.

## Post-Run Display

After Run completes:
- Progress summary shows completed or failed state.
- Result area shows SummaryDetail.GoalOutcome or the accepted Result summary.
- Markdown-like final Result / Artifact text is rendered in a lightweight
  terminal-friendly form for headings, bullets, and code blocks.
- Artifact references are displayed when provided by backend result events.
- Input prompt returns for next goal.
- The same Workspace remains active.

## State Model

```
Idle → Submitting → Running → (Decision/Approval) → Running → Completed/Failed → Idle
  ↑                                                         |
  └──────────────────── Ctrl+C interrupt ────────────────────┘
```

## Rendering Notes

- All state arrives via EventHub callbacks (no polling).
- Workspace list updates when `EventWorkspaceStateChanged` fires.
- Progress summary updates on `EventRunStateChanged`, `EventStepDispatched`,
  blocking item, and terminal result callbacks.
- Right panel alert counts update on decision/approval events.
- Input cursor remains in the bottom input area and changes prompt for
  decision/approval inline modes.
