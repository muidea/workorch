# 运行时编排组件

Status: active

Type: kernel-module-implementation-contract

Last Verified: 2026-08-17

## 功能范围与边界

`kernel/orchestrator` 是 Runtime convergence authority，负责 typed facts、authority transaction、prerequisite、candidate、sealed dispatch、operation evidence、governance wait、sealed terminal 与 SM-4 delivery acknowledgement。它组合多个 owner 和技术 block 完成同一 Run 的确定性收敛闭环，**但不拥有 Run、Workspace 或 Result lifecycle**。

SM-2 只以 `ConvergenceState` 作为 canonical decision；action/output kind 是纯派生字段，不进入 aggregate、
authority transaction、terminal delivery 或 Run notification 的最终合同。所有外部 feedback 必须同时携带
stable factual category 与完整、安全 original diagnostic；Orchestrator 只读取前者选择 internal transition，
不得解析原 message/code。

## Provider inference selection / ceiling

- activation 持久化 Run owner封存的 `RuntimeActivationContext.ProviderSelection`
- provider ceiling prerequisite 请求与封存完整 `ProviderCeiling{Selection, token/byte ceilings, revisions}`
- candidate 强制使用 ceiling selection；Resume 不携带或重读 selection patch，Restart 由 fresh activation读取 Workspace owner最新 canonical selection
- reasoning effort参与 activation/ceiling identity，但不改变 stable prefix、CWI、focus、预算、retry或 action
- Orchestrator 只校验 identity/revision/ceiling 完整性，不解释 ai-proxy、模型品牌或 effort语义

### Current development baseline 与 provider output contract

当前 Runtime schema 只维护一个无版本号的开发基线。开发调试阶段不维护历史 schema migration 或多版本 reducer；不兼容变化直接更新
当前代码/fixture baseline，并要求清理既有运行数据。activation/execution contract、config、runtime policy 与 stable-prefix hash 不携带
semantic-plan/candidate stage token limit。LLM owner 是 model capability 的唯一解释者；Orchestrator 只消费针对当前
workspace/run/authority、sealed selection 及 capability/configuration revision 的 detached persisted
`ProviderCeiling`，不得按 model name 分支或应用 token override。

semantic-plan 与 operations 的 `RuntimeExpectedDocumentContract.MaxOutputTokens` 只能从
`ProviderCeiling.EffectiveMaxOutputTokens` 创建，并在 provider call 前随 request session 持久化。ExpectedDocument、
LLM EventHub command、provider request、provider-facing `max_output_tokens` 与 audit 必须精确等值；LLM boundary 在任何
不等值时拒绝提交，physical call count 保持为零。SM-4 whole-document recovery 复用 sealed ExpectedDocument，不提高预算、
不重读配置、不拼接 partial output。

两个 request renderer 都要求恰好一个完整 compact JSON object，无 prose、fence、prefix/suffix 或 authority 重复。
sealed manifest 同时约束 prompt、closed JSON Schema、strict decoder 与 evaluator；object 禁止 unknown field，数组、字符串、
target、binding 与 content/effect shape 均有界。LLM owner只返回 provider factual outcome，不能选择 retry、terminal、
CandidateAttempt 或 Runtime transition。

activation-only `DefinitionFocuses` 与 SM-2 generation-specific focus/binding queue 分开持久化。semantic document
admission、feasibility merge 与 seal 只读前者的 immutable declaration；receipt/resource-change 触发的 binding retirement、CWI
reopen 与 reseal 只修改后者，并继续经过 Orchestrator private internal event、controller 与 transition coordinator 的单次 CAS。
focus/binding identity 必须同时绑定该 generation 的 `manifestHash + logicalRequestID`；同 manifest 下的新 logical request
不得复用旧 focus/binding。`CurrentBindingRef` 不得指向 reopen/retirement lineage 中的历史 binding，reducer 在写入前与
aggregate validator 在持久化后均 fail-closed。capability owner 只报告资源事实和当前 admissibility，不能请求 CWI/focus 状态。

非当前 aggregate 不原地迁移。Resume/SM-4 load 返回 `fresh_activation_required` 且不调用 LLM/capability owner；
针对该不兼容 predecessor 的显式 Restart 由 Run owner 返回 `data_reset_required`，不创建 replacement intent，也不要求
Orchestrator 加载旧 aggregate。current-baseline Restart 的 predecessor/replacement/successor 则完全由 Run owner 的
`ReplacementIntent` 与 owner-private reducer 管理，Orchestrator 只处理已封存的 administrative terminal delivery。

当前 baseline 为每个 authority 封存不含 request/lease/CAS bookkeeping 的 `AuthorityInputID`。相同 Definition、focus/binding、
evidence、capability/catalog、freshness 与 owner observation lineage 不能创建第二个 AuthoritySnapshot；精确 owner binding
contradiction 只通过 `semantic_binding_contradiction_observed` 私有事件退休，无法唯一绑定则 typed fail closed。

## Runtime 四状态机实现约束

完整规范以 `docs/00-core/runtime-stage-contract.md` 的“Runtime 四状态机固化规约”为唯一权威。本模块按以下四个状态机落实现有职责，不定义第二份状态语义：

