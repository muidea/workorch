# WorkOrch Current State

Status: active

Type: overview

Last Updated: 2026-07-27


## LLM Provider（2026-07-15）

- 首选 provider：内置 `ai-proxy`
- 模型发现：`blocks/llm` 读取 `/v1/models` catalog
- activation：提交/持久化 `llmTarget` / `ProviderModelTarget`
- workspace 默认 target：`blocks/workspace` 持久化可选 `DefaultLLMTarget`；新 Run 按 explicit activation → workspace → system `defaultChat` 解析
- TUI 运行控制统一为 `/run resume`；application 根据 Run 终态、Goal/DoD 定义及封存 provider/model 是否漂移，自动保留原 Run 或创建新的 Runtime V2 activation，TUI 不再要求用户区分 resume/restart
- TUI：`/model` 自动列出 ready chat `providerId/modelId`，双确认后更新当前 workspace；`/model show` 查看，`/model clear` 恢复继承系统默认
- Runtime V2：`ProviderCeiling` 封存 target + token/byte ceiling + revision
- 旧 `defaultProvider` / provider list / model pattern / provider context 路径已删除

## Purpose

本文件是仓库「现在怎样」的唯一事实入口。它只描述当前已落地的生产主链、文档权威分层与剩余未关项，不承载历史 MVP/P1 记录或已归档 roadmap 正文。

## Production Runtime Truth

**生产代码路径 = Runtime V2 only。**

> 实施状态限定：下列架构规则是当前目标合同，不表示 RUNTIME-SMO 已完成。2026-07-27 的实现复核已确认
> lifecycle generic 写入、sealed identity、administrative terminal、delivery CAS/fence 与 startup recovery
> 仍有缺口；唯一执行清单见
> [runtime-v2-single-owner-state-machine-closure-plan.md](../70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md) 第 12 节。

- Start / Resume 只提交到 Run owner 的 typed activation / continuation intent；Run owner 成功迁移后发布事实，
  Orchestrator 才进入 Runtime V2 drive。缺少 typed bootstrap 必须按 owner-local terminal delivery 收敛。
