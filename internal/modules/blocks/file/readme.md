# 文件能力组件

## 功能说明

`blocks/file` 提供受 workspace 路径边界约束的文件 list、read、write、edit、patch preview/apply/revert 等运行时能力，并返回规范化执行结果。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `ExecuteRuntimeOperationCommand/Result` 同步投递文件操作；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 V1 执行通知。
- Runtime V2 target snapshot prerequisite 使用 Orchestrator 定义的 `AcquireTargetSnapshotCommand` / `AcquireTargetSnapshotResult`；本 block 直接订阅 EventHub 并返回具体 `TargetSnapshot`，不使用跨组件 map 或 JSON bridge。
- Runtime V2 workspace manifest 与 File capability preflight 同样使用 Orchestrator 定义的 typed EventHub command/result；manifest revision 对文件路径和内容变化敏感。
- Runtime V2 state-change/validation operation 只返回 typed status、failure 与 evidence 摘要，不返回跨组件 map。
- request 必须携带 workspace/run/step scope、step kind、operation、path/ref 和有界输入；result 返回状态、摘要、target refs、side-effect metadata 和 raw ref。
- Orchestrator 负责 state-changing admission、approval、evidence quality、coverage binding 和 next action。

## 相关约束

- 所有路径必须限定在 workspace internal/deliverable 允许范围内，并防止路径逃逸。
- 大内容必须使用摘要和 raw ref，不能进入默认事件或投影。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result、Memory 或 coverage 状态。
- 文件写入成功只表示技术动作成功，不等于业务完成。
- 完整 target snapshot 必须携带 target ref、content、content hash 和明确的 complete content state；missing、truncated、binary 或 redacted 内容不得作为可 patch snapshot 提交。
