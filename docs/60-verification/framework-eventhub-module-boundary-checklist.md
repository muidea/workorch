# Framework / EventHub / Module Boundary Checklist

Status: active

Type: verification-checklist

Last Updated: 2026-06-02

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

## Application Entry / Framework Layer

| 模块 | 代码落点 | 核对内容 | 状态 |
| --- | --- | --- | --- |
| application entry | `application/workorch/main.go` | 是否只负责进程入口、信号、调用 `app.Run`；是否避免业务逻辑 | reviewed |
| workorch app | `internal/modules/application/workorch/app` | 是否统一使用 `application.Startup` / `Run` / `Shutdown`；server / interactive / remote 模式边界是否清晰 | reviewed |
| workorch plugin | `internal/modules/application/workorch` | plugin 生命周期是否符合 `Setup` / `Run` / `Teardown`；是否无重副作用 | reviewed |
| workorchd plugin | `internal/modules/application/workorchd` | 是否接收框架 `event.Hub` / `BackgroundRoutine`；`Startup` / `Run` / `Shutdown` 是否清晰 | reviewed |

## Initiator / Runtime Assembly Layer

| 模块 | 代码落点 | 核对内容 | 状态 |
| --- | --- | --- | --- |
| workorchruntime initiator | `internal/initiators/workorchruntime` | 是否只提供 repository、EventHub、runtime adapter/service helper；是否 fail-fast；是否不承担业务状态机 | reviewed |
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

## EventHub Verification Rules

- 状态机关键事件应使用同步 `Publish`，由 `MagicHub` 映射到 magicCommon `Send`。
- 纯通知或 UI/observability 类事件可使用 `PublishAsync`，由 `MagicHub` 映射到 magicCommon `Post`。
- 每个 workspace-scoped 事件必须包含 `WorkspaceID`；run/step 事件还必须包含 `RunID` / `StepID`。
- 同一 workspace 的事件应落在同一 lane，保持状态投影顺序一致。
- 事件 handler 不应做长耗时工作；若需要 resume、recovery、batch scan，应转交 scheduler/background task。
- 所有 `Subscribe` 返回的 unsubscribe 必须在 `Stop` / `Teardown` / `Shutdown` 中调用。

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

- [ ] 完成本轮 WorkOrch framework/module 边界整理后，回写并完善对应开发 skill，确保后续应用开发能准确创建和使用符合 `github.com/muidea/magicCommon/framework` 的 module。
  - Target skills: `go-module-initiator-lifecycle`, `go-multi-module-dev`, `go-application-event-runtime`。
  - Skill updates must cover: framework side-effect imports 只放入口 `main` 按需启用；module `Setup` / `Run` / `Teardown` 职责边界；initiator/helper 依赖获取和 fail-fast；EventHub / BackgroundRoutine 接线；禁止在 internal bootstrap 中复用 framework plugin manager；测试和文档同步要求。
  - Acceptance: 新增或调整 module 时，agent 可仅依赖 skill 生成符合 magicCommon framework lifecycle、EventHub 和 BackgroundRoutine 规范的代码骨架与验证清单。

## Audit Records

### application/workorch entry and app runtime

- Code: `application/workorch/main.go`, `internal/modules/application/workorch/app`
- Owner layer: Application entry / CLI runtime
- Expected responsibility: `main` 负责信号上下文、按进程入口启用 framework initiator/modules，并调用 `app.Run`；`app` 负责解析启动模式、配置目录、调用 `application.Startup` / `Run` / `Shutdown`，并在 local interactive/server 模式内复用同一 framework runtime。
- Current responsibility: 已对齐参考 `magicRunner` 入口：framework side-effect imports 位于 `application/workorch/main.go`，`app` 包不再隐式注册 modules；remote 模式走 remote backend，不启动本地 framework runtime；local 模式统一使用 `service.DefaultService()` 启动已注册 initiator/module。
- Lifecycle check: `runLocal` 在 `application.Startup` 成功后 defer `shutdownApplication`，再调用 `application.Run`；server 模式等待 `workorchd.Done()` 或进程 context 取消；interactive 模式创建 request-scoped `RuntimeSession` 并 close。
- EventHub check: `app` 不直接创建独立 EventHub；interactive backend 通过 `RuntimeSession` 获取 framework runtime 中的 EventHub。
- Background task check: `app` 不直接启动长期任务；长期任务由 framework service/module/initiator 接管。
- State ownership check: `app` 仅做 workspace 默认选择和 CLI flow 编排，不直接裁决 Run 状态机。
- Workspace scope check: 默认 workspace 解析只用于 CLI 初始上下文；run/snapshot scope 后续由 backend/appservice 核对。
- Findings: framework module 注册曾放在 `app` 包，导致应用包带 side-effect 注册；已上移到 `main` 入口按进程启用。
- Required action: 后续在 backend/appservice 核对默认 workspace 与 run scope 的一致性。
- Verification: `go test ./application/workorch ./internal/modules/application/workorch/app -count=1`
- Status: reviewed