| 状态机 | 本模块职责 | 外部 owner |
| --- | --- | --- |
| SM-1 Run lifecycle | 不拥有；只消费 Run state notification 触发自身 SM-4 drive | Run owner 持久化 SM-1 state/lineage，并为 Orchestrator aggregate 提供 opaque RuntimeStateRecord CAS；Workspace owner 持久化 activeRun |
| SM-2 authority/focus | reducer、focus/evidence/assessment/repair/governance/terminal 裁决；编译 sealed candidate manifest | application 只编译 typed bootstrap；owner 只返回 facts；Result 只存 outcome |
| SM-3 candidate/dispatch | request session、单 evaluator、attempt 0/1、transaction、seal、typed operation、evidence commit | LLM 返回 candidate；capability block 执行 operation |
| SM-4 drive/recovery/delivery ledger | bounded drive、singleflight、restart load、delivery acknowledgement/retry；不写 owner state | Result/Workspace/Run owner 各自完成资源迁移；projection store/read model 只保存派生状态 |

实现必须保持：

- SM-1 不选择 action，SM-2 不执行 operation，SM-3 不选择 next action，SM-4 不重建 authority。
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
  package-private compatibility adapter 只复用同一 dispatcher，不能形成额外 mutation entry；外部仍只能使用
  `ApplyInternalEvent`、`Load`、`QueryOperationReceipt`。
- `RuntimeAggregate.Validate` 仍是一次完整、顺序固定且 fail-fast 的 aggregate validation；activation、SM-2、SM-3、
  SM-4 与 cross-machine fence 仅以 private validator 分段。evidence commit 的 receipt admission、fact derivation、
  exact-CWI credit 与 completion 共用一次 transition commit；prerequisite kind handler 为纯 aggregate 变换；active
  dispatch 的 preflight、claim/recovery、owner call 与 receipt projection 仍按原顺序运行，并共享唯一 CAS 与 recovery
  budget handoff。
- 所有跨状态机推进都使用 persisted aggregate + typed EventHub command/result。
- projection failure 不得覆盖 reducer terminal；重启不得重复 prerequisite/candidate/side effect。
- 任何修改 reducer/controller/drive/operation projection 前，必须说明影响的 SM 编号并补合法/禁止迁移、幂等、restart、stale/duplicate、budget/guard 测试。
- 本模块不得恢复 禁止的 mainline、application continuation queue、通用 RuntimeAdapter registry、projection authority 或自动 successor。

## Candidate 合同

完整语义以 `docs/00-core/runtime-stage-contract.md` 的“规范化候选合同”为准。历史收口编号、旧 schema 记录和
deployed run ID 只属于 archive regression evidence，不定义当前候选、reconciliation 或 restart 语义。

- SM-2 先从 persisted Definition seed、template/profile policy 与 provider request 前通过 typed prerequisite
  获取的 owner capability/admissibility catalog 编译唯一 semantic-plan manifest；manifest/hash/schema/evaluator
  同时封存 owner snapshot/revision、operation kind、target/selector/constraint 与 recipe vocabulary，禁止 LLM 猜测
  未观察的合同值。声明了 managed-validation recipe 时，SM-2 必须先取得 File owner 的 manifest/observation，再把
  immutable binding 交给 Shell owner；Shell catalog 对每个声明 recipe 发布 `ready | prerequisite_required | unavailable`
  factual surface，并依赖 exact File workspace-tree revision；只有对当前 manifest 适用且静态可执行的 recipe 才进入
  confirmed set。没有 confirmed recipe 的 `validation/managed_session` 不进入 provider-visible allowed kinds，
  Orchestrator 不检查文件名、不判断 recipe 适用性。MCP 等动态 owner 同时发布 callable capability 与 typed
  `RuntimeValidationSurface`；只有带 owner-sealed input binding、non-durable effect、safe-to-replay class，并由 owner
  显式声明为 `direct` 的 `ready` surface 可进入 `ValidationRecipeRefs`。`auxiliary`/`not_declared`、
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
- renderer、wire schema、strict decoder、candidate evaluator、owner feasibility command 与 fixture 只消费该
  manifest 及其 hash，不得从 profile、focus、raw reason 或临时 inventory 重建第二份允许集。
- complete document 只调用一个纯 `CandidateEvaluator`，并按 manifest identity → wire/field →
  kind/variant/parameter → target → binding → effect/dependency → replay 的固定顺序返回 `accepted` 或完整
  `CandidateRejection`。owner feasibility 是 capability owner 的独立 typed factual observation，不得伪装为
  candidate rejection。
- 当前唯一结构 evaluator 是 `biz/internal/candidateadmission.Evaluate`；`biz/runtime/candidate_admission.go`
  只投影当前 authority/attempt/evidence lineage、调用该 evaluator 一次，并在返回后执行 SM-3 owner-local
  rejection seal。repair closure、validation-only gap 与 owner-grounded target scope 都在 replay 前由同一 evaluator
  判定；`biz/internal/grounding` 提供 target-selection/repair-target 的唯一纯比较实现。focused package 不读取
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
- rejection 只保存 evaluator 产生的完整、安全原始 diagnostic，不携带 correction、retry、terminal 或
  Runtime action。Orchestrator 将其提交为 SM-3 internal event 后，才根据 persisted manifest、attempt ledger、
  rejected shape 与 field violations 形成 `correctable_once` 或 `rejected_final` owner-local result；前者必须有
  可仅用 manifest + rejection 验证的 `CorrectionDirective`，后者零 correction call。无法归类是
  `runtime_contract_defect`，不能退化为无信息 governance ticket。
- semantic-plan 的 `CorrectionDirective` 必须持久化 exact dependency edge、actual position、manifest/seed/prefix、
  plan step 与 source rejection hash；LLM command 只取得 detached fact 并仅在 C4 渲染，不能从 `OriginalMessage`
  重建。correction lineage 只属于一个 sealed provider document；coverage-order attempt 1 即使 accepted，创建新的
  proof-batch request 时也必须在同一 SM-3 private CAS 中重置 attempt、directive 与 current rejection lineage。
