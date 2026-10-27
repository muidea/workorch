# workorchd Application Module

Status: active

Type: application-module-contract

Last Verified: 2026-09-10

## 功能范围与边界

生产 HTTP/Web 入口按 [`local-operator-access.md`](../../../../docs/50-product-interface/local-operator-access.md) 的
`workorch-local-operator/v1` 限定为本机操作者。listener/peer 必须回环；Host、Origin 和 fetch-site 门禁先于业务
路由，代理来源头不授信。私有 transport identity 不从 caller actor/header 创建，也不代替 Workspace 的精确确认、
授权 grant 或 Decision 批准。它不隔离同机进程，不支持远端代理曝光；Runtime SM-1～SM-4 与 owner 权限不变。
`service/server/local_operator_boundary_test.go` 验证生产 wrapper、零业务副作用拒绝、原输入保全与身份来源。

`application/workorchd` 组合 HTTP/Web、CLI/TUI facade、typed bootstrap、application use case、read model、projection、
SSE 和 runtime session 生命周期。它负责把用户入口转换成接收 owner 定义的 concrete command，并把 owner fact 转换成
detached public DTO；不拥有 Workspace、Run、Result、Decision、Approval、Memory、Skill、Soul、MCP Registry、
Eventstream 或 Runtime SM-1～SM-4 的正式状态。

本模块拥有三类进程内状态：统一 ingress barrier、`runtimeComponent` 持有的长期 session，以及
`projection.Component` 持有的派生 Store/后台任务。三者只服务应用生命周期，不能成为业务 authority、恢复队列或跨 owner
状态机。

Workspace 的启动输入是 Goal、DoD、Template/Profile snapshot 与 resolved project directory。Application 只做 typed
解析、规范化、snapshot assembly 和提交；Workspace owner 独占 `DefinitionAdmissionCandidate`、promotion、
`StartupInputHash` 与 activation eligibility。Definition 只接受完整 `all_of`；unsupported relation 必须形成 blocking
clarification，不能创建 Run。启动输入封存后，执行期 LLM 处理不得修改它。

Goal-only 创作用例先保存 Workspace owner 的版本化草稿，以持久 reserve permit 调用派生和六类独立审查，确认 exact proposal/specification hash 后才物化正式定义并准入。阻塞后继沿同一候选 lineage 重新审查；Application 只协调 Decision ticket 的替换或 resolution，Workspace 验证其持久来源并独占 pending → eligible 转换。刷新投影以 owner 查询为准；pending startup 显示 `admitting`，不能显示已准入或启动 Run。当前正式创作已接稳定 management ports 和 HTTP 创建／查询／修订／编译／确认 route，创建 RequestID 由 Workspace owner 持久验证；Web 创建向导已改为单一创作／审查／确认入口，发送前保存创建身份，重新打开按 owner 查询恢复。生产 HTTP 编译由 Application 的 BackgroundRoutine 托管，接受回执返回 202，任务使用独立于 HTTP 请求的生命周期 context，Stop 取消并排空已接受任务；任务数有界，结果与预算仍只在 Workspace ledger。已 reserve 但结果未决不重发 provider。旧 public Workspace／Goal／DoD 写 REST 和 Goal-first CLI 不参与创作；正式定义只由确认事务物化。保存请求使用原 RequestID、expected revision/hash 与目标输入，owner 在同一 Workspace CAS 中持久回执；精确重放不重复写入，历史回执不能将后续替代的输入冒充当前成功。验收证据按 HTTP／owner、独立进程、Web 组件与 deployed provider 分层，局部 helper 测试或构建不能替代端到端恢复与显式启动合同。

### 草稿保存与找回

草稿创建提供独立“保存草稿”动作，只调用创建／原请求重放，不调用 PUT、compile、confirm 或 activation。
Application 在 Workspace owner 持久化后准备允许的目录，再发送 `RecordAuthoringDirectoryPreparedCommand`，
核对 owner 回执并独立读回后才返回 201。`creation.directoryPrepared` 只证明原创建阶段完成，不证明当前资源就绪。
目录失败保留草稿与原身份；GET/List 不补目录。列表可打开只读草稿详情；只有 owner 能精确重建初始创建 hash
时才公开原 payload 供显式重放，无法重建保持待核对，禁止新身份替代。页面仅使用内存，迟到回执受会话 fence 保护。

### 合同变更材料登记


Application `RegisterContractChangeProposal` 以 Workspace owner 的原 Input/Definition 查询和 Run owner inventory 独立核对 baseline，当前仅接受零 Run Workspace；typed Decision command 保存原文和版本，不修改草稿、封存输入或启动 Run。该用例尚未暴露 HTTP/Web/CLI route，不把 caller `ProposedBy` 当授权证明，也不提供 approve、matching acceptance、定义应用或后继启动。Decision exact 回执必须校验；新能力完整就绪前普通治理入口仍拒绝该 kind。跨 owner 读取只是登记前的观察，不构成跨 owner 原子准入，应用时必须重新核对当前权威。

Application 的 `PrepareDefinitionChange`／`QueryDefinitionChange` 只发送 Workspace-owned typed 合同并校验 detached 回执，不提交 candidate after-image、批准或 target state。接收 Workspace 从实际 Decision/Run 读取反证来源，独立准备待审输入；原 authoring/current Definition 不变。当前仅支持原 Goal 和用户验收项的 exact 映射，其他差异拒绝。`DeriveDefinitionChangeAcceptance`／`CompileDefinitionChangeSpecification` 显式接收候选 input hash，复用原有界 task、File 资源事实、provider binding、strict mapping parser、独立六审及 permit settlement；未决 reserve 不重发，completion 响应丢失只查询原 owner 候选 ledger。普通查询仍返回原定义，候选输入查询不借用原 confirmation/admission；封存定义的材料登记通过原 owner 查询验证确认／准入，而不在裁剪的普通 Workspace DTO 上重建权威。`RegisterReviewedDefinitionChange` 从 Workspace 查询重建的完整已审身份，并只向原 Decision 提案追加 `FinalCandidate`；不接受 caller hash、不修改其他原因／差异／影响。Decision receiver 再向实际 Workspace 反证，版本漂移或身份不符拒绝；原 RequestID 响应丢失沿持久版 exact 重放，不重新调用 provider。此登记不生成 Resolution，不改变原 Workspace／confirmation journal，不启动 Run。尚无候选确认／approve／应用或公开 route，不能把准备、六审成功或最终登记显示为 approved、M3 完成或 Run 已执行。

