# Batch 4 Module Migration Audit

Status: active

Type: audit

Last Updated: 2026-05-29

## Scope

本审计记录 `Batch 4` 建立模块架构层时识别出的三类原始问题及当前收口状态：

- 冲突和不一致
- 原内容不完整项与当前收口记录
- 无法完整衔接的内容断点

## 1. 冲突和不一致

### 1.1 旧架构文档与当前代码树存在漂移

`architecture-overview.md` 中仍存在：

- kernel 数量描述过时
- blocks 数量描述过时
- 依赖清单和 UI 依赖描述过时
- 已注册 step kind 不完整

处理结论：

- 本批只抽稳定模块边界
- 旧架构文档后续应继续收缩并最终退出主入口

### 1.2 结构文档与架构文档职责重叠

`repository-structure.md` 和 `architecture-overview.md` 都在描述：

- 目录树
- 模块落点
- 模块职责

处理结论：

- `repository-structure.md` 只保留目录/模块形状
- `architecture-overview.md` 只保留稳定层级和职责

### 1.3 LLM 预算/auto profile 文档兼具设计与实现双重角色

`llm-provider-assembly.md` 和
`llm-provider-assembly.md` 既像设计，又像实现说明。

处理结论：

- 本层统一将其视为 provider assembly 的实现说明
- 具体 core contract 仍由 `00-core/` 裁决
- `llm-provider-assembly.md` 已补入 provider resolution、request rendering direction 和 provider-facing budget/audit 责任

## 2. 原内容不完整项与当前收口记录

### 2.1 kernel 分组原较粗，已完成当前收口

当前模块层只建立了：

- `kernel-workspace-run-result.md`
- `kernel-orchestrator-governance.md`
- `kernel-context-capability.md`

原缺口：

- 更细的 `eventstream` / `result` / `run evidence` 责任拆分
- governance 内 `decision` 与 `approval` 的实现边界细化

#### 2026-06-22 closure: kernel eventstream / result / governance split

收口范围：

- `docs/30-module-architecture/kernel-workspace-run-result.md`
- `docs/30-module-architecture/kernel-orchestrator-governance.md`
- `internal/modules/kernel/eventstream`
- `internal/modules/blocks/result`
- `internal/modules/blocks/decision`
- `internal/modules/blocks/approval`
- `internal/modules/kernel/governance`

当前结论：

- `kernel/eventstream` 已作为正式 EventRecord 持久化与 replay 输入模块存在，边界是不直接决定 workspace/run outcome 语义。
- `blocks/result` 已作为 Result / Artifact 正式业务结果模型和 persistence contract 模块存在，边界是不承担 provider request、runtime block 调用或 Web/TUI 展示策略。
- `blocks/decision` 与 `blocks/approval` 已分别承载 Decision Ticket / Resolution 与 Approval formal state，不再混在单一 governance 大模块中。
- `kernel/governance` 保留治理集成与 auto-governance 协同责任，不替代 `run`、`result` 或产品交互层。

验证：

- `go test ./internal/modules/kernel/eventstream ./internal/modules/blocks/result ./internal/modules/blocks/decision ./internal/modules/blocks/approval ./internal/modules/kernel/governance -count=1 -timeout=180s`

边界结论：

- 本次核对不需要新增 core 业务逻辑；当前代码已满足该审计项要求。
- 后续仍可继续细化 run evidence 子对象的实现文档，但这不再是“kernel 分组过粗”的阻塞项。

### 2.2 block adapter 原缺注册矩阵，已完成当前收口

当前 `blocks-runtime-adapters.md` 只抽出 block 共同责任，
原缺口：

- step kind -> block kind 映射矩阵
- capability boundary
- output normalization 差异

补充说明：

- 当前 `blocks-runtime-adapters.md` 已迁入 high-level step kind mapping 与 output normalization 方向。
- step kind 注册矩阵已完成第一轮代码和文档收口，见下方 closure record。后续若新增 step kind，必须同时更新代码矩阵、block executor 和本架构文档。

