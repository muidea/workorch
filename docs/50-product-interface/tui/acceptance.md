# WorkOrch TUI Agent Acceptance

Status: normative

Type: verification-contract

Last Verified: 2026-08-21

## Scope

Acceptance covers default embedded TUI, shared local backend, remote backend parity, interaction semantics, scoped projection
refresh and terminal rendering. It does not use TUI state as proof of Runtime authority.

## Static Guards

- TUI does not import/use `workorchclient` directly;
- local TUI does not start a workorchd child or depend on a local HTTP listener;
- TUI/agent code has no fixed polling ticker;
- CLI/TUI operations go through the shared backend ports;
- default view is result-first and does not render raw event lines.

## Model Tests

Tests must cover:

- current-directory, last-active and no-selection startup cases;
- Workspace selector confirmation and stale old-scope response rejection;
- bare-text Goal, active-Run Resume, Decision answer and Approval ambiguity handling;
- runtime-definition clarification禁用bare-text/Decision answer/Resume，只允许Goal/DoD修订后显式Restart；
- all implemented slash commands and model-selection CAS behavior;
- Resume versus Restart admission and fresh-activation guidance;
- local `Ctrl+C` display interruption without Run mutation;
- progress/blocker/terminal projection changes, duplicate events and errors;
- coverage/assessment/evidence三轴状态、`indeterminate`非成功、recoverable wait与governance blocker分流、budget摘要；
- viewport scrolling, follow-tail marker, CJK/ANSI layout and native mouse copy;
- inactive Workspace monitor refresh without active-view pollution.

## Backend And Transport Tests

- local EventHub notifications and remote SSE cause equivalent scoped refresh;
- reconnect/gap reads current projection instead of replaying events into business state;
- concurrent commands correlate by request/workspace/run identity;
- Decision/Approval resolution uses typed owner APIs and does not auto-Resume;
- runtime-definition clarification不会调用通用Decision resolution；TUI不代发replacement-resolution command；
- subscription/session cleanup occurs on shutdown.

## Scripted Entry Points

```bash
make cli-test
make verify-cli
make verify-tui-agent
make verify-tui-workspace-switch-visual
make verify-tui-terminal-render
make verify-tui-long-output
```

`verify-tui-agent` currently checks static guards, TUI/backend/agent Go tests, default non-TTY guidance, absence of the legacy
REPL, absence of a server child, keyboard viewport behavior, Workspace live refresh and Memory summary refresh.

Live SSE/restart history and Web/TUI parity have separate explicit targets; they require a deployed service and must record the
tested revision/config.

## Acceptance Matrix

| Scenario | Required result |
| --- | --- |
| startup | default mode is TUI; non-TTY explains `--server`; no silent Workspace creation |
| goal | bare text with selected Workspace/no active Run submits a Goal |
| resume | bare text or `/run resume` requests current Run continuation without context patch |
| restart | `/run restart` sends workspace intent; UI does not choose successor |
| governance | active blocker controls prompt; resolution clears prompt and waits for owner facts |
| definition clarification | static ticket只接受corrected candidate；Runtime ticket只引导revise-and-restart，零one-of confirm、generic answer、Resume或client-built replacement binding |
| recoverable wait | status shows typed reason/condition/deadline/budget; no Decision prompt、auto-Resume、force retry or local countdown authority |
| coverage | requirement/CWI三轴保持独立；partial/indeterminate/stale evidence不显示为完成 |
| switch | prompt/content/blocker all change scope; old async responses are ignored |
| terminal | formal Result/failure/artifact summary appears once for current revision |
| interrupt | `Ctrl+C` changes local display wait only; no cancel/supersede command is emitted |
| remote | HTTP/SSE latency does not change business semantics |

## References

- [TUI interaction contract](interaction-contract.md)
- [Display consistency](display-refresh-consistency.md)
