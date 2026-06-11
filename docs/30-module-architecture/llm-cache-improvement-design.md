# LLM Cache Improvement Design

Status: draft

Type: module-architecture

Last Updated: 2026-06-11

Related:

- [00-core/runtime-request-cache-note.md](../00-core/runtime-request-cache-note.md)
- [30-module-architecture/llm-provider-assembly.md](llm-provider-assembly.md)
- [30-module-architecture/architecture-overview.md](architecture-overview.md)

## Purpose

本文是 workorch LLM 缓存能力的改进设计文档。目标不是在 core runtime 中写入 provider 私有字段，而是在保持
`runtime-request-cache-note.md` provider-neutral 约束的前提下，补齐 provider-facing request 的缓存能力映射、观测与验证。

当前代码已经具备以下基础：

- runtime 生成 `ProviderCachePlan`、`ProviderAdapterCacheHint`、`stablePrefixHash`、`workspaceDefinitionHash`、`cacheKey` 等 provider-neutral 审计字段。
- `buildProviderFacingPrompt` 已经把连续的 `C0` / `C1` / `C2` 渲染为第一段 provider-facing stable prefix。
- LLM usage 已经可以记录 provider 原生 cache token 字段；provider 未返回时按 cache plan 估算。

仍需补齐的是：

- 不同调用面和 cache capability 没有统一描述和验证。
- HTTP compatible 路径没有明确区分 OpenAI-compatible / Anthropic-compatible wire contract，也没有明确的 cache marker 下传策略。
- Anthropic SDK 路径已有 `cache_control`，但缺少统一可观测字段和策略化开关。
- OpenAI SDK 路径尚未明确如何使用 `prompt_cache_key`。
- 请求渲染后的 stable prefix 与 cache plan 缺少一致性校验。

## Current State Analysis

### Provider-Facing Stable Prefix 已基本就位

`internal/modules/blocks/llm/biz/provider_prompt_projection.go` 当前按 section group 处理 prompt：

- `C0` / `C1` / `C2` 被收集为 `StablePrefix`。
- `C3` / `C4` / `C5` 进入动态 message。
- stable prefix 保留 section heading，便于 request log 与 `ProviderCachePlan.sectionHashes` 对齐。

这符合 `runtime-request-cache-note.md` 的核心约束：provider-facing 第一段稳定 prefix 只包含 `C0` stable runtime prefix、
`C1` workspace definition、`C2` capability contract；`C3` current stage bundle 与 `C4` volatile execution tail 不进入稳定前缀。

因此，旧判断“user[0] 动态字段导致 system 段无法命中缓存”需要收窄。准确说法是：

- 动态 tail 从 `C3` 开始分叉，会限制可复用的 prefix 长度到 `C0`/`C1`/`C2`。
- 只要 provider 的缓存机制按请求前缀工作，前置 stable system prefix 仍然可以被复用。
- 是否命中必须以 provider native usage 与 request log 共同验证，不能只从 user message 中存在 `runId` / `stepId` 推断。

### Cache Hint 已生成，但调用面消费不完整

`internal/modules/blocks/llm/pkg/models/decision.go` 已定义：

```go
type ProviderAdapterCacheHint struct {
    Enabled          bool     `json:"enabled"`
    Mode             string   `json:"mode,omitempty"`
    ProviderMapping  string   `json:"providerMapping,omitempty"`
    CacheablePrefix  []string `json:"cacheablePrefix,omitempty"`
    VolatileSections []string `json:"volatileSections,omitempty"`
    CacheBreakers    []string `json:"cacheBreakers,omitempty"`
}
```

`internal/modules/blocks/llm/biz/provider_cache_plan.go` 会填充 `AdapterHint`。当前主要消费点仍偏审计：

- `runtime_adapter.go` 输出 context budget / cache plan。
- `runtime_adapter.go` 输出 `cacheUsage` 估算或 provider-native usage。
- `prompt_messages.go` 将部分 cache plan 元数据投影到 timeline / prompt context。

Provider-facing 消费现状：

