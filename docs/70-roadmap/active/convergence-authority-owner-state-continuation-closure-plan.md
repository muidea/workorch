# Convergence Authority Owner State Continuation Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-26

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-decision-eligibility-narrow-retry-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`
- `workorchd-scheduler-continuation-diagnostics-plan.md`

## Purpose

本文固化 2026-06-26 live verification 后的下一阶段收口范围。目标是按 `Goal/DoD -> Coverage/Evidence Ledger -> Deterministic NextAction -> Owner State -> Projection/ViewModel` 的长期设计，修复当前仍存在的 owner 状态不一致、continuation 误判完成、decision resolution 无效恢复、验证能力表达不足和 projection/TUI 大对象资源压力。

本文只定义业务中立的 WorkOrch core 能力收口项。不得因为当前 live workspace、模板、语言、文件名、命令文本或 provider 现象，在 core 中加入场景特判。

## Contract Audit

- Scope:
  - live workspace:
    - `ws-4e83810542ac4d65`
    - `ws-7ced70e98ea50c43`
  - owner state, scheduler continuation, decision resolution, runtime validation contract, projection/ViewModel resource usage.
- Observed state:
  - `ws-4e83810542ac4d65`:
    - decision ticket `run-004-contract-retry-001-of-planning-001-2-repair-exhausted-decision` 已 `resolved`。
    - snapshot `openDecisions=0`。
    - run 仍为 `blocked_on_decision`，并保留同一个 `blockingTicketId`。
    - scheduler diagnostics 记录 `run_continuation completed/no_action_needed`。
    - 同一恢复窗口内事件出现 `/run/state/changed state=blocked_on_decision` 与 `/workspace/state/changed state=running` 的不一致组合。
    - resolution payload 为 `provide_missing_info`，但实际 answer 是 `/`，没有有效恢复信息。
  - `ws-7ced70e98ea50c43`:
    - runtime 已正确拒绝 LLM 返回的 validation graph。
    - 拒绝原因是 validation graph 包含后台进程启动与进程终止语义，违反 non-destructive bounded validation contract。
    - 当前阻塞不是 EventHub 丢失，而是 LLM 仍无法稳定生成安全 bounded foreground validation graph。
  - Resource observation:
    - workorch 进程 RSS 约 1.1GB，`ps` 可见 CPU 偏高。
    - 短时 pprof 未见 dispatch/EventHub 死循环，采样热点主要在 JSON 编解码和 TUI render/viewport。
- Expected state:
  - decision ticket 被 resolved 后，owner 必须确定性刷新 convergence authority，并使 run 离开旧 blocker、产生新的 dispatched step、创建 successor run，或进入明确 terminal state。
  - scheduler 只能在 owner readback 证明状态已推进后记录 continuation completed。
  - `provide_missing_info` 必须包含有效补充信息；空输入、`/` 或纯命令前缀不能关闭 ticket 并伪装成可恢复信息。
  - validation 需要临时服务、mock、端口、进程生命周期时，应由 runtime 提供业务中立的 managed validation capability，而不是让 LLM 手写 detached shell 和 cleanup。
  - projection/TUI/Web 只能展示 compact ViewModel；大对象必须按需读取，避免状态展示路径反复 JSON 编解码完整 snapshot。
- Deviation:
  - continuation completed 的判定依赖 command accepted，而不是 owner state readback。
  - decision resolved 与 run/workspace state transition 没有形成同源原子一致性。
  - TUI/Web decision input 允许空 `provide_missing_info`，导致治理对象被 resolved 但 runtime 无法获得有效恢复语义。
  - validation contract 过度依赖 LLM 自行写安全 shell，缺少 runtime-managed process/session 原语。
  - projection/TUI 路径仍可能消费或渲染过大的 runtime diagnostic payload。
- Owner:
  - WorkOrch core:
    - scheduler continuation lifecycle
    - orchestrator owner state transition
    - decision/approval governance resolution semantics
    - convergence authority refresh
    - runtime validation capability admission
    - projection/ViewModel read model
  - TUI/Web:
    - command input validation and projection display only
  - Template/Soul/policy:
    - software-dev 等业务验证偏好和 stage policy，不拥有 core 状态机
- Event path:
  - TUI/Web command -> scoped decision/run command -> workorchd scheduler/EventHub -> owner state -> convergence authority refresh -> dispatch or terminal/governance state -> projection/ViewModel -> TUI/Web display.
  - LLM candidate -> contract gate -> accepted/rejected evidence -> convergence authority recompute -> NextAction -> dispatch/governance.
- Contract gap:
  - Missing readback-confirmed continuation completion contract.
  - Missing invalid/empty decision resolution guard.
  - Missing single owner-state consistency gate when run and workspace state are published.
  - Missing managed validation process/session capability for bounded validation requiring temporary services.
  - Missing compact projection contract for high-cardinality diagnostics and large runtime payloads.
- Boundary check:
  - All planned changes use structured state, scoped IDs, decision resolution type, NextAction, evidence kind, validation capability, and projection revision.
  - No planned change may depend on live workspace IDs, template IDs, Soul IDs, language, file names, command text, provider names, or workspace titles.

## Closure Principles

1. Owner readback is mandatory for continuation completion.
2. Projection cannot repair owner state; it can only expose degraded or inconsistent diagnostics.
3. Decision resolution must be semantically valid before it closes a ticket.
4. Runtime validation should expose safe managed primitives where shell lifecycle control is unsafe.
5. LLM violations are normal input; rejected diagnostics must narrow the next action, not become accepted evidence or long-term memory.
6. TUI/Web are display/input adapters and must not infer business state from stale prompt or local cache.

## P0: Readback-Confirmed Continuation Completion

Owner: workorchd scheduler / appservice runports.

Status: done.

Problem:

- `ResumeRunView` publishes an owner command and immediately reads the run.
- orchestrator handles resume asynchronously when background routine is present.
- scheduler records continuation completed when command publish returns nil, even if the async owner operation has not advanced state.

Target behavior:

```text
schedule continuation
  -> publish owner command
  -> readback run/workspace state
  -> completed only if old blocker cleared, new step dispatched, successor run created, or terminal state reached
  -> pending/failed otherwise
