# workorchd Application Module

Status: active

Type: application-module-contract

Last Verified: 2026-08-17

## 功能范围与边界

`application/workorchd` 是服务端应用模块，负责把 HTTP/Web API、CLI facade、runtime session、execution admission、context registry、read model、projection 和 SSE 接入统一生命周期。它组合多个 owner 的能力形成应用用例，但不拥有 Workspace、Run、Result、Decision、Approval、Memory、MCP、Skill 或 Soul 的正式状态。

## 组件交互及合同

- 只有 `biz.Biz` 保存 framework 注入的 magicCommon `event.Hub` 并拥有订阅生命周期；生产 HTTP/SSE transport 不保存 Hub 或 runtime session。Biz 通过七组 concrete named `ApplicationPorts` 向 server 注入应用操作，禁止 callback type alias、通用 dispatcher 或 owner service/repository。包内测试基础设施可以构造 owner runtime，但不得扩展生产 `Service` API 或提供 exported Session-backed server constructor；生产默认构造器 fail-closed。
- 对 Workspace、Run、Orchestrator、Result、Decision、Approval、Memory、MCP Registry、Skill、Soul、Governance 和 Eventstream 的读写必须发送投递方 `pkg/events` 定义的具体 command，并校验具体 result。
- HTTP server 退出时发布只含数据的 `ServiceStoppedData` notification；process service 在 framework `Run` 前建立 observer，并在自身维护 local wait state。EventHub 不传退出 channel、server 或 error object。`WorkspaceRunRequestedCommand` / `WorkspaceRunRequestedResult` 用于 scheduler 请求；`CLICommand` / `CLIResult` 用于 embedded CLI facade。
- `biz/execution.UseCase` 是 HTTP 异步 admission 与 Workorchd typed scheduler 同步 admission 的共同入口。它只规范化 request identity/scope、使用 framework `BackgroundRoutine`、提交 Workspace owner typed intent，并把 queued/owner outcome 交给 Biz-owned execution observation；同一 workspace/request 的 HTTP 重复请求只在有界进程窗口内 coalesce，持久化幂等仍归 owner；它不拥有 retry、Resume successor 或 lifecycle target state。
- `biz/projection.Component` 构造并拥有唯一 `workspaceviewcore.Manager`/Store 的 Start、Close 和后台任务生命周期。它只消费 Eventstream 的 `EventRecordAppendedData`、Orchestrator 的 immutable drive observation、owner snapshot 和 execution observation；server 只通过无生命周期的 `WorkspaceProjection` reader 与 derived-write port 输出 HTTP/SSE/metrics 或显式 rebuild 请求。生产 execution observation 的派生 upsert/queued/accepted/rejected/rebuild/delete 全部由 Biz-owned projection component 写入，server 仅接收独立的 transport diagnostics；`WorkspaceProjectionChangedData` 仍由 Biz 发布。
- HTTP、SSE 和 remote adapter 使用应用层 DTO；组件间 EventHub 传输不得复用 HTTP DTO。
- `biz/runtimeComponent` 是 Workorchd 内唯一长生命周期 runtime session owner；`runtimeApplicationPorts` 只保存借用函数，不复制 Session 指针，并通过 `ApplicationPorts` 向 Biz/use case 提供短生命周期借用，`service/server` 不接触 `runtimecore.Session`。`ManagedRuntime` 的 Eventstream 订阅、有限缓存与释放仍由 `biz/appservice/runtimecore` 实现，但其构造和关闭只由该组件持有；`pkg/observed` 仅保存快照 DTO 与纯映射，不提供 owner action、snapshot facade 或跨 owner service adapter；生产 bootstrap 不暴露 owner repository。
- workspace activation 使用 cumulative effective Goal/DoD 与 profile capability policy 构造 typed `RuntimeBootstrap`，并随 Run create command 交给 Run owner 持久化。
- activation 可以把 profile 显式声明的 `targetSelectorOwnerRef + targetSelectorSubjectRef` 封存为 File target-selection scope。空 workspace 的 scope 只表示该 profile 已授权 File owner 接受一个后续的具体 workspace-relative 创建目标；它不从 Goal/DoD 或目录名猜文件路径，且不绕过 File owner 的 operation admissibility/feasibility。
- activation 将每个 DoD item 标记为需要 typed assessment，并按 profile contract 的 required acceptance dimension/deliverable expectation 追加稳定 focus；primary state-change 优先于 supplemental evidence，同一 `scope + id` 只生成一个 baseline item。
- activation 可以把 profile 的 `RuntimeOwnerFactRequirement` 编译进 immutable DefinitionFocus；例如 software-dev
  `validation_preparation` 要求 `blocks/shell` 当前发布 selectable `validation/managed_session` surface。Workorchd 只做
  typed parsing、normalization 与 bootstrap propagation，不读取 capability repository/catalog、不判断事实是否满足，也
  不触发 CWI/semantic state transition。
