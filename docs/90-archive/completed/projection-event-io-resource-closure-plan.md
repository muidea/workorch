# Projection Event IO Resource Closure Plan

Status: active

Type: closure-plan

Last Updated: 2026-06-26

Related:

- `../../30-module-architecture/goal-dod-nextaction-delivery-state-machine-design.md`
- `workorchd-state-loop-and-projection-closure-plan.md`
- `convergence-authority-dispatch-closure-checklist.md`
- `convergence-authority-single-source-closure-plan.md`
- `goal-dod-nextaction-s9-1-projection-readmodel-first.md`
- `goal-dod-nextaction-s9-3-snapshot-payload-on-demand.md`
- `goal-dod-nextaction-s11-7-cpu-io-observation-e2e.md`

## Purpose

本文档固化 2026-06-26 最新部署实例核对后的资源与投影收口方案。当前主执行链路已经恢复正向推进，`coverage_assessment_incomplete -> recompute_authority` 也已在 live 状态中生效；剩余主要偏差转移到 active execution 期间的 projection/event IO 压力、短暂 stale projection 和 TUI/Web 轻量展示契约。

本计划只处理 WorkOrch core / workorchd projection / read model / TUI-Web display contract 的中立能力，不处理具体 workspace 目标、模板业务策略、编程语言、provider 品牌或 live workspace 特判。

## Contract Audit

- Scope:
  - 最新部署服务 `http://127.0.0.1:8080`。
  - 两个 live workspace：
    - `ws-4e83810542ac4d65`
    - `ws-7ced70e98ea50c43`
  - workorchd projection、event record repository、run snapshot/status、TUI/Web ViewModel、runtime convergence status、CPU/IO resource envelope。
- Observed state:
  - 服务 ready，workspace projection ready，workspaceTotal=2。
  - 两个 workspace 均为 `running`，无 open decision，无 pending approval。
  - 30 秒只读观测窗口内：
    - `workspaceProgress.pass=true`。
    - `stalledRuns=0`。
    - `ineffectiveRetryRuns=0`。
    - runtime EventHub publish failure delta 为 `0`。
    - scheduler command / enqueue / continuation failure delta 为 `0`。
    - SSE 捕获到 `/run/llm_stream/progress`、`/step/dispatched`、`/step/state/changed`、`/run/llm_usage/recorded`、`/workorchd/workspace/projection/changed`。
  - `ws-4e83810542ac4d65` 的上一轮违例已经正确标记为 `coverage_assessment_incomplete`，`recoverability=recompute_authority`，说明合同分类修复生效。
  - `ws-7ced70e98ea50c43` 当前缺口为 missing accepted `validation` evidence，仍在 follow-up validation/repair 链路推进。
  - Projection display contract 通过，但 active execution 高频事件下出现短暂 stale：
    - 示例：`sourceEventRevision=2674`，`projectedSourceRevision=2673`，`staleReason=source_event`。
  - 资源压力明显：
    - 30 秒观测窗口 CPU 约 `251.78%`。
    - RSS 约 `784654336` bytes，后续瞬时采样接近 `1GB`。
    - `metadataRecordsScannedDelta=85943`。
    - `payloadHydratesDelta=2100`。
    - `payloadHydrateBytesDelta=37258458`。
    - `writeBytesDelta=36044800`。
  - pprof 热点集中在 GC、JSON decode/encode、repository clone 和 projection clone：
    - `runtime.scanobject`
    - `encoding/json.checkValid`
    - `encoding/json.(*decodeState).object`
    - `github.com/muidea/workorch/internal/pkg/repository/file.cloneAny`
    - `github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection.cloneStepViewMap`
- Expected state:
  - active execution 可以产生高频 progress，但不应反复 hydrate 大 payload 或重建完整 workspace detail。
  - TUI/Web 默认视图只消费 projection-owned compact ViewModel，不读取 template/contextHints/完整 timeline/result/patch 大对象。
  - Projection 可以短暂 stale，但端侧 promptMode、workspace state、current focus、next action、blocker 展示必须保持来自同一 projection owner，不得本地推断。
  - Run convergence status 应提供 compact read model，使观察脚本、TUI/Web 和人工核对不需要全量扫描历史事件与结果 payload。
  - active 运行期资源应与有效 event/progress 成比例，不能因为 projection/read model 重复 JSON clone/hydrate 导致持续高 CPU/RSS。
