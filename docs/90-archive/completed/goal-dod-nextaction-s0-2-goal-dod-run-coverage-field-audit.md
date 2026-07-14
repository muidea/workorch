# Goal/DoD NextAction S0.2 Goal/DoD Run Coverage Field Audit

Status: completed

Type: contract-audit

Last Updated: 2026-06-25

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [S2.1 Definition Revision Model](./goal-dod-nextaction-s2-1-definition-revision-model.md)
- [S2.2 Current Definition Revision Binding](./goal-dod-nextaction-s2-2-current-definition-revision-binding.md)
- [S2.3 Initial Gap Baseline](./goal-dod-nextaction-s2-3-initial-gap-baseline.md)
- [S2.4 Resume Gap Baseline](./goal-dod-nextaction-s2-4-resume-gap-baseline.md)
- [S2.5 Replan Definition Baseline](./goal-dod-nextaction-s2-5-replan-definition-baseline.md)
- [S2.6 Restart Replacement Baseline](./goal-dod-nextaction-s2-6-restart-replacement-baseline.md)
- [S4.1 Coverage Item Minimal Model](./goal-dod-nextaction-s4-1-coverage-item-minimal-model.md)
- [S4.2 Definition Revision Coverage Ledger Initialization](./goal-dod-nextaction-s4-2-definition-revision-coverage-ledger-initialization.md)
- [S4.6 Coverage Summary Projection](./goal-dod-nextaction-s4-6-coverage-summary-projection.md)

## Contract Audit

- Scope: Goal/DoD definition, run checkpoint fields, coverage work item fields, gap baseline, and projected coverage summary.
- Observed state: owner run state carries `definitionRevision`; activation/resume/replan/restart paths bind or rebuild gap baseline; coverage work items bind definition revision and required evidence kinds; projection exposes coverage summary.
- Expected state: every run can be traced to the current cumulative Goal/DoD definition revision, and every dispatch can explain the current gap without relying on historical free text.
- Deviation: S0.2 originally had no standalone field inventory document, so later slices contained the implementation evidence but the audit baseline was not independently discoverable.
- Owner: core model, workorchd appservice mapping, and projection read model.
- Event path: workspace definition selection -> activation/recovery context -> run owner `definitionRevision` -> coverage ledger -> gap baseline -> workspace projection.
- Contract gap: documentation gap only. Later S2 and S4 slices provide the implementation and tests.
- Proposed change: record this field inventory as the S0.2 audit baseline and link the completed implementation slices.
- Boundary check: all fields are neutral lifecycle and coverage metadata; no template, language, tool, or provider-specific rule is introduced.
- Verification: linked S2/S4 tests plus roadmap coverage audit.

## Field Inventory

| Area | Authoritative Fields | Verified By |
| --- | --- | --- |
| Definition binding | `definitionRevision`, `goalId`, `doDId`, cumulative `goals/dods` | S2.1, S2.2 |
| Initial run baseline | `gapBaseline`, `coverageWorkItemContext`, `todoItems`, `nextAction` | S2.3 |
| Resume baseline | refreshed `run.executionContext.gapBaseline`, dispatch gate action | S2.4 |
| Replan baseline | inherited `definitionRevision`, predecessor Goal/DoD IDs, `replanContinuation` | S2.5 |
| Restart baseline | replacement run latest definition revision, no stale predecessor baseline | S2.6 |
| Coverage ledger | coverage item id, definition revision, required evidence kinds, evidence refs | S4.1-S4.4 |
| Completion/projection | completion evidence refs, coverage summary, current focus, missing evidence | S4.5, S4.6 |

## Verification

- S2.1-S2.6 completed and preserve `definitionRevision` through run owner, appservice, client and port DTOs.
- S4.1-S4.6 completed and preserve coverage item state, missing evidence kinds, completion gate evidence refs and projection summary.
- `python3` roadmap coverage audit reports S0.2 as covered by this document.
