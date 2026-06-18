# EventHub Driven Workorchd State Loop Design

Status: active

Type: module-architecture

Last Updated: 2026-06-16

## Purpose

本文固化 WorkOrch 在 `magicCommon/framework` 与 `EventHub` 规范下的命令触发、状态更新和任务推动设计，作为后续收口 `workorchd`、Web REST API、TUI、run continuation、governance replan/restart 逻辑的依据。

当前阶段不考虑兼容性。若现有实现与本文规则冲突，应以本文作为后续代码收口依据。

本文主要覆盖 Web/TUI/REST 用户命令、workorchd scheduler continuation、projection 刷新和 owner state 协同。Runtime block 长耗时执行从同步 `Send` 调用链迁移到事件队列化状态循环的设计与待办，见 [runtime-execution-event-queue-state-loop-design.md](runtime-execution-event-queue-state-loop-design.md)。

## Design Intent

合理的目标设计是：

- Web、TUI、CLI、agent 都只是用户交互入口和事件生产者。
- Web REST API 也不直接承担正式状态机推进语义；REST handler 只解析请求、校验 scope、投递 EventHub command，并返回 command 接收或命令结果。
- TUI local/remote 模式都通过 backend port 产生同一类 command event，不复制业务推进逻辑。
- `workorchd` 根据 EventHub 中的 command、owner notification、projection changed 和 watchdog/recovery tick 等事件，执行必要状态更新和任务推动。
- 正式业务状态仍由 kernel owner module 管理；跨 module/block 状态读写和状态机转换只能通过 EventHub command/response 完成。
- 长耗时 run start/resume/replan/restart/provider execution 不在 EventHub observer 中直接执行；observer 只做轻量校验和入队，实际执行交给 framework 注入的 `BackgroundRoutine`。
- 多 workspace 并行执行时，队列、lane、pending key 和恢复扫描必须显式携带 `workspaceId`，避免 workspace-local `run-001`、ticket、approval 串扰。

## Current Implementation Review

### TUI Replan Path

当前 TUI `/decision replan` 的主要链路是：

```text
internal/services/workorch/service/tui/model.go
  -> ResolveDecisionWithType
internal/services/workorch/service/tui/commands.go
  -> backend.Decision().Resolve
internal/services/workorch/biz/backends/remote.go
  -> workorchclient.ResolveDecision
internal/modules/application/workorchd/service/server/http_governance.go
  -> state.resolveDecision(...)
  -> state.scheduleGovernanceRunResumeRefs(...)
```

现状判断：

- `resolveDecision` 本身已通过 appservice / owner command 路径写入 Decision owner 状态。
- 但决议后的 continuation 不是一条正式 EventHub command，而是 HTTP handler 在写入成功后直接调用 `scheduleGovernanceRunResumeRefs`。
- `scheduleGovernanceRunResumeRefs` 当前属于 serverState 内存调度器逻辑，不是 owner module 的正式 command，也不是可由 EventHub replay/recovery 统一重建的 durable work item。
- 因此会出现“decision resolution 已写入，但后续 run resume/replan 没有可靠入队”的裂缝。

### TUI Restart Path

当前 TUI `/run restart` 的主要链路是：

```text
internal/services/workorch/service/tui/commands.go
  -> RestartWorkspaceRun
  -> backend.Run().Activate
internal/services/workorch/biz/backends/remote.go
  -> workorchclient.ActivateWorkspaceExecution
internal/modules/application/workorchd/service/server/http_workspace.go
  -> state.activateWorkspace(...)
```

现状判断：

- `/run restart` 实际调用的是 workspace execution activation。
- activation 逻辑在 workspace 已有 active non-terminal run 时，可能直接返回现有 run snapshot，而不是生成 restart/supersede/replacement request。
- 这条路径没有进入统一调度队列，也没有和 replan/decision continuation 使用同一套事件驱动循环。
- 因此在多 workspace、waiting decision、blocked、running 交错时，restart 容易表现为“请求成功但状态未变化”或“端侧认为提交了但后端没有新的执行动作”。

### REST Run Start/Resume Path

当前 run start/resume REST 路径在 HTTP handler 内同步获取 run slot 并直接调用 `startRun` / `resumeRun`。

现状判断：

- 这能保证单次 HTTP 调用得到同步结果，但不符合“REST 只产生 event，workorchd 根据 event 推动执行”的长期目标。
- 当请求碰到并发 slot、projection refresh、governance recovery 或服务重启时，执行意图不是统一的 pending command，恢复路径分散。
- 仍需保留 owner command 的同步错误语义，但长耗时执行推动应转为 EventHub command + BackgroundRoutine work item。

