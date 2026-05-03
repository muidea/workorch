# workorch TUI Agent Interaction Contract (CLI-AGENT-14)

## First-Screen Layout

```
╔══════════════════════════════════════════════════════════╦═════════════╗
║                    WorkOrch Agent                         ║  Workspaces ║
║  ws-xxx ▸ _                                              ║  ▸ ws-xxx   ║
║                                                          ║    ws-yyy   ║
║  ── Event Stream ──────────────────────────────────────  ║    ws-zzz   ║
║  run: run-xxx  |  ⏳ step: shell/command                  ║             ║
║  ⚠ Decision: provide API key                             ║  ⚠ 1 open   ║
║  ✓ Done (15s)                                            ║  ⚡ 1 pending║
║                                                          ║             ║
║  ── Result ──                                            ║             ║
║  天气查询完成：多云 +21°C                                  ║             ║
╚══════════════════════════════════════════════════════════╩═════════════╝
```

### Key areas
1. **Top bar**: title + active workspace ID
2. **Input area** (row 2): `ws-xxx ▸ [cursor]` — user types goals here
3. **Event stream** (rows 3-8): scrollable live agent progress
4. **Result area** (rows 9-12): terminal result after run completes
5. **Right panel**: all-workspace monitor + alert counts
6. **Bottom bar**: keyboard shortcuts help

## Input Area Behavior

- Accepts free-text goals (same as current REPL).
- Press Enter to submit.
- Press Ctrl+C to interrupt the active Run.
- Press ESC to clear current input.
- `/` commands supported: `/quit`, `/help`, `/new`, `/retry`.

## Event Stream Display

Each event appears as a non-scrolling entry:

| Event | Display |
|-------|---------|
| Run created | `run: run-xxx` |
| Step dispatched | `⏳ step: shell/command [args]` |
| Step completed | `✓ step: shell/command (2s)` |
| Decision required | `⚠ Decision: question text` |
| Approval required | `⚡ Approval: reason text` |
| Run completed | `✓ Done (15s)` |
| Run failed | `✗ error message` |

Events scroll up as new ones arrive. At most 20 visible entries.

## Decision/Approval Inline Handling

When a decision arrives, the input area changes context:
```
  ⚠ Decision: How should I query the weather?
  ▸ [type your answer here]
```
User answer is sent via Enter. No modal dialog.

## Workspace Monitor (Right Panel)

- Lists all Workspaces with state icons.
- Active workspace highlighted.
- Up/Down arrows switch active workspace.
- Alert counts for blocking items.

## Keyboard Actions

| Key | Action |
|-----|--------|
| Enter | Submit input |
| Ctrl+C | Interrupt active Run |
| ESC | Clear input |
| Ctrl+W | Switch to next Workspace |
| Ctrl+T | Jump to next blocking item |
| Ctrl+R | Show recent Run results |
| Ctrl+H | Toggle help |
| Ctrl+Q / q | Quit |

## Interrupt Semantics

- Ctrl+C during a Run: cancel the Run context.
- Event stream shows "Interrupted.".
- Result area shows partial output if available.
- Agent returns to input prompt.

## Post-Run Display

After Run completes:
- Event stream shows final entry: `✓ Done (15s)` or `✗ error`.
- Result area shows SummaryDetail.GoalOutcome.
- Step outputs (shell/mcp) displayed below the summary.
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
- Event stream entries added on `EventRunStateChanged`, `EventStepDispatched`, etc.
- Right panel alert counts update on decision/approval events.
- Input cursor always visible except during decision/approval inline mode (where it changes context).
