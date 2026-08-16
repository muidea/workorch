# Runtime Owner Convergence Verification Matrix

Status: active

Type: verification-contract

Last Verified: 2026-08-16

## Purpose

本文把已经稳定的 CWI、single-owner、candidate、recovery 和 terminal-delivery 回归要求放入验证层。它不是第五个
状态机，也不定义实施顺序；Runtime 语义仍以
[核心合同](../00-core/runtime-stage-contract.md) 和 Orchestrator README 为准。历史逐反例收口过程见
[归档 CWI 计划](../90-archive/completed/runtime-cwi-reconciliation-single-owner-closure-plan.md)。

## Owner Matrix

| 状态/资源 | 唯一 Owner | 合法输入 | 必须拒绝 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle / replacement intent | `blocks/run` | Workspace-selected activation/continuation intent、governance/terminal fact | caller target state、Orchestrator/Application 代写 lifecycle |
| Workspace definition / active Run slot | `blocks/workspace` | Workspace-private definition/execution event、sealed terminal delivery | projection/Run notification 自动释放、外部 activeRun setter |
| Result lifecycle | `blocks/result` | sealed `RuntimeTerminalDeliveryFact` | generic Result/Artifact entity write、重新裁决 acceptance |
| SM-2 authority/focus/CWI | `kernel/orchestrator` | persisted facts、typed owner observation、private internal event | capability/LLM/Application 选择 focus、credit、repair 或 terminal |
| SM-3 semantic/candidate/dispatch | `kernel/orchestrator` | sealed request session、single evaluator、typed owner receipt | 未 seal 执行、attempt 2、错误文本 correction |
| SM-4 drive/recovery/delivery ledger | `kernel/orchestrator` | persisted aggregate、lease、delivery acknowledgement | projection rebuild authority、跨 Owner 状态写入 |
| Capability effect/session | 对应 `llm/file/git/shell/ssh/web/mcp` Block | Owner 自有 concrete command/result | 通用 executor、Orchestrator-defined capability wire、跨 Owner repository |
| Audit/projection | `eventstream` / Workorchd projection | immutable notifications、high-watermark | 反向修复 owner state 或触发 Runtime transition |

## Required Regression Dimensions

### Admission and CWI

- Definition declaration、presentation slice、eligible closure、semantic order 与 generation binding 必须分离。
- unbound focus 只能从 exact eligible closure 选择；settled CWI 只保留 audit lineage，重新执行必须先有 typed reopen。
- dependency settlement 改变 eligible closure 时，旧 semantic generation 必须在同一 Owner CAS 中退休。
- binding、manifest、candidate target、operation kind/variant、owner catalog revision 与 receipt proof 必须逐字段匹配。
- raw candidate CWI、assessment claim、文件存在、Git revision、exit 0 或非空 `PassedChecks` 都不能产生 credit。

### Candidate and Capability

- complete document 只进入一个 evaluator；结构 rejection 先于 owner observation，attempt 只允许 0/1。
- owner feasibility 的 `stale | prerequisite_required | infeasible` 不占 candidate attempt，也不由 message 选择后继。
- `file/write` 只允许 `prospective_owner_target`；existing target replacement 必须使用 owner-published、
  revision-bound `file/edit` 或 `patch/apply` variant。
- workspace-sensitive recipe catalog 必须绑定 File manifest/observation 的 exact tree revision；Shell 只发布当前适用
  recipe，空 recipe set 不得向 semantic provider 暴露 `validation/managed_session`。File tree change 只退休依赖相交
  catalog，duplicate re-drive/invalidation 不增加 refresh budget。
- capability execute command/result 必须来自接收 Owner 的 `pkg/events`；receipt 只返回 factual outcome/effect/evidence。

### Idempotency, Restart and Budgets

- duplicate event 不增加 revision/CAS、attempt、request、dispatch、effect、evidence 或 delivery acknowledgement。
- stale scope/definition/authority/dispatch/fence/result 只能进入 typed diagnostic 或明确拒绝。
- provider recovery、candidate attempt、repair、prerequisite refresh、operation recovery、CAS reload 和 delivery retry
  预算互不借用。
- crash 点覆盖 request seal 前后、candidate seal 前后、owner effect 返回前后、evidence CAS 前后和每段 terminal
  delivery；exact receipt replay 不重放 side effect。
- non-current development data 的 Resume/Restart 分别返回 `fresh_activation_required` / `data_reset_required`，且零
  provider/capability/replacement work。

### Terminal and Projection

- complete/failed 只能来自 sealed SM-2 terminal；cancelled/superseded 必须有 Run-owned administrative intent/fact。
- structured closeout 只能包含 current unsatisfied CWI 的 canonical gap；当前 `Facts.Failure` 必须优先于 active/formal
  gap，已满足 CWI 的 claimed gap 不得污染 top failure、missing evidence 或 target refs。
- delivery 固定为 `Result -> Workspace -> Run`；单段失败只保留 delivery pending，不改写 terminal。
- Eventstream、projection、HTTP/SSE/TUI 不得重建 authority或隐式 Resume。

## Gates

至少运行：

```bash
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/blocks/run/... ./internal/modules/blocks/workspace/... ./internal/modules/blocks/result/... -count=1
go test ./internal/modules/blocks/llm/... ./internal/modules/blocks/file/... ./internal/modules/blocks/git/... ./internal/modules/blocks/shell/... ./internal/modules/blocks/ssh/... ./internal/modules/blocks/web/... ./internal/modules/blocks/mcp/... -count=1
make verify-boundary-contract
make test-race
make build
```

部署态正向闭环另按
[Runtime Positive-Convergence Live Closure](../70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md)
执行；自动化绿灯不能代替真实 provider persisted terminal chain。