- Anthropic SDK: 已在 stable prefix system block 上设置 `CacheControl = anthropic.NewCacheControlEphemeralParam()`。
- OpenAI SDK: 当前未明确设置 `prompt_cache_key`。
- HTTP compatible: `HTTPProvider` 已按 `apiSurface` 分发 OpenAI-compatible chat completions 与 Anthropic-compatible messages 两套 request shape；OpenAI-compatible 默认不发送私有 cache marker，Anthropic-compatible 仅在显式 capability 下发送 `cache_control`。

### Provider 路径现状

当前 `NewProviderFromConfig` 的选择逻辑是按调用协议/SDK surface 选择，而不是按真实后端厂商选择：

- `ProviderOpenAI` -> `OpenAIProvider`，使用官方 OpenAI SDK。
- `ProviderAnthropic` -> `AnthropicProvider`，使用官方 Anthropic SDK。
- 其他 provider -> `HTTPProvider`，按 OpenAI-compatible HTTP chat completions 协议发送请求。

这只是当前实现状态，不应固化为目标架构约束。目标设计中，`HTTPProvider` 应被理解为 HTTP transport implementation，
而不是 OpenAI-compatible 协议本身。它需要通过 provider config 明确选择 wire protocol surface：

- OpenAI-compatible HTTP surface: `/chat/completions` request shape，支持 `prompt_cache_key` 或 provider-declared compatible marker。
- Anthropic-compatible HTTP surface: Anthropic Messages request shape，支持 `cache_control` marker，前提是 endpoint 声明显式兼容并可验证。

也可以在后续实现中拆成更聚焦的 `OpenAICompatibleHTTPProvider` 与 `AnthropicCompatibleHTTPProvider`，但拆分目标是明确协议契约，
不是按后端品牌或代理名称拆分。

DeepSeek、自建服务、ai-proxy、Bluetron 等都只是可能位于 OpenAI-compatible 或 Anthropic-compatible 调用面之后的后端或代理，
不能作为 WorkOrch 内部 provider cache 策略的一等分类。

## Provider Cache Capability Matrix

缓存策略必须按调用协议和显式声明的 cache capability 决定，不能按后端品牌或代理名称推断。
WorkOrch 的 provider 分类应以“如何调用”为准：

- OpenAI SDK surface
- Anthropic SDK surface
- OpenAI-compatible HTTP surface
- Anthropic-compatible HTTP surface

真实后端可以是官方地址，也可以是 ai-proxy 这类临时代理。只要调用面仍然是 OpenAI SDK 或 Anthropic SDK，
runtime 就只能按对应 SDK/wire contract 处理；后端是否真正透传、改写或实现 cache 字段，只能通过 provider capability 声明和 usage 观测确认。
HTTP 路径同理，必须先确认 HTTP endpoint 暴露的是 OpenAI-compatible 还是 Anthropic-compatible contract，再决定 request shape 与 cache marker。

| Call surface | 当前实现 | Cache capability 来源 | 首选改造 |
| --- | --- | --- | --- |
| Anthropic SDK surface | `AnthropicProvider` | SDK contract + provider config declaration | 保留 stable prefix `cache_control`，补齐策略开关、usage 解析、marker 观测 |
| OpenAI SDK surface | `OpenAIProvider` | SDK contract + provider config declaration | 在 SDK 支持时设置稳定 `prompt_cache_key`，并验证 wire |
| OpenAI-compatible HTTP surface | 当前由 `HTTPProvider` 承载 | provider config declaration，默认 none | 默认不发送私有字段；显式声明后才允许 `prompt_cache_key` 或兼容 marker |
| Anthropic-compatible HTTP surface | 当前缺失，目标由 protocol-aware `HTTPProvider` 或独立 provider 承载 | provider config declaration + wire verification | 使用 Anthropic Messages shape；显式声明后才允许 stable prefix `cache_control` |

外部语义参考：

- DeepSeek 若通过 OpenAI-compatible surface 调用，其 Context Caching 默认启用；WorkOrch 不需要也不应该仅因后端是 DeepSeek 就发送额外私有字段。
- DeepSeek 若通过 Anthropic-compatible surface 调用，仍不能等同于 Anthropic Prompt Caching 能力。DeepSeek Anthropic API compatibility
  文档声明 message / tool content 上的 `cache_control` 会被忽略；因此 DeepSeek Anthropic-compatible endpoint 应按 `auto_prefix`
  或 `none` 处理，不能声明为 `anthropic_cache_control`，除非后续官方文档明确改变该能力。
