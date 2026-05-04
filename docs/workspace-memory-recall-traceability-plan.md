# Workspace Memory Recall Traceability Plan

## 背景

当前目标是让 workspace 从 Goal 到 DoD 的执行过程中，能够完整追溯每个阶段 recall 到了哪些 Memory、为什么 recall、何时进入 Run/Step、最终如何在 API、Web/TUI 和 e2e 中验收。

本方案避免采用临时展示方案。不能只在 Web 侧解析 `executionContext` JSON，也不能只把 step input 展开给用户看。Memory recall 需要成为正式过程证据，具备持久化、事件通知、读模型投影和验收脚本覆盖。

## 当前核对结论

### 已存在能力

- Memory 作为业务知识模型已经存在，核心模型是 `MemoryItem`。
- workspace execution activation 会执行 memory 选择：
  - 入口：`internal/modules/application/workorchd/biz/appservice/workspace_execution_activation_service.go`
  - 关键函数：`selectedMemoryItems()`、`semanticMemoryItems()`、`memoryContexts()`
- recall 结果当前会进入 `Run.ExecutionContext`：
  - `memoryRetrieval`
  - `memoryIds`
  - `memories`
- LLM step 输入会继承 selected context：
  - 入口：`internal/modules/kernel/orchestrator/biz/execution_context_package.go`
  - `runExecutionContextPackage()` 生成 `executionContextPackage`
  - `selectedContextPackage()` 将 `memories` 和 `memoryRetrieval` 放入 `selectedContext`
- Web 已能读取 `RunSnapshot`、`RunObservability`、`ContextBundle` 和 Memory 列表。
- 当前 e2e 已能验证第二轮 Run 召回 Memory，并在 snapshot/observability 的部分上下文中可见。

### 主要缺口

- 缺少一等过程证据模型。`MemoryItem` 是知识内容，不是 recall 过程记录；`Run.ExecutionContext` 是启动上下文，不适合作为完整审计日志。
- 缺少独立持久化仓储。当前没有 `MemoryRecallTraceRepository` 或等价结构，不能按 workspace/run/step/stage 查询 recall 证据。
- 缺少事件流。当前 EventStream 可持久化 Workorch 事件，但没有 memory recall recorded 事件主题。
- 缺少阶段投影。无法直接回答“planning/execution/decision/approval/validation/artifact 阶段分别 recall 了哪些 memory”。
- `RunSnapshot`、`RunObservability`、`ContextBundle`、`ExecutionSection` 没有明确字段承载 memory recall trace。
- Web 当前更接近 raw JSON 展示，无法稳定表达 recall 来源、阶段、策略、命中分数和 evidence refs。
- TUI 当前保持 result-first 是合理的，但缺少显式命令查看当前 run 的 recall 摘要。

## 目标模型

新增 `MemoryRecallTrace`，作为过程证据，不替代 `MemoryItem`。

建议字段：

- `ID`
- `WorkspaceID`
- `RunID`
- `StepID`
- `GraphNodeID`
- `Stage`
- `Source`: `activation`、`step_input`、`recovery`
- `Policy`
- `Status`
- `ProviderID`
- `Operation`
- `QueryTextPreview`
- `QueryTextSha256`
- `MinScore`
- `Limit`
- `EmbeddingModel`
- `VectorDimension`
- `RetrievedMemoryIDs`
- `RetrievedMemoryHits`
- `Reason`
- `OccurredAt`
- `EvidenceRef`

`RetrievedMemoryHits` 至少包含：

- `ID`
- `Summary`
- `VectorScore`
- `VectorDimension`

## 写入链路

### Activation recall trace

在 workspace execution activation 完成 memory 选择后，不只把结果写入 `executionContext`，还要生成 `source=activation` 的 trace。

注意当前 `buildWorkspaceExecutionContextWithDefinitions()` 发生在 Run 创建前，因此 trace 写入需要等到 RunID 已生成后完成。建议做法：

- 保持 activation context 组装职责不扩张。
- 在创建 run 后，从 executionContext 中提取 `memoryRetrieval`、`memoryIds`、`memories`，生成 activation trace。
- 使用确定性 ID，避免 retry/resume 产生重复记录。

### Step input recall trace

在 LLM step 输入构建完成后，按 step/stage 记录 `source=step_input` 的 trace。

要求：

- stage 使用 step input 中已有 soul stage。
- trace 指向具体 `RunID`、`StepID`。
- 如果 selected context 复用上一轮 stable package，也必须可追溯为当前 step 的输入上下文。
- 使用确定性 ID 或 upsert/ignore-conflict，避免重试造成重复记录。

### Recovery recall trace

当 recovery 或 compressed context 重新装配 selected context 时，记录 `source=recovery`。

如果 recovery 当前只是复用已有 selected context，也需要能通过 trace 说明该阶段没有重新检索，而是继承了已有 recall evidence。

## 持久化与事件

### Repository

新增 repository interface：

- `Create` 或 `Upsert`
- `Get`
- `List`
- `ListByWorkspace`
- `ListByRun`
- `ListByStep`
- `ListByStage`

需要同步实现：

- memory backend
- file backend
- workspace scoped file backend

