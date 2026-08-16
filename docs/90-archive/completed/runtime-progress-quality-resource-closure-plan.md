# Runtime Progress Quality And Resource Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-28

Related:

- `../superseded/goal-dod-nextaction-delivery-state-machine-design.md`
- `convergence-authority-simplification-design.md`
- `runtime-terminal-outcome-authority-closure-plan.md`
- `runtime-live-convergence-gate-closure-plan.md`
- `projection-event-io-resource-closure-plan.md`
- `runtime-managed-validation-resource-lifecycle-closure-plan.md`

## Purpose

本文基于最新 live workspace 核对结果，收敛两个仍然影响 WorkOrch 交付质量和运行资源的问题：

1. workspace 能持续执行或进入 completed，但未按有效 Goal/DoD 产生充分、可追溯、全量覆盖的交付结果。
2. 长会话执行后 workorch 主进程 CPU/RSS、projection/TUI snapshot 成本、validation scratch 磁盘与残留进程资源持续升高。

本文不是替代长期设计，而是将 `goal-dod-nextaction-delivery-state-machine-design.md` 和 `convergence-authority-simplification-design.md` 的要求落实为本轮代码收口清单。后续代码修改必须逐项引用本文条目，避免继续按单点现象打补丁。

当前阶段不考虑兼容性。若现有实现、测试或端侧行为与本文冲突，应按本文和上游设计收口。

## Contract Audit

- Scope: runtime terminal completion、coverage/evidence ledger、NextAction retry、LLM contract violation recovery、TUI embedded snapshot、projection/resource IO、validation scratch/process lifecycle。
- Observed state:
  - `ws-7ced70e98ea50c43/run-010` 进入 `waiting_decision`，最后 step 为 `llm/workspace_execution` failed，failureClass 为 `llm_decision_invalid`。失败原因是 evidence acquisition 阶段的 `shell/command` 被判定会写 deliverable 文件。
  - `ws-7ced70e98ea50c43/run-010` coverage 为 `total=89, satisfied=5, failed=23, open=58, evidence_ready=3`，但当前有效文件变化只有 `go.mod` 增加依赖，未形成与当前 Goal/DoD 缺口匹配的实质交付。
  - `ws-4e83810542ac4d65` 投影为 `completed/accepted`，但 coverage 仍为 `total=40, satisfied=1, evidence_ready=39`。其 accepted result summary 明确说明只覆盖 current focus，不等同于整个有效 Goal/DoD 的最终验收。
  - workorch 主进程约 30%+ CPU、约 1.3GB RSS；CPU profile 热点集中在 TUI `FetchWorkspaceContext` -> embedded backend `Snapshot` -> EventHub command/result -> `decodeResultResource` 的 JSON marshal/unmarshal、projection detail compact 和 TUI render。
  - 单 workspace snapshot 约 1.8MB；`run-010` 有 1000+ 持久化文件，workspace 历史 runs 达 GB 级。
  - `/tmp/workorch-validation-scratch` 约 9.6GB，存在多个历史 validation scratch 下的 proxy/localproxy 进程仍持有 cwd、端口、fd 和二进制文件。
- Expected state:
  - terminal accepted 只能由全量有效 Goal/DoD coverage satisfied 触发；current focus 局部验收只能推进下一 focus，不能完成 workspace。
  - Runtime 每轮只从 coverage/evidence ledger 和治理状态推导唯一 NextAction；LLM 只生成当前 narrow candidate。
  - 可恢复合同违例应作为 rejected diagnostic evidence 并在同一 NextAction 下 bounded retry；只有缺少外部信息、权限、凭证、用户业务选择或不可恢复治理条件时才进入 decision。
  - TUI/Web/CLI 只消费 workorchd projection/ViewModel；embedded 同进程路径不应对大 snapshot 做 JSON round-trip。
  - validation resource 必须有 runtime-owned lifecycle，scratch/process/cache 必须 bounded、可释放、可投影。
- Deviation:
  - terminal completion gate 仍允许局部 result 和 completed run state 组合成 accepted workspace outcome，绕过全量 coverage authority。
  - contract violation recovery 在可自动继续的输出形态错误、forbidden kind、scratch/write violation 等场景仍可能升级 decision，导致 replan/decision/repair 循环。
  - TUI embedded backend 把 typed projection snapshot 转为 `map[string]any`，再 marshal/unmarshal 回 typed DTO，且多个事件触发全量 context refresh。
  - validation scratch 和子进程资源没有覆盖所有验证路径，历史资源累积放大后续扫描、投影、文件系统和端口压力。
