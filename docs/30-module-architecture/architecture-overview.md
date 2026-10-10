# WorkOrch Architecture Overview

Status: normative

Type: module-architecture

Last Verified: 2026-08-26

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

The exact 1+4+17 framework inventory and physical paths are maintained by
[Repository Structure](repository-structure.md) and the
[Component Contract Inventory](../01-overview/component-documentation-alignment.md).

## Runtime Ownership

| Machine/concern | Sole owner | External contribution |
| --- | --- | --- |
| Workspace Definition/startup eligibility | Workspace Module | application submits candidate/admission outcome and typed clarification acknowledgement |
| SM-1 Run lifecycle/lineage/replacement | Run block | Workspace/application submit eligible intent; Orchestrator delivers immutable wait facts and exact SM-4 terminal handoff |
| SM-2 authority/focus convergence | Orchestrator | application compiles from owner-sealed startup input; other owners return facts |
| SM-3 candidate/sealed dispatch transaction | Orchestrator | LLM returns exactly one factual `Stale\|Feedback\|DocumentOutcome`; Orchestrator alone decodes/evaluates a complete authority document into a candidate, and the capability owner executes only a sealed operation |
| SM-4 drive/recovery/delivery ledger | Orchestrator | Run persists opaque aggregate; `RuntimeTerminationRecord` is delivery handoff only; Result/Workspace/Run acknowledge and privately transition their own resources |
| formal outcome persistence | Result block | Orchestrator supplies sealed already-judged terminal material |
| projection/audit | workorchd/Eventstream | consume immutable facts; never write owner state |

No machine may be split across components. A state type has one owner, and every transition is triggered only after an input has
been converted to that owner's private internal event. A cross-owner request expresses intent or fact, never a target state
transition. `run.replacement` is a Run-owner SM-1 subordinate transaction, not a fifth machine.

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
- current-baseline Restart: eligible fresh bootstrap plus Run-owned predecessor `ReplacementIntent`; a Definition-gap intent
  first requires dedicated Decision acknowledgement. Orchestrator may persist/retry the exact `RuntimeTerminationRecord` SM-4
  handoff, but only a Run-private SM-1 delivery event makes the predecessor terminal; the Run-owned replacement transaction creates
  the successor only after ordered delivery acknowledgement;
- persisted data has one current shape and one repository layout; invalid/retired data fails at load and has no migration or projected disposition;
- repeated notification/Drive/acknowledgment cannot duplicate attempt, side effect, evidence, Result or successor.

## Stable Decisions

- one `go.mod`; directory modules are ownership boundaries, not separate Go modules;
- owner units expose EventHub contracts, not direct service facades;
- Runtime aggregate is typed, Orchestrator-owned semantically, and Run-owned only as opaque CAS storage;
- core Runtime remains profile/domain neutral;
- prospective owner-fact feasibility composes acquisition-owner delta and required-owner dependency observation through typed
  queries; one SM-3 semantic proposal carries 1–8 unique canonical exact target refs until SM-2 seals the owner-validated set,
  operation candidate references that binding, and the required operation sequence binds one exact-singleton target per ordered
  operation. The required owner publishes one provider-safe typed mandatory-`all_of` clause constraint that is sealed into
  acquisition/manifest/C3; every clause must be covered and every target must contribute, with no `one_of`/`any_of` or singular
  compatibility field. File applies whole-set hypothetical manifest admission with no side effect; Shell judges only the resulting
  manifest. Mismatch is handled by document admission/schema correction before any capability probe. Non-selectable dependency
  catalogs remain out of the LLM surface and no participant mutates a sibling owner. A complete
  `would_not_satisfy` is an owner-contract contradiction and directly closes
  `prospective_satisfaction_contract_violated -> semantic_contract_failed`; stale stays on fact refresh;
- removed StagePacket, NextAction, DispatchNextStep, ExecutionContextPatch, graph dispatcher and application recovery queues are
  not compatibility paths;
- LLM provider configuration/ceiling interpretation belongs to LLM owner; sealed selection cannot drift on Resume.

## Related Documents

- [Runtime Stage Contract](../00-core/runtime-stage-contract.md)
- [Component Contract Inventory](../01-overview/component-documentation-alignment.md)
- [Application Workorchd](application-workorchd.md)
- [Kernel Orchestrator / Governance](kernel-orchestrator-governance.md)
- [Block Boundaries](block-boundaries.md)
- [Repository Structure](repository-structure.md)
