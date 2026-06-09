# Workspace Soul Routing

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接 workspace 维度的 Soul routing 设计。

## Scope

- workspace default Soul
- stage-specific Soul binding
- run-scoped override
- priority resolution

## Stable Rules

### Routing Targets

Workspace 维度的 Soul routing 至少应覆盖以下阶段：

- `planning`
- `execution`
- `decision`
- `approval`
- `validation`
- `artifact`

同一 Workspace 可以同时配置默认 Soul 和阶段 Soul；同一 Run 可以再对默认 Soul 或阶段 Soul 做一次性覆盖。

### Priority Resolution

Soul 启用优先级从高到低应为：

1. Run 阶段级覆盖
2. Workspace 阶段级绑定
3. Run 默认 Soul
4. Workspace 默认 Soul
5. System 默认 Soul
6. `unbound`

阶段级绑定始终高于默认 Soul；Run 级覆盖只影响当前 Run，不回写 Workspace。

### Execution Context Projection

- 最终解析结果必须写入 `executionContext.soulRouting`
- routing 结果必须记录 `status`、`effectiveDefaultSoulId`、各阶段 `soulId` 和 `source`
- 无任何 Soul 时允许 `unbound`，但必须明确记录原因

### Capability Boundary

- Soul routing 只决定“当前阶段使用哪个 Soul”，不直接改写 Skill/MCP/Tool 权限来源。
- `soulMd` 是提示词内容，不是授权源。
- 结构化能力边界仍应来自 `allowedSkillRefs`、`allowedMCPServerRefs`、`allowedCapabilityRefs`、`allowedCapabilityCategories`、`allowedToolKinds`。
- execution 阶段的 Skill/MCP/Capability 集合统一由最终 execution Soul 校验；非执行阶段只约束各自阶段行为。

### Module Boundary

- `application/workorch` 是唯一的 routing 解析层，负责在 Run 创建/恢复前解析并固化 routing。
- `kernel/orchestrator` 只消费 execution context，不根据 Goal/DoD 文本选择 Soul。
- `blocks/llm` 只根据 Step input 和 `executionContext.soulRouting` 选择当前阶段 Soul 上下文，不读取仓储、不修改绑定。

## API / Read-Model Mapping

Soul routing 的稳定产品面包括：

- `GET /workspaces/:workspaceId/soul-bindings`
- `PUT /workspaces/:workspaceId/soul-bindings`
- `GET /workspaces/:workspaceId/snapshot` 中的 `contextBundle`
- `GET /workspaces/:workspaceId/runs/:runId/context-bundle`
- `GET /workspaces/:workspaceId/runs/:runId/observability`

读模型方向：

- workspace 级 `soulBindings`
  - 表达持久化绑定
- run 级 `executionContext.soulRouting`
  - 表达本次 run 的最终生效解析结果
- context bundle / observability
  - 表达当前阶段实际使用的 Soul 与来源

## Memory / Context UX

workspace profile 在运行态的 context UX 应遵循“先结果与阻塞，再上下文来源”的顺序。

当前稳定展示优先级：

1. active Workspace / active Run
2. task progress / blocker / result summary
3. 当前阶段生效的 Soul routing
4. `ContextBundle.sections`
5. `memoryRecallTraces`

展示约束：

- 用户默认只需要看到“当前阶段用了哪个 Soul、为什么”
- `memoryRecallTraces` 应作为可展开解释层，不应挤占主会话主视图
- `selectedContext`、`memoryIds`、`memoryRecallRefs` 应以摘要方式进入 UI，不直接暴露原始 executionContext JSON
- superseded / failed run 的低价值 recall 不应默认压过当前 active run 的高价值 context

对 TUI / Console 的典型方向：

- 主视图
  - active workspace / result / blocker / current Soul
- 右侧或次级面板
  - all-workspace monitor / context bundle summary
- 展开详情
  - memory recall trace、selected capabilities、stage-specific Soul source

## Deliverable Boundary Profile View

Soul routing 虽然不直接定义 deliverable boundary，但必须尊重 workspace deliverable boundary：

- execution Soul 只能约束“如何操作 workspace deliverable”
- 不应通过 Soul routing 把 internal runtime state 目录暴露为普通交付目录
- context bundle / observability 展示中的文件、artifact、patch、git 证据应优先面向 deliverable boundary，而不是 internal storage path

## Product Examples

典型产品展示方向：

- Workspace settings / bindings 面板
  - 展示 `defaultSoulId`、阶段 `bindings`、priority policy
- Run detail / context bundle
  - 展示当前阶段生效的 `soulId`、`source`、`effectiveDefaultSoulId`
- TUI 主视图
  - 只提示 active Workspace / blocker / current Soul，不把全部 routing 细节堆到主屏
- 可展开 context/memory 详情
  - 展示 `memoryRecallTraces`、selected context 摘要、阶段来源

## Verification Mapping

当前与 soul routing 直接相关的验证入口：

- `make verify-workspace-template-soul`
- `verify-llm-workspace-execution.sh`
- `verify-software-requirement-development-orchestration.sh`

这些入口分别验证：

- workspace 级 Soul bindings 的 CRUD 与模板初始化
- run activation / execution 中 Soul routing 是否进入 execution context
- 决策、审批、验证等阶段是否仍保持统一路由来源

## Cross-Layer Mapping

- domain model：
  `../../10-domain-model/context-capability-memory.md`
- context flow：
  `../../20-runtime-flow/context-assembly-flow.md`
- kernel/context capability：
  `../../30-module-architecture/kernel-context-capability.md`
- API：
  `../../50-product-interface/service-api.md`

## Boundary

- routing 结果写入 execution context
- core runtime 只消费 routing 结果，不自行选择 Soul

## Source Documents

- `../builtin-templates-and-souls.md`