### Workorchd Watcher / Scheduler Status

现有 `workorchd` 已有若干接近目标的基础设施：

- `workspaceviewcore.Manager` 订阅 WorkOrch owner events，并维护服务端 ViewModel projection。
- `serverState.registerAutoGovernanceResumeWatcher` 订阅 auto-governance resolved event。
- `serverState.registerReplanContinuationWatcher` 已存在但当前为空逻辑。
- `runContinuationScheduler` 管理 periodic stale dispatched runtime gap recovery、manual governance/replan/continue/restart continuation；服务启动只注册 periodic watchdog timer，不自动恢复历史 running run。resolved governance 必须由 decision/approval resolve 主路径发布 continuation command，不能依赖 watchdog 扫描兜底。
- 当前未提交修改中已给 governance resume 增加了内存 pending map 和重试测试。

缺口是：这些能力仍不是统一的 EventHub command loop。HTTP handler 和 watcher 会直接调 scheduler 函数；scheduler 的 pending 也不是正式事件契约或可审计 command。

## Design Rules

### Command Source Rule

所有来自 Web/TUI/CLI/agent 的用户动作，必须先表达为明确 command intent：

| 用户动作 | command intent | 最终 owner |
| --- | --- | --- |
| `/decision replan` | resolve decision with `resolutionType=replan` | Decision owner, then workorchd continuation loop |
| `/decision answer` | resolve decision with `resolutionType=provide_missing_info` or equivalent | Decision owner, then workorchd continuation loop |
| approval approve/reject | resolve approval | Approval owner, then workorchd continuation loop |
| `/run continue` | request run continuation | workorchd scheduler/orchestrator loop |
| `/run restart` | request workspace restart / replacement run | workorchd scheduler/activation loop |
| REST execution activation | request workspace execution activation | workorchd scheduler/activation loop |
| REST run start/resume | request run execution command | workorchd scheduler/orchestrator loop |

Web/TUI 可以等待同步 command response，也可以收到 accepted response 后通过 projection/SSE/EventHub 刷新状态；但不能自行承担“如果没动就再补一刀”的业务逻辑。

### Event Ownership Rule

事件 topic 按被操作业务模型 owner 归属：

- Decision ticket/resolution 写入 topic 归 `kernel/decision/pkg/events`。
- Approval 写入 topic 归 `kernel/approval/pkg/events`。
- Run 正式状态写入 topic 归 `kernel/run/pkg/events`。
- Run 编排推进 topic 归 `kernel/orchestrator/pkg/events`。
- Workspace activation/restart 请求如果只是 facade/use-case command，归 `application/workorchd/pkg/events`；它不能成为 workspace owner 的正式状态。
- Projection changed notification 继续归 `application/workorchd/pkg/events`。

`internal/pkg/events` 只聚合 contract、Envelope、Response 和通用 payload key，不新增业务 topic。

### Observer Rule

EventHub observer 必须轻量：

- 可以校验 payload、scope、幂等 key。
- 可以把 work item 放入 workorchd 内部 pending queue。
- 可以发布 projection changed / accepted notification。
- 不直接执行 LLM、MCP、shell、long running run resume、workspace activation。
- 不在同一 lane 中同步等待需要同 lane 完成的 command。

### Pending Queue Rule

workorchd 内部 pending queue 必须满足：

- key 显式包含 workspace scope。
- replan/governance continuation key 至少包含 `workspaceId + runId + ticketId/resolutionId`。
- restart key 至少包含 `workspaceId + requestId`；如果要幂等合并，可加入 activeRunId 和 command fingerprint。
- start/resume key 至少包含 `workspaceId + runId + action + requestId`。
- BackgroundRoutine 投递失败时 pending 不丢失。
- 执行失败时，只有 owner 状态已经不再需要该 work item，才能清除 pending。
- startup/watchdog 必须能从 owner state 重建 pending，而不是依赖 HTTP handler 曾经投递过。

### Lane Rule

建议 lane 分类：

| topic 类型 | lane |
| --- | --- |
| Decision/Approval owner write | `state/<owner>/workspace/<workspaceId>/subject/<id>` |
| workorchd command accepted / pending mutation | `state/workorchd/workspace/<workspaceId>` |
| orchestrator start/resume/fail/complete | `state/orchestrator/workspace/<workspaceId>/run/<runId>` |
| run owner update | `state/run/workspace/<workspaceId>/run/<runId>` |
| projection notification | `topic/<projection-changed-topic>` 等 notify topic lane |
| TUI/CLI facade result/progress | topic lane |

