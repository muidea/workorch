# WorkOrch Core Runtime Verification

Status: normative

Type: verification

Last Verified: 2026-09-01

## Purpose

本文定义 Runtime 当前唯一的 core 验证合同。验证对象是四状态机、单 owner、typed EventHub 边界、
canonical decision、无损反馈、sealed dispatch、receipt 与 terminal delivery；禁止的 graph/step/checkpoint 与
一次性 workspace fixture 不能作为通过标准。

## Authority And Scope

core verifier 只验证：

- SM-1～SM-4 的 owner、合法/禁止迁移与跨状态机交接；
- Runtime EventHub contract 的 `ReducerOwner + MachineRole` 与 canonical owner 一致，participant/opaque persistence
  不能伪装成 reducer；正式 state mutation 只能进入 owner-private internal-event dispatcher；
- `ConvergenceState` 是唯一 canonical decision，action/output kind 仅可派生；
- request、candidate、feasibility、operation、drive、delivery 的 stable factual category 与 safe original
  diagnostic 全链路保真；
- candidate request session、attempt、sealed dispatch、operation execution、assessment、governance binding 与
  delivery acknowledgement 保持正交；
- capability receipt outcome、terminal outcome 与 owner lifecycle 各自只有一个权威字段；
- C0～C5、provider ceiling、stable prefix、audit-safe/provider-safe 投影和敏感信息边界；
- workspace/run scope、restart、duplicate/stale/CAS、exactly-once effect 与 projection 单向性；
- core 的领域中立性和 22 运行单元的 pkg / concrete EventHub 边界。

core verifier 不验证 software-dev 特定文件、语言、测试命令、目录或业务 DoD；这些属于 profile verifier。

`scripts/state-transition-owners.manifest` 是唯一 verification-only owner/dispatcher 清单。它必须显式列出 SM-1～SM-4
以及每个 `*_internal_event.go` / `*_internal_events.go` / `internal_event.go` owner-private transition、transaction、effect、audit 或 derived
projection gate；owner root、private-event file 与 dispatcher 必须都位于同一组件 `biz/`。新增内部事件文件但未归属、
重复 transition ID、未知分类、dispatcher 缺失或 owner 漂移必须使 boundary verification 失败。该清单不得被生产代码
导入或用于 reducer dispatch；正式 Runtime owner 仍只由 typed contract 和 private reducer 决定。state model 可以列出
多个协调 transition ID，但 verifier 必须证明它们全部回到该 state type 的同一唯一 owner；跨 owner 组合必须失败。
`run.replacement` 必须分类为 Run-owner `owner_private_transaction` 并与 SM-1 owner 相同，不能作为第五个状态机。
Orchestrator private-event verifier 还必须穷尽 kind→concrete payload family→primary SM；允许的跨 SM 写集只能落在同 owner
package-private transaction 与单次 aggregate CAS 内。新增 kind 未分类、同一 payload 被多个 ingress 分步提交、协调写集留下
torn sibling ledger，或 primary SM 被当作第二个 reducer owner，都必须失败。
Workspace 删除、Run 退役以及各资源 owner purge 的 privileged physical sink 还必须在所属 owner production root 中只有
一个调用点，且该调用点必须位于清单声明的 private dispatcher；全 production tree 的其他 reference 仅允许列明的
repository storage delegation。仅证明 dispatcher 内“存在一次”不足以阻止第二入口或 owner 外 helper 转调。
repository family 的解析只允许经 `internal/pkg/workorchdeps` 完成，其他生产包直接调用 Initiator
`RepositoryProvider()` 必须由同一 AST 门禁拒绝。

静态门禁同时将 initial state 视为 transition：Workspace/Run/Decision/Approval 的创建必须经过各自
owner-private creation/lifecycle dispatcher，创建入口不得直接 repository `Create` 带 state。它还必须拒绝
application-local execution queue/queued projection 的回归，以及 setup-draft 等派生状态的单独持久化。

## Four-State-Machine Mandatory Matrix

| 状态机 | 唯一 owner | 必须验证的合法迁移 | 必须拒绝的迁移 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | activation/continuation/typed terminal delivery 先成为 owner-local event，再执行 pending→active、blocked continuation 或 terminal；replacement 仅作为同 owner subordinate transaction 协调交接 | 外部指定目标 state、读取 Orchestrator action/focus 选择 lifecycle、SM-4 handoff record 直接改 Run、terminal→active |
| SM-2 authority/focus | `kernel/orchestrator` | prerequisite/feasibility/receipt/assessment/governance fact→internal event→canonical decision；focus/evidence/repair 原子推进 | capability/adapter 选 decision、错误文本选 target、直接写 Run/Workspace/Result |
| SM-3 candidate/dispatch | `kernel/orchestrator` | request session→typed document observation→strict local admission/single evaluator→persist diagnostic→final/唯一 correction；accepted→本地派生 operation identity/dependencies→seal→execute | incomplete/transport 创建 attempt、format/schema/semantic 分设预算、attempt 2、adapter/SM-4 选 correction/terminal、未 seal 执行 |
| SM-4 drive/recovery/delivery | `kernel/orchestrator` | Run fact→run-scoped singleflight→load/re-drive；typed conflict/recovery/physical settlement；administrative `RuntimeTerminationRecord` handoff；Result→Workspace→Run acknowledgement | 重建 SM-2 authority、解释 document rejection或选择 attempt、projection/application 反向驱动、重复 durable effect、把 handoff 当 SM-1 transition、写其他 owner state |

每次 Runtime 变更必须标出受影响 SM，并补：transition、invariant、restart/idempotency、stale/duplicate、
budget/guard 以及适用的跨状态机合同测试。

## Owner Resource Matrix

| Resource | Sole owner | Verification must reject |
| --- | --- | --- |
| Workspace candidate/Definition/startup/active slot | Workspace Block | projection/Run/Application direct mutation or unresolved activation |
| Workspace deletion coordinator | Workspace Block | Application/participant phase journal、cross-owner partial ack authority或projection recovery |
| Run lifecycle/opaque record/replacement/retirement | Run Block | caller target state、Orchestrator lifecycle choice、把 same-owner transaction 注册为另一状态机、duplicate successor、跳过 `draining -> retired` token seal |
| Runtime quiescing/evidence deletion | Orchestrator | process-only gate、Run permanent fence前关闭opaque drain、伪造quiescence inventory或跨owner purge |
| Result formal outcome | Result Block | Result recomputing acceptance or generic entity write bypass |
| Decision/Approval | corresponding Block | unbound ticket/resolution or cross-owner repository access |
| capability effect/session | corresponding capability Block | generic executor、unsealed operation、owner receipt rebuilding credit |
| audit/projection | Eventstream/workorchd | replay/reverse repair into owner authority |

Cross-owner regression必须证明 concrete receiving-owner Command/Result、workspace/run scope、missing/wrong result handling，
以及 Send/Post 的语义分离；不得使用 Envelope、map/any、JSON/reflect bridge 或可变资源句柄。

## Contract-Normalization Assertions

### Canonical Decision And Terminal

- aggregate、authority snapshot、transaction、request identity 不持久化独立 `RuntimeAction` 或
  `RequiredOutput.Kind`；
- provider-facing operation mode 只能从 sealed canonical state/output constraints 派生；
- Runtime aggregate 只保存 canonical state + terminal code/summary，不保存第二个 terminal kind；
- SM-4 sealed delivery 使用 `TerminalOutcome.Kind = completed | failed | cancelled | superseded`；
- completed/failed 只来自 sealed SM-2 terminal；cancelled/superseded 只来自 Run SM-1 typed administrative intent，且
  `RuntimeTerminationRecord` 仅可作为 SM-4 exact handoff，不能直接改变 Run state；
- terminal delivery identity/acknowledgement 不保留 action 或第二份 administrative kind；
- Result owner acknowledgement 必须携带 exact owner-derived `ResultID`，Workspace/Run acknowledgement 必须拒绝该字段；
  最终 Run terminal fact/record exact 保存同一 `TerminalResultID`。terminal Restart 必须按 Run-owned generation选择 predecessor，
  从相同 replacement transaction materialize successor并保存 Run/Result 双 lineage，覆盖 duplicate、CAS cut、fresh reload、
  Result identity drift、terminal 不重开且不重复 delivery；
