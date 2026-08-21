# WorkOrch TUI Display And Refresh Consistency

Status: normative

Type: product-interface

Last Verified: 2026-08-18

## Display Source

TUI renders shared projection contracts: Workspace list/overview, selected Workspace detail/RuntimeView, Run, blocker,
coverage/progress, Result/artifacts and inspection pages. It does not join owner repositories or infer state from raw events.

Terminal or superseded historical Runs can appear in history but cannot drive the active status bar or prompt.

## Refresh Flow

```text
owner fact changes
  -> local EventHub notification or remote SSE event
  -> mark scoped projection dirty
  -> fetch/merge current owner projection revision
  -> render active view or inactive monitor cache
```

- command responses show admission/provisional feedback only;
- authoritative display follows the latest accepted projection revision;
- duplicate events may coalesce into one refresh;
- a gap/reconnect triggers snapshot refresh, not event replay into TUI state;
- no fixed interval HTTP polling or `time.Tick`/`time.Ticker`/`tea.Tick` refresh loop.

## Scope And Revision Rules

- every async result must match its Workspace and, where applicable, Run;
- switching Workspace invalidates outstanding active-view requests for the old scope;
- lower/stale revisions cannot overwrite newer display state;
- inactive Workspace changes update only the bounded monitor/cache;
- projection missing optional sections does not authorize reuse from another Run.

## Main View

The main viewport can include Goal/DoD, progress, Runtime/governance summaries, Result, artifacts, sources, patches, memory and
LLM usage/context. It supports `PgUp`/`PgDn`, tracks whether the user follows the tail, and surfaces a new-update marker when
the user has scrolled away.

coverage、assessment和evidence使用三个独立projection字段；单一progress percentage不能覆盖`uncovered`、
`partially_covered`、`indeterminate`或`stale_or_superseded`。recoverable wait与governance blocker、terminal failure分开展示，
并绑定同一Runtime state revision；execution-budget view只读展示confirmed/unknown/remaining，不能成为本地倒计时authority。

## Local Interrupt

`Ctrl+C` changes only local `running/progress` display fields. A later owner notification or explicit refresh may show that the
Run remains active. TUI must never project the local interrupted label as Run `cancelled` or `superseded`.

## Terminal Usability

- native mouse copy remains available;
- ANSI/CJK width and truncation remain stable;
- Workspace panel, input and help stay visible at supported terminal sizes;
- failure reason and terminal Result are rendered once per accepted revision rather than appended on every duplicate event.

See [interaction-contract.md](interaction-contract.md) and
[TUI acceptance](acceptance.md).