### application/workorch plugin lifecycle

- Code: `internal/modules/application/workorch`
- Owner layer: Application plugin module
- Expected responsibility: 作为 CLI application module 注册到 magicCommon plugin framework，验证 runtime initiator 已可获取，不承担业务状态机。
- Current responsibility: `Setup` 只通过 initiator helper 验证 `workorchruntime` 已接线并创建空 `Module`；`Run` / `Teardown` 无重副作用。
- Lifecycle check: `Setup` / `Run` / `Teardown` 均为轻量、幂等路径；权重晚于 runtime initiator。
- EventHub check: plugin 不直接使用 EventHub，符合当前空 module 职责。
- Background task check: plugin 不启动后台任务。
- State ownership check: 不持有或写入业务状态。
- Workspace scope check: 不涉及 run/workspace 查询。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 若后续 CLI module 承担实际订阅或后台任务，必须通过 framework `event.Hub` / `BackgroundRoutine` 接线。
- Verification: `go test ./internal/modules/application/workorch/... -count=1`
- Status: reviewed

### workorchd plugin lifecycle

- Code: `internal/modules/application/workorchd`
- Owner layer: Application HTTP plugin module
- Expected responsibility: 从配置构造 HTTP service，将 framework `event.Hub` / `BackgroundRoutine` 传给 service，并在 `Teardown` 关闭 service。
- Current responsibility: 符合 plugin module 生命周期；本轮已在 service 层完成 HTTP server background task 收口。
- Lifecycle check: `Setup` 解析配置并调用 `service.Startup`；`Run` 要求 service 已配置；`Teardown` 调用 `service.Shutdown` 并清空 service 引用。
- EventHub check: `Setup` 将 framework `event.Hub` 传入 HTTP service，service 再包装为 owner-scoped MagicHub。
- Background task check: `Setup` 将 framework `BackgroundRoutine` 传入 HTTP service；service `Run` 已优先通过 `AsyncFunction` 启动 listener。
- State ownership check: plugin 不直接写状态。
- Workspace scope check: plugin 不直接查询 run/workspace。
- Findings: service 层曾绕过 `BackgroundRoutine` 启动 listener，已收口。
- Required action: 继续核对 HTTP handler/serverState 内 continuation/recovery 调度边界。
- Verification: `go test ./internal/modules/application/workorchd -count=1`
- Status: reviewed

### workorchruntime initiator lifecycle

- Code: `internal/initiators/workorchruntime`
- Owner layer: Initiator / runtime assembly
- Expected responsibility: 提供 repository set、framework EventHub adapter、runtime policy、runtime adapter/service helper；不承担具体业务状态机裁决。
- Current responsibility: `Setup` 读取配置、构造 repository set、包装 framework EventHub、生成 runtime policy；kernel/block modules 通过 helper 回填 service/adapter。
- Lifecycle check: `Setup` fail-fast；`Run` 只确保内置默认数据；`Teardown` 清理 repository、event hub、policy、adapter 和 service 引用。本轮修复 `eventStreamService` 漏清理。
- EventHub check: 使用 `workorchevents.NewMagicHubWithHubOwner(eventHub, common.InitiatorID)` 复用 framework hub，未创建第二套 hub。
- Background task check: initiator 不启动后台任务；目前未保存 `BackgroundRoutine`。
- State ownership check: initiator 不直接推进 Run 状态机；内置默认数据初始化属于启动基础数据准备。
- Workspace scope check: repository set 对 file backend 使用 workspace-scoped repository set；后续仍需在 repository 层逐项核对 run id scope。
- Findings: `Teardown` 漏清理 `eventStreamService`，可能导致应用重启或测试重建后保留旧服务引用。
- Required action: 已修复并补回归测试；后续核对 bootstrap/module setup 顺序是否让所有 service/adapter 回填都 fail-fast。
- Verification: `go test ./internal/initiators/workorchruntime -count=1`
- Status: reviewed

### app bootstrap runtime assembly

