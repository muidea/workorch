# WorkOrch Unified Runtime Design Contract

Status: normative

Type: core-contract-index

Last Verified: 2026-09-10

## Purpose

本文是 WorkOrch Runtime 语义的唯一总入口。详细规则按单一专题拆分；下列子合同共同构成规范，不是可任选的参考资料。

## Contract Set

| 合同域 | 规范入口 | 回答的问题 |
| --- | --- | --- |
| Runtime semantics | [Runtime Topics](runtime/overview.md) | Owner、收敛、candidate transaction、四状态机与全局门禁 |
| Definition convergence | [Definition Convergence](definition-convergence/overview.md) | Goal/DoD all-of admission、语义图、调度、clarification 与上下文 |
| Request/cache | [Runtime Request Cache](runtime-request-cache-note.md) | C0～C5、provider ceiling、rendering 与 audit |

Runtime 行为变更必须先读取总入口，再从 Runtime Topics 进入受影响专题；不得只引用本页摘要替代子合同。

## Authority And Ownership

- SM-1 只属于 Run owner：Run lifecycle、continuation 与 administrative termination intent。`run.replacement` 是同一
  Run owner 在 SM-1 下的 subordinate transaction，不是第五个状态机或第二个 lifecycle owner。
- SM-2、SM-3、SM-4 只属于 Orchestrator：authority/focus convergence、candidate/sealed dispatch、drive/recovery/delivery；
  SM-4 的 delivery handoff record 不授予 Orchestrator 任何 SM-1 transition authority。
- Workspace、Result、Decision、Approval、Memory 与 capability owners 只修改自己的正式状态。
- Application 构造 typed bootstrap、提交 Start/Resume/Restart intent、提供协议与 projection；它不拥有 Runtime drive。
- 跨 owner I/O 只使用 EventHub 与接收 owner 定义的 concrete typed command/data/result；禁止 service/repository 直连。
- Runtime topic 标注某个 `StateMachine` 只表示参与该机器；`ReducerOwner + MachineRole` 必须分别固定唯一 reducer
  owner 与 participant/owner ingress/owner observation。EventHub envelope 从不直接调用 reducer，只有 owner-private
  internal event 可以选择 transition；Run 的 opaque aggregate CAS 也不获得 SM-2～SM-4 authority。

## Production Mainline

```text
Workspace owner seals admitted startup input
  -> Application builds typed RuntimeBootstrap
  -> Run owner seals SM-1 activation
  -> Orchestrator SM-4 drives persisted aggregate
  -> SM-2 seals authority, semantic contract, ready focus and evidence binding
  -> SM-3 persists/evaluates candidate, obtains manifest-declared static conformance and dynamic applicability facts, then seals dispatch
  -> capability owner returns factual result/receipt
  -> SM-2 credits evidence, settles/reopens CWI or seals terminal
  -> SM-4 CAS-seals immutable terminal material before owner I/O, then delivers Result -> Workspace -> Run
  -> Result acknowledgement relays the exact owner-derived ResultID and persisted ResultCloseout
```

administrative cancellation/replacement 使用独立但同方向的交接链：

```text
Run SM-1 administrative termination intent
  -> Orchestrator SM-4 CAS-persists RuntimeTerminationRecord as delivery handoff
  -> SM-4 delivers Result -> Workspace -> Run
  -> Run private SM-1 terminal-delivery event applies cancelled/superseded
```

显式 Restart 复用同一个 Run-owner `run.replacement` transaction。若 predecessor 尚未终态，先走上述交接链；若
Workspace 已没有 non-terminal Run，则 Run owner 按 `WorkspaceRunGeneration` 选择最新 terminal predecessor，验证其 exact
terminal delivery 与 owner-derived `TerminalResultID`，直接把 replacement 私有 stage 封存在 successor materialization，
不得重开 terminal、重投 terminal delivery 或创建第二套状态机。successor 必须同时封存 `PredecessorRunID +
PredecessorResultID`；predecessor 的 `SuccessorRunID/LineageState=superseded` 必须先由 Run-private CAS 落地，才允许激活
successor。

Recovery is idempotent re-drive from the Run-owned opaque aggregate. Projection、Eventstream、provider text、process-local cache
和历史 event sequence 都不能重建或替代 authority。

