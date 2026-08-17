# WorkOrch Unified Runtime Design Contract

Status: active

Type: core-contract

Last Verified: 2026-08-16

本文定义 WorkOrch 唯一的、领域中立的 Runtime 设计合同。生产主链仅为
`DriveRuntime`、reducer 与 sealed dispatch；它治理事实、约束、证据、恢复与终态，
不理解软件开发、文件名、语言、测试框架或任何业务领域实现。

实现细节的权威顺序如下：

1. 本文的状态机、权威与禁止迁移；
2. [Runtime request/cache contract](runtime-request-cache-note.md)；
3. `internal/modules/kernel/orchestrator/readme.md` 与
   `internal/pkg/workorchruntimecontract`；
4. [当前 Runtime live 计划](../70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md)
   只定义未关部署验收，不得覆盖本文状态机合同。

已归档计划、历史 live 记录与过程日志只能帮助定位回归夹具，不能成为新的运行时约束。

## 核心中立与职责分层

Runtime core 只理解 workspace、run、definition、focus、requirement、prerequisite、candidate、
sealed dispatch、operation、effect、evidence、assessment、governance、result、recovery 和
projection。它不推断“应创建哪个文件”“何种实现正确”或“哪个工具最适合某一领域”。

| 层 | 职责 | 不得承担 |
| --- | --- | --- |
| Runtime core | facts、reducer、authority、sealed dispatch、evidence、governance wait、terminal | 领域策略、owner 资源猜测、provider 私有逻辑 |
| profile / capability | 交付物类型、可用能力、owner admission、validation/evidence policy | 修改 Runtime authority 或越权关闭 coverage |
| LLM | 在 sealed required output 内提出 candidate | 选择 Runtime action、扩展授权、裁决证据或终态 |
| TUI/Web/projection | activation、审计和展示 | 重建 authority、隐式续跑或补偿状态 |

`internal/pkg/workorchruntimecontract` 仅保存 DTO、合同定义以及无状态的 normalize、validate、
clone、hash helper。它不得拥有状态迁移、CAS、EventHub、filesystem/network I/O 或 retry 决策。

### Current development baseline、owner identity 与 model capability token 权威

当前 Runtime aggregate 只维护一个无版本号的开发基线。开发调试阶段不维护历史 aggregate migration/reducer：合同结构发生
不兼容变化时，代码、fixture 与测试直接更新为当前基线，现有运行数据必须清理后重新创建。版本号不得作为
补字段、重解释旧 authority 或绕过 owner transition 的工具。

唯一 output-token 权威是 LLM owner 针对
已封存 provider/model selection 与 capability/configuration revision 返回并持久化的 typed `ProviderCeiling`；
Application、Web、TUI 与 Orchestrator 均不得维护固定值、override 或 model-name 分支。

每个 semantic-plan 或 operations request 的唯一已解析预算是先于 provider call 持久化的
`RuntimeExpectedDocumentContract.MaxOutputTokens`，其值必须精确等于当前
`ProviderCeiling.EffectiveMaxOutputTokens`。EventHub command、LLM provider request、provider-facing
`max_output_tokens` 与 request audit 必须复制同一个 sealed 值；任何不等值都在 provider submission 前形成 typed
context-integrity/contract failure。recovery 复用同一 ExpectedDocument，不重读配置、不提高 token 值，也不拼接 partial
output。

fresh activation 把不可变 `DefinitionFocuses` 与 SM-2 封存的 generation-specific
`Focus/RemainingFocuses/CompletedFocuses` 分离。前者只包含 CWI、slice、required evidence/assessment、声明式
metric/artifact obligation、owner-fact postcondition 及可选 profile-declared selector identity，不得携带 resolved target、operation kind/variant、recipe/check 或 binding；后者可随 owner
resource revision 退休、重开和重新封存。任何 semantic document admission、owner feasibility batch 与 final seal 都只能用
`DefinitionFocuses` 检查 source declaration，禁止把上一 generation 的 accepted operation kind 当成下一 generation 的声明。

当前基线同时封存 capability-owner policy revision、equivalent-authority input、binding contradiction 与 opaque
lifecycle fence identity。File semantic/operation feasibility 必须消费同一 owner policy；Orchestrator 只有在 Definition、
focus/binding、evidence、capability/catalog、prerequisite freshness 或 owner observation lineage 改变时才能创建新
AuthoritySnapshot；Run 只能比较随 opaque Runtime record 持久化的 lifecycle fence，不得解码 SM-2/3/4 内容选择 SM-1
迁移。

`OwnerPolicy` 只标识 observation 的 owner、policy revision 与 catalog snapshot，绝不隐式表示 binding contradiction。
只有 capability owner 显式返回 owner-neutral `sealed_target_authorization_contradicted` constraint impact，且
Orchestrator 将它精确关联到 persisted candidate attempt/operation、current CWI binding、reconciliation binding、owner
catalog、authority transaction、target set 与 observed resource revision 后，SM-2 才可退休该 binding。binding 自身封存的
semantic manifest 可以位于 active ledger，也可以位于 immutable generation history；不得要求它等于另一个 CWI 的最新
global manifest。`observation_unknown`、无效/无效应 patch、replay-proof unavailable 等 candidate-local observation 即使携带
OwnerPolicy，也只能留在 SM-3 invalidated-attempt/no-progress 路径，不能重开 CWI 或形成 binding contradiction。

任何非当前 aggregate 不得原地迁移、忽略旧字段、重解释 authority 或通过 Restart 包装为当前数据。Resume/drive
只能返回 `fresh_activation_required`，且不得产生 provider/capability work；Restart 由 Run owner 返回
`data_reset_required`，不得创建悬挂的 replacement intent。清理开发数据后，新 Run 从当前 bootstrap 开始，
不继承旧 request、candidate、CWI credit 或 recovery budget。

跨 owner I/O 只能通过 EventHub 的具体 typed command/result。禁止跨 owner service/repository、
`map[string]any`、无约束 `any`、JSON/reflect bridge 或通用 envelope。

Workspace scope 是每次事件执行上下文的一部分，而不是组件实例的“当前 workspace”状态。任何带
`WorkspaceID` 的入站 Runtime 命令必须先将该 ID 绑定到 EventHub context；已有 context 只能匹配，
不得被静默覆盖为另一 workspace。Biz、Service、Module、全局变量、共享 cache 和后台任务不得保存
未经该 scope 约束的当前 WorkspaceID。持久化模型、sealed contract 与 typed command 中的 ID 仅作为资源
归属和审计 identity；下游 repository、EventHub 和 background work 必须继承已绑定 context。
旧的 context 构造 helper 只能为未 scoped 的根 context 首次建立 scope，不能覆盖已有 scope；正式命令入口
必须使用会返回 conflict 的 checked binder，不能把该 helper 当成跨 workspace 切换机制。