- Code: `internal/modules/application/workorchd/biz/bootstrap`
- Owner layer: Runtime assembly / in-process bootstrap
- Expected responsibility: 组装 repository、kernel modules、block adapters、runtime registry、service exports 和 shared EventHub；不成为新的业务状态机拥有者。
- Current responsibility: 构造 memory/file repository set，按依赖顺序 setup modules，通过显式 `runtimeModuleLifecycle` 运行内部 modules，并导出 repository/service/infrastructure。
- Lifecycle check: bootstrap runtime 是 appservice 内部组装层，不再使用 `magicCommon/framework/plugin/common`；framework plugin/module 只在 application main 启动链路按需启用。发现 `Runtime.Shutdown` 和内部 lifecycle shutdown 曾在首个 teardown 错误处提前返回，可能跳过后续 modules 和 EventHub terminate；已改为继续执行完整 teardown、清空 modules，并用 `errors.Join` 返回聚合错误。
- EventHub check: 调用方传入 EventHub 时复用；未传入时只为独立 bootstrap runtime 创建 owner-scoped MagicHub。
- Background task check: bootstrap 本身不启动长期 background task；module `Run` 由 lifecycle 统一触发。
- State ownership check: bootstrap 只负责组装和内置默认数据，不直接裁决 Run/Result 状态。
- Workspace scope check: file backend 使用 workspace-scoped repository set；具体 run identity 在 repository 层继续核对。
- Findings: bootstrap 内部 runtime 曾复用 framework plugin manager，职责边界不清；shutdown 早退也会破坏内部资源释放语义。
- Required action: 已移除 bootstrap 内部 plugin manager，改为显式 runtime module lifecycle；已补充 lifecycle/runtime shutdown regression tests。
- Verification: `go test ./internal/modules/application/workorchd/biz/bootstrap -count=1`
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
- Required action: 后续核对 appservice 调用方是否始终按 request scope close session。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -run RuntimeSession -count=1`
- Status: reviewed

### events MagicHub framework adapter

- Code: `internal/pkg/events`
- Owner layer: Shared event infrastructure
- Expected responsibility: 将 WorkOrch `Hub` 映射到 magicCommon `event.Hub`，同步 `Publish` 使用 `Send`，异步 `PublishAsync` 使用 `Post`，并保持 workspace lane ordering 和 unsubscribe 释放。
- Current responsibility: `MagicHub.Publish` 使用 `Send` 并返回 handler response/error；`PublishAsync` 使用 `Post`；workspace 事件 lane 为 `workspace/<id>`，全局事件 fallback 到 topic lane；同一 raw hub 的 wrapper 共享 workspace revision state。
- Lifecycle check: owner-created MagicHub 支持 `Terminate`；wrapped framework hub 不由 adapter 关闭。
- EventHub check: 已有测试覆盖 Send/Post 行为、workspace lane、topic fallback、shared wrapper revision、multi-wrapper delivery、wildcard subscription 和 unsubscribe。
- Background task check: adapter 不启动业务后台任务；unsubscribe 为避免底层阻塞使用 bounded wait。
- State ownership check: 只记录 event envelope 和 revision，不裁决业务状态。
- Workspace scope check: revision 和 lane 均按 WorkspaceID scoped；缺失 WorkspaceID 的全局事件 revision 为 0。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续核对事件发布方是否对 workspace-scoped 事件完整填充 WorkspaceID/RunID/StepID。
- Verification: `go test ./internal/pkg/events -count=1`
- Status: reviewed

### kernel/run state machine facade

- Code: `internal/modules/kernel/run`
- Owner layer: Kernel formal run state facade
- Expected responsibility: 持有 Run / Step primitives 的正式 service 门面，公开创建、启动、阻塞、恢复、验证、完成、取消、替换、失败等状态转换入口；复杂执行推进交给 orchestrator。
- Current responsibility: `run.Service` 负责参数校验、幂等 ensure 语义和事件发布；执行推进仍委托 orchestrator。
- Lifecycle check: plugin/module 生命周期未在本轮修改；状态机 service 无后台任务。
- EventHub check: Run service 状态变更通过 shared EventHub 发布 run events。
- Background task check: 不启动后台任务。
- State ownership check: 本轮修复 public `ResumeRun` / `EnsureResumed` 边界：普通 `running` run 不再被 run service 当作可 resume 状态；blocked states 继续委托 orchestrator；已加载 `LoadedRun` 的内部 recovery 路径仍可由 appservice/orchestrator 使用。
- Workspace scope check: 本轮未修改 repository scope；run ID workspace scope 后续在 repository 层继续核对。
- Findings: `ResumeRun` 曾允许 `running` run 透传到 orchestrator recovery 分支，导致 public resume API 对非 blocked 状态返回成功。
- Required action: 已修复并用现有 `TestService_ResumeRun_RejectsNonBlockedStates` 覆盖；后续把 workspace-scoped run identity 收口到 repository 层。
- Verification: `go test ./internal/modules/kernel/run/... -count=1`
- Status: reviewed

### kernel/orchestrator execution state authority

- Code: `internal/modules/kernel/orchestrator`
- Owner layer: Kernel execution orchestrator
- Expected responsibility: 作为 dispatch、repair、replan、completion gate、runtime recovery 的唯一执行推进核心。
- Current responsibility: 保留 blocked resume、running recovery、terminal step recovery、initial dispatch recovery、replan continuation 等核心推进逻辑。
- Lifecycle check: 无 framework lifecycle 资源；由 kernel module setup 注入 repositories/runtime adapters。
- EventHub check: run/workspace 状态变化通过 event publisher 发布，保持 workspace-scoped event payload。
- Background task check: orchestrator 不直接持有后台 scheduler；调用方负责触发。
- State ownership check: orchestrator 继续允许内部 `running` recovery，这是恢复执行状态的一部分；public running resume 已在 run service facade 层收窄。
- Workspace scope check: orchestrator 对运行推进依赖 repositories；run id scope 后续在 repository 层继续核对。
- Findings: 无需移除 orchestrator running recovery；应由上层 facade 控制普通 resume 入口。
- Required action: 后续核对 completion gate、result/artifact closeout 与 `runtime-stage-contract.md` 的逐项一致性。
- Verification: `go test ./internal/modules/kernel/orchestrator/... -count=1`
- Status: reviewed

### kernel/governance resume boundary

- Code: `internal/modules/kernel/governance`
- Owner layer: Kernel governance automation
- Expected responsibility: 自动治理、ticket/approval resolution 和 resume trigger；不替代 Run 状态机。
- Current responsibility: governance 通过注入的 run service `EnsureResumed` 触发恢复，不直接调用 orchestrator 或改写 Run 状态。
- Lifecycle check: 本轮未发现 framework lifecycle 问题。
- EventHub check: governance 自动处理不应阻塞 EventHub handler；后续在 server/appservice scheduler 下沉时继续核对触发路径。
- Background task check: governance 本身不持有长期后台任务。
- State ownership check: 本轮 `EnsureResumed` 收窄后，governance 只能恢复 blocked run，不会把 running run 当作可恢复状态。
- Workspace scope check: 后续结合 repository scope 核对 governance traces 与 run/workspace 查询。
- Findings: governance 边界依赖 run service 的 `EnsureResumed` 契约；本轮已通过 run service 修复强化。
- Required action: 后续核对 auto-governance trace/event payload 是否完整携带 workspace/run refs。
- Verification: `go test ./internal/modules/kernel/governance/... -count=1`
- Status: reviewed

### appservice activation use case

- Code: `internal/modules/application/workorchd/biz/appservice/workspace_execution_activation_service.go`
- Owner layer: Application use-case orchestration
- Expected responsibility: 组合 workspace、goal、DoD、selection context 和 run creation/start；正式 Run 状态转换通过 kernel run service 完成。
- Current responsibility: 根据 workspace 当前定义和命令选择 Goal/DoD/Soul/Skill/MCP/Memory，构建 executionContext，调用 `RunService.SupersedeRun`、`CreateRun` / `StartRun` 进入 kernel 状态机。
- Lifecycle check: appservice 不注册 framework module，不持有长期生命周期资源；session 通过 `CloseOnReturn` 释放。
- EventHub check: 通过 session/context 传入统一 EventHub，创建/启动 run 的正式事件由 kernel service/orchestrator 发布。
- Background task check: 不启动长期后台任务；activation 是单次 use-case。
- State ownership check: active run stale replacement 已通过 `RunService.SupersedeRun`，新 run 通过 run service 创建和启动；未发现直接写 Run 终态的越界逻辑。
- Workspace scope check: activation 开始即读取 workspace 并写入 runtime workspace context，后续 run repository lookup 使用 scoped context。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续继续核对 selection context 中 Skill/MCP/Memory 自动绑定策略是否应进一步拆入专门 selector service。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -count=1`
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
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续 repository 层继续核对所有 read model 依赖的 `Get(runID)` 是否在 scoped context 下等价于 workspace-scoped lookup。
- Verification: `go test ./internal/modules/application/workorchd/biz/appservice -count=1`
- Status: reviewed

