# Runtime StateChange Target Snapshot Prerequisite Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-07-08

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-terminal-ledger-nextaction-dispatch-contract-hardening-plan.md`
- `runtime-target-snapshot-statechange-preflight-closure-plan.md`
- `runtime-required-output-authority-and-target-snapshot-closure-plan.md`

## Purpose

本文用于固化 `state_change` 前置 target snapshot 合同，避免 runtime 在没有 exact patch baseline 的情况下直接派发 state-changing LLM step，导致 LLM 在以下两类合同违例之间循环：

```text
read-only graph
  -> rejected: state_changing_required_missing

same-graph file/read -> patch/apply
  -> rejected: same_graph_file_read_before_patch
```

目标是把主链路固定为：

```text
abstract coverage focus
  -> target discovery / target snapshot acquisition
  -> accepted target_snapshot evidence
  -> state_change dispatch with patch-safe current target snapshot
```

本计划只处理 WorkOrch core 通用合同，不引入 workspace、template、Soul、语言、文件名、命令、provider 或错误文本特判。

## Contract Audit

- Scope:
  - `ws-db040d24a190f6e5/run-097`
  - `ws-705249a8d7816330/run-093-replan-001-replan-001`
  - `internal/modules/kernel/orchestrator/biz/runtime_convergence.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_target_snapshot_acquisition.go`
  - `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`

- Observed state:
  - `convergenceContract.state = need_state_change` 正确。
  - `stagePacket.nextAction.type = state_change` 正确。
  - `requiredOutput.mustIncludeStateChangingNode = true` 正确。
  - Runtime 正确拒绝 read-only graph 与 same-graph read before patch。
  - Step input 中存在 `primaryStateChangeWithoutTargetEvidence=true`。
  - Step input 中没有 `targetSnapshot`、`targetSnapshots`、`currentTargetSnapshots` 或 patch-safe target snapshot。
  - `requiredOutput.targetRefs` 只有 `cwi-*`、`source_change`、`deliverable:*` 等抽象 coverage refs，不是可读 workspace-relative file path。
  - `workspaceFileManifest` 在部分 run 中存在，但没有被转换为 target discovery/acquisition 前置动作。

- Expected state:
  - `state_change` 若缺少 patch-safe current target snapshot，不得直接 dispatch 给 LLM。
  - 当 focus 只有抽象 target refs 时，runtime 必须先进入 target discovery 或 target snapshot acquisition。
  - 当存在 readable targetRef 时，runtime 必须以 read-only `file/read` 捕获 current target snapshot。
  - 当仅存在 abstract coverage refs 且缺少可推导目标时，runtime 必须 fail-fast 为 internal contract gap 或进入明确治理 blocker，不得要求 LLM 直接 state change。
  - 获取 snapshot 后，下一轮 `state_change` 必须携带 `mustUseCurrentTargetSnapshot` 与 `mustPatchDirectlyFromProvidedSnapshot`。

- Deviation:
  - `primaryStateChangeWithoutTargetEvidence=true` 没有触发 `targetSnapshotAcquisitionRequired`。
  - `stateChangeRequiresTargetSnapshotAcquisition` 只识别 readable target 或显式 `mustUseCurrentTargetSnapshot`，没有把 primary state change 缺 target evidence 作为强制前置条件。
  - LLM 被要求直接输出 state-changing patch，但没有 exact file baseline；同时 same-graph read+patch 被禁止，形成不可稳定满足的合同。

- Owner:
  - WorkOrch core runtime。
  - 不是 LLM provider、workspaceTemplate、Soul 或具体 workspace 业务特判问题。

- Event path:

```text
ledger recompute
  -> convergenceAuthority nextAction=state_change
  -> convergenceContract need_state_change
  -> repairActionPackage primaryStateChangeWithoutTargetEvidence=true
  -> StagePacket state_change
  -> requiredOutput state-changing required
  -> dispatch
  -> LLM returns read-only graph or same-graph read+patch
  -> contract violation retry loop
  -> blocked_on_decision
