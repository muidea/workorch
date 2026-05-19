# WorkOrch LLM Auto Context Profile Design

## 背景

当前 WorkOrch 已支持 `contextWindowTokens`、`maxInputTokens`、`reservedOutputTokens`、`bytesHardLimit` 等 provider context profile，并在 provider request 发送前执行分层压缩与超预算 fail-fast。

但现有默认值仍偏保守：

```toml
contextWindowTokens = 32768
maxInputTokens = 28672
reservedOutputTokens = 4096
bytesHardLimit = 98304
```

这会让支持 128K、256K、1M 上下文的模型实际只能使用约 24K-28K 输入上下文。对于长流程 workspace、复杂 repair、代码库或文档审计类 agent 任务，这会导致必要中间信息被过早压缩，增加 LLM 决策偏差和反复修复风险。

本设计定义 **自动模型能力解析 + 阶段感知预算**，作为 `docs/model-aware-llm-context-budget-design.md` 与 `docs/workorch-runtime-llm-context-integrity-design.md` 的补充。

## 目标

- 根据 provider/model 能力自动解析上下文窗口，而不是统一固定 32K。
- 支持用户显式配置覆盖，显式配置永远优先。
- 按 runtime stage 自动调整可使用上下文比例，避免所有阶段都吃满模型窗口。
- `bytesHardLimit` 随 token budget 自动扩展，但受全局安全上限约束，避免单 JSON 无限制膨胀。
- 保持 WorkOrch core runtime 中立：runtime 只消费抽象能力字段，不写入具体业务、语言、框架或 workspaceTemplate 规则。
- 保留 LCI 上下文完整性约束：P0-P2 优先、P3-P5 摘要+引用、P6-P7 ref-only。

## 非目标

- 不把所有历史原文直接塞进 1M context。
- 不绕过 redaction、rejected raw 隔离和 contextBudget fail-fast。
- 不要求 provider 一定支持模型 metadata API；动态发现失败时可回退到配置化能力表或保守默认。
- 不在 kernel/orchestrator 中硬编码 provider/model 名称。

## 核心方案

### 1. Model Capability Resolver

LLM provider 配置解析阶段生成标准化 `ProviderContextProfile`。

能力解析优先级：

1. 用户显式配置的 `contextWindowTokens` / `maxInputTokens` / `bytesHardLimit` 等字段。
2. `contextProfileMode = "auto"` 时，根据配置化 provider/model 能力表解析。
3. provider 动态 metadata 能力发现，后续扩展。
4. 无法识别时使用保守默认。

输出的抽象字段：

- `profileMode`
- `capabilitySource`
- `contextWindowTokens`
- `maxInputTokens`
- `reservedOutputTokens`
- `safetyRatio`
- `contextUtilizationRatio`
- `bytesHardLimit`
- `maxRequestBytes`
- `estimatedTokensPerByte`

### 2. 配置化模型能力表

具体模型能力不得硬编码在 kernel/runtime，也不应硬编码在 LLM provider 解析逻辑中。WorkOrch 只提供通用匹配能力，具体模型信息进入配置文件：

```toml
[llm.modelCapabilities.gpt-codex-compatible-1m]
provider = "compatible"
modelPattern = "*codex*"
contextWindowTokens = 1000000

[llm.modelCapabilities.claude-4-7-1m]
provider = "anthropic"
modelPattern = "*claude*4?7*"
contextWindowTokens = 1000000
```

匹配规则：

- `provider` 可选；为空表示可匹配任意 provider。
- `modelPattern` 支持 `*` / `?` 通配符；匹配前统一做小写、空格转 `-`、`_` 转 `-`。
- provider 显式配置 `contextWindowTokens` 时优先级高于能力表。
- 能力表命中后只投影抽象字段 `contextWindowTokens` 与 `capabilitySource=model_registry`。

推荐配置基于用户提供的当前模型能力信息：

