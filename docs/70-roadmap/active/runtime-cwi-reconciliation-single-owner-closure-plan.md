# Runtime CWI Reconciliation Single-Owner Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-08-10

## 1. Purpose And Authority

This is the only active implementation queue for Runtime lifecycle, semantic planning, CWI reconciliation, and
single-owner state-machine closure. It is subordinate to:

1. `docs/00-core/runtime-stage-contract.md`;
2. `docs/00-core/runtime-request-cache-note.md`;
3. `internal/modules/kernel/orchestrator/readme.md`;
4. `internal/pkg/workorchruntimecontract`.

Archived plans and previous green artifacts are regression evidence only. They cannot override the latest persisted
aggregate, owner event order, provider audit, or projection counterexample.

The product contract remains unchanged: a user supplies a template, Goal, DoD, workspace directory, and optional
workspace inference selection. WorkOrch derives a bounded semantic plan, obtains owner feasibility, seals authority,
executes operations, collects evidence, and assesses the DoD. The user does not author Runtime JSON, targets, recipes,
checks, metrics, or per-CWI proof contracts.

## 2. Latest Live Assessment

Fresh deployed validation of workspace `ws-a8bada88e7b1b5f8`, Runs `run-061` and `run-062`, is the current fact baseline.
`run-054` through `run-060` observations remain historical regression evidence only.

### 2.1 Verified Owner-Correct Behavior

- Run owner performed the SM-1 lifecycle and terminalized the Run as `failed`.
- Terminal delivery used the required order: Result persisted the rejected outcome, Workspace released the exact
  `ActiveRunID`, and Run then entered its terminal state.
- File, Git, MCP, and Shell returned typed prerequisite/admissibility facts. The Git capability-state binding that was
  missing in an earlier deployment was present.
- SM-2 did not create binding credit, operation candidate, dispatch, receipt, evidence, governance, or workspace effect
  before the semantic contract was sealed.
- SM-3 sealed the expected-document identity. Attempt 0 and attempt 1 retained the same `coverage_order` plan step,
  manifest hash, Definition seed hash, stable prefix, and authority identity while using distinct logical requests.
- Provider request handling remained bounded and did not use raw suffix assembly or document continuation recovery;
  the latest run reached both coverage-order and proof-batch request steps before admission failure.
- The terminal aggregate stopped driving after failure; no provider loop, CAS loop, or high-CPU loop was observed.

### 2.2 Earlier Semantic Correction Gap (Code Closed)

Run `run-032` passed the previously uncovered order and empty-required-array cases, but failed on a sealed owner
catalog constraint:

```text
validationRecipeId is outside owner catalog
```

Admission still collapsed this to `path=$.proposals; actual=contract_violation`. The correction contract therefore
does not yet identify the affected CWI, proposal index, exact `validationRecipeId` path, returned value, or allowed
recipe catalog. This is the remaining semantic-plan contract gap; it must be closed as part of the unified evaluator
track below, not fixed with another provider-specific prompt patch.

### 2.3 Historical Projection Gap (Code Closed)

The authoritative Run state and convergence state were both `failed`, and projection revisions were fully caught up.
The Workspace RuntimeView nevertheless reported an `active`, non-terminal outcome with the summary
`workspace definitions changed; execution must be rerun`. Its activity feed repeated the derived active outcome.

This was a prior counterexample. The projection closure now selects the authoritative Run before applying definition
attention, and the latest run has `sourceEventRevision=479` and `projectedSourceRevision=479`. No new projection
divergence was observed in `run-032`.

### 2.4 Historical Run-032 Paths Not Proven By That Run

`run-032` did not reach semantic seal, operation candidate, sealed dispatch, capability execution, receipt, evidence
admission, CWI advance, assessment success, or durable workspace output. It did exercise Result rejection and terminal
delivery. SM-4 whole-document recovery and the positive semantic-to-effect lineage remain live gates.

### 2.5 Subsequent Proof-Batch Counterexample And Code Closure

A later deployed run passed coverage ordering and reached the first proof batch, exposing three narrower contract
defects that supersede the earlier local-green conclusion:

- the proof-batch provider document repeated the complete workspace `coverageOrder` even though SM-3 had already
  persisted that order; this expanded the response and let a batch document restate global authority;
- a malformed or structurally rejected attempt-0 proof batch had no typed semantic violation, so SM-3 could not seal a
  bounded correction and terminalized as `runtime_contract_defect` instead of issuing the sole allowed attempt 1;
- projection compared the Run owner's combined Goal/DoD Definition revision with one individual DoD revision, so a
  terminal Run could be displayed as definition-drift active.

The code closure keeps the accepted order only in the persisted order document/batch ledger. Provider-facing
proof-batch schema and payload now contain only exact batch identity, exact batch CWI membership, and proposals;
`batchSequence: 0` remains explicit. For an attempt-0 proof-batch rejection, SM-3 derives one typed
`proof_batch_document` violation from the sealed expected document, binds it to the exact active batch, and permits only
attempt 1. The private reducer independently verifies that binding before CAS. Projection now computes the same combined
Definition identity through the Workspace owner contract used by Run activation. These changes affect SM-3 and the
read-only SM-4 projection delivery surface; they do not give the LLM or projection transition authority.

### 2.6 Empty Proposal Constraint Counterexample And Code Closure

Historical deployed `run-031` proved that coverage-order correction and exact proof-batch scoping now work: attempt 1
repaired `cwi-046 -> cwi-043`, and the first proof batch contained only `cwi-046`, `cwi-043`, `cwi-001`, and
`cwi-002`. The batch nevertheless failed twice because proposals returned an empty required set, including:

The historical provider payload contained the now-deleted field:

```json
"requiredEffectScopes": []
```

The provider schema required the field but permitted an empty array, while admission required at least one effect.
Admission then collapsed the failure to `$.proposals`, so attempt 1 received only batch membership and could not repair
the exact CWI field. This was one logical proposal contract implemented independently by schema, validator, and
correction synthesis.

The code closure makes required evidence arrays non-empty and enum-bounded in the provider schema and preserves
an exact `proposal_constraint` fact through admission. Effect/idempotency are no longer provider proposal fields; they
are compiled from owner facts under RSO-23. The fact includes authoritative CWI ID, diagnostic proposal
index, exact JSON path, rule, actual value, and required/allowed values. SM-3 accepts that fact only when its CWI belongs
to the active sealed batch, then persists it in the existing attempt-1 correction directive. LLM remains a fact
producer/renderer; no additional state machine, authority owner, EventHub transition, or retry was added.

### 2.7 Unified Proposal Evaluator And Single-Owner Closure

The catalog failure proved that schema generation, proposal validation, and correction synthesis must not be maintained
as separate partial implementations. The code closure now uses one Orchestrator-owned evaluation/reduction flow. A
fresh deployment still has to prove the provider-facing result. The canonical flow is:

```text
LLM returns complete candidate document
  -> Orchestrator/SM-3 canonical evaluator
  -> owner-neutral typed proposal violations
  -> Orchestrator-private rejected/evaluated fact event
  -> SM-3 derives correction from sealed manifest + active batch + rejection
  -> one CAS-persisted attempt-1 directive or final rejection
  -> LLM renders only detached correction facts in C4
```

The canonical evaluator must cover, with the same rules used to render the provider schema:

- required and non-empty fields/arrays;
- evidence/effect relational constraints;
- owner/operation and operation-variant ownership;
- target policy, target refs, selectors, and prospective constraints;
- validation recipe and required-check membership in the owner catalog;
- batch membership, duplicate/missing CWI and plan-step identity;
- dependency ordering against the sealed Definition seed.

Each violation carries the authoritative `coverageWorkItemId`, diagnostic proposal index, exact JSON path, stable rule
(`required`, `non_empty`, `allowed_value`, or `relational`), actual value, and required/allowed values. CWI identity is
authoritative; proposal index is display-only.

SM-3 is the only component allowed to derive and persist `RuntimeSemanticCorrectionDirective`. The private evaluated or
rejected event carries facts only; callers must not submit a pre-decided correction or next transition. A correction is
accepted only when every referenced CWI is in the active sealed batch and the source rejection, manifest, Definition
seed, stable prefix, plan step, and attempt identity match the persisted ledger. Attempt 1 remains the sole retry.

No new state machine is introduced. SM-1 remains `blocks/run`; SM-2/SM-3/SM-4 remain `kernel/orchestrator`. LLM and
capability Blocks remain fact producers. Workspace, Result, projection, TUI, Web, and Application remain owners of
their existing state or read-only surfaces and cannot trigger Runtime transitions.

### 2.8 Target Policy Counterexample And Closure

After the empty sealed-catalog value fix was deployed, `run-034` produced the first field-level target fact:

```text
path=$.proposals[0].target.exactTargetRefs
actual=empty_array
required=at least one target ref
```

The evaluator is now precise, but attempt 1 still failed to produce an admissible target. This is not evidence of
cross-component state ownership. It shows that exact-target, existing-owner-scope, and prospective-owner-target rules
are not yet one canonical contract from schema through correction rendering.

The target closure must define one pure policy contract:

```text
exact_target:
  exactTargetRefs >= 1 and every ref is in owner catalog TargetRefs

existing_owner_scope:
  ownerScopeTargetRefs >= 1 OR ownerSelectorRefs >= 1
  ownerSelectorRefs <= 1
  every ref/selector is in the sealed owner catalog

prospective_owner_target:
  prospectiveTargetRef is non-empty
  ownerSelectorRefs == 1
  prospectiveConstraintRefs >= 1
  selector/constraint values are in the sealed owner catalog
```

