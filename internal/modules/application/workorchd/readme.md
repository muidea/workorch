# workorchd Application Module

Status: active

Type: application-module-contract

Last Verified: 2026-08-23

## 功能范围与边界

`application/workorchd` 是服务端应用模块，负责把 HTTP/Web API、CLI facade、runtime session、execution admission、context registry、read model、projection 和 SSE 接入统一生命周期。它组合多个 owner 的能力形成应用用例，但不拥有 Workspace、Run、Result、Decision、Approval、Memory、MCP、Skill 或 Soul 的正式状态。

## 组件交互及合同

- framework 注入的 magicCommon `event.Hub` 只进入 `biz/` 子树：`biz.Biz` 内嵌 Base 并拥有顶层订阅生命周期，`workspaceCommandGateway`、CLI dispatcher、runtime event collector 和 focused Biz port 可以按职责保存 Hub/Base；module root 与生产 HTTP/SSE transport 不保存 Hub 或 runtime session。Biz 通过七组 concrete named `ApplicationPorts` 向 server 注入应用操作，禁止 callback type alias、通用 dispatcher 或 owner service/repository。包内测试基础设施可以构造 owner runtime，但不得扩展生产 `Service` API 或提供 exported Session-backed server constructor；生产默认构造器 fail-closed。
- 对 Workspace、Orchestrator、Result、Decision、Approval、Memory、MCP Registry、Skill、Soul、Governance 和 Eventstream 的读写，以及对 Run 的读取，必须发送对应 owner `pkg/events` 定义的具体 command，并校验具体 result。execution mutation 只提交 Workspace owner；HTTP `RunPorts` 全部为只读端口。
- HTTP server 退出时发布只含数据的 `ServiceStoppedData` notification；process service 在 framework `Run` 前建立 observer，并在自身维护 local wait state。EventHub 不传退出 channel、server 或 error object。`WorkspaceRunRequestedCommand` / `WorkspaceRunRequestedResult` 是 Workorchd typed execution ingress；embedded CLI facade 使用按操作拆分的具体 command/result，例如 `CLIWorkspaceCreateCommand` / `CLIWorkspaceCreateResult`，不存在通用 `CLICommand` / `CLIResult`。
- Biz-owned `executionAdmissionGateway` 先将 HTTP 请求规范化并发送
  `TopicWorkorchDCommandWorkspaceRunRequested`；HTTP、CLI 与 TUI 因而在同一 workspace-scoped Workorchd lane 内调用
  `biz/execution.UseCase`，且该 lane 覆盖 Workspace owner 同步 admission、具体 result 校验与 accepted/rejected 派生投影
  的完整事务。它不保存 queue、pending/active request、进程内业务 dedupe、retry、Resume successor 或 lifecycle target
  state；重复 request 必须每次交给 Workspace/Run owner 持久化幂等合同裁决，projection Store 只保证同 request identity
  的 immutable observation 不被重复或矛盾回执改写。
