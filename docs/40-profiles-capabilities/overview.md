# WorkOrch Profiles And Capabilities Overview

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本目录用于承接 WorkOrch 的 profile/capability 层约束。

本层负责：

- software-dev / workspace / MCP / runtime capability 等特化语义
- template / Soul / capability allowlist / evidence policy 等 profile 规则
- 领域术语解释、建议验证方式、典型产物结构、profile-specific coverage matrix

本层不负责：

- 定义 core runtime contract
- 改写 domain model
- 取代 module architecture

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

- core runtime contract 仍以 `00-core/` 为准
- 业务对象定义属于 `10-domain-model/`
- 运行链路属于 `20-runtime-flow/`
- 代码实现落点属于 `30-module-architecture/`

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
