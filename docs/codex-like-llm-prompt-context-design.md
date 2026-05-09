# Codex-like LLM Prompt Context Design

## 背景

当前 WorkOrch LLM 请求主要采用单次快照方式：由 provider request 构造出一份较大的上下文对象，再序列化为单条 user message 提交给 LLM。通过对比 `/home/rangh/aispace/ai-proxy/interactions` 中 Codex 与 WorkOrch 的真实交互记录，可以看到两者存在明显差异：

- Codex 更接近多轮事件带：持续提交 message、reasoning、tool call、tool result 等结构化历史。
- WorkOrch 更接近单轮状态快照：每次提交当前执行上下文，历史连续性主要依赖快照字段本身。

本设计目标不是减少上下文到最小，而是在模型上下文容量允许范围内，尽量保留完整、多轮、连续的信息，以提升 LLM 输出稳定性、计划延续性和修复可靠性。

本设计与 [model-aware-llm-context-budget-design.md](/home/rangh/aispace/workorch/docs/model-aware-llm-context-budget-design.md) 配合使用：该文档定义模型感知预算能力，本设计定义预算内应该如何组织 prompt。

## 设计目标

- 将 WorkOrch LLM prompt 从单次快照组织升级为多轮上下文组织。
- 尽量在模型上下文窗口内保留完整历史，而不是主动压缩到最小。
- 保持 Goal / DoD 作为执行契约，禁止压缩、裁剪、摘要、改写或用 normalized view 替代。
- 在不损失语义的前提下清理 prompt 中无效修饰符和格式噪声，减少 token 浪费。
- 保持 WorkOrch 基础能力中立，不在 prompt 组织层引入具体业务场景规则。
- 所有被清理、压缩、省略的上下文必须可追溯。
- 不破坏现有 LLM JSON 输出契约、ticket、approval、memory recall、artifact 和 DoD validation 流程。

## 非目标

- 不在本阶段强制切换到 OpenAI Responses API。
- 不引入 Codex 私有事件格式作为 WorkOrch 的内部模型。
- 不把工具调用结果或 streaming delta 直接作为业务状态流转依据。
- 不把具体 workspaceTemplate 的业务知识写入 LLM 基础 prompt 构建逻辑。

## 核心原则

### Goal / DoD 不可压缩

Goal 和 DoD 是 WorkOrch 执行契约，不是普通上下文材料。

必须完整保留：

- 初始 Goal 原文。
- 所有后续补充 Goal 原文。
- 初始 DoD 原文。
- 所有后续补充 DoD 原文。
- 每条 Goal / DoD 的来源、时间、run、ticket 或输入通道。
- 当前有效 Goal View / DoD View。

禁止行为：

- 禁止摘要 Goal / DoD 原文。
- 禁止裁剪 Goal / DoD 原文。
- 禁止合并改写后只提交合并结果。
- 禁止用 normalized view 替代 original text。
- 禁止在 P0 超预算时继续调用 LLM。

如果完整 Goal / DoD 与其他 P0 必要上下文已经超过模型输入预算，必须中止本轮 LLM 调用，产生框架级错误或治理项，要求拆分目标、更换更大上下文模型或人工调整输入。

### 上下文尽量装满

上下文预算策略应以稳定输出为目标：

- 先根据 provider/model profile 计算可用输入预算。
- 在预算内优先保留完整历史。
- 只有超预算时才触发 normalization 之外的压缩。
- 最近多轮上下文优先保持原文。
- 早期历史可被摘要，但必须保留 source refs。

推荐目标占用：

- 正常请求可使用输入预算的 85% 到 95%。
- 不应为了追求小请求主动舍弃有效上下文。
- reserve output tokens 与 safety margin 必须保留，避免输出被截断或 provider 拒绝。

### 清理格式噪声不等于压缩语义

Prompt normalization 只允许做无语义损失的清理，目标是剔除无效修饰符和格式符，保留必须文本信息。

允许清理：