### 创建恢复响应

公开创建在有效请求发送至 Workspace owner 后失败，返回 `workspace_creation_incomplete` 和 typed `creationRecovery`：原 `requestId`、确定性 Workspace ID、`unknown`／`persisted`、失败阶段及原请求重放要求。owner 回执缺失或 transport／持久化失败只记 `unknown`，不能推断尚未创建；收到有效创建回执后目录准备／读取失败可记 `persisted`，仍不代表 M3 完成。输入／身份明确拒绝不附带既有对象恢复描述。内部 cause 原样保留在错误链，不混入通用 HTTP 错误；HTTP 提示不作为准入或执行权威。

Web 只在页面内存保留原 payload 和恢复提示；未收到完整创建成功响应时继续重放原请求，不因失败响应中的 ID 改成仅查询而跳过目录准备。查询／列表以服务端 owner 为准；关闭／刷新不保存浏览器恢复身份，也不取消服务端事务。目录失败不得覆盖或清理用户既有文件，恢复不启动 Run。

### 创建期调用与结果结算

派生、proposal 与六类 review 使用 `CompleteProviderDocumentCommand/Result`，绑定已 reserve 的 opaque permit
及完整 command hash；完整输出才进入 strict parser。provider 确定失败／不完整与 parser 失败分别保存，
文档、wire 和 transport leaf 由 Workspace 受限诊断查询保留原 bytes／SHA256／观察完整性；原文不进入通用日志或 Runtime。
三类 closed schema 全部计入请求预算，稳定合同放 C0，动态输入放 C3；native／prompt 共用严格本地校验且零 fallback。

reserve 只表示调用权已消费，LLM typed fact 可能仍在内存，诊断可读也不替代 completion；只有 Workspace completion CAS
成功才是阶段持久结果。可信结果返回后，parser／completion／exact owner readback 共用一个最多 5 秒的 settlement scope，
不因原 caller 取消丢弃已知结果，不重调 provider、不建立 Application 恢复队列。completion reply 丢失只在同一期限内
对账同一 immutable permit；CAS 前崩溃且没有持久结果仍 unresolved。file reopen 不等于断电级 fsync 保证。

direct ports 与后台任务共用 `runtimeComponent` 的 authoring lifecycle：Stop 先拒绝新调用并取消工作，首次 Stop 封存唯一有限的 settlement 窗口，重试不刷新该期限。等待真实退出使用每次停机调用的独立 context 预算；settlement root 取消不等于实际调用结束。等待超时返回失败并保留依赖，实际任务释放后允许新预算重试清理。execution admission 与 CLI dispatcher 同样追踪真实 handler 返回，超时不清除在途记账；CLI 部分订阅逐成功登记，取消失败保留清理项。

Module 在 Setup 前保存 Biz，部分 Setup/Run 失败不丢 owner 或提前释放依赖。Biz Stop 错误通过 plugin guard 传播，只有检查式清理完整成功才清空 Biz 引用。projection 的后台 callback 未退出前不能关闭 manager。上述 owner 局部合同不单独证明全应用共享任务／Hub／Initiator 屏障已完成，也不赋予强杀不配合取消任务的权限。

Workspace list/detail 的 `authoringSummary` 沿 owner Get/List → typed mapping → public projection 传递，不逐卡查询完整创作 ledger。创作 CAS 后既有 Workspace 通知经 Eventstream record 触发 projection 失效及 owner 重读；Store 同步更新列表条目和详情。摘要仅显示持久阶段与计数，`reviewed` 不等于当前可确认；原始诊断、恢复参数和权限仍由完整 authoring view 及 owner guard 管理。

`resume` 是既有 Run 的 continuation，不携带 fresh bootstrap；`restart` 先发送 Workspace-owned
`/workspace/command/recover-execution-intent`，在当前编译前查询原操作。该 POST-only 命令可精确结算原回执，但不登记/转发；
已完成或 pending 直接保留 owner 原结果，没有原登记和阻塞项才从当前 eligible definition 编译 fresh bootstrap。
absence 不是转发许可，完整 intent 仍须由 Workspace reserve-before-forward。公开执行语义输入为 Workspace/request/intent，
reason/source 是传输审计元数据，不修改已封存 payload；Application 不让调用者提交 bootstrap。
两者均进入 Workspace owner 的 workspace-scoped execution admission，Application 不直达 Run mutation，也不创建 replacement successor。
未知原结果按 `409/pending/accepted=false` 传递原 pending 三字段；不误报 owner 拒绝、受理或“没有 Run”。

## 组件交互及合同

