# Runtime Validation Command Contract Authority Closure Plan

## 背景

近期针对 `validation command contract failure` 已进行多轮修复，但 live 验证仍出现验证命令合同失败后被升级为 state-changing repair 的偏差。典型表现是：验证 shell command 被 runtime 拦截为 `validation_command_invalid` / `command_contract` / `shell_validation_contract_rejected` 后，后续 repair input 或 current action 又要求 `patch/apply`、`git/write` 或 `mustIncludeStateChangingNode=true`，导致 LLM 进入业务代码修改路径，最终因无效 patch 或合同漂移耗尽重试。

本计划用于将该类失败固化为单一权威路径：只能进入 bounded validation-only retry，或在重试耗尽后进入明确 blocker/terminal；不得被任何旧 state-changing projection、repairActionPackage、convergence contract、currentAction 或 StagePacket 覆盖。

## Contract Audit

- Scope: validation shell command 被 runtime 拦截为 command contract failure 后的自动恢复链路。
- Observed state:
  - diagnostic 能被 `runtime_validation_diagnostic.go` 识别为 `validation_command_invalid`。
  - convergence 层已有 `validationContractFailureMustRemainValidation` 局部拦截。
  - live 中仍出现 repair input/current action 要求 state-changing output，进而诱导 LLM 输出 patch/apply。
- Expected state:
  - `validation_command_invalid`、`recommendedEvidenceTargetType=command_contract`、`stage=shell_validation_contract_rejected`、`commandPolicyViolation=true` 任一命中时，当前恢复动作必须是 validation-only。
  - requiredOutput 必须满足：
    - `mustIncludeStateChangingNode=false`
    - `mustProduceConcreteValidationEvidence=true`
    - `acceptedKinds=["shell/command","mcp/tool_call","validation/managed_session"]`
    - `forbiddenKinds` 包含 `patch/apply` 和 `git/write`
  - currentAction、repairActionPackage、convergenceContract、stagePacket 必须一致。
- Deviation:
  - repair input 构造阶段仍可能先生成 state-changing projection。
  - repairActionPackage 默认从 `stateChangingRepairRequiredOutput()` 起步，存在先 state-changing 再局部降级的漂移窗口。
  - finalize/currentAction/stagePacket 缺少最终一致性断言。
- Owner: core orchestrator。
- Event path:
  - shell/command validation failure -> validation diagnostic -> recoveryAction/runtimeRepair -> runtimeRepairFollowUpStepInput -> BuildConvergenceContract -> ApplyConvergenceContract -> currentAction/stagePacket -> LLM input -> execution contract validation。
- Contract gap:
  - 缺少贯穿全链路的 `validation command contract failure authoritative route`。
- Boundary:
  - 规则只能基于结构化 diagnostic 字段。
  - 禁止在 core 中基于模板名、语言、命令文本、文件名或具体业务场景做特判。

## 收口目标

1. command contract failure 的 owner 归一为 core orchestrator validation diagnostic authority。
2. repair input、convergence、repairActionPackage、currentAction、StagePacket 五处输出同一 validation-only 合同。
3. 任意旧 state-changing context、stale nextAction、target snapshot、patch context、coverage state-change hint 都不能覆盖该合同。
4. 未耗尽重试时继续 bounded validation-only；耗尽后进入 managed harness 或明确 blocker/terminal，不再转入 state-changing repair。
5. 新增单测覆盖 live-like 失败链路，防止后续循环修复。

## P0 收口项

### P0-1 统一 command contract failure classifier

落点：

- `internal/modules/kernel/orchestrator/biz/runtime_validation_diagnostic.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`

新增或统一 helper：

```go
func validationCommandContractFailure(input map[string]any, pkg map[string]any, failedStep *runModel.Step) bool
```

判定来源：

- `validationFailureDiagnosticFromStep(failedStep, execErr)`
- `validationFailureDiagnosticCandidates(input, pkg, failedStep)`
- failedStep input/output 中的 `validationFailureDiagnostic`
- `runtimeDiagnostics.validationFailureDiagnostic`
- `lastFailureDiagnostics.validationFailureDiagnostic`
- `currentExecutionProgress.lastObservedFailure.validationFailureDiagnostic`

