# Module Architecture Code Alignment Checklist

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件用于核对 `30-module-architecture/` 当前模块职责设计是否已经被代码正确承接。

它不重新定义模块边界，只把当前需要逐项核对的实现对齐项拆成可执行清单，
方便后续按模块逐项处理。

## Canonical Sources

核对时以以下文档为准：

- `../30-module-architecture/architecture-overview.md`
- `../30-module-architecture/repository-structure.md`
- `../30-module-architecture/application-workorchd.md`
- `../30-module-architecture/kernel-workspace-run-result.md`
- `../30-module-architecture/kernel-orchestrator-governance.md`
- `../30-module-architecture/kernel-context-capability.md`
- `../30-module-architecture/blocks-runtime-adapters.md`
- `../30-module-architecture/llm-provider-assembly.md`
- `../30-module-architecture/storage-readmodel.md`
- `../20-runtime-flow/overview.md`
- `../50-product-interface/service-api.md`

## Review Goal

这一轮只回答三个问题：

1. 模块职责是否已经在代码中有明确落点。
2. 模块边界是否与 runtime flow、domain model 和 product projection 对齐。
3. 哪些点属于“文档超前”，哪些点属于“实现偏差”。

## Suggested Review Scope

建议优先核对以下代码目录：

- `application/workorch`
- `internal/initiators/workorchruntime`
- `internal/modules/application`
- `internal/modules/kernel`
- `internal/modules/blocks`
- `internal/pkg`
- `web/workorch-console`

## Review Order

建议顺序：

1. architecture overview / repository shape
2. application / workorchd
3. kernel workspace / run / result
4. kernel orchestrator / governance
5. kernel context / capability / memory
6. blocks / runtime adapters
7. llm provider assembly
8. storage / read-model

## 1. Architecture Overview / Repository Shape

重点核对：

- 当前代码树是否仍符合单 `go.mod` 仓库形态。
- 顶层目录职责是否仍为：
  - `application/workorch`
  - `internal/initiators/workorchruntime`
  - `internal/modules/application`
  - `internal/modules/kernel`
  - `internal/modules/blocks`
  - `internal/pkg`
  - `web/workorch-console`
- `main.go` 是否保持 thin main。
- `internal/modules/application`、`kernel`、`blocks`、`internal/pkg` 是否没有明显职责串层。

常见偏差：

- 启动接线逻辑散落到多个无关目录
- `main.go` 直接承接业务编排
- `internal/pkg` 混入业务状态裁决

## 2. Application / Workorchd

重点核对：

- `application/workorch` 是否只负责入口和 lifecycle 触发。
- `internal/initiators/workorchruntime` 是否提供 repository、event hub、runtime policy 等共享装配依赖。
- `workorchd/biz/bootstrap` 是否承接 grouped exports 和 runtime shell 组装。
- `workorchd/biz/appservice` 是否承接：
  - activation / recovery / governance use-case 组合
  - read-model assembly
  - runtime use-case composition
- `service/server` 是否只负责 request parse / validation / response，不复制业务逻辑。
- 是否存在 request-scoped runtime session，且不演化成业务状态拥有者。
- observed / presenter / appservice 三层是否有明确分工，而不是混在单个函数中。

常见偏差：

- handler 层复制业务逻辑
- appservice 直接承接 provider/block 细节
- observed / presenter / DTO 包装没有分层

## 3. Kernel Workspace / Run / Result

重点核对：

- `kernel/workspace` 是否承接 formal Goal / DoD / workspace state。
- `kernel/run` 是否承接：
  - Run / Step / ToolCall / Patch / ValidationResult / RuntimeCheckpoint
  - run-scoped coverage ledger
  - run-scoped continuation
- `kernel/result` 是否承接正式 Result / Artifact closeout。
- `kernel/eventstream` 是否承接 EventRecord append-only persistence/replay 输入。
- `CoverageWorkItem` 和 `ArtifactContinuation` 是否已经在 kernel 有正式持有或等价责任点。
- deliverable artifact 和 process evidence artifact 是否在 kernel/result 归属上被区分。

常见偏差：

- coverage ledger 只在 appservice/read-model 临时拼装
- continuation 只在 orchestrator 内部临时结构存在
- kernel/result 和 kernel/run 对 artifact ownership 边界不清

## 4. Kernel Orchestrator / Governance

重点核对：

- `kernel/orchestrator` 是否承接：
  - Run lifecycle coordination
  - Step dispatch
  - graph follow-up / completion follow-up
  - runtime repair / replan dispatch
  - context gate recovery routing
  - evidence mapping
  - completion gate
- `kernel/decision` 是否只承接 Decision / Resolution 本体。
- `kernel/governance` 是否承接 duplicate failure / repair exhausted / auto-governance 协同。
- approval 正式状态与 recovery allow/deny 输入是否有明确 kernel 责任点。
- decision / approval / governance 是否没有滑入产品交互层语义。

常见偏差：

- context gate routing 落在 appservice 或 llm block，而非 orchestrator
- auto-governance 与 Decision/Resolution 本体混写
- approval 状态和产品按钮语义混在一起

