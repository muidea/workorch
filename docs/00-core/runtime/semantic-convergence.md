# Runtime Semantic Convergence

Status: normative

Type: core-contract-part

Last Verified: 2026-09-03

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
   replacement intent；该 `run.replacement` 是同 Run owner 下的 SM-1 subordinate transaction。SM-4 仅把
   `RuntimeTerminationRecord` 持久化为 delivery handoff，再按 `Result -> Workspace -> Run` 投递 administrative terminal fact；Workspace 仅在
   `ActiveRunID` 精确匹配时通过内部事件释放 slot；Run owner 只有收到该顺序的最终 Run delivery 后，才能通过内部事件
   迁移 predecessor terminal，并幂等创建、激活 current-baseline successor。Application、Workspace 和 Orchestrator 均不得
   直接迁移 Run 或提前创建 successor。
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

每个已 admission 的 exact CWI 若要求 validation，activation compiler 必须从 immutable Definition revision、
CWI/source identity、source-text hash 与该 CWI 的 canonical declaration 编译唯一
`RuntimeValidationProofRequirement`；其中生成的 CWI check ref 对不同 CWI 必须不同。Profile 的 wildcard 只是声明模板：
Application 只能在一个已经 admission 的 exact CWI 上实例化其质量、target/selector、owner-fact acquisition、repair 与
evidence/operation 上界；wildcard identity 本身不能进入 Runtime，且不能贡献 current owner fact、recipe/native check、
dispatch 或 credit，也不能替代该 exact declaration。requirement 是 Definition 声明事实，不是 operation/binding/credit；recipe 仅可作为 exact Profile
declaration 的可选约束，空值表示由 capability owner 从当前 ready typed catalog 选择兼容载体。普通用户不需要编写
Runtime JSON、recipe 或 check。

SM-3 使用同一 request-session、complete-document evaluator 与 attempt-0/1 预算请求一个无副作用
`semantic_plan` document。它只能逐 CWI 提出 owner、operation/effect、三种 target policy 之一、target/selector、
owner catalog 中兼容的 recipe 与适用 metric/artifact；不能编写 declaration/check，也不能携带 Runtime action、coverage state、credit、terminal、retry、ticket 或
owner result。semantic provider request 前，SM-2/SM-4 必须已通过 typed prerequisite 获取并持久化 canonical owner
admissibility catalog；manifest/hash/schema/evaluator 只暴露该 snapshot/revision 中的 operation、target、selector、
constraint 与当前适用的 recipe vocabulary。声明了 workspace-sensitive validation recipe 时，SM-2 必须先取得 File
owner 的 manifest 及其 exact freshness observation，再通过 concrete typed request 交给 recipe capability owner；后者只
返回对该 manifest 当前适用的 recipe，并在自己的 catalog observation 中复制 exact File workspace-tree revision。
Orchestrator 不解释 manifest path 或 recipe。没有 confirmed recipe 的 `validation/managed_session` 不得进入
provider-visible allowed operation set。SM-3 接受 document 只形成 internal fact，不创建 `SealedDispatch`。

owner binding 声明 `InputReview` 时，proof-batch 还必须携带 exact 内容身份及所有原 required checks 的 closed
`validationInputReview`；check refs 只是引用原声明，不能增删或替换准则。SM-2 仅允许完整一致报告继续 feasibility/seal，
完整冲突明确失败，完整信息不足沿既有 Decision governance ticket 请求事实。matching resolution 保留旧 generation，
以新 semantic manifest/request 重新评审，不改 Definition 或直接 credit。request、信息一次性和 history 容量均有界；
格式/身份错误不能降级为业务 ticket。完整合同见 [Candidate and Dispatch](candidate-and-dispatch.md)。

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
`ValidationRecipeRefs`。Shell managed-validation surface 还必须对 request 中的 exact
`RuntimeValidationProofRequirement` 返回逐 CWI `ProofRequirementBinding`；surface-wide check union、recipe identity 或 exit 0
均不能替代该 binding。binding 必须逐 check 封存 capability-owner 的 opaque observation protocol/native check mapping；
execution receipt 只接受 matching typed attestation，raw stdout/stderr marker 不是 proof。其中`at_most_once`只允许首次sealed execution；claimed/no-receipt必须进入`outcome_unknown`而不能因
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

