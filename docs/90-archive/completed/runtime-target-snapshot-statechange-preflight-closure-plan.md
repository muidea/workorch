# Runtime Target Snapshot Statechange Preflight Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-07

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-required-output-authority-and-target-snapshot-closure-plan.md`
- `runtime-target-snapshot-authority-and-projection-closure-plan.md`
- `runtime-state-transition-contract-hardening-closure-plan.md`

## Purpose

本文用于收口当前 live 验证中暴露的 `target snapshot authority`
与 `state_change dispatch` 前置合同缺口。

目标不是放宽 LLM 合同，也不是在 repair/finalization 中兜底，而是在
provider/block dispatch 之前建立确定性前置门禁：

```text
coverage gap requires state_change
  -> target snapshot preflight
  -> acquire_target_snapshot if missing
  -> persist target_snapshot evidence
  -> recompute convergence authority
  -> dispatch state_change with usable snapshot
  -> validate patch/apply and git/write evidence
  -> recompute next action
```

最终要求是：当 `state_change` 需要精确 patch 时，runtime 必须先确认当前
authority snapshot 中已有可用 target snapshot；没有 snapshot 就不能把 LLM
推进到“禁止读取、但必须修改”的不可能合同。

## Contract Audit

- Scope:
  - live workspace `ws-705249a8d7816330/run-091`
  - target snapshot authority
  - state-changing dispatch preflight
  - contract violation classification
  - decision eligibility after retry exhaustion
- Observed state:
  - `run-091` 已进入 `blocked_on_decision`。
  - `currentActionContract.nextAction.kind=state_change`。
  - `stateChangingOutputRequired=true`。
  - `requiredOutput.acceptedKinds=["patch/apply","shell/command","mcp/tool_call","git/write"]`。
  - `requiredOutput.forbiddenKinds` 包含 `read_only_graph` 和
    `same_graph_file_read_before_patch`。
  - Step input 中没有可用 `targetSnapshot` / `targetSnapshots` 字段。
  - 最后一轮原始 LLM response 包含 `patch/apply`、`git/write` 和
    `shell/command`，但因 `patch/apply` 依赖同 graph 的 `file/read` 被拒绝。
  - 最终 step output 仍被归类为 `state_changing_required_missing` 和
    `repair_graph_without_patch_apply`。
  - retry 耗尽后创建了 decision ticket，但 ticket 只提供 `replan` /
    `terminate`，没有明确外部信息、权限、凭证、用户选择或治理确认缺口。
- Expected state:
  - 若当前 focus 需要 `state_change` 且 patch/apply 需要当前文件内容、baseHash
    或 exact oldText，runtime 必须先检查当前 authority snapshot 是否已有可用
    target snapshot。
  - 若 snapshot 缺失，deterministic NextAction 必须改为
    `acquire_target_snapshot`，并渲染只读 requiredOutput。
  - target snapshot acquisition 成功后，应记录 accepted `target_snapshot`
    evidence，并重新计算 authority；下一轮 `state_change` input 必须携带
    snapshot refs。
  - 有 `patch/apply` 但依赖 same-graph read 的输出，应分类为
    `same_graph_file_read_before_patch` / `graph_depends_on_llm_before_patch`，
    而不是 `state_changing_required_missing`。
  - snapshot 缺失不是 human decision；只有 runtime 无法自动获取外部信息、
    权限、凭证、用户选择或治理确认时，才能进入 decision gate。
- Deviation:
  - Runtime 可以生成不可能合同：要求 state-changing output，同时禁止同轮
    read-only acquisition，但又没有提供可用 target snapshot。
  - Contract violation 分类丢失了真实原因，把 same-graph read-before-patch
    误归成没有 state-changing output。
  - Retry exhaustion 可直接创建 decision blocker，未先区分 deterministic
    acquisition / deterministic fail / governed decision。
- Owner: WorkOrch core runtime。该问题属于中立状态机、authority snapshot、
  requiredOutput、contract gate 和 decision eligibility，不属于
  workspaceTemplate、Soul、语言、文件名、provider 或 live workspace 特判。
- Event path:
  - `resume/replan/restart`
  - owner run activation
  - convergence authority refresh
  - target snapshot preflight
  - convergenceContract
  - stagePacket/currentActionContract/requiredOutput render
  - LLM/block dispatch
  - contract validation
  - accepted/rejected evidence ledger
  - next action recompute
- Contract gap:
  - target snapshot readiness 未成为 direct `state_change` dispatch 的硬门禁。
  - `acquire_target_snapshot` 尚未作为缺 snapshot 时的确定性前置 action
    覆盖所有入口。
  - same-graph read-before-patch 的 adapter 修复输出与 orchestrator 违例分类
    没有闭环。
  - decision gate 未把 target snapshot 缺失排除在默认人工 blocker 之外。
- Boundary check:
  - 所有修复必须基于通用字段：`NextAction`、`coverageWorkItemIds`、
    `structuredTargetRefs`、`targetRef`、`baseHash`、`target_snapshot`
    evidence、capability kind、decision eligibility reason。
  - 禁止根据 template ID、Soul ID、workspace ID、项目名、语言、命令、文件名或
    provider 名称写分支。

## Design Rules

### 1. Target Snapshot Is Part Of Convergence Authority

`targetSnapshot` 不应散落在 repair context、memory、failed graph 或 LLM 自行读取
结果中。它必须成为 current convergence authority 的派生快照字段。

建议 authority snapshot 至少表达：

- `targetSnapshotStatus`: `ready | missing | stale | not_required`
- `targetSnapshotRefs`: targetRef、baseHash、contentHash、sourceRef、capturedAt
- `coverageWorkItemIds`
- `structuredTargetRefs`
- `snapshotRequiredReason`
- `snapshotFreshnessRevision`

`targetSnapshotStatus=ready` 的含义是：当前 action 可以在不依赖 same-graph read
的情况下生成可验证的 state-changing output。

### 2. State-Change Dispatch Requires Preflight

在任何 provider/block dispatch 前，若当前 action 满足以下条件，必须执行
target snapshot preflight：

- `NextAction=state_change`
- `requiredOutput.acceptedKinds` 包含 `patch/apply`
- 当前 focus 指向已有目标，或 patch contract 需要 `oldText`、`baseHash`、
  `diff`、完整 `content`
- 当前 authority snapshot 中没有可用 target snapshot

此时不得派发 state-changing LLM step。runtime 必须改派：

```text
NextAction=acquire_target_snapshot
runtimeStage=acquire_target_snapshot
runtimeStageMode=target_snapshot_acquisition
repairObjective=acquire_concrete_failure_evidence
```

### 3. Acquire Target Snapshot Is Read-Only

`acquire_target_snapshot` 是确定性只读 action。其 requiredOutput 必须表达：

- `mustAcquireCurrentTargetSnapshot=true`
- `mustUseReadOnlySnapshotAcquisition=true`
- `mustNotMutateWorkspace=true`
- accepted kinds:
  - `file/list`
  - `file/read`
  - `git/read`
  - `mcp/resource_read`
  - declared read-only `shell/command`
- forbidden kinds:
  - `patch/apply`
  - `git/write`
  - file/write
  - state-changing shell/tool call
  - approval/decision unless eligibility gate proves external blocker

该 action 成功后必须记录 accepted `target_snapshot` evidence，而不是把 read-only
graph 当作 state_change evidence。

### 4. Snapshot Evidence Must Recompute Authority

target snapshot acquisition 成功后，runtime 必须记录结构化 evidence：

- workspaceId
- runId
- stepId
- definitionRevision
- targetRef
- baseHash / contentHash
- capturedAt
- sourceRef
- coverageWorkItemIds
- structuredTargetRefs
- completeness / truncation metadata

随后重新执行：

```text
gapBaseline
  -> convergenceAuthority
  -> convergenceContract
  -> stagePacket
  -> currentActionContract
  -> requiredOutput