- candidate pipeline 只接收 `request_recoverable`、`candidate_rejection`、`owner_feasibility` 和 `accepted`
  factual result。attempt 0/1 与 correction policy 只由 SM-3 reducer 消费 persisted diagnostic；owner feasibility
  必须先成为 Orchestrator internal event，再由 SM-2 coordinator 选择 prerequisite、新 authority 或 typed
  terminal，零 correction attempt。SM-4 只从 persisted result 恢复，不再解释 raw failure。
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
- LLM owner 的 `RequestRuntimeCandidateResult` 只返回 provider-request domain 的 `RuntimeFeedback`，稳定类别仅允许
  authentication/configuration/transient/protocol；它不得跨 EventHub 返回 `RuntimeCandidateFailure` 或完整文档的
  schema/contract 裁决。Orchestrator adapter 必须先穷举校验 scope、authority、owner、category/class 与
  typed document disposition/content/feedback/stale 互斥关系，再在 SM-3 owner-private 边界重建内部 failure fact。
  `incomplete_document` 只提交 SM-4 owner-private request-recovery fact；下一 physical call 必须从头请求同一 sealed
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

## CWI semantic planning / reconciliation owner contract

- Activation 必须封存 Definition seed；fresh restart 缺 workspace-specific exact proof 时进入 persisted
  `semantic_contract_required`，不得由 application 终止或要求普通用户补 `contextHints/evidencePolicies` JSON。
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



## Effect / Receipt / Repair Directive

- `RuntimeEffectScope` 是 operation 的中立 effect 合同；`ResolveExpectedEffectScope` 由 descriptor + shell `stateChanging` + sealed MCP risk/EffectScope 解析 concrete scope。
- seal 前 `ValidateCandidateEffectCompatibility` 强制：canonical `need_state_change` 至少一个 durable effect；`need_validation` 不得含 durable/unknown；`stateChanging=false` shell 与 read-only/session MCP 不能关闭 durable state-change。
- `SealedDispatch.OperationExecutions` 在 seal 时写入 per-operation `ExecutionKey` + `ExpectedEffectScope`；execute 前 claim fence，owner 返回后 `owner_committed`，evidence CAS 后 `evidence_committed`；owner error 记 `outcome_unknown`，禁止盲重放。
- owner 已持久化 exact `ExecutionKey + Fence` receipt 但响应丢失时，executor 必须先重新加载 aggregate：存在 terminal receipt 则继续 governance/evidence commit，不得把 `owner_committed` 覆盖为 `outcome_unknown`。
- claimed/no-receipt recovery 只允许 sealed idempotency=`safe_to_replay|owner_managed`，并先消耗独立 operation-recovery budget；`at_most_once|unknown` 一律转 `outcome_unknown`。每项 operation evidence 独立 CAS 持久化；只有最后一项成功或任一项 rejected/stale 时，才在该 evidence CAS 中原子 settle dispatch。
- current-schema evidence commit 必须携带已持久化 owner receipt，并按 sealed expectation + `ObservedEffectScope` +
  `BindingRef` 的 exact target/selector、recipe/check、metric/artifact 匹配联合裁决 Kind；declared Kind、candidate
  自报 `CoverageWorkItemIDs`、非空 `PassedChecks` 或 exit code 0 都不构成 coverage authority。
- multi-operation dispatch 中，同一 dispatch receipt 造成的 prerequisite/resource revision 变化属于内生变化；可以
  退休后续 SM-2 observation，但不能仅因此清除 `ActiveDispatch`。SM-3 必须继续尚未完成且 dependency 已满足的
  sealed operation，直到全部成功或任一 operation rejected/stale 后再 settle/reduce。
- state-changing Shell receipt 必须使用 owner 实际观察到的 changed refs/resource changes；真实变化越出 sealed
  `TargetRefs` 时以 `undeclared_workspace_effect` 拒绝，不能通过额外 touch 一个声明目标取得 durable evidence。
- `Facts.RepairDirective` 是同 focus 唯一 next-action 权威（INV-05）；`ReduceRuntimeStateWithDirective` 同步 directive；anti-replay evidence 不得覆盖 directive action。不存在从 failure message 推导的 repair hint 第二权威。
- repair directive 的创建、切换和清除均属于 Orchestrator SM-2 reducer；`internal/pkg/workorchruntimecontract` 仅保存 directive DTO、校验和纯计算。`need_prerequisite` / governance wait 是同一 repair cycle 的临时 interlude，必须保留 directive；只有 accepted repair evidence、focus advance 或 terminal 才能切换或清除它。
- LLM assessment 的 disposition、`MissingEvidenceKinds`、target refs、summary、items 和 reason 都是完整保留的
  observation/original diagnostic，不具备选择 Runtime action 的权力。SM-2 只可从 persisted coverage、receipt/evidence
  quality、focus policy、owner-grounded repair surface 与 freshness 分类，并在同一 owner-local transition 中写入唯一
  `Facts.RepairDirective.RequiredState`。不得从 observation 的自由文本或其 claimed missing kind 直接进入
  `need_validation`、`need_state_change`、governance、retry 或 terminal。
- validation entry 不是 state-change repair target；只有 focus exact target、仍新鲜的 owner-grounded failure target 或
  sealed owner selector/prospective constraint可以形成 repair authority。validation/evidence-quality gap保持
  `need_validation`，不得在 validation-only 重试后退化为无关 `need_state_change`；耗尽封存 validation
  recipe/budget 时必须形成 typed terminal。