### workorchd service/server lifecycle

- Code: `internal/modules/application/workorchd/service/server`
- Owner layer: Application HTTP service
- Expected responsibility: 接收 framework 注入的 `event.Hub` / `BackgroundRoutine`，在 `Startup` 完成接线和校验，在 `Run` 启动 listener，在 `Shutdown` 关闭 listener 并等待退出。
- Current responsibility: HTTP handler、validation、response、SSE，以及 application 级 startup recovery / governance resume / replan continuation / snapshot cache warm 调度。
- Lifecycle check: `Startup` 保存 framework `BackgroundRoutine` 并传入 HTTP server；`Run` 通过 `BackgroundRoutine.AsyncFunction` 启动 listener；`Shutdown` cancel serverState context、unsubscribe event handlers、等待后台任务、关闭 shared session。
- EventHub check: HTTP service 通过 `workorchevents.NewMagicHubWithHubOwner(eventHub, "workorchd/http")` 复用 framework hub；SSE replay 和 governance watcher 订阅在 `close` 中释放。
- Background task check: 本轮将 `serverState` 的 startup recovery、governance resume、replan continuation、snapshot cache warm 统一收口到 `scheduleBackground`，框架路径使用 `BackgroundRoutine.AsyncFunction`，仅独立测试/未注入时 fallback 到受 context/WG 管理的 goroutine。
- State ownership check: HTTP handler 不直接改正式状态；治理 resolve 后只调度 appservice/kernel resume；recovery 状态转换已下沉到 appservice 使用 kernel `RunService.FailRun`。
- Workspace scope check: governance/read route 将 query workspaceID 绑定到 context；appservice 已补 decision/approval wrong-scope guard；run snapshot cache key 保留 workspace-aware key。
- Findings: serverState 曾自持服务级后台 goroutine，未接入 framework `BackgroundRoutine`；带 workspaceId 的 governance detail/resolve 曾未被 appservice 实际校验。
- Required action: 已完成本轮收口并补 background scheduling / wrong-scope regression tests；后续可将 continuation scheduler 从 HTTP serverState 进一步抽成 appservice scheduler，降低 handler 包体积。
- Verification: `go test ./internal/modules/application/workorchd/service/server -count=1`
- Status: reviewed