| 交互方 | 方向 | concrete contract | Application 责任 |
| --- | --- | --- | --- |
| `blocks/workspace` | 双向 | Workspace/Goal/DoD/Template CRUD、definition admission/promotion、execution intent、single deletion intent、typed read | 规范化输入、校验 exact result scope、形成 DTO；不选择 owner outcome |
| `blocks/run` | 双向只读 | Application → Run 的 Run/Step/Checkpoint/status read；Run → Application 的 immutable `RunStateChangedData` | `RunPorts` 不含 lifecycle、Runtime CAS 或 deletion mutation；通知只更新派生 read model |
| `kernel/orchestrator` | 双向只读 | immutable drive observation、ToolCall/runtime audit query | 不调用 reducer、不持有 aggregate、不生成 dispatch |
| `blocks/llm` | Application → LLM | active configuration、catalog/refresh、selection validation、generic completion/embedding | 只调用 LLM owner；generic result 必须与 exact command/target 绑定并 detached，不直连 provider |
| Decision / Approval / Governance | 双向应用用例 | scoped query、generic resolution、trace query | generic resolve 不发布 Resume；Definition clarification 不进入 generic resolve |
| Result / Memory / Skill / Soul / MCP Registry | Application → owner | concrete CRUD/query、bounded content read、consistent selection snapshot | 校验 identity/revision、deep clone；不读取 owner repository 或私有路径 |
| Eventstream | 双向只读 | bounded event/high-watermark query、`EventRecordAppendedData` | high-watermark result 必须精确匹配 normalized workspace scope 并 detached；projection/SSE 不从审计重建 authority |
| File / Git / Shell / SSH / Web / MCP executor | 无直接执行 I/O | 无 | operation 只由 Orchestrator sealed dispatch 调用 |

所有跨 owner I/O 都从接收 owner 的 `pkg/events` 导入具体 Command/Data/Result。HTTP DTO、CLI DTO、projection DTO 与
EventHub DTO 互不复用；response 在跨层前校验 workspace/run/item identity 并 deep clone。

### Application lifecycle

- `biz.Biz` 是 EventHub 订阅生命周期根；transport 不保存 Hub 或 Runtime Session。
- `runtimeComponent` 是唯一长期 runtime session owner；借用端口不复制 Session 指针。
- `projection.Component` 是唯一 projection Manager/Store lifecycle owner；server 只持有无生命周期 reader/write port。
- Stop 顺序固定为关闭 admission、drain CLI/HTTP、等待 in-flight、解除应用订阅、关闭 projection/background tasks，最后
  关闭 runtime session。重复 Stop 幂等。

### Admission and deletion ingress

- HTTP、CLI 与 TUI execution 都进入同一个 workspace-scoped owner lane；重复 RequestID 每次交给 Workspace/Run owner 的
  durable idempotency contract，Application 不保存 queue、pending map 或 retry successor。
- owner 返回具体 execution admission 后，Application 才可写 immutable accepted/rejected projection；transport/bootstrap
  error 不能伪装成 owner rejection。
- DELETE 只发送一个 `DeleteWorkspaceCommand`。Application 不预读 Run、不枚举 owner、不清理目录、不组合 purge saga；
  Workspace owner 的 durable deletion journal 负责 fence、quiescence、purge 与恢复。
- `WorkspaceDeletedData` 只进入 process-local SSE replay/fan-out，不追加回 Eventstream，也不触发任何 owner transition。

### Bootstrap and projection

- 可选 `ImplementationWorkPolicy` 只从匹配 Profile 声明实例化到 exact CWI 的不可变 Definition，wildcard/exact
  必须一致；不修改原 CWI/check/最终 validation proof，不把工作 operation/selector 写入 mutable focus。
  合并后的非 validation-only CWI 拒绝，Runtime 独立反证 Profile source。当前协议仍被执行支持门禁拒绝；
  编译成功不是可执行工作授权。`runtime_implementation_work_test.go` 覆盖精确匹配、冲突、隔离、身份与门禁。
  已匹配 CWI 的工作 operation kinds 同时编入 Definition capability contract、对应 owner 的 state-change
  inventory prerequisite 和 operation-admissibility prerequisite；不从未匹配策略或最终验证权限推导。
  Runtime 独立校验该投影的完整性；能力清单不是 CWI 工作 binding，也不扩大最终 proof。

- builtin-defaults query 只汇总 Soul/Workspace 的 detached concrete result；单资源 upgrade 只向对应 owner 发出带
  expected/target content hash 的 typed command。Application 不维护升级状态、不得用 query 结果代替 owner 写入时校验，
  也不隐式刷新 Workspace binding/admission 或重启 Run。
- bootstrap compiler 无损封存 definition/profile、provider、Soul、Skill、MCP、Memory、evidence policy、budget 与 directory
  snapshot；Goal-only authoring 在 owner 创建／修订事务中物化完整 Template binding。其显式 Restart 通过 typed
  authoring 查询核对原确认、当前 Workspace revision、正式 Goal／DoD 和模板／Definition policy，复用 exact eligible candidate，
  不进入退休的通用定义准入入口、不重新调用 provider。任何输入漂移均阻塞，不能自动替换用户已确认规格。
  其他受支持的 owner 定义路径仍由 Workspace owner-private admission 管理。Resume/recovery
  不重新读取当前资源。capability requirement 必须是终态证明
  kinds 与 Profile validation-repair route kinds 的并集，因此 repair-only owner operation 可被启动时采集，但不得被加入终态
  `AcceptedStepKinds`。authority-bearing route 数组按原 cardinality 严格解析；`null`/scalar/empty/unknown、重复 identity 或
  duplicate exact trigger 不能被过滤、折叠或默认补全，且 repair kinds 未共同声明 concrete owner confirmation protocol 时
  必须阻塞 pre-seal。
- Application 可以定义 profile 到 typed focus/proof declaration 的映射，但不得把 profile-specific action 逻辑下沉到
  kernel reducer。required source 缺 exact executable proof 时必须在 pre-seal admission 阻塞。
- Profile 集合在 Template/Workspace merge 前检查可无损转换的对象/数组形态；同一输入层的空/重复成员必须保留给 typed
  owner admission，不能用后写覆盖隐藏 required 义务。只有两层各自唯一的同 identity 项可按显式配置覆盖规则合并；
  该配置规则不是语义一致性证明。final bootstrap 独立验证同一完整 Profile，不能依赖前一 adapter 已检查。
