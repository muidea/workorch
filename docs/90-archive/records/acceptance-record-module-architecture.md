# Acceptance Record: Module Architecture

Status: active

Type: verification

Last Updated: 2026-06-22

## Purpose

本文件记录对 `module-architecture-code-alignment-checklist.md` 的当前正式验收结果。

判定口径：

- `pass`
  - 当前代码与模块职责设计一致，责任落点和依赖方向成立
- `partial`
  - 主体合同已经落地，但仍有明确的后续结构化收口项
- `doc-ahead`
  - 文档要求已明确，但当前证据不足以证明实现已经完全落地
- `impl-drift`
  - 当前实现与模块职责设计冲突

## Evidence Base

本轮主要检查以下实现与测试证据：

- `application/workorch/main.go`
- `internal/initiators/workorchruntime/*`
- `internal/modules/application/workorchd/biz/bootstrap/*`
- `internal/modules/application/workorchd/biz/appservice/*`
- `internal/modules/application/workorchd/pkg/observed/*`
- `internal/modules/application/workorchd/pkg/presenter/*`
- `internal/modules/application/workorchd/service/server/*`
- `internal/services/workorch/service/tui/*`
- `internal/modules/kernel/*`
- `internal/modules/blocks/*`
- `internal/pkg/repository/*`
- `internal/modules/application/workorchd/service/server/http_test.go`
- 多组 read-model / repository / orchestrator / TUI / boundary tests

## Acceptance Result

| Area | Evidence | Result | Notes |
| --- | --- | --- | --- |
| architecture / repo shape | `go.mod`, `application/workorch`, `internal/modules/*`, `internal/pkg`, `web/workorch-console` | pass | 单 `go.mod`、thin main、initiator + application/kernel/blocks/pkg/web 顶层结构成立。 |
| application / workorchd | initiator, bootstrap, appservice, server tests | pass | initiator 装配 repository/event hub/runtime policy；bootstrap 输出 grouped exports；appservice 组合 activation/recovery/governance/read-model；server 层主要做 parse/validation/response。 |
| observed / presenter / appservice split | `pkg/observed`, `pkg/presenter`, `read_model_service.go` | partial | observed/presenter 已有独立包和测试；旧 observable snapshot printer/builder 残留已从 `pkg/presenter` 移除，避免把 CLI/debug 输出职责混入 read-model mapper。主 snapshot/detail/observability 语义装配仍主要集中在 appservice，后续可分批细化。 |
| kernel workspace / run / result / eventstream | `internal/modules/kernel/{workspace,run,result,eventstream}` + `workorchruntimecontract.CoverageWorkItem` + repository contract | pass | Workspace/Goal/DoD、Run/Step/evidence、Result/Artifact、EventRecord 均有正式模块；`ArtifactContinuation` 已有正式 internal state；`CoverageWorkItem` 已有正式 runtime contract model、repository interface、memory/file/workspace-scoped backend 和 repository contract。 |
| kernel orchestrator / governance | `kernel/orchestrator`, `blocks/decision`, `blocks/approval`, `kernel/governance` | pass | orchestrator 承接 lifecycle/dispatch/repair/replan/context gate/completion gate；decision、approval、governance 分别承接 ticket/resolution、approval formal state/resolve action、auto-governance 协同。 |
| kernel context / capability / memory | `kernel/{soul,skill,mcp,memory}`, memory recall trace repository | pass | Soul/Skill/MCP/Memory 职责清晰；`MemoryRecallTrace` 有正式 model/repository/query；selectedContext provider-facing 组装不留在 kernel。 |
| blocks / runtime adapters | `blocks/{llm,mcp,shell,file,git,web,ssh}` | pass | blocks 承接 step kind 的 side effect/provider interaction/normalization；未拥有 Run lifecycle、Goal/DoD definition 或 accepted Result 最终判定。approval 已不再作为 block adapter。 |
| llm provider assembly | `blocks/llm/biz`, `runtime_output_mode_selector.go` | pass | LLM block 负责 request rendering、context budget、prompt sections、cache plan、provider audit；output mode selector 由 orchestrator 决策并投影，再由 LLM block 渲染/audit。架构文档已明确为双落点合同。 |
| storage / read-model | file/memory repositories, runtime raw content service, read-model tests | pass | itemized persistence、payload summary/ref、artifact continuation persistence、metadata-first projection、raw/detail open contract 均有正式证据。 |
| cross-module consistency | runtime session exports, orchestrator context package, LLM adapter, read-model projection | partial | 主链一致；`CoverageWorkItem` owner storage、read-model owner command 和 dispatch/provider context 前 hydration 已闭合；output-mode 双落点职责表达已收口。剩余差异集中在 observed/presenter 覆盖范围。 |

## Requirement Notes

### 1. Architecture / Repository Shape

已验收：

- 仓库保持单 `go.mod`。
- `main.go` 保持 thin main。
- application、kernel、blocks、internal/pkg、web 的顶层职责未见明显串层。

