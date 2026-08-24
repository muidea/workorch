# Runtime State Machines

Status: normative

Type: core-contract-part

Last Verified: 2026-08-24

## 四个协调状态机

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
- blocked 回 active 必须有匹配的 persisted governance resolution。
- complete/fail 只能来自 SM-2 sealed terminal decision；cancel/supersede 必须先持久化
  `RuntimeTerminationRecord`。
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
  recipe/check/metric/artifact 门禁。SM-2 只能从 sealed dispatch 的 `BindingRef`、owner receipt 与 persisted binding
  逐字段匹配后派生 CWI coverage；candidate 的 raw `coverageWorkItemIds`、assessment claim、非空 `PassedChecks`、
  exit code 0、历史 summary、memory、projection 和前一 focus evidence 都不能关闭当前 coverage。
- repairable gap 保持同一 focus，必须持久化结构化缺口。有效新 evidence 不清零同 focus 总 repair
  budget；只有原子 focus advance 才开启新的 repair ledger。
- LLM assessment 中的 disposition、缺失 evidence kind、目标和 reason 都只是 observation/original diagnostic，
  不是 Runtime action。SM-2 必须仅从已持久化的 current-focus coverage、accepted receipt/evidence quality、封存的
  focus/target policy、owner-grounded repair surface 及 freshness 生成唯一的 `RuntimeRepairDirective.RequiredState`。
  初次缺少有效验证只能进入 validation；已存在有效 current-focus validation、coverage 仍未收窄且存在新鲜
  owner-grounded implementation/effect repair surface 时才可进入 state change；accepted repair state change 必须按同一
  closure 固定进入 validation。repair surface 缺失/陈旧只能进入 prerequisite；无合法 target 或 validation recipe 时形成
  typed terminal。validation command entry 不得自动晋升为 repair target，且 validation-only budget 只能在 SM-2 已
  canonical 分类为 validation repair 后扣减。Shell 只能从 owner 支持的结构化工具诊断提取 manifest-bounded candidate
  target；无结构化位置时不得回填 command/input target，随后仍由 File owner confirmation 与 SM-2 freshness gate 封存
  failure target。原始 observation 必须完整保留，但不得反向选择 action、retry、ticket 或 terminal。
- assessment evidence gate 只验证 current-run/focus、binding-matched owner receipt、effect、quality 与封存的
  focus evidence policy；它不得把 assessment observation、claim signal 或 `PassedChecks` 非空解释为 CWI credit，
  也不得改写 observation 的 disposition、items、gap 或 refs。profile 的 observable validation recipe 只能约束候选
  选择该命令时的必要参数；只有 receipt 实际报告 binding 要求的 exact check/metric/artifact 才可满足该 binding，
  不得用 Goal/DoD、summary、operation ID 或检查名之间的自由文本相似度裁决 coverage。
- assessment 必须先独立证明 sealed binding 的其余 evidence/effect/quality 条件，再检查 current owner-fact postcondition。
  只有“其余 proof 全部成立、owner fact 是唯一缺口”时，SM-2 才能在提交 assessment dispatch 的同一 CAS 中把 current
  bound CWI 记为 `reopened`、清除 current binding、更新 reconciliation contract，并以 typed reopen lineage 退休当前
  semantic generation。旧 binding/receipt/evidence/assessment 只保留 audit；SM-4 随后从同一 aggregate 幂等驱动新
  semantic generation。普通 evidence gap 不得伪装为 owner-fact gap；新 semantic proof admission 只能接受 pre-seal
  closure 中 current all-of dependency 已就绪的 exact acquisition owner/kind/variant/target-policy/operation-selector。
  Definition target selector 只保留声明 lineage，不能替代 capability operation selector。SM-2/SM-3 必须先把二者
  编译为 manifest 内唯一的 per-CWI `RuntimeSemanticProofObligation`，provider schema、typed correction 与 binding seal
  全部消费该 obligation；batch 必须恰含一项，不能让 LLM、Shell、File 或后续 aggregate read 另选动作。预算按实际
  accepted acquisition operation 记账而不是按 provider replan 次数记账；当前 `MaxAcquisitionAttempts=1`，reobserve 后仍
  缺失即以 `owner_fact_convergence_exhausted` fail-closed。duplicate/restart 不重复
  reopen、request、effect 或预算扣减。重开 CWI 固定返回 `semantic-pending/<cwi>`；当新的 current-focus acquisition 与
  上一 generation 已退休 prerequisite 使用同一 canonical ID 时，SM-2 必须在排入 pending 前原子移出该 retired ID 及其
  旧 observation，且不得复用旧 observation。无关 retired lineage 必须保留，审计由 CWI reopen lineage 与 semantic
  generation history 持续承载，active/retired 不相交 invariant 保持严格成立。历史 reopened/retired binding 不得再次成为
  `CurrentBindingRef`。导致 owner-fact gap 的 accepted state-change operation shape 必须作为同一 CWI 的 anti-replay fact
  跨 successor semantic focus 保留；SM-2 不携带 predecessor repair attempt/gap/directive，SM-3 在 dispatch 前拒绝等价
  shape，因此新 binding generation 不能重置等价副作用的防重放边界。
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
- capability/provider unavailable、consumption unknown 或 owner dependency 暂时不可用时，只有已经存在 owner-defined typed
  recovery/reconciliation contract、明确 recovery condition、deadline/trigger 与可用 budget，SM-2 才可持久化
  `recoverable_runtime_wait` fact。缺少任一条件、guard/budget耗尽或owner明示不可恢复时必须形成typed terminal；
  不得用free-form retry hint创建wait。