- 多余空行。
- 行尾空格。
- 重复分隔线。
- Markdown 装饰性标题符号。
- Markdown 加粗、斜体等非语义装饰。
- UI 展示产生的重复缩进。
- 表格对齐空格。
- ANSI 控制字符。
- JSON pretty print 的无意义缩进。

禁止清理或需保护：

- Goal 原始文本。
- DoD 原始文本。
- 用户输入原文。
- ticket / approval 原文。
- 代码块。
- patch / diff。
- 命令输出中可能有语义的空格。
- artifact 原始内容。

对于受保护内容，可以额外生成 normalized view 辅助 LLM 阅读，但 original text 必须仍然完整进入上下文。

## 上下文分层

### P0: Mandatory Contract Context

P0 必须完整提交，禁止压缩：

- Workspace identity。
- Run identity。
- 当前 stage / step。
- 所有 Goal 原文。
- 所有 DoD 原文。
- 当前有效 Goal View。
- 当前有效 DoD View。
- 未解决 ticket。
- 未解决 approval。
- 最近失败原因与恢复要求。
- 当前 LLM 需要完成的明确动作。
- 输出 JSON schema 与治理契约。

P0 超预算时，直接中止 LLM 调用。

### P1: High Continuity Context

P1 在预算允许时尽量完整提交：

- 最近 N 轮用户输入。
- 最近 N 轮 LLM 决策输出。
- 最近 N 个 step 的计划与结果。
- 最近 ticket / approval 的提交内容和处理结果。
- 最近 memory recall 结果。
- 最近 validation / repair 结果。
- 当前有效 graph 或 plan 的最新版本。

P1 只有在 P2 已处理后仍然超预算时才允许摘要较早内容。最近轮次应尽量保持原文。

### P2: Traceable Historical Context

P2 可在超预算时摘要或引用：

- 较早执行日志。
- 较早工具调用输出。
- 大体积 artifact 内容。
- 已完成 step 的详细过程。
- 重复性状态快照。
- 可通过 artifact id 追溯的大文本。

压缩后必须保留：

- workspace id。
- run id。
- step id。
- artifact id。
- content hash 或摘要标识。
- 原始内容存储位置。

## 多轮 Prompt 结构

第一阶段继续使用 `/chat/completions`，通过多 message 组织模拟 Codex 风格多轮上下文，不改变现有 JSON 输出契约。

建议消息结构：

```text
system:
  WorkOrch stable contract, response schema, governance rules, language rules.

developer:
  Context usage rules, Goal/DoD protection, neutrality constraints, output requirements.

user:
  Workspace and run identity.

user:
  Original Goal entries.

user:
  Original DoD entries.

user:
  Effective Goal View and Effective DoD View.

assistant:
  Previous normalized LLM decision output, if available.

user:
  Recent execution events, governance resolutions, memory recalls, artifact refs.

user:
  Current requested action for this LLM call.
```

长期可选方向是在 provider 支持稳定后增加 Responses API typed input 适配，但不能作为第一阶段前置条件。

## Goal / DoD 数据模型

Goal / DoD 必须以 append-only 方式记录，不允许替换旧内容。

建议结构：

```json
{
  "goals": [
    {
      "id": "goal-001",
      "source": "initial",
      "text": "original goal text",
      "createdAt": "2026-05-08T10:00:00+08:00",
      "workspaceId": "ws-...",
      "runId": "run-..."
    },
    {
      "id": "goal-002",
      "source": "ticket",
      "text": "additional goal text",
      "createdAt": "2026-05-08T10:30:00+08:00",
      "workspaceId": "ws-...",
      "runId": "run-...",
      "ticketId": "ticket-..."
    }
  ],
  "effectiveGoalView": "current effective goal view built from all goal entries"
}
```

DoD 使用同样结构。

`effectiveGoalView` 与 `effectiveDoDView` 可以帮助 LLM 理解当前目标，但不能替代原文列表。

## Prompt Normalization

