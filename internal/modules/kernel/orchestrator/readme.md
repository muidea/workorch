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
- `runtime-transition-preimage/v1` 只用于本 owner 的单次 `Commit`：coordinator 私有调用身份绑定
  workspace/run/expected revision，adapter 每次经 Run typed query 真实重载、完整校验原 DTO；
  `RuntimeTransitionPreimageRequest.Compile` 在独立 raw graph 上完整校验 aggregate 后生成 private-field typed 前像。
  仅本次 coordinator 可一次消费，仍核验 scope/revision、全部五项迁移门禁、after-image 与 CAS。
  零值、跨调用、重复消费、未知版本、序列化和调用结束后均拒绝；不持久化、不跨调用缓存、不接受成功标志/hash。
  普通 Load/recovery/Save 和未实现此 owner-local complete-load 接线的 repository 仍完整校验；没有新的 owner、
  SM transition、预算或 provider/effect 权限。`transition_preimage_test.go` 固定完整原值、alias、生命周期和拒绝边界，
  `runtime_state_repository_test.go` 通过真实 Run owner 验证原始 DTO 门禁、逐次重载及 CAS/restart。
- 纯校验只可在同一次调用内复用本地规范化值：SM-2 binding hash 保持原字段、顺序和完整内容重算；SM-3 session
  可把本次完整 raw-canonical/source/hash 校验得到的 detached manifest 交给私有 wire compiler，不能仅凭调用者 hash
  推断成功。semantic assessment 前像重建可将同一 generation 完整校验所得的私有 manifest 产物复用于逐批 binding compiler，
  每批 document、request identity 与 owner feasibility 仍完整反证；公开 compiler 每次重新校验，
  `runtime_semantic_binding_validation_reuse_test.go` 固定产物一致性、逐批拒绝和输入不变边界。
  binding compiler 只读本次完整校验建立的 coverage/obligation 索引，仅为当前批次构造 detached source；私有 focus compiler 消费同一 validated manifest，不再次规范化整个 generation。document、proof obligation、owner feasibility 与 reconciliation 的完整校验保留，公开入口和后续调用仍重新验证；`runtime_semantic_binding_indexed_reference_test.go` 保留旧 compiler 路径，固定完整产物、拒绝原因、返回隔离与重载等价。
  私有 focus compiler 在一次调用内仅完整规范化 source 一次，复用于 proof/selector 与最终 focus 投影；
  返回的 focus/binding 仍逐项分离，document、owner facts、binding 与 reconciliation 校验和首个错误顺序不变。
  `runtime_semantic_focus_source_reuse_test.go` 复用独立旧 compiler 对照 definition/repair/多批/独立 proof 的完整产物、原值、fuzz 及分配；
  既有 raw/owner 反例与重载合同继续适用，不新增跨调用缓存或改变 SM-1～SM-4 和预算。
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
  raw-canonical candidate manifest 门禁在完整正文比较后复用已规范化产物计算 hash，公开 hash 入口仍完整规范化；
  input-protection、repair、value-rule、target binding、coverage projection 和 working commitment 门禁及首个错误顺序不变。
  `runtime_candidate_manifest_hash_reuse_test.go` 对照原规范化后再调用公开 hash 的路径，固定嵌套 recipe/variant、
  重复/空集/原 bytes、hash 篡改、reload、detached 返回与实际 fuzz；仅减少重复复制，不跨调用缓存或改变 SM/预算。
  candidate manifest hash 的一次编码使用既有短字段缓冲 writer，仍保留原 SHA256、big-endian 长度、cardinality 和完整正文；
  原无缓冲 writer 对照覆盖完整 manifest、120/121-byte 边界、长字段、fuzz 与分配，不改变 hash/wire identity。
  attempt 与 dispatch 两侧仍独立规范化、比较完整 binding 正文与 snapshot hash，不以自洽 hash 代替来源反证。
  proof/binding Clone 复用自身 Normalized 的 detached 产物，保持既有 nil/empty 语义；
  `runtime_coverage_proof_clone_reuse_test.go` 对照旧实现验证原值、嵌套隔离、重载与分配成本。
  effect scope 和 operation variant 集合的 canonical 快路径仍返回 detached 副本，保持旧排序、去重及 nil/empty 表示；
  `runtime_proof_canonical_collections_test.go` 用独立 reference 与 fuzz 校验非规范输入和返回隔离。
  CWI ID 集合的 canonical 快路径仅复制原 typed 集合，非规范输入仍走原 string 集合转换与去重排序；
  `runtime_coverage_ids_canonical_test.go` 对照独立原算法固定 Unicode/任意 bytes、空集、重复、原值、重载、隔离、fuzz 与分配，
  不改变原规范化/hash identity、raw 门禁或 SM-1～SM-4 转换和预算。
  hash 字符串编码合并同次长度前缀与正文写入，不改变 big-endian 长度、集合 cardinality 或任意原始 bytes；
  `runtime_hash_string_encoding_test.go` 固定无效 UTF-8、NUL、长原文与重复 hash，并以 fuzz 对照旧编码。
  原始 source-obligation check ref 与 validation-proof requirement hash 使用同一次编码私有缓冲，保留原规范化、
  SHA256、字段次序、长度前缀、集合 cardinality 与完整原文；不缓存任何来源或成功凭据。
  `runtime_obligation_buffered_hash_test.go` 对照旧无缓冲编码覆盖短字段阈值、长字段、任意 bytes、重载、fuzz 与分配，
  不改变 SM-1～SM-4、验收、预算、CAS 或 availability。
  canonical authority/catalog 与 coverage binding 的 hash writer 使用单次编码私有短字段缓冲，长字段仍完整写入；
  不跨调用共享、缓存输入或改动任何 SM-1～SM-4 transition。`runtime_hash_writer_test.go` 对照旧完整编码，
  固定边界、长短交替、Write/Sum/Reset、并行独立实例、fuzz 与分配成本，所有 raw/hash 门禁保持原样。
  超过 8 KiB 的长字段使用同次私有 4 KiB scratch 分块编码完整原 bytes，不按字段分配整个输出；
  原 big-endian 长度、字段次序、cardinality、短／中字段路径与 SHA256 保持不变，scratch 不跨 hash 调用共享。
  `runtime_hash_long_field_test.go` 保留旧完整分配 writer，以阈值／块边界、非法 UTF-8、NUL、长短交替、
  Write/Sum/Reset、fuzz 与分配对照固定身份；这不是截断、诊断改写或校验成功缓存，不改变四机或预算。
  executable operation contract revision 只标识本 binary 的编译期静态库存，在 descriptor init 完成后按原完整编码计算一次；
  persisted revision 每次仍独立比较并返回 exact mismatch，不缓存 Run、manifest、raw/hash 或 aggregate 校验成功。
  `runtime_operation_contract_revision_reference_test.go` 保全原完整编码；
  `runtime_operation_contract_revision_static_test.go` 固定并发身份、公开库存副本隔离、后续 persisted 漂移拒绝与分配对照。
  owner variant／convergence 的纯存在性查询从公开编译期 operation 合同派生本 binary 私有 variant ID 表，
  不再为 known／has-variants／contains 查询构造完整参数、value rules 与约束；公开合同仍每次返回独立值，
  owner、target policy、selector、constraint 和 acquisition 门禁仍逐项完整执行。
  `runtime_operation_variant_identity_test.go` 保留两个旧完整 validator，以全部 descriptor/variant、非规范 kind、
  任意 bytes、公开副本修改、并发、后续漂移、fuzz 与分配固定等价；不缓存运行数据或校验成功，SM-1～SM-4 转换不变。
  assessment purpose 前像在一次 ledger 遍历内使用同一 private semantic validation session；历史 generation 仍完整校验，
  manifest/catalog 仅在 exact raw 相等时复用本次产物，batch、document、owner feasibility 与唯一 binding 前像仍独立反证。
  `runtime_assessment_preimage_session_reference_test.go` 保留原逐代独立校验路径；
  `runtime_assessment_preimage_session_test.go` 固定完整前像、原 bytes、同 hash raw 篡改、后续调用、重载、隔离、fuzz 与分配。
  同次前像 document 索引保留既有完整 Normalized 表示，不在索引时再 Clone 未变 proposals；
  私有 binding compiler 已规范化的 focus 只读消费，返回前像仍分别复制 binding、obligation、document 和 owner catalogs。
  `runtime_assessment_preimage_copy_reuse_test.go` 与原逐代路径对照非规范输入、完整产物/首个错误、raw/重载、隔离、fuzz 和分配；
  active ledger 的原 Clone 语义、全部 manifest/batch/document/owner 门禁及 SM-1～SM-4/预算保持不变。
  assessment focus 的义务查找先按 exact CWI 比较，再仅复制匹配条目；仍扫描完整集合并拒绝重复/缺失义务，
  不省略 enclosing manifest raw/hash 校验。`runtime_assessment_matched_obligation_test.go` 保留逐条先复制的原算法，
  对照完整产物、首个错误、无关原始正文和 detached nested 返回值，并验证减少无关复制；SM-1～SM-4 与预算不变。
  同次完整 manifest 校验产物可保存已成功反证的 exact raw semantic document；首次成功时才复制 manifest/document 原值，
  再次使用前分别比较完整 manifest 与原始 document，不按 hash 或规范化后正文命中。失败不缓存，公开入口及后续调用
  仍重新完整校验；batch、owner feasibility、input review、binding 和 reconciliation 门禁不省略。
  `runtime_semantic_document_session_test.go` 对照禁用 session 的原完整 document 路径，固定首个错误、原值/重载/后续 mutation、
  detached 前像、compiler 完整产物与 owner-fact 反例、fuzz 和包含首次校验成本的分配对照；不改变 SM-1～SM-4 转换或预算。
  semantic proof compiler 的 raw owner catalog 集合门禁保持完整，仅复用本次 detached 目录索引解析 prospective all-of 约束，
  不重复校验未变的整份目录；公开 resolver 仍完整校验，每次 compiler 调用重新反证 raw、重复 owner/surface、hash 和 exact requirement。
  `runtime_semantic_proof_catalog_reuse_test.go` 固定公开／私有 resolver 等价、完整产物、原值、重载及后续调用拒绝。
  proof catalog 索引直接复用本次逐条完整校验返回的 detached 规范化值；prerequisite 重复仍逐条检查，owner 重复仍在
  全部条目校验后检查，不提前返回而改变首个错误。排序、空集合、嵌套 proof/snapshot 和公开 compiler 门禁保持不变。
  `runtime_semantic_proof_catalog_values_test.go` 对照原集合门禁后独立规范化路径，核验完整产物、混合反例错误顺序、
  raw/重载/输入保全、返回隔离、后续变化、fuzz 与完整校验分配；不改变 SM-1～SM-4 转换、预算或 availability。
  semantic owner catalog 校验直接复用同次完整 raw／嵌套 proof／snapshot 校验产生的 detached 规范化目录，
  不再次规范化未变的整份目录；重复 prerequisite／owner 门禁保留，公开 `Validate` 与后续调用仍完整重验。
  `runtime_owner_catalog_validation_reuse_test.go` 与独立旧 validator 固定产物、完整拒绝原因、原值、重载、隔离、fuzz 和分配成本。
  semantic work 与 owner catalogs 在同一次 semantic session 内只对 exact raw 相等的成功前像复用完整校验产物；
  原始前像复制隔离，不以 snapshot/hash 相等推断成功。work 的原 prerequisite 重复顺序、canonical 前像和排除集合仍完整反证；
  公开入口及后续调用重新验证，`runtime_semantic_work_validation_reuse_test.go` 保留独立旧门禁和 raw alias 反例。
  work/purpose compiler 与 aggregate 的逐历史用途重编共用同次私有 semantic session；完整 owner catalog 的逐项与
  prerequisite 门禁仍先于集合-wide owner duplicate，不借用另一 validator 的首错顺序。
  scope compiler 仅在完整 raw seed/focus/Profile 或完整 scope source 精确相等时复用本次成功来源门禁；
  manifest、request、原 CWI/policy、逐轮 receipt 与历史 source join 仍分别反证，公开入口及后续调用重新完整校验。
  `runtime_work_compiler_reference_test.go`、`runtime_work_scope_compiler_reference_test.go` 与原 state/ledger oracle
  固定完整产物、首个错误、原 bytes、source/return 隔离、重载、同 hash raw mutation、fuzz 和包含首次校验的成本。
  此私有只读复用不改变 SM-2/SM-3 acceptance、SM-1/SM-4 转换、预算、CAS、恢复权威或 availability。
  工作 round／binding／semantic-purpose／receipt-set 的 JSON hash 保留原 `Marshal` 表示和 SHA256，
  仅让本次编码缓冲直接进入私有 digest，不再复制完整 JSON 输出；只扣除 `Encoder` 添加的最后一个换行，
  任意分块及正文内原换行均完整保留。失败仍返回原空 hash，不缓存对象或成功凭据。
  `runtime_json_value_hash_test.go` 对照旧完整编码，固定原文字节、nil/empty、错误、长字段、分块写入、
  原公开 work 归档／重载、fuzz、隔离与分配；不改变四机、预算、wire 身份或 availability。
  semantic ledger 的 current／历史 generation 对完整 exact raw Definition seed 使用同次成功前像；
  第一次仍执行原全 gate，成功前像独立 raw clone，hash 相同但正文变化、nil/empty 或嵌套来源漂移不能命中。
  generation、manifest、batch、document、owner feasibility 及完整 ledger 门禁仍按原次序执行，公开入口重新反证；
  `runtime_semantic_seed_session_reference_test.go` 保留原完整 ledger/generation 路径，直接 seed oracle、公开归档
  对照、重载／后续变化、fuzz、隔离与包含第一次校验的 benchmark 固定原语义。此复用不改变四机、预算或 availability。
  同次完整 manifest 已验 work 时，proof-set／feasibility consumer 仅复用该 work 门禁产物；每项 proposal、owner、target、
  evidence/effect、原 declaration 和 exclusion 仍逐项核验。未验证的入口仍执行完整 work validator；原门禁 oracle、重载、
  后续变更和 fuzz 反证该复用不改 acceptance、预算或 SM 转换。
  Goal execution seed 在同次完整映射门禁后逐 CWI 编译原文与规则，不为每项重验未变的整份映射；公开 compiler 与
  后续调用仍完整反证，空 inventory／空项的旧行为和首个错误顺序不变。
  `runtime_goal_execution_seed_reuse_test.go` 对照原逐项公开 compiler，固定原值、重载、后续 source/rule 漂移、
  detached 结果、混合错误、fuzz 与完整校验分配成本；不提供 caller 已校验标志或跨调用缓存。
  source evidence policy 的纯编译先按原 trim 后的 exact/wildcard predicate 投影 Profile ID 与全部匹配声明，再执行
  原规范化、稳定排序和完整 merge 门禁；不复制随后丢弃的无关 Profile 图，不减少完整 Profile admission。
  `runtime_source_policy_projection_test.go` 保留旧全 Profile 编译 oracle，固定完整产物、duplicate／wildcard／additional、
  raw bytes、首个错误、重载、后续漂移、fuzz 与分配；不新增 authority、成功缓存或 SM-1～SM-4 转换。
  Run-owner record 与 aggregate 的纯转换仅复用已经分离的字段图，其余原 Definition/Facts/Decision、attempt、
  governance 和诊断以原 raw Clone 独立复制；语义 ledger 的比较规范化只在独立输入副本上运行。
  dispatch candidate 采用 raw detached Clone，目录 MCP 嵌套集合在规范化投影中同样隔离；不洗掉非法原文。
  `runtime_state_conversion_reference_test.go` 保留原四个转换路径，
  `runtime_state_conversion_reuse_test.go` 固定完整逐字段产物、nil/empty、原 bytes、全图 storage 隔离、
  首个 aggregate 错误、重载、fuzz 与分配；Load 的完整 DTO/aggregate 和 Save/CAS 门禁不减少，四机和预算不变。
  catalog 内逐项完整校验后产生的 detached surface 仅按原 ID 稳定排序再复用，不在整 catalog 规范化时重复深拷贝；
  raw input-review、重复 surface、owner/revision、recipe 与 snapshot 门禁及其拒绝顺序不变。
  `runtime_owner_catalog_surface_reuse_test.go` 固定 ready/preparation 多 surface 排序、嵌套隔离、V2 原读取前像、
  后续变更重新拒绝、重载、nested fuzz 与分配对照；不新增成功缓存或改变 SM-1～SM-4 转换。
  validation surface 在同次完整 raw proof-binding/preparation 校验后，仅继续反证跨 surface recipe、native-check、
  readiness/applicability 与 constraint 身份，不再次调用未变 proof 的完整 validator；公开 surface 入口及后续调用仍重验。
  `runtime_validation_surface_reference_test.go` 保留修改前完整旧校验；
  `runtime_validation_surface_single_pass_test.go` 固定接受／拒绝顺序、原 bytes、V2 input-review、重复／交叉身份、
  JSON 重载、fuzz 与逐分配对照。此复用不改变任何 SM transition、预算或 current applicability。
  surface 在逐项完整 proof 校验之后的 duplicate 查询只投影 trim 后的 requirement hash／CWI identity，
  不为这两个字段再次深拷贝整份 binding／preparation 与原诊断；后续 surface 正文规范化和全部身份门禁保持不变。
  `runtime_validation_surface_identity_projection_test.go` 对照旧完整 validator 固定 multi-preparation、raw trim、
  duplicate、完整 proof 失败优先顺序、owner 漂移、JSON 重载、后续篡改与分配；nested fuzz 继续走独立旧路径。
  此纯读取不提供已校验标志、缓存或新 SM-1～SM-4 转换。
  proof binding／preparation 集合自身仍按原次序逐项完整 Validate，再只读 trim 后的 requirement hash／CWI 做 duplicate 查询；
  不为身份集合再次规范化整份 proof、input review 或原诊断。`runtime_validation_proof_collection_identity_test.go`
  保留两个旧完整集合 validator，并供旧 surface oracle 使用，固定 nil/empty、raw trim、完整正文失败与 duplicate 的先后、
  后续篡改、JSON 重载、任意 bytes、fuzz 和分配对照；不改变任何机器转换、proof 门禁或持久化权威。
  preparation 的纯 Validate 只读原 `OriginalDiagnostic`，由诊断自身完整 Validate 构造比较副本，
  不先 raw-clone 随后丢弃；公开 preparation `Normalized`／`Clone` 仍返回 detached raw 诊断，
  owner／catalog revision 比较仍使用原诊断字段，不 trim 放宽。`runtime_validation_preparation_diagnostic_read_test.go`
  保留独立旧完整 validator 并接入集合／surface oracle，固定原值、公开 Clone 隔离、原安全／identity 拒绝顺序、
  后续变化、任意 bytes、fuzz 与分配；此纯只读路径不改变 SM-1～SM-4 转换、原诊断保存或 proof authority。
  assessment authority hash 保持原 Normalized、JSON 和域前缀编码，复用同次 marshal 缓冲，不增加跨调用缓存；
  `runtime_assessment_authority_hash_test.go` 以旧编码 reference、JSON 重载、fuzz 和分配对照固定等价边界。
  lineage rule hash 同样保持原完整 JSON／域前缀编码，只在 marshal 缓冲有余量时复用；容量不足仍走原编码路径，
  不缓存规则或校验结果。`runtime_lineage_rule_hash_encoding_test.go` 固定逐字段变动、非法原 bytes、重载、fuzz 与分配对照。
  原诊断的比较副本只对 `FieldViolations` 执行一次原有规范化，不先 raw-clone 再重复复制随后丢弃的有序集合；
  `CloneOriginal`、原诊断保存与 exact feedback hash 仍保全原 bytes、顺序、multiplicity 及 nil/empty，完整安全与身份门禁不变。
  `runtime_feedback_field_normalization_test.go` 对照旧完整规范化路径固定长原文、非法 bytes、所有嵌套副本隔离、
  原 hash、JSON 重载、后续变化、fuzz 和分配；这不是诊断脱敏或成功缓存，不改变 SM-1～SM-4 转换或权威。
  semantic planning transaction 完成 after-image 后直接交唯一 coordinator 执行完整 aggregate、独立前像及 revision 门禁，
  不在紧邻 Commit 前重复同一未变 aggregate 校验；原 ledger 门禁仍保留。
  `biz/runtime/semantic_planning_transaction_gate_test.go` 固定非法 ledger／after-image／前像、stale、保存失败和无变化事务。
  current/history validation catalog 的完整合取在同一次纯调用中仅复用已成功验证且 detached raw 完全相等的
  Definition requirements；所有 raw catalog、exact participation、原错误次序和标签仍完整校验。
  `runtime_validation_catalog_history_test.go` 对照独立完整门禁固定 raw/hash 漂移、首错、alias、重载、后续调用、
  只读归档、fuzz 与分配；不导出成功标志或前像，不改变 SM-1～SM-4 转换、Load、CAS、预算或 availability。
  semantic-rebinding projection 门禁先按原 reopen state／最后 lineage reason 判断是否适用，无需重绑时不复制无关 focus/binding；
  需要重绑时仍完整走原 Definition seed 投影比较，其它 aggregate 门禁不变。
  `biz/runtime/semantic_rebinding_projection_applicability_test.go` 以原完整路径固定接受／拒绝原因、原值及分配等价。
  预算 dimension 的纯查询按原顺序读取逐项规范化的标量，保留完整 policy 稳定排序后的首个同 dimension 匹配，
  不为返回一个值复制／排序整份 policy，也不 trim 调用者的查询 identity。allocation 校验的 policy revision
  比较只规范化该标量，不构造无关的完整 policy 副本。公开 policy Clone、完整 Validate／hash、
  allocation／consumption／unknown／exhaustion 门禁不变；`runtime_budget_dimension_projection_test.go` 以旧完整投影及
  allocation validator 固定 duplicate/raw/非法值、完整错误顺序、原输入、JSON 重载、后续漂移、fuzz 和分配，
  此查询不是预算批准或成功缓存。
  allocation／consumption／reconciliation 的 ID/hash 与 budget view hash 复用每次编码私有的短字段缓冲，
  长字段仍完整编码原 bytes；规范化、原长度前缀、字段次序、十进制／布尔表示、view 的原 dimension 顺序均不变。
  `runtime_budget_buffered_hash_test.go` 以独立旧非缓冲编码固定逐字段、边界整数、非法 bytes、长正文、nil/empty、
  原输入、完整 ledger 重载与后续篡改、fuzz 和分配；不保存 hash/校验成功缓存，也不改预算上限或 unknown 责任。
  opaque `RuntimeStateRecord.Validate` 的 Definition/facts revision 比较只 trim 两个标量，不为身份读取
  规范化无关的完整 Definition、catalog、proof 或诊断。前面的完整 raw facts、semantic、Definition 等门禁和错误顺序不变；
  `runtime_state_revision_projection_test.go` 对照旧完整规范化，固定原 bytes、重载、fuzz 和零分配投影，
  `biz/runtime/state_revision_projection_test.go` 从合法初始化记录固定 trim、mismatch、前置正文拒绝、重载与后续漂移。
  此纯读取不提供成功凭据、不改变 SM-1～SM-4 转换、CAS、持久化或恢复权威。
  target-parameter declaration 校验只读从同一编译期 operation 合同派生的私有参数／variant 冲突规则，
  不为规则查询构造完整 value rules、描述或 candidate constraints。公开 operation 合同仍返回独立值；原 owner/kind、
  singleton target、optional string、variant conflict/discriminator、inventory bound、exact preimage 与拒绝次序完整保留。
  `runtime_target_parameter_static_contract_test.go` 保留旧完整 validator 并接入旧 catalog oracle，固定全库存、
  raw 表示、反例、原值、JSON 重载、公开副本污染、并发、fuzz 和分配；不缓存 owner facts/preimage/校验成功，
  此静态查询不改变 SM-1～SM-4 转换、sealed target authority、持久化、预算或恢复规则。
  independent-binding 谓词只投影原规范化后声明的存在性，不复制无关 focus binding、proof 或诊断；
  空白 metric/artifact 与缺失 owner/fact/operation identity 的条目仍按原规范化丢弃，非空非法声明不在谓词中静默过滤。
  完整 focus declaration 的 verification、owner requirement、convergence、单 CWI 与 wildcard 门禁及拒绝顺序保留。
  `runtime_independent_binding_predicate_test.go` 对照旧完整规范化谓词与声明 validator，固定原输入、nil/empty、
  非法声明、公开副本隔离、JSON 重载、后续变化、fuzz 和零分配；此纯查询不授 credit、不缓存校验成功，
  不改变 SM-1～SM-4 转换、持久化、预算或恢复权威。
  binding 的 `EvidenceKinds` 仅从原 proof 投影 kind 集合，保留 trim、空白丢弃、排序去重及 detached 返回，
  不复制无关的完整 proof、target、lineage 或 policy；此查询不校验或授予 proof，完整 binding/raw/hash 门禁保留。
  `runtime_binding_evidence_kinds_projection_test.go` 对照旧完整规范化路径，固定 nil/empty、非法 bytes、重复声明、
  原输入、返回隔离、JSON 重载、后续变化、完整拒绝、fuzz 与分配；SM-1～SM-4 转换和恢复权威不变。
  focus evidence declaration 门禁只规范化实际消费的 focus ID、CWI 集合、declaration 与 owner/convergence 声明，
  verification policy 仍由自身完整 Validate 规范化校验；不复制该门禁未读取的 binding、proof、target 或诊断。
  `runtime_focus_declaration_projection_test.go` 与原完整 validator 固定接受／拒绝、字段空白、重复 CWI、原输入、
  JSON 重载、后续变化、完整 raw proof/hash 拒绝优先、fuzz 与分配；不删除任何声明门禁或改变 SM-1～SM-4 权威。
  binding-focus 校验复用同次 `focus.Normalized` 已建立的 detached binding 集合，不再次规范化未变集合；
  原 raw binding/proof/hash、重复 ID 及后续逐 binding 完整 Validate、focus/revision/policy/kind/target/CWI 门禁全部保留。
  `runtime_binding_focus_normalization_reuse_test.go` 保留独立旧完整 validator 并接入旧 semantic compiler oracle，
  固定接受／拒绝顺序、原输入、重载、后续篡改、fuzz 与分配；不提供已校验凭据或跨调用缓存，不改变四机权威。
  proof 集合仍逐项 Clone／规范化、丢弃空 kind 并返回 detached 完整正文，只以按索引的 typed stable sort 替代反射排序；
  二项集合仅在后项 kind 严格小于前项时交换，相同 kind 的原相对顺序和重复项全部保留，完整 raw proof/duplicate/hash 门禁不变。
  `runtime_proof_typed_stable_sort_test.go` 对照旧 `sort.SliceStable` 路径固定任意 kind、长集合、nil/empty、
  嵌套隔离、重载、后续变化、重复原权威拒绝、fuzz 与分配；不改变 SM-1～SM-4 转换或持久化表示。
  native check attestation 同样只替换反射稳定排序，原逐项规范化、空 RequiredCheckRef 丢弃、三字段排序键及同键原顺序保留；
  supplemental check 与 execution stages hash 不参与排序、不被改写，返回集合仍 detached，完整重复／协议门禁不变。
  `runtime_attestation_typed_stable_sort_test.go` 对照旧完整排序固定多键、重复、原值、Clone 隔离、重载、后续变化、
  fuzz 与分配；不改 SM-1～SM-4 转换、native proof 或持久化权威。
  aggregate 完整门禁内只生成一次 detached active/completed dispatch 原值投影，并交各私有阶段只读复用；
  SM-2、SM-3、operation recovery、receipt/governance 与 cross-machine 校验及原拒绝顺序仍全部执行。
  accepted-candidate 精确匹配复用同一投影，不从 dispatch 回填独立 admission 前像；公开后续 Validate 与 Coordinator
  after-image／独立前像／revision fence 仍重验，不保存成功凭据或跨调用缓存。
  `biz/runtime/aggregate_dispatch_projection_reference_test.go` 保留原完整调用次序及各阶段重复投影；
  `biz/runtime/aggregate_dispatch_projection_reuse_test.go` 固定完整拒绝、原值、投影隔离、重载、后续篡改、CAS、fuzz 与分配，
  不改变 SM-1～SM-4 转换、持久化、预算或恢复权威。
  accepted-attempt/dispatch 的纯 matcher 先排除原完整 matcher 最终也会返回 false 的非同一精确身份；
  身份相同不授信，target、binding、recipe、work/repair commitment 与最终 dispatch authority hash 门禁仍完整逐次执行。
  `runtime_admission_identity_prefilter_test.go` 保留原完整 matcher，固定有效配对、raw/duplicate/hash 反例、
  原输入、JSON 重载、后续变化、fuzz 与无关配对零分配；不改变 aggregate 门禁、四机转换或公开 availability。
  missing-owner-fact 查询复用同次完整规范化已生成的 detached catalog，不按每条 requirement 再复制整份 catalog；
  shared surface observation、ready/direct/published/exact requirement 语义和 missing 集合顺序保留，查询不替代完整 proof 校验。
  `runtime_missing_owner_fact_catalog_reuse_test.go` 以旧完整查询及独立 classifier 固定原值、非法声明、重复、nil/empty、
  重载、后续变化、完整 owner 门禁、fuzz 与分配；不改变 SM-1～SM-4 转换、credit、预算、持久化或恢复权威。
  这些纯校验成本收口不改变 SM-1～SM-4 转换、预算、保存或恢复权威，也不证明累计整包已通过。
  current focus 的 target-selection 纯投影只规范化实际返回的 selection，并按原 trim／首个同 ID prerequisite
  语义比较 Kind／Scope／FocusID，不复制无关 Definition、证据、catalog 或 coverage。原完整 aggregate/raw/hash
  校验不变，不增加已校验标志或跨调用缓存；`biz/internal/grounding/target_projection_reference_test.go`
  以旧完整 Facts.Normalized 路径固定 duplicate／stale／raw 表示、原值、返回隔离、重载、fuzz 与分配对照。
  此纯值投影参与既有 SM-2 authority 与 SM-3 admission，SM-1／SM-4 的转换和恢复规则不变。
  semantic manifest／proposal／document hash 复用同次 `runtimeHashWriter` 短字段缓冲，仍保持原完整长度前缀、
  字段顺序、集合 cardinality、长原文和 nested input-review/target/obligation 编码；规范化与完整校验不变。
  `runtime_semantic_buffered_hash_test.go` 使用原非缓冲 hash 路径固定原 bytes、有效 sealed ledger 重载、
  fuzz 和长短字段分配；不缓存 proposal 身份或 validation 成功，不改任何 SM 转换、预算或恢复规则。
  semantic document 的 proposal 集合仍逐项规范化并按 CWI／proposal hash 稳定排序；只在本次排序首次比较同 CWI 时
  编码该成员的 hash，随成员交换保留本地 key，不再次规范化整个 proposal 或在 comparator 内重复编码。
  不同 CWI 的比较不产生无用 hash，原空集合、duplicate 与同 hash 的 raw review 先后保持不变；公开 hash 仍重新规范化。
  `runtime_semantic_proposal_sort_test.go` 对照原稳定排序与独立完整编码，固定原 bytes、nil/empty、嵌套隔离、
  sealed ledger 重载、后续输入变化、fuzz 和分配；此纯值复用只参与既有 SM-2／SM-3 合同读取，
  不新增 SM-1～SM-4 转换、保存凭据、authority cache、预算或恢复规则。
  semantic proof obligation 的 Clone 复用自身 `Normalized` 已分离的集合与嵌套合同，不再次复制原 owner convergence、
  validation requirement、repair 或 verification policy；acquisition 的嵌套路径规范化对非法输入并非幂等，
  仍保留原 Clone 次序和拒绝表示，不以“正常路径应幂等”为由改变原始输入的处理。
  原 nil/empty 与源义务原判据表示保持不变；
  `runtime_semantic_obligation_clone_test.go` 以旧完整 Clone 固定所有嵌套副本隔离、完整拒绝原因、有效 manifest／
  sealed ledger 重载、原 hash、后续调用、fuzz 与分配；完整 SM-2／SM-3 admission、raw/hash、aggregate 门禁不变，
  不增加任何 SM 转换、持久化成功凭据或跨调用缓存。
  execution-budget policy 在一次完整 `Validate` 中复用已规范化的 detached policy 编码 hash，使用同次短字段缓冲，
  不重复深拷贝／排序；公开 hash 入口仍先规范化，所有维度、数值、duplicate、scope、restart 和 hash 门禁及拒绝顺序不变。
  `runtime_execution_budget_policy_single_pass_test.go` 以完整旧 validator/hash reference 固定原 bytes、长字段、非法输入、
  JSON 重载、后续篡改重验、公开副本隔离与 fuzz；不缓存校验成功、不改变 SM-1～SM-4 转换、预算上限或扣费规则。
  静态 operation descriptor 的内部证据资格查询只读初始化库存，不复制随后丢弃的 `AllowedStates`；公开 getter 仍深拷贝。
  `runtime_coverage_evidence_descriptor_lookup_test.go` 以旧 getter 路径为 reference 覆盖全部 kind、未知与非规范输入、重复调用和公开副本隔离；不减少任何 proof、raw 或 aggregate 校验。
  validation input targets 的 owner 存在性校验只读同一静态 descriptor inventory，不复制随后丢弃的完整库存；
  owner 原值比较、512 项上界、2048 bytes／控制字符／duplicate 门禁及拒绝顺序不变，公开库存 getter 仍返回深拷贝。
  `runtime_validation_input_inventory_test.go` 对照旧完整库存路径固定所有 owner、边界、非法原 bytes、重载、后续篡改、
  公开副本隔离、fuzz 与分配；不缓存 surface 校验成功、不改变 SM-1～SM-4 权威、转换、预算或恢复规则。
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
  CWI focus-binding 校验只规范化实际消费的 Definition revision 与 focus queue，不复制无关 resources/evidence/recovery；
  保留原队列过滤、顺序、逐 focus 规范化及完整 binding validator，每次调用重新反证。
  `runtime_cwi_focus_binding_projection_test.go` 对照修改前完整 Facts 路径，固定接受／拒绝顺序、nil/empty/重复队列、
  原输入、JSON 重载、后续篡改、任意 bytes、fuzz 与分配；不缓存成功凭据或减免完整 aggregate gate。
  semantic target policy 的三项封闭集合使用同次位集合，保留原 trim、未知值过滤、去重、字典序、nil 与 detached 返回语义；
  `runtime_semantic_target_policy_closed_set_test.go` 以旧 map／排序路径覆盖全部子集、重复／非法 bytes、原输入、
  JSON 重载、返回副本、后续调用、fuzz 与分配成本。不改变完整 policy 校验、公开 availability 或四机转换。
  proof-obligation 的私有校验在本次完整 manifest 校验后复用 detached 规范化正文，不再次复制未变 owner catalog；
  document、每项原义务、source、declaration、recipe/surface、owner-fact acquisition 与 repair 门禁按原顺序全部执行。
  `runtime_semantic_proof_obligation_validation_reuse_test.go` 保留旧完整校验路径，固定多批、独立 proof、repair/acquisition、
  反例与拒绝顺序、原值、重载、后续变化、fuzz 和分配；原入口仍规范化完整 manifest，不授调用者已验证标志。
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

