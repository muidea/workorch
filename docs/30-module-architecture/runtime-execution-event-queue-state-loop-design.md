# Runtime Execution Event Queue State Loop Design

Status: implemented / validated

Type: module-architecture

Last Updated: 2026-06-18

## Purpose

本文固化 WorkOrch runtime step 执行链路的长期收口方向：将当前 `orchestrator -> runtime block -> result -> follow-up` 的同步递归调用链，调整为基于 EventHub command / notification / pending queue 的事件驱动状态循环。

当前阶段不考虑兼容性。本文作为本轮代码收口的拆分范围、待办项、验收依据和最终核对记录。

## Original Runtime Blocker

本设计最初来源于部署实例中的历史 blocker：网络恢复后已确认 LLM provider 能返回，但 `ws-8f13ca30356d906f` 的 run 仍停留在运行态：

- workspace: `ws-8f13ca30356d906f`
- run: `run-008-replan-001-replan-001-replan-001-replan-001-replan-001-r-replan-004`
- step: `057-repair-002-of-graph-009-002-verify-concurrent-safety`
- step kind: `llm/workspace_execution`
- observed state: `dispatched`
- provider stream checkpoints: `runtime_step_done`, `doneReceived=true`, `streamIncomplete=false`
- persisted LLM call evidence: `request.json` / `response.json` / `response.sse` exist
- missing persisted step evidence: no step `output.json`, no `llm-calls/.../usage.json`, no completed/failed step state
- later checkpoints repeatedly show `step_dispatched` for the same step

Goroutine stack shows the active chain is blocked around:

```text
orchestrator.dispatchStepWithWorkspace
  -> RuntimeAdapterRegistry.ExecuteRuntimeStep
  -> eventRuntimeAdapter.ExecuteRuntimeStep
  -> MagicHub.Publish
  -> event.Hub.Send
```

同一 stack 中还出现深层递归的 graph / repair / follow-up dispatch：

```text
dispatchGraphStepBatchWithWorkspace
dispatchRuntimeRepairFollowUp
dispatchStepWithWorkspace
dispatchGraphStepBatchWithWorkspace
...
```

这说明原始问题不是 provider 不可用，也不是 TUI 显示态误差，而是 runtime execution 主链路把长耗时 block 执行、step finalize、graph continuation、repair/follow-up 派发绑定在同一个同步调用栈中，导致 provider 已返回后仍可能无法稳定完成 step 收口。当前实现已经按本文 Phase 1-6 完成主路径拆分，并通过下文 Phase 6 的 deployed-instance observation、mock provider e2e、全量测试和构建验证完成闭环。

## Design Intent

目标设计：

- Orchestrator 是 run/step 状态机 owner，负责创建 step、接收 runtime result、finalize step、选择下一步推进。
- Runtime block 只执行技术能力，不直接推进 run 状态机，也不递归触发后续 step。
- EventHub command 只表达清晰意图；长耗时执行不在 observer 调用栈里完成。
- Runtime execute request 进入 pending queue 后由 BackgroundRoutine worker 执行。
- Runtime result 以 event 形式返回 orchestrator，由 orchestrator 状态循环处理。
- Step finalize 和 next dispatch 之间通过明确事件断开，不在同一个 `Send` 调用栈中递归执行。
- 同一 workspace/run 内保持必要顺序；不同 workspace、不同 step 的无共享状态执行不被全局 lane 串行化。
- WorkOrch core 保持业务中立，不把软件开发、LSP、Go、具体测试命令等策略写入 runtime scheduler 或 orchestrator core。

## Pre-Implementation Scope Baseline

本节用于锁定本轮代码收口范围和后续增量修改边界。后续每次改代码前，必须先确认改动命中本节中的一个待办项或明确新增设计项；不能把 live workspace 的个别现象扩散成无边界的 prompt、TUI、watchdog 或 template 特判修改。

### Confirmed Mainline Defect

当前已确认的主线缺陷是：

```text
orchestrator.dispatchStepWithWorkspace
  -> runtime adapter registry
  -> runtimeevent block execute command
  -> MagicHub.Publish
  -> event.Hub.Send
  -> runtime block observer
  -> provider / shell / MCP / file / git long-running execution
  -> orchestrator finalize / graph / repair recursive continuation
```

该链路的问题不是“`Send` API 不能使用”，而是把长耗时 runtime execution 和后续递归调度放进了同步 `Send` 的 observer lane。`Send` 仍保留给短同步 owner command；runtime block execution 必须从同步 observer 调用栈拆出。

### Long-Term Closure Direction

最终长期收口方向为：

- 用户入口、Web REST、TUI、scheduler 只产生 command 或 accepted work item，不直接执行 runtime block。
- Orchestrator 只拥有 run/step 状态机、step 创建、result finalize、下一步 dispatch 决策。
- Runtime block executor 只拥有技术能力执行队列，observer 只校验、去重、入队并快速返回 accepted。
- 长耗时执行由 framework 注入的 `BackgroundRoutine` worker 承载；不裸起长期 goroutine。
- completed / failed / progress 通过 runtimeevent owner event 回传。
- Orchestrator 订阅 runtime result event，按同一 workspace/run state lane 幂等 finalize。
- Graph、follow-up、repair、completion continuation 通过 dispatch requested event 滚动推进，不再递归 Go 调用。
- Watchdog 只识别 request / accepted / progress / done / finalize gap，并发布恢复 command；不直接重复调用 provider。
- Web/TUI 只展示 workorchd projection，不实现端侧恢复状态机。
- Core 修改保持业务中立；软件开发、验证命令、LSP、具体文件类型等策略只能来自 workspaceTemplate / Soul / capability contract。

### In Scope

本轮长期收口范围包括：

| Area | Scope |
| --- | --- |
| Runtime event contract | runtime execute requested / accepted / progress / completed / failed payload、lane、idempotency key |
| Runtime execution queue | observer 轻量入队、BackgroundRoutine worker、queue key、shutdown、duplicate request |
| Runtime block adapters | 保留同步 `ExecuteRuntimeStep` 作为 worker 内部接口，移除生产 observer 内直接执行 |
| Orchestrator state loop | step create/request、runtime result subscriber、finalize、dispatch requested、graph/repair/follow-up 非递归化 |
| Run/step owner state | dispatched step 的 request id、accepted/progress/done/finalize gap 元数据 |
| Watchdog/recovery | request not accepted、accepted no progress、done not finalized、worker lost 的分型诊断/恢复 |
| Projection/ViewModel | 展示 pending/finalize gap；端侧不自行推断或补偿 |
| Tests/e2e | contract、lane、queue、finalize、duplicate、multi-workspace、restart recovery、CPU/IO gate |

### Out Of Scope

本轮不纳入：

- 为某个 workspace 目标、某种编程语言、某个测试命令或某个 MCP 工具写 core 特判。
- 用扩大 prompt、调整 Soul 文本或增加 LLM 禁止项来替代 runtime state loop 收口。
- 让 TUI/Web 在端侧检测卡住并自动补发 replan/restart。
- 在 `internal/pkg/events` 新增业务 topic 字符串或 runtime block 业务 payload owner。
- 通过增大 timeout、降低 watchdog 频率、忽略 duplicate event 来掩盖同步主链路问题。
- 在生产路径新增裸 goroutine、全局 channel 或绕过 framework `BackgroundRoutine` 的局部 worker。

### File-Level Review Targets

代码收口前必须逐一核对以下文件或包：

| Path | Review Focus |
| --- | --- |
| `internal/modules/blocks/pkg/runtimeevent/runtime_event.go` | observer 是否仍同步执行 adapter；是否具备 queue/worker/accepted/completed/failed 事件 |
| `internal/modules/blocks/*/biz/runtime_adapter.go` | adapter 是否只做技术能力执行；progress 是否可转 runtime progress event |
| `internal/modules/kernel/orchestrator/runtime_event_adapter.go` | orchestrator 发布 execute request 是否仍走同步 result 语义 |
| `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go` | `dispatchStepWithWorkspace` 是否同步等待 result；finalize 是否可拆出 |
| `internal/modules/kernel/orchestrator/biz/runtime_graph.go` | graph batch 是否仍用 goroutine/waitgroup 同步等待 terminal |
| `internal/modules/kernel/orchestrator/biz/runtime_repair.go` | repair/follow-up 是否仍递归 dispatch runtime block |
| `internal/modules/kernel/orchestrator/biz/*convergence*` | completion/acceptance/failure 是否依赖同步调用栈副作用 |
| `internal/modules/kernel/run/**` | run/step owner state 是否能表达 pending/accepted/progress/finalize gap |
| `internal/modules/application/workorchd/service/server/**` | scheduler/watchdog 是否直接补偿执行；是否只投递 command/work item |
| `internal/modules/application/workorchd/biz/appservice/**` | projection 是否从 owner/runtime events 聚合，不重新推导状态机 |
| `internal/pkg/events/event_contract.go` | lane policy 是否只承载 owner state 顺序，不承载长耗时执行本体 |
| `internal/pkg/workorchruntimecontract/**` | payload DTO 是否中立、无 owner model 反向依赖 |
| `internal/services/workorch/service/tui/**` | TUI 是否只消费 projection/port DTO，不保存第二套状态机 |

