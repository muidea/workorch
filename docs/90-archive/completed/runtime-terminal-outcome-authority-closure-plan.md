# Runtime Terminal Outcome Authority Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-28

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-live-convergence-gate-closure-plan.md`
- `runtime-evidence-proof-strength-and-assessment-closure-plan.md`
- `runtime-managed-validation-resource-lifecycle-closure-plan.md`

## Purpose

本文用于收口 runtime/readmodel 中“局部 accepted evidence 被投影为全局 terminal accepted outcome”的偏差。

长期设计要求是：

```text
Coverage/Evidence Ledger
  -> Deterministic NextAction
  -> Completion Gate
  -> Final Outcome Result
  -> Workspace/Run Outcome Projection
```

局部 `acceptance_slice` 只能表示某一组 coverage work item 已形成 accepted evidence。它不能替代 completion gate，不能把仍在 running 的 run 投影为 terminal accepted，也不能让 TUI/Web 误判 workspace 已完成。

本文只涉及通用 result/outcome/projection 语义，不引入 workspaceTemplate、Soul、编程语言、命令、provider、workspace title 或 live workspace id 特判。

## Contract Audit

- Scope:
  - `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
  - `internal/modules/kernel/orchestrator/biz/runtime_result_recording.go`
  - result progress、workspace outcome、run outcome、TUI/Web ViewModel consumption.
- Observed state:
  - live workspace `ws-7ced70e98ea50c43` 的 active run `run-009-replan-001` 仍为 `running`。
  - active step 仍在 `dispatched`。
  - coverage summary 仍有 `open=48`、`failed=1`。
  - snapshot outcome 却显示 `accepted=true`、`terminal=true`。
  - outcome result 指向 `run-009-replan-001/result/acceptance-slice/slice-remaining-acceptance-010`。
  - owner 写入该 result 时已有 `SourceType=acceptance_slice`，并在 diagnostics 中标注 `resultScope=acceptance_slice`、`finalCompletionGateState=not_final_result`。
- Expected state:
  - running run 的局部 accepted slice 只能投影为 accepted evidence/progress。
  - workspace/run outcome 在 completion gate 未完成且 coverage 仍 open/failed 时必须保持 non-terminal。
  - 只有 final outcome result 或 terminal run state 才能产生 terminal workspace outcome。
- Deviation:
  - readmodel 的 terminal result selection 只按 result state `accepted/rejected` 选择结果。
  - `applyRunOutcomeTerminalResult` 对任意 selected accepted result 设置 `Terminal=true`。
  - result progress helper 也可能把任意 accepted result 映射为 completed。
- Owner:
  - WorkOrch core readmodel/projection。
  - 不是 TUI/Web 问题；端侧只是展示 projection。
  - 不是 workspaceTemplate/Soul/policy 问题；owner 已提供结构化 result scope。
- Event path:
  - LLM step -> contract gate -> evidence/result recorder -> accepted `acceptance_slice` result -> readmodel projection -> workspace/run outcome -> REST/SSE -> TUI/Web.
- Contract gap:
  - 缺少通用 result scope 判定，无法区分 final outcome result 与 intermediate accepted evidence。
  - workspace/run outcome selection 没有绑定 completion gate、run terminal state 与 coverage terminal state。
  - projection 把局部 evidence result 提升成了全局 terminal outcome。
- Proposed change:
  - 在 readmodelprojection 增加通用 result outcome eligibility helper。
  - terminal outcome selection 只选择 final outcome eligible result。
  - intermediate accepted result 仍保留在 results/evidence/progress 中，但不得设置 terminal outcome。
  - 补充 readmodel 单测和 live verification checklist。
- Boundary check:
  - 使用结构化字段：`SourceType`、`SummaryDetail.Diagnostics.resultScope`、`finalCompletionGateState`、run state、coverage state。
  - 禁止通过 result title/summary 文本、template id、workspace id、命令输出或业务语义判断。
- Verification:
  - 增加失败样本单测：running run + accepted acceptance slice + open coverage => non-terminal outcome。
  - 增加正例单测：final accepted result + completed run + closed coverage => terminal accepted outcome。
  - 增加 rejected terminal result 正例：terminal failed/rejected run 仍能投影 failed outcome。
  - 边界搜索确认 core 未新增模板/语言/命令/provider 特判。

