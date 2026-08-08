# Runtime CWI Reconciliation Single-Owner Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-08-08

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

Fresh deployed validation of workspace `ws-a8bada88e7b1b5f8`, Run `run-029`, is the current fact baseline.

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
- Both provider calls returned bounded complete JSON documents. Neither used raw suffix assembly or document
  continuation recovery.
- The terminal aggregate stopped driving after failure; no provider loop, CAS loop, or high-CPU loop was observed.

### 2.2 Current Semantic Correction Gap

Attempt 0 was correctly rejected because the returned coverage order violated the sealed dependency:

```text
cwi-046 -> cwi-043
```

The persisted original diagnostic retained that exact cause. The provider-facing attempt-1 C4 tail, however, retained
only a generic field violation such as:

```text
path=$.proposals
actual=contract_violation
```

It did not contain an Orchestrator-sealed, provider-safe correction delta stating that `cwi-046` must precede
`cwi-043`. The attempt-1 document was therefore byte-identical to attempt 0 and failed again. This is not a prompt
wording defect. The missing authority is a typed semantic correction directive owned by SM-3.

### 2.3 Current Projection Gap

The authoritative Run state and convergence state were both `failed`, and projection revisions were fully caught up.
The Workspace RuntimeView nevertheless reported an `active`, non-terminal outcome with the summary
`workspace definitions changed; execution must be rerun`. Its activity feed repeated the derived active outcome.

This is not projection lag. `MapWorkspaceOutcome` currently applies the definition-changed branch before selecting the
authoritative display Run and returns early. The projection has therefore created a competing outcome interpretation.
Projection remains read-only, but the displayed outcome is contractually inconsistent with owner facts.

### 2.4 Paths Not Proven By This Run

This run did not reach proof-batch feasibility, semantic seal, operation candidate, sealed dispatch, capability
execution, receipt, evidence admission, CWI advance, assessment, or durable workspace output. Both documents were
complete, so SM-4 whole-document recovery was also not exercised. These paths remain regression/live gates even where
their code is already closed.

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

## 8. Component Work Matrix

| Component | Required closure | Explicitly forbidden |
| --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | semantic directive DTO, normalize/validate/clone/hash; dependency violation DTO | reducer, retry classification, owner lookup, EventHub |
| `kernel/orchestrator/biz/runtime` | evaluator emits precise typed violations; private reducer seals directive; invariant/CAS checks | provider rendering, capability execution, cross-owner repository |
| `kernel/orchestrator/biz` | drive reads persisted directive and maps LLM facts to private events | reconstructing directive from raw message at send time |
| `blocks/llm/pkg/events` | semantic command carries detached directive; result remains factual | correction disposition, terminal/action fields |
| `blocks/llm/pkg/models` | owner-private immutable request representation | Runtime aggregate or mutable owner entity |
| `blocks/llm/biz` | validate and render provider-safe C4 | infer delta, retry, terminal, coverage, batch transition |
| `application/workorchd/.../readmodelprojection` | terminal-first canonical outcome and activity derivation | competing execution state machine |
| `workorchprojectioncontract` | separate outcome from optional attention fact | authority or lifecycle command |
| TUI/Web/event mapping | render canonical outcome/activity and workspace switch isolation | local conflict resolution or state transition |
| `blocks/run` | regression only: SM-1 and replacement owner-private events | SM-2/3/4 interpretation |
| `blocks/workspace` | regression only: exact active-run claim/release | Run or Runtime terminal decision |
| `blocks/result` | regression only: idempotent sealed outcome storage | acceptance adjudication |
| File/Git/Shell/MCP/Web/SSH | regression only: typed feasibility/receipt facts | Runtime action, evidence credit, retry/terminal |

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
| RSO-9 | code_closed | Run focused/full/race/boundary/managed regression matrix after the latest proof-batch closure | fmt/vet/full/boundary, affected-package race and managed CI pass |
| RSO-10 | pending | Fresh deployed LSP Bridge positive convergence | complete semantic-to-effect event/audit lineage |

Implementation order is strict: RSO-5 precedes RSO-6; RSO-6 precedes RSO-7; RSO-8 may be implemented after RSO-5 in
parallel only if it does not change Runtime authority. RSO-9 follows all code changes. RSO-10 cannot be replaced by mock
provider proof.

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
- For the latest delta, `make fmt-check`, `make vet`, `make verify-boundary-contract`, `go test ./... -count=1`, the
  affected-package race matrix, and managed `verify-ci-mvp-file` pass. The Shell and MCP mock providers now follow the
  same minimal proof-batch wire and explicitly assert that `coverageOrder` is absent.

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
- no operation before semantic seal;
- sealed dispatch, fence, receipt, evidence quality, CWI advance, assessment, and terminal delivery survive restart;
- stable prefix and request sizes remain bounded within each logical request.

### 11.5 Commands

Run focused checks before the full repository gates:

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
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
  -> SM-2 binding/reconciliation seal
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
- correct workspace-isolated TUI/Web streaming after workspace switching.

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