### Final Closure Todo Matrix

本矩阵是后续逐一代码收口的权威待办。每个实现 PR / commit 必须只声明命中其中一组或少数组相关项；未命中矩阵的修改不得作为本问题收口。

| ID | Owner | Scope | Required Change | Required Tests / Evidence |
| --- | --- | --- | --- | --- |
| RQ-01 | `blocks/pkg/runtimeevent` | execute command observer | observer 只 decode、scope 校验、幂等去重、入队并返回 accepted；禁止在 observer 内调用长耗时 adapter | 单测证明 blocking adapter 不阻塞 `Publish/Send` 返回 accepted；goroutine stack 不再显示 adapter 卡在 EventHub observer lane |
| RQ-02 | `blocks/pkg/runtimeevent` | worker lifecycle | worker 通过 framework 注入的 `task.BackgroundRoutine` 执行；shutdown 后拒绝新任务；不裸起长期 goroutine | 单测覆盖 enqueue、duplicate request、shutdown reject、completed、failed、progress |
| RQ-03 | `blocks/pkg/runtimeevent` / `workorchruntimecontract` | runtime event contract | `requested/accepted/progress/completed/failed` DTO 保持业务中立，payload 携带 workspace/run/step/request scope | `go test ./internal/modules/blocks/pkg/runtimeevent ./internal/pkg/workorchruntimecontract -count=1` |
| RQ-04 | `internal/pkg/events` | lane policy | accepted 走 runtime-exec lane；completed/failed 走 orchestrator workspace/run state lane；不在基础包定义 owner topic 字符串 | lane 单测覆盖跨 workspace 不串扰、同 run result 有序 |
| OR-01 | `kernel/orchestrator` | runtime request publisher | 生产主路从“同步等待 `StepExecutionResult`”改为“持久化 dispatched + 发布 execute request + 记录 request id” | 单测证明 start/resume/replan 返回 accepted 后不等待 block result |
| OR-02 | `kernel/orchestrator` | result subscriber | 新增 runtime completed/failed subscriber，作为唯一正式 step finalize 入口 | 单测证明 completed event 后 step terminal；failed event 后进入恢复/decision/fail 路径 |
| OR-03 | `kernel/orchestrator` | idempotent finalize | duplicate completed/failed event 不重复 patch apply、memory write、result/artifact、decision/approval ticket | 单测覆盖 duplicate result event side-effect 只发生一次 |
| OR-04 | `kernel/orchestrator` | graph continuation | graph batch 不再 goroutine + `WaitGroup` 同步等待 terminal；依赖满足后发布 dispatch requested event | 单测证明 ready nodes 入队后返回；依赖满足由后续 event 推进 |
| OR-05 | `kernel/orchestrator` | repair/follow-up | `dispatchRuntimeRepairFollowUp` 不递归调用 runtime block；只创建 repair/provider retry step 并发布 execute request | 单测覆盖 runtime failed event 触发 provider retry/runtime repair/decision exhaustion |
| OR-06 | `kernel/orchestrator` | finalize failure path | output contract invalid、context gate failure、patch apply failure 等必须 terminal 或产生结构化 blocker，不得停留 dispatched | 单测覆盖 invalid output 不产生长期 dispatched |
| RN-01 | `kernel/run` / read model | step runtime state | dispatched step 的 request id、acceptedAt、lastProgressAt、doneAt/finalize gap 由 runtime checkpoint 聚合到服务端 read model | run snapshot/status 单测覆盖 pending/finalize gap 字段 |
| WD-01 | `workorchd/service/server` | watchdog classification | watchdog 按 request not accepted、accepted no progress、done not finalized、worker lost 分型诊断/恢复；request not accepted 不自动恢复 | scheduler/watchdog 单测覆盖四类 gap，不把 done-not-finalized 重派给 provider |
| WD-02 | `workorchd/service/server` | recovery entry | HTTP/TUI 只提交 user command；watchdog 只发布恢复 command/work item，不直接执行 runtime block | 单测/代码搜索证明 server 层无 runtime adapter 直接调用 |
| PV-01 | `workorchd/biz/appservice` | projection/ViewModel | projection 聚合 owner/runtime events 展示 pending/progress/finalize gap；不重新推导正式状态机 | appservice snapshot 单测覆盖 running、waiting decision、finalize gap 展示 |
| SE-01 | `workorchd/biz/appservice` / `service/server` | backend session lifecycle | `WithBackend` / default backend / server backend 必须与异步 state loop 兼容；不能在 start/resume 返回后立即销毁 runtime session | 覆盖 `StartRunWithBackend`、`ResumeRunWithBackend`、`ActivateWorkspaceExecutionWithBackend`、recovery helpers 的回归测试 |
| UI-01 | `internal/services/workorch/service/tui` | terminal UI | TUI 只消费 projection/port DTO；prompt、主视图、workspace list 都以服务端 ViewModel 为准 | TUI presenter/viewmodel 单测覆盖 workspace 切换后 prompt 与 projection 同步 |
| UI-02 | Web | web UI | Web 只展示 REST projection；服务状态/工作区 detail 不实现端侧恢复状态机 | 前端/handler 单测或 e2e 覆盖服务状态不白屏、detail gap 可展示 |
| OBS-01 | scripts/e2e | runtime observation | 新增 deployed instance 观测脚本，确认 provider done 后 step terminal 或 finalize recovery | 两 workspace 并行 e2e；pprof goroutine 无递归 dispatch stack |
| PERF-01 | runtime/read model | CPU/IO gate | state loop 不因 repeated scan、projection rebuild、watchdog 重派造成持续高 CPU/IO | pprof/metrics 证据：长跑多轮后 CPU/IO 可回落 |

### Implementation Order

长期收口必须按以下顺序推进，避免“补偿恢复”掩盖主链路缺陷：

1. 完成 RQ-01 到 RQ-04：先让 runtime block execution 从 EventHub observer lane 中拆出。
2. 完成 OR-01 到 OR-03：让 orchestrator 以 runtime result event 为唯一 finalize 入口，并保证幂等。
3. 完成 OR-04 到 OR-06：拆除 graph / repair / follow-up 的同步递归主链路。
4. 完成 RN-01、WD-01、WD-02：用 owner state 和 watchdog 分型恢复覆盖进程重启、worker 丢失、done-not-finalized。
5. 完成 SE-01、PV-01、UI-01、UI-02：收口 backend session 生命周期，端侧只显示 projection，不复制状态机。
6. 完成 OBS-01、PERF-01：用 e2e、pprof、CPU/IO gate 验证长期运行。

### Validated Repository Checkpoint

当前仓库已经按本文 Phase 1-6 完成 runtime execution event queue state loop 主路径收口，并通过测试与部署观测验证：