### 单一 decision 与双层反馈

SM-2 的 canonical decision 只由 `ConvergenceState` 表达。执行动作文案、candidate mode 和
required-output kind 均从该 state 与具体 output constraints 纯派生，不得作为独立 persisted authority、CAS、
transaction 或跨 owner lifecycle 字段。`RuntimeDecision` 只保存 canonical state 与 terminal code/summary；SM-4
seal 跨 owner delivery fact 时才生成 typed `TerminalOutcome{Kind, Code, Summary}`。`Kind` 只有
`completed | failed | cancelled | superseded`；前两者来自 sealed SM-2 terminal，后两者只能来自 SM-1 typed
administrative intent/fact。seal 后该 outcome 是 delivery identity；独立 administrative kind 和 Orchestrator action
均不得进入 terminal delivery identity。

所有 request、candidate、feasibility、operation、drive、delivery 反馈采用双层模型：状态机只读取当前结果域的少量
stable factual category；同时保留产生者提供的完整、安全 original diagnostic，包括 owner、原 class/code/message、
signature、field violations、target/revision 与允许的 details ref/hash。stable category 只能新增，不能覆盖原始
字段；message/code 不得选择 action、retry、target、ticket 或 terminal。workspace-scoped feedback 必须携带
workspace/run 及适用的 authority/dispatch/operation identity；只有 producer 确实没有 diagnostic 的 accepted fact
才允许 original diagnostic 为空。

“原始反馈保真”不覆盖 credential、endpoint、raw prompt、raw provider body、未脱敏文件内容等禁止持久化数据。
这些内容仍只能按 request/cache 合同记录允许的 hash、byte count、disposition 与 redacted/omitted 标记。
EventHub 继续传 owner-defined concrete Result；双层 feedback 是可嵌入的纯值合同，不是通用 Envelope。

Coverage 字段必须按同一分类解释：profile/template 与 Goal/DoD 形成的 Definition seed 不是 credit authority；
只有 SM-2 根据受约束 semantic-plan document 与 capability-owner feasibility 封存的 `CoverageEvidenceBinding` 才是
canonical declaration。activation 不接受预制 sealed binding。manifest/dispatch 的 `BindingRef` 是 sealed
authorization identity；capability receipt 的
effect/target/check/metric/artifact 是 owner fact；`EvidenceFact` 中的 CWI credit、coverage ledger 与 terminal gate
只能是 SM-2 从前述三类事实派生的 canonical authority。candidate 自报 CWI、assessment claim、`PassedChecks`、
exit code、summary 与 projection 均属于 context/original diagnostic，不能成为 CWI credit 的写入来源。

## Runtime 生产主链

```text
typed RuntimeBootstrap
  -> Run owner receives activation / continuation intent
  -> SM-1 owner-local lifecycle transition + RunStateChanged fact
  -> SM-4 load/singleflight drive
  -> SM-2 semantic contract required
  -> SM-3 semantic-plan document -> owner feasibility -> SM-2 binding seal
  -> SM-2 reducer / operation authority transaction
  -> acquire prerequisite | request candidate
  -> SM-3 complete candidate / seal / execute / receipt / evidence
  -> SM-2 reduce / focus advance / governance / terminal
  -> SM-4 terminal delivery ledger
  -> Result owner -> Workspace owner -> Run owner owner-local delivery transitions
```

1. Application 只构造 typed `RuntimeBootstrap` 并向 Workspace/Run owner 提交 scoped activation / continuation intent；
   不指定 Run、Workspace 或 Result 的目标状态。用户明确 restart 时，Run owner 先在 predecessor 上持久化 immutable
   replacement intent；SM-4 再按 `Result -> Workspace -> Run` 投递 administrative terminal fact。Workspace 仅在
   `ActiveRunID` 精确匹配时通过内部事件释放 slot；Run owner 只有收到该顺序的最终 Run delivery 后，才能通过内部事件
   幂等创建并激活 current-baseline successor。Application、Workspace 和 Orchestrator 均不得提前创建 successor。
2. Run owner 持久化 Run lifecycle、definition revision、lineage 与 Runtime aggregate；其中 aggregate 是
   Orchestrator 的不透明持久化负载。其语义 CAS 只比较 `RuntimeState.StateRevision`，不得以完整 Run 模型
   revision（SM-1/lifecycle 或其他 Run 字段）阻断 SM-2/SM-3/SM-4 的保存。
3. Orchestrator 是 reducer、prerequisite/freshness、candidate/dispatch、evidence、governance
   resolution 与 terminal decision 的唯一权威。
4. capability owner 只执行自己的 typed prerequisite/operation 并返回 receipt；Result、Workspace、Run
   owner 各自消费 terminal delivery fact 并只迁移自己的资源状态；projection/eventstream 仅为派生读模型。
5. 重启只从已持久化 aggregate 继续 drive；不得从进程内缓存、UI 状态或历史文本重建 authority。

### CWI semantic contract 与 reconciliation

Runtime activation 必须封存完整 Definition seed：Goal/DoD identity 与正文、definition revision、selected
template/profile policy、稳定 coverage item identity、presentation slice 以及 capability/policy refs。fresh restart
不要求普通用户预先填写 workspace-specific target/recipe/check；缺 exact binding 的有效 seed 进入 persisted
`semantic_contract_required` authority，而不是在 application 中终止。

SM-3 使用同一 request-session、complete-document evaluator 与 attempt-0/1 预算请求一个无副作用
`semantic_plan` document。它只能逐 CWI 提出 owner、operation/effect、三种 target policy 之一、target/selector、
recipe/check 与适用 metric/artifact；不能携带 Runtime action、coverage state、credit、terminal、retry、ticket 或
owner result。semantic provider request 前，SM-2/SM-4 必须已通过 typed prerequisite 获取并持久化 canonical owner
admissibility catalog；manifest/hash/schema/evaluator 只暴露该 snapshot/revision 中的 operation、target、selector、
constraint 与当前适用的 recipe vocabulary。声明了 workspace-sensitive validation recipe 时，SM-2 必须先取得 File
owner 的 manifest 及其 exact freshness observation，再通过 concrete typed request 交给 recipe capability owner；后者只
返回对该 manifest 当前适用的 recipe，并在自己的 catalog observation 中复制 exact File workspace-tree revision。
Orchestrator 不解释 manifest path 或 recipe。没有 confirmed recipe 的 `validation/managed_session` 不得进入
provider-visible allowed operation set。SM-3 接受 document 只形成 internal fact，不创建 `SealedDispatch`。