- Profile 的原始配置还必须符合当前对象层级的封闭字段与精确值类型；未知/错层字段、布尔字符串、混合/空白成员列表、
  非整数或越界数值均在两侧 merge 前返回输入错误。final activation 独立检查；不把非法配置转为默认值，也不伪造
  Workspace admission 或 Runtime 状态。既有 repair lossless decoder 保留其 owner 裁决路径。六个内置模板、两侧
  覆盖、直接 activation 和正常 payload 重载由 `activationports/runtime_profile_input_test.go` 验证；SM-1～SM-4 不变。
  `activationports/runtime_profile_decoder_contract_test.go` 在测试中逐对象核对实际 decoder 的字段与转换类型，防止
  校验器和 decoder 单边漂移；该 AST 索引不是生产 registry 或运行期权威重建。
- projection 只保存 bounded immutable public DTO。canonical Workspace/Run state、active Run 与 exact blocker ref 原样投影；
  display summary、consistency diagnostic、recovery hint 与 metrics 都不能生成 mutation intent。
- `contextHints.executionLimits` 的 `runtime-execution-limits/v1` 收紧 Run 级 provider call 与 capability operation 上限，
  并可选 `maxProofBatchesPerCWI`（1～17）；该字段只封存进 typed activation，由 Orchestrator 原 semantic ledger 消费，
  不编译为物理调用 dimension，也不是一般工作迭代授权。省略不附加逻辑上限，显式零/错类型/超界拒绝。
  两侧原始配置先独立检查，再按 Template 优先合并；effective typed identity 进入 Workspace admission policy binding，
  final bootstrap 反查该封存。`activationcore` 将声明编译到既有预算 policy，不增加计数 owner、调用入口或执行权限。
  `activationports/runtime_execution_limits_test.go` 与 `activationcore/runtime_execution_limits_test.go` 覆盖无损输入、
  合并、完整准入身份、当前协议重载与 policy 重封存漂移；未知协议、缺字段和扩大默认硬上限拒绝。
- blocking input-review 详情从 Orchestrator 的 sealed original diagnostic 深拷贝到 audit/outcome DTO；Web 的只读展开项
  保留全部 check/obligation、输入身份与原因，不受摘要标签的条数截断。脱敏文本及原报告 hash 原样显示，不能据此
  生成 approval/retry/验收权限，也不能把 review 当作实际 validation 或有效进展。
- Run heartbeat 区分 `latestActivityAt` 与 `latestEffectiveProgressAt`。事件、LLM 调用及普通 artifact/patch/result 时间只形成
  activity；effective progress 仅使用当前 Run 已被 Orchestrator 接受的 exact committed receipt（CWI proof 或 durable changed
  repair）及当前 Run accepted Result。activity age 用于心跳指标；局部有效进展不能覆盖 failed terminal。重复失败只来自
  当前 failure signature 的 owner count > 1 或正式 exhausted fact，不继承旧 projection 的 retry 标记。
- 版本展示分为 builtin-defaults catalog current/target、Workspace admission 的 template revision/snapshot/input hash，以及
  Run audit 的 sealed startup/profile hash 与 Soul stage/id/revision/content hash/provenance。展示不实时读取 catalog 替换
  Run snapshot，不泄漏 sealed guidance；这些 detached metadata 不能反向构造 Workspace admission 或 Runtime authority。
- Artifact、Step、ToolCall 和 RuntimeCheckpoint 内容只经其 owner 的 bounded typed read；禁止以 `dataRoot/backend`、
  `os.ReadFile` 或 raw-file fallback 绕过 owner。

### 前置准备诊断与正式进度

创建期 proposal／review 分别使用完整、版本化的 closed JSON Schema，稳定合同与准确字段说明位于 C0，当前身份、资源及候选项位于动态输入。Application 显式标记 schema 正文，通过既有 typed provider command 传递；schema ID 不能冒充可执行输出合同。结构约束不能替代 strict decoder、资源／来源／Profile 反证、六审查或用户确认，非法 provider 字段不做容错映射。

已持久失败的同输入 compile 只返回原失败；部署新 renderer 不授予新 permit，也不迁移旧 ledger。真实修订仍受 owner CAS、累计预算和原诊断保留约束；只修代码后的新验证须由用户显式创建新事务。

创作目标的资源、配置及请求构造失败在尚未 reserve 时，由 Application 保全原 error bytes，通过 Workspace concrete record command 持久诊断；它不生成 permit 或失败 completion。已 reserve 请求优先重放 ledger，完成阶段不重新构造请求，未决请求不自动调用 provider。每个新审查阶段使用最新 owner revision 保存失败，迟到结果不能重绑新输入。保存失败只记录安全定位元信息和未保存事实，不复制原文到通用日志，也不声称诊断可查。

常规创作 DTO 只展示 bounded preparation metadata；原始 cause 按 Workspace／DiagnosticID 独立只读查询、可逆编码。`preparation_failed` 只代表当前尚未 reserve 的目标已知失败，用户修复后可显式 compile；已 reserve 或确认后旧失败为历史。诊断不是恢复队列、预算或调用 authority，不改变 SM-1～SM-4。

正式进度只使用 owner 已封存事实：`workspace_defined` 来自 admitted startup hash；`formal_result` 必须关联当前适用 completed Run 的 exact terminal Result，且 Result accepted、scope 和 startup seal 精确一致。Result／Artifact 存在、历史成功或 Runtime terminal 均不能补造完成；Runtime overlay 后仍经同一只读最终投影，TaskProgress 与 compressed context 一致。内部 terminal ID／startup hash／Run 代次仅用于关联，不新增 public authority。

