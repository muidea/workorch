# Run Module

## 功能定位

`kernel/run` 是 Run、Step 与 RuntimeCheckpoint 生命周期的 owner module。它负责 Run 创建、启动、恢复、阻塞、完成、失败、取消、supersede，以及 Step/Checkpoint 查询入口。

## 拥有边界

- 拥有 Run 状态机与 Run/Step/Checkpoint owner command/query。
- 负责把 Run 状态变化发布为正式事件。
- 不解释 LLM 输出正确性，不拥有 coverage/evidence/result 判定逻辑。

## 主要交互

- 激活时读取 `kernel/workspace` 的 workspace definition。
- 通过 `kernel/orchestrator` 发起 Start/Resume/Validate/Complete/Fail 等编排动作。
- 与 `kernel/decision`、`kernel/approval` 协作表达 governed blocked 状态。
- 被 `application/workorchd` 通过 owner read/command ports 读取和展示。

## 组件交互合同

- Input contract: Run/Step command 必须包含 workspaceId/runId，Step/Checkpoint 查询必须能在 workspace scope 下解析。
- Output contract: Run 状态变化、Step 状态变化和 checkpoint 结果必须发布正式事件或通过 owner command response 返回。
- Workspace contract: 创建/启动 Run 前必须读取 `kernel/workspace` 的有效 definition revision。
- Orchestrator contract: Run module 只发起编排命令，不解释 contract gate 结果；状态转换输入来自 orchestrator 的正式 outcome。
- Governance contract: blocked_on_decision/blocked_on_approval 必须关联具体 ticket/approval scope。
- EventHub contract: workspace read 与 orchestrator command adapter 直接使用 module 注入的 magicCommon `event.Hub`；不得通过 `MagicHub.RawHub()` 兼容路径发送 owner command。

## 已知缺口

- 当前盘点未发现 Run module 自身职责或跨 owner EventHub 合同缺失；后续需继续补 pairwise contract tests 验证 stuck run recovery 和 duplicate run scope。

## 边界约束

- Run/Step 操作必须携带足够 workspace/run scope。
- 不允许出现 `running + currentStep failed + no blocker + no next action` 这类悬挂状态。
- Step 合同失败不能先记录为 completed 再回滚为 failed。
