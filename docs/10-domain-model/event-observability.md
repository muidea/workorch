# Event And Observability Domain Model

Status: active

Type: domain-model

Last Verified: 2026-08-16

## Event Record

`blocks/eventstream` 是 immutable EventRecord 的唯一 owner。当前 record contract 包含：

- `ID`、`Version`、`Topic`、`OccurredAt`；
- source module 与 subject type/ID；
- Workspace/Run/Step scope；
- source revision；
- typed payload、redacted/compact summary 与可选 payload reference。

EventHub message 是 owner 间同步 typed command/result；EventRecord 是可持久化审计事实。二者用途不同，不能把
Eventstream 当作命令总线或 Runtime 恢复源。

## Projection

workorchd 从 typed owner reads 与 owner events维护 Workspace overview、Run detail、coverage、task progress、timeline、
result/artifact、memory recall 和 governance 等展示模型。projection 可以失效后重建，并使用 owner revision/CAS fence
抵御乱序刷新。

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

## Outcome Boundary

- formal Result 由 Result owner保存。
- Workspace/Run outcome view 是 projection。
- accepted evidence 和 terminal outcome 由 Orchestrator裁决。
- audit event 只能证明某个事实被记录，不能单独证明 DoD 已满足。

## Operational Observability

服务可暴露 health、metrics、scheduler admission、provider request audit 和 cache diagnostics。这些信号用于运行诊断
和验证，不参与 reducer 输入，除非相应 owner 通过明确 typed contract 将事实提交给 Orchestrator。

## Invariants

- Event payload 必须有稳定 topic、scope、identity 和必要 revision。
- 敏感值在 EventRecord/projection/API 输出前按合同脱敏。
- read-model rebuild 不产生业务写命令。
- recovery 读取 Run 持久化的 aggregate，而不是重放 UI、SSE 或 Eventstream。
