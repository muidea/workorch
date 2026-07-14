# Runtime Diagnostic Lineage RequiredOutput Authority Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-deterministic-convergence-closure-plan.md`
- `runtime-contract-retry-convergence-closure-plan.md`
- `runtime-validation-evidence-context-closure-plan.md`

## Purpose

本文固化 2026-06-27 live verification 暴露的新收口项：runtime 已经能够生成 convergence authority、Stage Packet 和 requiredOutput，但 rejected diagnostic lineage、旧 LLM summary 或历史 progress 仍可能把错误的“下一步输出形态”带入 LLM 可见上下文，导致 LLM 按非权威合同输出，runtime 再连续拒绝并升级 decision。

本计划只处理业务中立的 runtime 收敛问题：

- `requiredOutput`、`allowedOutputShapes`、current focus 和 NextAction 必须来自当前 authority snapshot。
- rejected LLM output、旧 step summary、old repair package、old graph、old decision/replan 文本只能作为 diagnostic lineage。
- diagnostic lineage 可以说明 `what failed`，不能指导 `what to produce next`。
- 软件开发验证策略、测试发现策略、文件粒度和命令偏好必须由 workspaceTemplate/profile/Soul/policy 数据承载，core 只消费结构化策略。

当前阶段不考虑兼容性。若现有实现、测试或文档与本文冲突，应按本文逐项收口。

## Contract Audit

- Scope:
  - `ws-4e83810542ac4d65` / `run-007-replan-001`
  - `ws-7ced70e98ea50c43` / `run-007`
  - runtime contract retry、repair/follow-up、LLM C3/C4 rendering、requiredOutput authority、validation repair strategy。
- Observed state:
  - `ws-4e83810542ac4d65` REST projection 与 owner state 一致：workspace 为 `waiting_decision`，blocking ticket 为 `run-007-replan-001-contract-retry-001-of-planning-001-3-contract-violation-decision`。
  - `ws-4e83810542ac4d65` 当前权威 `requiredOutput.allowedOutputShapes` 要求 `executionGraph_with_declared_validationEvidence`、`executionGraph_with_managed_validation_session`、`satisfied_dodAssessment_with_concrete_evidenceRefs` 或真实 governance blocker。
  - 同一 run 的 rejected diagnostic / step summary 却提示“返回顶层 `shellCommands`”。LLM 后续按该提示返回 top-level `shellCommands`，虽然命令包含 `validationEvidence`，但不属于当前合同允许的输出形态，runtime 连续拒绝并升级 decision。
  - `ws-7ced70e98ea50c43` 已产生真实 state-changing outputs 与 accepted evidence，但在 `cwi-004` traceability validation 上反复猜测测试名。第一次验证无匹配测试，repair 后仍以关键字筛选 `main_test.go`，再次失败并继续 repair。
  - `ws-7ced70e98ea50c43` 的第一次 repair 还触发了 concrete validation contract violation：LLM 输出 validation graph 时有 shell 写入路径不满足 `validationScratchDir` 约束，runtime 正确拒绝并 bounded retry。
- Expected state:
  - Stage Packet、requiredOutput、current focus、NextAction 和 convergenceContract 必须来自同一次 convergence authority snapshot。
  - rejected diagnostic evidence 只能进入 C4 作为失败事实、违例规则、rejectedOutputRef、actualOutputShape、missingFields 和 failure surface。
  - LLM 可见的“下一步应返回什么形态”只能从当前 `requiredOutput` / `correctionContract.allowedOutputs` 派生。
  - 当 validation repair 需要测试发现时，template/profile/Soul 应以结构化策略要求先读取/枚举测试结构，再生成验证路径；找不到测试时应转为 state_change gap、managed validation、failed assessment 或 governed blocker，而不是继续猜测试名。
- Deviation:
  - rejected diagnostic / old step summary 仍能携带自然语言行动建议，并与当前 requiredOutput 形成冲突。
  - contract retry 的 correction prompt 没有充分禁止 diagnostic lineage 中的旧 output-shape 建议。
  - dispatch-time guard 能检查 focus-bearing 字段，但尚未检查 C3/C4 中互斥 output-shape advice。
  - 软件开发 validation strategy 缺少“测试发现优先、猜测失败转窄路径”的结构化约束。
- Owner:
  - WorkOrch core: diagnostic lineage isolation、requiredOutput rendering authority、dispatch-time output-shape conflict guard、contract retry correction rendering、decision eligibility。
  - workspaceTemplate/profile/Soul/policy: 软件开发测试发现、验证命令偏好、找不到目标测试时的 state_change/validation 策略。
  - TUI/Web: 只展示 projection 中的 waiting decision、authority diagnostics 和 blocking ticket，不推断业务状态。
- Event path:
  - LLM output -> contract gate -> rejected diagnostic evidence -> authority refresh -> Stage Packet / C3/C4 render -> provider dispatch -> contract gate -> accepted/rejected evidence -> ledger recompute -> projection.