- assessment gate 只消费 current-run accepted evidence、sealed binding/effect与 owner structured validation
  observation；只有 receipt 实际报告 binding 要求的 exact check/metric/artifact 才能给对应 CWI credit。非空
  `PassedChecks`、claim signal、assessment 文本、Goal/DoD、summary 与 operation/check name 的自由文本相似度
  都不能作为覆盖裁决；profile 封存 recipe 只约束候选选择该命令时的必要参数。
- profile-declared `RuntimeOwnerFactRequirement` 是 immutable focus postcondition，不是 capability target state。
  Workorchd 只编译声明，Shell/MCP 等 owner 只通过 current typed admissibility catalog 发布事实；SM-2 assessment gate
  在 sealed binding 的其余 proof 全部成立后，才检查指定 owner/operation 是否存在匹配的 `ready + direct` selectable
  validation surface。assessment claim、成功 operation、旧 catalog 或历史 recipe 均不能替代该 current owner fact。
- owner fact 是唯一缺口时，assessment commit 在同一 Orchestrator CAS 中提交 dispatch/audit、把 current bound CWI
  转为 `reopened`、清除 current binding、更新 reconciliation，并以 `assessment_owner_fact_postcondition_unmet` lineage
  退休当前 semantic generation；SM-4 只从该 aggregate 幂等 re-drive。若 sealed proof 本身仍缺失，则保持普通
  evidence/repair 主线，不得误触发 semantic replan。同 CWI owner-fact replan 由 `MaxRepairAttemptsPerFocus` 有界，耗尽
  后 deterministic `owner_fact_convergence_exhausted`；duplicate/restart 不产生第二次 reopen/CAS/request/effect。CWI 重开后
  固定回到 `semantic-pending/<cwi>`；若该 current-focus prerequisite ID 已由上一 semantic generation 退休，SM-2 必须在
  重新排入 acquisition 前原子移出同 ID 的 retired prerequisite 及其旧 observation，只保留无关 retired lineage。旧
  observation 不得复用为当前 authority，审计继续由 CWI reopen lineage 与 semantic generation history 承载，且
  active/retired 不相交 invariant 不得放宽。导致该 owner-fact gap 的 accepted state-change operation shape 是同一 CWI
  的防重放事实：SM-2 只把 shape 携带到 successor focus，repair attempt/gap/directive 仍按 focus 清零；SM-3 必须拒绝
  successor candidate 重放该 shape。新 generation 必须使用新 logical request 派生 focus/binding，历史 reopened binding
  绝不能重新成为 current。
- File owner 对 `unified_diff` 在 owner feasibility probe 中执行无写入 dry-run；hunk/context 冲突必须在 dispatch seal 前以 typed infeasible result 返回，owner 不选择后续 RuntimeAction。
- active Runtime Cancel/Supersede 必须先 `RequestTermination` CAS 持久化 `RuntimeTerminationRecord`，再投影 SM-1 terminal。
- drive failure 经 `classifyRuntimeDriveFailure` 分类：只有携带 expected/observed revision 的 `RuntimeCASConflictError` 才进入 CAS reload；owner-transient/projection 不终态化业务 Run，语义 conflict 不得消耗 CAS budget。

## 组件交互及合同

- 通过 EventHub 读取 `blocks/workspace` 和 `blocks/run` 的权威快照，并只通过 Run owner CAS 合同持久化
  Runtime aggregate；不通过任何 Run owner command 指定 lifecycle target state。
- Runtime 通过 File、Git、Shell、MCP、Web 和 SSH owner 各自 `pkg/events` 定义的 concrete execute command/result 调用能力 block；同步返回的 typed result 是提交 execution evidence 的唯一执行回执。
- 通过 EventHub 调用 Decision、Approval 和 Memory owner 自身 `pkg/events` 定义的具体 command；Runtime governance ticket 的 open request 同样由 Decision owner 定义。terminal 后只发布 immutable delivery fact，
  Result/Workspace/Run owner 各自消费并产生 acknowledgement。不得直接持有这些 owner 的 service 或 repository。
- Runtime terminal Result 只通过 Result owner 的 typed SM-4 delivery command 写入；Orchestrator 不构造或提交完整
  Result/Artifact entity。Orchestrator 的 Result adapter 只为 owner-local terminal cleanup 提供 EventHub read/delete，
  并对 owner query result 再次 detach，不伪装成具备 generic create/update 的 repository facade。
