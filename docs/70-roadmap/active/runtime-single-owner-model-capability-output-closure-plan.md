# Runtime Single-Owner And Model-Capability Output Closure Plan

Status: active

Type: implementation-closure-plan

Last Updated: 2026-08-14

Implementation status (2026-08-14): SO-MC-01 through SO-MC-09 remain code/automated-closed. The first real-provider
schema-v6 run proved model-ceiling, complete-document recovery, sealed dispatch, File receipt and evidence advance, then
exposed a generation/declaration authority defect. SO-MC-11 closes that defect with schema v7 and immutable
`DefinitionFocuses`; focused/full/vet/fmt/boundary/release gates pass. SO-MC-10 remains live-pending until a fresh
schema-v7 run reaches terminal delivery.

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

最新部署运行已经证明：

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

Run owner may physically store the opaque Runtime aggregate, but it does not own SM-2/3/4 semantics. Cross-owner messages may carry only immutable intent, persisted fact, typed observation/receipt or delivery acknowledgement. They must not carry a target receiver state, reducer name, retry/next-action/focus/terminal decision, mutable entity, repository/service/callback, generic envelope or `map[string]any` authority.

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

The token closure upgraded Runtime aggregate schema from v5 to v6 because schema-v5 activation and stable-prefix identity persisted stage token limits. The generation closure now upgrades v6 to v7 because v6 does not persist immutable source declarations separately from semantic focus bindings. Reinterpreting a v6 focus in place would change existing authority across restart.

Schema v7 retains the v6 token rules and additionally:

- removes semantic/candidate stage token authority from activation/execution contract;
- keeps ProviderCeiling as the sole persisted model capability fact;
- keeps ExpectedDocument as the resolved per-request value;
- removes stage token configuration from stable-prefix hashing;
- retains ceiling revision, ExpectedDocument hash and request identity fences.
- persists activation-only `DefinitionFocuses` separately from SM-2 generation focus queues;
- validates semantic documents and seal output only against immutable declarations;
- lets receipt/resource-change retirement and reseal replace generation bindings without widening declarations.

Schema v6 and earlier are not implicitly migrated or reinterpreted. Resume returns `fresh_activation_required` with zero provider/capability work. Explicit restart creates a new schema-v7 Run with new activation/request lineage and no predecessor candidate, request or CWI credit.

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
| `internal/pkg/workorchruntimecontract` | schema v7; remove stage token authority; separate immutable declaration from binding generation; retain pure ProviderCeiling/ExpectedDocument validation | I/O, retry choice, state mutation, model-specific branching |
| `internal/pkg/workorchconfig` | remove stage token and mode fields/defaults/values | second token authority |
| `internal/pkg/workorchruntimepolicy` | remove stage token policy projection | runtime override of model ceiling |
| `application/workorchd/.../activationports` | stop copying token limits; seal provider selection only | runtime ceiling calculation or retry |
| `kernel/orchestrator/biz/runtime` | build ExpectedDocument only from persisted ceiling; preserve SM-3/SM-4 recovery | model table, config override, in-recovery raise |
| `kernel/orchestrator/biz` | map typed owner outcomes to private events; one dispatcher/CAS | lifecycle facade or parallel queue |
| `blocks/llm` | own model capability interpretation; enforce sealed request equality; improve prompt/schema | Runtime action/retry/terminal choice |
| `blocks/approval` | fact-kind private events and owner reducer | target-state input |
| `blocks/run/workspace/result/decision` | regression-proof owner-private lifecycle | cross-owner state write |
| capability Blocks | exact sealed execution and factual receipt | CWI/focus/dispatch/evidence mutation |
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
- Explicitly restart into a fresh schema-v7 Run.
- Inspect persisted ceiling, ExpectedDocument, request audit, candidate, dispatch, receipt, evidence and delivery.

Exit: at least one legal CandidateAttempt, SealedDispatch, exact owner receipt and binding-matched evidence exist. Correct fail-closed alone is insufficient for live closure.

### SO-MC-11 — Immutable declaration / binding-generation separation

- Persist fresh activation declarations independently from mutable SM-2 focus queues.
- Route receipt-driven retirement, CWI reopen and reseal through existing Orchestrator private events and aggregate CAS.
- Reject schema-v6 Resume before provider/capability work.
- Prove a prior `file/write` generation cannot constrain a new owner-feasible `patch/apply` generation.

Exit: code/automated closure passes without File-specific Runtime branching or a fifth state machine; fresh schema-v7 live proof remains SO-MC-10.

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

### Runtime/CWI and schema

- blocked CWI performs zero provider/owner work;
- focus changes only through SM-2 private event;
- accepted candidate and dispatch seal remain one CAS;
- target drift rejects before dispatch;
- exact receipt replay causes at most one effect and one credit;
- v5 Resume returns `fresh_activation_required` with zero provider/capability work;
- v6 restart creates new lineage and inherits no old request/CWI credit;
- terminal delivery remains `Result -> Workspace -> Run`.

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
- migrate schema-v6 or earlier aggregate in place;
- mark live closure after a fail-closed run that never reaches dispatch.

## 18. Completion Criteria

Code closure requires SO-MC-01 through SO-MC-09 plus SO-MC-11 and all automated gates. Live closure additionally requires SO-MC-10 with persisted proof that:

1. schema-v7 activation contains no stage token authority and persists immutable DefinitionFocus declarations;
2. ProviderCeiling comes from the selected model's current capability revision;
3. ExpectedDocument and actual provider request use the exact effective ceiling;
4. prompt/schema enforce complete compact bounded JSON;
5. at least one candidate, sealed dispatch, owner receipt and exact evidence advances Runtime;
6. no duplicate request, dispatch or side effect occurs across restart/recovery;
7. owner boundaries and terminal delivery order remain intact.