- 已新增 runtimeevent accepted/progress/completed/failed topic 和基础 lane contract。
- 已新增 runtime event DTO 与 payload key。
- 已把 block module/bootstrap 的 BackgroundRoutine 接线作为 queue worker 前置条件。
- `runtimeevent.Executor` 已完成 observer 入队、BackgroundRoutine worker、accepted/progress/completed/failed 发布、shutdown reject、duplicate request 幂等与 terminal replay；同一 request id duplicate 不重复执行 adapter，但会重放 terminal event。Worker 在 accepted notification 发布成功后才被释放执行，accepted 发布失败会取消已入队 wrapper 并删除 queue key，避免“未确认 accepted 但外部执行已发生”的不可恢复状态。
- Runtime terminal event 发布成功后才写本地 terminal replay 状态；completed/failed 和 terminal replay 发布失败必须返回错误或记录可观测 warning，不能被静默吞掉。
- `eventRuntimeAdapter.ExecuteRuntimeStep` 目前仍保留为测试/内部兼容桥：它先等待 accepted，再同步等待 completed/failed event。生产 event-driven orchestrator 主路不得依赖该同步 result 接口。
- `internal/modules/kernel/orchestrator` 已增加 runtime completed/failed subscriber，并把 step finalize 抽成 `finalizeRuntimeStepResultWithWorkspace` / `FinalizeRuntimeStepResult` 统一入口。同步 fallback 只允许在未启用 event-driven dispatch 的测试/内部 adapter 路径使用。
- `dispatchStepWithWorkspace` 已优先使用 `RuntimeExecutionRequester` 发布 runtime execute request 并在 accepted 后返回，同时记录 `runtime_execute_accepted` checkpoint；生产 `eventRuntimeAdapter` 已实现该 requester，因此生产 EventHub runtime block 主路不再同步等待 `StepExecutionResult`。event-driven 模式下如果 requester 缺失或 step kind 仍注册为同步 adapter，会结构化失败为 `runtime_request_unavailable`，不会回退调用 `ExecuteRuntimeStep`。
- patch apply / preview / revert 已从生产 registry 的直接同步 adapter 注册切换为 `eventRuntimeAdapter`；orchestrator module 内部用 framework 注入的 `BackgroundRoutine` 启动 patch `runtimeevent.Executor`，patch 技术执行不再位于 EventHub observer 同步 lane。
- 异步 runtime request 不再携带 orchestrator progress callback；orchestrator module 已订阅 runtime progress event，并通过 `RecordRuntimeStepProgress` 在后台记录 checkpoint。同步 fallback 仍保留原 callback，待后续移除 fallback 时一并删除。
- step finalize 已增加按 step 的 terminal guard，并补充 completed / failed duplicate 回归测试，证明重复 runtime result 不会覆盖原 step output、重复写 validation、tool-call error、patch apply、memory write、accepted result/artifact、decision ticket 或 approval；OR-03 focused side-effect 覆盖已完成。
- orchestrator owner 已新增 `TopicOrchestratorDispatchStepRequested` 与 `DispatchStepCommand`；Module 订阅后只把 dispatch task 提交到 framework 注入的 `BackgroundRoutine`，observer 不执行 runtime block。
- event-driven dispatch 模式下，nextStep、graph ready batch、graph follow-up、completion follow-up、no-actionable follow-up、assessment-only correction、graph expansion limit follow-up、provider retry、runtime repair 和 artifact continuation 均发布 dispatch requested event；graph batch 不再 goroutine + `WaitGroup` 等待 terminal，同一 graph root 仍有 pending/dispatched 节点时不会提前触发 follow-up。
- dispatch requested event 与 `DispatchStepCommand` 已携带稳定 `idempotencyKey`，由 workspace/run/step/kind scope 生成，供 continuation / recovery 重复投递时保持同一幂等语义。
- `dispatchStepWithWorkspace` 在 enrich LLM step input 后、发布 dispatched / runtime execute request 前会立即持久化 step，避免异步 state loop 中 projection 或 snapshot 读到未带 `executionContextPackage` 的半成品 step。
- read model 已新增 `runtimeExecution` 投影，基于 runtime checkpoints 聚合当前 step 的 `requestId`、`queueKey`、`acceptedAt`、`lastProgressAt`、`doneAt/failedAt`、`finalizedAt` 和 gap 分类；RunSnapshot 与 RunStatus 均可展示 `request_not_accepted`、`accepted_no_progress`、`accepted_in_progress`、`runtime_terminal_not_finalized`。
- runtime completed/failed event 到达 orchestrator 后，会先写入稳定 ID 的 `runtime_step_result_available` recovery checkpoint，持久化 request id、terminal topic、output/error、failure class/signature 和 evidence refs，再进入 step finalize。
- `ResumeRun` 的 running recovery 已优先消费 `runtime_step_result_available` checkpoint：当前 terminal step 仍是 `dispatched` 时，直接使用持久化 `StepExecutionResult` 调用 finalize，不重新执行 provider/runtime adapter。
- stale dispatched recovery 已接入同一类 runtime checkpoint gap 分类；当只有 `runtime_step_done` / `runtime_step_failed` 但没有 durable terminal result 时，watchdog 不会重派 provider；当存在 `runtime_step_result_available` 时，watchdog 允许触发 resume，由 orchestrator owner 完成 finalize recovery。
- finalize failure path 已补充中立 stage 归一化与主链路测试：output contract invalid、context gate fail-fast、patch apply failure 均会让原 step 进入 failed 并带结构化 stage / failure diagnostics，不再可能保持长期 `dispatched`；OR-06 focused 覆盖已完成，后续只需随新增 failure gate 增量补测。
- stale dispatched recovery 已进一步收紧 accepted gap 策略：`request_not_accepted` 是主路径 dispatch/subscribe/queue 缺陷诊断，只投影展示，不由 periodic watchdog 自动恢复；`runtime_terminal_not_finalized` 只有存在 durable `runtime_step_result_available` 时才恢复；`worker_lost`、`accepted_no_progress` 和 `accepted_in_progress` 只有在 runtime recovery policy 明确允许 watchdog external retry，且具备 terminal evidence、worker-lost evidence 或 stale heartbeat 这类持久边界证据时才触发结构化 recovery command，避免 provider 仍运行时重复执行。
- `workorchruntimecontract.RuntimeRecoveryPolicy` 已作为中立 runtime contract 字段贯穿 dispatch request、accepted/progress/completed/failed event、runtime checkpoints、read model projection 与 `RecoverRuntimeGapCommand`。默认 `unknown/manual_only` 不允许 watchdog retry；只有显式 idempotent 或带 dedupe key 的 `retry_external` policy 才允许 supported accepted/worker-lost gap 自动恢复。
- `workorchd/service/server` watchdog 路径已核对：`recoverStaleDispatchedRuns` 只通过 appservice shared session / backend recovery 入口触发 owner recovery command，不直接调用 runtime adapter、runtime block execute topic 或 provider request；appservice recovery 已从统一 resume 收口为 gap-specific recovery command。
- TUI/Web 端侧状态审计已完成：Web `src` 主要展示服务端 recovery/projection 字段并提交用户 command；TUI 未发现端侧自动补发 provider/runtime recovery。
- TUI UI-01 第一轮已收口：prompt、主视图 tail、状态栏、inspect 和 blocker fetch/submit 均按当前 workspace/run scoped ViewModel 读取 blocker；服务端 workspace overview 为 running/validating/completed/failed 时会压掉旧本地 blocker 缓存，避免 workspace 切换后 `decision >` 或 stale decision command 污染当前 workspace。
- runtime executor 已通过 runtime progress 通道补充中立 worker lifecycle evidence：`runtime_worker_started`、`runtime_worker_heartbeat`、`runtime_worker_stopped`、`runtime_worker_lost`。`workorchd` read model 会将其投影为 `workerStartedAt`、`workerHeartbeatAt`、`workerStoppedAt`、`workerLostAt` 和 `worker_lost` gap；stale dispatched watchdog 只有看到明确 `runtime_worker_lost` 证据，或已存在 worker heartbeat 且 heartbeat 本身超过 stale minAge 时，才允许恢复 accepted gap，仍不会对无证据的 `accepted_no_progress` / `accepted_in_progress` 盲目重派 provider。
- stale dispatched watchdog 已从统一 resume 入口收口为 gap-specific recovery command：`workorchd` 通过 `TopicOrchestratorCommandRecoverRuntimeGap` 提交结构化 `RecoverRuntimeGapCommand`，payload 携带 workspace/run/step/gap/request/queue/latestStepTime/reason；orchestrator owner 将其转换为带 `runtimeGapRecovery` 的 background resume patch。watchdog/appservice 不直接调用 runtime adapter，也不再把不同 gap 混成无上下文 resume。
- gap-specific recovery command 主路径已补齐订阅：orchestrator module `Start()` 订阅 `TopicOrchestratorCommandRecoverRuntimeGap`，避免 appservice/watchdog 正确发布 recovery command 后因无 owner response 被判定为 `invalid argument`。
- 服务启动不再自动执行一次性 stale dispatched runtime gap 扫描；重启后历史运行态由用户显式恢复，周期 watchdog 只在服务运行期间基于持久 owner state 发布结构化 stale gap recovery command。
- 文件后端回归已覆盖 durable terminal result finalize：running run + dispatched step + `runtime_step_result_available` checkpoint 可通过 `RecoverStaleDispatchedRunsWithBackend` 恢复为 terminal，step output 保留 checkpoint marker，证明不重新执行 provider/runtime adapter。
- runtime executor 并发回归已覆盖：两个不同 workspace 的 runtime block request 在第一个 adapter 未释放时，第二个 adapter 也能进入执行，证明不同 workspace runtime blocks 不被全局串行化。
- graph 依赖与 evidence 顺序回归已复核：独立 graph nodes 可并发执行，依赖节点在前置节点完成后推进，follow-up 输入保持并行节点 evidence 的稳定顺序。
- TUI/Web runtime gap 展示链路已补齐：`runtimeExecution` 从 RunSnapshot / RunStatus 合并到 WorkspaceSnapshot，经 `workorchclient`、`workorchport`、remote/eventHub backend 透传；TUI 主视图、状态栏、`/status` 和 Web workspace detail 只显示 backend-projected pending/gap/worker/progress 字段，不新增端侧恢复状态机。
- OBS-01 第一轮已收口：`scripts/observe-workorch-runtime-closure.sh` 会采集 `/debug/pprof/goroutine?debug=2` 为 `pprof-goroutine.txt`，并在 profile 可用时检查 `dispatchGraphStepBatchWithWorkspace` / `dispatchRuntimeRepairFollowUp` 与 EventHub `Send` / `ExecuteRuntimeStep` 组合的递归 runtime dispatch 栈；pprof 不可用默认降级 warning，可通过 `WORKORCH_OBSERVE_REQUIRE_PPROF=1` 升级为失败；`verify-real-runtime-closure.sh` 默认设置 `WORKORCH_OBSERVE_REQUIRE_PPROF=1`，真实 runtime closure 必须具备 goroutine pprof 证据。
- PERF-01 第一轮已收口：`scripts/observe-workorch-runtime-closure.sh` 会采集 `/debug/pprof/profile?seconds=N` 为 `pprof-cpu.pb.gz`，通过 `go tool pprof -top` 生成 `pprof-cpu-top.txt`，并检查 projection pending rebuild、projection schedule dropped delta，以及 repeated event scan / stale recovery / recursive dispatch / projection rebuild 相关 CPU hotspot 是否超过阈值；pprof CPU 不可用默认 warning，可通过 `WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=1` 升级为失败；`verify-real-runtime-closure.sh` 默认设置 `WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=1`，真实 runtime closure 必须具备 CPU profile 证据。
- deployed-instance 长跑证据已覆盖两个 active workspace 并行、provider timeout failure terminal/finalize、CPU/IO 回落和 pprof gate；本地 compatible mock provider e2e 已覆盖 provider 正常返回后的 successful workspace usage / terminal step state 闭环。外部真实 provider 验证仍受部署环境 provider endpoint / credential / quota 影响，但不再是 runtime state loop 代码收口 blocker。
- `bootstrap.NewWithBackend*` 已补上 orchestrator `BackgroundRoutine` 依赖，`TopicOrchestratorCommandStartRun` 与 `TopicOrchestratorCommandResumeRun` 在本地 bootstrap 路径上已转为 background dispatch。
- `WithBackend` / default backend / server backend 生命周期第一轮缺口已完成收口：`StartRunWithBackend`、`ResumeRunWithBackend`、`ActivateWorkspaceExecutionWithBackend`、恢复 helper 已改为 shared backend session；`workorchd/service/server` shutdown 会释放对应 appservice shared backend session，避免 HTTP/TUI command 返回后异步 resume/start 仍写入已删除的 file backend；相关 appservice/server 回归测试已按“命令先返回 observable state，再等待 owner state 稳定”的异步合同更新。
- bootstrap local runtime shutdown 顺序已补齐：先 drain framework 注入的 `BackgroundRoutine`，再 teardown module，最后 terminate EventHub，避免后台 start/resume dispatch 在 owner observer 已解绑后出现 `...command returned no response`。
- decision ticket reopen/update 投影刷新链路已补齐：decision owner 新增 `/ticket/updated` notification，`TopicDecisionCommandUpdateTicket` 更新 owner repository 后发布该 notification；workorch event 聚合、observed snapshot contract、SSE enrichment 和 full stream subscription 均识别该 topic，避免 config-only dependency gate 重新打开 ticket 后 TUI/Web 仍展示旧 decision 状态。
- OBS/PERF 短窗口验证已补齐更细的 pprof 判定：`observe-workorch-runtime-closure.sh` 会独立报告 `runtimeAdapterInObserverLaneFound`、`synchronousRuntimeSendStackFound`、`recursiveDispatchStackFound` 及对应 stack 数量，区分“runtime adapter 卡在 observer lane”和“runtime Send bridge 同步等待 result”两类问题。
- 2026-06-18 对当前部署实例执行 3 秒 observation，artifact: `/tmp/workorch-observe-closure-20260618103858`。结果：pprof 可用，`runtimeAdapterInObserverLaneFound=false`、`synchronousRuntimeSendStackFound=false`、`recursiveDispatchStackFound=false`；CPU profile 可用且无 suspicious hotspot；projection pending rebuild 为 0，schedule dropped delta 为 0。该证据用于提前验证 pprof/CPU gate 可用；完整长跑证据见 `/tmp/workorch-real-runtime-closure-20260618104819` 和 `/tmp/workorch-observe-after-failed-real-20260618105812`。
- 2026-06-18 对当前部署实例执行完整 real runtime closure，artifact: `/tmp/workorch-real-runtime-closure-20260618104819`。两个 workspace（`ws-9560b6177f0b2200`、`ws-9b39ee4ab3a7ebbb`）均并发启动并推进到明确 `blocked_on_decision`，最终 blocker 为 `provider-retry-exhausted`；所有 LLM runtime steps 均已 terminal，最新 step 均带 `doneAt/failedAt/finalizedAt/finalizeEvent=step_failed`。失败根因是外部 provider endpoint `http://127.0.0.1:8090/v1/chat/completions` dial timeout，不是 runtime queue、EventHub observer lane 或 finalize gap。
- 2026-06-18 对上述失败 workspace 追加 observation，artifact: `/tmp/workorch-observe-after-failed-real-20260618105812`。结果：projection display contract pass，pprof 可用且 `runtimeAdapterInObserverLaneFound=false`、`synchronousRuntimeSendStackFound=false`、`recursiveDispatchStackFound=false`；CPU profile 可用且无 suspicious hotspot；projection pending rebuild 为 0，schedule dropped delta 为 0。
- 2026-06-18 对最新部署实例再次执行 5 秒 observation，artifact: `/tmp/workorch-observe-closure-20260618111447`。结果：service ready，projection state `ready`，workspace total 6，projection pending rebuild 为 0，schedule dropped delta 为 0；pprof 未发现 runtime adapter observer lane、sync Send bridge 或递归 dispatch stack；CPU profile 无 suspicious hotspot。
- 2026-06-18 对最新部署实例执行 `verify-llm-workspace-execution.sh`，使用本地 compatible mock provider `127.0.0.1:18181` 完成 happy path、decision、approval、multi-gate、schema failure、memory recall、streaming native usage 和 streaming estimated usage 场景。脚本已适配异步 activation：当 activation 响应只表示 accepted 时，轮询 workspace snapshot / runs 等待 run materialize，再继续断言 run status、snapshot、observability、usage 与 terminal state。
- 2026-06-18 最终收口审计再次对当前部署实例执行 `verify-llm-workspace-execution.sh` 并通过；随后执行 5 秒 observation，artifact: `/tmp/workorch-observe-final-20260618113540`。结果：service ready，projection state `ready`，workspace total 21，projection pending rebuild 为 0，schedule dropped delta 为 0；pprof 未发现 runtime adapter observer lane、sync Send bridge 或递归 dispatch stack；CPU profile 无 suspicious hotspot。
- 验证脚本补强：`loadtest-governed-workspace-concurrency.py` 在失败 workspace 上也会从 failure snapshot / observability 回填 workspace/run/progress state、LLM terminal/non-terminal step count 和 LLM usage trace/summary count；`verify-real-runtime-closure.sh` 在 loadtest 失败时不再直接退出，而是继续执行 observation 并生成 `real-runtime-closure-summary.json`，再统一返回失败。

