# Decision Stateful Block

## 功能定位

`blocks/decision` 是 DecisionTicket 与 Resolution 的单一治理资源聚合 block。它管理 ticket/resolution 正式状态，但不承担 Run、Approval、Result 或 runtime continuation 的跨组件编排。

## 拥有边界

- 拥有 DecisionTicket、DecisionResolution 的正式状态。
- 只访问自身 Ticket/Resolution repositories，不持有其他 module/block 的 service、repository、writer callback 或 adapter。
- Resolution 只能补充 continuation context 或治理选择，不能覆盖原始 runtime stage contract。
- 不拥有 Run 状态机、Approval 授权结果或 Result 接受判断。

## 主要交互

- 被 `kernel/orchestrator` 和 `kernel/governance` 用于创建治理决策 ticket。
- 与 `kernel/run` 协作表达 `blocked_on_decision` 状态。
- Resolution 被 `kernel/orchestrator` 消费后继续原 stage/repair/replan 流程。
- Resolution 成功落库后，由 Decision 投递 `/decision/command/record-evidence`，`kernel/orchestrator` 订阅并写入 EvidenceRecord。
- 被 `application/workorchd` 和 Web/TUI/API 读取、展示和提交解析命令。

## 组件交互合同

- Command contract：调用方导入 `blocks/decision/pkg/events`，通过 magicCommon EventHub 投递具体 ticket/resolution command，并接收 `DecisionCommandResult`。
- Input contract：ticket/resolution command 必须包含 ticketId，并在 runtime 场景携带 workspaceId/runId/stepId。
- Output contract：command result 必须返回 open/resolved/rejected 状态、resolution record 和 lineage。
- Orchestrator contract：decision resolution 只能补充 continuation intent，不覆盖原 requiredOutput 或 stage contract。
- Run contract：只有正式 ticket 创建成功后，Run 才能进入 `blocked_on_decision`；双方仅通过 EventHub 交互。
- Application contract：无 scope API 只能解析唯一 open ticket，否则必须返回 conflict。
- Evidence contract：`RecordDecisionEvidenceCommand`、嵌套 `DecisionEvidenceValue` 与 `RecordDecisionEvidenceResult` 均由 Decision 的 `pkg/events` 定义；Orchestrator 按需订阅，同步调用必须收到匹配的 evidenceRecordId。
- Notification contract：Decision 通过 `/decision/consumers/#` 异步投递 `TicketChangedData` 或 `TicketResolvedData`；resolution payload 使用递归 `DecisionEvidenceField` / `DecisionEvidenceValue`，不通过 Envelope、`map[string]any`、JSON 或 reflect 跨组件传输。
- Consumer contract：Governance 仅订阅 `TicketChangedData` 的 ticket-created topic；Eventstream 逐 topic 订阅 Decision 具体 data 并在自身边界生成 EventRecord，消费方不得恢复通用 Envelope 订阅。

## 已知缺口

- X02 repository 直连已关闭；Decision 不再注入或访问 EvidenceRecordRepository。
- C07 目录、组件 ID、入口装配与导入路径迁移已完成。
- Decision notification 的 Envelope 兼容投递已删除；created/updated/resolved 已直接使用 magicCommon EventHub 和 Decision 自有具体 data。
- 批量解析、重复 ticketId 和跨 workspace scope 仍需要持续 pairwise 回归。

## 边界约束

- runtime 相关 ticket/resolution 必须携带 workspaceId/runId/stepId scope。
- 不能把普通可恢复 runtime gap 升级成人工决策。
- 重复 ticket/resolution 必须幂等或明确 conflict。
- 与 Orchestrator 的 evidence 交互只能使用 EventHub；禁止恢复 EvidenceRecord repository、writer callback 或 adapter 注入。
