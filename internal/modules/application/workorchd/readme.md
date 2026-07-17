# workorchd Application Module

## 功能说明

`application/workorchd` 是服务端应用模块，负责把 HTTP/Web API、CLI facade、runtime session、scheduler、context registry、read model、projection 和 SSE 接入统一生命周期。它组合多个 owner 的能力形成应用用例，但不拥有 Workspace、Run、Result、Decision、Approval、Memory、MCP、Skill 或 Soul 的正式状态。

## 组件交互及合同

- 模块只使用 framework 注入的 magicCommon `event.Hub`；HTTP、CLI、scheduler、projection 和 owner command 共用同一个 Hub。
- 对 Workspace、Run、Orchestrator、Result、Decision、Approval、Memory、MCP Registry、Skill、Soul、Governance 和 Eventstream 的读写必须发送投递方 `pkg/events` 定义的具体 command，并校验具体 result。
- `pkg/events.DoneCommand` / `DoneCommandResult` 用于查询服务退出通道；`WorkspaceRunRequestedCommand` / `WorkspaceRunRequestedResult` 用于 scheduler 请求；`CLICommand` / `CLIResult` 用于 embedded CLI facade。
- `WorkspaceProjectionChangedData` 由本模块发布；workspace projection 只消费 Eventstream 的 `EventRecordAppendedData` 和 owner 快照。
- HTTP、SSE 和 remote adapter 使用应用层 DTO；组件间 EventHub 传输不得复用 HTTP DTO。
- runtime session 只持有应用基础设施和唯一 EventHub；`ManagedRuntime` 仅订阅 Eventstream 的 typed record notification 并提供进程内观测快照，不提供 owner action、snapshot facade 或跨 owner service adapter；生产 bootstrap 不暴露 owner repository。
- workspace activation 使用 cumulative effective Goal/DoD 与 profile capability policy 构造 typed `RuntimeBootstrap`，并随 Run create command 交给 Run owner 持久化。
- activation 将每个 DoD item 标记为需要 typed assessment，并按 profile contract 的 required acceptance dimension/deliverable expectation 追加稳定 focus；primary state-change 优先于 supplemental evidence，同一 `scope + id` 只生成一个 baseline item。
- RuntimeBootstrap 的 typed activation context 直接保存 dialog language、默认及 stage Soul 内容快照、Skill 指令、MCP server/capability 选择、Memory、evidence policy，以及 state-change、capability-usage、typed validation、profile acceptance/deliverable/evidence、acceptance-slice、template stage policy execution contract；application 将系统 repair limit 和 matched validation retry limit 封存进新 Run convergence policy，Orchestrator 不读取 template map。
- acceptance compiler 对 focus 总数执行中立上限门禁；software-dev profile 将共享 direct-validation evidence 的 coverage item 按最多 16 个一组编译，避免大 DoD 被放大为近百个 validation/assessment LLM 循环。
- RuntimeBootstrap 的 typed definition context 保存 definition change impact，包括累积 Goal/DoD identity、原位修订记录、pre-execution assessment 要求与中立 strategy requirements；该快照直接进入 LLM C1。
- 服务启动只启动 workspace projection 与 HTTP 生命周期；不再运行 V1 inactive-running workspace recovery，也不注册 stale-Step watchdog timer。
- Decision/Approval HTTP resolve 只提交对应 owner command 并刷新投影，不发布 Run Resume；Runtime V2 Decision resolution 由 Decision owner 通过 typed EventHub command 直接提交 Orchestrator。
- run-scoped Start/Resume scheduler command 直接发送 Orchestrator 定义的 typed owner command；Resume 只携带 workspace/run scope，不再通过 CLI、session 或 Run port 发送 execution context patch。application 不维护 run pending/started queue，也不执行 Run 状态机或 runtime drive。
- workspace activation/restart command 同步执行 activation 用例；用例通过 Workspace、Run 和 Orchestrator 的 typed EventHub command 创建带 Runtime V2 bootstrap 的 Run 并启动 drive，不维护 application background queue。
- WorkspaceTemplate 中的 capability / version-control 声明只作为定义与 activation 输入；application 不直接执行 Git、Shell、File 或其他 capability 副作用。
- Runtime V1 retirement planner 依据 Run owner 的 typed bootstrap/state 识别 V1 对象，确定性生成 Run、Workspace、Decision 和 Approval 的退役动作及 dry-run 报告；planner 本身不直接访问 owner repository。
- retirement inventory 只通过 Run、Workspace、Decision 和 Approval owner 的 typed EventHub list command 采集，不读取 application bootstrap export 或 owner repository。
- `RuntimeV1RetirementCommand/Result` 是 migration 的 typed application command；dry-run 只返回动作与资源统计，正式 apply 必须在 owner 写入完成且重新盘点无遗留后执行 projection full rebuild。
- 正式 apply 在 owner 状态写入前逐 Run 调用 Orchestrator 的 typed resource release command，并以具体 result 累计 managed validation、Step、ToolCall 和 ArtifactContinuation 的真实释放数量。
- 本地 embedded CLI 通过窄 `RuntimeV1RetirementBackend` 调用 migration command；默认及 `--dry-run` 均只预览，只有显式 `--apply` 才执行，remote backend 不暴露维护能力。
- `--apply` 只允许 file repository，并由进程 service 在 framework、EventHub 和 owner repository 启动前备份 repository data 与配置目录；备份成功后才允许进入 typed migration command。
- `--restore-from` 由进程 service 在 framework 启动前执行，恢复完成后直接退出；migration 或后续启动失败时返回对应备份目录和恢复命令。

