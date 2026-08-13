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

### 2.4 Live counterexample: `run-085` focus/eligibility divergence

The fresh schema-v5 `/run restart` performed on 2026-08-13 created a new `run-085` with a new activation request and
no predecessor credit or request identity. The run then accepted the durable `patch/apply` evidence for `cwi-043` and
correctly retired the affected prerequisite observations. It failed before the next dispatch with:

```text
runtime_contract_defect: runtime CWI semantic seal omits active coverage "cwi-048"
```

The persisted facts show one Orchestrator-owned inconsistency, not an owner capability failure:

1. the focus queue advanced by taking `RemainingFocuses[0]`, which was `cwi-048`;
2. `cwi-048` still had unsettled prerequisites and therefore was not in
   `RuntimeCWIsEligibleForPlanning`;
3. semantic planning correctly compiled and requested the eligible closure
   `cwi-044`, `cwi-045`, and `cwi-046`;
4. semantic sealing still required a binding for the current focus `cwi-048`;
5. the seal failed even though the semantic batch itself was correctly bounded to its eligible CWI.

This is a single authority defect expressed at three existing boundaries: focus advancement, eligible-closure
selection, and semantic seal. It does not justify a second CWI ledger, a provider-side correction, a relaxed seal
check, or a cross-owner state transition. The closure rule is now:

```text
SM-2 internal fact
  -> exact eligible closure
  -> Orchestrator-owned focus projection
  -> accepted semantic coverage order over that closure
  -> proof batch and owner feasibility
  -> seal of the same closure
```

`Facts.Focus`, `RuntimeCWIsEligibleForPlanning`, the semantic manifest, coverage order, proof batch, feasibility set,
and reconciliation binding set must all describe the same persisted closure. A blocked CWI may remain in the ordered
queue, but it cannot be the unbound current focus of a nonterminal planning generation and cannot be required by the
active semantic batch. If unfinished CWI exist but none is eligible, SM-2 must preserve the last canonical cursor for
audit and form a typed dependency-contract terminal; it must not fabricate an executable focus.

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

### 5.6 One eligible-closure and focus projection

SM-2 must use one pure projection for every focus-selection site. The projection consumes the persisted CWI ledger
and returns a detached decision containing:

- the exact eligible CWI closure in Definition-seed order;
- the current bound/in-flight focus, when one is fenced against replacement;
- the next unbound current focus, selected only from the eligible closure;
- the remaining eligible focuses in an accepted semantic order when one exists;
- blocked focuses retained in stable prior order; and
- a typed conflict when no legal projection can preserve current binding/effect lineage.

The three orders have different roles and must not be conflated:

| Order | Source | Authority |
| --- | --- | --- |
| Definition order | activation-sealed `DefinitionSeed.CoverageItems[].Order` | stable fallback identity and deterministic tie-break |
| presentation order | activation-compiled acceptance slices and priorities | display and bounded request grouping only |
| execution focus order | SM-2 eligible closure plus accepted semantic `coverageOrder` | sole Runtime cursor order |

The projection rules are fixed:

1. A CWI is eligible only when its convergence state is `unbound`, `planning`, or explicitly `reopened`, and every
   declared prerequisite is currently `settled`, `satisfied`, and has a current binding ref.
2. A current focus that is already bound, owns an active dispatch, or has accepted receipt/effect/evidence lineage is
   fenced in place until its existing settle, retirement, reopen, or terminal transition completes.
3. An unbound current focus that is not eligible must be moved back to the stable blocked portion of the queue in the
   same owner-local CAS that selects an eligible replacement. This changes only the focus projection; it does not
   change the blocked CWI state or dependency.
4. Before an accepted semantic coverage order exists, eligible CWI use Definition order. This is only a deterministic
   request cursor; it grants no binding or execution authority.
5. An accepted `coverageOrder` must be a complete permutation of the manifest's exact eligible closure. Missing,
   additional, duplicate, settled, blocked, or effect-locked CWI make the complete document invalid.
