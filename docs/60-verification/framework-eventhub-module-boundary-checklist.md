# Framework / EventHub / Module Boundary Checklist

Status: active

Type: verification-checklist

Last Updated: 2026-06-08

## Purpose

本文用于逐一核对 WorkOrch 当前核心模块是否遵循：

- `github.com/muidea/magicCommon/framework/application`
- `github.com/muidea/magicCommon/framework/service`
- `github.com/muidea/magicCommon/framework/plugin/initiator`
- `github.com/muidea/magicCommon/framework/plugin/module`
- `github.com/muidea/magicCommon/event.Hub`
- `github.com/muidea/magicCommon/task.BackgroundRoutine`

本文只作为核对与整改跟踪清单，不替代 runtime core contract。
涉及 runtime 状态机、阶段合约、repair/replan、Result/Artifact closeout 的判断，
仍以 `../00-core/runtime-stage-contract.md` 为准。

## Global Verification Dimensions

每个模块按以下 8 项核对：

| # | 维度 | 核对问题 |
| --- | --- | --- |
| 1 | 职责边界 | 是否只做本层该做的事，是否越权承担其他层职责 |
| 2 | 生命周期 | `Setup` / `Run` / `Teardown` / `Shutdown` 是否符合 magicCommon 规范 |
| 3 | 依赖方向 | 是否通过 initiator/helper/service/interface 接线，避免跨模块全局变量或反向耦合 |
| 4 | EventHub | 是否使用统一 EventHub，事件是否包含 workspace/run/step refs，订阅是否释放 |
| 5 | BackgroundRoutine | 长期任务、timer、异步恢复是否可取消、可等待，是否避免散落裸 goroutine |
| 6 | 状态机 | 是否直接写 Run/Workspace/Result 等正式状态；若写，是否属于权威模块 |
| 7 | Workspace Scope | run/step/result/snapshot 查询是否显式绑定 workspace scope |
| 8 | 测试与文档 | 是否有 contract / regression test 与对应架构说明 |

## Global Boundary Rules

- Application 负责进程入口、HTTP/TUI surface、appservice use-case 组合，不拥有业务状态机。
- Initiator 负责应用级基础能力和模块间依赖源，不承担具体业务状态裁决。
- Kernel 负责正式领域对象、业务状态、orchestrator 编排和治理状态转换。
- Blocks 负责外部动作和 provider interaction，只产出 runtime 可消费证据，不裁决 accepted business outcome。
- Shared internal packages 只放 common types、event helper、repository contract、config 等共享基础设施。
- Web/TUI/HTTP 层只消费 read model 或调用 appservice，不直接定义业务状态语义。
- Runtime session 只管理 request-scoped runtime access 与 teardown，不能成为新的业务状态拥有者。
- Run ID 是 workspace 内部运行序号；run snapshot / status / resume / detail 应优先使用 `(workspaceID, runID)`。
- 生产 module/block 间同步交互只能通过 `EventHub` command event 完成；不得在一个 module/block 的生产装配中直接注入另一个 owner module 的 service 或正式 state repository。
- `biz` 层可以保留窄接口或 repository-shaped contract 以便单元测试和局部算法复用，但生产 `module.Setup` / plugin / bootstrap 必须注入 EventHub-backed adapter，而不是跨 owner repository/service。
- owner module 负责订阅自己的 command topic、调用自身 service/repository，并返回 `events.Response`；调用方只发布 command envelope，不直接触达 owner state。

## Application Entry / Framework Layer

| 模块 | 代码落点 | 核对内容 | 状态 |
| --- | --- | --- | --- |
| application entry | `application/workorch/main.go` | 是否只负责进程入口、信号、显式启用 initiator/module/block，并调用 process service；是否避免业务逻辑 | reviewed |
| workorch process service | `internal/services/workorch` | 是否实现进程级 `framework/service.LifecycleService`；server / interactive / remote 模式边界是否清晰；是否不作为 plugin module 注册 | reviewed |
| workorch CLI/TUI port | `internal/pkg/workorchport` | CLI/TUI/agent 是否只依赖 port 契约；port 是否不依赖具体 backend 实现或 `internal/services/workorch` | reviewed |
| workorch REST client | `internal/pkg/workorchclient` | REST client 是否只负责 HTTP path、request/response 和 error mapping；是否不依赖 framework application、EventHub、CLI/TUI | reviewed |
| workorch backend implementations | `internal/services/workorch/biz/backends` | remote backend 是否只使用 `workorchclient`；embedded backend 是否只通过 EventHub command/response 访问正式 owner state | reviewed |
| workorchd plugin | `internal/modules/application/workorchd` | 是否接收框架 `event.Hub` / `BackgroundRoutine`；`Startup` / `Run` / `Shutdown` 是否清晰 | reviewed |

## Initiator / Runtime Assembly Layer

| 模块 | 代码落点 | 核对内容 | 状态 |
| --- | --- | --- | --- |
| workorchruntime initiator | `internal/initiators/workorchruntime` | 是否只提供 repository、EventHub、runtime policy 等窄装配能力；是否 fail-fast；是否不承担业务状态机或跨模块 service registry | reviewed |
| app bootstrap | `internal/modules/application/workorchd/biz/bootstrap` | grouped exports 是否清晰；module setup 顺序是否合理；是否避免多套 EventHub 误用 | reviewed |
| RuntimeSession | `internal/modules/application/workorchd/biz/appservice/runtime_session.go` | 是否只管理 request-scoped runtime access 和 teardown；是否不拥有业务状态 | reviewed |

## Application Use-Case Layer

| 模块 | 代码落点 | 核对内容 | 状态 |
| --- | --- | --- | --- |
| appservice activation | `internal/modules/application/workorchd/biz/appservice/workspace_execution_activation_service.go` | 是否只组合 Goal/DoD/Run 创建与启动；状态转换是否交给 kernel service | reviewed |
| appservice recovery | `internal/modules/application/workorchd/biz/appservice/run_recovery_service.go` | 启动恢复是否只识别通用状态形态；是否避免直接裁决复杂状态 | reviewed |
| appservice decision | `internal/modules/application/workorchd/biz/appservice/decision_service.go` | 是否只做 use-case / API 组合；正式 ticket 状态是否由 kernel 持有 | reviewed |
| appservice approval | `internal/modules/application/workorchd/biz/appservice/approval_service.go` | 是否只做 use-case / API 组合；正式 approval 状态是否由 kernel 持有 | reviewed |
| read model | `internal/modules/application/workorchd/biz/appservice/read_model_*.go` | snapshot 是否只是投影；是否显式 workspace scope；是否不反写状态 | reviewed |
| HTTP server | `internal/modules/application/workorchd/service/server` | 是否只做 handler / validation / response / SSE；治理续跑、recovery、replan 是否有下沉计划 | reviewed |

## Kernel Formal State Layer

| 模块 | 代码落点 | 核对内容 | 状态 |
| --- | --- | --- | --- |
| workspace | `internal/modules/kernel/workspace` | Workspace / Goal / DoD lifecycle 是否自洽；是否不混入 runtime adapter 逻辑 | reviewed |
| run | `internal/modules/kernel/run` | Run / Step / evidence primitives 是否权威；状态转换规则是否集中 | reviewed |
| orchestrator | `internal/modules/kernel/orchestrator` | dispatch、repair、replan、completion gate 是否是唯一运行推进核心 | reviewed |
| decision | `internal/modules/kernel/decision` | Ticket / Resolution lifecycle 是否独立；是否不承担 UI 交互 | reviewed |
| approval | `internal/modules/kernel/approval` | pending / approved / rejected 转换是否权威；是否只作为恢复允许/拒绝输入 | reviewed |
| governance | `internal/modules/kernel/governance` | 自动治理、升级、resume 触发是否不阻塞 EventHub；是否不替代 Run 状态机 | reviewed |
| result | `internal/modules/kernel/result` | Result / Artifact 是否只表达正式 outcome / deliverable；是否不被 block 直接裁决 | reviewed |
| eventstream | `internal/modules/kernel/eventstream` | 是否 append-only 记录事件；订阅是否在 `Stop` / `Teardown` 释放 | reviewed |
| soul | `internal/modules/kernel/soul` | 是否只持有 Soul 正式对象；是否不写死业务 profile 行为 | reviewed |
| skill | `internal/modules/kernel/skill` | 是否只持有 Skill 正式对象；是否不写死业务 profile 行为 | reviewed |
| mcp catalog | `internal/modules/kernel/mcp` | 是否只持有 MCP server/capability 正式对象；是否不执行 MCP 调用 | reviewed |
| memory | `internal/modules/kernel/memory` | 是否只持有 memory/recall trace 正式对象；是否不替代 runtime context assembly | reviewed |

## Blocks Runtime Adapter Layer

| 模块 | 代码落点 | 核对内容 | 状态 |
| --- | --- | --- | --- |
| llm | `internal/modules/blocks/llm` | 是否只负责 provider request / response / schema normalization；不直接执行 side effect | reviewed |
| mcp | `internal/modules/blocks/mcp` | MCP tool/resource/prompt 是否只产出 evidence；approval/risk 是否交治理 | reviewed |
| shell | `internal/modules/blocks/shell` | command 执行是否有 timeout、bounded output、failure classification | reviewed |
| file | `internal/modules/blocks/file` | file/read/edit/patch 是否只产出 patch/evidence；不裁决 Run 终态 | reviewed |
| git | `internal/modules/blocks/git` | git read/write 是否受限、结构化输出、不能越过 orchestrator | reviewed |
| web | `internal/modules/blocks/web` | search/fetch 是否只返回来源证据；不写业务结论 | reviewed |
| ssh | `internal/modules/blocks/ssh` | 远端命令执行边界、timeout、输出裁剪、风险治理 | reviewed |

## Shared Infrastructure Layer

| 模块 | 代码落点 | 核对内容 | 状态 |
| --- | --- | --- | --- |
| common types | `internal/pkg/common` | 状态枚举、ID、context scope 是否统一；是否适合承接公共状态机 helper | reviewed |
| events | `internal/pkg/events` | `MagicHub` 的 `Send` / `Post`、lane key、revision、unsubscribe 是否符合规范 | reviewed |
| repository | `internal/pkg/repository` | workspace-scoped run identity 是否完整；file / memory backend 行为是否一致 | reviewed |
| config | `internal/pkg/workorchconfig` | 配置加载是否只在入口/bootstrap；业务层是否避免散乱读取 | reviewed |
| repo bootstrap helpers | `internal/pkg/workorchrepo` | repository backend/data root 解析、workspace-scoped repository set、artifact storage repository 是否作为共享基础设施，而非 application bootstrap 私有实现被 initiator 反向依赖 | reviewed |
| builtin defaults | `internal/pkg/workorchdefaults` | 内置 soul/template/MCP 默认值是否由 runtime initiator 显式加载；application bootstrap 测试 fixture 不被隐式改写 | reviewed |
| runtime policy mapping | `internal/pkg/workorchruntimepolicy` | config 到 orchestrator runtime policy 的映射是否位于共享 helper，避免 initiator/application bootstrap 循环依赖 | reviewed |

## EventHub Verification Rules

- 状态机关键事件应使用同步 `Publish`，由 `MagicHub` 映射到 magicCommon `Send`。
- 纯通知或 UI/observability 类事件可使用 `PublishAsync`，由 `MagicHub` 映射到 magicCommon `Post`。
- 每个 workspace-scoped 事件必须包含 `WorkspaceID`；run/step 事件还必须包含 `RunID` / `StepID`。
- lane 表达一致性边界，不直接表达业务作用域；`RunID` / `WorkspaceID` 只是 scope，不能自动决定串行队列。
- owner read/query command 应落在 query lane，不与 state write lane 共队列。
- run owner 正式记录写入和 formal evidence 绑定应落在 `state/run/<workspaceID>/<runID>` lane；orchestrator 执行推进应落在 `state/orchestrator/workspace/<workspaceID>/run/<runID>` lane；`runID` 是 workspace-local，不能单独作为跨 workspace 串行 key；workspace owner 写入应落在 `state/workspace/<workspaceID>` lane。
- block execution 使用 execute/resource lane，例如 `exec/block/<kind>/workspace/<workspaceID>/step/<stepID>`，不因携带 `RunID` 默认进入 run state lane；当 `stepID` 是 workspace-local 时也必须带 workspace scope。
- CLI/TUI facade、progress、notification 和 observability 事件按契约使用 topic lane 或异步通知 lane。
- 详细规则以 `../30-module-architecture/eventhub-lane-policy-and-module-event-contract-plan.md` 为准。
- 事件 handler 不应做长耗时工作；若需要 resume、recovery、batch scan，应转交 scheduler/background task。
- 所有 `Subscribe` 返回的 unsubscribe 必须在 `Stop` / `Teardown` / `Shutdown` 中调用。
- command topic 必须使用同步 `Publish`/`Send` 路径，并要求收到 `events.Response`；无 response 视为装配或生命周期错误。

## BackgroundRoutine Verification Rules

- 应用级长期任务、timer、队列型恢复任务应优先使用框架传入的 `BackgroundRoutine` 或统一 scheduler 封装。
- 模块内部短期并发可用局部 goroutine，但必须受 context / wait group / timeout 管理。
- `Setup` 不启动长期任务；`Run` 启动订阅、timer、listener 或 scheduler；`Teardown` 取消并等待。
- shutdown 后不得继续提交 background task。

## Suggested Audit Order

1. `application/workorch`、`workorchd`、`workorchruntime`：确认框架生命周期和运行域。
2. `bootstrap`、`RuntimeSession`、`events.MagicHub`：确认 EventHub / module assembly / request scope。
3. `kernel/run`、`kernel/orchestrator`、`kernel/governance`：确认状态机权威边界。
4. `kernel/decision`、`kernel/approval`、`kernel/result`、`kernel/eventstream`：确认治理与结果对象边界。
5. `appservice`、`server`、read model：确认 application use-case 与 HTTP surface 不越界。
6. `blocks/*`：确认 adapter 只执行外部动作并产出证据。
7. `repository`、workspace-scoped identity：确认 run id scope 和 snapshot/detail 查询一致。

## Follow-Up TODOs