The same policy rules must drive provider schema, document admission, typed violation generation, and correction
rendering. When a finite allowed target set exists, SM-3 emits one `allowed_value` or `non_empty` correction containing
the sealed allowed refs. When no admissible target exists in the owner catalog, SM-3 records an unrepairable catalog gap
and terminates or requests the existing prerequisite refresh path; it must not spend another ineffective LLM attempt.

LLM remains a renderer of detached target facts. File/Git/Shell/MCP/Web/SSH remain catalog and feasibility fact owners.
Only Orchestrator SM-3 may classify target correction and transition the semantic ledger.

### 2.9 Dynamic Capability Identity And Owner-Scoped Schema Closure

Fresh live validation exposed a second contract collision: an MCP tool identity such as
`mcp/tool_call/mcp-lsp-bridge/lsp_status` was emitted as both `operationVariantId` and `validationRecipeId`.
MCP tools have no static `RuntimeOperationVariantContract`; the tool/recipe identity belongs to the MCP owner
catalog. Shell validation recipes likewise remain Shell-owned. A semantic proposal must omit `operationVariantId`
for dynamic operations and use only the owner-scoped recipe/capability fact.

The request schema and evaluator are now owner/kind-scoped from the sealed manifest. They no longer expose a global
union of selectors, target refs, constraints, recipes, or static variants. Empty catalog values encode as an empty enum
array, never JSON `null`; an empty admissibility domain is a bounded catalog gap, not an invitation to guess.

The Orchestrator remains the only component that turns these facts into an SM-3 correction/CAS transition. MCP, Shell,
File, and Git continue to return typed catalog/feasibility facts only; LLM remains a detached fact renderer.

### 2.10 MCP Capability Effect Counterexample And Active Closure

Fresh `run-054` proved that the typed MCP metadata default is deployed and that the mainline now reaches semantic seal,
sealed dispatch, MCP execution, receipt persistence, evidence commit, and terminal delivery. It then failed closed:

```text
semantic binding: lsp_initialize requires effect [none]
sealed execution: expected effect ephemeral_session
owner receipt: observed effect ephemeral_session
SM-2 admission: coverage_binding_mismatch
```

The safe parts behaved correctly: no CWI credit was granted, Result stored the rejected outcome, Workspace released the
exact active Run, Run entered `failed`, and the aggregate stopped driving. The defect is earlier than evidence
admission. MCP semantic feasibility checked server, recipe, selector, and transport readiness but did not compare the
proposal effect with the exact capability owner's declared effect. The recipe was therefore sealed as feasible even
though its proof shape could never match the owner receipt.

The same live trace exposed two related duplicate-authority risks:

- recipe identities are derived from `Server.AllowedTools`, while effect/idempotency are stored in the capability
  catalog;
- MCP receipts currently may populate `ObservedEffectScope` from the caller's sealed expected effect instead of an
  exact owner capability/execution fact.

This plan therefore keeps evidence admission strict and moves the missing check to the owner-feasibility boundary. It
must not add an MCP name branch to Runtime, infer effect from risk/tool names, or let Orchestrator repair registry state.

### 2.11 Latest Live Counterexamples: Provider-Owned Effect Echo Is Still Authoritative

Fresh `run-061` and `run-062` prove that the preceding schema/evaluator closure is fail-closed but not convergent. The
provider was still asked to submit `requiredEffectScopes`. It alternated between two valid owner recipes:

```text
lsp_status:      actual=ephemeral_session, allowed=none
lsp_initialize:  actual=none,             allowed=ephemeral_session
```

Both runs stopped before dispatch (`dispatch=0`, `receipt=0`, `evidence=0`). This is not an MCP metadata defect and
must not be repaired with tool-name branches, prompt wording, larger retry budgets, or a second correction loop. The
remaining contract violation is that a provider-facing proposal field duplicates an owner fact and lets the LLM choose
an authority value that the owner can only reject.

The required closure is RSO-23: remove `requiredEffectScopes` from the provider proposal, wire schema, DTO, decoder,
hash/clone/merge and correction surface. The field is removed without a compatibility or fallback path; a payload that
contains the deleted field is rejected as an unknown field. The original provider response remains available in the
bounded diagnostic/audit record, while the authoritative effect and idempotency facts are retained in the owner catalog,
owner feasibility observation, SM-2 `CoverageEvidenceBinding`, sealed dispatch, receipt, and evidence.

### 2.12 Latest Live Counterexample: Provider-Owned Check Echo

Fresh deployed `run-063` proves the effect closure is active and exposes the next duplicate authority:

```text
semantic_contract_sealed
  -> 47 owner feasibility observations
  -> SM-2 binding seal
  -> 1 dispatch
  -> 1 MCP receipt
  -> evidence admission rejected
```

The sealed binding required the provider-authored check `lsp_status_returns_ten_tools`, while the MCP receipt could only
return the owner recipe check `mcp/tool_call/mcp-lsp-bridge/lsp_status`. The run therefore failed with
`coverage_binding_mismatch` after dispatch. No CWI credit was granted and terminal delivery remained owner-correct, but
the mismatch should have been rejected before dispatch.

This is RSO-24, a cross-owner contract closure rather than an LSP-specific fix. The provider must not author
`requiredCheckRefs`; canonical checks are returned by the capability owner and compiled by SM-2. If an owner cannot
execute or observe a check that proves the requested CWI, feasibility is `unverifiable`/`infeasible` and dispatch is zero.
The failure must not be classified as `no_progress_exhausted` when no repair attempt occurred.

## 3. Non-Negotiable Ownership Rules

1. There are exactly four Runtime state machines. Semantic planning, request sessions, reconciliation, restart,
   projection, activity, and scheduling do not introduce another state machine.
2. SM-1 is owned only by `blocks/run`. Activation, blocked recovery, cancellation, supersession, and terminal lifecycle
   transitions occur only through Run-private events and the Run reducer.
3. SM-2, SM-3, and SM-4 are owned only by `kernel/orchestrator`. Their state changes occur only through exhaustive
   Orchestrator-private internal events and the single Runtime aggregate CAS coordinator.
4. Workspace state, definition selection, and `ActiveRunID` are owned only by `blocks/workspace`; Result lifecycle is
   owned only by `blocks/result`.
5. Cross-owner EventHub contracts carry immutable typed intent, fact, query, receipt, or acknowledgement. They never
   carry target state, next action, retry choice, terminal choice, reducer instruction, repository, mutable owner entity,
   or unbounded raw provider content.
6. LLM and capability owners return facts only. They cannot choose correction, retry, Runtime action, coverage credit,
   governance, or terminal outcome.
7. Application, scheduler, projection, eventstream, TUI, and Web submit intent or display facts only. They cannot infer a
   transition and feed it back into an owner.
8. `workorchruntimecontract` contains owner-neutral DTO, normalize, validate, clone, and deterministic hash helpers only.
   It contains no reducer, EventHub dispatch, repository access, owner selection, retry policy, or terminal policy.
9. Run persists the Runtime aggregate as opaque data and provides semantic CAS. It does not interpret or rebuild
   SM-2/3/4 authority.
10. A complete semantic contract is sealed before operation candidate generation. Only SM-2 can derive a
    `CoverageEvidenceBinding`; only a binding-matched owner receipt can create CWI credit.

## 4. Unified Target Flow

```text
Application compiles typed RuntimeBootstrap and submits execution intent
  -> Workspace owner validates selection/definition and owns ActiveRunID
  -> Run owner handles SM-1 activation or predecessor replacement
  -> Orchestrator SM-4 drives the persisted aggregate
  -> SM-2 requires a semantic contract
  -> SM-3 seals expected coverage-order document identity
  -> LLM owner returns one complete-document or provider fact
  -> Orchestrator evaluator returns accepted or typed factual rejection
  -> SM-3 private event persists the rejection
  -> SM-3 reducer seals one typed correction directive or a final rejection
  -> accepted order lets SM-2/SM-3 compile bounded proof batches
  -> capability owners return typed semantic feasibility facts
  -> SM-2 seals binding/reconciliation authority
  -> SM-2 derives operation authority
  -> SM-3 seals and dispatches the operation candidate
  -> capability owner returns a typed receipt
  -> SM-2 admits binding-matched evidence and advances focus
  -> SM-4 delivers one terminal outcome: Result -> Workspace -> Run
  -> application projection maps the owner facts without re-adjudication
```

## 5. Closure Track A: Semantic Correction Is SM-3 State

### 5.1 Separate Three Contracts

The implementation must keep these values separate:

| Contract | Producer | Meaning | Must not contain |
| --- | --- | --- | --- |
| `RuntimeOriginalDiagnostic` | evaluator/producer adapter | complete factual rejection and audit identity | retry, correction disposition, terminal choice |
| `RuntimeSemanticCorrectionDirective` | SM-3 private reducer | the one bounded same-contract delta allowed for attempt 1 | raw provider body, raw `OriginalMessage`, owner action, authority expansion |
| provider-safe C4 projection | LLM owner renderer | bounded diagnostic plus the sealed directive | reconstruction from error prose or transient context |

The existing semantic correction hash binds a rejection identity but is not itself a correction directive. A hash does
not tell the provider which exact value, order, or field must change.

### 5.2 Typed Directive Shape

Add one owner-neutral immutable value with at least:

- schema version and correction ID/hash;
- workspace/run scope only where required by the containing private ledger;
- semantic plan step;
- manifest hash and Definition seed hash;
- stable prefix hash;
- source rejection hash and attempt `1`;
- an ordered, bounded list of structured violations;
- each violation's exact path, actual shape/value, and required/allowed/forbidden delta.

