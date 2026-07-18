# Runtime V2 Production Admissibility Closure

Status: completed

Type: completion record

Last Updated: 2026-07-18

## Purpose

This plan closes the production-admissibility gaps found by the 2026-07-18
read-only Runtime V2 live review. It is the implementation plan for making a
new Runtime V2 activation both safely convergent and a meaningful candidate for
positive acceptance. It does not reopen V1 execution, introduce an application
continuation queue, or alter terminal Runs.

The governing state-machine rules remain
[runtime-stage-contract.md](../../00-core/runtime-stage-contract.md). The
Orchestrator implementation rules remain
`internal/modules/kernel/orchestrator/readme.md`.

## Observed Production Facts

The deployed `LSP-Bridge` and `ai-proxy` Runs reached legal typed `failed`
terminals. They did not stall, leak `activeRun`, lose projection alignment, or
repeat a side effect. Their failures were candidate-quality failures:

- LSP-Bridge sealed a patch candidate whose target `.` was a directory; the
  File owner returned `patch_read_failed`.
- ai-proxy sealed a validation candidate whose Go test path/selector had no
  matching test files; the validation owner returned `validation_target_missing`.
- The provider then proposed the exact rejected operation shape again. Runtime
  V2 correctly rejected it under the 0/1 candidate-attempt contract and
  terminalized with `candidate_retry_exhausted`.

The actual profile policy also compiled large slices: up to 16 coverage items
per slice, five slices for LSP-Bridge and 31 slices for ai-proxy. This is below
the current technical ceilings (32 items / 64 focuses), but it is outside the
small bounded window that has demonstrated positive terminal convergence.

Finally, liveness observation passed because a legal terminal lifecycle change
counts as liveness. That is correct for safety, but its success label must not
be read as accepted positive convergence.

## Non-Goals

- Do not resume, rewrite, migrate, or otherwise mutate existing terminal Runs.
  New behavior applies only to a newly created Runtime V2 activation.
- Do not hard-code Go, test-framework, patch, LSP, HTTP, or software-product
  semantics in the reducer or Runtime V2 core.
- Do not add V1 `StagePacket`, `NextAction`, generic dispatcher, execution
  patch, auto-successor, or application-side drive queue.
- Do not let an owner mutate an LLM candidate or decide focus, Result, repair,
  or Run terminal state.

## Target Architecture

```text
typed bootstrap + sealed execution budget
  -> prerequisites and owner capability parameter inventory
  -> reducer selects sole required action
  -> LLM candidate attempt 0/1
  -> typed owner candidate-feasibility probes (read-only)
  -> feasible candidate only: seal dispatch
  -> typed owner execution receipt
  -> Orchestrator evidence/reducer/terminal authority
  -> Result + Run/Workspace projection
```

The new feasibility phase is an SM-3 admission gate. It is not execution and
cannot create an owner receipt, modify a deliverable, or advance a focus. It
may use SM-2 prerequisite facts, but it cannot create a second reducer.

## Authority And Persistence Boundaries

The following placement is mandatory so that the new data has one authority
and service restart has no semantic effect:

| Concern | Authority and persisted location | Allowed transition |
| --- | --- | --- |
| Profile execution budget | activation input, copied to `RuntimeBootstrap` and `RuntimeConvergencePolicy` | activation only; immutable afterwards |
| Owner admissibility inventory | `RuntimeFacts`, keyed by owner, prerequisite ID and authority snapshot | SM-2 prerequisite commit only |
| Candidate feasibility result | current `RuntimeCandidateAttempt` failure record; diagnostics may be referenced from facts | SM-3 candidate-attempt commit only |
| Accepted candidate / dispatch | existing sealed dispatch and candidate-attempt ledger | only after every probe returns feasible |
| Owner execution receipt and evidence | existing receipt/evidence ledgers | only after a sealed dispatch is executed |
| Run and workspace state | existing SM-1 and SM-4 projections | derived from persisted aggregate only |

