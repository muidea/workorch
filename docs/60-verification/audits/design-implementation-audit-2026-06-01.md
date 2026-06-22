# 设计文档 vs 实现 核对报告

> 核对日期：2026-06-01
> 核对范围：`internal/modules/kernel/orchestrator/` 自 2026-05-12 审计以来的变更
> 核对方法：逐条对照 `runtime-stage-contract.md`、`verification-acceptance-flow.md`、`repair-replan-flow.md`、`kernel-orchestrator-governance.md` 与当前代码实现
> 前置审计：`design-implementation-audit-2026-05-12.md`

---

## 一、审计后新增变更概览

自 2026-05-12 以来，orchestrator 模块累计 30 个 commit，主要集中在：

- coverage work item evidence 角色精细化
- graph evidence 跨 sibling 继承到 repair context
- repair target scope 收窄与 eventhub payload 压缩
- evidence follow-up failure anchor 修正
- graph limit state change continuation 修正

本次核对后已完成收口的新增变更包括：

- `runtime_repair.go` — 增加结构化 `contractViolationType`，state-changing repair 路由优先使用结构化合同违规类型
- `runtime_orchestrator.go` — step dispatch 入口增加 active rejected result 前置拦截
- `read_model_service.go` / `read_model_types.go` — workspace snapshot 增加 coverage summary，context bundle 过滤完整 coverage ledger
- `verification-acceptance-flow.md` — 补齐 `evidenceRole` convergence 约束与 graph follow-up 扩展上限
- `runtime-stage-contract.md` — 固化 `contractViolationType` 中立合同

---

## 二、核对结论

### 已确认无偏差（3 条）

| # | 变更 | 核对依据 | 结论 |
|---|------|----------|------|
| 1 | `coverageSourceCanCarryStateChange` 限制为 `evidenceRole=primary_state_change` | `runtime-stage-contract.md:1132` 明确规定 `supplemental_evidence` 不得启动 primary state-changing patch | 修正了旧实现的过宽行为，完全合规 |
| 2 | `completedGraphEvidenceFromSteps` 从 graph root step 获取完整 graph spec 并按 `graphRootStepID` 过滤 scope | `repair-replan-flow.md` §Concrete Failure Evidence Acquisition 要求 rebuild failure target scope | 正确实现，scope 隔离保证了 repair 预算不被跨 graph 污染 |
| 3 | `runtimeRepairFollowUpStepInput` 从 graph evidence 提取 `targetSnapshots` 合并到 `failureTargetRepairContext` | `repair-replan-flow.md` §Wrapper Recovery Inheritance 要求保留 target snapshots | 正确实现 rebuild failure target scope and snapshots |

### 必须处理（2 条）

已完成收口，详见下方 §三 行动项。

### 文档同步建议（2 条）

已完成收口，详见下方 §三 行动项。

---

## 三、行动项

### P1-01：`stateChangingRepairRequired` 改用结构化标记

**状态**：已收口，已验证

**问题**：`runtime_repair.go:6257-6278` 通过 `strings.Contains(text, marker)` 匹配 18 个硬编码字符串判断是否需要 state-changing repair。字符串分散在多个源头函数中，任何 error message 文本微调都可能静默破坏 repair 路由。

**设计合约依据**：
- `repair-replan-flow.md` §1 Failure Classification：失败首先需要被结构化分类
- `runtime-stage-contract.md` §设计原则：恢复链依赖 failure class / failure signature / failure target scope

**风险等级**：中。当前功能正确，但脆弱性随代码演进持续累积。

**处理方案**：

1. 定义 `contractViolationType` 枚举常量：

```go
const (
    contractViolationGraphNodeReturnedGraph          = "graph_node_returned_graph"
    contractViolationRepairReturnedGraph             = "repair_returned_graph"
    contractViolationRepairNoStateChangingOutput     = "repair_no_state_changing_output"
    contractViolationRepairGraphWithoutPatchApply    = "repair_graph_without_patch_apply"
    contractViolationExecutionNoStateChangingOutput  = "execution_no_state_changing_output"
    contractViolationGraphDependsOnLLMBeforePatch    = "graph_depends_on_llm_before_patch"
    contractViolationPatchTargetRefRequired          = "patch_target_ref_required"
    contractViolationPatchContentRequired            = "patch_content_required"
    contractViolationPatchUnresolvedTemplate         = "patch_unresolved_template"
    contractViolationPatchDispatchInvalid            = "patch_dispatch_invalid"
    contractViolationFileEditConflict                = "file_edit_conflict"
)
```

