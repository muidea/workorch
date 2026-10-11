# Governance Module

Status: active

Type: kernel-module-contract

Last Verified: 2026-08-30

## 功能范围与边界

`kernel/governance` 组合 Decision、Approval、Run、Workspace 与 Memory owner 合同，执行自动治理评估、自动解决、升级和阻塞，并管理 Governance 自身的 `AutoGovernanceTrace` 协调状态。它不拥有 Decision、Approval、Run 或 Runtime aggregate，也不持有 Orchestrator 合同。

Governance 与 Orchestrator 没有直接 EventHub I/O。Governance 只向 Decision / Approval owner 提交治理结果；Orchestrator 独立消费这些 owner 已持久化的 evidence/fact，并按 Runtime 合同决定 continuation 或 terminal。Governance 不 Resume、Drive 或恢复 Run。

## 组件交互及合同

- 生产装配只注入 Governance 自身的 `AutoGovernanceTraceRepository`、EventHub、配置加载器和进程级后台任务；Decision、Approval、Run、Workspace 与 Memory 的生产读写均由 Biz 内部 EventHub adapter 完成。
- Governance 订阅 Decision `TicketCreated` 与 Run `StateChanged`。Approval 没有 created 订阅；待处理 Approval 由 Run 的 `blocked_on_approval` owner fact 或启动恢复发现。
- 自动解决只发送 Decision `EnsureResolvedCommand` 或 Approval `EnsureApprovedCommand`。两者携带 Workspace scope 和 `auto_governance` actor；Decision resolution ID 同时包含 Workspace 隔离。
- `workspace_definition_clarification` 与 `runtime_definition_clarification` 不属于自动治理输入。Governance 对两类 ticket
  均 fail-closed 为人工定义修订 blocker，不发送 generic `EnsureResolvedCommand`，也不生成 continue/fail Runtime proposal；
  它们只能由 Decision owner 的 admission/replacement 专用合同收口。
- `contract_change` 及携带 `ContractChangeRevisions` 的 Ticket 不属于自动治理输入，policy 在预算/LLM 处理前返回 `contract_change_requires_exact_approval`。提案登记、拟议 candidate 和 unknown 原文不构成自动批准或定义应用权；ordinary notification 也不能启用该能力。
- `pkg/events.GovernanceData` 发布 evaluated、resolved、escalated、blocked 通知；`ListAutoGovernanceTracesCommand` / `GovernanceCommandResult` 提供 trace 查询。
- `ListAutoGovernanceTracesCommand.Query` 使用共享的 owner-neutral `workorchquerycontract.AutoGovernanceTraceQuery`；只接受 exact `application/workorchd/appservice` producer、canonical non-empty Workspace scope 和 `1..500` limit，完整门禁通过后才进入 Governance repository。owner-private 启动恢复继续直接读取有独立硬上限的全局 inventory。
- application 按 Workspace / Run / item scope 查询 trace；Eventstream 消费 Governance 通知形成审计投影。两者都不能用 trace 替代 Decision、Approval 或 Run 的权威状态。

### Trace 一致性与恢复合同

- Trace identity 由 `WorkspaceID + ItemType + ItemID` 决定；相同原始 Ticket / Approval ID 在不同 Workspace，或 Ticket 与 Approval 使用相同原始 ID，必须得到不同的 trace ID。repository `Upsert` 不允许同一 ID 改绑 Workspace、Run、Step 或 item identity。
- 幂等检查、每 Run budget 和 shared budget history 查询都必须同时携带 `WorkspaceID`；RunID 或 item ID 不能单独充当全局 scope。
- owner mutation 前必须依次持久化 `evaluated` 和 `resolving`。任一 trace List / Upsert 失败都 fail-closed，不得继续调用 Decision / Approval owner。
- Goal、DoD 或 Memory owner read 的任一错误必须原样返回；不得重解释为缺失/空上下文并继续 policy evaluation、trace
  persistence 或 Decision / Approval mutation。
- worker 只提交 evaluated/escalated/resolution-started/blocked/completed immutable fact；唯一 Governance-private
  dispatcher 选择命名的 `AutoGovernanceTraceState`（`evaluated | resolving | escalated | blocked | resolved`）并写 repository，worker 不能直接设置 status
  或调用 Upsert。当前 repository 没有 CAS，因此 dispatcher 使用 owner-local mutex 串行化 load/reduce/upsert；这不授予
  Governance 修改 Decision、Approval、Run 或 Runtime state 的权限。
- owner ensure 成功后再把 `resolving` 提交为 `resolved`。若最终 trace 写入失败或进程崩溃，启动恢复从 Governance 全局未完成 trace inventory 读取 `evaluated` / `resolving`，先绑定每条 owner fact 的 Workspace，再查询 Decision / Approval 状态并幂等完成或阻塞 trace；不得重复已提交的 owner mutation。
- 启动时 Run 恢复使用 Run owner 的进程级全局 `List` inventory，而不是 scoped query。全局 trace query 显式请求至多 `100001` 条并以 `100000` 为硬上限，Run inventory 同样以 `100000` 为硬上限；超限或包含 nil/缺失 identity 的 owner fact 均 fail-closed，本轮不执行任何恢复 mutation。每个有效 Run fact 必须先提供 WorkspaceID，之后才能进行 scoped Ticket / Approval 读取。该 inventory 只发现阻塞项，不授予 Run lifecycle 或 Runtime drive 权限。
- Governance 通知使用无 acknowledgement 的 EventHub `Post`，因此 trace 必须先持久化再发布；通知丢失不得反向修改已提交的 trace 或 owner 状态。

