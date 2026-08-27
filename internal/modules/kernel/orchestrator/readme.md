# 运行时编排组件

Status: active

Type: kernel-module-implementation-contract

Last Verified: 2026-08-27

## 功能范围与边界

`kernel/orchestrator` 是 Runtime convergence authority，负责 typed facts、authority transaction、prerequisite、candidate、sealed dispatch、operation evidence、governance wait、sealed terminal 与 SM-4 delivery acknowledgement。它组合多个 owner 和技术 block 完成同一 Run 的确定性收敛闭环，**但不拥有 Run、Workspace 或 Result lifecycle**。

SM-2 只以 `ConvergenceState` 作为 canonical decision；action/output kind 是纯派生字段，不进入 aggregate、
authority transaction、terminal delivery 或 Run notification 的最终合同。所有外部 feedback 必须同时携带
stable factual category 与完整、安全 original diagnostic；Orchestrator 只读取前者选择 internal transition，
不得解析原 message/code。原 message/code/signature 仍必须通过 private internal event 原样封入 `FailureFact.OriginalDiagnostic`；
canonical terminal/recovery code 可以保持稳定抽象，但不能覆盖 LLM/capability owner 的精确诊断 identity。

## Provider inference selection / ceiling

- activation 持久化 Run owner封存的 `RuntimeActivationContext.ProviderSelection`
- provider ceiling prerequisite 请求与封存完整 `ProviderCeiling{Selection, token/byte ceilings, revisions}`
- candidate 强制使用 ceiling selection；Resume 不携带或重读 selection patch，Restart 由 fresh activation读取 Workspace owner最新 canonical selection
- reasoning effort参与 activation/ceiling identity，但不改变 stable prefix、CWI、focus、预算、retry或 action
- Orchestrator 只校验 identity/revision/ceiling 完整性，不解释 ai-proxy、模型品牌或 effort语义

provider-ceiling acquisition 的 typed timeout/temporary failure是当前唯一已实现的 bounded recoverable-wait组合。
SM-2/SM-4 在同一 aggregate 中封存 exact `AcquireProviderCeilingCommand` hash、condition、deadline、attempt guard 与
`recovery_probe` allocation；普通 drive/Resume 只返回只读 wait，只有 matching due `RuntimeDriveLease` generation 可以
先 CAS claim 再重发相同 probe。matching ceiling result与 prerequisite fact、probe consumption和
`condition_satisfied`在一次 CAS 中提交；transient unknown保留相同 physical identity等待 reconciliation，deadline或
non-transient contract failure分别封存 typed expired/exhausted terminal，不创建 Decision/Approval。

## Runtime execution budget

- fresh activation 把 `RuntimeExecutionBudgetPolicy` 与 convergence/recovery policy 一起封存；Resume只读取既有 policy，
  Restart由successor fresh policy重新开始，不复制predecessor消费。
- provider request、semantic generation/correction、whole-document recovery、candidate correction、reasoning effort、
  capability operation与recovery probe都先在授权 external call/effect 的同一 CAS 中创建 exact allocation，再通过
  provider usage、owner receipt或probe result确认；unknown只能以相同 logical/physical identity reconciliation。
- submitted provider session 的任何 outcome/document/terminal transition 都必须结算同一 logical/physical allocation；
  observation identity 漂移时零状态变化，owner 已确认调用但未返回可靠 usage 时只把 token 量保守记为 unknown，不能遗留
  reserved allocation。operation governance continuation 是新的物理 effect，resume dispatch 与新 fence allocation 必须同一 CAS。
- operation recovery 与 same-focus repair 已分别由 SM-4 `Facts.Recovery` 和 SM-2 `Facts.Repair` 独占计数。统一预算层禁止
  为这两个 dimension再创建 allocation/consumption；`RuntimeExecutionBudgetView`只把这两个权威子账本与其余
  execution ledger确定性汇总，避免第二份消费权威。
- `confirmed/reserved/unknown/remaining/exhausted`只读视图绑定aggregate revision、active wait和terminal code；
  projection、metrics、Web/TUI均不能top-up、清零、释放allocation或驱动恢复。
- unknown reconciliation 的 matching owner proof 只允许原子进入 confirmed/released/still_unknown/exhausted；failed
  reconciliation 必须绑定 exhausted allocation并继续保守占用unknown amount，不能把超时解释为未发生消费。

### Current development baseline 与 provider output contract

当前 Runtime persistence 只维护一个 current shape；每个 required owner fact 都必须封存完整 convergence declaration。
`RuntimeStateRecord`/`RuntimeBootstrap` 不携带用于选择历史 decoder 的顶层版本。旧 layout 不再探测或导入；当前路径上的
未知/退役字段或缺失 current invariant 在严格 load 边界失败；不得补 convergence、重建 authority、生成替代 fact 或进入
多版本 reducer。
activation/execution contract、config、runtime policy 与 stable-prefix hash 不携带
semantic-plan/candidate stage token limit。LLM owner 是 model capability 的唯一解释者；Orchestrator 只消费针对当前
workspace/run/authority、sealed selection 及 capability/configuration revision 的 detached persisted
`ProviderCeiling`，不得按 model name 分支或应用 token override。

semantic-plan 与 operations 的 `RuntimeExpectedDocumentContract.MaxOutputTokens` 只能从
`ProviderCeiling.EffectiveMaxOutputTokens` 创建，并在 provider call 前随 request session 持久化。ExpectedDocument、
LLM EventHub command、provider request、provider-facing `max_output_tokens` 与 audit 必须精确等值；LLM boundary 在任何
不等值时拒绝提交，physical call count 保持为零。SM-4 whole-document recovery 复用 sealed ExpectedDocument，不提高预算、
不重读配置、不拼接 partial output。

两个 request renderer 的 provider-facing instruction 都要求恰好一个完整 compact JSON object，无 prose、fence、
prefix/suffix 或 authority 重复；LLM wire normalization 只可移除一层完整且独占响应的精确 JSON fence，不能接受 prose、
多 value 或畸形 fence。sealed manifest 同时约束 prompt、closed JSON Schema、strict decoder 与 evaluator；object 禁止
unknown field，数组、字符串、target、binding 与 content/effect shape 均有界。LLM owner只返回 provider factual outcome，
不能选择 retry、terminal、CandidateAttempt 或 Runtime transition。

activation-only `DefinitionFocuses` 与 SM-2 generation-specific focus/binding queue 分开持久化。semantic document
admission、feasibility merge 与 seal 只读前者的 immutable declaration；receipt/resource-change 触发的 binding retirement、CWI
reopen 与 reseal 只修改后者，并继续经过 Orchestrator private internal event、controller 与 transition coordinator 的单次 CAS。
focus/binding identity 必须同时绑定该 generation 的 `manifestHash + logicalRequestID`；同 manifest 下的新 logical request
不得复用旧 focus/binding。`CurrentBindingRef` 不得指向 reopen/retirement lineage 中的历史 binding，reducer 在写入前与
aggregate validator 在持久化后均 fail-closed。capability owner 只报告资源事实和当前 admissibility，不能请求 CWI/focus 状态。

无法按 current shape 读取或校验的 aggregate 不进入 Resume/Restart/SM-4，且不调用 LLM/capability owner、不创建
invalid-data fact 或 replacement intent。数据清理后创建的 current Run 才能进入 Runtime。`fresh_activation_required`
只适用于可读 current Run 缺少 sealed bootstrap。current Restart 的 predecessor/replacement/successor 完全由 Run owner 的
`run.replacement` subordinate transaction 与 owner-private reducer 管理。administrative cancellation/supersede intent 属于
Run SM-1；Orchestrator 只在 SM-4 private event 中把 `RuntimeTerminationRecord` CAS 保存为 exact delivery handoff，驱动
`Result -> Workspace -> Run`，但不选择或执行 Run terminal transition。最终 terminal 只由 Run-private SM-1 delivery event迁移。

当前 baseline 为每个 authority 封存不含 request/lease/CAS bookkeeping 的 `AuthorityInputID`。相同 Definition、focus/binding、
evidence、capability/catalog、freshness 与 owner observation lineage 不能创建第二个 AuthoritySnapshot；精确 owner binding
contradiction 只通过 `semantic_binding_contradiction_observed` 私有事件退休，无法唯一绑定则 typed fail closed。

## Runtime 四状态机实现约束

完整规范以 `docs/00-core/runtime-stage-contract.md` 的“Runtime 四状态机固化规约”为唯一权威。本模块按以下四个状态机落实现有职责，不定义第二份状态语义：

| 状态机 | 本模块职责 | 外部 owner |
| --- | --- | --- |
| SM-1 Run lifecycle | 不拥有；只消费 Run state notification 触发自身 SM-4 drive | Run owner 持久化 SM-1 state/lineage，并为 Orchestrator aggregate 提供 opaque RuntimeStateRecord CAS；Workspace owner 持久化 activeRun |
| SM-2 authority/focus | reducer、focus/evidence/assessment/repair/governance/terminal 裁决；编译 sealed candidate manifest | application 只编译 typed bootstrap；owner 只返回 facts；Result 只存 outcome |
| SM-3 candidate/dispatch | request session、document-conformance/单 evaluator、共享 attempt 0/1、transaction、local operation compile、seal、evidence commit | LLM 返回 exactly-one `Stale | Feedback | DocumentOutcome` fact；Orchestrator 对 empty/invalid 保存 typed shape rejection、对 complete 执行 strict local admission，并确定性派生 operation identity/dependencies；capability block执行operation |
| SM-4 drive/recovery/delivery ledger | bounded drive、physical settlement、singleflight、restart load、delivery acknowledgement/retry；不写 owner state且不选择 candidate correction | Result/Workspace/Run owner 各自完成资源迁移；projection store/read model 只保存派生状态 |

实现必须保持：

- SM-1 不选择 action，SM-2 不执行 operation，SM-3 不选择 next action，SM-4 不重建 authority。
- Runtime EventHub metadata 的 `StateMachine` 只说明参与链；`ReducerOwner` 固定 SM-1=`run`、SM-2～SM-4=`orchestrator`，
  `MachineRole` 区分 participant、owner ingress 与 owner observation。只有本 owner 的 private internal fact 可标记
  `owner_internal_reducer`；Run owner 的 opaque aggregate CAS 只标记 `opaque_persistence`。
- Orchestrator 的外部 typed command/result、owner receipt、request outcome 与 delivery acknowledgement 必须先映射为
  owner-local `RuntimeInternalEvent`；Biz 只能经 `submitRuntimeInternalEvent` 提交，
  `RuntimeController.ApplyInternalEvent` 是唯一 mutation dispatcher，
  `RuntimeTransitionCoordinator.Commit` 是唯一 aggregate CAS 边界。controller 只导出该 mutation dispatcher 与
  `Load`、`QueryOperationReceipt` 两个 scoped read；所有 reducer handler 均为 package-private。internal event 不得成为跨 owner EventHub envelope。
- private internal fact 的 layer/SM/effect/source/scope/identity 分类只在 `biz/runtime` 以 exhaustive switch 固化；它不是
  EventHub topic、生产 registry 或跨组件 lookup。fact 词汇按 activation、SM-2 authority、SM-3 transaction、SM-4
  delivery 分别落在 `internal_event_{activation,authority,transaction,delivery}.go`；唯一 exported dispatcher 在完成
  envelope 校验后，通过四个 package-private payload-family route 委派，但自身仍显式穷举全部 kind。route 只调用共享
  mutation kernel 上的私有 component，不持有 repository/CAS、EventHub 或第二 mutation entry。完整性测试从全部 fact
  常量源核对 dispatcher/classification，并冻结每个 kind 只进入一个 family；不在
  `internal/pkg/events` 或 `workorchruntimecontract` 建立全局索引。
- `RuntimeInternalEvent` 只保留 `Kind + Scope + Activation|Authority|Transaction|Delivery` 四类 concrete payload；
  每个 kind 必须精确匹配一个非空 payload family。缺 scope、未知 metadata、missing/wrong/multiple payload 均在任何
  Runtime aggregate load/CAS 前拒绝，payload 不得复制 workspace/run identity。SM-3 claimed-operation recovery 只允许
  `dispatch_execution_requested` 携带成对的 `RecoveryOperationID + RecoveryBudgetAttempt`；SM-4 必须先通过独立
  `recovery_budget_consumed` event 持久化 exact counter，其他 transaction、半截授权或非正 attempt 一律在 envelope
  校验阶段 fail-closed。
- `biz/runtime` 内部的状态机职责使用五个私有组件表达，但仍是一个 Orchestrator owner、一个 aggregate 和一个
  CAS 边界：`runtimeMutationKernel` 唯一持有 Runtime repository port 与 `RuntimeTransitionCoordinator`；
  `authorityComponent` 只处理 SM-2 authority/focus 事实，`planningTransactionComponent` 只处理 SM-3
  semantic/request ledger，`candidateDispatchComponent` 只处理 candidate/dispatch/receipt，
  `driveDeliveryComponent` 只处理 SM-4 lease/recovery/delivery。组件只嵌入同一个 kernel，不持有 sibling、EventHub、
  lifecycle 或第二份 aggregate。当前 dispatcher 已路由 semantic/request ledger、authority facts/reconciliation/
  governance、candidate admission/receipt/execution 与 drive/delivery lease/budget/ack handler；prerequisite freshness
  的 candidate/planning/authority transition 也已按 owner 分文件。执行恢复先由
  `candidateDispatchComponent` 返回只读 recovery-budget handoff，再由独立 `recovery_budget_consumed` SM-4 event
  提交预算并以 operation identity/fenced attempt 授权重试，不在 SM-3 事件内修改 SM-4 ledger。Controller 的
  focused reducer 测试所需的 package-private controller adapter 只存在于 `_test.go`，不进入 production 编译；生产
  `RuntimeController` 只能由 `ApplyInternalEvent` 的 `internal_event_route_*` 直接分派 component mutator，外部仍只能使用
  `ApplyInternalEvent`、`Load`、`QueryOperationReceipt`。
- `RuntimeAggregate.Validate` 仍是一次完整、顺序固定且 fail-fast 的 aggregate validation；activation、SM-2、SM-3、
  SM-4 与 cross-machine fence 仅以 private validator 分段。evidence commit 的 receipt admission、fact derivation、
  exact-CWI credit 与 completion 共用一次 transition commit；prerequisite kind handler 为纯 aggregate 变换；active
  dispatch 的 preflight、claim/recovery、owner call 与 receipt projection 仍按原顺序运行，并共享唯一 CAS 与 recovery
  budget handoff。
- 所有跨状态机推进都使用 persisted aggregate + typed EventHub command/result。
- semantic acquisition 对 SM-2/SM-3/SM-4 的任何状态变化也必须先映射为 Orchestrator owner-private
  `RuntimeInternalEvent`：SM-3 只封存 proposal/request/candidate transaction，SM-2 只封存 feasibility/binding/credit/
  terminal，SM-4 只负责 drive/recovery/delivery。File、Shell、LLM 或 Application 的 concrete EventHub result 都是 participant fact，
  不得携带 transition target、调用 reducer 或更新任何一个机器的 state。
