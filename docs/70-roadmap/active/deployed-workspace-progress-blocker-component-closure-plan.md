# 已部署 Workspace 推进阻塞组件收口计划

Status: active

Type: closure-plan

Last Updated: 2026-07-11

Related:

- `../../00-core/runtime-stage-contract.md`
- `module-block-initiator-inventory-audit-2026-07-09.md`
- `current-action-contract-authority-closure-plan.md`
- `runtime-validation-required-output-authority-closure-plan.md`
- `runtime-terminal-outcome-authority-closure-plan.md`
- `workorchd-state-loop-and-projection-closure-plan.md`

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