- target-grounding 0/1/N matrix 必须证明 semantic provider 前先取得 focus-scoped File-owner TargetSelection：exact current
  selection 必须回显 Definition selector/owner/subject/candidates 与完整 `ProspectiveTargetAllOfConstraintRefs`。其 freshness
  为 absent/ambiguous/unsupported，或同时无 bounded Definition candidates 与 owner-accepted mandatory set 的 empty-scope 时，零
  semantic/candidate provider call并交付
  `target_grounding_unresolved`；无 selection/observation、wrong focus/declaration/revision 仍为 transient；selected，或带
  bounded Definition candidates/exact owner-accepted mandatory set 的 empty scope，才继续 prospective semantic。subset、superset
  或 owner 不支持的 set 必须 typed fail-closed 且零 semantic provider。catalog retirement 后若旧 no-effect binding 尚未由
  typed `CWIBindingRetired` event 退休，该 provisional
  terminal 不得生成 SM-4 drive result、startup classification 或 delivery request，旧 binding 也不得执行；restart/duplicate
  必须只退休一次并使 CWI 重新 eligible。已有 effect credit 的 binding 不得被 catalog refresh 退休；其它
  contract/budget/owner terminal 必须原样交付，administrative termination 继续使用独立 lane；
- Run/Workspace/Result 不解释 Orchestrator decision，只执行各自 owner-local typed outcome transition。

### Feedback Preservation

- concrete Result 同时携带本 domain stable category 与 producer 的 audit-safe original diagnostic；
- original class/code/message/signature、field violations、target/revision、scope 与 details ref/hash 在
  normalize/clone/hash/persistence/EventHub/projection 中不被覆盖；
- provider adapter 的 typed protocol/transport/status identity 必须先于 error-text fallback；canonical
  `provider_protocol_failure` 等 terminal code 不得覆盖 nested original guard code，restart reload 后二者仍应分别可见；
- completion response guard必须从同一sealed provider ceiling/profile派生，并分别覆盖non-stream JSON body与SSE wire；
  SSE还要独立验证content/event/line limits和wire-limit error优先级，embedding使用独立input-profile bound；media type不匹配、
  arithmetic overflow或任一limit breach均形成typed protocol fact，不能通过提高Runtime budget或切换decode路径恢复；
- streaming timeout必须覆盖 response header idle、body idle、持续正数wire进展跨过一个idle interval仍成功、独立hard/caller
  deadline，以及每条失败路径的submitted/response-bytes/零partial-content事实；body idle必须保留
  `provider_body_idle_timeout` exact identity并走既有SM-3/SM-4 transient settlement，不得由adapter重试；
- operations request projection必须覆盖manifest引用的target-selection、capability-state与operation-admissibility snapshot
  缺失/内容漂移，以及admissibility owner/kind错配；所有这类完整性失败必须证明`providerSubmitted=false`，不能以空
  supporting context继续physical call；C0/C4指令中引用的字段路径必须与实际provider-facing分区一致；
- producer 没有 diagnostic 的普通 accepted fact 才允许 original 为空；
- unknown code 保留 original 并 typed fail-fast，不能默认 retry、CAS、repair、governance 或 terminal；
- core classification 必须只消费 typed feasibility/status、sealed operation descriptor 与 Profile declaration。测试需给出相同
  typed status + 不同 capability-private code/prefix，以及未知 typed status，证明私有码不改变 failure/repair/retry/governance/
  terminal，未知 status fail closed；静态扫描必须拒绝 reducer 引用 concrete capability owner 常量或其 diagnostic prefix；
- provider-safe correction projection 与 audit-safe original diagnostic 分开生成，互不覆盖；
- parameter-entry malformed corpus 必须覆盖 fixed declared parameter 被内嵌为 sibling 的聚合识别，以及 undeclared、wrong-type、
  duplicate/mixed ambiguity 不获得 re-encode authority；diagnostic/C4/aggregate 只保留 index/kind/name/type，不保留原值；
- credential、endpoint、raw prompt/provider body/stream、未脱敏文件内容只允许记录 hash/bytes 与
  redacted/omitted metadata，不得进入 aggregate、eventstream、projection、memory 或下一次 candidate context。

### Candidate And Request

- `JSONSchemaResponse=true` 只允许 adapter 发送可选 provider format；false/未声明必须省略。两条路径返回相同文档时，
  strict local decoder/schema/evaluator 结果必须完全相同；structured-output unsupported/failure 不得触发 adapter-local
  fallback/second physical call，也不能跳过本地 admission或直接产生 correction authority；
- incomplete document、fragment/EOF 与 transient transport 只使用 request-session recovery，不创建 attempt；
- streaming `response.incomplete/max_output_tokens` 是唯一 terminal fact；必须继续校验剩余 wire framing，拒绝 late event、
  第二个 terminal 与重复 `[DONE]`，且 partial bytes 只进入 bounded framing/audit；
- authentication/configuration/protocol/submission-unknown/document-recovery-exhausted 形成 factual result，不创建
  candidate attempt；
- LLM request result 必须是 `Stale | Feedback | DocumentOutcome` exactly-one；complete content 与 framing hash/bytes
  精确一致，其他 disposition 零 content；physical response mode 是 `streaming | non_streaming` 唯一事实，布尔 streaming
  仅允许派生；
- stream/non-stream 的 coherent `completed` + empty final output（包括 reasoning-only response）必须形成唯一
  `empty_document/provider_final_output_missing`；coherent non-JSON 必须形成
  `invalid_provider_output/provider_final_output_invalid`。二者均零 content、零 `Feedback(provider_protocol)`，验证 native usage、
  completed audit 与一次 usage event 无损保留，且不消耗 whole-document recovery。LLM owner/adapter 对本次 observation 零
  fallback/replay；只有 SM-3 持久化 typed rejection 后才可用新 request/physical identity 授权唯一 attempt 1；
- result audit 必须逐项匹配 workspace/run/logical/physical identity、target、kind、stable prefix 与 context budget；覆盖
  before-Do、Do error、response error、stream/non-stream/embed，并证明 request bytes 不推断 submission；DocumentOutcome、
  stale、transient/auth/config/protocol 的 submitted 矩阵与 unsubmitted 零 physical fact 必须穷尽；
- Runtime LLM usage event/read trace 必须无损贯穿 `logicalRequestId`/`physicalCallId`，并验证
  `physicalCallId=requestId`；generic non-Runtime trace 允许两者都没有，partial/mismatched identity fail-closed，且不得从
  request bytes 或 response mode 推断；
- provider settlement 必须覆盖 confirmed/released/unknown、duplicate/stale/tamper。验证 stale/unsubmitted terminal release
  全部 reservation；submitted usage 精确 confirm；无可靠 audit 进入 unknown 且零 provider replay；
- incomplete、transient 与 body-idle 必须覆盖同 CAS 的 physical settlement + SM-3 session + SM-4 transport budget + optional
  SM-2 terminal，以及 CAS-before/CAS-after/fresh reload；released/unknown 不得进入 recovery；
- provider terminal 必须用一次 owner-private aggregate CAS 原子完成 SM-3 session close 与 exact SM-2 failure；验证
  normal、duplicate、stale、CAS-before、CAS-after response loss 与 fresh reload，并断言这些 terminal crash/reload 分支
  零 provider replay；允许恢复的 incomplete/transient 分支仍以新 `PhysicalCallID` 请求同一完整 document；
- empty/invalid 的 submitted settlement、request-session close、typed document-shape rejection 与 correction/final 必须在
  owner-private CAS 中保持一致；SM-4 只能 settlement/re-drive，不能把该 observation 改写为 protocol terminal 或选择 correction；
- complete document 无论 provider 是否使用 JSON Schema 都必须经过 strict local admission 且只进入一个 evaluator，
  first-failure 顺序稳定；
- evaluator 只返回 accepted 或完整 typed rejection，不返回 correction/retry/terminal disposition；
- evaluator 拒绝同一 candidate 内重复的 durable `(OwnerModuleID, TargetRef)`；普通依赖边不能豁免，只有 owner-sealed
  exact `RequiredOperationSequence` 可按 requirement ref/variant 执行，同时证明不同 target、non-durable operation 与该
  owner-required sequence 不被误伤；