## 5. Kernel Context / Capability / Memory

重点核对：

- `kernel/soul` 是否承接 Soul allowlist / policy validation。
- `kernel/skill` 是否承接 Skill metadata / trigger / selection contract。
- `kernel/mcp` 是否承接 registry / capability metadata / selection policy。
- `kernel/memory` 是否承接：
  - Memory CRUD
  - workspace-scoped recall/search
  - memory write policy
  - `MemoryRecallTrace` 的正式持久化与查询输入
- selectedContext 的 request-side 组装是否没有误留在 kernel，而是继续由 provider assembly 承接。

常见偏差：

- MemoryRecallTrace 没有正式 repository/query 落点
- selectedContext 的 provider-facing 组装落在 kernel
- Soul/Skill/MCP 的 profile-specific 语义回流到基础 kernel contract

## 6. Blocks / Runtime Adapters

重点核对：

- `blocks/llm` / `mcp` / `shell` / `file` / `git` / `web` / `ssh` / `approval`
  是否各自承接对应 step kind。
- blocks 是否只执行 side effect / provider interaction / normalization。
- block 是否没有拥有：
  - Run lifecycle
  - Goal / DoD formal definition
  - accepted Result 最终判定
- `blocks/file` 是否区分 `file/*` 与 `patch/*` 的行为职责。
- approval 是否不再作为 block adapter 存在，正式状态与 resolve 动作统一由 `kernel/approval` 承接。
- 所有 block 是否默认输出 structured metadata + bounded preview + raw ref。

常见偏差：

- block 直接决定 run terminal state
- shell/file/web 输出直接被当成 accepted business outcome
- approval block 侵入治理对象本体

## 7. LLM Provider Assembly

重点核对：

- provider/model capability resolution 是否集中在本模块层。
- stage-aware budget / request audit / request rendering 是否集中在本模块层。
- `selectOutputMode` 的 provider-facing 组装责任是否真实存在。
- request-side stage contract 是否投影：
  - `outputMode`
  - `outputModeSelector`
  - `artifactOutputContract`
  - `artifactContinuationContext`
- mode-specific request/response schema 是否互斥。
- request audit 是否记录 output mode / selector / artifact phase metadata。
- protected field preservation 是否按字段级处理，而不是按大对象粗略判断。

常见偏差：

- selector 逻辑散落在 orchestrator / block / appservice 多处
- incremental-only 字段回落后未清理
- request audit 没有 mode / phase 维度

## 8. Storage / Read-Model

重点核对：

- repository persistence contract 是否 itemized，而不是单个大 JSON map 重写。
- raw payload ref strategy 是否存在，并与主投影分离。
- snapshot / observability / detail / raw 四层是否分清。
- `coverageWorkItems` / `coverageWorkItemContext` 是否有 run-scoped ledger persistence。
- `ArtifactContinuation` 是否有正式 persistence contract 和 metadata-only projection。
- `payloadSummary` / `payloadRef` / artifact URI 是否有稳定 read-open contract。
- summary/snapshot 是否不会默认内嵌 raw payload / continuation 正文。
- path-safe readable naming 是否存在，且不会暴露本地绝对路径。

常见偏差：

- read-model 默认加载 raw payload 大对象
- continuation 正文进入 snapshot/observability
- repository 只支持对象大图重写，不支持 itemized persistence

## 9. Cross-Module Consistency Checks

重点核对以下跨模块一致性：

- application 的 appservice / observed / presenter 是否与 storage/read-model 的投影边界一致。
- orchestrator 的 recovery / completion gate 是否与 kernel/run/result/eventstream 的对象归属一致。
- provider assembly 的 stage-contract 投影是否与 blocks/llm 的执行输入一致。
- storage/read-model 的 metadata-first projection 是否与 service API / Web/TUI 一致。
- kernel/memory 的 recall trace 是否被 appservice / product query 正式消费。

常见偏差：

- appservice 直接拼 raw repository 数据绕过 observed/presenter
- provider assembly 和 llm block 对 stage-contract 字段理解不一致
- storage 文档说 metadata-first，但 API / UI 仍默认读正文

## Classification Rules

核对结果建议分三类：

- `pass`
  - 设计与代码一致，职责落点和依赖方向成立
- `doc-ahead`
  - 文档定义已明确，但代码尚未正式落地
- `impl-drift`
  - 代码已有实现，但模块责任、依赖方向或对象归属与当前设计冲突

## High-Priority Checks

优先检查这些高风险点：

1. `kernel/run` 是否真实承接 run-scoped coverage ledger / continuation。
2. `kernel/orchestrator` 是否真实承接 context gate recovery routing。
3. `kernel/memory` 是否真实承接 `MemoryRecallTrace`。
4. `appservice` / observed / presenter 是否已经分层。
5. `llm-provider-assembly` 是否真实承接 stage-contract projection 和 mode-specific schema。
6. `storage-readmodel` 是否真实承接 metadata-first + raw/detail 分离。

