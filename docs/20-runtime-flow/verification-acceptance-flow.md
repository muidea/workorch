# Verification And Acceptance Flow

Status: active

Type: runtime-flow

Last Updated: 2026-05-30

## Purpose

本文件定义 WorkOrch 如何从执行证据收口到正式 DoD assessment、Result 和 Artifact。

## Flow

```text
execution evidence accumulated
-> graph follow-up or completion follow-up
-> verification evidence collection
-> dodAssessment
-> completion gate
-> accepted / rejected Result
-> Artifact projection
```

## Main Stages

### 1. Verification

verification 的职责是基于已有执行结果收集和判定证据。

verification 不应继续承担：

- 新的实现主动作
- 无界 graph 扩张
- 用模糊摘要替代 evidence refs

### 2. Acceptance

acceptance 的职责是：

- 对照当前有效 Goal / DoD
- 基于 evidence refs
- 输出正式 accepted / rejected 结果

accepted Result 必须是正式业务结果，而不是运行时过程摘要。

### 2.1 Assessment-To-Continuation Routing

当 `completionState=failed|continue|incomplete` 且输出中已经包含 failed
`dodAssessment` / `goalCoverage` / `deliverableCoverage` 时，runtime 不能先把它视为
“no-actionable continuation”。

稳定分流应为：

| assessment 形态 | 下一步 |
| --- | --- |
| 只有证据不足，但已有候选 evidence | `need_validation` 或 concrete validation evidence repair |
| 需要状态变更，但尚无 concrete target evidence | `need_evidence`，先做 bounded read / target discovery |
| 已有 `coverageTargetSet`、target refs、target snapshots 或等价当前证据 | `need_state_change` |
| 明确缺少 runtime 无法自动取得的信息、权限、审批或外部依赖 | `need_governance` |

该转换只依赖 assessment 结构、coverage gap、evidence refs/candidates、manifest、
target refs 和 next actions，不得按语言、框架、业务类型做 core 特判。

### 2.2 Assessment-Only Correction

若 `dodAssessment.satisfied=true` 但 DoD / Goal / deliverable coverage 字段缺失或不完整，
该失败首先属于 assessment contract correction，而不是新的实现缺口。

稳定要求：

- 先进入 `assessmentOnly` / `verification` correction
- 要求补齐：
  - `dodAssessment.items`
  - `goalCoverage`
  - `acceptanceDimensions`
  - `deliverableCoverage`
  - `evidenceRefs` / `artifactRefs`
- 若仍无候选证据或无法机械回链，再进入 concrete validation evidence repair
- 不得因为 assessment 字段不完整就直接强制新的 state-changing patch

### 2.3 Completion Focus Priority

completion / acceptance 应遵守以下优先级：

- 最新 non-superseded accepted evidence 优先
- 已被后续成功 evidence 覆盖的历史失败，只能保留为 lineage / 诊断
- graph expansion limit follow-up 应优先转 assessment-only，而不是继续自由扩图

这意味着：

- accepted Result 是 workspace/run outcome 的首选 truth source
- failed step / failed validation 仍保留，但不能压过最新 accepted closure
- completed Run 没有 accepted Result 时，必须投影为 incomplete / not accepted

### 2.4 Current-Run Evidence Gate

verification / acceptance 默认优先消费当前有效 run lineage 的证据。

稳定要求：

- accepted / rejected assessment 默认优先引用当前 run 的 step / patch / validation / artifact evidence
- superseded run、历史 terminal run、memory-only ref 和 wrapper failure 不能默认充当当前 closeout evidence
- 只有显式继承到当前 lineage 的 evidence，才可进入当前 acceptance 候选集

该 gate 的作用是阻止历史失败、历史通过或低价值 summary 覆盖当前 run 的真实收口状态。

### 3. Graph Follow-up Closure

当 graph 已经执行完成后：

- follow-up 的目标应是收口已有证据
- 已有足够证据时，应优先进入 DoD assessment
- 不能让 follow-up 持续扩图并无限延期 acceptance

graph follow-up 收口补充要求：

- 当 follow-up 达到扩展上限后，应先进入 assessment-only，而不是继续自由扩图
- assessment-only 模式下返回的 `executionGraph` 不应被执行；它应被视为 contract violation，并进入更窄的 assessment correction 或 DoD-gap driven repair
- `dodAssessment.satisfied=false`、failed/skipped item 和 `missingDoDItems` 应成为后续 repair/replan 的主要输入
- repair/replan 不应再被自由 graph expansion 主导，而应围绕结构化 DoD gap 展开

#### MissingDoDItems

`missingDoDItems` 是 verification / acceptance 阶段的缺口视图，而不是新的正式 DoD。

它的稳定职责是：

- 摘要当前仍未关闭的 DoD coverage
- 为 assessment-only correction、concrete validation evidence repair、replan 提供输入
- 与 accepted / rejected evidence refs、coverage targets 和 current gap 保持可回链关系

约束：

- `missingDoDItems` 只描述缺口，不追加新的正式 DoD 定义
- 它可以被 rehydrate 到 work item / currentGap，但不应变成第二套正式定义账本

closure 质量补充要求：

- 最终业务成功由 DoD-backed accepted Result 决定，而不是“没有历史失败步骤”
- 历史 failed Step / ToolCall / ValidationResult 仍保留为证据，但不能覆盖最终 accepted outcome
- completed Run 但没有 accepted Result，应显式显示为 incomplete / not accepted
- Decision / Approval resolution 必须要么排入真实 continuation，要么进入明确终态失败

## Key Flow Guarantees

