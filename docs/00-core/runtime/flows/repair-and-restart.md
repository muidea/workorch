# Repair And Restart Flow

Status: normative

Type: runtime-flow

Last Verified: 2026-09-04

## Purpose

本文件定义 Runtime 在失败、governance wait、candidate retry 与“需要重新定义”时的恢复语义。

稳定三分：

| 语义 | Runtime 含义 |
| --- | --- |
| **repair** | **同一 Run、同一 focus** 上重新收敛：prerequisite 补齐、candidate attempt 1、operation evidence、governance continue |
| **recoverable runtime wait** | **同一 Run、同一 aggregate** 等待typed recovery condition/trigger；必须有deadline、budget和persisted re-drive successor，不需要人类选择 |
| **restart/replacement** | **不是**同 Run 内自动 successor；需要 fresh baseline 时由显式 Workspace `restart` 提交 fresh bootstrap，并由 SM-1 唯一 Run owner 的 subordinate `run.replacement` transaction 创建独立 Run |

生产主线**不**存在自动 replan successor lifecycle、`ExecutionContextPatch` resume 或 graph 重放。

## State Machine Binding

本 flow 绑定核心规约中的：

- SM-2：same-focus repair、failure signature budget、typed external dependency governance 或 terminal；
- SM-3：same-transaction candidate retry 与 rejected operation evidence；
- SM-1：blocked/active/terminal lifecycle，以及同 owner subordinate replacement transaction 和显式新 Run lineage；
- SM-4：restart/load、governance continuation、administrative delivery handoff 与 bounded no-progress guard；不迁移 Run。

repair 与 restart/replacement 的边界不得由 application queue、自动 successor 或 projection watcher实现。完整规约见
[Runtime State Machines](../state-machines.md)。

## Flow

```text
failure / missing prerequisite / no-candidate framing / complete candidate rejection / operation failure / repairable assessment / typed governance dependency
  -> producer factual result becomes Orchestrator internal event
  -> reducer updates typed facts / diagnostics / prerequisites / canonical ConvergenceState
  -> same-run reconverge:
       need_prerequisite
       | incomplete/empty/invalid framing -> request-session bounded whole-document recovery, no candidate attempt
       | complete-object strict-schema/semantic rejection -> persisted typed diagnostic -> at most one shared correction (same transaction + stable prefix)
       | repairable implementation gap -> need_state_change
       | repairable evidence gap       -> need_validation
       | need_assessment -> partial/satisfied/repairable/governance/terminal disposition
       | need_governance -> governed_wait
            Decision: OpenRuntimeGovernanceRequest / Approval: OpenRuntimeApprovalRequest
            ... external Decision / Approval ...
            CommitRuntimeGovernanceTicketResolution / CommitRuntimeGovernanceApprovalResolution
              (once per resolution ID + persisted binding)
            accepted governance evidence -> same reducer
            governance fail -> governance_rejected terminal
       | typed transient dependency / consumption unknown -> recoverable_runtime_wait
            persisted condition + trigger/deadline + budget
            condition met -> same aggregate re-drive
            deadline/budget/guard exhausted -> typed terminal
  -> if path cannot continue within Runtime invariants or repair budget exhausted:
       structured Terminal Fail  (operator may later submit an explicit Workspace restart)
```

## Main Stages

### 1. Repair = Same Focus Reconverge

repair 在 Runtime 中不是独立 stage 对象，而是 **drive/reducer 在当前 authority 与 focus 上的再推进**：

- 缺失 manifest / existing-target snapshot / capability / validation scratch / provider ceiling
  → `need_prerequisite`（确定性，非 LLM）
- incomplete/empty/invalid framing（含 non-JSON、scalar/array、multiple roots、trailing bytes）
  → request-session 有界 whole-document recovery，不创建 candidate attempt、不创建 correction witness
- one complete JSON object 的 candidate schema / contract rejection
  → SM-3 先持久化 provider-safe field/semantic diagnostic，再决定 typed terminal 或同一 authority 上唯一 correction（复用
  transaction + stable prefix）。provider protocol failure 仍走 request terminal；SM-4 只结算/重驱，不决定 correction
- operation 失败 evidence
  → typed operation feedback 进入 internal event；reducer 决定下一 canonical state