For a dependency-order rejection, the directive must be structurally equivalent to:

```text
path=$.coverageOrder
actual=cwi-043 appears before cwi-046
required=cwi-046 before cwi-043
```

Do not encode this as a new free-form instruction string. Dependency endpoints must remain typed CWI IDs so validation
can prove that both belong to the same manifest and that the required edge is sealed.

### 5.3 Owner-Private Transition Sequence

```text
complete semantic document
  -> evaluator returns accepted or RuntimeOriginalDiagnostic
  -> semantic_plan_evaluated private event persists the factual result
  -> semantic_plan_correction_required private event asks the SM-3 reducer to classify it
  -> reducer verifies manifest + attempt ledger + typed violations
       -> unique bounded delta: persist correctable_once + directive
       -> no safe delta or attempt already used: persist rejected_final
       -> diagnostic cannot be classified: runtime_contract_defect
  -> drive reads the persisted directive and issues attempt 1
  -> attempt-1 complete document re-enters the same evaluator
```

No EventHub topic exposes these transition events. The LLM command carries a cloned directive as immutable input; it
does not receive a reducer instruction.

### 5.4 Identity And Validation Rules

- Attempt 1 must keep plan step, manifest, Definition seed, stable prefix, expected-document shape, and batch identity
  identical to attempt 0.
- The directive source rejection hash must equal the sole persisted attempt-0 rejection.
- Every referenced field/CWI/dependency/allowed value must be derivable from the same sealed manifest and rejection.
- A directive that changes owner catalog, target authorization, operation permission, batch membership, or plan step is
  invalid; SM-2 must form new authority instead.
- Duplicate correction events do not change state revision or create another request.
- A stale directive cannot be reused after manifest, Definition seed, or authority identity changes.
- Attempt 1 rejection cannot create attempt 2.
- `OriginalMessage`, provider body, details content, and raw evidence remain absent from provider-facing C4.

### 5.5 LLM Boundary

Extend the semantic-only fields of the LLM owner's preflight/request command and internal request model with the typed
directive. Semantic and operations fields remain mutually exclusive.

The LLM owner must:

- validate directive identity against the semantic context and expected document;
- clone/detach all slices before retaining or returning data;
- render the directive only in C4;
- preserve C0-C3 byte/hash stability between attempts;
- reject attempt 1 without exactly one valid directive;
- reject attempt 0 carrying any directive;
- avoid all correction classification and next-step selection.

## 6. Closure Track B: Projection Must Not Re-Adjudicate Outcome

### 6.1 Canonical Precedence

Workspace RuntimeView must use one deterministic precedence:

```text
selected current/display Run has terminal owner state
  -> map that Run terminal outcome
selected Run is active/blocked/pending
  -> map that Run lifecycle state
no selected Run and current definitions require execution
  -> expose a workspace attention hint, not a Run outcome
```

Workspace resource state `active` is not equivalent to Run execution outcome `active`. Definition drift/restart-needed
is not a terminal or lifecycle outcome.

### 6.2 Projection Contract

- `WorkspaceOutcome` is derived from the selected authoritative Run and its matching terminal Result.
- A definition-change signal moves to an explicit read-only attention/action field, or remains absent when the contract
  does not require one. It must not overwrite `Outcome.State`, `Outcome.Terminal`, `Outcome.RunID`, or terminal summary.
- Activity outcome items derive from the same canonical outcome function and selected Run identity. They cannot infer a
  second state from Workspace state.
- `RunState`, convergence state, task progress, heartbeat, outcome, and activity must agree on the selected Run and
  runtime state revision.
- Source/projected revision equality proves freshness only; it must not be treated as semantic correctness.
- TUI and Web render the projection contract. They do not resolve conflicts locally or issue lifecycle commands from a
  displayed attention hint.

### 6.3 Projection Staleness And Workspace Switching

- A projection item whose workspace/run/revision does not match the selected display Run is omitted, not merged.
- Terminal outcomes remain visible after Workspace releases `ActiveRunID` by selecting the latest matching Run.
- Switching active workspaces changes only the presentation subscription/cursor; no activity or outcome from another
  workspace may be reused.
- Duplicate SSE/EventHub projection notifications do not duplicate activity items.

## 7. Closure Track C: Preserve Closed Runtime Baselines

The following work is code-closed and remains mandatory regression coverage, not active redesign:

- predecessor-owned replacement intent and release-before-successor materialization;
- one SM-4 terminal delivery path in `Result -> Workspace -> Run` order;
- sealed expected-document identity;
- removal of raw suffix/fragment authority assembly;
- bounded whole-document recovery from the beginning of the same expected document;
- same-plan-step correction and persisted proof-batch identity;
- owner-local EventHub topics and concrete command/result contracts;
- one Runtime aggregate mutation dispatcher and semantic CAS boundary;
- provider request size/batch bounds and stable C0-C5 cache layout.

The corresponding symbols and alternate paths must remain absent:

```text
FreshActivationRelease
FreshActivationRunSlotReleaseFact
successor-owned pending ReplacementTerminationIntent
authority document raw suffix concatenation
provider prompt requesting exact next bytes
continuation fragment admitted as authority
projection- or application-selected Runtime transition
```

## 7.1 Closure Track D: Effective MCP Capability Is One Owner Fact

This track closes the `run-054` counterexample without adding a state machine or a cross-owner mutation path.

### 7.1.1 Ownership And State Boundaries

| State/fact | Sole owner | Legal transition/input |
| --- | --- | --- |
| MCP server/capability catalog, metadata, enabled state, catalog revision | `blocks/mcpregistry` | registry owner-private mutation event and repository CAS |
| MCP transport/session execution state | `blocks/mcp` | MCP owner-private execution/session event |
| effective MCP admissibility and semantic feasibility observation | `blocks/mcp` | detached fact compiled from typed registry query plus owner-local probe |
| semantic feasibility ledger, binding seal, candidate, dispatch, receipt admission, CWI credit | `kernel/orchestrator` SM-2/SM-3/SM-4 | Orchestrator private internal event and Runtime aggregate CAS |
| Run lifecycle | `blocks/run` SM-1 | Run owner-private lifecycle event |

`mcpregistry` must not drive Runtime. `mcp` must not persist or mutate registry entities. Orchestrator must not read the
registry repository, infer a tool effect, or write MCP state. Application, LLM, TUI, Web, projection, Result, Workspace,
and Run do not participate in MCP feasibility state.

### 7.1.2 Canonical Effective Capability Fact

Remove the split semantic authority between server `AllowedTools` and capability metadata. Keep `AllowedTools` only as
the server transport/admission whitelist. MCP owner compiles one bounded effective fact from the intersection of an
enabled server, its whitelist, and an enabled exact capability:

```go
type RuntimeEffectiveMCPCapability struct {
    CapabilityID       string
    ServerID           string
    ToolName           string
    OperationKind      string
    EffectScope        RuntimeEffectScope
    IdempotencyClass   RuntimeOperationIdempotencyClass
    ApprovalPolicy     string
    Enabled            bool
    CapabilityRevision string
    CatalogRevision    string
}
```

The concrete EventHub query/result belongs to `blocks/mcpregistry/pkg/events`. The owner-neutral detached value,
normalization, validation, clone, and stable hash helper belong to `workorchruntimecontract`. It contains no repository,
Hub, callback, state transition, retry choice, Runtime action, or mutable entity.

An effective tool is unavailable when any required identity or typed effect/idempotency field is empty/unknown. Unknown
metadata remains a diagnostic catalog fact but cannot become an executable semantic recipe. No risk-level, tool-name,
stage-hint, or template inference is allowed.

### 7.1.3 One Catalog Revision

The MCP semantic admissibility revision must hash the normalized effective capability set, including:

- server ID and enabled state;
- capability ID, tool name, operation kind, enabled/status state;
- effect scope and idempotency class;
- approval policy and capability revision;
- whitelist membership relevant to the effective projection.

A change to any field produces a new catalog revision. Server-only hashing and parallel recipe/effect revisions are
removed. A feasibility request bound to an older revision returns `stale`; MCP owner does not select reacquisition.
Orchestrator first persists the stale observation through its private event, then re-enters its existing bounded catalog
acquisition generation without consuming semantic candidate correction attempt.

### 7.1.4 Semantic Manifest And Owner Feasibility

The persisted semantic manifest (private Orchestrator input, not provider authority) projects each dynamic MCP recipe
together with its finite effect domain:

```json
{
  "validationRecipeId": "mcp/tool_call/<server>/<tool>",
  "operationKind": "mcp/tool_call",
  "allowedEffectScopes": ["ephemeral_session"],
  "capabilityRef": "<server>:tool:<tool>",
  "catalogRevision": "<sealed revision>"
}
```

The LLM receives only recipe identity and operation/target/check facts. It does not receive or submit effect or
idempotency authority. MCP owner remains the final factual feasibility source. For the exact recipe it must validate, in
order:

1. workspace/run/definition/request scope;
2. catalog revision freshness;
3. exact server/tool/capability identity and enabled state;
4. operation kind;
5. compile the canonical effect/idempotency from the owner fact;
6. target/selector/constraint and approval facts;
7. transport/prerequisite probe when applicable.

An unavailable, stale, or incomplete owner fact returns `infeasible`/`stale` with a bounded original diagnostic. It does
not return correction, retry, action, terminal, coverage, or binding state. Orchestrator converts the fact to its existing
owner-private semantic feasibility event; only SM-2/SM-3 may persist rejection, compile the canonical effect, or seal the
binding.

### 7.1.5 Dispatch And Receipt