### Owner 验证执行合取事实

`RuntimeValidationCheckAttestationBinding.ExecutionStagesHash` 可由 validation owner 显式封存有序的
`ID + Scope + RequireTests + RequireBehavior` all-of 合同；空值保持既有 native mapping 语义和旧 hash。
非空声明的准确 native PASS 还必须匹配同 protocol、同 stage hash 的完整
`RuntimeValidationObservation.ExecutionConjunction`。Core 只验证中立身份、完整成员、执行完成、测试计数、
输出完整性、source 前后／跨阶段一致性和实际行为要求，不识别 Shell、Go 或软件开发准则。
同一 requirement 不得混用带合取和无合取的 mappings，阶段顺序、测试／行为要求不得降级。

receipt、EvidenceFact、catalog ready/preparation、coverage binding、proof lineage、reconciliation 与输入评审
身份均绑定完整相应正文；Clone/Normalized/raw clone 保留原事实并隔离嵌套 argv/stages/targets，
非法 raw 不能通过规范化洗成合法。准确 duplicate 返回原 persisted receipt；fact 自洽 hash 仍须反查原 receipt，
不能删除或替换 stage argv、source 或 output identity。`runtime_validation_execution_conjunction*_test.go`
固定合同反例、旧空扩展、hash、重载和隔离；`biz/runtime/validation_execution_conjunction_receipt_test.go`
验证原 receipt→fact 来源、重载、duplicate、自洽 fact 篡改拒绝，以及 CAS 前失败／CAS 后响应丢失
沿原 aggregate 恢复且不重复提交的边界。

