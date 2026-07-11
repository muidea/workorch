# File Block

## 功能说明

`blocks/file` 提供受 workspace 路径边界约束的文件 list、read、write、edit、patch preview/apply/revert 等运行时能力，并返回规范化执行结果。

## 组件交互及合同

- `kernel/orchestrator` 使用 `internal/pkg/runtimeevent.ExecuteCommand` 派发 File step，并同步接收 `ExecuteCommandResult`。
- 本 block 通过 runtimeevent 发布 Accepted、Progress、Completed 或 Failed 具体通知。
- request 必须携带 workspace/run/step scope、step kind、operation、path/ref 和有界输入；result 返回状态、摘要、target refs、side-effect metadata 和 raw ref。
- Orchestrator 负责 state-changing admission、approval、evidence quality、coverage binding 和 next action。

## 相关约束

- 所有路径必须限定在 workspace internal/deliverable 允许范围内，并防止路径逃逸。
- 大内容必须使用摘要和 raw ref，不能进入默认事件或投影。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result、Memory 或 coverage 状态。
- 文件写入成功只表示技术动作成功，不等于业务完成。