2. 在产生这些 error 的源头函数中，同时写入 `step.Output["contractViolationType"]`

3. `stateChangingRepairRequired` 改为优先检查结构化字段：

```go
if vType := stringFromMap(failedStep.Output, "contractViolationType"); vType != "" {
    return contractViolationRequiresStateChangingRepair(vType)
}
// fallback: 保留字符串匹配兼容已持久化旧 step
```

**涉及文件**：
- `runtime_repair.go` — `stateChangingRepairRequired` 函数
- `runtime_orchestrator.go` — output contract validation 函数（产生 error message 的源头）
- `runtime_graph.go` — graph node contract validation

**验证方式**：现有测试应全部通过；新增测试验证结构化字段路径。

**收口结果**：

- `runtime_repair.go` 增加中立 `contractViolationType` 常量集合。
- `handleRuntimeStepFailure` 写入结构化 `contractViolationType`，`stateChangingRepairRequired` 优先读取结构化字段，字符串匹配仅作为旧数据 fallback。
- `runtime_context_integrity_gate_test.go` 增加结构化 repair 路由和 patch target contract 分类回归测试。
- `runtime-stage-contract.md` 补充 `contractViolationType` 中立合同。

---

### P1-02：`dispatchStep` 入口增加 rejected result 前置拦截

**状态**：已收口，已验证

**问题**：`dispatchStep` 是 step 派发的公共入口，但缺少 rejected result 前置检查。当前依赖上游调用方（`dispatchRuntimeRepairFollowUp`、`nextDispatchSpecs`）的检查，但 `dispatchStep` 本身无防御。

**设计合约依据**：
- `runtime-stage-contract.md` §状态机：终态不可逆，superseded 后禁止派发
- 2026-05-12 审计偏差 #1：本次已完成收口

**风险等级**：低。当前所有路径已覆盖，但随代码演进可能出现绕过。

**处理方案**：

在 `dispatchStep` 入口增加短路检查：

```go
func (o *RuntimeOrchestrator) dispatchStep(
    ctx context.Context,
    run *runModel.Run,
    stepID common.StepID,
    kind string,
    input map[string]any,
) error {
    if !boolValue(input["allowDispatchAfterRejectedResult"]) {
        if summary, ok := o.rejectedRunResultSummary(ctx, run.ID); ok {
            return fmt.Errorf("%w: run has rejected result: %s", common.ErrConflict, summary)
        }
    }
    // ... existing logic
}
```

**注意事项**：
- 确认 rejected result continuation 场景是否需要豁免（通过 `allowDispatchAfterRejectedResult` input 标记）
- `dispatchStepWithWorkspace` 同样需要检查（或确认它总是经过 `dispatchStep`）

**涉及文件**：
- `runtime_orchestrator.go` — `dispatchStep` / `dispatchStepWithWorkspace`

**验证方式**：新增测试验证 rejected result 后 dispatch 被拦截；确认现有 rejected result continuation 测试仍通过。

**收口结果**：

- `dispatchStepWithWorkspace` 增加 active rejected result 前置拦截，并保留 `allowDispatchAfterRejectedResult` 豁免入口。
- `runtime_orchestrator_test.go` 增加 rejected result 后禁止继续派发 step 的回归测试。
- 已确认 rejected result 被后续恢复进展 supersede 的既有测试仍通过。

---

### P2-01：`verification-acceptance-flow.md` 补充 `evidenceRole` 对 convergence 的影响

**状态**：已收口，已验证

**问题**：核心合约 `runtime-stage-contract.md:1262-1263` 已定义 `evidenceRole` 对 convergence action 的约束，但 `verification-acceptance-flow.md` §Convergence Mapping 表中未体现。

**处理方案**：在 Convergence Mapping 表中增加：

```markdown
| 缺 `state_change` 但 `evidenceRole=supplemental_evidence` | `need_validation` 或 `need_evidence` | 不得启动 primary state-changing patch；先补充 evidence ledger 或等待 primary target |
```

**涉及文件**：`docs/20-runtime-flow/verification-acceptance-flow.md`

**收口结果**：Convergence Mapping 已补充 `evidenceRole=supplemental_evidence` 的收敛约束，明确不得启动 primary state-changing patch。

---

### P2-02：`verification-acceptance-flow.md` 补充 graph follow-up 扩展上限数值

**状态**：已收口，已验证

**问题**：文档只说"达到扩展上限"，未写明具体数值。