- Anthropic Prompt Caching 通过 `cache_control` 显式标记可缓存内容。
- OpenAI Prompt Caching 可使用 `prompt_cache_key` 影响请求路由并提高共享前缀场景下的命中率；该能力仍必须按 OpenAI SDK/wire contract 验证。
- ai-proxy 只能视为透明或半透明代理；它不能改变 WorkOrch 的调用面分类，只能通过配置声明“是否透传 cache 字段、是否返回 provider-native usage”。

## Goals and Non-Goals

### Goals

1. 建立 provider cache capability matrix，明确每个调用面支持什么 cache 映射。
2. 保持 `C0` / `C1` / `C2` provider-facing stable prefix 字节级稳定。
3. 在支持显式 cache marker 的调用面上，把 `ProviderAdapterCacheHint` 转换为 provider-facing wire 字段。
4. 增加可观测性，记录 cache marker 是否发送、stable prefix hash 是否漂移、provider native cache usage 是否命中。
5. 用测试和 request log 验证真实 wire，而不是只验证 runtime audit map。

### Non-Goals

1. 不在 core runtime 中写 provider 私有字段。
2. 不默认向所有 HTTP compatible endpoint 写入 `cache_control` 或 `prompt_cache_key`。
3. 不把 `C3` 当前 stage bundle 移入 stable prefix。
4. 不默认做 cache 预热；预热会增加成本和延迟，只能作为 provider-specific future optimization。
5. 不修改 ai-proxy 配置；代理是否透传 cache 字段由独立配置和验证决定。

## Design Overview

### 分层责任

```text
[provider-neutral core]
    ProviderCachePlan
    ProviderAdapterCacheHint
    PromptContext sections C0-C5
           |
           v
[provider-facing call surface]
    OpenAIProvider       -> OpenAI SDK contract, optional prompt_cache_key
    AnthropicProvider    -> Anthropic SDK contract, cache_control on stable prefix block
    HTTPProvider         -> HTTP transport; protocol-aware OpenAI-compatible or Anthropic-compatible contract
           |
           v
[observability / read model]
    cacheMarkersSent
    provider native cacheUsage
    stable prefix drift
    cache efficiency summary
```

### 改造矩阵

| Phase | 范围 | 目标 |
| --- | --- | --- |
| P0 | 修正文档、补齐 capability matrix、增加 wire 级测试 | 避免错误假设进入实现 |
| P1 | Provider-facing marker 显式化与观测 | 明确哪些请求发送了哪些 cache marker |
| P2 | Stable prefix render invariant | 发现 request rendering 与 cache plan 漂移 |
| P3 | Cache efficiency read model / TUI 展示 | 可持续观测命中率和 provider 差异 |
| P4 | 可选 adapter 抽象、预热、跨 run cache key | 仅在数据证明必要时演进 |

## Detailed Design

### P0 — 文档与契约修正

#### P0-1. 固化 provider cache capability

新增或扩展 provider config contract，表达 provider cache 能力。该能力依附于调用面，但不由后端品牌或代理名称隐式推断：

```go
type ProviderCacheCapability struct {
    Surface string // "openai_sdk", "anthropic_sdk", "openai_compatible_http", "anthropic_compatible_http"
    Mode string // "none", "auto_prefix", "prompt_cache_key", "anthropic_cache_control", "compatible_declared"
    PromptCacheKey bool
    MessageCacheControl bool
    UsageFields []string
}
```

落点可选：

- `ProviderConfig` 中新增 `CacheCapability`。
- `ProviderConfig` 中新增或复用明确的 `Surface` / `Protocol` / `APISurface` 字段，禁止仅靠 provider ID 或 baseURL 推断 HTTP wire shape。
- 或在 config loader 中基于 `Provider` / provider template 生成 runtime-only capability。

默认策略按 call surface 派生：

