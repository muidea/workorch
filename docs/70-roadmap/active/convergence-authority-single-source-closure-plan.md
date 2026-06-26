# Convergence Authority Single Source Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-26

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `convergence-authority-focus-drift-closure-plan.md`
- `convergence-authority-dispatch-closure-checklist.md`
- `../../60-verification/convergence-authority-dispatch-live-verification.md`

## Purpose

本文用于固化最新 live 核对后的长期收口方向：WorkOrch runtime 必须把
`convergenceAuthority` 收敛为唯一当前动作权威，并把所有 focus-bearing 派生字段改成
dispatch/projection 时现场生成的只读派生数据。

本计划不是继续围绕某个 live workspace 的单点 focus drift 补丁化修复，而是要求清理实现模型：

```text
Coverage/Evidence Ledger
  -> one ConvergenceAuthoritySnapshot
  -> StagePacket / requiredOutput / convergenceContract / gapBaseline
  -> candidate generation or deterministic runtime action
  -> contract gate
  -> accepted/rejected evidence
  -> ledger recompute
```

当前阶段不考虑兼容性。若已有字段、测试、projection 或端侧展示与本文冲突，应按本文收口。

## Contract Audit

- Scope: runtime convergence authority、dispatch input、Stage Packet、requiredOutput、
  contract violation retry、decision replan、projection diagnostics、TUI/Web 只读展示链路。
- Observed state:
  - 服务进程 ready，projection ready，EventHub publish failure delta 为 0，projection schedule drop delta 为 0。
  - 两个 live workspace 均稳定停在 `waiting_decision` / `blocked_on_decision`，不是 running 正向推进。
  - `ws-4e83810542ac4d65` 当前 run 为
    `run-003-replan-001-replan-001-replan-001-replan-001-replan-001-r-replan-019`。
    最新失败为 `dod_assessment_missing_evidence_refs`，projection 暴露
    `authority_focus_drift`: `coverageFocus` focus `cwi-039` differs from
    `convergenceAuthority` current focus `cwi-002`。
  - `ws-7ced70e98ea50c43` 当前 run 为
    `run-004-replan-001-replan-001-replan-001-replan-001-replan-001-replan-001`。
    最新失败为 `dod_assessment_missing_evidence_refs`，projection 暴露多个
    `authority_focus_drift`: `coverageFocus`、`repairActionPackage.requiredOutput`
    和 `stagePacket.requiredOutput` 仍绑定 `cwi-085`，而
    `convergenceAuthority` current focus 为 `cwi-023`。
  - 60 秒观察窗口内无 SSE 新事件，无 run queue，`ineffectiveRetryRuns=2`。
  - 观察窗口进程 CPU 约 27%，RSS 约 923MB；metadata scan、payload hydrate 和
    EventHub publish failure 增量为 0。
- Expected state:
  - 按 `goal-dod-nextaction-delivery-state-machine-design.md`，每次 LLM/runtime step
    dispatch 前，`Stage Packet`、current focus、NextAction、requiredOutput、
    convergenceContract 和 gapBaseline 必须来自同一次 convergence authority snapshot。
  - 按 `convergence-authority-simplification-design.md`，`coverageFocus`、
    `acceptance.currentFocus`、`repairActionPackage.requiredOutput`、top-level
    `requiredOutput` 和 `stagePacket.requiredOutput` 都是 focus-bearing 派生字段，
    必须在 dispatch 前由当前 authority focus 重建，或只能作为 diagnostic lineage 保留。
  - 如果刷新重建后仍不一致，应 fail-fast 为
    `runtime_internal_contract_misalignment`，不得把互相矛盾的合同发送给 LLM，也不得包装成普通 LLM 合同违例。
- Deviation:
  - live projection 中仍能看到 persisted derived field 与 current authority focus 不一致。
  - 当前合同违例虽然被 rejected 并升级 decision，但违例发生前的 current action/focus contract 已经不唯一。
  - 已有 `convergence-authority-focus-drift-closure-plan.md` 标记若干项为 closed，但 latest live 事实证明 closed 范围只覆盖了部分入口和部分字段组合。