验证输入一致性评审复用上述四机：SM-3 持久化 owner-declared exact input 的完整 LLM 逐项报告，SM-2 在 feasibility/binding
前区分一致、冲突和真实信息不足；只有后者封存 typed governance dependency，由 Decision owner 管理 ticket。
信息 resolution 仅使 SM-2 留存旧 semantic generation 并发起新请求重新评审，SM-4 交付 continuation，Run 独占 SM-1。
原 Goal/DoD、startup、旧输入和报告不得改写，ticket/review 零 credit；格式/身份错误仍走旧 request/correction budget。
细则与证据见 [Candidate and Dispatch](runtime/candidate-and-dispatch.md) 和 Orchestrator implementation SoT。

validation failure repair 也必须保持 core owner-neutral：sealed Profile 的 exact
`(trigger owner, operation kind, diagnostic class, evidence target type, owner code)` route，或 source binding 的 operation
descriptor，先确定唯一 resource/effect owner；该 route 的全部 repair operation 还必须在 pre-seal 共同声明一个可执行的
typed confirmation protocol，否则 Definition admission 直接阻塞。validation-owner observation 的 diagnostic hash 必须绑定上述
route 字段和 sealed validation scope；该 scope 只能取自 matching operation execution 所绑定、随 dispatch 封存的完整
validation-owner admissibility catalog，不能用 per-operation entry target authorization 或 current manifest 替代。receipt admission
必须验证 diagnostic operation kind exact-match sealed operation kind；
有 observation 且 route 匹配时才可发起 owner confirmation，无 exact route 时必须在
rejected receipt 的同一 Orchestrator-private CAS 封存 `validation_failure_repair_route_unavailable`。confirmation、candidate
operations 与 receipts 必须逐项匹配该 owner；confirmed repair directive 的 operation kinds 必须精确等于 route 声明，不能从
同 owner 的宽泛 capability/binding 集合扩张。`existing_owner_scope` route 必须先取得 structured validation observation；缺失时
同一 receipt CAS 直接封存 `repair_surface_unavailable`，禁止 target-free semantic fallback。只有
`prospective_owner_target` exact route 才能在无 observation 时开启 bounded semantic target generation。repair-purpose provider
document 若返回 Definition gap，也必须封存 `repair_surface_unavailable`，不能创建 clarification ticket。只有
`need_state_change` repair-effect pre-seal manifest 的 purpose 固定为 `confirmed_validation_repair`，
`RuntimeDispatchRepairCommitment` 必须显式封存
`RepairOwnerModuleID` 及 origin/confirmation/rejected receipt/evidence/closure lineage；fresh-load 不得依据 capability 名称或
current mutable facts重选 owner。repair receipts 全部接受后，target-free fresh validation 保留同一 closure ID，但 purpose 必须
为空并按普通 Definition validation 生成 manifest；它不得再次要求 repair commitment。若该 continuation 发生确定性内部失败，
同一个 Orchestrator-private CAS 必须退休 directive 并封存 SM-2 failed terminal，SM-4 不得生成重复
`persisted_pending` successor。software-dev Profile 的 exact route 可将 owner specialization 为 File；任何具体
owner、diagnostic 或 repair vocabulary 都不是 Core repair 常量。

验证输入补充复用上述四机合同，不创建新 owner/状态机。Profile route 的 `RequireInputAddition` 只允许
`prospective_owner_target`；SM-2 在 rejected receipt 的私有事件中验证 typed `InputAddition` 的 required/original native
check 与 completed dispatch binding 精确匹配，且原 binding 尚无补充检查。缺失或不匹配与无可用 route 同样有界终止。
SM-3 的 semantic manifest/schema 只允许该 exact new target；resource owner 返回 feasibility，SM-2 内部事件才可 seal。
创建 receipt 不授予 validation credit；fresh owner catalog/binding 的 `SupplementalNativeCheckRef` 与原检查是 all-of。
新增字段进入诊断、manifest、binding、proof lineage 和 reconciliation identity；空扩展保留旧 JSON/hash。
SM-1 生命周期不变，SM-4 只重放 aggregate 与投递只读事实；模板/Soul 新配置仍仅进入新 Start/Restart 的快照，不改旧归档。