The sealed operation authorization binds exact capability ref, catalog revision, operation kind, effect,
idempotency, target policy, recipe/check, and CWI binding. Immediately before execution, MCP owner verifies that the
exact effective capability still matches the sealed identity. Drift returns a typed stale/infeasible receipt and no
tool call.

`ObservedEffectScope` must be derived from MCP owner capability/execution facts. The following assignment pattern is
forbidden:

```go
receipt.ObservedEffectScope = command.Execution.ExpectedEffectScope
```

Expected effect belongs to the sealed dispatch; observed effect belongs to the owner receipt. SM-2 compares both with
the sealed binding. A mismatch remains fail-closed and grants zero CWI credit.

### 7.1.6 Required Deletions And Boundary Scans

- delete server-only semantic recipe hashing when it competes with the effective capability revision;
- delete parallel string-list recipe/effect authority at the MCP Runtime boundary;
- delete receipt construction that echoes caller expected effect as owner observation;
- do not add LSP/software/template names to `blocks/mcp`, `blocks/mcpregistry`, Orchestrator, or Runtime contracts;
- do not add direct `mcp -> mcpregistry` service/repository injection; use the registry owner's concrete EventHub query;
- do not create a generic capability registry, global topic index, map payload, compatibility alias, or second Runtime
  mutation facade.

## 8. Component Work Matrix

| Component | Required closure | Explicitly forbidden |
| --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | canonical pure proposal/effective-capability facts, schema projection, normalize/validate/clone/hash | reducer, retry classification, owner lookup, EventHub |
| `kernel/orchestrator/biz/runtime` | invoke one evaluator, map feasibility facts to owner-private events, derive/persist directive and binding through SM-2/SM-3 CAS | provider rendering, capability/effect inference or execution, cross-owner repository |
| `kernel/orchestrator/biz` | drive reads persisted directive and maps LLM facts to private events | reconstructing directive from raw message at send time |
| `blocks/llm/pkg/events` | semantic command carries detached directive; result remains factual | correction disposition, terminal/action fields |
| `blocks/llm/pkg/models` | owner-private immutable request representation | Runtime aggregate or mutable owner entity |
| `blocks/llm/biz` | validate detached request identity and render provider-safe C4 | proposal validation, infer delta, retry, terminal, coverage, batch transition |
| `blocks/mcpregistry` | sole catalog owner; validate/persist typed capability metadata and return detached catalog query facts/revision | Runtime transition, MCP transport execution, cross-owner repository exposure |
| `blocks/mcp` | compile effective capability facts, exact recipe/effect feasibility, transport probe/execute, owner-observed receipt | registry mutation, Runtime action/correction/credit, expected-effect echo as observation |
| `application/workorchd/.../readmodelprojection` | terminal-first canonical outcome and activity derivation | competing execution state machine |
| `workorchprojectioncontract` | separate outcome from optional attention fact | authority or lifecycle command |
| TUI/Web/event mapping | render canonical outcome/activity and workspace switch isolation | local conflict resolution or state transition |
| `blocks/run` | regression only: SM-1 and replacement owner-private events | SM-2/3/4 interpretation |
| `blocks/workspace` | regression only: exact active-run claim/release | Run or Runtime terminal decision |
| `blocks/result` | regression only: idempotent sealed outcome storage | acceptance adjudication |
| File/Git/Shell/Web/SSH | regression only: typed feasibility/receipt facts | Runtime action, evidence credit, retry/terminal |

No component may receive another owner's service, repository, reducer, mutable model, or state callback.

The matrix covers the complete registered 22-unit baseline. Units not changed by RSO-5 through RSO-8 remain explicit
regression boundaries rather than implicit exclusions:

- Initiator: `internal/initiators/workorchruntime`.
- Application/kernel Modules: `internal/modules/application/workorchd`, `internal/modules/kernel/governance`,
  `internal/modules/kernel/orchestrator`, `internal/modules/kernel/system`.
- Resource and capability Blocks: `internal/modules/blocks/approval`, `internal/modules/blocks/decision`,
  `internal/modules/blocks/eventstream`, `internal/modules/blocks/file`, `internal/modules/blocks/git`,
  `internal/modules/blocks/llm`, `internal/modules/blocks/mcp`, `internal/modules/blocks/mcpregistry`,
  `internal/modules/blocks/memory`, `internal/modules/blocks/result`, `internal/modules/blocks/run`,
  `internal/modules/blocks/shell`, `internal/modules/blocks/skill`, `internal/modules/blocks/soul`,
  `internal/modules/blocks/ssh`, `internal/modules/blocks/web`, `internal/modules/blocks/workspace`.

## 9. Primary Files To Audit

Semantic correction:

- `internal/pkg/workorchruntimecontract/runtime_semantic_*.go`
- `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_semantic_drive.go`
- `internal/modules/kernel/orchestrator/biz/runtime_candidate_event_adapter.go`
- `internal/modules/blocks/llm/pkg/events/messages.go`
- `internal/modules/blocks/llm/pkg/models/runtime_candidate.go`
- `internal/modules/blocks/llm/biz/runtime/candidate_boundary.go`
- `internal/modules/blocks/llm/biz/runtime_semantic_provider.go`