surface 的 recipe-wide `Readiness` 与逐 CWI proof readiness 必须分离：同一个 ready recipe 可以同时携带 CWI-A 的
`ProofRequirementBinding` 与 CWI-B 的 `ProofRequirementPreparation`。preparation 只是 capability owner 对缺失 native
check/input 发布的 exact、all-of、revision-bound prospective fact，不是 proof 或 operation authority；只有 Profile 已显式
声明该 selectable-owner-fact 的唯一 acquisition/reobserve convergence 时，SM-2 才能封存 acquisition transaction。receiving
owner receipt 后必须重新查询 capability owner，由 fresh observation 把 preparation 转成 binding，再由 SM-2 私有事件 seal。
exact acquisition receipt 同时消费该 prospective transaction 的 create-only `TargetSelection` lifecycle identity；目录变更失效
只能重新取得受影响的 owner facts，不得把已执行 selector 放回 pending 或在 fresh restart 后重建它。
preparation 可以附带 capability-owner 原始、有界、audit-safe `OriginalDiagnostic`；owner 与 catalog revision 必须精确
匹配，且原文进入 catalog snapshot hash。SM-2 仅在既有 owner-private exhaustion event 中，将当前缺失 exact requirement
的唯一原因复制到 `RuntimeTerminalDecision.OriginalDiagnostic`；缺失/歧义/过期原因不从历史或文本重建。它是原因事实，
不是第五状态机或第二份 action/terminal kind，不能改变单次 acquisition、重试或 credit 合同。SM-4 与投影层只消费已封存
原因；旧 archive 中字段缺省时保留原哈希和旧 delivery material，不做 backfill。
preparation 还可携带成对的 `ContentContractRef + ContentRequirements`：由 capability owner 定义的完整有界输入语法，
分别不超过 512/4096 bytes，无控制字符，必须为 UTF-8。二者进入同一 catalog hash，semantic/operations C3 随 exact
target/native identity 保留；Core 只验证合同身份与边界，不解析语言、裁剪内容或授予 effect/review/credit。
空扩展保持旧 JSON/hash；输入生成仍只走既有 SM-2 acquisition、SM-3 dispatch 和 receiving owner 的 create-only 合同。
不能通过声明的 prospective acquisition 补齐的输入必须独立发布为 `ProofRequirementUnavailability`，包含 exact
Definition requirement、recipe 与 owner 原始 diagnostic，并由同一 catalog snapshot/revision 绑定；不得同时发布该
requirement 的 binding/preparation，也不得推导替代 target。SM-2 等 current prerequisites 全部结算后，在既有私有
reduction/CAS 中按 `owner_fact_convergence_exhausted` 有界收口，先于新的 semantic provider request 与 acquisition。
binding-free focus 的 postcondition 只从 exact CWI 的 immutable Definition declaration 读取，不向 cursor 回填 authority。
原因完整进入 terminal/Result，fresh-load 拒绝缺失/漂移，SM-4 与资源 owner 不重新裁决。
Application 只把 Profile 声明实例化到 admitted exact CWI 并写入 requirement hash，不得自行发明跨 owner convergence；缺
显式 Profile closure 的 managed CWI 在 activation admission fail closed。

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
acceptance/deliverable CWI 还必须在 pre-seal 具有可确定编译的 canonical exact proof declaration、非空受支持 evidence kind 和 owner-receipt-capable
operation；activation compiler 只从已经 admission 的 exact CWI 生成 declaration/requirement。wildcard policy 只能在该编译点成为
exact CWI 的声明约束，不能被实例化成 current owner fact、recipe/native check、dispatch 或 credit。
无法实例化时返回 `definition_profile_contract_invalid`，不得降级为 assessment-only focus。SM-2 只消费这条
sealed declaration：Definition 的 target selector 只标识声明侧 subject/lineage，不是 capability operation authorization；
acquisition 必须另外绑定 capability owner catalog 中的 exact operation selector。current catalog 缺 fact 时，SM-2/SM-3
先把每个 CWI 的声明 lineage、required fact 与 exact acquisition 编译为唯一 `RuntimeSemanticProofObligation` 并封存进
semantic manifest，后续 provider schema、纠错和 binding compiler 只能消费该 obligation，不能再次读取 aggregate
`DefinitionFocuses` 或把两套 selector 当作别名。semantic proof batch 必须恰好匹配当前 all-of dependency 已就绪的
acquisition owner/kind/variant/target-policy/operation-selector。`RuntimeSemanticEvidenceObligation` 必须同时保留 immutable
`TerminalRequiredEvidenceKinds` 与 generation-specific `CurrentRequiredEvidenceKinds`：正常 generation 二者一致；acquisition
generation 的 current kinds 只能是 acquisition 所需的 `state_change`，不能覆盖 Definition 的 terminal `validation` 等声明，
也不能继承 terminal metric/artifact credit。

