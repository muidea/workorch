# Runtime Orchestrator Module

Status: active

Type: kernel-module-implementation-contract

Last Verified: 2026-09-11

## 功能范围与边界

`kernel/orchestrator` 是 Runtime SM-2、SM-3、SM-4 的唯一 owner。它在一个 persisted Runtime aggregate 和一个 CAS 边界内
完成 authority/focus convergence、semantic proof、candidate transaction、sealed dispatch、receipt/evidence judgment、
governance continuation、bounded recovery 和 terminal delivery。它不拥有 Workspace、Run、Result、Decision、Approval 或
capability resource state。

| 状态机 | 本模块拥有 | 本模块不得 |
| --- | --- | --- |
| SM-1 Run lifecycle | 无；只消费 Run owner notification/ack | 创建 Run、选择 lifecycle target、持久化 Run terminal 或 successor |
| SM-2 authority/focus | focus、CWI、binding、prerequisite、evidence credit、assessment、repair、terminal decision | 调 provider、执行 capability effect、修改其它 owner state |
| SM-3 candidate/dispatch | request session、document admission、attempt 0/1、candidate proposal、owner conformance、feasibility transaction、sealed dispatch、receipt transaction | 选择 next action、绕过 seal、把 LLM/owner fact 当 transition command |
| SM-4 drive/recovery/delivery | bounded drive、singleflight、lease、budget settlement、startup recovery、Result→Workspace→Run delivery ack | 从 projection/Eventstream/text 重建 authority，或代写 SM-1 |

规范入口是 [`docs/00-core/runtime-stage-contract.md`](../../../../docs/00-core/runtime-stage-contract.md)。专题语义分别位于
[`state-machines.md`](../../../../docs/00-core/runtime/state-machines.md)、
[`semantic-convergence.md`](../../../../docs/00-core/runtime/semantic-convergence.md)、
[`candidate-and-dispatch.md`](../../../../docs/00-core/runtime/candidate-and-dispatch.md)、
[`execution-budget.md`](../../../../docs/00-core/runtime/execution-budget.md) 和
[`recoverable-wait.md`](../../../../docs/00-core/runtime/recoverable-wait.md)。本 README 只固定该规范在 Orchestrator 的 owner、
交互和可执行验收落点，不复制专题全文。

### Single-owner mutation contract

- 外部 concrete result、owner receipt、governance resolution 和 delivery acknowledgement 必须先映射为
  `RuntimeInternalEvent`。只有 `RuntimeController.ApplyInternalEvent` 可以分派 mutation；handler 为 package-private。
- private event 只允许 `Activation | Authority | Transaction | Delivery` 四个 concrete payload family，且 kind 与非空 payload
  一一对应。missing/wrong/multiple payload、scope/identity drift 在 aggregate load/CAS 前拒绝。
- 每个 private event 只有一个 primary SM 分类；若同 owner 的一次事实提交需要同时协调 SM-2/SM-3/SM-4 ledger，其完整写集必须
  留在一个 package-private transaction 内，并由一次 aggregate CAS 与 cross-machine fence 原子验收。不得把同一事实拆成多个
  EventHub ingress、多个 CAS 或由某个 sibling state machine 事后补写。
- `runtimeMutationKernel` 唯一持有 repository port 与 `RuntimeTransitionCoordinator`；authority、planning transaction、
  candidate dispatch、drive delivery component 只共享该 kernel，不持有 sibling、Hub 或第二份 aggregate。
- `RuntimeAggregate.Validate` 顺序验证 SM-2、SM-3、SM-4 及 cross-machine fence。所有合法变化经一次 semantic CAS；
  projection、timer、adapter 和 shared contract package都不能直接写 aggregate。
  private transition 完成写集后直接提交同一 aggregate 时，由 `RuntimeTransitionCoordinator.Commit` 统一执行完整校验，
  不在无中间变更的相邻调用处重复验证；revision fence、普通 repository Save 与 fresh-load 的原始校验仍保留。
- 纯校验只可在同一次调用内复用本地规范化值：SM-2 binding hash 保持原字段、顺序和完整内容重算；SM-3 session
  可把本次完整 raw-canonical/source/hash 校验得到的 detached manifest 交给私有 wire compiler，不能仅凭调用者 hash
  推断成功。semantic assessment 前像重建可将同一 generation 完整校验所得的私有 manifest 产物复用于逐批 binding compiler，
  每批 document、request identity 与 owner feasibility 仍完整反证；公开 compiler 每次重新校验，
  `runtime_semantic_binding_validation_reuse_test.go` 固定产物一致性、逐批拒绝和输入不变边界。
  binding compiler 只读本次完整校验建立的 coverage/obligation 索引，仅为当前批次构造 detached source；私有 focus compiler 消费同一 validated manifest，不再次规范化整个 generation。document、proof obligation、owner feasibility 与 reconciliation 的完整校验保留，公开入口和后续调用仍重新验证；`runtime_semantic_binding_indexed_reference_test.go` 保留旧 compiler 路径，固定完整产物、拒绝原因、返回隔离与重载等价。
  accepted batch adapter 使用 `CompileRuntimeSemanticAcceptedBatchBindings`，在合同包内同次复用完整 manifest 校验产物，
  仍校验原始 batch identity/cardinality/hash、精确 accepted document 及完整 owner facts；
  proof-batch binding compiler 复用本次完整 document 校验中已执行的 obligation gate，不对未变正文重复整份校验；
  原 source proof、owner feasibility、input review 与 reconciliation 门禁保留，其它 step 仍走既有路径。
  `runtime_semantic_document_binding_reuse_test.go` 固定独立旧 compiler 产物／拒绝等价、逐身份反例及 fuzz；不增加跨调用缓存。
  `runtime_semantic_accepted_batch_binding_test.go` 固定产物等价、协调篡改拒绝及可重复成本对比。
  assessment-purpose 的 archived batch owner-result 校验复用本代同次完整 manifest 校验产物；batch、document、result identity 与完整 owner feasibility 仍逐项反证，公开入口仍重新验证。
  `runtime_assessment_result_validation_reuse_test.go` 对照公开 result validator 固定接受／拒绝等价、原始 ledger 字节不变和返回副本隔离。
  公开 manifest、wire、session 与 fresh-load 入口每次仍完整反证，不持久化成功凭据、不增加 caller 已校验标志。
  accepted candidate lineage 在同次完整 raw-source/hash 校验后保留私有 detached manifest，candidate admission 仍逐候选
  完整执行，再复用本次 manifest 派生 target/recipe 投影；attempt 与 dispatch 两侧身份、内容和 hash 仍独立反证。
  `runtime_candidate_lineage_validation_reuse_test.go` 固定私有产物深拷贝、公开拒绝等价及后续调用重新验证。
  accepted lineage 同次复用完整校验的 detached manifest，不再次 Clone session manifest 或重验未变 binding；
  attempt 与 dispatch 两侧仍独立规范化、比较完整 binding 正文与 snapshot hash，不以自洽 hash 代替来源反证。
  proof/binding Clone 复用自身 Normalized 的 detached 产物，保持既有 nil/empty 语义；
  `runtime_coverage_proof_clone_reuse_test.go` 对照旧实现验证原值、嵌套隔离、重载与分配成本。
  effect scope 和 operation variant 集合的 canonical 快路径仍返回 detached 副本，保持旧排序、去重及 nil/empty 表示；
  `runtime_proof_canonical_collections_test.go` 用独立 reference 与 fuzz 校验非规范输入和返回隔离。
  hash 字符串编码合并同次长度前缀与正文写入，不改变 big-endian 长度、集合 cardinality 或任意原始 bytes；
  `runtime_hash_string_encoding_test.go` 固定无效 UTF-8、NUL、长原文与重复 hash，并以 fuzz 对照旧编码。
  semantic proof compiler 的 raw owner catalog 集合门禁保持完整，仅复用本次 detached 目录索引解析 prospective all-of 约束，
  不重复校验未变的整份目录；公开 resolver 仍完整校验，每次 compiler 调用重新反证 raw、重复 owner/surface、hash 和 exact requirement。
  `runtime_semantic_proof_catalog_reuse_test.go` 固定公开／私有 resolver 等价、完整产物、原值、重载及后续调用拒绝。
  semantic owner catalog 校验直接复用同次完整 raw／嵌套 proof／snapshot 校验产生的 detached 规范化目录，
  不再次规范化未变的整份目录；重复 prerequisite／owner 门禁保留，公开 `Validate` 与后续调用仍完整重验。
  `runtime_owner_catalog_validation_reuse_test.go` 与独立旧 validator 固定产物、完整拒绝原因、原值、重载、隔离、fuzz 和分配成本。
  assessment authority hash 保持原 Normalized、JSON 和域前缀编码，复用同次 marshal 缓冲，不增加跨调用缓存；
  `runtime_assessment_authority_hash_test.go` 以旧编码 reference、JSON 重载、fuzz 和分配对照固定等价边界。
  静态 operation descriptor 的内部证据资格查询只读初始化库存，不复制随后丢弃的 `AllowedStates`；公开 getter 仍深拷贝。
  `runtime_coverage_evidence_descriptor_lookup_test.go` 以旧 getter 路径为 reference 覆盖全部 kind、未知与非规范输入、重复调用和公开副本隔离；不减少任何 proof、raw 或 aggregate 校验。
  `runtime_coverage_evidence_binding_hash_test.go` 和 `runtime_candidate_session_validation_test.go` 固定 hash、重复调用、
  同版本重载、来源篡改与深拷贝边界。binding projection 在同次完整 raw-canonical 正文／hash 比较后复用 detached
  规范化正文，仍逐项验证原始 proof、重复身份、目标与 policy；公开 binding 校验与后续调用重新反证，不保存成功缓存。
  `runtime_binding_projection_validation_reuse_test.go` 以旧校验路径固定接受／拒绝原因等价、原值、重载、fuzz 和分配成本。
  已规范化字符串集合仍返回 detached 副本，非规范输入保留 trim／去重／排序语义，
  由 `runtime_normalize_strings_test.go` 的 reference 与 fuzz 固定。SM-3 证据投影先反证原始 accepted/rejected exact cardinality，
  再仅规范化匹配的单条证据，并逐次检查 exact processed-event fence；不复制无关 Facts，也不缓存成功结果，
  由 `biz/runtime/evidence_projection_normalization_test.go` 与既有 fresh-load 账本篡改测试固定。崩溃注入 wrapper 保留 production Coordinator 的完整 aggregate gate 和
  `SaveValidated` 写入路径；CAS 前后切点、普通 `Save`、失败回执与非法 aggregate 拒绝由
  `biz/runtime_crash_repository_test.go` 固定。SM-1～SM-4 的 transition、authority、预算和恢复规则均不改变。
- SM-4 终态材料保全 persisted `CoverageProofLineage`，owner-local adapter 与独立重载产物一致。
  requirement evidence 只把 exact current binding/proof、完整 all-of 和唯一 equivalent lineage 来源列为 current accepted；
  历史不适用项保留为 `stale_or_superseded`，不回写 SM-2 coverage/assessment，不改变 SM-1/SM-3 transition。
  `biz/runtime/coverage_proof_lineage_test.go` 固定 adapter/reload 一致；
  `runtime_terminal_evidence_applicability_test.go` 固定 binding/hash/scope 漂移、弱证据、完整 proof 与重复／冲突 resolution。
- `Facts.Failure` 的替换只能经过 `replaceRuntimeFailureAuthority`，并必须显式选择三种 policy 之一：
  `runtimeFailureAuthorityRetainInterlude` 仅在同一原子事务的有界中间态保留既有 target/directive；
  `runtimeFailureAuthorityRetire` 同时清除旧 `FailureTarget + RepairDirective`；
  `runtimeFailureAuthorityReplaceFromExactOwnerFact` 只接受与新 failure signature/targets/observation exact-match
  的 owner fact，并先退休旧 directive。任何普通 failure 写入不得隐式继承 repair authority。
- `internal/pkg/workorchruntimecontract` 只包含 DTO、enum、normalize/validate/clone/hash、manifest compiler 和纯 matcher；
  不持有 Hub/I/O/CAS，也不选择 retry、terminal 或 Runtime action。

### 原回执的实际读取依赖

capability owner 可在 `RuntimeOperationEvidence.OwnerRevisionSet` 返回本次实际读取的 Definition-bound 依赖。
SM-3 receipt commit 对非空集合执行完整 raw-canonical、集合/hash 及 exact dispatch/aggregate Definition 校验；
receipt identity 绑定完整依赖正文，而不只绑定调用方提供的 `SetHash`。SM-2/SM-3 私有 evidence transaction 仅从
persisted receipt 将该集合 detached 投影到 `EvidenceFact`，fact identity 同样绑定完整正文。

