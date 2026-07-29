# 运行时编排组件

## 功能说明

`kernel/orchestrator` 是 Runtime V2 convergence authority，负责 typed facts、authority transaction、prerequisite、candidate、sealed dispatch、operation evidence、governance wait、sealed terminal 与 SM-4 delivery acknowledgement。它组合多个 owner 和技术 block 完成同一 Run 的确定性收敛闭环，**但不拥有 Run、Workspace 或 Result lifecycle**。

`RUNTIME-SMO` 收口期间，本文件把 owner-local lifecycle 作为实现目标合同：现有任何由
Orchestrator 直接准入、blocked/terminal 投影或写入外部 owner state 的路径均为待删除违例，不能被视为
可兼容生产路径。

## Provider / model ceiling（2026-07-15）

- activation 持久化 `RuntimeActivationContext.ProviderTarget`
- provider ceiling prerequisite 请求与封存完整 `ProviderCeiling{Target, token/byte ceilings, revisions}`
- candidate 强制使用 ceiling target；Restart/Resume 不携带 provider/model patch
- Orchestrator 只校验 identity/revision/ceiling 完整性，不解释 ai-proxy 或模型品牌

## Runtime V2 四状态机实现约束（2026-07-16）

完整规范以 `docs/00-core/runtime-stage-contract.md` 的“Runtime V2 四状态机固化规约”为唯一权威。本模块按以下四个状态机落实现有职责，不定义第二份状态语义：

| 状态机 | 本模块职责 | 外部 owner |
| --- | --- | --- |
| SM-1 Run lifecycle | 不拥有；只消费 Run state notification 触发自身 SM-4 drive | Run owner 持久化 state/aggregate/lineage；Workspace owner 持久化 activeRun |
| SM-2 authority/focus | reducer、focus/evidence/assessment/repair/governance/terminal 裁决；编译 sealed candidate manifest | application 只编译 typed bootstrap；owner 只返回 facts；Result 只存 outcome |
| SM-3 candidate/dispatch | request session、单 evaluator、attempt 0/1、transaction、seal、typed operation、evidence commit | LLM 返回 candidate；capability block 执行 operation |
| SM-4 drive/recovery/delivery ledger | bounded drive、singleflight、restart load、delivery acknowledgement/retry；不写 owner state | Result/Workspace/Run owner 各自完成资源迁移；projection store/read model 只保存派生状态 |

实现必须保持：

- SM-1 不选择 action，SM-2 不执行 operation，SM-3 不选择 next action，SM-4 不重建 authority。
- Orchestrator 的外部 typed command/result、owner receipt、request outcome 与 delivery acknowledgement 必须先映射为
  owner-local `RuntimeInternalEvent`；Biz 只能经 `submitRuntimeV2InternalEvent` 提交，
  `RuntimeController.ApplyInternalEvent` 是唯一 mutation dispatcher，
  `RuntimeTransitionCoordinator.Commit` 是唯一 aggregate CAS 边界。internal event 不得成为跨 owner EventHub envelope。
- 所有跨状态机推进都使用 persisted aggregate + typed EventHub command/result。
- projection failure 不得覆盖 reducer terminal；重启不得重复 prerequisite/candidate/side effect。
- 任何修改 reducer/controller/drive/operation projection 前，必须说明影响的 SM 编号并补合法/禁止迁移、幂等、restart、stale/duplicate、budget/guard 测试。
- 本模块不得恢复 V1 mainline、application continuation queue、通用 RuntimeAdapter registry、projection authority 或自动 successor。

## Candidate 合同归一目标

完整语义以 `docs/00-core/runtime-stage-contract.md` 的“规范化候选合同”为准，未关闭的 candidate/receipt、
跨 SM 原子交接与 single-owner 收口顺序只由
`docs/70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md` 定义。旧 VPC R5 N1～N8、
R4/P0～P6/A-4 只作回归基线，不再是活动实施标准。

- SM-2 从 persisted focus/required output 与 typed owner facts 编译唯一
  `RuntimeCandidateContractManifest`。每个 operation kind 恰好对应一条纯值
  `SealedOperationAuthorization`，target 只能使用 `exact_target`、`existing_owner_scope`
  或 `prospective_owner_target`。