- [x] 完成本轮 WorkOrch framework/module 边界整理后，回写并完善对应开发 skill，确保后续应用开发能准确创建和使用符合 `github.com/muidea/magicCommon/framework` 的 module。
  - Updated skills: `go-module-initiator-lifecycle@1.1.3`, `go-multi-module-dev@2.2.1`, `go-application-event-runtime@1.0.3`。
  - Skill updates cover: framework side-effect imports 只放入口 `main` 按需启用；module `Setup` / `Run` / `Teardown` 职责边界；initiator/helper 依赖获取和 fail-fast；EventHub / BackgroundRoutine 接线；禁止在 internal bootstrap 中复用 framework plugin manager；禁止 runtime/helper/bootstrap 暴露跨 owner `ServiceExports` / `SetXService` / `XService()`；跨 owner 正式状态读写必须通过 EventHub command/`events.Response`；测试和文档同步要求。
  - Evidence: `skill-hub validate go-multi-module-dev --links`, `skill-hub validate go-module-initiator-lifecycle --links`, `skill-hub validate go-application-event-runtime --links` all passed; `skill-hub feedback <skill-id> --force` synced the three project skills to the local default skill repository.

## Latest Closure Summary

- Production module/block write-side cross-owner interactions have been moved behind synchronous EventHub command topics. Owner modules subscribe in `Run` and unsubscribe in `Teardown`; tests that use framework plugins now execute both `Setup` and `Run` before relying on command topics.
- Appservice facades for Workspace/Goal/DoD/Run/Decision/Approval now use owner command events for formal state writes and state-machine transitions. Goal/DoD define/update/delete, run create/start/resume/fail/update, decision resolve, approval approve/reject, and recovery workspace pointer updates are routed through owner EventHub topics.
- Appservice activation no longer calls kernel Run/Memory/MCP/Soul services directly for create/start/supersede/fail, memory selection, memory-recall trace persistence, MCP selection, or soul capability validation. It publishes owner command events and treats a missing `events.Response` as lifecycle wiring failure.
- Appservice workspace setup draft generation no longer constructs an LLM provider directly. It publishes synchronous `TopicLLMCommandCompleteProvider`; the LLM block owns provider config loading, provider selection, provider invocation, and returns `ProviderCompletionResult` through `events.Response`.
- Appservice semantic memory retrieval no longer constructs an LLM runtime adapter for embeddings. It publishes synchronous `TopicLLMCommandGenerateEmbedding`; the LLM block owns embedding provider config loading and embedding generation, and appservice only consumes the returned `ProviderEmbeddingResult`.
- Appservice production code no longer imports kernel owner `service` packages or owner module command aliases for EventHub command payloads. Command publishers use stable event payload keys and owner model payloads; owner modules translate those payloads into their internal service commands inside the subscribed handler.
- Context registry Soul/Skill/MCP/Memory write/read command paths now publish owner commands. CLI/TUI embedded mode publishes `/cli/command` envelopes only; the workorchd owner dispatcher calls appservice EventHub-backed use-cases and returns `/cli/result`.
- Governance no longer falls back to direct Goal/DoD repository access in production; it uses a workspace definition reader backed by owner commands. Tests retain narrow fakes for local algorithm coverage.
- MagicHub lane policy now scopes run owner state lanes as `state/run/<workspaceID>/<runID>` and orchestrator execution lanes as `state/orchestrator/workspace/<workspaceID>/run/<runID>`. This preserves owner-local per-run serialization without making different workspaces' local `run-001` executions block each other, and avoids synchronous orchestrator handlers waiting on their own lane when they write back through the run owner.
- MagicHub subscription/unsubscription against the underlying magicCommon hub now uses bounded waits and no longer holds the WorkOrch wrapper lock while subscribing. Wildcard subscriptions are intentionally wrapper-local, while concrete topic subscriptions still deliver across wrappers sharing the same underlying hub.
- Governance notification observers no longer execute auto-governance synchronously inside the EventHub dispatch stack. Ticket/approval/run-state notifications enqueue lifecycle-managed governance work, so nested owner command `Send` calls do not block the original EventHub handler.
- MCP catalog default capability derivation remains generic for arbitrary tools/resources/prompts, while built-in acceptance resource/prompt metadata is intentionally applied for the stdio e2e and user-facing catalog display.
- Memory recall trace persistence is now owned by `kernel/memory`: appservice activation/recovery publish synchronous `TopicMemoryRecallTraceCommandUpsert`, the memory module handles trace repository writes, and `TopicMemoryRecallRecorded` remains the notification event consumed by observability/UI.
- Orchestrator production wiring no longer receives the `MemoryRecallTraceRepository` directly. Runtime memory recall trace create/upsert/get/list/delete operations from orchestrator now go through an EventHub-backed repository adapter that publishes memory owner command topics.
- `workorchruntime` initiator no longer exposes kernel owner service getters/setters. Its helper surface is limited to `Repositories()`, `EventHub()`, and `RuntimePolicy()`, so framework plugin modules obtain only runtime infrastructure and must communicate cross-owner actions through EventHub command topics.
- `workorchruntime` initiator no longer imports `internal/modules/application/workorchd/biz/bootstrap`. Shared repository backend/data-root helpers, workspace-scoped file repository construction, artifact storage repository, builtin defaults seeding, and runtime policy mapping now live under `internal/pkg/workorchrepo`, `internal/pkg/workorchdefaults`, and `internal/pkg/workorchruntimepolicy`.
- `bootstrap.Exports` no longer contains `ServiceExports`; application/session assembly receives repositories, EventHub, and config loader only. Tests that previously used `exports.Services.*` now either publish owner command events through EventHub or create repository fixtures for local algorithm setup.
- Kernel/block/application `plugin.go` files no longer create extra `plugin` wrapper structs. The framework registration path now registers `New()` directly, and the concrete `*Module` implements framework `Setup(ctx,eventHub,bg)`, `Run(ctx)`, and `Teardown(ctx)` while explicit embedded-runtime/test wiring uses `Configure/Start/Stop`.
- `application/workorch/main.go` now only handles process signal context, `workorch.Run`, and explicit framework side-effect imports for the entrypoint-selected initiator/modules. Test-only default-workspace wrappers were removed from production `main`, so the entry file no longer imports CLI backend/common packages.
- `application/workorch` no longer imports the `workorchd` module package to wait for server shutdown. `main` explicitly enables `workorchd` with a side-effect import, while server mode queries `TopicWorkorchDCommandDone` through EventHub; the `workorchd` module subscribes in `Run` and unsubscribes in `Teardown`.
- `workorchd` no longer keeps a global `defaultModule` singleton or package-level `Done()` helper. The framework registration path uses `pluginmodule.Register(New())`, and server done-channel access remains an EventHub command handled by the module instance.
- `application/workorch/app` no longer imports `workorchd/biz/appservice` or constructs `RuntimeSession` directly. Embedded interactive mode obtains a `backend.NewEmbeddedEventHub()` backend from the runtime initiator EventHub; appservice/session handling is owned by workorchd.
- The old appservice-backed `localBackend` has been removed. CLI/TUI embedded operations now use EventHub command/result transport, with the workorchd dispatcher borrowing request-scoped `RuntimeSession` instances per command.
- The workorchd CLI dispatcher no longer uses an unmanaged goroutine fallback when publishing `/cli/result`. Async-capable hubs still use `PublishAsync`; non-async hubs synchronously publish the result envelope and also return `events.Response`, preserving command/result semantics without introducing untracked background work.
- Runtime block execution has been rechecked end to end: orchestrator publishes synchronous `/runtime/block/command/execute/<kind>` envelopes, block modules subscribe via `runtimeevent.Executor.Run`, return `events.Response`, and unsubscribe through module `Teardown`.
- EventHub command subscription lifecycles are now idempotent across owner modules and block executors. Repeated `Run()` calls do not add duplicate command observers; `Teardown()` still releases subscriptions and allows a later setup/run cycle to subscribe again.
- EventHub command response handling has been rechecked for explicit `Publish(... events.Envelope ...)` call sites. Command helpers for governance, run/orchestrator, runtime block execution, result/artifact, state evidence, runtime actions, owner reads, context registry, workspace setup draft, LLM embedding, MCP/Web owner lookups, and activation now require an `events.Response`; notification publishers remain response-free by design.
- Remaining production goroutines were reclassified. Request-local fan-out in orchestrator result closeout, execution-context assembly, appservice definition validation, and activation selection uses channels or `sync.WaitGroup` and waits before returning. Long-lived HTTP recovery/replan/governance continuation work is routed through `serverState.scheduleBackground`, which uses the framework `BackgroundRoutine` when available and otherwise keeps explicit context cancellation plus wait-group shutdown.
- CLI `agent` command-local execution now propagates `RunWithBackend` errors even when the backend fails before emitting any terminal event. The command also owns a cancelable context and cancels it on return, so early CLI exits do not leave the agent run goroutine tied to `context.Background()`.
- Appservice Workspace/Soul read-side owner access for soul binding, soul routing, workspace setup draft, context registry SkillHub workspace resolution, run definition validation, and workspace execution activation now uses synchronous owner EventHub command topics instead of direct `Workspaces` / `Souls` repository reads.
- Appservice Goal/DoD/Run/Skill/Decision/Approval/Result/Artifact read-side owner access for definition selection, Goal/DoD facade operations, quality check, run facade operations, decision/approval facade operations, activation response assembly, activation governance history, rejected-result continuation lookup, skill selection, raw result content, artifact content, run ID generation, and stuck-run recovery now uses synchronous owner EventHub command topics instead of direct owner repository `Get` / `List` calls.
- Run owner `/run/command/get` now honors optional `workspaceId` from envelope or payload and returns not found when the persisted run belongs to a different workspace. Appservice `readRunByOwnerCommand` forwards the runtime workspace scope into the owner command, so scoped read-model/recovery paths are guarded both in the caller helper and at the owner command boundary.
- Run-owned Step and RuntimeCheckpoint reads used by raw runtime content and stuck-run recovery now route through `TopicRunCommandListStepsByRun`, `TopicRunCommandGetRuntimeCheckpoint`, and `TopicRunCommandListCheckpointsByRun`. The run module owns the repository access and responds with `events.Response`; appservice no longer reads `Steps` / `RuntimeCheckpoints` directly outside read-model projection assembly.
- EventStream `Service.Start()` and module `Run()` are now explicitly idempotent in tests. `Service.Stop()` and module `Teardown()` release subscriptions, and events published after stop are no longer persisted.
- TUI/EventHub backend run continuation paths now have regression coverage proving active workspace scope is bound into `Run.Get`, `Run.List`, and `Run.Resume` calls. This keeps workspace-local run IDs from resuming or displaying another workspace's run after workspace switching.
- Auto-governance event publishing now has explicit contract coverage for evaluated/resolved/escalated/blocked events. The emitted envelope carries `WorkspaceID`, `RunID`, and `StepID` from the persisted `AutoGovernanceTrace`, and the payload keeps the trace/item refs decodable through `DecodeAutoGovernanceEvent`.
- Result/Artifact closeout production wiring has been verified: orchestrator receives EventHub-backed Result/Artifact repository adapters, publishes synchronous result owner command topics for create/get/list/update/delete, and treats missing `events.Response` as lifecycle wiring failure. Artifact commands now carry `SubjectType=artifact`; list-by-workspace/run/result commands carry the scoped subject type they query.
- Direct orchestrator `CompleteRun` command now enforces the acceptance contract for DoD-gated LLM workspace runs: a run that requires the DoD completion gate cannot enter `completed` unless the result owner already has an accepted Result for the run. Normal graph/LLM completion paths still validate DoD/evidence/coverage, record accepted Result/Artifact evidence, and then complete.
- Web approval-required policy remains a block output, not an adapter-owned governance state transition. Orchestrator converts `blockApproval*` output into an Approval owner record and blocks the run; Web only validates an already-approved Approval through an EventHub owner command. The Web approval getter now includes `RunID` in the owner command envelope/payload.
- Workspace patch preview/apply/revert remains centralized in the orchestrator patch subsystem. `file/edit` returns patch preview evidence only; incremental artifact apply builds patch output and then uses the same orchestrator patch path; patch preview/applied/failed/reverted events are emitted by orchestrator.
- The TUI/CLI backend no longer reads WorkOrch runtime owner repositories or imports workorchd appservice in production. It publishes CLI command envelopes through EventHub; workorchd owns command handling and appservice access.
- Application read-model and snapshot builders now use EventHub-backed reader interfaces for formal/catalog/run-evidence/governance/eventstream state. Workspace/Goal/DoD/Run/Step/RuntimeCheckpoint/Decision/Approval/Result/Artifact/Soul/Skill/MCP/MemoryRecallTrace/EventRecord/ToolCall/Patch/ValidationResult/ArtifactContinuation/AutoGovernanceTrace reads are routed to owner command topics; owner modules perform repository access and return `events.Response`.
- Application and initiator production code no longer imports block/kernel `biz` packages. Shared runtime policy is defined in `kernel/orchestrator/pkg/models`; LLM and Web provider config parsing/redaction is exposed through `blocks/*/pkg/providerconfig`; bootstrap runtime assembly uses the orchestrator module public constructors for local runtime registry and patch adapter setup.
- Remaining direct appservice repository access is limited to application-owned workspace templates and test-only helpers/fakes. This is not treated as kernel module/block state interaction.
- Current scans:
  - `rg -n 'internal/modules/(blocks|kernel)/.*/biz' internal/modules/application internal/initiators -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "exports\\.Services\\.[A-Za-z]+Service\\.[A-Za-z]+\\(" internal -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "internal/modules/kernel/.*/service" internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "internal/modules/kernel/(soul|skill|mcp|run|workspace|decision|approval)\\\"" internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "PayloadKeyCommand" internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "exports\\.Repositories\\.(Workspaces|Goals|DoDs|Runs|MemoryItems|Tickets|Resolutions|Approvals|Results|Artifacts|MCPServers|MCPCapabilities|Souls|Skills)\\.(Create|Update|Delete|Upsert)" internal -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "exports\\.Repositories\\.(Workspaces|Souls)\\.(Get|List)" internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "exports\\.Repositories\\.(Workspaces|Souls|Goals|DoDs|Runs|Skills|Results|Artifacts)\\.(Get|List|ListByWorkspaceID|ListByRun|ListByWorkspace|ListByResult)" internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "exports\\.Repositories\\.(Workspaces|Goals|DoDs|Runs|Steps|Results|Artifacts|MemoryRecallTraces|MCPCapabilities|Tickets|Resolutions|Approvals|ArtifactContinuations|EventRecords|ToolCalls|Patches|ValidationResults|AutoGovernanceTraces)" internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "repository\\.(WorkspaceRepository|GoalRepository|DoDRepository|RunRepository|StepRepository|RuntimeCheckpointRepository|DecisionTicketRepository|DecisionResolutionRepository|ApprovalRepository|ResultRepository|ArtifactRepository|SoulRepository|SkillRepository|MCPServerRepository|MCPCapabilityRepository|MemoryRecallTraceRepository|ArtifactContinuationRepository|EventRecordRepository|ToolCallRepository|PatchRepository|ValidationResultRepository|AutoGovernanceTraceRepository)" internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "exports\\.Services|Services:|ServiceExports|Bootstrap\\.Exports\\(\\)\\.Services|Set[A-Za-z]+Service|[A-Za-z]+Service\\(\\)" internal application -g '*.go' --glob '!**/*_test.go'` returns no cross-owner service facade matches.
  - `rg "Repositories\\.(Workspaces|Goals|DoDs|Runs|Steps|Results|Artifacts|MemoryRecallTraces|MCPCapabilities|Tickets|Resolutions|Approvals|ArtifactContinuations|EventRecords|ToolCalls|Patches|ValidationResults|AutoGovernanceTraces)" internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg -n 'internal/modules/application/workorchd/biz/appservice' application/workorch internal/services/workorch -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg -n 'NewLocal\\(|localBackend' application internal -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg -n 'TopicCLICommand|NewCLICommandDispatcher|CLICommandDispatcher' internal/services/workorch internal/modules/application/workorchd internal/pkg -g '*.go' --glob '!**/*_test.go'` shows the workorch backend publisher and workorchd dispatcher subscriber only.
  - `rg "\\.Repositories\\.(Workspaces|Goals|DoDs|Runs|Steps|RuntimeCheckpoints|Tickets|Resolutions|Approvals|Results|Artifacts|Souls|Skills|MCPServers|MCPCapabilities|MemoryRecallTraces|ArtifactContinuations|EventRecords|ToolCalls|Patches|ValidationResults|AutoGovernanceTraces)\\.(Get|List|ListByWorkspaceID|ListByRun|ListByWorkspace|ListByResult|GetByTicketID|Create|Update|Delete|Upsert)" internal/modules -g '*.go' --glob '!**/*_test.go'` returns no production matches outside owner modules.
  - `rg "Services\\.(WorkspaceService|RunService|DecisionService|ApprovalService|ResultService|SoulService|MCPService|MemoryService|SkillService|EventStreamService)\\.[A-Za-z]" internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg "\\.(Create|Update|Delete|Upsert)\\(ctx," internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` returns only application-owned `workspace_template_service.go` writes.
  - `rg "NewRuntimeAdapter|ExecuteRuntimeStep|Complete\\(|GenerateEmbedding|LoadProviderConfigs|NewProviderFromConfig" internal/modules/application -g '*.go' --glob '!**/*_test.go'` returns no direct runtime/provider invocation matches outside configuration display/validation helpers.
  - `rg "TopicLLMCommand(CompleteProvider|GenerateEmbedding)" internal/modules internal/pkg -g '*.go'` shows LLM owner subscriptions in `internal/modules/blocks/llm/module.go` and appservice publishers in workspace setup draft / activation semantic memory retrieval.
  - `rg -n 'github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap' internal/initiators application internal/modules -g '*.go' --glob '!**/*_test.go'` returns only same-application appservice/bootstrap wiring matches and no initiator matches.
  - `rg -n 'RuntimePolicyFromConfig|appbootstrap\\.NewWorkspaceScopedFileRepositorySet|bootstrap\\.ResolveRepositoryDataRoot|NormalizeRepositoryBackend' internal application -g '*.go'` returns no old bootstrap helper usage; remaining `NormalizeRepositoryBackend` matches are in `internal/pkg/workorchrepo` and its callers.
  - `rg -n 'github.com/muidea/workorch/internal/modules/(blocks|kernel)/[^/]+/biz' internal/modules/application internal/initiators application -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg -n 'exports\\.Services|ServiceExports|Bootstrap\\.Exports\\(\\)\\.Services|Set(Soul|Memory|Skill|MCP|EventStream|Workspace|Decision|Approval|Result|Orchestrator|Run)Service|\\.(Soul|Memory|Skill|MCP|EventStream|Workspace|Decision|Approval|Result|Orchestrator|Run)Service\\(\\)' internal application -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg -n '\\.Repositories\\.(Workspaces|Goals|DoDs|Runs|Steps|RuntimeCheckpoints|Tickets|Resolutions|Approvals|Results|Artifacts|Souls|Skills|MCPServers|MCPCapabilities|MemoryItems|MemoryRecallTraces|ArtifactContinuations|EventRecords|ToolCalls|Patches|ValidationResults|AutoGovernanceTraces)\\.(Get|List|ListByWorkspaceID|ListByRun|ListByWorkspace|ListByResult|GetByTicketID|Create|Update|Delete|Upsert)' internal application -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg -n 'github.com/muidea/workorch/internal/services/workorch/(biz|service)' application/workorch/main.go application/workorch -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg -n 'internal/modules/application/workorchd\"|workorchserver|Done\\(\\)' internal/services/workorch application/workorch -g '*.go' --glob '!**/*_test.go'` returns only the explicit `application/workorch/main.go` side-effect import for enabling `workorchd`; server done waiting uses EventHub command.
  - `rg -n 'TopicWorkorchDCommandDone|PayloadKeyDoneChannel' internal application -g '*.go'` shows app publisher, workorchd module subscriber/response, and module regression coverage.
  - `rg -n 'internal/modules/application/workorchd/biz/appservice|appservice\\.' internal/services/workorch application/workorch -g '*.go' --glob '!**/*_test.go'` returns no production matches.
  - `rg -n 'NewEmbeddedEventHub|NewRuntimeSessionFromInitiator|RuntimeSession' internal/services/workorch internal/services/workorch/biz/backends -g '*.go' --glob '!**/*_test.go'` shows app uses `backend.NewEmbeddedEventHub`, while no runtime session construction remains in the workorch backend production path.
  - `rg -n 'session\\.Borrow\\(\\)' internal/services/workorch/biz/backends -g '*.go' -C 1` returns no production matches after deleting the appservice-backed local backend.
  - `rg -n '\\bgo func\\b|Subscribe\\(|Unsubscribe\\(|BackgroundRoutine|task\\.' internal/modules internal/initiators application -g '*.go' --glob '!**/*_test.go'` was reviewed for long-lived work. No new module-level untracked background worker was found; request-local fan-out waits before returning, and HTTP continuation work uses `scheduleBackground`.
  - `rg -n 'NewMagicHub\\(|event\\.NewHub|task\\.NewBackgroundRoutine|BackgroundRoutine\\(|AsyncFunction\\(|AsyncTask\\(|Timer\\(' internal application -g '*.go' --glob '!**/*_test.go'` shows application modules reuse framework-provided EventHub/BackgroundRoutine; standalone hub creation is limited to the shared events wrapper constructor.
  - `rg -n 'if len\\(m\\.unsub\\) > 0|if m\\.executor != nil|if len\\(e\\.unsub\\) > 0|if d\\.unsub != nil' internal/modules/blocks internal/modules/kernel internal/modules/application/workorchd/biz/appservice -g '*.go' --glob '!**/*_test.go'` shows duplicate-run guards in EventHub command owner modules, block executors, shared runtime executor, and the workorchd CLI dispatcher.
  - `rg -n 'agent \\[goal\\]|RunWithBackend|go func' internal/services/workorch/service/cli internal/services/workorch/biz/agent -g '*.go'` shows the CLI agent command is the only command-local goroutine around agent execution; it now captures the returned error and uses a cancelable context.
  - `rg -n 'Publish\\([^\\n]*events\\.Envelope|Topic[A-Za-z0-9]+Command|RuntimeBlockExecuteTopic|ResponseEnvelope\\(' internal application -g '*.go' --glob '!**/*_test.go'` and `rg -n '_\\s*=\\s*[^\\n]*\\.Publish\\(|if err := [^\\n]*\\.Publish\\([^\\n]*; err != nil|err := [^\\n]*\\.Publish\\(' internal application -g '*.go' --glob '!**/*_test.go'` were reviewed for command response handling. The only missing-response-as-success helper found was `publishActivationRunCommand`, now fixed and covered.
  - `rg -n 'type plugin|newPlugin|pluginmodule\\.Register\\(newPlugin\\(\\)\\)|module \\*Module|p\\.module' internal/modules -g 'plugin.go'` returns no matches after integrating `plugin.go` into concrete module lifecycle implementations.
  - Remaining appservice direct repository writes are limited to application-owned workspace templates, not kernel formal owner state.