## 相关约束

- application 只负责入口、用例编排、facade 和投影，不得成为正式状态 owner。
- HTTP、CLI、TUI、scheduler 和 projection 不得直接访问 owner repository 或 service。
- projection 只能读取和投影事件，不得修复 owner 状态或重建业务状态机。
- 跨组件 command/data/result 必须使用具体类型，不得引入 Envelope、通用 map payload、JSON 类型桥、reflect 或双 Hub 兼容层。
- repository builder、owner module fixture 和 repository 直写只能存在于 `biz/testkit` 或测试文件。
- application 可以定义 profile 到 focus/capability/prerequisite 的映射，但不得把该领域映射下沉到 kernel reducer。
- 没有显式 executable evidence requirement 的 supplemental deliverable 只能生成 assessment-only focus；不得为了关闭 coverage 制造报告文件或泛化 validation。声明 accepted step kind/operation 的 profile item 必须同步生成对应 capability prerequisite。
- 历史 V1 active/blocked Run 在切换迁移前只允许保留审计，不得由 application 启动恢复逻辑取消、重派发或重建 continuation。
- application 不订阅 Decision/Approval resolved notification 来恢复 Run，不维护 governance pending、attempt、recovery scan 或 replan scheduler 分支。
- application event、HTTP/client、backend port、CLI/TUI 和 Web surface 已全部删除 `executionContextPatch`；历史上下文 patch 不得作为 activation、恢复 authority、兼容 DTO 或跨组件 payload。
- application bootstrap/session 构造器不接收或缓存 Runtime V1 repair/output policy；Runtime V2 按需读取 canonical configuration，配置投影只用于管理接口展示。
- `Run.ExecutionContext`、activation execution-context population、resume patch 和对应 API/read-model fallback 已物理删除；activation 只允许构造 typed `RuntimeBootstrap`，治理、rejected-result 与 continuation 只允许进入具体 Runtime V2 合同。
- application 不保留 `scheduleWatchdog`、stale dispatched recovery、startup inactive Run recovery 或 stuck Run recovery helper；watchdog interval audit 为 `0`。
- application 不提供 orphan Step、stale dispatched Run 或 runtime-gap 的恢复 API；Runtime V2 重启推进只能重新发送 Orchestrator drive command，并从 Run owner 持久化 aggregate 幂等恢复。
- retirement planner 必须排除任何带 typed Runtime V2 bootstrap/state 的 Run；正式 apply 只能通过各 owner 的 EventHub command 执行动作，重复规划不得再次生成已完成的 supersede/clear 动作。
- 任一 Runtime V1 resource release command 失败时，正式 migration 必须停止 owner apply 和 projection rebuild；不得把计划数量当作真实释放数量。
- dry-run 不创建备份、不写 owner 状态或 projection；restore 不得在 framework 或 owner repository 已打开后执行，data/config 恢复必须在替换目标前完成全部快照校验。
- application 不保留 execution pending/started、owner readback retry 或 scheduler recovery map；请求只有在 typed owner command 成功接收后才返回 accepted。
