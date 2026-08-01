# Kernel Orchestrator / Governance Architecture

Status: active

Type: module-architecture

Last Updated: 2026-07-29

## Purpose

本文件说明 `kernel/orchestrator` 与 `kernel/governance` 在 Runtime 下的代码责任与边界。

**Orchestrator = Runtime convergence authority**，不是 禁止的 Step dispatch 器。生产主链以 `internal/modules/kernel/orchestrator/readme.md` 与 `internal/pkg/workorchruntimecontract` 为权威。

## Main Modules

### `kernel/orchestrator`

职责（Runtime）：

- typed facts、authority transaction、prerequisite、candidate、sealed dispatch
- operation evidence、assessment ledger、governance wait
- sealed terminal、terminal delivery fact 与 SM-4 acknowledgement ledger（不写 Run / Workspace / Result state）
- `DriveRuntimeCommand/Result` 唯一同步推进合同
- 消费 Run owner notification 触发同一 drive；不准入 Start/Resume lifecycle

**不是：**

- 禁止的 `StartRun` / `DispatchNextStep` / patch resume facade
- 通用 RuntimeAdapter registry / Step kind registration matrix
- `NextActionForRun`、自动 replan successor、进程内 policy cache 双权威
- application continuation queue 的执行者

实现落点：

| 路径 | 内容 |
| --- | --- |
| `internal/modules/kernel/orchestrator/biz/runtime/` | controller、reducer、prerequisite、dispatch commit、executor、governance |
| `internal/modules/kernel/orchestrator/pkg/events/` | drive / operation / candidate / governance typed 合同 |
| `runtime_*.go`（service 层） | drive 入口、aggregate CAS、operation executor、terminal delivery ledger |

#### Drive 闭环

```text
Load Run aggregate（CAS via Run owner EventHub）
InitializeRuntimeFromRun（已有 aggregate 只 load，不重复 reduce）
for transition < guard:
  reducer(RuntimeFacts) → RuntimeDecision + AuthorityTransaction
  ConvergenceState:
    need_prerequisite  → typed prerequisite + commit + reduce
    need_state_change | need_validation | need_assessment | need_governance
                          → candidate (attempt 0, 同 transaction 可 1)
                          → SealDispatch → ExecuteRuntimeOperation
                          → evidence / assessment / governance open
    complete | failed     → typed terminal outcome / delivery acknowledgement
  governed_wait → governance wait fact；Run owner-local blocked transition
  非等待非终态 transition 必须递增 StateRevision
```

#### 跨组件合同

- **读**：EventHub 读取 `blocks/workspace`、`blocks/run` 权威快照；不直接持有其他 owner service/repository
- **写 Run aggregate**：Run owner update + expected revision CAS
- **capability 执行**：`ExecuteRuntimeOperationCommand/Result` → File / Git / Shell / MCP / Web / SSH
- **LLM candidate**：`RequestRuntimeCandidateCommand/Result`；请求携带 typed `RuntimeCandidateContext`，不得从 禁止的 map 重建
- **Decision / Approval / Memory**：各 owner 具体 command；Decision/Approval evidence 由各自 `pkg/events` 定义，Orchestrator 幂等写 EvidenceRecord；Result/Workspace/Run 只消费 terminal delivery fact 并回执
- **governance**：`OpenRuntimeGovernanceRequestCommand` 创建 ticket；`CommitRuntimeGovernanceResolutionCommand` / ticket-resolution 提交；同一 resolution ID + authority snapshot 最多消费一次
- **prerequisite**：target snapshot 等按 requirement owner 调用 EventHub；多 capability 结果合并，不得互相覆盖

#### Start / Resume

| 入口 | 行为 |
| --- | --- |
| Start | Run owner接受 activation intent，内部 `pending→active` 后通知 Orchestrator drive |
| Resume | Run owner接受仅 workspace/run scope 的 continuation intent，校验 blocker 后内部恢复并通知 Orchestrator drive |

约束：

- 无 execution-context patch、无 loaded Run 重放、无 application continuation queue
- `runtime_governance` resolution committed 后 Orchestrator 发布 continuation/terminal fact；Run owner内部恢复或终态；**application resolved watcher 不得再发 Resume**
- 服务重启只加载 persisted aggregate；重复 Start/Resume 不得重复 reduce 或增加 attempt