target-grounded Profile declaration 若允许在没有 bounded exact candidate path 的空 scope 中进行 prospective File creation，必须在 pre-seal snapshot 中携带完整
`ProspectiveTargetAllOfConstraintRefs`。该集合最多 8 项、规范化后每项都必需，并与完整 declaration selector
ref/owner/subject 一同进入 Definition/Profile hash；它既不是 exact target set，也没有任选一项的语义。File 的
pre-semantic `TargetSelection` 必须回显该集合，且只在集合 exact-equal owner 支持的 prospective constraint set 时返回可继续的
selection。compiled `RuntimeSemanticProofObligation` 以
`DeclarationProspectiveTargetAllOfConstraintRefs` 封存同一值，C3 只投影这一 sealed declaration；SM-3 schema 与本地 admission
要求 prospective proposal 的 `ProspectiveConstraintRefs` exact-equal 整个集合。缺项、扩项或漂移均在 owner probe 前形成 typed
field violation，不能由 File/Shell result 补齐或缩减。

`prospective_owner_target` acquisition 还必须在 pre-seal Profile declaration、compiled obligation 与 provider projection 中携带
同一个非空 `ProspectiveSatisfactionContractRef`；preexisting 与 `existing_owner_scope` 禁止携带。pre-seal
Definition 只封存 declaration selector/candidate refs、mandatory prospective constraint refs 与 acquisition closure；compiled obligation/manifest 只追加
acquisition owner/kind/variant/target-policy/operation-selector 与 current owner constraints，两者都不携带 exact
prospective target set。required owner catalog 发布 provider-safe typed `ProspectiveTargetSatisfactionConstraint`，只含
`all_of` clause；每个 clause 内的 predicate 全部必须满足，全部 clause 都是必需条件，不存在替代分支。compiled
acquisition/manifest 完整封存它并只在 C3 投影。SM-3 只能提交一份 bounded semantic proposal，其
`ProspectiveTargetRefs` 包含 1～8 个唯一、规范化的 exact target ref；每个 clause 至少由集合中一项完整覆盖，
每个 target 也必须覆盖至少一个 clause，不允许无关附加写入。集合中每项都必须在 Definition declaration
selector/candidate refs 与 current operation selector/owner constraints 的交集内；declaration candidate refs 非空时，
每项都必须 exact member match。current semantic wire 不接受 singular `ProspectiveTargetRef` 或兼容 alias。
schema/document admission 必须在 owner probe 前验证整个 target set 的合取完整性。它在 SM-2 seal 前只是
proposal，不是 Definition 或 operation authority。proposal 通过 File owner 只读 feasibility 后，File 对同一 current complete
manifest 执行整组 path、absence、parent 与 aggregate capacity admission，仅生成 exact base-revision-bound
`RuntimeProspectiveWorkspaceManifestDelta`；不创建任何文件或目录。Orchestrator 再把该 immutable delta、activation-sealed recipe declarations（固定
command 或 owner-adapter ref）和
required-owner current dependency catalog/observation 交给 required owner 的 concrete typed pure query。semantic manifest
必须把 provider-selectable `OwnerCatalogs` 与 `OwnerFactDependencyCatalogs` 分开：后者进入 manifest/hash/freshness/CAS/seal，
但不进入 LLM C3 owner catalog、allowed operation/surface、schema enum 或 correction choice。required owner 逐项重验
contract ref、owner-resolved recipe command/ordered args/timeout schema identity、catalog/observation 与 delta lineage，只返回 nested
`would_satisfy | would_not_satisfy | typed stale` factual observation；它不访问 File/workspace、不执行 operation、不选择
Runtime state。Orchestrator 必须在 owner-private observation CAS 与 semantic seal 前重验 File outer feasibility/delta、
required-owner nested observation，以及 manifest/batch/request 和两组 catalog/observation identity。required owner 只对 delta 中的
resulting manifest 判定已封存 recipe 是否可选，不以任意单 target 代替完整 manifest。`would_satisfy` 才可加入
final feasibility set；nested typed stale 不能伪装成 File outer stale或直接 terminal，只进入既有 bounded catalog refresh；
missing dependency 在 query 前退休 generation并保持零 required-owner call。typed constraint 不匹配只产生普通完整
document/schema correction，C4 引用 manifest/CWI/field path 且不复制 C3 constraint；该分支零 File/Shell probe。

