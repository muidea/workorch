# Convergence Authority Simplification Design

Status: active

Type: closure-design

Last Updated: 2026-06-25

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `../../30-module-architecture/mvvp-workspace-viewmodel-architecture.md`
- `../../30-module-architecture/eventhub-driven-workorchd-state-loop-design.md`
- `runtime-convergence-authority-final-fix-scope.md`
- `goal-dod-nextaction-iterative-delivery-plan.md`

## Purpose

本文用于按最新方向重新清点 WorkOrch 收敛链路，并给出长期收口设计：减少并统一 runtime 收敛状态权威，避免 `coverage/evidence ledger`、`gapBaseline`、`coverageFocus`、`convergenceContract`、`stagePacket`、旧 `repairActionPackage` 和 projection 各自成为状态机。

目标不是继续叠加更多补丁，而是把执行链路收拢为一个业务中立、可审计、可测试的状态闭环：

```text
Goal/DoD Definition
  -> Coverage/Evidence Ledger
  -> Current Focus
  -> Deterministic NextAction
  -> Derived Stage Contract / Stage Packet
  -> Candidate Generation or Runtime Action
  -> Contract Gate
  -> Accepted/Rejected Evidence
  -> Ledger Recompute
  -> Projection/ViewModel
```

当前阶段不考虑兼容性。若现有字段、测试或端侧命令与本文冲突，应按本文收口。

## Contract Audit

- Scope: runtime convergence、replan/resume、LLM contract retry、projection/ViewModel 与 TUI/Web 展示链路。
- Observed state:
  - live workspace 曾出现 projection `gapBaseline.currentFocus=cwi-001/evidence_ready`、`nextAction=assessment`，但 run execution context 仍残留 `coverageFocus=focus-cwi-039`、`convergenceContract=need_evidence/acquire_concrete_failure_evidence`。
  - live workspace 进入 `waiting_decision` 后，replan/resume 可以触发命令提交，但后续仍可能围绕旧 repair lineage、旧 budget 或旧 requiredOutput 重复失败。
  - TUI/Web 展示曾出现 prompt、workspace list、主视图与真实 workspace 状态不一致，本质上是端侧消费 projection，但 projection 本身可能读取了不一致的派生字段。
  - LLM 输出违例是常态，runtime 若把违例恢复、旧 repair 包或旧自然语言历史当作下一动作权威，会导致无限 retry、反复 decision 或低质量推进。
- Expected state:
  - WorkOrch core 只根据结构化 ledger 与治理状态推导唯一 `NextAction`。
  - `convergenceContract`、`stagePacket`、`gapBaseline` 和 LLM input 都由同一次收敛快照派生。
  - TUI/Web/CLI 只提交 scoped command，并展示 workorchd ViewModel，不持有业务状态机。
  - LLM 违例只能成为 rejected diagnostic evidence，不能直接改变 owner state、accepted evidence、artifact/result 或 memory。
- Deviation:
  - 当前实现仍存在多个“像权威”的字段，导致同一 run 内可能出现 current focus、budget、contract、projection next action 不一致。
  - replan/resume/contract retry 等入口可能继承旧 repair 上下文，使当前 ledger 已经变化后仍按旧目标推进。
  - projection 为了展示可用信息做了局部归一化，但 projection 不应成为第二套修复状态机。
- Owner: WorkOrch core。问题位于中立 runtime convergence authority、event-driven owner state、read model projection 与端侧访问边界，不属于某个 workspaceTemplate、Soul、语言、provider 或工具特判。
- Event path:
  - TUI/Web/CLI command -> workorchd command API/EventHub -> owner state -> runtime scheduler -> orchestrator convergence -> step input/contract -> block/LLM output -> contract gate -> evidence ledger -> convergence recompute -> projection/ViewModel -> TUI/Web display。
- Contract gap:
  - 缺少单一收敛权威对象和单一刷新入口。
  - 派生字段未明确禁止作为下一轮状态判断输入。
  - 各入口对 `resume/replan/restart/decision/approval/provider retry/contract retry` 的状态刷新语义不一致。
