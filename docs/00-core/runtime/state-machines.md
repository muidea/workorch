# Runtime State Machines

Status: normative

Type: core-contract-part

Last Verified: 2026-09-04

## 四个协调状态机

每个 authoritative state type 只有一个 owner；同 owner 可以用多个私有 transition 协调，但每次状态变化都只能由该
owner 的 private internal event 触发。跨 owner command/result只交付intent或fact，不能携带target state、调用对方reducer或
直接修改对方aggregate。以下任何 handoff 都不构成共享 owner。

每个 Orchestrator private event 必须声明唯一 primary SM 和唯一 concrete payload family。若该事实需要协调同 owner 的多个
SM ledger，其完整写集必须在一个 package-private transaction 中经同一次 aggregate CAS 提交，并由 cross-machine invariant
整体拒绝 torn shape；primary 分类不授权在另一个 CAS 中补写 sibling machine，也不把协调事务变成第五个状态机。

### SM-1：Run lifecycle

```text
pending -> active <-> blocked_on_decision | blocked_on_approval
active -> completed | failed | cancelled | superseded
```

- activation / resume intent 仅携带 workspace/run scope 与必要 activation identity；Run owner 在自己的
  reducer 中决定 `pending -> active` 或匹配 blocker 的 `blocked -> active`，成功后才发布 Run state fact。
- terminal 不可变。继续未完成工作只能由产品层显式创建新的 typed activation / Run，并保存
  predecessor lineage；不允许自动 successor。
- 不可按当前持久化合同解码或校验的 Run/Runtime 数据不进入 SM-1：owner/repository load 直接失败，且不发布
  `fresh_activation_required`、replacement、terminal 或治理 fact。部署者清理数据后重新创建的 current Run 才能进入 activation。
  `fresh_activation_required` 仅适用于可读 current Run 缺失 sealed bootstrap。current Run 的显式 Restart 才能持久化
  Run-owned replacement intent，经 SM-4 `Result -> Workspace -> Run` delivery 释放 Workspace 后创建唯一 successor并保存lineage。
- 显式 Restart 若没有 non-terminal current Run，Run owner 只按自身 `WorkspaceRunGeneration` 选择最新 terminal
  predecessor。该 predecessor 必须已有 exact `TerminalDelivery + TerminalResultID`；Run owner 在相同
  `run.replacement` 私有 reducer中从 `termination_acknowledged/successor_materialization` 继续，不把 terminal 改回 active、
  不重复 delivery。successor 同时保存 `PredecessorRunID + PredecessorResultID`，且 predecessor lineage CAS 必须先于
  successor activation；重复/重启只收敛到同一 successor。
- blocked 回 active 必须有匹配的 persisted governance resolution。
- completed/failed 的 authority source 只能是 SM-2 sealed terminal decision；cancelled/superseded 的 intent source 只能是
  Run SM-1。两类 terminal 都经 SM-4 exact delivery handoff，最终 Run state 只由 Run private SM-1 event 迁移。
- administrative termination 由 Orchestrator SM-4 在 opaque aggregate 中 CAS 持久化 `RuntimeTerminationRecord` 后交付；
  该 record 是 delivery handoff，不是 Run state，也不允许 Orchestrator 选择或执行 SM-1 terminal transition。
- `run.replacement` 是 SM-1 唯一 owner 内的 subordinate transaction：它只管理 clarification、termination handoff
  acknowledgement 与 successor materialization，不是独立 lifecycle state machine，也不共享 Run state owner。
- Orchestrator Module 的 framework context、`BackgroundRoutine`、Start/Stop 锁与 subscription teardown 只管理进程内生命周期，
  不是 Run state 或 SM-1 transition authority；它们不得创建、恢复或终止 Run。
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
  -> focus advance | repairable gap | governance wait | recoverable runtime wait | sealed terminal