```

只有 recompute 后 `targetSnapshotStatus=ready`，才允许下一轮
`state_change` dispatch。

### 5. State-Changing Graph Contract Narrows After Snapshot

当 `targetSnapshotStatus=ready` 时，`state_change` requiredOutput 必须明确：

- 修改已有文件时，`patch/apply` 必须引用 `targetRef`。
- 修改已有文件时，必须提供 `baseHash`，或使用 snapshot 中可验证的 exact
  `oldText`。
- `patch/apply` 不得依赖同 graph 的 `file/read`。
- patch 后可以接 validation 或 git/write，但 patch 的适用性不能依赖同图读取。
- `git/write` 可以满足 version-control snapshot，但不能替代 primary
  `source_change` state-changing evidence。

### 6. Contract Violation Classification Must Preserve Real Cause

违例分类必须区分以下情况：

- 无任何 patch/apply、git/write 或声明的 state-changing tool：
  `state_changing_required_missing`
- 有 patch/apply，但依赖 same-graph file/read：
  `same_graph_file_read_before_patch` 或
  `graph_depends_on_llm_before_patch`
- 有 patch/apply，但缺 targetRef/baseHash/oldText/content：
  `incomplete_patch_apply_input`
- 有 patch/apply，但目标与 focus 不匹配：
  `focus_binding_mismatch`
- 有 patch/apply，但目标 snapshot 缺失：
  `target_snapshot_required`

当真实原因是 snapshot 缺失时，recoverability 应为：

```text
acquire_target_snapshot_then_retry_state_change
```

而不是继续消耗 same-contract retry budget。

### 7. Decision Gate Must Exclude Runtime-Acquirable Snapshot Gaps

target snapshot 缺失不是 human decision。只有以下结构化条件成立时才能创建
decision ticket：

- target path 无法确定，需要用户选择。
- 缺权限、凭证、外部输入或外部系统确认。
- workspace/read capability 不可用，且 runtime 无可替代 acquisition path。
- governance policy 明确要求人工确认。

否则：

- snapshot missing -> `acquire_target_snapshot`
- same-graph read-before-patch with no snapshot -> `acquire_target_snapshot`
- same-graph read-before-patch with ready snapshot -> bounded retry same contract
- retry exhausted without external blocker -> deterministic fail/block, not
  `blocked_on_decision`

## Closure Tasks

### P0. Add Target Snapshot Readiness To Authority Snapshot

Files to inspect and adjust:

- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_context.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`