6. SM-3 first persists the accepted complete semantic-order document. Orchestrator then submits a separate SM-2
   private event that atomically installs the eligible focus projection. Only after that CAS is visible may a later
   SM-3 private event open the first proof batch. No internal event crosses two state machines, and no provider or
   application result directly writes `Facts.Focus`.
7. A dependency, CWI state, binding, reopen, retirement, or catalog change that alters the exact eligible closure
   makes the active semantic generation stale. SM-2 creates a new bounded generation after persisted re-projection;
   it never patches the old manifest or order.
8. Semantic seal requires the unbound current focus, manifest eligible closure, accepted coverage order, active proof
   batch, owner feasibility set, and pending lineage to share the same closure identity.

No new persisted queue or scheduling state is required. `RuntimeFacts.Coverage`, `Focus`, `RemainingFocuses`, and
`CompletedFocuses` remain canonical. The implementation may introduce a detached pure projection/decision type in
`workorchruntimecontract`, but only an Orchestrator-private event may apply it to the aggregate.

### 5.7 Eligible-focus owner-local transition

All focus changes must use one owner-private transition, named according to existing Go conventions, with this shape:

```text
persisted SM-2 fact or accepted semantic-order fact
  -> clone RuntimeFacts
  -> evaluate exact eligible closure and binding/effect fences
  -> validate projection against the current semantic generation
  -> atomically write Focus/RemainingFocuses/CompletedFocuses/Repair
  -> validate aggregate
  -> RuntimeTransitionCoordinator CAS
```

Legal triggers are limited to:

- activation initialization;
- evidence or assessment commit that settles the current CWI;
- prerequisite commit that changes dependency admission;
- explicit binding retirement or CWI reopen;
- reconciliation observation that retains or reopens exact CWI;
- accepted semantic coverage order; and
- semantic seal when it settles or installs an exact binding projection.

Catalog invalidation alone may retire the semantic generation but may not reorder a bound/effect-locked focus.
Duplicate facts must return the existing aggregate without a new revision. CAS conflicts must reload and recompute
the projection; a process-local projection cannot be replayed as authority.

The existing seal invariant that rejects omitted active coverage remains mandatory. The code closure changes the
focus selected before seal; it must not weaken `sealRuntimeCWIBindings` or admit a batch for an ineligible focus.

## 6. Owner-Private Transition Catalogue

Names may follow existing Go naming, but the authority and atomic write set are fixed.

| Private event | SM | Accepted fact | Atomic write set |
| --- | --- | --- | --- |
| activation initialized | SM-2 | Run-owned persisted bootstrap | Definition facts and one canonical row per CWI |
| prerequisite fact observed | SM-2 | exact owner capability/admissibility fact | resource fact and reducer result |
| semantic authority snapshot sealed | SM-2 | complete current owner fact set | snapshot, eligible closure and request identity |
| eligible focus projected | SM-2 | persisted CWI/dependency ledger or accepted semantic-order fact | current/remaining/completed focus projection and focus repair ledger |
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

`eligible focus projected` is not a public EventHub command and not another state machine. It is the single internal
mutation used wherever the current implementation would otherwise take `RemainingFocuses[0]`, directly install a
semantic batch order, or independently repair a blocked current focus.

## 7. Component Closure Matrix

### 7.1 Components requiring code inspection or change

