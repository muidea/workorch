# Model-Aware LLM Context Budget Todo

Status: active

Type: roadmap

Last Updated: 2026-05-30

## Stable Definition Entry

以下稳定定义已迁入正式结构，不再由本 roadmap 文档重复维护正文：

- [llm-provider-assembly.md](../../30-module-architecture/llm-provider-assembly.md)
- [runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)
- [context-assembly-flow.md](../../20-runtime-flow/context-assembly-flow.md)
- [llm-provider-assembly.md](../../30-module-architecture/llm-provider-assembly.md)
- [application-workorchd.md](../../30-module-architecture/application-workorchd.md)
- [service-api.md](../../50-product-interface/service-api.md)
- [core-runtime-verifier.md](../../60-verification/core-runtime-verifier.md)
- [service-acceptance.md](../../60-verification/service-acceptance.md)

## Purpose

本文件只保留尚未完全收口的 context budget / provider failure / usage
projection follow-up。

它不再承接以下稳定正文：

- core context contract
- context bundle / gate / semantic admission
- provider assembly / request audit 主体设计
- TUI/Web/API 的基础投影语义
- 已完成能力的 dated closure 记录

## Current Stable Baseline

当前已经稳定成立的基线是：

- 按 provider/model 能力动态计算输入预算，而不是使用单一全局上限
- 在 provider submit 前执行分层压缩、阶段预算匹配和 context gate
- `auto` / `manual` 两类预算模式已定型
- `contextBudget`、request audit、LLM usage、stream progress 已进入正式运行期投影
- provider timeout / context-too-large / rate-limit 已进入结构化失败分类和恢复链

这些内容的正式定义以后续查看上述 stable definition entry 为准。

## 本轮核对结果

### 已完成并已迁入正式结构

- `CTX-01 Provider/Model Context Profile` 已接入配置链路：
  - `workorchconfig.LLMProviderConfig`
  - `llm.ProviderConfig`
  - `RedactedProviderConfig`
- `CTX-02 Token Budget Estimation` 已接入预算链路：
  - auto 模式使用 `contextWindowTokens` 推导 `reservedOutputTokens`、`maxInputTokens`、`bytesHardLimit`
  - manual 模式允许显式 `reservedOutputTokens`、`maxInputTokens`、`bytesHardLimit`
  - 使用 `estimatedTokensPerByte` 做保守 token 估算
  - token 超预算时触发同一套分层收缩
- `CTX-03 Priority-Based Context Compaction` 已收口基础版：
  - 收缩阶段引入 `P0/P1/P2` 优先级保留策略
  - 在深度收缩阶段优先保留 Goal/DoD/治理与运行态关键字段
- `CTX-04 Stage-Aware Context Budget` 已收口基础版：
  - `planning/execution/decision/approval/validation/artifact` 使用不同的 executionContext 保留键集
  - 深度收缩阶段保留 stage 关键上下文，避免无差别截断
- `CTX-05 Adaptive Shrink on Provider Failure` 已收口基础版：
  - `provider_timeout` 重试链路按 `providerRetryAttempt` 递进触发预收缩
  - 第 2 次及以上重试默认直接进入更激进上下文收缩层级
- `CTX-06 Traceable Context References` 已收口基础版：
  - `contextBudget` 新增 `sourceRefs`
  - 压缩阶段会记录关键输入段引用（`input.*` / `input.executionContext.*`）
- `CTX-07/CTX-10 Observability + Read Model Projection` 已收口基础版：
  - `llm/workspace_execution` step 输出包含结构化 `contextBudget`
  - `RunSnapshot/RunObservability` 增加 `llmContextBudgets` 投影，web/tui 共用同一读模型
  - `workorchclient` / `workorchport` / remote backend 已贯穿 `llmContextBudgets`，TUI 优先使用正式 context budget trace，而不是从 usage trace 的内嵌字段派生最新上下文。
  - Web `LLMContextBudgetCard` 已展示 over-budget、context utilization、profile mode、capability source、stage budget 等细粒度元数据。
  - `RunSnapshot` / `WorkspaceSnapshot` / `RunObservability` 已新增 `llmContextBudgetSummary`，由 workorchd read-model 统一聚合 trace total、压缩/超预算/P0 overflow/Goal-DoD 压缩、最大压缩级别、最大/平均利用率和最大输入规模；client/port/Web/TUI 共用该投影。
  - `llmContextBudgetSummary` 已补充最近窗口聚合与利用率趋势字段（recent trace/window、recent avg/max utilization、recent max input tokens、trend），由 workorchd read-model 统一计算并供 Web/TUI 展示。