```

- semantic-plan manifest 所需的 template/profile policy、capability catalog 与 provider ceiling 未完成前不得请求
  semantic-plan document；sealed semantic contract、operation manifest、selection、snapshot、failure target、capability、
  validation scratch、provider ceiling 等未完成前不得请求 operation candidate。全部 prerequisite 必须通过 typed
  acquisition 获得并持久化。
- immutable DefinitionFocus 要求 target grounding 时，focus-scoped File-owner `TargetSelection` 及其 exact workspace-tree
  freshness observation 也是 semantic provider 前置条件。selection 必须逐字段回显 current Definition 的
  selector/owner/subject/candidates 与 `ProspectiveTargetAllOfConstraintRefs`；File 只确认 exact owner-supported mandatory set。
  `absent | ambiguous | unsupported`，以及既无 bounded Definition candidate 也无 owner-accepted all-of set 的 empty scope，
  只有在 prerequisite、observation 与声明全部一致时，才能零 LLM fail-closed 为 `target_grounding_unresolved`；无 selection
  的 binding-free view 继续 pending，selected 或带 bounded candidates/exact mandatory set 的 empty scope 作为合法
  prospective creation 进入 semantic request。
- semantic generation 的每个 proof batch 都必须重新通过该前置门禁。`EligibleFocusProjected` 推进 current CWI 后产生的
  `TargetSelection` 或其它 semantic prerequisite，必须先经 typed owner acquisition 与 SM-2 private event/CAS 结算，再从 fresh
  authority transaction 编译 batch 及 immutable `RequestLogicalID`。未结算期间可保留 accepted order/manifest，但不得创建
  request session、调用 provider 或扣减 budget；duplicate、response loss 和 fresh restart 只允许重驱同一 owner fact transaction。
- `need_validation` candidate 前，profile 声明的 validation discovery source 必须由 fresh owner resource 与 matching prerequisite observation 同时证明。
  缺失 `workspace_manifest | project_files | declared_commands | available_capabilities` 是 SM-2 prerequisite debt，不是
  capability unavailable feasibility：SM-2 必须用既有 typed requirement 返回 `need_prerequisite` 并重获 owner fact。
  任一局部 freshness/grounding preflight 只能在完整保留当前 pending graph 后追加 requirement，不得因取得 target snapshot
  而覆盖 manifest、capability、operation-admissibility 或 provider debt；已经 satisfied 的 owner fact 不得被重新编译为新 ID。
- `RequiredOutputContract` 必须是 current focus、owner-confirmed capability、evidence policy 和
  side-effect compatibility 的中立交集。operation 可行动性不足必须在 operation provider 前确定失败；semantic-plan
  LLM 只能在封存 catalog/selector/recipe family 内提出 proposal，不能自由猜测 owner capability。
- validation catalog 必须同时保留 callable capability、ready/non-ready 和 direct/auxiliary/not-declared surface audit fact，
  但 provider projection 和 request-specific schema 只能包含 `ready + direct` 的 `ValidationRecipeRefs`；rendering boundary
  必须再次按 readiness 与 proof applicability fail-closed，不能把 `prerequisite_required`/auxiliary identity、
  schema/input binding、MCP effect metadata 或 Shell executable recipe 当作 model hint。readiness、proof applicability 或
  binding 变化必须改变 owner catalog snapshot/manifest identity。
- prospective owner-fact acquisition 的 manifest 必须把 provider-selectable `OwnerCatalogs` 与
  `OwnerFactDependencyCatalogs` 分开。required owner 的 `prerequisite_required` catalog/observation 只作为 composite
  feasibility dependency进入 manifest/hash/freshness/CAS/seal，不进入 LLM C3、allowed operation/surface、schema enum 或
  correction values。SM-2 先接收 acquisition owner 的 outer feasibility 与 base-revision-bound hypothetical delta，再经
  required owner concrete pure query取得 nested `would_satisfy | would_not_satisfy | typed stale` observation；两个 owner
  都只报告事实，只有 Orchestrator private event 可以改变 semantic ledger。SM-2 持久化 full composite
  fact 并决定 seal/fact refresh/terminal eligibility；仅该 owner-private persisted fact 可交给同 owner 的 SM-3
  既有 correction ledger，capability result 不携带 correction/terminal target state。
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
  recipe/check/metric/artifact 门禁。managed-validation 的 Definition check 还必须经 owner catalog 中 sealed 的
  observation-protocol/native-check mapping 与 receipt typed attestation exact-match；raw process output 不构成该映射或
  attestation。SM-2 只能从 sealed dispatch 的 `BindingRef`、owner receipt 与 persisted binding
  逐字段匹配后派生 CWI coverage；candidate 的 raw `coverageWorkItemIds`、assessment claim、非空 `PassedChecks`、
  exit code 0、历史 summary、memory、projection 和前一 focus evidence 都不能关闭当前 coverage。
- recipe-wide readiness 不得代表所有 CWI ready。同一 capability surface 可以对 ready CWI 持 binding、对 missing CWI 持
  preparation；两者必须按 requirement hash 分离，且所有 binding/preparation 的 native identity 跨 CWI 唯一。
  preparation 只可通过 Profile 显式声明的 acquisition→owner reobserve closure 推进，不能由 Application、provider、
  receiving owner 或 SM-2 临时补策略；缺 closure 时 activation 必须 fail closed。
- multi-target prospective all-of binding 还必须通过 collective receipt gate：同一个 sealed dispatch 中，每个 target
  恰有一个 matching required operation、owner-committed `accepted` exact receipt 与 Orchestrator-committed
  `evidence_committed` execution member。partial accepted set 仍保持 CWI unsatisfied/unsettled；rejected、stale、weak、
  mismatched 或 conflicting duplicate 不计入完成集合。duplicate/restart 只能复用同一 execution/receipt/evidence lineage，
  不能跨 dispatch、repair 或 semantic generation 合并 member。只有完整集合才允许 SM-2 private event exactly-once 认可
  collective proof；CWI credit、settlement 与 focus advance 仍须同时通过其余 quality/effect/assessment gate，并只经 aggregate
  CAS 提交。capability/LLM result 不拥有该 transition。
- repairable gap 保持同一 focus，必须持久化结构化缺口。有效新 evidence 不清零同 focus 总 repair
  budget；只有原子 focus advance 才开启新的 repair ledger。
- validation failure 的 transaction signature 与语义比较必须分离。`RequiredOutputHash`、repair closure、dispatch 和 receipt
  fence 保持原有 exact identity；同一 CWI、recipe/protocol、failed check/case 与 assertion digest 的再次失败，由 SM-2
  rejected-receipt private event 从 immutable evidence ledger 派生并更新既有 `NoProgressCount`，不能靠新 closure 清零。
  只有这些 exact checks 的后继 credited validation pass 才关闭比较窗口；无关检查、无 credit 的修复 effect 与 activity
  均不能关闭窗口。fingerprint/occurrences 是纯比较结果，不是第二份 receipt identity、repair authority 或 persisted counter。
  rejected evidence 不携带 CWI credit；比较时只通过已封存 binding 解析诊断归属，未知 binding 不得从 current focus 猜测，
  更不得回填 `CoverageWorkItemIDs` 或把失败证据移入 accepted ledger。
- owner-native observation 必须区分 passed、failed、skipped、`IncompleteChecks` 与 `OutputTruncated`。父检查 PASS 但必需
  子检查被跳过，或 transcript 不完整时，不能成为 exact native-check proof。可选 `CheckFailures` 只保留有界 check/case、
  workspace-relative location/line 和 assertion digest；不能把诊断位置当作 root cause 或 repair target authority。
  receipt/evidence hash 与 raw clone 同时绑定完整扩展；缺省扩展不改变既有记录 hash，禁止 backfill 旧归档。
- mandatory assessment 尚未 adjudicate 时，accepted durable state receipt 不得提前清除 same-gap repair debt、active gap、
  cycle 或 no-progress provenance。若且仅若该 receipt 具有 exact current-binding CWI credit、真实 changed workspace-durable
  effect、命中 active repair directive 的 owner-grounded target，且 ordered all-of 仍缺声明内 validation，SM-2 可将
  `ValidationOnlyRetries` 重开为 `0` 以执行 fresh validation；无 credit、错 target 或无需 validation 的 receipt 保持原窗口。
- validation-only CWI 只有在 immutable Profile 已封存单一 `ValidationFailureRepairConvergence`、实际 rejected receipt
  精确匹配某条 route 的 `(trigger owner, operation kind, diagnostic class, evidence target type, owner code)` 且 repair cycle
  未耗尽时，才可进入无 exact failure target 的 repair semantic generation。route 的全部 repair operation 必须属于同一
  owner，并在 pre-seal 共同声明一个非 `none` 的 typed confirmation protocol；缺失或混用协议属于 Definition admission
  blocker，不是 Runtime fallback。该 generation 的 current evidence 是 matching route 所声明 repair owner 的
  `state_change`，terminal
  evidence 仍是原 Definition 的 `validation`；它绑定原 failure、convergence hash 和 matching repair-owner catalog，但不在
  pre-seal 或 reducer 中发明 target。只有该 resource/effect owner 的 feasibility 与 SM-2 binding 封存 exact prospective
  target 后才可进入 SM-3。accepted repair 必须在同一 SM-2 CAS 中退休 repair binding、重开同一 CWI并固定进入 fresh
  validation，不能授予 DoD credit、创建新 CWI 或选择替代 terminal。
- semantic generation 的 eligible closure 只能由 generation purpose 派生一次，并由 accepted-order projection、focus
  advance 与 retirement check 共用。普通 definition generation 使用当时全部尚未绑定且可规划的 current CWI；
  `validation_failure_repair` generation 只使用触发 repair 的 exact current CWI。其它全局 eligible CWI 的出现不能退休
  repair-local manifest 或触发第二个 provider request；普通 generation 的真实新增 eligible member，或 repair CWI、purpose、
  lineage 漂移，才可由 SM-2 private event 退休 current generation。任何 LLM/capability result 都不能扩张该 closure。
- semantic owner-private event 的 CAS 返回后，controller 必须从 returned aggregate 重新判定 phase、generation、manifest、
  accepted order 与 request identity。若 event 已退休 generation/manifest，当前 transaction 立即结束并交回 SM-4 重新
  drive；禁止继续使用 event 前的 manifest/hash，也禁止把 nil/stale manifest panic 当作 recovery signal。
- LLM assessment 中的 disposition、缺失 evidence kind、目标和 reason 都只是 observation/original diagnostic，
  不是 Runtime action。SM-2 必须仅从已持久化的 current-focus coverage、accepted receipt/evidence quality、封存的
  focus/target policy、owner-grounded repair surface 及 freshness 生成唯一的 `RuntimeRepairDirective.RequiredState`。
  初次缺少有效验证只能进入 validation；已存在有效 current-focus validation、coverage 仍未收窄且存在新鲜
  owner-grounded implementation/effect repair surface 时才可进入 state change；accepted repair state change 必须按同一
  closure 固定进入 validation。repair surface 缺失/陈旧只能进入 prerequisite；无合法 target 或 validation recipe 时形成
  typed terminal。validation command entry 不得自动晋升为 repair target，且 validation-only budget 只能在 SM-2 已
  canonical 分类并实际再次授权同 focus validation 后扣减；初次 validation、duplicate receipt 与 restart 不重复计数。
  rejected receipt 不获得 CWI credit，若其 CWI 列表为空，只能用已完成 dispatch 封存的 exact `FocusID` 判断当前 focus
  防重放归属；operation ID 或 summary 变化不能改变 stable semantic shape。validation owner 只能从自身支持的结构化诊断提取
  sealed-scope-bounded candidate target；无结构化位置时不得回填 command/input target。结构化 observation 必须匹配
  completed sealed dispatch 中不可变的 `FocusTargetRole + authority snapshot + operation + validation scope`，不得读取已经变化的
  current `Focus.TargetRole`。
  observation 的 `DiagnosticHash` 必须绑定 typed diagnostic 的 class、owner code、evidence target type、stage、operation、target
  refs 与 sealed validation scope；producer、receipt commit 和 fresh-load 使用同一个 canonical hash。receipt commit 必须保留原
  focus/binding。若 observation 的完整五元组命中 sealed Profile route，则在任何 semantic/candidate provider call 前，由该
  route 或 source binding descriptor 确定的唯一 resource/effect owner confirmation 与 SM-2 freshness gate 封存 failure
  target；此路径不得提前开启 semantic repair generation或记一次 validation-only retry。完整 structured observation 未命中
  exact route 时，SM-2 必须在 rejected receipt 的同一 private event/CAS 退休 repair authority并封存
  `validation_failure_repair_route_unavailable`，零 owner query、provider、capability effect、governance 或 SM-4 wait。
  只有无 observation 且 exact `prospective_owner_target` route 匹配时，才可进入 Profile 已封存的 bounded
  `validation_failure_repair` semantic fallback。exact `existing_owner_scope` route 缺少 observation 时必须在同一 receipt CAS
  封存 `repair_surface_unavailable`；repair-purpose provider document 返回 Definition gap 时也进入相同 private terminal，不能
  创建 clarification authority。
  原始 observation 必须完整保留，但不得反向选择 action、retry、ticket 或其它 terminal。
  canonical managed-validation rejected receipt 的 `receipt.TargetRefs` 仅保留 execution/entry lineage；
  `OriginalDiagnostic.TargetRefs` 与 `TargetObservation` 必须为空，candidate 只能位于最多八项的
  `ValidationFailureObservation.CandidateTargetRefs`。该 nested observation 只允许出现在 operation descriptor 声明
  `validation_failure_candidates` protocol 的 validation receipt，并须与 receipt 的
  workspace/run/actual dispatch/authority/operation/signature、typed
  diagnostic operation kind/class/candidates 及 matching sealed per-operation scope 完整一致；其中 diagnostic operation kind 必须
  exact-match sealed operation kind，不能让首次路由匹配与 confirmation 后的 directive 重建使用不同 tuple；顶层 reducer copy 与 nested owner-diagnostic copy
  必须同时存在并 canonical exact-match。commit 与 fresh-load 使用同一校验，且只有 `committed` dispatch 中已标记完成、
  execution 为 `evidence_committed`、`ExecutionKey + Fence + receipt ref/hash` exact-match 的 receipt 才形成 SM-2 fact；
  `owner_committed`、invalidated dispatch 或 generic failure event 均不得提前交接。等待 repair-owner confirmation 时不得保留或
  重建带 target 的 `RepairDirective`。confirmation 的 `OwnerModuleID` 必须 exact-match 上述唯一 repair owner；反向不变量同时
  要求任何已出现的 owner `TargetObservation`、`FailureTarget` 或 validation-derived concrete target 均能回指 exact confirmed
  record 与 exact validation-owner receipt，generic failure ingress 不得旁路该事务。
  同一 VFO 的 repair-owner confirmation 可在不同 revision 上形成零到多条 `stale`，但首条 `confirmed | rejected` 是该序列的终端
  response；其后的延迟/冲突响应必须 fail closed。concrete target authority 只认该 VFO 的 latest valid confirmation，不能从
  任意历史 confirmed record 恢复。是否允许下一次 read-only owner query 只能由 SM-2 owner-private confirmation plan 从当前
  persisted aggregate、该 VFO 的 exact sequence 与 repair transaction lineage 一次性计算；`FailureTarget == nil` 不是 phase
  pointer。SM-4 delayed drive 必须 reload 后重新计算，不能用调用方携带的旧 projection 继续 query。
  SM-2 reducer、query plan 与 SM-3 candidate/provider 准入共用同一 owner-private 待确认义务判定；其输入是 exact committed
  validation receipt、current focus/definition binding、sealed route、剩余 budget 和 observation sequence，而非它将生成的
  Decision。新的 fresh-validation rejection 可以产生新 VFO，但只能在新 receipt 的同一 CAS 消费旧 continuation、退休旧
  target/directive 并打开独立确认窗口。普通 current-catalog target-grounding 不得抢先终止此窗口；窗口内禁止 candidate
  attempt 与 semantic/operations provider request。已封存 terminal、contract failure 和 budget exhaustion 仍优先，不能重开。
  Profile repair budget 在同一 rejected receipt CAS 结算，超限不再发出确认 query；重复 receipt 不重复扣费。
  同一个 plan 同时约束 query admission 与 confirmation writer 的 fresh-load CAS admission，并封存
  `RequestID + IssuedAtStateRevision + protocol-bound SubjectRef + ExpectedSubjectRevision`。protocol descriptor 只声明 owner-neutral
  resource domain/subject；expected revision 必须来自 matching current owner prerequisite observation，Runtime core 不得按 concrete owner
  或 prerequisite kind 分支。`RequestID` 必须绑定 observation、owner、protocol、subject、expected subject revision、current revision 与
  stale refresh lineage。owner actual revision 与 expected 不同时只能返回 `stale`；相同时才允许 `confirmed | rejected`。SM-2 接受
  `stale` 的同一 CAS 必须退休 exact dependency 上的旧 owner facts、追加 invalidation lineage 并结算 prerequisite-refresh budget，不得建立
  target authority或消耗 validation-only/repair budget；成功 writer CAS 的 record 必须满足
  `RecordedAtStateRevision == IssuedAtStateRevision + 1 == resulting StateRevision`。opaque reload 必须重验该 revision chain、request
  identity 与同一 VFO 的 subject continuity；全部 observation 共用一条 append-only revision 时间线，后一 request window 的
  `IssuedAtStateRevision` 必须严格晚于前一 record 的 `RecordedAtStateRevision`，不同 observation 也不得复用 revision、逆序或重叠。
  stale→prerequisite refresh→reopen 后旧 lineage response 即使再次遇到
  `need_validation` 也必须 conflict 且零写。

非 VFO direct-target 使用 descriptor-authorized owner-fact 路径。只有 descriptor 声明 `receipt_evidence_observation` 时，
rejected owner receipt 才可保留 response 显式观察到的 bounded
`RuntimeFailureTargetObservation`，但不得从 operation target、request 或 manifest 推断；promoted failure copy 与 nested
`OriginalDiagnostic` copy 必须同时存在且 canonical exact-match。该 observation 只有反向匹配 completed committed dispatch、
completed operation、`evidence_committed` execution、exact key/fence/ref/hash rejected owner receipt、唯一 receipt-derived rejected
evidence 与 processed-event fence，以及 sealed/current exact owner catalog snapshot、acquisition、revision、subject selector 和
satisfied prerequisite observation 时，才是 current `FailureTarget` authority。`owner_committed`、generic failure ingress、任一
receipt/evidence/catalog 漂移或 stale 均 fail closed；后续 resource-owner target snapshot 只提供 current resource grounding，不能
重建原 diagnostic owner authority。

descriptor 的三种 target 协议互斥：`sealed_operation_targets` 只允许无 observation 的 diagnostic refs 且必须是 sealed operation
targets 的子集；`receipt_evidence_observation` 允许上述完整 evidence/catalog 反向证明的 discovered target；
`validation_failure_candidates` 只允许 VFO candidates 并等待 sealed repair owner confirmation。`none` 不允许任何
authority-shaped target。
该 policy 是 operation descriptor hash 的一部分，receipt commit 与 fresh-load 必须执行同一校验。

`Facts.Failure`、`FailureTarget` 与 `RepairDirective` 是同一份可行动 failure authority，不能作为三个独立字段随意覆盖。
所有 failure 替换只允许通过唯一 owner-private writer，并显式选择以下一种 policy：

- `runtimeFailureAuthorityRetainInterlude`：只允许同一原子事务中的有界中间态保留原 target/directive，不能携带 replacement target；
- `runtimeFailureAuthorityRetire`：写入新 failure 时同时退休旧 target/directive，普通 provider、candidate、feasibility、governance
  或 terminal failure均不得隐式继承旧 repair authority；
- `runtimeFailureAuthorityReplaceFromExactOwnerFact`：新 target 必须与新 failure 的 signature、canonical targets 与 observation
  exact-match，并先清除旧 directive；只能由当前 receipt/confirmation transaction 已验收的 exact owner fact 使用。

任何绕过该 writer 的直接非空 `Facts.Failure` 写入、保留旧 target 但替换 failure lineage、或用 current manifest/diagnostic 文本
重建 replacement target，都属于 torn SM-2 authority，必须在静态门禁、transaction test 与 fresh-load invariant 中失败。

已封存 validation-repair effect 的 rejected receipt 在完成 dispatch 的同一次 owner-private SM-3/SM-2 CAS 中退休
target/directive，并封存 `repair_surface_unavailable`；semantic repair intermediate binding 同步退休，Definition source binding
只保留其原 proof 语义。原 receipt diagnostic、confirmation、repair commitment 与 evidence lineage 不改写，禁止由新 failure
继承旧可执行授权。candidate-local constraint correction 必须在 dispatch 前完成；此边不增加 retry、owner 或状态机，
也不接管 typed stale/governance/unknown effect 的独立事件。

validation failure 因果中的三类 target 是相互独立的 immutable fact：

| Target | 唯一来源 | 约束 |
| --- | --- | --- |
| validation operation entry `TargetRefs` | completed dispatch 中 exact validation operation | 是本次 recipe/validation entry，不是 full scope；只有 matching sealed per-operation authorization 的 `RequireTargets=true` 时才强制非空。matching Shell-owner catalog 存在 finite `TargetRefs` 时，每个 entry member 必须属于该集合 |
| finite full validation scope | matching completed-dispatch `OperationExecution.AdmissibilitySnapshotID + AcquisitionBinding` 所绑定的完整 Shell-owner `OperationAdmissibility.TargetRefs` | per-operation target authorization 只裁决 validation entry；structured observation 的 `ValidationScopeTargetRefs` 必须与 owner catalog finite set canonical exact-match。execution、receipt causation 与 restart 均不得从 mutable manifest、current focus 或 fresh owner inventory 重建 |
| structured diagnostic candidate targets | validation owner 支持的结构化诊断位置 | observation 存在时必须是 full validation scope 的 1～8 项子集，随后仍由 sealed Profile/descriptor 指定的 resource/effect owner 按其 current manifest 独立确认 |

在 finite scope 的 structured-observation 路径中，operation entry 与 diagnostic candidate 各自只需满足其 scope-subset
invariant；二者不要求相等或相交。因而不得以
operation entry 覆写 diagnostic candidate，operation entry 也不得被自动晋升为 `FailureTarget`。current manifest 只参与后续 repair-owner
freshness confirmation，不能改写 completed dispatch 已封存的 validation scope 或历史 observation causation。

execution producer、receipt admission、repair confirmation/commitment 与 fresh-load invariant 必须共用上述 sealed-scope
投影函数。单文件 entry、多文件 full catalog 是必测正例；缩小/扩张 observation scope、entry 越权、错误 dispatch 与
重启后从 current catalog 重建范围均须拒绝。范围修正不改变 SM-1 owner，也不授予 capability owner SM-2/SM-3 转换权。
software-dev Profile 中 Shell 的 closed `WORKORCH_FAILURE_TARGET` marker 也只是 owner-supported bounded diagnostic candidate；它不得直接填充或生成
`RuntimeFailureTargetObservation` authority，只能在通过 sealed finite scope gate 后进入 `RuntimeValidationFailureObservation`，再由
Profile 指定的 repair-owner confirmation 与 SM-2 private event 封存真实 repair surface。sealed Shell-owner catalog 的
`TargetRefs` 为空时没有 finite diagnostic scope；无论 operation 是否因 `RequireTargets`/owner feasibility 而可执行，
Shell 都必须保持零 scope-bound structured observation。失败只可进入已封存的 Profile semantic fallback，任何组件都不得用
current manifest 临时补全 scope；没有 matching sealed fallback 时按 typed contract fail closed。
- assessment evidence gate 只验证 current-run/focus、binding-matched owner receipt、effect、quality 与封存的
  focus evidence policy；它不得把 assessment observation、claim signal 或 `PassedChecks` 非空解释为 CWI credit，
  也不得改写 observation 的 disposition、items、gap 或 refs。profile 的 observable validation recipe 只能约束候选
  选择该命令时的必要参数；只有 receipt 通过 sealed owner observation protocol 对 exact native check 产生 matching typed
  attestation，且满足 binding 要求的 check/metric/artifact，才可满足该 binding，
  不得用 Goal/DoD、summary、operation ID 或检查名之间的自由文本相似度裁决 coverage。
- 每个 assessment focus 的 `AssessmentScope + AssessmentID + AssessmentText` 必须逐字段等于 Definition seed 对应 slice 的
  `SourceScope + SourceID + TargetText`；active/completed assessment authority 都必须反证到该 immutable seed source。SM-3
  在 dispatch seal 前持久化 raw-canonical `RuntimeAssessmentAuthorityInput`，完整绑定
  `definition_proof | owner_fact_acquisition` purpose、source slice/source 字段、Focus、CoverageEvidenceBindings 与 exact owner
  admissibility catalog；其 hash 进入 immutable dispatch authority。LLM assessment document 与 mutable current facts 均不能
  替换该 pre-seal classifier input。
- assessment dispatch commit 时，SM-2 只能从上述 pre-seal input、current accepted evidence facts、completed dispatch receipts
  与 `CoverageProofLineage` 确定性重放 `RuntimeAssessmentAdjudication`。adjudication/hash 必须与 `RuntimeAssessmentFact` 的
  `AdjudicationHash` 在同一 CAS 双记账；post-seal adjudication 不进入 immutable dispatch authority hash。fresh-load 必须重放
  classification 并 exact-match disposition、逐 CWI proof、`EvidenceFactID + EvidenceFactHash` 与两侧 hash；raw normalization
  laundering、seed/source 漂移、proof 删除/替换或协调改写 disposition/fact/hash 均 fail closed。
- assessment 必须先独立证明 sealed binding 的其余 evidence/effect/quality 条件，再检查 current owner-fact postcondition。
  只有“其余 proof 全部成立、owner fact 是唯一缺口”时，SM-2 才能在提交 assessment dispatch 的同一 CAS 中把 current
  bound CWI 记为 `reopened`、清除 current binding、更新 reconciliation contract，并以 typed reopen lineage 退休当前
  semantic generation。旧 binding/receipt/evidence/assessment 只保留 audit；SM-4 随后从同一 aggregate 幂等驱动新
  semantic generation。普通 evidence gap 不得伪装为 owner-fact gap；新 semantic proof admission 只能接受 pre-seal
  closure 中 current all-of dependency 已就绪的 exact acquisition owner/kind/variant/target-policy/operation-selector。
  Definition target selector 只保留声明 lineage，不能替代 capability operation selector。SM-2/SM-3 必须先把二者
  编译为 manifest 内唯一的 per-CWI `RuntimeSemanticProofObligation`，provider schema、typed correction 与 binding seal
  全部消费该 obligation；batch 必须恰含一项，不能让 LLM、Shell、File 或后续 aggregate read 另选动作。obligation 必须将
  Definition terminal evidence 与 current acquisition evidence 分字段保存；acquisition generation 的 `state_change` 不能满足
  terminal `validation`。预算按实际 accepted acquisition operation 记账而不是按 provider replan 次数记账；当前
  `MaxAcquisitionAttempts=1`，reobserve 后仍缺失即以 `owner_fact_convergence_exhausted` fail-closed。reobserve 已满足时，
  `assessment_observed` private event 在同一 CAS 中保留 acquisition audit/anti-replay fact、保持 CWI open、写入
  `assessment_owner_fact_acquisition_settled` reopen lineage并退休 generation；successor semantic generation 必须恢复原
  terminal evidence obligation，不能继承 acquisition credit。重开投影不得复制 predecessor generation 后按字段清空；
  它必须由 immutable `RuntimeDefinitionFocus.RuntimeFocusSeed()` 正向构造，并且除固定改写为
  `semantic-pending/<cwi>` 的 ID 外与该 seed 全字段 exact-match。Definition 的 target selector、mandatory all-of、grounding、
  owner-fact 与 Profile convergence 声明只保留在 immutable Definition，不能进入 pending current cursor；predecessor 的 exact
  target、prospective target、selector、accepted operation/variant、validation recipe/check 同样不能进入。create-only target 已由 File effect
  实现后，不能再次发布旧 prospective selector 并把 File owner 的 `unsupported` 误作 terminal Definition fact。该
  binding-free projection 允许 reducer 暂存 provisional `target_grounding_unresolved`，但 SM-3 semantic request admission、
  SM-4 delivery 与 fresh-load bootstrap identity 必须消费同一个 semantic-re-drive classification。duplicate/restart 不重复
  reopen、request、effect 或预算扣减。重开 CWI 固定返回 `semantic-pending/<cwi>`；当新的 current-focus acquisition 与
  上一 generation 已退休 prerequisite 使用同一 canonical ID 时，SM-2 必须在排入 pending 前原子移出该 retired ID 及其
  旧 observation，且不得复用旧 observation。无关 retired lineage 必须保留，审计由 CWI reopen lineage 与 semantic
  generation history 持续承载，active/retired 不相交 invariant 保持严格成立。current binding 的 proof purpose 必须由
  exact `SemanticManifestHash + CWI ID` 从 active manifest 或 immutable generation history 唯一恢复；history 只解释
  purpose lineage，不恢复 candidate/action authority。aggregate invariant 必须拒绝 acquisition purpose 的
  `satisfied/settled` terminal credit。历史 reopened/retired binding 不得再次成为
  `CurrentBindingRef`。导致 owner-fact gap 的 accepted state-change operation shape 必须作为同一 CWI 的 anti-replay fact
  跨 successor semantic focus 保留；SM-2 不携带 predecessor repair attempt/gap/directive，SM-3 在 dispatch 前拒绝等价
  shape，因此新 binding generation 不能重置等价副作用的防重放边界。
  assessment 的 canonical quality gate 必须同时读取 pre-seal purpose：`owner_fact_acquisition` 只裁决当前 binding 的
  acquisition evidence，不能把 Definition 的 terminal `RequireExecutableBehaviorEvidence` 应用到中间 `state_change` receipt；
  `definition_proof` 仍完整应用该门禁。producer 的 disposition、missing evidence、target 与 reason 只进入 audit，不能覆盖
  purpose-aware adjudication。acquisition 成功仍必须 reopen CWI，绝不因此取得 terminal credit。
- prospective composite feasibility 的 pre-seal Definition 不携带 exact target set，但必须封存完整 mandatory
  `ProspectiveTargetAllOfConstraintRefs`；compiled obligation 以
  `DeclarationProspectiveTargetAllOfConstraintRefs` 原样传递，SM-3 schema/document admission 要求 proposal
  `ProspectiveConstraintRefs` exact-equal，禁止子集、超集和 alternative。File target-selection fact 与后续 reconciliation
  同样绑定该集合；只有 exact owner-supported set 可继续。compiled obligation/manifest 不补写 exact target path。
  required owner catalog 发布 provider-safe typed `ProspectiveTargetSatisfactionConstraint`，且只允许 mandatory `all_of`
  clauses；同一值封存到 acquisition/manifest 并在 C3 投影。SM-3 只能提交一份 bounded semantic proposal，其中
  `ProspectiveTargetRefs` 必须包含 1～8 个唯一 canonical exact target。每项都必须位于 Definition declaration
  selector/candidate refs 与 current operation selector/owner constraints 的交集内，非空 candidate refs 还要求逐项 exact
  member match；每个 clause 至少由一个 target 完整满足，每个 target 也必须对至少一个 clause 有贡献。不存在
  `one_of`/`any_of` 关系，current wire 拒绝 singular `ProspectiveTargetRef` 及兼容 alias。schema/document admission
  不匹配只使用普通完整文档 correction，零 File/Shell probe。File 必须基于同一 complete manifest 对完整集合执行 path、
  absence、parent conflict 与 aggregate-capacity admission，只返回无副作用 hypothetical delta；Shell 只能基于该 delta 的
  resulting manifest 判断 `would_satisfy`。只有在 File delta、File catalog/observation、Shell dependency
  catalog/observation、activation-sealed exact recipe identity、
  manifest/batch/request/contract ref全部 current 且 nested outcome 为
  `would_satisfy` 时才 seal-ready。SM-2 在 observation CAS 与 semantic seal 前各重验一次；任一 lineage 漂移都保持零
  binding/operation/effect。Shell typed stale 保留在 nested observation 中，不能改写成 File outer stale或即时 terminal；
  它只通过既有 `semantic_contract_required` private transition消费 bounded refresh generation并退休 exact dependency。
  missing dependency 在 query 前退休 generation，零 Shell call。`would_not_satisfy` 必须先以
  `semantic_feasibility_observed` 持久化完整 composite fact；该 target set 仍是未封存的 proposal，不产生 binding。
  stale 只消耗独立 bounded refresh，不消耗 correction。由于 owner probe 之前的 admission 已证明 target 满足 published
  constraint，exact-current full-composite `would_not_satisfy` 是 owner contract contradiction；下一 owner-private drive
  记录 `prospective_satisfaction_contract_violated` 并统一收口为 `semantic_contract_failed`，不得生成 replacement-target
  correction。零 ticket/governance/acquisition/operation/effect/coverage；duplicate/restart 不重复 terminal 或同一
  File/Shell probe。
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
- complete Definition-gap document 只是 LLM observation candidate。`undeclared_prerequisite` 必须为每个 offending CWI 提供至少
  一条唯一、bounded、typed `PrerequisiteClaim(CoverageWorkItemID, OwnerModuleID, FactKind, OperationKind)`；其它 gap kind 禁止携带
  claim；shared source ref 必须覆盖其全部 CWI，proof-batch ref/claim 不得逃逸 sealed batch。SM-2/SM-3 在任何 owner-private
  mutation 前用 sealed proof obligation、provider-selectable owner catalog 与 dependency
  catalog 做 pure admission：exact requirement 已满足、已有 acquisition、已声明但 unresolved 或 claim/schema/stale 漂移都只能
  进入同 request 的 bounded semantic correction，不能提交 `definition_gap_observed`、`need_governance` 或 ticket。只有所有 claim
  都是 `genuinely_undeclared` 的 admitted gap 才能由 Orchestrator owner-private event 选择 clarification transition。
- capability/provider unavailable、consumption unknown 或 owner dependency 暂时不可用时，只有已经存在 owner-defined typed
  recovery/reconciliation contract、明确 recovery condition、deadline/trigger 与可用 budget，SM-2 才可持久化
  `recoverable_runtime_wait` fact。缺少任一条件、guard/budget耗尽或owner明示不可恢复时必须形成typed terminal；
  不得用free-form retry hint创建wait。
- core 对 capability result 的分类只能读取 typed feasibility/status、sealed operation descriptor 与 Profile declaration。
  owner-private diagnostic code/prefix必须原样保留为 audit fact，但不能选择 failure class、repair owner、refresh、retry、
  governance 或 terminal；unknown typed status 必须 fail closed。

禁止：SM-2 调 provider、执行 operation、直读其他 owner repository、将 profile/软件领域语义写进 reducer、
用数量而非质量关闭 evidence gate；禁止 capability/application/projection 直接提交 SM-2 transition 或由 Orchestrator
修改 resource/validation owner state。

### SM-3：Candidate and sealed-dispatch transaction

```text
sealed authority + preflight ready
  -> persisted request session
  -> typed document result
       -> no-candidate framing -> bounded whole-document recovery
       -> one complete JSON object -> strict local admission
  -> candidate attempt 0 -> optional attempt 1
  -> accepted candidate -> persist sealed dispatch
  -> claimed operation -> owner receipt -> evidence commit -> close dispatch
