# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-08-14

## Purpose

This document indexes only work that is currently open. Historical plans, dated live records, and prior code-closure
claims are archived and cannot override a current persisted aggregate counterexample.

## Runtime Status

| Topic | Current status |
| --- | --- |
| Production Runtime path | Runtime-only: typed activation, `DriveRuntime`, Orchestrator reducer, sealed dispatch, owner receipt, and terminal delivery. |
| Owner boundaries | Active baseline: cross-owner I/O uses concrete EventHub contracts; Run, Workspace, Result, capability Blocks, projection, and UI do not own each other's state. |
| CWI reconciliation | **Mandatory regression reference.** CWI-SO-13/14/16/18 matrices remain required; schema-v7 adds immutable declaration / generation binding separation after the latest live counterexample. |
| Model-capability output and state ownership | **Code/automated-closed; deployed verification pending.** SO-MC-12..17 close schema-v8 owner envelopes, unified File feasibility, exact binding retirement, pre-provider equivalent-authority fencing, opaque Run persistence and repository-wide gates. SO-MC-10 remains the fresh real-provider live proof. |
| Workspace reasoning effort | Code-closed, deployed verification pending. See [WRE plan](workspace-reasoning-effort-single-owner-closure-plan.md). |
| 22-component EventHub structure | Completed/archive baseline. It remains a boundary regression, not an active Runtime authority queue. |

## Mandatory Work

| ID | Work | Required result | Plan |
| --- | --- | --- | --- |
| CWI-SO | Mandatory Runtime admission regression matrix | Definition, presentation and execution order remain distinct; only an eligible CWI can become the unbound current focus; semantic order and seal use the same exact closure; settled CWI still require typed reopen. | [Runtime CWI regression plan](runtime-cwi-reconciliation-single-owner-closure-plan.md) |
| SO-MC | Runtime single-owner and model-capability output closure | SO-MC-12..17 code/automated closure is complete; verify a fresh schema-v8 real-provider candidate/dispatch/receipt/evidence terminal chain under SO-MC-10. | [SO-MC plan](runtime-single-owner-model-capability-output-closure-plan.md) |
| WRE | Workspace reasoning effort deployed verification | Web Start/Restart, TUI `/model reasoning`, Resume sealed selection, and workspace isolation verified against the deployed service. | [WRE plan](workspace-reasoning-effort-single-owner-closure-plan.md) |
| OPT-1 | Optional file-patch deployed validation | One `patch apply + validation + DoD closeout` live proof. It does not block CWI-SO. | [archive record](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

The prior CWI automated matrix remains mandatory regression coverage, but its schema-v5 deployed-only queue is
superseded by `SO-MC`. The next live proof must use the fresh schema-v8 path after SO-MC-12..17; a provider,
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
- [Completed archive](../../90-archive/completed/overview.md)
- [Superseded archive](../../90-archive/superseded/overview.md)
