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

### 激活前准备只读与纯编译边界

`QueryExactConfirmedStartupSnapshotCommand/Result`（`/workspace/activation-preparation/exact-startup/query`）
只接受 exact `application/workorchd`，使用 Workspace query lane 并参与只读 drain。请求必须携带
Workspace 与非零 expected revision。owner 反查完整持久 authoring/六审/确认/准入链和实际正式 Goal、DoD 行，
再读 revision 排除混合观察；缺失或漂移拒绝，不 fallback admission、不生成 ticket，也不物化资源。
返回有界 detached snapshot，保留正式 DoD 原顺序与逐项来源。snapshot hash 是内容身份，不是 readiness。
内部目录含无效 UTF-8 或 NUL 时明确拒绝，不借 JSON 替换字符、路径清洗或截断修复原输入。

`QueryWorkspacePreparationStateCommand/Result`（`/workspace/activation-preparation/state/query`）
同样只接受 Application source、使用 query lane。只读取本 owner 已有准备操作账本，
无记录返回 `not_checked`，不为读取创建记录。`State` 与 `CurrentValidity` 分离：时间、配置、
当前 owner revision、封存身份或检查/编译版本的已知漂移可投影 `stale`，但不覆盖原
`accepted/blocked/failed/unknown` 或清除原请求身份。查询不检查远端资源、不对账或恢复未知操作。
`ActiveRunID` 只是独立已有投影，不以空值或记录缺失证明没有历史 Run。

`pkg/startuppreparation` 只从 exact snapshot 纯编译有界、稳定的 required 检查清单；
选择项要求的是原合同实际选中集合的完整性事实，不要求安装未使用能力。清单无 PASS 字段，
不将 DECLARED、空列表、配置存在或 membership 当成 owner 可用性事实。
当前持久记录不含正 readiness、M3 或激活权限；首次完成与 no-Run 并发协议不由上述查询实现。

原结算保存 `Assessment`（`workorch-preparation-all-of-assessment/v1`）：绑定原 manifest，
给出完整 required 数、通过数，以及按原清单顺序保留 ID/owner/state/reason 的全部未通过项。
同一 CAS 保存单项事实和汇总，不额外写一次事务。`PreparationCheckRequirements` 同时约束 manifest
与持久读回的清单成员：从原 candidate 恢复十类检查及逐条规格资源；删项、换 owner、required 降级
或自洽重算的汇总都不能取代完整 owner 反证。`checks_passed` 只描述清单合取，绝不是 ready/M3。
GET/原请求重放只读原值；旧记录无汇总时不补造，后续版本漂移仍独立投影 stale。

同次原结算在完整 seal 可反查且未漂移时保存 `DefinitionCompilation`
（`workorch-preparation-definition-compiler/v1`）：从原 snapshot/manifest/eligible candidate
纯编译完整 source obligation inventory，记录原 snapshot/manifest、inventory/profile hash 和
source/requirement/obligation/verification 数量。即使 Provider 阻塞也能完成此独立编译，
不填假模型、不调用 activation port、不创建 Runtime；它不是完整 prepared bootstrap 或 readiness。
File/memory repository 在写入和读取时从完整原封存材料重新编译并比较全部摘要，hash 不作为成功凭据。
旧记录缺省不补造；中断/漂移不补编译，已完成摘要不允许后续删除或替换。

原非漂移结算可在同一私有 seal archive 保存 `ExecutionInputs`
（`workorch-preparation-execution-inputs/v1`）。Application 仅双读明确的非敏感进程输入：
配置对话语言、LC_ALL/LC_MESSAGES/LANG 原值、Runtime 修复上限与文档恢复上限；缺 loader、
双读漂移、无法完整表示或取消时不以默认零值补证。Workspace 自行捕获完整配置 hash 前像和
已确认 authoring Template snapshot，不接收 Application 提交的 Workspace 策略或模板。
配置前像保留原字段及空集合形态，hash 保持既有字段顺序与 nil 规范化，不含网关凭据。
Repository 反查原确认账本、模板全文、原配置 hash、scope/revision/时窗，并在首次 CAS 精确比较
真实 Workspace 前像；分目录适配器在本地持久锁内真实读取 Soul sidecar 后做完整前像比较，
底层 metadata CAS 只核对自身实际保存的字段，不把被拆分的 Soul 绑定误当成完整配置。
这不承诺对外部文件修改的跨文件原子性。既有 seal archive 的 clone、0600、append-only 和一一引用约束继续适用。
公开只投影 `executionInputVersion`，不输出原配置或进程输入；该字段只是原材料归档版本，
不是策略编译成功、持续 currentness、完整 bootstrap、M3 或执行权。
GET、重放、恢复与重启不以当前配置/环境回填旧无输入记录。

显式 check/recover 的 typed 回复可附 `CompilationSource` 无 map 的具体 DTO 原材料，
显式可逆转换、深复制配置和完整诊断 bytes；不以普通投影的 metadata 代替原文。
仅供 Application 从同一原操作归档完整结构编译；与原 state 的 scope、revision、attempts 及
全部 evidence persistence 精确关联，消费者独立完整重验，`json:"-"` 不进入 HTTP。不提供 repository 或可变 owner 句柄，
不接受调用方回传它作为成功凭据。GET 不附该源、不编译；原材料缺失不回填，
编译产物不写入新的成功标记，也不改变逐项结果、有效性、M3 或执行授权。

`emptySelectionEvaluationVersion=workorch-preparation-empty-selection-evaluation/v1` 只消费有界的
精确空 Skill/MCP 选择。必须有原 Goal/DoD 与 seal 绑定、完整六阶段 Soul、完整 Registry/SkillHub
前后材料及独立重编译的 Skill selection/context/references/status；真实选择为空时，
Skill 单项可为 `passed/skill_empty_selection_observed_remaining_checks`。
在此基础上，完整 MCP 双读、原配置及 Skill 依赖重编译均无失败且选中 server/capability 都为空时，
MCP 单项可为 `passed/mcp_empty_selection_observed_remaining_checks`。不是把缺来源当空、跳过 required
或声明通用 N/A；非空集合仍需既定资源事实与连接证明，不启动 MCP 或联网补证。
原编译材料和其旧诊断原样保留，正向消费由独立版本描述，不改写原诊断。
此处仅证明原检查时点的选择闭合，不产生持续有效性租约、Provider 授权、M3 或执行权限。