raw clone、receipt/fact normalization 和当前 schema JSON reload 保留原依赖表示，不通过规范化清洗非法输入。
fresh-load、accepted/rejected lineage、confirmed repair、implementation-work receipt members 与 lineage guard
反查原 receipt 的 exact payload；同步重算 fact hash 不能绕过该来源反证。duplicate 不增加 CAS、evidence revision
或 credit，错 Definition 的提交零变更拒绝。`biz/runtime/owner_read_dependencies_test.go` 和
`workorchruntimecontract/runtime_receipt_owner_dependencies_test.go` 固定原值、重载、隔离和篡改反例；真实 File bytes
producer 由 File owner 的独立回归验证。

此合同不新增 SM-1～SM-4 transition，不用 nil 集合伪造无依赖或当前有效证明，不开放 capability availability。
File existing-read 是该事实的具体 producer；它不替代 assessment/artifact 的实际依赖、current owner observation、
相交资源失效/reopen 或 terminal 的真实 dependency union。终态 compiler 的 synthetic `runtime_authority` 集合
不是 actual-read 来源，该来源问题不能由一个 optional 字段或纯 applicability matcher 自动解决。

### 逐源义务与共同 CWI

Workspace eligible candidate 的 immutable `RuntimeSourceObligationInventory` 随 Definition/seed/startup/hash
封存；Application 只编译 exact source checks，不创建运行期 authority。每项 verification 的 check identity
绑定完整 inventory、Definition、CWI、ObligationID、RequirementID、VerificationID 和原 Criterion/Declaration。
同 CWI 的全部 source checks 与共同检查进入原 validation requirement 的 all-of 集合，不增加可变闭包 ledger。
Workspace compiler、Runtime bootstrap、fresh facts 与 semantic request 逐次反查 exact
`ProfileID + EvidenceRequirementID + Scope` 及每个 DoD source 的合取声明贡献；库存自洽 hash 不免除适用性校验，
声明存在也不代表 owner readiness 或 proof。`runtime_source_obligation_applicability_test.go` 覆盖缺失、重复、错源、
无 proof operation、wildcard 冒充 exact recipe 与同版本重载；activationcore 回归独立证明启动／fresh facts 的拒绝落点。

SM-2 仍独占 current binding、evidence credit、assessment、reopen 与 terminal；SM-3 持久化完整输入评审和
封存 dispatch，V2 每 source check 必须使用该源 Criterion 而非共同 CWI 或另一义务的文本。
SM-4 从 aggregate 派生完整源 closeout 并幂等交付；SM-1 lifecycle 无新增 transition。
纯 compiler 从 closeout inventory 重编逐源集合，current proof 缺 B、旧 binding 或 CWI 重开不能恢复 credit。
Result/UI 只读投影，不执行 proof 或重裁成功；当前 all-of 投影不报告独立部分源 credit。

`biz/runtime/source_obligation_test.go` 校验 receipt→assessment→terminal、A-only 拒绝与重载；
`biz/runtime_source_obligation_owner_mainline_test.go` 使用实际 File/Shell、Run CAS 和三 owner 交接，provider 为脚本替身。
各交付 owner 持久化后 ACK 丢失从独立 file-backed 前像恢复：保持原 seal、逐源 outcomes、ResultID 和 ordered ACK，
重复投递不增加 provider 消费；B 失败的 rejected closeout 也保持无逐源 credit。
Shell v5 多检查输入包合同归该 capability owner，不在 Core 硬编码 native grammar、软件栈或 adapter identity；
这些回归不代表公开 authoring、非 managed proof、真实模型或 release 验收。

`biz/runtime_execution_handoff_barrier_test.go` 验证 SM-1／SM-4 交接屏障：真实 Workspace handoff 已持久、
slot 为空而 Run 尚 active 时，合法 Restart 只能保留原 Run 的 replacement intent，不能首次激活第二 Run。
无 predecessor 的同请求正例可激活；空 slot Resume 不创建 Run，竞争 request identity 拒绝且不改持久前像。
owner 重启后的原 request 重放仍守住屏障，恢复只交付同一 seal／Result 和有序三个 ACK。
该回归不驱动后继 materialization，也不证明首次 activation 的手动／自动语义去重或真实 provider 验收。

### Semantic and candidate authority

