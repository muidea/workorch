# WorkOrch Current Design Baseline

Status: normative

Type: design-baseline

Last Verified: 2026-08-26

## Purpose

本文汇总 WorkOrch 的现行设计基线，供产品、组件与验证文档统一引用。它不声明每个合同已经完成代码收口；代码符合度
由 executable tests 和文档树外的 issue/ticket 跟踪。实施进度、候选方案、单次审计和 dated verification 不在 `docs/` 中维护。

## Required Production Runtime

**Production runtime path = Runtime only.**

```text
Workspace owner seals admitted startup input
  -> Workorchd builds typed RuntimeBootstrap
  -> Run owner performs SM-1 activation/continuation/replacement
  -> Orchestrator drives persisted aggregate through SM-2/SM-3/SM-4
  -> capability owner returns factual receipt
  -> Orchestrator judges evidence and terminal
  -> Result -> Workspace -> Run terminal delivery
```

- Run owner CAS-persists lifecycle, replacement intent and opaque `RuntimeStateRecord`; it does not interpret SM-2/3/4.
- Orchestrator is the sole reducer/drive/evidence/governance-resolution/terminal authority for SM-2/3/4.
- capability blocks execute owner-defined typed operations; Result stores already-judged formal outcomes.
- Eventstream, projection, HTTP/SSE, TUI/Web and historical process records are audit/display only.
- recovery is idempotent re-drive from the persisted aggregate.
- `run.replacement` is a Run-owner SM-1 subordinate transaction, not another machine. Administrative termination starts as a
  Run-private intent; Orchestrator only persists/retries the SM-4 `RuntimeTerminationRecord` delivery handoff, and the final Run
  lifecycle change occurs only through a Run-private SM-1 event.
- bounded recoverable runtime wait只在typed recovery condition、trigger/deadline和budget完整时成立；它是SM-2/3/4
  aggregate fact与SM-4 re-drive责任，不新增SM-1 lifecycle state。
- provider、candidate、semantic proof、operation、repair与recovery消费由各自persisted sub-ledger持有；产品级budget
  view只确定性汇总。

The normative machine and transaction details are in [Runtime Core](../00-core/overview.md); implementation must converge to
this path without a parallel compatibility authority.

## Definition And Activation

- setup input consists of Template, Goal, DoD and an optional workspace directory; blank directory resolves to
  `.workorch-workspaces/<workspace-id>` before sealing;
- Workspace owner persists a pre-seal `DefinitionAdmissionCandidate`; pure admission accepts only complete `all_of` semantics;
- explicit `one_of`/`any_of`, threshold or unresolved required relation creates a blocking clarification ticket and cannot create a Run;
- promotion seals exact Goal/DoD, Template/Profile snapshot, resolved directory and `StartupInputHash`; Definition admission
  additionally binds canonical Template/Workspace evidence-policy snapshots through `AdmissionPolicyBindings` and rejects any
  legacy `prospectiveTargetRef` before promotion, so policy sealing cannot preselect a Runtime exact target set;
- formal Workspace-owned DoD resource only has `ready`; pre-seal draft belongs to the admission candidate, while
  satisfied/unsatisfied/indeterminate are per-Run Orchestrator assessment facts and are never written back to the DoD definition;
- every Start/Restart ingress requires `WorkspaceActivationAdmission=eligible`;
- after startup input is sealed, bounded LLM semantic planning/diagnosis/candidate generation is allowed, but cannot alter the
  startup input, choose Runtime action, grant credit or decide terminal;
- Definition changes require a newly admitted revision and explicit Restart; Resume stays on the original revision.
- execution-time implicit one-of、undeclared prerequisite或semantic ambiguity走专用Run-scoped clarification
  command/result；affected focus停止新operation，修正只通过新candidate + explicit Restart进入successor，predecessor不扩图。