### kernel/decision ticket lifecycle

- Code: `internal/modules/kernel/decision`
- Owner layer: Kernel governance formal state
- Expected responsibility: 持有 Decision Ticket / Resolution 正式对象与 open -> resolved 状态转换，不承担 UI 交互或运行推进。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 repository/EventHub，biz service 创建 ticket、幂等 ensure、解析 resolution，并由 service facade 对外暴露窄接口。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 无长期任务；`Teardown` 清空 module 引用。
- EventHub check: ticket created/resolved 事件通过统一 EventHub 同步发布；本轮收紧创建/ensure 参数，要求 run-scoped ticket 必须携带 `WorkspaceID`。
- Background task check: 不启动后台任务。
- State ownership check: open/resolved 转换集中在 decision biz；resolution 类型校验在 kernel decision common 内，不由 HTTP/TUI 裁决。
- Workspace scope check: `CreateTicket` / `EnsureTicket` 已要求 `TicketID`、`WorkspaceID`、`RunID`、`Reason` 非空，避免发布缺 workspace 的 run 相关治理事件。
- Findings: ticket 创建曾允许空 `WorkspaceID`，与 workspace-scoped EventHub 规则不一致。
- Required action: 已修复并补参数校验回归；后续 repository 层继续核对 ticket ID 与 workspace scope 查询一致性。
- Verification: `go test ./internal/modules/kernel/decision/... -count=1`
- Status: reviewed

### kernel/approval approval lifecycle

- Code: `internal/modules/kernel/approval`
- Owner layer: Kernel governance formal state
- Expected responsibility: 持有 Approval 正式对象与 pending -> approved/rejected 终态转换，只作为恢复允许/拒绝输入。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 repository/EventHub，biz service 创建 approval、幂等 ensure、approve/reject，并由 service facade 对外暴露窄接口。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 无长期任务；`Teardown` 清空 module 引用。
- EventHub check: approval created/resolved 事件通过统一 EventHub 同步发布；本轮收紧创建/ensure 参数，要求 run-scoped approval 必须携带 `WorkspaceID`。
- Background task check: 不启动后台任务。
- State ownership check: pending/approved/rejected 转换集中在 approval biz；同一 approval 终态幂等、相反终态冲突。
- Workspace scope check: `CreateApproval` / `EnsureApproval` 已要求 `ApprovalID`、`WorkspaceID`、`RunID`、`Reason` 非空，避免发布缺 workspace 的 run 相关治理事件。
- Findings: approval 创建曾允许空 `WorkspaceID`，与 workspace-scoped EventHub 规则不一致。
- Required action: 已修复并补参数校验回归；后续 repository 层继续核对 approval ID 与 workspace scope 查询一致性。
- Verification: `go test ./internal/modules/kernel/approval/... -count=1`
- Status: reviewed

### kernel/result formal outcome models