- Anthropic SDK surface: `anthropic_cache_control`
- OpenAI SDK surface: `prompt_cache_key`，前提是当前 SDK/wire 支持
- OpenAI-compatible HTTP surface: `none`
- Anthropic-compatible HTTP surface: `none`，显式声明后才启用 `anthropic_cache_control`

后端特例只能通过配置模板或用户配置覆盖：

- DeepSeek over OpenAI-compatible HTTP: 可声明 `auto_prefix`，不发送私有字段，只观测 native cache usage。
- DeepSeek over Anthropic-compatible HTTP: 可声明 `auto_prefix` 或 `none`；不发送或不依赖 `cache_control`，因为当前 DeepSeek Anthropic API
  compatibility 明确忽略该字段。
- ai-proxy over OpenAI-compatible HTTP: 默认 `none`；若确认代理透传对应字段，再声明 `compatible_declared`。
- ai-proxy over Anthropic-compatible HTTP: 默认 `none`；若确认代理完全兼容 Anthropic Messages wire contract 并透传 `cache_control`，再声明 `anthropic_cache_control`。

#### P0-2. 修正测试目标

测试不应只断言 audit 中存在 `AdapterHint`，还要断言 provider-facing request wire：

- Anthropic SDK mock server：system block 包含 `cache_control`。
- HTTPProvider OpenAI-compatible default：默认不包含未知私有 cache 字段。
- HTTPProvider Anthropic-compatible default：默认不包含 `cache_control`。
- HTTPProvider declared cache support：显式声明后才按选定 protocol surface 包含对应字段。
- OpenAI SDK：若 SDK 支持 `prompt_cache_key`，断言请求体包含稳定 key；若当前 SDK 不支持，记录技术债而不是伪造字段。

### P1 — Provider-Facing Marker 显式化

#### P1-1. Anthropic SDK 路径

当前 `AnthropicProvider.messageParams` 已在 stable prefix block 上设置 `CacheControl`。需补齐：

- 只有 `request.ContextBudget.CachePlan.AdapterHint.Enabled` 为 true 时启用。
- 在 provider call output 中记录 `cacheMarkersSent.anthropic.cacheControl=true`。
- usage 解析中继续保留 `cache_creation_input_tokens`、`cache_read_input_tokens` 等字段。

#### P1-2. OpenAI SDK 路径

根据 SDK 与当前 call surface 能力设置 `prompt_cache_key`。当前 `OpenAIProvider` 使用 Chat Completions surface；
是否能在当前 SDK 类型化参数中直接承载 `prompt_cache_key` 必须先验证，不能假定可用：

- key 来源优先使用 `ProviderCachePlan.CacheKey`。
- key 不应包含 runId / stepId / provider baseURL。
- 如果当前 Chat Completions SDK 参数没有类型化字段，先通过 SDK extra fields 或低层 request option 验证。
- 如果该能力只在 OpenAI Responses surface 上稳定暴露，则不要在当前 Chat Completions 路径伪造字段；应先记录
  `cacheMarkersSent.openai.promptCacheKeyUnsupported=true`，再另行评估 OpenAIProvider 是否需要新增 Responses surface。

#### P1-3. HTTP Compatible 路径

`HTTPProvider` 的目标职责是 HTTP transport，不是单一 OpenAI-compatible contract。它必须先根据 provider config 决定 protocol surface，
再按对应 wire shape 构造请求。当前实现已覆盖 OpenAI-compatible chat completions 与 Anthropic-compatible HTTP messages 两条路径，后续只需继续收口未支持的更细能力。

HTTP protocol surface 的选择必须是显式配置或 template 派生后的确定值：

- 未声明时，为了保持当前行为，可暂按 OpenAI-compatible HTTP 处理，但必须在 configuration / provider capability 中暴露为已解析结果。
- 声明为 Anthropic-compatible HTTP 时，不允许走 OpenAI chat completions builder。
- 声明值与 request builder 不匹配时，应在 provider 初始化或请求发送前失败，避免把无效请求提交给 LLM。

OpenAI-compatible HTTP shape 默认不发送 provider 私有 cache 字段。只有 provider cache capability 显式声明支持时，才允许扩展：

```go
type chatCompletionRequest struct {
    Model string `json:"model"`
    Messages []chatCompletionMessage `json:"messages"`
    PromptCacheKey string `json:"prompt_cache_key,omitempty"`
    // ...
}
```