### Phase 0 Todo: Scope Lock Before Code

在开始代码修改前，先完成以下文档与审计项：

- [x] 记录 live blocker：provider 已返回，但 step 保持 `dispatched`，stack 卡在同步 EventHub `Send` 主链路。
- [x] 明确 `Send` 的保留范围：短同步 owner command 可用，runtime long-running execution 不可用。
- [x] 明确 runtime execution queue、orchestrator state loop、watchdog recovery、projection/TUI 的责任边界。
- [x] 搜索并列出所有生产路径中的 `ExecuteRuntimeStep(` 调用点，区分 worker 内部接口和 orchestrator 主路。
- [x] 搜索并列出所有 runtime block execute topic 的 observer，确认是否有长耗时执行残留。
- [x] 搜索并列出所有 graph / repair / follow-up 递归 dispatch 调用点。
- [x] 搜索并列出 watchdog 对 dispatched step 的重派路径，标记是否可能重复调用 provider。
- [x] 搜索并列出 TUI/Web 端侧状态推断或补偿逻辑，确认是否需要删除。
- [x] 为 Phase 1 contract/lane 待改包补充最小单测清单，确保先有可证明的 contract test。
- [x] 为 Phase 2-6 待改包补充最小单测清单，确保先有失败样本或可证明的 contract test。
- [x] 更新本文件 Phase 2-6 中每个任务的 owner、测试命令和验收证据。