owner operation-admissibility catalog 必须把“可调用 operation”与“当前可作为验证证明的 surface”分开。每个
`RuntimeValidationSurface` 由对应 capability owner 在一次一致 snapshot 中声明 `ready | prerequisite_required |
unavailable`、`direct | auxiliary | not_declared` proof applicability、input schema hash、owner-sealed input binding 或
prerequisite lineage、check refs、effect 与 replay class；只有已经具有完整 input binding、non-durable、safe-to-replay，
并由 owner 显式声明为 `direct` 的 `ready` surface 才能进入 `ValidationRecipeRefs`。`auxiliary`/`not_declared`、
`prerequisite_required`/`unavailable` surface 仍随 aggregate 和 audit 持久化，但不得
进入 provider schema、C3 ownerCatalog、correction allowed values 或 behavior probe。catalog 分类只读取 owner 本地/typed
registry fact，不调用 provider/MCP endpoint、不创建 candidate/dispatch/receipt，也不触发状态转换；Orchestrator 不能
根据 schema、tool 名称、描述、category 或 DoD 自行补 input binding、推断 direct proof。该 catalog disposition 不是
semantic-feasibility observation，不消耗 semantic attempt 或 prerequisite-refresh budget；若仍有合法 `ready + direct`
alternative，SM-3 只在该交集中规划，若无
可满足 required proof 的 alternative，则沿既有 bounded semantic-contract fail-closed 主线处理。

profile 可以在 immutable DefinitionFocus 中声明中立的 `RuntimeOwnerFactRequirement`，但不能提交 owner 的目标状态或
生成第二份 capability authority。当前支持的 `selectable_validation_surface` 只要求指定 capability owner 的当前 typed
admissibility catalog 同时包含声明 operation kind、`ready + direct` surface 和同一 `ValidationRecipeRefs` identity。
Workorchd 只把 profile 声明编译进 bootstrap/DefinitionFocus；capability owner 只发布 catalog；只有 Orchestrator SM-2
可在 assessment commit 中读取该事实并决定 CWI credit。LLM assessment、operation 成功、文件变化或历史 recipe 均不能
代替当前 owner fact。

同一 proposal 的 operation kind、concrete variant 与 target policy 必须来自同一条 owner catalog fact；catalog
已列出的现有目标不得使用仅允许 prospective target 的 create-only fact。若相同 evidence kind 与 target policy
在同一 owner snapshot 中存在合法 replacement kind/variant，strict evaluator 与同-manifest correction 只能返回该
sealed alternative，不能解析 owner diagnostic 文本、扩宽 catalog，或把 existing target 改写成 prospective target。
target content/repository revision 不是 provider 字段；它由 capability owner 的 snapshot/preflight 观测并在 SM-3
candidate manifest/dispatch 前绑定。该约束只改变 SM-2/SM-3 的 admissible value intersection，不新增状态机或 owner。

canonical capability owner 对 proposal 返回无副作用 feasibility observation。SM-2 校验 current
workspace/run/definition/seed identity、完整 CWI 覆盖、template/profile 与 owner catalog 交集以及完整 owner batch 后，
通过 private internal event 原子封存 `CoverageEvidenceBinding`、`RuntimeCWIReconciliationContract`、binding set/hash
与 lineage。`internal/pkg/workorchruntimecontract` 只负责 DTO、规范化、校验、稳定 hash 和纯 evaluator helper；
它不读取工作区、不调用 EventHub，也不决定 retry、terminal、coverage 或 lifecycle。

所有 fresh restart 都经过同一 persisted semantic planning、owner feasibility 与 SM-2 seal 主线；activation 不得直接
携带 sealed semantic contract，也不得在普通 Workspace 创建/编辑流程暴露 Runtime JSON 必填项。

每次运行前都先由 Orchestrator SM-4 投递一次 reconciliation request，再由 SM-2 消费 capability owner 的
typed observation/receipt：

- `/run resume` 的 scope 固定为 `current_run`，只复核当前非终态 Run 已封存的 Definition、selected CWI 与
  binding；同一 request/generation 重复投递必须幂等，新的复核 generation 必须保留旧 batch/receipt lineage。
- `/run restart` 的 scope 固定为 `workspace_full`，新 Run 不继承 predecessor coverage；即使 owner observation
  显示已有文件、Git revision 或 endpoint readiness，也必须由新 Run 的 matching receipt 重新取得 credit。
- observation 只能让 SM-2 保留已存在的 credit，或通过 `reopened` lineage 重开 CWI；它不能直接新增 credit。
- reconciliation 重开后，同一 current semantic manifest 已精确重绑、仍位于 active/remaining focus queue，且所有声明
  prerequisite 仍 settled+satisfied 的 CWI 可以保留在 executable lineage 中；较早的 effect-fenced current focus 不会因此
  被越过。该 bound row 不重新获得 planning eligibility，duplicate/restart re-drive 只由 SM-2 过滤后保持零 CAS；stale
  manifest/binding、completed row 或 prerequisite reopen 必须拒绝。
- 一个 binding 的 owner receipt 与 observation 必须一对一且完整；缺失、重复、跨 workspace/run、binding/hash 或
  generation 不匹配均为合同错误。owner 的 exit 0、summary、通用 test、文件/Git 存在和 LLM claim 都不是 semantic
  proof。
- File 只返回 owner-grounded target/artifact/selector fact；Shell 必须执行封存 recipe 并报告 CWI-specific checks，
  没有 required checks 时为 `unverifiable`；Git revision 不等于 semantic proof；MCP catalog/tool surface readiness
  没有可执行 behavior probe 时也只能为 `unverifiable`。

Semantic planning 与 reconciliation 都不是第五个状态机：request/attempt/feasibility/seal/batch/history/lineage
均属于 Orchestrator SM-2/SM-3/SM-4 aggregate；Workspace、
Run、Result 只能通过各自 owner-local internal event 迁移自己的状态。旧 aggregate/schema 缺少该合同时不得推断或
旧状态推导，必须返回 `fresh_activation_required`，等待用户显式 restart。