| Component | Required closure | Must not do |
| --- | --- | --- |
| `kernel/orchestrator/biz/runtime` | create one admission assembly/evaluation/seal path and one eligible-focus projection reducer; route activation, settle, reopen, retirement, prerequisite, semantic-order and seal focus writes through it; keep mutation in private reducers | take the remaining queue head without eligibility, duplicate local admission, direct field mutation, or infer transition from message text |
| `kernel/orchestrator/biz` | keep `DriveRuntime` as the only synchronous drive entry; translate typed owner results to private events; compile semantic requests only after persisted eligible-focus projection; use persisted lease/delivery ledger | expose lifecycle/next-step facade, choose focus in an adapter, or maintain parallel retry/queue state |
| `internal/pkg/workorchruntimecontract` | define neutral immutable admission and eligible-focus projection input/decision; provide pure dependency, complete-permutation and complete-matrix validation using existing canonical records | EventHub, repository, global registry, domain semantics, persisted scheduler or state mutation |
| `blocks/run` | keep SM-1 intent reducer and opaque Runtime aggregate CAS; ensure Resume retains current lineage and Restart creates a new lineage | choose CWI/action, inspect semantic authority or accept a caller-supplied Run entity |
| `application/workorchd/biz/appservice/activationcore` | compile template/profile Goal/DoD declarations into stable CWI, dependency DAG, Definition order, presentation slices, reopen and proof requirement facts | convert presentation priority into the live execution cursor, inspect live resources, infer Runtime transitions or mutate Runtime state |
| `blocks/llm` | accept one bounded candidate request identity; return one detached typed complete document/provider outcome; order only the exact eligible closure supplied in the semantic manifest | add blocked/settled CWI, create focus/proof requirements/credit/binding, or choose retry/reopen/terminal authority |
| `blocks/file`, `git`, `shell`, `mcp`, `web`, `ssh` | return typed catalog/admissibility/feasibility facts and execute only exact sealed commands; receipts include complete factual identity | update CWI/binding/dispatch or decide next action |
| `blocks/mcpregistry` | expose only MCP resource/catalog facts from its own state | embed software-development template or Runtime policy |
| `blocks/result`, `workspace` | consume exact sealed terminal delivery through owner-private reducer | adjudicate Runtime acceptance or accept direct repository mutation |
| `blocks/decision`, `approval` | own ticket/resolution lifecycle; return typed scoped resolution | directly resume/patch Runtime state |
| `blocks/eventstream` and application projections | project committed current/blocked/eligible focus notifications only | rebuild order/authority or acknowledge a transition on observation alone |
| Web/TUI/CLI services | submit typed user intent and display committed focus/dependency projections | infer ordering/retry/resume or mutate owner state from UI state |

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
| CWI-SO-13 | reopened by `run-085` | The mutation chain is owner-private, but the audit missed a second decision rule: `advanceSatisfiedCoverageFocus` selects the queue head while semantic planning selects only eligible CWI. Inventory and scanners must cover every focus-selection decision, not only repository writes. |
| CWI-SO-14 | reopened by `run-085` | `EvaluateRuntimeCompleteAdmission` validates dependency rows, but current focus, exact eligible closure, semantic manifest/order/batch and seal are not yet bound by one pure projection/identity. |
| CWI-SO-15 | closed | `admitRuntimeDispatch` is the sole production dispatch constructor. Provider candidates pass complete admission; reducer-authored governance dispatches pass the same active/manifest/seal fence without fabricating a provider attempt. `candidate_sealed` closes the attempt and creates dispatch in one aggregate CAS. |
| CWI-SO-16 | reopened by `run-085` | Receipt/evidence lineage is correct, but evidence settlement can advance to a prerequisite-blocked queue head. Focus advance, semantic-order acceptance and reopen/retirement re-projection must share one SM-2 internal transition. |
| CWI-SO-17 | closed | Run/LLM/capability/governance/delivery clone and scope tests plus the 22-unit boundary scan pass. Feasibility records now preserve the exact candidate-manifest identity returned by the owner, and empty/mismatched identities fail before persistence. |
| CWI-SO-18 | reopened; live failed | Fresh `run-085` proved new lineage/schema-v5 admission and correct owner receipts, but failed on focus/eligibility divergence. Automated and deployed combined verification must be rerun after CWI-SO-13/14/16 close. |

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
- inventory every read or write that selects, advances, requeues or replaces `Facts.Focus`, including pure helpers that
  choose `RemainingFocuses[0]`, semantic batch helpers and activation-time queue projection;
