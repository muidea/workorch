# Runtime CWI Reconciliation Single-Owner Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-08-08

## 1. Purpose And Authority

This is the only active implementation queue for Runtime lifecycle, CWI semantic planning, reconciliation, and
single-owner state-machine closure. It is subordinate to:

1. `docs/00-core/runtime-stage-contract.md`;
2. `docs/00-core/runtime-request-cache-note.md`;
3. `internal/modules/kernel/orchestrator/readme.md`;
4. `internal/pkg/workorchruntimecontract`.

Archived plans and previous green verification artifacts are regression evidence only. They cannot override a current
persisted aggregate or live event-order counterexample.

The product contract remains unchanged: a user supplies a template, Goal, DoD, and workspace directory. WorkOrch must
derive the bounded semantic plan, execute it, collect evidence, and assess the DoD without requiring the user to author
Runtime JSON, target selectors, recipes, checks, metrics, or per-CWI proof contracts.

## 2. Current Live Facts

Fresh deployed validation of workspace `ws-a8bada88e7b1b5f8`, Run `run-025`, established the following facts.

### 2.1 Working Baseline

- The service and projection were ready; source and projected event revisions both reached `385`.
- Workspace owner persisted the next-run inference selection. Run owner sealed `ai-proxy/deepseek-v4-flash` with
  `reasoningEffort=none` into the Run.
- Run owner performed `pending -> active -> failed` through owner-local lifecycle events.
- SM-2 rejected the invalid semantic document before binding, candidate, dispatch, receipt, or evidence creation.
- SM-4 persisted terminal acknowledgements in the required `Result -> Workspace -> Run` order.
- Result, Workspace, and Run owners changed only their own resources.

### 2.2 Reopened Contract Gaps

Two previously claimed closure areas are reopened.

#### Restart successor ordering

The observed event order was:

```text
revision 367  run-025 created
revision 368  run-024 result rejected
revision 369  workspace released run-024
revision 370  run-024 superseded
revision 372  run-025 started
```

The successor was created before predecessor terminal delivery released the Workspace slot. Starting the successor was
delayed correctly, but early creation violates the required release-before-create contract and can leave an orphan
pending successor if predecessor delivery fails.

#### Semantic correction and whole-document recovery

The first request returned a complete `coverage_order` document. SM-3 correctly rejected it because it violated the
sealed dependency `cwi-046 -> cwi-043`.

The one allowed correction then returned a `proof_batch` document instead of a corrected `coverage_order`. It omitted
`batchId`, `batchSequence`, and `batchCoverageWorkItemIds`, retained the invalid order, reached the 8192-token output
limit, and stopped inside the JSON key `requiredEffectScopes`. A second physical call returned escaped text rather than
the exact next JSON bytes. Concatenation produced an invalid document.

This is not a single backslash defect. The current path fails to preserve both:

- the sealed expected document contract across correction; and
- the byte/document boundary across provider output-limit recovery.

## 3. Non-Negotiable Rules

1. There are exactly four state machines. No restart saga, semantic-plan batch, provider request session, continuation
   carrier, projection, or scheduler may become a fifth state machine.
2. SM-1 is owned only by `blocks/run`.
3. SM-2, SM-3, and SM-4 are owned only by `kernel/orchestrator` and mutate only through exhaustive private internal
   events and the single Runtime CAS coordinator.
4. Workspace state and `ActiveRunID` are owned only by `blocks/workspace`; Result lifecycle is owned only by
   `blocks/result`.
5. Cross-owner EventHub contracts carry only immutable intent, fact, query, receipt, or acknowledgement. They never
   carry target state, next action, retry choice, terminal choice, reducer instruction, repository, or complete mutable
   owner entity.
6. Application, TUI, Web, projection, LLM, and capability owners do not choose Runtime transitions.
7. Run persists the Runtime aggregate as opaque data. It cannot interpret or rebuild SM-2/3/4 authority.
8. A complete semantic contract must be sealed before operation candidate generation. Candidate claims, prose, exit
   code, file existence, Git revision, generic checks, or assessments never create CWI credit.
9. Provider-facing authority documents must be bounded complete documents. Runtime authority must not depend on an LLM
   reproducing an exact raw string suffix across physical calls.

## 4. Target Unified Flow

