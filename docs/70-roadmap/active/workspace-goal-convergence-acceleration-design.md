# Workspace Goal Convergence Acceleration Design

Status: active

Type: design

Last Updated: 2026-06-20

## Purpose

本文档定义 WorkOrch 在复杂 workspace 中加速 Goal/DoD 收敛的长期收口方案。

当前线上核对显示，workorchd 调度、EventHub、runtime queue 和 workspace projection 没有停滞；workspace 仍未完成的主要原因是 Goal/DoD 覆盖面大、repair 输入上下文过重、failure focus 不够窄、validation failure 误导修复目标，以及 software-dev template 对“先闭合可验收片段”的约束不足。

本设计的目标是让 WorkOrch 从“一个 run 背负全部 DoD，一直 repair 到全量通过”调整为“按可验收 slice 聚焦推进、按当前 failure focus 精确修复、按预算升级或切片”，从而减少无效 LLM 轮次，加快可交付结果闭合。

## Current Observations

基于 2026-06-20 的 live workspace 核对：

| Workspace | Current State | Coverage | Latest Failure | Main Drag |
| --- | --- | --- | --- | --- |
| `ws-599a026e5f506f46` | running / repairing | `total=40, open=39, in_progress=1` | `dod_coverage_missing` | DoD 目标面过宽，缺少 validation/state_change/version snapshot 证据闭合 |
| `ws-6b5f2f24cda25364` | running / repairing | `total=89, evidence_ready=62, open=24, failed=2, in_progress=1` | `shell_command_failed` | 验证失败被定位到 vendor 文件，真实问题更接近 toolchain/module config mismatch |

Observed runtime characteristics:

- 单轮 LLM 请求体常在 500KB 到 1MB 以上。
- repair input 可达 900KB 到 1.14MB。
- 单轮 LLM 耗时常在 70s 到 170s。
- 多轮 follow-up / repair 仍会围绕 validation、evidence binding、version snapshot 反复循环。
- Completion gate 正确阻止了不完整结果被接受，但缺少更细粒度的阶段性 accepted result。

## Goals

1. 支持将大 Goal / 大 DoD 拆成可验收的 Acceptance Slice，允许 workspace 先完成 MVP 或当前 milestone。
2. 让 repair 严格围绕当前 coverage focus / failure target 推进，减少扩图和泛化检查。
3. 降低 repair / follow-up LLM 请求体大小，同时保留 Goal/DoD、当前 failure、accepted/rejected evidence 和当前 requested action。
4. 提升 validation failure 分类质量，避免把环境、toolchain、命令合同、依赖配置错误误导为源码或 vendor 修复。
5. 将 software-dev 相关策略沉淀到 template / Soul / profile / evidence policy，不污染 core。
6. 增加 convergence budget，避免同一 focus 无限 repair。
7. 保证 TUI/Web 仍只展示 workorchd projection，不形成端侧业务状态机。

## Non-Goals

- 不降低 completion gate 对最终 accepted result 的严格性。
- 不在 kernel/orchestrator 中写入 Go、Node、Python、vendor、`go test` 等场景特判。
- 不以缓存命中为目的删除当前 Goal/DoD、requiredOutput、failure evidence、target snapshot、accepted/rejected evidence。
- 不把某个 live workspace 的目标文本、文件路径或命令输出硬编码为 core 逻辑。
- 不要求一次性重写所有 runtime flow；本设计按可验证阶段逐步收口。

## Boundary Rules

Core 可以实现：

- Acceptance Slice 通用模型和状态机。
- Coverage focus selection、repair focus lock、evidence binding gate。
- Context compaction / history summary / rejected evidence filtering。
- Provider 前 context gate。
- Validation failure 通用分类 schema。
- Convergence budget 和 escalation 机制。
- Read model / projection / API / TUI DTO 支持。

Template / Soul / profile 可以实现：