file backend 建议落地到 workspace scoped 数据目录，例如：

- `.workorch-data/workspaces/{workspaceId}/memory_recall_trace.json`

### EventHub/EventStream

新增事件主题，例如：

- `/workorch/memory/recall/recorded`

事件 payload 包含 trace 摘要：

- trace id
- workspace id
- run id
- step id
- stage
- source
- policy
- status
- retrieved memory ids
- reason

本地 CLI/TUI 仍然通过 EventHub 获取刷新信号，不能引入定时轮询或本地 HTTP 旁路。

## 读模型与 API

### RunSnapshot

新增：

- `MemoryRecallTraces []MemoryRecallTrace`

用于完整展示当前 run 的 recall 证据。

### RunObservability

新增：

- `MemoryRecallTraces []MemoryRecallTrace`
- timeline 中新增 `memory_recall` 类型事件

timeline 条目要能展示：

- source
- stage
- status
- retrieved count
- evidence ref

### ContextBundle

新增 `memory_recall` section：

- `kind=memory_recall`
- `evidenceRefs` 指向 trace id 和 memory ids
- `data` 包含按 source/stage 聚合后的 trace 列表

### ExecutionSection

新增：

- `memoryRecallRefs`
- `memoryIds`

用于把每个执行段和 recall 证据关联起来，而不是让前端反查 raw step input。

### HTTP API

建议新增只读 API：

- `GET /api/v1/workorch/runs/{runId}/memory-recalls`
- `GET /api/v1/workorch/workspaces/{workspaceId}/memory-recalls`

这些 API 是 Web 排查入口。TUI 本地模式不通过 HTTP 调用这些接口，而是复用后端 service/eventhub 能力。

## Web/TUI 展示

### Web

Web 应展示正式 `MemoryRecallTrace`，而不是只展示 raw `executionContext`。

需要补齐：

- Run 详情增加 Memory Recall 卡片或 tab。
- 按 source/stage 分组展示 recall 记录。
- 展示 recall 策略、状态、命中 memory、score、reason。
- 将现有“Memory 输出”改名为“Memory 写入/引用”，避免与 recall 混淆。
- step 展开区域可以展示关联 trace refs，但不应该依赖 raw input 作为唯一数据源。

### TUI

TUI 默认仍保持结果优先，不展示详细 recall trace，避免干扰主交互。

需要补齐显式查看能力：

- `/memory` 或 `/context memory` 显示当前 run 的 compact recall 摘要。
- 长任务进度栏可显示 `Memory recall: selected N` 或 `Memory recall: degraded`。
- 当 EventHub 收到 memory recall recorded 事件时刷新相关视图。

## 验收清单

### 单元测试

- repository contract：memory/file/workspace scoped file 后端都能创建、读取、按 workspace/run/step/stage 查询 trace。
- activation trace：语义检索命中时生成 trace，包含 provider、query hash、命中 memory ids。
- activation degrade：embedding provider 不可用时生成 degraded trace，不能静默 fallback 成 selected。
- step input trace：LLM step 输入构建后生成当前 step/stage trace。
- retry/resume 幂等：相同 run/step/stage 不重复产生 trace。

### API/read model 测试

- `RunSnapshot` 返回 `memoryRecallTraces`。
- `RunObservability` 返回 `memoryRecallTraces`，timeline 包含 `memory_recall`。
- `ContextBundle` 包含 `memory_recall` section。
- `ExecutionSection` 包含 `memoryRecallRefs` 和 `memoryIds`。
- HTTP memory-recalls API 支持 run/workspace 查询。

### Web 验收

- Run 详情能直接看到每个阶段 recall 到的 memory。
- recall view 不依赖 raw executionContext JSON。
- Memory 写入与 Memory Recall 分开展示。

### TUI 验收

- 默认界面仍只显示最终结果和必要进度。
- `/memory` 或等价命令能显示当前 run recall 摘要。
- memory recall 事件到达后 TUI 通过 EventHub 异步刷新。

### E2E 验收

- 第一轮 Run 写入 memory。
- 第二轮 Run 通过语义 recall 命中第一轮 memory。
- 验证 snapshot、observability、context-bundle、memory-recalls API 都能看到同一个 trace id 和 memory id。
- 验证 Web 可以显示该 trace。
- 验证 TUI 能通过命令查看 compact recall 摘要。
- 重启 file backend 后 trace 仍可查询。

## 收口顺序

1. 新增 `MemoryRecallTrace` 模型、ID、repository interface 和 memory/file 实现。
2. 接入 bootstrap repository set 和 runtime exports。
3. 在 activation run 创建后记录 `source=activation` trace。
4. 在 LLM step input 构建后记录 `source=step_input` trace。
5. 增加 EventHub topic 并确保 EventStream 持久化。
6. 扩展 read model：snapshot、observability、context-bundle、execution-section。
7. 增加 HTTP 只读 API。
8. Web 增加正式 recall view。
9. TUI 增加显式 compact recall 命令。
10. 补齐单元、API、Web/TUI 和 deployed e2e 验收。

## 当前状态

- requirement status: confirmed
- implementation status: not started
- temporary workaround accepted: no
- tracking owner: workorch memory recall traceability
