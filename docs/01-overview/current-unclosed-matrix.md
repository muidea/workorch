# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-08-13

## Purpose

This matrix records only current work. Historical success, completed code slices, and dated live probes are archive
evidence; they do not close a later persisted aggregate counterexample.

## A. Runtime CWI Canonical Ledger And Reconciliation

Status: active

`run-073` through `run-082` showed that structural owner boundaries can remain correct while Orchestrator admission is
still incomplete. Canonical CWI identity, retirement, target policy, variant authorization, dependency admission and
proof-requirement authority were closed one counterexample at a time. The recent commit audit also confirmed one real
authority reversal: provider-declared artifact/metric requirements were introduced and later removed. The remaining
work therefore consolidates all dimensions into one Orchestrator-owned admission transaction rather than adding
another field-specific repair.

| Item | Status | Owner | Required closure |
| --- | --- | --- | --- |
| Prior CWI-SO slices | regression baseline only | Orchestrator + typed owner contracts | Canonical ledger, target, variant, dependency, request-bound and proof-authority work remains required regression coverage, but no prior field-level `code-closed` claim establishes complete admission closure. |
| CWI-SO-13 | code/automated-closed | Runtime mutation/admission inventory | One private-event/CAS mutation chain and mechanical admission/dispatch/provider-authority scans are in place. |
| CWI-SO-14 | code/automated-closed | Pure runtime contract | One detached complete admission input/outcome validates the full identity/dependency/target/variant/proof/binding/fence/lineage matrix. |
| CWI-SO-15 | code/automated-closed | Orchestrator SM-3 | Every production dispatch passes one owner-local dispatcher; candidate attempt acceptance and dispatch creation share one CAS. |
| CWI-SO-16 | code/automated-closed | Orchestrator SM-2/SM-3/SM-4 | Canonical retirement, reopen, receipt and evidence transitions pass restart/idempotency/effect fences. |
| CWI-SO-17 | code/automated-closed | Run/LLM/capability/governance/delivery owners | Cross-owner contracts remain detached typed facts; exact candidate-manifest identity is now mandatory on persisted owner feasibility. |
| CWI-SO-18 | automated-closed; deployed-live-pending | Automated/live verification | Full/race/boundary and managed file-backed gates pass. Deploy schema v5 and record fresh `/run restart` combined proof. |

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

Status: independent code baseline; deployed verification pending.

The remaining proof covers Web Start/Restart, TUI `/model reasoning`, Resume sealed selection, and two-workspace
isolation. It is independent of CWI/focus/dispatch authority. See the
[WRE plan](../70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md).

## C. Optional And Deferred Work

| ID | Status | Work |
| --- | --- | --- |
| OPT-1 | optional | One deployed file-patch validation/closeout proof. |
| CAND-LLM-BUDGET | deferred | Provider failover, tokenizer precision, native usage precision, and differentiated context-too-large handling. |

## Judgment

The unified code and automated admission matrix are closed through CWI-SO-17. CWI-SO-18 remains active only for a
fresh deployed schema-v5 `/run restart` combined proof; managed mock verification cannot substitute for persisted live
evidence.

## Navigation

- [Current state](current-state.md)
- [Unfinished todo index](unfinished-todo-index.md)
- [Active roadmap](../70-roadmap/active/overview.md)
- [Runtime CWI plan](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md)
- [Archive](../90-archive/completed/overview.md)