| 匹配 | contextWindowTokens | 默认定位 |
| --- | ---: | --- |
| DeepSeek V4 / v4-pro / v4-flash | 1000000 | 长上下文代码工程与复杂推理 |
| GPT-5.5 Thinking | 262144 | 深度推理，较大但不按 1M 处理 |
| GPT-5.5 Instant | 131072 | 快速交互 |
| GPT-5.4 / GPT-5.3 Codex | 1000000 | 长流程 agent / 代码库任务 |
| Claude 4.6 / 4.7 Opus/Sonnet | 1000000 | 企业文档和多步 agent |
| Claude 4.5 | 200000 | 常规长上下文任务 |

无法命中配置化能力表时使用保守默认：

```text
contextWindowTokens = 32768
reservedOutputTokens = 4096
maxInputTokens = contextWindowTokens - reservedOutputTokens
bytesHardLimit = 96 KiB
capabilitySource = conservative_default
```

### 3. 自动预算计算

自动模式不直接吃满最大窗口。

基础公式：

```text
reservedOutputTokens = explicitReservedOutputTokens
  or min(max(contextWindowTokens / 16, 4096), 65536)

inputWindow = contextWindowTokens - reservedOutputTokens
baseInputTokens = floor(inputWindow * contextUtilizationRatio)
maxInputTokens = min(explicitMaxInputTokens or baseInputTokens, inputWindow)
```

默认 `contextUtilizationRatio = 0.75`。

`bytesHardLimit` 自动反推：

```text
estimatedBytes = maxInputTokens / estimatedTokensPerByte
bytesHardLimit = min(estimatedBytes, maxRequestBytes)
```

默认 `maxRequestBytes = 4 MiB`，避免 1M 模型导致单 request JSON 无限制膨胀。

### 4. Stage-aware Budget

provider request budget 在执行阶段再应用 stage ratio，只影响本次 request 的 `maxInputTokens` / `bytesHardLimit`，不修改 provider 基础 profile。

建议比例：

| Stage | Ratio | 说明 |
| --- | ---: | --- |
| planning | 1.00 | 允许完整目标、模板、manifest、约束 |
| execution | 0.80 | 保留当前任务、必要 evidence、目标快照 |
| repair | 1.00 | repair 需要 failure、coverage、progress、target snapshot |
| validation | 0.55 | 聚焦 Goal/DoD、证据和验证输出 |
| artifact | 0.50 | 聚焦结果、artifact、残余风险 |
| decision / approval | 0.60 | 聚焦治理上下文和候选影响 |
| provider retry timeout | 最多 0.50 | 自动降档缩容 |

runtime stage 来源保持中立：

- `runtimeStage`
- `currentSoulStage`
- `runtimeRepairFollowUp`
- `providerRetryFollowUp`
- `completionFollowUp`
- `graphFollowUp`

### 5. Context Budget 审计

`contextBudget` 增加或保留以下审计字段：

- `profile.profileMode`
- `profile.capabilitySource`
- `profile.contextUtilizationRatio`
- `profile.maxRequestBytes`
- `stageBudgetRatio`
- `stageBudgetSource`
- `maxBytes`
- `maxTokens`
- `actualBytes`
- `estimatedInputTokens`
- `shrinkLevel`
- `overBudget`
- `p0Overflow`

如果 `profileMode=auto` 但模型未知，应投影：

```json
{
  "capabilitySource": "conservative_default"
}
```

未知模型使用保守默认是可恢复的正常配置状态，不作为 `integrityWarnings` 处理；`integrityWarnings` 继续只表达可能影响当前 LLM 决策正确性的上下文完整性问题。

## 配置形态

自动模式示例：

```toml
[llm.modelCapabilities.gpt-codex-compatible-1m]
provider = "compatible"
modelPattern = "*codex*"
contextWindowTokens = 1000000

[llm.providers.local]
provider = "compatible"
model = "gpt-5.3-codex"
contextProfileMode = "auto"
contextUtilizationRatio = 0.75
maxRequestBytes = 4194304
temperature = 0.0
maxOutputTokens = 65536
reasoningEffort = "high"
thinkingBudgetTokens = 0
```