- 与 `kernel/governance` 协作处理治理策略、decision/approval blocker 和恢复，但 Orchestrator 保持 runtime contract gate 权威。
- Decision 与 Approval 使用各自 `pkg/events` 定义 evidence command；Orchestrator 幂等写入自己的 EvidenceRecord 并返回具体 result。
- Orchestrator 只在 `kernel/orchestrator/pkg/events` 定义由自身接收的 Runtime drive、receipt commit/query、governance resolution 与只读查询合同；投递给其他 owner 的 command/result 由接收 owner 定义。
- File target snapshot、target selection、workspace manifest 与 repair-surface confirmation command/result 由 File owner 在 `blocks/file/pkg/events` 定义；前三类 acquisition Result 必须回显 workspace/run/authority scope，prerequisite identity 只由 payload/必需 freshness observation 表达。Git capability-state 与 LLM provider-ceiling acquisition 使用相同 scoped Result 规则。Decision、File、Git、LLM、MCP、Shell、SSH、Web 各自在自身 `pkg/events` 定义并消费 Runtime capability acquisition；其 Result 只返回一份带 workspace/run/authority scope 的 canonical acquisition，prerequisite identity 只由必需 freshness observation 承载。File、Git、MCP、Shell、SSH、Web 的 operation admissibility acquisition 采用相同 scoped canonical fact，prerequisite identity 只存在于 inventory/必需 observation，不再由 Result 外层重复回显；六个 owner 同时拥有 operation feasibility 与 semantic-proof feasibility topic/Command/Result，File、Git、MCP、Shell 各自拥有 CWI reconciliation query。Orchestrator 仅按 canonical owner 分派具体合同并归一 owner-neutral observation。
- `kernel/orchestrator/pkg/events` 只保存 typed command/result、receipt DTO 与确定性校验/构造；能力 Block 的 Biz 负责发送 receipt query/commit command，Orchestrator Biz 负责消费并持久化，任何 `pkg` 不得持有 Hub、执行 I/O 或推进状态。
- Provider ceiling、candidate preflight 与 candidate request command/result 由 LLM owner 在 `blocks/llm/pkg/events` 定义；Orchestrator 的 SM-3 request session 封存完整 authority transaction，跨 Orchestrator → LLM → provider 的一次物理请求只携带 canonical `RuntimeCandidateRequestIdentity`，EventHub command/result 与 LLM request/response model 不得再次平铺 logical/sequence/physical 三字段。Orchestrator private session/internal event 可按 owner-local ledger 结构保存其状态字段，不构成跨 owner wire authority。LLM block 返回的 document 必须绑定同一 snapshot 和 request identity，transaction/identity 漂移不得进入执行。
- LLM owner-private candidate response 不复制 sealed provider audit/typed error 已拥有的 `RequestID/Status`；usage audit 从 canonical audit/error 读取，不能以 response mirror 覆盖 provider outcome。
- candidate request 携带从 persisted facts 和唯一 manifest 渲染的 typed `RuntimeCandidateContext`；当前 focus/evidence/failure 是 context，operation kind/parameter/target/effect/dependency 与 `BindingRef` 授权只来自 manifest。candidate 不得以 raw `coverageWorkItemIds` 自报 credit。C4 只投影 provider-safe failure/evidence/assessment/shape identity 与 attempt-1 的 typed rejection/directive；它不得携带 raw reason/summary/payload、owner feasibility 或 recovery/governance action。LLM block 不从 禁止的 map、raw reason 或临时上下文重建 authority。
- Runtime dispatch 在执行前持久化 `SealedDispatch`、完整 transaction、candidate hash、sealed binding references 和 typed step input；execution evidence 只有在 snapshot、dispatch、operation、binding 与 owner receipt 一致时才能提交并触发下一次 reducer。
- capability owner 的 validation receipt 必须持久化中立 quality/zero-work/check observation，并纳入 stable receipt hash；SM-3 在普通响应与 exact-fence restart recovery 两条路径执行同一质量裁决。receipt 只有实际匹配 sealed binding 的 target/effect/recipe/check（适用时 metric/artifact）才可令 SM-2 给 CWI credit；弱验证、非空 `PassedChecks` 或 owner exit code 0 只进入 rejected/diagnostic evidence。
- Runtime aggregate 通过 Run owner 的 EventHub update 合同持久化；Orchestrator 使用
  `ExpectedRuntimeStateRevision` 执行语义 CAS，不直接访问 Run repository，也不把 aggregate 编码进
  `map[string]any`。Run owner 对同一 aggregate revision 上的无关 Run model/lifecycle 写入执行物理
  CAS rebase；完整 Run model revision 不是 aggregate fence。
- Runtime 初始化时封存 typed Goal/DoD definition context；后续 candidate request 与重启恢复只能读取 Run owner 中持久化的定义快照，不能由调用方临时重建。
- `InitializeRuntimeFromRun` 只读取 Run owner 已持久化的 typed bootstrap，生成 initial facts 并执行 preflight/reducer；Orchestrator 不解析 禁止的 execution context map 来重建定义或 focus。
- Runtime controller 统一负责 initialize、prerequisite commit、candidate seal 和 evidence commit；服务重启只加载既有 aggregate，不得因重复 Start/Resume 再次执行 reducer 或增加 attempt。
- initialize/restart identity 只比较不可变 workspace/run、definition revision/content 与 activation；`Facts.Focus` 是 SM-2 persisted cursor，合法 focus advance 后不得被拿来与 bootstrap 首 focus 比较并误判 definition drift。
- prerequisite mainline 按 typed requirement 的 canonical owner module 调用 EventHub，并在每个结果提交后原子执行 reducer；profile capability-state owner 必须与 accepted operation descriptor owner 相同，不能使用 EventHub destination 别名；多个 capability owner 的结果必须合并，不能互相覆盖。
- capability prerequisite 的 owner/class/kind 不匹配或没有 responder 必须以 owner-neutral typed `RuntimeCapabilityUnsupportedError` 返回，并由 SM-4 分类为 `runtime_capability_unsupported` terminal contract defect；它不得退化成默认 capability、candidate rejection、泛化 decision/approval ticket 或 retry loop。各 capability owner 的支持 kind 只在其 `pkg/common.RuntimeCapabilityKinds()` 声明，handler 只消费该纯 inventory；具体 EventHub result 由接收 owner 定义，owner-neutral catalog observation 仅由 `workorchruntimecontract` 的纯 helper 构造。
- operation admissibility inventory 由 canonical owner 提供完整 bounded scope 或封闭 selector，SM-2 将其编译为三种 `TargetPolicy` 之一。既有资源与 prospective target 都必须在 seal 前由同一 owner 做只读 feasibility probe；Orchestrator 不扩展 selector，也不把 inventory/context 截断当成完整授权。
- semantic manifest、provider JSON schema 与纯 evaluator 必须消费同一条 owner-published
  `operation kind + concrete variant + target policy` fact。owner catalog 已列出的现有目标不得与只允许
  `prospective_owner_target` 的 create-only fact 组合；若同一 owner catalog 为该 evidence/target policy 发布了合法
  replacement kind/variant，attempt-1 correction 只携带该 sealed alternative（例如 `patch/apply/text_replace`），
  不解析 owner message，也不把 exact target 改写成虚构的 prospective target。目标内容 revision 仍由后续 owner
  snapshot/preflight 与 candidate manifest 绑定，LLM 不提供 revision authority。
