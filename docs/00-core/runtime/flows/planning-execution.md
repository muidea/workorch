# Planning And Execution Flow

Status: normative

Type: runtime-flow

Last Verified: 2026-08-16

## Purpose

本文件定义 Runtime 从 reducer decision 到 candidate、sealed dispatch、operation
execution 与 evidence commit 的主执行链路。

生产主线是：

```text
candidate → seal → ExecuteRuntimeOperation → evidence → reducer
```

**不是**已删除的 `llm → executionGraph → checkpoint` 路径。

## State Machine Binding

本 flow 绑定核心规约中的：

- SM-2：reducer authority、focus、required output 与 evidence 后续动作；
- SM-3：candidate attempt 0/1、seal、operation execution 与 evidence commit；
- SM-4：bounded drive transition 与 persisted progress。

本文件不允许以 execution graph、block callback 或 application queue 建立第二条状态机；完整规约见
[Runtime State Machines](../state-machines.md)。

## Flow

```text
RuntimeDecision.ConvergenceState ∈ {
  need_state_change,
  need_validation,
  need_assessment,
  need_governance
}
  -> BeginRuntimeCandidateAttempt (attempt 0 or 1)
  -> RequestRuntimeCandidate
       Decision + AuthorityTransaction
       DefinitionContext
       RuntimeCandidateContext (from persisted facts)
       StablePrefixHash
  -> Accept / Fail candidate attempt
  -> seal SealedDispatch
       full transaction + candidate hash + typed step input
  -> branch:
       Assessment  -> CommitRuntimeAssessment
       Governance  -> OpenRuntimeGovernanceRequest -> governed_wait
       Operations  -> dependency-ordered ExecuteRuntimeOperation*
                       each op commits evidence independently
  -> reducer (focus advance / next action / terminal)
```

## Main Stages

### 1. Planning = Candidate Request（不是 graph 规划）

在 Runtime 中，“planning” 的实现形态是 **LLM 返回 typed candidate**，绑定当前 authority transaction。

允许的 candidate 形态：

| Candidate | 对应 canonical state | 内容 |
| --- | --- | --- |
| StateChangeCandidate | `need_state_change` | 一组 `RuntimeOperation`（state-changing） |
| ValidationCandidate | `need_validation` | 一组 validation operations |
| AssessmentCandidate | `need_assessment` | focus / coverage / satisfied / evidence refs |
| Governance candidate | `need_governance` | external dependency ticket 输入 |

约束：

- block / LLM 输出只是 **candidate**，不是 accepted Result
- candidate 必须绑定同一 authority snapshot；transaction 漂移不得进入执行
- attempt 0/1 的 pending / failed / accepted 随 aggregate 持久化
- attempt 1 **必须**复用相同 transaction 与 stable prefix；第二次 complete candidate rejection 只能形成 typed terminal，不能创建无信息 governance
- 不存在第三次 candidate 请求；accepted attempt 必须对应 active 或 completed sealed dispatch

### 2. Seal Before Execute

执行前必须持久化：

- `SealedDispatch`（state = sealed）
- 完整 `AuthorityTransaction`
- `candidateHash`
- typed step / operation input

没有 sealed dispatch 的 accepted attempt 会被 invariant validation 拒绝。

### 3. Operation Execution

sealed candidate 通过各接收 owner 在自身 `pkg/events` 定义的 concrete execute command/result，按 operation kind 投递到：

- File
- Git
- Shell
- MCP
- Web
- SSH

规则：

- command/result 使用**具体类型**；同步返回的 typed result 是提交 execution evidence 的**唯一**执行回执
- 跨组件只经 EventHub；不持有 owner service / repository
- 多 operation 按 dependency graph **确定性**执行；每个 operation **独立**提交 evidence
- 全部成功或任一失败后才结束 dispatch，再进入 reducer
- execution evidence 必须携带来自 sealed operation 的 coverage work item IDs
- 前序 focus 的 evidence **不得**满足后续 focus

Orchestrator **不**注册通用 runtime executor、禁止的 step kind matrix 或 RuntimeAdapter registry。

### 4. Coverage Focus And Required Output

- `RuntimeFacts` 持久化当前 focus 与有序 remaining focus queue
- 当前 focus 的 required evidence 满足后，evidence commit **原子**切换到下一 focus，再调用同一 reducer
- `CoverageFocus` 可携带 evidence role、accepted/rejected step kinds、accepted operations
- reducer 只在 owner 已确认的 capability kinds 中做中立交集/排除，结果封入 `RequiredOutputContract`
- 若声明 accepted operations，candidate 至少包含一个匹配的具体 operation 参数；同 step kind 的其他动作不能替代

### 5. Prerequisite Gate Before LLM

LLM dispatch 前必须先完成 preflight：

- manifest
- target snapshot
- failure target
- capability
- validation scratch
- provider ceiling

缺失资源只能进入 canonical `need_prerequisite`，不得混入 LLM state-changing requiredOutput。

### 6. Owner Admissibility Must Drive Correction

Candidate context and any bounded correction must use the latest capability-owner observation/catalog, including target
existence and revision. A complete candidate rejected by owner semantic contract is persisted with its stable factual
category and safe diagnostic before the one allowed correction.

For example:

- `file/write` is create-only and may target only an absent/prospective owner target;
- when the target already exists, correction must choose an admitted revision-bound replacement such as `file/edit` or
  `patch/apply`;
- retrying the equivalent create-only operation is not progress and must be rejected before side effect;
- stale catalog/target revision cannot be hidden by changing prose or operation identity.

LLM proposes the replacement, File owner checks current feasibility/admissibility, SM-3 seals it, and SM-2 alone decides the
post-receipt convergence state. No component may patch CWI/focus state to force closure.

### 7. What Is Not On This Path

以下内容已从生产主线移除，不得作为 planning/execution 正式路径：

- 禁止的 `executionGraph` 作为 orchestrator 调度权威
- graph follow-up expansion / leaf → NextAction
- `RuntimeCheckpoint` 推进状态
- `DispatchNextStep` / `EnableEventDrivenDispatch`
- `StagePacket` / `NextAction` 包装 candidate 或 evidence
- incremental artifact 作为独立 禁止的 stage 主链（若 profile 需要大正文承载，仍须落入
  sealed operation + evidence，不得绕过 seal/commit）

## Key Flow Guarantees

- LLM 只产生 structured candidate；不能直接写文件、执行 shell 或调用 MCP
- Orchestrator 拥有 seal、evidence quality、coverage binding 与 next action 判定
- block modules 只拥有外部动作执行
- 每次 dispatch 基于一致的 workspace/run/definition/evidence 快照
- authority attempt 仅在 canonical state、focus、definition/evidence revision、requiredOutput hash 全部相同时才增加；派生 action 不参与 identity，prerequisite 或其他事实推进必须重置 attempt
- drive 每个非等待、非终态 transition 必须增加 `stateRevision`

## Evidence Produced

本链路沉淀的是 Runtime typed facts / evidence ledger 输入，例如：

- operation evidence（coverage-bound）
- assessment facts（见 Verification And Acceptance）
- governance request / resolution records
- diagnostic lineage（stale / duplicate）

这些是后续 assessment、formal Result payload 与派生 projection 的输入，不是最终业务结果本体。

## Source Documents

- [overview.md](overview.md)
- [Verification And Acceptance](verification-acceptance.md)
- [Evidence And Result](evidence-result.md)
- [Context Assembly](../../runtime-request-context-assembly.md)
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`（`runtime_candidate.go`、`runtime_execution_contract.go`）