显式覆盖示例：

```toml
[llm.providers.strict]
provider = "compatible"
model = "custom-small"
contextProfileMode = "manual"
contextWindowTokens = 65536
maxInputTokens = 32768
reservedOutputTokens = 8192
bytesHardLimit = 262144
```

显式字段优先；如果配置了 `maxInputTokens` 或 `bytesHardLimit`，自动解析不得覆盖这些字段。

推理强度配置：

- `temperature`: 采样温度；未配置时保持 WorkOrch 默认 `0`。
- `maxOutputTokens`: 输出 token 上限；OpenAI/OpenAI-compatible 映射为 `max_completion_tokens`，Anthropic 映射为 `max_tokens`。
- `reasoningEffort`: 抽象推理强度；OpenAI/OpenAI-compatible 映射为 `reasoning_effort`。
- `thinkingBudgetTokens`: 显式思考预算；Anthropic 映射为 `thinking.budget_tokens`，OpenAI-compatible 在配置后透传为 `thinking` 对象。

这些字段只属于 LLM provider adapter。kernel/runtime 只使用 provider 配置结果，不根据具体模型名做分支。

## 待办跟踪

| ID | 优先级 | 状态 | 内容 | 验收 |
| --- | --- | --- | --- | --- |
| ACP-01 | P0 | Done | 输出自动上下文 profile 设计文档 | 本文档已落地并链接到 model-aware/context integrity 文档 |
| ACP-02 | P0 | Done | 扩展配置字段 `contextProfileMode`、`contextUtilizationRatio`、`maxRequestBytes` | TOML/API/Redacted profile 可读写，CLI client DTO 已同步 |
| ACP-03 | P0 | Done | 实现模型能力解析器 | 已知 128K/256K/1M 模型自动解析；未知模型回退保守默认 |
| ACP-04 | P0 | Done | 自动计算 `maxInputTokens` 与 `bytesHardLimit` | 1M 模型默认不再被 96KiB 限死，同时受 `maxRequestBytes` 限制 |
| ACP-05 | P1 | Done | stage-aware request budget | 不同 stage 投影 `stageBudgetRatio`，validation/decision 自动缩小 |
| ACP-06 | P1 | Done | contextBudget 审计投影 | 输出 profile mode、capability source、context utilization、maxRequestBytes、stage ratio/source |
| ACP-07 | P1 | Done | 测试与文档同步 | 单测覆盖自动、手动覆盖、显式预算覆盖、stage ratio |
| ACP-08 | P0 | Done | 模型能力表配置化 | 具体模型上下文能力进入 `llm.modelCapabilities`，代码只保留通用匹配 |
| ACP-09 | P0 | Done | 推理强度配置化 | provider 支持 `temperature`、`maxOutputTokens`、`reasoningEffort`、`thinkingBudgetTokens` |

## 验收标准

- `gpt-5.3-codex` / `gpt-5.4-codex` 自动解析为 1M context。
- `gpt-5.5-thinking` 自动解析为 256K context。
- `gpt-5.5-instant` 自动解析为 128K context。
- `deepseek-v4-pro` 自动解析为 1M context。
- `claude-opus-4.7` / `claude-sonnet-4.6` 自动解析为 1M context。
- `claude-4.5` 自动解析为 200K context。
- 未知模型保持保守默认，并记录 `capabilitySource=conservative_default`。
- 显式 `maxInputTokens` / `bytesHardLimit` 不被自动模式覆盖。
- validation stage 使用更小 request budget，但 P0-P2 完整性约束仍有效。
- 所有修改不进入 workspace 输出物，不绑定具体业务 goal/DoD。

## 设计结论

WorkOrch 应将上下文大小从固定配置升级为配置驱动的自动能力解析。LLM block 可以基于配置理解 provider/model 能力并映射推理强度；kernel runtime 只消费抽象 profile、inference profile 和 stage ratio。这样既能利用 1M 模型的长上下文能力，也能继续保持 LCI 的上下文治理和 WorkOrch 核心框架中立。
