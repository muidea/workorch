# Verification And Acceptance Flow

Status: active

Type: runtime-flow

Last Updated: 2026-07-14

## Purpose

本文件定义 Runtime V2 如何从 execution / validation evidence 进入 assessment ledger，
并投影正式 accepted / rejected Result。

核心原则：

- **Result 不裁决**：acceptance 判定在 Orchestrator reducer + assessment ledger
- Result owner 只持久化正式 outcome projection
- 不存在 V1 completion gate / graph follow-up / `dodAssessment` 自由 JSON 作为主链权威

## Flow

```text
execution evidence accumulated for current CoverageFocus
  -> required evidence kinds satisfied?
       no  -> ProduceStateChange | ProduceValidation | AcquirePrerequisite | RequestGovernance
       yes -> if RequireAssessment:
                ProduceAssessment
                  -> AssessmentCandidate (bound to current focus/coverage)
                  -> CommitRuntimeV2Assessment
                  -> write RuntimeAssessmentFact into assessment ledger
              else advance focus / remaining queue
  -> last assessment closed (or no remaining focus requiring assessment)
  -> reducer Terminal Complete | Fail
  -> project Result via Result owner create command (accepted/rejected)
  -> project Run / Workspace terminal
```

## Main Stages

### 1. ProduceValidation（验证执行）

validation 在 V2 中是 reducer action `ProduceValidation` 的 sealed candidate 路径：

```text
ProduceValidation
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

- 新的无界实现主动作（那是 `ProduceStateChange`）
- 用模糊摘要替代 evidence refs
- 由 validation block 直接判定 DoD satisfied

### 2. Assessment Ledger

显式声明 `RequireAssessment` 的 focus，在执行证据齐备后进入 typed `need_assessment`
（实现上为 `ProduceAssessment`）。

规则：

- LLM 只能返回绑定当前 focus / coverage 的 `AssessmentCandidate`
- Orchestrator 校验 `evidenceRefs` 必须来自**当前 Run** 的 accepted evidence
- 通过后写入 assessment ledger（`RuntimeAssessmentFact`）
- assessment-only focus **不允许** executable operation
- 没有 current-run accepted evidence 的 satisfied assessment **必须拒绝**
- 不得用 summary、历史 memory 或自造 ref 关闭 coverage

assessment fact 可携带 scope（如 goalCoverage / acceptanceDimension / deliverable）与
requirement text，供后续 Result projection 组装 coverage 视图。

### 3. Result Projection（不裁决）

最后一个 assessment 关闭后（或 terminal 决策已形成）：

1. Orchestrator 通过 Result owner 的具体 EventHub create command 写入 accepted/rejected Result
2. 再投影 Run / Workspace terminal
3. Result block **只持久化**正式 outcome，**不参与** acceptance 裁决

投影输入来自：

- `RuntimeDecision.Terminal`（summary、action Complete/Fail）
- assessment ledger facts 与其 evidence refs
- definition 上的 GoalID / DoDID

幂等：同一 Result identity 已存在且 state 一致则成功返回；冲突则 fail。

### 4. Focus Advance And Complete

- 当前 focus required evidence 满足后，evidence commit 原子切换下一 focus
- 最后一个 focus 满足后才允许 `Complete`
- 前序 focus evidence 不得满足后续 focus
- profile-specific accepted/rejected kinds 只作为 typed focus requirement 输入；
  reducer 不识别 profile ID、deliverable ID 或业务语义

### 5. Coverage Work Item（中立 ledger 输入）

`CoverageFocus.CoverageWorkItemIDs` 与 evidence ledger 共同表达“当前还缺什么中立 evidence”。

稳定职责：

- 来源是 activation 时封存的 focus / baseline，而不是执行期扩张新业务需求
- evidence role（如 primary_state_change / supplemental_evidence）影响 required evidence kinds，
  由 bootstrap/profile 在 activation 侧声明
- claim-to-evidence alignment：satisfied assessment 必须回链 current-run accepted evidence
  与当前 focus coverage

不负责：

- 生成新的 Goal / DoD 定义
- 替代 Runtime V2 aggregate 状态机
- 表达 profile-specific 修复方案正文

### 6. What Is Not On This Path

- V1 graph follow-up / `maxGraphFollowUpExpansionAttempts` 作为 acceptance 主链
- assessment-only 模式下仍执行 `executionGraph`
- completed Run 却依赖 V1 Result/DoD completion gate
- Result / Artifact 自行判断 DoD

## Key Flow Guarantees

- satisfied assessment 必须有 current-run accepted evidence refs
- Orchestrator 判定 acceptance；Result 只投影
- accepted / rejected Result 都能从 assessment ledger 与 terminal summary 解释来源
- 历史失败 evidence 可保留在 diagnostic / rejected lineage，但不能覆盖最新 accepted closure
- 正式 terminal 只能来自 reducer decision + typed owner projection；
  Orchestrator 不注册 ValidateRun / CompleteRun 作为第二套 completion 入口

## Output

- assessment ledger facts
- accepted / rejected Result（Result owner）
- Run / Workspace terminal projection
- 可选 Artifact refs（由 Result / evidence 附件路径承载，见 evidence-result-flow）

## Source Documents

- [overview.md](overview.md)
- [planning-execution-flow.md](planning-execution-flow.md)
- [evidence-result-flow.md](evidence-result-flow.md)
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/modules/kernel/orchestrator/runtime_v2_result_projection.go`