#### 2026-06-22 closure: runtime step kind registration matrix

收口范围：

- `internal/modules/kernel/orchestrator/runtime_registration_matrix.go`
- `internal/modules/kernel/orchestrator/plugin.go`
- `internal/modules/kernel/orchestrator/runtime_registration_matrix_test.go`
- `docs/30-module-architecture/blocks-runtime-adapters.md`

实际调整：

- orchestrator framework module setup 不再手写多段 step kind 注册循环，统一通过 `runtimeStepKindRegistrations()` 注册。
- 注册矩阵显式记录 `stepKind`、`runtimeOwner`、`executorModule` 和 responsibility。
- `blocks-runtime-adapters.md` 已对齐当前真实注册：
  - `llm/assessment_only` 不是独立 step kind，而是 `llm/workspace_execution` 的 runtime mode / stage contract。
  - `patch/preview`、`patch/apply`、`patch/revert` 当前由 `kernel/orchestrator` controlled patch executor 承接。
  - 其他外部动作 step kind 仍由对应 `blocks/*` executor 承接。

验证：

- `go test ./internal/modules/kernel/orchestrator -run 'TestRuntimeStepKindRegistrationMatrixMatchesCurrentRuntimeExecutors|TestModule' -count=1 -timeout=180s`
- `go test ./internal/modules/blocks/... ./internal/modules/kernel/orchestrator -count=1 -timeout=240s`
- `make verify-boundary-contract`

边界结论：

- 本次收口只固化 step kind 注册来源和文档矩阵，不新增业务策略。
- `patch/*` 仍是可选后续迁移项；若迁移到 `blocks/file`，必须同时迁移 approval、patch evidence、repair contract 和 result gate 关联能力，不能只搬文件写入 helper。

### 2.3 storage/readmodel 原缺 snapshot/read endpoints 映射，已完成当前收口

当前 `storage-readmodel.md` 只抽稳定存储边界，
原缺口：

- snapshot projection 结构
- paginated detail endpoint 支撑关系
- payload ref 打开链路

补充说明：

- 当前 `storage-readmodel.md` 已迁入 itemized runtime evidence storage、raw payload ref strategy、projection-oriented read model，以及 file backend 原子写 / append-only 基本方向。
- `storage-readmodel.md` 的稳定内容已拆到 `../10-domain-model/event-observability.md` 和 `storage-readmodel.md`。
- path-safe readable naming 与 filesystem segment overflow fallback 已补充到 `storage-readmodel.md` 并完成代码收口，见下方 closure record。
- full repository contract 与代码落点映射已完成第一轮核对，当前正式 repository contract 位于 `internal/pkg/repository`，workspace-scoped file repository set 位于 `internal/pkg/workorchrepo`，见下方 closure record。后续若继续扩展，应只补新增 repository 类型的映射，不再重复拆解已存在 contract。

#### 2026-06-22 closure: path-safe readable naming / segment overflow fallback

收口范围：

- `internal/pkg/runtimepath`
- `internal/pkg/repository/file`
- `docs/30-module-architecture/storage-readmodel.md`

实际调整：

- `runtimepath` 新增 bounded safe segment/path 能力：
  - `DefaultMaxSegmentLen=160`
  - `SafePathWithMax`
  - `SafeSegmentsWithMax`
  - `TruncateSafeSegment`
- `SequenceScopedPath` 现在对 scope segment 和 sequence item directory segment 都应用 bounded segment 规则。
- file repository 普通 item JSON 路径和 step/tool/validation 这类 run-scoped sequence metadata 目录复用同一 bounded segment 规则。
- 超长 segment 使用“可读前缀 + 稳定 hash suffix”作为内部 persistence path fallback；正式对象 identity 仍来自 repository metadata 中的原始 ID / scope。

验证：

- `go test ./internal/pkg/runtimepath ./internal/pkg/repository/file -run 'Test(SafePath|Sequence|StepRepositoryStoresLongReadableIDs|DecisionResolutionRepositoryStoresLongReadableIDs)' -count=1 -timeout=120s`
- `go test ./internal/pkg/repository -run 'Test.*RepositoryContract.*File|Test.*RepositoryContract.*Memory' -count=1 -timeout=240s`