- classify Definition order, presentation slice order and execution focus order separately; and
- prove that only one Orchestrator-private event may apply execution focus order to the aggregate.

Exit gate: every mutable Runtime field has one owner and one reducer entry; every admission and focus-selection
decision has one planned landing point. Production scan finds no direct queue-head execution selection outside the
eligible-focus reducer. Unknown writes or decisions block CWI-SO-14.

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
- a detached `RuntimeEligibleFocusProjection` input/decision (or equivalently narrow existing type) using the current
  canonical CWI/focus records only;
- one pure evaluator for prerequisite eligibility, current binding/effect fences, accepted semantic-order permutation,
  stable blocked ordering and repair-ledger rebinding; and
- one invariant tying current unbound focus, exact eligible closure, semantic manifest/order/batch and pending lineage.

Required tests:

- table test for every individual matrix row;
- pairwise tests for dependency × target, target × catalog, kind × variant, binding × execution fence,
  retirement × effect, and proof authority × proof owner;
- permutation stability and hash stability;
- unknown/provider-controlled fields fail closed; and
- evaluator performs no I/O and mutates no input.
- queue head blocked while a later CWI is eligible;
- all CWI blocked, with zero provider/feasibility/dispatch authorization;
- bound or effect-locked current focus cannot be reordered;
- semantic order is an exact permutation of eligible CWI; and
- Definition ordering is stable under input permutations and presentation-slice priority changes.

Exit gate: all known `run-073` through `run-085` counterexample shapes either fail at the same pure boundary before
provider/dispatch or produce one deterministic legal eligible-focus projection. The evaluator performs no mutation or
I/O and introduces no persisted scheduler.

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
- one `eligible focus projected` private event/reducer applies focus changes for activation, evidence/assessment
  settle, prerequisite change, binding retirement, CWI reopen, reconciliation and semantic-order acceptance;
- `advanceSatisfiedCoverageFocus` no longer selects the remaining queue head independently of prerequisite
  eligibility;
- accepted semantic order reprojects only unbound eligible focuses, while blocked focuses retain stable lineage;
- focus re-projection and the new focus repair ledger are written in the same Runtime aggregate CAS; and
- semantic generation is retired and rebuilt when its exact eligible closure changes.

Required tests:

- retirement × active dispatch and retirement × accepted effect fail closed;
- catalog invalidation alone never reopens or clears a binding;
- stale/duplicate receipt and reconciliation batch are no-op;
- current-run satisfied proof survives workspace-full reconciliation;
- explicit reopen creates exactly one successor lineage; and
- process restart after every CAS repeats no durable owner effect.
- the persisted `run-085` shape advances from settled `cwi-043` to an eligible member of `cwi-044/045/046`, never to
  blocked `cwi-048`;
- a provider order that names blocked `cwi-048` is rejected without changing the queue;
- duplicate evidence, prerequisite and reconciliation events do not reproject or increment revision; and
- CAS conflict reload recomputes eligibility instead of applying a stale process-local order.

Exit gate: there is no implicit `bound -> planning`, no second binding without retirement/reopen lineage, no
receipt-only credit path, and no prerequisite-blocked unbound current focus. Every focus change passes through the
single Orchestrator-private projection transition.

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
12. all 22 component boundary assertions;
13. queue-head blocked × later eligible focus projection;
14. accepted semantic order × exact eligible closure identity;
15. eligible-closure change × semantic-generation retirement/rebuild; and
16. `run-085` regression: settled `cwi-043` must not advance to blocked `cwi-048`.

Exit gate: the fresh workspace progresses without manual CWI/evidence JSON, produces no duplicate durable effect,
and either reaches a truthful success terminal or a deliberately injected typed failure with correct delivery order.

### 8.2 Reopened implementation sequence

The `run-085` closure is performed in the following order. Each slice must pass its focused gate before the next
slice begins; a later adapter or guard cannot compensate for an incomplete earlier owner invariant.