OpenAI-compatible HTTP 默认只考虑 `prompt_cache_key` 或该 endpoint 明确声明的兼容扩展；不应把 Anthropic `cache_control`
作为通用 OpenAI-compatible 字段发送。

Anthropic-compatible HTTP shape 不复用 OpenAI chat completions struct，必须使用 Anthropic Messages wire contract：

```go
type anthropicMessagesRequest struct {
    Model string `json:"model"`
    System []anthropicContentBlock `json:"system,omitempty"`
    Messages []anthropicMessage `json:"messages"`
    // ...
}
```

`cache_control` 只能写在 Anthropic-compatible content block 上，并且必须由 capability 声明开启。

DeepSeek 这类后端如果通过 OpenAI-compatible HTTP surface 调用，可以由配置声明 `auto_prefix`：

- 不写 `prompt_cache_key`。
- 不写 `cache_control`。
- 依赖稳定 prefix 和 provider native usage 验证命中。

DeepSeek 如果通过 Anthropic-compatible HTTP surface 调用，也应继续依赖 DeepSeek Context Caching 的自动前缀缓存语义：

- 可以使用 Anthropic Messages request shape。
- 不依赖 `cache_control` 命中缓存；即使字段存在，当前 DeepSeek Anthropic API compatibility 会忽略。
- 命中状态仍以 provider usage 中的 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` 或等价字段为准。

### P2 — Stable Prefix Render Invariant

当前 `ProviderCachePlan.StablePrefixHash` 只代表 `C0` group hash；provider-facing stable prefix 实际包含 `C0` / `C1` / `C2`。

因此不能直接用 `sha256(renderedStablePrefixBytes) == plan.StablePrefixHash` 作为断言。需要新增或计算一个明确的 rendered stable prefix hash：

```go
type ProviderCachePlan struct {
    // existing fields...
    RenderedStablePrefixHash string `json:"renderedStablePrefixHash,omitempty"`
}
```

或在 provider call output 中记录：

```json
{
  "cacheMarkersSent": {
    "renderedStablePrefixHash": "sha256:...",
    "planCacheKey": "sha256:...",
    "cacheableSections": ["C0.system", "C1.goal-contract-original"],
    "markerViolation": ""
  }
}
```

Invariant 规则：

- `RenderedStablePrefixHash` 必须只由 `AdapterHint.CacheablePrefix` 对应 section 的 rendered bytes 计算。
- 该 hash 在同一 workspace definition / capability contract 未变化时应保持稳定。
- 若 drift 发生，记录 `stable_prefix_render_drift` contract violation。
- drift 不应自动重试；应进入可观测事件或 runtime rejected result。

### P3 — Cache Efficiency Observability

#### P3-1. providerCallOutput 增加 cacheMarkersSent

`runtime_adapter.go` 的 provider call output 增加：

```go
"cacheMarkersSent": map[string]any{
    "providerMode":              capability.Mode,
    "promptCacheKeySent":        bool,
    "cacheControlSent":          bool,
    "autoPrefixCacheExpected":   bool,
    "renderedStablePrefixBytes": len(stablePrefix),
    "renderedStablePrefixHash":  renderedHash,
    "planCacheKey":              plan.CacheKey,
    "cacheableSections":         plan.AdapterHint.CacheablePrefix,
}
```

这些字段属于 observability，不参与业务状态裁决。

#### P3-2. 命中率统计投影

优先扩展现有 read model 投影，而不是默认新增并行模型。当前已有：

- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/llm_usage_cache_metrics.go`
- `internal/modules/application/workorchd/biz/appservice/readmodelprojection/llm_usage_cache_summary.go`

若现有文件承载后变得过大，再新增 `llm_cache_efficiency.go` 这类 focused projection 文件。投影职责：

- 按 provider / model 聚合。
- 输入来自 `LLMUsageTrace.CacheUsage` 与 `cacheMarkersSent`。
- 输出字段：
  - `providerID`
  - `model`
  - `providerMode`
  - `cacheablePromptCount`
  - `actualCacheHitCount`
  - `cacheCreationCount`
  - `cacheReadTokens`
  - `cacheWriteTokens`
  - `cacheHitRate`
  - `usageSource`