无适用 Run 时，进度摘要独立显示持久 `authoringSummary`、执行未启动与 Result 未验收，不把 Workspace `active` 或零 blocker 当作执行正常；RunState 只来自同 Workspace 的实际 Run owner 对象。用量摘要的 `availability` 区分 `not_loaded`、`partial` 与 `complete`，并携带查询 `scope`、单次 `recordLimit` 和 `observedRecords`。`complete` 仅表示当前有界查询没有触及上限且返回记录全部可解析，不代表完整历史；上限命中、缺失／无效 payload 或仅有 Run 补充数据时不得冒充完整 Workspace 用量。detail／summary 保持不增加 Workspace usage payload 查询，未加载不显示零调用或零 Token。显示、clone 与 projection diff 保留这些范围信息，不从普通 event topic 推算调用或 Token。

### 激活前准备的只读入口

`GET /workspaces/:workspaceId/activation-preparation` 经 Application 的窄只读端口发送
Workspace-owned `QueryWorkspacePreparationStateCommand`；HTTP 不接 EventHub 或 owner repository。
仅投影持久操作、当前已知有效性和独立 ActiveRunID，缺记录为待检查，原未知操作不会因 GET 解锁。
响应 `no-store`，错误按安全类别返回，不输出 owner 原始诊断；本入口不接收 caller PASS 或恢复指令。

`activationcore.CompilePreparedBootstrap` 接受 detached exact snapshot、稳定 manifest 和 typed activation input，
校验精确 Profile 并从原 candidate 编译 source obligations；无 Hub/client/repository/callback，
不调用 InitialFacts、Runtime initialize/drive 或既有含资源写入的 activation port。
其结果只证明结构编译，不替代资源/配置来源的 owner 观察、required 检查或 no-Run 排序证明。
`activationports.CompileOriginalPreparedExecutionPolicy` 只消费完整原封存与原 `ExecutionInputs`
私有材料，反查真实持久 attempt、完整来源、模板和配置前像；复用既有纯策略 decoder，
编译 execution contract、Workspace evidence policies 及按原配置/locale 优先级确定的对话语言。
该 helper 无 context、Hub、loader 或环境读取；缺原材料、来源漂移或无效策略均拒绝，
不借新配置、默认模型或占位资源构造 activation。其输出只是策略部分。
`activationports.CompileOriginalPreparedBootstrap` 在显式 check/recover 回复中消费同一原操作的
完整私有归档，组合原 Soul、Skill context、Memory 原模式、MCP 选择、Provider 声明和执行策略，
再经 `CompilePreparedBootstrap` 完整结构编译。Workspace concrete result 的 `CompilationSource`
只传无 map 的具体 detached DTO 原材料，显式转换后独立完整重验，禁止 JSON 输出；
不额外查询当前 Workspace，不从普通读模型恢复私有材料或以 metadata 代替原始诊断 bytes。
缺来源、非空 semantic Memory 缺证明或原模板策略无效均明确拒绝；不把缺失转为空选择。
HTTP 只返回有限失败码或绑定原 snapshot/manifest 的 bootstrap hash，不输出原材料/诊断/完整 bootstrap。
GET 不执行该编译；恢复和重启后的显式 recover 从完整原材料重编，不以历史 hash 授信。
这不是准备检查的正向结算、currentness、M3 或 Start/Restart 的回执消费入口。
当前开放准备 check/recover 的阻塞主体流程，不开放 M3 签发或恢复执行；既有 Start/Restart 不被宣称已消费准备回执。

`CheckStartupPreparation` 将原 request/revision 映射为 Workspace concrete check/recover 命令。
仅初始接纳 CAS 明确成功的响应触发一次 cache-only/完整 Run 历史观察，再向 Workspace
`settle-blocked` 发 concrete negative-only command；不绕过 owner 写库，也不把延续提示当成凭据。
settlement 内 Workspace 独立向 File 验证原绑定目录，原始材料由 Workspace 受限保存；Application 不提交路径、
File PASS 或 caller-authored 证据。管理 DTO 仅投影 `directoryEvidenceHash` 历史引用和具体负向分类；
Web 展示引用不授予当前资源成功，GET/重开/恢复不触发目录观察。
最终 File 重观察和 epoch/年龄判定由 Workspace/File 原 owner 完成，Application 不接收 caller token；
GET 与原 check/recover 在 File 重启或年龄到期时一致展示 stale，原结论和身份不变。
验证 scope、原 payload 和完整结果后转为公开读模型；普通重放、恢复与 GET 不执行这些观察。
HTTP 的显式 POST 拒绝未知字段、尾随文档和超过 4KiB 的请求；200 blocked 不是 ready，
202 accepted/unknown 不是检查完成，错误响应不证明无写入。GET 始终只读。
详情页独立准备面仅在用户打开时 GET，显式按钮发 check/recover，单飞并隔离 Workspace 与迟到响应；
POST 后 GET 对账，保留原意图及原 revision，未见原请求不自动解锁新键，不使用浏览器持久存储。
资源事实尚未完整组合，v2 总体仍阻塞；已接通消费者可保存原时点逐项结论，未接通项保持
明确阻塞，不借框架闭环授予 M3 或执行权。