#### FOCUS-01 — Freeze the live counterexample

Status: code-closed. A neutral blocked-head/later-eligible fixture and the
`run-085` shape are covered by `runtime_cwi_ledger_test.go` and the Orchestrator
projection regression. No domain-specific template or path is used.

Primary landing:

- `internal/modules/kernel/orchestrator/biz/runtime/cwi_ledger_schema_test.go`
- `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning_test.go`
- `internal/modules/kernel/orchestrator/biz/runtime/dispatch_commit_test.go`
- focused test support only where an existing fixture cannot express the persisted shape

Create a neutral fixture equivalent to `run-085` without embedding the LSP/software domain:

- one settled source CWI with exact binding-matched durable evidence;
- one queue-head dependent CWI whose prerequisites remain unsettled;
- at least two later independent eligible CWI;
- a retired semantic generation caused by an owner resource revision change; and
- no active dispatch when the next semantic generation begins.

The failing baseline must prove that a raw queue-head advance chooses the blocked CWI while
`RuntimeCWIsEligibleForPlanning` excludes it. The final assertion must require the current unbound focus to belong to
the eligible closure and require zero provider/feasibility/dispatch work for the blocked CWI.

Exit gate: the fixture reproduces the authority conflict before implementation and contains no domain-specific path,
tool, language or template assumption.

#### FOCUS-02 — Add the pure eligible-focus projection contract

Status: code-closed. `ProjectRuntimeCWIEligibility` is detached, deterministic,
dependency-aware, validates exact semantic permutations, reports blocked and
fenced current rows, and has alias/order regression coverage.

Primary landing:

- `internal/pkg/workorchruntimecontract/runtime_cwi_ledger.go`
- `internal/pkg/workorchruntimecontract/runtime_cwi_ledger_test.go`
- `internal/pkg/workorchruntimecontract/canonical_runtime.go` only for aggregate invariants
- a focused neutral contract file when the existing CWI ledger file would mix unrelated concerns

Implement detached pure values/functions for:

1. exact prerequisite eligibility;
2. current binding/effect/dispatch fence input;
3. complete semantic-order permutation validation;
4. deterministic eligible and blocked projections; and
5. validation that an unbound current focus belongs to the exact eligible closure.

The pure contract must clone all slices/maps it returns, must not mutate `RuntimeFacts`, and must not import EventHub,
repositories, Orchestrator Biz, profile/template code or capability-owner packages.

Exit gate: table/permutation/alias tests pass, including blocked head, all blocked, bound current, effect-locked current,
valid/invalid semantic order and presentation-priority independence.

#### FOCUS-03 — Introduce one Orchestrator-private projection transition

Status: code-closed. `eligible_focus_projected` is an Orchestrator-private
SM-2 event. It reloads the aggregate, recomputes the pure projection, applies
one CAS, and treats duplicate projections as zero-revision no-ops.

Primary landing:

- `internal/modules/kernel/orchestrator/biz/runtime/internal_event.go`
- `internal/modules/kernel/orchestrator/biz/runtime/internal_event_authority.go`
- `internal/modules/kernel/orchestrator/biz/runtime/internal_event_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime/cwi_ledger.go`
- `internal/modules/kernel/orchestrator/biz/runtime/transition_coordinator.go` only through its existing CAS API

Add or narrow one private internal event for eligible-focus projection. Its payload may carry a persisted causation
identity or an already validated semantic order, but it must not accept a caller-authored complete `RuntimeFacts`,
focus queue, target state or action. The reducer reloads the aggregate, recomputes the pure projection, writes the four
canonical focus/repair fields, validates the aggregate and commits through the existing transition coordinator.

Required assertions:

- external EventHub has no corresponding topic;
- duplicate input is a zero-revision no-op;
- stale semantic-order/closure identity is rejected;
- CAS conflict does not apply the pre-conflict projection; and
- the event metadata remains `receiver_owner_private` and SM-2 classified.