- SM-4 使用单一 persisted lifecycle-handoff 派生器穷举普通 Decision/Approval wait、Definition clarification wait、
  governance continuation 与 terminal。该派生器同时生成 Run opaque-record CAS 中的 `GovernanceFenceID` 和后续 Run-owner
  typed fact identity；pre-seal observation/dispatch 尚未绑定 owner request 时不授权 lifecycle transition。Run owner返回的
  concrete lifecycle result必须逐字段确认相同 state revision、fence、blocker/resolution 和最终 owner state。wait 的 opaque
  resolution slot 固定为空；continuation 在 opaque fence、owner-private reducer 与 acknowledgement 中都必须匹配 exact
  `ResolutionID`，active duplicate 也不能仅凭 `GovernanceFenceID` 成功。successor 已消费且 aggregate 已越过该 continuation
  fence 时，旧 resolution duplicate 只以 exact persisted request/resolution/evidence history 加 Run 对当前更晚 aggregate 的
  owner proof 作为历史 no-op，绝不把旧 fact 重投给新 blocker。opaque fence 的 governance/terminal mode 必须恰选其一；
  terminal seal 只发布 terminal identity，不沿用历史 governance resolution。
- governance wait fact 的 owner delivery failure 与 terminal delivery严格分流：前者保存 exact handoff signature 到现有
  SM-4 `RuntimeDriveLease`，不消费 `TerminalDeliveryAttempts`；响应丢失后的 duplicate只读取 Run owner 已提交状态，fresh
  process inventory发现 `need_governance + pending|active` 时重新投递同一 wait handoff；发现
  `blocked + persisted resolution` 时先重投 exact continuation，再允许 execution drive。continuation command失败也进入同一
  lifecycle lease ledger。后者才进入 Result → Workspace → Run terminal delivery ledger。两条路径都不得由 projection、
  watchdog或人工 Resume充当后继。
- `GovernanceResolutionObserved` 的 `continue|fail` private event 必须携带 exact lease template；resolution evidence、
  governed-dispatch completion、terminal/continuation authority 与替换旧 lease 的新 `scheduled` generation 在同一次 CAS
  封存。`continue` 先以该 lease 保留 run gate、提交 exact `ResolutionID` 的 Run-owner acknowledgement，再由同一
  generation claim 后继续；`fail` 不得同步裸投 terminal，而只能由该 lease claim 后进入唯一 Result → Workspace → Run
  lane。partial terminal acknowledgement 将 successor 原子续排为 canonical `terminal-delivery` identity；同一 resolution
  的 response-loss duplicate 必须验证并加入该 current successor，零写入且不创建第二个 timer。
  `continue|fail + nil lease` 在 private-event/reducer 入口一律拒绝，fresh-load 也不能补 successor。
- lifecycle resolution 在旧 drive 持有 singleflight gate 时只先 CAS 持久化 scheduled lease，并把 exact generation 交给当前
  gate owner 的退出边界；退出边界在同一 process-local 临界区选择“保留 gate 挂载 persisted timer”或“释放 gate 由提交者
  接管”。timer 永不绕过 gate 并发 drive；该 generation hint 不是 queue、watchdog 或第二份 retry authority，claim 仍以完整
  persisted lease 为唯一依据。duplicate/stale generation 不替换 current scheduled lease，Stop 仅清理本地 hint/timer，fresh
  startup 继续从 persisted scheduled/running lease 恢复，且整个路径保持 terminal-delivery budget 为零。
- projection failure 不得覆盖 reducer terminal；重启不得重复 prerequisite/candidate/side effect。
- 任何修改 reducer/controller/drive/operation projection 前，必须说明影响的 SM 编号并补合法/禁止迁移、幂等、restart、stale/duplicate、budget/guard 测试。
- 本模块不得恢复 禁止的 mainline、application continuation queue、通用 RuntimeAdapter registry、projection authority 或自动 successor。

## Candidate 合同

完整语义以 `docs/00-core/runtime-stage-contract.md` 的“规范化候选合同”为准。历史收口编号、旧 schema 记录和
deployed run ID 只属于 archive regression evidence，不定义当前候选、reconciliation 或 restart 语义。

- SM-2 先从 persisted Definition seed、template/profile policy 与 provider request 前通过 typed prerequisite
  获取的 owner capability/admissibility catalog 编译唯一 semantic-plan manifest；manifest/hash/schema/evaluator
  同时封存 owner snapshot/revision、operation kind、target/selector/constraint 与 recipe vocabulary，禁止 LLM 猜测
  未观察的合同值。Shell capability acquisition 对 managed-validation 发布通用
  `RuntimeCapabilityPrerequisiteFact`，其中声明它需要 File owner 的 manifest；SM-2 只把该 typed resource requirement
  编译进 prerequisite，不按 Shell、operation kind 或 recipe 名称分支。取得 manifest/observation 后，immutable binding
  才交给 Shell owner；Shell catalog 对每个声明 recipe 发布 `ready | prerequisite_required | unavailable`
  factual surface，并依赖 exact File workspace-tree revision；只有对当前 manifest 适用且静态可执行的 recipe 才进入
  confirmed set。没有 confirmed recipe 的 `validation/managed_session` 不进入 provider-visible allowed kinds，
  Orchestrator 不检查文件名、不判断 recipe 适用性。MCP 等动态 owner 同时发布 callable capability 与 typed
  `RuntimeValidationSurface`；只有带 owner-sealed input binding、non-durable effect、明确
  `safe_to_replay | at_most_once | owner_managed` class，并由 owner 显式声明为 `direct` 的 `ready` surface 可进入
  `ValidationRecipeRefs`。`at_most_once` surface 的 claimed/no-receipt 恢复必须进入 `outcome_unknown`，不能因验证身份
  盲重放。`auxiliary`/`not_declared`、
  `prerequisite_required`/`unavailable` 只保留在 aggregate/audit，不进入 provider C3/schema/correction，Orchestrator
  不解析 schema、合成输入或根据 tool 名称/描述推断 direct proof。SM-3 只请求/校验逐
  CWI 的无副作用 proposal；SM-2 通过唯一 typed
  `runtime-semantic/probe-proof-feasibility` command 按 canonical owner 投递完整 batch，File/Shell/MCP/Git/Web/SSH
  只返回绑定 manifest/seed/proposal/catalog revision 的 factual feasibility，
  只有 SM-2 private internal event 能封存 `CoverageEvidenceBinding` 与 reconciliation binding set。semantic-plan
  accepted 不创建 dispatch、不写 coverage、不选择 action。semantic contract sealed 后，SM-2 才从 persisted
  focus/required output 与 typed owner facts 编译唯一 `RuntimeCandidateContractManifest`。每个 operation kind 恰好对应一条纯值
  `SealedOperationAuthorization`，target 只能使用 `exact_target`、`existing_owner_scope`
  或 `prospective_owner_target`；每个可关闭 CWI 都必须带唯一、封存的 `CoverageEvidenceBinding` / `BindingRef`。
- owner-fact acquisition 使用 `prospective_owner_target` 时，manifest 同时维护 provider-selectable `OwnerCatalogs` 与
  non-selectable `OwnerFactDependencyCatalogs`。后者只保存 required owner 对 prospective satisfaction 判断所需的 exact
  catalog snapshot；它进入 manifest/hash/freshness/CAS/seal，但不进入 LLM C3、allowed operation/surface、schema enum、
  correction values 或 provider choice。required owner catalog 还必须发布唯一 provider-safe typed
  `ProspectiveTargetSatisfactionConstraint`，只允许 `all_of`；同一值封存到 acquisition/manifest 并只在 C3 投影，dependency
  recipe identity/command 不进入 provider。Definition 只封存 declaration selector/candidate refs 与 acquisition closure，
  obligation/manifest 只追加 owner/kind/variant/policy/operation-selector/current constraints 与 exact contract ref，两者都不封存
  prospective target set。SM-3 只能提交一份 bounded semantic proposal，其 `ProspectiveTargetRefs` 必须是 1～8 个唯一
  canonical exact target；集合中每项都必须在 declaration selector/candidate refs 与 current operation selector/owner
  constraints 的交集内。required-owner constraint 由 mandatory `all_of` clauses 组成：每个 clause 至少被一个 target
  完整满足，每个 target 也必须对至少一个 clause 有贡献；不存在 `one_of`/`any_of` 关系。current semantic
  wire 不接受 singular `ProspectiveTargetRef` 或 alias。Orchestrator 不把
  non-ready Shell surface伪装为 provider 可选 operation。
- SM-2 composite prospective probe 固定为 File outer feasibility + immutable
  `RuntimeProspectiveWorkspaceManifestDelta` + Shell nested
  `RuntimeOwnerFactProspectiveSatisfactionObservation`。File 对整个 target set 使用同一 complete manifest revision 执行 path/
  absence/parent/aggregate-capacity admission，delta 绑定 exact current workspace-tree revision 并只描述完整 hypothetical
  resulting refs。Shell concrete typed query 只消费 delta、activation-sealed recipes 与 exact Shell dependency
  catalog/observation，并逐 recipe 校验 command/ordered args/timeout 对应的 surface schema identity。File 不创建 target，
  目录或文件；Shell 仅基于 resulting manifest 判定 recipe 是否 `would_satisfy`，不读 File/workspace、不启动
  managed session；Orchestrator 不解释路径/recipe，三方都不得跨 owner 修改状态。
- observation CAS 与 semantic seal 都必须重验 manifest/batch/request/contract ref、delta、File catalog/observation 和 Shell
  dependency catalog/observation。只有 nested `would_satisfy` seal-ready；nested typed stale保持 Shell-authored factual
  identity并进入既有 bounded prerequisite-refresh generation，不能重写为 File outer stale或即时 terminal。missing dependency
  在 Shell query 前退休 semantic generation。semantic schema/document admission 先验证 proposal target 同时满足
  candidate const/enum 与 sealed `all_of` constraint；mismatch 只产生普通完整 document correction，C4 仅引用
  manifest/CWI/field path 且不复制 C3 constraint，零 File/Shell probe。`would_not_satisfy` 必须先作为 full composite
  observation 经 owner-private `semantic_feasibility_observed` 持久化；该 exact target set 仍只是未封存的 semantic proposal，
  不产生 binding。因为 admission 已证明 target 满足 published contract，exact-current full-composite
  `would_not_satisfy` 表示 owner contract contradiction；下一 drive 直接记录
  `prospective_satisfaction_contract_violated` 并形成 `semantic_contract_failed`，不得生成 replacement target、
  `Allowed` 或第二 provider request。Shell 仍只拥有 persisted satisfaction/reason，terminal 由 Orchestrator
  private event 署名和提交。typed stale 只走 bounded fact refresh；零 ticket/governance/acquisition/operation/effect/
  coverage，duplicate/restart 不重复 terminal 或同一 File/Shell probe。
- `validation_failure_repair` semantic manifest 的 provider-safe C3 projection 必须同时携带 failed focus、stable failure
  signature、typed trigger owner/operation/diagnostic class、Profile-sealed convergence ID，以及 exact repair
  owner/operation vocabulary/target policy/declaration candidates/operation selector/constraints/fresh-validation bound。
  它不得携带 raw diagnostic、receipt、Runtime action、retry 或 terminal disposition。LLM 只据此提出与 typed failure
  相关的一份 `state_change` proposal；File 对完整 prospective all-of target set 返回 factual feasibility，只有 SM-2
  private internal event 可以封存 binding。该 volatile repair generation 不得改变 C0～C2 或 stable-prefix identity。
- renderer、wire schema、strict decoder、candidate evaluator、owner feasibility command 与 fixture 只消费该
  manifest 及其 hash，不得从 profile、focus、raw reason 或临时 inventory 重建第二份允许集。
- `JSONSchemaResponse` 只决定 LLM adapter 是否可发送 provider format，是可选 wire acceleration，不是 SM-3 authority。
  format on/off、provider声称 schema accepted 或未声明 capability 的所有 complete output 都必须经过同一 strict local
  decoder/schema/evaluator；structured-output failure 不授权 adapter fallback 或第二次 physical call。
- operations provider schema/instruction/strict decoder 禁止 `id`、`dependsOn` 与 dependency graph。strict local compiler
  把 proposal 唯一映射到 manifest authorization/required-sequence slot 后，按 authority transaction、manifest 与 canonical
  slot/order 确定性派生 operation ID/dependencies；wire order、自造 identity/edge 或 ambiguous/duplicate slot 均零 owner call/dispatch。
- complete document 只调用一个纯 `CandidateEvaluator`，并按 manifest identity → wire/field →
  kind/variant/parameter → target → binding → effect/dependency → replay → durable owner-target overlap 的固定顺序返回 `accepted` 或完整
  `CandidateRejection`。owner feasibility 是 capability owner 的独立 typed factual observation，不得伪装为
  candidate rejection。
- 完整 candidate request bytes 的 `ArtifactContinuation` 只是 Orchestrator-owned audit carrier，不是 SM-3 authority。
  created/applied/failed 只能由一个 carrier-private reducer 写入；它绑定 workspace/run/logical request/authority
  transaction，但不能创建 attempt、选择 recovery/admission 或 seal dispatch。重启若发现内容已完整组装而 carrier 仍为
  chunking，必须幂等补交 applied fact，关闭“内容落盘后、metadata transition 前”崩溃窗口。
- 当前唯一结构 evaluator 是 `biz/internal/candidateadmission.Evaluate`；`biz/runtime/candidate_admission.go`
  只投影当前 authority/attempt/evidence lineage、调用该 evaluator 一次，并在返回后执行 SM-3 owner-local
  rejection seal。repair closure、validation-only gap 与 owner-grounded target scope 都在 replay 前由同一 evaluator
  判定；`biz/internal/grounding` 提供 target-selection/repair-target 的唯一纯比较实现，
  `biz/runtime/target_authority.go` 再以唯一 owner-private projection 把同一集合分类为 existing 或
  binding-sealed prospective，供 SM-2 grounding 与 SM-3 prerequisite/candidate admission 共同消费。只有 existing target
  可以请求 File snapshot；File-feasible prospective target 在创建前不得被重新解释为 existing target。Definition/Profile
  的 `TargetSelectorOwnerRef` 只是声明 lineage，capability owner 必须从 sealed proof operation descriptor 推导并与对应
  owner catalog 精确匹配，不能把两种 owner identity 当作同一个 matcher。focused package 不读取
  aggregate repository、不调用 EventHub、不探测 capability owner，也不返回 correction、retry、terminal 或 next action。
- `biz/internal/proof` 是 SM-2/SM-3 事实比较的唯一纯入口：它承载 reconciliation batch/owner observation
  重放、repair-surface confirmation 与 validation causation、current-CWI/binding lineage、owner receipt/binding
  proof 和 evidence attribution；Runtime 只投影 aggregate 私有事实、把 proof finding 映射为 retain/reopen/
  prerequisite/credit 或 CAS。`grounding` 承载 target-set 与 validation target freshness 比较。两者不持有
  aggregate、ledger、repository/EventHub、预算或状态转换，不会产生第二个 Owner。
- `biz/internal/governancecheck` 是治理身份与 factual mapping 的唯一纯入口：request/dispatch、operation
  binding/execution/receipt、ticket/resolution、grant/resume fence 及 dependency source 必须先在该包比较；它只返回
  detached finding，不创建 ticket、不关闭 dispatch、不写 evidence、不运行 reducer。operation-scoped governance
  receipt 的 closeout 属于 `candidateDispatchComponent`，request/resolution mutation 与 resume dispatch 仍属于
  `authorityComponent`；两者共享同一 mutation kernel 和 CAS coordinator，且仅由既有 Owner 私有事件触发。
- owner probe 前只允许用 kind 解析 manifest 中的 owner/authorization identity，该解析不做 admission；
  unknown kind 零 owner call，可解析 kind 只调用无副作用 probe。evaluator 仍优先返回顺序中更早的
  field/target 违例，owner observation 不得形成第二套 first-failure 路由。
