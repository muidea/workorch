# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-08-13

## Purpose

This document indexes only work that is currently open. Historical plans, dated live records, and prior code-closure
claims are archived and cannot override a current persisted aggregate counterexample.

## Runtime Status

| Topic | Current status |
| --- | --- |
| Production Runtime path | Runtime-only: typed activation, `DriveRuntime`, Orchestrator reducer, sealed dispatch, owner receipt, and terminal delivery. |
| Owner boundaries | Active baseline: cross-owner I/O uses concrete EventHub contracts; Run, Workspace, Result, capability Blocks, projection, and UI do not own each other's state. |
| CWI reconciliation | **CWI-SO-13/14/16/18 code and automated matrices are closed; deployed verification remains open.** `run-085` blocked-focus selection and `run-087` settled-order reuse now converge through one Orchestrator-owned eligible closure and typed semantic-generation retirement. Full/race/boundary and file-backed managed gates pass; a fresh deployed proof remains. |
| Workspace reasoning effort | Code-closed, deployed verification pending. See [WRE plan](workspace-reasoning-effort-single-owner-closure-plan.md). |
| 22-component EventHub structure | Completed/archive baseline. It remains a boundary regression, not an active Runtime authority queue. |

## Mandatory Work

| ID | Work | Required result | Plan |
| --- | --- | --- | --- |
| CWI-SO | Canonical CWI ledger, eligible-focus projection, dependency admission, and single-owner reconciliation closure | Definition, presentation and execution order remain distinct; only an eligible CWI can become the unbound current focus; semantic order and seal use the same exact closure; settled CWI still require typed reopen. | [Runtime CWI plan](runtime-cwi-reconciliation-single-owner-closure-plan.md) |
| WRE | Workspace reasoning effort deployed verification | Web Start/Restart, TUI `/model reasoning`, Resume sealed selection, and workspace isolation verified against the deployed service. | [WRE plan](workspace-reasoning-effort-single-owner-closure-plan.md) |
| OPT-1 | Optional file-patch deployed validation | One `patch apply + validation + DoD closeout` live proof. It does not block CWI-SO. | [archive record](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

`CWI-SO` is now at `FOCUS-07`: the eligible-focus projection, owner-private transition routing, settled-order narrowing
and dependency-unlock generation retirement are implemented and covered by the combined automated matrix. The remaining
work is a fresh deployed `/run restart` proof. A provider, capability owner, UI, projection or managed mock result cannot
substitute for Orchestrator-owned closure.

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
- [Completed archive](../../90-archive/completed/overview.md)
- [Superseded archive](../../90-archive/superseded/overview.md)