- Latest verification:
  - `go test ./... -count=1` passed after removing the `workorchd` package-level module singleton and `Done()` helper.
  - `WORKORCH_BASE_URL=http://127.0.0.1:8080 make verify-ci-mvp-file` passed against a local file-backend service started by `make run-file` after the latest module registration cleanup. HTTP, workspace template/soul, LLM workspace execution, and MCP stdio workspace execution passed; MCP stdio completed `workspaceId=ws-30f622a59dbb6ce1`, `runId=run-001`, `runState=completed`, `steps=4`, `toolCalls=4`, `validations=4`, `runtimeCheckpoints=11`.
  - `go test ./internal/modules/application/workorchd/biz/bootstrap -count=1` passed after the `plugin.go` integration and embedded runtime lifecycle rename.
  - `go test ./internal/modules/application/workorchd/biz/appservice -count=1` passed after the `plugin.go` integration and embedded runtime lifecycle rename.
  - `go test ./internal/initiators/... ./internal/modules/... ./internal/pkg/... -count=1` passed after the `plugin.go` integration and embedded runtime lifecycle rename.
  - `go test ./application/workorch -count=1` passed after the `plugin.go` integration and embedded runtime lifecycle rename.
  - `git diff --check` passed after the `plugin.go` integration and embedded runtime lifecycle rename.
  - `WORKORCH_BASE_URL=http://127.0.0.1:8080 make verify-ci-mvp-file` passed against a local file-backend service started by `make run-file` after the `plugin.go` integration. HTTP, workspace template/soul, LLM workspace execution, and MCP stdio workspace execution passed; MCP stdio completed `workspaceId=ws-38df869bf58aa925`, `runId=run-001`, `runState=completed`, `steps=4`, `toolCalls=4`, `validations=4`, `runtimeCheckpoints=11`.
  - `git diff --check` passed after the `plugin.go` integration e2e verification.
  - `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestPublishActivationRunCommandRequiresResponse|TestActivateWorkspaceExecution' -count=1` passed after fixing activation run command missing-response handling.
  - `go test ./internal/modules/application/workorchd/biz/appservice -count=1` passed after the command response audit update.
  - `go test ./internal/initiators/... ./internal/modules/... ./internal/pkg/... -count=1` passed after the command response audit update.
  - `rg -n "\bevents\.Topic[A-Za-z0-9]+" internal application -g '*.go'` returns no matches after owner event package migration.
  - `rg -n "Topic[A-Za-z0-9]+\s*=\s*\"/" internal/pkg/events -g '*.go'` returns no matches after removing business topic definitions and aliases from the shared EventHub package.
  - `go test ./... -count=1` passed after the owner-owned event definition and contract metadata split.
  - `make verify-managed-full` passed with an automatically managed WorkOrch service at `http://127.0.0.1:58166`. The run covered fmt-check, console build, vet, full Go tests, HTTP/workspace/template/soul checks, LLM workspace execution, incremental artifact e2e, MCP stdio, SkillHub execution, auto-governance ticket/approval, software-dev goal/DoD, software requirement development orchestration, workspace file/diff/patch, web search, weather governance mock, TUI agent acceptance, and TUI long-output e2e.
  - `go test ./application/workorch -count=1` passed after the command response audit update.
  - `WORKORCH_BASE_URL=http://127.0.0.1:8080 make verify-ci-mvp-file` passed against a local file-backend service started by `make run-file` after the command response audit update. HTTP, workspace template/soul, LLM workspace execution, and MCP stdio workspace execution passed; MCP stdio completed `workspaceId=ws-2196db8036940a76`, `runId=run-001`, `runState=completed`, `steps=4`, `toolCalls=4`, `validations=4`, `runtimeCheckpoints=11`.
  - `git diff --check` passed after the command response audit update.
  - `go test ./internal/modules/blocks/pkg/runtimeevent ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/result -count=1` passed after adding duplicate-`Run()` subscription guards and regression coverage for runtime executor, CLI dispatcher, and representative result owner module.
  - `go test ./internal/services/workorch/service/cli ./internal/services/workorch/biz/agent -count=1` passed after fixing CLI agent error propagation and adding regression coverage for backend failure before event emission.
  - `go test ./internal/services/workorch/biz/backends ./internal/modules/application/workorchd/biz/appservice ./internal/pkg/events -count=1` passed after removing the unmanaged CLI result fallback goroutine and rechecking EventHub command/result behavior.
  - `go test ./internal/services/workorch/biz/backends ./internal/modules/application/workorchd -count=1` passed after moving `/cli/command` handling from the workorch backend into the workorchd owner dispatcher.
  - `go test ./application/workorch ./internal/services/workorch ./internal/services/workorch/biz/backends ./internal/modules/application/workorchd ./internal/pkg/events -count=1` passed after deleting the appservice-backed local backend and keeping workorch embedded mode on EventHub command/result transport.
  - `go test ./internal/initiators/... ./internal/modules/... ./internal/pkg/... -count=1` passed after the CLI dispatcher migration and local backend removal.
  - `git diff --check` passed after the CLI dispatcher migration and documentation update.
  - `WORKORCH_BASE_URL=http://127.0.0.1:8080 make verify-ci-mvp-file` passed against a local file-backend service started by `make run-file` after the CLI dispatcher migration. HTTP, workspace template/soul, LLM workspace execution, and MCP stdio workspace execution passed; MCP stdio completed `workspaceId=ws-8d2d9b2560111449`, `runId=run-001`, `runState=completed`, `steps=4`, `toolCalls=4`, `validations=4`, `runtimeCheckpoints=11`.
  - `go test ./application/workorch ./internal/services/workorch ./internal/services/workorch/biz/backends -count=1` passed after moving embedded RuntimeSession creation/close from app into the backend facade.
  - `go test ./application/workorch ./internal/services/workorch ./internal/modules/application/workorchd ./internal/pkg/events -count=1` passed after moving workorchd server done waiting behind an EventHub command and adding module command coverage.
  - `go test ./application/workorch ./internal/services/workorch/... -count=1` passed after removing production `main` test helpers and moving CLI/TUI embedded command handling to workorchd.
  - `go test ./internal/initiators/... ./internal/modules/... ./internal/pkg/... -count=1` passed after the CLI/TUI EventHub dispatcher boundary cleanup.
  - `go test ./internal/pkg/workorchrepo ./internal/pkg/workorchdefaults ./internal/pkg/workorchruntimepolicy ./internal/initiators/workorchruntime ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server ./application/workorch -count=1` passed after moving shared bootstrap helpers out of application bootstrap.
  - `go test ./internal/initiators/... ./internal/modules/... ./internal/pkg/... -count=1` passed after the latest shared-helper package-boundary cleanup.
  - `make verify-ci-mvp-file` passed against a clean local file-backend service started by `make run-file` after the latest shared-helper package-boundary cleanup. HTTP, workspace template/soul, LLM workspace execution, and MCP stdio workspace execution all passed; the MCP stdio e2e completed `run-001` with 4 steps, 4 tool calls, 4 validations, and 11 runtime checkpoints.
  - `go test ./internal/modules/kernel/orchestrator ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice ./internal/modules/blocks/llm ./internal/modules/blocks/llm/biz ./internal/modules/blocks/llm/pkg/providerconfig ./internal/modules/blocks/web ./internal/modules/blocks/web/biz ./internal/modules/blocks/web/pkg/providerconfig ./internal/initiators/workorchruntime -count=1` passed after moving runtime policy and provider config helpers to `pkg` boundaries and removing remaining application/initiator direct `biz` imports.
  - `go test ./internal/initiators/... ./internal/modules/... ./internal/pkg/... -count=1` passed after the package-boundary cleanup.
  - `make verify-ci-mvp-file` passed against a local file-backend service started by `make run-file` after the package-boundary cleanup; HTTP, workspace template/soul, LLM workspace execution, and MCP stdio workspace execution all passed.
  - `go test ./internal/modules/blocks/llm ./internal/modules/application/workorchd/biz/appservice -count=1` passed after routing workspace setup draft completion and semantic memory embedding generation through LLM EventHub commands.
  - `go test ./internal/initiators/... ./internal/modules/... ./internal/pkg/... -count=1` passed after the latest EventHub command boundary closure and git capability test isolation.
  - `make verify-ci-mvp-file` passed against a clean local file-backend service started by `make run-file` after the latest LLM command boundary closure, including HTTP, workspace template/soul, LLM workspace execution, and MCP stdio workspace execution.
  - `go test ./internal/modules/kernel/eventstream ./internal/modules/kernel/orchestrator ./internal/modules/kernel/governance -count=1 -timeout=240s` passed after adding EventStream/Orchestrator/Governance read command topics.
  - `go test ./internal/modules/application/workorchd/biz/appservice -count=1 -timeout=240s` passed after replacing read-model EventRecord/ToolCall/Patch/ValidationResult/ArtifactContinuation/AutoGovernanceTrace repository access with EventHub-backed readers.
  - `go test ./internal/initiators/workorchruntime ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/service/server -count=1 -timeout=300s` passed after removing runtime/bootstrap service facades.
  - `go test ./internal/modules/kernel/... ./internal/modules/blocks/... -count=1 -timeout=300s` passed after the plugin facade cleanup.
  - `go test ./application/workorch -count=1 -timeout=240s` passed after the appservice boundary closure.
  - `go test ./internal/modules/kernel/run ./internal/modules/application/workorchd/biz/appservice -count=1 -timeout=300s` passed after adding the scoped run owner command guard.
  - `go test ./internal/modules/kernel/eventstream ./internal/modules/kernel/eventstream/biz -count=1 -timeout=120s` passed after adding EventStream duplicate start/stop regression coverage.
  - `go test ./internal/services/workorch/service/tui -count=1 -timeout=240s` passed after adding TUI active-workspace run scope regression coverage.
  - `go test ./internal/modules/kernel/governance/biz -count=1 -timeout=240s` passed after adding auto-governance event envelope scope contract coverage.
  - `go test ./internal/modules/kernel/orchestrator ./internal/modules/kernel/result -count=1 -timeout=240s` passed after adding Result/Artifact EventHub adapter scope and missing-response contract coverage.
  - `go test ./internal/modules/kernel/orchestrator/biz -run 'CompleteRun|CompletesLLMRunWithSatisfiedDoDAssessment|CompletionGate' -count=1 -timeout=240s` passed after adding the direct CompleteRun accepted-result gate.
  - `go test ./internal/modules/blocks/web ./internal/modules/blocks/web/biz ./internal/modules/blocks/mcp ./internal/modules/blocks/mcp/biz -count=1 -timeout=240s` passed after adding Web approval owner command run-scope coverage.
  - `go test ./internal/modules/blocks/file ./internal/modules/blocks/file/biz ./internal/modules/blocks/git ./internal/modules/blocks/git/biz ./internal/modules/kernel/orchestrator/biz -run 'Patch|ArtifactContinuation|Git' -count=1 -timeout=300s` passed after patch/write boundary review.
  - `go test ./... -count=1 -timeout=600s` passed after runtime/bootstrap service facade removal. A prior concurrent full run overlapped another `application/workorch` package run and produced a server-mode test environment race; the package and full suite both passed when run without overlapping the same integration test.
  - `go build -o ./bin/workorch ./application/workorch` passed.
  - `git diff --check` passed.
  - `make verify-ci-mvp-file` passed against a clean deployed local file-backend service started by `make run-file`, including HTTP, workspace template/soul, LLM workspace execution, and MCP stdio workspace execution.

