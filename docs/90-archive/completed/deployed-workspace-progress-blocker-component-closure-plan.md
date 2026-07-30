# 已部署 Workspace 推进阻塞组件收口计划

Status: archived — superseded

Type: closure-plan

Last Updated: 2026-07-30

Related:

- `../../00-core/runtime-stage-contract.md`
- `module-block-initiator-inventory-audit-2026-07-09.md`
- `current-action-contract-authority-closure-plan.md`
- `runtime-validation-required-output-authority-closure-plan.md`
- `runtime-terminal-outcome-authority-closure-plan.md`
- `workorchd-state-loop-and-projection-closure-plan.md`

> Archive notice (2026-07-30): this is a dated diagnostic and closure diary, not a current implementation or
> operations specification. Its later Runtime V1 maintenance/CLI assertions have no matching source, typed
> contract, or CLI entry point and are superseded by the Runtime V2-only production boundary. They must not be
> recreated or scheduled as a maintenance action. Current authority is `docs/00-core/runtime-stage-contract.md`,
> the Orchestrator readme, and `internal/pkg/workorchruntimecontract`.

## Purpose

本文用于收口最新部署服务中两个真实 workspace 暴露的推进阻塞问题，并将问题归属到当前 22 个组件中的实际 owner，以便按组件逐项实现、验证和关闭。

本文是实现计划，不新增 runtime core 业务语义。所有收口必须继续遵守统一运行设计合同：core 只治理 stage、contract、artifact、evidence、result、repair、replan、lineage 和状态一致性，不得针对语言、文件名、测试框架、MCP server ID、workspace ID 或具体业务目标写特判。

## Live Audit Baseline

核对时间：2026-07-11 15:41 至 15:50，服务使用 file backend，projection 为 `ready`，source revision 与 projected revision 对齐。

### Workspace A：LSP-Bridge

- Workspace：`ws-5cfc9874cebda0b8`
- Run：`run-003`
- 状态：`active`
- source revision：`1171 -> 1274`
- current focus：`cwi-003 -> cwi-004`
- accepted evidence：`12 -> 14`
- rejected evidence：`2 -> 14`
- evidence-ready：`4 -> 5`
- 当前 `cwi-004` 状态：`failed`
- 当前动作：继续生成 concrete validation evidence repair。
- 交付物状态：Git clean，`go test ./...` 通过。

最新合同拒绝原因：

```text
concrete validation evidence cannot use static inspection or source scanning alone to close runtime-behavior coverage
```

运行时能够拒绝无效证据并自动续跑，说明门禁与 continuation 基础链路有效；但同类无效输出、stale mirror 重绑和 repair lineage 持续增长，说明当前推进质量不健康。

### Workspace B：轻量级 LLM 本地代理网关

- Workspace：`ws-609ce9149734ebb7`
- Run：`run-002`
- 状态：`failed`
- Workspace `activeRunId`：已释放。
- source revision：保持 `2297`，无后续事件。
- Result：`rejected`。
- 交付物状态：`main.go`、`go.mod`、`config.yaml` 未提交。
- `go test ./...` 失败：

```text
undefined: LoadConfig
undefined: setupRoutes
```

Run 最终 failureReason 为 repair budget 耗尽及 `runtime_internal_contract_misalignment`，Result 原因为实际编译失败 `exit status 1`。直接 Run snapshot 已指向 `run-002/failed`，但 Workspace snapshot 的 convergenceStatus 仍指向旧 `run-001/superseded`。

## Root Cause Classification

| ID | 问题 | 根因判断 | 主要 owner | 影响 |
| --- | --- | --- | --- | --- |
| R01 | concrete validation graph 持续被判为 static-only | 当前动作合同虽然要求动态验证，但 provider-facing 输入与输出约束仍未稳定促成可观察、可绑定的动态证据 | `kernel/orchestrator`、`blocks/llm` | repair 重复、rejected evidence 激增 |
| R02 | stale requiredOutput / stagePacket mirror 持续重绑 | 当前 authority 已能纠正 stale mirror，但旧镜像仍反复进入派生输入，没有在 dispatch 前完成单一权威归一化 | `kernel/orchestrator` | lineage 膨胀、合同冲突风险、额外 LLM round |
| R03 | repair 输入过大且持续截断 | 当前 action 与历史 diagnostics、repair packages、evidence lineage 未形成足够严格的 provider-facing 分区和压缩 | `blocks/llm`，`kernel/orchestrator` | 关键合同被稀释、token/CPU/时延开销增加 |
| R04 | `need_state_change` 与 requiredOutput 不一致 | convergence state、repair objective 和 accepted output kinds 不是同一最终权威快照派生 | `kernel/orchestrator` | 网关修复无法产生缺失实现，最终耗尽 budget |
| R05 | Workspace snapshot 指向旧 Run convergence | Workspace 聚合投影没有稳定选择当前有效或最新终态 Run | `application/workorchd` | Web/TUI/诊断读取错误恢复目标 |
| R06 | Run 与 Result 终态原因割裂 | Run 保存恢复治理终止原因，Result 保存直接执行失败，缺少统一终态原因合同 | `kernel/orchestrator`、`kernel/run`、`blocks/result` | 终态解释不一致，恢复与验收判断困难 |
| R07 | terminal runtime execution 短暂处于 done 但未 finalized | worker terminal、Step finalized、下一 repair dispatch 和 projection overlay 的顺序仍可能产生不一致窗口 | `kernel/orchestrator`、`kernel/run`、`application/workorchd` | watchdog/用户误判 stalled 或 pending resource |

## Component Closure Inventory

| Closure ID | 组件 | 优先级 | 收口目标 | 状态 |
| --- | --- | --- | --- | --- |
| CC01 | `kernel/orchestrator` | P0 | 单一 current action authority、validation preflight、repair 去重与终态原因归一化 | reopened: convergence state/output conflict |
| CC02 | `blocks/llm` | P0 | provider-facing 当前合同隔离、上下文压缩和结构输出预检 | reopened: retry tail still oversized |
| CC03 | `application/workorchd` | P0 | Workspace/Run projection effective-run 一致性 | live verified |
| CC04 | `kernel/run` | P1 | Step/Run terminal finalize 顺序与 failure reason 合同 | completed |
| CC05 | `blocks/result` pairwise contract | P1 | 正式 Result 同时保留直接失败与恢复终止语义 | completed |
| CC06 | live verification | P0 | 重新部署并确认两个 workspace 不再复现同类阻塞 | failed |

## 2026-07-11 Code Closure Progress

- CC01：派生 `requiredOutput` / `stagePacket` mirror 改为无噪声确定性重建；真正的最终 mirror 不一致仍由 dispatch preflight 返回 conflict。`authorityDiagnostics` 对完全相同的诊断执行幂等去重，不再在同一输入中重复累积。
- CC02：存在 `currentActionContract` 时，provider budget 不再恢复旧 `stagePacket.nextAction`、`stagePacket.requiredOutput`、current gap/failure 等并列合同；Stage Packet 只保留 target snapshot transport 与 scope identity。`outputDecisionContract` 和 current-action target snapshots 直接进入 Current Action Contract prompt。
- CC03：Workspace 无 `activeRunId` 时，如果存在正式 Result 或 terminal Run event，projection 会选择该最新终态 successor；没有正式终态证据时仍保持无 projection run，避免把任意历史 Run 当作当前状态。
- CC04：核对确认 `runtime_terminal_not_finalized` 是 runtime result available 到 Step formal finalize 之间的有界暂态；现有 Step terminal、checkpoint、resource release 与 restart recovery 测试通过，本轮不修改 Run owner 状态机。
- CC05：Run terminal outcome 在 execution context 中保存 `directFailureReason`、`recoveryTerminationReason`、`sourceStepId` 和 evidence refs；Run `FailureReason` 同时包含直接失败与恢复终止原因。Rejected Result 的 summary diagnostics 同步保留两类原因。
- 边界门禁补齐：`blocks/decision` 与 `blocks/approval` 的 `RecordEvidence` command topic 已补入 owner `Contracts()` 和具体 command/result `MessageContracts()`；不改变现有 EventHub 投递方向或状态 owner。
- CC01 至 CC05 focused/pairwise tests 已通过；文档继续保持 `active`，等待 CC06 重新部署后的 live verification。

## 2026-07-11 CC06 Live Verification

部署进程于 2026-07-11 17:39 启动，二进制时间为 17:39:14。`/readyz` 为 `ready`，全局及 workspace-scoped source/projected revision 对齐。

### 已通过项

- 两个 workspace 的新 Run 均未再累计 `stale_required_output_mirror_rebound` 或 `stale_stage_packet_mirror_rebound`；LSP successor authority diagnostics 为 0。
- LSP successor 的失败 Step 已完整记录 worker stopped、Step finalized 和 resource released，不再残留 `runtime_terminal_not_finalized`。
- 网关 `run-003` 终态正确释放 `activeRunId`，全部 runtime resources 为 released。
- 网关 Run `FailureReason` 已同时包含 direct failure 与 recovery termination；`executionContext.terminalOutcome` 和 Rejected Result diagnostics 也保留两类原因。
- 网关终态 Workspace snapshot、direct Run snapshot、convergenceStatus 和 outcome 最终均指向 `run-003/failed`。
- 两个交付目录的核对结果保持稳定：LSP `go test ./...` 通过；网关仍因 `LoadConfig`、`setupRoutes` 缺失而编译失败。
- provider-facing cache prefix 在同一 workspace 的所有 retry 中保持稳定；网关 10 次 LLM trace 的 `stablePrefixHash` 和 `renderedStablePrefixHash` 均未漂移。

### 未通过项

1. `kernel/orchestrator` authority mirror 仍未闭环。
   - 网关 `run-003` 最终 recovery termination 为：

   ```text
   executionContext.stagePacket.requiredOutput does not match current authority requiredOutput
   ```

   - 说明普通 dispatch 的 mirror 重建已去除告警噪声，但 contract-retry/final authority 路径仍可能携带旧 executionContext stage packet。
   - CC01 必须重新打开，覆盖 contract violation retry、forced-narrow managed validation 和 terminal recovery dispatch 路径。

2. active Run 的 Workspace outcome 仍可能陈旧。
   - 网关 `run-003` active 且 convergenceStatus 已指向 run-003 时，Workspace outcome 仍短暂指向 `run-002/failed`。
   - 只有 run-003 进入终态后 outcome 才切换为 run-003。
   - CC03 必须重新打开，active effective Run 应优先于历史 terminal Result/Run outcome。

3. persisted focus mirror 仍有漂移。
   - 网关终态仍报告 `authority_focus_drift`：旧 `coverageFocus=cwi-085`，当前 authority 为 `cwi-002`。
   - 当前不会覆盖 convergence authority，但仍说明历史派生 focus 未在 authority refresh 边界清理。

4. provider timeout 的治理文案与实际计数不一致。
   - LSP successor 只有 1 个 failed LLM Step，acceptance budget 为 `repairAttempts=1`、`repeatedFailureSignatures=1`，Decision context 的 `attempts=0`。
   - Ticket reason 却显示“自动修复尝试已耗尽（共 10 次）”，其中 10 实际来自 `maxAttempts`，不是已执行次数。
   - 该 TLS handshake timeout 被正确识别为 external dependency blocker，但 reason 必须区分 actual attempts 与 configured maximum。

5. provider input 仍随 retry 增长。
   - LSP 单次 request 为约 94 KB。
   - 网关 10 次 trace 从约 100 KB 增至最高约 170 KB，estimated input tokens 从约 25K 增至 42K。
   - cache prefix 稳定，但动态 tail/repair lineage 仍需进一步有界化。

### CC06 结论

本轮部署验证证明 terminal reason、resource finalize、cache prefix 和 stale mirror warning noise 的收口有效，但 authority mirror 的根因、active effective-run outcome、focus mirror 清理和实际 retry count 文案仍未闭环。本文保持 `active`，CC01、CC02、CC03 需要针对以上 live failure 重新进入代码收口，CC06 保持 `failed`，不得标记 completed。

## 2026-07-11 Post-Live Code Closure

针对 CC06 暴露的剩余项已完成第二轮代码收口，等待重新部署验证：

- CC01 contract-retry mirror：`currentActionSetRequiredOutput` 现在同步更新顶层、repair package、stage packet、stage packet next action 及 executionContext 内对应 mirror；统一 authority renderer 在 forced-narrow current action 归一化后同步 mirror，并从 canonical mirror 重建 CurrentActionContract。新增 contract violation retry + managed validation + stale executionContext stage packet fixture，所有 requiredOutput mirror 必须完全一致。
- CC01 persisted focus：Run 进入 failed/cancelled/superseded 等终态前执行 owner convergence normalization，确保最终持久化的 `coverageFocus`、requiredOutput 和 stage packet 与 convergence authority 一致，不再把历史 focus 带入终态投影。
- CC03 active outcome：Workspace outcome selection 先选择 `activeRunId` 指向的非终态 Run；若聚合 snapshot 已有非终态 direct Run，也优先于历史 terminal event/result，避免 active Run 期间 outcome 继续指向 predecessor failure。
- provider governance count：runtime repair Decision reason 改为同时使用 actual attempts 与 configured maximum；只有 actual 达到 maximum 时才使用“尝试已耗尽（共 N 次）”，提前停止时显示“实际 N 次，上限 M 次”。
- CC02 retry tail：provider context budget 对 runtime repair / contract violation retry 应用随 attempt 增强的 adaptive shrink；第 1/2/3+ 次分别至少进入 shrink level 2/3/4。重复 contract retry fixture 验证 failed graph 被移除、current contract 保留且 request bytes 低于默认 96 KiB dynamic request budget。
- 本轮未修改组件间通信方向、EventHub topic、command/data/result 合同或 owner repository 边界。

### Post-Live Code Verification

以下验证已通过：

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
go test -p 1 ./internal/modules/... -count=1
go test ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

第一次与其它全量门禁并行执行 `go test ./...` 时，`internal/services/workorch/biz/backends` 的一个 EventHub 启动测试在 `Hub.Subscribe` 等待并触发 10 分钟测试超时；该测试单独重跑、该包完整重跑以及随后顺序执行的 `go test ./...` 均通过，当前没有证据表明与本轮变更存在功能回归。

CC06 继续保持 `failed`。只有重新部署后确认 forced-narrow retry 不再出现 executionContext mirror misalignment、active outcome 指向当前 Run、终态无 focus drift、reason 计数正确且 retry request tail 有界，才能将 CC01 至 CC03 的 `live pending` 与 CC06 一并关闭。

## 2026-07-11 Second Deployment Live Verification

重新部署进程于 2026-07-11 20:03 启动，二进制构建时间为 20:03:15。Go build info 为 `59276638+dirty`，说明服务基于 `59276638` 加当前工作树收口代码构建。验证完成时 `/readyz` 为 `ready`，全局 source/projected revision 为 `2626/2626`，两个 workspace-scoped projection revision 也保持对齐。

### 已通过项

- CC03 active effective-run outcome 已生效。LSP `run-003-replan-001-replan-001` 和网关 `run-004` active 时，Workspace `activeRunId`、snapshot Run、`convergenceStatus.runId` 与 `outcome.runId` 均指向当前 Run；不再回退到历史 failed Run。
- requiredOutput mirror 根因修复已生效。两个 Run 终态的顶层 requiredOutput、repair package、stage packet、stage packet next action 与 CurrentActionContract requiredOutput 完全一致；未再出现 `executionContext.stagePacket.requiredOutput does not match current authority requiredOutput`。
- persisted focus 清理已生效。两个 Run 终态 `coverageFocus.id` 与 `convergenceAuthority.currentFocus.id` 均为 `cwi-001`，Workspace snapshot `authorityDiagnostics` 为 0。
- runtime repair actual/max 文案已生效。两个 Run 的 recovery termination 均显示“实际 1 次，上限 10 次”，不再把配置上限误报为实际尝试次数。
- terminal finalize 正常。两个 Workspace 均释放 `activeRunId`；终态 runtime Step 有 `doneAt`、`finalizedAt` 和 `finalizeEvent=step_failed`，LSP 10 个资源、网关 16 个资源全部为 released。
- provider cache prefix 稳定。LSP 6 次请求与网关 9 次请求各自只有 1 个 `stablePrefixHash` 和 1 个 `renderedStablePrefixHash`。
- 交付物有正向变化：网关已生成并提交基础代理框架，`go test ./...` 当前通过；LSP `go test ./...` 继续通过，但新生成的 `.gitignore` 尚未提交。

### 仍未通过项

1. CC01 仍存在 convergence state 与 requiredOutput 的最终冲突。
   - 两个 Run 最终 recovery termination 都变为：

   ```text
   contractState=need_state_change requires state-changing requiredOutput
   ```

   - 失败前当前 authority、stage packet 与 current action 均为 `need_validation` / validation-only requiredOutput；在为 `validation_static_only` 构建下一轮 runtime repair dispatch 时，`BuildConvergenceContract` 或后续 authority refresh 将合同提升为 `need_state_change`，但 requiredOutput 仍保持 validation-only。
   - 这不再是 mirror 不相等，而是同一 canonical requiredOutput 与 convergence state 的语义不相容。需要继续收口 validation contract violation -> next convergence contract 的转换优先级。

2. CC02 retry dynamic tail 仍未达到有界目标。
   - LSP 6 次请求为 94,555–149,971 bytes、23,639–37,493 estimated input tokens。
   - 网关 9 次请求为 101,337–164,365 bytes、25,335–41,092 estimated input tokens。
   - adaptive shrink 已生效，但所有 runtime/contract retry 的 `runtimeRepairAttempt` 与 `contractViolationRecovery.attempt` 都保持为 1，因此 shrinkLevel 只能到 2；同一 root scope 的第二条 contract retry 仍显示 attempt 1。
   - contract retry request 仍显著高于预期 96 KiB，且 `lastRepairInputBytes` 分别达到 792,493 和 1,070,821 bytes。需要按 recovery budget scope/root lineage 计算累计 attempt，并进一步限制 protected stage packet、coverage context 与 repair package 的重复载荷。

3. 两个 workspace 都没有实现 coverage focus 前移。
   - 两个新 Run 最终都停留在 `cwi-001`，并因上述 runtime internal contract conflict 提前失败。
   - 因此“至少一个真实 active workspace 连续推进多个 coverage focus”的 Completion Definition 仍未满足。

### 第二次部署结论

CC03 可视为 live verified；CC01 的 mirror/focus 子项和 retry count 文案已通过，但 CC01 整体因新的 `need_state_change`/validation requiredOutput 冲突重新打开。CC02 adaptive shrink 已部分生效，但 attempt scope 和动态 tail 大小仍失败。CC06 继续保持 `failed`，下一轮应优先修复 convergence transition，再修复 scoped retry attempt 与 provider protected tail 压缩。

## Remaining Closure Execution Plan

本节是第二次部署验证后的逐项代码收口入口。当前只允许修改 `kernel/orchestrator` 与 `blocks/llm` 的相关实现；`application/workorchd` effective-run projection、Run terminal finalize、focus mirror、actual/max 文案及 EventHub typed contract 边界已经通过 live 验证，不得在没有新证据时重复改动。

### RC01：收口 validation violation 到下一 convergence contract 的转换

#### 目标

消除以下最终合同冲突：

```text
contractState=need_state_change requires state-changing requiredOutput
```

#### 权威优先级

下一轮 dispatch 的生效合同按以下顺序生成，低优先级只能补充 diagnostic lineage，不得覆盖高优先级：

1. 当前 Run owner 的 convergence authority。
2. 当前 forced-narrow correction contract。
3. 基于最新失败证据重新构建的 convergence contract。
4. 历史 repair package、expected contract、failure target 和 rejected output。

#### 必须实现的转换规则

- `validation_static_only`、validation scratch/process-control 等 validation contract violation，如果没有 concrete implementation target evidence，下一轮必须继续保持：

  ```text
  convergenceState=need_validation
  runtimeStage=verification
  repairObjective=produce_concrete_validation_evidence
  requiredOutput=validation-only
  ```

- 只有最新失败自身提供 target snapshot、line refs、patch repair context、failure target repair context 或等价 structured implementation diagnostics 时，才允许转为：

  ```text
  convergenceState=need_state_change
  runtimeStage=repair
  repairObjective=produce_state_changing_output
  requiredOutput=state-changing
  ```

- `coverageTargetSet`、历史 `failureTargetScope`、旧 repair lineage 或旧 step target refs 不能单独构成 implementation target evidence。
- forced-narrow managed validation 生效时，普通 `BuildConvergenceContract` 不得仅因历史 failure target 将其覆盖为 `need_state_change`。
- 如果最终选择 `need_state_change`，必须原子更新 convergence state、runtime stage/mode、repair objective、next action、CurrentActionContract 和全部 requiredOutput mirror；禁止只改变 state。
- 如果最终选择 `need_validation`，必须移除 state-changing-only 字段，包括 `stateChangingOutputRequired`、patch-only accepted kinds 和旧 target repair package。

#### 目标文件

- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`

#### 回归测试

- [x] `validation_static_only -> forced_narrow managed validation -> next dispatch` 完整链路。
- [x] 无 implementation target evidence 时保持 `need_validation`。
- [x] 只有历史 target/failure scope 时仍保持 `need_validation`。
- [x] 有当前 concrete implementation target evidence 时原子切换为 `need_state_change`。
- [x] forced-narrow 过期后根据最新 owner authority 原子重建全部合同字段。
- [x] dispatch preflight 不再出现 state/requiredOutput semantic conflict。

### RC02：统一 scoped retry attempt

#### 目标

解决同一 recovery scope 内连续 contract retry 的 step ID 已出现 `-2`，但 `runtimeRepairAttempt` 与 `contractViolationRecovery.attempt` 仍始终为 1 的问题。

#### Recovery Scope

attempt 必须按以下稳定 scope 计数：

```text
workspaceId
+ runId
+ rootStepId
+ focusId
+ violatedRule/correctionProfileId
```

#### 必须实现的计数规则

- 同一 scope 的有效 retry attempt 必须依次为 1、2、3……。
- 不同 focus、root step、violation rule 或 correction profile 独立计数。
- `runtimeRepairAttempt`、`contractViolationRecovery.attempt`、step ID、recovery budget、Decision context 和 provider request audit 必须读取同一计数结果。
- 不得再通过 `contract-retry-001-...-2` 表示第二次 attempt；step ID 中的 attempt 必须与输入字段一致。
- 当前 focus 获得有效 evidence、coverage 前移、root scope 改变或 recovery 成功后，后续新 scope 从 1 开始。
- heartbeat、mirror normalization、projection refresh 和未实际提交 provider 的 preflight rejection 不得增加有效 attempt。

#### 目标文件

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_budget.go`
- contract retry step ID、root step 和 attempt count helper 所在文件。