禁止：SM-2 调 provider、执行 operation、直读其他 owner repository、将 profile/软件领域语义写进 reducer、
用数量而非质量关闭 evidence gate。

### SM-3：Candidate and sealed-dispatch transaction

```text
sealed authority + preflight ready
  -> persisted request session
  -> complete document -> candidate attempt 0 -> optional attempt 1
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
- 只有完整 document 才可创建 candidate attempt。attempt 0/1 必须使用同一 transaction 与 stable prefix；
  第二次完整 schema/contract rejection 形成 typed `candidate_retry_exhausted`，不存在 attempt 2。
- renderer、wire DTO、strict decoder、admission、owner feasibility 与 fixture 对同一
  `CandidateContractManifest` 给出一致解释；候选只能选择 manifest 已封存的 `bindingRefs`。未知字段、参数、
  target、effect、dependency、operation kind、未知 binding 或裸 `coverageWorkItemIds` 不得由第二解析路径静默放宽。
- 单一 evaluator 必须在 seal 前拒绝同一 candidate 中重复的 durable `(OwnerModuleID, TargetRef)`，并要求 provider 将同目标
  变更合并为一个 operation/receipt；普通 `DependsOn` 不能放宽，因为所有 replay proof 均在首个 effect 前封存。只有
  owner fact 明确封存、逐项绑定 exact requirement ref/variant 的 `RequiredOperationSequence` 可在同一 owner-target 上执行；
  不同目标与 non-durable operation 不受该唯一性约束。
- accepted candidate 必须在执行前持久化 `SealedDispatch`。operation 依 sealed dependency order 执行，
  每项使用稳定的 `ExecutionKey + Fence`；owner receipt 是 evidence 的唯一副作用回执。
- claimed operation 在没有 receipt 时，只能由 SM-3 返回一个只读的 recovery-budget handoff；SM-3 不得直接修改
  SM-4 的 recovery ledger。后续 `recovery_budget_consumed` private event 由 SM-4 独立 CAS 提交预算，并以
  `operation ID + exact persisted recovery attempt` 作为下一次安全重放的授权 fence；授权字段不属于其他 transaction
  fact，缺失、半截或错用的 envelope 必须在 aggregate load 前拒绝。
- 同一 dispatch 内 accepted receipt 造成的资源 revision 变化是内生事实：它可以使后续 SM-2 prerequisite
  observation 失效，但不得仅因此结束尚有 sealed operation 的 active dispatch。全部 operation 成功或任一
  operation rejected/stale 后，SM-3 才结算 dispatch 并交回 SM-2 reduce。
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

禁止：未持久化 request session 请求 provider、未完整 document 消耗 attempt、accepted candidate 不 seal
直接执行、executor/owner 选择下一 action、owner 直接终态化 Run/Result、依错误文本猜 target 或创建 ticket。

### SM-4：Drive, recovery and projection delivery

- `DriveRuntimeCommand/Result` 是 Orchestrator 唯一同步推进合同。Application 不保留 continuation queue；
  Run owner state notification 才能投递 Orchestrator 的内部 drive event。
- 每个非等待、非终态 transition 必须持久化递增 `StateRevision`；无进展或超过 guard 必须 typed
  fail-fast，不能在进程内空转。
- singleflight key 必须包含 workspace/run。重复 Start、Resume 和 governance continuation 只合并到
  同一 drive。
- recoverable runtime wait必须持久化exact wait identity、reason、recovery condition、next eligible drive/deadline、trigger
  source、budget和guard count。SM-4只按该persisted successor安排退避的reload/re-drive；Web/TUI、projection或人工普通
  Resume不能充当隐式timer/trigger。条件满足、超时、预算耗尽和stale trigger均通过owner-local internal event确定收敛。
- Runtime aggregate 的物理存储可与 Run record 共用 CAS，但 Run owner 必须在未变化的
  `RuntimeState.StateRevision` 上重载并 rebase 无关模型写入；只有 aggregate revision 漂移才返回给
  Orchestrator 作为语义 CAS conflict。CAS recovery ledger 耗尽不改变业务 Run 为 terminal，也不得留下
  `active + nonterminal + no scheduled drive`：SM-4 必须保留 owner-local、退避的 reload/re-drive 后继。
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
- 已 claim 为 `running` 的 lifecycle delivery lease 再次失败时，只有持有 exact generation 的 worker 可以在一次 owner-private
  CAS 中把它续排为更晚 `dueAt` 的新 `scheduled` generation。普通 duplicate/stale scheduler 不得续期 running lease；旧 worker
  的 finish/release 也不得移除新 generation。该路径同样不消费 terminal-delivery budget，并由 fresh startup恢复 persisted
  scheduled/running successor。若 lifecycle resolution 在另一个 worker 持有 run-scoped drive gate 时封存 scheduled lease，
  它不得直接启动 timer；当前 gate owner 的唯一退出路径必须原子保留 gate 并挂载 exact persisted generation，或先释放 gate
  让提交者接管。gate owner 退出后不得留下既无 mount latch、也无 timer/gate successor 的 scheduled lease；不得以
  watchdog/queue/timer 并发 drive 补洞。旧 timer 的所有失败/not-claimed 退出必须原子消费 authoritative newer generation
  或不同 signature 的 persisted mount；adopt newer generation 时必须清理不晚于它的 latch。running renewal CAS 失败时旧
  generation 不得 release；fallback recovery 仍失败则保留本地 gate并等待 Stop/fresh startup，普通 Start/Resume 即使到达也
  不得以 generation zero 绕过 persisted running authority。Stop cancel 点必须与 `closing + lifecycle epoch` 原子发布；其后
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
- inventory pagination必须有cursor visited-set、page/item hard limit并拒绝`A -> B -> A`循环；duplicate commands-ready只释放
  同一owner-local handoff一次，不能重复drive或跳过inventory completeness。
- reducer terminal 已 sealed 后，SM-4 仅重试已持久化 terminal delivery request，等待 Result、Workspace、Run
  owner acknowledgement。projection health 不参与 reducer，也不是 runtime blocker。
- catalog retirement 后若 current eligible CWI 尚无 sealed binding，SM-4 必须把 semantic generation 视为更高优先级的
  owner-local 待办；该窗口中由缺少 binding 产生的 `target_grounding_unresolved` view 不能生成 drive result、startup
  terminal-delivery classification 或 Result delivery request。current semantic operation authority ready 或 semantic
  transaction 自身 failed 后才满足该 convergence terminal 的 delivery readiness。其它 sealed terminal 不得被重规划覆盖；
  administrative termination 走独立 lane。此例外只允许 `semantic_plan` request；operations request 继续 fail-closed。
- current-baseline Restart 的 SM-1 replacement 使用 Run-owned persisted stage/generation lease。clarification resolution、
  administrative termination、successor materialization 每一步都先由 Run private event claim，再由 exact result/ack 推进；
  process-local task 只唤醒 lease，失败/重启不能退化为 application/Orchestrator 第二份 scheduler。

禁止：重启重新 initialize 已有 aggregate、watcher/projection/TUI/Web 驱动 reducer、无 revision 进展的循环、
projection failure 覆盖既有 terminal、重放已知 provider/durable effect。