- renderer、wire schema、strict decoder、candidate evaluator、owner feasibility command 与 fixture 只消费该
  manifest 及其 hash，不得从 profile、focus、raw reason 或临时 inventory 重建第二份允许集。
- complete document 只调用一个纯 `CandidateEvaluator`，并按 manifest identity → wire/field →
  kind/variant/parameter → target → effect/coverage/dependency → replay 的固定顺序返回 `accepted` 或完整
  `CandidateRejection`。owner feasibility 是 capability owner 的独立 typed factual observation，不得伪装为
  candidate rejection。
- owner probe 前只允许用 kind 解析 manifest 中的 owner/authorization identity，该解析不做 admission；
  unknown kind 零 owner call，可解析 kind 只调用无副作用 probe。evaluator 仍优先返回顺序中更早的
  field/target 违例，owner observation 不得形成第二套 first-failure 路由。
- rejection 的 `CorrectionDisposition` 只能是 `correctable_once` 或 `rejected_final`。前者必须有
  可仅用 manifest + rejection 验证的 `CorrectionDirective`，后者零 correction call；无法归类是
  `runtime_contract_defect`，不能退化为 `candidate_correction_undefined` 或无信息 governance ticket。
- candidate pipeline 只产生 `request_recoverable`、`candidate_correctable`、
  `candidate_rejected_final`、`owner_feasibility` 和 `accepted`。attempt 0/1 与 correction matrix 只消费
  evaluator 产生的 `CorrectionDisposition`；owner feasibility 必须先成为 Orchestrator internal event，再由
  SM-2/SM-3 coordinator 选择 prerequisite、新 authority 或 typed terminal，零 correction attempt。SM-4 只从
  persisted result 恢复，不再解释 raw failure。
- owner feasibility 若证明当前 sealed validation recipe 不可用，SM-2 必须以
  `validation_recipe_exhausted` 直接形成 typed terminal；不得将其降级为普通 repair、candidate correction 或
  `runtime_contract_defect`。
- candidate retry contract 必须绑定完整 `AuthoritySnapshotID`。同 focus 收到新的 owner feasibility 事实后，即使
  action、required output 与 prerequisite hash 相同，也只能生成新合同的 attempt-0；旧 invalidated attempt 保留为
  audit lineage，不能被覆盖或变成 attempt-1。
- `OwnerInventoryBudget`、`CandidateContextBudget`、`RequestRecoveryBudget`、
  `CandidateAttemptBudget` 与 `PrerequisiteRefreshBudget` 是不可互借的合同。inventory 必须完整或
  使用 owner 可独立验证的封闭 selector，不得将 context 裁剪当成 authority 裁剪。
- `internal/pkg/workorchruntimecontract` 只实现 DTO、enum、normalize/validate/clone/hash、纯 manifest
  compiler 与 evaluator helper；不持有 Hub、不执行 I/O/CAS、不选择 retry/terminal/Runtime action。



## Effect / Receipt / Repair Directive 基线（2026-07-17）

- `RuntimeEffectScope` 是 operation 的中立 effect 合同；`ResolveExpectedEffectScope` 由 descriptor + shell `stateChanging` + sealed MCP risk/EffectScope 解析 concrete scope。
- seal 前 `ValidateCandidateEffectCompatibility` 强制：`produce_state_change` 至少一个 durable effect；`produce_validation` 不得含 durable/unknown；`stateChanging=false` shell 与 read-only/session MCP 不能关闭 durable state-change。
- `SealedDispatch.OperationExecutions` 在 seal 时写入 per-operation `ExecutionKey` + `ExpectedEffectScope`；execute 前 claim fence，owner 返回后 `owner_committed`，evidence CAS 后 `evidence_committed`；owner error 记 `outcome_unknown`，禁止盲重放。
- owner 已持久化 exact `ExecutionKey + Fence` receipt 但响应丢失时，executor 必须先重新加载 aggregate：存在 terminal receipt 则继续 governance/evidence commit，不得把 `owner_committed` 覆盖为 `outcome_unknown`。
- claimed/no-receipt recovery 只允许 sealed idempotency=`safe_to_replay|owner_managed`，并先消耗独立 operation-recovery budget；`at_most_once|unknown` 一律转 `outcome_unknown`。每项 operation evidence 独立 CAS 持久化；只有最后一项成功或任一项 rejected/stale 时，才在该 evidence CAS 中原子 settle dispatch。
- current-schema evidence commit 必须携带已持久化 owner receipt，并按 sealed expectation + `ObservedEffectScope` 联合裁决 Kind；declared Kind 不构成 authority。
- multi-operation dispatch 中，同一 dispatch receipt 造成的 prerequisite/resource revision 变化属于内生变化；可以
  退休后续 SM-2 observation，但不能仅因此清除 `ActiveDispatch`。SM-3 必须继续尚未完成且 dependency 已满足的
  sealed operation，直到全部成功或任一 operation rejected/stale 后再 settle/reduce。
