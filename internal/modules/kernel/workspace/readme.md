# Workspace 协调 Module

Status: active

Type: coordination-module-contract

Last Verified: 2026-08-30

## 功能范围与边界

`kernel/workspace` 是 Workspace、Goal、DoD、WorkspaceTemplate、pre-seal `DefinitionAdmissionCandidate`、sealed startup input、activation admission、active-run 投影和 Workspace 删除 journal 的唯一 owner。它负责定义聚合的版本/CAS、完整 `all_of` admission、启动资格、执行意图准入和 Workspace 资源退役协调。

默认项目目录由 `application/workorchd` 在 setup 输入阶段解析；Workspace 只封存并校验解析后的目录，不实现 `.workorch-workspaces/<workspace-id>` 默认解析器。one-of/歧义定义只保留 pre-seal candidate，并通过 Decision 持久 clarification；不得创建 eligible admission 或 Run。

目录分类为 Module，因为定义准入、执行意图及删除闭环组合多个 owner 的 typed 合同。稳定 owner ID 仍为 `blocks/workspace`；目录不决定事件身份、repository consumer 或持久命名空间。全进程仅注册此唯一 Workspace owner。

## 组件交互及合同

| 交互方 | typed command/data/result | 边界 |
| --- | --- | --- |
| `application/workorchd` | Workspace/Goal/DoD/Template CRUD；`ConfirmDefinitionAuthoringCommand/Result`；`DefinitionAdmissionIntentCommand/Result`；`WorkspaceExecutionIntentCommand/Result`；`DeleteWorkspaceCommand/Result` | Application 提交输入和意图；Workspace 决定自身聚合转换。authoring confirmation、definition admission、execution intent 只接受 Application source。 |
| `kernel/system` | `EnsureWorkspaceTemplateDefaultsCommand/Result` | startup ensure 唯一 producer；owner 创建缺失资源并自动更新 exact reviewed 未定制 Template；只接受 `kernel/system`。 |
| `application/workorchd` | `QueryBuiltinWorkspaceTemplateDefaultsCommand/Result`、`UpgradeBuiltinWorkspaceTemplateDefaultCommand/Result` | exact `application/workorchd/appservice`；只操作 Template catalog，不改变已 admission Workspace 或 Run。 |
| `blocks/decision` | `QueryWorkspaceDefinitionClarificationFactCommand/Result` | Workspace 在 promotion 前读取 Decision 已持久化的 ticket/resolution 事实，不接受 caller 自证 binding。 |
| `blocks/run` | Workspace→Run execution/retirement/purge；Run→Workspace lifecycle fact/inventory | Workspace 管 active slot 投影；Run 管 SM-1 与 retirement 事实。 |
| `kernel/orchestrator` | `RuntimeTerminalDeliveryFact → WorkspaceLifecycleCommandResult`；Runtime quiescence command/result | terminal delivery 只接受 Orchestrator；Workspace 只映射已验证事实到自身 journal。 |
| Result/Memory/Decision/Approval/Eventstream owners | concrete seal/purge command/result | Workspace 排序协调删除；各资源 owner 独自执行自身 fence、purge 与证明。 |
| projection consumers | `WorkspaceChangedData`、`WorkspaceDeletedData`、`GoalDefinedData`、`DoDDefinedData` | 提交后通知，不是 authority。 |

公开 EventHub 合同均位于 `pkg/events`，每个 topic 使用具体 command/result；创作 record、派生 request 与历史回执使用 concrete DTO，嵌套 Template 的 Soul binding 为有序 typed slice；转换到同 owner model 前拒绝空／重复 binding，不允许 map 覆盖隐藏原 cardinality。显式转换不更改持久 schema 或 hash，也不授予已校验身份；查询、确认与准入仍完整反证 ledger。派生 reserve／complete、authoring 保存／查询／规格 reserve／complete／确认／准入均登记 delivery 与 typed message 合同。既有 pre-seal 规格 ledger 保留 begin／reserve／complete 三个受控写入口；无生产调用方的单请求 query 不注册 EventHub，owner-private 查询 helper 只作内部校验与测试。跨 owner 不暴露 repository/service。`biz.New` 只接收 backend factory 生成的 `WorkspaceOwnerRepositories` aggregate；Workspace、Goal、DoD、Template repository 与 deletion cleaner 必须携带同一个 opaque owner binding，独立构造、跨 backend 或跨 factory 混装在 owner 启动前拒绝。

### 合同变更待审输入

`PrepareDefinitionChangeCommand/Result` 与 `QueryDefinitionChangeCommand/Result` 只接受 Application source。准备命令仅传 Ticket/version/hash 与 Workspace CAS 身份；Workspace 在自身 mutation lock 外读取 Decision 原持久最新版本和 Run inventory，再在原规格 private dispatcher 内核对输入、模板和定义基线并 CAS。读取是原版本观察，不保证与并发 Decision 修订或启动构成跨 owner 原子事务，不能用于批准或激活。