- sealed candidate 通过接收 owner 自有 concrete execute command/result 按 operation kind 投递到 File、Git、Shell、MCP、Web 或 SSH owner；跨组件 concrete result 只包含具体 operation/transaction、唯一 receipt outcome、stable factual category、完整安全 original diagnostic 和 evidence，不保留 `Status + Receipt.Outcome` 双权威。
- 多 operation dispatch 按 dependency graph 确定性执行；每个 operation 独立提交 evidence，全部成功或任一失败后才结束 dispatch 并再次 reducer。
- RuntimeFacts 持久化当前 focus 与有序 remaining focus queue；只有当前 focus 每个 required CWI 都获得 exact binding-matched evidence 后，evidence commit 才能原子切换到下一 focus，再调用同一 reducer。
- 显式声明 `RequireAssessment` 的 focus 在执行证据齐备后进入 typed `need_assessment` authority；LLM 只能返回绑定当前 focus/coverage 的 `AssessmentCandidate`，Orchestrator 校验其 evidence refs 来自当前 Run 的 accepted evidence 后写入 assessment ledger。
- SM-2 在 assessment commit 前执行 item-level evidence gate：weak evidence 永不关闭 coverage；任何 executable/claim evidence 必须逐项匹配 sealed `CoverageEvidenceBinding` 与 owner receipt 的 exact proof。`PassedChecks` 非空和中立 claim signal 不能单独通过门禁。门禁失败写入 `assessment_evidence_quality_gap` repairable assessment，并由 reducer 在封存预算内继续同 focus repair。
- 最后一个 assessment 关闭或 typed terminal failure 形成后，Orchestrator 从 current-run evidence/coverage/assessment ledger
  密封 terminal 与 result payload，并创建 terminal delivery fact；Result owner 自行持久化 accepted/rejected outcome，
  Workspace owner 自行释放 active run，Run owner 自行进入 terminal。Result block 不参与 acceptance 裁决。Result、Workspace、
  Run 三段 SM-4 delivery 只携带同一 sealed `TerminalOutcome`；禁止另传 `FailureReason` 覆盖 code/summary，Run owner 仅在
  private reducer 中从 outcome 派生自身持久化展示原因。terminal closeout 只投影仍未满足 CWI 的 canonical
  assessment gap；已满足 CWI 的 producer claimed gap 仅留在 assessment audit，不得成为 MissingEvidence/TargetRefs
  或 top failure。失败摘要优先使用当前 `Facts.Failure`，其次为 current active/formal gap，最后才使用 terminal
  code/recovery lineage。
- CoverageFocus 可携带 evidence role、accepted/rejected step kinds 与 accepted operations；reducer 只在 owner 已确认的 capability kinds 中做中立交集/排除，并把结果封入 `RequiredOutputContract`。若声明 accepted operations，candidate 至少包含一个匹配的具体 operation 参数，不能用同 step kind 的其他动作替代。
- provider request session（logical request ID、expected-document identity、physical-call recovery generation、transport
  outcome）先于 candidate attempt 随 aggregate 持久化；未形成单次完整 candidate document 时不得创建/失败 attempt。
  EOF/output limit 只能重提同一 expected document，不能拼接 raw output。complete rejection 先保存原始 diagnostic，再由
  SM-3 owner-local reducer 决定 final 或唯一 same-step correction；只有 persisted `correctable_once` result 可在相同
  transaction、manifest 和 stable prefix 下开启 attempt 1，attempt 1 再次 complete rejection 形成
  `candidate_retry_exhausted`，不能创建第三次请求或无信息 governance ticket。