`RuntimeFacts` stores bounded, normalized inventory facts and their snapshot
identity, not an owner-specific execution plan.  A candidate feasibility
failure is stored in the existing candidate-attempt record with its stable
failure signature.  It is deliberately not inserted into rejected execution
shapes, evidence, repair ledger, or operation recovery ledger.

All added EventHub DTOs and topics are owned by
`kernel/orchestrator/pkg/events`, alongside the existing prerequisite,
candidate, and concrete-operation contracts.  The Orchestrator validates the
scope and authority transaction before `Send`; each owner validates the same
scope and canonical owner binding before replying.  The EventHub result is
concrete typed data, never an envelope, `map[string]any`, serialized JSON, or
direct service/repository call.

## RPA-1: Seal A Production Execution Budget

### Contract

Replace the activation acceptance-slice policy with a current, explicit schema
that requires all production bounds:

```text
schemaVersion: workorch.acceptance_slice_policy.v2
maxWorkItemsPerSlice: positive integer
maxFocusesPerRun: positive integer
maxCandidateContextItems: positive integer
groups[].maxWorkItems: 1..maxWorkItemsPerSlice
```

The policy is profile input at activation and is copied into the typed
`RuntimeBootstrap` / `RuntimeConvergencePolicy`. It is immutable after Run
creation. New activations reject a missing or non-current schema; there is no
v1 policy fallback or aggregate migration.

`RuntimeBootstrap.Validate` must independently enforce all of the following,
not merely trust the activation compiler:

1. Focus count is at most the sealed `maxFocusesPerRun`.
2. Every slice and corresponding focus has at most the sealed
   `maxWorkItemsPerSlice` coverage items.
3. Focus and slice identity are one-to-one; each focus references exactly its
   declared slice's coverage set.
4. Every coverage item is declared once, belongs to one slice, and is covered
   by one focus.
5. The candidate-context item bound can be satisfied before a provider request.

The current global constants remain absolute denial-of-service ceilings, not
the profile's operational budget.

### Initial Profile Policy

The software-dev profile must start with a bounded value demonstrated by the
terminal fixture: `maxWorkItemsPerSlice=4`. It must declare a finite
`maxFocusesPerRun` based on the profile's intended Run size. A definition that
does not fit the budget is rejected before Run creation and must be decomposed
by product-level planning into an explicit new activation; it must not silently
create dozens of LLM loops.

The exact profile focus cap is a product contract value and must be committed
with the template/config change plus a capacity test. It is not a kernel
constant and must not default from legacy configuration.

### Files Expected To Change

- `internal/pkg/workorchruntimecontract/runtime_acceptance_slice.go`
- `internal/pkg/workorchruntimecontract/runtime_repair_ledger.go`
- `internal/pkg/workorchruntimecontract/runtime_bootstrap.go`
- `internal/modules/application/workorchd/biz/appservice/activationports/runtime_v2_activation.go`
- `internal/modules/application/workorchd/biz/appservice/activationcore/runtime_v2_focus_compiler.go`
- current template/config fixtures and their tests

## RPA-2: Add Owner Parameter-Admissibility Facts

### Two Read-Only Contracts

Capability owners provide two complementary typed contracts through EventHub.
Both contracts contain concrete DTO fields only. They must not use `map[string]any`,
JSON bridges, callbacks, owner repositories, or opaque command envelopes.

1. **Prerequisite inventory** before candidate generation:

```text
AcquireRuntimeOperationAdmissibilityCommand
  WorkspaceID, RunID, AuthorityTransaction, OwnerModuleID, RequiredKinds

AcquireRuntimeOperationAdmissibilityResult
  OwnerModuleID, OperationKinds, TargetRefs, SelectorRefs, Constraints, SnapshotRef
```

