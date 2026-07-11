# Result Stateful Block

## 功能定位

`blocks/result` 是 Result 与 Artifact 的单一 formal-outcome 聚合 block。它管理这组结果与交付物的正式状态，但不承担 Goal/DoD 判定、runtime completion 或跨组件收敛编排。

## 拥有边界

- 拥有 Result、Artifact owner command/query 与状态。
- 只记录经 runtime completion gate 或正式命令创建的结果。
- 只访问自身 Result/Artifact repositories，不持有其他 module/block 的 service、repository 或 adapter。
- 不负责判断 Goal/DoD 是否满足；满足性判断由 orchestrator/completion gate 提供证据合同。

## 主要交互

- 被 `kernel/orchestrator` 在 acceptance/result finalization 时调用。
- 向 `kernel/eventstream` 发布 result/artifact 事件。
- 被 `application/workorchd` 读取后投影到 run snapshot、workspace snapshot 与 artifact content API。
- 与 `kernel/run` 通过 run/workspace scope 关联。

## 组件交互合同

- Command contract：调用方导入 `blocks/result/pkg/events`，通过 magicCommon EventHub 投递具体 Result/Artifact command，并接收 `ResultCommandResult`。
- Input contract：Result/Artifact create/update/delete 必须携带 workspace/run/result/artifact scope 与 accepted evidence refs；禁止 service/repository 旁路写入。
- Output contract：command result 必须返回正式 owner model 或明确 not found/conflict/error。
- Orchestrator contract：`kernel/orchestrator` 负责 completion gate；Result block 只持久化 gate 后的 outcome。
- Projection contract：`application/workorchd` 只能通过 EventHub result/notification 展示 logical artifact refs 和 bounded content metadata。
- Event contract：Result/Artifact command 持久化成功后由 Result owner 单点发布，并携带可追溯 subject/source refs；投递 source 固定为 `blocks/result`，调用方不得重复补发。
- Notification contract：Result created/accepted/rejected/superseded 使用 `ResultChangedData`，Artifact created 使用 `ArtifactCreatedData`；owner 通过 `/result/consumers/#` 投递，消费方直接导入 `blocks/result/pkg/events`，不使用 Envelope/map/JSON/reflect 桥。

## 已知缺口

- 当前未发现 Result/Artifact owner 通知覆盖缺口：Result create/update 按正式状态发布 created/accepted/rejected/superseded，Artifact create 发布 created；Orchestrator 的外围重复发布路径已删除。

## 边界约束

- 无 evidence 的 LLM 自称完成不能创建 accepted Result。
- Artifact 对外展示应使用逻辑 ref，不能泄露本地绝对路径。
- Result/Artifact 读写必须携带足够 workspace/run/result scope。