- Owner: WorkOrch core。
  - 主 owner: orchestrator convergence authority、dispatch input builder、Stage Packet builder、contract violation retry。
  - 次 owner: read model projection / ViewModel diagnostics。
  - TUI/Web 不是 owner，只负责提交 scoped command 和展示 workorchd projection。
  - workspaceTemplate/Soul/policy 不是本问题主 owner；不得把 live workspace 目标、语言、文件名、命令或 provider 特判写入 core。
- Event path:
  - TUI/Web/REST command -> workorchd scoped command/EventHub -> owner state ->
    runtime scheduler -> convergence authority refresh -> dispatch input / Stage Packet ->
    LLM/block output -> contract gate -> accepted/rejected evidence ->
    coverage/evidence ledger recompute -> projection/ViewModel -> TUI/Web display。
- Contract gap:
  - 多个派生字段仍可以被持久化、继承、merge 或从旧 lineage 复制，形成第二套当前动作权威。
  - dispatch hard gate 没有在所有入口和所有 focus-bearing 字段上做到“重建后仍不一致即 fail-fast”。
  - contract violation retry / decision replan 等恢复路径仍可能携带旧 requiredOutput、旧 budget scope 或旧 repairActionPackage。
  - 测试主要证明单个 stale field 被修正，缺少覆盖 live 组合链路的矩阵断言。
- Boundary check:
  - 本计划只比较结构化 authority/focus/NextAction/requiredOutput/evidence/ticket 字段。
  - core 不允许新增模板 ID、Soul ID、语言、命令、文件名、provider、workspace title 或 live workspace ID 分支。

## Root Cause

核心缺陷不是 Goal/DoD/NextAction 合同过严，也不是 LLM 输出不稳定本身，而是实现仍保留了旧模型的多权威残留。

准确原因：

1. `convergenceAuthority` 已经开始写入，但 `gapBaseline`、`coverageFocus`、
   `convergenceContract`、`repairActionPackage`、`stagePacket`、
   `acceptance.currentFocus` 等字段仍能作为可继承、可 merge、可持久化的当前信号存在。
2. 多入口执行路径没有统一经过同一个 authority refresh 和同一个 dispatch hard gate。
   首次 run、resume、decision replan、contract retry、provider retry、follow-up 和
   event-driven dispatch 的字段刷新语义仍可能不同。
3. 旧 failed-step input、旧 replanContinuation、旧 repairActionPackage 和旧 violation
   recovery 没有被彻底降级为 diagnostic lineage。
4. Projection 正确暴露了 `authorityDiagnostics`，但 projection 只读诊断不能修复 owner state。
5. LLM 违例处理本身大体符合“rejected diagnostic evidence”的方向；当前反复违例是上游 current contract 不唯一后的结果。

## Target Model

### Single Authority Snapshot

引入或强化一个单一模型：

```text
ConvergenceAuthoritySnapshot
  workspaceId
  runId
  definitionRevision
  currentFocus
  deterministicNextAction
  acceptedEvidenceRefs[]
  rejectedEvidenceRefs[]
  missingEvidenceKinds[]
  openDecisionIds[]
  openApprovalIds[]
  focusScopedBudget
  derivedContractSeed
  idempotencyKey
  createdAt
```

该 snapshot 是唯一当前动作权威。任何当前 action/focus/requiredOutput 都必须从它派生。

### Derived Fields

下列字段不得再作为状态机输入或旧值继承源：

- `gapBaseline`
- `coverageFocus`
- `convergenceContract`
- `repairActionPackage.requiredOutput`
- top-level `requiredOutput`
- `stagePacket.activeFocus`
- `stagePacket.requiredOutput`
- `acceptance.currentFocus`
- `convergenceBudget`

允许存在的形态只有两种：

1. dispatch/projection 时从 `ConvergenceAuthoritySnapshot` 现场生成；
2. 旧值移动到 diagnostic lineage，字段名和路径必须明确表明不可作为 current authority。

### Dispatch Rule

所有 provider/block dispatch 前必须执行：

```text
RefreshConvergenceAuthority
  -> BuildDispatchInputFromAuthority
  -> ValidateDispatchAuthorityAlignment
  -> provider/block dispatch
```

`ValidateDispatchAuthorityAlignment` 必须覆盖所有 focus-bearing 字段。刷新重建后仍不一致时：