#### 回归测试

- [x] 同一 scope 连续三次 contract retry 输出 attempt 1/2/3。
- [x] step ID、input attempt、budget context 与 request audit 完全一致。
- [x] scope 中任一关键字段变化后重新从 attempt 1 开始。
- [x] 同一失败未实际 dispatch provider 时不消费 attempt。
- [x] retry exhausted 判断使用 scoped actual attempts，而不是 step 名称后缀数量。

### RC03：限制 provider dynamic tail

#### 目标

在不牺牲 P0-P4 必要合同的前提下，使 retry request 不再随 lineage 增长，并将 contract retry 控制在配置预算内。首轮 live 门禁继续采用 96 KiB。

#### Attempt 分级压缩

| Attempt | 最低 shrinkLevel | 保留内容 | 必须移除或摘要化 |
| --- | --- | --- | --- |
| 1 | 2 | current action、current focus、requiredOutput、最新失败、必要 target evidence | 完整 failed graph、重复 step input/output |
| 2 | 3 | attempt 1 内容的进一步 bounded summary | 完整 coverage ledger、历史 repair package、重复 validation signal |
| 3+ | 4 | Goal/DoD、current action/focus、requiredOutput、最新非等价失败摘要、必要 snapshots/refs | 全量 historical context、process timeline、旧 rejected output 正文 |

#### 必须实现的压缩规则

- `stagePacket` 只保留当前 identity、stage/action、focus、requiredOutput 和必要 target snapshot refs。
- `coverageWorkItemContext` 只保留 current focus、必要相邻候选、summary 和 full ledger ref；不携带完整 unresolved/historical items。
- `repairActionPackage` 只保留当前失败、current requiredOutput、当前 target evidence、当前 coverage scope 和最近一次非等价尝试。
- `processEvidenceTimeline`、`evidenceLedger`、历史 step output 和 rejected output 使用 bounded summary/ref。
- 同一语义对象不得同时以完整形态出现在 stage packet、executionContext、repair package、validation signal 和 current progress digest。
- Goal/DoD、current action、current focus、requiredOutput、capability contract 与当前 concrete target evidence 继续作为 protected fields。
- retry attempt 变化只能影响 C3-C5；`stablePrefixHash` 与 `renderedStablePrefixHash` 必须稳定。

#### 目标文件

- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/provider_cache_plan.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`

#### 回归测试

- [x] 使用 40 个 coverage item 的 LSP 形态 fixture。
- [x] 使用 89 个 coverage item 的网关形态 fixture。
- [x] attempt 1/2/3 分别进入至少 shrinkLevel 2/3/4。
- [x] attempt 2 request bytes 不大于 attempt 1，attempt 3 不大于 attempt 2。
- [x] contract retry request body 不超过 96 KiB live 门禁。
- [x] context gate 为 pass，protected Goal/DoD/current action/focus/requiredOutput/target evidence 完整。
- [x] stable prefix 两类 hash 在所有 retry 中保持单值。

### RC04：验证与重新部署顺序

#### 2026-07-11 代码收口结果

- RC01 已完成：validation contract violation 现在直接消费结构化 `violatedRule`；validation-only focus 且最新失败没有 concrete implementation target evidence 时，下一 dispatch 保持 `need_validation / verification / validation-only requiredOutput`，历史 failure target、repair lineage 与 rejected output 不再触发误升级。
- RC02 已完成：contract retry attempt 按 `workspace + run + rootStepId + focusId + violatedRule + correctionProfileId` 的稳定 scope 计数；step summary 持久化保留该 scope，step ID、`runtimeRepairAttempt`、`contractViolationRecovery.attempt` 与 budget scope 使用同一 attempt。pending/未提交 dispatch 不消费 attempt。
- RC03 已完成：runtime/contract retry attempt 1/2/3 分别执行至少 shrinkLevel 2/3/4；provider dynamic tail 去重 coverage context，压缩 contract violation lineage、repair package 和 process evidence，只保留 current action/focus/requiredOutput、最新失败与必要 target evidence。
- 40/89 coverage item fixture 均验证 request 不超过 96 KiB、attempt 间单调不增长、context gate 为 `pass`；stable prefix hash 回归继续通过。
- Focused tests、`go test -p 1 ./internal/modules/... -count=1`、`go test ./... -count=1`、`make fmt-check`、`make vet`、`make verify-boundary-contract` 与 `git diff --check` 已通过。
- `make build` 已通过。本文状态暂不改为 `completed`：仍需部署最新服务，并对 LSP 与网关真实 workspace 执行下方 Final Live Closure Gate。

必须按以下顺序执行，不得先通过扩大 provider context budget 绕过问题：

1. 完成 RC01 convergence transition。
2. 完成 RC02 scoped retry attempt。
3. 完成 RC03 provider dynamic tail。
4. 运行 focused tests。
5. 运行全量测试和边界门禁。
6. 构建并重新部署最新服务。
7. 对 LSP 与网关两个真实 workspace 再次执行 CC06 live 验证。

#### Focused Tests

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
```

#### Full Gates

```bash
go test -p 1 ./internal/modules/... -count=1
go test ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

#### 最终 Live Closure Gate

- [ ] 不再出现 `executionContext.stagePacket.requiredOutput` mirror mismatch。
- [ ] 不再出现 `contractState=need_state_change requires state-changing requiredOutput`。
- [ ] 同一 scoped contract retry attempt 连续递增。
- [ ] retry request bytes/tokens 不随 attempt 增长，contract retry 不超过 96 KiB。
- [ ] stable prefix hash 保持稳定。
- [ ] 至少一个真实 workspace 从 `cwi-001` 连续推进至后续多个 focus。
- [ ] active outcome、terminal Run/Result、focus、runtime execution 与资源释放继续一致。
- [ ] CC06 完成后才允许把本文状态改为 `completed`。

## Third Deployment Live Verification and Reopened Closure Plan

本节记录提交 `b5db0766` 部署后的第三次 live 验证结果，并作为下一轮逐项代码收口入口。此前 RC01-RC03 的 focused fixture 继续有效，但真实运行链证明其覆盖面不足，因此相关 live 项重新打开；不得仅依据单元测试勾选结果把本文改为 `completed`。

### 部署与服务状态

- 部署进程：PID `2329918`，启动时间 `2026-07-11 21:33:48 +08:00`。
- 服务监听：`127.0.0.1:8080`。
- `/readyz`：`ready`。
- 全局 source/projected revision：`2686 / 2686`。
- LSP workspace source/projected revision：`2244 / 2244`。
- 网关 workspace source/projected revision：`2686 / 2686`。
- 两个 terminal Run 均已释放 `activeRunId`，runtime resources 均为 released。

### Live Run 结果

| Workspace | Run | 结果 | Focus 进展 | 最终失败 |
| --- | --- | --- | --- | --- |
| `ws-5cfc9874cebda0b8`，LSP-Bridge MVP | `run-004` | `failed` | 从 `cwi-036` 前移到 `cwi-039`，但未连续完成多个 focus | `validation_static_only` 后 repair dispatch 出现 `contractState=need_state_change requires state-changing requiredOutput` |
| `ws-609ce9149734ebb7`，轻量级 LLM 本地代理网关 | `run-005` | `failed` | 停留在 `cwi-085` | build validation failure 后 runtime repair 实际 `0` 次即出现相同 state/requiredOutput 冲突 |

### 已通过的 live 子项

- [x] 服务 readiness 与 projection revision 对齐。
- [x] active outcome 指向当前 Run，terminal outcome/Run/Result 指向一致。
- [x] terminal Run 已释放 `activeRunId`。
- [x] LSP `run-004` 的 37 个 runtime resources 全部 released。
- [x] 网关 `run-005` 的 runtime resources 全部 released。
- [x] contract retry 请求 `60,344 bytes`，低于 96 KiB。
- [x] runtime repair attempt 已真实递增，attempt 2/3/4+ 分别进入 shrinkLevel 3/4。
- [x] 两个 workspace 的非空 `stablePrefixHash` 与 `renderedStablePrefixHash` 各自保持单值。

### 重新打开的 live 子项

- [ ] `validation_static_only` 必须生成真实 managed-validation correction profile，而不是 generic correction。
- [ ] forced-narrow validation correction 不得被仍缺 `state_change` 的 owner focus 覆盖。
- [ ] state、nextAction、runtime stage、repair objective 与 requiredOutput 必须在 dispatch 构造阶段原子切换。
- [ ] 首次 contract violation 的 budget scope 必须包含 `violatedRule`、`correctionProfileId` 与 attempt。
- [ ] provider retry 不得从父 contract/runtime retry 的压缩结果重新膨胀。
- [ ] 同一 recovery scope 的 request bytes/tokens 必须单调不增。
- [ ] authority diagnostics 不得按每轮 dispatch 重复累积。
- [ ] 至少一个 workspace 连续完成两个以上 coverage focus 前移。

## Reopened Closure Execution Plan

必须严格按 RC05 → RC06 → RC07 → RC08 → RC09 顺序执行。RC05-RC07 属于 `kernel/orchestrator` owner 语义；RC08 属于 `blocks/llm` 请求预算。不得把 convergence 判断、focus 裁决或 correction profile 选择下沉到 LLM block。

### RC05：收口 validation violation correction profile

#### Live 根因

`validation_static_only` 虽被 `contractViolationRuleRequiresManagedValidationNarrow` 标记为 forced-narrow，但 `contractViolationCorrectionProfileForStep` 的 validation correction switch 未包含该 rule，真实输出仍为：

```text
correctionProfileId=contract_correction_generic
correctionMode=targeted
requiredOutputShape=expected_contract_compliant_output
```

因此 focused test 中手工注入的 `managed_validation_evidence` 没有覆盖真实 correction profile 生成链路。

#### 必须修改

- 将 `validation_static_only` 纳入 validation correction profile 分支。
- validation scratch/process-control/detached/shell-safety/static-only/managed-required 使用同一结构化 rule 集合，避免 recoverability、retry hint 与 correction profile 三处列表漂移。
- forced-narrow 时必须生成：

  ```text
  correctionProfileId=managed_validation_evidence
  correctionMode=forced_narrow
  requiredOutputShape=managed_validation_evidence
  validationRoute=managed_harness
  requiredOutput=validation/managed_session only
  ```

- generic correction 不得处理已知 validation contract rule。

#### 目标文件

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`

#### 回归测试

- [ ] 使用真实 `attachContractViolationDiagnostics` 生成 `validation_static_only` violation。
- [ ] 使用真实 `contractViolationCorrectionProfileForStep`，禁止手工注入 correction contract。
- [ ] attempt 1 即产生 managed-validation forced-narrow profile。
- [ ] recovery input、budget scope、CurrentActionContract 和 requiredOutput mirrors 均使用同一 profile。

### RC06：收口 forced-narrow correction 与 owner authority 优先级

#### Live 根因

`dispatchInputShouldPreserveLocalRepairAuthority` 当前在 focus 仍缺 `state_change` 时拒绝 local validation authority，并持续写入：

```text
local_validation_repair_authority_rejected
```

LSP `run-004` 最终累计 43 条 authority diagnostics。该规则把“当前 validation action 的合同修正”误当成“覆盖 owner 最终 convergence focus”，导致 forced-narrow validation correction 被旧 state-change authority 覆盖。

#### 必须修改

- 区分两类 local validation authority：
  - 普通 validation repair authority。
  - 当前 action 的 contract correction authority。
- 当前失败为 validation contract violation 时，必须先完成当前 validation action correction；focus 同时缺少 `state_change` 不能覆盖该 correction。
- correction 成功并获得有效 validation evidence 后，再由 owner coverage ledger 决定是否进入 state-change。
- 只有最新失败自身包含 admissible file target、line refs、patch repair context、failure target snapshot 或等价 implementation diagnostic 时，才允许直接切换 state-change。
- `cwi-*`、`source_change`、deliverable scope、coverage target refs 等抽象引用不能单独作为 implementation target evidence。
- 删除或降级重复 `local_validation_repair_authority_rejected`，同一 authority snapshot/focus/code 只保留一条。

#### 目标文件

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_policy.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`

#### 回归测试

- [ ] focus 同时缺 validation/state_change，最新失败为 `validation_static_only` 时保持 validation correction。
- [ ] build validation failure仅携带 coverage refs 时不得进入 state-change。
- [ ] build validation failure携带真实 file/line/snapshot evidence 时允许进入 state-change。
- [ ] correction 成功后由 owner authority 再次计算下一 convergence state。
- [ ] 同一 authority warning 不重复累积。

### RC07：建立唯一原子 dispatch contract builder

#### Live 根因

当前 dispatch 构造依次执行 authority injection、Build/Apply、requiredOutput seed restore、StagePacket attach、CurrentActionContract attach、mirror sync、再次 attach 与 semantic validation。中间态仍可能出现：

```text
state=need_state_change
requiredOutput=validation-only
```

网关 `run-005` 在 runtime repair 实际 `0` 次时即在该中间阶段失败；终态持久化 mirrors 已经一致，证明错误发生在 dispatch 构造而非 application projection。

#### 必须修改

建立唯一原子入口，顺序固定为：

```text
resolve effective owner/correction authority
→ BuildConvergenceContract
→ derive canonical requiredOutput from selected state
→ derive nextAction/runtime stage/repair objective
→ build CurrentActionContract
→ build StagePacket from CurrentActionContract
→ synchronize all mirrors
→ seal authority lineage
→ single semantic preflight
```

- StagePacket 不得在 canonical CurrentActionContract 之前构造。
- 不再通过旧 requiredOutput seed 恢复与 selected state 相反的合同。
- `attachCurrentActionContractToInput` 不得在同一次 render 中重复调用。
- `need_state_change` 原子清理 validation-only 字段。
- `need_validation` 原子清理 state-changing-only 字段和旧 patch package。
- semantic preflight 只在完整 canonical input 上执行一次。

#### 目标文件

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_current_action_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`

#### 回归测试

- [ ] 网关 live 形态：build validation failure → runtime repair dispatch 完整链路。
- [ ] LSP live 形态：validation static-only → forced-narrow → next dispatch 完整链路。
- [ ] 任意中间旧 mirror 不参与 canonical contract 计算。
- [ ] provider dispatch 前所有 requiredOutput mirrors 完全一致。
- [ ] 不再出现 `contractState=need_state_change requires state-changing requiredOutput`。

### RC08：统一 recovery scope、request ceiling 与 dynamic tail

#### Live 根因

- 首次 projected `retryBudgetScope` 仍未完整展示 `violatedRule/correctionProfileId/attempt`。
- LSP contract retry 为 `60,344 bytes`，其 provider retry 膨胀至 `122,868 bytes`。
- 网关 runtime repair 为 `59,620 bytes`，provider retry 膨胀至 `110,952 bytes`。
- 同一 LSP convergence focus 的 shrinkLevel 4 请求仍出现 `61,786 → 62,472 → 66,491 bytes` 增长。
- authority diagnostics 与重复 lineage 持续增长，重新进入 protected/dynamic context。

#### Orchestrator 必须修改

- 首次创建 ContractViolation 时即持久化完整 scope：

  ```text
  workspaceId
  runId
  rootStepId
  focusId
  violatedRule
  correctionProfileId
  attempt
  maxAttempts
  ```

- recovery input 增加中立请求预算合同：

  ```text
  previousSubmittedRequestBytes
  maxRecoveryRequestBytes
  parentShrinkLevel
  ```

- provider retry 继承父 contract/runtime retry 的 scope、attempt lineage 与 request ceiling。
- 只有实际提交 provider 的请求才更新 previousSubmittedRequestBytes。
- authority diagnostics 按 `authoritySnapshotId + focusId + code + source` 去重，历史项只保留 count/refs。

#### LLM block 必须修改

- `blocks/llm` 只消费请求预算合同，不判断 convergence state。
- provider retry 的 shrinkLevel 不得低于 parentShrinkLevel。
- provider retry 不得恢复父请求已删除的 historical context。
- 当前 request ceiling：

  ```text
  min(configuredLimit, maxRecoveryRequestBytes, previousSubmittedRequestBytes)
  ```

- attempt 3+ 即使同为 shrinkLevel 4，也必须受上一有效 request bytes 上限约束。
- Goal/DoD/current action/current focus/requiredOutput/必要 target evidence 继续受保护。
- retry 变化仍只能影响 C3-C5，稳定前缀不变。

#### 目标文件

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- `internal/pkg/repository/file/file.go`
- `internal/pkg/repository/memory/memory.go`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`

#### 回归测试

- [ ] contract retry → provider retry 不增长且均低于 96 KiB。
- [ ] runtime repair → provider retry 不增长且均低于 96 KiB。
- [ ] attempt 1/2/3/4 请求 bytes/tokens 单调不增。
- [ ] 同一 scope 的 step ID、input、budget、audit attempt 完全一致。
- [ ] scope 变化后 request ceiling 和 attempt 正确重置。
- [ ] stable prefix 两类 hash 保持单值。
- [ ] diagnostics 数量在重复 retry 中保持有界。

### RC09：重新部署与双 workspace live closure

#### 执行顺序

1. 完成 RC05 correction profile。
2. 完成 RC06 authority priority。
3. 完成 RC07 atomic dispatch builder。
4. 完成 RC08 scoped request ceiling。
5. 运行 focused tests。
6. 运行全量测试和边界门禁。
7. `make build` 并部署最新服务。
8. 启动 LSP 与网关两个全新 Run。
9. 执行最终 live gate。

#### Focused Tests

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/pkg/repository/... -count=1
```

#### Full Gates

```bash
go test -p 1 ./internal/modules/... -count=1
go test ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

#### Final Live Gate

- [ ] 两个新 Run 均不再出现 state/requiredOutput semantic conflict。
- [ ] `validation_static_only` 使用真实 managed-validation forced-narrow correction。
- [ ] forced-narrow correction 后能够继续 provider dispatch，而不是实际 0/1 次即终止。
- [ ] contract/runtime/provider retry attempt 连续且 scope 字段完整。
- [ ] contract/runtime/provider retry request bytes/tokens 单调不增。
- [ ] 所有 recovery request 不超过 96 KiB live 门禁。
- [ ] stable prefix hash 保持稳定。
- [ ] authority diagnostics 不再随 retry 线性增长。
- [ ] 至少一个 workspace 连续完成两个以上 coverage focus 前移。
- [ ] Run/Result/outcome、activeRunId、runtime execution、projection revision 和资源释放继续一致。
- [ ] 全部通过后才允许将本文状态改为 `completed`。

### RC05-RC08 代码收口执行记录（2026-07-11）

当前代码收口已按 RC05 → RC06 → RC07 → RC08 顺序完成；RC09 已执行重新部署与双 workspace live gate，但未通过 request budget 单调性和 protected floor 门禁，因此本文继续保持 `active` 并进入 RC10。

| 收口项 | 状态 | 已完成内容 |
| --- | --- | --- |
| RC05 | 代码完成 | `validation_static_only` 首轮直接进入 `managed_validation_evidence` forced-narrow correction；真实 diagnostics → repair input → dispatch preflight 链路已覆盖。 |
| RC06 | 代码完成 | 区分普通 validation repair 与当前 validation contract correction；抽象 state-change coverage gap 不再覆盖正在执行的 correction，普通 local validation authority 仍服从 owner authority。 |
| RC07 | 代码完成 | 删除旧 requiredOutput seed 反向恢复和重复 render；固定为 owner/correction authority → convergence contract → CurrentActionContract → StagePacket → mirrors → seal → preflight。 |
| RC08 | 代码完成 | recovery scope 补齐 workspace/run/root/focus/rule/profile/attempt；provider retry 继承父请求 bytes ceiling 与 shrink level；authority diagnostics 聚合去重并限制为 32 条。 |
| RC09 | live 未通过 | 最新服务已重新部署并创建两个全新 Run；LSP 能连续推进多个 focus，但网关 recovery request 出现 protected P0 overflow 和 provider retry bytes 反向增长，需要继续执行 RC10。 |

本轮新增或强化的关键回归包括：

- 真实 `validation_static_only` correction profile 生成，不再由测试手工注入。
- validation correction 与抽象 state-change gap 的 authority 优先级。
- stale requiredOutput mirrors 不得参与 canonical dispatch contract 计算。
- contract/runtime retry 到 provider retry 的请求 bytes ceiling 与 parent shrink 继承。
- recovery request 在同一 scope 内 bytes 单调不增。
- authority diagnostics 按 authority snapshot、focus、code、source 聚合并保持有界。

已通过门禁：

```text
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/pkg/repository/... -count=1
go test -p 1 ./internal/modules/... -count=1
go test ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
```

其中首次并行执行 `go test ./... -count=1` 时，`workorchd/service/server` 出现一次后台 routine 关闭时序失败；该包单独复跑通过，随后完整 `go test ./... -count=1` 复跑通过。未据此修改与本轮收口无关的 server 代码。

## Fourth Deployment Live Verification and RC10 Closure Plan

核对时间：2026-07-11 23:09 至 23:14。

部署进程：

```text
PID=2365563
startedAt=2026-07-11 23:09:06 +08:00
/readyz=ready
global source/projected revision=2753/2753
```

本轮使用部署后新建的两个 Run：

- LSP：`ws-5cfc9874cebda0b8/run-005`
- 网关：`ws-609ce9149734ebb7/run-006`

### Live 正向证据

#### LSP Run

- current focus 连续前移：

  ```text
  cwi-036 → cwi-039 → cwi-001
  ```

- 已成功执行真实状态变更：
  - `patch/apply` 创建或更新实现文件。
  - `git/write` add。
  - `git/write` commit。
- 未再出现：

  ```text
  contractState=need_state_change requires state-changing requiredOutput
  currentActionContract requiredOutput does not match active required output
  ```

- 两组 contract retry 均正确继承父请求 ceiling 并缩小：

  ```text
  88,407 → 80,946 bytes
  91,600 → 81,609 bytes
  ```

- recovery scope 已包含 workspace/run/root/focus/rule/profile/attempt。
- `stablePrefixHash` 和 `renderedStablePrefixHash` 在当前 workspace 内均保持单值。
- authority diagnostics 未再出现旧部署中的 43 条线性增长；历史 warning 当前保持有界。

#### 网关 Run

- 初始 schema violation 正确生成：

  ```text
  violatedRule=unparseable_output
  correctionProfileId=json_output_shape
  correctionMode=targeted
  ```

- 后续 focus-binding violation 正确生成：

  ```text
  violatedRule=focus_binding_mismatch
  correctionProfileId=focus_binding_contract
  focusId=cwi-085
  ```

- contract/provider retry 的 scope、attempt、parent shrink 和 request ceiling 均已持久化。
- stable prefix 两类 hash 均保持单值。
- Run 阻塞后 runtime resources 已释放。

### Live 未通过证据

网关请求序列：

```text
initial                         100,587 bytes
contract retry 1                61,178 bytes
contract retry 2                63,952 bytes (ceiling=61,178)
provider retry 1                62,759 bytes (ceiling=61,178)
provider retry 2                62,847 bytes (ceiling=61,178)
```

失败表现：

```text
llm provider request too large:
p0 overflow with protected goal/dod contract
```

结论：

- request ceiling 已正确继承，但 protected P0 的实际最小渲染尺寸超过 ceiling。
- provider retry attempt 2 比 attempt 1 增长 88 bytes，未满足同 scope 单调不增。
- 当前失败发生在本地 request rendering/budget gate，不是 provider 配置或网络故障。
- 生成的 decision ticket 却要求用户修复或切换 provider，failure classification 与治理提示错误。
- `validation_static_only` 本轮未再次触发，因此 RC05 仍缺新部署后的真实 managed-validation live 证据。
- LSP Run 仍为 active，尚不能核对 terminal activeRunId、Result/outcome 和最终资源释放。

### RC10：收口 protected request floor、deterministic retry 与 budget failure classification

必须严格按 RC10.1 → RC10.2 → RC10.3 → RC10.4 → RC10.5 顺序执行。

#### RC10.1：收缩 LLM protected P0

目标：保护当前动作语义，不保护重复诊断正文。

必须保留：

- Goal/DoD 原始定义与 effective contract。
- current focus。
- current action。
- canonical requiredOutput。
- 当前 correction 的 rule/profile/mode/requiredOutputShape。
- 当前动作必要 target evidence。

必须移出 P0 或压缩为结构化引用：

- `contractViolationRecovery.actualOutputSummary` 全量内容。
- 重复 failure signature 和自然语言 error/reason。
- 历史 repair guidance。
- 旧 authority diagnostics 正文。
- 旧 expected contract、repair package 和 stage packet mirrors。
- 非当前 scope 的 rejected output/history。

详细诊断只能进入 bounded C4/C5 summary 或 evidence refs，不得与当前 correction contract 并列成为 protected instruction。

目标文件：

- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`

