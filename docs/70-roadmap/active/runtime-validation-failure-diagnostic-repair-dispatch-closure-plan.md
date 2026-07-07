# Runtime Validation Failure Diagnostic Repair Dispatch Closure Plan

## 背景

最新 live 验证中，`ws-db040d24a190f6e5 / run-087` 在 `go build ./...` 验证失败后，未稳定进入下一轮可控修复，而是以 `run=failed + currentStep=failed + no blocker` 结束。

关键失败信息：

- 当前失败 step：`run-087/graph-004/001-verify-go-build`
- step kind：`shell/command`
- failureClass：`shell_command_failed`
- diagnostic：`validation_build_failed`
- 最终 run failureReason：`runtime repair dispatch failed: conflict: runtime_internal_contract_misalignment: contractState=need_state_change requires state-changing requiredOutput`

这说明 `validation failed diagnostic -> coverage ledger recompute -> convergenceAuthority nextAction -> repairActionPackage requiredOutput -> repair dispatch` 主链路仍存在合同错配：coverage 已经要求 state-changing 修复，但 repair dispatch 仍可能沿用 validation-only 合同。

## Contract Audit

- Scope：`ws-db040d24a190f6e5 / run-087 / run-087/graph-004/001-verify-go-build`
- Observed state：验证 step 失败后，runtime 写入 rejected result，随后 repair dispatch 因 `contractState=need_state_change` 与 requiredOutput 不一致失败，最终 run terminal failed，且无 blocker/decision。
- Expected state：验证失败应先记录 rejected validation diagnostic，再由 coverage ledger 重算唯一 nextAction；若诊断可通过代码、依赖、配置或文件修改修复，应进入 state-changing repair；若只缺验证，应进入 validation repair；若预算耗尽，应进入 decision/blocker。
- Deviation：validation failure 后的 repair dispatch 未完全使用当前 ledger 派生的 authority contract，导致旧 validation contract 与新的 `need_state_change` 状态发生冲突。
- Owner：workorch core orchestrator。
- Event path：`shell/command validation failed -> validationFailureDiagnostic -> rejected evidence/result -> coverage ledger recompute -> convergenceAuthority -> stagePacket -> repairActionPackage.requiredOutput -> repair dispatch`
- Contract gap：repair dispatch 仍允许失败 step 的旧 stage/requiredOutput/repairObjective 参与派生，未强制以当前 coverage ledger 和 convergenceAuthority 作为唯一事实源。
- Proposed change：固化 validation failure diagnostic 的分类、ledger 重算、authority 派生、requiredOutput 构造与 dispatch 前不变量校验。
- Boundary check：只处理 runtime 通用状态机和合同派生，不加入模板、语言、命令输出 marker 或具体项目文件特判。
- Verification：新增失败样本单测，覆盖 validation failure 后 state-changing repair 派生、旧 validation requiredOutput 被拒绝/重建、预算耗尽进入 blocker、禁止 recoverable diagnostic 直接 terminal failed no blocker。

## 收口目标

1. validation command failure 不直接终止 run。
2. validation failure diagnostic 必须成为 rejected diagnostic evidence，并绑定 coverage focus。
3. coverage ledger 必须在 diagnostic 写入后重算当前唯一 gap。
4. convergenceAuthority 是后续 stagePacket、requiredOutput、repair dispatch 的唯一来源。
5. repair dispatch 前必须校验 `contractState / nextAction / requiredOutput` 强一致。
6. 可自动修复时进入 state-changing repair；可自动验证时进入 validation repair；预算耗尽时进入 decision/blocker。
7. 禁止 recoverable validation diagnostic 路径出现 `run=failed + currentStep=failed + no blocker`。

## Phase 1: Validation Failure Diagnostic 合同固化

- [ ] 将 `shell/command` 验证失败稳定归类为 rejected validation diagnostic，而不是普通 terminal failure。
- [ ] 确保 diagnostic 包含：
  - `failureClass`
  - `failureSignature`
  - `validationFailureDiagnostic.class`
  - `recommendedEvidenceTargetType`
  - `coverageWorkItemIds`
  - `structuredTargetRefs`
  - `failureTargets`
  - `stderrPreview/stdoutPreview`
- [ ] diagnostic 只能作为 rejected evidence 进入 ledger，不得作为 accepted validation evidence。
- [ ] 对缺依赖、缺文件、构建失败、测试失败等可行动诊断，标记为可自动收敛输入。

## Phase 2: Coverage Ledger Recompute 固化

- [ ] validation failure diagnostic 写入后，必须触发 current focus coverage recompute。
- [ ] 若当前 focus 仍缺 `state_change`，或 diagnostic 指向需要修改生产文件、依赖、配置、入口或交付物，则 convergence state 必须派生为 `need_state_change`。
- [ ] 只有当 state_change 已被 accepted，且仅缺可执行验证证据时，才允许派生为 `need_validation`。
- [ ] rejected diagnostic 的 target refs 必须参与当前 focus binding，避免下一轮 repair 丢失失败面。
- [ ] 禁止 stale acceptance / stale slice 覆盖当前 authority focus。

## Phase 3: ConvergenceAuthority 单一事实源固化