`would_not_satisfy` 必须先随 File delta 持久化为 exact-current full composite observation；该 target set 仍未封存为 binding，
且不产生 operation authority。由于 document admission 已证明 target 满足 published `all_of` contract，这个 owner result
表示 required-owner contract contradiction。Orchestrator private drive 直接记录
`prospective_satisfaction_contract_violated` 并形成 `semantic_contract_failed`，不得创建 replacement-target correction、
`Allowed` 或第二 provider request。Shell 只拥有 persisted satisfaction/reason；terminal
diagnostic 仍由 Orchestrator private event 署名和提交。typed stale 只刷新 exact dependency fact，不增加 semantic attempt
或消耗 correction；所有分支零 ticket/governance/acquisition/operation/effect/coverage，duplicate/restart 不重复 terminal
或同一 owner probe。这个边界不新增状态机或跨 owner transition。

owner feasibility 与 receipt 后只重获声明的 owner/fact。若 reobserve 仍缺失，一次 accepted acquisition operation/evidence
立即耗尽，不能再生成第二个 semantic acquisition；若 reobserve 已满足，SM-2 也不得把 acquisition evidence 记为 terminal
coverage。只有 `assessment_observed` owner-private event 可以在同一 CAS 中提交 acquisition assessment/audit、保留 exact
operation anti-replay/attempt fact、将 CWI 保持为 open 并记为 `reopened`、以
`assessment_owner_fact_acquisition_settled` lineage 退休当前 generation，然后回到 `semantic-pending/<cwi>`。successor
manifest 必须移除 acquisition、恢复原 terminal evidence obligation；只有 successor binding 的 matching terminal receipt/
assessment 可以 settle CWI。当前 binding 的 purpose 必须由 exact `SemanticManifestHash + CWI ID` 从 active manifest 或
immutable generation history 恢复；该历史只解释 purpose，不能授权候选、operation 或 transition。aggregate invariant
必须拒绝 acquisition purpose 的 `satisfied/settled` 组合。duplicate/restart 只从该 aggregate 幂等 re-drive，不能重复
effect、assessment、reopen 或 attempt。
acquisition assessment 的质量裁决只检查该 purpose 当前 binding 的 `state_change` proof；Definition 中仍然封存的
`RequireExecutableBehaviorEvidence` 是 successor terminal-proof gate，不能反向要求 acquisition receipt 伪装为可执行行为证据，
也不能因 producer 声称 `repairable_gap`、missing kind 或 target unsupported 而阻止上述 settlement。该门禁只延后到 successor，
不因 acquisition 成功而被删除或视为已满足。
Shell 不选择 File 操作，File 不解释 Shell fact，kernel 不硬编码语言/测试框架。

