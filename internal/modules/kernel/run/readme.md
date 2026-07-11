# Run Module

## 功能说明

`kernel/run` 管理 Run、Step 与 RuntimeCheckpoint 的正式状态和查询合同，并组合 Workspace definition 与 Orchestrator 命令完成 Run 的创建、启动、恢复、阻塞和终态流转。

## 组件交互及合同

- Run 自身的 create/ensure/get/list/update/delete、Step 查询和 Checkpoint 查询由 `kernel/run/pkg/events` 定义具体 command 与 `RunCommandResult`。
- 创建或启动 Run 前，通过 EventHub 读取 `blocks/workspace` 的有效 workspace definition 和 revision。
- 启动、恢复、验证、完成、失败、取消和 supersede 通过 EventHub 调用 `kernel/orchestrator` 的具体 command；Run 不解释 Orchestrator 的 contract gate。
- governed blocker 必须携带对应 DecisionTicket 或 Approval 的 workspace/run scope。
- Run 状态通知使用 `RunStateChangedData`；Step 通知使用 `StepChangedData`；LLM 进度和用量分别使用 `LLMStreamProgressData` 与 `LLMUsageRecordedData`。
- Governance、Eventstream 和 application 按需导入 `kernel/run/pkg/events` 订阅或发送命令。

## 相关约束

- Run、Step 和 Checkpoint 操作必须携带足够的 workspace/run/step scope。
- 生产装配只接收 Run 自身 repository 与 EventHub，不得注入 WorkspaceReader、Orchestrator service、callback 或其他 owner repository。
- Run 不负责 coverage、evidence quality、Result acceptance 或 LLM 输出正确性判断。
- 不允许形成 active Run 无 next action、无 blocker 且当前 Step 已失败的悬挂状态。
- Step 合同失败不得先记录 completed 再回滚为 failed。