SM-3 candidate admission 与拒绝重放窗口必须复用同一 completed repair/confirmation/receipt/current-binding 反向证明：
target-free handoff 的旧 repair targets 仅保留因果含义，新验证入口由普通 Definition manifest 授权，允许与修复文件不同。
final repair evidence revision 之前的失败验证不能阻止在已改变状态上重跑同一 check；这不授予 repair CWI credit，
也不清除更晚的新失败。该事实仅在 Orchestrator 内部派生，不新增状态机、持久化 phase 或跨 owner 状态写入口。

Workspace 删除不是第五个 Runtime 状态机。Workspace owner 持久化唯一 deletion coordinator aggregate；Application
只提交一次 typed 删除意图。初次发现 active Run 时必须零删除意图返回 conflict。准入后，Run owner 先在自身
generation-head 线性化点以 private reducer/CAS 封存 `draining` fence：它永久拒绝新 Run、Restart、普通 SM-1、Step、
Checkpoint 与 generic Run mutation，只保留 Run owner 验证后的 exact opaque Runtime CAS drain lane。Orchestrator 在这一
`draining` 区间只允许已经存在的 terminal-delivery 与 delivery-lease claim/recover/release 收尾；SM-2/SM-3、普通 drive、
provider/capability/evidence 与非 terminal lease authority 都必须拒绝。其 `BeginWorkspaceRuntimeQuiescence` 与所有 Runtime
private mutation 共用 owner write barrier：只要仍有进程内 writer、timer、mount、persisted lease 或未完整确认的 terminal
delivery，就零状态变化返回 conflict。全部排空后，Orchestrator 才由自身 private reducer 持久化 `quiescing` gate；该
gate 落地后所有 Runtime mutation 一律 fail-closed，不再保留 drain 例外。随后 Orchestrator 封存绑定有序逐 Run
generation、Run revision、Runtime ref/revision/hash 的 typed quiescence token。Run owner 在同一 generation-head
锁内精确复核 token inventory后，才把自身 fence从 `draining` 原子升级为永久 `retired`；这是禁止后续 opaque Runtime
CAS 的唯一线性化点。CAS 先完成会使旧 token stale，`retired` 先完成则后续 CAS fail-closed，Run 不解释 Runtime
aggregate语义。Workspace 只有取得 exact retired fact后才能请求 Orchestrator seal；随后由独立的 Orchestrator-owner
private reducer把 durable deletion aggregate推进到 evidence-fenced。该 tombstone 与 ToolCall、Patch、
ValidationResult、Evidence、Coverage 和 ArtifactContinuation 的所有读写及 exact purge 共用同一
owner gate，`retain` 与 `purge` 后都禁止复生。Orchestrator 不能终止 Run、替 Workspace 推进删除 phase，也不能跨 owner
删除 Memory、Result、Decision、Approval 或 Eventstream 数据。各资源 owner 以各自 private reducer封存 tombstone 并执行
fixed-point purge；cross-owner command adapter 只能提交 owner-private `purge_requested`，物理 sink 成功后由同一个 private
dispatcher 生成 `purge_completed` 并 CAS，直接 completion 注入必须拒绝。Workspace 自有 Goal/DoD/目录与 ordinary
configuration sidecar/index 清理同样只允许 `directory_cleanup_requested` dispatcher 执行；它必须先通过独立 owner-family
typed cleaner 对 exact deletion identity 做 pre-clean/空集复核，再生成 `directory_cleanup_observed`，普通 Workspace CRUD
repository 不暴露该 effect authority。Eventstream 最后封存且 revision/global position floor 不复用。恢复只从
这些 owner aggregate/fence
幂等续驱；启动必须先恢复 Orchestrator durable gate 和 Run retirement fact：若 durable gate 已存在则不得再恢复任何
Runtime mutation；若只有 Run `draining`，则只恢复 exact terminal-delivery drain，排空后再重试 Begin。普通 Runtime
recovery不能越过任一 retirement admission。projection 与删除通知都不提供 authority。

凡 owner purge proof 绑定首次观察计数，owner 必须在任何物理删除前持久化包含该 exact receipt 的 `pending` journal，
清空并复核全部 owner surface 后再标记 `completed`。物理删除后、aggregate `purged` CAS 前的进程崩溃只能继续同一
pending transaction，并重放原 receipt/proof；不得从已经为空的 surface 重新推导零计数 proof。该 journal 是相同 owner
的事务技术状态，不构成新的 Runtime 状态机或跨 owner authority。