禁止让所有带 run/workspace scope 的事件默认进入同一全局 run lane 或 workspace lane。

## Target Event Loop

### Governance Replan / Resolve

目标链路：

```text
TUI/Web
  -> ResolveDecision command request
HTTP/backend adapter
  -> Publish/Send Decision owner resolve command
Decision owner
  -> write resolution
  -> publish DecisionResolved notification
workorchd governance continuation subscriber
  -> validate workspace/run/ticket scope
  -> enqueue GovernanceContinuationRequested work item
BackgroundRoutine worker
  -> acquire run slot
  -> Send orchestrator resume command
  -> if run superseded, enqueue ReplanSuccessorRequested
  -> publish projection changed notification
Projection manager
  -> rebuild affected workspace ViewModel
Web/TUI
  -> refresh display from workorchd projection
```

关键点：

- HTTP handler 不直接调用 `scheduleGovernanceRunResumeRefs`。
- auto-governance 和 manual decision 走同一条 continuation event。
- resolution 已存在但 continuation 丢失时，startup/watchdog 从 Decision/Run owner state 重建 pending。

### Workspace Restart

目标链路：

```text
TUI/Web
  -> WorkspaceRestartRequested command
workorchd command subscriber
  -> enqueue restart work item
BackgroundRoutine worker
  -> read current workspace/run owner state
  -> if active non-terminal run exists, request orchestrator supersede/cancel according to restart policy
  -> create replacement run through activation use-case
  -> start replacement run through orchestrator command
  -> publish projection changed notification
```

关键点：

- `/run restart` 不再等价于普通 activation。
- restart 是明确的执行意图：旧 active path 被 supersede/terminated/replaced，新 run 必须可追踪 lineage。
- 如果 workspace 缺失或当前状态不允许 restart，应产生明确失败事件和端侧错误，而不是静默返回现有 run。

### Run Continue

目标链路：

```text
TUI/Web
  -> RunContinuationRequested command
workorchd command subscriber
  -> enqueue run continuation work item
BackgroundRoutine worker
  -> acquire run slot
  -> Send orchestrator resume command
  -> publish projection changed notification
```

关键点：

- `/run continue`、decision resolved auto-resume、approval resolved auto-resume 都应复用同一个 continuation worker，只是 source/reason 不同。
- continue 只能在 owner state 仍需要继续时执行；否则幂等清除 pending。

### Projection Refresh

目标链路：

```text
owner state changed notification
  -> workorchd projection manager observer
  -> enqueue projection rebuild by workspace
BackgroundRoutine projection worker
  -> load authoritative owner state through EventHub-backed read ports
  -> update ViewModel store
  -> publish projection changed
Web/TUI
  -> consume projection changed signal and read projection detail/list
```

关键点：

- Projection 是显示态，不驱动正式状态机。
- workorchd 根据 event 更新 ViewModel；Web/TUI 不自行 fan-out owner state。

## Adjustment Scope

### `internal/modules/application/workorchd/pkg/events`

需要新增 workorchd facade command topics 和 contract：

- `TopicWorkorchDCommandRequestRunContinuation`
- `TopicWorkorchDCommandRequestGovernanceContinuation`
- `TopicWorkorchDCommandRequestWorkspaceRestart`
- `TopicWorkorchDCommandRequestWorkspaceActivation`
- 可选：`TopicWorkorchDCommandSchedulerTick`

需要补充 payload contract：

- workspaceId
- runId
- ticketId / approvalId
- resolutionId
- requestId
- action
- priority
- executionContextPatch
- source

这些 topic 表达 workorchd use-case command，不拥有 kernel 正式状态。

### `internal/modules/application/workorchd/service/server`

需要调整：

- `http_governance.go`
  - resolve decision/approval 后不直接调用 `scheduleGovernanceRunResumeRefs`。
  - 改为发布 workorchd governance continuation requested event，或依赖 Decision/Approval owner resolved notification 触发同一 subscriber。
- `http_workspace.go`
  - execution activation handler 不直接同步执行长耗时 activation。
  - activation/restart 进入 workorchd scheduler command；若保留同步 REST response，需要等待 command accepted 或短同步 result。
