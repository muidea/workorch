# Software-Dev LSP Bridge Integration

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接 `software-dev` profile 中对 LSP bridge 集成的特化说明。

## Scope

- 仅描述 software-dev profile 如何使用 LSP bridge
- 不定义 core runtime contract
- 不定义通用 MCP 或通用 capability 规则

## Positioning

`lsp-bridge` 应作为 `software-dev` 的可选 MCP server / capability 接入，用于在 planning、execution、repair、validation 阶段提供语义导航和诊断辅助。

推荐能力集合：

- `lsp_initialize`
- `lsp_sync`
- `lsp_definition`
- `lsp_hover`
- `lsp_references`
- `lsp_diagnostics`
- `lsp_status`
- `lsp_shutdown`
- `lsp_repair`

不推荐做法：

- 在 orchestrator 或 core runtime 中固定调用 LSP
- 用 LSP 诊断替代 build/test/lint/DoD evidence
- 把语言服务器命令、语言规则和初始化策略写入 core

## Configuration Boundary

- WorkOrch 不负责在线安装 `lsp-bridge` 或语言服务器；部署环境必须预先准备可执行命令或绝对路径。
- 配置应采用“全局默认 + workspace 私有配置”的模式。
- workspace 私有配置属于 runtime internal state，不进入 workspace 交付物目录，也不应成为 patch/apply 目标。
- 若使用 long-lived stdio session，session key 至少应包含 `workspaceId + serverId`，避免不同 workspace 串扰。

## Stage Usage Contract

### Planning

- 可用 `lsp_initialize`、`lsp_definition`、`lsp_references`、`lsp_hover` 辅助定位入口、接口与影响面；可用 `lsp_status` 判断语义会话与依赖是否可用。
- 多语言项目可按语言分别初始化。
- LSP 不可用时应回退到 file/read、rg、build/test 输出，不得阻塞整个 run。

### Execution

- 生成 patch 前可用 definition/references 辅助定位目标。
- 文件修改后，如需基于未落盘内容继续查询，应使用 `lsp_sync` 同步完整文件内容。
- `lsp_diagnostics` 可用于快速发现语法、类型、未定义符号等问题。
- `lsp_status` 可用于确认语义会话状态；`lsp_shutdown` / `lsp_repair` 只能用于 workspace-local 实例级恢复，不能修改交付文件。
- LSP 不负责状态变更；文件修改仍必须通过 patch/apply 或其他明确 state-changing tool 完成。

### Repair

- 对 build/test/lint 失败中的符号、类型、接口问题，可优先使用 diagnostics + definition/references 收敛根因。
- `lsp_references.truncated=true` 时，不得声称已完成全量影响面分析。
- LSP 查询为空时不得扩散为无限探索。

### Validation

- diagnostics clean 只能作为辅助质量信号。
- build/test/lint 仍是 DoD 验收主证据。
- LSP evidence 可作为辅助 `evidenceRef`，但不能单独支撑 accepted result。

## Context And Evidence

- LSP 输出进入 LLM 上下文时应压缩为结构化摘要，只保留当前失败相关 diagnostics、当前 patch 影响文件和当前 repair target 的 definition/references 摘要。
- 不应把完整源码或无关 hover 长文本重复注入上下文。
- `lsp_diagnostics` clean、`lsp_definition`、`lsp_references`、`lsp_sync` success 都属于辅助证据，不应单独满足高风险 DoD 维度。

## Failure Handling

LSP 类故障应保持为辅助能力降级，而不是默认 blocker：

- server command missing / language server missing：标记 capability unavailable，回退普通文件/命令流程
- initialize / diagnostics timeout：有界重试一次，仍失败则跳过该辅助能力
- query empty：作为空结果处理，不进入 `blocked_on_decision`
- sync failed：记录 tool failure，回退到磁盘文件状态

只有当 Goal / DoD 明确要求语义索引或 LSP 能力时，LSP 不可用才可以成为 blocker。

## MCP / Capability Mapping

`lsp-bridge` 在 WorkOrch 中应作为 `software-dev` profile 的 MCP server / capability
集合出现，而不是特殊内置 runtime block。

稳定能力方向：

| 能力 | 典型用途 | 证据属性 |
| --- | --- | --- |
| `lsp_initialize` | 初始化 workspace 语义会话 | 辅助证据 |
| `lsp_sync` | 同步未落盘文件内容 | 辅助证据 |
| `lsp_definition` | 查找定义/入口 | 辅助证据 |
| `lsp_hover` | 读取符号说明 | 辅助上下文 |
| `lsp_references` | 分析影响面 | 辅助证据 |
| `lsp_diagnostics` | 发现语法/类型/符号错误 | 辅助质量信号 |
| `lsp_status` | 查看语义会话、依赖、活跃/空闲和重启状态 | 辅助运行状态证据 |
| `lsp_shutdown` | 关闭 workspace-local 语义实例 | 辅助恢复动作 |
| `lsp_repair` | 诊断或安全恢复 workspace-local 语义实例 | 辅助恢复动作 |

约束：

- 这些能力应通过 `mcp/tool_call` 落到统一 adapter 路径
- LSP 诊断不应越权替代 build/test/lint 主证据

## Step Kind / Adapter / Read-Model Mapping

当前跨层映射方向：

- step kind
  - `mcp/tool_call`
- adapter
  - `blocks/mcp`
- registry / policy
  - `kernel/mcp`
- 读模型承接
  - `tool-calls`
  - `run observability`
  - `context bundle`
  - 必要时 `memoryRecallTraces`

产品层主要查看：

- `tool-calls`
  - 具体 LSP 查询动作
- `run observability`
  - repair / validation 阶段如何使用语义能力
- `context bundle`
  - 当前注入给 LLM 的压缩语义摘要

## Verification Mapping

当前与 LSP bridge 直接相关的验证入口：

- `make verify-runtime-profile-software-dev`
- `verify-software-dev-goal-dod-e2e.sh`
- `verify-software-dev-code-agent.sh`

这些入口分别验证：

- LSP 输出是否仍停留在 profile/capability 层
- software-dev 执行链是否把 LSP 视为辅助能力而非主证据
- code-agent 与 MCP capability 的边界是否保持一致

## Cross-Layer Mapping

- software-dev profile：
  `goal-coverage-and-dod.md`
- MCP capability：
  `../mcp/capability-management.md`
- block 分发：
  `../../30-module-architecture/blocks-runtime-adapters.md`
- profile verifier：
  `../../60-verification/profile-software-dev-verifier.md`

## Source Documents

- `../mcp/capability-management.md`