- rejection 只保存 LLM document-conformance observation、strict local schema 或 evaluator 产生的完整、安全 typed diagnostic，
  不携带 correction、retry、terminal 或
  Runtime action。Orchestrator 将其提交为 SM-3 internal event 后，才根据 persisted manifest、attempt ledger、
  rejected shape 与 field violations 形成 `correctable_once` 或 `rejected_final` owner-local result；前者必须有
  可仅用 manifest + rejection 验证的 `CorrectionDirective`：empty/invalid 使用 closed `document_shape` witness，local schema
  使用 manifest-bound field/rule witness，semantic rejection 才封存 concrete legal semantic-difference witness。
  operation ID/dependencies 禁止出现在 provider wire并由本地派生，不能成为 correction field/progress；summary/reason-only
  delta 同样无效。attempt 1 在 provider 前重验 manifest/witness，返回后重新走完整 strict local admission，并在 semantic
  correction 时证明 proposal 使用 exact alternative。没有 legal frontier 的 attempt 0 形成 `candidate_authority_exhausted`，
  attempt 1 的任意 document/schema/semantic rejection 都形成 `candidate_retry_exhausted`；两者都不调用额外 owner 或治理。
  后者零 correction call。无法归类是
  `runtime_contract_defect`，不能退化为无信息 governance ticket。
- semantic-plan 的 `CorrectionDirective` 必须持久化 exact dependency edge、actual position、manifest/seed/prefix、
  plan step 与 bounded source observation hash；LLM command 只取得 Orchestrator 预先投影的 detached observation 并仅在
  C4 验证/渲染，不能接收或从 `OriginalMessage` 重建。coverage-order 缺 schema-required 空 `proposals`、覆盖不全、
  未知/重复 CWI 或其他 complete-document 结构错误，
  必须由 SM-3 绑定 manifest 的完整 CWI identity，形成 typed `coverage_order_document` violation；proof-batch 对应
  `proof_batch_document`。二者都只有 attempt 1，不得因无 typed delta 误报 `runtime_contract_defect`，也不得由 LLM 或
  application 选择 retry。correction lineage 只属于一个 sealed provider document；coverage-order attempt 1 即使 accepted，创建新的
  proof-batch request 时也必须在同一 SM-3 private CAS 中重置 attempt、directive 与 current rejection lineage。
  prospective target constraint mismatch 属于相同的普通 document/schema correction ledger：attempt 1 复用
  transaction/manifest/C0～C3/stable prefix，C4 只引用 typed field violation。feasibility 后的 full-composite
  `would_not_satisfy` 不进入该 ledger，而按 owner-contract contradiction 直接 terminal。
- candidate pipeline 只接收 `request_recoverable`、`request_terminal`、`document_conformance`、`candidate_rejection`、
  `owner_feasibility` 和 `accepted` factual result。coherent completed empty/non-JSON 属于
  `document_conformance`，不是 provider protocol failure，也不消费 incomplete/transport recovery；它与 complete local
  schema/semantic rejection 共用唯一 attempt 0/1。attempt 0/1 与 correction policy 只由 SM-3 reducer 消费 persisted diagnostic；owner feasibility
  必须先成为 Orchestrator internal event，再由 SM-2 coordinator 选择 prerequisite、新 authority 或 typed
  terminal，普通 operation owner feasibility 零 correction attempt。SM-4 只结算 physical call、调度并从 persisted result
  幂等恢复，不解释 document failure 或选择 correction，也不得为 format/schema/semantic 各建预算。它不再解释
  raw failure。prospective satisfaction 的 exact-current full-composite `would_not_satisfy` 在 operation candidate 前
  属于 persisted semantic-plan factual rejection；它不进入本段普通 operation `owner_feasibility`，只按上述
  `prospective_satisfaction_contract_violated` terminal 处理，也不能借此为普通 owner feasibility 增加 correction。
- owner feasibility 若证明当前 sealed validation recipe 不可用，SM-2 必须以
  `validation_recipe_exhausted` 直接形成 typed terminal；不得将其降级为普通 repair、candidate correction 或
  `runtime_contract_defect`。
- operation feasibility 的 owner wire、owner-neutral observation 与 persisted candidate ledger 只使用一个
  `RuntimeOwnerFeasibilityOutcome`；失败只携带一份 `RuntimeFeedback.OriginalDiagnostic`，不得并行保存
  `Feasible bool`、`FailureFact` 或重复的 code/reason/target 字段。stale、prerequisite request 与 replay proof 只能作为
  outcome-specific detail，重启必须原样恢复 diagnostic 的顺序、重复项和有界详情元数据。
- `RuntimeOwnerPolicyIdentity` 只证明 observation 来源，不是 SM-2 transition signal。只有 owner 显式给出
  `sealed_target_authorization_contradicted` constraint impact，并同时匹配 persisted attempt/operation、candidate
  manifest、catalog snapshot、authority transaction、current CWI/binding、reconciliation binding、target set 和 observed
  resource revision，Orchestrator 才提交 `semantic_binding_contradiction_observed`。binding 的 semantic manifest 可在当前
  ledger 或 immutable generation history 中；不得与另一个 CWI 的最新 manifest 强行相等。普通
  `observation_unknown`/replay-proof unavailable 仍提交 `owner_feasibility_observed`，只失效当前候选并进入有界
  no-progress 处理，不退休 binding、不重开 CWI。
- operation receipt 只以 `RuntimeOperationReceipt.Outcome` 表达 owner result，并只以
  `RuntimeFeedback.OriginalDiagnostic` 传播 rejected diagnostic；跨 EventHub receipt 不再携带 `FailureFact`。只有
  Orchestrator owner-private adapter/reducer 可从该 diagnostic 重建内部 `FailureFact`。raw validation stdout/stderr
  必须在 owner 边界清空并以 `omittedReason=raw_validation_output + redacted` 留痕，任何直接携带 raw output 的 feedback
  都必须校验失败。Result/audit、TUI/Web 和 typed client/port 只携带一份嵌套 canonical original diagnostic，禁止再把
  owner/code/message/revision/details 等字段平铺成第二组投影权威。
- live owner response 与 fresh-load persisted receipt 复用唯一 outcome-first judgment。`rejected` 必须先还原并保留 owner
  exact diagnostic，不得因其 `ObservedEffectScope=unknown` 被重写为 effect mismatch；只有 `accepted | no_op` 才执行
  sealed effect compatibility 与 evidence-quality gate，未知 outcome fail-closed。
- LLM owner 的 `RequestRuntimeCandidateResult` 只返回 provider-request domain 的 `RuntimeFeedback`，稳定类别仅允许
  authentication/configuration/transient/protocol；它不得跨 EventHub 返回 `RuntimeCandidateFailure` 或完整文档的
  schema/contract 裁决。Orchestrator adapter 必须先穷举校验 scope、authority、owner、category/class 与
  typed document disposition/content/feedback/stale 互斥关系，再在 SM-3 owner-private 边界重建内部 failure fact。
  coherent completed 的 `empty_document/provider_final_output_missing` 与
  `invalid_provider_output/provider_final_output_invalid` 是零 content 的 document-conformance observation，禁止归并为
  provider-protocol `Feedback`。其 disposition/code/signature 必须闭集匹配。唯一 raw-content crossing 是 coherent
  `complete_document` 的同步、volatile、typed `DocumentOutcome.Content`：仅在 LLM→Orchestrator EventHub Result 的 nested
  carrier 中存在并标记 `json:"-"`，供 SM-3 当次本地 decode；若为 crash recovery 持久化，只能进入 Orchestrator-private
  restart-content carrier。它不得进入 EventHub JSON、audit、aggregate、C4 prompt、eventstream、projection 或 memory。
  除此以外的 raw body/reasoning/decoder message 只允许留在其来源 owner 的 private audit
  carrier，跨 owner 仅接收 bounded hash/bytes/ref。
  Orchestrator 在一次 owner-private CAS 中结算 submitted call、关闭当前 request session 并由 SM-3 持久化 typed shape
  rejection/correction result；SM-4 仅执行 settlement/re-drive，不选择 attempt。
  `incomplete_document` 只提交 primary-classified-as-SM-4 的 owner-private `provider_request_recovery_coordinated`
  fact；transport（包括 body-idle）与 incomplete recovery 都必须在同一 aggregate CAS 中结算本次 physical call 并推进
  persisted request-session physical sequence，使下一 physical call 使用新 `PhysicalCallID`，并从头请求同一 sealed
  expected document，禁止跨 owner 携带 partial bytes、拼接 raw suffix 或消耗 candidate correction budget。
  provider request 的 request ID/status 只保存在 sealed `ProviderRequestAuditView`，EventHub Result 不再重复携带未消费的
  `RequestID/Status`。
- LLM request preflight 同样只返回 `provider_preflight` domain 的 `RuntimeFeedback`，稳定类别仅为
  `accepted | context_integrity`；旧 `Ready + Reason` 平行权威已删除。preflight feedback 必须证明未提交 provider，
  并绑定同一 workspace/run/operations authority；完整安全原始原因只位于 owner diagnostic。
- Run owner 的 opaque aggregate CAS Result 只通过 `RuntimeFeedback.StableCategory` 表达 accepted、aggregate revision
  mismatch、semantic rejection 或 owner transient；不得再增加与它平行的 disposition/status。Orchestrator SM-4 仅对
  exact `aggregate_revision_mismatch` 构造 typed CAS conflict。
- candidate retry contract 必须绑定完整 `AuthoritySnapshotID`。同 focus 收到新的 owner feasibility 事实后，即使
  action、required output 与 prerequisite hash 相同，也只能生成新合同的 attempt-0；旧 invalidated attempt 保留为
  audit lineage，不能被覆盖或变成 attempt-1。
- `OwnerInventoryBudget`、`CandidateContextBudget`、`RequestRecoveryBudget`、
  `CandidateAttemptBudget` 与 `PrerequisiteRefreshBudget` 是不可互借的合同。inventory 必须完整或
  使用 owner 可独立验证的封闭 selector，不得将 context 裁剪当成 authority 裁剪。
- required CWI 的 binding 必须在 operation candidate 前由 SM-2 从 Definition seed、受约束 semantic-plan document
  与 matching owner feasibility 编译并封存；validation proof 当前采用一 focus 一 CWI 的保守形状，多个 CWI
  共享 recipe 也必须各自具有独立 check/target 并编译为独立 binding。缺失 deterministic binding、binding set/hash
  漂移或 owner 不可验证时不得请求 operation candidate；只能有界修正 semantic plan、取得 owner prerequisite 或
  typed terminal，零 operation、ticket 或替代执行路径。activation 不接受预制 sealed binding，所有 fresh restart
  必须经过同一 semantic planning/owner feasibility/SM-2 seal 主线；普通用户不得被要求填写 Runtime JSON。
  `workorchruntimecontract` 只定义 binding DTO 和纯校验/匹配 helper；SM-2 才能依据
  sealed binding + owner receipt 迁移 coverage。
- `internal/pkg/workorchruntimecontract` 只实现 DTO、enum、normalize/validate/clone/hash、纯 manifest
  compiler 与 evaluator helper；不持有 Hub、不执行 I/O/CAS、不选择 retry/terminal/Runtime action。
  `RuntimeSemanticPlanBatchState` 的初始 `pending` 只由 `biz/internal/semanticplan` 的 owner-private compiler 产生，并经
  SM-3 internal event 进入 `biz/runtime` 私有 reducer；后续合法 forward transition 也只由该 reducer 选择。共享包只校验
  persisted state shape 和 stable hash，不导出 initialization/compiler/transition helper。

## CWI semantic planning / reconciliation owner contract

- Workspace pre-seal admission must reject every Profile-required acceptance/deliverable source that lacks an exact typed proof
  declaration; wildcard evidence policy cannot substitute for its `scope + id`, and activation cannot silently emit an
  assessment-only CWI. Activation/compiler and bootstrap validation repeat the same pure Profile contract check only as a fail-closed
  input defense before any SM-2 mutation or SM-3 request. They do not open tickets, repair Profile input or create a second state owner.
- Activation 必须封存 Definition seed；fresh restart 缺 workspace-specific exact proof 时进入 persisted
  `semantic_contract_required`，不得由 application 终止或要求普通用户补 `contextHints/evidencePolicies` JSON。
- activation declaration 与 generation authority 使用不同类型：`RuntimeDefinitionFocus` 只能携带 CWI、slice、evidence/
  assessment、declarative selector/owner-fact/metric/artifact obligation，以及可选的单一 Profile
  `ValidationFailureRepairConvergence`；`RuntimeFocusSeedsFromDefinitionFocuses` 生成的初始 `CoverageFocus` cursor 不携带
  selector closure、repair convergence 或任何 resolved target/operation/recipe/check/binding。只有 SM-2 semantic
  seal 可以写 generation-specific `CoverageFocus`；restart 不得从旧 focus queue 重建 Definition declaration。
- semantic-plan document 使用 SM-3 现有 request session/evaluator/attempt 预算，但 accepted 只产生 internal fact。
  canonical owner feasibility 完整匹配后，SM-2 原子封存 `RuntimeCWIReconciliationContract`；每个 CWI/binding
  必须有 deterministic target 或 owner selector、operation kind/variant、validation recipe/check 以及适用的
  metric/artifact。缺 binding 或 hash drift 禁止进入 operation provider/dispatch。
- LLM EventHub command/result 与 LLM owner 内部 request/response 使用强制 `DocumentKind=semantic_plan|operations`。
  两种 document 的 manifest/context/rejection 字段严格互斥，result 回显 exact kind；semantic plan 与 operations 共用
  provider transport、sealed expected-document、whole-document recovery、C0～C5 cache plan 和 audit，不存在 raw suffix
  authority 拼接、空 kind fallback 或第二套 provider client。
- semantic ledger 只允许通过八个 Orchestrator private event/reducer entry 迁移：required、request started、plan
  evaluated、feasibility required/observed、correction required、sealed、failed。它持久化 attempt 0/1、stable prefix、
  logical request、correction、document、owner batch/set 与 seal lineage；duplicate 不写 CAS，scope/seed/request/batch
  stale 必须拒绝。任何这些 entry 都不得向外暴露为 EventHub command。
- Orchestrator SM-4 仅持久化并调度 `current_run` / `workspace_full` request，SM-2 internal event 才能提交完整 owner
  observation batch、retain/reopen/prerequisite 结果和 reopened lineage。reconciliation 不新增状态机，也不向其他
  owner 发送 target-state command。
- accepted semantic order 在 effect-fenced 旧 current focus 尚未完成时，可以继续保留位于 remaining queue、已被同一
  semantic manifest 精确绑定的 CWI；该例外必须同时匹配 current binding ref、active/remaining focus binding，且所有声明
  prerequisite 仍为 settled+satisfied。它只维持当前代 executable lineage，不把 bound CWI 重新变成 planning eligible；
  duplicate/restart projection 必须过滤该 row 并保持零 CAS。stale manifest、binding 漂移、completed row 或 prerequisite
  重开仍按 stale authority 拒绝。
- request generation/history、batch hash、canonical observation identity 和 workspace/run scope 是幂等 identity；
  duplicate/stale/CAS restart 不重放 operation、不覆盖旧 evidence。batch 只保存每个 binding-owner 的唯一 canonical
  observation；lineage ref 按需由 observation 纯派生，不并行持久化 synthetic owner receipt ref。operation execution
  owner receipt 仍是 capability owner 的真实 durable receipt identity，不受此归一影响。