#### RC10.2：建立 protected floor 与 request ceiling 合同

保留三个独立字段：

```text
previousSubmittedRequestBytes
maxRecoveryRequestBytes
minimumProtectedRequestBytes
```

有效 ceiling：

```text
effectiveCeiling = min(configuredLimit, maxRecoveryRequestBytes)
targetBytes = min(previousSubmittedRequestBytes, effectiveCeiling)
```

若 `minimumProtectedRequestBytes > targetBytes`：

1. 压缩 diagnostic lineage。
2. 移除非当前 scope 的 C3-C5 context。
3. 移除父请求已删除的历史内容。
4. 重新计算 protected floor。
5. 仍超过 ceiling 时返回本地结构化 blocker，禁止调用 provider。

不得通过提高或忽略 inherited ceiling 掩盖问题。

#### RC10.3：provider retry 复用父请求压缩结果

- provider retry 必须从父步骤实际提交的 normalized provider input 派生。
- 不得从更完整的原始 Step input 恢复历史 context。
- retry 只允许修改 attempt、当前 failure reason 和必要 audit lineage。
- 同一 scope、相同 shrink level 的序列化结果必须确定。
- attempt 1-4 的 bytes/tokens 必须单调不增。
- provider retry 的 `maxRecoveryRequestBytes` 不得被当前失败请求的超限 `actualBytes` 放大。

目标文件：

- `internal/modules/kernel/orchestrator/biz/runtime_repair_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`

#### RC10.4：拆分本地 budget failure 与 provider failure

至少拆分以下 failure class：

```text
provider_context_too_large
runtime_request_budget_overflow
protected_context_floor_exceeded
```

- `provider_context_too_large`：provider 明确拒绝实际提交的请求。
- `runtime_request_budget_overflow`：本地最终 request 超过 effective ceiling，尚未调用 provider。
- `protected_context_floor_exceeded`：必要保护合同经最小化后仍超过 ceiling。

后两类属于 runtime/LLM rendering contract failure：

- 不得计为 provider timeout/configuration retry。
- 不得生成“修复或切换 provider”的 decision question。
- 应进入明确的 runtime budget blocker、bounded re-render 或内部治理分支。

目标文件：

- `internal/modules/kernel/orchestrator/biz/failure_classification.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_governance_localization.go`

#### RC10.5：回归与重新部署门禁

必须增加以下 fixture：

- focus-binding correction 的父请求为 `61,178 bytes`。
- 新 correction 加入后仍不超过父请求和 96 KiB。
- provider retry attempt 1-4 bytes/tokens 单调不增。
- shrink level 4 连续 retry 的 normalized input 和 request bytes 确定一致或持续缩小。
- stable prefix 两类 hash 保持单值。
- Goal/DoD/current focus/current action/requiredOutput 无语义丢失。
- protected floor 无法满足 ceiling 时不调用 provider。
- 本地 budget blocker 不生成 provider configuration decision。
- scope 变化时 ceiling、attempt 和 normalized parent input 正确重置。

Focused tests：

```bash
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelports -count=1
```

Full gates：

```bash
go test -p 1 ./internal/modules/... -count=1
go test ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

重新部署后必须创建两个全新 Run，并满足：

- LSP 连续完成两个以上 focus 前移。
- `validation_static_only` 若触发，必须使用真实 managed-validation forced-narrow correction。
- 网关所有 contract/runtime/provider recovery request 均低于 96 KiB。
- 同一 recovery scope request bytes/tokens 单调不增。
- protected floor overflow 不调用 provider且不误报 provider 配置问题。
- stable prefix 两类 hash 保持单值。
- terminal Run 的 activeRunId、runtime resources、Run/Result/outcome 和 projection revision 一致。

RC10 完成并通过重新部署 live gate 前，本文不得标记为 `completed`。

### RC10 代码收口执行记录（2026-07-11）

RC10.1 至 RC10.5 的代码侧收口与门禁已完成，等待重新部署后的双 workspace live 验证。本文继续保持 `active`。

已完成：

- shrink level 2+ 不再保护完整 `lastFailureDiagnostics`、`previousRejectedOutputSummary`、旧 expected contract 和 strategy reassessment。
- `contractViolationRecovery.actualOutputSummary` 收缩为 status/stage/failureClass/violationType/rejectedOutputRef 等结构化最小摘要。
- shrink level 4 删除 failure reason/signature、failed step input/output、repair guidance、authority diagnostics 和旧 current-action rejected diagnostic。
- context budget 新增：

  ```text
  minimumProtectedRequestBytes
  budgetFailureClass
  providerSubmitted
  ```

- 只有实际进入 provider 调用的请求才设置 `providerSubmitted=true`。
- 本地 overflow 不再使用其超限 `actualBytes` 更新 `previousSubmittedRequestBytes` 或放大 recovery ceiling。
- provider retry 保留父 recovery root，并单独记录 `providerRetryRootStepId`、`parentLineageType` 和 `providerRetryAttempt`。
- 新增本地 failure class/stage：

  ```text
  runtime_request_budget_overflow
  protected_context_floor_exceeded
  llm_runtime_request_budget_overflow
  llm_protected_context_floor_exceeded
  ```

- 两类本地 budget failure 不再属于 provider failure，不触发 provider retry/failover/provider configuration ticket。
- 本地 budget failure 进入 context-budget gate，并明确提示该问题属于 runtime request rendering，而不是 provider 配置。

新增 live 形态回归：

- 以 `61,178 bytes` 作为 inherited parent ceiling。
- provider retry attempt 1-4 均不超过 parent ceiling，且 bytes 单调不增。
- shrink level 4 不恢复 verbose authority diagnostics 和 rejected output。
- local overflow 在 `providerSubmitted=false` 时保留上一实际提交请求 `61,178 bytes`，不使用本地计算的 `63,952 bytes` 更新 ceiling。
- 本地 budget/floor failure 由 orchestrator 识别为 context gate，不进入 provider retry。
- governance question 不再要求修复或切换 runtime LLM provider。

已通过：

```text
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelports -count=1
go test -p 1 ./internal/modules/... -count=1
go test ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
make build
git diff --check
```

下一步必须重新部署当前构建，并使用两个全新 Run 执行 RC10 live gate；未通过 live gate 前不得将 RC10 或本文标记为 `completed`。

## Fifth Deployment Live Verification and Reopened Closure Plan

核对时间：2026-07-11 23:54 至 2026-07-12 00:03。

部署版本：

```text
commit=88d9ef2c1854cf8914b695c48050f7f3cdf33e0b
PID=2388486
startedAt=2026-07-11 23:54:42 +08:00
/readyz=ready
global source/projected revision=2783/2783
workspaceTotal=2
```

本轮部署后有效 Run：

- LSP：`ws-5cfc9874cebda0b8/run-005-replan-001`
- 网关：`ws-609ce9149734ebb7/run-006-replan-001`

### RC10 Live 已通过子项

LSP Run 已产生 RC10 所需的真实 managed-validation correction profile：

```text
violatedRule=validation_static_only
correctionProfileId=managed_validation_evidence
previousSubmittedRequestBytes=110940
recoveryRequestBytes=90574
maxRecoveryRequestBytes=110940
minimumProtectedRequestBytes=90574
shrinkLevel=2
providerSubmitted=false
```

已确认：

- `validation_static_only` 已真实映射到 `managed_validation_evidence`，RC05 长期缺失的 live profile 证据已出现。
- recovery request 从 `110,940` 收缩至 `90,574 bytes`，低于 96 KiB 且满足 inherited ceiling。
- recovery 的 workspace、run、root step、focus、rule、profile、attempt 和 max attempts scope 均完整。
- 本地 context gate 失败未进入 provider retry；`providerSubmitted=false` 符合 RC10 failure classification。
- `stablePrefixHash` 与 `renderedStablePrefixHash` 在父请求和 recovery 中保持稳定。
- 当前失败不是 P0 overflow、provider timeout、provider configuration 或 projection lag。

### Live 新阻塞一：protected runtime contract 完整性误判

LSP contract retry 在请求已低于预算后被以下本地 gate 拦截：

```text
stage=llm_context_integrity_failed
contextGateAction=fail_fast
contextGateReason=protected_runtime_contract_changed_after_context_budget
providerSubmitted=false
```

Decision ticket 明确将依赖标记为 `runtime_context_gate`，允许动作仅为 `terminate` 或 `replan`。这说明当前阻塞发生在 `blocks/llm` 的本地 request rendering/integrity 判断，不是用户需要补充的信息或外部 provider 问题。

已定位的实现级原因：

1. `captureProtectedProviderRuntimeContract(input, adaptiveLevel)` 按 level 2 保存 compact recovery contract，但 `isProtectedProviderRuntimeContractEqual` 固定以 level 0 重捕获当前值，比较双方不处于同一 normalization level。
2. `restoreProtectedProviderRuntimeContract` 恢复后，`normalizeAdaptiveProviderRecoveryTail` 再次压缩 `contractViolationRecovery`；当前 `compactProviderContractViolationRecovery` 对已压缩输入不完全幂等，第二次压缩仍会删减字段或加入截断摘要。
3. `currentActionContract.currentFocus` 中空 evidence binding map 在预算处理后从 `{}` 变为 `null`，精确合同比较将该 transport normalization 判定为语义变化。
4. budget/rendering 阶段可能补充派生 runtime stage mirror；完整性检查若比较整个重捕获 map，会把原输入不存在的派生字段误判为受保护合同被修改。

因此 RC10 的 request ceiling、failure classification 和 provider submission gate 已部分通过，但 protected contract integrity 尚未闭环。

### Live 新阻塞二：authority recompute 选取陈旧 requiredOutput

网关初始 LLM Step 返回 continuable candidate decision，runtime 正确记录：

```text
violatedRule=coverage_assessment_incomplete
recoverability=recompute_authority
correctionProfileId=state_changing_required_output
focusId=cwi-085
```

但没有形成 contract retry 或 runtime repair Step，Run 以实际 0 次 retry 直接失败：

```text
runtime repair dispatch failed:
runtime_internal_contract_misalignment:
contractState=need_state_change requires state-changing requiredOutput
```

当前 snapshot 中 convergence authority、stage packet 和 current action 已包含 state-changing accepted kinds，但 dispatch semantic preflight 通过 `firstRequiredOutputForSemantics` 选取到的生效 requiredOutput 仍不具备 state-changing 语义。说明 diagnostic lineage、旧 assessment contract 或并列 requiredOutput mirror 仍可能先于 canonical current action 被选为 dispatch 权威。

该失败不是 contract violation 不可恢复：`coverage_assessment_incomplete` 已正确分类为 `recompute_authority`。真正问题是 authority recompute 完成后，current action、convergence contract 和 dispatch semantic requiredOutput 没有以同一个 canonical snapshot 原子提交。

### RC11：收口 adaptive protected runtime contract

#### Owner 与边界

- Owner：`blocks/llm`。
- 只负责 provider request rendering、budget normalization、protected contract restore 与 integrity gate。
- 不得在 LLM block 内修改 convergence state、coverage authority 或 recovery policy。

#### 必须修改

1. 在 `protectedProviderRuntimeContract` 中保存捕获时的 adaptive level 或等价 normalization profile。
2. `isProtectedProviderRuntimeContractEqual` 必须使用与 protected snapshot 相同的 normalization level，禁止固定以 level 0 比较 level 2+ snapshot。
3. `compactProviderContractViolationRecovery` 必须满足幂等合同：

   ```text
   compact(compact(x, level), level) == compact(x, level)
   ```

4. restore 与 adaptive normalization 只能形成一次确定性转换；不得在每个 budget 阶段重复压缩同一 protected contract。
5. 完整性检查只比较原始输入中实际受保护的字段；预算阶段新增的派生 mirror 不得被视为原合同字段变化。
6. 统一空集合语义。受保护合同中的空 map、空 slice 与 `null` 不得因 transport compaction 互换；如确需 canonicalization，捕获与比较必须使用相同 canonical representation。
7. 保持当前完整性门禁强度：Goal、DoD、current focus、current action、requiredOutput、runtime stage 和 correction contract 的真实删除、弱化或替换仍必须 fail fast。

#### 目标文件

- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/provider_context_budget_test.go`
- 与 recovery live fixture 直接相关的 LLM budget/cache tests。

#### 回归测试

- level 2、3、4 的 contract violation recovery 连续压缩两次结果完全一致。
- `validation_static_only -> managed_validation_evidence` fixture 在 96 KiB ceiling 下无 integrity warning。
- recovery request 不高于 `previousSubmittedRequestBytes` 与 `maxRecoveryRequestBytes`。
- `providerSubmitted` 仅在通过 context gate 并实际调用 provider 后变为 true。
- `{}`、`[]` 和 `null` 在 protected current action contract 中保持既定 canonical 语义。
- 删除 Goal/DoD、requiredOutput 或 correction required fields 时仍产生对应 integrity warning。
- stable prefix 两类 hash 不因 adaptive retry level 漂移。

### RC12：收口 recompute authority 的 canonical requiredOutput

#### Owner 与边界

- Owner：`kernel/orchestrator`。
- Orchestrator 继续作为 convergence、authority recompute、recovery 和 dispatch owner。
- 不修改 Run owner、Result owner、projection owner 或 EventHub 交互方向。

#### 必须修改

1. `coverage_assessment_incomplete + recompute_authority` 完成后，以 `currentActionContract.requiredOutput` 作为唯一 canonical 当前动作合同。
2. 从 canonical requiredOutput 同步重建以下 mirror：

   - 顶层 `requiredOutput`；
   - `repairActionPackage.requiredOutput`；
   - `stagePacket.requiredOutput`；
   - `stagePacket.nextAction.requiredOutput`；
   - `convergenceContract.state`、`repairObjective` 与 `stateChangingOutputRequired`。

3. `need_state_change` 必须同时满足：

   ```text
   stateChangingOutputRequired=true
   mustIncludeStateChangingNode=true
   acceptedKinds contains at least one allowed state-changing kind
   repairObjective=produce_state_changing_output
   ```

4. authority recompute 切换到 state change 时，清除 assessment-only、validation-only、failure-evidence acquisition 等不再生效的旧动作状态。
5. `firstRequiredOutputForSemantics` 或其替代实现必须优先读取 canonical current action；diagnostic lineage、violation expected contract、rejected output 和旧 repair package 只能用于诊断，不得参与当前 dispatch authority 选择。
6. authority finalization 与 mirror rebuild 必须作为一个原子 builder 输出；dispatch preflight 不得看到 state 已更新但 requiredOutput 尚未更新的中间态。
7. 真正无法形成合法 current action 时继续返回 `runtime_internal_contract_misalignment`，不得通过跳过 preflight 掩盖内部冲突。

#### 目标文件

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- authority/current-action/repair dispatch 相关测试文件。

#### 回归测试

- `coverage_assessment_incomplete + recompute_authority + need_state_change` 能成功创建第一个 recovery Step。
- retry attempt 从 0 进入 1，不因 dispatch semantic preflight 直接终止 Run。
- canonical current action 与全部 requiredOutput mirror 完全一致。
- 陈旧 assessment requiredOutput 不得覆盖 state-changing requiredOutput。
- state-changing accepted kinds 与当前 runtime capabilities 相容。
- 真正的 authority state/requiredOutput 冲突仍被 fail fast 拦截。

### RC13：验证与重新部署顺序

必须按以下顺序执行，避免在错误 authority contract 上验证 request rendering：

1. 完成 RC11 的 adaptive protected contract 幂等性和同级完整性比较。
2. 完成 RC12 的 canonical requiredOutput 原子构建与 semantic selection。
3. 运行 focused tests。
4. 运行全量组件、格式、vet 和 boundary gates。
5. 重新部署当前 commit 构建的新二进制。
6. 创建或恢复能够触发对应场景的新 Run，重新执行双 workspace live gate。

Focused tests：

```bash
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

Full gates：

```bash
go test -p 1 ./internal/modules/... -count=1
go test ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

### RC11-RC13 最终 Live Gate

LSP 必须满足：

- 再次触发 `validation_static_only` 并生成 `managed_validation_evidence` recovery。
- recovery request 小于 96 KiB，且不高于上一实际提交请求。
- 不再出现 `protected_runtime_contract_changed_after_context_budget`。
- context gate 通过后 recovery 实际提交 provider，并生成 `validation/managed_session` 或合同允许的真实治理 blocker。
- stable prefix 两类 hash 保持稳定。

网关必须满足：

- `coverage_assessment_incomplete` 进入 authority recompute。
- 至少生成一次 contract/runtime recovery Step，不再以实际 0 次 retry 终止。
- 不再出现 `contractState=need_state_change requires state-changing requiredOutput`。
- state-changing Step 明确绑定 `cwi-085` 或 runtime 当时的 canonical current focus。
- Run 正向进入验证、下一 coverage focus，或仅在真实外部依赖存在时进入治理阻塞。

公共门禁：

- `/readyz=ready`，source/projected revision 对齐。
- recovery request bytes/tokens 在同 scope 内单调不增。
- 本地 context/budget failure 不进入 provider retry，也不生成 provider configuration 提示。
- terminal Run 的 activeRunId、runtime resources、Run/Result/outcome 与 projection 一致。

RC11 至 RC13 和上述双 workspace live gate 全部通过前，RC10 只能标记为 `partially live verified`，CC01、CC02 与 CC06 继续保持 reopened/failed，本文不得标记为 `completed`。

### RC11-RC13 代码收口执行记录（2026-07-12）

RC11、RC12 的代码侧收口和 RC13 静态/测试门禁已完成，等待重新部署后的双 workspace live 验证。本文继续保持 `active`。

RC11 已完成：

- `protectedProviderRuntimeContract` 记录捕获时的 adaptive level，完整性比较使用相同 normalization level，不再固定回到 level 0。
- runtime contract equality 只比较原输入实际受保护的字段；budget/rendering 阶段新增的派生 stage mirror 不再被误判为原合同变化。
- `budgetProviderValue` 对已有 `_truncatedEntryCount` 采用累计且幂等的处理方式，不再把截断元数据作为普通 entry 重复截断。
- `compactProviderContractViolationRecovery` 保留已压缩 `actualOutputSummary.rejectedOutputRef`，level 2、3、4 连续压缩满足幂等合同。
- protected current action 中的非 nil 空 map 在 adaptive budget 后保持空 map，不再静默归一化为 `null`。
- Goal/DoD、runtime stage、current action 和 correction contract 的真实变化仍由 integrity gate 拦截。

RC12 已完成：

- requiredOutput mirror 同步优先读取已经生成的 `currentActionContract.requiredOutput`，并验证其与当前 convergence state/next action 语义一致。
- current action 缺失或与当前 state 不相容时，才回退到现有 authority-derived requiredOutput builder。
- semantic preflight 的首选 requiredOutput 改为 current action，其次才是顶层和其它 mirror；diagnostic lineage 或陈旧 repair package 不再优先于 canonical current action。
- mirror synchronization 继续覆盖顶层、repair package、stage packet、stage packet next action 和 execution context；真正未同步的 mirror 仍会 fail fast。
- 新增 `coverage_assessment_incomplete + recompute_authority + need_state_change` recovery dispatch 回归，确认第一个 retry attempt 能生成 canonical state-changing Step，而不是以实际 0 次 retry 终止。

新增或扩展的关键回归：

- adaptive level 2/3/4 contract violation recovery 幂等压缩。
- protected runtime contract 同级比较与派生字段隔离。
- protected current action 空集合保持。
- current action 覆盖 stale validation requiredOutput mirrors。
- semantic preflight 优先使用 canonical current action，同时继续检查全部 mirror 一致性。
- coverage assessment authority recompute 生成 attempt 1 state-changing recovery Step。

已通过：