The result is normalized, bounded, persisted in Runtime facts, and rendered in
the volatile candidate context. It describes only currently admissible
parameters, not a business plan.

2. **Candidate feasibility probe** after candidate parsing and before seal:

```text
ProbeRuntimeOperationFeasibilityCommand
  WorkspaceID, RunID, AuthorityTransaction, Operation, AdmissibilitySnapshotRef

ProbeRuntimeOperationFeasibilityResult
  OperationID, Feasible, FailureFact, DiagnosticRefs
```

The probe is side-effect-free. `Feasible=false` must include a stable typed
failure class/code/signature and bounded diagnostics suitable for attempt-1
context. It must never rewrite the operation.

The Orchestrator owns these topics and routes each command only to the sealed
descriptor's canonical owner. A missing owner, stale snapshot, mismatched
workspace/run/transaction, or invalid result is a typed contract failure.

### Owner Responsibilities

| Owner | Inventory / probe responsibility |
| --- | --- |
| File/Patch | controlled target is in workspace scope; patch/read target is a regular file when required; base/content constraints are usable |
| Shell / managed validation | working directory, executable validation entry, selector/test target and bounded command parameters are presently usable |
| Git | repository state and operation variant prerequisites are coherent with the owner snapshot |
| MCP/Web/SSH | declared capability, target, parameter domain and required governance preconditions are satisfiable |

The reducer only sees neutral feasible/infeasible facts and never branches on
Go, file extensions, test names, or a specific tool.

## RPA-3: Make Feasibility Part Of Candidate Admission

The Runtime V2 candidate path becomes:

1. SM-2 acquires all normal prerequisites plus required owner admissibility
   inventories.
2. LLM returns a schema-valid candidate for attempt 0 or 1.
3. Existing static validation remains: action, effect scope, operation kind,
   variant, capability, coverage, dependency, and parameter shape.
4. Orchestrator sends one feasibility probe per candidate operation in stable
   dependency order.
5. Only an all-feasible candidate can be accepted and sealed.

Failure handling is exact:

- A probe failure before seal records a `RuntimeCandidateFailure` for the
  current attempt. It creates no dispatch, receipt, evidence, side effect, or
  repair debit.
- Attempt 1 receives the persisted attempt-0 failure and relevant admissibility
  facts in C3/C4. The stable prefix and authority transaction remain unchanged.
- If attempt 1 remains infeasible, the existing bounded candidate exhaustion
  path produces the typed rejected terminal.
- A failure after a dispatch is sealed remains owner evidence and returns to
  SM-2 repair/governance/fail reduction. `ActiveRejectedOperationShapes`
  continues to derive only from persisted rejected execution evidence and
  completed sealed dispatches.

This preserves the existing anti-replay rule. It does not redefine a failed
unsealed proposal as executed rejected evidence.

### Candidate Rendering

`blocks/llm` must render:

- the current required action and repair directive;
- owner admissibility inventories and snapshot refs;
- attempt-0 feasibility failure for attempt 1;
- exact active rejected execution shapes only when such evidence exists.

The provider cannot choose a target or selector outside the inventory. Context
compaction may omit diagnostic prose, but cannot omit the admissible operation
set, required effect scopes, or current failure constraints.

### Admission Algorithm

For a response that passed the existing static candidate validator, the
Orchestrator performs this deterministic algorithm before calling
`AcceptRuntimeV2CandidateAttempt`:

1. Order the candidate operations using the same validated dependency order
   used by sealed dispatch execution.
2. Resolve each operation's canonical owner from the sealed descriptor, then
   load the matching current inventory snapshot from `RuntimeFacts`.
3. Send exactly one typed feasibility command per operation. Reject a missing,
   stale, mismatched, duplicate, or malformed result as a candidate contract
   failure.
4. On the first infeasible result, persist a bounded
   `RuntimeCandidateFailure`, with the owner failure class/code/signature and
   diagnostic reference. Do not call acceptance, seal a dispatch, or probe
   later independent operations.
