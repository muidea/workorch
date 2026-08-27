# Workspace Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-27

## 功能范围与边界

`blocks/workspace` 管理 Workspace、Goal、DoD 和 WorkspaceTemplate 组成的 workspace-definition 聚合，包括定义版本、激活绑定和模板查询。它只管理该聚合的正式状态，不承担 Run 调度或跨组件治理。

Workspace 还独占 immutable pre-seal `DefinitionAdmissionCandidate`、promoted `WorkspaceStartupInputContract` 与
`WorkspaceActivationAdmission`。candidate 可以保存尚未获准的 relation input；正式 startup input 只允许 pure compiler
确认的完整 `all_of`，并以 `StartupInputHash` 封存 Definition、Template/Profile contract hash、目录和 candidate-item mapping。
candidate 还封存 required owner-fact binding 与唯一 acquisition/reobserve closure；Workspace 只持久化/校验该事实，
不读取 capability catalog 或选择运行期动作。`CandidateHash` 覆盖 acquisition operation variant、operation target policy、
operation selector 与 Definition declaration selector/candidate refs；owner ingress 不替 caller 重新计算 hash。pre-seal
candidate/startup input 不携带 exact prospective target set；运行期只有一份 SM-3 semantic proposal 可以在这份
declaration 与 current owner constraints 的交集中提交 1～8 个唯一 canonical `ProspectiveTargetRefs`，Workspace 不补默认值、
不接受 singular 兼容字段，也不回写 proposal/owner fact/binding。
candidate 的 `AdmissionPolicyBindings` 分别保存 Template 与 Workspace 两侧每项 canonical evidence-policy
ref/revision/snapshot hash，`CandidateHash` 覆盖这些 immutable bindings。任一 raw policy snapshot 含退役
`prospectiveTargetRef`、空/非字符串 ID、同 owner 重复 ID、invalid typed policy 或 normalize cardinality drift 时必须在
submit/promotion 前拒绝；Workspace 不把 policy binding 解释为 target authority，也不让 live policy 漂移修改既有
candidate/admission lineage。Application final bootstrap gate 另行 exact 比较 admitted Workspace policy 的 typed projection。

candidate 同时封存完整 normalized typed `RuntimeProfileContracts` snapshot，而不只保存 hash。pure compiler 从该 snapshot
独立派生 owner-fact/convergence manifest，并要求与 candidate bindings 逐项相等；每个 Profile-required acceptance/deliverable
CWI 还必须有 exact `scope + id` evidence requirement、非空受支持 evidence kinds，以及至少一个能返回对应 typed owner receipt
proof 的 accepted operation。wildcard policy 不能替代 exact Profile-owned CWI proof，也不能把无证明交付项降级为
assessment-only Runtime focus。可选 validation-failure repair declaration 只能是一份 validation-triggered、`state_change`
repair、fresh-validation、bounded-attempt、declaration/operation selector 分离的 all-of convergence；current input 中该字段的
非 object/空 object、unknown/alternative/alias shape，以及同一 Profile 内重复 normalized `scope + id`，都必须在 promotion
前返回 `definition_profile_contract_invalid`。pure compiler 不读取 live owner fact，不选择 repair target，也不产生 Run、provider
或 capability 调用。

pure compiler 位于 Workspace owner package，只做 immutable candidate 的静态计算。唯一
`DefinitionAdmissionIntentCommand` 被映射为 Workspace-private event；该 reducer 在同一 CAS 路径生成 exact
`DefinitionAdmissionResult`，并独自选择 candidate persistence、all-of promotion、pending ticket binding 与 eligibility。
Application 只能附带从 Decision concrete result 映射出的 exact immutable Identity/Binding values；Workspace 会从自身
candidate/admission 事实重新派生 request identity，并重算 ticket/resolution identity 与 binding hash。Workspace Biz 还必须在
进入 private reducer/CAS 前，通过 Decision-owned typed persisted-fact query 逐字段确认 ticket/resolution 已由 Decision durable
保存；caller 自洽 pair、非空 ID/hash 或 `SourceOwner` 字符串都不能提供 provenance。Application 不能提交 promotion/ack
target command 或 caller 计算的 admission outcome。
Application 对 pure helper 的 read-side 调用只是防御校验，不产生或迁移 admission state。