**处理方案**：在 §3 Graph Follow-up Closure 中补充：

```markdown
graph follow-up 扩展上限为 `maxGraphFollowUpExpansionAttempts=2`
（定义于 `runtime_orchestrator.go` 常量）。
```

**涉及文件**：`docs/20-runtime-flow/verification-acceptance-flow.md`

**收口结果**：Graph follow-up closure 已补充 `maxGraphFollowUpExpansionAttempts=2`。

---

## 四、2026-05-12 旧偏差状态跟踪

| 旧偏差 # | 描述 | 当前状态 | 处置 |
|----------|------|----------|------|
| 1 | `dispatchStep` 缺少 rejected result 前置拦截 | 已修复 | 本次 P1-02 已收口 |
| 2 | `created` 状态未拦截直接 result 写入 | 已修复 | formal result closeout 主路径已补 created-run gate；Result owner 仍只维护 outcome 对象，不反向耦合 Run owner |
| 3 | Planning repair 依赖字符串匹配 | 已修复 | 本次 P1-01 已收口；字符串匹配仅保留为旧数据 fallback |
| 4 | `dod_coverage_missing` 在 assessment-only step 中仍可进入 repair | 已修复 | assessment-only coverage correction 失败且仍存在 actionable coverage gap 时，下一轮 repair 会清除 assessment-only，转入 concrete validation evidence / state-changing evidence 路径；新增主入口回归测试覆盖 |
| 5 | `cancelled` 终态文档遗漏 | 已修复 | `runtime-stage-contract.md` 状态桥接图和阶段转换表已补 `cancelled` 终态 |
| 6 | `blockRecoveryActionFromFailedStep` 可读性 | 已修复 | 已补函数注释和回归测试，明确 executor 先持久化 blocker，再返回 `ErrBlocked` 作为派发停止信号 |

---

### 2026-06-22 补充收口：旧偏差 #6

旧偏差指出 `blockRecoveryActionFromFailedStep` 会先变更 run/blocker 状态，
再返回 error 给 dispatch loop，功能正确但控制流不直观。本次保持行为不变，
只把该控制流固化为代码注释和测试合同：

- `blockRecoveryActionFromFailedStep` 注释明确：executor 先持久化选定
  blocker 状态，随后返回 `ErrBlocked` 作为 dispatch-loop stop signal。
- `TestRepairSubsystemBlockRecoveryActionPersistsBlockerBeforeErrBlocked`
  明确断言 executor side effect 发生后才以 `ErrBlocked` 收口。

验证：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run TestRepairSubsystemBlockRecoveryActionPersistsBlockerBeforeErrBlocked -count=1 -timeout=180s
```

---

### 2026-06-22 补充收口：旧偏差 #5

旧偏差指出 `cancelled` 终态在文档状态机表中未独立列出。代码侧已经
通过 `RunStateCancelled`、`CancelRun`、`isTerminalRunState` 和 read model
terminal projection 支持 `cancelled` 终态，本次只补齐权威核心合同：

- `runtime-stage-contract.md` 的 RunState 桥接图已列出
  `cancelled` / `superseded`。
- 阶段流图已把 `cancelled` 列为终态。
- 转换表已明确任意非终态 Run 可通过显式 cancel command 进入
  `cancelled`，且终态不可逆。

验证依据：

```bash
go test ./internal/modules/kernel/run/biz -run TestService_RunStateMachineTransitionsWithoutOrchestrator -count=1 -timeout=180s
```

---

### 2026-06-22 补充收口：旧偏差 #2

旧建议要求在 result 写入 API 增加 run state 校验。按当前
framework/EventHub owner 边界，`kernel/result` 是正式 outcome owner，
不拥有 Run 状态，也不应为了校验直接注入 Run repository 或 Run owner
service。实际收口落点放在 orchestrator 正式 result closeout 主路径：

- `recordRunResultFromDoDAssessment`
- `recordRunResultFromValidatedDoDAssessment`
- `recordRejectedCompletionDoDAssessment`
- `recordAcceptedSliceResultsFromRun`
- `recordCompletionEvidenceFromValidatedDoDAssessment`

这些入口在写入正式 Result/Artifact/Memory evidence 前统一执行
created-run gate。若 run 仍处于 `created`，直接返回 `conflict`，并阻止
completion evidence 并行路径产生 result、artifact 或 memory 副作用。

验证：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_DoesNotRecordCompletionEvidenceForCreatedRun|TestRuntimeOrchestrator_ReusesValidatedDoDAssessmentForCompletionEvidence|TestRuntimeOrchestrator_RecordsRejectedResultForUnsatisfiedDoDAssessment|TestRuntimeOrchestrator_CompletesLLMRunWithSatisfiedDoDAssessment' -count=1 -timeout=180s
```

