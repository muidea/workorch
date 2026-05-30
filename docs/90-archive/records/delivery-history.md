# WorkOrch Delivery History

Status: active

Type: archive

Last Updated: 2026-05-30

## Purpose

本文件承接历史综合状态与阶段记录中拆出的历史交付范围、阶段性完成记录和已完成能力摘要。

它只用于追溯，不再作为当前设计、当前 API 或当前验收的权威入口。

当前稳定入口应优先查看：

- `../../01-overview/current-state.md`
- `../../50-product-interface/overview.md`
- `../../60-verification/overview.md`
- `../../70-roadmap/active/overview.md`

## Out Of Current Delivery Scope

以下内容不是当前已完成 MVP / framework alignment / P1 交付范围内的活跃待办项：

- distributed scheduling
- external worker fleets
- multi-tenant execution quota

以下后续增强已完成，不再列为待办：

- file-backed repository adapter as the first non-memory persistence path
- priority-aware local run scheduling behind `[server].maxConcurrentRuns`
- MCP resources, prompts, streaming evidence, and long-lived stdio session reuse
- LLM provider streaming and non-chat provider APIs
- vector Memory search and real embedding-backed recall verification
- Decision batch resolution and Approval batch resolution

## Completed P1 Scope

P1 已完成，完成项包括：

- `P1-1 LLM Provider Streaming And Non-Chat APIs`
- `P1-2 Vector Memory Search`
- `P1-3 Global Long-Lived Runtime Module Registration`

归档主文档：

- `../completed/p1-development-record.md`

## Result And Event Stream Delivery

下一阶段中已完成的 Result / Event Stream 交付包括：

- Result / Artifact 正式模型、仓储、file/memory 持久化与 runtime/bootstrap 导出
- LLM-driven Run 在满足正式 DoD assessment 后自动生成 accepted Result
- Run Snapshot 与 Workspace Snapshot 暴露 Result / Artifact
- Web Console 优先消费正式 Result / Artifact
- EventRecord 正式模型、memory/file 仓储与 repository contract
- `kernel/eventstream` 模块订阅正式 WorkOrch EventHub 事件并持久化
- Run / Workspace Snapshot 与 Run Observability 接入持久化事件记录
- `/api/v1/workorch/events` 支持按 workspace、run、topic、subject、cursor、limit 查询
- file-backed 服务重启后的事件流 replay 验证完成

边界：

- Result / Artifact 用于表达正式业务成果，不替代 ToolCall、Patch、ValidationResult、RuntimeCheckpoint、Memory 或 Event evidence
- Persistent Event Stream 用于审计与回放，不成为 Run / Workspace 状态拥有者

归档主文档：

- `../completed/result-event-development-plan.md`

## Runtime Expansion Scope

以下 runtime expansion 能力已完成：

- `R1 External Runtime And Tool Adapter Contract`
- `R2 Controlled Shell Sandbox`
- `R3 Generalized Diff Patch Apply Engine`
- `R4 Block Execution Graph`
- `R5 Observability Pipeline`

归档主文档：

- `../completed/runtime-expansion-plan.md`

## Advanced Runtime And Scheduling Scope

以下 advanced runtime / scheduling 能力已完成：

- `A1 MCP Resources And Prompts`
- `A2 MCP Streaming Evidence`
- `A3 MCP Long-Lived Stdio Session`
- `A4 Priority-Aware Local Run Scheduling`
- `A5 Web Console Runtime Management`
- `A6 Runtime Acceptance Gap Closure`

归档主文档：

- `../completed/advanced-runtime-scheduling-plan.md`

## Decision Arbitration Scope

当前已完成：

- `Decision Batch Resolution`

能力说明：

- Decision Queue can resolve multiple open Decision Tickets in one operation
- prevalidates ticket existence, open state, supported resolution type, and duplicate IDs

## LLM Driven Workspace Execution Delivery

LLM-driven Workspace execution 的设计、实现和验收已完成，边界包括：

- Soul / Skill / MCP / Persistent Memory 是正式业务上下文
- LLM 只输出结构化 `LLMExecutionDecision`
- Orchestrator 继续消费标准 Step output，并拥有 Run 状态、Evidence、Decision、Approval
- `application/workorch --server` / `internal/modules/application/workorchd` 负责 HTTP/Web、执行激活和上下文装配
- `blocks/llm` 负责 provider request、provider call、schema parsing 和 normalized Step output
- Web Console 只通过 HTTP API 交互，不暴露 provider secret 或低层 adapter 直接执行入口