Workspace 也独占 Workspace retirement/deletion lifecycle。`WorkspaceDeletionRecord` 是嵌在 Workspace 聚合中的
owner-private durable journal；外部 DTO 不暴露 phase、owner acknowledgement 或 cleanup authority。唯一 public ingress
是来自 `application/workorchd` 的 concrete `DeleteWorkspaceCommand{WorkspaceID, RequestID, Purge,
ExpectedRevision}`，它只携带 immutable intent。`workspace.deletion` private event/reducer 是 requested、waiting-for-idle、
Run draining、Runtime quiescence、Run retired、owner seal/purge、directory cleanup、Eventstream finalization 与 completed 的唯一转换入口；
Workspace 自有 Goal/DoD/目录与 ordinary configuration sidecar/index 的物理清理也只能由 `directory_cleanup_requested`
private dispatcher 调用。dispatcher 先用 current Workspace/deletion revision、deletion identity/intent 与 stable cleanup ID 调用
独立注入的 owner-family `WorkspaceDeletionMaterialCleaner`；file backend 幂等清空 SoulBindings sidecar、fresh-read 后只清目标
Workspace index entry 的目录/config material，并复读空集证明，随后才提交 `directory_cleanup_observed` CAS。该 cleaner 不属于
普通 `WorkspaceRepository` CRUD interface，Application、Initiator adapter 与其他 owner 不获得调用 authority。same request/exact
payload 是可重放 noop；同一 request/deletion identity 的 mode、
expected revision 或 sealed directory 漂移直接 conflict。

本地 `ActiveRunID` 非空时初始删除零 intent；本地 slot 为空但 Run owner authoritative prepare 返回 busy 时，聚合进入稳定
`waiting_for_idle`。一旦 journal 落盘，它仍永久封闭新的 setup、Goal/DoD、DefinitionAdmission、LLM/config 与 execution
intent ingress；重复 DELETE 与周期 recovery 可继续重试 Run prepare，busy 不烧 revision，owner-private Run
projection/terminal fact 仍可收敛。固定交接顺序是 Run Prepare 落 `draining` → Orchestrator Begin 落 durable
`quiescing` gate → Confirm 返回 exact Runtime-quiescence token → Run Finalize exact compare 并落 `retired` → owner sealing。
Workspace 只把每一步 validated typed result 映射为 private fact，在自身 journal 依次持久化
`run_draining -> runtime_quiescent -> run_retired`；它不选择或改写 Run/Orchestrator phase，也不在 Finalize conflict 时
刷新 authority token。后续 seal Result/Memory/Decision/Approval；retain 再以 Eventstream seal 完成，purge 则先按
Orchestrator/Result/Memory/Decision/Approval/Run 顺序取得 exact purge proof，清理 Workspace-owned Goal/DoD 与
deliverable directory，再最后 seal/purge Eventstream。每个 typed result 必须先 `ValidateFor(exact command)`，再映射为
private fact；任一失败保留当前 phase，由相同 DELETE 或启动/heartbeat inventory 继续。owner-private transition time 对
持久 `UpdatedAt` 做单调 clamp，系统时钟回拨不会破坏 journal。

retain 与 purge 都保留 Workspace tombstone及 deletion journal；普通 Workspace/Goal/DoD Get/List 对 completed
tombstone 隐藏。retain 不物理删除 Goal/DoD；purge 清除 Goal/DoD、ordinary Workspace configuration 与 deliverable
directory，但保留只能由各自 private reducer 改写的 definition-admission/lifecycle records，且不能删除封存 journal 的
internal directory。Eventstream 是最后 owner，使前序 owner purge evidence 在封存前仍可审计。

## 组件交互及合同

- Workspace、Goal、DoD 和 Template 的 CRUD input，以及 Template defaults ensure input，均由
  `blocks/workspace/pkg/events` 定义具体 command/result 类型。Workspace/Goal/DoD/Template 的 item、list、delete receipt
  与 LLM-selection CAS 分别使用窄结果；EventHub 上不存在依赖“哪个字段非空”判别语义的共享 result union。四个 command
  family 只在 Workspace Biz 内通过包私有响应结构复用 reducer plumbing，不能成为跨组件合同。
- Workspace creation command 只携带 identity、描述和已解析目录，不携带 `State` 或 active slot。
  `workspace.creation` owner-private event/dispatcher 是唯一 repository Create 入口，由 Workspace owner
  选择 initial `active` 并绑定 matching workspace context；protocol/service 不得构造 caller-selected initial state。
