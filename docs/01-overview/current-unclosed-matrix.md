# WorkOrch Current Unclosed Matrix

Status: active

Type: overview

Last Updated: 2026-08-14

## Purpose

This matrix records only current work. Historical success, completed code slices, and dated live probes are archive
evidence; they do not close a later persisted aggregate counterexample.

## A. Runtime CWI Canonical Ledger And Reconciliation

Status: regression baseline

`run-073` through `run-085` showed that structural owner boundaries can remain correct while Orchestrator admission is
still incomplete. Canonical CWI identity, retirement, target policy, variant authorization, dependency admission and
proof-requirement authority were closed one counterexample at a time. The recent commit audit also confirmed one real
authority reversal: provider-declared artifact/metric requirements were introduced and later removed. The remaining
work therefore consolidates all dimensions into one Orchestrator-owned admission transaction rather than adding
another field-specific repair.

Fresh `run-085` accepted exact durable evidence for `cwi-043`, then advanced the unbound current focus to blocked
`cwi-048` while semantic planning correctly selected eligible `cwi-044/045/046`. This reopens focus selection and
complete closure identity without reopening cross-owner ownership: the repair belongs only to Orchestrator SM-2/SM-3
private transitions and pure Runtime contract validation.

| Item | Status | Owner | Required closure |
| --- | --- | --- | --- |
| Prior CWI-SO slices | regression baseline only | Orchestrator + typed owner contracts | Canonical ledger, target, variant, dependency, request-bound and proof-authority work remains required regression coverage, but no prior field-level `code-closed` claim establishes complete admission closure. |
| CWI-SO-13 | implementation/focused-closed | Runtime mutation/admission inventory | One Orchestrator-private eligible-focus reducer owns unbound execution cursor selection; settled order predecessors are audit-only. |
| CWI-SO-14 | implementation/focused-closed | Pure runtime contract | Current focus, exact eligible closure, semantic order/batch and seal identity share one closure; dependency unlock retires stale generation authority. |
| CWI-SO-15 | code/automated-closed | Orchestrator SM-3 | Every production dispatch passes one owner-local dispatcher; candidate attempt acceptance and dispatch creation share one CAS. |
| CWI-SO-16 | implementation/focused-closed | Orchestrator SM-2/SM-3/SM-4 | Settle, prerequisite, reopen, retirement, reconciliation and semantic-order projection remain owner-private and CAS-bound. |
| CWI-SO-17 | code/automated-closed | Run/LLM/capability/governance/delivery owners | Cross-owner contracts remain detached typed facts; Decision/Approval transitions and File effects now enter only owner-private dispatchers. `file/write` is create-only and replacement requires revision-bound `patch/apply`. |
| CWI-SO-18 | code/automated-closed; live exit superseded by SO-MC | Automated/live verification | `run-086` target drift remains frozen as a pre-dispatch manifest rejection and File owner independently refuses overwrite. Re-run these shapes inside the fresh schema-v8 SO-MC live proof. |

The detailed owner map, transition rules, migration boundary, and regression matrix are in the
[Runtime CWI plan](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md).

The former schema-v5 deployed-only exit is superseded by the active
[Runtime single-owner and model-capability output closure plan](../70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md).
The CWI matrix remains mandatory regression coverage for that queue.

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

## C. Runtime Single-Owner And Model-Capability Output

Status: code/automated-closed; deployed schema-v8 verification pending.

The latest deployed Run correctly preserved one Orchestrator request session, stable authority and zero side effect,
but three operations calls reached the WorkOrch-sealed 8192-token limit and ended before CandidateAttempt/Dispatch.
The required closure removes configuration/activation token authority, upgrades fresh Runtime to schema v8, obtains
the effective output limit only from the selected model's typed ProviderCeiling, seals it into ExpectedDocument, adds
compact complete-JSON prompt/schema limits, and completes target-state removal for all owner-private lifecycle inputs.

| Item | Status | Owner | Required closure |
| --- | --- | --- | --- |
| SO-MC-01..03 | code/focused-closed | Runtime contract + config/Application | Interrupted mode work removed; configured stage token authority is deleted. |
| SO-MC-04..07 | code/focused-closed | LLM owner + Orchestrator SM-3/SM-4 | Typed model ceiling, exact ExpectedDocument equality, compact bounded schema/prompt and whole-document recovery are covered. |
| SO-MC-08 | code/focused-closed | Approval + all state owners | Approval uses observed fact kinds; owner-private dispatcher and zero-side-effect regressions pass. |
| SO-MC-09 | automated-closed | repository-wide | Core docs and narrow boundary gates are updated; fmt/vet/full/race/boundary/isolated managed file-backed/build gates pass. |
| SO-MC-10 | open; deployed-only | deployed Runtime | Requires a real-provider fresh schema-v8 candidate/dispatch/receipt/evidence/delivery terminal proof. |
| SO-MC-11 | code/automated-closed | Runtime contract + Orchestrator SM-2/3/4 | Schema v7 persists immutable DefinitionFocus declarations; prior generation operation kinds cannot constrain resource-change reseal; v6 Resume is side-effect free. |
| SO-MC-12 | code/automated-closed | Runtime contract + event contract | Schema v8 owner-policy, authority-input and opaque lifecycle-fence identities; v7 Resume is side-effect free. |
| SO-MC-13 | code/automated-closed | File owner | Semantic and operation feasibility share one kind/variant/target/resource policy. |
| SO-MC-14 | code/automated-closed | Orchestrator SM-2 | Contradictory owner fact retires the exact binding and triggers bounded semantic replan or typed terminal. |
| SO-MC-15 | code/automated-closed | Orchestrator SM-2/3/4 | Unchanged binding/revision/signature/targets cannot mint another AuthoritySnapshot or provider request. |
| SO-MC-16 | code/automated-closed | Run owner + Orchestrator persistence adapter | Run owns SM-1 and opaque CAS but does not decode SM-2/3/4 to select lifecycle transitions. |
| SO-MC-17 | code/automated-closed | repository-wide | Boundary scanner plus transition/restart/stale/duplicate/budget gates prove one owner-private mutation entry per machine. |

## D. Optional And Deferred Work

| ID | Status | Work |
| --- | --- | --- |
| OPT-1 | optional | One deployed file-patch validation/closeout proof. |
| CAND-LLM-BUDGET | deferred | Provider failover, tokenizer precision, native usage precision, and differentiated context-too-large handling. |

## Judgment

The fresh schema-v7 persisted counterexample remains a regression baseline. SO-MC-12..17 now close its code and
automated owner/convergence defects; live closure still requires one fresh schema-v8
candidate/dispatch/receipt/evidence/terminal chain. Managed mock verification, aggregate revision growth and correct
per-attempt fail-closed behavior cannot substitute for bounded forward convergence.

## Navigation

- [Current state](current-state.md)
- [Unfinished todo index](unfinished-todo-index.md)
- [Active roadmap](../70-roadmap/active/overview.md)
- [Runtime CWI plan](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md)
- [Runtime single-owner and model-capability output plan](../70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md)
- [Archive](../90-archive/completed/overview.md)