该扩展只参与原 SM-2 证明匹配／SM-3 receipt 事务及 SM-4 原 aggregate 重载，不新增转换或账本；
SM-1 不变。typed 接收端测试不等于实际 owner producer 或公开 fullflow，当前不因此开放
implementation-work availability，也不将输出身份当作原始诊断已持久保存／可读取的证明。

`RuntimeValidationOutputIdentity.Storage` 可保全 validation owner 的有界原文引用或明确未保存原因，
不包含原 bytes、路径或读取授权；接收完整性与持久保存失败分开记录。声明 storage 后的完整合取必须逐阶段
具有 stdout/stderr 引用，缺失、混合、非法引用或未保存均不能完整通过；未声明扩展保留原 JSON/hash，
不得把历史输出身份自动提升为已保存。完整 storage 正文进入原 receipt/fact hash，Clone/raw clone 隔离指针，
原 receipt 来源反证拒绝同时重算 fact hash 的引用替换／全删。`runtime_validation_output_storage_test.go`
固定旧 wire/hash、非法 raw、重载、隔离和 fuzz；receipt 恢复回归覆盖原 CAS 前失败、响应丢失及 duplicate。
`biz/runtime/validation_output_storage_receipt_test.go` 固定 rejected receipt 的已知 exit/code、已保存流与未保存原因，
以及 rejected fact 重载／来源篡改拒绝；既有前置 accepted evidence 不被删除，失败不新增 accepted credit。
该数据合同不提供 owner raw read 准入，也不开放 adapter、availability 或新的 SM 转换。

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
File existing-read、mutation 的最终 existing patch-input/readback 与 Shell managed-validation scratch copy 是具体 producer。
File 的 mutation/recovery 读取事实从 actual bytes 生成，不能借用 receipt 的历史 `BeforeHash`；恢复确认后像与原
执行读前像保持不同 dependency identity。File mutation 实际确认不存在的目标以独立 existence 资源保存原 absence
观察，不冒空文件 bytes；读失败不生成观察，未保存原 receipt 的恢复仍只能记录实际后像，不能重建过去的 absence。
它不枚举全部 preview/manifest/metadata 读取。
Shell 保留本次实际复制的逐文件 bytes/selector，成功和已完成复制后的失败回执均可由原 evidence transaction 接收。
`biz/runtime/shell_owner_receipt_integration_test.go` 配对真实 Shell 失败 receipt、原始文件 hash 与 rejected fact 的完整
依赖投影；复制不是子进程全部读取追踪或跨文件原子快照。
这些事实不替代 assessment/artifact 的实际依赖、current owner observation 或相交资源失效/reopen。
终态每个 `RuntimeRequirementEvidenceItem.OwnerRevisionSet` detached 保全原 evidence 的读取集合，
包括 rejected/stale 历史项；完整正文参与 closeout/material seal hash，raw 校验拒绝非法集合/hash/Definition。
集合是原读取快照，不是 current owner observation；Runtime evidence chronology、当前 binding 或等价 lineage
不能将某个读版本提升为 owner 最新版本，也不能任取最新版掩盖不同证据的依赖冲突。nil 不补造无依赖证明。
terminal top-level 集合对 versioned proof 只消费原 SM-2 current observation 的完整集合，不把 mutation 历史读前像
再次并入当前集合；逐项原读取和 applicability snapshot 仍完整保全。legacy nil 协议保持原读取语义，不升级。
terminal top-level 集合只合并 exact current proof 的依赖，以及 latest satisfied assessment 仍适用的
原 proof/owner-fact consumed-field reads；不再构造 `runtime_authority/accepted_evidence` synthetic refs。
历史/拒绝项仍保全逐项快照，不进入 current union；相同资源冲突版本严格拒绝，nil 不制造 sample。
owner-fact fingerprint 只表示 classifier 实际读取的 catalog 字段，不追溯底层资源或证明当前性。
未采样的 catalog/artifact 资源、currentness/reopen 和完整依赖范围仍须独立接线。`runtime_terminal_source_obligation_test.go` 覆盖原读取快照的
重载保全、历史多版本并存但不恢复 credit、clone 隔离与篡改拒绝。
反馈合同为 v5：v4 及更旧 aggregate 不升级、不回填或重驱，必须在 load/drive/material 边界拒绝；
新 activation 封存 v5，同版本恢复从原 aggregate 编译并重放同一 seal。

assessment 的 `RuntimeAssessmentAdjudicationProof.OwnerRevisionSet` detached 保留 classifier 实际选中 evidence 的
原集合。SM-2 commit 与 SM-3 completed dispatch 双账 hash 绑定完整 proof 正文；fresh-load 用原 dispatch evidence cutoff、
sealed binding、completed operation、原 fact hash 和 receipt 来源重编后 exact-match。删除依赖或同时重算两侧 hash
不能代替原来源。proof normalization/raw clone 保留非法 raw 与 nil/empty 表示，独立集合/hash/Definition 门禁仍执行。
逐 proof 集合不被强行合并或任取同资源版本，也不代表 classifier 读取的 catalog/owner postcondition、artifact 或
完整 assessment 依赖；nil 不补造无依赖，现有 disposition、credit、预算、reopen 与 SM-1/SM-4 transition 不变。
`biz/runtime/assessment_proof_dependencies_test.go` 固定 satisfied/repairable、cutoff、重载、隔离与协调篡改；
`biz/runtime/assessment_proof_dependencies_recovery_test.go` 固定完整 aggregate 门禁下的 CAS 前失败与 CAS 后响应丢失，
重启不重复提交或替换原 proof 来源。
`biz/runtime_assessment_proof_dependencies_mainline_test.go` 在既有真实 Shell/File/Run 装配及三 owner ACK 恢复中
反查 receipt→fact→selected proof。provider 仍是脚本替身，这些验证不替代公开正式累计验收或 live。

workorchd 的既有 `/api/v1/workorch/runtime/payloads` 入口接受 `validation-output` 的不可变读取引用，
通过 Orchestrator 的 `QueryRuntimeValidationOutput` 只读合同调用 `queryRuntimeValidationRawOutput`，
作为 Shell 受限原文读取 topic 的唯一生产 sender；Runtime audit 的每阶段 stdout/stderr 投影发布该引用，
Web 只展示下载入口、原字节数/SHA256、保存字节数及不完整/未保存原因。adapter 绑定
Workspace/Run lane 并完整校验 typed response，Shell owner 仍独立反查 persisted receipt、execution key/fence、stage/stream、
Workspace internal directory 与保存引用。返回值为 detached bytes，只读过程不写 aggregate、不授 evidence credit、
不清除 unknown effect，也不改变 SM-1～SM-4 transition 或预算。`runtime_shell_raw_output_receipt_integration_test.go`
覆盖应用 Reader→Orchestrator→Shell→原 receipt 的合法读取、错 identity、异引用、owner/repository 重建与 Workspace retirement 拒绝。
`runtime_validation_output_reader_test.go`、`runtime_validation_output_test.go` 和 `http_cache_test.go` 固定二进制原文、
保存 hash/完整性、投影引用及 no-store 边界；这些只读查询不代替 Runtime 的证据判断。
relay 只接受 exact `application/workorchd/appservice` strong-call，不能把其它 capability caller 升级为 Shell 的 Orchestrator caller。
`runtime_validation_output_guard_test.go` 固定非法 source、nil/Post、错 DTO、Workspace context 冲突与 execution scope 漂移
在 Shell owner I/O 前拒绝。HTTP 同时保留 capture 遗漏与 storage 遗漏的独立原因，不以保存前缀冒充完整原输出。

`RuntimeAssessmentAdjudication.OwnerFactSources` 在同一 SM-2/SM-3 assessment CAS 中保全实际消费的逐 required
owner postcondition：exact requirement 与 pre-seal catalog snapshot 单独记录，`OwnerRevisionSet` 只绑定该 operation kind
的 surface identity/readiness/proof applicability/ready publication/exact requirement binding observation。scope/selector 与
observation 分域 hash；classifier 与 dependency projection 共用同一字段观察，独立旧 classifier reference 固定布尔结果等价。
整 catalog observed revision、无关 targets 或 diagnostic 不进入 scoped dependency revision。
matching catalog 缺失保留 requirement 和 nil source，不伪造 absence。来源与完整依赖正文进入 adjudication hash，raw clone、
normalization 和 JSON reload 保全 detached 正文，fresh-load 从原 `AssessmentAuthorityInput` 重编后 exact-match。
`runtime_assessment_owner_fact_sources_test.go` 与 `biz/runtime/assessment_owner_fact_sources_test.go` 固定独立 observation
oracle、无关变化、相交变化、raw/hash/重复/Definition 门禁和协调篡改；既有 recovery 测试覆盖 CAS 前失败与响应丢失。
真实 File/Shell/Run mainline 与三 owner ACK 恢复验证原 catalog 来源。这仍不覆盖 classifier 的所有 binding/lineage/guard
输入、formal artifact、current query 或 terminal union，不改变 disposition、credit、budget、reopen 或 availability。

