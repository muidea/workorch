# Acceptance Record: Domain Model

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件记录对 `domain-model-code-alignment-checklist.md` 的当前正式验收结果。

判定口径：

- `pass`
  - 当前代码与对象层设计一致，语义和归属成立
- `partial`
  - 主体合同已经落地，但仍有明确的后续结构化收口项
- `doc-ahead`
  - 文档要求已明确，但当前证据不足以证明实现已经完全落地
- `impl-drift`
  - 当前实现与文档要求冲突

## Evidence Base

本轮主要检查以下实现与映射证据：

- `internal/modules/blocks/workspace/pkg/models/{goal,dod}.go`
- `internal/modules/blocks/workspace/biz/goal_dod_manager.go`
- `internal/modules/kernel/run/pkg/models/run.go`
- `internal/modules/blocks/result/pkg/models`
- `internal/pkg/workorchruntimecontract/artifact_continuation.go`
- `internal/pkg/repository/artifact_continuation_repository.go`
- `internal/modules/kernel/orchestrator/biz/runtime_coverage_work_items.go`
- `internal/modules/blocks/decision/pkg/models`
- `internal/modules/blocks/memory/pkg/models/memory_recall_trace.go`
- `internal/modules/kernel/eventstream/pkg/models/event_record.go`
- `internal/modules/application/workorchd/biz/appservice/read_model_service.go`
- `internal/modules/application/workorchd/biz/appservice/read_model_types.go`
- `internal/pkg/repository/*`
- `internal/pkg/repository/repository_contract_test.go`

## Acceptance Result

| Area | Evidence | Result | Notes |
| --- | --- | --- | --- |
| Workspace / Goal / DoD | workspace models + `GoalDoDManager` + tests | pass | `deliverableDirectory` / `internalDirectory`、definition revision、previous definition hash/id、lineage state 均有正式落点。 |
| Run | `kernel/run/pkg/models/run.go` + activation/run service tests | pass | `Run.definitionRevision`、`goalId`、`doDId`、`blockingTicketId`、`blockingApprovalId` 均是正式字段。 |
| Result / Artifact | `blocks/result/pkg/models` + read-model/repository tests | pass | Result / Artifact 是正式对象；Artifact 支持 run/step/result 归属，deliverable 与 process evidence 通过 category/owner/projection 分层。 |
| CoverageWorkItem | `runtime_coverage_work_items.go` + `workorchruntimecontract.CoverageWorkItem` + repository contract | pass | 已新增正式 runtime contract model、repository interface、memory/file/workspace-scoped backend 和 repository contract；orchestrator coverage ledger 主路径已同步写入独立 `CoverageWorkItemRepository`，read-model 列表已通过 EventHub owner command 读取 owner repository；生产 dispatch/provider context 前会从 owner repository 水合 `Run.ExecutionContext.coverageWorkItems`，该字段仅作为 provider-facing/cache-stable 投影。 |
| ArtifactContinuation | `artifact_continuation.go` + repositories + tests | pass | `currentStepId`、`outputMode`、sequence/hash/progress/coverage/state 等字段齐备；plan/chunk/assembled 与 metadata-first projection 成立。 |
| Governance | decision/approval models + appservice tests | pass | Ticket/Approval/Resolution 是正式治理对象；`Resolution` 已将 `continuationAction`、`resumeRunId`、`successorRunId` 提升为顶层恢复引用字段，同时保留 `payload` 作为扩展数据。 |
| Memory / MemoryRecallTrace | memory recall trace model + repository + read-model mapping | pass | `selectedMemoryIds`、`candidateMemoryIds`、`excludedMemoryIds`、`retrievedMemoryHits` 已是正式 trace 字段；旧 `retrievedMemoryIds` 仍作为辅助读模型/事件字段存在，但不再是唯一 recall 口径。 |
| Event / Observability | `EventRecord` model + file repo + contract tests | pass | `payload` / `payloadSummary` / `payloadRef` 三层分离成立；大 payload 走 summary + ref，raw detail 通过受控打开路径读取。 |
| API / read-model naming | `read_model_types.go` + Web/TUI/client DTO | pass | `coverageWorkItems`、`coverageWorkItemContext`、`artifactContinuations`、blocking refs、memory recall 字段命名与对象层一致。 |

## Requirement Notes

### 1. Workspace / Goal / DoD

已验收：

- Workspace 明确区分 deliverable 与 internal state 目录。
- Goal / DoD 不是压缩摘要，而是正式对象。
- definition revision、previous definition hash/id、lineage state 在模型、管理器和事件 payload 中都有证据。
- activation 可基于 effective definition 创建新 run。

### 2. Run / Result / Artifact

已验收：

- Run 持有 Goal / DoD 引用、definition revision 和当前治理阻塞引用。
- Result 是正式业务结果对象，不等同于 step output。
- Artifact 支持 run/step/result 归属，并通过 read-model 保持 deliverable/evidence 的投影边界。

### 3. CoverageWorkItem

已验收：

- 运行期存在 coverage ledger，支持 baseline build、revision handling、accepted/rejected evidence update、acceptance gate 和 read-model projection。
- 该 ledger 不回写 formal Goal / DoD。

后续约束：

- `CoverageWorkItem` 已完成正式 model/repository 对象化，并已接入 orchestrator 主路径同步写 owner storage。
- 当前 `Run.ExecutionContext.coverageWorkItems` 仍作为 provider context / `coverageWorkItemContext` / gap baseline 的稳定投影保留，但生产 dispatch 前会先从 owner repository 水合该投影。
- 后续修改 provider context / `coverageWorkItemContext` / gap baseline 生成逻辑时，必须保持现有投影顺序稳定、cache 前缀稳定和 provider context 投影稳定。