```text
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test -p 1 ./internal/modules/... -count=1
go test -p 1 ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

并行执行 `go test ./... -count=1` 时，`internal/modules/application/workorchd/biz/appservice/TestReadApprovalWithBackendIncludesAutoGovernanceSummary` 曾在 `mcpregistry.Module.Start -> Hub.Subscribe` 触发一次 10 分钟测试超时。该测试单独复跑和 `appservice` 整包复跑均通过，随后串行 `go test -p 1 ./... -count=1` 全量通过。当前没有证据表明该 EventHub 测试夹具启动竞争由 RC11/RC12 修改引入。

下一步必须重新部署包含本轮修改的新二进制，并执行 RC11-RC13 最终 Live Gate。live 验证通过前，不得关闭 CC01、CC02、CC06 或将本文标记为 `completed`。

## Sixth Deployment Live Verification and RC14-RC16 Closure Plan

核对时间：2026-07-12 09:05 至 09:09。

部署版本：

```text
commit=c7c4186c274553b758dccc7f267888214f9b24dd
PID=2507953
startedAt=2026-07-12 09:05:31 +08:00
/readyz=ready
global source/projected revision=2998/2998
```

本轮有效 Run：

- LSP：`ws-5cfc9874cebda0b8/run-005-replan-001-replan-001`
- 网关：`ws-609ce9149734ebb7/run-007`

### Live 已通过子项

RC11 protected runtime contract 已通过真实 managed-validation recovery：

```text
workspace=LSP
violatedRule=validation_static_only
correctionProfileId=managed_validation_evidence
requestBytes=91488
maxBytes=113066
shrinkLevel=2
providerSubmitted=true
contextGateAction=pass
integrityWarnings=[]
```

已确认不再出现：

```text
protected_runtime_contract_changed_after_context_budget
```

Goal/DoD、current action、managed-validation requiredOutput 和 stable prefix 均保持完整。LSP recovery 已实际提交 provider，因此 RC11 可标记为 `live verified`。

网关 authority recompute 首段也已通过：

```text
initial=100609 bytes
contract retry 1=81871 bytes
contract retry 2=62222 bytes
```

- `coverage_assessment_incomplete` 正确进入 `recompute_authority`。
- 不再以实际 0 次 retry 直接终止。
- 两次 recovery request 单调下降、均提交 provider、无 integrity warning。
- 后续成功生成并执行绑定 `cwi-085` 的 `patch/apply`。
- `stablePrefixHash` 与 `renderedStablePrefixHash` 在当前 workspace 内保持稳定。

### Live 未通过子项一：重复 validation violation 再次切换为 state change

LSP managed-validation contract retry 的持久化输入完全一致：

```text
convergenceState=need_validation
nextAction=validate
repairObjective=produce_concrete_validation_evidence
stateChangingOutputRequired=false
acceptedKinds=[validation/managed_session]
```

顶层、repair package、stage packet、stage packet next action 和 current action requiredOutput 均为同一 managed-validation contract。但 provider 再次返回 `validation_static_only` 后，构建下一 recovery dispatch 时仍失败：

```text
contractState=need_state_change requires state-changing requiredOutput
```

因此当前问题不再是已持久化 Step input 的 mirror 不一致，而是“重复 validation violation -> 下一 recovery authority”生成路径重新应用通用 state-change policy，覆盖了当前 forced-narrow validation correction。

LSP Run 最终状态：

- Run：`failed`
- Workspace `ActiveRunID`：已清空。
- direct failure：第二次 static-only validation output。
- recovery termination：下一 recovery dispatch 内部 authority misalignment。

### Live 未通过子项二：protected floor 递归 context-gate retry

网关后续 managed-validation recovery 的 inherited ceiling 为 `81,018 bytes`，首次本地 floor failure：

```text
requestBytes=90515
minimumProtectedRequestBytes=90515
maxBytes=81018
failureClass=protected_context_floor_exceeded
providerSubmitted=false
shrinkLevel=4
p0Overflow=true
```

本地 failure classification、provider 隔离和 ticket 文案均正确：没有调用 provider，也没有提示用户修复 provider 配置。但 runtime 继续生成嵌套 context-gate retry：

```text
90515 -> 92002 -> 92173 -> 92345 -> 92516 -> 92687 bytes
```

全部请求均满足：

```text
providerSubmitted=false
contextGateAction=fail_fast
failureClass=protected_context_floor_exceeded
```

说明同一不可满足的 protected floor scope 被重复渲染，且 decision/governance lineage 使 request bytes 每轮增加约 171 bytes，违反同 scope 确定性和单调不增合同。

网关最终进入 `blocked_on_decision`，validation scratch resources 已释放；Workspace `ActiveRunID=run-007` 符合阻塞态 owner 语义。

### Live 未通过子项三：resolved Decision continuation 未原子推进

网关进入预算 Decision 后，scheduler 多次报告：

```text
owner_readback_not_advanced
blocked_on_resolved_decision
```

说明 Decision resolution 已存在，但 Run 仍保留原 `BlockingTicketID`，owner continuation 没有原子完成“消费 resolution、清理 blocker、创建下一动作或新 blocker”的状态迁移。该问题不是 projection lag；最终 source/projected revision 已对齐。

### RC14：重复 validation violation 保持 validation authority

#### Owner 与边界

- Owner：`kernel/orchestrator`。
- Orchestrator 负责 correction、convergence authority 和 recovery dispatch。
- `blocks/llm` 只返回结构化 violation，不得决定下一 convergence state。

#### 必须修改

1. 当前 violation 同时满足以下条件时，下一 recovery 必须继续保持 validation authority：

   ```text
   recoverability=narrow_contract
   correctionProfileId=managed_validation_evidence
   requiredOutputShape=managed_validation_evidence
   current focus missingEvidenceKinds excludes state_change
   ```

2. forced-narrow validation correction 的优先级必须高于：
   - 通用 `stateChangingRepairRequired`；
   - workspace execution contract 的 state-change policy；
   - 历史 failure targets、repair target 和 state-changing evidence；
   - rejected output diagnostic lineage。
3. `BuildConvergenceContract` 或 authority refresh 后再次应用当前 correction contract，并原子重建 current action 与全部 requiredOutput mirrors。
4. state-change evidence 已接受、当前只缺 validation 时，不得重新打开 state change。
5. 只有 coverage ledger 明确重新出现 `state_change` 缺口，才允许切换到 `need_state_change`。
6. 同一 correction scope 的重复 static-only violation 应增加 contract retry attempt，而不是转为普通 runtime repair。

#### 目标文件

- `internal/modules/kernel/orchestrator/biz/runtime_repair_policy.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`

#### 回归测试

- 连续两次 `validation_static_only` 均生成 managed-validation recovery。
- 第二次 violation 后仍为 `need_validation/validate`。
- state-change 已接受且只缺 validation 时，不出现 state-changing requiredOutput。
- coverage ledger 真实重新缺少 state_change 时允许切换。
- 重复 contract retry 的 attempt、budget scope 和 request ceiling 正确递增/继承。

### RC15：protected floor 首次失败后确定性阻塞

#### Owner 与边界

- Owner：`kernel/orchestrator`、`blocks/llm`。
- LLM block 负责确定 floor 与 rendering audit。
- Orchestrator 负责判定该 failure 是否仍可重渲染以及创建唯一治理 blocker。

#### 必须修改

1. `protected_context_floor_exceeded` 在 authority、definition、focus 和 ceiling 不变时属于确定性 request-scope 终态。
2. `shrinkLevel=4 && p0Overflow=true` 时直接停止重渲染，禁止生成 `context-gate-retry`。
3. 首次 floor failure 后创建或复用一个 runtime budget Decision；不得创建新的 LLM Step。
4. 建立稳定 dedupe signature，至少包含：

   ```text
   workspaceId
   runId
   recoveryRootStepId
   focusId
   effectiveCeiling
   minimumProtectedRequestBytes
   stablePrefixHash
   correctionProfileId
   ```

5. 相同 signature 再次出现时复用既有 ticket，不新增 Step、ticket、repair attempt 或 rejected evidence。
6. context-gate retry 仅允许用于仍存在可删除非保护字段且能够证明下一 normalized request 会缩小的场景。
7. 如需重新渲染，只能从首次 normalized provider input 派生；不得重新注入完整 failed step、decision resolution 或不断增长的 governance history。
8. 相同 scope 下 `minimumProtectedRequestBytes` 和 serialized request bytes 必须确定性稳定，不能每轮增加。

#### 目标文件

- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_policy.go`
- `internal/modules/kernel/orchestrator/biz/runtime_governance_localization.go`

#### 回归测试

- 第一次 floor overflow 后只产生一个 failed Step 和一个 ticket。
- 不出现嵌套 `/context-gate-retry/context-gate-retry`。
- floor overflow 后 provider 调用次数不增加。
- 同 scope request bytes/floor 不增长。
- 本地 ticket 继续说明 runtime rendering failure，不提示 provider 配置。
- authority、definition、focus 或 ceiling 真实变化后才允许建立新 recovery scope。

### RC16：resolved Decision 与 blocked Run 原子 continuation

#### Owner 与边界

- 状态 owner：`kernel/orchestrator`。
- 调度触发与只读核对：`application/workorchd` scheduler。
- scheduler 不得直接修补 Run/Decision owner state。

#### 必须修改

1. Decision resolve 后由 orchestrator owner 原子执行：
   - 验证 workspace/run/ticket scope；
   - 消费 resolution；
   - 清除旧 `BlockingTicketID`；
   - 创建唯一 continuation、新 blocker 或明确 terminal outcome；
   - 更新 Run state 并发布正式事件。
2. continuation dedupe key 必须包含 resolution ID 和 owner authority snapshot ID。
3. scheduler 对同一 resolved ticket 最多触发一次 resume command。
4. owner readback 未推进时只记录一次 durable diagnostic；不得反复触发相同 resume。
5. Run 仍需外部治理时必须创建新的 open ticket，不能继续引用已 resolved ticket。

#### 目标文件

- `internal/modules/kernel/orchestrator/biz/runtime_blocks.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_governance_continuation.go`
- `internal/modules/application/workorchd/biz/appservice` 中 scheduler continuation/readback 实现。

#### 回归测试

- resolved Decision 后旧 blocker 被清除并只创建一个 continuation。
- 同一 resolution 重放保持幂等。
- 无法继续时创建新的 open ticket，而不是引用 resolved ticket。
- scheduler 不产生重复 `owner_readback_not_advanced`。
- restart/projection rebuild 后 continuation 状态不漂移。

### RC14-RC16 执行顺序与门禁

必须按以下顺序执行：

1. RC14：先关闭重复 validation violation 的 authority 优先级。
2. RC15：再阻断 protected floor context-gate retry 递归。
3. RC16：最后收口 resolved Decision continuation。
4. focused tests。
5. 串行全量、格式、vet、boundary gates。
6. 重新部署并执行双 workspace live gate。

Focused tests：

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
```

Full gates：

```bash
go test -p 1 ./internal/modules/... -count=1
go test -p 1 ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

最终 live gate：

- LSP 第二次 `validation_static_only` 仍进入 managed validation，不再出现 state-change requiredOutput 冲突。
- 网关第一次 floor overflow 后只产生一个本地失败 Step 和一个 Decision。
- floor overflow 后 provider 调用数不增加。
- 不存在嵌套 `context-gate-retry`。
- stable prefix 不漂移，同 scope request bytes/floor 不增长。
- resolved Decision 不再产生重复 `owner_readback_not_advanced`。
- `/readyz=ready`，source/projected revision 对齐。
- blocked/terminal Run 的 runtime 与 validation resources 全部释放。
- Run、Workspace、Result/outcome 和 `ActiveRunID` 符合 owner 状态。

RC14 至 RC16 和上述 live gate 全部通过前，CC01、CC02、CC06 继续保持 reopened/failed，本文不得标记为 `completed`。

### RC14-RC16 代码收口执行记录（2026-07-12）

本轮已按 RC14 → RC15 → RC16 顺序完成代码收口，live gate 保持待重新部署验证。

#### RC14 执行结果

- 增加连续第二次 `validation_static_only` 的 live 形态回归：失败 Step 本身已是 managed-validation retry，历史 Run nextAction 仍为 state_change，但当前 coverage focus 已接受 state-change 且只缺 validation。
- 已确认下一 recovery 继续保持 `need_validation/validate`，所有 requiredOutput mirror 只接受 `validation/managed_session`。
- 已确认 contract retry attempt 从 1 递增到 2，并继承 `maxRecoveryRequestBytes`、`previousSubmittedRequestBytes` 和 shrink lineage。
- 当前实现已满足该形态，无需新增跨 owner 覆盖逻辑；本轮以回归测试锁定，防止后续 authority refresh 回退。

#### RC15 执行结果

- 新增确定性 protected floor 判定：`protected_context_floor_exceeded + shrinkLevel>=4 + p0Overflow=true + minimumProtectedRequestBytes>effective ceiling`。
- 相同 request scope 的 resolved Decision 不再生成 `context-gate-retry`；owner 直接保持 governed wait。
- request-scope 签名包含 workspace、run、recovery root Step、focus、effective ceiling、minimum protected bytes、stable prefix hash 和 correction profile。
- 相同签名复用并重新打开原 ticket，同时删除已失效 resolution；不新增 LLM Step、ticket 或 provider 调用。
- 只有 resolution 明确改变 authority、definition、focus、ceiling、stable prefix 或 correction profile 时，才允许建立新的 context rendering scope。

#### RC16 执行结果

- orchestrator 在确定性 floor resolution 无法改变 request scope 时，原子恢复为“原 Run 继续 blocked_on_decision + 原 ticket 重新 open + resolution 已消费”，不会留下 resolved ticket blocker。
- scheduler governance resume ref 增加 resolution ID 与 authority snapshot ID，并纳入 resume dedupe key。
- scheduler 对同一 resolution/authority scope 只执行一次 owner resume；owner readback 未推进时记录一次 durable diagnostic 并清除可重复执行的 pending 项。
- replan continuation 的既有 idempotency key 保持不变。

#### 已通过测试

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
go test -p 1 ./internal/modules/... -count=1
go test -p 1 ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

#### 待执行 live gate

- 重新部署后验证 LSP 第二次 static-only violation 仍进入 managed validation。
- 重新部署后验证网关 floor overflow 只保留一个 failed Step 和一个 open Decision，且 provider 调用数不增加。
- 验证不存在嵌套 `context-gate-retry`，同 scope request bytes/floor 与 stable prefix 不增长。
- 验证 resolved Decision 不再重复产生 `owner_readback_not_advanced`。
- live gate 通过前，CC01、CC02、CC06 状态继续保持 reopened/failed。

## Runtime V2 Deterministic Convergence Closure Plan

核对时间：2026-07-12 10:07 至 10:11。

### 第七次部署 Live 核对结论

部署进程监听 `127.0.0.1:8080`，最终核对时：

```text
/readyz=ready
global source/projected revision=3071/3071
LSP workspace source/projected revision=2869/2869
gateway workspace source/projected revision=3071/3071
```

部署二进制构建时间为 `2026-07-12 10:07:14 +08:00`，提交 `9c8f6dca` 时间为 `2026-07-12 10:07:28 +08:00`。二进制 build info 为：

```text
vcs.revision=c7c4186c274553b758dccc7f267888214f9b24dd
vcs.modified=true
```

该二进制包含提交前工作树修改，但不是提交后的 clean build。后续最终 live gate 必须使用 `vcs.revision` 等于目标提交且 `vcs.modified=false` 的可复现构建。

本轮新 Run：

- LSP：`ws-5cfc9874cebda0b8/run-006`。
- 网关：`ws-609ce9149734ebb7/run-007-replan-001`。

两个 Run 均完成了真实 provider 调用、状态变更或验证动作，但最终都在构造下一 recovery dispatch 时失败：

```text
contractState=need_state_change requires state-changing requiredOutput
```

LSP 失败前的持久化合同是正确的：

```text
state=need_validation
nextAction=validate
focus=cwi-036
missingEvidenceKinds=[validation]
acceptedEvidenceKinds=[state_change]
acceptedKinds=[shell/command,validation/managed_session]
mustIncludeStateChangingNode=false
```

`go vet` 返回真实、可定位诊断：

```text
server.go:29:6: main redeclared in this block
```

该验证失败应使 coverage ledger 重新打开 state-change，并原子生成 state-changing authority；当前实现只完成了部分切换，最终 authority 与 requiredOutput 分裂。

网关失败前的持久化合同同样正确：

```text
state=need_state_change
nextAction=state_change
focus=cwi-085
missingEvidenceKinds=[state_change]
acceptedEvidenceKinds=[validation]
mustIncludeStateChangingNode=true
```

LLM 返回 continuable Decision 后，`coverage_assessment_incomplete` 被识别为 `recompute_authority`，但 targeted correction 没有原子重建 state-changing requiredOutput，下一 dispatch 仍发生相同内部合同错位。

本轮没有产生新的 protected floor，最大 shrink level 为 `2`，所有相关 provider request 均实际提交，因此 RC15 只能标记为“本轮未复现”，不能标记为 live verified。历史 `owner_readback_not_advanced` 在多个 watchdog 周期内计数保持为 `1`，未重复增长，但本轮没有形成新的 resolved floor Decision 闭环，RC16 只能标记为部分通过。

两个 terminal Run 均已清空 Workspace `ActiveRunID`，source/projected revision 对齐，validation resource lifecycle 显示已释放。当前阻塞点已经收敛为 orchestrator 内部 authority 推导存在多个写入口，而不是 provider、projection、EventHub 或资源释放问题。

### 架构收口决策

从本节开始停止继续为 V1 recovery 增加单点条件和专用分支。RC14 至 RC16 保留为问题证据和回归样本，不再通过增加新的 mirror 修复、retry 类型或 correction 特例扩大 V1 状态空间。

本次明确不保证 V1 Run、旧 recovery lineage、旧 checkpoint 和旧事件重放的运行兼容性，因此不再采用旁路双栈。Runtime V2 将一次性成为唯一正式主链：

1. 不增加 V1/V2 运行期路由，不维护两套 authority、recovery 或 scheduler continuation。
2. `runtimeContractVersion=v2` 只作为持久化审计标识，不参与生产路径分支。
3. 新主链完整并通过静态测试后，在维护窗口内一次性停止 V1、执行运行态退役迁移并切换到 V2。
4. 所有未终态 V1 Run 统一标记为 `superseded`，原因固定为 `runtime_contract_v1_retired`，不再恢复或 replan。
5. Workspace、Goal、DoD、Profile、交付目录、已完成 Result/Artifact 和可验证 evidence 保留；V1 authority、retry、checkpoint、open blocker 只保留只读审计或被 supersede。
6. V2 使用单一 reducer、有限动作、sealed authority transaction 和最多一次同合同 LLM retry。
7. V2 主链切换完成后立即删除 V1 recovery authority、自愈、嵌套 retry 和兼容 continuation，不设置长期观察期。
8. 实施过程可以拆分为多个编译通过的提交，但生产运行时只允许一次主链切换，禁止长期双栈。

### V2 不变量

以下规则必须转化为具体类型、单元测试和 dispatch preflight，不能只作为文档约定：

1. 一个 authority snapshot 只对应一个 convergence state、一个 focus、一个 action 和一个 requiredOutput hash。
2. reducer 完成后，任何 helper 不得单独修改 state、focus、action、requiredOutput 或 state-changing requirement。
3. `need_state_change` 只能配 state-changing requiredOutput。
4. `need_validation` 只能配 validation requiredOutput。
5. 缺少 snapshot、manifest、failure target 或 capability 时只能进入 deterministic prerequisite，不得要求 LLM 自行读取或请求 continuable Decision。
6. LLM 输出只能提交到它收到的 authority snapshot；snapshot 已变化时，旧输出只能作为 diagnostic evidence。
7. 相同 authority snapshot 的 LLM 合同错误最多重试一次。
8. 每次 transition 必须增加 evidence revision、改变 focus、改变 authority snapshot、进入 governed wait 或进入 terminal；否则视为无进展并停止自动执行。
9. 相同 failure signature 在同一 focus 下不得生成第三个 LLM Step。
10. `runtime_internal_contract_misalignment` 属于 orchestrator 缺陷，不得再次交给 LLM repair。

### V2 Canonical Contract

V2 直接替换现有共享运行合同，不创建长期 `/v1`、`/v2` 双包。正式落点：

```text
internal/pkg/workorchruntimecontract
```

核心类型至少包括：

```go
type RuntimeFacts struct {
    WorkspaceID       common.WorkspaceID
    RunID             common.RunID
    DefinitionRevision string
    EvidenceRevision   string
    Focus              CoverageFocus
    Evidence           EvidenceLedger
    Failure            *FailureFact
    Prerequisites      PrerequisiteState
}

type RuntimeDecision struct {
    State             ConvergenceState
    Action            RuntimeAction
    Focus             CoverageFocus
    RequiredOutput    RequiredOutputContract
    Prerequisites     []RuntimePrerequisite
    AuthoritySnapshot AuthoritySnapshot
    Terminal          *TerminalOutcome
}

type AuthorityTransaction struct {
    AuthoritySnapshotID string
    FocusID              string
    Action               RuntimeAction
    RequiredOutputHash   string
    DefinitionRevision   string
    EvidenceRevision     string
    Attempt              int
}
```

V2 跨 module/block EventHub command、data、result 继续使用具体类型，由投递组件定义；禁止使用跨组件 `map[string]any`、JSON bridge、reflect contract 或 Envelope。内部 provider JSON 仅允许存在于 LLM adapter 边界。

### V2 有限动作集合

V2 初期只允许：

```text
AcquirePrerequisite
ProduceStateChange
ProduceValidation
RequestGovernance
Complete
Fail
```

以下概念只能作为 reason、failure class 或 attempt metadata，不能再成为独立 authority：

- runtime repair；
- contract retry；
- provider retry；
- context-gate retry；
- assessment-only repair；
- correction-specific next action；
- mirror recovery state。

### RV2-01：建立唯一 typed canonical contract

Owner：`internal/pkg/workorchruntimecontract`。

必须完成：

1. 定义 `RuntimeFacts`、`RuntimeDecision`、`RuntimeAction`、`RequiredOutputContract`、`AuthoritySnapshot` 和 `AuthorityTransaction`。
2. 类型必须表达互斥状态，避免使用布尔字段组合表达 action。
3. requiredOutput hash 只能由 canonical typed contract 计算。
4. 提供纯函数 normalization、hash 和 invariant validation。
5. 不依赖 orchestrator repository、service 或业务 profile 实现。
6. 直接替换生产运行合同；不新增长期 V1/V2 facade、adapter 或 runtime version switch。
7. 旧合同只允许在迁移脚本或 fixture 中读取，不能继续被生产 orchestrator、LLM block 或 scheduler import。

完成门禁：

