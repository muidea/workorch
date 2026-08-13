# Runtime CWI Reconciliation Single-Owner Closure Plan

Status: active

Type: implementation-closure-plan

Last Updated: 2026-08-13

## 1. Purpose And Authority

This is the sole active implementation queue for closing Runtime CWI reconciliation and dispatch admission without
cross-component state management. It supersedes the former field-by-field closure queue in this file. Historical
implementation narratives and superseded status claims are audit evidence only and do not authorize new code.

Authority order:

1. [`docs/00-core/runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)
2. [`internal/modules/kernel/orchestrator/readme.md`](../../../internal/modules/kernel/orchestrator/readme.md)
3. `internal/pkg/workorchruntimecontract`
4. this plan, which defines the ordered implementation and verification queue

Production remains `DriveRuntime + Orchestrator reducer + sealed dispatch`. This plan adds no state machine, owner,
global index, service facade, compatibility path, or domain-specific Runtime rule.

## 2. Why This Closure Is Reopened

The recent commit audit found one real authority reversal and one repeated incremental-gap pattern.

### 2.1 Confirmed authority reversal

`1161005e` allowed the semantic provider to declare `ArtifactRefs` and `MetricConstraints` in a proof proposal.
`run-082` proved that this let provider output create proof requirements not owned by the activation-sealed source
declaration. `79fb7026` removed those fields from the provider wire and restored declaration-owned compilation.

The final rule is permanent:

- target, artifact, metric, check, effect and evidence requirement are different concepts;
- provider output may select only from a sealed semantic vocabulary;
- it may not create artifact, metric, binding, credit, dependency or effect authority; and
- proof requirements are compiled only from the activation-sealed template/profile declaration and assigned to one
  unambiguous proof before dispatch.

### 2.2 Incremental-gap sequence

The following live counterexamples exposed different dimensions of one incomplete admission boundary:

| Run | Counterexample | Missing admission dimension |
| --- | --- | --- |
| `run-073` | duplicate CWI/binding lineage after repeated File effects | canonical CWI identity |
| `run-077` | effect-bearing binding returned to planning without typed reopen | retirement/effect lineage |
| `run-078` | existing owner target was proposed as prospective | target policy versus sealed catalog |
| `run-079` | active transaction collided with binding retirement | dispatch/retirement fence |
| `run-080` | Git `add -> commit` attached proof authority at kind rather than variant/operation level | variant and required operation identity |
| `run-081` | validation was admitted before preparation | dependency admission |
| `run-082` | provider-declared artifact requirement passed semantic evaluation | proof-requirement authority |

These are not seven independent bugs. They show that SM-2/SM-3 admission was closed one field at a time. No further
change may claim closure by handling only the latest diagnostic. Each implementation slice below must preserve the
complete admission matrix.

### 2.3 Closure objective

Replace dispersed local admission decisions with one Orchestrator-owned transaction:

```text
activation-sealed definition and declarations
  -> SM-2 selects the exact eligible CWI closure
  -> capability owners return typed catalog/admissibility facts
  -> Orchestrator internal event seals the authority snapshot
  -> LLM owner returns a bounded non-authoritative semantic document
  -> one pure admission evaluator checks the complete matrix
  -> Orchestrator internal event atomically seals binding + candidate + dispatch
  -> capability owner executes and returns a typed receipt
  -> Orchestrator internal event admits evidence and advances/reopens CWI
```

The evaluator is pure. It may return a typed decision, but it may not mutate state. Only an Orchestrator-private
internal event and reducer may change SM-2, SM-3 or SM-4.

## 3. Final State-Machine Ownership

| Machine/resource | Sole owner | Legal mutation entry | Forbidden mutation source |
| --- | --- | --- | --- |
| SM-1 Run lifecycle and lineage | `blocks/run` | Run-private internal event and Run repository/CAS | Application, Workspace, Orchestrator or projection |
| SM-2 authority/focus/CWI convergence | `kernel/orchestrator` | Orchestrator-private internal event and Runtime aggregate CAS | LLM, capability owner, Run, UI or projection |
| SM-3 candidate/sealed-dispatch transaction | `kernel/orchestrator` | Orchestrator-private internal event and Runtime aggregate CAS | provider document, capability owner or application directly |
| SM-4 drive/recovery/reconciliation/delivery ledger | `kernel/orchestrator` | Orchestrator-private internal event and Runtime aggregate CAS | timer, startup scanner, UI or acknowledgement sender directly |
| Workspace definition and active Run slot | `blocks/workspace` | Workspace-private internal event | Orchestrator repository write |
| terminal result lifecycle | `blocks/result` | Result-private internal event | Orchestrator acceptance adjudication write-through |
| governance ticket/resolution | `blocks/decision`, `blocks/approval` | corresponding owner-private internal event | direct Runtime continuation |
| capability resource state and receipts | corresponding capability Block | capability-private internal event | Orchestrator or another capability Block |
| provider request/response lifecycle | `blocks/llm` | LLM-private internal event | Runtime authority mutation |

Cross-owner EventHub commands/results carry immutable scoped facts only. They must not contain a target owner state,
complete mutable owner entity, repository/service handle, generic envelope, `map[string]any`, or transition callback.

## 4. One Owner-Local Mutation Pattern

Every state-owning component must use the same shape:

```text
typed external command or fact
  -> owner handler validates scope, identity and payload
  -> owner-private internal event
  -> owner-private reducer
  -> one owner repository/CAS write
  -> typed result or notification
```

Rules:

1. EventHub topics are defined by the owner of the addressed resource/capability.
2. A synchronous result reports a fact; it cannot instruct another owner which state to enter.
3. Internal events are not EventHub topics and cannot be submitted by Web, TUI, Application, LLM or another owner.
4. Pure validators may be shared through `workorchruntimecontract`; mutable ledgers and dispatchers may not.
5. In-memory maps may exist only as invocation-local lookup structures. They cannot become persisted/global authority.
6. Duplicate or stale facts produce a typed no-op/conflict through the same reducer; they never open a parallel path.

## 5. Canonical Authority And Admission Transaction

### 5.1 Existing canonical records

Do not introduce a second CWI, binding, request or dispatch model.

| Authority | Canonical record | Owner |
| --- | --- | --- |
| CWI identity, state, dependency and current binding ref | `RuntimeFacts.Coverage` | Orchestrator SM-2 |
| complete proof binding | existing sealed focus/reconciliation structures | Orchestrator SM-2 |
| semantic manifest/request session/document decision | existing Runtime semantic records | Orchestrator SM-3 |
| operation authorization | existing candidate manifest and sealed dispatch | Orchestrator SM-3 |
| owner execution outcome | owner typed receipt persisted in Runtime facts | capability owner fact; Orchestrator SM-3 admission |
| evidence, credit and reopen lineage | existing Runtime evidence/CWI ledger | Orchestrator SM-2 |
| recovery and terminal delivery progress | existing lease/delivery ledger | Orchestrator SM-4 |

### 5.2 Pure admission input

The pure evaluator must receive one immutable input assembled from already-persisted values:

- workspace, Run, definition and authority snapshot identity;
- canonical current CWI and its prerequisite closure;
- sealed source declaration and proof requirements;
- sealed owner catalog/admissibility facts and revisions;
- accepted complete semantic document;
- owner feasibility observations;
- current binding and retirement/reopen/effect lineage;
- candidate manifest and proposed operation graph; and
- active dispatch/request/reconciliation fences.

It must not query repositories, EventHub, files, Git, projections or provider text. It must not parse diagnostic
messages to recover authority.

### 5.3 Complete admission matrix

One evaluation must check all rows before a sealed dispatch can exist:

| Dimension | Required invariant | Failure class |
| --- | --- | --- |
| snapshot identity | workspace/Run/definition/authority identities all match current aggregate | stale or terminal contract defect |
| CWI identity | exactly one canonical current eligible CWI row exists | terminal contract defect |
| dependency | every declared prerequisite is settled, satisfied and currently bound | prerequisite blocked; zero provider/owner work |
| proposal scope | proposal names only current proof-batch CWI and sealed declaration refs | complete-document rejection |
| owner | operation kind belongs to the declared canonical owner | complete-document rejection |
| kind/variant | kind, variant and required operation ref match one sealed source alternative | complete-document rejection |
| target policy | target satisfies the variant's sealed policy and selector | complete-document rejection |
| owner catalog | existing/prospective target semantics do not contradict the same sealed catalog | correctable-once rejection |
| proof authority | evidence/check/effect/metric/artifact requirements equal declaration-compiled requirements | terminal contract defect |
| feasibility | observation matches owner, catalog revision, kind, variant, target and proposal hash | stale/prerequisite/infeasible typed fact |
| binding | exactly one canonical proof binding authorizes only the proof-bearing operation | terminal contract defect |
| operation graph | dependencies are acyclic; auxiliary operations carry no proof binding | terminal contract defect |
| retirement | active dispatch, accepted receipt/effect/evidence forbids implicit retirement | terminal contract defect |
| execution fence | dispatch, operation, binding, execution key and fence form one identity | stale/duplicate/conflict |
| lineage | successor binding requires explicit no-effect retirement or typed reopen lineage | terminal contract defect |
| request/reconciliation | active generations equal the current canonical binding set | stale/duplicate/conflict |

### 5.4 Bounded evaluator outcomes

The evaluator returns only a typed value such as:

- `admitted`;
- `correctable_once`;
- `prerequisite_blocked`;
- `owner_prerequisite_required`;
- `owner_stale`;
- `owner_infeasible`;
- `duplicate_noop`;
- `stale_conflict`; or
- `terminal_contract_failure`.

It does not choose the next action and does not update an attempt, CWI, binding, dispatch, retry or terminal. The
Orchestrator maps the result to exactly one private internal event. Correction remains the existing attempt-0/1
complete-document transaction; owner feasibility does not consume that budget.

### 5.5 Atomic seal

For `admitted`, one SM-3 internal event and one Runtime aggregate CAS must create or confirm all of:

1. exact current CWI/binding reference;
2. accepted semantic document identity;
3. candidate manifest identity;
4. operation graph with auxiliary/proof-bearing authorization;
5. sealed dispatch, execution keys and fences; and
6. the supersession status of prior request/reconciliation generations.

No partially sealed candidate may be visible to drive or recovery. A CAS conflict reloads the aggregate and repeats
the pure evaluation; it does not reuse a process-local decision.

## 6. Owner-Private Transition Catalogue

Names may follow existing Go naming, but the authority and atomic write set are fixed.

| Private event | SM | Accepted fact | Atomic write set |
| --- | --- | --- | --- |
| activation initialized | SM-2 | Run-owned persisted bootstrap | Definition facts and one canonical row per CWI |
| prerequisite fact observed | SM-2 | exact owner capability/admissibility fact | resource fact and reducer result |
| semantic authority snapshot sealed | SM-2 | complete current owner fact set | snapshot, eligible closure and request identity |
| semantic document evaluated | SM-3 | detached complete LLM result | accepted decision or typed rejection only |
| semantic correction opened | SM-3 | exact correctable attempt-0 rejection | one attempt-1 session; unchanged authority identity |
| semantic feasibility observed | SM-2 | exact owner feasibility result | observation only; no candidate correction |
| dispatch admission decided | SM-3 | pure complete-matrix decision | rejection or atomic binding/candidate/dispatch seal |
| no-effect binding retired | SM-2 | exact binding and typed retirement cause | lineage, CWI/focus and generation supersession |
| owner receipt admitted | SM-3 | exact execution receipt | receipt/effect lineage; no CWI credit yet |
| evidence committed | SM-2 | admitted binding-matched receipt/evidence | credit, settle/reopen and focus advance |
| reconciliation observed | SM-4 -> SM-2 | exact generation owner batch | audit plus one retain/reopen/fail transition |
| recovery lease changed | SM-4 | due lease or genuine CAS conflict | one persisted generation/state change |
| terminal delivery acknowledged | SM-4 | exact Result/Workspace/Run response | acknowledgement for one delivery stage only |

Catalog invalidation retires only catalog-dependent planning/session facts. It never clears a binding, reopens a CWI,
grants a successor binding, or changes Run lifecycle.

## 7. Component Closure Matrix

### 7.1 Components requiring code inspection or change

| Component | Required closure | Must not do |
| --- | --- | --- |
| `kernel/orchestrator/biz/runtime` | create one admission assembly/evaluation/seal path; route all semantic, feasibility, binding and dispatch paths through it; keep mutation in private reducers | duplicate local admission, direct field mutation, message-text transition inference |
| `kernel/orchestrator/biz` | keep `DriveRuntime` as the only synchronous drive entry; translate typed owner results to private events; use persisted lease/delivery ledger | expose lifecycle/next-step facade or maintain parallel retry state |
| `internal/pkg/workorchruntimecontract` | define neutral immutable admission input/decision and pure complete-matrix validation using existing canonical records | EventHub, repository, global registry, domain semantics or state mutation |
| `blocks/run` | keep SM-1 intent reducer and opaque Runtime aggregate CAS; ensure Resume retains current lineage and Restart creates a new lineage | choose CWI/action, inspect semantic authority or accept a caller-supplied Run entity |
| `application/workorchd/biz/appservice/activationcore` | compile template/profile Goal/DoD declarations into stable CWI, prerequisite, reopen and proof requirement facts | inspect live resources, infer Runtime transitions or mutate Runtime state |
| `blocks/llm` | accept one bounded candidate request identity; return one detached typed complete document/provider outcome | create proof requirements, credit, binding, retry/reopen or terminal authority |
| `blocks/file`, `git`, `shell`, `mcp`, `web`, `ssh` | return typed catalog/admissibility/feasibility facts and execute only exact sealed commands; receipts include complete factual identity | update CWI/binding/dispatch or decide next action |
| `blocks/mcpregistry` | expose only MCP resource/catalog facts from its own state | embed software-development template or Runtime policy |
| `blocks/result`, `workspace` | consume exact sealed terminal delivery through owner-private reducer | adjudicate Runtime acceptance or accept direct repository mutation |
| `blocks/decision`, `approval` | own ticket/resolution lifecycle; return typed scoped resolution | directly resume/patch Runtime state |
| `blocks/eventstream` and application projections | project committed notifications only | rebuild authority or acknowledge a transition on observation alone |
| Web/TUI/CLI services | submit typed user intent and display projections | infer retry/resume or mutate owner state from UI state |

### 7.2 Components expected to remain structurally unchanged

These still require boundary scanning, but should not receive Runtime state logic:

- `initiators/workorchruntime`: repository factory/provider only;
- `kernel/system`: startup/configuration ownership only;
- `kernel/governance`: governance policy composition only;
- `blocks/memory`, `skill`, `soul`: their own resources and typed facts only.

If closing this plan requires adding a CWI, binding, dispatch, retry or lifecycle reducer to one of these components,
the design has diverged and implementation must stop.

### 7.3 All 22 registered units boundary gate

The final boundary scan covers 1 Initiator, 4 Modules and 17 Blocks. For every unit verify:

1. no cross-owner service/repository/adapter injection;
2. EventHub command/data/result are concrete typed owner contracts;
3. no `map[string]any`, generic envelope or mutable entity crosses authority boundaries;
4. topics are defined by the resource/capability owner;
5. owner state changes occur only through its private reducer;
6. projection and notifications are never consumed as authoritative state; and
7. no Runtime behavior is hard-coded to a template, language, test framework or MCP recipe.

## 8. Ordered Code Closure Queue

Items are sequential. A later item cannot compensate for a missing earlier invariant.

### 8.1 Execution status

| Item | Code/automated status | Closure evidence |
| --- | --- | --- |
| CWI-SO-13 | closed | The sole production mutation chain is `Biz.submitRuntimeInternalEvent -> RuntimeController.ApplyInternalEvent -> RuntimeTransitionCoordinator.Commit`; private events are classified by SM, and the boundary scanner rejects direct CAS, escaped admission input and alternate dispatch construction. |
| CWI-SO-14 | closed | `EvaluateRuntimeCompleteAdmission` consumes a detached invocation-local view and returns only bounded typed dispositions. Matrix tests cover snapshot/CWI/dependency/semantic/manifest/candidate/owner-fact/binding/retirement/dispatch/replay identities, typed owner outcomes, normalization/hash stability and input immutability. |
| CWI-SO-15 | closed | `admitRuntimeDispatch` is the sole production dispatch constructor. Provider candidates pass complete admission; reducer-authored governance dispatches pass the same active/manifest/seal fence without fabricating a provider attempt. `candidate_sealed` closes the attempt and creates dispatch in one aggregate CAS. |
| CWI-SO-16 | closed | Existing explicit catalog retirement, no-effect binding retirement, receipt admission, evidence credit, reconciliation and reopen reducers pass the retirement/effect, duplicate/stale, restart and current-run retention matrix; no second ledger or transition was added. |
| CWI-SO-17 | closed | Run/LLM/capability/governance/delivery clone and scope tests plus the 22-unit boundary scan pass. Feasibility records now preserve the exact candidate-manifest identity returned by the owner, and empty/mismatched identities fail before persistence. |
| CWI-SO-18 | automated closed; deployed live pending | Focused/full/race/vet/fmt/boundary and managed file-backed verification pass. A fresh deployment is required because the only persisted Runtime schema is now v5; fresh `/run restart` combined evidence remains open. |

There is no compatibility reconstruction from schema v4. The v5 change is deliberate: candidate feasibility now persists
`CandidateManifestHash`, so a v4 aggregate cannot prove that an owner observation belongs to the current sealed proposal.
Run owner reports the non-current aggregate as `fresh_activation_required`; Orchestrator does not infer the missing value.

### CWI-SO-13 — Inventory every mutation and decision site

Affected machines: SM-1 through SM-4. Code change is limited to tests/scanners unless a violation is found.

Deliverables:

- list every write to CWI, focus, binding, semantic session, candidate, dispatch, evidence, recovery and terminal
  delivery fields;
- map each write to one owner-private event and reducer;
- list every admission decision site currently outside the future evaluator;
- classify duplicate checks as precondition, complete admission, receipt admission or aggregate invariant; and
- extend `verify-boundary-contract.sh` to reject direct cross-owner repository/service and forbidden provider proof
  authority patterns where mechanically detectable.

Exit gate: every mutable Runtime field has one owner and one reducer entry; every admission decision has one planned
landing point. Unknown writes block CWI-SO-14.

### CWI-SO-14 — Pure complete admission contract

Affected machines: SM-2/SM-3; no state mutation.

Primary landing:

- `internal/pkg/workorchruntimecontract`

Deliverables:

- immutable admission input assembled from existing canonical types;
- typed bounded outcome and stable diagnostic paths;
- one validator covering every row in section 5.3;
- clone/normalize/hash behavior that cannot alias owner state; and
- no duplicate persisted admission entity.

Required tests:

- table test for every individual matrix row;
- pairwise tests for dependency × target, target × catalog, kind × variant, binding × execution fence,
  retirement × effect, and proof authority × proof owner;
- permutation stability and hash stability;
- unknown/provider-controlled fields fail closed; and
- evaluator performs no I/O and mutates no input.

Exit gate: all known `run-073` through `run-082` counterexample shapes fail at the same pure boundary before dispatch.

### CWI-SO-15 — One Orchestrator admission dispatcher and atomic seal

Affected machine: SM-3, with SM-2 snapshot input.

Primary landing:

- `internal/modules/kernel/orchestrator/biz/runtime`
- `internal/modules/kernel/orchestrator/biz/runtime_semantic_drive.go`

Deliverables:

- one assembler from persisted aggregate facts;
- one call to the CWI-SO-14 evaluator;
- one mapping from outcome to private internal event;
- one reducer CAS that seals binding, candidate and dispatch together; and
- removal of local branches that independently admit target, variant, proof requirement or binding.

Required tests:

- no candidate/dispatch is visible before atomic seal;
- CAS conflict reload repeats evaluation against new persisted facts;
- duplicate result causes zero additional CAS/dispatch;
- attempt-0/1 correction is unchanged and only evaluator `correctable_once` can open attempt 1;
- owner stale/prerequisite/infeasible never consumes correction; and
- restart at every pre/post-seal boundary produces no partial transaction or duplicate operation.

Exit gate: all production dispatch creation flows pass through one dispatcher and one private reducer event.

### CWI-SO-16 — Canonical retirement, reopen and receipt/evidence convergence

Affected machines: SM-2/SM-3/SM-4.

Deliverables:

- catalog generation retirement remains independent from binding retirement;
- no-effect binding retirement has one explicit typed cause and atomic lineage update;
- accepted receipt/effect/evidence locks the authorizing binding until explicit typed reopen;
- receipt admission records execution/effect facts without directly granting CWI credit;
- evidence commit alone grants binding-matched credit and advances focus; and
- receipt- and reconciliation-triggered reopen use one SM-2 transition.

Required tests:

- retirement × active dispatch and retirement × accepted effect fail closed;
- catalog invalidation alone never reopens or clears a binding;
- stale/duplicate receipt and reconciliation batch are no-op;
- current-run satisfied proof survives workspace-full reconciliation;
- explicit reopen creates exactly one successor lineage; and
- process restart after every CAS repeats no durable owner effect.

Exit gate: there is no implicit `bound -> planning`, no second binding without retirement/reopen lineage, and no
receipt-only credit path.

### CWI-SO-17 — Cross-owner contract narrowing

Affected machines: none directly; protects all owner boundaries.

Deliverables:

- Run commands carry scoped intent, not complete owner entities or target lifecycle state;
- LLM result is detached and non-authoritative;
- capability facts/receipts contain exact scope and identity but no CWI target state;
- governance resolution remains a typed fact consumed once by Orchestrator;
- terminal delivery consumers derive owner-local state only in their reducers; and
- projection/UI contracts remain read-only.

Required tests:

- command/result clone/detach tests;
- caller mutation after `Send` cannot change stored owner state;
- malformed/missing scope fails before handler mutation;
- duplicate delivery is idempotent per owner; and
- production dependency scan finds no cross-owner service/repository path.

Exit gate: every cross-owner Runtime interaction is typed fact exchange; no external caller can submit an internal
transition or complete mutable owner entity.

### CWI-SO-18 — Combined fault and deployed closure matrix

Affected machines: all four are verified; no owner changes.

Automated gates:

```bash
go test ./internal/pkg/workorchruntimecontract/... -count=1
go test ./internal/modules/application/workorchd/biz/appservice/activationcore/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtime/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/... -count=1
go test ./internal/modules/blocks/run/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
make fmt-check
make vet
make verify-boundary-contract
make test
make test-race
WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file ./scripts/verify-workorch-managed-full.sh
```

Deployed proof uses a fresh `/run restart` and records bounded persisted facts for:

1. dependency × target-policy admission;
2. target-policy × sealed owner catalog;
3. kind × variant × required operation reference;
4. binding × execution key × fence;
5. retirement × active dispatch;
6. retirement × accepted durable effect;
7. metric/artifact authority × unique proof owner;
8. reconciliation × current-run satisfied CWI;
9. restart/resume × stale and duplicate receipt;
10. receipt × evidence × exact CWI advance;
11. terminal delivery × partial acknowledgement × restart; and
12. all 22 component boundary assertions.

Exit gate: the fresh workspace progresses without manual CWI/evidence JSON, produces no duplicate durable effect,
and either reaches a truthful success terminal or a deliberately injected typed failure with correct delivery order.

## 9. Regression Matrix

| Scenario | Required assertion |
| --- | --- |
| blocked prerequisite | zero provider request, feasibility probe, binding, dispatch or side effect |
| provider orders dependent first | SM-2 eligibility wins; provider order grants no authority |
| existing target proposed as prospective | complete-document rejection before owner probe |
| target changes after catalog snapshot | typed owner stale result; new bounded generation, no correction/side effect |
| auxiliary `add` plus proof-bearing `commit` | only commit has one exact binding and earns credit |
| provider supplies artifact/metric fields | strict decode rejects unknown authority fields |
| declaration has artifact/metric requirement | exactly one compatible proof receives it before binding seal |
| ambiguous proof owner | seal fails before capability dispatch |
| catalog invalidates effect-bearing binding | binding and effect lineage remain locked |
| explicit no-effect retirement | one retirement lineage and one eligible planning projection |
| accepted receipt arrives twice | one receipt/effect fact and one evidence opportunity |
| evidence arrives before/mismatched receipt | zero credit and typed failure/conflict |
| workspace-full reconciliation after current proof | retain credit; no reopen/request/operation |
| explicit declared invalidation | reopen exact dependent rows once, preserving audit |
| CAS conflict before seal | reload and reevaluate; no partial candidate/dispatch |
| crash after seal before owner result | exact-fence recovery; at most one owner effect |
| crash after owner receipt before evidence | persist/recover same receipt; no replay; one credit commit |
| partial terminal delivery | retain acknowledged stages; retry only missing stage |
| `/run resume` | retain current-Run lineage and do not re-plan settled CWI |
| `/run restart` | new Run/lineage; no predecessor credit/request identity |
| legacy or ambiguous aggregate | `fresh_activation_required`; zero provider/capability work |
| large Goal/DoD | C0-C2 stable/complete; C3-C5 bounded by active dependency closure |

## 10. Documentation And Status Rules

1. A code slice is `code-closed` only after its focused and combined automated matrix passes.
2. It is not `live-closed` until fresh deployed persisted evidence covers its claimed path.
3. A new live counterexample reopens the matching invariant row, not a new field-specific plan.
4. The first response to a counterexample is to run the whole admission matrix against the new shape.
5. Historical logs belong under `docs/90-archive`; this active plan retains only counterexample identity and invariant.
6. Active overview and unclosed matrix must match this queue. They cannot claim only live verification remains while
   CWI-SO-13 through CWI-SO-17 are open.

## 11. Completion Criteria

Code closure requires all of the following:

- every Runtime mutable field maps to one owner-private event and reducer;
- SM-1 is Run-owned and SM-2/SM-3/SM-4 are Orchestrator-owned;
- one pure complete admission evaluator covers the whole matrix;
- one Orchestrator dispatcher and atomic CAS seal binding, candidate and dispatch;
- provider output cannot create proof, dependency, binding, credit, retry or transition authority;
- capability owners return facts/receipts and never mutate Runtime state;
- catalog retirement, binding retirement and CWI reopen are distinct transitions;
- accepted effects cannot lose or silently replace their authorizing binding;
- receipt admission and evidence credit are separate and exact-binding matched;
- Run, Workspace, Result and governance owners mutate only their own state through private reducers;
- projection, Web, TUI and Eventstream remain read-only consumers;
- Runtime core remains template/domain neutral;
- no global index, fallback, second authority model or fifth state machine exists; and
- CWI-SO-13 through CWI-SO-18 pass their exit gates.

Live closure additionally requires the deployed combined matrix in CWI-SO-18 with zero unintended duplicate effect.

## 12. Explicit Non-Solutions

The following are prohibited:

- adding another check only at the latest failing field or owner;
- creating a global CWI, target, check, event-contract or owner registry;
- allowing provider/candidate output to declare artifact, metric, dependency, binding or credit;
- treating target as artifact or operation success as proof credit;
- treating catalog invalidation as binding retirement or CWI reopen;
- allowing a receipt handler to settle/reopen CWI without SM-2 evidence commit;
- using diagnostic/free text, projection, UI state, files or Git state to rebuild authority;
- adding cross-owner state-transition commands or accepting complete owner entities;
- introducing compatibility reconstruction, V1 fallback, replay-based migration or implicit restart repair;
- adding a retry/correction counter outside existing persisted request/recovery ledgers;
- hard-coding software-development templates, paths, languages, test frameworks or MCP recipes in Runtime core; or
- claiming live closure from mock/automated verification alone.