- candidate retry 预算按稳定且显式持久化的 `AuthorityContractID` 计算；identity 绑定 workspace/run、完整 `AuthoritySnapshotID`、focus、canonical convergence state、definition/evidence revision 与 requiredOutput hash。派生 action 不参与 identity。缺失或不匹配该字段属于 current-schema invariant violation；同一 contract 超过 attempt 0/1 必须 fail-fast，禁止再次请求 provider。
- complete schema/contract failure 只记录统一 evaluator 生成的原始 typed rejection；correction/final 由 SM-3 internal event 决定。owner feasibility 的
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
- governance candidate 作为 sealed dispatch 持久化；治理结果通过 `CommitRuntimeGovernanceResolutionCommand/Result` typed owner command 提交，同一 resolution ID 与 authority snapshot 最多消费一次。Result 强制绑定 workspace/run/resolution scope，并保留正交的 `committed | stale_diagnostic | duplicate` CAS 提交事实；它不携带自由文本 diagnostic/reason/failure，也不得由 convergence state 猜测提交事实。
- Decision owner 可通过 `CommitRuntimeGovernanceTicketResolutionCommand` 只提交 ticket resolution；Orchestrator 从 persisted binding 补齐 authority snapshot、dispatch 与 dependency 后进入同一 resolution commit。
- governed dispatch 通过 `OpenRuntimeGovernanceRequestCommand/Result` 向 Decision owner 创建确定性 ticket；ticket、snapshot、dispatch 与 dependency 的绑定随 Runtime aggregate 持久化。
- `DriveRuntimeCommand/Result` 是 Runtime 唯一同步推进合同；Orchestrator 在有界循环内依次处理 prerequisite、candidate、sealed dispatch、typed operation、evidence/reducer。Result 不保存第二个 disposition：`need_governance + BlockingTicketID` 表示 governed wait，`complete|failed + Terminal` 表示 typed terminal，其它组合均是合同缺陷。重复调用和进程重启必须从 Run owner aggregate 幂等恢复。
- drive 同时校验 persisted `StateRevision` 与 `RuntimeConvergenceProgressSignature`；signature 覆盖 semantic seed/request/attempt/document/owner batch/sealed binding、focus/evidence/canonical state/requiredOutput/repair/prerequisite/dispatch/governance/terminal，不包含派生 action、operation-candidate retry、snapshot attempt、SM-4 lease 或 revision bookkeeping。只增加 bookkeeping 的 transition 必须 fail-fast。
- SM-4 delayed recovery 只由 aggregate 中的 `RuntimeDriveLease` 授权。lease 绑定 workspace/run、aggregate revision、recovery generation、stable reason/signature、due time 与 `scheduled|running`；timer 只能 claim exact due lease，不能自行决定 retry。进程内 workspace/run singleflight 与 timer map 仅合并该 persisted identity；重复 Start/Resume/governance continuation、startup recovery 和旧 timer 均不得建立第二条链。crash 后遗留 `running` lease 由 startup 重新封存为新 generation 的 scheduled lease。
- 真实 `RuntimeCASConflictError` 是唯一进入 CAS reload ledger 的事实；semantic persistence rejection 不创建 CAS/retry lease。CAS ledger 耗尽停止计数增长但仍保留带退避的 persisted owner-local successor，避免 active aggregate CPU 空转或无后继静止。
- reducer/aggregate 已进入 governed wait 或 terminal 后，后续 owner projection 失败只能记录 projection error 并等待幂等重试，不得把已确定的 complete/fail authority 改写成 `runtime_drive_failed`。
- 普通 drive、governance resolution 与 drive-failure terminal 共用唯一 terminal delivery lane：先校验 persisted sealed aggregate，
  再发布 Result -> Workspace -> Run 的有界 delivery fact 序列，并仅在 SM-4 记录 acknowledgement；ack 只能取自
  该三个 owner 的同步 typed response 后转入 Orchestrator private internal event，绝不开放独立 EventHub ack command；
  任一 delivery 失败进入延迟重试，不得改写 reducer terminal 或越权写 owner state。
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
- application projection 消费 Orchestrator immutable drive observation；Eventstream 当前不直接订阅 Orchestrator topic，Runtime 只消费同步 typed operation result 推进状态。
- workspace/run purge 时，Orchestrator 只删除自身 ToolCall/Patch/ValidationResult/Evidence/Coverage/ArtifactContinuation/MemoryTrace；Step 与 RuntimeCheckpoint 必须通过带 WorkspaceID/RunID 的 Run owner purge command 删除，Orchestrator 不持有其 repository。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `blocks/run` | 双向 | SM-1 notification、recovery inventory/readiness、opaque Runtime record query/CAS、terminal/lifecycle fact | 完整；Run 不解释 SM-2/3/4 |
| Result / Workspace / Run | Orchestrator → owner | 同一 sealed `TerminalOutcome` 的三段 delivery/ack | 完整；各 owner 只做自身 reducer |
| `blocks/llm` | Orchestrator → LLM | provider ceiling、preflight、candidate request | 完整；LLM 只返回 factual result |
| File / Git / Shell / MCP / Web / SSH | 双向 | owner capability/admissibility/feasibility/reconciliation/execute 与 receipt query/commit | 完整；能力 owner 不选择下一 action |
| Decision / Approval / Governance | 双向 | governance open/resolution/evidence 与 immutable continuation/terminal fact | 完整；无 Application resume 旁路 |
| `blocks/memory` | Orchestrator → Memory | scoped recall/search/trace typed contract | 完整；Memory 只提供 owner fact |
| `application/workorchd` | Orchestrator → Application | immutable drive observation 与只读 Runtime view | 完整；Application 不推进 reducer |
| Skill / Soul / MCP Registry | 无直接 I/O | 只消费 Run owner 已持久化的 activation snapshot | 完整；Registry 更新不改变既有 Run |
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
- Runtime/Orchestrator 不自动创建 replan successor；current-baseline restart 的 replacement intent、terminal delivery
  后 successor materialization 与 lineage 都由 Run owner 管理。重新定义目标或执行基准必须使用新的 typed
  activation/bootstrap，不能由 Orchestrator 从旧 aggregate 派生。
- 不记录或恢复 `runtime_step_result_available`、watchdog recovery policy 等旧 checkpoint；历史 checkpoint 仅作为只读审计数据。
- Run/Workspace 状态通知不再隐式写入 禁止的 runtime checkpoint；状态事件与 Runtime aggregate 各自保持单一职责。
- HTTP runtime 配置更新不得向 Orchestrator 推送进程内 repair policy 副本，避免 canonical config 与 禁止的 policy cache 形成双权威。
- Orchestrator 不注册 ValidateRun/CompleteRun command；validation 只能提交 sealed operation evidence，completed terminal
  只能来自 reducer decision 与 Result/Workspace/Run 各自的 typed owner-local delivery。
