# WorkOrch Profiles And Capabilities Overview

Status: active

Type: profile-capability

Last Updated: 2026-07-29

## Purpose

本目录用于承接 WorkOrch 的 profile/capability 层约束。

本层负责：

- software-dev / workspace / MCP / runtime capability 等特化语义
- template / Soul / capability allowlist / evidence policy 等 profile 规则
- 领域术语解释、建议验证方式、典型产物结构、profile-specific coverage matrix
- **stage policy / focus 映射作为 typed C2 与 activation 输入**，供 candidate 上下文与 requiredOutput 中立交集使用

本层不负责：

- 定义 core runtime contract 或 Runtime 生产状态机
- 改写 domain model
- 取代 module architecture
- 让 kernel 理解 profile ID、模板名或业务领域语义

## Kernel Neutrality

- core / Orchestrator / reducer **不理解 profile IDs**。
- profile 只能输出结构化字段：stage policy、evidence policy、capability allowlist、focus/coverage 提示、marker 等。
- 这些字段进入：
  - typed `RuntimeBootstrap` / Activation
  - candidate context 的 **C2 stable capability contract** 与相关 section
  - requiredOutput / evidence gate 的中立约束
- 中立 stage 语义（exploration / planning / execution / ...）用于 profile 映射与 LLM 输出策略；生产决策只以 Runtime canonical `ConvergenceState` 为准，candidate mode/output kind 由其派生。
- 不得把 profile 策略重新包装为 `StagePacket` / graph dispatch / 自动 replan successor。

## Reading Order

1. [builtin-templates-and-souls.md](builtin-templates-and-souls.md)
2. `software-dev/`
   - [goal-coverage-and-dod.md](software-dev/goal-coverage-and-dod.md)
   - [lsp-bridge-integration.md](software-dev/lsp-bridge-integration.md)
   - [code-agent.md](software-dev/code-agent.md)
3. `workspace/`
   - [soul-routing.md](workspace/soul-routing.md)
   - [soul-md-default-resource.md](workspace/soul-md-default-resource.md)
   - [setup-draft-session.md](workspace/setup-draft-session.md)
4. `mcp/`
   - [capability-management.md](mcp/capability-management.md)
5. `runtime-capabilities/`
   - [builtin-git-tool.md](runtime-capabilities/builtin-git-tool.md)
   - [file-diff-patch.md](runtime-capabilities/file-diff-patch.md)
   - [web-search.md](runtime-capabilities/web-search.md)

## Cross-Layer Boundary

- core runtime contract / Runtime 主链仍以 `00-core/` 为准
- 业务对象定义属于 `10-domain-model/`
- 运行链路属于 `20-runtime-flow/`
- 代码实现落点属于 `30-module-architecture/`
- candidate request 的 C0–C5 分区见 `../00-core/runtime-request-cache-note.md`

## Current Gaps

当前本层仍未完全收口的重点包括：

- `builtin-templates-and-souls.md`
  - 缺逐模板 / 逐 Soul 的进一步拆分
- `software-dev/*`
  - 已补 verifier / module / product 第一轮映射，仍缺更细 acceptance dimension -> fixture 对应表
- `workspace/*`
  - 已补 soul routing / default resource / setup-draft 的 API、verifier、表单字段、deliverable boundary、memory/context UX、activation 映射与产品示例
- `mcp/capability-management.md`
  - 已补 catalog / adapter / governance / read-model 映射、第一轮审批矩阵、UI 字段表与 activation contract 字段映射
- `runtime-capabilities/*`
  - 已补 step kind / adapter / verifier / artifact 第一轮交叉引用，以及审批矩阵 / product examples / read-model 字段表，仍缺少量更细跨产品示例

完整目录级缺口总表见：

- `../01-overview/current-unclosed-matrix.md`
