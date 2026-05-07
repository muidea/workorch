# Model-Aware LLM Context Budget Todo

对应设计：

- [model-aware-llm-context-budget-design.md](/home/rangh/aispace/workorch/docs/model-aware-llm-context-budget-design.md)

## 背景

当前 WorkOrch 已具备基础的 LLM provider request context budget 保护，会在最终提交给 LLM 前按固定字节上限压缩上下文。下一轮需要进一步升级为模型感知的 token budget，以适配不同 LLM provider/model 的 context window 差异。

## 目标

- 不使用单一全局 context 上限。
- 根据 provider/model 能力动态计算输入预算。
- 严控最终提交给 LLM 的 context 大小，避免 provider timeout 和 context overflow。
- 保持 WorkOrch 基础能力中立，不在基础层硬编码具体业务场景。
- 所有被裁剪内容必须保留摘要和 sourceRefs，确保可追溯。
- LLM 流式调用只作为运行期进度和可观测事件，不改变最终业务决策输入边界。
- 以 workspace 为单位记录 LLM usage/cost，支持 run/step 级追溯和成本治理。

## 本轮核对结果

### 已完成基础收口

- 固定 bytes 级别的 provider request context budget 已接入最终 LLM 请求构造链路。
- 大 context 会在提交 provider 前进行分层压缩，并写入 `contextBudget` 元数据。
- retryable provider timeout 不再直接导致 run 终态失败，会按 provider retry 规则自动重试。
- provider timeout retry 耗尽后进入 decision 阻塞，避免静默失败，并允许用户补充处理策略。
- 当前实现未引入具体业务场景硬编码，仍保持 WorkOrch 基础调度和编排能力中立。
- 已补充 provider context budget 与 provider timeout retry 的单元测试。

### 本轮新增收口

- `CTX-01 Provider/Model Context Profile` 已接入配置链路：
  - `workorchconfig.LLMProviderConfig`
  - `llm.ProviderConfig`
  - `RedactedProviderConfig`
- `CTX-02 Token Budget Estimation` 已接入预算链路：
  - 使用 `contextWindowTokens`、`reservedOutputTokens`、`maxInputTokens`、`safetyRatio`、`bytesHardLimit`
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
- 已补充受影响测试：
  - `internal/modules/blocks/llm/biz`
  - `internal/pkg/workorchconfig`
  - `internal/modules/application/workorchd/biz/appservice`

### 未完全收口，需要纳入下一轮

- 当前仍未接入真实 tokenizer 或 provider 原生 token usage。
- context budget 元数据已完成基础投影；细粒度展示和聚合统计仍待补齐。
- 尚未支持 context-too-large 与 provider timeout 的差异化治理策略。
- 尚未支持 provider/model failover，provider 持续不可用时仍依赖治理阻塞处理。
- HTTP/console 等非 TUI 客户端的 decision/approval 语义归一仍需统一，避免不同入口形成治理行为差异。
- 当前 LLM streaming 能力已有 provider 层接口，但默认未启用，且 stream delta 尚未通过 eventhub 投影为 run progress。
- 当前尚未以 workspace 为单位建立 LLM usage/cost ledger，无法对 provider/model 成本进行运行期追溯和汇总。

## 待办项

### CTX-01 Provider/Model Context Profile

- 为 LLM provider/model 增加 context profile 配置。
- 建议字段：
  - `contextWindowTokens`
  - `maxInputTokens`
  - `reservedOutputTokens`
  - `safetyRatio`
  - `bytesHardLimit`
- 支持 provider 默认值和 model 覆盖值。
- 未配置时使用保守默认值。

### CTX-02 Token Budget Estimation

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

- 区分不同 provider 失败类型：
  - `provider_timeout`
  - `context_too_large`
  - `rate_limited`
  - `auth_or_config_error`
  - `provider_unavailable`
- 对 retryable 失败执行有限重试和 context shrink。
- 对不可自动恢复失败进入 governance decision，而不是直接终态失败。
- 支持按配置启用 provider/model failover。
- failover 后必须记录原 provider/model、目标 provider/model、失败原因和上下文预算变化。

### CTX-09 Multi-Client Governance Semantic Normalization

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

### CTX-11 Context Split and Continuation

- 当 P0/P1 仍超过当前模型预算时，不应继续盲目裁剪核心信息。
- 支持将任务拆分为多个 continuation run 或 continuation step。
- continuation 必须保留同一个 Goal/DoD 闭环，并通过 evidence/sourceRefs 串联上下文。
- 拆分策略必须由通用运行时能力承载，不绑定软件开发等具体 workspaceTemplate。

### CTX-12 Streaming LLM Progress Event Projection

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
- web 可以提供更完整的 stream progress 和错误诊断视图。
- 需要避免将完整 delta 默认显示到 TUI 主结果区，防止干扰最终结果展示。
- stream 记录需要有上限和摘要策略，避免自身造成新的上下文膨胀。
- e2e 覆盖：
  - provider streaming 开启后能收到 progress event。
  - streaming 完成后仍按完整 JSON 决策执行。
  - streaming 中断时记录已收到的 progress，并进入统一 provider failure policy。

### CTX-13 Workspace LLM Usage and Cost Ledger

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
  - `estimatedCost`
  - `currency`
  - `requestBytes`
  - `responseBytes`
  - `streaming`
  - `contextBudget`
  - `startedAt`
  - `completedAt`
  - `durationMs`
  - `status`
  - `failureClass`
- provider 返回 usage 时优先使用真实 usage。
- provider 未返回 usage 时，使用 provider/model context profile 的估算规则，并标记 `estimated=true`。
- provider/model 价格信息放在配置或 profile 中，不硬编码在业务逻辑里。
- 支持 workspace 维度聚合：
  - total input/output/total tokens
  - estimated cost
  - provider/model 分布
  - run/step 级明细
  - failure cost and retry cost
- web 侧展示 workspace 级成本汇总和明细追溯。
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
  - `go test ./internal/modules/kernel/orchestrator/biz ./internal/modules/blocks/llm/biz ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorch/service/tui -count=1`
- 前置能力：固定 bytes 预算和 provider timeout retry 已完成基础收口。
- 主要遗留：provider failover、多入口治理语义归一、streaming progress event、workspace usage/cost ledger、真实 tokenizer/原生 token usage、context-too-large 差异化治理策略。
