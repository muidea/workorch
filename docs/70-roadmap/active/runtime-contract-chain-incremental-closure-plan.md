# Runtime Contract Chain Incremental Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-30

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-deterministic-ledger-assessment-closure-plan.md`

## Purpose

本文用于把后续 WorkOrch 收敛修改从“多点同时补丁”改为“单环节合同固化”。每轮只处理一个环节，先写清合同、再改代码、再用失败样本和投影证据证明该合同生效，避免在多轮修改中引入互相覆盖的并行路径。

当前 live verification 暴露的主要偏差不是某个业务模板失败，而是 runtime 主链路仍未足够硬：

```text
coverage/evidence ledger
  -> current action
  -> required output
  -> candidate validation
  -> rejected or accepted evidence
  -> ledger recompute
  -> projection
```

后续收口只能围绕这条链路逐环节推进。WorkOrch core 必须保持业务中立，不允许写入模板、语言、工具、命令、workspace title、live workspace id 或业务目标文本特判。

## Contract Audit

- Scope: live workspace `ws-705249a8d7816330/run-012`、`ws-db040d24a190f6e5/run-012` 暴露出的 current contract 审计缺失、LLM 错形态输出、decision fallback、evidence binding 和 projection/resource 偏差。
- Observed state:
  - `ws-705249a8d7816330/run-012` 最终 `failed`，current focus `cwi-085` 仍缺 `validation`、`state_change`，convergence status 的 next action 已是 `state_change`，但最新 LLM step 输出仍是 `llm_decision_required` / `dodAssessment` 形态。
  - `ws-db040d24a190f6e5/run-012` 处于 `blocked_on_decision`，current focus `cwi-036` 仍缺 `validation`、`state_change`，latest violation 仍是 `coverage_assessment_incomplete`。
  - snapshot 中最近 step 的 `runtimeStage`、`convergenceContract`、`requiredOutput` 在 summary/detail projection 下不可审计，无法判断是投影裁剪还是 step input 未携带 current contract。
  - 无 in-flight run 时 metadata scan 仍持续增长，说明 projection/status/snapshot 资源路径仍可能高频读取。
- Expected state:
  - 每个可调度 step 都能审计到同一 authority snapshot 派生的 `currentAction`、`currentFocusId`、`missingEvidenceKinds`、`requiredOutput` 和 `runtimeStage`。
  - ledger/current action 是唯一下一步来源；LLM 只能按 required output 生成候选内容。
  - LLM 错形态输出只能进入 rejected diagnostic evidence，不能转成 accepted evidence、result、artifact、memory 或普通 decision fallback。
  - 只有真实外部依赖、权限、凭证、人工选择、审批或配置缺失才允许 decision gate。
- Deviation:
  - 当前无法通过投影稳定审计 current contract，导致后续 live 验证只能推断 LLM 是否收到正确合同。
  - 违例输出仍可能沿 repair/decision/failure 分支影响 run 结果，而不是被隔离在 rejected diagnostic 后由 ledger 重新推导。
  - 已有 runtime validation 记录与 coverage item accepted binding 之间仍可能断裂。
- Owner: WorkOrch core。当前偏差位于中立 runtime contract、read model projection、evidence ledger 与 governance eligibility，不属于 workspaceTemplate/Soul/policy 的业务策略。
- Event path:
  - command/event -> owner state -> coverage/evidence ledger -> current action -> required output -> step input -> LLM/block output -> contract gate -> accepted/rejected evidence -> ledger recompute -> projection/ViewModel。
- Contract gap:
  - 缺少逐环节可审计门禁，导致多个模块仍可能在同一轮修改中同时改变 next action、required output、decision fallback 或 projection 表达。
  - summary/detail projection 没有保留 compact current contract audit，使 live verification 无法区分“输入缺合同”和“投影裁剪”。
- Proposed change:
  - 先新增本计划，后续每轮只关闭一个环节。
  - 本轮只收口 `S1 Current Contract Audit Projection`：在 compact projected step 中暴露中立的 `contractAudit`，不改变 runtime 状态机、ledger、decision、repair 或 validator 语义。
- Boundary check:
  - 所有字段来自结构化 runtime contract、stage packet、coverage work item context、current action contract 和 convergence authority。
  - 不解析业务文本，不识别模板/语言/命令，不根据 workspace title 或 live id 分支。
- Verification:
  - 新增 read model projection 单测，证明 projected step 不暴露 full input，但保留 compact contract audit。
  - 边界搜索确认没有新增模板、语言、工具、命令特判。
  - 后续 live verification 通过 snapshot 检查 `step.contractAudit`，确认 LLM step 是否携带 current contract。

## Incremental Closure Discipline

每轮开始前必须写明唯一 active contract。本轮之外的相邻环节不得修改，除非文档先更新并重新执行 Contract Audit。

每轮完成标准：

- 有失败样本或投影样本测试。
- 有正常路径测试或现有测试覆盖。
- 有边界搜索，确认 core 未新增业务特判。
- 说明本轮未修改哪些相邻环节。
- 若同类 live 偏差连续两轮未改善，停止局部补丁，回到完整链路审计。

禁止事项：

- 禁止一轮同时修改 ledger、required output、validator、repair、decision、projection 多个语义环节。
- 禁止让 LLM 输出决定 next action。
- 禁止把 rejected output 写成 accepted evidence、result、artifact 或高价值 memory。
- 禁止把普通合同违例、缺 evidence、retry 耗尽直接转成人工 decision。

## Closure Slices

### S1 Current Contract Audit Projection

Status: completed

目标：让每个 projected step 都能审计当前合同，而不暴露 full input，也不改变 runtime 状态机。

合同：

- `ProjectStepView` 不应输出完整 `input`。
- 若原始 step input 中存在 current contract 相关结构，projected step 必须输出 compact `contractAudit`。
- `contractAudit` 至少包含可得的：
  - `runtimeStage`
  - `currentActionKind`
  - `currentFocusId`
  - `coverageWorkItemIds`
  - `missingEvidenceKinds`
  - `requiredOutputSource`
  - compact `requiredOutput`
- `contractAudit` 只能从结构化字段读取：`currentActionContract`、`stagePacket`、`coverageWorkItemContext`、`convergenceAuthority`、`repairActionPackage.requiredOutput`。
- 缺字段时只体现缺失，不做业务推断。

不处理：

- 不改变 next action 选择。
- 不改变 required output 渲染。
- 不改变 validator。
- 不改变 repair/decision fallback。
- 不改变 evidence binding。

验收：

- projected step 的 `input` 为空或省略。
- projected step 的 `contractAudit.currentActionKind`、`currentFocusId`、`missingEvidenceKinds`、`requiredOutput` 可见。
- status/snapshot 可据此判断 step 是否收到 current contract。

### S2 Ledger NextAction Single Authority

Status: completed

目标：只有 coverage/evidence ledger 与治理对象能决定 deterministic next action。

合同：

- repair、assessment、projection、历史 step input、旧 required output 只能作为 diagnostic lineage。
- 若 ledger 当前缺 `validation` 或 `state_change`，next action 必须指向对应 narrow action。
- open scoped decision/approval 只能在同 scope owner state 中存在时 preempt next action。

不处理：

- 不改 validator 错形态拒绝策略。
- 不改 evidence binding。

验收：

- LLM assessment failure 不能覆盖更新后的 ledger next action。
- 无 open scoped decision 时，缺 evidence 不显示 `request_decision`。

### S3 NextAction To RequiredOutput Mapping

Status: completed

目标：required output 只能由 current action 派生，不能给 LLM 多套可选输出合同。

合同：

- `state_change` 只能要求 state-changing graph 或等价 state-changing candidate。
- `need_evidence` 只能要求 evidence acquisition / validation graph。
- `assessment` 只在 evidence ready 且需要语义判断时允许。
- `decision` 只在 governance eligibility 成立时允许。

验收：

- current action 与 `currentActionContract.requiredOutput`、`stagePacket.requiredOutput`、`repairActionPackage.requiredOutput` 完全同源。

### S4 Validator Rejects Wrong Output Shape

Status: completed

目标：LLM 返回与 current action 不匹配的输出时，稳定进入 rejected diagnostic，不改变状态机。

合同：

- `state_change` 下返回 `dodAssessment`、`llm_decision_required` 或空 graph 必须被拒绝。
- `need_evidence` 下返回纯 decision/assessment 必须被拒绝。
- rejected diagnostic 不得创建 accepted result/artifact/memory。

验收：

- 以 live 形态构造失败样本：expected `state_change`，actual `llm_decision_required` + `dodAssessment`，应得到 rejected diagnostic 和可审计 retry/terminal reason。

### S5 Accepted Evidence Binding Closure

Status: completed

目标：只有结构化绑定完整的 accepted evidence 能关闭 coverage item。

合同：

- accepted evidence 必须具备 `coverageWorkItemIds`、`structuredTargetRefs`、`observedClaims` 或等价 proof summary。
- runtime-step record 不能自动等同于 DoD validation evidence。
- 缺 binding 的记录只能 supporting diagnostic，不能关闭 DoD。

验收：

- complete accepted validation closes the current evidence-ready item。
- missing binding keeps item unresolved with exact missing reason。

### S6 Decision Gate Narrowing

Status: completed

目标：decision 不再作为普通合同违例、缺 evidence 或 retry exhausted 的兜底。

合同：

- 只有 external dependency、permission、credential、approval、configuration、user choice 或无法自动判断的治理问题才能创建 decision。
- 若 ledger 仍有 deterministic next action，则 decision eligibility must deny。

验收：

- 缺 `validation/state_change` 但无外部依赖时，不创建 waiting decision。

### S7 Projection And Resource Read Path

Status: completed

目标：TUI/Web 主视图和 status/snapshot 不应在空闲或 blocked 状态下持续触发全量 metadata scan。

合同：

- 主视图消费 compact projection。
- full snapshot/detail 仅用于显式详情或调试。
- metadata scan 必须有 revision/cache/dirty guard。

验收：

- queue=0、in-flight=0 时 metadata scan 不持续快速增长。
- projection list/summary 路径不克隆 detail-heavy payload；显式 detail/snapshot 路径仍保留完整 detail 投影。

### S8 Observability Full Payload Read Boundary

Status: completed

目标：显式 observability/debug 读取必须保持完整事件时间线和 payload hydrate，
不能被普通 projection/detail snapshot 的轻量缓存降级。

合同：

- 普通 run/workspace snapshot 默认可使用 compact metadata-only event timeline。
- terminal run snapshot warm cache 只能服务普通 snapshot/status/detail 投影路径。
- run observability 必须走 observability read model 路径，保留 stream progress
  timeline 和 payload hydrate。
- observability 不能从 compact run snapshot cache 或 warmup inflight 中派生。

不处理：

- 不改变 projection list/summary 行为。
- 不改变 runtime 状态机、ledger、required output、validator、repair 或 decision gate。
- 不新增 watchdog 或补偿恢复。

验收：

- snapshot/status 仍可复用 terminal run snapshot cache。
- observability 即使存在 terminal snapshot cache 或 warmup inflight，也会触发独立
  observability read。
- observability 入口不被 compact snapshot 的 payload/timeline 策略影响。

### S9 SSE Projection-First Default View

Status: completed

目标：TUI/Web 默认事件流必须是 projection-first，full owner event stream 只能
通过显式 debug/full view 使用。

合同：

- `/api/v1/workorch/events/stream` 默认 view 是 `projection`。
- `?view=projection` 与默认行为一致，只订阅 `workspace_projection_changed`。
- `?view=full` 保留原 full owner event stream，用于调试、观测和 raw event
  replay。
- 未识别 view 必须降级为 projection，而不是 full。

不处理：

- 不改变 EventHub owner event 发布。
- 不改变 event replay recorder 的内部 full 记录能力。
- 不改变 projection builder、runtime 状态机、ledger、required output、
  validator、repair 或 decision gate。

验收：

- 默认 stream 不转发 raw step/workspace owner event payload。
- 显式 `view=full` 仍能流式和 replay raw owner event。
- 默认和 projection view 都只暴露 server-projected ViewModel event。

### S10 Canonical Run Command Intent Boundary

Status: completed

目标：`resume` 是唯一常规推进语义，`recover` 不再作为 port、REST、
EventHub scheduler 或 projection 可见的 run command intent。

合同：

- `workorchport.NormalizeRunCommandIntent` 只接受 `resume`、`restart`、
  `replan`，空 intent 归一化为 `resume`。
- REST run resume body 的 `intent` 只接受 `resume` 或 `replan`；`recover`
  必须被拒绝，不能进入 scheduler command。
- workorchd scheduler command 只接受 `resume`、`restart`、`replan`。
- run command projection/idempotency key 不再生成 `recover/...`。

不处理：

- 不改变 watchdog 的 runtime gap recovery owner command。
- 不改变 startup inactive running reconciliation。
- 不改变 run continuation queue、workspace restart queue 或 decision replan
  continuation 语义。
- 不改变 runtime ledger、required output、validator、repair 或 decision gate。

验收：

- port normalize 单测证明 `recover` 被拒绝。
- HTTP run resume 单测证明 `intent=recover` 不被 scheduler 接受。
- scheduler command 单测证明直接投递 `recover` 不入队。
- 边界搜索确认主 run command intent 不再暴露 `RunCommandIntentRecover`。

### S11 TUI Decision Prompt Requires Blocking Ticket

Status: completed

目标：TUI 只能展示 workorchd 投影中可解析的 decision/approval blocker，不得
仅凭 workspace state 本地推断 `waiting_decision` 或 approval prompt。

合同：

- `waiting_decision` prompt/progress 必须同时具备 projected authority 与
  non-empty `blockingTicketId`。
- approval prompt/progress 必须同时具备 projected authority 与 non-empty
  `blockingApprovalId`。
- 缺少 blocker id 时，TUI 不创建本地 blocker、不切换 decision/approval
  prompt、不显示 waiting decision/approval progress。
- TUI 只能把缺 blocker 的异常作为普通 workspace overview 显示，等待下一次
  server-projected ViewModel 修正。

不处理：

- 不改变 workorchd projection builder。
- 不改变 decision/approval owner state 或 eligibility。
- 不改变 runtime ledger、required output、validator、repair 或 scheduler。

验收：

- TUI overview 单测证明 projected `waiting_decision` 但无 `blockingTicketId`
  不会进入 decision prompt/progress。
- 既有有 ticket 的 projected waiting decision 仍进入 decision prompt。

### S12 Final Dispatch StagePacket Rebuild

Status: completed

目标：provider/block dispatch 前的最终 LLM input 必须拥有从当前 authority
snapshot 重建的 StagePacket，不能因为中途 enrichment 或旧 input 带入的
`stagePacket` 漂移而把可修复派生字段误判为不可恢复合同错位。

合同：

- final dispatch gate 必须在验证前重新构建 `stagePacket`。
- 重建顺序必须是：rebuild StagePacket -> synchronize requiredOutput mirrors
  -> attach currentActionContract -> validate dispatch input。
- `stagePacket.activeFocus`、`stagePacket.requiredOutput`、
  `currentActionContract.requiredOutput` 和 top-level `requiredOutput` 必须来自
  同一当前 convergence authority。
- 若重建后仍不一致，才按 `runtime_internal_contract_misalignment` fail-fast。

不处理：

- 不改变 NextAction 选择。
- 不改变 requiredOutput 派生规则。
- 不改变 validator 输出形态拒绝策略。
- 不改变 evidence ledger、decision gate、projection 或 TUI。

验收：

- final gate 能修正 stale `stagePacket.activeFocus` 并通过验证。
- final gate 仍会拒绝重建后无法对齐的 dispatch input。

## Progress Log

- 2026-06-30: created plan from live verification. Active slice is S1 only.
- 2026-06-30: completed S1 by adding compact projected `step.contractAudit`.
  Full `step.input` remains omitted from projected step views; the audit only
  exposes current contract fields derived from structured runtime DTOs.
  Runtime state machine, ledger, required output rendering, validator,
  repair/decision fallback and evidence binding were intentionally not changed
  in this slice.
- 2026-06-30: completed S2 by preventing validation policy routes from
  directly overriding a missing-validation ledger gap into top-level
  `request_decision` when there is no open scoped governance object. Policy
  governance metadata is still retained in the validation required output for
  later S6 handling. Required output rendering, validator behavior,
  repair/decision creation and evidence binding were intentionally not changed
  in this slice.
- 2026-06-30: completed S3 by deriving active `requiredOutput` from the
  current convergence state/action before rebinding stale mirrors.
  `currentActionContract.requiredOutput`, `stagePacket.requiredOutput`,
  `repairActionPackage.requiredOutput` and top-level `requiredOutput` now
  converge to the same state-scoped contract for validation, evidence,
  state-change and target-snapshot acquisition paths. Validator behavior,
  decision creation, repair retry handling and evidence binding were
  intentionally not changed in this slice.
- 2026-06-30: completed S4 by adding a current-action output-shape gate in
  the LLM execution contract validator. Steps with an explicit
  `currentActionContract` now reject `state_change` outputs that return
  `llm_decision_required`, `dodAssessment` or an empty graph instead of a
  state-changing candidate, and reject evidence-acquisition outputs that
  return pure `llm_decision_required`/assessment shapes. Existing runtime
  repair, provider retry and replan-continuation recovery paths remain owned
  by their existing validators. Decision creation, repair retry budget,
  accepted evidence binding and projection were intentionally not changed in
  this slice.
- 2026-06-30: completed S5 by making accepted `validation` and `state_change`
  evidence count toward coverage closure only when a structured binding/proof
  is present. Derived coverage state now ignores flat accepted refs for
  binding-required evidence kinds; insufficient validation/state-change records
  are kept as supporting evidence with an explicit reason. DoD assessment
  updates inherit the matched coverage item's structured target refs only after
  the item match succeeds and still require valid proof claims. Evidence record
  reuse now carries structured source requirement, coverage target and coverage
  work item refs into acceptance options. Decision narrowing, repair retry
  budgets, projection/resource read paths and TUI/Web behavior were
  intentionally not changed in this slice.
- 2026-06-30: completed S6 by making deterministic ledger next actions
  preempt decision ticket creation. `ShouldCreateDecisionTicket` now denies
  decision creation whenever the current convergence authority already exposes
  a continuable next action such as validation, state change, assessment,
  target-snapshot acquisition or completion. Decision creation remains allowed
  only when no deterministic next action is present and the decision context
  declares a real governance dependency, including external evidence,
  permission, credential, approval, configuration, user choice, provider or
  context-gate dependency. Runtime contract violations, missing evidence and
  retry exhaustion no longer fall through to decision by default. Required
  output rendering, accepted evidence binding, repair retry budgets and
  projection/resource read paths were intentionally not changed in this slice.
- 2026-06-30: completed S7 by narrowing projection read paths for list and
  summary consumers. `Store.List()` now builds its prompt/ViewModel state from
  a compact list snapshot containing only workspace, active run status,
  heartbeat, task progress and scoped governance blockers. Summary reads reuse
  that compact source and add only lightweight summary fields such as outcome,
  coverage summary, command status and usage summary; detail-heavy fields like
  context bundles, full run/result/artifact collections, LLM traces and event
  records remain available only through explicit detail snapshot reads. TUI
  `FetchAll` is covered by a regression test proving the main view calls
  workspace list without hydrating snapshots. Runtime S1-S6 semantics,
  coverage ledger, required output rendering, validator, repair and decision
  gates were intentionally not changed in this slice.
- 2026-07-01: completed S8 by isolating run observability from compact terminal
  run snapshot cache and snapshot warmup inflight. The `/observability` route
  now always uses the explicit observability read path so debug reads preserve
  full timeline/payload semantics, while ordinary snapshot/status reads keep
  their compact cache behavior. Runtime state machine, ledger, required output,
  validator, repair, decision and template policy were intentionally not
  changed in this slice.
- 2026-07-01: completed S9 by changing public SSE stream view normalization to
  projection-first. Unqualified `/events/stream` requests now subscribe only to
  `workspace_projection_changed`, while existing raw owner event stream tests
  and clients must opt in with `view=full`. The internal replay recorder still
  records the full stream explicitly, so observability/debug replay remains
  available. Runtime state machine, ledger, projection builder, required output,
  validator, repair, decision and template policy were intentionally not
  changed in this slice.
- 2026-07-01: completed S10 by removing the non-canonical `recover` run command
  intent from workorchport, remote backend dispatch, REST resume payload
  validation and workorchd scheduler command handling. Runtime gap recovery
  remains a separate owner command for watchdog/external timeout paths; it is
  no longer exposed as a user-visible run command intent. Run continuation
  queue behavior, restart/replan semantics, runtime state machine, ledger,
  required output, validator, repair and decision gates were intentionally not
  changed in this slice.
- 2026-07-01: completed S11 by tightening TUI blocker progress and overview
  synchronization so projected `waiting_decision` requires a non-empty
  `blockingTicketId`, and projected approval wait requires a non-empty
  `blockingApprovalId`. This keeps TUI as a projection consumer and prevents a
  local decision/approval prompt from being inferred from state alone. workorchd
  projection, governance owner state, scheduler, runtime state machine, ledger,
  required output, validator, repair and decision gates were intentionally not
  changed in this slice.
- 2026-07-01: completed S12 by rebuilding StagePacket in the final LLM dispatch
  authority gate before synchronizing requiredOutput mirrors and attaching
  currentActionContract. Stale derived `stagePacket.activeFocus` can now be
  repaired from current convergence authority before provider/block dispatch;
  unresolved mismatch still fails fast as runtime internal contract
  misalignment. NextAction selection, requiredOutput derivation, validator
  shape rules, evidence ledger, decision gate, projection and TUI were
  intentionally not changed in this slice.
