# WorkOrch Current State

Status: active

Type: overview

Last Updated: 2026-07-16


## LLM Provider（2026-07-15）

- 首选 provider：内置 `ai-proxy`
- 模型发现：`blocks/llm` 读取 `/v1/models` catalog
- activation：提交/持久化 `llmTarget` / `ProviderModelTarget`
- workspace 默认 target：`blocks/workspace` 持久化可选 `DefaultLLMTarget`；新 Run 按 explicit activation → workspace → system `defaultChat` 解析
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

代码主链已收口。当前剩余主要是：

1. **运维切换（OPS-1）**：真实生产目录 `runtime-v1-retirement --apply` + clean build 维护窗口（过程日记已归档，不在 active 放长文）。
2. **可选验证（OPT-1）**：file-patch `apply + validation + DoD closeout` 的 deployed live 一次。
3. **延期候选**：LLM context budget 高级项见 `70-roadmap/candidates/`，**不占 active**。
4. **文档维护**：权威层术语抽查、个别 block readme 措辞、archive 索引（见 matrix D-*）。

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
