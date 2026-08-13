# Runtime CWI Canonical Ledger Single-Owner Closure Plan

Status: active

Type: implementation-closure-plan

Last Updated: 2026-08-13

## 1. Purpose And Authority

This is the sole active implementation queue for Runtime CWI identity, semantic-generation retirement, focus
projection, reconciliation generation, and semantic target-policy closure. It is derived from the current Runtime
contract and the persisted `run-073`, `run-077`, and `run-078` counterexamples. The prior plan was moved to
[`90-archive/superseded`](../../90-archive/superseded/runtime-cwi-reconciliation-single-owner-closure-plan-pre-cwi-ledger-2026-08-11.md);
its historical RSO status claims are not implementation authority.

Authority order:

1. [`docs/00-core/runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)
2. [`internal/modules/kernel/orchestrator/readme.md`](../../../internal/modules/kernel/orchestrator/readme.md)
3. `internal/pkg/workorchruntimecontract`
4. This plan, which defines only the remaining implementation work

This plan does not add a Runtime path, state machine, global registry, compatibility fallback, or domain-specific
rule. Production remains `DriveRuntime + Orchestrator reducer + sealed dispatch`.

## 2. Triggering Counterexample

The fresh workspace-full restart `ws-a8bada88e7b1b5f8/run-073` failed with:

```text
runtime_contract_defect: reconciliation contract duplicates coverage item "cwi-043"
```

Before the terminal failure, the same Run persisted two accepted File `patch/apply` receipts for `cwi-043`, both
targeting `cmd/lsp-bridge/main.go`, under two distinct semantic bindings. The aggregate also retained:

- a completed focus for `cwi-043` with binding `semantic-binding-8cc18e31867729bb343c`;
- a new pending focus for the same CWI;
- a current reconciliation contract with a newer binding set; and
- a reconciliation request still bound to an older binding set.

The failure proves that structural owner boundaries are insufficient: the Orchestrator remains the owner, but it lacks
a single persisted CWI identity rule across semantic generations. The compiler detects duplicate coverage only after a
second durable effect has already been committed.

### 2.1 Earlier invalid transition exposed by `run-077`

`run-077` confirms the defect exists before the late duplicate compiler failure. Its owner boundaries and terminal
delivery were correct:

- SM-1 followed `created -> active -> failed` under `blocks/run`;
- one File durable effect was accepted for `cwi-043` under execution key
  `exec/ws-a8bada88e7b1b5f8/run-077/dispatch-5451d9e5755e9eb98556/op-001` and fence `1`;
- terminal delivery followed `Result -> Workspace -> Run`; and
- the process stopped driving without a retry or CPU spin.

The persisted CWI row nevertheless remained `open/bound`, had two binding-lineage entries, and had no reopen lineage.
The active implementation allows this sequence:

```text
accepted durable owner receipt/effect
  -> catalog generation invalidated
  -> retireSemanticPlanningGeneration
  -> retireRuntimeCWIBindingsForManifest
  -> CurrentBindingRef cleared and bound changed to planning
  -> later semantic seal installs a successor binding
```

This is invalid even when the CWI has not yet reached `settled`. An accepted durable effect is an irreversible execution
fact. Catalog invalidation may invalidate future planning authority, but it cannot erase or replace the binding that
authorized an already accepted durable effect. A successor binding requires an explicit SM-2 reopen transition with
typed lineage in the same aggregate CAS.

### 2.2 Root cause and closure boundary

The root cause is not a File, LLM, Run, Workspace, Result, TUI, or projection state mutation. It is an Orchestrator-local
authority collision: semantic-generation lifecycle currently mutates CWI binding lifecycle as a side effect. Therefore
the closure must remove that coupling rather than add a receipt-only fence or another cross-component manager.

The following concepts are distinct and must have distinct private transitions:

| Concept | May retire | Must preserve | May authorize successor binding |
| --- | --- | --- | --- |
| catalog generation invalidation | manifest, dependent observations, provider sessions, unsealed planning request | canonical CWI row, current binding, accepted receipt/effect/evidence lineage, completed focus | no |
| binding retirement | a current binding that has produced no accepted receipt/effect/evidence | full retired-binding audit and exact typed cause | only after the retirement CAS makes the CWI eligible |
| explicit CWI reopen | current credit eligibility and completed/current focus projection for the exact CWI | prior binding, receipt, effect and evidence audit | yes, after the reopen CAS records typed lineage |

There is no implicit `bound -> planning` transition. In particular, catalog revision drift, provider rejection, process
restart, reconciliation retry, projection lag, or a changed LLM proposal is not a reopen or binding-retirement cause.

### 2.3 Pre-binding target-policy contradiction exposed by `run-078`

The fresh workspace-full restart `ws-a8bada88e7b1b5f8/run-078` did not reach binding seal or operation dispatch. Its
semantic proof batch selected `cwi-043` and proposed:

```text
owner=blocks/file
operation=patch/apply
variant=unified_diff
policy=prospective_owner_target
prospectiveTargetRef=cmd/lsp-bridge/main.go
```

The same sealed File admissibility catalog already contained `cmd/lsp-bridge/main.go` in its existing `TargetRefs`.
The complete semantic document evaluator accepted the proposal because it checked the prospective selector and
constraint vocabulary but did not reject the contradiction between `ProspectiveTargetRef` and the same catalog's
existing target set. File then correctly returned the typed feasibility fact `prospective_target_exists`; SM-2
persisted that observation and immediately sealed `semantic_contract_failed`. The terminal delivery order remained
`Result -> Workspace -> Run`, no owner operation ran, and no durable effect occurred.

This is not a File owner defect and must not be fixed by weakening File feasibility. It is a two-part Orchestrator
contract gap before binding seal:

1. the pure SM-3 semantic evaluator does not consume all target-policy facts already present in the immutable owner
   catalog; and
2. the contradiction therefore escapes the only legal correction point and reaches SM-2 as an owner `infeasible`
   fact, where fail-closed terminalization is correct but too late.

The closure is owner-neutral. It must express only the invariant that one target cannot be both an existing catalog
member and a prospective target in the same sealed generation. It must not mention File paths, software-development
templates, `main.go`, or infer which alternative policy the provider should choose.

## 3. Final Ownership Model

| State or resource | Sole owner | Legal mutation entry | Explicitly not owned here |
| --- | --- | --- | --- |
| SM-1 Run lifecycle and lineage | `blocks/run` | Run-private internal event | focus, binding, candidate, coverage, reconciliation |
| SM-2 CWI authority, focus convergence, coverage credit, binding lineage and reopen | `kernel/orchestrator` | Orchestrator private internal event plus one Runtime aggregate CAS | capability resource state, Run lifecycle |
| SM-3 semantic request, candidate, sealed dispatch and receipt/evidence transaction | `kernel/orchestrator` | Orchestrator private internal event plus one Runtime aggregate CAS | next action selection by LLM or capability owner |
| SM-4 drive, recovery, reconciliation request scheduling and terminal delivery ledger | `kernel/orchestrator` | Orchestrator private internal event plus one Runtime aggregate CAS | CWI credit, focus selection, Run/Workspace/Result mutation |
| Workspace definition and active slot | `blocks/workspace` | Workspace-private internal event | Runtime aggregate authority |
| Result lifecycle | `blocks/result` | Result-private internal event | acceptance adjudication |
| File, Git, Shell, MCP, SSH and Web resource facts | corresponding capability Block | capability-private internal event | CWI state, binding, retry, terminal |
| LLM provider request/response | `blocks/llm` | LLM-private internal event | Runtime action, CWI credit, correction or terminal |

Cross-owner interaction remains a concrete typed EventHub command/result. A producer can report an immutable fact;
only the receiving owner turns it into an owner-local internal event. No command may carry a target Runtime state,
full owner entity, repository, service facade, `map[string]any`, or generic envelope.

## 4. Canonical CWI Representation

### 4.1 Do not add a second CWI model

`RuntimeFacts.Coverage` is already the persisted CWI set. It must become the one canonical CWI ledger rather than be
supplemented by a parallel global index. Extend its existing CWI row with a bounded, append-only binding lineage and
one current binding reference:

| Value | Meaning | Source of truth |
| --- | --- | --- |
| CWI ID and Definition seed identity | immutable CWI identity for this Run | Definition seed / existing coverage row |
| convergence state | `unbound`, `planning`, `bound`, `settled`, or `reopened` | SM-2 reducer |
| current binding ref | exact binding ID/hash and semantic generation currently authorized for the CWI | SM-2 seal |
| binding lineage | prior binding refs, receipt/evidence refs, and explicit reopen cause | SM-2 only |
| focus ref | stable focus identity currently representing the CWI, if non-terminal | SM-2 projection |

The complete `CoverageEvidenceBinding` stays in the existing sealed focus/contract structures. The CWI ledger stores
only references and lifecycle lineage, avoiding a duplicate full proof model. Any temporary lookup map is local to a
single reducer/validator invocation and is never persisted, exported, or made global.

### 4.2 Canonical invariants

The aggregate validator must reject all of the following before CAS:

1. a CWI appearing in more than one of current, remaining, and completed focus queues;
2. a non-reopened CWI receiving a second current binding in the same Run;
3. a focus binding that is absent from, or differs from, the CWI's current binding ref;
4. a settled CWI returned to planning only because an owner catalog changed;
5. a reopened CWI without a typed receipt/observation lineage and prior binding ref;
6. a reconciliation contract containing a binding not equal to the canonical current binding for that CWI;
7. an active reconciliation request whose definition revision, binding set hash, or binding refs differ from the
   current reconciliation contract;
8. a current request or batch retained after its binding set has been superseded;
9. a `semantic-pending/semantic-pending/...` recursive focus ID;
10. a receipt/evidence that grants credit to a CWI whose current binding does not exactly match its sealed dispatch;
11. a retired semantic generation being used as current planning authority; and
12. a legacy aggregate without the canonical CWI fields entering DriveRuntime;
13. a CWI with an accepted durable receipt/effect losing or replacing its authorizing binding without matching reopen
    lineage in the same transition;
14. a binding retirement without a typed cause, or after any accepted receipt/effect/evidence for that binding; and
15. a successor binding whose predecessor lineage is neither explicitly retired before effect nor explicitly reopened
    after effect.

The validator may inspect only aggregate values. It must not query a capability owner, infer from files, or choose a
recovery action.

### 4.3 Stable focus projection

Current Runtime policy is one focus per CWI. Pending focus IDs must be generated solely as
`semantic-pending/<cwi-id>`, not from a prior source focus ID. A focus is a projection of the canonical CWI ledger:

- `unbound` or `planning` CWI: one pending/current focus with no sealed proof binding;
- `bound`: one current or remaining focus with the exact current binding;
- `settled`: one completed focus retaining immutable proof lineage;
- `reopened`: move that same CWI from completed to current/remaining in one SM-2 transaction, preserving prior
  receipt/evidence lineage and creating no duplicate queue member.

## 5. Required Internal Transition Contract

All transitions below are private Orchestrator internal events. External EventHub results are typed facts only and do
not choose a transition.

```text
typed owner fact
  -> Orchestrator validates scope and identity
  -> one RuntimeInternalEvent
  -> SM-2/SM-3/SM-4 reducer validates canonical CWI ledger
  -> one Runtime aggregate CAS
  -> next owner-neutral drive fact, if needed
```

### 5.1 Fresh activation

1. Run owner creates/activates a fresh Run through SM-1.
2. Orchestrator initializes the Definition seed and one `unbound` canonical coverage row per CWI.
3. SM-4 records a reconciliation request only after SM-2 has sealed the first valid current binding set.
4. No capability operation candidate or dispatch exists before the CWI ledger, semantic contract, and active
   reconciliation generation validate together.

### 5.2 Owner catalog invalidation

`RuntimeInternalEventSemanticCatalogInvalidated` must:

1. verify the expected/observed revision and exact dependency intersection;
2. preserve all settled CWI state and every bound CWI whose binding has an accepted receipt/effect/evidence;
3. retire only the active planning manifest, owner observations, and request sessions that depend on the stale catalog;
4. leave canonical binding eligibility unchanged and let planning select only rows already in `unbound`, `planning`, or
   explicitly `reopened` state;
5. never rebuild the pending order from the entire Definition seed without subtracting settled CWI; and
6. reject the transition while an active sealed dispatch exists.

Catalog invalidation alone is not a CWI reopen. It must not erase credit, queue a completed CWI, or authorize a
replacement binding.

The existing call from `retireSemanticPlanningGeneration` to `retireRuntimeCWIBindingsForManifest` must be removed.
If a no-effect binding genuinely needs retirement, the Orchestrator must dispatch a separate private typed binding
retirement event. Its reducer must prove that no accepted receipt, durable effect, evidence, active dispatch, or current
reconciliation batch depends on that binding, append the retirement cause, update the CWI/focus/contract projection,
supersede the old reconciliation generation, and commit all changes in one CAS.

### 5.3 Semantic document and binding seal

`SemanticPlanEvaluated`, feasibility observations, and `SemanticContractSealed` must operate on the canonical CWI
row set:

1. the proof batch may name only CWI rows in `unbound`, `planning`, or `reopened` state;
2. LLM proposals remain non-authoritative and cannot introduce a CWI, binding ID, coverage state, receipt, or action;
3. owner feasibility returns only typed factual feasibility for its declared proposal;
4. SM-2 derives the binding and checks that no non-reopened current binding exists for that CWI;
5. SM-2 rejects a successor binding when the predecessor has an accepted durable effect unless the same canonical row
   already contains matching explicit reopen lineage;
6. one CAS updates the binding lineage, current focus projection, coverage state, reconciliation contract, and any
   superseded reconciliation generation marker; and
7. SM-3 may request an operation candidate only after the CAS validates.

Duplicate coverage must therefore fail at SM-2 seal, before provider operation request, sealed dispatch, owner execute,
or receipt.

### 5.4 Reconciliation generation lifecycle

The current reconciliation request is an SM-4 delivery/scheduling fact, but its eligibility is defined by SM-2's
sealed CWI ledger.

When a new binding set is sealed:

1. archive the prior request/batch/results as immutable history, including its old binding set;
2. mark it superseded rather than requiring historical requests to equal the new contract;
3. clear the active request/batch/results from the current ledger;
4. create a pending current-generation identity derived from the new contract; and
5. let SM-4 issue exactly that request before a dependent operation drive proceeds.

When an owner batch arrives:

1. SM-4 validates only transport/scope and submits the typed batch fact;
2. SM-2 checks that the batch matches the current request and current contract hash exactly;
3. stale/superseded batches are rejected without changing coverage, focus, request history, or operations;
4. retain, prerequisite, and reopen dispositions are computed by SM-2; and
5. a reopen atomically changes the CWI row, coverage state, focus projection, reconciliation lineage, and reducer
   decision in one CAS.

`reconcileRuntimeWorkspaceWithCommand` must not silently substitute an old active request merely because one exists.

### 5.5 Receipt-driven reopen and CWI dependencies

The existing receipt-driven dependency reopen and reconciliation-driven reopen must call the same owner-local helper.
That helper receives only canonical CWI IDs and typed lineage refs, then:

1. verifies that the reopening trigger is declared by the Definition/profile contract;
2. moves matching completed focus rows back into the remaining/current queue once;
3. leaves unrelated completed CWI unchanged;
4. preserves the previous receipt/evidence for audit, but removes its current-credit eligibility; and
5. marks only the exact CWI as `reopened`.

No capability owner, projection, LLM result, raw diagnostic, or user interface may reopen a CWI.

### 5.6 Explicit no-effect binding retirement

Binding retirement is narrower than reopen and is allowed only before an effect has been accepted. The private event
must carry facts, not a requested target state:

- exact workspace/run/definition/CWI identity;
- current binding ref and semantic manifest identity;
- typed retirement reason and causation ref; and
- expected Runtime state revision.

The SM-2 reducer must reject it when the binding is referenced by an active dispatch, accepted receipt, durable effect,
evidence, settled/reopened CWI lineage, or an in-flight reconciliation batch. On success, one CAS appends immutable
retirement lineage, clears the current binding, projects the exact CWI once into planning, rebuilds the canonical
reconciliation contract, and supersedes the prior request generation. A duplicate event with the same causation and
already-retired binding is a no-op; a stale or mismatched event is a typed conflict.

### 5.7 Candidate, dispatch, receipt, and evidence

SM-3 remains unchanged in authority: it consumes the exact current focus/binding selected by SM-2 and seals a dispatch.
Before candidate request and before dispatch seal, validate that the focus's CWI current binding ref is still current
and eligible. Receipt/evidence commit must advance `bound -> settled` only when the sealed binding, receipt, evidence,
CWI ledger row, execution key, and fence agree. Once an accepted durable receipt is recorded, the authorizing binding is
lineage-locked until an explicit reopen CAS. A duplicate/stale receipt is an idempotent no-op or typed conflict; it
cannot create a new binding or focus and must never cause the capability owner to execute twice.

### 5.8 Resume, restart, and schema boundary

- `/run resume` uses the current Run's canonical CWI ledger and may only retain or explicitly reopen current-run
  binding lineage. It must never re-plan an already settled CWI because of catalog invalidation.
- `/run restart` creates a new Run through the existing SM-1 replacement sequence. The new Run has a new canonical CWI
  ledger and receives no predecessor credit, binding, receipt, focus, or reconciliation request.
- The new ledger is a current-schema requirement. An aggregate missing it, or containing ambiguous duplicate CWI
  identity, returns `fresh_activation_required`; it is not guessed, rebuilt from history, or resumed.
- There is no Runtime V1 fallback, compatibility alias, or automatic replay of an old durable operation.

## 6. Component Work Matrix

| Component | Required change | Must not do |
| --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | Preserve canonical coverage/binding validation and add a pure owner-neutral target-policy consistency rule: a prospective target must not be a member of the same sealed catalog's existing target set; return a typed proposal violation suitable for bounded correction | persist state, use EventHub, select retry/action/terminal, inspect the workspace, create a global index, hard-code File/domain semantics |
| `kernel/orchestrator/biz/runtime/cwi_ledger.go` | Provide the sole canonical CWI eligibility, binding retirement, reopen and focus-projection reducers; lock an accepted-effect binding until explicit reopen | expose a second owner, persist a global lookup, infer lineage from history |
| `kernel/orchestrator/biz/runtime/semantic_planning.go` | Retire only semantic-generation authority; make order/batch generation consume canonical eligible CWI rows; enforce pre-seal uniqueness; generate stable pending IDs; persist exactly one typed correction transition for an attempt-0 target-policy contradiction | clear bindings as a catalog-retirement side effect, reset all CWI to pending, rebuild credit from text/history, add a second retry state machine |
| `kernel/orchestrator/biz/runtime_semantic_drive.go` | Route the new pure semantic evaluator rejection through the existing attempt-0/1 correction session and preserve the current terminal handling for owner `infeasible` facts | retry owner infeasibility, refresh a current catalog, let diagnostic text choose action, submit an unbounded provider request |
| `kernel/orchestrator/biz/runtime/reconciliation.go` | Make reconciliation commit validate the current contract and call the shared reopen/focus transition | directly reopen only coverage without focus, infer new credit |
| `kernel/orchestrator/biz/runtime/dispatch_commit.go` | Reuse one canonical reopen helper; record accepted effect lineage and gate evidence on current binding/execution/fence refs | create a parallel CWI owner or bypass SM-2 |
| `kernel/orchestrator/biz/runtime/internal_event.go` | Add only private typed events needed for generation supersession, no-effect binding retirement and atomic CWI transitions | expose these events as EventHub commands or accept target states |
| `kernel/orchestrator/biz/runtime/transition_coordinator.go` | Validate the complete post-reducer aggregate and perform exactly one semantic CAS per private event | persist partial CWI/focus/request updates or retry an owner effect |
| `kernel/orchestrator/biz/runtime_reconciliation.go` | Schedule current request generation only; archive/supersede stale request generations | mutate CWI/coverage/focus or reuse stale request blindly |
| `blocks/run` | Persist opaque current-schema Runtime aggregate and preserve semantic CAS behavior; add schema-boundary tests | interpret SM-2/3/4 or choose successor state |
| File/Git/Shell/MCP capability Blocks | Keep returning scoped feasibility observations and exact receipts; File retains the real-resource prospective/existing fence for post-catalog drift and concurrency; add duplicate/stale response tests where needed | mutate focus, coverage, binding, correction, request generation, or lifecycle; choose exact versus prospective policy for Orchestrator |
| Workspace/Result | Preserve terminal delivery/acknowledgement tests | write Runtime CWI state |
| LLM, application, TUI, Web, projection, eventstream | Render/transport typed facts only; optionally display canonical CWI state after it is authoritative | infer/rebuild authority or drive a transition |

### 6.1 Registered Component Boundary Matrix

The CWI closure applies to every registered runtime unit. This matrix is the active 22-unit boundary
baseline used by the component-contract verifier; it records the final responsibility rather than creating
a second Runtime implementation plan.

| Registered unit | Kind | Runtime responsibility and boundary |
| --- | --- | --- |
| `internal/initiators/workorchruntime` | Initiator | Provides repository factory/provider wiring only; owns no EventHub, Runtime policy, bootstrap export, or Runtime state. |
| `internal/modules/application/workorchd` | Application Module | Adapts HTTP/SSE and typed activation/read requests; it submits Run-owner intents and projects facts, but never drives or reduces Runtime state. |
| `internal/modules/blocks/approval` | Block | Owns approval resources and returns typed approval facts only; it does not mutate CWI, focus, binding, dispatch, or lifecycle. |
| `internal/modules/blocks/decision` | Block | Owns decision tickets and returns typed resolution facts only; Orchestrator commits any Runtime consequence. |
| `internal/modules/blocks/eventstream` | Block | Persists audit/display notifications only; it never reconstructs or advances Runtime authority. |
| `internal/modules/blocks/file` | Capability Block | Owns File feasibility, reconciliation observations, execution, and exact receipts; it cannot grant CWI credit or choose the next transition. |
| `internal/modules/blocks/git` | Capability Block | Owns Git capability facts and receipts only; it does not manage CWI or Run state. |
| `internal/modules/blocks/llm` | Capability Block | Owns provider request/response transport and typed document facts only; it never selects Runtime action, correction, or terminal state. |
| `internal/modules/blocks/mcp` | Capability Block | Owns MCP feasibility/execution and scoped receipts only; it cannot mutate Runtime authority. |
| `internal/modules/blocks/mcpregistry` | Block | Owns MCP registry resources/configuration only; it contributes typed capability facts but no Runtime state transition. |
| `internal/modules/blocks/memory` | Block | Owns memory resources only; recalled content is non-authoritative request context. |
| `internal/modules/blocks/result` | Block | Owns Result persistence and local lifecycle after receiving the sealed `TerminalOutcome`; it does not adjudicate acceptance. |
| `internal/modules/blocks/run` | Block | Sole SM-1 lifecycle/lineage owner and opaque Runtime aggregate CAS owner; it does not interpret or reduce SM-2/SM-3/SM-4. |
| `internal/modules/blocks/shell` | Capability Block | Owns Shell/managed-validation feasibility, execution, and receipts only; it cannot change coverage or focus. |
| `internal/modules/blocks/skill` | Block | Owns skill resources only; skill content cannot become a Runtime state authority. |
| `internal/modules/blocks/soul` | Block | Owns soul resources/bindings only; it cannot select Runtime transitions. |
| `internal/modules/blocks/ssh` | Capability Block | Owns SSH feasibility/execution and scoped receipts only; it cannot alter CWI or lifecycle state. |
| `internal/modules/blocks/web` | Capability Block | Owns Web capability facts/receipts only; it cannot alter Runtime authority. |
| `internal/modules/blocks/workspace` | Block | Owns Goal/DoD definitions and active-run slot through Workspace-private events; it does not mutate the Runtime aggregate. |
| `internal/modules/kernel/governance` | Kernel Module | Coordinates governance-domain contracts only; a resolution reaches Runtime as a typed fact and is reduced solely by Orchestrator. |
| `internal/modules/kernel/orchestrator` | Kernel Module | Sole owner of SM-2, SM-3, and SM-4: private internal events validate/reduce facts and one aggregate CAS persists the result. |
| `internal/modules/kernel/system` | Kernel Module | Owns startup defaults/system resources only; it does not hold Runtime progression state or issue Runtime transitions. |

## 7. Ordered Code Closure Queue

All items are sequential. A later item may not compensate for an earlier missing invariant.

| ID | Status | Scope | Required outcome |
| --- | --- | --- | --- |
| CWI-SO-1 | code-closed | Contract and validation foundation | The existing coverage row is the canonical binding/focus ledger; queue uniqueness, current binding uniqueness, contract/request coherence and stable pending IDs are aggregate invariants. |
| CWI-SO-2 | code-closed | SM-2 generation retirement and semantic seal | Generation retirement preserves effect-bearing bindings; planning batches use canonical CWI eligibility; binding seal and focus projection share one aggregate CAS. Typed owner-scope catalogs are distinguished from exact-target catalogs without retry loops. |
| CWI-SO-3 | code-closed | SM-4 request generation and SM-2 reconciliation commit | Request generations are fenced by the current binding set; stale generations are retained only as history; reconciliation and receipt-driven reopen use the canonical CWI transition. |
| CWI-SO-4 | code-closed | SM-3 admission and evidence fences | Candidate, sealed dispatch, receipt and evidence admission require the exact canonical binding/execution/fence identity; stale or duplicate facts cannot replay a durable operation. |
| CWI-SO-5 | code-closed | Current-schema boundary and component regressions | Legacy schema and current-schema ambiguous CWI ledgers fail closed through the typed Run-owned `fresh_activation_required` fact. No compatibility reconstruction, Runtime LLM topic, capability-owner I/O, retry lease, or replay occurs. |
| CWI-SO-6 | code-closed | SM-3 target-policy consistency and bounded correction | A prospective target already present in the same sealed owner catalog is rejected before owner probing; the existing attempt-0/1 correction preserves immutable generation identity; the owner feasibility fence remains unchanged. |
| CWI-SO-7 | automated-verified; live-pending | Full verification and deployed proof | Focused/full/race/boundary and isolated managed mock-provider suites pass. Complete a fresh positive run and catalog-invalidation/reopen/restart/resume fault matrix. |

### 7.1 CWI-SO-1 detail

Land in `workorchruntimecontract` first. This slice affects SM-2 validation only; it must not add a transition or I/O.

Implementation scope:

- extend the existing CWI row/lineage value only where the current values cannot represent a typed no-effect binding
  retirement and an accepted-effect lineage lock;
- keep `RuntimeFacts.Coverage` as the persisted authority and build lookup maps only inside pure validation calls;
- make aggregate validation compare the canonical row with current/remaining/completed focus, reconciliation contract,
  active request/batch, sealed dispatch, accepted receipt and evidence identities; and
- make clone, normalize and stable hash include every newly persisted lineage field.

Required pure fixtures before reducer changes:

- same CWI in current and completed focus is invalid;
- duplicate current binding for a non-reopened CWI is invalid;
- an explicit reopened lineage permits one successor current binding while retaining prior audit;
- an accepted durable receipt/effect with cleared or replaced current binding is invalid;
- a no-effect retired binding with typed retirement lineage may return once to planning;
- a retirement after accepted receipt/effect/evidence is invalid;
- active reconciliation request equals the current sealed contract exactly;
- historical request may retain a previous contract only when marked superseded;
- pending IDs are stable across normalize/reload and never recursively prefixed.

Do not add a map-based persisted authority, facade alias, JSON bridge, or compatibility decoder.

Exit gate: focused contract tests prove every invalid aggregate is rejected using aggregate data alone, and all existing
current-schema positive fixtures still normalize/hash deterministically.

### 7.2 CWI-SO-2 detail

Refactor `retireSemanticPlanningGeneration`, coverage-order acceptance, `semanticPendingFocuses`, and
`semanticReconciliationContractAfterFocusBatch` as one SM-2 closure. The implementation must derive eligible CWI from
the canonical coverage rows, not from the full Definition order. A completed CWI remains out of a new order unless a
matching private reopen event has changed its canonical state.

The new contract compiler input must be the canonical current binding projection, not `CompletedFocuses + newly
compiled focuses`. This removes the late duplicate detector as the primary guard; the compiler remains a defensive
validator only.

Required code sequence:

1. remove the catalog-retirement call to `retireRuntimeCWIBindingsForManifest`;
2. make catalog retirement archive the semantic generation and dependent request sessions without changing any CWI
   binding, focus, coverage or reconciliation contract;
3. replace the old broad binding-retirement helper with a private SM-2 reducer that accepts the exact CWI/current
   binding/cause/revision fact and rejects any effect-bearing lineage;
4. derive coverage order and proof batches only from canonical rows already eligible for planning;
5. before semantic seal, reject a selected CWI that is bound/settled or has an unretired current binding, and reject an
   accepted-effect predecessor without explicit reopen lineage; and
6. in one semantic-seal CAS, update the row binding lineage, focus projection, binding set/hash, reconciliation contract
   and supersession marker.

Atomic write sets:

| Private event | Allowed write set | Must remain byte-for-byte stable |
| --- | --- | --- |
| catalog invalidated | semantic ledger/history, dependent request sessions, catalog freshness lineage | CWI rows, focus queues, binding set, reconciliation contract/request, receipts/evidence |
| no-effect binding retired | exact CWI row, exact focus projection, binding set, reconciliation contract and request supersession | unrelated CWI, accepted effects/evidence, semantic history |
| semantic contract sealed | selected eligible CWI rows, their focus projection, binding set/hash, contract and request supersession | settled/unselected CWI and all prior effect/evidence audit |

Idempotency rules:

- duplicate catalog invalidation does not append another history row or increase state revision;
- duplicate binding retirement with the same cause and retired ref is a no-op;
- stale binding retirement is a typed conflict and changes no state;
- duplicate semantic document/feasibility/seal creates no second binding or CAS; and
- no owner operation topic is reachable until the post-seal aggregate validates.

Exit gate: a fixture matching `run-077` cannot clear or replace the effect-bearing binding, and the rejection occurs
before a provider operation request or capability execution.

### 7.3 CWI-SO-3 detail

Refactor request construction and reconciliation ledger validation together. An active request is valid only for the
current contract. Historical request/batch rows retain their own contract hash and must no longer be forced to equal the
current one. On re-seal, archive then clear the active row in the same owner-local transition; do not query owners until
SM-4 schedules the fresh request.

Update reconciliation batch commit so `reopened` uses the same requeue helper as receipt-driven dependency reopen.
The helper remains inside the Runtime owner and is invoked only from a private reducer transition.

Required code sequence:

1. define one current reconciliation generation identity from definition revision plus canonical binding-set hash;
2. when that identity changes, archive the complete old request/batch/result as bounded superseded history and clear
   the active transport slots in the same CAS;
3. let SM-4 schedule only the pending current identity; it may not substitute an older active request;
4. let SM-4 validate transport scope and submit the detached typed batch fact only;
5. let SM-2 validate exact request ID, generation, definition, binding refs and contract hash; and
6. route `reopened` through the same canonical reopen helper used by receipt-driven dependency reopen.

The reopen CAS must update the exact CWI row, reopen lineage, current-credit eligibility, current/completed/remaining
focus projection, binding set, reconciliation contract, request supersession and reducer decision together. It must
not update unrelated CWI rows.

Exit gate: stale and duplicate batches produce zero CWI/focus/request mutation; a valid reopen produces one queue move,
one successor-planning opportunity and preserved predecessor audit.

### 7.4 CWI-SO-4 detail

Add canonical binding-ref checks at four fences:

1. before operation candidate request;
2. before sealed dispatch commit;
3. before owner receipt/evidence admission; and
4. before focus advance.

These are validation guards, not four mutation paths. All state change still occurs once through the internal reducer and
CAS coordinator.

Each fence must compare the same identity tuple:

```text
workspace + run + definition revision + CWI + focus
+ current binding ref + binding-set hash
+ dispatch + operation + execution key + fence (when available)
```

Required behavior by fence:

| Fence | Rejection condition | Rejection effect |
| --- | --- | --- |
| candidate request | focus/binding is no longer canonical or CWI is not eligible | zero provider request |
| dispatch seal | candidate refers to retired, superseded or effect-locked binding | zero sealed dispatch and zero owner call |
| receipt/evidence admission | scope, binding, operation, execution key or fence differs | typed stale/conflict; no credit, reopen or replacement binding |
| focus advance | CWI row, sealed binding, accepted receipt and evidence do not agree | no queue mutation and fail closed |

Accepted durable receipts must be recorded before any dependent invalidation is reduced, so a catalog invalidation
cannot pass through a timing window and erase the binding. Duplicate delivery of the same accepted receipt is an
idempotent fact commit; it cannot invoke the capability owner again. A different receipt for the same execution key or
fence is a typed conflict.

Exit gate: fault injection at every durable CAS boundary, including process restart, proves exactly one owner execution
and at most one durable effect for an execution key.

### 7.5 CWI-SO-5 detail

Bump the Runtime aggregate current schema only if required by the new persisted fields. Do not reconstruct ambiguous
lineage from `CompletedFocuses`, prior request history, raw events, or provider text. Existing non-terminal aggregates
without the new schema are not executable and must return `fresh_activation_required`; the user may then explicitly
restart into a new Run.

No capability Block migration changes its state model. Its contract tests only prove that stale/duplicate facts cannot
cause the Orchestrator to mutate twice.

Code closure evidence:

- `RuntimeStateSchemaVersionCurrent=4` is the only accepted persisted aggregate schema; a missing canonical CWI
  ledger or duplicate CWI projection under schema 4 is classified as `cwi_ledger_invalid`, not silently migrated.
- startup recovery and direct `DriveRuntime` report the immutable incompatibility fact to the Run owner, which alone
  persists `fresh_activation_required`; repeat drive is a no-op and does not create a recovery lease.
- focused regressions observe zero calls to all Runtime LLM topics (capability acquisition, provider ceiling,
  preflight, and candidate request) and all reachable File Runtime prerequisite/admissibility/feasibility,
  reconciliation, and execute topics for both legacy-schema and ambiguous-current-schema aggregates.
- Run opaque persistence, application projection, projection clone, and TUI surface preserve the typed reason without
  interpreting SM-2/SM-3/SM-4 state. The TUI directs the user to `/run restart` without falsely reporting a schema
  version mismatch for a same-schema ledger defect.

This slice closed only the current-schema boundary. CWI-SO-6 subsequently closed target-policy consistency; CWI-SO-7
has passed the full/race/managed automated gates, while its fresh deployed invalidation/reopen/restart matrix remains
open.

### 7.6 CWI-SO-6 detail

This slice closes the pre-binding contradiction from `run-078` without adding a state machine or weakening owner
feasibility. Implement it in this order:

1. In `workorchruntimecontract`, extend `validateSemanticProposalAgainstOwnerCatalog` with one pure relational rule for
   `prospective_owner_target`: the normalized `ProspectiveTargetRef` must not occur in the normalized sealed
   `catalog.TargetRefs`. Return `RuntimeSemanticProposalConstraintError` with the exact proposal field, relational rule,
   actual target, and a bounded required statement. Do not return the complete target inventory as an error payload.
2. Ensure complete semantic document validation converts that constraint into the existing
   `RuntimeSemanticProposalViolationError` and `RuntimeOriginalDiagnostic` path. Attempt 0 must therefore enter the
   existing `semantic_plan_correction_required` private transition; attempt 1 must fail deterministically with the
   original typed violation preserved.
3. Keep correction identity inside the existing semantic request session: same workspace/run, Definition seed,
   manifest hash, catalog snapshot/revision, coverage order, batch identity and stable prefix; only logical request and
   `CorrectionAttempt=1` may advance. Correction must not refresh or reacquire the catalog.
4. Keep SM-2 owner-feasibility handling unchanged: a revision mismatch remains the owner's typed `stale` fact and may
   use the existing bounded catalog refresh; a current-revision `infeasible` fact remains terminal. It must never be
   converted into semantic correction based on diagnostic code or message.
5. On correction, retire only the rejected semantic document/request authority. Preserve CWI rows, focus, catalog
   observation, reconciliation authority, receipts and evidence byte-for-byte. There is still no binding to retire in
   this phase.
6. File owner continues to stat the target and return `prospective_target_exists` or
   `prospective_target_unobservable`. This remains the final resource-truth fence and must not choose the replacement
   policy or trigger an Orchestrator transition directly.

Allowed transition sequence:

```text
attempt-0 complete semantic document
  -> pure catalog-consistency rejection
  -> SM-3 semantic_plan_correction_required (one CAS)
  -> attempt-1 complete semantic document
  -> evaluate against the same sealed catalog
  -> owner feasibility
  -> SM-2 semantic_contract_sealed
```

Resource-race sequence:

```text
attempt-0 document accepted against catalog revision R
  -> owner observes resource revision R+1
  -> SM-2 semantic_feasibility_observed(stale) (fact-only CAS)
  -> existing bounded catalog refresh
  -> new manifest/request identity for revision R+1
```

This is distinct from semantic correction. A same-revision owner infeasibility, missing diagnostic identity,
batch/snapshot drift, a second semantic contradiction, or exhausted correction/refresh budget must terminate with the
original diagnostic.

Required tests:

- prospective target equals one sealed existing target: rejected before EventHub owner feasibility;
- exact-target proposal for that target: accepted when otherwise admissible;
- existing-owner-scope proposal for that target: accepted when otherwise admissible;
- truly new prospective target: accepted by pure evaluation and still probed by the owner;
- attempt-0 contradiction: exactly one correction session and one private CAS;
- attempt-1 contradiction: typed terminal, no third provider request;
- owner reports same-revision `prospective_target_exists` after document acceptance: terminal without correction,
  binding, dispatch, receipt, effect, Decision or Approval;
- owner reports a matching revision change: existing bounded stale catalog refresh, not semantic correction;
- unrelated owner infeasibility: terminal without correction;
- duplicate/stale document, feasibility fact and correction event: no additional revision or request;
- restart after rejection CAS and after correction-required CAS: reload preserves attempt identity and does not replay
  attempt 0; and
- stable prefix, manifest, Definition seed, catalog snapshot and coverage-order hashes remain unchanged across
  correction.

Exit gate: a fixture matching `run-078` cannot reach File feasibility with the contradictory attempt-0 proposal; a
corrected exact/existing or truly prospective proposal reaches feasibility and binding seal, while all rejected paths
have zero operation side effects.

### 7.7 CWI-SO-7 detail

Verification is a required implementation slice, not a documentation-only sign-off. Run it only after CWI-SO-1 through
CWI-SO-6 are code-closed.

Evidence must include:

- focused transition and invariant tests for every new/restricted private event;
- stale/duplicate fact tests for semantic document, feasibility, reconciliation batch and receipt;
- restart after semantic retirement, binding retirement, semantic seal, reconciliation supersession, dispatch seal,
  accepted receipt, evidence commit and terminal delivery CAS;
- `/run resume` retaining only current-run authority and `/run restart` starting a clean lineage;
- boundary verification proving all 22 registered units retain their owner role; and
- a fresh deployed catalog-invalidation plus explicit-reopen matrix proving zero duplicate execution/effect.

Do not mark CWI-SO-7 complete from old green artifacts or a single positive run.

### 7.8 2026-08-13 post-CWI-SO-6 automated verification record

The current code-closure packet passed:

- `go test ./internal/pkg/workorchruntimecontract/... -count=1`;
- `go test ./internal/modules/kernel/orchestrator/biz/runtime/... -count=1`;
- `go test ./internal/modules/kernel/orchestrator/biz/... -count=1`;
- `make fmt-check`;
- `make vet`;
- `make verify-boundary-contract`;
- `make test`;
- `make test-race`; and
- `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file ./scripts/verify-workorch-managed-full.sh` against an isolated
  file-backed instance. Direct `make verify-ci-mvp-file` is a deployed-instance target, not the managed harness entry.

The managed instance passed HTTP health/readiness, workspace template/Soul, mock LLM execution and MCP stdio execution;
the MCP Run reached `completed` with two completed dispatches and evidence sequence two. Its bounded step log is under
`.ci-artifacts/managed-workorch-cwi-so-7/`. This automated record does not replace CWI-SO-7's fresh real-workspace
positive convergence, catalog-invalidation, explicit-reopen, process-restart, `/run resume`, `/run restart` and
duplicate-effect matrix.

## 8. Owner-Local Event And CAS Catalogue

This catalogue is the implementation review checklist. Names may follow the existing Go naming convention, but their
authority and write sets may not be merged with cross-owner commands.

| Owner-private event | SM | Input fact | Atomic outcome |
| --- | --- | --- | --- |
| semantic catalog invalidated | SM-2/SM-3 | exact catalog revision/dependency gap | retire planning authority only |
| no-effect binding retired | SM-2 | exact current binding and typed cause | retire one eligible binding and synchronize focus/contract/request |
| semantic plan evaluated | SM-3 | detached complete provider document evaluated against the complete sealed catalog | persist accepted document or one bounded correction/failure |
| semantic plan correction required | SM-3 | exact attempt-0 complete-document typed violation | preserve immutable generation identity and open attempt 1 exactly once |
| semantic feasibility observed | SM-2 | exact owner proposal observation | persist factual observation only |
| semantic contract sealed | SM-2 | complete eligible proposal/observation set | seal bindings, focus projection and current reconciliation authority |
| reconciliation batch observed | SM-4 -> SM-2 | exact typed owner batch fact | validate current generation, then retain/reopen/fail in SM-2 |
| CWI reopened | SM-2 | declared trigger plus prior binding/receipt/evidence refs | atomically reopen one CWI and synchronize all projections |
| dispatch candidate admitted/sealed | SM-3 | current canonical binding/candidate fact | seal one dispatch after all fences pass |
| owner receipt/evidence admitted | SM-3 -> SM-2 | exact execution receipt and evidence fact | record effect lineage and settle/advance exact CWI |
| terminal delivery acknowledged | SM-4 | Result/Workspace/Run owner acknowledgement | advance delivery ledger only |

External EventHub contracts remain defined by the producing resource/capability owner. None of the private events above
is exported as a topic or accepted from Web, TUI, application, projection, LLM, or a capability Block.

## 9. Required Regression Matrix

| Scenario | Required assertion |
| --- | --- |
| prospective target already belongs to sealed catalog | attempt 0 is rejected by the pure evaluator; zero owner probe, binding, dispatch, receipt or effect |
| corrected existing target policy | one attempt-1 document can select exact/existing policy within the unchanged catalog and proceed to owner feasibility |
| truly new prospective target | pure evaluator accepts it; capability owner remains the final real-resource feasibility fence |
| target appears after catalog snapshot with revision change | owner returns typed stale; existing bounded catalog refresh creates a new manifest identity; no semantic correction or operation side effect |
| same-revision owner infeasibility | typed terminal is preserved; no semantic correction is inferred from owner code/message |
| repeated prospective/existing contradiction | attempt 1 terminates with the original typed diagnostic; no attempt 2 |
| unrelated owner infeasibility | no semantic correction is inferred from message text; typed terminal remains fail-closed |
| completed CWI + catalog invalidation | no new pending focus, semantic proposal, binding, LLM operation request, dispatch, receipt, or effect for that CWI |
| bound CWI + accepted durable effect + catalog invalidation | current binding and effect lineage remain locked; no successor binding or operation without explicit reopen |
| bound CWI + no effect + explicit binding retirement | one typed retirement lineage, one planning projection, synchronized contract/request, no owner operation |
| bound CWI + no effect + catalog invalidation only | binding remains unchanged; catalog invalidation does not impersonate binding retirement |
| current unbound CWI + catalog invalidation | old generation is retired once; fresh generation covers only eligible CWI |
| explicit receipt-driven reopen | one queue move, preserved old audit, one allowed successor binding only after fresh seal |
| reconciliation-driven reopen | coverage and focus move in the same CAS; unrelated completed focus remains completed |
| re-seal after binding-set change | old request/batch become superseded history; new request exactly equals new contract |
| stale reconciliation batch | zero CWI/coverage/focus/request mutation and zero operation |
| duplicate semantic document, feasibility, receipt, or batch | no extra CAS, request, binding, dispatch, or operation |
| restart after each durable CAS | reload yields the same CWI ledger and never repeats owner execution |
| `/run resume` | retains matching current-run binding/receipt lineage and does not re-plan settled CWI |
| `/run restart` | new Run has no predecessor CWI credit or request lineage |
| legacy/ambiguous aggregate | typed `fresh_activation_required`, zero provider/capability execution |
| focus identity reload loop | no recursive pending prefix and stable aggregate/request hashes |
| terminal delivery after typed contract defect | `Result -> Workspace -> Run` acknowledgement order remains intact |
| 22-component boundary scan | no new cross-owner repository/service/state transition path |

## 10. Verification Sequence

Run the following only after each implementation slice is complete:

```bash
go test ./internal/pkg/workorchruntimecontract/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtime/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/... -count=1
make verify-boundary-contract
make test
make test-race
make verify-ci-mvp-file
```

Deployed verification must use a fresh Run and record only bounded audit facts:

1. restart the `run-078` workspace shape and prove the first state-change CWI passes target-policy evaluation,
   feasibility and binding seal without `prospective_target_exists` looping;
2. prove the Run reaches at least one sealed dispatch, accepted receipt, evidence commit and CWI advance;
3. restart a workspace with at least one completed CWI;
4. induce a declared catalog/resource invalidation without changing the CWI's reopen condition;
5. prove no duplicate binding, LLM operation request, dispatch, receipt, or workspace effect;
6. exercise one explicit reopen and prove exactly one successor lineage and effect path;
7. resume after process restart and prove no re-drive of an accepted execution key;
8. verify `healthz` and `readyz`, including projection catch-up; and
9. validate terminal delivery order if a typed contract failure is intentionally injected.

Raw provider prompts, credentials, full workspace contents, and unredacted responses are not evidence artifacts.

## 11. Completion Criteria

This plan is code-closed only when all of the following hold:

- one persisted canonical CWI row governs focus, binding, coverage, reconciliation, and reopen eligibility;
- semantic generation retirement cannot clear or replace a binding and cannot reintroduce a settled/effect-bearing CWI;
- no-effect binding retirement is a separate typed SM-2 transition with a bounded cause and one atomic write set;
- an accepted durable effect lineage locks its binding until an explicit typed reopen is committed;
- duplicate CWI binding is rejected before provider operation request and before any side effect;
- active reconciliation request/batch exactly match the current sealed contract, while superseded history remains auditable;
- receipt- and reconciliation-driven reopen share one SM-2 queue/coverage transition;
- semantic proposal evaluation rejects a prospective target already present in the same sealed catalog before owner
  probing, while a real post-catalog resource change still reaches the capability-owner feasibility fence;
- evaluator-detected target-policy contradiction can consume only the existing attempt-0/1 correction budget and
  cannot refresh the catalog, add a state machine, or create operation side effects; owner `infeasible` cannot consume
  that correction budget;
- no runtime component outside Orchestrator owns or mutates SM-2/SM-3/SM-4 state;
- legacy/ambiguous aggregates fail closed without fallback or replay; and
- the regression matrix and deployed fault matrix pass.

It is live-closed only after a new workspace run demonstrates positive convergence and the catalog-invalidation,
reopen, restart, and duplicate-delivery cases demonstrate zero unintended duplicate effects.

## 12. Explicit Non-Solutions

The following do not close this issue and must not be introduced:

- filtering only `PendingCoverageIDs` while leaving binding/request/focus state independent;
- accepting duplicate CWI binding then relying on the reconciliation compiler to fail later;
- reconstructing current authority from event history, LLM text, files, Git state, UI state, or projection;
- using a global CWI registry, global EventHub type index, service facade, or shared mutable cache;
- adding a fifth reconciliation state machine or cross-owner target-state command;
- treating catalog invalidation as an implicit reopen;
- treating catalog invalidation as implicit binding retirement, including for a not-yet-settled CWI;
- adding only a late receipt/evidence fence while preserving implicit `bound -> planning -> bound` replacement;
- fixing `prospective_target_exists` only inside File, removing File's real-resource fence, or hard-coding path/file
  semantics in Orchestrator;
- retrying every owner `infeasible` status, using diagnostic message text to select correction, or refreshing an
  unchanged catalog to repair a provider target-policy contradiction;
- adding a second correction counter, retry loop, target policy, or state machine for semantic feasibility;
- migrating an ambiguous live aggregate by guessing lineage; or
- retrying/replaying a durable capability operation solely to repair bookkeeping.