File 的 `QueryRuntimeFileReadDependencies` 是只读 concrete owner query：adapter 经 Workspace typed read 取得当前目录，
File 核对原 sealed `file/read`、exact Definition、原集合和 receipt hash，再通过 receipt-owner query 的 `ReadSource`
从同次 loaded aggregate 反证原 transaction、operation scope、execution authority/key/fence 与 receipt ledger。
receipt miss 不授执行；current bytes 按原 range/MaxBytes selector 重读并重算 dependency hash，missing 不返回完整 set，
失败不作为 missing。回应 exact echo request/state revision/source receipt hash，adapter 完整校验并 detached 返回。
新查询在 BeginShutdown 后拒绝，已接纳 query 连同 nested source read 等真实结束；原 receipt replay 的 drain 不变。
非空 `ApplicabilitySource` 显式选择原 File mutation receipt 的实际后像双集合协议；SM-4 的
`driveRuntimeEvidenceApplicability` 从有效 aggregate 编译一项 exact obligation，只执行 concrete owner query，
随后提交 SM-2 private `EvidenceApplicabilityObserved`。Shell managed-validation 的非空复制快照同样通过独立
`QueryRuntimeValidationReadDependencies` 重读原 selector；Core 不解释 File/Shell 的 selector 或 bytes grammar。
query 错误零 observation，missing 是 owner 事实。writer 重验原 fact/receipt/body、state/evidence/reconciliation
fence，在单 CAS 追加观察；stale 精确 reopen 原 CWI，退休相应 semantic/reconciliation authority，不改原 effect/预算。
相同结果重复 query 不漂移 equivalent-authority identity，已 stale 的 source 不因 bytes 恢复而复活。
新 evidence revision 或 Resume/reconciliation request generation 要求 fresh query；versioned proof 未观察不得用于
assessment、无 assessment credit 或消费 fresh-validation continuation。历史 assessment 仅反查其 sealed observation
append-only 原 prefix，同 EvidenceRevision 的未来观察不能污染旧前像。
仍适用的 current/remaining/completed focus 原 source 都参与采样 fence，最后 CWI 先采样不能关闭整个 Run。
未完成读取以 decision-only `evidence_applicability/v1` 投影为既有 `need_prerequisite`，不进入普通 acquisition
`Pending/Satisfied` ledger；identity 绑定原 fact/hash、revision/generation，fresh-load 反查原来源，不赋予 operation
或 provider authority。它不把采样债务伪装成 state-change、validation 或 target-grounding 缺口。
matching 观察提交后复用普通 CWI advance 与 semantic preparation，再 reduce；原候选/调用责任依旧独立管理。
`assessment_applicability_seal.go` 独立反查新 assessment 的观察 prefix 和 reconciliation generation；attempt-begin
及 final seal 都不得越过 fresh query。观察 semantic identity 改变的同一 CAS 退休未封存 candidate/unbound request，
原观察 request 是持久退休原因，不生成 correction 或 attempt-1 权限。submitted provider 的原结算责任优先，
不允许观察丢弃 allocation；bound document 与旧 receipt/effect/预算均保持原值。
SM-4 的 `settleRuntimeSubmittedProviderBeforeObservation` 在新 reconciliation batch／owner采样之前，优先交原
`ProviderRequestTerminalCoordinated` 处理唯一 current-authority 的 recovered submitted session。没有 durable outcome
时仍按原协议结算 unknown 及SM-2 failure，不再次请求 provider；complete document保持原 admission lane。
新 request generation可以先封存，但不得先改变原提交 authority。非唯一／历史 mismatching authority fail-closed，
不能借此重绑定历史 session。真实 File/Shell receipt、file-backed Run、提交切点重建及 settlement CAS前／后响应丢失由
`TestDriveRuntimeShellApplicabilityResumeSettlesSubmittedRequestBeforeSampling` 验证；provider documents是脚本，
durable submission切点不证明远端调用已发生，historical late-settlement不在该测试范围。
终态 current union 对 versioned proof 取已裁决的实际 current set，仍拒绝相同资源冲突版本。
`terminalMaterialSource` 保留 reconciliation ledger；共享材料 compiler 使用 exact revision/generation，而不是
较早 matching observation。成功材料缺少本轮观察 fail-closed，失败/取消/替代材料仍保留历史并正常交付。
本路径不证明持续 freshness、完整 assessment/artifact/catalog currentness，也不开放 availability；SM-1 不变。
`biz/runtime_file_read_dependency_event_adapter_test.go` 使用实际 File 与 typed Workspace/receipt fixture；
`biz/runtime_file_read_dependency_receipt_integration_test.go` 在同一 Hub 连接实际 File、execution adapter、Orchestrator
receipt handler/controller 与 current-query adapter，原 read 经 candidate/claim/receipt/evidence 完整门禁，逐 CAS 校验
完整 aggregate。Workspace、CAS repository、pre-seal semantic 输入与非授信 auxiliary decision 是显式 fixture，
不声称 production reducer 已选择读取。独立 actual bytes/range/bound selector oracle 固定无关／相交变化；协调重算
Definition/operation/dependency 身份不能替代原 receipt，重载／owner 重建、missing/error 和 BeginShutdown 查询均零写入。
`biz/runtime_operation_receipt_source_test.go` 和 `biz/runtime/operation_receipt_test.go` 独立反证原 persisted 来源；
这些组合与局部配对不冒充完整公开 drive、SM-2 freshness 闭环或 live 验证。

`CompileRuntimeOperationReadSource` 从完整有效的 persisted aggregate 重编原 dispatch/operation/execution/receipt
读取来源，反证 exact receipt hash、key/fence、governance binding 与 sealed operation policies；只有原回执带实际依赖
才能返回 detached context、operation 和原集合。它不使用当前 decision、目录投影或进程内原 request，nil 依赖不补造
无依赖证明；`RuntimeOperation.Clone` 复用原 raw-clone helper，隔离参数指针并保留正文与 nil/empty 表示。
File adapter 在原 read 或显式 versioned mutation owner 边界封装 query；Shell adapter 只传原 copied selector 来源。
当前目录由 Workspace typed read 提供，owner 再反查原回执。
`biz/runtime/operation_read_source_compiler_test.go` 固定 claimed/missing/nil 来源拒绝、owner-committed 与重载等价；
实际 File 组合回归对照原执行 request，覆盖 completed dispatch、来源篡改、返回隔离与 owner 重建后重编。
source compiler 和 query 自身不写 CAS/credit/预算；只有 SM-2 private applicability event 可提交观察与裁决。
`CompileRuntimeEvidenceApplicabilityRead` 在同次完整 aggregate 门禁内同时编译适用性 request 与 detached 原 receipt
读取来源；公开独立 source/request 入口仍各自执行完整门禁。内部 continuation 不接收已验证标志，不缓存结果，
只在同一原始 aggregate 上复用本次已执行的完整遍历。File/Shell typed query 仍核验原 receipt/key/fence，SM-2
观察写入仍独立反查来源、当前版本及 CAS。SM-4 仅减少重复只读编译，不改变 SM-1/2/3、预算、credit 或恢复权威。
`runtime_file_applicability_mainline_test.go` 配对真实 File mutation/query 与 SM-2；
其中 matching/changed/missing × CAS 前中断/CAS 后响应丢失重建实际 File owner/controller，并原 JSON reload；
before-CAS 仅重查保存一次、after-CAS 不重复观察，原 receipt/effect/预算不变。相同 Hub 和 Workspace/CAS fixture
不冒 fresh OS-process。`evidence_applicability_candidate_window_test.go` 覆盖 prepared、document-complete、pending、
proposal 的同 CAS 退休/重载/duplicate/来源篡改及延迟 proposal/seal 零写，submitted allocation 不得被退休。
`runtime_shell_applicability_mainline_test.go` 配对实际 Shell/Run owners、原查询 CAS 前中断／CAS 后响应丢失及 owner
重建，恢复不重复 provider/process/receipt。测试使用 scripted provider/private startup，不等于公开 fullflow 或 live。

### 逐源义务与共同 CWI

`evidence-applicability-dependencies/v2` 的历史 effect 用途见
[Evidence Proof Purpose](../../../../docs/00-core/runtime/evidence-proof-purpose.md)。descriptor 发布 owner 用途资格，
`RuntimeHistoricalEffectProofContractForDispatch` 从原普通 Definition dispatch 与 sealed all-of effect+validation binding
编译 execution context；repair／implementation-work commitment 禁止升级中间用途。
SM-3 receipt writer、dispatch/receipt lineage 与 read-source 反查同一原绑定；Core 不以具体 owner 名称选择用途。
SM-2 behavioral matcher 可消费该原行为，但 stale／fresh current 谓词保留，旧 validation stale 仍精确 reopen，
原 effect 不重新执行。Definition assessment 与成功材料须有同 binding 的独立普通当前证明；历史 effect 输入／
后像不进入 terminal current union，逐项历史完整保留。state-change-only／acquisition／v1 不自动升级。
用途资格进入 executable revision；SM-1 不变，SM-4 只驱动真实当前采样、原 seal 与有序三 owner ACK。
历史查询 request 的 `PurposeRef` 由原 receipt／fact 派生，完整来源反证拒绝自报／删除用途，即使重算 request identity。
观察仍遵守原 private CAS 窗口并 append-only 保全实际 stale 结果，但不进入 current applicability semantic hash，
不能退役普通未封存 candidate／request 或改变当前 credit。`biz/runtime/evidence_historical_audit_recovery_test.go`
固定四种候选窗口、CAS 前失败／响应丢失、重载、duplicate 与用途篡改；这是 typed owner-fact fixture。
重载后还以原 transaction 完成普通候选 feasibility／seal，证明审计未将原 request 留在不可继续的旧 action attempt；
新的普通 dispatch 尚无 effect／验收 credit，审计 CAS 的独立完整写集门禁禁止夹带其它状态变化。
共享文件主链的独立实际 File readback 固定旧后像 stale 及终态查询零写，不把该只读查询冒作真实观察 CAS 恢复。

跨 CWI 的 `runtime/cross-cwi-evidence-purpose/v1` pre-seal 来源见同一用途合同。Workspace 独立封存
`EvidencePurposeMappings` 的唯一历史／当前来源，Application 只投影 exact dependency，Web 确认前展示完整两端。
这不自动升级未确认的 state-change-only binding／receipt；ordinary prerequisite 保持原 settled 判断，唯一例外是
由原完整 owner effect 证明的历史进度对 exact acceptance consumer 的准入。mapped bootstrap／seed
由版本化 availability gate 校验原 inventory，bootstrap／seed 仍反证完整 coverage、Profile 与依赖闭包，禁止降级
为旧 same-binding 用途。pre-seal 合同、确认身份、strict schema、克隆／重载与 UI 回归不替代 Runtime 恢复闭环。
`CompileRuntimeSourceObligationChecks` 从原 confirmed mapping 为验收 CWI 合取编译自身与全部点名历史来源的
original verification；历史 CWI 原检查不变。原 Criterion/Declaration 字节保留，consumer check identity 独立绑定
该验收 CWI，shared original verification 仅要求一次。seed、普通 validation requirement、输入评审和逐源材料共用
完整编译结果；`runtime_evidence_purpose_checks_test.go` 固定原文、独立身份、all-of 缺项拒绝、strict review wire、
协调重哈希替换拒绝与重载。该声明接线不授行为或终验 credit。
seed、用途映射与 closeout 的完整逐源校验先克隆并验证同一次调用的 exact inventory，再逐 CWI 编译检查；只复用本次
不可变输入的校验结果，不缓存 hash 成功、不跨调用或恢复复用。公开 check compiler 每次仍完整验证 inventory。
旧逐项重验路径的对照测试固定拒绝顺序、原文字节、缺项／篡改、JSON reload 与后续调用重新验证。
原 inventory compiler 为历史 CWI 生成 `HistoricalAcceptance` 两端与原文／检查身份，普通 semantic obligation 和
sealed binding 保全同一原声明；proof identity 的等价比较不能遗漏用途来源。receiving-owner execution、receipt
admission、dispatch lineage 与 read-source 共用原 dispatch purpose，mapped receipt 拒绝用途缺失／v1 降级和中间
repair／work 升级。`runtime_historical_acceptance_source_test.go` 固定 pure compiler、clone/raw reload、来源替换、
用途降级与等价反例，不替代真实跨 CWI owner 主链。
`RuntimeHistoricalBehaviorCompletion` 只在原 complete receipt/evidence CAS 创建；独立 compiler 重放原 accepted
semantic document／owner feasibility、完整 dispatch、execution/receipt/evidence/processed fence。exact completion
使前驱退出行为执行而保持 `bound/open`，只对原 consumer 放行；共享 consumer 的另一 eligible 历史来源照原准入
继续，前一回执不因等待后继而丢失。`RuntimeHistoricalAcceptanceProof` 在原普通 assessment CAS 内独立反查
封存 cutoff、所有原 checks/native 映射、普通 owner receipt、原 applicability prefix 后追加并结算前驱。
验收 consumer reopened 时撤销前驱 credit，历史 effect 和 acceptance proof history append-only 保全，前驱不重发
effect。SM-4 材料反查同一原用途来源及当前普通 proof，原 Criterion/Declaration/check identity 保留，实际验收 binding
与 evidence 指向独立 consumer；current union 仍排除全部历史输入／后像。`runtime_historical_behavior_test.go`、
`runtime_historical_acceptance_proof_test.go` 与 `runtime_terminal_historical_acceptance_test.go` 是纯 persisted-shape
合同测试；`biz/runtime/historical_behavior_transition_test.go` 是 detached projection／transition gate 测试，均不冒充
真实 owner/CAS 恢复主链、fresh OS-process、公开 fullflow 或 live。

历史行为／验收消费者在同次完整反查内只读复用 semantic-purpose 前像索引；首次仍完整遍历原 ledger，
后续命中须比较独立 raw clone 的完整 ledger，不依据 manifest/binding hash 授信。失败不保存成功产物，
每项 endpoint、binding、startup、receipt/evidence、assessment/cutoff/current-consumer 门禁仍按原次序执行。
`runtime_historical_preimage_session_reference_test.go` 保留原完整三类消费者，fixture/公开归档对照固定完整结果、
首个错误、原输入、返回隔离、重载／后续变化、fuzz 与含首次反查的成本；不改变 SM-1～SM-4、credit、预算或 availability。

同一历史验收／assessment 调用的 execution 来源映射、repair 历史与 fresh input-review 消费者共用上述完整前像；
各自仍独立验证 exact source、ordinary binding、confirmation、receipt/evidence、截止点与预算。
公开独立 review compiler 不保留成功前像，历史 ledger 或嵌套原值变化须重新完整反查；不暴露 caller 成功标志。
`runtime_lineage_preimage_session_reference_test.go` 固定原完整消费者及 assessment history 路径，fixture／公开归档
对照完整结果、拒绝顺序、原 bytes、重载／后续篡改、返回隔离、fuzz 与成本；不新增四机转换或开放 availability。

同一次私有历史／assessment 校验中的 owner receipt、retired repair 与 confirmed ordinary-binding repair
可复用完整 operation lineage 门禁的成功前像。前像完整包含 scope、active/completed dispatch、attempt/session、
governance/resolution、accepted/rejected evidence 与 owner receipts；首次仍按原次序执行全部七项门禁。
命中比较 detached raw 全文，失败不登记；binding、预算、confirmation、source、cutoff 和 fresh validation
仍独立完整核验。`runtime_lineage_operation_session_test.go` 固定逐集合漂移、嵌套 alias、JSON 重载、
后续独立调用及真实只读归档 oracle；不跨 Load/CAS/公共消费调用保留成功，终态独立入口仍完整反查。