- Code: `internal/modules/kernel/result`
- Owner layer: Kernel formal result model
- Expected responsibility: 定义 Result / Artifact / DoD assessment / deliverable manifest 等正式 outcome 数据结构，不由 block adapter 直接裁决业务结果。
- Current responsibility: 当前该目录只包含 `pkg/common` 和 `pkg/models`，没有独立 plugin/module/service；正式 result/artifact 的生成和关闭由 orchestrator closeout 路径与 repository 承载。
- Lifecycle check: model-only package 无 framework lifecycle；不存在单独的 `Setup` / `Run` / `Teardown`。
- EventHub check: result/artifact 相关 topic 在 shared events 中定义；实际发布方需在 orchestrator/result closeout 路径继续核对。
- Background task check: 不启动后台任务。
- State ownership check: Result/Artifact 只表达正式 outcome/deliverable 模型；blocks 只能产出 evidence/patch/artifact input，不能绕过 orchestrator 直接接受业务结果。
- Workspace scope check: 模型包含 `WorkspaceID` / `RunID`；repository 查询一致性后续在 repository 和 closeout 路径核对。
- Findings: 当前没有 result runtime module，这不是 framework 生命周期遗漏；它是正式模型包，权威生成逻辑在 orchestrator closeout。
- Required action: 后续核对 orchestrator completion gate 与 result/artifact repository 写入是否完全符合 `runtime-stage-contract.md`。
- Verification: `go test ./internal/modules/kernel/result/... -count=1`
- Status: reviewed

### kernel/eventstream append-only projection

- Code: `internal/modules/kernel/eventstream`
- Owner layer: Kernel event projection
- Expected responsibility: 订阅 WorkOrch 事件并 append-only 写入 event record repository，对外提供查询，不推进业务状态。
- Current responsibility: module `Run` 调用 biz `Start` 订阅事件；biz 将有效 WorkOrch event envelope 转成 event record；service facade 只暴露 `ListEvents`。
- Lifecycle check: `Start` 对重复启动幂等；`Stop` 调用所有 unsubscribe 并清空订阅；module `Teardown` 调用 `Stop` 后清空引用。
- EventHub check: 通过统一 EventHub `SubscribeWorkorch` 订阅所有 WorkOrch contract topics；非 WorkOrch 事件被忽略；append 冲突按幂等处理。
- Background task check: 不启动后台任务，事件 handler 只做 append 操作。
- State ownership check: eventstream 是只读投影和审计记录，不裁决 run/result/governance 状态。
- Workspace scope check: event record 保留 envelope 中的 `WorkspaceID`、`RunID`、`StepID` 和 revision；事件生产方是否完整填充由对应模块继续核对。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续可补重复 `Start` 和 `Stop` 后不再记录事件的显式 regression test；当前已有持久化和非 WorkOrch 事件过滤测试。
- Verification: `go test ./internal/modules/kernel/eventstream/... -count=1`
- Status: reviewed

### kernel/workspace formal workspace definitions

- Code: `internal/modules/kernel/workspace`
- Owner layer: Kernel formal workspace state
- Expected responsibility: 持有 Workspace / Goal / DoD 正式对象和定义变更状态，不混入 runtime adapter 或执行推进。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 workspace/goal/dod repositories 与 EventHub；biz manager 创建/确保 workspace、定义/更新 goal 和 DoD，并发布 workspace/definition events。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 无长期任务；`Teardown` 清空 module 引用。
- EventHub check: workspace/goal/dod 事件通过统一 EventHub 同步发布，payload 携带 `WorkspaceID` 与定义 lineage 字段。
- Background task check: 不启动后台任务。
- State ownership check: Workspace active/completed/failed/archived 相关基础状态由 workspace manager 维护；run execution 状态不在此模块裁决。
- Workspace scope check: Goal/DoD 定义要求显式 `WorkspaceID`；本轮收紧空白 title/deliverable/goal text/DoD item 校验，避免纯空定义进入正式对象。
- Findings: 目标文本、DoD 条目和 workspace 基础字段曾只检查空字符串，纯空白可绕过正式定义校验。
- Required action: 已修复并补回归测试；后续 repository 层继续核对 goal/dod ID 是否需要强制 workspace-scoped lookup。
- Verification: `go test ./internal/modules/kernel/workspace/... -count=1`
- Status: reviewed

### kernel/soul catalog

- Code: `internal/modules/kernel/soul`
- Owner layer: Kernel formal catalog
- Expected responsibility: 持有 Soul 正式对象和 capability selection 约束，不写死具体业务 profile 执行行为。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 soul repository；biz service 负责 Soul CRUD 和 selection allow-list 校验。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 无长期任务；`Teardown` 清空 module 引用。
- EventHub check: 当前 Soul catalog 不发布事件；如后续需要变更审计，应通过统一 EventHub 发布 catalog event。
- Background task check: 不启动后台任务。
- State ownership check: 只维护 Soul 对象与选择约束，不执行 skill/MCP/tool，也不裁决 run 状态。
- Workspace scope check: Soul 是全局 catalog 对象，不绑定 workspace；workspace/profile 绑定在 appservice/template/selection 层继续核对。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续核对 template/default soul seed 数据是否仍只在 bootstrap/default-data 层完成。
- Verification: `go test ./internal/modules/kernel/soul/... -count=1`
- Status: reviewed

### kernel/skill catalog