历史生产主路问题及当前收口状态：

- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go` 中 `dispatchStepWithWorkspace` 已在 event-driven 生产主路发布 execute request 并返回 accepted，不再同步等待 `StepExecutionResult`。
- `internal/modules/kernel/orchestrator/runtime_event_adapter.go` 中 `eventRuntimeAdapter.ExecuteRuntimeStep` 仅保留为测试/内部兼容桥；生产主路使用 `RuntimeExecutionRequester`。
- `internal/modules/blocks/pkg/runtimeevent/runtime_event.go` 中 `Executor.handle` 已收口为 decode、scope 校验、幂等去重、入队并返回 accepted；adapter 执行由 BackgroundRoutine worker 承载。
- `internal/modules/kernel/orchestrator/biz/runtime_graph.go` 中 graph batch 已改为发布 ready node dispatch request，不再通过 goroutine / `WaitGroup` 等待 batch terminal。
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go` 中 repair/provider retry 已通过 dispatch requested event 推进，不再在同一 Go 调用栈递归执行 runtime block。
- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go` 中 stale dispatched watchdog 已按 request/accepted/progress/done/finalize gap 分型，并通过 gap-specific owner recovery command 推进，不直接调用 provider/runtime adapter。

Phase 1 已落地的最小 contract/lane 收口：

- runtimeevent owner 新增 `accepted/progress/completed/failed` notification topic。
- `internal/pkg/workorchruntimecontract` 新增 runtime execution requested/accepted/progress/completed/failed DTO。
- `internal/pkg/events` 聚合 runtimeevent owner contract。
- `accepted` 事件 lane 固定为 `state/runtime-exec/workspace/<workspaceId>/run/<runId>/step/<stepId>`。
- `completed/failed` 事件 lane 固定为 `state/orchestrator/workspace/<workspaceId>/run/<runId>`。
- 已补单测覆盖 runtime execute request lane、accepted lane、completed/failed lane、跨 workspace runtime result lane 隔离。

建议审计命令：

```bash
rg -n "ExecuteRuntimeStep\\(" internal/modules internal/pkg -g "*.go"
rg -n "TopicRuntimeBlockCommandExecute|runtime/block/execute|NewSimpleObserver|Subscribe" internal/modules/blocks internal/modules/kernel internal/pkg -g "*.go"
rg -n "dispatchGraphStepBatchWithWorkspace|dispatchRuntimeRepairFollowUp|dispatchStepWithWorkspace|WaitGroup" internal/modules/kernel/orchestrator -g "*.go"
rg -n "recoverStaleDispatched|stale dispatched|step_dispatched|runtime_step_done|finalize" internal/modules/application internal/modules/kernel -g "*.go"
rg -n "decision >|Submitting|replan|restart|resume current run|workspace projection" internal/services/workorch/service/tui internal/modules/application/workorchd -g "*.go"
```

### Implementation Guardrails

- 先补 contract/lane/queue/finalize 单测，再替换生产主路。
- 每个 phase 只迁移一条权威路径；不要在同一次提交里同时改 prompt、watchdog、TUI 和 runtime queue。
- 每次删除同步路径前，必须有新的 accepted/progress/completed/failed event 证据链。
- 每次引入 worker 前，必须验证 shutdown 后拒绝新任务，且 worker context 可取消。
- 每次迁移 graph/repair/follow-up 前，必须证明 duplicate result event 不会重复 apply patch、写 memory、创建 decision/result/artifact。
- 每次改 projection/TUI 前，必须证明服务端 ViewModel 是唯一展示状态来源。
- 每次改 core 前，必须反向搜索业务词和具体命令，确认没有新增 template 业务特判。

## Send / Post Rule

`Send` 仍可用于短小、确定、必须同步返回的 owner read/write command，例如：

- workspace/run/step/ticket/approval/result 的 owner read command
- 需要立即拿到拒绝原因的状态机裁决 command
- 短同步查询和轻量 facade command

`Send` 不应再用于以下主链路：

- LLM provider call
- shell command
- MCP stdio / HTTP tool call
- file patch apply
- git write
- web fetch
- 任何执行完成后还会继续触发 graph/follow-up/repair 的 runtime block

这些 runtime block 应采用：

```text
orchestrator state loop
  -> publish runtime step execute requested event
  -> runtime execution queue accepts work item
  -> BackgroundRoutine worker executes block
  -> publish runtime step completed / failed event
  -> orchestrator state loop finalizes step
  -> publish next dispatch requested event when needed
```

## Target Event Model

### Runtime Execute Request

Owner: `internal/modules/blocks/pkg/runtimeevent`

Command topic:

- `TopicRuntimeBlockCommandExecute(kind)` 保留为 runtime block execute command family。

新增或固化 payload:

- `workspaceId`
- `runId`
- `stepId`
- `stepKind`
- `requestId`
- `attempt`
- `workspaceDeliverableDirectory`
- `workspaceInternalDirectory`
- `input`
- `progressPolicy`

语义：

- 该 topic 表达“请求执行一个 runtime step”。
- observer 只做 payload decode、scope 校验、幂等去重和入队。
- observer 不直接执行 block adapter。

### Runtime Execute Accepted

Owner: `internal/modules/blocks/pkg/runtimeevent`

Notification topic:

- `runtime/block/execute/accepted`

Payload:

- `workspaceId`
- `runId`
- `stepId`
- `stepKind`
- `requestId`
- `queueKey`
- `acceptedAt`

用途：

- orchestrator 可以记录 step 已被执行队列接受。
- projection / TUI / Web 只作为显示进展，不作为正式完成依据。

### Runtime Progress

Owner: `internal/modules/blocks/pkg/runtimeevent`

Notification topic:

- `runtime/block/execute/progress`

Payload:

- `workspaceId`
- `runId`
- `stepId`
- `stepKind`
- `event`
- `stage`
- `summary`
- `data`
- `occurredAt`

用途：

- 替代当前 `StepExecutionProgressFunc` 在同步调用栈内回调 orchestrator 的方式。
- orchestrator 可订阅并记录 runtime checkpoint。
- projection 可订阅并刷新 ViewModel。

### Runtime Completed / Failed

Owner: `internal/modules/blocks/pkg/runtimeevent`

Notification topics:

- `runtime/block/execute/completed`
- `runtime/block/execute/failed`

Payload:

- `workspaceId`
- `runId`
- `stepId`
- `stepKind`
- `requestId`
- `output`
- `error`
- `failureClass`
- `failureSignature`
- `llmCallRefs`
- `usage`
- `completedAt`

语义：

- completed / failed 是 runtime block 的执行结果事件。
- 只有 orchestrator 消费该事件并完成 step finalize 后，step 才能从 `dispatched` 转为 terminal step state。
- runtime block 不直接写 run state，也不创建 decision/approval/result。

### Orchestrator Step Finalized

Owner: `internal/modules/kernel/orchestrator/pkg/events`

Notification topics:

- `orchestrator/step/finalized`
- `orchestrator/dispatch/requested`

语义：

- step finalize 完成后，orchestrator 发布状态变化和下一步调度意图。
- graph continuation、follow-up、repair 不再通过 Go 函数递归调用，而是转为 dispatch request work item。

## Lane Policy

### Required Ordering

需要串行的 lane：

- 同一 `workspaceId + runId` 的 orchestrator state transition。
- 同一 `workspaceId + runId + stepId` 的 runtime result finalize。
- 同一 owner state write，例如 run state、decision、approval、result。

### Not Globally Serialized

不应全局串行的执行：

- 不同 workspace 的 runtime block execution。
- 同一 workspace 下不同无依赖 step 的 runtime block execution。
- projection rebuild 和 runtime block execution。
- TUI/Web display refresh 和 orchestrator state transition。

### Proposed Lanes

| Event | Lane |
| --- | --- |
| runtime execute request accepted | `state/runtime-exec/workspace/<workspaceId>/run/<runId>/step/<stepId>` |
| runtime worker execution | worker queue key, not EventHub synchronous lane |
| runtime progress notification | `topic/runtime/block/execute/progress` or scoped notify lane |
| runtime completed / failed | `state/orchestrator/workspace/<workspaceId>/run/<runId>` |
| orchestrator finalize step | `state/orchestrator/workspace/<workspaceId>/run/<runId>` |
| orchestrator dispatch requested | `state/orchestrator/workspace/<workspaceId>/run/<runId>` |
| runtime block internal provider call | no EventHub lane; worker-owned context |

原则：

- EventHub lane 保证 owner state 顺序，不承载长耗时执行本体。
- Runtime worker queue 自己管理执行并发、取消、幂等和超时。
- 同一个 run 的 finalize 顺序必须由 orchestrator state lane 保证。

## Target State Loop

### Start / Resume / Replan

```text
workorchd scheduler worker
  -> Send orchestrator start/resume/replan command