#### P3-3. TUI / Web 展示落点

TUI 落点应为：

- `internal/services/workorch/service/tui`

workorchd 只负责 read model / projection，不新增 `internal/modules/application/workorchd/biz/appservice/tui`。

展示原则：

- TUI 只消费 read model 或 backend event，不直接解析 provider raw request。
- Web 与 TUI 使用同一 cache efficiency projection。
- UI 只展示状态，不承担 cache 策略判断。

### P4 — 可选架构演进

#### P4-1. Provider Adapter 抽象

当前已有 `OpenAIProvider`、`AnthropicProvider`、`HTTPProvider` 三条调用面实现路径。是否新增 `provider_adapters/` 包，需要基于重复度决定。

只有同时满足以下条件才拆：

- 多个 provider 共享大量 wire building 逻辑。
- cache marker overlay、usage parsing、request verification 已出现明显重复。
- 新包不是只包一层 wrapper。

否则优先在现有 provider 文件中补 focused helper，避免产生额外 wrapper。

如果 `HTTPProvider` 同时承载 OpenAI-compatible 与 Anthropic-compatible 两套 wire contract，至少需要在文件内部形成清晰边界：

- protocol detection / config validation。
- OpenAI-compatible request builder。
- Anthropic-compatible request builder。
- 各自独立的 cache marker overlay 与 usage parsing。

当上述逻辑在同一文件内开始互相干扰时，应拆成 focused provider 实现或子文件，而不是增加只转发调用的 wrapper。

#### P4-2. Cache 预热

预热不是默认行为。只有在数据证明 provider 明确支持且收益稳定时才考虑：

- Anthropic SDK surface: 首次带 cache_control 的真实请求已经会创建 cache，额外预热可能只增加成本。
- OpenAI-compatible HTTP surface with auto-prefix backend: 通常无预热接口，应依赖真实请求前缀复用。
- Anthropic-compatible HTTP surface: 与 Anthropic SDK 一样，优先依赖真实请求的 `cache_control`；是否预热由 endpoint capability 证明。
- OpenAI SDK surface: prompt cache 行为与 routing 相关，预热收益需要实测。

若后续实现，必须满足：

- 配置显式开启。
- 有独立超时和预算。
- 失败不影响主请求。
- usage 中明确标记 warmup cost。

#### P4-3. 跨 run cache key

可选新增 `crossRunCacheKey`，由 workspace definition / goal / DoD / capability contract 派生。

约束：

- 不包含 runId / stepId。
- definition revision 改变时必须变更。
- 只用于支持 prompt cache key 的 provider。

## File-by-File Change List

| 文件 | Phase | 改动 |
| --- | --- | --- |
| `internal/modules/blocks/llm/pkg/models/provider_config.go` | P0 | 增加或承载 provider cache capability |
| `internal/modules/blocks/llm/biz/provider_config.go` | P0 | provider template 到 protocol surface / cache capability 的派生 |
| `internal/modules/blocks/llm/biz/sdk_provider.go` | P1 | Anthropic cache_control 策略化；OpenAI prompt_cache_key 能力验证 |
| `internal/modules/blocks/llm/biz/http_provider.go` | P1 | 按 protocol surface 构造 OpenAI-compatible / Anthropic-compatible HTTP request；cache marker capability-gated 下传 |
| `internal/modules/blocks/llm/biz/provider_prompt_projection.go` | P2 | 暴露 rendered stable prefix hash 输入或 helper |
| `internal/modules/blocks/llm/biz/runtime_adapter.go` | P2/P3 | stable prefix render invariant + cacheMarkersSent |
| `internal/modules/application/workorchd/biz/appservice/readmodelprojection/llm_usage_cache_metrics.go` / `llm_usage_cache_summary.go` | P3 | 扩展现有 cache usage projection，纳入 cacheMarkersSent 与 provider/model breakdown |
| `internal/modules/application/workorchd/biz/appservice/readmodelprojection/llm_cache_efficiency.go` | P3 | 仅在现有投影过大时新增 focused cache efficiency projection |
| `internal/services/workorch/service/tui` | P3 | 消费 read model 展示 cache summary |
| `docs/00-core/runtime-request-cache-note.md` | P0 | 如需，仅引用本文档；不展开 provider 私有字段 |