## Design Rules

### Result Scope

Readmodel 必须把 result 分为三类：

| Category | Meaning | Can satisfy evidence | Can produce terminal outcome |
| --- | --- | --- | --- |
| final outcome result | completion gate 产生的最终 DoD/Goal outcome | yes | yes |
| intermediate accepted evidence | acceptance slice、partial evidence bundle、progress evidence | yes | no |
| rejected diagnostic result | rejected assessment、contract violation diagnostic | no | only when selected terminal failed outcome for terminal failed run |

判定必须基于结构化字段：

- `Result.SourceType`
- `Result.SummaryDetail.Diagnostics.resultScope`
- `Result.SummaryDetail.Diagnostics.finalCompletionGateState`
- `Result.DoDAssessment.Satisfied`
- run state
- coverage summary / convergence status when available

禁止基于以下信息做判断：

- result summary/title 自然语言
- workspace title
- template id / soul id
- command name
- provider/model name
- live workspace/run id

### Terminal Outcome Eligibility

Accepted result 只有满足以下条件之一，才能成为 terminal accepted outcome：

1. result 是明确的 final outcome result，且 run 已 terminal completed/accepted。
2. completion gate 已确认所有 required coverage closed，且 result 不是 intermediate result。

Rejected result 只有满足以下条件，才能成为 terminal failed outcome：

1. run 已 terminal failed/rejected/cancelled。
2. result 表达 terminal failure 或 final rejected assessment。
3. result 不是 intermediate accepted evidence。

`acceptance_slice` 永远不能单独成为 terminal outcome，即使 state 是 `accepted`。

### Projection Behavior

Projection 可以展示 intermediate accepted evidence：

- `results[]`
- result quality
- coverage accepted evidence refs
- task progress milestones
- latest result timestamp

Projection 不得因为 intermediate accepted evidence 改写：

- `WorkspaceOutcome.Accepted`
- `WorkspaceOutcome.Terminal`
- `RunOutcome.Accepted`
- `RunOutcome.Terminal`
- TUI/Web 主视图的 final accepted result

如果 run 仍在 running 且存在 accepted slice，workspace outcome 应保持：

```text
state = running
rawWorkspaceState = running
accepted = false
terminal = false
```

### Progress State

Result progress helper 也必须区分 result scope：

- final accepted outcome result -> `completed`
- intermediate accepted evidence -> `in_progress` or existing non-terminal progress state
- rejected diagnostic under running run -> `in_progress` with diagnostic refs
- terminal rejected result under terminal failed run -> `failed`

不得因任意 accepted result 把 progress 显示为 completed。

## Closure Checklist

### RTO-01 Add Result Outcome Eligibility Helper

Target files:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/outcome_result_selection.go`
- optional new file: `result_outcome_eligibility.go`

Tasks:

- Add `isFinalOutcomeResult(result readmodelcore.Result) bool`.
- Add `isIntermediateAcceptedEvidence(result readmodelcore.Result) bool`.
- Add `isTerminalOutcomeEligibleResult(result readmodelcore.Result, run readmodelcore.Run) bool`.
- Treat `SourceType=acceptance_slice` and diagnostics `resultScope=acceptance_slice` as intermediate evidence.
- Treat diagnostics `finalCompletionGateState=not_final_result` as explicitly non-terminal.

Acceptance:

- No business/template/language/provider string is introduced.
- Helper behavior is covered by unit tests.

### RTO-02 Narrow Terminal Result Selection

Target files:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/outcome_result_selection.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/run_outcome.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_outcome_result_fallback.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_outcome_event_selection.go`

Tasks:

- Replace `latestTerminalWorkspaceResult` selection with final-outcome-aware selection.
- Ensure run outcome mapping receives the selected run state when checking eligibility.
- Prevent historical accepted slice from being selected as workspace outcome fallback.
- Keep terminal failed/rejected run outcome behavior intact.

Acceptance:

- Running run with accepted slice produces non-terminal outcome.
- Completed run with final accepted result still produces accepted terminal outcome.
- Failed terminal run with final rejected result still produces failed terminal outcome.

