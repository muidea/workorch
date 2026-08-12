# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-08-11

## Purpose

This matrix records only current work. Historical success, completed code slices, and dated live probes are archive
evidence; they do not close a later persisted aggregate counterexample.

## A. Runtime CWI Canonical Ledger And Reconciliation

Status: active

`run-073` showed that the production owner boundaries and terminal delivery can be structurally correct while the
Orchestrator still violates CWI canonicality internally. The Run produced two accepted File effects for `cwi-043`, then
failed when the reconciliation compiler finally observed duplicate coverage. Its current reconciliation request also
referenced an older binding set than the current sealed contract.

| Item | Status | Owner | Required closure |
| --- | --- | --- | --- |
| CWI-SO-1 | pending | Orchestrator SM-2 + pure runtime contract | Make the existing coverage rows the canonical CWI ledger and validate queue/binding/request uniqueness. |
| CWI-SO-2 | pending | Orchestrator SM-2 | Retire catalog generations without reintroducing settled CWI; reject duplicate binding before operation request. |
| CWI-SO-3 | pending | Orchestrator SM-2/SM-4 | Supersede old reconciliation generations and atomically synchronize reopen coverage with focus projection. |
| CWI-SO-4 | pending | Orchestrator SM-3 | Fence candidate, dispatch, receipt, and evidence by the canonical current binding. |
| CWI-SO-5 | code-closed | Orchestrator + Run opaque persistence | Legacy or ambiguous persisted aggregate is routed as a typed `fresh_activation_required` fact; no inferred migration, LLM/capability call, retry lease, or durable replay. Full and deployed matrix remains CWI-SO-6. |
| CWI-SO-6 | pending | Runtime verification | Pass focused/full/race/boundary/managed and fresh deployed invalidation/reopen/restart matrix. |

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