```text
Application compiles typed RuntimeBootstrap
  -> Workspace owner validates current definition and active-run identity
  -> Run owner handles SM-1 activation/restart intent
  -> Orchestrator SM-4 drives persisted work
  -> SM-2 requires semantic contract
  -> SM-3 requests one bounded complete coverage-order document
  -> Orchestrator compiles persisted proof batches
  -> SM-3 requests one bounded complete document per proof batch
  -> capability owners return typed feasibility facts
  -> SM-2 atomically seals binding/reconciliation authority
  -> SM-2 derives operation authority
  -> SM-3 seals and dispatches operation candidate
  -> capability owner returns receipt
  -> SM-2 admits evidence and advances focus
  -> SM-4 delivers terminal fact: Result -> Workspace -> Run
```

## 5. Closure Track A: Restart Ownership And Ordering

### 5.1 Target Sequence

All fresh-activation and ordinary restart paths must use one sequence:

```text
Application -> WorkspaceExecutionIntent(restart)
  -> Workspace validates Goal/DoD/revision and forwards immutable selection fact
  -> Run private event persists ReplacementIntent on the predecessor
  -> Run publishes a persisted administrative termination fact
  -> Orchestrator SM-4 seals and delivers predecessor terminal outcome
       Result owner internal event
       Workspace owner internal event releases exact ActiveRunID
       Run owner internal event terminalizes predecessor
  -> Run owner idempotently creates successor from persisted ReplacementIntent
  -> Run owner internal activation event starts successor
  -> Run publishes lifecycle facts for projection and Orchestrator drive
```

The final Run delivery is sent only after the matching Workspace acknowledgement is persisted. Receipt of that final
delivery is therefore the Run owner's authorization to materialize the successor.

### 5.2 Run-Owned Replacement Intent

The predecessor Run owns the durable replacement request until the successor is created. The value must include:

- workspace and predecessor Run identity;
- idempotency/request ID;
- immutable current-schema bootstrap;
- definition revision and `workspace_full` reconciliation scope;
- requested administrative terminal kind (`superseded`);
- request state needed for owner-local recovery;
- successor ID only after successful creation.

It must not include Workspace target state, SM-2 convergence state, SM-4 acknowledgement state, or an Orchestrator
action.

### 5.3 Remove Parallel Restart Paths

Delete or absorb the following behavior after migration:

- creating the replacement before predecessor delivery;
- storing `ReplacementTerminationIntent` on an already-created successor;
- synchronous `FreshActivationRelease` returned from Run intent handling;
- Workspace-specific fresh-release bypass distinct from normal terminal delivery;
- Application retries or queues that infer restart phase;
- any callback or repository access used to bypass EventHub owner contracts.

Fresh schema replacement and normal user restart differ only in the Run-owned reason/fact that initiates the same SM-4
delivery. They do not use separate lifecycle protocols.

### 5.4 Restart Invariants

- No successor record exists before the matching Workspace release acknowledgement.
- At most one replacement intent and one successor exist per restart request ID.
- Workspace releases only when `ActiveRunID` exactly matches the predecessor.
- A stale release cannot clear a newer Run.
- Result success plus Workspace failure remains delivery-pending; it does not create the successor.
- Process restart resumes from the Run-owned replacement intent and SM-4 ledger.
- Duplicate terminal delivery is a no-op after the same successor is created.
- Two workspaces with equal local Run IDs cannot share restart identity, timer, or singleflight state.

## 6. Closure Track B: Complete Semantic Documents

### 6.1 Sealed Expected Document Contract

Before every physical provider call, SM-3 persists an owner-neutral expected document contract containing:

- `documentKind`;
- `planStep`;
- `manifestHash` and `definitionSeedHash`;
- logical request ID and semantic correction attempt;
- stable prefix hash;
- for proof batches: `batchId`, `batchSequence`, `batchCoverageWorkItemIds`, and coverage-order hash;
- maximum output tokens and request recovery budget.

The LLM command carries this value as immutable input. The LLM result echoes its identity but cannot modify it or select
the next phase.

### 6.2 One Complete Document Per Physical Call

Remove raw suffix concatenation from Runtime authority documents. This applies to both `semantic_plan` and `operations`
documents so the shared provider transaction has one rule.

The LLM owner returns one typed factual outcome:

| Outcome | Meaning | Forbidden interpretation |
| --- | --- | --- |
| `complete_document` | one complete final-output JSON value was received | accepted candidate or next action |
| `incomplete_document` | output ended before the first JSON value completed | semantic rejection |
| `empty_document` | no final output text | successful no-op |
| `invalid_provider_output` | final channel is not one JSON value | schema correction |
| provider transport/auth/config fact | provider request did not produce an adjudicable document | Runtime terminal selected by LLM |

Raw provider bytes remain LLM-owner audit material subject to redaction rules. Cross-owner results carry only bounded
content where allowed, hashes, sizes, usage, identity, disposition, and safe original diagnostic.