- Workspace 创建、状态变化、删除以及 Goal/DoD 定义变化由本 block 发布具体通知。`TopicWorkspaceDeleted`
  使用独立 concrete `WorkspaceDeletedData`，只携带 stable deletion/request identity、mode、completed disposition、
  owner/deletion revision 与完成时间，不携带 private phase/ack journal；它在 Eventstream 最终 fence 后作为
  transport-only receipt 直接供 SSE/remote consumer 使用，Eventstream 不 reopen 也不接受 late ordinary append。
- `application/workorchd` 与需要 definition fact 的 owner 通过 EventHub 读取 Workspace、Goal、DoD、Template 和
  definition revision；其他 owner 不直接访问 Workspace repository。
- `application/workorchd` 通过 owner command/read port 管理和展示 workspace definition。
- Definition admission 只有一个 concrete `DefinitionAdmissionIntentCommand/Result`。command 携带 immutable candidate intent，
  且至多附带一个 `WorkspaceDefinitionClarificationTicketFact` 或
  `WorkspaceDefinitionClarificationResolutionFact`；两者封装 exact immutable Identity/Binding values，字段类型来自 owner-neutral
  `internal/pkg/workorchdefinitioncontract`，Workspace `pkg/events` 不依赖 Decision owner package。Workspace Biz 查询 Decision
  concrete persisted-fact read contract；只有 open ticket 的 ticket fact，或 resolved admission 的 exact ticket+resolution record
  可以进入 private reducer，Found=false、retired ticket、partial/mismatched record 均零 CAS。Workspace 仍独立重算 deterministic
  identity、binding hash 与 disposition，shared values 不携带 target state 或任何 owner transition。`CandidateID` 在一个 Workspace 的连续 startup-input authoring lineage 内固定；materially changed input
  只能用同一 ID 的全局 next revision 与 exact expected-revision CAS 提交，新的 `CandidateID@revision=1` 不能绕过 outstanding ticket。
  owner result 返回 exact admission、candidate create/duplicate disposition，以及
  `clarification_required | clarification_ticket_required | clarification_resolution_required | eligible` factual outcome。
  Workspace-private reducer自行选择 all-of promotion；绑定 blocking ticket 时先进入 `pending_clarification_resolution`，
  重算并逐字段匹配 resolution fact 后才 CAS 为 `eligible`。eligible revision 只关闭不晚于自身的旧 clarification lineage；其后的
  invalid candidate 仍须由后续 corrected candidate 精确 reconcile。任一步失败都不能把 admission 解释成可启动。
- `kernel/system` 发送 `EnsureWorkspaceTemplateDefaultsCommand`；本 block 只为缺失的内置 ID 创建默认模板，已存在对象保持不变，并返回具体 result。
- `WorkspaceTemplate` 是 workspace owner 的 DTO；EventHub result 只返回该 owner DTO，并在 module 内部完成与 repository entity 的双向 mapping、slice/map/payload clone。repository 类型不得泄漏给 application 或其他 owner。
- 外部定义命令保持唯一入口 `handleCommand -> executeCommand`；Owner 内部按 Workspace、Goal、DoD、Template
  四个命令族做穷尽且 fail-closed 的静态分派。各 handler 只是同一 Biz 内的职责切片，不是新的 Owner、状态机、
  repository/CAS 边界或 EventHub facade。
- Goal/DoD/Workspace repository 协调器是 Biz 包内私有实现；本 block 不导出可绕过 EventHub ingress 的 manager 或
  CRUD service facade。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Workspace | Workspace/Goal/DoD/Template CRUD、single Definition admission intent/fact、definition read、execution intent 与唯一 deletion intent ingress | 完整；Application 不持有 deletion saga |
| `kernel/system` | System → Workspace | `EnsureWorkspaceTemplateDefaultsCommand` | 完整 |
| `blocks/run` | 双向 | Workspace → Run 的 scoped execution intent、retirement prepare/finalize/purge；Run → Workspace 的 lifecycle fact、draining/retired fence | 完整；状态转换分别留在各自 owner |
| `kernel/orchestrator` | 双向 | typed definition/activation fact 读取、sealed terminal delivery；Workspace → Orchestrator Begin/Confirm quiescence 与 purge | 完整；不访问 repository，不直接写 Workspace model |
| Result / Memory / Decision / Approval | Workspace → owner | deletion scope seal；purge mode 下 exact purge proof | 完整；只消费 owner typed fact |
| `blocks/eventstream` | 双向 | create/state/delete、Goal/DoD 通知；deletion 最终 seal/purge proof | 完整；最后封存前序 audit |