- SM-3 持久化 rejection 后才可形成 typed terminal 或唯一 `correctable_once` internal result；
- manifest semantic rejection 若保留严格解码 candidate，必须只存在于 failed attempt 的 private correction frontier；验证它不进入
  feasibility/accepted attempt/dispatch/authority。`bindingRefs` matrix 必须覆盖 exact operation slot/path、wrong binding、唯一
  allowed alternative、不同 semantic-shape hash、attempt-1/restart重验、无 alternative直接 authority exhausted，以及 framing/
  closed-schema rejection 保持零 persisted candidate；
- empty/non-JSON document shape、complete schema 与 semantic rejection 共用 candidate attempt 0/1；attempt 1 复用
  transaction、manifest、C0～C3 与 stable prefix，任意第二次 rejection terminal，零 attempt 2/第三次请求；
- operations provider schema/instruction/decoder 必须禁止 `id`、`dependsOn` 与 dependency graph；transaction 先执行 exact-match
  gate，strict local compiler 再从 manifest hash 与 manifest-owned authorization slot 确定性派生 identity/edges。required proposal
  按 requirement ref 唯一匹配 normalized required-sequence slot，auxiliary proposal 按 canonical execution-semantic key 派生 identity；
  provider 数组下标不参与 identity 或 dependency authority。任意重排、duplicate/restart 后派生结果必须稳定，provider 自造字段、
  ambiguous/duplicate/missing slot 必须 fail-closed 且零 dispatch；
- manifest→wire matrix 必须分别覆盖 required 与 auxiliary rule 的有/无 variant、reachable singleton binding、required
  cardinality-one singleton target 与仍可选多值；证明 unreachable variant authorization 不扩大 wire，schema、instruction、
  fixture encoder 与 strict compiler 共享同一 `RuntimeCandidateWireContract`，provider 只提供真实选择。
  JSONSchemaResponse on/off 必须接受同一最小文档；本地 kind/variant/binding/target/fixed parameter 回显、null
  container、unknown/trailing 字段、提前闭合 root 都精确拒绝且零 repair/effect；
- 条件必填 operation field 的 semantic diagnostic 必须以 `missing_<field>` 与 `Required=[<field>]` 表达；缺失
  `bindingRefs` 只能形成 `schema_field/supply_required_field`，不能同时形成 `evidence_binding/select_allowed_evidence_binding`。
  只有具体错误 binding 与不同 allowed member 才进入 semantic witness；CAS/fresh-load 必须拒绝维度不兼容的 failure/correction；
- wire identity matrix 必须覆盖 ExpectedDocument/request-session exact hash 正向、manifest 字段漂移、hash 缺失/篡改、
  duplicate、attempt 1、JSON round-trip 与 fresh restart。编译失败或重算不一致必须在 provider/feasibility/
  dispatch 前 fail-closed，不得以空 hash 或当前 renderer 形状继续；
- target authorization 必须覆盖同一纯 matcher 的纵向一致性：multi-target `exact_target` binding 中，不同 singleton
  operation 及其 exact matching receipt 均可通过 membership gate；空 target、越界 target、receipt/operation target 漂移必须
  分别在 candidate/dispatch 前或 receipt credit 时拒绝。dispatch seal 与 fresh-load invariant 必须重验同一规则，不能等到
  owner effect 后才发现 admission/credit 解释冲突；测试还必须覆盖 manifest hash、per-operation sealed preimage、operation
  hash、accepted-attempt independent projection commitment 与 `DispatchAuthorityHash` 的 missing/tamper/JSON round-trip/restart
  fence，包括 `{A}`→`{A,B}` 后协调重算 dispatch/attempt self-seal仍被 admission commitment 拒绝，以及 governance resume
  逐 hop exact 继承。`prospective_owner_target` 仍须额外通过完整同-dispatch collective
  receipt-set，不能因共享 matcher 而退化为首条 receipt credit；
- structured validation repair 必须独立验证三类 target。normal case 至少覆盖 validation operation entry `A`、matching
  `CompletedDispatch.OperationTargetAuthorizations` 的 canonical `Target.AllowedTargetRefs()={A,B}`、structured diagnostic
  candidate `B`：在 `existing_owner_scope` case 中该 finite set 同时等于 canonical `OwnerScopeTargetRefs`；entry 与 candidate
  分别是 full scope 的子集，observation `ValidationScopeTargetRefs` 与该 sealed full
  scope exact-match；candidate 与 entry 不相等、甚至不相交仍须通过 causation并进入 sealed Profile/descriptor 指定的
  resource/effect owner confirmation。expected scope 必须按
  exact `OperationID` + operation hash 从 completed dispatch 读取 `Target.AllowedTargetRefs()`，不得从 mutable manifest、current
  focus、owner inventory 或 operation entry 重建；
- operation entry cardinality 必须分别覆盖 sealed per-operation `RequireTargets=true|false`：前者 empty entry 在 owner call 前拒绝，
  后者不因 empty entry 单独失败；finite allowed set 存在时，任何非空 entry member 都必须属于该 set。structured validation
  negative matrix 还必须覆盖：observation scope 缺项/附加项，entry/candidate 任一越界或 candidate 为空，
  missing/duplicate target authorization，wrong operation hash，wrong dispatch/authority/operation/`FocusTargetRole`，以及 persisted
  authorization/observation tamper。所有分支在 repair-owner query、provider call、repair transition、budget consumption 前
  fail closed；
- dispatch execution 必须证明 `dispatch_execution_requested` 只做单次 SM-3 preparation/claim CAS 并返回 immutable call
  handoff，`ApplyInternalEvent` 内零 capability call/callback；owner receipt 之后由 evidence/stale/governance 三个互斥 private event
  或独立 outcome-unknown event 收口，并逐项验证 declared write-set、pre/post-CAS、duplicate/stale 与 fresh restart；
- accepted admission projection matrix 必须覆盖 target authorization 与 managed-validation recipe 两份独立 commitment：真实
  admission/duplicate/restart 正向，missing/hash mismatch，协调改写 dispatch projection + self-seal，以及 recipe ID、command、
  ordered args、timeout 漂移；governance multi-hop 必须逐跳 exact 继承。非 managed File/MCP candidate 必须保持空 recipe
  projection/hash；
- software-dev specialization 的 Shell `WORKORCH_FAILURE_TARGET` matrix 必须证明 closed/role/class/scope-bounded marker 只贡献 diagnostic candidate refs，receipt
  的 direct `RuntimeFailureTargetObservation`/`TargetObservation` authority 保持为空；只有 matching finite scope 才能形成
  `RuntimeValidationFailureObservation`，且必须继续等待 sealed repair-owner confirmation 与 SM-2 transition。missing/multiple/unknown-field、
  role/class mismatch、越界 marker 均不能形成 candidate authority；tool diagnostic 与 marker 的 canonical union 必须仍为
  1～8 项，`8+1`/`8+8` 越界 union 整组失效；
- managed-validation receipt matrix 必须在 commit 与 fresh-load 同时拒绝 direct diagnostic `TargetRefs`（即使它等于 current
  focus、operation entry 或 sealed scope member）、direct/cross-owner `TargetObservation`、出现在 File/MCP/durable Shell receipt
  中的 VFO，以及 workspace/run/actual dispatch/authority/operation/signature/role/class/candidate/scope 任一漂移。顶层与 nested
  observation/typed diagnostic 必须覆盖 presence 缺失与 canonical drift。合法 VFO 必须回指 completed/committed dispatch 中
  completed operation 的 `evidence_committed` execution，并 exact-match key/fence/ref/hash Shell receipt；owner-committed-only、
  invalidated dispatch、generic FailureObserved/DriveFailureObserved 与 governance-resume origin/current dispatch 混淆均须在
  commit/fresh-load 原子拒绝。pending VFO 保持零 concrete `RepairDirective`、零 `FailureTarget`。repair-owner confirmation 必须在
  ingress 与 reload 都拒绝与 sealed Profile convergence/source binding descriptor 不一致的 owner；任何 promoted owner-grounded
  target authority 必须反向匹配 exact confirmed record、matching `OwnerModuleID` 与 exact validation-owner receipt，generic
  failure ingress 与协调修改 receipt hash 不得旁路；
