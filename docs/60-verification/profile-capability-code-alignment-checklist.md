# Profile Capability Code Alignment Checklist

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件用于核对 `40-profiles-capabilities/` 当前 profile/capability 设计是否已经被代码正确承接。

它不重新定义 profile 规则，只把当前需要逐项核对的实现对齐项拆成可执行清单，
便于后续按专题逐项处理。

## Canonical Sources

核对时以以下文档为准：

- `../40-profiles-capabilities/builtin-templates-and-souls.md`
- `../40-profiles-capabilities/software-dev/goal-coverage-and-dod.md`
- `../40-profiles-capabilities/software-dev/code-agent.md`
- `../40-profiles-capabilities/software-dev/lsp-bridge-integration.md`
- `../40-profiles-capabilities/workspace/soul-routing.md`
- `../40-profiles-capabilities/workspace/soul-md-default-resource.md`
- `../40-profiles-capabilities/workspace/setup-draft-session.md`
- `../40-profiles-capabilities/mcp/capability-management.md`
- `../40-profiles-capabilities/runtime-capabilities/file-diff-patch.md`
- `../40-profiles-capabilities/runtime-capabilities/builtin-git-tool.md`
- `../40-profiles-capabilities/runtime-capabilities/web-search.md`
- `../00-core/runtime-stage-contract.md`
- `../20-runtime-flow/overview.md`
- `../30-module-architecture/overview.md`

## Review Goal

这一轮只回答三个问题：

1. profile/capability 规则是否已经在代码中有明确落点。
2. profile 规则是否仍停留在 profile/template 层，而没有错误下沉到 core/module。
3. 哪些点属于“文档超前”，哪些点属于“实现偏差”。

## Suggested Review Scope

建议优先核对以下代码目录：

- `internal/modules/application/workorchd/biz/bootstrap/defaults`
- `internal/modules/application/workorchd/biz/appservice`
- `internal/modules/kernel`
- `internal/modules/blocks`
- `web/workorch-console`

如 profile 配置、默认资源或 verifier 脚本另有落点，也应一并纳入。

## Review Order

建议顺序：

1. built-in templates / default resources
2. workspace soul routing / setup-draft
3. software-dev profile
4. MCP capability
5. runtime capabilities
6. cross-layer boundary

## 1. Built-in Templates / Default Resources

重点核对：

- 内置模板是否至少包括文档声明的模板集合。
- `templates.json` / `souls.json` / `defaults` 资源是否与文档描述一致。
- 每个模板是否有：
  - `defaultSoulId`
  - `soulBindings`
  - `contextHints`
  - `evidencePolicies`
- setup-draft 和 activation 是否真正消费这些模板默认资源。
- 模板是否只提供默认 contract / guidance，而不直接写死业务答案。

常见偏差：

- 文档声明了模板，但 defaults 资源不存在
- 模板在 activation 阶段被重新绑定 Workspace 持久化配置
- 模板通过 prompt 文本隐式授予结构化能力

## 2. Workspace Soul Routing / Setup-Draft

重点核对：

- Soul routing 是否真的遵守优先级：
  - Run 阶段覆盖
  - Workspace 阶段绑定
  - Run 默认 Soul
  - Workspace 默认 Soul
  - System 默认 Soul
- routing 结果是否写入 `executionContext.soulRouting`。
- routing 结果是否包含：
  - `status`
  - `effectiveDefaultSoulId`
  - stage `soulId`
  - `source`
- setup-draft 是否不创建 Run，只形成创建前草案。
- activation 是否只消费正式 Workspace / Goal / DoD / bindings。

常见偏差：

- orchestrator 根据 Goal/DoD 文本自行选择 Soul
- routing 结果没有落入 executionContext
- setup-draft 直接创建 Run

## 3. Software-Dev Profile

重点核对：

- `software-dev` 是否通过 template/profile 层生效，而不是下沉到 core runtime。
- Goal decomposition / DoD mapping / evidence policy 是否在 profile 配置或 Soul/default resource 中有明确落点。
- `software-dev` 是否依赖：
  - `file/*` / `patch/*`
  - `shell/command`
  - `git/*`
  - `mcp/tool_call`
  - `web/*`（仅在 profile 允许时）
- code-agent 是否仍围绕 Goal / DoD 收敛，而没有引入并行状态机。
- governance / approval 边界是否在 profile 规则中成立，而不是退化成普通步骤确认。

常见偏差：

- software-dev 规则直接写进 generic core/module
- DoD/evidence policy 只有文档，没有 defaults/profile 实现
- TUI/CLI 单独实现一套 software-dev 状态机

## 4. MCP Capability

重点核对：

- `MCPServer` / `MCPCapability` 是否作为正式 profile/capability 对象存在。
- capability 是否具备：
  - `displayName`
  - `kind`
  - `category`
  - `stageHints`
  - `riskLevel`
  - `approvalPolicy`
  - `enabled`
- Soul / Skill / Workspace / activation 是否引用 capability refs，而不是裸工具名白名单。
- `selectedCapabilities` / `usedCapabilities` 是否是正式投影。
- core runtime 是否没有按具体 tool/resource/prompt 名称做业务判断。

常见偏差：

