# Runtime State Change Target Evidence Decision Gate Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-28

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-deterministic-convergence-closure-plan.md`
- `runtime-nextaction-validation-to-statechange-closure-plan.md`
- `runtime-validation-contract-gate-managed-session-closure-plan.md`

## Purpose

本文根据最新 live workspace 核对结果，收口 WorkOrch runtime 在 `state_change`
NextAction、target evidence、Stage Packet 和 decision gate 之间仍存在的偏差。

目标是保证当 runtime 能通过 workspace 当前文件、manifest、evidence ledger 或中立
capability 自动获取目标快照时，不把该缺口升级成人工 decision；同时保证缺少完整
target snapshot 时，不把矛盾的 state-changing contract 直接发送给 LLM。

本文只描述 WorkOrch core 的业务中立收口范围。不得引入 template ID、Soul ID、
语言、命令、文件名、provider、workspace title 或 live workspace ID 分支。

## Contract Audit

- Scope: `ws-4e83810542ac4d65` / `run-008` / `run-008/step-1` /
  `run-008-step-1-decision`，以及对应的 runtime state-changing dispatch、
  Stage Packet、repairActionPackage 和 decision gate。
- Observed state:
  - workspace state 为 `waiting_decision`，active run 为 `run-008`。
  - projection 为 `ready`，`sourceEventRevision=1218`，
    `projectedSourceRevision=1218`，无投影滞后。
  - step input 中存在：
    - `runtimeStage=repair`
    - `repairObjective=produce_state_changing_output`
    - `stateChangingOutputRequired=true`
    - `firstRunGapStateChangingContract=true`
    - `emptyWorkspaceEvidenceEscalatedToImplementation=true`
  - 当前 workspace 并非空目录；
    `executionContextPackage.workspaceFileManifest` 已包含多个可读文件条目。
  - `repairActionPackage.targetSnapshots` 为空，
    `repairActionPackage.workspaceFileManifest` 为空。
  - LLM 返回 `requiredDecision`，要求人工提供当前目标快照，并声明
    `runtimeCanContinue=false`。
  - runtime 创建人工 decision ticket，而不是转入自动 target snapshot /
    concrete failure evidence acquisition。
- Expected state:
  - `state_change` NextAction dispatch 前，runtime 必须确认当前 Stage Packet、
    current focus、requiredOutput、convergenceContract 和 target evidence 来自同一次
    convergence authority snapshot。
  - 若 workspace 已有可读资源但缺少 full target snapshot，runtime 应自动转入
    `acquire_concrete_failure_evidence` 或等价 target snapshot acquisition。
  - 只有缺少 runtime 无法获得的外部信息、权限、凭证、审批、用户选择或配置时，
    才创建人工 decision。
- Deviation:
  - 非空 workspace 仍被标记为 `emptyWorkspaceEvidenceEscalatedToImplementation`。
  - 可读 workspace manifest 未进入当前 `repairActionPackage` 的 actionable evidence。
  - 缺 target snapshot 的 state-changing contract 被直接发送给 LLM。
  - LLM 对可自动读取资源的 blocker 被允许创建人工 decision。
- Owner: WorkOrch core。问题位于 convergence authority、step input rendering、
  Stage Packet/repairActionPackage 派生和 decision eligibility gate，不属于
  workspaceTemplate/Soul/policy、TUI/Web 或 provider。
- Event path:
  - resume/replan/dispatch command
  - owner run state
  - convergence authority refresh
  - first-run gap / state_change NextAction
  - Stage Packet and repairActionPackage rendering
  - LLM candidate output
  - decision gate
  - decision ticket / repair follow-up
  - projection ViewModel
- Contract gap:
  - `emptyWorkspaceEvidenceEscalatedToImplementation` 缺少真实空 workspace 判定。
  - `workspaceFileManifest`、`visibleWorkspaceFiles`、`candidate repair target path`
    等结构化可读资源没有统一归一为 runtime-readable target source。
  - state-changing dispatch 缺少 target evidence 完整性门禁。
  - decision gate 仍可能信任 LLM 的 `runtimeCanContinue=false`，而不是由 runtime
    根据 capability 和可读资源做 deterministic 判断。
- Boundary check:
  - 收口只能消费中立结构化字段：workspace manifest、target snapshot、
    evidence kind、NextAction、requiredOutput、decision context scope。
  - 测试 fixture 可以包含具体路径，但 production 代码不得按路径、语言、模板或
    workspace ID 分支。

## Required Closure

### P0. Correct Empty Workspace Escalation

`emptyWorkspaceEvidenceEscalatedToImplementation` 只能在真实空 workspace 或无可用
deliverable target resource 时设置。

Required behavior:

- If current input or execution context contains a non-empty workspace file manifest, do not
  mark the step as empty-workspace escalation.
- If accepted evidence or runtime-readable resources already point to existing targets, do not
  use empty-workspace escalation.
- True empty workspace may still enter state-changing file creation, but the required output
  must be bounded to the smallest cohesive deliverable resource.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_graph.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`