### `kernel/governance`

职责：

- 组合 Decision / Approval / Run / Workspace / Memory / Orchestrator 合同做自动治理评估、自动解决、升级、阻塞
- 管理 AutoGovernanceTrace（协调过程，不替代正式 owner 状态）

边界：

- 生产依赖仅自身 repository + EventHub
- 不得绕过 Orchestrator 的 evidence / coverage / requiredOutput / runtime contract gate
- 不替代 `blocks/run` 生命周期、`blocks/result` closeout、产品交互面

### `blocks/decision` / `blocks/approval`（治理相关 owner）

| 模块 | 职责 | 不负责 |
| --- | --- | --- |
| `blocks/decision` | DecisionTicket / Resolution 生命周期；`runtime_governance` 创建后经 typed ticket-resolution 提交 Orchestrator | 不修改 Run/Result 收敛状态；不猜 authority snapshot |
| `blocks/approval` | pending / approved / rejected 正式状态与 scope 关联 | 不执行副作用；不裁决 accepted business outcome |

## Authority Objects

| 对象 | 权威 |
| --- | --- |
| `RuntimeBootstrap` / Definition / Activation | 创建时封存；后续不得 map 重建 |
| `RuntimeFacts` + focus queue + evidence ledger | Orchestrator aggregate |
| `RuntimeDecision` / `AuthorityTransaction` | reducer 密封 |
| `RequiredOutputContract` | focus 与 capability 中立交集 |
| `SealedDispatch` | 执行前持久化；execution evidence 唯一合法入口 |
| `RuntimeStateRecord` | Run owner CAS；正式 Runtime 状态 |
| Result accepted/rejected | Result owner消费 Orchestrator sealed terminal delivery fact 后内部记录 |
| Run / Workspace terminal / blocker | 对应 owner消费 immutable governance/terminal fact 后内部迁移 |

## Dual-Layer Stage Semantic

- **层 A（语义）**：中立 stage / output policy / profile 映射 — 用于 LLM 意图与严格度，**不是** Orchestrator 状态机状态
- **层 B（实现）**：Runtime focus / requiredOutput / candidate / sealed dispatch — 生产决策以 canonical `ConvergenceState` 为准，action/output mode 仅为派生投影

profile-specific accepted/rejected kinds 只能作为 typed focus requirement 输入；reducer 不识别 profile ID、deliverable ID 或业务术语。

## Boundary

Orchestrator / Governance 负责如何治理与收敛，不直接负责：

- provider-side request rendering（`blocks/llm`）
- MCP 协议调用 / shell / file / git / web / ssh 外部动作细节
- product queue / drawer / form 交互

## Responsibility Split

| 关注点 | 主要责任 |
| --- | --- |
| drive / reducer / sealed dispatch / evidence / sealed terminal / delivery ledger | `kernel/orchestrator` |
| RuntimeStateRecord 持久化 CAS | `blocks/run` |
| decision ticket / resolution 本体 | `blocks/decision` |
| approval 正式状态 | `blocks/approval` |
| auto-governance 协调 / trace | `kernel/governance` |
| formal Result / Artifact 持久化与 result lifecycle | `blocks/result`（不裁决 acceptance） |
| capability operation 执行 | 各 capability block |

## Explicit Non-Goals（已删除 禁止的）

- `StagePacket`、`NextAction`、RuntimeStep Accepted/Progress/Completed/Failed 通用事件结构
- `DispatchNextStep`、`EnableEventDrivenDispatch`、通用 runtime executor
- `ExecutionContextPatch` resume、approval patch resume、decision resume subsystem
- 自动 replan successor；重定义目标必须新 typed activation/bootstrap
- ValidateRun/CompleteRun command；completed 只能来自 reducer + typed owner projection
- RuntimeAdapter registry 作为生产调度主链

## Source Documents

- `architecture-overview.md`
- `../00-core/runtime-stage-contract.md`
- `internal/modules/kernel/orchestrator/readme.md`
- `internal/modules/blocks/run/readme.md`
- `internal/modules/kernel/governance/readme.md`