- state-changing Shell receipt 必须使用 owner 实际观察到的 changed refs/resource changes；真实变化越出 sealed
  `TargetRefs` 时以 `undeclared_workspace_effect` 拒绝，不能通过额外 touch 一个声明目标取得 durable evidence。
- `Facts.RepairDirective` 是同 focus 唯一 next-action 权威（INV-05）；`ReduceRuntimeStateWithDirective` 同步 directive；anti-replay 与 repair hints 不得覆盖 directive action。
- repair directive 的创建、切换和清除均属于 Orchestrator SM-2 reducer；`internal/pkg/workorchruntimecontract` 仅保存 directive DTO、校验和纯计算。`acquire_prerequisite` / governance wait 是同一 repair cycle 的临时 interlude，必须保留 directive；只有 accepted repair evidence、focus advance 或 terminal 才能切换或清除它。
- validation entry 不是 state-change repair target；只有 focus exact target、仍新鲜的 owner-grounded failure target 或
  sealed owner selector/prospective constraint可以形成 repair authority。validation/evidence-quality gap保持
  `produce_validation`，不得在 validation-only 重试后退化为无关 `produce_state_change`；耗尽封存 validation
  recipe/budget 时必须形成 typed terminal。
- assessment gate 只消费 current-run accepted evidence、sealed coverage/effect与 owner structured validation
  observation；profile 封存的 observable recipe 只在候选选择该命令时校验必要参数；不得用 Goal/DoD、summary、
  operation/check name 的自由文本 token 相似度作为覆盖裁决。
- File owner 对 `unified_diff` 在 owner feasibility probe 中执行无写入 dry-run；hunk/context 冲突必须在 dispatch seal 前以 typed infeasible result 返回，owner 不选择后续 RuntimeAction。
- active V2 Cancel/Supersede 必须先 `RequestTermination` CAS 持久化 `RuntimeTerminationRecord`，再投影 SM-1 terminal。
- drive failure 经 `classifyRuntimeV2DriveFailure` 分类：CAS/owner-transient/projection 不终态化业务 Run。

## 组件交互及合同

- 通过 EventHub 读取 `blocks/workspace` 和 `blocks/run` 的权威快照，并只通过 Run owner CAS 合同持久化
  Runtime V2 aggregate；不通过任何 Run owner command 指定 lifecycle target state。
- Runtime V2 通过 Orchestrator 自有 `ExecuteRuntimeOperationCommand/Result` 调用 File、Git、Shell、MCP、Web 和 SSH block；operation command/result 使用具体类型，同步返回的 typed result 是提交 execution evidence 的唯一执行回执。
- 通过 EventHub 调用 Decision、Approval 和 Memory owner 的具体 command；terminal 后只发布 immutable delivery fact，
  Result/Workspace/Run owner 各自消费并产生 acknowledgement。不得直接持有这些 owner 的 service 或 repository。
