# Runtime Decision Eligibility Narrow Retry Closure Plan

Status: implementation-closed

Type: closure-plan

Last Updated: 2026-06-26

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-contract-retry-convergence-closure-plan.md`
- `runtime-convergence-authority-final-fix-scope.md`

## Purpose

本文固化 2026-06-26 live workspace 深入核对后的下一阶段收口范围。

当前 workorchd EventHub、projection、TUI/Web ViewModel 与 snapshot display contract 已基本符合预期；剩余偏差集中在 runtime 收敛状态机：

- LLM 输出违例后，runtime 能拒绝并记录 rejected diagnostic evidence。
- 但当缺口仍可自动继续时，runtime 仍可能因为 repair/contract retry budget 耗尽而过早进入 `waiting_decision`。
- 这与长期设计中的原则存在偏差：只有缺少外部信息、权限、凭证、审批、用户选择或不可自动恢复条件时，才应进入人工 decision。

本计划只处理 WorkOrch core 的中立 NextAction、Stage Packet、contract violation recovery 和 decision eligibility；不加入 workspace ID、模板 ID、语言、命令、文件名或 provider 特判。

## Contract Audit

- Scope:
  - live workspace:
    - `ws-4e83810542ac4d65`
    - `ws-7ced70e98ea50c43`
  - runtime NextAction、Stage Packet、contract violation recovery、runtime repair exhaustion、governance auto decision eligibility。
- Observed state:
  - 两个 workspace 的 projection 均为 `ready`，snapshot projection 均包含 workspace-scoped `workspaceRevision`。
  - `displayContract.pass=true`。
  - EventHub publish failure delta 为 `0`。
  - `projectionPendingRebuilds=0`，`projectionScheduleDroppedDelta=0`。
  - 两个 workspace 都进入 `waiting_decision`：
    - `ws-4e83810542ac4d65`: `runtime_convergence_budget_exhausted`，直接错误为 `decision requests a continuable candidate choice`。
    - `ws-7ced70e98ea50c43`: `contract_violation-decision`，直接错误为 `concrete_validation_contract_missing`，validation graph 试图修改 workspace 文件或写出 validation scratch。
- Expected state:
  - 如果 current authority 仍能推导出可自动执行的 `acquire_target_snapshot`、`state_change`、`validate` 或 `assessment`，runtime 不应创建人工 decision。
  - LLM 违例只能成为 rejected diagnostic evidence；当 NextAction 仍需要 LLM candidate content 时，应进入 bounded retry 或 narrowed contract。
  - `request_decision` 只用于外部信息、权限、凭证、审批、用户选择、不可恢复预算耗尽或 runtime internal contract misalignment 无法自动修复的场景。
- Deviation:
  - `ws-4e83810542ac4d65` 暴露出 state-changing patch 需要目标快照时，runtime 没有拆成 `acquire_target_snapshot -> state_change` 两阶段，而是因同图 `file/read -> patch/apply` 禁止规则陷入人工 decision。
  - `ws-7ced70e98ea50c43` 暴露出 validation graph mutation 违例后，runtime 没有自动收窄为 read-only / scratch-only validation retry，而是创建 contract violation decision。
  - `runtime_repair_exhausted_requires_strategy` 与 contract violation exhausted 的 decision eligibility 仍偏宽，能把 continuable candidate 升级成人工治理。
- Owner:
  - WorkOrch core: orchestrator convergence authority、Stage Packet、contract gate、runtime repair、governance auto eligibility。
  - 非 owner: TUI/Web projection、workspaceTemplate/Soul/policy、LLM provider、具体 workspace 目标。
- Event path:
  - LLM candidate -> contract gate -> rejected diagnostic evidence -> convergence authority refresh -> retry/narrowed contract or governance decision。
  - runtime failure -> repair budget / contract violation budget -> decision eligibility gate -> ticket creation or automatic NextAction。
- Contract gap:
  - 缺少显式 `acquire_target_snapshot` 或等价 read-only target snapshot NextAction。
  - validation contract recovery 没有把 mutation violation 映射为更窄的 read-only / scratch-only retry。
  - decision 创建前缺少统一的 “current authority still continuable” gate。
- Boundary check:
  - 本计划只引入结构化 NextAction、evidence kind、contract violation type、decision eligibility reason。
  - 禁止在 core 中按 `ws-4e...`、`ws-7ced...`、具体文件名、Go/LSP、命令文本或 provider 判断。

## Target Runtime Flow

### State Change With Target Snapshot

当 current authority 选择 `state_change`，但当前 required output 需要精确目标快照才能安全生成 patch/apply 时，runtime 应拆成两个自动阶段：

```text
state_change needs exact target snapshot
  -> NextAction: acquire_target_snapshot
  -> execute bounded read-only graph
  -> record accepted target_snapshot / diagnostic evidence
  -> recompute authority
  -> NextAction: state_change
  -> render patch/apply-only state-changing contract
