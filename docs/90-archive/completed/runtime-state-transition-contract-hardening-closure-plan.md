# Runtime State Transition Contract Hardening Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-07

Related:

- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [Convergence Authority Simplification Design](convergence-authority-simplification-design.md)
- [Runtime Authority Derived Snapshot Hardening Closure Plan](runtime-authority-derived-snapshot-hardening-closure-plan.md)
- [Runtime Validation Failure Diagnostic Repair Dispatch Closure Plan](runtime-validation-failure-diagnostic-repair-dispatch-closure-plan.md)

## Purpose

本文用于把近期 live 验证暴露出的循环式修补收敛为一个最小、确定、可验收的合同固化计划。

当前不再围绕单个 LLM 输出、某个 workspace 的表象状态或某个 projection 展示做单点补丁。后续代码收口必须按本文顺序逐项完成，且每项只修改一个主 owner 环节，避免在 runtime、projection、prompt、TUI/Web 多处同时补同一个问题。

## Contract Audit

- Scope: `ws-705249a8d7816330 / run-089-replan-001-replan-001-replan-001`
- Observed state:
  - `run.state = active`
  - latest/current step 为 failed
  - `blockingTicketId = null`
  - `decisionCount = 0`
  - 最后事件停在 contract violation recorded / step state changed / llm usage recorded，没有后续 dispatch requested 或 blocker event。
- Expected state:
  - step failed 后必须在同一 owner 状态迁移内产生且只产生一个结果：dispatch next step、blocked_on_decision、blocked_on_approval、run failed 或 run completed。
  - `coverage_assessment_incomplete + recompute_authority` 若 runtime 可继续，应重新计算 authority 并派发下一步；若不可继续，应落到明确 blocker 或 deterministic terminal。
- Deviation:
  - 出现 `active run + failed latest/current step + no blocker + no next dispatch`。
  - 这说明 failure recovery 已能识别合同违例，但 recovery action 到 dispatch/block/terminal 的最终出口没有被硬固化。
- Owner: core orchestrator。
- Event path:
  - LLM candidate -> contract gate -> rejected diagnostic evidence -> contract violation recorded -> step failed -> recovery action selected -> dispatch/block/terminal。
- Contract gap:
  - failed-step 后的唯一出口合同未固化。
  - dispatch prepare/publish 失败缺少明确状态落点。
  - rejected diagnostic 到 NextAction recompute 的权威边界仍允许旧 input / old recovery lineage 参与当前动作派生。
- Proposed change:
  - 先固化 failed-step 唯一出口，再固化 authority-derived snapshot、rejected diagnostic recompute、deterministic evidence acquisition、decision eligibility。
- Verification:
  - 单测覆盖每个 failure recovery action。
  - live 验证禁止再次出现 active idle failed-step。
  - 搜索确认没有新增场景特判、模板 ID、文件名或 provider 特判。

## Closure Principle

本轮收口只接受一种主链路：

```text
LLM/runtime output
  -> contract gate
  -> accepted evidence or rejected diagnostic evidence
  -> deterministic NextAction recompute
  -> owner state transition
  -> dispatch/block/terminal
  -> projection/ViewModel
```

不允许新增并行判断路径：

- 不允许 projection 推断 blocker。
- 不允许 TUI/Web 推断状态。
- 不允许 LLM decision text 覆盖 NextAction。
- 不允许旧 failed-step input、旧 `repairActionPackage`、旧 `contractViolationRecovery` 抢占当前 authority。
- 不允许用 watchdog、run recovery finalization 或隐式 resume 兜底主链路缺口。

## Primary Invariant

任意 step failed 后，run 必须进入以下状态之一：

1. 已成功请求下一步 dispatch，并能从事件或 step repository 观察到下一步。
2. 已创建同 scope open decision ticket，并进入 `blocked_on_decision`。
3. 已创建同 scope open approval，并进入 `blocked_on_approval`。
4. 已进入 deterministic terminal：`failed` 或 `completed`。

禁止状态：

```text
run.state = active
latest/current step.state = failed
blockingTicketId = empty
blockingApprovalId = empty
no dispatch requested
no executing/queued next step
```

该不变量是后续所有收口项的首要验收条件。

## P0-1 Failed Step Unique Exit Contract

### Owner

`internal/modules/kernel/orchestrator/biz/runtime_repair.go`

`internal/modules/kernel/orchestrator/biz/runtime_dispatch_event.go`

### Required Behavior

`handleRuntimeStepFailure` 完成 failed step 持久化、rejected diagnostic evidence、coverage update、step state event 后，必须通过 `dispatchRuntimeRepairFollowUpOrBlock` 得到明确结果。

`runtimeRecoveryActionForFailedStep` 的每个 action 都必须有唯一出口：