- File/Shell/Git/MCP owner 只能返回 factual observation：File 不从 Goal/DoD 猜 target，Shell 没有 CWI-specific checks
  时返回 `unverifiable`，Git repository revision 不等于 semantic proof，MCP surface readiness 没有 behavior probe 时
  返回 `unverifiable`。coverage credit 仍只由 SM-2 从 matching sealed receipt 派生。
- 空 target scope 只是一条“当前无 existing target”的 owner observation，不能授权候选任意创建路径。prospective
  target set 必须由一份 SM-3 document 作为 exact proposal 提出，同时匹配完整 declaration selector
  ref/owner/subject/candidate refs 和 File owner catalog 的 operation selector/constraint/variant；非空 candidate refs 对集合中
  每项要求 exact member match。File/Shell fact 只验证该 proposal 与 resulting manifest，SM-2 sealed binding 才使整个集合
  成为 authority。operation candidate 只引用该 binding，并必须完整满足每个 target 的 exact-singleton required
  operation；dispatch 只从 binding 派生 target。
  ordinary Shell validation acquisition 所需 target vocabulary 仅由 Orchestrator 转交当前 persisted File owner inventory；
  prospective satisfaction pure query 只额外接收 File owner 从该 exact inventory 计算的 immutable hypothetical delta。
  File effect退休旧 observation 后仍重获 fresh inventory；Shell 只拥有 recipe/surface readiness 与 prospective satisfaction
  judgment，不获得 File target state authority。



## Effect / Receipt / Repair Directive

- `RuntimeEffectScope` 是 operation 的中立 effect 合同；descriptor 只提供保守的 technical default，concrete
  effect/replay 必须由 capability owner 发布的 `RuntimeOperationPolicyFact` 按 typed scalar selector 精确匹配。Runtime core
  不按 Shell/MCP、tool 名、risk label 或业务语义分支；当前 owner catalog 存在时会完整替换同 owner/kind 的 activation fallback。
- target create/existing-scope 权限同样只来自 owner catalog 的 `RuntimeOperationAdmissibilityVariant`；descriptor 不携带
  create 或跨 owner target policy。seal 前 `ValidateCandidateEffectCompatibility` 强制：canonical `need_state_change`
  至少一个 durable effect，`need_validation` 不得含 durable/unknown；owner 声明为 non-durable 的 operation 不能关闭
  durable state-change。
- SM-3 seal 保存 canonical candidate、validation recipe body、完整 owner catalog/observation preimage、仅被选中的 activation
  fallback policy 与 per-operation execution authority，并统一纳入 `DispatchAuthorityHash`。owner inventory、policy 或
  prerequisite observation 在 seal 后演进，只影响后续 authority，不得改变 active/history dispatch；execution、restart 和
  governance resume 都只能使用 dispatch 内的 sealed snapshot。
- 每个 executable dispatch 必须恰好由一个 durable admission 产生：普通 dispatch 精确绑定 accepted candidate attempt 的
  transaction/candidate/dispatch authority，resume dispatch 精确绑定 granted operation-governance binding 的
  `ResumeDispatchID`，两种来源不得同时出现或同时缺失。自洽的 dispatch record 本身不是执行授权。
- `SealedDispatch.OperationExecutions` 在 seal 时写入 per-operation `OperationHash + ExecutionKey + ExpectedEffectScope +
  IdempotencyClass + owner snapshot binding`；execute 前 claim fence，owner 返回后 `owner_committed`，evidence CAS 后
  `evidence_committed`；owner error 后先 query exact receipt，缺 receipt 且没有 sealed replay authority 时才记
  `outcome_unknown`，禁止盲重放。
- owner receipt ledger 与 dispatch execution projection 必须按 `ExecutionKey + fence + OperationID + receipt ref/hash`
  双向完全匹配。committed execution 不得缺 receipt，receipt 不得成为无 committed execution 的孤儿；governance source 与
  completed resume clone 只能共享完全相同的 receipt projection。
- owner 已持久化 exact `ExecutionKey + Fence` receipt 但响应丢失时，executor 必须先重新加载 aggregate：存在 terminal receipt 则继续 governance/evidence commit，不得把 `owner_committed` 覆盖为 `outcome_unknown`。
- claimed/no-receipt recovery 只允许 sealed idempotency=`safe_to_replay|owner_managed`，并先消耗独立 operation-recovery budget；`at_most_once|unknown` 一律转 `outcome_unknown`。每项 operation evidence 独立 CAS 持久化；只有最后一项成功或任一项 rejected/stale 时，才在该 evidence CAS 中原子 settle dispatch。
- current-schema evidence commit 必须携带已持久化 owner receipt，并按 sealed expectation + `ObservedEffectScope` +
  `BindingRef` 的 exact target/selector、recipe/check、metric/artifact 匹配联合裁决 Kind；check identity 只来自 current
  owner surface，recipe ID 不替代 check。同一 check 同时出现在 passed 与 failed/skipped 时不是 pass；declared Kind、
  candidate 自报 `CoverageWorkItemIDs`、非空 `PassedChecks` 或 exit code 0 都不构成 coverage authority。
- multi-operation dispatch 中，同一 dispatch receipt 造成的 prerequisite/resource revision 变化属于内生变化；可以
  退休后续 SM-2 observation，但不能仅因此清除 `ActiveDispatch`。SM-3 必须继续尚未完成且 dependency 已满足的
  sealed operation，直到全部成功或任一 operation rejected/stale 后再 settle/reduce。该规则只适用于不同 canonical
  owner-target resource：
  candidate evaluator 在 seal 前拒绝重复 durable `(OwnerModuleID, TargetRef)`，普通 manifest-derived dependency edge 不能授权第二次同目标写入；
  provider 必须合并为一个 operation/receipt。唯一例外是 owner fact 封存的 exact `RequiredOperationSequence`，其每项必须
  绑定不同 requirement ref/variant，并按本地派生依赖执行。prospective all-of binding 的 sequence 还必须与完整
  `ProspectiveTargetRefs` 一一对应：每项封存一个 exact `TargetRef`，admitted proposal 的 `targetRefs` 必须为该值的
  singleton，所有 requirement 齐全且可唯一映射到 canonical dependency chain。缺少、额外、重复或 ambiguous 项均在
  owner effect 前拒绝，不允许“部分准备后先执行”。
- prospective all-of 的 receipt completion 也是集合合同，而不是“首条成功即完成”。同一 sealed dispatch 的每个 target
  operation 必须取得 owner-committed `Outcome=accepted` exact receipt；该 receipt 只有在 Orchestrator private evidence event
  把 matching execution、receipt-derived evidence 与 processed-event identity 原子提交为 `evidence_committed` 后才成为
  collective member。前 N-1 个 member 只保留 operation/evidence progress，`ActiveDispatch` 与 CWI unsatisfied/unsettled
  状态不被首条 receipt 清除。rejected/stale/weak/mismatched/conflicting duplicate 不进入集合；exact duplicate 零 revision/
  effect/credit，restart 只恢复同一 dispatch 的缺失 member，不跨 dispatch、repair 或 semantic generation 拼接 receipt。
  全部 target 对每个 required evidence kind 齐全后，只有 SM-2 private event 可以 exactly-once 认可 collective proof，并在
  其它 quality/effect/assessment gate 也满足时通过 aggregate CAS 授予 CWI credit/settlement；capability owner 只署名单项 receipt。
- state-changing Shell receipt 必须使用 owner 实际观察到的 changed refs/resource changes；任意 `shell/command` 只允许
  workspace-durable、at-most-once shape，真实变化越出 sealed `TargetRefs` 时以 `undeclared_workspace_effect` 拒绝，
  不能通过额外 touch 一个声明目标取得 durable evidence。Shell managed validation 使用 owner 临时 workspace 副本，
  只发布 `ephemeral_session + at_most_once` receipt；其 recipe aggregate proof check 只有在真实、无冲突 pass 后出现。
- `Facts.RepairDirective` 是同 focus 唯一 next-action 权威（INV-05）；`ReduceRuntimeStateWithDirective` 同步 directive；anti-replay evidence 不得覆盖 directive action。不存在从 failure message 推导的 repair hint 第二权威。
- repair directive 的创建、切换和清除均属于 Orchestrator SM-2 reducer；`internal/pkg/workorchruntimecontract` 仅保存 directive DTO、校验和纯计算。`need_prerequisite` / governance wait 是同一 repair cycle 的临时 interlude，必须保留 directive；只有 accepted repair evidence、focus advance 或 terminal 才能切换或清除它。
- LLM assessment 的 disposition、`MissingEvidenceKinds`、target refs、summary、items 和 reason 都是完整保留的
  observation/original diagnostic，不具备选择 Runtime action 的权力。SM-2 只可从 persisted coverage、receipt/evidence
  quality、focus policy、owner-grounded repair surface 与 freshness 分类，并在同一 owner-local transition 中写入唯一
  `Facts.RepairDirective.RequiredState`。不得从 observation 的自由文本或其 claimed missing kind 直接进入
  `need_validation`、`need_state_change`、governance、retry 或 terminal。
- validation entry 不是 state-change repair target；Shell 只可从 owner 支持的结构化工具诊断提取 fresh File-manifest
  scope 内的 actual candidate target，无位置或越界时不发布 observation，不能回显 operation/input target。只有 focus exact target、仍新鲜的 owner-grounded failure target 或
  sealed owner selector/prospective constraint可以形成 repair authority。validation/evidence-quality gap保持
  `need_validation`，不得在 validation-only 重试后退化为无关 `need_state_change`；耗尽封存 validation
  recipe/budget 时必须形成 typed terminal。普通 rejected validation receipt 不获得 CWI credit；其防重放归属只能来自
  completed dispatch 中封存的 exact `FocusID`。stable operation shape 忽略 operation ID 与 summary 元数据，语义相同的
  重试必须在 owner 调用前拒绝；只有同 focus reducer 实际再次授权 `need_validation` 时才递增
  `ValidationOnlyRetries`，初次 validation、duplicate receipt 与 restart 均不得重复计数。
- 无 actual target 的 rejected validation 也不得从命令、输出文本或 terminal selector 猜测 repair target。只有
  RuntimeDefinitionFocus 已封存的单一 `ValidationFailureRepairConvergence` 精确匹配 trigger 时，SM-2 才可把该 failure
  转为同一 CWI 的 `validation_failure_repair` semantic purpose。manifest 将 terminal `validation` 与 current File
  `state_change` 分离，并绑定 failure/convergence/File catalog；File feasibility 确认 proposed target 后，普通
  `CoverageEvidenceBinding` 才成为 exact repair authority。accepted repair receipt 不授予 CWI credit，而是在同一
  owner-private CAS 中退休 repair binding、reopen 同一 CWI并要求 fresh validation。stale/duplicate/restart 与 cycle
  budget 均从 aggregate 重验。缺失、歧义或非法 Profile declaration 在 pre-seal 打开 blocking Definition clarification
  ticket 且不创建 Run；已封存合同的 owner catalog 漂移或无合法 repair surface 则 Runtime typed fail closed，不创建通用
  Decision/Approval ticket。该 convergence 只存在于 immutable `RuntimeDefinitionFocus`；mutable `CoverageFocus` 不复制、
  不修补该声明，generation 只能消费由 definition snapshot 编译并封存的 obligation。
- semantic generation 的 eligible closure 由 purpose 唯一派生，accepted-order projection、focus advance 与 generation
  retirement 必须调用同一纯规则。普通 definition generation 使用当时全部尚未绑定且可规划的 current CWI；
  `validation_failure_repair` 只包含触发 repair 的 exact current CWI。无关的全局 eligible CWI 不得退休 repair-local
  manifest、增加 CAS 或创建第二 provider request；普通 generation 的真实新增 eligible member，或 exact repair
  CWI/purpose/lineage 漂移，才退休 current generation。
- `eligible_focus_projected` 等 semantic owner-private event 完成 CAS 后，drive 必须从 returned aggregate 重新读取 phase、
  generation、manifest、accepted order 与 logical request identity。event 已退休 generation/manifest 时立即把 aggregate
  交回 outer drive 重新分派；不得解引用 CAS 前的 manifest/hash，也不得以 panic 充当隐式 continuation。
- assessment gate 只消费 current-run accepted evidence、sealed binding/effect与 owner structured validation
  observation；只有 receipt 实际报告 binding 要求的 exact check/metric/artifact 才能给对应 CWI credit。非空
  `PassedChecks`、claim signal、assessment 文本、Goal/DoD、summary 与 operation/check name 的自由文本相似度
  都不能作为覆盖裁决；profile 封存 recipe 只约束候选选择该命令时的必要参数。
- profile-declared `RuntimeOwnerFactRequirement` 是 immutable focus postcondition，不是 capability target state。
  Workorchd 只编译声明，Shell/MCP 等 owner 只通过 current typed admissibility catalog 发布事实；SM-2 assessment gate
  在 sealed binding 的其余 proof 全部成立后，才检查指定 owner/operation 是否存在匹配的 `ready + direct` selectable
  validation surface。assessment claim、成功 operation、旧 catalog 或历史 recipe 均不能替代该 current owner fact。
- semantic proof admission 发现 current owner fact 缺失时，只允许 pre-seal Profile contract 中当前 all-of dependency
  已就绪的 exact acquisition owner/kind/variant/target-policy/operation-selector。Definition target selector 仅保留声明
  lineage，不是 capability authorization。Orchestrator 在 manifest seal 时为每个 CWI 编译唯一
  `RuntimeSemanticProofObligation`；provider schema、typed correction 与 binding compiler 只消费该 immutable obligation，
  不得随后重读 aggregate `DefinitionFocuses` 或将两套 selector 归一为同一值。proof batch 必须恰含这一项，不能让
  LLM、Shell 或 File 另选动作。obligation 的 terminal evidence kinds 必须始终保留 Definition 声明；current acquisition
  evidence kinds 单独派生为 `state_change`，不得获得 terminal validation/metric/artifact credit。
  `prospective_owner_target` acquisition 的 pre-seal declaration、compiled obligation 与 provider projection还必须具有同一
  非空 `ProspectiveSatisfactionContractRef`；preexisting/existing-scope 禁止携带。required-owner dependency catalog不属于
  provider-selectable catalog，LLM 不能根据它选择 recipe readiness或 satisfaction outcome。pre-seal
  declaration/obligation 不携带 exact prospective target set；SM-3 proposal 提出 1～8 个 `ProspectiveTargetRefs`，File/Shell fact
  验证完整 all-of set，只有 SM-2 binding seal 首次形成 authority。SM-2 只消费上述 composite
  File-delta/Shell-observation事实，并通过自己的 private event决定 refresh、typed terminal 或 seal；File/Shell result本身不带
  transition target。
  若历史/旧 generation 先执行了非 acquisition proof，assessment commit 可在同一 Orchestrator CAS 中提交 audit、把
  current bound CWI 转为 `reopened`、清除 current binding、更新 reconciliation，并以
  `assessment_owner_fact_postcondition_unmet` lineage 退休该 generation；SM-4 只从 aggregate 幂等 re-drive。真正的
  acquisition receipt/assessment 计入封存的 `MaxAcquisitionAttempts=1`；reobserve 后 fact 仍缺失即 deterministic
  `owner_fact_convergence_exhausted`，不得以 semantic replan 形成第二次 acquisition。duplicate/restart 不产生第二次
  reopen/CAS/request/effect。reobserve 已满足时，只有 `assessment_observed` private event 可在同一 CAS 中保留 exact
  acquisition attempt/audit、保持 CWI open、写入 `assessment_owner_fact_acquisition_settled` reopen lineage并退休当前
  generation；successor manifest 恢复原 terminal evidence obligation，acquisition evidence 不得关闭 CWI。若 sealed proof
  本身仍缺失，则保持普通 evidence/repair 主线。`CurrentBindingRef.SemanticManifestHash + CWI ID` 必须从 active manifest
  或 immutable generation history 唯一恢复 proof purpose；history 仅用于 purpose lineage，不得恢复 candidate/action
  authority。SM-2 aggregate invariant 禁止 acquisition purpose 保留 `satisfied/settled` terminal credit，并要求该 focus
  保持 assessment gate。CWI 重开后
  固定回到 `semantic-pending/<cwi>`；若该 current-focus prerequisite ID 已由上一 semantic generation 退休，SM-2 必须在
  重新排入 acquisition 前原子移出同 ID 的 retired prerequisite 及其旧 observation，只保留无关 retired lineage。旧
  observation 不得复用为当前 authority，审计继续由 CWI reopen lineage 与 semantic generation history 承载，且
  active/retired 不相交 invariant 不得放宽。导致该 owner-fact gap 的 accepted state-change operation shape 是同一 CWI
  的防重放事实：SM-2 只把 shape 携带到 successor focus，repair attempt/gap/directive 仍按 focus 清零；SM-3 必须拒绝
  successor candidate 重放该 shape。新 generation 必须使用新 logical request 派生 focus/binding，历史 reopened binding
  绝不能重新成为 current。