```

Rules:

- `acquire_target_snapshot` 只允许 read-only / metadata / bounded content capture。
- `acquire_target_snapshot` 不能执行 patch/apply、shell mutation 或 artifact write。
- `state_change` 可消费 accepted snapshot evidence，但不得在同一 graph 中先 file/read 再 patch/apply。
- 如果 snapshot evidence 已存在且仍有效，不重复读取。

### Validation Violation Narrow Retry

当 current authority 选择 `validate`，但 LLM 返回的 validation graph 违反只读/scratch 约束时，runtime 应生成 narrowed validation retry：

```text
validate candidate violates validation contract
  -> reject as diagnostic evidence
  -> preserve current NextAction=validate
  -> render narrowed validation contract:
       shell/command read-only
       writes only under validationScratchDir
       workspace file mutation forbidden
       patch/apply required for state changes
  -> bounded retry
```

Rules:

- validation shell 不能修改 workspace 文件。
- validation shell 只能读取 workspace 或写入 `validationScratchDir`。
- 任何文件状态变更必须走 `state_change` / `patch/apply`，不能混入 validation。
- 同一 signature 重复超预算后，只有在 current authority 无法继续生成 narrowed validation contract 时才进入 decision。

### Decision Eligibility Gate

所有 runtime 创建 decision ticket 前必须先执行统一 gate：

```text
RefreshConvergenceAuthority(workspaceId, runId)
  -> if NextAction in [acquire_target_snapshot, state_change, validate, assessment, repair]
       and required runtime capability is available
       and no external dependency marker exists
       then decision is not eligible
       route to automatic NextAction / narrowed retry
  -> else request_decision
