# Run Module

## 功能定位

`kernel/run` 是 Run、Step 与 RuntimeCheckpoint 生命周期的 owner module。它负责 Run 创建、启动、恢复、阻塞、完成、失败、取消、supersede，以及 Step/Checkpoint 查询入口。

## 拥有边界

- 拥有 Run 状态机与 Run/Step/Checkpoint owner command/query。
- 负责把 Run 状态变化发布为正式事件。
- 不解释 LLM 输出正确性，不拥有 coverage/evidence/result 判定逻辑。

## 主要交互

- 激活时读取 `blocks/workspace` 的 workspace definition。
- 通过 `kernel/orchestrator` 发起 Start/Resume/Validate/Complete/Fail 等编排动作。
- 与 `blocks/decision`、`blocks/approval` 协作表达 governed blocked 状态。
- 被 `application/workorchd` 通过 owner read/command ports 读取和展示。

## 组件交互合同

- Input contract: Run/Step command 必须包含 workspaceId/runId，Step/Checkpoint 查询必须能在 workspace scope 下解析。
- Output contract: Run 状态变化、Step 状态变化和 checkpoint 结果必须发布正式事件或通过 owner command response 返回。
- Workspace contract: 创建/启动 Run 前必须读取 `blocks/workspace` 的有效 definition revision。
- Orchestrator contract: Run module 只发起编排命令，不解释 contract gate 结果；状态转换输入来自 orchestrator 的正式 outcome。
- Governance contract: blocked_on_decision/blocked_on_approval 必须关联具体 ticket/approval scope。
- EventHub contract: workspace read 与 orchestrator command adapter 直接使用 module 注入的 magicCommon `event.Hub`；不得通过 `MagicHub.RawHub()` 兼容路径发送 owner command。
- Run state notification contract: created/started/resumed/blocked/completed/failed/cancelled/superseded/state-changed 统一通过 `/run/consumers/#` 投递 `RunStateChangedData`；该 data 不携带 ExecutionContext 或其他 map/any 字段。
- Step notification contract: Step created/dispatched/succeeded/failed/skipped/state-changed 统一投递 `StepChangedData`；输入输出摘要使用 Run 自有递归 `NotificationPayload`，原始内容仅通过 ref 引用，不以 map/any 跨组件传输。
- LLM notification contract: 流式进度和用量分别投递 `LLMStreamProgressData`、`LLMUsageRecordedData`；context budget、cache usage、cache markers 使用递归具体值，不通过 JSON/reflect 转换。
- Consumer contract: Governance 与 Eventstream 直接导入 `kernel/run/pkg/events` 并按 topic 订阅具体 Run/Step/LLM data；生产 EventDrivenDispatch 路径不再为这些通知发布 Envelope。
- Assembly contract: 生产 `Dependencies` 只接收 Run 自身 repositories 与 EventHub；不接受可注入的 WorkspaceReader、Orchestrator、callback 或跨组件 adapter。repository/fake ports 只存在于 `_test.go`。

## 已知缺口

- C04 生产旁路已关闭：Workspace 读取与 Orchestrator command adapter 已下沉到 Run biz 的 EventHub-only assembly，测试 fake 构造器仅存在于 `_test.go`。后续仍需持续验证 stuck run recovery 和 duplicate run scope。
- Run state、Step、LLM progress/usage 的生产与测试路径均已完成 typed 迁移；test-only `NewService` 直接接收 magicCommon Hub，状态通知断言直接观察 `RunStateChangedData`，不再保留 legacy `events.Hub`、Envelope 或 `Published()` 兼容夹具。

## 边界约束

- Run/Step 操作必须携带足够 workspace/run scope。
- 不允许出现 `running + currentStep failed + no blocker + no next action` 这类悬挂状态。
- Step 合同失败不能先记录为 completed 再回滚为 failed。
