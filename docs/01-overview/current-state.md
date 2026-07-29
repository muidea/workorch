# WorkOrch Current State

Status: active

Type: overview

Last Updated: 2026-07-29


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

> 实施状态限定：最新 fresh deployed live 已证明 activation、receipt、sealed dispatch 与 terminal delivery
> 的局部基线，但暴露可修复 implementation failure 缺少 Shell → File → Orchestrator 的 owner-grounded
> repair-authority handoff，因而错误进入 validation-only loop。SMO-7A 已完成代码、targeted regression 与
> static gate；当前唯一实施项是 RUNTIME-SMO SMO-7 fresh live。历史 C7/C8
> 只作回归基线，不能据此声明 workspace 已持续收敛：
> [runtime-v2-single-owner-state-machine-closure-plan.md](../70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md)。

- Start / Resume 只提交到 Run owner 的 typed activation / continuation intent；Run owner 成功迁移后发布事实，
  Orchestrator 才进入 Runtime V2 drive。缺少 typed bootstrap 必须按 owner-local terminal delivery 收敛。
- 唯一同步推进合同：`DriveRuntimeV2Command/Result`（Orchestrator）。
- 正式运行时状态：`RuntimeStateRecord` / Runtime V2 aggregate（Run owner CAS 持久化）。
- `runtimeContractVersion=v2` 仅为审计字段，不是 V1/V2 路由开关。
- V1 主链已删除：`StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、自动 replan successor、V1 progress/terminal checkpoint replay、通用 RuntimeAdapter registry。
- 完整生产规约由四个协调状态机组成：SM-1 Run lifecycle、SM-2 authority/focus convergence、SM-3 candidate/sealed dispatch transaction、SM-4 drive/recovery/projection delivery；任何 runtime 变更必须通过四状态机变更门禁。
- complete candidate 只经过一个 evaluator；其 structural rejection 才能是同 manifest 内可修正一次或明确终态。owner feasibility 是独立事实，必须经 Orchestrator internal event 进入 SM-2 后继，绝不消耗 candidate attempt。各预算按稳定 authority contract identity 分离。
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

历史 fresh Run 的局部成功、R4/P0～P6/A-4 过程分段和旧 VPC 的 N1～N8 只保留为 archive regression
evidence，不再定义当前实施顺序，也不能代替最新 deployed live。最新 persisted aggregate 反例优先决定当前未关项；
active 文档不保存 workspace/run 的 dated 过程日志或 raw provider/runtime data。

1. **Runtime V2 单 owner convergence 收口（RUNTIME-SMO SMO-7）**：candidate rejection 与
   owner feasibility 的正交合同、Orchestrator internal event、唯一 aggregate coordinator、activation-time
   coverage/blueprint、capability code taxonomy 与 request boundary均为已完成基线。当前先收口可修复
   validation failure 的 Shell fact → File repair-surface confirmation → Orchestrator internal event → new
   authority attempt-0 已完成代码收口；当前执行 restart/concurrency/fresh live。历史 C7/C8、canonical target、receipt/effect
   等只作为回归基线，不能新增兼容路径或独立规则。详见
   [单 owner 统一收口方案](../70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md)。
2. **组件边界 / 生命周期（ARCH-1）**：SM-4 startup recovery、channel-free process lifecycle、owner Service facade、repository DTO 与 test-data isolation 均已有实现/回归证据；2026-07-19 的 go-multi-module-dev 审计发现的共享 Base Biz 缺失和 framework Module 根直接持有 EventHub，现已迁移到 component-local Biz，且本地 structural/race/file-backed 门禁已重跑。22 个组件的职责、typed 交互合同、禁止边界和可单独执行的回归命令已固化在 `70-roadmap/active/component-contract-and-regression-closure-matrix.md`。仅待提供重启后的外部目标与 `active_no_dispatch`、`active_sealed_dispatch`、`terminal_delivery_pending` 三类 fixture manifest，以执行 deployed SM-4 matrix；workspace 删除后的迟到投影还必须验证 `/readyz` 恢复 ready，不能以放宽 readiness 绕过。
3. **运维切换（OPS-1）**：真实生产目录 `runtime-v1-retirement --apply` + clean build 维护窗口（过程日记已归档，不在 active 放长文）。
4. **可选验证（OPT-1）**：file-patch `apply + validation + DoD closeout` 的 deployed live 一次。
5. **延期候选**：LLM context budget 高级项见 `70-roadmap/candidates/`，**不占 active**。
6. **文档维护**：权威层术语抽查、个别 block readme 措辞、archive 索引（见 matrix）。

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
