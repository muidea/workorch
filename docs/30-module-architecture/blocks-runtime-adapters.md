# Blocks Runtime Adapters Architecture

Status: active

Type: module-architecture

Last Updated: 2026-07-29

## Purpose

本文件说明 `internal/modules/blocks/*` 在 Runtime V2 下的统一职责与边界。

**生产主链：** capability blocks **只执行** sealed operation 或 candidate 请求；跨组件交互走 **typed EventHub command/result**。通用 RuntimeAdapter registry / Step kind registration matrix **不再是** 生产调度主链。

## Block Categories

### Stateful owners

| 模块 | 正式状态 |
| --- | --- |
| `blocks/workspace` | Workspace / Goal / DoD / Template |
| `blocks/result` | Result / Artifact（formal outcome） |
| `blocks/decision` | DecisionTicket / Resolution |
| `blocks/approval` | Approval |
| `blocks/memory` | MemoryItem / RecallTrace |
| `blocks/soul` | Soul catalog |
| `blocks/skill` | Skill registry |
| `blocks/mcpregistry` | MCPServer / MCPCapability |

### Capability execution

| 模块 | 角色 |
| --- | --- |
| `blocks/llm` | Runtime V2 candidate 请求（provider call + strict decode） |
| `blocks/file` | 文件 list/read/write/edit/patch；target snapshot prerequisite |
| `blocks/git` | git read/write 操作 |
| `blocks/shell` | 有界本地 command |
| `blocks/mcp` | MCP tool/resource/prompt 调用 |
| `blocks/web` | search / fetch |
| `blocks/ssh` | 受限远端 command |

## Shared Responsibility

所有 block 的共同职责：

- 订阅本模块或 Orchestrator 定义的 **具体** EventHub command
- 执行外部 side effect 或 provider interaction（capability）/ 持久化 owner 状态（stateful）
- 返回 typed result、有界摘要、raw ref、具体 failure

block **不** 拥有：

- Run 生命周期与 Runtime V2 drive
- Goal / DoD 正式裁决语义（Workspace 只管理定义本体）
- accepted Result / coverage / next action 判定
- 自动 retry / replan 策略

## Runtime V2 Interaction Model

```text
Orchestrator sealed dispatch / prerequisite / candidate
  → typed EventHub command（Orchestrator pkg/events 或 block pkg/events）
  → block 同步执行
  → typed result / evidence 摘要 / failure
  → Orchestrator 提交 evidence 并 reducer
```

要点：

- Orchestrator plugin **不** 创建 RuntimeAdapter registry、EventHub runtime adapter 或 Step kind registration matrix
- V2 operation 按 capability 使用独立的 concrete EventHub command/result 合同投递：`ExecuteFileList/Read/EditOperation`、`ExecutePatchPreview/ApplyOperation`、`ExecuteShellOperation`、`ExecuteGitOperation`、`ExecuteMCPOperation`、`ExecuteSSHOperation`、`ExecuteWebOperation`
- LLM 通过 `RequestRuntimeCandidateCommand/Result` 返回绑定当前 authority transaction 的 typed candidate
- File 另承担 `AcquireTargetSnapshot` 等 prerequisite 合同
- capability block 内部可以为外部 provider 或本地执行器保留局部解析结构；不得把 `map[string]any` 作为跨模块 command/data/result，也不得重新进入 EventHub / RuntimeFacts / candidate / dispatch / evidence

## Capability Operation Mapping

| Operation 方向 | Executor | 说明 |
| --- | --- | --- |
| Runtime V2 candidate | `blocks/llm` | `StateChange` / `Validation` / `Assessment` / `Governance` candidate |
| file / patch operations | `blocks/file` | 技术执行；approval / evidence quality 仍归 Orchestrator |
| git operations | `blocks/git` | 有界 repo 读写 |
| shell command | `blocks/shell` | exitCode + bounded stdout/stderr + evidence policy 质量结论 |
| mcp tool/resource/prompt | `blocks/mcp` | 查询 mcpregistry 后调用；不注入 registry service |
| web search/fetch | `blocks/web` | 候选发现与正文抓取 |
| ssh command | `blocks/ssh` | 受限远端执行 |
| target snapshot / manifest preflight | `blocks/file` 等 | prerequisite / capability preflight via typed commands |

约束：

- 映射只表达“谁执行”，不表达“业务上为什么执行”
- 是否允许某 operation 由 runtime contract、focus requiredOutput、Soul/Skill/policy、approval 共同决定
- 无独立 `llm/assessment_only` step kind；assessment 是 canonical `need_assessment` / focus 模式
- patch 技术执行在 file；Patch evidence、coverage、result gate 权威在 Orchestrator

## LLM Block

负责：

- typed `PromptContext` 渲染与 C0–C5 稳定 cache prefix
- provider ceiling 校验、流式/非流式调用
- Runtime V2 candidate 严格解码与合同校验

输出：

- sealed canonical state 对应的 typed candidate，或完整 factual candidate diagnostic
- **不** 返回 convergence / focus / authority / repair objective
- **不** 在进程内决定 correction、retry 或 terminal；attempt/policy 由 Orchestrator SM-3 管理

请求不得包含自由 `Input map[string]any`、V1 execution decision 或从历史 map 重建 definition。

## Capability Blocks Normalization

| Block | 规范化重点 |
| --- | --- |
| `llm` | candidate / schema repair / provider failure / request audit |
| `mcp` | tool/resource/prompt 结构化返回、server/capability 标识、risk 命中 |
| `shell` | command/args/exitCode/bounded preview/duration/error class |
| `file` | path/ref/hash/conflicts/approvalRequired/affectedFiles/target snapshot |
| `git` | operation/branch/head/changedFiles/summary |
| `web` | query/url/provider/resultCount/contentHash |
| `ssh` | host/session/exitCode/bounded preview |

统一约束：

- 大正文默认 bounded preview + raw ref
- 不得输出“已 accepted business outcome”
- 技术成功 ≠ 业务完成；evidence 进入 Orchestrator ledger 后才参与收敛

## Stateful Owner Boundary

| Owner | 可写 | 不可做 |
| --- | --- | --- |
| workspace | definition / revision / template | 启动 Run、判定 Result |
| result | formal Result/Artifact | acceptance / coverage 裁决 |
| decision | ticket / resolution | 直接改 Run 收敛状态；`runtime_v2_governance` 不得并行通用 Resume |
| approval | 授权状态 | 执行原副作用 |
| memory / soul / skill / mcpregistry | 各自 registry 状态 | 调度 runtime 或写 Run/Result |

Orchestrator 与 application 一律经 EventHub 访问这些 owner。

## Capability Boundary Summary

- `blocks/llm`：provider-facing request/response only，不执行 workspace side effect
- capability execution blocks：外部动作 only，不拥有 Run 终态与 accepted closeout
- decision / approval：治理选择与授权，恢复路径由 Orchestrator contract gate 决定
- 生产调度权威 = Orchestrator reducer + sealed dispatch，**不是** 通用 adapter 注册表

## Explicit Non-Goals（V1）

- 通用 `RuntimeAdapter` 接口作为生产主链
- Step kind → adapter registry 矩阵驱动 dispatch
- block 发布 V1 RuntimeStep Accepted/Progress/Completed/Failed 推进状态
- block 内本地 repair / replan 主链
- 跨组件 map / JSON / reflect 类型桥

## Source Documents

- `architecture-overview.md`
- `kernel-orchestrator-governance.md`
- `../00-core/runtime-stage-contract.md`
- `internal/modules/kernel/orchestrator/readme.md`
- 各 `internal/modules/blocks/*/readme.md`
