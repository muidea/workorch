# Verification And Acceptance Flow

Status: normative

Type: runtime-flow

Last Verified: 2026-08-18

## Purpose

本文件定义 Runtime 如何从 execution / validation evidence 进入 assessment ledger，
并投影正式 accepted / rejected Result。

核心原则：

- **Result 不裁决**：acceptance 判定在 Orchestrator reducer + assessment ledger
- Result owner 只持久化已经裁决的 formal outcome
- 不存在 禁止的 completion gate / graph follow-up / `dodAssessment` 自由 JSON 作为主链权威

## State Machine Binding

本 flow 绑定核心规约中的：

- SM-2：evidence quality/coverage gate、partial assessment、repairable gap、Run-level completion gate；
- SM-3：AssessmentCandidate attempt/seal/commit；
- SM-1：Run owner-local complete/failed terminal transition；
- SM-4：Result → Workspace → Run owner-local delivery acknowledgement 与重试。

`Assessment.Satisfied=false` 不得绕过 SM-2 的 structured disposition/gap 直接由 Result 或 application 终态化。完整规约见
[Runtime State Machines](../state-machines.md)。

## Flow

```text
execution evidence accumulated for current CoverageFocus
  -> required evidence kind + coverage + quality + current-run gates satisfied?
       no  -> need_state_change | need_validation | need_prerequisite | need_governance
       yes -> if RequireAssessment:
                need_assessment
                  -> AssessmentCandidate (bound to current focus/coverage)
                  -> CommitRuntimeAssessment
                  -> write RuntimeAssessmentFact into assessment ledger
                  -> disposition:
                       satisfied / partially_satisfied
                         -> close explicit satisfied work items
                         -> current slice remainder / next focus
                       repairable_gap
                         -> structured gap -> same-focus state-change/validation repair
                       governance_required
                         -> decision/approval wait
                       terminal_rejection
                         -> structured rejected result
              else close current focus items and advance
  -> no remaining work items
       -> Run-level completion gate
            -> pass: TerminalOutcome{Kind=completed} -> accepted Result
            -> repairable gap: same-focus/run-level repair
            -> irrecoverable/budget exhausted: TerminalOutcome{Kind=failed} -> rejected Result
  -> persisted terminal delivery fact -> Result owner -> Workspace owner -> Run owner
```

## Main Stages

### 1. need_validation（验证执行）

validation 在 Runtime 中是 canonical state `need_validation` 的 sealed candidate 路径：

```text
need_validation
  -> ValidationCandidate.Operations
  -> seal dispatch
  -> ExecuteRuntimeOperation (validation kinds)
  -> commit evidence with coverage work item IDs
  -> reducer
```

职责：

- 为当前 focus 补齐 required validation evidence
- 绑定 coverage work item IDs 与 target refs

不得：

- 新的无界实现主动作（那属于 `need_state_change`）
- 用模糊摘要替代 evidence refs
- 由 validation block 直接判定 DoD satisfied

### 2. Assessment Ledger

显式声明 `RequireAssessment` 的 focus，在执行证据齐备后进入 typed `need_assessment`。

规则：

- LLM 只能返回绑定当前 focus / coverage 的 `AssessmentCandidate`
- Orchestrator 校验 `evidenceRefs` 必须来自**当前 Run** 的 accepted evidence
- 通过后写入 assessment ledger（`RuntimeAssessmentFact`）
- assessment 必须区分 satisfied、partially_satisfied、repairable_gap、governance_required、terminal_rejection
- partial assessment 只能关闭显式 satisfied 的 coverage work item；未满足 item 保留在 current/remaining queue
- repairable gap 必须携带 work item IDs、missing evidence kinds、target refs、reason 与 evidence refs，并回到同 focus repair
- assessment-only focus **不允许** executable operation
- 没有 current-run accepted evidence 的 satisfied assessment **必须拒绝**
- 不得用 summary、历史 memory 或自造 ref 关闭 coverage

assessment fact 可携带 scope（如 goalCoverage / acceptanceDimension / deliverable）与
requirement text，供后续 formal Result payload 与派生 projection 组装 coverage 视图。

内部assessment disposition不能直接作为产品三轴状态。read model必须从canonical ledger纯派生：

| 产品维度 | 允许值 | 派生来源 |
| --- | --- | --- |
| coverage | `uncovered | partially_covered | covered` | CWI required bindings与current accepted evidence completeness |
| assessment | `unassessed | satisfied | unsatisfied | indeterminate` | applicable accepted assessment fact；内部`partially_satisfied`表示只关闭明确satisfied CWI，其余逐项仍为unassessed/unsatisfied/indeterminate |
| evidence | `candidate | accepted | rejected | stale_or_superseded` | receipt judgement、diagnostic、binding/freshness lineage |

三个维度不得折叠为一个enum或overall score。`indeterminate`保留reason和missing evidence/owner fact；当前 all-of 基线下
它永远不能视为satisfied，也不存在Definition/Profile可声明的成功例外。projection只读取同workspace/run/Definition/Runtime
revision的facts，不写回assessment或coverage authority。