- Boundary check:
  - 本设计只使用结构化状态、证据类型、治理状态、scope、幂等键和合同类型。
  - core 不允许加入模板 ID、Soul ID、语言、文件名、命令、provider、workspace title 或 live workspace ID 分支。

## Simplified Authority Model

### Authoritative State

下列数据是收敛权威，只能由 owner state / runtime 写入：

| Authority | Role | Write owner | Read consumers |
| --- | --- | --- | --- |
| Workspace definition revision | 当前 Goal/DoD/template/profile/policy 绑定 | workorchd appservice | runtime, projection, LLM context renderer |
| Coverage ledger | 每个 Goal/DoD work item 的状态与缺口 | runtime coverage owner | NextAction interpreter, projection |
| Evidence ledger | accepted/rejected evidence、sourceRef、quality、diagnostic | runtime evidence owner | coverage ledger, projection, LLM C4/C5 |
| Current focus | 当前应收敛的 coverage work item | convergence authority | Stage Packet builder, budget, projection |
| Deterministic NextAction | 当前唯一下一动作 | convergence authority | scheduler, Stage Packet builder, projection |
| Run/step state | run、step 生命周期 | runtime scheduler | projection, governance |
| Ticket/approval state | decision/approval 等治理对象 | governance owner | NextAction interpreter, projection |
| Focus-scoped budget | 当前 focus 的 retry/no-delta/violation budget | convergence authority | escalation gate |

### Derived State

下列数据必须每次从权威快照派生，不能反向驱动状态机：

| Derived field | Derivation source | Rule |
| --- | --- | --- |
| `gapBaseline` | definition + coverage/evidence ledger + current focus + NextAction | 可持久化用于审计和投影，但 dispatch 前必须重建 |
| `convergenceContract` | NextAction + current focus + accepted/rejected evidence + policy | 不能读取旧 repair 包决定当前 state |
| `stagePacket` | NextAction + convergenceContract + scope + capability refs | 每个 step input 前重建 |
| `repairActionPackage.requiredOutput` | convergenceContract | 只表达当前 narrow action |
| LLM C3/C4/C5 input | Stage Packet + current progress + diagnostics | 不能让旧动态字段污染 C0/C1/C2 |
| Projection/ViewModel | owner state snapshot + derived display fields | 只读派生，不修 owner state |

### Diagnostic Lineage

下列数据只可作为诊断、审计或 retry hint，不能作为当前动作权威：

- old `repairActionPackage`
- old execution graph
- `replanContinuation`
- `contractViolationRecovery`
- rejected LLM output
- previous decision question/answer
- governance history
- compacted historical summaries
- memory recall
- provider retry diagnostics

## Unified Runtime Flow

### 1. Command Ingress

所有入口必须只产生 scoped command，不直接改 runtime 内部派生字段：

```text
TUI/Web/CLI/agent
  -> workspaceId/runId/ticketId/approvalId scoped command
  -> workorchd owner API/EventHub
  -> command accepted/rejected result
```

要求：

- `/run resume` 是继续执行、首次激活、服务重启后激活、decision/approval 释放后的统一入口。
- `/run restart` 表示创建 replacement run，并从最新 definition revision 重新开始。
- `replan` 是 decision resolution 的一种 payload，必须进入 owner state 后触发当前 run 的收敛刷新。
- 不保留独立 `continue` 语义；若端侧仍有旧入口，只能作为 alias 路由到 `resume`，不得产生第二套流程。

### 2. Authority Refresh

runtime 在每次可能调度新 step 前执行单一刷新函数：

```text
RefreshConvergenceAuthority(workspaceId, runId)
  load latest definition revision
  load current coverage ledger
  load accepted/rejected evidence refs
  load open governance objects
  select current focus
  derive deterministic NextAction
  reset or update focus-scoped budget
  derive convergenceContract
  derive gapBaseline
  derive stagePacket seed
  persist one aligned convergence snapshot
```

该函数是唯一允许同步更新 `coverageFocus`、`convergenceBudget`、`nextAction`、`gapBaseline`、`convergenceContract` 派生快照的入口。其他代码不得局部 patch 这些字段。