- descriptor-authorized receipt-evidence direct-target matrix 必须使用真实 capability owner/EventHub receipt 保留合法非 VFO
  正向路径（当前 concrete fixture 为 MCP），并在 commit、JSON round-trip 与
  fresh-load 同时证明 promoted/nested observation exact parity，completed committed dispatch/operation、`evidence_committed`
  execution、exact key/fence/ref/hash rejected receipt、唯一 `runtime-receipt|<receipt-hash>` rejected evidence 与 processed-event
  fence，以及 sealed/current exact catalog snapshot/acquisition/revision、subject selector 和 satisfied prerequisite observation。
  negative matrix 至少覆盖 generic ingress、owner-committed-only、missing/duplicate evidence、missing/duplicate processed fence、
  receipt ref/hash或 diagnostic drift、descriptor policy/owner/target-role drift、catalog dependency
  domain/owner/subject/revision drift、stale restart 与 coordinated tamper；所有分支均
  不得由后续 resource-owner snapshot、current manifest 或 terminal projection 补建原 diagnostic owner authority；
- evidence-commit reverse-proof matrix 必须证明每个 `evidence_committed` execution 都唯一绑定 exact
  `EvidenceFactID + EvidenceFactHash`，覆盖 accepted/rejected、active/completed、JSON round-trip、restart/duplicate，以及
  kind、revision、binding refs、CWI refs、failure/reason、receipt/operation lineage 的单字段和协调篡改；ledger commitment 不得
  进入 immutable dispatch authority hash，owner-committed execution 不得提前携带它；
- global evidence chronology matrix 必须覆盖 ordinary accepted/rejected、governance resolution/fact、assessment fact 在同一
  `EvidenceSequence` 中按 persisted completed-dispatch order 连续消费；active dispatch、governance-resume source-fact clone、
  JSON round-trip/restart 均不得重复推进。negative matrix 必须拒绝 revision gap/倒序/协调重写、orphan/duplicate、跨
  accepted/rejected 重复、assessment kind 混入 ordinary ledger、governance credit pollution，以及最终 sequence/
  `Facts.EvidenceRevision` 漂移；
- semantic provider/owner preimage matrix 必须从 active/history accepted batch 的 exact manifest、batch generation
  `RequestLogicalID`、document hash/content 与 archived/retained feasibility facts重放 `CompileRuntimeSemanticBindings`，并覆盖
  attempt-0 直接接受、attempt-1 correction→feasibility→seal、duplicate、multi-batch merge、response loss、fresh-load 与 settled-CWI
  generation history。attempt-1 ledger request cursor 与 batch generation identity 必须明确不同；seal 使用 cursor、batch/document
  identity 漂移均须在 owner/provider/effect 前 fail closed。negative matrix 还必须分别及协调改写 Focus/binding、CWI current/history refs、reconciliation、session
  manifest bindings、accepted-attempt bindings 与 active/completed dispatch bindings；同步重算下游 hash 仍须因上游 preimage
  不匹配而 fail closed；
- assessment authority/adjudication matrix 必须覆盖 DefinitionFocus 对 seed source 的 exact projection、pre-seal raw-canonical
  `RuntimeAssessmentAuthorityInput`（definition proof/owner-fact acquisition、Focus/binding/catalog）、post-seal deterministic
  adjudication 与 `RuntimeAssessmentFact` hash 双记账、repairable→satisfied / satisfied→repairable 协调改写、proof/fact hash
  drift、raw whitespace/order/duplicate/nil-empty、JSON/raw clone 与 fresh restart；post-seal adjudication不得进入 immutable
  dispatch authority hash；
- operations request-session manifest matrix 必须覆盖完整 raw-canonical CandidateManifest body 的 positive restart/duplicate，
  nil/empty、顺序、duplicate、whitespace、missing/duplicate session、body/hash drift，以及 attempt+dispatch target/coverage/recipe
  projection 协调改写；confirmed validation repair 还必须覆盖 exact purpose `confirmed_validation_repair`、closure、
  `RepairOwnerModuleID` 与三份 RepairCommitment exact inheritance，删除
  commitment/confirmation/directive及其协调删除、source receipt/evidence/fence/hash drift、governance resume 与 partial/rejected/
  completed restart；
- descriptor policy matrix 还必须证明 `sealed_operation_targets` 的无 observation refs 只能是 sealed operation target 子集，
  `validation_failure_candidates` 不能携带 direct refs/observation，`none` 全拒绝，并拒绝通过删除 observation、同步改写
  failure message/signature/refs 或注入已退役 prerequisite 来降级 receipt-evidence authority；
- repair-owner confirmation sequence matrix 必须覆盖 stale(v1)→confirmed(v2)、confirmed(v1)→late rejected/stale(v2)、rejected→late
  confirmed、exact response-loss replay 与 fresh-load coordinated append。允许 stale refresh，但 confirmed/rejected 是同一 VFO
  的终端 response；Runtime current failure/target 必须只匹配 latest valid record，不能从任意历史 confirmed record恢复 authority；
- confirmed validation repair transaction matrix 必须覆盖单项与多项 all-of：sealed Profile convergence 与 source binding descriptor
  各自确定唯一 repair owner 的正向路径，以及 ambiguous/mixed/wrong owner 的零 effect 反向路径；manifest purpose 必须为
  `confirmed_validation_repair`，三份 commitment 的 `RepairOwnerModuleID`、confirmation、每个 operation descriptor、receipt producer
  与 resource change owner exact-match。accepted durable changed receipt 的每个 operation target、target resource change、sealed
  target authorization 与 confirmed surface exact binding；workspace-tree revision chain 从
  confirmation revision 唯一连续推进，禁止按 operation ID 排序。首项 partial CAS、fresh-process reload、首/末 receipt duplicate 与
  final accepted→fresh-validation 都必须保留 closure 且不重复 effect；首项 partial CAS 后 mutable `FailureTarget` 与旧
  `need_state_change RepairDirective` 必须同时为空，current pending prerequisite 必须精确等于该 dispatch invalidation 的 prerequisite
  union。negative matrix 至少覆盖恢复旧 target/directive、额外/缺失 pending prerequisite、恢复 invalidated observation、第二 receipt
  复用原 revision、branch/detached chain、missing/duplicate accepted evidence 或 processed fence、payload/hash drift、partial member
  rejected、refresh-exhausted/dispatch invalidation，以及把 `need_validation` partial dispatch 误识别为 repair transaction；所有失败
  均不得恢复 stale repair-owner authority；
- confirmed validation repair governance matrix 必须覆盖 single/multi-operation、首项 accepted 后 pause、未完成首项直接 pause、
  `continue|fail`、fresh-process reload 与 resolution/receipt duplicate。continue 只能从 origin accepted admission、confirmation、
  source dispatch/candidate seal、completed receipt chain 与 exact binding/grant 生成同一 candidate 的 next-fence resume；不得重新请求
  candidate、增加/替换 operation、恢复旧 target/directive 或让 repair owner 写 Runtime state。accepted receipt、governance resolution 与后续
  receipt 必须消费同一全局连续 evidence revision；测试需协调更新 execution fact hash 后单独漂移 revision，并同时证明 aggregate
  ledger 与 owner-neutral repair transaction reverse proof 都 fail closed，不能依赖某个 validator 的先后次序或 diagnostic 文本；
- selector-only managed-validation authorization 的 canonical `Target.AllowedTargetRefs()` 为空；operation 可按 sealed
  `RequireTargets` 与 owner feasibility 执行，但必须向 owner 传递空 `ValidationScopeTargetRefs` 且零 scope-bound structured repair
  observation。manifest/current focus 漂移不得把它补成 finite scope，无 observation 的 rejected validation 只走 Profile 已封存的
  matching semantic fallback；fallback 缺失时 typed fail closed；
