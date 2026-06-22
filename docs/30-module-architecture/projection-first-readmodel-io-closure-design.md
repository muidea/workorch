# Projection-First Read Model And IO Closure Design

Status: active

Type: module-architecture

Last Updated: 2026-06-12

## Purpose

本文根据长期运行后 CPU/IO 高负荷必然复现的核对结果，固化
`workorchd` 的长期收口设计。

目标是把高频展示读路径从“运行期反复扫描事实日志和大 payload”调整为
“服务端维护权威 ViewModel projection，Web/TUI 只读取投影并订阅刷新信号”。

本文用于后续代码收口和验收，当前阶段不考虑兼容性。

## Confirmed Problem

长期执行多个 workspace 后，系统会进入持续高 CPU/IO 负载，甚至导致机器无响应。

已确认根因不是单纯的 LLM provider 慢或 TUI 刷新频率问题，而是读路径存在确定性放大：

- `EventRecordRepository.List` 历史上默认读取 event metadata 后继续 hydrate 大 payload。
- event payload 中包含 LLM request/response、execution graph、checkpoint、artifact/patch 等大对象。
- TUI/Web/workspace list/status/snapshot/watchdog 等高频读路径会反复触发事件列表读取。
- workspace projection/list 读取存在无界或近似无界的全局事件扫描。
- 随着事件总量、payload 体积、workspace 数和执行轮次增加，CPU、GC 和磁盘 IO 必然线性或超线性放大。

合理的文件写入包括：

- 正式状态变更持久化。
- append-only event metadata 写入。
- 大 payload 以 ref 方式单独落盘。
- run/step/result/artifact 等 itemized evidence 写入。

不合理的运行期行为包括：

- 高频展示请求扫描全量 event history。
- 默认读取 event payload 正文。
- TUI/Web 为刷新主视图触发重型 snapshot fan-out。
- watchdog/recovery 为轻量判断间接触发 heavy read-model assembly。
- projection rebuild 缺少有界 query、去重和 backpressure。

## Design Goals

长期目标：

- `workorchd` 是 workspace ViewModel projection 的服务端 owner。
- Web/TUI/CLI 主视图只读取服务端整理后的 projection。
- EventRecord 是事实日志，不作为高频实时视图查询表。
- 大 payload 默认 ref-only，只有显式 raw/detail/replay 路径才读取正文。
- workspace list/detail、status bar、prompt hint、SSE refresh 都不能触发全局历史扫描。
- watchdog、manual recovery、projection gap repair 使用轻量索引或 projection，不使用 heavy snapshot。
- file backend 长期运行时 CPU/IO 不应随历史 event payload 体积线性增长。
- 设计保持业务中立，不根据 workspace template、目标文件、测试框架或具体业务语义做分支。

非目标：

- projection 不替代 kernel owner module 的正式状态。
- projection 不直接执行状态机转换。
- Web/TUI 不承担服务端状态补偿。
- 不把 LLM/provider/MCP 等业务能力硬编码进通用 read-model 基础设施。

## Target Architecture

目标读写结构：

```text
                 kernel owner modules
       workspace/run/decision/approval/result/...
                         |
                  formal repository
                         |
              append-only event metadata
                         |
              payload refs / raw payload files
                         |
                 workorchd projection
       workspace overview/detail + run heartbeat
                         |
        REST snapshot/list + SSE projection changed
                         |
                    Web / TUI View
```

核心边界：

| Layer | Responsibility |
| --- | --- |
| Model owner | 正式状态、状态机、业务 command/notification event |
| EventRecord repository | 事实日志 metadata、可选 payload ref、显式 payload 打开 |
| Projection manager | 启动 baseline、事件增量投影、gap repair、degraded 状态 |
| Appservice read ports | projection 查询、detail/raw 显式查询、分页查询 |
| Server presenter | REST/SSE 参数解析与输出，不做重型聚合 |
| Web/TUI | 展示 projection、输入 command、订阅刷新，不聚合正式状态 |

## Read Contract

### EventRecord List

`EventRecordRepository.List` 的默认 contract：