- Contract gap:
  - 缺少 diagnostic action advice scrubber。
  - 缺少 requiredOutput-derived correction instruction renderer。
  - 缺少 C3/C4 output-shape conflict gate。
  - 缺少 template-bound validation discovery policy。
- Boundary check:
  - core 不得根据 workspace title、template ID、Soul ID、Go、`go test`、`main_test.go`、LSP bridge、某个文件名或 live workspace ID 分支。
  - core 只能理解结构化字段：output shape、evidence kind、validationEvidence mode、scratch boundary、coverage focus、diagnostic source、policy refs。
  - 软件开发测试发现和命令偏好必须以 workspaceTemplate/profile/Soul/policy 数据进入 run context，由 core 中立解释器消费。
- Verification:
  - 增加失败样本测试覆盖 top-level `shellCommands` advice 污染当前 `executionGraph` requiredOutput。
  - 增加 C3/C4 conflict gate 测试。
  - 增加 rejected diagnostic 不进入 high-value memory / accepted evidence / requiredOutput authority 的测试。
  - 增加 template-bound validation discovery policy 绑定与消费测试。

## Closure Scope

### D1: Diagnostic Lineage Action Advice Scrubber

Problem:

- rejected LLM output、old step summary 和 prior repair summary 可能包含“下一步应返回顶层 shellCommands”这类行动建议。
- 这些文本进入 C4 后会与当前 requiredOutput 冲突，并误导 LLM。

Target:

- Diagnostic lineage 只保留结构化失败事实：
  - `violatedRule`
  - `violationType`
  - `failureClass`
  - `failureSignature`
  - `actualOutputShape`
  - `expectedOutputShapes`
  - `missingFields`
  - `rejectedOutputRef`
  - `sourceStepId`
  - `failureSurface`
- 禁止把 rejected output summary 中的 next-step prose 作为 current action guidance。
- 若必须保留自然语言摘要，只能标记为 `diagnosticOnly=true`，并不得出现在 `C4.current-action.requirements`、`retryHint`、`requiredOutput` 或 `nextAction.summary`。

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_current_action*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_prompt_context*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_memory*.go`

Acceptance:

- A rejected output whose summary says “return top-level shellCommands” cannot appear as actionable current guidance when current requiredOutput allows only executionGraph.
- Rejected diagnostic remains available by `rejectedOutputRef`, but not as authority.
- Diagnostic-only summaries are excluded from high-value memory recall.

### D2: RequiredOutput-Derived Correction Renderer

Problem:

- Contract retry correction prompt may include generic or stale recovery hints that do not exactly match current `requiredOutput.allowedOutputShapes`.

Target:

- Retry instruction is generated from current authority requiredOutput only.
- If current requiredOutput allows `executionGraph_with_declared_validationEvidence`, rendered instruction must explicitly require `executionGraph.nodes[].input.validationEvidence`.
- If top-level `shellCommands` is not an allowed shape, rendered instruction must explicitly forbid top-level `shellCommands`.
- `retryHint` becomes derived from `correctionContract.allowedOutputs` and `requiredFields`, not from old LLM summary.

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation_recovery*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`

Acceptance:

- Contract retry input contains one authoritative output-shape instruction set.
- `requiredOutput.allowedOutputShapes` and natural language correction text cannot contradict each other.
- Tests cover executionGraph-only retry, managed validation retry, satisfied assessment retry and governance blocker retry.

### D3: Dispatch-Time Output Shape Conflict Gate

Problem:

- Existing dispatch guard focuses on current focus / requiredOutput binding, but live evidence shows C4 summaries can still contradict C3 requiredOutput.

Target:

- Before provider dispatch, scan current Stage Packet and C4 current-action payload for structured output-shape conflicts.
- If C3 requires executionGraph but C4 actionable guidance says top-level shellCommands, runtime must:
  - rebuild input after diagnostic scrub, or
  - fail fast as `runtime_internal_contract_misalignment`.
- This is not a provider failure and not a normal LLM contract violation.

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_specs.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`

Acceptance:

- Mismatch between `requiredOutput.allowedOutputShapes` and C4 actionable text is detected before LLM call.
- Internal mismatch emits authority diagnostic and does not consume LLM retry budget.
- Projection can expose the mismatch as read-only `authorityDiagnostics`.

### D4: Contract Retry Budget And Decision Eligibility Narrowing

Problem:

- Same-rule contract violations can escalate to decision even when the underlying blocker is core-rendered misleading context or an automatically narrowable validation route.

Target:

- Decision is allowed only when runtime lacks external information, credentials, permission, manual confirmation, user preference or governance approval.
- Same-rule retry exhaustion first checks:
  - Did current requiredOutput conflict with C4 guidance?
  - Was rejected diagnostic scrubbed?
  - Is there a deterministic route or managed validation route?
  - Is this a template validation discovery gap?
- If the issue is internal authority/context mismatch, open an internal failure/diagnostic rather than asking user for replan.

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_convergence_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_decision_eligibility*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`

Acceptance:

- A recoverable same-rule violation caused by conflicting output-shape guidance does not create user decision.
- Decision ticket payload includes `eligibilityReason` and proves no deterministic/runtime route exists.
- Replan remains a governance recovery path, not the default response to core-generated contradictory guidance.

### D5: Template-Bound Validation Discovery Policy

Problem:

- Software development validation currently lets LLM guess test names or command patterns. This causes repeated failed validations when target tests do not exist or naming differs.

Target:

- software-dev template/profile/Soul provides structured validation discovery policy:
  - first inspect or list existing tests/commands/capabilities;
  - only run discovered targets;
  - if no target exists, choose a declared route: create missing validation artifact via state_change, use controlled substitute, return failed assessment with concrete missing evidence, or request governance only for external blockers.
- Core consumes the policy generically as validation discovery/route metadata. Core must not hardcode Go, `go test`, `main_test.go`, test name regexes, LSP bridge or file names.

Code areas:

- `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/souls.json`
- template/profile policy binding code
- neutral runtime validation policy interpreter
- tests under orchestrator/appservice defaults

Acceptance:

- software-dev policy enters workspace binding and run executionContext.
- Core sees only structured policy fields such as `discoveryRequired`, `discoverySources`, `ifNoTargetRoute`, `validationScratchRequired`, `allowedValidationModes`.
- Live-style fixture with missing target tests does not loop on guessed test-name regex.

### D6: Projection Authority Diagnostics

Problem:

- When runtime blocks because of contract mismatch or diagnostic-lineage conflict, TUI/Web need a precise read-only explanation from workorchd projection.

Target:

- Projection exposes `authorityDiagnostics` for:
  - output-shape conflict
  - stale diagnostic action advice removed
  - internal authority mismatch
  - decision eligibility reason
  - validation discovery gap
- Projection must not repair owner state.

Code areas:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`
- TUI/Web display models

Acceptance:

- TUI/Web can show why a workspace is waiting decision or degraded without local inference.
- Projection detail and workspace list remain based on the same owner revision.

## Implementation Order

1. D1 Diagnostic Lineage Action Advice Scrubber.
2. D2 RequiredOutput-Derived Correction Renderer.
3. D3 Dispatch-Time Output Shape Conflict Gate.
4. D4 Contract Retry Budget And Decision Eligibility Narrowing.
5. D5 Template-Bound Validation Discovery Policy.
6. D6 Projection Authority Diagnostics.

## Regression Fixtures

### Fixture A: Output Shape Advice Conflict

Input:

- current requiredOutput allows `executionGraph_with_declared_validationEvidence`;
- rejected diagnostic summary says “return top-level shellCommands”;
- retry is about the same focus and same violation.

Expected:

- retry prompt forbids top-level `shellCommands`;
- C4 keeps rejected summary diagnostic-only;
- dispatch either rebuilds clean input or fails fast before provider call.

### Fixture B: Concrete Validation Graph Accepted

Input:

- LLM returns `executionGraph.nodes[].kind=shell/command`;
- node input includes `validationEvidence.mode` and `validationEvidence.observationType`;
- coverage focus and coverage work item IDs match current authority.

Expected:

- candidate passes contract gate;
- validation executes;
- accepted/rejected validation evidence updates ledger.

### Fixture C: Missing Validation Target Discovery

Input:

- validation focus needs evidence;
- template policy requires discovery;
- no target test/command/capability exists.

Expected:

- runtime does not repeatedly guess names;
- NextAction narrows to declared route from policy: state_change, controlled substitute, failed assessment, deferred, or governed blocker.

### Fixture D: Decision Eligibility

Input:

- repeated same-rule violation;
- no external blocker;
- deterministic route or internal mismatch exists.

Expected:

- no user decision ticket;
- runtime records internal authority diagnostic or derives a narrower NextAction.

## Verification Plan

- Run focused orchestrator tests covering D1-D4.
- Run appservice/defaults tests covering D5 policy binding.
- Run projection/ViewModel tests covering D6 diagnostics.
- Run boundary search:

```bash
rg -n "software-dev|go test|main_test|lsp-bridge|LSP|Trace|Fingerprint|RequestID|shellCommands" internal/modules/kernel/orchestrator -g '*.go'
rg -n "diagnosticOnly|requiredOutput|allowedOutputShapes|outputShape|authorityDiagnostics" internal/modules/kernel internal/modules/application/workorchd internal/pkg -g '*.go'
```

- Live verification must record:
  - workspaceId / runId / stepId;
  - current requiredOutput output shape;
  - rejected diagnostic refs;
  - rendered retry instruction;
  - accepted/rejected evidence refs;
  - final run/workspace state.

## Done Criteria

- No rejected LLM prose can override current requiredOutput.
- No top-level output shape is suggested unless current `allowedOutputShapes` permits it.
- Same-rule contract violation caused by internal authority mismatch does not escalate to user decision.
- Software development validation discovery policy is template-bound and consumed through neutral runtime fields.
- Projection exposes enough diagnostics for TUI/Web to show the real blocker without local business-state inference.