## Audit Records

### application/workorch entry and process service runtime

- Code: `application/workorch/main.go`, `internal/services/workorch`
- Owner layer: Application entry / process service runtime
- Expected responsibility: `main` 负责信号上下文、按进程入口启用 framework initiator/modules，并调用 `workorch.Run`；process service 负责解析启动模式、配置目录、调用 `application.Startup` / `Run` / `Shutdown`，并在 local interactive/server 模式内复用同一 framework runtime。
- Current responsibility: 已对齐参考 `magicRunner` 入口：framework side-effect imports 位于 `application/workorch/main.go`，`app` 包不再隐式注册 modules；remote 模式走 remote backend，不启动本地 framework runtime；local 模式统一使用 `service.DefaultService()` 启动已注册 initiator/module。
- Lifecycle check: local runtime 在 `application.Startup` 成功后 defer `shutdownApplication`，再调用 `application.Run`；server 模式通过 EventHub `TopicWorkorchDCommandDone` 查询 workorchd module 实例的 done channel，或等待进程 context 取消；interactive 模式创建 embedded EventHub backend，并由 workorchd CLI dispatcher 借用 request-scoped `RuntimeSession`。
- EventHub check: process service 不直接创建独立 EventHub；interactive backend 通过 runtime initiator 暴露的 framework EventHub 发布 CLI command/result envelope。
- Background task check: process service 不直接启动长期任务；长期任务由 framework service/module/initiator 接管。
- State ownership check: process service 仅做 workspace 默认选择和 CLI flow 编排，不直接裁决 Run 状态机。
- Workspace scope check: 默认 workspace 解析只用于 CLI 初始上下文；TUI run continuation / stale run recovery 路径通过 `ContextWithRuntimeWorkspaceID` 将 active workspace 传给 backend `Run.Get` / `Run.List` / `Run.Resume`，backend 再交给 appservice owner command scope guard。
- Findings: framework module 注册曾放在 `app` 包，导致应用包带 side-effect 注册；已上移到 `main` 入口按进程启用。TUI active workspace run scope 已补回归测试，覆盖切换 workspace 后继续执行不会误用其他 workspace 的同名/旧 run。
- Required action: 后续新增 TUI/backend run 操作必须继续绑定 active workspace context，不能只传 workspace-local `runID`。
- Verification: `go test ./application/workorch ./internal/services/workorch -count=1`; `go test ./internal/services/workorch/service/tui -count=1 -timeout=240s`
- Status: reviewed

### workorch process service lifecycle

- Code: `internal/services/workorch`
- Owner layer: Process service
- Expected responsibility: 作为 `framework/service.LifecycleService` 风格的进程级 service 管理 `remote` / `server` / `interactive` 模式，接入 `framework/application`，但不注册为 plugin module。
- Current responsibility: `Startup` 解析选项和配置；local 模式启动 framework application；remote / embedded backend 由 process service 选择并传给 CLI/TUI/agent；`Shutdown` 负责释放 process service 持有的 runtime。
- Lifecycle check: `Startup` / `Run` / `Shutdown` 均为进程级生命周期路径；plugin module 启用仍由 `application/workorch/main.go` 的 side-effect imports 决定。
- EventHub check: process service 只通过 embedded backend 使用 runtime initiator 暴露的 EventHub command/result transport，不直接访问 owner service/repository。
- Background task check: process service 不启动长期后台任务；长期任务由 framework service/module/initiator 接管。
- State ownership check: 不持有或写入业务状态。
- Workspace scope check: 默认 workspace/session 只通过 `workorchport.Backend` 查询或创建；run/ticket 等 workspace-local ID 必须由调用路径传递 workspace scope 或由服务端唯一 open 对象解析。
- Findings: 原 `internal/modules/application/workorch` 不是 plugin module，已迁移到 `internal/services/workorch`。
- Required action: 后续新增 process service 能力不得放回 `internal/modules`；如需要正式 plugin lifecycle，应新建真正的 module/block/initiator 并通过入口显式启用。
- Verification: `go test ./internal/services/workorch/... -count=1`
- Status: reviewed

### workorchd plugin lifecycle

- Code: `internal/modules/application/workorchd`
- Owner layer: Application HTTP plugin module
- Expected responsibility: 从配置构造 HTTP service，将 framework `event.Hub` / `BackgroundRoutine` 传给 service，并在 `Teardown` 关闭 service。
- Current responsibility: 符合 plugin module 生命周期；本轮已在 service 层完成 HTTP server background task 收口，并将 continuation/recovery 去重调度抽入 `runContinuationScheduler`。
- Lifecycle check: `Setup` 解析配置并调用 `service.Startup`；`Run` 要求 service 已配置；`Teardown` 调用 `service.Shutdown` 并清空 service 引用。
- EventHub check: `Setup` 将 framework `event.Hub` 传入 HTTP service，service 再包装为 owner-scoped MagicHub。
- Background task check: `Setup` 将 framework `BackgroundRoutine` 传入 HTTP service；service `Run` 已优先通过 `AsyncFunction` 启动 listener。
- State ownership check: plugin 不直接写状态。
- Workspace scope check: plugin 不直接查询 run/workspace。
- Findings: service 层曾绕过 `BackgroundRoutine` 启动 listener，已收口；serverState 中散落的 governance/replan continuation 去重状态已收敛到 server-local scheduler。
- Required action: 已完成 continuation/recovery 调度边界核对；后续新增后台调度必须继续经 framework `BackgroundRoutine` 或 `scheduleBackground`。
- Verification: `go test ./internal/modules/application/workorchd/... -count=1 -timeout=300s`
- Status: reviewed

### workorchruntime initiator lifecycle

- Code: `internal/initiators/workorchruntime`
- Owner layer: Initiator / runtime assembly
- Expected responsibility: 提供 repository set、framework EventHub adapter、runtime policy 等窄装配能力；不承担具体业务状态机裁决，也不作为跨模块 service registry。
- Current responsibility: `Setup` 读取配置、构造 repository set、包装 framework EventHub、生成 runtime policy；kernel/block modules 只从 runtime helper 获取仓储、EventHub 和 policy，跨 owner 交互通过 EventHub command adapter 完成。
- Lifecycle check: `Setup` fail-fast；`Run` 只确保内置默认数据；`Teardown` 清理 repository、event hub 和 policy。
- EventHub check: 使用 `workorchevents.NewMagicHubWithHubOwner(eventHub, common.InitiatorID)` 复用 framework hub，未创建第二套 hub。
- Background task check: initiator 不启动后台任务；目前未保存 `BackgroundRoutine`。
- State ownership check: initiator 不直接推进 Run 状态机；内置默认数据初始化属于启动基础数据准备。
- Workspace scope check: repository set 对 file backend 使用 workspace-scoped repository set；后续仍需在 repository 层逐项核对 run id scope。
- Findings: runtime helper 曾暴露 `SetXService/XService` service facade，形成潜在跨 owner 直接调用入口；已移除 service getter/setter 和 bootstrap `ServiceExports`。RuntimePolicy 类型已移入 `kernel/orchestrator/pkg/models`，initiator helper 不再因策略类型 import orchestrator `biz`。
- Required action: 已修复并补回归测试；后续新增跨 owner 能力时只能通过 EventHub command 或 initiator 窄接口接线。
- Verification: `go test ./internal/initiators/workorchruntime -count=1`; `go test ./internal/modules/kernel/orchestrator ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice ./internal/modules/blocks/llm ./internal/modules/blocks/llm/biz ./internal/modules/blocks/llm/pkg/providerconfig ./internal/modules/blocks/web ./internal/modules/blocks/web/biz ./internal/modules/blocks/web/pkg/providerconfig ./internal/initiators/workorchruntime -count=1`
- Status: reviewed

### app bootstrap runtime assembly