- RuntimeBootstrap 的 typed activation context 直接保存 dialog language、默认及 stage Soul 内容快照、Skill 指令、MCP server/capability 选择、Memory、evidence policy，以及 state-change、capability-usage、typed validation、profile acceptance/deliverable/evidence、acceptance-slice、template stage policy execution contract；application 将系统 repair limit 和 matched validation retry limit 封存进新 Run convergence policy，Orchestrator 不读取 template map。
- acceptance compiler 对 focus 总数执行中立上限门禁；software-dev profile 将共享 direct-validation evidence 的 coverage item 按最多 16 个一组编译，避免大 DoD 被放大为近百个 validation/assessment LLM 循环。
- RuntimeBootstrap 的 typed definition context 保存 definition change impact，包括累积 Goal/DoD identity、原位修订记录、pre-execution assessment 要求与中立 strategy requirements；该快照直接进入 LLM C1。
- Biz 启动 workspace projection 后再启动 HTTP；Stop 先关闭 execution admission，再优雅关闭 transport，随后关闭 projection 并等待已接收后台任务，最后关闭 runtime session。不再运行禁止的 inactive-running workspace recovery，也不注册 stale-Step watchdog timer。
- Decision/Approval HTTP resolve 只提交对应 owner command 并刷新投影，不发布 Run Resume；Runtime Decision resolution 由 Decision owner 通过 typed EventHub command 直接提交 Orchestrator。
- run-scoped Start/Resume scheduler command 直接发送 Run owner 定义的 typed activation / continuation intent；Resume 只携带
  workspace/run scope，不再通过 CLI、session 或 Run port 发送 execution context patch。application 不维护 run
  pending/started queue，也不执行 Run 状态机或 runtime drive。
- workspace-scoped `resume`/`restart` 同步执行同一 activation 用例。`resume` 是严格 continuation：只携带
  workspace/run scope，保留原聚合与 sealed provider selection。`restart` 必须没有 RunID，并携带按当前
  Workspace definition 构造的 fresh bootstrap；Workspace owner 校验 definition/scope 后把 typed intent 交给 Run owner。
  Run owner 对普通非终态 predecessor 持久化 `ReplacementIntent`，并在既有 `Result -> Workspace -> Run` terminal
  delivery 完成后创建 successor；Application、Workspace 与 Orchestrator 均不提前创建或链接 successor。非当前
  development baseline 的 predecessor 由 Run owner返回 `data_reset_required`，不创建 replacement intent。
- restart compiler 只接受 Workspace identity 与进程默认值；Goal/DoD、provider、Soul、Skill、MCP、Memory 和 capability
  selection 只从当前 owner state 编译，不暴露 rich per-run activation command、显式 definition ID 或可跨包调用的
  activation builder。
- WorkspaceTemplate 中的 capability / version-control 声明只作为定义与 activation 输入；application 不直接执行 Git、Shell、File 或其他 capability 副作用。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `blocks/workspace` | Application → Workspace | definition/template CRUD、execution intent、terminal/read model query | 完整；Workspace owner 决定 definition 与 active slot |
| `blocks/run` | Application → Run | create/read、activation/continuation、recovery-safe lifecycle intent | 完整；Application 不指定目标状态 |
| `kernel/orchestrator` | Application → Orchestrator | Runtime read/drive observation、受限 purge/management query | 完整；不持有 drive/reducer |
| `blocks/llm` | Application → LLM | active config、catalog、refresh、selection validation、通用 completion/embedding | 完整；不直连 ai-proxy |
| Decision / Approval / Governance | 双向应用用例 | owner query/resolve 与 governance trace 展示 | 完整；resolve 不触发 Application Resume |
| Result / Memory / MCP Registry / Skill / Soul | Application → owner | 各 owner 的具体 CRUD/query；activation 只封存 detached snapshot | 完整 |
| Eventstream / projection | 双向只读链路 | event list/high-watermark/purge、appended/drive/execution observation | 完整；projection 不修复 owner |
| File / Git / Shell / SSH / Web / MCP execution | 无直接执行 I/O | capability operation 只由 Orchestrator sealed dispatch 调用 | 完整 |