Definition-gap provider document 还必须通过 pure、manifest-bound admission 才能成为
`RuntimeDefinitionGapObservation`。当 offending CWI 已有 sealed `OwnerFactAcquisition` 时，provider 报告
`undeclared_prerequisite` 与 manifest 冲突，必须进入同 request 的 bounded typed correction，且零 clarification observation、
`need_governance`、ticket 或 Definition mutation。只有 manifest 未声明 closure 的真实 gap/ambiguity才可进入专用
Runtime clarification lane；LLM 不能借 gap kind 选择 ticket 或任何状态转换。

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
- Shell 当前 Run 的 validation reconciliation 从同 workspace/run/definition、exact binding/hash 的已完成 SM-3
  dispatch 投影 recipe 与完整 validation scope，并复用普通执行的 owner-native 检查映射和准备路径。
  该读取不授予新 operation authority；不得用当前目录、后来 catalog 或操作入口 target 替代封存 scope。
  缺少精确历史上下文的 behavior proof 保持 unverifiable；具体 owner preparation/execution cause 必须保留，
  不能全部折叠为通用 proof-incomplete。owner 不决定 retain/reopen，仍由 SM-2 private event 消费批次。

Semantic planning 与 reconciliation 都不是第五个状态机：request/attempt/feasibility/seal/batch/history/lineage
均属于 Orchestrator SM-2/SM-3/SM-4 aggregate；Workspace、
Run、Result 只能通过各自 owner-local internal event 迁移自己的状态。持久化 aggregate 缺少当前合同时必须在严格
decode/validate 边界失败；不得补声明、推导旧状态、生成 compatibility disposition 或进入 Runtime drive。

### Definition declaration and owner-policy boundaries

Fresh activation 的 immutable `DefinitionFocuses` 与 SM-2 generation-specific
`Focus/RemainingFocuses/CompletedFocuses` 必须分离。前者只包含 CWI、presentation slice、required
evidence/assessment、声明式 metric/artifact obligation、owner-fact postcondition、target role 与可选 profile
selector/candidate scope、mandatory prospective constraint refs、可选的单一 Profile `ValidationFailureRepairConvergence`，
以及 activation compiler 生成的 exact CWI declaration 和可选 exact Profile recipe/check constraint；不得携带
exact/resolved/prospective target、selected operation kind/variant、owner applicability 或 proof binding。后者可以随 owner resource revision
退休、重开和重新 seal。semantic document admission、owner feasibility 与 final seal 都只能用 immutable
DefinitionFocus 检查 source declaration，不能把上一 generation 的 accepted operation 当成下一 generation 声明。

该分离必须由类型而非运行期字段清空实现：`RuntimeBootstrap.DefinitionFocuses` 与
`RuntimeFacts.DefinitionFocuses` 使用 `RuntimeDefinitionFocus`；初始执行游标只能由
`RuntimeFocusSeedsFromDefinitionFocuses` 生成不含 target selector、owner-fact closure、resolved target、operation、recipe、
check、binding 或 `ValidationFailureRepairConvergence` 的 `CoverageFocus` seed。只有 SM-2 semantic seal 可以用
generation-specific `CoverageFocus` 替换该游标。
Definition/activation 只封存 declaration selector identity、candidate refs、mandatory prospective constraint refs 与 acquisition closure；它们不封存
exact prospective target set。`ProspectiveTargetRefs` 首次出现在 SM-3 semantic proposal，File/Shell owner fact 只通过
typed lineage 绑定该 proposal/target set 并返回 factual feasibility/satisfaction；只有 SM-2 private event 封存的
`CoverageEvidenceBinding` 才是完整 target-set authority。后续 operations manifest 为每个 target 封存一项
`RequiredOperationSequence`，每项的 `TargetRef` 要求 candidate `targetRefs` 为 exact singleton，并通过 requirement dependency
按 canonical order 执行；candidate 必须完整提交全部项，dispatch 只从 binding 派生 target，不重新选择或允许部分执行。