- 所有 action/state 组合都有表驱动测试。
- 非法组合无法通过 invariant validation。
- hash 对字段顺序稳定，对语义变化敏感。
- 生产依赖扫描中不存在并行 V1/V2 runtime contract。

### RV2-02：实现单一 Runtime Reducer

Owner：`kernel/orchestrator`。

正式入口：

```text
RuntimeFacts + PreviousAuthority -> ReduceRuntimeState -> RuntimeDecision
```

必须完成：

1. reducer 为纯函数，不访问 repository、EventHub、provider 或全局配置。
2. reducer 是 state、focus、action 和 requiredOutput 的唯一推导入口。
3. validation success、validation failure、state-change success、execution failure、missing prerequisite 和 governance dependency 均转化为 typed facts 后进入同一 reducer。
4. LSP build failure 必须从 validation facts 原子生成 `ProduceStateChange`。
5. 网关 `coverage_assessment_incomplete` 必须从 coverage facts 原子生成 `ProduceStateChange`，不依赖 targeted correction 后处理。
6. reducer 返回后不再调用 V1 `stateChangingRepairRequired`、correction authority 或 mirror normalization。

建议新增 focused package：

```text
internal/modules/kernel/orchestrator/biz/runtimev2
```

完成门禁：

- LSP live facts fixture 归约为 state-change decision。
- 网关 live facts fixture 归约为 state-change decision。
- 相同 facts 始终生成相同 decision、snapshot 和 requiredOutput hash。

### RV2-03：runtime-managed Prerequisite Engine

Owner：`kernel/orchestrator`；资源 owner：`blocks/workspace`、capability owner module。

必须完成：

1. 在 LLM dispatch 前检查 manifest、target snapshot、failure target、capability、validation scratch 和 provider ceiling。
2. 缺失项生成 `AcquirePrerequisite`，由 orchestrator 通过 EventHub 调用资源 owner。
3. `file/list`、`file/read` 等 prerequisite 不属于 LLM state-changing requiredOutput accepted kinds。
4. snapshot 完整时禁止重复 list/read。
5. prerequisite result 写入 typed facts，再次调用 reducer。
6. module/block 间只通过 EventHub，不允许 orchestrator 直接访问 workspace repository/service。

完成门禁：

- 网关缺少 `go.mod`/source snapshot 时先执行 deterministic preflight。
- preflight 后 state-change LLM request 携带完整、安全 snapshot。
- 同一 snapshot 不重复读取。

### RV2-04：LLM Candidate Boundary

Owner：`blocks/llm`；authority owner 仍为 `kernel/orchestrator`。

LLM 只返回当前 action 对应的候选类型：

```go
type StateChangeCandidate struct {
    Operations []StateChangingOperation
}

type ValidationCandidate struct {
    Operations []ValidationOperation
}

type GovernanceCandidate struct {
    ExternalDependency ExternalDependency
}
```

必须完成：

1. LLM block 不返回或修改 convergence state、focus、authority 或 repair objective。
2. provider request 只渲染 sealed `RuntimeDecision`。
3. 第一次候选不符合合同，只允许使用完全相同的 authority transaction 重试一次。
4. 第二次仍无效时进入 governed wait 或 terminal，不重新解释 output 并生成新 recovery authority。
5. provider/schema/contract failure 不得隐式切换 action。

完成门禁：

- 相同 snapshot 两次请求的 stable prefix 和 requiredOutput hash 一致。
- 第二次无效输出后不存在第三个 LLM Step。
- LLM block 测试中不存在 convergence state 推导。

### RV2-05：sealed dispatch 与 evidence commit

Owner：`kernel/orchestrator`。

执行链固定为：

```text
Reduce
  -> PreparePrerequisites
  -> SealAuthorityTransaction
  -> Dispatch
  -> ValidateCandidate
  -> Execute
  -> CommitEvidence(snapshotId)
  -> Reduce
```

必须完成：

1. Step input 持久化完整 authority transaction。
2. candidate 和 execution evidence 必须携带 snapshot ID。
3. commit 前核对当前 authority snapshot；旧 snapshot 结果只能进入 diagnostic lineage。
4. evidence commit 成功后递增 evidence revision，再调用 reducer。
5. 同一 transaction 的重复 event/result 保持幂等。
6. scheduler 只触发 owner command，不直接修改 Run、authority 或 evidence。

完成门禁：

- stale output 不覆盖新 focus。
- duplicate event 不产生重复 transition。
- restart 后可从 persisted facts/transaction 恢复，不依赖进程内 map。

### RV2-06：一次性切换 orchestrator 主链

Owner：`kernel/orchestrator`、`kernel/run`、`application/workorchd` 装配层。

正式执行链必须替换为：

```text
LoadFacts
  -> Reduce
  -> PreparePrerequisites
  -> SealAuthorityTransaction
  -> DispatchCandidate
  -> ValidateCandidate
  -> Execute
  -> CommitEvidence
  -> Reduce
```

必须完成：

1. Start、Resume、Step completion、governance resolution 和 restart recovery 全部进入同一 V2 主链。
2. 不保留基于 `runtimeContractVersion` 的 V1/V2 分支。
3. Run 持久化 `runtimeContractVersion=v2` 仅用于审计。
4. `runtime_internal_contract_misalignment` 直接生成 typed internal terminal outcome，不进入 LLM repair。
5. scheduler 只投递 typed owner command 并读取 owner 结果，不维护独立 recovery state。
6. 生产入口切换提交必须同时关闭 V1 dispatch 注册和旧 scheduler continuation 入口。

主要目标文件：

- `internal/modules/kernel/orchestrator/biz/runtime_lifecycle.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_blocks.go`
- `internal/modules/kernel/run/pkg/models`
- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go`
- Orchestrator、Run、WorkorchD EventHub typed command/data/result 合同。

完成门禁：

- 所有生产 Start/Resume/Recovery 调用链只进入 V2。
- `rg` 扫描不存在生产 V1 dispatch 路由。
- restart 后通过 persisted facts/transaction 恢复。

### RV2-07：实现 V1 运行态退役迁移工具

Owner：`kernel/orchestrator`、`kernel/run`、`blocks/decision`、`blocks/approval`、projection owner。

本项只实现、测试 migration，并在生产数据副本上完成 dry-run；正式迁移在 RV2-08 维护窗口内执行。必须提供显式、幂等的 migration：

```text
runtime-v1-retirement
```

迁移规则：

1. 所有 V1 pending/active/blocked Run 标记为：

   ```text
   state=superseded
   reason=runtime_contract_v1_retired
   ```

2. 清理对应 Workspace `ActiveRunID`、`BlockingTicketID` 和 `BlockingApprovalID`。
3. 所有 V1 open Decision/Approval 标记为 superseded，不再触发 continuation。
4. 删除或关闭 V1 resolution resume pending、recovery checkpoint、provider retry 和 context-gate pending work。
5. 释放 validation、prerequisite、provider session、background routine 等运行资源。
6. Workspace、Goal、DoD、Profile、交付目录、completed Result/Artifact 和可验证 evidence 保留。
7. 历史 Run、Step、event、rejected evidence 保留只读审计，不参与 V2 facts 构造，除非通过明确 evidence import policy 接受。
8. 发布正式 Run/Workspace/governance 状态事件，并执行全量 projection rebuild。
9. migration 重复执行不得产生新事件风暴、重复 terminal result 或重复 supersede。

迁移输出至少包含：

- scanned V1 Run 数量；
- superseded Run 数量；
- 清理的 ActiveRunID 数量；
- superseded ticket/approval 数量；
- 释放的资源数量；
- projection rebuild revision；
- 无法迁移对象及原因。

完成门禁：

- production data copy dry-run 能完整列出所有待退役对象。
- fixture 正式迁移后不存在非终态 V1 Run、V1 open blocker 或 stale pointer。
- fixture migration 二次执行无状态变化。
- migration 输出、备份和失败恢复流程已有测试。

### RV2-08：删除 V1、执行一次性切换并完成双 workspace 验证

V2 主链实现完成且 migration 工具验证通过后立即删除 V1，不等待长期双栈观察；随后在维护窗口内执行正式迁移并启动 V2-only 服务。

RV2-08 首批兼容面删除记录（2026-07-12）：

- `ResumeRunCommand` 已删除 `ExecutionContextPatch map[string]any` 与 loaded Run payload，只保留具体 workspace/run scope；Orchestrator module 不再保留“接收后拒绝 patch”的伪兼容字段。
- Run owner event adapter、Orchestrator service facade、application run port/session/backend facade、embedded CLI 和 server session 已同步删除 patch resume 入口，不再把 compressed execution context 或 operator input map 注入 Resume。
- application activation event、HTTP/client DTO、embedded/remote backend port、CLI/TUI command 和 Web TypeScript surface 已进一步物理删除 `executionContextPatch` 字段及 merge helper；非空 patch 不再被接受、转码、拒绝或静默保留，新的 Runtime V2 Run 只由 typed bootstrap 建立定义与收敛状态。
- application bootstrap、runtime session 和 testkit 已删除无效的 RuntimePolicy 构造参数及 `NewWithBackend*RuntimePolicy*` facade；旧 policy 在生产构造链中此前仅被 `_ = runtimePolicy` 丢弃，现不再形成“策略已注入”的伪权威，配置管理读模型仍可从 canonical config 生成展示值。
- Run owner仍执行自身状态准入，但向 Orchestrator 只发送投递组件定义的 typed Resume command；workspace activation/restart 继续通过新的 typed RuntimeBootstrap 创建独立 V2 Run，不复用 Resume patch。
- application scheduler 对历史 `WorkspaceRunRequestedCommand` 中的非空 resume patch fail-fast；该 application 内部 activation payload 后续单独收口，不能重新映射到 Orchestrator command。
- 删除只验证 loaded Run/patch 兼容转发的测试，保留并更新 owner scope、状态准入与 typed scheduler command 回归测试。
- Orchestrator service facade 已删除无生产调用方、无 EventHub 正式合同的 `DispatchNextStep`；外部 module 无法再直接触发 V1 Step dispatch，Runtime V2 只允许通过 persisted drive/reducer 推进 operation。
- Orchestrator service facade 已继续删除 Start/Resume；正式 Start/Resume EventHub handler 直接执行 Runtime V2 owner projection和 persisted drive，不再向外暴露可调用 V1 lifecycle 的 service 方法。
- 已删除 Run service 到 Orchestrator 的 ValidateRun/CompleteRun 失效命令链，包括 Run DTO/service/adapter、Orchestrator topic/message/delivery contract、module subscription 与 facade；Runtime V2 validation 只接受 sealed operation evidence，Complete 只接受 reducer terminal projection。
- Run module service wrapper 已收窄为仅处理正式 EventHub Create/Ensure handler；Start/Resume/Block/Cancel/Supersede/Fail 不再通过 module `Service()` 暴露，防止组件绕过 owner EventHub 合同调用状态机。
- Run biz 已删除 Run→Orchestrator event adapter及不可达的 Start/Resume/Block/Cancel/Supersede/Fail lifecycle 方法，近千行对应 V1 fixture 改为精简的 Create/Ensure owner 持久化、幂等、scope 与 typed notification 测试；Run service 中无引用的生命周期 DTO alias和 test-only constructor 同步删除。
- Orchestrator biz 已物理删除 V1 `StartRun`、`dispatchAcceptedRun`、`DispatchNextStep`、`ResumeRun`、execution-context-patch resume 及其 running recovery helper；生产只保留 `AcceptRunStart`、`CanResumeRun` 和 V2 drive 使用的 blocker/resource/terminal 投影。
- 已删除 V1 resolution resume 自动 replan successor 子图与 `compressedContext.v1` 恢复摘要，不再由 resolved decision 启动旧 successor dispatch；Runtime V2 需要新目标时只接受新的 typed activation/bootstrap。
- 所有直接启动 V1 dispatch/resume/replan 主链的大型 integration fixtures 已删除；仍被窄算法测试复用的 authority/target-snapshot fixture 已收拢到独立 test support，不恢复任何生产兼容方法。
- 已删除 V1 runtime progress checkpoint、terminal-result checkpoint、`runtime_step_result_available` restart replay 与对应测试；Runtime V2 restart 只从 persisted aggregate/facts 恢复，不再把旧 Step output map 重新送入 finalize 主链。
- 已删除无调用方的 Orchestrator biz `CompleteRun/ValidateRun`、decision resume subsystem、approval patch resume 和 replan lineage ID/继承 helper；治理 resolution 不再触发旧 Step retry、patch apply 或 successor Run。
- 已清理仅服务 V1 replan 的 derived action contract strip、actionable replan continuation probe，以及无生产调用的旧 context Step ID 排序；保留 evidence 明确序号排序，避免影响历史 evidence/query 的确定性。
- Orchestrator plugin 与 application testkit 已停止构造 RuntimeAdapter registry 和 EventHub runtime adapter，`Dependencies.Runtime` 已删除；生产启动不再维护 Step kind registration matrix，Runtime V2 operation 只走组件 typed EventHub command/result。
- 已删除 Orchestrator module 对 RuntimeAdapter/registry 的公共 facade 和旧 registration matrix；Run/Workspace 状态通知不再隐式生成 V1 runtime checkpoint，历史 checkpoint repository 只服务 retirement、purge 和只读审计。
- 已删除无生产调用方的 V1 `NextActionForRun` 与整套直接测试 fixture；Runtime V2 next action 只由 persisted typed reducer 输出，不再从 Run execution-context map 动态推导。
- 已删除 `UpdateRuntimePolicyCommand` topic、DTO、EventHub contract、Orchestrator handler 与 HTTP 推送链；配置更新后 Runtime V2 从 canonical `workorchconfig` 派生当前策略，不维护 Orchestrator 进程内动态 policy 副本。
- 已删除 V1 completion gate、target-snapshot acquisition、repair dispatch/policy 与 governance localization 专属测试入口；仍被其他窄测试复用的 builder 仅保留在 test support，后续随对应生产闭包删除。
- 已物理删除 Orchestrator V1 dispatch、graph、repair、acceptance、convergence、coverage、patch、result assemble、LLM context、checkpoint/replay 主闭包及其专用测试；Orchestrator biz 生产主体已收窄为 Runtime V2 controller、owner projection、retirement resource release 和必要的终态资源清理。
- 已删除 Orchestrator `RuntimeAdapterRegistry`、EventRuntimeAdapter、Step kind registration matrix 及 application testkit registry fixture；Runtime V2 operation 不再经过集中 adapter facade。
- 已删除 Orchestrator 内部 V1 event subsystem；Run/Workspace 状态通知由各自 owner EventHub repository update 单一发布，避免双权威和重复投影。
- File、Git、Shell、SSH、MCP、Web、LLM 七个 block 已停止启动通用 `runtimeevent.Executor`，并删除 V1 Accepted/Progress/Completed/Failed 生产通知、worker 状态 map、heartbeat、发布失败指标和动态 execute topic contract；所有执行只保留组件直接订阅的 Runtime V2 typed EventHub command/result。
- `internal/pkg/runtimeevent` 已物理删除，HTTP 不再暴露恒为零的 V1 runtime event publish/heartbeat 指标，组件 README 已同步刷新为 Runtime V2 交互合同。
- application `runtimecore.ManagedRuntime` 已收窄为纯 Eventstream typed record collector，只保留 `PublishedEventRecords` 观测能力；已删除无生产调用方的 generic runtime adapter、依赖聚合、Workspace/Run/Decision/Approval action facade 和 snapshot read facade，以及整个 `runtimeports` 兼容层。
- application owner 相关测试 fixture 已改为测试专用 repository fixture或投递 owner 定义的具体 typed EventHub command；生产代码与测试均不再通过 runtime action/snapshot facade 绕过 owner 合同。
- Runtime V2 typed activation context 随 `RuntimeBootstrap -> RuntimeFacts -> RuntimeCandidateContext` 持久化和投递 dialog language、默认及 stage Soul 内容快照、Skill 指令、MCP server/capability 选择、Memory 内容和 evidence policy；LLM candidate provider 直接消费这些具体字段，不再从 `Run.ExecutionContext` 提取对应输入。
- evidence policy 已定义为具体 `RuntimeEvidencePolicy` / `RuntimeCoverageEvidenceRequirement`；Orchestrator 按 capability 随对应的 concrete operation command 投递，Shell block 直接从 `ExecuteShellOperationCommand` 的 typed data 执行 zero-work/work-marker 判定，不再通过 StepExecution 或 `input["evidencePolicies"]` map 解析。
- execution contract 第一阶段已定义为具体 `RuntimeExecutionContract` / `RuntimeCapabilityUsagePolicy`，保存 `stateChangePolicy` 与 capability usage guidance；LLM provider 将其放入 C2 capability contract，stable prefix hash 对 execution contract 变化敏感。
- profile acceptance contract 已定义为具体 `RuntimeProfileContract`、`RuntimeAcceptanceDimension`、`RuntimeDeliverableExpectation` 与 `RuntimeProfileEvidenceRequirement`；application 只在 activation 边界把 workspace/template 配置 map 转成 typed snapshot，随后经 `RuntimeBootstrap -> RuntimeFacts -> RuntimeCandidateContext` 持久化并进入 LLM C2 capability contract。stable prefix hash 覆盖 acceptance、deliverable、evidence role、accepted/rejected step kinds 与 accepted operations，不使用 JSON/reflect 做组件间类型桥。
- focus requirement 已扩展为 typed evidence role、accepted/rejected step kinds 和 accepted operations；reducer 只在 owner-confirmed capability kinds 中执行交集与排除，并把 accepted operations 封入 `RequiredOutputContract` 和 stable hash。candidate contract 在声明 accepted operations 时要求至少一个 operation 参数命中允许值，因此 `git/write add` 可作为依赖动作，但不能在缺少 `git/write commit` 时满足只接受 `commit` 的 focus。
- profile required acceptance dimension/deliverable expectation 已由 application activation 层按 `scope + id` 确定性生成 focus；primary state-change focus 优先，supplemental focus 按 canonical ID 稳定排序。无显式 executable evidence requirement 的 item 生成 assessment-only focus；声明 accepted step kind/operation 的 item 同步生成对应 owner capability prerequisite。
- template stage policy 已定义为具体 `RuntimeTemplateStagePolicy` 及 stage、entry、required output、completion、governance、policy refs 子类型；application 在 activation 边界把 `executionContract.templateStagePolicies` 配置 map 转成 typed snapshot，随后持久化并进入 LLM C2。stable prefix hash 覆盖完整 stage policy；core 不按 template ID、Soul ID、语言或业务名称解释其内容。
- definition change impact 已定义为具体 `RuntimeDefinitionChangeImpact`、`RuntimeDefinitionChangeSet` 与 `RuntimeDefinitionRevision`；application 在 activation 边界把 cumulative Goal/DoD impact map 转成 typed definition snapshot，随 bootstrap 持久化并进入 LLM C1。stable prefix hash 覆盖 effective/latest/previous/all identity、原位修订 lineage、pre-execution assessment 标记和 strategy requirements。
- `requiredPreExecutionAssessment=true` 已由 typed `need_assessment` authority、assessment candidate、assessment ledger、completeness gate 和 Result owner 投影闭环执行；不再依赖或回退 V1 assessment map 主链。
- Runtime V2 已新增 typed `need_assessment` authority、`AssessmentCandidate`、assessment ledger 和 current-run evidence ref gate；显式 `RequireAssessment` 的 focus 在 evidence 齐备后必须完成 assessment，assessment-only focus 可引用当前 Run 前序 accepted evidence，但不能引用历史/raw payload。
- 最终 assessment 关闭后，Orchestrator 使用 Result owner 的 typed EventHub create command写入 workspace/run scoped accepted/rejected DoD Result，再投影 Run/Workspace terminal；Result block 不参与 evidence quality 或 acceptance 裁决。
- Runtime V2 bootstrap 不再无条件要求 state-change：`must_produce_state_change` focus 要求 state-change + validation，`allowed_but_not_required` focus 初始只要求 validation，后续仅在 typed failure/evidence 证明需要变更时由 reducer 转入 state-change。
- `Run.ExecutionContext` 已从 owner model、command、repository、API、Web DTO、CLI/TUI 和 read-model 物理删除；activation 只构造 typed `RuntimeBootstrap`，governance history、rejected-result 与 continuation 已迁入具体 Runtime V2 owner contract，不保留 map 迁移源、JSON/reflect bridge、Envelope 或双栈兼容。
- LLM block 已删除 Runtime V1 `RuntimeAdapter`、execution decision/repair/capability DTO、execution-context prompt/budget/redaction 主链和长 prompt 死代码；provider 直接渲染 typed `PromptContext`，C0/C1/C2 为稳定前缀，C3+ 为动态消息，并在提交前执行具体 provider ceiling fail-fast。
- `workorchruntimecontract` 已删除无生产引用的 V1 `StagePacket`、`NextAction`、BlockExecutionGraph、RuntimeStep 通用事件结构及 `StepExecutionRequest/Result`；File/Git/Shell/MCP/SSH/Web 均直接使用各自 concrete operation request/result，map 不进入 EventHub command/result、facts、candidate、dispatch 或 evidence。
- Run/Orchestrator 已删除无生产发布方的 Step/LLM usage/contract-violation 通用 notification data contract、动态值包装和 Eventstream typed subscription；历史 EventRecord 仍可作为通用只读审计数据读取，但不再注册为当前生产消息合同。

删除范围至少包括：

- V1 runtime repair authority 推导；
- `stateChangingRepairRequired` 与 correction-specific authority 推导；
- context-gate retry Step 生成；
- requiredOutput mirror 自愈和 rebound；
- 多层嵌套 recovery Step ID；
- exhausted contract violation 转普通 runtime repair；
- LLM 输出驱动 convergence state 的兼容路径；
- V1 scheduler continuation、resume compatibility 和版本路由；
- 仅为 V1 服务的 tests、fixtures、DTO 和文档说明。

删除后保留：

- provider request rendering/budget/cache；
- deterministic protected floor；
- typed governance blocker；
- EventHub owner 边界；
- evidence、projection 和 resource release；
- 历史数据只读查询与审计能力。

V1 删除、full gates 和 clean build 完成后，按“全面切换维护窗口”执行正式 V1 retirement migration、启动 V2-only 服务并开放新 Run。不得先在生产数据执行迁移、随后继续运行 V1 binary。

LSP live gate：

- 真实 `go vet` failure 重新打开 state-change。
- target snapshot 自动准备。
- patch 后重新验证。
- 连续推进至少三个 focus。

网关 live gate：

- 缺 source snapshot 时先执行 prerequisite。
- 不再因 `file/list` 不在 state-change accepted kinds 而失败。
- continuable candidate choice 不创建伪外部 Decision。
- 连续完成 state-change 和 validation，并推进至少三个 focus。

### V2 Step ID 与 retry budget

V2 Step ID 只使用稳定序号：

```text
run-xxx/action-001
run-xxx/action-002
```

禁止：

```text
.../context-gate-retry/context-gate-retry
.../repair-001-of-contract-retry-...
```

每个 authority snapshot 的预算：

- LLM candidate attempt：最多 `2` 次。
- deterministic prerequisite：每个 prerequisite key 最多一次成功提交；失败后进入 governed wait 或 terminal。
- governance resolution：每个 resolution ID + authority snapshot ID 最多消费一次。
- Runtime V2 不提供自动 V1-compatible replan；需要重新定义目标时创建新的独立 V2 Run。

### 全面切换维护窗口

正式切换必须按以下顺序执行，禁止边运行 V1 边迁移：

1. 停止接收新的 Start/Resume/governance continuation。
2. 等待当前 owner command 退出有界 finalize 窗口。
3. 备份 `.workorch-data`、配置和 workspace projection 数据。
4. 执行 `runtime-v1-retirement --dry-run` 并核对对象数量。
5. 执行正式 V1 retirement migration。
6. 部署只包含 V2 主链的 clean build。
7. 执行 projection full rebuild。
8. 核对无 V1 active/blocked Run、无 V1 open blocker、无 stale `ActiveRunID`。
9. 恢复 Start/Resume 接口。
10. 创建全新 LSP、网关 V2 Run 执行 live gate。

任一步失败必须停止开放流量并恢复备份，不允许临时重新启用 V1 production dispatch 绕过问题。

### V2 验证门禁

Focused：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
```