- every prospective owner-fact acquisition seals an exact `ProspectiveSatisfactionContractRef`; Runtime composes the acquisition
  owner's hypothetical delta with the required owner's non-selectable dependency observation. The dependency catalog never
  becomes an LLM-selectable operation/surface. The required owner publishes one provider-safe typed mandatory-`all_of` clause
  constraint; Runtime seals it into the acquisition/manifest and projects it only in C3. Definition/activation does not seal an
  exact prospective target set: one SM-3 semantic proposal supplies 1–8 unique canonical `ProspectiveTargetRefs` at the intersection
  of Definition declaration selector/candidate refs and current operation selector/owner constraints. Every clause must be
  covered and every target must contribute; no `one_of`/`any_of` or singular compatibility field is accepted. File applies
  path/absence/parent/aggregate-capacity admission to the entire set on one complete manifest and returns only a no-side-effect
  hypothetical delta; Shell judges `would_satisfy` only from its resulting manifest. A mismatch is rejected by document
  admission/schema correction with zero File/Shell probe; only the SM-2 binding makes the admitted owner-validated set
  authoritative. SM-3 then requires one exact-singleton, canonically ordered operation per target. An exact-current composite
  `would_not_satisfy` contradicts the published contract and directly closes
  `prospective_satisfaction_contract_violated -> semantic_contract_failed`; stale only refreshes facts.

See [Definition Convergence](../00-core/definition-convergence/overview.md) and
[Activation Flow](../00-core/definition-convergence/activation-flow.md).

## Coverage And Formal Outcome

- Definition-derived required item确定性映射到stable CWI；coverage、evidence和assessment始终是三个独立维度。
- accepted evidence必须匹配current binding、effect、target、quality、freshness与实际依赖的owner revision set；receipt、exit 0、
  artifact存在或模型声明均不直接授予credit。
- current all-of baseline没有`indeterminate`成功例外；全部CWI settled后仍须由Orchestrator形成Run-level terminal witness。
- formal closeout逐requirement/CWI携带三轴、accepted refs、artifact/deliverable、known limitations和execution lineage；Result
  owner只幂等保存，不重新评估DoD。

See [Semantic Convergence](../00-core/runtime/semantic-convergence.md) and
[Result Closeout](../10-domain-model/result-closeout.md).

## State Ownership

| Machine | Sole owner | Scope |
| --- | --- | --- |
| SM-1 | `blocks/run` | Run lifecycle, continuation, administrative intent and same-owner subordinate replacement transaction |
| SM-2 | `kernel/orchestrator` | authority/focus, CWI binding/credit, assessment, repair and terminal |
| SM-3 | `kernel/orchestrator` | request session, candidate attempts, sealed dispatch and receipt transaction |
| SM-4 | `kernel/orchestrator` | bounded drive, recovery, singleflight and delivery ledger |
| Workspace deletion | `blocks/workspace` | durable retain/purge phase, ordered owner acknowledgements and restart re-drive |

All formal state transitions first become a private internal event of their sole owner. A state type may have multiple coordinated
private transition IDs, but all of them must resolve to that same owner; no cross-owner transition is legal. External Runtime results become
Orchestrator private internal events before aggregate mutation; Workspace definition admission, Run replacement intent and
Governance trace progress follow the same owner-local rule. Cross-owner I/O uses EventHub plus concrete typed contracts only.
EventHub `StateMachine` metadata means participation, while `ReducerOwner + MachineRole` fixes SM-1 to Run and SM-2～SM-4 to
Orchestrator; participant and opaque-persistence contracts never acquire reducer authority.

Stateful multi-owner coordination follows the same rule: one business owner persists the only coordinator aggregate, while each
participant changes only its own state through its own private reducer. Workspace deletion is Workspace-owned; Application sends
one intent, Run owns the two-phase `draining -> retired` fence, and Orchestrator owns the durable
`quiescing -> quiesced -> evidence-fenced -> purged` aggregate plus exact cleanup. `draining` blocks ordinary Run mutation while
allowing only the narrow terminal SM-4 opaque drain lane；Orchestrator 仅在该 lane 已完全排空后才持久化 `quiescing`，此后
所有 Runtime mutation 都 fail-closed。其 token 绑定 exact ordered Run/Runtime inventory，Run 才能 seal `retired`。
Every resource owner owns its local tombstone. Eventstream seals last, and projection is never a deletion
recovery journal. These deletion phases coordinate SM-1 and SM-4 owners but do not create a fifth Runtime state machine.

- 同一 Orchestrator owner 内需要跨 SM 协调的 provider terminal，只能先归一为一个 private fact，再由一次 aggregate CAS
  同时关闭 SM-3 request session 并应用 exact SM-2 failure；不存在可被重启观察到的双写中间态。
