# WorkOrch Current State

Status: active

Type: overview

Last Updated: 2026-07-21


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

- Start / Resume 只进入 Runtime V2 drive；缺少 typed bootstrap 必须失败并投影 typed terminal。
- 唯一同步推进合同：`DriveRuntimeV2Command/Result`（Orchestrator）。
- 正式运行时状态：`RuntimeStateRecord` / Runtime V2 aggregate（Run owner CAS 持久化）。
- `runtimeContractVersion=v2` 仅为审计字段，不是 V1/V2 路由开关。
- V1 主链已删除：`StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、自动 replan successor、V1 progress/terminal checkpoint replay、通用 RuntimeAdapter registry。
- 完整生产规约由四个协调状态机组成：SM-1 Run lifecycle、SM-2 authority/focus convergence、SM-3 candidate/sealed dispatch transaction、SM-4 drive/recovery/projection delivery；任何 runtime 变更必须通过四状态机变更门禁。
- candidate attempt 预算按稳定 authority contract identity 计算，不按包含 reducer attempt 的 snapshot ID 计算；历史 reseal runaway 会正式收口为 `candidate_retry_exhausted`。
- drive 同时要求 persisted revision 与 convergence progress signature 推进；仅增加 retry/snapshot bookkeeping 不算正向进展。
- terminal delivery 统一为 persisted terminal → Result → Run/Workspace，并对派生交付失败执行幂等重试。
- projection readiness 通过 EventStream typed high-watermark 查询核对真实 source revision；stale/degraded projection 不再通过 `/readyz`。

一句话架构真值：

> Application 只构造 typed `RuntimeBootstrap` 并触发 Start/Resume；Run owner 持久化 Runtime V2 aggregate；Orchestrator 是唯一 reducer / drive / evidence / governance-resolution 权威；capability blocks 只执行并回执；Result 只记录已裁决 outcome；Eventstream / projection 只审计与展示；恢复 = 从 aggregate 幂等再 drive。

关键代码落点：

- 实现 SoT：`internal/modules/kernel/orchestrator/readme.md`
- 类型合同：`internal/pkg/workorchruntimecontract`
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
| 2 | 当前事实入口 | 本文 |
| 3 | 实现落点 SoT | `internal/modules/**/readme.md` |
| 4 | 类型级真相 | `internal/pkg/workorchruntimecontract` |
| 5 | 主链叙述 | [20-runtime-flow](../20-runtime-flow/overview.md)、[30-module-architecture](../30-module-architecture/overview.md) |
| 6 | 未关项 | [70-roadmap/active/overview.md](../70-roadmap/active/overview.md) |
| 7 | 历史 | [90-archive](../90-archive/completed/overview.md) |

冲突规则：实现细节以模块 readme + contract 包为准；中立约束以 `00-core` 为准；已归档 plan 不得并列约束新实现。

## Ownership Snapshot

| 组件 | 职责 |
| --- | --- |
| `application/workorchd` | HTTP/CLI/SSE/projection；构造 bootstrap；不持有 drive queue |
| `kernel/orchestrator` | Runtime V2 convergence authority |
| `kernel/run` | Run / Step / V2 aggregate 持久化 owner；不做 action 选择 |
| `blocks/workspace` | Goal / DoD / Template 定义 owner |
| `blocks/result` | 正式 outcome 存储；不裁决 acceptance |
| `blocks/decision` / `approval` | 治理 ticket / 授权 |
| capability blocks（llm/file/git/shell/…） | 同步执行；返回 candidate 或 operation evidence |
| `kernel/eventstream` | 审计 EventRecord；不是业务修复入口 |

跨组件交互：EventHub + 具体 command/data/result。禁止跨 owner service/repo、Envelope、`map[string]any` bridge。

## Remaining Open Work

Runtime V2-only 主链骨架、RVC-R5～R15 foundation、RVC-R17～R26 与 RPA-1 首次 inventory/selector/feasibility 切片已完成。2026-07-18 的 deployed rerun 已将 capability-state canonical owner 与 discriminated operation variant 进入 sealed authority；随后发现的 `requiredOperationRef` provider-schema/strict-DTO 字段漂移也已收口为 typed mapping 和 file-gate 回归。2026-07-20 的 RPF-1～5 已通过 owner-local、file-backed restart/crash、CI-safe 和当时的 fresh deployed bounded acceptance；证据见[归档 completion record](../90-archive/completed/runtime-v2-prerequisite-freshness-closure.md)。

2026-07-21 fresh deployed `ws-fa686f29f54bc493/run-003` 提供了新的反证：provider target、sealed transaction、stable prefix、terminal delivery 与 projection 均正常，但 all-skipped validation 被标为 valid，任意 current-focus accepted evidence 又被当作 effective progress，导致 validation-only counter/active gap 反复清零，最终在 `repairAttempts=10/10` 进入 `no_progress_exhausted`。因此历史 completion record 不能再证明 validation-only same-focus convergence 已关闭。当前剩余主要是：

1. **Validation / effective progress（RUNTIME-VPC）**：逐项收口 Shell all-skipped quality、Orchestrator evidence admission 与 effective progress 分离、accepted-but-no-progress shape、validation-only escalation、owner-grounded discovery、async terminal/repair metrics；详见 [详细方案](../70-roadmap/active/runtime-v2-validation-progress-closure-plan.md)。
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

历史追溯请使用 [90-archive](../90-archive/completed/overview.md)。
