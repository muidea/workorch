# Acceptance Record: Profile Capability

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件记录对
`profile-capability-code-alignment-checklist.md`
的第一轮正式验收结果。

判定口径：

- `pass`
  - 当前代码与 profile/capability 设计一致，规则和边界落点成立
- `doc-ahead`
  - 文档要求已明确，但当前证据不足以证明实现已经完全落地
- `impl-drift`
  - 当前实现与 profile/capability 设计冲突

## Evidence Base

本轮主要检查以下实现与测试证据：

- `internal/modules/application/workorchd/biz/bootstrap/defaults/*`
- `internal/modules/application/workorchd/biz/appservice/workspace_execution_activation_service.go`
- `internal/modules/application/workorchd/biz/appservice/workspace_setup_draft_service.go`
- `internal/modules/application/workorchd/biz/appservice/{contextregistrycore,contextregistryports}`
- `internal/modules/application/workorchd/biz/appservice/{soul_service.go,skill_service.go,mcp_service.go,memory_service.go}`
- `internal/modules/kernel/soul/*`
- `internal/modules/kernel/mcp/*`
- `internal/modules/blocks/{file,git,web,shell,mcp}/*`
- `scripts/verify-workspace-template-soul.sh`
- `scripts/verify-workorch-runtime-profile-software-dev.sh`
- `scripts/verify-software-dev-goal-dod-e2e.sh`
- 相关 defaults / activation / read-model / runtime adapter 测试

## Acceptance Result

| Area | Evidence | Result | Notes |
| --- | --- | --- | --- |
| built-in templates / defaults | defaults embed、`templates.json`、`souls.json`、`mcp_capabilities.json`、`defaults_test.go` | pass | 内置模板、Soul 资源、MCP capability 默认资源均有正式 bootstrap 落点；`defaultSoulId` 已覆盖内置模板 |
| workspace soul routing / setup-draft | `activationports/soul_routing.go`, activation service, setup-draft service, tests | pass | routing 优先级、`executionContext.soulRouting` 投影、setup-draft 不创建 Run、activation 只消费正式定义均有直接证据 |
| software-dev profile | defaults + software-dev souls + verifier/e2e scripts | pass | software-dev 规则保留在 template/Soul/profile/verifier 层，没有看到错误下沉到 core/module |
| MCP capability | context registry ports/core, defaults, activation service, read-model projection | pass | `MCPServer` / `MCPCapability` 是正式对象；selected/used capability 投影成立；risk/approval/stage/display 字段有正式来源 |
| runtime capabilities | `blocks/file/git/web/shell/mcp`, patch approval/runtime patch adapters | pass | file/patch/git/web 等能力作为 runtime capability 承接 side effect；deliverable boundary 和 approval/risk 边界有代码与脚本证据 |
| cross-layer boundary | boundary verifier + activation contract + Web/TUI projection code | pass | template/profile 规则通过 defaults/contextHints/evidencePolicies/profileContracts 注入；前端消费投影结果，没有重新实现业务状态机 |

## Requirement Notes

### 1. Built-in templates / defaults

证据：

- `defaults.go` 的 embed 列表
- `defaults/templates.json`
- `defaults/souls.json`
- `defaults/souls/*/*.md`
- `defaults_test.go`

结论：

- 已证明文档声明的内置模板集合有正式默认资源落点
- 已证明模板至少具备：
  - `defaultSoulId`
  - `soulBindings`
  - `contextHints`
  - `evidencePolicies`
- 已证明 bootstrap 默认资源校验存在

### 2. Soul routing / setup-draft

证据：

- `activationports/soul_routing.go`
- `workspace_execution_activation_service_test.go`
- `workspace_setup_draft_service.go`

结论：

- 已证明 routing 按优先级解析
- 已证明结果写入 `executionContext.soulRouting`
- 已证明 setup-draft 只形成草案，不创建 Run
- 已证明 activation 只消费正式 Workspace / Goal / DoD / bindings

### 3. Software-dev profile

证据：

- `defaults/templates.json`
- `defaults/souls/software-dev/*.md`
- `verify-workorch-runtime-profile-software-dev.sh`
- `verify-software-dev-goal-dod-e2e.sh`

结论：

- 已证明 software-dev 规则主要通过 profile/template/Soul/default resource 生效
- 已证明 verifier/e2e 承接 software-dev 特化规则
- 未见软件开发语义错误下沉到 reusable core contract

### 4. MCP capability

证据：

- `contextregistrycore`
- `contextregistryports`
- `soul_service.go` / `skill_service.go` / `mcp_service.go` / `memory_service.go`
- `defaults/mcp_servers.json`
- `defaults/mcp_capabilities.json`
- activation service capability projection
- read-model `selectedCapabilities` / `usedCapabilities`

结论：

- 已证明 capability catalog 不是裸工具列表，而是正式 capability 对象
- 已证明 capability 具备：
  - `displayName`
  - `kind`
  - `category`
  - `stageHints`
  - `riskLevel`
  - `approvalPolicy`
  - `enabled`
- 已证明 selected/used capability 有正式后端投影

### 5. Runtime capabilities

证据：

- `blocks/file`, `blocks/git`, `blocks/web`, `blocks/shell`, `blocks/mcp`
- patch apply / approval 相关测试与脚本

结论：

- 已证明 file/patch/git/web 等能力作为 runtime capability 执行外部动作
- 已证明 patch/apply、preview、approval relation 有正式链路
- 未见它们直接拥有 accepted closeout 语义

### 6. Cross-layer boundary

证据：

- `verify-boundary-contract.sh`
- activation execution contract assembly
- Web/TUI 类型与页面代码中 selected/used capability、soulBindings 的消费路径

结论：

- 已证明 profile 规则通过 execution contract/profileContracts 注入
- activation 刷新中立 contract，但不重写 Workspace 持久化绑定
- Web/TUI 主要消费后端投影字段，没有看到前端重演 capability/template 选择规则

## Summary

本轮结论：

- profile/capability 层的设计与默认资源、activation contract、verifier/e2e、read-model 投影基本一致。
- 当前未发现明显 `impl-drift`。
- 也未看到必须保留为关键 `doc-ahead` 的核心缺口。

总体判定：

- 总体结果：`pass`