同一次 `RuntimeStateRecord.Validate` 的 work、assessment、semantic ledger、accepted-purpose 和历史验收
消费者可共享私有 semantic／历史前像 session；work-round guard 也使用该调用内的完整源校验产物。
每项完整 gate 及原首错次序保留，前像命中仍逐次比较 detached exact raw，不按 hash 授信；public consumer、
后续 Validate、独立 aggregate 前像与 CAS 校验重新开始，不携带 caller 已校验标志或持久化成功凭据。
`runtime_state_consumer_session_reference_test.go` 固定原完整 record gate／次序，fixture 和显式只读公开归档
对照原值、混合失败、同 hash raw 漂移、重载、后续调用、fuzz 和包含首次校验的成本。
此纯值复用不改变 SM-2／SM-3 acceptance、SM-1／SM-4 转换、预算、保存或恢复权威。

同次私有工作／历史／assessment 来源校验还可保存完整 work-state gate 的成功 raw 前像：原 Definition、
全部 Facts、work/semantic ledger、active/completed dispatch、receipt、governance 与 budget 必须完全相等。
purpose vocabulary 重编译、round/dispatch/receipt-set、unknown 消费及所有旧首错门禁在首次仍完整执行；
失败不登记。公开独立 `ValidateRuntimeImplementationWorkState` 总是执行完整门禁，不保存这份成功前像。
`runtime_work_state_preimage_session_test.go` 与冻结原完整 compiler gate 核对逐参数漂移、原输入、alias、
JSON 重载、后续调用、fuzz 和公开只读归档；成功前像不离开本次私有 scope。

同一 work 源反查中的 round、working binding、dispatch commitment 和 collective receipt-set
也可复用上述完整 catalog gate；各自的 round/hash、原 scope、proposal/feasibility、排除集合与 receipt/fence
仍完整反证。mutation-input 集合先逐 catalog 校验再检查 prerequisite 重复，保留原错误顺序和 raw-canonical 门禁；
公开独立 round／binding／receipt-set 入口不复用前次成功。`runtime_work_round_catalog_session_reference_test.go`
保留原完整 round／binding 算法，对照完整结果、首错、同 hash raw 漂移、nil/empty、重载、后续变化和 fuzz。

同次完整 round 门禁成功后可保存 detached raw round，后续 round／dispatch／receipt-set 消费者只有完整原值
相等时才复用；首次失败不形成成功前像，公开独立入口仍完整重验。scope/compiler 与原 hash、catalog、binding
门禁不变；`runtime_work_round_preimage_session_test.go` 用原完整 round oracle 固定首错、嵌套 alias 隔离、
同 hash raw 漂移、重载、后续调用、fuzz 及包含首次校验的真实只读归档成本。

aggregate 的连续历史行为／工作／evidence／applicability／assessment 来源门禁统一由
`ValidateRuntimeConvergenceSourceLedgers` 纯校验；私有 semantic／历史前像不离开这一次完整调用。
每个 interstitial gate、原拒绝标签与次序保持不变，aggregate 后续四机、独立前像、DTO、Load、Save/CAS 门禁保留。
`runtime_convergence_source_ledgers_reference_test.go` 与 `biz/runtime/aggregate_source_session_reference_test.go`
保全原完整独立消费者次序，fixture／只读归档核对首错、原输入、重复、重载、后续漂移和 fuzz。
逐代 manifest→Definition 反查同样在完整调用内共享精确 raw manifest/catalog/seed 与成功 focus 前像，
每代 CWI 全文、普通义务、repair route、Profile 声明和逐 catalog 身份仍完整反编；公开单 manifest 入口重新开始。
`runtime_semantic_definition_session_reference_test.go` 保全原独立完整反查，固定原值、首错、raw alias、
同 hash source drift、重载、fuzz 和包含首次校验成本的归档对照。以上只读复用不新增四机转换、
状态、成功凭据、跨调用缓存、预算或公开 availability。

`runtime-owner-state-joint-preimage/v1` 只在 SM-4 重新读取 Run owner 的单次 Commit 前像编译内复用
DTO→aggregate 的纯来源子校验产物。完整 raw DTO 先验证，aggregate 的声明／feedback／来源与全部剩余
invariant 保持原首错次序；成功命中须比较独立原值的完整子树，转换差异重新完整反证，不凭 hash 授信。
`ValidateRuntimeStateReloadSourcePair` 不返回 session／成功凭据；owner 的 `CompileOwnerRecord` 通过同步纯
continuation 按原顺序执行全部 interstitial／剩余 aggregate 门禁，只返回同次调用的一次性 detached 产品。
aggregate source 与其后原位置的 semantic-ledger gate 也在一次完整调用内共享私有上下文；continuation
不接收任何 session、成功子产物或已验证标志，不进入 owner I/O。独立 aggregate Validate 每次重新创建
本次上下文，不从 joint／前次调用借用。普通 Load/recovery/Save、独立 Validate、
after-image、scope/revision、跨机迁移与 Run CAS 均保持完整校验，不跨调用／重启缓存。
`biz/runtime_owner_state_joint_preimage_test.go` 对照独立 DTO＋aggregate 路径，覆盖两侧独立篡改、同 hash
raw 漂移、首错、原值／返回隔离、后续调用、序列化拒绝、只读公开归档及 fuzz；原真实 Run-owner
preimage CAS 回归继续覆盖提交前失败、提交后响应丢失和并发写入。完整 Commit benchmark 包含首次校验，
不以局部成本变化证明公开 fullflow 或 live 已完成；SM-1～SM-4 的 transition、预算和 availability 不变。

`runtime-owner-state-transition-transaction/v1` 由 `ApplyInternalEvent` 在完整 envelope／kind gate 后创建
一次局部 controller/kernel；`owner_state_transaction.go` 只包装首次 Load，coordinator 仍持原 repository
直接重读前像并 CAS。具体 handler、纯 reducer 和原位置完整 invariant 不变，既有 SM-2／SM-3／SM-4
协调写集与首错顺序不变，SM-1 不变；普通 Load/recovery/Save 与公开独立 Validate 每次重新开始。
`CompileRuntimeOwnerStateTransaction` 在词法调用内保留纯 source／semantic 子产物，不导出 session、成功
凭据或已验证标志。owner 两组 before／after-semantic invariant 分别反查完整 detached aggregate raw 超集，
精确原文相等才共享本次成功，变化、失败或另一组均不能命中；source／semantic／预算／迁移／CAS 门禁保留。
退出、异常及 panic 均撤销 gate；禁止递归／并发使用，Commit 的一次性 request 原子清除 compiler 引用。
上下文不进入共享 controller、context、EventHub、持久化、结果或下一事件，不凭 hash 授信，不增加执行权限。
唯一跨事件纯子树例外是 `runtime-owner-historical-subtree-compiler/v2`：controller 私有
`RuntimeOwnerHistoricalCompiler` 仅保留同一 workspace/run 的 detached semantic manifest/document、catalog、
Definition 与工作 scope/round 纯子产物，完整 raw 精确相等才可复用；所有当前状态门禁仍逐事件运行。
已退休 semantic generation 的完整 raw／normalized 双参数门禁及其完整用途前像也在白名单内，
enclosing ledger 的 current、append-order、duplicate 与 Definition join 每次执行，active 用途前像不缓存。
完整 work-state／historical-source／owner invariant／Commit 产品不在跨事件白名单内。并发／递归走独立冷编译，
事件失败／panic、scope 切换整体淘汰；成功事件以确定性有界子集保留最多 128 个完整条目（包含嵌套 document），
超限只淘汰纯子产物，不清空仍可保留的子集，不改原始 owner 数据。优先保留 seed／scope／round／历史来源和节点，
再保留 catalog、完整 manifest 及剩余额度内的 document。map key 排序仅决定淘汰顺序，命中仍逐原值核验；
淘汰后走原完整冷门禁。真实排空后 Close 释放，重启从冷状态重新读取。
此例外只影响 SM-2/3/4 私有事件的纯编译成本，SM-1、全部 transition、预算、scope/revision、CAS 和 availability 不变。
`runtime-owner-query-reload-compiler/v1` 另外允许 `RuntimeController.Load` 使用同一个私有历史纯编译器：
每次通过 `LoadOwnerStateOriginal` 真实向 Run owner 读取并独立转换，完整执行 DTO→aggregate 原顺序门禁，
仅复用 exact-raw 白名单纯子产物。当前 Facts／预算、两组 owner invariant 均重新完整执行，不保留 aggregate
成功前像或导出凭据，错误返回零 aggregate；查询不运行 reducer、Save 或 CAS。普通 repository Load/recovery/Save、
独立 Validate 与 Commit 合同不变，无 raw 接线时回退原 Load。查询与事件遵循同一 scope、失败／panic、并发冷路径、
淘汰和真实排空后关闭规则；不增加四机转换、权限或预算，不改变十分钟 fullflow 与 availability。
`runtime-owner-applicability-read-compiler/v1` 将 SM-4 的证据适用性读取投影接到同一私有历史纯编译器。
`RuntimeController.CompileEvidenceApplicabilityRead` 对每次完整 aggregate raw 执行与独立 `Validate` 相同顺序的
全部门禁，只有白名单内完整精确相等的历史子产物可复用；请求、source、当前 Facts／applicability／budget、
owner invariant 或 aggregate 成功均不保留。该方法不读取 Run owner、不运行 reducer／CAS，输入来自原 drive
重载／已提交迁移的 aggregate；它不是一次新的重载证明。随后仍真实查询 capability owner，并由原 private
observation event 重新读取 Run owner、反查来源及 revision、执行全部迁移与 CAS。独立公开 compiler 保持冷门禁，
不导出 unchecked continuation／成功凭据，不扩大 128 条上限；失败、scope 切换、并发冷路径、Close 与重启规则不变。
`runtime/evidence_applicability_owner_compiler_test.go` 对照完整冷结果与首错，覆盖当前预算、receipt、scope、重复、
返回隔离、关闭、重建与并发；只读公开归档 oracle／benchmark 同时包含真实历史子树，不冒充全链路通过。

可选 Go execution trace 在 query reload、private event transaction／load、drive transition、applicability、
Commit／reread 和 Run query／CAS 记录固定 `workorch.runtime.*` 区段。区段不携带 Workspace、Run、provider
原文、配置或凭据，不进入持久账本、EventHub DTO 或 authority；关闭 trace 时不收集区域诊断。
`scripts/lib/runtime-trace-summary.py` 流式读取完整 `go tool trace -d=parsed` 输出，按 goroutine 配对嵌套区段，
分别统计 inclusive／self wall time；并发与嵌套不能相加为端到端耗时，wall time 不能称为 CPU time。
缺区段、未结束、错配或非单调时序不能记完整通过。该诊断不改变 SM-1～SM-4、预算、恢复或十分钟判据。

`runtime-owner-lineage-subgraph-compiler/v1` 追加 owner-private 历史纯节点索引：只保留完整原门禁通过的
closed dispatch intrinsic、operation-admissibility catalog、owner receipt 与 repair-surface confirmation raw 副本。
候选身份不授信，每次命中完整精确比较所有实际参数、嵌套原值及 nil／empty；active dispatch intrinsic 不缓存，
raw／normalized 子参数沿用原调用位置，不能以规范化掩盖漂移。已验证 receipt 的纯 hash 产物仅在原 hash 位置、
完整 raw 精确命中后复用，不凭 hash 授信、不代替 join。集合分区／scope／顺序／重复、candidate admission、
request／dispatch／execution-key／governance、receipt／execution／evidence／processed fence 跨账本 join 仍每次执行，
当前 Facts、版本／applicability、预算、cutoff、后续失败与所有原首错门禁不变。完整 repair／assessment／lineage-source
或 aggregate 成功不跨事件保存；普通独立 validator 不借用索引。新增节点计入原 128 条完整上限，沿用同一 scope、
失败／panic、并发冷路径、淘汰和关闭规则，不持久化、不导出凭据、不跨重启。SM-2/3/4 只变纯编译成本，
SM-1、transition、owner I/O、scope/revision、after-image、CAS、effect、预算及十分钟 fullflow／availability 不变；
完整路径成本倒退则撤回。`runtime_owner_lineage_index_test.go` 对照独立门禁覆盖完整 raw 字段漂移、alias 隔离、
nil／empty、同身份命中、active／failed 零保留、重复／顺序／scope／跨账本 join 与只读公开归档。
`runtime/owner_state_query_test.go` 对照独立 DTO＋aggregate 门禁，固定首错、当前漂移、逐次读取、错误重试、
返回隔离、并发冷路径、事件交替与 fuzz；`biz/runtime_owner_query_test.go` 验证真实 Run owner 的 revision
可见性、零查询写入、stale CAS、文件重启，以及只读公开归档漂移和包含首次编译的完整查询 benchmark。
`runtime_owner_historical_compiler_test.go`、`runtime/owner_historical_compiler_test.go` 对照冷完整门禁固定
漂移、首错、原值／nil-empty 隔离、生命周期、并发、淘汰和只读归档；真实 owner CAS／restart 回归继续适用。
`runtime/owner_state_transaction_test.go` 对照完整独立门禁，固定 raw／converted 漂移、独立 owner gate、
原值隔离、错误顺序、关闭、失败、panic、递归／并发拒绝、重复事件、stale／CAS failure 与 fuzz；
`biz/runtime_owner_state_transaction_owner_test.go` 验证真实 Run owner 的两次读取、CAS 前中断／后响应丢失，
重建文件 repository 与 owner 后精确续驱且不重复消费预算或 effect。完整事务 benchmark 包含首次 Load、
具体 reducer、after-image、Commit 重读、全部迁移与 CAS 调用；不代表公开 fullflow 或 live 已通过。

`runtime-owner-invariant-source-compiler/v1` 将 owner after-semantic continuation 的 Definition 来源、accepted
batch binding、document／feasibility、obligation-purpose 及 accepted-assessment 来源反查接到既有私有历史纯编译器，原门禁位置、原始参数、
全部 join 与首错次序不变。`RuntimeOwnerInvariantSourceGate` 仅提供完整纯校验方法；不导出 semantic session、
成功凭据或 caller 已验证标志，返回的 binding／purpose 是普通 detached 纯值，不授执行或终验权。
assessment gate 每次完整验证原 ledger 后执行全部 attempt／session／dispatch join，不借用 caller 已验证假设。
nil gate 走独立冷校验，非 nil 零值／关闭／递归／并发 gate 拒绝；continuation 退出或 panic 清除引用，
失败 sticky 且不能由 owner 忽略后返回成功。owner contextual error 保留，跨事件仍只保留既有白名单子产物，
不保留完整反查／inventory／owner invariant／aggregate 成功，不扩大 128 条总限。
当前 focus/CWI/candidate/dispatch/reconciliation、Facts、版本／applicability、预算和全部剩余 invariant 仍完整执行；
SM-2/3/4 只变纯编译接线，SM-1、所有 transition、真实 Run 重读、scope/revision、after-image、迁移、CAS、effect
和十分钟 fullflow／availability 不变，成本倒退撤回。独立 Validate、普通 repository、冷重启和既有恢复路径不授信。
`runtime_owner_invariant_sources_test.go` 用独立完整 gate 对照完整产物、首错、同 hash raw 漂移、nil/empty、owner
缺失／重复、返回隔离、捕获／复制／序列化／panic／失败／并发及 fuzz；`runtime/owner_invariant_sources_test.go`
固定完整 owner 查询的独立冷路径首错与当前语义／预算漂移，原查询 archive oracle 和完整事件/CAS/restart 回归继续适用。