`DefinitionChangeCandidate` 保存既有 `DefinitionAuthoringRecord` 的独立待审输入，不新增 confirmation journal、批准副本或正式定义权威。当前仅映射 exact `authoring-goal` 修改及 `derived-dod:user:NNNNNN` 用户验收的增删改；原文必须匹配当前输入，模板／生成判据和约束等不能映射的变更明确拒绝，不猜测语义。原当前 record、Goal／DoD、startup、admission 和 materialized journal 均保持不变；全部旧 permit、审查、诊断及预算历史继承，新 input revision/hash 不能借用旧六审确认。原始候选记录 detached，包含诊断时沿原 0600 原子发布与读取门禁保护。

当前只允许一个 exact preparation；同 RequestID/version/hash 重放恢复原回执，异请求覆盖或迟到旧提案拒绝。原 ledger 存在尚待结算的 permit 时拒绝准备，不冻结其 completion 或清除 unknown；结算后可重新提出同一准备请求，既有消费预算与结果原样继承。新候选派生／六审、修订／替换、可信主体 exact approve、原 journal 应用及 Run 交接尚未开放，普通保存不会消费该候选，普通确认／准入在候选存在时拒绝改变原定义基线。准备不调用 provider、不创建 Run，不改变 SM-1～SM-4。Application 提供实际 typed sender 和查询，但没有 HTTP/Web/CLI route。

### 阶段生命周期

`BeginShutdown` 关闭新创建、修改、确认、准入、执行、删除及后台调度，并取消 projection Timer 的后续触发；不等待、不退订、不清除 repository、cancel 或订阅账本。必要只读查询、既有 permit 的规格／派生完成、诊断交付、sealed terminal delivery 与 Run 事实接收保留原 source、scope、hash、预算及 CAS 校验，不授予新事务权限。Run 通知仍只是提示，关闭后不启动新的 inventory；普通通知不能释放 slot。

每项普通／matched 订阅仅在 checked 注册成功后加入账本。部分 Start 失败保留已获得的订阅与调度句柄，不在原实例重复注册。同步 handler 持生命周期票直到真实返回，包括锁等待与嵌套 Send；一次性 AsyncFunction 在提交前取票，accepted 排队任务到 callback 真实结束才释放，提交拒绝才由提交方释放。Timer callback 先检查 fresh admission，closing 后迟到的 callback 不访问已释放依赖。context 取消不是 handler 或 accepted task 完成事实。

`Quiesce` 按调用预算等待真实在途工作；超时保留全部未释放资源以便新预算重试。最终 `Teardown` 封闭所有入站，再次排空并逆序 checked 退订，逐成功移除账本，全部成功后才清除依赖。Module 的 Stop 错误保留原 cause 和 Biz；无返回值 plugin Teardown 通过 framework guard 暴露失败，不吞错误。Configure 不覆盖尚未释放的实例。

上述 owner-local 排空不替代全进程的共享 BackgroundRoutine Shutdown、Hub Drain 与最后的 Initiator 释放屏障；Timer 取消不证明共享队列排空。本合同不新增或改变 Definition、删除 journal、slot 及 SM-1～SM-4 的业务转换或权威。

## 不变量与非职责