## 2026-05-30 Code Alignment Result

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| architecture / repo shape | `go.mod`, `application/workorch`, `internal/modules/{application,kernel,blocks}`, `internal/pkg`, `web/workorch-console` | pass | 当前仍是单 `go.mod` 仓库；顶层结构与架构文档一致；`application/workorch/main.go` 保持 thin main，只委托 `app.Run`。 |
| application / workorchd | `internal/initiators/workorchruntime`, `internal/modules/application/workorchd/biz/bootstrap`, `biz/appservice`, `service/server` | pass | initiator 负责 repository/event hub/runtime policy 装配；bootstrap 输出 grouped exports；appservice 组合 activation/recovery/governance/read-model；server 层主要做请求解析、校验和响应包装。 |
| observed / presenter / appservice split | `internal/modules/application/workorchd/pkg/observed`, `pkg/presenter`, `biz/appservice/read_model_service.go` | partial | observed/presenter 已有独立包和测试；但主 read-model 语义装配仍主要集中在 appservice，三层尚未完全覆盖所有 snapshot/detail/observability 入口，属于结构继续细化项。 |
| kernel workspace / run / result | `internal/modules/kernel/{workspace,run,result,eventstream}` | partial | Workspace/Goal/DoD、Run/Step/evidence、Result/Artifact、EventRecord 均有正式模块；`ArtifactContinuation` 已有正式 internal state 与 repository。`CoverageWorkItem` 仍为 `Run.ExecutionContext` ledger，尚未独立 model/repository 化。 |
| kernel orchestrator / governance | `internal/modules/kernel/orchestrator`, `internal/modules/kernel/decision`, `internal/modules/kernel/approval`, `internal/modules/kernel/governance` | pass | orchestrator 承接 run lifecycle、dispatch、repair/replan、context gate recovery、completion gate；decision、approval、governance 分别承接 ticket/resolution、approval formal state/resolve action、auto-governance 协同。Approval 已统一迁移至 `kernel/approval`。 |
| kernel context / capability / memory | `internal/modules/kernel/{soul,skill,mcp,memory}`, `internal/pkg/repository/memory_recall_trace_repository.go` | pass | Soul/Skill/MCP/Memory 模块职责清晰；`MemoryRecallTrace` 有正式 model/repository/query，并由 orchestrator step input recall 写入。 |
| blocks / runtime adapters | `internal/modules/blocks/{llm,mcp,shell,file,git,web,ssh}` | pass | blocks 均按 step kind 承接外部动作/provider interaction；未发现 block 直接决定 run terminal state 或 accepted Result 的代码路径。approval 不再属于 block adapter。 |
| llm provider assembly | `internal/modules/blocks/llm/biz`, `internal/modules/kernel/orchestrator/biz/runtime_output_mode_selector.go` | partial | provider request rendering、context budget、prompt sections、cache plan、provider audit 集中在 `blocks/llm`；output mode selector 和 artifact stage contract 由 orchestrator 选择并投影到 step input，再由 LLM block 渲染。文档中“selectOutputMode 的 provider-facing 组装责任”实际跨 orchestrator 与 LLM block 分担。 |
| storage / read-model | `internal/pkg/repository/file`, `internal/pkg/repository/memory`, `internal/modules/application/workorchd/biz/bootstrap/workspace_scoped_repositories.go`, `biz/appservice/runtime_raw_content_service.go` | pass | step/tool/validation/continuation 等 run evidence 已 itemized；event payload 支持 summary/ref；ArtifactContinuation plan/chunk/assembled content 与 metadata 分离；raw/detail 通过受控 read-open 入口打开。 |
| cross-module consistency | `internal/modules/application/workorchd/biz/appservice/runtimecore`, `runtimeports`, `internal/modules/blocks/llm/biz/runtime_adapter.go`, `internal/modules/kernel/orchestrator/biz/execution_context_package.go` | partial | runtime session/EventHub-backed runtime ports、provider assembly、orchestrator context package、read-model 投影主链一致；剩余差异集中在 coverage ledger 独立化、observed/presenter 覆盖范围和 output-mode 职责文档表达。 |

### Remaining Follow-Up

- `CoverageWorkItem` 独立对象化仍是 `doc-ahead`：当前 run-scoped ledger 已可用，但落点仍在 `Run.ExecutionContext.coverageWorkItems`。
- Approval 模型与 service 已迁移至 `internal/modules/kernel/approval`，旧 block 归属已移除；该项 `impl-drift` 已收口。
- `observed` / `presenter` 已存在，但大部分 read-model assembly 仍集中在 appservice；后续可按 snapshot/detail/observability 分批迁移，不应在本轮做大范围搬迁。
- `selectOutputMode` 目前由 orchestrator 基于 runtime policy 和 continuation state 决策，LLM block 负责 provider-facing 渲染/audit。当前实现业务中立且可运行；后续若调整文档，应明确这是“orchestrator 选择 + provider assembly 渲染”的双落点，而不是单模块独占。

## Suggested Output Template

后续逐项核对时，建议按以下格式记录：

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| architecture / repo shape | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| application / workorchd | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| kernel workspace/run/result | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| kernel orchestrator/governance | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| kernel context/capability/memory | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| blocks/runtime adapters | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| llm provider assembly | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| storage/read-model | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| cross-module consistency | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