Projection consistency:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_outcome*.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/run_outcome*.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_runtime_view.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_activity*.go`
- `internal/pkg/workorchprojectioncontract`
- `internal/modules/application/workorchd/pkg/eventmapping`
- `internal/services/workorch/service/tui`
- `web/workorch-console/src`

MCP capability/effect closure:

- `internal/modules/blocks/mcpregistry/pkg/events`
- `internal/modules/blocks/mcpregistry/pkg/models`
- `internal/modules/blocks/mcpregistry/biz/mcp_service.go`
- `internal/modules/blocks/mcp/biz/mcp_runtime_adapter.go`
- `internal/modules/blocks/mcp/biz/owner.go`
- `internal/modules/blocks/mcp/pkg/events/runtime_admissibility.go`
- `internal/pkg/workorchruntimecontract/runtime_effect_resolve.go`
- `internal/pkg/workorchruntimecontract/runtime_semantic_feasibility_*.go`
- `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go`
- `internal/pkg/workorchdefaults/defaults/mcp_capabilities.json`

## 10. Ordered Implementation Queue

Statuses are `pending | in_progress | code_closed | live_closed`. A code-closed item without required deployed proof
remains open.

| ID | Status | Work item | Completion evidence |
| --- | --- | --- | --- |
| RSO-1 | code_closed | Single-owner core contract, typed owner EventHub boundaries, one Runtime mutation surface | boundary scans and owner contract tests remain green |
| RSO-2 | code_closed | Predecessor-owned restart intent and release-before-successor sequence | restart crash/duplicate/stale/workspace-isolation tests |
| RSO-3 | code_closed | Sealed complete-document identity; raw suffix removal; whole-document recovery | provider boundary, recovery, restart, exhaustion tests |
| RSO-4 | code_closed | Bounded coverage-order/proof-batch requests and same-step identity fencing | proof wire omits global order; batch membership/sequence/hash and request-size tests |
| RSO-5 | code_closed | Add and validate `RuntimeSemanticCorrectionDirective` | pure DTO/hash/clone and dependency-delta tests pass |
| RSO-6 | code_closed | Seal semantic correction through SM-3 private events and Runtime CAS | dependency and exact-active-batch correction, duplicate, stale, restart and exhaustion tests pass |
| RSO-7 | code_closed | Carry/render the sealed directive through the LLM owner boundary | preflight/request identity, detach and C0-C4 stability tests pass |
| RSO-8 | code_closed | Remove projection outcome re-adjudication and unify activity | combined Definition identity plus terminal/definition-drift/canonical activity tests pass |
| RSO-9 | code_closed | Run focused/full/race/boundary/managed regression matrix after the latest proposal-constraint closure | fmt/vet/full tests, affected-package race, boundary contract and managed `verify-ci-mvp-file` pass |
| RSO-10 | pending | Fresh deployed LSP Bridge positive convergence | complete semantic-to-effect event/audit lineage |
| RSO-11 | code_closed | Unify empty required-set schema/admission/correction behavior | exact evidence schema bounds, typed CWI field violation, active-batch and hash/clone tests |
| RSO-12 | code_closed | Unify all proposal-local and sealed owner-catalog checks behind one pure evaluator | recipe/variant/target/evidence/effect violations carry exact CWI/path/rule facts; local contract tests pass |
| RSO-13 | code_closed | Make SM-3 derive correction directives from rejected facts and persisted ledger context | production private event carries no directive; reducer derives under CAS; duplicate/stale/restart tests pass |
| RSO-14 | code_closed | Reduce LLM boundary to detached fact rendering only | C4 contains typed safe facts; LLM cannot classify correction or transition; LLM tests pass |
| RSO-15 | pending | Fresh deployed positive convergence after RSO-12 through RSO-14 | semantic seal → dispatch → receipt → evidence → CWI close → DoD result |
| RSO-16 | code_closed | Normalize exact/existing/prospective target policy into one schema/evaluator/correction contract | target policy matrix, finite-allowed-value and empty-catalog tests pass |
| RSO-17 | pending | Prove target correction and no-ineffective-retry behavior in a fresh deployment | target correction → semantic seal, or bounded catalog-gap terminal with no attempt 2 |
| RSO-18 | code_closed | Separate dynamic MCP/Shell capability identities from static operation variants and scope schema by owner/kind | dynamic-variant rejection, owner-selector isolation, static-variant and null-enum tests pass |
| RSO-19 | code_closed | Unify MCP effective capability catalog and revision | enabled capability/whitelist intersection, capability metadata in snapshot hash, and catalog query boundary tests pass |
| RSO-20 | code_closed | Enforce MCP recipe/effect feasibility before SM-2 binding seal | schema/evaluator and owner feasibility fail closed on stale/unknown/mismatched owner facts; mismatch creates zero dispatch (provider effect echo remains to be removed by RSO-23) |
| RSO-21 | code_closed | Make MCP receipt effect owner-observed | receipt derives effect from exact enabled capability; expected effect is never copied into observed effect; receipt integration passes |
| RSO-22 | pending | Fresh deployed positive convergence after owner-fact effect compilation | semantic seal → owner-fact binding → dispatch → receipt → binding-matched evidence → CWI advance |
| RSO-23 | code_closed | Remove provider-owned effect/idempotency echo and compile canonical binding from owner facts | provider schema/DTO/decoder contain no `requiredEffectScopes`; old field is unknown-field rejected; SM-2 binding compilation overwrites intermediate effect with sealed owner-catalog fact |
| RSO-24 | code_closed | Remove provider-owned validation-check echo and compile canonical checks from owner facts | provider schema/DTO/decoder contain no `requiredCheckRefs`; owner recipe/check feasibility is confirmed before binding seal; unsupported check yields zero dispatch |
| RSO-25 | code_closed | Close owner feasibility timing, current-focus batching and failure classification | owner returns one executable feasibility fact; SM-2 seals only matching facts; current focus is isolated from later CWI; zero-attempt semantic failures are not `no_progress_exhausted`; local probe, batch-isolation, failure-class, restart/idempotency and zero-dispatch tests pass. Fresh deployed positive and zero-dispatch paths remain live-open under RSO-22 |

Implementation order is strict: RSO-5 precedes RSO-6; RSO-11 is a prerequisite for RSO-12; RSO-12 precedes RSO-13;
RSO-13 precedes RSO-14; RSO-16 follows RSO-12 and precedes RSO-17; RSO-8 may run in parallel only if it does not
change Runtime authority. RSO-9 follows every code change. RSO-19 precedes RSO-20; RSO-20 precedes RSO-21; RSO-23
precedes RSO-24; RSO-24 precedes RSO-25; RSO-25 precedes RSO-22. RSO-10, RSO-15, RSO-17, RSO-22, RSO-23, RSO-24
and RSO-25 cannot be replaced by mock provider proof.

RSO-10 and RSO-15 are retained as historical positive-lineage labels for the earlier semantic/candidate closure. Their
fresh deployed proof is now executed under RSO-22 together with the MCP effect contract. RSO-17 remains the separate
target-correction deployed proof and is not satisfied by the RSO-22 positive path.

### 10.1 2026-08-08 Code-Closure Evidence

- SM-3 now persists one cloned `RuntimeSemanticCorrectionDirective`; the semantic ledger no longer treats a standalone
  correction hash as authority. The generic request session retains only the directive hash as request identity.
- Dependency-order rejection preserves exact typed predecessor/successor IDs and actual positions. Non-derivable
  diagnostics fail as `runtime_contract_defect`; attempt 1 cannot create attempt 2.
- New proof-batch documents reset the previous document's correction lineage in the same private CAS, preventing an
  accepted coverage-order correction from leaking into proof-batch identity.
- LLM EventHub commands and owner models carry detached rejection/directive values. C4 renders the directive and bounded
  safe diagnostic without `OriginalMessage`; C0-C3 and the stable prefix remain unchanged.
- Workspace outcome selects a current-Definition Run by Goal, DoD and Definition revision before applying the
  definition-change fallback. Activity consumes that same canonical outcome.
- Proof-batch wire documents no longer repeat `coverageOrder`; their zero-based sequence is not erased by `omitempty`.
  Attempt-0 structural rejection produces a typed exact-batch correction, and the private reducer rejects any valid
  manifest CWI that is outside the active batch.
- Projection compares the Run Definition identity with the canonical combined Goal/DoD identity from the Workspace
  owner contract rather than an individual definition revision.
- For the proposal-constraint delta, `make fmt-check`, `make vet`, `make verify-boundary-contract`,
  `go test ./... -count=1`, the affected-package race matrix, and managed `verify-ci-mvp-file` all pass. The managed
  artifacts were retained under `.ci-artifacts/managed-workorch-local` for inspection.
- RSO-12 now preserves owner-catalog violations for validation recipes, operation variants, target refs, selectors,
  constraints, owner/operation membership, and target policies as typed proposal facts.
- RSO-13 production correction events carry only the sealed expected-document identity and rejection facts; SM-3 derives
  and hashes the correction directive inside the Runtime CAS reducer.
- RSO-14 no longer exposes a semantic correction directive as an internal-event input; the LLM owner receives only the
  detached request-side rendering value and remains a factual provider adapter.
- These are local code-closure results. A fresh deployment must still prove that `validationRecipeId` failures produce
  field-level C4 correction and that a corrected run reaches semantic seal and downstream effect execution.
- `run-034` confirms the catalog correction path now emits an exact target violation, but target-policy correction did
  not yet produce an admissible `exactTargetRefs` value. The local RSO-16 closure now emits sealed catalog refs for
  empty exact/owner-scope fields, enforces the three policy shapes in the request schema, and leaves prospective
  target refs free to name a not-yet-existing resource under sealed selector/constraint facts. RSO-17 remains open
  for fresh deployed target correction and no-ineffective-retry proof.
- Fresh `run-035` exposed the dynamic MCP variant collision. The local RSO-18 closure now rejects dynamic
  `operationVariantId`, scopes schema values to each sealed owner catalog, and emits valid empty enums instead of
  `enum:null`. Live proof must still be rerun after deployment; `run-035` itself is not positive convergence.

### 10.2 Ordered Code-Closure Steps For RSO-12 Through RSO-15

1. **Freeze the owner-neutral constraint vocabulary** in
   `internal/pkg/workorchruntimecontract`.
   - Define one pure proposal evaluation result with bounded violation facts.
   - Cover local proposal shape and sealed manifest/catalog membership in one API.
   - Keep CWI ID authoritative and proposal index diagnostic-only.
   - Generate provider schema bounds from the same constraint vocabulary.
2. **Replace generic semantic document collapse** in
   `internal/pkg/workorchruntimecontract/runtime_semantic_payload.go` and
   `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go`.
   - Preserve all typed proposal, batch, dependency, and field violations.
   - Never use `$.proposals; contract_violation` when a typed fact is derivable.
   - Ensure validation recipe, operation variant, target, selector, and constraint catalog failures are
     `proposal_constraint` facts.
3. **Move correction derivation behind the SM-3 reducer** in
   `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go` and
   `internal/modules/kernel/orchestrator/biz/runtime/internal_event.go`.
   - Owner-private evaluated/rejected events carry facts only.
   - Reducer derives the directive from persisted rejection, manifest, active batch, expected document, and attempt.
   - Persist exactly one attempt-1 directive under the existing Runtime CAS.
   - Reject caller-provided directive hashes or directives that do not match derived facts.
4. **Keep the LLM owner factual** in `internal/modules/blocks/llm`.
   - Accept only detached manifest/context/rejection/directive DTOs.
   - Render typed correction facts in C4 without raw provider prose or `OriginalMessage`.
   - Preserve C0-C3 and stable prefix hashes between attempt 0 and attempt 1.
5. **Keep capability owners factual** in File/Git/Shell/MCP/Web/SSH.
   - Return catalog and feasibility observations through their owner event contracts.
   - Do not validate or repair another owner’s proposal and do not mutate SM-2/SM-3/SM-4.
6. **Verify the four-machine invariants**.
   - SM-1 remains Run-private; SM-2/3/4 remain Orchestrator-private.
   - No operation before semantic seal; no attempt 2; no cross-owner repository/service access.
   - Restart, duplicate, stale, CAS conflict, projection lag, and workspace isolation remain covered.
7. **Run live positive validation only after code gates pass**.
   - A failed semantic admission is not positive convergence, even if terminal delivery and projection are correct.
   - RSO-15 requires complete semantic-to-effect lineage and must record the exact live request/audit evidence.
8. **Close target policy as one contract** in
   `internal/pkg/workorchruntimecontract/runtime_semantic_contract.go`,
   `internal/pkg/workorchruntimecontract/runtime_semantic_payload.go`, and
   `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go`.
   - Define exact-target, existing-owner-scope, and prospective-owner-target invariants once.
   - Project those invariants into the request schema and use the same evaluator at admission.
   - Include sealed target refs/selectors/constraints in provider-safe correction facts.
   - If the owner catalog has no admissible target, classify a bounded catalog gap instead of issuing ineffective retry.
   - Add attempt-0/attempt-1, finite catalog, empty catalog, stale directive, duplicate event, and restart tests.
9. **Separate dynamic capability identities** in
   `internal/pkg/workorchruntimecontract/runtime_semantic_payload.go` and
   `internal/pkg/workorchruntimecontract/runtime_semantic_contract.go`.
   - MCP/Shell dynamic recipes must not be emitted as `operationVariantId`.
   - Restrict target, selector, constraint, recipe, and static variant values by sealed owner/kind.
   - Reject cross-owner selector/recipe facts before owner feasibility dispatch.
   - Add dynamic-variant, owner-isolation, static-variant, and empty-enum schema tests.

### 10.3 Ordered Code-Closure Steps For RSO-19 Through RSO-22

1. **Extend the registry owner contract first.** Add typed capability metadata to the registry query/result and make
   the owner validate non-empty effect/idempotency values when a capability is enabled for Runtime. Keep arbitrary
   custom capability metadata configurable; do not hard-code template, language, LSP, or software-development names in
   registry or Runtime code.
2. **Compile one effective MCP catalog in `blocks/mcp`.** Intersect enabled server whitelist and enabled exact
   capability records, normalize deterministically, and hash the complete effective set. Remove server-only recipe
   authority from semantic feasibility. If the typed capability query is unavailable, return an explicit typed catalog
   prerequisite failure; never manufacture recipe refs from `Server.AllowedTools`. Return a detached catalog fact with
   workspace/run/authority scope and revision.
3. **Bind semantic recipes to catalog facts without provider effect echo.** Extend the private sealed
   manifest/context so each dynamic recipe carries the exact capability reference, operation kind, finite effect domain,
   idempotency class, and catalog revision. The provider-facing proposal contains no effect/idempotency field; the LLM
   only renders recipe, operation, target, selector, check, and declaration facts.
4. **Compile effect and idempotency in SM-2.** After owner feasibility returns a confirmed typed fact, Orchestrator
   submits one private internal event. SM-2 derives the canonical effect/idempotency and seals `CoverageEvidenceBinding`.
   Missing, stale, unknown, or contradictory owner facts fail closed before candidate/dispatch; there is no LLM correction
   for an owner fact and no second effect-selection loop.
5. **Re-check exact capability at dispatch.** Before executing, MCP owner validates capability ID, catalog revision,
   operation kind, effect, idempotency, target policy, and transport. Drift returns stale/infeasible typed facts and
   schedules the existing bounded prerequisite refresh; it does not mutate Orchestrator state directly.
6. **Make receipts factual.** MCP owner derives observed effect/idempotency/target/check facts from its capability and
   execution result. Never copy expected dispatch fields into observed fields. SM-2 compares sealed authorization,
   receipt, and binding before granting evidence or CWI credit.
7. **Preserve the existing four-machine boundaries.** No new MCP state machine, no application facade, no registry
   repository injection, no Runtime-specific MCP branch, and no fifth retry/correction loop.
8. **Run local and deployed gates.** Add transition, stale/duplicate/restart, effect mismatch, exact receipt, zero
   dispatch, bounded retry, and positive semantic-to-effect lineage tests. Then execute a fresh deployment proving
   `semantic_contract_sealed → dispatch → receipt → binding-matched evidence → CWI advance`.

### 10.4 2026-08-09 Live Evidence

Fresh deployment loaded the corrected default metadata:

```text
lsp_status: effectScope=none, idempotencyClass=safe_to_replay
```

`run-054` then reached `semantic_contract_sealed`, sealed one dispatch, executed
`mcp/tool_call/mcp-lsp-bridge/lsp_initialize`, persisted an owner receipt, and committed the operation evidence.
It did not receive CWI credit because the sealed binding required `none` while the owner receipt observed
`ephemeral_session`. This is a valid fail-closed proof of the evidence gate, not positive convergence. The workspace
released `activeRunId` and the terminal delivery completed; RSO-22 remains open until a fresh run reaches
binding-matched evidence/CWI advance without a post-dispatch mismatch.

Fresh `run-055` and explicitly restarted `run-056` proved that RSO-20 rejects an inverse owner-effect mismatch before dispatch:
the semantic proposal selected `mcp/tool_call/mcp-lsp-bridge/lsp_status` with `ephemeral_session`, while the effective
owner catalog declares `none`. Both runs terminated as `semantic_contract_failed` with zero dispatch, zero receipt and
zero evidence; no post-dispatch `coverage_binding_mismatch` occurred. The follow-up local closure now projects every
dynamic MCP recipe's exact owner `effectScope` into the request-specific JSON Schema and applies the same check in the
pure semantic document/catalog evaluator. This is a fail-closed baseline only; it does not make the provider the effect
authority. RSO-23 must remove this provider-facing echo. RSO-22 remains pending until RSO-23 is deployed and a fresh run
reaches binding-matched CWI advance.

### 10.5 Ordered Code-Closure Steps For RSO-23: Remove Provider-Owned Effect Echo

RSO-23 is the remaining code item exposed by `run-061`/`run-062`. It is a contract simplification, not a new retry or
state machine. Implement it in this order:

1. **Remove the duplicate wire field** from the provider-facing semantic proposal DTO, JSON schema, decoder, normalize/
   validate, merge and correction surface in `internal/pkg/workorchruntimecontract`. The internal proposal value may
   retain a derived, non-wire effect slice solely as the canonical binding intermediate; it must be populated from the
   sealed owner catalog and never from provider JSON. The canonical provider proposal contains CWI, owner, operation
   kind/variant, target policy, validation recipe, checks/metrics/artifacts, and declaration refs; it does not contain
   `requiredEffectScopes` or any provider-selected idempotency.
2. **Fail closed on stale wire**, with no compatibility or fallback path. Unknown-field decoding must preserve a bounded
   original diagnostic/audit record, but must not enter semantic evaluation, correction, binding, dispatch, or retry.
3. **Compile owner facts through one Orchestrator-private event.** Capability owners (`file`, `git`, `shell`, `mcp`, `web`,
   `ssh`) return typed feasibility facts containing operation identity, effective effect scope, idempotency class,
   target/selector, recipe/check identity, catalog revision, and feasibility status. They never mutate Runtime state.
4. **Have SM-2 seal the canonical binding.** The Orchestrator maps each confirmed owner fact to a private
   `RuntimeInternalEvent`; only the SM-2 reducer derives and persists effect/idempotency in
   `CoverageEvidenceBinding`. SM-3 consumes that sealed binding for candidate/dispatch, and SM-4 records receipt/evidence.
5. **Delete effect correction paths.** Remove prompt/schema instructions, provider DTO fields, and evaluator branches that
   ask the LLM to repair or select effect/idempotency. Keep mismatch/stale/unknown owner facts as typed prerequisite or
   semantic diagnostics with zero dispatch.
6. **Preserve all original information at the factual boundaries.** Do not discard owner catalog metadata,
   feasibility observations, sealed binding, dispatch authorization, owner receipt, evidence, or audit diagnostics. Only
   remove the duplicate provider echo.

#### RSO-23 Component And State-Machine Matrix

| Component | RSO-23 responsibility | Forbidden responsibility |
| --- | --- | --- |
| `workorchruntimecontract` | owner-neutral proposal without effect; canonical binding value/validation/hash helpers | owner lookup, reducer, EventHub, compatibility alias |
| `kernel/orchestrator/biz/runtime` | collect typed owner feasibility; emit private event; SM-2 compiles binding; SM-3/4 continue candidate/receipt/evidence | provider effect inference, direct owner repository/service, cross-SM transition |
| `blocks/llm` | render/parse detached proposal and bounded diagnostics | effect/idempotency selection, correction, retry, Runtime transition |
| `blocks/file`, `git`, `shell`, `mcp`, `web`, `ssh` | return exact owner capability/feasibility facts and owner receipts | mutate SM-2/3/4 or echo expected effect as observed fact |
| `blocks/mcpregistry` | own capability catalog, metadata, enabled state and revision | Runtime binding, MCP execution, template/domain assumptions |
| `blocks/run` | own SM-1 lifecycle and CAS persistence of opaque Runtime aggregate | semantic effect compilation or SM-2/3/4 transitions |
| Application/Web/TUI/projection/EventHub adapters | pass typed intent or display canonical facts | local state machine, effect selection, correction/retry |

#### RSO-23 Invariants And Tests

- schema and DTO tests prove `requiredEffectScopes` is absent and an old field is rejected as unknown;
- `lsp_status` compiles `none`, `lsp_initialize` compiles `ephemeral_session`, without provider selection;
- File/Git durable, Shell validation, and MCP/Web/SSH effects all compile from owner facts through the same path;
- stale/unknown/missing capability facts produce zero dispatch, receipt, evidence, and CWI credit;
- duplicate/stale private events do not seal a second binding; restart reproduces the same binding hash;
- receipt/binding mismatch remains fail-closed and preserves the original owner diagnostic;
- fresh deployed positive path reaches semantic proposal → owner feasibility → SM-2 binding seal → SM-3 dispatch → owner
  receipt → binding-matched evidence → CWI advance.

Local RSO-23 code closure is complete: provider-facing semantic schema and strict payload decoder no longer admit the
effect field, old payloads fail closed, and SM-2 binding compilation re-resolves effect from the sealed owner catalog.
The remaining deployed proof is intentionally tracked under RSO-22; code-closed does not imply live-closed.

### 10.6 Ordered Code-Closure Steps For RSO-24: Remove Provider-Owned Check Echo

RSO-24 closes the `run-063` check mismatch without adding a fifth state machine or a tool-specific branch.

1. **Define canonical owner check facts.** Each capability owner's sealed admissibility/validation recipe identity set is
   the finite canonical check set for that owner (the recipe identity is the check identity unless the owner publishes a
   more specific typed check fact). A check is executable only when the owner can perform and observe it. Recipe/check
   identity, effect, idempotency, target and catalog revision remain one owner fact; no global check index is introduced.
2. **Remove provider check authority.** Delete `requiredCheckRefs` from the provider-facing semantic-plan schema, DTO,
   strict decoder, normalize/hash/clone/merge and correction rendering. Old payloads containing the field are rejected as
   unknown; there is no compatibility or fallback path. The provider may select a sealed recipe identity only when that
   recipe is in the owner catalog.
3. **Return typed feasibility facts.** `blocks/file`, `blocks/git`, `blocks/shell`, `blocks/mcp`, `blocks/web`, and
   `blocks/ssh` return confirmed recipe/check/effect/idempotency/target facts through their owner EventHub contracts. A
   missing, stale, unsupported or unverifiable check returns a typed fact and never changes Runtime state.
4. **Compile binding only in SM-2.** Orchestrator maps owner results to one private `RuntimeInternalEvent`. SM-2 derives
   `CoverageEvidenceBinding.RequiredCheckRefs` from confirmed owner facts and seals the binding under the existing Runtime
   CAS. SM-3 consumes only that sealed binding; SM-4 records receipt/evidence. No caller may submit a binding or check list.
5. **Make receipts factual.** Capability owners return the exact check identities actually executed and observed. A receipt
   must never copy expected checks from dispatch. SM-2 compares recipe/check/effect/idempotency/target and revision before
   evidence or CWI credit.
6. **Correct failure classification.** Unsupported or unverifiable checks fail as a typed semantic/capability contract
   gap before dispatch. A post-dispatch mismatch is `runtime_contract_defect` or `owner_receipt_contract_mismatch`, not
   `no_progress_exhausted`, unless a real bounded retry budget was consumed.

#### RSO-24 Component And State-Machine Matrix

| Component | Sole responsibility | Forbidden responsibility |
| --- | --- | --- |
| `workorchruntimecontract` | owner-neutral check/recipe fact DTOs, validation, clone/hash | global check registry, owner lookup, reducer, EventHub |
| `blocks/mcpregistry` | MCP capability metadata, recipe/check catalog, enabled state and revision | Runtime binding, execution, CWI credit |
| `blocks/mcp` | MCP feasibility probe, exact check execution, factual receipt | mutating registry or Runtime state |
| `blocks/shell` | executable validation recipe/check facts and receipt | accepting provider command/check authority |
| `blocks/file`, `git`, `web`, `ssh` | owner-specific recipe/check feasibility and receipts | Runtime transitions or cross-owner repository access |
| `blocks/llm` | render recipe selection and parse check-free proposal | selecting checks, effect, idempotency, retry or terminal |
| `kernel/orchestrator/biz/runtime` | private event dispatch; SM-2 binding compilation; SM-3/4 execution/evidence | reading owner repositories or repairing owner facts |
| `blocks/run` | SM-1 lifecycle and opaque Runtime aggregate CAS | SM-2/3/4 or capability fact compilation |
| Workspace/Result/Application/Web/TUI/projection | own lifecycle or display only | transition inference, check selection, correction/retry |

#### RSO-24 Invariants And Tests

- provider schema and DTO contain no `requiredCheckRefs`;
- old provider payloads containing the field fail closed as unknown-field diagnostics;
- every sealed validation binding has checks sourced from one owner fact and a matching catalog revision;
- MCP `lsp_status` cannot seal `lsp_status_returns_ten_tools` unless MCP owner declares and executes that check;
- an unverifiable check produces zero dispatch, receipt, evidence and CWI credit;
- duplicate/stale owner feasibility events cannot seal a second binding;
- restart preserves the same owner check/binding hash;
- receipts cannot copy expected checks and check mismatch remains fail-closed;
- failure classification distinguishes contract gap from actual retry exhaustion;
- fresh live proof reaches owner-confirmed check → binding → dispatch → matching receipt → evidence → CWI advance.

Local code closure is complete. In the current owner-neutral contract, a recipe identity is also the canonical check
identity when (and only when) it is present in the addressed owner's sealed `ValidationRecipeRefs`/`ValidationRecipes`;
there is no global check index or provider-supplied alias. Fresh deployed positive convergence remains an RSO-22 live
verification item.

### 10.7 Ordered Code-Closure Steps For RSO-25: Owner Feasibility Transaction Closure

`run-064` proves that removing provider check authority is necessary but not sufficient. Shell returned a feasible
semantic recipe/check fact, then rejected the same recipe during operation preflight because the workspace had no
`*_test.go` target. This is an owner-fact timing defect, not a new Runtime state machine.

#### Contract objective

Each capability owner must return one complete feasibility outcome for the exact recipe/check/target in the addressed
catalog revision. The outcome must include the result of the owner-local executable preflight. Orchestrator must never
seal an SM-2 binding from a catalog identity that the owner has not proven executable.

#### Ordered implementation slices

1. **Owner-local preflight closure (`blocks/shell`).** Move validation recipe discovery and executable-target preflight
   into the Shell semantic feasibility handler. The returned typed fact must be `feasible` only when the same command,
   target and check can execute and be observed. Missing `*_test.go`, invalid command, stale workspace manifest or
   unavailable toolchain returns `prerequisite_required`/`infeasible` with the original diagnostic. The later operation
   admissibility path may re-check freshness, but it must not be the first place that discovers basic recipe absence.
2. **Capability-owner parity.** Apply the same one-owner fact rule to `blocks/mcp`, `blocks/file`, `blocks/git`,
   `blocks/web` and `blocks/ssh`: catalog, capability state, recipe/check preflight, target and revision are one typed
   owner result. File/Git/Web/SSH now fail closed with `*_semantic_proof_unavailable` whenever a proposal declares
   validation recipe/checks that those owners cannot independently prove. MCP catalog/tool probing is likewise not
   sufficient by itself: when the registry catalog declares a validation recipe, `blocks/mcp` now performs a bounded,
   owner-local, non-durable safe-to-replay behavior probe and returns a factual `feasible` observation only after the
   probe succeeds. An unsafe, unsupported, malformed, stale, or unavailable probe returns typed
   `mcp_semantic_proof_unavailable`/prerequisite facts. No owner may publish a semantic `feasible` result while its
   operation preflight would reject the same sealed identity.
3. **SM-2 admission guard (`kernel/orchestrator/biz/runtime`).** Before compiling `CoverageEvidenceBinding`, require
   owner feasibility status `feasible`, canonical check set non-empty for validation, matching recipe/effect/idempotency,
   exact target/selector and matching catalog/observed revision. Any missing or stale fact produces zero binding seal,
   zero operation candidate, zero dispatch and zero CWI credit.
4. **SM-3 dispatch gate.** Operations candidate generation is allowed only after the SM-2 binding set is sealed. If owner
   feasibility is unavailable, SM-3 must not issue an operations provider request. This gate is an Orchestrator internal
   event transition; capability owners never mutate SM-3 state.
5. **Failure classification closure.** `validation_recipe_exhausted` with zero repair attempts must remain a typed
   owner/capability contract failure. It must not be projected as `no_progress_exhausted`; that class is valid only
   after a real bounded repair or retry budget is consumed. Result, Workspace and Run must preserve the same nested
   original diagnostic.
6. **Restart/idempotency regression.** Persist owner preflight revision and canonical check identity in the feasibility
   observation/binding hash. Duplicate/stale owner events cannot reseal a binding. Restart resumes the same owner fact or
   requests a new bounded owner observation; it never reuses a provider check or replays dispatch.
7. **Current-focus batch admission (`kernel/orchestrator/biz/runtime`).** The active `focus`/slice and its dependency
   closure are the only inputs to the next proof batch. The provider-authored global `coverageOrder` remains an order
   proposal, but it must not mix later focus items into the current SM-2 admission transaction. Pending CWI IDs remain
   immutable lineage and are admitted only after the current focus is accepted or terminalized. A failure in a later
   CWI therefore cannot terminate the current focus.
8. **Semantic failure classification (`kernel/orchestrator/biz/runtime`).** Replace the fixed
   `FailureClassNoProgressExhausted` assignment in `failSemanticContract` with an exhaustive mapping from typed owner /
   provider category. `candidate_infeasible`, `prerequisite_required`, `runtime_contract_defect` and provider transient
   must retain their own class. `no_progress_exhausted` is legal only after a persisted repair/retry budget consumption.
   Result, Run, Workspace and projections must preserve the same nested original diagnostic and class.

#### RSO-25 owner/state-machine matrix

| Component | Owner-local contract | Forbidden cross-owner action |
| --- | --- | --- |
| `blocks/shell` | recipe discovery, executable preflight, canonical validation checks, factual receipt | writing Runtime state or accepting provider check lists |
| `blocks/mcpregistry` | MCP catalog, enabled capability, recipe/check vocabulary and revision | executing checks or sealing CWI bindings |
| `blocks/mcp` | catalog-backed tool/resource feasibility, bounded side-effect-free behavior check execution and factual receipt | mutating registry or SM-1/2/3/4 |
| `blocks/file`, `git`, `web`, `ssh` | owner-specific target and recipe/check preflight plus receipt | reading another owner repository/service or migrating Runtime state |
| `blocks/llm` | provider transport and check-free semantic proposal | selecting check/effect/idempotency/retry/terminal |
| `kernel/orchestrator/biz/runtime` | only SM-2/3/4 reducer, private internal events, binding/dispatch/evidence gate | direct owner repository/service access |
| `blocks/run` | only SM-1 lifecycle and aggregate CAS | compiling binding or executing capabilities |
| Result/Workspace/Application/Web/TUI/Projection | result, lifecycle, API and display projection | inferring or triggering Runtime transitions |

#### RSO-25 state-transition closure contract

The following sequence is the only accepted production path. The arrows are owner-internal events; an EventHub
command/result may carry a fact across an owner boundary, but it never performs the transition itself.

```text
Run owner (SM-1)
  activation/start CAS
        ↓ RunStarted fact