Exit gate: there is exactly one CAS-writing focus-projection entry and no new owner, ledger, retry counter or state
machine.

#### FOCUS-04 — Route every focus decision through the private transition

Status: code-closed for production focus writes. All canonical queue field
writes are installed by `installRuntimeCWIFocusProjection`; semantic order
acceptance dispatches the separate SM-2 projection event before proof-batch
compilation. Boundary scanning rejects alternate writes and queue-head reads.

Primary landing:

- `internal/modules/kernel/orchestrator/biz/runtime/reducer.go`
- `internal/modules/kernel/orchestrator/biz/runtime/cwi_ledger.go`
- `internal/modules/kernel/orchestrator/biz/runtime/controller.go`
- `internal/modules/kernel/orchestrator/biz/runtime/dispatch_commit.go`
- `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go`
- `internal/modules/kernel/orchestrator/biz/runtime_semantic_drive.go`

Replace every independent execution cursor decision:

- activation installs the initial canonical projection through the same pure rule;
- evidence and assessment settlement no longer take `RemainingFocuses[0]`;
- prerequisite commit re-evaluates admission before semantic planning;
- binding retirement/reopen/reconciliation use the same projection;
- accepted semantic `coverageOrder` is validated and applied before the first proof batch;
- semantic seal consumes the already projected current eligible focus; and
- catalog invalidation preserves bound/effect-locked focus and only reprojects after the matching persisted fact.

`advanceSatisfiedCoverageFocus` must either become a pure settle helper called by the one projection reducer or be
removed after all production callers migrate. No alternate wrapper may retain direct queue-head selection.

Exit gate: code search and boundary scan find no production focus assignment or remaining-queue head selection outside
initial normalization and the private projection reducer.

#### FOCUS-05 — Bind semantic generation to exact closure identity

Status: code-closed for the active order/batch boundary. The projection event
rejects stale manifest/order identity and the pure contract rejects blocked,
missing, duplicate, or extra CWI in an accepted order. Full generation-retire
live proof remains part of FOCUS-07.

Primary landing:

- existing semantic manifest/document/batch types in `internal/pkg/workorchruntimecontract`
- `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go`
- `internal/modules/kernel/orchestrator/biz/runtime_semantic_drive.go`

Use existing manifest/hash fields where sufficient. Do not add a second closure entity merely to copy CWI IDs. The
accepted coverage order, proof batches, pending IDs, feasibility results, merged document and sealed reconciliation
set must all validate against the manifest's exact eligible CWI set and the post-order persisted current focus.

If focus eligibility changes after request start, retire the generation through the existing typed semantic retirement
path and compile a new generation. Do not patch the old order, append the newly eligible CWI, or reuse its complete
provider session as current authority.

Exit gate: no semantic document can be sealed for a blocked/non-current focus, and a legal batch for the projected
current focus passes the existing strict active-coverage seal check.

#### FOCUS-06 — Remove obsolete decisions and enforce boundaries

Status: code-closed for the Runtime focus boundary. `make verify-boundary-contract`
passes with structural checks for the single focus reducer and no production
`RemainingFocuses[0]` execution selection.

Primary landing:

- `scripts/verify-boundary-contract.sh`
- focused source scanners/tests under existing verification packages
- documentation comments describing presentation versus execution order

Remove or make unreachable:

- direct `RemainingFocuses[0]` execution selection;
- semantic helpers that prefer a blocked current focus over accepted eligible order;
- application/profile code that treats slice priority as live cursor authority;
- adapter-side focus mutation or retry; and
- duplicate focus normalization that can persist state outside the private reducer.

Scanner rules should be structural and narrowly scoped; they must not create a global symbol/type/event index.

Exit gate: `make verify-boundary-contract` proves the single owner/mutation path and Runtime core neutrality.

#### FOCUS-07 — Combined automated and live closure