### 3. Dispatch

dispatch 只读取刷新后的 authority snapshot：

```text
authority snapshot
  -> Stage Packet
  -> step input
  -> block/LLM/runtime adapter
```

如果 NextAction 不需要 LLM 生成内容，例如已有足够证据可完成 assessment gate 或需要治理等待，则 runtime 不应继续请求 LLM 生成“自我判断”。

### 4. Candidate Gate

LLM 或 block 输出先进入合同门禁：

```text
candidate output
  -> parse/schema gate
  -> action-specific contract gate
  -> side-effect admission gate
  -> accepted evidence or rejected diagnostic evidence
```

要求：

- 未通过合同的输出不能执行、不能写 artifact/result、不能成为 accepted evidence、不能成为高价值 memory。
- 违例可以进入 rejected diagnostic evidence，并在当前 NextAction 仍需要 LLM content 时做 bounded retry。
- 若 ledger 已能决定下一步，runtime 必须直接刷新并推进，而不是让 LLM 重述状态转移。

### 5. Projection

projection 只读 owner state：

```text
owner snapshot
  -> projection builder
  -> workspace ViewModel
  -> REST/SSE
  -> TUI/Web display
```

projection 可暴露 `authorityDiagnostics`，例如“persisted derived field stale”，但不能修复或写回 owner state。

## Access Inventory

### External Client Access

| Access | Expected role | Involved code |
| --- | --- | --- |
| TUI local backend | 提交 scoped command，订阅 ViewModel；不维护业务状态 | `internal/services/workorch/**`, `internal/pkg/workorchport/**` |
| TUI remote backend | 通过 REST/SSE 访问 workorchd；无 remote client 时不启动额外处理 | `internal/pkg/workorchclient/**`, `internal/services/workorch/**` |
| Web REST | 提交 workspace/run/decision/approval command | `internal/modules/application/workorchd/service/server/**`, `web/**` |
| Web/TUI stream | 只消费 projection/ViewModel stream | `internal/modules/application/workorchd/service/server/**`, `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/**` |

### Owner State Access

| Access | Read | Write | Design rule |
| --- | --- | --- | --- |
| Workspace definition | runtime, projection, LLM context | workorchd appservice | 形成 revision，不被 LLM 输出直接改写 |
| Run/step repository | scheduler, projection | runtime owner | scope + idempotency 必须完整 |
| Coverage ledger | convergence authority, projection | runtime coverage owner | `currentFocus` 与 `NextAction` 从这里推导 |
| Evidence ledger | coverage ledger, LLM context, projection | contract gate/evidence recorder | accepted/rejected 分账 |
| Ticket/approval | NextAction interpreter, projection | governance owner | 端侧只 resolve/approve/reject |

### Derived Runtime Access

| Derived access | Producer | Consumer | Closure requirement |
| --- | --- | --- | --- |
| `gapBaseline` | authority refresh | projection, LLM input | dispatch 前重建，不能作为权威输入 |
| `convergenceContract` | authority refresh | Stage Packet, contract gate | 从 NextAction 生成 |
| `repairActionPackage` | Stage Packet/contract renderer | LLM C4, contract gate | 只表达当前 requiredOutput |
| `stagePacket` | Stage Packet builder | LLM input/block adapter | 不继承旧 dynamic field |
| `coverageFocus` | authority refresh | budget, contract renderer | focus scope 变化时 reset budget |
| `convergenceBudget` | authority refresh/budget recorder | escalation gate | 只对当前 focus 生效 |

### LLM Context Access

| Partition | Allowed content | Must not contain |
| --- | --- | --- |
| C0 stable contract | 长期稳定 runtime schema、语言规则、中立合同 | workspace/run/step、当前失败、repair mode、provider retry、Goal/DoD 正文 |
| C1 definition | 当前 Goal/DoD revision、definition hash | transient failure、old repair lineage |
| C2 capability | 声明式 capability/profile/policy refs | 当前阶段临时允许/禁止项 |
| C3 stage contract | NextAction、stagePacket、requiredOutput | 旧 step 的 authority 字段 |
| C4 current action | 当前 progress、current focus、diagnostics、rejected evidence | 可长期召回但无 provenance 的历史流水 |
| C5 large refs | snapshots、artifact refs、trace refs | 被截断却标记为完整可 patch 的内容 |