Application 另有 `ObservePreparationProviderCatalog` 和 `ObservePreparationRunHistory` 两个
准备子用例，分别只向 LLM cache-only 和 Run 完整历史 topic 发 concrete typed query，
保留父 scope/context，每次 2 秒预算，无 retry/fallback；发布前后核对完整 DTO、scope 和观察身份，
错误只向上返回安全分类，不透传 owner 配置诊断。它们接入首次显式检查（整体观察预算 5 秒），
未接入持久状态 GET、恢复或 M3。原请求取消时不借后台 context 偷交结果，accepted 由显式恢复阻塞结算。
负向分类不是 owner 原始结果的替代归档。Provider 的同一次 cache-only 完整结果经
`PreparationProviderCacheFacts` detached 传入原 settlement，由 Workspace 独立验证并与原 binding
在同一 blocked CAS 保存私有历史，公开仅有随机 `providerCacheEvidenceRef`。
不增加查询、refresh 或 provider 请求；单点原值不是持续 freshness、连通性、授权或精确 selection。
Run 历史当前仍仅保存负向分类，不以原因码反建完整观察或 readiness。
`ObservePreparationMemoryInventory` 只向 Memory 专用库存 query 发一次 scoped typed 请求，
2 秒预算、完整 DTO/scope/观察区间校验，不进入 activation 的 semantic recall 或 list fallback。
仅首次明确接纳的检查调用，沿原整体观察预算与 settlement 保存完整私有历史；
公开随机 `memoryInventoryEvidenceRef` 不含内容或向量。完整/空库存仍为选择待核验阻塞；
不能据此证明原模式已验证、semantic 可用、持续 freshness 或 M3，GET/恢复不重查。

`ObservePreparationSkillHub` 是显式外部检查子用例，不是严格只读 query：观察前后经 Workspace typed query
读取同 revision 的精确封存快照，在封存目录执行 SkillHub `status --json`，全局执行
`status --json --global`（当前 CLI 无 `--agent` 参数）。沿用 `skillhub-status-effects/v1`：
SkillHub 可优先联系其配置服务端、保存项目检查状态；不修改 SkillHub 项目，不宣称零联网/零写盘。
整体 5 秒、每次 status 2 秒，不 retry、sync、use、apply、install 或 repair。
普通准备 GET/刷新/导航/重启读取不调用该用例；`CommandStarted` 保留真实启动事实，超时不代表外部
未写入/未执行，不自动重发，不将本次检查当成跨 owner 原子或幂等外部事务。
不复用允许部分列表的展示/执行选择路径；缺命令、缺目录、失败、歧义、超限不得伪装空库。
项目原 scope `project` 保留；global 按 skill＋agent 判唯一，skill_count 按不同技能数校验；
全部 agent/orphaned/原状态、reason、时间和 hash 保留。Codex 指令仅取独立解析安装根中的正式文件，
不读其他 agent 或 orphan 的指令，不回退 SourcePath/RepoPath；目录逃逸、链接、非普通文件、
超限/非法内容/读取前后漂移均拒绝。每个文件最多 64 KiB、首轮最多 8 MiB 原 bytes，逐项真实重读；
首份事实及至多一份 64 KiB 的失败重读原值保留。这里只是诊断，不放宽 activation 的 8 KiB/24 KiB 指令预算。
原 stdout/stderr 分流各保留至 1 MiB，超限显式标记不完整并保留已读字节计数与所存前缀 SHA256；
原诊断仅限受限内部返回，不进入 public GET、SSE 或通用日志，尚未接入持久准备检查账本。
`InventoryComplete` 仅指观察到一份完整无歧义的 status inventory，不将其 status/hash/version
提升为精确选择、freshness 或可用性证明；`InstructionsReobserved` 只证明本机安装文件两次观察完整一致，
不证明 status 服务端观察与本机目录一致、目录附属文件或 metadata 语义已验证。`BindingCurrent` 只确认该次
观察结束时封存快照未漂移，不是资源当前性、final CAS、ready 或 M3。

`ObservePreparationSouls` 经现有 Workspace 与 Soul typed query 只读观察实际选中记录：
先核对精确 seal，再读取同 revision 的 Workspace 配置并反查配置 hash。服务默认 Soul
必须来自 Application 已注入的配置 loader，缺 loader/读取失败不伪装默认为空，不回退全局配置。
阶段绑定、Workspace 默认、服务默认的优先级与现有激活路由一致，无 Run override；
全部阶段及 unbound 显式返回，未使用的默认不会仅因其存在被强制读取。整体 context 预算 3 秒，
不安装/更新默认资源、不编译完整 activation、不调用 Memory/provider。完整保留所选 Soul 原内容、
revision、content hash 和 provenance，独立 clone 后逐项真实重读全记录，最后再次核对服务默认与 seal。
缺失/非法/漂移均阻塞并保留已观察原值；该结果是有界只读事实，不是跨 owner 原子快照、
长期 freshness、所列 Skill/MCP 可用性、准备 PASS 或 M3。
首次明确受理的生产准备调用经真实 process config accessor 运行上述观察，将完整前后 records、
前后 system default 和 seal hash 作为 concrete `PreparationSoulFacts` 交 Workspace 独立核验原请求绑定。
完整事实只随原 blocked 结算 CAS 保存到 0600 私有历史；公开 GET/Web 仅显示随机 `soulEvidenceRef`。
不把 helper 的 `Reobserved`/`SealBindingCurrent` bool 带回 owner 作为成功凭据；失败/不完整明确为
`soul_observation_unavailable_not_saved`，不将部分事实标为完整 archive。
完整相等也仅为 `souls_observed_remaining_checks`；GET、完成重放、恢复与 owner 重开不重新观察。

