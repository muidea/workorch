# WorkOrch Service API

Status: active

Type: product-interface

Last Updated: 2026-05-30

## Purpose

本文件作为 WorkOrch HTTP API 的统一入口文档。

## API Shape

当前 HTTP API 使用统一 JSON envelope 风格。常见外层字段包括：

- `service`
- `backend`
- `dataRoot`
- 资源本体字段，例如 `workspace`、`run`、`snapshot`、`observability`、`page`

当前以 `internal/modules/application/workorchd/service/server/http.go` 中的
response struct 为准，而不是单独维护 schema registry。

## Resource Families

### Operational

- health / ready / metrics / configuration

### Workspace-Facing

- workspace
- workspace snapshot
- taskProgress
- contextBundle
- taskDecompositions
- memoryRecallTraces
- soulBindings

### Run-Facing

- run
- run snapshot
- run status
- run observability
- steps / toolCalls / runtimeCheckpoints / results / patches /
  validationResults / artifacts

### Governance

- decisions
- approvals

### Context Registry

- souls
- workspace templates
- skills
- mcp servers
- mcp capabilities
- memory

## Endpoint Family To Response Family

| Endpoint Family | 主要响应族 |
| --- | --- |
| workspace singleton | `workspaceResponse` |
| workspace list | `workspacesResponse` |
| workspace snapshot | `workspaceSnapshotResponse` |
| workspace task/context/decomposition | `taskProgressResponse` / `contextBundleResponse` / `taskDecompositionsResponse` |
| workspace memory recall | `memoryRecallTracesResponse` |
| run singleton | `runInstanceResponse` |
| run snapshot / status / observability | `runSnapshotResponse` / `runStatusResponse` / `runObservabilityResponse` |
| run detail page | `runStepsResponse` / `runToolCallsResponse` / `runRuntimeCheckpointsResponse` / `runResultsResponse` / `runPatchesResponse` / `runValidationResultsResponse` / `runArtifactsResponse` |
| governance list/detail | `decisionsResponse` / `decisionResponse` / `approvalsResponse` / `approvalResponse` |
| registry singleton/list | `soulResponse` / `soulsResponse` / `workspaceTemplateResponse` / `workspaceTemplatesResponse` / `skillResponse` / `skillsResponse` / `mcpServerResponse` / `mcpServersResponse` / `mcpCapabilityResponse` / `mcpCapabilitiesResponse` / `memoryItemResponse` / `memoryItemsResponse` |
| payload / event / content | `eventRecordsResponse` / raw content endpoints |

## Endpoint Groups

### System And Operational

- `GET /debug/pprof/**`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`

### Configuration

- `GET /api/v1/workorch/configuration`
- `PUT /api/v1/workorch/configuration/server`
- `PUT /api/v1/workorch/configuration/llm`
- `PUT /api/v1/workorch/configuration/web-search`

### Workspace

- `GET /api/v1/workorch/workspaces`
- `POST /api/v1/workorch/workspaces`
- `GET /api/v1/workorch/workspaces/:workspaceId`
- `DELETE /api/v1/workorch/workspaces/:workspaceId`
- `GET /api/v1/workorch/workspaces/:workspaceId/snapshot`
- `GET /api/v1/workorch/workspaces/:workspaceId/task-progress`
- `GET /api/v1/workorch/workspaces/:workspaceId/context-bundle`
- `GET /api/v1/workorch/workspaces/:workspaceId/task-decompositions`
- `GET /api/v1/workorch/workspaces/:workspaceId/memory-recalls`
- `GET /api/v1/workorch/workspaces/:workspaceId/soul-bindings`
- `PUT /api/v1/workorch/workspaces/:workspaceId/soul-bindings`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs`
- `POST /api/v1/workorch/workspaces/:workspaceId/runs`
- `POST /api/v1/workorch/workspaces/:workspaceId/execution-activations`
- `POST /api/v1/workorch/workspaces/:workspaceId/goal-dod-quality-check`

### Goal And DoD

- `GET /api/v1/workorch/workspaces/:workspaceId/goals`
- `POST /api/v1/workorch/workspaces/:workspaceId/goals`
- `GET /api/v1/workorch/goals/:goalId`
- `PUT /api/v1/workorch/goals/:goalId`
- `GET /api/v1/workorch/workspaces/:workspaceId/dods`
- `POST /api/v1/workorch/workspaces/:workspaceId/dods`
- `GET /api/v1/workorch/dods/:dodId`
- `PUT /api/v1/workorch/dods/:dodId`

### Run And Run Detail