### 6.3 Request Recovery

`incomplete_document` and provider output-limit outcomes enter the existing SM-4 request-recovery ledger. Recovery
resubmits the same sealed expected contract as a new physical call and requests a complete document from the beginning.
It does not concatenate prior output, create a candidate attempt, consume semantic correction attempt, alter the stable
prefix, or expand the batch.

After the bounded request-recovery budget is exhausted, SM-2 terminalizes with the original provider diagnostic. There
is no immediate in-memory retry loop.

### 6.4 Coverage Order And Proof Batch Discipline

1. `coverage_order` is requested first and contains the complete CWI order only.
2. A complete order document is strictly checked against manifest membership, uniqueness, dependency barriers, and
   current definition identity.
3. A correctable complete rejection may create exactly one correction for the same `planStep=coverage_order` and the
   same manifest/stable prefix.
4. Only an accepted order allows an Orchestrator private event to compile the next bounded proof batch.
5. `proof_batch` is invalid unless it exactly matches a persisted batch ID, sequence, CWI set, and order hash.
6. Provider context for a proof batch contains only that batch's CWI/proof vocabulary plus stable C0-C2 contracts.
7. Current batch feasibility must complete before the next batch is compiled.
8. Only after all batches are accepted may SM-2 merge documents/feasibilities and seal the binding set.

Wrong `planStep`, missing batch identity, out-of-batch proposal, repeated CWI, or altered order is a complete semantic
contract rejection. It never becomes continuation or provider transport recovery.

### 6.5 Internal Transition Ownership

External LLM results are facts only. The Orchestrator maps them through the single private dispatcher:

| Current authority | Private internal event | Owner | Result |
| --- | --- | --- | --- |
| semantic contract required | `semantic_plan_request_started` | SM-3 | sealed expected document contract/session |
| provider complete fact | `candidate_document_observed` | SM-3 | durable complete document, then strict evaluation |
| provider incomplete fact | `candidate_request_recovery_required` | SM-4 | persisted bounded recovery lease |
| complete correctable rejection | `semantic_plan_correction_required` | SM-3 | one same-step correction |
| accepted coverage order | `semantic_plan_batch_compiled` | SM-2/SM-3 private transaction | persisted bounded batch |
| accepted proof batch | `semantic_plan_batch_document_observed` | SM-3 | batch document fact only |
| matching owner facts | `semantic_plan_batch_feasibility_observed` | SM-2 | accepted batch or typed gap |
| all batches complete | `semantic_contract_sealed` | SM-2 | binding/reconciliation authority |
| final rejection/budget exhaustion | `semantic_contract_failed` | SM-2 | sealed terminal decision |

Exact names may reuse current internal-event constants, but no new EventHub topic may expose these transitions.

## 7. Component Scope

| Component | Required change | Must continue to own only |
| --- | --- | --- |
| `internal/initiators/workorchruntime` | repository provider/factory only; no Runtime drive, EventHub or policy export | repository construction and persistence wiring |
| `application/workorchd` | submit one Workspace execution intent; remove restart phase assumptions | bootstrap compilation and protocol ingress |
| `blocks/workspace` | validate current selection; release slot only through private lifecycle event | Workspace state, definition selection, `ActiveRunID` |
| `blocks/run` | persist predecessor replacement intent; create successor only after final delivery | SM-1, lineage, opaque Runtime aggregate storage |
| `kernel/orchestrator` | enforce expected-document identity, complete-call recovery, batch transitions, SM-4 ordering | SM-2, SM-3, SM-4 |
| `blocks/llm` | Responses final-channel parsing and typed complete/incomplete/invalid facts; no authority suffix stitching | provider transport, catalog capability, usage/audit |
| `blocks/result` | preserve idempotent terminal materialization | Result lifecycle only |
| File/Git/Shell/MCP/Web/SSH | no state-machine change; return typed feasibility/receipt facts | each capability resource only |
| `workorchruntimecontract` | pure expected-document/outcome/identity validation and hash helpers | immutable owner-neutral values only |
| projection/eventstream/TUI/Web | display replacement/recovery/semantic phase facts | read models and user intent only |

No component may receive another owner's service, repository, reducer, mutable model, or state callback.