```

Decision eligible conditions:

- 缺少外部信息、用户选择、权限、凭证、配置或审批。
- provider / external service 故障达到不可自动恢复条件。
- runtime internal contract misalignment fail-fast，且刷新 authority 后仍不一致。
- current authority is `request_decision` with structured blocker reason.
- run/focus budget 已耗尽，且没有任何可自动 NextAction 或 narrowed retry contract。

Decision not eligible conditions:

- `decision requests a continuable candidate choice`。
- `coverage_assessment_incomplete` with `recoverability=recompute_authority`。
- `concrete_validation_contract_missing` where narrowed validation retry can be rendered。
- state_change 缺少 target snapshot but runtime can acquire bounded read-only snapshot。
- repair exhausted alone, without structured external blocker.

## Closure Scope

Implementation status: the scoped P0/P1/P2 runtime changes in this document have been closed in code. P3 live verification remains an operator-run validation activity after deploy; it does not require additional production logic unless the live run exposes a new audited deviation.

## 2026-06-26 Closure Notes

已完成：

- P1/P2 主路径收口：
  - `contract_violation_exhausted` 在 current action 仍可自动 retry 时，不再直接创建人工 decision。
  - `runtime_repair_exhausted` 在失败原因仍是 continuable candidate 或 current NextAction 仍可自动推进时，不再直接创建人工 decision。
  - 通过 `decisionEligibilityDenied` / `decisionNotEligibleReason` / `decisionEligibilityGate` 标记自动放行的 retry；同一失败如果已经经过该 gate retry，再次失败允许升级 decision，避免无限循环。
  - 服务重启后的 `nextDispatchSpecsFromStep` 路径同步支持 `contract_violation_retry` 与 `contract_violation_exhausted`，避免只修运行时主路径。
- P0 基础 schema/contract 收口：
  - runtime contract schema 已增加中立 `acquire_target_snapshot` NextAction type。
  - convergence contract 已能识别 `acquire_target_snapshot` 并映射为 target snapshot acquisition 阶段。
  - contract violation retry allow-list 已包含 `acquire_target_snapshot`。
- P0 authority 自动选择收口：
  - 当 `state_change` 明确声明需要 current target snapshot 且当前 input/repairActionPackage 没有可用 target snapshot 时，convergence contract 会先转入 `target_snapshot_acquisition`。
  - `NextActionForRun` 在相同条件下返回 `acquire_target_snapshot`，并禁止在快照获取前直接执行 `state_change` / `complete` / `request_decision`。
  - 当 `targetSnapshots` / `currentTargetSnapshots` 已存在时，convergence 仍保持 `state_change`，避免重复读取。
- P0 accepted target snapshot evidence ledger 收口：
  - runtime contract schema 已增加中立 accepted evidence kind: `target_snapshot`。
  - completed `file/read` step 会在 evidence ledger 写入 accepted `target_snapshot` evidence record，记录 workspace/run/step/sourceRef/graph root/node、targetRef、contentHash、size、完整性和可安全写入状态。
  - 只有完整、非截断、非脱敏、非二进制、`safeForFileWrite=true` 的 read-only snapshot 会被接受；不安全输出不会升级为 accepted evidence。
  - evidence record 只保存 hash/ref/trace 元数据，不保存完整 content，避免把大对象写入长期 evidence metadata。
  - subsequent `state_change` 的完整内容消费仍通过 graph follow-up 从已完成 `file/read` step output 重建 `targetSnapshots/currentTargetSnapshots`；evidence record 作为审计与恢复索引，不直接替代完整 step output。
  - `NextActionForRun` 在计算 missing `state_change` 前，会基于 accepted `target_snapshot` evidence record 找到 source step output，重新校验完整性、hash、definition revision 与非脱敏/非截断状态，然后临时恢复 `targetSnapshots/currentTargetSnapshots`，避免服务重启或 graph follow-up 输入丢失后重复进入 `acquire_target_snapshot`。
  - 该恢复只作用于当前 authority 计算，不持久化写回 run executionContext，避免把快照内容长期固化或扩大过期窗口。
  - 快照有效性已接入 runtime-managed patch lineage：如果 accepted snapshot 捕获后同一 target 存在未回滚的 applied patch，runtime 会拒绝复用该 snapshot，并重新进入 `acquire_target_snapshot`。
  - target snapshot evidence metadata 已记录 `capturedAt`，用于后续 lineage/过期判定；缺少 applied patch timestamp 时按保守 stale 处理。
- Projection 可观测性收口：
  - `ConvergenceStatus` 已暴露 `decisionEligibilityDenied`、`decisionNotEligibleReason`、`decisionNotEligibleStepId`。
  - 字段来源只读取 runtime step input 中的结构化 gate 结果，不由 Web/TUI/read model 自行推断 decision eligibility。
- P3 观测脚本收口：
  - `observe-workorch-runtime-closure.sh` 的 workspace summary 已输出 decision eligibility denied/reason/stepId。
  - 当 workspace 存在 open decision 但 projection 表明 decision eligibility 已被 runtime gate 拒绝时，观测脚本会报错，避免 live verification 忽略“不应进入 decision”的偏差。

后续独立收口项：

- runtime-managed state change 范围内的 target snapshot 过期判定已完成。若未来需要覆盖绕过 WorkOrch patch/evidence ledger 的外部文件系统改动，需要引入 workspace filesystem revision 或 file watcher；该项属于独立环境一致性能力，不应由当前 decision eligibility closure 继续扩散。

### P0: Target Snapshot Acquisition NextAction

Owner: orchestrator convergence authority / Stage Packet.

Closure status: complete.

Implemented closure:

- Add a neutral `acquire_target_snapshot` or equivalent structured NextAction.
- Map state-change contract failures that need exact target content to this NextAction.
- Record accepted target snapshot evidence with source refs and bounded content metadata.
- Recompute authority before state-changing patch/apply.

Acceptance coverage:

- Unit test: continuable candidate caused by missing exact oldText does not create decision.
- Unit test: runtime emits read-only target snapshot action before patch/apply.
- Unit test: accepted snapshot evidence allows subsequent state_change contract to omit same-graph read.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`

### P1: Narrowed Validation Retry

Owner: contract violation recovery / validation contract renderer.

Closure status: complete.

Implemented closure:

- Classify validation graph workspace mutation and scratch escape as recoverable validation contract violation.
- Preserve `NextAction=validate` when current authority still needs validation evidence.
- Render narrowed retry contract that explicitly requires read-only / scratch-only validation.
- Prevent validation retry from becoming state-changing output unless authority switches to `state_change`.