Acceptance:

- A non-empty workspace manifest prevents `emptyWorkspaceEvidenceEscalatedToImplementation`.
- Existing tests for true empty workspace initialization still pass through a neutral fixture.

### P0. Gate State-Changing Dispatch On Target Evidence

Before provider/block dispatch, if current NextAction requires state-changing output, runtime
must verify target evidence state.

Required behavior:

- If full target snapshots are available, bind them into `repairActionPackage.targetSnapshots`,
  `stagePacket.targetSnapshots`, and requiredOutput snapshot rules.
- If workspace resources are readable but full target snapshots are missing, select
  `acquire_concrete_failure_evidence` or equivalent target snapshot acquisition before asking
  LLM for a patch.
- If no target resource can be inferred and the workspace is not proven empty, fail fast as
  runtime internal contract misalignment or request a real governance blocker only when the
  missing information is external to runtime.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`

Acceptance:

- `stateChangingOutputRequired=true` plus non-empty manifest plus no target snapshot does not
  dispatch a direct patch contract.
- The next step becomes evidence acquisition / target snapshot acquisition, or the dispatch is
  rejected as internal misalignment.

### P0. Make Decision Gate Runtime-Authoritative

LLM-provided `runtimeCanContinue=false` must not be authoritative. Runtime determines whether
the blocker is continuable.

Required behavior:

- Decision contexts that reference runtime-readable workspace resources must convert to repair
  or evidence acquisition instead of ticket creation.
- The gate must recognize structured readable resource fields, including canonicalized forms
  derived from:
  - `workspaceFileManifest`
  - `workspaceFiles`
  - `existingFiles`
  - `readableResources`
  - `knownResources`
  - `visibleWorkspaceFiles`
  - `candidateValues.repairTargetSnapshot.path`
- If the context declares WorkOrch runtime dependency, provider dependency, external dependency,
  credentials, permissions, approval, or explicit user choice, the decision can remain governed.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_blocks.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_context_material.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_specs.go`

Acceptance:

- LLM decision asking for a current file snapshot is converted to automated repair when the
  path is runtime-readable.
- Provider configuration failures still create governed decision and are not converted to repair.

### P1. Normalize Readable Resource Signals

Runtime should normalize structurally equivalent readable resource signals before decision
eligibility and repair input rendering.

Required behavior:

- Do not rely on LLM-specific field names as authority.
- Convert supported readable resource signals into one neutral internal representation, such as
  `readableResources`.