- software-dev 的 slice 默认划分。
- software-dev 的 evidence kind 要求。
- software-dev 的 validation strategy guidance。
- toolchain/config mismatch 的修复优先级描述。
- version_control_snapshot 的提交策略和默认阈值。

Core 禁止实现：

- 根据 `software-dev`、具体 DoD 文本、具体语言、具体命令输出或具体文件名决定分支。
- 根据 `vendor/gopkg.in/yaml.v3`、`go1.16`、`go1.18` 等文本做特判。
- 直接生成某个业务模板的验证命令。

## Target Concepts

### Acceptance Slice

Acceptance Slice 是 Goal/DoD 的一个可闭合验收片段。它不是新的业务目标，而是当前 Goal/DoD 的收敛视图。

Suggested fields:

```json
{
  "id": "slice-mvp-required",
  "title": "MVP required acceptance",
  "state": "active",
  "priority": 10,
  "scope": "mvp_required",
  "coverageWorkItemIds": ["cwi-001", "cwi-002"],
  "requiredEvidenceKinds": ["state_change", "validation"],
  "completionPolicy": "all_required_items_accepted",
  "resultPolicy": "slice_result_allowed",
  "createdAt": "...",
  "activatedAt": "...",
  "closedAt": ""
}
```

State model:

- `pending`: slice declared but not active.
- `active`: runtime should focus on this slice.
- `blocked`: slice cannot continue without decision/approval/external input.
- `accepted`: all required items accepted and slice result created.
- `failed`: budget exhausted or unrecoverable failure.
- `superseded`: replaced by a newer slice revision.

### Coverage Focus

Coverage Focus is the single current work item or small bounded group that repair is allowed to address.

Suggested fields:

```json
{
  "id": "focus-cwi-062",
  "sliceId": "slice-validation-required",
  "coverageWorkItemIds": ["cwi-062"],
  "failureClass": "validation_failed",
  "failureSignature": "...",
  "requiredEvidenceKinds": ["validation"],
  "acceptedEvidenceRefs": [],
  "rejectedEvidenceRefs": ["..."],
  "targetRefs": ["..."],
  "state": "active",
  "attempt": 2
}
```

Repair must either:

- produce evidence that binds to this focus;
- produce state change that targets this focus;
- return `cannot_progress` with a structured blocker.

### Focus-Locked Repair

Focus-locked repair is a runtime contract that prevents broad graph expansion during repair.

Rules:

- Repair input contains one primary `coverageFocus`.
- LLM output must cite `coverageFocus.id`.
- Every state-changing node must declare which focus it addresses.
- Every validation node must declare which accepted/rejected evidence it expects.
- If output does not reference current focus, runtime rejects it as `repair_focus_mismatch`.
- If output only adds unrelated reports/scripts/fixtures, runtime rejects it as `unrelated_repair_output`.

### Convergence Budget

Convergence budget is a generic runtime guard for ineffective loops.

Suggested dimensions:

- `maxRepairAttemptsPerFocus`
- `maxLLMRoundsPerSlice`
- `maxRepeatedFailureSignature`
- `maxValidationOnlyRetries`
- `maxNoCoverageDeltaRounds`
- `maxContextBytesPerRepair`

Budget outcomes:

- `continue`: keep current focus.
- `narrow_focus`: reduce to smaller focus.
- `switch_slice`: activate next viable slice.
- `decision_required`: request user decision.
- `failed`: mark run/slice failed when no automatic recovery is safe.

### Validation Failure Classification

Validation failure classification should be generic and structured.

Initial classes:

- `validation_command_invalid`: command shape is invalid or forbidden.
- `validation_environment_missing`: required local executable/service/env/config is missing.
- `validation_dependency_mismatch`: dependency or module version mismatch.
- `validation_toolchain_mismatch`: compiler/runtime/toolchain version mismatch.
- `validation_build_failed`: source does not compile/build.
- `validation_test_failed`: test assertion failed.
- `validation_timeout`: bounded command timed out.
- `validation_output_inconclusive`: command ran but did not produce required evidence.
- `validation_evidence_binding_missing`: output exists but cannot bind to current coverage focus.