- semantic Definition-gap document 在提交 `definition_gap_observed` 前必须通过 pure manifest admission。已有 sealed
  `OwnerFactAcquisition` 时，provider 的 `undeclared_prerequisite` 只进入 bounded typed correction，不能创建
  clarification observation、`need_governance` 或 ticket；只有 admitted gap 才由 owner-private internal event 选择专用
  clarification transition。
- File owner 对 `unified_diff` 在 owner feasibility probe 中执行无写入 dry-run；hunk/context 冲突必须在 dispatch seal 前以 typed infeasible result 返回，owner 不选择后续 RuntimeAction。
- active Runtime Cancel/Supersede 必须来自 Run SM-1 typed administrative intent；Orchestrator SM-4 先 CAS 持久化
  `RuntimeTerminationRecord` handoff 并执行 ordered delivery，Run owner 再由 private SM-1 event 持久化 terminal。
  handoff record 不是 SM-1 state，Orchestrator 不得“投影”或代写 Run lifecycle。
- semantic owner catalog 退休后，若 current eligible CWI 尚无 binding，SM-4 drive/startup recovery 必须先推进新的 semantic
  generation；该窗口中因 binding 缺失产生的 `target_grounding_unresolved` decision 不得进入 Result/Workspace/Run
  delivery。`candidateRequestSessionActionAllowed` 只为这一精确窗口放行 `semantic_plan`，operations request 继续拒绝。
  current semantic operation authority ready 或 semantic transaction failed 后才满足该 convergence terminal 的 delivery
  readiness；其它 sealed terminal 不得被语义重规划覆盖，administrative termination 走独立 Run-owner lane。
- aggregate invariant 要求非终态 Decision 且无 administrative termination 时
  `TerminalDeliveryAttempts=0` 且无 terminal acknowledgement。不满足时由当前 invariant 校验在 owner load 边界
  直接失败；不得形成兼容分类、升级或 data-reset 状态，不得由 projection 修复、归零或隐藏。清理数据后
  重新创建是当前阶段唯一恢复方式。
- drive failure 经 `classifyRuntimeDriveFailure` 分类：只有携带 expected/observed revision 的 `RuntimeCASConflictError` 才进入 CAS reload；owner-transient/projection 不终态化业务 Run，语义 conflict 不得消耗 CAS budget。

## 组件交互及合同

- 通过 EventHub 读取 `blocks/workspace` 和 `blocks/run` 的权威快照，并只通过 Run owner CAS 合同持久化
  Runtime aggregate；不通过任何 Run owner command 指定 lifecycle target state。
- Runtime 通过 File、Git、Shell、MCP、Web 和 SSH owner 各自 `pkg/events` 定义的 concrete execute command/result 调用能力 block；同步返回的 typed result 是提交 execution evidence 的唯一执行回执。
- 通过 EventHub 调用 Decision、Approval 和 Memory owner 自身 `pkg/events` 定义的具体 command；Runtime
  governance 的 ticket/approval open request 分别由 Decision/Approval owner 定义。terminal 后只发布 immutable delivery fact，
  Result/Workspace/Run owner 各自消费并产生 acknowledgement。不得直接持有这些 owner 的 service 或 repository。
- Runtime terminal Result 只通过 Result owner 的 typed SM-4 delivery command 写入；Orchestrator 不构造或提交完整
  Result/Artifact entity，也不保留 Result/Artifact generic repository facade。Result/Artifact 的查询、清理和生命周期
  继续完全属于 Result owner；Workspace deletion 只向 Orchestrator 提交 workspace-scoped typed purge，Orchestrator 仅清理
  自身证据资源，不发起跨 Run/Memory/Result 的逐 Run purge saga。
- Orchestrator 与 `kernel/governance` 没有直接 EventHub 合同；治理策略经 Decision/Approval/Run owner 的 typed fact 与
  resolution 间接汇合，Orchestrator 保持 runtime contract gate 权威。
- Decision 与 Approval 使用各自 `pkg/events` 定义 evidence command；Orchestrator 幂等写入自己的 EvidenceRecord 并返回具体 result。
- Orchestrator 只在 `kernel/orchestrator/pkg/events` 定义由自身接收的 Runtime drive、receipt commit/query、governance resolution 与只读查询合同；投递给其他 owner 的 command/result 由接收 owner 定义。
- File target snapshot、target selection、workspace manifest 与 repair-surface confirmation command/result 由 File owner 在 `blocks/file/pkg/events` 定义；前三类 acquisition Result 必须回显 workspace/run/authority scope，prerequisite identity 只由 payload/必需 freshness observation 表达。Git capability-state 与 LLM provider-ceiling acquisition 使用相同 scoped Result 规则。Decision、Approval、File、Git、LLM、MCP、Shell、SSH、Web 各自在自身 `pkg/events` 定义并消费 Runtime capability acquisition；其 Result 只返回一份带 workspace/run/authority scope 的 canonical acquisition，prerequisite identity 只由必需 freshness observation 承载。File、Git、MCP、Shell、SSH、Web 的 operation admissibility acquisition 采用相同 scoped canonical fact，prerequisite identity 只存在于 inventory/必需 observation，不再由 Result 外层重复回显；六个 owner 同时拥有 operation feasibility 与 semantic-proof feasibility topic/Command/Result，File、Git、MCP、Shell 各自拥有 CWI reconciliation query。Shell 还独占 required-owner prospective-satisfaction concrete typed query/result，File 只在自己的 semantic feasibility result 中发布 hypothetical manifest delta；Orchestrator 仅按 canonical owner 分派具体合同并归一 composite observation。
- `kernel/orchestrator/pkg/events` 只保存 typed command/result、receipt DTO 与确定性校验/构造；能力 Block 的 Biz 负责发送 receipt query/commit command，Orchestrator Biz 负责消费并持久化，任何 `pkg` 不得持有 Hub、执行 I/O 或推进状态。
- Provider ceiling、candidate preflight 与 candidate request command/result 由 LLM owner 在 `blocks/llm/pkg/events` 定义；Orchestrator 的 SM-3 request session 封存完整 authority transaction，跨 Orchestrator → LLM → provider 的一次物理请求只携带 canonical `RuntimeCandidateRequestIdentity`，EventHub command/result 与 LLM request/response model 不得再次平铺 logical/sequence/physical 三字段。Orchestrator private session/internal event 可按 owner-local ledger 结构保存其状态字段，不构成跨 owner wire authority。LLM block 返回的 document 必须绑定同一 snapshot 和 request identity，transaction/identity 漂移不得进入执行。
- LLM owner-private candidate response 不复制 sealed provider audit/typed error 已拥有的 `RequestID/Status`；usage audit 从 canonical audit/error 读取，不能以 response mirror 覆盖 provider outcome。
- candidate request 携带从 persisted facts 和唯一 manifest 渲染的 typed `RuntimeCandidateContext`；当前 focus/evidence/failure 是 context，operation kind/parameter/target/effect/dependency 与 `BindingRef` 授权只来自 manifest。provider wire 只允许 proposal 字段并明确禁止 operation `id`/`dependsOn`；identity/dependency 由 Orchestrator local compiler 派生。candidate 不得以 raw `coverageWorkItemIds` 自报 credit。C4 只投影 provider-safe failure/evidence/assessment/shape identity 与 attempt-1 persisted directive 的 detached correction input（manifest/objectives/witness）；它不携带 `Disposition`/`TerminalCode`、raw reason/summary/payload、owner feasibility 或 recovery/governance action。LLM block 不从 禁止的 map、raw reason 或临时上下文重建 authority或 persisted directive。
- Runtime dispatch 在执行前持久化 `SealedDispatch`、完整 transaction、candidate hash、sealed binding references 和 typed step input；execution evidence 只有在 snapshot、dispatch、operation、binding 与 owner receipt 一致时才能提交并触发下一次 reducer。
- capability owner 的 validation receipt 必须持久化中立 quality/zero-work/check observation，并纳入 stable receipt hash；SM-3 在普通响应与 exact-fence restart recovery 两条路径执行同一质量裁决。receipt 只有实际匹配 sealed binding 的 target/effect/recipe/check（适用时 metric/artifact）才可进入 SM-2 proof gate；multi-target prospective binding 必须再满足同一 dispatch 的完整 collective receipt set，单个 receipt 不授予 CWI credit。弱验证、非空 `PassedChecks` 或 owner exit code 0 只进入 rejected/diagnostic evidence。
- Runtime aggregate 通过 Run owner 的 EventHub update 合同持久化；Orchestrator 使用
  `ExpectedRuntimeStateRevision` 执行语义 CAS，不直接访问 Run repository，也不把 aggregate 编码进
  `map[string]any`。Run owner 对同一 aggregate revision 上的无关 Run model/lifecycle 写入执行物理
  CAS rebase；完整 Run model revision 不是 aggregate fence。
- Orchestrator 对 Run/Workspace lifecycle 只持有显式 snapshot reader（各自仅一个 `Read`）；没有
  `Create/Update/Delete/List` lifecycle mutation facade。opaque RuntimeStateRecord 的 read/CAS 是独立 Run owner 合同，
  不能借 snapshot reader 扩宽为跨 owner repository。原先没有生产调用方的 generic Run Create EventHub command 同步
  删除；Run 创建只由 Run owner 消费 typed workspace-execution intent 后在 owner 内完成。
- Runtime 初始化时封存 typed Goal/DoD definition context；后续 candidate request 与重启恢复只能读取 Run owner 中持久化的定义快照，不能由调用方临时重建。
- Runtime activation 同时封存 bounded `MemorySelection` provenance。candidate/selected/excluded、quota、score、hit 与
  byte/token budget 只说明 context selection，不能作为 SM-2 coverage credit；selected identity 必须与 activation 中
  的 Memory context 精确一致。Run owner 以稳定 CreatedAt 将该 evidence 交给 Memory owner 做 create-once audit，
  Orchestrator 恢复不得从 MemoryRecallTrace 重建 authority。
- `InitializeRuntimeFromRun` 只读取 Run owner 已持久化的 typed bootstrap，生成 initial facts 并执行 preflight/reducer；Orchestrator 不解析 禁止的 execution context map 来重建定义或 focus。
- Runtime controller 统一负责 initialize、prerequisite commit、candidate seal 和 evidence commit；服务重启只加载既有 aggregate，不得因重复 Start/Resume 再次执行 reducer 或增加 attempt。
- initialize/restart identity 只比较不可变 workspace/run、definition revision/content 与 activation；`Facts.Focus` 是 SM-2 persisted cursor，合法 focus advance 后不得被拿来与 bootstrap 首 focus 比较并误判 definition drift。
- prerequisite mainline 按 typed requirement 的 canonical owner module 调用 EventHub，并在每个结果提交后原子执行 reducer；profile capability-state owner 必须与 accepted operation descriptor owner 相同，不能使用 EventHub destination 别名；多个 capability owner 的结果必须合并，不能互相覆盖。
- `need_validation` candidate 前，validation policy 声明的
  `workspace_manifest | project_files | declared_commands | available_capabilities` 必须由 fresh resource 与 matching
  prerequisite observation 同时证明。缺失 discovery 是 SM-2 typed prerequisite debt，不是
  capability-unavailable feasibility；局部 target/failure/provider freshness preflight 只能保留现有 pending graph 后追加，
  不得覆盖其他 owner debt 或把 satisfied prerequisite 重编译为新 identity。
- capability prerequisite 的 owner/class/kind 不匹配或没有 responder 必须以 owner-neutral typed `RuntimeCapabilityUnsupportedError` 返回，并由 SM-4 分类为 `runtime_capability_unsupported` terminal contract defect；它不得退化成默认 capability、candidate rejection、泛化 decision/approval ticket 或 retry loop。各 capability owner 的支持 kind 只在其 `pkg/common.RuntimeCapabilityKinds()` 声明，handler 只消费该纯 inventory；具体 EventHub result 由接收 owner 定义，owner-neutral catalog observation 仅由 `workorchruntimecontract` 的纯 helper 构造。
- operation admissibility inventory 由 canonical owner 提供完整 bounded scope 或封闭 selector，SM-2 将其编译为三种 `TargetPolicy` 之一。既有资源与 prospective target 都必须在 seal 前由同一 owner 做只读 feasibility probe；Orchestrator 不扩展 selector，也不把 inventory/context 截断当成完整授权。
- semantic manifest、provider JSON schema 与纯 evaluator 必须消费同一条 owner-published
  `operation kind + concrete variant + target policy` fact。owner catalog 已列出的现有目标不得与只允许
  `prospective_owner_target` 的 create-only fact 组合；若同一 owner catalog 为该 evidence/target policy 发布了合法
  replacement kind/variant，attempt-1 correction 只携带该 sealed alternative（例如 `patch/apply/text_replace`），
  不解析 owner message，也不把 exact target 改写成虚构的 prospective target。目标内容 revision 仍由后续 owner
  snapshot/preflight 与 candidate manifest 绑定，LLM 不提供 revision authority。
- sealed candidate 通过接收 owner 自有 concrete execute command/result 按 operation kind 投递到 File、Git、Shell、MCP、Web 或 SSH owner；跨组件 concrete result 只包含具体 operation/transaction、唯一 receipt outcome、stable factual category、完整安全 original diagnostic 和 evidence，不保留 `Status + Receipt.Outcome` 双权威。
- SSH dispatch 只封存 opaque target/session/credential refs、bounded command 与 timeout；endpoint、username 与 credential material
  不进入 aggregate 或 EventHub。SSH owner 在 exact grant preflight 后执行 strict host-key transport，并以 owner-private 前后 observation
  形成 `remote_durable`/`no_op` 事实；Orchestrator 不从 sealed expectation 推断远端 changed。
- Web dispatch 只向 owner 发送完整 `SealedRuntimeOperation`，不再发送可与 sealed payload 漂移的第二份 operation data，也不接受
  `approvalId` 参数或普通 Approval 查询旁路。Web search/fetch 发布 `none + at_most_once`；owner 在 external request 前验证
  Orchestrator source、current claimed receipt authority 与 exact grant。DNS/redirect/SSRF 判定和实际 transport dial 由 owner 绑定，
  receipt 只返回真实有界 content summary/hash、opaque source/raw refs；请求提交后的 receipt-loss 必须由 SM-3 收敛为 unknown。