- software-dev Profile specialization 中，managed validation 的 zero-work 只表示 exit-zero command 未执行匹配检查；aggregate command 非零退出时，无关 package 的
  `[no test files]` 不能把 build/test failure 降级为 `validation_output_inconclusive`，Shell 必须先发布失败 factual class，再由
  SM-2 对照 sealed Profile convergence 决定 repair 或 terminal
- assessment 返回 repairable gap
  → 结构化 gap 进入 facts；按 missing evidence kind 回 state change / validation，不能直接 terminal
- Profile-declared typed validation rejection without a structured observation and with one exact matching
  `prospective_owner_target` route
  → SM-2 reopens the same CWI for one `validation_failure_repair` semantic generation
  → that generation's eligible closure is the exact current CWI; unrelated globally eligible CWIs remain outside it
  → C3 exposes only the typed failure signature/class and exact Profile repair vocabulary; no raw diagnostic or Runtime disposition
  → the resource/effect owner named by that route confirms the complete
  prospective all-of target set without applying the effect, and SM-2 seals a generation binding
  → accepted repair state change retires that binding and requires fresh validation; it never grants terminal validation credit
- validation-owner-recognized structured locations are bounded candidate surfaces rather than failure-target authority; the
  software-dev Profile's Shell/Go package-conflict diagnostic is one concrete specialization
  → the rejected receipt retains the original focus/binding and the completed dispatch's sealed `FocusTargetRole`
  → Orchestrator first validates the exact matching `CompletedDispatch.OperationTargetAuthorizations` member by `OperationID`
  and operation hash, then resolves that operation's `OperationExecution.AdmissibilitySnapshotID + AcquisitionBinding` to the
  complete Shell-owner catalog retained in the same dispatch; the catalog's canonical non-empty `TargetRefs` is the finite full
  validation scope
  → the observation diagnostic hash binds its complete typed route tuple plus the sealed validation scope; receipt commit and
  fresh-load recompute the same hash instead of trusting coordinated copies
  → when the exact route matches, its repair operations must share one non-`none` owner confirmation protocol admitted at pre-seal;
  before any semantic/candidate provider call, the selected repair owner confirms the current surface through that concrete typed
  command/result and SM-2 internal events
  → confirmed becomes an owner-grounded `FailureTarget`; stale reacquires owner prerequisites; rejected becomes typed infeasibility
  → a complete structured observation with no exact route seals `validation_failure_repair_route_unavailable` in the same rejected-
  receipt CAS, with zero owner query/provider/effect/governance/wait; a rejection without that structured observation may enter the
  Profile-declared semantic fallback only for an exact `prospective_owner_target` route. An exact `existing_owner_scope` route with
  no structured observation instead seals `repair_surface_unavailable` in the receipt CAS
  → the validation owner does not choose the repair transition, and Orchestrator never derives targets from free text, mutable current
  Focus or a current manifest that was not part of the completed dispatch seal
- capability-owner typed dependency/toolchain/environment diagnostics are not implementation-effect surfaces. For example, Shell
  may classify bounded dependency metadata output as `validation_dependency_mismatch + dependency_contract` and emits zero
  implementation candidate refs; incidental compiler locations cannot enter File confirmation. The built-in software-dev Profile
  deliberately declares no dependency/toolchain/environment repair route. The unmatched exact tuple makes SM-2 seal
  `validation_failure_repair_route_unavailable` in the same rejected-receipt CAS, with no LLM command synthesis, File fallback,
  governance ticket or retry loop. A future project-specific extension must introduce an explicit owner capability and typed route;
  it cannot be encoded as a central default recipe.
- an ordinary capability-owner infeasibility after a `validation_failure_repair` binding is sealed cannot clear the exact repair
  directive while retaining the intermediate binding. One Orchestrator-private CAS invalidates the candidate, retires that binding
  with the persisted owner observation and seals `repair_surface_unavailable`; it neither opens an implicit semantic retry nor enters
  SM-4 delayed recovery.