- 不调用 provider；
- 不产生普通 LLM 合同违例；
- 记录 `runtime_internal_contract_misalignment`；
- run 进入 governed failure 或 decision，原因必须指向 runtime 内部合同未对齐。

### Contract Violation Retry Rule

LLM 输出违例后：

1. rejected output 写入 rejected diagnostic evidence；
2. coverage/evidence ledger 重新计算；
3. 重新生成 `ConvergenceAuthoritySnapshot`；
4. 只有当前 `deterministicNextAction` 仍需要 LLM candidate content 时才 retry；
5. retry 的 C4 只附加违例诊断，不继承旧 focus、旧 requiredOutput 或旧 budget scope。

### Projection Rule

Projection/ViewModel 只能展示 owner state 和派生只读字段：

- current focus
- NextAction
- promptMode
- blocking ticket/approval
- latest rejected diagnostic
- authorityDiagnostics

Projection 不得修 owner state，不得用 coverage summary 或历史 result 反推新的当前 NextAction。

## Closure Scope

## Closure Progress

Last updated: 2026-06-26

- P0 Dispatch Hard Gate: code-closed.
  - Added a structured `runtime_internal_contract_misalignment` failure class.
  - Dispatch now re-validates convergence authority alignment after LLM context enrichment and before runtime requester/provider execution.
  - Alignment errors keep `common.ErrConflict` wrapping but carry the runtime-internal misalignment marker.
  - Tests now cover stale `stagePacket.requiredOutput`, stale top-level `requiredOutput`, unrebuildable pre-create focus drift preventing runtime/provider calls, and runtime-internal misalignment classification.
- P1 Single Refresh Entry: code-closed.
  - Entry-path matrix audited for first run, resume, decision replan, approval resolution, provider retry, contract retry, follow-up graph, graph batch, and event-driven dispatch.
  - `requestDispatchStep` now refreshes/rebuilds dispatch specs before publishing queued event-driven dispatch commands, so queued payloads cannot keep stale current authority fields.
  - Tests cover event-driven dispatch publishing authority-aligned queued input.
- P2 Stage Packet From Authority Only: code-closed.
  - `stagePacket.requiredOutput` is rebound from current authority focus during Stage Packet construction.
  - Assessment/governance authority states drop executable required output during Stage Packet construction.
  - Tests cover direct Stage Packet construction without relying on prior dispatch rebuild.
  - Prompt rendering suppresses the C4 acceptance-focus current signal when C3 Stage Packet already carries the active authority focus.
- P3 Contract Violation Retry Authority Recompute: code-closed.
  - Runtime repair / contract retry follow-up input finalization now normalizes focus-bearing fields from the current authority before Stage Packet attachment.
  - Tests cover stale retry `requiredOutput` rebinding at follow-up input finalization.
  - Contract violation retry budget is scoped to the current authority focus and is also copied into the retry diagnostic.
  - Contract violation retry is skipped when the current authority NextAction is governance/complete/fail/no-action instead of an LLM candidate action.
- P4 Projection Diagnostic Read Model: code-closed.
  - `authorityDiagnostics` now carries structured diagnostic metadata (`kind`, `source`, `candidate`, `currentAuthority`) so stale persisted derived fields are distinguishable from future dispatch alignment failures or historical lineage diagnostics.
  - Projection still remains read-only and does not repair owner state.
  - Tests cover stale persisted focus-bearing fields surfacing as `stale_persisted_derived_field` diagnostics.
- P5 Resource Envelope Guard: code-closed for projection rebuild burst coalescing; live verification still required.
  - Latest live observation showed scan/hydrate/rebuild growth occurs during active event bursts, not while workspaces are idle in decision.
  - Workspace detail projection rebuild scheduling now has a server-side short coalescing window, so same-workspace non-progress event bursts collapse into fewer detail snapshot rebuilds.
  - This does not change owner state semantics and does not suppress projection source revision tracking; it only delays the rebuild task slightly to batch bursts.
  - Remaining verification: after redeploy, compare active-run 20s windows for `metadata_records_scanned`, `payload_hydrates`, `workspace_projection_rebuild_total`, CPU and RSS.