- 禁止的 `EnableEventDrivenDispatch`、`DispatchStepRequested`、通用 runtime executor 与 runtime-gap recovery command 已删除；组件执行不得形成第二条 dispatch owner。
- 禁止的 `StagePacket`、`NextAction`、RuntimeStep Accepted/Progress/Completed/Failed 通用事件结构已删除；不得以历史合同重新包装 Runtime facts、candidate 或 operation evidence。
- governance continue 必须写入 accepted governance evidence、递增 evidence revision 并回到同一 reducer；governance fail 直接生成 `governance_rejected` terminal，stale resolution 只能进入 diagnostic lineage。
- resolution 必须携带并匹配已绑定 ticket；不存在绑定、同一 dispatch 绑定不同 ticket、或 dependency 漂移都必须返回 conflict。
- operation-scoped governance continue 保持原 ExecutionKey，通过 CAS 将 governance fence `N` 推进到 resume fence `N+1`；typed grant 必须绑定 ticket、resolution、dependency、subject 与治理 receipt hash。各 concrete owner 在 receipt preflight 时验证 grant，禁止复用 `Receipt(K,N)=governance_required` 作为普通 evidence。
- File/Git/MCP/Shell/SSH/Web execute Result 只返回 owner-persisted `RuntimeOperationReceipt`；EventID、operation/evidence identity 与 source ref 由 Orchestrator 从 sealed dispatch/operation contract 纯派生，capability owner 不回显或重建第二套证据身份。
- operation-scoped governance candidate 必须保留 suspended binding 的 dependency kind/subject；不得把 persisted `approval/configuration/external` 退化为默认 `decision`，否则 ticket 无法回绑 source operation。
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
  SM-4 在 Orchestrator `Run` 中同步完成全部 inventory/load 分页；缺 subscriber、owner error、nil/wrong result 或
  cursor 异常必须使启动失败。Run 在 Setup 注册 inventory、opaque state-record 与 readiness query；Orchestrator 仅将
  已加载且已分类的 startup action 暂存为自身 process-lifecycle handoff，直到 Run owner 发布
  `TopicRunRuntimeCommandsReady` 后才以 run-scoped singleflight 交给 `BackgroundRoutine`。该 handoff 在每次
  启动重新由持久化 inventory 构建，不是第二套 persisted recovery/state machine，也不得依赖 `Weight()` 或调用普通
  `/run/command/get`。
- typed record load 返回 `RuntimeStateSchemaVersionError` 时，SM-4 只发送 immutable non-current-schema fact 给 Run owner；它不提交 drive
  failure aggregate、不请求 LLM/operation、不创建 governance ticket，且 Run owner 的
  `fresh_activation_required` notification 不再进入 drive scheduling。
- Run owner opaque aggregate update 返回 `accepted`、`aggregate_revision_mismatch`、`semantic_rejection` 或
  `owner_transient` concrete result。仅 `aggregate_revision_mismatch` 的实际 revision mismatch 可构造
  `RuntimeCASConflictError`；Orchestrator 禁止用泛化 `errors.Is(err, common.ErrConflict)` 推断 CAS retry。
- definition revision、Goal/DoD identity、正文或 capability contract 任一漂移都必须返回 conflict；definition context revision 必须与 facts 和 authority transaction 一致。
- LLM dispatch 前必须先完成 manifest、target snapshot、failure target、capability、validation scratch 和 provider ceiling preflight；缺失资源只能进入 canonical `need_prerequisite`，不得混入 LLM state-changing requiredOutput。
- block 输出只是 candidate；evidence quality、coverage binding、next action 和 accepted Result 只能由 Orchestrator 判定。
- assessment-only focus 不允许 executable operation；没有 current-run accepted evidence 的 satisfied assessment 必须被拒绝，不能用 summary、历史 memory 或自造 ref 关闭 coverage。
- ToolCall、Patch、ValidationResult、EvidenceRecord、CoverageWorkItem 和 ArtifactContinuation 只属于 Orchestrator 的单一收敛状态机，不对外暴露写入 repository。
- 跨组件交互只能使用 EventHub 与具体 command/data/result；禁止跨 owner service/repository、Envelope、JSON/reflect 类型桥和通用投递包装。
- provider-facing LLM cache prefix 必须遵守 C0、C1、C2、C3-C5 稳定分层。

## 待完善测试点

- [ ] 补齐 SM-1～SM-4 跨围栏矩阵：每个外部 fact 只能进入对应 internal event/reducer，禁止跨 owner 目标 state setter。
- [ ] 覆盖大量 Goal/DoD 的语义依赖排序、分批 focus、公平推进和 C0～C5 context budget，确认不按 ID/序号排序。
- [ ] 增加 create-only 目标重开回归：已有目标时 semantic replan 必须选择 owner catalog 中合法且 revision-bound 的 replacement，不能重复 `file/write`。
- [ ] 覆盖 effect 已发生但 receipt 未提交、receipt 已提交但 evidence 未提交、terminal 三段 delivery 每个 ack 前后崩溃的恢复。
- [ ] 覆盖 owner fact 的 stale、duplicate、out-of-order、rollback/reappearance，只退休精确依赖且不重放旧 authority。
- [ ] 覆盖 provider request incomplete/unknown submission、CAS conflict、owner transient、governance wait 与 delivery retry 的独立预算。
- [ ] 覆盖 restart inventory 的分页、cursor loop、nil/wrong result、不兼容 schema和 commands-ready handoff。
- [ ] 增加大 aggregate、长 focus queue、manifest rendering、invariant validation 和 drive loop 的 allocation/CPU benchmark。
- [ ] 增加静态边界扫描，持续拒绝 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch、auto-replan successor 与 map authority rebuild。