- File edit/patch candidate-local content mismatch is checked before dispatch through the same owner matcher used for execution.
  Operation-level and selected variant constraints share the same bounded SM-3 correction ledger. A current revision with an
  incorrect oldText is not a stale-resource fact. If an already-sealed validation-repair effect instead produces a rejected owner
  receipt, the completing SM-3/SM-2 CAS retires executable target/directive and any intermediate semantic repair binding, preserves
  immutable receipt/confirmation/commitment causation, and seals `repair_surface_unavailable`. It cannot resurrect predecessor
  authority or open an implicit retry. Typed stale, governance and unknown outcomes retain their own event lanes.
- a repair-purpose semantic document cannot redefine the problem as a Definition gap. The manifest-only admission rejects that
  branch, and one Orchestrator-private semantic-failure event seals `repair_surface_unavailable`; no clarification ticket, restart
  request or second provider attempt is created.
- 需要外部信息 / 审批 / 依赖
  → sealed governance candidate → ticket → resolution commit
- 存在owner-defined typed recovery/reconciliation合同的暂时不可用或consumption unknown
  → persisted recoverable wait；只由matching trigger/deadline进入SM-4 re-drive

structured validation failure 的 target 关系固定为：

| Target | 含义 | 合法关系 |
| --- | --- | --- |
| validation operation entry `TargetRefs` | 选择本次 validation recipe/entry 的 operation target | 只有 sealed per-operation `RequireTargets=true` 才要求非空；Shell owner catalog 存在 finite `TargetRefs` 时，`operation entry ⊆ sealed full validation scope` |
| sealed finite full validation scope | matching completed-dispatch operation execution 所绑定的完整 Shell-owner `OperationAdmissibility.TargetRefs` | operation target authorization 只裁决 entry，不能代替 diagnostic scope；structured observation scope 必须 canonical exact-match，执行、duplicate 与 restart 原样复用 |
| diagnostic candidate targets | validation owner 从自身支持的 structured diagnostic 得到的 actual locations | observation 存在时为 1～8 项且 `candidate ⊆ sealed full validation scope`；仍需 sealed Profile/descriptor 指定的 resource/effect owner current-manifest confirmation |

operation entry 与 diagnostic candidate 没有 equality 或 overlap 要求；例如 entry `A`、scope `{A,B}`、candidate `B` 是正常
repair causation。任何把 entry 当成 full scope、把 candidate 强制等于 entry，或用 current manifest 重算 full scope 的路径都必须
fail closed，而不是把合法 rejected validation 误判为 terminal。
software-dev Profile 中的 `WORKORCH_FAILURE_TARGET` 只是 Shell closed protocol 的 bounded diagnostic candidate；它不能直接产生或填充
`RuntimeFailureTargetObservation` authority，只能经 finite sealed scope 约束形成 `RuntimeValidationFailureObservation`，再等待该
Profile 指定的 repair-owner confirmation 与 SM-2 transition；software-dev 当前具体 owner 为 File。selector-only authorization 的
`Target.AllowedTargetRefs()` 为空，没有 finite full scope；即使
operation 仍可由 sealed `RequireTargets` 与 owner feasibility 执行，Shell 也必须保持零 scope-bound structured observation，并按
“无 exact failure target”进入 matching sealed Profile fallback；fallback 不存在时 typed fail closed，禁止用 current manifest 补成
scope。

managed-validation receipt 的外层 `TargetRefs` 只记录 entry execution lineage；其 `OriginalDiagnostic.TargetRefs` 与
`TargetObservation` 必须为空。VFO 只能嵌套在产生它、且 operation descriptor 声明
`validation_failure_candidates` protocol 的 exact validation-owner receipt 中（software-dev specialization 为 Shell
managed-validation），并与 receipt scope、actual dispatch、authority、operation、signature、typed diagnostic 和 sealed
per-operation authorization 全量绑定；reducer copy 与
nested owner-diagnostic copy 必须 presence-parity 且 exact-match。只有 completed/committed dispatch 中
`evidence_committed` execution 的 exact key/fence/ref/hash receipt 可进入该交接；owner-committed-only、invalidated dispatch 与
generic failure ingress 全部 fail closed。pending VFO 不产生
concrete `RepairDirective`。confirmation 的唯一 owner 与 concrete query protocol 由 sealed Profile exact route 或 source binding
中 state-change descriptor 确定；route 的 operation descriptor 在 pre-seal 必须共同声明同一个非 `none` protocol。SM-2 只有在
confirmation 的 `OwnerModuleID`、candidate/receipt descriptor owner 与该 owner exact-match，且 exact confirmed
record 与 exact validation-owner receipt 同时存在时，才能发布 owner-grounded `TargetObservation`/`FailureTarget`。fresh-load
也按此反向证明，不接受 generic failure event、capability-name branch 或协调篡改。
同一 VFO 允许 stale revision refresh；首条 confirmed/rejected 终止 confirmation 序列，延迟响应不能覆盖其结果，Runtime 只从
latest valid confirmation 派生当前 target authority。