`/workspace/activation-preparation/check` 与 `/workspace/activation-preparation/recover` 使用 state lane，
只接受 Application source 的 concrete `CheckWorkspacePreparationCommand`，返回
`CheckWorkspacePreparationResult`。输入仅为 Workspace、原 RequestID、原 expected revision，
不接受 caller checklist、PASS 或 owner 已观察标志。公开 HTTP 与 Web 经 Application 发送同一合同。

`workspace.preparation` 是 Workspace owner-private transaction，不是 SM-1～SM-4 迁移或执行授权。
原显式结算在所有原来源观察后，另向 File 请求完整目录/资源/托管配置后像，附在既有受限目录 archive；
原 bytes、失败及检查结论不覆盖。完整 scope 与原 resource/config archive 的数量、顺序、根路径和时间反查，
仅重算 hash 不能删除目标。File 后像变化、30 秒观察年龄上限到期或当前内存 epoch 不符均投影 stale；
缺当前 epoch 也 fail closed。GET、重复 check 与 recover 使用同一只读投影，只查询 File 内存 epoch，
不重新探测文件、不续期、不写历史；旧记录不补发 token。该 File 投影不是文件租约，
未来 use-time 仍须真实重核。

新原操作在 accepted CAS 固定 `OwnerRevalidationVersion=workorch-preparation-owner-revalidation/v1`，
settlement/recovery 不可更换或为旧操作补挂版本。原 Soul/Skill Registry/Memory（含原模式）/MCP Registry
四类来源完整时，Application 在同一原请求 deadline 内复用既有只读 typed query 取得最终完整原值；
不重发 SkillHub status、不刷新 catalog、不连接 MCP、不推理或 embedding。
`SettleWorkspacePreparationBlockedCommand.FinalOwners` 不含调用方判词，Workspace 精确核验
Workspace/request/payload/revision 与时间窗，随原 seal 在同一 blocked CAS 保存，clone 隔离且终态不可覆写。
最终值只忽略观察时间差异，与唯一原 archive 的完整值逐项比较，保留 epoch、revision、顺序、nil/empty、
内容及配置/选择来源。原 archive 完整但最终值缺失、漂移、时钟回退或最终观察开始满 30 秒均投影 stale。
原来源不齐仍阻塞，不为凑结构重复查询；GET/replay/recover 不重读这些 owner，不续期、不改历史逐项结果，
旧操作不补造观察。该材料仍是时点事实，不是全资源有效性、ready、M3 或执行授权；公开投影不含原文和内部版本。
owner 反查 exact snapshot 并纯编译完整 manifest 后，私有 `workspacePreparationAccepted` reducer
经 CAS 保存 v2 原意图 hash、封存绑定、creation/preparation epoch 和逐项 pending 清单；
只有初始 CAS 明确成功的调用获得一次非持久延续提示；Application 释放 Workspace 事件调用后，
向 LLM/Run owner 发 cache-only/完整历史 query，再以具体 `SettleWorkspacePreparationBlockedCommand`
经内部 state-lane `/workspace/activation-preparation/settle-blocked` 返回负向分类。
此合同只接受 exact Application source 与原意图；不接收 caller PASS、M3 或执行权限。
资源数据延续仅为明确的 concrete typed 事实：Soul/Registry 完整有界前后值，以及
`PreparationProviderCacheFacts` 的单点完整缓存观察，以及 `PreparationRunHistoryFacts` 的完整 Run owner 结果；
不接受已观察成功标志。`workspacePreparationBlocked` reducer/CAS 记录具体缓存/历史阻塞码，
未接入消费者的项仍为 `blocked/check_pipeline_incomplete`，没有 PASS 或 N/A 捷径。
负向分类本身不是完整资源证据归档，
新鲜声明、空历史均不替代 required 事实、final epoch/CAS 或 M3。
`workorch-preparation-run-history-evidence/v1` 在原 accepted → blocked 结算 CAS 中保存
完整 `ObserveWorkspacePreparationHistoryResult`（原 request、观察时间、Workspace scope、generation head、
Run/pending/replacement 计数及 retirement fence），连同原 binding、payload、revision 和 creation/preparation epoch。
材料私有、clone 隔离、0600 保存且 append-only；公开只投影随机 `runHistoryEvidenceRef`
（`run-history-observation:` 加 32 位小写 hex），不是 no-Run 凭据。
归档与原 attempt 引用一一对应，完整 typed 事实和原 scope/revision/时间窗均重验；
有历史/retirement 分别保持 `run_history_present/retired`，仅空历史仍是 `run_history_empty_fence_not_final`，
required/run/blocked，不能单凭空集合升级 passed、N/A 或 M3。漂移保留原材料，仅以 `binding_changed` 阻塞；
超界或无法保存完整观察不附引用，不把部分材料当完整事实。
GET/replay/recover 不重取、不回填旧无材料历史，也不替代 reservation/epoch/final CAS。
原非中断结算可另带 `noRunEvaluationVersion=workorch-preparation-no-run-final-fence/v1`：
只有完整空 Run-owner 历史、完整精确封存与持久完整性两个通过项，以及当次实际 Workspace CAS
前像联合满足时，`first_activation_no_run` 单项才为 `passed/run_history_empty_final_fence_observed`。
私有 Run-history archive 保存完整 detached `FinalFence`（原 Workspace revision、ActiveRunID 和完整 creation/reservation fence）；
必须为同一原 accepted attempt、expected+1 前像与 expected+2 提交、creation 精确相等、epoch=0、
无 pending/settled 且 ActiveRunID 为空。Repository 除完整持久反证外，独立比较实际 CAS 前像及原封存材料，
拒绝仅重算 hash、自洽替换、unknown 恢复补造、并发 reservation、revision 漂移及同提交引入 Run。
公共 DTO 只增加有限版本，不输出完整前像；总体仍 blocked，不签发 M3 或执行权。
此单项是原结算时点的历史证明；之后 fresh reservation 使 currentValidity stale，原材料和 passed 不改写，
不据其声称“当前没有 Run”。无 FinalFence 的旧空历史保持原阻塞，不自动升级。
`exact_seal` / `persistence_integrity` 使用 `workorch-preparation-seal-evidence/v1`：
仅原非中断延续在 owner 内执行两次既有精确 query（真实正式 Goal/DoD、完整创作账本及 all_of 编译门禁不变），
封存两份完整 detached 输入、原 binding/request/payload、受理后精确 revision 和观察时间。
两读不相等或来源不可用不授通过；新材料随原结算 CAS 进入私有 `PreparationSealEvidence`，
公开仅有随机 `sealEvidenceRef`。读回反查原 candidate、完整六审账本、materialized journal、原正式行及
StartupInput/admission；旧来源从不可变 authoring history 定位，不借用新版本来源，不凭 hash 或 caller 标志授信。
两项通过分别为 `seal_observed_remaining_checks` / `persistence_binding_observed_remaining_checks`；
仅证明原时点精确封存及完整持久绑定，不证明磁盘永久可靠、持续 currentness、资源 readiness 或 M3。
原材料 clone 隔离、0600 保存且历史 append-only；stale 保留原材料但 blocked，GET/replay/recover 不重读或升级旧历史。