SM-4 convergence-terminal delivery 还必须通过 current semantic-authority fence：catalog retirement 后，只要当前 eligible
CWI 仍是 binding-free、semantic generation 尚未 seal，且 reducer 仅因该缺口形成
`target_grounding_unresolved`，SM-4 必须先 re-drive SM-3/SM-2 semantic transaction，不能把该临时 view 投递给
Result/Workspace/Run。current semantic operation authority ready 或 semantic transaction 自身 failed 后才可交付该终态；
其它已封存的 contract/budget/owner failure terminal 不得被重写为 semantic replan。administrative termination 使用独立的
Run-owner intent 与 SM-4 delivery handoff lane，不经过这一 convergence-only 例外；handoff record 本身不修改 Run state。

Runtime 必须区分 genuine governance wait、具备 typed recovery condition/trigger/deadline/budget 的 bounded recoverable
runtime wait 和 terminal failure。recoverable wait 是 SM-2/3/4 aggregate fact 与 SM-4 re-drive responsibility，不新增
SM-1 lifecycle state，也不能留下无后继的 `active + nonterminal` Run。完整记录、owner probe 映射与迁移见
[Recoverable Wait](runtime/recoverable-wait.md)。

provider、candidate、semantic proof、capability operation、repair 与 recovery 的消费按各自 persisted sub-ledger 管理；
产品级 execution-budget view 只确定性汇总，不成为第二份消费 authority。完整合同见
[Execution Budget](runtime/execution-budget.md)。
新 activation 可显式封存 `runtime-execution-limits/v1`，收紧同 Run 的 provider call/capability operation 物理上限，
并可选逐原 CWI proof-batch 上限。逻辑批次从 SM-3 当前/退休批次汇总，不新增账本或一般工作授权；Bootstrap 与
fresh-load 反查同一声明，SM-2 exhaustion 与 SM-4 recovery/delivery 继续使用原私有事件。

## Definition And LLM Boundary

- 原 Goal/DoD 是验收基准。Template、Profile、执行期验证输入和补充检查只能细化或增加与该基准一致的约束，不能
  删除、替换、放宽或通过冲突约束改变原验收语义；所有新增义务与原义务合取。审批、外部参考实现通过、版本升级、
  测试自通过及覆盖率均不是违反该规则的豁免。原准则与具体测试实现是不同对象：旧测试不能仅因存在而取得准则权威，
  也不能仅因失败就被认定错误；输入修改仍须有保留原准则的明确合同和证据。
- 输入补充/修复准入必须检查完整 binding 中唯一的原 required/native 映射，不得 first-match 后忽略其余冲突。
  新补充不能复用任何已有 native/supplement identity，输入修复不能借用另一 required check 的 identity；重复、替换
  或别名映射在既有 SM-2/SM-3 输入维护授权前拒绝。此纯身份校验不证明任意自然语言判据的语义一致性；无法建立
  一致性的候选不得获得新证明绑定或验收 credit，也不能自动变成实现修复。SM-1/SM-4 不新增迁移或归档回填。