Orchestrator 以唯一 owner-private confirmation plan 决定是否执行该 read-only query。plan 必须从 reload 后的 current aggregate、
exact VFO sequence 与当前 repair transaction lineage 派生；不得把 `FailureTarget == nil` 当作进度状态。delayed drive 若发现 revision
已推进，只返回 current aggregate 交由正常 drive 重算。confirmed/rejected terminal、active partial repair、target-free
fresh-validation 与 administrative termination 均不得再次查询 repair owner；termination 后到达的新 response fail closed，已持久化的
exact response duplicate 保持零 CAS。confirmation CAS 前发生 crash/response loss 时，同一 observation 的 read-only query 是
at-least-once；CAS 后必须 zero-query。workspace read、owner query 或 confirmation event 任一 await 返回 error 时，adapter 都先 reload；
revision 已推进则交回 current aggregate 继续正常 drive/terminal delivery，而不是生成新的 semantic failure。

每次 required plan 还必须封存派生的
`RequestID + IssuedAtStateRevision + protocol-bound SubjectRef + ExpectedSubjectRevision`。protocol descriptor 提供 owner-neutral 的
resource domain/subject 元数据，plan 只能从 matching current owner prerequisite observation 派生 expected revision；不得在 Runtime
core 按 File、manifest kind 或业务语言分支。`RequestID` 绑定 observation、owner、protocol、subject、expected subject revision、current
revision 与 stale refresh lineage；同一个 current plan 同时裁决 read-only query 与 writer CAS。File 只按 concrete typed command 做
当前 surface read并原样回显全部 fence：actual workspace-tree revision 与 expected 不同时只能返回 `stale` 且零 target；二者相同时
才可返回 `confirmed | rejected`。SM-2 confirmation writer fresh-load 后再次校验完整 plan；`stale` 必须在同一 SM-2 CAS 退休 exact
dependency 上的旧 owner facts、记入 invalidation lineage 并消耗独立 prerequisite-refresh budget，不能消耗 repair/validation-only budget
或建立 target authority。成功 CAS 写入的 record 必须满足
`RecordedAtStateRevision == IssuedAtStateRevision + 1 == resulting StateRevision`，opaque reload 重验该 revision chain、request
identity 与 sequence subject。confirmation history 在不同 VFO/observation 之间同样按 aggregate append 顺序严格递增；后一
request window 必须晚于前一 recorded revision，不能复用、逆序或重叠。stale response 触发 prerequisite refresh 后，即使同一 VFO 再次进入 `need_validation`，新的 plan
fence 也必须不同；旧窗口中在途的 confirmed/rejected response 不得跨越 reopen 边界创建 target authority。

非 VFO direct-target 只能由 operation descriptor 显式声明 `receipt_evidence_observation` 协议；当前 MCP owner 是该协议的
具体实现。owner 只能在 rejected receipt 中返回显式、bounded、content-addressed、catalog-revision-bound observation；
operation target、request 与 manifest 不能作为 fallback。SM-2 不按 owner 名称分支，仅在 descriptor policy、promoted/nested observation
exact-match，且可反向证明 completed committed dispatch、completed operation、`evidence_committed` execution、exact rejected
receipt key/fence/ref/hash、唯一 receipt-derived rejected evidence/processed-event fence，以及 sealed/current owner catalog
snapshot、acquisition、revision、subject selector 和 prerequisite freshness 时封存 current `FailureTarget`。duplicate/restart
复用同一 receipt-derived identity；`owner_committed`、generic ingress、receipt/evidence/catalog drift 或 stale 均 fail closed。
随后取得的 resource-owner target snapshot 只证明 current target grounding，不是原 diagnostic capability-owner authority 的替代来源。