- Code: `internal/modules/application/workorchd/biz/bootstrap`
- Owner layer: Runtime assembly / in-process bootstrap
- Expected responsibility: 组装 repository、kernel modules、block adapters、runtime registry 和 shared EventHub；不成为新的业务状态机拥有者或跨模块 service registry。
- Current responsibility: 构造 memory/file repository set，按依赖顺序 setup modules，通过显式 `runtimeModuleLifecycle` 运行内部 modules，并仅导出 repository/infrastructure。
- Lifecycle check: bootstrap runtime 是 appservice 内部组装层，不再使用 `magicCommon/framework/plugin/common`；framework plugin/module 只在 application main 启动链路按需启用。发现 `Runtime.Shutdown` 和内部 lifecycle shutdown 曾在首个 teardown 错误处提前返回，可能跳过后续 modules 和 EventHub terminate；已改为继续执行完整 teardown、清空 modules，并用 `errors.Join` 返回聚合错误。
- EventHub check: 调用方传入 EventHub 时复用；未传入时只为独立 bootstrap runtime 创建 owner-scoped MagicHub。
- Background task check: bootstrap 本身不启动长期 background task；module `Run` 由 lifecycle 统一触发。
- State ownership check: bootstrap 只负责组装和内置默认数据，不直接裁决 Run/Result 状态。
- Workspace scope check: file backend 使用 workspace-scoped repository set；具体 run identity 在 repository 层继续核对。
- Findings: bootstrap 内部 runtime 曾复用 framework plugin manager，职责边界不清；shutdown 早退也会破坏内部资源释放语义。本轮将 runtime policy 类型改为 `orchestrator/pkg/models`，并通过 orchestrator module 公共构造函数组装局部 runtime registry / patch adapter，bootstrap 不再直接 import orchestrator `biz`。
- Required action: 已移除 bootstrap 内部 plugin manager，改为显式 runtime module lifecycle；已补充 lifecycle/runtime shutdown regression tests。
- Verification: `go test ./internal/modules/application/workorchd/biz/bootstrap -count=1`; `rg -n 'internal/modules/(blocks|kernel)/.*/biz' internal/modules/application internal/initiators -g '*.go' --glob '!**/*_test.go'`
- Status: reviewed

### RuntimeSession request scope

- Code: `internal/modules/application/workorchd/biz/appservice/runtime_session.go`
- Owner layer: Application use-case runtime access
- Expected responsibility: 管理 request-scoped runtime access、borrowed session 和 bootstrap runtime close；不拥有正式业务状态。
- Current responsibility: 支持从 bootstrap runtime、initiator exports、显式 backend/eventHub 创建 session；`Close` 区分 owned 和 borrowed runtime。
- Lifecycle check: owned session close 会关闭 managed runtime 并 shutdown bootstrap；borrowed session close 只释放 request wrapper，不关闭长生命周期 runtime。
- EventHub check: `NewRuntimeSessionWithBackendAndEventHub` 接受 context/runtime 传入的 EventHub；未发现创建第二套 framework hub 的问题。
- Background task check: session 不启动后台任务。
- State ownership check: session 只包装 exports/service access，不直接推进状态机。
- Workspace scope check: repository backend/data root 元信息由 session 保存；具体 repository scope 后续在 repository 层核对。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 已核对 appservice session scope：`WithBackend` / `FromEnv` 包装器创建 owned `RuntimeSession` 后交给对应 `WithSession`，由 `CloseOnReturn` 关闭；CLI interactive session defer close；HTTP shared session 只由 `serverState` 持有，并在 shutdown 关闭，单次操作通过 `Borrow()` 后关闭 borrowed session。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -run RuntimeSession -count=1`
- Status: reviewed

### events MagicHub framework adapter

- Code: `internal/pkg/events`
- Owner layer: Shared event infrastructure
- Expected responsibility: 将 WorkOrch `Hub` 映射到 magicCommon `event.Hub`，同步 `Publish` 使用 `Send`，异步 `PublishAsync` 使用 `Post`，并保持 scoped lane ordering 和 unsubscribe 释放。
- Current responsibility: `MagicHub.Publish` 使用 `Send` 并返回 handler response/error；`PublishAsync` 使用 `Post`；lane 由 owner event contract 显式计算，run owner 写入在有 workspace scope 时使用 `state/run/<workspaceID>/<runID>`，orchestrator 执行推进使用 `state/orchestrator/workspace/<workspaceID>/run/<runID>`，owner read/query 使用 query lane，block execution 使用 execute/resource lane，全局事件 fallback 到 topic lane；同一 raw hub 的 wrapper 共享 workspace revision state。
- Lifecycle check: owner-created MagicHub 支持 `Terminate`；wrapped framework hub 不由 adapter 关闭。
- EventHub check: 已有测试覆盖 Send/Post 行为、run/workspace scoped lane、topic fallback、shared wrapper revision、multi-wrapper delivery、wildcard subscription 和 unsubscribe。
- Background task check: adapter 不启动业务后台任务；unsubscribe 为避免底层阻塞使用 bounded wait。
- State ownership check: 只记录 event envelope 和 revision，不裁决业务状态。
- Workspace scope check: revision 仍按 WorkspaceID scoped；涉及 workspace-local `runID` / `stepID` 的 state/query/execute lane 在有 WorkspaceID 时必须带 workspace scope；缺失 WorkspaceID 的全局事件 revision 为 0。
- Findings: 同 workspace lane 曾导致 run command handler 内同步发布 workspace command 时发生嵌套等待；已通过显式 owner contract 拆分 state/query/execute lane 收口。随后发现不同 workspace 的 `run-001` 会因 runID-only state lane 被错误串行，已改为 workspace-scoped owner state lane；继续发现 orchestrator 与 run owner 共用 `state/run` 会在 orchestrator handler 内同步回写 run owner 时自锁，已拆为 `state/orchestrator/...` 与 `state/run/...`。
- Required action: 已核对主要 workspace-scoped 事件发布方：run/workspace/decision/approval/governance/orchestrator/memory/result/artifact 事件均携带对应 `WorkspaceID` / `RunID` / `StepID`；本轮补强 Decision/Approval owner command 的 scope guard，调用方携带 workspace/run/step scope 时由 owner module 强制校验。
- Verification: `go test ./internal/pkg/events -count=1`
- Status: reviewed

### kernel/run state machine facade

- Code: `internal/modules/kernel/run`
- Owner layer: Kernel formal run state facade
- Expected responsibility: 持有 Run / Step primitives 的正式 service 门面，公开创建、启动、阻塞、恢复、验证、完成、取消、替换、失败等状态转换入口；复杂执行推进交给 orchestrator。
- Current responsibility: `run.Service` 负责参数校验、幂等 ensure 语义和事件发布；执行推进仍委托 orchestrator。生产 module 只直接持有自身 Run repository；workspace 校验通过 workspace owner command reader 完成。
- Lifecycle check: plugin/module `Setup` 接线 Run repository、EventHub 和 workspace EventHub reader；`Run` 订阅 run command topics；`Teardown` 取消订阅并清空引用。
- EventHub check: Run service 状态变更通过 shared EventHub 发布 run events；跨 owner workspace 读取、governance resume 和 appservice/orchestrator command 均通过同步 EventHub command response。
- Background task check: 不启动后台任务。
- State ownership check: 本轮修复 public `ResumeRun` / `EnsureResumed` 边界：普通 `running` run 不再被 run service 当作可 resume 状态；blocked states 继续委托 orchestrator；已加载 `LoadedRun` 的内部 recovery 路径仍可由 appservice/orchestrator 使用。
- Workspace scope check: run command payload 携带 workspace/run refs；workspace-sensitive run lookup 后续仍需在 repository contract 层评估 `GetByWorkspaceID`。
- Findings: `ResumeRun` 曾允许 `running` run 透传到 orchestrator recovery 分支，导致 public resume API 对非 blocked 状态返回成功；生产 module 还曾直接注入 Workspace repository 进行跨 owner 校验。
- Required action: 已修复 public resume gate；已移除生产 Run module 对 Workspace repository 的直接注入，改为 `/workspace/command/get` EventHub reader。
- Verification: `go test ./internal/modules/kernel/run/... -count=1`; `go test ./internal/modules/kernel/run ./internal/modules/kernel/workspace ./internal/modules/application/workorchd/biz/bootstrap -count=1`
- Status: reviewed

### kernel/orchestrator execution state authority

- Code: `internal/modules/kernel/orchestrator`
- Owner layer: Kernel execution orchestrator
- Expected responsibility: 作为 dispatch、repair、replan、completion gate、runtime recovery 的唯一执行推进核心。
- Current responsibility: 保留 blocked resume、running recovery、terminal step recovery、initial dispatch recovery、replan continuation 等核心推进逻辑；生产装配中对 run/workspace/memory/result/artifact、decision/resolution/approval 和 block runtime execution 的跨 owner 访问均通过 EventHub-backed adapter。orchestrator 自身仍持有 Steps、ToolCalls、Patches、ValidationResults、RuntimeCheckpoints、ArtifactContinuations、MemoryRecallTraces 这组执行证据 primitive。
- Lifecycle check: module `Setup` 只接线 runtime adapter registry、orchestrator-owned evidence repositories 和 EventHub command adapters；`Run` 订阅 orchestrator command topics，包括 `/orchestrator/command/purge-run` 管理清理命令；`Teardown` 取消订阅。
- EventHub check: run/workspace 状态变化通过 event publisher 发布，保持 workspace-scoped event payload；run facade 调 orchestrator、orchestrator 调 block/run/workspace/memory/result/artifact/decision/approval 均要求同步 command response。Result/Artifact closeout 通过 `resultEventResultRepository` / `resultEventArtifactRepository` 发布 result owner command，缺失 `events.Response` 时失败。
- Background task check: orchestrator 不直接持有后台 scheduler；调用方负责触发。
- State ownership check: orchestrator 继续允许内部 `running` recovery，这是恢复执行状态的一部分；public running resume 已在 run service facade 层收窄。Direct `CompleteRun` 现在对 DoD-gated LLM workspace run 要求同 run 已存在匹配 workspace/run/Goal/DoD、`SourceType=dod_assessment`、`DoDAssessment.Satisfied=true` 且带 evidenceRefs 的 accepted Result，不能绕过 acceptance/result closeout 直接完成。
- Workspace scope check: EventHub command adapters 带 workspace/run refs；run id scope 后续在 repository 层继续核对。
- Findings: 曾在生产 module 装配中直接注入 Run/Workspace/Memory、Result/Artifact 和 Decision/Approval owner repositories、直接注册 block runtime adapter；已改为 EventHub command adapter。appservice workspace purge 曾直接删除 run 证据仓储；本轮已改为发布 orchestrator owner 的 `/orchestrator/command/purge-run`。Artifact command adapter 曾统一写入 `SubjectType=result`，已修正为 Artifact create/get/update/delete 使用 `artifact` subject，list 命令使用其查询对象 subject。Direct `CompleteRun` 曾可用任意 accepted Result 将 DoD-gated run 标记为 completed，已收紧为正式 DoD assessment accepted result gate。
- Required action: completion gate 与 `runtime-stage-contract.md` 中 accepted Result / DoD assessment / evidenceRefs 的核心完成门禁已核对并补回归测试；当前 doc-ahead 差异仍是 CoverageWorkItem 独立对象化与完整 context-gate action 枚举。当前执行证据 primitive 明确归属 orchestrator owner，如后续拆分独立 evidence module，仍必须经 EventHub command 接入。
- Verification: `go test ./internal/modules/kernel/orchestrator/... -count=1`; `go test ./internal/modules/kernel/orchestrator ./internal/modules/kernel/result -count=1 -timeout=240s`; `go test ./internal/modules/kernel/orchestrator/biz -run 'CompleteRun|CompletesLLMRunWithSatisfiedDoDAssessment|CompletionGate|ReusesValidatedDoDAssessment' -count=1 -timeout=240s`; `go test ./internal/modules/kernel/orchestrator/biz -count=1 -timeout=360s`; `go test ./internal/initiators/workorchruntime ./internal/modules/blocks/... ./internal/modules/kernel/... ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/biz/bootstrap -count=1`
- Status: reviewed

### kernel/governance resume boundary

- Code: `internal/modules/kernel/governance`
- Owner layer: Kernel governance automation
- Expected responsibility: 自动治理、ticket/approval resolution 和 resume trigger；不替代 Run 状态机。
- Current responsibility: governance 通过 EventHub command 读取 ticket/resolution/approval/run/goal/dod/memory owner state，并通过 EventHub command 触发 decision resolve、approval approve 和 run resume；自身只持有 AutoGovernanceTrace owner repository 和自动治理工作队列。
- Lifecycle check: `Setup` 只接线 EventHub readers/callbacks 和 trace repository；`Run` 订阅 ticket/approval/run-state events，启动可取消 worker 与 reconciliation timer；`Teardown`/`Stop` 释放订阅、取消 timer/worker 并等待退出。
- EventHub check: ticket/approval/run-state 通知 handler 只做轻量入队；治理处理在 worker 中执行，读 owner state 和触发状态转换均使用同步 command response，避免在 EventHub dispatch stack 内嵌套长流程。
- Background task check: framework plugin 路径将 `task.BackgroundRoutine` 注入 governance service，worker 和 reconciliation timer 优先通过 `BackgroundRoutine.AsyncFunction` 调度；独立 bootstrap/test 路径无 framework routine 时才 fallback 到受 context/done channel 管理的局部 goroutine。`Stop` 取消并等待 worker/timer 退出。
- State ownership check: 本轮 `EnsureResumed` 收窄后，governance 只能恢复 blocked run，不会把 running run 当作可恢复状态。
- Workspace scope check: 后续结合 repository scope 核对 governance traces 与 run/workspace 查询。
- Findings: 曾在生产 module 装配中直接注入 ticket/resolution/approval/run/goal/dod/memory repositories；已替换为 EventHub reader/callback，生产模块交互不再直连这些 owner repositories。治理 observer 曾在 EventHub handler 内同步执行 auto-resolve/resume，导致嵌套 command `Send` 可能阻塞原始分发栈；已改为异步入队。
- Required action: 已补 auto-governance evaluated/resolved/escalated/blocked event contract tests，验证 envelope 携带 `WorkspaceID` / `RunID` / `StepID` 且 payload 可解码；治理 worker 已接入 framework `BackgroundRoutine`，后续不得回退到 observer 内同步重活。
- Verification: `go test ./internal/modules/kernel/governance/... -count=1`
- Status: reviewed

### appservice activation use case

- Code: `internal/modules/application/workorchd/biz/appservice/workspace_execution_activation_service.go`
- Owner layer: Application use-case orchestration
- Expected responsibility: 组合 workspace、goal、DoD、selection context 和 run creation/start；正式 Run 状态转换通过 kernel run service 完成。
- Current responsibility: 根据 workspace 当前定义和命令选择 Goal/DoD/Soul/Skill/MCP/Memory，构建 executionContext；通过 run owner command 创建 Run 记录，通过 orchestrator owner command 触发 supersede/start/fail 执行推进。
- Lifecycle check: appservice 不注册 framework module，不持有长期生命周期资源；session 通过 `CloseOnReturn` 释放。
- EventHub check: 通过 session/context 传入统一 EventHub，activation 对 Run 记录创建发布 run owner command，对 supersede/start/fail 发布 orchestrator owner command，所有同步命令均要求 `events.Response`；创建/启动 run 的正式事件由 kernel run/orchestrator owner 发布。
- Background task check: 不启动长期后台任务；activation 是单次 use-case。
- State ownership check: active run stale replacement 的 supersede、run start/fail 均通过 orchestrator owner command；新 run 创建仍通过 run owner command handler；未发现 application 直接写 Run 终态的越界逻辑。
- Workspace scope check: activation 开始即读取 workspace 并写入 runtime workspace context，后续 run repository lookup 使用 scoped context。
- Findings: activation 曾直接调用 `RunService` facade；已改为 EventHub command，避免 application use-case 直接触达 run owner state machine facade。本轮 command response 审计发现 `publishActivationRunCommand` 在缺少 `events.Response` 且 `Publish` 无错误时会误返回成功，已改为返回 missing-response 错误。
- Required action: 后续继续核对 selection context 中 Skill/MCP/Memory 自动绑定策略是否应进一步拆入专门 selector service。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -run 'TestPublishActivationRunCommandRequiresResponse|TestActivateWorkspaceExecution' -count=1`; `go test ./internal/modules/application/workorchd/biz/appservice -count=1`
- Status: reviewed