- P6 Owner-State Persistence Normalization: code-closed.
  - Latest live verification found projection ready but owner `run.ExecutionContext` still persisted stale focus-bearing fields after governance blocking:
    `coverageFocus` and `repairActionPackage.requiredOutput` could differ from `convergenceAuthority.currentFocus`.
  - Root cause: previous closure normalized dispatch input, but blocked/governance run persistence paths did not normalize owner-state derived fields before repository update.
  - Run persistence now normalizes convergence owner state from the current authority/gap baseline before generic progress updates and active run/workspace state sync.
  - Tests cover stale persisted derived fields being rebound and `blocked_on_decision` persistence no longer keeping a stale focus.
- P7 Contract Violation Classification And Runtime-Normalizable Validation Setup: code-closed.
  - Latest live verification exposed two residual convergence blockers after the single-authority cleanup:
    1. Coverage assessment output could include structured open coverage gaps while being classified as
       `dod_assessment_missing_evidence_refs`, causing retries to ask for another traceable assessment instead of
       returning to the convergence authority for a current NextAction.
    2. Concrete validation repair could emit a simple scratch setup command for a non-runtime scratch path even though
       the runtime had already supplied a neutral `validationScratchDir` contract.
  - `failureClassDoDCoverageMissing` now maps to `coverage_assessment_incomplete` when the failed output carries
    structured missing goal, deliverable, or DoD coverage diagnostics. This recoverability is
    `recompute_authority`, so the path skips LLM contract retry and lets runtime repair / authority recompute decide
    the next action.
  - Concrete validation evidence output now normalizes only a simple controlled-substitute scratch setup node
    (`mkdir -p <single target>` through `mkdir`, `sh -c`, or `bash -c`) to the runtime-owned
    `validationScratchDir`. The normalization does not allow generic workspace writes, shell redirections, background
    processes, template-specific commands, or provider-specific behavior.
  - Tests cover the structured coverage-gap classification, retry skip into runtime repair, and scratch setup
    normalization before the existing validation graph safety gate runs.

### P0: Dispatch Hard Gate

Owner: orchestrator dispatch path.

Required closure:

- 扩展 `ValidateDispatchAuthorityAlignment`，覆盖：
  - `convergenceAuthority.currentFocus`
  - `gapBaseline.currentFocus`
  - `coverageFocus`
  - `acceptance.currentFocus`
  - `repairActionPackage.requiredOutput.coverageFocusId`
  - `repairActionPackage.requiredOutput.coverageWorkItemIds`
  - top-level `requiredOutput.coverageFocusId`
  - top-level `requiredOutput.coverageWorkItemIds`
  - `stagePacket.activeFocus`
  - `stagePacket.requiredOutput.coverageFocusId`
  - `stagePacket.requiredOutput.coverageWorkItemIds`
- 重建后仍不一致时 fail-fast 为 `runtime_internal_contract_misalignment`。

Acceptance:

- 构造 stale `stagePacket.requiredOutput` / stale `repairActionPackage.requiredOutput`
  fixture，断言可重建字段会被重建；构造重建后仍不一致的 fixture，断言 provider 不会被调用。
- 错误分类不是 `dod_assessment_missing_evidence_refs`，而是 runtime internal misalignment。

### P1: Single Refresh Entry

Owner: orchestrator convergence authority.

Required closure:

- 所有可能调度 step 的入口统一调用 `RefreshConvergenceAuthority`：
  - first run
  - resume
  - decision replan
  - approval resolution
  - contract retry
  - provider retry
  - follow-up graph
  - event-driven dispatch
- 禁止入口直接 merge 旧 `gapBaseline`、`coverageFocus`、`convergenceContract`、
  `repairActionPackage` 或 `convergenceBudget` 作为当前值。

Acceptance:

- 每个入口的单测都断言生成同一 shape 的 authority snapshot。
- stale queued payload 不可覆盖当前 authority snapshot。

### P2: Stage Packet From Authority Only

Owner: Stage Packet builder and LLM context renderer.

Required closure:

- `BuildStagePacket` 只消费 `ConvergenceAuthoritySnapshot` 和 diagnostic refs。
- `stagePacket.requiredOutput` 只能从当前 NextAction 生成。
- C3 只暴露一个 current focus/action contract。
- C4 不能再出现并列 current focus 信号；旧 acceptance slice 只能 diagnostic 化。

Acceptance:

- 当 C3 有 `stagePacket.activeFocus` 时，C4 无 `acceptance-focus` current signal。
- stale `repairActionPackage` 只能出现在 diagnostics，不能影响 `requiredOutput`。

### P3: Contract Violation Retry Authority Recompute

Owner: contract violation recovery and retry scheduler.

Required closure:

- 违例输出只写 rejected diagnostic evidence。
- retry 前强制重新计算 authority。
- retry budget 绑定 current focus，不继承旧 focus scope。
- 如果 current NextAction 已经不需要 LLM，则不发 retry LLM 请求。

Acceptance:

- 重复同 rule 违例后，budget escalation 使用 current focus scope。
- current NextAction 已经是 governance/complete/fail/no-action 时，不再发起 LLM contract retry。
- rejected output 没有 state-changing side effect，没有 accepted result/artifact/memory。

### P4: Projection Diagnostic Read Model

Owner: read model projection and workspace ViewModel.

Required closure:

- Projection 只读 owner authority 和派生 display fields。
- `authorityDiagnostics` 明确区分：
  - stale persisted derived field
  - dispatch alignment failure
  - projection-only historical lineage
- Workspace list、main view、promptMode 使用同一 projection revision。

Acceptance:

- 对 stale persisted fields 的 snapshot fixture，projection 显示 diagnostic，但不修复 owner state。
- TUI/Web 只展示 projection，不本地推断 decision/running/prompt mode。

### P5: Resource Envelope Guard

Owner: read model projection, detail snapshot, observability.

Required closure:

- normal projection 继续保持 metadata-only。
- detail/observability 允许 payload hydrate，但必须按需、限量、可观测。
- live verification 继续记录 CPU/RSS/metadata scan/payload hydrate/write delta。

Acceptance:

- waiting_decision 空闲窗口内 metadata scan 和 payload hydrate delta 应接近 0。
- active execution 窗口内资源指标必须与有效 event/progress 对齐。

## Test Matrix

必须补充或更新以下测试，避免后续继续按 live 单点打补丁：

| Scenario | Required assertion |
| --- | --- |
| decision replan -> dispatch | all focus-bearing fields match authority snapshot |
| contract violation -> retry | rejected diagnostic recorded, authority recomputed before retry |
| provider retry | retry does not inherit stale requiredOutput or budget scope |
| follow-up graph | old repairActionPackage is diagnostic-only |
| event-driven dispatch | queued stale payload cannot override current authority |
| Stage Packet rendering | C3 has exactly one current focus/action contract |
| prompt rendering | C4 has no competing current focus when C3 authority exists |
| projection snapshot | authorityDiagnostics exposed for stale persisted derived fields |
| hard gate | provider not called on post-rebuild focus mismatch |
| TUI/Web projection | list/main/prompt read same projection revision and promptMode |

## Verification Commands

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/llm/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelcore -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "replanContinuation|repairActionPackage|convergenceContract|gapBaseline|coverageFocus|convergenceBudget|stagePacket|requiredOutput|acceptance" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
```

Live verification:

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_OBSERVE_WORKSPACE_IDS=<workspace-a>,<workspace-b> \
WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 \
WORKORCH_OBSERVE_DURATION_SECONDS=60 \
WORKORCH_OBSERVE_REQUIRE_PROCESS_RESOURCE=1 \
make verify-convergence-authority-dispatch-live
```

## Done Criteria

- All runtime dispatch paths derive current action/focus/requiredOutput from one authority snapshot.
- No stale focus-bearing field can reach provider/block dispatch as current authority.
- Contract violation retry recomputes authority before retry and never inherits old requiredOutput.
- Projection exposes diagnostics for stale persisted fields but does not repair owner state.
- Live two-workspace verification shows no `authority_focus_drift` diagnostics for current dispatch input.
- If a runtime internal misalignment is intentionally injected, provider is not called and the run records a runtime internal contract failure.
- Core remains business-neutral; no template/language/provider/live-workspace-specific branch is introduced.
