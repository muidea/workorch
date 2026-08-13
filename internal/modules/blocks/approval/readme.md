# Approval Stateful Block

## 功能说明

`blocks/approval` 管理 Approval 聚合，负责授权请求的创建、查询、更新、批准、拒绝、supersede 和生命周期通知。

## 组件交互及合同

- Approval command/query 由 `blocks/approval/pkg/events` 定义，并返回 `ApprovalCommandResult`。
- 创建、批准、拒绝、解决和 supersede 后由本 block 发布 `ApprovalChangedData`。
- `kernel/governance`、`kernel/orchestrator` 和 `application/workorchd` 通过 EventHub 创建、查询或解决 Approval。
- File、Git、MCP、Shell、SSH、Web 等高风险能力只能消费 Approval 的公开状态合同，不能访问 Approval service/repository。
- Approval 需要记录 Orchestrator evidence 时，发送本 block 定义的 `RecordApprovalEvidenceCommand`，并接收 `RecordApprovalEvidenceResult`。
- approve/reject command 只提供不可变授权事实；目标状态、解决人、时间戳和 repository update 只允许经 Approval-private lifecycle event/reducer 完成。

## 相关约束

- Approval 操作必须携带 workspace/run/approval scope；同 ID 跨 workspace 或 run 冲突时不得误操作。
- 本 block 只访问 Approval repository，不得直接访问 EvidenceRecord、Run、Workspace 或技术 block 实现。
- 已终态 Approval 的重复解决必须幂等或返回冲突，不能覆盖既有授权结论。
- private reducer 只允许 `pending -> approved|rejected`，未知目标和反向/交叉终态迁移必须 fail closed。
- Approval 只表达授权结果，不直接执行副作用或修改 Run/Result 状态。