- `biz/projection.Component` 构造并拥有唯一 `workspaceviewcore.Manager`/Store 的 Start、Close 和后台任务生命周期。它只消费 Eventstream 的 `EventRecordAppendedData`、Orchestrator 的 immutable drive observation、owner snapshot 和 execution observation；server 只通过无生命周期的 `WorkspaceProjection` reader 与 derived-write port 输出 HTTP/SSE/metrics 或显式 rebuild 请求。生产 execution observation 的派生 accepted/rejected/rebuild/delete 全部由 Biz-owned projection component 写入，不存在 `queued` observation 或 `queuedAt`；server 仅接收独立的 monotonic transport metrics；`WorkspaceProjectionChangedData` 仍由 Biz 发布。
- opaque `RuntimeStateRecord` 只允许作为一次 projector 调用的瞬时输入；`workspaceviewcore` Store、application read model 与 HTTP cache 只能保存 bounded immutable public DTO，不能保存 aggregate、其指针或可用于 re-drive 的 authority-shaped 副本。
- Workspace/Run projection 原样保留各 owner 的 canonical `state`、Workspace `activeRunId` 与 Run `blockingTicketId|blockingApprovalId`。Workspace DTO 不暴露 blocker 字段；Decision/Approval 的跨 owner 关联只进入 `governanceDisplay`，不一致只进入 `consistencyDiagnostic`。`displayOutcome` 是非权威展示摘要，formal output 始终是独立 `result/results`；三类 display/diagnostic 字段均不能生成、阻断或选择 `Drive/Resume/Restart` mutation intent，也不能伪造、隐藏或选择治理 resolution target。`WorkspaceRuntimeView.blockingDecision|blockingApproval` 只有在 canonical active Run 已进入匹配 blocked state、其 exact blocker ref 命中同 Workspace/Run scope 的真实 open/pending owner resource 时才可出现；同 Run 的其他未决资源和 `governanceDisplay` ID 均不能替代该引用。
- `TaskProgress.recovery`、ContextBundle 与 compressed context 复用同一 factual `BlockingResourceRef`。它只在上述 exact
  canonical 交集成立时出现，并逐字段复制 owner/resource/scope/state 及 owner 提供的 `requiredAction/allowedUserActions`；
  `suggestedNavigation` 是独立 display hint。它们不构造 mutation。SM-2 repair 只投影在 `RepairLoop`/convergence budget，
  SM-4 transport、operation、CAS 与 terminal delivery 只投影在独立 recovery budget/terminal audit，禁止互相镜像。
- Decision owner 的`DefinitionClarification`只经bounded typed `Decision.definitionClarification`投影，完整保留class、binding、
  required action、gap/offending refs与candidate/runtime/resolution lineage identity，并deep-clone `offendingRefs`。该DTO仅供
  Queue/Workspace Detail/TUI展示和workspace-scoped Definition修订导航；Workorchd不从Goal/DoD、SSE或display字段推导
  Restart freshness/admission，不把它送入generic resolve。Board零Definition execution command；详情页显式Restart只进入
  统一Workspace owner admission并原样保留typed rejection。
- `snapshot.runCommand` 只有 `ApplyWorkspaceExecutionAdmission(command metadata, exact RuntimeExecutionIntentAdmission)` 一个
  projection mutation。该入口先执行 owner admission `Validate`，再调用共享 `PresentAdmission` 纯派生 `state`、
  `diagnostic` 与 `acceptedAt|rejectedAt`；caller 无法选择 state 或 revision，Store 在同一个加锁事务中把
  `advanceEntryRevision` 的结果同时写入 entry 与 runCommand。read DTO 只保留
  `workspaceId/runId/state/intent/requestId/requestReason/source/outcome/admissionReason/diagnostic/acceptedAt|rejectedAt/revision`，
  不保留 `disposition/reason/error/definitionRevision/decisionId` shadow。
- projection Store lifecycle 是 Workorchd-owned 的派生状态机：rebuild requested/completed、mutation applied、degraded、
  failed 与 stopped 只能经过一个 private-event reducer 写入。`stopped` 对 late mutation/completion/failure 保持终态；
  `failed` 只能由成功提交的 owner mutation 或显式 rebuild 恢复。该状态只描述 read-model 健康度，不能反向修复
  任何 owner authority。
- Runtime audit 的 sealed-operation 视图只投影 operation identity、binding、`ExecutionKey` 与 `Fence` 等只读关联字段，使 governance continuation 后的 accepted receipt 仍按聚合中保存的原始 logical execution identity 精确核对；它不投影 operation 参数，不生成 dispatch，也不得作为 reducer/capability 可执行 authority。
- HTTP、SSE 和 remote adapter 使用应用层 DTO；组件间 EventHub 传输不得复用 HTTP DTO。
- `RunStateChanged` 的 process-local metrics/cache invalidation 入口只接受 `source=blocks/run` 且通过 typed `Validate` 的通知；Eventstream replay、伪造 source 或 invalid data 均不能成为 Run owner fact。
- MCP 管理列表通过 owner 的 cursor/limit concrete result 分页，并要求所有页面保持同一 `RegistryRevision`；fresh activation
  用一次 `ReadSelectionSnapshotCommand` 同时取得并验证 selected Server/Capability。管理 DTO 只公开 `environmentRefs`，不接受、缓存或投影 credential 明文。