Registered component paths covered by this matrix are:
`internal/initiators/workorchruntime`, `internal/modules/application/workorchd`,
`internal/modules/blocks/approval`, `internal/modules/blocks/decision`,
`internal/modules/blocks/eventstream`, `internal/modules/blocks/file`, `internal/modules/blocks/git`,
`internal/modules/blocks/llm`, `internal/modules/blocks/mcp`, `internal/modules/blocks/mcpregistry`,
`internal/modules/blocks/memory`, `internal/modules/blocks/result`, `internal/modules/blocks/run`,
`internal/modules/blocks/shell`, `internal/modules/blocks/skill`, `internal/modules/blocks/soul`,
`internal/modules/blocks/ssh`, `internal/modules/blocks/web`, `internal/modules/blocks/workspace`,
`internal/modules/kernel/governance`, `internal/modules/kernel/orchestrator`, and
`internal/modules/kernel/system`.

## 8. Contract And File Migration

### 8.1 Restart Path

Primary files to audit and change:

- `internal/modules/blocks/run/biz/workspace_execution_intent.go`
- `internal/modules/blocks/run/biz/replacement_termination_delivery.go`
- `internal/modules/blocks/run/biz/lifecycle_internal_event.go`
- `internal/modules/blocks/run/pkg/models/run.go`
- `internal/modules/blocks/run/pkg/events/lifecycle_messages.go`
- `internal/modules/blocks/workspace/biz/execution_intent.go`
- `internal/modules/blocks/workspace/biz/lifecycle_internal_event.go`
- `internal/modules/blocks/workspace/pkg/events/lifecycle_messages.go`
- Orchestrator administrative terminal-delivery adapter and tests

Required deletion scan after migration:

```text
FreshActivationRelease
FreshActivationRunSlotReleaseFact
successor-owned pending ReplacementTerminationIntent
replacement creation before workspace release acknowledgement
```

### 8.2 Provider Transaction Path

Primary files to audit and change:

- `internal/modules/blocks/llm/biz/runtime_semantic_provider.go`
- `internal/modules/blocks/llm/biz/runtime_candidate_provider.go`
- `internal/modules/blocks/llm/biz/runtime/candidate_boundary.go`
- `internal/modules/blocks/llm/biz/http_provider.go`
- `internal/modules/blocks/llm/pkg/events`
- `internal/modules/blocks/llm/pkg/models`
- `internal/modules/kernel/orchestrator/biz/runtime_candidate_request*.go`
- `internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go`
- `internal/pkg/workorchruntimecontract/runtime_candidate_*.go`
- `internal/pkg/workorchruntimecontract/runtime_semantic_*.go`

Required deletion scan after migration:

```text
authority document raw suffix concatenation
provider prompt requesting exact next bytes
continuation fragment accepted as a complete authority suffix
correction that changes expected plan step
proof-batch request without persisted batch identity
```

Artifact continuation storage may remain only for audit artifacts unrelated to authority document assembly. Its state
must not select Runtime recovery, correction, candidate admission, or terminal outcome.

## 9. Ordered Implementation Queue

Statuses are `pending | in_progress | code_closed | live_closed`. A code-closed item without fresh deployed proof remains
open.

| ID | Status | Work item | Completion evidence |
| --- | --- | --- | --- |
| RSO-1 | code_closed | Synchronize core contract, request/cache note, Orchestrator readme, and typed value vocabulary | core docs synchronized; obsolete request-size plan removed; contract packages compile |
| RSO-2 | code_closed | Move restart replacement intent to predecessor Run and add owner-private transitions | predecessor CAS, conflict, clone, pre-terminal absence and duplicate materialization tests |
| RSO-3 | in_progress | Route predecessor termination through one SM-4 delivery and create successor only after Workspace ack | current-schema final Run delivery materializes one successor; schema-incompatible persisted delivery remains open |
| RSO-4 | in_progress | Remove fresh-activation release bypass and successor-owned termination intent | production symbols/imports removed; unified fresh restart test depends on RSO-3 schema-incompatible delivery |
| RSO-5 | code_closed | Add sealed expected-document contract and typed provider disposition | pure value validation/hash/order-sensitive clone and EventHub boundary round-trip tests pass |
| RSO-6 | code_closed | Remove raw suffix assembly from semantic and operations authority documents | partial bytes no longer cross the LLM boundary; raw suffix functions/types/config removed; forbidden-symbol scan is clean |
| RSO-7 | code_closed | Implement whole-document bounded request recovery in SM-4 | persisted incomplete fact, duplicate/conflict, whole-document restart, expected-identity stability and exhaustion-without-attempt tests pass |
| RSO-8 | code_closed | Enforce same-step correction and persisted proof-batch identity | expected-document session + EventHub hash fence; empty-step, first-batch sequence, correction-step/batch and request-order tests pass |
| RSO-9 | pending | Re-run binding/feasibility/seal/dispatch/evidence regression matrix | focused, full, race, boundary, managed checks |
| RSO-10 | pending | Fresh deployed LSP Bridge positive convergence | complete live event/audit chain |

