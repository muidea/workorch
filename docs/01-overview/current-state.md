# WorkOrch Current State

Status: active

Type: overview

Last Updated: 2026-08-08


## LLM Provider（2026-07-15）

- 首选 provider：内置 `ai-proxy`
- 模型发现：`blocks/llm` 读取 `/v1/models` catalog
- workspace 下一次运行选择：`blocks/workspace` CAS 持久化可选 `DefaultLLMSelection`；选择包含完整 ai-proxy model target 与可选 reasoning effort
- activation：fresh Start/Restart 读取 Workspace owner canonical selection；为空时使用当前系统 `defaultChat` 且省略 reasoning；caller 不得提交仅对本次 Run 生效的 selection patch
- Run 封存：`blocks/run` 在 activation 中持久化 `ProviderInferenceSelection`；Resume 与进程恢复只使用该 sealed fact，不重读 mutable Workspace 偏好
- TUI 运行控制通过 scoped Run intent 提交；可继续的 current-schema Run 才可 `/run resume`，terminal 或
  `fresh_activation_required` Run 必须由用户显式 restart 创建新的 Runtime activation，TUI 不再以本地状态猜测续跑路径
- TUI：`/model` 自动列出 ready Responses target；`/model reasoning <effort|default>` 更新当前 workspace 的下一次运行偏好；`/model show` 区分 next-run 与 current-run sealed selection
- Runtime：`ProviderCeiling` 封存完整 selection + token/byte ceiling + capability/configuration revision；reasoning effort 不改变 CWI、预算或稳定 prompt prefix
- LLM wire：`blocks/llm` 只依据 ai-proxy catalog 校验显式 effort，并在 `/v1/responses` 中编码 nested `reasoning.effort`；空 effort 完全省略 `reasoning`
- 旧 `defaultProvider` / provider list / model pattern / provider context 路径已删除

## Purpose

本文件是仓库「现在怎样」的唯一事实入口。它只描述当前已落地的生产主链、文档权威分层与剩余未关项，不承载历史 MVP/P1 记录或已归档 roadmap 正文。

## Production Runtime Truth

**生产代码路径 = Runtime only。**

> 实施状态限定：历史 RUNTIME-SMO 与 SMO-R2-C12 已完成 decision/output、failure disposition、owner code
> taxonomy、operation outcome、terminal projection 和 semantic evidence-binding 的 contract 归一，并只作回归证据。
> per-CWI immutable binding 已收口 raw-credit 越权：candidate 自报 coverage、非空 `PassedChecks`、文件/Git
> 存在均不能直接令交付物成为 `coverage_satisfied`。slice/focus 分离、reconciliation contract、generation/history、
> owner observation、activation recommendation 与 single-owner intent 主链已落地并有定向回归覆盖。fresh restart
> 已能从 Goal/DoD + selected template/profile + owner facts 请求受约束 semantic plan，由 capability owner 返回
> feasibility，并由 SM-2 原子 seal；普通 UI/API 已不再暴露 raw execution-contract JSON。Run snapshot/RuntimeView
> 已提供 run-scoped semantic phase、逐 CWI owner fact、完整原始 diagnostic 与 gap lineage，并拒绝跨 run/revision 的
> stale 投影。fresh-process recovery、CPU guard、sealed-contract operation 主链与 R3-F 全链验收均已闭环；当前仅因
> LSP Bridge deployed proof 尚未验收，R3 保持 active。

- 当前 Start / Resume 由 TUI/Web/application 仅提交 scoped workspace intent；Workspace/Run 各自 internal event
  完成 slot/lifecycle 后发布事实，Orchestrator 先执行 `current_run` 或 `workspace_full` reconciliation，再进入
  Runtime drive。缺少 typed bootstrap 必须按 owner-local `fresh_activation_required` 事实收敛。
- 唯一同步推进合同：`DriveRuntimeCommand/Result`（Orchestrator）。
- 正式运行时状态：`RuntimeStateRecord` / Runtime aggregate（Run owner CAS 持久化）。
- 生产主链不包含 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、自动 replan successor、progress/terminal checkpoint replay 或通用 RuntimeAdapter registry。
- 完整生产规约由四个协调状态机组成：SM-1 Run lifecycle、SM-2 authority/focus convergence、SM-3 candidate/sealed dispatch transaction、SM-4 drive/recovery/projection delivery；任何 runtime 变更必须通过四状态机变更门禁。
- complete candidate 只经过一个 evaluator；evaluator 只返回完整 typed rejection，SM-3 internal event 再决定
  同 manifest 内唯一 correction 或明确终态。owner feasibility 是独立事实，必须经 Orchestrator internal event
  进入 SM-2 后继，绝不消耗 candidate attempt。各预算按稳定 authority contract identity 分离。