## Code Scope

### Must Consolidate

| Area | Current risk | Target |
| --- | --- | --- |
| `runtime_gap_baseline.go` | baseline 与 NextAction 局部生成 | 成为 authority refresh 的一部分或只做派生 builder |
| `runtime_convergence.go` | contract 可能读取旧 input/continuation | contract 只由 NextAction + current focus 驱动 |
| `runtime_convergence_budget.go` | budget 与 focus 可漂移 | budget scope 绑定 current focus |
| `runtime_coverage_context.go` / `runtime_coverage_state.go` | work item 状态与 action 映射分散 | 提供唯一 structured NextAction interpreter |
| `runtime_step_input.go` / `runtime_stage_packet.go` | step input 可能继承旧动态字段 | step input 只消费最新 authority snapshot |
| `runtime_repair*.go` | repair/replan 可能把 lineage 当 authority | lineage 只进 diagnostics |
| `runtime_contract_violation*.go` | 违例恢复可能生成新 authority | rejected diagnostic + bounded retry |
| `runtime_run_state.go` / dispatch code | 多入口调度语义不统一 | 全部入口先 refresh 再 dispatch |
| `readmodelprojection/coverage_summary.go` | projection 可能二次推导 next action | 只展示 owner NextAction，冲突只诊断 |
| `workspaceviewcore/store.go` | ViewModel 聚合可能读取不一致快照 | 基于 owner projection revision 输出 |
| `internal/pkg/workorchport/workspace.go` | DTO 需暴露 structured NextAction/diagnostics | 端侧不本地推断 |

### Should Remove or Downgrade

- 将 persisted `schedule_step` 作为 runtime 权威 next action 的逻辑。
- 从旧 `repairActionPackage`、`replanContinuation`、旧 graph、旧 decision text 反推当前 requiredOutput 的逻辑。
- projection 对 owner state 进行“修复式归一化”的逻辑；可保留为 diagnostic，但最终应由 owner refresh 消除冲突。
- TUI/Web 端对 decision/approval/run/coverage 状态的本地推断。
- 非 EventHub/owner command 主路径上的直接 runtime 状态变更。

## Iterative Delivery Plan

### S1: Authority Snapshot Model

- Define neutral `ConvergenceAuthoritySnapshot` in orchestrator core.
- Fields: definition revision, current focus, next action, accepted/rejected evidence refs, open governance, budget scope, derived contract seed.
- Acceptance: unit test proves one snapshot can generate `gapBaseline`、`convergenceContract`、`stagePacket` without reading old repair lineage.

### S2: Single Refresh Entry

- Implement `RefreshConvergenceAuthority` and route dispatch/replan/resume through it.
- Remove scattered updates of `coverageFocus`、`convergenceBudget`、`gapBaseline`、`convergenceContract` where possible.
- Acceptance: stale focus/contract fixture is corrected before dispatch.

### S3: NextAction Interpreter

- Make structured NextAction the only action selector.
- Cover `assess_gap`、`state_change`、`validate`、`assessment`、`request_decision`、`request_approval`、`complete`、`fail`。
- Acceptance: no generic `schedule_step` persists as authority.

### S4: Stage Contract From NextAction

- Rebuild `convergenceContract` and `repairActionPackage.requiredOutput` from NextAction.
- Keep old packages only under diagnostic lineage.
- Acceptance: `evidence_ready` cannot generate `need_evidence` contract.

### S5: Candidate Violation Handling

- Persist invalid LLM output as rejected diagnostic evidence.
- Retry only if current NextAction still needs LLM content and retry budget allows.
- Acceptance: repeated same-rule violation escalates with focus-scoped budget and no side effects.

### S6: Projection/ViewModel Alignment

- Projection reads owner structured NextAction.
- Add `authorityDiagnostics` for mismatch during transition.
- TUI/Web prompt and main view consume same ViewModel field.
- Acceptance: decision/replan/resume changes update list, main view and prompt consistently.