Provider 原观察使用 `workorch-preparation-provider-cache-evidence/v1` 私有封装，完整保留
LLM owner 的目录、capability 列表、revision、获取/过期/观察时间及随机生命周期 epoch。
原 accepted binding、request/payload、受理 revision 与材料在同一 blocked CAS 落盘；
普通 Workspace JSON 不含原材料，file wire 要求 0600，repository 反证 scope、时间、
原原因码和引用一一对应，完成历史 append-only。公开仅投影 `providerCacheEvidenceRef`
（`provider-cache-observation:` 加 32 位随机小写 hex），不是内容摘要或成功凭据。
旧 cache-only 历史的完整 miss/expired/fresh 仍为 miss/expired/declaration_only 阻塞；观察无效或未保存不补造历史。
新原非 stale 结算从完整冻结 provider configuration 声明和本次 Workspace.DefaultLLMSelection 编译
`workorch-preparation-provider-selection/v1`，在同一私有 archive 保存来源、精确选择、原 descriptor、
override 身份和失败码。显式 Workspace 选择优先；nil 才使用同次 LLM owner 的 DefaultChat，不按目录顺序猜模型。
独立读回复算全部选择；首次 CAS 逐项反查真实 Workspace 原声明，完整配置身份绑定原 accepted binding，
不以缺少另存 Soul 的 scoped metadata 重算完整配置 hash。新非 stale configuration 归档不得省略 Selection；
旧无 configuration 来源不补编，stale 不产生新编译。完成历史不可覆盖、删除或降级。
miss/expired 仍分别 blocked；其余公开有限 `provider_selection_*` 原因。旧无容量观察的成功编译仍为
`provider_selection_authorization_pending`。新首次 CAS 成功回执仅向原 Application continuation 返回
detached 原 Workspace provider 声明，绑定受理配置 hash；重放及恢复不返回新观察意图。
Application 将原精确 target（nil 才用冻结 owner 默认）交给同一次 cache-only query；LLM owner
仅从已有 fresh cache 和冻结配置调用既有纯 resolver，返回
`workorch-preparation-provider-budget-observation/v1` 的容量预览，不访问网络或刷新 catalog。
Workspace 将完整观察及编译后的容量原值随原结算保存并反查目标；新有限原因为
`provider_selection_budget_observed_authorization_pending`、`provider_selection_budget_unavailable` 和
`provider_selection_budget_source_conflict`，全部仍 blocked。不输出 credential-derived 配置 revision，
也不生成 Runtime ProviderCeiling、请求 allocation、Run 物理预算或远端授权证明。
原历史缺容量不补算；容量原值和独立重编译结果受同一 clone、hash、append-only 与恢复门禁保护。
新原因必须绑定原逐项 evaluation version、required/llm/blocked 和完整历史 ref；任意后缀、raw-config、
caller PASS 与相邻模型替代均拒绝。不授予 PASS，不改原 activation 选择路径。
单点观察及选择编译不证明 epoch 仍存活、持续 freshness、连通性、授权或 M3；
GET、重放、恢复及 owner 重开均不重新查询 LLM，revision/epoch 漂移只保留历史并标 stale。
状态读取另从最新操作的完整原 Provider 材料核对既有 catalog `ExpiresAt`：到期边界（含等于）
仅投影 `CurrentValidity=stale`，保留原 catalog state、逐项原因、操作状态、引用及请求身份。
不把过期历史归给没有观察的新操作，不从无材料历史补造期限；完整材料缺失/重复或原绑定冲突拒绝读取。
它不写 attempt 的全 required 最早 `ExpiresAt`，未过期也仍为 `not_evaluated`，不代表当前 epoch、授权或 readiness。
Memory 完整单点库存以 `PreparationMemoryInventoryFacts` 进入同一原操作；Workspace 独立反查
accepted 身份和 exact seal，保存原 binding/request/payload/revision/时间与完整 typed 库存。
`workorch-preparation-memory-inventory-evidence/v1` 私有材料以 0600 保存，公开仅有
`memoryInventoryEvidenceRef`（`memory-inventory-observation:` 加 32 位随机小写 hex）。
无模式来源的完整库存（含真实空库）仍为 `memory_inventory_observed_selection_pending`，无法取得/保存完整值为
`memory_inventory_unavailable_not_saved`；均 blocked。前者必须有完整一一对应的原历史，不能凭分类码补造。
新原操作可携带 `workorch-preparation-memory-selection/v1` 来源：Application 在完整库存观察前后通过
LLM owner typed query 读取 active provider/revision/defaultEmbedding，并双读 exact seal。Workspace 独立核验原
accepted 的 `ObservationSnapshotHash`、revision、时序及原 binding；不接受调用方 mode、PASS 或选择结果。
没有 embedding target 才按原 `activation-memory-selection/v1` 纯算法编译完整候选、选中、排除、quota、score/reason
与 byte/token budget；200 项选择上限之外明确 `memory_selection_candidate_limit`，不截断库存。
非空 semantic 库存为 `memory_semantic_evidence_required`，不降级 list-only；真实空库存保留实际 mode，
按既有 activation 空库 fast path 编译。完整 list-only 或真实空库可在原裁决版本下单项 passed，原因分别为
`memory_list_selection_observed_remaining_checks`、`memory_empty_selection_observed_remaining_checks`。
配置查询失败/错 DTO/双读漂移不会成为 nil embedding；已取得的完整库存保留，但选择仍 pending。
完整来源和纯编译与库存同一原 CAS 保存；读回从完整原材料重编比较，不接受 selected-only 或自洽重哈希替换，
clone 隔离 config target 与全部记录。旧无来源库存不升级，不写 Run recall trace，不调用 embedding、不授 ready/M3；
单点 epoch 不是持续有效性。GET/重放/恢复/重开不重查，写失败不补造，完成材料 append-only，
revision/epoch 漂移仅保留历史并 binding_changed/stale。
MCP 完整 registry 观察采用 `workorch-preparation-mcp-registry-evidence/v1`；Application 在原操作预算内
双读 exact seal 和 MCP owner 完整 snapshot，不使用 selected-only 回执。Workspace 独立反查原 accepted
`ObservationSnapshotHash`、request/payload、revision、binding 与观察时间，完整前后值随同一 blocked CAS
写入 `PreparationMCPRegistryEvidence` 私有材料（file wire 0600）。读回重新核验完整内容 revision、
双读精确相等与一一对应的历史引用；不接收 caller PASS，完成材料 append-only，三后端 deep clone。
公共只投影 `mcpRegistryEvidenceRef`（`mcp-registry-observation:` 加 32 位随机小写 hex）及有限阻塞原因，
不公开选中对象或原材料。未保存选择裁决的历史保持 `blocked/mcp_registry_observed_connection_pending`。
错误/超限/漂移/未保存为 `mcp_registry_observation_unavailable_not_saved`，不伪造空历史或保存部分内容。
原非 stale 结算在同一次 Skill 选择编译后，另以 `workorch-preparation-mcp-selection/v1` 保存私有
`Selection`：typed 默认声明沿用 Workspace-over-Template 递归覆盖，与该原操作完整选中 Skill 的
必需 server/capability 引用取排序去重并集。重复声明键、非法 ID、超限、缺 Skill 来源、缺失/禁用对象、
未选中所属 server 或 allowlist 不允许均保留精确失败，不补 server、不把缺来源当空选择。
成功只复制完整 registry 中对应的原 owner 值，不连接或授予就绪。首次 CAS 独立从真实 metadata
重派生 MCP 声明，完整配置身份绑定原 accepted 操作（scoped Soul 绑定另存，不用 metadata 重算完整哈希）；
读回反查同 request/payload 的完整 Skill 历史并重新纯编译全部结果，完成材料不可改写。
新非 stale 归档必须以私有 `SelectionEvaluationVersion=workorch-preparation-mcp-selection-evaluation/v1`
消费编译结果，并随同一 CAS 保存公共 `mcp_selection` 原因。非空成功是
`mcp_selection_compiled_connection_pending`，真实空选择是 `mcp_selection_empty_policy_pending`；
原编译失败映射为有限的 `mcp_selection_` 原因，具体清单见 service-api。所有原因均保持 blocked，
不将未知前缀当有效分类，也不从错误的失败码生成成功/空选择分类。公共结果必须为 v2、原裁决版本、
required/mcpregistry、原完整 registry 引用；私有读回从原材料重编并核对精确原因。
旧 registry-only 或无选择裁决版本的历史不补算、不升级；新归档不能省略该消费者版本，完成历史不可降级。
完整 registry 或选择编译均不证明 required connection、连接 epoch 或持续有效性；GET/重放/恢复/重开
不重读 registry、不连接、不 initialize/refresh/EnsureDefaults/invoke，revision/epoch 漂移只保留历史并 stale。
重复普通提交只返回原记录，不续跑 accepted/unknown；显式恢复仅结算为
`blocked/original_check_interrupted`，不重发外部效果、执行或资源观察。revision/epoch 漂移仍保留
原绑定并记录 `binding_changed`/stale，不把新输入冒充原检查。