- coverage 只能由 Orchestrator SM-2 从 sealed `CoverageEvidenceBinding`、sealed operation 和 matching owner
  receipt 派生。LLM candidate 的 raw CWI、assessment claim、非空 `PassedChecks`、exit 0、文件/Git 存在或通用
  test 成功均不是 coverage authority；每 binding 还必须具有 CWI-specific semantic proof，并先由 current
  workspace reconciliation 决定 retain/reopen。fresh restart 缺 exact proof 时进入有界 semantic planning：LLM 只提
  proposal，capability owner 只给 feasibility，只有 SM-2 seal 能形成 canonical binding；未 seal 前禁止 operation candidate。
- drive 同时要求 persisted revision 与 convergence progress signature 推进；仅增加 retry/snapshot bookkeeping 不算正向进展。
- terminal delivery 统一为 persisted terminal → Result owner → Workspace owner → Run owner；SM-4 只保存
  delivery acknowledgement 并对缺失回执执行幂等重试。
- projection readiness 通过 EventStream typed high-watermark 查询核对真实 source revision；stale/degraded projection 不再通过 `/readyz`。

一句话架构真值：

> Application 只构造 typed `RuntimeBootstrap` 并提交 Run activation/resume intent；Run owner持久化 lifecycle 与
> Runtime aggregate；Orchestrator 是唯一 reducer / drive / evidence / governance-resolution 权威；capability
> blocks 只执行并回执；Result/Workspace/Run 各自只迁移自己的终态资源；Eventstream / projection 只审计与展示；
> 恢复 = 从 aggregate 幂等再 drive。

关键代码落点：

- 实现 SoT：`internal/modules/kernel/orchestrator/readme.md`
- 类型合同：`internal/pkg/workorchruntimecontract`（仅 DTO、合同与纯无状态 helper）
- 主链实现：`internal/modules/kernel/orchestrator/runtime_*.go`、`biz/runtime/`

## Dual-Layer Contract Model

正式文档采用 **双层并存**：

| 层 | 语言 | 用途 | 是否生产状态机 |
| --- | --- | --- | --- |
| A. 语义 / profile | 中立 stage：`exploration` / `planning` / `execution` / `verification` / `acceptance` / `repair` / `replan` | output policy、profile 映射、LLM 意图、evidence 严格度 | 否 |
| B. Runtime 实现 | bootstrap → facts → reducer → prerequisite / candidate → sealed dispatch → evidence → terminal | 生产调度与收敛 | 是 |

映射与不变量见：[00-core/runtime-stage-contract.md](../00-core/runtime-stage-contract.md)。

## Documentation Authority

| 优先级 | 角色 | 路径 |
| --- | --- | --- |
| 1 | Runtime 中立规则 SoT | [runtime-stage-contract.md](../00-core/runtime-stage-contract.md) |
| 1b | Request / cache 从属 | [runtime-request-cache-note.md](../00-core/runtime-request-cache-note.md) |
| 2 | 实现 SoT | `internal/modules/kernel/orchestrator/readme.md` + `internal/pkg/workorchruntimecontract` |
| 3 | 当前事实入口 | 本文 |
| 4 | 主链叙述 | [20-runtime-flow](../20-runtime-flow/overview.md)、[30-module-architecture](../30-module-architecture/overview.md) |
| 5 | 未关项 | [70-roadmap/active/overview.md](../70-roadmap/active/overview.md) |
| 6 | 历史 | [90-archive](../90-archive/completed/overview.md) |

冲突规则：中立约束以 `00-core` 为准；实现细节以 Orchestrator readme + contract 包为准；本文只摘要当前事实；已归档 plan 不得并列约束新实现。

## Ownership and Component Type Snapshot

22 个 framework 运行单元的职责类型固定为 **1 Initiator / 4 Modules / 17
Blocks**。目录与职责类型一致：`blocks/run` 与 `blocks/eventstream` 是单一资源
Block owner，不承担跨 owner 编排。