owner target scope 的空观察只证明当前不存在既有目标，不能授权任意新路径。开放 scope 的 `prospective_owner_target` creation 必须同时具有
SM-3 document 提出的 bounded nonempty exact target set、Definition 的完整 `selector ref + owner ref + subject ref`/candidate scope、
exact mandatory `ProspectiveTargetAllOfConstraintRefs`，以及同一
capability owner catalog 发布的 matching operation selector/constraint/variant。缺 proposal target set 由 strict schema/admission 拒绝；
scope/catalog 不匹配由 pure evaluator 或 File feasibility 拒绝。全部 exact-current owner fact 通过后，SM-2 才封存 binding；
之前始终零 candidate/dispatch/effect。

`exact_target | existing_owner_scope | prospective_owner_target` 三种 policy 共用同一 pure admission intersection：semantic
proposal 必须同时落在 immutable Definition selector/candidate declaration 与 current operation selector/catalog surface 内。
Definition selector 只提供 declaration lineage，不能冒充 operation selector；任何 selector、candidate、target 或 owner-scope
漂移都形成 typed proposal violation，不能进入 binding、dispatch 或 effect。

Orchestrator 必须用一份 owner-private target-authority projection 统一 SM-2 grounding 与 SM-3 prerequisite/candidate
admission：先从 current repair/focus facts 得到 exact target set，再依据 current SM-2 binding + owner feasibility lineage
将其分类为 existing 或 prospective。只有 existing target 可以产生 File `target_snapshot` prerequisite；已经通过 File
feasibility 并由 binding 封存的 absent prospective set 在 create 前不得再次进入 snapshot/lstat 路径。Definition/Profile
的 selector owner ref 仅表示声明 lineage；capability owner 必须由 sealed proof operation descriptor 推导，并与匹配的
owner catalog snapshot 校验，不能把 declaration owner 与 operation owner 合并为同一身份。该 projection 是纯派生事实，
不持久化新状态、不新增状态机，也不允许 File/LLM/Shell 触发 Orchestrator transition。
ordinary Shell managed-validation catalog acquisition 需要 target vocabulary 时，Orchestrator 只可把已持久化的 File-owner
target inventory 作为 typed input 转交 Shell；prospective-satisfaction pure query 只额外接收 File owner 从该 inventory
计算的 immutable hypothetical delta/resulting refs。File effect 使旧目录失效后，新一代 acquisition 仍使用 fresh File
observation。Shell 独立决定 recipe/surface readiness 与 prospective satisfaction judgment，Orchestrator 不扩张路径，
Shell 也不成为 File target owner。

要求 target grounding 的 immutable DefinitionFocus 必须在 semantic provider request 前形成 focus-scoped typed
`TargetSelection` prerequisite，并由 File owner 返回 exact selection 与 workspace-tree freshness observation。只有该 observation
逐字段匹配 current focus、Definition selector/owner/subject/candidate declaration，且 disposition 为
`absent | ambiguous | unsupported`，或“empty scope 且 Definition 既无 bounded selector candidates 也无 owner-accepted
`ProspectiveTargetAllOfConstraintRefs`”时，
`target_grounding_unresolved` 才是可交付的 owner-grounded terminal，整个
路径保持零 semantic/candidate provider call。缺少 selection、observation 或 current lineage 的 binding-free view 仍是 transient，
不得提前交付；`selected`，以及“empty scope 但 Definition 有 bounded selector candidates 或 exact owner-accepted mandatory
constraint set”的合法 prospective creation则继续
semantic planning。

