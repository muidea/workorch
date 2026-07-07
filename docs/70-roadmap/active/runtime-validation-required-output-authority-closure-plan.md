# Runtime Validation RequiredOutput Authority Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-07

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-state-transition-contract-hardening-closure-plan.md`
- `runtime-validation-command-contract-authority-closure-plan.md`
- `runtime-validation-contract-gate-managed-session-closure-plan.md`
- `runtime-validation-failure-diagnostic-repair-dispatch-closure-plan.md`

## Purpose

本文用于收口 live 验证中暴露的 `validation requiredOutput authority`
不一致问题。

目标不是放宽 LLM 输出合同，也不是在 run recovery / finalization 中兜底，而是把
`validate` 主链路固定为单一权威派生：

```text
convergenceAuthority.nextAction = validate
  -> convergenceContract.state = need_validation
  -> repairActionPackage.requiredOutput = validation-only
  -> stagePacket.nextAction = validate
  -> currentActionContract.requiredOutput = same validation-only contract
  -> runtime dispatch only accepts declared validation producers
```

最终要求是：当 current authority 要求 validation evidence 时，runtime 必须一次性
确定可用 validation producer，并保证 provider/block dispatch 前所有 requiredOutput
镜像同源、同焦点、同 action，不再出现 validate 被旧 state_change 合同覆盖，或可用
MCP/managed validation 能力被 requiredOutput 拒绝。

## Contract Audit

- Scope:
  - live workspace `ws-705249a8d7816330/run-092`
  - live workspace `ws-db040d24a190f6e5/run-093`
  - validation requiredOutput authority
  - convergenceAuthority -> convergenceContract -> stagePacket -> currentActionContract
  - validation repair dispatch
  - runtime requiredOutput kind gate
- Observed state:
  - `ws-705249a8d7816330/run-092` 有正向产出：`patch/apply`、`git/write`、
    `shell/command` validation 均出现 accepted evidence。
  - 同 run 后续 focus 进入 `cwi-002` validation，managed validation 可用，但最后因
    `validation_detached_process_forbidden` 进入 `blocked_on_decision`。
  - `ws-db040d24a190f6e5/run-093` 有 `patch/apply`、`git/write` accepted evidence，
    但后续 `mcp/tool_call` validation 被拒绝：
    `runtime step kind mcp/tool_call is not accepted by current requiredOutput.acceptedKinds`。
  - `run-093` failure reason 同时出现
    `contractState=need_state_change requires state-changing requiredOutput`，而 projection
    中 current authority / gapBaseline 显示 `nextActionKind=validate`。
- Expected state:
  - `nextAction=validate` 时，必须派生 `convergenceContract.state=need_validation`。
  - validation requiredOutput 必须是 validation-only，且包含当前 runtime capability
    可用的 validation producer。
  - 若 MCP validation capability 可用，`mcp/tool_call` 必须进入 acceptedKinds；若不可用，
    应显式输出 capability gap，而不是派发后再被 requiredOutput 拒绝。
  - managed validation 输出形态违例时，若 runtime 仍可收窄重建合同，不应直接升级到
    human decision。
  - 旧失败、旧 stagePacket、旧 repairActionPackage、旧 convergenceContract 不得覆盖
    当前 authority。
- Deviation:
  - validation authority 下仍可能被旧 state_change 合同抢占。
  - validation requiredOutput acceptedKinds 与 runtime capability catalog 不一致。
  - runtime requiredOutput kind gate 把自身合同派生缺陷表现成普通 runtime step failure。
  - managed validation 可用时，违例耗尽仍可能过早进入 decision。
- Owner: WorkOrch core runtime。该问题属于中立状态机、requiredOutput 派生、
  capability interpretation、repair dispatch 和 contract gate，不属于 template、
  Soul、业务目标、文件名、语言或 provider 特判。
- Event path:
  - coverage ledger
  - gapBaseline
  - convergenceAuthority
  - convergenceContract
  - repairActionPackage.requiredOutput
  - stagePacket
  - currentActionContract
  - runtime block dispatch
  - requiredOutput kind gate
  - validation evidence ledger
  - next action recompute
- Contract gap:
  - validation requiredOutput 没有唯一构造入口。
  - validation producer 列表没有稳定从 runtime capability 派生。
  - validation repair dispatch 缺少“current authority 优先”的硬门禁。
  - invalid validation producer kind 缺少可恢复的 authority rebuild 分类。
  - managed validation narrow retry 和 decision eligibility 边界不够硬。
- Boundary check:
  - 修复只能基于通用字段：`nextAction`、`convergenceContract.state`、
    `requiredOutput.acceptedKinds`、`runtimeCapabilities.blockKinds`、
    `coverageWorkItemIds`、`structuredTargetRefs`、`validationEvidence`。
  - 禁止按 workspace ID、template ID、Soul ID、业务文本、文件名、语言、命令或 provider
    名称写分支。

## Design Rules

### 1. Validation RequiredOutput Has A Single Source

`nextAction=validate` 时，requiredOutput 只能由 validation authority 构造器生成。
允许读取当前 `repairActionPackage` 中与诊断、失败、scratch、focus 相关的上下文，但不得
继承 state-changing 字段。

必备字段：

- `mustProduceConcreteValidationEvidence=true`
- `mustIncludeStateChangingNode=false`
- `mustNotMutateWorkspace=true`
- `mustBindCoverageFocus=true`
- `mustDeclareValidationEvidenceMode=true`
- `mustDeclareValidationEvidenceObservation=true`
- `coverageWorkItemIds`
- `structuredTargetRefs`
- `targetRefs`
- `validationScratchDir` when scratch is required

禁止字段或值：

- `stateChangingOutputRequired=true`
- `mustIncludeStateChangingNode=true`
- `patch/apply` in `acceptedKinds`
- `git/write` in `acceptedKinds`
- state-changing shell/tool instructions

### 2. AcceptedKinds Comes From Runtime Capability

validation requiredOutput 的 `acceptedKinds` 必须按 runtime capability 派生，而不是手写
固定列表覆盖所有场景。

基础规则：

- `shell/command` 可作为 bounded validation producer。
- `validation/managed_session` 可用于需要 runtime 管理 timeout、scratch、port、cleanup、
  process group 的 validation。
- `mcp/tool_call` 只有在当前 workspace/run 声明了可用 MCP server，且该 capability 可产生
  validation evidence 时才允许。
- `file/read`、`file/list`、`git/read` 不能直接满足 validation evidence；需要目标快照时应走
  `acquire_target_snapshot` 或只读 acquisition action。

当 capability 缺失时，requiredOutput 必须表达 capability gap：

```text
mustResolveCapabilityGap = true
missingKinds = [...]
availableKinds = [...]
```

此时不得向 runtime dispatch 一个随后必然被 `acceptedKinds` 拒绝的 block node。

### 3. Validate Authority Cannot Rebuild As StateChange

provider/block dispatch 前必须校验：

```text
if convergenceAuthority.nextAction.kind == validate:
  convergenceContract.state == need_validation
  stateChangingOutputRequired != true
  requiredOutput is validation-only
  stagePacket.nextAction == validate
  currentActionContract.nextAction == validate
