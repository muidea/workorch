# WorkOrch TUI Interaction Contract

Status: normative

Type: product-interface

Last Verified: 2026-08-23

## Mode And Authority

TUI 是默认 binary mode。local mode embeds WorkOrch units and shared backend/EventHub，不启动或调用本地 HTTP server；
`--remote <base-url>` 通过同一 port interfaces 使用 HTTP/SSE backend。两种模式只提交 Workspace、execution、resource
和 governance intent并消费 projection；TUI 不拥有 Runtime、Run lifecycle 或 resource state。

启动选择顺序为：current directory 精确匹配的 Workspace、configured last-active Workspace、无选择。前两项均未命中时
必须等待用户选择或创建，不能静默创建 Workspace。

## Layout

The screen contains an active Workspace header, compact progress/status, scrollable result/context area, bounded Workspace
monitor, bottom input and help. Decision/Approval change prompt context; ordinary Run states stay in the status area.

## Bare Text Semantics

Input is evaluated in this order:

| Current context | Bare text behavior |
| --- | --- |
| runtime-definition clarification | keep the typed blocker and show revise/restart remediation; bare text emits no resolution or Resume |
| blocking Decision | resolve the current Ticket with the text |
| blocking Approval | parse approve/reject; keep waiting when ambiguous |
| no active Workspace | show an error |
| active Workspace with active Run | request `resume`; text remains transcript only and is not a Runtime patch |
| active Workspace without active Run | edit/submit a new Definition admission candidate; do not bypass all-of admission or auto-start |

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

local/remote backend 共用一个 execution admission classifier：两类 owner outcome 显示 accepted，其他五类显示 rejected，
并原样显示 `state`、exact `outcome`、`admissionReason` 与 `diagnostic`；caller `requestReason` 只可单列，不能参与分类。
remote HTTP 409 不得被 TUI 重判为 transport failure 或 governance/progress transition。TUI refresh equality 覆盖
runCommand 的全部 canonical 字段与互斥时间戳，不因相同 revision 丢失 owner fact 变化。
TUI 不发送 caller Run priority。

Normal TUI creation uses `/workspace create <title> --template <template-id> [--dir <path>]`. Template is required; when
`--dir` is omitted, the application resolves and creates the default `.workorch-workspaces/<workspace-id>` directory.
Goal、DoD 和 template admission完成且 activation eligible 后，用户仍需显式执行 intent。Static clarification必须显示
typed violation 与 `BlockingTicketID`；TUI 不提供 one-of branch selection 或“确认后放行”。

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

Run-scoped runtime-definition clarification是专用例外：prompt显示gap、offending refs与`revise_definition_and_restart`，不把裸文本、
`/decision answer`、batch resolution或`/run resume`解释为合法resolution。用户必须用`/goal`、`/dod`形成新的all-of candidate，
待admission eligible后显式`/run restart`；TUI不关闭ticket、不构造replacement binding，也不预创建successor。pre-seal
clarification同样只能通过修正candidate收口，不能“确认one-of”。

## Model Selection

`/model` selects an ai-proxy model for the Workspace; `/model reasoning` updates the complete selection with a reasoning effort;
`/model clear` restores system-default inheritance. Writes use expected Workspace revision. The active Run retains its sealed
selection; a changed selection applies to a fresh activation, not an in-place Resume.

## Result And Inspection

`/result`, `/artifacts`, `/patches`, `/steps`, `/timeline`, `/coverage`, `/memory`, `/status` expose compact projection views.
Artifact continuation content and raw provider/event payloads are not expanded by default. These views cannot trigger owner
state transitions.

`/coverage`按requirement/CWI分别展示coverage、assessment和evidence；`partially_covered`或`indeterminate`不能渲染为
完成。`/status`可以展示recoverable wait的safe reason、condition、next eligible drive/deadline和budget摘要，但不把它
当作Decision/Approval prompt，也不提供强制retry、clear wait、补预算或自动Resume。`/result`展示closeout lineage、
accepted evidence、artifact、known limitations和applicable owner revision摘要。

## Non-Goals

- no daemon-launching `--local` mode or CLI-only lifecycle implementation；
- no raw-event-stream-first main UI；
- no caller-selected Run state、successor identity or `ExecutionContextPatch`；
- no fixed polling loop or display state reflected back into owner state。