该前置条件必须在 accepted coverage order 的每个 proof batch 重新验证，而不只验证 semantic generation 的首个 batch。
`EligibleFocusProjected` 推进到下一 CWI 后若投影出新的 focus-scoped `TargetSelection` 或其它 semantic prerequisite，SM-4 drive
必须先调用既有 typed owner command/result，且只有 SM-2 owner-private CAS 可以将其结算；
在 fresh aggregate 上仍有待办时，不得编译下一 proof batch、派生其 `RequestLogicalID`、创建 request session、调用 provider
或扣减 provider budget。accepted order 与 manifest
可以保持不变，但 batch identity 必须从包含 current owner observation 的 authority transaction 派生。结算后的 drive 只从 CAS
返回的 aggregate 重新分派；duplicate、response loss 与 fresh restart 不得重复 owner effect 或 provider call。

catalog retirement 可以在同一 Orchestrator owner-local CAS 中打开新的 semantic generation。若旧 generation 的 no-effect
binding 尚为 current binding，SM-4 必须先投递既有 typed `CWIBindingRetired` internal fact，由 SM-2 唯一 reducer 退休旧
catalog lineage 并 reopen CWI；在该事件完成前，旧 binding 既不是 operation authority，也不能使 provisional
`target_grounding_unresolved` 具备 delivery authority。随后 drive、candidate request guard 与 fresh-start recovery 才能完成新的
`semantic_plan` request/owner feasibility/SM-2 seal。已有 effect credit 的 binding 不得因 catalog refresh 自动退休；其它已封存的
contract defect、budget exhaustion 或 owner failure terminal 也必须保持原权威，不能被语义重规划覆盖。administrative
termination 仍走独立 Run-owner intent/SM-4 handoff lane。该门禁不允许 operations request 穿过 transient 窗口，也不允许
capability owner、Application 或 projection 改写任何 Runtime state。

execution-locked binding 下的新 fresh validation 若 rejected，SM-2 先用 exact receipt 与当前 sealed route/budget 判定新
repair-owner confirmation 义务，不得因 current catalog 已更新就跳过确认直接交付普通 target-grounding terminal。义务不
退休原 binding、不触发 semantic successor，也不授权 operation；SM-3 在确认结算前拒绝 semantic/operations candidate
入口。确认后的 state-change repair 仍需新的 validation receipt，不能复用此前失败序列的 confirmed target 或 CWI credit。

semantic generation 的 eligible closure 必须由 generation purpose 唯一派生，并由 accepted-order projection、focus
advance 与 retirement check 共用。普通 definition generation 使用当时全部尚未绑定且可规划的 current CWI；
`validation_failure_repair` generation 只包含触发 repair 的 exact current CWI，不得把其它全局 eligible CWI 拼入该
generation。因而无关 CWI 变为 eligible 不能退休 repair-local manifest，也不能制造新的 provider request；普通 generation
中确有新的 eligible member、或 repair CWI/purpose/lineage 漂移时，才以 owner-private event 退休 exact generation 并重新
规划。该 scope 是 SM-2 authority，不由 LLM document、capability result 或 SM-4 推断。