- Deviation:
  - projection/event IO 仍然把大量 active run 历史、result、patch、usage、timeline payload 拉入热路径。
  - `workspace list -> snapshot/detail -> observability` 边界仍不够清晰，部分核对路径会读取完整 workspace/template/contextHints 或大量 run 历史。
  - 高频 LLM stream/progress 期间 workspace projection rebuild 次数和 event hydrate 数偏高。
  - Projection 短暂 stale 会被直接暴露给 TUI/Web；虽然 backlog/drop 为 0，但端侧仍可能看到一拍旧状态。
  - 2026-06-26 最新部署复核发现 `/api/v1/workorch/workspaces/:workspaceId/snapshot` 响应仍返回全局 projection status，导致 snapshot display contract 缺少 workspace-scoped `workspaceRevision`。
- Owner:
  - 主 owner: workorchd projection/read model、event record repository query、run snapshot/status compact projection。
  - 次 owner: TUI/Web display contract、observation scripts。
  - 非 owner: workspaceTemplate/Soul/policy、LLM provider、specific workspace goal。
- Event path:
  - runtime event -> event record repository -> projection scheduler -> read model rebuild / compact ViewModel -> REST/SSE -> TUI/Web display。
  - runtime owner state -> compact run status / convergence status -> observation/TUI/Web。
- Contract gap:
  - 缺少 active execution 高频事件的 projection rebuild/hydrate 分级规则。
  - 缺少 compact status/ViewModel 作为 TUI/Web 默认展示源，导致端侧或观测侧容易回退到重 snapshot。
  - 缺少 event repository metadata-only / payload-on-demand 的强制接口边界和测试。
  - 缺少 projection stale 时端侧使用 last ready ViewModel 的一致性契约。
- Boundary check:
  - 本计划只处理结构化 event topic、payload hydration、projection revision、run status、convergence status、resource metrics。
  - 不允许在 core 中加入模板 ID、Soul ID、编程语言、文件名、命令名、provider、workspace title 或 live workspace ID 分支。

## Target Model

### Read Path Layers

workorchd 对外读路径分为三层：

1. Compact projection path:
   - 用于 workspace list、TUI main view、TUI prompt、Web board、状态栏。
   - 只包含小字段：
     - workspace id/state/title
     - activeRunId
     - promptMode
     - projection revision/source revision/projected source revision
     - active run state
     - current focus id
     - next action kind
     - blocker id/type
     - latest compact diagnostic
     - heartbeat summary
   - 禁止 hydrate 大 payload。
2. Detail projection path:
   - 用于 workspace detail 首屏。
   - 在 compact projection 基础上按需附加 coverage summary、latest result summary、latest patch summary。
   - 默认只读取最近窗口和 compact payload，不全量扫描历史。
3. Heavy observability path:
   - 用于 timeline、debug、审计和验证脚本。
   - 可以 hydrate 大 payload，但必须显式请求、限量、分页、可观测。

### Event Topic Policy

Event topic 按 projection 影响分级：

| Class | Example topics | Projection behavior |
| --- | --- | --- |
| heartbeat-only | `/run/llm_stream/progress`, worker heartbeat, step progress | update in-memory heartbeat / compact revision only; no full detail hydrate |
| compact-state | `/step/dispatched`, `/step/state/changed`, `/run/state/changed` | rebuild compact ViewModel; detail rebuild debounced |
| evidence-summary | result/patch/artifact/usage accepted or rejected | update compact summaries; hydrate only latest bounded payload |
| governance | decision/approval open/resolved | rebuild compact ViewModel immediately |
| definition/topology | workspace/goal/dod/template binding changes | rebuild affected workspace detail; full list only when list membership/order changes |

### Stale Projection Contract

