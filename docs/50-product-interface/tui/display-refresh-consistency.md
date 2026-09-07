# WorkOrch TUI Display And Refresh Consistency

Status: normative

Type: product-interface

Last Verified: 2026-08-23

## Display Source

TUI renders shared projection contracts: Workspace list/overview, selected Workspace detail/RuntimeView, Run, blocker,
coverage/progress, Result/artifacts and inspection pages. It does not join owner repositories or infer state from raw events.

Terminal or superseded historical Runs can appear in history but cannot drive the active status bar or prompt.

Workspace/Run canonical owner state 与 lifecycle handoff 必须分栏：Workspace `state/activeRunId`、Run `state/blocker refs`
原样显示；Workspace 无 blocker 字段。`GovernanceDisplay` 只提示跨 owner handoff，`ConsistencyDiagnostic` 只提示偏差，
`DisplayOutcome` 只展示非权威结果且不能替代 formal Result。TUI 不得从这些 display 字段构造/选择 Resume、Restart、Drive
或治理 resolution target，也不能因 display-only handoff 抑制 Resume；`GovernanceDisplay.DecisionID|ApprovalID|RunID` 即使与
owner resource 冲突也不能过滤、隐藏或改写 target。治理 mutation 只使用 canonical Workspace `activeRunId` 对应 Run 的
matching blocked state + exact blocker ref 与已加载 concrete open Decision / pending Approval 的交集；同 Run 的其他未决资源
不能作为 first-open fallback。Workspace owner 已投影 `blocked_on_approval + activeRunId`、但 Approval resource 尚未到达时，
TUI 只能刷新 owner read model；不得把普通输入或 Resume 送入另一条 mutation 路径，也不得把 Approval conflict 从终态读回
重判为成功。

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

Runtime progress 必须明确区分整个 Run 与当前 focus 的口径。`runEvidence` 汇总当前 Run accepted/rejected ledger 的
全部 evidence 和 validation，与正式 Result 的累计口径一致；legacy flat `acceptedEvidenceCount`、
`acceptedValidationEvidenceCount`、`rejectedEvidenceCount` 保持当前 focus diagnostic lineage 语义，其中 accepted
validation 还要求 CWI credit。durable effects、satisfied CWI 与 dispatch 是全 Run 指标，不能与 focus validation 数量
混排成同一累计摘要。旧 producer 未提供 `runEvidence` 时显示缺省，不把未知累计数填为零。TUI 与 Web 共用的
Runtime summary 必须标注 `whole Run` / `current focus`；scope 切换、统计和 terminal owner cause 变化均触发显示刷新，
不重算 acceptance 或迁移任何 Runtime 状态。

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
