# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-08-13

## Purpose

This matrix records only current work. Historical success, completed code slices, and dated live probes are archive
evidence; they do not close a later persisted aggregate counterexample.

## A. Runtime CWI Canonical Ledger And Reconciliation

Status: active

`run-073` showed that the production owner boundaries and terminal delivery can be structurally correct while the
Orchestrator still violates CWI canonicality internally. The Run produced two accepted File effects for `cwi-043`, then
failed when the reconciliation compiler finally observed duplicate coverage. Its current reconciliation request also
referenced an older binding set than the current sealed contract. CWI-SO-1 through CWI-SO-5 closed that ledger defect.
The later `run-078` exposed the remaining pre-binding gap: a semantic proposal may label an existing sealed-catalog
target as prospective and escape the only legal complete-document correction point.

| Item | Status | Owner | Required closure |
| --- | --- | --- | --- |
| CWI-SO-1 through CWI-SO-5 | code-closed | Orchestrator + pure runtime contract + Run opaque persistence | Canonical CWI ledger, generation/reconciliation fencing, dispatch/evidence fences, and current-schema fail-closed boundary are implemented. |
| CWI-SO-6 | code-closed | Orchestrator SM-3 + pure runtime contract | A prospective target already present in the same sealed owner catalog is rejected before owner probing and uses only the existing attempt-0/1 complete-document correction. |
| CWI-SO-7 | automated-verified; live-pending | Runtime verification | Focused/full/race/boundary and isolated managed mock-provider verification pass. Complete fresh positive convergence and deployed invalidation/reopen/restart/resume matrix. |

The detailed owner map, transition rules, migration boundary, and regression matrix are in the
[Runtime CWI plan](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md).

### Required Baselines

- SM-1 remains Run-owned; Workspace and Result remain owner-local terminal delivery consumers.
- SM-2, SM-3, and SM-4 remain Orchestrator-owned, with one private internal-event mutation dispatcher and aggregate
  CAS boundary.
- Capability Blocks return only scoped typed observations and receipts. They do not alter CWI, focus, binding,
  reconciliation, retry, or terminal state.
- `/run resume` retains only current-Run lineage; `/run restart` creates a new Run with no predecessor CWI credit.
- Non-current or ambiguous Runtime aggregate schema returns `fresh_activation_required`, with no compatibility fallback.

## B. Workspace Reasoning Effort

Status: code-closed, deployed verification pending.

The remaining proof covers Web Start/Restart, TUI `/model reasoning`, Resume sealed selection, and two-workspace
isolation. It is independent of CWI/focus/dispatch authority. See the
[WRE plan](../70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md).

## C. Optional And Deferred Work

| ID | Status | Work |
| --- | --- | --- |
| OPT-1 | optional | One deployed file-patch validation/closeout proof. |
| CAND-LLM-BUDGET | deferred | Provider failover, tokenizer precision, native usage precision, and differentiated context-too-large handling. |

## Judgment

The Runtime path is not fully closed until CWI-SO is complete. Passing `verify-boundary-contract`, focused tests, or
healthy projection confirms useful layers, but does not prove cross-generation CWI uniqueness or absence of duplicate
durable effects.

## Navigation

- [Current state](current-state.md)
- [Unfinished todo index](unfinished-todo-index.md)
- [Active roadmap](../70-roadmap/active/overview.md)
- [Runtime CWI plan](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md)
- [Archive](../90-archive/completed/overview.md)