The classifier may use generic structured fields:

- exit code
- stderr/stdout summaries
- command kind
- target refs
- dependency/toolchain metadata when available
- runtime environment summary

It must not branch on language-specific strings in core. Language-specific interpretation belongs to template/Soul guidance or capability metadata.

### Context Compaction

Context compaction reduces LLM request size while preserving convergence-critical fields.

Required protected fields:

- current Goal/DoD and definition revision
- active acceptance slice
- current coverage focus
- repairActionPackage
- current failure and latest actionable diagnostics
- accepted evidence for current slice/focus
- rejected evidence for current focus
- target snapshots required for patch safety
- current requested action

Compaction rules:

- Recent key events: include complete records for the last bounded N relevant events.
- Older events: include ordered summary and source refs only.
- Rejected evidence: include full detail only when it targets current focus.
- Old execution graphs: include summary and node IDs unless current focus depends on exact graph output.
- Memory recall: include only high-value, non-stale, current-definition-compatible references.
- Large file snapshots: include hash, path, bounded excerpt, and source ref unless exact patch needs full content.

## Target Flow

### Activation / Replan

1. Load latest Goal/DoD definition.
2. Generate or refresh coverage work items.
3. Generate acceptance slices from coverage work items.
4. Activate highest-priority pending slice.
5. Select initial coverage focus inside active slice.
6. Start LLM planning with active slice and focus in C3/C4, not C0.

### Execution

1. LLM returns execution graph for active slice/focus.
2. Runtime validates graph focus bindings.
3. Runtime executes allowed steps.
4. Runtime records evidence and binds evidence to coverage work items.
5. Completion gate evaluates current focus, then active slice.

### Repair

1. On failure, runtime builds repairActionPackage from current focus.
2. Context compaction includes only relevant recent evidence and rejected refs.
3. LLM must repair current focus or return structured blocker.
4. Runtime rejects unrelated graph expansion before execution.
5. Convergence budget updates after every repair attempt.

### Slice Acceptance

1. When all required coverage items in active slice are accepted, create a slice result.
2. Workspace remains running if other required slices remain.
3. TUI/Web display accepted slices and remaining slices separately.
4. Final accepted result is created only when final completion policy passes.

### Budget Escalation

1. If repeated failure signature exceeds budget, runtime stops automatic repair for that focus.
2. If another slice can progress independently, runtime may switch slice.
3. If no slice can progress safely, runtime opens decision ticket with:
   - current focus
   - repeated failure signature
   - accepted/rejected evidence
   - recommended actions

## Data Model Changes

### Kernel / Common Models

Add generic model definitions:

- `AcceptanceSlice`
- `AcceptanceSliceState`
- `CoverageFocus`
- `CoverageFocusState`
- `ConvergenceBudget`
- `ConvergenceBudgetDecision`
- `ValidationFailureDiagnostic`

Recommended package locations:

- `internal/modules/kernel/orchestrator/pkg/models` or a shared runtime contract package for orchestration-owned state.
- If multiple modules must consume these as event payloads, define public DTOs in module `pkg/contracts` or existing runtime contract package.

### Run Execution Context

Extend run execution context with:

```json
{
  "acceptanceSlices": [],
  "activeSliceId": "",
  "coverageFocus": {},
  "convergenceBudget": {},
  "convergenceHistory": []
}
```

Rules:

- Slice/focus data is runtime dynamic and belongs to C3/C4 request sections.
- It must not affect C0 stable prefix.
- Goal/DoD changes create new definition revision and may supersede slices.

### Evidence Binding

Every evidence candidate should carry:

```json
{
  "coverageFocusId": "",
  "coverageWorkItemIds": [],
  "evidenceKind": "validation",
  "evidenceRole": "primary_state_change",
  "targetRefs": [],
  "validationObservationType": "",
  "accepted": false,
  "rejectionReason": ""
}
```

## Module / Code Impact

### Core Runtime / Orchestrator

Expected areas:

- coverage work item generation and ledger
- completion gate
- repairActionPackage builder
- LLM request context builder
- graph validation / graph gate
- result acceptance
- repair budget / escalation

Required behavior:

- Select and persist active slice/focus.
- Reject repair graph output that does not bind to current focus.
- Track focus-level repair attempts and coverage delta.
- Create slice-level result when slice is accepted.
- Escalate repeated no-delta repair loops.

### Blocks

Expected areas:

- shell block result metadata
- patch/apply result metadata
- file/read snapshot metadata
- git/write result metadata

Required behavior:

- Preserve enough structured metadata for evidence binding.
- Do not add business-specific interpretation inside blocks.
- Shell block should expose generic failure diagnostics: exit code, stdout/stderr excerpt, timeout, command policy violation, target refs when provided.

### LLM Context / Provider

Expected areas:

- prompt/context section assembly
- context budget compaction
- request log/cache plan

Required behavior:

- Add active slice/focus to dynamic sections only.
- Add compacted history summary.
- Preserve stable prefix hashes when only focus/repair/progress changes.
- Keep current failure and requiredOutput protected.

### AppService / Workorchd Projection

Expected areas:

- workspace snapshot projection
- run observability
- task progress
- metrics
- REST DTOs

Required behavior:

- Project active slice/focus.
- Show accepted slice count, open slice count, current focus and budget status.
- Do not let TUI/Web infer slice/focus state independently.

### TUI / Web

Expected behavior:

- Display active slice and current focus.
- Display why run is not complete:
  - open slice count
  - failed focus
  - missing evidence kinds
  - repeated failure signature
- Keep command submission unchanged: TUI/Web submit commands, workorchd owns state.

### Template / Soul / Profile

software-dev changes:

- Define default acceptance slice policy.
- Prioritize source_change + validation + version_control_snapshot closure.
- Treat broad performance/platform compatibility as later slice unless explicitly required by current goal.
- Guide toolchain/dependency mismatch repair toward config/module/toolchain evidence, not vendor source edits.
- Require meaningful source changes before creating supplemental reports.

## REST / Projection Additions

Workspace snapshot should include:

```json
{
  "acceptance": {
    "activeSliceId": "",
    "slices": [],
    "currentFocus": {},
    "budget": {},
    "summary": {
      "acceptedSlices": 0,
      "activeSlices": 1,
      "openSlices": 0,
      "failedSlices": 0
    }
  }
}
```

Run observability should include:

- `activeSlice`
- `coverageFocus`
- `convergenceBudget`
- `focusAttempts`
- `coverageDeltaHistory`
- `lastNoDeltaReason`

Metrics should include:

- `workorch_convergence_active_slices`
- `workorch_convergence_open_focuses`
- `workorch_convergence_repair_attempts_total`
- `workorch_convergence_no_delta_rounds_total`
- `workorch_convergence_budget_escalations_total`

## Verification Strategy

### Unit Tests

Core:

- slice generation from coverage work items
- active slice selection
- focus selection and persistence
- repair graph focus mismatch rejection
- convergence budget no-delta escalation
- slice accepted result creation
- final accepted result waits for required slices

Context:

- active slice/focus is in C3/C4, not C0
- stable prefix unchanged across focus/repair progress changes
- current failure and rejected evidence for current focus are protected
- unrelated old rejected evidence is summarized

Validation classification:

- command policy violation
- timeout
- missing executable/environment
- build failure
- dependency/toolchain mismatch using generic structured diagnostics
- inconclusive output

Projection:

- workspace snapshot projects active slice/focus
- TUI/Web DTO can render not-complete reason without local inference

### E2E Tests