- Owner:
  - `core`: terminal gate、coverage/evidence ledger、NextAction、contract violation recovery、projection/read model、embedded backend DTO、resource lifecycle 通用机制。
  - `workspaceTemplate/Soul/policy`: 软件开发类验证策略、文件粒度、服务验证方式、测试 harness 偏好。
  - `TUI/Web`: 只负责 projection/ViewModel 展示和 scoped command 提交，不拥有业务状态机。
- Event path:
  - `command/event -> workorchd owner state -> runtime scheduler -> convergence authority refresh -> Stage Packet -> LLM/block candidate -> contract gate -> accepted/rejected evidence -> coverage recompute -> terminal/next action -> projection/ViewModel -> TUI/Web`。
- Contract gap:
  - terminal gate 没有把全量 coverage summary 作为 hard authority。
  - contract violation recovery 没有统一判定“当前 NextAction 是否仍可自动 narrow retry”。
  - embedded backend command/result 缺少 typed fast path 或 projection DTO，导致同进程大对象通过 JSON 搬运。
  - validation resource lifecycle 只覆盖 managed validation happy path，未覆盖普通 bounded validation 历史残留、scratch TTL/size limit 和 orphan process reconciliation。
- Boundary check:
  - 本文只使用 coverage item、evidence kind、NextAction、stage contract、projection DTO、resource lifecycle 等中立概念。
  - 不允许在 core 中加入 workspace title、模板 ID、Soul ID、语言、文件名、命令、provider 或 live workspace ID 分支。
- Verification:
  - 每项收口必须有失败样本单测或 live verification 证据。
  - 所有 core 改动必须执行业务中立边界搜索。

## Closure Principles

1. Terminal authority before progress appearance. 不能为了让 workspace 看起来完成而放宽 completion gate。
2. Deterministic NextAction before prompt adjustment. 先保证 runtime 唯一下一步正确，再优化 LLM 文案。
3. Rejected diagnostics are evidence, not state. 违例可记录、可反馈、可重试，但不能 accepted、不能 side effect、不能 high-value memory。
4. Projection is display, not repair. Projection 可以暴露 authority diagnostics，不能修 owner state。
5. Embedded local path is not remote JSON. 同进程 TUI/backend 应复用 typed projection/port DTO；HTTP/REST remote 路径继续 JSON。
6. Resource lifecycle is part of validation contract. 外部调用、shell、MCP、web、file 或 provider 等可能卡住或产生资源的路径，都必须可超时、可释放、可审计。

## Closure Scope

### P0: Terminal Completion Gate

Problem:

- `ws-4e83810542ac4d65` 显示 `completed/accepted`，但 coverage 只有 `satisfied=1,total=40`。
- Accepted result summary 明确为 current focus scoped，却被终局 outcome 选中。

Target:

- `complete` NextAction 只能在当前 definition revision 下所有 required coverage items 满足时产生。
- `ResultStateAccepted` 不等于 terminal accepted。只有 result 具备 final coverage authority，且 run terminal state 由 completion gate 决定时，才可成为 workspace outcome。
- current focus acceptance、acceptance slice、partial assessment、focus-scoped result 必须标记为 non-terminal accepted evidence。

Code areas:

- `internal/modules/kernel/orchestrator/biz/*completion*`
- `internal/modules/kernel/orchestrator/biz/*coverage*`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/result_outcome_eligibility.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_result_selection.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/run_outcome.go`

Acceptance:

- 单测构造 `coverage total=40 satisfied=1` + focus-scoped accepted result，run/workspace 不能 completed/accepted。
- 单测构造全量 required evidence satisfied，completion gate 才能产生 terminal accepted。
- Projection 对历史局部 accepted result 只显示为 progress/evidence，不显示为 final outcome。

### P1: Contract Violation Recovery And Decision Eligibility

Problem:

- `ws-7ced70e98ea50c43/run-010` 当前失败属于 evidence acquisition 输出形态违例：LLM 返回的 validation graph 会写 deliverable。
- runtime 正确拒绝 side effect，但进入 `waiting_decision` 后会造成 replan/decision/repair 循环，降低正向产出。

Target:

- 对 parse/schema、forbidden kind、scratch escape、deliverable write forbidden、missing coverage binding 等可结构化修正的违例，默认进入 rejected diagnostic + bounded retry。
- 只有满足外部不可自动获得条件时才能 request decision，例如缺少凭证、权限、用户业务选择、外部环境确认、不可替代资源或预算耗尽后的治理选择。
- retry 必须保持当前 NextAction、focus、requiredOutput、stage contract，不允许旧 repair lineage 覆盖当前 authority。

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_decision_eligibility*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_repair*.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_next_action.go`