- `biz/runtimeComponent` 是 Workorchd 内唯一长生命周期 runtime session owner；`runtimeApplicationPorts` 只保存借用函数，不复制 Session 指针，并通过 `ApplicationPorts` 向 Biz/use case 提供短生命周期借用，`service/server` 不接触 `runtimecore.Session`。`ManagedRuntime` 的 Eventstream 订阅、有限缓存与释放仍由 `biz/appservice/runtimecore` 实现，但其构造和关闭只由该组件持有；`pkg/observed` 仅保存快照 DTO 与纯映射，不提供 owner action、snapshot facade 或跨 owner service adapter；生产 bootstrap 不暴露 owner repository。
- workspace activation 使用 cumulative effective Goal/DoD 与 profile capability policy 构造 typed `RuntimeBootstrap`，并将它封装进 workspace-scoped restart intent；Workspace owner 校验 definition/scope 后再把 typed intent 交给 Run owner，Application 不直接发送 Run create command。
- RuntimeBootstrap 编译前，application 先把 exact Goal、DoD、Template snapshot 与 resolved workspace directory 组装为
  Workspace-owned immutable `DefinitionAdmissionCandidate`。pure compiler 不调用 LLM/capability，只接受完整 `all_of`；
  candidate 同步封存 required owner-fact binding、唯一 one-shot acquisition/reobserve closure 及 Profile contract hash；
  `one_of/any_of/quorum/threshold/first_success`、缺失/重复/成环的 owner-fact closure 或其他 unresolved declared prerequisite
  通过 Decision owner 专用 typed ticket
  fail-closed。新 invalid revision（包括既有 eligible revision 之后的 authoring attempt）使用 reconcile-candidate；新 admitted revision严格执行 Workspace promotion(pending) →
  Decision resolution → Workspace acknowledgement(eligible)，之后仍需用户显式 Restart。
- activation 可以把 profile 显式声明的 `targetSelectorOwnerRef + targetSelectorSubjectRef` 封存为 File target-selection scope。空 workspace 的 scope 只表示该 profile 已授权 File owner 接受一个后续的具体 workspace-relative 创建目标；它不从 Goal/DoD 或目录名猜文件路径，且不绕过 File owner 的 operation admissibility/feasibility。
- activation 将每个 DoD item 标记为需要 typed assessment，并按 profile contract 的 required acceptance dimension/deliverable expectation 追加稳定 focus；primary state-change 优先于 supplemental evidence，同一 `scope + id` 只生成一个 baseline item。
- activation 可以把 profile 的 `RuntimeOwnerFactRequirement` 编译进 immutable DefinitionFocus；例如 software-dev
  `validation_preparation` 要求 `blocks/shell` 当前发布 selectable `validation/managed_session` surface。Workorchd 只做
  typed parsing、normalization、pre-seal closure 验证与 bootstrap propagation，不读取 capability repository/catalog、不判断
  事实是否满足，也不触发 CWI/semantic state transition。默认 software profile 封存一次 `blocks/file file/write`
  acquisition 后由 `blocks/shell` reobserve 的 typed 合同；SM-2 只能消费该合同，不能在 kernel 中硬编码 Go 或测试文件。