边界结论：

- 本次收口属于 shared repository/path 基础设施，不属于 runtime core 业务策略。
- 没有引入 workspaceTemplate、Soul、语言、provider、文件名或 live workspace 特判。
- 对外 ref / URI 的产品可读命名仍由 `storage-readmodel.md` 的 raw/detail/open contract 约束；内部 hash fallback 不暴露为业务身份。

#### 2026-06-22 closure: repository contract / workspace-scoped code landing

收口范围：

- `internal/pkg/repository`
- `internal/pkg/repository/file`
- `internal/pkg/repository/memory`
- `internal/pkg/workorchrepo`
- `docs/30-module-architecture/storage-readmodel.md`

当前结论：

- repository interface contract 集中在 `internal/pkg/repository`，覆盖 workspace、goal、DoD、run、step、tool-call、patch、validation-result、runtime-checkpoint、coverage-work-item、artifact-continuation、result、artifact、event-record、decision、approval、memory、soul、skill、MCP server/capability、workspace-template 等正式模型。
- file / memory backend 的具体实现分别落在 `internal/pkg/repository/file` 与 `internal/pkg/repository/memory`。
- workspace-scoped file repository set 统一落在 `internal/pkg/workorchrepo`，负责把 workspace scope 绑定到 run/result/artifact/event/governance 等可能跨 workspace 重名的正式对象读取和写入路径。
- `storage-readmodel.md` 已补充 snapshot projection structure、detail endpoint support mapping、repository/read-open contract、workspace-scoped run identity、run-scoped coverage ledger、artifact continuation projection 等稳定设计点。

验证：

- `go test ./internal/pkg/repository ./internal/pkg/workorchrepo -count=1 -timeout=240s`

边界结论：

- 本次核对属于 repository/readmodel 基础设施边界收口，不引入 template、Soul、语言、provider 或 live workspace 特判。
- 后续新增 repository 类型时，必须同步补充 `internal/pkg/repository` contract、对应 backend、workspace scope 行为和 `storage-readmodel.md` 映射。

### 2.4 application-workorchd 原缺 route 层与 session 层细化，已完成当前收口

当前 `application-workorchd.md` 已迁入：

- grouped exports
- request-scoped runtime session 方向
- observed / presenter / read-model 分层

原缺口：

- request-scoped runtime session
- bootstrap exports 分组
- observed/presenter/read-model 的更细边界

补充说明：

- `application-workorchd.md` 的稳定内容已迁入 `application-workorchd.md`
- lifecycle/context-first 升级已明确归入应用层/框架接线边界，而非 runtime core contract

#### 2026-06-22 closure: application-workorchd route/session/read-model boundary

收口范围：

