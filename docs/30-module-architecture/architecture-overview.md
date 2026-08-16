# WorkOrch Architecture Overview

Status: active

Type: module-architecture

Last Verified: 2026-08-16

## Stable Layers

| Layer | Units | Responsibility |
| --- | --- | --- |
| Entry/process | `application/workorch`, `internal/services/workorch` | select TUI/server/remote mode and own process lifecycle |
| Initiator | `internal/initiators/workorchruntime` | create owner-scoped repository groups |
| Application module | `internal/modules/application/workorchd` | typed bootstrap, HTTP/SSE, admission, projection |
| Kernel modules | `orchestrator`, `governance`, `system` | Runtime convergence, governance policy, startup coordination |
| Stateful blocks | Workspace, Run, Result, Decision, Approval, Memory, Soul, Skill, MCP Registry, Eventstream | own formal aggregates or audit records |
| Capability blocks | LLM, File, Git, Shell, SSH, Web, MCP | own concrete capability calls and factual results |
| Shared packages | typed contracts, owner-neutral primitives, repository/config/ports | compile-time contracts and infrastructure only |
| Product clients | TUI and Web console | invoke stable interfaces and render projections |

There are 22 framework units: 1 Initiator, 4 Modules and 17 Blocks. The process service is intentionally outside this count.

## Runtime Ownership

| Machine/concern | Sole owner | External contribution |
| --- | --- | --- |
| SM-1 Run lifecycle/lineage/replacement | Run block | Workspace/application submit intent; Orchestrator submits immutable wait/terminal facts |
| SM-2 authority/focus convergence | Orchestrator | application seals definition seed; other owners return facts |
| SM-3 candidate/sealed dispatch transaction | Orchestrator | LLM returns candidate fact; capability owner executes sealed operation |
| SM-4 drive/recovery/delivery ledger | Orchestrator | Run persists opaque aggregate; Result/Workspace/Run acknowledge their own delivery |
| formal outcome persistence | Result block | Orchestrator supplies sealed already-judged terminal material |
| projection/audit | workorchd/Eventstream | consume immutable facts; never write owner state |

No machine may be split across components. A cross-owner request expresses intent or fact, not a target state transition.

## Dependency Direction

```text
entry/process -> framework units
workorchd -> EventHub typed owner contracts
orchestrator -> EventHub typed owner/capability contracts
owner biz -> own repositories + EventHub
transport/client -> workorchd ports/API
projection -> immutable facts and owner reads
```

Forbidden dependencies include sibling owner repository/service, generic dispatcher/envelope, EventHub carrying live objects,
projection-to-owner repair, and application-to-Orchestrator service calls.

## Restart Model

- process restart: Run owner inventories SM-1 state and opaque Runtime records; Orchestrator loads and idempotently re-drives;
- Resume: same Run, same sealed bootstrap/provider selection, no context patch;
- current-baseline Restart: fresh bootstrap plus Run-owned predecessor `ReplacementIntent`; successor only after terminal delivery;
- non-current baseline: `fresh_activation_required`/`data_reset_required` fail-closed behavior, no in-place migration;
- repeated notification/Drive/acknowledgment cannot duplicate attempt, side effect, evidence, Result or successor.

## Stable Decisions

- one `go.mod`; directory modules are ownership boundaries, not separate Go modules;
- owner units expose EventHub contracts, not direct service facades;
- Runtime aggregate is typed, Orchestrator-owned semantically, and Run-owned only as opaque CAS storage;
- core Runtime remains profile/domain neutral;
- removed StagePacket, NextAction, DispatchNextStep, ExecutionContextPatch, graph dispatcher and application recovery queues are
  not compatibility paths;
- LLM provider configuration/ceiling interpretation belongs to LLM owner; sealed selection cannot drift on Resume.

## Related Documents

- [Runtime Stage Contract](../00-core/runtime-stage-contract.md)
- [Component Documentation Alignment](../01-overview/component-documentation-alignment.md)
- [Application Workorchd](application-workorchd.md)
- [Kernel Orchestrator / Governance](kernel-orchestrator-governance.md)
- [Workspace / Run / Result](blocks-workspace-run-result.md)
- [Capability Adapters](blocks-runtime-adapters.md)
- [Repository Structure](repository-structure.md)