### appservice recovery use case

- Code: `internal/modules/application/workorchd/biz/appservice/run_recovery_service.go`
- Owner layer: Application startup/recovery orchestration
- Expected responsibility: 启动时识别通用恢复形态，恢复 active workspace pointer，处理 orphan dispatched step；正式 Run 状态转换交给 kernel run service。
- Current responsibility: 扫描 running run，跳过带 continuation checkpoint 的 run；对无 continuation 的 stuck running run 调用 `RunService.FailRun`，然后只补偿 workspace active pointer。
- Lifecycle check: recovery 不注册 module，不持有后台资源；由 HTTP/server scheduler 触发。
- EventHub check: failed run 状态事件由 kernel run service 发布；recovery 本身不直接发布 run state event。
- Background task check: recovery 函数本身同步执行，后台调度由 serverState 通过 `BackgroundRoutine` 管理。
- State ownership check: 本轮将 stuck running run 的直接 repository 状态写入收口为 `RunService.FailRun`；workspace active pointer 清理保留为 recovery 补偿。
- Workspace scope check: duplicate run ID 恢复和 orphan step 恢复均使用 `ContextWithRuntimeWorkspaceID` 绑定 workspace。
- Findings: recovery 曾直接修改 Run state / blockers / failureReason，绕过 kernel run facade。
- Required action: 已改为通过 `RunService.FailRun`，并由现有 recovery 回归测试覆盖。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -count=1`
- Status: reviewed

### appservice decision use case

- Code: `internal/modules/application/workorchd/biz/appservice/decision_service.go`
- Owner layer: Application governance use-case facade
- Expected responsibility: 将 HTTP/TUI 命令转换为 kernel decision service 调用，做 payload normalization/redaction 和 projection；正式 ticket/resolution 状态由 kernel decision 持有。
- Current responsibility: list/page/read/resolve/batch resolve 均通过 session repository/service；resolution 类型走 governance/decision common normalization。
- Lifecycle check: 无 framework lifecycle；session 生命周期由 `CloseOnReturn` 管理。
- EventHub check: resolved 事件由 kernel decision service 发布；appservice 不直接发布治理状态事件。
- Background task check: 仅使用请求内短期并发读取 resolution，不启动长期任务。
- State ownership check: appservice 不直接修改 Ticket/Resolution；resolve 通过 `DecisionService.EnsureResolvedTicketAndReturn`。
- Workspace scope check: 本轮补充 runtime workspace scope guard；当 context 带 workspace scope 时，read/resolve/batch resolve 只能处理同 workspace ticket。
- Findings: 带 `workspaceId` 的 governance 请求曾可按 ticket ID 跨 workspace 读取或 resolve。
- Required action: 已补 scope guard 和 wrong-scope regression tests。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -run 'Decision|Approval' -count=1`
- Status: reviewed

### appservice approval use case

- Code: `internal/modules/application/workorchd/biz/appservice/approval_service.go`
- Owner layer: Application governance use-case facade
- Expected responsibility: 将 approve/reject/list/read 命令转换为 kernel approval service 调用；正式 approval 状态由 kernel approval 持有。
- Current responsibility: list/page/read/approve/reject/batch resolve 均通过 session repository/service；action normalization 走 governance common。
- Lifecycle check: 无 framework lifecycle；session 生命周期由 `CloseOnReturn` 管理。
- EventHub check: approval resolved 事件由 kernel approval service 发布；appservice 不直接发布治理状态事件。
- Background task check: 仅使用请求内短期并发读取 approval，不启动长期任务。
- State ownership check: approve/reject 通过 `ApprovalService.EnsureApprovedAndReturn` / `EnsureRejectedAndReturn`；带 workspace scope 时先校验再转换，避免 wrong-scope 请求先改状态。
- Workspace scope check: 本轮补充 runtime workspace scope guard；当 context 带 workspace scope 时，read/approve/reject/batch resolve 只能处理同 workspace approval。
- Findings: 带 `workspaceId` 的 governance 请求曾可按 approval ID 跨 workspace 读取或处理。
- Required action: 已补 scope guard 和 wrong-scope regression tests，同时保持无 scope 路径的单次 kernel transition read 契约。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -run 'Decision|Approval' -count=1`
- Status: reviewed

### appservice read model projections

- Code: `internal/modules/application/workorchd/biz/appservice/read_model_*.go`
- Owner layer: Application read/projection facade
- Expected responsibility: 基于 repositories 和 published events 构造 workspace/run/status/task/context projection，不反写正式状态。
- Current responsibility: read model 读取 workspace/run scoped repositories，聚合 steps/results/artifacts/patches/checkpoints/decisions/approvals/usage traces 并输出 projection。
- Lifecycle check: 无 framework lifecycle；session 生命周期由 entrypoints 管理。
- EventHub check: 只读取 session runtime 已发布事件或 event records，不直接发布事件。
- Background task check: `runReadModelTasks` 仅做请求内短期并发聚合并同步等待，不属于长期 background task。
- State ownership check: 未发现 read model 反写 Run/Workspace/Result 正式状态。
- Workspace scope check: run/detail entrypoints 会先解析 run 所属 workspace 并写入 runtime workspace context；workspace snapshot/task progress 使用显式 workspaceID。
- Findings: 未发现需要本轮代码收口的问题；read model 的 `getRunFromReadModelReader` 在 context 带 runtime workspace scope 时先 `ListByWorkspaceID` 再匹配 `runID`，无 scope 兼容路径才退回 `Get(runID)`；EventHub-backed run reader 也会把 context workspace scope 传给 run owner command，由 run owner guard 校验。
- Required action: 已核对 read model `Get(runID)` 作用域闭环；后续新增 read model 入口必须先解析并绑定 workspace scope，再访问 run-scoped child evidence。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -count=1`
- Status: reviewed

### workorchd service/server lifecycle

- Code: `internal/modules/application/workorchd/service/server`
- Owner layer: Application HTTP service
- Expected responsibility: 接收 framework 注入的 `event.Hub` / `BackgroundRoutine`，在 `Startup` 完成接线和校验，在 `Run` 启动 listener，在 `Shutdown` 关闭 listener 并等待退出。
- Current responsibility: HTTP handler、validation、response、SSE；startup recovery / governance resume / replan continuation 调度已由 `runContinuationScheduler` 收敛，snapshot cache warm 仍作为 read-model cache helper 使用同一 background scheduler。
- Lifecycle check: `Startup` 保存 framework `BackgroundRoutine` 并传入 HTTP server；`Run` 通过 `BackgroundRoutine.AsyncFunction` 启动 listener；`Shutdown` cancel serverState context、unsubscribe event handlers、等待后台任务、关闭 shared session。
- EventHub check: HTTP service 通过 `workorchevents.NewMagicHubWithHubOwner(eventHub, "workorchd/http")` 复用 framework hub；SSE replay 和 governance watcher 订阅在 `close` 中释放。
- Background task check: 本轮将 `serverState` 的 startup recovery、governance resume、replan continuation、snapshot cache warm 统一收口到 `scheduleBackground`，框架路径使用 `BackgroundRoutine.AsyncFunction`，仅独立测试/未注入时 fallback 到受 context/WG 管理的 goroutine；governance/replan 去重状态已从 `serverState` 字段迁入 `runContinuationScheduler`。
- State ownership check: HTTP handler 不直接改正式状态；治理 resolve 后只调度 appservice/kernel resume；recovery 状态转换已下沉到 appservice 使用 kernel `RunService.FailRun`。
- Workspace scope check: governance/read route 将 query workspaceID 绑定到 context；appservice 已补 decision/approval wrong-scope guard；run snapshot cache key 保留 workspace-aware key。
- Findings: serverState 曾自持服务级后台 goroutine，未接入 framework `BackgroundRoutine`；带 workspaceId 的 governance detail/resolve 曾未被 appservice 实际校验；continuation/recovery 去重状态曾散落在 `serverState`。
- Required action: 已完成本轮收口并补 background scheduling / wrong-scope regression tests；后续若继续降低 handler 包体积，可将当前 server-local scheduler 作为窄接口传入 appservice recovery use case，但不得绕过 EventHub owner command。
- Verification: `go test ./internal/modules/application/workorchd/service/server -run 'Background|Recovery|Governance|Replan|Resume|RunQueue' -count=1 -timeout=300s`; `go test ./internal/modules/application/workorchd/service/server -count=1 -timeout=300s`
- Status: reviewed

### kernel/decision ticket lifecycle

- Code: `internal/modules/kernel/decision`
- Owner layer: Kernel governance formal state
- Expected responsibility: 持有 Decision Ticket / Resolution 正式对象与 open -> resolved 状态转换，不承担 UI 交互或运行推进。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 repository/EventHub，biz service 创建 ticket、幂等 ensure、解析 resolution，并由 service facade 对外暴露窄接口。
- Lifecycle check: plugin/module `Setup` 只接线依赖和回填 runtime helper；`Run` 订阅 decision command topics；`Teardown` 取消订阅并清空 module 引用。
- EventHub check: ticket created/resolved 事件通过统一 EventHub 同步发布；本轮收紧创建/ensure 参数，要求 run-scoped ticket 必须携带 `WorkspaceID`；跨模块创建/查询/更新 ticket/resolution 通过 decision command topics。
- Background task check: 不启动后台任务。
- State ownership check: open/resolved 转换集中在 decision biz；resolution 类型校验在 kernel decision common 内，不由 HTTP/TUI 裁决。
- Workspace scope check: `CreateTicket` / `EnsureTicket` 已要求 `TicketID`、`WorkspaceID`、`RunID`、`Reason` 非空，避免发布缺 workspace 的 run 相关治理事件；`GetTicket`、`ListTicketsByRun`、`EnsureResolved` 和 `DeleteTicket` command 在调用方传入 workspace/run/step scope 时由 decision owner 过滤或拒绝 wrong-scope 数据。`EnsureResolved` 的 scope guard 位于 decision biz service 内，复用原本的 ticket read 并在写入前校验。
- Findings: ticket 创建曾允许空 `WorkspaceID`，与 workspace-scoped EventHub 规则不一致；ticket command get/list-by-run/resolve/delete 曾未在 owner module/service 内执行 workspace scope guard。
- Required action: 已修复并补参数校验、wrong-scope owner command get/list/mutation 回归；appservice owner read helper 会把 context runtime workspace scope 写入 Decision command envelope/payload。
- Verification: `go test ./internal/modules/kernel/decision/... -count=1`; `go test ./internal/modules/kernel/approval ./internal/modules/kernel/decision -count=1 -timeout=180s`
- Status: reviewed

### kernel/approval approval lifecycle

- Code: `internal/modules/kernel/approval`
- Owner layer: Kernel governance formal state
- Expected responsibility: 持有 Approval 正式对象与 pending -> approved/rejected 终态转换，只作为恢复允许/拒绝输入。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 repository/EventHub，biz service 创建 approval、幂等 ensure、approve/reject，并由 service facade 对外暴露窄接口。
- Lifecycle check: plugin/module `Setup` 只接线依赖和回填 runtime helper；`Run` 订阅 approval command topics；`Teardown` 取消订阅并清空 module 引用。
- EventHub check: approval created/resolved 事件通过统一 EventHub 同步发布；本轮收紧创建/ensure 参数，要求 run-scoped approval 必须携带 `WorkspaceID`；跨模块创建/查询/更新 approval 通过 approval command topics。
- Background task check: 不启动后台任务。
- State ownership check: pending/approved/rejected 转换集中在 approval biz；同一 approval 终态幂等、相反终态冲突。
- Workspace scope check: `CreateApproval` / `EnsureApproval` 已要求 `ApprovalID`、`WorkspaceID`、`RunID`、`Reason` 非空，避免发布缺 workspace 的 run 相关治理事件；`GetApproval`、`ListApprovalsByRun`、`EnsureApproved`、`EnsureRejected` 和 `DeleteApproval` command 在调用方传入 workspace/run/step scope 时由 approval owner module 过滤或拒绝 wrong-scope 数据。
- Findings: approval 创建曾允许空 `WorkspaceID`，与 workspace-scoped EventHub 规则不一致；approval command get/list-by-run/approve/reject/delete 曾未在 owner module 内执行 workspace scope guard。
- Required action: 已修复并补参数校验、wrong-scope owner command get/list/mutation 回归；appservice owner read helper 会把 context runtime workspace scope 写入 Approval command envelope/payload。
- Verification: `go test ./internal/modules/kernel/approval/... -count=1`; `go test ./internal/modules/kernel/approval ./internal/modules/kernel/decision -count=1 -timeout=180s`
- Status: reviewed

### kernel/result formal outcome module