- 默认不 hydrate payload。
- 返回 metadata、payload summary、payload ref。
- 必须支持 `Limit`。
- 支持 workspace/run/topic/subject/cursor/time filter。
- 对高频调用方，必须显式传入 workspace/run scope 和 limit。

只有以下路径允许 `IncludePayload=true`：

- SSE persisted replay 需要恢复完整 envelope。
- LLM usage trace 需要读取 usage payload。
- run observability / timeline drill-down 需要从事件 payload 派生诊断进展。
- 显式 raw/detail/event inspection API。
- 测试中明确验证 payload round-trip。

禁止：

- workspace list/detail 默认 IncludePayload。
- TUI/Web 主视图 IncludePayload。
- watchdog/recovery IncludePayload。
- 无 query scope 的 IncludePayload。

### Workspace List

workspace list 是高频主视图入口，必须满足：

- 只读取 projection store 或 projection index。
- 不扫描全局 event history。
- 不 hydrate payload。
- 不为每个 workspace 执行 heavy snapshot assembly。
- 可返回 degraded/stale projection，但必须暴露状态字段。

如果 projection 不 ready：

- 返回 projection state、stale reason、last built time。
- 允许 detail 后台 warmup。
- 不在 request path 内同步 full rebuild。

### Workspace Detail

workspace detail 应读取 projection detail：

- 默认只包含 compact task progress、governance、run heartbeat、summary evidence。
- runtime events 只保留最新有限数量 metadata。
- LLM usage 只保留 compact summary 和最近有限 trace。
- historical runs 只保留 active run 与最近有限 run，且不返回 run `ExecutionContext`。
- workspace `ContextHints` / `EvidencePolicies` 不在默认 detail projection 中返回。
- full event/raw payload 通过 detail/raw endpoint 打开。

禁止：

- detail 每次请求重新全量扫描 run events。
- detail 默认返回完整 eventRecords payload。
- detail 默认返回完整 checkpoint / graph / LLM response body。

### Run Detail And Observability

run snapshot/observability 是显式 drill-down，不属于 workspace 主视图自动刷新路径。

要求：

- 使用 workspace scoped run ID。
- 默认分页读取 step/tool-call/checkpoint/result/artifact/event。
- 默认 compact summary。
- raw body 只能通过受控 ref 打开。

## Write Contract

写入路径保持 append/itemized，不因 projection 优化改变正式状态写入语义：

- owner module 写正式 repository。
- owner module 发布 notification event。
- eventstream 追加 event metadata。
- 大 payload 写 payload ref。
- projection manager 订阅 event 后异步更新投影。

projection 写入只能写 projection store / projection cache，不写正式 owner repository。

## Projection Store Contract

projection store 需要按 workspace entry 统一管理：

- overview projection
- detail projection
- active/projection run id
- revision
- source event revision
- projected source revision
- updatedAt
- rebuiltAt
- stale/degraded/failed state
- stale reason / last error
- run heartbeat
- pending rebuild / in-flight rebuild metadata

同一 workspace 的上述运行期状态不得拆散在多张长期 map 中维护。

projection store 可以是内存优先，后续可增加持久化快照：

- 内存 projection 是服务运行期权威展示源。
- 持久化 projection snapshot 只用于快速启动 baseline，不是正式状态。
- projection snapshot 可由 Model + EventRecord 重建。

## Index And Query Direction

为了避免 file backend 运行期全表扫描，需要逐步补齐轻量索引。

### Required Indexes

至少需要以下索引或等价查询能力：

| Index | Purpose |
| --- | --- |
| workspace -> latest event metadata | projection baseline / workspace list |
| workspace/run -> latest event metadata | run heartbeat / detail refresh |
| workspace/run/topic -> paged event metadata | runtime timeline / observability |
| event id -> payload ref | explicit raw/detail open |
| workspace -> active/projection run | workspace overview/detail |
| workspace/run -> latest step/checkpoint/result/artifact summary | heartbeat / drill-down summary |

索引可以来自：

- file backend marker / segment metadata。
- repository 内部轻量 index。
- projection baseline cache。
- 启动时一次性 compact rebuild。

禁止把“大 payload 正文”放入索引。

### Event Segment Rules

append-only event segment 应保持：

