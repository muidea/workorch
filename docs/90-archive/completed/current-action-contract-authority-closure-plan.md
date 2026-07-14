# Current Action Contract Authority Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-contract-retry-convergence-closure-plan.md`
- `current-contract-lineage-isolation-and-assessment-recompute-closure-plan.md`
- `resolved-decision-continuation-owner-state-closure-plan.md`
- `runtime-decision-eligibility-narrow-retry-closure-plan.md`

## Purpose

本文用于收口最新 live 核对暴露的核心偏差：contract violation forced-narrow retry 已经要求 `bounded_validation_evidence`，但同一次 LLM dispatch input 中仍残留 `need_state_change`、`stateChangingOutputRequired=true`、`patch/apply` 和 `mustIncludeStateChangingNode=true` 等 state-changing 合同，导致 LLM 收到互相冲突的当前动作要求并持续违例。

后续代码收口必须按本文逐项推进，避免继续围绕某个 workspace、某条 prompt 或某次 LLM 输出做局部补丁。当前阶段不考虑兼容性；若现有实现、测试或端侧行为与本文冲突，应按本文收口。

## Contract Audit

- Scope:
  - `ws-4e83810542ac4d65/run-006/contract-retry-001-of-planning-001-5`
  - `ws-7ced70e98ea50c43/run-005/contract-retry-001-of-planning-001-7`
  - Runtime repair follow-up input rendering、convergence contract、stage packet、LLM current action prompt。
- Observed state:
  - 两个 workspace 均进入 `waiting_decision`。
  - blocking ticket 均为 `concrete_validation_contract_missing`。
  - 最新 retry response 均返回 `file/read` read-only graph，并被 runtime 拒绝。
  - 最新 input 顶层 `requiredOutput` 已被 forced-narrow 为 `bounded_validation_evidence`，但 `repairActionPackage.requiredOutput` 与 `stagePacket.requiredOutput` 仍要求 state-changing output。
- Expected state:
  - forced-narrow retry 中 `contractViolationRecovery.correctionContract.requiredOutputShape=bounded_validation_evidence` 时，当前 action contract 应唯一且一致。
  - `requiredOutput`、`repairActionPackage.requiredOutput`、`stagePacket.requiredOutput`、`convergenceContract` 和 C4 current-action prompt 必须表达同一当前动作。
  - `contractViolationRecovery.expectedContract` 只能作为 rejected diagnostic lineage，不得再次成为 LLM 需要满足的权威合同。
- Deviation:
  - 顶层 requiredOutput 与 package/stagePacket requiredOutput 分裂。
  - `convergenceContract.state=need_state_change` 与 validation-only requiredOutput 同时存在。
  - Prompt 中仍存在要求满足 `contractViolationRecovery.expectedContract` 的表达。
  - dispatch 前 semantic gate 未拦截这类合同冲突。
- Owner: WorkOrch core。
- Event path:
  - failed LLM output -> contract violation recovery projection -> convergence build/apply -> output mode selection -> stage packet attach -> LLM request rendering -> contract gate -> rejected diagnostic evidence -> decision escalation。
- Contract gap:
  - 缺少最终 `CurrentActionContract` 归一化入口。
  - 缺少 dispatch 前语义一致性门禁。
  - prompt 对 diagnostic lineage 与 authoritative correction contract 的角色隔离不充分。
- Proposed change:
  - 增加最终 CurrentActionContract normalizer。
  - 将 forced-narrow correction contract 下沉并覆盖所有当前派生合同。
  - 扩展 dispatch semantic alignment gate。
  - 修正 C4 current-action prompt。
- Boundary check:
  - 只使用结构化字段：`NextAction`、`convergenceContract.state`、`requiredOutput`、`contractViolationRecovery.correctionContract`、`stagePacket`、`repairActionPackage`。
  - 不引入 workspace ID、template ID、Soul ID、语言、文件名、命令、provider 或业务文本特判。
- Verification:
  - 用当前 live 失败形态补 orchestrator 单测。
  - 补 prompt rendering 单测。
  - 运行 focused tests 和边界搜索。
  - 重新 live verify 两个 workspace 的 next retry input 是否只有一个当前权威合同。

## Design Position

`CurrentActionContract` 是 dispatch 前的最终权威动作合同。它不是新的业务状态机，而是从 convergence authority snapshot 派生出的只读、短生命周期结构，用于保证 provider-facing input 中所有动态合同字段一致。

目标链路：

```text
Coverage/Evidence Ledger
  -> Deterministic NextAction
  -> ConvergenceContract
  -> CurrentActionContract
  -> repairActionPackage.requiredOutput
  -> stagePacket.requiredOutput
  -> LLM C3/C4 current action
  -> contract gate
```