- 输入维护的可执行范围以 Shell 的 [input-maintenance contracts](../../blocks/shell/readme.md#executable-input-maintenance-contracts)
  为准：已有 addition/repair 合同不授权通用验证实现替换。检查身份一致、普通 approval resolution 或外部测试通过
  都不能替代 LLM 对原准则与新验证实现的一致性评审；评审要求见下节。这不增加审批豁免或第五个状态机，也不允许
  通过泛化 repair route 绕过不可用的证明面。
- 普通 state-change 的输入保护由 capability owner 的 `InputTargetOwnerModuleID + InputTargetRefs` 事实经同一 manifest
  compiler 形成 `ExcludedTargetRefs`；三种 target policy（含 selector-only）共同执行。manifest 保留完整
  `MutationInputCatalogs` 前像，accepted attempt、dispatch target authorization、hash 与 restart fence 不得丢失或改写
  排除集合。只有 exact confirmed-validation `InputRepair` 可开放自己的单一 target，其余输入不开放；create-only addition
  与合法实现修改仍走原事务。Core 不解析文件名/语言，capability owner 不选择 SM-2/SM-3/SM-4 transition；SM-1 不变。
  这是普通修改准入，不是 LLM semantic review；越界重复只走既有 bounded candidate failure，不能制造治理 ticket。
- 验收扩展只允许与原 Goal/DoD 一致的补充和细化，并与原义务合取；不允许通过 approval、输入版本或外部参考
  覆盖原准则。测试实现不等同于验收准则，修改输入不能从失败本身推导授权。shared contract 的
  `RuntimeValidationInputAddition.MatchesBinding` / `RuntimeValidationInputRepair.MatchesBinding` 对完整原始
  binding 执行唯一映射和别名冲突校验，不能在 normalization 去重后或首条匹配后认可输入维护。
  该检查仍由既有 SM-2/SM-3 owner-private transaction 消费，不新增 event、状态机、预算或组件写入入口；
  SM-1 与 SM-4 不变。`runtime_validation_input_identity_test.go` 覆盖独立补充、重复/替换/别名、排列、重复调用及
  序列化重载。身份一致是必要条件，不是自然语言语义一致证明；未有适用性证明的版本不得发布可用 proof。
- activation 只消费 Run owner 已封存的 `RuntimeBootstrap`。Definition startup input、Profile contract、provider selection、
  Soul/Skill/MCP/Memory snapshot 和 `StartupInputHash` 在既有 Run 中不可改写。
- semantic repair 请求的 C4 execution feedback 由本 owner 从 CWI reopen trigger、retired binding、sealed dispatch、
  committed execution 与 rejected owner receipt 反证后只读投影；不从 mutable failure 或最近 evidence 猜测来源。
  在新建 provider session 前核对来源，缺失时走既有 SM-2 `SemanticContractFailed` 私有事件，不伪造 physical call
  或 budget settlement。SM-3 仍独占请求/dispatch 事务，SM-4 只驱动读取，SM-1 不变；没有新增主写字段、状态或额度。
  同版本 fresh-load 重建 exact feedback，跨 owner 只通过原 typed semantic context；LLM 只渲染和校验 C4，不能选择动作。
  `biz/runtime/semantic_execution_feedback_test.go`、LLM feedback tests 和真实 File/Shell input-addition 主线覆盖来源、
  错源/重复/篡改、无来源零请求、稳定 prefix 与重启，不能据此声称一般 CWI 开发循环或真实模型目标完成。
- Profile identity 的共享纯合同必须保留全部声明供准入拒绝，recipe、required checks、metric、artifact、owner-fact proof identity、capability-state
  owner/kind/subject 均进入封存 hash。SM-2/3/4 消费完整当前 Profile，不从缺字段的旧 hash 推断等价；该防御检查不新增
  transition、operation、预算或 owner。验证见 `workorchruntimecontract/runtime_profile_identity_test.go`。
- required CWI 在 operation candidate 前必须由 SM-2 从 immutable Definition seed、typed owner catalog、SM-3 semantic document
  与 matching owner feasibility 封存唯一 `CoverageEvidenceBinding`。不存在 binding-free operation authority。
- 每个已 admission 且要求 validation 的 exact CWI 必须由 Application 本地纯 compiler 绑定 Definition revision、
  CWI/source identity、source-text hash 与 canonical exact declaration，形成 recipe-neutral
  `RuntimeValidationProofRequirement`。Profile wildcard 只是由 Application 施加到已 admission exact CWI 的声明模板；它可收紧
  quality、target/selector、owner-fact acquisition、repair 与 evidence/operation 上界，但不能贡献 current owner fact、recipe/native check、dispatch 或 credit；capability
  owner 从当前 typed ready catalog 选择兼容 recipe 并返回 exact per-CWI applicability binding。semantic provider 不能编写
  declaration/check，SM-2 也不能把 recipe ID、generic exit 0 或另一个 CWI 的 marker 提升为 proof。
- capability owner 可对尚缺 native check/input 的 exact requirement 返回 revision-bound preparation；它不是 Runtime authority。
  preparation 可携带 owner-authored `OriginalDiagnostic`，其 owner/revision 必须匹配 exact catalog，并进入 snapshot hash。
  SM-2 在既有 owner-fact exhaustion internal event 中把唯一 exact-current 原因封存为 terminal 的 audit-only cause；
  原因文本/code 不选择 transition、不增加 acquisition budget。fresh-load invariant 拒绝 owner、definition、requirement、
  catalog revision 或原文漂移，旧记录缺少该可选字段时不补写。SM-4 Result materializer 与 Application 只 detached 透传。
  只有 Profile 已封存的唯一 owner-fact acquisition/reobserve convergence 能使 SM-2 建立 receiving-owner prospective
  transaction；exact acquisition receipt 会消费对应的 create-only `TargetSelection` lifecycle identity，resource-change
  invalidation 不得重新激活或重发该 selector；receipt 后仍须 capability owner fresh reobserve、SM-2 私有 seal。Orchestrator 不解释 native protocol，
  Application 不自动生成跨 owner convergence，MCP typed validation 也不借用 Shell preparation。
- 不能通过 declared prospective acquisition 补齐的 exact validation requirement，由 capability owner 单独发布
  `ProofRequirementUnavailability`，不能伪装为 preparation。SM-2 在 current prerequisite 结算的既有私有事件/CAS 中，
  以 `owner_fact_convergence_exhausted` 封存原始原因；先于 semantic request、repair directive 与新 acquisition。
  binding-free cursor 只按 exact CWI 读取 immutable Definition owner-fact declaration，不回填 focus/binding。
  fresh-load 重验 requirement/catalog/owner/diagnostic；缺失或漂移 cause 拒绝。SM-4 只按既有 Result→Workspace→Run
  交付，无额外 provider/operation、acquisition/recovery budget 或新状态机。
- Goal/DoD、prospective constraint、required sequence 和 collective receipt 只支持 conjunctive `all_of`；不存在
  `one_of`/`any_of` terminal escape。Definition gap 只能进入专用 clarification transaction。
- provider-facing manifest、closed JSON Schema、strict decoder、single evaluator 和 owner feasibility 共享一个 sealed
  `RuntimeCandidateContractManifest`。Orchestrator 从它纯编译 `RuntimeCandidateWireContract`；required slot 仅由
  `requiredOperationRef` 选中，auxiliary rule 仅由 canonical kind 选中。唯一 reachable variant、所有 reachable
  authorization 共有的 singleton binding、required cardinality-one singleton target 和 fixed parameter 均本地派生；
  unreachable variant authorization 不扩大 wire。LLM wire 不包含这些本地值或 operation ID/dependency；本地 compiler
  唯一派生完整 DTO、identity 与 graph。
- owner catalog 可提供 typed target-parameter preimage。SM-3 将完整 bounded existing target literal 或 prospective 无
  preimage 声明封入 manifest/wire/hash，并在选定 singleton target 后纯派生 exact 参数；provider echo 必须拒绝，owner
  ingress 仍重验 catalog/freshness。Core 不解释参数的业务含义，不重新观察资源，也不新增 owner/correction authority；
  nil/empty 扩展保持旧 archive identity。动态声明只影响 C3。
- operations ExpectedDocument 持久化 exact `WireContractHash`，request session 保留 manifest identity；任一 framing 为单一
  JSON object 的 completed document
  进入 accepted 或 rejected candidate attempt 时，都在关闭该 attempt 的同一个 owner-private CAS 中封存 exact
  `CandidateManifest` body。prepared/recovery restart 从 current sealed authority 重编译；
  attempt 0/1、duplicate 和后续 fresh restart 从 exact body 重算。编译错误、缺失或漂移在 provider 前 fail-closed。
  Orchestrator 仍是 admission/attempt/retry/seal 唯一 owner；LLM block 只渲染该合同并返回 opaque document fact。
- Orchestrator 的 typed candidate command保留完整owner facts用于LLM owner的本地integrity/freshness核对；这不授权把raw
  admissibility catalog、prerequisite ledger或无关capability展开给provider。operations C3的operation vocabulary仍只来自
  sealed manifest，supporting context由LLM owner按该manifest有界投影；任一manifest snapshot ref无法绑定同一canonical
  typed fact时，LLM owner必须在physical submission前fail closed并返回事实，不得修改SM-2/SM-3 authority。
- core reducer 只读取 typed feasibility/status、sealed operation descriptor 与 Profile declaration；capability owner 的私有
  diagnostic code/prefix只作为 original diagnostic 保真，不能选择 failure class、repair owner、retry、governance 或 terminal。
- validation-failure convergence 的 `Routes` 是 exact trigger lookup table，不是 terminal alternatives。SM-2 用完整
  `(owner, operation, diagnostic class, evidence target type, owner code)` 查唯一 route；匹配后只编译该 route 的唯一 owner
  catalog/surface。无 route 时，在提交 rejected receipt 的同一个 private event/CAS 中退休 repair authority并封存
  `validation_failure_repair_route_unavailable`，零 semantic/candidate provider、capability effect、governance 或 SM-4 wait。
  structured observation 只有匹配 route 才进入 owner confirmation；`existing_owner_scope` route 缺少 observation 时在同一 receipt
  CAS 封存 `repair_surface_unavailable`，不得调用 semantic provider。只有 `prospective_owner_target` exact route 可在无 observation
  时进入 bounded semantic repair fallback。repair-purpose provider document 返回 Definition gap 时同样走
  `repair_surface_unavailable` private terminal，不得创建 Definition clarification ticket。observation diagnostic hash 绑定完整
  route 字段与 sealed validation scope，并在 receipt admission/fresh-load 从 immutable owner receipt 重算。该 scope 只能来自
  matching operation execution 的 `AdmissibilitySnapshotID + AcquisitionBinding` 所绑定、已随 dispatch 封存的完整 Shell-owner
  catalog `TargetRefs`；per-operation target authorization 只裁决 validation entry，不能代替 diagnostic scope。
- 验证输入补充仍由同一 SM-2 receipt/reopen/seal 私有事件处理：exact prospective Profile route 的
  `RequireInputAddition` 要求 owner typed `InputAddition` 精确匹配 completed dispatch 的原 required/native binding，且没有
  existing supplement。缺失/错配同一 CAS 终止，不回退任意文件修复。SM-3 schema/proposal 只允许一个 exact new input；
  创建只结算中间 state-change，必须 fresh owner catalog → 新 binding → 原/补充检查 all-of 才可获 validation credit。
  owner-authored `contentContractRef + contentRequirements` 只作为 bounded typed value 进入 input/hash 与 C3/C4，不由 Core
  解释语法；既有补充输入无效时不允许新 acquisition 穿过 fresh-validation 的 Definition-proof seal guard。
  SM-1 不变；SM-4 重启/投影不能推断该合同。`validation_input_addition_test.go` 覆盖 duplicate/stale/restart、无效合同、
  exact target seal 与创建零 credit；`default_native_validation_integration_test.go` 校准真实 File/Shell receipt 和 matcher。
- criterion-preserving 输入修复复用 confirmed-validation transaction：Profile 的独立 `RequireInputRepair` route 必须匹配
  rejected receipt 的 typed `InputRepair`、原 required/protocol/native/supplement binding 和唯一 input target；普通实现 route
  不可借用该事实。SM-2 初次 decision 与后续 directive 均按 exact route 限制 kinds，不从 capability union 扩权。
  File 确认 existing regular target 后，SM-3 manifest 封存同一 bounded delta，最多一项 operation；File feasibility 的
  ReplayProof 必须证明 exact before/after hash。manifest、pre-seal commitment、source receipt 与 execution proof 在
  final seal/fresh-load 逐一核对，缺失、替换或放宽均拒绝。repair 零 validation credit，仍由原 SM-2 私有事件转 fresh all-of；
  不增加 repair/recovery budget，不修改 SM-1、旧归档或 sealed startup，SM-4 只恢复该 transaction。
  `biz/runtime_validation_input_addition_mainline_test.go` 的 compile-repair 场景覆盖真实 File/Shell、5 个 crash cuts、
  零额外 provider 调用与终态归档 tamper；`biz/runtime/validation_input_repair_test.go` 校准 exact feasibility proof gate。
- candidate proposal 持久化后，SM-3 必须按 manifest-declared `CandidateConstraints` 的 kind 获取 exact owner fact。
  constraints 是 operation-level 与 selected variant-level 的合并合同；无 variant 的 operation 同样受约束，不能创建虚拟 variant。
  已声明 dynamic constraint 的 operation 在缺少 owner admissibility 时禁止走 static capability seal lane。
  `parameter_shape` 在 ordinary feasibility 前通过 typed conformance command 完成，owner 只复用自身 parser/matcher 返回封闭
  outcome/shape且不读写 workspace；`target_applicability` 只能由 ordinary typed feasibility 对 current target/revision 返回
  `candidate_constraint_violated`。两者均以 `attempt + LogicalRequestID + operation + manifest constraint` 持久化 restart fence；
  `nonconformant` 或 applicability violation 与 attempt closure 必须在同一个 private event/CAS 中原子提交，
  `owner_contract_defect` 直接 fail closed。没有声明相应 constraint 的 operation 不发 placeholder call或开启 correction。
- failed attempt 保留 feasibility fact 时，同一失败 CAS 必须保存 exact owner catalog preimage。历史 reverse proof 使用
  sealed manifest snapshot ref + catalog self hash + operation/owner/acquisition/revision；不得读取新 catalog 重建旧拒绝。
  pending attempt 仍只认 current catalog；历史 preimage 不进入 admission、provider projection 或 dispatch authority。
- SM-2 ordinary infeasibility failure 与后继 `equivalent_authority_no_progress` terminal 必须保留 exact matching
  invalidated attempt 的 `OriginalDiagnostic`。guard code 与 owner cause 是独立事实；保留诊断不授权额外 provider、
  correction、repair 或 successor。Application/TUI/Web 只能投影原始 owner、code、target 与 message。
- ordinary owner infeasibility 在当前 binding purpose 为 `validation_failure_repair` 时不得走会清空 directive、保留 binding 的
  通用 failure 路径。Orchestrator 必须在同一个 private event/CAS 中 invalidates candidate、以 exact owner observation 退役该
  intermediate binding，并形成有界 `repair_surface_unavailable` terminal；duplicate/restart 不得重复退役或消耗 effect/budget。
- JSON Schema response format 是可选 transport acceleration。format on/off 都必须经过同一 local decoder/evaluator；format
  failure不授权第二次无 format physical call。
- incomplete/empty/invalid framing 是 no-candidate fact，只使用 request-session whole-document recovery，零 candidate attempt/C4。
  只有 complete-object schema/contract 与 semantic rejection 共用 attempt 0/1；只有 persisted typed correction witness 可开启
  attempt 1，且 attempt 1 任意再次拒绝 final。transport/document recovery、owner feasibility、repair 与 governance budget不得
  借用该 attempt ledger。
- manifest v7 静态约束普通 durable candidate 的 `(OwnerModuleID, canonical TargetRef)` cardinality 为 1；只有 exact
  `RequiredOperationSequence` 可声明同 owner/target 例外。operation-set correction 必须保持原 owner/target 覆盖与非冲突
  operation 不变，只收敛 exact 冲突组。strict decoder、single evaluator、correction witness 与 final seal 必须
  调用同一 manifest-only 校验，不得根据 mutable facts 重算。
- single evaluator 必须在任何 owner probe 前继续执行 sealed activation 的 executable parameter、per-kind variant 与 effect
  policy fence；final dispatch admission 只重复同一组校验，不能首次拒绝已持久化 proposal。某 kind 的 variant allowlist 是普通
  accepted variants 与 exact required-sequence variants 的并集；其它未声明 variant restriction 的 accepted kind 不受影响。
- owner candidate constraint violation 也只使用同一 attempt 0/1 ledger：C4 仅携带 sealed constraint ref/path、封闭
  actual/required shape及 bounded target observation；attempt 1 在再次调用 provider/owner 前必须证明 constrained parameter value 已
  变化。static shape 只能由新的 owner conformance fact证明，dynamic applicability 只能由新的 current feasibility observation 证明。
  第二次 violation final，不能借 ordinary feasibility/no-progress 另开 retry。
- manifest v7 的 operation/variant `ValueRules` 是 provider-safe 纯合同，不是 owner observation。strict decoder、single evaluator
  与 final validator 在 owner I/O 前复用同一实现；SM-3 只凭 exact rule ref/path 与 closed shape 封存一次
  `operation_value_rule` witness。attempt 1 只允许规则末端参数发生语义变化，还原该参数后必须与 attempt 0 的完整 candidate
  execution-semantic shape 相同，并重新通过全部 selected rules；raw value 不进入 C4，能力 owner、SM-2 与 SM-4 均不取得
  correction transition authority。
- `undeclared_prerequisite` provider document 必须携带 CWI-bound typed `PrerequisiteClaims`。Orchestrator 在 private event/CAS 前只调用
  pure `EvaluateRuntimeDefinitionGapAdmission`，按 sealed proof obligation 与 owner catalogs 区分 already-satisfied、
  declared-with-acquisition、declared-unresolved 和 genuinely-undeclared；只有最后一类能提交
  `RuntimeInternalEventDefinitionGapObserved`。其余分支复用普通 semantic correction ledger，零 governance/ticket/operation effect。
- proof batch 的 `RequestLogicalID` 是 immutable semantic generation identity；attempt-1 只推进 ledger 当前 provider
  request-session cursor。accepted document hash、owner fact、SM-2 seal 与 fresh-load replay 都必须绑定同一 batch identity，
  禁止 seal 从 ledger cursor、最后一个 session 或下游 projection 选择 generation。
- semantic coverage-order、proof-batch 与 correction 私有事件只封存请求意图；统一 request path 在 causal context/content 和
  LLM rendered preflight 通过后才 CAS 创建 session。新请求预检失败由既有 SM-2 semantic failure 终止，零新 session、
  physical call 或预算 allocation；不伪造结算以调用 provider-terminal。精确 persisted session 是同版本预检证明，SM-4
  重驱不重复预检，SM-3 仍重验完整 session/expected-document 身份。prepared 继续首次提交，submitted-without-outcome
  只封存 unknown，document-complete 只本地准入。`biz/runtime_semantic_preflight_test.go` 覆盖真实 LLM 渲染拒绝、三个入口、
  session/submitted/document-complete CAS 前中断和 CAS 后响应丢失；SM-1 owner、现有预算与跨 owner 合同不变。
- semantic ExpectedDocument 由 `biz/runtime/semantic_request_contract.go` 的同一 owner-local 纯投影供 rendering 与 session
  writer 复用；writer 在 fresh-load 后反查当前 ledger logical request、active batch/CWI/order、修正身份与 output/recovery
  budget，新建和复用一视同仁。required/order-accepted/proof-batch-document-accepted 不授权 session，旧预检结果不能
  补写身份后入账。`biz/runtime/semantic_request_contract_test.go` 覆盖 owner 准入、精确 batch 和同版本重启复用。
- accepted order 内 `EligibleFocusProjected` 推进 current CWI 后若产生新的 semantic prerequisite，Orchestrator 必须先通过
  typed owner command/result 与 SM-2 private event/CAS 结算，再从 fresh authority transaction 编译下一 proof batch 和
  `RequestLogicalID`。pending 窗口不得创建 request session、调用 provider 或扣减 budget；provider 前 guard 只是 fail-closed
  防线，不能替代 batch identity 之前的 owner-fact convergence。
- missing/empty required field 必须 schema-first 封存 `field_absent -> field_present`；required field 已存在但为 `null` 或错误
  JSON type 时封存 `<actual_type> -> field_present`，并由 diagnostic 的 `Required`/expected type 证明 schema rule。两类情况都不能
  把 strict decoder 的 field/type vocabulary 当作 target/binding alternative。strict decoder 一旦选中 operation wire rule，必须把
  `wireContractHash + operationIndex + requiredOperationRef|operationKind + variantId` 作为
  `RuntimeCandidateWireRuleIdentity` 随 diagnostic 封存；SM-3 只能用该 exact identity 选择 manifest operation，禁止从 `Actual`
  或单 operation manifest 推断。failed attempt 的 diagnostic/correction pair 在 CAS 与 fresh load 都校验 manifest body、wire rule、
  objectives、objective↔witness-dimension、path、actual/shape；attempt 1 必须让完整 document 重新通过 strict typed admission，
  任何 lineage mismatch 零 provider/dispatch/effect。
- fixed declared parameter 被错误放进另一个 `parameters[]` entry 时，SM-3 只封存 provider-safe
  `parameter_entry_encoding` facts 与 `reencode_parameter_entries` objective；不得把合法 parameter 解释为 forbidden-field removal，
  不得本地改写原文后 admission。attempt 1 必须把全部 misplaced parameter 形成独立 named typed entries并重新经过同一 strict
  admission；diagnostic/witness 的 index、kind、container、name、type、manifest 或 hash 任一漂移都在 provider/effect 前拒绝。
- accepted candidate 在 effect 前必须同时持久化 candidate attempt、admission target/binding/recipe commitment、
  `SealedDispatch`、operation execution key/fence 与 `DispatchAuthorityHash`。fresh load 从原始 manifest/document/owner fact
  preimage 反证，不从 current mutable facts重编译。
- assessment purpose 在候选构建和历史 accepted-attempt 校验中共用纯合同解析：先唯一 exact-match binding preimage，再解释该
  obligation 的用途。合法的 current/history repair generation 不污染另一证明的 assessment；匹配 repair-only 的 assessment
  仍拒绝。index 仅 invocation-local，不新增状态机、owner、持久化字段或归档回填，SM-2/3/4 仍只消费本 owner private event。
- semantic repair 的 target-free fresh-validation directive 同时覆盖 binding-free 与重新 sealed 窗口。SM-2 不得在前者用
  generic missing-evidence 覆盖它，在新 validation accepted 后也不得继续保留它；该 evidence CAS 消费 handoff 后才进入
  assessment。SM-3 admission、replay fence 与 aggregate fresh-load 共用 `semantic_repair_continuation.go` 的旧 binding、
  candidate manifest、完整同-dispatch receipt/evidence 反证；旧 closure targets 只留作因果审计，不约束新 Definition 验证目标。

semantic request 的 `CoverageProgress` 由 Orchestrator 从当前 aggregate 纯投影：总量使用完整 sealed CWI inventory，
明细仅限当前 manifest/batch，并保留 scope、state/evidence revision 与 hash。当前证据引用数、重开次数不代替
SM-2 的验收状态或 fresh proof。请求前检查 owner facts 完整性，非法来源沿既有 SM-2 私有 failure 处理，不创建新
provider session/allocation。LLM 仅负责 typed 校验与 C4 完整渲染；SM-4 重启重新读取同一 owner，不存第二份账本，
SM-1、原准则、dispatch/repair/预算权限不变。

同一请求的 `ExecutionHistory` 只从本 owner 验证过的 aggregate/receipt ledger 投影最近 16 条已提交回执，含总量、
省略数、提交序号、原 binding CWI、owner/kind、receipt fence/hash、target/effect/changed 与 typed validation 事实。
全量来源先检查，再选择有界尾部；不能忽略窗口外坏记录。已完成且不在当前 batch 的原 CWI 仍可作为历史工作观察，
但 bound CWI 和 owner accepted 不产生当前 credit，也不能替代 exact repair origin。无回执的在途/unknown execution
不在此计数内，不授权重试。所有值仅进入 C4 并逐值校验；SM-3 新 session 前的来源失败沿原 SM-2 semantic failure
处理，SM-4 仅重投影，SM-1 不变；无新增账本/状态/预算/操作权限。`biz/runtime/semantic_execution_history_test.go`
及真实 File/Shell input-addition 主线覆盖来源拒绝、成功工作反馈、零虚构验收与同版本重启。

`ExpectedEffectScope`/`RequestedTargetRefs` 分别只取原 sealed execution/operation，与 receipt observed 值分开。
历史及 exact repair feedback 复用 `ProjectRuntimeSemanticExecutionDiagnostic`，从原 receipt feedback 保留
owner/class/code/signature、revision、来源 hash、truncated/redacted 及可选 typed validation diagnostic；修复来源
不得缺失 diagnostic。只读投影不包含 message/reason/preview、私有 detail ref、输入维护或治理权限；自报退出码
不证明检查执行或验收通过。SM-2/3/4 的私有 writer、CAS、预算、状态和调度权限不变，SM-1 无变更。

semantic `ExecutionBudget` 从同一 exact revision 的完整 aggregate 只读派生：复用既有 budget view 的两个 Run 物理
维度、current/history semantic batches 的原 CWI 消费，以及原 accepted dispatch lineage 的逐 CWI durable seal
用量/剩余额度；治理续派不重复占额，失败/失效不退款，不复制 token/repair scope 或持久化第二份计数。
SM-3 新 session 前组装、来源失败沿原 SM-2 semantic failure；SM-4 只重投影当前快照，SM-1 不变。快照先于本次
submission，后者仍独立预留；unknown 保守占额，已消费批次不因退休/重启退款，null 附加 cap 不表示无限工作。
LLM 仅 C4 完整渲染/校验，无 action/credit/预算权限。`semantic_execution_budget_test.go`、原 proof-budget CAS/history
矩阵及真实 File/Shell 主线覆盖 pre-submission 对账、same-version reload、duplicate、unknown 和稳定前缀。

### Validation-input consistency review boundary

验证输入的一致性允许交由 LLM 评审，不要求用户逐项预先确认，也不要求外部参考实现。原 Goal/DoD 仍是唯一基准；
新增验收只能一致地补充、细化，并与原义务合取。LLM 必须评审原准则与准确输入内容，而不是仅复述候选自身的符合性声明。
评审是语义判断，不是确定性等价证明，也不是验收通过事实。

| 完整评审结论 | 唯一合法后续边界 |
| --- | --- |
| 一致 | Orchestrator 校验完整逐项映射、原准则、输入内容身份及当前 request 后，才可由 SM-2 私有事件认可相应绑定；仍须实际执行和证据验收 |
| 冲突 | 不得绑定或取得 credit；只能在已有预算与授权内生成保留原准则的新候选，或明确失败，不能请求审批豁免冲突 |
| 无法判断 | Orchestrator 封存具体未决问题，经既有 typed governance 协作由 Decision owner 创建阻塞 ticket；LLM 不创建票据或修改 Runtime 状态 |

评审身份必须绑定 workspace/run、sealed Definition/startup input、原准则来源、待评审输入的 exact 内容版本和 request。
输入变化或评审过期不得复用“一致”结论。ticket 必须保留相同身份、未决准则和所需信息；ticket 处理结果仅补充事实，
不能改变原准则或直接授予验证 credit，继续执行前必须重新评审适用的准确输入。Decision 独占 ticket 生命周期，
Orchestrator 独占 SM-2～SM-4；恢复、重复结果及重复 ticket 请求必须沿持久化身份幂等处理。

超时、传输故障、空响应、解析失败、缺少逐项结果和身份不匹配不是业务“无法判断”结论，不得直接制造 ticket；它们遵守
既有 request recovery、document admission 和预算合同。ordinary Definition gap/repair 路径也不得猜测评审结论。
生产接线复用 `semantic_plan/proof_batch`，不新增请求种类或状态机。capability owner 的 proof binding 可声明
`InputReview`，封存 activation 选择的 `validation/input-consistency/v1|v2`、完整输入字节/hash、原 required checks/native mapping；LLM
只返回 `validationInputReview` 的 exact subjectHash 和逐 check 的 `consistent|conflict|inconclusive`、reason、input refs。
缺项、重复项、未知字段/结论或身份漂移仍是 document admission error。完整报告持久化后，SM-2 的
`SemanticFeasibilityRequired` 私有事件在 proposal feasibility probe 前分类：一致才进入既有 feasibility/binding seal；冲突明确失败。

v2 必须由新 activation 的 `RecipeInputReviewProtocolRef` 显式声明，并在 owner-confirmed recipe、catalog、manifest 与
dispatch recipe commitment 中保留。空值沿用旧 owner 协议；不得省略、降级或把 v2 注入未声明的 recipe。
v2 每个 check 另含至多 16 个 `obligations`：原文 `criterionText`、`covered|missing|conflicting|unknown` finding、reason 和
input refs；完整报告 JSON 至多 32 KiB。摘录必须共同覆盖原 CWI 准则全部非空白字符，不能省略限定词或合取项。
该门禁证明文本来源与覆盖完整，不证明断言蕴含准则；语义仍须独立评审，真实验证与 assessment 仍不可缺。
check conclusion 必须与 findings 一致，missing/conflicting 优先于 unknown。只有全报告的非一致项均为已确定的覆盖遗漏时，
SM-2 才使用 `validation_input_review_coverage_incomplete` 区分缺项；真实冲突或混合不明不能被标记为纯缺项。
此分类仍为明确失败，不是 create/patch/supplement 或 retry 授权；不能伪造 validation receipt 接入已有 validation-failure
repair route，不能改写已发布输入、改变原准则或发放 credit。

完整无法判断经本地 governance dispatch（无额外 LLM 调用）封存 `provide_validation_input_facts`，由 Decision
创建 `runtime_governance` ticket。`provide_missing_info` 必须提供非空、完整 UTF-8、最多 2048 bytes 的事实摘要。
SM-2 验证 exact ticket/request/report lineage，在同一个 resolution CAS 中以 `input_review_information` 保留原 semantic
generation，重入 `required`；SM-4 只交付 continuation。新 manifest 仅携带准确 subject 的 preceding resolution facts，
使用新 logical request 重新评审；不改写 Definition/startup、旧报告或历史证据，也不授予 CWI credit。
每个 Run 的同一 exact subject 最多接受一次信息补充；仍无法判断以 `validation_input_review_information_exhausted`
明确失败，不循环开票；既有 history 容量不足以消费答案时，在开票前以 `validation_input_review_history_exhausted` 终止。
拒绝 ticket 则沿既有治理 fail 路径终止。既有 history、request/correction、provider budget 不另开账户。
v2 进一步以同一 Run 的原 CWI/check 为信息预算身份；内容、路径、native/recipe/catalog revision 变化和进程重启均不能
刷新额度。预算从 preceding `input_review_information` 原记录反证，不新增跨 owner counter；fresh-load 同样拒绝超额
retirement 或治理 dependency。ticket 事实仍只供 exact subject 使用，不能把旧输入的信息移植为新输入的一致结论。

Shell v3/v4 adapter 是此合同的 capability producer；输入来源不可读、超出完整 review envelope 或 native mapping 不支持时仍不可用。
v4 的逐 CWI 输入/执行范围由 Shell 独占定义；Core 不按语言或路径裁剪评审输入。默认模板只在新 Start/Restart 选择
当前 adapter，不在恢复中自动升级。preparation 的 owner-authored 内容合同随 catalog hash 封存，只在 C3 投影，不授予
File 输入修改、review 或执行 credit。脚本化 provider 主线只能证明事务/owner/恢复合同，不代表真实 LLM
语义可靠性或 release DCE 已验收。

### Receipt, evidence and repair

- capability observation 的新版本由 activation-sealed adapter 与 exact owner binding 选择；Runtime 不根据当前模板/Soul、
  diagnostic 或历史文件形态推断升级。能力 owner 对未改写输入的 fresh reobservation 仍只是事实，只有现有 SM-2/3
  transaction 可 seal 并认可同一 CWI 的新证明；SM-4 不迁移历史，SM-1 ownership 不变。
  `TestDriveRuntimeEntrypointValidationUsesSameOwnerTransactionsAcrossRestart` 验证真实 File/Shell 子进程证据沿用既有
  input-addition、receipt、assessment 与 terminal-delivery 链，以及相同 pre/post-CAS/restart/duplicate 约束。

- SM-2 rejected-receipt event 对同一 CWI/recipe/protocol/check/case/assertion 的重复失败进行纯 evidence-ledger 比较，在原 CAS
  更新既有 no-progress debt；不改 transaction signature、RequiredOutputHash、repair closure 或 receipt fence。exact credited
  validation pass 关闭比较窗口，零 CWI credit 的 durable repair 不关闭。SM-3 receipt/evidence hashes 同时绑定 owner 的有界
  check failure details、incomplete checks 与 output truncation；SM-4 fresh load/duplicate 复用原 ledger，零归档 backfill。
  rejected evidence 的 CWI 诊断归属只从 sealed binding 读取；没有 binding 的记录不猜测归属，不能补写验收 credit。
  可选 `validation/check-failure/v2` 的比较使用 owner 提供的 opaque `ComparisonHash`，而 receipt/evidence 仍绑定精确
  `DiagnosticHash`、协议和全部 bounded reported assertion。路径归一与测试语法只在 capability owner；Core 不解析日志。
  未携带扩展的归档及旧比较身份保持原样，不 backfill、不跨版本猜测同一失败，不新增 repair counter 或 transition。
  `validation_failure_comparison_test.go`、Shell reference-grader tests 与既有 repair receipt/CAS/restart matrix 共同验证此边界。
  `TestDriveRuntimeAssertionFeedbackRepairsBehaviorAcrossRestart` 以真实 File/Shell/Run 和 scripted provider 验证同一输入
  经真实失败→诊断→实现修复→fresh validation→CWI/Result/Run 关闭，覆盖 rejected-evidence CAS 前后中断；不是真实模型求解验收。
- validation entry authorization 与 full diagnostic scope 分离。发送 owner execution、接纳 failure receipt、确认 repair
  surface、commitment 和 fresh-load invariant 必须复用同一个 completed-dispatch scope 投影：取 exact
  admissibility snapshot/acquisition binding 的完整 catalog，不以单 operation entry 的 authorization 集合代替。
  catalog 只提供诊断范围，不扩大 entry 授权；候选修复 target 仍需 capability owner confirmation。
  `shell_owner_receipt_integration_test.go` 用单文件 entry + 多文件真实 Shell catalog 覆盖 SM-3 receipt → SM-2 confirmation/
  repair → fresh validation，以及 SM-4 reload/duplicate；复合回归还覆盖 catalog 更新后再次失败→独立新确认→第二次修复
  成功或 profile budget 耗尽、旧 continuation 消费的反向篡改校验与零候选/provider 旁路。SM-1 lifecycle authority 不变，
  SM-2/3/4 的迁移仍只通过 Orchestrator private internal event/统一 aggregate reducer。
- 已封存 validation-repair effect 的 rejected owner receipt 必须在完成 dispatch 的同一次 private SM-3/SM-2 CAS 中
  退休 target/directive，并封存 `repair_surface_unavailable`；若持有 semantic repair intermediate binding，同步退休该 binding。
  receipt 原诊断、confirmation、commitment 与 evidence lineage 保持不可变；普通拒绝不授权恢复旧修复范围或自动 successor。
  candidate-local applicability 在 dispatch 前使用唯一 attempt 0/1；typed stale、governance 与 unknown effect 仍各走既有独立事件。

- capability owner 只返回 factual catalog/feasibility/receipt。receipt 必须绑定 exact dispatch、operation、execution key/fence、
  target authorization、effect、binding 与 recipe/check；只有 SM-2 private event 可以裁决 evidence kind、credit、CWI settlement
  或 terminal。
- receipt Query/Commit 的 producer 必须与 persisted sealed dispatch 中 exact operation kind 的 descriptor owner 一致；source
  identity 不做 whitespace normalization。empty、whitespace/noncanonical、wrong 或 cross-capability source 在 receipt lookup 或
  CAS 前拒绝。Commit adapter 只把 observed source 映射为 `OperationReceiptObserved` private event；source/operation 校验与 receipt
  mutation 使用同一次 aggregate load/CAS，并统一经 `submitRuntimeInternalEvent` 的 Workspace-retirement fence。该 gate 是 SM-3
  receipt admission；`found=false` 虽由 SM-4 drive 中的 owner executor 使用，也不能成为独立 SM-4 transition authority。
- `rejected` receipt 先保留 owner exact diagnostic；只有 `accepted|no_op` 进入 effect/quality gate。raw stdout/stderr、provider
  reasoning 和 decoder message不能进入 aggregate、Eventstream、projection、memory 或 C4。
- claimed/no-receipt 只在 sealed idempotency 允许时消费 SM-4 operation-recovery budget，并持久化 exact
  `RuntimeOperationRecoveryAuthorization` 后重放。authorization 逐项绑定 workspace/run、dispatch、operation、
  `ExecutionKey + Fence`、全局 recovery attempt 与 issued revision；`issued` 必须先由独立 private event CAS 为带 lease 的
  `in_flight` claim，只有成功 claim caller 获得一次 physical-call handoff。exact receipt 原子 `settled`；返回无 receipt 记为
  `uncertain`；active claim 禁止新 grant，lease expiry/restart 也必须先持久化 uncertain 才可消费下一预算。counter/ledger、顺序、
  identity、claim lease 与 dispatch reverse proof 在 fresh load 全量校验；at-most-once/unknown effect 进入 `outcome_unknown`。
  exact receipt duplicate零 effect、零 evidence revision、零 credit。
- `Facts.RepairDirective` 是同 focus 唯一 repair action authority。LLM assessment 的文本、missing claim 和 owner diagnostic
  只作为 observation，不能选择 state change、validation、retry 或 terminal。
- owner-grounded repair 必须区分 diagnostic observer 与 effect owner。repair owner 只能由 sealed Profile exact route 的
  `RepairOwnerModuleID`，或 source binding 中 operation descriptor 的唯一 owner 确定；
  `RuntimeRepairSurfaceConfirmation.OwnerModuleID`、candidate 中每个 state-change descriptor owner 与后续 receipt producer 必须
  exact-match。route 的全部 operation descriptor 必须在 Definition pre-seal 共同声明同一个非 `none` confirmation protocol；
  Biz adapter 只按该 technical protocol 发送对应 owner 的 concrete typed command/result。其它 owner 的 catalog/diagnostic 即使
  同时存在也不能进入该 repair manifest。confirmation 后的 repair directive 只能携带 exact route 的 operation kinds，不能继承
  同 owner catalog、current binding 或 capability inventory 中的其它 state-change kinds。
- validation failure target 必须来自 sealed finite scope 内的 structured owner observation，再由上述 resource/effect owner 执行
  read-only confirmation；pending/stale/rejected confirmation 均无 concrete repair authority。只有 `need_state_change`
  repair-effect pre-seal manifest 必须将 purpose 固定为 `confirmed_validation_repair`，并在
  `RuntimeDispatchRepairCommitment.RepairOwnerModuleID` 中封存 owner。confirmed
  surface 在 Decision、owner confirmation 与 commitment closure 中保持 exact；`exact_target` candidate 可选择其中非空子集，
  但不能改写该 surface。该 manifest 的每个 repair operation 必须把 source rejected-validation operation 的唯一 current
  `bindingRef` 声明为 required/allowed 并原样保留；这是 causation lineage，不是 repair receipt 的 terminal validation credit。
  若 source binding 不声明 `state_change` proof，accepted repair receipt 仍按 sealed repair commitment 合法提交，但
  `EvidenceFact.CoverageWorkItemIDs` 必须为空，且不能生成 `coverage_binding_mismatch`；owner-authored
  `EvidenceReason` 必须与 receipt exact-match，SM-2 matcher diagnostic 只允许进入 `FailureFact.OriginalDiagnostic`，不得回写
  receipt projection。普通非 repair binding/proof mismatch 仍必须 fail-closed。
  缺失、额外或漂移的 binding 在 provider correction/admission 边界拒绝，不能留到 commitment fresh-load 校验才成为内部冲突。
  sealed candidate all-of 的每个 matching-owner receipt 都必须形成连续、owner-scoped workspace
  revision edge，最后一项后才生成 target-free fresh-validation directive；该 directive 保留 closure ID、清空 purpose，并生成
  普通 Definition validation manifest，不得再次创建 repair commitment。其后若发生确定性内部失败，SM-2 failure transition
  必须在同一 private CAS 中退休该 continuation 并封存 failed terminal，SM-4 不得循环生成 `persisted_pending` lease。只有显式
  required sequence/collective receipt-set 才要求完整 surface 全部产生 effect。software-dev Profile 的
  `Shell diagnostic → File confirmation/effect` 只是该中立合同的一种 specialization，不是 Orchestrator core 分支条件。
  SM-3 的 `candidateRepairClosureFacts` 复用 fresh-load 的 exact completed-transaction reverse proof，把 final repair evidence
  revision 投影为同一份 invocation-local 因果边界：旧 closure targets 不再限制普通 validation manifest 的入口，且该边界前的
  rejected operation shapes 退休；修复回执仍零 CWI credit，边界后的新 rejection 仍受防重放限制。不能凭 state/purpose/ID
  标签绕过证明。候选准入与最终 seal 必须接受同一合法 manifest，真实验证入口不要求等于修复文件。
  `candidate_repair_closure_mismatch` 显式归入 SM-3 rejected-final，保留原始 code/diagnostic；未知 producer code 才封存
  SM-3 `runtime_contract_defect` failure 并保留有界 source code；SM-2 的 canonical contract-defect terminal vocabulary 不因此改变。
  生产 code 常量、decoder/probe failure 分支由源码导出测试核对 disposition，
  不是运行时第二份 policy registry。SM-2/SM-3/SM-4 仍共享 owner-private event 和 aggregate CAS，SM-1、capability 与投影不选状态。
- `CurrentRuntimeRepairSurfaceConfirmationPlan` 是该 read-only handoff 的唯一 owner-private action classifier；它只从当前持久化
  aggregate、exact VFO confirmation sequence 与 repair transaction lineage 派生，不持久化第二份 phase。`FailureTarget == nil`
  只是 mutable authority shape，不能单独表示“等待确认”。SM-4 delayed drive 必须先 reload/re-plan，再决定是否查询 owner；同一
  VFO 已 `confirmed|rejected`、active partial repair、target-free fresh validation 或 administrative termination 均为零 query。
  reducer、query plan 与 SM-3 candidate/provider guard 共用 `runtimeRepairSurfaceConfirmationObligation`：它从 exact committed
  validation receipt、current focus/definition binding、sealed route、未耗尽 budget 与该 observation 的 sequence 派生待确认义务，
  不依赖它将要生成的 Decision。新的 rejected fresh validation 在同一 receipt CAS 消费旧 target-free continuation，且不授予
  CWI credit；新 VFO 可以形成独立、受限的确认窗口。该义务先于可变 catalog 的普通 target-grounding 判定，确认前禁止
  semantic/operations provider request 与 candidate attempt。catalog refresh 不退休 execution-locked binding、不修改 startup
  input，也不复用旧 confirmed target；已封存 terminal、contract defect 或 budget exhaustion 不能被该窗口重新打开。
  profile repair budget 在 rejected receipt 的同一 CAS 结算，耗尽后无新 owner query。fresh-load 只有 exact rejected evidence、
  processed event、candidate manifest closure、committed execution 和 matching receipt fence/hash 才能证明旧 continuation 已消费。
  plan 必须派生 exact
  `RequestID + IssuedAtStateRevision + protocol-bound SubjectRef + ExpectedSubjectRevision`；protocol descriptor 只向 core 暴露
  owner-neutral resource domain/subject，expected revision 来自 matching current prerequisite observation，不能硬编码 File 或 manifest
  kind。`RequestID` 同时绑定 observation、owner、protocol、subject、expected subject revision、current revision 与 stale refresh lineage。
  concrete File command/result 原样回显全部 fence；actual workspace-tree revision 不同时只返回 stale，只有相同 revision 才能
  confirmed/rejected。SM-2 writer 在 fresh-load 后复用同一 plan 同时执行 query admission 与
  new-response CAS admission。成功 CAS 生成的 record 必须满足
  `RecordedAtStateRevision == IssuedAtStateRevision + 1 == resulting StateRevision`，opaque reload 同时重验该 revision chain 与同一
  VFO 的 subject continuity。stale→prerequisite refresh→reopen 后 plan fence 必须变化，前一窗口的延迟 response 即使此时又处于
  `need_validation` 也只能 conflict 且零写。
  stale confirmation 的同一 SM-2 CAS 必须退休 exact dependency 上的 current owner facts、追加 invalidation record 并结算独立
  prerequisite-refresh budget；不能建立 `FailureTarget`、消耗 validation-only/repair budget或把 refresh 委托给 File/Shell。
  confirmation CAS 前的 crash/response loss 允许同一 read-only query at-least-once 重发；CAS 后必须为零 query。handoff 任一步返回
  error 时先 reload：若 revision 已推进则返回 current aggregate，不得把 termination 或并发 confirmation 误分类为新 drive failure。
  termination 前已持久化的 exact confirmation replay 仍为零 CAS；termination 后到达的新 in-flight response 必须 fail closed。

### 附加 lineage guards

- `RuntimeLineageAssertionRule` 是 sealed Profile 的 opt-in all-of 声明；`ConsumptionPoint`（配置字段
  `consumptionPoint`）只区分 `execution` 与 `post_terminal_delivery` 的消费时点，不是 Runtime phase、目标状态或
  跨 owner transition command。旧 `phase` 字段不属于当前闭合 wire，必须拒绝，不做兼容读取或持久记录回填。
- SM-3 的 Definition → semantic obligation → `CoverageEvidenceBinding` 保留 exact CWI/source/declaration、完整
  `TargetText` 的字节 hash 与 rule hash。SM-2 在原 assessment CAS 内将普通 proof 与 execution guards 合取；归档复核
  同样使用原 assessment dispatch 的 `Transaction.EvidenceRevision`，未来 receipt/round 不得补成旧 credit。
- `completed_work_rounds` 消费既有 distinct ordinal/dispatch/execution、完整 changed durable receipt set 与 fresh
  observation/input-protection 前像；`owner_operation_receipt` 消费 canonical owner/kind/variant/effect inventory、
  sealed operation、execution fence、唯一 receipt/evidence 和 processed-event 链。guard 不独立授 credit，也不放宽
  rejected receipt 的 effect 校验、ordinary proof、预算或 immutable source。
- `repair_then_fresh_validation` 只接受同一原 rejected validation、exact route/confirmation/closure、全部 changed
  repair receipts 与严格更晚的同 source ordinary all-of validation。fresh binding 的评审必须回指产生该 binding 的
  exact accepted generation/batch/document/request/owner feasibility；不能扫描旧报告或凭 consumed/satisfied 标签恢复成功。
- SM-4 的 `RuntimeDeliveryLineageState` 是只读投影：preterminal `deferred`，terminal 无 seal 或不足有序三 ack 为
  `pending`，exact immutable seal 与 Result → Workspace → Run 三 ack 全部通过共享校验才为 `satisfied`；无声明为
  `not_applicable`。projection 不改变 terminal、Result material 或 coverage credit，不引入 Result-before-Result 循环。
- 纯消费者不持有 I/O/CAS，fresh-load 从完整原值前像反证；仓储 `Load`/`Save` 必须传递 detached raw 深拷贝。
  `RuntimeAggregate.CloneRaw` 保留 nil/empty、全部 dispatch/receipt/seal 内容，不通过 normalization 修复非法前像。

### Recovery, governance and delivery

- 可选 `MaxDurableDispatchesPerCWI` 复用 accepted attempt → original dispatch 的 sealed ledger；SM-3 final admission
  在与 accepted attempt/ActiveDispatch 同一 CAS 中占额，对完整 binding CWI 按 durable dispatch 计数，而非文件/操作数。
  governance resume 不产生新 accepted attempt，继承原额度；失败、失效、换 generation、重启不退额。aggregate/opaque
  state 同时验证 admission lineage 和上限；typed 耗尽复用 SM-4 failure ingress 与 SM-2 terminal，SM-1 不变。
  该逻辑上限不替代物理 claim 预算，不开放通用工作权限，也不保证已有候选 provider 零消费。

- activation-sealed `RuntimeExecutionLimits` 收紧现有 Run 级物理调用/操作维度；bootstrap 与 facts reload 反查对应
  policy 的 scope、limit、unit、单次增量及耗尽合同，不接受重新 hash 后的扩额。SM-3 submission/claim 的原私有
  event/CAS 是 reservation 唯一写入点，SM-2/SM-4 沿用既有耗尽/恢复路径，SM-1 不变；没有第二份账本或一般工作授权。
  `biz/runtime/execution_limits_test.go` 覆盖调用/操作 CAS 前后、duplicate、stale、unknown 占额与同版本重载；
  `TestDriveRuntimeExecutionLimitsPreserveRealOwnerCompletionAcrossRestart` 覆盖显式限额下真实 File/Shell、file-backed
  Run、Result 的完成与重启，并逐项对比物理调用/owner receipt 与原预算视图。provider 为 scripted fixture，不代表真实模型验收。
- 可选 `MaxProofBatchesPerCWI` 只约束原 CWI 的证明规划批次，不是实现迭代数。SM-3 compile 私有事件在 fresh-load
  后汇总 current/history selected CWI 消费并与 requesting batch 同 CAS 提交；pending 窗口不计，预检失败不退还，
  同批次 correction/recovery 不重复计。exact duplicate 零写、身份漂移拒绝、generation retirement 不清零；额度不足
  在 session/provider 前返回 typed budget exhaustion，由原 SM-4 failure ingress 与 SM-2 terminal 处理。
  shared usage 只是原 semantic ledger 的纯投影；aggregate/opaque-state 均检查上限，Application、Run、LLM 不获得计数
  或 transition 权限，SM-1 不变。`biz/runtime/semantic_proof_budget_test.go` 覆盖 CAS/restart/历史占额，真实 owner
  `TestDriveRuntimeProofBudgetExhaustionClosesWithoutAnotherProviderOrEffect` 覆盖零额外请求/effect/ticket 的终态交付与重放。
- provider、operation、CAS、repair、governance、terminal delivery 使用独立 persisted budget/guard。allocation 在 external
  call/effect 前与授权同 CAS 创建；submitted outcome必须 settlement，unknown conservatively占用预算。
- generic nonterminal recovery 仅接收 Run persistence owner 产生的 scoped typed CAS conflict，或带 exact
  `persistence/owner_transient` feedback 的 owner-transient outcome。Orchestrator 从 fresh aggregate revision、typed expected/observed
  revision 与 failed generation 派生稳定 `RuntimeNonTerminalRecoveryRequest`；`nonterminal_recovery_scheduled` 是唯一 writer，并在同一
  private CAS 写入 latest record、CAS-reload guard disposition 与 request-bound scheduled lease。duplicate/response-loss 复用同一
  RequestID，outer drive 只挂载 event 已持久化的 lease，禁止再发第二个 lease event。普通 conflict/timeout/Temporary、provider 与
  operation error 不得进入该 lane；owner transient 不得占用 operation recovery ledger。guard exhausted 保持业务非终态，并以 capped
  successor 继续 owner-local reload，而不是生成 terminal 或无 authority 空转。record 不得超前于 aggregate revision；在其写入 revision
  上，current lease 必须与 record 内嵌 successor normalized exact-match，record-only/替换 lease 的持久状态一律 fail closed。
- scheduled worker 的统一 SM-4 error boundary 在分类任何非 terminal-delivery error 前 reload current aggregate。若另一条
  owner-private CAS 已封存 convergence terminal 或 administrative termination，则 durable terminal 覆盖在途旧 error，只走既有
  Result→Workspace→Run delivery；不得生成新的 semantic failure、recovery budget或 lease。terminal-delivery error 本身先 reload；
  final ack 已完整持久化时零预算结束，否则进入 bounded delivery retry，且不能由 rebase boundary eager 重投。classifier 以显式 handled disposition 区分“terminal 已交付、无需
  retry”和“尚未分类”，panic/preflight 调用点不得在前一种结果后补造 fallback lease或保留旧 gate。
- `RuntimeDriveLease` 是 delayed recovery 的唯一 authority。timer只 claim exact due generation；process-local gate/timer 仅合并
  persisted identity。administrative termination 可在同一 owner-private CAS 退休并发的 running generation `G`；旧 worker 仅在
  terminal delivery 尚未完整 ack 时，才能以 canonical `delivery_retry/terminal-delivery` 在调度 CAS 创建 `G+1` successor。
  旧 timer 的 `end(G)` 不得删除 `G+1` registration。若 final ack 在预算或 lease CAS 前完成，二者均为零写；若 exact running
  `G` 尚在，lease CAS 必须原子退休它并返回 terminal-complete disposition，worker 据此释放 gate，不能留下 zombie running lease。
  acknowledgement 在 budget/lease CAS 间抢先时，SM-4 owner bounded reload/retry：partial ack 仍只消费一次并生成 successor，final ack
  直接完成；budget CAS、new scheduled-successor CAS 或 exact-running cleanup CAS 已提交但返回普通 response-loss error 时，fresh load
  必须分别识别已消费 ledger、已持久化 exact scheduled lease 或 terminal-complete，不得重复 budget、generation 或 timer。预算结果无法
  线性化时只保留 process gate，禁止无预算 lease。
  termination aggregate 上唯一合法 retained lease 是 exact `delivery_retry/terminal-delivery`。
  synchronous `DriveRuntime` EventHub ingress 与 scheduled worker 共用同一 workspace/run gate；两者都先检查 Run-owner durable
  retirement fence，同步入口持 gate 后还必须 fresh-load 并拒绝任何 persisted lease，不得执行 owner/provider work。
  同步退出使用 module lifecycle context 处理 persisted-lease latch，caller cancellation 不能取消已提交 lifecycle successor 的 exact
  timer mount；mount 后仍由同一 gate 持有该 generation。所有非 Stop gate exit 汇入同一个 fresh-load settlement reconciler；本地
  settlement 只是 lifecycle/epoch/gate-token fenced wake hint。仅 pending active entry 通过 one-shot timer 提交 framework
  `BackgroundRoutine` 短任务，失败 exact re-arm，空闲零轮询；它不能持久化 phase、选择 action或作为 lease authority。dormant
  termination/governance handoff 仅等待 exact gate owner exit，release 与 handoff 激活在同一本地锁内线性化，active entry 不得被重新
  dormancy。历史 administrative retired marker 只绑定首代 terminal handoff，不阻止 current exact later-generation request 重物化。
  Stop 清理本地等待但不删除 durable lease，fresh startup从 Run owner inventory恢复。
- framework `Setup` 必须同时注入 EventHub 与 `BackgroundRoutine`，`Run(ctx)` 把 framework context 绑定到全部 owner-local drive、
  timer 和 startup recovery。startup 失败关闭 fresh admission、取消后续本地调度，但保留已获得的逐项订阅、cancel、gate 与依赖，不能提前清理 accepted 队列工作；检查式 Stop 成功后由新实例重试启动。`BeginShutdown` 非等待地关闭新 drive／新事务／新调度，保留原 source、scope、dispatch、grant、binding 与 CAS 校验下的 receipt、治理结果和只读查询入口。`Quiesce` 等待 handler、提交中的工作及 accepted 队列 callback 真实结束，并检查原 lifecycle 读锁；cancel、Timer.Stop 或 Send 超时不等于真实退出。
  `Stop(ctx)` 在预算内排空后封闭全部入站，再次排空，逆序 checked 退订并逐成功移除账本；任何失败或未知 panic 保留剩余账本与依赖供新预算重试。异步 drive、lease timer 与 settlement task 在提交前取生命周期票，拒绝释放票，accepted callback 真实退出才释放。该锁与 context 只做 Module 生命周期同步，不是 SM-1/SM-4 authority；
  durable successor仍只来自 aggregate lease/authorization，重启从 Run owner inventory 恢复。Module 保持 startup weight 30、
  teardown weight 90：前者保证 Orchestrator ingress 先于 Run recovery，后者保证 Orchestrator worker 先于 Run/capability teardown
  停止；两个依赖顺序不得再复用同一个反向权重推导。
  无返回值的 plugin `Teardown(ctx)` 必须检查 `Stop(ctx)`：失败保留原 Biz，并将带原始 cause 的 `*def.Error` panic
  交给现有 plugin guard，由真实 `PluginMgr.TeardownChecked` 返回错误、停止后续插件释放且不标记该 Module released。
  阻塞 handler 真正退出后，新的停机 context 可重试；已完成插件跳过，成功后才清空 Biz，重复 Teardown 幂等。
  这是 Module/manager 检查式失败贯通，不是全进程 Quiescer/Initiator 屏障或完整生命周期验收；不改变 SM-1～SM-4
  的迁移、authority、持久化事实或预算，也不授予超时后的强制释放权限。
- 已提交的 Run active/resume/continuation 异步通知进入 SM-4 后，retirement admission 与 worker submission 都使用
  Orchestrator lifecycle + exact workspace context，不能沿用已结束的 caller context。同步 command 仍尊重 caller cancellation；
  owner Stop/cancel、source/typed fact 校验、retirement/quiescence fence 与 singleflight 不得旁路。该 context 切换不授予
  通知 reducer authority，也不新增状态或恢复路径；`biz/runtime_notification_context_test.go` 覆盖取消请求、owner 关闭、
  retired workspace、错误来源和重复通知。
- startup 只用 Run owner 的 bounded recovery inventory/readiness/opaque record query。unknown/retired current JSON、cursor cycle、
  page/item limit、nil/wrong result直接使启动失败，零 provider/capability/governance side effect。
- Decision/Approval owner 持久化 resolution；Orchestrator只暴露 ticket-resolution 与 approval-resolution 两个具体 typed ingress，
  先从 aggregate 重建 exact persisted binding，再在 owner-private value adapter 中提交 continuation或terminal。不存在 generic
  governance-resolution EventHub command/topic；Application和Governance都不发布 Resume。
- terminal outcome先由 SM-2 seal（administrative termination 由 SM-4 保存 SM-1 intent 的 typed handoff），
  SM-4 在首次 owner I/O 前通过 scope-only private event 封存不可变 `RuntimeTerminalMaterialSeal`，再按
  Result→Workspace→Run 顺序交付。generation 是首次 seal CAS 的 after-image revision；同次 CAS 只增加 seal 与 revision，
  独立加载并校验的持久前像证明材料来源。后续 lease、recovery 与 ack 可以推进，但不能删除或修改 seal。
  各 owner 只执行自身 private reducer；Result ack 携 exact 已持久化 `ResultID` 和完整 `ResultCloseout`，Workspace/Run ack
  不携 Result body/ID。partial/response-loss 重用同一 seal 幂等续驱，不重编材料、不改写 terminal decision。
  terminal 未 seal 且无 ack 可以等待首次交付；无 seal 却有 ack、Result body 缺失或不等于 seal 均 fail closed，不回填旧归档。
- SM-4 的 Result material 从已裁决的 requirement outcomes、evidence 与 receipts 投影逐项 DoD、实现/验证摘要、限制、
  风险和仅供阅读的后续建议；没有 validation evidence 不能描述为验证已执行或失败。terminal guard 与原始 owner cause
  分别保存，Result owner 不重判验收。新 aggregate 初始化时一次封存 `ResultFeedbackVersion=3`；以当前 SM-2
  coverage 判断历史 satisfied assessment 的适用性，当前未满足时显示 indeterminate，不删除历史评估或证据。
  v3 的 input-review terminal diagnostic 另保留完整 blocking batch 的 typed reports（CWI、requirement、subject、review ref/hash、
  逐 check/obligation 事实），并从原 persisted document/binding 反证；删除或篡改反馈不得在 fresh-load 被接受。
  原始报告、诊断文本与控制字符按原值保全，不为脱敏改写；受限读取边界和可逆展示编码保护原文。Result、Web 和 TUI
  仅展示，不能据此选择 action；完整详情不能只剩被裁剪的通用摘要。
  当前只允许版本 3；缺失、旧版或未知版本均在 Run record、aggregate、initialize 和 Result material 入口 fail closed，
  不 fallback、不回填、不改写旧 Result/Run 归档。同版本 response loss、重启及 pending delivery replay 的内容与 hash 必须相同。
- Workspace deletion只允许在 Run owner `draining` fence后排空既有 terminal delivery，包括为已封存终态首次固定 material seal；
  此时仍拒绝 caller material、SM-2/SM-3 推进与普通恢复。Orchestrator 独占自身 quiescence/purge private reducer，
  不终止 Run、不推进 Workspace deletion phase、不跨 owner 删除资源。

## 组件交互及合同

| 交互方 | 方向 | concrete contract | owner 边界 |
| --- | --- | --- | --- |
| `blocks/run` | 双向 | state notification、bounded recovery inventory/readiness、opaque Runtime record read/CAS、lifecycle/terminal ack | Run拥有SM-1与物理opaque store；不解释SM-2/3/4 |
| `blocks/llm` | Orchestrator → LLM | provider ceiling、preflight、semantic/operations request | LLM返回provider/document fact，不创建attempt或transition |
| File / Git / Shell / MCP / Web / SSH | 双向 | 各 owner 支持的 capability/admissibility/candidate-conformance/feasibility/semantic/reconciliation 子集，以及 execute、receipt query/commit | capability owner拥有effect/receipt并只返回自身 static conformance 或 dynamic feasibility fact；不选择next action；Web/SSH不提供CWI reconciliation；仅有 matching manifest constraint 的 operation 请求对应 constraint fact |
| Decision / Approval | 双向 | governance/clarification open、resolution/evidence | 资源owner拥有ticket/approval lifecycle；Orchestrator拥有Runtime continuation |
| Result / Workspace / Run | Orchestrator → owner | sealed terminal delivery fact与exact ack | 三个owner各自迁移，SM-4只记delivery ledger |
| `application/workorchd` | 双向只读 | immutable drive observation、runtime audit query | Application不Drive、不持有aggregate |
| `kernel/governance` | 无直接I/O | 经Decision/Approval/Run owner fact间接协作 | 不存在Governance↔Orchestrator生产topic |
| Skill / Soul / MCP Registry / Memory | 无直接I/O | 只消费Run activation中的detached snapshot | registry或memory更新不改变既有Run |
| Eventstream | 无直接订阅 | owner notification形成审计 | 审计不重建authority |

## 不变量与非职责

- 核心必须领域中立，不写死 profile、语言、测试框架、MCP tool 或业务领域语义。
- 每个 formal state 只有一个 owner；外部 EventHub metadata 只描述 participant，不授权外部 reducer。
- SM-1不选择 action，SM-2不执行 effect，SM-3不选择 next action，SM-4不重建 authority。
- duplicate幂等；stale duplicate、result 或 trigger 不得越过当前 fence，只能进入其 typed refresh、retirement 或 diagnostic
  route。CAS前中断、CAS后响应丢失和fresh-process reload不得重复provider、effect、receipt、evidence、budget或delivery。
- projection/read model/metrics/Eventstream、LLM文本和process-local cache均非authority。
- 禁止 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher、
  application continuation queue、auto replan successor、checkpoint authority replay与map/JSON/reflect authority bridge。
- Orchestrator不暴露Start/Resume service facade；生产execution ingress是Workspace-scoped intent，经Run owner SM-1 notification
  触发Drive。
- current persistence只接受current shape；没有legacy decoder、migration/backfill或由projection修复invalid aggregate的路径。
  候选前 authority reconciliation 也必须先校验完整持久前像；历史 focus 的 prerequisite/resource 退休由原 SM-2
  transition 的协调写集完成，不能先加载非法归档再以一个看似合法的 after-image 绕过独立前像 CAS gate。
- provider-facing cache保持C0 system/schema/runtime、C1 Goal/DoD、C2 declarative capability/governance、C3～C5 volatile runtime
  分层；unexpected stable-prefix drift是rendering defect。

## 能力验收

Profile 的独立 `RuntimeImplementationWorkPolicy`（`runtime-implementation-work/v1`）声明有界的 owner、operation、selector、轮次、fresh observation/validation 与输入保护，进入 Profile 身份和 exact CWI 的不可变 Definition 投影。声明本身不授予 effect；`RuntimeExecutionContract.ValidateExecutableAdmission` 与 Workspace promotion 仍通过 availability gate 拒绝尚未开放的协议执行。纯 `Validate` 保留合法声明供合同校验，不代表可执行准入。

工作事务复用 SM-2/3/4 和同一个 Runtime aggregate，SM-1 不新增迁移。SM-2 私有 scope 事件先 CAS 封存只读 `ScopePreview` 需求，再接收真实 owner 观察；独立来源由 sealed Profile、Definition seed、exact CWI 与当前 manifest 前像重新编译，不能从待验证请求反向构造。File 适配层保留完整有界 inventory、unsupported 负事实和 typed stale；单轮目标上限不能裁剪观察。scope 不写入终验 `Resources.TargetSelection`，也不授 variant、dispatch 或 credit。

原 accepted semantic `coverage_order` 提议 `implementation_effect` 或 `terminal_proof`。SM-2 消费该文档并原子退休原请求 generation：effect 才 reserve 原 CWI 的有界 round，terminal 只恢复原完整终验前置。工作 purpose 必须取得 manifest、provider ceiling、capability、capability state 和全部已声明 owner catalogs；仅延后当前 focus 的终验 target/scratch，working binding 点名的 exact snapshot 仍须取得。sealed capability inventory 独立反查 owner 输入保护 catalog，不能通过协调删除 Pending/Satisfied/catalog 缩小 exclusions。普通无工作策略路径保持原前置合同。

SM-3 复用完整 semantic proof、owner feasibility、request session、candidate、sealed dispatch 与 execution。只有原 accepted proof batch 和 matching owner facts 能经 SM-2 私有事务封存 `RuntimeImplementationWorkingBinding`；它与 `CoverageEvidenceBinding` 分离，完整 policy、scope/catalog 前像、输入排除、原 semantic lineage 和前轮 collective receipt 身份进入 working commitment、manifest 与 dispatch。每轮独立请求/dispatch，不能借用 owner-fact acquisition 或 confirmed validation repair 权限。

`OperationReceiptObserved` 的一次 aggregate CAS 同时提交 SM-3 execution/receipt/budget、SM-2 working effect/round 和 SM-4 必要 retirement。完整 collective receipts 齐全并反证资源失效后才结轮；工作 effect 保留 owner 原 quality，零 validation/terminal/CWI credit。后续轮须 fresh owner observation 和新的 accepted semantic proposal/feasibility，预算不重置；逻辑 durable dispatch 使用原 sealed working CWI 和 accepted attempt 来源计数，归档不退款。进入 terminal purpose 后恢复原 all-of 终验、输入审查和 exact repair 合同。

rejected、stale、missing invalidation 与 unknown 关闭原工作权限，不通过新轮重发；governance continuation 保留同轮原 execution key，新的 fence/dispatch hop 必须与原 grant/receipt lineage 匹配。无 receipt 的原 started claim 进入 unknown 后，只允许 exact original key/fence 的迟到 owner receipt 入账与资源失效同 CAS；不能重开 round、补 terminal credit 或将原 physical unknown consumption 改成 confirmed。fresh-load 同时反证原 unknown allocation/consumption，不能从文件存在、provider 文本或重写 hash 推断 effect 成功。

合同测试包括 `runtime_implementation_work_policy_test.go`、`runtime_definition_implementation_work_test.go`、`runtime_implementation_work_ledger_test.go`、`runtime_implementation_work_receipt_hop_test.go` 与原各 owner 准入反例。`biz/runtime/implementation_work_*_test.go` 覆盖 private transition、完整写集、duplicate/stale、来源篡改、CAS 前后回复丢失和预算；unknown receipt 的独立测试进程使用 test-owned repository，不是公开 Run-owner 全链恢复。
`biz/runtime_implementation_work_scope_test.go` 校验真实 File 只读 scope、完整 inventory 与 revision fence；`biz/runtime_implementation_work_two_round_drive_test.go` 在 private startup fixture 后使用 production outer driver 和真实 File owner 完成同 CWI 两轮，不能替代公开 authoring/activation、终验、Git、正式 Result 和全部 delivery ack 的完整工程验收。availability 不因这些局部证据自动开放。

`make verify-runtime-implementation-work-readiness` 对每个目标包拒绝零测试选择，并包含公开 `internal/integration/implementation_work_fullflow_test.go` 完整正例及 unsupported 反例。公开正例的 failure 使 readiness 和依赖它的 CI 验收失败，不能作为 expected failure、skip 或 bounded-terminal 成功。Git 交付必须将 sealed execution 的 exact key/fence/receipt hash 与实际 commit 对象、最终 HEAD、完整 tree/blob bytes、已评审输入 hash 和正式 Result 的同一 evidence ref 联结；仅 `evidence_committed` 状态、`.git` 存在或 commit 文本不构成该证明。

同一个 `CoverageEvidenceBinding` 的 proof 按 evidence kind 唯一，不能用多个 validation recipe proof 冒未支持的 build/test/race 合取。validation observation 必须匹配 exact recipe、protocol 与 native-check attestation；另一 recipe 的 passed checks 不授予本 proof credit。复合执行须由 capability owner 实际生产并持久化完整事实；命令名称、检查标签、验证输入内子命令或 README 自述不替代 sealed owner execution 的证明。

| 验收面 | 自动化证据 | 必需维度 |
| --- | --- | --- |
| ordinary validation-input protection | `workorchruntimecontract/runtime_validation_input_protection_test.go`、`biz/runtime_validation_input_addition_mainline_test.go::TestDriveRuntimeOrdinaryMutationCannotWeakenValidationInputAcrossRestart`、`TestDriveRuntimeRepeatedInputWeakeningClosesWithoutEffectOrTicket` | 三策略及 selector-only 排除、source preimage/hash/clone/replay、exact repair 例外；真实 File/Shell + file-backed Run + scripted provider 下，越界拒绝→合法源码修改→独立补充→fresh validation→Result/Workspace/Run 完成；source dispatch/input receipt/拒绝终态切点重启零额外 provider，重复越界零 effect/credit/ticket，不宣称真实模型或 release DCE 验收 |
| validation-input review / factual ticket | `biz/runtime_validation_input_review_mainline_test.go`、`workorchruntimecontract/runtime_validation_input_review_test.go` | SM-2/3/4 共用旧事务；一致→实际验证，冲突/格式错误零 ticket，无法判断→Decision 信息→新请求重新评审；同一 subject 信息上限、reject、exact duplicate、内容/lineage tamper、五个持久化切点恢复与零额外 provider；SM-1 仍由 Run owner 交付 |
| v2 obligation review / full feedback | `workorchruntimecontract/runtime_validation_input_obligations_test.go`、`runtime_validation_input_review_feedback_test.go`、`biz/runtime/validation_input_review_test.go`、`biz/runtime_validation_input_review_mainline_test.go` | 完整原文映射及 Unicode/容量边界、缺项与矛盾分离、Run 内原 CWI/check 信息额度、协议降级拒绝；真实 owner 原输入不变、零虚构 receipt/credit、终态逐项反馈及脱敏、删除/篡改反证、重复和重启幂等 |
| assessment purpose / repair generations | `workorchruntimecontract/runtime_assessment_purpose_preimage_test.go`、`biz/runtime/assessment_purpose_preseal_test.go`、`biz/runtime_validation_input_addition_mainline_test.go` | active/history purpose 隔离、exact binding 唯一反证、repair-only 不授予 assessment、缺失/重复/篡改拒绝；file-backed Run + 真实 File/Shell 的历史 assessment→弱验证拒绝→semantic repair seal→create-only input→fresh validation→assessment→终态投递；SM-2 seal CAS 前后、input receipt 与 fresh validation 提交后重启；终态 duplicate、各切点零额外 provider/预算，旧输入与 startup 不变，SM-1 ownership 不变 |
| target preimage / terminal feedback | `workorchruntimecontract/runtime_target_parameter_binding_test.go`、`blocks/file/target_parameter_test.go`、`biz/runtime/prerequisite_freshness_test.go`、`biz/runtime_result_feedback_test.go`、`biz/runtime/controller_test.go::TestRuntimeControllerInitializeRejectsNoncurrentFeedbackVersionWithoutWrite`、`biz/runtime_result_projection_test.go::TestDeliverRuntimeTerminalRestartsAfterResultPersistenceBeforeOwnerProjection` | SM-3 exact target 派生与 echo 拒绝；SM-2 无进展 guard 保留 root cause 且零额外预算；SM-4 完整且不虚构验收的反馈、非当前版本零改写拒绝、当前版本 partial delivery 重启重放；SM-1 与 capability owner 不变 |
| repair rejection / historical probe lineage | `biz/runtime/repair_rejection_closure_test.go`、`biz/runtime/candidate_owner_conformance_test.go`、`biz/runtime/shell_owner_receipt_integration_test.go::TestShellOwnerObservedFailureTargetBindsThroughEventHubAndFileSnapshot` | first-member/partial-effect rejected repair 同 CAS 退役；candidate correction 的 exact catalog preimage、tamper/协调重封存拒绝、CAS-before/response-loss/restart duplicate；真实 Shell→File correction→effect→fresh validation，历史拒绝不阻塞 current owner fact 推进 |
| Module lifecycle/core dependencies | `plugin_test.go::TestModuleRunBindsFrameworkContextToOwnerLifecycle`、`plugin_test.go::TestModuleSetupRejectsMissingBackgroundRoutine`、`plugin_test.go::TestModuleTeardownCheckedRetainsBlockedOwnerAndRetriesWithoutDuplicateRelease`、`biz/owner_test.go::TestModuleIdentityConfigureStartStopContract`、`biz/owner_test.go::TestModuleConfigureRequiresCoreDependencies`、`biz/owner_test.go::TestFailedStartRemovesAllIngressAndCancelsLifecycle` | identity、Configure/Start/Stop、framework context、required BackgroundRoutine、startup rollback、deadline-aware Stop、canonical evidence repository bundle 与 EventHub dependency fail-fast；真实隔离 PluginMgr + Module/Biz + 阻塞 Runtime read fixture 覆盖错误 guard、未 released/原 Biz 保留、后续插件零释放、解除阻塞后的重试及重复 Teardown；不证明真实 DefaultService→Initiator 的整进程屏障 |
| owner-private dispatcher/CAS/aggregate | `biz/runtime/internal_event_*_test.go`、`biz/runtime/component_facades_test.go`、`biz/runtime/aggregate_invariant_test.go`、`biz/runtime/structure_contract_test.go`、`biz/runtime/failure_authority_transition_test.go`（含 `TestOwnerInfeasibilityAtomicallyTerminatesValidationRepairBinding`）、`biz/runtime/candidate_owner_conformance_test.go::TestOwnerCandidateApplicabilityViolationClosesFactAndCorrectionInOneCAS` | exactly-one payload、primary SM 与 coordinated write-set、single mutation root/CAS、三种 failure-authority policy、candidate feasibility fact+disposition 单 CAS、validation-repair infeasibility 原子退役/terminal、旧 target/directive 不泄漏、duplicate/fresh-load 与 pre/post-CAS failure |
| SM-2 semantic/CWI/binding | `biz/runtime/semantic_*_test.go`、`biz/runtime/cwi_*_test.go`、`biz/runtime/coverage_*_test.go`、`biz/runtime/assessment_*_test.go`（含 prospective effect 后 File `unsupported`、Definition-seeded owner-fact successor、restart/duplicate）、`biz/prerequisite_event_adapter_test.go::TestAcquireRuntimeTargetSelectionPrerequisiteUsesTypedEventHubContract`、`biz/runtime_drive_test.go::TestRuntimeFileBackedNextProofBatchSettlesTargetSelectionBeforeProvider`、`biz/runtime_state_repository_test.go::TestRuntimeStateRepositoryTargetSelectionCommitCrashRestartIsAtomic`、`biz/runtime/cwi_ledger_schema_test.go::TestValidationFailureRepairSemanticRebindingClearsExecutionAuthorityThroughFocusReducer`、`biz/runtime_semantic_drive_test.go::TestRuntimeSemanticFalseUndeclaredPrerequisiteUsesCorrectionAcrossRestart` | all-of、owner fact、typed gap claim 的 satisfied/acquisition/genuinely-undeclared admission、duplicate/stale/ABA、reopen/settle、semantic-pending exact `RuntimeFocusSeed`、batch rollover 的 owner prerequisite→CAS→request identity 顺序与零额外 provider/budget、predecessor execution-authority purge、attempt-1 correction→feasibility→batch-generation seal、proof preimage/fresh-load、false gap 零 governance/effect |
| SM-3 document/candidate/seal | `biz/runtime_candidate_request_content_test.go::TestRuntimeCandidateRequestContentBridgeCompletesDocumentWithoutCandidateAttempt`、`biz/runtime_drive_test.go::TestRequestAndSealRuntimeCandidateNoDocumentUsesBoundedRequestRecovery`、`biz/runtime/candidate_*_test.go`、`biz/runtime/dispatch_*_test.go`；`candidate_owner_conformance_test.go::TestOwnerCandidateConformanceTransitionIsAtomicRestartSafeAndBounded`、`TestOwnerCandidateConformantFactSurvivesRestartAndAllowsDispatchSeal`、`TestOwnerCandidateApplicabilityViolationClosesFactAndCorrectionInOneCAS`；`biz/runtime_operation_admissibility_event_adapter_test.go::TestProbeRuntimeOperationFeasibilityAcceptsManifestBoundCandidateApplicabilityViolation`；`TestRequestAndSealRuntimeCandidateReencodesDeclaredParameterEntriesOnAttemptOne`、`TestRequestAndSealRuntimeCandidateRepairsDeclaredValueRuleOnceAcrossRestart`；`TestCandidateCorrectionFrontierSealsRequiredParametersContainerDefects`、`TestCandidateCorrectionFrontierSealsAndCrossesOperationSetConsolidation`、`TestCandidateCorrectionFrontierSealsCrossesAndRecoversOperationValueRule`、`TestCandidateCorrectionFrontierRejectsUnboundOrDriftedMultiOperationWireRule`；`TestRuntimeCandidateWireDiagnosticRequiresExactPersistedManifestPreimage`；`workorchruntimecontract::TestRuntimeCandidateRequiredFieldTypeCorrectionBindsExactDiagnostic`、`TestRuntimeCandidateProviderWireRejectsNullContainersAndFixedFieldEcho`、`TestDecodeRuntimeCandidatePayloadWithManifestPreservesExactMultiOperationWireRule`、`TestDecodeRuntimeCandidatePayloadWithManifestRejectsDeclaredValueRulesBeforeOwnerIO`、`TestRuntimeCandidateWireContractCompilesAuxiliarySingletonAuthorityLocally`、`TestRuntimeCandidateWireContractRetainsNonSingletonAuxiliaryChoices`；`blocks/llm::TestRuntimeCandidateCorrectionInputAcceptsExactNullRequiredContainer`、`TestRuntimeCandidateCorrectionInputValidatesOperationValueRuleAgainstManifest`、`TestRuntimeCandidateManifestCorrectionProofUsesExactWireRuleAcrossOperationKinds`、`TestRuntimeCandidateAuxiliarySingletonWireIsMinimalWithJSONSchemaResponseOnOrOff`、`TestRuntimeCandidateOutputProjectsExecutableValueRulesIntoC3AndSchema`、`TestRuntimeCandidateRetryProjectsOwnerConstraintOnlyInC4WithoutStablePrefixDrift`；`biz/runtime_state_repository_test.go` 中的 `TestRuntimeMainlineMissingRequiredTargetCorrectionSurvivesRestart`、`TestRuntimeMainlineSchemaCorrectionDocumentCASRestartMatrix`、`TestRuntimeFileBackedSemanticCandidateReplayExhaustionSurvivesCASBoundaries`、`TestRuntimeMainlineCandidateAuthorityExhaustionHasNoCorrectionProviderOrBudgetEffect`、`TestRuntimeMainlineRenderedPreflightStopsBeforeCandidateAttempt` | no-candidate framing 零 attempt、corrupt complete carrier terminal、schema/semantic/value-rule correction、required/auxiliary 最小 wire、multi-operation exact rule identity、operation-set cardinality、schema-first missing/empty/null/wrong-type required field、parameter-entry re-encode、value-rule owner-I/O-before gate、owner conformance/applicability proposal→fact→feasibility→seal、跨组件 typed result exact binding、failure↔correction durable lineage、attempt 0/1、target/binding/effect/dependency；owner fact duplicate/response-loss/fresh restart、atomic violation closure、single retry、correction preflight 零 provider/attempt、document-complete 后 attempt-begin CAS 前中断/CAS 后响应丢失、预算单次扣减、无合法 correction 时零额外 provider/budget、schema rejection 零 dispatch/receipt 均属 SM-3 |
| capability effect/receipt/evidence | `biz/runtime_operation_receipt_source_test.go`、`biz/runtime/capability_receipt_transaction_matrix_test.go`、`biz/runtime/*_owner_receipt_integration_test.go`、`biz/runtime/operation_receipt_test.go`、`biz/runtime/candidate_evidence_*_test.go` | 六类 owner 的 descriptor/source/query 边界矩阵；empty/whitespace/noncanonical/wrong/cross 的 Query/Commit 零 CAS、aggregate 不变；File 的真实 adapter 路径及 Git/Shell/SSH/MCP 的 proof/state-change 路径均覆盖 production-shaped admission→seal→claim→accepted/no-op/rejected Commit/Query；Web 只作为无 CWI credit 的 auxiliary validation 覆盖同一事务，不能被提升为验收证明；retirement fence 与 receipt-loss recovery均有回归 |
| owner-grounded repair | `biz/runtime/validation_failure_repair_test.go`、`biz/runtime/repair_surface_confirmation_test.go`、`biz/runtime/repair_surface_confirmation_plan_test.go`、`biz/prerequisite_event_adapter_test.go`、`biz/runtime/candidate_evidence_repair_transaction_test.go`（含 `TestConfirmedRepairValidationOnlyBindingManifestRetainsSourceLineage`）、`biz/runtime/repair_owner_neutrality_test.go`、`biz/runtime/mcp_owner_receipt_integration_test.go`、`biz/runtime/reducer_test.go` | exact route lookup/no-route same-CAS terminal、唯一 owner-private confirmation plan、request/revision/subject fence、stale refresh、query/CAS response-loss 与 administrative termination rebase、sealed Profile/descriptor→唯一 `RepairOwnerModuleID`、confirmation/operation/receipt owner exact-match、validation-only source binding只保留 causation、零 terminal CWI credit；`confirmed_validation_repair` 仅属于 state-change，fresh validation保留 closure/清空 purpose/可原子 terminal；single/multi all-of、partial/rejected/duplicate/restart/governance；具体 owner specialization不进入 core 分支 |
| governance/clarification | `biz/runtime/governance*_test.go`、`biz/runtime/definition_clarification_test.go`、`biz/runtime_lifecycle_handoff_test.go` | ticket/approval 两个 concrete ingress、无 generic resolution ingress、exact binding、continue/fail、duplicate/stale、response loss、blocked Run ack |
| SM-4 wait/recovery/delivery | `biz/runtime/recovery_budget_test.go`、`biz/runtime/nonterminal_recovery_test.go`、`biz/runtime_nonterminal_recovery_outer_test.go`、`biz/runtime/executor_test.go`、`biz/runtime/drive_lease_test.go`、`biz/runtime/terminal_delivery_rematerialization_test.go`、`biz/runtime_lifecycle_handoff_test.go`、`biz/runtime_drive_result_test.go::TestRuntimeTerminalDeliveryWaitsForCurrentSemanticAuthority`、`biz/runtime_result_projection_test.go` | split CAS budget writer rejection、atomic budget+record+request-bound lease、outer response-loss/exhaustion、singleflight、one-shot gate settlement/rejection handoff、terminal multi-generation/startup rematerialization、timer/Stop epoch、CAS cut points、operation authorization issued→in_flight→settled\|uncertain、claim lease/双 worker/counter/ledger/dispatch fresh-load reverse proof、fresh process、semantic-rebinding provisional terminal 与 SM-3/restart 同分类、budget、three-owner partial ack；`runtime_state_repository_test.go` 中的 correction transaction 不归入 SM-4 |
| Workspace deletion | `biz/workspace_retirement_test.go`、`biz/workspace_runtime_quiescence_adversarial_test.go` | draining/quiescing fence、in-flight writer、retain/purge、pending journal、restart/tombstone |
| LLM request/cache/audit | `biz/runtime/provider_request_recovery_test.go`、`biz/runtime/provider_request_terminal_test.go`、`biz/runtime_candidate_event_adapter_test.go` 与 `../../blocks/llm/biz/runtime_*_test.go` | ceiling、identity、format on/off、submission settlement、raw isolation、C0～C5 stability |
| core neutrality | `biz/runtime/core_neutrality_contract_test.go`、`biz/runtime/semantic_failure_class_test.go`、`biz/runtime_semantic_drive_test.go` | typed feasibility/status 与 descriptor/Profile 驱动分类；不解释 capability owner 常量或私有 diagnostic prefix |
| static owner boundary | `make verify-boundary-contract` | no forbidden surface、no cross-owner service/repository、state owner manifest consistency |

真实 provider、远端 endpoint 和长时间网络故障由
[`deployed-convergence-scenarios.md`](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) 验收；部署事实不能替代
本表的 deterministic contract tests。

显式设置 `WORKORCH_READONLY_RUN_ARCHIVE` 为一个已有 `run.json` 的绝对路径后，可单独运行
`biz/runtime_archive_readonly_test.go::TestRuntimeArchiveReadOnlyCompatibility`。
该诊断只读取并验证原 bootstrap、record 和 aggregate，核对封存 feedback 版本及文件 hash；不启动 Hub、repository、
Runtime drive 或 provider，也不改写归档。默认 CI 不读取本地运行数据；通过该检查不是重新执行或 workspace 验收。
