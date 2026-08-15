# workorchd Application Module

## 功能说明

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
- RuntimeBootstrap 的 typed activation context 直接保存 dialog language、默认及 stage Soul 内容快照、Skill 指令、MCP server/capability 选择、Memory、evidence policy，以及 state-change、capability-usage、typed validation、profile acceptance/deliverable/evidence、acceptance-slice、template stage policy execution contract；application 将系统 repair limit 和 matched validation retry limit 封存进新 Run convergence policy，Orchestrator 不读取 template map。
- acceptance compiler 对 focus 总数执行中立上限门禁；software-dev profile 将共享 direct-validation evidence 的 coverage item 按最多 16 个一组编译，避免大 DoD 被放大为近百个 validation/assessment LLM 循环。
- RuntimeBootstrap 的 typed definition context 保存 definition change impact，包括累积 Goal/DoD identity、原位修订记录、pre-execution assessment 要求与中立 strategy requirements；该快照直接进入 LLM C1。
- Biz 启动 workspace projection 后再启动 HTTP；Stop 先关闭 execution admission，再优雅关闭 transport，随后关闭 projection 并等待已接收后台任务，最后关闭 runtime session。不再运行禁止的 inactive-running workspace recovery，也不注册 stale-Step watchdog timer。
- Decision/Approval HTTP resolve 只提交对应 owner command 并刷新投影，不发布 Run Resume；Runtime Decision resolution 由 Decision owner 通过 typed EventHub command 直接提交 Orchestrator。
- run-scoped Start/Resume scheduler command 直接发送 Run owner 定义的 typed activation / continuation intent；Resume 只携带
  workspace/run scope，不再通过 CLI、session 或 Run port 发送 execution context patch。application 不维护 run
  pending/started queue，也不执行 Run 状态机或 runtime drive。
- workspace-scoped `resume`/`restart` 同步执行同一 activation 用例。`resume` 是严格 continuation：非终态且
  Goal/DoD 与封存 provider/model 未漂移时保留原聚合，绝不隐式 replacement。`restart` 必须没有 RunID；普通 active
  Run 仍走 Orchestrator sealed supersede，而 `fresh_activation_required` Run 由 application 仅转发已持久化的
  Run fact 给 Workspace owner。Workspace 只释放精确匹配的 `ActiveRunID`，application 随后创建独立 current-schema
  Run；它不终态化、迁移、读取或链接旧 aggregate。TUI 同时展示这两个显式入口。Resume 不注入 provider/model 或
  definition patch。该编排不让 application 选择 SM-2 next action，也不绕过 SM-3 sealed dispatch。
- WorkspaceTemplate 中的 capability / version-control 声明只作为定义与 activation 输入；application 不直接执行 Git、Shell、File 或其他 capability 副作用。

### 当前内聚收口切片（2026-08-16）

- `readmodelprojection` 仍只有一个 manager/store/revision 规则；微型 mapper 已机械归并为 140 个生产文件，workspace/run/context/evidence/activity/governance
  六族已通过 typed facade 统一 application 调用；公开面精确限定为六个 facade factory 和两个 snapshot input，
  其余 helper 全部 package-private。原文件名保留在 consolidated 文件注释中，33 个临时 `projection_family_NN.go`
  已全部改为职责语义名，未改变 projection DTO 或 high-watermark 语义。
- `definitionquality`、`content`、`setupdraft` 分别吸收原 `core/ports` 薄配对；这些包只提供 typed application
  facts/reader/generator，不拥有正式状态机，也不创建新的 framework 运行单元。
- `biz/workspaceCommandGateway` 是 send-only concrete Workspace intent gateway；它不选择 focus、重试、Resume 或
  Runtime transition。`production_transport_boundary_test.go` 与 repository boundary gate 防止 server 重新传播
  runtimecore/EventHub/root appservice、Session/cache authority 或 projection lifecycle。
- `ApplicationPorts.Validate` 只按 Workspace、Definition、Run、Governance、Registry、Memory、Configuration 七组
  named validator 顺序 fail-closed；Workspace HTTP 注册按 resource/projection/context/definition 与
  create/inference/delete/execution/context/definition endpoint family 分段，route/method/DTO/status code 不变。
- `workspaceviewcore.Store`、唯一 `sync.RWMutex` 与 revision/high-watermark authority 仍只在 `store.go` 声明；其
  lifecycle/status、revision、mutation、query、metrics、clone 实现按语义分文件。`readmodelprojection` 的 workspace
  diff/equality/convergence normalization 也按纯规则分段，未新增 projection state、I/O 或 export。
- WD0-WD8 与 C22-04 已完成 code/automated closure；当前指标为 Workorchd 659 个生产文件/56,414 LOC、server 42 个
  生产文件/8,230 LOC、readmodelprojection 142 个生产文件/17,187 LOC/8 个 exported declaration。deployed HTTP/SSE/CLI
  regression 与 SO-MC-10 fresh-data live proof 仍是 active。

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
