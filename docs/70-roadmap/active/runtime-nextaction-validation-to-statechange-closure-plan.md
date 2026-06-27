# Runtime NextAction Validation To StateChange Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-27

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-deterministic-convergence-closure-plan.md`
- `runtime-validation-evidence-context-closure-plan.md`
- `runtime-statechanging-evidence-ledger-closure-plan.md`
- `runtime-diagnostic-lineage-required-output-authority-closure-plan.md`

## Purpose

本文用于收口最新 live verification 暴露出的核心偏差：WorkOrch runtime 能够继续调度、投影和记录 rejected diagnostics，但在验证失败已经证明真实交付缺口或 LLM 多次无法产出合规验证图时，仍可能停留在 `validate` / `repair validation` 循环，不能足够确定地切换到 `state_change`、`managed_validation`、治理阻塞或失败。

本计划只处理 WorkOrch core 的业务中立收敛能力与 software-dev template/profile 的结构化策略补充，不把具体 workspace、语言、文件名、命令、provider 或业务文本写入 core 判定。

## Contract Audit

- Scope:
  - `ws-4e83810542ac4d65` / `run-007-replan-001-replan-001`
  - `ws-7ced70e98ea50c43` / `run-007-replan-001`
  - Runtime convergence authority, NextAction interpreter, validation failure diagnostics, contract retry budget, dispatch input authority alignment, software-dev template policy.
- Observed state:
  - `ws-4e83810542ac4d65` 当前 focus 为 `cwi-022`，NextAction 为 `validate`，缺少 validation evidence。
  - 同一 focus 连续出现 `concrete_validation_contract_missing`，runtime 正确将 LLM 输出记为 rejected diagnostic，并继续 `retry_same_contract`。
  - `ws-7ced70e98ea50c43` 当前 focus 为 `cwi-008`，验证失败包含 `missing-config-key:port`。进一步核对交付物后，`config.example.yaml` 有样例配置，但 `main.go` 只读取 `PORT` / `UPSTREAM_BASE_URL`，尚未实现完整 `config.yaml` / provider 配置加载。
  - step input 中仍可观察到 focus-bearing 字段并存风险，例如 active acceptance slice 仍携带旧 focus，而 top-level `coverageFocusId` 指向当前 focus。
  - Projection ready，workspace list / promptMode 为 `running`，EventHub 与 projection 链路没有显示丢事件或展示滞后。
- Expected state:
  - `Coverage/Evidence Ledger -> Current Focus -> Deterministic NextAction -> Stage Packet -> Candidate Gate -> Evidence Ledger` 是唯一收敛闭环。
  - `validation` 失败若结构化证明真实交付缺口，应由 runtime 确定性切换到 `state_change`，而不是继续只修验证。
  - 同一 focus 上重复合同违例达到预算后，应升级为 `managed_validation`、`request_decision` 或 `fail`，不能无限重试。
  - 所有 focus-bearing derived fields 必须来自同一次 authority snapshot；旧 lineage 只能作为 diagnostics。
- Deviation:
  - Runtime 对 validation failure 的分类粒度不足，无法稳定区分验证环境问题、验证合同违例、真实实现缺口和外部依赖缺失。
  - Contract retry 仍可能持续依赖 LLM 修正同一验证图，而没有确定性升级路径。
  - Dispatch input authority alignment 仍需加强，防止旧 `acceptance.currentFocus`、旧 slice budget 或旧 requiredOutput 参与当前动作。
- Owner:
  - Primary: WorkOrch core runtime convergence authority / NextAction interpreter / evidence diagnostic classifier.
  - Secondary: software-dev workspaceTemplate/profile policy for validation failure to implementation strategy.
  - Not owner: TUI/Web projection, provider, or specific live workspace target code.
- Event path:
  - Runtime step result / contract violation -> rejected diagnostic evidence -> coverage/evidence ledger recompute -> `RefreshConvergenceAuthority` -> deterministic NextAction -> rebuilt Stage Packet -> dispatch -> projection ViewModel.
- Contract gap:
  - 缺少 `validation failure diagnostic -> implementation gap -> state_change` 的业务中立转换规则。
  - 缺少重复 same-rule contract violation 的确定性升级策略。
  - dispatch 前 focus-bearing 字段一致性门禁仍不完整。
- Boundary check:
  - Core 只能消费结构化 diagnostic class、evidence kind、NextAction、policy flags、retry budget 和 authority scope。
  - `software-dev` 可声明验证失败如何转实现、如何偏好 patch / validation，但不得由 core 识别 Go、YAML、provider、端口、具体测试名或具体文件。
- Verification:
  - 新增失败样本单测覆盖 validation diagnostic 触发 `state_change`。
  - 新增重复合同违例预算测试覆盖 `managed_validation` / `fail` 升级。
  - 新增 dispatch input focus alignment fixture 覆盖旧 focus-bearing 字段被重建或 fail-fast。
  - live verification 复核上述两个 workspace 是否从验证循环转入实现或明确治理/失败。

## Current Deviation Summary

### ws-4e83810542ac4d65

- Current focus: `cwi-022`
- Current gap: 内存控制 validation evidence 缺失。
- Deviation:
  - LLM 多次输出验证图候选，但未满足 `validationEvidence.mode` / `validationEvidence.observationType` / `validationScratchDir` 约束。
  - Runtime 正确 rejected，但重复 `retry_same_contract` 后仍未升级。
- Expected closure:
  - 若 runtime 能管理验证，应切换为 `managed_validation` 或更窄的 runtime-managed validation action。
  - 若仍依赖 LLM 且 same-rule violation 达预算，应进入 governed blocker 或 fail，而不是继续相同 retry。

### ws-7ced70e98ea50c43

- Current focus: `cwi-008`
- Current gap: provider/config capability validation evidence 缺失。
- Deviation:
  - 验证失败暴露交付物存在真实配置加载缺口，但 NextAction 仍停留在 validation repair。
  - `main.go` 尚未实现完整 `config.yaml` / provider config loading；这是 target workspace 的交付缺口，不应由 core 特判。
- Expected closure:
  - Runtime 应根据结构化 validation failure diagnostic 将当前 focus 标记为 `implementation_gap_detected`。
  - NextAction 应切换为 `state_change`，让 LLM 产出当前 focus 范围内的最小实现补丁，再进入 validation。

## Target State

目标收敛路径：

```text
validation / contract candidate fails
  -> rejected diagnostic evidence
  -> classify diagnostic class
  -> recompute coverage + current focus
  -> derive deterministic NextAction:
       validation_environment_failure -> repair validation
       validation_contract_violation  -> bounded contract retry / managed validation
       implementation_gap_detected    -> state_change
       external_dependency_missing    -> request_decision / request_approval / blocked
       repeated_same_rule_exhausted   -> managed_validation / fail
  -> rebuild Stage Packet from current authority snapshot
  -> dispatch
