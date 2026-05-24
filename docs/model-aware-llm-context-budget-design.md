# Model-Aware LLM Context Budget Design

## 背景

WorkOrch 已有 LLM provider request 上下文保护、provider timeout 重试和模型感知预算能力。当前设计目标是把预算规则固定为可解释的运行时合同：普通配置只声明模型能力，运行时自动推导 token/byte 预算，并按阶段压缩最终提交给 LLM 的上下文。

本设计用于补齐下一轮能力边界，覆盖：

- provider / model 感知的 context budget。
- 分阶段、分优先级的上下文压缩。
- provider failure 的自适应缩容与继续执行策略。
- streaming 进度事件投影。
- workspace 维度的 LLM usage ledger。
- 统一的 read model 投影与可观测性。

自动模型能力解析与阶段感知预算的细化设计见：

- `docs/workorch-llm-auto-context-profile-design.md`

该补充文档同时约束两类配置必须保持 provider 层中立：

- `llm.modelCapabilities`: 具体模型上下文窗口能力表，禁止在代码中硬编码模型名。
- provider inference profile: `temperature`、`maxOutputTokens`、`reasoningEffort`、`thinkingBudgetTokens`，由 LLM adapter 映射到不同厂商请求字段。

## 设计目标

- 不使用单一全局 context 上限。
- 根据 provider / model 的能力动态计算输入预算。
- auto 模式默认只要求用户提供模型能力入口字段，预算字段由运行时推导，避免配置复杂度和 stale budget。
- 控制最终提交给 LLM 的 context 大小，避免 timeout 和 overflow。
- 保持 WorkOrch 基础能力中立，不在基础层硬编码具体业务场景。
- 保持所有裁剪内容可追溯。
- streaming 只作为进度与观测，不改变最终业务输入边界。
- LLM usage 必须可按 workspace / run / step 追溯，并可按 workspace 累积输入/输出/总 token 与 cache usage。

## 设计边界

### 必做

- Provider / model profile 配置。
- token budget 估算与 bytes hard limit 兜底。
- 优先级上下文压缩。
- stage-aware context 组成。
- provider failure 的 shrink / retry / decision policy。
- contextBudget、stream progress、usage ledger 的 read model 投影。

### 不做

- 不把 streaming delta 直接作为业务状态流转依据。
- 不把 usage 逻辑硬编码进具体 workspaceTemplate。
- 不引入与 provider 无关的独立 context 规则分支。

## 总体架构

### 1. Provider/Model Profile

为每个 provider/model 建立 context profile，作为预算计算输入。

运行时 profile 字段：

- `contextWindowTokens`
- `maxInputTokens`
- `reservedOutputTokens`
- `safetyRatio`
- `contextUtilizationRatio`
- `bytesHardLimit`
- `maxRequestBytes`
- `estimatedTokensPerByte`

profile 分层：

- provider default
- model override
- manual provider override

普通配置入口应保持最小化：

- `provider`
- `baseURL`
- `apiKey`
- `model`
- `timeout`
- `streaming`
- `contextWindowTokens`

`auto` 模式下，`maxInputTokens`、`reservedOutputTokens`、`bytesHardLimit`、`maxRequestBytes`、`estimatedTokensPerByte` 等高级预算字段不作为用户配置入口；即使旧配置中残留这些字段，也应由运行时忽略并重新推导。`manual` 模式才允许这些字段生效。

### 2. Context Budget Pipeline

最终 provider request 构造链路增加以下步骤：

1. 收集原始 request。
2. 按 profile 计算预算。
3. 按 stage 组装上下文。
4. 按优先级压缩。
5. 生成 `contextBudget` 元数据。
6. 若仍超限，执行 adaptive shrink / continuation policy。

### 3. Event Projection

streaming provider 产生的 chunk 不直接改变业务结果，只投影为运行期 progress event。

### 4. Usage Ledger

每次 LLM 调用生成 usage ledger entry，按 workspace / run / step 聚合。

## Context Budget 设计

### 预算计算

能力来源优先级如下：

1. provider 显式 `contextWindowTokens`。
2. `llm.modelCapabilities` 按 provider/model pattern 命中的 `contextWindowTokens`。
3. provider 动态 metadata 能力发现，后续扩展。
4. 保守默认。

auto 模式公式：

```text
reservedOutputTokens = min(max(contextWindowTokens / 16, 4096), 65536)
inputWindow = contextWindowTokens - reservedOutputTokens
maxInputTokens = floor(inputWindow * contextUtilizationRatio)
bytesHardLimit = min(maxInputTokens / estimatedTokensPerByte, maxRequestBytes)
```

默认值：

```text
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
maxRequestBytes = 4 MiB
```

该策略会尽量合理使用模型能力，但不会压满 provider 上限。它给输出、token 估算误差、provider 网关差异和请求 envelope 留出缓冲。

以 `contextWindowTokens=204800` 为例：

```text
reservedOutputTokens = 12800
inputWindow = 192000
maxInputTokens = 144000
bytesHardLimit = 576000 bytes
```

使用率：

```text
最大输入预算占总上下文 = 70.31%
输入预算 + 保留输出占总上下文 = 76.56%
```

如果需要突破默认使用率，应进入 `manual` 模式，或在后续版本引入单一 auto 调节项；不应重新要求普通用户配置多个高级预算字段。