- MCP dispatch 同样只向 owner 发送完整 `SealedRuntimeOperation`；Orchestrator adapter 从 persisted MCP admissibility
  选择 exact Capability/policy 并绑定 catalog/Registry revision、完整参数、effect/idempotency 与 validation surface。
  MCP owner 的 SM-2 feasibility 零 endpoint/process side effect；SM-3 external call 前重验 producer、claimed receipt
  authority、current Registry revision、Server/Capability/allowlist/schema/policy 与 exact approval grant。tool/resource/prompt
  输出只以 owner-private bounded hash/raw ref 进入 receipt；durable effect 必须有真实 changed/no-op revision observation，
  不能从 capability 声明回显 `Changed=true`。
- 多 operation dispatch 按 dependency graph 确定性执行；每个 operation 独立提交 evidence，全部成功或任一失败后才结束 dispatch 并再次 reducer。
- 同一 dispatch 的 durable operation 必须具有唯一 `(OwnerModuleID, TargetRef)`；同一目标的多段修改必须在候选中合并，
  owner-sealed exact `RequiredOperationSequence` 除外；capability owner 的 replay-proof/CAS guard 继续作为 effect 前最后防线。
- RuntimeFacts 持久化当前 focus 与有序 remaining focus queue；只有当前 focus 每个 required CWI 都获得 exact binding-matched evidence 后，evidence commit 才能原子切换到下一 focus，再调用同一 reducer。
- 显式声明 `RequireAssessment` 的 focus 在执行证据齐备后进入 typed `need_assessment` authority；LLM 只能返回绑定当前 focus/coverage 的 `AssessmentCandidate`，Orchestrator 校验其 evidence refs 来自当前 Run 的 accepted evidence 后写入 assessment ledger。
- SM-2 在 assessment commit 前执行 item-level evidence gate：weak evidence 永不关闭 coverage；任何 executable/claim evidence 必须逐项匹配 sealed `CoverageEvidenceBinding` 与 owner receipt 的 exact proof。`PassedChecks` 非空和中立 claim signal 不能单独通过门禁。门禁失败写入 `assessment_evidence_quality_gap` repairable assessment，并由 reducer 在封存预算内继续同 focus repair。
- 最后一个 assessment 关闭或 typed terminal failure 形成后，Orchestrator 从 current-run evidence/coverage/assessment ledger
  密封 terminal 与 result payload，并创建 terminal delivery fact；Result owner 自行持久化 accepted/rejected outcome，
  Workspace owner 自行释放 active run，Run owner 自行进入 terminal。Result block 不参与 acceptance 裁决。Result、Workspace、
  Run 三段 SM-4 delivery 只携带同一 sealed `TerminalOutcome`；禁止另传 `FailureReason` 覆盖 code/summary，Run owner 仅在
  private reducer 中从 outcome 派生自身持久化展示原因。current-schema terminal closeout 还绑定 StartupInputHash、
  aggregate revision、逐 requirement/CWI coverage/assessment/evidence 三轴、accepted evidence union、deliverable refs、
  known limitations 与实际依赖的 owner revision set，并封存 canonical closeout hash；completed 要求每项 covered、satisfied
  且至少一条 accepted evidence。public projection 只包含 current canonical closeout 字段。terminal closeout 只投影仍未满足 CWI 的 canonical
  assessment gap；已满足 CWI 的 producer claimed gap 仅留在 assessment audit，不得成为 MissingEvidence/TargetRefs
  或 top failure。失败摘要优先使用当前 `Facts.Failure`，其次为 current active/formal gap，最后才使用 terminal
  code/recovery lineage。
- CoverageFocus 可携带 evidence role、accepted/rejected step kinds 与 accepted operations；reducer 只在 owner 已确认的 capability kinds 中做中立交集/排除，并把结果封入 `RequiredOutputContract`。若声明 accepted operations，candidate 至少包含一个匹配的具体 operation 参数，不能用同 step kind 的其他动作替代。
- provider request session（logical request ID、expected-document identity、physical-call recovery generation、transport
  outcome）先于 candidate attempt 随 aggregate 持久化。EOF/output limit 不能创建/失败 attempt，只能重提同一 expected
  document且不能拼接 raw output；coherent empty/invalid 则在结算当前 session 后形成 SM-3 typed document-conformance attempt。
  document-shape、complete schema 与 semantic rejection 先保存 typed diagnostic，再由 SM-3 owner-local reducer 决定 final
  或唯一 same-step correction；只有 persisted `correctable_once` result 可在相同 transaction、manifest 和 stable prefix 下
  开启 attempt 1，attempt 1 再次出现任一 rejection 形成
  `candidate_retry_exhausted`，不能创建第三次请求或无信息 governance ticket。
- recoverable provider result 统一经 `NormalizeRuntimeProviderRequestRecovery` 形成一个 deterministic owner-private fact，
  operations 与 semantic-plan 不得分别推导恢复/耗尽语义。`provider_request_recovery_coordinated` 的 metadata primary class
  是 SM-4，但它只在唯一 Orchestrator aggregate 内组合三个彼此独立的 reducer 结果：结算 SM-3 request session、transport
  时消费 SM-4 `provider_transport` guard、guard 耗尽时应用预先归一的 SM-2 failure；整组结果只提交一次 CAS。incomplete
  document 不消费 transport guard，只依据 sealed ExpectedDocument `MaxRecoveryAttempts` 在同一 CAS 中进入 recovery-pending
  或 terminal。body-idle 不得旁路该 guard，也不得直接伪装为 protocol terminal。
- LLM result 的 exact audit 是 physical submission 的唯一事实来源；Orchestrator 将其归约为 owner-private immutable
  `confirmed | released | unknown` settlement，并与 SM-3 session、SM-4 recovery/ledger 以及可选 SM-2 terminal 在同一
  aggregate CAS 中提交。`ProviderSubmitted=false` 必须释放该 physical call 的 provider、reasoning、semantic generation
  与 token reservations，不能产生 consumption；`ProviderSubmitted=true` 才能确认 consumption，usage 缺失时仅 token
  allocation 进入 unknown。audit 缺失或 workspace/run/logical/physical identity 不可信时全部进入 unknown 并以
  submission-unknown 终结，禁止盲重放；`RequestBytes` 永远不能推断 submission。stale 只接受 released，incomplete 与
  transient/body-idle recovery 只接受 confirmed，避免无 transport-budget 循环。
- recovery CAS 写入前中断时，fresh load 只能看到既有 `ProviderSubmitted=true`，必须收敛为 submission-unknown 且不得再次
  调用 provider；CAS 已写入但调用方丢失响应时，fresh load 必须直接重放 durable pending/terminal decision，duplicate 不得
  再消费 budget 或增加 physical call。terminal transport session 的 `LastOutcome` 保留 LLM owner 的 factual
  `provider_transient` class 与 exact original code/signature；operations 的 stable terminal code 可为
  `provider_transport_exhausted`，semantic 的 stable terminal code 可为 `semantic_provider_transport_exhausted`，但两者的
  nested `OriginalDiagnostic` 必须与 producer fact 精确一致。
- authentication/configuration/protocol/transient-exhausted/submission-unknown/document-recovery-exhausted 统一先经
  `NormalizeRuntimeProviderRequestTerminal` 形成唯一 owner-private fact，再由
  `provider_request_terminal_coordinated` 在同一 aggregate CAS 内同时关闭 SM-3 request session 并应用完整 SM-2
  successor failure；operations 与 semantic-plan 不得各自重写 code/signature/`OriginalDiagnostic`，也不得恢复旧的
  “先 terminal session、后 failure”双 CAS。SM-2 与 SM-3 的 reducer 语义仍独立，只在 Orchestrator owner 内部以该明确
  transaction 原子协调：CAS 写入前中断时 fresh load 只看到 submitted marker，必须确定性封存
  `provider_submission_unknown` 且不重调 provider；CAS 已写入但响应丢失时 fresh load 必须看到 exact terminal+failure，
  duplicate 只读返回。terminal session 若缺少 matching failure/semantic ledger 必须 fail closed，不能作为成功恢复旁路。
  empty/invalid 明确不在该 request-terminal 集合；它们经 SM-3 document-conformance rejection 使用共享 attempt 0/1，
  不应用 provider-protocol SM-2 failure，也不消费 SM-4 transport/whole-document recovery budget。
- LLM provider feedback 的 stable category 只用于选择 SM-2/SM-3 路径；authentication/configuration/transient/protocol
  terminal failure 必须同时保存 LLM owner 提供的 exact `OriginalDiagnostic`。例如 canonical terminal 可保持
  `provider_protocol_failure`，但 nested original code 必须仍是实际 guard（如 `response_byte_limit_exceeded`），且
  aggregate reload、duplicate drive 与 projection clone 均不得将其泛化或覆盖。
- candidate retry 预算按稳定且显式持久化的 `AuthorityContractID` 计算；identity 绑定 workspace/run、完整 `AuthoritySnapshotID`、focus、canonical convergence state、definition/evidence revision 与 requiredOutput hash。派生 action 不参与 identity。缺失或不匹配该字段属于 current-schema invariant violation；同一 contract 超过 attempt 0/1 必须 fail-fast，禁止再次请求 provider。
- coherent empty/invalid、complete local schema/contract failure 只记录 provider-safe typed rejection；correction/final 由
  SM-3 internal event决定，三者共用 candidate attempt 0/1。owner feasibility 的
  `stale`、`prerequisite_required`、`infeasible` 必须作为独立 internal event 交给 SM-2，绝不进入
  correction policy 或占用 attempt。provider authentication/configuration、transient transport、incomplete
  document 与 unknown submission 走独立 dependency 或 request-session/recovery ledger。prerequisite stale
  精确退休旧授权，历史 execution failure 不得触发 authority reseal。
- semantic `stale | prerequisite_required` 必须先以 `semantic_feasibility_observed` 独立 CAS 保存。只有带完整
  expected/observed revision、精确匹配 current catalog dependency 的 `stale`，才能复用 `semantic_contract_required`
  private entry，以 persisted refresh generation 原子保存 gap lineage、退休 matching catalog observation 并回到有界重获。
  当前没有 owner-defined typed semantic prerequisite request 时，`prerequisite_required` 保留 catalog、observation、预算
  与原始 diagnostic，由 SM-2 fail-closed terminal；不得伪装成 stale 或 generic refresh。新 revision 编译新
  manifest/request session，旧 complete session 只读保留；generation history identity 固定为
  `manifestHash + logicalRequestID`：同一 active generation 的 correction/focus request 变化不重复记 generation，
  资源回滚重现旧 catalog hash 时可由新 logical request 开启独立 generation。duplicate generation 不写 CAS，预算耗尽才以原始 owner
  diagnostic 执行 `semantic_contract_failed`。上述处理不新增 phase、不消费 candidate correction，也不创建
  Decision/Approval。
- catalog acquisition 阶段的 validation-surface `prerequisite_required` 是 capability owner 的 readiness fact，不是上述
  semantic-feasibility transition：它不打开 request、不调 provider/endpoint、不增加 attempt/revision budget，只从
  provider-selectable recipe set 中移除该 identity。技术上 `ready` 但声明为 `auxiliary`/`not_declared` 的 surface 同样只
  保留 audit，不能进入 provider 或直接关闭 CWI。若存在 `ready + direct` alternative，后续 planning 继续使用同一 canonical
  catalog；没有合法 proof alternative 时才由既有 SM-2/SM-3 bounded fail-closed 规则终结。未来若自动获取输入，必须
  先由对应 owner 新增 concrete typed acquisition command/result，不能由 Orchestrator 或 LLM 猜参数。
- semantic planning发现 Definition 本身存在无法由owner fact补齐的typed gap时，SM-2先持久化
  `RuntimeDefinitionClarificationGap`，SM-3再封存唯一 clarification dispatch并通过Decision专用
  `OpenRuntimeDefinitionClarificationCommand/Result`创建 blocker。ticket、StartupInputHash、Definition revision、authority、
  focus、semantic request/document与dispatch必须逐字段一致；existing Run不接收文本patch或普通governance continuation。
  解决只能由Run owner先持久化successor `ReplacementIntent`，再用Decision专用replacement resolution回绑；重复/重启复用
  同一identity，stale或另一replacement fail-closed。
- governance candidate 作为 sealed dispatch 持久化；治理结果只能由 Decision/Approval 各自的 owner-specific source command
  提交，生产路径不订阅可由任意 sender 调用的 generic resolution topic。同一 resolution ID 与 authority snapshot 最多消费
  一次；processed replay 必须逐字段匹配 normalized ticket/approval、dispatch、outcome、dependency、subject、summary 与
  evidence refs，漂移一律 conflict/零写。Result 强制绑定 workspace/run/resolution scope，并保留正交的
  `committed | stale_diagnostic | duplicate` CAS 提交事实；exact stale replay仍返回 `stale_diagnostic` 且不进入 lifecycle
  continuation，不能被 ProcessedEventID 重判成 committed duplicate。
- Decision owner 通过 `CommitRuntimeGovernanceTicketResolutionCommand` 只提交 ticket resolution；Approval owner
  通过 `CommitRuntimeGovernanceApprovalResolutionCommand` 只提交 approval resolution。Orchestrator 从 persisted
  binding 补齐 authority snapshot、dispatch 与 dependency 后进入同一 resolution commit。
- governed dispatch 对非 approval dependency 使用 `OpenRuntimeGovernanceRequestCommand/Result` 向 Decision owner
  创建确定性 ticket；`approval` dependency 使用 `OpenRuntimeApprovalRequestCommand/Result` 向 Approval owner 创建
  不可变授权请求。ticket/approval、snapshot、dispatch、dependency、requested action 与 authorization hash 的绑定
  随 Runtime aggregate 持久化。request raw record 必须已经 canonical，且与 governance dispatch 中的完整 dependency
  `kind + subject + requested action + risk + authorization hash` 精确一致；缺字段、重复 request 或任一漂移都 fail closed。