### 跨 owner 生命周期规则

任何正式状态只由唯一 owner 的 Biz 内部 reducer 修改并持久化。跨 owner 消息只能携带 immutable intent、
已持久化 fact 或 delivery acknowledgement；不得携带“把接收方状态设为 X”的命令。接收 owner 必须先校验
 workspace/run scope、causation、definition/evidence revision 与 sealed identity，再转为 owner-local event。

Orchestrator 对 SM-2/SM-3/SM-4 的 owner-local event 使用唯一内部 dispatch：外部结果不得直接选择具体
aggregate transition；内部 dispatcher 校验后调用 reducer/controller，并且只能通过 transition coordinator
执行一次 CAS。internal event 不是新的跨 owner EventHub 通用 envelope。

- SM-1 Run lifecycle 只由 `blocks/run` 迁移；Workspace active-run/state projection 只由
  `blocks/workspace` 迁移；Result lifecycle 只由 `blocks/result` 迁移。
- Orchestrator 只拥有 SM-2、SM-3、SM-4。SM-4 只记录 delivery request/acknowledgement 与重试，
  不直接写 Run、Workspace 或 Result。
- terminal delivery 的稳定顺序是 `Result -> Workspace -> Run`。任一步失败仅为 delivery pending；
  不得改写已 sealed 的 SM-2 terminal，亦不得以投影失败把 Run 反向置为 failed。

## 规范化候选合同

Runtime 不要求穷举所有模型输出组合。它要求把动态 owner 事实先编译成一份有限、可验证的
`RuntimeCandidateContractManifest`，此后 renderer、decoder、candidate evaluator、owner feasibility 和
fixture 都只能消费这份封存合同，不能再次解释原始 profile、目录、错误文本或临时上下文。

每个 manifest 由零到多个 `SealedOperationAuthorization` 组成。该名称表示 manifest 内的一条纯值授权记录，
不是新的状态机或 owner；实现可在现有 manifest DTO 内表达。每条授权至少包含：

- canonical operation kind、owner module、parameter/variant contract；
- accepted/required effect scope、dependency 与封存的 `CoverageEvidenceBinding` 引用；每个可关闭
  CWI 的 binding 必须精确绑定 CWI、proof shape、target/selector、validation recipe/check（适用时的 metric/artifact）
  与来源 declaration/snapshot，不能用 candidate 自报 CWI 代替；
- 唯一 `TargetPolicy` 以及该策略所需的 exact refs、bounded owner scope 或 owner selector；
- 来源 prerequisite/owner receipt 的 snapshot/revision 引用；
- manifest hash 与 operation-contract revision。

目标策略只允许三种，禁止为 live 症状增加第四种临时分支：

| TargetPolicy | 含义 | 必须拒绝 |
| --- | --- | --- |
| `exact_target` | operation 的全部 target 必须属于封存的精确 target set | 空 target、越界 target、运行时猜测 target |
| `existing_owner_scope` | operation 必须提供 concrete target，且每个 target 属于 owner 封存的既有资源 scope | 新建 target、截断/不完整 inventory、只命中部分 target |
| `prospective_owner_target` | candidate 必须提交 concrete target，并命中 owner 封存的 create selector/prospective constraint | 非 owner 允许的新建、无 target、绕过 owner feasibility |

`TargetPolicy` 是 owner-neutral 的纯合同词汇。File、Git、Shell、MCP 或业务 profile 的具体语义仍由对应
owner/profile 提供；Orchestrator 只编译和验证 typed receipt，不按文件名、命令或领域文本推断策略。

owner inventory 与 provider context 是两个不同合同：

- owner inventory 必须是完整的有界授权事实，或是能被 owner 独立验证的封闭 selector；截断列表不得封存为
  scope；
- candidate context 可以从 manifest 生成有损展示，但不得删除执行所需的 kind、parameter、target policy、
  binding reference 或 effect 约束；
- `OwnerInventoryBudget`、`CandidateContextBudget`、`RequestRecoveryBudget` 必须使用不同 DTO/字段和校验入口，
  禁止因数值相同而互用；
- inventory 超限且 owner 无法提供封闭 selector 时，在 provider 前形成 typed prerequisite failure。提高固定
  常量不能替代该产品语义。

每个 required CWI 必须在任何 operation candidate/dispatch 前，由 SM-2 从 current Definition seed、受约束
semantic-plan document 与 matching owner feasibility 编译出 deterministic、hashable 的
`CoverageEvidenceBinding`，并封存其 binding set/hash。binding 是 DTO/合同，不是新的状态机：它至少含
`BindingID`、exact CWI ID、definition/focus identity、required evidence kind/effect scope、允许 operation
kind/variant、exact target 或 sealed selector、recipe/check、适用 metric/artifact 与 declaration/snapshot lineage。
当前 Runtime 对 validation proof 采用一 focus 一 CWI 的保守形状；共享 recipe 只有在每个 CWI 具有独立
check/target 并编译为独立 focus/binding 时才允许。focus 级泛化 recipe/check 不得由一个 owner receipt 扇出关闭
多个异构 CWI。缺 binding、binding 不能被 owner 独立验证或 binding set 与 current definition 不一致时，
SM-2 不得进入 operation provider/dispatch；它只能继续有界 semantic planning/owner prerequisite，或以完整 typed
diagnostic 终态化。该失败零 operation、Decision、Approval 或替代执行路径。Goal/DoD、TargetText、LLM semantic-plan
文本与历史 evidence 都只能是 proposal/context/diagnostic；只有 SM-2 seal 才能补全 canonical binding。

semantic planning 是 SM-2/SM-3 aggregate 子域，不是第五个状态机。唯一 private transition 序列为
`semantic_contract_required -> semantic_plan_request_started -> semantic_plan_evaluated ->
semantic_feasibility_required -> semantic_feasibility_observed -> semantic_contract_sealed`；完整 rejection 只能进入一次
`semantic_plan_correction_required`，再次 rejection 或 owner/budget 终局只能进入 `semantic_contract_failed`。
ledger 必须持久化 Definition seed、manifest、attempt 0/1、stable prefix、logical request、correction、document、
owner batch/observation set 与 sealed binding lineage hash；这些 identity 必须进入 convergence progress signature。
duplicate event 不增加 revision，scope/seed/request/batch 漂移必须拒绝，任何阶段都不得直接创建 dispatch、coverage、
Decision 或 Approval。