- invalid/empty raw、reasoning、decoder message 与可逆片段不得进入 EventHub durable payload、aggregate、eventstream、projection、
  memory 或 C4；只允许 owner-private audit carrier 与 bounded hash/bytes/ref。attempt-1 C4 仅包含 manifest-bound
  document-shape/schema/parameter-entry/semantic witness，不含 raw、Disposition、TerminalCode 或 retry 选择；parameter-entry
  attempt 0 必须零 probe/dispatch/effect且不扣 correction budget，fresh-load attempt 1 仅提交一次 provider/probe并只扣一次 budget，
  corrected document 仍须通过完整 strict admission；
- duplicate result、stale transaction、CAS-before/CAS-after response loss、fresh-process reload 与普通 Resume 必须覆盖 attempt0
  rejection/directive/request identity 幂等：相同 observation 只结算一次，attempt1 最多提交一次，provider/attempt/token budget
  不重复消费；document correction 不借用 whole-document/transport/repair/governance budget；
- owner feasibility 不进入 candidate correction 或 attempt ledger。
- request session + authority transaction + canonical state/RequiredOutput + DocumentKind/ExpectedDocument + provider
  ceiling/reasoning/budget/stable prefix构成唯一purpose-bound semantic invocation seal；不得出现独立SemanticActivation
  repository/lifecycle或assessment通过operations envelope升级role。
- owner-fact acquisition manifest 必须同时投影 immutable terminal evidence kinds 与 current acquisition evidence kinds；
  schema、decoder 与 binding compiler 对 terminal `validation` + current `RepairOwnerModuleID` `state_change` 给出同一解释，且 C0～C2 bytes/hash
  不因 current acquisition 改变；acquisition 不携带 terminal metric/artifact credit。
- `prospective_owner_target` declaration/obligation/projection 必须具有同一 nonempty
  `ProspectiveSatisfactionContractRef`，其它 policy 禁止携带；manifest 分离 provider-selectable `OwnerCatalogs` 与
  `OwnerFactDependencyCatalogs`。dependency catalog参与hash/freshness/CAS/seal但必须从 C3、allowed operation/surface、
  schema enum、correction values中完全缺席。required owner catalog 还必须发布唯一 provider-safe typed
  `ProspectiveTargetSatisfactionConstraint`，只能表达 `all_of` mandatory clauses，每个 clause 内也只有 conjunctive
  predicates；同一 constraint 封存到 acquisition/manifest/C3，dependency recipe identity/command 不进入 provider，且不存在
  `one_of`/`any_of`/fallback 分支。
- composite prospective probe必须覆盖：File outer feasibility生成exact base-workspace-tree-revision-bound immutable delta且
  零文件/目录写入；同一 complete manifest revision 上 1～8 个 prospective refs 的规范化、去重、路径逃逸、已存在、
  parent/child 冲突、excluded tree 和 aggregate capacity 任一违例都必须零 delta/零写入；Shell 只消费该 delta、
  activation-sealed recipes 和自身 dependency catalog/observation，逐 surface 校验 recipe command/ordered args/timeout schema
  identity，并只基于 resulting manifest 判定，全程零 workspace read/process/session/effect；Orchestrator 不解释 path/recipe。
- observation CAS与semantic seal分别重验manifest/batch/request/contract ref、delta、File catalog/observation与Shell dependency
  catalog/observation；probe→CAS、persist→seal任一漂移均零binding/dispatch/effect。duplicate/restart使用full composite hash
  幂等，冲突payload拒绝。
- semantic eligible-scope 矩阵必须覆盖：普通 generation 的全局 current eligible order；
  `validation_failure_repair` 只含 exact current CWI；repair 期间无关 CWI 变为 eligible 不退休 manifest、零额外 CAS/request；
  普通 generation 的真实新增 eligible member会退休 exact generation；accepted-order projection 与 focus-advance retirement
  对同一 purpose 得到相同 closure。还必须覆盖 owner-private CAS 恰好退休 manifest 的 cut point，断言 caller 从 returned
  aggregate 重新分派且不会读取旧/nil manifest、不会重复 provider/effect。
- pre-seal Definition 不携带 exact prospective target set，compiled obligation/manifest 也不回填；对没有 bounded exact candidate path 的开放 scope，SM-3
  必须另行封存 normalized mandatory `ProspectiveTargetAllOfConstraintRefs`，并在 Definition/Profile hash、target-selection
  command/result/snapshot、proof obligation/C3、binding 与 reconciliation hash 中逐 hop exact 保留。proposal
  `ProspectiveConstraintRefs` 必须 exact-equal；缺项、扩项、duplicate normalization drift 与 restart raw-clone alias 均需
  回归覆盖，且 rejection 发生在 owner probe 前。
- pre-seal Definition 不携带 exact prospective target set，compiled obligation/manifest 也不回填；SM-3
  schema/admission 只接受一份 proposal 中 1～8 个唯一 canonical `ProspectiveTargetRefs`，不接受 singular
  `ProspectiveTargetRef` 兼容字段。集合中每项都必须位于 Definition declaration selector/candidate refs 与 current
  operation selector/owner constraints 交集；非空 candidate refs 要求逐项 exact member match。每个 owner `all_of` clause
  至少由一项完整覆盖，且每个 target 至少覆盖一个 clause。partial/extra/duplicate/mismatch 在 document
  admission/schema correction 阶段形成 typed field violation，零 File/Shell probe。File/Shell fact 绑定同一 proposal
  hash/target set，只有 SM-2 sealed binding 形成 authority；operations schema 只把 binding ref 作为选择面，并为
  每个 target 要求一个 `RequiredOperationSequence` member。每项必须带 exact singleton `TargetRef`，要求齐全、
  无额外项并按 canonical dependency chain 执行；candidate/dispatch 只从 binding 派生并消费 target。
- semantic target-policy matrix 必须对 `exact_target | existing_owner_scope | prospective_owner_target` 使用同一纯 admission
  intersection：proposal 既不能越过 Definition selector/candidate declaration，也不能越过 current operation
  selector/catalog surface。Definition selector 不能被当成 operation selector；任一 policy 的 selector、candidate、target 或
  owner scope drift 都必须产生 typed proposal violation，并保持零 binding/dispatch/effect。
- multi-target prospective receipt gate 必须逐 cut point 验证：第 1..N-1 个 operation 的 owner-committed accepted exact
  receipt 与 matching `evidence_committed` execution 只增加同一 dispatch 的 persisted progress，`ActiveDispatch` 保留，CWI
  仍为 unsatisfied/unsettled，focus/collective credit 不推进；第 N 个 matching member 才允许 Orchestrator private event 在同一
  CAS 中 complete dispatch 并 exactly-once 认可 collective proof。无 assessment 要求且其它 gate 已满足时可在该 CAS 中
  credit/settle CWI；assessment-required 路径必须继续等待独立 SM-2 assessment gate。每个 member 必须逐字段匹配 dispatch、
  binding、evidence kind、operation、`ExecutionKey + Fence` 与 singleton target。
- rejected/stale/weak/mismatched member、partial set、同 target 冲突 receipt 都不得产生 collective credit；exact duplicate
  receipt/evidence event 必须零新增 member、revision、effect、credit 或 settlement。partial-receipt crash/reload 必须复用同一
  sealed dispatch，已完成 operation 零重放，只执行缺失 member；来自不同 dispatch、repair/semantic generation 的 accepted
  receipts 即使 target union 完整也不得拼接成 proof。aggregate invariant 必须拒绝任何“partial set + satisfied/settled CWI”
  的持久化 current shape。
- Shell typed stale必须作为nested owner-authored observation走一次bounded prerequisite refresh，不能伪装成File outer stale或
  直接terminal；missing dependency在Shell call前退休generation。stale 不改变 semantic attempt/correction
  directive/rejection ledger，duplicate/restart 不重复扣减 refresh，也不消耗 correction。
- `would_not_satisfy` 必须先持久化唯一、exact-current 且完整的 File delta + Shell nested observation composite；
  partial/forged/stale/mismatched proposal hash/target 均不得 terminal。完整结果因与 admission 已验证的 published constraint
  矛盾，必须一次性记录 `prospective_satisfaction_contract_violated` 并形成 `semantic_contract_failed`；不得生成
  replacement target、`Allowed`、第二 provider request 或第二 owner probe。
