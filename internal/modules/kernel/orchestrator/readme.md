# Runtime Orchestrator Module

Status: active

Type: kernel-module-implementation-contract

Last Verified: 2026-09-03

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
- `Facts.Failure` 的替换只能经过 `replaceRuntimeFailureAuthority`，并必须显式选择三种 policy 之一：
  `runtimeFailureAuthorityRetainInterlude` 仅在同一原子事务的有界中间态保留既有 target/directive；
  `runtimeFailureAuthorityRetire` 同时清除旧 `FailureTarget + RepairDirective`；
  `runtimeFailureAuthorityReplaceFromExactOwnerFact` 只接受与新 failure signature/targets/observation exact-match
  的 owner fact，并先退休旧 directive。任何普通 failure 写入不得隐式继承 repair authority。
- `internal/pkg/workorchruntimecontract` 只包含 DTO、enum、normalize/validate/clone/hash、manifest compiler 和纯 matcher；
  不持有 Hub/I/O/CAS，也不选择 retry、terminal 或 Runtime action。

### Semantic and candidate authority

- activation 只消费 Run owner 已封存的 `RuntimeBootstrap`。Definition startup input、Profile contract、provider selection、
  Soul/Skill/MCP/Memory snapshot 和 `StartupInputHash` 在既有 Run 中不可改写。
- required CWI 在 operation candidate 前必须由 SM-2 从 immutable Definition seed、typed owner catalog、SM-3 semantic document
  与 matching owner feasibility 封存唯一 `CoverageEvidenceBinding`。不存在 binding-free operation authority。
- Goal/DoD、prospective constraint、required sequence 和 collective receipt 只支持 conjunctive `all_of`；不存在
  `one_of`/`any_of` terminal escape。Definition gap 只能进入专用 clarification transaction。
- provider-facing manifest、closed JSON Schema、strict decoder、single evaluator 和 owner feasibility 共享一个 sealed
  `RuntimeCandidateContractManifest`。Orchestrator 从它纯编译 `RuntimeCandidateWireContract`；required slot 仅由
  `requiredOperationRef` 选中，auxiliary rule 仅由 canonical kind 选中。唯一 reachable variant、所有 reachable
  authorization 共有的 singleton binding、required cardinality-one singleton target 和 fixed parameter 均本地派生；
  unreachable variant authorization 不扩大 wire。LLM wire 不包含这些本地值或 operation ID/dependency；本地 compiler
  唯一派生完整 DTO、identity 与 graph。
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
  structured observation 只有匹配 route 才进入 owner confirmation；无 observation 且匹配 route 才进入 bounded semantic
  repair fallback。observation diagnostic hash 绑定完整 route 字段与 sealed validation scope，并在 receipt admission/fresh-load
  从 immutable owner receipt 重算。
- candidate proposal 持久化后，SM-3 必须按 manifest-declared `CandidateConstraints` 的 kind 获取 exact owner fact。
  `parameter_shape` 在 ordinary feasibility 前通过 typed conformance command 完成，owner 只复用自身 parser/matcher 返回封闭
  outcome/shape且不读写 workspace；`target_applicability` 只能由 ordinary typed feasibility 对 current target/revision 返回
  `candidate_constraint_violated`。两者均以 `attempt + LogicalRequestID + operation + manifest constraint` 持久化 restart fence；
  `nonconformant` 或 applicability violation 与 attempt closure 必须在同一个 private event/CAS 中原子提交，
  `owner_contract_defect` 直接 fail closed。没有声明相应 constraint 的 operation 不发 placeholder call或开启 correction。
- ordinary owner infeasibility 在当前 binding purpose 为 `validation_failure_repair` 时不得走会清空 directive、保留 binding 的
  通用 failure 路径。Orchestrator 必须在同一个 private event/CAS 中 invalidates candidate、以 exact owner observation 退役该
  intermediate binding，并形成有界 `repair_surface_unavailable` terminal；duplicate/restart 不得重复退役或消耗 effect/budget。