命中条件：

- `class == validation_command_invalid`
- `recommendedEvidenceTargetType == command_contract`
- `stage == shell_validation_contract_rejected`
- `commandPolicyViolation == true`

验收：

- 所有相关路径只调用该 helper 或其底层同源 helper。
- 不新增文本 contains 业务特判；已有文本 fallback 可保留为兼容观察，但不能作为新主路径。

### P0-2 repair input 最早硬门禁

落点：

- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`

在 `runtimeRepairFollowUpStepInput` 中，`applyRuntimeRepairFailureProjection(input, failedStep)` 后立即计算：

```go
commandContractValidationRetry := validationCommandContractFailure(input, nil, failedStep)
```

若命中：

- 设置：
  - `repairObjective=produce_concrete_validation_evidence`
  - `boundedValidationRequired=true`
  - `evidenceRepairRequired=true`
  - `validationCommandContractRetry=true`
  - `validationFailureDiagnostic=<normalized diagnostic>`
- 删除：
  - `stateChangingOutputRequired`
  - `stateChangingEvidenceRepairRequired`
  - `validationRepairEscalatedToImplementation`
  - `emptyWorkspaceEvidenceEscalatedToImplementation`
  - `concreteFailureEvidenceRequired`
  - stale state-changing requiredOutput
- 强制：
  - `repairActionPackage.requiredOutput = concreteValidationEvidenceRequiredOutputWithScratch(...)`

后续必须用 `commandContractValidationRetry` guard 住：

- `applyRuntimeRepairValidationEscalationProjection`
- `applyRuntimeRepairEmptyWorkspaceStateChangingProjection`
- `applyRuntimeRepairStateChangingEvidenceQualityProjection`
- `stateChangingRepairRequired(...)`
- `applyRuntimeRepairStateChangingActionPackageProjection`
- `applyRuntimeRepairStateChangingTargetEvidenceGate`

验收：

- 即使 failedStep/input 中存在 stale `stateChangingOutputRequired=true`，最终 input 也必须 validation-only。

### P0-3 repairActionPackage builder 不再默认抢占 state-changing

落点：

- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`

当前 `repairActionPackageFromRepairInput` 默认使用 `stateChangingRepairRequiredOutput()`。需要在函数开头识别 command contract failure：

- 命中时直接构建 validation-only package。
- 不调用 `stateChangingRepairRequiredOutput()`。
- 不追加 `patch/apply`、`git/write` 到 acceptedKinds。
- `outputDecisionContract.requiredMode=produce_concrete_validation_evidence`。

验收：

- `repairActionPackageFromRepairInput` 单测证明 command contract failure 不会生成 patch/apply 合同。

### P0-4 convergence 层最终权威门禁

落点：

- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`

在 `populateConvergenceContractFromNextAction` 的入口或各分支前增加最高优先级判断：

- 若 `validationCommandContractFailure(input, contract.RepairActionPackage, failedStep)` 命中：
  - `State=need_validation`
  - `RuntimeStage=verification`
  - `RuntimeStageMode=strict_convergence`
  - `RepairObjective=produce_concrete_validation_evidence`
  - `StateChangingOutputRequired=false`
  - `RepairActionPackage.requiredOutput=concreteValidationEvidenceRequiredOutputWithScratch(...)`
  - 返回 true

覆盖分支：

- `state_change`
- `produce_state_change`
- `repair`
- `acquire_evidence`
- stale authority from executionContext

验收：

- stale `convergenceAuthority.nextAction=state_change` 不能覆盖 command contract failure。

### P0-5 finalize/currentAction/stagePacket 一致性断言

落点：

- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`

在 `finalizeRuntimeRepairFollowUpStepInput` 的 `normalizeCurrentActionContract` 和 `attachStagePacketToStepInput` 前后加入最终 sanitizer：

- 若 command contract failure 命中：
  - currentAction.nextAction 必须是 `validate` 或 `produce_validation_evidence`
  - requiredOutput 必须 validation-only
  - repairActionPackage.requiredOutput 必须 validation-only
  - convergenceContract.state 必须 `need_validation`
  - stagePacket.nextAction 必须与 currentAction 一致