## 相关约束

- application 只负责入口、用例编排、facade 和投影，不得成为正式状态 owner。
- HTTP、CLI、TUI、scheduler 和 projection 不得直接访问 owner repository 或 service。
- projection 只能读取和投影事件，不得修复 owner 状态或重建业务状态机。
- 跨组件 command/data/result 必须使用具体类型，不得引入 Envelope、通用 map payload、JSON 类型桥、reflect 或双 Hub 兼容层。
- repository builder、owner module fixture 和 repository 直写只能存在于 `biz/testkit` 或测试文件。
- application 可以定义 profile 到 focus/capability/prerequisite 的映射，但不得把该领域映射下沉到 kernel reducer。
- 没有显式 executable evidence requirement 的 supplemental deliverable 只能生成 assessment-only focus；不得为了关闭 coverage 制造报告文件或泛化 validation。声明 accepted step kind/operation 的 profile item 必须同步生成对应 capability prerequisite。
- 历史主链对象只保留为只读审计；application 不得为其取消、重派发、重建 continuation 或新增维护命令。
- application 不订阅 Decision/Approval resolved notification 来恢复 Run，不维护 governance pending、attempt、recovery scan 或 replan scheduler 分支。
- application event、HTTP/client、backend port、CLI/TUI 和 Web surface 已全部删除 `executionContextPatch`；历史上下文 patch 不得作为 activation、恢复 authority、兼容 DTO 或跨组件 payload。
- application bootstrap/session 构造器不接收或缓存 Runtime repair/output policy；Runtime 按需读取 canonical configuration，配置投影只用于管理接口展示。
- `Run.ExecutionContext`、activation execution-context population、resume patch 和对应 API/read-model fallback 已物理删除；activation 只允许构造 typed `RuntimeBootstrap`，治理、rejected-result 与 continuation 只允许进入具体 Runtime 合同。
- application 不保留 `scheduleWatchdog`、stale dispatched recovery、startup inactive Run recovery 或 stuck Run recovery helper；watchdog interval audit 为 `0`。
- application 不提供 orphan Step、stale dispatched Run 或 runtime-gap 的恢复 API；Runtime 重启推进由 Run owner
  的 SM-1 state + opaque RuntimeStateRecord inventory/notification 触发 Orchestrator internal drive；SM-4 typed load
  aggregate 后 owner-local 分类并幂等恢复。
- application 不保留 execution pending/started、owner readback retry 或 scheduler recovery map；请求只有在 typed owner command 成功接收后才返回 accepted。

## 待完善测试点

- [ ] 为七组 `ApplicationPorts` 覆盖缺失 port、nil/wrong owner result、scope mismatch 与 transport-safe error mapping。
- [ ] 覆盖 HTTP/scheduler 同一 request 的进程内 coalesce、owner 持久化幂等及 Stop 后拒绝新 admission。
- [ ] 覆盖 Stop 顺序：admission → transport → projection/background tasks → runtime session，并验证重复 Stop 幂等。
- [ ] 覆盖大量 Goal/DoD 的语义排序、分批 focus 编译、16 项 acceptance slice 上限与 context budget 不全量复制。
- [ ] 覆盖 projection lag、high-watermark readiness、显式 rebuild 和重启，确认不向任何 owner 回写修复状态。
- [ ] 覆盖 resume 不携带 patch、restart fresh bootstrap、replacement intent 与 successor 均不在 Application 创建。
- [ ] 覆盖 activation 中 Soul/Skill/MCP/Memory/profile DTO 深拷贝，以及 registry 后续更新不改变已 sealed Run。
- [ ] 增加生产边界扫描，确认 server/transport 不持有 Hub、Session、owner service/repository 或 capability executor。