边界说明：

- Result owner 仍只做 Result/Artifact model 校验与 owner repository 操作。
- Run 状态约束留在 orchestrator closeout 主路径，避免 kernel/result
  跨 owner 读取 Run 状态。
- 该收口不引入模板、语言、provider 或 workspace 特判。

---

### 2026-06-22 补充收口：旧偏差 #4

旧建议要求在 `shouldAttemptRuntimeRepair` 中直接排除
`assessment-only + dod_coverage_missing`。当前实现采用了更细的中立收敛语义：

- 首次 DoD/Goal/deliverable coverage 合同缺失时，允许一次
  assessment-only coverage correction，只补齐结构化 `dodAssessment`。
- 如果该 assessment-only correction 后仍返回 `dod_coverage_missing`
  且存在 actionable coverage gap，则下一轮 repair 输入会清除
  `assessmentOnly` / `assessmentOnlyReason`，不再继续 assessment-only graph。
- 后续 repair 转入 `repairObjective=produce_concrete_validation_evidence`
  或必要的 state-changing evidence 路径，并通过 `repairActionPackage`
  约束 bounded concrete evidence。

验证：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeOrchestrator_RuntimeRepairInput(StopsAssessmentOnlyAfterCoverageCorrectionStillMissing|RequiresValidationEvidenceForMissingDoDWithoutTarget)' -count=1 -timeout=180s
```

对应回归测试：

- `TestRuntimeOrchestrator_RuntimeRepairInputStopsAssessmentOnlyAfterCoverageCorrectionStillMissing`
- `TestRuntimeOrchestrator_RuntimeRepairInputRequiresValidationEvidenceForMissingDoDWithoutTarget`

---

## 五、核心中立性合规

本次核对确认：所有新增代码均保持领域中立。

- coverage work item 使用中立 `source`/`sourceScope`/`evidenceRole` 分类
- repair recovery 决策基于结构化 `failureClass`/`failureSignature`/budget
- graph dispatch 和 evidence 收集基于结构化 node/dependency
- convergence state 映射基于 evidence kind gap

未发现任何软件开发、代码 patch、测试框架或具体业务领域的硬编码逻辑。

---

## 六、Workspace 信息投影数据衔接核对

### 核对范围

对照 `storage-readmodel.md`、`verification-acceptance-flow.md:155`（closure projection 语义一致性）、
`service-api.md` 与 `read_model_service.go` / `read_model_types.go` 的实际投影实现。

### 6.1 投影层架构衔接（已确认一致）

| 设计要求 | 代码实现 | 状态 |
|----------|----------|------|
| Snapshot 分三层：summary / detail / raw | `RunSnapshot`（detail）、`RunObservability`（observability）、paginated pages（detail）、payload ref（raw） | 一致 |
| `WorkspaceSnapshot` 最小结构 | 包含 workspace、outcome、runs、results、artifacts、taskProgress、governance 摘要 | 一致 |
| `RunSnapshot` 最小结构 | 包含 run、outcome、taskProgress、results、artifacts、closureTraces、evidence summary | 一致 |
| `RunObservability` 最小结构 | 包含 timeline、evidence、latestCheckpoint、llmUsageSummary、usedCapabilities | 一致 |
| Workspace-scoped run identity | `selectWorkspaceProjectionRun` 绑定 workspace scope，不跨 workspace 冲突 | 一致 |
| Artifact continuation 投影分层 | workspace summary 只含 metadata；run snapshot 含 continuation metadata；raw 通过 ref 打开 | 一致 |

### 6.2 Outcome 投影衔接（已确认一致）

| 设计要求 | 代码实现 | 状态 |
|----------|----------|------|
| completed Run 没有 accepted Result → incomplete / not accepted | `mapRunOutcome:6633-6636` → `state="failed"`, `reason="run completed without accepted result"` | 一致 |
| accepted Result 优先投影 | `mapRunOutcome:6617-6630` 优先检查 `latestTerminalWorkspaceResult` | 一致 |
| Workspace outcome 复用 run outcome 语义 | `mapWorkspaceOutcome:6703` 调用 `mapRunOutcome(runSnapshot)` | 一致 |
| 终态定义一致 | orchestrator `isTerminalRunState` 与 read model `isTerminalRunState` 均为 completed/failed/cancelled/superseded | 一致 |
| blocked 状态非终态 | `mapRunOutcome` switch 不处理 blocked，保持 `Terminal=false` | 一致 |
| workspace active 无 run → 显式说明 | `mapWorkspaceOutcome:6662-6669` → "workspace definitions changed; execution must be rerun" | 一致 |

### 6.3 TaskProgress 投影衔接（已确认一致）

| 设计要求 | 代码实现 | 状态 |
|----------|----------|------|
| Run-level taskProgress 包含 milestones / blockers / recovery / repairLoop | `mapTaskProgress:4793-4841` 完整填充 | 一致 |
| Workspace-level taskProgress 在有 projection run 时使用 run-level progress | `readWorkspaceSnapshot:1270` → `ret.TaskProgress = runSnapshot.TaskProgress` | 一致 |
| Workspace-level 无 active run 时使用 workspace-level progress | `mapWorkspaceTaskProgress:4711-4791` 独立构建 | 一致 |
| RepairLoop 投影包含 failureClass / failureSignature / missingDoDItems | `RepairLoop` 类型完整定义 | 一致 |

### 6.4 Coverage Work Item 投影衔接

| 设计要求 | 代码实现 | 状态 |
|----------|----------|------|
| run-scoped ledger 持久化 | `coverageWorkItems` / `coverageWorkItemContext` 存储在 `run.ExecutionContext` 中 | 一致 |
| run snapshot 投影 coverage | `readRunSnapshot:861-862` 从 `ExecutionContext` 提取 | 一致 |
| workspace 级只投影 active run 的 coverage summary | `WorkspaceSnapshot.CoverageSummary` 从 active run 的 `CoverageWorkItemContext` 提取 summary / currentFocus / omittedCount / fullLedgerRef | 一致 |
| LLM context bundle 携带 coverage | `contextExecutionContextSection:5622-5635` 将 `ExecutionContext`（含 coverage）传入 context bundle | 一致 |
| eventhub 事件压缩 coverage 大对象 | eventhub 传播时删除 `coverageWorkItems`（测试确认） | 一致 |
| projection 不反向覆盖 run-scoped ledger | read model 只读取，不写回 | 一致 |
| currentFocus / omitted counts / full-ledger ref 保持同一投影语义 | `coverageWorkItemContext` 在 run snapshot 中完整保留，workspace 级通过 run snapshot 间接获取 | 一致 |

### 6.5 发现的衔接缺口（2 条）

#### 缺口 G1：Workspace snapshot 缺少 coverage summary 字段

**状态**：已收口，已验证

**问题**：设计文档 `storage-readmodel.md:229` 要求 "workspace 级只投影 active run 的 coverage summary"。当前实现中 `WorkspaceSnapshot` 类型没有 `CoverageWorkItems` / `CoverageWorkItemContext` 字段。

**实际影响**：
- Web/TUI 通过 workspace snapshot API 无法直接获取 coverage 状态
- 必须额外请求 run snapshot 才能看到 coverage ledger
- `TaskProgress.RepairLoop.MissingDoDItems` 只携带了部分 coverage 信息（仅 repair 阶段的 missing items），不等价于完整 coverage summary

**风险**：中。workspace 级 UI 无法一次性展示 coverage 进度。

**处理指导**：在 `WorkspaceSnapshot` 中增加 coverage summary 投影（不是完整 ledger）：

```go
type WorkspaceSnapshot struct {
    // ... existing fields
    CoverageSummary map[string]any `json:"coverageSummary,omitempty"`
}
```

在 `readWorkspaceSnapshot` 中，当有 projection run 时，从 `runSnapshot.CoverageWorkItemContext` 提取 summary：

```go
if projectionRun != nil {
    // ... existing code
    if ctx := runSnapshot.CoverageWorkItemContext; len(ctx) > 0 {
        ret.CoverageSummary = map[string]any{
            "summary":       ctx["summary"],
            "currentFocus":  ctx["currentFocus"],
            "omittedCount":  ctx["omittedCount"],
            "fullLedgerRef": ctx["fullLedgerRef"],
        }
    }
}
```

**收口结果**：

- `WorkspaceSnapshot` 增加 `coverageSummary` 摘要字段，不暴露完整 ledger。
- `readWorkspaceSnapshot` 从 active projection run 的 `CoverageWorkItemContext` 提取 `summary`、`currentFocus`、`omittedCount`、`fullLedgerRef`。
- `read_model_service_test.go` 增加 workspace snapshot coverage summary 投影回归测试。

#### 缺口 G2：Context bundle 未显式过滤 coverage 大对象

**状态**：已收口，已验证

**问题**：`contextExecutionContextSection` 将 `ExecutionContext` 直接传入 context bundle，只删除了 `goal`、`dod`、`compressedContext`。`coverageWorkItems`（可能很大）会完整进入 LLM context。

**实际影响**：
- 当 coverage work items 数量多时，LLM context 会被大量 ledger 数据占据
- 设计文档要求 "full ledger 默认通过 raw/payload ref 或 detail 打开"
- eventhub 已正确压缩，但 context bundle 未做同等处理

**风险**：中。LLM context budget 可能被 coverage ledger 挤占。

**处理指导**：在 `contextExecutionContextSection` 中删除 `coverageWorkItems`，只保留 `coverageWorkItemContext`（已是 summary 投影）：

```go
func contextExecutionContextSection(snapshot RunSnapshot) ContextSection {
    data := cloneReadModelMap(snapshot.Run.ExecutionContext)
    delete(data, "goal")
    delete(data, "dod")
    delete(data, "compressedContext")
    delete(data, "coverageWorkItems") // full ledger 不进入 LLM context
    return ContextSection{
        // ...
    }
}
```

**收口结果**：

- `contextExecutionContextSection` 删除 `coverageWorkItems`，仅保留摘要型 `coverageWorkItemContext`。
- `read_model_service_test.go` 增加 context bundle 过滤完整 coverage ledger 的回归测试。

### 6.6 投影语义一致性验证

设计文档要求 "Workspace outcome、Run outcome、taskProgress、observability 和 Web/TUI 需要复用同一 closure projection 语义"。

验证结果：

| 投影面 | outcome 来源 | taskProgress 来源 | 一致性 |
|--------|-------------|-------------------|--------|
| Run snapshot | `mapRunOutcome(runSnapshot)` | `mapTaskProgress(runSnapshot)` | 基准 |
| Run observability | `mapRunOutcome(snapshot)` | `mapTaskProgress(snapshot)` | 一致（同一 snapshot） |
| Workspace snapshot | `mapWorkspaceOutcome(wsSnapshot)` → 内部调用 `mapRunOutcome` | `runSnapshot.TaskProgress`（直接复用） | 一致 |
| Context bundle | 通过 `runSnapshotTaskProgress(snapshot)` | 同上 | 一致 |
| TUI/CLI | 消费 workspace snapshot API 返回的 `outcome` / `taskProgress` | 同上 | 一致 |

**结论：closure projection 语义在所有投影面保持一致。**

---

## 七、综合行动项汇总

| 项 | 类别 | 优先级 | 来源 |
|---|------|--------|------|
| P1-01 | `stateChangingRepairRequired` 结构化标记 | P1 | 已收口 |
| P1-02 | `dispatchStep` rejected result 前置拦截 | P1 | 已收口 |
| P1-03 | Context bundle 过滤 `coverageWorkItems` 大对象 | P1 | 已收口 |
| P2-01 | `verification-acceptance-flow.md` evidenceRole 补充 | P2 | 已收口 |
| P2-02 | `verification-acceptance-flow.md` 数值补充 | P2 | 已收口 |
| P2-03 | Workspace snapshot 增加 coverage summary 投影 | P2 | 已收口 |

## 八、收口验证

已执行并通过：

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TestRuntimeContextIntegrityGateFailureIsNotStateChangingRepair|TestStateChangingRepairRequiredUsesStructuredContractViolationType|TestAttachContractViolationTypeClassifiesPatchTargetRef|TestRuntimeOrchestrator_DispatchStepRejectsActiveRejectedResult|TestRuntimeOrchestrator_RejectedResultSupersededByRecoveryProgress' -count=1
go test ./internal/modules/application/workorchd/biz/appservice -run 'TestRunContextBundleFiltersCoverageWorkItemsFullLedger|TestReadWorkspaceSnapshotProjectsCoverageSummaryFromActiveRun|TestRunContextBundleReusesSnapshotTaskProgress|TestReadWorkspaceSnapshotProjectsActiveRunAndCurrentStep' -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
```

---

## 九、下次核对建议

- 下次核对应继续关注新增 runtime contract violation 是否同步写入 `contractViolationType`
- 下次核对应验证 LLM context budget 中 coverage summary 与 full ledger ref 的实际占比
- 下次核对应验证 workspace snapshot coverage summary 与 run-scoped ledger 的一致性