- `CTX-08 Provider Failure Policy and Failover` 已收口基础版：
  - provider 失败细分类已扩展：`provider_timeout`、`context_too_large`、`rate_limited`、`auth_or_config_error`、`provider_unavailable`
  - `RuntimeAdapter` 支持按 provider 配置启用 failover，并按 `failoverProviders` / default provider / 其他可用 provider 顺序执行多级候选切换
  - failover 候选会跳过不可用 provider、缺失 provider runtime、以及不支持当前 operation 的 provider，并记录中立 attempts / failureClass / stage / budget 元数据
  - failover 候选已接入运行期内存健康窗口；连续 transient provider failure 会临时打开候选熔断窗口，后续 failover 在窗口内跳过该候选，并仅输出中立 skipped count/reason，不把 provider/model 动态绑定写入业务恢复状态
  - retryable provider failure 已扩展到 `provider_timeout` / `context_too_large` / `rate_limited`
- `CTX-09 Multi-Client Governance Semantic Normalization` 已收口基础版：
  - 新增治理语义归一能力（`kernel/governance/pkg/common`），统一推断 decision/approval 语义意图
  - `appservice` 侧统一接入 decision resolution 归一，支持 `resolutionType=auto` + 文本意图推断
  - 归一结果写入 `resolutionPayload.governanceSemantic`（`resolutionType/intent/confidence/rawInput/source/inferred`）
  - TUI 本地 decision/approval 文本判断改为复用同一治理语义归一逻辑
  - batch approval 支持语义 action 文本归一到 `approve/reject`
  - 单条 approve/reject endpoint、`workorchclient`、`workorchport`、remote/eventhub backend 与 TUI approval 输入已支持可选语义 payload 追溯；归一结果写入 approval owner state 的 `context.governanceSemantic`
- `CTX-12 Streaming LLM Progress Event Projection` 已收口基础版：
  - 新增统一事件主题 `TopicRunLLMStreamProgress`（`/run/llm_stream/progress`）
  - orchestrator 在 LLM step 完成/失败后从 `streamEvents` 聚合 `chunkCount/receivedBytes/elapsedMs/status` 并发布 EventHub 事件
  - eventstream 持久化链路自动覆盖该主题，read-model timeline 新增 `llm_stream_progress_event` 投影
  - 保持 streaming 仅用于进度观测，不改变最终 `LLMExecutionDecision` 业务决策边界
  - read-model timeline 对历史/兼容 `streamEvents` 执行头尾窗口保留、omitted summary 和单 delta preview 截断；通用 step timeline 不再向端侧输出完整 raw streamEvents
  - Web 时间线已展示 stream summary、omitted/truncated 标记、chunk/bytes 诊断和 raw stream omitted 摘要
- `CTX-13 Workspace LLM Usage Ledger` 已收口基础版：
  - 新增统一事件主题 `TopicRunLLMUsageRecorded`（`/run/llm_usage/recorded`）
  - LLM runtime 输出新增结构化 `llmUsage`（workspace/run/step/provider/model/operation/token/bytes/streaming/duration/status/failureClass/contextBudget）
  - orchestrator 在 step 状态变更后将 `llmUsage` 投影为 EventHub 事件并进入 eventstream 持久化
  - read-model 新增 `llmUsageTraces` + `llmUsageSummary`，支持 run/workspace 级追溯与汇总
  - `llmUsageSummary` 已补充 `providerNativeEntries/estimatedEntries` 聚合指标
  - `llmUsageSummary` 已补充 `providerModelBreakdown`（providerId/provider/model 维度 entry/token/cache 聚合）
  - `llmUsage` 保留 `usageSource` 语义字段，区分 token 来源为 provider 原生或本地估算
  - read-model 已对缺失 `usageSource` 做统一归一，保障投影稳定
  - fallback usage 估算已改为使用实际请求/响应体量，不再将 `contextBudget.actualTokens` 或 `reservedOutputTokens` 等预算字段误计为实际 usage
  - `cacheUsage` 已接入 provider 原生 cache token 字段；provider 未返回 cache usage 时按 cachePlan 估算 cacheable / non-cacheable input tokens
  - workspace snapshot 摘要区展示 workspace 累积 LLM 调用次数、输入 token、输出 token、总 token，以及 cache read/write token
  - 部署 e2e 脚本 `verify-llm-workspace-execution.sh` 已补充 `llmUsageTraces/llmUsageSummary` 语义一致性断言
  - 部署 e2e mock 现已同时覆盖 `usageSource=estimated`（provider 无 usage 返回）与 `usageSource=provider_native`（provider 返回 usage）两条链路并逐 run 断言
  - 部署 e2e 进一步覆盖 streaming 场景：`verify-stream-native`（SSE usage 返回）与 `verify-stream-estimated`（SSE 无 usage 返回）已纳入逐 run 断言
  - 部署 e2e 已新增 workspace 级 provider/model breakdown 断言
  - 已在本地新实例（`127.0.0.1:18081`）执行 `verify-llm-workspace-execution.sh` 验证通过
  - OpenAI/Anthropic/HTTP 非流式调用已接入 provider 原生 usage，runtime 优先写入 `estimated=false`
  - streaming 场景已接入 provider 原生 usage 优先链路（provider 返回 usage 时 `estimated=false`；未返回时回退估算）