- RuntimeBootstrap 的 typed activation context 直接保存 dialog language、默认及 stage Soul 内容快照、Skill、MCP server/capability 选择、Memory、evidence policy，以及 state-change、capability-usage、typed validation、profile acceptance/deliverable/evidence、acceptance-slice、template stage policy execution contract。Skill 明确区分 WorkOrch Registry 与外部 SkillHub Catalog provenance，并在进入 Run bootstrap 前封存最多 5 项、单项 instruction 8 KiB、instruction 总计 24 KiB、metadata 总计 16 KiB 的 C3 byte budget；只有 instruction 可做确定性 UTF-8 裁剪，来源身份与原始长度随 snapshot 保存。Memory 同时封存 bounded candidate/selected/excluded、quota、score/hit 与 byte/token budget provenance；这些 provenance 只解释 context selection，不参与 SM-2 credit。application 将系统 repair limit 和 matched validation retry limit 封存进新 Run convergence policy，Orchestrator 不读取 template map。
- acceptance compiler 对 focus 总数执行中立上限门禁。software-dev profile 的 `direct_validation` 分组每个 slice 最多 4 个 work item，全部 focus 最多 64 个；slice 只约束 presentation/candidate context，每个 CWI 仍生成独立 focus，不能据此声称减少 assessment 循环。
- RuntimeBootstrap 的 typed definition context 保存 definition change impact，包括累积 Goal/DoD identity、原位修订记录、pre-execution assessment 要求与中立 strategy requirements；该快照直接进入 LLM C1。
- Runtime definition context 同时保存 Workspace owner 签发的 `StartupInputHash`。Restart 必须与 current eligible admission
  精确匹配；Resume 不读取最新 Goal/DoD，也不把后续 candidate/promotion 注入既有 Run。
- activation同时封存与repair/recovery source policy一致的`RuntimeExecutionBudgetPolicy`。read-model从opaque Runtime
  aggregate纯派生`RuntimeExecutionBudgetView`与active `RuntimeRecoverableWaitView`，明确区分recoverable wait、
  governance blocker和terminal；Application、HTTP、SSE、Web/TUI不提供retry-now、clear wait、top-up或预算回写入口。
- Biz 启动 workspace projection 后再启动 HTTP；execution admission 是无状态同步 use case，没有独立 lifecycle。Stop 先封闭统一 admission barrier、停止 CLI 并 drain HTTP，再等待全部 ingress 的 in-flight admission 完成；只有在 Workorchd topic、use case 与 projection 不再被占用后才解除订阅、清理 port、关闭 projection 并等待其后台任务，最后关闭 runtime session。不再运行禁止的 inactive-running workspace recovery，也不注册 stale-Step watchdog timer。只有经过校验的 Workspace owner result 才形成 immutable `snapshot.runCommand.state=accepted|rejected` 观测；context、transport、bootstrap 或 result-contract failure 只同步返回错误并累加无请求身份的 monotonic failure counter，绝不伪装成 owner rejection。该投影没有 running/completed/no-action 迁移、readback recovery 或 owner command 能力。
- Decision/Approval HTTP resolve 只提交对应 owner command 并刷新投影，不发布 Run Resume；Runtime Decision resolution 由 Decision owner 通过 typed EventHub command 直接提交 Orchestrator。新的open/pending transition在owner write前通过typed Workspace/Run read与canonical active Run exact blocker做application admission；single/batch的extra或stale resource typed conflict且batch零部分写入。pre-seal/runtime Definition clarification永久拒绝generic resolve。已terminal的exact response-loss duplicate仍交给Decision/Approval owner按完整payload/action/context判定幂等，application不重判owner终态。
- 生产路径不存在 run-scoped Start/Resume mutation 或直达 Run owner 的 application-local execution command；唯一 execution ingress 是
  workspace-scoped `resume`/`restart`，由 `workspaceCommandGateway` 提交 Workspace owner。`RunPorts` 只提供 Run、snapshot、
  status 和关联资源读取；Application 不维护 run pending/started queue，也不执行 Run 状态机或 runtime drive。
