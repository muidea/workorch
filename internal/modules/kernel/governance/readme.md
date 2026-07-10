# Governance Module

## 功能定位

`kernel/governance` 是 AutoGovernance trace、policy gate 和治理建议的 owner module。它负责把结构化 failure/retry/policy signal 转成 bounded governance recommendation 和审计轨迹。

## 拥有边界

- 拥有 AutoGovernanceTrace 与治理建议记录。
- 可以建议 decision、approval、terminal 或 runtime recovery，但不直接拥有 Run/Result 状态。
- 不解析业务文本、模板 ID、语言或命令输出作为核心策略权威。

## 主要交互

- 从 `kernel/orchestrator` 接收 failure/retry/policy 信号。
- 与 `kernel/decision`、`kernel/approval` 协作创建治理对象。
- 向 `application/workorchd` 提供治理摘要与审计信息。
- 与 `kernel/eventstream` 协作保留治理事件。

## 组件交互合同

- Input contract: 只消费结构化 failure class、retry state、policy signal、source refs 和 scope。
- Output contract: 输出 decision/approval/terminal/runtime-action recommendation 与 AutoGovernanceTrace。
- Decision/Approval contract: 创建治理对象时必须携带 workspace/run/step/action scope 和幂等键。
- Orchestrator contract: recoverable runtime gap 应返回 runtime recovery action，而不是直接阻塞。
- Projection contract: application 只能展示治理摘要与 trace，不把治理摘要当 owner state。
- EventHub contract: owner command/read adapter 直接持有 framework 提供的 magicCommon `event.Hub`；禁止先包装为 `MagicHub` 再通过 `RawHub()` 反向取回。

## 已知缺口

- 当前盘点未发现 Governance module 自身职责或 owner command/read EventHub 合同缺失；后续需继续验证 repeated failure escalation 的幂等边界。

## 边界约束

- 只有真实 governance gap 才能阻塞 Run。
- 可恢复 runtime gap 应返回 runtime action，不默认升级为人工决策。
- 治理建议必须可追溯到 source refs。
