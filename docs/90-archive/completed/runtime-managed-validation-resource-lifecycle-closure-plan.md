# Runtime Managed Validation Resource Lifecycle Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-28

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-validation-contract-gate-managed-session-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`
- `runtime-contract-retry-convergence-closure-plan.md`

## Purpose

本文用于根据最新 live verification 结果，继续收口 runtime validation、resource lifecycle、completion gate 和 high-frequency progress IO 的偏差。

当前执行链路已经证明 EventHub command、projection、convergence authority、contract violation retry 主路径基本可用：workspace 能持续 running，projection revision 对齐，LLM 违例会被记录并 bounded retry，部分 retry 后能产生可接受验证证据。

新的主要偏差不再是 replan 未触发或 projection 不刷新，而是：

- 服务、长运行进程、端口和 scratch 验证仍主要由 LLM 生成 `shell/command` harness，导致验证脚本反复失败、进程残留和 scratch 膨胀。
- accepted validation evidence 能持续增长，但 completion gate 仍可能继续扩图和 follow-up，未及时进入 deterministic assessment / complete。
- 高频 LLM stream progress 和验证 payload 写入导致 CPU/IO/RSS 持续升高。

本文将上述偏差拆成可逐一交付的功能点。所有修改必须保持 WorkOrch core 业务中立；软件开发验证偏好只可通过 workspaceTemplate / Soul / profile / policy / skill 绑定进入 runtime。

## Contract Audit

- Scope:
  - `ws-4e83810542ac4d65/run-008-replan-001`
  - `ws-7ced70e98ea50c43/run-007-replan-001-replan-001-replan-001-replan-001`
  - runtime validation, managed session, scratch lifecycle, completion gate, stream progress IO
- Observed state:
  - projection ready, `sourceEventRevision == projectedSourceRevision`.
  - `ws-4e83810542ac4d65` 持续正向推进，已有文件变更和有效验证，例如 `go test ./cmd/lsp-bridge -count=1 -v` 通过，并进入多语言 LSP 验证。
  - `ws-7ced70e98ea50c43` 从多轮 shell validation failure 与 `validation_scratch_escape` 合同违例中恢复，后续 `graph-005/001-verify-cwi-016-concurrency-and-usage-via-scratch-bounded-go-test` 通过。
  - live 环境发现 `/tmp/workorch-validation-scratch` 约 3.5GB，存在 scratch 下代理进程残留，workorch RSS 曾超过 1GB，LLM stream progress 事件密集。
- Expected state:
  - 服务类、长运行、端口、进程和 scratch 验证应优先由 runtime managed validation session 管理，而不是让 LLM 手写长 shell harness。
  - 每个 validation scratch 和子进程都必须绑定 step/run scope、TTL、resource budget 和 cleanup policy。
  - 当 coverage required evidence 已满足时，runtime 应确定性进入 assessment / complete / artifact-result，而不是继续请求 LLM 扩展验证图。
  - 高频 progress 只应作为 lightweight progress，不应强制完整 projection fan-out、payload hydrate 或持续大写入。
- Deviation:
  - `validation/managed_session` 只是合同允许的 output shape，尚未成为 runtime 可执行的默认通用能力。
  - shell validation 失败可通过 contract retry 修复，但修复成本高，且每轮可能产生额外 scratch、进程和 IO。
  - completion gate 对“证据足够后停止扩图”的约束仍不够强。
  - stream progress、payload 与 scratch lifecycle 缺少统一资源预算与回收闭环。
- Owner:
  - Core: managed validation session, resource lifecycle, completion gate, stream progress persistence policy, projection IO contract.
  - Template/Soul/policy: 软件开发场景的验证偏好，例如服务类验证优先 managed session、项目测试优先 scratch-bounded test、MCP/LSP 验证优先声明式工具调用。
  - LLM context/provider: 仅负责候选内容与违例重试，不拥有资源生命周期和 completion 判定。
- Event path:
  - LLM candidate validation graph -> contract gate -> admitted runtime validation action -> managed session / shell / MCP block -> accepted/rejected evidence -> coverage ledger recompute -> deterministic NextAction -> projection.
  - For resource cleanup: step accepted -> session created -> runtime action terminal / timeout / service shutdown -> cleanup event / diagnostic -> projection resource health.
- Contract gap:
  - 缺少 managed validation session 的通用执行模型、资源账本和 cleanup owner。
  - 缺少 validation scratch / process / port 的 scope 化生命周期状态。
  - 缺少 evidence-sufficient 后禁止继续扩图的 completion gate 测试。
  - 缺少 stream progress 持久化节流与 projection fan-out 边界测试。
- Boundary check:
  - 本文只定义通用 capability、resource、session、evidence、NextAction 和 projection 规则。
  - 不允许在 core 中加入模板 ID、Soul ID、语言、命令、端口、provider、workspace title、workspace ID 或具体文件名特判。

## Target Model

### Managed Validation Session

`validation/managed_session` 是 core 通用验证能力，不是软件开发专用能力。

```text
ValidationManagedSession
  id
  workspaceId
  runId
  stepId
  validationScratchDir
  resources[]
  requestedCapabilities[]
  timeoutPolicy
  cleanupPolicy
  evidenceContract
  status