`runtime-dto-source-ledger-compiler/v1` 将 DTO／source-ledger assessment 和 confirmed-repair 的 obligation-purpose、
owner-fact catalog 与 operation-lineage working commitment 嵌套校验接入同一私有纯编译器。
只读复用既有 exact detached raw semantic catalog／work-round 等白名单子产物；不另存完整消费者成功，
不新增白名单、历史索引种类、128 条额度、凭据出口或跨重启缓存。原 report、binding、当前 Facts、版本／适用性、
budget／cutoff、顺序／重复、cross-ledger join 和原首错位置全部保留。公开独立入口冷编译，owner 查询／事务
仍真实读取 Run owner，scope/revision、after-image、跨机迁移及 CAS 不变；SM-2/3/4 仅改变纯校验成本，
SM-1 与全部 transition、effect、原十分钟 fullflow、预算及 availability 不变，完整路径成本倒退撤回。
`runtime_source_ledger_compiler_test.go` 对照独立冷 gate 和旧完整 assessment／repair consumer，覆盖同 hash raw
漂移、nil/empty、原文、alias／返回隔离、duplicate／首错、当前来源与预算、重载及只读公开归档；
既有真实 Run-owner CAS／restart／stale／terminal-material 回归继续适用，局部通过不证明公开 fullflow/live。

事务内的历史行为／验收和完整 assessment-history 门禁分别复用 exact full raw source 成功前像，
两者互不授信；explicit facts／completed／lineage 与来源不相等时，assessment 仍执行原完整路径。
独立单项 adjudication 的 authority 缺失在原 adjudication／hash 门禁之后显式拒绝，不得 nil 解引用；
此前已经定义的首错顺序保持不变，成功前像不能遮蔽缺失。
普通独立 consumer 不启用此复用，失败不缓存。`runtime_historical_source_compiler_test.go` 用独立旧完整
消费者对照首错、全字段漂移、嵌套 alias、JSON 重载和只读公开归档；raw clone 的全图隔离还必须覆盖
receipt governance／external dependency 的授权上下文及其 nil／empty 嵌套集合，不通过规范化修复原值。

DTO 与 aggregate 的完整 lineage 来源使用同一字段投影，包含原 `Reconciliation`；不得遗漏字段来制造
事务内命中。`runtime_historical_input_review_compiler_test.go` 逐字段核对持久来源投影。原 accepted binding 的
input-review 纯投影仅在本次 `runtimeHistoricalPreimageSession` 内复用：每次先核验完整 detached raw ledger，
再比较完整 raw binding；返回值独立复制。ledger 原值变化即清除该 ledger 的投影容器，失败不缓存，
该容器不进入跨事件 128 条历史白名单，不保存当前验收成功，也不授 credit 或修复权限。

私有 `runtimeRawEqual` 对完整 raw 值保持 `reflect.DeepEqual` 语义，包括类型、nil／empty、指针／slice
同址和 NaN；只缓存不含业务值的反射类型比较计划。普通 typed 树不分配逐节点图访问表；动态 map／interface、
不透明值及深递归回退标准比较器。它不持有 Workspace、hash 或校验结论，不减少任何字段门禁。
`runtime_raw_equal_test.go` 对照标准比较器覆盖完整 DTO 字段变更、循环图、并发类型计划及 fuzz。
上述接线仅改变 SM-2/3/4 纯编译成本，SM-1、完整首错、真实重读、迁移、预算、适用性和 CAS 均不变；
公开十分钟 fullflow 及实际三 owner ACK 必须单独通过，局部 benchmark 不构成收口证明。

`TestDriveRuntimeCrossCWIHistoricalAcceptanceOwnerCASRecovery` 使用真实 File／Shell／Git／Run／Workspace／Result
及脚本 provider 固定 create→edit→Git、延期零 credit、普通 all-of 后结算、receipt／assessment／material／delivery
切点恢复和 validation／已结算 acceptance stale 后撤销再验；历史 effect 不重发，proof history append-only。
`TestDriveRuntimeCrossCWIHistoricalAcceptanceProcessRecovery` 在 behavior CAS 后真实退出第一个进程，新进程以
独立 Hub／owners 从原 file-backed aggregate 续驱，核对原 bootstrap、key/fence/hash、单次预算和有序三 ACK。
`TestDriveRuntimeMappedHistoricalBehaviorRepairsAcceptanceAcrossRestart` 固定真实行为失败→精确 owner 修复→fresh
普通 all-of，四切点恢复不增加 provider／effect，原输入与全部来源准则不改写。confirmed repair 的续接消费反查
原匹配观察 prefix；assessment／Resume 新采样债务不复活已消费 directive，也不把旧观察提升为 current credit。
这些 private admitted startup／scripted provider 回归支持版本化用途执行，不开放默认 ImplementationWork，
不冒充公开 authoring fullflow、真实模型或 live 验证。

`runtime/cross-source-execution-proof/v1` 只允许封存的 execution work-round/repair guard 反查用户确认的唯一原 DoD
来源，消费者普通证明仍独立合取。Application 添加 exact source predecessor；SM-2 从原 accepted semantic batch
反查 source 的 CurrentBindingRef、manifest/binding/focus 和普通 Definition purpose，不借用 repair 或退休 binding。
缺失／歧义／漂移均拒绝，不从 Runtime 文本选源，不开放默认 ImplementationWork policy 或 public availability。
完整合同与局部证据边界见 [Cross-Source Execution Proof](../../../../docs/00-core/runtime/cross-source-execution-proof.md)。

`runtime/goal-source-execution-proof/v2` 独立支持经确认的 Goal/root work/repair 消费者到唯一原 DoD 的来源映射。
完整 Goal bytes 及规则随规格确认、startup/inventory 封存；原 DoD 普通当前验收处同时保留全文普通检查及
`GoalExecutionAssertions`，不生成 Goal 工作 CWI、工作轮次或自依赖。Bootstrap/fresh-load 从原 inventory/Profile
独立反编，SM-2 在普通证明后反查 exact current ordinary binding/accepted semantic batch 及真实原 work/repair。
SM-3 复用原 manifest/requirement/composite review/dispatch，SM-4 只重驱与交付；v1 仍拒绝 Goal，旧归档不升级。
`runtime_goal_execution_source_mainline_test.go` 固定真实 owner repair、CAS 切点恢复和 duplicate delivery 边界；
该 private startup/scripted provider 证据不开放默认 ImplementationWork 或宣称公开 fullflow/live 已通过。

`runtime/profile-deliverable-evidence-purpose/v2` 将已确认的 required Profile deliverable 显式映射到唯一原 DoD
普通验收 consumer，不将交付来源伪装成 DoD。确认前封存并展示 Profile ID、deliverable ID、完整单 Profile
snapshot hash 和原 Description 全文；Workspace admission、bootstrap、seed、semantic binding 与重载分别反查
原来源。全 Profile 集合内交付 source ID 必须唯一；optional、重复、漂移、缺项和旧 v1 wire 携扩展均拒绝。
source-check compiler 为两端保全 Profile 全文检查，consumer 合取自身全部原 DoD checks，材料重编同一来源。
consumer 的独立工作策略不授终验 credit，历史 behavior 只解锁精确 consumer，终验仍须普通 Definition
all-of assessment 与真实当前 owner observation。SM-1 不变，SM-2/3/4 复用原行为、验收及材料私有 CAS，
current union 不含历史输入／后像，预算及三 ACK 门禁不变。
`TestDriveRuntimeProfileDeliverableHistoricalAcceptanceOwnerCASRecovery` 覆盖真实 File/Shell/Git/Run/Workspace/Result
主链、行为／验收 CAS 前后恢复及 stale 撤销再验；
`TestDriveRuntimeProfileDeliverableHistoricalAcceptanceProcessRecovery` 覆盖真实进程退出、新 Hub/owners 与原
file-backed aggregate 恢复。二者采用 private startup/scripted provider，不代表公开 authoring fullflow 或 live。
Profile 的 `RequiredEvidencePurposeContractRef` 仅声明确认前必须封存的来源关系，空扩展不改变既有 Profile hash。
Workspace 与 bootstrap 从完整 Profile 集合核对必需关系、原文及单 Profile hash，缺失在进入 Runtime 前阻塞；
不在运行期补映射或升级旧 aggregate。`TestMappedProfileHistoricalSourceFourRealWorkRoundsThenOrdinaryAcceptance`
覆盖初始历史 File effect 后，同一文件四轮真实修改、零提前 validation credit、普通终验结算及 Git 后继，
以及普通证明 stale 后原消费者重验。`TestDriveRuntimeProfileDeliverableFourRoundProcessRecovery`
覆盖行为提交后退出进程，再由新 owners 继续四轮及终验，保留原 receipt 和预算。这些是确定性 owner 回归，
不替代公开十分钟 fullflow 或真实项目 live。

owner-fact assessment 的同 CAS handoff 使用刚裁决的 exact proof preimage，不能把新增 assessment sequence 当成
新 owner sampling。Definition／seed／activation／focus、receipt／evidence／applicability／semantic 和 catalog 必须
未变；唯一 committed dispatch／adjudication 与 assessment hash 逐项相同。后继或历史前像不能复用此窗口。
`biz/runtime/assessment_owner_fact_preimage_test.go` 固定 drift、duplicate、CAS 前中断／后响应丢失及重建，
原 receipt／budget 不变、单次 reopen 且无 terminal credit；不证明公开完整多焦点验收或 historical late settlement。

Workspace eligible candidate 的 immutable `RuntimeSourceObligationInventory` 随 Definition/seed/startup/hash
封存；Application 只编译 exact source checks，不创建运行期 authority。每项 verification 的 check identity
绑定完整 inventory、Definition、CWI、ObligationID、RequirementID、VerificationID 和原 Criterion/Declaration。
同 CWI 的全部 source checks 与共同检查进入原 validation requirement 的 all-of 集合，不增加可变闭包 ledger。
Workspace compiler、Runtime bootstrap、fresh facts 与 semantic request 逐次反查 exact
`ProfileID + EvidenceRequirementID + Scope` 及每个 DoD source 的合取声明贡献；库存自洽 hash 不免除适用性校验，
声明存在也不代表 owner readiness 或 proof。`runtime_source_obligation_applicability_test.go` 覆盖缺失、重复、错源、
无 proof operation、wildcard 冒充 exact recipe 与同版本重载；activationcore 回归独立证明启动／fresh facts 的拒绝落点。

inventory hash 的一次编码使用私有短字段缓冲，保留原 SHA256、长度前缀、字段顺序、cardinality 与完整原文字节；
不缓存 inventory 或校验成功，也不改变 SM-1～SM-4、预算或 availability。
`runtime_source_inventory_buffered_hash_reference_test.go` 保留原完整编码，边界／非法 bytes／长原文、重载、
后续漂移、fuzz 与分配对照固定同一身份；完整来源反证门禁仍逐次执行。

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

`runtime/cwi-suspension/v1` 的安全暂停／普通重验／原后继恢复见
[CWI Suspension](../../../../docs/00-core/runtime/cwi-suspension.md)。SM-2 原 applicability-observation CAS
封存 `Facts.CWISuspensions` 的完整来源；current bound fence 本身不放宽。首版只支持唯一已声明 validation-only
ordinary 前驱，state-change-only 不补造来源，明确 `cwi_revalidation_source_unavailable` 失败。
原 frozen binding/effect/receipt/budget 和 repair debt 保留；新普通 assessment 与本轮 owner sampling 后才恢复，
旧 owner qualification 不复用。独立 coordinator 前像／写集与 aggregate/schema reload 双门禁反证来源。
`TestDriveRuntimeCWISuspensionRevalidatesOrdinaryPredecessorWithinOriginalBudget` 配对真实 File/Shell/Run/Workspace/Result、
暂停／恢复 CAS 前后四切点、原预算、单次 effect、来源篡改与三 ACK；scripted provider/controller 重建不冒 live/fresh-process。
`TestDriveRuntimeCWISuspensionProcessRecovery` 在暂停／恢复 CAS 后真实退出进程，新 Hub/owners 续驱原 JSON，
保全 pause/receipt/原分配和消费；仍不是公开 authoring fullflow 或 live 模型验证。
`TestDriveRuntimeDurableDispatchBudgetRejectsUndeclaredPredecessorRevalidation` 保留原无普通来源输入作为明确失败负例。

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
- SM-3 的 operations ExpectedDocument 封存 `OperationsRenderContractRef`；fresh authority 默认使用
  `operations-render/cache-layout/v4`，已封存空引用、v2/v3 继续原渲染。
  同 authority/manifest 的已封存 session（含空引用 legacy）决定 reload/recovery/correction
  版本，私有 writer 在 CAS 前拒绝版本切换或冲突。引用进入 expected-document hash，空值省略且不改变旧 hash/序列化。
  该引用仅选择 LLM owner 的纯 rendering；manifest/wire、预算、attempt、dispatch 和全部 SM-2 门禁不变。
  SM-1 生命周期与 SM-4 恢复/交付不新增迁移，也不能通过部署升级旧请求或恢复失败 Run。
- semantic ExpectedDocument 的 fresh intent 使用 `semantic-render/cache-layout/v4`；同 logical request 的 reload/recovery
  和同 generation、batch、authority、manifest 的 attempt1 继承 attempt0 renderer（包括旧 nil/空引用）。来源 ID 由同一
  `SemanticRequestIdentityHash` 计算，不扫描其它 generation 的成功结果；匹配 ID 但 document kind/transaction/manifest
  不一致或同 lineage 版本冲突均拒绝。请求 retirement 后的新 generation 可选择新 renderer，不能修改旧 session。
  v4 在 LLM owner 内仅重排当前 C3、复用等价 prompt schema 子树和引用精确重复的 Focus；owner 完整输入不删除，
  native schema/local decoder/manifest/wire/预算不变。SM-3 hash 绑定版本，SM-4 按原版本重放；SM-1/SM-2 无新增迁移、
  credit 或执行授权。`semantic_cache_renderer_test.go` 与 `runtime_operations_renderer_test.go` 覆盖版本、reload、correction、
  旧 nil session、foreign scope 和版本冲突；缓存命中不能替代运行及终态验收。
- `runtime-candidate-parameter-cardinality/v1` 仅处理完整原文中重复的已声明 provider 参数。strict decoder 保留原首错；
  完整 bounded 文档的 bytes/SHA256、每 operation exact wire identity、scope hash、全部参数 name/type/indexes 和单次值 hash
  成为 SM-3 纯事实。未知字段/类型、动态参数、缺 slot/required、重复 slot、歧义 JSON key 或超界不能产生部分纠错事实。
  `consolidate_parameter_entries` 只打开既有 attempt1：保持操作顺序/数量、slot/kind/variant、target/binding、完整参数集合，
  只有重复 name 的值可重新生成；非重复值必须精确保持。Core 不解释文本编辑、不去重/选择保留值、不修改原文。
  完整后继仍须通过 manifest、owner conformance/feasibility、preimage 和 seal；零局部派发、零终验 credit。
  fact 参与 observation/witness/session hash、raw clone 与 fresh-load；nil 扩展保持既有失败 archive identity。
  CAS 失败/响应丢失/重启不增开修订身份；attempt1 再失败仍 `candidate_retry_exhausted`，预算与四状态机边界不变。
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
  `consumeSemanticRepairFreshValidationReceipt` 在原 ordinary validation receipt/evidence CAS 内反查唯一
  manifest、closure、当前 binding、completed execution 和原 receipt key/fence/hash，并独立重编旧 repair handoff。
  accepted/rejected 均消费准确续接；只清旧 directive，尚未采样的当前依赖仍阻塞 credit/assessment，不能恢复旧 credit。
  `semantic_repair_receipt_consumption_test.go` 复用原 sealed repair fixture，覆盖来源缺失／漂移零变更、非匹配
  receipt 零消费、完整 CAS 前失败／后响应丢失、JSON 重载及重复零写；真实 owner 回归另校准正常恢复。

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