- 唯一同步推进合同：`DriveRuntimeV2Command/Result`（Orchestrator）。
- 正式运行时状态：`RuntimeStateRecord` / Runtime V2 aggregate（Run owner CAS 持久化）。
- `runtimeContractVersion=v2` 仅为审计字段，不是 V1/V2 路由开关。
- V1 主链已删除：`StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、自动 replan successor、V1 progress/terminal checkpoint replay、通用 RuntimeAdapter registry。
- 完整生产规约由四个协调状态机组成：SM-1 Run lifecycle、SM-2 authority/focus convergence、SM-3 candidate/sealed dispatch transaction、SM-4 drive/recovery/projection delivery；任何 runtime 变更必须通过四状态机变更门禁。
- complete candidate 只经过一个 evaluator；rejection 只能是同 manifest 内可修正一次或明确终态。candidate attempt 预算按稳定 authority contract identity 计算，不与 request recovery、prerequisite refresh 或 focus repair 共用。
- drive 同时要求 persisted revision 与 convergence progress signature 推进；仅增加 retry/snapshot bookkeeping 不算正向进展。
- terminal delivery 统一为 persisted terminal → Result owner → Workspace owner → Run owner；SM-4 只保存
  delivery acknowledgement 并对缺失回执执行幂等重试。
- projection readiness 通过 EventStream typed high-watermark 查询核对真实 source revision；stale/degraded projection 不再通过 `/readyz`。

一句话架构真值：

> Application 只构造 typed `RuntimeBootstrap` 并提交 Run activation/resume intent；Run owner持久化 lifecycle 与
> Runtime V2 aggregate；Orchestrator 是唯一 reducer / drive / evidence / governance-resolution 权威；capability
> blocks 只执行并回执；Result/Workspace/Run 各自只迁移自己的终态资源；Eventstream / projection 只审计与展示；
> 恢复 = 从 aggregate 幂等再 drive。

关键代码落点：

- 实现 SoT：`internal/modules/kernel/orchestrator/readme.md`
- 类型合同：`internal/pkg/workorchruntimecontract`（仅 DTO、合同与纯无状态 helper）
- 主链实现：`internal/modules/kernel/orchestrator/runtime_v2_*.go`、`biz/runtimev2/`

## Dual-Layer Contract Model

正式文档采用 **双层并存**：

| 层 | 语言 | 用途 | 是否生产状态机 |
| --- | --- | --- | --- |
| A. 语义 / profile | 中立 stage：`exploration` / `planning` / `execution` / `verification` / `acceptance` / `repair` / `replan` | output policy、profile 映射、LLM 意图、evidence 严格度 | 否 |
| B. Runtime V2 实现 | bootstrap → facts → reducer → prerequisite / candidate → sealed dispatch → evidence → terminal | 生产调度与收敛 | 是 |

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
| Module | `kernel/orchestrator` | Runtime V2 convergence authority |
| Module | `kernel/governance` | 跨 Decision/Approval/Run 的治理协调；不拥有 Runtime continuation |
| Module | `kernel/system` | 跨 owner 的启动默认资源 ensure 协调 |
| Block | `blocks/run` | Run / Step / V2 aggregate 持久化 owner；不做 action 选择 |
| Block | `blocks/eventstream` | 审计 EventRecord/high-watermark owner；不是业务修复入口 |
| Block | `blocks/workspace` | Goal / DoD / Template 定义 owner |
| Block | `blocks/result` | 正式 outcome 存储；不裁决 acceptance |
| Block | `blocks/decision` / `approval` | 治理 ticket / 授权 owner |
| Block | `blocks/memory` / `skill` / `soul` / `mcpregistry` | 单一正式资源 registry/aggregate owner |
| Block | `blocks/llm` / `file` / `git` / `shell` / `mcp` / `ssh` / `web` | 单一技术能力；返回 candidate、observation 或 receipt |

跨组件交互：EventHub + 具体 command/data/result。禁止跨 owner service/repo、Envelope、`map[string]any` bridge。

## Remaining Open Work

历史 fresh Run 的局部成功和 R4/P0～P6/A-4 过程分段只保留为 archive regression evidence，不再定义当前实施顺序，也不能证明当前 live 链路可持续推进。最新反证表明，owner fact、manifest、provider schema、decoder、admission、feasibility 与 correction 仍在多处重复解释同一授权；成功路径已 typed，部分拒绝路径却缺少有界 allowed set 或可执行 directive，导致 live 中持续暴露相邻合同违例。

1. **Runtime V2 合同归一（RUNTIME-VPC）**：R5 使每个 authority transaction 只有一份 persisted `RuntimeCandidateContractManifest`、每个 complete candidate 只经过一个 evaluator，并将 pipeline 结果限定为 `request_recoverable`、`candidate_correctable`、`candidate_rejected_final`、`prerequisite_stale`、`accepted`。N1～N8 代码/本地门禁已完成（`SealedOperationAuthorization` + 三种 `TargetPolicy` + owner fact/admissibility 编译、独立预算类型、manifest-only schema/C3/C4 renderer 与 strict decoder、complete rejection 的 `correctable_once`/`rejected_final` 矩阵与 `runtime_contract_defect` 替代 `candidate_correction_undefined`、owner feasibility 统一 outer observation、五类结果 persistence/recovery 门禁、legacy 清零，以及 selector-only MCP inventory 不强制 `targetRefs`）。剩余 real-provider 双 activation 与三态 deployed restart 为外部 fixture observation residual。不通过放宽业务合同、调高固定预算、增加 prompt 补丁、UI Resume 或 live 热修换取推进。详见 [VPC candidate/receipt 收口方案](../70-roadmap/active/runtime-v2-validation-progress-closure-plan.md)。

2. **Runtime V2 单 owner 状态机（RUNTIME-SMO）**：最新核对发现 Orchestrator 仍直接准入或投影
   Run/Workspace/Result lifecycle，违反唯一 owner。必须迁为 Run、Workspace、Result 各自的 owner-local reducer；
   跨 owner 只传 activation/governance/terminal immutable fact，SM-4 仅保持 delivery acknowledgement。
   此项尚未实施，详见 [单 owner 收口方案](../70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md)。
3. **组件边界 / 生命周期（ARCH-1）**：SM-4 startup recovery、channel-free process lifecycle、owner Service facade、repository DTO 与 test-data isolation 均已有实现/回归证据；2026-07-19 的 go-multi-module-dev 审计发现的共享 Base Biz 缺失和 framework Module 根直接持有 EventHub，现已迁移到 component-local Biz，且本地 structural/race/file-backed 门禁已重跑。22 个组件的职责、typed 交互合同、禁止边界和可单独执行的回归命令已固化在 `70-roadmap/active/component-contract-and-regression-closure-matrix.md`。仅待提供重启后的外部目标与 `active_no_dispatch`、`active_sealed_dispatch`、`terminal_delivery_pending` 三类 fixture manifest，以执行 deployed SM-4 matrix；workspace 删除后的迟到投影还必须验证 `/readyz` 恢复 ready，不能以放宽 readiness 绕过。
4. **运维切换（OPS-1）**：真实生产目录 `runtime-v1-retirement --apply` + clean build 维护窗口（过程日记已归档，不在 active 放长文）。
5. **可选验证（OPT-1）**：file-patch `apply + validation + DoD closeout` 的 deployed live 一次。
6. **延期候选**：LLM context budget 高级项见 `70-roadmap/candidates/`，**不占 active**。
7. **文档维护**：权威层术语抽查、个别 block readme 措辞、archive 索引（见 matrix）。

2026-07-20 RCA-1 已由 `1ca44996` 完成：candidate admission authority 位于
Orchestrator，full/partial replay、attempt 0/1、terminal closure 与 freshness priority
均使用 typed CAS transition；真实 provider fresh Run 和隔离 deployed partial-replay Run
均无 pending attempt、无 internal misalignment、无人工 Resume。完成记录见
[runtime-v2-candidate-admission-transaction-closure.md](../90-archive/completed/runtime-v2-candidate-admission-transaction-closure.md)。

2026-07-16 已将 active 中假活跃产品/审计日记、dated live 记录与 `60-verification/artifacts` 清理出当前入口；检索见 [90-archive](../90-archive/completed/overview.md)。

统一清单：

- [current-unclosed-matrix.md](current-unclosed-matrix.md)
- [unfinished-todo-index.md](unfinished-todo-index.md)
- [70-roadmap/active/overview.md](../70-roadmap/active/overview.md)

## Explicitly Not Current

以下内容不得再当作生产实现说明：

- StagePacket / NextAction / graph dispatch 作为 orchestrator 主链
- application 持有 Start/Resume continuation queue 或 Decision resolve 后再 Resume
- 自动创建 V1 replan successor Run
- 以 `ExecutionContext` / patch 重建 authority
- 把 profile / software-dev 语义写进 kernel reducer
- Orchestrator、application、TUI/Web、projection 或 governance 直接指定 Run/Workspace/Result 状态；
  跨 owner lifecycle 只能由接收 owner 的内部 reducer 完成

历史追溯请使用 [90-archive](../90-archive/completed/overview.md)。
