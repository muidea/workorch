# Repair And Replan Flow

Status: active

Type: runtime-flow

Last Updated: 2026-07-27

## Purpose

本文件定义 Runtime V2 在失败、governance wait、candidate retry 与“需要重新定义”时的恢复语义。

稳定二分：

| 语义 | V2 含义 |
| --- | --- |
| **repair** | **同一 Run、同一 focus** 上重新收敛：prerequisite 补齐、candidate attempt 1、operation evidence、governance continue |
| **replan** | **不是**同 Run 内自动 successor；重新定义目标或执行基准时，只能通过**新的** typed activation/bootstrap 创建**独立** V2 Run |

生产主线**不**存在 V1 自动 replan successor lifecycle、`ExecutionContextPatch` resume 或 graph 重放。

## State Machine Binding

本 flow 绑定核心规约中的：

- SM-2：same-focus repair、failure signature budget、governance/terminal；
- SM-3：same-transaction candidate retry 与 rejected operation evidence；
- SM-1：blocked/active/terminal lifecycle，以及显式新 Run lineage；
- SM-4：restart/load、governance continuation 与 bounded no-progress guard。

repair 与 replan 的边界不得由 application queue、自动 successor 或 projection watcher实现。完整规约见 [Runtime V2 四状态机固化规约](../00-core/runtime-stage-contract.md#runtime-v2-四状态机固化规约)。

## Flow

```text
failure / missing prerequisite / candidate schema failure / operation failure / repairable assessment / governance need
  -> reducer updates FailureFact / Diagnostics / Prerequisites / Decision.Action
  -> same-run reconverge:
       AcquirePrerequisite
       | candidate attempt 0 -> fail -> attempt 1 (same transaction + stable prefix)
       | repairable implementation gap -> ProduceStateChange
       | repairable evidence gap       -> ProduceValidation
       | ProduceAssessment -> partial/satisfied/repairable/governance/terminal disposition
       | RequestGovernance -> governed_wait
            OpenRuntimeGovernanceRequest
            ... external Decision / Approval ...
            CommitRuntimeGovernanceResolution (once per resolution ID + snapshot)
            accepted governance evidence -> same reducer
            governance fail -> governance_rejected terminal
  -> if path cannot continue within V2 invariants or repair budget exhausted:
       structured Terminal Fail  (or product later creates a new activation/run)
```

## Main Stages

### 1. Repair = Same Focus Reconverge

repair 在 V2 中不是独立 stage 对象，而是 **drive/reducer 在当前 authority 与 focus 上的再推进**：

- 缺失 manifest / target snapshot / capability / validation scratch / provider ceiling  
  → `AcquirePrerequisite`（确定性，非 LLM）
- candidate schema / contract / provider failure  
  → 同一 authority 上 attempt 1（复用 transaction + stable prefix）
- operation 失败 evidence  
  → FailureFact 进入 facts；reducer 决定下一 action（仍可能是 state change / validation / governance）
- assessment 返回 repairable gap
  → 结构化 gap 进入 facts；按 missing evidence kind 回 state change / validation，不能直接 terminal
- 需要外部信息 / 审批 / 依赖  
  → sealed governance candidate → ticket → resolution commit

不得：

- 广泛重新规划为新的 V1 graph
- 第三次 candidate 请求
- accepted 后无 dispatch 重试
- 用 wrapper / schema 外壳覆盖原 failure surface 的权威身份（diagnostic 可记录，但 authority 仍由 facts 决定）

### 2. Candidate Attempt Budget

- attempt 0 与 attempt 1 状态随 aggregate 持久化
- attempt 1 必须复用相同 `AuthorityTransaction` 与 `StablePrefixHash`
- 第二次失败后生成 structured candidate exhaustion fact，交 SM-2 按合同进入 governance 或 bounded terminal；不得发起第三次 same-contract candidate
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
- `runtime_v2_governance` resolution committed 后 Orchestrator 发布 immutable continuation/terminal fact；Run owner内部迁移后通知 drive；application resolved watcher **不得**再次发送 Resume 形成双驱动

### 4. Replan Semantic = New Activation / Run Only

Runtime V2 **不自动创建** replan successor Run。

当产品语义需要“重新规划”时：

```text
operator / application
  -> new activation (new RuntimeBootstrap)
  -> new Run
  -> independent DriveRuntimeV2
```

约束：

- 旧 Run 可 terminal / supersede，只作 lineage
- 新 Run 不得复制旧 active focus、stale snapshots、已失效 failure 作为默认真值
- `Resume` 不能通过 patch / loaded Run 重写定义；定义变化 = 新 bootstrap

### 5. Restart Recovery

- 服务重启只加载 Run owner 中的 `RuntimeStateRecord` aggregate
- 不记录 / 不恢复 V1 `runtime_step_result_available`、watchdog recovery policy 等 checkpoint
- 不自动重放 sealed 但未完成的进程内 executor 假象；推进只经 `DriveRuntimeV2` 与 typed evidence/resolution
- 重复 Drive 必须幂等；无持久化进展 fail-fast

### 6. What Is Not On This Path

- V1 recovery action 矩阵作为第二套状态机（`retry_same_step` 自动建 Step 等）
- `dispatched` Step 进程内 executor 重启自动重派
- application continuation queue / patch resume facade
- auto-governance 静默创建 successor Run
- context gate 伪装成普通 state-changing repair 但绕过 prerequisite / candidate seal

Context 前置失败（缺 snapshot、缺 capability 等）应优先 `AcquirePrerequisite` 或 governance，
而不是伪造 state-changing candidate。

## Key Flow Guarantees

- repair 继承当前 definition revision、focus 与 authority 约束
- failure / diagnostic 分层：stale 只进 diagnostic lineage
- replan = 新 activation/run，不是同 Run 自动 successor
- governance resolution 单次消费并回到同一 reducer
- Start/Resume 生产入口只向 Run owner 发送 intent；缺少 bootstrap 通过 owner-local terminal delivery 收敛

## Output

repair / recovery 的正式输出可以是：

- 同一 Run 上继续的 drive transition（prerequisite / candidate / evidence / governance wait）
- typed terminal Fail（含 governance_rejected）
- 由产品层发起的**新** V2 Run（replan 语义）

## Source Documents

- [overview.md](overview.md)
- [activation-definition-flow.md](activation-definition-flow.md)
- [planning-execution-flow.md](planning-execution-flow.md)
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`（`runtime_governance.go`、`runtime_continuation.go`）