- attempt 1 同时覆盖 document-shape、普通完整 document/schema 与 semantic correction：复用
  transaction/manifest/C0～C3/stable prefix，C4 只携带 manifest/CWI/field-path-bound typed violation，不复制 constraint
  或 raw provider content；第二次任意 rejection 仍为
  `candidate_retry_exhausted`。duplicate composite、CAS-before/CAS-after response loss 与 fresh-process reload 必须证明
  correction/terminal 只消耗一次，同 proposal 的 owner probe 不重放；全部分支零 ticket/governance/acquisition/
  operation/effect/coverage，File/Shell result 不含 SM-2/SM-3 target state。
- Definition-gap complete document 在 owner-private transition 前必须通过 pure manifest/expected-document admission；已有 sealed
  acquisition 的 `undeclared_prerequisite` 只产生 attempt-0 typed correction，零 gap observation、governance、ticket、dispatch、
  effect 或 Definition mutation；stale identity 同样不能越过 admission。

### Capability And Receipt

- File/Git/Shell/MCP/SSH/Web 各自拥有 emitted-code total mapping，共享 pkg 不维护全 owner code taxonomy；
- Web execute必须校验Orchestrator producer、完整sealed operation、current claimed receipt fence与exact grant；payload/execution/
  grant drift保持零network call。search redirect同origin，fetch DNS绑定actual dial并在redirect前重验；两者`at_most_once`且
  receipt loss不重复external request；
- feasibility 只返回 feasible/stale/prerequisite_required/infeasible factual result；
  `prerequisite_required` 仅在接收 owner 已定义对应 typed prerequisite acquisition command/result 时合法。
  当前没有该合同的 semantic proof owner 必须保留完整 original diagnostic 并 fail-closed（通常为
  `infeasible`/`unverifiable`），不得把它伪装成 revision stale、退休有效 catalog 或消耗 refresh budget；只有
  携带并精确匹配 expected/observed revision fence 的 `stale` 才能进入有界 catalog refresh。
- operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，不存在跨 EventHub `Status + Outcome` 双权威；
- live response 与 restart receipt recovery 使用同一 outcome-first judgment；rejected receipt 的 owner diagnostic 不得被
  `ObservedEffectScope=unknown` 覆盖为 effect mismatch，accepted/no-op 仍必须通过 effect 与 quality gate；
- durable no-op 使用 `Outcome=no_op`，删除重复 `NoOp bool`；read-only/validation 的正常零 durable effect 仍为
  accepted，`Changed` 只作 observed effect metadata；
- accepted receipt 不携带 failure，rejected/unknown outcome 保留完整 safe original diagnostic；
- durable effect 使用稳定 ExecutionKey/Fence/owner receipt，restart/duplicate 不重复副作用或改变 diagnostic hash；
- File/Shell/Git fresh chain 必须使用 owner catalog 的 exact kind/variant/target policy；File effect 退休 catalog 后，Shell
  从 fresh File-owner inventory 重获 target vocabulary，不把空 scope 当创建授权；
- software-dev specialization 的 MCP owner failure-target + File repair receipt 矩阵至少覆盖 `file_write|text_replace|unified_diff` × CAS-before/
  CAS-after-response-loss；重启复用 exact owner receipt，MCP remote call 与 File durable effect 均不重复；
- stale/resource revision 独立于 failure/candidate budget；accepted resource change 不消耗 recovery budget。
- `need_validation` 前的 validation discovery 缺失必须恢复为 matching typed prerequisite，而不是 capability-unavailable feasibility；局部 target、
  failure-surface 或 provider preflight 必须保留全部既有 pending owner debt，且不得把 satisfied catalog 重编译成新 ID。

### Failure, Recovery And Governance

- 只有 repository expected/observed revision 不一致产生 typed concurrent-state conflict 并消费 CAS reload；
- semantic reject、stale identity、candidate session conflict、duplicate/no-op 不得借 `common.ErrConflict` 进入 CAS；
- provider transport、operation outcome recovery、delivery retry 使用独立 persisted ledger；
- failure-authority matrix 必须覆盖唯一 `replaceRuntimeFailureAuthority` production writer 与三种显式 policy：bounded interlude
  exact保留；retire 同时清空旧 `FailureTarget + RepairDirective`；replace-from-exact-owner-fact 要求新 failure/target 的
  signature、canonical targets、observation逐字段一致并清空旧 directive。至少覆盖 owner infeasibility、targetless rejected
  receipt、provider/candidate/governance/terminal failure、confirmation replacement、duplicate/restart 及协调 tamper；任何普通 failure
  不得继承上一 lineage 的 repair authority；
- operation recovery 必须覆盖 `recovery_budget_consumed` 在同一 CAS 中增加 counter 与 exact
  `RuntimeOperationRecoveryAuthorization`、`operation_recovery_claimed` 的 `issued -> in_flight`，以及 receipt/call-return 的
  `in_flight -> settled|uncertain`。矩阵至少覆盖 budget/claim CAS 前后、pre-call crash、physical replay 前后、response loss、
  claim lease expiry、fresh-process reload、双 worker、exact duplicate、wrong claim/operation/authorization、counter/ledger 缺项或
  乱序、duplicate identity、wrong dispatch/key/fence、orphan execution 与 forged state；只有成功 claim exact matching
  `in_flight` 的 caller 可获得一次 physical-call handoff，active/expired/uncertain/settled/retired record 与 process-local counter
  均不得单独授权 effect；
- governance wait/continuation lifecycle handoff 与 terminal delivery 必须分流：前者以 exact persisted governance fence 和
  SM-4 drive lease等待 Run-owner acknowledgement，后者才可消费 terminal-delivery ledger；Definition clarification 必须与
  普通 Decision/Approval 一样覆盖 bind-CAS、duplicate、stale、response-loss 和 fresh reload；
- lifecycle exactness 必须覆盖 wait fence 无 resolution、continuation fence/Run-private reducer/ack 的 exact `ResolutionID`、
  active duplicate、旧/错 resolution 零写入，以及 successor 已消费、Runtime 已进入第二 blocker 后旧 exact resolution 的
  historical no-op（零 Run mutation/CAS/timer）；还必须覆盖 resolution 在旧 drive 成功退出前封存 lease 的交错，证明 timer
  不绕过 gate、gate owner 退出时挂载 exact persisted generation、duplicate/stale/Stop/fresh restart 均不遗失 successor且
  `TerminalDeliveryAttempts` 保持为零；
- governance resolution exactness 还必须覆盖 `continue|fail` CAS 原子替换旧 lease、Fail 首次/partial
  Result→Workspace→Run acknowledgement、同 resolution response-loss duplicate、Stop/fresh restart，以及旧 timer 对
  same-signature newer generation/different-signature successor 的 gate-exit handoff。renewal schedule CAS 与 fallback
  recovery CAS 同时失败时必须证明 running lease 未被 release、本地 gate 未丢失、普通 ingress 零第二 drive，fresh startup
  只恢复一个 successor；timer-vs-Stop 必须在 race 下分别阻塞 claim-return 与持 lifecycle read lock 但尚未 mount 的 handler，
  以及阻塞 bounded BackgroundRoutine submission 后再执行 already-due queued callback，证明 cancel 点后零 observe/claim、旧
  epoch defer不挂 zombie successor、不清理新 epoch gate、Stop及时清空本地 map/context且不删除 persisted lease；
- governance ingress 必须覆盖 Decision/Approval wrong-source在 authority load 前拒绝；same ResolutionID+snapshot 的 Summary、
  EvidenceRefs、outcome、ticket/approval、dispatch、dependency、subject任一漂移均 conflict/零 CAS。首次 stale diagnostic 与其
  exact replay必须持续返回 stale且不触发 Run/timer，stale payload漂移同样 fail-closed；同时静态/合同测试必须证明只存在
  ticket-resolution 与 approval-resolution 两个 concrete ingress，generic governance-resolution topic/command/handler 不存在，
  且 concrete adapter 必须先从 persisted request 重建 exact binding；