- metadata segment 与 payload file 分离。
- metadata segment 尽量小而稳定。
- payload ref 可读且受控。
- segment scan 可以按 workspace/run/topic 快速缩小范围。

如果当前 file backend 暂时无法做到物理索引，必须先在上层查询收口：

- 所有高频 query 必须带 scope 和 limit。
- 禁止空 query 用于主视图。
- full scan 只允许在启动 baseline、维护任务或显式管理命令中出现。

## Projection Update Flow

事件驱动更新规则：

1. owner module 完成正式状态变更。
2. owner module 发布 notification event。
3. eventstream append metadata/payload ref。
4. projection manager observer 只做轻量分类。
5. observer 把 rebuild/patch task 提交给 framework `BackgroundRoutine`。
6. projection task 按 workspace 去重、限流、合并。
7. projection store 更新 revision 和 status。
8. workorchd 发布 projection changed event。
9. Web/TUI 收到 SSE 后读取 projection。

observer 禁止：

- 直接执行 heavy snapshot。
- 直接读取 raw payload。
- 长时间阻塞 EventHub lane。
- 为不同 workspace 使用不必要的全局 lane 串行化。

## Watchdog And Recovery Boundary

watchdog 是通用运行期健康检查，不是业务修复器。

允许读取：

- projection run heartbeat。
- run state compact metadata。
- latest step/checkpoint compact metadata。
- governance blocker compact state。
- stale dispatched / resolved governance blocker / continuation checkpoint summary。

禁止读取：

- 完整 workspace snapshot。
- 完整 event payload。
- 全量 LLM response/request body。
- Web/TUI display-only state。

watchdog 触发动作必须继续通过 appservice/kernel runtime contract：

- resume run
- recover resolved governance blocker
- mark generic runtime failure
- schedule projection repair

不得直接改写正式状态机。

Scheduler continuation 另有一条独立边界：REST/TUI/CLI 命令必须通过
workorchd scheduler command 主路径进入 `BackgroundRoutine` 后才算 accepted。
missing subscriber、missing response、not accepted、enqueue failure 或 owner
command failure 必须表现为同步错误或 bounded scheduler diagnostic。周期 watchdog
不得扫描 resolved decision/approval 后自动创建 replan continuation，也不得用
stale runtime gap recovery 补偿 scheduler command/result 丢失。`/readyz`、
workspace/run snapshot 和 `/metrics` 可以暴露 scheduler diagnostics 与建议的
manual action，但这些 diagnostics 不属于 kernel owner state，也不能触发自动
activation/resume/replan。

## Web/TUI Contract

Web/TUI 只做 View + Presenter：

- 启动读取 workspace projection list。
- 切换 workspace 读取 workspace projection detail。
- 收到 projection changed SSE 后按 workspace scope 刷新。
- prompt/status/main view 均来自当前 workspace projection。
- command 提交通过 backend port / REST API，不修改本地正式状态。

Web/TUI 禁止：

- 自行扫描所有 workspace。
- 自行组合 run/goal/dod/decision/approval/result 正式状态。
- 为了刷新显示读取 event raw payload。
- 在 projection not ready 时 fallback 到 heavy fan-out。

## Migration Plan

### Phase 1: Stop Payload Amplification

目标：

- `EventRecordRepository.List` 默认不 hydrate payload。
- `IncludePayload=true` 只保留在 explicit replay/detail/raw/LLM usage 路径。
- HTTP `/events` 默认不返回 payload。
- TUI/Web 主视图不请求 payload。

验收：

- repository contract 测试覆盖默认 payload omitted。
- explicit payload 测试覆盖 `IncludePayload=true`。
- run observability 使用独立 payload opt-in query，避免恢复 run event 默认重 payload。
- pprof 中 `readEventPayload/json.Unmarshal` 不再出现在 TUI idle/status 主热路径。

### Phase 2: Remove Global Event Scans From Main Views

目标：

- workspace list 不使用 `EventRecordQuery{}`。
- workspace list/detail 读取 projection store。
- projection baseline/rebuild 内部查询也必须按 workspace/run/topic/limit 收口。
- status bar/prompt hint 只依赖当前 workspace projection。

验收：

