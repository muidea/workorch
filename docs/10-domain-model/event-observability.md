# Event And Observability Domain Model

Status: normative

Type: domain-model

Last Verified: 2026-08-26

## Event Record

`blocks/eventstream` 是 immutable EventRecord 的唯一 owner。当前 record contract 包含：

- `ID`、`Version`、`Topic`、`OccurredAt`；
- source module 与 subject type/ID；
- Workspace/Run/Step scope；
- source revision；
- owner-issued全局单调`StreamPosition`与workspace-local revision；`OccurredAt`不参与cursor顺序；
- typed payload、redacted/compact summary 与可选 payload reference。

EventHub message 是 owner 间同步 typed command/result；EventRecord 是可持久化审计事实。二者用途不同，不能把
Eventstream 当作命令总线或 Runtime 恢复源。

`StreamPosition`允许预留空洞但在 restart/purge 后不复用；Workspace owner 的 deletion aggregate
要求 Eventstream 最后封存。retain 只隐藏该 Workspace 的 records/high-watermark；purge 可物理删除其 audit records
并清理 workspace-local read/high-watermark material，但永久 tombstone 与 revision floor 必须保留，不能授权 local revision
重置或复用，也不能回退全局 position 或影响其它 owner files。
file-backed physical purge 必须绑定完整 persisted Eventstream fence，并以 owner-private durable journal 完成 crash-safe generation
swap：安装点之前只允许 exact rollback，安装点之后只允许 roll-forward；所有 EventRecord 读写入口先恢复或 fail-closed。
journal、replacement、backup 与 deterministic pending 必须限制在 canonical root 的受管路径，逐次 rename/phase 持久化并同步
相关 parent。corrupt、stale、路径越界、symlink、歧义或无 journal 的受管目录不能被猜测恢复；非目标 Workspace 的 record、
external payload、delete marker 与全局 position 必须逐字节保留。
query/high-watermark/seal/purge 均使用 concrete bounded result；
Post不执行强写，payload在append与read两端脱敏和限界。

## Projection

workorchd 从 typed owner reads 与 owner events维护 Workspace overview、Run detail、coverage、task progress、timeline、
result/artifact、memory recall 和 governance 等展示模型。projection 可以失效后重建，并使用 owner revision/CAS fence
抵御乱序刷新。

coverage read model必须分别展示coverage、assessment和evidence状态；`partially_covered`、`indeterminate`与
`stale_or_superseded`不能被一个progress percentage或overall status覆盖。每项投影绑定workspace/run/Definition、Runtime
state revision和实际依赖的owner revision set/hash。稳定对象与派生规则见
[Result Closeout And Requirement Projection](result-closeout.md)。

projection 不拥有：

- Workspace/Run/Result/Decision/Approval lifecycle；
- Orchestrator authority/focus/candidate/evidence；
- capability operation acceptance；
- restart successor creation。

`progress`、派生 action label 或 UI prompt mode 只用于展示，不得回传为状态转换指令。

## SSE And TUI Refresh

- remote consumer通过 SSE 获取变更通知，再按 scope 刷新 projection；断线后用 owner snapshot 恢复。
- embedded TUI使用同一 process EventHub 获取通知，仍以 backend projection 为显示事实。
- duplicate、late 或 missing display event 可以导致刷新，但不能改变业务 authority。
- `/workspace/deleted` 是 Workspace deletion aggregate 完成且 Eventstream 已永久封存后的 transport-only invalidation
  receipt；它不重新打开 Eventstream、也不追加 EventRecord。在线消费者收到后刷新/移除 view，断线重连以 Workspace
  owner 的 list/get tombstone visibility 为准，不能依赖该通知恢复删除 authority。

## Outcome Boundary

- formal Result 由 Result owner保存。
- Workspace/Run outcome view 是 projection。
- accepted evidence 和 terminal outcome 由 Orchestrator裁决。
- audit event 只能证明某个事实被记录，不能单独证明 DoD 已满足。

## Operational Observability

服务可暴露 health、metrics、execution admission、provider request audit 和 cache diagnostics。这些信号用于运行诊断
和验证，不参与 reducer 输入，除非相应 owner 通过明确 typed contract 将事实提交给 Orchestrator。

observability可以展示recoverable wait的reason、condition、next eligible drive/deadline和budget guard，以及
`RuntimeExecutionBudgetView`各dimension的limit、confirmed、unknown/reserved、remaining和exhausted。metrics/provider billing
只能用于观察，不能修正消费ledger、解除wait或驱动Runtime。

## Invariants

- Event payload 必须有稳定 topic、scope、identity 和必要 revision。
- 敏感值在 EventRecord/projection/API 输出前按合同脱敏。
- read-model rebuild 不产生业务写命令。
- recovery 读取 Run 持久化的 aggregate，而不是重放 UI、SSE 或 Eventstream。