### Concrete interaction matrix

| 交互组件 | 方向 | 具体合同 | owner 边界 |
| --- | --- | --- | --- |
| `blocks/decision` | 双向 | Decision → Governance `TicketCreated`；Governance → Decision scoped ticket query / ensure-resolved | resolved notification 不是 Governance 订阅入口 |
| `blocks/approval` | Governance → Approval | scoped approval query / ensure-approved | 待处理项来自 Run owner fact 或启动 inventory |
| `blocks/run` | 双向 | Run → Governance `StateChanged`；Governance → Run scoped read，以及启动时全局 List inventory | Governance 不写 Run、不解释 Runtime aggregate |
| `blocks/workspace` / `blocks/memory` | Governance → owner | scoped Goal / DoD / Memory typed query | 只读 owner fact |
| `kernel/orchestrator` | 无直接 I/O | 经 Decision / Approval / Run typed fact 间接协作 | 不存在 Governance ↔ Orchestrator 生产合同 |
| `application/workorchd` | Application → Governance | scoped trace list command | Application 只形成 read model |
| `blocks/eventstream` | Eventstream ← Governance | evaluated / resolved / escalated / blocked notification | 仅审计投影 |

## 不变量与非职责

- 生产依赖不得注入其他 owner reader、resolver、resume callback、service 或 repository。
- 无法读取治理历史、无法提交审计前置状态、无法满足策略或权限时必须 fail-closed；不得把错误解释为“预算使用 0”或静默修改 owner 状态。
- 所有 Decision / Approval / Run / Workspace / Memory `Send` response 必须先通过 nil、EventHub error、concrete result type 与请求 identity 校验；串 Workspace、Run 或 item 的结果以 conflict 拒绝。返回 DTO 在离开 adapter 前必须克隆，不向 Governance 暴露 owner 的可变指针。
- consecutive-failure history 由 Governance 按 `OccurredAt + trace ID` 稳定排序，不依赖 repository 迭代顺序；通过自动 Approval 策略与置信度门槛后，evaluated trace 不得残留 `policyBlocked`。
- Governance 不得绕过 Orchestrator 的 stage、evidence、coverage、requiredOutput 与四状态机合同；Governance 自身协调事务不得修改 SM-1～SM-4 transition。

### 进程生命周期合同

- 首次 Start 建立 owner-local `basebiz.Lifecycle` 代次；重复 Start 不 Reset。Module 先持有 Service，worker/订阅逐成功记账，部分 Start 失败保留清理入口，不在失败分支提前退订/丢任务句柄。
- `BeginShutdown` 非阻塞关 fresh notification/work enqueue 并取消 worker/reconciliation，不等待、不退订。已接纳任务/handler 用真实退出回执计数；取消后队列剩余项仅释放回执、不发新 owner I/O。只读 trace query 保留显式 drain，原 producer/scope/limit 校验不变。
- `Quiesce(ctx)` 等待 worker、scheduler 与在途 handler 真正结束；timeout/cancel 保留 work/reconcile done/cancel、queue 与订阅/依赖，允许用新停机预算重试。
- `StopChecked(ctx) error` 为 final release：封闭 drain 后复核在途，逆序 checked 取消 topic 与 matched observer，成功一项才移除一项 ledger，失败保留原 Service。Hub 调用不持 `mu`；未知 panic 原样上抛且锁/cleanup claim 不被毒化。
- Module 暴露 BeginShutdown/Quiesce，plugin void Teardown 检查 Stop error 并通过 `cd.WrapError` panic 交给 framework guard，成功后才清空 owner。必须配合应用共享 Hub/task 屏障，不能将本批适配声明为完整 S1–S5；不改 Governance formal trace transition、Runtime SM 或预算。

## 能力验收

| 验收面 | 自动化证据 | 覆盖维度 |
| --- | --- | --- |
| Module lifecycle 与 dependencies | `module_test.go`、`biz/service_test.go` | configure/start/stop、subscription rollback、background registration、scoped worker |
| concrete owner adapters | `biz/event_contract_matrix_test.go`、`biz/event_readers_test.go`、`biz/owner_context_read_fail_closed_test.go` | notification exact producer、missing/nil/wrong result、owner error、scope/identity mismatch、detached result；Goal/DoD/Memory read error 零 trace、零 Decision/Approval mutation |
| policy and budget | `biz/policy_matrix_test.go`、`biz/service_test.go` | boundary values、human permission、shared/consecutive/prompt budget、auto resolve/escalate |
| trace private state | `biz/trace_internal_event_test.go`、`biz/trace_reliability_test.go` | legal/illegal transition、precommit failure、owner-commit response loss、restart recovery |
| duplicate/recovery inventory | `biz/notification_idempotency_test.go`、`biz/recovery_inventory_test.go` | duplicate fact、workspace isolation、invalid fact、hard inventory limit |
| typed notification/query | `pkg/events/messages_test.go`、`biz/trace_query_contract_test.go`、`module_test.go` | clone、exact producer、canonical Workspace scope、bounded list query、owner I/O 前拒绝、immutable audit notification |

EventHub `Post` 是无 acknowledgement 的审计通知；组件验收证明“trace 先持久化、通知不反向驱动状态”，不把异步
transport delivery 当作 owner transaction 成功条件。
