# Governance Module

Status: active

Type: kernel-module-contract

Last Verified: 2026-08-17

## 功能范围与边界

`kernel/governance` 组合 Decision、Approval、Run、Workspace、Memory 和 Orchestrator 合同执行自动治理评估、自动解决、升级和阻塞，并管理 AutoGovernanceTrace 协调状态。

## 组件交互及合同

- 通过 EventHub 读取 DecisionTicket、Approval、Run、Workspace 与 Memory 的权威状态。
- 自动解决时通过 Decision 或 Approval owner 的具体 resolve command 写入治理结果；Orchestrator 将已持久化治理
  evidence 转为 immutable continuation/terminal fact，由 Run owner 在自身 reducer 中恢复或终态，Governance 不直接恢复 Run。
- `pkg/events.GovernanceData` 发布 evaluated、resolved、escalated、blocked 通知；`ListAutoGovernanceTracesCommand` / `GovernanceCommandResult` 提供 trace 查询。
- `AutoGovernanceTraceQuery` 是 Governance owner DTO；module 内部映射 repository query，EventHub 合同不引用 repository 类型。
- application 和 Eventstream 按需导入 `kernel/governance/pkg/events` 消费治理 trace 与通知。
- AutoGovernanceTrace 只记录治理协调过程，不替代 Decision、Approval 或 Run 的正式状态。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `blocks/decision` / `blocks/approval` | 双向 | ticket/approval 查询、ensure resolution；created/resolved 通知 | 完整；正式治理状态仍归各 owner |
| `blocks/run` / `blocks/workspace` / `blocks/memory` | Governance → owner | scoped state/definition/recall 查询 | 完整；只读 typed command/result |
| `kernel/orchestrator` | 双向 | persisted governance evidence/fact 与 typed continuation/terminal 协议 | 完整；Governance 不直接恢复 Run |
| `application/workorchd` / `blocks/eventstream` | consumer ← Governance | trace 查询及 evaluated/resolved/escalated/blocked 通知 | 完整 |

## 相关约束

- 生产依赖只能包含 Governance 自身 repository 与 EventHub，不得注入其他 owner reader、resolver、resume callback、service 或 repository。
- 自动治理必须保留 workspace/run/ticket/approval scope、决策依据和幂等键。
- 无法满足策略或权限时必须升级或阻塞，不得静默修改 owner 状态。
- Governance 不得绕过 Orchestrator 的 stage、evidence、coverage 和 requiredOutput 合同。

## 待完善测试点

- [ ] 覆盖重复 Decision/Approval/Run 通知只形成一次治理评估与 trace 更新。
- [ ] 覆盖自动解决、升级、阻塞的 policy budget、权限不足和边界值矩阵。
- [ ] 覆盖所有跨 owner Send 的缺失 subscriber、nil/wrong result、timeout 和 owner error。
- [ ] 增加边界测试，确认 Governance 无 Run resume callback、无其他 owner repository/service。
- [ ] 覆盖 trace 在重启、重复评估和部分失败后的幂等恢复与 workspace/run scope 隔离。
- [ ] 覆盖 Post 通知失败不反向改写已提交的治理 trace 或 Decision/Approval 状态。