```

## Required Changes

### S1. Validation Failure Diagnostic Taxonomy

Status: implemented

Add or consolidate structured diagnostic classes:

- `validation_environment_failure`
- `validation_contract_violation`
- `implementation_gap_detected`
- `external_dependency_missing`
- `provider_transient_failure`
- `runtime_internal_contract_misalignment`

Rules:

- Core must not infer these classes from business strings.
- Classification must use structured block/runtime outputs, contract gate reason, evidence policy, and template/profile-provided diagnostic hints.
- Raw stderr/stdout may be retained as diagnostic evidence, but cannot be parsed in core for specific business semantics.

Acceptance:

- A shell validation failure can record a generic `validationFailureDiagnostic` with class and evidence role.
- Rejected diagnostics do not satisfy coverage.
- 2026-06-27 closure:
  - Added neutral runtime contract classes for validation contract violation, implementation gap, external dependency, provider transient failure and runtime internal misalignment.
  - `normalizeValidationFailureDiagnostic` now admits those structured classes without parsing business stderr/stdout.

### S2. NextAction Transition From Validation Failure To StateChange

Status: implemented

Extend deterministic NextAction interpreter:

- If current focus lacks validation evidence and latest rejected diagnostic class is `implementation_gap_detected`, derive `state_change`.
- If current focus lacks validation evidence and latest rejected diagnostic class is `validation_environment_failure`, derive `repair` or `validate` with controlled substitute.
- If current focus latest diagnostic is `validation_contract_violation`, derive bounded contract retry only while budget allows.
- If no state-changing output is allowed for the current policy, derive governed blocker or fail with explicit reason.

Acceptance:

- Fixture: current focus has failed validation diagnostic classified as `implementation_gap_detected`; expected NextAction is `state_change`.
- Fixture: validation environment failure remains `repair` / `validate`.
- Fixture: rejected diagnostic alone never makes coverage satisfied.
- 2026-06-27 closure:
  - Missing validation NextAction now loads rejected diagnostic evidence and derives `state_change` when the current focus has `implementation_gap_detected`.
  - The generated requiredOutput is bound to the rejected diagnostic id and requires state-changing evidence; rejected diagnostic evidence remains rejected.

### S3. Repeated Contract Violation Escalation

Status: implemented

Focus-scoped retry budget must count same-rule and same-signature violations across retry lineage.

Rules:

- Same `focusId + violatedRule + failureSignature` increments repeat count.
- Within budget: retry same contract with targeted correction.
- At budget boundary:
  - If managed validation is available and action is validation, derive `managed_validation`.
  - Else if missing external dependency is explicit, derive `request_decision` or `request_approval`.
  - Else derive `fail` with structured reason `contract_retry_exhausted`.

Acceptance:

- `concrete_validation_contract_missing` repeated beyond budget cannot dispatch another ordinary LLM retry.
- Projection exposes retry count, max attempts, latest violation rule and escalation reason.
- 2026-06-27 closure:
  - Focus-scoped rejected diagnostics count same signature/rule and route validation contract retry exhaustion to managed harness when declared, otherwise deterministic `fail`.
  - NextAction requiredOutput exposes retry attempt, max attempts, violated rule, failure signature and `contract_retry_exhausted`.

### S4. Dispatch Input Authority Alignment Gate

Status: implemented

Before every provider/block dispatch, validate all focus-bearing fields against the current authority snapshot:

- `coverageFocus`
- `acceptance.currentFocus`
- `convergenceBudget.focusId`
- `repairActionPackage.coverageFocus`
- `repairActionPackage.requiredOutput.coverageFocusId`
- top-level `requiredOutput.coverageFocusId`
- `stagePacket.requiredOutput.coverageFocusId`
- current `coverageWorkItemContext.currentFocus`

Rules:

- If stale fields can be rebuilt from current authority, rebuild them.
- If stale fields remain after rebuild, fail-fast as `runtime_internal_contract_misalignment`.
- Historical lineage may retain old focus only under diagnostic-only sections.

Acceptance:

- Fixture with `acceptance.currentFocus=cwi-085` and authority focus `cwi-008` is either rebuilt or rejected before LLM dispatch.
- LLM request C3/C4 contains only one current authority focus.
- 2026-06-27 closure:
  - Dispatch input normalization now rebuilds nested `executionContext.requiredOutput`, `executionContext.repairActionPackage.requiredOutput`, `executionContext.stagePacket.requiredOutput`, `convergenceBudget`, and nested `coverageWorkItemContext.currentFocus`.
  - Validation gate checks those same focus-bearing fields and fails fast as `runtime_internal_contract_misalignment` if stale focus remains.

### S5. Software-Dev Template Policy For Validation Failure To Implementation

Status: implemented

Add software-dev profile/template policy that declares how validation failures should influence action selection.

Policy examples:

- `validationFailureImpliesStateChange=true` for executable contract failures tied to current deliverable capability.
- `preferMinimalPatchBeforeRepeatedValidation=true`.
- `maxValidationOnlyRetriesPerFocus`.
- `implementationGapDiagnosticHints` as structured template data, not core hard-coded strings.

Rules:

- The policy must be bound through workspaceTemplate/profile into workspace/run execution context.
- Core only reads policy flags and structured diagnostic classes.

Acceptance:

- Unit/integration test proves policy is present in run execution context.
- Non software-dev template is unaffected unless it declares equivalent policy.
- 2026-06-27 closure:
  - `software-dev-validation-discovery-first` declares `validationFailureImpliesStateChange`, `implementationGapDiagnosticClasses`, `maxValidationOnlyRetriesPerFocus`, and minimal patch preference in template data.
  - Core reads only neutral policy flags/classes; it does not inspect the `software-dev` template id or target project text.

### S6. Context Reduction For Current Focus Repair

Status: partially implemented

Reduce LLM context for repeated repair/validation loops:

- Always include current Goal/DoD, current focus, requiredOutput, accepted/rejected evidence refs and latest failure diagnostic.
- Prefer recent complete data + historical summary refs.
- Omit stale rejected LLM summaries as reusable progress; keep source refs and compact failure class.
- Do not include parallel focus-bearing fields from old slices as current authority.

Acceptance:

- LLM request for retry keeps C0 stable prefix unchanged.
- C4 current-action contains latest failure surface and current focus only.
- Live request input size trend decreases without losing required evidence refs.
- 2026-06-27 closure:
  - Authority alignment removes stale parallel focus-bearing fields from current dispatch input and keeps old focus only as diagnostic lineage where applicable.
  - Live request size trend and cache-prefix verification remain deployment-time checks; no broad prompt partition or context budget rewrite was included in this code pass.

## Implementation Order

1. S4 Dispatch Input Authority Alignment Gate.
2. S1 Validation Failure Diagnostic Taxonomy.
3. S2 NextAction Transition From Validation Failure To StateChange.
4. S3 Repeated Contract Violation Escalation.
5. S5 Software-Dev Template Policy.
6. S6 Context Reduction.

Rationale:

- S4 prevents stale authority from polluting any later change.
- S1/S2 make the runtime switch from validation repair to state_change deterministic.
- S3 prevents LLM contract retry loops.
- S5 keeps software-development behavior in template/profile rather than core.
- S6 reduces repeated noisy context after the state machine is correct.

## Verification Plan

### Targeted Tests

- `go test ./internal/modules/kernel/orchestrator/biz -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1`
- `go test ./internal/pkg/workorchdefaults -count=1`

Add focused tests for:

- Validation diagnostic classification.
- NextAction transition to `state_change` after implementation gap.
- Same-rule contract retry exhaustion.
- Dispatch input focus alignment.
- Template policy binding into run execution context.

### Boundary Search

Run after code changes:

```bash
rg -n "software-dev|LSP|config.yaml|provider|go test|port|main.go|lsp-bridge" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "coverageFocus|acceptance.currentFocus|requiredOutput|stagePacket|convergenceBudget|repairActionPackage" internal/modules/kernel/orchestrator -g "*.go"
rg -n "contractViolation|validationFailureDiagnostic|NextAction|state_change|managed_validation" internal/modules/kernel/orchestrator -g "*.go"
```

Expected:

- First search must not reveal new business/template/language/file/command special cases in core.
- Later searches should show all focus-bearing fields pass through authority refresh / dispatch gate.

### Live Verification

After deployment:

- `ws-4e83810542ac4d65`:
  - same-rule contract violation should either produce admitted managed validation, governed blocker, or fail after budget.
  - It should not keep dispatching ordinary LLM retries for the same `concrete_validation_contract_missing`.
- `ws-7ced70e98ea50c43`:
  - validation failure that proves implementation gap should switch NextAction to `state_change`.
  - The run should produce a minimal target patch or explicit governed blocker, not only further validation repair.
- Projection:
  - workspace list, snapshot, TUI prompt and ViewModel show the same run state, current focus, NextAction and retry diagnostics.

## Completion Criteria

- Deterministic NextAction can switch from validation failure to state_change using only structured evidence and template policy.
- Repeated contract violations are bounded and escalate deterministically.
- Dispatch input contains exactly one current authority focus.
- Rejected diagnostics remain rejected and never satisfy coverage.
- Core remains business neutral; software-development-specific recovery preference is data-driven through template/profile policy.
