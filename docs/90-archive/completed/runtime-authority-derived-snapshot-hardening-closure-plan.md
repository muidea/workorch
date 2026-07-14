# Runtime Authority Derived Snapshot Hardening Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-07

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-validation-failure-diagnostic-repair-dispatch-closure-plan.md`
- `runtime-required-output-authority-and-target-snapshot-closure-plan.md`
- `runtime-live-authority-loop-full-chain-closure-plan.md`

## Purpose

本文用于收口 `convergenceAuthority -> convergenceContract -> stagePacket -> requiredOutput -> repair dispatch` 主链路中仍未固化的派生合同问题。

目标是把当前 authority 派生字段收敛为唯一、可预测、可审计的刷新路径，避免 replan、resume、repair follow-up、graph continuation 或 progress merge 将旧 `stagePacket`、旧 `repairActionPackage.requiredOutput`、旧 `runtimeStage` 重新带回当前 dispatch。

当前阶段不考虑兼容性。若现有实现、测试或旧持久化字段与本文冲突，应按本文收口。禁止修改 `docs-product/`。

## Contract Audit

- Scope: `ws-db040d24a190f6e5 / run-089-replan-001` 及同类 replan/resume successor dispatch。
- Observed state:
  - `run-089-replan-001` 进入 `failed`。
  - failureReason 为 `conflict: runtime_internal_contract_misalignment: contractState=need_state_change requires state-changing requiredOutput`。
  - workspace 处于 `active`，但 `activeRunId=null`，没有可继续执行的 current run。
  - live 观察中 CPU/IO 不构成主因，projection ready，问题聚焦在 core orchestrator 合同派生。
- Expected state:
  - 每次 dispatch 前，`convergenceAuthority.nextAction` 必须唯一派生 `convergenceContract`、`stagePacket`、`repairActionPackage.requiredOutput`、top-level `requiredOutput` 和 `currentActionContract`。
  - replan successor 只能继承诊断 lineage、definition、executionContract、ledger/evidence 事实，不能继承 predecessor 的旧派生 action contract。
  - 若缺少 target snapshot 等前置事实，应明确派生为 `acquire_target_snapshot` 或等价前置 action，而不是产生 `need_state_change + evidence acquisition requiredOutput` 的自相矛盾合同。
- Deviation:
  - replan successor 创建时仍从 predecessor clone 完整 `ExecutionContext`，旧派生字段有机会混入 successor。
  - progress merge 的 managed key 集合没有覆盖完整派生字段，旧 `convergenceContract/stagePacket/runtimeStage` 可能残留。
  - repair follow-up finalization 后仍有 projection 函数改写 `repairActionPackage/requiredOutput/stagePacket`，违反 current authority finalization 最后写者原则。
  - target snapshot prerequisite 与 state-changing requiredOutput 之间的前置 action 表达仍不够硬。
- Owner: WorkOrch core orchestrator。
- Event path:
  - decision replan/resume command
  - run owner 创建 successor 或恢复 run
  - coverage ledger / convergence authority refresh
  - stage packet / current action contract render
  - dispatch gate
  - repair dispatch
  - evidence ledger / projection
- Contract gap:
  - 缺少单一的 current authority derived snapshot 刷新入口。
  - 缺少 replan successor 派生字段剥离规则。
  - 缺少 finalization 后禁止再写派生合同的代码结构约束。
  - 缺少 target snapshot prerequisite 的硬状态表达。
- Boundary check:
  - 本计划只处理 runtime 通用状态机、合同派生和 dispatch gate。
  - 不写模板 ID、Soul ID、workspace ID、语言、命令、文件名、provider 或 live case 特判。
- Verification:
  - 新增失败样本单测覆盖 replan successor、progress merge、finalization order、target snapshot prerequisite 和 dispatch gate。
  - 执行受影响包测试、fmt、diff check。
  - live 验证同类 workspace 不再出现 `contractState=need_state_change requires state-changing requiredOutput` 或 `failed + no activeRunId + no blocker`。

## Current Findings

### 1. Replan Successor 创建期复制旧派生字段

当前 `runtime_run_state.go` 中 `replanLoadedRun` 会 clone predecessor `ExecutionContext` 作为 successor context。虽然后续会重算部分合同，但 clone 后仍存在旧派生字段污染窗口。

需要剥离的 predecessor 派生字段包括：

- `convergenceContract`
- `stagePacket`
- `currentActionContract`
- `repairActionPackage`
- top-level `requiredOutput`
- `runtimeStage`
- `runtimeStageMode`
- `soulStage`
- `repairObjective`
- `stateChangingOutputRequired`
- `boundedValidationRequired`
- `evidenceRepairRequired`
- `concreteFailureEvidenceRequired`
- `assessmentOnly`
- `assessmentOnlyReason`
- `coverageRepairRequired`
- `coverageTargetSet`
- `governanceRequired`
- `governanceReason`

允许继承的数据只应是事实或 lineage：

- workspace definition / executionContract
- coverage ledger / evidence ledger facts
- accepted/rejected evidence refs
- target snapshot facts
- governance history compact lineage
- replanContinuation compact diagnostic lineage
- dialog / language / soul routing 等非 action contract 字段

### 2. Progress Merge 管理字段不完整

`managedRunProgressExecutionContextKeys()` 当前没有完整覆盖 `convergenceDerivedExecutionContextKeys()`，导致进度更新时若新 progress 未携带某些派生字段，旧字段可能留在 run execution context。

应将 managed progress keys 与 derived keys 对齐，至少覆盖：

- `convergenceContract`
- `stagePacket`
- `runtimeStage`
- `runtimeStageMode`
- `soulStage`
- `repairObjective`
- `stateChangingOutputRequired`
- `boundedValidationRequired`
- `evidenceRepairRequired`
- `concreteFailureEvidenceRequired`
- `assessmentOnly`
- `assessmentOnlyReason`
- `currentActionContract`
- `repairActionPackage`
- `requiredOutput`

### 3. Finalization 后仍有函数改写当前合同

`finalizeRuntimeRepairFollowUpStepInput` 中存在“finalization 后继续 apply projection / attach packet”的结构。按设计，current authority finalization 必须是最后写者。

收口后顺序应固定为：

```text
repair/validation/diagnostic projection
  -> output mode selection
  -> convergence authority rebuild
  -> requiredOutput mirror sync
  -> currentActionContract attach
  -> dispatch validation