`stale | prerequisite_required` 先以 `semantic_feasibility_observed` 单独持久化 owner factual observation。
只有带有 expected/observed revision 且精确匹配当前 catalog dependency 的 `stale` 事实，才能复用
`semantic_contract_required` private transition，按 persisted refresh generation 原子退休 matching catalog
observation、保存完整 gap/diagnostic lineage，并回到 bounded catalog acquisition。`prerequisite_required` 不得被
泛化为 stale、不得退休仍然有效的 catalog/observation，也不得消耗 prerequisite-refresh budget；当前 semantic
proof 没有 owner-defined typed prerequisite request，因此 SM-2 必须在保留该事实和完整 diagnostic 的前提下
fail-closed 为 typed terminal。未来若增加 prerequisite acquisition，必须先新增接收 owner 定义的 typed command/result，
再由同一 private transition 接入，不能复用 stale refresh。以上处理不是新的 phase 或第五状态机，不增加 semantic
candidate attempt。新 catalog revision 必须产生新 manifest/request identity，旧 complete session 只保留 audit；
generation history 的 identity 是封存的 `manifestHash + logicalRequestID`，所以同一 active generation 内的
correction/focus request 变化不会制造重复 history，而资源回滚重现旧 catalog hash 时可以用新的 logical request
建立独立 generation。duplicate generation 不写 CAS，refresh budget 耗尽才以原始 owner diagnostic 进入
`semantic_contract_failed`。

catalog acquisition 已标记的 validation-surface `prerequisite_required` 不进入上述 feasibility transition：正常
provider request 根本看不到该 identity，也不会为它创建 semantic attempt 或调用 endpoint。只有已经封存为 ready 的
surface 在后续发生 catalog 漂移，或接收了不符合 current catalog 的旧/伪造 proposal 时，owner feasibility 才按上述
规则 fail-closed；任何自动 input acquisition 仍必须先新增 owner-defined typed command/result。

### 单一 candidate evaluator

完整 candidate document 只允许进入一个纯 `CandidateEvaluator`。它的输入是 persisted manifest、decoded
candidate 和同一 authority contract 的 prior attempt fact；输出只能是 `accepted` 或一个完整的
`CandidateRejection`。owner feasibility 是 capability owner 的独立 typed factual observation，不是
`CandidateRejection` 的子类。decoder、admission 与 owner-feasibility adapter 可以提供 typed observation，但不能各自
选择 retry、terminal、governance 或下一 action。

为取得 feasibility observation，Orchestrator 只可先用 operation kind 解析 manifest 中的 owner 和
authorization identity；该步骤不做 candidate admission。无法解析的 kind 不调 owner，由 evaluator 在
kind 层拒绝；可解析的 operation 可调用无副作用 owner probe，owner 只返回
`feasible | stale | prerequisite_required | infeasible` observation。候选的结构性 evaluator 必须先按固定顺序
返回 wire/parameter/target 违例；只有结构性校验接受后，Orchestrator 才将 owner observation 作为独立后继处理。

`CandidateRejection` 至少包含：原始安全 class/code/message、operation ID（若适用）、field path、actual、
allowed/required/forbidden、manifest hash、rejected shape hash 与完整 scope/lineage。它是 evaluator 的事实输出，
不得携带 retry、terminal、governance、Runtime action 或 `CorrectionDisposition`。

SM-3 在收到 rejection 后，必须先把它转换为 owner-local internal event 并持久化；随后只能根据 current
manifest、attempt ledger、rejected shape 与 typed field violations 决定：

| SM-3 owner-local result | 语义 | 唯一行为 |
| --- | --- | --- |
| `correctable_once` | 同一 manifest 内存在唯一、有界的结构修正，且尚未使用 correction attempt | seal 一个 `CorrectionDirective`，只允许 attempt 1 |
| `rejected_final` | 没有安全同合同修正，或 correction attempt 已用尽 | 形成 typed candidate terminal，零额外 provider call |

`CorrectionDirective` 是 Orchestrator persisted state，不是 evaluator/LLM/capability feedback。它只描述 candidate
delta，例如删除 forbidden field、补 required field、选择 allowed kind、替换为 allowed target、改变 rejected
semantic shape；它不修改 authority，也不包含 raw provider/decoder/owner 错误文本。

若 complete rejection 无法由 SM-3 根据 typed diagnostic 归类，属于 `runtime_contract_defect`，必须 fail-fast
且零额外 provider、ticket、dispatch 或 effect。`candidate_correction_undefined` 不得作为正常模型输出兜底。

### 正交候选与 owner 处理结果

provider dependency/governance 在进入 candidate pipeline 前单独处理。进入候选处理后，所有结果只允许归入：

| 结果 | 来源 | 状态机行为 |
| --- | --- | --- |
| `request_recoverable` | EOF、output limit、timeout、429/5xx、未完成 document | 同一 sealed expected-document 的 whole-document 有界恢复；不创建 attempt、不拼接 raw suffix |
| `request_terminal` | provider authentication/configuration/protocol/submission unknown、request recovery exhausted 或 prerequisite stale | 先持久化 safe original diagnostic；转为 SM-3 internal event，再按 typed dependency、freshness 或 terminal 合同处理；不创建 attempt |
| `candidate_rejection` | CandidateEvaluator 的 complete typed rejection | 先持久化 original diagnostic；由 SM-3 owner-local reducer 决定唯一 correction 或 typed terminal |
| `owner_feasibility` | capability owner 的 `stale`、`prerequisite_required` 或 `infeasible` factual observation | 先转为 Orchestrator internal event，再由 SM-2 退休 prerequisite、取得新 authority 或形成 typed terminal；零 correction attempt |
| `accepted` | evaluator 接受且 owner feasibility 为 feasible | persist sealed dispatch 后才可执行 |

correctable/final 只属于 SM-3 persisted transition result，不是 `CandidateRejection` 字段，也不得接收 owner
code。以上分类是候选处理的唯一分流表。错误字符串、TUI/Web、projection、repair hint、owner adapter 或
测试 fixture 不得定义额外分流。

## 四个协调状态机

### SM-1：Run lifecycle

```text
pending -> active <-> blocked_on_decision | blocked_on_approval
active -> completed | failed | cancelled | superseded
```

- activation / resume intent 仅携带 workspace/run scope 与必要 activation identity；Run owner 在自己的
  reducer 中决定 `pending -> active` 或匹配 blocker 的 `blocked -> active`，成功后才发布 Run state fact。
