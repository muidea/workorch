# Web 能力组件

## 功能说明

`blocks/web` 提供受控的外部搜索和网页获取能力，并返回规范化内容摘要、source refs、truncation flags、provider diagnostic、错误和 raw ref。

## 组件交互及合同

- `kernel/orchestrator` 通过 Orchestrator 定义的 `ExecuteRuntimeOperationCommand/Result` 同步投递 Web search/fetch；本组件直接订阅 EventHub，不再启动通用 runtime executor 或发布 V1 执行通知。
- Runtime V2 capability preflight 通过 Orchestrator 定义的 typed EventHub command 查询本 block 实际提供的 search/fetch kinds。
- Runtime V2 Web operation 只返回 typed status、failure 和有界 evidence 引用。
- 需要校验外部访问授权时，本 block 导入 `blocks/approval/pkg/events`，通过 EventHub 查询 Approval；不注入 Approval service/repository。
- request 必须携带 workspace/run/step scope、operation、provider/ref 和有界 query/url；result 返回 source refs、摘要、truncation、provider diagnostic、错误和 raw ref。

## 相关约束

- query、URL、响应大小和执行时间必须有界，并区分 provider/network failure 与 contract violation。
- 大内容只能通过摘要和 raw ref 传递。
- 本 block 不拥有 Memory、Result、Run 或业务完成状态。
- Web 内容只作为 context/evidence candidate，不能直接写入 owner 状态或形成 accepted Result。