- recoverable runtime wait只有在typed owner recovery/reconciliation合同、condition、trigger/deadline和budget完整时合法；
  matching trigger幂等re-drive，stale/duplicate trigger零迁移，deadline/budget/guard耗尽typed terminal；
- wait不新增SM-1 state，不创建Decision/Approval，不由Web/TUI/projection/普通Resume驱动，且不存在
  `active + nonterminal + no persisted successor`；
- repair 只读取 typed gap/category，不解析 message、stderr/stdout 或 summary；
- retry/repair exhaustion 与 repeated failure 形成 typed terminal/no-progress fact，本身不能创建 ticket；
- governance 只来自独立封存的 `ExternalDependency/GovernanceRequirement`，不存在无信息 Decision/Approval；
- projection、TUI、Web、eventstream 不根据 error/message 自动 Resume、ticket、action 或 owner state change。

### Execution Budget

- 每个provider physical call、candidate correction、semantic generation、capability operation、repair/reopen和recovery
  consumption都绑定exact logical identity与唯一sub-ledger；
- allocation先于physical call/effect，allocation失败保持零外部动作；
- duplicate/stale/result loss/fresh-process reload不重复扣减；confirmed、unknown/reserved和remaining可确定重建；
- accepted resource invalidation、projection refresh、duplicate drive和CAS reload不消费业务预算；
- unknown consumption必须先走owner reconciliation或bounded wait，不能按zero重放；
- exhaustion只能进入合同定义的wait或typed terminal，不能产生治理ticket；
- product `RuntimeExecutionBudgetView`等于sub-ledger纯派生结果，任何UI/API写回尝试必须拒绝；
- Restart的新budget来自fresh policy，predecessor消费不复制为successor authority。

### Coverage, Freshness And Closeout

- canonical ledger到产品projection的mapping分别产生coverage、assessment、evidence三个维度，不能合并为overall enum/score；
- 内部`partially_satisfied`只关闭明确satisfied CWI；其余逐项仍投影为unassessed/unsatisfied/indeterminate；
- `indeterminate`在当前all-of基线下始终不通过terminal gate，且保留reason、missing evidence/owner fact；任何
  Definition/Profile/LLM/client声明的成功例外都必须在admission或candidate校验时拒绝；
- owner-fact acquisition 的 accepted receipt/assessment 只结算当前前置 transaction。reobserve missing 必须按 actual operation
  耗尽 one-shot budget；reobserve ready 必须在 `assessment_observed` 单一 CAS 中保留 audit/attempt lineage、保持 CWI open、
  重开并退休 generation，successor manifest 恢复原 terminal proof。两者都不得直接产生 terminal credit 或 ticket，fresh
  reload 不重复 CAS/effect/request；
- candidate/accepted/rejected/stale-or-superseded由receipt judgement、diagnostic和binding/freshness lineage纯派生；
- evidence只绑定实际依赖的owner revision/snapshot/fence set/hash；无关owner mutation不扩大stale范围；
- successful closeout包含Definition/Run lineage、逐CWI coverage、accepted evidence、assessment、deliverable/artifact、
  applicable owner revision set和known limitations；Result owner不重判或补字段形成成功。

## Workspace, Restart And Concurrency

- workspace-scoped command/result/feedback 携带 workspace/run，适用时携带 authority/dispatch/operation identity；
- component instance、Service、global/cache/background task 不保存未经 workspace scope 约束的“current workspace”；
- 至少两个 workspace 并发 candidate、receipt、recovery、delivery 时不串 identity、diagnostic 或 singleflight；
- CAS 前/后 crash、owner commit 前/后 crash、response 丢失、duplicate/stale/乱序均零重复副作用；
- Orchestrator Module lifecycle 必须覆盖 Setup 缺 EventHub/`BackgroundRoutine` fail-fast、`Run(ctx)` 绑定 framework context、
  startup inventory 中途失败撤销全部 ingress/context、重复 Start/Stop、Stop cancellation/deadline、已排队 handler/drive 与
  already-due timer race。cancel 点后不得新增 task/context/timer 或 zombie successor；Stop 只清 process-local gate，不删除
  persisted lease/operation authorization，fresh Start 必须从 Run inventory + opaque aggregate 恢复；该 lifecycle 不得被登记为
  SM-1 transition或新的 SM-4 authority。还必须验证 startup priority 保持 Orchestrator 先于 Run，而独立 teardown priority 保持
  Workorchd → Orchestrator → Run/capability 的依赖释放顺序；
- Orchestrator worker-exit fence 必须注入 drive body panic、panic 后 recovery-lease CAS 成功、CAS-before/CAS-after response
  loss、recovery CAS 失败、timer registration/submission/mount 失败、persisted-lease preflight 的 load/recover/mount 失败、
  duplicate ingress、Stop 与 fresh-process restart；证明 panic 只形成稳定 typed owner-local failure，
  exact successor 至多一个，provider/owner durable effect 零重复，且任何 active nonterminal 退出都不会同时失去 gate、timer、
  persisted wait/handoff 与 `RuntimeDriveLease`。panic 原文不得进入 authority identity。
- bound governance request 已持久化、Run wait fact 尚未送达这一 cut point 重启后必须自动收敛到 exact blocked Run；不得
  依赖人工 Resume，且整个非终态 handoff 的 `TerminalDeliveryAttempts` 保持不变；
- 任意非终态 aggregate 携带非零 `TerminalDeliveryAttempts` 或 terminal acknowledgement 必须以
  当前 invariant 校验 fail-closed，不能继续挂 lifecycle lease、由projection归零、生成兼容分类或伪装成SM-2 repair；
- governance resolution CAS 已成功但 continuation fact 尚未提交时，fresh startup 必须识别
  `blocked + persisted resolution` 并重投 exact continuation；continuation owner commit 后响应丢失必须作为 duplicate 收敛。
  同一 lifecycle lease 连续失败必须证明 running generation 原子续排为新 scheduled generation、`dueAt` 退避、旧 release
  无法删除后继、fresh controller可重载，且 terminal-delivery budget 始终为零消费；
- Run inventory 只返回 SM-1 state + opaque record presence/revision/ref；Orchestrator load 后 owner-local 分类
  `active_no_dispatch`、`active_sealed_dispatch`、`blocked_governance`、`recoverable_runtime_wait`、
  `terminal_delivery_pending` 并自动恢复；
- 不出现 `active + nonterminal + no scheduled drive`，也不使用人工 Resume 作为恢复机制。
- Workspace 删除必须证明 Run `draining -> retired` 与 Orchestrator `quiescing -> quiesced -> evidence-fenced` 分属各自
  owner-private reducer，Workspace只持久化协调 phase和typed ack；这些资源状态机不得标成第五 Runtime SM，也不得由
  Application、projection或对方 owner推进。
- Run `draining` 与 Restart/Create/普通 SM-1/Step/Checkpoint mutation共享 generation-head线性化点并全部拒绝后者；
  仅 `kernel/orchestrator` 的专用 opaque Runtime CAS drain lane可继续。Run finalize必须精确比较 deletion identity、
  generation、每个 Run revision/terminal state及 Runtime ref/revision/hash，再原子升级 `retired`；CAS-first/token-stale与
  finalize-first/CAS-rejected 两种顺序都必须零越权写入。
- Run `draining` fact必须在 Orchestrator 的 drive、timer、mount、internal-event入口形成 typed admission：只允许 exact
  terminal-delivery acknowledgement及其 delivery lease claim/recover/release drain；SM-2/SM-3、普通 drive、非 terminal
  lease、budget、provider、capability与evidence mutation全部拒绝。Orchestrator Begin与这些入口共享 write barrier，只要
  local activity、persisted lease或未确认 delivery 非空就必须零 durable gate返回 conflict。完全排空后才持久化
  `quiescing`，且 gate 后所有 Runtime mutation一律拒绝。terminal Run commit后 Runtime ack CAS pending/response loss、
  scheduled/running terminal lease及 fresh-process restart都必须先在 `draining` 区间收敛，再产生同一完整 token和
  retired fence。
- Run 的 recovery inventory、readiness 与 exact opaque Runtime-state read 都只接受 `kernel/orchestrator` source；forged
  Application/owner source 必须在 repository read 前拒绝。`draining` 可为 terminal drain 返回 exact record，`retired`
  必须从 recovery inventory消失且 exact read返回 NotFound，不能借 Setup-phase 私有 topic 绕过 Run/Step/Checkpoint 隐藏。