- `provider_retry` -> request dispatch provider retry step。
- `provider_retry_exhausted` -> block or terminal。
- `contract_violation_retry` -> request dispatch contract retry step。
- `contract_violation_exhausted` -> block or deterministic terminal。
- `runtime_repair` -> request dispatch runtime repair step。
- `runtime_repair_exhausted` -> block or deterministic terminal。
- `duplicate_failure` -> block or deterministic terminal。
- `context_integrity_blocked` -> block or deterministic terminal。
- `fail_run` -> terminal failed。

如果 `requestDispatchStep` 在 prepare、authority rebuild、idempotency、event publish 或 direct dispatch 任一阶段失败，不能只返回错误并让 run 保持 active idle。必须转换为：

- internal contract blocker，或
- deterministic failed terminal，或
- 明确 rejected diagnostic + NextAction recompute 后的下一步。

### Implementation Notes

- `requestDispatchStep` 应返回结构化错误分类，调用方据此选择 blocker/terminal。
- recovery dispatch failure 应写入 failed step output 的 `repairDispatchError` 与 structured diagnostic。
- dispatch prepare 失败属于 runtime internal contract misalignment，不是 LLM 输出问题。
- 不要新增 watchdog 来补偿未发布 dispatch event。

### Tests

- provider retry dispatch success。
- contract violation retry dispatch success。
- runtime repair dispatch success。
- dispatch prepare failure -> blocker/terminal。
- event publish failure -> blocker/terminal。
- `coverage_assessment_incomplete + recompute_authority` -> dispatch or blocker。
- 禁止 active idle failed-step。

## P0-2 Authority Derived Snapshot Hard Gate

### Owner

`internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`

`internal/modules/kernel/orchestrator/biz/runtime_convergence.go`

### Required Behavior

每次 provider/block dispatch 前，必须从当前 run owner state 的 convergence authority 一次性重建：

- `gapBaseline`
- `convergenceAuthority`
- `convergenceContract`
- `stagePacket`
- `currentActionContract`
- `repairActionPackage.requiredOutput`
- top-level `requiredOutput`

旧 input 中的同名字段只允许作为 diagnostic lineage。当前派发使用的 action 合同只能来自本次 authority snapshot。

### Hard Gate

dispatch 前必须校验：

- `convergenceAuthority.nextAction` 与 `convergenceContract.state` 一致。
- `stagePacket.nextAction` 与 `currentActionContract.nextAction` 一致。
- `requiredOutput.acceptedKinds/forbiddenKinds` 与当前 NextAction 一致。
- `state_change` 不得配 `need_evidence`。
- `validate` 不得配 `need_state_change`。
- `assessment` 不得携带 state-changing required output。
- `acquire_target_snapshot` 必须是 read-only acquisition contract。

重建后仍不一致时，应 fail-fast 为 runtime internal contract misalignment，不发送给 LLM。

### Tests

- state_change + need_evidence 被拒绝。
- state_change + requiredOutput 禁止 patch/apply 被拒绝。
- acquire_target_snapshot + read-only requiredOutput 合法。
- old failed-step `repairActionPackage` 不能覆盖当前 requiredOutput。
- provider retry / contract retry / runtime repair 都必须经过同一 hard gate。

## P0-3 Rejected Diagnostic To NextAction Recompute

### Owner

`internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`

`internal/modules/kernel/orchestrator/biz/runtime_repair.go`

`internal/modules/kernel/orchestrator/biz/runtime_convergence.go`

### Required Behavior

所有未通过合同的 LLM/runtime 输出只能进入 `diagnostic/rejected` evidence。随后必须以 ledger 为权威重新推导 NextAction。

recompute 输入只允许来自：

- current coverage ledger。
- accepted evidence ledger。
- rejected diagnostic evidence ledger。
- current definition revision。
- current policy/capability binding。

recompute 输入不得来自：

- 旧 failed-step input 里的 current action。
- 旧 execution graph。
- 旧 decision question。
- 旧 repairActionPackage。
- 旧 contractViolationRecovery 的 action advice。

### Required Rules

- `coverage_assessment_incomplete + recompute_authority` 必须重新推导当前 focus。
- 如果 current focus 缺 `state_change`，NextAction 优先为 `state_change` 或其必要前置 `acquire_target_snapshot`。
- 如果 current focus 已有 state_change 但缺 validation，NextAction 为 `validate`。
- 如果 runtime 可继续，LLM 的 `blocked_on_decision` 只能成为 rejected diagnostic。
- 重复违例超预算后，必须先过 Decision Eligibility Authority。

### Tests

- failed dodAssessment cannot close coverage。
- continuable decision request -> rejected diagnostic + recompute。
- recompute authority does not inherit old requiredOutput。
- repeated coverage_assessment_incomplete does not create fake decision without external dependency。

## P0-4 Deterministic Evidence Acquisition

### Owner

`internal/modules/kernel/orchestrator/biz/runtime_convergence.go`

`internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`

`internal/modules/kernel/orchestrator/biz/runtime_repair_context_material.go`

### Required Behavior

`acquire_target_snapshot`、workspace manifest read、read-only target evidence acquisition 这类 runtime 可确定执行的动作，应优先使用 deterministic runtime-managed action 或受限 block adapter。