```

- request session 必须先绑定 transaction、definition/evidence revision、required-output hash、expected-document
  identity、stable-prefix hash 和 logical request identity。EOF、output limit、timeout、429/5xx 等允许恢复的事实走
  session/provider recovery；401/403、configuration/protocol、未知提交和 document recovery 耗尽走 terminal，不消耗 candidate 或
  repair budget。恢复只能重新请求同一有界完整 document；历史 raw output 只作 owner audit，不得拼接成 authority document。
- provider terminal 是明确的 SM-3→SM-2 owner-local transaction：唯一 normalized fact 必须在一次 aggregate CAS 中同时
  关闭 request session 与应用 exact failure。不得恢复“先 terminal session、后 failure”的双 CAS；fresh reload 对缺少
  matching failure 的 torn terminal fail-closed，对 submitted-without-outcome 只封存 unknown 且零 provider replay。
- LLM result 的 `providerSubmitted` 是 physical-call 唯一事实，Orchestrator 不从 request bytes/response mode 推断。
  每个 result 先形成 owner-private budget settlement；stale、document-complete、recoverable 或 terminal transition 必须在其
  同一次 aggregate CAS 中应用 settlement。recoverable CAS 还原子消费适用的 SM-4 transport guard，并在耗尽时应用 SM-2
  failure；released/unknown settlement 不能授权下一 physical call。
- request session、authority transaction、canonical state/required output、`DocumentKind`、ExpectedDocument、provider
  ceiling、reasoning effort、budget和stable-prefix identity共同构成purpose-bound semantic invocation seal。它实现产品层
  semantic activation要求，但不是新owner、第五状态机或`RuntimeBootstrap.Activation`替代物。
- incomplete/empty/invalid framing、transport 与 provider-terminal 不得创建 candidate attempt；前述 document framing 只推进
  bounded request-session recovery。只有 one complete JSON object 的 schema/contract/semantic rejection 共用 attempt 0/1；
  attempt 0/1 必须使用同一 transaction 与 stable prefix，第二次任一可纠错 rejection 形成 typed
  `candidate_retry_exhausted`，不存在 attempt 2。
- required field missing/empty 与已存在但为 `null`/wrong JSON type 共享同一 SM-3 correction ledger：前者封存
  `field_absent -> field_present`，后者封存 exact `<actual_type> -> field_present`，且都只授权 attempt 1 重写完整 document。
  strict decoder 不本地 repair；attempt 1 必须重新通过同一 typed admission，不新增 correction、transport 或 recovery budget。
- renderer、wire DTO、strict decoder、admission、owner feasibility 与 fixture 对同一
  `CandidateContractManifest` 给出一致解释；候选只能选择 manifest 已封存的 `bindingRefs`。未知字段、参数、
  target、effect、dependency、operation kind、未知 binding 或裸 `coverageWorkItemIds` 不得由第二解析路径静默放宽。
- manifest 静态 operation-set policy 对普通 durable proposal 强制每个 `(OwnerModuleID, canonical TargetRef)` 最多一项；
  只有 manifest 内 exact `RequiredOperationSequence` 可声明例外。decoder、evaluator、attempt-1 witness 与 final seal 必须共享
  同一纯校验，不得从 mutable owner facts 重新推导。
- single evaluator 还必须在 owner probe 前执行 sealed activation 的 executable parameter/variant/effect fence；final seal 只重复
  同一 manifest + activation 校验，不能首次发现一个已持久化 proposal 的可执行合同违例。variant restriction 以 kind 为域，
  exact required-sequence variant 与普通 accepted variant 取并集，不能限制未声明 variant 的其它 accepted kind。
- semantic-plan document 可以为 `prospective_owner_target` 提出一份含 1～8 个 exact `ProspectiveTargetRefs` 的 bounded
  all-of proposal，但在 File/Shell fact 完整且 SM-2 封存 binding 前不得生成 operations request/dispatch。operations
  document 只能选择 binding ref；封存的 `RequiredOperationSequence` 必须为每个 target 编译一项 exact-singleton
  `TargetRef` requirement，并以 canonical dependency chain 固定完整顺序。candidate evaluator 和 dispatch sealer 必须从
  current binding 派生/校验 target；缺项、附加 target、重复或乱序均在 owner effect 前拒绝，provider 不得重新选择或替换。
- required sequence 的每个 owner receipt 先独立落入 matching execution 的 `owner_committed` ledger；随后只有
  Orchestrator owner-private evidence event 可以把该 execution、receipt-derived evidence 与 processed-event identity 在同一 CAS
  中提交为 `evidence_committed`。前 N-1 项完成时 `ActiveDispatch` 仍保留并继续 canonical next operation；单项 receipt 不得
  关闭 multi-target binding。rejected/stale 可以按既有 typed transaction route 结束或刷新 SM-3 工作，但不得被解释为
  successful CWI settlement。duplicate 不推进 revision，fresh restart 不重放已有 effect，只恢复同一 sealed dispatch 的缺失项。
- prospective `would_not_satisfy` 不属于 SM-3 correction ledger：它在 SM-2 作为 published constraint 与 owner judgment
  矛盾的 full-composite factual observation，直接形成 `prospective_satisfaction_contract_violated` terminal。SM-3 的
  attempt 1 只服务普通完整 document/schema rejection，继续复用 transaction/manifest/C0～C3/stable prefix；C4 只携带
  manifest/CWI/field-path-bound typed violation，不复制 C3 constraint，也不新增 machine、owner 或跨 owner
  state write。typed stale 仍只走 SM-2 fact refresh，不触发该边。
- 单一 evaluator 必须在 seal 前拒绝同一 candidate 中重复的 durable `(OwnerModuleID, TargetRef)`，并要求 provider 将同目标
  变更合并为一个 operation/receipt；普通 `DependsOn` 不能放宽，因为所有 replay proof 均在首个 effect 前封存。只有
  owner fact 明确封存、逐项绑定 exact requirement ref/variant 的 `RequiredOperationSequence` 可在同一 owner-target 上执行；
  不同目标与 non-durable operation 不受该唯一性约束。
- accepted candidate 必须在执行前持久化 `SealedDispatch`，并在 accepted attempt 中另存 canonical
  `OperationTargetAuthorizations` projection commitment。fresh-load 必须同时匹配该 admission-owned commitment 与 dispatch
  self-seal；仅协调扩大 target scope 并重算 `DispatchAuthorityHash` 仍须失败。governance resume 逐 hop exact 继承 source
  commitment，最终可追溯到唯一 accepted attempt。operation 依 sealed dependency order 执行，
  每项使用稳定的 `ExecutionKey + Fence`；owner receipt 是 evidence 的唯一副作用回执。
- `dispatch_execution_requested` 只允许完成一次 SM-3 preparation/claim CAS，并返回 immutable
  `RuntimeOperationExecutionCallHandoff`；它不得持有 executor callback，也不得在 `ApplyInternalEvent` 内调用 capability owner。
  Biz 在 private-event dispatcher 外发送 concrete typed EventHub command；回执持久化后，下一次 owner-private event 只能在
  `operation_evidence_observed | operation_prerequisite_stale_observed | operation_governance_required_observed` 中恰选其一，未知
  physical outcome 则单独提交 `operation_execution_outcome_unknown_observed`。任何一个 event 都必须声明完整 coordinated write-set。
- claimed operation 在没有 receipt 时，只能由 SM-3 返回一个只读的 recovery-budget handoff；SM-3 不得直接修改
  SM-4 的 recovery ledger。后续 `recovery_budget_consumed` private event 由 SM-4 独立 CAS 提交预算，并在同一 CAS 新增
  `RuntimeOperationRecoveryAuthorization{workspace, run, dispatch, operation, ExecutionKey, Fence, RecoveryAttempt,
  IssuedAtStateRevision, issued}`。`operation_recovery_claimed` private event 再以独立 CAS 把 exact record 变为
  `in_flight`，写入唯一 `ClaimID + ClaimGeneration + ClaimedAt + LeaseDeadline`；只有该次 CAS 的成功 caller 持有一次性
  physical-call handoff。其 identity/state 不属于普通 dispatch transaction payload，缺失、半截、错 operation、错
  authorization、未 claim 或过期 claim 的 envelope 必须在 owner call 前拒绝。exact receipt 在同一 receipt CAS 将 claim
  `settled`；call 返回仍无 receipt 时，`operation_recovery_call_returned` 将其置为 `uncertain`。active `in_flight` 禁止发放
  下一 grant；只有可信 lease expiry/restart 把旧 claim 归为 `uncertain` 后，才能消费新预算取得新 authorization。终态只可
  把未执行的 `issued` 退休为 `retired`，不得把仍在飞行的 effect 虚报为已完成。
- 同一 dispatch 内 accepted receipt 造成的资源 revision 变化是内生事实：它可以使后续 SM-2 prerequisite
  observation 失效，但不得仅因此结束尚有 sealed operation 的 active dispatch。全部 operation 成功或任一
  operation rejected/stale 后，SM-3 才结算自身 transaction 并交回 SM-2 reduce；后两类 transaction 结算不等于 CWI
  successful settlement，也不能提供 collective completion proof。
- exact owner-confirmed validation repair 是上述规则的窄化事务：manifest purpose 必须为
  `confirmed_validation_repair`，`RuntimeDispatchRepairCommitment.RepairOwnerModuleID` 必须等于 sealed Profile exact route 或
  source binding descriptor 确定的唯一 resource/effect owner。accepted matching-owner receipt 必须是 target-bound durable change，
  每个 operation descriptor/target/producer 属于同一 owner 与 confirmed surface，并携带 exact target resource changes 与唯一 workspace-tree
  `beforeRevision -> afterRevision` edge。多 operation all-of 的 partial CAS 只可继续同一 active sealed dispatch；generic
  freshness 退休 confirmed revision 后，原 `FailureTarget` 与 `need_state_change RepairDirective` 必须保持为空。fresh-load 必须由
  confirmed record、completed `evidence_committed` executions、exact receipts、accepted evidence/processed fences、freshness
  invalidation 与 current pending prerequisite 的 exact union 重新证明，不能把 partial 变成新 owner fact 或第二状态机。
  workspace-tree edges 必须从 confirmation revision 形成唯一连续链，operation ID 排序没有 revision authority。最后一项
  accepted 时，SM-2 在同一 CAS 从 immutable validation-owner receipt + repair-owner confirmation + sealed Decision/dispatch 重算 closure，并生成新的
  target-free fresh-validation directive；任一 rejected/dispatch-invalidated member 均禁止恢复 stale target/directive。
  后续 fresh validation 的 rejection 只消费其 manifest exact closure 所指向的 continuation，不提供成功 proof。反向不变量
  必须同时匹配更新的 rejected evidence revision、processed event、manifest transaction/hash/closure、committed operation 与
  exact rejected receipt fence/hash；缺任一项都不能用历史失败掩盖丢失的 continuation。新失败必须独立经过 SM-2 route/budget/
  current resource-owner confirmation，旧 confirmation、binding 和 startup input 不改写。
  active partial dispatch、上述 target-free fresh-validation continuation 与 administrative termination 都必须使 confirmation plan
  为空；它们不能因 mutable `FailureTarget` 已退休而再次触发 resource-owner query。termination 后的新 confirmation response 不得
  改写 SM-2；termination 前已持久化的 exact response duplicate 仍是零写 replay。
- duplicate 结果幂等，stale 只进 diagnostic lineage。已存在 exact receipt 时重启只提交后续 evidence，
  不得重放副作用；未知、非安全重放的副作用进入 typed outcome-unknown/failure 路由。
- effect/evidence 类型由 sealed `RequiredOutput`、sealed expected effect、sealed binding 与 owner observed effect
  联合裁决。receipt 只有同时匹配 `dispatch + operation + BindingRef + target/selector + effect + recipe/check`
  才能获得该 CWI credit；`none`、`ephemeral_session`、`unknown`、zero-work、unrelated target 或泛化 test success
  不能满足 durable state change 或 behavior coverage。
- live owner response 与 fresh-load receipt recovery 必须复用同一 outcome-first judgment：`rejected` 保留 owner exact
  diagnostic 且不再运行 effect/quality 改写；`accepted | no_op` 才进入 effect compatibility 与 evidence-quality gate。
- exact `ExecutionKey + Fence` receipt 重放只复用既有 receipt；no-op 的 current-run provenance 与普通无
  provenance no-op 必须分流。后者只留下 rejected/no-progress lineage，不能被重放、target 变化或文本修改伪装成
  新的 durable evidence。
- confirmed-validation all-of 的 operation-scoped governance continue 由 SM-3 在同一 owner-private CAS 证明 origin admission、
  exact `RepairOwnerModuleID`、
  source sealed dispatch、completed receipt chain 与 exact granted binding，再把 exact resolution/binding/source seal 作为
  `governance_resolution_observed` 输入交给 SM-2 唯一 `ReduceRuntimeStateWithDirective`。只有该 reducer 可派生
  continuation decision；SM-3 只据此封存 next-fence resume dispatch，transition 不得在 reducer 返回后手工覆盖或重封
  Decision。旧 `FailureTarget`/`RepairDirective` 保持退休，restart 直接恢复已持久化 decision/dispatch lineage。

禁止：未持久化 request session 请求 provider、未完整 document 消耗 attempt、accepted candidate 不 seal
直接执行、executor/owner 选择下一 action、owner 直接终态化 Run/Result、依错误文本猜 target 或创建 ticket。

### SM-4：Drive, recovery and projection delivery

- `DriveRuntimeCommand/Result` 是 Orchestrator 唯一同步推进合同。Application 不保留 continuation queue；
  Run owner state notification 才能投递 Orchestrator 的内部 drive event。
- 每个非等待、非终态 transition 必须持久化递增 `StateRevision`；无进展或超过 guard 必须 typed
  fail-fast，不能在进程内空转。
- 每个 Orchestrator worker 的退出边界必须满足以下二选一：仍由当前 run-scoped gate 同步执行下一 owner-private transition；
  或在释放 gate 前，aggregate 已具有 exact terminal/governance/recoverable-wait/lifecycle handoff，或已持久化并挂载 exact
  `RuntimeDriveLease` successor。owner worker 的 panic 必须在 Orchestrator 边界恢复为稳定 owner-local failure，并沿既有
  private event/lease 路径持久化 recovery successor；panic 文本不是 authority，Application、Run、projection 与外部 watchdog
  均不得代为续驱。若 successor CAS 或本地 timer mount 基础设施失败，当前 gate 必须与已有 scheduled/running authority
  一同保留，并登记 lifecycle/epoch/gate-token fenced 的本地 settlement wake；本进程持续 fresh-load/reconcile，进程崩溃后再由
  fresh startup 恢复 exact generation。此时不得虚报 live timer，也不能以 `retry=nil` 或 generation zero 释放为 active orphan。
- scheduled worker 收到任意非 terminal-delivery drive error 时，必须在统一 SM-4 error boundary reload current aggregate；若并发
  owner-private transition 已封存 convergence terminal 或 administrative termination，则该 durable terminal 优先于在途旧 error，
  worker 只进入既有 terminal delivery lane，不得再分类 semantic failure、消费 operation/provider recovery budget或生成另一份
  `persisted_pending` successor。已经属于 terminal delivery lane 的 error 先 reload：若 final acknowledgement CAS 已完整持久化则
  零预算结束，否则进入其 bounded retry；不能先 eager 重投一次 delivery。
  error classifier 必须显式区分“已处理且无需 retry”与“未处理”，防止 panic/preflight 调用点在 terminal 已交付后补造 fallback lease。
- administrative termination 的 owner-private CAS 可以退休当时 exact running lease `G`。该 worker 随后只可在 terminal delivery
  尚未完整 acknowledgement 时，以 canonical `delivery_retry/terminal-delivery` 通过同一调度 CAS 建立 `G+1` successor；普通
  stale scheduler、其它 failure signature 或非终态不得使用此例外。`G+1` 必须覆盖本地 registration，使旧 timer 的 `end(G)`
  无法删除 successor。若 final acknowledgement 在 recovery-budget 或 lease CAS 前完成，两者均须返回零写 completion；exact
  running `G` 仍存在时必须在 lease CAS 原子退休，worker 根据显式 terminal-complete disposition 释放 gate，不得留下无 timer 的
  running zombie 或 fully-acknowledged terminal 上的无意义 lease。acknowledgement 与 budget/lease CAS 冲突时，owner 必须 bounded
  fresh-load/retry：partial acknowledgement 后仍要只消费一次 budget 并建立 successor，final acknowledgement 后直接 complete；
  budget CAS、new scheduled-successor CAS 或 exact-running cleanup CAS 已提交但返回普通 response-loss error 时，必须从 current
  ledger/lease 分别识别已消费、exact scheduled lease 或 terminal-complete，不能重复 budget、generation 或 timer。无法线性化 budget
  时只能保留当前 process gate，
  不得释放 gate 或创建未获预算授权的 lease。administrative termination 下任何 retained lease 只能是 exact
  `delivery_retry/terminal-delivery`，fresh load 必须拒绝其它 signature。retired running marker 只证明 termination 的首个 `G -> G+1`
  handoff；后续 terminal recovery 的 current exact request `R(F)` 可以在 scheduled `F+1` 因 revision drift 退休后重物化同一代，历史
  marker 不得把当前 request 错配回首代或迫使再次消费 budget。
- synchronous `DriveRuntime` EventHub ingress 与 scheduled worker 必须共用同一 workspace/run singleflight gate；两者在 owner work 前
  都检查 Run-owner durable retirement fence，同步入口在持 gate 后还必须 fresh-load，current persisted lease 存在时不得旁路其
  authority。同步退出也必须执行 persisted-lease latch handoff：并发 governance continuation 已封存的 lifecycle-delivery lease 要用
  module lifecycle context 挂载 exact timer 并继续持有同一 gate，不能被 caller cancellation 丢失。
- 除 Stop/epoch teardown 外，run gate 的所有退出只允许经过一个 fresh-load settlement reconciler。process-local settlement 只保存
  wake hint，不保存或推导 Runtime phase；只有存在 pending active settlement 时，lifecycle-fenced one-shot timer 才向 framework
  `BackgroundRoutine` 提交短任务，提交拒绝或瞬时 load/CAS/mount 失败必须按 exact epoch/token 重新 arm，空闲时没有轮询任务。
  dormant handoff 只允许等待 exact 当前 gate owner 退出；gate release 与 handoff 注册/激活必须在同一 `driveMu` 临界区线性化，已经
  active 的 settlement 不得被后到的 termination/governance handoff 重新置为 dormant。wakeup、gate、timer、latch 均非业务 authority，
  每次执行仍须从 Run owner aggregate 重建并只调用 Orchestrator-private SM-4 event。
- singleflight key 必须包含 workspace/run。重复 Start、Resume 和 governance continuation 只合并到
  同一 drive。
- recoverable runtime wait必须持久化exact wait identity、reason、recovery condition、next eligible drive/deadline、trigger
  source、budget和guard count。SM-4只按该persisted successor安排退避的reload/re-drive；Web/TUI、projection或人工普通
  Resume不能充当隐式timer/trigger。条件满足、超时、预算耗尽和stale trigger均通过owner-local internal event确定收敛。
- Runtime aggregate 的物理存储可与 Run record 共用 CAS，但 Run owner 必须在未变化的
  `RuntimeState.StateRevision` 上重载并 rebase 无关模型写入；只有 aggregate revision 漂移才返回给
  Orchestrator 作为语义 CAS conflict。CAS recovery ledger 耗尽不改变业务 Run 为 terminal，也不得留下
  `active + nonterminal + no scheduled drive`：SM-4 必须保留 owner-local、退避的 reload/re-drive 后继。
- generic nonterminal recovery 只接受 scoped typed `RuntimeCASConflictError`，或携带 exact Run persistence
  `owner_transient` feedback 的 `RuntimeOwnerTransientError`。普通 `common.ErrConflict`、任意实现 `Temporary/Timeout` 的 error、provider
  transient 与未定位 operation timeout 均不得取得该权限。Orchestrator 由 fresh aggregate revision 和 failed lease generation 派生稳定
  `RuntimeNonTerminalRecoveryRequest`；唯一 `nonterminal_recovery_scheduled` private event 必须在同一 CAS 中写入 latest request/预算
  disposition并建立唯一 request-bound `scheduled` successor。response-loss/duplicate 只按 RequestID 重读既有 record/lease，不能再消费
  budget或另发 `DriveLeaseScheduled`。CAS 与 Run-owner transient 共用 owner-local CAS-reload guard，但后者不得冒充 operation/provider
  recovery；guard exhausted 只使用 capped backoff successor，不能把业务 Runtime 终态化或留下无 successor active aggregate。fresh-load
  还必须证明 `StateRevision >= record.RecordedAtStateRevision`；二者相等时 current lease 必须 normalized exact 等于 record 内嵌 successor，
  防止将撕裂的 record-only 或被替换 lease 状态误认成原子事务结果。
- operation-recovery counter 与 authorization ledger 必须等长且按全局 attempt 连续；每项 identity 必须唯一，并反向匹配
  current/completed sealed dispatch 中 exact execution。fresh-process reload 若发现缺项、重复、顺序漂移、wrong key/fence、
  orphan dispatch 或伪造 `issued/in_flight/uncertain/settled/retired` state，必须在任何 owner query/effect 前 fail closed。
  合法 `issued` 只允许竞争一次 claim；未过期 `in_flight` 只能等待同一 call receipt/return，不能授权第二次 effect；
  `uncertain` 只能作为下一次预算申请的前置审计，`settled/retired` 永不授权 effect。
- governance wait、governance continuation 与 terminal 的 lifecycle handoff identity 必须穷举地从同一份 persisted
  aggregate 派生。普通 Decision/Approval request、Definition clarification request、matching resolution 与 terminal seal
  分别生成唯一 opaque fence；fence 必须与 aggregate 在同一次 Run-owner opaque-record CAS 中持久化，随后发送给 Run 的
  immutable typed fact 必须逐字段使用同一 identity。opaque fence 必须在 governance 与 terminal 两种 mode 中恰选其一，
  terminal seal 不得继续携带历史 governance fence/resolution。Run 仍只把该 fact 映射为自己的 private internal event并独占 SM-1
  transition；Orchestrator 不得直接写 Run state。wait fence 的 resolution identity 必须为空；continuation fence 与
  owner acknowledgement 必须持久化并逐字段匹配 exact `ResolutionID`，已经 active 的 duplicate 也不得跳过该比较，旧/错
  resolution 必须零状态变化 fail-closed。若 exact successor 已消费且 Runtime 已通过该 fence 推进到更晚 authority/新 blocker，
  旧 resolution replay 只能以 persisted request/resolution/evidence history 与 Run 当前 exact aggregate owner proof 收敛为历史
  no-op；不得用当前 aggregate 重建旧 handoff、重投 Run fact或挂载 timer。
- 非终态 governance lifecycle fact 投递不是 projection，也不是 terminal delivery。owner acknowledgement 缺失或响应丢失时，
  SM-4 必须以 exact handoff identity 持久化 `RuntimeDriveLease` 并只重投同一事实；不得增加
  `TerminalDeliveryAttempts`，不得因 terminal-delivery budget耗尽静默遗留 `active + need_governance`。fresh-process inventory
  发现 persisted `need_governance` aggregate 但 Run 仍为 pending/active 时，必须重新驱动该 lifecycle handoff；已经匹配
  blocked 且 aggregate 仍为 `need_governance` 的 Run 保持真实治理等待。若 resolution 已 CAS 持久化而 Run 仍 blocked，startup
  必须把它分类为 continuation delivery cut point，从 persisted resolution 重投 exact continuation，不能先推进 execution。
- 非终态 aggregate 不得携带 `TerminalDeliveryAttempts` 或 terminal acknowledgement；该组合不属于当前持久化结构，
  当前 invariant 校验必须使 owner load 直接失败。不得为该旧形态生成兼容分类或升级状态；projection 不得归零或隐藏，运维侧清理数据后
  重新创建 workspace/Run。
- governance `continue|fail` resolution 的 owner-private CAS 必须同时替换任意旧 lease并封存 exact `scheduled`
  successor；新 private event 不接受 nil/wrong lease template。`continue` 的 successor 先围住 run gate 和 exact
  Run-owner resolution acknowledgement，再由同一 claimed generation 继续；`fail` 的 successor 只授权既有
  Result → Workspace → Run terminal lane，resolution command 本身不得裸投 terminal。partial owner ack 后续排为 canonical
  `terminal-delivery` lease；同一 Fail resolution 的 duplicate 必须验证并加入 current terminal successor，不得要求原始
  resolution signature 永久存在，也不得重写 aggregate或创建第二 timer。resolution ingress 只接受匹配 blocker 类型的
  Decision/Approval owner source；ProcessedEventID 只负责定位，duplicate 还必须逐字段匹配 persisted request、resolution 与
  governance evidence。exact stale diagnostic replay继续返回 stale且零 CAS/Run/timer，payload漂移一律 conflict。
- governance resolution 的外部入口只允许 Decision ticket-resolution 与 Approval approval-resolution 两个 concrete typed
  command。adapter 必须先从 current aggregate 找到 exact ticket/approval binding，再构造 owner-private resolution value；不存在
  generic governance-resolution topic/command，也不允许 Application、Governance 或其它组件以完整 authority-shaped payload
  绕过 binding reconstruction。
- 已 claim 为 `running` 的 lifecycle delivery lease 再次失败时，只有持有 exact generation 的 worker 可以在一次 owner-private
  CAS 中把它续排为更晚 `dueAt` 的新 `scheduled` generation。普通 duplicate/stale scheduler 不得续期 running lease；旧 worker
  的 finish/release 也不得移除新 generation。该路径同样不消费 terminal-delivery budget，并由 fresh startup恢复 persisted
  scheduled/running successor。若 lifecycle resolution 在另一个 worker 持有 run-scoped drive gate 时封存 scheduled lease，
  它不得直接启动 timer；当前 gate owner 的唯一退出路径必须原子保留 gate 并挂载 exact persisted generation，或先释放 gate
  让提交者接管。gate owner 退出后不得留下既无 mount latch、也无 timer/gate successor 的 scheduled lease；不得以
  watchdog/queue/timer 并发 drive 补洞。旧 timer 的所有失败/not-claimed 退出必须原子消费 authoritative newer generation
  或不同 signature 的 persisted mount；adopt newer generation 时必须清理不晚于它的 latch。running renewal CAS 失败时旧
  generation 不得 release；fallback recovery 仍失败则保留本地 gate并登记 exact settlement wake，本进程持续 fresh reconcile，
  crash 后由 startup recovery 接管。普通 Start/Resume 即使到达也不得以 generation zero 绕过 persisted running authority。
  Stop cancel 点必须与 `closing + lifecycle epoch` 原子发布；其后
  尚持 lifecycle read lock 的 handler、已捕获旧 context 的 timer defer 都不得创建新 context/background timer、挂载 zombie
  successor或删除新 epoch gate。BackgroundRoutine submission不得持 `driveMu` 或另一 Stop fence；queued/already-due callback
  在 observe与claim前必须重验 captured cancellation和exact epoch。write fence 内必须二次 cancel并清空全部本地 hint，
  persisted lease保持不变。
- startup 的 Run owner typed inventory 只返回其拥有的 SM-1 state、RuntimeStateRecord 是否存在及 opaque
  revision/ref；不得读取 dispatch/governance/delivery ledger 分类 SM-2/3/4。Run 必须在 Setup 同时提供 scoped opaque
  record query；Orchestrator 通过该 query load 后在 owner-local SM-4 中分类 `active_no_dispatch`、
  `active_sealed_dispatch`、`blocked_governance`、`recoverable_runtime_wait`、`terminal_delivery_pending`。inventory/load 失败必须 fail-fast，
  不能由 application 或 Run owner 猜测。盘点完成但 Run 常规 commands 尚未注册时，Orchestrator 只能暂存本次进程的
  owner-local recovery handoff；仅 Run 的 commands-ready lifecycle fact 可释放当前 drive/lease action。
  readiness 不是 workspace state transition，crash 后 handoff 必须由新进程重新 inventory，且此边界不得依赖
  `Weight()` 或普通 Run Get/Update command。
- Orchestrator framework `Setup` 缺少 EventHub 或 `BackgroundRoutine` 必须 fail-fast；`Run(ctx)` 将 framework context 绑定到
  startup recovery、drive 与 timer。startup 中途失败必须撤销已注册 ingress 并取消本地 lifecycle；`Stop(ctx)` 先关闭新 ingress
  和 context，再按 deadline 等待已有 handler/drive，最后清理本地 timer/gate/dependency。Stop 不删除 persisted lease 或
  operation authorization，fresh startup仍只从 Run inventory + opaque aggregate 恢复；这些进程锁/context不是另一份 SM-4 state。
  framework startup 与 teardown 使用独立优先级：startup 必须先注册 Orchestrator ingress、再启动 Run recovery；正常 teardown
  必须先关闭 Workorchd ingress、再停止 Orchestrator worker，最后才允许 Run/capability owner 拆除 repository 与 subscription。
  不得通过倒置 startup 权重来修复 teardown 顺序。
- inventory pagination必须有cursor visited-set、page/item hard limit并拒绝`A -> B -> A`循环；duplicate commands-ready只释放
  同一owner-local handoff一次，不能重复drive或跳过inventory completeness。
- reducer terminal 已 sealed 后，SM-4 仅重试已持久化 terminal delivery request，等待 Result、Workspace、Run
  owner acknowledgement。Result owner acknowledgement 必须携带其按 sealed material 派生并已持久化的 exact `ResultID`；
  SM-4 只把该 fact 原样交给最终 Run delivery，Run owner将其封存为 `TerminalResultID`，不得自行派生或查询 Result。
  Workspace/Run acknowledgement 禁止携带 `ResultID`。projection health 不参与 reducer，也不是 runtime blocker。
- catalog retirement 后若旧 no-effect binding 仍为 current binding，SM-4 必须先投递 typed `CWIBindingRetired` fact，由
  SM-2 唯一 reducer 退休 exact catalog lineage 并 reopen CWI；事件完成前旧 binding 不可作为 operation authority，也不能把
  provisional `target_grounding_unresolved` 交付。CWI 真正 binding-free 后，SM-4 把新 semantic generation 视为更高优先级的
  owner-local 待办；只有 exact current File-owner TargetSelection 已证明 unresolved，或 semantic transaction 自身 failed，才可
  交付对应 terminal。已有 effect credit 的 binding 不得由 catalog refresh 自动退休。其它 sealed terminal 不得被重规划覆盖；
  administrative termination 走独立 lane。transient 窗口只允许 `semantic_plan` request，operations request 继续 fail-closed。
- current-baseline Restart 的 `run.replacement` 是 Run SM-1 subordinate transaction，使用 Run-owned persisted
  stage/generation lease。clarification resolution、administrative termination intent、SM-4 delivery handoff acknowledgement、
  successor materialization 每一步都先由 Run private event claim，再由 exact result/ack 推进；SM-4 可保存并重投
  `RuntimeTerminationRecord` handoff，但 Run terminal 仍只由 private SM-1 delivery event迁移。process-local task 只唤醒 lease，
  失败/重启不能退化为 application/Orchestrator 第二份 scheduler。already-terminal predecessor 复用这份 transaction，
  只跳过已经完成的 termination/delivery stages；它仍必须由 Run-private claim/materialize/lineage CAS/activation 顺序收敛。

禁止：重启重新 initialize 已有 aggregate、watcher/projection/TUI/Web 驱动 reducer、无 revision 进展的循环、
projection failure 覆盖既有 terminal、重放已知 provider/durable effect。