约束：

- `CurrentActionContract` 必须由当前 authority snapshot 与当前 correction contract 派生。
- 旧 failed-step input、旧 repairActionPackage、旧 convergenceContract、旧 stagePacket 和旧 expectedContract 只能作为 diagnostic lineage。
- 同一 step input 中不得同时存在两个互相冲突的 action contract。
- 若无法归一化，必须 fail-fast 为 runtime internal contract misalignment，不能提交给 LLM 自修。

## Required CurrentActionContract Rules

### Forced Narrow Validation Evidence

当满足以下条件：

```text
contractViolationRecovery.correctionMode = forced_narrow
contractViolationRecovery.correctionContract.requiredOutputShape = bounded_validation_evidence
```

当前权威合同必须为 validation evidence only：

- `requiredOutput.mustProduceConcreteValidationEvidence=true`
- `requiredOutput.mustIncludeStateChangingNode=false`
- `requiredOutput.acceptedKinds` 只能包含 bounded validation-capable kinds，例如 `shell/command`、`mcp/tool_call`、`validation/managed_session`
- `requiredOutput.forbiddenKinds` 必须包含 `patch/apply`
- 顶层 `stateChangingOutputRequired` 必须清理
- 顶层 `repairObjective` 必须与 evidence acquisition / concrete validation evidence 语义一致，不得继续是 `produce_state_changing_output`
- `convergenceContract.state` 不得继续是 `need_state_change`
- `repairActionPackage.requiredOutput`、`stagePacket.requiredOutput` 与顶层 `requiredOutput` 必须同源

### Forced Narrow Failure Evidence

当 `requiredOutputShape=bounded_failure_evidence`：

- 当前合同必须是 bounded evidence acquisition。
- 不允许同一 action 中要求 patch/apply。
- 如果后续 evidence 表明已有足够 implementation target evidence，再由下一次 authority refresh 推导 state-changing NextAction。

### State-Changing Retry

当 forced-narrow shape 明确是 `state_changing_output`、`valid_patch_apply` 或 `state_changing_output_from_available_snapshot`：

- state-changing 合同才可成为当前权威。
- `mustIncludeStateChangingNode=true` 必须与 `convergenceContract.state=need_state_change` 一致。
- 若缺少可用于 state-changing 的 current target snapshot 或 target evidence，应先进入 evidence acquisition，而不是在同一 action 里同时要求读取和 patch。

### Diagnostic Expected Contract

`contractViolationRecovery.expectedContractAuthoritative=false` 时：

- `expectedContract` 只能解释 rejected output 为什么失败。
- prompt 不得要求 LLM 满足 `expectedContract`。
- validator 不得从 `expectedContract` 推导当前 requiredOutput。
- C4 current-action 可以包含 compact diagnostic summary，但必须标注其为 diagnostic lineage。

## Implementation Scope

### 1. Current Action Normalizer

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`

Required changes:

- 新增或收拢一个 dispatch 前最终归一化函数，例如：

```text
normalizeCurrentActionContract(input, failedStep)
```

- 该函数必须在所有 convergence apply、output mode selection、repair output decision refresh 之后，stage packet attach 之前执行。
- forced-narrow correction contract 必须覆盖：
  - top-level `requiredOutput`
  - `repairActionPackage.requiredOutput`
  - `stagePacket` seed / rebuilt packet input
  - top-level `repairObjective`
  - top-level state-changing flags
  - `convergenceContract`
- 函数只消费结构化字段，不读取模板名、语言、文件名或自然语言业务内容。

### 2. Dispatch Semantic Gate

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`

Required checks:

- `convergenceContract.state=need_state_change` 时，当前 requiredOutput 必须允许 state-changing output。
- validation-only requiredOutput 时，`repairActionPackage.requiredOutput`、`stagePacket.requiredOutput` 和 top-level `requiredOutput` 不能允许 `patch/apply`。
- `mustIncludeStateChangingNode` 不能在三个 requiredOutput 来源中出现冲突。
- `repairObjective` 与 `requiredOutput.contractViolationRequiredOutputShape` 不得语义冲突。
- gate 失败时返回 runtime internal contract misalignment，不进入 LLM provider。

### 3. Stage Packet Rebuild

Target files:

- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`

Required changes:

- `stagePacket.requiredOutput` 必须从 normalized CurrentActionContract 派生。
- 不允许从旧 `executionContext.repairActionPackage` 或旧 `stagePacket` 恢复当前 requiredOutput。
- 保留旧信息时只能作为 diagnostic refs，不得进入 current action requiredOutput。

### 4. Prompt Current Action Role Isolation

Target file:

- `internal/modules/blocks/llm/biz/prompt_messages.go`

Required changes:

- 当 `expectedContractAuthoritative=false` 时，将现有表达：

```text
satisfies contractViolationRecovery.expectedContract and the current requiredOutput
```

替换为：

```text
contractViolationRecovery.expectedContract is diagnostic lineage only; satisfy the current correctionContract and requiredOutput.
```

- C4 current-action 必须突出唯一当前 requiredOutput。
- forced-narrow 模式下避免把 expectedContract 的 state-changing 字段渲染成并列约束。

### 5. Tests

Target test areas:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation_test.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence_test.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet_test.go`
- `internal/modules/blocks/llm/biz/prompt_messages_test.go`

Required tests:

- Forced-narrow `bounded_validation_evidence` 后，top-level requiredOutput、repairActionPackage.requiredOutput、stagePacket.requiredOutput 完全一致。
- Forced-narrow validation retry 不被 coverage missing `state_change` 重新推广为 state-changing action。
- Semantic gate 拒绝 `need_state_change + validation-only requiredOutput`。
- Semantic gate 拒绝 top-level 禁止 `patch/apply` 但 package/stagePacket 允许 `patch/apply`。
- Prompt 在 `expectedContractAuthoritative=false` 时不要求满足 expectedContract。

## Task Checklist

- [x] T1: 新增 CurrentActionContract normalizer，覆盖 forced-narrow validation/failure/state-changing 三类 shape。
- [x] T2: 将 normalizer 接入 runtime repair follow-up dispatch 最终路径。
- [x] T3: 确保 stagePacket attach 只消费 normalized requiredOutput。
- [x] T4: 增加 dispatch semantic alignment gate。
- [x] T5: 修正 C4 current-action prompt 中 expectedContract 的角色表达。
- [x] T6: 增加 forced-narrow validation live-shape 回归测试。
- [x] T7: 增加 semantic misalignment fail-fast 测试。
- [x] T8: 增加 prompt role isolation 测试。
- [x] T9: 运行 focused tests。
- [x] T10: 执行业务中立边界搜索。
- [ ] T11: 重新 live verify 两个 workspace 的最新 retry input。

## Verification Commands

Focused tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'ContractViolation|Convergence|StagePacket|Authority|Dispatch' -count=1
go test ./internal/modules/blocks/llm/biz -run 'Prompt|ContractViolation|CurrentAction' -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server|ws-4e83810542ac4d65|ws-7ced70e98ea50c43" internal/modules/kernel internal/modules/blocks internal/pkg
rg -n "expectedContract|correctionContract|requiredOutput|stateChangingOutputRequired|repairObjective|stagePacket|repairActionPackage" internal/modules/kernel/orchestrator/biz internal/modules/blocks/llm/biz
```

Live verification:

```bash
curl -sS http://127.0.0.1:8080/readyz | jq '.projection,.diagnostics'
curl -sS http://127.0.0.1:8080/api/v1/workorch/workspaces | jq '{projection, workspaces:[.workspaces[]|{id,state,activeRunId,blockingTicketId,promptMode:.viewModel.promptMode,revision:.viewModel.revision,sourceEventRevision:.viewModel.sourceEventRevision,projectedSourceRevision:.viewModel.projectedSourceRevision}]}'
```

For each new retry input:

```bash
jq '{runtimeStage,runtimeStageMode,repairObjective,stateChangingOutputRequired,requiredOutput,convergenceContract,contractViolationRecovery:{correctionMode:.contractViolationRecovery.correctionMode, correctionContract:.contractViolationRecovery.correctionContract, expectedContractAuthoritative:.contractViolationRecovery.expectedContractAuthoritative}, repairActionPackage:{repairObjective:.repairActionPackage.repairObjective, requiredOutput:.repairActionPackage.requiredOutput}, stagePacket:{requiredOutput:.stagePacket.requiredOutput}}' <input.json>
```

Expected live result:

- forced-narrow validation retry 中三处 requiredOutput 一致。
- `patch/apply` 不出现在当前 acceptedKinds。
- `stateChangingOutputRequired` 不存在或为 false。
- `convergenceContract.state` 不再是 `need_state_change`。
- LLM thoughtSummary 不再声称当前必须 state-changing。

## Completion Criteria

本文收口完成必须同时满足：

- 所有 task checklist 项完成。
- focused tests 通过。
- 边界搜索无新增业务/模板/语言/live workspace 特判。
- live retry input 中不存在 current action contract 分裂。
- 若 LLM 仍违例，违例原因必须是 LLM 未满足唯一当前合同，而不是 runtime 提供互相冲突的合同。