confirmed owner surface 被 matching-owner state-change operation 消费时，其 workspace revision 会由 receipt 自身推进。generic freshness
invalidation 仍必须退休旧 owner observations；对于多 operation all-of repair，Orchestrator 只允许在原 active sealed dispatch
上继续已封存但尚未完成的 operation；首个 changed receipt 使 confirmed revision 失效后，旧 `FailureTarget` 与旧
`need_state_change RepairDirective` 必须保持为空，不能作为 partial continuation 恢复。每个 completed member 必须是
`RepairOwnerModuleID`-owned、target 属于 confirmed surface、target authorization exact-match、accepted durable changed receipt，并以一条唯一
workspace-tree revision edge 延续前一 receipt；链从 confirmation revision 开始，不能按 operation ID 猜执行顺序。partial
restart 只从 active dispatch、receipt/evidence/processed-event、exact invalidation 与 current pending prerequisite 全链重验。
最终 accepted member 才在 completing CAS 内从 rejected validation-owner receipt、repair-owner confirmation 与 sealed Decision/dispatch 重算原
closure，并直接生成新的 target-free `need_validation RepairDirective`；该新 directive 不保留旧 `CurrentFailure` target copy。
它保留同一 closure ID，但 `Purpose` 必须为空；后续 pre-seal manifest 是普通 Definition validation，不得再次触发
`RuntimeDispatchRepairCommitment`。`confirmed_validation_repair` 只属于前一项 `need_state_change` repair-effect transaction，
不能同时表示“修复动作”和“修复后验证”两个相位。
rejected、refresh-exhausted 或其它 dispatch invalidation 均不恢复旧 revision authority。

confirmed validation state-change 在 pre-seal manifest 中还必须携带 exact purpose `confirmed_validation_repair` 与 closure
identity。schema-valid proposal 的同一 SM-3 CAS 从 rejected validation-owner receipt、repair-owner confirmation、source
evidence 与 closure 派生完整
`RuntimeDispatchRepairCommitment`，先写 operations request session，再由 accepted attempt 与 origin dispatch exact 复制并纳入
dispatch authority hash；commitment 的 `RepairOwnerModuleID` 必须等于 confirmation、所有 state-change descriptor 与 receipt
producer 的唯一 owner，governance resume 只能逐 hop 原样继承。fresh-load 先用 manifest purpose 判定 commitment 必填，再从
commitment 反向要求 exact confirmation、source dispatch/operation、receipt key/fence/hash、evidence ID/hash 与 closure，不能从
current `FailureTarget`、`RepairDirective`、Focus 或 mutable resource facts重建用途。删除 commitment、confirmation、outstanding
target-free validation directive，或协调删除这些副本，都必须 fail closed；partial/rejected/invalidated transaction 也必须保留
immutable audit lineage，但只有 all-accepted completed receipt chain 可以创建 fresh-validation continuation。
mandatory detector 还必须独立核对 same-focus exact rejected validation receipt/evidence、其 structured candidate surface 与
single-`RepairOwnerModuleID` state-change seal；因此即使 manifest purpose 与三份 commitment projection 被协调清空，也不能把该
transaction 降级成普通 dispatch。

fresh-validation continuation 尚未取得 matching proof 时，合法非终态 aggregate 必须保留上述 exact target-free directive；但
Orchestrator-private failure transition 可以在同一个 CAS 中以 exact failure fact 退休它并封存 SM-2 failed terminal。该原子
terminal closure 不删除 completed repair audit lineage，也不授权 SM-4 重建 authority；SM-4 必须进入 terminal delivery，不能因
continuation 已退休而循环创建 `persisted_pending` lease。

repair receipt、causation 与 terminal credit 必须保持分离：accepted owner receipt 的 payload（包括 `EvidenceReason`）原样进入
`EvidenceFact`；source `BindingRefs` 原样保留。若该 validation source binding 没有 `state_change` proof，sealed repair
commitment 将其分类为 causation-only，`CoverageWorkItemIDs` 保持为空且不产生 `coverage_binding_mismatch`。只有后续 fresh
validation 的 matching proof 才能取得 terminal credit；无 exact repair commitment 的同形 binding/proof mismatch 不享受此例外。