- 正式 startup input 是 Goal、DoD、Workspace Template snapshot 与 resolved project workspace directory。
- 新 Start/Restart 读取当前 Template/Soul。Goal-only authoring 只复用 exact confirmed、eligible candidate；模板或 Definition policy 漂移保持阻塞，不能自动替换确认。其他受控 owner 定义路径由 Workspace-private admission 追加新 candidate、封存新 startup input。
  已准入历史只作不可变 lineage，不以当前 schema/compiler 重编译或改写。新配置无效仍在 pre-seal 阻塞。Resume/recovery
  保持原 Run snapshot，不热替换 SM-1～SM-4 输入，详见 [Automatic Configuration Loading](../40-profiles-capabilities/workspace/builtin-default-resources.md#automatic-configuration-loading)。
- canonical Goal/DoD aggregation、dependency、gate 与 terminal 只支持 `all_of`；不提供 `one_of`/`any_of` terminal escape。
- 显式 unsupported relation 只能存在于 Workspace owner 的 pre-seal admission candidate，并必须在 Run 前形成 blocking ticket。
- Definition admission 在 promotion 前分别无损编译 Template 与 Workspace 的 `EvidencePolicies`，拒绝空/重复 ID、
  invalid typed policy、normalize cardinality drift 及任何退役 `prospectiveTargetRef`，并把双方 exact policy
  ref/revision/snapshot hash 封存为 candidate 的 `AdmissionPolicyBindings`；final bootstrap 还 exact 比较 admitted
  Workspace policy projection。这些 binding 是 immutable policy authority，不是 exact prospective target-set authority。
- 没有 bounded exact candidate path 的 target-grounded prospective Profile declaration 以 `ProspectiveTargetAllOfConstraintRefs` 封存完整 mandatory File
  creation constraints；它随 Profile/startup hash 固定、没有 one-of 解释且不含 exact path。File owner 的 typed
  `TargetSelection` 必须逐字段回显/确认该集合；SM-3 proposal 的 `prospectiveConstraintRefs` 必须 exact-equal，只有 SM-2
  sealed binding 才可把随后提出的 exact `ProspectiveTargetRefs` 变成 authority。
- startup input 封存后允许执行期 LLM 基于 exact revision/hash 处理 semantic proof、candidate 或 bounded gap diagnostic；LLM
  不得改写输入、选择 Runtime action、授予 credit 或裁决 terminal。
- owner-fact acquisition 是同一 CWI 的有界前置 transaction，不是 terminal proof：manifest 必须同时保留 Definition 的
  terminal evidence kinds 与当前 acquisition evidence kinds。acquisition 成功只允许 SM-2 通过 owner-private event 重开
  同一 CWI 并生成 terminal-proof successor。LLM 的 `undeclared_prerequisite` 必须携带按 CWI 绑定的 exact typed
  `PrerequisiteClaims`；pure manifest admission 逐项区分 `already_satisfied | declared_with_acquisition |
  genuinely_undeclared`。前两类以及已声明但 unresolved 的矛盾合同都只进入同 request 的 bounded semantic correction，只有
  `genuinely_undeclared` 才允许形成 gap observation。
- prospective acquisition 还必须用 pre-seal exact contract ref 组合 acquisition-owner hypothetical delta 与 required-owner
  dependency observation。pre-seal Definition 只封存 target policy、declaration selector/candidate refs 与 acquisition
  closure；由其编译的 generation-specific obligation/manifest 再封存 operation selector 与 current owner
  constraints，两者都不封存 exact prospective target set。required owner catalog 必须发布 provider-safe、仅含
  `all_of` 的 typed `ProspectiveTargetSatisfactionConstraint`：每个 clause 内的 predicate 全部合取，所有 clause 也全部必须
  被覆盖，不存在 `one_of`/`any_of`/fallback relation。generation-specific acquisition/manifest 将该合同完整封存并
  只在 C3 投影。SM-3 只能提交一份 bounded semantic proposal，其 typed `ProspectiveTargetRefs`（JSON
  `prospectiveTargetRefs`）必须是 1～8 个唯一、规范化的 exact target ref；每个必需 clause 至少由一项满足，每个
  target 也必须对至少一个 clause 有贡献。所有 target 都必须位于 declaration selector/candidate refs 与 current
  operation selector/owner constraints 的交集；非空 candidate refs 对集合中每项都要求 exact member match。singular
  `ProspectiveTargetRef` 不是 Runtime semantic wire 的兼容字段；出现时必须由 strict current decoder 拒绝。schema/document
  admission 在 owner probe 前校验完整合取集合；缺项、多余 target、重复、越界或不满足 clause 只形成
  普通完整文档/schema correction，且零 File/Shell probe。
- SM-3 必须把 sealed manifest 的每条 required/auxiliary operation authorization 编译为同一最小
  `RuntimeCandidateWireContract`。required slot 用 `requiredOperationRef`、auxiliary rule 用 canonical kind 选择；唯一
  reachable variant、共有 singleton binding、required cardinality-one singleton target 与 fixed parameter 均由本地派生，
  只有真实多值选择可进入 provider wire。unreachable variant authorization 不得扩大 binding surface，JSONSchemaResponse
  on/off、schema、instruction、decoder 与 fixture encoder 不得产生不同 shape。
- operation document 的 `missing_*`/`empty_*` required field 必须先于 semantic frontier 形成同 path 的
  `schema_field(field_absent -> field_present)` witness；required field 已存在但为 `null` 或错误 JSON type 时，必须保留 strict
  decoder 观察到的 exact type，并形成同 path 的 `schema_field(<actual_type> -> field_present)` witness。两类 correction 都只授权
  attempt 1 重写一份完整 document，再由同一 strict decoder 证明 required typed shape；field/type vocabulary 不是
  target/binding alternative。operation rule 被选中后，diagnostic 必须携带 exact wire contract hash、operation index、required
  ref/canonical kind 与 variant identity；SM-3/LLM 不得从 `Actual` 或单 operation manifest 推断 matching rule。failed attempt 在
  CAS 与 fresh load 都必须验证 diagnostic/correction 的 manifest body、wire-rule identity、objectives、
  objective↔witness-dimension、path、actual/shape lineage；
  条件必填 binding 缺失必须表达为 `Actual=missing_bindingRefs`、`Required=[bindingRefs]`，不得用人类 cardinality 文本充当
  Required，也不得同时创建 evidence-binding alternative。不一致时零 attempt-1
  provider、dispatch 与 capability effect。
- fixed manifest parameter 被错误编码为另一个 `parameters[]` entry 的 sibling 时，strict decoder 仍拒绝原文；SM-3 可在
  declared name/type、entry/container、operation kind/index 均无歧义时封存一次 `parameter_entry_encoding` witness，要求
  attempt 1 重新生成全部独立 named typed entries。它不得本地搬移并接受原值，不得把合法 parameter 当作 forbidden field
  删除，也不改变 SM-2 authority 或 SM-4 recovery；mixed unknown/type/duplicate、lineage 漂移或 attempt 1 再拒绝均 final。
- executable operation contract 可在 manifest v7 的 operation/variant 上声明 owner-neutral、provider-safe 的 executable
  `ValueRules`，只表达非空、整数/字符串字节范围、布尔定值、同类型参数不同值和整数参数顺序。它们是声明者拥有的纯值合同，
  不是 capability owner fact、EventHub topic 或第五个状态机。strict decoder、manifest semantic evaluator 与 final operation
  validator 必须消费同一规则；在任何 owner I/O 前返回 exact rule ref、parameter path 与封闭
  `actual_shape -> required_shape`，raw value 不得进入 aggregate diagnostic 或 C4。SM-3 只能封存一次
  `operation_value_rule` witness，并只授权 attempt 1 修改规则点名的末端参数；还原该参数后完整 candidate 的 execution-semantic
  shape 必须与 attempt 0 相同，且修正结果须重新通过 selected operation/variant 的全部 `ValueRules`。没有实际变化、夹带
  target/binding/其它参数变化、rule/variant 漂移或 attempt 1 再拒绝均 final；该路径零 capability owner call、零独立预算，
  不改变 SM-2 authority，SM-4 只按 persisted result 幂等 re-drive。
- executable operation 及其 variant 可在 manifest 中声明 owner-defined、provider-safe 的 `CandidateConstraints`，描述普通 JSON schema
  无法完整表达的参数 shape 或 owner-observed target applicability。candidate proposal 必须先持久化；`parameter_shape` 由 receiving
  capability owner 通过 concrete typed conformance command 在零 workspace I/O 下返回
  `conformant | nonconformant | owner_contract_defect`，`target_applicability` 则只能在 ordinary typed feasibility 中以只读方式绑定
  current target/revision，并返回 `candidate_constraint_violated` 或正常 feasibility outcome。两类 owner fact 都必须精确绑定
  `attempt + LogicalRequestID + operation + manifest constraint`，owner 不得执行 operation、选择 correction 或推进 terminal。
  Orchestrator 只通过 private SM-3 event/CAS 持久化 fact 与 disposition；尤其 `CandidateFeasibilityObserved` 必须在同一 CAS 中同时
  保存 applicability violation 并关闭 attempt 0 为唯一 correction，attempt 1 重复违反直接
  `candidate_retry_exhausted`。final seal 必须证明每个 static constraint 有 matching `conformant` fact，且 dynamic applicability
  已由 current feasibility observation 接受。operation-level 与 selected variant-level constraints 合并后共同验收；
  variant-less operation 不得漏检，dynamic constraint 缺少 owner admissibility 时不得走 static seal。
  未声明相应 constraint 的 operation 不得生成占位 fact、额外 owner call 或 correction。
- semantic-plan 与 operations preflight 必须发生在 exact request-session CAS 之前；同一 transaction、stable prefix、manifest、correction hash 与
  expected-document attempt 的 persisted session 是 restart proof。document-complete 后 attempt-begin CAS 前中断或 CAS 后响应丢失
  只能恢复同一 local admission transaction，不能重发 preflight/provider、重复扣减 correction budget 或提前执行 capability。
  completed document 一旦进入 attempt，无论 accepted/rejected，都必须在 attempt closure 的同一 owner-private CAS 保存 exact
  CandidateManifest body；wire diagnostic 在 fresh load 时只能由该前像重验，不能从 current mutable authority 重建。
  semantic coverage-order、proof-batch 和 correction private event 只封存请求意图，不提前创建 session；预检拒绝由 SM-2
  `semantic_contract_failed` 收口，零新 session/physical call/allocation，不能走要求已结算调用的 provider-terminal 路径。
  SM-3 session writer 必须 fresh-load 当前请求意图，新建/复用都反查 ledger logical identity、active batch/CWI/order、
  correction attempt 和 sealed output/recovery budget；仅 required/order-accepted/document-accepted 不授权 session，
  旧预检成功不能越过此门禁，不能补写传入 ExpectedDocument 的错误身份。
- strict decoder 已得到 current wire 的完整 typed operation document、但 manifest semantic admission 因 exact
  `bindingRefs` 等字段失败时，SM-3 只可把该 typed value 保存在 failed `CandidateAttempt` 的 owner-private correction
  frontier；它从不成为 accepted candidate、feasibility input、dispatch 或 authority。manifest 为 exact operation slot 声明
  非空 allowed evidence binding 时，attempt 0 可封存一次 `evidence_binding` witness，并要求 attempt 1 在原 field path
  选择不同且 exact allowed binding；没有合法 alternative 直接 `candidate_authority_exhausted`，attempt 1 任意再拒绝直接
  `candidate_retry_exhausted`。framing 不完整、非 current wire 或无法严格解码的 document 不得保存候选值。
- File 对整个 target set 使用同一 complete manifest revision 执行 path/existence/parent/capacity admission，并只返回
  bounded hypothetical resulting-manifest delta；该过程不创建文件或目录。Shell 只使用 delta 的 resulting manifest
  判定 `would_satisfy | would_not_satisfy | stale`，不读 File/workspace、不启动 process/session。File/Shell 都只返回
  factual feasibility/satisfaction；只有 Orchestrator SM-2 private internal event 可封存
  `CoverageEvidenceBinding` 与这一完整 target set。封存后，SM-3 `RequiredOperationSequence` 必须为每个 target
  编译一项 exact singleton `TargetRef`，按 canonical dependency chain 完整、有序执行；候选缺项、附加 target、同一
  requirement 重复或顺序漂移均在 dispatch/effect 前拒绝。operation candidate 只引用 current binding，dispatch 不得重新
  选择 target。dependency catalog 不进入 LLM selectable surface。
- multi-target prospective all-of execution 的完成证明必须来自同一个 sealed dispatch。每个 required operation 都必须取得
  owner-committed、`Outcome=accepted`、exact `ExecutionKey + Fence`、exact binding/evidence kind 且 singleton target 匹配的
  receipt；`owner_committed` 只是必要 owner fact，只有 Orchestrator private evidence event 把对应 execution 与 evidence
  原子提交为 `evidence_committed` 后，该 member 才能进入 collective completion proof。前 N-1 个 accepted member 只能形成
  持久化 operation/evidence progress，不能令 CWI `satisfied/settled`、推进 focus 或关闭 all-of dispatch。rejected、stale、
  weak/mismatched receipt 不属于完成集合；duplicate 不得重复计数、CAS、credit 或 effect；restart 只能从同一 dispatch 的
  persisted execution/receipt/evidence ledger 继续缺失 member，不得跨 dispatch、repair 或 semantic generation 拼接集合。只有
  全部 target member 对每个 required evidence kind 齐全时，Orchestrator SM-2 private event 才能 exactly-once 认可
  collective proof；只有其它 quality/effect/assessment gate 也满足时，SM-2 才能通过 aggregate CAS 授予 CWI
  credit/settlement。capability owner、LLM、Application 与 projection 均无此 authority。
  typed stale 只走 bounded fact refresh，不消耗 semantic correction。除显式且 manifest-bound 的
  `candidate_constraint_violated` 外，owner feasibility 不进入 candidate correction。已通过 typed constraint 的 proposal 若仍得到
  exact-current full-composite `would_not_satisfy`，该事实表示 required-owner published contract 与 owner judgment 自相矛盾；
  Orchestrator 直接记录 `prospective_satisfaction_contract_violated` 并收口为 `semantic_contract_failed`，不得打开
  replacement-target correction、再次请求 provider 或创建 ticket。attempt 1 仅保留普通完整文档/schema
  correction，C4 只引用 manifest/CWI/违规 field，不复制 constraint。File/Shell 只拥有 factual
  feasibility/satisfaction，不触发 correction/terminal/SM-2 transition；该边不新增状态机、跨 owner transition 或可替代
  terminal 关系。
- File `unified_diff` candidate conformance 必须在零 workspace I/O 下分别证明 single-file target identity 与 sealed target-policy
  mode：prospective 只接受 add-file `/dev/null`，exact/existing 只接受 update 形态。对 current owner-observed target 无法应用的合法
  diff，File feasibility 必须返回 manifest-declared `target_applicability` constraint 的 typed
  `candidate_constraint_violated`，绑定 exact target/revision；不得将其压平为普通 infeasible、binding contradiction 或私有
  diagnostic correction。资源 probe 若证明 prospective 已存在或 exact/existing 已缺失，必须返回 typed
  `sealed_target_authorization_contradicted`；Orchestrator 才能由 private SM-2 event退役 exact binding。validation-failure repair 的
  其它 owner infeasibility 必须在同一 private CAS 中失效 candidate、退役中间 binding 并 bounded-terminal 为
  `repair_surface_unavailable`，禁止遗留 pending candidate 进入 SM-4 重复 re-drive。
- Definition 修订不能被 Resume 吸收；需要新输入时必须显式 Restart，并遵守 replacement/delivery transaction。

## Four Coordinated Machines

| ID | Sole owner | Owns | Must not |
| --- | --- | --- | --- |
| SM-1 | Run | activation、continuation、administrative lifecycle，以及同 owner subordinate replacement transaction | 选择 focus/action、解释 aggregate，或让 SM-4 代写 Run terminal |
| SM-2 | Orchestrator | authority、CWI、binding、credit、assessment、terminal | 调 provider 或执行 capability effect |
| SM-3 | Orchestrator | request session、attempt、candidate、sealed dispatch、receipt transaction | 选择下一 action 或绕过 seal |
| SM-4 | Orchestrator | bounded drive、singleflight、recovery、delivery ledger | 从 projection/history 重建 authority |

## Forbidden Production Surfaces

`StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher、application
Start/Resume queue、自动 replan successor、checkpoint replay，以及 map/JSON/reflect authority bridge 均不得回到生产合同。

## Change Gate

每次 Runtime 变更必须：

1. 标明受影响的 SM、owner、persisted fact、合法及禁止迁移；
2. 为 owner-defined concrete command/result 覆盖 producer/consumer；
3. 覆盖 normal、duplicate、stale、CAS 前中断、CAS 后响应丢失与 fresh-process reload；
4. 覆盖 budget/guard 和零副作用断言；
5. 同步对应 core 子合同、组件 README、flow 与 verification；
6. 新增或重命名任何 `State | Status | Phase | Disposition | Outcome | Mode | Lifecycle | Stage` string type 时，必须在
   verification-only `scripts/state-model-owners.manifest` 中把它穷尽分类；只有 authoritative/owner-private state 可以绑定
   `scripts/state-transition-owners.manifest` transition。一个 state type 可绑定多个协调 transition，但所有 transition 必须
   回到该 type 的同一个唯一 owner；projection、outcome、disposition、technical mode 与 invariant 不得绑定 transition；
7. 通过 `make verify-boundary-contract` 及受影响测试。

实施进度、候选需求、dated run 和 closure diary 不属于本合同，也不进入 `docs/` 的规范树。