- `projection.state=stale` 表示 source event 已领先 projected event，不表示 owner state unknown。
- TUI/Web 必须继续展示 last ready ViewModel，同时标记 projection refreshing。
- promptMode、command suggestions 和 main view 不得在 stale 时本地重新推断。
- 如果 stale 超过阈值或 `pendingRebuilds>0` 长时间不下降，才进入 degraded display warning。

### Compact Convergence Status

每个 active run 应有 compact convergence status：

```json
{
  "workspaceId": "...",
  "runId": "...",
  "runState": "running",
  "currentFocusId": "cwi-...",
  "nextActionKind": "validate",
  "currentGap": "missing accepted evidence binding for required evidence kind: validation",
  "missingEvidenceKinds": ["validation"],
  "acceptedEvidenceCount": 6,
  "rejectedEvidenceCount": 30,
  "latestEffectiveProgressAt": "...",
  "latestContractViolationRule": "coverage_assessment_incomplete",
  "latestRecovery": "recompute_authority",
  "stalled": false,
  "ineffectiveRetry": false
}
```

该 status 由 owner/read model 派生，不由 TUI/Web 或脚本扫描完整 timeline 推断。

## Closure Scope

### P0: Projection/Event IO Resource Envelope

Status: closed

Owner: workorchd projection, read model projection, event record repository.

Required closure:

- 高频 stream/progress 事件只更新 lightweight heartbeat，不触发完整 workspace detail rebuild。
- Workspace list projection 必须 metadata-only，不 hydrate result/patch/artifact/run payload。
- Detail projection 只 hydrate latest bounded summaries，不重复读取全量 run 历史。
- Event record repository 查询默认 metadata-only；payload hydrate 必须显式、限量、可统计。
- 将热路径中的 `map[string]any` 深拷贝逐步替换为 typed compact struct 或 copy-on-write summary。

Acceptance:

- active 30 秒窗口 CPU 先压到 `<100%`。
- `payloadHydratesDelta` 较当前基线下降至少 70%。
- `payloadHydrateBytesDelta` 较当前基线下降至少 70%。
- `projectionPendingRebuilds=0`。
- `projectionScheduleDroppedDelta=0`。
- EventHub publish failure delta 仍为 `0`。

Closure notes:

- 2026-06-26 P0-1 code closure:
  - `workspaceviewcore.Manager` now treats runtime progress-only topics as lightweight heartbeat/status updates:
    - `/run/llm_stream/progress`
    - `/run/llm_usage/recorded`
    - `/step/dispatched`
    - `/step/state/changed`
  - These topics no longer directly schedule full workspace detail rebuilds when the current projected snapshot can be updated in memory.
  - Result, patch, governance, definition, workspace topology, and other evidence-bearing events still trigger projection rebuilds.
  - Tests cover stream progress and step dispatched/state-changed events updating `RunHeartbeat` / `RuntimeExecution` without incrementing workspace rebuild scheduling.
  - Remaining P0 work: reduce payload hydrate volume inside actual detail/run snapshot rebuilds and replace hot `map[string]any` clone paths with compact typed summaries where feasible.
- 2026-06-26 P0-2 code closure:
  - Added a projection-specific workspace snapshot load mode for workorchd projection rebuilds.
  - Projection snapshot rebuilds now skip workspace-level and run-level `/run/llm_usage/recorded` payload hydration.
  - Default run projection snapshots now use compact metadata-only event loading; LLM usage payload hydration is reserved for explicit detail/observability reads.
  - The full workspace snapshot path still preserves existing LLM usage trace and summary behavior for explicit detail/observability reads.
  - The projection loader is wired through `serverState.readWorkspaceProjectionSnapshot`, so session-backed and test-backed read paths remain consistent with normal server state injection.
  - `/api/v1/workorch/workspaces/:workspaceId/snapshot` remains an explicit detail endpoint and reads the full workspace snapshot while returning the current projection status; it overlays projection-owned transient fields such as ViewModel, scheduler diagnostics, and accepted run command status so detail payload does not lose projection state.
  - Tests cover projection snapshot reads not issuing LLM usage payload queries, run projection snapshots using metadata-only event timeline, and HTTP snapshot detail retaining bounded LLM usage traces.
  - Remaining P0 work: reduce non-usage payload hydration and map clone cost in detail/run snapshot rebuilds, and validate live `payloadHydratesDelta` after redeploy.