- Code: `internal/modules/kernel/skill`
- Owner layer: Kernel formal catalog
- Expected responsibility: 持有 Skill 正式对象和声明式需求，不写死业务 profile 行为。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 skill repository；biz service 负责 Skill create/update/get/list。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 无长期任务；`Teardown` 清空 module 引用。
- EventHub check: 当前 Skill catalog 不发布事件；如后续需要变更审计，应通过统一 EventHub 发布 catalog event。
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
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 MCP server/capability repositories；biz service 负责 server/capability CRUD、default capability 派生和目录过滤。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 无长期任务；`Teardown` 清空 module 引用。
- EventHub check: 当前 MCP catalog 不发布事件；如后续需要变更审计，应通过统一 EventHub 发布 catalog event。
- Background task check: 不启动后台任务。
- State ownership check: 只维护 catalog metadata 和 approval policy hints，不执行 MCP 调用，也不直接批准/拒绝风险。
- Workspace scope check: MCP catalog 是全局对象；workspace 绑定与 capability selection 在 appservice/template/runtime context assembly 层继续核对。
- Findings: default capability display name 曾对 `repo.summary`、`workspace://acceptance`、`workorch.acceptance` 做特定业务硬编码；server name/transport 也只拒绝空字符串。
- Required action: 已移除特定名称硬编码，改为按 capability kind 生成通用展示名；已收紧 server name/transport 空白校验并补回归测试。
- Verification: `go test ./internal/modules/kernel/mcp/... -count=1`
- Status: reviewed

### kernel/memory catalog and recall objects

- Code: `internal/modules/kernel/memory`
- Owner layer: Kernel formal memory state
- Expected responsibility: 持有 memory item / recall trace 正式对象，不替代 runtime context assembly 或 embedding/provider 执行。
- Current responsibility: plugin 通过 `workorchruntime` initiator 获取 memory repository；biz service 负责 memory CRUD、workspace list 和 repository search。
- Lifecycle check: plugin `Setup` 只接线依赖和回填 runtime helper；`Run` 无长期任务；`Teardown` 清空 module 引用。
- EventHub check: 当前 memory module 不发布事件；shared events 已有 memory topic，后续若启用需统一通过 EventHub。
- Background task check: 不启动后台任务；embedding/vector 生成不在该 kernel service 内执行。
- State ownership check: 只校验和保存 memory item，不承担 recall selection policy 或 runtime context assembly。
- Workspace scope check: memory create/list/search 均要求 `WorkspaceID`；run/source refs 作为 source attribution 保存。
- Findings: memory 校验曾只拒绝空字符串，纯空白 sourceRef/summary/tag 可进入正式 memory 对象。
- Required action: 已修复为 `strings.TrimSpace` 校验并补回归测试。
- Verification: `go test ./internal/modules/kernel/memory/... -count=1`
- Status: reviewed

### blocks/llm provider adapter

- Code: `internal/modules/blocks/llm`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 调用 LLM provider、标准化 schema、记录 provider call evidence，不直接执行文件/状态 side effect。
- Current responsibility: plugin 从 `workorchruntime` 回填 runtime adapter；biz adapter 选择 provider、调用 complete/stream/embed、解析/repair LLMExecutionDecision、输出 runtime 可消费的结构化决策和 LLM evidence refs。
- Lifecycle check: plugin `Setup` 只构造 adapter 并回填 runtime helper；`Run` 无长期任务；`Teardown` 清空引用。
- EventHub check: 不直接发布业务状态事件；provider usage/evidence 作为 step output 交 orchestrator 消费。
- Background task check: 不启动后台任务。
- State ownership check: 不直接写 Run/Result/Artifact 状态；schema invalid、provider unavailable 等只作为 step failure output 返回。
- Workspace scope check: provider request/evidence refs 基于 request 中的 workspace/run/step 字段生成。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续在 orchestrator closeout 核对 LLM decision 到 Result/Artifact 的唯一落点。
- Verification: `go test ./internal/modules/blocks/llm/... -count=1`
- Status: reviewed

### blocks/mcp runtime adapter

- Code: `internal/modules/blocks/mcp`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 执行 MCP tool/resource/prompt 调用并产出结构化证据，不维护 MCP catalog 或业务状态。
- Current responsibility: plugin 依赖 kernel MCP service 读取 server/capability catalog；adapter 支持 HTTP/stdio/long-lived stdio，按 server policy 校验 allowed tools/resources/prompts。
- Lifecycle check: plugin `Setup` 只构造 adapter 并回填 runtime helper；module `Teardown` 调用 adapter `Close` 释放 long-lived stdio sessions。
- EventHub check: 不直接发布业务状态事件。
- Background task check: 不启动 framework background task；stdio session 生命周期由 adapter close 管理。
- State ownership check: 不直接批准风险或裁决 Run/Result；approval policy 只作为 catalog hint，实际治理由 kernel/appservice/orchestrator 消费。
- Workspace scope check: MCP request payload 携带 workspace/run/step refs；long-lived stdio session key 使用 workspace/server scope。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续核对 approval-required MCP 风险是否统一通过 governance/approval gate，而非 adapter 自行裁决。
- Verification: `go test ./internal/modules/blocks/mcp/... -count=1`
- Status: reviewed