- satisfied DoD item 必须有 evidence refs
- satisfied DoD item 必须优先引用当前有效 run lineage 中的直接 evidence
- failed assessment 只要仍带结构化 coverage gap，就必须进入 action routing，而不是停在 no-actionable 解释
- current-run evidence gate 必须优先于历史 summary / superseded evidence / memory-only ref
- accepted Result 必须通过 completion gate
- Result / Artifact 必须基于当前最新有效 Run 和 evidence
- 历史失败证据不能压过最新 accepted closure evidence
- Workspace outcome / Run outcome 若存在 accepted Result，必须优先投影该 Result，而不是回退到更旧的 failed Step / failed Run
- Workspace outcome、Run outcome、taskProgress、observability 和 Web/TUI 需要复用同一 closure projection 语义
- graph exhausted 后若进入 completion follow-up，必须在 validation / repair / replan 之间明确分流，不能直接掉回无意义失败或无限继续
- assessment-only 不得作为隐藏的 executable graph 通道
- accepted / rejected Result 都必须能够解释 DoD gap、evidence refs 和 next actions 的来源

## Coverage Work Item Flow

`coverageWorkItems` 是 acceptance/verification 阶段的可执行 coverage ledger 投影，
用于表达“当前 effective Goal / DoD / deliverable source 还缺什么中立 evidence 才能关闭”。

稳定职责：

- 来源只能是当前 effective definition baseline 的 leaf coverage source
- 只能表达 evidence / governance / artifact gap
- 不得生成新的业务需求、重复 DoD 或实现方案

### Claim-To-Evidence Alignment

当 assessment 声称某个 Goal / DoD / deliverable source 已满足时，
verification / acceptance 必须执行中立 claim-to-evidence alignment。

稳定规则：

- claim 必须能够回链到：
  - accepted evidence ref
  - 当前 work item / coverage target
  - 当前 run lineage 中的验证或状态变更证据
- 泛化 summary、memory-only ref、无 direct scope 的总体验证通过，不能替代 claim 级 alignment
- alignment 失败时，应进入 assessment correction 或 concrete validation evidence repair，
  而不是直接 accepted

### Convergence Mapping

当前 work item 与 `ConvergenceContract` 的稳定映射至少应为：

| Work item 状态 | Convergence 状态 | 下一步 |
| --- | --- | --- |
| 缺 `state_change` 且无 concrete target evidence | `need_evidence` | bounded read / target discovery |
| 缺 `state_change` 且已有 target evidence | `need_state_change` | state-changing artifact 或 blocker |
| 已有 `state_change`，缺 `validation` | `need_validation` | bounded validation evidence |
| evidence kinds 齐备，但 assessment / alignment 不完整 | `need_assessment` | 结构化 assessment |
| 缺用户信息、权限、审批或外部依赖 | `need_governance` | decision / approval / dependency ticket |

### Stage Update Rules

- `planning`
  - 负责把 baseline source 投影为计划单元，但不能扩张出新的 active source
- `execution`
  - 只生成 candidate / accepted / rejected evidence，不能凭执行摘要直接 satisfied
- `verification`
  - 负责补 validation evidence 与 alignment 结果
- `acceptance`
  - 执行完整性门禁，并决定 accepted / rejected Result 是否成立
- `repair` / `follow-up`
  - 继承 unresolved work item scope，不得因 wrapper failure 清空账本

### Ledger Role In Acceptance

在本 flow 层中，`coverageWorkItems` 的职责固定为：

- verification / acceptance 的 coverage ledger 投影
- 说明哪些 source 已 `satisfied`、哪些仍 `open` / `blocked` / `failed`
- 驱动 current action 分流与 accepted / rejected Result 的完整性门禁

它不负责：

- 生成新的 Goal / DoD 定义
- 替代 Run / Step / Result 的主状态机
- 表达 profile-specific 修复方案

### Coverage Ledger Audit Families

verification / acceptance 在审计 `coverageWorkItems` 时，至少应按以下族进行核对：

- baseline completeness
  - 当前 effective Goal leaf、DoD item、required acceptance dimension、
    required deliverable expectation 都必须有对应 active / satisfied / blocked /
    skipped-with-reason item，或显式 `excludedReason`
- one-source-one-item
  - 同一 `definitionRevision` 内，相同 `normalizedSourceKey` 不得重复生成 active item；
    不同 source 不得因文本相似被静默合并
- stage update permission
  - activation / definition changed / planning / successor rebuild 才能重建 baseline；
    execution / verification / acceptance / repair / follow-up 只能更新运行期 status、
    evidence refs、`currentGap`、reason 等字段
- wrapper inheritance
  - provider retry、schema repair、repair-of-repair、decision/approval resolved
    不得清空 unresolved work item scope
- focus completeness
  - unresolved item 存在时，LLM-facing context 必须包含 `summary`、`currentFocus`
    和当前阶段可行动缺口；缺失时必须 gate / fail-fast
- evidence alignment
  - satisfied claim、accepted result、assessment 结论都必须回链到 active work item、
    accepted evidence refs 和当前 run lineage 中的 direct evidence
- revision isolation
  - 旧 `definitionRevision` item 必须进入 `historical` / `superseded` 视图，
    不得混入当前 `currentFocus`

## Output

verification / acceptance 链最终输出：

- `dodAssessment`
- accepted / rejected Result
- Artifact refs
- outcome / taskProgress / read-model projection

## Source Documents

- `../60-verification/audits/full-chain-assessment-closure-plan.md`
- `../70-roadmap/active/run-closure-quality-improvement-plan.md`
- `../00-core/runtime-stage-contract.md`

## Open Migration Notes

- Workspace outcome / taskProgress 与 latest result/run 的一致性规则还需后续补强。
- `RunOutcome` / `WorkspaceOutcome` projection 与 completion gate 的更细读模型映射仍需继续展开。