- Code: `internal/modules/kernel/result`
- Owner layer: Kernel formal result and artifact owner
- Expected responsibility: 定义并维护 Result / Artifact / DoD assessment / deliverable manifest 等正式 outcome 对象，不由 block adapter 直接裁决业务结果。
- Current responsibility: `module.go` / `plugin.go` 接入 framework lifecycle；`biz` 负责 Result/Artifact 基本校验与 repository owner 操作；`service` 暴露窄接口，orchestrator 生产路径和 appservice purge 管理路径均通过 EventHub command adapter 访问。
- Lifecycle check: `Setup` 要求 Result 与 Artifact repository，`Run` 订阅 result/artifact command topics，`Teardown` 取消订阅并清理 service/biz 指针；framework plugin 通过 `workorchruntime` helper 获取 repository 并注册 ResultService。
- EventHub check: result/artifact command topics 由 result owner module 订阅，调用方必须收到 `events.Response`；orchestrator Result/Artifact closeout 已通过 EventHub-backed repository adapters 访问 result owner command topics，缺 response 失败。
- Background task check: 不启动后台任务。
- State ownership check: Result/Artifact 只表达正式 outcome/deliverable 模型；blocks 只能产出 evidence/patch/artifact input，不能绕过 orchestrator 直接接受业务结果。
- Workspace scope check: 模型包含 `WorkspaceID` / `RunID`；closeout create/update command envelope 带 workspace/run scope，list-by-workspace/run/result 命令带对应 subject scope。repository 查询一致性后续在 repository contract 层继续核对。
- Findings: 原目录只有 `pkg/common` 和 `pkg/models`，不符合 kernel module 结构规范；已补齐 `biz/service/module/plugin` 并接入 bootstrap runtime、framework plugin runtime 和入口 side-effect import。按当前仓库分组，Result/Artifact 是 kernel formal state owner，保留在 `internal/modules/kernel/result` 符合 README 已固化的模块落点。
- Required action: Result/Artifact owner command 边界已核对；direct completion gate 已要求正式 accepted DoD assessment result。若后续重新规划 formal outcome 分组，应先更新 README/architecture checklist，再迁移目录，避免只移动路径不改变职责。
- Verification: `go test ./internal/modules/kernel/result/... -count=1`; `go test ./internal/modules/kernel/orchestrator ./internal/modules/kernel/result -count=1 -timeout=240s`; `go test ./internal/initiators/workorchruntime ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice ./application/workorch -count=1`
- Status: reviewed

### kernel/eventstream append-only projection

- Code: `internal/modules/kernel/eventstream`
- Owner layer: Kernel event projection
- Expected responsibility: 订阅 WorkOrch 事件并 append-only 写入 event record repository，对外提供查询，不推进业务状态。
- Current responsibility: module `Run` 调用 biz `Start` 订阅事件；biz 将有效 WorkOrch event envelope 转成 event record；service facade 暴露 `ListEvents` 和 owner-scoped `PurgeWorkspaceEvents`。appservice workspace purge 通过 `/eventstream/command/purge-workspace` 触发事件投影清理。
- Lifecycle check: `Start` 对重复启动幂等；module `Run` 对重复启动幂等；`Stop` 调用所有 unsubscribe 并清空订阅；module `Teardown` 调用 `Stop` 后清空引用并释放 command subscriptions。
- EventHub check: 通过统一 EventHub `SubscribeWorkorch` 订阅所有 WorkOrch contract topics；非 WorkOrch 事件被忽略；append 冲突按幂等处理。
- Background task check: 不启动后台任务，事件 handler 只做 append 操作。
- State ownership check: eventstream 是只读投影和审计记录，不裁决 run/result/governance 状态。
- Workspace scope check: event record 保留 envelope 中的 `WorkspaceID`、`RunID`、`StepID` 和 revision；事件生产方是否完整填充由对应模块继续核对。
- Findings: appservice workspace purge 曾直接删除 event record repository；本轮已收口为 eventstream owner command，并补 workspace event purge 回归测试；重复 `Start` / module `Run` 不会重复注册订阅，`Stop` 后事件不再记录。
- Required action: 已补重复 `Start`、重复 module `Run` 和 `Stop` 后不再记录事件的显式 regression tests。
- Verification: `go test ./internal/modules/kernel/eventstream ./internal/modules/kernel/eventstream/biz -count=1 -timeout=120s`
- Status: reviewed

### kernel/workspace formal workspace definitions

- Code: `internal/modules/kernel/workspace`
- Owner layer: Kernel formal workspace state
- Expected responsibility: 持有 Workspace / Goal / DoD 正式对象和定义变更状态，不混入 runtime adapter 或执行推进。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 workspace/goal/dod repositories 与 EventHub；biz manager 创建/确保 workspace、定义/更新 goal 和 DoD，并发布 workspace/definition events。作为 workspace/goal/dod owner，module 订阅 workspace command topics，为 run/orchestrator/governance/appservice 提供同步 command response；删除 workspace 时由 workspace owner 级联清理 Goal/DoD。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 订阅 workspace/goal/dod command topics；`Teardown` 取消订阅并清空 module 引用。
- EventHub check: workspace/goal/dod 事件通过统一 EventHub 同步发布，payload 携带 `WorkspaceID` 与定义 lineage 字段；跨 module workspace/goal/dod 读取或更新通过 owner command handler。
- Background task check: 不启动后台任务。
- State ownership check: Workspace active/completed/failed/archived 相关基础状态由 workspace manager 维护；run execution 状态不在此模块裁决。
- Workspace scope check: Goal/DoD 定义要求显式 `WorkspaceID`；本轮收紧空白 title/deliverable/goal text/DoD item 校验，避免纯空定义进入正式对象。
- Findings: 目标文本、DoD 条目和 workspace 基础字段曾只检查空字符串，纯空白可绕过正式定义校验；run/orchestrator/appservice 曾通过直接 repository 注入读取或更新 workspace owner state。appservice purge 曾直接删除 Goal/DoD 仓储。
- Required action: 已修复空白校验并补回归测试；已补 workspace command handlers，生产跨 module 访问改为 EventHub command；本轮将 workspace delete 级联 Goal/DoD 清理放回 workspace owner。后续 repository 层继续核对 goal/dod ID 是否需要强制 workspace-scoped lookup。
- Verification: `go test ./internal/modules/kernel/workspace/... -count=1`; `go test ./internal/modules/kernel/run ./internal/modules/kernel/workspace ./internal/modules/kernel/orchestrator ./internal/modules/application/workorchd/biz/bootstrap -count=1`
- Status: reviewed

### kernel/soul catalog

- Code: `internal/modules/kernel/soul`
- Owner layer: Kernel formal catalog
- Expected responsibility: 持有 Soul 正式对象和 capability selection 约束，不写死具体业务 profile 执行行为。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 soul repository 与 EventHub；biz service 负责 Soul CRUD 和 selection allow-list 校验。作为 soul owner，module 订阅 soul command topics，为 appservice/context registry 和 workorchd CLI dispatcher 写路径提供同步 command response。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 订阅 soul command topics；`Teardown` 取消订阅并清空 module 引用。
- EventHub check: Soul catalog CRUD 通过 owner command topics 暴露；当前 Soul catalog 不发布变更通知事件，如后续需要变更审计，应通过统一 EventHub 发布 catalog event。
- Background task check: 不启动后台任务。
- State ownership check: 只维护 Soul 对象与选择约束，不执行 skill/MCP/tool，也不裁决 run 状态。
- Workspace scope check: Soul 是全局 catalog 对象，不绑定 workspace；workspace/profile 绑定在 appservice/template/selection 层继续核对。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 已核对 template/default soul seed 数据落点：内置 Soul/WorkspaceTemplate 只由 `workorchd/biz/bootstrap/defaults.go` 的 embedded defaults 写入；Workspace 创建期应用 template 绑定；Run activation 只使用/校验 workspace/template contract，不重新写入 default soul 或 stage bindings。
- Verification: `go test ./internal/modules/kernel/soul/... -count=1`
- Status: reviewed

### kernel/skill catalog

- Code: `internal/modules/kernel/skill`
- Owner layer: Kernel formal catalog
- Expected responsibility: 持有 Skill 正式对象和声明式需求，不写死业务 profile 行为。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 skill repository 与 EventHub；biz service 负责 Skill create/update/get/list。作为 skill owner，module 订阅 skill command topics，为 appservice/context registry 提供同步 command response。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 订阅 skill command topics；`Teardown` 取消订阅并清空 module 引用。
- EventHub check: Skill catalog CRUD/read 通过 owner command topics 暴露；当前 Skill catalog 不发布变更通知事件，如后续需要变更审计，应通过统一 EventHub 发布 catalog event。
- Background task check: 不启动后台任务。
- State ownership check: 只维护 skill instructions、trigger rules 和 required capability refs，不负责运行时自动匹配或执行。
- Workspace scope check: Skill 是全局 catalog 对象，不绑定 workspace；workspace 选择和 matching 在 appservice/runtime context assembly 层继续核对。
- Findings: skill 校验曾只拒绝空字符串，纯空白 name/version/instruction/list item 可进入正式 catalog。
- Required action: 已修复为 `strings.TrimSpace` 校验并补回归测试。
- Verification: `go test ./internal/modules/kernel/skill/... -count=1`
- Status: reviewed

### kernel/mcp catalog

- Code: `internal/modules/kernel/mcp`
- Owner layer: Kernel formal catalog
- Expected responsibility: 持有 MCP server/capability 正式目录对象，不执行 MCP tool/resource/prompt 调用。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 MCP server/capability repositories 与 EventHub；biz service 负责 server/capability CRUD、default capability 派生和目录过滤。作为 MCP catalog owner，module 订阅 MCP command topics，为 appservice/context registry 和 blocks/mcp runtime adapter 提供同步 command response。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 订阅 MCP command topics；`Teardown` 取消订阅并清空 module 引用。
- EventHub check: MCP server/capability CRUD/read/list 通过 owner command topics 暴露；当前 MCP catalog 不发布变更通知事件，如后续需要变更审计，应通过统一 EventHub 发布 catalog event。
- Background task check: 不启动后台任务。
- State ownership check: 只维护 catalog metadata 和 approval policy hints，不执行 MCP 调用，也不直接批准/拒绝风险。
- Workspace scope check: MCP catalog 是全局对象；workspace 绑定与 capability selection 在 appservice/template/runtime context assembly 层继续核对。
- Findings: default capability display name 曾对 `repo.summary`、`workspace://acceptance`、`workorch.acceptance` 混用特定业务硬编码；server name/transport 也只拒绝空字符串。
- Required action: 已将任意 tool/resource/prompt 默认展示名改为按 capability kind 生成通用名称；仅对内置验收 resource `workspace://acceptance` 和 prompt `workorch.acceptance` 应用受控 metadata，以满足 stdio e2e 和用户可读目录展示；已收紧 server name/transport 空白校验并补回归测试。
- Verification: `go test ./internal/modules/kernel/mcp/... -count=1`
- Status: reviewed

### kernel/memory catalog and recall objects

- Code: `internal/modules/kernel/memory`
- Owner layer: Kernel formal memory state
- Expected responsibility: 持有 memory item / recall trace 正式对象，不替代 runtime context assembly 或 embedding/provider 执行。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 memory repository；biz service 负责 memory CRUD、workspace list 和 repository search。作为 memory owner，module 订阅 memory command topics，为 orchestrator/governance 提供同步 command response。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 订阅 memory command topics；`Teardown` 取消订阅并清空引用。
- EventHub check: 当前 memory module 通过 command topics 暴露 CRUD/list/search owner 操作；shared events 已有 memory notification topic，后续若启用需统一通过 EventHub。
- Background task check: 不启动后台任务；embedding/vector 生成不在该 kernel service 内执行。
- State ownership check: 只校验和保存 memory item，不承担 recall selection policy 或 runtime context assembly。
- Workspace scope check: memory create/list/search 均要求 `WorkspaceID`；run/source refs 作为 source attribution 保存。
- Findings: memory 校验曾只拒绝空字符串，纯空白 sourceRef/summary/tag 可进入正式 memory 对象；orchestrator 生产路径曾直接注入 MemoryItem repository。
- Required action: 已修复为 `strings.TrimSpace` 校验并补回归测试；已补 memory command handlers，orchestrator 生产路径改为 EventHub-backed memory repository adapter。
- Verification: `go test ./internal/modules/kernel/memory/... -count=1`; `go test ./internal/modules/kernel/memory ./internal/modules/kernel/orchestrator ./internal/modules/application/workorchd/biz/bootstrap -count=1`
- Status: reviewed

### blocks/llm provider adapter

- Code: `internal/modules/blocks/llm`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 调用 LLM provider、标准化 schema、记录 provider call evidence，不直接执行文件/状态 side effect。
- Current responsibility: module 订阅 `/runtime/block/command/execute/llm/workspace_execution`、`/llm/command/complete-provider` 和 `/llm/command/generate-embedding`；biz adapter 选择 provider、调用 complete/stream/embed、解析/repair LLMExecutionDecision、输出 runtime 可消费的结构化决策和 LLM evidence refs。
- Lifecycle check: plugin/module `Setup` 只构造 adapter 并保存 EventHub；`Run` 注册 runtime block command observer 和 LLM provider command observers；`Teardown` 取消订阅并清空引用。
- EventHub check: appservice workspace setup draft 与 semantic memory retrieval 只发布 LLM owner command；LLM block 返回 `ProviderCompletionResult` / `ProviderEmbeddingResult` 的 `events.Response`。不直接发布业务状态事件；provider usage/evidence 作为 step output 交 orchestrator 消费。
- Background task check: 不启动后台任务。
- State ownership check: 不直接写 Run/Result/Artifact 状态；schema invalid、provider unavailable 等只作为 step failure output 返回。
- Workspace scope check: provider request/evidence refs 基于 request 中的 workspace/run/step 字段生成。
- Findings: appservice workspace setup draft 曾直接构造 LLM provider；activation semantic memory retrieval 曾直接构造 LLM runtime adapter 生成 embedding，均违反 application -> block 运行期交互应走 EventHub 的要求。本轮将 LLM provider config parsing/redaction 移到 `blocks/llm/pkg/providerconfig`，appservice 配置展示/校验不再 import LLM `biz`。
- Required action: 已新增 LLM completion/embedding command topics，由 LLM owner module 在 `Run` 订阅并在 `Teardown` 释放；appservice 只构造 provider request / embedding request 并消费 response。已核对 LLM decision 到 Result/Artifact 的唯一落点；正式 Result/Artifact 只由 orchestrator closeout/result subsystem 经 result owner command adapter 创建，block/appservice 不直接生成 accepted outcome。
- Verification: `go test ./internal/modules/blocks/llm ./internal/modules/blocks/llm/biz ./internal/modules/application/workorchd/biz/appservice -count=1`; `go test ./internal/initiators/... ./internal/modules/... ./internal/pkg/... -count=1`
- Status: reviewed