Orchestrator (SM-2)
  active focus → owner prerequisite → semantic feasibility → binding sealed
        ↓ BindingSealed internal event
Orchestrator (SM-3)
  operations candidate → candidate accepted → dispatch sealed
        ↓ typed operation command / owner receipt
Capability owner (fact only)
        ↓ receipt/evidence fact
Orchestrator (SM-4 → SM-2)
  evidence committed → CWI accepted → next focus selected
        ↓ lifecycle/result facts
Run owner (SM-1)
  run continues or terminalizes through its own CAS
```

Required invariants:

1. `blocks/run` is the only writer of SM-1 state and lineage; Orchestrator may only consume its typed facts.
2. `kernel/orchestrator/biz/runtime` is the only writer of SM-2/SM-3/SM-4 aggregate state. Capability owners never
   receive a Runtime state target and never call a Runtime reducer.
3. A semantic proof batch is scoped to `activeFocusID + dependencyClosure`; global `coverageOrder` is retained only as
   immutable provider order and cannot broaden the current admission set.
4. `feasible` is permitted only when the addressed owner has independently completed the same recipe/target/check
   preflight that the later operation path will use. Catalog readiness alone is never sufficient.
5. Any missing, stale, unsupported or unverifiable owner fact produces zero binding, candidate, dispatch, receipt,
   evidence and credit for that focus, while unrelated pending focuses remain pending.
6. Every terminal diagnostic has one typed failure class and one nested `OriginalDiagnostic`; projections must not
   reclassify it. `no_progress_exhausted` requires persisted repair/retry budget consumption.
7. Duplicate/stale/restarted facts are idempotent by workspace/run/focus/batch/manifest/catalog identity and cannot
   reseal a binding or replay an operation.

Implementation order is therefore: (a) MCP owner behavior probe and catalog/probe parity, (b) current-focus batch
compiler and dependency barrier, (c) semantic failure-class mapper, (d) reducer/restart/idempotency tests, and only then
(e) fresh deployed positive convergence.

#### RSO-25 acceptance matrix

- Shell semantic feasibility rejects a workspace with no executable validation target before SM-2 binding seal;
- no owner can report semantic `feasible` and later reject the same recipe solely during first operation preflight;
- unsupported/stale/unverifiable facts produce zero binding, candidate, dispatch, receipt, evidence and CWI credit;
- `run-064`-style zero-attempt `validation_recipe_exhausted` is not classified as `no_progress_exhausted`;
- provider requests remain free of `requiredCheckRefs`, `requiredEffectScopes` and other owner authority;
- sealed binding check/effect/idempotency/target/revision are sourced from one matching owner fact;
- duplicate, stale and restart paths preserve the same owner/binding identity without cross-owner state writes;
- a fresh deployed run with an executable validation target reaches binding → operations candidate → dispatch → matching
  receipt → evidence → CWI advance.
- MCP catalog-declared validation recipe with a safe behavior probe reaches `feasible`; an unsafe or unsupported probe
  returns typed `mcp_semantic_proof_unavailable` without blocking an unrelated current focus.
- a proof batch contains only the active focus and dependency closure; later-focus infeasibility remains pending.
- semantic feasibility failure with zero repair attempts preserves `candidate_infeasible` (or the exact typed prerequisite
  class) instead of `no_progress_exhausted`.

#### RSO-25 verification commands

```bash
go test ./internal/modules/blocks/shell/... -count=1
go test ./internal/modules/blocks/mcp/... ./internal/modules/blocks/file/... ./internal/modules/blocks/git/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./... -count=1
make fmt-check
make vet
make verify-boundary-contract
```

RSO-25 is locally `code_closed`: the MCP positive/unsupported probe matrix, current-focus batch isolation, typed
zero-attempt failure classification, restart/idempotency fencing, full tests, race tests, formatting, vet and boundary
checks pass. It remains `live-open` until a fresh deployment demonstrates both the zero-dispatch unavailable-recipe
path and the executable-recipe positive path; those deployed proofs are tracked with RSO-22 and must not be inferred
from mock-provider or historical runs.

## 11. Verification Matrix

### 11.1 Semantic Correction

- dependency order violation produces exact typed predecessor/successor IDs;
- membership, duplicate, missing-CWI, invalid plan step, and batch mismatch produce bounded structured violations;
- one unique correction is sealed; an ambiguous/non-derivable correction fails as `runtime_contract_defect`;
- attempt 0 plus directive is rejected;
- attempt 1 without directive or with mismatching rejection hash is rejected;
- attempt 1 keeps C0-C3 and all authority identities unchanged;
- provider-facing C4 contains the safe diagnostic and directive but no `OriginalMessage` or raw details;
- attempt-1 rejection creates no attempt 2, operation, governance request, or effect;
- duplicate/stale private events and CAS conflicts cannot duplicate the request;
- restart before/after directive persistence resumes the same ledger deterministically.

### 11.2 Four State Machines And Owners

- SM-1 transition and forbidden-transition tests remain Run-owner local;
- semantic transitions are reachable only through the Orchestrator internal dispatcher;
- SM-2 cannot call provider or execute operations;
- SM-3 cannot select next focus or create coverage credit;
- SM-4 cannot rebuild authority from diagnostics or projection;
- capability result affects only its matching sealed operation/binding;
- Result, Workspace, and Run terminal delivery is idempotent and ordered;
- no cross-owner target-state command, service/repository injection, entity command, map authority, or raw-message branch;
- no busy loop while provider/CAS/prerequisite/delivery recovery is pending.

### 11.3 Projection

- failed/completed/cancelled/superseded Run overrides definition-change attention;
- terminal Run remains selected after Workspace releases `ActiveRunID`;
- active Workspace resource does not imply active Run outcome;
- outcome/activity/run state/convergence all use the same run and revision;
- source/projected revisions caught up with a bad outcome fails the contract test rather than passing as fresh;
- duplicate notifications do not duplicate activity;
- switching between two active workspaces resumes each workspace's independent activity stream.

### 11.4 Provider And Downstream Regression

- complete semantic and operations documents;
- output-limit/EOF whole-document recovery without raw concatenation or attempt consumption;
- recovery exhaustion creates no candidate, dispatch, Decision, Approval, or effect;
- accepted order compiles bounded proof batches in the sealed order;
- owner feasibility seals exact per-CWI bindings;
- MCP owner rejects stale, unknown, or contradictory owner facts before binding seal and dispatch;
- MCP catalog revision changes when capability effect/idempotency changes;
- MCP receipt observed effect is owner-derived and never copied from expected dispatch effect;
- no operation before semantic seal;
- sealed dispatch, fence, receipt, evidence quality, CWI advance, assessment, and terminal delivery survive restart;
- stable prefix and request sizes remain bounded within each logical request.

### 11.5 Commands

Run focused checks before the full repository gates:

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/blocks/mcpregistry/... -count=1
go test ./internal/modules/blocks/mcp/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/service/tui/... -count=1
make fmt-check
make vet
go test ./... -count=1
go test -race ./... -count=1
make console-build
make verify-boundary-contract
make verify-ci-mvp-file
```