```

Managed session 可以中立管理：

- 临时端口分配。
- 进程启动和前台运行。
- 超时、取消和退出码捕获。
- stdout/stderr、structured observation、artifact refs。
- 子进程组清理。
- scratch dir 创建、预算和回收。

LLM 不应为服务类验证手写长 shell harness。LLM 应输出声明式 session request，runtime 负责实际资源管理。

### Resource Ledger

runtime 需要记录验证资源的 scope 与生命周期。

```text
ValidationResourceRecord
  id
  workspaceId
  runId
  stepId
  sessionId
  kind              # scratch_dir | process | port | temp_file | cache_dir
  path
  pid
  port
  bytes
  status            # allocated | released | orphaned | cleanup_failed
  cleanupPolicy
  createdAt
  releasedAt
  diagnosticRef
```

规则：

- 所有 managed session 资源必须带 workspace/run/step scope。
- step terminal 后执行 step-scope cleanup。
- run terminal 后执行 run-scope cleanup。
- service startup 可以扫描 orphan resource，但只能做资源诊断或安全清理，不自动恢复业务执行。
- cleanup 失败记录 rejected/diagnostic evidence，不满足 coverage。

### Completion Gate Strengthening

当 current focus 的 required evidence 已满足：

- NextAction 必须优先为 `assessment`，不能继续 `validate`。
- assessment 通过后更新 coverage ledger。
- 所有 required coverage 满足后，NextAction 必须为 `complete` 或 `artifact/result`。
- LLM 返回额外 validation graph 时，如果不匹配当前 NextAction，应作为 rejected diagnostic evidence，不执行。

### Stream Progress IO Policy

LLM stream progress 应拆成三类：

- In-memory progress: UI 轻量刷新，不强制持久化每个 chunk。
- Persisted milestones: started, first_token, periodic heartbeat, completed, failed。
- Observability trace: 显式诊断请求时按 ref 读取完整 trace。

普通 projection 不 hydrate 高频 stream progress payload；workspace list、prompt、main view 只依赖 owner state、current step、latest milestone 和 projection revision。

## Closure Items

### M1: Managed Session Contract Model

Status: closed in current code pass.

Implementation:

- `runtime_execution_contract.go` now validates managed validation nodes as first-class contract participants after normalization.
- Managed validation requires concrete `validationEvidence`, evidence `targetRefs`, runtime-scoped `validationScratchDir`, bounded `timeoutMillis` and runtime `autoCleanup/cleanupPolicy`.
- Missing managed-session target binding or scope is rejected as contract violation / diagnostic retry input, not executed as an ambiguous shell step.

- Define neutral managed validation session input/output DTO.
- Add contract validation for required fields: scratch scope, timeout, cleanup policy, evidence target, observation type.
- Reject session requests that attempt workspace mutation unless current NextAction allows state_change through admitted state-changing action.
- Acceptance:
  - Unit test: valid managed session request is admitted for validation NextAction.
  - Unit test: workspace-mutating validation session is rejected as diagnostic-only.

### M2: Managed Session Runtime Adapter

Status: closed for the current shell-backed managed-session vertical slice.

Implementation:

- Existing `validation/managed_session` normalization continues to route through `shell/command` as the neutral bounded foreground runtime adapter.
- The shell runtime adapter now starts commands via `Start/Wait` so the step-scoped process PID can be recorded.
- Managed validation outputs now include scoped `validationResources` records for process and scratch resources.

- Implement runtime adapter/block for `validation/managed_session`.
- Provide neutral resource operations: allocate temp dir, allocate port, start foreground command/process, capture output, timeout, cleanup.
- Ensure process cleanup uses process group or equivalent safe lifecycle tracking.
- Acceptance:
  - Integration test starts a short-lived local process, captures output, and leaves no child process.
  - Timeout test terminates process and records structured diagnostic.

### M3: Scratch Resource Ledger And Cleanup

Status: partially closed.

Implementation:

- Managed validation shell execution records process and scratch resources with workspace/run/step scope.
- Step-scoped managed scratch under the system temporary directory is removed on terminal execution when `managedValidation.autoCleanup` is true.
- Cleanup failures are surfaced as resource status `cleanup_failed` and in read-only resource health projection.

Remaining:

- Persistent owner-side resource ledger repository.
- Service startup orphan scan that records stale resources as diagnostics without resuming business execution.
- Step/run terminal hooks outside the shell-backed managed-session path.

- Add resource records for scratch dirs, process pids, ports and cache dirs.
- Enforce max bytes and TTL at step/run scope.
- Add startup orphan scan for diagnostics/safe cleanup.
- Acceptance:
  - Unit test terminal step releases step-scoped scratch/process resources.
  - Unit test service startup marks stale running resource as orphaned/recoverable diagnostic.
  - Live verification confirms `/tmp/workorch-validation-scratch` does not grow unbounded across repeated validation repair.

### M4: Template Policy Binding For Validation Preference

- Add template/profile policy field for validation preference, such as `preferManagedSessionForLongRunningValidation`.
- Bind policy at workspace creation / activation into run execution context.
- Core consumes only structured policy fields and capability refs.
- Acceptance:
  - Unit test proves policy is copied from template to workspace/run context.
  - Boundary search proves core has no template/language/command/file/port special branch.

### M5: Completion Gate Evidence-Sufficient Preemption

- Strengthen NextAction interpreter so evidence-sufficient focus selects `assessment` / `complete` before any new validation/follow-up graph.
- Add contract gate rejection for LLM validation graph when current NextAction is assessment/complete.
- Acceptance:
  - Unit test accepted validation evidence changes NextAction from `validate` to `assessment`.
  - Unit test all required evidence changes NextAction to `complete` or result/artifact action.
  - Unit test extra validation graph after evidence-sufficient state is rejected and not executed.

### M6: Follow-Up Budget And No-Delta Escalation

- Bind follow-up budget to current focus and NextAction.
- Count no coverage delta after accepted validation or rejected diagnostics.
- Escalate to assessment/fail/governance according to deterministic policy instead of indefinite LLM follow-up.
- Acceptance:
  - Unit test repeated follow-up with no new accepted evidence reaches deterministic escalation.
  - Unit test focus change resets only focus-scoped budget, not run-global evidence.

### M7: Stream Progress Persistence Throttle

Status: closed for provider stream progress emission.

Implementation:

- Provider stream progress now emits first-token, 30-second heartbeat and terminal milestones instead of frequent 5-second stream progress.
- Progress still carries counts/bytes only and does not expose raw stream delta content.

- Persist stream milestones rather than every progress chunk.
- Keep full stream refs for explicit observability/debug reads.
- Ensure projection list/detail default paths do not hydrate full progress timeline.
- Acceptance:
  - Unit test stream progress emits bounded persisted events for long stream.
  - Projection test proves workspace list/main view can render from milestone state.
  - Live verification observes reduced write amplification under active LLM streams.

### M8: Resource Health Projection

Status: closed for read-only projection summary.

Implementation:

- `RuntimeExecutionState.resourceHealth` now summarizes step output `validationResources`.
- Projection reports resource counts, status counts, cleanup failure count, orphan count and latest degraded resource issue.
- Projection remains read-only and does not mutate owner state or perform cleanup.

- Expose resource diagnostics in projection/ViewModel without making projection a second state machine.
- Include scratch bytes, orphan count, cleanup failures and high IO warnings as read-only health signals.
- Acceptance:
  - Projection test shows resource health from owner/resource ledger.
  - TUI/Web can display degraded resource health without local inference.

## Code Scope

Expected areas:

- `internal/modules/kernel/orchestrator/biz/runtime_*`
- `internal/modules/blocks/**`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/**`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`
- `internal/services/workorch/**`

Potential new areas:

- managed validation session adapter/block.
- resource ledger model/repository under existing runtime ownership boundaries.
- projection DTO fields for resource health.

Must not add:

- branches based on `software-dev`, language names, test command text, port `8080`, live workspace IDs, project file names or provider names in core.
- projection repair logic that writes owner state.
- TUI/Web local resource or convergence state machines.

## Verification Plan

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/blocks/... -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|8080|LSP-Bridge|localproxy|ws-" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "nohup|setsid|kill|pkill|killall|fuser -k| &" internal/modules/kernel internal/modules/blocks internal/modules/application/workorchd/biz/bootstrap/defaults -g '*.go' -g '*.json'
rg -n "validation/managed_session|validationScratchDir|resourceHealth|stream progress|llm_stream" internal/modules internal/pkg docs/70-roadmap/active docs/30-module-architecture
```

Live verification:

1. Start latest `workorch` and record pid, binary path, commit and start time.
2. Run two active workspaces concurrently.
3. Verify projection source/projected revisions stay aligned.
4. Verify service/long-running validation uses managed session or scratch-bounded executable validation.
5. Verify no orphan process remains after terminal validation step.
6. Verify scratch size does not grow without bound across repeated repair.
7. Verify accepted evidence moves NextAction to assessment/complete when sufficient.
8. Verify stream progress write rate is bounded and projection remains responsive.

## Done Criteria

- Runtime has a business-neutral managed validation session path for service/process/port/scratch validation.
- Every validation resource has scope, lifecycle status and cleanup policy.
- Step/run/service cleanup handles process and scratch resources without relying on LLM shell scripts.
- Evidence-sufficient coverage cannot continue unlimited validation/follow-up graph expansion.
- Contract violation retry remains bounded and diagnostic-only until valid output is admitted.
- Stream progress persistence is bounded; projection does not hydrate high-frequency progress by default.
- Resource health is visible in projection as diagnostic, not repaired by TUI/Web/projection.
- Core remains business-neutral under boundary search.