```

若发现旧 state_change 合同残留，必须在 dispatch 前重建 validation authority snapshot；
重建后仍不一致时返回 `runtime_internal_contract_misalignment`，不得继续执行。

### 4. Invalid Validation Producer Kind Is Authority Drift

以下错误不应作为普通 runtime step failure 终止 run：

```text
runtime step kind <kind> is not accepted by current requiredOutput.acceptedKinds
```

若当前 action 是 `validate`，且 `<kind>` 是 runtime capability 中声明过的 validation
producer，则应分类为：

```text
validation_required_output_capability_mismatch
recoverability = rebuild_validation_required_output_from_authority
```

若 `<kind>` 未声明或 capability 不可用，则应分类为 capability gap，并进入可治理的能力配置
路径，而不是伪装成 validation 失败。

### 5. Managed Validation Decision Gate

`validation_detached_process_forbidden`、`managed validation shell must wait` 等输出形态
违例应先进入 managed validation narrow retry。

只有满足以下任一条件，才能进入 decision：

- 缺少外部凭证、权限、用户选择或治理确认。
- runtime capability 不可用，且无法通过配置自动启用。
- managed validation narrow retry 已耗尽，且失败原因不是可机械收窄的输出形态问题。
- validation 环境真实不可达，并且没有 controlled substitute。

如果 `managedValidationAvailable=true` 且失败仍是输出形态违例，应继续重建窄合同或转换为
deterministic blocker，不应默认创建 human decision。

## Closure Tasks

### Phase 1: Validation RequiredOutput Single Builder

- [x] 收敛 `populateCoverageWorkItemValidationConvergenceContract`，确保 validation
  requiredOutput 只由 validation authority builder 生成。
- [x] 清理 validation requiredOutput 中的 state-changing 残留字段。
- [x] 确保 `repairActionPackage.requiredOutput`、`stagePacket.requiredOutput`、
  `stagePacket.nextAction.requiredOutput`、`currentActionContract.requiredOutput` 完全同源。

### Phase 2: Capability-Derived AcceptedKinds

- [x] 增加 validation producer capability 派生函数。
- [x] 有可用 MCP server / MCP validation capability 时加入 `mcp/tool_call`。
- [x] 无 MCP capability 时输出 capability gap，不派发非法 MCP node。
- [x] 保留 `shell/command` 与 `validation/managed_session` 的 bounded validation 规则。

### Phase 3: Dispatch Preflight Hard Gate

- [x] 在 convergence dispatch preflight 中增加 validate authority 硬校验。
- [x] 当 authority 为 validate 时，拒绝 `need_state_change`、`stateChangingOutputRequired`、
  `patch/apply`、`git/write` 残留。
- [x] repair dispatch 前必须以 current authority 重建快照；旧失败合同只能作为 diagnostic。

### Phase 4: Failure Classification And Recovery

- [x] 将 validation producer kind mismatch 分类为
  `validation_required_output_capability_mismatch`。
- [x] recoverability 设置为 `rebuild_validation_required_output_from_authority`。
- [x] capability 不可用时输出 capability gap / governed configuration path。
- [x] 禁止该类缺陷直接 terminal failed。

### Phase 5: Managed Validation Decision Eligibility

- [x] 调整 managed validation 违例耗尽逻辑。
- [x] `managedValidationAvailable=true` 且仍可机械收窄时不得进入 human decision。
- [x] 只有真实外部依赖、权限、治理或不可自动恢复能力缺口才允许 decision。

### Phase 6: Tests And Live Verification

- [x] 单测：`nextAction=validate` 派生 `need_validation`。
- [x] 单测：validation requiredOutput 不允许 `patch/apply` / `git/write`。
- [x] 单测：MCP capability 可用时 acceptedKinds 包含 `mcp/tool_call`。
- [x] 单测：MCP capability 不可用时输出 capability gap，不派发 MCP node。
- [x] 单测：repair dispatch 不能把 validate authority 重建成 state_change。
- [x] 单测：managed validation 可用时，输出形态违例耗尽不直接 decision。
- [ ] live 验证：`ws-db040d24a190f6e5` 类路径不再因 declared MCP validation producer 被
  requiredOutput 拒绝。
- [ ] live 验证：`ws-705249a8d7816330` 类路径不再在 managed validation 可用时过早进入
  decision。

## Acceptance Criteria

完成后，live 执行必须满足：

```text
run active
currentFocus missing validation
nextActionKind validate
convergenceContract.state need_validation
requiredOutput acceptedKinds includes available validation producers
runtime dispatch does not reject declared validation producer kind
managed validation output-shape violation narrows or deterministically blocks before decision
no validate -> need_state_change drift
```

不允许再出现：

```text
current authority = validate
contractState = need_state_change
requiredOutput forbids/omits declared validation producer
runtime step kind mcp/tool_call is not accepted by current requiredOutput.acceptedKinds
managedValidationAvailable=true but immediately blocked_on_decision for output-shape violation
```

## Verification Commands

建议代码收口后至少执行：

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/mcp/biz -count=1
go test ./internal/modules/blocks/shell/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
```

live 验证重点观察：

- `convergenceStatus.nextActionKind`
- current step `contractAudit.requiredOutput.acceptedKinds`
- `convergenceContract.state`
- `runtimeExecution.contractViolation`
- `repairLoop.state`
- `decisionCount`
- validation accepted/rejected evidence binding