### 3. Formal Result And Projection（不裁决）

最后一个 work item 关闭并通过 Run-level completion gate后，或 bounded terminal 决策已经形成：

1. Orchestrator 密封 terminal 与 result payload，并发布 terminal delivery fact
2. Result owner 内部持久化 accepted/rejected Result；成功 notification 驱动 Workspace owner release，随后由 Run owner内部 terminal
3. Result block **只持久化**正式 outcome，**不参与** acceptance 裁决；SM-4 只记录各步 acknowledgement

投影输入来自：

- `RuntimeDecision.Terminal`（typed `TerminalOutcome.Kind/Code/Summary`，不含 Runtime action）
- Definition revision、startup-input identity、Goal/DoD 与execution lineage；
- canonical CWI coverage ledger、assessment facts与current accepted evidence refs；
- deliverable/artifact refs、applicable owner revision/snapshot/fence set及其hash；
- known limitations、indeterminate/unsatisfied reason与terminal diagnostic。

幂等：同一 Result identity 已存在且 state 一致则成功返回；冲突则 fail。

### 4. Focus Advance And Complete

- 当前 focus required evidence 满足后先进入适用的 assessment；只有不要求 assessment 或 assessment 明确关闭对应 item 时才能推进
- partial assessment 原子固化 satisfied items，并保留未关闭 item
- 最后一个 focus/work item 满足且 Run-level completion gate 通过后才允许 `Complete`
- 前序 focus evidence 不得满足后续 focus
- profile-specific accepted/rejected kinds 只作为 typed focus requirement 输入；
  reducer 不识别 profile ID、deliverable ID 或业务语义

### 5. Coverage Work Item（中立 ledger 输入）

`CoverageFocus.CoverageWorkItemIDs` 与 evidence ledger 共同表达“当前还缺什么中立 evidence”。

稳定职责：

- 来源是 activation 时封存的 Definition seed；focus/baseline 只能由 SM-2 根据受约束 semantic proposal 与 owner
  feasibility 封存，不能在执行期扩张 Goal/DoD 之外的新业务需求
- evidence role（如 primary_state_change / supplemental_evidence）影响 required evidence kinds，
  由 template/profile policy 约束并在 SM-2 semantic contract seal 时固化
- claim-to-evidence alignment：satisfied assessment 必须回链 current-run accepted evidence
  与当前 focus coverage

不负责：

- 生成新的 Goal / DoD 定义
- 替代 Runtime aggregate 状态机
- 表达 profile-specific 修复方案正文

### 5.1 Semantic planning observation

Run owner 仍只 CAS 保存 opaque Runtime aggregate。Application 可从该 aggregate 构建只读
`acceptance.semanticPlanning`，展示当前 phase、逐 CWI proposal、capability-owner feasibility、retired gap lineage、
sealed binding set hash 和原始 diagnostic；该 projection 不发送推进事件、不选择 retry/terminal，也不形成 binding。

投影必须同时绑定 workspace ID、run ID、definition revision 与 Runtime state revision。Workspace RuntimeView 只在
semantic state revision 与当前 Runtime high-watermark 一致时展示；如果没有其它 Runtime progress 投影，semantic
state revision 可建立该只读 high-watermark。跨 scope 或跨 revision 内容必须省略，不能通过 eventstream 或 UI 合并。

### 6. What Is Not On This Path

- 禁止的 graph follow-up / `maxGraphFollowUpExpansionAttempts` 作为 acceptance 主链
- assessment-only 模式下仍执行 `executionGraph`
- completed Run 却依赖 禁止的 Result/DoD completion gate
- Result / Artifact 自行判断 DoD

## Key Flow Guarantees

- satisfied assessment 必须有 current-run accepted evidence refs
- repairable assessment 不直接 terminal；partial assessment 不丢失未关闭 item
- Orchestrator 判定 acceptance；Result owner只持久化自身 outcome lifecycle
- accepted / rejected Result 都能从 assessment ledger 与 terminal summary 解释来源
- Result逐项投影coverage/assessment/evidence三轴，且只使用closeout时仍applicable的owner revision set；
- 历史失败 evidence 可保留在 diagnostic / rejected lineage，但不能覆盖最新 accepted closure
- 正式 terminal 只能来自 reducer decision + Result/Workspace/Run 的 typed owner-local delivery；
  Orchestrator 不注册 ValidateRun / CompleteRun 或 lifecycle projection 作为第二套 completion 入口

## Output

- assessment ledger facts
- accepted / rejected Result（Result owner）
- Result / Workspace / Run owner-local terminal delivery
- 可选 Artifact refs（由 Result / evidence 附件路径承载，见 Evidence And Result）

## Source Documents

- [overview.md](overview.md)
- [Planning And Execution](planning-execution.md)
- [Evidence And Result](evidence-result.md)
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/modules/blocks/result/readme.md`