- terminal 不可变。继续未完成工作只能由产品层显式创建新的 typed activation / Run，并保存
  predecessor lineage；不允许自动 successor。
- `fresh_activation_required` 是 Run owner 对不可读 opaque aggregate 的非终态行政结果。普通 Resume 只返回
  non-resumable fact，不可 drive；用户明确 restart 时，它只作为 Run-owned replacement/administrative termination 的
  原因，仍复用 SM-4 `Result -> Workspace -> Run` delivery。Workspace release 前不存在 successor；旧 aggregate 不执行
  schema migration，新 successor 由 Run owner 从 persisted replacement intent 创建并保存 lineage。
- blocked 回 active 必须有匹配的 persisted governance resolution。
- complete/fail 只能来自 SM-2 sealed terminal decision；cancel/supersede 必须先持久化
  `RuntimeTerminationRecord`。
- Run/Workspace/Result terminal delivery 是单向 owner-local saga；失败仅可重试对应 delivery，不能改写 authority。

禁止：SM-1 选择 action、HTTP/TUI/Web 直接写 terminal、Resume 注入 definition/provider/model/repair
patch、terminal 回到 active、projection 反向驱动 lifecycle。

### SM-2：Authority and focus convergence

SM-2 是 `RuntimeFacts -> RuntimeDecision + AuthoritySnapshot + AuthorityTransaction` 的唯一权威。

```text
load facts
  -> prerequisite missing: acquire + typed commit + reduce
  -> focus requires state change / validation / assessment / governance
  -> evidence and coverage gate
  -> focus advance | repairable gap | governance wait | sealed terminal
```

- semantic-plan manifest 所需的 template/profile policy、capability catalog 与 provider ceiling 未完成前不得请求
  semantic-plan document；sealed semantic contract、operation manifest、selection、snapshot、failure target、capability、
  validation scratch、provider ceiling 等未完成前不得请求 operation candidate。全部 prerequisite 必须通过 typed
  acquisition 获得并持久化。
- `RequiredOutputContract` 必须是 current focus、owner-confirmed capability、evidence policy 和
  side-effect compatibility 的中立交集。operation 可行动性不足必须在 operation provider 前确定失败；semantic-plan
  LLM 只能在封存 catalog/selector/recipe family 内提出 proposal，不能自由猜测 owner capability。
- validation catalog 必须同时保留 callable capability、ready/non-ready 和 direct/auxiliary/not-declared surface audit fact，
  但 provider projection 和 request-specific schema 只能包含 `ready + direct` 的 `ValidationRecipeRefs`；rendering boundary
  必须再次按 readiness 与 proof applicability fail-closed，不能把 `prerequisite_required`/auxiliary identity、
  schema/input binding、MCP effect metadata 或 Shell executable recipe 当作 model hint。readiness、proof applicability 或
  binding 变化必须改变 owner catalog snapshot/manifest identity。
- accepted owner receipt 的资源变更是正常前进：它只退休 dependency 精确相交的 observation，并以该
  `PrerequisiteID` 已持久化 invalidation record 的代际重获；不得消耗 recovery budget。read-only stale/
  no-progress 才使用封存且有界的 `prerequisite_refresh` ledger。duplicate 不得重复 CAS、查询、代际或扣减预算。
  semantic feasibility 的 `stale` 使用独立且有界的 prerequisite-refresh budget，并在 semantic ledger 保存 batch、
  refresh generation、全部 owner facts 与原始 diagnostic；`prerequisite_required` 在当前没有 typed semantic
  prerequisite request 时不退休 catalog、不增加该 budget，而是由 SM-2 形成保留原始诊断的 typed terminal。两者都
  不得复用 candidate attempt 0/1；若未来接入 typed prerequisite acquisition，必须使用 owner-defined command/result
  和独立代际，不能把它重解释为 stale。
- File workspace-tree revision 变化必须退休所有显式依赖该 exact revision 的 workspace-sensitive owner catalog，
  包括经过 manifest preflight 的 Shell validation catalog；没有该依赖的普通 Shell policy/target catalog 不得按 owner
  名称或 workspace 范围被扩大退休。重复 resource change/invalidation 必须幂等，且内生 owner receipt 不消耗 stale budget。
- evidence 必须同时通过 current-run、focus、sealed `CoverageEvidenceBinding`、quality、effect、receipt 和适用的
  recipe/check/metric/artifact 门禁。SM-2 只能从 sealed dispatch 的 `BindingRef`、owner receipt 与 persisted binding
  逐字段匹配后派生 CWI coverage；candidate 的 raw `coverageWorkItemIds`、assessment claim、非空 `PassedChecks`、
  exit code 0、历史 summary、memory、projection 和前一 focus evidence 都不能关闭当前 coverage。
- repairable gap 保持同一 focus，必须持久化结构化缺口。有效新 evidence 不清零同 focus 总 repair
  budget；只有原子 focus advance 才开启新的 repair ledger。
- LLM assessment 中的 disposition、缺失 evidence kind、目标和 reason 都只是 observation/original diagnostic，
  不是 Runtime action。SM-2 必须仅从已持久化的 current-focus coverage、accepted receipt/evidence quality、封存的
  focus/target policy、owner-grounded repair surface 及 freshness 生成唯一的 `RuntimeRepairDirective.RequiredState`。
  初次缺少有效验证只能进入 validation；已存在有效 current-focus validation、coverage 仍未收窄且存在新鲜
  owner-grounded implementation/effect repair surface 时才可进入 state change；accepted repair state change 必须按同一
  closure 固定进入 validation。repair surface 缺失/陈旧只能进入 prerequisite；无合法 target 或 validation recipe 时形成
  typed terminal。validation command entry 不得自动晋升为 repair target，且 validation-only budget 只能在 SM-2 已
  canonical 分类为 validation repair 后扣减。原始 observation 必须完整保留，但不得反向选择 action、retry、ticket 或
  terminal。
- assessment evidence gate 只验证 current-run/focus、binding-matched owner receipt、effect、quality 与封存的
  focus evidence policy；它不得把 assessment observation、claim signal 或 `PassedChecks` 非空解释为 CWI credit，
  也不得改写 observation 的 disposition、items、gap 或 refs。profile 的 observable validation recipe 只能约束候选
  选择该命令时的必要参数；只有 receipt 实际报告 binding 要求的 exact check/metric/artifact 才可满足该 binding，
  不得用 Goal/DoD、summary、operation ID 或检查名之间的自由文本相似度裁决 coverage。
