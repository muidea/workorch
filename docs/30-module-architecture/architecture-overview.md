# WorkOrch Architecture Overview

Status: active

Type: module-architecture

Last Updated: 2026-07-27

## Purpose

本文件提供 WorkOrch 当前稳定的模块架构总览。生产实现主链是 **Runtime only**；本文件不承担完整 runtime contract、全量 API 清单或 禁止的 迁移记录。

## Stable Layers

WorkOrch 按稳定责任划分为：

- application layer
- kernel layer
- block layer（stateful owner + capability execution）
- shared internal package layer
- web console layer

### Layer Roles

- **application layer**：进程入口、bootstrap、HTTP/CLI/TUI surface、appservice use-case 组合、workspace projection；不拥有正式业务状态
- **kernel layer**：Orchestrator / Governance / System 多 owner 协调 Modules
- **block layer**：
  - stateful owner：Workspace / Run / Result / Decision / Approval / Memory / Soul / Skill / MCP Registry / Eventstream
  - capability execution：LLM / File / Git / Shell / MCP / Web / SSH（只执行，不裁决）
- **shared internal package layer**：`workorchruntimecontract`、repository、config、EventHub helpers 等
- **web console layer**：基于 read model / projection 的产品展示与操作入口

## Top-Level Structure

核心代码落点：

- `application/workorch` — thin main
- `internal/initiators/workorchruntime` — repository provider 装配
- `internal/modules/application/workorchd` — appservice / server / bootstrap / projection
- `internal/modules/kernel` — orchestrator / governance / system
- `internal/modules/blocks` — workspace、run、result、decision、approval、memory、soul、skill、mcpregistry、eventstream 与能力 block
- `internal/pkg` — runtime contract、repo、config、ports
- `internal/services/workorch` — CLI / TUI
- `web/workorch-console` — Web Console

## Runtime Mainline

```text
Application: RuntimeBootstrap → Run create
Start / Resume intent (Run owner EventHub handlers)
  → Run owner-local lifecycle transition → RunStateChanged
  → DriveRuntime（唯一同步推进）
    → InitializeRuntimeFromRun（幂等 load/init）
    → reducer(RuntimeFacts) → RuntimeDecision + AuthorityTransaction
    → prerequisite | candidate → SealedDispatch
    → ExecuteRuntimeOperation（typed EventHub → capability blocks）
    → evidence / assessment / governance commit
    → governed_wait fact | typed terminal delivery fact / acknowledgement
```

双层语义：

- **层 A**：中立 stage / profile / output policy（意图与严格度，非生产状态名）
- **层 B**：Runtime focus / requiredOutput / candidate / sealed dispatch（实现状态机）

已删除、不得作为生产主链：

- `StagePacket` / `NextAction` / `DispatchNextStep`
- `ExecutionContextPatch` 与 application continuation queue
- 自动 replan successor Run
- 通用 RuntimeAdapter registry / Step kind registration matrix 作为生产调度

## Key Runtime Units

组件类型基线为 1 Initiator / 4 Modules / 17 Blocks。目录与职责类型一致；
`blocks/run` 与 `blocks/eventstream` 均为单一资源 Block owner。

### Application

| 类型 | 单元 | 职责 |
| --- | --- | --- |
| Entry | `application/workorch` | 主可执行入口（TUI / `--server`） |
| Initiator | `internal/initiators/workorchruntime` | owner repository provider |
| Module | `internal/modules/application/workorchd` | bootstrap、appservice、HTTP、projection、scheduler Start/Resume intent 提交 |

### Kernel

| 类型 | 单元 | 职责 |
| --- | --- | --- |
| Module | `kernel/orchestrator` | Runtime convergence authority：drive、reducer、sealed dispatch、evidence、governance wait、sealed terminal、delivery acknowledgement ledger |
| Module | `kernel/governance` | 自动治理评估 / 升级 / AutoGovernanceTrace 协调（不替代 Orchestrator contract gate） |
| Module | `kernel/system` | 默认 Soul / Template / MCP 等 ensure |

### Blocks

**Stateful owners**

- `blocks/workspace` — Workspace / Goal / DoD / Template
- `blocks/run` — Run / Step / Checkpoint / `RuntimeStateRecord` 持久化与 CAS
- `blocks/result` — Result / Artifact（formal outcome；不裁决 acceptance）
- `blocks/decision` / `blocks/approval` — 治理 ticket / 授权
- `blocks/memory` / `blocks/soul` / `blocks/skill` / `blocks/mcpregistry` — 记忆与能力注册
- `blocks/eventstream` — 审计事件持久化与 high-watermark 查询

**Capability execution（typed EventHub operation only）**

- `blocks/llm` — Runtime candidate 请求
- `blocks/file` / `git` / `shell` / `mcp` / `web` / `ssh` — sealed operation 执行

## Architecture Boundary

| 层 | 负责 | 不负责 |
| --- | --- | --- |
| Application | 入口、用例组合、projection、Start/Resume intent 提交 | owner 状态机、drive loop、Decision resolve 后 Resume |
| Orchestrator | 收敛权威、evidence quality、coverage、Result 裁决、sealed terminal/delivery ledger | provider 调用细节、领域 profile 硬编码、外部 owner 状态迁移 |
| Run owner | 持久化与 CAS、owner-local lifecycle transition | 编排、coverage 裁决 |
| Capability blocks | 同步执行并返回 typed result/evidence 摘要 | Run 终态、accepted Result、自动 retry 策略 |
| Shared packages | 中立 contract 与基础设施 | 业务状态机 |

## Key Rules

1. **business state transitions** 留在各自唯一 owner reducer：SM-2/3/4 在 Orchestrator，Run 在 Run owner，
   Workspace 在 Workspace owner，Result 在 Result owner；**side effects** 留在 capability blocks。
2. 跨组件交互只走 **EventHub + 具体 command/data/result**；禁止跨 owner service/repository、Envelope、通用 map payload。
3. product/UI 不得绕过 read model 直接定义业务状态。
4. template / Soul / profile 特化不得下沉到 kernel reducer。
5. block 输出只是 candidate 或 operation evidence；evidence quality、next action、accepted Result 只能由 Orchestrator 判定。
6. Start/Resume 生产入口只进入 Runtime；缺少 typed bootstrap 必须失败。

## Current Stable Decisions

- 仓库保持单 `go.mod`
- `application/workorch` 保持 thin main
- Runtime aggregate 经 Run owner EventHub CAS 持久化，不编码为 `map[string]any`
- Orchestrator plugin 不创建 RuntimeAdapter registry；operation 只走各组件 typed EventHub 合同
- Decision `runtime_governance` resolve 由 Decision owner 直接提交 Orchestrator resolution；application **不** 再发 Resume

## Source Documents

- `repository-structure.md`
- `kernel-orchestrator-governance.md`
- `application-workorchd.md`
- `blocks-runtime-adapters.md`
- `../00-core/runtime-stage-contract.md`
- `internal/modules/kernel/orchestrator/readme.md`
- `internal/modules/blocks/run/readme.md`
- `internal/modules/application/workorchd/readme.md`
