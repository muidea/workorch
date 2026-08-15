# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-08-15

## Purpose

This document indexes only work that is currently open. Historical plans, dated live records, and prior code-closure
claims are archived and cannot override a current persisted aggregate counterexample.

## Runtime Status

| Topic | Current status |
| --- | --- |
| Production Runtime path | Runtime-only: typed activation, `DriveRuntime`, Orchestrator reducer, sealed dispatch, owner receipt, and terminal delivery. |
| Owner boundaries | Active baseline: cross-owner I/O uses concrete EventHub contracts; Run, Workspace, Result, capability Blocks, projection, and UI do not own each other's state. |
| CWI reconciliation | **Mandatory regression reference.** CWI-SO-13/14/16/18 matrices remain required; the development baseline separates immutable declaration from generation binding. |
| Model-capability output and state ownership | **Code/automated-closed; deployed verification pending.** SO-MC-12..18 close the current-baseline owner envelopes, unified File feasibility/catalog, target-bound semantic replacement, exact binding retirement, pre-provider equivalent-authority fencing, opaque Run persistence and repository-wide gates. SO-MC-10 remains the fresh real-provider live proof. |
| Orchestrator internal cohesion | **O0-O8 code/automated-closed; deployed verification remains open.** The private event union, one mutation/CAS kernel, five stateful private components, one pure CandidateEvaluator, stateless rule projections, owner-scoped transition files, independent SM-4 recovery-budget handoff and malformed recovery-envelope fence are covered by structure/restart tests and repository gates. Fresh SO-MC-10 live proof is still required; the 22 framework units remain unchanged. |
| Workorchd internal cohesion | **WD0-WD8 code/automated-closed; deployed verification remains open.** The single Application Module has a typed-only production server boundary, one Biz-owned runtime/session component, one projection manager/store/lifecycle, six typed read-model facades with an exact eight-symbol package export whitelist, semantic projection filenames, and merged `definitionquality`/`content`/`setupdraft` packages. Server-local Session/cache authority/projection lifecycle and forwarding compatibility paths are removed; deployed HTTP/SSE/CLI regression remains without moving formal owner state into Application. |
| Workspace reasoning effort | Code-closed, deployed verification pending. See [WRE plan](workspace-reasoning-effort-single-owner-closure-plan.md). |
| 22-component EventHub structure | The cross-owner EventHub baseline remains completed; OIC/WIC are active internal-cohesion work and do not add a Runtime authority queue or framework unit. |

## Mandatory Work

| ID | Work | Required result | Plan |
| --- | --- | --- | --- |
| CWI-SO | Mandatory Runtime admission regression matrix | Definition, presentation and execution order remain distinct; only an eligible CWI can become the unbound current focus; semantic order and seal use the same exact closure; settled CWI still require typed reopen. | [Runtime CWI regression plan](runtime-cwi-reconciliation-single-owner-closure-plan.md) |
| SO-MC | Runtime single-owner and model-capability output closure | SO-MC-12..18 code/automated closure is complete; after deploying the target-bound catalog fix, verify a fresh current-baseline real-provider candidate/dispatch/receipt/evidence terminal chain under SO-MC-10. | [SO-MC plan](runtime-single-owner-model-capability-output-closure-plan.md) |
| OIC | Orchestrator internal cohesion closure | Complete O0-O8 without adding an Owner, state machine, aggregate, mutation entry or CAS boundary; prove existing-target semantic replan converges through typed owner facts and bounded candidate admission. | [single-owner code-closure runbook](runtime-single-owner-code-closure-runbook.md#4-orchestrator-内部功能内聚专项) |
| WIC | Workorchd internal cohesion closure | Preserve the completed WD0-WD8 typed boundaries and exact read-model export whitelist; finish deployed HTTP/SSE/CLI regression without adding a Module/Block or application authority. | [single-owner code-closure runbook](runtime-single-owner-code-closure-runbook.md#5-workorchd-application-module-功能内聚专项) |
| WRE | Workspace reasoning effort deployed verification | Web Start/Restart, TUI `/model reasoning`, Resume sealed selection, and workspace isolation verified against the deployed service. | [WRE plan](workspace-reasoning-effort-single-owner-closure-plan.md) |
| OPT-1 | Optional file-patch deployed validation | One `patch apply + validation + DoD closeout` live proof. It does not block CWI-SO. | [archive record](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

The prior CWI automated matrix remains mandatory regression coverage, but its historical deployed-only queue is
superseded by `SO-MC`. The next live proof must use a fresh current development baseline path after SO-MC-12..18 and data
cleanup; a provider,
capability owner, UI, projection or managed mock result cannot substitute for persisted Orchestrator-owned closure.

## Active Documentation Rules

1. Runtime authority is defined by `00-core`, the Orchestrator readme, and typed runtime contracts; active plans only
   describe remaining implementation work.
2. A current persisted aggregate or deployed counterexample overrides a prior local test or historical code-closed
   statement.
3. Completed, superseded, and process-only plans belong in `90-archive/`; they are searchable but not implementation
   authority.
4. Active documents must not retain a parallel Runtime state machine, global authority registry, V1 fallback, or dated
   live-log narrative.
5. The CWI chain remains `Definition CWI -> SM-2 sealed binding -> SM-3 sealed operation -> owner receipt -> SM-2
   evidence/credit`; a raw candidate claim, provider summary, file existence, Git revision, or terminal result never
   substitutes for it.

## Navigation

- [Current state](../../01-overview/current-state.md)
- [Current unclosed matrix](../../01-overview/current-unclosed-matrix.md)
- [Unfinished todo index](../../01-overview/unfinished-todo-index.md)
- [Runtime CWI plan](runtime-cwi-reconciliation-single-owner-closure-plan.md)
- [Runtime single-owner and model-capability output plan](runtime-single-owner-model-capability-output-closure-plan.md)
- [Runtime single-owner code-closure runbook](runtime-single-owner-code-closure-runbook.md)
- [Runtime 22-component function and complexity inventory](runtime-22-component-function-complexity-inventory.md)
- [Completed archive](../../90-archive/completed/overview.md)
- [Superseded archive](../../90-archive/superseded/overview.md)