`ObservePreparationSkillRegistry` 只通过 Skill owner 新的完整有界 typed query 观察正式库，
整体预算 3 秒。先读取精确 seal，再真实查询两次完整库，校验完整 DTO、scope、观察时间、
1024 项/8 MiB 上限、canonical 原内容与 provenance、排序/唯一 ID、内容 hash 和 owner epoch，
逐项比较全部独立记录（不凭 hash 命中授信），最后再次核对精确 seal。未知/损坏/超限/漂移均拒绝，
已成功取得的事实保留用于受限诊断，不丢弃为无操作；该独立 helper 不重试、不写账本，
不调用 SkillHub 或 provider。该结果只证明两次正式库观察及封存绑定，不是完整 Registry+SkillHub
目录、精确选择、原子跨 owner 快照、长期 freshness、准备 PASS 或 M3；这些仍由后续显式检查裁决。
首次明确受理的原准备检查将两轮完整 registry 值和前后 seal 交 Workspace 独立核验与结算，
不发送 helper 成功 bool。完整材料随原 blocked 结算同 CAS 存入受限私有历史，
GET/Web 仅显示随机 `skillRegistryEvidenceRef`；完整空库也不授精确选择 credit。
失败或不完整明确为 `skill_registry_observation_unavailable_not_saved`；GET、完成重放、
恢复和 owner 重开不再查询，不将未保存的材料称完整。Registry 持久观察本身不调用 SkillHub status；
显式检查现已把 SkillHub status 的两个目标放入 Workspace 首次 accepted CAS；只有无歧义首次延续
执行 global 后 project，各最多两秒且共享原五秒观察预算。执行前重新读取同 accepted revision 的
exact seal，核对原确认、candidate、配置、startup 与目录；不满足时封存真实 not_started。
默认生产 adapter 沿用当前 SkillHub 的服务访问和状态保存行为；测试仅注入 Application 私有外部替身。
原 bytes/计数/完整性/退出/诊断随原 blocked CAS 私有保存，不消费 inventory/PASS bool。
GET、普通重放、恢复、重开均不调用 status；丢失回执按 unresolved 封存，不接受迟到补写，后端禁止
换 request 绕过未决效果。公开仅随机 ref 和 reserved/recorded/unresolved，不授 ready/M3。
成功 status 后接入无 runner 的安装观察入口：独立重解析该次 raw 回执（清空调用方 parsed 字段），
只在独立确定的根下双读对应 SKILL.md，不再次执行 status。两轮实际读值和严格 metadata 解析失败
随原 effects archive 同 CAS 私有封存，Workspace 独立反查原 inventory/path、双读字节、metadata 与
时间/界限；保留失败和部分材料，不把调用方 reobserved bool 当证明。公开只给
`skillHubInstalledState=incomplete|observed_selection_pending`，不泄露原文、不表示选择成功。
安装文件后续变化不导致 GET/replay/recovery/reopen 补读或覆盖历史。原 Workspace 私有结算将
本次 Registry、Soul、SkillHub 原材料纯编译为组合选择，随同一 Registry archive/blocked CAS 保存，
不再次调用独立选择 helper。公开 `skillSelectionState=sources_incomplete|conflict|compiled_checks_pending`
只呈现历史编译状态，缺来源不冒充空目录；Workspace 独立重算完整依赖、allowlist、分数、理由和
选中原内容，不接受调用者成功凭据。历史原 status 保留，不授当前 freshness、资源 PASS 或 M3。
Workspace 在该次结算内复用既有纯 context 预算合同生成独立预览，保存投影、预算计数、裁剪标记
或原拒绝诊断；完整源材料仍原样保留。公开 `skillContextState` 为
`compiled_checks_pending|bounded_checks_pending|rejected`，必须绑定已编译的原选择及来源引用。
Application 只映射 owner 结果；不重新编译、裁剪、恢复缺失预算或向执行入口提供预览作为凭据。

`ObservePreparationSkillSelection` 在单次五秒预算内组合精确 seal、Soul 和正式 Skill owner 的 typed
观察，并显式调用每个 SkillHub scope 的一次 status 和已安装文件双读。使用封存 Goal/DoD 原文及
execution Soul 的 allowlist，不接受 caller 提供的“已验证目录”、宽松管理列表或 source/repo 文件
fallback。准备专用 YAML AST 解析保留原文，重复／别名冲突、错误需求类型、不完整或超界明确失败；
YAML 依赖固定为 vendored `gopkg.in/yaml.v3 v3.0.1`，不增加新运行单元或 EventHub topic。
选取沿用现有评分与排序策略，返回有界 typed matches 和独立克隆的完整 Skill；原 status、目录 hash、
其他 agent、orphan 和失败原文均不改写。初始 stale 不执行 status；后续失败保留已读来源，不隐式重试。
`SelectionCompiled` 只表示已生成选择材料，`SealBindingCurrent` 仅表示本次末次 Workspace seal 核对；
二者都不是 ready、PASS、M3、跨 owner 原子观察或长期 freshness。辅助文件、实际状态／freshness、
daemon/local 一致性、context 预算与资源 readiness 持久裁决仍须后续接线；该独立 helper 不进入 GET、SSE、Start/Restart 或
Runtime drive，不改变 SM-1～SM-4。

## 不变量与非职责

- Application 不写任何 sibling owner repository/service，不拥有 SM-1～SM-4 reducer、delivery phase 或 deletion phase。
- 生产代码不得引入 generic envelope、`map[string]any` authority、JSON/reflect bridge、callback dispatcher 或双 Hub 兼容层。
- 不存在 application Start/Resume queue、inactive-run recovery、stale-step watchdog、owner readback retry 或 caller-controlled
  Run priority；恢复只来自 owner aggregate 的幂等 re-drive。
- projection failure 只能改变派生 Store 健康度；不得回写或修复 owner state。opaque Runtime aggregate 不进入 Store/HTTP cache。
- Decision/Approval resolve、display blocker、SSE、metrics 和 context bundle 都不能选择 governance continuation、Restart 或
  terminal。
- Runtime raw provider body、reasoning 与 credential 不进入 public content API、projection、SSE 或 audit DTO。
- 当前配置严格拒绝 unknown/retired field；Application 不提供 data migration、legacy decoder 或 compatibility outcome。
- repository builder、owner fixture 与 direct repository write 只允许在 `biz/testkit` 或 `_test.go`。
- `WorkspaceProjectionChangedData` 是 Application self-notification；consumer 只接受 exact `application/workorchd` source 与
  concrete data，forged/noncanonical source 不得进入 SSE transport。

### 局部 Runtime 生命周期