## 相关约束

- 只访问 Workspace、Goal、DoD、WorkspaceTemplate repository 与独立的 Workspace-owned deletion material cleaner，不得持有
  Run、Result 或其他 owner repository/service；普通 Workspace CRUD repository 不暴露该 cleaner。
- Workspace deletion coordinator 只能通过 EventHub concrete command/result 调用其他 owner；required owner 顺序是
  Workspace-private invariant，Application、repository adapter 与其他 owner 不得选择 phase 或 acknowledgement 集合。
- 所有 Goal/DoD command 必须显式携带 WorkspaceID；owner command boundary 绑定同一 execution scope，memory/file
  repository 对 scoped Get/Update/Delete 使用一致语义。同一 GoalID/DoDID 可在不同 Workspace 中重复；无 scope 且
  identity 不唯一时必须 fail-closed。
- Goal/DoD definition revision 使用 owner-local `<kind>:<20 位递增代次>:<content-hash>`；同一 Workspace、同一
  definition kind 内可按代次/字符串单调比较，`A → B → A` 仍产生新 revision。相同规范化内容的重复 update 是零
  revision。新 identity 只能连接当前唯一 canonical leaf，不得按 ID、列表顺序或 repository 顺序推断前驱；历史
  definition 不可直接更新，当前 canonical leaf 不可物理删除。激活绑定变化必须使旧定义选择可检测为 stale。
- Goal/DoD 当前 definition 必须显式携带 `LineageState=active`。空 lineage 或其他不完整 current 字段直接冲突且零写入；
  Workspace owner 不补默认值、不重写对象，也不产生任何 schema upgrade 结果。
- Workspace block 不驱动 Runtime、不判定 Result、不执行治理恢复；execution-intent ingress 只校验当前
  Workspace startup input/admission、选择 active Run，并向 Run owner 发送 concrete typed intent。Restart 必须逐字段匹配
  current eligible `DefinitionRevision + StartupInputHash`；通过 owner admission 后、向 Run owner 发送 intent 前，Workspace
  以唯一 execution-private event 将 sealed bootstrap 的 target-only provider selection 幂等物化为缺失的 next-run 默认值。
  该 reducer 在每次 CAS 前重验 current startup/admission，stale hash 零写入，已有用户选择保持不变；Run-scoped reasoning
  effort 不回写为 Workspace 默认值。Resume 只选择现有 Run，不重新读取或附带最新 Goal/DoD。它消费 scoped sealed terminal outcome，按
  outcome identity 在自身 reducer 中释放 `ActiveRunID` 并维护 Workspace state projection；首次确认必须满足
  `ActiveRunID == outcome.RunID`，空 slot 不得补写新的 acknowledgement fence，已持久化的同 identity 重投递才是
  idempotent noop。它不读取
  `ConvergenceState`、RuntimeAction、focus、repair 或 error message 重新裁决；delivery feedback 保留 stable
  category 与完整安全 original diagnostic。
- 正式 DoD definition resource 只保存 `ready`。pre-seal draft 属于 `DefinitionAdmissionCandidate`；每个 Run 的
  satisfied/unsatisfied/indeterminate assessment 与 terminal closeout 属于 Orchestrator，不能回写 DoD state。
- `fresh_activation_required` 只表示可读 current Run 缺少 sealed bootstrap；普通 Run notification 不释放 Workspace slot。
  无法按当前结构读取的持久化数据在 execution admission 前失败，Workspace 不创建 successor、不读取或修补该数据，
  也不通过旁路 fact 清除 active slot。