同键异完整输入冲突，未决原操作不能被新键绕过；完成记录不可改写或删除，新检查只能追加（最多 64 次）。
file/memory repository 的 v2 backstop 拒绝非 CAS 变更、伪造正状态和历史覆盖，并深克隆逐项结果。
内部 settlement 的目录验证由 Workspace 从原 accepted 操作的当前 exact seal 解析路径，再通过 File-only concrete query
实际观察（不持有 definition/reducer 锁跨 owner I/O）。独立 `fileobservation.Directory` 仅为无权威纯值；
File 结果经显式 typed mapping 进入私有 `WorkspacePreparationDirectoryEvidence`，不是 JSON 权威桥。
原目录身份、有效权限、失败 bytes/字节数/SHA256 与原操作结算同一 CAS 保存；公共 record 仅有
`DirectoryEvidenceHash` 历史引用，普通 Workspace JSON、SSE 和 HTTP 不输出材料。文件 wire 显式保存私有材料且要求 0600；
clone、读回及 repository backstop 核验原操作/revision/时间/hash 和材料引用一一对应，终态不可改写或删除材料。
revision/epoch 漂移后原材料只作历史，所有项 binding_changed/stale；结算未提交的材料不在恢复时重造。
未声明逐项裁决版本的历史目录项保持原 blocked 语义，不自动升级；版本化单项裁决见下文。
同一原 settlement 还从精确封存的 Specification.Resources 解析有序 ID/ref/required/expected hash，
通过 File 的 `ObservePreparationResourcesCommand/Result` 实读交付目录中的完整当前文件。
独立纯值 `fileobservation.Resource` 经显式映射成为私有 `WorkspacePreparationResourceEvidence`；
原文 bytes/版本/诊断与 `ResourceEvidenceHash` 原子落盘，0600、deep clone、逐项清单关联及 append-only backstop 与目录材料同边界。
内容相等仅为 `resource_observed_remaining_checks`；未知原 hash、内容漂移、读取失败与 owner 不可用分别阻塞，
不把封存 Available 声明当作当前事实。超出完整读取边界不保存前缀；原失败与原文不规范化、不送 provider。
公共记录仅投影 hash/阻塞码，不提供材料；GET、重放、恢复及 owner 重开不重取资源，漂移后只保留历史。