- [ ] validation failure 后的后续 dispatch input 必须从当前 ledger 重建 `convergenceAuthority`。
- [ ] 禁止直接复用失败 step 的以下旧字段作为当前权威：
  - `runtimeStage`
  - `runtimeStageMode`
  - `repairObjective`
  - `stagePacket`
  - `requiredOutput`
  - validation-only `repairActionPackage`
- [ ] `convergenceAuthority.nextAction` 必须唯一决定：
  - `convergenceContract.state`
  - `stagePacket.nextAction`
  - `repairActionPackage.repairObjective`
  - `repairActionPackage.requiredOutput`
- [ ] 若 graph continuation、follow-up、contract retry 带有旧本地 authority，必须在 dispatch 前重建或拒绝。

## Phase 4: RequiredOutput 派生规则固化

- [ ] 当 `nextAction=state_change`：
  - `acceptedKinds` 必须包含 `patch/apply`
  - 可包含声明了 state-changing metadata 的 `shell/command`、`mcp/tool_call`
  - `mustIncludeStateChangingNode=true`
  - `mustBindCoverageFocus=true`
  - `mustDeclareCoverageTargets=true`
  - validation-only acceptedKinds 不得覆盖 state-changing contract
- [ ] 当 `nextAction=validate`：
  - `acceptedKinds` 只能包含 validation 能力，如 `shell/command`、`mcp/tool_call`、`validation/managed_session`
  - `patch/apply` 不得进入 validation-only requiredOutput
  - `mustProduceConcreteValidationEvidence=true`
  - `mustDeclareValidationEvidenceMode=true`
  - `mustDeclareValidationEvidenceObservation=true`
- [ ] requiredOutput 中的 coverage focus、targetRefs、structuredTargetRefs 必须来自当前 authority focus。

## Phase 5: Repair Dispatch Gate 固化

- [ ] dispatch 前增加硬不变量校验：
  - `contractState=need_state_change` 时，requiredOutput 必须是 state-changing contract
  - `contractState=need_validation` 时，requiredOutput 必须是 validation contract
  - `stagePacket.nextAction == convergenceAuthority.nextAction`
  - `repairActionPackage.requiredOutput == stagePacket.requiredOutput`
  - `repairActionPackage.coverageFocus == convergenceAuthority.currentFocus`
- [ ] 若不变量不一致，先按当前 ledger/authority 重建 dispatch input。
- [ ] 重建后仍不一致，应标记为 core internal invariant violation，并进入明确 blocker 或测试失败路径，不得伪装成业务验证失败。
- [ ] repair dispatch 不得把 internal contract misalignment 直接转换成 terminal run failed no blocker。

## Phase 6: Recovery Outcome 固化

- [ ] validation failed diagnostic 后只允许以下结果：
  - 自动 state-changing repair step
  - 自动 validation repair step
  - retry budget exhausted 后 decision/blocker
  - core invariant violation blocker
- [ ] budget exhausted 时必须创建 ticket/decision，并写明：
  - failedStepId
  - failureClass
  - failureSignature
  - rejected diagnostic refs
  - allowedRecoveryActions
- [ ] 对 recoverable diagnostic，禁止直接进入 `run=failed + no blocker`。

## Phase 7: Projection / Status 一致性

- [ ] run status、snapshot、projection 必须一致展示：
  - 当前 run state
  - currentStep
  - latest diagnostic
  - blocking ticket / approval
  - next automatic repair step
- [ ] 若 run 处于 failed，必须能解释 terminal 原因；若可恢复，则不得 terminal failed。
- [ ] live 验证中 status endpoint 不得长期出现 `active + failed currentStep + no blocker + no runtimeExecution pending`。

## Phase 8: 回归验证

- [ ] 单测：validation command failed with actionable diagnostic -> ledger 派生 `need_state_change`。
- [ ] 单测：`need_state_change` -> requiredOutput 包含 `patch/apply` 且 `mustIncludeStateChangingNode=true`。
- [ ] 单测：旧 validation requiredOutput 不能通过 state_change repair dispatch。
- [ ] 单测：dispatch gate 检测到 drift 后按当前 authority 重建 input。
- [ ] 单测：重复 validation failure 且预算耗尽 -> decision/blocker。
- [ ] 单测：recoverable validation diagnostic 不允许 terminal failed no blocker。
- [ ] live 验证：同一 workspace 在 build/test validation 失败后，应自动进入唯一 state-changing repair 或明确 blocker。

## 非目标

- 不通过 run recovery finalization 兜底修复状态。
- 不在 core 中硬编码具体语言、模板、依赖包、命令输出 marker 或目标项目文件名。
- 不通过扩大超时、降低验证强度或弱化 DoD 来绕过合同错配。
- 不修改 `docs-product/`。

## 完成判定

完成后，针对 validation failure diagnostic 主链路必须满足：

```text
validation failed
  -> rejected diagnostic evidence
  -> coverage ledger recompute
  -> convergenceAuthority.nextAction
  -> stagePacket
  -> repairActionPackage.requiredOutput
  -> dispatch gate
  -> state-changing repair | validation repair | decision/blocker | invariant blocker
```

且不再出现：

```text
contractState=need_state_change + validation-only requiredOutput
run=failed + currentStep=failed + no blocker
旧失败 step requiredOutput 覆盖当前 authority
```
