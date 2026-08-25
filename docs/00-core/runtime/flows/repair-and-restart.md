# Repair And Restart Flow

Status: normative

Type: runtime-flow

Last Verified: 2026-08-25

## Purpose

本文件定义 Runtime 在失败、governance wait、candidate retry 与“需要重新定义”时的恢复语义。

稳定三分：

| 语义 | Runtime 含义 |
| --- | --- |
| **repair** | **同一 Run、同一 focus** 上重新收敛：prerequisite 补齐、candidate attempt 1、operation evidence、governance continue |
| **recoverable runtime wait** | **同一 Run、同一 aggregate** 等待typed recovery condition/trigger；必须有deadline、budget和persisted re-drive successor，不需要人类选择 |
| **restart/replacement** | **不是**同 Run 内自动 successor；需要 fresh baseline 时由显式 Workspace `restart` 提交 fresh bootstrap，并由 SM-1 唯一 Run owner 的 subordinate `run.replacement` transaction 创建独立 Run |

生产主线**不**存在自动 replan successor lifecycle、`ExecutionContextPatch` resume 或 graph 重放。

## State Machine Binding

本 flow 绑定核心规约中的：

- SM-2：same-focus repair、failure signature budget、typed external dependency governance 或 terminal；
- SM-3：same-transaction candidate retry 与 rejected operation evidence；
- SM-1：blocked/active/terminal lifecycle，以及同 owner subordinate replacement transaction 和显式新 Run lineage；
- SM-4：restart/load、governance continuation、administrative delivery handoff 与 bounded no-progress guard；不迁移 Run。

repair 与 restart/replacement 的边界不得由 application queue、自动 successor 或 projection watcher实现。完整规约见
[Runtime State Machines](../state-machines.md)。

## Flow

```text
failure / missing prerequisite / complete candidate rejection / operation failure / repairable assessment / typed governance dependency
  -> producer factual result becomes Orchestrator internal event
  -> reducer updates typed facts / diagnostics / prerequisites / canonical ConvergenceState
  -> same-run reconverge:
       need_prerequisite
       | complete candidate rejection -> persisted diagnostic -> at most one correction (same transaction + stable prefix)
       | incomplete/transport request  -> request-session bounded recovery, no candidate attempt
       | repairable implementation gap -> need_state_change
       | repairable evidence gap       -> need_validation
       | need_assessment -> partial/satisfied/repairable/governance/terminal disposition
       | need_governance -> governed_wait
            OpenRuntimeGovernanceRequest
            ... external Decision / Approval ...
            CommitRuntimeGovernanceResolution (once per resolution ID + snapshot)
            accepted governance evidence -> same reducer
            governance fail -> governance_rejected terminal
       | typed transient dependency / consumption unknown -> recoverable_runtime_wait
            persisted condition + trigger/deadline + budget
            condition met -> same aggregate re-drive
            deadline/budget/guard exhausted -> typed terminal
  -> if path cannot continue within Runtime invariants or repair budget exhausted:
       structured Terminal Fail  (operator may later submit an explicit Workspace restart)
```

## Main Stages

### 1. Repair = Same Focus Reconverge

repair 在 Runtime 中不是独立 stage 对象，而是 **drive/reducer 在当前 authority 与 focus 上的再推进**：

- 缺失 manifest / target snapshot / capability / validation scratch / provider ceiling
  → `need_prerequisite`（确定性，非 LLM）
- complete candidate schema / contract rejection
  → SM-3 先持久化完整 diagnostic，再决定 typed terminal 或同一 authority 上唯一 correction（复用 transaction + stable prefix）
- incomplete document / provider transport failure
  → request-session 有界 continuation/recovery，不创建 candidate attempt
- operation 失败 evidence
  → typed operation feedback 进入 internal event；reducer 决定下一 canonical state
- assessment 返回 repairable gap
  → 结构化 gap 进入 facts；按 missing evidence kind 回 state change / validation，不能直接 terminal
- 需要外部信息 / 审批 / 依赖
  → sealed governance candidate → ticket → resolution commit
- 存在owner-defined typed recovery/reconciliation合同的暂时不可用或consumption unknown
  → persisted recoverable wait；只由matching trigger/deadline进入SM-4 re-drive

不得：

- 广泛重新规划为新的 禁止的 graph
- 第三次 candidate 请求
- accepted 后无 dispatch 重试
- 用 wrapper / schema 外壳覆盖原 failure surface 的权威身份（diagnostic 可记录，但 authority 仍由 facts 决定）
- 把unknown/unavailable一律重试、把普通Resume/UI refresh当作wait trigger，或留下无deadline/后继的active Run

### 2. Candidate Attempt Budget

