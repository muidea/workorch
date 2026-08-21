# Blocks Runtime Adapters Architecture

Status: active

Type: module-architecture

Last Verified: 2026-08-21

## Purpose

本文件说明 `internal/modules/blocks/*` 在 Runtime 下的统一职责与边界。

**生产主链：** capability blocks **只执行** sealed operation 或 candidate 请求；跨组件交互走 **typed EventHub command/result**。通用 RuntimeAdapter registry / Step kind registration matrix **不再是** 生产调度主链。

## Block Categories

### Stateful owners

| 模块 | 正式状态 |
| --- | --- |
| `blocks/workspace` | Workspace / Goal / DoD / Template |
| `blocks/run` | SM-1 Run lifecycle/lineage/ReplacementIntent；opaque RuntimeStateRecord CAS |
| `blocks/result` | Result / Artifact（formal outcome） |
| `blocks/decision` | DecisionTicket / Resolution |
| `blocks/approval` | Approval |
| `blocks/memory` | MemoryItem / RecallTrace |
| `blocks/soul` | Soul catalog |
| `blocks/skill` | Skill registry |
| `blocks/mcpregistry` | MCPServer / MCPCapability |
| `blocks/eventstream` | bounded EventRecord / high-watermark |

### Capability execution

| 模块 | 角色 |
| --- | --- |
| `blocks/llm` | Runtime candidate 请求（provider call + strict decode） |
| `blocks/file` | 文件 list/read/write/edit/patch；target snapshot prerequisite |
| `blocks/git` | git read/write 操作 |
| `blocks/shell` | 有界本地 command |
| `blocks/mcp` | MCP tool/resource/prompt 调用 |
| `blocks/web` | search / fetch |
| `blocks/ssh` | 受限远端 command |

## Shared Responsibility

所有 block 的共同职责：

- 订阅合同 owner 定义的 **具体** EventHub command；Runtime capability operation command/result 由接收 block 定义
- 执行外部 side effect 或 provider interaction（capability）/ 持久化 owner 状态（stateful）
- 返回 typed result、有界摘要、安全的 content-addressed ref（仅在 owner 合同允许时携带 raw ref）与具体 failure

block **不** 拥有：

- Run 生命周期与 Runtime drive
- Goal / DoD 正式裁决语义（Workspace 只管理定义本体）
- accepted Result / coverage / next action 判定
- 自动 retry / replan 策略

## Runtime Interaction Model

```text
Orchestrator sealed dispatch / prerequisite / candidate
  → contract owner 的 typed EventHub command
  → block 同步执行
  → typed result / evidence 摘要 / failure
  → Orchestrator 提交 evidence 并 reducer
```

要点：

- Orchestrator plugin **不** 创建 RuntimeAdapter registry、EventHub runtime adapter 或 Step kind registration matrix
- Runtime operation 按 capability 使用接收 owner 自己定义的 concrete command/result：File 按 list/read/edit/write/
  patch-preview/patch-apply 分 topic，Git/Shell/MCP/SSH/Web 使用各自 `ExecuteOperationCommand/Result`
- LLM 通过 `RequestRuntimeCandidateCommand/Result` 返回绑定当前 authority transaction 的 typed candidate
- File 另承担 `AcquireTargetSnapshot` 等 prerequisite 合同
- capability block 内部可以为外部 provider 或本地执行器保留局部解析结构；不得把 `map[string]any` 作为跨模块 command/data/result，也不得重新进入 EventHub / RuntimeFacts / candidate / dispatch / evidence

## Capability Operation Mapping

| Operation 方向 | Executor | 说明 |
| --- | --- | --- |
| Runtime candidate | `blocks/llm` | `StateChange` / `Validation` / `Assessment` / `Governance` candidate |
| file / patch operations | `blocks/file` | 技术执行；approval / evidence quality 仍归 Orchestrator |
| git operations | `blocks/git` | 有界 repo 读写 |
| shell command | `blocks/shell` | exitCode + bounded stdout/stderr + factual failure/diagnostic；evidence quality 仍归 Orchestrator |
| mcp tool/resource/prompt | `blocks/mcp` | 查询 mcpregistry 后调用；不注入 registry service |
| web search/fetch | `blocks/web` | 候选发现与正文抓取 |
| ssh command | `blocks/ssh` | opaque owner refs、exact approval、strict host identity 与前后 observation |
| target snapshot / manifest preflight | `blocks/file` 等 | prerequisite / capability preflight via typed commands |