- 已补充受影响测试：
  - `internal/modules/blocks/llm/biz`
  - `internal/pkg/workorchconfig`
  - `internal/modules/application/workorchd/biz/appservice`
  - `internal/modules/kernel/orchestrator/biz`

### 当前仍未完全收口

- provider 原生 token usage 已接入非流式/流式主链；剩余缺口是未接入可插拔真实 tokenizer，高精度估算仍依赖 provider 返回 usage 或保守 bytes-ratio fallback。
- context budget 元数据已完成基础投影、端侧细粒度展示、run/workspace 基础聚合统计和最近窗口趋势统计；跨 run 长期趋势视图和真实 tokenizer 精度仍待补齐。
- provider failover 已支持多级有序候选、基础可用性/operation eligibility 筛选和运行期内存健康/熔断窗口；权重策略与更细粒度健康评分仍待补齐。
- 当前 LLM streaming 已完成 eventhub/run progress 基础投影、read-model 记录上限策略和 Web 时间线基础诊断展示；独立 stream 详情页、筛选和跨 run 趋势视图仍待补齐。
- 当前已具备 workspace 维度 LLM usage ledger、usageSource 语义区分、cache usage 统计、以及非流式/流式 provider 原生 usage 优先链路；剩余项聚焦 provider 侧覆盖率与精度提升。

## 待办项

### CTX-01 Provider/Model Context Profile

状态：基础收口已完成。provider 配置、model capability table、REST/Web 配置读写和 runtime budget 计算已支持 `contextWindowTokens`、`maxInputTokens`、`reservedOutputTokens`、`safetyRatio`、`contextUtilizationRatio`、`bytesHardLimit`、`maxRequestBytes` 和 `estimatedTokensPerByte`。后续只保留更细粒度 model capability registry 和 tokenizer 绑定策略增强。

- 为 LLM provider/model 增加 context profile 配置。
- 普通配置入口：
  - `contextWindowTokens`
- manual 高级字段：
  - `maxInputTokens`
  - `reservedOutputTokens`
  - `safetyRatio`
  - `bytesHardLimit`
- 支持 provider 显式 `contextWindowTokens` 和 model capability table 覆盖值。
- 未配置时使用保守默认值。

### CTX-02 Token Budget Estimation

状态：基础收口已完成。runtime 当前使用 provider/model context profile 的 `estimatedTokensPerByte` 做保守输入 token 估算，provider 返回 usage 时优先使用 provider-native token usage；尚未接入可插拔 tokenizer，因此高精度估算仍是后续增强项。

- 引入 token 预算计算。
- 优先使用 provider/model tokenizer。
- 无 tokenizer 时使用保守估算。
- 保留 bytes hard limit 作为兜底保护。

### CTX-03 Priority-Based Context Compaction

- 建立通用上下文优先级：
  - P0 必保留：Goal、DoD、当前 step、当前失败、最新 decision/approval、runtimeCapabilities。
  - P1 尽量保留：recent governance、current soul summary、必要 memory recall。
  - P2 可摘要：历史 steps、历史 graph、workspace manifest。
  - P3 可引用：完整 soulMd、大文件内容、旧 evidence、完整历史结果。
- 压缩时按优先级逐层裁剪，而不是平均截断。

### CTX-04 Stage-Aware Context Budget