Full：

```bash
go test -p 1 ./internal/modules/... -count=1
go test -p 1 ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

Final live gate：

- clean build：`vcs.revision=<target commit>`、`vcs.modified=false`。
- migration 后非终态 V1 Run、V1 open blocker 和 stale V1 `ActiveRunID` 均为 `0`。
- production dependency scan 不存在 V1 dispatch/recovery/version routing。
- 至少连续十个 V2 Run 中 `runtime_internal_contract_misalignment=0`。
- 每个 focus 最多一个普通 LLM dispatch 和一个 same-contract retry。
- 相同 failure signature 不产生第三个 LLM Step。
- 不存在嵌套 retry Step ID。
- 每次 transition 都能证明 evidence/focus/authority/governance/terminal 至少一项推进。
- provider 未提交时，同 scope request bytes 和 protected floor 不增长。
- resolved governance 对同一 resolution/authority snapshot 只消费一次。
- LSP 和网关均连续推进至少三个 focus。
- blocked/terminal Run 的 runtime、validation 和 prerequisite resources 全部释放。
- Run、Workspace、Result/outcome、projection 和 `ActiveRunID` 符合 owner 状态。

### 执行顺序与状态

| 顺序 | 收口项 | 当前状态 | 进入下一项条件 |
|---|---|---|---|
| 1 | RV2-01 唯一 typed canonical contract | complete：typed core 已成为生产唯一合同，V1 StagePacket/NextAction 已删除 | 类型、invariant 和依赖扫描通过 |
| 2 | RV2-02 单一 reducer | complete：生产 drive 只使用 persisted typed reducer | 两个 live facts fixture 确定性通过 |
| 3 | RV2-03 prerequisite engine | complete：snapshot/capability/validation/provider ceiling 已接入唯一主链 | snapshot/capability preflight 闭环 |
| 4 | RV2-04 LLM candidate boundary | complete：typed PromptContext 与 same-contract retry 已接入生产 | same-contract 最多一次 retry |
| 5 | RV2-05 sealed dispatch/evidence commit | complete：repository、dispatch 和 evidence commit 已接入唯一主链 | stale/duplicate/restart tests 通过 |
| 6 | RV2-06 一次性主链切换 | implementation complete：Start/Resume/governance/restart 生产入口只进入 V2 | 所有生产入口只进入 V2 |
| 7 | RV2-07 V1 retirement migration 工具 | complete：typed migration、数据副本 dry-run/apply、幂等、备份与恢复校验全部通过 | data copy dry-run、幂等和恢复测试通过 |
| 8 | RV2-08 删除 V1、正式迁移并双 workspace live | code/live complete：V1 主闭包及兼容层已删除，fresh file-backend 连续 10 个 V2 Run 与有界 provider failure live gate 已通过；等待目标提交 clean build 与真实生产目录维护窗口正式 apply | clean build、正式 migration 与切换后复核通过 |

执行期间不得跳过 RV2-01/RV2-02 直接修复某个 live failure；新的异常必须先归类为 facts、reducer、prerequisite、candidate validation 或 evidence commit 问题，再在对应唯一 owner 内处理。生产切换前可以分提交实现，但不得启用任何临时双栈 production routing。

### RV2-01/RV2-02 代码收口记录（2026-07-12）

本轮已在正式共享合同包 `internal/pkg/workorchruntimecontract` 直接建立 Runtime V2 typed canonical contract，没有创建 `/v1`、`/v2` 子包、facade 或运行期版本分支：

- 新增 `RuntimeFacts`、`RuntimeDecision`、`ConvergenceState`、`RuntimeAction`、`CoverageFocus`、`EvidenceLedger`、`FailureFact`、`PrerequisiteState`、`RequiredOutputContract`、`AuthoritySnapshot`、`AuthorityTransaction` 和 `TerminalOutcome`。
- authority action 收敛为 `AcquirePrerequisite`、`ProduceStateChange`、`ProduceValidation`、`RequestGovernance`、`Complete`、`Fail` 六类；state/action/requiredOutput kind 使用固定一一映射，不再依赖布尔字段组合表达 authority。
- `RequiredOutputContract` 只包含具体 typed 字段；normalization 对集合去空、去重并排序，stable hash 使用长度前缀的确定性二进制输入计算，不使用 JSON marshal/unmarshal。
- `SealRuntimeDecision` 原子生成 requiredOutput hash、authority snapshot ID、审计版本和 transaction；snapshot 中任一 state、action、focus、revision、attempt 或 requiredOutput hash 漂移都会被 invariant validation 拒绝。
- 新增 action/state 全组合、非法 requiredOutput、terminal、snapshot mutation、hash 顺序稳定和语义变化敏感测试。

本轮同时新增 `internal/modules/kernel/orchestrator/biz/runtimev2.ReduceRuntimeState` 纯 reducer：

- reducer 只依赖 typed `RuntimeFacts`，不访问 repository、EventHub、provider、profile 或全局配置。
- pending prerequisite、state-change gap、validation gap、validation failure、governance dependency、complete 和 runtime internal defect 均从同一入口生成 sealed `RuntimeDecision`。
- 相同 facts 生成相同 decision、snapshot 和 requiredOutput hash；只有同 focus、同 definition revision、同 evidence revision 的再次归约才增加 attempt，evidence 推进后 attempt 重置。
- LSP fixture `ws-5cfc9874cebda0b8/run-006` 将 `server.go:29:6: main redeclared in this block` 的 rejected validation evidence 原子归约为 `ProduceStateChange`。
- 网关 fixture `ws-609ce9149734ebb7/run-007-replan-001` 将 `coverage_assessment_incomplete` 和 `cwi-085` state-change gap 原子归约为 `ProduceStateChange`。

静态扫描确认本轮 V2 新代码中不存在 `map[string]any`、JSON bridge、reflect contract、`workorchruntimecontract/v1`、`workorchruntimecontract/v2` 或 runtime contract version switch。

本轮已通过：

```text
go test ./internal/pkg/workorchruntimecontract ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test -p 1 ./internal/modules/... -count=1
go test -p 1 ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

当前没有启用 V2 production routing。现有 V1 `NextAction`、`StagePacket`、convergence mirror 和 recovery 生产依赖必须在 RV2-06 一次性主链切换及 RV2-08 删除阶段退役；在此之前 RV2-01 状态只能标记为 typed core complete，不能宣称 production dependency retirement 已完成。

### RV2-03 prerequisite engine 收口记录（2026-07-12）

本轮已建立 Runtime V2 deterministic prerequisite engine 的 typed 核心：

- `RuntimeFacts` 新增 manifest、target snapshots、failure target、capabilities、validation scratch 和 provider ceiling 资源事实；snapshot 使用 `complete`、`missing`、`truncated`、`binary`、`redacted` 互斥状态，不用多个布尔字段组合表达安全性。
- `PreflightRuntimeFacts` 在 reducer 前统一检查六类 prerequisite，生成去重、稳定排序的 `AcquirePrerequisite` 输入；已满足 prerequisite 不会再次调度。
- `CommitPrerequisiteResult` 只接受当前 pending prerequisite 的具体 typed result；重复 result、未知 ID、不完整 snapshot、缺失 capability、无效 scratch 和不足 provider ceiling 均被拒绝。
- prerequisite result 提交后写回 typed facts，再次调用同一 reducer；网关 fixture 在缺少 `go.mod` 和 source snapshot 时先进入 deterministic preflight，两个完整 snapshot 提交后原子进入 `ProduceStateChange`。
- `file/list`、`file/read` 只属于 runtime-managed prerequisite acquisition，不会进入 state-change `RequiredOutputContract.AcceptedKinds`。

target snapshot 的跨组件路径已完成：

- command/result 由投递方 Orchestrator 在 `kernel/orchestrator/pkg/events` 定义为 `AcquireTargetSnapshotCommand` / `AcquireTargetSnapshotResult`。
- File block 导入投递方合同并直接订阅 magicCommon EventHub，返回具体 `TargetSnapshot`；交互不使用 `map[string]any`、Envelope、JSON bridge、reflect 或 service injection。
- Orchestrator 的 EventHub adapter 校验 authority snapshot ID、prerequisite ID 和完整 snapshot，并原子提交回 typed facts。
- File block 只接受完整、非 binary、非 truncated、非 redacted 且具备 content hash 的 snapshot；不安全结果不能进入 patch authority。

本轮后续已完成全部 prerequisite owner adapter：

- workspace manifest 由 File block 通过 typed EventHub command 获取；revision 对路径和内容 hash 变化敏感，超过有界 listing 时 fail fast。
- provider ceiling 由 LLM block 通过 typed EventHub command 返回，取当前 provider context profile 与 inherited ceiling 的较小值；未配置或不可用 provider 不会伪造 ceiling。
- runtime capability 由 File、Git、Shell、MCP、Web、SSH 和 LLM block 分别订阅 Orchestrator 定义的 typed command，并只返回自身实际支持的 kinds；Orchestrator 不再用本地注册矩阵代替 capability owner 在线应答。
- failure target 来自 Orchestrator 自有 typed failure facts，validation scratch 由 Orchestrator 在受控临时根目录创建；二者属于同一 owner 内部资源，不经过跨模块 service 或伪 EventHub 调用。

因此 RV2-03 的代码实现和 owner adapter 已完成，但当前仍不启用 V2 production routing；正式调用只允许在 RV2-06 一次性主链切换时接入。

实现 manifest revision 时同时修复了 File block `file/list` 的既有路径解包错误：旧实现把 workspace root 当成 entry resolved path，导致文件 `contentHash` 为空并使同路径同大小内容变化无法反映到 manifest revision；当前已改为读取受路径保护的实际 entry path，并增加内容变化回归。

本轮新增 prerequisite focused tests、typed EventHub integration test、message/delivery contract test，并再次通过串行 modules/full tests、fmt、vet、boundary contract 和 diff check。

### RV2-04 LLM candidate boundary 收口记录（2026-07-12）

本轮已建立独立于 V1 `LLMExecutionDecision` 的 Runtime V2 typed candidate 协议：

- 共享合同只允许 `StateChangeCandidate`、`ValidationCandidate` 和 `GovernanceCandidate` 三类互斥 candidate。
- state-change/validation operation 使用具体 `RuntimeOperation`、coverage IDs、target refs 和 typed parameter union；不使用 `map[string]any` 传递组件合同。
- candidate 必须与 sealed `RuntimeDecision.Action` 一致，operation kind 必须属于当前 `RequiredOutputContract.AcceptedKinds`，并绑定当前 coverage focus；重复 operation ID、未知 dependency、重复 parameter 或多值 parameter 都会被拒绝。
- `Complete`、`Fail` 和 `AcquirePrerequisite` action 不能进入 LLM candidate boundary。

LLM adapter 边界已实现严格 JSON 解码：

- provider JSON 只在 `blocks/llm/biz/runtimev2` adapter 边界解码，并启用 unknown-field rejection。
- provider 返回 `convergenceState`、`nextAction`、`focus`、`authority`、`repairObjective` 或任何非 candidate schema 字段时直接判定 schema failure，不能影响 Orchestrator authority。
- LLM block 收到的 typed provider request 只包含 sealed decision/transaction、Goal/DoD definition context、capability contract、candidate attempt 和 stable prefix hash；不再接收另一套可修改 authority。
- LLM block 将 candidate 自动绑定请求 transaction，provider 无权自行声明 snapshot ID。

same-contract retry 已收敛为有限状态：

- candidate attempt 只允许 `0` 或 `1`。
- attempt 0 的 schema/provider/contract failure 只能生成一次 `retry_same_contract`。
- retry 必须保持完全相同的 authority transaction、authority snapshot ID、requiredOutput hash、action 和 stable prefix hash，只允许 candidate attempt 从 0 变为 1。
- attempt 1 再次失败进入 `request_governance`；内部 transaction mismatch 直接 terminal，不允许第三个 candidate request。
- provider、schema 或 candidate contract failure 不会隐式切换 action。

跨组件路径使用 Orchestrator 定义的 `RequestRuntimeCandidateCommand` / `RequestRuntimeCandidateResult`，LLM block 导入投递方合同并直接订阅 EventHub。当前 RV2-04 实现不会启用 production routing；V1 宽输出和 convergence parsing 只在 RV2-06 一次性切换、RV2-08 删除阶段退出生产依赖。

### RV2-05 sealed dispatch/evidence commit 收口记录（2026-07-12）

本轮已实现 Runtime V2 可持久化 dispatch aggregate 和 evidence commit coordinator：

- `SealDispatch` 在执行前校验 candidate 与当前 decision，封存完整 `AuthorityTransaction`、candidate hash、workspace/run scope 和 candidate 本体。
- `DispatchStepInput` 直接包含 dispatch ID、完整 transaction、candidate hash 和 typed candidate，后续 Step persistence 不需要从 stage packet、mirror 或 map 重建 authority。
- 相同 snapshot/candidate 重复 seal 返回同一 dispatch；同 snapshot 出现不同 active dispatch 时 fail fast。
- candidate hash 使用 deterministic typed hashing，不通过 JSON marshal/unmarshal。

evidence commit 固定执行：

```text
Load persisted aggregate
  -> verify event/scope/snapshot/dispatch/operation
  -> append accepted or rejected typed evidence fact
  -> increment evidence sequence/revision
  -> mark dispatch committed
  -> ReduceRuntimeState
  -> CAS save aggregate
```

约束已经转化为代码和测试：

- execution evidence 必须携带 event ID、workspace/run、dispatch ID、authority snapshot ID、operation ID、evidence kind 和 accepted/rejected status。
- snapshot 与当前 authority 不一致时只追加 `DiagnosticEvidence`，不修改 focus、facts evidence、evidence revision 或 decision。
- 相同 event ID 由 persisted `ProcessedEventIDs` 幂等拦截，duplicate 不增加 revision、不重复 evidence、不再次 reducer。
- snapshot 相同但 dispatch 或 operation 不属于当前 sealed candidate 时按 internal mismatch 拒绝。
- accepted evidence 原子更新 ledger 并进入下一 reducer；rejected validation evidence 携带 typed failure fact，可直接从 `ProduceValidation` 原子回到 `ProduceStateChange`。
- aggregate repository 使用 load + expected revision CAS save；coordinator 不保存进程内 dispatch/evidence map。
- restart fixture 在第一个 coordinator 持久化 dispatch 后销毁实例，第二个 coordinator 仅从 repository aggregate 恢复并成功提交 evidence。

当前实现提供 `RuntimeStateRepository` owner port，正式 repository adapter 和生产 dispatch/commit 入口只在 RV2-06 一次性主链切换时装配；在此之前不启用并行 V2 production routing。

### RV2-06 正式持久化接入记录（2026-07-12）

已完成一次性主链切换的第一项生产依赖：

- 在共享 runtime contract 中新增 typed `RuntimeStateRecord`、`RuntimeDispatchRecord` 和 `RuntimeDiagnosticRecord`，完整保存 facts、decision、sealed dispatch、processed event 与 diagnostic lineage；未使用 JSON bridge、reflect、Envelope 或跨组件 `map[string]any`。
- Run owner model 新增 `runtimeContractVersion` 与 typed runtime state；file/memory repository clone 路径同步执行深复制，避免进程内引用共享破坏持久化快照。
- Run update command 新增 expected runtime state revision。Run owner 在 EventHub command lane 内校验 canonical invariant、workspace/run scope 和 expected revision；stale update 返回 conflict。
- Orchestrator 新增正式 `RuntimeStateRepository` EventHub adapter，并在 module Configure 时装配 `DispatchCoordinator`。Load/Save 均通过 Run owner command，不直接访问 Run repository/service。
- repository adapter 已覆盖首次保存、重启读取与 stale CAS rejection；focused orchestrator/run/repository/runtime-contract tests 通过。

当前尚未切换 Start、Resume、Step completion、governance resolution 和 restart recovery 的生产入口，因此 RV2-06 保持 `in progress`。下一步必须在同一主链 coordinator 中完成 typed facts 初始化、prerequisite、candidate、execute 和 evidence commit，再一次性关闭 V1 dispatch 与 scheduler continuation；不得把当前 repository 装配解释为并行 V2 production routing。

后续已继续完成统一 controller 与主链前半段：

- 新增 `RuntimeController`，统一暴露 initialize、load、prerequisite commit、candidate seal 和 evidence commit；`RuntimeOrchestrator` 不再分别持有可绕过主链的 repository/coordinator 操作入口。
- initialize 固定执行 `Normalize Facts -> Preflight -> Reduce -> CAS Save`。相同 Run 的 Start/Resume/restart 只幂等读取既有 aggregate，不重复 reducer；definition revision 或 focus identity 漂移直接返回 conflict。
- prerequisite result 通过 controller 加载当前 aggregate、校验当前 action、提交 typed result、再次 reducer，并使用 expected revision 原子保存。
- reducer attempt 判定已改为比较最终 state、action、requiredOutput hash、focus 和 revision；manifest/snapshot/capability 等事实推进后 attempt 重置，不再被误记为同合同 LLM retry。
- `RuntimeRequirement` 已提升到共享 typed contract，并携带 prerequisite owner module；capability acquisition 可按 File、Shell 等实际 owner 投递 EventHub command。
- 修复多个 capability prerequisite 顺序提交时后一个结果覆盖前一个结果的问题；state-change、validation 和 governance capability 现在按具体类型合并。
- module mainline 已接通 deterministic prerequisite acquisition 和 typed candidate request/seal：workspace manifest、target snapshot、failure target、validation scratch、provider ceiling 和 capability 均在 LLM 前完成；candidate 必须绑定当前 authority transaction 后才能持久化 active dispatch。
- integration tests 已覆盖 `Initialize -> local prerequisite -> Reduce -> persisted reload` 以及 `Initialize -> typed LLM candidate -> sealed dispatch -> persisted reload`。

当前主链已完成：

```text
Initialize/Load
  -> PreparePrerequisites
  -> Reduce
  -> RequestCandidate
  -> Seal/PersistDispatch
```

上一阶段尚未接入 typed operation executor 和 runtime block evidence 转换，因此当时未注册 V2 production routing。

后续已完成 typed operation executor 与 evidence commit 闭环：

- 修复 RV2-05 多 operation dispatch 的提前完成缺陷。当前每个 operation 独立记录 `CompletedOperationIDs`；全部 operation 成功，或任一 operation rejected 后，dispatch 才转为 committed 并再次 reducer。
- `RuntimeController.ExecuteActiveDispatch` 对 candidate operation dependency graph 执行确定性拓扑排序；同层 operation 按 ID 排序，未知 dependency 和 cycle fail fast。
- operation executor 输入包含 workspace/run、dispatch、完整 authority transaction、具体 `RuntimeOperation` 和 evidence kind；结果使用具体 event ID、operation ID、accepted/rejected status、source ref 和 typed failure facts。
- Orchestrator 新增 `ExecuteRuntimeOperationCommand/Result`，并按 operation kind 将 EventHub command 投递给 File、Git、Shell、MCP、Web 或 SSH block。所有 command/data/result 均为具体类型，不包含跨组件 `map[string]any`。
- 六个 capability block 已直接订阅 typed runtime operation command。既有 block adapter 所需的 map 只在消费组件内部由 typed parameter union 转换，不进入 EventHub、candidate、dispatch、facts 或 evidence。
- block 执行失败转换为 typed `execution_failure`，validation rejection 可由 reducer 原子回到 state-change；transport/internal error 保留 active dispatch，供重启后从 persisted `CompletedOperationIDs` 继续。
- 新增多 operation 全成功、首 operation rejected、typed command 本地适配、EventHub owner routing，以及 `Candidate -> Seal -> Execute -> Evidence -> Reduce -> persisted reload` 集成测试。

当前主链已推进为：

```text
Initialize/Load
  -> PreparePrerequisites
  -> Reduce
  -> RequestCandidate
  -> Seal/PersistDispatch
  -> ExecuteTypedOperations
  -> CommitEvidence
  -> Reduce
```

RV2-06 剩余关键工作为：same-contract candidate retry/governance terminal 收口、typed bootstrap/definition 输入持久化，以及 Start/Resume/Step completion/governance resolution/restart recovery 的唯一入口切换与 V1 scheduler continuation 关闭。

后续已完成 same-contract candidate retry 与失败收口：