新受理 attempt 封存 `EvaluationVersion=workorch-preparation-check-evaluation/v1`，并为目录、托管配置和
每个规格资源封存由原 manifest 与两目录身份确定的 `SubjectHash`；它只绑定原检查对象，不是成功凭据，
不包含配置原文或密钥派生摘要，不输出公共 DTO。它与原清单 owner/required 等字段在 accepted 后不可修改。
原私有结算只消费本次完整 File 材料：精确目录权限满足、配置原策略满足、资源原 hash 满足时，
对应项为 `passed/*_observed_remaining_checks`；已完整观察的目录权限不足、配置格式/内容冲突、
资源内容漂移为 `failed` 并保留原原因；缺材料、不完整读取或未知原 hash 仍 blocked，不解析诊断猜故障。
配置结算还从独立的确认模板与 Workspace 前像重编完整声明，逐项比较数量、scope/path/policy 和
DefaultsJSON，不能用空集、子集或自洽重哈希替代原要求。scoped metadata 的独立 Soul sidecar 不参与该声明比较；
完整配置身份仍绑定原 accepted attempt，最终 Workspace revision/epoch fence 不变。
原材料与逐项结果仍同一次 blocked CAS 保存；repository 读回从 archive 复算每个 File 判断，
原对象 hash 不匹配不能授单项通过，完成材料/结果 append-only。GET 只复验已有事实，不重取资源或补写裁决。
`binding_changed`、中断恢复和无裁决版本的历史均不得升级；单项 passed 只表达原观察时点的结论，
不证明持续有效、跨 owner 原子快照或全 required 合取。操作仍 blocked、currentValidity 仍独立
not_evaluated/stale；其他 owner、ready、M3 和执行授权不能从这些单项结果导出。
同版裁决还消费完整 Soul 双读：accepted 时由同一 detached 正式 Goal/DoD 与原 owner 前像纯编译
接纳 revision 增量后的 `ObservationSnapshotHash`，与原清单 snapshot 区分并保持不可变。
完整原六阶段均有明确选择、配置身份、双读内容/版本、原 request/revision 及该精确 snapshot 都匹配时，
Soul 单项可为 passed；任一阶段 unbound 的完整事实仍 blocked，不自动授 PASS/N/A；
缺失、漂移或重哈希的另一 snapshot 不能通过。它不授予 Skill/MCP 可用性，
也不证明选中 Soul 在未来持续不变。GET/recover 不重读或重编该来源。
同一原检查从确认的 TemplateSnapshot.ContextHints 与当前 exact-bound Workspace.ContextHints 独立解析
`managedWorkspaceConfigs`，不读最新 Template、不使用合并后的来源猜选、不调用 `EnsureManagedConfigs`。
system 内容须为完整默认对象；workspace preserve 允许合法覆盖对象，mergeMissing 只核对原递归补缺规则是否仍需添加键，
已有值（含 null/非对象）保留，不能用默认全相等替代。非法声明、重复目标、格式歧义、非法 JSON 或缺失/冲突均阻塞；
无声明仍需 File owner 完整回执，不以空数组自授 PASS。严格 JSON 解析保留数值精度，拒绝重复键及有损 Unicode 转换。
File 只接收 scope/path，实际原文和默认语义对象以 bytes 存入私有 `WorkspacePreparationManagedConfigEvidence`，
与原阻塞记录同 CAS 保存；公开 `ManagedConfigEvidenceRef` 为随机独立引用，不输出配置原文或内容派生摘要。
archive 的原 request/revision/configuration/time/ref 关联、0600、deep clone 和 append-only backstop 与已有材料同边界。
成功匹配仅为 `managed_configs_observed_remaining_checks`，不授 ready/M3；GET、完成重放、恢复、owner 重开不重读。
原检查的 Application 延续还按 Workspace stage binding → Workspace default → system default 精确选择 Soul，
保留原 activation 的 default lookup 规则；进程配置必须来自实际 accessor，缺失不能冒充空 default。
Soul owner 的完整 typed 原值（含 revision、provenance、全部列表与 SoulMD）、两轮实读、前后 system default、
精确封存 hash 与时间以 `PreparationSoulFacts` 传递，不消费 helper 的成功 bool 或 hash-only 命中。
Workspace 重建六阶段选择、核验完整前后 records、反查原受理 request/revision/configuration 与当前 exact seal，
仅把完整观察以 `WorkspacePreparationSoulEvidence` 随结算同 CAS 归档。公开 `SoulEvidenceRef` 是随机引用，
原文不进入普通 Workspace JSON/HTTP/SSE；0600、deep clone、reopen 和 append-only backstop 同已有私有材料。
无裁决版本的历史完整相等仍为 `blocked/souls_observed_remaining_checks`；新版本单项裁决见上文；缺配置、缺 owner、错误 DTO、漂移或不完整资料均为
`soul_observation_unavailable_not_saved`，明确本次完整材料未保存，不伪造完整证据或静默保存部分前缀。
完成重放、GET、恢复和 owner 重开不重读 Soul；CAS 未提交不补造，结算 revision/epoch 漂移只保留历史并 binding_changed。
这些观察不是资源读取至 Workspace CAS 的跨 owner 原子承诺，完整 readiness 合取仍未接通。
原请求延续另以 Skill owner 的完整 typed registry query 做两轮真实读取；每轮仍限 1024 项／8 MiB，
记录完整版本、Workspace scope、owner epoch、时间、catalog hash、全部原 records 及前后 seal hash。
Workspace 独立反查原 accepted request/revision、当前 exact seal，核验两轮原值而非 helper bool，
随 blocked 结算同 CAS 保存私有 `WorkspacePreparationSkillRegistryEvidence`；原完整两轮均保留，
两轮合计至多各自上限之和，不静默截断。公开仅随机 `SkillRegistryEvidenceRef`，
不输出原指令、私有内容摘要或选中声明；0600、deep clone、append-only 与 reopen 保持。
完整（包括真实空库）只给 `skill_registry_observed_selection_pending`，缺 owner／错误／漂移／
不完整则 `skill_registry_observation_unavailable_not_saved`，明确完整材料未保存。
GET、完成重放、恢复与 owner 重开不重新查询；结算 epoch/revision 漂移只保存历史并 binding_changed。
这不是 Registry＋SkillHub 组合目录、精确选择证明、ready 或 M3；Registry 查询本身不执行外部效果。
v1 保持只读 trace，不自动升级；legacy 缺 creation fence 拒绝。资源查询发生在两个 owner 事务之间，
不持 Workspace 锁执行跨 owner I/O；没有收费调用。
查询和服务重开不推进事务；恢复先结算原中断操作，不重跑读取。

