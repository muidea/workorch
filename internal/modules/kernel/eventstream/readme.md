# Eventstream Module

## 功能定位

`kernel/eventstream` 是 WorkOrch EventRecord 存储、查询、清理与审计回放输入面的 owner module。它保存业务事件和 runtime 事件的可查询记录。

## 拥有边界

- 拥有 EventRecord 的存储、查询和 purge command。
- Event payload 是审计与投影输入，不是 owner state mutation。
- 不修复 Workspace、Run、Result 或其他 owner 的正式状态。

## 主要交互

- 接收 `kernel/workspace`、`kernel/run`、`kernel/orchestrator`、`kernel/result`、`kernel/decision`、`kernel/approval` 等模块发布的事件。
- 被 `application/workorchd` 用于 projection、timeline、SSE、audit 和 snapshot 构建。
- 与 runtime progress/evidence/result event 共同形成观测链路。

## 组件交互合同

- Input contract: EventRecord 写入必须包含 topic、sourceModule、subject、scope、version 和 bounded payload/ref。
- Output contract: query 返回 ordered bounded EventRecord list；purge 返回 count 与诊断信息。
- Owner contract: Eventstream 不接受通过 event payload 修复 owner state 的请求。
- Projection contract: application 消费 EventRecord 只能作为投影输入或审计输入。
- Purge contract: workspace/run 相关 purge 必须携带 scope，避免跨 workspace 误删。

## 已知缺口

- 当前盘点未发现 Eventstream module 职责缺失；大 payload 默认投影预算仍应持续验证。

## 边界约束

- 查询和 purge 必须有 bounded limit 与必要 workspace/run scope。
- 大 payload 不应默认内联到 projection。
- Eventstream 不是唯一真相源，不能替代 owner repository。