- `GET /api/v1/workorch/runs`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/snapshot`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/status`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/observability`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/task-progress`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/context-bundle`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/task-decompositions`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/memory-recalls`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/steps`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/tool-calls`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/runtime-checkpoints`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/results`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/patches`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/validation-results`
- `GET /api/v1/workorch/workspaces/:workspaceId/runs/:runId/artifacts`
- `POST /api/v1/workorch/workspaces/:workspaceId/runs/:runId/start`
- `POST /api/v1/workorch/workspaces/:workspaceId/runs/:runId/resume`

### Decision And Approval

- `GET /api/v1/workorch/decisions`
- `GET /api/v1/workorch/decisions/:ticketId`
- `POST /api/v1/workorch/decisions/:ticketId/resolve`
- `POST /api/v1/workorch/decisions/resolve-batch`
- `GET /api/v1/workorch/approvals`
- `GET /api/v1/workorch/approvals/:approvalId`
- `POST /api/v1/workorch/approvals/:approvalId/approve`
- `POST /api/v1/workorch/approvals/:approvalId/reject`
- `POST /api/v1/workorch/approvals/resolve-batch`

### Soul, Template, Skill

- `GET /api/v1/workorch/souls`
- `POST /api/v1/workorch/souls`
- `GET /api/v1/workorch/souls/:soulId`
- `PUT /api/v1/workorch/souls/:soulId`
- `DELETE /api/v1/workorch/souls/:soulId`
- `GET /api/v1/workorch/workspace-templates`
- `POST /api/v1/workorch/workspace-templates`
- `PUT /api/v1/workorch/workspace-templates/:templateId`
- `DELETE /api/v1/workorch/workspace-templates/:templateId`
- `GET /api/v1/workorch/skills`
- `POST /api/v1/workorch/skills`
- `GET /api/v1/workorch/skills/:skillId`

### MCP And Capability

- `GET /api/v1/workorch/mcp/servers`
- `POST /api/v1/workorch/mcp/servers`
- `GET /api/v1/workorch/mcp/servers/:serverId`
- `PUT /api/v1/workorch/mcp/servers/:serverId`
- `GET /api/v1/workorch/mcp/servers/:serverId/tools`
- `GET /api/v1/workorch/mcp/servers/:serverId/resources`
- `GET /api/v1/workorch/mcp/servers/:serverId/prompts`
- `GET /api/v1/workorch/mcp/capabilities`
- `POST /api/v1/workorch/mcp/capabilities`
- `GET /api/v1/workorch/mcp/capabilities/:capabilityId`
- `PUT /api/v1/workorch/mcp/capabilities/:capabilityId`

### Memory

- `GET /api/v1/workorch/memory`
- `POST /api/v1/workorch/workspaces/:workspaceId/memory`
- `GET /api/v1/workorch/memory/:memoryId`
- `PUT /api/v1/workorch/memory/:memoryId`
- `DELETE /api/v1/workorch/memory/:memoryId`

### Payload, Artifact, Event

- `GET /api/v1/workorch/artifacts/content`
- `GET /api/v1/workorch/runtime/payloads`
- `GET /api/v1/workorch/events`
- `GET /api/v1/workorch/events/stream`

### Console And Static

- `GET /favicon.ico`
- `GET /console`
- `GET /console/`
- `GET /console/**`

## Projection Levels

### Summary

典型入口：

- `GET /workspaces`
- `GET /runs`
- `GET /decisions`
- `GET /approvals`

特点：

- 面向列表/概览
- 只返回 compact metadata
- 不默认展开 raw payload

### Detail / Snapshot

典型入口：

- `GET /workspaces/:id`
- `GET /workspaces/:id/snapshot`
- `GET /runs/:id`
- `GET /runs/:id/snapshot`
- `GET /runs/:id/observability`

特点：

- 返回结构化详情投影
- 允许包含 result、taskProgress、contextBundle、observability 等聚合视图
- 仍应避免无界 raw payload 加载

### Raw / Content

典型入口：

- `GET /artifacts/content`
- `GET /runtime/payloads`
- `GET /events/stream`

特点：

- 显式打开 artifact 正文、runtime payload 或 SSE stream
- 不应作为默认 summary/detail 读取路径

## Response Families

当前 `http.go` 中的响应结构大致分为：

- singleton resource
  - `workspaceResponse`
  - `goalResponse`
  - `dodResponse`
  - `runInstanceResponse`
  - `decisionResponse`
  - `approvalResponse`
  - `soulResponse`
  - `workspaceTemplateResponse`
  - `skillResponse`
  - `mcpServerResponse`
  - `mcpCapabilityResponse`
  - `memoryItemResponse`
- list resource
  - `workspacesResponse`
  - `goalsResponse`
  - `dodsResponse`
  - `runsResponse`
  - `soulsResponse`
  - `skillsResponse`
  - `mcpServersResponse`
  - `mcpCapabilitiesResponse`
  - `memoryItemsResponse`
- snapshot / observability / projection
  - `workspaceSnapshotResponse`
  - `runSnapshotResponse`
  - `runObservabilityResponse`
  - `taskProgressResponse`
  - `contextBundleResponse`
  - `taskDecompositionsResponse`
  - `memoryRecallTracesResponse`
  - `workspaceSoulBindingsResponse`
  - `workspaceExecutionActivationResponse`
- paginated detail
  - `runStepsResponse`
  - `runToolCallsResponse`
  - `runRuntimeCheckpointsResponse`
  - `runResultsResponse`
  - `runPatchesResponse`
  - `runValidationResultsResponse`
  - `runArtifactsResponse`
  - `decisionsResponse`
  - `approvalsResponse`
- payload / event / content
  - `eventRecordsResponse`
  - `artifacts/content`
  - `runtime/payloads`
  - `events/stream`

### Key Field Direction

关键响应族的字段语义方向可先按以下层次理解：

- `workspaceResponse` / `runInstanceResponse`
  - 面向单资源本体读取
- `workspaceSnapshotResponse` / `runSnapshotResponse`
  - 面向聚合读模型视图
- `runStatusResponse`
  - 面向轻量状态与 `waitState` / `waitTimeoutMillis` 场景
- `runObservabilityResponse`
  - 面向 timeline / evidence / closure / usage 的聚合观察视图
- `taskProgressResponse` / `contextBundleResponse`
  - 面向收敛视角，而不是原始 evidence 全量展开
- `eventRecordsResponse`
  - 面向事件列表，不替代实时 `events/stream`

### First-Cut Field Semantics

| Response | 关键字段 | 当前语义方向 |
| --- | --- | --- |
| `workspaceResponse` | `workspace` | Workspace 单体本体 |
| `workspaceSnapshotResponse` | `snapshot.workspace` / `snapshot.runs` / `snapshot.outcome` | Workspace 的聚合读模型 |
| `runInstanceResponse` | `run` | Run 单体本体 |
| `runSnapshotResponse` | `snapshot.run` / `snapshot.outcome` / `snapshot.taskProgress` / `snapshot.results` / `snapshot.artifacts` | 当前 Run 的聚合详情 |
| `runStatusResponse` | `status` / `waitMatched` / `waitElapsedMs` | 轻量状态与 wait 查询结果 |
| `runObservabilityResponse` | `observability.timeline` / `observability.evidence` / `observability.llmUsageSummary` | 观测/证据/usage 聚合视图 |
| `taskProgressResponse` | `taskProgress.milestones` / `taskProgress.blockers` / `taskProgress.recovery` | 收敛视角的进度投影 |
| `contextBundleResponse` | `contextBundle.sections` / `contextBundle.recentEvidence` / `contextBundle.compressedContext` | 压缩上下文与恢复视图 |
| `eventRecordsResponse` | `eventRecords` | 事件列表页，不等于实时 SSE |

### Snapshot / Projection Field Direction

| Projection | 关键字段方向 |
| --- | --- |
| `RunSnapshot` | `workspace`、`run`、`outcome`、`taskProgress`、`results`、`artifacts`、`closureTraces`、`doDEvidence`、`resultQuality`、`llmUsageSummary`、`memoryRecallTraces` |
| `RunStatus` | `state`、`currentStep*`、`blockingTicketId`、`blockingApprovalId`、`failureReason` |
| `RunObservability` | `timeline`、`evidence`、`llmContextBudgets`、`llmUsageTraces`、`selectedCapabilities`、`usedCapabilities` |
| `TaskProgress` | `milestones`、`items`、`blockers`、`recovery`、`repairLoop` |
| `ContextBundle` | `sections`、`recentEvidence`、`compressedContext`、`recovery` |

### Additional Projection Fields To Track

| Projection | 关键字段方向 |
| --- | --- |
| `WorkspaceSnapshot` | `workspace`、`outcome`、`runs`、`results`、`artifacts`、`taskProgress` |
| `DecisionPage` / `ApprovalPage` | `limit`、`cursorAfterId`、`nextCursorAfterId`、`hasMore` |
| `TaskDecomposition` | `rootStepId`、`state`、`nodeTotal`、`nodeCompleted`、`evidenceRefs` |
| `ExecutionSection` | `relatedDoDItems`、`actualEvidenceRefs`、`decisionRefs`、`approvalRefs` |

### Detail Page Field Direction

| Page Model | 关键字段 | 当前语义方向 |
| --- | --- | --- |
| `DecisionPage` | `decisions` / `limit` / `cursorAfterId` / `nextCursorAfterId` / `hasMore` | Decision 队列分页 |
| `ApprovalPage` | `approvals` / `limit` / `cursorAfterId` / `nextCursorAfterId` / `hasMore` | Approval 队列分页 |
| `StepPage` | `steps` / `page fields` | Step detail 分页 |
| `ToolCallPage` | `toolCalls` / `page fields` | ToolCall detail 分页 |
| `RuntimeCheckpointPage` | `runtimeCheckpoints` / `page fields` | Checkpoint detail 分页 |
| `ResultPage` | `results` / `page fields` | Result detail 分页 |
| `PatchPage` | `patches` / `page fields` | Patch detail 分页 |
| `ValidationResultPage` | `validationResults` / `page fields` | Validation detail 分页 |
| `ArtifactPage` | `artifacts` / `page fields` | Artifact detail 分页 |

### Aggregate Model Semantics

| Model | 关键字段 | 当前语义方向 |
| --- | --- | --- |
| `WorkspaceSnapshot` | `workspace` / `outcome` / `runs` / `results` / `artifacts` / `taskProgress` | workspace 级聚合读模型 |
| `RunSnapshot` | `run` / `outcome` / `taskProgress` / `results` / `artifacts` / `llmUsageSummary` | run 级聚合详情 |
| `RunObservability` | `timeline` / `evidence` / `llmContextBudgets` / `llmUsageTraces` / `selectedCapabilities` | 观测与证据视图 |
| `TaskProgress` | `milestones` / `items` / `blockers` / `recovery` / `repairLoop` | 当前收敛状态投影 |
| `ContextBundle` | `compressedContext` / `sections` / `recentEvidence` / `compressionCandidates` | 上下文恢复与压缩视图 |
| `MemoryRecallTrace` | `source` / `stage` / `selectedMemoryIds` / `candidateMemoryIds` / `excludedMemoryIds` | recall 过程证据与选择解释 |
| `Decision` / `Approval` | `reason` / `state` / `resolutionPayload` / `autoGovernance` | 治理阻塞与恢复视图 |

### Response Field Semantics

#### `runStatusResponse`

| 字段 | 语义方向 |
| --- | --- |
| `status.id` / `status.workspaceId` | 当前 run 身份 |
| `status.state` | 轻量运行态；适合轮询/等待，不替代 snapshot |
| `status.currentStepId` / `currentStepKind` / `currentStepState` | 当前前台执行步的快速视图 |
| `status.stepCount` / `decisionCount` / `approvalCount` | 轻量计数，不等于全量 detail page |
| `status.blockingTicketId` / `blockingApprovalId` | 当前治理阻塞引用 |
| `status.failureReason` | 当前终态或阻塞的简洁原因 |
| `waitMatched` / `waitElapsedMs` | 仅在 wait 查询时表达服务端等待结果 |

#### `runSnapshotResponse`

| 字段 | 语义方向 |
| --- | --- |
| `snapshot.workspace` | 当前 run 所属 workspace 的必要聚合上下文 |
| `snapshot.run` | run 本体 |
| `snapshot.outcome` | run 级正式 closure 投影 |
| `snapshot.taskProgress` / `snapshot.repairLoop` | 收敛状态与恢复循环投影 |
| `snapshot.taskPlans` / `taskDecompositions` | planning / graph / revision 投影 |
| `snapshot.steps` / `toolCalls` / `patches` / `validationResults` / `runtimeCheckpoints` | 过程证据集合 |
| `snapshot.results` / `artifacts` | 正式结果与交付物集合 |
| `snapshot.closureTraces` / `doDEvidence` / `resultQuality` | acceptance / quality / closure 诊断视图 |
| `snapshot.memoryRecallTraces` | recall 进入运行态的证据 |
| `snapshot.selectedCapabilities` / `usedCapabilities` | capability 选择与使用投影 |
| `snapshot.eventRecords` | 仅作为补充事件视图，不是默认 truth source |
| `snapshot.decisions` / `approvals` | 当前 run 关联的治理对象聚合 |

#### `runObservabilityResponse`

| 字段 | 语义方向 |
| --- | --- |
| `observability.run` / `outcome` / `progress` | 运行态总览 |
| `observability.taskProgress` / `repairLoop` | 当前修复与恢复状态 |
| `observability.executionSections` | DoD / evidence / section 聚合 |
| `observability.llmContextBudgets` / `llmUsageTraces` / `llmUsageSummary` | LLM 请求预算与 usage 投影 |
| `observability.memoryRecallTraces` | recall 轨迹 |
| `observability.gitOperations` / `webSources` | runtime capability 专项证据摘要 |
| `observability.closureTraces` / `doDEvidence` / `resultQuality` | closure 质量与验收轨迹 |
| `observability.evidence` | tool/patch/validation/checkpoint/result/artifact/event 计数摘要 |
| `observability.latestCheckpoint` | 最新 checkpoint 快照 |
| `observability.timeline` | 跨 event/step/result 的时间线视图 |

#### `taskProgressResponse`

| 字段 | 语义方向 |
| --- | --- |
| `taskProgress.state` / `summary` | 当前收敛态摘要 |
| `currentStep*` / `currentMilestone` | 当前前台工作焦点 |
| `blockingTicketId` / `blockingApprovalId` | 当前治理阻塞 |
| `milestones` / `items` | 目标与工作项分层视图 |
| `blockers` | 当前阻塞面 |
| `recovery` | 下一恢复动作建议 |
| `repairLoop` | repair 次数、升级、缺口与下一步 |
| `evidenceRefs` | 支撑当前 progress 投影的证据引用 |

#### `contextBundleResponse`

| 字段 | 语义方向 |
| --- | --- |
| `contextBundle.mode` / `summary` | 当前上下文压缩模式和摘要 |
| `compressedContext` | 面向 LLM/request 的压缩上下文基线 |
| `taskProgress` / `recovery` | 当前收敛态与恢复建议 |
| `latestCheckpoint` | 最近 checkpoint |
| `sections` | 结构化上下文分段 |
| `recentEvidence` | 最近高价值证据 |
| `compressionCandidates` | 可继续压缩的区段建议 |

#### `workspaceSnapshotResponse`

| 字段 | 语义方向 |
| --- | --- |
| `snapshot.workspace` | workspace 本体 |
| `snapshot.run` / `step` / `result` | 当前活跃或最近焦点对象 |
| `snapshot.outcome` | workspace 级正式 closure 投影 |
| `snapshot.taskProgress` | workspace 级收敛态 |
| `snapshot.goals` / `dods` | 当前有效目标与验收基线 |
| `snapshot.runs` / `results` / `artifacts` / `patches` | 运行与交付聚合面 |
| `snapshot.memoryRecallTraces` / `executionSections` | recall 与 section 投影 |
| `snapshot.decisions` / `approvals` | workspace 级治理队列聚合 |
| `snapshot.llmUsageSummary` | workspace 级 usage 汇总 |

#### `DecisionPage` / `ApprovalPage`

| 字段 | 语义方向 |
| --- | --- |
| `workspaceId` / `runId` | 当前分页过滤作用域 |
| `decisions` / `approvals` | 当前页对象列表 |
| `limit` / `cursorAfterId` / `nextCursorAfterId` / `hasMore` | cursor-based 分页协议 |

#### `taskDecompositionsResponse`

| 字段 | 语义方向 |
| --- | --- |
| `taskDecompositions[].id` / `workspaceId` / `runId` | decomposition 身份与归属 |
| `rootStepId` | graph / decomposition 根步骤 |
| `state` / `summary` / `graphBranch` | 当前 decomposition 状态摘要 |
| `nodeTotal` / `nodeCompleted` / `nodeFailed` / `nodeRunning` | 节点级进度计数 |
| `nodes` | graph node 的结构化投影 |
| `evidenceRefs` | 支撑当前 decomposition 摘要的证据引用 |

#### `memoryRecallTracesResponse`

| 字段 | 语义方向 |
| --- | --- |
| `memoryRecallTraces[].workspaceId` / `runId` / `stepId` / `graphNodeId` | recall 发生位置 |
| `stage` / `source` / `policy` / `status` | recall 所处阶段、来源、策略与结果 |
| `candidateMemoryIds` / `selectedMemoryIds` / `excludedMemoryIds` | recall 选择过程 |
| `retrievedMemoryIds` / `retrievedMemoryHits` | recall 实际命中的 memory 与得分摘要 |
| `reason` / `occurredAt` / `evidenceRef` | recall 原因、时间与证据引用 |

#### `workspaceSoulBindingsResponse`

| 字段 | 语义方向 |
| --- | --- |
| `soulBindings.defaultSoulId` | workspace 默认 Soul |
| `soulBindings.bindings` | 各 stage 的显式 Soul 绑定 |
| `soulBindings.priorityPolicy` | 默认 Soul 与阶段绑定的优先级说明 |

#### `workspaceExecutionActivationResponse`

| 字段 | 语义方向 |
| --- | --- |
| `activation.workspace` / `activation.run` | activation 生成时的 workspace / run 基线 |
| `activation.executionContext` / `executionContract` | 本次 run 生效的中立执行合同 |
| `activation.soulBindings` / `selectedCapabilities` / `memoryRefs` | 本次 activation 选中的上下文与能力 |
| `activation.taskProgress` / `outcome` | activation 返回的 lightweight projection |

#### `Decision` / `Approval` Singleton Responses

| 字段 | 语义方向 |
| --- | --- |
| `decision.id` / `approval.id` | 治理对象身份 |
| `workspaceId` / `runId` / `stepId` | 归属作用域 |
| `reason` / `question` / `action` | 当前治理意图 |
| `context` | 结构化治理上下文 |
| `state` | 当前治理状态 |
| `resolutionId` / `resolutionType` / `resolutionPayload` | Decision 的已处理结果 |
| `autoGovernance` | 自动治理摘要，不替代正式治理对象 |

#### `eventRecordsResponse`

| 字段 | 语义方向 |
| --- | --- |
| `eventRecords[].id` / `version` / `topic` | 事件身份与主题 |
| `occurredAt` / `sourceModule` | 事件来源时间线 |
| `subjectType` / `subjectId` / `workspaceId` / `runId` / `stepId` | 事件作用域 |
| `payload` / `payloadSummary` / `payloadRef` | 默认列表只应承载结构化摘要；大正文应优先通过 ref/detail 打开 |

#### Trace / Capability Field Semantics

| Model | 字段 | 语义方向 |
| --- | --- | --- |
| `RuntimeCapability` | `id` / `serverId` / `kind` / `displayName` / `riskLevel` / `approvalPolicy` / `enabled` / `status` / `evidenceRefs` | capability 的 catalog/selection/use 三用投影 |
| `MemoryRecallTrace` | `source` / `policy` / `status` / `selectedMemoryIds` / `candidateMemoryIds` / `reason` | recall 选择解释，不等于 Memory 本体 |
| `GitOperationTrace` | `operation` / `branch` / `headCommit` / `changedFiles` / `summary` | Git 证据摘要 |
| `WebSourceTrace` | `query` / `providerId` / `sources` / `fetchedSources` / `approvalId` / `policyDecision` | search/fetch 来源链摘要 |
| `LLMUsageTrace` | `providerId` / `model` / `operation` / `inputTokens` / `outputTokens` / `estimated` / `failureClass` | 单次 LLM 请求 usage 记录 |
| `LLMUsageSummary` | `entryTotal` / `inputTokens` / `outputTokens` / `cache*` / `providerModelBreakdown` | usage 聚合摘要 |
| `ArtifactContinuation` | `artifactId` / `targetRef` / `state` / `expectedSequence` / `receivedSequences` / `finalReceived` | incremental artifact 生命周期投影 |
| `ClosureTrace` | `graphFollowUp` / `assessmentOnly` / `missingDoDItems` / `evidenceRefs` | closure / follow-up / assessment-only 诊断轨迹 |
| `RecoveryPlan` | `mode` / `nextAction` / `blockingTicketId` / `blockingApprovalId` / `failureClass` / `failureSignature` | 当前恢复建议与阻塞面 |
| `RepairLoop` | `state` / `attemptCount` / `lastFailure*` / `nextAction` / `escalation*` / `evidenceRefs` | repair 循环与升级诊断 |
| `AutoGovernanceSummary` | `traceId` / `action` / `status` / `riskLevel` / `policyMatched` / `reason` | 自动治理决策摘要 |
| `DoDAssessment` / `DoDCoverage` | `satisfied` / `items` / `summary` | Goal/DoD/acceptance/deliverable 覆盖评估 |
| `ResultSummary` | `implementationSummary` / `verificationSummary` / `residualRisks` / `nextActions` | 最终结果摘要补充层 |

补充：

- `MemoryRecallTrace.retrievedMemoryHits`
  - 用于解释 recall 命中的摘要与分值，不等于完整 Memory 正文
- `RuntimeCapability.useCount`
  - 是使用投影，不是权限声明本体
- `RepairLoop`
  - 是 read-model 诊断对象，不直接驱动调度

#### Singleton Resource Responses

| Response | 字段 | 语义方向 |
| --- | --- | --- |
| `workspaceResponse` | `workspace.id` / `title` / `deliverableDirectory` / `templateId` / `defaultSoulId` / `soulBindings` | workspace 本体和默认执行配置 |
| `goalResponse` | `goal.id` / `workspaceId` / `text` | 单条 Goal 本体 |
| `dodResponse` | `dod.id` / `workspaceId` / `items` / `state` | 单条 DoD 本体 |
| `runInstanceResponse` | `run.id` / `workspaceId` / `goalId` / `doDId` / `initialStepKind` / `executionContext` / `priority` / `state` | 单条 Run 本体 |
| `soulResponse` | `soul.id` / `name` / `soulMd` / allowlist fields | Soul 声明对象 |
| `workspaceTemplateResponse` | `workspaceTemplate.id` / `goalText` / `doDItems` / `defaultSoulId` / `soulBindings` / `contextHints` / `evidencePolicies` | template 初始化基线 |
| `skillResponse` | `skill.id` / `instruction` / `triggerRules` / capability requirements | Skill 触发与依赖声明 |
| `mcpServerResponse` | `server.id` / `transport` / `endpoint` / `command` / `args` / `env` / allowlist / timeout / session policy | MCP server 连接与策略基线 |
| `mcpCapabilityResponse` | `capability.id` / `serverId` / `kind` / `displayName` / `stageHints` / `riskLevel` / `approvalPolicy` | 单条 capability 声明 |
| `memoryItemResponse` | `memory.id` / `workspaceId` / `memoryType` / `summary` / `visibility` / `vectorReady` / embedding fields | 单条 Memory item 本体 |
| `decisionResponse` | `decision.reason` / `question` / `context` / `state` / `resolution*` | 单条 Decision 治理对象 |
| `approvalResponse` | `approval.reason` / `action` / `context` / `state` | 单条 Approval 治理对象 |

补充：

- `memoryItemResponse`
  - `vectorReady=true` 表示已具备向量检索条件
- `mcpServerResponse`
  - `command/args/env` 只在 stdio/command 类 transport 下有意义

#### List Resource Responses

| Response | 字段 | 语义方向 |
| --- | --- | --- |
| `workspacesResponse.workspaces` | workspace 列表 | 不含 snapshot 级 evidence 聚合 |
| `goalsResponse.goals` | Goal 列表 | workspace 当前目标集合 |
| `dodsResponse.dods` | DoD 列表 | workspace 当前验收基线集合 |
| `runsResponse.runs` | Run 列表 | 当前 run 本体列表，可用 `workspaceId` 过滤 |
| `soulsResponse.souls` | Soul 列表 | 可绑定执行人格集合 |
| `skillsResponse.skills` | Skill 列表 | 可按 `workspaceId` 过滤的 Skill 集 |
| `mcpServersResponse.servers` | MCP server 列表 | 注册的 server 集 |
| `mcpCapabilitiesResponse.capabilities` | capability 列表 | 可按 `serverId` / `kind` / `category` / `stage` 等过滤 |
| `memoryItemsResponse.memory` | Memory item 列表 | 需 `workspaceId` 作用域；不是 recall trace |

#### Detail Page Field Semantics

| Response | 字段 | 语义方向 |
| --- | --- | --- |
| `runStepsResponse.page` / `steps` | Step 明细分页 | `page` 是分页协议，`steps` 是当前页数据 |
| `runToolCallsResponse.page` / `toolCalls` | ToolCall 明细分页 | 外部动作明细，不替代 observability 摘要 |
| `runRuntimeCheckpointsResponse.page` / `runtimeCheckpoints` | Checkpoint 明细分页 | 恢复与运行态快照明细 |
| `runResultsResponse.page` / `results` | Result 明细分页 | accepted/rejected 结果明细 |
| `runPatchesResponse.page` / `patches` | Patch 明细分页 | 文件变更证据页 |
| `runValidationResultsResponse.page` / `validationResults` | Validation 明细分页 | build/test/lint/contract 验证页 |
| `runArtifactsResponse.page` / `artifacts` | Artifact 明细分页 | 交付物及附件页 |

#### Configuration / Operational Responses

| Response | 字段 | 语义方向 |
| --- | --- | --- |
| `statusResponse` | `service` / `status` | 最小服务健康状态；仅用于 health/ready，不表达业务运行态 |
| `configurationResponse` | `configuration.server` / `repository` / `runtime` / `llm` / `webSearch` | 当前服务配置投影；用于配置读写，不是 runtime snapshot |
| `goalDoDQualityCheckResponse` | `qualityCheck` | Goal/DoD 质量检查结果，不等于正式 Goal/DoD 持久化对象 |

`qualityCheck` 当前应优先被理解为：

- 当前 Goal / DoD 草案质量诊断
- 建议补充项 / 风险项 / 不完整项
- 不自动落库、不自动修改当前定义

当前高频补充字段方向：

- `qualityCheck.missingDoDItems`
  - 当前 DoD 尚未覆盖的缺口项
- `qualityCheck.coverageWorkItems`
  - 建议继续补齐的 coverage 工作项
- `qualityCheck.warnings`
  - 不阻塞保存但需要用户确认的风险项
- `qualityCheck.needsClarification`
  - 是否仍需要继续澄清后再正式落库

`configurationResponse` 当前应优先被理解为：

- `server`
  - addr / timeout / `maxConcurrentRuns` / `defaultDialogLanguage`
- `repository`
  - backend / dataDir
- `runtime`
  - repair / retry / shell timeout / LLM output defaults
- `llm`
  - provider 列表、默认 provider、capability/context profiles
- `webSearch`
  - provider/key/header 等配置投影


#### Registry List / Detail Semantics

| Response | 字段 | 语义方向 |
| --- | --- | --- |
| `mcpToolsResponse.tools` | tool capability 列表 | server 暴露的 tool catalog，不等于 selected capability |
| `mcpResourcesResponse.resources` | resource capability 列表 | 只读资源 catalog |
| `mcpPromptsResponse.prompts` | prompt capability 列表 | prompt catalog |
| `workspaceTemplatesResponse.workspaceTemplates` | template 列表 | 创建前默认配置集合 |
| `soulsResponse.souls` | Soul 列表 | 可选执行人格 catalog |
| `skillsResponse.skills` | Skill 列表 | 按 workspace 可见性过滤后的 Skill catalog |

补充：

- `mcpToolsResponse.tools`
  - 更偏 server 原始暴露面
- `mcpCapabilitiesResponse.capabilities`
  - 更偏 WorkOrch 已治理后的能力目录
- `skillsResponse.skills`
  - 可附带 `source` / `scope` / `workspaceId` / `status`
- `memoryItemsResponse.memory`
  - 可附带 `vectorReady` / `vectorDimension` / `vectorScore`

#### Activation / Setup Projection Semantics

| Response | 字段 | 语义方向 |
| --- | --- | --- |
| `workspaceSetupDraftSessionResponse.draftSession.workspace` | 创建前 workspace 草案 | 仅服务 setup draft，不是正式 workspace |
| `workspaceSetupDraftSessionResponse.draftSession.messages` | 对话历史 | 用于草案修订，不进入 run/ticket/approval |
| `workspaceSetupDraftSessionResponse.draftSession.draft` | Goal/DoD 结构化草案 | 需用户确认后才正式落库 |
| `workspaceExecutionActivationResponse.activation` | execution activation contract | 轻量返回本次 run 生效的 execution context / contract / bindings |

补充：

- `activation.snapshot`
  - 是 lightweight activation projection，不等于 full workspace snapshot

#### Coverage / Capability / Trace Fields

| Model | 字段 | 语义方向 |
| --- | --- | --- |
| `RunSnapshot` | `coverageWorkItems` / `coverageWorkItemContext` | 当前 run 的 coverage 工作项账本和上下文 |
| `RunSnapshot` / `RunObservability` | `selectedCapabilities` / `usedCapabilities` | capability 被选中和被实际使用的两层视图 |
| `RunSnapshot` / `WorkspaceSnapshot` | `artifactContinuations` | incremental artifact 生命周期投影；默认只返回 metadata，不内嵌 plan/chunk/assembled 正文 |
| `RunSnapshot` / `RunObservability` | `llmContextBudgets` / `llmUsageTraces` / `llmUsageSummary` | LLM 请求预算与 usage 分层视图 |

## Pagination Parameters

当前 detail / governance 列表端点的分页响应都带 `page` 字段，对应 appservice 中的：

- `StepPage`
- `ToolCallPage`
- `RuntimeCheckpointPage`
- `ResultPage`
- `PatchPage`
- `ValidationResultPage`
- `ArtifactPage`
- `DecisionPage`
- `ApprovalPage`

使用方向：

- 默认列表只返回一页 payload
- 调用方应显式处理 `page` 信息，而不是假设全量返回
- run snapshot / workspace snapshot 不是 detail page 的替代品

## Detail / Payload Mapping

典型映射关系：

- `run snapshot`
  - 面向当前 Run 的聚合详情
- `run observability`
  - 面向 timeline / evidence / status 的聚合详情
- `steps` / `tool-calls` / `runtime-checkpoints`
  - 面向逐类 detail 分页
- `artifacts/content`
  - 面向 artifact 正文或内容打开
- `runtime/payloads`
  - 面向 raw runtime payload ref 打开
- `events`
  - 面向事件列表查询
- `events/stream`
  - 面向实时 SSE 订阅

这意味着产品侧应优先：

- 先用 summary/list 做导航
- 再用 snapshot/observability 做聚合详情
- 最后用 detail/payload/content 做按需深挖

## Query / Cursor Notes

当前文档在产品设计层只保留 query/cursor 的稳定方向，不逐端点穷举所有 handler 参数：

- `events` 默认使用 `limit` / cursor
- `status` 支持 `waitState` / `waitTimeoutMillis`
- detail page 类端点应依赖 page/cursor 结构，而不是默认全量扫描

更细参数枚举若后续继续补写，只应作为字段级索引增强，不改变本文件现有语义边界。

## Request Body Families

当前写接口请求体大致分为：

- create / update workspace
  - `createWorkspaceRequest`
  - `deleteWorkspaceRequest`
- run / activation
  - `createRunRequest`
  - `activateWorkspaceExecutionRequest`
- governance
  - `resolveDecisionRequest`
  - `batchResolveDecisionsRequest`
  - `batchResolveApprovalsRequest`
- goal / DoD
  - `defineGoalRequest`
  - `defineDoDRequest`
  - `goalDoDQualityCheckRequest`
- setup draft session
  - `createWorkspaceSetupDraftSessionRequest`
  - `appendWorkspaceSetupDraftMessageRequest`
- registry / capability write
  - `createSoulRequest`
  - `createWorkspaceTemplateRequest`
  - `createSkillRequest`
  - `createMCPServerRequest`
  - `updateMCPServerRequest`
  - `createMCPCapabilityRequest`
  - `createMemoryItemRequest`

### Endpoint Family To Request Family

| Endpoint Family | 主要请求体 |
| --- | --- |
| workspace create/delete | `createWorkspaceRequest` / `deleteWorkspaceRequest` |
| run create | `createRunRequest` |
| execution activation | `activateWorkspaceExecutionRequest` |
| decision resolve | `resolveDecisionRequest` / `batchResolveDecisionsRequest` |
| approval resolve | `batchResolveApprovalsRequest` |
| goal / DoD write | `defineGoalRequest` / `defineDoDRequest` / `goalDoDQualityCheckRequest` |
| workspace setup draft session | `createWorkspaceSetupDraftSessionRequest` / `appendWorkspaceSetupDraftMessageRequest` |
| soul / template / skill / mcp / capability / memory create-update | registry / capability write family |

### Request Contract Direction

这些请求体的共同方向是：

- 结构化 command/request body 优先
- path / provider / capability / governance 等关键字段不应依赖自由文本猜测
- 资源 ID 是否允许客户端显式提供，取决于接口具体语义；不能一概假设可由客户端覆盖
- 业务写入必须经过 appservice command 路径，避免产品层直接改写内部状态

方向上：

- 业务写接口优先使用结构化 command/request body
- root/generated IDs 不应由客户端自由覆盖，除非接口明确允许
- activation / governance / registry 写入都通过 appservice command 路径进入业务层

### First-Cut Write Contract

| Request | 关键字段 | 当前契约方向 |
| --- | --- | --- |
| `createWorkspaceRequest` | `workspaceId?` / `title` / `deliverableDirectory` / `templateId` | 创建 Workspace，本体目录和 template 初始化入口 |
| `createRunRequest` | `runId` / `goalId` / `doDId` | 在已定义 Goal/DoD 上创建 Run |
| `activateWorkspaceExecutionRequest` | `goalId` / `doDId` / `llmProviderId` / `soulId` / `skillIds` / `mcpServerIds` / `capabilityIds` / `memoryIds` | 生成 execution activation contract |
| `resolveDecisionRequest` | `resolutionType` / `resolutionPayload` | 解决 Ticket |
| `batchResolveApprovalsRequest` | `approvalIds` / `action` | 批量批准/拒绝 Approval |
| `defineGoalRequest` | `text` | 创建或更新 Goal 文本 |
| `defineDoDRequest` | `items` | 创建或更新 DoD 项 |
| `goalDoDQualityCheckRequest` | `goalId` / `doDId` / `goalText` / `doDItems` | Goal/DoD 质量检查 |
| `createWorkspaceSetupDraftSessionRequest` | `templateId` / `title` / `description` / `deliverableDirectory` / `dialogLanguage` | 创建 setup draft session |
| `appendWorkspaceSetupDraftMessageRequest` | `content` / `draft` / workspace-template 上下文 | 追加 draft 消息并修订草案 |

### Goal / DoD Quality Check Field Direction

`goalDoDQualityCheckRequest` 当前稳定字段方向：

| 字段 | 当前方向 |
| --- | --- |
| `goalId` | 优先引用已存在 Goal |
| `doDId` | 优先引用已存在 DoD |
| `goalText` | 当不依赖持久化 Goal 时，直接提供待检查目标文本 |
| `doDItems` | 当不依赖持久化 DoD 时，直接提供待检查验收项 |

约束：

- `workspaceId` 来自 path，不在 body 中重复声明
- 该请求用于质量检查，不直接创建或更新 Goal / DoD
- `goalId` / `doDId` 与 `goalText` / `doDItems` 可以形成“引用已有对象”或“检查临时草案”两种输入路径

### Request Families Still To Detail

后续最值得继续拆的请求体包括：

- `goalDoDQualityCheckRequest`
- `createWorkspaceSetupDraftSessionRequest`
- `appendWorkspaceSetupDraftMessageRequest`
- `createSoulRequest`
- `createWorkspaceTemplateRequest`
- `createSkillRequest`
- `createMCPServerRequest`
- `updateMCPServerRequest`
- `createMCPCapabilityRequest`
- `createMemoryItemRequest`

优先顺序建议：

1. `activateWorkspaceExecutionRequest`
2. `resolveDecisionRequest` / `batchResolveApprovalsRequest`
3. `goalDoDQualityCheckRequest`
4. `createWorkspaceSetupDraftSessionRequest` / `appendWorkspaceSetupDraftMessageRequest`

### Setup Draft Session Field Direction

`workspace-setup-draft-sessions` 当前由独立 session 状态机承接，HTTP response 中的
`draftSession` 方向至少包括：

- `id`
- `templateId`
- `workspace`
- `messages`
- `draft`
- `state`
- `dialogLanguage`
- `createdAt`
- `updatedAt`

其中：

- `state`
  - 当前稳定值方向为 `clarifying` / `draft_ready`
- `workspace`
  - 只承接创建前草案，不等于正式 `workspace`
- `messages`
  - 记录草案对话历史，不进入 run/ticket/approval 领域对象
- `draft`
  - 承接 Goal / DoD 草案、clarifying questions、warnings、confidence

### Setup Draft Session Write Contract

| Request | 关键字段 | 当前契约方向 |
| --- | --- | --- |
| `createWorkspaceSetupDraftSessionRequest` | `templateId` / `title` / `description` / `deliverableDirectory` / `dialogLanguage` | 创建创建前草案 session；`title` 与 `description` 不能同时为空 |
| `appendWorkspaceSetupDraftMessageRequest` | `templateId` / `title` / `description` / `deliverableDirectory` / `content` / `draft` / `dialogLanguage` | 追加一轮 user message，并可同时修订当前草案上下文 |
| `POST .../regenerate` | 复用 `appendWorkspaceSetupDraftMessageRequest` | 在已有 session 上重新生成草案；请求体可为空，空体表示仅按当前上下文 regenerate |

当前行为补充：

- `templateId` 改变时，服务端会重新载入 template 默认 Goal / DoD 草案并与当前 draft 合并
- `draft` 存在时，会先并入当前 session draft，再触发生成
- `content` 为空时：
  - `messages` 接口返回 `400`
  - `regenerate` 接口允许为空体

### Draft Data Field Direction

`draft` 当前稳定字段方向至少包括：

- `goalText`
- `doDItems`
- `assumptions`
- `clarifyingQuestions`
- `warnings`
- `confidence`
- `needsClarification`
- `generationSummary`
- `templateContextUsed`
- `previousDraftApplied`

当前服务端方向：

- `doDItems` 会统一走 `doditems.Normalize`
- `needsClarification=true` 通常表示 Goal/DoD 仍不完整或仍有待追问项
- `previousDraftApplied` 表达本轮是否基于上一轮草案继续修订

### Registry Write Contract

以下写接口当前直接复用 appservice / kernel 的结构化 command/model：

| Request | 关键字段方向 | 当前约束 |
| --- | --- | --- |
| `createSoulRequest` | `id` / `name` / `description` / `businessDomains` / `soulMd` / `allowedSkillRefs` / `allowedMCPServerRefs` / `allowedCapabilityRefs` / `allowedCapabilityCategories` / `allowedToolKinds` | 以 Soul 作为结构化能力边界声明对象；`soulMd` 是提示词，不是授权源 |
| `createWorkspaceTemplateRequest` | `id` / `name` / `description` / `goalText` / `doDItems` / `defaultSoulId` / `soulBindings` / `contextHints` / `evidencePolicies` | template 创建期必须有 `id` / `name`，并校验引用的 Soul 是否存在 |
| `createSkillRequest` | `id` / `name` / `version` / `description` / `triggerRules` / `instruction` / `referenceRefs` / `requiredToolKinds` / `requiredMCPServerRefs` / `requiredCapabilityRefs` / `constraints` | Skill 只声明触发和依赖，不直接授权运行时外部动作 |
| `createMCPServerRequest` / `updateMCPServerRequest` | 复用 `MCPServer` 结构 | server 负责连接、transport、默认运行策略 |
| `createMCPCapabilityRequest` | 复用 `MCPCapability` 结构 | capability 负责单个 tool/resource/prompt 的结构化声明 |
| `createMemoryItemRequest` | `memoryId` / `runId` / `sourceType` / `sourceRef` / `memoryType` / `content` / `summary` / `tags` / `importance` / `confidence` / `visibility` / `embeddingProviderId` / `embeddingModel` / `embeddingText` / `vector` | `workspaceId` 来自 path，不由 body 自带 |

### Registry Write Behavior Notes

当前稳定行为方向：

- `PUT /souls/:soulId`
  - 若 body 中 `id` 非空且与 path 不一致，按 invalid argument 失败
- `POST/PUT /workspace-templates`
  - 会归一化 `doDItems`、`soulBindings`
  - `defaultSoulId` 和 `soulBindings` 中引用的 Soul 必须存在
- `POST /workspaces/:workspaceId/memory`
  - `workspaceId` 由 path 确定；body 只描述 Memory item 本体
- `DELETE /memory/:memoryId`
  - 返回 `200` + `{ "deleted": true }`，不是 `204`

### Registry Field-Level Constraints

| Request | 字段级约束方向 |
| --- | --- |
| `createSoulRequest` | `id` / `name` 必须稳定；`soulMd` 应非空；allowlist 字段只表达能力边界，不表达运行结果 |
| `createWorkspaceTemplateRequest` | `id` / `name` 必填；`doDItems` 会归一化；`defaultSoulId` / `soulBindings.*` 必须引用已存在 Soul |
| `createSkillRequest` | `id` / `name` / `version` / `instruction` 应形成稳定 Skill 身份；`required*` 字段只声明依赖，不授予权限 |
| `createMCPServerRequest` | `id` / `name` / `transport` 是 server 身份主字段；`endpoint` / allowlists / timeout / session policy 形成连接基线 |
| `createMCPCapabilityRequest` | `id` / `serverId` / `kind` / `name` / `displayName` 形成 capability 身份；`approvalPolicy` / `riskLevel` 形成治理边界 |
| `createMemoryItemRequest` | `sourceType` / `memoryType` / `summary` 是最稳定主字段；`content` 可为空摘要化；embedding/vector 字段只表达索引状态 |

### Registry Fine-Grained Notes

- `createSoulRequest`
  - `businessDomains` 只表达归属和检索标签，不形成运行态授权
  - `allowed*` 字段可为空；为空不等于无限授权，仍受其他层约束
- `createWorkspaceTemplateRequest`
  - `goalText` 可为空，但空模板只适合最小初始化场景
  - `contextHints` / `evidencePolicies` 属于 template 初始化提示，不应内嵌运行结果
- `createSkillRequest`
  - `triggerRules` 面向发现/选择，不应被误解为自动执行命令
  - `referenceRefs` 属于外部引用，不直接进入 accepted evidence
  - `source` / `scope` / `workspaceId` / `managedBy` / `contentHash` 等字段优先表达来源与管理状态，而不是执行权限
- `createMCPServerRequest`
  - `transport` 决定 server 连接模式，是最关键的行为字段
  - `command` / `args` / `env` 只在非 HTTP transport 下作为连接配置，不构成业务证据
  - `allowedTools/resources/prompts` 只表达 server 允许面，不代替 capability catalog
- `createMCPCapabilityRequest`
  - `schema` 与 `uri` 二选一或按 kind 使用，不应同时被产品层混成自由输入面
  - `stageHints` / `category` / `tags` 优先服务 catalog 和 selection
- `createMemoryItemRequest`
  - `visibility` 决定 workspace/run 可见范围
  - `embedding*` / `vector*` / `vectorReady` / `vectorScore` 字段只服务 recall/index，不构成业务结果证据
- `createMCPCapabilityRequest`
  - `enabled=false` 表示 catalog 可见但不可进入候选集
  - `approvalPolicy` 若为空，通常回退到 server/profile/policy 默认值，而不是表示“永不审批”
- `createWorkspaceTemplateRequest`
  - `evidencePolicies` / `contextHints` 是创建期 contract 种子，不应在产品层解释为当前 run 的实时状态

## Raw / SSE Usage Examples

### `events/stream`

浏览器 / 控制台当前使用方向：

```js
const params = new URLSearchParams({ workspaceId, runId });
const source = new EventSource(`/api/v1/workorch/events/stream?${params.toString()}`);
source.onmessage = (event) => {
  const envelope = JSON.parse(event.data);
  console.log(envelope.topic, envelope.runId, envelope.workspaceId);
};
```

当前行为约束：

- server 发送 `text/event-stream`
- 初始会先发送 `: connected`
- 当前事件名固定为 `workorch`
- `workspaceId` / `runId` query 只做服务端过滤，不改变 event payload 结构

关闭/重连方向：

- 当客户端主动关闭 `EventSource` 或 HTTP 连接断开时，服务端根据 `r.Context().Done()` 结束订阅
- 当前服务端不自定义 retry interval、event id 或 last-event-id 协议
- 重连由浏览器/EventSource 默认机制负责；重新连接后应重新带上相同的 `workspaceId` / `runId` 过滤参数
- `events/stream` 不保证补发断线期间历史事件；断线补偿应回退到 `GET /events`

错误方向：

- 未配置 `eventHub` 时，返回 not-found 方向错误
- 当前 writer 不支持 flush 时，返回 `500` 和 `"streaming unsupported"`
- 连接建立成功后若后续无匹配事件，连接可保持空闲，不表示错误

典型错误示例方向：

- `GET /events/stream` with missing `eventHub`
  - `404` / not-found 方向
- `GET /runtime/payloads?ref=file:///tmp/secret`
  - `400` / invalid argument
- `GET /runtime/payloads?ref=workorch://workspaces/ws/runs/run/llm-calls/step/secret.txt`
  - `400` / invalid argument
- `GET /artifacts/content?uri=file:///tmp/secret`
  - `400` / invalid argument
- `GET /artifacts/content?uri=workorch://workspaces/ws/artifacts/run/secret.txt`
  - `404` / 未注册 artifact metadata

典型权限/异常示例方向：

- `GET /events/stream?workspaceId=...`
  - 连接成功但无匹配事件：保持空闲
- `GET /events/stream` on unsupported writer
  - `500` + `{"error":"streaming unsupported"}`
- `GET /runtime/payloads?ref=<unsafe>`
  - `400` invalid argument
- `GET /artifacts/content?uri=<unregistered>`
  - `404` not found

### `runtime/payloads`

前端 / 脚本当前使用方向：

```text
GET /api/v1/workorch/runtime/payloads?ref=<runtime-ref>
```

典型行为：

- 返回原始内容，不走 JSON envelope
- `Content-Type` 来自 payload 自身或回退 `application/octet-stream`
- 响应头带 `X-Workorch-Raw-Ref`
- 只有显式 raw/detail 打开才允许看到 artifact continuation 相关正文；默认 snapshot/observability 只暴露 metadata

### `artifacts/content`

前端 / 脚本当前使用方向：

```text
GET /api/v1/workorch/artifacts/content?uri=<artifact-uri>
```

典型行为：

- 返回 artifact 正文，不走 JSON envelope
- `Content-Type` 来自 artifact 自身或回退 `application/octet-stream`
- `Content-Disposition` 给出建议文件名
- 响应头带 `X-Workorch-Artifact-URI`

### Console Client Usage Direction

当前 console/client 层已经按以下方式消费这些接口：

- `artifactContentURL(uri)`
  - 生成 artifact 下载链接
- `runtimePayloadURL(ref)`
  - 生成 raw payload 打开链接
- `new EventSource(/events/stream?... )`
  - 建立 SSE 订阅

因此产品侧默认方向应为：

- snapshot / observability 承担主阅读面
- raw payload / artifact content 只在用户显式展开时打开
- events/stream 只承担实时更新，不承担默认历史回放
- 断线后的历史补偿应显式走 `GET /events`

## File-Backend Ref / URI Contract

### Artifact URI

`artifacts/content` 当前面向受控 `workorch://` URI，而不是任意文件路径。

稳定方向：

- URI 必须能解析到 `workspace -> artifacts -> relative path`
- 不接受 `file://`、绝对路径、`..` 逃逸路径或未知 scheme
- 读取时会先定位 artifact metadata，再按 artifact 根目录解析相对路径
- `run snapshot` 已缓存 artifact metadata 时，可优先复用缓存 metadata，而不是重新扫全量 artifact 列表

### Runtime Payload Ref

`runtime/payloads` 当前面向受控 runtime ref，而不是任意文件名拼接。

稳定方向：

- ref 只能指向允许的 runtime raw content collection
- file backend 下会经过白名单 collection + safe path 校验
- 不允许任意扩展名或逃逸目录访问
- memory backend / 不存在 collection 时，按 not-found 或 invalid-argument 方向处理

当前 file backend 允许的 collection/file 组合方向：

| Collection | 允许文件 |
| --- | --- |
| `llm-calls` | `request.json` / `response.json` / `response.sse` / `usage.json` / `repair-response.json` |
| `steps` | `input.json` / `output.json` |
| `tool-calls` | `arguments.json` / `output.json` |
| `runtime-checkpoints` | `data.json` |
| `results` | `dod-assessment.json` / `summary-detail.json` |

约束：

- `collection` 与 `fileName` 必须匹配
- 合法文件名不代表资源一定存在；不存在仍按 `404` 方向处理
- `response.sse` 的 `Content-Type` 应为 `text/event-stream; charset=utf-8`
- 其余受控 JSON 文件应返回 `application/json; charset=utf-8`

### Artifact URI Naming Direction

artifact URI 当前稳定方向为：

```text
workorch://workspaces/<workspace-id>/artifacts/<run-id>/<relative-path>
```

约束：

- `<workspace-id>` / `<run-id>` 必须与 artifact metadata 归属一致
- `<relative-path>` 不能为空，且每个 path segment 不能是空串、`.`、`..`
- 已注册 artifact metadata 与磁盘文件路径必须同时成立，才能返回正文

### Common Safety Guarantees

- `artifacts/content` 与 `runtime/payloads` 都必须拒绝：
  - `file://`
  - 绝对路径
  - `..`
  - workspace/run 边界之外的路径
- 两者都只承担“显式打开详情”，不应成为默认 summary 读取路径

## Failure Response Direction

当前统一错误 envelope 为：

- `errorResponse{ error }`

常见状态方向：

- `400 Bad Request`
  - 请求体不合法
  - query 参数不合法
  - 缺少必填字段
- `404 Not Found`
  - 资源不存在
- `409 Conflict`
  - 业务状态冲突
  - run/step/terminal state conflict
  - duplicate / invalid transition
- `408 Request Timeout`
  - wait / timeout 类请求超时
- `500 Internal Server Error`
  - 未归类内部失败

这意味着产品侧应优先把 `400/404/409/408/500` 视为不同性质的失败，而不是统一显示成 generic request error。

### Typical Failure Families

- validation failure
  - body / query / required field / enum / path parameter 不合法
- state conflict
  - run terminal / duplicate / invalid transition / blocker mismatch
- not found
  - workspace / run / ticket / approval / artifact / payload 不存在
- wait timeout
  - `run status` 服务端等待未命中目标状态
- internal error
  - appservice / repository / runtime 内部错误

- setup draft session failure
  - body/context invalid
  - template missing
  - session state conflict

### Resource-Level Failure Direction

| Endpoint Family | 常见失败方向 |
| --- | --- |
| workspace / goal / DoD write | `400` body/field error, `404` parent resource missing, `409` state conflict |
| run / activation / start / resume | `400` request invalid, `404` run/workspace missing, `409` terminal or transition conflict |
| decision / approval resolve | `404` governance item missing, `409` already resolved/rejected or invalid state |
| registry / capability write | `400` schema/body invalid, `404` target missing, `409` duplicate/constraint conflict |
| status wait | `408` timeout not matched, `404` run missing |
| setup draft session | `400` body/context invalid, `404` template missing, `409` session state conflict |

### First-Cut Read Failure Direction

| Endpoint Family | 常见失败方向 |
| --- | --- |
| singleton read | `404` resource missing |
| snapshot / observability | `404` run/workspace missing, `500` projection/read error |
| detail page | `404` run missing, `400` invalid pagination query |
| raw payload / artifact content | `404` ref/uri missing, `400` invalid ref/uri |
| event query | `400` invalid filter/pagination query |

### Common Failure Envelope Notes

- 错误正文当前统一通过 `errorResponse{error}` 返回
- `409` 在本服务中很关键，常表示业务状态冲突，而不仅是 HTTP 级资源冲突
- `408` 目前主要与 `run status` 的 wait 模式相关

## Pagination And Detail Direction

run 级 detail 已拆为专门端点：

- `steps`
- `tool-calls`
- `runtime-checkpoints`
- `results`
- `patches`
- `validation-results`
- `artifacts`

这类端点的职责是避免把所有运行态 payload 都塞进 `run snapshot` 或
`workspace snapshot`。

当前 appservice 的分页 page model 已明确包括：

- `StepPage`
- `ToolCallPage`
- `RuntimeCheckpointPage`
- `ResultPage`
- `PatchPage`
- `ValidationResultPage`
- `ArtifactPage`
- `DecisionPage`
- `ApprovalPage`

共同方向：

- `limit`
- `cursorAfterId`
- `nextCursorAfterId`
- `hasMore`

调用方应优先依赖这些 page 字段，而不是假设 detail endpoint 默认返回全量数据。

## Common Query Parameters

### Run / Workspace Scope Filters

当前多类列表端点已支持：

- `workspaceId`
- `runId`

典型场景：

- `GET /runs?workspaceId=...`
- `GET /decisions?workspaceId=...&runId=...`
- `GET /approvals?workspaceId=...&runId=...`
- `GET /skills?workspaceId=...`
- `GET /events?workspaceId=...&runId=...`

### Pagination Parameters

detail / governance / event 端点当前普遍使用：

- `limit`
- `cursorAfterId`

其中：

- `events` 有显式默认 limit 和最大 limit 保护
- run detail / decision / approval page 方向也基于 `limit` + `cursorAfterId`

当前行为补充：

- `events` 若未显式给出 `limit`，会回退到服务端默认值
- `events` 若给出非正数或超大值，会被服务端归一化到允许范围
- run detail / decision / approval page 端点当前不会在 HTTP 层强制 default limit，是否分页由 appservice page model 决定

### Status Wait Parameters

`GET /workspaces/:workspaceId/runs/:runId/status` 额外支持：

- `waitState`
- `waitTimeoutMillis`

用途：

- 服务端等待 run 进入目标状态
- 减少客户端轮询压力

当前行为补充：

- `waitState` 为空时立即返回当前状态
- `waitTimeoutMillis` 只接受毫秒值；超时或等待被取消时，仍返回 `status` 结构，另以
  `waitMatched` / `waitElapsedMs` 表达等待结果

### Event Query Parameters

`GET /events` 当前已支持：

- `workspaceId`
- `runId`
- `topic`
- `subjectType`
- `subjectId`
- `cursorAfterId`
- `occurredAfter`
- `occurredBefore`
- `limit`

当前行为方向：

- 该接口是“事件列表查询”，不是 SSE 订阅
- 适合分页、过滤、回放入口，不适合承担默认实时视图

### Detail Page Query Parameters

以下 detail page 端点当前统一走：

- `GET /workspaces/:workspaceId/runs/:runId/steps`
- `GET /workspaces/:workspaceId/runs/:runId/tool-calls`
- `GET /workspaces/:workspaceId/runs/:runId/runtime-checkpoints`
- `GET /workspaces/:workspaceId/runs/:runId/results`
- `GET /workspaces/:workspaceId/runs/:runId/patches`
- `GET /workspaces/:workspaceId/runs/:runId/validation-results`
- `GET /workspaces/:workspaceId/runs/:runId/artifacts`
- `GET /decisions`
- `GET /approvals`

HTTP 层统一支持：

- `cursorAfterId`
- `limit`

其中 `GET /decisions`、`GET /approvals` 额外支持：

- `workspaceId`
- `runId`

### Memory / Recall Query Parameters

`GET /memory` 当前要求：

- 必填 `workspaceId`
- 可选 `q`
- 可选多值 `tag`
- 可选多值 `type`
- 可选 `limit`

若缺少 `workspaceId`，HTTP 层直接返回 `400`.

`GET /workspaces/:workspaceId/memory-recalls` 与
`GET /workspaces/:workspaceId/runs/:runId/memory-recalls` 当前支持：

- `stepId`
- `stage`
- `source`
- `status`
- `limit`

并由路径作用域强制覆盖 `workspaceId` 或 `runId`。

### Capability Query Parameters

`GET /mcp/capabilities` 当前支持：

- `serverId`
- `kind`
- `category`
- `stage`
- `riskLevel`
- `enabled`
- `q`

### Payload / Content Parameters

- `GET /runtime/payloads`
  - 使用 `ref`
- `GET /artifacts/content`
  - 使用 `uri`
- `GET /events`
  - 支持 topic / workspace / run / cursor / limit 等事件查询参数

这类接口应视为 raw/detail 打开路径，而不是默认业务 summary 入口。

当前 raw/content 响应补充：

- `GET /artifacts/content` 返回二进制或文本内容，`Content-Type` 由 artifact 决定，
  并附带 `X-Workorch-Artifact-URI`
- `GET /runtime/payloads` 返回 raw payload，`Content-Type` 由 payload 决定，
  并附带 `X-Workorch-Raw-Ref`
- 两者都通过 `Content-Disposition` 暴露建议文件名，不走 JSON envelope

### Parameter Direction Summary

| Parameter | 主要用途 |
| --- | --- |
| `workspaceId` | 过滤 workspace 作用域资源 |
| `runId` | 过滤 run 作用域资源 |
| `limit` | 控制 page/event 列表大小 |
| `cursorAfterId` | 翻页游标 |
| `waitState` | `run status` 服务端等待目标状态 |
| `waitTimeoutMillis` | `run status` 等待超时 |
| `ref` | `runtime/payloads` raw payload 打开 |
| `uri` | `artifacts/content` artifact 正文打开 |

### Endpoint Parameter Direction

| Endpoint Family | 关键参数方向 |
| --- | --- |
| `GET /runs` | `workspaceId` |
| `GET /decisions` / `GET /approvals` | `workspaceId`、`runId`、page params |
| `GET /steps` / `tool-calls` / `runtime-checkpoints` / `results` / `patches` / `validation-results` / `artifacts` | `limit`、`cursorAfterId` |
| `GET /workspaces/:workspaceId/runs/:runId/status` | `waitState`、`waitTimeoutMillis` |
| `GET /events` | `workspaceId`、`runId`、`topic`、`subjectType`、`subjectId`、`occurredAfter`、`occurredBefore`、`limit`、`cursorAfterId` |
| `GET /memory` | `workspaceId`、`q`、`tag`、`type`、`limit` |
| `GET /workspaces/:workspaceId/memory-recalls` / `GET /workspaces/:workspaceId/runs/:runId/memory-recalls` | `stepId`、`stage`、`source`、`status`、`limit` |
| `GET /mcp/capabilities` | `serverId`、`kind`、`category`、`stage`、`riskLevel`、`enabled`、`q` |
| `GET /runtime/payloads` | `ref` |
| `GET /artifacts/content` | `uri` |

## Error Contract

当前 HTTP 层错误输出统一为：

- JSON 结构：`{"error": "<message>"}`

主要状态分类如下：

| 场景 | 状态码 | 当前方向 |
| --- | --- | --- |
| JSON 解码失败、unknown field、显式参数缺失 | `400` | `decodeJSON` / `decodeOptionalJSON` 失败或 handler 主动校验失败 |
| 业务参数非法 | `400` | `common.ErrInvalidArgument` |
| 阻塞或冲突 | `409` | `common.ErrBlocked` / `common.ErrConflict` |
| 资源不存在 | `404` | `common.ErrNotFound` |
| 运行队列等待被取消 | `408` | `start` / `resume` / `execution-activations` 的 run slot 等待失败 |
| 其余未分类服务错误 | `500` | 兜底内部错误 |

当前具名 HTTP 层特例包括：

- `POST /workspaces`
  - 若请求体显式提供 `workspaceId`，直接 `400`
- `POST /workspaces/:workspaceId/runs`
  - 若缺少 `goalId` 或 `doDId`，直接 `400`
- `DELETE /workspaces/:workspaceId`
  - 若 `purge` query 不是合法布尔值，直接 `400`
- `GET /memory`
  - 若缺少 `workspaceId`，直接 `400`
- `GET /events/stream`
  - 若服务未配置 `eventHub`，按 not-found 方向返回
  - 若当前 writer 不支持 flush，返回 `500` 和 `"streaming unsupported"`

## Write Contract Refinement

### Create / Update / Delete Status Direction

当前写接口的状态码方向：

| 写接口类型 | 状态码 |
| --- | --- |
| 创建资源 | `201` |
| 普通更新 | `200` |
| 删除资源 | `204` 或 `{ "deleted": true }` |

当前例外：

- `POST /workspaces/:workspaceId/execution-activations` 返回 `200`
  - 因为它返回 activation contract，而不是单纯资源创建确认
- `POST /workspaces/:workspaceId/runs/:runId/start` / `POST /workspaces/:workspaceId/runs/:runId/resume` 返回 `200`
  - 返回最新 `run` 视图
- `DELETE /memory/:memoryId` 返回 `200` + `{ "deleted": true }`
  - 不走 `204`

### First-Cut Write Guards

当前 HTTP 层已经明确承担的写接口门禁包括：

- `createWorkspaceRequest`
  - `workspaceId` 不能由客户端指定
- `createRunRequest`
  - `goalId` 和 `doDId` 必须引用已定义对象
- `activateWorkspaceExecutionRequest`
  - `priority` 既可来自请求体，也可被 query `priority` 覆盖
- `resolveDecisionRequest`
  - 通过 `resolutionType` / `resolutionPayload` 进入结构化治理路径
- `batchResolveApprovalsRequest`
  - 通过 `approvalIds` / `action` 批量进入审批决议

## Closure Status

本文件当前已完成 active 设计层所需的主收口：

- 资源分组与正式路径前缀已对齐
- response family / projection / detail page / raw-content 边界已对齐
- query / pagination / error contract / write contract 主体已对齐
- setup-draft、registry write、raw/SSE、安全约束已形成稳定说明

后续若继续补细，应只视为“实现枚举补全”而不是“信息模型未闭环”。

## Optional Further Detailing

后续若需要继续穷举，可只补：

- 个别低频 `workspaceSnapshotResponse` / `runObservabilityResponse` 附属字段
- 个别 registry 请求体的实现枚举字段
- 个别 raw/SSE 错误样例

## Source Of Truth

当前接口分组以实际服务路由注册为准：

- `internal/modules/application/workorchd/service/server/http.go`

## Migration Note

旧综合状态文档中的接口清单已经落后于当前实际路由面。
本文件在本轮迁移中已改为基于当前 `http.go` 路由注册整理，而不是继续沿用旧状态文档的过期枚举。
