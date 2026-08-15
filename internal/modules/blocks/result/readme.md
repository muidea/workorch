# Result Stateful Block

## 功能说明

`blocks/result` 管理 Result 与 Artifact 组成的 formal-outcome 聚合，负责结果状态、证据引用、artifact 元数据和 owner 查询。它不负责 evidence quality、DoD coverage 或 accepted 判定策略。

## 组件交互及合同

- Result 的 get/list/delete 与 Artifact 的 list/delete 由 `blocks/result/pkg/events` 定义具体 command 和
  `ResultCommandResult`；没有生产调用方的单项 Artifact get 合同不保留；
  query result 在离开 owner 前必须显式深拷贝，调用方不能取得 repository entity 的共享可变引用。
- Runtime Result 的唯一写入口是 typed `RuntimeTerminalDeliveryFact`。调用方只提交 sealed terminal material，
  Result owner 在自身 reducer 中派生 Result identity、state、时间戳和 delivery fence；不存在接收完整 Result/Artifact
  entity 的 generic create/update command。
- Result owner 持久化 terminal delivery 后单点发布 `ResultChangedData`；topic 按 accepted/rejected 状态选择。
- `kernel/orchestrator` 只有通过 evidence 和 coverage gate 后才能密封 typed `TerminalOutcome`/result payload 并发布
  terminal delivery fact；delivery 不携带 RuntimeAction 或独立 administrative kind。
- Runtime accepted/rejected Result 由 Result owner 消费该事实、在自身 reducer 中构造并持久化；结果 ID 同时包含
  workspace/run scope，重复 terminal delivery 必须幂等。
- terminal material 只在统一 `CloneResult` 边界执行一次深拷贝；Result owner 不在进入该边界前重复复制顶层
  evidence/artifact slice。该转换必须继续通过隔离回归和 allocation benchmark。
- `application/workorchd`、Eventstream 和 projection 按需导入本 block 的事件合同查询或订阅。

## 相关约束

- Result 和 Artifact 必须携带一致的 workspace/run/result scope，引用不存在或 scope 不一致时必须拒绝。
- 本 block 只访问 Result 与 Artifact repository，不得访问 Run、Workspace、Decision 或 EvidenceRecord repository。
- Result 状态转换必须遵守正式状态机；accepted、rejected 和 superseded 通知不得由外围组件重复发布。
- 如需新增 Artifact 写入，必须定义只携带不可变 artifact material 的 typed owner command，并由 Result owner
  派生 identity、state 和时间戳；不得恢复允许调用方提交完整 Artifact entity 的 generic write 合同。
- Artifact 正文或大载荷使用 URI/raw ref，不进入 EventHub 通知。
- Result owner 只按 sealed outcome identity 执行 owner-local transition，不读取 `ConvergenceState`、focus、repair 或
  error message 重新裁决。delivery feedback 保留 stable category 与完整安全 original diagnostic。