- workspace-scoped `resume`/`restart` 进入同一 execution admission 用例。`resume` 是严格 continuation：只携带
  workspace/request identity 与 admission metadata，不携带 bootstrap；当前 Run 由 Workspace owner 解析，原聚合与
  sealed provider selection 保持不变。`restart` 携带按当前 Workspace definition 构造的 fresh bootstrap；
  execution intent 本身不含 RunID，Workspace owner 校验 definition/scope 后把 typed intent 交给 Run owner。
  Run owner 对普通非终态 predecessor 持久化 `ReplacementIntent`，并在既有 `Result -> Workspace -> Run` terminal
  delivery 完成后创建 successor；Application、Workspace 与 Orchestrator 均不提前创建或链接 successor。无法按当前
  persisted shape 读取的 predecessor 在 admission 前失败，不生成 application compatibility outcome 或 replacement intent。
- restart compiler 只接受 Workspace identity 与进程默认值；Goal/DoD、provider、Soul、Skill、MCP、Memory 和 capability
  selection 只从当前 owner state 编译，不暴露 rich per-run activation command、显式 definition ID 或可跨包调用的
  activation builder。
- WorkspaceTemplate 中的 capability / version-control 声明只作为定义与 activation 输入；application 不直接执行 Git、Shell、File 或其他 capability 副作用。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `blocks/workspace` | Application → Workspace | definition/template CRUD、candidate admission/promotion/ack、execution intent、terminal/read model query | 完整；Workspace owner 决定 startup input、eligibility 与 active slot |
| `blocks/run` | Application → Run | Run/snapshot/status/关联资源读取；显式 workspace purge 的 owner cleanup | 完整；`RunPorts` 只读，execution intent 不直达 Run owner |
| `kernel/orchestrator` | Application → Orchestrator | Runtime read/drive observation、受限 purge/management query | 完整；不持有 drive/reducer |
| `blocks/llm` | Application → LLM | active config、catalog、refresh、selection validation、通用 completion/embedding | 完整；不直连 ai-proxy |
| Decision / Approval / Governance | 双向应用用例 | owner query/resolve 与 governance trace 展示 | 完整；resolve 不触发 Application Resume |
| Result / Memory / MCP Registry / Soul | Application → owner | 各 owner 的具体 CRUD/query；MCP paged read 与一致 selection snapshot；activation 只封存 detached snapshot | 完整 |
| Skill Registry / SkillHub Catalog | Application → owner/catalog | Registry concrete create/get/bounded-list；SkillHub external catalog read；typed provenance + sealed activation budget | 完整；SkillHub 不是 Skill Block 正式状态 |
| Eventstream / projection | 双向只读链路 | event list/high-watermark/purge、appended/drive/execution observation | 完整；projection 不修复 owner |
| File / Git / Shell / SSH / Web / MCP execution | 无直接执行 I/O | capability operation 只由 Orchestrator sealed dispatch 调用 | 完整 |

## 相关约束