Acceptance:

- Current authority snapshot exposes `targetSnapshotStatus`.
- Snapshot readiness is focus scoped.
- Stale failed-step input, memory, old graph and old repair package cannot mark
  snapshot ready.
- Snapshot diagnostics are visible as lineage but not selected as authority.

### P0. Add State-Change Dispatch Preflight Gate

Files to inspect and adjust:

- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_event.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_current_action_contract.go`

Acceptance:

- `NextAction=state_change` plus missing required snapshot is rewritten before
  dispatch to `acquire_target_snapshot`.
- No LLM request can contain `state_change`, empty target snapshot, and
  read-only acquisition forbidden at the same time.
- The rewrite occurs before provider call, so it does not consume LLM contract
  retry budget.

### P0. Implement Runtime-Managed Acquire Target Snapshot Action

Files to inspect and adjust:

- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_context_material.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_evidence.go`
- `internal/modules/blocks/file/**`
- `internal/modules/blocks/git/**`

Acceptance:

- Runtime can acquire bounded target snapshots using read-only capabilities.
- Accepted evidence is recorded as `target_snapshot`.
- Partial snapshot evidence is marked partial/truncated and does not falsely
  enable direct patch if incomplete.
- Acquisition failure with external blocker enters decision only through
  decision eligibility gate.

### P0. Recompute Authority After Snapshot Evidence

Files to inspect and adjust:

- `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_reuse.go`
- `internal/modules/kernel/orchestrator/biz/runtime_evidence_ledger*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_next_action*.go`

Acceptance:

- `target_snapshot` evidence updates focus-scoped readiness.
- Next recompute can select `state_change` with `targetSnapshotStatus=ready`.
- Later accepted patch/apply invalidates older snapshots for the same target.
- Evidence ledger never treats `target_snapshot` as `state_change`.

### P0. Correct Contract Violation Classification

Files to inspect and adjust:

- `internal/modules/blocks/llm/biz/runtime_adapter.go`
- `internal/modules/kernel/orchestrator/biz/runtime_graph_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_policy.go`

Acceptance:

- LLM response with patch/apply depending on same-graph file/read is classified
  as `same_graph_file_read_before_patch` /
  `graph_depends_on_llm_before_patch`.
- LLM response with no state-changing node remains
  `state_changing_required_missing`.
- Same-graph read-before-patch with missing snapshot routes to
  `acquire_target_snapshot_then_retry_state_change`.
- Same-graph read-before-patch with ready snapshot routes to bounded retry
  without weakening state-change requiredOutput.