- assessment 必须先独立证明 sealed binding 的其余 evidence/effect/quality 条件，再检查 current owner-fact postcondition。
  只有“其余 proof 全部成立、owner fact 是唯一缺口”时，SM-2 才能在提交 assessment dispatch 的同一 CAS 中把 current
  bound CWI 记为 `reopened`、清除 current binding、更新 reconciliation contract，并以 typed reopen lineage 退休当前
  semantic generation。旧 binding/receipt/evidence/assessment 只保留 audit；SM-4 随后从同一 aggregate 幂等驱动新
  semantic generation。普通 evidence gap 不得伪装为 owner-fact gap；同 CWI 重规划次数使用封存的
  `MaxRepairAttemptsPerFocus` 有界，耗尽后以 `owner_fact_convergence_exhausted` fail-closed。duplicate/restart 不重复
  reopen、request、effect 或预算扣减。重开 CWI 固定返回 `semantic-pending/<cwi>`；当新的 current-focus acquisition 与
  上一 generation 已退休 prerequisite 使用同一 canonical ID 时，SM-2 必须在排入 pending 前原子移出该 retired ID 及其
  旧 observation，且不得复用旧 observation。无关 retired lineage 必须保留，审计由 CWI reopen lineage 与 semantic
  generation history 持续承载，active/retired 不相交 invariant 保持严格成立。
- terminal `RuntimeResultCloseout` 以 canonical coverage ledger 为 closure authority。只有 canonical non-satisfied
  assessment/active gap 与当前 unsatisfied CWI 的交集可进入结构化 gap；producer 的 claimed disposition 或已满足 CWI
  的历史 gap 只能保留为 audit。top failure 依次使用 current `Facts.Failure`、current active/formal gap、terminal
  code/recovery lineage，Result owner 不得重新排序或重判。
- state-changing operation 的 owner no-op 不能形成 durable state-change evidence。SM-2 必须持久化 exact
  no-progress operation shape；若 focus 具备可观测 validation 且要求 assessment，只能走一次
  `validation -> assessment` 来确认已有状态。没有该语义验证路径时必须形成 typed provenance terminal，禁止修改
  无关文件制造 delta；重复同 shape 仍受 no-progress/repair budget 约束。
- governance 只用于独立封存的真实权限、外部依赖或明确要求人工输入的配置/策略阻塞。capability infeasible、
  retry/repair budget exhaustion、候选/证据合同违规本身不能创建 ticket；只有对应 owner fact 同时携带 typed
  `ExternalDependency/GovernanceRequirement` 时才可进入 `need_governance`。

禁止：SM-2 调 provider、执行 operation、直读其他 owner repository、将 profile/软件领域语义写进 reducer、
用数量而非质量关闭 evidence gate。

### SM-3：Candidate and sealed-dispatch transaction

```text
sealed authority + preflight ready
  -> persisted request session
  -> complete document -> candidate attempt 0 -> optional attempt 1
  -> accepted candidate -> persist sealed dispatch
  -> claimed operation -> owner receipt -> evidence commit -> close dispatch
```

- request session 必须先绑定 transaction、definition/evidence revision、required-output hash、expected-document
  identity、stable-prefix hash 和 logical request identity。EOF、output limit、timeout、429/5xx、401/403 与未知提交
  走 session/provider recovery，不消耗 candidate 或 repair budget。恢复只能重新请求同一有界完整 document；历史 raw
  output 只作 owner audit，不得拼接成 authority document。
- 只有完整 document 才可创建 candidate attempt。attempt 0/1 必须使用同一 transaction 与 stable prefix；
  第二次完整 schema/contract rejection 形成 typed `candidate_retry_exhausted`，不存在 attempt 2。
- renderer、wire DTO、strict decoder、admission、owner feasibility 与 fixture 对同一
  `CandidateContractManifest` 给出一致解释；候选只能选择 manifest 已封存的 `bindingRefs`。未知字段、参数、
  target、effect、dependency、operation kind、未知 binding 或裸 `coverageWorkItemIds` 不得由第二解析路径静默放宽。
- accepted candidate 必须在执行前持久化 `SealedDispatch`。operation 依 sealed dependency order 执行，
  每项使用稳定的 `ExecutionKey + Fence`；owner receipt 是 evidence 的唯一副作用回执。
- claimed operation 在没有 receipt 时，只能由 SM-3 返回一个只读的 recovery-budget handoff；SM-3 不得直接修改
  SM-4 的 recovery ledger。后续 `recovery_budget_consumed` private event 由 SM-4 独立 CAS 提交预算，并以
  `operation ID + exact persisted recovery attempt` 作为下一次安全重放的授权 fence；授权字段不属于其他 transaction
  fact，缺失、半截或错用的 envelope 必须在 aggregate load 前拒绝。
- 同一 dispatch 内 accepted receipt 造成的资源 revision 变化是内生事实：它可以使后续 SM-2 prerequisite
  observation 失效，但不得仅因此结束尚有 sealed operation 的 active dispatch。全部 operation 成功或任一
  operation rejected/stale 后，SM-3 才结算 dispatch 并交回 SM-2 reduce。
- duplicate 结果幂等，stale 只进 diagnostic lineage。已存在 exact receipt 时重启只提交后续 evidence，
  不得重放副作用；未知、非安全重放的副作用进入 typed outcome-unknown/failure 路由。
- effect/evidence 类型由 sealed `RequiredOutput`、sealed expected effect、sealed binding 与 owner observed effect
  联合裁决。receipt 只有同时匹配 `dispatch + operation + BindingRef + target/selector + effect + recipe/check`
  才能获得该 CWI credit；`none`、`ephemeral_session`、`unknown`、zero-work、unrelated target 或泛化 test success
  不能满足 durable state change 或 behavior coverage。
- exact `ExecutionKey + Fence` receipt 重放只复用既有 receipt；no-op 的 current-run provenance 与普通无
  provenance no-op 必须分流。后者只留下 rejected/no-progress lineage，不能被重放、target 变化或文本修改伪装成
  新的 durable evidence。

禁止：未持久化 request session 请求 provider、未完整 document 消耗 attempt、accepted candidate 不 seal
直接执行、executor/owner 选择下一 action、owner 直接终态化 Run/Result、依错误文本猜 target 或创建 ticket。

### SM-4：Drive, recovery and projection delivery

- `DriveRuntimeCommand/Result` 是 Orchestrator 唯一同步推进合同。Application 不保留 continuation queue；
  Run owner state notification 才能投递 Orchestrator 的内部 drive event。