- `http_run.go`
  - start/resume handler 不直接同步占用 run slot 执行。
  - 改为发布 run continuation/start command event，由 scheduler worker 执行。
- `http_run_continuation_scheduler.go`
  - 保留 BackgroundRoutine worker 和并发 slot 管理。
  - pending queue 改成 work item model，支持 continuation/replan/restart/start/resume 多类型。
  - 由 EventHub subscriber 入队，不由 HTTP handler 直接调用。
  - startup/watchdog 从 owner state 重建 pending。
- `http_types.go`
  - `registerReplanContinuationWatcher` 补齐或替换为统一 scheduler command subscriber。
  - `registerAutoGovernanceResumeWatcher` 不直接调用 scheduler 函数，改投递统一 command event 或调用同一个 enqueue event handler。
- `workspace_projection.go`
  - 仅维护 projection，不承担调度语义。

### `internal/modules/application/workorchd/biz/appservice`

需要调整：

- 保持 EventHub-backed appservice/read ports。
- activation use-case 可以继续负责创建 run、冻结 execution context、读取 template/defaults。
- 但 restart policy、run continuation work item、scheduler pending 不应散落在 HTTP layer。
- 若新增 workorchd scheduler use-case，应放在 appservice focused package 或 server scheduler 内部，避免变成 kernel owner。

### `internal/modules/kernel/orchestrator`

需要核对：

- start/resume/supersede/fail/complete command 是否已经具备 workspace scoped lane。
- resume replan successor 的状态机语义是否足够表达 `restart` 这种 operator requested replacement。
- provider/retry/replan 失败后是否发布足够 notification 让 workorchd projection 和 scheduler recovery 感知。

原则上不把 TUI/Web/restart 的交互语义写进 orchestrator；orchestrator 只执行中立 run state transition。

### `internal/modules/kernel/run`

需要核对：

- run owner command 和 read command 是否都携带 workspace scope。
- activeRunId、superseded predecessor、replacement/successor lineage 是否可以支持 restart/replan 查询。
- 无 workspace 的 run command 必须失败并提示 workspace 缺失。

### `internal/modules/kernel/decision` / `approval`

需要核对：

- resolved notification payload 是否包含 `workspaceId`、`runId`、ticket/approval id、resolution id、resolution type/action。
- 如果 notification 信息不足，workorchd watcher 需要通过 owner read command 补齐，不应无 scope 查找。
- batch resolve 必须逐项产生可恢复的 continuation work item。

### `internal/pkg/workorchport`

需要调整：

- port 层保留中立命令接口，不暴露 HTTP 细节。
- `/run restart` 对应 port command 应表达 restart intent，而不是普通 activate。
- `/run continue` 对应 port command 应表达 continuation intent。
- Decision/Approval resolve 返回后，端侧只依据 projection/event 刷新状态，不自行推断 run 已继续。

### `internal/pkg/workorchclient`

需要调整：

- REST client path/request 与新 command endpoint 对齐。
- 对 async accepted response 和 sync result response 做明确 error mapping。
- 所有 command 必须携带 workspace scope。

### `internal/services/workorch/service/tui`

需要调整：

- TUI command adapter 只把 slash command 转为 port command。
- `/run restart` 不再调用 activation。
- `/decision replan` 提交成功后立即清理本地输入和 blocker prompt，但最终状态以 projection event/detail 为准。
- TUI 不为“后端未动”增加本地重试状态机；后端 scheduler 负责 pending/retry/recovery。

### `web/workorch-console`

需要核对：

- Web action button 只调用 command endpoint。
- 页面状态来自 workspace projection/detail 或 run drill-down API。
- 不在前端用历史状态推断是否已继续执行。

### Scripts / E2E

需要新增或调整验证：

- TUI `/decision replan` 提交后，后端必须出现 governance continuation requested event 和 pending/started worker evidence。
- TUI `/run restart` 不再调用 activation-only path；必须创建 restart command event。
- 多 workspace 同时处于 waiting_decision 时，分别 replan 后都能形成独立 pending key 并被执行。
- 服务重启后，resolved decision 但无 successor/resume 的 workspace 能从 owner state 重建 pending。
- 并发 slot 被占满时，command 不丢失，后续 slot 释放后继续执行。
- Web/TUI 只消费 projection refreshed 后的状态，不依赖请求同步返回推断最终状态。

## Migration Phases

### Phase 1: Design Contract And Audit

