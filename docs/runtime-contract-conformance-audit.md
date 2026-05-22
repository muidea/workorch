# Runtime Contract Conformance Audit

本文用于把运行期问题从单点修复切换为合同符合性核查。唯一设计源是 `workorch-runtime-stage-contract-neutrality-design.md`；本文只记录核查矩阵、实现落点、当前缺口和收口跟踪，不引入新的业务规则。

## 核查原则

- 每个偏差先判断设计是否已有约束，再定位代码落点，最后补回归测试。
- 核心框架保持功能中立，只处理 stage、contract、evidence、target、repair、validation、governance 和 projection。
- 禁止通过修改 workspace deliverable 输出物来修复 WorkOrch runtime 偏差。
- 没有测试覆盖的修复不视为完成收口。

## 路径矩阵

| 路径 | 必须保持的合同 | 当前核查结论 | 收口状态 |
| --- | --- | --- | --- |
| initial planning | Goal/DoD、executionContract、runtimeStage、processEvidenceTimeline | 已有实现，需持续回归 | 已覆盖 |
| exploration graph follow-up | read-only evidence、currentProgressDigest、bounded follow-up | 暂未发现本轮偏差 | 待持续核查 |
| evidence acquisition follow-up | 继承 unresolved failure、currentFailure、currentExecutionProgress、repairActionPackage、targetSnapshots | `repairActionPackageForEvidenceAcquisitionFollowUp` 未提升当前失败，导致 context gate `runtime_repair_current_failure_missing` | 本轮收口 |
| repair-of-repair | 当前失败优先，旧 rejected output 只作摘要，不复用 raw content | 已有清理 rejected summary 的实现，需保持 | 已覆盖 |
| replan successor 首步 | 继承 predecessor unresolved failure、processEvidenceTimeline、currentProgressDigest、repairActionPackage | 已有首步 hydration，本轮继续通过测试守护 | 已覆盖 |
| decision resolved 恢复 | resolved decision 只作恢复信号，不能清空 repair contract；context-gate resolved 后必须生成新的 context retry step | resolved `runtime_context_required` 后曾重放旧失败 step 并生成重复 ticket | 本轮收口 |
| validation / acceptance repair | evidenceRefs 归一、coverage correction 与 state-changing repair 隔离 | 暂未发现本轮偏差 | 待持续核查 |
| context gate fail-fast | 必要上下文缺失时本地 fail-fast；可继承上下文不得误判缺失 | 本轮误判来自 follow-up 未继承 current failure，且恢复路径没有重建上下文 | 本轮收口 |
| workspace/run/ticket projection | workspace detail/list 必须投影 active run blocker | workspace API 出现 `waiting_decision` 但 `blockingTicketId` 为空 | 本轮收口 |

## 本轮偏差

### Evidence Follow-up 上下文断链

事实：

- active run 已进入 `run-ea34332557f20b61-replan-001`。
- evidence acquisition 已完成并带有 `targetSnapshots`。
- `executionContextPackage.replanContinuation.failureContext` 保留了 failed step 和 failure signature。
- follow-up 输入缺少 `repairActionPackage.currentFailure` 与 `currentExecutionProgress`，触发 `runtime_repair_current_failure_missing`。

归因：

- 设计已覆盖该路径，属于实现偏差。
- `repairActionPackageForEvidenceAcquisitionFollowUp` 只合并 evidence 与 requiredOutput，未从 source package、source input 或 replan continuation 中恢复当前失败对象。

修复要求：

- evidence acquisition follow-up 必须从当前输入、source repair package 或 replan continuation 中构造中立 current failure。
- 若有 process evidence digest，也必须同步放入 `currentExecutionProgress`。
- 不允许引入具体语言、文件名、测试命令或业务目标判断。

### Context Gate Resolved 后重复阻塞

事实：

- `runtime_context_required` ticket 以“继续处理” resolved 后，run 短暂恢复 `running`。
- runtime 随后仍以同一 failed context-gate step 作为恢复点，未生成新的 provider request。
- 同一 `runtime_repair_current_failure_missing` 被重新创建为 `context-integrity-decision-2`。

归因：

- 恢复路径只把 resolved decision 合并为 resume input，然后继续调用失败 step 的 next-dispatch 逻辑。
- 对于 `llm_context_integrity_failed`，next-dispatch 会再次进入 context gate blocker，而不是重新 hydrate 可发送给 LLM 的 context。

修复要求：

- resolved `runtime_context_required` 必须进入专用 context retry 分支。
- retry input 必须从 ticket context、failed step input 与 replan continuation 中补齐 `repairActionPackage.currentFailure` 和 `currentExecutionProgress`。
- 若补齐失败，不得重复创建同一 blocker；必须显式转入 replan、终态或新的结构化 blocker。

### Workspace Blocker 投影缺失

事实：

- run 已持久化 `BlockingTicketID`。
- workspace detail 返回 `waiting_decision`，但 `blockingTicketId` 为空。

归因：

- 已在核心设计合约的 Active Run 一致性章节补充 read model blocker projection 约束。
- 实现层 `Workspace` DTO 和 `mapWorkspaceWithRunProjection` 未把 active run blocker 投影到 workspace detail/list。

修复要求：

- `Workspace` projection 增加 `blockingTicketId` 与 `blockingApprovalId`。
- 当 projection run 非 terminal 时，workspace detail/list 应派生 active run blocker。
- terminal run 不应污染 workspace active blocker。

## 回归测试要求

- Evidence acquisition follow-up with completed target snapshots must keep `repairActionPackage.currentFailure`.
- Evidence acquisition follow-up must keep `currentExecutionProgress` or equivalent current failure progress when source has process digest.
- Resolved `runtime_context_required` must dispatch a new context retry step instead of replaying the old failed context-gate step.
- Workspace detail/list projection must expose active run `blockingTicketId` / `blockingApprovalId`.
- Tests must stay domain-neutral.

## 收口跟踪

- [x] 完成设计合同对照。
- [x] 输出本文档。
- [x] 补 evidence acquisition follow-up 回归测试。
- [x] 补 context gate resolved retry 回归测试。
- [x] 补 workspace blocker projection 回归测试。
- [x] 刷新核心设计合约中的 blocker projection 约束。
- [x] 修复 runtime follow-up current failure 继承。
- [x] 修复 context gate resolved 后重复 blocker。
- [x] 修复 workspace projection blocker 字段。
- [x] 运行定向测试。