### 2. Application / Workorchd

已验收：

- runtime initiator 提供 repository、event hub、runtime policy 等共享装配依赖。
- `workorchd/biz/bootstrap` 承接 grouped exports 和 runtime shell 组装。
- appservice 承接 activation、recovery、governance、read-model 和 raw/detail open use-case 组合。
- server handler 层通过 fixture 证明主要职责是 request parse、validation 和 response envelope。

保留项：

- observed / presenter 包已经存在，但 read-model 的大部分语义装配仍在 appservice；后续可按 snapshot、detail page、observability 分批迁移或明确文档为“appservice projection 主导，observed/presenter 辅助”。
- 2026-06-22 已清理 `pkg/presenter` 中无外部调用的旧 observable snapshot `PrintBanner` / `PrintSnapshot` / `BuildSnapshot` 和配套输出 DTO；`pkg/presenter` 当前收窄为 read-model projection 使用的轻量 mapper，HTTP/TUI Presenter 职责继续由 server/client/TUI adapter 承接。
- 2026-06-22 已把 read-model 共享 DTO `StepView` / `ObservedView` 从 `pkg/presenter` 上移到 `readmodelcore`，并把 step/observed projection mapper 下沉到 `readmodelprojection`。`pkg/presenter` 仅保留 outward mapper 和类型 alias。`readmodelcore` / `readmodelprojection` 不再 import `pkg/presenter`，避免 read-model core/projection 反向依赖展示 mapper。

### 3. Kernel Workspace / Run / Result

已验收：

- `blocks/workspace` 承接 formal Workspace / Goal / DoD。
- `kernel/run` 承接 Run / Step / ToolCall / Patch / ValidationResult / RuntimeCheckpoint。
- `blocks/result` 承接 Result / Artifact closeout。
- `kernel/eventstream` 承接 EventRecord append-only persistence / replay input。
- `ArtifactContinuation` 已是正式 run-scoped internal state。

已验收：

- `CoverageWorkItem` 已完成正式 runtime contract model、repository interface、memory/file/workspace-scoped backend 和 repository contract。
- orchestrator coverage ledger 主路径会把 provider-facing `Run.ExecutionContext.coverageWorkItems` 投影同步写入独立 `CoverageWorkItemRepository`。
- read-model 列表通过 EventHub owner command 读取 owner repository，不再直接依赖 appservice 临时拼装。
- dispatch/provider context 前会从 owner repository 水合 `Run.ExecutionContext.coverageWorkItems` 投影，并据此生成 `coverageWorkItemContext` / gap baseline。
- `Run.ExecutionContext.coverageWorkItems` 仅作为 provider-facing/cache-stable 投影载体，不再是正式 owner storage。

### 4. Kernel Orchestrator / Governance

已验收：

- orchestrator 承接 run lifecycle coordination、step dispatch、graph/follow-up、repair/replan、context gate recovery、evidence mapping 和 completion gate。
- decision/approval/governance 没有滑入产品交互语义。
- approval 正式状态与 resolve action 已统一迁移至 `blocks/approval`。

### 5. Kernel Context / Capability / Memory

已验收：

- Soul、Skill、MCP、Memory 各模块职责明确。
- `MemoryRecallTrace` 有正式持久化与查询输入。
- selectedContext 的 provider-facing 组装由 execution context package 与 LLM provider assembly 承接，不由 blocks/memory 直接渲染 prompt。

### 6. Blocks / Runtime Adapters

已验收：

- `blocks/llm`、`mcp`、`shell`、`file`、`git`、`web`、`ssh` 承接对应外部动作和 normalization。
- blocks 不拥有 accepted Result 终态裁决。
- `blocks/approval` 已被彻底迁移出 active architecture；approval 不再作为 block adapter 存在。

### 7. LLM Provider Assembly

已验收：

- provider/model capability、stage-aware budget、request audit、request rendering、prompt sections 和 cache plan 集中在 `blocks/llm`。
- output mode selector 是 orchestrator 决策 + LLM block provider-facing 渲染的双落点。
- incremental-only 字段回落时会清理，request audit 有 mode / selector / artifact phase 维度。

已验收：

- 架构文档已把 “selectOutputMode 的 provider-facing 组装责任” 明确表达为 orchestrator 选择 + LLM block 渲染/audit 的双落点合同，避免误读成单模块独占。

### 8. Storage / Read-Model

已验收：

- repository persistence 已按对象 itemized。
- raw payload ref strategy 与主投影分离。
- snapshot / observability / detail / raw 四层边界清晰。
- summary/snapshot 默认不内嵌 raw payload 或 continuation 正文。
- readable runtime naming 与受控 raw open contract 成立。

## Summary

本轮结论：

- 模块层主干责任分配与代码实现一致。
- 旧 `blocks/approval` 遗留已收口，正式 approval 位于 `blocks/approval`。
- 当前未发现需要立即修复的 `impl-drift`。
- 保留项集中在：
  - observed/presenter 覆盖范围：`partial`