显式检查的首次 accepted CAS 同时封存私有 `WorkspacePreparationSkillHubEffects`：原 request/payload、
revision、完整 binding、原交付目录、global/project 两个 status 目标及随机引用。仅无歧义首次回执
携带一次延续；Application 执行现有 SkillHub status（可能访问配置服务、保存检查状态），不安装、
同步或修复。原 stdout/stderr bytes、总读取字节数、完整性、退出事实、错误原文与时间随原 blocked
结算 CAS 保存，普通 Workspace JSON 和公开 HTTP 不含这些材料；file 0600、clone 和 append-only
backstop 保持。每流最多保存 1 MiB 已捕获前缀，超界明确 incomplete；错误超过 64 KiB 时不保存
前缀，仅保存原总大小/full SHA256 和明确未保存原因，不冒充完整诊断。
公开 `SkillHubEffectRef/SkillHubEffectState` 与操作 blocked、currentValidity 分轴；`recorded` 仅指
已保存历史（可含真实未启动），不是 Skill readiness。命令失败、退出未知、不完整或回执缺失为
`unresolved`，仍保留已知失败/非零退出；恢复不重发，迟到回执不覆盖已结算历史。
新检查不能绕过任何 reserved/unresolved；当前没有凭超时、空库存或换 request 清除效果 fence 的入口。
相对/非规范交付目录保留原值，但不执行 status，明确记录 directory_invalid_not_started。
成功 status 后仅消费该次原回执，不再执行 status：Application 重解析 raw inventory，在独立确定的
Codex 全局／原交付目录 `.agents/skills` 根下真实双读已安装 SKILL.md，保留两轮原 bytes、路径、
大小、计数、完整性、hash、失败与时间。严格 metadata 解析不覆盖原文。每文件 64 KiB、每 scope
每轮 8 MiB；无 source/repo fallback。Workspace 从原 raw 独立核对 membership/path、双读、metadata、
时序和界限，随同一 effects archive/原 blocked CAS 保存；不接受 parsed inventory/reobserved/PASS bool。
公开 `SkillHubInstalledState` 仅为 incomplete 或 observed_selection_pending，缺材料则不显示此字段；
它与 status 效果状态分轴，不向公开 DTO 暴露原材料。完成重放、恢复和重开不补读文件，已结算材料不可改写。
原 blocked 结算在最终 seal/revision/epoch 未漂移时，以本次完整 Registry、Soul、SkillHub 原事实
纯编译组合选择，嵌入同一 Registry archive，不新增外部效果或结算 revision。保留封存 Goal/DoD 原文、
来源引用、execution Soul allowlist、完整候选、typed 匹配分数／理由和选中原内容／provenance／status。
共享匹配策略只做纯计算；Workspace 对全部原依赖重算并逐项比较，不凭 hash 或 helper bool 授信。
公开 `SkillSelectionState=sources_incomplete|conflict|compiled_checks_pending` 与安装材料、操作状态分轴；
完整空目录仍须全部原来源完整，重复 ID 不得以 allowlist 过滤掩盖。历史编译不表示状态／freshness、
辅助资源、context 预算或资源 readiness 已通过，不授 ready/M3，不改执行 reservation 或 Runtime SM-1–4。
GET、完成重放、恢复和 owner 重开不重编译缺失材料、不补读来源、不改写已结算选择。
同一私有结算还复用既有纯 `SealRuntimeSkillContexts` 对完整选中材料编译 context 预算预览；
不再选择、不访问 owner/provider，不构造 Run。完整原文及 status 保持不变，单独保存固定 5 项、
单项正文 8 KiB／合计 24 KiB、元数据 16 KiB 的有界投影、精确计数与显式裁剪记录。
失败原诊断按 64 KiB 界限完整保存或明确未保存及原大小/hash；公开仅
`SkillContextState=compiled_checks_pending|bounded_checks_pending|rejected`，不输出正文或私有诊断。
Workspace 从原选择独立重编译全部投影和预算并逐项比较；同 envelope hash 不能替代此校验。
缺原选择不能授预算结果；旧历史缺该材料不自动补写。预览不是 Runtime context 封存、freshness 或资源 PASS。

同次首次延续在真实安装指令双读之间，按原 frontmatter 读取显式本地辅助文件，
`workorch-skillhub-reference-observation/v1` 原材料嵌入 Installed，随原 effects archive 保存；
单 scope 256 项、每文件 64 KiB、每轮 1 MiB，保留两轮原 bytes/完整性/失败/诊断和时间。
原安装根及单技能目录独立固定，禁止越界、symlink、URL、非规范路径及 repo/source/cwd fallback。
Workspace 从完整原选择和 effects 原值重新编译 `workorch-preparation-skill-reference-compilation/v1`，
选中 SkillHub 引用精确绑定 scope/sourceSkillID/ref/path；Registry 无文件根的声明为 source_unresolved，
不借同名技能补源。公开独立 `SkillReferenceState` 仅为 observed_checks_pending、sources_incomplete、
source_unresolved 或 unavailable，要求原 blocked/compiled_checks_pending 及全部选择来源；
不输出文件路径/正文/hash，不把 modified/conflict 改为通过，不授 Skill PASS、持续有效性或 M3。
nil 字段省略以保持旧 archive hash；GET/replay/recover/reopen 不补读，完成历史 append-only。

同次私有结算另从完整原选择与 effects 编译 `workorch-preparation-skill-status-compilation/v1`，
保存每个选中来源的原 ObservedStatus 及有限诊断，独立重算全部值；不凭摘要或公开 summary 授信。
`SkillStatusState` 只允许 observed_freshness_pending、status_blocked、version_conflict 或 sources_incomplete，
绑定原 blocked 及完整选择。只有精确 synced 能进入待 freshness 状态；modified/conflict/空/未知及
大小写空白变体均受阻；frontmatter 与非空 status local_version 不同明确冲突，原仓库版本不能代替。
缺一侧版本仍不能证明一致，Registry 不造 SkillHub 状态。所有分类均不授 PASS/currentness/daemon 一致性/M3。
原 status 与完整原安装 bytes 不变，完成重放、恢复和 owner 重开不补写；删除分类并重算 hash 也不得改历史。