若 all-of confirmed validation repair 的未完成 member 返回 `governance_required`，pause 只提交该 operation 的 owner receipt、
committed source dispatch 与 governance binding；repair owner 不拥有或保存 Runtime continuation。`continue` 由 Orchestrator 的同一个
`governance_resolution_observed` private CAS 从 origin accepted admission、confirmed surface、source candidate/dispatch seal、已提交
receipt chain 与 exact grant 反向证明剩余 transaction，并只为同一 candidate 的 granted operation 封存 next-fence resume
dispatch。该 CAS 把 exact resolution、granted binding 与 source sealed dispatch 作为 typed owner-private event 交给 SM-2 唯一
`ReduceRuntimeStateWithDirective`；reducer 从 immutable source seal 派生并重封新的 governance evidence revision authority，SM-3
只据此构造 resume dispatch。transition 不得在 reducer 返回后手工覆盖 Decision；该路径不调用 LLM、不恢复旧
`FailureTarget`/directive，也不允许替换或新增 operation；`fail` 不生成 resume 或
fresh-validation authority。single/multi-operation、partial-before-pause、duplicate 与 fresh-load 都必须复用同一 lineage。

不得：

- 广泛重新规划为新的 禁止的 graph
- 第三次 candidate 请求
- accepted 后无 dispatch 重试
- 用 wrapper / schema 外壳覆盖原 failure surface 的权威身份（diagnostic 可记录，但 authority 仍由 facts 决定）
- 把unknown/unavailable一律重试、把普通Resume/UI refresh当作wait trigger，或留下无deadline/后继的active Run

### 2. Candidate Attempt Budget

- attempt 0 与 attempt 1 状态随 aggregate 持久化
- attempt 1 必须复用相同 `AuthorityTransaction` 与 `StablePrefixHash`
- incomplete/empty/invalid framing 不进入 attempt ledger，只消费 whole-document recovery；只有 complete JSON object 的
  strict-schema/contract/semantic rejection 共用 attempt 0/1，禁止为 schema 或 evaluator 分别建立预算
- attempt 0 witness 只允许引用 sealed manifest 的 schema/parameter-entry/owner-constraint/operation-set rule 或有限 semantic
  frontier 中的 concrete alternative。raw provider output、decoder message、summary、document shape 或 reason-only delta 均无效
- operations provider wire 禁止 `id`/`dependsOn`；operation ID 与 dependency graph 由 strict local admission 根据
  normalized manifest required-sequence slot 或 manifest-bound auxiliary execution-semantic key 确定性派生；
  `AuthorityTransaction` 只做 exact-match gate，不能把 provider identity/order 变化当作 correction progress
- attempt 1 在 provider call 前重验 persisted manifest/witness，返回结果重新通过完整 strict local admission；semantic
  correction 还必须实际使用 sealed alternative
- attempt 1 的任意再次 complete-object schema/contract 或 semantic rejection 都生成 structured candidate exhaustion fact，
  由 SM-3 形成 typed candidate terminal；不得发起第三次 same-contract candidate。governance 只能来自独立的 typed external dependency
- SM-4 只能从 persisted SM-3 result 幂等 re-drive；duplicate result、CAS response loss、普通 Resume 与进程重启均不增加
  attempt、physical call 或预算消费，只加载已有 `CandidateAttempts`/directive/request identity

### 3. Governance-Mediated Recovery

governed dispatch：

1. Decision 路径通过 `OpenRuntimeGovernanceRequestCommand/Result` 创建确定性 ticket；Approval 路径通过
   `OpenRuntimeApprovalRequestCommand/Result` 创建确定性 approval
2. ticket/approval、snapshot、dispatch、dependency 绑定随 aggregate 持久化
3. Orchestrator 持久化 `governed_wait` 事实；Run owner 消费后在自身 reducer 中进入 `blocked_on_decision`
4. owner 只可通过 concrete `CommitRuntimeGovernanceTicketResolutionCommand/Result` 或
   `CommitRuntimeGovernanceApprovalResolutionCommand/Result` 提交；Orchestrator 从 persisted request 重建 authority
5. 同一 resolution ID 与 authority snapshot **最多消费一次**
6. Decision owner 也可只提交 ticket resolution；Orchestrator 从 persisted binding 补齐后进入同一 commit