orchestrator
  -> create run/step state
  -> persist step dispatched
  -> publish runtime execute requested
runtime queue subscriber
  -> enqueue work item
runtime worker
  -> execute block adapter
  -> publish runtime completed/failed
orchestrator result subscriber
  -> finalize step
  -> publish dispatch requested when continuation exists
orchestrator dispatch subscriber
  -> create next step or graph batch
  -> publish runtime execute requested
```

### Graph Batch

```text
orchestrator dispatch subscriber
  -> calculate ready graph nodes
  -> create steps for ready nodes
  -> publish one execute request per step
runtime workers
  -> execute ready steps concurrently when dependencies allow
orchestrator result subscriber
  -> finalize each step
  -> after dependency satisfaction, publish next dispatch requested
```

Graph batch 不再用 goroutine + `WaitGroup` 同步等待所有 node；依赖满足由 persisted step state 和 graph state 计算。

### Runtime Repair

```text
orchestrator result subscriber receives failed step
  -> classify failure
  -> update coverage ledger / validation evidence
  -> decide recovery action
  -> if provider retry / runtime repair / follow-up needed:
       create repair step
       publish runtime execute requested
  -> if exhausted:
       create decision ticket or fail run
```

Repair 不再从 `handleRuntimeStepFailure` 直接递归调用 `dispatchRuntimeRepairFollowUp` 到 runtime block `Send`。

### Completion / Acceptance

```text
orchestrator result subscriber receives completed step
  -> validate output contract
  -> apply patch/revert through runtime result handling rules
  -> record memory/evidence/result/artifact
  -> update coverage ledger
  -> decide graph continuation / completion follow-up / acceptance
  -> publish next dispatch requested or terminal run state
