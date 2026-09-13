# Runtime Candidate And Dispatch Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-09-10

## 规范化候选合同

可选逐原 CWI durable-dispatch 上限属于 SM-3 final seal guard，而非 candidate correction 或新工作授权。
候选通过完整 admission 后，accepted attempt 与 dispatch 在同一 CAS 中封存并占额；耗尽只走 typed budget failure，
不得通过生成另一候选、修改 binding 或治理 ticket 绕过。精确重放不重复占额，计量与恢复合同见
[Execution Budget](execution-budget.md#activation-sealed-execution-limits)。

Runtime 不要求穷举所有模型输出组合。它要求把动态 owner 事实先编译成一份有限、可验证的
`RuntimeCandidateContractManifest`，此后 renderer、decoder、candidate evaluator、owner candidate conformance、owner feasibility 和
fixture 都只能消费这份封存合同，不能再次解释原始 profile、目录、错误文本或临时上下文。

failed attempt 保留 owner feasibility fact 时，Orchestrator 必须在同一失败 internal-event/CAS 内保留其 exact owner
catalog preimage。它仅用于按 sealed manifest snapshot ref、自身内容 hash、operation owner、acquisition 与 observed
revision 反证历史，不构成 current admissibility；pending attempt 仍必须匹配当前 owner catalog。后续 effect 或
prerequisite refresh 不得用新 catalog 重建历史拒绝，也不得因历史 catalog 已退休而否定合法的成功修复。
上述留存同时适用于完整候选拒绝和 SM-2/SM-4 terminal failure 关闭 pending attempt；后者不能只更改状态而遗漏
事实前像，必须与 terminal authority 在同一 aggregate CAS 完成，不产生新的候选或 correction 额度。

每个 manifest 由零到多个 `SealedOperationAuthorization` 组成。该名称表示 manifest 内的一条纯值授权记录，
不是新的状态机或 owner；实现可在现有 manifest DTO 内表达。每条授权至少包含：

- canonical operation kind、owner module、parameter/variant contract；
- operation 及 selected variant 的 executable `ValueRules`：只包含 sealed ref、parameter names、closed rule kind/bounds，计入
  operation-contract revision 与 manifest hash；strict decoder、semantic evaluator 和 final validator 共用同一纯实现，不携带
  raw candidate value，也不触发 capability owner I/O；
- operation 及 variant 可选的 `CandidateConstraints`：两级合并，只允许 sealed ref、parameter name 与 closed required-shape vocabulary，且计入
  operation-contract revision 与 manifest hash；不得携带 raw candidate、parser text 或 Runtime disposition；
- accepted/required effect scope、dependency 与封存的 `CoverageEvidenceBinding` 引用；每个可关闭
  CWI 的 binding 必须精确绑定 CWI、proof shape、target/selector、validation recipe/check（适用时的 metric/artifact）
  与来源 declaration/snapshot，不能用 candidate 自报 CWI 代替；
- 唯一 `TargetPolicy` 以及该策略所需的 exact refs、bounded owner scope 或 owner selector；
- 来源 prerequisite/owner receipt 的 snapshot/revision 引用；
- 仅包含本授权实际依赖的bounded owner revision/snapshot/fence set及其deterministic hash；无关owner revision不得进入
  freshness identity或使该authorization失效；
- manifest hash 与 operation-contract revision。

operations provider document 只提出 manifest 允许模型选择的 kind/variant/parameter/target/binding proposal，禁止携带
`id`、`dependsOn`、dependency graph 或其它执行 identity/ordering 字段；schema 与 strict decoder 必须以
`additionalProperties=false` 拒绝这些字段。strict local admission 先执行 authority transaction exact-match gate，再将 required
proposal 唯一映射到 sealed authorization 与 normalized required-sequence slot；auxiliary proposal 则必须具有唯一 canonical
execution-semantic key。Orchestrator 只从 manifest hash 与上述 manifest-owned slot/key 确定性派生 operation ID，并从
required-sequence dependency refs 派生 dependency edges。wire 数组顺序或下标不授予 identity/dependency authority；ambiguous、
missing、duplicate 或越界 slot/key 只形成 typed rejection，零 feasibility side effect、seal 或 dispatch。

SM-3 还必须从已封存 manifest 纯编译一份 `RuntimeCandidateWireContract`，并对 required 与 auxiliary operation
应用同一条最小 wire 规则。required-sequence slot 只由 `requiredOperationRef` 选中，auxiliary rule 只由 canonical
`kind` 选中；kind（required slot）、唯一 reachable variant 的 discriminator、所有 reachable authorization 共有的
singleton binding、required cardinality-one 且 allowed set 为 singleton 的 target，以及固定 parameter 都由本地
compiler 派生，provider 必须省略。已被 manifest 排除的 variant 对应 binding authorization 不得重新扩大 wire；只有
variant、binding、target 或 parameter 仍存在多个合法值时才保留为 provider 选择。provider 回显本地派生字段、required
container 为 `null`/错误 JSON type、未知字段或提前闭合 root 后继续输出 operation 字段，均是 typed document rejection，
decoder 不做本地补全、删除或 JSON repair；只有 SM-3 持久化同 path、exact rejected type 的 schema witness 后，才可授权
attempt 1 重写一份完整 document。schema、instruction、strict compiler、fixture encoder 必须消费同一份 wire contract，
禁止各自重新推导字段；JSONSchemaResponse on/off 不得改变最小文档或本地编译结果。

capability owner 可在 catalog 声明 typed `TargetParameterBindings`：它只绑定已声明的 optional string parameter 与
bounded exact target preimage，不携带状态或 transition。existing/exact target 的 manifest 必须具备允许集合内每个 target
的完整 literal；singleton operation 选择 target 后，SM-3 纯 compiler 从 sealed manifest/wire 取该 target 的 exact literal，
不能借用其它 target、重新读取资源或由 provider 猜值。prospective target 保留派生参数声明但无 existing preimage，编译时
省略该参数。两者都禁止 provider 回显该参数，即使值正确；echo 是既有 typed document rejection，不是本地 JSON repair。
catalog、manifest、wire hash 均绑定声明和 literal；空扩展不增加 hash 字节，不改写旧 sealed archive。owner feasibility
ingress 重验 authorization 与 catalog preimage 的一致性，并继续验证 current freshness；本地派生不绕过 owner stale、
applicability 或 effect/replay guard。该动态合同只进入 C3，不改变 C0～C2。
非法 target 的 syntax/membership/cardinality 仍由同一 strict boundary 拒绝并保留既有 correction frontier；不能以
“无法派生参数”覆盖原始 target diagnostic。SM-3 若证明某个已授权 target alternative，必须用同一 pure projection
重新派生该 target 的依赖 literal，保证 witness hash 与实际 attempt-1 wire compilation 一致，且不增加任何可选参数。

strict compiler 一旦为 provider operation 选中 wire rule，任何后续 operation-scoped schema diagnostic 都必须携带
`RuntimeCandidateWireRuleIdentity`：exact `wireContractHash`、provider `operationIndex`、`requiredOperationRef`（required slot）
或 canonical `operationKind`（auxiliary rule）以及 `variantId`。该 identity 只是 diagnostic lineage，不授予 candidate、retry
或 transition authority。SM-3 与 LLM correction preflight 都必须用同一 sealed manifest exact 重验它；禁止从 diagnostic
`Actual`、数组顺序所暗示的业务含义或“manifest 只有一个 operation”推断 matching rule。

wire contract 的 deterministic hash 必须绑定 operations `RuntimeExpectedDocumentContract`、request-session manifest
identity 与 candidate attempt 前封存的 exact `CandidateManifest`。未进入 attempt 的 prepared/recovery session 在 restart
时从 current sealed authority 重编译并与 persisted identity exact-match；已进入 attempt 后则还必须从 session 的
exact manifest body 重算。首次请求、attempt 1、duplicate 和 fresh restart 的 hash 必须相同；无法编译、
hash 缺失或漂移必须在 provider/feasibility/effect 前 fail-closed。

目标策略只允许三种，禁止为 live 症状增加第四种临时分支：

| TargetPolicy | 含义 | 必须拒绝 |
| --- | --- | --- |
| `exact_target` | operation 的全部 target 必须属于封存的精确 target set | 空 target、越界 target、运行时猜测 target |
| `existing_owner_scope` | operation 必须提供 concrete target，且每个 target 属于 owner 封存的既有资源 scope | 新建 target、截断/不完整 inventory、只命中部分 target |
| `prospective_owner_target` | 一份 bounded semantic proposal 的 1～8 个 exact target 经 owner fact 整体验证后由 SM-2 binding 封存；operation candidate 只引用该 binding，并按每个 target 的 required sequence 提交；dispatch 从 binding 派生 target set 并命中同一 create selector/constraint | 重新选择/替换/缩小 target set、无 binding target、部分 required sequence、绕过 owner feasibility |

`TargetPolicy` 是 owner-neutral 的纯合同词汇。File、Git、Shell、MCP 或业务 profile 的具体语义仍由对应
owner/profile 提供；Orchestrator 只编译和验证 typed receipt，不按文件名、命令或领域文本推断策略。

validation owner 可在 `RuntimeValidationSurface` 发布 `InputTargetOwnerModuleID + InputTargetRefs`，声明当前观察到的
验证输入资源身份。该事实独立于 recipe/requirement readiness，不是可写目标、repair permission 或 validation credit。
SM-3 从当前 typed owner catalogs 为 `need_state_change` manifest 纯编译 `ExcludedTargetRefs`，并保存来源
`MutationInputCatalogs` 的完整封存前像。它是现有三种 target policy 的负向成员约束，不是第四种 policy；即使
ordinary source binding 的 scope 包含输入、或 scope 只声明 owner selector，输入也不能被普通修改借道写入。
Core 不识别文件名、语言、测试框架或 adapter namespace。来源事实只收紧同一 resource owner 的操作，不跨 owner 扩张授权。
非可选 input protection observation 单独受 `RuntimeValidationInputTargetsMaxCount` 的完整集合上界约束，不借用
32 项可选 owner inventory 或 candidate context budget；超限必须拒绝，不能截断后把遗漏输入留作可写目标。

schema、C3 instruction、strict decoder、target-parameter derivation、candidate evaluator、owner feasibility ingress 和
sealed dispatch/replay 共用同一排除集合；scope/参数 preimage 只能从排除后的合法集合派生。manifest hash 绑定完整
catalog identity，target authorization/dispatch hash 绑定 exact exclusions；fresh-load 必须重验前像与集合，不能清空后
仅重算 hash。来源 catalog 不是额外可选 operation/surface，不得展开为 provider authority；动态约束只改变 C3，不改变
C0～C2。空扩展保持旧 JSON/hash，不为旧 session、dispatch 或 archive 补写新权限/约束。
semantic provider proposal 不得自报该 operation-only 排除字段；共享 target DTO 的复用不能把它带入 SM-2 proof authority。

唯一既有输入写入例外是下述 confirmed-validation `InputRepair` 的 exact criterion-preserving delta：原 binding、
closure/purpose 与单一 input 身份全部匹配，才可从排除集合移除该一个 target；其它输入仍受保护。fresh validation
不继承例外。create-only input addition 不覆盖旧输入，不因创建成功取得 validation credit。普通输入越界仍是 SM-3
既有 bounded candidate rejection/correction；重复越界不能产生 effect、credit 或 clarification ticket。
SM-1 生命周期与 SM-2/SM-4 ownership 不变，不引入第五状态机。这项保护不等同于验证实现的一致性评审；声明
`InputReview` 的替代输入必须经过下述独立 review/binding transaction。
这是 exact owner/operation target 的准入合同，不能宣称为任意 Shell/MCP 子进程隐含文件 I/O 的隔离证明；这些
effect owner 必须在自身执行合同中另行证明资源写入边界，不能从 File target 排除集合推导它们已获得或已受限的权限。

验证输入的一致性评审还必须满足以下独立合同：

capability owner 可在 proof binding 上声明 `InputReview`：支持的协议、完整有界输入字节/hash，以及封存的原准则与
required checks/native mapping。Core 不识别语言、文件名或 adapter 业务语义。LLM 的现有 `semantic_plan/proof_batch`
请求在 C3 获得这些准确事实，对照 C1 原 Goal/DoD，返回 closed `validationInputReview`：subjectHash 和每个原 check 的
`consistent|conflict|inconclusive`、非空 reason、准确 input refs。完整 all-of 映射不能遗漏、重复或换成新准则。
文件注释、候选自身声明、自测通过和 ticket 摘要均不产生一致性或验收 authority。
评审输入范围与执行范围必须由同一 capability owner 定义并按原 native mapping 版本化封存；Core 不得为了容量截断
内容、替 owner 选择文件或把独立 CWI 的输入合成新的全局授权。超出 owner 完整输入闭包的来源保持不可验证；局部
检查执行不得冒充原准则要求的完整工程验证。新默认只影响新 activation，旧封存协议与归档保持原解释。
可选 `ScopeContractRef + ScopeRequirements` 成对声明 owner 执行/观测范围，分别最多 512/4096 bytes，完整 UTF-8 且
无控制字符；二者进入 subject hash 与完整 C3 integrity 校验。范围改变使旧报告失效；空扩展不改变旧 JSON/hash。

新 activation 可显式声明 `RecipeInputReviewProtocolRef=validation/input-consistency/v2`；接收 owner 必须原样确认到
recipe 的 `InputReviewProtocolRef`，再发布相同协议的 subject。所有 recipe commitment/hash 绑定该可选字段；空值
保留旧 JSON/hash，未知、降级或未声明的 v2 不得进入 catalog/dispatch。
v2 每个 check 必须包含 1～16 项 `obligations`，字段为 `criterionText/finding/reason/inputTargetRefs`；finding 只允许
`covered|missing|conflicting|unknown`。原文摘录须覆盖原 CWI 准则全部非空白字符，不能换词、遗漏合取项或限定词；
报告 JSON 最多 32 KiB，reason/excerpt 各最多 2048 bytes。文本覆盖并不证明语义蕴含，引用准则也不是执行证据。
missing/conflicting 导出 conflict，只有无此两类而存在 unknown 时为 inconclusive；没有负项才可 consistent。
SM-2 将纯覆盖遗漏单独记录为 `validation_input_review_coverage_incomplete`，仍不能 seal validation binding 或进入旧
validation-failure repair。当前评审合同不授予发布前候选的写入、已发布输入的自动合取补充或准则替换权限。

SM-3 按原 request session 持久化完整报告；SM-2 的 `SemanticFeasibilityRequired` 私有事件在 owner feasibility 前分类。
一致才继续原 binding/reconciliation seal，冲突明确失败。完整无法判断封存 exact governance dependency，复用本地
governance dispatch 和 Decision ticket，不新增 provider 调用或状态机。ticket 信息 resolution 的同一次 SM-2 CAS
将旧 semantic generation 以 `input_review_information` 留存，重入 `required`；新 manifest 只能携带相同 subject 的
preceding resolution facts，以新 request 重新评审。原 Definition/startup、旧输入/报告不改写，review/resolution 零 credit。
SM-4 只交付既有 continuation，SM-1 仍由 Run owner 处理。每 Run/exact subject 的信息补充最多一次，仍无法判断明确失败。
v2 在此基础上按 Run/原 CWI/check 反证 preceding 信息补充记录，输入、路径、native/recipe/catalog 变化或进程重启不能
重置一次性预算。fresh-load 重验历史消费和当前治理 dependency；旧 subject 事实仍不得用于另一 subject。
格式、内容不完整、身份漂移和 provider transport/recovery 属于既有 bounded request/correction，不得伪造业务不确定性开票。

`runtime_validation_input_review_test.go`、LLM `runtime_input_review_projection_test.go` 和 Orchestrator
`runtime_validation_input_review_mainline_test.go` 分别证明纯合同、完整 C3 与缓存不变、真实 capability/Decision + file-backed Run
的执行和恢复边界；脚本化 provider 证据不得登记为真实 LLM 或 release DCE 验收。
`runtime_validation_input_obligations_test.go` 覆盖完整原文映射、缺项/冲突/不明分离、严格 wire 与降级拒绝；
`TestDriveRuntimeInputReviewV2OmissionAndRestart` 与使用当前模板的 `TestDriveRuntimeIsolatedInputReviewMainline`
证明真实 owner 的零越权、信息复核、原输入保留、终态反馈和持久化切点。

三种策略的 operation target 校验必须只有一份 owner-neutral 纯实现，并由 manifest strict decode、candidate
admission、owner feasibility preflight、dispatch seal/fresh-load 与 receipt credit 共同调用。owner receipt 的 `TargetRefs` 必须先与其 sealed
operation 的 `TargetRefs` 精确相等，再用同一策略验证该 operation 是否处于 binding/manifest 授权范围内。对于
`exact_target`，封存集合是 bounded membership set；一个合法 singleton operation/receipt 不需要等于整个集合，也不构成
`one_of`/`any_of` terminal 关系。若一项义务要求完整 target set 全部完成，必须由 sealed required sequence/collective
receipt-set 或独立 CWI `all_of` 明确表达，禁止只在 receipt 阶段把 membership 偷换为 full-set equality。

`confirmed_validation_repair` 同样遵守该规则：`Decision.RequiredOutput.TargetRefs`、resource-owner
`RuntimeRepairSurfaceConfirmation.ConfirmedTargetRefs` 与 repair commitment 的 `Closure.GroundedTargetRefs` 必须精确保存
同一份完整 confirmed surface；它是 state-change candidate 的有限授权集合，不是“每个 target 都必须修改”的隐式 operation
清单。candidate 必须选择该 surface 内的非空 target 子集，所有已封存 operation/receipt 仍按 `all_of` 完成，最后一项后才可
生成 target-free fresh-validation directive。`confirmed_validation_repair` 只标记前一项 `need_state_change` repair-effect
manifest；fresh-validation manifest 必须保留 closure ID、清空 purpose，并作为普通 Definition validation 编译，不得要求第二份
repair commitment。若某个 repair 确实要求完整 surface 全部产生 effect，SM-2 必须事先编译显式
`RequiredOperationSequence`/collective receipt-set；LLM candidate、commitment 派生或 receipt 校验均不得自行把 membership
升级为 full-set equality，也不得把 closure 缩成 candidate 选择的子集。

独立 Profile `RequireInputRepair` 的 criterion-preserving 特例必须携带 validation owner 的 typed `InputRepair`，
精确绑定原 required/protocol/native/supplement check、单一输入与 before/after content hash。它仍使用
`confirmed_validation_repair`：manifest 只允许一项 operation，commitment 必须保留与 original receipt 相同的 delta，
feasibility/dispatch execution 的 resource-owner `ReplayProof` 必须证明两个 exact hash。final seal 与 fresh-load 均重验
这条 lineage；仅 target membership 相同不够，不能夹带断言、expected value 或其它字节修改。Core 只验证 typed identity，
不解析语言或 compiler 输出。修复不贡献 validation credit，后继 fresh-validation manifest 不携带 input-write authority，
原检查和 supplement 仍不可互相替代。缺省 extension 不改变历史 JSON/hash，不能为旧 dispatch 补写该 authority。

target-free 指不继承 repair action target，不表示 validation 无 target 或放宽 manifest。SM-3 必须复用 aggregate
fresh-load 的 completed repair/confirmation/receipt/current-binding 反向证明；只有 exact handoff 才把旧 closure targets
视为因果记录，并使用普通 Definition validation manifest 的 target/binding 授权。修复文件与 validation entry 可以不同。
同一证明产生的 final repair evidence revision 也是拒绝重放的因果边界：即使修复没有 CWI credit，边界之前的失败验证
也不能阻止在新状态上重跑同一 check；边界之后的新 rejection 仍受重放限制。不得仅凭 `need_validation`、空 purpose/target
或 canonical-looking directive ID 放宽任何准入；partial repair、缺失/漂移的 confirmation、receipt、binding 或 closure 均不能
产生这份 invocation-local proof。该投影不是第二份持久化 phase，不能改写历史 closure、receipt 或 startup 配置。

repair operation 的 source `BindingRefs` 是 immutable validation-failure causation，不自动成为该 state-change receipt 的
coverage proof。若 matching source binding 没有 `state_change` proof，Orchestrator 必须从 sealed
`RuntimeDispatchRepairCommitment` 将其分类为 causation-only：accepted fact 保留 binding、保持零
`CoverageWorkItemIDs`，owner `EvidenceReason` 与 receipt exact-match，且不得生成 `coverage_binding_mismatch`。该例外不适用于
普通 dispatch；不存在 exact repair commitment 的 binding/proof mismatch 仍按 SM-2 contract defect 收口。

final SM-3 seal 必须把 candidate manifest hash 与每项 operation 的最小 target-policy preimage 一并封入
`DispatchAuthorityHash`。该 preimage 只可由 final manifest seal 生成；Run 只 opaque 持久化，governance resume 只复制，
fresh-load 只重验，任何路径都不得从 mutable current facts 重编译、增量修改或把它提升为第二 target owner。缺少该 sealed
preimage 的 operation dispatch 直接按无效当前合同拒绝，不保留旧 record 兼容分支。assessment 的 purpose 也必须先写入
request-session `CandidateManifest`，并以 exact semantic manifest/provider document/owner feasibility batch 为独立 preimage；
dispatch 的 `AssessmentAuthorityInput` 只能在一次性 final seal 中消费该 purpose。repair commitment 及 governance continuation 的
source/grant/execution projection同样必须在首次 hash/Validate 前完成，seal 返回后不得再写 authority 字段或重算 hash。

assessment purpose 的纯解析必须先在 active/history accepted batch 中找到唯一 exact binding preimage，再解析该证明的
obligation purpose。候选 manifest 构建和 accepted-assessment fresh-load 校验共用同一算法；合法但无关的
`validation_failure_repair` generation 不能否定历史 `definition_proof` 或 `owner_fact_acquisition` assessment。
若匹配的证明本身为 repair-only，则不得创建或接受 assessment；missing/duplicate preimage、未知合同、manifest/document/owner
fact 漂移仍 fail closed。invocation-local index 只重放历史事实，不跳过坏记录、不增加持久化字段，也不改写归档或授予 CWI credit。

同一 locally compiled candidate/dispatch 内，每个 durable operation 必须独占 canonical
`(OwnerModuleID, TargetRef)`。两个普通 operation 即使由 manifest 派生出 dependency edge，也不能先后修改同一 owner resource，因为它们的
target snapshot/replay proof 都在第一个 effect 前封存；provider 必须把同一目标的完整变化合并为一个 operation 和一个
receipt。唯一例外是 owner fact 明确封存的 exact `RequiredOperationSequence`：其中每项必须绑定不同 requirement ref/variant
并按本地派生的依赖执行。prospective all-of binding 的 sequence 必须与完整 target set 一一对应：每项携带不同 requirement ref、
一个 exact `TargetRef`，且 admitted proposal `targetRefs` 必须为该值的 singleton；缺少、附加、重复或无法唯一映射均拒绝。
不同 owner/target 的 operation 仍可按 manifest-derived dependency graph 确定性执行，non-durable operation 不参与该约束。

该 sequence 只有一份 collective completion proof：每个 target 的 operation 都必须在同一个 sealed dispatch 内产生
owner-committed `accepted` exact receipt，并由 Orchestrator private evidence transition 将 matching execution 原子提交为
`evidence_committed`。单个或 partial receipt 只证明自身 singleton target，不能 credit/settle 整个 binding；rejected、stale、
weak/mismatched receipt 不计入集合。duplicate 只复用同一 `ExecutionKey + Fence` receipt，restart 只恢复同一 dispatch 的
persisted members；不同 dispatch、repair 或 semantic generation 的 receipt 不得合并。仅完整集合可由 SM-2 exactly-once
认可为 collective proof；CWI credit/settlement 还必须通过其余 quality/effect/assessment gate 和 aggregate CAS。任何
capability result 都不能携带 collective/settled target state。

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
kind/variant、exact target set 或 sealed selector、recipe/check、适用 metric/artifact 与 declaration/snapshot lineage。
managed-validation proof 还必须保存 Definition-owned、recipe-neutral 的 `RuntimeValidationProofRequirement.Hash`，并与
Shell-owner surface 为当前 ready recipe 返回的 exact per-CWI applicability binding 一致；recipe ID、surface-wide check
union 或成功退出均不构成该关系。该 owner binding 还必须逐 check 封存 opaque observation protocol/native check mapping，
并进入 dispatch execution seal；只有 owner receipt 的 matching typed attestation 可以满足 required check，raw process
marker 只能作为诊断。
可选 `SupplementalNativeCheckRef` 是同一 required check 的第二个 conjunctive native input，不能替代原检查；两者
必须在同一 sealed protocol 下独立 executed/passed，任一 skipped/failed/incomplete 均拒绝。该字段必须进入 owner
catalog、binding、dispatch、proof lineage 与 reconciliation hash，不能跨 CWI 复用；缺省字段保持旧协议身份。
若 exact native check 或 recipe input 尚不存在，capability owner 可为该 requirement 发布独立
`ProofRequirementPreparation`。它只允许经 Profile 已声明的 owner-fact acquisition closure 进入 prospective all-of
transaction；candidate 可见 exact target 与 opaque native identity，但不得把它当作 proof。receiving owner 完成后必须
重新取得 capability catalog 并由 SM-2 seal 新 binding，不能由 receiving owner、provider 或 Application 直接把
preparation 升级为 credit。
已有输入不合法且不能经该 create-only acquisition 补齐时，owner 必须发布独立的 exact requirement unavailability，
而非另一条“准备文件”路径；它不能进入 `OwnerFactAcquisition` obligation、prospective satisfaction 或新 candidate。
SM-2 在 current owner facts 结算后提前封存原始原因并有界停止，不放宽 fresh-validation 的 `definition_proof` guard。
input addition 的 owner content contract 与 exact path/check 一起传给生成方；文件 effect 不证明验证结构/行为已经通过，
fresh owner reobservation 仍不可省略。
当前 Runtime 对 validation proof 采用一 focus 一 CWI 的保守形状；共享 recipe 只有在每个 CWI 具有独立
requirement/check/target 并编译为独立 focus/binding 时才允许。Profile wildcard 或 focus 级泛化 recipe/check 不得由一个 owner receipt 扇出关闭
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

proof batch 只能在 current focus 的 semantic prerequisites 已持久化结算后打开。尤其 `EligibleFocusProjected` 在同一 accepted
coverage order 中推进 CWI 时，新产生的 focus-scoped `TargetSelection` 必须先经 typed File owner result 和 SM-2 private CAS
进入 fresh aggregate；不能先创建 batch/request session、再依赖 provider 前的最终 guard 补结算。否则 owner observation 会改变
authority transaction，使已封存的 batch identity 失效。该窗口保持零 provider call、零 attempt、零 provider budget debit。

proof batch 的 `RequestLogicalID` 是 batch 打开时即封存的 semantic generation identity；attempt 1 correction 只把 ledger
当前 `LogicalRequestID` 推进为新的 provider request-session cursor，不得改写 batch identity。最终 accepted document 由同一
batch 的 `DocumentHash` 独立封存，因此 SM-2 seal 与 fresh-load replay 都必须消费 `batch.RequestLogicalID + DocumentHash`，
禁止用可变 ledger cursor、最后一个 request session 或下游 binding ID 替代。

fresh-load 不能把已经互相一致的 Focus、CWI binding ref、reconciliation、candidate session、accepted attempt 与 dispatch
projection 当作 semantic authority 的自证。Orchestrator 必须逐个 replay active/history 中每个 accepted semantic batch：只使用
persisted manifest、该 batch 封存的 generation `RequestLogicalID`、exact-hash provider document，以及同 batch 已归档或仍封存的 capability-owner
feasibility facts 调用唯一纯编译器 `CompileRuntimeSemanticBindings`。replay 结果必须与 current/remaining/completed Focus、CWI
`CurrentBindingRef`/`BindingLineage`、由 current refs 编译的 reconciliation contract，以及 session/attempt/dispatch 的完整
coverage binding projection exact-match。缺失/重复 batch preimage、manifest/document/owner fact 漂移，或协调改写所有下游
projection 与 self-hash，均必须在 owner call、provider call、transition 与 delivery 前 fail closed。该 invocation-local replay
inventory 不持久化、不选择 action，也不成为另一份 binding ledger 或 owner。

### Prospective owner-fact composite feasibility

当一个 sealed owner-fact acquisition 使用 `prospective_owner_target` 时，普通“File 可创建该 target”的 observation
不足以证明 acquisition 后 required owner fact 会成立。pre-seal convergence declaration、semantic proof obligation 和
provider projection 必须携带同一个非空 `ProspectiveSatisfactionContractRef`；其它 target policy 禁止携带该 ref。
declaration/obligation/manifest 都不封存 exact target set；SM-3 只能提交一份 bounded semantic proposal，其 typed
`ProspectiveTargetRefs`（JSON `prospectiveTargetRefs`）包含 1～8 个唯一 canonical exact target。集合中每项都必须位于
Definition declaration selector/candidate refs 与 current operation selector/owner constraints 的交集内，非空 candidate refs 要求逐项 exact member
match。required owner catalog 同时发布 provider-safe typed `ProspectiveTargetSatisfactionConstraint`，只允许 mandatory
`all_of` clauses；每个 clause 至少由集合中一项完整满足，每个 target 也必须对至少一个 clause 有贡献。不存在
`one_of`/`any_of` 关系，singular `ProspectiveTargetRef` 与兼容 alias 均由 current decoder/schema 拒绝。
generation-specific acquisition/manifest 完整封存并在 C3 投影该事实，dependency recipe identity/command 仍不可选择。

SM-2 对每个 proposal 只接受以下 composite factual chain：

1. acquisition owner（当前为 File）按同一个 current complete workspace manifest、exact File catalog/observation 对完整 target
   set 一次执行 path、absence、parent conflict 与 aggregate-capacity admission，并生成 immutable、
   base-workspace-tree-revision-bound 的 `RuntimeProspectiveWorkspaceManifestDelta`；delta 只描述完整 hypothetical resulting
   target refs，不创建目录或文件，也不返回可先执行的 partial subset；
2. Orchestrator 把该 delta、activation-sealed exact recipe declarations，以及 required owner（当前为 Shell）的 exact
   dependency catalog/observation交给 required owner 自有 concrete typed pure query；
3. required owner 独立验证 contract ref、recipe command/ordered args/timeout schema identity、catalog/observation 与 delta
   lineage，并且只基于 delta 的 resulting manifest 返回嵌套的 `would_satisfy | would_not_satisfy | typed stale`
   observation；它不读取 File/workspace、
   不执行 recipe、不开 session，也不选择 Runtime transition；
4. Orchestrator 只在同一个 SM-2 private event/CAS 中提交完整 File outer feasibility、delta 与 required-owner nested
   observation，并在 CAS 和 semantic seal 两个边界重验 manifest/batch/request/delta 以及两组 catalog/observation freshness。

semantic manifest 因此必须把 provider-selectable `OwnerCatalogs` 与 non-selectable
`OwnerFactDependencyCatalogs` 分开。后者只封存 prospective fact judgment 所依赖的 required-owner catalog snapshot，
进入 manifest/hash/freshness/CAS/seal，但不得进入 LLM C3 owner catalog、allowed operation/surface、schema enum、correction
allowed values 或任何 provider choice。LLM 只能提出 manifest 已授权的 acquisition proposal，不能选择 dependency catalog、
required owner recipe readiness 或 satisfaction outcome。

只有 exact-current `would_satisfy` 可进入 seal-ready feasibility set。required owner 返回的 typed stale 必须保留为 nested
owner-authored observation，不能伪装成 acquisition owner 的 outer `stale`，也不能直接终态化；SM-2 通过既有
`semantic_contract_required` owner-private transition消费一次 persisted prerequisite-refresh generation，退休精确相交的
dependency catalog/observation并重获新 manifest/request。missing dependency catalog/observation 在 probe 前同样先退休
generation，零 required-owner query。refresh 耗尽才以原始 diagnostic fail-closed。stale 只更新 owner
fact freshness，不记录 failed target、不改变 semantic attempt，也不消耗 correction ledger。

schema 与 document admission 使用同一 sealed constraint 在 owner probe 前校验完整 exact proposal target set。candidate refs
的 `const/enum`、all-of clause coverage 与“每个 target 均有贡献”必须同时成立；empty/partial/extra/duplicate、越过 8 项或
singular 字段形成 manifest/CWI/field-path-bound typed violation，并只可
消费普通完整文档/schema 的 sole attempt-1 correction。C3 保留唯一 constraint authority，C4 只引用违规 field，不复制
constraint；整个分支零 File/Shell query、binding、dispatch、effect、coverage 或 ticket。

`would_not_satisfy` 不是 target replacement 信号。Orchestrator 仍先以 `semantic_feasibility_observed` 持久化 exact-current
File delta + Shell observation；该 target set 不产生 binding。由于 admission 已证明它满足 required owner 发布的 typed
constraint，完整 composite `would_not_satisfy` 表示 published contract 与 owner judgment 自相矛盾。下一 owner-private drive
必须记录 `prospective_satisfaction_contract_violated` 并收口为 `semantic_contract_failed`，不得生成 `Allowed`、
replacement proposal、第二 provider request 或第二次 owner probe。partial File delta、单独 Shell reason、
free-form diagnostic 与 stale 仍不得触发 terminal；typed stale 只走既有 bounded refresh。duplicate/restart 不重复
terminal 或同一 proposal query。

`would_satisfy` 的 full composite fact 通过 SM-2 private event 后，只有 sealed `CoverageEvidenceBinding`
对完整 exact target set 具有 authority。后续 operations document 的 schema 只把 binding ref 作为选择面；SM-3 将每个
target 编译为 `RequiredOperationSequence` 中一个 exact-singleton `TargetRef` requirement，并用 canonical dependency chain
固定完整顺序。candidate evaluator、`SealedOperationAuthorization` 与 `SealedDispatch` 从 current binding 派生/校验该
sequence；它们不直接消费 semantic proposal 或 File/Shell fact，也不产生第二份 target authority。

每次semantic-plan或operations provider call都必须使用SM-3 persisted request session形成purpose-bound invocation seal；
seal不是另一份activation authority。对应call、correction、semantic generation和operation consumption必须引用
[Execution Budget](execution-budget.md)中的exact sub-ledger identity。
semantic-plan 与 operations 的 rendered preflight 必须先于 exact request-session CAS；因此同一 authority transaction、stable prefix、manifest、
correction hash 与 expected-document attempt 的 session 是 restart 已通过 preflight 的 durable proof。prepared、recovery-pending、document-complete 或
已绑定 pending proposal 的恢复不得重发 preflight，也不得重复扣减 correction budget；任一 identity 漂移都不能复用旧
session。prepared 只可继续首次 physical submission；recovery-pending 只按原预算授权新 physical identity；已提交但无持久化结果只能
封存 unknown，complete 只继续 local admission。document-complete 后 attempt-begin CAS 前中断与 CAS 后响应丢失必须分别恢复为“无新 attempt”或“原 pending attempt”，
两者都只继续 local admission/feasibility/seal，不能产生第二次 capability effect。

semantic coverage-order、proof-batch 和 attempt-1 correction 的 private transition 只封存 manifest/batch/correction 与 logical
request 意图；渲染预检统一在后继 request path 执行，通过后才 ensure session。新请求的 causal context、correction/content 或
rendered preflight 拒绝使用既有 SM-2 `semantic_contract_failed`，不能伪造 physical settlement 或留下 prepared session。
session CAS 前中断可重新做无副作用预检；CAS 后响应丢失必须复用同一 session。已有请求的 unknown/terminal 仍遵守同 CAS
的 SM-3/SM-2/预算结算合同，不为此增加 machine、owner、兼容字段或重试额度。

semantic session writer 在自己的 private event 内 fresh-load，并反查当前 request-open/correction 意图与完整
ExpectedDocument；仅在 required、order-accepted 或 proof-batch-document-accepted 不得新建请求。rendering 与 writer
共享 owner-local 纯投影，不能分别选择 batch、CWI membership、coverage-order、修正轮次或 output/recovery budget。
新建和复用都必须匹配当前 ledger logical identity，不能通过覆盖传入 ExpectedDocument 的身份字段或重算 hash 接受
旧预检结果。拒绝发生在 session append/CAS、physical allocation 和 provider call 之前。

operations document 被接受时，SM-3 必须先从 exact manifest + candidate 前像独立生成并持久化
`AdmissionTargetAuthorizations + TargetAuthorizationHash`；若选择 `validation/managed_session`，同时生成并持久化
`AdmissionValidationRecipes + ValidationRecipeHash`，完整覆盖 recipe command、ordered args 与 timeout。随后才可构造
dispatch。restart/duplicate admission 必须将 accepted commitment 与 active/completed dispatch 做 exact 双向匹配；同步改写
dispatch projection 与 `DispatchAuthorityHash` 不能改写 accepted admission。普通 File/MCP candidate 的 recipe projection 必须为空。
operations request session 必须在 accepted seal 之前保存完整 `CandidateManifest` body；持久化 body 必须 raw-canonical，不能让
normalize 消除 duplicate、顺序、空白或 nil/empty 漂移。accepted attempt 和 origin dispatch 的 target authorization、coverage
binding、validation recipe 必须从该 body 与 persisted candidate 重新派生并 exact-match；missing/duplicate session、body/hash
漂移或三类 projection 的协调改写都 fail closed。assessment body 额外封存 `AssessmentPurpose` 与 semantic-manifest hash；
fresh-load 从 immutable semantic batch 重放 purpose，禁止 EventHub/LLM/evaluator 从 `CandidateContext.Resources` 合成副本。
governance resume 不生成新 candidate authority，只逐 hop 继承 origin seal并在唯一 final seal 前完成 continuation projection。

普通 owner feasibility 的 `stale | prerequisite_required` 先以 `semantic_feasibility_observed` 单独持久化 owner factual observation。
只有带有 expected/observed revision 且精确匹配当前 catalog dependency 的 `stale` 事实，才能复用
`semantic_contract_required` private transition，按 persisted refresh generation 原子退休 matching catalog
observation、保存完整 gap/diagnostic lineage，并回到 bounded catalog acquisition。`prerequisite_required` 不得被
泛化为 stale、不得退休仍然有效的 catalog/observation，也不得消耗 prerequisite-refresh budget；当前 semantic
proof 没有 owner-defined typed prerequisite request，因此 SM-2 必须在保留该事实和完整 diagnostic 的前提下
fail-closed 为 typed terminal。未来若增加 prerequisite acquisition，必须先新增接收 owner 定义的 typed command/result，
再由同一 private transition 接入，不能复用 stale refresh。以上处理不是新的 phase 或第五状态机，不增加 semantic
candidate attempt。新 catalog revision 必须产生新 manifest/request identity，旧 complete session 只保留 audit；
generation history 的 identity 是封存的 `manifestHash + logicalRequestID`，所以同一 active generation 内的
correction/focus request-session 变化不会制造重复 history，而资源回滚重现旧 catalog hash 时可以用新的 batch generation request
建立独立 generation。每个 focus/binding 的 generation pair 必须取其 accepted batch 的
`manifestHash + batch.RequestLogicalID`；同 manifest 的新 batch generation request
必须生成不同 focus/binding ID，而同一 pair 的 restart/replay 必须稳定。duplicate generation 不写 CAS，refresh budget
耗尽才以原始 owner diagnostic 进入
`semantic_contract_failed`。

catalog acquisition 已标记的 validation-surface `prerequisite_required` 不进入普通 provider-selectable catalog 或普通
operation feasibility transition：正常 provider request 看不到该 identity，也不会为它创建 executable operation 或调用
endpoint。唯一当前例外是 pre-seal 已声明 `ProspectiveSatisfactionContractRef` 的 owner-fact acquisition；该 surface 只能
进入 `OwnerFactDependencyCatalogs` 并由上述 concrete typed pure query 判断 hypothetical delta，不成为 LLM choice。
其它自动 input acquisition 仍必须先新增 owner-defined typed command/result，不能复用该例外或扩大 dependency catalog。

### 单一 candidate evaluator

所有 provider output 先经过 LLM owner 的 typed framing observation。coherent completed response 的 empty final output、
non-JSON、scalar/array root、multiple JSON roots或trailing bytes 是 no-candidate observation，不是 provider protocol failure；
它们不进入 decoder/evaluator，而由 SM-3 的 request session 使用 bounded whole-document recovery，始终为零 candidate attempt。
恰好一个完整 JSON object 则无条件经过 sealed manifest 驱动的 strict local decoder/schema admission；provider JSON Schema
仅可提前减少坏输出，不能跳过或改变本地裁决。operations wire 固定使用 root `operations` array；每个 operation 的
`parameters` 也固定为 `{name,<typed-value-field>}` entry array，禁止 object/map 编码。C3 canonical layout 在 required sequence
存在时展示完整 required slots；只有 auxiliary operations 时只展示一个非强制 shape example，不能把所有可选 operation
渲染成隐含必选集合。

通过 strict local admission 的完整 candidate document 只允许进入一个纯 `CandidateEvaluator`。它的输入是 persisted manifest、decoded
candidate 和同一 authority contract 的 prior attempt fact；输出只能是 `accepted` 或一个完整的
`CandidateRejection`。owner feasibility 是 capability owner 的独立 typed factual observation，不是
`CandidateRejection` 的子类。decoder、admission、owner-conformance 与 owner-feasibility adapter 可以提供 typed observation，但不能各自
选择 retry、terminal、governance 或下一 action。

该 evaluator 必须先执行 manifest-only 的 provider-selectable 字段、binding、target、依赖与 operation-set 校验，再执行
sealed activation 的 parameter bound、variant 与 owner-published effect policy 校验；两层都必须在任何 owner probe 前完成，
并由 final dispatch admission 原样重复。final seal 不得成为某项可执行合同的首次拒绝点。variant restriction 按 operation
kind 生效：该 kind 的合法集合是 `AcceptedOperationVariants` 与 exact `RequiredOperationSequence` variants 的并集；未在
二者中声明 variant 限制的另一 accepted kind 仍保持合法，不能被全局 variant 集误伤。

普通 JSON/schema/manifest admission 接受 candidate 后，SM-3 必须先持久化完整 proposal，再按 manifest 中当前 variant
实际选择的 `CandidateConstraints` 取得 exact owner fact。`parameter_shape` 只允许通过 receiving owner 定义的 concrete typed
conformance command/result 取得 `conformant | nonconformant | owner_contract_defect`：owner 可以复用其执行路径的同一
parser/matcher，但不得读取或修改 workspace。`target_applicability` 则只能在 ordinary typed feasibility 中以只读方式观察 current
target/revision，并返回 `candidate_constraint_violated` 或正常 feasibility outcome。owner 在两条路径都不得生成 effect/receipt、
选择 retry/terminal，或返回 raw parameter/parser diagnostic。当前 File `patch/apply|patch/preview` 的 `unified_diff` 完整
header/hunk 规则由 File operation manifest 投影到 C3；static conformance 验证 single-file syntax 与 header/target 一致，
通过 dynamic feasibility 验证 diff 可应用到 owner-observed target；
`patch/preview` 的 content-sensitive variant 同样必须先取得 current target snapshot。Core 不解释 File code 或 parser message。
未声明相应 constraint 的 operation 不调用对应通道，也不持久化占位 record。

`file/edit` 的 operation-level 与 patch `text_replace` variant-level 均声明 exact `oldText` applicability；
File 复用同一只读执行 matcher 检查匹配、歧义与 occurrence。当前 revision 下不适用只返回 typed constraint violation；
baseHash/resource 错误保持 ordinary feasibility。final seal 不得因缺少 prerequisite 而跳过已声明 dynamic constraint，
也不得为了 variant-less operation 新增伪 variant。纠正仍只允许同一 manifest 的 attempt 1，且不能改变 File effect replay class。

两类 owner fact 与普通 resource infeasibility 正交：static conformance 证明 candidate representation 满足 immutable operation
contract，dynamic applicability 证明同一 candidate 满足 manifest-declared、provider-safe 的 current target constraint；其它
feasibility 才描述 resource、policy、base hash 与 replay prerequisite。它们都以
`LogicalRequestID + operation + manifest constraint` 作为 pending attempt 的 restart fence；final seal 必须逐项 exact-match。若
static `nonconformant` 或 dynamic `candidate_constraint_violated`，Orchestrator 以唯一 private SM-3 event 在同一 aggregate CAS 内同时
持久化 bounded violation 与 attempt failure/correction，禁止出现“已有 owner fact、尚无 disposition”的中间态。attempt 0 只可
产生一次同 manifest `owner_candidate_constraint` correction；attempt 1 再次违反直接 `candidate_retry_exhausted`。owner
`owner_contract_defect` 与 command/result identity 漂移均 final，不能降级为 `policy_disallowed` 或普通 resource infeasibility。

为取得 feasibility observation，Orchestrator 只可先用 operation kind 解析 manifest 中的 owner 和
authorization identity；该步骤不做 candidate admission。无法解析的 kind 不调 owner，由 evaluator 在
kind 层拒绝；可解析的 operation 可调用无副作用 owner probe，owner 只返回
`feasible | stale | prerequisite_required | candidate_constraint_violated | infeasible | owner_contract_defect` closed observation。
其中只有 exact manifest-bound 的 `candidate_constraint_violated` 进入上述 candidate correction；其它 feasibility outcome 不进入
attempt correction ledger。候选的结构性 evaluator 必须先按固定顺序
返回 wire/parameter/target 违例；只有结构性校验接受后，Orchestrator 才将 owner observation 作为独立后继处理。

`CandidateRejection` 至少包含：原始安全 class/code/message、provider document operation index 与 matching manifest slot
（若适用；strict wire rejection 使用 exact `RuntimeCandidateWireRuleIdentity`）、field path、actual、
allowed/required/forbidden、manifest hash、rejected shape hash 与完整 scope/lineage。它是 evaluator 的事实输出，
不得携带 retry、terminal、governance、Runtime action 或 `CorrectionDisposition`。

strict decoder 若已得到 current wire 的完整 typed candidate、但同一次 manifest semantic admission 拒绝它，可以把该
detached typed value随 failed `CandidateAttempt` 保存为 correction frontier 的前像。该值只服务于同 owner 的有限差异证明，
不得进入 accepted proposal、owner feasibility、sealed dispatch、projection 或任何 authority hash；framing 尚未形成单一
JSON object 时根本不存在 `CandidateAttempt`，closed-schema 尚未形成 current typed candidate 时 preimage 必须为空。fresh-load
只能重验这份 persisted failed-attempt preimage，不能从 raw
provider body、diagnostic 或 mutable manifest 重建。

SM-3 在收到 rejection 后，必须先把它转换为 owner-local internal event 并持久化；随后只能根据 current
manifest、attempt ledger、rejected shape 与 typed field violations 决定：

| SM-3 owner-local result | 语义 | 唯一行为 |
| --- | --- | --- |
| `correctable_once` | 单一 JSON object 在同一 manifest 下存在可验证的 schema/parameter-entry/operation-value-rule/owner-constraint/operation-set correction，或有限 frontier 中存在已封存的合法语义差异 witness，且尚未使用 correction attempt | 将 manifest hash、目标 delta 和 witness 随 attempt 0 持久化，只允许 attempt 1 |
| `rejected_final` | 没有可证明的同合同语义差异，或 correction attempt 已用尽 | 形成 typed candidate terminal，零额外 provider call |

每个生产 rejection producer 的 code 必须由 Orchestrator 的单一 disposition policy 显式分类，并由源码生产分支导出的
验证清单覆盖，不能只验证一份手工抄录的 code 列表。candidate 已通过 manifest、却违反 SM-2 sealed closure 的
`candidate_repair_closure_mismatch` 是 `rejected_final`：provider 不能通过修改候选修复 owner authority。普通可选择的
target/binding 错误仍由前置 manifest gate 及合法 witness 决定 attempt 1。未知 code 必须以 `runtime_contract_defect`
封闭，保留有界 source code 及 typed path/manifest/shape diagnostic，不复制 raw provider reason；禁止默认重试或吞掉原始原因。

`CorrectionDirective` 是 Orchestrator persisted state，不是 evaluator/LLM/capability feedback。它只描述 candidate
delta，例如删除 forbidden field、补 provider wire 的 required proposal field、把错误内嵌的 declared parameter 重编码为独立
named typed entries、把同 owner/canonical target 的普通重复 durable operation 合并为一项、选择 allowed kind、为 exact
operation slot 选择 allowed evidence binding、替换为 allowed target或改变 rejected semantic shape；它不修改 authority，也不
包含 raw provider/decoder/owner 错误文本。empty/incomplete/invalid framing 没有 candidate，因此不能产生 directive。
`correctable_once` 还必须携带 `RuntimeCandidateCorrectionWitness`：schema witness 携带 manifest-bound field/rule；
`parameter_entry_encoding` witness 只携带 operation/parameter index、operation kind、container parameter name 与
misplaced parameter 的 name/contract type，并固定表达
`inline_declared_parameter_siblings -> independent_named_typed_parameter_entries`；semantic witness 才要求
rejected/alternative value、适用 operation kind 与不同 semantic-shape hash。parameter-entry witness 不得携带原始 content、
hash、coverage、summary value 或其它可逆 provider 数据。
`operation_set_cardinality` witness 必须携带 manifest-derived exact owner、canonical target、冲突 operation index/kind 集与
rejected candidate semantic-shape hash，并固定要求 `one_operation_per_owner_target`；attempt 1 必须消除该 typed violation，且
不得增加或删除原候选的 owner/target 覆盖，也不得修改非冲突 operation；冲突 owner/target 只能保留一项。该 typed violation
必须同时进入 sealed correction hash 与 provider-safe C4 projection，任一字段
漂移都不能复用 correction。只有 manifest 已封存的 exact required sequence 可合法保留同 owner/target 多项 operation。
`owner_candidate_constraint` witness 只携带 operation kind、exact parameter path、sealed constraint ref、closed
`actual_shape` 与 `required_shape`。attempt 1 provider call 前只验证同 manifest/path 且 constrained parameter 确实发生变化；
参数是否达到 required shape 仍只能由同一 capability owner 的后继 conformance read 证明，Orchestrator/LLM 不得复制 owner
parser。该 correction 不修改 C0～C3、stable prefix 或 ordinary feasibility contract。
`operation_value_rule` witness 只携带 actual candidate 已选中 operation/variant 中的 exact rule ref、末端 parameter path 和
closed `actual_shape -> required_shape`，不携带 raw value、owner diagnostic 或可逆 hash。attempt 1 只允许该 parameter 发生
execution-semantic 变化；SM-3 必须用相同 manifest 证明原值确实命中该 violation、修正后 selected operation/variant 未漂移且全部
`ValueRules` 通过。该 pure gate 在 capability owner I/O 前完成，不创建 owner fact、EventHub 事件或新的 correction budget。
strict decoder 报告 `missing_*`/`empty_*` required field 时，SM-3 必须先于 semantic frontier 封存同 path 的
`schema_field` witness（`field_absent -> field_present`）；required field 已存在但为 `null` 或错误 JSON type 时，则封存
`schema_field(<actual_type> -> field_present)`，其中 rejected value 必须等于 diagnostic 的 exact JSON type，且
`Required`/`Allowed` 必须证明该 path 的 expected schema type。两类 correction 的 objective 都只能是
`supply_required_field`；diagnostic 的 field/type vocabulary 不能解释为 target/binding alternative。attempt 1 对
`field_present` 的验收不是仅检查 key presence，而是让完整 document 重新通过同一 strict decoder 和 manifest admission，
也不能把 `field_present` 当作 kind、target、binding 或 parameter 的字面业务值。manifest semantic
validator 发现条件必填字段缺失时也必须使用相同机器表达，例如 `Actual=missing_bindingRefs`、
`Required=[bindingRefs]`；`Required` 不得承载“exactly one bindingRef”等人类说明。只有 provider 已提交一个具体但错误的
binding 且 manifest 存在另一个合法 member 时，才允许形成 `evidence_binding` semantic witness。
strict decoder 发现 `parameters[i]` 的 sibling key 同时满足以下条件时，必须一次聚合同一文档内的全部结构事实：key 是 exact
operation manifest 的 fixed declared parameter、raw JSON type 与 contract type 一致、该 parameter 没有另一个 named entry、
container 本身是合法的 named typed entry，且不存在 mixed undeclared/type-invalid/duplicate ambiguity。原文仍立即拒绝，不能在
本地搬移字段后接受；SM-3 只可封存 objective `reencode_parameter_entries` 和上述 witness。attempt 1 必须把 witness 中每个
logical parameter 重新生成为独立的 `{name, <typed-value-field>}` entry，并重新通过完整 strict decoder、manifest evaluator 与
普通 feasibility。任一参数只被删除、类型漂移、operation/index/manifest 漂移或 attempt 1 再次不合法都进入
`candidate_retry_exhausted`，不能再开 attempt 2。
其中 `evidence_binding` witness 必须绑定 exact `$.operations[i].bindingRefs[j]`、operation kind/slot、当前 rejected binding、
manifest 授权的一个不同 binding，以及 rejected/alternative 两个不同 semantic-shape hash；attempt 1 仍须用同一 manifest
重新严格验证完整 candidate。allowed binding 无不同 member、field path/slot 漂移或替换后仍不通过同一 manifest 时，frontier
为空并立即 final。
operation ID/dependencies 本就由本地 manifest 确定性派生且禁止出现在 wire，不能成为 correction field 或 progress witness；
wire order、display summary 或 reason 文本变化同样不构成 semantic witness。attempt 1 在 provider call 前重验相同
manifest/witness；返回结果必须重新执行完整 strict local admission，semantic alternative 还必须证明实际 proposal 使用
封存值。有限 allowed frontier 为空时，attempt 0 直接以 `candidate_authority_exhausted` 终止；attempt 1 再拒绝才是
`candidate_retry_exhausted`。Orchestrator 只把该 directive 的 provider-safe detached projection 放入 attempt 1 的 C4
retry tail；该 input 仅包含 manifest、objectives 与 witness，不含 persisted `Disposition`/`TerminalCode`。LLM Block 可以
据此渲染 exact alternative，但不能选择、修改或持久化 correction。failure 与 detached input 共同参与
correction/request-session identity，且不得改变 C0～C3 或 `stablePrefixHash`。duplicate/restart 复用 attempt 0 的 exact
directive 并重新投影相同 input，不重新推导或重新分配预算。
`candidate_authority_exhausted` 与 `candidate_retry_exhausted` 必须保持不同 terminal failure class：前者证明 attempt 0 后不存在
合法 correction authority，可能从未发起 correction provider call；后者才证明已消费唯一 attempt 1。projection 必须优先按
terminal code 保持该区别，并兼容旧记录中 authority code 曾落入 retry class 的数据，不能把零纠正调用展示为“重试无效”。
failed `CandidateAttempt` 的 durable invariant 必须在 CAS 前及 fresh load 时共同校验 failure/correction pair：manifest、normalized
objectives、field path、observed actual、parameter-entry encoding facts、适用 rejected shape 与 witness 必须一致；objective 还必须与 witness dimension 的 bounded
兼容矩阵一致，不能由 diagnostic 与 directive 携带同一错误映射而共同越过校验。结构合法但与 diagnostic 不一致的 directive
属于 `runtime_contract_defect`，不得进入 attempt-1 preflight/provider、dispatch 或 capability effect。

任一满足单一 JSON object framing 的 completed operations document，无论 local admission 最终 accepted 或 rejected，都必须在对应 attempt closure 的同一
owner-private CAS 中把 exact `CandidateManifest` body 写入 request session。operation-scoped wire diagnostic 必须能由该 persisted
body 重算 wire contract 并 exact 验证；缺失 body、hash/ref/kind/variant/index/path 任一漂移都是 `runtime_contract_defect`，不得
通过 mutable authority 重建、单例推断或兼容升级继续执行。

只有 complete-object schema 与 semantic rejection 共享同一 candidate attempt 0/1。empty/incomplete/invalid framing（包含
non-JSON、scalar/array、multiple roots 与 trailing bytes）发生在 attempt 前，只使用 request-session whole-document recovery；
它们不能创建任何 framing correction witness 或减少 attempt 0/1。SM-3 是唯一 correction/final owner；SM-4 只结算 physical call、调度并幂等
re-drive persisted result，不解释 framing/schema diagnostic，也不选择 attempt 1。

semantic coverage-order 的完整文档
结构错误必须保存 manifest 全量 CWI identity 与 field path，使用 typed `coverage_order_document` violation；proof-batch
保存 exact batch CWI identity 并使用 `proof_batch_document`。两者都只能由 SM-3 owner-private transition seal。

若任一 document/schema/semantic rejection 无法由 SM-3 根据 typed diagnostic 归类，属于 `runtime_contract_defect`，必须 fail-fast
且零额外 provider、ticket、dispatch 或 effect。`candidate_correction_undefined` 不得作为正常模型输出兜底。

### 正交候选与 owner 处理结果

provider dependency/governance 在进入 candidate pipeline 前单独处理。上述 semantic-plan prospective
`would_not_satisfy` 是 SM-2 persisted full-composite factual rejection；它不属于下表的普通 operation
`owner_feasibility`，仅能在 operation candidate pipeline 前按上述 owner-contract contradiction terminal 处理，不放宽
普通 operation owner feasibility 的零 correction 规则。进入候选处理后，所有结果只允许归入：

| 结果 | 来源 | 状态机行为 |
| --- | --- | --- |
| `request_recoverable` | incomplete/empty/invalid framing、output limit、timeout、429/5xx 或其它 recoverable transport | 仅接受 LLM owner 已确认 physical submission 的 fact；同一次 Orchestrator CAS 结算本次调用、推进 SM-3 session，并按 typed outcome 消费 whole-document 或适用的 SM-4 transport budget，在耗尽时应用 SM-2 failure；随后才可用新 PhysicalCallID 做同一 sealed expected-document 的 whole-document 有界恢复；不创建 attempt、不拼接 raw suffix |
| `request_terminal` | provider authentication/configuration/protocol/submission unknown 或 document recovery exhausted | 归一为唯一 owner-private fact，在同一次 Orchestrator aggregate CAS 中关闭 SM-3 session 并应用 exact SM-2 failure；CAS-before fresh reload 只从 submitted marker 封存 unknown，CAS-after/duplicate 复用 exact terminal；不创建 attempt、不重调 provider |
| `candidate_rejection` | 单一 complete JSON object 的 strict schema 或 CandidateEvaluator typed rejection | 先持久化 provider-safe original diagnostic；由 SM-3 owner-local reducer 在共享 attempt 0/1 内决定唯一 correction 或 typed terminal |
| `owner_candidate_constraint` | capability owner 对 manifest-declared parameter shape 的 static conformance，或对 current target/revision 的 dynamic `candidate_constraint_violated` factual observation | proposal 持久化后，以 Orchestrator private SM-3 event/CAS 持久化；static nonconformant 或 dynamic violation 与唯一 correction/final 原子关闭 attempt。static lane 零 workspace I/O；dynamic lane 只读且两者均零 effect、receipt 与独立预算 |
| `owner_feasibility` | capability owner 的 ordinary `stale`、`prerequisite_required`、`infeasible` 或 `owner_contract_defect` factual observation | 同一个 `CandidateFeasibilityObserved` owner-private event/CAS 同时保存事实并由 SM-2/SM-3/SM-4 协调归约：退休 prerequisite、取得新 authority、形成 typed terminal 或进入有合同的 recovery；不得先存半状态再发第二事件，且零 candidate correction attempt。它不与 provider request terminal 共用结果或 transition |
| `accepted` | evaluator 接受、全部 static constraints 有 conformant fact，dynamic applicability 被 current feasibility 接受，且 ordinary owner feasibility 为 feasible | persist sealed dispatch 后才可执行 |

ordinary owner infeasibility 及其 invalidated attempt 必须保留 owner 原始 diagnostic。若随后相同 authority input 被
`equivalent_authority_no_progress` guard 终止，SM-2 failure 同时保留 guard code 与该 exact infeasibility 的
`OriginalDiagnostic`，不得以泛化 guard 文本覆盖原因，也不得借此放宽 correction、repair 或 provider budget。投影只展示
两层事实；它不能据 owner code 发出 retry 或选择下一 action。

`requestBytes>0`、selected streaming mode、错误文本或 reservation 都不证明 physical submission。exact unsubmitted audit
必须在对应 session transition CAS 中 release reservations；缺失/篡改 audit 只能结算为 unknown 并终止当前 request，禁止
以 application retry、transport fallback 或 fresh reload 再次提交。

sealed execute adapter必须把完整operation payload、catalog/policy revision、exact owner subject、effect/replay class、
ExecutionKey/Fence和适用governance grant绑定到receiving-owner concrete command。owner在任何call/effect前重新验证producer、
current claimed receipt authority、current resource/schema/policy以及exact grant；独立mutable data copy、missing catalog、
stale revision或grant drift均保持零外部动作。

普通响应与 fresh-load receipt recovery 必须调用同一 outcome-first judgment。`Outcome=rejected` 先保留 owner 的 exact
original diagnostic，禁止再以 rejected receipt 的 `ObservedEffectScope=unknown` 覆盖成 Orchestrator effect mismatch；只有
`accepted | no_op` 才继续执行 sealed effect compatibility 与 evidence-quality gate。未知 outcome 直接 fail-closed。

claimed operation没有receipt时，只有sealed replay class为`safe_to_replay | owner_managed`或owner给出exact replay proof，
才可进入SM-4 recovery-budget handoff；`at_most_once | unknown`直接持久化`outcome_unknown`，零owner重放且不消费replay
budget。recovery不能因为operation被称为validation/read-only就放宽该规则。

correctable/final 只属于 SM-3 persisted transition result，不是 `DocumentOutcome`/`CandidateRejection` 字段，也不得接收 owner
code。以上分类是候选处理的唯一分流表。错误字符串、TUI/Web、projection、repair hint、owner adapter 或
测试 fixture 不得定义额外分流。