- 正式创作创建入口接受稳定 RequestID，Workspace ID 由该身份确定。初始草稿与不可变创建身份在同一次 owner repository Create 中保存。file backend 在 metadata 内封存待提交绑定 after-image，核对／补齐 Soul 绑定和共享索引后，CAS 清除待完成标记；Get/List 在 owner 自有持久化层恢复缺件，未完成或冲突时不暴露完整对象。相同文件路径的创建、恢复、普通更新及删除串行；待完成窗口禁止普通写入和删除，版本／绑定／索引／删除冲突不覆盖，重开可继续。完成后的响应丢失／file reopen 重放不增写，后续修订也不会被初始创建重放覆盖；同身份不同内容拒绝。创建通知只是刷新提示，重放可重复补发当前事实，列表／详情恢复不依赖通知送达；Post 不是持久完成回执。目录初始化由 Application 在身份持久后执行，失败可用同一请求重试；创建不物化正式 Goal／DoD、不准入、不创建 Run。模板与目录修改通过创作保存事件随新 input revision 一次 CAS 更新，不以普通配置修改替代创作输入。
- confirmation、正式 candidate inputs 与 admission ref 从同一次完整 ledger 校验派生。内部上下文仅在本次调用中复用派生项、资源与 proposal hash，不持久缓存、不接受调用方校验标记。确认必须绑定 record 的 exact Workspace scope，当前 formal sequence 必须等于历史长度加一；不能用其他 Workspace ledger 或虚构前序身份建立准入。每个公开 model／query 入口均验证完整历史和当前来源；未选中的坏请求、重复 review、FAIL／UNKNOWN 与确认来源篡改仍 fail closed。
- startup input 一经 promotion 不可被 Runtime LLM 修改；candidate/admission lineage、definition revision 与 `StartupInputHash` 必须 exact 绑定。
- 新建期 `DefinitionAuthoringRecord` 仅保存 Goal-only 草稿、Template 快照、派生与审查请求；正式 Goal／DoD 与 startup/eligible admission 必须在用户精确确认后另行物化。资源引用最多 8 个；Application 向 File owner 取得有界文件事实，派生 reserve 若含资源，Workspace 在任何自身 mutation lock 外向 File owner 核对 exact 内容/hash 和当前草稿 revision，再由纯 reducer 对 Workspace revision 及模板做 CAS 栅栏。创作规格结构校验把持久派生快照映射为不可由 provider 改写的必需资源身份，六审查依然只是候选事实。持久资源快照和 permit 仅证明该时点的输入身份，不保证多文件原子读取或后续内容不变。确认 journal 和最终准入各自在 mutation/deletion 锁外重新向 File owner 验证原派生资源及当前 Workspace revision，随后在锁内核对 input、permit、snapshot hash、Template/configuration 与 revision 才可 CAS；Prepared journal 恢复及阻塞候选的重复准入也必须重新验证。File 与 Workspace 没有共同事务或跨文件原子快照，文件在最终验证后至 Workspace CAS 之间发生的外部变化不能被宣称已原子阻止。
- `DefinitionAuthoringConfirmation` 是同 owner 的持久 prepare／materialized journal：只从已持久派生提案和六份唯一 PASS 审查重组 exact hash，封存原派生资源快照 hash 与确定性 Goal／DoD after-image，再逐仓幂等物化；任一 file-backed cutpoint 可重开续写，重复请求不增写，冲突行不得覆盖。prepared 阶段的部分 Goal／DoD 写入对普通正式定义读取不可见，旧规格编译入口也不能消费创作期正式行。已物化确认的纯候选构建保留原始六审查的候选 item ref 和 `SpecificationHash`，并将正式 Goal／DoD 的精确 ID／revision 绑定到候选来源身份；后续 startup 映射负责生成正式 item ref，不能假称审查者评审过重写 hash 的规格，也不能把 `UNVERIFIED` 变成 Profile 证据声明。Application 的确认后继已接正式候选与准入，阻塞候选由 Decision owner 打开澄清 ticket，不创建 Run；新建期旧准入入口仍关闭。journal 在场时普通 Workspace 配置更新、正式 Goal／DoD CRUD 与删除入口均不允许改写该事务。未封存 startup 的阻塞候选允许提交新的创作 revision：归档原记录及 materialized journal，保留请求预算和正式定义 lineage，重新派生并完成六审查后沿同一 `CandidateID` 追加连续 candidate revision；prepared 后继仍不能暴露部分正式行。仍阻塞的后继由 Application 协调 Decision 替换旧票据；通过纯准入的后继须先验证持久旧票据，封存 `pending_clarification_resolution`，再验证 Decision 持久 resolution 后才能 eligible。确认重放和查询保留原 blocker 身份，不自动创建 Run。此 owner/Application 路径与正式 HTTP 创作 ports 已接通；Web 源码接线不替代 mounted／浏览器验收。
- Goal-only authoring 在创建／修订事务内物化完整 Template binding；精确确认后，显式 Restart 只复用原 eligible candidate，并核对正式 Goal／DoD、当前模板、目录与 Definition policy。漂移保持阻塞，不自动替换原确认或重新调用创作 provider；首次启动物化默认 provider selection 不改变原确认或 activation 重放身份。
- 其他受控 owner 定义路径的新 Start/Restart 自动加载当前 Template/Soul。Application 提交带 `CurrentTemplate`、expected Workspace revision/configuration
  hash 的新 candidate；Workspace-private admission event 持有 Template mutation lock，复核当前 Template hash，物化变化的
  template-bound 字段并 CAS 追加新 candidate/promotion。Goal/DoD、目录、provider selection、生命周期与旧 Run 不变。
  `TemplateBindingHash` 是 live authoring 版本标记，不是 Run seal；同 Template 保留 Workspace 配置，变化的 Template 整体
  替换旧绑定策略，包括移除字段。无需手动 refresh；详见 [automatic configuration loading](../../../../docs/40-profiles-capabilities/workspace/builtin-default-resources.md#automatic-configuration-loading)。
- 已准入历史只检查 scope/identity/lineage，不用当前 schema/compiler 重新准入；candidate clone 必须原样 detached，不做
  normalization、补字段或 rehash。新输入严格使用当前 compiler，历史身份损坏与未解决 clarification 不能被自动更新绕过。
  Resume/recovery 不进入此 authoring 事务，SM-1～SM-4 仍消费原 Run seal。
- Definition 仅支持完整 `all_of`；unsupported alternative 必须保留阻塞 clarification，不得降级或补默认语义。
- Profile 声明必须无损保留全部成员直至 owner admission；空/重复 identity、缺失 owner/state/variant 不能被 normalization
  清除。Workspace 只通过既有 private admission event 拒绝并保存 invalid candidate，不创建 startup/eligible admission。
  recipe/check/metric/artifact/owner-fact proof identity/capability-state 全部进入 Profile seal；同版本重复、重启和内容漂移遵守相同 hash/CAS fence。
- Workspace 的 active-run slot 只消费 Run owner fact 和 Orchestrator sealed terminal delivery；普通 Run 通知不能释放 slot。
- `RunProjectionCursor` 是 Workspace owner-private delivery fence：每个更高 Run/Runtime revision 都必须持久化，但只有
  `WorkspaceChangedData` 的公开字段实际变化时才发布 `WorkspaceStateChanged`。cursor-only CAS 不得伪装成公开 Workspace
  状态变化；workorchd 继续直接消费 typed Run notification 更新运行视图。
- 删除 journal 的转换只由 Workspace-private event/reducer 触发；下游 owner result 只能作为已验证事实输入。
- Template builtin upgrade 只进入 `workspaceTemplateUpgradeEvent → submitBuiltinTemplateUpgrade`；比较与替换在同一
  owner-local lock 内，并与普通 Template CRUD/ensure 串行。current full-content hash 必须精确命中 reviewed historical
  fingerprint 与 caller expected hash，target hash 必须等于当前内嵌值。未知/定制/stale 内容拒绝；已达目标的 exact replay
  不写入。此私有 catalog 事务不修改 definition admission、active-run slot 或 SM-1～SM-4。
  内置 adapter 升版必须登记此前发布内容的精确指纹，并以完整 frozen fixture 验证自动更新及定制保护；
  `TestEntrypointTemplateUpgradeReopensReviewedPreviousDefault` 覆盖 memory/file reopen，不凭内置 ID 扩大升级范围。
- 不拥有 Run SM-1、Orchestrator SM-2/SM-3/SM-4、Result 裁决、Decision/Approval 生命周期或 capability 执行。
- 不解析默认项目目录，不读取 capability catalog，不选择 Runtime operation/target，也不跨 owner 直接清理资源。

### 规格候选与模板组合

- Template 的 `ParentTemplateIDs` 由 Workspace owner 解析为有界 DAG；父声明顺序决定稳定拓扑顺序，共享祖先只合并一次。循环、缺失、重复父、深度/规模超限或无法安全合并的冲突拒绝。DoD 合取，不允许子模板覆盖父验收要求；同路径 context、同 ID evidence policy 的不一致当前直接拒绝，不猜测哪个更严格。
- 模板读取返回 effective 内容及只读 `EffectiveSources`。父内容及拓扑参与派生快照身份；create/update 只能提交原始声明，不能自报 effective 来源。新 Start/Restart 在 owner mutation lock 内核对整个有效快照，既有 Run 不热替换；无父模板保留既有 snapshot hash 算法。
- `DefinitionAdmissionCandidate.Specification` 是可选、版本化的规格提案。资源、维度、需求来源、义务、exact Profile 验证声明和六类 completeness review 全部进入候选身份；审查绑定不含审查自身的 content hash，startup 的 `SpecificationHash` 绑定包含审查的 admission hash。
- 独立 Goal requirement 必须唯一绑定 `GoalItemRef`、`USER_EXPLICIT` 与完整原 Goal 文本；它不是候选 DoD，不要求借用或补造 DoD obligation。仅 Required 且精确独立绑定的 Goal 沿用输入的 32 KiB 字节上限，普通 requirement 与 obligation 保持 4096 字节；不截断或扩大请求预算。非 exact、重复或混来源的无义务 requirement 仍阻断；各 DoD 的义务、验证声明、Profile 适用性和六份唯一 PASS 审查保持完整合取。Application 的六审请求携带同一 owner 派生的 `ExpectedResources` 和无 Reviews 的 immutable proposal；critic 只判断，不修改 proposal 或回显不属于 review wire 的资源字段。
- Authoring 提案只接收 `workorch.authoring.sealed-ref-mapping/v1` 的顶层 `mapping`：provider 提出分组、来源与资源引用以及验证声明，owner 按封存 input／derivation 编译 canonical 规格。每个候选恰好属于一个分组并有一个 obligation，Goal 单独绑定；provider 不指定义务原文、SourceKind、Required、资源事实或 canonical IDs。成功 completion 必须从完整原文重新解码，与 typed mapping 精确相同，再重新编译验证 canonical proposal；原文与 wire 的 byte count／SHA256 进入 completion 身份，但不成为准入或执行权威。普通 DTO 仅带 metadata，受限 success-byte-audit query 才返回原 bytes；save／replay／archive／derivation／preparation／私有 reducer 与 CAS 前后像均反证 current/history 的原封存来源。
- provider 的 derive/proposal/review 文档仅接受当前封闭 wire 形状：所有字段必填、精确大小写，标量与数组不得为 null；未知／重复字段、多 root、超出 256 KiB／32 层、非法 UTF-8 与孤立 UTF-16 surrogate 均拒绝。不修复或重写原始输出；结构拒绝保留 field path／错误分类，owner 来源、声明、独立审查及精确确认仍独立校验。`SourceItemRefs`、`ResourceIDs` 与候选 `DoDItemRefs` 是分离的身份空间，资源路径／章节不能充当来源。`UNVERIFIED` 的三个 Profile 绑定字段必须为空，不能静默清空非法绑定再继续。
- 六类审查分别覆盖维度、负路径、回归、资源环境、显式约束追踪和可验证性。`RequiredReviewRequirementIDs` 从 owner 编译规格派生每类应审的全部 Required IDs（含独立 Goal），Application 以 `requiredRequirementIDs` 提供给 critic。PASS 的 `RequirementIDs` 必须覆盖完整集合，合法 optional 项可额外审查但不能替代；缺少资源、负面路径或回归声明不是免审事实，不接受 N/A 或空范围自豁免。`goalSpecificationReviewContext` 在模型与持久 ledger 中反证覆盖，provider parser 同样尽早拒绝并保留 kind／缺失 ID；不补写 provider 结果。FAIL/UNKNOWN、缺必需资源、缺必需维度/义务、缺 DoD 映射或不匹配的 Profile 声明不能晋升。PASS 只是有来源的评审事实，不直接授予 Runtime evidence credit，也不证明任意自然语言目标的完整性。
- 验证声明适用性必须逐项沿 `Verification → Obligation → DoDItemRefs → 正式 DoD sourceID → Profile(scope,id|*)` 反证；一个义务引用多个 DoD 时全部检查，不因同名 declaration 存在或另一个 DoD 命中而通过。正式候选引用中的 `:item:NNNNNN` 保留原序号；derived 引用按确认 after-image 的 user、template、generated 分组及组内原序号解析，不能使用准入 normalize 后的 slice 位置。`dod-001` 是 sourceID，不是 `cwi-001`；Profile bootstrap artifact 和其他 work unit 独立占用 CWI 序号。
- Workspace 保留规格图解析；中立 `workorchruntimecontract.CompileRuntimeSourceEvidencePolicy` 与 activation 共用 exact→wildcard 的完整合并及 source selector matcher。所有命中声明共同贡献 evidence／operation 集合、约束及 owner-fact closure，冲突不能被 exact 优先级隐藏；被引用声明的非空角色、selector、recipe／check、合取约束和 closure 必须在最终产物保留。wildcard 的 recipe／check 被剥离，带这些具体身份的 wildcard 不能据 selector 命中冒充适用验证；无具体 recipe／check 的合法 wildcard 可贡献声明约束，默认 software-dev 模板仍可准入。此门禁不读取 capability catalog、不证明实际可执行／通过，不生成 `CoverageEvidenceBinding`，不改确认 hash 或旧封存准入。
- 无绑定 `UNVERIFIED` 可持久表达草稿缺口，但六份 PASS 也不能取得确认资格：全部 Verification 必须为 `DECLARED` 且声明非空，每个 Obligation 必须有声明。owner 在提案 completion、确认准备和 prepared 恢复时核对原封存 Template 的实际 Profile 适用性，准入再核对当前 Workspace／Template 合同；逐项覆盖全部确认后 DoD source 并保留最终合并贡献。上述检查不产生 owner fact、proof 或 credit。
- 确认 journal 遇模板／资源漂移保持阻塞，不新增 abandon／清 journal 协议。恢复原 exact snapshot 后，owner 重新核对原 input／permit／proposal／specification／configuration identity，沿 prepared→materialized after-image 幂等续写；任何冲突正式行仍阻塞、不得覆盖。prepared 必须匹配当前 Template snapshot；materialized 的 exact confirmation replay 不重新物化模板，但不等同于 activation eligible：准入和显式 Start 仍校验当前模板，资源绑定的 replay 每次仍向 File 重验。不得制造新 hash 绕过已准备事务。
- Workspace 的 `pkg/definitionadmission.CompileRuntimeSourceObligationInventory` 只从 eligible exact candidate 与 startup/admission 身份投影完整来源图；保留原 Requirement、Obligation、Verification 正文及正式 item ref，不读取草稿、不授予 Runtime binding 或 credit。声明逐项反查封存 Profile 的完整适用贡献；`runtime_source_obligation_test.go` 固定共享 DoD、多源身份、原文、独立副本及 seal／声明漂移拒绝。Application 最终 bootstrap gate 从原 candidate 独立重编 inventory；自洽删 B 或删除整份 inventory 不能通过该前像反证。
- nil Specification 保留既有候选与 startup hash。此合同约束提交的规格候选，不表示所有 Web/TUI 输入已自动经过完整 Goal Compiler；正式执行仍须通过 exact CWI、owner feasibility、sealed dispatch 和 SM-2 闭包。

### 原始创作诊断

Web 不以浏览器持久存储保留创作输入或事务恢复身份；已保存草稿与确认恢复依据由 owner 管理。Application 从已校验的 typed query confirmation 投影有界恢复参数，不能暴露整个 journal 或从页面旧状态重建。ExpectedWorkspaceRevision 仅限制首次确认准备，精确 journal 重放仍校验原 input／permit／proposal／specification hashes；查询不驱动确认、provider 或 Run。

派生／规格提案／审查解析失败时，Application 经 typed completion 提交原始 error 和响应 bytes，Workspace owner
将其与失败结果绑定并持久化。provider 确定失败、不完整输出及严格 parser 失败均可保留各自独立的文档／wire 观察，
transport leaf 另存 bytes；parser 失败不补造 provider failure code，完整文档须与原 payload 的 byte count／SHA256 一致。
原文不脱敏、不替换、不静默截断；actual observed bytes／SHA256、observed、complete、saved 与 reason 共同表达完整性，
未读取不发明 hash，部分观察的 hash 不代表完整响应。单项文档／wire／cause 保存上限 256 KiB、parser 错误 64 KiB；
派生沿用 16 条 request，proposal／review 沿用 112 条 request 及各阶段 2 次预算。超限不保存截断前缀并明确报告，
不将已知失败倒退为未决。原始诊断不进入 permit 身份、specification hash 或 startup authority。

请求诊断随owner Clone深拷贝，重复completion必须精确一致，变更诊断冲突；reserve不接受预填失败诊断。查询按Workspace/permit精确scope走typed EventHub，仅只读原请求，不驱动provider。普通 view 仅返回错误分类及 hash／bytes／保存状态，原始错误、响应和 provider 观察通过受限查询按需单条读取；旧失败无诊断时不能虚构历史原文。包含诊断的Workspace运行文件必须0600，权限通过原子publish收紧；不写Git或通用日志。数据保护依靠访问边界而非改写数据，见 [Workspace 规格创作](../../../../docs/40-profiles-capabilities/workspace/setup-draft-session.md)。

同输入的已失败请求保持原失败及 permit；Application 重复 compile 或进程重启不授予新的调用权。软件输出合同升级不迁移或清空历史 ledger。真实输入／模板修订继续通过原 CAS 和累计预算，不以无意义输入变化或新请求 ID 绕过失败。

### 创作只读摘要

Get/List 从当前未确认创作 record 派生有界 `AuthoringSummary`，仅含阶段、失败 phase/code、已完成审查数和 input revision；无当前记录或已经进入确认／准入时不补造摘要。只有六审查 PASS、全部声明满足确认结构门禁且适用于原封存 Template 实际 Profile 时使用 `reviewed`；未声明或自报绑定不适用均为 `review_blocked`。列表与详情复用同一声明适用性边界，摘要仍不宣称模板当前性、当前 capability readiness 或正式确认资格。摘要经 typed DTO 传递，模型载体不序列化，memory/file repository clone 丢弃该观察，不能反向恢复 owner ledger。创作保存、reserve、complete 和准备诊断的成功 CAS 复用 `WorkspaceStateChanged` 通知刷新投影；无写入重放及失败 CAS 不发该通知。通知仅用于读取刷新，不是执行或恢复权威。

### 前置准备诊断

前置准备失败使用独立 `DefinitionAuthoringPreparationDiagnostic`，经 `RecordPreparationDiagnosticCommand/Result` 进入既有规格私有 dispatcher/CAS，原文经 `QueryPreparationDiagnosticCommand/Result` 按 Workspace／DiagnosticID 读取；不是 provider request 或 permit。诊断绑定 input／template、目标阶段与请求身份；观察 revision 只用于首次 CAS，语义 ID 不包含该 revision。duplicate 先于 stale 校验返回原记录，迟到新失败不能重绑最新输入，已 reserve／确认后旧诊断不参与当前失败状态。

原始 cause 为 bytes，单条 64 KiB，超限保留完整 hash／bytes 和明确未保存状态；累计创作账本最多 32 条，满时拒绝且不淘汰。Clone／Validate／修订继承与确认归档均保持原文，当前和历史共用 0600 原子发布。普通管理 view 与普通 concrete DTO 均仅携带 metadata，不得以 metadata 校验冒充 owner 完整持久化反证；原始字节仅经受限 scoped query 返回，不进入 public JSON 周期轮询。仅确定未签发 permit 的 typed budget refusal 可记录为前置失败；未知 reserve 冲突、错误响应或响应丢失不能推断未 reserve。诊断不增加预算或调用许可，也不改变 SM-1～SM-4。

## 能力验收

| 能力/维度 | 测试证据 | 结论 |
| --- | --- | --- |
| 前置准备诊断与零 permit | `pkg/models/definition_authoring_preparation_diagnostic_test.go`、`pkg/events/definition_authoring_preparation_diagnostic_test.go`、`pkg/eventmapping/definition_authoring_preparation_diagnostic_test.go`、`biz/definition_authoring_preparation_diagnostic_test.go`、`internal/pkg/repository/file/diagnostic_permissions_test.go` | 覆盖空／非 UTF-8 bytes、hash、超限未保存、32 条容量、完整 typed 校验、duplicate／stale／并发 CAS、删除栅栏、重开与历史继承、0600；不增加 provider 请求或 Run |
| 创作 EventHub DTO 来源与隔离 | `pkg/eventmapping/definition_authoring_test.go`、`pkg/events/definition_derivation_test.go` | record／derivation／history 显式往返保留快照与 permit／预算；模板、资源、派生结果、回执与正式 after-image 深拷贝；当前／嵌套／历史的重复或空 binding 在 map 转换前拒绝 |
| 创作阻塞后继与确认恢复 | `biz/definition_authoring_successor_test.go` | 同 CandidateID 连续 revision、旧票据 resolved／新票据 open、候选追加与 pending startup 重开、安全 resolution、prepared／Goal／DoD／materialized cutpoint 及零自动 Run；不是 HTTP/Web 闭环验收 |
| 多父模板 DAG 与解析结果隔离 | `pkg/models/template_inheritance_dag_test.go`、`biz/template_inheritance_test.go` | 覆盖 diamond 去重、父顺序身份、共享祖先最长路径上限、父引用深拷贝及 owner 模板操作 |
| 验证声明 source 适用性与生产 Start 对照 | `pkg/definitionadmission/goal_specification_test.go::TestCompileVerificationAppliesToEveryObligationDoD`、`TestSpecificationDoDSourceOrderIsConfirmationAfterImageNotNormalizedSlice`、`biz/definition_authoring_admission_test.go::TestAuthoringVerificationApplicabilityBlocksWrongDeclarationBeforeRun`、`TestAuthoringApplicableDeclarationsSurviveProductionBootstrapAndExplicitStart`、`activationports/definition_admission_test.go::TestBuiltinSoftwareDevSpecificationWildcardRemainsApplicableToProductionBootstrap`、`workorchruntimecontract/runtime_profile_contract_test.go::TestRuntimeSourceEvidencePolicyPreservesExactAndWildcardContributions` | wrongid proof／wrongscope／otherdod／多义务／单义务多 DoD、recipe/check stripping、shadowed recipe 与 exact/* 冲突 fail closed；合法 exact/* 保留贡献，经真实 production bootstrap 和 Workspace/Run owner 显式首次执行创建单一 Run；默认模板可用；不代表 SM-2/3/4 执行或验证 credit |
| 确认 snapshot 恢复与冲突保护 | `biz/definition_authoring_confirmation_test.go::TestAuthoringConfirmationRejectsTemplateAndConfigurationDrift`、`TestAuthoringConfirmationRejectsConflictingFormalRows`、`biz/definition_authoring_resource_final_test.go::TestResourceBoundConfirmationRejectsDriftBeforePrepareAndAtPreparedCut` | 恢复原 Template／资源 snapshot 后原 identity exact replay；file owner 重开、prepared/materialized 差异、正式行冲突仍拒绝且零覆盖／自动 Run；不提供 abandon |
| 未 opt-in lineage Profile 的持久化身份 | `internal/pkg/runtimeprofiledecode/lineage_assertion_test.go::TestRuntimeProfileWithoutLineageRulesPreservesExactPersistenceIdentity`、`biz/definition_authoring_process_recovery_test.go::TestAuthoringProcessDecisionAdmissionRecovery` | 缺声明保持 nil，JSON/file reopen 保持 exact Profile 身份；不得以放宽 owner 比较或回填历史修复空切片漂移 |
| 独立 exact Goal requirement 与 DoD 门禁 | `pkg/definitionadmission/goal_independent_requirement_test.go::TestCompileIndependentExactGoalDoesNotInventDoDObligation` | 唯一原 Goal 无伪 DoD obligation 正例、重复／混来源／改原文／错 kind 拒绝、普通 DoD obligation／DECLARED／六审缺口仍阻断；只证明纯准入，不替代 current mapping 全事务验收 |
| 规格候选结构与防降级 | `pkg/models/goal_specification_test.go`、`pkg/definitionadmission/goal_specification_test.go`、`biz/goal_specification_persistence_test.go` | 覆盖六类审查绑定、引用唯一性、占位验证声明拒绝、startup 哈希及 owner 后继候选禁止移除规格；不等同于自动规格生产入口验收 |
| module lifecycle与 owner repository binding | `module_test.go::TestModuleIdentityConfigureStartStopContract`、`module_test.go::TestModuleConfigureRequiresBoundOwnerRepositoriesAndEventHub`、`internal/pkg/repository/resource_owner_repositories_test.go::TestResourceOwnerRepositoryBundlesRejectMixedMemorySets`、`TestResourceOwnerRepositoryBundlesValidateForMemoryAndReopenedFileBackends` | 已覆盖必需 aggregate/EventHub、memory/file factory 正向构造、zero/unbound/mixed set 启动前拒绝与 file reopen binding 重建 |
| Workspace create/get；Goal/DoD scope与definition revision；Template mapping clone | `biz/goal_dod_manager_test.go::TestGoalDoDManager_CreateWorkspace`、`biz/goal_dod_manager_test.go::TestGoalDoDManager_DefinitionRevisionIsMonotonicAcrossContentReversion`、`biz/command_router_test.go::TestWorkspaceDefinitionCommandsEnforceScopeAcrossBackends`、`biz/template_commands_test.go::TestWorkspaceTemplateOwnerMappingsDeepCloneCommandAndRepositoryValues` | 已覆盖这些明确路径 |
| Workspace update/list、Goal/DoD update/delete/list、Template CRUD | `biz/command_router_test.go::TestWorkspacePublicManagementCRUDPositivePaths` | 已覆盖；Workspace coordinated delete由下方deletion矩阵验收 |
| Template explicit upgrade | [`biz/builtin_defaults_test.go`](biz/builtin_defaults_test.go) | memory/file、seed 不覆盖旧版本、错误 source/Post、未知/定制/stale/changed-target 拒绝、写后响应丢失与 fresh-handle replay、并发普通编辑保护 |
| automatic configuration / archive isolation | [`biz/current_configuration_admission_test.go`](biz/current_configuration_admission_test.go)、[`biz/defaults_test.go`](biz/defaults_test.go)、[`pkg/models/template_test.go`](pkg/models/template_test.go) | file reopen、旧 schema 原样保存、当前配置准入与幂等、无效配置阻塞、并发 fence、Soul fresh read、历史 Run 零改写、未定制内置 Template 自动更新 |
| Definition normal/unsupported/duplicate/stale/provenance | `biz/definition_admission_test.go::TestDefinitionAdmissionIntentAutomaticallyPromotesEligibleCandidate`、`biz/definition_admission_test.go::TestDefinitionAdmissionIntentKeepsUnsupportedAlternativePreseal`、`biz/definition_admission_test.go::TestDefinitionAdmissionRejectsSelfConsistentUnpersistedDecisionTicketWithoutPromotion`、`biz/definition_admission_test.go::TestDefinitionAdmissionRejectsStaleLineageCAS` | 已覆盖 |
| lossless Profile identity / current-schema restart | `biz/definition_profile_identity_test.go::TestDefinitionProfileIdentityRejectionSurvivesOwnerRestart`、`workorchruntimecontract/runtime_profile_identity_test.go` | required→optional 重复声明不能消失；file-backed candidate 保留两项、fresh owner 重复零写、零 startup/activation、同 hash 篡改拒绝；无 Run/provider/capability 依赖 |
| privileged source matrix 与零 mutation | `biz/privileged_source_test.go::TestWorkspacePrivilegedIngressRejectsForgedSourcesWithoutOwnerMutation`、`biz/privileged_source_test.go::TestWorkspaceTemplateDefaultsIngressAcceptsOnlySystemOwner` | 已覆盖 |
| execution normal/duplicate/stale/restart | `biz/execution_intent_test.go::TestWorkspaceExecutionIntentDuplicateReplayUsesActiveRunAndRejectsResultDrift`、`biz/execution_intent_test.go::TestWorkspaceExecutionInternalEventRejectsStaleStartupWithoutMutation`、`biz/execution_intent_test.go::TestWorkspaceRestartMaterializesDefaultSelectionByPrivateEventBeforeRunOwner` | 已覆盖 |
| Run projection lost/duplicate/stale/restart/public notification | `biz/run_projection_reconciliation_test.go::TestWorkspaceRunProjectionFreshOwnerRecoversWithoutHistoricalNotification`、`biz/run_projection_reconciliation_test.go::TestWorkspaceRunProjectionRevisionFenceRejectsStaleAndConflictingFacts`、`biz/lifecycle_reducer_test.go::TestReduceRunProjectionFactDuplicateIsNoop`、`biz/run_projection_notification_test.go` | 已覆盖 cursor 持久化、真实公开字段变化通知与 cursor-only 零通知 |
| deletion normal/negative/idempotency/restart | `biz/deletion_coordinator_test.go::TestWorkspaceDeletionCoordinatorUsesExactRetainAndPurgeOrder`、`biz/deletion_internal_event_test.go::TestWorkspaceDeletionRejectsOutOfOrderAndMismatchedOwnerAcknowledgements`、`biz/deletion_coordinator_test.go::TestWorkspaceDeletionResponseLossResumesFromPersistedPhaseOnExactDeleteRetry`、`biz/deletion_coordinator_test.go::TestWorkspaceDeletionFreshOwnerStartRecoversPersistedFileFinalization` | 已覆盖 coordinated delete 正向路径与恢复 |
| file-backed admission/clarification cutpoints | `biz/definition_admission_file_recovery_test.go::TestDefinitionAdmissionFileBackedCutpointsRecoverThroughClarificationPromotion`、`internal/pkg/workorchrepo/workspace_scoped_repositories_test.go::TestWorkspaceScopedPreSealDecisionPathSupportsWorkspaceCRUDAndReopen` | 已覆盖 unsupported candidate、corrected candidate、无 RunID 的 Decision ticket、pending promotion、Decision resolution、eligible CAS 各 durable cut 后 fresh-owner reopen与 duplicate/response-loss replay；同时覆盖该 Workspace-scoped Decision file path 的 list/update/delete/reopen及 Decision-owner workspace purge。单次 repository atomic write 内部的 OS kill/fsync/rename crash atomicity 属于 host/filesystem deployment 边界，由 [`Service Acceptance / Persistence And Restart`](../../../../docs/50-product-interface/service-acceptance.md#persistence-and-restart) 验收，不由 Workspace reducer 推导 |
