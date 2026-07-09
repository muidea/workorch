# Result Module

## 功能定位

`kernel/result` 是 Result 与 Artifact 的 owner module。它负责持久化运行结果、artifact 元数据、结果查询、更新和删除，并为 acceptance/projection 提供正式 outcome 数据。

## 拥有边界

- 拥有 Result、Artifact owner command/query 与状态。
- 只记录经 runtime completion gate 或正式命令创建的结果。
- 不负责判断 Goal/DoD 是否满足；满足性判断由 orchestrator/completion gate 提供证据合同。

## 主要交互

- 被 `kernel/orchestrator` 在 acceptance/result finalization 时调用。
- 向 `kernel/eventstream` 发布 result/artifact 事件。
- 被 `application/workorchd` 读取后投影到 run snapshot、workspace snapshot 与 artifact content API。
- 与 `kernel/run` 通过 run/workspace scope 关联。

## 组件交互合同

- Input contract: Result/Artifact create/update/delete 必须携带 workspace/run/result/artifact scope 与 accepted evidence refs。
- Output contract: Result/Artifact command response 必须返回正式 owner model 或明确 not found/conflict/error。
- Orchestrator contract: `kernel/orchestrator` 负责 completion gate；Result module 只持久化 gate 后的 outcome。
- Projection contract: `application/workorchd` 只能展示 logical artifact refs 和 bounded content metadata。
- Event contract: Result/Artifact 变更必须发布可追溯 source refs 和 subject refs。

## 已知缺口

- 当前盘点未发现 Result module 职责缺失；artifact content API 相关失败需要在 application/server 边界继续归因。

## 边界约束

- 无 evidence 的 LLM 自称完成不能创建 accepted Result。
- Artifact 对外展示应使用逻辑 ref，不能泄露本地绝对路径。
- Result/Artifact 读写必须携带足够 workspace/run/result scope。