5. Only when every operation is feasible, call the existing candidate
   acceptance/seal path unchanged.

The failure record for attempt 0 is the only retry input for attempt 1. It
must be rendered together with its inventory snapshot. A restarted drive
reloads that persisted attempt record; it must not rerun an already committed
probe or request an additional candidate attempt.

### Work Packages And Tests

| Package | Code boundary | Completion test |
| --- | --- | --- |
| RPA-2a contracts | orchestrator event DTOs, topics, contract registry and typed adapters | wrong type, scope, authority transaction, owner, snapshot, or duplicate result is rejected |
| RPA-2b File owner | File module inventory/probe handler and File runtime adapter | directory patch target and out-of-scope target return a stable infeasible fact without a file receipt or filesystem mutation |
| RPA-2c validation owner | Shell managed-validation inventory/probe handler and adapter | unavailable working directory, executable, or selector returns a stable infeasible fact without starting a process or validation receipt |
| RPA-2d remaining owners | Git, MCP, Web and SSH handlers only for descriptors their profile exposes | each exposed descriptor has inventory/probe coverage and an idempotent no-side-effect test; an unimplemented exposed descriptor rejects activation rather than bypassing admission |
| RPA-3 reducer integration | Runtime V2 prerequisite/mainline and candidate-context rendering | attempt 0 infeasible -> persisted failure -> corrected attempt 1 -> one sealed dispatch; two infeasible attempts -> existing typed exhaustion terminal |

## RPA-4: Separate Liveness From Positive Acceptance

Split deployed verification results into explicit classifications:

| Classification | Required assertion | Intended command |
| --- | --- | --- |
| `lifecycle_liveness` | no drive stall, no `outcome_unknown`, no active-run leak, projection aligned; a legal rejected terminal is observable | `observe-runtime-v2-liveness` |
| `positive_acceptance` | accepted/completed outcome, accepted evidence, closed coverage, accepted durable owner receipt, released active run | `verify-runtime-v2-positive-convergence-live` |
| `bounded_terminal_acceptance` | positive acceptance plus declared fixture bounds | `verify-runtime-v2-bounded-terminal-live` |

The observer command may exit zero for a safe rejected terminal, but its summary
must say `safe_terminal_rejected`, never `positive-convergence passed`.
The positive commands must reject failed, cancelled, superseded, or merely
legal terminal Runs.

The final Make targets and script summaries must use these names consistently:

```text
observe-runtime-v2-liveness
verify-runtime-v2-positive-convergence-live
verify-runtime-v2-bounded-terminal-live
```

The existing script mode switch can remain an implementation detail, but its
human-readable summary and generated `summary.json` must include the explicit
classification. In particular, a liveness observation of a rejected terminal
reports `safe_terminal_rejected`, while a terminal acceptance command fails
with the rejected terminal reason. All artifacts remain outside the repository
under the configured temporary artifact root.

## Four-State-Machine Mapping

| State machine | Change | Forbidden consequence |
| --- | --- | --- |
| SM-1 Run lifecycle | none to legal transitions; terminal Runs remain immutable | no Resume or terminal rewrite for current failed Runs |
| SM-2 authority/focus | admissibility inventory is a prerequisite fact; execution budget is sealed | no profile/tool-specific reducer branch or auto successor |
| SM-3 candidate/dispatch | read-only probe gates seal; infeasible proposals consume only candidate attempt budget | no probe side effect, receipt, evidence, or owner-driven next action |
| SM-4 drive/recovery/projection | verifier reports safety and positive acceptance separately | no projection-derived authority or terminal rewrite |

## Implementation Order

1. **RPA-1a Contract budget**: add current acceptance policy schema and sealed
   bootstrap fields; make bootstrap validation complete; add rejection tests.
2. **RPA-1b Profile policy**: update software-dev template/config and activation
   tests to the explicit bounded policy.