### 优先级压缩

建议上下文分层：

- P0: 必保留
  - Goal
  - DoD
  - 当前 step
  - 当前失败
  - 最新 decision / approval
  - runtimeCapabilities
- P1: 尽量保留
  - recent governance
  - current soul summary
  - 必要 memory recall
- P2: 可摘要
  - 历史 steps
  - 历史 graph
  - workspace manifest
- P3: 可引用
  - 完整 soulMd
  - 大文件正文
  - 旧 evidence
  - 完整历史结果

压缩原则：

- 先裁剪 P3，再裁剪 P2。
- P1 只在绝对必要时摘要。
- P0 不能无声删除；若无法容纳，必须触发 continuation / decision。

### Stage-aware Context

不同 stage 使用不同上下文配比：

| Stage | Ratio | 主要上下文 |
| --- | ---: | --- |
| planning | 1.00 | Goal / DoD / soul / manifest / 约束 |
| execution | 0.80 | 当前任务、patch、文件上下文、必要 evidence |
| runtime repair | 1.00 | failure、coverage、progress、target snapshot |
| validation / verification | 0.55 | DoD、测试输出、evidence refs |
| artifact / final / result | 0.50 | 结果、artifact、残余风险 |
| decision / approval | 0.60 | 治理上下文、候选补充信息、影响范围 |
| provider retry | 0.50 | provider failure recovery 所需的紧凑上下文 |

stage ratio 只影响本次 provider request 的 `maxInputTokens` 和 `bytesHardLimit`，不修改 provider 基础 profile。

## Adaptive Shrink Policy

遇到以下情况时触发 shrink：

- provider timeout
- context-too-large
- provider response invalid due to oversized input

shrinking 分级：

- level 1: 压缩 P3。
- level 2: 压缩 P2 + P3。
- level 3: 仅保留 P0 / P1。
- level 4: 进入 continuation / decision。

要求：

- shrink level 必须写入 request metadata。
- shrink 不得改变治理语义。
- provider retry 要按同一失败根因递进，而不是每次重置。

## Provider Failure Policy

建议统一分类：

- `provider_timeout`
- `context_too_large`
- `rate_limited`
- `auth_or_config_error`
- `provider_unavailable`

处理原则：

- retryable failure 执行有限次重试 + shrink。
- 不可自动恢复失败进入治理 decision。
- provider failover 可选启用，但必须保留原 provider/model 记录。
- 失败后要记录原因、预算、shrink level 和原始请求摘要。

## Streaming 设计

### 目标

streaming 的价值是：

- 降低长任务静默感。
- 提供实时可观测性。
- 辅助排查 provider 卡顿或中断。

### 约束

- streaming 不直接驱动业务状态。
- streaming 不代替完整 JSON 决策。
- streaming delta 不默认展示给 TUI 主结果区。

### 事件模型

建议通过 EventHub 投影 run progress event，字段至少包括：

- `workspaceId`
- `runId`
- `stepId`
- `providerId`
- `model`
- `chunkCount`
- `receivedBytes`
- `elapsedMs`
- `status`
- `summary`

## Usage Ledger 设计

### 目标

按 workspace 维度记录 LLM usage，支持：

- workspace 聚合
- run 追溯
- step 明细
- provider/model 分布
- 估算与真实 usage 区分
- cache usage 统计

### 记录字段

建议字段：

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
- `cacheUsage`
- `requestBytes`
- `responseBytes`
- `streaming`
- `contextBudget`
- `startedAt`
- `completedAt`
- `durationMs`
- `status`
- `failureClass`

### 估算策略

- provider 返回真实 usage 时优先使用真实值。
- provider 未返回 usage 时使用 profile 估算。
- 估算结果必须标记 `estimated=true`。
- provider 返回 cache usage 时优先使用原生 cache token 字段。
- provider 未返回 cache usage 时，只能基于 provider-neutral cachePlan 估算 cacheable / non-cacheable input tokens。
- `llmUsage` 不处理费用、账单或价格字段；费用治理不属于 runtime usage ledger。

## Read Model / UI

### Run Snapshot

应投影：

- provider / model
- contextBudget
- shrink level
- truncation rules
- stream progress
- usage summary

### Workspace Snapshot

应投影：

- workspace 级 token / cache 汇总
- 最近 run 使用趋势
- 失败请求与重试次数

### UI 约束

- Web 可展示完整细节。
- TUI 只展示简化状态，不显示过多 delta 或原始账本细节。

## 验收标准

- 小上下文不触发裁剪。
- 大上下文按 profile 预算裁剪。
- provider timeout 后会缩容重试。
- 超过可承载预算时进入 continuation / decision。
- streaming 开启后能看到 progress event。
- streaming 完成后仍按完整 JSON 决策执行。
- workspace 维度能查询 LLM usage 聚合，包括输入 token、输出 token、总 token 与 cache usage。
- web / TUI 使用同一 read model。

## 与 TODO 的关系

`docs/model-aware-llm-context-budget-todo.md` 负责跟踪任务拆解和收口状态。

本设计负责定义：

- 预算模型
- 压缩策略
- failure policy
- streaming 事件投影
- usage ledger
- read model 展示边界
