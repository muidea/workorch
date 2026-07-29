# Shell 能力组件

## 功能说明

`blocks/shell` 在受控工作目录、超时和资源策略下执行本地 shell command，并返回 exit code、stdout/stderr 摘要、错误分类、耗时和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `ExecuteRuntimeOperationCommand/Result` 同步投递 Shell 操作；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 V1 执行通知。
- Runtime V2 capability preflight 通过 Orchestrator 定义的 typed EventHub command 确认 `shell/command` 当前由本 block 提供。
- stdout/stderr 不直接跨组件传递，只输出有界 typed evidence 摘要与引用。
- 对 state-changing `shell/command`，本 owner 在执行前后以有界 workspace regular-file snapshot 计算真实 changed refs；`.git`、symlink 与不可安全内容寻址的对象不作为交付物 effect。所有真实 changed ref 必须是 sealed `TargetRefs` 的子集，否则返回 `effect_mismatch/undeclared_workspace_effect` rejected receipt；不能通过同时 touch 一个已声明目标掩盖额外写入。
- accepted state-changing receipt 的 `TargetRefs` 与 `ResourceChanges` 来自 owner observation，而非直接回显候选输入；无可观察 workspace delta 时返回 no-progress，而不是 durable evidence。
- request 必须携带 workspace/run/step scope、command、working directory/ref、timeout 和 policy；result 返回 exit code、有界输出、duration、error class 和 raw ref。
- Runtime V2 evidence policy 使用 Orchestrator command 携带的具体 `RuntimeEvidencePolicy`，用于 zero-work marker、work-evidence marker、evidence quality、reason 和 tag 判定；本组件不从 operation parameter map 解析跨组件 policy。
- Orchestrator 与 Approval 流程在高风险命令进入本 block 前完成准入，或根据本 block 返回的 blocker 进入治理。

## 相关约束

- command、工作目录、运行时间和输出大小必须有界，不能默认启动后台长期任务。
- destructive、unbounded 或不符合 policy 的命令必须拒绝。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result 或 Memory 状态。
- stdout/stderr 只是 evidence input，不能直接判定业务完成。
- evidence policy 只调整本次 Shell evidence 的结构化质量结论，不得修改 Runtime authority、coverage focus 或 Run 状态。