- JSON Schema response format 是可选 transport acceleration。format on/off 都必须经过同一 local decoder/evaluator；format
  failure不授权第二次无 format physical call。
- incomplete/empty/invalid framing 是 no-candidate fact，只使用 request-session whole-document recovery，零 candidate attempt/C4。
  只有 complete-object schema/contract 与 semantic rejection 共用 attempt 0/1；只有 persisted typed correction witness 可开启
  attempt 1，且 attempt 1 任意再次拒绝 final。transport/document recovery、owner feasibility、repair 与 governance budget不得
  借用该 attempt ledger。
- manifest v6 静态约束普通 durable candidate 的 `(OwnerModuleID, canonical TargetRef)` cardinality 为 1；只有 exact
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
- `undeclared_prerequisite` provider document 必须携带 CWI-bound typed `PrerequisiteClaims`。Orchestrator 在 private event/CAS 前只调用
  pure `EvaluateRuntimeDefinitionGapAdmission`，按 sealed proof obligation 与 owner catalogs 区分 already-satisfied、
  declared-with-acquisition、declared-unresolved 和 genuinely-undeclared；只有最后一类能提交
  `RuntimeInternalEventDefinitionGapObserved`。其余分支复用普通 semantic correction ledger，零 governance/ticket/operation effect。
- proof batch 的 `RequestLogicalID` 是 immutable semantic generation identity；attempt-1 只推进 ledger 当前 provider
  request-session cursor。accepted document hash、owner fact、SM-2 seal 与 fresh-load replay 都必须绑定同一 batch identity，
  禁止 seal 从 ledger cursor、最后一个 session 或下游 projection 选择 generation。
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

### Receipt, evidence and repair

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
  同时存在也不能进入该 repair manifest。
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

### Recovery, governance and delivery

- provider、operation、CAS、repair、governance、terminal delivery 使用独立 persisted budget/guard。allocation 在 external
  call/effect 前与授权同 CAS 创建；submitted outcome必须 settlement，unknown conservatively占用预算。
- `RuntimeDriveLease` 是 delayed recovery 的唯一 authority。timer只 claim exact due generation；process-local gate/timer 仅合并
  persisted identity。Stop 清理本地等待但不删除 durable lease，fresh startup从 Run owner inventory恢复。
- framework `Setup` 必须同时注入 EventHub 与 `BackgroundRoutine`，`Run(ctx)` 把 framework context 绑定到全部 owner-local drive、
  timer 和 startup recovery。startup 任一步失败必须撤销订阅并取消本地 lifecycle；`Stop(ctx)` 先取消 context、关闭 ingress，
  再按 deadline 等待既有 handler/drive 退出并清空本地依赖。该锁与 context 只做 Module 生命周期同步，不是 SM-1/SM-4 authority；
  durable successor仍只来自 aggregate lease/authorization，重启从 Run owner inventory 恢复。Module 保持 startup weight 30、
  teardown weight 90：前者保证 Orchestrator ingress 先于 Run recovery，后者保证 Orchestrator worker 先于 Run/capability teardown
  停止；两个依赖顺序不得再复用同一个反向权重推导。
- startup 只用 Run owner 的 bounded recovery inventory/readiness/opaque record query。unknown/retired current JSON、cursor cycle、
  page/item limit、nil/wrong result直接使启动失败，零 provider/capability/governance side effect。
- Decision/Approval owner 持久化 resolution；Orchestrator只暴露 ticket-resolution 与 approval-resolution 两个具体 typed ingress，
  先从 aggregate 重建 exact persisted binding，再在 owner-private value adapter 中提交 continuation或terminal。不存在 generic
  governance-resolution EventHub command/topic；Application和Governance都不发布 Resume。
