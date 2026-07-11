# Eventstream Module

## 功能定位

`kernel/eventstream` 是 WorkOrch EventRecord 存储、查询、清理与审计回放输入面的 owner module。它保存业务事件和 runtime 事件的可查询记录。

## 拥有边界

- 拥有 EventRecord 的存储、查询和 purge command。
- Event payload 是审计与投影输入，不是 owner state mutation。
- 不修复 Workspace、Run、Result 或其他 owner 的正式状态。

## 主要交互

- 接收 `blocks/workspace`、`kernel/run`、`kernel/orchestrator`、`blocks/result`、`blocks/decision`、`blocks/approval` 等模块发布的事件。
- 被 `application/workorchd` 用于 projection、timeline、SSE、audit 和 snapshot 构建。
- 与 runtime progress/evidence/result event 共同形成观测链路。

## 组件交互合同

- Input contract: EventRecord 写入必须包含 topic、sourceModule、subject、scope、version 和 bounded payload/ref。
- Query contract: query 通过 Eventstream 自有 `EventStreamCommand` / `EventStreamCommandResult` 返回有序且有界的 `EventRecordData`；递归 payload 使用具体 `EventValue` 类型，不通过 `map[string]any`、JSON 或 reflect 跨组件传输。
- Notification contract: EventRecord 持久化成功后向 `/eventstream/consumers/#` 投递 `EventRecordAppendedData`；消费方必须使用唯一 consumer match ID 订阅，避免 magicCommon observer 注册覆盖。
- Purge contract: purge 返回删除数量与诊断信息。
- Owner contract: Eventstream 不接受通过 event payload 修复 owner state 的请求。
- Projection contract: application 消费 EventRecord 只能作为投影输入或审计输入。
- Scope contract: workspace/run 相关 purge 必须携带 scope，避免跨 workspace 误删。

## 已知缺口

- EventRecord 只承担审计与投影状态，未发现通过该状态反向修复 owner 的入口。
- EventRecord query result、appended notification、SSE/replay、运行状态缓存、治理续跑 watcher、observed collector、workspace projection manager 和 embedded WorkOrch backend 已迁移到具体 `EventRecordData` / `EventRecordAppendedData`。
- Decision owner 已作为首个输入切片完成迁移：Eventstream 直接订阅 `blocks/decision/pkg/events` 的 `TicketChangedData` / `TicketResolvedData`，在内部边界恢复审计 payload，并统一生成 workspace 连续 revision。
- Approval owner 已完成输入迁移：Eventstream 直接订阅 `blocks/approval/pkg/events` 的 `ApprovalChangedData`，递归 context 只在 Eventstream 内部投影边界恢复为审计 payload。
- Governance owner 已完成输出迁移：AutoGovernance trace 通过具体 `GovernanceData` 投递，Eventstream 在内部生成治理审计 payload；Governance 本身不再发布 Envelope notification。
- Run owner 输入已完成当前切片迁移：Run service 与生产 EventDrivenDispatch Orchestrator 投递 `RunStateChangedData`、`StepChangedData`、`LLMStreamProgressData`、`LLMUsageRecordedData`，Eventstream 在内部恢复审计 payload、统一生成连续 revision 并保留真实 sourceModule。
- Workspace owner 已完成输入迁移：Workspace created/state-changed/deleted、Goal defined、DoD defined 分别使用 `WorkspaceChangedData`、`GoalDefinedData`、`DoDDefinedData`；Workspace block 生产路径不再发布 Envelope，Eventstream 仅在内部恢复 workspace-definition 审计 payload。
- Result/Artifact finalization 输入已完成迁移：Result 状态使用 `ResultChangedData`，Artifact created 使用 `ArtifactCreatedData`；Eventstream 逐 topic 生成结果与交付物审计 payload，不接收 map/any 合同。
- Memory 输入已完成迁移：Memory created/updated/deleted 使用 `MemoryChangedData`，recall recorded 使用 `MemoryRecallRecordedData`；Eventstream 保留实际 application/orchestrator source，并仅在内部恢复 memory 审计 payload。
- Orchestrator detail 输入已完成一个切片：Patch 生命周期使用 `PatchChangedData`，ArtifactContinuation 生命周期使用 `ArtifactContinuationChangedData`；Eventstream 仅在内部恢复 patch/continuation 审计 payload。
- ContractViolation 输入已完成迁移：recorded/retry-requested/escalated 使用 `ContractViolationData`，递归诊断只在 Eventstream 内部恢复为审计 payload。
- `stage_changed`、`graph_dispatched`、`repair_started`、`replan_triggered` 未发现生产发布点，已从 Orchestrator topic/contract 与 legacy 版本矩阵中删除，Eventstream 不为失效声明建立订阅。
- 输入采集链已全部改为逐 topic 导入投递组件具体 data；Eventstream 不再持有 legacy `events.Hub`，`SubscribeWorkorch`、`eventRecordFromEnvelope` 与公开 topic 聚合订阅 API 已删除。
- Eventstream test service helper 也只创建并返回 magicCommon Hub，不再接收或忽略 legacy `events.Hub` 参数。
- application/Observed/Server 的 Eventstream 测试输入直接构造 `EventRecordData` 或本地 HTTP fixture；全仓测试不再通过 legacy Envelope 转换生成 EventRecord。
- 大 payload 默认投影预算仍应持续验证。

## 边界约束

- 查询和 purge 必须有 bounded limit 与必要 workspace/run scope。
- 大 payload 不应默认内联到 projection。
- Eventstream 不是唯一真相源，不能替代 owner repository。