约束：

- 映射只表达“谁执行”，不表达“业务上为什么执行”
- `file/write` 是 create-only；目标已经存在时必须选择带 current target revision 的 `file/edit`、
  `patch/apply` 或其他 owner-admitted replacement variant，重复 write 必须在副作用前拒绝
- 是否允许某 operation 由 runtime contract、focus requiredOutput、Soul/Skill/policy、approval 共同决定
- 无独立 `llm/assessment_only` step kind；assessment 是 canonical `need_assessment` / focus 模式
- patch 技术执行在 file；Patch evidence、coverage、result gate 权威在 Orchestrator

## LLM Block

负责：

- typed `PromptContext` 渲染与 C0–C5 稳定 cache prefix
- provider ceiling 校验、流式/非流式调用
- Runtime candidate 严格解码与合同校验

输出：

- sealed canonical state 对应的 typed candidate，或完整 factual candidate diagnostic
- **不** 返回 convergence / focus / authority / repair objective
- **不** 在进程内决定 correction、retry 或 terminal；attempt/policy 由 Orchestrator SM-3 管理

请求不得包含自由 `Input map[string]any`、禁止的 execution decision 或从历史 map 重建 definition。

## Capability Blocks Normalization

| Block | 规范化重点 |
| --- | --- |
| `llm` | candidate / schema repair / provider failure / request audit |
| `mcp` | tool/resource/prompt 结构化返回、server/capability 标识、risk 命中 |
| `shell` | command/args/exitCode/bounded preview/duration/error class |
| `file` | path/ref/hash/conflicts/approvalRequired/affectedFiles/target snapshot |
| `git` | operation/branch/head/changedFiles/summary |
| `web` | query/url/provider/resultCount/contentHash |
| `ssh` | opaque target/session/credential refs、exitCode/duration、bounded full-output hash、before/after remote revision |

统一约束：

- 大正文默认 bounded preview + raw ref
- 不得输出“已 accepted business outcome”
- 技术成功 ≠ 业务完成；evidence 进入 Orchestrator ledger 后才参与收敛

## Stateful Owner Boundary

| Owner | 可写 | 不可做 |
| --- | --- | --- |
| workspace | definition/revision/template/selection、execution-intent admission、active-run release | 选择 Run target state、判定 Result |
| run | SM-1 lifecycle/lineage/replacement、opaque Runtime CAS | 解释 authority、选择 action、裁决 evidence |
| result | formal Result/Artifact | acceptance / coverage 裁决 |
| decision | ticket / resolution | 直接改 Run 收敛状态；`runtime_governance` 不得并行通用 Resume |
| approval | 授权状态 | 执行原副作用 |
| memory / soul / skill / mcpregistry | 各自 registry 状态 | 调度 runtime 或写 Run/Result |
| eventstream | bounded audit records | 修复 owner 状态或重建 Runtime authority |

Orchestrator 与 application 一律经 EventHub 访问这些 owner。

## Capability Boundary Summary

- `blocks/llm`：provider-facing request/response only，不执行 workspace side effect
- capability execution blocks：外部动作 only，不拥有 Run 终态与 accepted closeout
- decision / approval：治理选择与授权，恢复路径由 Orchestrator contract gate 决定
- 生产调度权威 = Orchestrator reducer + sealed dispatch，**不是** 通用 adapter 注册表

## Explicit Non-Goals（禁止的）

- 通用 `RuntimeAdapter` 接口作为生产主链
- Step kind → adapter registry 矩阵驱动 dispatch
- block 发布 禁止的 RuntimeStep Accepted/Progress/Completed/Failed 推进状态
- block 内本地 repair / replan 主链
- 跨组件 map / JSON / reflect 类型桥

## Source Documents

- `architecture-overview.md`
- `kernel-orchestrator-governance.md`
- `../00-core/runtime-stage-contract.md`
- `internal/modules/kernel/orchestrator/readme.md`
- 各 `internal/modules/blocks/*/readme.md`