- terminal outcome先由 SM-2 seal，再由 SM-4按 Result→Workspace→Run顺序交付。各 owner只执行自身 private reducer并返回 exact
  acknowledgement；partial/response-loss从同一 delivery ledger幂等续驱，不改写 terminal decision。
- Workspace deletion只允许在 Run owner `draining` fence后排空既有 terminal delivery。Orchestrator独占自身 quiescence/purge
  private reducer，不终止 Run、不推进 Workspace deletion phase、不跨 owner删除资源。

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
- provider-facing cache保持C0 system/schema/runtime、C1 Goal/DoD、C2 declarative capability/governance、C3～C5 volatile runtime
  分层；unexpected stable-prefix drift是rendering defect。

## 能力验收

| 验收面 | 自动化证据 | 必需维度 |
| --- | --- | --- |
| Module lifecycle/core dependencies | `plugin_test.go::TestModuleRunBindsFrameworkContextToOwnerLifecycle`、`plugin_test.go::TestModuleSetupRejectsMissingBackgroundRoutine`、`biz/owner_test.go::TestModuleIdentityConfigureStartStopContract`、`biz/owner_test.go::TestModuleConfigureRequiresCoreDependencies`、`biz/owner_test.go::TestFailedStartRemovesAllIngressAndCancelsLifecycle` | identity、Configure/Start/Stop、framework context、required BackgroundRoutine、startup rollback、deadline-aware Stop、canonical evidence repository bundle 与 EventHub dependency fail-fast |
| owner-private dispatcher/CAS/aggregate | `biz/runtime/internal_event_*_test.go`、`biz/runtime/component_facades_test.go`、`biz/runtime/aggregate_invariant_test.go`、`biz/runtime/structure_contract_test.go`、`biz/runtime/failure_authority_transition_test.go`（含 `TestOwnerInfeasibilityAtomicallyTerminatesValidationRepairBinding`）、`biz/runtime/candidate_owner_conformance_test.go::TestOwnerCandidateApplicabilityViolationClosesFactAndCorrectionInOneCAS` | exactly-one payload、primary SM 与 coordinated write-set、single mutation root/CAS、三种 failure-authority policy、candidate feasibility fact+disposition 单 CAS、validation-repair infeasibility 原子退役/terminal、旧 target/directive 不泄漏、duplicate/fresh-load 与 pre/post-CAS failure |
| SM-2 semantic/CWI/binding | `biz/runtime/semantic_*_test.go`、`biz/runtime/cwi_*_test.go`、`biz/runtime/coverage_*_test.go`、`biz/runtime/assessment_*_test.go`（含 prospective effect 后 File `unsupported`、Definition-seeded owner-fact successor、restart/duplicate）、`biz/prerequisite_event_adapter_test.go::TestAcquireRuntimeTargetSelectionPrerequisiteUsesTypedEventHubContract`、`biz/runtime_drive_test.go::TestRuntimeFileBackedNextProofBatchSettlesTargetSelectionBeforeProvider`、`biz/runtime_state_repository_test.go::TestRuntimeStateRepositoryTargetSelectionCommitCrashRestartIsAtomic`、`biz/runtime/cwi_ledger_schema_test.go::TestValidationFailureRepairSemanticRebindingClearsExecutionAuthorityThroughFocusReducer`、`biz/runtime_semantic_drive_test.go::TestRuntimeSemanticFalseUndeclaredPrerequisiteUsesCorrectionAcrossRestart` | all-of、owner fact、typed gap claim 的 satisfied/acquisition/genuinely-undeclared admission、duplicate/stale/ABA、reopen/settle、semantic-pending exact `RuntimeFocusSeed`、batch rollover 的 owner prerequisite→CAS→request identity 顺序与零额外 provider/budget、predecessor execution-authority purge、attempt-1 correction→feasibility→batch-generation seal、proof preimage/fresh-load、false gap 零 governance/effect |
| SM-3 document/candidate/seal | `biz/runtime_candidate_request_content_test.go::TestRuntimeCandidateRequestContentBridgeCompletesDocumentWithoutCandidateAttempt`、`biz/runtime_drive_test.go::TestRequestAndSealRuntimeCandidateNoDocumentUsesBoundedRequestRecovery`、`biz/runtime/candidate_*_test.go`、`biz/runtime/dispatch_*_test.go`；`candidate_owner_conformance_test.go::TestOwnerCandidateConformanceTransitionIsAtomicRestartSafeAndBounded`、`TestOwnerCandidateConformantFactSurvivesRestartAndAllowsDispatchSeal`、`TestOwnerCandidateApplicabilityViolationClosesFactAndCorrectionInOneCAS`；`biz/runtime_operation_admissibility_event_adapter_test.go::TestProbeRuntimeOperationFeasibilityAcceptsManifestBoundCandidateApplicabilityViolation`；`TestRequestAndSealRuntimeCandidateReencodesDeclaredParameterEntriesOnAttemptOne`；`TestCandidateCorrectionFrontierSealsRequiredParametersContainerDefects`、`TestCandidateCorrectionFrontierSealsAndCrossesOperationSetConsolidation`、`TestCandidateCorrectionFrontierRejectsUnboundOrDriftedMultiOperationWireRule`；`TestRuntimeCandidateWireDiagnosticRequiresExactPersistedManifestPreimage`；`workorchruntimecontract::TestRuntimeCandidateRequiredFieldTypeCorrectionBindsExactDiagnostic`、`TestRuntimeCandidateProviderWireRejectsNullContainersAndFixedFieldEcho`、`TestDecodeRuntimeCandidatePayloadWithManifestPreservesExactMultiOperationWireRule`、`TestRuntimeCandidateWireContractCompilesAuxiliarySingletonAuthorityLocally`、`TestRuntimeCandidateWireContractRetainsNonSingletonAuxiliaryChoices`；`blocks/llm::TestRuntimeCandidateCorrectionInputAcceptsExactNullRequiredContainer`、`TestRuntimeCandidateManifestCorrectionProofUsesExactWireRuleAcrossOperationKinds`、`TestRuntimeCandidateAuxiliarySingletonWireIsMinimalWithJSONSchemaResponseOnOrOff`、`TestRuntimeCandidateRetryProjectsOwnerConstraintOnlyInC4WithoutStablePrefixDrift`；`biz/runtime_state_repository_test.go` 中的 `TestRuntimeMainlineMissingRequiredTargetCorrectionSurvivesRestart`、`TestRuntimeMainlineSchemaCorrectionDocumentCASRestartMatrix`、`TestRuntimeFileBackedSemanticCandidateReplayExhaustionSurvivesCASBoundaries`、`TestRuntimeMainlineCandidateAuthorityExhaustionHasNoCorrectionProviderOrBudgetEffect`、`TestRuntimeMainlineRenderedPreflightStopsBeforeCandidateAttempt` | no-candidate framing 零 attempt、corrupt complete carrier terminal、schema/semantic correction、required/auxiliary 最小 wire、multi-operation exact rule identity、operation-set cardinality、schema-first missing/empty/null/wrong-type required field、parameter-entry re-encode、owner conformance/applicability proposal→fact→feasibility→seal、跨组件 typed result exact binding、failure↔correction durable lineage、attempt 0/1、target/binding/effect/dependency；owner fact duplicate/response-loss/fresh restart、atomic violation closure、single retry、correction preflight 零 provider/attempt、document-complete 后 attempt-begin CAS 前中断/CAS 后响应丢失、预算单次扣减、无合法 correction 时零额外 provider/budget、schema rejection 零 dispatch/receipt 均属 SM-3 |
| capability effect/receipt/evidence | `biz/runtime_operation_receipt_source_test.go`、`biz/runtime/capability_receipt_transaction_matrix_test.go`、`biz/runtime/*_owner_receipt_integration_test.go`、`biz/runtime/operation_receipt_test.go`、`biz/runtime/candidate_evidence_*_test.go` | 六类 owner 的 descriptor/source/query 边界矩阵；empty/whitespace/noncanonical/wrong/cross 的 Query/Commit 零 CAS、aggregate 不变；File 的真实 adapter 路径及 Git/Shell/SSH/MCP 的 proof/state-change 路径均覆盖 production-shaped admission→seal→claim→accepted/no-op/rejected Commit/Query；Web 只作为无 CWI credit 的 auxiliary validation 覆盖同一事务，不能被提升为验收证明；retirement fence 与 receipt-loss recovery均有回归 |
| owner-grounded repair | `biz/runtime/validation_failure_repair_test.go`、`biz/runtime/repair_surface_confirmation_test.go`、`biz/runtime/candidate_evidence_repair_transaction_test.go`（含 `TestConfirmedRepairValidationOnlyBindingManifestRetainsSourceLineage`）、`biz/runtime/repair_owner_neutrality_test.go`、`biz/runtime/mcp_owner_receipt_integration_test.go`、`biz/runtime/reducer_test.go` | exact route lookup/no-route same-CAS terminal、sealed Profile/descriptor→唯一 `RepairOwnerModuleID`、confirmation/operation/receipt owner exact-match、validation-only source binding只保留 causation、零 terminal CWI credit；`confirmed_validation_repair` 仅属于 state-change，fresh validation保留 closure/清空 purpose/可原子 terminal；single/multi all-of、partial/rejected/duplicate/restart/governance；具体 owner specialization不进入 core 分支 |
| governance/clarification | `biz/runtime/governance*_test.go`、`biz/runtime/definition_clarification_test.go`、`biz/runtime_lifecycle_handoff_test.go` | ticket/approval 两个 concrete ingress、无 generic resolution ingress、exact binding、continue/fail、duplicate/stale、response loss、blocked Run ack |
| SM-4 wait/recovery/delivery | `biz/runtime/recovery_budget_test.go`、`biz/runtime/executor_test.go`、`biz/runtime/drive_lease_test.go`、`biz/runtime_lifecycle_handoff_test.go`、`biz/runtime_drive_result_test.go::TestRuntimeTerminalDeliveryWaitsForCurrentSemanticAuthority`、`biz/runtime_result_projection_test.go` | singleflight、timer/Stop epoch、CAS cut points、operation authorization issued→in_flight→settled\|uncertain、claim lease/双 worker/counter/ledger/dispatch fresh-load reverse proof、fresh process、semantic-rebinding provisional terminal 与 SM-3/restart 同分类、budget、three-owner partial ack；`runtime_state_repository_test.go` 中的 correction transaction 不归入 SM-4 |
| Workspace deletion | `biz/workspace_retirement_test.go`、`biz/workspace_runtime_quiescence_adversarial_test.go` | draining/quiescing fence、in-flight writer、retain/purge、pending journal、restart/tombstone |
| LLM request/cache/audit | `biz/runtime/provider_request_recovery_test.go`、`biz/runtime/provider_request_terminal_test.go`、`biz/runtime_candidate_event_adapter_test.go` 与 `../../blocks/llm/biz/runtime_*_test.go` | ceiling、identity、format on/off、submission settlement、raw isolation、C0～C5 stability |
| core neutrality | `biz/runtime/core_neutrality_contract_test.go`、`biz/runtime/semantic_failure_class_test.go`、`biz/runtime_semantic_drive_test.go` | typed feasibility/status 与 descriptor/Profile 驱动分类；不解释 capability owner 常量或私有 diagnostic prefix |
| static owner boundary | `make verify-boundary-contract` | no forbidden surface、no cross-owner service/repository、state owner manifest consistency |

真实 provider、远端 endpoint 和长时间网络故障由
[`deployed-convergence-scenarios.md`](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) 验收；部署事实不能替代
本表的 deterministic contract tests。