- 与 `kernel/governance` 协作处理治理策略、decision/approval blocker 和恢复，但 Orchestrator 保持 runtime contract gate 权威。
- Decision 与 Approval 使用各自 `pkg/events` 定义 evidence command；Orchestrator 幂等写入自己的 EvidenceRecord 并返回具体 result。
- Orchestrator 自有 Runtime V2 drive、prerequisite、candidate、operation、governance resolution、retirement resource release与只读查询合同定义在 `kernel/orchestrator/pkg/events`。
- Runtime V2 prerequisite command/result 由 Orchestrator 在 `kernel/orchestrator/pkg/events` 定义；当前 target snapshot acquisition 通过 EventHub 交给 File block，并将具体 `TargetSnapshot` 提交回 typed runtime facts。
- `kernel/orchestrator/pkg/events` 只保存 typed command/result、receipt DTO 与确定性校验/构造；能力 Block 的 Biz 负责发送 receipt query/commit command，Orchestrator Biz 负责消费并持久化，任何 `pkg` 不得持有 Hub、执行 I/O 或推进状态。
- Runtime V2 candidate command/result 同样由 Orchestrator 定义；请求封存完整 authority transaction，LLM block 返回的 candidate 必须绑定同一 snapshot，transaction 漂移不得进入执行。
- candidate request 携带从 persisted facts 和唯一 manifest 渲染的 typed `RuntimeCandidateContext`；当前 focus/evidence/failure 是 context，operation kind/parameter/target/effect/coverage/dependency 授权只来自 manifest。C4 只投影 provider-safe failure/evidence/assessment/shape identity 与 attempt-1 的 typed rejection/directive；它不得携带 raw reason/summary/payload、owner feasibility 或 recovery/governance action。LLM block 不从 V1 map、raw reason 或临时上下文重建 authority。
- Runtime V2 dispatch 在执行前持久化 `SealedDispatch`、完整 transaction、candidate hash 和 typed step input；execution evidence 只有在 snapshot、dispatch 与 operation 一致时才能提交并触发下一次 reducer。
- capability owner 的 validation receipt 必须持久化中立 quality/zero-work/check observation，并纳入 stable receipt hash；SM-3 在普通响应与 exact-fence restart recovery 两条路径执行同一质量裁决，弱验证只进入 rejected evidence，不得因 owner exit code 为 0 关闭 coverage。
- Runtime V2 aggregate 通过 Run owner 的 EventHub update 合同持久化；Orchestrator 使用 expected revision 执行 CAS，不直接访问 Run repository，也不把 aggregate 编码进 `map[string]any`。
- Runtime V2 初始化时封存 typed Goal/DoD definition context；后续 candidate request 与重启恢复只能读取 Run owner 中持久化的定义快照，不能由调用方临时重建。
- `InitializeRuntimeV2FromRun` 只读取 Run owner 已持久化的 typed bootstrap，生成 initial facts 并执行 preflight/reducer；Orchestrator 不解析 V1 execution context map 来重建定义或 focus。
- Runtime V2 controller 统一负责 initialize、prerequisite commit、candidate seal 和 evidence commit；服务重启只加载既有 aggregate，不得因重复 Start/Resume 再次执行 reducer 或增加 attempt。
- initialize/restart identity 只比较不可变 workspace/run、definition revision/content 与 activation；`Facts.Focus` 是 SM-2 persisted cursor，合法 focus advance 后不得被拿来与 bootstrap 首 focus 比较并误判 definition drift。
- prerequisite mainline 按 typed requirement 的 canonical owner module 调用 EventHub，并在每个结果提交后原子执行 reducer；profile capability-state owner 必须与 accepted operation descriptor owner 相同，不能使用 EventHub destination 别名；多个 capability owner 的结果必须合并，不能互相覆盖。
- capability prerequisite 的 owner/class/kind 不匹配或没有 responder 必须以 typed `RuntimeCapabilityUnsupportedError` 返回，并由 SM-4 分类为 `runtime_capability_unsupported` terminal contract defect；它不得退化成默认 capability、candidate rejection、泛化 decision/approval ticket 或 retry loop。各 capability owner 的支持 kind 只在其 `pkg/common.RuntimeCapabilityKinds()` 声明，handler 只消费该纯 inventory；共享的 result/catalog observation 由 Orchestrator events 合同构造。
- operation admissibility inventory 由 canonical owner 提供完整 bounded scope 或封闭 selector，SM-2 将其编译为三种 `TargetPolicy` 之一。既有资源与 prospective target 都必须在 seal 前由同一 owner 做只读 feasibility probe；Orchestrator 不扩展 selector，也不把 inventory/context 截断当成完整授权。
- sealed candidate 通过 `ExecuteRuntimeOperationCommand/Result` 按 operation kind 投递到 File、Git、Shell、MCP、Web 或 SSH owner；跨组件 command/result 只包含具体 operation、transaction、status、failure 和 evidence 类型。
- 多 operation dispatch 按 dependency graph 确定性执行；每个 operation 独立提交 evidence，全部成功或任一失败后才结束 dispatch 并再次 reducer。
- RuntimeFacts 持久化当前 focus 与有序 remaining focus queue；当前 focus 的 required evidence 满足后，evidence commit 原子切换到下一 focus，再调用同一 reducer。
- 显式声明 `RequireAssessment` 的 focus 在执行证据齐备后进入 typed `need_assessment` authority；LLM 只能返回绑定当前 focus/coverage 的 `AssessmentCandidate`，Orchestrator 校验其 evidence refs 来自当前 Run 的 accepted evidence 后写入 assessment ledger。
- SM-2 在 assessment commit 前执行 item-level evidence gate：weak evidence 永不关闭 coverage；profile 显式声明 executable/claim evidence 时，再校验 passed-check observation 与中立 claim signal。门禁失败写入 `assessment_evidence_quality_gap` repairable assessment，并由 reducer 在封存预算内继续同 focus repair。
- 最后一个 assessment 关闭或 typed terminal failure 形成后，Orchestrator 从 current-run evidence/coverage/assessment ledger
  密封 terminal 与 result payload，并创建 terminal delivery fact；Result owner 自行持久化 accepted/rejected outcome，
  Workspace owner 自行释放 active run，Run owner 自行进入 terminal。Result block 不参与 acceptance 裁决。
