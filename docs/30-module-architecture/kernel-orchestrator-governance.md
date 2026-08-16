# Kernel Orchestrator / Governance Architecture

Status: active

Type: module-architecture

Last Verified: 2026-08-16

## Purpose

本文说明 `kernel/orchestrator` 与 `kernel/governance` 的实现落点和边界。Runtime 语义以
[Runtime Stage Contract](../00-core/runtime-stage-contract.md)、
[Orchestrator README](../../internal/modules/kernel/orchestrator/readme.md) 和
`internal/pkg/workorchruntimecontract` 为准。

## Orchestrator Ownership

Orchestrator is the sole semantic owner of SM-2, SM-3 and SM-4:

- facts, authority input/snapshot/transaction and canonical `ConvergenceState`;
- definition-generation semantic plan, feasibility merge, binding/reconciliation seal;
- prerequisite acquisition and freshness lineage;
- candidate request session, bounded attempt/correction and sealed dispatch transaction;
- operation receipt/evidence, assessment and governance binding/resolution;
- bounded drive/recovery, sealed terminal outcome and delivery ledger.

It does not own SM-1 Run lifecycle, Workspace definition/active slot, Result lifecycle, capability side effects, Decision or
Approval state.

## Internal Mutation Architecture

Every external command/result/notification is first mapped to a private `RuntimeInternalEvent`. The only mutation path is:

```text
typed external fact
  -> submitRuntimeInternalEvent
  -> RuntimeController.ApplyInternalEvent
  -> one package-private fact-family route
  -> owner-private reducer/component
  -> RuntimeTransitionCoordinator.Commit
  -> Run owner expected-revision CAS of opaque RuntimeStateRecord
```

The internal event is not an EventHub envelope or public registry. Fact kind classification is exhaustive and each kind maps
to exactly one activation/authority/transaction/delivery family. No handler may bypass the transition coordinator or create a
second aggregate mutation/CAS entry.

## Drive Loop

```text
Run owner notification / recovery inventory
  -> load current-baseline aggregate
  -> acquire bounded drive lease/singleflight
  -> reducer derives one canonical state
       need_prerequisite -> owner observation -> internal event -> CAS
       need_state_change | need_validation | need_assessment
         -> candidate request session -> seal -> capability/assessment -> evidence -> CAS
       need_governance -> sealed ticket request -> persisted wait -> resolution evidence -> CAS
       complete | failed -> sealed terminal -> Result -> Workspace -> Run delivery acknowledgements
  -> stop on wait/terminal; otherwise require persisted revision progress within guard
```

SM-2 chooses the next canonical state but never executes an operation. SM-3 executes only an already selected/sealed
transaction and never chooses the next action. SM-4 re-drives/delivers but never rebuilds authority.

## Cross-Owner Contracts

| Interaction | Contract owner/boundary |
| --- | --- |
| Run aggregate read/CAS/inventory | Run block `pkg/events`; payload opaque to Run |
| Workspace snapshot/terminal release | Workspace block typed contracts |
| LLM ceiling/preflight/candidate | LLM block `pkg/events`; factual result only |
| File/Git/Shell/SSH/Web/MCP execution | each capability block defines its own concrete command/result |
| Decision/Approval/Memory | corresponding state owner concrete commands |
| Result terminal persistence | Result owner consumes sealed terminal delivery material |
| Orchestrator observations | immutable drive/projection notification; never mutation authority |

No cross-owner service/repository, generic callback/envelope, JSON/reflect bridge or `map[string]any` authority rebuild is
allowed.

## Start, Resume And Restart

- Start/Resume admission belongs to Run owner SM-1; Orchestrator only consumes the resulting immutable Run fact.
- Resume carries scope only and reloads the same aggregate/sealed selection.
- Governance resolution returns through Decision owner to Orchestrator; application does not issue Resume.
- Current-baseline Restart is Run-owned `ReplacementIntent`; Orchestrator only seals/delivers predecessor administrative
  terminal material and never creates the successor.
- Non-current aggregate/restart fails closed before provider/capability work; no in-place schema migration occurs.

Repeated notification, resolution, drive or acknowledgment must not duplicate candidate attempt, side effect, evidence,
terminal Result or replacement lineage.

## Governance Module

`kernel/governance` owns governance policy evaluation and `AutoGovernanceTrace` coordination. It may combine typed
Workspace/Run/Decision/Approval/Memory/Orchestrator facts through EventHub, but it cannot:

- mutate another owner's ticket/approval/Run/Result state;
- bypass Orchestrator evidence/coverage/required-output gates;
- decide candidate/sealed dispatch or create successor Run;
- maintain a process-local policy state that conflicts with persisted facts.

`blocks/decision` owns DecisionTicket/Resolution. `blocks/approval` owns pending→approved/rejected authorization. Each uses
its own private reducer and repository transaction.

## Authority And Persistence

| Data | Semantic owner | Physical persistence |
| --- | --- | --- |
| Bootstrap/Definition/Activation seed | created by application, sealed with Run | Run owner |
| authority/focus/binding/evidence/candidate/delivery ledgers | Orchestrator | opaque `RuntimeStateRecord` CAS via Run owner |
| Run lifecycle/lineage/ReplacementIntent | Run owner | Run repository |
| Decision/Approval | respective owner | respective repository |
| accepted/rejected formal outcome | Result owner receives Orchestrator seal | Result/Artifact repositories |
| EventRecord/projection | Eventstream/workorchd derived surfaces | audit/projection stores only |

Result owner records the already-judged terminal outcome; it does not rerun acceptance. Run owner validates Runtime record
envelope/scope/revision only; it does not decode Orchestrator facts.

## Implementation Locations

| Path | Responsibility |
| --- | --- |
| `orchestrator/biz/runtime` | reducer/controller components, private event families, transactions/invariants |
| `orchestrator/biz` | EventHub adapters, drive shell, owner/capability calls, projection observations |
| `orchestrator/pkg/events` | public Orchestrator drive/governance/evidence/observation contracts |
| `orchestrator/pkg/models` | EvidenceRecord owner model |
| `governance/biz` | governance policy/trace owner logic |

Orchestrator's `service` directory has no production facade; external components interact through EventHub contracts.

## Verification

- `go test ./internal/modules/kernel/orchestrator/... -count=1`;
- `go test ./internal/modules/kernel/governance/... -count=1`;
- `make verify-boundary-contract`;
- [Runtime Owner Convergence Matrix](../60-verification/runtime-owner-convergence-matrix.md).

## Related Documents

- [Architecture Overview](architecture-overview.md)
- [Capability Adapters](blocks-runtime-adapters.md)
- [Workspace / Run / Result](blocks-workspace-run-result.md)