- quiescence token必须稳定覆盖有序逐 Run generation/Run revision/Runtime ref/revision/hash；仅 workspace/deletion/generation
  hash不合格。duplicate返回同一 token，stale/source-forged/cross-workspace token零状态变化。
- Run 与 Orchestrator evidence 的 purge receipt 绑定首次观察计数；测试必须覆盖 `pending` receipt 先于物理删除、删除后
  且 owner aggregate `purged` CAS 前崩溃、fresh-process 重开继续清理，以及完成后 duplicate 精确重放原 non-empty
  observations/proof。重试从空 surface 生成新的 zero-count proof 不合格；journal 只能是对应 owner 的私有事务技术状态。
- 每个参与 Workspace 删除的资源 owner 都必须用静态边界测试证明 command adapter 不直接调用 purge repository、
  `WithWorkspacePurgeFence` 或物理 fixed-point sink；这些 effect 只能出现在 owner-private `purge_requested` dispatcher，
  成功后再提交 `purge_completed` CAS。崩溃、重复与 stale scope 测试必须证明不会出现第二条 transition/effect ingress。
- current-baseline Restart 必须先 CAS predecessor `ReplacementIntent`，再完成 Result→Workspace→Run terminal delivery，
  最后由 Run-private reducer 幂等创建/激活唯一 successor；其他组件不能提前创建或链接；
- runtime clarification Restart 必须覆盖 Definition revision/StartupInputHash 的同 seal、仅 revision 变化、仅 hash 变化与两项均
  变化矩阵；前三类返回 typed `governance_required/definition_clarification_pending` 且零 CAS/跨 owner command/recovery，只有最后
  一类允许持久化 replacement intent；
- replacement 的 clarification resolution、administrative termination intent、SM-4 handoff acknowledgement 与 successor
  materialization 使用 Run-owned persisted stage/state/generation/attempt/dueAt lease；每一步先claim、failure续排、Stop/fresh
  restart恢复；Run terminal 必须经 private SM-1 delivery event。进程内task不得成为唯一后继，
  duplicate/stale generation不得重复Decision/Orchestrator call或successor create；
- terminal delivery 已提交、successor 已创建但 lineage 未提交、lineage 已提交但 successor 尚未激活三个 restart cut point
  均必须在进程重启后收敛到同一 successor，重复 recovery 不得再次创建或激活；
- unknown/retired field、当前路径之外的旧目录或当前不变量缺失不能进入持久化读取主线；当前路径上的 decode/validate
  错误直接失败，不写 owner transition/replacement intent，不调用 provider/capability，也不投影替代状态；
- 首次 `file/write` 创建成功后若同 CWI reopen，latest owner catalog 必须使 correction 选择 revision-bound
  `file/edit`/`patch/apply`；重复 create-only write 在副作用前拒绝且不能形成无限 revision/等价操作循环。
- profile verifier 必须证明 validation entry 只保留 recipe/receipt lineage：无 owner-supported 结构化位置时零 failure-target
  candidate；支持的工具诊断只能报告 sealed validation scope 内的 actual target，越界目标丢弃且仍需 sealed repair-owner
  confirmation。
  `WORKORCH_FAILURE_TARGET` marker 只能进入同一 bounded diagnostic-candidate path，不能直接产生
  `RuntimeFailureTargetObservation` authority。
  repair owner 的 current manifest 只确认 candidate freshness，不重定义 completed-dispatch full scope。software-dev specialization
  可用 File manifest 作 fixture。回归还必须覆盖：dispatch
  seal/持久化保留 `FocusTargetRole`、per-operation target authorization 与 operation hash；current Focus/manifest 漂移不改变
  completed-dispatch causation；observation receipt 到 repair-owner confirmation 之间零 CWI reopen、零 repair semantic input、零
  validation-only retry 和零 provider call；
- duplicate/restart matrix 必须覆盖 observation persist 前后、repair-owner query 前后、confirmation CAS 前后与 response loss。exact
  duplicate 只能复用同一 operation entry、sealed full scope、candidate、diagnostic/causation identity；已持久化 confirmation 的
  replay 为零 CAS，pending confirmation 只能重发相同 read-only query。fresh load 必须从 completed dispatch matching
  authorization 恢复 scope 并重验 subset/exact invariants，不能读取 current manifest 补 scope；payload 漂移 conflict 且零
  repair/provider/effect/budget。stale/rejected repair-owner disposition 分别只进入 prerequisite/typed terminal，confirmed 只由 SM-2
  private event 封存 `FailureTarget`。

## Verification Entry

按变更范围先运行受影响 contract、owner 和 Orchestrator package tests，再执行仓库级 `make verify-boundary-contract`
以及 AGENTS/Makefile 规定的 aggregate test、race 和 vet gate。具体 package/命令清单以源码、Makefile 和 CI 为执行 SoT，
本合同不复制一份易漂移的 inventory。

Boundary verification 必须覆盖：共享 package transition policy、verification-only owner manifest 完整性、全 owner code 表、
error-text routing、Action/Output.Kind 与 Status/Outcome 双权威、terminal action、Runtime reducer-owner/role 漂移、
secondary owner 直接 state write、DoD definition 的 draft/satisfied 影子状态、通用 Envelope/map/any/JSON/reflect bridge、
scope 缺失及禁止 raw provider 内容。Orchestrator 专项门禁还必须拒绝绕过 failure-authority writer 的直接字段写入、
operation-recovery counter 与 authorization ledger 脱节、generic governance-resolution ingress，以及 core reducer 对 concrete
capability owner/私有 diagnostic prefix 的依赖。它还必须拒绝workorchd `biz/execution`反向依赖protocol `service/server`，并拒绝
有效verifier把retired application scheduler/watchdog/continuation state、metric或automatic successor重新定义为正向证据；
退休metric的显式“必须不存在”负向断言不属于该禁止范围。
测试选择为空或只命中 projection-only wrapper 不能算通过。

任何以 graph step、checkpoint 或 incremental artifact projection 为对象的 wrapper，只能验证非权威读侧，不能替代
上述 Runtime typed contract 测试或重新定义通过标准。

## Deployed Evidence Boundary

Core assertions进入 deployed environment时仍必须使用 fresh data、latest build 和至少两个隔离 workspace，且不得人工
修改 owner data/schema、重复 Resume、提高 budget、放宽 decoder 或启用第二执行路径。服务启停、HTTP、projection、
filesystem 和 provider evidence 的环境编排由 [Service Acceptance](../../50-product-interface/service-acceptance.md) 负责；profile-specific artifact、
validation 和 DoD proof 由对应 profile verifier负责。它们只能提供本合同断言的 deployed evidence，不能重新定义 Runtime
transition 或通过标准。

deployed authority verifier读取Workspace/Run/Decision owner事实，并把workspace snapshot中的matching immutable
`runCommand accepted|rejected`视为execution-admission completion。该投影不是第五状态机；后续Runtime progression仍由
owner state与SM-4 aggregate evidence证明，不得以readyz scheduler diagnostics、continuation readback、watchdog recovery或
自动successor替代。

## Boundary

core verifier 不应验证：

- software-dev 特定测试命令、文件名、语言栈或项目结构；
- profile/Soul/Skill/MCP 的具体策略文本；
- 禁止的 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch、graph checkpoint 或自动 successor；
- 一次性 workspace/run 记录或 raw runtime/provider payload。

## Source Documents

- [Runtime Stage Contract](../runtime-stage-contract.md)
- [Runtime Request And Cache](../runtime-request-cache-note.md)
- [Recoverable Wait](recoverable-wait.md)
- [Execution Budget](execution-budget.md)
- [Result Closeout And Requirement Projection](../../10-domain-model/result-closeout.md)
- [Definition Verification](../definition-convergence/verification.md)
- `internal/modules/kernel/orchestrator/readme.md`
- [Service Acceptance](../../50-product-interface/service-acceptance.md)
- [Software-Dev Profile Verification](../../40-profiles-capabilities/software-dev/verification.md)