```

Implementation requirements:

- Add a neutral readback verifier for run continuation result.
- A `resume` continuation with a resolved decision must not complete while the run still has the same `blockingTicketId`.
- A `replan` continuation must not complete unless a successor run exists or the original run is superseded/terminal.
- If owner command is accepted but readback still needs work, keep the pending item or record failed with a clear diagnostic reason.
- `no_action_needed` is allowed only when readback proves the requested work no longer applies.

Likely code areas:

- `internal/modules/application/workorchd/service/server/http_run_continuation_scheduler.go`
- `internal/modules/application/workorchd/biz/appservice/runports/lifecycle_start_resume.go`
- `internal/modules/application/workorchd/biz/appservice/runports/commands.go`
- `internal/modules/application/workorchd/biz/appservice/ownercommandports/publisher.go`

Acceptance:

- Unit test: resolved decision + stale blocked run does not record continuation completed. (done)
- Unit test: async accepted resume with unchanged `blockingTicketId` remains pending or failed diagnostic.
- Unit test: start/resume completion requires owner readback to prove the run advanced. (done)
- Live check: scheduler diagnostics no longer show `completed/no_action_needed` for a run still blocked on the resolved ticket.

## P1: Owner State Consistency Gate

Owner: orchestrator runtime lifecycle / run state manager.

Status: done.

Problem:

- Live events showed run state remained `blocked_on_decision` while workspace state was published as `running`.
- This breaks the single owner state contract and causes projection/TUI to display inconsistent state.

Target behavior:

```text
run state transition
  -> workspace state derived from the same loaded run state
  -> publish run/workspace events only after repository state is aligned
  -> fail fast if run state and workspace state cannot be made consistent
```

Implementation requirements:

- Add an invariant check before publishing workspace running state:
  - active run must be `running` or a state explicitly compatible with workspace running.
  - blocked run must map to `waiting_decision` or `waiting_approval`.
- `ResumeLoadedRunWithExecutionContextPatch` must not publish workspace running if `nextDispatchSpecsAfterResolvedDecision` or downstream dispatch preparation failed and run still matches original blocker.
- If resume cannot progress after a resolved ticket, owner must emit failed/pending diagnostic, not a mixed state.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_lifecycle.go`
- `internal/modules/kernel/orchestrator/biz/runtime_state_manager.go`
- `internal/modules/kernel/orchestrator/module.go`

Acceptance:

- Unit test: shared run/workspace sync rejects `blocked_on_decision` + `workspace=running`. (done)
- Unit test: run blocked states map to waiting workspace states. (done)
- Event assertion: no adjacent owner events contain `run=blocked_on_decision` and `workspace=running` for the same active run.

## P1: Decision Resolution Semantic Guard

Owner: governance command handlers / TUI/Web input adapters.

Status: done.

Problem:

- TUI accepted `/` as `provide_missing_info`.
- The ticket was resolved, but no useful recovery information was added; run stayed blocked.

Target behavior:

```text
decision input
  -> parse command or free-form answer
  -> reject empty provide_missing_info
  -> resolve ticket only with valid semantic resolution
  -> trigger owner continuation after resolution
```

Implementation requirements:

- Empty strings, `/`, command prefix-only input, and whitespace-only input are invalid `provide_missing_info`.
- If operator intends to continue without new info, expose a `resume` command path rather than `provide_missing_info`.
- `replan` remains an explicit resolution type and must trigger owner continuation through the same EventHub-driven path.
- Web and TUI must share the same port/client contract for decision resolution semantics.

Likely code areas:

- `internal/services/workorch/service/tui/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`
- `internal/modules/application/workorchd/service/server/**`
- `internal/modules/blocks/decision/**`

Acceptance:

- Unit test: `/` is rejected as missing decision info. (done)
- Unit test: valid `/decision replan` creates replan resolution and schedules continuation.
- Unit test: free-form non-empty answer can be accepted as `provide_missing_info`.
- TUI prompt remains driven by projection state after rejection.

## P1: Authority Refresh Before Every Dispatch

Owner: orchestrator convergence authority.

Status: done.

Problem:

- Resolved governance, resume, replan, provider retry, contract retry, and follow-up can still carry old dynamic fields such as old `repairActionPackage`, old `contractViolationRecovery`, old `coverageFocus`, or old `convergenceBudget`.

Target behavior:

```text
any dispatch-capable path
  -> RefreshConvergenceAuthority(workspaceId, runId)
  -> derive current focus and deterministic NextAction
  -> derive stage contract and Stage Packet from same snapshot
  -> dispatch only with aligned derived fields
```

Implementation requirements:

- `RefreshConvergenceAuthority` or equivalent single refresh path must run before:
  - resume after decision/approval
  - replan continuation
  - provider retry
  - contract retry
  - graph follow-up
  - watchdog external-timeout recovery
- Old dynamic fields can be retained only under diagnostic lineage namespaces.
- Dispatch input must fail fast if current focus, NextAction, requiredOutput, and gap baseline are not from the same authority snapshot.
- Specs-based runtime dispatch must go through the orchestrator-owned prepared dispatch entrypoint, so callers cannot bypass authority refresh/rebuild by calling the low-level dispatch helper directly.

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_lifecycle.go`
- `internal/modules/kernel/orchestrator/biz/runtime_dispatch_specs.go`
- `internal/modules/kernel/orchestrator/biz/runtime_convergence*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet*.go`

Acceptance:

- Unit test: resume/replan/contract retry rebuilds Stage Packet from current authority. (covered by existing resume/retry authority tests)
- Unit test: stale focus-bearing fields are marked diagnostic-only or overwritten before dispatch. (done)
- Unit test: `DispatchNextStep` reaches runtime with authority-prepared input and does not leak stale state-changing fields. (done)
- Unit test: dispatch fails internal contract misalignment rather than sending contradictory input to LLM. (covered by existing misalignment tests)

## P2: Managed Validation Capability

Owner: runtime block capability / validation contract renderer.

Status: done.

Problem:

- Some validation tasks require temporary services, mock upstreams, local ports, and cleanup.
- Current contract forbids detached shell and kill, but LLM is still asked to express process lifecycle using raw shell.

Target behavior:

```text
validation requiring process lifecycle
  -> LLM declares managed validation intent
  -> runtime starts bounded managed process/session
  -> runtime executes foreground observation command
  -> runtime cleanup is automatic and idempotent
  -> accepted validation evidence records observation