- 静态检查主视图路径无空 event query。
- workspace snapshot HTTP endpoint 返回 compact projection：历史 runs 有界、LLM usage trace 有界、LLM context budget 有界、memory recall trace 有界、execution section 有界、context bundle 有界、run execution context 省略、summary 保持完整。
- TUI 切换 workspace 不触发全局 event scan。
- Web workspace board refresh 不触发 heavy snapshot fan-out。

2026-06-18 核对补充：

- deployed instance 显示 projection ready、pending rebuild 为 0、run queue 为 0，但两个 workspace 已处于 terminal failed，说明当前不是 projection lag 或 active runtime hang。
- heap pprof 显示主要驻留来自 workspace detail projection、observed event collector、file step full-run cache 和 presenter/resource map clone。
- `observed.Collector` 不能在生产期无界保存所有 WorkOrch event；只能保留最近窗口，完整审计以 eventstream owner repository 为准。
- `workspaceviewcore.Store` 常驻的是服务端 ViewModel projection，不应承担完整审计明细缓存；detail projection 必须保持 compact，raw context/event/observability 通过显式 endpoint 按需读取。

### Phase 3: Projection Index And Backpressure

目标：

- projection manager 维护 per-workspace rebuild entry。
- rebuild task 去重、合并、限流。
- 不同 workspace 的无共享状态 rebuild 可并行。
- 同一 workspace 的 rebuild 保持顺序。
- projection metrics 暴露 pending、dropped、duration、stale count。

验收：

- 多 workspace 并行执行时 projection pending 不持续增长。
- `workorch_run_requests_in_flight=0` 且无新事件时 CPU 回落到稳定低位。
- projection rebuild duration 与 event history 总 payload size 无明显相关。

### Phase 4: File Backend Lightweight Index

目标：

- event metadata 查询支持 workspace/run/topic 有界读取。
- latest event / latest run heartbeat 不依赖扫描全部 segment。
- payload file 只在 explicit open 时读取。
- large event segment 支持 rotation 与 cursor。

验收：

- 长历史 workspace 下 workspace list 查询耗时稳定。
- event list page 查询按 limit 返回。
- raw/detail 打开大 payload 时只影响该请求，不影响主视图。

### Phase 5: Persisted Projection Snapshot

目标：

- projection store 可定期保存 compact snapshot。
- 启动时先加载 compact projection snapshot，再进行后台 gap repair。
- readyz 能表达 projection baseline/stale/degraded。

验收：

- 大历史数据集启动不会同步扫描所有 payload。
- projection 可服务时间与历史 payload 总量解耦。
- gap repair 期间 Web/TUI 可展示 degraded projection。

## Verification Plan

### Static Verification

检查项：

- 高频路径不存在 `EventRecordQuery{}`。
- 高频路径不存在 `IncludePayload=true`。
- TUI/Web backend 不直接调用 raw event payload。
- watchdog/recovery 不调用 heavy snapshot assembly。
- server handler 不直接做 full rebuild。

建议命令：

```bash
rg "EventRecordQuery\\{\\}" internal
rg "IncludePayload:\\s*true" internal
rg "ListProjectedWorkspaces|Snapshot|ListEvents" internal/modules/application internal/services
```

### Unit And Integration Tests

至少覆盖：

- repository default list omits payload。
- explicit include payload works。
- workspace list uses projection / scoped bounded event query。
- projection changed SSE carries workspace scope and revision。
- TUI workspace switch consumes projection detail。
- watchdog uses compact state and does not hydrate payload。

### Runtime Verification

长期运行验证必须采集：

- CPU percent
- RSS
- GC pause / heap
- disk read/write bytes
- event payload hydrate count
- event metadata scan count
- projection rebuild count/duration
- projection pending/dropped
- workspace list/detail latency
- `/metrics` idle baseline

验收标准：

- 多轮执行后，当 `run_requests_in_flight=0` 且无新 projection task 时，CPU 回落。
- TUI idle 不持续触发磁盘读。
- Web/TUI workspace switch 不触发 payload hydrate。
- 历史 payload 总量增长不会导致 workspace list/detail 成本同比增长。

## Code Areas To Close

优先核对以下路径：