- 根据 `soulStage` / run stage 调整 context 组成：
  - planning：Goal/DoD、workspace manifest、模板/soul 摘要。
  - execution：当前文件、patch、当前任务上下文。
  - repair：failedStep、failedGraph、recent failure、governance resolution。
  - validation：DoD、evidenceRefs、测试输出摘要。
  - final/result：result、artifact、DoD assessment、残余风险。

### CTX-05 Adaptive Shrink on Provider Failure

- provider timeout 或 context-too-large 后自动降级重试：
  - 第一次：压缩 P2/P3。
  - 第二次：只保留 P0/P1。
  - 第三次：拆分任务或进入 decision 阻塞。
- 将 shrink level 写入 provider request metadata。

### CTX-06 Traceable Context References

- 被裁剪内容必须替换成结构化引用：

```json
{
  "summary": "...",
  "truncated": true,
  "sourceRefs": ["run-x/step-y", "artifact-z"]
}
```

- 确保 web/TUI/read model 可以展示 context budget 和被裁剪引用。

### CTX-07 Observability and Acceptance

- 在 run snapshot / observability 中展示：
  - provider/model context profile
  - input token budget
  - estimated input tokens
  - actual bytes
  - shrink level
  - truncation rules
- 增加 e2e：
  - 小 context 不裁剪。
  - 大 context 按模型预算裁剪。
  - provider timeout 后会缩小 context 并重试。
  - 裁剪后仍保留 Goal/DoD/governance 核心信息。

### CTX-08 Provider Failure Policy and Failover

状态：基础收口已完成（失败分类 + 多级有序 failover + 基础可用性/operation eligibility 筛选 + 运行期内存健康/熔断窗口 + 可追溯中立元数据），高级权重和更细粒度健康评分策略待补齐。

- 区分不同 provider 失败类型：
  - `provider_timeout`
  - `context_too_large`
  - `rate_limited`
  - `auth_or_config_error`
  - `provider_unavailable`
- 对 retryable 失败执行有限重试和 context shrink。
- 对不可自动恢复失败进入 governance decision，而不是直接终态失败。
- 支持按配置启用 provider/model failover。
- failover 后必须记录失败原因、上下文预算变化和尝试序号；provider/model 动态绑定只允许通过受控 request audit / raw ref 追溯，不进入 workspace/run 业务恢复状态。
- 多级 failover 必须保持 provider/model 动态绑定不进入业务恢复状态；运行期输出只暴露中立 attempt、failureClass、stage、error、budget 和成功序号。
- 运行期健康/熔断只作为 LLM block 内存态选择策略；服务重启或 provider 配置 revision 变化会重置健康窗口，不作为 workspace/run 恢复状态持久化。

### CTX-09 Multi-Client Governance Semantic Normalization

状态：基础收口已完成（统一归一能力 + appservice/tui 接入 + approval owner payload 追溯字段），入口覆盖与治理记录细化待补齐。

- TUI、HTTP、console、自动治理入口必须复用同一套 decision/approval 语义归一逻辑。
- 用户输入的治理意图应根据语义识别为：
  - 补充信息
  - 修改 Goal
  - 修改 DoD
  - 同意执行
  - 拒绝执行
  - 请求重新规划
  - 请求终止
- 归一逻辑必须保持业务中立，不引入具体应用场景关键字。
- 归一后的 resolution type、原始输入、推断置信度和来源入口需要写入治理记录，便于追溯。

### CTX-10 Context Budget Read Model Projection

- 将 `contextBudget` 投影到 run snapshot / read model。
- web 侧展示每次 LLM 请求的：
  - provider/model
  - budget 上限
  - 实际输入大小
  - token 估算值
  - shrink level
  - truncation rules
  - sourceRefs
- TUI 侧只展示简化状态，避免干扰主交互：
  - 是否裁剪
  - 当前 shrink level
  - 是否需要用户决策
- 保证 web/TUI 展示来自同一 read model，避免功能分叉。
- workorchd read-model 负责最近窗口与趋势聚合；Web/TUI 只展示 `llmContextBudgetSummary`，不得自行维护另一套统计状态。

### CTX-11 Context Split and Continuation

- 当 P0/P1 仍超过当前模型预算时，不应继续盲目裁剪核心信息。
- 支持将任务拆分为多个 continuation run 或 continuation step。
- continuation 必须保留同一个 Goal/DoD 闭环，并通过 evidence/sourceRefs 串联上下文。
- 拆分策略必须由通用运行时能力承载，不绑定软件开发等具体 workspaceTemplate。