### P0. Tighten Decision Eligibility

Files to inspect and adjust:

- `internal/modules/kernel/orchestrator/biz/runtime_decision*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_terminal_outcome*.go`

Acceptance:

- Missing target snapshot does not create decision ticket when runtime can read
  workspace state.
- Retry exhaustion without external blocker does not default to
  `blocked_on_decision`.
- Decision ticket context includes structured missing external dependency when
  decision is valid.
- Projection still shows decision only when同 scope open ticket exists.

### P1. Reduce Prompt Noise For Snapshot/State-Change Boundary

Files to inspect and adjust:

- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`

Acceptance:

- `state_change` request with ready snapshot surfaces current snapshot refs in
  C3/C4 current action context.
- Old failed read-only graphs stay diagnostic-only and do not compete with
  current requiredOutput.
- Memory recall that only repeats old read-only graph behavior is excluded from
  high-value current action context unless it has target snapshot provenance.

### P1. Projection Diagnostics

Files to inspect and adjust:

- `internal/modules/application/workorchd/biz/appservice/**`
- `internal/modules/kernel/orchestrator/biz/runtime_projection*.go`
- `internal/pkg/workorchport/**`

Acceptance:

- Projection can expose read-only authority diagnostics such as
  `target_snapshot_missing_before_state_change`.
- Projection does not repair owner state.
- TUI/Web do not synthesize alternate decision or snapshot state.

## Required Tests

### Unit Tests

- `state_change` with existing target and missing snapshot selects
  `acquire_target_snapshot`.
- `state_change` with ready snapshot remains `state_change`.
- New bounded creation flow can remain direct `state_change` when existing
  target content is not required.
- `acquire_target_snapshot` renders read-only requiredOutput and forbids
  mutation.
- Accepted `target_snapshot` evidence is focus-bound and can be reused only for
  matching definition revision / target.
- Accepted patch/apply invalidates same-target older snapshot.
- Same-graph file/read -> patch/apply is classified as
  `same_graph_file_read_before_patch`, not `state_changing_required_missing`.
- Missing snapshot does not pass decision eligibility when read-only capability
  is available.

### Integration Tests

- `acquire_target_snapshot -> target_snapshot evidence -> state_change` completes
  without human decision.
- Contract retry with missing snapshot does not consume same-contract LLM retry
  before acquisition.
- Replan/restart/resume all rebuild target snapshot authority from owner state,
  not from old step input.
- Projection exposes consistent run state, prompt mode and authority diagnostics.

### Live Verification

After code closure and redeploy:

1. Start/replan a workspace with existing files and missing state_change evidence.
2. Verify first relevant request selects `acquire_target_snapshot` when snapshot
   is missing.
3. Verify acquisition records accepted `target_snapshot` evidence.
4. Verify next request selects `state_change` and includes target snapshot refs.
5. Verify LLM does not need same-graph `file/read -> patch/apply`.
6. Verify no decision ticket is created unless a structured external blocker
   exists.
7. Verify accepted `patch/apply` / `git/write` evidence updates coverage and
   NextAction moves to validation or the next focus.

## Non-Goals

- Do not weaken `patch/apply` validation.
- Do not allow same-graph read-before-patch as a shortcut.
- Do not classify read-only graph as accepted state_change evidence.
- Do not add workspace/template/language/file-name/provider special cases.
- Do not use run recovery finalization as the primary fix.
- Do not make TUI/Web decide snapshot or decision eligibility locally.

## Completion Checklist

- [x] P0 authority snapshot includes focus-scoped target snapshot readiness.
- [x] P0 dispatch preflight prevents impossible state-change contracts.
- [x] P0 runtime-managed `acquire_target_snapshot` records accepted snapshot
      evidence.
- [x] P0 authority recompute consumes snapshot evidence before next dispatch.
- [x] P0 contract violation classification preserves same-graph read-before-patch
      as its own rule.
- [x] P0 decision gate excludes runtime-acquirable snapshot gaps.
- [x] P1 prompt/context reduces old read-only graph noise around current
      state_change.
- [x] P1 projection exposes diagnostics without becoming owner state.
- [x] Required unit tests pass.
- [x] Required integration tests pass.
- [ ] Live verification proves positive state-changing progress without
      defaulting to decision.