Implementation order is strict. RSO-2/3 must not begin before RSO-1 fixes the authority documents. RSO-7/8 must not
begin before RSO-5/6 establish the single provider outcome contract. RSO-10 cannot be replaced by mock-provider proof.

## 10. Verification Matrix

### 10.1 Restart

- no predecessor: create and activate exactly one Run;
- active predecessor: no successor before `Result -> Workspace -> Run` delivery completes;
- fresh-activation-required predecessor: same sequence as ordinary restart;
- Workspace release rejection: no successor;
- Result accepted, Workspace response lost: replay same delivery and create one successor;
- Run terminal response lost after successor creation: replay is no-op;
- duplicate restart before/after crash: one replacement request and one successor;
- stale predecessor ID cannot release or replace a newer active Run;
- two workspaces and equal local Run IDs remain isolated;
- startup recovery resumes persisted replacement and delivery without CPU spin.

### 10.2 Semantic Provider Boundary

- direct compact JSON;
- one exact outer `json` fence;
- prose/reasoning-only final output;
- JSON-string-encoded or escaped JSON;
- multiple JSON values;
- empty final channel;
- wrong `planStep` on correction;
- proof batch missing/mismatching batch identity;
- output limit before JSON completion;
- transport failure before/after provider submission;
- duplicate physical outcome and response loss;
- whole-document recovery preserves logical request, transaction, stable prefix, expected contract, and batch;
- recovery exhaustion creates no candidate attempt, dispatch, Decision, or Approval.

### 10.3 Four State Machines

- transition and forbidden-transition table tests for SM-1 through SM-4;
- scope, source owner, authority identity, stale revision, duplicate event, and CAS conflict tests;
- crash/restart at every persistence/response boundary;
- state revision and convergence progress signature advance together;
- terminal decision remains immutable while delivery is pending;
- capability receipt can affect only matching sealed operation/binding;
- projection failure cannot rewrite owner authority;
- no busy loop when provider, CAS, prerequisite, or delivery recovery is pending.

### 10.4 Commands

Run focused checks first, then the full repository gates:

```bash
go test ./internal/modules/blocks/run/... -count=1
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/pkg/workorchruntimecontract -count=1
make fmt-check
make vet
go test ./... -count=1
go test -race ./... -count=1
make console-build
make verify-boundary-contract
make verify-ci-mvp-file
```

Managed verifiers must use a managed disposable instance. They must not bypass the guard protecting an unmanaged live
workspace.

## 11. Fresh Deployed Completion Gate

The plan is `live_closed` only when a fresh deployment proves all of the following in one auditable lineage:

```text
restart intent
  -> predecessor replacement intent persisted
  -> Result acknowledgement
  -> Workspace release acknowledgement
  -> predecessor Run terminal acknowledgement
  -> successor created and activated
  -> coverage_order accepted
  -> bounded proof batches accepted
  -> complete owner feasibility facts
  -> SM-2 binding/reconciliation seal
  -> operation candidate accepted
  -> sealed dispatch
  -> capability receipt
  -> binding-matched evidence
  -> focus/CWI progress
  -> durable workspace output
```

The live proof must also show:

- zero cross-owner target-state command;
- no authority document suffix concatenation;
- no operation before semantic seal;
- no predecessor credit inherited by restart;
- stable prefix consistency within each sealed logical request;
- bounded retries and no high-CPU loop;
- source/projected revision convergence;
- correct TUI/Web projection after workspace switching.

Until this gate passes, previous `R3-E2`, `R3-E3`, `R3-F5`, managed mock, or local test results remain regression
baselines and must not be described as current live closure.

## 12. Forbidden Closure Tactics

- Do not fix only the observed backslash or strip arbitrary leading text.
- Do not search for the first `{` or accept multiple candidate JSON objects.
- Do not let LLM output choose correction, retry, batch, owner, terminal, or Runtime action.
- Do not move restart coordination state into Application, Workspace projection, TUI, or Web.
- Do not let Orchestrator create/modify Run or Workspace entities directly.
- Do not add a generic event envelope, global topic index, cross-owner facade, service registry, or repository callback.
- Do not retain compatibility aliases or dual restart/continuation paths.
- Do not increase token/time limits as a substitute for bounded batches and complete-document recovery.
- Do not mark code-closed work live-closed without fresh deployed positive convergence.