### S7: Access Boundary Cleanup

- Remove local TUI/Web state machine remnants.
- Ensure remote mode is on-demand.
- Ensure all command paths carry workspace/run/ticket/approval scope.
- Acceptance: local/remote/web command tests hit the same owner command handler.

### S8: Regression and Boundary Gate

- Add fixtures for stale focus/contract, replan successor, LLM violation retry, projection mismatch.
- Run core business-neutral boundary search.
- Acceptance: tests fail on old drift class and pass after closure.

## Verification Plan

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/services/workorch/... ./internal/pkg/workorchport/... ./internal/pkg/workorchclient/... -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "replanContinuation|repairActionPackage|convergenceContract|gapBaseline|coverageFocus|convergenceBudget|stagePacket" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "schedule_step|continue current run|/run continue|Intent\\(\"continue\"\\)" internal/services internal/modules internal/pkg web
```

Live verification after code closure:

1. Start fresh workorchd and record binary path, pid, start time, git commit.
2. Pick two workspaces with different goals.
3. Trigger `resume` or decision `replan`.
4. Verify event stream shows command accepted -> authority refresh -> dispatch or governed wait.
5. Verify projection ViewModel shows the same NextAction in workspace list, main view and prompt.
6. Verify LLM contract violation becomes rejected diagnostic evidence and bounded retry, not accepted output.
7. Verify CPU/IO does not increase due to projection full scans or repeated history full reads.

## Done Criteria

- One owner refresh path determines current focus, NextAction, budget scope and derived contract before every dispatch.
- `gapBaseline`、`convergenceContract`、`stagePacket`、`repairActionPackage` are derived from the same snapshot.
- Old repair/replan/violation lineage can influence diagnostics but cannot override current NextAction.
- TUI/Web/CLI only access WorkOrch through scoped commands and projection/ViewModel.
- Projection no longer hides owner-state conflicts; any conflict is visible as diagnostic until owner refresh removes it.
- Core remains business-neutral and contains no template/language/provider/workspace-specific behavior branches.

## Closure Progress

| Item | Status | Evidence |
| --- | --- | --- |
| S1 Authority Snapshot Model | code-closed | Runtime now writes `convergenceAuthority` with current focus, structured NextAction, evidence refs and budget scope. `TestConvergenceAuthoritySnapshotDrivesBaselineContractAndStagePacket` covers authority-driven baseline/contract/stage packet derivation. |
| S2 Single Refresh Entry | code-closed | `ensureGapBaselineBeforeDispatch` routes through `refreshRunConvergenceAuthority` before dispatch and persists one aligned snapshot. `initialStepInput` also refreshes authority before rendering the first step input, so replan successor startup cannot bypass the authority snapshot. |
| S3 NextAction Interpreter | code-closed | `BuildConvergenceContract` accepts current structured NextAction from `convergenceAuthority` / `gapBaseline` without requiring replan flags; runtime no longer treats `schedule_step` as a valid first-run action. |
| S4 Stage Contract From NextAction | code-closed | Validation/assessment contracts clear stale `replanStateChangingContinuation`; old `repairActionPackage` cannot override authority-selected assessment/validation. `TestInitialStepInputAuthorityAssessmentPreemptsStaleReplanStateChange` covers stale replan state-change lineage being preempted by authority `assessment`. |
| S5 Candidate Violation Handling | code-closed | Existing bounded contract-violation tests remain in orchestrator; this pass keeps rejected diagnostics out of authority and focuses on stale lineage preemption. |
| S6 Projection/ViewModel Alignment | code-closed | Projection reads `convergenceAuthority.nextAction` before local derivation and emits `stale_persisted_next_action` diagnostic on persisted baseline conflict. |
| S7 Access Boundary Cleanup | verified | Precise search found no user-visible `/run continue` command or `Intent("continue")`; remote mode remains on-demand through projection events unless agent requires full stream. |
| S8 Regression and Boundary Gate | code-closed | Added authority snapshot and projection authority regression tests; targeted package tests and full `go test ./... -count=1` pass. |
