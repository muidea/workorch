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
| CC01 | `kernel/orchestrator` | P0 | 单一 current action authority、validation preflight、repair 去重与终态原因归一化 | completed |
| CC02 | `blocks/llm` | P0 | provider-facing 当前合同隔离、上下文压缩和结构输出预检 | completed |
| CC03 | `application/workorchd` | P0 | Workspace/Run projection effective-run 一致性 | completed |
| CC04 | `kernel/run` | P1 | Step/Run terminal finalize 顺序与 failure reason 合同 | completed |
| CC05 | `blocks/result` pairwise contract | P1 | 正式 Result 同时保留直接失败与恢复终止语义 | completed |
| CC06 | live verification | P0 | 重新部署并确认两个 workspace 不再复现同类阻塞 | pending |

## 2026-07-11 Code Closure Progress

- CC01：派生 `requiredOutput` / `stagePacket` mirror 改为无噪声确定性重建；真正的最终 mirror 不一致仍由 dispatch preflight 返回 conflict。`authorityDiagnostics` 对完全相同的诊断执行幂等去重，不再在同一输入中重复累积。
- CC02：存在 `currentActionContract` 时，provider budget 不再恢复旧 `stagePacket.nextAction`、`stagePacket.requiredOutput`、current gap/failure 等并列合同；Stage Packet 只保留 target snapshot transport 与 scope identity。`outputDecisionContract` 和 current-action target snapshots 直接进入 Current Action Contract prompt。
- CC03：Workspace 无 `activeRunId` 时，如果存在正式 Result 或 terminal Run event，projection 会选择该最新终态 successor；没有正式终态证据时仍保持无 projection run，避免把任意历史 Run 当作当前状态。
- CC04：核对确认 `runtime_terminal_not_finalized` 是 runtime result available 到 Step formal finalize 之间的有界暂态；现有 Step terminal、checkpoint、resource release 与 restart recovery 测试通过，本轮不修改 Run owner 状态机。
- CC05：Run terminal outcome 在 execution context 中保存 `directFailureReason`、`recoveryTerminationReason`、`sourceStepId` 和 evidence refs；Run `FailureReason` 同时包含直接失败与恢复终止原因。Rejected Result 的 summary diagnostics 同步保留两类原因。
- 边界门禁补齐：`blocks/decision` 与 `blocks/approval` 的 `RecordEvidence` command topic 已补入 owner `Contracts()` 和具体 command/result `MessageContracts()`；不改变现有 EventHub 投递方向或状态 owner。
- CC01 至 CC05 focused/pairwise tests 已通过；文档继续保持 `active`，等待 CC06 重新部署后的 live verification。

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