- CoverageFocus 可携带 evidence role、accepted/rejected step kinds 与 accepted operations；reducer 只在 owner 已确认的 capability kinds 中做中立交集/排除，并把结果封入 `RequiredOutputContract`。若声明 accepted operations，candidate 至少包含一个匹配的具体 operation 参数，不能用同 step kind 的其他动作替代。
- provider request session（logical request ID、physical-call/fragment sequence、transport outcome）先于 candidate attempt 随 aggregate 持久化；未形成完整 candidate document 时不得创建/失败 attempt。complete attempt 0 的 `rejected_final` 直接终态；只有 `correctable_once` 可在相同 transaction、manifest 和 stable prefix 下开启 attempt 1，attempt 1 再次 complete rejection 形成 `candidate_retry_exhausted`，不能创建第三次请求或无信息 governance ticket。
- candidate retry 预算按稳定且显式持久化的 `AuthorityContractID` 计算；identity 绑定 workspace/run、完整 `AuthoritySnapshotID`、focus、action、definition/evidence revision 与 requiredOutput hash。缺失或不匹配该字段属于 current-schema invariant violation；同一 contract 超过 attempt 0/1 必须 fail-fast，禁止再次请求 provider。
- complete schema/contract failure 只记录统一 evaluator 生成的 rejection/disposition；owner feasibility 的
  `stale`、`prerequisite_required`、`infeasible` 必须作为独立 internal event 交给 SM-2，绝不进入
  correction matrix 或占用 attempt。provider authentication/configuration、transient transport、incomplete
  document 与 unknown submission 走独立 dependency 或 request-session/recovery ledger。prerequisite stale
  精确退休旧授权，历史 execution failure 不得触发 authority reseal。
- governance candidate 作为 sealed dispatch 持久化；治理结果通过 `CommitRuntimeGovernanceResolutionCommand/Result` typed owner command 提交，同一 resolution ID 与 authority snapshot 最多消费一次。
- Decision owner 可通过 `CommitRuntimeGovernanceTicketResolutionCommand` 只提交 ticket resolution；Orchestrator 从 persisted binding 补齐 authority snapshot、dispatch 与 dependency 后进入同一 resolution commit。
- governed dispatch 通过 `OpenRuntimeGovernanceRequestCommand/Result` 向 Decision owner 创建确定性 ticket；ticket、snapshot、dispatch 与 dependency 的绑定随 Runtime V2 aggregate 持久化。
- `DriveRuntimeV2Command/Result` 是 Runtime V2 唯一同步推进合同；Orchestrator 在有界循环内依次处理 prerequisite、candidate、sealed dispatch、typed operation、evidence/reducer，并只返回 `governed_wait` 或 typed terminal。重复调用和进程重启必须从 Run owner aggregate 幂等恢复。
- drive 同时校验 persisted `StateRevision` 与 `RuntimeConvergenceProgressSignature`；signature 覆盖 focus/evidence/action/requiredOutput/repair/prerequisite/dispatch/governance/terminal，不包含 candidate retry、snapshot attempt 或 revision bookkeeping。只增加 bookkeeping 的 transition 必须 fail-fast。
- 进程内后台 drive 按 workspace/run scope 使用 singleflight gate；重复 Start/Resume/governance continuation 只复用当前 drive，调度失败或 drive 结束必须释放 gate，不得用并发 CAS conflict 生成伪 `runtime_v2_drive_failed` terminal。
- reducer/aggregate 已进入 governed wait 或 terminal 后，后续 owner projection 失败只能记录 projection error 并等待幂等重试，不得把已确定的 complete/fail authority 改写成 `runtime_v2_drive_failed`。
- 普通 drive、governance resolution 与 drive-failure terminal 共用唯一 terminal delivery lane：先校验 persisted sealed aggregate，
  再发布 Result -> Workspace -> Run 的有界 delivery fact 序列，并仅在 SM-4 记录 acknowledgement；任一 delivery
  失败进入延迟重试，不得改写 reducer terminal 或越权写 owner state。
