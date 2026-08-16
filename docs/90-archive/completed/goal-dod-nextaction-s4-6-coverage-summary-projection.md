# S4.6 Coverage Summary Projection

Status: completed

Type: roadmap-slice

Owner: workorchd/readmodelprojection

Related:

- [Goal/DoD NextAction Iterative Delivery Plan](./goal-dod-nextaction-iterative-delivery-plan.md)
- [Goal/DoD NextAction Delivery State Machine Design](../superseded/goal-dod-nextaction-delivery-state-machine-design.md)
- [MVVP Workspace ViewModel Architecture](../superseded/mvvp-workspace-viewmodel-architecture.md)

## Contract Audit

- Scope: `readmodelprojection.CoverageSummaryFromRunSnapshot` and workspace snapshot read model.
- Observed state: coverage ledger items are already available on `RunSnapshot.CoverageWorkItems`, while `CoverageSummary` only copied selected `coverageWorkItemContext` fields.
- Expected state: workorchd projection should expose a compact UI-facing summary with `open`, `satisfied`, and `blocked` coverage counts without embedding the full coverage ledger.
- Deviation: a run with authoritative coverage work items but no context summary had no `CoverageSummary`; UI consumers could not render coverage status from the projection alone.
- Owner: workorchd/readmodelprojection. The core coverage ledger remains authoritative; projection only derives display data.
- Event path: orchestrator coverage ledger -> run snapshot `CoverageWorkItems` -> workspace snapshot `CoverageSummary` -> port/client/TUI/Web display.
- Contract gap: projection lacked deterministic aggregation of coverage item status counts.
- Proposed change: derive `total`, `open`, `satisfied`, `blocked`, and explicit closed/diagnostic status counts from `CoverageWorkItems`; keep existing context fields and never include `coverageWorkItems` in the summary payload.
- Boundary check: status aggregation is business-neutral and uses the runtime coverage status contract; no template, language, provider, command, or workspace-specific branching is introduced.
- Verification: projection unit coverage through appservice snapshot test, direct readmodelprojection tests, and core boundary search.

## Design

`CoverageSummary` is a compact ViewModel field. It is not the owner of coverage state and must not carry the full ledger.

The projection rules are:

- `satisfied` counts items whose status is `satisfied`.
- `blocked` counts items whose status is `blocked`.
- `open` counts active unresolved items, including empty or unknown active statuses.
- `skipped` and `superseded` are reported when present, but are not counted as `open`.
- `total` counts all non-empty coverage items seen by the projection.

Existing context fields remain supported:

- `summary`
- `currentFocus`
- `omittedCount`
- `fullLedgerRef`

## Acceptance

- A workspace snapshot with active coverage items exposes `coverageSummary.open`, `coverageSummary.satisfied`, and `coverageSummary.blocked`.
- A run with coverage items but no `coverageWorkItemContext` still has a coverage summary.
- The coverage summary does not include the full `coverageWorkItems` ledger.
- The change is limited to read model projection and does not alter orchestrator coverage state.

## Verification

- `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice -run TestReadWorkspaceSnapshotProjectsCoverageSummaryFromActiveRun -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice -count=1`
- Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|DeepSeek|OpenAI|Anthropic|mcp-lsp-bridge" \
  internal/modules/application/workorchd/biz/appservice/readmodelprojection/coverage_summary.go \
  internal/modules/application/workorchd/biz/appservice/read_model_service_test.go \
  docs/70-roadmap/active/goal-dod-nextaction-s4-6-coverage-summary-projection.md \
  docs/70-roadmap/active/goal-dod-nextaction-iterative-delivery-plan.md
```
