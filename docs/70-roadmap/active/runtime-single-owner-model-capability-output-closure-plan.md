# Runtime Single-Owner And Model-Capability Output Closure Plan

Status: active

Type: implementation-closure-plan

Last Updated: 2026-08-14

Implementation status (2026-08-14): SO-MC-01 through SO-MC-09 and SO-MC-11 remain regression baselines. SO-MC-12
through SO-MC-17 are code/automated-closed: current development baseline rejects non-current Resume/Restart, File semantic/operation feasibility shares one
owner policy, exact binding contradictions retire through one Orchestrator-private event, equivalent authority is
fenced before provider submission, and Run persists SM-2/3/4 as opaque state behind an Orchestrator-authored lifecycle
fence. Focused/full/race/vet/fmt/boundary/isolated managed/build gates pass. SO-MC-10 remains open and requires a fresh
deployed current-baseline real-provider terminal proof after development-data cleanup; managed/mock success is not that exit.

## 1. Purpose And Authority

本文是 Workspace Runtime 单 owner 状态迁移与 LLM 完整文档输出收口的**唯一 active 实施队列**。后续代码修改、测试、部署和 live 验收必须逐项依据本文执行，不得再按单个 live 错误增加字段级修补。

权威顺序：

1. [`docs/00-core/runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)
2. [`docs/00-core/runtime-request-cache-note.md`](../../00-core/runtime-request-cache-note.md)
3. [`internal/modules/kernel/orchestrator/readme.md`](../../../internal/modules/kernel/orchestrator/readme.md)
4. `internal/pkg/workorchruntimecontract`
5. 本文定义的实施顺序与验收门禁

既有 [Runtime CWI plan](runtime-cwi-reconciliation-single-owner-closure-plan.md) 保留 CWI、focus、admission、receipt 和 evidence 的完整回归矩阵，但不再定义后续实施顺序。本文不增加第五状态机、owner、全局索引、兼容主链或领域专用 Runtime 规则。

## 2. Triggering Evidence

Earlier deployed schema-v5 execution proved:

- schema-v5 semantic contract 正确封存，SM-2 正确选择 eligible `cwi-026`；
- operations 请求连续三次精确达到 `8192` output token 后被截断；
- 三次 physical call 保持同一 authority、manifest 与 stable prefix；
- incomplete document 没有创建 CandidateAttempt、Dispatch、Receipt 或 side effect；
- Orchestrator 在恢复预算耗尽后以 `provider_protocol_failure/provider_final_output_missing` fail-closed。

这证明当前恢复与零副作用边界基本正确，同时暴露两个未关问题：

1. `8192` 来自 WorkOrch 配置和 activation，而不是当前 model 的能力声明；
2. prompt/schema 没有把完整、紧凑、有限 JSON 的边界统一传达给 LLM。

因此不能只提高一个常量，也不能只切换为“使用 provider ceiling”。必须同时关闭 token 唯一权威、ExpectedDocument identity、输出引导和 owner-private mutation 边界。

2026-08-14 的 fresh schema-v6 `run-002` 进一步证明：selected model 的 `ProviderCeiling` 与两类
ExpectedDocument 均为 29000；attempt 0 unknown-field rejection 后，attempt 1 复用 authority/manifest/stable prefix 并形成
sealed dispatch；File owner create receipt 的 target/hash 与工作区一致，assessment dispatch 已提交，evidence revision 从 0
推进到 2，且没有 duplicate side effect。随后资源变化重开同一 CWI 时，owner catalog 已允许带 base revision 的
`patch/apply`，但 semantic admission 错把上一 generation 的 `file/write` focus 当作 immutable source declaration，最终以
`runtime_contract_defect` 终止。该反例要求 schema v7 显式分离 immutable Definition declaration 与 generation binding；
不得通过扩宽 File 权限或跨 owner 指定 CWI 状态修补。

Fresh schema-v7 deployed execution then proved that declaration/generation separation alone is insufficient:

- SM-2 semantic proof feasibility marked `file/write + exact existing target` as feasible and sealed that operation
  kind into the current `CoverageEvidenceBinding`;
- the File owner's ordinary operation feasibility correctly rejected the same target because `file/write` is
  create-only and replacement requires revision-bound `patch/apply`;
- the sealed manifest therefore could not legally emit `patch/apply`, so prompt correction could not repair the
  authority contradiction;
- repeated `operation_target_exists` observations retained the same binding, catalog revision, target and failure
  signature but created fresh authority attempt-0 transactions;
- the persisted aggregate advanced beyond revision 100 and consumed more than 400k provider tokens while producing
  zero dispatch, receipt, evidence or workspace effect.

This is not an LLM-quality-only failure. It is an owner-contract and convergence defect: File emitted contradictory
facts across SM-2/SM-3, and Orchestrator treated unchanged infeasibility as new authority. The closure must prevent the
invalid seal, bound already-sealed contradictions, and make physical persistence unable to interpret another owner's
state machine.

## 3. Non-Negotiable Production Path

```text
typed Run intent
  -> Run owner SM-1 private transition
  -> RunStateChanged committed fact
  -> Orchestrator SM-4 drive
  -> Orchestrator SM-2 authority/focus convergence
  -> Orchestrator SM-3 request/candidate/sealed dispatch
  -> capability owner execution and receipt
  -> Orchestrator SM-2 evidence/terminal
  -> Orchestrator SM-4 terminal delivery
  -> Result -> Workspace -> Run owner-local transitions
```

生产路径继续禁止 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、自动 successor、projection authority 和 map-based authority rebuild。

## 4. Sole Owner And Mutation Contract

所有有状态组件统一采用：

```text
typed external intent/fact
  -> owner handler validates scope/identity/causation
  -> owner-private internal event
  -> owner-private reducer
  -> one owner repository/CAS write
  -> immutable fact/result/acknowledgement
```

| State or resource | Sole owner | Only legal mutation entry |
| --- | --- | --- |
| SM-1 Run lifecycle, lineage, replacement intent | `blocks/run` | Run-private internal event + Run repository/CAS |
| SM-2 authority, focus, CWI, evidence, terminal decision | `kernel/orchestrator` | Orchestrator-private SM-2 event + Runtime aggregate CAS |
| SM-3 request session, candidate, dispatch, receipt admission | `kernel/orchestrator` | Orchestrator-private SM-3 event + Runtime aggregate CAS |
| SM-4 drive, recovery, lease, reconciliation, delivery ledger | `kernel/orchestrator` | Orchestrator-private SM-4 event + Runtime aggregate CAS |
| Workspace definition/state/active Run slot | `blocks/workspace` | Workspace-private internal event/reducer |
| terminal Result lifecycle | `blocks/result` | Result-private internal event/reducer |
| Decision ticket/resolution | `blocks/decision` | Decision-private internal event/reducer |
| Approval lifecycle | `blocks/approval` | Approval-private internal event/reducer |
| provider request/response lifecycle | `blocks/llm` | LLM-owner request lifecycle |
| capability resource state and receipt | matching capability Block | capability-owner execution/reducer |
| projection/read model | `workorchd` / `eventstream` | derived projection only; never formal authority |

Run owner may physically store the opaque Runtime aggregate, but it does not own SM-2/3/4 semantics. Storage ownership
is not state-machine ownership: Run may compare scope, schema envelope, state revision, record hash and CAS revision,
but it must not inspect `Decision`, `SemanticPlanning`, candidate, dispatch, governance, evidence or delivery content to
decide SM-1. Cross-owner messages may carry only immutable intent, persisted fact, typed observation/receipt or delivery
acknowledgement. They must not carry a target receiver state, reducer name, retry/next-action/focus/terminal decision,
mutable entity, repository/service/callback, generic envelope or `map[string]any` authority.

For every state machine, the only legal trigger is an owner-private internal event. EventHub commands/results are never
state transitions themselves:

```text
cross-owner typed fact
  -> receiver owner validates scope, identity, causation and freshness
  -> receiver owner creates a private internal event
  -> receiver owner reducer selects the transition
  -> receiver owner commits one CAS
  -> receiver owner emits an immutable fact/ack
```

SM-2, SM-3 and SM-4 remain subdomains of one Orchestrator aggregate and must not be split into separate framework
Modules. They share one owner, one private dispatcher and one transition coordinator; file/package separation inside
the Orchestrator is implementation organization only.

## 5. Model Capability Is The Sole Token Authority

### 5.1 Canonical source

The only output-token authority is the LLM owner's typed declaration for the exact selected provider/model:

```text
ProviderModelCatalog
  -> ValidateProviderInferenceSelection
  -> AcquireProviderCeiling
  -> persisted ProviderCeiling
```

`ProviderCeiling` must bind at least provider/model selection, context window, provider maximum output, effective maximum output, maximum input, maximum bytes, capability revision and configuration revision.

LLM owner alone interprets model capabilities. Orchestrator must not branch on model name, maintain a parallel model table, infer a ceiling from history, or apply a configuration override. Application, Web and TUI select provider/model/reasoning effort only.

### 5.2 Remove configuration token authority

Delete all configuration and activation authority for:

- `outputTokenBudgetMode`;
- `semanticPlanMaxOutputTokens`;
- `candidateMaxOutputTokens`.

Remove the corresponding fields/defaults/projections from `workorchconfig`, `workorchruntimepolicy`, TOML, activation assembly, `RuntimeExecutionContract`, definition hashing and UI/read models where present.

Keep document/effect shape limits such as `maxPatchContentBytes`, `maxGraphBytes`, `maxGraphFileWrites`, `maxArtifactChunkBytes` and `maxDocumentRecoveryAttempts`. These constrain admissible document/effect shape; they are not a second provider-token authority.

## 6. ExpectedDocument Is The Sole Resolved Request Budget

Both semantic-plan and operations requests must follow:

```text
persisted ProviderCeiling
  -> Orchestrator creates RuntimeExpectedDocumentContract
  -> ExpectedDocument.MaxOutputTokens = ProviderCeiling.EffectiveMaxOutputTokens
  -> request session is persisted
  -> LLM owner receives the sealed ExpectedDocument
  -> provider receives the exact sealed max_output_tokens
```

The following values must be equal before provider submission:

```text
ExpectedDocument.MaxOutputTokens
= RequestRuntimeCandidateCommand.ExpectedDocument.MaxOutputTokens
= LLM ProviderRequest.MaxOutputTokens
= provider-facing max_output_tokens
```

A mismatch is a pre-submission typed context-integrity/contract failure. Resume and recovery reuse the persisted value; neither re-reads configuration nor silently changes the request budget.

## 7. Provider Ceiling Acquisition And Invalidation

Before any semantic-plan or operations request, SM-2/SM-4 must prove that the ceiling:

- binds the current workspace/run/authority snapshot and sealed provider selection;
- has current capability/configuration revisions;
- has positive effective output, input and byte limits;
- satisfies input + output <= context window;
- can contain the protected request sections without dropping P0-P4.

Missing ceiling enters typed prerequisite acquisition. Stale ceiling is first persisted as a fact, retires only the exact dependent request generation, reacquires the LLM-owner ceiling, and creates a new ExpectedDocument identity. It does not consume candidate attempt, execute an operation or create Decision/Approval. Unsupported capability fails before provider submission with zero attempt, dispatch and effect.

## 8. Output Guidance And Schema Closure

Using the full effective model ceiling does not authorize verbose or unbounded output. Semantic-plan and operations requests must tell the model:

- output exactly one complete JSON object;
- no Markdown fence, explanation, preface or suffix;
- do not repeat Goal, DoD, manifest or input context;
- use compact JSON;
- emit only schema fields and manifest-authorized operations;
- do not add unknown fields or unrequested operations;
- do not create proof, binding, credit, retry or transition authority;
- finish the JSON document in one response.

The same sealed manifest must generate renderer constraints, JSON Schema, strict decoder and evaluator admission. Schema must apply `additionalProperties: false`, bounded operation/dependency/target arrays, bounded strings/content, exact kind/variant/binding vocabularies and applicable patch/content limits.

Provider token ceiling and document shape remain orthogonal:

```text
provider ceiling -> how many tokens the model can produce
schema/manifest limits -> what Runtime is willing to admit
```

## 9. Truncation And Recovery Remain Orchestrator-Owned

LLM owner returns only typed provider facts: complete, incomplete/output-limit, empty, invalid, transport, authentication or configuration outcome. It never chooses retry, model switch, token increase, CandidateAttempt or Runtime terminal.

```text
incomplete/output-limit fact
  -> Orchestrator-private request outcome event
  -> persisted SM-4 request session
  -> bounded whole-document retry of the same ExpectedDocument
  -> complete document or typed terminal
```

Mandatory invariants:

- incomplete document creates no CandidateAttempt or Dispatch;
- partial output is never concatenated into authority;
- each physical call has a unique PhysicalCallID under one logical identity;
- duplicate outcome consumes no revision or budget;
- stale response only enters diagnostic lineage;
- recovery never raises MaxOutputTokens;
- recovery exhaustion produces typed fail-closed with zero side effect.

## 10. Runtime Schema Boundary

WorkOrch is in active development. The completed token, generation and owner-convergence closures are consolidated into
one current development baseline instead of being maintained as a v5-v8 migration chain. Old version labels in triggering
evidence are historical defect identifiers only; no production reader, reducer or Restart path supports them.

Current baseline:

- removes semantic/candidate stage token authority from activation/execution contract;
- keeps ProviderCeiling as the sole persisted model capability fact;
- keeps ExpectedDocument as the resolved per-request value;
- removes stage token configuration from stable-prefix hashing;
- retains ceiling revision, ExpectedDocument hash and request identity fences.
- persists activation-only `DefinitionFocuses` separately from SM-2 generation focus queues;
- validates semantic documents and seal output only against immutable declarations;
- lets receipt/resource-change retirement and reseal replace generation bindings without widening declarations.

Current baseline additionally:

- bind every sealed semantic feasibility fact to a canonical owner policy revision covering kind, variant, target
  policy and current resource shape;
- persist enough owner-convergence lineage to prove whether a later infeasibility contains a new authority input;
- forbid a new AuthoritySnapshot when binding hash, owner revision, failure signature and targets are unchanged;
- persist an opaque lifecycle fence/hash that Run can compare without decoding SM-2/3/4 fields;
- retain binding retirement lineage for a contradiction discovered after seal;
- reject every non-current Resume as `fresh_activation_required` before provider/capability work.

Explicit Restart against a non-current predecessor is rejected by the Run owner as `data_reset_required` before a
ReplacementIntent is persisted or Orchestrator is invoked. No production migration may patch old JSON, synthesize
`patch/apply`, copy prior candidate/receipt/evidence credit, or reconstruct authority through a map/JSON compatibility
bridge. Development data must be cleared before a fresh current-baseline activation.

## 11. Approval Owner Closure

Approval private lifecycle input must express observed intent/fact, not a target state:

```text
pending + approval_granted_observed  -> approved
pending + approval_rejected_observed -> rejected
```

Remove `Target common.ApprovalState` from the private event. External commands cannot submit arbitrary Approval state. Decision/Orchestrator cannot write Approval repository, and Approval resolution cannot directly Resume Runtime.

## 12. Component Closure Matrix

| Component | Required change | Forbidden result |
| --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | current baseline schema; remove stage token authority; separate immutable declaration from binding generation; retain pure ProviderCeiling/ExpectedDocument and owner/fence identity validation | I/O, retry choice, state mutation, model-specific branching |
| `internal/pkg/workorchconfig` | remove stage token and mode fields/defaults/values | second token authority |
| `internal/pkg/workorchruntimepolicy` | remove stage token policy projection | runtime override of model ceiling |
| `application/workorchd/.../activationports` | stop copying token limits; seal provider selection only | runtime ceiling calculation or retry |
| `kernel/orchestrator/biz/runtime` | build ExpectedDocument only from persisted ceiling; preserve SM-3/SM-4 recovery | model table, config override, in-recovery raise |
| `kernel/orchestrator/biz/runtime` | own equivalent-authority fence, binding contradiction retirement, semantic replan and bounded terminal through private events | unchanged owner infeasibility opening another authority attempt |
| `kernel/orchestrator/biz` | map typed owner outcomes to private events; one dispatcher/CAS | lifecycle facade, direct reducer call or parallel queue |
| `blocks/llm` | own model capability interpretation; enforce sealed request equality; improve prompt/schema | Runtime action/retry/terminal choice |
| `blocks/approval` | fact-kind private events and owner reducer | target-state input |
| `blocks/run` | SM-1 private reducer; opaque Runtime record CAS and lifecycle-fence comparison only | decoding SM-2/3/4 to select SM-1 |
| `blocks/workspace/result/decision` | regression-proof owner-private lifecycle | cross-owner state write or Runtime interpretation |
| `blocks/file` | one owner-local policy for semantic and operation feasibility | contradictory feasibility or Runtime next-action choice |
| capability Blocks | exact sealed execution and factual receipt | CWI/focus/dispatch/evidence/retry mutation |
| Eventstream/Web/TUI/CLI | intent and projection only | Resume/Drive from projection or UI state |
| boundary verifier | scan state and token authority bypasses | global event/state registry |

## 13. Ordered Implementation Queue

### SO-MC-01 — Clean interrupted implementation

- Preserve the Approval fact-event direction.
- Remove unverified `OutputTokenBudgetMode` and provider/fixed resolver changes.
- Do not discard unrelated user changes.

Exit: working diff matches this plan and introduces no configuration mode.

### SO-MC-02 — Schema-v6 contract

- Increment current Runtime schema to v6.
- Remove stage token fields from execution contract, clone/equality/hash/serialization/tests.
- Add v5 `fresh_activation_required` regression with zero work.

Exit: no in-place reinterpretation of a v5 request session.

### SO-MC-03 — Remove configuration and activation authority

- Delete semantic/candidate token fields and defaults from config/policy/TOML.
- Remove activation assembly and projection fields.
- Ensure ordinary users still provide only template, Goal, DoD and workspace directory plus provider selection where applicable.

Exit: repository scan finds no configuration-stage token authority.

### SO-MC-04 — LLM-owner capability contract

- Validate catalog and selected model capability revisions.
- Produce one typed ProviderCeiling.
- Prove effective ceiling integrity and request-scope binding.

Exit: Orchestrator consumes a detached fact and contains no model-name logic.

### SO-MC-05 — ExpectedDocument single resolution

- Use `ProviderCeiling.EffectiveMaxOutputTokens` for semantic and operations documents.
- Persist before provider call.
- Verify command, provider request and audit equality.

Exit: no request can submit with a different max output value.

### SO-MC-06 — Prompt/schema compact-output contract

- Add complete compact JSON instructions.
- Derive bounded schema from the sealed manifest.
- Keep renderer/schema/decoder/evaluator vocabulary identical.

Exit: prose, fence, unknown fields, excess arrays/content and repeated authority are rejected or prevented without weakening admission.

### SO-MC-07 — SM-4 truncation/restart matrix

- Cover output-limit, duplicate, stale response, crash boundaries and recovery exhaustion.
- Assert same ExpectedDocument, no raise, no concatenation, zero attempt/dispatch/effect until complete.

Exit: recovery remains one Orchestrator-owned persisted transaction.

### SO-MC-08 — Approval and all-owner boundary gate

- Complete Approval fact-event tests.
- Scan production DTOs and mutation sites for target-state input and cross-owner writes.
- Re-run Run/Workspace/Result/Decision/capability/projection owner regressions.

Exit: every mutable state has one owner-private dispatcher.

### SO-MC-09 — Combined verification and documentation

- Update core contract, request/cache note, component readmes and current-state indexes.
- Run focused/full/race/vet/fmt/boundary/managed gates.

Exit: all automated gates pass and active docs contain one implementation queue.

### SO-MC-10 — Fresh deployed live closure

- Build and deploy the verified binary.
- After clearing incompatible development data, start a fresh current-baseline Run.
- Inspect persisted ceiling, ExpectedDocument, request audit, candidate, dispatch, receipt, evidence and delivery.
- Assert provider calls remain bounded by actual document recovery/correction and do not grow by recreating an
  equivalent authority.

Exit: at least one legal CandidateAttempt, SealedDispatch, exact owner receipt and binding-matched evidence exist. Correct fail-closed alone is insufficient for live closure.

### SO-MC-11 — Immutable declaration / binding-generation separation

- Persist fresh activation declarations independently from mutable SM-2 focus queues.
- Route receipt-driven retirement, CWI reopen and reseal through existing Orchestrator private events and aggregate CAS.
- Reject non-current Resume before provider/capability work.
- Prove a prior `file/write` generation cannot constrain a new owner-feasible `patch/apply` generation.

Exit: code/automated closure passes without File-specific Runtime branching or a fifth state machine; historical
results remain regression baselines and the fresh current-baseline live proof remains SO-MC-10.

### Reopened execution order

The current implementation order is strictly:

```text
SO-MC-12 -> SO-MC-13 -> SO-MC-14 -> SO-MC-15 -> SO-MC-16 -> SO-MC-17 -> SO-MC-10
```

SO-MC-01 through SO-MC-09 and SO-MC-11 remain regression baselines. They are not rerun as design phases, but every
affected regression remains mandatory. SO-MC-10 must start only after incompatible development data is cleared and
SO-MC-17 passes.

### SO-MC-12 — Current-baseline owner and transition envelope

Status: code/automated-closed (2026-08-14).

Code scope:

- `internal/pkg/workorchruntimecontract`;
- `internal/pkg/eventcontract`;
- `internal/modules/kernel/orchestrator/biz/runtime/internal_event*`;
- Run Runtime-state EventHub contracts.

Work:

- reset the Runtime schema to current development baseline; make non-current Resume return `fresh_activation_required` and
  non-current Restart return `data_reset_required`, both with zero provider, capability, dispatch or projection-driven
  recovery work;
- define pure typed values for owner policy revision, equivalent-authority input identity, binding contradiction
  identity and opaque lifecycle fence;
- keep all retry/replan/terminal selection out of the contract package;
- classify `StoreRuntimeStateRecord` as opaque persistence/CAS, not an SM-1 transition;
- document an exhaustive owner matrix for SM-1..SM-4 and resource state machines without adding a runtime registry.

Required tests:

- non-current Resume/Restart performs zero owner work and persists no ReplacementIntent;
- current-baseline clone/normalize/hash/validate preserves all new identities across restart;
- an EventHub fact cannot be classified as an owner-private transition;
- every Orchestrator mutation kind is exhaustively classified and handled only by `ApplyInternalEvent`.

Exit: current-baseline identity can express the later fixes without target state, mutable handles, map authority or policy
selection outside the owner.

### SO-MC-13 — File owner feasibility unification

Status: code/automated-closed (2026-08-14).

Code scope:

- `internal/modules/blocks/file/biz/owner.go`;
- `internal/modules/blocks/file/biz/file_runtime_adapter.go`;
- `internal/modules/blocks/file/biz/feasibility_category.go`;
- `internal/modules/blocks/file/pkg/events`.

Work:

- extract one File-owner pure policy that evaluates operation kind, variant, target policy, target existence/type,
  catalog revision and required base revision;
- make semantic proof feasibility and ordinary operation feasibility consume that same policy;
- return `infeasible` for `file/write + existing target` during SM-2 semantic feasibility;
- keep `patch/apply` replacement feasible only with an owner-observed base hash and supported variant;
- return only canonical typed observations. Diagnostics may state the violated owner rule but must not select Runtime
  retry, correction, binding retirement or next action.

Required tests:

- missing exact target, existing regular target, directory/symlink, prospective target, target drift and stale catalog;
- `file/write` create succeeds only for an admissible missing target;
- `file/write` existing target is identically infeasible in semantic and operation probes;
- `patch/apply` text replacement/file-write/unified-diff variants enforce matching base hash and target inventory;
- duplicate probes are side-effect free and return stable policy revision/diagnostic identity.

Exit: the File owner cannot emit `feasible` in SM-2 for an operation it deterministically rejects in SM-3 under the
same resource revision.

### SO-MC-14 — Orchestrator binding contradiction retirement

Status: code/automated-closed (2026-08-14).

Code scope:

- `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go`;
- `internal/modules/kernel/orchestrator/biz/runtime/cwi_ledger.go`;
- `internal/modules/kernel/orchestrator/biz/runtime/prerequisite_freshness.go`;
- Orchestrator private internal-event vocabulary and reducer tests.

Work:

- introduce or specialize an Orchestrator-private `semantic_binding_contradiction_observed` fact;
- map a capability owner's typed contradiction to that private event only after scope, binding, catalog, target and
  observation identity validation;
- atomically invalidate the pending candidate, retire the exact binding with no-effect lineage, clear only the
  dependent semantic generation, and return SM-2 to bounded semantic planning;
- allow the next plan to select any kind/variant already authorized by the immutable Definition declaration and fresh
  owner catalog, including revision-bound `patch/apply`;
- if the contradiction cannot be tied to one current binding, fail closed as `runtime_contract_defect` without another
  provider request.

Required tests:

- restart at each CAS boundary between observation, retirement and replan;
- duplicate/stale contradiction is a no-op;
- exactly one binding is retired and CWI remains open;
- no dispatch, receipt, evidence, governance request or repair debt is created by retirement;
- semantic correction/replan exhaustion reaches a typed terminal;
- old generation `file/write` cannot constrain the newly sealed `patch/apply` generation.

Exit: an already-sealed owner contradiction has one owner-private recovery path and cannot be repaired by changing
state in File, Run, Application or projection.

### SO-MC-15 — Equivalent-authority and no-progress fence

Status: code/automated-closed (2026-08-14).

Code scope:

- Orchestrator authority compiler/reducer;
- candidate attempt/request session ledger;
- convergence progress signature and recovery tests.

Work:

- compute the next-authority input identity from Definition/focus/binding hashes, evidence revision, capability/catalog
  revisions, prerequisite freshness and exact owner observation lineage;
- prohibit a fresh AuthoritySnapshot when that identity is unchanged;
- treat revision-only bookkeeping, lease generation, request ID, physical call ID and repeated failure persistence as
  non-progress;
- route a repeated same-input infeasibility to binding contradiction retirement or a typed bounded terminal;
- retain CandidateAttemptBudget as attempt 0/1 within one authority; do not add a new budget that merely permits more
  equivalent authorities.

Required tests:

- same binding/revision/signature/targets cannot open a second equivalent authority;
- a genuinely changed catalog, target revision, evidence revision or binding lineage can open one new authority;
- duplicate and restart preserve the same judgment;
- provider calls remain zero after the no-progress fence chooses terminal;
- progress signature does not count CAS retry, lease or audit-only revision as forward progress.

Exit: every new authority has a provably new owner/authority input; otherwise the aggregate retires the invalid binding
or terminates.

### SO-MC-16 — Run opaque persistence and SM-1 lifecycle fence

Status: code/automated-closed (2026-08-14).

Code scope:

- `internal/modules/blocks/run/biz`;
- `internal/modules/blocks/run/pkg/events`;
- Orchestrator Run-state repository/event adapter;
- terminal/governance delivery contracts.

Work:

- keep Run as physical CAS owner for the Runtime record but restrict validation to workspace/run scope, schema envelope,
  Runtime state revision/ref/hash, bootstrap identity and physical CAS revision;
- persist Orchestrator-authored terminal/governance lifecycle fence identities beside the opaque record;
- make Run compare typed fence identities without inspecting Runtime `Decision`, `SemanticPlanning`, candidate,
  dispatch, governance or evidence collections;
- continue converting accepted cross-owner facts into Run-private lifecycle events before the SM-1 reducer;
- ensure terminal outcome mapping remains Run-owned and external inputs never carry a target `RunState`.

Required tests:

- Run cannot derive terminal/governance state by decoding Runtime fields;
- forged, stale and duplicate lifecycle fences are rejected/no-op as appropriate;
- physical Run-record CAS rebase does not alter Runtime semantic revision;
- Runtime aggregate CAS mismatch returns a typed persistence fact to Orchestrator SM-4;
- terminal delivery remains idempotent across Result -> Workspace -> Run ordering and restart.

Exit: Run owns SM-1 and physical storage, while Orchestrator alone owns every SM-2/3/4 semantic transition.

### SO-MC-17 — Repository-wide owner boundary and combined gates

Status: code/automated-closed (2026-08-14).

Code scope:

- boundary verifier;
- `blocks/workspace`, `result`, `decision`, `approval`, `llm`, capability Blocks and `eventstream` regressions;
- `application/workorchd`, TUI/CLI and `internal/services/workorch` production scans;
- core/current docs.

Work:

- reject production direct reducer/repository calls across owner boundaries;
- reject EventHub DTOs carrying receiver target state, reducer/action/retry choice or mutable handles;
- reject non-Orchestrator writes of CWI, binding, authority, candidate, dispatch, evidence, terminal or drive lease;
- reject Run decoding of SM-2/3/4 content for lifecycle selection;
- prove application/scheduler issue only typed Start/Resume/Drive intent and projection remains read-only;
- update the four-state-machine transition, stale/duplicate, idempotency/restart and budget/guard matrices together.

Exit commands:

```bash
go test ./internal/pkg/workorchruntimecontract/... -count=1
go test ./internal/modules/blocks/file/... -count=1
go test ./internal/modules/blocks/run/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
make fmt-check
make vet
make test
make test-race
make verify-boundary-contract
make verify-ci-mvp-file
make build
```

Exit: all gates pass, production scans show one owner-private transition entry per machine, and no managed/mock result is
used as the deployed live exit.

## 14. Verification Matrix

### Model capability and request identity

- model declares 8192 -> exact 8192 request;
- model declares 29000 -> exact 29000 request;
- context pressure lowers effective ceiling -> exact lowered value;
- ceiling missing/invalid/stale -> zero provider submission until typed convergence;
- capability/configuration revision drift retires exact old generation;
- no Orchestrator branch on provider/model name;
- ExpectedDocument, EventHub command, provider request and audit agree.

### Prompt/schema

- one compact JSON object instruction is present;
- Markdown/prose/repeated context is forbidden;
- arrays, strings, targets, bindings and content are bounded;
- manifest, schema, decoder and evaluator have identical authorization vocabulary;
- stable prefix does not drift within the same activation/recovery identity.

### Recovery

- output limit creates no attempt/dispatch;
- duplicate incomplete fact is a no-op;
- stale physical response cannot overwrite current session;
- crash after submission reloads persisted session;
- all retries use the same ExpectedDocument and token value;
- exhaustion forms typed terminal with zero effect.

### Owner boundaries

- Approval input carries fact kind, not state target;
- Run input cannot specify terminal state;
- Workspace slot cannot be stolen by another Run;
- Result derives lifecycle only from sealed outcome;
- Decision resolution cannot directly Resume;
- projection cannot trigger formal transition;
- stale/duplicate input returns through the same owner reducer;
- File write cannot overwrite existing target; patch requires matching owner-observed base hash.
- File semantic and operation feasibility agree for the same policy/resource revision;
- unchanged owner infeasibility cannot mint another authority;
- Run compares an opaque lifecycle fence and does not decode Orchestrator state.

### Runtime/CWI and schema

- blocked CWI performs zero provider/owner work;
- focus changes only through SM-2 private event;
- accepted candidate and dispatch seal remain one CAS;
- target drift rejects before dispatch;
- exact receipt replay causes at most one effect and one credit;
- v5 Resume returns `fresh_activation_required` with zero provider/capability work;
- v6 restart creates new lineage and inherits no old request/CWI credit;
- terminal delivery remains `Result -> Workspace -> Run`.
- v7 Resume returns `fresh_activation_required` with zero provider/capability work;
- a v8 sealed-binding contradiction retires exactly one binding or reaches a typed terminal;
- each fresh authority proves a changed authority-input identity.

## 15. Verification Commands

```bash
go test ./internal/pkg/workorchruntimecontract/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/approval/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1

make fmt-check
make vet
make test
make test-race
make verify-boundary-contract
make verify-ci-mvp-file
make build
```

## 16. Boundary Scanner Requirements

`make verify-boundary-contract` must reject production occurrences that create a bypass, including:

- lifecycle command/internal fact carrying a receiver-owner target state;
- `next.State = command.*`-style target-state mutation;
- Application/Orchestrator/projection direct writes to another owner repository/service;
- projection-driven `Drive`/`Resume`;
- LLM-owner retry, terminal or token-raise decision;
- configuration/activation stage max-output token authority;
- non-Orchestrator CWI/binding/dispatch mutation;
- capability semantic/operation feasibility divergence for the same owner policy revision;
- equivalent AuthoritySnapshot creation without changed authority inputs;
- Run lifecycle code decoding Orchestrator `Decision`, semantic, candidate, dispatch, governance or evidence state;
- `RemainingFocuses[0]` as execution cursor;
- map/JSON/reflect authority bridge or forbidden Runtime surface.

Scanner rules must remain narrowly structural and production-scoped; they must not introduce a global event or state registry.

## 17. Explicit Non-Solutions

The following do not close this plan:

- replace 8192 with another fixed constant;
- add `outputTokenBudgetMode` or a user-visible token selector;
- let Orchestrator interpret model brands;
- automatically raise tokens after an incomplete response;
- concatenate partial provider output;
- ask Application/UI to Resume after truncation;
- weaken strict schema, semantic seal, target policy or File overwrite protection;
- migrate any non-current aggregate in place;
- mark live closure after a fail-closed run that never reaches dispatch;
- let File choose `patch/apply`, correction, retry or binding retirement for Orchestrator;
- parse `operation_target_exists` message text to select a Runtime action;
- add a larger retry/authority budget instead of preventing equivalent-authority recreation;
- split SM-2, SM-3 and SM-4 into different framework owners;
- retain an internal version-upgrade chain during development or reinterpret old sealed authority in place.

## 18. Completion Criteria

Code closure requires SO-MC-01 through SO-MC-09, SO-MC-11 through SO-MC-17 and all automated gates. Live closure
additionally requires SO-MC-10 with a fresh current-baseline persisted proof after development-data cleanup that:

1. current-baseline activation contains no stage token authority, persists immutable DefinitionFocus declarations and seals
   owner-policy/authority-input/lifecycle-fence identities;
2. ProviderCeiling comes from the selected model's current capability revision;
3. ExpectedDocument and actual provider request use the exact effective ceiling;
4. prompt/schema enforce complete compact bounded JSON;
5. at least one candidate, sealed dispatch, owner receipt and exact evidence advances Runtime;
6. no duplicate request, dispatch or side effect occurs across restart/recovery;
7. owner boundaries and terminal delivery order remain intact;
8. semantic and operation feasibility agree for the same File owner revision;
9. every new authority proves a changed input identity and no equivalent-authority loop occurs;
10. Run reaches SM-1 transitions only through Run-private internal events without decoding SM-2/3/4 authority.