- Run notification 只是投影交付提示。Workspace 在自己的聚合中持久化
  `WorkspaceRunGeneration + RunID + RunRevision + RuntimeStateRevision/RuntimeStateRef + CausationID` observation cursor，
  并只通过 `workspace.lifecycle` private event/reducer 更新 `State`/`ActiveRunID`。该 cursor 是唯一 durable
  Workspace Run-lineage high-water；terminal release 后保留而不复制 generation 到第二个 terminal 字段。低于 cursor
  generation 的任意延迟事实均为 noop；同 generation 必须是同一 Run，否则 conflict；更高 generation 只有在 active slot
  为空且 retained cursor 对应的 prior sealed terminal fence 已匹配时才能接管，generation gap 合法但不得绕过 terminal
  handoff。同 RunID 的更高 generation 一律 conflict。exact duplicate 与同代严格旧事实为 noop；两个
  revision 维度不可比较、同 revision 身份漂移或当前投影不一致时 fail-closed。handler 的 CAS exhaustion/存储失败不会
  被当作成功交付：owner 保留 process-local dirty hint，分页重读 Run owner 的 current typed lifecycle fact inventory 并
  重放同一 private reducer；启动时无条件重读，且每个 periodic heartbeat 都从空 cursor 执行一次 bounded full scan，
  因而非 snapshot 分页期间落在既有 cursor 后方的新事实会在下一 heartbeat 收敛，通知完全丢失或 process-local dirty
  已清也不会永久漏投影。进程崩溃不依赖历史 notification 或持久化 retry timer。未注入
  BackgroundRoutine 的最小装配在 Start 与后续 hint 上执行同一 bounded synchronous fallback；Timer/initial Async 安装失败则
  Start 完整取消 worker、退订 topics 并保持可重试，不留下 `started=true` 的半启动实例。terminal Run notification 只触发
  current-fact retry 清理，不选择 Workspace target state；sealed terminal delivery 才会在 Workspace owner 内封存 release fence。
  因此 run1 terminal → run2（含 watermark gap）active/terminal 后，任意更晚到达的 run1 non-terminal fact 都不能重新占用 slot。
- Start/Restart 在任何 Run-owner command 前必须从 `WorkspaceActivationAdmission` 精确定位唯一 immutable candidate，并验证
  `candidate -> startup input -> admission` 的完整 current-shape 绑定。缺 candidate、重复 identity、hash 漂移、旧 schema/compiler
  或不再满足 `admitted_all_of` 的 closure 都直接 conflict；不得重封存、补字段、创建替代 candidate 或生成兼容 admission outcome。
- Start/Restart 必须逐字段匹配 promoted `RuntimeProfileContractsHash` 与 candidate 中的 exact typed Profile snapshot。
  Profile-required CWI proof 不闭包时，pure admission 在 promotion 前返回 `definition_profile_contract_invalid`；Profile-derived
  owner fact 缺少 preexisting 声明或 exact one-shot acquisition/reobserve closure，或 candidate manifest 与 snapshot 派生值不同时，
  返回 `definition_profile_prerequisite_unresolved`。两者都要求 `select_or_fix_template_profile` blocking ticket；Workspace 不得用
  live owner 事实、LLM 文本、wildcard evidence 或默认动作修补 candidate。candidate-owned relation/ref failure 仍使用
  `declared_prerequisite_unresolved + rewrite_definition_all_of`。
- 默认 WorkspaceTemplate 是可由用户编辑的首次启动种子；普通 ensure 不得覆盖内置 ID 上的当前对象。当前阶段不提供
  默认资源升级或数据迁移；需要新默认值时清理数据后重新初始化。
- current candidate schema/compiler 严格要求完整 Profile snapshot（schema v4/compiler v4）；旧 shape 不迁移、不补写，必须
  清理并按 current Template 重新创建 Workspace。
- file repository 只接受 current object-shaped `ContextHints`；`null`、数组、标量、未知/退役字段与空持久化文件均 fail-closed。

## 已覆盖测试点

- [x] memory/file backend 上 Template defaults 的部分已存在、缺失补建、重复 ensure 与内置 ID 用户修改非覆盖语义。
- [x] Workspace creation 经唯一 private dispatcher 选择 initial state，错误 workspace scope/非 creation event 零写入，静态门禁拒绝其他 Biz `State` literal 或 repository Create 旁路。
- [x] Workspace CAS revision，以及 Goal/DoD create/update、`A → B → A` 的单调 revision、相同内容 noop 和 stale restart definition 拒绝。
- [x] Goal/DoD 非递增 ID 的 canonical leaf 谱系、历史 definition 更新拒绝和 current leaf 删除拒绝。
- [x] 缺失 current lineage 的 Goal/DoD 更新直接拒绝且不会补写 `active`。
- [x] memory/file backend 上 Goal/DoD 局部 identity、错误 Workspace scope 的 Get/Update/Delete 与无 scope 歧义拒绝。
- [x] Workspace deletion 的 stable identity/payload conflict、Application-only source、initial active Run 零 intent、waiting
  ingress fence与零 revision busy retry、forged/stale ack、retain/purge exact owner顺序、Eventstream-last、response-loss
  duplicate、fresh file owner启动恢复、时钟回拨 clamp、safe path guard、directory-cleanup direct-observed拒绝，以及 exact
  sidecar/index pre-clean 的 stale/forged拒绝、重复稳定、其他 Workspace 隔离和 main-metadata-only crash cut reopen proof；同时覆盖
  retained tombstone 的 Workspace/Goal/DoD read hiding。