- LLM adapter 是 `providerSubmitted` 的唯一 producer；Orchestrator 只把 exact owner audit 归一为 private
  confirmed/released/unknown settlement，并在同一 CAS 中与 SM-3 session、适用的 SM-4 recovery budget、可选 SM-2
  terminal 一起提交。request bytes、projection 或错误文本不能证明调用发生，也不能授权重放。
- Workorchd execution admission 只同步转发并投影 Workspace owner 的 typed result；transport/context failure 不得伪造成
  owner rejection，调用方 priority、application recovery selector 和 process-local repair counter 均不属于执行状态。

## Request And Capability Facts

- provider-facing context is partitioned C0–C5; stable system/contracts, Definition, declarative capability policy and volatile
  Runtime facts have distinct cache/budget treatment;
- LLM owner interprets provider/model capabilities and `ProviderCeiling`; Orchestrator validates sealed typed facts;
- LLM candidate result严格为`Stale | Feedback | DocumentOutcome` exactly-one；complete content只属于LLM owner factual
  outcome并与framing hash/bytes一致，其他disposition禁止携带content。物理响应模式只以
  `streaming | non_streaming`记录，旧布尔字段只能派生；
- capability feasibility/readiness is separate from Definition completion: multiple operation variants for one fixed CWI do not
  create a terminal `one_of`;
- File write is create-only; existing targets require a current revision/base-hash-bound replacement operation;
- receipt success, file existence, Git revision, exit 0 or provider claim does not itself grant accepted evidence.
- evidence freshness绑定实际依赖的owner revision/snapshot/fence set；不存在拥有跨owner写authority的global world counter。
- Shell普通command只允许workspace-durable、at-most-once effect；非持久验证只走owner-confirmed managed recipe，并在
  bounded临时workspace副本中执行。recipe/check identity分离，claimed/no-receipt不重放。
- MCP feasibility只读取同一Registry revision并做静态prerequisite校验；SM-3 execute绑定完整operation、exact
  Capability/policy/catalog revision和grant，provider output只以bounded hash/ref进入receipt。
- SSH aggregate只携带opaque target/session/credential refs，owner冻结strict host-key配置并返回before/after observation；
  Web search/fetch执行DNS-pinned、proxy-disabled、redirect重验的at-most-once request，raw transport数据不进入Runtime。
- activation将Memory选择候选/排除/score/quota/budget evidence与Skill来源/content hash/byte budget一并封存；RecallTrace
  与Skill provenance只解释context选择，不参与SM-2 credit或恢复authority。

See [Request And Cache](../00-core/runtime-request-cache-note.md) and
[Capability Catalog](../10-domain-model/capability-catalog.md).

## Repository Shape

The repository has **1 Initiator + 4 Modules + 17 Blocks = 22 framework units**, plus the non-framework
`internal/services/workorch` process shell. The canonical directory/contract mapping is
[Component Contract Inventory](component-documentation-alignment.md).

The default binary mode starts TUI; `--server` starts HTTP/Web. Workorchd owns typed bootstrap, application use cases and
derived projections, but not Runtime drive or another owner's state. Formal resources are written only by their owner repository.

MCP Registry将Server、Capability和binding保存在一个revisioned aggregate中，以whole-snapshot CAS提交；Capability的
`enabled`是唯一availability字段，`source/sourceRef/revision`为owner-issued provenance。Eventstream仍只拥有bounded audit，
其持久化stream position、cursor和purge均不能成为Definition或Runtime输入。

## Unsupported Production Surfaces

The production contract excludes `StagePacket`, `NextAction`, `DispatchNextStep`, `ExecutionContextPatch`, generic
RuntimeAdapter/step dispatcher, application continuation queue, automatic successor planning, checkpoint replay, target-state
setters, map/JSON/reflect authority bridges and cross-owner service/repository calls.

Persistence and public projections expose only the one current shape. There are no read-only compatibility fields, old-layout
imports, version classifiers or upgrade reducers. Unknown/retired data fails at its repository/owner read boundary; resetting the
configured data root and creating a fresh Workspace/Run is an external development operation, not a Runtime transition.