| 类型 | 组件 | 职责 |
| --- | --- | --- |
| Initiator | `initiators/workorchruntime` | 单一 repository provider/factory；无业务状态或 EventHub |
| Module | `application/workorchd` | HTTP/CLI/SSE/projection；构造 bootstrap；不持有 drive queue |
| Module | `kernel/orchestrator` | Runtime convergence authority |
| Module | `kernel/governance` | 跨 Decision/Approval/Run 的治理协调；不拥有 Runtime continuation |
| Module | `kernel/system` | 跨 owner 的启动默认资源 ensure 协调 |
| Block | `blocks/run` | Run/Step 的 SM-1 owner；提供 Orchestrator-owned RuntimeStateRecord opaque 物理 CAS/query，不解释 SM-2/3/4 |
| Block | `blocks/eventstream` | 审计 EventRecord/high-watermark owner；不是业务修复入口 |
| Block | `blocks/workspace` | Goal / DoD / Template 定义 owner |
| Block | `blocks/result` | 正式 outcome 存储；不裁决 acceptance |
| Block | `blocks/decision` / `approval` | 治理 ticket / 授权 owner |
| Block | `blocks/memory` / `skill` / `soul` / `mcpregistry` | 单一正式资源 registry/aggregate owner |
| Block | `blocks/llm` / `file` / `git` / `shell` / `mcp` / `ssh` / `web` | 单一技术能力；返回 candidate、observation 或 receipt |

跨组件交互：EventHub + 具体 command/data/result。禁止跨 owner service/repo、Envelope、`map[string]any` bridge。

## Remaining Open Work

历史 fresh Run 的局部成功、R4/P0～P6/A-4 过程分段和旧 VPC 的 N1～N8 只保留为 archive regression
evidence，不再定义当前实施顺序，也不能代替最新 deployed live。最新 persisted aggregate 反例优先决定当前未关项；
active 文档不保存 workspace/run 的 dated 过程日志或 raw provider/runtime data。

1. **必需 Runtime live 验收：R3-E3 fresh LSP Bridge semantic matrix。** single-owner lifecycle、minimal bootstrap、
   受约束 semantic-plan document、owner feasibility、SM-2 逐 CWI seal、`resume=current_run`、
   `restart=workspace_full`、空 workspace prospective target、crash/concurrency 与 CPU guard 已完成代码及 deployed
   闭环。普通用户只提供 Goal/DoD/template/directory，不填写 Runtime JSON。当前只剩 fresh LSP Bridge 的逐 CWI
   binding-matched proof；所有状态仍只由 owner-local event/reducer 变更。详见
   [R3 active plan](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md)。
2. **回归基线**：immutable binding、strict candidate、receipt match、replay/CAS 与双 workspace
   regressions 必须持续通过。
3. **可选验证（OPT-1）**：可补 file-patch `apply + validation + DoD closeout` 的 deployed live 一次。
4. **延期候选**：LLM context budget 高级项见 `70-roadmap/candidates/`，**不占 active**。

此前的 OPS-1 预设了一个不存在的 Runtime maintenance command；经实现、合同与 CLI 复核后已从 active
移除。历史数据只能按现有 owner 的只读审计边界保留，不能据此重建第二条 production maintenance 主链。

ARCH-1 的 22 组件 boundary/lifecycle 收口已归档，包含 workspace delete → late projection event 的
managed deployed 证据；EC 的 owner-local concrete Topic/Data/Result 审计及 full race、console、managed deployed、
fresh 双 workspace/C7 验证也已归档。它们均不再是 active 工作项。

旧 candidate-admission、fresh live 与 dated cleanup 记录只在 [90-archive](../90-archive/completed/overview.md)
检索；它们不再出现在当前事实正文，也不能覆盖 Runtime core 的 current-schema 反例。

统一清单：

- [current-unclosed-matrix.md](current-unclosed-matrix.md)
- [unfinished-todo-index.md](unfinished-todo-index.md)
- [70-roadmap/active/overview.md](../70-roadmap/active/overview.md)

## Explicitly Not Current

以下内容不得再当作生产实现说明：

- StagePacket / NextAction / graph dispatch 作为 orchestrator 主链
- application 持有 Start/Resume continuation queue 或 Decision resolve 后再 Resume
- 自动创建 禁止的 replan successor Run
- 以 `ExecutionContext` / patch 重建 authority
- 把 profile / software-dev 语义写进 kernel reducer
- Orchestrator、application、TUI/Web、projection 或 governance 直接指定 Run/Workspace/Result 状态；
  跨 owner lifecycle 只能由接收 owner 的内部 reducer 完成

历史追溯请使用 [90-archive](../90-archive/completed/overview.md)。