- 每个非等待、非终态 transition 必须持久化递增 `StateRevision`；无进展或超过 guard 必须 typed
  fail-fast，不能在进程内空转。
- singleflight key 必须包含 workspace/run。重复 Start、Resume 和 governance continuation 只合并到
  同一 drive。
- Runtime aggregate 的物理存储可与 Run record 共用 CAS，但 Run owner 必须在未变化的
  `RuntimeState.StateRevision` 上重载并 rebase 无关模型写入；只有 aggregate revision 漂移才返回给
  Orchestrator 作为语义 CAS conflict。CAS recovery ledger 耗尽不改变业务 Run 为 terminal，也不得留下
  `active + nonterminal + no scheduled drive`：SM-4 必须保留 owner-local、退避的 reload/re-drive 后继。
- startup 的 Run owner typed inventory 只返回其拥有的 SM-1 state、RuntimeStateRecord 是否存在及 opaque
  revision/ref；不得读取 dispatch/governance/delivery ledger 分类 SM-2/3/4。Run 必须在 Setup 同时提供 scoped opaque
  record query；Orchestrator 通过该 query load 后在 owner-local SM-4 中分类 `active_no_dispatch`、
  `active_sealed_dispatch`、`blocked_governance`、`terminal_delivery_pending`。inventory/load 失败必须 fail-fast，
  不能由 application 或 Run owner 猜测。盘点完成但 Run 常规 commands 尚未注册时，Orchestrator 只能暂存本次进程的
  owner-local recovery handoff；仅 Run 的 commands-ready lifecycle fact 可释放 drive/lease/incompatibility action。
  readiness 不是 workspace state transition，crash 后 handoff 必须由新进程重新 inventory，且此边界不得依赖
  `Weight()` 或普通 Run Get/Update command。
- reducer terminal 已 sealed 后，SM-4 仅重试已持久化 terminal delivery request，等待 Result、Workspace、Run
  owner acknowledgement。projection health 不参与 reducer，也不是 runtime blocker。

禁止：重启重新 initialize 已有 aggregate、watcher/projection/TUI/Web 驱动 reducer、无 revision 进展的循环、
projection failure 覆盖既有 terminal、重放已知 provider/durable effect。

## 跨状态机不变量

1. SM-1 不选择 action；SM-2 不执行 operation；SM-3 不选择下一 action；SM-4 不重建 authority，亦不修改别的 owner state。
2. 每次跨状态机交互都携带 workspace/run scope 以及适用的 definition/evidence revision、authority
   snapshot、dispatch/operation identity。
3. 正式事实必须先持久化，后产生外部可见 projection；projection 可最终一致，authority 不可反推。
4. candidate、dispatch、governance resolution、evidence commit、terminal delivery 都必须幂等。
5. provider target、definition 与 activation 在 Run 内封存；变化需要新的 typed activation，不能由 Resume
   patch。
6. contract/invariant violation 必须形成 typed diagnostic/failure 并沿以上方向收敛；不得旁路某个状态机
   修补正式状态。

## 中立 effect、evidence 与治理

| Effect scope | 可关闭 durable state change |
| --- | --- |
| `none`、`ephemeral_session`、`governance_only`、`unknown` | 否 |
| `workspace_durable`、`remote_durable` | 仅在 RequiredOutput 接受、owner receipt 证明且 sealed binding 逐字段匹配时 |

`need_state_change` 的 sealed candidate 至少含一个与 RequiredOutput 兼容的 durable operation；
`need_validation` 只能含 `none` 或显式允许的 `ephemeral_session` operation。assessment 只消费当前
Run 的 accepted evidence；assessment-only focus 不执行 executable operation。

operation-scoped governance 必须保存 ticket、resolution、dependency、subject、receipt hash、ExecutionKey
与 fence binding。continue 以同一 ExecutionKey 的下一 fence 执行，owner 必须 preflight grant；普通 receipt
不能绕过治理。

state-changing capability operation 的 receipt 必须由 owner 对真实 effect 做有界观察。对 workspace durable Shell
operation，真实 changed refs 必须非空且属于 sealed `RuntimeOperation.TargetRefs`；存在未声明 changed ref、只有
unknown effect 或 zero-work 时不能形成 accepted durable evidence。owner observation 只返回 typed receipt，不得
解释 Goal/DoD 或选择下一 Runtime action。

## LLM request、cache 与上下文

LLM 是不可靠的 protocol client，不是 authority。请求必须由 sealed typed context 渲染：

- C0：stable runtime/schema；C1：Goal/DoD；C2：declarative capability/governance contract；
  C3：current focus/required output；C4：failure/evidence/diagnostic；C5：有界 ref。
- P0–P4 的必要字段缺失时不得提交 provider。raw prompt、raw provider body、credential、endpoint、
  snapshot content 不得进入 aggregate、audit、projection 或 memory。
- unexpected stable-prefix drift 是 renderer defect。完整 request/cache 规则以
  [runtime-request-cache-note.md](runtime-request-cache-note.md) 为准。

## Forbidden surface

以下路径已从生产和活动实施约束中删除：`StagePacket`、`NextAction`、`DispatchNextStep`、
`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher、application Start/Resume queue、
自动 replan successor，以及 checkpoint replay。

这些结构不得用于 DTO、EventHub 合同、状态迁移、配置或 prompt。

## 变更门禁与文档纪律

所有 Runtime 变更必须：

1. 标明影响的 SM-1～SM-4、owner、persisted fact、合法/禁止迁移；
2. 为每个 owner-defined typed command/result 添加 producer/consumer 回归；
3. 覆盖 normal、duplicate、stale、CAS 前中断、CAS 后响应丢失和 fresh-process reload；不可达语义必须
   由测试证明；
4. 覆盖预算/guard 与零副作用断言：在 prerequisite/authority 失败时不得产生 provider request、attempt、
   dispatch、ticket 或 effect；
5. 同步更新当前事实/未关索引、对应 module readme，及适用的 completed 验证记录；
6. 通过 `make verify-boundary-contract`，确认没有将状态迁移放入 `pkg`、没有 forbidden surface、没有让
   projection/TUI/Web 成为 authority。

活动实施顺序必须在 `70-roadmap/active/` 的真实未关项中定义。若发现第二份
Runtime 状态迁移、graph/step continuation、自动 successor 或 live 热修计划，
必须先删除或迁入 `docs/90-archive/`，不能与本文并列。
