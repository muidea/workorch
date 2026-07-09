# Decision Module

## 功能定位

`kernel/decision` 是 DecisionTicket 与 Resolution 的 owner module。它负责创建、查询、解析、拒绝和删除运行期或产品面需要人工决策的事项。

## 拥有边界

- 拥有 DecisionTicket、DecisionResolution 的正式状态。
- Resolution 只能补充 continuation context 或治理选择，不能覆盖原始 runtime stage contract。
- 不拥有 Run 状态机、Approval 授权结果或 Result 接受判断。

## 主要交互

- 被 `kernel/orchestrator` 和 `kernel/governance` 用于创建治理决策 ticket。
- 与 `kernel/run` 协作表达 `blocked_on_decision` 状态。
- Resolution 被 `kernel/orchestrator` 消费后继续原 stage/repair/replan 流程。
- 被 `application/workorchd` 和 Web/TUI/API 读取、展示和提交解析命令。

## 组件交互合同

- Input contract: ticket/resolution command 必须包含 ticketId，并在 runtime 场景携带 workspaceId/runId/stepId。
- Output contract: command response 必须返回 open/resolved/rejected 状态、resolution record 和 lineage。
- Orchestrator contract: decision resolution 只能补充 continuation intent，不覆盖原 requiredOutput 或 stage contract。
- Run contract: 只有正式 ticket 创建成功后，Run 才能进入 `blocked_on_decision`。
- Application contract: 无 scope API 只能解析唯一 open ticket，否则必须返回 conflict。

## 已知缺口

- 当前盘点未发现 Decision module 自身职责缺失；批量解析、重复 ticketId 和跨 workspace scope 仍需要持续 pairwise 回归。

## 边界约束

- runtime 相关 ticket/resolution 必须携带 workspaceId/runId/stepId scope。
- 不能把普通可恢复 runtime gap 升级成人工决策。
- 重复 ticket/resolution 必须幂等或明确 conflict。