Managed verifiers use a disposable managed instance and must not bypass the guard protecting an unmanaged live
workspace.

## 12. Fresh Deployed Completion Gate

The plan is `live_closed` only when a fresh deployment proves this lineage:

```text
restart/activation intent
  -> Run owner activation
  -> coverage_order attempt 0 factual rejection (fault-injection lineage)
  -> SM-3 sealed typed correction directive
  -> corrected coverage_order accepted
  -> bounded proof batches accepted
  -> complete owner feasibility facts
  -> SM-2 owner-fact effect/idempotency binding seal
  -> operation candidate accepted
  -> sealed dispatch
  -> capability receipt
  -> binding-matched evidence
  -> focus/CWI progress
  -> durable workspace output
  -> Result -> Workspace -> Run terminal delivery
  -> terminal-consistent TUI/Web outcome and activity
```

The live proof must also show:

- zero cross-owner target-state commands and zero owner repository bypass;
- no authority document suffix concatenation;
- no operation before semantic seal;
- no predecessor credit inherited by restart;
- stable prefix consistency and bounded request size;
- bounded retries and no high-CPU loop;
- source/projected revision convergence;
- correct workspace-isolated TUI/Web streaming after workspace switching;
- MCP effective capability revision includes effect/idempotency metadata;
- provider proposal contains no effect/idempotency authority and an old effect field is rejected as unknown;
- an MCP owner-fact mismatch fails during owner feasibility with zero dispatch;
- provider proposal contains no check authority and an old `requiredCheckRefs` field is rejected as unknown;
- an owner-unverifiable check fails during feasibility with zero dispatch;
- a matching MCP recipe reaches owner-observed receipt, binding-matched evidence, and CWI advance without a
  post-dispatch `coverage_binding_mismatch`.

Whole-document recovery requires a separate deployed fault-injection proof because a normal complete response does not
exercise it. Until both positive convergence and recovery fault injection pass, code-closed recovery remains a
regression baseline rather than a live-closed claim.

## 13. Forbidden Closure Tactics

- Do not add `OriginalMessage`, raw provider output, or free-form repair prose to C4.
- Do not let the LLM owner infer dependency edges, correction disposition, retry, terminal, or next action.
- Do not treat the existing semantic correction hash as an actionable directive.
- Do not add a fifth semantic, correction, request-session, restart, projection, or scheduler state machine.
- Do not move coordination state into Application, TUI, Web, projection, Workspace, Run, or Result.
- Do not let Orchestrator create or mutate Run/Workspace/Result entities directly.
- Do not let projection overwrite terminal owner facts with definition-change or UI convenience state.
- Do not add generic event envelopes, global topic indexes, facade/service registries, repository callbacks, map authority,
  compatibility aliases, or dual paths.
- Do not relax dependency/order validation, increase retry/token/time limits, or manually reorder CWIs as a substitute
  for the sealed semantic correction contract.
- Do not mark code-closed work live-closed without fresh deployed proof of the path being claimed.