- attempt 0 与 attempt 1 状态随 aggregate 持久化
- attempt 1 必须复用相同 `AuthorityTransaction` 与 `StablePrefixHash`
- 第二次 complete rejection 后生成 structured candidate exhaustion fact，由 SM-3 形成 typed candidate terminal；不得发起第三次 same-contract candidate。governance 只能来自独立的 typed external dependency
- 进程重启不增加 attempt；只加载已有 `CandidateAttempts`

### 3. Governance-Mediated Recovery

governed dispatch：

1. `OpenRuntimeGovernanceRequestCommand/Result` 向 Decision owner 创建确定性 ticket
2. ticket、snapshot、dispatch、dependency 绑定随 aggregate 持久化
3. Orchestrator 持久化 `governed_wait` 事实；Run owner 消费后在自身 reducer 中进入 `blocked_on_decision`
4. resolution 通过 `CommitRuntimeGovernanceResolutionCommand/Result` 提交
5. 同一 resolution ID 与 authority snapshot **最多消费一次**
6. Decision owner 也可只提交 ticket resolution；Orchestrator 从 persisted binding 补齐后进入同一 commit

规则：

- governance continue：写入 accepted governance evidence、递增 evidence revision、回到同一 reducer
- governance fail：生成 `governance_rejected` terminal
- stale resolution：只进入 diagnostic lineage
- 必须匹配已绑定 ticket；无绑定、同 dispatch 绑定不同 ticket、dependency 漂移 → conflict
- `runtime_governance` resolution committed 后 Orchestrator 发布 immutable continuation/terminal fact；Run owner内部迁移后通知 drive；application resolved watcher **不得**再次发送 Resume 形成双驱动

### 4. Replacement Semantic = Explicit Workspace Restart

Runtime/Orchestrator **不自动创建** replan successor Run；HTTP/TUI 也不提供 `intent=replan`。

```text
operator
  -> Workspace execution intent=restart + fresh RuntimeBootstrap
  -> Workspace owner validates current definition/scope
  -> Run owner persists predecessor ReplacementIntent and claims termination intent
  -> Orchestrator SM-4 persists RuntimeTerminationRecord delivery handoff
  -> Result owner ack -> Workspace owner releases active slot -> Run owner private SM-1 terminal event
  -> Run-private reducer creates/links/activates one successor
  -> successor independently enters DriveRuntime
```

约束：

- application、Workspace、Orchestrator 和 projection 都不能提前创建或链接 successor；
- predecessor 的 active focus、stale snapshot、attempt/receipt/evidence 不复制为 successor authority；
- fresh bootstrap 可以携带有界 definition-change/governance history，但其内容是显式 typed contract；
- `Resume` 不能通过 patch 或 loaded Run 改写定义；
- 无法按当前结构读取或校验的 predecessor 在 repository/owner load 边界失败，不进入 Restart admission，也不持久化 replacement intent；
- 相同 request/intent/terminal delivery 重放只能得到同一 successor。

### 5. Restart Recovery

- 服务重启只加载 Run owner 中的 `RuntimeStateRecord` aggregate
- 不记录 / 不恢复 禁止的 `runtime_step_result_available`、watchdog recovery policy 等 checkpoint
- 不自动重放 sealed 但未完成的进程内 executor 假象；推进只经 `DriveRuntime` 与 typed evidence/resolution
- 重复 Drive 必须幂等；无持久化进展 fail-fast

### 6. What Is Not On This Path

- 禁止的 recovery action 矩阵作为第二套状态机（`retry_same_step` 自动建 Step 等）
- `dispatched` Step 进程内 executor 重启自动重派
- application continuation queue / patch resume facade
- auto-governance 静默创建 successor Run
- context gate 伪装成普通 state-changing repair 但绕过 prerequisite / candidate seal

Context 前置失败（缺 snapshot、缺 capability 等）应优先进入 `need_prerequisite`；只有独立 typed external
dependency 才可进入 `need_governance`，
而不是伪造 state-changing candidate。

## Key Flow Guarantees

- repair 继承当前 definition revision、focus 与 authority 约束
- failure / diagnostic 分层：stale 只进 diagnostic lineage
- fresh replacement = 显式 Workspace restart + SM-1 sole-owner subordinate Run transaction，不是第五个状态机或
  Orchestrator/application 自动 successor
- governance resolution 单次消费并回到同一 reducer
- Start/Resume 生产入口只向 Run owner 发送 intent；缺少 bootstrap 通过 owner-local terminal delivery 收敛

## Output

repair / recovery 的正式输出可以是：

- 同一 Run 上继续的 drive transition（prerequisite / candidate / evidence / governance wait）
- typed terminal Fail（含 governance_rejected）
- 显式 Workspace restart 后由 Run-private reducer创建的独立 successor Run

## Source Documents

- [overview.md](overview.md)
- [Activation And Definition](../../definition-convergence/activation-flow.md)
- [Planning And Execution](planning-execution.md)
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`（`runtime_governance.go`、`runtime_continuation.go`）