```

如果 finalize 阶段失败，错误也必须落为 terminal step failure 或 run blocker；不能让 step 长期停留在 `dispatched`。

## Adjustment Scope

### `internal/modules/blocks/pkg/runtimeevent`

需要调整：

- 将 `Executor.handle` 从直接调用 `adapter.ExecuteRuntimeStep` 改为轻量入队。
- 增加 runtime execution queue / worker model。
- 增加 accepted/progress/completed/failed event payload contract。
- 支持 worker 并发、queue key、request id、attempt 和 shutdown。
- 使用 framework 注入的 `BackgroundRoutine`，不裸起长期 goroutine。
- 保留测试用同步 adapter 时，应只作为单元测试 helper，不作为生产主路。

待办：

- [x] 定义 runtime execute accepted/progress/completed/failed topic。
- [x] 定义 runtime execute work item DTO。
- [x] 定义 worker queue key：`workspaceId/runId/stepId/requestId`。
- [x] 为每个 block executor 接入 BackgroundRoutine。
- [x] 增加 shutdown 后拒绝入队的错误路径。
- [x] 增加重复 request 幂等测试。
- [x] 增加 worker 完成后发布 completed/failed event 测试。

### `internal/modules/blocks/*`

需要调整：

- block adapter 的 `ExecuteRuntimeStep` 可以保留为 worker 内部执行接口。
- block module `Run` 注册 runtimeevent executor，但 executor 不在 observer 内直接执行 adapter。
- LLM block 的 provider progress 从 callback 改为 runtime progress event。
- Shell/MCP/File/Git/Web/SSH block 输出保持结构化 result，不直接写 orchestrator state。

待办：

- [x] LLM progress 事件替换同步 `StepExecutionProgressFunc` 依赖。
- [x] MCP stdio 长连接执行纳入 worker context/cancel。
- [x] Shell timeout/cancel 与 worker context 对齐。
- [x] File/Git patch/apply 输出引用保持原有 evidence contract。
- [x] 所有 block 增加 executor queue 回归测试。

### `internal/modules/kernel/orchestrator`

需要调整：

- `RuntimeAdapter` 同步返回 result 的生产主路替换为 runtime execute request publisher。
- Start/resume/replan 只创建 step 并发布 execute request，不等待 block result。
- 增加 runtime completed/failed subscriber，作为唯一正式 finalize 入口。
- `dispatchGraphStepBatchWithWorkspace` 不再递归等待 batch 完成。
- `dispatchRuntimeRepairFollowUp` 不再直接调用 runtime block；改为创建 repair step + publish execute request。
- `handleRuntimeStepFailure` 保留分类和恢复决策，但由 result event 驱动。
- Step finalize 必须幂等：重复 completed/failed event 不能重复 apply patch 或重复创建 ticket/result。
- Provider stream done 但 step 未 finalize 的情况必须由 watchdog 判定为 finalize missing，并转入恢复路径。

待办：

- [x] 新增 orchestrator runtime result event subscriber。
- [x] 抽出 `finalizeRuntimeStepResult`，输入为 persisted step + runtime result event。
- [x] 抽出 runtime request publisher 行为，只负责 step dispatched 持久化和 execute request 发布；event-driven 模式缺 requester 时结构化失败，不同步 fallback。
- [x] 重构 `dispatchStepWithWorkspace`：event-driven 生产主路不再同步等待 `ExecuteRuntimeStep`。
- [x] 重构 `dispatchGraphStepBatchWithWorkspace`：按 ready nodes 发请求，返回 accepted，不等待 batch terminal。
- [x] 重构 `dispatchRuntimeRepairFollowUp`：只创建 repair/provider retry step 和执行请求。
- [x] 对 step terminal side effect 建幂等 guard：patch apply、memory write、result/artifact、decision/approval ticket。
- [x] 增加单测：provider completed event 后 step 从 dispatched 到 completed。
- [x] 增加单测：provider completed event 后 output contract invalid 转 failed/repair。
- [x] 增加单测：duplicate completed event 不重复 side effect。
- [x] 增加单测：graph batch 不递归同步等待，依赖满足后由 dispatch event 推进。
- [x] 增加单测：runtime failed event 触发 provider retry/runtime repair/decision exhaustion。

### `internal/modules/kernel/run`

需要核对：

- run owner 是否能表达 active step / dispatched step / terminal step 的权威状态。
- stale dispatched step 的 owner state 是否足够让 orchestrator 重建 execution request。

待办：

- [x] 核对 run/step owner write command 是否携带 workspace scope。
- [x] 增加 stale dispatched step metadata：last request id、last accepted at、last progress at。
- [x] 增加 run snapshot 对 pending runtime execution 的显示字段。

### `internal/modules/application/workorchd/service/server`

需要调整：

- 现有 runContinuationScheduler 继续负责用户 command / governance continuation / restart / activation 的 pending queue。
- Runtime block execution queue 不应放在 HTTP layer；它属于 runtimeevent executor 或 orchestrator runtime execution subsystem。
- Watchdog 需要从“重派同一个 step”改为“检查 execution request/result/finalize 缺口”，避免重复触碰同一 dispatched step。

待办：

- [x] 将 stale dispatched recovery 分类为：request not accepted、accepted no progress、stream done but not finalized、worker lost。
- [x] 对 `stream done but not finalized` 优先发布 finalize recovery command，而不是重复 dispatch step。
- [x] HTTP handler 不直接驱动 runtime block；只提交 user command。
- [x] projection 中展示 pending/finalize-missing 状态。

### `internal/modules/application/workorchd/biz/appservice`

需要调整：

- appservice 继续作为 facade/read model 组合层。
- 不承载 runtime block execution 队列本体。
- Snapshot/read model 应展示 runtime event 状态，不重新推导正式状态机。

待办：

- [x] Run snapshot DTO 增加 runtime execution queue / finalize gap 摘要。
- [x] Read model 从 orchestrator/run owner events 聚合 step pending/progress/finalize 状态。

### `internal/pkg/events`

需要调整：

- 保持 topic owner contract 聚合，不在基础包定义业务 topic。
- Lane policy 继续由 owner `pkg/events` 声明。
- Execute lane 只用于 execute request accepted/fan-out，不应承载长耗时执行本体。

待办：

- [x] 增加 runtime execute topic 的 lane contract 测试。
- [x] 增加 runtime completed/failed 进入 orchestrator state lane 测试。
- [x] 增加跨 workspace execute request 不互相阻塞测试。

### `internal/pkg/workorchruntimecontract`

需要调整：

- 保留通用 `StepExecutionRequest` / `StepExecutionResult` contract。
- 增加 event payload DTO 时应放在通用 runtime contract 或 runtimeevent package，避免反向依赖 owner model。

待办：

- [x] 定义 `RuntimeStepExecutionRequested`。
- [x] 定义 `RuntimeStepExecutionAccepted`。
- [x] 定义 `RuntimeStepExecutionProgressed`。
- [x] 定义 `RuntimeStepExecutionCompleted`。
- [x] 定义 `RuntimeStepExecutionFailed`。

### `internal/services/workorch/service/tui` / Web

需要调整：

- 端侧仍只消费 projection / snapshot / event stream。
- TUI/Web 不根据 runtime progress 自行推进 run state。
- 对 running 但 finalize missing 的状态，显示服务端 ViewModel 提供的 stalled/finalize gap 信息。

待办：

- [x] TUI 状态栏展示 backend-projected finalize gap。
- [x] Web detail 展示 backend-projected runtime execution pending/finalize gap。
- [x] 不新增端侧补偿重试逻辑。

## Migration Phases

### Phase 1: Contract And Tests First

- [x] 定义 runtime execution event contract。
- [x] 定义 queue work item contract。
- [x] 补 EventHub lane tests。
- [x] 补 orchestrator result event finalize tests。
- [x] 不改 block adapter 行为，只补接口和测试 scaffold。

验收：

- [x] 新 topic 全部位于 runtimeevent owner package。
- [x] `internal/pkg/events` 只聚合 owner contract 和 lane policy，不定义 runtimeevent topic 字符串。
- [x] 单测证明 execute request、accepted、completed/failed event lane 符合预期。
- [x] 单测证明 orchestrator result event 可进入 finalize 入口并保持幂等。

### Phase 2: Runtimeevent Queue

- [x] block modules 和 bootstrap local runtime 显式接入 framework / runtime 注入的 BackgroundRoutine。
- [x] `runtimeevent.Executor` 构造契约持有 BackgroundRoutine，后续 queue worker 不再需要 block 自建 goroutine。
- [x] `runtimeevent.Executor` observer 改为轻量入队，并以单测证明不会等待 blocking adapter。
- [x] Worker 使用 BackgroundRoutine 执行 adapter，worker context、cancel、shutdown 语义明确。
- [x] Worker 发布 accepted/progress/completed/failed，且 publish failure 不吞掉可诊断信息。
- [x] 保留同步测试 helper，但生产路径不再在 observer 内执行长耗时 adapter。
- [x] 明确 duplicate request 生命周期：同一 request id 幂等；新 attempt 必须使用新 request id。

验收：

- [x] block adapter 单测仍通过。
- [x] runtimeevent executor 单测覆盖 enqueue、duplicate、shutdown、completed/failed。
- [x] runtimeevent executor 单测覆盖 progress event 和 publish failure 诊断。
- [x] goroutine stack 不再显示 runtime block adapter 在 EventHub observer lane 中长时间执行。
  - 2026-06-18 短窗口和 real runtime closure 后置 observation 均已通过该 gate；未发现 runtime block adapter 在 EventHub observer lane 中长时间执行。

### Phase 3: Orchestrator Finalize Subscriber

- [x] Orchestrator 订阅 runtime completed/failed。
- [x] 将 step finalize 从 `dispatchStepWithWorkspace` 拆出。
- [x] 执行结果事件驱动 step terminal state、evidence、patch、coverage ledger、decision/approval/result。
- [x] 过渡桥 `eventRuntimeAdapter.ExecuteRuntimeStep` 退出生产主路，保留时只能用于测试或兼容内部 adapter；event-driven 缺 requester 不再同步 fallback。
- [x] `WithBackend` / default backend / shared session 生命周期与异步 dispatch 语义对齐，避免 start/resume 返回后销毁 runtime。

验收：

- [x] provider completed event 后 step terminal。
- [x] provider failed event 后恢复路径正确。
- [x] duplicate result event 基础幂等。
- [x] output contract failure 不会让 step 停在 `dispatched`。

### Phase 4: Remove Recursive Dispatch Mainline

- [x] `dispatchStepWithWorkspace` 在 event-driven 生产主路只创建 step 和发布 execute request；未启用 event-driven 的同步 fallback 仅保留给测试/内部 adapter。
- [x] graph batch 只发布 ready nodes dispatch request。
- [x] follow-up / repair / completion continuation 通过 dispatch requested event 推进。
- [x] 去除 event-driven 主路中同步 `WaitGroup` 等待 graph batch terminal 的行为。
- [x] 所有 continuation 事件携带 workspace/run/step scope 和幂等 key。

验收：

- [x] goroutine stack 不再出现深层 `dispatchGraphStepBatchWithWorkspace` / `dispatchRuntimeRepairFollowUp` 递归。
  - 2026-06-18 短窗口和 real runtime closure 后置 observation 均已通过该 gate；未发现递归 dispatch stack。
- [x] 多 workspace 并行执行时不同 workspace 的 runtime blocks 可并行。
- [x] 同一 run 内依赖顺序仍正确。

### Phase 5: Watchdog And Recovery

- [x] RunSnapshot / RunStatus read model 可展示 request accepted、progress、done、finalize 的不同缺口。
- [x] Watchdog 识别 request accepted、progress、done、finalize 的不同缺口。
  - [x] stale dispatched selector 已识别 `runtime_terminal_not_finalized`。
  - [x] 有 durable terminal result 时触发 resume finalize recovery；只有 stream done 没有 durable result 时阻止 provider 重派。
  - [x] request not accepted / accepted no progress / accepted in progress / worker lost 已有独立恢复策略和回归：request not accepted 只作为诊断 gap，不由 periodic watchdog 自动恢复；accepted/progress/worker lost 必须同时具备持久边界证据和显式可重试 recovery policy，才允许结构化 recovery command。
- [x] 对 `stream done but not finalized` 执行 finalize recovery。
  - completed/failed event 会先写 `runtime_step_result_available` durable checkpoint。
  - `ResumeRun` 在 running recovery 中优先使用 durable terminal result finalize 当前 dispatched terminal step。
  - 仅有 stream done 但没有 durable result 时仍不能伪造 output，watchdog 只展示 gap，不重派 provider。
- [x] 对显式 recoverable 的 worker lost 或 accepted gap 执行 retry/requeue；unknown、manual-only 或 non-idempotent policy 不自动重试。
- [x] 服务启动不自动从 persisted owner state 重建 pending；历史运行态恢复由显式用户命令或诊断/恢复入口触发。
- [x] Watchdog 不直接调用 provider 或 runtime adapter；只发布恢复 command/work item。

验收：

- [x] 服务重启后 dispatched step 可恢复。
- [x] provider response 已落盘但 step 未 terminal 时，不重复调用 provider，优先完成 finalize。
- [x] stale dispatched 不再无限重复 `step_dispatched`。

### Phase 6: E2E And Runtime Observation

- [x] 更新 deployed-instance e2e。
- [x] 增加真实 provider 场景验证。
- [x] 增加 pprof goroutine 递归 dispatch 栈观测 gate。
- [x] 增加 pprof CPU / metrics 观测 gate。
- [x] 增加多 workspace 并行运行观测：至少两个 active workspace 同时推进。

收口进展：

- `verify-deployed-full` 已纳入 `verify-real-runtime-closure`，部署全量验证会执行真实 provider 并行闭环与 runtime observation。
- `loadtest-governed-workspace-concurrency.py` 的 summary 已输出每个 workspace 的 `workspaceState`、`runState`、`runStatusState`、`progressState`、`progressTerminal`、LLM step terminal 计数和 LLM usage trace/summary 计数。
- `loadtest-governed-workspace-concurrency.py` 已兼容异步 activation 返回：activation 只返回 accepted 时，会通过 workspace snapshot / run list 等待 active run materialize 后再进入状态驱动。
- `loadtest-governed-workspace-concurrency.py` 已把 `provider-retry-exhausted` 识别为真实 runtime closure 失败证据；真实 provider 场景不得把 provider 配置/外部依赖恢复类 blocker 自动 resolve 成无限 replan/resume 循环。
- `loadtest-governed-workspace-concurrency.py` 已在 failure evidence path 回填 failed workspace 的 workspace/run/progress terminal state、LLM step terminal/non-terminal counts、LLM usage trace/summary counts，避免真实 provider 失败时 summary 丢失 runtime closure 证据。
- Orchestrator resume 已补充 config-only dependency gate：resolved decision 未声明 `externalDependencyRecovered` / `systemConfigChanged` / `runtimeDependencyRecovered` 时保持 blocked，不重新调度 provider retry，也不重复创建同类 provider blocker ticket。
- `verify-real-runtime-closure.sh` 已对 successful workspace 增加硬断言：completed projection、terminal progress、LLM terminal step、usage trace 非空且 summary 计数一致、并行 overlap 通过。
- `verify-real-runtime-closure.sh` 默认要求 observation 阶段必须采集 goroutine pprof 和 CPU profile；pprof 不可用、递归 dispatch stack 命中、CPU hotspot 超阈值都会让真实 runtime closure 失败。
- `verify-real-runtime-closure.sh` 已改为 loadtest 失败后仍继续执行 observation，保证 provider 不可用、runtime blocker、projection/CPU/pprof 状态都有统一 summary 证据。
- 当前部署实例已完成 real runtime closure 观测：两个 workspace 并发推进到明确 `blocked_on_decision`，失败根因为外部 provider endpoint dial timeout；后置 observation 未发现 runtime observer lane、sync Send bridge、递归 dispatch stack 或 CPU hotspot。
- 以上为验证脚本能力、部署实例 provider-failure 闭环证据，以及本地 compatible mock provider 正常返回后的 successful workspace usage / terminal step state 闭环证据。

验收：

- [x] 两个 workspace 同时运行时都能持续推进或明确 blocked/failed。
  - 2026-06-18 `ws-9560b6177f0b2200`、`ws-9b39ee4ab3a7ebbb` 并发启动，最终都明确进入 `blocked_on_decision`，blocker 为 `provider-retry-exhausted`。
- [x] LLM provider 返回后一定出现 usage 和 terminal step state。
  - 2026-06-18 compatible mock provider e2e 已证明 provider 正常返回时，happy path、governance、memory recall 和 streaming usage 场景均出现 run terminal state、observability terminal marker、LLM usage trace/summary 或 estimated usage。外部真实 provider 失败样本为 endpoint dial timeout，属于环境不可用，不改变该代码路径验收结论。
- [x] 观测脚本具备 CPU/IO 门禁：projection pending rebuild、schedule dropped delta、CPU hotspot 和递归 dispatch stack 可形成失败证据。
- [x] 部署实例长跑证明 CPU/IO 不因重复扫描、重复 dispatch 或深递归而持续升高。
  - 2026-06-18 real runtime closure 运行约 344 秒后追加 pprof/CPU observation，未发现 recursive dispatch / runtime Send / observer lane stack，CPU profile 无 suspicious hotspot，projection pending rebuild 和 schedule dropped delta 均为 0。
- [x] TUI prompt、主视图 tail、状态栏、inspect 和 blocker submit/fetch 以当前 workspace/run scoped projection/ViewModel 为准，不让旧本地 blocker 缓存覆盖 running projection。
- [x] TUI/Web 显示 backend-projected runtime finalize/pending gap，不形成端侧恢复状态机。

## Acceptance Criteria

- Runtime block 长耗时执行不再通过 EventHub `Send` 在 observer lane 中同步完成。
- `orchestrator.dispatchStepWithWorkspace` 不再同步等待 `StepExecutionResult`。
- Provider stream done 后，step 必须进入 completed/failed，或产生明确 finalize recovery state；不能无限保持 `dispatched`。
- Graph/follow-up/repair 不再通过递归 Go 调用链推进。
- Runtime result event finalize 幂等。
- Watchdog 不再把“已完成 provider call 但未 finalize”的 step 当作普通未执行 step 重派。
- EventHub lane 只用于 owner state 顺序和轻量 command，不承载长耗时 block 执行。
- 所有新增 runtime execution contract 保持业务中立。
- Web/TUI 不复制 runtime recovery 逻辑。

## Verification Plan

代码级：

```bash
go test ./internal/modules/blocks/pkg/runtimeevent -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/pkg/events -count=1
go test ./internal/modules/application/workorchd/service/server -run 'Continuation|Stale|Recovery|Runtime' -count=1 -timeout=240s
```

全量：

```bash
go test ./... -count=1
go build -o ./bin/workorch ./application/workorch
```

部署实例：

```bash
./scripts/observe-workorch-runtime-closure.sh
./scripts/verify-real-runtime-closure.sh
make verify-deployed-full
./scripts/verify-workorch-managed-full.sh
```

运行期观测：

- pprof goroutine 中不应出现深层递归 dispatch stack。
- pprof CPU 中不应持续由 repeated event scan / stale recovery 占用。
- active run 的 current step 如果已有 provider done checkpoint，应在有限时间内 terminal。
- workspace projection 显示 runtime queue / finalize gap 状态。

## Open Questions

1. Runtime execution queue 是否归 `blocks/pkg/runtimeevent` 还是单独 `kernel/runtimeexec`？
   - 推荐：短期归 `blocks/pkg/runtimeevent`，因为它是 block execute topic owner；orchestrator 只订阅 completed/failed。

2. Runtime result event 的 payload 是否直接携带完整 `StepExecutionResult.Output`？
   - 推荐：携带 bounded output + raw refs；大对象保持现有 evidence/raw content 引用规则。

3. Patch apply 属于 block worker 还是 orchestrator finalize？
   - 当前实现中 patch apply 在 orchestrator 处理 LLM output 后执行，LLM graph 中显式 patch apply / preview / revert runtime step 已经通过 orchestrator 内的 patch `runtimeevent.Executor` 异步执行。长期应保持 orchestrator finalize 负责“是否应用”和 evidence 绑定，技术执行不进入 EventHub observer 同步 lane，并继续防止重复 side effect。

4. 是否保留同步 runtime adapter 接口？
   - 推荐：保留为 worker 内部接口和单元测试接口；生产 orchestrator 不直接调用同步 runtime adapter。

5. Existing in-memory pending queue 是否需要持久化？
   - 推荐：短期通过 owner state + execution request evidence 重建；长期如需要强审计，再引入 scheduler/runtime execution work item owner。

## Review Checklist

- 搜索同步 runtime 主路：
  - `ExecuteRuntimeStep(`
  - `MagicHub.Publish(`
  - `event.Hub.Send`
  - `dispatchGraphStepBatchWithWorkspace`
  - `dispatchRuntimeRepairFollowUp`
- 搜索重复恢复和重派：
  - `recoverStaleDispatchedRuns`
  - `step_dispatched`
  - `runtime_step_done`
  - `provider stream done`
- 确认新增 topic 位于 owner `pkg/events`。
- 确认 observer 不执行长耗时 adapter。
- 确认 BackgroundRoutine 缺失时启动失败，不创建裸 goroutine fallback。
- 确认 runtime result finalize 幂等。
- 确认没有新增具体模板、语言、命令、业务实体特判。
