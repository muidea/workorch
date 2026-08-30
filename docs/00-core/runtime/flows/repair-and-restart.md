# Repair And Restart Flow

Status: normative

Type: runtime-flow

Last Verified: 2026-08-30

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
failure / missing prerequisite / document-conformance or complete candidate rejection / operation failure / repairable assessment / typed governance dependency
  -> producer factual result becomes Orchestrator internal event
  -> reducer updates typed facts / diagnostics / prerequisites / canonical ConvergenceState
  -> same-run reconverge:
       need_prerequisite
       | empty/non-JSON/strict-schema/semantic rejection -> persisted typed diagnostic -> at most one shared correction (same transaction + stable prefix)
       | incomplete/transport request  -> request-session bounded recovery, no candidate attempt
       | repairable implementation gap -> need_state_change
       | repairable evidence gap       -> need_validation
       | need_assessment -> partial/satisfied/repairable/governance/terminal disposition
       | need_governance -> governed_wait
            OpenRuntimeGovernanceRequest
            ... external Decision / Approval ...
            CommitRuntimeGovernanceResolution (once per resolution ID + snapshot)
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
- coherent completed empty/non-JSON observation，或 complete candidate schema / contract rejection
  → SM-3 先持久化 provider-safe document-shape/field/semantic diagnostic，再决定 typed terminal 或同一 authority 上唯一
  correction（复用 transaction + stable prefix）。provider protocol failure 仍走 request terminal；SM-4 只结算/重驱，不决定 correction
- incomplete document / provider transport failure
  → request-session 有界 continuation/recovery，不创建 candidate attempt
- operation 失败 evidence
  → typed operation feedback 进入 internal event；reducer 决定下一 canonical state
- managed validation 的 zero-work 只表示 exit-zero command 未执行匹配检查；aggregate command 非零退出时，无关 package 的
  `[no test files]` 不能把 build/test failure 降级为 `validation_output_inconclusive`，Shell 必须先发布失败 factual class，再由
  SM-2 对照 sealed Profile convergence 决定 repair 或 terminal
- assessment 返回 repairable gap
  → 结构化 gap 进入 facts；按 missing evidence kind 回 state change / validation，不能直接 terminal
- Profile-declared typed validation rejection without an exact failure target
  → SM-2 reopens the same CWI for one `validation_failure_repair` semantic generation
  → that generation's eligible closure is the exact current CWI; unrelated globally eligible CWIs remain outside it
  → C3 exposes only the typed failure signature/class and exact Profile repair vocabulary; no raw diagnostic or Runtime disposition
  → File confirms the complete prospective all-of target set without creating/snapshotting it, and SM-2 seals a generation binding
  → accepted repair state change retires that binding and requires fresh validation; it never grants terminal validation credit
- Shell-recognized structured validation locations, including both files in the exact Go package-conflict diagnostic, are bounded
  candidate surfaces rather than failure-target authority
  → the rejected receipt retains the original focus/binding and the completed dispatch's sealed `FocusTargetRole`
  → Orchestrator resolves the exact matching `CompletedDispatch.OperationTargetAuthorizations` member by `OperationID` and
  operation hash; its non-empty canonical `Target.AllowedTargetRefs()` is the finite full validation scope, and only an
  `existing_owner_scope` authorization maps that value to canonical `OwnerScopeTargetRefs`
  → before any semantic/candidate provider call, File confirms the current surface through typed command/result and SM-2 internal events
  → confirmed becomes an owner-grounded `FailureTarget`; stale reacquires owner prerequisites; rejected becomes typed infeasibility
  → only a rejection without that structured observation may enter the Profile-declared semantic fallback above
  Shell does not choose the repair transition, and Orchestrator never derives targets from free text, mutable current Focus or a
  current manifest that was not part of the completed dispatch seal
- 需要外部信息 / 审批 / 依赖
  → sealed governance candidate → ticket → resolution commit
- 存在owner-defined typed recovery/reconciliation合同的暂时不可用或consumption unknown
  → persisted recoverable wait；只由matching trigger/deadline进入SM-4 re-drive

structured validation failure 的 target 关系固定为：

| Target | 含义 | 合法关系 |
| --- | --- | --- |
| validation operation entry `TargetRefs` | 选择本次 validation recipe/entry 的 operation target | 只有 sealed per-operation `RequireTargets=true` 才要求非空；finite allowed set 存在时，`operation entry ⊆ sealed full validation scope` |
| sealed finite full validation scope | matching completed-dispatch per-operation authorization 的 canonical `Target.AllowedTargetRefs()` | 只有 `existing_owner_scope` 时才等于 `OwnerScopeTargetRefs`；structured observation scope 必须 canonical exact-match，执行、duplicate 与 restart 原样复用 |
| diagnostic candidate targets | Shell 从 owner-supported structured diagnostic 得到的 actual locations | observation 存在时为 1～8 项且 `candidate ⊆ sealed full validation scope`；仍需 File current-manifest confirmation |

