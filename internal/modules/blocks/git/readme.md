# Git 能力组件

## 功能说明

`blocks/git` 提供受控的 Git read/write 运行时能力，并返回规范化结果、变更引用、摘要、错误和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `ExecuteRuntimeOperationCommand/Result` 同步投递 Git 操作；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 V1 执行通知。
- Runtime V2 capability preflight 通过 Orchestrator 定义的 typed EventHub command 查询本 block 实际提供的 Git read/write kinds。
- Runtime V2 Git operation 将技术执行结果收口为 typed status、failure 和 evidence。
- request 必须携带 workspace repository scope、run/step scope、operation 和结构化参数；result 返回状态、摘要、changed refs、错误分类和 raw ref。
- Orchestrator 和 Approval 流程在写操作进入本 block 前完成策略准入和必要授权。

## 相关约束

- Git 操作只能作用于 workspace deliverable repository。
- 不得执行未声明或未准入的远程操作、历史改写或任意 Git CLI 文本。
- 本 block 不访问其他组件 service/repository，不拥有 Run、Result 或 completion 状态。
- Git 输出只作为 evidence candidate，不能直接判定 DoD 或 Result accepted。