规则：

- governance continue：写入 accepted governance evidence、递增 evidence revision、回到同一 reducer
- governance fail：生成 `governance_rejected` terminal
- stale resolution：只进入 diagnostic lineage
- 必须匹配已绑定 ticket/approval；无绑定、同 dispatch 绑定不同 owner request、dependency 漂移 → conflict
- `runtime_governance` resolution committed 后 Orchestrator 发布 immutable continuation/terminal fact；Run owner内部迁移后通知 drive；application resolved watcher **不得**再次发送 Resume 形成双驱动

### 4. Replacement Semantic = Explicit Workspace Restart

Runtime/Orchestrator **不自动创建** replan successor Run；HTTP/TUI 也不提供 `intent=replan`。

```text
operator
  -> Workspace execution intent=restart + fresh RuntimeBootstrap
  -> Workspace owner validates current definition/scope
  -> Run owner selects the sole non-terminal predecessor, otherwise latest terminal predecessor by owner generation
  -> Run owner persists predecessor ReplacementIntent
       non-terminal: claim termination -> Orchestrator SM-4 delivery handoff
                     -> Result owner ack(ResultID) -> Workspace release -> Run private SM-1 terminal event
       terminal: require existing TerminalDelivery + TerminalResultID and continue at successor materialization
  -> Run-private reducer materializes successor with PredecessorRunID + PredecessorResultID
  -> predecessor lineage CAS records the successor before successor activation
  -> successor independently enters DriveRuntime
```

约束：

- application、Workspace、Orchestrator 和 projection 都不能提前创建或链接 successor；
- terminal predecessor 不得重开 lifecycle、重投 terminal delivery 或丢弃 Result lineage；Run ID/时间戳不得代替
  `WorkspaceRunGeneration` 选择 latest predecessor；
- predecessor 的 active focus、stale snapshot、attempt/receipt/evidence 不复制为 successor authority；
- fresh bootstrap 可以携带有界 definition-change/governance history，但其内容是显式 typed contract；
- `Resume` 不能通过 patch 或 loaded Run 改写定义；
- 无法按当前结构读取或校验的 predecessor 在 repository/owner load 边界失败，不进入 Restart admission，也不持久化 replacement intent；
- 相同 request/intent/terminal delivery/terminal Result 重放只能得到同一 successor；Result identity 漂移必须 conflict。

### 5. Restart Recovery

- 服务重启只加载 Run owner 中的 `RuntimeStateRecord` aggregate
- administrative termination 可在 owner-private CAS 中退休当时 running lease `G`；该旧 worker 只能在 terminal delivery 尚未完整
  acknowledgement 时建立 canonical `delivery_retry/terminal-delivery` 的 `G+1`。旧 timer 的 `end(G)` 不得删除 `G+1` registration。
  final ack 在 budget/lease CAS 前胜出时必须零 successor；exact-running cleanup、budget 或 new scheduled-successor CAS 已提交后返回
  普通 response-loss error 时，fresh load 必须分别识别 terminal-complete、已消费 ledger 或 exact scheduled lease，不重复预算、
  generation、timer，也不留下 zombie gate/lease。
- retired marker 只授权并审计 termination 时首个 exact `G -> G+1` handoff；一旦该 request 被消费，它只是历史记录。后续 terminal
  recovery 的 current exact request `R(F)` 若因 aggregate revision drift 使已调度的 `F+1` 退休，startup recovery 必须按当前 request
  重物化同一 exact `F+1`，不能被历史 marker 阻断、回退到首代 request、重复消费 budget 或另增 generation。
- synchronous `DriveRuntime` 与 scheduled worker 共用 workspace/run singleflight gate，并在 owner work 前检查 Run-owner durable
  retirement；同步入口持 gate 后还要 fresh-load 并拒绝任何 persisted lease。退出时即使 caller context 已取消，也必须用 module
  lifecycle context 挂载并发 latched 的 exact lifecycle-delivery lease timer，继续由同一 gate 持有。