当前细化开发项已全部完成：

- `L1` through `L19`

归档主文档：

- `../../20-runtime-flow/planning-execution-flow.md`
- `../completed/llm-driven-development-plan.md`

## SkillHub Capability Integration

已完成：

- SkillHub command adapter and status JSON mapping
- Skill read model fields for source/scope/status/path/hash/agent/managedBy
- `GET /api/v1/workorch/skills?workspaceId=...` 暴露 repository、global SkillHub、workspace SkillHub skills
- Workspace execution activation 解析 repository Skill 和 SkillHub skill refs
- Web activation Skill selector 使用 workspace-aware Skill list
- Goal/DoD-based automatic Skill selection
- repository Skill required MCP auto-binding
- `skill_feedback` Memory 写入
- deployed-instance e2e `verify-skillhub-workspace-execution.sh`
- deployed-instance full regression `make verify-deployed-full`

边界：

- Web 不直接触发 `skill-hub use/apply/feedback`
- 自动选择不依赖模糊业务语义推断
- `skill-hub feedback` 不由 workorch 自动触发

## Task Progress And Recovery Projection

已完成：

- Run snapshot 和 Run observability 暴露 `taskProgress`
- run / workspace `task-progress` API
- Web Workspace Detail 展示任务进度里程碑、阻塞项和恢复建议
- `ContextBundle` 只读上下文恢复包
- `CompressedContext` 确定性压缩摘要
- run / workspace `context-bundle` API
- `TaskDecomposition` 只读任务拆分投影
- run / workspace `task-decompositions` API
- start / resume 前自动将 `compressedContext` 写入 Run `executionContext`

边界：

- `TaskProgress` 不直接驱动调度或修改 Run 状态
- `TaskDecomposition` 不作为 scheduler / dispatcher 的新事实源
- `CompressedContext` 当前采用确定性压缩，不调用 LLM 二次摘要

## MVP Closure Record

MVP 已收口，不再作为活跃待办清单。

归档主文档：

- `../../mvp-backlog.md`

## Workspace Concurrency Historical Summary

历史综合状态记录中的阶段性收尾结论为：

- 快速回归基线：`concurrency=10,total=50`
- 高压验收基线：`concurrency=20,total=100`
- `concurrency>=30` 不再作为常规验收项

完整压测正文后续继续保留在：

- `workspace-concurrency-loadtest-record.md`

当前稳定并发门禁、脚本入口与指标口径已转入：

- `../../60-verification/loadtest-baseline.md`

## Run Closure Quality Follow-up

已识别下一轮运行闭环质量改进方向：

- unified `RunOutcome` / `WorkspaceOutcome`
- strict DoD-backed acceptance
- stronger Decision / Approval / repair recovery stability
- execution efficiency improvements
- plan / evidence / final Result / Artifact closure quality

当前这部分属于后续活跃跟踪，而不是已完成历史记录。

当前稳定入口与后续跟踪应以 roadmap / verification / overview 文档为准，
本节只保留“历史上已识别过这些方向”的追溯说明。

## Archive Boundary

本文件只承接：

- 已完成交付范围
- 历史阶段性完成记录
- 已完成能力的归档索引

本文件不承接：

- 当前稳定设计定义
- 当前稳定验收门禁
- 当前活跃 roadmap 待办

上述入口均应以当前正式结构为准，而不是本历史记录。

## Framework Alignment Closure Record

当前 framework alignment 已完成：

- `application/workorch` 通过 `magicCommon/framework/application` 启动
- `internal/modules/application/workorchd/service/server.Service` 接入 `magicCommon/framework/service.Service`
- 默认 runtime EventHub 接入 `magicCommon/event.Hub`
- request-scoped runtime modules 接入本地 plugin manager lifecycle
- HTTP dispatch 进入 `magicEngine/http`
- native system routes for `healthz` / `readyz` / `metrics`
- native workspace / run / decision / approval read routes
- native workspace / run / decision / approval write and execution routes
- native route registration guard for accepted API endpoints

## Migration Note

本文件承接的是历史综合状态记录里与“已完成交付”和“阶段性收口”有关的部分，
不再要求读者回到旧根路径查找当前事实入口。
