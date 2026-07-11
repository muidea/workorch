# Git Block

## 功能说明

`blocks/git` 提供受控的 Git read/write 运行时能力，并返回规范化结果、变更引用、摘要、错误和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 使用 `internal/pkg/runtimeevent.ExecuteCommand` 派发 Git step，并接收 `ExecuteCommandResult`。
- 本 block 通过 runtimeevent 发布 Accepted、Progress、Completed 或 Failed 具体通知。
- request 必须携带 workspace repository scope、run/step scope、operation 和结构化参数；result 返回状态、摘要、changed refs、错误分类和 raw ref。
- Orchestrator 和 Approval 流程在写操作进入本 block 前完成策略准入和必要授权。

## 相关约束

- Git 操作只能作用于 workspace deliverable repository。
- 不得执行未声明或未准入的远程操作、历史改写或任意 Git CLI 文本。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result 或 completion 状态。
- Git 输出只作为 evidence candidate，不能直接判定 DoD 或 Result accepted。