- 补齐本文档。
- 核对所有 TUI/Web/REST 触发执行的入口。
- 列出仍直接调用 scheduler/start/resume/activation 的 HTTP handler。
- 固化新 workorchd command topic、payload、lane contract。

### Phase 2: Workorchd Command Topics And Subscriber

- 在 `workorchd/pkg/events` 增加 command topics/contracts。
- 在 server startup 注册统一 scheduler command subscriber。
- observer 只做 payload decode、scope 校验、幂等 key 生成和入队。
- 增加单测覆盖 topic/lane、payload scope、缺 response/缺 scope 错误。

### Phase 3: Scheduler Work Item Model

- 将当前 governance/replan continuation pending map 收口为统一 work item entry。
- 支持 governance continuation、run continuation、workspace restart、workspace activation、run start/resume。
- pending clear 必须基于 owner state still-needs-work 判定。
- startup/watchdog 从 owner state 重建 pending。

### Phase 4: REST/TUI/Web Command Path Migration

- HTTP handler 从直接执行改为发布 command event。
- TUI `/run restart` 从 activation 改为 restart command。
- TUI `/run continue`、decision/approval resolve 只提交 command，不复制恢复逻辑。
- Web action button 对齐同一 command endpoint。

### Phase 5: Verification

- 运行受影响包测试。
- 运行全量 `go test ./... -p 1 -count=1`。
- 使用自动管理服务执行 deployed e2e。
- 在真实多 workspace 场景验证两个 waiting decision workspace 分别 replan 后都能入队并推进。
- 观测 CPU/IO，确认没有因为丢失 command 进入高频扫描或重复 fan-out。

## Acceptance Criteria

- 所有来自 TUI/Web/REST 的 run 推动类动作都能在 EventHub 中看到明确 command event。
- HTTP handler 不再直接调用 `scheduleGovernanceRunResumeRefs`、`scheduleReplanContinuationIfNeeded`、长耗时 `startRun/resumeRun/activateWorkspace` 路径。
- workorchd scheduler observer 只轻量入队，实际执行在 BackgroundRoutine 中完成。
- pending work item 按 workspace scoped key 管理，不同 workspace 的同名 run/ticket 不串扰。
- 服务重启后，resolved governance blocker、superseded replan predecessor、stale running/dispatched run 都能由 recovery 重建 pending。
- `/run restart` 语义明确为 restart/replacement intent，不再等价于普通 activation 返回现有 active run。
- Web/TUI prompt、主视图、workspace list 的最终状态只来自 workorchd projection。
- 核心框架保持业务中立，不把软件开发、LSP、Go、具体验证命令等 profile 策略写入 scheduler/orchestrator core。

## Open Questions

1. REST command endpoint 是否立即切换为 `202 Accepted` 异步语义，还是短期保留同步 result response？
   - 推荐：执行推动类接口采用 accepted + projection refresh；查询/owner read 仍同步返回。

2. restart 的旧 active run 应默认 supersede、cancel，还是根据 run state 分流？
   - 推荐：restart command 使用中立 policy：running/dispatched 先 request supersede/cancel；waiting_decision/waiting_approval/blocked 直接创建 replacement lineage；具体可由 owner state transition 返回是否允许。

3. pending queue 是否需要持久化实体？
   - 推荐：短期以内存 pending + owner-state recovery 闭环；长期如需要强审计，可引入 scheduler work item owner 或 eventstream checkpoint。前提是任何 pending 都能从正式 owner state 重建。

4. auto-governance resolved notification 是否直接触发 continuation，还是只发布 owner notification？
   - 推荐：所有 manual/auto governance resolution 都通过同一个 resolved notification 或 workorchd command subscriber 进入 continuation queue。

## Code Review Checklist

- 搜索 HTTP handler 中的直接执行路径：
  - `scheduleGovernanceRunResumeRefs`
  - `scheduleReplanContinuationIfNeeded`
  - `acquireRunSlotWithPriority`
  - `startRun(`
  - `resumeRun(`
  - `activateWorkspace(`
- 搜索 TUI command 中的 activation/restart 混用：
  - `RestartWorkspaceRun`
  - `ActivateWorkspaceExecution`
  - `Run().Activate`
- 确认新增 topic 在 owner `pkg/events` 中定义，并登记 contract。
- 确认所有 command envelope 携带 workspace scope。
- 确认 observer 不执行长耗时动作。
- 确认 BackgroundRoutine 缺失时作为装配错误，不创建裸 goroutine fallback。
- 确认 ViewModel/projection 只更新显示态，不驱动正式状态机。