LLM 可以提供候选定位或解释，但不能拥有 read-only acquisition 的状态迁移权威。

### Required Rules

- 若 LLM 请求用户提供文件内容，但 runtime 已知道 readable resource，应自动转换为 runtime-managed `file/read` target snapshot acquisition。
- 若没有 exact-safe target snapshot，不得直接要求 LLM 产生 exact patch。
- read-only acquisition contract 必须 forbid state-changing output。
- snapshot evidence 必须写入 sourceRef、targetRef、coverage binding、completeness、observedClaims。

### Tests

- converted decision readable resources -> readableResourceRepairContext。
- readableResourceRepairContext -> acquire_target_snapshot 或 exact-safe snapshot。
- acquire_target_snapshot 不允许 patch/apply。
- snapshot accepted 后下一轮可进入 state_change。

## P0-5 Decision Eligibility Authority

### Owner

`internal/modules/kernel/orchestrator/biz/runtime_repair.go`

Decision / ticket owner modules。

### Required Behavior

Decision 是治理动作，不是 LLM 违例、repair exhaustion 或 context uncertainty 的默认出口。

只有满足以下条件之一才允许创建 decision ticket：

- 缺少外部信息、凭证、权限、环境配置或第三方确认。
- 存在用户选择，且选择会改变 Goal/DoD、scope、风险接受或 destructive action。
- governance policy 明确要求人工确认。
- 不可恢复失败需要用户选择 fail、waive、change definition 或 restart。

### Forbidden

- Runtime 可以 read 文件时，不因 LLM 请求文件内容创建 decision。
- Runtime 可以 recompute NextAction 时，不因 coverage assessment incomplete 创建 decision。
- Runtime 可以 retry same contract 时，不因 LLM blocked_on_decision 创建 decision。
- 没有 open ticket 时，不允许 projection 显示 `blocked_on_decision`。

### Tests

- continuable candidate choice -> no decision。
- readable resource known -> no decision。
- external dependency missing -> decision allowed。
- blocker projection must resolve same-scope open ticket。

## P1 Evidence And Projection Closure

### Evidence Ledger

- accepted evidence 才能关闭 coverage。
- rejected diagnostic 只参与 retry/recompute。
- state_change / validation / assessment / artifact / decision / approval 必须绑定 targetRefs 和 coverageWorkItemIds。
- Memory 不得作为 completion authority。

### Projection

- workorchd projection 只展示 owner state。
- TUI/Web 不维护状态机。
- blocking ticket / approval 必须来自同 scope owner snapshot。
- full snapshot 仅用于 detail/status，主视图优先 compact projection。

## P1 Resource And Performance Closure

- managed validation resource 在 completed、failed、contract violation、converted decision repair、terminal 五类路径释放。
- read-only acquisition 和 snapshot projection 不触发高频全量 JSON marshal/unmarshal。
- idle active/blocked 状态不得持续触发 full snapshot rebuild。

## P2 Live Verification Matrix

每项 P0 完成后执行 live 验证：

```text
workspace active + failed latest step + no blocker + no dispatch = fail
workspace blocked_on_decision + open same-scope ticket = pass
workspace active + next step queued/executing = pass
workspace terminal failed/completed + reason/evidence = pass
```

必须覆盖：

- provider timeout -> provider retry -> success/block。
- state-changing required missing -> contract retry -> block/terminal。
- coverage assessment incomplete -> recompute authority -> dispatch/block。
- converted decision readable resource -> acquire target snapshot -> state_change。
- target snapshot missing -> read-only acquisition。
- decision with real external dependency -> decision ticket。

## Anti-Loop Guardrail

若同一 failure signature 连续两次收口后仍未改善，停止局部补丁，重新执行完整 Contract Audit。

禁止继续新增：

- 场景特判。
- provider 特判。
- workspace ID 特判。
- prompt-only 修复。
- projection 兜底。
- watchdog 兜底。
- run recovery finalization 兜底。

允许新增：

- owner state hard gate。
- structured diagnostic。
- deterministic NextAction recompute。
- dispatch/block/terminal 唯一出口。
- 对应回归测试。

## Done Criteria

本文全部收口完成必须满足：

- [ ] P0-1 failed-step 唯一出口合同完成。
- [ ] P0-2 authority-derived snapshot hard gate 完成。
- [ ] P0-3 rejected diagnostic -> NextAction recompute 完成。
- [ ] P0-4 deterministic evidence acquisition 完成。
- [ ] P0-5 decision eligibility authority 完成。
- [ ] 所有 P0 项均有单测。
- [ ] live 验证不再出现 active idle failed-step。
- [ ] projection 与 owner state 一致。
- [ ] 没有新增 core 场景特判、模板名、workspace ID、provider 行为特判。
- [ ] `go test ./internal/modules/kernel/orchestrator/biz -count=1` 通过。
- [ ] `make fmt-check` 通过。
- [ ] `git diff --check` 通过。