#### 准备 creation fence 与私有 reservation 基础

新 Workspace 的唯一 creation dispatcher 在首次 Create 中持久化随机 `CreationEpoch`；
authoring 原请求重放、部分 metadata/sidecar 创建恢复均保留同一个标识。
`PreparationFence == nil` 明确表示 legacy，不因查询、空 Run 列表、普通 Update 或 CAS 自动补造标识。
memory/file/scoped persistence 保全该 owner-private 字段，拒绝错版本、错 scope、非法 raw 及标识抹除；
普通 metadata Update 不可登记 reservation，必须经过 owner-private reducer/CAS。

私有 `workspaceFreshIntentReserved` 只接收已校验 fresh typed input 的完整规范化身份，
绑定 scope、request、kind、StartupInputHash 和完整 bootstrap，不读取配置或凭据。
相同 pending 身份先于可变 revision/admission 识别且不再 CAS；不同身份不能绕过未决项。
只有 CAS 明确成功的本次调用返回新登记标记；写后丢回复、取消、冲突、重开或重复均不能取得第二个标记。
该标记不是执行授权。reservation 推进持久 revision/epoch，已有准备记录只能投影 stale，不能清除原历史。

`workspaceFreshIntentSettlementFact` 只消费独立只读查询得到的精确持久原回执，反查完整原 Workspace
payload 和独立 Run payload 身份；二者在首次 reservation 同时封存，持久行核对 Run hash 与 receipt 一致。
普通 Restart 成功响应、replay 投影和缺失回执不能结算。
私有 reducer/CAS 将原 pending 追加为不可变 `Settled` 历史，保留原接纳结果及原时间，不推进 epoch；
同一原回执重放零写，异回执冲突。后续新 reservation 单独推进 epoch，不能改写、淘汰或复用原 request。
历史上限 128，满时只读/原身份重放仍可用，新登记阻塞；普通 Update 不能结算。
`reconcileWorkspaceFreshIntent` 先从本地原登记/历史解析身份，再读取 Run owner；未知或查询失败
保持 pending，不清除、不 forward。可变 Definition/admission 漂移不阻断原身份对账；跨 owner 查询
不持 Workspace 锁，回写前 fresh-load/reducer/CAS 重新核对原登记。它不重建旧 bootstrap。

生产 execution ingress 在 admission 之后、Run forward 之前调用私有 reservation dispatcher；仅 CAS 明确成功的
首次调用可以 forward，重复、写后丢回复或 unknown 都不能重发。已登记身份先于当前 Goal/DoD/admission 解析；
同键异完整 typed payload 冲突。Run 返回正常响应后仍由独立持久原回执查询结算，不能凭响应清 pending。
`/workspace/command/recover-execution-intent` 是 Application 显式执行 POST 的窄前置命令：只接收 scope/request/restart，
从本 owner 不可变登记取得完整原 payload 身份，查询/结算原结果；无登记不写、不 forward，亦不授转发许可。
它在当前 bootstrap 编译前使用，不从当前规格重建旧输入。新请求遇 pending 返回本次 request 与原 pending 的独立身份。
未知结果返回 `fresh_intent_unresolved/fresh_intent_result_unknown`，不是已接纳或确定拒绝。
缺 creation fence 的 Workspace 新 Restart 明确返回 `preparation_creation_fence_missing`，不自动迁移；Resume 不补标记、保持原义，
但不得复用已登记 fresh RequestID。当前不签发 readiness/M3，不新增 SM-1～SM-4 authority。
独立子进程覆盖文件 owner 的 pending/settled 冷重载及原字节不变；这不是生产服务
stop/start、Run 转发或生产浏览器验收。

`queryRunExecutionIntentReceipt` 通过 Run-owned `/run/workspace-execution/receipt/query` concrete
合同读取精确原响应。发送前校验 scope/request/payload 身份并绑定父 context 和最多两秒的只读预算；
缺响应、owner 错误、类型错误或原身份漂移均失败，不转成 unknown、不重试或改走 Restart replay。
只有真实合法结果的 `Receipt=nil` 才表示 unknown；返回 detached 原值。sender 本身不结算、清除
pending、不授予第二次 forward，也不从当前 Definition 重构旧请求。Run owner 原回执读取和
Workspace 私有结算及生产 ingress 已有真实 Run owner EventHub 配对测试；这不是生产服务或浏览器验收。

### 合同变更待审输入

`PrepareDefinitionChangeCommand/Result` 只接受 Application source；`QueryDefinitionChangeCommand/Result` 另允许 exact `blocks/decision` 只读反证最终材料。准备命令仅传 Ticket/version/hash 与 Workspace CAS 身份；Workspace 在自身 mutation lock 外读取 Decision 原持久最新版本和 Run inventory，再在原规格 private dispatcher 内核对输入、模板和定义基线并 CAS。读取是原版本观察，不保证与并发 Decision 修订或启动构成跨 owner 原子事务，不能用于批准或激活。

查询从持久原文账本重建 `DefinitionChangeReviewedIdentity`：当前唯一完整派生、唯一 strict mapping 提案、六份独立完整 PASS 和逐义务验证声明全部满足时才返回身份。未决、失败、漏审、歧义或陈旧输入不产生已审身份；detached DTO 重编同一身份并核对所有字段，但不能替代 owner 对原 mapping bytes 的重验。ID/hash 绑定 Workspace、Ticket、准备请求、原基线、候选输入、模板、资源快照、派生 permit、proposal 与六审规格；不绑定随后 Decision 追加版，避免循环身份。它是持久审查材料身份，不是当前资源／模板 readiness、批准、confirmation journal 或启动回执；查询不产生任何持久写入。