- `internal/modules/application/workorchd/biz/bootstrap`
- `internal/modules/application/workorchd/biz/appservice/runtimecore`
- `internal/modules/application/workorchd/biz/appservice/runtimeports`
- `internal/modules/application/workorchd/biz/appservice/readmodelcore`
- `internal/modules/application/workorchd/biz/appservice/readmodelports`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection`
- `internal/modules/application/workorchd/biz/appservice/workspaceviewcore`
- `internal/modules/application/workorchd/pkg/observed`
- `internal/modules/application/workorchd/pkg/presenter`
- `internal/modules/application/workorchd/service/server`

当前结论：

- bootstrap exports 已按当前依赖方向稳定为 `Repositories` 与 `Infrastructure` 两组；`Runtime` face 不放回 bootstrap，避免 bootstrap 反向依赖 appservice 形成循环边界。
- request-scoped runtime session 已落在 `appservice/runtimecore`，`runtimeports` 通过 EventHub action/read port 访问 owner module，不持有正式 workspace/run/governance 状态。
- shared backend session 仅复用 file backend runtime access 与 EventHub identity；正式状态仍由 owner modules / repositories 持久化。
- HTTP server route 层已拆成 `http_workspace.go`、`http_run.go`、`http_governance.go`、`http_events.go`、`http_read.go`、`http_write.go` 等 focused route 文件；handler 负责 path/query/body 校验、调用 appservice/projection、输出 response/SSE。
- workspace projection core 已落在 `appservice/workspaceviewcore`；server 只接线 manager lifecycle、读取 projected list/detail、输出 REST/SSE。
- read model 已拆成：
  - `readmodelcore`：DTO、query、reader interface 和 read-model 类型。
  - `readmodelports`：EventHub owner reader 适配与 owner-scope 读取。
  - `readmodelprojection`：snapshot/detail/context/task/result/artifact/coverage 等投影计算。
  - `pkg/observed`：事件/状态观测 snapshot。
  - `pkg/presenter`：输出层 mapping helper。
- 本次代码收口移除了 `workspaceviewcore.Store` 对 `pkg/presenter` 的依赖，改为直接使用 `readmodelcore.StepView`，避免 projection store 反向依赖 presenter 输出包装层。

验证：

- `rg -n "internal/modules/kernel/.*/(biz|service|repository)" internal/modules/application/workorchd/service/server internal/modules/application/workorchd/biz/appservice internal/modules/application/workorchd/pkg/observed internal/modules/application/workorchd/pkg/presenter -g '*.go'`
- `rg -n "pkg/presenter" internal/modules/application/workorchd/biz/appservice/readmodelcore internal/modules/application/workorchd/biz/appservice/readmodelprojection internal/modules/application/workorchd/biz/appservice/workspaceviewcore -g '*.go'`
- `go test ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice/runtimecore ./internal/modules/application/workorchd/biz/appservice/runtimeports ./internal/modules/application/workorchd/biz/appservice/readmodelcore ./internal/modules/application/workorchd/biz/appservice/readmodelports ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore ./internal/modules/application/workorchd/pkg/observed ./internal/modules/application/workorchd/pkg/presenter ./internal/modules/application/workorchd/service/server -count=1 -timeout=240s`

边界结论：

- 本次收口属于应用层 route/session/projection/read-model 边界整理，不新增 workspaceTemplate、Soul、语言、provider 或 live workspace 特判。
- bootstrap 不再被要求承载 appservice Runtime face；Runtime/session 归 appservice focused packages，bootstrap 只输出 repository/infrastructure grouped dependencies。
- Web/TUI 仍应消费 workorchd-owned projection/DTO，不应在端侧重建 workspace 业务状态机。

### 2.5 llm-provider-assembly provider 细化原缺口，已完成当前收口

当前 `llm-provider-assembly.md` 已迁入：

- provider resolution
- request rendering direction
- provider-facing budget / audit 责任

原缺口：

- provider-specific cache mapping
- request-log verifier
- model capability table / revision cache 的更细实现落点

补充说明：

- `llm-provider-assembly.md` 与 `llm-provider-assembly.md` 的稳定内容已继续迁入本文件
- provider-specific cache mapping、request-log verifier、model capability table / revision cache 的代码落点已完成第一轮收口，见下方 closure record。后续如继续演进，应只围绕新增 provider surface、wire contract 或 request audit 字段补充 focused 测试，不再按“HTTP provider 只能是 OpenAI-compatible”这一旧假设处理。

#### 2026-06-22 closure: LLM provider assembly / cache capability surface

收口范围：

- `internal/modules/blocks/llm/pkg/models/provider_config.go`
- `internal/modules/blocks/llm/pkg/providerconfig`
- `internal/modules/blocks/llm/pkg/common`
- `internal/modules/blocks/llm/biz/provider_config.go`
- `internal/modules/blocks/llm/biz/provider_runtime_config.go`
- `internal/modules/blocks/llm/biz/provider_prompt_projection.go`
- `internal/modules/blocks/llm/biz/provider_cache_plan.go`
- `internal/modules/blocks/llm/biz/sdk_provider.go`
- `internal/modules/blocks/llm/biz/http_provider.go`
- `internal/modules/blocks/llm/biz/runtime_adapter.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/llm_usage_*`
- `docs/30-module-architecture/llm-provider-assembly.md`
- `docs/30-module-architecture/llm-cache-improvement-design.md`

当前结论：

- Provider config 已显式承载 `apiSurface`、`cacheMode`、`promptCacheKey`、`messageCacheControl`、`cacheUsageFields`，并映射到 `ProviderCacheCapability`。
- 调用面以 wire/SDK surface 为准：
  - `openai_sdk`
  - `anthropic_sdk`
  - `openai_compatible_http`
  - `anthropic_compatible_http`
- `HTTPProvider` 已按 `apiSurface` 分发 OpenAI-compatible Chat Completions 与 Anthropic-compatible Messages 两套 request shape；未知 HTTP surface 会在提交 provider 请求前失败。
- OpenAI-compatible HTTP 默认不发送 `prompt_cache_key` / `cache_control`；只有显式声明 `prompt_cache_key` 或 `compatible_declared` 且 capability 允许时才发送 `prompt_cache_key`。
- Anthropic SDK / Anthropic-compatible HTTP 只有在 capability 允许且 mode 为 `anthropic_cache_control` 或 `compatible_declared` 时，才在 stable prefix block 上发送 `cache_control`。
- OpenAI SDK 当前不伪造 `prompt_cache_key` wire 字段；runtime adapter 通过 `cacheMarkersSent.promptCacheKeyUnsupported=true` 记录该能力暂未在当前 Chat Completions SDK path 下发送。
- provider-facing stable prefix 已由连续 `C0` / `C1` / `C2` 渲染；`ProviderCachePlan.RenderedStablePrefixHash` 和 `cacheMarkersSent.renderedStablePrefixHash` 用于审计 rendered bytes，不再混用仅代表 C0 的 `StablePrefixHash`。
- `runtime_adapter` 已输出 provider/model/apiSurface、native/estimated usage、cacheUsage、cacheMarkersSent、request/response/stream/usage refs；read-model projection 已消费 `cacheMarkersSent` 和 provider/model breakdown。

验证：

- `go test ./internal/modules/blocks/llm/biz ./internal/modules/blocks/llm/pkg/providerconfig ./internal/modules/blocks/llm/pkg/models ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -run 'Test.*(Provider|Cache|LLMUsage|ContextBudget|StablePrefix|HTTPProvider|OpenAI|Anthropic)' -count=1 -timeout=240s`

边界结论：

- 本次收口只处理 provider-facing assembly / adapter / observability，不把 provider 私有字段上浮到 core runtime、flow、product 或 domain 层。
- provider cache 策略不按 DeepSeek、ai-proxy 或其它后端品牌推断；后端差异只能通过显式 `apiSurface` / cache capability / usage fields 表达。
- Cache 观测字段只作为 observability/read-model evidence，不参与 Goal/DoD、Result/Artifact 或恢复状态裁决。

## 3. 无法完整衔接的内容断点

### 3.1 旧文档大量混入历史实现记录

`repository-structure.md`、`application-workorchd.md`、
`storage-readmodel.md` 都混有：

- 历史收口记录
- 验证命令
- 实施步骤

这使模块层当前只能抽出“稳定职责”，不能直接完整迁移原文。

### 3.2 旧综合状态文档仍是当时部分模块落点事实来源

例如：

- route registration
- paginated detail endpoints
- read-model bounded projection

这些内容在当时仍分散在旧综合状态文档里，
模块层后续还要继续从中抽取稳定实现边界。

### 3.3 provider request rendering 规则仍深埋 core/flow 文档

虽然已建立 `llm-provider-assembly.md`，
但 request rendering、cachePlan、request audit 的细化规则仍大量散落在：

- `../00-core/runtime-stage-contract.md`
- `../20-runtime-flow/context-assembly-flow.md`
- `../00-core/runtime-request-cache-note.md`

后续必须继续从 core/flow 中抽取“实现落点”部分，才能让本层完整闭合。