- capability 目录只存在 server 原始工具列表，没有业务化 capability 对象
- selected/used capability 只有前端临时拼装
- tool 名称判断写进 core / orchestrator

## 5. Runtime Capabilities

重点核对：

- `file-diff-patch`、`git`、`web-search` 等能力文档是否在代码中有明确策略落点。
- file/patch 能力是否默认受 `deliverableDirectory` 约束。
- `patch/preview` / `patch/apply` / `file/edit` 是否职责分离。
- git/web/search 等能力是否只作为 runtime capability，不拥有 accepted closeout 语义。
- 风险操作是否通过结构化 policy/approval，而不是硬编码散落在多层。

常见偏差：

- file/write 或自由路径写入绕过 profile/deliverable boundary
- patch preview/apply 语义混杂
- web/git/tool 结果直接被当成业务完成

## 6. Cross-Layer Boundary Checks

重点核对以下跨层一致性：

- template / profile 规则是否只进入 profile/config/default resources，不进入 core runtime contract。
- Soul / Skill / MCP 的结构化 allowlist 是否由 profile/kernel/context 层消费，而不是由 `soul.md` 文本隐式授权。
- activation 是否刷新中立 execution contract，但不重绑 Workspace 持久化 template/bindings。
- API / Web / TUI 是否只消费 profile 投影结果，而不在前端重演业务规则。

常见偏差：

- profile 特定规则下沉到 core/module 通用逻辑
- 前端自行根据模板名、tool 名或业务词做判断
- activation 改写持久化 Workspace 绑定

## Classification Rules

核对结果建议分三类：

- `pass`
  - 设计与代码一致，profile 规则和边界落点成立
- `doc-ahead`
  - 文档定义已明确，但代码尚未正式落地
- `impl-drift`
  - 代码已有实现，但 profile 规则、默认资源或边界方向与当前设计冲突

## High-Priority Checks

优先检查这些高风险点：

1. template / Soul 默认资源是否与文档描述一致
2. Soul routing 是否真实进入 executionContext
3. software-dev 规则是否错误下沉到 core/module 通用逻辑
4. MCP capability 是否已从裸工具暴露面升级为正式 capability catalog
5. file/patch 能力是否真实受 deliverable boundary 和 approval policy 约束

## 2026-05-30 Code Alignment Result

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| built-in templates / defaults | `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`, `defaults/souls.json`, `defaults/souls/*/*.md`, `defaults.go` | pass | 内置模板、Soul Markdown 拆分资源、MCP server/capability 默认资源均有应用层 bootstrap 落点；本轮补齐所有内置模板 `defaultSoulId`，统一指向各模板 execution Soul，阶段绑定仍按 `soulBindings` 生效。 |
| workspace soul routing / setup-draft | `workspace_soul_routing.go`, `workspace_execution_activation_service.go`, `workspace_setup_draft_service.go` | pass | routing 优先级按 run stage、workspace stage、run default、workspace default、system default 解析，并写入 `executionContext.soulRouting`；setup-draft 只形成创建前草案，不创建 Run。 |
| software-dev profile | `defaults/templates.json`, `defaults/souls/software-dev/*.md`, `defaults_test.go`, `verify-workorch-runtime-profile-software-dev.sh` | pass | software-dev 规则保留在 template/Soul/default resource/profile verifier 层；core prompt 与 reusable runtime 已有边界脚本防止软件开发语义下沉。 |
| MCP capability | `internal/modules/kernel/mcp`, `defaults/mcp_servers.json`, `defaults/mcp_capabilities.json`, `workspace_execution_activation_service.go` | pass | `MCPServer` / `MCPCapability` 已是正式对象；capability 具备 display/category/stage/risk/approval/enabled 字段；activation 通过 capability refs 选择并投影。 |
| runtime capabilities | `internal/modules/blocks/{file,git,web,shell,mcp}`, `runtime_patch_adapter.go`, `runtime_patch_approval.go` | pass | file/patch/git/web/shell/mcp 均作为 runtime capability 执行外部动作，不拥有 accepted closeout；shell 与 patch/file 路径围绕 workspace deliverable boundary 执行。 |
| cross-layer boundary | `verify-boundary-contract.sh`, `workspaceExecutionContract`, `read_model_service.go`, Web/TUI projection | pass | profile 规则通过 template/contextHints/evidencePolicies/profileContracts 注入 execution contract；activation 不回写 workspace 绑定；Web/TUI 消费投影字段，不重新实现 profile 状态机。 |

### Closed In This Pass

- 内置模板缺少 `defaultSoulId` 属于默认资源 `impl-drift`；已在 `defaults/templates.json`
  补齐，并新增 bootstrap defaults 测试覆盖。

### Remaining Follow-Up

- 更细粒度的 profile 专项 e2e（例如 code-review / doc-generator / data-analysis）
  仍属于验证覆盖继续增强项，不是当前代码实现偏差。
- `selectedCapabilities` / `usedCapabilities` 已有 read-model 投影；后续可继续增强 UI 展示解释，
  但不应在前端重演 capability selection 规则。

## Suggested Output Template

后续逐项核对时，建议按以下格式记录：

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| built-in templates / defaults | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| soul routing / setup-draft | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| software-dev profile | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| MCP capability | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| runtime capabilities | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| cross-layer boundary | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
