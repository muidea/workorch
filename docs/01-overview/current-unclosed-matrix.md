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
The later `run-078` exposed a pre-binding gap: a semantic proposal could label an existing sealed-catalog target as
prospective and escape the only legal complete-document correction point. `run-081` verified that closure and the
`run-080` Git variant/current-run retention closure, then exposed a separate dependency-admission defect: validation
CWI have no preparation prerequisites and the final Git snapshot is ordered before validation.

| Item | Status | Owner | Required closure |
| --- | --- | --- | --- |
| CWI-SO-1 through CWI-SO-5 | code-closed | Orchestrator + pure runtime contract + Run opaque persistence | Canonical CWI ledger, generation/reconciliation fencing, dispatch/evidence fences, and current-schema fail-closed boundary are implemented. |
| CWI-SO-6 | code-closed | Orchestrator SM-3 + pure runtime contract | A prospective target already present in the same sealed owner catalog is rejected before owner probing and uses only the existing attempt-0/1 complete-document correction. |
| CWI-SO-7 | automated-verified; live-superseded-by-run-081 | Runtime verification | `run-081` verified prior CWI binding/variant behavior and provided the next persisted counterexample. |
| CWI-SO-8 | code-closed; automated-verified | Pure runtime contract | Prerequisite admission and evidence-triggered invalidation/reopen are separate, validated, hash-stable relations with no second graph owner. |
| CWI-SO-9 | code-closed; automated-verified | Template/profile + Activation compiler | The software-development profile compiles validation preparation, validation, report, and final snapshot into stable prerequisite/reopen edges; neutral templates do not inherit them. |
| CWI-SO-10 | code-closed; automated-verified | Orchestrator SM-2 | Planning eligibility requires every prerequisite to be settled, satisfied, and currently bound before provider request, feasibility, binding, or dispatch. |
| CWI-SO-11 | code-closed; automated-verified | Runtime request projection + LLM transport | C0-C2 remain complete/stable; coverage-order and proof-batch C3-C5 project only current eligible CWI slices and protected refs. |
| CWI-SO-12 | automated-verified; deployed-live-pending | Automated/live verification | Focused/full/race/boundary and managed file-backed verification pass; fresh deployed `/run restart` persisted proof remains open. |

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

The Runtime code path and automated matrix are closed through CWI-SO-11. CWI-SO remains active only because the
deployed fresh-workspace evidence in CWI-SO-12 has not yet been recorded; managed mock verification cannot substitute
for that persisted live proof.

## Navigation

- [Current state](current-state.md)
- [Unfinished todo index](unfinished-todo-index.md)
- [Active roadmap](../70-roadmap/active/overview.md)
- [Runtime CWI plan](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md)
- [Archive](../90-archive/completed/overview.md)