- `RuntimeStateRecord` 新增 typed `RuntimeCandidateAttemptRecord`，持久化完整 transaction、attempt、stable prefix、pending/failed/accepted 状态和 typed failure。
- attempt 0 的 pending replay 幂等，不重复保存或生成新请求；attempt 1 只有在 attempt 0 已 failed、transaction 与 stable prefix 完全一致时才可开始。
- LLM block 将 provider、schema、candidate contract 和 internal failure 作为具体 `RuntimeCandidateFailure` 返回，不再把可治理失败压成普通 EventHub error。
- candidate accepted 与 sealed dispatch 使用同一次 CAS 原子保存；accepted attempt 必须能关联 active/completed dispatch，不能留下无 dispatch 的成功记录。
- attempt 0 非 internal 失败保持原 authority 不变并允许一次 same-contract retry；internal failure 立即生成 `runtime_contract_defect` terminal。
- attempt 1 再次失败时，有 governance capability 则原子进入 `RequestGovernance`；没有 governance capability 则生成 `candidate_retry_exhausted` terminal。
- invariant validation 拒绝第三次请求、attempt 1 缺 failed attempt 0、stable prefix 漂移、accepted 后重试、重复 attempt 和无 sealed dispatch 的 accepted 状态。
- integration tests 已覆盖 provider typed failure、attempt 0 schema failure 后 attempt 1 成功、连续两次 provider failure 后 terminal，以及 pending attempt 的重复调用幂等。

当前 Runtime V2 LLM 请求预算已经固定为每个 authority snapshot 最多两次，不再依赖 V1 nested retry Step、repair lineage 或进程内计数器。

后续已完成 typed bootstrap/definition 输入持久化：

- 共享 runtime contract 新增 `RuntimeDefinitionContext`，具体保存 definition revision、Goal/DoD identity、完整定义正文和 capability contract；normalization、validation、clone 与 equality 均使用 typed 字段，不使用 JSON、reflect 或 map bridge。
- `RuntimeStateRecord` 与 `RuntimeAggregate` 直接持久化 definition context，并强制其 revision 与 `RuntimeFacts.DefinitionRevision` 一致。
- `RuntimeController.Initialize` 在首次初始化时封存 definition；重复 Start/Resume/restart 只有完整 definition context 相同才幂等返回，identity、正文、revision 或 capability contract 漂移均返回 conflict。
- candidate mainline 不再接收调用方临时传入的 definition DTO；Orchestrator 从 persisted aggregate 读取定义，并通过自身定义的 typed EventHub command 投递给 LLM block。
- 删除 Orchestrator events 与 LLM model 中重复的 definition context 类型，双方直接使用共享 canonical type，避免 DTO 转换产生定义漂移。
- repository、controller、candidate request 和 definition drift 测试已覆盖重启读取、Run owner 持久化、revision mismatch 与正文漂移拒绝。

后续已完成 governance resolution typed facts/evidence 核心闭环：

- 新增 typed `RuntimeGovernanceResolution`、continue/fail outcome 和持久化 consumption record；合同包含 workspace/run、resolution、authority snapshot、dispatch、dependency、summary 和 evidence refs，不使用 map、JSON bridge 或 reflect。
- governance candidate 继续作为 sealed dispatch 保存；resolution 只能提交到当前 `RequestGovernance` authority 和匹配的 governed dispatch，dependency kind/subject 漂移返回 conflict。
- continue outcome 写入 accepted governance evidence、递增 evidence revision、清除 governance failure，并通过同一 reducer 回到原 state-change/validation evidence gap。
- fail outcome 写入 rejected governance evidence，并由 reducer 生成 `governance_rejected` typed terminal；不再进入 LLM repair。
- stale resolution 只追加 diagnostic，不改变 authority、dispatch 或 evidence revision；同一 resolution ID + authority snapshot 的重复提交在重启后仍幂等。
- Orchestrator 新增 `CommitRuntimeGovernanceResolutionCommand/Result` typed EventHub owner command，返回 disposition、state/evidence revision、next action 与 terminal；外部 continuation 不再需要直接修改 Run 或 runtime facts。
- invariant validation 要求每个已消费 governance resolution 都关联 completed governed dispatch；Run owner CAS persistence、restart duplicate 和 EventHub integration tests 已覆盖。

后续已完成 typed governance ticket/request binding：

- Orchestrator 新增 `OpenRuntimeGovernanceRequestCommand/Result`，command 具体携带 workspace/run、ticket、dispatch、完整 authority transaction、external dependency 和允许的 resolution type；Decision block 导入投递方合同并直接订阅 EventHub。
- Decision owner 将 typed command 转换为自身 `EnsureTicket` 操作，不通过跨组件 `map[string]any` 传递 runtime snapshot、dispatch 或 dependency。
- ticket ID 从 sealed dispatch 确定性生成；重复请求由 Decision `EnsureTicket` 与 Orchestrator binding CAS 双重保证幂等。
- `RuntimeStateRecord` 新增 governance request binding，具体保存 ticket ID、authority snapshot ID、dispatch ID、dependency kind 和 subject ref；restart 后不需要 scheduler 扫描或猜测 ticket 对应 authority。
- resolution 必须携带并匹配已绑定 ticket；缺少 binding、同一 dispatch 绑定不同 ticket、dependency 漂移或 ticket 漂移均返回 conflict。
- invariant validation 要求每个 request binding 关联 active/completed governed dispatch，每个 consumed resolution 同时关联 request binding 和 completed governed dispatch。
- integration test 已覆盖 typed LLM governance candidate、Decision ticket 创建、Run owner binding 持久化、restart replay、typed resolution commit 和 reducer continuation。

RV2-06 剩余工作收敛为生产 Start/Resume/Step completion/governance resolution/restart recovery 唯一入口切换，以及 V1 scheduler continuation 关闭。

生产入口切换审计随后发现并补齐多 focus 推进缺口：

- 原 Runtime V2 facts 只有单个 `Focus`，accepted evidence 也没有 coverage binding；首个 focus 满足后 reducer 会直接 Complete，无法满足连续推进多个 coverage focus 的 live gate，因此不得直接切换生产 Start。
- `RuntimeFacts` 现新增有序 remaining focus queue；顺序属于 bootstrap contract，不在 reducer 内重新排序。
- `EvidenceFact` 新增具体 coverage work item IDs，并从 sealed operation 原子复制；LLM 或 block 不能在 execution result 中重新声明 coverage。
- missing evidence 与 rejected validation 只读取绑定当前 focus 的 evidence；前序 focus evidence 不得满足后续 focus。
- dispatch 完成后，若当前 focus 已满足且仍有 remaining focus，controller 原子切换 facts current focus、缩短 queue，再调用同一 reducer；只有最后一个 focus 满足后才生成 Complete。
- regression test 已覆盖 focus-1 state-change evidence 提交后进入 focus-2 `ProduceStateChange`，而不是错误 terminal，并验证 evidence 保持 `cwi-1` 绑定。

RV2-06 下一步为在 Run 创建时持久化完整 typed bootstrap（definition、focus queue、requirements），随后建立唯一 V2 drive command，才能安全切换生产 Start/Resume/Recovery。

后续已完成 Run 创建阶段 typed bootstrap 持久化：

- 共享 runtime contract 新增 `RuntimeBootstrap`，具体包含 `RuntimeDefinitionContext`、ordered coverage focus queue 和 typed prerequisite requirements；提供 normalization、validation、clone、equality 与 `InitialFacts`。
- Workorch activation profile 使用 cumulative effective Goal/DoD 构造 definition context，避免只保存最新单条定义导致 C1 正文与 definition revision 漂移。
- 每个有效 DoD item 按声明顺序生成一个 focus/cwi，并要求 state-change 与 validation evidence；该 profile 映射位于 application activation 层，不进入 kernel reducer。
- activation profile 当前声明 File state-change、Shell validation、Decision governance，以及 manifest、validation scratch、provider ceiling prerequisite；跨组件仍由各 owner 在线确认 capability。
- Run create/update command、Run model、file/memory repository clone 与 EventHub adapter 均保存 typed bootstrap；bootstrap definition identity/revision 与 Run GoalID、DoDID、DefinitionRevision 不一致时返回 conflict。
- Decision block 已订阅 typed runtime capability prerequisite并声明 `decision/request`，避免 governance bootstrap 在 owner preflight 阶段缺失 capability responder。
- Orchestrator 新增 `InitializeRuntimeV2FromRun`，只从 Run owner bootstrap 生成 initial facts 并调用统一 controller，不读取 V1 execution context map。
- tests 已覆盖三 DoD focus 顺序、bootstrap clone/drift、initial facts、Run owner 持久化及从 persisted Run 初始化 aggregate。

后续已完成正式 candidate context 与 provider 接线：

- `RequestRuntimeCandidateCommand` 新增 typed `RuntimeCandidateContext`，由 persisted RuntimeFacts 确定性构造 workspace/run scope、definition/evidence revision、current focus、accepted/rejected evidence、failure、capabilities 与 prerequisite resources。
- Orchestrator 和 LLM block 同时校验 candidate context、decision 与 sealed authority transaction 一致；LLM 不再依赖 V1 execution context map 猜测当前事实。
- LLM Module 默认使用自身 `GenerateCandidate` 作为正式 `CandidateProvider`，通过现有 provider adapter 请求 typed operation/governance candidate；JSON marshal 只存在于 provider adapter 边界。
- provider-facing prompt 固定按 C0 Runtime V2 candidate contract、C1 Goal/DoD、C2 capability contract、C3 current authority/context 组织，并使用 definition 派生的 canonical stable prefix hash。

后续已完成唯一 Runtime V2 drive owner command/mainline loop：

- Orchestrator 新增 `DriveRuntimeV2Command/Result` 与 `/orchestrator/runtime-v2/drive` typed EventHub topic，不复用 legacy `OrchestratorCommandResult`。
- drive 从 Run owner 读取 typed bootstrap/state，并按唯一有界循环执行 `Initialize/Load -> prerequisite -> candidate -> seal -> typed operation -> evidence -> reducer`。
- executable dispatch 从 persisted `CompletedOperationIDs` 继续；governance dispatch 确定性绑定 Decision ticket 后返回 `governed_wait`；Complete/Fail 返回 typed terminal。
- 每个继续循环的 transition 必须增加 persisted state revision；无进展或超过 1024 次 transition 立即 fail-fast，禁止单次调用无界空转。
- 重复 drive 和新 Orchestrator 实例重启均返回同一 persisted terminal，不重复请求 candidate 或执行 operation；governed wait 重放返回同一 ticket 与 revision。
- integration tests 已覆盖完整 typed EventHub 路径、terminal replay、restart replay 和 persisted governance wait。

RV2-06 下一步收敛为生产 Start/Resume/governance resolution/restart recovery 的一次性入口切换，并在同一提交关闭 V1 dispatch 与 application scheduler continuation；不得增加 runtime version switch 或 V1/V2 双栈。

生产入口切换预检已补齐 Run/Workspace 投影能力：

- 新增 `ActivateRuntimeV2Run`，只执行 blocker→active owner 投影并清除 blocker，不创建 V1 Step。
- 新增 `ProjectRuntimeV2Terminal`，使 typed reducer Complete/Fail 可直接更新 Run/Workspace owner；Complete 不经过 V1 Result/DoD completion gate，重复 terminal 投影幂等。
- tests 已覆盖 blocker→active 不创建 legacy Step、V2 Complete 绕过 legacy gate和 terminal replay。

尝试直接切换 Module Start/Resume/recovery 后，全量 application server 测试证明大量启动、恢复、approval/replan 和 watchdog fixture 仍创建无 typed bootstrap 的 V1 Run。V2 正确地以 invalid argument 拒绝这些 Run；如果按 `RuntimeBootstrap == nil` 回退 V1，会形成本文禁止的运行期双栈。因此入口切换未保留该中间态，当前顺序调整为：先迁移或删除 application server 的 V1 Run fixtures、governance/replan continuation 与 watchdog recovery，再在同一原子变更中关闭 `EnableEventDrivenDispatch` 并切换 Start/Resume/governance/restart。不得为保持旧测试增加 compatibility switch。

后续已完成 Runtime V2 governance production continuation 闭环：

- Orchestrator 新增 `CommitRuntimeGovernanceTicketResolutionCommand` typed EventHub 合同，Decision 只提交 resolution/ticket/workspace/run/outcome/summary，不需要访问 Runtime V2 state。
- Orchestrator 从 Run owner persisted aggregate 查找 ticket binding，补齐 authority snapshot、dispatch、dependency kind 与 subject ref，再复用唯一 governance resolution commit。
- Decision owner 在 `runtime_v2_governance` ticket `EnsureResolved` 成功后直接调用该 command；continue/fail outcome 由 resolution type 确定，不使用 application map bridge。
- committed resolution 由 Orchestrator 自行投影 terminal 或调度 Runtime V2 drive continuation；重复 resolution 继续由 persisted consumption record 幂等拦截。
- Decision resolved notification 新增具体 `DecisionKind`；application HTTP continuation helper 和 resolved-event watcher对 `runtime_v2_governance` 跳过，防止同一 resolution 再进入 V1 Resume scheduler。
- integration test 已覆盖 `Decision EnsureResolved -> typed ticket resolution -> Orchestrator binding lookup -> governance evidence -> reducer -> V2 continuation scheduled`。

因此 application governance scheduler 已不再负责 Runtime V2 Decision continuation。下一步继续删除 V1 approval/replan、stale Step watchdog 与 run continuation 队列，并迁移其 fixture。

后续已解除 application server 生命周期中的 V1 自动恢复接线：

- `Service.Run` 不再调用 `recoverStartupInactiveRunningWorkspaces`；服务重启不会把历史 active Run 自动取消、释放 activeRunId 或生成 restart diagnostic。
- `Service.Run` 不再调用 `scheduleWatchdog`；生产启动不再注册周期 stale-dispatched Step recovery timer。
- workspace projection 仍按 owner snapshot 启动，但不承担修复 owner 状态或决定 continuation 的职责。
- 删除“启动恢复旧 Run 后再触发 workspace activation”的 V1 fixture；新的回归测试要求 legacy active Run 在迁移前保持原样，且启动过程不提交 recovery background function、watchdog timer 或 scheduler diagnostic。
- 后续收口已物理删除 `http_run_continuation_scheduler.go` 及其显式 V1 recovery、run/workspace continuation queue 和局部 fixtures，不得重新接回 `Service.Run`。

后续已完成 application governance continuation 生产链删除：

- Decision/Approval HTTP resolve handler 只写入对应 owner 并刷新 projection，不再发布 `WorkspaceRunRequestedCommand`。
- 删除 Eventstream 上 Decision/Approval/Governance resolved watcher；application 不再根据 resolved notification 推断 Resume 或 replan。
- 删除 governance pending/started/attempted queue、resolved blocked Run recovery scan、governance continuation diagnostic kind 和全部对应 V1 fixtures。
- `WorkspaceRunRequestedCommand` 不再接受 `replan` intent；Runtime V2 需要改变目标时创建新的独立 V2 Run。
- Runtime V2 Decision continuation 继续由 Decision owner 直接发送 Orchestrator 的 typed ticket-resolution command，不经过 application scheduler。

后续已删除 application V1 replan continuation：

- 不再扫描 superseded Run 或 resolved replan Decision 来自动创建 successor Run。
- workspace activation、Start 和 Resume 完成后不再派生 `replan_continuation` 后台任务。
- 删除 replan started dedupe、successor readback、recovery scan、diagnostic kind/reason 和对应 fixtures。
- HTTP/scheduler 已拒绝 `replan` intent；重新定义目标必须创建新的独立 Runtime V2 Run。

后续已完成 Start/Resume Runtime V2 生产入口切换与 application run continuation queue 删除：

- Orchestrator module 不再调用 `EnableEventDrivenDispatch`；V1 Step/runtime notification 不再拥有生产 dispatch 权。
- Start command 执行 `AcceptRunStart -> schedule Runtime V2 drive`，Resume command执行 `CanResumeRun -> ActivateRuntimeV2Run -> schedule Runtime V2 drive`。
- drive 只调用唯一 typed mainline；失败投影 `runtime_v2_drive_failed` typed terminal，不回退 V1 service dispatch。
- application event、HTTP/client、backend port、CLI/TUI 和 Web surface 已物理删除 `executionContextPatch`；scheduler 不再保留“接收后拒绝”的兼容字段，Runtime V2 activation/bootstrap 不从任意 map patch 合并 operator input 或恢复 authority。
- workorchd run-scoped scheduler 分支直接发送 Orchestrator 的具体 Start/Resume command；删除 `runPending`、`runStarted`、readback retry 和 background queue fixtures。
- 删除 appservice 中依赖无 typed bootstrap、V1 provider blocker、approval/replan Resume、orphan Step redispatch 和 V1 Step evidence 的集成 fixtures；activationcore、typed bootstrap、owner command 与 read-model纯投影测试继续保留。

后续已物理删除 application stale Step/watchdog recovery helper：

- 删除 watchdog timer 注册、周期 tick、stale dispatched recovery、stuck Run recovery 与 startup inactive Run recovery。
- 删除进程内 recovery mutex/running 标记、timer task wrapper 和仅验证自动恢复的 fixtures。
- 服务启动与 `Validate` 均不会注册 recovery timer；历史 active Run 只保留审计，不由 application 修改。
- watchdog interval metric 保留为退役审计字段并固定为 `0`，不得作为重新接线依据。

后续已完成 workspace activation/restart queue 删除：

- workorchd scheduler command 不再写入 `restartPending/restartStarted`，不提交 application background function。
- activation/restart 在 workspace lane 内同步执行 activation 用例，并由用例直接发送 Workspace、Run、Orchestrator 的 typed command。
- 新 Run 创建时必须携带 typed Runtime V2 bootstrap；Orchestrator Start 只调度唯一 Runtime V2 drive。
- 删除 owner readback、NextAction compatibility 判定、pending retry、enqueue failure compensation 和对应 fixtures。
- `http_run_continuation_scheduler.go` 已整体删除；application 不再拥有 execution scheduler 状态。

后续已完成 Orchestrator V1 event-driven Step dispatch、runtime notification continuation 与 runtime-gap recovery 删除：

- 删除 `DispatchStepRequested` topic、command、contract、订阅处理器、publish readback、idempotency helper 和 `eventDrivenDispatch` 运行期开关；graph/repair 遗留内部调用固定为直接执行语义，不再经过第二条 EventHub Step owner 链。
- 删除 runtime block Progress/Completed/Failed 订阅、旧 runtime result handler 与 terminal notification continuation；Runtime V2 operation 只接受同步 typed `ExecuteRuntimeOperationCommand/Result` 回执，并由 Orchestrator 提交 evidence。
- 删除 `RecoverRuntimeGap` owner command、module handler、application sender，以及 orphan Step、stale dispatched Run、startup inactive Run、stuck Run 等 application recovery API/queue/watchdog。
- 删除只验证上述 V1 command、notification、readback 和 event-driven graph dispatch 的测试，不恢复 no-op facade 或双栈 helper；保留的 Runtime V2、typed notification 和直接执行测试均通过。
- Start/Resume、Decision resolution 与 workspace activation/restart 已只进入 Runtime V2 bootstrap/aggregate/drive；application 不再拥有 continuation scheduler。

RV2-06 的生产入口和 V1 dispatch/scheduler compatibility 删除已完成。下一步进入 RV2-07：实现一次性 V1 retirement migration、历史资源释放和 projection rebuild dry-run；不得重新引入 runtime-gap 自动恢复或 V1/V2 路由开关。

### RV2-07 retirement planner 收口记录（2026-07-12）

已先建立 migration 的确定性 typed planner，作为 dry-run 与正式 apply 共用的唯一动作来源：

- `retirementcore.BuildRuntimeV1RetirementPlan` 只把没有 typed Runtime V2 bootstrap/state、且未声明 V2 contract 的 Run 识别为 V1；带 V2 bootstrap 但尚未初始化 aggregate 的 pending Run 不得被误退役。
- planner 对 V1 Run、stale Workspace `ActiveRunID`、open DecisionTicket 和 pending Approval 生成稳定排序的具体动作，并固定终态原因 `runtime_contract_v1_retired`。
- report 已包含 scanned V1 Run、待 supersede Run、待清理 ActiveRunID、待 supersede ticket/approval、待释放 resource scope 和无法迁移对象原因。
- 同一 inventory 重复规划结果完全一致；模拟 apply 后再次规划时不再产生 supersede/clear 动作，锁定 migration 幂等基础语义。
- planner 是 application focused package，不访问 owner repository，也不发布事件；下一步由 application migration coordinator 通过各 owner EventHub command 执行动作，并在成功后调用 projection full rebuild。
- `retirementports.LoadRuntimeV1Inventory` 已通过 Run、Workspace、Decision、Approval owner 的具体 typed list command 采集 planner 输入；缺失 owner response 或返回错误类型时 fail-fast，不允许回退 repository export。
- `retirementports.ApplyRuntimeV1RetirementPlan` 已按 Decision、Approval、Run、Workspace 顺序发送具体 typed owner update：先关闭 continuation blocker，再 supersede Run，最后清理 Workspace pointer。
- Run update contract 已补齐 lineage 与 created/started/ended 时间字段，migration 不会因 owner update 丢失历史身份；Run 写入固定 `runtime_contract_v1_retired` reason 并清空 blocker refs。
- Run、Workspace、Decision、Approval owner 对 migration update 发布正式 typed 状态通知；Decision/Approval supersede 不复用 resolved/approved/rejected 语义。
- 真实 memory owner fixture 已验证 apply 后不存在非终态 V1 Run、open ticket、pending approval 或 stale `ActiveRunID`，再次 inventory/plan 不产生任何动作。
- application 已定义具体 `RuntimeV1RetirementCommand/Result`：dry-run 返回 typed report，不执行 owner mutation 或 projection rebuild；apply 在成功后返回 projection full rebuild revision。
- resource report 已区分 `ResourceScopesToRelease` 与 `ReleasedResourceScopes`；计划 scope 数量不再冒充真实释放数量。
- Orchestrator 已定义 `ReleaseRuntimeV1ResourcesCommand/Result`，按 workspace/run scope 关闭 pending/executing Step、ToolCall、ArtifactContinuation，并释放 managed validation resource；命令拒绝 Runtime V2 Run，重复调用返回零变化。
- application 在任何 owner supersede/clear 之前逐 Run 同步发送 resource release command并累计具体回执；任一 release 失败时不执行后续 owner apply 和 projection rebuild。
- coordinator 测试已覆盖完整 `inventory -> release -> owner apply -> re-inventory -> zero-action audit -> projection rebuild revision` 顺序。
- 已提供本地 embedded `runtime-v1-retirement` CLI；无参数和 `--dry-run` 均执行只读盘点，只有显式 `--apply` 才执行正式迁移，两个 flag 同时出现时拒绝执行。
- CLI 通过窄 `RuntimeV1RetirementBackend` 发送 application typed command；remote backend 不实现该维护 port，禁止通过在线 HTTP 服务误执行维护迁移。
- projection owner 已提供同步 full rebuild 入口并返回 global revision；migration 只有在 apply 后重新 inventory/plan 确认零遗留动作时才允许调用。
- maintenance process 已在 framework/module/EventHub 启动前建立强制备份与恢复入口：`--apply` 仅允许 file repository，先备份 repository data 和 config directory，备份成功后才启动 owner migration；dry-run 不创建备份。
- `--backup-dir` 只允许与 `--apply` 组合；`--restore-from` 与 apply/dry-run/backup-dir 互斥，并在 framework 与 owner repository 打开前完成恢复后直接退出。
- backup manifest 保存绝对 source、snapshot 与 backup directory；创建时拒绝 data/config 重叠、backup 位于任一 source 内部、缺失 config snapshot 或覆盖现有 destination。
- restore 在替换任何目标前完整校验 data/config 两份 snapshot，并通过同文件系统 staging、previous rename 与回滚流程完成双目录事务恢复；大目录复制不累计打开文件句柄。
- 从备份成功到 migration、framework shutdown 结束之间的任一失败均返回具体 `--restore-from` 命令；测试已覆盖 create/restore、缺失 snapshot 不产生半恢复、参数互斥、dry-run 零备份、apply 前备份和备份失败阻止启动。