3. **RPA-2 Contracts**: add EventHub topics/DTOs, owner inventory/probe ports,
   lane binding and scope/transaction validation.
4. **RPA-2 Owners**: implement File/Patch and managed-validation first, then
   Git/MCP/Web/SSH as applicable. Every owner gets direct no-side-effect tests.
5. **RPA-3 Orchestration**: persist prerequisite facts, call probes before seal,
   record attempt failures, and render attempt-specific admissibility context.
6. **RPA-4 Verification**: split liveness/positive result classes, update Make
   targets and artifact summaries, then update docs/readmes.
7. **Live acceptance**: create fresh bounded workspaces only. Existing terminal
   Runs remain audit evidence and are not reused.

## Code Closure Status (2026-07-18)

RPA-1 through RPA-4 are code-closed locally. The implementation now has the
following durable boundaries:

- RPA-1 seals schema-v2 work-item, focus, and candidate-context budgets into
  bootstrap/convergence policy and rejects a definition that exceeds them.
- RPA-2 acquires bounded owner inventories through typed EventHub commands.
  File and Shell perform workspace/executable/target preflight; Git rechecks a
  read-only repository-state snapshot; MCP publishes the current enabled
  server IDs as selectors and checks their allowlist/local transport launch
  contract; Web checks current provider
  configuration and URL policy; SSH checks local command/auth/key prerequisites.
- RPA-3 persists the unsealed proposal **and each completed read-only probe**
  in the candidate-attempt ledger. Restart/duplicate delivery reuses those
  records; a feasible ledger is cleared atomically when the candidate becomes
  a sealed dispatch, while an infeasible ledger remains audit-only with the
  failed attempt. It never becomes evidence, a receipt, a repair debit, or a
  rejected execution shape. `AcceptCandidateAttempt` independently requires
  the complete persisted feasible ledger before it may write a dispatch.
- RPA-4 gives deployed verification mutually exclusive `lifecycle_liveness`,
  `safe_terminal_rejected`, `positive_acceptance`, and
  `bounded_terminal_acceptance` classifications.

Local proof completed: `go test ./... -count=1`,
`make verify-runtime-v2-positive-convergence-file`, `make fmt-check`,
`make verify-boundary-contract`, `go vet ./...`, and `git diff --check`.
Fresh deployed evidence completed on 2026-07-18: the two newly created bounded
fixtures reached `bounded_terminal_acceptance` without errors, followed by
`positive_acceptance`; artifacts are retained outside the repository at
`/tmp/workorch-e2e/runtime-v2-bounded-fixtures/20260718T101000Z/`.

## Code Closure Checklist

Implementation is complete only after every checkbox is true. A partial owner
implementation is not sufficient when its descriptor remains activatable.

- [x] RPA-1 current-schema acceptance budget is required by production
  activation, persisted in bootstrap, and independently validated on aggregate
  load.
- [x] RPA-1 profile template declares all three operational bounds and rejects
  a definition that exceeds them before any provider request.
- [x] RPA-2 EventHub contract registration, subscribers, adapters, and
  transaction/snapshot validation exist for every exposed operation owner.
- [x] RPA-2 inventory facts are bounded, canonicalized, persisted, and included
  in the volatile candidate context without changing C0--C2 cache content.
- [x] RPA-3 static validation and all feasibility probes finish before the
  accepted-attempt record or sealed dispatch can be written.
- [x] RPA-3 infeasible proposals create neither an execution receipt nor
  evidence, repair debit, rejected execution shape, effect, or owner mutation.
- [x] RPA-3 candidate retry remains exactly 0/1 and restart/duplicate delivery
  cannot produce an extra probe, candidate request, or dispatch.
- [x] RPA-4 liveness, positive acceptance, and bounded terminal acceptance
  produce distinct machine-readable classifications and exit semantics.