`biz/bootstrap.Runtime` 只管理共享调度器与 Hub；测试装配由 `biz/testkit` 显式列出 20 个业务 owner，
不扫描全局 plugin 注册表，也不替代生产的 21 个 Hub owner 与 Repository Initiator 装配。
局部停止遵循全体 `BeginShutdown` → 全体 `Quiesce` → 共享任务 Shutdown → Hub Drain → 逆序检查式
owner Stop → Hub TerminateChecked。缺少 DrainingHub 的装配明确拒绝停止，不能以 void Terminate 冒充完成。

部分 Configure/Start 的错误或 panic 必须保留已配置 owner；Run.Configure 已登记订阅时，Run 也在调用前纳入账本。
`TryNewRuntime` 在 owner 装配失败时同时返回原 Runtime 与错误，调用方必须对该实例执行检查式 Shutdown。
共享任务或 Hub 排空失败不释放 owner；最终 Stop 逐成功移除账本，失败保留剩余 owner 与 repository 引用，
新预算重试不重复释放已完成项。只有完整停止成功才清除 Hub 导出；取消请求不代表真实 handler 已退出。
这些阶段不改变 SM-1～SM-4 的业务权威、转换或持久恢复语义。

## 能力验收

| 验收面 | 自动化证据 | 覆盖维度 |
| --- | --- | --- |
| 前置准备失败与原文读取 | `biz/definition_authoring_preparation_test.go`、`biz/definition_authoring_http_integration_test.go`、`service/server/definition_authoring_preparation_test.go`、`internal/pkg/workorchclient/definition_authoring_test.go` | 捕获原始 bytes、保存失败与 typed 无 permit 拒绝、next-unreserved target、独立 raw scope、file 重开、重复失败、同输入显式修复、零自动 Run |
| 正式进度与上下文一致性 | `biz/appservice/formal_progress_service_test.go`、`biz/appservice/readmodelprojection/progress_owner_facts_test.go`、`progress_context_test.go`、`runtimecore/run_helpers_test.go` | 空 active 草稿、startup seal、当前 Run owner 代次、exact terminal Result、历史隔离、overlay 与独立压缩上下文、私有字段不进入 public DTO |
| Module/port/session lifecycle | `module_test.go`、`biz/application_ports_test.go`、`biz/runtime_component_test.go`、`biz/owner_stop_contract_test.go` | setup failure、port completeness、single owner、ordered/idempotent shutdown |
| workspace-scoped execution admission | `biz/execution/*_test.go`、`biz/execution_admission_gateway_test.go`、`service/server/workspace_execution_admission_typed_test.go` | normal/rejected、scope mismatch、duplicate、concurrency、transport failure、zero queue side effect |
| Definition/bootstrap/restart | `biz/appservice/activationports/definition_admission_test.go`、`biz/appservice/activationports/sealed_restart_snapshot_test.go`、`biz/appservice/activationcore/*_test.go`、`biz/appservice/workspacecore/directory_layout_test.go`、`service/server/http_test.go` | all-of admission、profile proof closure、bounds、directory safety、sealed snapshot、fresh restart ingress |
| owner reads and content isolation | `biz/appservice/*_service_test.go`、`biz/appservice/contextregistryports/*_test.go`、`service/server/production_transport_boundary_test.go` | nil/wrong result、identity/revision drift、pagination cycle、deep clone、private-path rejection |
| Eventstream high-watermark result | `TestReadWorkspaceProjectionHighWatermarksRejectsResultContractDrift`、`TestReadWorkspaceProjectionHighWatermarksReturnsDetachedCanonicalResult` | 覆盖 exact requested scope/order/revision 校验与 detached clone |
| LLM embedding result | `TestGenerateEmbeddingRejectsCompletionResultOnEmbeddingTopic`、`TestGenerateEmbeddingRejectsInvalidConcreteResult`、`TestGenerateEmbeddingReturnsDetachedVectorAndUsage` | 覆盖 topic/result type、exact target/vector fact 与 vector/usage detachment |
| projection/read model/SSE | `biz/projection/*_test.go`、`biz/appservice/readmodelprojection/*_test.go`、`service/server/projection_sse_test.go` | legal/illegal lifecycle、duplicate/stale event、restart generation、failed/degraded recovery、transport detachment |
| activity/progress 与封存版本 | `biz/appservice/readmodelprojection/progress_owner_facts_test.go`、`runtime_audit_test.go`、`biz/appservice/workspacecore/workspace_test.go`、`biz/appservice/workspaceviewcore/store_metrics_activity_test.go` | exact current-run receipt、activity 不冒充 proof、当前 signature 重复计数、旧 projection 零继承、catalog/admission/Run 分离、metadata clone 与禁止反向重建 |
| Application self-source authorization | `TestWorkspaceProjectionChangedConsumerRequiresExactApplicationOwnerSource` | 覆盖 exact owner、forged/noncanonical source 与 wrong concrete payload |
| Workspace deletion ingress | `biz/appservice/appmodelcore/workspace_deletion_test.go`、`biz/appservice/workspaceports/workspace_command_payload_test.go`、`service/server/event_subscriptions_test.go` | stable RequestID、single owner command、duplicate disposition、SSE-only receipt；payload-conflict/response-loss 的 durable replay 属于 Workspace owner 验收 |
| persistence and owner boundaries | `biz/testkit/runtime_integration_test.go`、`biz/production_setup_test.go`、`service/server/production_transport_boundary_test.go`、`service/server/application_port_validation_test.go` | memory/file assembly、typed production wiring、repository/service/EventHub AST fences |

真实 HTTP/file-backed/managed-service 场景由
[`docs/50-product-interface/service-acceptance.md`](../../../../docs/50-product-interface/service-acceptance.md) 与
[`deployed-convergence-scenarios.md`](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) 验收；它们不替代
上表的组件级合同测试。
