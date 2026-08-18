# Governance Module

Status: active

Type: kernel-module-contract

Last Verified: 2026-08-18

## 功能范围与边界

`kernel/governance` 组合 Decision、Approval、Run、Workspace 与 Memory owner 合同，执行自动治理评估、自动解决、升级和阻塞，并管理 Governance 自身的 `AutoGovernanceTrace` 协调状态。它不拥有 Decision、Approval、Run 或 Runtime aggregate，也不持有 Orchestrator 合同。

Governance 与 Orchestrator 没有直接 EventHub I/O。Governance 只向 Decision / Approval owner 提交治理结果；Orchestrator 独立消费这些 owner 已持久化的 evidence/fact，并按 Runtime 合同决定 continuation 或 terminal。Governance 不 Resume、Drive 或恢复 Run。

## 组件交互及合同

- 生产装配只注入 Governance 自身的 `AutoGovernanceTraceRepository`、EventHub、配置加载器和进程级后台任务；Decision、Approval、Run、Workspace 与 Memory 的生产读写均由 Biz 内部 EventHub adapter 完成。
- Governance 订阅 Decision `TicketCreated` 与 Run `StateChanged`。Approval 没有 created 订阅；待处理 Approval 由 Run 的 `blocked_on_approval` owner fact 或启动恢复发现。
- 自动解决只发送 Decision `EnsureResolvedCommand` 或 Approval `EnsureApprovedCommand`。两者携带 Workspace scope 和 `auto_governance` actor；Decision resolution ID 同时包含 Workspace 隔离。
- `pkg/events.GovernanceData` 发布 evaluated、resolved、escalated、blocked 通知；`ListAutoGovernanceTracesCommand` / `GovernanceCommandResult` 提供 trace 查询。
- `ListAutoGovernanceTracesCommand.Query` 使用共享的 owner-neutral `workorchquerycontract.AutoGovernanceTraceQuery`，当前直接透传给 Governance repository；不存在文档层声称的第二次 query 映射。
- application 按 Workspace / Run / item scope 查询 trace；Eventstream 消费 Governance 通知形成审计投影。两者都不能用 trace 替代 Decision、Approval 或 Run 的权威状态。

## Trace 一致性与恢复合同

- Trace identity 由 `WorkspaceID + ItemType + ItemID` 决定；相同原始 Ticket / Approval ID 在不同 Workspace，或 Ticket 与 Approval 使用相同原始 ID，必须得到不同的 trace ID。repository `Upsert` 不允许同一 ID 改绑 Workspace、Run、Step 或 item identity。
- 幂等检查、每 Run budget 和 shared budget history 查询都必须同时携带 `WorkspaceID`；RunID 或 item ID 不能单独充当全局 scope。
- owner mutation 前必须依次持久化 `evaluated` 和 `resolving`。任一 trace List / Upsert 失败都 fail-closed，不得继续调用 Decision / Approval owner。
- owner ensure 成功后再把 `resolving` 提交为 `resolved`。若最终 trace 写入失败或进程崩溃，启动恢复从 Governance 全局未完成 trace inventory 读取 `evaluated` / `resolving`，先绑定每条 owner fact 的 Workspace，再查询 Decision / Approval 状态并幂等完成或阻塞 trace；不得重复已提交的 owner mutation。
- 启动时 Run 恢复使用 Run owner 的进程级全局 `List` inventory，而不是 scoped query。每个 Run fact 必须先提供 WorkspaceID，之后才能进行 scoped Ticket / Approval 读取。该 inventory 只发现阻塞项，不授予 Run lifecycle 或 Runtime drive 权限。
- Governance 通知使用无 acknowledgement 的 EventHub `Post`，因此 trace 必须先持久化再发布；通知丢失不得反向修改已提交的 trace 或 owner 状态。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `blocks/decision` | 双向 | Decision → Governance `TicketCreated`；Governance → Decision scoped ticket query / ensure-resolved | 完整；resolved 通知不是 Governance 订阅入口 |
| `blocks/approval` | Governance → Approval | scoped approval query / ensure-approved | 完整；待处理项来自 Run owner fact 或启动 inventory |
| `blocks/run` | 双向 | Run → Governance `StateChanged`；Governance → Run scoped read，以及启动时全局 List inventory | 完整；Governance 不写 Run、不解释 Runtime aggregate |
| `blocks/workspace` / `blocks/memory` | Governance → owner | scoped Goal / DoD / Memory typed query | 完整；只读 owner fact |
| `kernel/orchestrator` | 无直接 I/O | 经 Decision / Approval / Run typed fact 间接协作 | 完整；不存在 Governance ↔ Orchestrator 生产合同 |
| `application/workorchd` | application → Governance | scoped trace list command | 完整；application 只形成 read model |
| `blocks/eventstream` | Eventstream ← Governance | evaluated / resolved / escalated / blocked notification | 完整；仅审计投影 |

## 相关约束

- 生产依赖不得注入其他 owner reader、resolver、resume callback、service 或 repository。
- 无法读取治理历史、无法提交审计前置状态、无法满足策略或权限时必须 fail-closed；不得把错误解释为“预算使用 0”或静默修改 owner 状态。
- Governance 不得绕过 Orchestrator 的 stage、evidence、coverage、requiredOutput 与四状态机合同。本组件修复不修改 SM-1～SM-4 transition，只收紧 Governance 自身协调事务。

## 已覆盖测试点

- [x] Workspace-scoped Ticket / Approval / Run / Goal / DoD owner 读取，以及无 scope identifier 在 owner I/O 前被拒绝。
- [x] trace 幂等查询、budget 查询失败及 evaluated/resolving precommit 写失败均阻止 owner mutation。
- [x] owner 已提交但最终 trace 写失败后，从 `resolving` 启动恢复为 `resolved`，且不重复 owner mutation。
- [x] Workspace / item type 隔离的 trace identity、budget、closed-state 查询，以及 memory/file repository identity rebinding 拒绝。
- [x] 自动解决、升级、重复 prompt budget、启动发现 Approval 与 Governance 通知 scope 的代表性回归。
- [x] 边界扫描确认 Governance 无 Run resume / drive / create path；生产装配不注入其他 owner repository/service。

## 待完善测试点

- [ ] 覆盖重复 Decision / Run 通知的通知次数语义，确认同一 owner fact 不产生重复的 evaluated/resolved 通知。
- [ ] 补齐 Ticket / Approval 的 policy budget、权限不足、阈值边界与 consecutive-failure 全矩阵。
- [ ] 补齐所有跨 owner Send 的 missing subscriber、nil/wrong result、timeout 与 owner error 矩阵。
- [ ] 为启动使用的全局 trace / Run inventory 增加显式分页或总量上限及超限测试。
- [ ] EventHub `Post` 当前无 delivery acknowledgement；若框架提供可观测失败合同，再补充通知失败注入测试。
