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

## 当前内聚收口切片（2026-08-16）

- `Service` 仍是唯一 Governance lifecycle/coordination component；原 1,909 行文件已按 service lifecycle/command、
  worker/reconciliation、ticket policy、approval policy、typed payload parser、trace store query/budget 与共享纯 helper
  分成同 package 的 owner-private 文件。没有新增 Module、state machine、worker queue、trace repository、EventHub
  round trip、resume callback 或跨 owner service/repository。
- ticket/approval evaluator 仍只产生 Governance owner 的 evaluation fact，并通过 Decision/Approval typed resolve command
  提交；worker 继续串行消费同一 `governanceWork` queue。blocked-run reconciliation 只重读具体 owner fact，不直接
  迁移 Run 或 Runtime aggregate。
- `AutoGovernanceTrace` 仍是唯一协调状态，trace 的 resolved/closed/shared-budget 查询均只经过原 repository；payload
  parser 是纯 typed payload 读取，不构造通用 map authority，也不改变 event contract。
- C22-06 Governance 切片完成后，非测试 Go 为 17 个文件/2,576 LOC，测试保持 5 个文件/2,135 LOC，package
  保持 5；最大生产文件从 `biz/service.go` 1,909 行降为 `biz/payload_parser.go` 485 行，至少 500 行生产文件从
  1 个降为 0，生产 `CC>30` 继续为 0。

## 相关约束

- 生产依赖只能包含 Governance 自身 repository 与 EventHub，不得注入其他 owner reader、resolver、resume callback、service 或 repository。
- 自动治理必须保留 workspace/run/ticket/approval scope、决策依据和幂等键。
- 无法满足策略或权限时必须升级或阻塞，不得静默修改 owner 状态。
- Governance 不得绕过 Orchestrator 的 stage、evidence、coverage 和 requiredOutput 合同。
