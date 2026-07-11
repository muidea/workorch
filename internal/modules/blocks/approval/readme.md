# Approval Stateful Block

## 功能定位

`blocks/approval` 是 Approval 与授权结果的单一资源聚合 block。它管理 pending/approved/rejected 正式状态，但不承担 Run、Result、coverage 或 runtime continuation 的跨组件编排。

## 拥有边界

- 拥有 Approval 的 pending/approved/rejected 状态与 owner command/query。
- 只访问自身 Approval repository，不持有其他 module/block 的 service、repository、writer callback 或 adapter。
- Approval 表达授权结果，不表达业务 evidence 是否满足。
- 不创建 accepted Result，也不直接推进 coverage。

## 主要交互

- 被 `kernel/orchestrator`、`kernel/governance` 或 runtime block approval gate 创建 pending approval。
- 与 `kernel/run` 协作表达 `blocked_on_approval` 状态。
- approval resolution 被 `kernel/orchestrator` 或相关 block continuation 消费。
- approved/rejected 状态成功落库后，由 Approval 投递 `/approval/command/record-evidence`，`kernel/orchestrator` 订阅并写入 EvidenceRecord。
- 被 `application/workorchd`、Web/TUI/API 读取和提交 approve/reject 命令。

## 组件交互合同

- Command contract：调用方导入 `blocks/approval/pkg/events`，通过 magicCommon EventHub 投递具体 approval command，并接收 `ApprovalCommandResult`。
- Input contract：approval command 必须包含 approvalId，并在 runtime 场景携带 workspaceId/runId/stepId/action。
- Output contract：command result 必须返回 pending/approved/rejected 状态和授权 decision data。
- Orchestrator/block contract：approval 只授权动作执行；后续 evidence/result 仍由 orchestrator gate 判定。
- Run contract：只有 pending approval 正式创建成功后，Run 才能进入 `blocked_on_approval`；双方仅通过 EventHub 交互。
- Application contract：无 scope approval 操作只能命中唯一 open approval，否则返回 conflict。
- Evidence contract：`RecordApprovalEvidenceCommand`、嵌套 `ApprovalEvidenceValue` 与 `RecordApprovalEvidenceResult` 均由 Approval 的 `pkg/events` 定义；Orchestrator 按需订阅，同步调用必须收到匹配的 evidenceRecordId。
- Notification contract：Approval 通过 `/approval/consumers/#` 异步投递 `ApprovalChangedData`；context 使用递归 `ApprovalEvidenceField` / `ApprovalEvidenceValue`，不通过 Envelope、`map[string]any`、JSON 或 reflect 跨组件传输。
- Consumer contract：Governance 仅订阅 pending approval-created 通知；Eventstream 逐 topic 订阅 Approval 具体 data 并在自身边界生成 EventRecord。

## 已知缺口

- X02 repository 直连已关闭；Approval 不再注入或访问 EvidenceRecordRepository。
- C08 目录、组件 ID、入口装配与导入路径迁移已完成。
- Approval notification 的 Envelope 兼容投递已删除；created/resolved 已直接使用 magicCommon EventHub 和 Approval 自有具体 data。
- 批量审批、重复 approvalId 和跨 workspace scope 仍需要持续 pairwise 回归。

## 边界约束

- runtime approval 必须携带 workspaceId/runId/stepId/action scope。
- 跨 workspace 重复 approval ID 读取必须有 scope 或返回 conflict。
- 授权通过不等于验证通过。
- 与 Orchestrator 的 evidence 交互只能使用 EventHub；禁止恢复 EvidenceRecord repository、writer callback 或 adapter 注入。