### 处理顺序

```text
collect context
mark protected sections
normalize safe sections
estimate token budget
pack P0/P1/P2
compress only if still over budget
build chat messages
record budget manifest
```

### Section 级保护

每个上下文 section 必须标记 protection policy：

- `raw_required`: 原文必须提交，不允许 normalization 或压缩。
- `normalize_allowed`: 允许无语义损失清理。
- `summarize_allowed`: 超预算时允许摘要。
- `reference_allowed`: 超预算时允许只保留引用。

Goal / DoD / 用户原始输入 / ticket 原文 / approval 原文必须标记为 `raw_required`。

### Normalization Manifest

每次请求应记录 normalization 结果：

```json
{
  "normalizedSections": ["recentExecutionEvents", "historicalEvidence"],
  "protectedSections": ["goals", "dods", "openGovernance"],
  "estimatedTokensSaved": 1200,
  "semanticLoss": false
}
```

`semanticLoss` 必须为 `false`。如需要有损摘要，应进入 compression manifest，而不是 normalization manifest。

## Context Budget Manifest

每次 LLM 调用必须生成可追溯 manifest：

```json
{
  "model": "provider/model",
  "maxContextTokens": 128000,
  "reservedOutputTokens": 8192,
  "safetyMarginTokens": 4096,
  "inputBudgetTokens": 115712,
  "estimatedInputTokens": 98000,
  "includedSections": ["p0", "p1.recentTurns", "p2.artifactRefs"],
  "normalizedSections": ["p1.executionEvents"],
  "compressedSections": ["p2.historicalSteps"],
  "omittedSections": [],
  "goalDoDCompressed": false,
  "p0Overflow": false
}
```

约束：

- `goalDoDCompressed` 必须永远为 `false`。
- `p0Overflow=true` 时不得发起 provider 调用。
- `compressedSections` 必须有 source refs。
- `omittedSections` 只能包含非 P0 内容。

## 代码落点

### Provider Request Model

文件：

- [decision.go](/home/rangh/aispace/workorch/internal/modules/blocks/llm/pkg/models/decision.go)

建议新增或扩展：

- `PromptContext`
- `PromptSection`
- `PromptTurn`
- `ContractEntry`
- `ContextBudgetManifest`
- `NormalizationManifest`

### Runtime Adapter

文件：

- [runtime_adapter.go](/home/rangh/aispace/workorch/internal/modules/blocks/llm/biz/runtime_adapter.go)

职责调整：

- 从 execution request 收集 Goal / DoD append-only 记录。
- 构建多轮 PromptContext。
- 执行 section 标记、normalization、budget packing、compression。
- 生成 manifest。
- 保持 provider request 与 run/step/workspace 可追溯。

### HTTP Provider

文件：

- [http_provider.go](/home/rangh/aispace/workorch/internal/modules/blocks/llm/biz/http_provider.go)

职责调整：

- 将当前 `userPrompt()` 单条 JSON 序列化替换为 `buildMessages()`。
- 保留 system contract。
- 按 PromptContext 生成多条 chat messages。
- 保持 streaming / non-streaming 使用同一 message builder。

## 实施阶段

### Phase 1: Chat Completions 多轮化

- 建立 PromptContext 模型。
- 实现 Goal / DoD raw_required 保护。
- 实现 section normalization。
- 实现多 message builder。
- 保持现有 JSON 输出契约不变。

### Phase 2: 预算与追溯增强

- 接入 model-aware token budget。
- 完善 ContextBudgetManifest 与 NormalizationManifest。
- 将 manifest 投影到 run observability。
- 在 web / TUI 中展示本轮 prompt context 组织结果。

### Phase 3: 历史连续性优化

- 引入最近 N 轮完整上下文策略。
- 引入较早历史摘要与 artifact refs。
- 对 ticket / approval / repair continuation 做回归优化。
- 验证 Goal / DoD 补充后计划重算与后续执行一致性。