- Start/Resume 由 Application 向 Run owner 提交 scoped intent；Run owner 内部迁移并发布 `RunStateChanged` 后，
  Orchestrator 才后台执行唯一 Runtime V2 drive。application 不再拥有 Start/Resume continuation queue，Orchestrator
  service 不暴露 execution-context-patch resume facade。
- Orchestrator biz 不保留 `AcceptRunStart`、`CanResumeRun`、blocker/terminal Run/Workspace 投影或等价 lifecycle
  facade；不存在可直接调用的 V1 `StartRun`、`DispatchNextStep`、`ResumeRun`、patch resume 或自动 replan lifecycle。
- V1 runtime progress/terminal-result checkpoint、restart replay、decision resume subsystem 和 approval patch resume 已删除；Runtime V2 重启只加载 persisted aggregate，治理继续只消费 typed resolution。
- Orchestrator plugin 不再创建 RuntimeAdapter registry、EventHub runtime adapter 或 Step kind registration matrix；V2 operation 只通过各组件 typed EventHub command/result 执行。
- Orchestrator 不再暴露 V1 `NextActionForRun` 或接收 `UpdateRuntimePolicyCommand`；Runtime V2 action 只来自 typed reducer，动态策略从 canonical configuration source 按需读取。
- blocker/continue/terminal 只作为 Orchestrator persisted governance/terminal fact 发布；Run/Workspace/Result owner 的
  reducer 负责各自迁移。不得借此创建 V1 Step 或经过 V1 Result/DoD completion gate。
- SM-4 对 owner-local delivery acknowledgement 做有界重试：重复 fact 必须幂等，单边成功不得重写，缺失 ack
  只保留 delivery pending，避免单边成功后永久悬挂。
- repair ledger 的 `RepairAttempts` 是同 focus 总预算；有效新 evidence 只重置 failure-signature、validation-only 与 no-progress 窗口，不清零总预算。只有原子 focus advance 才创建下一 focus 的全新 repair ledger。
- owner 报告 state-changing no-op 时，SM-3 只持久化 rejected/no-progress shape，绝不写入 state-change evidence。
  SM-2 对要求 assessment 且有可观测 validation 的 focus 只能按 `validation -> assessment` 验证已有状态；否则以
  typed current-run provenance gap 终态化，禁止通过无关文件修改制造 durable delta。exact receipt replay 保持原
  `ExecutionKey + Fence`，不重放 owner side effect。