- `internal/pkg/repository/event_record_repository.go`
- `internal/pkg/repository/file/file.go`
- `internal/pkg/repository/memory/memory.go`
- `internal/pkg/workorchrepo`
- `internal/modules/kernel/eventstream`
- `internal/modules/application/workorchd/biz/appservice/readmodelcore`
- `internal/modules/application/workorchd/biz/appservice/readmodelports`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore`
- `internal/modules/application/workorchd/pkg/observed`
- `internal/modules/application/workorchd/service/server`
- `internal/services/workorch/biz/backends`
- `internal/services/workorch/service/tui`
- `web/workorch-console`
- `scripts/observe-workorch-runtime-closure.sh`
- `scripts/verify-workorch-managed-full.sh`

## Acceptance Checklist

- [x] Event list default is metadata-only.
- [x] Payload hydration is opt-in and scope-bound.
- [x] Workspace list reads projection, not global event history.
- [x] Workspace detail projection is compact by default, including run history, LLM usage, LLM context budget, memory recall, execution sections and context bundle.
- [x] TUI prompt/status/main view are projection-driven.
- [x] Web board/detail are projection-driven.
- [x] Watchdog uses compact state only.
- [x] Projection manager has per-workspace dedupe/backpressure.
- [x] File backend supports bounded metadata queries.
- [x] Metrics expose payload hydrate and metadata scan pressure.
- [x] Long-running multi-workspace idle CPU/IO returns to stable baseline.

### Closure Notes

2026-06-21 workspace list projection path closure:

- HTTP `/api/v1/workorch/workspaces` reads `serverState.listProjectedWorkspaces()`,
  which delegates to `workspaceviewcore.Manager.ListWorkspaces()` rather than
  assembling list output from global event history.
- `HTTPServer.ListWorkspaceView` exposes the same projection-backed list to the
  appservice CLI command dispatcher.
- `CommandWorkspaceList` delegates to `WorkspaceViewReader.ListWorkspaceView`
  and no longer needs owner repository/session list assembly for the list
  command path.
- remote TUI/CLI backend uses `workorchclient.ListWorkspaceProjection()` against
  `/api/v1/workorch/workspaces`.
- embedded TUI/CLI backend sends `CommandWorkspaceList` through EventHub and
  receives the projection view reader result.

Verification:

```bash
go test ./internal/modules/application/workorchd/biz/appservice ./internal/services/workorch/biz/backends -count=1
```

Regression tests:

- `TestCLIWorkspaceListCommandUsesWorkspaceViewReader`
- `TestEventHubBackendWorkspaceListUsesProjectionViewReader`

2026-06-21 TUI projection-driven view closure:

- `FetchAll` reads `Workspace().List()`, which is projection-backed for both
  remote and embedded backends.
- `FetchWorkspaceContext` reads `Workspace().Snapshot()` and builds the TUI
  context from the server projection snapshot envelope. It does not issue
  additional run status/list/get reads for prompt/status/main view refresh.
- `renderInput` derives prompt suffixes from `activeWorkspaceOverview()`, whose
  source is workspace projection overview state.
- `renderRuntimeStatusBar` and main view state are refreshed from
  `WorkspaceContextMsg` / projection snapshot, while live EventHub notifications
  only trigger scoped projection refresh or transient UI progress updates.

Verification:

```bash
go test ./internal/services/workorch/service/tui -count=1
```

Regression tests:

- `TestFetchWorkspaceContextUsesProjectionSnapshotOnly`
- `TestModelWorkspaceListSynchronizesWaitingDecisionPrompt`
- `TestTUIE2EWorkspaceSwitchProjectionRefreshVisualSnapshot`
- `TestModelDecisionResolvedDoesNotHideServerOpenProjection`

2026-06-21 Web projection-driven board/detail closure:

- workspace board uses `workorchApi.listWorkspaceProjection()` and refreshes
  from projection-changed SSE only.
- workspace detail uses `workorchApi.listWorkspaceProjection()` for workspace
  selector data and `workorchApi.readWorkspaceSnapshotProjection()` for the
  detail ViewModel.
- workspace detail no longer auto-loads run snapshot / observability / event
  pages after every workspace projection refresh. Run process evidence remains
  available as an explicit drill-down action through the run table.
- run process drill-down calls are workspace scoped and paginated where page
  endpoints exist; they are no longer part of the default board/detail refresh
  path.

Verification:

```bash
cd web/workorch-console && npm run build
```

2026-06-21 watchdog compact-state closure:

- periodic watchdog still only invokes stale dispatched runtime-gap recovery.
- runtime-gap candidate enumeration now uses run owner read command with
  `states=[running]` and `compact=true`.
- compact run list items omit `ExecutionContext` and retain only generic run
  metadata needed for recovery classification.
- step lookup uses scoped step summaries; runtime evidence lookup uses scoped
  runtime checkpoints by run. The watchdog path does not read workspace
  snapshot, run snapshot, global event list, or raw event payload.
- automatic recovery remains gated by neutral runtime recovery policy:
  `request_not_accepted` is diagnostic-only, terminal gaps require terminal
  result evidence, and in-progress/worker-lost gaps require explicit watchdog
  retry safety metadata.

Verification:

```bash
go test ./internal/modules/kernel/run ./internal/modules/application/workorchd/biz/appservice/runports -count=1
go test ./internal/modules/application/workorchd/service/server -count=1
```

Regression tests:

- `TestRunCommandListSupportsCompactStateFilter`
- `TestRecoverStaleRuntimeGapRunViewsNoEligibleGapUsesBoundedReads`

2026-06-21 projection manager per-workspace dedupe/backpressure closure:

- `workspaceviewcore.Manager` maintains a per-workspace rebuild state entry with
  `inFlight`, `pending`, and `pendingAfterFull` flags.
- repeated rebuild requests for the same workspace are debounced into one
  pending rerun while the in-flight rebuild completes.
- rebuild requests for different workspaces are scheduled independently and do
  not share the same pending gate.
- workspace events observed while a full rebuild is in flight are retained as
  per-workspace `pendingAfterFull` entries and scheduled after the full rebuild.
- metrics expose pending workspace rebuild count, debounced rebuild count,
  dropped schedule count, and per-workspace rebuild duration/failure counters.

Verification:

```bash
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1
```

Regression tests:

- `TestManagerReschedulesWorkspaceRebuildWhenEventArrivesInFlight`
- `TestManagerSchedulesIndependentWorkspaceRebuildsWithPerWorkspaceDebounce`
- `TestManagerSchedulesWorkspaceRebuildAfterWorkspaceEventDuringFullRebuild`
- `TestManagerMetricsTracksPendingWorkspaceRebuilds`

2026-06-21 file backend bounded metadata query closure:

- file `EventRecordRepository.List` keeps default list results metadata-only and
  only hydrates payload on explicit `IncludePayload=true`.
- `Limit > 0` with no cursor uses reverse segment scan and stops after matching
  the bounded latest set, while preserving chronological return order.
- workspace-scoped event repository keeps run-scoped cursor/limit queries
  unchanged.
- workspace-scoped latest queries now push `Limit` down to each run event
  repository before global merge, avoiding full per-run event history reads for
  latest workspace metadata.
- workspace-scoped cursor pagination remains a scoped global merge path because
  cursor semantics are global to the merged workspace event list.

Verification:

```bash
go test ./internal/pkg/workorchrepo -count=1
go test ./internal/pkg/repository -count=1
go test ./internal/pkg/repository/file -count=1
```

Regression tests:

- `TestEventRecordRepositoryContract_File`
- `TestEventRecordRepositoryAppendsJSONLSegmentsWithoutMapRewrite`
- `TestWorkspaceScopedEventRecordListHonorsRunScopeCursorAndLimit`
- `TestWorkspaceScopedEventRecordWorkspaceLatestUsesBoundedPerRunLists`

2026-06-21 event record IO pressure metrics closure:

- repository-level `EventRecordIOMetricsSnapshot()` exposes neutral cumulative
  counters for event metadata scans, metadata records scanned, payload hydrate
  calls, and payload hydrate bytes.
- file event repository records metadata scan pressure while scanning JSONL
  event metadata segments in both forward and reverse paths.
- file event repository records payload hydrate pressure only when payload refs
  are explicitly opened.
- HTTP `/metrics` exposes the counters as:
  `workorch_event_record_metadata_scans_total`,
  `workorch_event_record_metadata_records_scanned_total`,
  `workorch_event_record_payload_hydrates_total`, and
  `workorch_event_record_payload_hydrate_bytes_total`.
- default event list remains metadata-only; explicit `IncludePayload=true` is
  the path that increases payload hydrate counters.

Verification:

```bash
go test ./internal/pkg/repository/file ./internal/modules/application/workorchd/service/server -count=1
```

Regression tests:

- `TestEventRecordRepositoryAppendsJSONLSegmentsWithoutMapRewrite`
- `TestMetricsEndpointReportsCounters`

2026-06-21 runtime idle baseline observation gate update:

- `scripts/observe-workorch-runtime-closure.sh` now requires the event record IO
  pressure metrics to exist in `/metrics`.
- the observation summary includes before/after deltas for metadata scans,
  metadata records scanned, payload hydrate calls, and payload hydrate bytes.
- deployed-instance idle gates can bound event IO pressure with:
  `WORKORCH_OBSERVE_MAX_EVENT_METADATA_RECORDS_SCANNED_DELTA`,
  `WORKORCH_OBSERVE_MAX_EVENT_PAYLOAD_HYDRATES_DELTA`, and
  `WORKORCH_OBSERVE_MAX_EVENT_PAYLOAD_HYDRATE_BYTES_DELTA`.
- the same observation script now records process-level CPU/RSS/IO when it can
  infer a local listening PID, or when `WORKORCH_OBSERVE_PROCESS_PID` is set.
- deployed-instance idle gates can require process resource evidence with
  `WORKORCH_OBSERVE_REQUIRE_PROCESS_RESOURCE=1` and can bound CPU/RSS/IO with:
  `WORKORCH_OBSERVE_MAX_PROCESS_CPU_PERCENT`,
  `WORKORCH_OBSERVE_MAX_PROCESS_RSS_BYTES`,
  `WORKORCH_OBSERVE_MAX_PROCESS_READ_BYTES_DELTA`, and
  `WORKORCH_OBSERVE_MAX_PROCESS_WRITE_BYTES_DELTA`.
- this prepares the remaining long-running CPU/IO baseline item to be verified
  with real deployed-instance observation rather than inferred from unit tests.

Verification:

```bash
bash -n scripts/observe-workorch-runtime-closure.sh
git diff --check
```

2026-06-21 managed multi-workspace idle baseline closure:

- added `make verify-runtime-idle-baseline`, which automatically manages a
  latest local `./bin/workorch` instance through
  `scripts/verify-workorch-managed-full.sh`.
- managed verification precreates idle workspaces before the observation
  baseline, waits for projection to drain, and then runs
  `observe-runtime-closure`.
- the default gate observes 2 idle workspaces, requires process resource
  snapshots, requires pending projection rebuilds to stay at 0, requires
  projection schedule drops to stay at 0, requires event metadata scan delta and
  payload hydrate delta to stay at 0, and bounds process CPU at 20%.
- 2026-06-21 latest-code managed run passed with artifacts at
  `/tmp/tmp.NxI6aSS9dM/observe-runtime-closure`:
  - `workspaceTotal=2`
  - both workspaces `state=active`, `activityState=idle`, no active run
  - `projectionPendingRebuilds=0`
  - `projectionScheduleDroppedDelta=0`
  - `metadataScanDelta=0`
  - `metadataRecordsScannedDelta=0`
  - `payloadHydratesDelta=0`
  - `payloadHydrateBytesDelta=0`
  - `processResource.cpuPercent=0.03322078528846294`
  - `processResource.readBytesDelta=0`
  - `processResource.writeBytesDelta=0`

Verification:

```bash
WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 make verify-runtime-idle-baseline
```

## Source Documents

- `application-workorchd.md`
- `mvvp-workspace-viewmodel-architecture.md`
- `storage-readmodel.md`
- `../50-product-interface/tui/display-refresh-consistency-design.md`
- `../70-roadmap/active/workorchd-state-loop-and-projection-closure-plan.md`