- application 只负责入口、用例编排、facade 和投影，不得成为正式状态 owner。
- HTTP、CLI、TUI、execution-admission adapter 和 projection 不得直接访问 owner repository 或 service。
- projection 只能读取和投影事件，不得修复 owner 状态或重建业务状态机。
- 跨组件 command/data/result 必须使用具体类型，不得引入 Envelope、通用 map payload、JSON 类型桥、reflect 或双 Hub 兼容层。
- repository builder、owner module fixture 和 repository 直写只能存在于 `biz/testkit` 或测试文件。
- application 可以定义 profile 到 focus/capability/prerequisite 的映射，但不得把该领域映射下沉到 kernel reducer。
- 没有显式 executable evidence requirement 的 supplemental deliverable 只能生成 assessment-only focus；不得为了关闭 coverage 制造报告文件或泛化 validation。声明 accepted step kind/operation 的 profile item 必须同步生成对应 capability prerequisite。
- 历史主链对象只保留为只读审计；application 不得为其取消、重派发、重建 continuation 或新增维护命令。
- application 不订阅 Decision/Approval resolved notification 来恢复 Run，不维护 governance pending、attempt、recovery scan 或 application replan 分支。
- application event、HTTP/client、backend port、CLI/TUI 和 Web surface 已全部删除 `executionContextPatch`；历史上下文 patch 不得作为 activation、恢复 authority、兼容 DTO 或跨组件 payload。
- application bootstrap/session 构造器不接收或缓存 Runtime repair/output policy；Runtime 按需读取 canonical configuration，配置投影只用于管理接口展示。
- `server` 配置只包含 HTTP transport 与默认对话设置；已退休的 application run 并发上限和 stale-dispatch watchdog key 必须在 active/persisted 配置加载及任一配置 update 的 pre-read 阶段作为 unknown field fail-closed，不能被更新操作静默删除或保留为无效控制面。
- `Run.ExecutionContext`、activation execution-context population、resume patch 和对应 API/read-model fallback 已物理删除；activation 只允许构造 typed `RuntimeBootstrap`，治理、rejected-result 与 continuation 只允许进入具体 Runtime 合同。
- application 不保留 `scheduleWatchdog`、stale dispatched recovery、startup inactive Run recovery 或 stuck Run recovery helper；对应 watchdog/continuation projection 与 metrics 已删除，旧指标再次出现必须由 closure observer 判为失败。
- application 不提供 orphan Step、stale dispatched Run 或 runtime-gap 的恢复 API；Runtime 重启推进由 Run owner
  的 SM-1 state + opaque RuntimeStateRecord inventory/notification 触发 Orchestrator internal drive；SM-4 typed load
  aggregate 后 owner-local 分类并幂等恢复。
- execution admission 已删除 caller-controlled Run priority：HTTP body/query/header、Workorchd command、client/backend、CLI/TUI、Web 与 public Run projection 均不再携带该字段；acceptance slice、context compaction 与 Soul routing priority 保持各自独立语义。
- application 不保留 execution queue、pending/started、owner readback retry 或 application recovery map，也不按 intent/workspace 拼装第二份 `idempotencyKey`；owner 使用的 `requestId` 是唯一请求身份。HTTP 收到并校验具体 Workspace owner result 后，只有 `reconciliation_requested|replacement_pending` 返回 `202`，其余五类 canonical outcome 返回携带 exact identity/outcome/admissionReason 的 typed `409`。唯一 pure classifier 同时给 HTTP 与 projection 提供 accepted/rejected 分类；client、Web/TUI 只显示该分类，不重判 outcome。没有具体 owner result 的同步失败直接映射为当前 HTTP error，不写 `runCommand`。
- LLM usage projection 原样保留 Eventstream 的 optional `logicalRequestId`/`physicalCallId` 双层身份，并以
  `physicalResponseMode=streaming|non_streaming` 与 `providerSubmitted` 为唯一物理调用审计输入。Runtime usage 必须
  同时携带两级身份且 `physicalCallId=requestId`；generic non-Runtime 记录可以两者都没有，但只出现一级或相关 ID 不一致
  必须拒绝。公共 DTO 不含 `streaming` 布尔字段；不得从 request bytes 或其他展示事实补造身份/提交状态。
- setup-draft session 只保存 message 与 Draft fact，API `state` 每次从 Draft 纯派生，不持久化第二份 authoring lifecycle。acceptance projection 只从 Runtime aggregate 派生 active slice；已有 sealed terminal 或 canonical terminal state 时不得继续标记 active slice/focus。

## 测试状态

已覆盖：