- candidate attempt 只投影为 SM-3 contract attempt；read model 的 repair loop、ineffective retry 和 repair budget 必须来自 `Facts.Repair` 与当前 failure，不得把历史 candidate retry 当成当前 focus repair。
- prerequisite freshness 只有一条有界的 stale/no-progress `prerequisite_refresh` ledger。owner accepted receipt 的真实资源变更是正常进展：它原子退休精确相交的 observation、持久化 invalidation record，但不扣减 recovery budget。capability-state acquisition key 的代际仅由同一 `PrerequisiteID` 的 persisted invalidation records 推导；因此资源变更会重查 owner 而不会重放旧 receipt，且无关 prerequisite、provider、CAS 与 delivery 记账都不能改变 key。
- Eventstream 订阅仍在生产发布的 Orchestrator、Run 和 Result typed notification 形成审计记录；Runtime V2 只消费同步 typed operation result 推进状态。
- workspace/run purge 时，Orchestrator 只删除自身 ToolCall/Patch/ValidationResult/Evidence/Coverage/ArtifactContinuation/MemoryTrace；Step 与 RuntimeCheckpoint 必须通过带 WorkspaceID/RunID 的 Run owner purge command 删除，Orchestrator 不持有其 repository。
- Runtime V1 retirement resource release 使用 Orchestrator 自有 typed command，关闭 pending/executing Step、ToolCall、ArtifactContinuation，并释放 managed validation resource；重复调用只返回本次实际变化数量。

## 相关约束

- 核心 runtime 必须保持领域中立，不得写死软件开发、语言栈、测试框架、MCP 或业务领域逻辑。
- 每次 dispatch 必须基于一致的 workspace/run/definition/evidence 快照。
- stale evidence 只能进入 diagnostic lineage，duplicate event 必须幂等；运行重启后从持久化 aggregate 恢复，不允许依赖进程内去重 map。
- execution evidence 必须携带来自 sealed operation 的 coverage work item IDs；前序 focus evidence 不得满足后续 focus，最后一个 focus 满足后才允许 Complete。
- profile-specific accepted/rejected kinds 只能作为 typed focus requirement 输入；reducer 不识别 profile ID、deliverable ID、软件开发术语或具体业务语义。
- `runtimeContractVersion=v2` 仅为审计字段；正式状态以 typed `RuntimeStateRecord` 为准，不得作为 V1/V2 路由开关。
- authority attempt 只有在 state、action、focus、definition/evidence revision 和 requiredOutput hash 全部相同时才增加；prerequisite 或其他事实推进必须重置 attempt。
- block adapter 的既有 map 输入只能存在于消费 block 内部的技术适配边界，不得重新进入 EventHub command/result、RuntimeFacts、candidate、dispatch 或 evidence。
- accepted candidate attempt 必须对应 active 或 completed sealed dispatch；不存在 dispatch 的 accepted attempt、第三次请求或 accepted 后重试都必须被 invariant validation 拒绝。
- drive loop 的每个非等待、非终态 transition 都必须增加 persisted state revision；无持久化进展或超过 transition guard 必须 fail-fast，禁止进程内空转。
- Start/Resume 生产入口仅提交 Run owner intent；缺少 typed bootstrap 由 Run owner 与 terminal delivery saga
  收敛为 typed terminal，禁止以 `RuntimeBootstrap == nil` 为条件保留兼容路由。
- `ResumeRunCommand` 不得重新增加 loaded Run、`ExecutionContextPatch` 或其他 V1 continuation payload；上下文变化只能通过创建新的 Runtime V2 activation/bootstrap 表达。
- Orchestrator service facade 不暴露 `DispatchNextStep`；V2 operation 只能由 persisted drive/reducer 主链调度，外部 module 不得直接推进 legacy Step。
- Orchestrator service facade 不暴露 Start/Resume；这两个生产 EventHub handler 属于 Run owner，Run state notification
  才触发 Orchestrator internal drive，禁止外部组件通过 service lifecycle 重新进入 V1 主链。
- Runtime V2 不自动创建 V1 replan successor；重新定义目标或执行基准时必须通过新的 typed activation/bootstrap 创建独立 V2 Run。
- 不记录或恢复 `runtime_step_result_available`、watchdog recovery policy 等 V1 checkpoint；历史 checkpoint 仅作为 retirement 后只读审计数据。
- Run/Workspace 状态通知不再隐式写入 V1 runtime checkpoint；状态事件与 Runtime V2 aggregate 各自保持单一职责。
- HTTP runtime 配置更新不得向 Orchestrator 推送进程内 repair policy 副本，避免 canonical config 与 V1 policy cache 形成双权威。
- Orchestrator 不注册 ValidateRun/CompleteRun command；validation 只能提交 sealed operation evidence，completed terminal
  只能来自 reducer decision 与 Result/Workspace/Run 各自的 typed owner-local delivery。
