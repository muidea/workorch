# Governance Module

## 功能定位

`kernel/governance` 是 AutoGovernance trace、policy gate 和治理建议的 owner module。它负责把结构化 failure/retry/policy signal 转成 bounded governance recommendation 和审计轨迹。

## 拥有边界

- 拥有 AutoGovernanceTrace 与治理建议记录。
- 可以建议 decision、approval、terminal 或 runtime recovery，但不直接拥有 Run/Result 状态。
- 不解析业务文本、模板 ID、语言或命令输出作为核心策略权威。

## 主要交互

- 从 `kernel/orchestrator` 接收 failure/retry/policy 信号。
- 与 `blocks/decision`、`blocks/approval` 协作创建治理对象。
- 向 `application/workorchd` 提供治理摘要与审计信息。
- 与 `kernel/eventstream` 协作保留治理事件。

## 组件交互合同

- Input contract: 只消费结构化 failure class、retry state、policy signal、source refs 和 scope。
- Output contract: 输出 decision/approval/terminal/runtime-action recommendation 与 AutoGovernanceTrace。
- Decision/Approval contract: 创建治理对象时必须携带 workspace/run/step/action scope 和幂等键。
- Orchestrator contract: recoverable runtime gap 应返回 runtime recovery action，而不是直接阻塞。
- Projection contract: application 只能展示治理摘要与 trace，不把治理摘要当 owner state。
- EventHub contract: owner command/read adapter 直接持有 framework 提供的 magicCommon `event.Hub`；禁止先包装为 `MagicHub` 再通过 `RawHub()` 反向取回。
- Notification contract: AutoGovernance 通知通过 `/governance/consumers/#` 投递 `GovernanceData`；Decision/Approval 输入分别订阅投递方的 `TicketChangedData` 与 `ApprovalChangedData`，不使用 Envelope 或 map 作为跨组件合同。
- Eventstream contract: Eventstream 逐 topic 导入 Governance 的 `pkg/events` 定义，并仅在自身审计投影边界构造 payload。
- Assembly contract: 生产 `Dependencies` 只接收 AutoGovernanceTrace repository、magicCommon EventHub、配置加载和 BackgroundRoutine；biz 不再接收或保存 legacy projection Hub，不接受 owner reader、resolve/resume callback 或跨组件 adapter。fake ports 仅存在于 `_test.go`。

## 已知缺口

- C09 生产旁路已关闭：六类 owner reader 与 resolve/resume callbacks 已从 module `Dependencies` 删除，EventHub adapters 下沉到 governance biz，fake port assembly 仅存在于 `_test.go`。后续仍需验证 repeated failure escalation 的幂等边界。
- Governance module 已删除只用于非空校验的 MagicHub bridge 与 legacy `events.Hub` 字段；订阅、owner command 和通知发布统一使用同一个 framework EventHub。
- Governance 输出以及 Decision、Approval、Run state 输入的 Envelope notification 已清理；三类输入均直接订阅投递方具体 data。

## 边界约束

- 只有真实 governance gap 才能阻塞 Run。
- 可恢复 runtime gap 应返回 runtime action，不默认升级为人工决策。
- 治理建议必须可追溯到 source refs。