总体结果：`partial`

## Verification Evidence

本轮收口使用以下证据确认当前结论：

- `make verify-boundary-contract`
  - 证明 application/kernel/blocks/Web 可复用层没有重新引入 software-dev、LSP、weather 等 profile/domain 语义。
- `test ! -d internal/modules/blocks/approval && test -d internal/modules/blocks/approval`
  - 证明 `blocks/approval` 已不在 active module tree，正式 Approval 模块位于 `blocks/approval`。
- `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(CoverageWorkItems|CoverageWorkItem|RuntimeOrchestrator_.*Coverage|ApplyOutputModeSelection|SelectOutputMode|HandleArtifactOutput|ValidateArtifact|BuildExecutionContextPackage|ProviderContext|RuntimeOrchestrator_Context|VerifyWorkorchRuntime)' -count=1`
  - 证明 orchestrator、coverage ledger、output mode selector、artifact continuation、context package 等核心模块协作仍通过。
- `go test ./internal/pkg/repository ./internal/pkg/workorchrepo -run 'TestCoverageWorkItemRepositoryContract|TestRepositoryBundleContract|TestWorkspaceScoped' -count=1`
  - 证明 `CoverageWorkItem` repository contract、repository bundle 和 workspace-scoped file backend 接入成立。
- `go test ./internal/modules/application/workorchd/biz/appservice/ownerreadports ./internal/modules/application/workorchd/biz/appservice/readmodelports ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice ./internal/modules/kernel/orchestrator -count=1 -timeout=240s`
  - 证明 `CoverageWorkItem` read-model 列表已通过 EventHub owner command 读取 owner repository，并能继续投影为现有 snapshot 字段形态。
- `go test ./internal/modules/kernel/orchestrator/biz -run 'Test(CoverageWorkItemsSyncProjectionToRepository|EnsureGapBaselineHydratesCoverageContextFromOwnerRepository|CoverageWorkItemsGovernanceOwnerSyncRespectsRequiredCounts|NextDispatchSpecsPersistsGapBaselineBeforeInitialStep|InitialDispatchBuildsStateChangingContractFromGapBaseline)' -count=1 -timeout=120s`
  - 证明 dispatch/provider context 前会从 owner repository 水合 coverage projection，并据此生成 gap baseline、accepted/rejected evidence refs 与当前 todo/focus。
- `go test ./internal/pkg/repository ./internal/modules/application/workorchd/biz/appservice -run 'Test(ArtifactContinuation|MemoryRecallTrace|EventRecord|Decision|Approval|ReadRunSnapshotIncludesArtifactContinuation|ListDecision|ListApproval|ReadRunSnapshotWithBackendReturnsExecutionEvidence|ReadRunObservability|MapTaskProgress|MapResultQuality)' -count=1`
  - 证明 storage/read-model、Decision/Approval、MemoryRecallTrace、EventRecord 与 ArtifactContinuation 的跨模块投影仍通过。
- `git diff --check`
  - 证明本轮文档与当前改动没有 whitespace/formatted diff 问题。
- `go test ./internal/modules/application/workorchd/biz/appservice/readmodelcore ./internal/modules/application/workorchd/pkg/presenter ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice/readmodelports -count=1 -timeout=240s`
  - 证明 `StepView` / `ObservedView` 上移至 `readmodelcore`、step/observed mapper 下沉到 `readmodelprojection` 后，presenter mapper、read-model projection 和 readmodelports DTO 链路仍保持兼容。
- `make verify-boundary-contract`
  - 包含 `readmodelcore` / `readmodelprojection` 不得 import `pkg/presenter` 的边界守护，防止共享 read-model DTO 与投影逻辑再次依赖展示 mapper。

2026-06-22 已完成 `CoverageWorkItem` 正式 model/repository、orchestrator 写路径同步、read-model owner command 读路径和 dispatch/provider context 前 owner hydration 收口。`llm-provider-assembly.md` 已同步明确 output-mode 双落点合同。旧 observable snapshot presenter 残留已从 `pkg/presenter` 移除。当前 `partial` 只记录 observed/presenter 覆盖范围的后续架构细化方向，不代表存在必须立即修复的模块边界漂移。

2026-06-22 已进一步完成 readmodelcore / readmodelprojection / presenter 依赖方向收口：共享 read-model DTO 位于 `readmodelcore`，step/observed projection mapper 位于 `readmodelprojection`，`pkg/presenter` 只做 outward mapper 和 alias；`verify-boundary-contract.sh` 已增加对应防回归规则。当前 `partial` 剩余范围收窄为 snapshot/detail/observability 主语义装配是否继续分批从 appservice 根入口下沉到 readmodelprojection/readmodelports 的结构细化，不再包含 read-model core/projection 依赖 presenter 的反向依赖问题。