### CTX-12 Streaming LLM Progress Event Projection

状态：基础收口已完成（EventHub 事件主题 + orchestrator 聚合发布 + eventstream 持久化 + read-model timeline 投影 + read-model bounded stream record + Web 时间线基础诊断展示），高级展示待补齐。

- 支持按 provider/model capability 启用 LLM streaming。
- streaming 只用于进度展示、运行期观测和故障排查，不直接驱动业务状态流转。
- 最终仍以完整响应解析出的 `LLMExecutionDecision` 作为唯一业务决策输入。
- provider stream delta 需要通过 eventhub 投影为 run progress event。
- stream progress event 建议字段：
  - `workspaceId`
  - `runId`
  - `stepId`
  - `providerId`
  - `model`
  - `chunkCount`
  - `receivedBytes`
  - `elapsedMs`
  - `status`
- TUI 默认只显示简化进度，例如 `LLM responding... received N chunks`。
- Web 已在运行时间线中展示 stream progress、omitted/truncated 和错误诊断摘要；更完整的独立详情页、筛选和跨 run 趋势视图仍待补齐。
- 需要避免将完整 delta 默认显示到 TUI 主结果区，防止干扰最终结果展示。
- stream 记录已具备 read-model 上限和摘要策略，避免历史/兼容 `streamEvents` 自身造成新的端侧 payload 膨胀；后续如新增独立 stream 详情页，需要继续复用该 bounded projection。
- e2e 覆盖：
  - provider streaming 开启后能收到 progress event。
  - streaming 完成后仍按完整 JSON 决策执行。
  - streaming 中断时记录已收到的 progress，并进入统一 provider failure policy。

### CTX-13 Workspace LLM Usage Ledger

状态：基础收口已完成并补齐原生 usage 优先能力与 cache usage 统计（runtime usage 输出 + EventHub 事件 + eventstream 持久化 + run/workspace read-model 投影 + provider native usage 优先），高精度能力待补齐。

- 建立独立的 LLM usage ledger，按 workspace/run/step 记录每次 LLM 调用。
- usage ledger 必须保持基础能力中立，不绑定具体 workspaceTemplate。
- 记录字段建议包括：
  - `workspaceId`
  - `runId`
  - `stepId`
  - `providerId`
  - `provider`
  - `model`
  - `operation`
  - `inputTokens`
  - `outputTokens`
  - `totalTokens`
  - `estimated`
  - `usageSource`
  - `requestBytes`
  - `responseBytes`
  - `streaming`
  - `contextBudget`
  - `cacheUsage`
  - `startedAt`
  - `completedAt`
  - `durationMs`
  - `status`
  - `failureClass`
- provider 返回 usage 时优先使用真实 usage。
- provider 未返回 usage 时，使用 provider/model context profile 的估算规则，并标记 `estimated=true`。
- provider 返回 cache usage 时优先使用 provider 原生字段。
- provider 未返回 cache usage 时，运行时只能基于 provider-neutral `cachePlan` 估算 cacheable / non-cacheable input tokens，并标记 `estimated=true`。
- `llmUsage` 不处理费用、账单或价格字段；费用治理不是 runtime usage ledger 的职责。
- 支持 workspace 维度聚合：
  - total input/output/total tokens
  - cache read/write/creation tokens
  - cacheable / non-cacheable input tokens
  - provider/model 分布
  - run/step 级明细
- web 侧展示 workspace 级 token/cache 汇总和明细追溯。
- TUI 侧只显示简化汇总，避免影响主交互。
- e2e 覆盖：
  - 非流式请求记录 usage。
  - 流式请求记录 usage。
  - provider 无 usage 返回时使用估算 usage。
  - 失败请求记录 request bytes、duration 和 failureClass。
  - workspace 聚合结果与 run/step 明细一致。

## 当前状态

- 状态：基础防护已收口，CTX-01/CTX-02 已完成本轮收口，剩余项待下一轮需求推进。
- 已验证：受影响范围测试通过：
  - `go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/blocks/llm/biz ./internal/modules/application/workorchd/biz/appservice ./internal/services/workorch/service/tui -count=1`
- 前置能力：固定 bytes 预算和 provider timeout retry 已完成基础收口。
- 主要遗留：provider failover 高级策略、多入口治理语义归一的剩余入口覆盖、真实 tokenizer/原生 token usage、provider 原生 usage 的跨模型覆盖率与精度提升、context-too-large 差异化治理策略。