- 2026-06-26 P0-3 test/contract closure:
  - Closed the stale default run snapshot usage expectation in `TestReadRunSnapshotUsesBoundedProjectedEventRecords`.
  - Default `ReadRunSnapshot` / compact run snapshot now explicitly asserts metadata-only event projection and no LLM usage payload hydration.
  - Explicit `RunObservability` now has a direct regression test proving it performs an LLM usage payload query and produces the expected usage summary.
  - This preserves the resource contract: compact/default status paths stay light, while heavy usage details remain available through explicit observability/detail reads.
- 2026-06-26 P0-4 code closure:
  - Added a `view_compact` run snapshot load mode for workspace projection reads.
  - `WorkspaceProjectionSnapshot` now uses `view_compact` for the selected projection run.
  - The mode bounds selected run steps to the latest `100` while preserving the checkpoint current step even when it falls outside the latest window.
  - The returned compact run snapshot projects step views before merging into workspace projection:
    - step `input` is omitted;
    - raw/unbounded step output is filtered through `ProjectStepOutput`;
    - derived read models are still computed from full in-memory step data before compacting the returned view.
  - This directly reduces the hot `cloneStepViewMap` surface in projection rebuilds without changing internal `ReadRunSnapshot` audit/detail behavior.
  - Tests cover bounded compact steps, current checkpoint step retention, omitted step input, and raw output removal.
- 2026-06-26 P0-5 display contract closure:
  - Live verification after redeploy showed resource pressure improved but display contract still failed:
    - `projectionPendingRebuilds=0`
    - `projectionScheduleDroppedDelta=0`
    - `runtimeEventPublishFailures.totalPublishFailureDelta=0`
    - `payloadHydratesDelta=27`
    - `payloadHydrateBytesDelta=520755`
    - `cpuPercent=74.50`
    - snapshot projection for both observed workspaces omitted `workspaceRevision`.
  - Root cause: the workspace snapshot HTTP handler returned the global projection status even after successfully reading workspace-scoped projected snapshot state.
  - Code closure: `/api/v1/workorch/workspaces/:workspaceId/snapshot` now uses the `StatusForWorkspace` result returned by the projection manager whenever projected snapshot overlay succeeds.
  - This keeps workorchd as the projection owner and avoids TUI/Web fallback inference.
  - Regression: `TestWorkspaceSnapshotEndpointReturnsCompactProjection` now asserts that snapshot response projection contains positive `workspaceRevision`.
  - Live acceptance still requires redeploy and rerun of `scripts/observe-workorch-runtime-closure.sh`.

Likely code areas:

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore`
- `internal/modules/application/workorchd/service/server/workspace_projection.go`
- `internal/modules/application/workorchd/service/server/http_run.go`
- `internal/modules/application/workorchd/service/server/http_response_helpers.go`
- `internal/pkg/repository`

### P1: Compact ViewModel And Status Read Path

Status: closed

Owner: workorchd projection/ViewModel, TUI/Web display contract.

Required closure:

- TUI main view、prompt 和 workspace list 默认只使用 compact projection ViewModel。
- `/api/v1/workorch/workspaces/:workspaceId` 不作为 TUI 主刷新路径；若保留完整 workspace definition 读取，必须与 projection snapshot/display path 明确区分。
- 新增或强化 compact workspace snapshot/status，避免默认返回 template/contextHints/evidencePolicies 等大对象。
- Stale projection 时端侧使用 last ready ViewModel，不本地推断 promptMode。

Acceptance:

- TUI workspace list/main/prompt 显示同一 projection revision 或明确显示 refreshing。
- projection stale 一拍时，promptMode 不跳变为错误状态。
- Web/TUI 默认刷新不读取完整 template/contextHints。
- 自动化测试覆盖 stale projection -> last ready ViewModel display。

Likely code areas:

- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore`
- `internal/modules/application/workorchd/service/server/http_workspace.go`
- `internal/pkg/workorchport`
- `internal/pkg/workorchclient`
- `internal/services/workorch/service/tui`
- `web/`

Closure notes:

- 2026-06-26 P1-1 code closure:
  - TUI remote workspace list already reads `ListWorkspaceProjection`.
  - TUI remote workspace snapshot already reads `GetWorkspaceSnapshotProjection`.
  - `/api/v1/workorch/workspaces/:workspaceId` remains a full workspace definition endpoint and is not the TUI main refresh path.
  - Stale projection snapshot reads now preserve the last ready workspace state and promptMode while exposing stale metadata:
    - `projection.state=stale`
    - `staleReason=source_event`
    - latest `sourceEventRevision`
    - last ready `projectedSourceRevision`
  - This prevents a stale one-frame projection from falsely appearing fully projected or forcing TUI/Web to infer promptMode locally.
  - Tests cover stale projection returning the last ready decision ViewModel, projection metadata carrying the stale source revision, TUI projection/prompt refresh paths, and server projection snapshot paths.
  - Web audit confirms `WorkspaceBoardPage` default refresh reads `listWorkspaceProjection`; `/snapshot` is only used for explicit workspace detail or user-triggered execution request construction.
  - P1 remaining work is folded into P2/P3 when compact status is consumed by detail/status panels.

### P2: Compact Run Convergence Status

Status: closed

Owner: orchestrator/read model projection.

Required closure:

- 为 active run 暴露 compact convergence status。
- status 字段至少包含 currentFocusId、nextActionKind、currentGap、missingEvidenceKinds、accepted/rejected evidence count、latest effective progress、latest contract violation rule/recovery。
- Observation script、TUI `/status`、Web detail 优先读取 compact status，不扫描完整 result/timeline。

Acceptance:

- 观测脚本可以不用 full payload hydrate 判定：
  - positive progress
  - current gap
  - stalled
  - ineffective retry
  - latest contract violation rule
- `coverage_assessment_incomplete` 可在 compact status 中看到其 `recompute_authority` 后续 next action。
- `ws-7...` 这类 missing validation evidence 可以直接显示为 `nextActionKind=validate` 或等价当前动作。

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelcore`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`
- `internal/modules/application/workorchd/service/server/http_run.go`
- `scripts/observe-workorch-runtime-closure.sh`

Closure notes:

- 2026-06-26 P2-1 code closure:
  - Added neutral `ConvergenceStatus` compact DTO to the read model.
  - `RunSnapshot`, `WorkspaceSnapshot`, and `RunStatus` now expose `convergenceStatus`.
  - The compact status is derived only from existing structured projection fields:
    - `GapBaseline.CurrentFocus`
    - `GapBaseline.NextAction`
    - `GapBaseline.TodoItems`
    - accepted/rejected evidence refs
    - `Acceptance.CurrentFocus`
    - `RuntimeExecution.ContractViolation`
    - workspace-level `RunHeartbeat`
  - No template, provider, programming language, command, workspace title, or live workspace ID logic was added.
  - Go client, TUI/backend port DTOs, remote backend mapping, and Web TypeScript DTOs now preserve `convergenceStatus`.
  - Tests cover compact run convergence status projection and workspace heartbeat problem-signal projection.
  - Observation script `scripts/observe-workorch-runtime-closure.sh` now preserves `convergenceStatus` in each workspace summary and uses it before legacy `coverageSummary.currentFocus` / `runHeartbeat` fallback fields for current gap, missing evidence, next action, latest contract violation, stalled, and ineffective retry checks.
  - TUI now stores `ConvergenceStatusInfo` from projection snapshots and run status messages. `/status` and the runtime status bar render compact focus, next action, gap, missing evidence, evidence counts, violation recovery, and heartbeat problem signals without issuing additional reads.
  - Web detail now renders `convergenceStatusDescriptions` in the workspace detail runtime summary so current focus/gap/next action are visible without scanning `GapBaseline` tables.
  - Direct `ReadRunStatus` remains a lightweight status endpoint: it exposes identity/state/runtime violation recovery but does not hydrate full `GapBaseline` history just to compute gap. Full gap status is available through projection snapshot `convergenceStatus`, which is the intended TUI/Web/observation read path.
  - P2 is closed at code level; live resource acceptance still needs to be verified after redeploy with the live observation command.