- [x] Unit, file-backed restart, and fresh deployed acceptance evidence meet
  the verification matrix below; no test artifact is under the repository.

## Required Verification

### Contract And Unit

- current acceptance policy rejects missing budget/schema and out-of-bound
  focus/slice/context counts;
- `RuntimeBootstrap.Validate` rejects bypassed compiler output;
- inventories and probes reject mismatched workspace/run/transaction/owner;
- owner probes are side-effect-free and idempotent;
- candidate attempt 0 infeasible -> attempt 1 corrected -> seal is valid;
- candidate attempt 1 infeasible -> typed `candidate_retry_exhausted` is valid;
- existing rejected execution shape remains distinct from an unsealed proposal.

### File-Backed Runtime

- directory-as-patch-target is rejected before seal and cannot create a receipt;
- invalid validation selector is rejected before seal and cannot create a receipt;
- valid patch/validation proceeds to owner receipt and evidence;
- restart between inventory, probe, candidate attempt, seal, owner receipt and
  evidence commit remains idempotent;
- stale inventory or probe result is diagnostic-only and cannot advance state.

### Deployed

- liveness observer explicitly classifies the current failed Runs as
  `safe_terminal_rejected` (passed before fresh-fixture activation);
- a fresh bounded fixture reaches `bounded_terminal_acceptance` and
  `positive_acceptance` with durable owner receipts and closed coverage
  (passed 2026-07-18);
- a fresh real workspace stays within its sealed profile budget or is rejected
  at activation before provider use;
- no test artifact is written beneath the repository.

## Local Evidence Matrix

This matrix is the code-closure evidence to re-run or inspect when a later
change touches the admission path. It does not replace the fresh deployed
evidence in the next section.

| Requirement | Direct regression evidence | Broader gate |
| --- | --- | --- |
| RPA-1 sealed budget | `TestRuntimeV2BootstrapSealsConfiguredConvergencePolicy`, bootstrap/focus compiler contract tests | `make verify-runtime-v2-positive-convergence-file` |
| File/Shell read-only feasibility | directory target, stale inventory, bounded inventory priority, and missing validation-target tests in `blocks/file/biz` / `blocks/shell/biz` | owner package tests |
| Git/MCP/Web/SSH owner feasibility | `TestGitOperationAdmissibilityProbeUsesCurrentRepositorySnapshot`, MCP/Web/SSH adapter preflight tests | owner package tests |
| Typed EventHub admissibility boundary | `TestAcquireRuntimeOperationAdmissibilityPrerequisiteRejectsInvalidScopeAndOwnerResult`; `TestProbeRuntimeOperationFeasibilityRejectsMalformedResult` | `make verify-boundary-contract` |
| RPA-3 no pre-seal effect / bounded retry | `TestRuntimeV2MainlineRecordsInfeasibleCandidateBeforeDispatchSeal`; corrected retry and two-infeasible exhaustion tests | Runtime V2 file-backed verifier |
| RPA-3 restart/idempotence | proposal and feasibility restart tests; candidate-attempt/feasibility record invariant tests | Runtime V2 file-backed verifier |
| RPA-4 classifier split | `scripts/verify-runtime-v2-positive-convergence-live.sh` mode/classification assertions and Make targets | deployed commands below |

The current local gate set is:

```text
go test ./... -count=1
make verify-runtime-v2-positive-convergence-file
make fmt-check
make verify-boundary-contract
go vet ./...
git diff --check
```

## Completion Criteria

This plan is complete only when all of the following are true:

1. No candidate with an owner-infeasible parameter can be sealed.
2. All new Runtime V2 bootstraps validate their own acceptance budget.
3. Profile policy, bootstrap facts, candidate context and verifier use the same
   sealed bounds.
4. Liveness output cannot be mistaken for accepted convergence.
5. File-backed restart tests and fresh deployed bounded positive acceptance pass.
6. Core remains neutral and all cross-owner calls are typed EventHub contracts.