## Verification Plan

### Unit Tests

- `sdk_provider_test.go`
  - Anthropic stable prefix block 在 cache hint enabled 时包含 `cache_control`。
  - cache hint disabled 时不发送 `cache_control`。
  - OpenAI prompt cache key 若启用，wire 中使用 stable key。
- `http_provider_test.go`
  - unknown compatible provider 默认不发送 `prompt_cache_key` / `cache_control`。
  - declared compatible provider 才发送允许的 cache marker。
  - OpenAI-compatible HTTP auto-prefix mode 不发送私有 cache marker。
  - DeepSeek OpenAI-compatible auto-prefix mode 不发送 `prompt_cache_key` / `cache_control`，并解析 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`。
  - DeepSeek Anthropic-compatible auto-prefix mode 不声明 `anthropic_cache_control`，不依赖 `cache_control` 命中缓存。
  - Anthropic-compatible HTTP declared cache mode 使用 Anthropic Messages shape，并只在 stable prefix block 发送 `cache_control`。
- `provider_prompt_projection_test.go`
  - `C0` / `C1` / `C2` 位于 stable prefix。
  - `C3` / `C4` 不进入 stable prefix。
  - rendered stable prefix hash 在动态 run/step 变化时保持稳定。
- `runtime_adapter_test.go`
  - `cacheMarkersSent` 输出完整。
  - stable prefix render drift 可被检测。

### Integration Tests

- mock provider 记录 raw request，校验 provider-facing cache marker。
- 多轮同 workspace request，断言 `renderedStablePrefixHash` 稳定。
- provider native usage 返回 cache tokens 时，read model 汇总命中率。

### Acceptance Criteria

| 指标 | 要求 |
| --- | --- |
| stable prefix rendering | 同一 workspace definition 下动态 run/step 变化不改变 rendered stable prefix hash |
| compatible default safety | 未声明能力的 HTTP compatible endpoint 不收到未知私有 cache 字段 |
| Anthropic explicit cache | cache hint enabled 时发送 cache_control，并记录 marker |
| HTTP protocol correctness | HTTPProvider 不把 Anthropic-compatible endpoint 按 OpenAI chat completions shape 调用，也不把 OpenAI-compatible endpoint 按 Anthropic Messages shape 调用 |
| observability | run snapshot / usage trace 可看到 cacheMarkersSent 与 cacheUsage |
| provider-native usage | provider 返回 cache token 时 read model 正确聚合 |

## Risks

### R1. 向不支持的 compatible provider 发送私有字段

缓解：默认不发送；必须由 provider cache capability 显式开启。

### R2. 错误 hash invariant 阻断正常请求

缓解：区分 `StablePrefixHash(C0)`、`CacheKey(C0+C1+C2)` 和 `RenderedStablePrefixHash(rendered bytes)`，不要混用。

### R3. 预热增加成本

缓解：不作为默认计划；仅在 P4 中按调用面和显式 capability 开启。

### R4. Adapter 抽象过早拆分

缓解：先用 provider 文件内 focused helper；只有重复度和边界足够明确时再拆包。

## Open Questions

1. 当前 OpenAI SDK 版本是否支持类型化 `prompt_cache_key`，还是需要低层 extra field?
2. 当前 provider config 是否已有明确字段表达 HTTP protocol surface，还是需要新增 `protocol` / `apiSurface`?
3. ai-proxy 是否透传 provider 私有 cache 字段，还是会剥离未知字段?
4. provider cache capability 应存为用户配置，还是由 provider template 按 call surface 自动派生?
5. cache efficiency summary 是进入 workspace snapshot detail，还是独立 observability endpoint?

## References

- DeepSeek Context Caching: https://api-docs.deepseek.com/guides/kv_cache
- Anthropic Prompt Caching: https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching
- OpenAI Prompt Caching: https://developers.openai.com/api/docs/guides/prompt-caching
- WorkOrch Runtime Request Cache Note: `docs/00-core/runtime-request-cache-note.md`
