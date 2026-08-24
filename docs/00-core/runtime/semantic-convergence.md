# Runtime Semantic Convergence

Status: normative

Type: core-contract-part

Last Verified: 2026-08-24

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

coverage-order request 的 closed schema 与 provider instruction 必须一致：`proposals` 是 required field 且只能为 `[]`，
不能用“不返回 proposals”替代空数组。缺字段、覆盖不全、重复/未知 CWI 等 complete-document 结构错误，由 SM-3 从
current manifest 合成 typed `coverage_order_document` factual violation，并只允许同 request contract 的 attempt 1；
proof-batch 的同类结构错误使用 `proof_batch_document`。纠错不修改 Definition/authority、不扩宽 owner catalog、不执行
operation；attempt 1 再失败才沿既有 typed terminal 收口。

owner operation-admissibility catalog 必须把“可调用 operation”与“当前可作为验证证明的 surface”分开。每个
`RuntimeValidationSurface` 由对应 capability owner 在一次一致 snapshot 中声明 `ready | prerequisite_required |
unavailable`、`direct | auxiliary | not_declared` proof applicability、input schema hash、owner-sealed input binding 或
prerequisite lineage、check refs、effect 与 replay class；只有已经具有完整 input binding、non-durable effect、明确的
`safe_to_replay | at_most_once | owner_managed` replay class，并由 owner 显式声明为 `direct` 的 `ready` surface 才能进入
`ValidationRecipeRefs`。其中`at_most_once`只允许首次sealed execution；claimed/no-receipt必须进入`outcome_unknown`而不能因
validation身份重放。`auxiliary`/`not_declared`、
`prerequisite_required`/`unavailable` surface 仍随 aggregate 和 audit 持久化，但不得
进入 provider schema、C3 ownerCatalog、correction allowed values 或 behavior probe。catalog 分类只读取 owner 本地/typed
registry fact，不调用 provider/MCP endpoint、不创建 candidate/dispatch/receipt，也不触发状态转换；Orchestrator 不能
根据 schema、tool 名称、描述、category 或 DoD 自行补 input binding、推断 direct proof。该 catalog disposition 不是
semantic-feasibility observation，不消耗 semantic attempt 或 prerequisite-refresh budget；若仍有合法 `ready + direct`
alternative，SM-3 只在该交集中规划，若无
可满足 required proof 的 alternative，则先检查owner是否提供typed recovery condition/trigger/deadline且当前budget允许；
完整时进入persisted recoverable runtime wait，否则沿bounded semantic-contract fail-closed主线处理。catalog文本或
`unavailable`标签本身不能创建wait。

profile 可以在 immutable DefinitionFocus 中声明中立的 `RuntimeOwnerFactRequirement`，但不能提交 owner 的目标状态或
生成第二份 capability authority。当前支持的 `selectable_validation_surface` 只要求指定 capability owner 的当前 typed
admissibility catalog 同时包含声明 operation kind、`ready + direct` surface 和同一 `ValidationRecipeRefs` identity。
Workorchd 只把 profile 声明编译进 bootstrap/DefinitionFocus；capability owner 只发布 catalog；只有 Orchestrator SM-2
可在 assessment commit 中读取该事实并决定 CWI credit。LLM assessment、operation 成功、文件变化或历史 recipe 均不能
代替当前 owner fact。

每个 required owner fact 在 pre-seal candidate 中还必须封存 preexisting，或唯一的
`owner_fact_missing -> acquisition owner/kind/variant/target-policy/operation-selector -> reobserve owner/fact` closure，且
`MaxAcquisitionAttempts=1`。Profile-derived closure 缺失、重复、与 exact Profile snapshot 派生 manifest 不一致、依赖悬空/
成环时，在 Run 前返回 `definition_profile_prerequisite_unresolved + select_or_fix_template_profile`。每个 Profile-required
acceptance/deliverable CWI 还必须在 pre-seal 具有 exact proof declaration、非空受支持 evidence kind 和 owner-receipt-capable
operation；否则返回 `definition_profile_contract_invalid`，不得降级为 assessment-only focus。SM-2 只消费这条
sealed declaration：Definition 的 target selector 只标识声明侧 subject/lineage，不是 capability operation authorization；
acquisition 必须另外绑定 capability owner catalog 中的 exact operation selector。current catalog 缺 fact 时，SM-2/SM-3
先把每个 CWI 的声明 lineage、required fact 与 exact acquisition 编译为唯一 `RuntimeSemanticProofObligation` 并封存进
semantic manifest，后续 provider schema、纠错和 binding compiler 只能消费该 obligation，不能再次读取 aggregate
`DefinitionFocuses` 或把两套 selector 当作别名。semantic proof batch 必须恰好匹配当前 all-of dependency 已就绪的
acquisition owner/kind/variant/target-policy/operation-selector；owner feasibility 与 receipt 后只重获声明的 owner/fact。一次 acquisition 已形成 accepted
operation/evidence 但 reobserve 仍缺失时直接耗尽，不能再生成第二个 semantic acquisition。Shell 不选择 File 操作，File
不解释 Shell fact，kernel 不硬编码语言/测试框架。

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
dynamic feasibility只能核对owner-issued catalog/schema/input binding与本地静态prerequisite；即使声明non-durable或
`safe_to_replay`，也不得调用endpoint、启动provider process或采样operation结果。外部行为事实只能来自persisted SM-3
dispatch对应的owner receipt。

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
Run、Result 只能通过各自 owner-local internal event 迁移自己的状态。持久化 aggregate 缺少当前合同时必须在严格
decode/validate 边界失败；不得补声明、推导旧状态、生成 compatibility disposition 或进入 Runtime drive。