Add or extend deployed-instance e2e:

1. Large DoD split into two required slices and one later slice.
2. First slice accepted while final workspace remains running.
3. Repair output that targets unrelated focus is rejected before execution.
4. Repeated validation-only repair escalates to decision or slice switch.
5. software-dev workspace closes source_change -> validation -> version_control_snapshot in order.
6. Toolchain/config mismatch is surfaced as validation diagnostic, not vendor source target.

### Live Verification

For existing workspaces:

- `ws-599a026e5f506f46`: should show active slice and current focus; open items should be bounded by slice instead of all 40 items.
- `ws-6b5f2f24cda25364`: toolchain/config validation failure should be classified and repair target should not point to vendor source.

## Phased Implementation Plan

### Phase 1: Read Model and Design Plumbing

| ID | Task | Owner Area | Status |
| --- | --- | --- | --- |
| WGC-01 | Add `AcceptanceSlice` and `CoverageFocus` DTO/model definitions | core model | completed |
| WGC-02 | Project slice/focus in workspace snapshot and run observability | appservice/readmodel | completed |
| WGC-03 | Display active slice/focus in TUI/Web from projection only | product interface | completed |
| WGC-04 | Add metrics for convergence state | workorchd/server | completed |

Acceptance:

- No behavior change to execution yet.
- Existing runs still work.
- Projection can render empty slice/focus for old runs.

Implementation notes:

- `readmodelcore.AcceptanceProjection` is now exposed on workspace snapshot, run snapshot and run observability.
- Projection reads generic run execution context fields (`acceptanceSlices`, `activeSliceId`, `coverageFocus`, `convergenceBudget`) and the equivalent nested `acceptance` object. Missing fields render an empty summary for old runs.
- TUI and Web display only the workorchd projection. They do not infer slice/focus state locally.
- Metrics are projected from workspace read model snapshots:
  - `workorch_convergence_active_slices`
  - `workorch_convergence_open_focuses`
  - `workorch_convergence_repair_attempts_total`
  - `workorch_convergence_no_delta_rounds_total`
  - `workorch_convergence_budget_escalations_total`

### Phase 2: Slice Generation and Selection

| ID | Task | Owner Area | Status |
| --- | --- | --- | --- |
| WGC-05 | Generate slices from coverage work items using generic policy fields | orchestrator | pending |
| WGC-06 | Persist active slice in run execution context | orchestrator/run | pending |
| WGC-07 | Select current focus inside active slice | orchestrator | pending |
| WGC-08 | Add software-dev default slice policy in template/profile | template | pending |

Acceptance:

- Large DoD no longer presents all work items as one active focus.
- Current LLM input includes active slice/focus.
- Core remains template-neutral.

### Phase 3: Focus-Locked Repair

| ID | Task | Owner Area | Status |
| --- | --- | --- | --- |
| WGC-09 | Extend repairActionPackage with required focus binding | orchestrator | pending |
| WGC-10 | Reject LLM graph outputs that do not bind to current focus | orchestrator graph gate | pending |
| WGC-11 | Reject unrelated reports/scripts/fixtures when focus needs source_change | generic graph/evidence gate plus template policy | pending |
| WGC-12 | Add tests for repair focus mismatch and unrelated repair output | tests | pending |

Acceptance:

- Repair cannot drift to unrelated graph.
- Rejected repair output is visible as structured failure.
- Runtime can still return `cannot_progress` when focus is genuinely blocked.

### Phase 4: Context Compaction for Repair

| ID | Task | Owner Area | Status |
| --- | --- | --- | --- |
| WGC-13 | Add focus-scoped rejected evidence filtering | context builder | pending |
| WGC-14 | Summarize old execution graphs by default | context builder | pending |
| WGC-15 | Restrict memory recall to current-definition high-value items | memory/context | pending |
| WGC-16 | Add stable-prefix regression tests for focus changes | llm/provider context tests | pending |

Acceptance:

- Repair request bytes are materially reduced on large workspaces.
- Current failure/focus/requiredOutput remain protected.
- Cache stable prefix does not change because of dynamic repair state.

### Phase 5: Validation Failure Diagnostics

| ID | Task | Owner Area | Status |
| --- | --- | --- | --- |
| WGC-17 | Define generic validation diagnostic schema | runtime contract | pending |
| WGC-18 | Populate shell failure metadata for classifier | shell block/runtime evidence | pending |
| WGC-19 | Classify validation failure into generic categories | orchestrator/evidence quality | pending |
| WGC-20 | Add software-dev guidance for toolchain/config mismatch | template/Soul | pending |

Acceptance:

- Toolchain/dependency/config failures are not misdirected to vendor/source file edits.
- Core classifier stays language-neutral.
- Repair prompt sees the diagnostic class and recommended evidence target type.

### Phase 6: Convergence Budget

| ID | Task | Owner Area | Status |
| --- | --- | --- | --- |
| WGC-21 | Add budget fields and update rules | orchestrator | pending |
| WGC-22 | Track no-delta rounds per focus and per slice | orchestrator/readmodel | pending |
| WGC-23 | Escalate repeated failure to decision or slice switch | governance/orchestrator | pending |
| WGC-24 | Add metrics and observability for budget decisions | workorchd/readmodel | pending |

Acceptance:

- Repeated same failure does not loop indefinitely.
- User can see why runtime stopped automatic repair.
- Slice switch only happens when policy allows it.

### Phase 7: Slice Result and Final Closure

| ID | Task | Owner Area | Status |
| --- | --- | --- | --- |
| WGC-25 | Create accepted slice result when slice closes | result/orchestrator | pending |
| WGC-26 | Keep final accepted result gated by required slices | completion gate | pending |
| WGC-27 | Add version_control_snapshot ordering policy for software-dev | template/evidence policy | pending |
| WGC-28 | Add e2e for partial slice acceptance and final acceptance | e2e | pending |

Acceptance:

- A workspace can show meaningful partial acceptance.
- Final accepted result still requires all required slices.
- version snapshot no longer becomes a late repeated blocker after many untracked changes.

## Backward Compatibility

Current stage does not require compatibility with older runtime behavior, but migration should remain safe:

- Old runs without slices project an empty `acceptance` section.
- New runs should generate slices at activation/replan.
- Existing in-flight runs may require replan/restart to adopt slice policy.

## Risks and Controls

| Risk | Control |
| --- | --- |
| Slice acceptance weakens final quality | Final accepted result remains gated by all required slices |
| Template policy leaks into core | Keep slice policy data-driven; review boundary search after each phase |
| Context compaction removes critical evidence | Protected field tests and sourceRef integrity checks |
| Repair focus too narrow blocks valid progress | Allow structured `cannot_progress` and slice switch with budget decision |
| Validation classifier becomes language-specific | Use generic categories; language guidance stays in template/Soul |
| TUI/Web drift from backend state | TUI/Web consume projection DTO only |

## Review Checklist

Before code closure for each phase:

- Does the change keep core business-neutral?
- Does any core code contain template ID, language name, command text, or concrete file path branching?
- Does LLM request still include current Goal/DoD, active slice, current focus, current failure, accepted/rejected evidence and requested action?
- Does stable prefix remain stable when only focus/repair state changes?
- Does TUI/Web use workorchd projection rather than local inference?
- Does each test prove the old slow/failing path is directly affected?

## Expected Outcome

After this design is implemented:

- Large Goal/DoD workspaces converge by slice rather than by all-at-once repair.
- LLM repair rounds become smaller and more focused.
- Validation failures point to the correct class of corrective action.
- Repeated no-delta repair loops become visible and bounded.
- Users can see exactly why a workspace is not complete and which slice/focus is blocking closure.
- WorkOrch core remains a business-neutral orchestration framework.