Status: pending. The new focused tests and boundary gate pass. The full runtime
suite still contains the pre-existing `candidate_proposal_mismatch` baseline
failures (reproduced from HEAD), and a fresh deployed `/run restart` has not
yet supplied post-fix persisted evidence.

Run the full CWI-SO-18 automated matrix, then deploy and issue a fresh `/run restart`. The live record must show:

1. new Run/activation/request lineage and schema v5;
2. settled current CWI advances only to an eligible CWI;
3. blocked `cwi-048` remains pending until all prerequisites settle;
4. persisted SM-3 order, following SM-2 focus projection, and later SM-3 proof batch/feasibility/seal use one closure
   identity without a cross-machine event;
5. exact owner receipt and evidence credit remain one-to-one;
6. restart/recovery creates no duplicate provider session, dispatch or durable effect; and
7. terminal delivery remains `Result -> Workspace -> Run`.

Exit gate: the fresh deployed run no longer produces `semantic seal omits active coverage`, and either continues
truthful convergence or stops at a different deliberately typed owner/contract fact. A new failure must be evaluated
against the entire admission matrix before any code change.

## 9. Regression Matrix

| Scenario | Required assertion |
| --- | --- |
| blocked prerequisite | zero provider request, feasibility probe, binding, dispatch or side effect |
| provider orders dependent first | SM-2 eligibility wins; provider order grants no authority |
| queue head is prerequisite-blocked | same-CAS projection selects the first legal eligible focus; blocked CWI remains unchanged in the stable remaining queue |
| all remaining CWI are blocked | no unbound current execution focus is fabricated; reducer remains in prerequisite/terminal contract flow with zero provider or owner work |
| presentation priority puts dependent first | presentation order changes no execution authority; eligible closure and Definition-order fallback remain stable |
| semantic order omits/adds/duplicates CWI | complete document rejection; zero focus mutation, proof batch, feasibility or dispatch |
| semantic order is a valid eligible permutation | SM-3 persists the accepted document, a following SM-2 event installs eligible execution order, and only a later SM-3 event opens its first bounded proof batch |
| current focus is bound or effect-locked | new semantic ordering cannot replace, retire or requeue the current focus |
| eligibility changes after manifest seal | retire the exact semantic generation and compile a new bounded generation after persisted focus projection |
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
| `run-085` persisted shape | after `cwi-043` settles, current focus belongs to eligible `cwi-044/045/046`; `cwi-048` remains blocked until all prerequisites settle |

## 10. Documentation And Status Rules

1. A code slice is `code-closed` only after its focused and combined automated matrix passes.
2. It is not `live-closed` until fresh deployed persisted evidence covers its claimed path.
3. A new live counterexample reopens the matching invariant row, not a new field-specific plan.
4. The first response to a counterexample is to run the whole admission matrix against the new shape.
5. Historical logs belong under `docs/90-archive`; this active plan retains only counterexample identity and invariant.
6. Active overview and unclosed matrix must match this queue. They cannot claim only live verification remains while
   any CWI-SO implementation item is reopened.

## 11. Completion Criteria

Code closure requires all of the following:

- every Runtime mutable field maps to one owner-private event and reducer;
- SM-1 is Run-owned and SM-2/SM-3/SM-4 are Orchestrator-owned;
- one pure complete admission evaluator covers the whole matrix;
- one pure eligible-focus evaluator defines the exact executable closure and validates semantic ordering;
- one Orchestrator-private reducer applies every execution focus projection;
- current unbound focus, semantic manifest/order/batch/feasibility and seal share one exact closure identity;
- presentation slice priority never selects the Runtime execution cursor;
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
- deleting or weakening the semantic seal check for current active coverage;
- allowing `RemainingFocuses[0]`, acceptance-slice priority, UI order or provider order to bypass prerequisite eligibility;
- moving focus selection into Application, LLM, Run, a capability owner, projection or a shared mutable scheduler;
- adding a second persisted eligible queue, focus index or process-global CWI scheduler;
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