operation entry 与 diagnostic candidate 没有 equality 或 overlap 要求；例如 entry `A`、scope `{A,B}`、candidate `B` 是正常
repair causation。任何把 entry 当成 full scope、把 candidate 强制等于 entry，或用 current manifest 重算 full scope 的路径都必须
fail closed，而不是把合法 rejected validation 误判为 terminal。
`WORKORCH_FAILURE_TARGET` 只是 Shell closed protocol 中的 bounded diagnostic candidate；它不能直接产生或填充
`RuntimeFailureTargetObservation` authority，只能经 finite sealed scope 约束形成 `RuntimeValidationFailureObservation`，再等待 File
confirmation 与 SM-2 transition。selector-only authorization 的 `Target.AllowedTargetRefs()` 为空，没有 finite full scope；即使
operation 仍可由 sealed `RequireTargets` 与 owner feasibility 执行，Shell 也必须保持零 scope-bound structured observation，并按
“无 exact failure target”进入 matching sealed Profile fallback；fallback 不存在时 typed fail closed，禁止用 current manifest 补成
scope。

managed-validation receipt 的外层 `TargetRefs` 只记录 entry execution lineage；其 `OriginalDiagnostic.TargetRefs` 与
`TargetObservation` 必须为空。VFO 只能嵌套在产生它的 exact Shell managed-validation receipt 中，并与 receipt scope、actual
dispatch、authority、operation、signature、typed diagnostic 和 sealed per-operation authorization 全量绑定；reducer copy 与
nested owner-diagnostic copy 必须 presence-parity 且 exact-match。只有 completed/committed dispatch 中
`evidence_committed` execution 的 exact key/fence/ref/hash receipt 可进入该交接；owner-committed-only、invalidated dispatch 与
generic failure ingress 全部 fail closed。pending VFO 不产生
concrete `RepairDirective`。File 是 confirmation 的唯一 owner；SM-2 只有在 exact confirmed record 与 exact Shell receipt 同时
存在时才能发布 File `TargetObservation`/`FailureTarget`，fresh-load 也按此反向证明，不接受 generic failure event 或协调篡改。
同一 VFO 允许 stale revision refresh；首条 confirmed/rejected 终止 confirmation 序列，延迟响应不能覆盖其结果，Runtime 只从
latest valid confirmation 派生当前 target authority。

非 VFO direct-target 只能由 operation descriptor 显式声明 `receipt_evidence_observation` 协议；当前 MCP owner 是该协议的
具体实现。owner 只能在 rejected receipt 中返回显式、bounded、content-addressed、catalog-revision-bound observation；
operation target、request 与 manifest 不能作为 fallback。SM-2 不按 owner 名称分支，仅在 descriptor policy、promoted/nested observation
exact-match，且可反向证明 completed committed dispatch、completed operation、`evidence_committed` execution、exact rejected
receipt key/fence/ref/hash、唯一 receipt-derived rejected evidence/processed-event fence，以及 sealed/current owner catalog
snapshot、acquisition、revision、subject selector 和 prerequisite freshness 时封存 current `FailureTarget`。duplicate/restart
复用同一 receipt-derived identity；`owner_committed`、generic ingress、receipt/evidence/catalog drift 或 stale 均 fail closed。
随后取得的 File target snapshot 只证明 workspace 中的 current target grounding，不是原 capability-owner authority 的替代来源。

confirmed File surface 被 state-change operation 消费时，其 workspace revision 会由 receipt 自身推进。generic freshness
invalidation 仍必须退休旧 owner observations；对于多 operation all-of repair，Orchestrator 只允许在原 active sealed dispatch
上继续已封存但尚未完成的 operation；首个 changed receipt 使 confirmed revision 失效后，旧 `FailureTarget` 与旧
`need_state_change RepairDirective` 必须保持为空，不能作为 partial continuation 恢复。每个 completed member 必须是
File-owned、target 属于 confirmed surface、target authorization exact-match、accepted durable changed receipt，并以一条唯一
workspace-tree revision edge 延续前一 receipt；链从 confirmation revision 开始，不能按 operation ID 猜执行顺序。partial
restart 只从 active dispatch、receipt/evidence/processed-event、exact invalidation 与 current pending prerequisite 全链重验。
最终 accepted member 才在 completing CAS 内从 rejected Shell receipt、File confirmation 与 sealed Decision/dispatch 重算原
closure，并直接生成新的 target-free `need_validation RepairDirective`；该新 directive 不保留旧 `CurrentFailure` target copy。
rejected、refresh-exhausted 或其它 dispatch invalidation 均不恢复旧 revision authority。