- [x] sealed terminal outcome 的匹配释放、空 slot/其他 active Run/冲突 identity 拒绝和同 identity 重投递幂等。
- [x] immutable candidate create/duplicate/stale revision、owner-generated admission result、single-intent promotion 重编译、
  unsupported alternative 零 promotion、all-of auto-promotion、ticket-required/pending-resolution factual outcome、
  owner-neutral Identity/Binding/Record values 上的 exact deterministic binding、Workspace events 零 Decision owner import、
  self-consistent 但未持久化的 ticket/resolution、Found=false、resolved-ticket ticket-only、retired/partial/mismatched record 拒绝、
  same-lineage next revision、new-CandidateID bypass、duplicate against corrupted later lineage、stale CAS、response-loss duplicate
  经 Decision persisted query 恢复，以及 startup-input hash clone/persistence。
- [x] Template/Workspace evidence-policy snapshot bindings 完整进入 candidate hash；任一侧 legacy target 字段或 snapshot
  漂移在 promotion 前 fail-closed，且零 Run/provider/capability work。
- [x] application-owned directory resolver 的空目录默认值、相对/绝对路径规范化、workspace ID/相对路径逃逸拒绝和
  多 workspace 默认目录隔离。
- [x] execution intent 的缺失 Goal/DoD、context scope mismatch、duplicate replay 与 active Run result fence。
- [x] Restart provider selection 在 Run intent 前只经 Workspace-private event/reducer 物化；覆盖 stale startup 零写入、
  duplicate noop、existing preference 不覆盖与 Run-scoped reasoning effort 隔离。
- [x] Run owner 的 `RunStateChangedData` 在 Workspace observation handler 中携带 exact Run revision 并翻译为 private
  projection fact（含 exact `WorkspaceRunGeneration`），且 `pkg/events` 不暴露可由其他 owner 提交的 Workspace projection
  reducer input。覆盖 CAS exhaustion、
  非 CAS 存储失败、stale/duplicate/不可比较 revision、terminal 后延迟 fact，以及 fresh owner 从 current Run fact inventory
  恢复并由 file repository 重开验证持久化 cursor；另覆盖 nil BackgroundRoutine 同步恢复、terminal hint 清理 retry 不释放
  active slot、pending 已清且 notification 完全丢失后的 heartbeat 恢复、分页扫描中 behind-cursor 插入后的下一轮 full-scan
  恢复、Timer/Async worker 安装失败的完整 Start rollback 与 clean retry，以及 generation 1 terminal → skipped generation 3
  active/terminal → 极迟 generation 1 non-terminal noop、同 generation 异 Run、无 terminal fence 的跨代接管和同 RunID 更高代拒绝。
- [x] 无效持久化数据不会生成 execution admission outcome，也不会修改 Workspace 元数据。
- [x] Restart 对 admission-bound candidate 的缺失/重复/旧 schema/hash 与 acquisition selector 漂移严格拒绝，零 Run command；
  current exact candidate 可正常进入 Run owner。
- [x] Profile snapshot/hash 漂移、required CWI 缺 exact proof/evidence kind/owner receipt operation、重复 Profile source，以及
  snapshot-derived owner-fact manifest 缺失/附加/漂移均在 promotion 前 fail-closed，并映射到可执行的 Template/Profile 修订动作。
- [x] strict-current validation-failure repair shape 与重复 normalized requirement ref 都映射为
  `definition_profile_contract_invalid`；owner result 无 startup input/activation admission，Workspace 聚合零 Runtime promotion。
- [x] 普通/治理/冲突 Run notification 与 `fresh_activation_required` 均不能释放 active slot；只有 matching terminal delivery 可释放。
- [x] Template command、repository DTO、EventHub DTO 与 list mapping 对 slice/map/nested payload 的完整深拷贝。
- [x] `ContextHints:null`、非对象 payload、未知/退役字段与空文件严格拒绝。

当前没有与本合同对应的已知可执行测试缺口。