### RTO-03 Fix Result Progress Semantics

Target files:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/result_progress.go`
- any helper currently using `HasAcceptedRunResult` or `PrimaryTaskProgressResult`.

Tasks:

- Ensure `ResultProgressState` only returns `completed` for final accepted outcome.
- Preserve intermediate accepted evidence in progress details without terminal promotion.
- Review `HasAcceptedRunResult` callers and rename or narrow if it actually means final accepted outcome.

Acceptance:

- TUI/Web progress does not show completed for accepted slice while run is running.
- Existing final accepted result tests continue to pass.

### RTO-04 Add Readmodel Regression Tests

Target files:

- `internal/modules/application/workorchd/biz/appservice/read_model_service_test.go`
- optional focused projection test file.

Required tests:

1. `TestMapWorkspaceOutcomeIgnoresAcceptedSliceForRunningRun`
   - workspace state `running`
   - active run state `running`
   - result state `accepted`
   - `SourceType=acceptance_slice`
   - diagnostics `finalCompletionGateState=not_final_result`
   - expected: outcome non-terminal, accepted false.

2. `TestMapRunOutcomeIgnoresAcceptedSliceForRunningRun`
   - same condition at run snapshot level.
   - expected: run outcome non-terminal.

3. `TestResultProgressDoesNotCompleteFromAcceptedSlice`
   - accepted slice exists.
   - no final outcome result.
   - expected: progress not completed.

4. `TestFinalAcceptedResultStillProducesTerminalOutcome`
   - final accepted result and completed run.
   - expected: accepted terminal outcome.

5. `TestTerminalRejectedResultStillProducesFailedOutcome`
   - final rejected result and failed run.
   - expected: failed terminal outcome.

### RTO-05 Live Verification

After deployment, verify:

```bash
curl -sS http://127.0.0.1:8080/api/v1/workorch/workspaces/<workspaceId>/snapshot | jq '.snapshot | {run:.run, step:.step, coverage:.coverageSummary.summary, outcome:.outcome, resultsCount:(.results|length)}'
```

Expected for running run with open coverage:

- `run.state == "running"`
- `coverage.open > 0` or `coverage.failed > 0`
- `outcome.terminal == false`
- `outcome.accepted == false`
- latest accepted slice remains visible in `results[]`

Expected for completed run:

- `run.state` is terminal
- coverage has no blocking open/failed items
- final outcome result is selected
- `outcome.terminal == true`
- `outcome.accepted == true` when final accepted

### RTO-06 Resource Lifecycle Follow-up

This is not part of terminal outcome semantics, but current live observation also showed large validation scratch data and stale validation child processes.

Follow-up should remain scoped to:

- `runtime-managed-validation-resource-lifecycle-closure-plan.md`

Rules:

- Do not solve resource lifecycle by changing outcome projection.
- Do not hide stale resource state with terminal outcome.
- Cleanup must be based on managed validation resource owner records and idempotent reconciliation.

## Out of Scope

- Changing workspaceTemplate default DoD.
- Changing software development prompt wording.
- Adding command/provider/language-specific core rules.
- Treating any current live workspace id as special.
- Killing arbitrary processes by name as part of outcome projection.

## Implementation Order

1. Add result outcome eligibility helper and focused tests.
2. Narrow `latestTerminalWorkspaceResult` and workspace fallback selection.
3. Update run/workspace outcome mapping to use eligibility.
4. Update result progress helpers.
5. Run targeted readmodel tests.
6. Run boundary search.
7. Deploy and perform live verification.

## Boundary Search

Before closing this plan, run:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|provider|model|workspace title|ws-7ced|ws-4e838" internal/modules/application/workorchd/biz/appservice/readmodelprojection internal/modules/kernel/orchestrator/biz -g "*.go"
rg -n "acceptance_slice|finalCompletionGateState|not_final_result|SourceTypeAcceptanceSlice" internal/modules/application/workorchd/biz/appservice/readmodelprojection internal/modules/kernel/orchestrator/biz -g "*.go"
```

The first search must not show new business-specific branches. The second search should show only structured scope handling and tests.