### Definition declaration and owner-policy boundaries

Fresh activation 的 immutable `DefinitionFocuses` 与 SM-2 generation-specific
`Focus/RemainingFocuses/CompletedFocuses` 必须分离。前者只包含 CWI、presentation slice、required
evidence/assessment、声明式 metric/artifact obligation、owner-fact postcondition 与可选 profile selector identity；
不得携带 resolved target、operation kind/variant、recipe/check 或 binding。后者可以随 owner resource revision
退休、重开和重新 seal。semantic document admission、owner feasibility 与 final seal 都只能用 immutable
DefinitionFocus 检查 source declaration，不能把上一 generation 的 accepted operation 当成下一 generation 声明。

该分离必须由类型而非运行期字段清空实现：`RuntimeBootstrap.DefinitionFocuses` 与
`RuntimeFacts.DefinitionFocuses` 使用 `RuntimeDefinitionFocus`；初始执行游标只能由
`RuntimeFocusSeedsFromDefinitionFocuses` 生成不含 target selector、owner-fact closure、resolved target、operation、recipe、
check 或 binding 的 `CoverageFocus` seed。只有 SM-2 semantic seal 可以用 generation-specific `CoverageFocus` 替换该游标。
Profile 声明的 exact target 仍保留在 sealed activation/profile contract，并与 current capability-owner catalog 取交集；
不得把它复制进 immutable DefinitionFocus 冒充 owner-resolved target authority。

owner target scope 的空观察只证明当前不存在既有目标，不能授权任意新路径。prospective creation 必须同时具有 activation
封存的 exact prospective target、完整 `selector ref + owner ref + subject ref`，以及同一 capability owner catalog 发布的
matching selector/constraint/operation variant；缺任一项即在 provider 前以 typed target-grounding terminal 收口。
Shell managed-validation catalog 需要 target vocabulary 时，Orchestrator 只可把已持久化的 File-owner target inventory 作为
typed acquisition input 转交 Shell；File effect 使旧目录失效后，新一代 acquisition 仍使用 fresh File observation。
Shell 独立决定 recipe/surface readiness，Orchestrator 不扩张路径，Shell 也不成为 File target owner。

catalog retirement 可以在同一 Orchestrator owner-local CAS 中打开新的 semantic generation，而当前 binding-free CWI 的
SM-2 reducer view 尚未获得 target/binding。仅当该窗口形成的 terminal code 是 `target_grounding_unresolved` 时，它不具备
SM-4 delivery authority：drive、candidate request guard 与 fresh-start recovery 必须先完成 `semantic_plan`
request/owner feasibility/SM-2 seal。`SemanticOperationAuthorityReady` 或 `semantic_contract_failed` 成立后才可交付该终态；
其它已封存的 contract defect、budget exhaustion 或 owner failure terminal 必须保持原权威，不能被语义重规划覆盖。
administrative termination 仍走独立 Run-owner fact/delivery lane。该门禁不允许 operations request 穿过 binding-free 窗口，
也不允许 capability owner、Application 或 projection 改写任何 Runtime state。

当前 authority 同时绑定 capability-owner policy revision、equivalent-authority input、binding contradiction 和 opaque
lifecycle fence。File 等 owner 的 semantic/operation feasibility 必须消费同一 owner policy；Orchestrator 只有在
Definition、focus/binding、evidence、catalog/prerequisite freshness 或 owner observation lineage 变化时才创建新
`AuthoritySnapshot`。Run 只能比较随 opaque Runtime record 保存的 lifecycle fence，不能解码 SM-2/3/4 选择 SM-1 transition。

`OwnerPolicy` 只标识 observation owner、policy revision 与 catalog snapshot，不隐含 binding contradiction。只有
capability owner 显式返回 owner-neutral `sealed_target_authorization_contradicted` impact，且 SM-2 把它逐字段关联到
persisted attempt/operation、current CWI/binding、reconciliation binding、catalog、authority transaction、target set 与
observed resource revision，才能退休 binding。`observation_unknown`、无效 patch 或 replay-proof unavailable 等
candidate-local observation 即使携带 OwnerPolicy，也只能进入 SM-3 invalidated-attempt/no-progress，不能重开 CWI。

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