- `DriveRuntimeCommand/Result` 是 Runtime 唯一同步推进合同；Orchestrator 在有界循环内依次处理 prerequisite、candidate、sealed dispatch、typed operation、evidence/reducer。Result 不保存第二个 disposition：`need_governance` 必须携带且只携带 `BlockingTicketID` 或 `BlockingApprovalID`，`complete|failed + Terminal` 表示 typed terminal，其它组合均是合同缺陷。重复调用和进程重启必须从 Run owner aggregate 幂等恢复。
- drive 同时校验 persisted `StateRevision` 与 `RuntimeConvergenceProgressSignature`；signature 覆盖 semantic seed/request/attempt/document/owner batch/sealed binding、focus/evidence/canonical state/requiredOutput/repair/prerequisite/dispatch/governance/terminal，不包含派生 action、operation-candidate retry、snapshot attempt、SM-4 lease 或 revision bookkeeping。只增加 bookkeeping 的 transition 必须 fail-fast。
- SM-4 delayed recovery 只由 aggregate 中的 `RuntimeDriveLease` 授权。lease 绑定 workspace/run、aggregate revision、recovery generation、stable reason/signature、due time 与 `scheduled|running`；timer 只能 claim exact due lease，不能自行决定 retry。进程内 workspace/run singleflight 与 timer map 仅合并该 persisted identity；重复 Start/Resume/governance continuation、startup recovery 和旧 timer 均不得建立第二条链。crash 后遗留 `running` lease 由 startup 重新封存为新 generation 的 scheduled lease；live worker 再次失败时也只能凭其 exact claimed generation 以单次 CAS 续排更晚 dueAt 的新 scheduled generation，旧 generation finish/release 不得删除该后继。旧 timer 的 parse/claim/not-claimed/adopt/schedule 任一退出都必须在同一 gate 临界区选择 authoritative newer lease 或 mount latch 并转交 timer，否则释放 gate；同/异 signature 的 resolution overwrite 均不得遗留 latch 或泄漏 gate。renewal CAS 失败后禁止 release running generation；owner-private recovery 也失败时保留 persisted running authority 与本地 gate，普通 ingress 不得建立 generation-zero drive。Stop 的首次 cancel 必须原子发布 lifecycle closing/epoch；BackgroundRoutine submission 外部调用期间不得持有 `driveMu` 或另一 Stop fence，已排队/已到期 callback 必须在 observe、claim 前重复核对 captured context 与 exact lifecycle epoch。cancel 点之后 handler 与旧 timer defer 均不得创建 context、提交 background timer或清理新 epoch gate，write fence 内再 cancel/clear 本地状态，fresh startup 只恢复唯一 generation。
- 所有 Orchestrator drive worker（包括 generation zero 的同步 ingress）还必须满足统一退出 fence：退出时要么仍持
  run-scoped gate 继续下一 owner-private transition，要么 aggregate 已处于 exact terminal/governance/recoverable wait/
  lifecycle handoff，或 exact `RuntimeDriveLease` 已持久化并挂载。drive body panic 在 Orchestrator owner 边界被规范化为
  稳定 owner-local failure，并复用既有 private lease transition；不保存 panic 原文，不让 Run/Application/projection
  选择后继。lease CAS 或本地 timer mount 失败时保留 gate 与已有 scheduled/running authority，由 fresh startup 恢复 exact
  generation；不得虚报 live timer，禁止 defer 以 `retry=nil` 释放成 active orphan。
- 真实 `RuntimeCASConflictError` 是唯一进入 CAS reload ledger 的事实；semantic persistence rejection 不创建 CAS/retry lease。CAS ledger 耗尽停止计数增长但仍保留带退避的 persisted owner-local successor，避免 active aggregate CPU 空转或无后继静止。
- reducer/aggregate 已进入 governed wait 或 terminal 后，后续 owner projection 失败只能记录 projection error 并等待幂等重试，不得把已确定的 complete/fail authority 改写成 `runtime_drive_failed`。
- 普通 drive、governance resolution 与 drive-failure terminal 共用唯一 terminal delivery lane：先校验 persisted sealed aggregate，
  再发布 Result -> Workspace -> Run 的有界 delivery fact 序列，并仅在 SM-4 记录 acknowledgement；ack 只能取自
  该三个 owner 的同步 typed response 后转入 Orchestrator private internal event，绝不开放独立 EventHub ack command；
  任一 delivery 失败进入延迟重试，不得改写 reducer terminal 或越权写 owner state。SM-4 的 delivery request、Result
  material、Run lifecycle fence 与 acknowledgement 校验必须共用 `runtime.DeriveTerminalOutcome`；administrative 与
  convergence 双 terminal、terminal state 缺 detail、non-terminal 携带 detail 或非法 outcome 一律 fail closed。
  `runtimeController == nil` 时 acknowledgement 必须返回 invalid argument，禁止将未持久化 ack 当作成功。
- Start/Resume 由 Application 经 Workspace owner ingress 提交 scoped intent；Run owner 内部迁移并发布 `RunStateChanged` 后，
  Orchestrator 才后台执行唯一 Runtime drive。application 不再拥有 Start/Resume continuation queue，Orchestrator
  service 不暴露 execution-context-patch resume facade。Workspace 与 Run owner 的同步 intent Result 只携带 typed
  `RuntimeExecutionIntentOutcome + RuntimeExecutionIntentReason`；不得用自由文本 `Disposition + Diagnostic` 形成第二套
  admission authority。外部协议中的展示文本只能由 reason 纯派生，不能反向驱动 SM-1。Result、Workspace、Run 的
  lifecycle synchronous Result 同样不得输出 owner-private no-op `Diagnostic`；调用方只消费 model、transition fact 与
  sealed delivery acknowledgement。
- Orchestrator biz 不保留 `AcceptRunStart`、`CanResumeRun`、blocker/terminal Run/Workspace 投影或等价 lifecycle
  facade；不存在可直接调用的 禁止的 `StartRun`、`DispatchNextStep`、`ResumeRun`、patch resume 或自动 replan lifecycle。
- 禁止的 runtime progress/terminal-result checkpoint、restart replay、decision resume subsystem 和 approval patch resume 已删除；Runtime 重启只加载 persisted aggregate，治理继续只消费 typed resolution。
- Orchestrator plugin 不再创建 RuntimeAdapter registry、EventHub runtime adapter 或 Step kind registration matrix；Runtime operation 只通过各组件 typed EventHub command/result 执行。
- Orchestrator 不再暴露 禁止的 `NextActionForRun` 或接收 `UpdateRuntimePolicyCommand`；Runtime action 只来自 typed reducer，动态策略从 canonical configuration source 按需读取。
- blocker/continue/terminal 只作为 Orchestrator persisted governance/terminal fact 发布；Run/Workspace/Result owner 的
  reducer 负责各自迁移。不得借此创建 禁止的 Step 或经过 禁止的 Result/DoD completion gate。
- SM-4 对 owner-local delivery acknowledgement 做有界重试：重复 fact 必须幂等，单边成功不得重写，缺失 ack
  只保留 delivery pending，避免单边成功后永久悬挂。
- repair ledger 的 `RepairAttempts` 是同 focus 总预算；有效新 evidence 只重置 failure-signature、validation-only 与 no-progress 窗口，不清零总预算。只有原子 focus advance 才创建下一 focus 的全新 repair ledger。
- owner 报告 state-changing no-op 时，SM-3 只持久化 rejected/no-progress shape，绝不写入 state-change evidence。
  SM-2 对要求 assessment 且有可观测 validation 的 focus 只能按 `validation -> assessment` 验证已有状态；否则以
  typed current-run provenance gap 终态化，禁止通过无关文件修改制造 durable delta。exact receipt replay 保持原
  `ExecutionKey + Fence`，不重放 owner side effect。
- candidate attempt 只投影为 SM-3 contract attempt；read model 的 repair loop、ineffective retry 和 repair budget 必须来自 `Facts.Repair` 与当前 failure，不得把历史 candidate retry 当成当前 focus repair。
- prerequisite freshness 只有一条有界的 stale/no-progress `prerequisite_refresh` ledger。owner accepted receipt 的真实资源变更是正常进展：它原子退休精确相交的 observation、持久化 invalidation record，但不扣减 recovery budget。capability-state acquisition key 的代际仅由同一 `PrerequisiteID` 的 persisted invalidation records 推导；因此资源变更会重查 owner 而不会重放旧 receipt，且无关 prerequisite、provider、CAS 与 delivery 记账都不能改变 key。
- 同一 prerequisite 已被 invalidation record 退休的 observation/revision 不得以延迟、乱序或 ABA 回滚重现的形式重新进入
  current authority；即使资源内容回到历史形态，owner 也必须发布新的 opaque generation/revision。semantic manifest 的内容
  hash 可以在资源回滚后重现，但只能绑定新的 logical request generation，不能复用旧 owner observation 或 active request。
- application projection 消费 Orchestrator immutable drive observation；Eventstream 当前不直接订阅 Orchestrator topic，Runtime 只消费同步 typed operation result 推进状态。
- Workspace 删除采用 typed 两阶段握手：Orchestrator 只在 exact Run-owner `draining` fact 已持久化后允许既有
  terminal-delivery/lease drain；`BeginWorkspaceRuntimeQuiescence` 在 Runtime CAS、drive/timer 与 evidence mutation 共用的
  workspace barrier 内确认无进程内 writer、无 lease 且 terminal delivery 已完整 ack，随后由 owner-private reducer 落
  `quiescing` gate。gate 落盘后所有普通、延迟与恢复 Runtime/evidence ingress 均 fail closed；`Confirm` 只从同一冻结快照
  生成绑定 Run generation/revision、canonical Runtime state ref/hash 与 Orchestrator revision 的 typed quiescence token。
  Run owner exact finalize 为 `retired` 后，Orchestrator 才能将私有资源推进为 `fenced`；retain 与 purge 都永久保留 tombstone。
- purge 只接受 `blocks/workspace` 的 fixed-mode concrete command，调用者不能选择 owner phase 或把 retain replay 为 purge。
  command adapter 只能提交 owner-private `purge_requested` event；同一 private dispatcher 先推进 evidence purge journal，
  再以 `purge_completed` event 提交 deletion aggregate CAS，adapter 不得直接调用物理 purge sink。canonical memory/file
  repository set 在同一 durable workspace gate 下按 WorkspaceID 清理自身
  ToolCall/Patch/ValidationResult/Evidence/Coverage/ArtifactContinuation 以及 orphan chunk/assembled children；receipt 对十个
  logical surface 分别记录 `observed-before/deleted/remaining=0` 并纳入 proof hash。file backend 在任何物理删除前先持久化
  `pending` receipt，删除并复核十个 surface 归零后才持久化 `completed`；进程在物理删除与 aggregate CAS 之间崩溃时，
  restart 必须完成 pending sweep 并 exact replay 首次非空 receipt/proof，不能重算全零证明。Run、Step、RuntimeCheckpoint、
  MemoryRecallTrace、Result 与 Artifact 均由各自 owner 的 fixed-point sink 清理；不存在逐 Run purge、跨 owner cleanup saga
  或 foreign-owner generic repository。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `blocks/run` | 双向 | SM-1 notification、recovery inventory/readiness、opaque Runtime record query/CAS、terminal/lifecycle fact | 完整；Run 不解释 SM-2/3/4 |
| Result / Workspace / Run | Orchestrator → owner | 同一 sealed `TerminalOutcome` 的三段 delivery/ack | 完整；各 owner 只做自身 reducer |
| `blocks/llm` | Orchestrator → LLM | provider ceiling、preflight、candidate request | 完整；LLM 只返回 factual result |
| File / Git / Shell / MCP / Web / SSH | 双向 | owner capability/admissibility/feasibility/reconciliation/execute 与 receipt query/commit | 完整；能力 owner 不选择下一 action |
| Decision / Approval | 双向 | governance open/resolution/evidence 与 immutable continuation/terminal fact | 完整；无 Application resume 旁路 |
| `kernel/governance` | 无直接 I/O | 经 Decision/Approval/Run typed fact 间接协作 | 完整；不存在 Orchestrator ↔ Governance 直连合同 |
| `blocks/memory` | Orchestrator → Memory | workspace-scoped concrete item/trace read | 完整；Memory 删除只由其 owner-private fixed-point sink推进，activation trace 由 Run owner 记录 |
| `application/workorchd` | 双向 | Orchestrator → Application immutable drive observation；Application → Orchestrator evidence/read-model list | 完整；Application 不推进 reducer、不持有 aggregate |
| `blocks/workspace` | Workspace → Orchestrator | typed quiescence begin/confirm、deletion seal 与 fixed-mode owner-local evidence purge | 完整；Workspace 是 durable coordinator，Orchestrator 只推进自身 private reducer |
| Skill Registry / SkillHub Catalog / Soul / MCP Registry | 无直接 I/O | 只消费 Run owner 已持久化的 activation snapshot | 完整；Skill provenance/budget 与 Soul guidance/allowlist 已在 activation 封存，后续 Registry/Catalog 更新不改变既有 Run |
| `blocks/eventstream` | 无直接订阅 | 审计通过正式 owner notification 形成 | 完整；不从审计重建 authority |

## 相关约束

- 核心 runtime 必须保持领域中立，不得写死软件开发、语言栈、测试框架、MCP 或业务领域逻辑。
- 每次 dispatch 必须基于一致的 workspace/run/definition/evidence 快照。
- stale evidence 只能进入 diagnostic lineage，duplicate event 必须幂等；运行重启后从持久化 aggregate 恢复，不允许依赖进程内去重 map。
- execution evidence 的 CWI credit 必须由 sealed operation 的 `BindingRef`、matching owner receipt 与 persisted
  `CoverageEvidenceBinding` 派生；不得复制 candidate 自报的 coverage work item IDs。前序 focus evidence、
  assessment、check 名称相似、文件存在、Git commit 或通用 validation 成功不得满足后续 focus；最后一个 focus 的
  每个 required CWI 都有 binding-matched proof 后才允许 Complete。
- profile-specific accepted/rejected kinds 只能作为 typed focus requirement 输入；reducer 不识别 profile ID、deliverable ID、软件开发术语或具体业务语义。
- Profile required-source completeness is decided before Run creation; Orchestrator consumes only proof-complete typed focus
  declarations and never invents proof kinds, owner operations or terminal members from a Profile label or description.
- authority attempt 只有在 canonical state、focus、definition/evidence revision 和 requiredOutput hash 全部相同时才增加；prerequisite 或其他事实推进必须重置 attempt。派生 action 不参与 identity。
- block adapter 的既有 map 输入只能存在于消费 block 内部的技术适配边界，不得重新进入 EventHub command/result、RuntimeFacts、candidate、dispatch 或 evidence。
- accepted candidate attempt 必须对应 active 或 completed sealed dispatch；不存在 dispatch 的 accepted attempt、第三次请求或 accepted 后重试都必须被 invariant validation 拒绝。
- drive loop 的每个非等待、非终态 transition 都必须增加 persisted state revision；无持久化进展或超过 transition guard 必须 fail-fast，禁止进程内空转。
- Start/Resume 生产入口仅提交 Run owner intent；缺少 typed bootstrap 由 Run owner 与 terminal delivery saga
  收敛为 typed terminal；`RuntimeBootstrap == nil` 必须失败。
- `ResumeRunCommand` 不得重新增加 loaded Run、`ExecutionContextPatch` 或其他 禁止的 continuation payload；上下文变化只能通过创建新的 Runtime activation/bootstrap 表达。
- Orchestrator service facade 不暴露 `DispatchNextStep`；Runtime operation 只能由 persisted drive/reducer 主链调度，外部 module 不得直接推进 Step 投影。
- Orchestrator service facade 不暴露 Start/Resume；这两个生产 EventHub handler 属于 Run owner，Run state notification
  才触发 Orchestrator internal drive，禁止外部组件通过 service lifecycle 重新进入 禁止的 主链。
- Runtime/Orchestrator 不自动创建 replan successor；current-baseline restart 的 `run.replacement` 是同一 Run owner 下
  的 SM-1 subordinate transaction，replacement intent、terminal delivery acknowledgement 后 successor materialization
  与 lineage 都由 Run owner 管理。重新定义目标或执行基准必须使用新的 typed
  activation/bootstrap，不能由 Orchestrator 从旧 aggregate 派生。
- 不记录或恢复 `runtime_step_result_available`、watchdog recovery policy 等旧 checkpoint；历史 checkpoint 仅作为只读审计数据。
- Run/Workspace 状态通知不再隐式写入 禁止的 runtime checkpoint；状态事件与 Runtime aggregate 各自保持单一职责。
- HTTP runtime 配置更新不得向 Orchestrator 推送进程内 repair policy 副本，避免 canonical config 与 禁止的 policy cache 形成双权威。
- Orchestrator 不注册 ValidateRun/CompleteRun command；validation 只能提交 sealed operation evidence，completed terminal
  只能来自 reducer decision 与 Result/Workspace/Run 各自的 typed owner-local delivery。