```

Implementation requirements:

- Add business-neutral managed validation process/session capability or extend existing runtime adapter contract. (done: `validation/managed_session` declaration normalizes to bounded foreground `shell/command`)
- Capability must support:
  - scratch directory root (done)
  - bounded timeout (done)
  - automatic cleanup (done through shell process-group timeout/completion cleanup metadata)
  - port allocation or conflict-safe port input (done through managed validation port allocation and placeholder replacement)
  - stdout/stderr capture (done through shell runtime adapter)
  - evidence binding metadata (done through `validationEvidence` and `managedValidation` output metadata)
- LLM must not need to emit `&`, `nohup`, `setsid`, `kill`, `pkill`, or `fuser -k`. (done)
- Software-dev template may prefer this capability through policy, but core capability remains generic.

Likely code areas:

- `internal/modules/blocks/shell/**`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/kernel/orchestrator/biz/runtime_validation_diagnostic.go`
- workspaceTemplate/profile policy data if a template wants to prefer the capability

Acceptance:

- Unit test: validation graph with raw background process remains rejected. (covered by existing concrete validation shell safety tests)
- Unit test: managed validation declaration is accepted and translated to bounded runtime execution. (done)
- Unit test: cleanup is idempotent and evidence is recorded with validation metadata. (done through managed cleanup metadata and shell output evidence propagation)
- No core tests depend on a specific language, provider, file name, or workspace title.

## P2: Narrow Retry Output Shape

Owner: contract violation recovery / LLM context renderer.

Status: done.

Problem:

- Forced narrow retry exists, but LLM can still produce an oversized or unsafe full graph.
- Repeated same-rule failures should reduce the allowed output shape further rather than adding more natural-language warning text.

Target behavior:

```text
same contract violation repeated
  -> rejected diagnostic evidence
  -> current NextAction preserved
  -> allowed output shape narrowed to a minimal safe schema
  -> no broad planning or global assessment unless NextAction is assessment
```

Implementation requirements:

- Same-rule validation violations must render a minimal schema that only allows safe validation nodes or managed validation declarations. (safe validation-only schema done; managed validation declaration remains covered by the separate managed validation capability item)
- Same-rule state-change violations must render a minimal patch/apply schema with current target snapshot refs. (done)
- Rejected diagnostics must enter C4 current-action only, not long-term memory.
- Retry budget exhaustion should create decision only if current authority has no automatic or managed narrowed path. (automatic narrowed path done; managed path pending with managed validation capability)

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair_action_package.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/http_provider.go`

Acceptance:

- Unit test: repeated validation violation yields scratch-only narrow validation contract. (done)
- Unit test: rejected output does not become accepted evidence/result/artifact/memory.
- Cache test: dynamic retry contract stays outside C0 stable prefix.

## P2: Compact Projection And TUI Resource Guard

Owner: workorchd projection / TUI ViewModel.

Status: done.

Problem:

- pprof showed JSON encode/decode and TUI viewport/render as visible hotspots under observation.
- Workspace list and prompt should not require full runtime diagnostics, contract violation payload, or large coverage ledger.

Target behavior:

```text
owner state
  -> compact ViewModel projection
  -> list/main/prompt consume same revision
  -> details are fetched on demand
```

Implementation requirements:

- Workspace list returns compact fields only:
  - workspace id/title/state
  - active run id
  - blocking ticket/approval id
  - projection revision
  - concise NextAction/status summary
- Large fields are available through explicit detail endpoints or on-demand snapshot fetch.
- TUI prompt, workspace list, main view, and status bar must use the same projected revision.
- TUI must not render full contractViolation/repairActionPackage unless a detail view requests it.
- Workspace projection store must compact list overview payloads even when the entry is built from workspace-list-only data without a detail snapshot.

Likely code areas:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**`
- `internal/modules/application/workorchd/service/server/**`
- `internal/services/workorch/service/tui/**`
- `internal/pkg/workorchport/**`
- `internal/pkg/workorchclient/**`

Acceptance:

- Unit test: list projection omits large runtime payloads and workspace context/policy fields. (done)
- TUI regression: prompt/list/main view are same workspace revision. (covered by existing projection ViewModel revision tests)
- Observation: repeated list/status refresh does not inflate JSON payload size with full diagnostics. (live verification pending)

## P3: Live Verification Checklist

Status: pending.

After P0/P1/P2 code closure, verify with a freshly deployed service:

1. Resolve a decision with invalid `/` input:
   - command is rejected
   - ticket remains open
   - prompt stays consistent with projection
2. Resolve a decision with valid `replan`:
   - continuation is not completed until successor run exists or original run is superseded
   - new run dispatches a step or enters a clear terminal/governance state
3. Resolve a decision with valid `provide_missing_info`:
   - run leaves old `blockingTicketId`
   - owner emits consistent run/workspace state events
4. Trigger repeated validation contract violation:
   - rejected diagnostics are recorded
   - retry narrows to safe schema or managed validation
   - decision is created only after no automatic path remains
5. Observe CPU/RSS and pprof:
   - no EventHub/dispatch spin
   - projection/list/status path avoids full large payload render
   - RSS does not grow unbounded during repeated projection updates

## Open Questions

- Whether continuation readback should poll for a bounded short window or require an explicit async completion event from orchestrator background execution.
- Whether projection should expose owner-state inconsistency as a first-class `authorityDiagnostics` field before failing health.

## Out Of Scope

- Adding software-dev-only command, language, file, benchmark, provider, or workspace-specific logic to core.
- Treating LLM contract violations as accepted evidence.
- Letting TUI/Web repair owner state.
- Using watchdog compensation to hide EventHub command loss or owner state inconsistency.