- V1 `EnableEventDrivenDispatch`、`DispatchStepRequested`、通用 runtime executor 与 runtime-gap recovery command 已删除；组件执行不得形成第二条 dispatch owner。
- V1 `StagePacket`、`NextAction`、RuntimeStep Accepted/Progress/Completed/Failed 通用事件结构已删除；不得以历史合同重新包装 Runtime V2 facts、candidate 或 operation evidence。
- retirement resource command 必须拒绝任何带 Runtime V2 bootstrap/state 或 V2 contract 的 Run；历史 evidence、terminal Step、completed ToolCall 和已应用 ArtifactContinuation 不得被修改。
- governance continue 必须写入 accepted governance evidence、递增 evidence revision 并回到同一 reducer；governance fail 直接生成 `governance_rejected` terminal，stale resolution 只能进入 diagnostic lineage。
- resolution 必须携带并匹配已绑定 ticket；不存在绑定、同一 dispatch 绑定不同 ticket、或 dependency 漂移都必须返回 conflict。
- operation-scoped governance continue 保持原 ExecutionKey，通过 CAS 将 governance fence `N` 推进到 resume fence `N+1`；typed grant 必须绑定 ticket、resolution、dependency、subject 与治理 receipt hash。各 concrete owner 在 receipt preflight 时验证 grant，禁止复用 `Receipt(K,N)=governance_required` 作为普通 evidence。
- operation-scoped governance candidate 必须保留 suspended binding 的 dependency kind/subject；不得把 persisted `approval/configuration/external` 退化为默认 `decision`，否则 ticket 无法回绑 source operation。
- `runtime_v2_governance` resolution committed 后，Orchestrator 先持久化 governance evidence 并发布 immutable
  continuation/terminal fact；Run owner 决定自身 blocked→active 或 terminal，application resolved watcher 不得发送 Resume。
- 相同 governance resolution 的 duplicate commit 只能重试幂等 fact delivery；Run owner reducer 与 singleflight gate
  分别合并 lifecycle input 与 drive，避免“aggregate 已提交但 Run 仍 blocked”的单边状态。
- provider transport、operation recovery、CAS reload、terminal delivery 使用独立 persisted recovery policy/ledger；candidate attempt 0/1 与 per-focus repair budget 不得消费这些预算。
- startup recovery 必须通过 Run owner concrete typed inventory 获取 `active_no_dispatch`、`active_sealed_dispatch`、`blocked_governance`、`terminal_delivery_pending`，不得由 Orchestrator 使用通用 Run List 自行推断；`active + RuntimeBootstrap + no RuntimeStateRecord` 是 `active_no_dispatch`，必须进入同一 DriveRuntimeV2 初始化链路；`runtimeContractVersion` 仅为审计字段，不得作为 inventory 或路由门槛。SM-4 在 Orchestrator `Run` 中同步完成全部 inventory 分页，缺 subscriber、owner error、nil/wrong result 或 cursor 异常必须使启动失败。盘点成功后才以 run-scoped singleflight 将具体 re-drive 交给 `BackgroundRoutine`，不建立 application scan 或第二套 startup retry 状态机。
- definition revision、Goal/DoD identity、正文或 capability contract 任一漂移都必须返回 conflict；definition context revision 必须与 facts 和 authority transaction 一致。
- LLM dispatch 前必须先完成 manifest、target snapshot、failure target、capability、validation scratch 和 provider ceiling preflight；缺失资源只能进入 deterministic `AcquirePrerequisite`，不得混入 LLM state-changing requiredOutput。
- block 输出只是 candidate；evidence quality、coverage binding、next action 和 accepted Result 只能由 Orchestrator 判定。
- assessment-only focus 不允许 executable operation；没有 current-run accepted evidence 的 satisfied assessment 必须被拒绝，不能用 summary、历史 memory 或自造 ref 关闭 coverage。
- ToolCall、Patch、ValidationResult、EvidenceRecord、CoverageWorkItem 和 ArtifactContinuation 只属于 Orchestrator 的单一收敛状态机，不对外暴露写入 repository。
- 跨组件交互只能使用 EventHub 与具体 command/data/result；禁止跨 owner service/repository、Envelope、JSON/reflect 类型桥和通用投递包装。
- provider-facing LLM cache prefix 必须遵守 C0、C1、C2、C3-C5 稳定分层。