Validation notes:

- Passed:
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice/readmodelports ./internal/pkg/workorchclient ./internal/pkg/workorchport ./internal/services/workorch/biz/backends -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore ./internal/modules/application/workorchd/service/server ./internal/services/workorch/service/tui -count=1`
  - `npm run build` in `web/workorch-console`
  - `git diff --check`
- Additional P2-2 validation:
  - `go test ./internal/services/workorch/service/tui ./internal/services/workorch/biz/backends ./internal/pkg/workorchport -count=1`
  - `bash -n scripts/observe-workorch-runtime-closure.sh`
  - `npm run build` in `web/workorch-console`
- Additional P0-3 validation:
  - `go test ./internal/modules/application/workorchd/biz/appservice -run TestReadRunSnapshotUsesBoundedProjectedEventRecords -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelports -run 'RunSnapshot|WorkspaceSnapshot|Projection|Observability|Usage' -count=1`
- Additional P0-4 validation:
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelports -run 'ViewCompact|RunSnapshot|WorkspaceProjection' -count=1`
  - `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'RunSnapshot|WorkspaceSnapshot|Detail|Convergence' -count=1`

### P3: Convergence Behavior Guard

Status: closed

Owner: orchestrator convergence authority and contract recovery.

Required closure:

- 对 `coverage_assessment_incomplete` 的后续闭环增加观测和测试：
  - 已归类为 `recompute_authority` 后，下一步必须来自 current authority。
  - 若同一 focus 连续 assessment incomplete，必须切换到 concrete evidence acquisition 或 governed blocker，不应无界继续 assessment。
- 对 missing validation evidence 的 workspace，下一步必须生成 bounded validation graph、accepted validation evidence 或明确 rejected reason。

Acceptance:

- 连续同 focus 的 assessment incomplete 不形成无界 retry loop。
- missing validation evidence 不被全局 assessment 或 read-only summary 关闭。
- 仍保持 core 业务中立，不加入模板/语言/命令特判。

Likely code areas:

- `internal/modules/kernel/orchestrator/biz/runtime_contract_violation.go`
- `internal/modules/kernel/orchestrator/biz/runtime_gap_baseline.go`
- `internal/modules/kernel/orchestrator/biz/runtime_execution_contract.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_state.go`

Closure notes:

- 2026-06-26 P3-1 contract audit:
  - Scope: `coverage_assessment_incomplete` recovery after a failed assessment output has been classified as `recoverability=recompute_authority`.
  - Observed deviation: live/resource audits showed this path can be hard to distinguish from unbounded assessment retry unless dispatch input and stage packet are explicitly proven to be rebuilt from current convergence authority.
  - Expected state: after recompute authority, the next dispatch input must use `convergenceAuthority` / `gapBaseline.nextAction`; if the current authority says validation is missing, the next contract must be `need_validation` with concrete validation evidence required, not another assessment-only output.
  - Owner: core orchestrator convergence authority and dispatch input rebuild.
  - Event path: contract violation -> rejected diagnostic evidence / recovery metadata -> runtime repair dispatch input -> `BuildConvergenceContract` -> `ApplyConvergenceContract` -> `normalizeDispatchInputAuthorityFocus` -> `stagePacket`.
  - Contract gap closed: added a direct regression sample proving stale assessment-only fields and `coverage_assessment_incomplete` recovery metadata are overridden by current authority when the current next action is validation.
  - Boundary check: no template, Soul, language, command, provider, file name, or live workspace ID logic was added.
- 2026-06-26 P3-1 code closure:
  - Added `TestRebuildDispatchInputUsesAuthorityAfterCoverageAssessmentIncomplete`.
  - The test verifies:
    - stale `need_assessment` / `assessmentOnly` input is rebuilt to `need_validation`;
    - `repairObjective=produce_concrete_validation_evidence`;
    - `repairActionPackage.requiredOutput.mustProduceConcreteValidationEvidence=true`;
    - stage packet active focus and required output are aligned to the same authority focus.
  - Existing tests already cover:
    - `coverage_assessment_incomplete` classification as `recompute_authority`;
    - skipping same-contract retry for assessment incomplete and routing to runtime repair;
    - validation focus staying validation after failed assessment;
    - read-only validation contract loops switching to evidence acquisition when target evidence is missing;
    - focus drift rejection before runtime/provider execution.
  - P3 is closed at code/test level. Live convergence acceptance still depends on redeploy observation showing no unbounded same-focus assessment retry.