### Phase 4: 可选 Responses API 适配

- 在 provider 能力允许时增加 typed input 适配。
- 不改变 WorkOrch 内部 PromptContext。
- provider 层做 chat messages 与 typed input 的转换。

## 验收要求

### 单元测试

- Goal 原文进入 P0，且不被 normalization 修改。
- DoD 原文进入 P0，且不被 normalization 修改。
- 多次补充 Goal 时旧内容不丢失。
- 多次补充 DoD 时旧内容不丢失。
- normalization 只作用于允许 section。
- patch / diff / code block 不被清理。
- P0 超预算时不调用 provider。
- P2 超预算时生成 source refs。
- `goalDoDCompressed` 永远为 false。

### E2E 测试

- 补充 ticket 后，后续 LLM 请求能看到完整补充历史并继续同一目标。
- 补充 approval 后，后续 LLM 请求能看到审批语义与原文。
- 补充 Goal 后，新旧 Goal 均保留，执行计划基于完整目标重算。
- 补充 DoD 后，新旧 DoD 均保留，验收基于完整 DoD 执行。
- 长历史 run 在预算内尽量保留完整多轮上下文。
- 超预算时只压缩 P2 / 较早 P1，不压缩 P0。
- provider request manifest 可以追溯 included、normalized、compressed、omitted sections。
- 不出现业务场景硬编码。

### 人工核对

- 从 `ai-proxy/interactions` 抽样核对 WorkOrch 请求，不再只有单个巨大 user JSON。
- 核对 chat messages 中 Goal / DoD 原文完整存在。
- 核对 normalization 后文本更干净，但语义信息未丢失。
- 核对上下文容量允许时，历史信息保留量接近预算上限。

## 风险与约束

- 多轮上下文会增大请求体，必须依赖 model-aware budget 和 provider timeout retry 兜底。
- 若 provider 对多 message 支持不一致，需要在 provider adapter 层做兼容，但不能回退为不可追溯的大字符串。
- Normalization 必须采用 allow-list，不允许全局正则清理所有文本。
- Goal / DoD 原文进入上下文可能导致 P0 变大；这属于执行契约成本，不应通过压缩规避。

## 跟踪项

| ID | 事项 | 状态 |
| --- | --- | --- |
| CP-01 | 建立 PromptContext / PromptSection / PromptTurn 模型 | 已完成（基础模型已接入 ProviderRequest） |
| CP-02 | Goal / DoD append-only 原文进入 P0 | 部分完成（已实现 Goal/DoD 保护与不可压缩；append-only 持久化核对待补） |
| CP-03 | 实现 section protection policy | 已完成（`raw_required/normalize_allowed` 基础策略已接入） |
| CP-04 | 实现 prompt normalization allow-list | 已完成（无语义损失格式清理已接入） |
| CP-05 | 替换单 user JSON 为多 message builder | 已完成（HTTP/OpenAI/Anthropic provider 已统一） |
| CP-06 | 接入 model-aware budget packing | 已完成（复用现有 model-aware budget） |
| CP-07 | 实现 ContextBudgetManifest / NormalizationManifest | 已完成（预算/normalization 字段已投影至 read-model，并在 web/TUI 展示） |
| CP-08 | P0 overflow 阻断 provider 调用 | 已完成（runtime 已阻断 provider 调用） |
| CP-09 | ticket / approval continuation 回归验证 | 已完成（单测+`verify-software-dev-goal-dod-e2e.sh` 部署态验证通过） |
| CP-10 | Goal / DoD 补充后计划重算回归验证 | 已完成（单测+`verify-software-dev-goal-dod-e2e.sh` 部署态验证通过） |
| CP-11 | web / TUI 展示 prompt context trace | 已完成（TUI 摘要 + web 细粒度 LLM Context Budget trace 面板） |
| CP-12 | ai-proxy interactions 抽样验证 | 部分完成（脚本已就绪；strict 模式缺少 interactions 数据目录，待补数据后验收） |