### 4. ArtifactContinuation

已验收：

- `ArtifactContinuation` 是正式 runtime internal state。
- continuation 默认只用于恢复、诊断和 metadata-only projection。
- chunk/assembled 正文不进入默认 snapshot / observability。

### 5. Governance

已验收：

- Decision、Approval、Resolution 是正式治理对象。
- blocking refs 从 run/gov 对象投影，不由前端临时拼装。
- replan/resume 语义不替代 successor run 本体。

保留项：

- 无。Resolution 的恢复引用字段已经从 payload 提升为顶层字段，payload 仅保留扩展数据用途。

### 6. MemoryRecallTrace

已验收：

- recall trace 是正式过程证据对象，有 repository/query/read-model 落点。
- selected/candidate/excluded/retrieved hits 已表达 recall 选择过程。
- activation、step input、recovery recall 可通过 trace 追踪。

### 7. Event / Observability

已验收：

- EventRecord 支持 payload、payloadSummary、payloadRef。
- Outcome / TaskProgress 是投影，不反向覆盖正式 Run / Result 状态。
- RunObservability 消费 summary/ref，不默认把 raw payload 作为主视图。

## Summary

本轮结论：

- 对象层主干已经完成代码对齐。
- 当前未发现需要立即修复的 `impl-drift`。
- 当前无 `CoverageWorkItem` owner/read-model/provider dispatch 主路径缺口；`Run.ExecutionContext` 仅作为 provider-facing/cache-stable 投影保留。

总体结果：`pass`

## Verification Evidence

本轮收口使用以下证据确认当前结论：

- `make verify-boundary-contract`
  - 证明当前核心 runtime / block / Web 可复用层没有重新引入 profile/domain 特化语义。
- `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(CoverageWorkItems|CoverageWorkItem|RuntimeOrchestrator_.*Coverage|ApplyOutputModeSelection|SelectOutputMode|HandleArtifactOutput|ValidateArtifact|BuildExecutionContextPackage|ProviderContext|RuntimeOrchestrator_Context|VerifyWorkorchRuntime)' -count=1`
  - 证明 coverage ledger、artifact continuation、context package、output mode、runtime verifier fixture 等对象相关主链仍通过。
- `go test ./internal/pkg/repository ./internal/modules/application/workorchd/biz/appservice -run 'Test(ArtifactContinuation|MemoryRecallTrace|EventRecord|Decision|Approval|ReadRunSnapshotIncludesArtifactContinuation|ListDecision|ListApproval|ReadRunSnapshotWithBackendReturnsExecutionEvidence|ReadRunObservability|MapTaskProgress|MapResultQuality)' -count=1`
  - 证明 repository contract、read-model projection、Decision/Approval、MemoryRecallTrace、EventRecord、ArtifactContinuation 相关对象证据仍成立。
- `go test ./internal/modules/blocks/decision/... ./internal/pkg/events ./internal/pkg/repository ./internal/modules/application/workorchd/biz/appservice/readmodelports ./internal/services/workorch/biz/backends ./internal/services/workorch/service/tui -count=1 -timeout=180s`
  - 证明 Resolution 顶层恢复引用字段在 decision model、EventHub payload、repository contract、read-model port、remote backend 与 TUI port 链路中保持一致。
- `go test ./internal/modules/application/workorchd/biz/appservice/governanceports ./internal/modules/application/workorchd/biz/appservice/... ./internal/pkg/workorchclient ./internal/pkg/workorchport ./internal/services/workorch/... -count=1 -timeout=240s`
  - 证明 appservice governance command、read-model projection、client/port DTO 与 process service 消费链路保持一致。
- `make console-build`
  - 证明 Web DTO 更新后前端类型与构建仍通过。
- `git diff --check`
  - 证明本轮文档与当前改动没有 whitespace/formatted diff 问题。
- `go test ./internal/pkg/repository ./internal/pkg/workorchrepo -run 'TestCoverageWorkItemRepositoryContract|TestRepositoryBundleContract|TestWorkspaceScoped' -count=1`
  - 证明 `CoverageWorkItem` repository contract、bundle 持久化和 workspace-scoped file backend 接入成立。
- `go test ./internal/modules/application/workorchd/biz/bootstrap ./internal/pkg/repository/memory ./internal/pkg/repository/file -count=1`
  - 证明 bootstrap exports、memory/file backend 编译与基础 repository 包回归通过。
- `go test ./internal/modules/application/workorchd/biz/appservice/ownerreadports ./internal/modules/application/workorchd/biz/appservice/readmodelports ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/orchestrator -count=1 -timeout=240s`
  - 证明 `CoverageWorkItem` read-model 列表已通过 EventHub owner command 读取 owner repository，并能继续投影为现有 snapshot 字段形态。
- `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(CoverageWorkItemsSyncProjectionToRepository|EnsureGapBaselineHydratesCoverageContextFromOwnerRepository|CoverageWorkItemsGovernanceOwnerSyncRespectsRequiredCounts|NextDispatchSpecsPersistsGapBaselineBeforeInitialStep|InitialDispatchBuildsStateChangingContractFromGapBaseline)' -count=1 -timeout=120s`
  - 证明 dispatch/provider context 前会从 owner repository 水合 coverage projection，并据此生成 gap baseline、accepted/rejected evidence refs 与当前 todo/focus。

2026-06-22 已完成 Resolution 顶层恢复引用字段代码收口，并完成 `CoverageWorkItem` 正式 model/repository、orchestrator 写路径同步、read-model owner command 读路径和 dispatch/provider context 前 owner hydration 收口；`Run.ExecutionContext` 中的 coverage 字段保留为 provider-facing/cache-stable 投影，不再是正式 owner storage。
