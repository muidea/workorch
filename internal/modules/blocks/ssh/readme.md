# SSH Block

## 功能说明

`blocks/ssh` 使用结构化 target、session 和 credential refs 执行受控远程命令，并返回规范化远程执行结果。

## 组件交互及合同

- `kernel/orchestrator` 使用 `internal/pkg/runtimeevent.ExecuteCommand` 派发 SSH step，并接收 `ExecuteCommandResult`。
- 本 block 通过 runtimeevent 发布 Accepted、Progress、Completed 或 Failed 具体通知。
- request 必须携带 workspace/run/step scope、target/session/credential refs、command、timeout 和 policy；result 返回远程 exit status、有界输出、target ref、错误分类和 raw ref。
- runtime policy、Governance 和 Approval 在远程副作用执行前完成准入。

## 相关约束

- target、session 和 credential 必须使用结构化引用；secret 不得进入事件、投影或日志。
- 远程副作用必须有明确 stage contract admission，权限缺失必须返回 structured blocker。
- 本 block 不拥有远程业务状态、Run 状态或 Result 状态。
- 远程输出必须经过 evidence binding 才能用于 coverage。