- 禁止的 `EnableEventDrivenDispatch`、`DispatchStepRequested`、通用 runtime executor 与 runtime-gap recovery command 已删除；组件执行不得形成第二条 dispatch owner。
- 禁止的 `StagePacket`、`NextAction`、RuntimeStep Accepted/Progress/Completed/Failed 通用事件结构已删除；不得以历史合同重新包装 Runtime facts、candidate 或 operation evidence。
- governance continue 必须写入 accepted governance evidence、递增 evidence revision 并回到同一 reducer；governance fail 直接生成 `governance_rejected` terminal，stale resolution 只能进入 diagnostic lineage。
- resolution 必须携带并匹配已绑定的唯一 ticket/approval identity；不存在绑定、同一 dispatch 绑定不同 owner
  identity、或 dependency/authorization target 漂移都必须返回 conflict。
- operation-scoped governance continue 只恢复 suspended binding 指向且未列入 source `CompletedOperationIDs` 的 exact
  `owner_committed` governed execution。source dispatch 必须为 committed，且 history 顺序严格为
  `source operation dispatch < committed governance dispatch < resume dispatch`；grant 只能授权唯一 `ResumeDispatchID`。
  source `governance_required` receipt/history 保持不变；新 resume execution
  原样继承 `ExecutionKey`、`ExpectedEffectScope`、sealed idempotency、`AdmissibilitySnapshotID`、`AcquisitionBinding` 与 `ReplayProof`，
  清空 receipt ref/hash 后以 fence `N+1` 和 `governance_granted` marker 进入 claimed。completed sibling 必须在 source
  `CompletedOperationIDs` 中且为 `evidence_committed`，其 key/fence/receipt/authority 完整保留并禁止重执行；pending
  sibling 必须为 `sealed`，source candidate（包括 target/parameters/dependencies）保持不变，只生成新的 dispatch-scoped
  `ExecutionKey`，并继承 effect scope/catalog/acquisition/replay authority。source execution 与 binding 不匹配或
  execution/replay authority 自身不一致必须 fail-closed，禁止从 current facts/catalog 重建或替换 authority。
  首次 owner call 前必须以独立 CAS 把 claim marker 推进为 `governance_effect_started`；该 marker 只表示 owner call
  可能已尝试，CAS 失败时 owner call 必须为零。owner error/response loss 后先 query exact receipt；只有 receipt 缺失且
  具备 sealed replay authority 时，任一再次 physical owner call 才须由独立 SM-4 event 先消费一次 operation-recovery
  counter，并携带 exact OperationID/persisted attempt，否则进入 `outcome_unknown`。typed grant 必须绑定唯一
  ticket/approval、resolution、dependency、subject、requested action、authorization hash 与治理 receipt hash；
  各 concrete owner 在 receipt preflight 时验证 grant，禁止复用 `Receipt(K,N)=governance_required` 作为普通 evidence。
- receipt query 对 persisted governance grant 执行严格 presence parity：调用方省略已持久化 grant、无 binding 时额外附加 grant、或任一字段
  漂移均 conflict，并在 capability side effect 前返回。
- File/Git/MCP/Shell/SSH/Web execute Result 只返回 owner-persisted `RuntimeOperationReceipt`；Runtime EventID、
  operation/evidence identity 与 authority source ref 由 Orchestrator 从 sealed dispatch/operation contract 纯派生，
  capability owner 不回显或重建第二套 Runtime 身份。owner 可在 receipt evidence 内提供内容寻址的 opaque
  source/raw ref 与 hash 作为 factual payload locator，但它们不是 Runtime EventID 或第二份 evidence authority。
- operation-scoped governance candidate 必须保留 suspended binding 的 dependency kind/subject/authorization target；
  不得把 persisted `approval/configuration/external` 退化为默认 `decision`，否则 ticket/approval 无法回绑 source operation。
- `runtime_governance` resolution committed 后，Orchestrator 先持久化 governance evidence 并发布 immutable
  continuation/terminal fact；Run owner 决定自身 blocked→active 或 terminal，application resolved watcher 不得发送 Resume。
- 相同 governance resolution 的 duplicate commit 只能重试幂等 fact delivery；Run owner reducer 与 singleflight gate
  分别合并 lifecycle input 与 drive，避免“aggregate 已提交但 Run 仍 blocked”的单边状态。
- provider transport、operation recovery、CAS reload、terminal delivery 使用独立 persisted recovery policy/ledger；candidate attempt 0/1 与 per-focus repair budget 不得消费这些预算。CAS ledger 耗尽只停止其计数增长，不能将业务 Run 终态化或使 active aggregate 静止；SM-4 保留带退避的 owner-local reload/re-drive，直到物理竞争消失或 fresh-process inventory 接管。
- startup recovery 必须通过 Run owner concrete typed inventory 获取 SM-1 state、RuntimeStateRecord presence 与 opaque
  revision/ref，不得使用通用 Run List，也不得要求 Run owner解释 dispatch/governance/delivery ledger。Orchestrator 随后
  通过 typed record query load aggregate，并在 owner-local SM-4 中分类 `active_no_dispatch`、
  `active_sealed_dispatch`、`blocked_governance`、`terminal_delivery_pending`；`active + RuntimeBootstrap + no
  RuntimeStateRecord` 进入同一初始化链。
  SM-4 在 Orchestrator `Run` 中同步完成全部 inventory/load 分页；每次请求最多 100 项，单次启动最多 1,000 页、
  100,000 项，并对所有已见非空 cursor 建立 visited set。任意 cursor 重复（包括 `A → B → A`）、页数/条目越界、
  缺 subscriber、owner error、nil/wrong result 都必须使启动失败，不能继续空转。Run 在 Setup 注册 inventory、
  opaque state-record 与 readiness query；Orchestrator 仅将
  已加载且已分类的 startup action 暂存为自身 process-lifecycle handoff，直到 Run owner 发布
  `TopicRunRuntimeCommandsReady` 后才以 run-scoped singleflight 交给 `BackgroundRoutine`。该 handoff 在每次
  启动重新由持久化 inventory 构建，不是第二套 persisted recovery/state machine，也不得依赖 `Weight()` 或调用普通
  `/run/command/get`。
- typed record load 使用严格 current JSON 与 aggregate validation；任何 unknown/retired field、旧 layout 或缺失 owner-fact
  convergence 都使 startup/load 直接失败。SM-4 不发送 schema fact、不提交 drive failure aggregate、不请求 LLM/operation、
  不创建 governance ticket，也不让 projection 将该错误转成 Run disposition。
- Run owner opaque aggregate update 返回 `accepted`、`aggregate_revision_mismatch`、`semantic_rejection` 或
  `owner_transient` concrete result。仅 `aggregate_revision_mismatch` 的实际 revision mismatch 可构造
  `RuntimeCASConflictError`；Orchestrator 禁止用泛化 `errors.Is(err, common.ErrConflict)` 推断 CAS retry。
- definition revision、Goal/DoD identity、正文或 capability contract 任一漂移都必须返回 conflict；definition context revision 必须与 facts 和 authority transaction 一致。
- LLM dispatch 前必须先完成 manifest、target snapshot、failure target、capability、validation scratch 和 provider ceiling preflight；缺失资源只能进入 canonical `need_prerequisite`，不得混入 LLM state-changing requiredOutput。
- block 输出只是 candidate；evidence quality、coverage binding、next action 和 accepted Result 只能由 Orchestrator 判定。
- assessment-only focus 不允许 executable operation；没有 current-run accepted evidence 的 satisfied assessment 必须被拒绝，不能用 summary、历史 memory 或自造 ref 关闭 coverage。
- ToolCall、Patch、ValidationResult、EvidenceRecord、CoverageWorkItem 和 ArtifactContinuation 只属于 Orchestrator 的单一收敛状态机，不对外暴露写入 repository。
- 跨组件交互只能使用 EventHub 与具体 command/data/result；禁止跨 owner service/repository、Envelope、JSON/reflect 类型桥和通用投递包装。
- provider-facing LLM cache prefix 必须遵守 C0、C1、C2、C3-C5 稳定分层。Skill 只以 Run owner 持久化的 typed
  provenance、bounded instruction/metadata snapshot 和 sealed budget 进入 volatile C3；Orchestrator 不查询 Skill
  Registry/SkillHub、不在 drive 时重新选择或裁剪，也不把 Skill 内容加入 C0～C2。
- Soul 同样只来自 Run owner 的 activation snapshot：semantic-plan 在 C3 使用 planning guidance，operation candidate
  在 C3 使用当前 execution/validation/decision guidance 及已封存的 stage-filtered Skill/MCP context。Soul 不进入 C0～C2，
  不改变 sealed manifest，也不替代 capability owner 的 effect/risk/approval authority。

## Document Conformance 验证要求

- provider protocol `Feedback` 与 coherent empty/invalid `DocumentOutcome` 必须分别覆盖 stream/non-stream、scope/identity、
  exact disposition/code/signature、submitted settlement、usage 与 raw-content isolation；任何 adapter/SM-4 reclassification 失败。
- empty/invalid、strict local schema 与 semantic rejection 的全组合必须证明共享 attempt 0/1：attempt 0 最多持久化一个
  directive，attempt 1 任意 rejection final，零 attempt 2，且 document correction不消费 transport/whole-document/repair budget。
- duplicate/stale result、CAS-before/CAS-after response loss、fresh-process reload、普通 Resume 与 delayed SM-4 drive 必须复用
  exact rejection/directive/request identity；不得重复 physical call、attempt、usage/token settlement 或 budget consumption。
- JSON Schema capability on/off 必须得到相同 local admission；operations wire 必须拒绝 ID/dependencies，相同 admitted proposal
  在 restart 前后必须派生相同 operation identity/graph，ambiguous/duplicate slot 零 owner call、seal、dispatch。
- aggregate、C4、eventstream、projection 与 memory 必须证明不含 raw provider document/reasoning/decoder message；EventHub
  只允许 coherent complete document 通过同步 typed `DocumentOutcome.Content` 的 `json:"-"` nested carrier 到达
  Orchestrator，其余 raw 一律禁止。owner-private audit carrier 只以 bounded hash/bytes/ref 对外关联。

## 已覆盖回归

- [x] create-only 目标重开与 revision-bound existing-target replacement；已有目标不能重复走 `file/write` create。
- [x] effect/receipt/evidence CAS、owner response loss 与 Result → Workspace → Run 三段 delivery ack 前后崩溃恢复。
- [x] live/recovery outcome-first receipt judgment、rejected owner diagnostic 保真、accepted/no-op effect/quality gate。
- [x] provider transport、operation recovery、CAS reload、governance wait 与 terminal delivery 的独立预算/guard。
- [x] execution-budget allocation-before-effect、provider/operation consumption、unknown reconciliation，以及SM-2 repair/
  SM-4 operation-recovery source-ledger的只读统一投影和duplicate/restart不重复消费。
- [x] provider-ceiling bounded recoverable wait的open/due claim/transient defer/exact satisfaction/expire/exhaust/retire、
  stale lease、fresh-process recovery与projection隔离。
- [x] runtime Definition clarification gap/dispatch/Decision ticket与Run replacement resolution的exact binding、duplicate和stale gate。
- [x] Definition clarification/Decision/Approval wait与continuation的统一 lifecycle fence、Run-owner exact acknowledgement、
  bind-CAS前后、response-loss、fresh reload，以及非终态 delivery零 terminal-budget消费。
- [x] startup inventory 多页成功、任意 cursor cycle、页数/条目上限、nil/wrong result、invalid current persistence，以及
  commands-ready 首次/重复 handoff。
- [x] forbidden production surface、generic effect/prerequisite compiler 的 capability-neutrality，以及 Run/Workspace
  snapshot reader 只读表面的静态/反射边界。
- [x] Workspace two-phase retirement、Runtime/evidence shared barrier、durable quiescence token 与 owner-local evidence
  purge receipt；purge effect 只由 owner-private dispatcher 推进，pending-before-delete journal 在物理删除/aggregate CAS
  crash cut 后 exact replay 首次 receipt；Orchestrator 不暴露逐 Run purge，也不跨 Run/Memory owner 组合删除，
  duplicate/restart 复用同一 durable deletion identity，retain/purge 后均无法复生。
- [x] Skill activation 的 typed provenance/budget invariant，以及 Orchestrator/LLM 不导入 Skill owner、repository 或
  EventHub topic 的静态边界。
- [x] 全量 owner-private fact 的 SM-2/SM-3/SM-4 分类、唯一 payload route、malformed pre-CAS gate，以及单 aggregate/
  mutation kernel 边界；Orchestrator 不拥有 SM-1 transition，也不存在跨 owner state setter。
- [x] Runtime EventHub reducer-owner/role 分类、LLM/capability participant 与 Run opaque-persistence 边界，以及 candidate
  request content carrier 的 private transition、duplicate/stale identity 和 assembled-before-metadata restart recovery。
- [x] 64 CWI 的依赖闭包按 provider order 分成 16 个四项 batch，current focus fence 与后续公平 drain 不按 ID 或
  Definition `Order` 重排；proof-batch provider projection 只携带相关 coverage，C0～C5 stable prefix/context 分层保持不变。
- [x] owner fact 的 missing、stale、duplicate、out-of-order、rollback/reappearance（ABA）、新 generation 与无关
  prerequisite 组合矩阵；semantic manifest hash 回滚重现仍由新 logical request 隔离。
- [x] prospective owner-fact composite probe 的 1～8 target-set File hypothetical delta + Shell dependency catalog/observation exact binding、
  dependency catalog provider 隔离、CAS/seal 双 freshness gate、nested typed stale bounded refresh 且零 correction 消耗，
  required-owner provider-safe typed `all_of` clause contract 的 acquisition/manifest/C3 seal、schema/admission pre-probe
  对 singular/partial/extra/duplicate target set 的 rejection，完整 binding 向每个 exact singleton `TargetRef` required-operation
  dependency chain 的编译，以及 persisted full-composite `would_not_satisfy` 的
  `prospective_satisfaction_contract_violated -> semantic_contract_failed` 矩阵；验证普通 document attempt-1 只在 C4
  引用违规 field、不复制 constraint，duplicate/restart 零重复 correction/terminal/probe，所有分支零 probe-time
  effect/ticket/governance/coverage。
- [x] multi-target prospective all-of receipt-set 的 1..N cut-point、同 dispatch/binding/evidence-kind/execution-key/fence/
  singleton-target exact match、partial accepted 零 settlement、最终 member exactly-once collective completion，以及
  rejected/stale/weak/mismatched/conflicting duplicate、exact duplicate、partial crash/restart 和跨 dispatch/generation union
  拒绝矩阵；aggregate invariant 拒绝 incomplete receipt set 已将 CWI 标记为 `satisfied/settled`，assessment-required 路径仍
  必须通过独立 SM-2 assessment gate。
- [x] validation discovery 回到 typed prerequisite 且局部 preflight 保留完整 pending graph；同 owner-target durable
  overlap 在 seal 前拒绝，Shell validation entry 不被提升为 failure target 且 Go diagnostic 只落入 manifest scope。
- [x] `BenchmarkRuntimeAggregateInvariantValidation64CWI`、`BenchmarkRuntimeLongFocusQueueProjection64CWI`、
  `BenchmarkRuntimeSemanticManifestRendering64CWI` 与 `BenchmarkRuntimeDriveLoopReduction64CWI` 均报告 allocation/CPU。

## 收口结论

当前没有与本实现合同对应的可执行待完善项。真实 provider、远端 endpoint、外部网络抖动等环境事实继续作为 deployed
verification 执行，不改变 Runtime 的 owner、状态机与 fail-closed code contract。