The SM-2 semantic fresh-validation guard requires the exact canonical
`validation-failure-repair/<focus>/<attempt>/fresh-validation` identity. A settled semantic reopen lineage can remain
after a later same-binding confirmed repair; it does not make that newer confirmed handoff a semantic continuation.
SM-3 receipt consumption must use its own existing lineage proof, and SM-4 reload/replay retains that distinction.
The checks do not add a transition, forgive a missing receipt, change the original criteria or grant credit; SM-1 is unchanged.

On an aggregate-backed SM-2 reduction, an exact confirmed-repair fresh-validation directive is retired only after
reverse-proving its completed repair transaction and the existing accepted-validation applicability consumption.
This also applies while another prerequisite remains pending. SM-3 preserves the original dispatch/receipts, and
SM-4 reload cannot resurrect the consumed handoff. A historical consumption does not settle fresh sampling debt,
restore stale CWI credit, clear a different directive or forgive missing/tampered observations.

SM-3 request context may carry the optional `runtime/validation-generation-context/v1` capability-owner observation and
exact source-check mapping. The catalog snapshot covers all observed bytes, identity and completeness outcome; LLM C3 projects
only current preparation/batch members, once per surface. Core does not select project languages or files. This is
request context only: no new SM-1 transition, SM-2 credit/retry, SM-3 write surface or SM-4 recovery authority is introduced.
In particular a conflict report still follows the failure boundary below; generation context does not authorize input
replacement or turn a rejected review into a validation execution failure. Historical raw clones detach the new fields.

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
`InputReview`，封存 activation 选择的 `validation/input-consistency/v1|v2|v3`、完整输入字节/hash、原 required checks/native mapping；LLM
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

v3 是显式 opt-in 的[组合证明合同](../../../../docs/00-core/runtime/validation-proof-composition.md)：
Definition requirement、owner subject、catalog、semantic manifest 与报告完整绑定同 CWI 原 lineage declarations。
每个原准则／来源检查的 obligation 映射必须保留 ordinary `validation_input`，并完整引用该 CWI 的封存规则；
自洽重算 hash 不能替代原 Definition 来源反证。SM-3 仍持久化有界完整报告，SM-2 仍必须先一致性／feasibility
再 seal，并在 assessment 将实际普通证明与 execution guards 合取。未来 delivery 声明不是现存 ACK，
SM-4 只在原 immutable seal 与真实有序三 ACK 后消费；SM-1 不变，review/ticket 零 credit。
v3 与 v2 共用 Run 内原 CWI/check 信息额度，换输入、规则映射、路径或重启不刷新预算；v1/v2 禁止组合扩展。
`runtime_validation_proof_composition_test.go` 固定完整身份、克隆／重载、协调重哈希来源替换拒绝及 legacy-null
wire 反例；`biz/runtime_proof_composition_mainline_test.go` 使用真实 File/Shell/Run/Result owners，覆盖一致、冲突、
遗漏、信息重评、评审／binding／receipt 与三方 delivery 响应丢失恢复、重复零执行及独立 delivery consumer。
该测试采用 private startup 与 scripted reviewer，不代表公开 fullflow 或真实 provider 已验收，不开放 availability。

v3 的可选 `validation/diagnostic-source-review/v1` 进一步封存 validation-owner 完整有界的 opaque native 来源与准确
输入，SM-3 原报告事务必须独立覆盖每个来源；SM-2 的一致性分类同时消费普通原检查与逐源 findings，不能以其它
consistent check 隐藏邻接 conflict/unknown。该扩展的未知信息与原 Run/CWI 共享一次既有信息机会，换来源身份、
输入或重启不刷新；已知 source conflict 不能标为纯 coverage omission。Core 不解析 native grammar 或业务测试，
真实 behavior attribution 仍属 capability owner，repair confirmation/effect 仍沿原 exact route/receipt fence。
逐源 execution stage 的原 source ref/behavior targets 进入完整 receipt/fact identity 和 detached reload，不授终验 credit。
rejected receipt writer 与 fresh-load 通过同一中立门禁反查原 dispatch/catalog 的准确来源库存、protocol/stages 和 owner
scope，并核对独立 behavior union；时间顺序、重复来源和版本漂移均拒绝，原输出保存缺失严格拒绝修复授权。旧 noncomplete stage
history 不自动升级。原 binding 选择门禁，删除逐源字段、整个合取或 union 不得保留 structured failure 的修复授权；
failure candidate 必须与逐源归属 union 完全一致。`operation_receipt_diagnostic_source_test.go` 固定 ingress／重载同一反查门禁。
普通执行在后续 deadline/storage 失败前观察到的 behavior 只可保留为 rejected 历史，无逐源归属不得授 structured
repair；多失败源的部分归属保留真实旧阶段及精确 union，但后一来源未知时仍零修复／验收授权，不借用旧行为补缺。
真实归属后的原文发布失败也保留逐源历史、原输出 bytes/hash、已保存引用和明确 omission；structured failure
消费者独立要求所有原阶段 stdout/stderr 均已保存，不能从缺失原文的历史取得 repair authority。
`runtime_project_multi_failure_owner_mainline_test.go` 使用真实 File/Shell/Run CAS、原输出读取和
owner 重建，覆盖逐源评审／binding／receipt 响应丢失与未观察到实现的相邻失败；provider 仍为脚本替身。
SM-1 不变，SM-2/3/4 的 writer/CAS、预算、recovery 和 delivery 不新增转换；默认 availability 保持原门禁。

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
  `TestDriveRuntimeMappedHistoricalBehaviorRepairsAcceptanceAcrossRestart` 以真实 File/Shell/Run 和 scripted provider 验证
  已声明历史行为／普通验收来源的真实失败→诊断→实现修复→fresh validation→CWI/Result/Run 关闭。
  原 state-change-only 前驱输入由 `TestDriveRuntimeAssertionFeedbackCannotInventPredecessorRevalidationAcrossRestart`
  验证合法后继修复后，缺少普通前驱重验来源明确失败；原行为与终态 CAS 前后恢复不复活旧 credit、重复不增加预算。
  两者均不是真实模型求解验收。
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

- prospective File delta/required-owner satisfaction 的纯 proposal 校验只规范化实际读取的 scope/hash 和 dependency
  catalog 投影，不再为此复制无关 Coverage、义务及普通 owner catalog。原读取字段的规范化次数、value.Validate、
  exact proposal/File delta/catalog 身份、stale 和完整 feasibility/manifest gate 均保留；不新增持久校验位或跨调用缓存。
  `runtime_prospective_manifest_projection_test.go` 使用优化前完整规范化 oracle 对照首个错误、raw/重复/空值、
  accepted/stale、JSON 重载、原值保全、后续变化、返回隔离、fuzz 和分配；此片不改变 SM-1～SM-4 transition 或预算。

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
  `biz/runtime_project_conjunction_owner_mainline_test.go` 配对真实 Shell 四阶段/原工程失败与文件型 Run CAS，
  在 SM-3 `owner_committed` 原 receipt 已保存但 SM-2 evidence/credit 尚未形成时丢失响应，重建 Shell 和驱动对象后
  沿原 receipt 续驱；实际工程执行计数、原 key/fence/receipt/raw identity、单次 operation 分配/消费、recovery ledger
  和既有 confirmed/unknown 消费事实均独立核验。该回归不改变 SM-1～SM-4 transition、预算或 availability；provider
  是脚本替身且未重建 Hub/进程，不能替代公开 authoring、fresh-process 或 live 验收。
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

版本化终态材料消费者见 [`terminal-material-proof.md`](../../../../docs/00-core/runtime/terminal-material-proof.md)。
`terminal_result_evidence_index` 反证原 DoD/source 逐项、current proof 与正式 Result 索引；
`terminal_owner_operation_receipt` 只按 exact original source 反证原 operation/receipt/evidence 链，不冒最终 Git bytes。
SM-4 原 material-seal event 在首次材料 CAS 前校验成功材料，fresh-load 重验同一原值；之后仍只消费真实有序 ACK。
V2 `terminal_owner_snapshot` 只允许 bounded typed owner observations，不允许 caller material；原 receipt 封存 snapshot/root
identity，SM-4 adapter 取得 Git V3 实际 clean/head/graph 与 validation/input byte membership，再由 private reducer 独立
反证 exact sources/current binding/revision 和完整原依赖。没有 snapshot rule 仍为 scope-only；seal 后不再 requery Git，
恢复只重放原材料和三 owner ACK。具体语法在 Git adapter，Core 不解释 Git/工程行为；此证明不等于全 owner currentness。
失败／取消／替代终态如实交付，其成功材料义务为 `unsatisfied`，不能冒已满足或阻止已完成的失败交付退出。

SM-4 唯一终态材料 compiler 规范化权威投影时必须另外保留原 `Facts.Failure.OriginalDiagnostic`，不得把比较用规范化副本写入不可变材料。semantic violation 的原序列、空字段及原文字节由 raw clone 保全；JSON 重载和交付重试从同一原来源重编，不能因规范化产生的空集合与持久 wire 不一致而阻断失败交付。材料来源／hash／前像比较门禁不放宽，后续 recovery 仍使用 seal 中的原 generation 与 recovery 前像。
无新账本、预算、state machine 或 topic；SM-1 不变，SM-2/SM-3 只保留原声明与义务映射。
`biz/runtime_terminal_proof_mainline_test.go` 验证真实 owner、原材料/回执、八个 CAS／响应丢失／重启切点、duplicate、
零额外 provider 和来源篡改；private startup/scripted reviewer 不等于公开 fullflow、Git snapshot 或 live。

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
- 可选 `RepairHistoryContractRef=runtime/confirmed-binding-repair-proof/v1`（Profile 配置
  `repairHistoryContractRef`）显式消费同 ordinary binding 的 existing/exact confirmed repair，不要求或伪造
  prospective retirement/reopen。反查原 rejected validation、完整 commitment/accepted admission、sealed Profile
  route、唯一 owner confirmation、原 closure 与全部 evidence-committed accepted durable mutation receipts/fences；
  actual target 必须在确认的有限 scope 内，不把 scope 的每个候选都强制改写。原 mutation fact 的 CWI credit
  必须为空，并保留其原 ledger；accepted owner 行为不等于 accepted CWI proof。rejected validation 的 unknown
  effect 只可作为非 durable 历史诊断来源，不能证明 effect 或授权重发。fresh ordinary validation 必须严格更晚，
  同 binding/closure、完整原 independently-reviewed subject 与 all-of proof；截止点内后来同 CWI 或 exact binding
  失败阻塞，即使原 rejected fact 的 CWI credit 为空；截止点后的事实不补证或改判旧 assessment。
  缺失、duplicate、hash/fence/source 漂移、预算非法均拒绝。空扩展保留原 JSON/hash 与 retired/reopen 消费者。
  这是 SM-2 原 assessment 附加 AND、SM-3 原 receipt/admission 反查与 SM-4 原 aggregate 重载的纯消费者；
  不新增转换、账本、预算、effect 或 credit，SM-1 不变。
  ordinary existing-owner `TargetRefs` 是有限授权范围，不是逐目标全执行要求；每个 fresh validation receipt 仍须
  匹配 exact binding、operation、recipe/native all-of 与原确认 closure。只对显式 `ProspectiveTargetRefs`
  要求完整逐目标成员，空验收集合仍拒绝；不能把目录中的其它文件强加为不存在的验收 operation。
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
  分别保存，Result owner 不重判验收。新 aggregate 初始化时一次封存 `ResultFeedbackVersion=5`；以当前 SM-2
  coverage 判断历史 satisfied assessment 的适用性，当前未满足时显示 indeterminate，不删除历史评估或证据。
  v3 的 input-review terminal diagnostic 另保留完整 blocking batch 的 typed reports（CWI、requirement、subject、review ref/hash、
  逐 check/obligation 事实），并从原 persisted document/binding 反证；删除或篡改反馈不得在 fresh-load 被接受。
  原始报告、诊断文本与控制字符按原值保全，不为脱敏改写；受限读取边界和可逆展示编码保护原文。Result、Web 和 TUI
  仅展示，不能据此选择 action；完整详情不能只剩被裁剪的通用摘要。
  当前只允许版本 5；缺失、旧版或未知版本均在 Run record、aggregate、initialize 和 Result material 入口 fail closed，
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

Profile 的独立 `RuntimeImplementationWorkPolicy`（`runtime-implementation-work/v1`）声明有界的 owner、operation、selector、轮次、fresh observation/validation 与输入保护，进入 Profile 身份和 exact CWI 的不可变 Definition 投影。声明本身不授予 effect；`RuntimeExecutionContract.ValidateExecutableAdmission` 与 Workspace promotion 的 availability gate 只接受已接线的 File `file/workspace_deliverable`、无固定 candidate refs 的 write/edit 组合及其 exact prospective 约束，其他组合拒绝。纯 `Validate` 不代表可执行准入，静态 availability 也不代表公开 fullflow/readiness 已通过。

工作事务复用 SM-2/3/4 和同一个 Runtime aggregate，SM-1 不新增迁移。SM-2 私有 scope 事件先 CAS 封存只读 `ScopePreview` 需求，再接收真实 owner 观察；独立来源由 sealed Profile、Definition seed、exact CWI 与当前 manifest 前像重新编译，不能从待验证请求反向构造。File 适配层保留完整有界 inventory、unsupported 负事实和 typed stale；单轮目标上限不能裁剪观察。scope 不写入终验 `Resources.TargetSelection`，也不授 variant、dispatch 或 credit。

SM-4 的下一 scope 只读需求投影必须先检查 SM-2 当前规划资格。保留的 focus 在前驱精确 reopened、bound 或 settled 时不产生新需求，也不因该暂时窗口封存内部语义失败；显式 scope reserve／owner observation 仍独立反查同一资格，不能绕过前驱结算。该 query 零 CAS、零 owner I/O、零轮次或预算变化。

原 accepted semantic `coverage_order` 提议 `implementation_effect` 或 `terminal_proof`。SM-2 消费该文档并原子退休原请求 generation：effect 才 reserve 原 CWI 的有界 round，terminal 只恢复原完整终验前置。工作 purpose 必须取得 manifest、provider ceiling、capability、capability state 和全部已声明 owner catalogs；仅延后当前 focus 的终验 target/scratch，working binding 点名的 exact snapshot 仍须取得。sealed capability inventory 独立反查 owner 输入保护 catalog，不能通过协调删除 Pending/Satisfied/catalog 缩小 exclusions。普通无工作策略路径保持原前置合同。

SM-3 复用完整 semantic proof、owner feasibility、request session、candidate、sealed dispatch 与 execution。只有原 accepted proof batch 和 matching owner facts 能经 SM-2 私有事务封存 `RuntimeImplementationWorkingBinding`；它与 `CoverageEvidenceBinding` 分离，完整 policy、scope/catalog 前像、输入排除、原 semantic lineage 和前轮 collective receipt 身份进入 working commitment、manifest 与 dispatch。每轮独立请求/dispatch，不能借用 owner-fact acquisition 或 confirmed validation repair 权限。