Acceptance:

- 单测覆盖 evidence acquisition shell 写 deliverable 被拒后，若 retry budget 未耗尽，NextAction 仍为 bounded retry/repair，不创建 decision ticket。
- 单测覆盖同 signature 超出 focus budget 后，进入 governed decision 或 failed，并暴露明确 reason。
- 单测证明 rejected output 没有 side effect、没有 accepted result/artifact、没有 high-value memory。

### P2: Narrow Stage Packet And LLM Context Diet

Problem:

- `run-010` 单次 LLM request 约 1.75MB，input tokens 超 65k，累计 tokens 接近百万。
- C4/C5 动态上下文占比高，cache 实际无 read/write 收益；LLM 易被历史 rejected graph、旧 decision、旧 memory 和大 coverage 噪声干扰。

Target:

- 每轮 Stage Packet 只携带当前 focus 或一组强相关 focus。
- 必须携带最新 Goal/DoD revision、current gap、requiredOutput、allowed/forbidden actions、accepted/rejected evidence refs、target snapshot integrity。
- 历史只保留近期完整关键事件 + compact summary + sourceRef；需要完整 payload 时按需读取。
- C0/C1/C2 稳定分区不被 repair、provider retry、current failure、memory recall 污染。

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_stage_packet.go`
- `internal/modules/kernel/orchestrator/biz/runtime_step_input.go`
- `internal/modules/kernel/orchestrator/biz/runtime_llm_context.go`
- `internal/modules/blocks/llm/biz/prompt_messages.go`
- `internal/modules/blocks/llm/biz/provider_context_budget.go`
- `internal/modules/blocks/llm/biz/provider_cache_plan.go`

Acceptance:

- 单测证明 Stage Packet 中只有一个 authority current focus，不渲染并列 stale focus。
- 单测证明关键字段不可被 budget 裁剪：Goal/DoD、requiredOutput、current failure、accepted/rejected evidence、target snapshot integrity。
- request log 验证同一 run 内动态阶段变化不改变 C0 stable prefix。
- live verification 中每轮 request size 和 non-cacheable tail 明显下降。

### P3: TUI Embedded Snapshot Typed Path

Problem:

- CPU profile 显示 50%+ 累计 CPU 在 TUI `FetchWorkspaceContext` -> EventHub `Snapshot` -> `decodeResultResource`。
- embedded backend 将 typed `WorkspaceSnapshotInfo` 转为 `map[string]any` 再 JSON round-trip 回 typed DTO。
- `EventStepCompleted`、`EventLLMUsageRecorded`、`EventMemoryRecallRecorded`、`EventResultAccepted`、`EventWorkspaceProjectionChanged` 等会触发全量 context refresh。

Target:

- embedded backend 对高频 DTO 使用 typed result path，至少覆盖 `WorkspaceSnapshotInfo`、workspace list ViewModel、run status、decision/approval list/detail。
- `FetchDecisions`、`FetchApprovals`、`FetchDecision`、`FetchApproval` 不再通过全量 snapshot 过滤，改用专用 projection/port DTO 或事件 payload。
- TUI 对高频事件优先增量更新 ViewModel；只有 workspace switch、projection revision gap、terminal/governance transition 或 explicit detail open 才拉 detail snapshot。
- HTTP/REST remote 路径继续 JSON，不纳入本项去 JSON 范围。

Code areas:

- `internal/services/workorch/biz/backends/eventhub.go`
- `internal/modules/application/workorchd/biz/appservice/clicore/*`
- `internal/modules/application/workorchd/biz/appservice/cli_*commands.go`
- `internal/services/workorch/service/tui/commands.go`
- `internal/services/workorch/service/tui/model.go`
- `internal/pkg/workorchport/workspace.go`

Acceptance:

- CPU profile 中 `decodeResultResource` 不再出现在 active TUI snapshot 热点前列。
- 单测证明 embedded `Workspace().Snapshot()` 不走 JSON marshal/unmarshal fallback。
- TUI model 测试证明 LLM usage/memory recall/step progress 不触发全量 snapshot，仍能更新主视图和状态栏。
- remote backend REST JSON 行为保持不变。

### P4: Projection And History IO Budget

Problem:

- workspace run history 达 GB 级，snapshot/detail 构建和 projection compact 会放大 CPU/GC。
- 当前 active snapshot 仍包含大量集合字段，TUI 常规展示不需要全部 detail。

Target:

- 常规 ViewModel 使用 projection-first compact DTO，不携带完整 results/patches/decisions/events/llm traces。
- detail snapshot 按需读取，并区分 `summary`、`detail`、`observability` 三种 load mode。
- Projection manager 对 runtime progress only event 继续轻量处理；其他事件合并/节流 rebuild，避免多事件风暴重复构建。
- 大对象必须通过 `payloadRef`、`snapshotRef` 或 trace ref 按需读取。

Code areas:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore/*`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/workspace_snapshot_detail.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelports/*`
- `internal/modules/application/workorchd/service/server/http_workspace.go`
- `internal/modules/application/workorchd/service/server/workspace_projection.go`

Acceptance:

- 单测覆盖 summary/detail/observability load mode 字段差异。
- Projection rebuild 对 progress-only event 不做 full detail hydrate。
- live verification 中 active TUI refresh 的 snapshot payload 显著低于当前 1.8MB。

### P5: Validation Resource Lifecycle

Problem:

- `/tmp/workorch-validation-scratch` 已增长到 GB 级。
- 历史 proxy/localproxy 进程仍持有 scratch cwd、端口、fd 和二进制文件。
- managed validation 当前有 `autoCleanup` 路径，但普通 validation shell 或历史路径仍可能残留资源。

Target:

- 所有 validation 相关外部调用都进入 runtime-managed resource ledger，记录 process group、scratch dir、allocated port、cache dir、createdAt、releasedAt、cleanup status。
- shell/MCP/web/provider/file 等外部调用都必须有 timeout、idempotency key 和 terminal resource release event。
- validation scratch 支持 per-step cleanup、per-run TTL/size limit 和 orphan process reconciliation。
- 具体 build cache 策略属于 template/profile；core 只提供中立 cache dir/scratch dir lifecycle 能力。

Code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_managed_validation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_validation_scratch.go`
- `internal/modules/blocks/shell/biz/shell_runtime_adapter.go`
- `internal/modules/blocks/mcp/biz/*`
- `internal/modules/blocks/web/biz/*`
- `internal/modules/application/workorchd/biz/appservice/runports/recovery*`

Acceptance:

- 单测覆盖 managed validation 正常完成、timeout、contract rejection、process orphan 四种路径均写 resource release/cleanup diagnostic。
- live verification 后没有历史 validation proxy/localproxy 残留进程。
- scratch 目录大小受 TTL/size policy 控制；cleanup 不删除 workspace deliverable。

### P6: Template Strategy For Software Development

Problem:

- 当前 live software-dev 类 workspace 多轮执行后，有时只改依赖或测试，未围绕 Goal/DoD 产出目标文件。
- 这类推进偏差不能写入 core 的语言/命令特判。

Target:

- software-dev workspaceTemplate/profile 明确阶段偏好：
  - 先确认 current focus 与目标资源快照。
  - 缺 state_change 时必须优先产生目标交付 patch，而不是只做观测或只改测试。
  - 缺 validation 时优先使用 bounded foreground harness、fixture、临时端口、managed validation。
  - 文件粒度过大时按模块拆分，避免生成巨大单文件。
- Core 只消费结构化 policy，例如 required evidence kinds、allowed output kind、file granularity hint、validation strategy hint。

Code areas:

- `internal/modules/application/workorchd/biz/bootstrap/defaults/templates.json`
- `internal/modules/application/workorchd/biz/bootstrap/defaults/souls.json`
- software-dev profile/policy binding code
- skill files under project-level `.agents` only when needed

Acceptance:

- 新 workspace 从 template 创建时能在 workspace binding/run executionContext 中追溯策略。
- Core 边界搜索证明没有新增模板/语言/命令特判。
- live verification 中每个 state_change NextAction 至少产生与当前 focus targetRefs 对齐的候选 patch 或明确结构化 blocker。

## Implementation Order

1. P0 Terminal Completion Gate。
2. P1 Contract Violation Recovery And Decision Eligibility。
3. P3 TUI Embedded Snapshot Typed Path。
4. P4 Projection And History IO Budget。
5. P5 Validation Resource Lifecycle。
6. P2 Narrow Stage Packet And LLM Context Diet。
7. P6 Template Strategy For Software Development。

理由：

- P0 先防止错误完成，避免“表面成功”掩盖真实未收敛。
- P1 先减少无效 decision/replan 循环，恢复自动推进能力。
- P3/P4/P5 先降低 CPU/IO 和资源压力，保证后续 live verification 可持续执行。
- P2/P6 再提升 LLM 输入质量和模板产出质量。

## Implementation Progress

Last Updated: 2026-06-28

Completed:

- P0 Terminal Completion Gate:
  - Read model and runtime result recording now reject final accepted/completed outcome when current coverage still has active/open required work items.
  - Current-focus accepted results remain progress evidence unless the full coverage completion gate approves terminal outcome.
- P1 Contract Violation Recovery And Decision Eligibility:
  - Contract violation correction now preserves explicit current requiredOutput shape for validation/failure evidence acquisition instead of forcing generic state-changing output.
  - Recoverable validation evidence output-shape drift can stay in bounded retry under the same authority contract.
- P3 TUI Embedded Snapshot Typed Path:
  - Embedded EventHub result payloads support typed resource fast path for high-frequency DTOs.
  - TUI decision/approval fetches use dedicated backend ports instead of full workspace snapshot filtering.
- P4 Projection And History IO Budget, first closure:
  - Workspace projection snapshot skips cross-run patch and artifact-continuation derived collection scans; selected run detail still comes from the projection run snapshot.
  - Default HTTP snapshot path stays projection-only; `view=observability` remains the explicit full-read path.
- P4 Projection And History IO Budget, load-mode closure:
  - Public workspace snapshot load modes are now explicit: `summary`, `detail`, and `observability`.
  - Summary projection snapshots strip run/detail/heavy collections and do not hydrate detail state.
  - Default HTTP snapshot remains compact projected detail; `view=summary` uses summary projection and `view=observability`/`debug` uses explicit full read.
- P5 Validation Resource Lifecycle, first closure:
  - Managed shell validation now releases and reports scratch resources even when the shell command is rejected before process start by command policy or timeout policy.
  - Managed shell validation timeout now reports scoped process/scratch resource lifecycle diagnostics, removes auto-cleanup scratch, and separates execution outcome from resource release state.
- D1 Current Action Label / Lineage closure:
  - Task progress and run status now expose `currentActionLabel` separately from raw `currentStepId` / `diagnosticLineage`.
  - Projection derives the action label from the current convergence authority / gap baseline focus, so old failure lineage in step IDs no longer hides the current authority focus.
- D2 Partial Assessment Outcome closure:
  - Task progress now exposes a neutral `assessmentOutcome` summary with accepted slice count, accepted/rejected result count, satisfied/remaining coverage counts, latest accepted/rejected result refs and rejected reason.
  - Partial accepted slices plus rejected global DoD assessment are projected as `partial_progress_accepted_global_assessment_rejected` instead of an ambiguous terminal failure signal.
- D3 Authority Recompute Contract Wording closure:
  - `coverage_next_action_recompute` keeps `recoverability=recompute_authority`, but `correctionContract.requiredOutputShape` is now the LLM-facing narrow output shape.
  - Runtime-owned recompute is represented as `correctionContract.runtimeRecomputedAuthority`; `authority_recomputed_next_action` is no longer used as an LLM required output shape.
- D5 Ordinary Shell Validation Scratch closure:
  - Ordinary shell validation using `validationScratchDir` now records scoped `validationResources` for process and scratch lifecycle even without a managed validation session.
  - Temporary validation scratch cleanup defaults on for step-scoped scratch and remains guarded against deleting workspace deliverables or non-temporary paths.
- P2 Narrow Stage Packet And LLM Context Diet, first closure:
  - Provider context budget integrity gate now verifies protected runtime contract and protected coverage work item context after compaction, in addition to Goal/DoD, repair targets and stage packet.
  - Direct tests now prove Stage Packet uses only the authority current focus when stale focus fields exist elsewhere, target snapshot integrity metadata is preserved under aggressive budget compaction, and C0/C1/C2 cache hashes remain stable under provider retry + state-changing dynamic fields.

Remaining:

- P4: continue reducing normal ViewModel payload fields only if live profile still shows heavy projection/detail reads after summary/detail/observability load-mode rollout.
- P5: extend resource lifecycle ledger/release diagnostics beyond shell managed validation to MCP/Web/provider external calls and recovery/orphan reconciliation paths. This requires a broader runtime-owned resource ledger and should be verified with live orphan-process/scratch checks.
- D6: runtime progress event/checkpoint volume already has projection-side progress-only handling and progress checkpoint dedupe. Further checkpoint sampling must keep worker heartbeat recovery intact and should be driven by live write profile evidence.
- P6: no core strategy move is required in this pass. Continue validating that software-development execution quality guidance remains in template/profile/Soul data and is traceable through workspace binding and run executionContext.

## Live Verification Addendum: run-010-replan-001

Observed At: 2026-06-28

Scope:

- Workspace: `ws-7ced70e98ea50c43`
- Run: `run-010-replan-001`
- Service process: `./bin/workorch`, PID `932998`
- Verification mode: read-only live observation after latest deployment

### Observed Progress

The latest service is not stuck in EventHub, projection, decision, or provider
startup. The active run keeps producing stream progress, terminal step events,
accepted results, rejected diagnostics and projection revisions.

Positive evidence:

- Projection is aligned: `sourceEventRevision == projectedSourceRevision`.
- TUI/Web projected prompt mode is `running` for the active workspace.
- Initial validation-output contract violation was rejected and retried.
- `cwi-005` provider routing validation completed.
- `cwi-006` local single-binary validation first failed on Go VCS stamping, then
  repair produced a bounded validation rerun and completed.
- `cwi-007` observability validation completed.
- `cwi-008` provider configuration validation completed.
- Coverage advanced from `5/89 satisfied` to `13/89 satisfied`.
- Assessment produced 4 accepted result slices, but the incomplete global DoD
  assessment was still rejected with `dod_coverage_missing`.
- Current focus advanced to `cwi-009` provider selection priority.
- LLM cache partitioning remained stable across dynamic repair/follow-up calls:
  `stablePrefixHash`, workspace definition hash and capability contract hash
  stayed stable while the volatile tail changed.

Latest observed state:

```text
run state: running
steps: 15
results: 4
artifacts: 0
coverage: 13/89 satisfied, 2 evidence_ready, 16 failed, 58 open
current focus: cwi-009
current gap: missing validation evidence
current step: assessment LLM, provider waiting
deliverable diff: go.mod only
```

Resource evidence:

```text
CPU: about 28-30%
RSS: about 1.1GB+
process write_bytes: about 1.1GB+
validation scratch: about 10GB
```

### Deviations Against Architecture Contracts

#### D1: Lineage ID Still Looks Like Stale Focus

Observed:

- Several current step IDs still contain old fragments such as
  `verify-focus-cwi-006`, even when the dispatch input is correctly bound to
  `focus-cwi-008` or `cwi-009`.

Expected:

- Diagnostic lineage may preserve old failure references, but user-visible step
  IDs, projection summaries and current action labels should make the current
  authority focus obvious.

Owner:

- `core`, specifically runtime step ID/lineage naming and projection display.

Contract gap:

- Current authority focus is correct in `coverageFocus`,
  `repairActionPackage.coverageFocus`, `requiredOutput.coverageFocusId` and
  `convergenceAuthority.nextAction`, but step naming still inherits stale
  failure lineage too prominently.

Closure target:

- Keep immutable source failure lineage in explicit diagnostic fields.
- Build current step display name from current authority focus and NextAction.
- Projection should expose both `currentActionLabel` and `diagnosticLineage`,
  instead of forcing operators to infer current focus from long step IDs.

Acceptance:

- A repair caused by an old failed step but rebound to `focus-cwi-008` displays
  `currentFocus=cwi-008` and `diagnosticLineage=<old-step-id>` separately.
- TUI/Web main view uses projected current action label, not raw step ID, for
  user-facing status.

#### D2: Partial Accepted Results And Failed Global Assessment Need Clearer Outcome Semantics

Observed:

- The assessment step accepted 4 result slices and then recorded
  `dod_coverage_missing` for the same assessment because global DoD coverage was
  still incomplete.

Expected:

- This is directionally correct: valid partial evidence can be accepted, while
  incomplete global assessment must not complete the run.
- However, projection and diagnostics must make this explicit as
  `partial_progress_accepted + global_assessment_rejected`, not as an ambiguous
  failure.

Owner:

- `core`, specifically assessment result recording, contract violation
  diagnostics and read model projection.

Contract gap:

- The accepted result slices and rejected global assessment are both valid state
  updates, but current summaries mostly surface the repair-loop failure,
  obscuring the fact that coverage advanced from 5 to 13.

Closure target:

- Add a neutral assessment outcome summary:
  `acceptedSlices`, `newlySatisfiedCoverageCount`,
  `remainingCoverageCount`, `globalAssessmentState`.
- Repair loop summary should distinguish "last failed global completion check"
  from "latest effective progress".

Acceptance:

- Projection for a partial assessment shows coverage delta and remaining
  blockers together.
- A failed global assessment with accepted slices is not shown as terminal
  failure and does not hide coverage improvement.

#### D3: Correction Contract And Derived Required Output Are Semantically Aligned But Verbally Divergent

Observed:

- `contractViolationRecovery.correctionContract.requiredOutputShape` requested
  `authority_recomputed_next_action`.
- The next dispatched input had `convergenceAuthority.nextAction.kind=validate`
  and `repairObjective=produce_concrete_validation_evidence`.
- The actual behavior was acceptable because runtime recomputed authority and
  selected validation, but the diagnostic contract wording still appears to ask
  LLM to recompute authority.

Expected:

- Runtime should recompute authority deterministically.
- If runtime has already recomputed `NextAction=validate`, the LLM contract
  should ask only for the current narrow validation output, while the
  recomputation evidence remains runtime diagnostic metadata.

Owner:

- `core`, specifically contract violation recovery rendering and Stage Packet
  derived contract wording.

Contract gap:

- The system behavior follows `convergence-authority-simplification-design.md`,
  but the correction contract phrase can mislead future code and LLM behavior
  into treating authority recompute as an LLM responsibility.

Closure target:

- Split correction recovery into:
  - `runtimeRecomputedAuthority`: runtime-owned diagnostic output.
  - `llmRequiredOutput`: current narrow candidate output, such as bounded
    validation evidence.
- For recoverability `recompute_authority`, never ask the LLM to own authority
  recompute once runtime has selected a concrete NextAction.

Acceptance:

- Request log for a `dod_coverage_missing` retry shows runtime recompute
  metadata but LLM required output is only the selected narrow action.
- Tests prove `authority_recomputed_next_action` is not rendered as the LLM
  candidate shape when `convergenceAuthority.nextAction` is already known.

#### D4: State-Change Output Remains Too Weak For Goal Completion

Observed:

- After many steps and 9 LLM calls, the deliverable workspace only shows
  `go.mod` modified and no artifacts.
- Several focus items are being closed by validation of existing behavior or
  dependency updates, but broad Goal/DoD coverage still requires concrete
  implementation output.

Expected:

- If current focus lacks state-change evidence, NextAction must prefer
  state-changing output.
- If current focus lacks validation evidence, validation is correct, but after
  validation/assessment identifies missing implementation, the next action must
  return to a focused state-change slice, not continue only verifying around the
  gap.

Owner:

- Mixed:
  - `core`: evidence-kind-driven NextAction and state-change/validation ordering.
  - `workspaceTemplate/Soul/policy`: software-development strategy for patch
    granularity, file scope and validation harness preference.

Contract gap:

- The current ledger can select validation for specific focus items, but the
  broader run still lacks a strong "implementation delta required before more
  validation" signal for unmet DoD items.

Closure target:

- Keep core business-neutral by using evidence kind, not language or file names:
  if a coverage item requires `state_change` and has no accepted state-change
  evidence, `validate` cannot satisfy it.
- Software-dev template should ensure implementation-phase state-change tasks
  are sliced by current targetRefs and avoid repeatedly producing validation-only
  graphs for unmet behavior.

Acceptance:

- Unit tests prove validation evidence alone cannot satisfy a coverage item that
  still requires state-change evidence.
- Software-dev live run shows each state-change NextAction produces a patch,
  artifact update or structured blocker scoped to current targetRefs.

#### D5: Validation Scratch And Resource Lifecycle Still Exceed Sustainable Bounds

Observed:

- `/tmp/workorch-validation-scratch` is about 10GB.
- Workorch process writes exceeded about 1.1GB in the observed run window.
- Historical proxy/localproxy processes from prior runs still exist.
- The current run uses ordinary `shell/command` validation with scratch copies;
  managed validation lifecycle coverage is better than before but does not yet
  cover all validation paths and historical orphan reconciliation.

Expected:

- Validation scratch/process/cache resources are runtime-owned, bounded,
  releasable and auditable.
- Ordinary validation shell that uses `validationScratchDir` should have neutral
  resource lifecycle metadata, not only managed-session validation.

Owner:

- `core` for neutral resource ledger, scratch TTL/size policy and orphan
  reconciliation.
- `workspaceTemplate/Soul/policy` for preferring managed validation or bounded
  foreground harnesses when validation is scratch-heavy.

Contract gap:

- P5 is only partially closed. Resource lifecycle is still not uniformly applied
  to all validation producers.

Closure target:

- Extend resource ledger to shell validation using `validationScratchDir`.
- Add run-scoped scratch cleanup policy and orphan process diagnostics.
- Projection should show live scratch/process resource pressure as
  diagnostics, not require manual shell inspection.

Acceptance:

- Live run completion or timeout leaves no new orphan validation processes.
- Scratch usage for a run is bounded by policy and projected as resource
  diagnostics.
- Tests cover ordinary shell validation scratch cleanup, not only managed
  validation timeout/rejection.

#### D6: LLM Context Tail And Runtime Event Volume Still Drive Resource Cost

Observed:

- Request logs reached 9 LLM entries and over 530k provider tokens.
- Latest max estimated input tokens rose to about 460k, with request byte caps
  still at 4MiB.
- Stream progress and checkpoint files continue to grow during long provider
  calls.

Expected:

- C0/C1/C2 stability is preserved, but C3/C4/C5 should stay narrow enough for
  active progress.
- Progress-only events should not force full detail hydration or excessive
  persistent writes.

Owner:

- `core`, specifically Stage Packet compaction, process evidence timeline,
  checkpoint cadence, progress event persistence and projection load modes.

Contract gap:

- Cache partitioning is now structurally correct, but dynamic tail volume is
  still large enough to slow convergence and increase memory/IO cost.

Closure target:

- Cap current action context to one authority focus plus a small related target
  set unless NextAction explicitly requires a group.
- Move high-frequency provider waiting/heartbeat progress to compact progress
  state; persist terminal milestones and bounded periodic samples.
- Ensure normal summary/detail projection does not hydrate full stream progress.

Acceptance:

- Live request logs show lower C4/C5 bytes after the same number of steps.
- Stream progress event volume does not dominate checkpoint/projection writes.
- CPU/RSS/write_bytes remain bounded during a multi-step live run.

## Updated Remaining Closure Items

The previous P0-P6 items remain valid. Based on `run-010-replan-001`, the next
closure pass should be ordered as follows:

1. D2: clarify partial assessment accepted vs global assessment rejected in
   result recording and projection.
2. D3: split runtime authority recompute diagnostics from LLM required output.
3. D1: separate current action labels from diagnostic lineage in step naming and
   ViewModel.
4. D5: extend resource lifecycle from managed validation to ordinary shell
   validation scratch and orphan reconciliation.
5. D6: reduce C3/C4/C5 dynamic tail and progress/checkpoint write volume.
6. D4: strengthen evidence-kind-driven state-change requirements and ensure
   software-dev template strategy keeps implementation work in template/Soul
   policy, not core.

These items are deliberately framed with business-neutral core behavior. Any
software-development-specific execution guidance belongs in
workspaceTemplate/profile/Soul/skill data and must be traceable through
workspace binding and run executionContext.

## Verification Plan

Targeted tests:

```bash
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1
go test ./internal/services/workorch/... -count=1
go test ./internal/modules/blocks/shell/biz -count=1
```

Boundary search:

```bash
rg -n "software-dev|weather-query|code-review|go test|npm test|no tests to run|city|ssh|server|proxy|lsp" internal/modules/kernel internal/modules/application/workorchd/biz/appservice internal/pkg
rg -n "json\\.Marshal\\(|json\\.Unmarshal\\(" internal/services/workorch/biz/backends internal/modules/application/workorchd/biz/appservice/clicore
rg -n "nohup|setsid|pkill|killall|fuser -k| &" internal/modules internal/services internal/pkg
```

Live verification:

1. Record binary path、pid、git commit、service start time。
2. Pick one active software-dev workspace and one completed workspace with remaining coverage.
3. Verify completed workspace cannot remain terminal accepted if coverage is not fully satisfied.
4. Trigger resume/replan on active workspace.
5. Observe command accepted -> authority refresh -> NextAction -> dispatch/retry/governed wait。
6. Verify contract violation is rejected diagnostic evidence and bounded retry when recoverable.
7. Capture CPU profile after active TUI session and confirm JSON round-trip hotspot is reduced.
8. Verify validation scratch size and orphan validation processes after run completion/timeout.

## Done Criteria

- No workspace can enter final accepted/completed while current definition revision has unsatisfied required coverage.
- Current focus accepted result is progress evidence only unless full coverage completion gate approves terminal outcome.
- Recoverable LLM contract violations retry under the same authority contract without side effects and without immediate human decision.
- TUI embedded snapshot path avoids JSON round-trip for high-frequency typed DTOs.
- Projection/ViewModel normal path does not hydrate full history or large detail payloads.
- Validation scratch/process resources are bounded, released, projected and recoverable.
- Software-dev specific execution quality rules live in template/profile/Soul/policy, not core.
- Core boundary search shows no new template/language/provider/workspace-specific behavior branches.

## Open Questions

1. Whether typed EventHub result should be implemented as a new internal-only field on `CLIResultPayload`, or as a separate embedded backend direct projection reader. Preferred direction: keep EventHub command/result for owner boundary, but add internal typed payload fast path that is never exposed to REST/SSE JSON.
2. Whether terminal gate should fail-fast historical inconsistent terminal runs on projection read, or require an explicit repair/reconcile command. Preferred direction: owner recovery should reconcile, projection should only expose diagnostics until owner state is fixed.
3. Whether validation scratch TTL/size policy should be globally configured or workspaceTemplate-bound. Preferred direction: core provides neutral defaults and hooks; template/profile may lower or tune limits.