### blocks/shell command adapter

- Code: `internal/modules/blocks/shell`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 在 workspace deliverable directory 内执行受限 shell 命令，返回 bounded output 和 failure classification。
- Current responsibility: plugin 从配置读取 shell timeout 并回填 runtime adapter；adapter 校验危险命令、限制执行目录、设置 timeout、输出 stdout/stderr/status/exitCode/evidence quality。
- Lifecycle check: plugin `Setup` 只构造 adapter 并回填 runtime helper；`Run` 无长期任务；`Teardown` 清空引用。
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
- Current responsibility: plugin 构造 file runtime adapter；adapter 通过 guarded workspace path 限制 deliverable directory，list/read 有条目/字节上限，edit 只产出 patch preview。
- Lifecycle check: plugin `Setup` 只构造 adapter 并回填 runtime helper；`Run` 无长期任务；`Teardown` 清空引用。
- EventHub check: 不直接发布业务状态事件。
- Background task check: 不启动后台任务。
- State ownership check: `file/edit` 不直接落盘 patch apply；正式 patch/result 状态仍由 orchestrator 处理。
- Workspace scope check: 所有路径必须位于 workspace deliverable directory 内。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续核对 orchestrator patch/apply 是否仍是唯一正式写入和事件发布入口。
- Verification: `go test ./internal/modules/blocks/file/... -count=1`
- Status: reviewed

### blocks/git repository adapter

- Code: `internal/modules/blocks/git`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 执行受限 git read/write 操作，输出结构化证据，不能越过 orchestrator。
- Current responsibility: plugin 从配置读取 git runtime enable/write/timeout 并回填 adapter；adapter 限制操作集、pathspec、timeout、输出大小和 changed file 数。
- Lifecycle check: plugin `Setup` 只构造 adapter 并回填 runtime helper；`Run` 无长期任务；`Teardown` 清空引用。
- EventHub check: 不直接发布业务状态事件。
- Background task check: 不启动后台任务。
- State ownership check: git write 只执行 workspace repo 操作并返回 step output，不直接完成 run/result。
- Workspace scope check: repo root 从 workspace deliverable directory 解析，pathspec 有边界校验。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续核对配置默认是否按环境明确启用/禁用 git write。
- Verification: `go test ./internal/modules/blocks/git/... -count=1`
- Status: reviewed

### blocks/web provider adapter

- Code: `internal/modules/blocks/web`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 调用受治理的 web search/fetch provider，返回来源证据，不写业务结论。
- Current responsibility: plugin 从配置构造 web runtime adapter 并注入 approval repository；adapter 执行 provider request、域名/IP policy、max results/fetch bytes/redirect/timeout、approval-required gating。
- Lifecycle check: plugin `Setup` 只构造 adapter 并回填 runtime helper；`Run` 无长期任务；`Teardown` 清空引用。
- EventHub check: 不直接发布业务状态事件。
- Background task check: 不启动后台任务。
- State ownership check: approval check 只验证已批准 approval 是否匹配请求，不创建/批准 approval，也不直接裁决 run/result。
- Workspace scope check: provider payload 和 output 携带 workspace/run/step refs。
- Findings: 未发现需要本轮代码收口的问题。
- Required action: 后续核对 web approval request 生成是否统一走 governance/approval kernel service。
- Verification: `go test ./internal/modules/blocks/web/... -count=1`
- Status: reviewed

### blocks/ssh remote command adapter

- Code: `internal/modules/blocks/ssh`
- Owner layer: Blocks runtime adapter
- Expected responsibility: 执行远端命令并返回 bounded output；风险治理由上层决定。
- Current responsibility: plugin 构造 SSH runtime adapter并回填 runtime helper；adapter 校验 host/user/auth/timeout，调用本地 ssh/setsid，隐藏密码，仅输出 command result。
- Lifecycle check: plugin `Setup` 只构造 adapter并回填 runtime helper；`Run` 无长期任务；`Teardown` 清空引用。
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
- Findings: 未发现需要本轮代码收口的问题；但 interface 仍保留全局 `Get(runID)`，调用方必须在 workspace-sensitive 路径使用 scoped helper 或显式 workspace list。
- Required action: 后续可评估增加 workspace-scoped `GetByWorkspaceID(workspaceID, runID)` 到 contract，进一步减少调用方约束。
- Verification: `go test ./internal/pkg/repository/... -count=1`
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