`OperationReceiptObserved` 的一次 aggregate CAS 同时提交 SM-3 execution/receipt/budget、SM-2 working effect/round 和 SM-4 必要 retirement。完整 collective receipts 齐全并反证资源失效后才结轮；工作 effect 保留 owner 原 quality，零 validation/terminal/CWI credit。后续轮须 fresh owner observation 和新的 accepted semantic proposal/feasibility，预算不重置；逻辑 durable dispatch 使用原 sealed working CWI 和 accepted attempt 来源计数，归档不退款。进入 terminal purpose 后恢复原 all-of 终验、输入审查和 exact repair 合同。

新 working binding 的 `ReceiptProtocolRef=runtime-implementation-work/receipt-chain/v1` 进入 binding/round/dispatch 身份；空扩展保留已封存历史的单回执判断，不升级、重算或复活旧失败。逐项 receipt consumer、collective validator 与 fresh-load 使用同一纯版本链门禁：起点为原 owner catalog dependency，按 sealed candidate operation 顺序反查同一原 round/dispatch 的 execution key/fence、原 receipt hash 与已提交的无终验 credit evidence，再以 exact owner/domain/subject 的 before→after 推进本次局部游标。后继 before 必须等于已证明前驱 after；遗漏、重复、断链、分叉、循环、跨作用域、未知 outcome 或未提交前驱不得推进。治理 hop 仍须原 grant 和原 dispatch lineage，不能借用其它 dispatch 回执。批中 CAS 将新 evidence 按既有 ID 顺序保存，不改写或去重原事实；游标不持久化、不跨事务缓存、不代替 freshness 或授予后继执行权限。`runtime_implementation_work_receipt_chain_test.go`、四目标真实 File driver、`runtime_implementation_work_chain_guards_test.go` 及批中退出的独立进程回归覆盖完整集、部分集、重复、隔离、预算与恢复；不代表公开 fullflow/live 已通过。

rejected、stale、missing invalidation 与 unknown 关闭原工作权限，不通过新轮重发；governance continuation 保留同轮原 execution key，新的 fence/dispatch hop 必须与原 grant/receipt lineage 匹配。无 receipt 的原 started claim 进入 unknown 后，只允许 exact original key/fence 的迟到 owner receipt 入账与资源失效同 CAS；不能重开 round、补 terminal credit 或将原 physical unknown consumption 改成 confirmed。fresh-load 同时反证原 unknown allocation/consumption，不能从文件存在、provider 文本或重写 hash 推断 effect 成功。

合同测试包括 `runtime_implementation_work_policy_test.go`、`runtime_definition_implementation_work_test.go`、`runtime_implementation_work_ledger_test.go`、`runtime_implementation_work_receipt_hop_test.go` 与原各 owner 准入反例。`biz/runtime/implementation_work_*_test.go` 覆盖 private transition、完整写集、duplicate/stale、来源篡改、CAS 前后回复丢失和预算；unknown receipt 的独立测试进程使用 test-owned repository，不是公开 Run-owner 全链恢复。
`biz/runtime_implementation_work_scope_test.go` 校验真实 File 只读 scope、完整 inventory 与 revision fence；`biz/runtime_implementation_work_two_round_drive_test.go` 在 private startup fixture 后使用 production outer driver 和真实 File owner 完成同 CWI 两轮，不能替代公开 authoring/activation、终验、Git、正式 Result 和全部 delivery ack 的完整工程验收。availability 不因这些局部证据自动开放。
`biz/runtime_implementation_work_process_recovery_test.go` 使用两个独立测试进程、各自新 Hub/Run/File/Shell owner 和真实 file-backed Run opaque CAS；首进程在真实工作 receipt 的完整 CAS 成功后、向 File 返回回执前退出。该 CAS 已同时结轮、归档 dispatch、退休资源并提交预算和零 terminal credit 的 effect；恢复不能假定仍有 active dispatch。新进程向 fresh File owner 重投原 sealed operation，必须读回 exact key/fence/receipt，完整 aggregate、effect 与预算均不变，再沿原 CWI 开启 fresh scope 的第二轮；原 round/dispatch、startup、consumption/allocation 和 recovery 身份保持不变。父进程从隔离的原 bytes 前像及最终 Run 文件独立核对，避免同路径 repository 进程缓存遮蔽 child 写入。此测试仍采用 private startup 与 scripted provider，只证明 SM-2/3/4 原工作事务的响应丢失恢复，SM-1 不新增迁移；不代表公开 admission、真实 provider、终验或交付已验收。

`make verify-runtime-implementation-work-readiness` 对每个目标包拒绝零测试选择，并包含公开 `internal/integration/implementation_work_fullflow_test.go` 完整正例及 unsupported 反例。公开正例的 failure 使 readiness 和依赖它的 CI 验收失败，不能作为 expected failure、skip 或 bounded-terminal 成功。Git 交付必须将 sealed execution 的 exact key/fence/receipt hash 与实际 commit 对象、最终 HEAD、完整 tree/blob bytes、已评审输入 hash 和正式 Result 的同一 evidence ref 联结；仅 `evidence_committed` 状态、`.git` 存在或 commit 文本不构成该证明。
Git commit owner 的原始对象归档通过既有 `ContentAddress` 与 actual `OwnerRevisionSet` 进入原 receipt/evidence 事务。
`runtime_git_commit_snapshot_test.go` 在真实 File/Shell/Git/Run/Result 主链中独立读回受限 owner 原文，与 Git
`ls-tree/cat-file` 对照全部递归 tree/blob、binary bytes、scope/operation/authority hash；同一原 execution 的 receipt hash、
EvidenceFact hash、材料 seal、正式 Result 的引用及三 ACK 必须齐全。该读取只属于 test oracle，不是 production 跨 owner
文件读取入口；提交时 HEAD 双观察不替代终态 currentness、已评审输入 hash 或最终快照 typed consumer。

`runtime_git_commit_snapshot_event_adapter.go` 仅从完整合法 aggregate 重编原 `ReadSource`、operation、receipt hash、
content address 与 actual object dependencies，经 Git-owner `QueryRuntimeCommitSnapshot` typed topic 读取。
Git 独立反查原 receipt/transaction/execution，Workspace owner 提供 actual directories，raw archive 不跨 owner。
same HEAD 必须完整重读原 commit/tree/blob 且 HEAD 前后相同；changed HEAD 返回实际身份与 nil current set，
archive/object corruption、budget/cancellation 或错 scope 明确失败。adapter 校验 exact protocol/request/state revision/
receipt/address、完整 bounded response 并返回 detached 集合；查询不写 aggregate、material、credit 或 budget。
显式 v2 查询可核对 input-byte membership。`CompileRuntimeValidationInputSnapshotSource` 先从完整合法 aggregate 反查
exact evidence-committed ordinary validation、原 receipt、唯一 accepted fact 与 current binding；
`CompileRuntimeBindingValidationInputReview` 再只消费产生该 exact binding 的唯一 accepted semantic batch、owner catalog、
Definition proof purpose、完整 consistent report 和 subject。返回 detached complete input，不扫描无关历史报告或复用新 catalog。
每个 subject file 还必须在原 validation owner 的实际读取集合中唯一匹配 exact workspace-target/path/content hash；
另一 owner/domain/path 的相同内容或自洽重算 set hash 不能补证。Core 只比较 typed resource/bytes identity，不解释文件语法。
本合同有界支持单 operation/single binding subject；未声明评审或缺失实际读取依赖的旧来源明确拒绝，不回填。
`queryRuntimeGitCommitSnapshotWithReviewedInputsFromAggregate` 将同一 aggregate 的两份原来源经 v2 typed query 发送给 Git，
由 Git 对原 tree/blob 的 exact paths 与完整 bytes 进行独立 membership 核对；Core 不解析 Git paths／blob grammar。
响应校验完整 subject/path/hash/blob、原依赖成员与 HEAD fence，原 membership 在 changed HEAD 时仍不授 current proof。

该 SM-4 participant 的只读接线不新增 SM-1～SM-4 transition，不把 immutable object freshness 或原 input membership 扩为
工作树/index clean、全 owner dependency union 或已封存终态材料证明，也不开放 public availability。
真实 File/Shell/Git/Run/Result fixture 独立校准 production sender→Git→原 receipt handler 的完整来源、v2 技术字节联结、duplicate、
后来真实 HEAD drift 与零 aggregate mutation；无评审来源不能由 helper 自动提升为 reviewed proof。
`runtime_validation_input_snapshot_test.go` 另以真实 File/Shell/Run 评审主链固定 accepted batch→原 validation→current binding
来源、JSON reload、返回隔离、wrong receipt/dispatch/operation、替换 binding／缺 current/ref/report 拒绝和重启零额外 provider。
`biz/runtime/validation_input_snapshot_source_test.go` 固定 original actual read set 的 owner/domain/path/bytes/Definition、
缺失／歧义与自洽重哈希反例；此逐输入联结不是 validation 子进程的完整读取追踪或全 owner union。
这些配对尚不是 reviewed-source→Git→terminal consumer 的完整组合主线；private startup/scripted provider 不等于公开 fullflow 或 live。
公开脚本 provider 的 input review 必须逐 `sourceObligationChecks.checkRef` 使用该来源的原 criterion/declaration；
非来源检查才使用共同 CWI 判据。`implementation_work_fullflow_review_test.go` 固定 A/B 独立、缺项/重复拒绝及
owner-fact declaration 不得被工程断言替代；该 provider 守卫不提供 terminal proof consumer 或可执行 availability。

同一个 `CoverageEvidenceBinding` 的 proof 按 evidence kind 唯一，不能用多个 validation recipe proof 冒未支持的 build/test/race 合取。validation observation 必须匹配 exact recipe、protocol 与 native-check attestation；另一 recipe 的 passed checks 不授予本 proof credit。复合执行须由 capability owner 实际生产并持久化完整事实；命令名称、检查标签、验证输入内子命令或 README 自述不替代 sealed owner execution 的证明。

| 验收面 | 自动化证据 | 必需维度 |
| --- | --- | --- |
| ordinary validation-input protection | `workorchruntimecontract/runtime_validation_input_protection_test.go`、`biz/runtime_validation_input_addition_mainline_test.go::TestDriveRuntimeOrdinaryMutationCannotWeakenValidationInputAcrossRestart`、`TestDriveRuntimeRepeatedInputWeakeningClosesWithoutEffectOrTicket` | 三策略及 selector-only 排除、source preimage/hash/clone/replay、exact repair 例外；真实 File/Shell + file-backed Run + scripted provider 下，越界拒绝→合法源码修改→独立补充→fresh validation→Result/Workspace/Run 完成；source dispatch/input receipt/拒绝终态切点重启零额外 provider，重复越界零 effect/credit/ticket，不宣称真实模型或 release DCE 验收 |
| validation-input review / factual ticket | `biz/runtime_validation_input_review_mainline_test.go`、`workorchruntimecontract/runtime_validation_input_review_test.go` | SM-2/3/4 共用旧事务；一致→实际验证，冲突/格式错误零 ticket，无法判断→Decision 信息→新请求重新评审；同一 subject 信息上限、reject、exact duplicate、内容/lineage tamper、五个持久化切点恢复与零额外 provider；SM-1 仍由 Run owner 交付 |
| v2 obligation review / full feedback | `workorchruntimecontract/runtime_validation_input_obligations_test.go`、`runtime_validation_input_review_feedback_test.go`、`biz/runtime/validation_input_review_test.go`、`biz/runtime_validation_input_review_mainline_test.go` | 完整原文映射及 Unicode/容量边界、缺项与矛盾分离、Run 内原 CWI/check 信息额度、协议降级拒绝；真实 owner 原输入不变、零虚构 receipt/credit、终态逐项原文反馈及可逆展示、删除/篡改反证、重复和重启幂等 |
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

### 阶段内验证输入修订 v1

封存 Profile 可用 `ImplementationWorkPolicy.InputRevisionProtocolRef=validation/input-revision/v1` 声明阶段修订。
SM-2 的 exact complete input-review conflict 私有事件保留旧 generation，以 `input_review_revision` 回到 required；
唯一原 CWI、同一 input resource owner、冲突 findings/owner protected targets/实际读取输入三者交集才可进入既有普通工作。
新 `RuntimeValidationInputRevision` 保留完整原 binding/input/report，随 scope→semantic work→working binding→candidate manifest
→dispatch 封存；fresh load 反查原拒绝 generation 与 request/session，历史消费不能前向引用。旧 terminal purpose 留存，
待修订报告只允许真实 effect purpose，不允许 unchanged re-review 或借用 post-execution repair。未 opt-in 继续原 conflict 失败。

File opt-in edit feasibility 预览同一 edit，核对原输入 before hash、变化后像；候选/最终 seal/dispatch reload 重核。
仍使用原 MaxRoundsPerCWI、history/provider/capability budget，真实 accepted receipt 零 CWI/validation credit；
资源失效后取得真实当前输入，再独立审查、执行全部原 validation 和终态交付。未知、stale、预算不足或混合/歧义来源不放行。
Core 不解释语言/框架/项目语义；具体改法来自原 Profile/project 和完整审查反馈。SM-1/4 不获新决策权。
验证：`TestInputRevision*`、`TestDriveRuntimeInputReviewRevision*`，后者用真实 File/Shell/Run/Result/Workspace owner，
仅脚本化 provider，覆盖真实修改→fresh review→validation→ACK 与两处 restart；不等同收费 live 或十分钟公开 fullflow。

### File 原结果交付与内容绑定的工作语义上下文

SM-3 原 sealed dispatch/executionKey/fence 和 Run CAS 仍是 receipt 接纳唯一权威。SM-4 在把已 claimed 的 File 变更判为
unknown 前，可通过 File-owned `/file/runtime-operation/deliver-mutation-result` 请求交付已保存的完整原 receipt；
`CompileOperationResultDeliveryRequest` 只是原 claim 的只读投影，不是 physical call token。File 技术 guard、结果文件、
Found 或普通 response 均不授 effect/evidence/完成；驱动必须重读 Run owner。原回执不存在、确定拒绝或有界交付耗尽仍执行
原 unknown/预算门禁。工作 unknown 的同一失败 CAS 保留原 owner diagnostic（阶段、受限引用、bytes/hash），终态投影不得
将它替换为推测底层原因。SM-1 不变、不复活 failed；SM-2 仅消费最终可信事实。

工作 scope 新增可选版本化 `implementation-work-content/v1` exact 原文观察。生产 File adapter 在完整 scope target set
中读取有界完整正文，并再次真实读 manifest 核对原 revision；容量 128 KiB、缺失/二进制/截断或漂移显式拒绝。
完整内容随原 scope private event/CAS 冻结，不借修订/重启重读另一个输入。旧封存 scope 不原地升级。
`SemanticWorkContext` 从原同 CWI 完整 round、sealed dispatch/execution、可信 receipt/version 边和历史原用途理由编译 priorRounds，
不受通用 16 receipt history suffix 遗漏影响，不另设完成账本。真实内容及 priorRounds 只进入 C4；C0-C2 和稳定前缀不变。

内容绑定请求封存 `semantic-render/owner-work-content/v3`，用途文档只增加最小 typed `implementationIntent`：原 declarationRef、
exact contentHash、remainingWork、reason。schema、strict decoder、document hash/clone、compiler 与 owner admission 一致。
代码只检查身份、来源和有限形状；模型的理由不构成语义充分性、readiness、validation 或 CWI credit。
原 Goal/DoD、Profile、protected inputs、8 轮/4 目标与普通独立验证均保持；业务差异继续在 Profile 和项目上下文。
必要修复仍走既有 revision/repair/fresh validation，不强制切换终验、不把重复抽样直至 PASS 当成稳定性。