该 repair generation 的 provider C3 必须投影 sealed `ValidationFailureRepair` 的 failed focus、stable failure signature、
typed trigger owner/operation/diagnostic class/evidence target type/owner code、convergence ID/hash、matching route ID/hash、
repair owner/operation vocabulary、target policy、declaration candidate scope、operation selector/constraints 与
fresh-validation bound。repair owner 来自 sealed Profile exact route；该 route 的 operation descriptor 必须共同声明一个已接线的
typed confirmation protocol。若该
declaration 不适用，则只能由 source binding 中 state-change operation descriptor 的唯一 owner 确定，不能按 capability 名称或
diagnostic 文本推断。不得投影 raw diagnostic、owner message、receipt、Runtime action、retry 或 terminal disposition。provider
只能提出一份结构匹配且与 typed failure 相关的 `state_change` proposal；每个 prospective target 都必须对修复该 failure 有
贡献。选定的 resource/effect owner 只判断完整 target set 的 factual feasibility，只有 SM-2 private internal event 可以 seal
binding；`RequireFreshValidation` 保证 repair receipt 不授予 terminal validation credit。prospective semantic repair 的 purpose
由 exact `validation_failure_repair` manifest/binding 证明；execution-locked binding 的 owner-confirmed repair 则在 pre-seal
封存 `confirmed_validation_repair` commitment，以 `RepairOwnerModuleID` 绑定同一 owner。二者均由同一 Orchestrator owner
消费完成证明并要求 fresh validation，不是两套状态管理。该内容只属于 C3 volatile
authority，相同 Definition 的普通/repair generation 必须保持 C0～C2 与 stable prefix 完全一致。software-dev Profile 将该
owner specialization 为 File 只是 Profile 事实，不是 Core reducer 分支。

当 exact prospective route 声明 `RequireInputAddition` 时，generation input 只能来自已提交 owner receipt 中与原 binding
匹配的 `InputAddition`，不能从 diagnostic 文本推断 target/check。manifest/hash 与 provider C3 保留该 bounded typed
合同，schema/document admission 将 `prospectiveTargetRefs` 限定为它的一个 exact target；额外/替代 target 在 owner probe
前拒绝。原 native check 和 Definition 不可替换；repair receipt 仍只触发 SM-2 内部 fresh-validation reopen。
新 binding 只能来自重新观测后的 owner applicability，不得把旧 observation 解释为已通过新增检查。
`InputAddition` 可携带成对的 owner-authored `contentContractRef + contentRequirements`，限定新增输入的完整内容与
观测合同；它们是有界、hash-bound、provider-safe 的声明，不是 raw diagnostic 或 Runtime disposition。Core 只绑定并
透传到 volatile C3/C4，接收验证 owner 独立解释/检查；空扩展不改变旧 JSON/hash 或 C0～C2。

repair receipt 的 SM-2 CAS 必须保留 target-free fresh-validation continuation，覆盖 `semantic-pending`、prerequisite
重获与 successor binding seal 的完整窗口；generic missing-evidence 同步不得在 binding-free 窗口改写 trigger、ID 或恢复
旧 repair targets。fresh-load、SM-3 candidate admission 和旧失败 replay fence 共用 owner-local 完成证明：从 CWI reopen 的
previous binding/trigger、immutable semantic manifest、pre-seal candidate manifest、同一 completed dispatch 的完整
operation/execution/owner receipt/accepted evidence 集合反证 closure 与最终 repair evidence revision。缺失、重复、stale fence、
部分 all-of、改写 closure 或 barrier 均拒绝；该证明只解除旧修复目标对新 validation surface 的限制，不授予 terminal credit。
新的 accepted validation 消费该 continuation 后，SM-2 在同一次 reduction 中清除已满足的 directive 并进入 assessment；
不能保留旧 validation requirements 污染 assessment context，也不能由 candidate adapter 或 SM-4 修补 authority。

每次 semantic owner-private event 完成 CAS 后，调用方必须仅从返回的 current aggregate 重新读取 phase、generation、
manifest、accepted order 与 request identity。若该 event 已退休 generation 或清空 manifest，本轮 semantic transaction
立即返回 aggregate 交给 owner drive 重新分派；不得继续使用 CAS 前的 manifest/hash/request 假设，也不得依赖 panic 或
外部 watchdog 触发后继。

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
  administrative termination 的 `RuntimeTerminationRecord` 也只属于 handoff ledger；SM-4 不直接写 Run、Workspace 或 Result。
- terminal delivery 的稳定顺序是 `Result -> Workspace -> Run`。任一步失败仅为 delivery pending；
  不得改写已 sealed 的 SM-2 terminal，亦不得以投影失败把 Run 反向置为 failed。