Acceptance coverage:

- Unit test: validation shell mutation violation does not create decision on first recoverable retry.
- Unit test: retry contract forbids workspace writes and only allows scratch writes.
- Unit test: repeated same validation mutation signature escalates only after bounded retries and only if no narrowed retry remains available.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`

### P2: Unified Decision Eligibility Gate

Owner: runtime repair / governance auto escalation.

Closure status: complete.

Implemented closure:

- Add a shared pre-ticket eligibility check for runtime repair exhausted and contract violation exhausted paths.
- Gate must refresh authority and reject ticket creation when an automatic NextAction remains available.
- Expose structured diagnostic when decision creation is denied because current action remains continuable.
- Governance auto service should not depend only on replan lineage depth when current authority can continue automatically.

Acceptance coverage:

- Unit test: `runtime_repair_exhausted` with continuable NextAction routes to automatic action, not ticket.
- Unit test: `contract_violation_exhausted` with `concrete_validation_contract_missing` and validation capability routes to narrowed retry, not ticket.
- Unit test: true external dependency still creates decision.
- Projection test: ViewModel shows `promptMode=running` or current automatic action, not `decision`, when ticket is denied.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_repair.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_execution.go`
- `internal/modules/kernel/governance/biz/service.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`

### P3: Live Verification And Regression Fixtures

Owner: verification scripts / targeted integration tests.

Closure status: complete.

Implemented closure:

- Add two fixture-style tests matching the observed live failure classes without live workspace IDs.
- Extend runtime observation script to summarize open decision eligibility reason when workspaces enter waiting decision.
- Live acceptance after deploy:
  - replan or resume both workspaces;
  - neither workspace should immediately enter `waiting_decision` for continuable candidate;
  - at least one automatic follow-up step should be dispatched for target snapshot acquisition or narrowed validation retry.

Acceptance:

- `displayContract.pass=true`
- `projectionPendingRebuilds=0`
- `runtimeEventPublishFailures.totalPublishFailureDelta=0`
- No open decision where `decisionNotEligibleReason=continuable_next_action_available`
- Workspace either progresses to accepted evidence or reaches a decision with external blocker evidence.

Likely code areas:

- `scripts/observe-workorch-runtime-closure.sh`
- `internal/modules/kernel/orchestrator/biz/*_test.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/*_test.go`

## Verification Plan

### Targeted Tests

```bash
go test ./internal/modules/kernel/orchestrator/biz -run 'Continuable|DecisionEligibility|TargetSnapshot|Validation.*Narrow|ContractViolation|RepairExhausted' -count=1
go test ./internal/modules/kernel/governance/biz -run 'RuntimeRepairExhausted|AutoReplan|DecisionEligibility' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'Decision|Convergence|ViewModel' -count=1
```

### Boundary Search

```bash
rg -n "ws-4e83810542ac4d65|ws-7ced70e98ea50c43|cmd/lsp-bridge|main.go|go test|gopls|DeepSeek|provider=" internal/modules/kernel internal/modules/application/workorchd internal/pkg -g '*.go'
rg -n "acquire_target_snapshot|decisionNotEligible|continuable_next_action|concrete_validation_contract_missing|validationScratchDir" internal/modules/kernel internal/modules/application/workorchd internal/pkg -g '*.go'
```

### Live Verification

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_OBSERVE_DURATION_SECONDS=30 \
WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 \
WORKORCH_OBSERVE_REQUIRE_PROCESS_RESOURCE=1 \
WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 \
WORKORCH_OBSERVE_MAX_PENDING_REBUILDS=10 \
WORKORCH_OBSERVE_MAX_PROJECTION_SCHEDULE_DROPPED_DELTA=0 \
./scripts/observe-workorch-runtime-closure.sh
```

## Done Criteria

- Continuable candidate 不再因为 repair/contract budget exhausted 直接进入人工 decision。
- 需要目标快照的 state_change 自动拆成 read-only snapshot acquisition 与 patch/apply。
- validation graph mutation 违例自动收窄为 read-only / scratch-only validation retry。
- `request_decision` 只在结构化 external blocker 或无法自动恢复条件下出现。
- Projection/TUI/Web 继续只展示 workorchd ViewModel，不本地推断状态。
- Core 保持业务中立，没有新增模板、语言、命令、文件名、provider 或 live workspace ID 特判。
