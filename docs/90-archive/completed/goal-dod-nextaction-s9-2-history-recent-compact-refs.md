# S9.2 History Recent Compact Refs

Status: completed

Owner: core

Roadmap: [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)

## Contract Audit

- Scope: `processEvidenceTimelineFromRunSteps` and LLM-visible `processEvidenceTimeline`.
- Observed state: current run process evidence already uses `ListByRunSummary` and caps timeline items, but when the candidate count exceeds the LLM context item budget it selects bounded detailed steps without an explicit historical compact summary. Old history can therefore either occupy detailed slots or disappear without compact refs.
- Expected state: LLM-visible history should preserve recent complete evidence in order, represent older history as compact metadata with source refs, and avoid full historical payload injection.
- Deviation: the timeline boundary is a generic item cap, not an explicit "recent complete + compact historical refs" contract.
- Owner: core.
- Event path: run steps -> process evidence timeline -> current progress digest -> LLM execution context package -> provider prompt.
- Contract gap: no first-class compact historical summary item that tells the LLM older evidence exists but is reference-only.
- Proposed change: when process evidence candidates exceed the context item budget, emit one compact historical summary item for older candidates and keep the most recent complete window as detailed timeline items.
- Boundary check: no template ID, Soul ID, programming language, command output marker, or software-development-specific policy is introduced. The summary is based on generic step state/kind/stage/source refs.
- Verification: orchestrator unit tests for bounded timeline shape, historical refs, recent ordering, digest based on latest recent evidence, plus boundary search.

## Design Notes

- Total timeline item count stays bounded by `maxLLMContextSliceItems`.
- The compact history item is `historicalOnly=true` and `summaryKind=compact_history`.
- The compact history item carries counts and refs, not raw step output.
- Recent detailed items keep repository order after the summary item.
- Current progress digest continues to be driven by the most recent detailed evidence window.

## Acceptance

- A long run timeline includes exactly one compact historical summary plus recent detailed evidence.
- Historical summary contains source refs and latest historical failure metadata.
- Recent evidence order remains stable.
- No raw historical output payload is injected through the summary.

## Verification

- `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(ProcessEvidenceTimeline|RunExecutionContextPackageIncludesProcessEvidenceTimeline|CompactProcessEvidenceTimeline)' -count=1`
- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `git diff --check`
- Boundary search: `rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel/orchestrator/biz/runtime_graph.go internal/modules/kernel/orchestrator/biz/runtime_llm_context.go internal/modules/kernel/orchestrator/biz/execution_context_package_test.go`