如果发现 state-changing 字段残留：

- 优先重写为 validation-only。
- 若无法重写，输出 internal contract drift diagnostic，不发送给 LLM 执行。

验收：

- `finalizeRuntimeRepairFollowUpStepInput` 后不存在：
  - `stateChangingOutputRequired=true`
  - `mustIncludeStateChangingNode=true`
  - acceptedKinds 包含 `patch/apply`
  - acceptedKinds 包含 `git/write`

### P0-6 NextAction 未耗尽/耗尽路径固化

落点：

- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`

`validationNextActionRouteForRejectedDiagnostics` 对 command contract failure 应明确：

- 未耗尽：返回 validation route。
- managed harness 可用且达到策略条件：返回 managed harness validation route。
- 耗尽且无 managed harness：返回 fail/blocker route，不转 state_change。
- `validationNoDeltaRejectedDiagnosticCountForCoverageItem` 不得把 command contract failure 计入 state-change escalation。

验收：

- repeated `validation_command_invalid` 不会生成 `NextActionTypeStateChange`。

## P1 收口项

### P1-1 recoveryAction 耗尽语义

落点：

- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`

要求：

- command contract validation retry 耗尽后进入明确 blocker/terminal。
- 不允许 `run=running + currentStep=failed + no blocker`。
- 若 policy 存在 managed harness，则进入 managed harness；否则生成可投影的 blocker/failure。

验收：

- retry exhausted 后 run snapshot 有明确 terminal/blocker reason。

### P1-2 projection/readmodel 展示一致性

落点：

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`

要求：

- current action 展示为 validation-only。
- 不展示 stale state-changing package。
- rejected diagnostic 能看到 `validation_command_invalid` 和 command contract reason。

验收：

- status/snapshot 中 currentStep、currentAction、blocker/terminal reason 一致。

## P2 收口项

### P2-1 live-like fixture

构造 fixture：

1. 一个 validation shell command 被 runtime 拒绝：
   - `validationFailureDiagnostic.class=validation_command_invalid`
   - `recommendedEvidenceTargetType=command_contract`
   - `stage=shell_validation_contract_rejected`
   - `commandPolicyViolation=true`
2. failedStep input 携带 stale state-changing context：
   - `stateChangingOutputRequired=true`
   - `repairActionPackage.requiredOutput.mustIncludeStateChangingNode=true`
   - `targetSnapshots` 存在
3. 调用 runtime repair follow-up input。

断言：

- final input 是 validation-only。
- currentAction 是 validation-only。
- stagePacket 是 validation-only。
- repairActionPackage outputDecisionContract 是 `produce_concrete_validation_evidence`。

### P2-2 live 验证

验证目标：

- 触发同类 validation command contract failure 后，下一步不是 patch/apply。
- LLM request C4.current-action 中：
  - acceptedKinds 只有 validation allowed kinds。
  - forbiddenKinds 包含 `patch/apply`、`git/write`。
  - requirements 中没有 state-changing required wording。
- 若 LLM 继续输出 patch/apply，runtime 拒绝为合同违例，但下一轮仍保持 validation-only 或进入明确 blocker，不再升级到 state-changing repair。

## 非目标

- 不修改 `docs-product/`。
- 不改 workspaceTemplate/Soul/policy 来规避该问题。
- 不引入 Go、Node、Python、具体命令、具体文件名或模板 ID 特判。
- 不和 external dependency、provider timeout、target snapshot acquisition 其他收口项混在同一补丁中。

## 推荐执行顺序

1. P0-1：统一 classifier。
2. P0-2：repair input 最早硬门禁。
3. P0-3：repairActionPackage builder validation-only 分支。
4. P0-4：convergence 最终权威门禁。
5. P0-5：finalize/currentAction/stagePacket 一致性断言。
6. P0-6：NextAction 未耗尽/耗尽路径固化。
7. P1/P2：补 projection 与 live-like fixture。

每完成一项只跑对应单测；P0 全部完成后再跑 orchestrator biz 包测试和 live 验证，避免多环节并发修改导致循环修复。