```

最后一步之后不得再调用会修改下列字段的函数：

- `convergenceContract`
- `stagePacket`
- `repairActionPackage.requiredOutput`
- `requiredOutput`
- `currentActionContract`
- `runtimeStage`
- `runtimeStageMode`
- `repairObjective`
- `stateChangingOutputRequired`

### 4. Target Snapshot Prerequisite 表达不够硬

当 current focus 需要 state change，但 runtime 缺少可执行目标快照或当前目标事实时，不应表达为：

```text
convergenceContract.state = need_state_change
requiredOutput = evidence acquisition / validation-only
```

应表达为以下二选一：

```text
nextAction = acquire_target_snapshot
convergenceContract.state = need_evidence
requiredOutput = read-only target snapshot acquisition
```

或：

```text
nextAction = state_change
convergenceContract.state = need_state_change
convergenceContract.prerequisite.kind = acquire_target_snapshot
requiredOutput = read-only target snapshot acquisition
```

两种形态必须由 validator 明确允许；除此之外的 `need_state_change + non-state-changing requiredOutput` 必须在 dispatch 前被重建或拒绝。

## Closure Strategy

本轮收口只处理主链路，不扩大到 provider、TUI/Web、template/Soul 或业务验证策略。

修复顺序必须保持单点推进：

1. 固化 replan successor 派生字段剥离。
2. 固化 current authority derived snapshot 单一刷新入口。
3. 固化 progress merge 派生字段删除语义。
4. 固化 repair follow-up finalization 最后写者原则。
5. 固化 target snapshot prerequisite 的状态表达。
6. 补齐失败样本测试与 live 验证。

不得使用 run recovery finalization 兜底；状态迁移必须在 owner state、authority refresh、dispatch gate 中可控发生。

## Phase 1: Replan Successor Strip

- [ ] 新增 `stripDerivedActionContractForReplanSuccessor(context)` 或等价函数。
- [ ] 在 `replanLoadedRun` clone predecessor `ExecutionContext` 后立即剥离旧派生字段。
- [ ] 保留 `replanContinuation` 仅作为 diagnostic lineage，不作为 current action authority。
- [ ] successor 创建前必须调用 current authority derived snapshot 刷新。
- [ ] 如果刷新失败，不应创建 failed successor 并 supersede predecessor；应保持原 governed blocker 或返回 scoped conflict。

验收：

- [ ] 单测：predecessor 为 `blocked_on_decision + stagePacket=request_governance`，replan successor 不继承该 `stagePacket`。
- [ ] 单测：successor `stagePacket.runId` 必须等于 successor run id。
- [ ] 单测：successor `currentActionContract.requiredOutput` 与 active requiredOutput 同源。

## Phase 2: Single Derived Snapshot Entry

- [ ] 新增或固化统一入口，例如 `refreshCurrentAuthorityDerivedSnapshot(run, stepID, input)`。
- [ ] 该入口必须顺序执行：
  - refresh coverage ledger / convergence authority
  - build convergence contract
  - apply convergence contract
  - normalize authority focus
  - attach stage packet
  - synchronize requiredOutput mirrors
  - attach current action contract
  - enforce current authority derived consistency
  - validate convergence dispatch input
- [ ] 所有 LLM dispatch、non-LLM graph continuation、resolved decision resume、replan successor initial dispatch 都必须通过该入口。
- [ ] 旧 input 只能作为 diagnostic lineage，不能决定当前 contract shape。

验收：

- [ ] 单测：`convergenceAuthority.nextAction=state_change` 时，刷新后 `convergenceContract.state=need_state_change`。
- [ ] 单测：刷新后 `stagePacket.nextAction.type=state_change`。
- [ ] 单测：刷新后 `repairActionPackage.requiredOutput.mustIncludeStateChangingNode=true`。
- [ ] 单测：刷新后 top-level `requiredOutput`、`stagePacket.requiredOutput`、`currentActionContract.requiredOutput` 内容一致。

## Phase 3: Progress Merge Derived Keys

- [ ] 将 `managedRunProgressExecutionContextKeys()` 与 `convergenceDerivedExecutionContextKeys()` 对齐。
- [ ] progress update 缺失某个派生字段时，必须删除 current run 中对应旧字段。
- [ ] 避免旧 `convergenceContract/stagePacket/runtimeStage` 在 run update 后残留。

验收：

- [ ] 单测：current run 带旧 `stagePacket/request_governance`，progress update 不带该字段时，merge 后旧字段被删除。
- [ ] 单测：progress update 带新的 state-changing derived snapshot 时，merge 后所有镜像同源。

## Phase 4: Finalization Last Writer

- [ ] 重排 `finalizeRuntimeRepairFollowUpStepInput`。
- [ ] 所有 repair/validation/command-contract projection 必须在 authority finalization 前完成。
- [ ] authority finalization 后不得再调用可能修改派生合同的 projection 或 attach 函数。
- [ ] 去除重复 attach stage packet / apply command contract projection 的交错调用。

验收：

- [ ] 单测：validation command contract projection 不能在 finalization 后覆盖 state-changing requiredOutput。
- [ ] 单测：state_change repair finalization 后 `validateConvergenceDispatchInput` 通过。
- [ ] 单测：finalization 后再次读取 active requiredOutput 不会变回 validation-only/evidence-only。

## Phase 5: Target Snapshot Prerequisite Contract

- [ ] 当缺少 current target snapshot 且 current action 仍需要 state change 时，派生明确 prerequisite action。
- [ ] 允许的合同形态仅为：
  - `need_evidence + acquire_target_snapshot + read-only snapshot requiredOutput`
  - `need_state_change + prerequisite.acquire_target_snapshot + read-only snapshot requiredOutput`
  - `need_state_change + state-changing requiredOutput`
- [ ] 禁止 `need_state_change + generic evidence acquisition requiredOutput`。
- [ ] dispatch gate 对上述形态做硬校验。

验收：

- [ ] 单测：缺 target snapshot 时不会产生 `contractState=need_state_change requires state-changing requiredOutput`。
- [ ] 单测：snapshot acquisition accepted 后下一轮回到 `state_change + state-changing requiredOutput`。

## Phase 6: Dispatch Outcome Guard

- [ ] internal contract misalignment 不得直接落为 `run failed + no blocker + activeRunId=null`。
- [ ] 对可由 current authority 重建修复的 mismatch，先重建再 dispatch。
- [ ] 对重建后仍不一致的 invariant violation，形成明确 core blocker 或保持 governed blocker，不伪装为业务验证失败。
- [ ] 不使用 terminal recovery finalization 兜底。

验收：

- [ ] 单测：rebuild 可修复 mismatch 时不创建 terminal failed run。
- [ ] 单测：rebuild 后仍 mismatch 时进入明确 blocker/decision，而不是 no-active-run failed 状态。
- [ ] live 验证：同一 workspace 不再出现 `activeRunId=null` 且最新 run 因 internal contract misalignment failed。

## Phase 7: Regression Verification

- [ ] `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- [ ] `make fmt-check`
- [ ] `git diff --check`
- [ ] 边界搜索确认 core 未新增模板、语言、命令、workspace/provider 特判。
- [ ] live 验证至少覆盖：
  - replan successor 创建
  - resolved decision resume
  - validation failed diagnostic
  - state-changing repair dispatch
  - target snapshot prerequisite

## Done Definition

完成后，主链路必须满足：

```text
coverage/evidence ledger
  -> convergenceAuthority
  -> convergenceContract
  -> stagePacket
  -> requiredOutput mirrors
  -> currentActionContract
  -> dispatch gate
  -> repair dispatch
```

并且满足：

- 所有派生字段来自同一次 current authority snapshot。
- replan successor 不继承 predecessor 的 action contract。
- progress merge 不保留旧派生字段。
- finalization 是最后写者。
- target snapshot prerequisite 有明确 action 表达。
- 不再出现 `contractState=need_state_change requires state-changing requiredOutput`。
- 不再出现 `run=failed + activeRunId=null + no blocker` 的可恢复合同错配终态。

## Non-Goals

- 不修改 `docs-product/`。
- 不处理具体 workspace 的业务代码质量问题。
- 不加入 template、Soul、语言、命令、文件名或 provider 特判。
- 不通过扩大超时、降低验证强度或放宽 DoD 解决合同错配。
- 不使用 run recovery finalization 兜底状态迁移。
