# Restart Context Budget Continuation Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-29

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-current-action-projection-convergence-closure-plan.md`

## Contract Audit

- Scope:
  - live workspace `ws-705249a8d7816330` / `run-009`
  - live workspace `ws-db040d24a190f6e5` / `run-008`
  - restart command -> workorchd scheduler -> runtime start -> LLM context budget gate
- Observed state:
  - restart did create or activate the latest runs: `ws-705249a8d7816330` has `activeRunId=run-009`; `ws-db040d24a190f6e5` has `activeRunId=run-008`.
  - both active runs stopped at step 1 with open context-integrity decisions.
  - decision reason is `protected_coverage_work_item_context_changed_after_context_budget`.
  - latest request context was under the configured provider budget, but `currentActionContract` triggered intentional current-action narrowing.
- Expected state:
  - restart should create a replacement run and dispatch the first runtime step.
  - current-action narrowing may remove duplicate historical context, but it must preserve the authoritative current coverage work item context used by the next action.
  - duplicate nested coverage projections must not override or invalidate the top-level coverage authority.
- Deviation:
  - the context budget integrity gate treats drift in duplicate nested coverage projections as a fail-fast context-integrity error even when the top-level current coverage work item context is preserved.
  - runtime therefore blocks on a decision immediately after restart instead of continuing to the LLM/provider call.
- Owner:
  - WorkOrch core LLM context budget and convergence contract authority.
  - restart scheduling itself is not the current blocker; it successfully produced the new active runs.
- Event path:
  - TUI/Web restart -> workorchd scheduler `TopicWorkorchDCommandWorkspaceRunRequested` -> workspace restart queue -> activation creates replacement run -> orchestrator start -> LLM provider request budget -> context gate -> context-integrity decision.
- Contract gap:
  - after the convergence authority simplification, `coverageWorkItemContext` top-level is the provider-facing authority.
  - nested copies under execution context packages are compatibility/audit mirrors and must not be independently treated as P0 authority when top-level context exists.
- Proposed change:
  - compare protected coverage context by top-level authority when present.
  - only fall back to nested execution/package/repair coverage comparisons when top-level coverage authority is absent.
  - add a regression test proving current-action narrowing does not fail-fast when top-level coverage context remains intact and nested mirrors are compacted differently.
- Boundary check:
  - change is core-neutral and only concerns generic context-budget authority.
  - no template ID, Soul ID, language, file name, provider, or workspace-specific logic is introduced.
- Verification:
  - targeted provider context budget tests.
  - boundary search for accidental template/language/provider special cases in the modified core file.

## Closure Tasks

- [x] Adjust provider context budget protected coverage equality to honor single top-level authority.
- [x] Add regression coverage for current-action narrowing with duplicate nested coverage mirrors.
- [x] Run targeted tests for LLM provider budget and context-integrity recovery.
- [ ] Re-run live restart verification after redeploy.