- typed CAS conflict 与带 exact Run persistence feedback 的 owner transient 只进入一个 `RuntimeNonTerminalRecoveryRequest` 事务；
  Orchestrator 在同一 private CAS 写 request record、CAS-reload guard disposition 与 request-bound scheduled lease。outer drive 不再
  单独拼装或调度 `cas_reload` lease；duplicate/response-loss/restart 仅复用同 RequestID，guard exhausted 使用 capped successor但不改变
  业务终态。同一 recorded revision 的 persisted state 必须同时保有 normalized exact successor lease；record 超前、record-only 或同 revision
  被其他 lease 替换均在 fresh-load 时 fail closed。普通 conflict、文本 timeout/Temporary、provider 与 operation error 均不能借该入口取得重驱 authority。
- 任意非 Stop owner exit 若不能立即证明完成或挂载 durable lease，必须保留 exact gate 并登记 process-local settlement wake。
  该 wake 只在 pending 时以 lifecycle/epoch/gate-token fenced one-shot timer提交 framework short task；每次 task 都 fresh-load aggregate，
  submission/load/CAS/mount 失败时 exact re-arm。dormant termination/governance handoff 只能由 exact gate owner exit 激活，active settlement
  不得重新休眠；release、handoff 与 generation timer 的本地线性化不能创建第二份 Runtime phase/authority。
- replacement recovery 只加载 Run owner 的 `ReplacementIntent.Recovery` 与 terminal lineage；already-terminal predecessor
  从 successor-materialization lease继续，不重新请求 Orchestrator delivery，且 successor 的 predecessor Run/Result 双引用
  必须与 owner record exact-match
- active/completed dispatch 必须原样恢复 per-operation `OperationTargetAuthorizations`、operation hash、matching
  `OperationExecution`、完整 `OperationAdmissibilities` 与 `DispatchAuthorityHash`；validation failure causation 只从 operation
  execution 所绑定的 sealed Shell-owner catalog 恢复 full scope，不从 `Facts.Resources.Manifest`、current `Focus` 或 fresh owner
  inventory 重建
- exact observation/confirmation duplicate 复用同一 causation identity，不新建 repair authority、provider request、effect 或
  budget consumption；若 sealed repair owner 的 read-only confirmation 尚未持久化，只能以完全相同 observation 重试该 query
- scope 缺项/附加项、operation/candidate 越界、missing/duplicate authorization、operation hash 漂移或 observation identity
  漂移在 owner transition 前 fail closed，不能由 fresh manifest “修复”历史 authority
- 不记录 / 不恢复 禁止的 `runtime_step_result_available`、watchdog recovery policy 等 checkpoint
- 不自动重放 sealed 但未完成的进程内 executor 假象；推进只经 `DriveRuntime` 与 typed evidence/resolution
- 重复 Drive 必须幂等；无持久化进展 fail-fast

### 6. What Is Not On This Path

- 禁止的 recovery action 矩阵作为第二套状态机（`retry_same_step` 自动建 Step 等）
- `dispatched` Step 进程内 executor 重启自动重派
- application continuation queue / patch resume facade
- auto-governance 静默创建 successor Run
- context gate 伪装成普通 state-changing repair 但绕过 prerequisite / candidate seal

Context 前置失败（缺 snapshot、缺 capability 等）应优先进入 `need_prerequisite`；只有独立 typed external
dependency 才可进入 `need_governance`，
而不是伪造 state-changing candidate。

## Key Flow Guarantees

- repair 继承当前 definition revision、focus 与 authority 约束
- failure / diagnostic 分层：stale 只进 diagnostic lineage
- fresh replacement = 显式 Workspace restart + SM-1 sole-owner subordinate Run transaction，不是第五个状态机或
  Orchestrator/application 自动 successor
- governance resolution 单次消费并回到同一 reducer
- Start/Resume 生产入口只向 Run owner 发送 intent；缺少 bootstrap 通过 owner-local terminal delivery 收敛

## Output

repair / recovery 的正式输出可以是：

- 同一 Run 上继续的 drive transition（prerequisite / candidate / evidence / governance wait）
- typed terminal Fail（含 governance_rejected）
- 显式 Workspace restart 后由 Run-private reducer创建的独立 successor Run

## Source Documents

- [overview.md](overview.md)
- [Activation And Definition](../../definition-convergence/activation-flow.md)
- [Planning And Execution](planning-execution.md)
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`（`runtime_governance.go`、`runtime_continuation.go`）