## Verification Plan

### Unit / Package Tests

```bash
go test ./internal/modules/application/workorchd/biz/appservice/readmodelcore -count=1
go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1
go test ./internal/modules/application/workorchd/service/server -run 'Projection|Snapshot|Metrics|Workspace|Run' -count=1
go test ./internal/services/workorch/service/tui -run 'Projection|Prompt|Workspace|Status' -count=1
go test ./internal/modules/kernel/orchestrator/biz -run 'CoverageAssessmentIncomplete|Convergence|GapBaseline|ContractViolation|Validation' -count=1
```

### Boundary Search

```bash
rg -n "ws-4e83810542ac4d65|ws-7ced70e98ea50c43|software-dev|weather-query|code-review|gopls|go test|npm test|provider=" internal/modules/application/workorchd internal/modules/kernel internal/pkg internal/services/workorch -g '*.go'
rg -n "payloadHydrate|metadataOnly|projection|ViewModel|runHeartbeat|convergenceStatus|sourceEventRevision|projectedSourceRevision" internal/modules/application/workorchd internal/pkg internal/services/workorch -g '*.go'
```

### Live Resource Gate

Use current live baseline as the before value:

- CPU: `251.78%`
- RSS: `784654336` bytes to near `1GB`
- `metadataRecordsScannedDelta=85943`
- `payloadHydratesDelta=2100`
- `payloadHydrateBytesDelta=37258458`
- `writeBytesDelta=36044800`

Target command:

```bash
WORKORCH_BASE_URL=http://127.0.0.1:8080 \
WORKORCH_OBSERVE_WORKSPACE_IDS=ws-4e83810542ac4d65,ws-7ced70e98ea50c43 \
WORKORCH_OBSERVE_DURATION_SECONDS=30 \
WORKORCH_OBSERVE_KEEP_ARTIFACTS=1 \
WORKORCH_OBSERVE_REQUIRE_PROCESS_RESOURCE=1 \
WORKORCH_OBSERVE_REQUIRE_CACHE_SAMPLES=0 \
WORKORCH_OBSERVE_REQUIRE_PPROF=1 \
WORKORCH_OBSERVE_REQUIRE_CPU_PROFILE=1 \
WORKORCH_OBSERVE_MAX_PENDING_REBUILDS=10 \
WORKORCH_OBSERVE_MAX_PROJECTION_SCHEDULE_DROPPED_DELTA=0 \
./scripts/observe-workorch-runtime-closure.sh
```

Pass thresholds after P0/P1:

- `processResource.cpuPercent < 100`
- `payloadHydratesDelta <= 630`
- `payloadHydrateBytesDelta <= 11177537`
- `projectionPendingRebuilds == 0`
- `projectionScheduleDroppedDelta == 0`
- `runtimeEventPublishFailures.totalPublishFailureDelta == 0`
- `stalledRuns == 0`
- `ineffectiveRetryRuns == 0`

Latest local validation after P0-5:

```bash
go test ./internal/modules/application/workorchd/service/server -run 'TestWorkspaceSnapshotEndpointReturnsCompactProjection|Test.*Projection' -count=1
go test ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore -count=1
go test ./internal/modules/application/workorchd/service/server ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore ./internal/pkg/workorchclient -count=1
git diff --check
```

## Done Criteria

- active execution 期间 projection/event IO 不再主导 CPU。
- TUI/Web 默认视图只读 compact ViewModel，不读取完整 workspace definition 或历史 payload。
- projection stale 一拍不会导致 prompt/main view 状态错乱。
- compact convergence status 足以说明当前 focus、next action、gap、latest violation 和 progress。
- 两个 workspace 要么持续产生 accepted evidence，要么进入明确 governed blocker；不得无界高负载推进。
- Core 仍保持业务中立，没有新增模板、语言、provider 或 live workspace 特判。
