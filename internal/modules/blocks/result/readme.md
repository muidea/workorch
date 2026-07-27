# Result Stateful Block

## 功能说明

`blocks/result` 管理 Result 与 Artifact 组成的 formal-outcome 聚合，负责结果状态、证据引用、artifact 元数据和 owner 查询。它不负责 evidence quality、DoD coverage 或 accepted 判定策略。

## 组件交互及合同

- Result 与 Artifact 的 create/get/list/update/delete 由 `blocks/result/pkg/events` 定义具体 command 和 `ResultCommandResult`。
- Result create/update 持久化成功后，由本 block 单点发布 `ResultChangedData`；topic 按 draft、accepted、rejected、superseded 状态选择。
- Artifact create 成功后，由本 block 单点发布 `ArtifactCreatedData`。
- `kernel/orchestrator` 只有通过 evidence 和 coverage gate 后才能密封 terminal/result payload 并发布 terminal delivery fact。
- Runtime V2 accepted/rejected Result 由 Result owner 消费该事实、在自身 reducer 中构造并持久化；结果 ID 同时包含
  workspace/run scope，重复 terminal delivery 必须幂等。
- `application/workorchd`、Eventstream 和 projection 按需导入本 block 的事件合同查询或订阅。

## 相关约束

- Result 和 Artifact 必须携带一致的 workspace/run/result scope，引用不存在或 scope 不一致时必须拒绝。
- 本 block 只访问 Result 与 Artifact repository，不得访问 Run、Workspace、Decision 或 EvidenceRecord repository。
- Result 状态转换必须遵守正式状态机；accepted、rejected 和 superseded 通知不得由外围组件重复发布。
- Artifact 正文或大载荷使用 URI/raw ref，不进入 EventHub 通知。