- [x] 七组 `ApplicationPorts` 的每个缺失 port 都会在 transport 启动前 fail-closed。
- [x] HTTP handler 经 workspace-scoped Workorchd lane 等待 Workspace owner 同步 admission；成功前零 `queued` 投影，owner 失败在当前请求返回；同 workspace 并发结果不能反序提交，重复 request 每次交由 owner 幂等裁决且不改写既有 immutable observation。
- [x] 大 DoD 的稳定语义排序、slice 分组、每 CWI 独立 focus 以及 focus 总数上限门禁。
- [x] projection lag/high-watermark reconciliation、显式 rebuild、重启 generation 与“不得向 owner 回写修复”。
- [x] projection lifecycle 合法/禁止迁移、stopped late-event noop、failed 经成功 owner mutation/显式 rebuild 恢复与
  未知 state/event 拒绝。
- [x] run-scoped execution route 已移除，workspace-scoped resume/restart 进入 Workspace owner，且 restart 使用 fresh bootstrap。
- [x] owner-lane 测试确认 HTTP/CLI/TUI execution ingress 的 owner admission 与 projection commit 共享 workspace-scoped Workorchd lane，并确认 execution gateway 投递 Workspace owner；聚合边界校验确认唯一 HTTP execution write route 为 workspace-scoped `execution-activations`，Run HTTP route 保持只读。
- [x] 生产 transport AST 扫描确认 `service/server` 不导入或保存 EventHub、runtime Session、root appservice 兼容状态；聚合边界校验禁止 workorchd 生产代码直接导入 repository。
- [x] activation Memory selection 执行 candidate/final item、type quota、byte/token hard limit；完整 selection evidence 深拷贝后进入 sealed Runtime activation，并与最终 Memory identity 精确匹配。
- [x] Skill Registry 使用 concrete create/get/bounded-list result 并由 owner 签发 canonical provenance；SkillHub Catalog 保持独立来源，activation 在 LLM C3 前执行 Skill 数量、instruction/metadata byte budget 与 UTF-8 裁剪。
- [x] Runtime execution-budget的source-ledger聚合、active wait/terminal projection分离与只读clone路径。
- [x] Runtime audit 对 sealed operation 显式投影 `ExecutionKey + Fence`，覆盖 governance resume 后 evidence dispatch 与原始 logical execution identity 不同的 receipt 核对，同时保持参数和可执行 authority 不出投影。
- [x] Workspace directory resolver 对空输入生成隔离的 `.workorch-workspaces/<workspace-id>`，规范化显式相对/绝对
  目录并拒绝相对逃逸或不安全 workspace path segment。

闭合验证：

- [x] concrete owner response decoder 矩阵覆盖 nil、wrong concrete result 与 owner error cause；七组 `ApplicationPorts`
  的代表 HTTP 路由覆盖 transport-safe error mapping，workspace/request/intent 以及现有 owner read/mutation 测试覆盖
  scope mismatch 并 fail-closed。
- [x] Run owner 的 file-backed restart 重放跨 owner restart 仍只保留一个 `ActivationRequestID` 对应的 Run；changed
  payload 冲突。execution use case 对缺失 dependency、invalid intent 和 owner result scope mismatch 均 fail-closed 且无 retry/queue side effect。
- [x] AST 顺序测试固定 Stop 为 transport → projection/background tasks → runtime session，并确认 admission port 在 transport 完成 graceful shutdown 后才清理；真实 Setup
  后重复 Stop 幂等并清空所有生命周期引用。
- [x] 512 份、单份 32 KiB 的 cumulative Goal/DoD 压力用例确认 activation 只传 identity/revision，不复制稳定正文，
  definition change context 保持有界。
- [x] 生产静态边界扫描禁止 `ApplicationPorts.Run` 引入 mutation，禁止 transport 导入其他 owner Biz/service 或
  capability executor，并禁止 Application 构造 `ReplacementIntent`、Run successor。
- [x] Soul/MCP/profile 的 DTO 与 activation clone 深拷贝已覆盖；file-backed 真实 restart admission 在 Run owner
  重启、Registry/Catalog/caller DTO 更新后仍读取原 sealed snapshot，exact replay 命中同一 Run。

当前没有已知的 executable contract gap；新增接口或 owner 交互必须同时扩展上述边界与回归矩阵。