confirmed File state-change 在 pre-seal manifest 中还必须携带 typed repair purpose 与 closure identity。schema-valid proposal 的
同一 SM-3 CAS 从 rejected Shell receipt、File confirmation、source evidence 与 closure 派生完整
`RuntimeDispatchRepairCommitment`，先写 operations request session，再由 accepted attempt 与 origin dispatch exact 复制并纳入
dispatch authority hash；governance resume 只能逐 hop 原样继承。fresh-load 先用 manifest purpose 判定 commitment 必填，再从
commitment 反向要求 exact confirmation、source dispatch/operation、receipt key/fence/hash、evidence ID/hash 与 closure，不能从
current `FailureTarget`、`RepairDirective`、Focus 或 mutable resource facts重建用途。删除 commitment、confirmation、outstanding
target-free validation directive，或协调删除这些副本，都必须 fail closed；partial/rejected/invalidated transaction 也必须保留
immutable audit lineage，但只有 all-accepted completed receipt chain 可以创建 fresh-validation continuation。
mandatory detector 还必须独立核对 same-focus exact rejected managed-validation receipt/evidence、其 structured candidate surface 与
all-File state-change seal；因此即使 manifest purpose 与三份 commitment projection 被协调清空，也不能把该 transaction 降级成普通
File dispatch。

若 all-of File repair 的未完成 member 返回 `governance_required`，pause 只提交该 operation 的 owner receipt、committed source
dispatch 与 governance binding；File 不拥有或保存 Runtime continuation。`continue` 由 Orchestrator 的同一个
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
- coherent empty/non-JSON、complete JSON strict-schema rejection 与 semantic rejection 只共用这一份 attempt 0/1，禁止为
  format、schema 或 evaluator 分别建立预算；attempt 0 empty 后，attempt 1 即使首次形成 complete JSON，也没有第二轮 correction
- attempt 0 的 `document_shape` witness 只证明 `$` 必须从 `empty_document | invalid_provider_output` 变为
  `one_complete_json_object`；schema witness 只引用 sealed manifest 的 field/rule；semantic correction 才要求有限 frontier 中的
  concrete alternative。raw provider output、decoder message、summary 或 reason-only delta 均无效
- operations provider wire 禁止 `id`/`dependsOn`；operation ID 与 dependency graph 由 strict local admission 根据
  normalized manifest required-sequence slot 或 manifest-bound auxiliary execution-semantic key 确定性派生；
  `AuthorityTransaction` 只做 exact-match gate，不能把 provider identity/order 变化当作 correction progress
- attempt 1 在 provider call 前重验 persisted manifest/witness，返回结果重新通过完整 strict local admission；semantic
  correction 还必须实际使用 sealed alternative
- attempt 1 的任意再次 document-conformance、schema 或 semantic rejection 都生成 structured candidate exhaustion fact，
  由 SM-3 形成 typed candidate terminal；不得发起第三次 same-contract candidate。governance 只能来自独立的 typed external dependency
- SM-4 只能从 persisted SM-3 result 幂等 re-drive；duplicate result、CAS response loss、普通 Resume 与进程重启均不增加
  attempt、physical call 或预算消费，只加载已有 `CandidateAttempts`/directive/request identity

### 3. Governance-Mediated Recovery

governed dispatch：

1. `OpenRuntimeGovernanceRequestCommand/Result` 向 Decision owner 创建确定性 ticket
2. ticket、snapshot、dispatch、dependency 绑定随 aggregate 持久化
3. Orchestrator 持久化 `governed_wait` 事实；Run owner 消费后在自身 reducer 中进入 `blocked_on_decision`
4. resolution 通过 `CommitRuntimeGovernanceResolutionCommand/Result` 提交
5. 同一 resolution ID 与 authority snapshot **最多消费一次**
6. Decision owner 也可只提交 ticket resolution；Orchestrator 从 persisted binding 补齐后进入同一 commit

规则：

- governance continue：写入 accepted governance evidence、递增 evidence revision、回到同一 reducer
- governance fail：生成 `governance_rejected` terminal
- stale resolution：只进入 diagnostic lineage
- 必须匹配已绑定 ticket；无绑定、同 dispatch 绑定不同 ticket、dependency 漂移 → conflict
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
- replacement recovery 只加载 Run owner 的 `ReplacementIntent.Recovery` 与 terminal lineage；already-terminal predecessor
  从 successor-materialization lease继续，不重新请求 Orchestrator delivery，且 successor 的 predecessor Run/Result 双引用
  必须与 owner record exact-match
- active/completed dispatch 必须原样恢复 per-operation `OperationTargetAuthorizations`、operation hash 与
  `DispatchAuthorityHash`；validation failure causation 只从 matching sealed authorization 恢复 full scope，不从
  `Facts.Resources.Manifest`、current `Focus` 或 fresh owner inventory 重建
- exact observation/confirmation duplicate 复用同一 causation identity，不新建 repair authority、provider request、effect 或
  budget consumption；若 File read-only confirmation 尚未持久化，只能以完全相同 observation 重试该 query
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