- Preserve original fields only as diagnostics.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_blocks.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_context_material.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go` only if rendering guidance must be
  clarified without moving dynamic fields into C0.

Acceptance:

- A fixture using `visibleWorkspaceFiles` behaves the same as one using `readableResources`.
- LLM request partition remains valid: dynamic current resource details stay in C3/C4/C5, not C0.

### P1. Preserve Provider Failure Semantics

Provider/environment blockers must remain separate from core convergence blockers.

Required behavior:

- Provider retry exhausted decisions with `scope=workorch_runtime`,
  `dependency=llm_provider`, or `recoveryPolicy=system_config_required` remain governed.
- Decision gate must not convert provider/proxy timeout decisions into repair follow-up.

Acceptance:

- A provider timeout / retry exhausted fixture still creates the system configuration decision.

## Regression Test Plan

Add targeted tests under orchestrator core using neutral fixtures.

Required failing samples:

1. Non-empty workspace manifest must not set empty escalation.
2. State-changing NextAction with readable resources but missing target snapshot must select
   evidence acquisition before direct patch dispatch.
3. LLM decision requesting a runtime-readable target snapshot must convert to repair/follow-up.
4. LLM decision requesting external provider/config/credential input must remain governed.
5. Stage Packet, convergenceContract, gapBaseline and repairActionPackage requiredOutput must
   remain aligned after the target evidence gate.

Suggested commands:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "emptyWorkspaceEvidenceEscalatedToImplementation|stateChangingOutputRequired|targetSnapshots|visibleWorkspaceFiles|runtimeCanContinue|readableResources" internal/modules/kernel/orchestrator internal/modules/blocks/llm -g "*.go"
```

## Live Verification Plan

After code closure and redeploy:

1. Record git commit, binary path, PID, start time.
2. Trigger the affected non-empty workspace through `resume` or decision `replan`.
3. Verify the next run/step does not send a direct state-changing patch contract without full
   target snapshot.
4. Verify the run either:
   - acquires concrete target evidence automatically, then dispatches patch generation, or
   - fails fast with runtime internal contract misalignment if no safe target can be derived.
5. Verify no artificial decision is created for runtime-readable target snapshot gaps.
6. Verify provider retry exhausted workspaces still enter governed provider configuration
   decision.
7. Verify projection remains ready and TUI/Web display the same run state, prompt mode,
   current focus and blocking object.

## Done Criteria

- Non-empty workspace no longer enters empty workspace implementation escalation.
- State-changing dispatch cannot reach LLM with missing target evidence unless it is a true
  create-new-resource flow.
- Runtime-readable target snapshot requests are automatically converted to repair/evidence
  acquisition instead of human decision.
- Provider/environment blockers remain governed decisions.
- Stage Packet、convergenceContract、gapBaseline、repairActionPackage and projection are aligned
  with the current convergence authority snapshot.
- Core remains business-neutral; no template/language/provider/workspace-specific production
  branch is introduced.

## Closure Progress

Status: code closure implemented; live verification pending redeploy.

Implemented code paths:

- `runtime_step_input.go`
  - First-run `state_change` with a non-empty structured workspace manifest now selects
    `acquire_concrete_failure_evidence` instead of direct state-changing output.
  - True empty workspace remains eligible for bounded state-changing output.
- `runtime_repair_action_package.go`
  - `emptyWorkspaceEvidenceEscalatedToImplementation` is no longer treated as actionable
    implementation surface when the input already proves a non-empty workspace manifest.
  - Existing state-changing repair contracts without concrete target evidence are rewritten to
    evidence acquisition before patch output is accepted.
- `runtime_blocks.go`
  - Runtime-readable workspace resource signals now include `visibleWorkspaceFiles` and
    candidate target snapshot path fields.
  - Decision conversion remains blocked for provider, runtime, external dependency,
    credential, permission, approval, or configuration blockers.
- `runtime_statechange_target_evidence_gate_test.go`
  - Added neutral regression fixtures for non-empty manifest evidence acquisition, true empty
    workspace escalation, runtime-readable decision conversion, and provider-governed decisions.

Verification evidence:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "emptyWorkspaceEvidenceEscalatedToImplementation|stateChangingOutputRequired|targetSnapshots|visibleWorkspaceFiles|runtimeCanContinue|readableResources" internal/modules/kernel/orchestrator internal/modules/blocks/llm -g "*.go"
```

Boundary conclusion:

- Production code consumes only neutral structured runtime fields: workspace manifests, target
  snapshots, candidate target paths, decision dependency scope, and required output contracts.
- No production branch is keyed by template ID, Soul ID, language, command, provider,
  workspace title, workspace ID, or specific target file name.
