# Governance Module

## 功能说明

`kernel/governance` 组合 Decision、Approval、Run、Workspace、Memory 和 Orchestrator 合同执行自动治理评估、自动解决、升级和阻塞，并管理 AutoGovernanceTrace 协调状态。

## 组件交互及合同

- 通过 EventHub 读取 DecisionTicket、Approval、Run、Workspace 与 Memory 的权威状态。
- 自动解决时通过 Decision 或 Approval owner 的具体 resolve command 写入治理结果；Orchestrator 将已持久化治理
  evidence 转为 immutable continuation/terminal fact，由 Run owner 在自身 reducer 中恢复或终态，Governance 不直接恢复 Run。
- `pkg/events.GovernanceData` 发布 evaluated、resolved、escalated、blocked 通知；`ListAutoGovernanceTracesCommand` / `GovernanceCommandResult` 提供 trace 查询。
- `AutoGovernanceTraceQuery` 是 Governance owner DTO；module 内部映射 repository query，EventHub 合同不引用 repository 类型。
- application 和 Eventstream 按需导入 `kernel/governance/pkg/events` 消费治理 trace 与通知。
- AutoGovernanceTrace 只记录治理协调过程，不替代 Decision、Approval 或 Run 的正式状态。

## 相关约束

- 生产依赖只能包含 Governance 自身 repository 与 EventHub，不得注入其他 owner reader、resolver、resume callback、service 或 repository。
- 自动治理必须保留 workspace/run/ticket/approval scope、决策依据和幂等键。
- 无法满足策略或权限时必须升级或阻塞，不得静默修改 owner 状态。
- Governance 不得绕过 Orchestrator 的 stage、evidence、coverage 和 requiredOutput 合同。