RV2-07 当前状态为 `complete`。typed migration command、资源释放 owner command/回执、owner apply command、projection rebuild revision、本地维护 CLI、强制备份和启动前事务恢复已完成，并已在本机现有 file-backend 数据的隔离副本上完成 dry-run、apply、二次 dry-run 和 restore 全流程验证。真实生产目录的正式 apply 仍只允许在 RV2-08 维护窗口执行。

### RV2-07 数据副本验证记录（2026-07-13）

验证使用隔离副本：

```text
/tmp/workorch-rv2-closure-audit.pLuWZm
```

原始 `.workorch-data` 与 `.workorch-workspaces` 未被修改。副本 dry-run：

```json
{
  "DryRun": true,
  "Report": {
    "ScannedV1Runs": 24,
    "SupersededRuns": 0,
    "ClearedActiveRunIDs": 0,
    "SupersededTickets": 0,
    "SupersededApprovals": 0,
    "ResourceScopesToRelease": 0,
    "ReleasedResourceScopes": 0,
    "ProjectionRebuildRevision": 0,
    "Failures": []
  }
}
```

副本 apply：

```json
{
  "DryRun": false,
  "BackupDirectory": "/tmp/workorch-rv2-closure-audit.pLuWZm/retirement-backup",
  "Report": {
    "ScannedV1Runs": 24,
    "SupersededRuns": 0,
    "ClearedActiveRunIDs": 0,
    "SupersededTickets": 0,
    "SupersededApprovals": 0,
    "ResourceScopesToRelease": 0,
    "ReleasedResourceScopes": 0,
    "ProjectionRebuildRevision": 2,
    "Failures": []
  }
}
```

apply 后再次 dry-run 保持零动作，确认 migration 幂等。随后执行 restore，并对 data/config 全目录重新计算 hash：

```text
data before/after restore:
c2ea531645144d05da6cb5ebdbe7b539401fe09f380fe2c1da552145b3424330

config before/after restore:
4a35a07e93bc6a3dc98b4a92f939c2a32bf8cce489a566d86a210985eb126aaf
```

两组 before/after hash 完全一致，备份恢复链闭环通过。

### RV2-08 fresh Runtime V2 live 验证记录（2026-07-13）

验证使用全新隔离 file-backend，服务监听 `127.0.0.1:18080`，mock OpenAI-compatible provider 监听 `127.0.0.1:19090`。两个 workspace：

- LSP：`ws-bfe81d5d1f25fb67`。
- Gateway：`ws-9a668efb16798b53`。

每个 workspace 定义一个 Goal 和三个 DoD item；每个 DoD item 对应一个独立 focus，并依次执行 `produce_validation -> produce_assessment`。两个 workspace 各连续创建 5 个 Run，共 10 个 Runtime V2 Run。

成功路径核对结果：

```text
workspaceCount=2
runCount=10
completedRunCount=10
acceptedResultCount=10
providerRequestCount=60
provider candidateAttempt 0=60
provider candidateAttempt 1=0
projection ready=2/2
activeRunId empty=2/2
runtime_internal_contract_misalignment=0
```

每个 Run 的 persisted aggregate 均满足：

```text
RuntimeContractVersion=v2
Decision.Action=complete
Terminal.Code=coverage_satisfied
AcceptedEvidence=3
RejectedEvidence=0
Assessments=3
RemainingFocuses=0
CandidateAttempts=6，全部 attempt=0/state=accepted
CompletedDispatches=6
ActiveDispatch=nil
GovernanceRequests=0
Result.State=accepted
Result.DoDAssessment.Satisfied=true
Result.EvidenceRefs=3
```

验证期间先用一个协议解析错误的 mock 暴露了真实 P0 缺陷：candidate attempt 1 失败后，因为 governance capability 存在，controller 把 failure 重新归约为 `request_governance`，而 governance candidate 又依赖同一失效 provider，导致 authority attempt 持续增长并最终命中 1024 transition 上限。该问题不是 mock 专属；任意 provider 连续失败都可触发递归。

收口修改限定在 `kernel/orchestrator` candidate owner：

- candidate attempt 1 的非 internal failure 统一写入 typed `FailureClassCandidateExhausted`。
- reducer 直接生成 terminal `candidate_retry_exhausted`。
- 不再把 provider/schema/contract exhaustion 重新解释为新的 LLM governance candidate。
- 显式业务 `FailureClassGovernanceDependency` 仍保留原有 governed wait 主链，不受影响。

修复后增加强制 provider 503 live Run：`ws-fdef8e39455b48b4/run-001`。持久化结果：

```text
Run.State=failed
Decision.Action=fail
Terminal.Code=candidate_retry_exhausted
CandidateAttempts=2
attempts=[0,1]
两个 attempt 使用同一 AuthoritySnapshotID
provider request count=2
ActiveDispatch=nil
Workspace.ActiveRunID=""
```

该 live failure gate 证明同一 authority 只允许一次普通请求和一次 same-contract retry，不再产生第三次 provider 调用、governance 递归或嵌套 retry。

当前 RV2-08 代码侧与 fresh live gate 已通过，但本文仍保持 `active`，原因仅剩正式切换维护条件：

1. 当前验证 binary 来自未提交工作树，不能满足 `vcs.revision=<target commit>`、`vcs.modified=false`。
2. 真实生产目录尚未执行正式 `runtime-v1-retirement --apply`；本轮只在隔离副本执行 apply/restore。
3. 目标提交完成后，必须使用 clean build 按“全面切换维护窗口”执行正式 migration，并复核非终态 V1 Run、open blocker、stale `ActiveRunID` 均为 `0`。

### 旧 CC 收口项处置

本文后续现有 CC01 至 CC06 内容保留为历史问题、测试证据和删除检查清单，不再作为可独立执行的 V1 修补计划。正式实施只以 RV2-01 至 RV2-08 为准：

| 历史项 | V2 归属 | 处置 |
|---|---|---|
| CC01 Orchestrator authority/retry | RV2-01、RV2-02、RV2-05、RV2-06 | 由 typed contract、single reducer 和 sealed transaction 替换，不继续修补 V1 |
| CC02 LLM request/contract | RV2-04 | 收口为 candidate boundary，删除 LLM convergence 推导 |
| CC03 projection/read model | RV2-07、RV2-08 | 服务于 retirement migration、projection rebuild 和 V2 readback |
| CC04 runtime/validation resource | RV2-03、RV2-05、RV2-07 | 纳入 prerequisite ownership、evidence commit 和 retirement resource release |
| CC05 terminal reason | RV2-01、RV2-02、RV2-05 | 由 typed `TerminalOutcome` 单源生成 |
| CC06 application scheduler | RV2-06、RV2-07 | 删除 V1 continuation，scheduler 仅触发 V2 owner command |

如果后续 CC 条目与本节 Runtime V2 决策冲突，以本节为准。V1-specific Required Changes 只能用于确认删除是否完整，不能据此新增兼容代码。

### Architecture Boundary

本轮收口不得改变以下边界：

- module/block 间交互继续只能通过 magicCommon EventHub。
- command/data/result 继续由投递组件定义具体类型，消费组件导入投递方 `pkg` 合同。
- 不得恢复 Envelope、MagicHub、callback、service injection、repository export、JSON marshal/unmarshal bridge、reflect contract 或组件间 `map[string]any` payload。
- `kernel/orchestrator` 负责 convergence 与 recovery owner 语义；`blocks/llm` 只负责 provider request rendering、budget、cache 和结构输出边界，不能在 LLM block 内决定 convergence state。

## CC01：`kernel/orchestrator` 收口

### 职责边界

Orchestrator 是 convergence、coverage、evidence、repair/replan、runtime block dispatch 与 completion gate 的权威 owner。本项只完善中立合同治理，不把具体语言、命令、文件或业务验收逻辑写入 core。

### Required Changes

1. 建立 dispatch 前唯一 CurrentActionContract 归一化入口。
   - 输入只能来自当前 convergence authority snapshot 和当前 correction contract。
   - 顶层 `requiredOutput`、`repairActionPackage.requiredOutput`、`stagePacket.requiredOutput`、`currentActionContract.requiredOutput` 与 execution context mirror 必须同源生成。
   - 旧 mirror 只能进入 diagnostic lineage，不得再次参与当前合同计算。
   - 正常执行路径不应继续依赖 `stale_*_mirror_rebound` 自愈；出现 stale mirror 时应在权威刷新边界消除根因。

2. 增加 dispatch semantic preflight。
   - `need_state_change` 必须允许并要求至少一种 state-changing output。
   - `need_validation` 必须只允许当前 capability 可执行的 validation output。
   - validation node 必须声明结构化 evidence mode、observation type、target refs 和 observed claims。
   - controlled substitute 必须携带 substitute relation，并遵守 scratch/resource policy。
   - accepted kinds、forbidden kinds、runtime stage、repair objective 和 output mode 发生冲突时 fail-fast，不调用 provider。

3. 收口重复 failure signature。
   - 相同 authority snapshot、focus、rule、target scope 和实际输出摘要构成稳定 dedupe key。
   - 同一无效输出不得重复写入多份 rejected evidence。
   - forced-narrow 后再次命中同一规则时，应缩短为结构化修正或进入明确治理分支，不再提交等价完整 repair prompt。
   - repair budget 应按有效策略尝试计数，heartbeat、mirror rebound 和等价 schema retry 不得虚增业务 repair attempt。

4. 统一终态原因输入。
   - 保存 direct execution failure、contract/recovery termination reason、root evidence refs 和 residual gap。
   - Run failure 与 Result rejection 从同一个 terminal outcome input 派生，但保留不同展示字段。

### Target Areas

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_result_recording.go`
- `internal/modules/kernel/orchestrator/biz/runtime_result_finalize_event.go`

### Verification Gate

- 构造 stale mirrors 与 fresh authority 并存的 fixture，dispatch 后所有 active contract mirror 必须完全一致。
- `need_state_change` + validation-only requiredOutput 必须在 provider 调用前失败。
- static-only validation graph 必须只记录一次 rejected evidence，并产生下一条非等价 action。
- 同一 failure signature 连续返回时，repair budget 和 LLM 调用次数保持有界。
- terminal outcome 同时保留 direct failure 与 recovery termination 信息。

## CC02：`blocks/llm` 收口

### 职责边界

LLM block 只负责单一 LLM 调用生命周期中的 request rendering、provider 调用、structured output、budget 和 cache。它不决定 convergence state，不修改 coverage，也不接受旧 diagnostic contract 作为当前动作权威。

### Required Changes

1. Current action 独占 provider-facing 动态合同。
   - C3/C4 中只能有一套 authoritative requiredOutput。
   - 旧 stage packet、repair package、expected contract 和 rejected output 必须压缩为 diagnostic refs/summary。
   - prompt 必须明确 diagnostic lineage 不可覆盖 current action。

2. 调整上下文预算与压缩优先级。
   - 保护 Goal/DoD、current focus、current action、required output、capability snapshot 和最新有效 evidence。
   - 优先移除重复 mirror、重复 repair package、重复 rejected output 和全量历史 diagnostics。
   - retry 时采用随 attempt 增强的压缩策略，避免 request bytes 和 estimated tokens 随 lineage 单调增长。

3. 增加结构输出预检。
   - 检查 execution graph node 是否满足当前 requiredOutput 的结构字段。
   - 缺少 evidence mode、observation、target refs 或 accepted kind 时返回具体 provider-output diagnostic。
   - 预检只做结构和合同一致性判断，不判断具体业务行为是否正确。

4. 保持 cache prefix 稳定。
   - current repair、failure、focus 和 diagnostics 只能进入动态 tail。
   - `stablePrefixHash` 与 `renderedStablePrefixHash` 不得因 retry attempt 或 current failure 漂移。

### Target Areas

- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/provider_cache_plan.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`

### Verification Gate

- stale stage packet 和旧 expected contract 不得作为并列 instruction 渲染。
- 100K token 级历史输入 fixture 经压缩后，current action 所需字段完整且无 integrity warning。
- retry attempt 增长时 request body 不再随重复 diagnostics 线性增长。
- static-only validation graph 在 block 输出边界产生结构化 diagnostic。
- stable prefix regression tests 通过。

## CC03：`application/workorchd` 投影收口

### 职责边界

Application 负责 read model、Workspace/Run snapshot、HTTP/CLI/TUI facade 和 projection overlay。它不得修复 owner 状态，但必须忠实选择和展示当前 owner authority。

### Required Changes

1. 定义唯一 effective-run selection。
   - 优先选择 Workspace `activeRunId` 指向的非终态 Run。
   - 无 active Run 时，选择最新有效终态 successor，而不是旧 superseded Run。
   - direct Run snapshot、Workspace detail、summary 和 observability 使用同一选择函数。

2. 收口 projection overlay。
   - cached projection 只能覆盖同一 workspace/run scope。
   - 旧 Run 的 convergenceStatus、heartbeat、outcome 或 runtime execution 不得覆盖新 Run。
   - projection rebuild 和服务重启后必须保持相同选择结果。

3. 暴露只读一致性诊断。
   - Workspace selected run 与 convergenceStatus.runId 不一致时输出 diagnostic。
   - 该 diagnostic 不修改 owner 数据，也不作为 scheduler 自动恢复依据。

### Target Areas

- `internal/modules/application/workorchd/biz/appservice/readmodelports/workspace_snapshot.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelports/workspace_snapshot_run_derived_loading.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelports/workspace_snapshot_effective_readers.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/run_status_projection.go`

### Verification Gate

- `run-001/superseded -> run-002/failed` 时 Workspace snapshot 必须选择 `run-002`。
- Workspace 无 activeRunId 时仍能选择最新终态 Run。
- direct Run snapshot 与 Workspace snapshot 的 convergenceStatus/run outcome 一致。
- file backend restart 和 projection rebuild 后结果不漂移。

## CC04：`kernel/run` 终态收口

### 职责边界

Run module 拥有 Run、Step 与 checkpoint 状态。Orchestrator 决定 convergence 与终态语义，Run 负责原子保存和发布正式状态，不自行推导业务完成条件。

### Required Changes

1. 明确 Step terminal 顺序。

```text
runtime worker terminal
  -> Step output persisted
  -> Step state terminal
  -> runtime finalize checkpoint persisted
  -> resources released
  -> next continuation dispatch or Run terminal
```

2. 同一个 Step 已有 `doneAt` 时，`pending=true` 只能存在于有界 finalize 窗口；超过窗口必须由 durable recovery 检查接管。
3. 下一 repair Step 被接受后，Workspace/Run projection 不得继续把前一 Step 作为 current runtime execution。
4. Run failure command 接收结构化 terminal reason input，至少区分 direct failure、recovery termination 和 evidence refs。

### Target Areas

- `internal/modules/kernel/run/biz/run_service.go`
- `internal/modules/kernel/run/pkg/contracts/commands.go`
- `internal/modules/kernel/run/pkg/events/messages.go`
- Orchestrator 与 Run 的 terminal/finalize pairwise tests。

### Verification Gate

- worker done、finalize event 与 resource release 的事件顺序可重复回放。
- 服务在 worker done 后、finalize 前重启时能够幂等收口。
- 新 Step dispatch 后 current runtime execution 不再引用旧 Step。
- terminal Run 的 failure reason 不丢失 direct execution root cause。

## CC05：`blocks/result` Pairwise Contract 收口

### 职责边界

Result block 只管理 Result/Artifact 正式聚合。它不决定 Run 是否失败，也不从事件历史重新推导失败原因。

### Required Changes

1. 扩展 Orchestrator 投递的正式 Result 输入，使 rejected Result 可表达：
   - direct failure summary；
   - recovery termination summary；
   - root evidence refs；
   - residual gaps / next actions。
2. Result owner 负责结构校验、持久化和 typed notification，不自行读取 Run/Orchestrator repository。
3. 保持 Run 与 Result ��展示语义可关联，但不要求两个 summary 字符串完全相同。

### Target Areas

- `internal/modules/blocks/result/pkg/events`
- `internal/modules/blocks/result/pkg/models`
- `internal/modules/blocks/result/biz`
- `internal/modules/kernel/orchestrator/biz/runtime_result_recording.go`
- `internal/modules/kernel/orchestrator/biz/runtime_result_dod.go`

### Verification Gate

- 编译/执行失败后，Result 包含直接失败原因。
- repair budget 耗尽后，Result 同时包含恢复终止原因。
- Result 通知继续由 Result owner 单点发布。
- 不新增 Orchestrator 对 Result service/repository 的直接依赖。

## Cross-Component Contract

收口期间 module/block 交互继续只能通过 magicCommon EventHub：

- command/data/result 由投递组件的 `pkg/events` 定义。
- 消费组件按需导入投递组件事件合同。
- 同步命令必须使用 `Send` 并返回具体 result。
- 通知使用 typed `Post/Subscribe`。
- 不得恢复 callback、service injection、repository export、Envelope、`map[string]any` 顶层 payload、JSON marshal/unmarshal 或 reflect bridge。

共享中立 runtime contract 可以继续位于 `internal/pkg/workorchruntimecontract`，但不得承载某个 owner 的状态转换实现。

## Explicit Non-Scope

当前没有证据要求修改以下组件：

- `blocks/workspace`：失败后已正确释放 `activeRunId`。
- `kernel/eventstream`：事件 revision 连续，typed event 持久化正常；当前问题发生在投影选择而非事件缺失。
- `blocks/file`、`blocks/git`、`blocks/shell`：patch、Git 和验证命令执行链正常。
- `blocks/mcp`、`blocks/mcpregistry`：当前 LSP 失败发生在 graph contract admission 阶段，尚无 MCP invocation/registry 故障证据。
- `kernel/governance`、`blocks/decision`、`blocks/approval`：当前没有 open Decision/Approval 阻塞。
- `workorch/repositories` initiator：repository 和 EventHub 装配正常。

若后续 pairwise test 或 live audit 出现新的 owner 证据，再将对应组件显式加入本文，不得预防性修改全部 22 个组件。

## Closure Order

1. CC01：先关闭 Orchestrator 单一 authority、semantic preflight 和重复 retry。
2. CC02：在新 authority 合同之上收口 LLM prompt、budget 和 structured output preflight。
3. CC03：修复 Workspace effective-run projection，确保后续 live 核对可信。
4. CC04：收口 Step/Run terminal finalize 顺序和恢复窗口。
5. CC05：统一 Run/Result terminal reason pairwise contract。
6. CC06：重新部署并执行双 workspace live verification。

CC01 和 CC02 必须先完成，避免在错误 provider-facing contract 上继续修补 projection 或终态展示。

## Verification Matrix

### Focused Tests

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelports -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/kernel/run/... -count=1
go test ./internal/modules/blocks/result/... -count=1
```

### Component and Boundary Gates

```bash
go test -p 1 ./internal/modules/... -count=1
make fmt-check
make vet
make verify-boundary-contract
git diff --check
```

### Static Boundary Scan

```bash
rg -n "Envelope|MagicHub|RawHub|json\.(Marshal|Unmarshal)|reflect\.Type|map\[string\]any" \
  internal/modules/kernel/orchestrator \
  internal/modules/kernel/run \
  internal/modules/blocks/llm \
  internal/modules/blocks/result \
  internal/modules/application/workorchd
```

扫描结果中的内部算法 map 必须逐项判断；任何组件间 command/data/result 或 JSON/reflect 类型桥必须为零。

## Live Verification Gate

重新部署后至少核对：

### LSP 类 Workspace

- current focus 能持续前移，而不是只增长 heartbeat revision。
- rejected evidence 不再因同一 failure signature 快速增长。
- `stale_required_output_mirror_rebound` 与 `stale_stage_packet_mirror_rebound` 不再按每轮 dispatch 累积。
- concrete validation repair 能产生符合当前 requiredOutput 的动态 observation。
- repair request bytes/tokens 不随 lineage 单调增长。
- `stablePrefixHash` 与 `renderedStablePrefixHash` 保持稳定。

### 不完整交付物 Workspace

- compile/build failure 能驱动明确的 `need_state_change`。
- state-changing requiredOutput 至少允许一个当前 capability 可执行的状态变更 kind。
- 缺失实现能够被补齐，或在存在真实 blocker 时进入明确治理状态。
- Run 与 Result 同时保留直接失败和恢复终止原因。
- Workspace snapshot 与直接 Run snapshot 指向同一 effective Run。

### Projection and Service Health

- `/readyz` projection 为 `ready`。
- workspace-scoped source/projected revision 对齐。
- active Run 的 runtime execution 指向最新 Step。
- terminal Run 不残留 activeRunId、pending worker 或未释放资源。

## Completion Definition

本文只能在以下条件全部满足后标记 `completed`：

- CC01 至 CC05 的 focused tests 和 pairwise tests 全部通过。
- 组件交互仍符合 EventHub-only 与 typed contract 约束。
- 两类 live workspace 不再复现同类 stale mirror、contract retry loop、effective-run projection drift 和 terminal reason split。
- 至少一个真实 active workspace 连续推进多个 coverage focus，且 rejected evidence 未因重复合同错误无界增长。
- 至少一个真实 failure/recovery workspace 的 Workspace snapshot、Run snapshot、Result 和 runtime execution 终态一致。
- 全量组件测试、格式、vet、boundary contract 和 diff check 通过。
