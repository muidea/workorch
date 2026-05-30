# Blocks Runtime Adapter Architecture

Status: active

Type: module-architecture

Last Updated: 2026-05-30

## Purpose

本文件说明 `internal/modules/blocks/*` 的统一职责和边界。

## Main Blocks

- `blocks/llm`
- `blocks/mcp`
- `blocks/shell`
- `blocks/file`
- `blocks/git`
- `blocks/web`
- `blocks/ssh`

## Shared Responsibility

所有 block 的共同职责是：

- 实现某类 runtime adapter
- 执行外部 side effect 或 provider interaction
- 将执行结果标准化为 runtime 可消费输出

block 不拥有：

- Run 生命周期
- Goal / DoD 正式定义
- accepted Result 的最终判定

## High-Level Step Kind Mapping

当前稳定映射方向：

- `llm/*` -> `blocks/llm`
- `mcp/tool_call` / `mcp/resource_read` / `mcp/prompt_get` -> `blocks/mcp`
- `shell/command` -> `blocks/shell`
- `file/*` / `patch/*` -> `blocks/file`
- `git/*` -> `blocks/git`
- `web/*` -> `blocks/web`
- `ssh/command` -> `blocks/ssh`
- approval 正式状态与治理动作 -> `kernel/approval`

## Step Kind Registration Matrix

当前稳定 step kind -> block kind 注册矩阵至少应细化为：

| Step Kind | Block | 当前职责 |
| --- | --- | --- |
| `llm/workspace_execution` | `blocks/llm` | 主 LLM 决策、follow-up、assessment-only、repair/replan LLM 输出 |
| `llm/assessment_only` | `blocks/llm` | assessment / schema recovery / bounded correction |
| `mcp/tool_call` | `blocks/mcp` | 受治理的 MCP tool 调用 |
| `mcp/resource_read` | `blocks/mcp` | MCP resource 只读获取 |
| `mcp/prompt_get` | `blocks/mcp` | MCP prompt / template 读取 |
| `shell/command` | `blocks/shell` | 有界 shell / command 执行 |
| `file/list` | `blocks/file` | 目录扫描与相对路径枚举 |
| `file/read` | `blocks/file` | 文件正文读取与摘要 |
| `file/edit` | `blocks/file` | 小范围编辑请求，转换为 patch intent |
| `patch/preview` | `blocks/file` | diff 预演、风险、审批需求 |
| `patch/apply` | `blocks/file` | 正式落盘并生成 patch evidence |
| `git/read` | `blocks/git` | repo 状态、diff、log、show、branch 摘要 |
| `git/write` | `blocks/git` | add / commit 等最小写入闭环 |
| `web/search` | `blocks/web` | 候选来源发现 |
| `web/fetch` | `blocks/web` | 来源正文 / 摘要 / metadata 抓取 |
| `ssh/command` | `blocks/ssh` | 受限远端命令执行 |

约束：

- step kind 注册矩阵只表达“谁执行”，不表达“业务上为什么执行”
- 是否允许某个 step kind 出现在当前 run，继续由 runtime contract、Soul、Skill、policy、approval 共同决定
- 具体业务解释必须留在 profile / capability / product 层

## LLM Block

LLM block 负责：

- provider request assembly
- provider call
- schema parsing / repair normalization
- request audit / context budget related provider-facing output

### Output Direction

LLM block 输出应保持为：

- normalized decision object
- provider-facing context budget / request audit
- schema / provider failure 分类

## MCP / Shell / File / Git / Web / SSH Blocks

这些 block 负责：

- 外部动作执行
- output normalization
- 必要的 runtime-safe policy enforcement

它们不负责：

- 自主决定业务完成
- 绕过 orchestrator 直接改变 Run 终态

### Normalization Direction

这些 block 的输出应尽量统一为：

- structured result metadata
- bounded previews / summaries
- refs to large raw payloads
- deterministic failure classification when possible

## Block-Specific Normalization Differences

虽然统一要求“结构化元数据 + bounded summary + raw ref”，各 block 仍有稳定差异：

| Block | 主要规范化重点 |
| --- | --- |
| `blocks/llm` | normalized decision object、schema repair、provider failure classification、request audit |
| `blocks/mcp` | tool/resource/prompt 的结构化返回、server/capability 标识、approval/risk 命中 |
| `blocks/shell` | command/args/exitCode/stdoutPreview/stderrPreview/truncated |
| `blocks/file` | targetRef/baseHash/diffHash/conflicts/approvalRequired/affectedFiles |
| `blocks/git` | operation/branch/headCommit/changedFiles/summary |
| `blocks/web` | query/finalUrl/provider/resultCount/contentHash/retrievedAt |
| `blocks/ssh` | target host/session/exitCode/stdoutPreview/stderrPreview |

统一约束：

- 大正文、大 diff、大网页、大 stdout/stderr 默认只保留 bounded preview + raw ref
- block 不得输出“已 accepted business outcome”这类上层结论
- 若 block 结果可形成高风险或高价值 evidence，应通过 Step / ToolCall / Patch / ValidationResult / Artifact 进入正式证据链

## Capability Boundary Summary

各 block 的能力边界当前应稳定理解为：

- `blocks/llm`
  - 只负责 provider-facing request / response，不直接执行 side effect
- `blocks/mcp` / `blocks/shell` / `blocks/file` / `blocks/git` / `blocks/web` / `blocks/ssh`
  - 负责外部动作执行，但不拥有 Run 终态与 accepted closeout 决策
- approval 不属于 block adapter。Approval 正式状态、resolve 动作和治理关联由 `kernel/approval`
  承接，并由 orchestrator/governance 选择恢复路径。

## Source Documents

- `architecture-overview.md`
- `../20-runtime-flow/planning-execution-flow.md`
- `../50-product-interface/service-api.md`