### blocks/mcp runtime adapter

- Code: `internal/modules/blocks/mcp`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 执行 MCP tool/resource/prompt 调用并产出结构化证据，不维护 MCP catalog 或业务状态。
- Current responsibility: module 订阅 MCP runtime block command；adapter 通过 `/mcp/command/get-server` EventHub command 查询 kernel MCP catalog，支持 HTTP/stdio/long-lived stdio，并按 server policy 校验 allowed tools/resources/prompts。
- Lifecycle check: plugin/module `Setup` 只构造 adapter 并保存 EventHub；`Run` 注册 runtime block command observer；`Teardown` 取消订阅并调用 adapter `Close` 释放 long-lived stdio sessions。
- EventHub check: 不直接发布业务状态事件。
- Background task check: 不启动 framework background task；stdio session 生命周期由 adapter close 管理。
- State ownership check: 不直接批准风险或裁决 Run/Result；adapter 只执行 catalog allow-list 校验，approval policy 只作为 catalog hint，实际治理由 kernel/appservice/orchestrator 消费。
- Workspace scope check: MCP request payload 携带 workspace/run/step refs；long-lived stdio session key 使用 workspace/server scope。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续若 MCP capability risk policy 从 catalog hint 升级为运行期审批要求，必须由 orchestrator/governance 创建 Approval 并阻塞 run，MCP adapter 仍只能校验已批准凭证或返回结构化 blocker。
- Verification: `go test ./internal/modules/blocks/mcp ./internal/modules/blocks/mcp/biz -count=1 -timeout=240s`
- Status: reviewed

### blocks/shell command adapter

- Code: `internal/modules/blocks/shell`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 在 workspace deliverable directory 内执行受限 shell 命令，返回 bounded output 和 failure classification。
- Current responsibility: module 订阅 shell runtime block command；adapter 校验危险命令、限制执行目录、设置 timeout、输出 stdout/stderr/status/exitCode/evidence quality。
- Lifecycle check: plugin/module `Setup` 只构造 adapter 并保存 EventHub；`Run` 注册 runtime block command observer；`Teardown` 取消订阅并清空引用。
- EventHub check: 不直接发布业务状态事件。
- Background task check: 不启动后台任务；单次命令由 context timeout 控制。
- State ownership check: 不直接写 run/result 状态；命令结果作为 step evidence 返回。
- Workspace scope check: 命令工作目录固定为 workspace deliverable directory，禁止危险路径/命令越界。
- Findings: stdout/stderr 曾使用无界 `bytes.Buffer` 捕获外部命令输出。
- Required action: 已增加 64KiB per-stream 输出上限和 truncation metadata，并补回归测试。
- Verification: `go test ./internal/modules/blocks/shell/... -count=1`
- Status: reviewed

### blocks/file workspace file adapter

- Code: `internal/modules/blocks/file`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 执行 workspace file list/read/edit preview，产出文件证据或 patch preview，不裁决 Run 终态。
- Current responsibility: module 订阅 file runtime block command；adapter 通过 guarded workspace path 限制 deliverable directory，list/read 有条目/字节上限，edit 只产出 patch preview。
- Lifecycle check: plugin/module `Setup` 只构造 adapter 并保存 EventHub；`Run` 注册 runtime block command observer；`Teardown` 取消订阅并清空引用。
- EventHub check: 不直接发布业务状态事件。
- Background task check: 不启动后台任务。
- State ownership check: `file/edit` 不直接落盘 patch apply；正式 patch preview/apply/revert、patch evidence 和 patch events 仍由 orchestrator patch subsystem 处理。
- Workspace scope check: 所有路径必须位于 workspace deliverable directory 内。
- Findings: 未发现需要本轮代码收口的问题；本轮复核确认 `file/edit` 只产出 patch preview evidence，不绕过 orchestrator 写 workspace。
- Required action: 后续新增文件写能力时必须继续走 orchestrator patch subsystem 或返回可由 orchestrator 转换的 patch output，不能在 file block 内直接落盘。
- Verification: `go test ./internal/modules/blocks/file ./internal/modules/blocks/file/biz ./internal/modules/kernel/orchestrator/biz -run 'Patch|ArtifactContinuation' -count=1 -timeout=300s`
- Status: reviewed

### blocks/git repository adapter

- Code: `internal/modules/blocks/git`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 执行受限 git read/write 操作，输出结构化证据，不能越过 orchestrator。
- Current responsibility: module 订阅 git runtime block command；adapter 限制操作集、pathspec、timeout、输出大小和 changed file 数。
- Lifecycle check: plugin/module `Setup` 只构造 adapter 并保存 EventHub；`Run` 注册 runtime block command observer；`Teardown` 取消订阅并清空引用。
- EventHub check: 不直接发布业务状态事件。
- Background task check: 不启动后台任务。
- State ownership check: git write 只执行 workspace repo 操作并返回 step output，不直接完成 run/result；它属于明确声明的 git capability evidence，不替代 orchestrator patch result closeout。
- Workspace scope check: repo root 从 workspace deliverable directory 解析，pathspec 有边界校验。
- Findings: 本轮复核未发现 git block 直接写 Run/Result/Artifact owner state；`runtime.git.writeEnabled` 在运行配置中必须显式 opt-in，默认配置文件继续显式配置 `writeEnabled=true`。LLM capability catalog 测试已显式隔离配置，避免外部 `CONFIG_PATH` 或其它测试状态影响 git/write capability 断言。
- Required action: 后续调整 git write 策略时必须同时更新 `workorchconfig` 默认语义、LLM capability catalog、git block runtime policy 和 e2e 配置，不能只改其中一处。
- Verification: `go test ./internal/pkg/workorchconfig -count=1 -timeout=120s`; `go test ./internal/modules/blocks/git ./internal/modules/blocks/git/biz ./internal/modules/blocks/llm ./internal/modules/blocks/llm/biz -run 'Git|RuntimeCapabilities|Capability|Config' -count=1 -timeout=240s`; `go test ./internal/modules/blocks/git ./internal/modules/blocks/git/biz ./internal/modules/kernel/orchestrator/biz -run 'Git|Patch' -count=1 -timeout=300s`; `go test ./internal/modules/blocks/llm/biz -count=1`
- Status: reviewed

### blocks/web provider adapter

- Code: `internal/modules/blocks/web`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 调用受治理的 web search/fetch provider，返回来源证据，不写业务结论。
- Current responsibility: module 订阅 web runtime block command；adapter 执行 provider request、域名/IP policy、max results/fetch bytes/redirect/timeout、approval-required gating；需要审批的 web policy 只返回 `blockApproval*` / `approvalRequired` output，正式 Approval 创建和 run blocking 由 orchestrator 处理；approval 校验通过 `/approval/command/get` EventHub command 查询 approval owner module。
- Lifecycle check: plugin/module `Setup` 只构造 adapter 并保存 EventHub；`Run` 注册 runtime block command observer；`Teardown` 取消订阅并清空引用。
- EventHub check: approval 查询和 runtime execution 均通过 EventHub command，block 不直接注入 approval repository；Web adapter 需要校验已有 approval 时，owner command envelope/payload 携带 `RunID`。
- Background task check: 不启动后台任务。
- State ownership check: approval check 只验证已批准 approval 是否匹配请求，不创建/批准 approval，也不直接裁决 run/result。
- Workspace scope check: provider payload 和 output 携带 workspace/run/step refs；approval owner command 也携带 run scope。
- Findings: 曾直接注入 `ApprovalRepository` 用于 web approval 校验，违反 block -> kernel formal state 边界；已改为 EventHub command。本轮补充 `GetForRun` scoped getter，避免 approval owner command 缺少 run scope。Web provider config model 和 parsing 已移入 `blocks/web/pkg/models` / `blocks/web/pkg/providerconfig`，appservice 配置展示/校验不再 import Web `biz`。
- Required action: 后续新增 web risk policy 时继续只返回 block output 或校验已批准 Approval，不能在 Web adapter 内创建/更新 Approval 或 Run 状态。
- Verification: `go test ./internal/modules/blocks/web ./internal/modules/blocks/web/biz -count=1 -timeout=240s`; `go test ./internal/modules/blocks/web ./internal/modules/blocks/web/biz ./internal/modules/blocks/web/pkg/providerconfig ./internal/modules/application/workorchd/biz/appservice -count=1`
- Status: reviewed

### blocks/ssh remote command adapter

- Code: `internal/modules/blocks/ssh`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 执行远端命令并返回 bounded output；风险治理由上层决定。
- Current responsibility: module 订阅 ssh runtime block command；adapter 校验 host/user/auth/timeout，调用本地 ssh/setsid，隐藏密码，仅输出 command result。
- Lifecycle check: plugin/module `Setup` 只构造 adapter 并保存 EventHub；`Run` 注册 runtime block command observer；`Teardown` 取消订阅并清空引用。
- EventHub check: 不直接发布业务状态事件。
- Background task check: 不启动后台任务；单次远端命令由 context timeout 控制。
- State ownership check: 不直接写 run/result 状态；远端输出作为 step evidence 返回。
- Workspace scope check: privateKeyPath 相对路径基于 workspace deliverable directory 解析。
- Findings: stdout/stderr 曾使用无界 `bytes.Buffer` 捕获远端命令输出。
- Required action: 已增加 64KiB per-stream 输出上限和 truncation metadata，并补 fake ssh 回归测试。
- Verification: `go test ./internal/modules/blocks/ssh/... -count=1`
- Status: reviewed

### common shared types and runtime context

- Code: `internal/pkg/common`
- Owner layer: Shared infrastructure
- Expected responsibility: 提供错误、ID、状态枚举、runtime context scope、redaction 等跨模块基础类型；不承载业务状态机实现。
- Current responsibility: `errors.go` 只定义基础 sentinel errors；`ids.go` / `states.go` 定义共享 ID 和状态枚举；`runtime_context.go` 管理 workspace / LLM provider runtime scope。
- Lifecycle check: 无 framework lifecycle。
- EventHub check: 不直接接入 EventHub。
- Background task check: 不启动后台任务。
- State ownership check: 只声明状态枚举，不实现状态转换。
- Workspace scope check: `ContextWithRuntimeWorkspaceID` / `RuntimeWorkspaceIDFromContext` 会 trim 空白并兼容 string / typed ID，作为 repository/appservice workspace scope 统一入口。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续若新增 workspace-scoped helper，应继续放在 common 作为窄 helper，不把业务状态转换下沉到 common。
- Verification: `go test ./internal/pkg/common -count=1`
- Status: reviewed

### repository contracts and backends

- Code: `internal/pkg/repository`
- Owner layer: Shared repository contract / memory and file persistence
- Expected responsibility: 定义 repository interfaces，提供 memory/file backend 的一致行为；workspace-scoped run identity 由 contract、backend 和调用方 context 共同约束。
- Current responsibility: repository interfaces 提供 `Get`、`List`、`ListByWorkspaceID` / `ListByRun` 等基础能力；memory/file backends 已有 contract tests 覆盖 CRUD、clone、list scope、missing errors 和 file reload。
- Lifecycle check: repository 本身无 framework lifecycle；由 bootstrap/runtime/session 创建和关闭。
- EventHub check: repository 不发布事件；事件由 kernel service/orchestrator 在状态转换后发布。
- Background task check: repository 不启动后台任务。
- State ownership check: repository 只持久化模型，不裁决状态转换是否合法。
- Workspace scope check: run repository contract 保留 `ListByWorkspaceID`；appservice `getRunFromRepository` 已在 runtime workspace context 存在时用 workspace scoped list 解析 run ID，避免 duplicate run ID 恢复误读。
- Findings: Run repository interface 仍保留全局 `Get(runID)` 以兼容单次 kernel transition read；本轮已在 run owner EventHub command 边界增加 workspace guard，带 `workspaceId` 的 `/run/command/get` 不会返回其他 workspace 的同名 run。
- Required action: 后续可评估增加 workspace-scoped `GetByWorkspaceID(workspaceID, runID)` 到 repository contract，进一步减少 owner module 内部约束。
- Verification: `go test ./internal/pkg/repository/... -count=1`; `go test ./internal/modules/kernel/run ./internal/modules/application/workorchd/biz/appservice -count=1 -timeout=300s`
- Status: reviewed

### workorchconfig shared configuration

- Code: `internal/pkg/workorchconfig`
- Owner layer: Shared configuration infrastructure
- Expected responsibility: 统一读取、保存、normalize 和 reload magicCommon application configuration；配置读取应由入口/bootstrap/config appservice 等边界层集中使用。
- Current responsibility: `Current` / `Save` / `Reload` / provider value helpers 统一封装配置；initiator/bootstrap/server config service 正常使用该包。
- Lifecycle check: 通过 magicCommon configuration manager 管理全局配置，测试使用 `Close` 重置。
- EventHub check: 不接入 EventHub。
- Background task check: 不启动后台任务。
- State ownership check: 不写业务状态，只写 application config。
- Workspace scope check: 不涉及 workspace/run 查询。
- Findings: runtime session/bootstrap 已导出 `LoadConfig` 配置读取能力；activation、setup draft、read model、soul routing 和 run compressed context 等运行时 use-case 改为通过 session/export/context 注入配置读取，不再直接调用全局 `workorchconfig.Current()`。剩余直接读取集中在 runtime/session 构造、configuration service、bootstrap/initiator/server 展示层等边界位置。
- Required action: 后续新增依赖运行配置的 appservice use-case 必须通过 session/export loader 或显式 config accessor 注入，不得在业务流程中散乱读取全局配置。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -count=1`; `go test ./internal/modules/application/workorchd/... -count=1`
- Status: reviewed

## Per-Module Audit Record Template

逐项核对时可复制以下模板：

```markdown
### <module>

- Code:
- Owner layer:
- Expected responsibility:
- Current responsibility:
- Lifecycle check:
- EventHub check:
- Background task check:
- State ownership check:
- Workspace scope check:
- Findings:
- Required action:
- Verification:
- Status:
```