`DefinitionChangeCandidate` 保存既有 `DefinitionAuthoringRecord` 的独立待审输入，不新增 confirmation journal、批准副本或正式定义权威。当前仅映射 exact `authoring-goal` 修改及 `derived-dod:user:NNNNNN` 用户验收的增删改；原文必须匹配当前输入，模板／生成判据和约束等不能映射的变更明确拒绝，不猜测语义。原当前 record、Goal／DoD、startup、admission 和 materialized journal 均保持不变；全部旧 permit、审查、诊断及预算历史继承，新 input revision/hash 不能借用旧六审确认。原始候选记录 detached，包含诊断时沿原 0600 原子发布与读取门禁保护。

当前只允许一个 exact preparation；同 RequestID/version/hash 重放恢复原回执，异请求覆盖或迟到旧提案拒绝。原 ledger 存在尚待结算的 permit 时拒绝准备，不冻结其 completion 或清除 unknown；结算后可重新提出同一准备请求，既有消费预算与结果原样继承。候选派生／mapping／六审复用原 reserve/complete 合同与唯一 private CAS dispatcher；新 reserve 显式绑定候选 InputHash，Workspace 再读取 Decision 最新提案和零 Run inventory，并保留模板、预算、资源 owner 核对与 CAS 栅栏。观察不是跨 owner 原子事务，不能作为批准或激活事实。已接纳 permit 按原持久身份结算，之后提案漂移不会丢弃已知结果；响应丢失只查询原 ledger，不重发 provider。普通原输入不再接纳新工作，普通确认／准入拒绝改变基线。

原 authoring 查询空 InputHash 仍返回原定义来源；显式 InputHash 只返回选中 record 和当前模板／目录观察，不复制原 confirmation/admission。File 资源 scope 使用同一选中输入核对 refs，两端沿原具体合同；成功审计、失败与准备诊断受限查询覆盖候选 permit，原文及继承副本完整反证，跨历史与候选诊断总预算不重开。候选修订／替换、可信主体 exact approve、原 journal 应用及 Run 交接尚未开放；六审成功不代表 approved、M3 完成或可启动。准备不调用 provider，审查仅调用有界 provider transaction，均不创建 Run、不改变 SM-1～SM-4。Application 提供实际 typed sender 和查询，但没有 HTTP/Web/CLI route。

### 阶段生命周期

`BeginShutdown` 关闭新创建、修改、确认、准入、执行、删除及后台调度，并取消 projection Timer 的后续触发；不等待、不退订、不清除 repository、cancel 或订阅账本。必要只读查询、既有 permit 的规格／派生完成、诊断交付、sealed terminal delivery 与 Run 事实接收保留原 source、scope、hash、预算及 CAS 校验，不授予新事务权限。Run 通知仍只是提示，关闭后不启动新的 inventory；普通通知不能释放 slot。

每项普通／matched 订阅仅在 checked 注册成功后加入账本。部分 Start 失败保留已获得的订阅与调度句柄，不在原实例重复注册。同步 handler 持生命周期票直到真实返回，包括锁等待与嵌套 Send；一次性 AsyncFunction 在提交前取票，accepted 排队任务到 callback 真实结束才释放，提交拒绝才由提交方释放。Timer callback 先检查 fresh admission，closing 后迟到的 callback 不访问已释放依赖。context 取消不是 handler 或 accepted task 完成事实。

`Quiesce` 按调用预算等待真实在途工作；超时保留全部未释放资源以便新预算重试。最终 `Teardown` 封闭所有入站，再次排空并逆序 checked 退订，逐成功移除账本，全部成功后才清除依赖。Module 的 Stop 错误保留原 cause 和 Biz；无返回值 plugin Teardown 通过 framework guard 暴露失败，不吞错误。Configure 不覆盖尚未释放的实例。

上述 owner-local 排空不替代全进程的共享 BackgroundRoutine Shutdown、Hub Drain 与最后的 Initiator 释放屏障；Timer 取消不证明共享队列排空。本合同不新增或改变 Definition、删除 journal、slot 及 SM-1～SM-4 的业务转换或权威。

## 不变量与非职责

- 草稿创建的 `DefinitionAuthoringCreation.DirectoryPrepared` 默认为 false（包括旧数据），不从目录存在或 GET draft 推断。
  只有 Application 源的精确原创建目录完成事实可由 owner CAS 记录；重复事实不增 revision，原身份、路径或 hash 漂移拒绝。
  Query 返回创建身份与历史阶段，仅当当前 revision 1 的完整输入、标题及目录能重组原 creation hash 时提供 replay；
  GET 不创建目录、不编译、不确认、不激活。该历史阶段不改变 Workspace lifecycle 或 SM-1～SM-4，也不是当前 readiness。

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

### 跨源 execution 证明映射

规格可选 `ExecutionProofMappings` 随完整 proposal hash、六审及 exact confirmation 封存，只有声明
`runtime/cross-source-execution-proof/v1` 的 execution work/repair rule 可以消费。consumer/source 必须是不同原 DoD
candidate refs；缺失、重复、未知、未声明或未重新评审的映射阻断准入。库存完整保留原文与映射，不授予 proof。
详见 [跨源证明合同](../../../../docs/00-core/runtime/cross-source-execution-proof.md)。wire 扩展可省略；存在时每个字段
仍必须精确且非 null，旧基础字段的必填门禁不变。

独立 `EvidencePurposeMappings` 使用 `runtime/cross-cwi-evidence-purpose/v1`，封存原历史行为 DoD 与唯一普通当前
验收 DoD candidate ref；不借用 execution work/repair rule。完整映射进入相同 proposal／六审／confirmation／startup
身份，原文不改写，clone／inventory 不共享来源存储；未知、自映射、重复历史来源、映射链及无用途资格／普通验收的
声明拒绝。`evidence_purpose_mapping_test.go` 证明 pre-seal 原图、exact admission 投影与身份替换拒绝，不证明 Runtime
延期终验已执行；mapped bootstrap／seed 的 executable availability 独立保持 fail-closed。

Profile deliverable 来源另使用 `runtime/profile-deliverable-evidence-purpose/v2`：原 Profile ID、唯一 required deliverable ID、单 Profile 完整快照 hash 和原 Description 全文随六审／用户确认封存，不将其伪装成原 DoD。接受端仍为唯一原 DoD，运行期独立反查原快照；完整字段进入 content/admission/startup hash，v1 不接受扩展。普通验收及双用途依赖闭包不放宽。

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
