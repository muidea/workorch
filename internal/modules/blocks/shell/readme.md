# Shell Block

## 功能说明

`blocks/shell` 在受控工作目录、超时和资源策略下执行本地 shell command，并返回 exit code、stdout/stderr 摘要、错误分类、耗时和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 使用 `internal/pkg/runtimeevent.ExecuteCommand` 派发 Shell step，并接收 `ExecuteCommandResult`。
- 本 block 通过 runtimeevent 发布 Accepted、Progress、Completed 或 Failed 具体通知。
- request 必须携带 workspace/run/step scope、command、working directory/ref、timeout 和 policy；result 返回 exit code、有界输出、duration、error class 和 raw ref。
- Orchestrator 与 Approval 流程在高风险命令进入本 block 前完成准入，或根据本 block 返回的 blocker 进入治理。

## 相关约束

- command、工作目录、运行时间和输出大小必须有界，不能默认启动后台长期任务。
- destructive、unbounded 或不符合 policy 的命令必须拒绝。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result 或 Memory 状态。
- stdout/stderr 只是 evidence input，不能直接判定业务完成。