```

- Contract gap:
  - `primaryStateChangeWithoutTargetEvidence` 没有被提升为 `acquire_target_snapshot` 前置合同。
  - 抽象 coverage focus 到 concrete target snapshot 的转换缺少确定性状态迁移。
  - `workspaceFileManifest` 只能作为候选事实，不能替代 exact patch baseline，但当前 runtime 没有显式区分。

- Proposed change:
  - 固化 `state_change` dispatch 前置 snapshot gate。
  - 固化 abstract focus 的 target discovery/acquisition 路径。
  - 固化 acquisition accepted evidence 到 repairActionPackage/currentTargetSnapshots 的回写。
  - 固化下一轮 state_change 必须基于 patch-safe snapshot。

## Closure Principles

1. 没有 patch-safe current target snapshot，不派发 `state_change`。

2. `workspaceFileManifest` 只能用于 target discovery，不能作为 exact patch baseline。

3. 抽象 coverage refs 不能伪装为 readable target refs。

4. snapshot acquisition 必须 read-only、有限、可审计。

5. acquisition 成功后必须写入 accepted target snapshot evidence，并绑定当前 coverage focus。

6. acquisition 失败必须产生明确 blocker 或 internal contract gap，不能让 LLM 在 state_change 中自修。

## P0: StateChange Snapshot Prerequisite Gate

- [x] Add failing test: `primaryStateChangeWithoutTargetEvidence=true` derives `acquire_target_snapshot`.
  - Input:
    - `NextAction=state_change`
    - `convergenceContract.state=need_state_change`
    - `repairActionPackage.primaryStateChangeWithoutTargetEvidence=true`
    - no `targetSnapshots/currentTargetSnapshots`
  - Expected:
    - `deriveAcquireTargetSnapshotNextAction` returns `acquire_target_snapshot`
    - `requiredOutput.mustAcquireCurrentTargetSnapshot=true`
    - `requiredOutput.mustUseReadOnlySnapshotAcquisition=true`
    - `requiredOutput.mustIncludeStateChangingNode=false`

- [x] Update `stateChangeRequiresTargetSnapshotAcquisition`.
  - Treat these fields as hard prerequisite signals:
    - `primaryStateChangeWithoutTargetEvidence`
    - `primaryStateChangeCoverageScoped` with no patch-safe snapshot
    - `requiredOutput.mustUseCurrentTargetSnapshot`
    - `requiresCurrentTargetSnapshotForStateChange`
  - Do not allow abstract coverage refs to satisfy readable target checks.

- [x] Ensure `state_change` dispatch gate rejects missing snapshot before LLM call.
  - Expected:
    - no provider call for state_change when snapshot prerequisite is active
    - dispatch spec points to `acquire_target_snapshot`

Acceptance:

- No `state_change` LLM input may contain `mustIncludeStateChangingNode=true` while all target snapshot fields are empty and current focus only has abstract refs.

## P0: Abstract Focus Target Discovery

- [x] Add failing test: abstract coverage refs plus `workspaceFileManifest` enters target discovery/acquisition, not state_change.
  - Input target refs:
    - `cwi-*`
    - `source_change`
    - `deliverable:*`
  - Input has `workspaceFileManifest`.
  - Expected:
    - runtime emits read-only target discovery/acquisition contract
    - state-changing output is forbidden

- [x] Add a deterministic target discovery contract.
  - Allowed outputs:
    - selected workspace-relative target refs
    - selection reason
    - read-only acquisition graph
  - Forbidden outputs:
    - `patch/apply`
    - `git/write`
    - shell/tool mutation
    - decision request when manifest exists and discovery can proceed

- [x] Limit discovery scope.
  - Use current focus and manifest only.
  - Limit selected target refs to a small bounded number.
  - Do not inject template/language/file-name heuristics into core.

Acceptance:

- Abstract focus does not go directly to state_change.
- Manifest-backed discovery is read-only and finite.

## P0: Runtime-Managed Snapshot Acquisition Evidence

- [x] Add failing test: successful runtime-managed file/read acquisition writes accepted `target_snapshot` evidence.

- [x] Ensure snapshot evidence contains:
  - `targetRef`
  - `contentHash`
  - complete content or safe payload ref
  - `coverageWorkItemIds`
  - `structuredTargetRefs`
  - `observedClaims`
  - source step id

- [x] Ensure accepted snapshot is propagated to:
  - `run.ExecutionContext.targetSnapshots`
  - `run.ExecutionContext.currentTargetSnapshots`
  - `repairActionPackage.targetSnapshots`
  - `repairActionPackage.currentTargetSnapshots`
  - `availableTargetSnapshots` for compact LLM context

Acceptance:

- After acquisition, next authority recompute sees patch-safe snapshot and does not re-enter acquisition unless snapshot is stale.

## P0: Snapshot-Safe StateChange Dispatch

- [x] Add failing test: state_change after accepted target snapshot carries snapshot safety contract.

- [x] Required output must include:
  - `mustUseCurrentTargetSnapshot=true`
  - `mustPatchDirectlyFromProvidedSnapshot=true`
  - `sameGraphReadPatchRule=forbidden`
  - `mustDeclareCoverageTargets=true`
  - `mustBindCoverageFocus=true`

- [x] `patch/apply` validation must accept patches based on provided snapshot and reject stale/no-op patches.

Acceptance:

- LLM no longer needs to read target files inside the same state-changing graph.
- Same-graph read+patch no longer appears as the expected repair path.

## P1: Failure And Blocker Semantics

- [x] Add test: no manifest + abstract refs + no readable target produces internal contract diagnostic or precise blocker.

- [x] Add test: acquisition only finds missing optional files.
  - Expected:
    - if greenfield creation is allowed by current contract, state_change may proceed with explicit greenfield target contract
    - otherwise fail-fast/blocker

- [x] Add test: acquisition produces stale snapshot after patch.
  - Expected:
    - stale snapshot is superseded
    - next state_change requires new snapshot or fresh current target evidence

Acceptance:

- Runtime never escalates to generic decision merely because it failed to translate abstract focus into a patch baseline.

## P1: Context And Prompt Hygiene

- [x] Ensure current-action context explicitly distinguishes:
  - abstract coverage refs
  - manifest candidate refs
  - patch-safe current target snapshots

- [x] Compact old failed read-only graph noise.
  - Preserve rejected diagnostic summary.
  - Do not let old read-only graph compete with current acquisition/state_change contract.

- [x] Keep dynamic acquisition/state_change fields out of C0 stable prefix.

Acceptance:

- LLM input makes the active contract unambiguous:
  - acquire snapshot stage: read-only only
  - state_change stage: patch from provided snapshot only

## P2: Resource And Observation Guardrails

- [x] Add observation for state_change dispatch without snapshot.
  - Metric or diagnostic counter:
    - `state_change_missing_target_snapshot_preflight`
    - `target_snapshot_acquisition_requested`
    - `target_snapshot_acquisition_failed_abstract_refs`

- [x] Add live verification assertion:
  - no active or blocked run should show repeated `state_changing_required_missing` after snapshot prerequisite was available.

- [x] Add resource regression check:
  - acquisition/discovery must not repeatedly hydrate full run snapshots or scan all historical event payloads.

Acceptance:

- Live artifacts can prove whether the loop was prevented by deterministic preflight rather than by retry exhaustion.

## Verification Plan

Run targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'TargetSnapshot|StateChange|NextAction|StagePacket|ContractViolation' -count=1
```

Run package tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

Run formatting/checks:

```bash
make fmt-check
git diff --check -- . ':!docs-product'
```

Live verification criteria:

- `need_state_change` without snapshot first dispatches `acquire_target_snapshot`.
- Accepted target snapshot evidence is visible in run snapshot/evidence ledger.
- Subsequent `state_change` input contains patch-safe snapshot contract.
- No recurrence of:
  - repeated read-only graph retry
  - same-graph read+patch retry
  - generic `blocked_on_decision` for missing current file content
