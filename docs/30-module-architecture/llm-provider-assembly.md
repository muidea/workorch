# LLM Provider Assembly

Status: active

Type: module-architecture

Last Updated: 2026-07-17

## 当前生产合同（2026-07-15 cutover）

WorkOrch 以内置 `ai-proxy` 为唯一默认 LLM endpoint：

1. canonical config 使用 `llm.aiProxy` + `llm.defaultChat` + `llm.budget` + 可选 `llm.customProviders`。
2. `blocks/llm` 通过 `GET /v1/models` 获取精确模型容量，并维护进程内 TTL/LKG catalog cache。
3. application activation 解析完整 `ProviderModelTarget`，写入 typed `RuntimeBootstrap.Activation.ProviderTarget`。
4. Runtime V2 prerequisite 封存完整 `ProviderCeiling`（target + token/byte ceiling + capability/configuration revision）；输出上限直接采用 selected model 的 `maxOutputTokens`，输入 token/byte ceiling 由 selected model 的 `contextWindowTokens` 自动派生，不叠加进程级固定 output/request cap。
5. candidate/completion/embedding 强制使用已封存 target；不存在 provider map fallback 或 provider context 传递。
6. `llm.aiProxy.apiKey` 是可选客户端身份配置；`/v1/models`、chat、stream 与 embedding 必须使用同一 header 规则，仅在 key 非空时发送 Bearer credential。

已删除：

- `defaultProvider` / `providers` / `modelCapabilities`
- model pattern registry / auto-manual context profile / conservative default
- `selectProviderConfig` 与 map 遍历回退
- activation `llmProviderId` 与 provider-only context value

设计执行依据（历史收口稿，非当前实现 SoT）：`docs/90-archive/completed/ai-proxy-primary-llm-provider-cutover-design-2026-07-15.md`。
当前事实见 `docs/01-overview/current-state.md` 与 `internal/modules/blocks/llm/readme.md`。

---


## 历史正文（cutover 前参考，不再作为实现 SoT）

# LLM Provider Assembly Architecture

Status: active

Type: module-architecture

Last Updated: 2026-07-14

## Purpose

本文件说明 LLM provider 相关配置解析、上下文预算、request 组装和 provider-facing 审计的实现落点。

生产主线以 **Runtime V2** 为准：`blocks/llm` 经 EventHub 响应 Orchestrator 的
`RequestRuntimeCandidateCommand/Result`，只做 provider-facing 组装与 typed candidate
解码；不拥有 Run aggregate，不裁决 acceptance，不驱动 `StagePacket` / `NextAction` /
`DispatchNextStep`。

## Runtime V2 Boundary

| 关注点 | 边界 |
| --- | --- |
| 入口 | Orchestrator sealed `RuntimeDecision` + `AuthorityTransaction` + 已持久化 definition context + candidate attempt |
| 输出 | 与当前 action 对应的 typed candidate（state-change / validation / assessment / governance）或 `RuntimeCandidateFailure` |
| 不输出 | convergence state、focus 重排、authority、repair objective、Result owner 写入指令 |
| 状态 | 不写 Run / Result / Memory / EvidenceRecord；attempt 预算与 retry disposition 由 Orchestrator |
| 非生产 | V1 `RuntimeAdapter` registry、execution decision map 解析、通用 runtime executor、`StagePacket` / `NextAction` / `DispatchNextStep` |

语义层 stage / output policy 只用于 LLM 意图与严格度（层 A），**不是** Orchestrator 状态机状态（层 B 以 `RuntimeDecision.Action` 为准）。下文 output mode / stage-contract 描述若仍使用历史字段名，一律理解为 provider-facing 渲染输入，不得回写成 V1 调度权威。

## Main Responsibility

该层负责：

- provider/model capability resolution
- context profile resolution
- auto/manual budget calculation
- stage-aware / required-output-aware budget application
- request audit projection
- provider request rendering
- Runtime V2 candidate 严格解码与 contract 校验

## Main Inputs

- provider config
- model capability table
- Orchestrator 投影的 typed decision / authority / definition / requiredOutput
- runtime stage / output policy（语义层，非 V2 状态机）
- context budget policy
- typed `PromptContext`（C0–C5）

## Provider Resolution

本层需要完成：

- provider type / model / endpoint 的解析
- model capability table 与 context profile 的匹配
- auto/manual budget resolution
- stage-aware / ceiling-aware budget application

但不决定：

- Goal / DoD 语义
- acceptance gate
- Result / Artifact 结论
- Runtime V2 focus / authority / terminal 推进

## Main Outputs

- normalized provider context profile
- request messages / prompt sections
- contextBudget / request audit metadata
- provider-facing output mode / stage-contract projection（渲染用）
- typed Runtime V2 candidate 或具体 failure

## Request Rendering Direction

provider-facing request 组装应至少负责：

- stable/runtime prefix 与 volatile execution tail 的分层渲染
- cache-aware section organization
- redaction-aware request rendering
- request audit projection
- raw request/response/stream 的 ref 策略

provider-specific 差异应尽量限制在 adapter 内，不扩散到上层 flow / product 文档。

C0/C1/C2 由 typed `PromptContext` 渲染为稳定前缀；C3 及以后只进入动态消息。请求不得携带自由 `Input map[string]any`、V1 execution decision 或 capability map 重建。

## Output Mode Selector Landing

> **Runtime V2 注**：output mode 选择若仍存在，只能作为 Orchestrator 已密封决策的
> provider-facing 渲染输入；不得由 LLM block 重选，也不得改写 V2 authority /
> focus / terminal。下列 `executionGraph` / `incrementalArtifact` 字段属于历史渲染
> 合同面，不是 `StagePacket` / `NextAction` 生产调度。

当 runtime 需要大输出承载且 Orchestrator 已选定输出模式时，采用
orchestrator 选择 + LLM block provider-facing 渲染的双落点合同。

orchestrator 至少负责：

- 基于 runtime policy、continuation state、上下文预算和中立阈值选择
  `executionGraph` 或 `incrementalArtifact`（若该路径仍启用）
- 将选择结果写入 sealed candidate context / step 投影字段（非 V1 executionContext map）：
  - `outputMode`
  - `requestedOutputMode`
  - `outputModeSelector.reason`
  - `outputModeSelector.priority`
  - `artifactOutputContract`
  - `artifactContinuationContext`
- 当 active continuation 存在时，恢复 continuation 相关 stage metadata

LLM provider assembly 至少负责：

- 读取 orchestrator 已投影的 output mode / stage contract
- 按 mode 生成互斥的 provider-facing prompt sections 和 response schema
- 将 mode、selector、artifact phase、continuation metadata 写入 request audit
- 对 mode-specific protected fields 执行 context budget / cache 分层校验

约束：

- selector 只能选择输出承载模式，不能改写 V2 `RuntimeDecision` / authority / focus
- selector 只能使用中立信号，不能依赖业务类型、语言、框架、文件名或 deliverable 文本
- selector 结果必须进入 LLM-facing contract 投影，不能只保留在 runtime internal map
- LLM block 不得重新选择 output mode；发现 contract 缺失或互斥字段冲突时，应返回
  request assembly defect / contract failure（typed），由 Orchestrator 处理 attempt

## Mode-Specific Request Assembly

本层必须对不同输出模式生成互斥的 provider-facing request 形态（渲染合同；非 V1 调度）。

### `executionGraph`

当 mode 为 `executionGraph` 时，本层负责：

- 组装普通 graph / assessment / blocker 相关 schema
- 清理 incremental-only 字段：
  - `artifactOutputContract`
  - `artifactContinuationContext`
  - `artifactContinuationId`
  - chunk/plan/repair 专用 contract

### `incrementalArtifact`

当 mode 为 `incrementalArtifact` 时，本层负责：

- 组装 plan / chunk / context / chunk-repair 对应的 request-side stage contract
- 只暴露 metadata-only continuation cursor
- 按当前 phase 选择唯一 response schema

最小稳定要求：

- `artifactPlan`、`artifactChunk`、`artifact_context`、`artifactChunkRepair` 互斥
- response 顶层不得包含 `outputMode`
- `artifact_context` 只能要求 bounded read graph
- chunk 请求只能携带 metadata-only 连续性信号，不能把 raw tail 正文重新注入 prompt
- 当上一轮 `incrementalArtifact` 响应因 schema / mode contract / artifact 字段合同失败被拒绝时，下一轮 request 仍必须投影 `incrementalArtifact` 及对应 phase contract；只允许修正 artifact 输出结构，不得把大载荷回退到普通 `executionGraph`
- same-contract retry 最多 attempt 1；disposition 由 Orchestrator，不由本层发起第三次请求

## Stage-Contract Projection Boundary

本层对 stage-contract 的 provider-facing 投影至少应覆盖：

- `runtimeStage`
- `runtimeStageMode`
- `stateChangingOutputRequired`
- `repairObjective`
- `requiredOutput` / `RequiredOutputContract`
- `outputMode`
- `artifactOutputContract`
- `artifactContinuationContext`
- `outputModeSelector`

约束：

- 当 mode 回落为 `executionGraph` 时，incremental-only 字段必须从当前 step 投影中清理
- 当 mode 保持 `incrementalArtifact` 做 contract retry 时，`requiredOutput` 不得残留 `mustIncludeStateChangingNode=true` 等 graph-only 强约束
- completion follow-up / assessment-only correction 不得继承已完成 artifact 的 chunk/plan contract
- stage-contract 投影缺失时，应视为 request assembly defect，而不是让 LLM 自行猜测输出模式
- assessment candidate 只能声明当前 focus 的 satisfied/failed、summary 与 evidence/artifact refs；不得夹带 execution graph 或 Result 写入指令

## Provider-Specific Cache Mapping

本层对 provider-specific cache mapping 的稳定职责至少包括：

- 将 core-note 中的 `C0`~`C5` cache-aware 分层映射到具体 provider request 结构
- 消费 provider 私有 cache-control / cache-key / equivalent field
- 保证 provider/model/baseURL 等动态绑定不进入 provider-neutral stable prefix 语义

当前 provider-facing 渲染规则：

- 先从 `PromptContext` 或 prompt builder 中取连续 `C0` / `C1` / `C2`
- 将这些 section 合并为第一段 stable prefix，并保留 section id heading
- OpenAI Chat Completions 与 compatible HTTP provider 将 stable prefix 渲染为第一条 `system` message
- Anthropic Messages 将 stable prefix 渲染为 `system` text block，并在该 text block 上设置 provider 私有 cache marker
- `C3` / `C4` / `C5` 保持在 stable prefix 后方，作为 user/assistant 动态 tail

约束：

- cache mapping 只改变 provider-facing request 形态，不改变 core contract
- provider-specific 字段名不得上浮到 flow / product / domain 文档
- 不同 provider 的 cache 策略差异必须收敛在 adapter 或 provider assembly 层
- 不得使用可能把动态 tail 一起标记为缓存断点的顶层 cache marker，除非该 provider 明确支持只指向 stable prefix

## Request-Log Verifier Landing

provider request / response / stream 的 request-log verifier 当前应落在本层及其验证入口中。

至少需要校验：

- stable prefix / volatile tail 是否按 provider-facing 规则正确分层
- protected fields / included / omitted / redacted sections 是否进入 request audit
- raw request / response / stream ref 是否按受控路径落盘
- request audit metadata 与实际 provider-facing request 是否一致

与此对应的验证入口应继续复用：

- `../60-verification/core-runtime-verifier.md`
- request-log / cache-aware 相关专项 verifier

### Request Audit Metadata Direction

本层输出的 request audit metadata 至少应稳定承接：

- capability source
- provider/model context profile
- max tokens / max bytes / actual bytes / estimated tokens
- included / omitted / redacted sections
- protected fields
- integrity warnings
- source refs
- output mode / selector / artifact phase metadata

这些字段用于证明“provider-facing request 是如何被组装的”，
而不是直接表达业务状态或 accepted/rejected 结论。

若当前 request 处于 `incrementalArtifact`，request audit 还至少应能说明：

- 当前 `outputMode`
- 当前 artifact phase
- continuation id / targetRef / expectedSequence 的 metadata-only 摘要
- 是否发生 mode 切换、恢复 continuation 或从 continuation 回切普通 verification

### Protected Field / Gate Metadata Boundary

当 budget / rendering 影响 provider request 准入时，本层还应负责产出：

- `contextGateAction`
- `contextGateReason`
- protected field omission / truncation 事实
- section-level included / omitted / redacted projection

边界：

- 本层只负责计算和投影这些 metadata
- 本层不直接选择 replan / blocker / terminate 等治理动作
- provider/model/baseURL 等动态绑定只能进入 request audit，不得回写为恢复状态

### Field-Level Preservation Direction

本层对受保护字段的处理必须按字段级 presence bit，而不是按大对象“是否存在”粗略判定。

至少需要稳定保护：

- `repairActionPackage.requiredOutput`
- `repairActionPackage.targetSnapshots`
- `currentFailure`
- `currentExecutionProgress`
- `coverageTargetSet`
- `processEvidenceTimeline`
- `currentProgressDigest`
- `artifactOutputContract`
- `artifactContinuationContext`

约束：

- 合同字段、进展字段、coverage 字段和 target snapshot 字段互不隐式代表
- `protected_repair_action_target_snapshot_missing_after_context_budget`
  只能在“原始输入已有 target snapshot，但预算后丢失”时触发
- `runtime_repair_process_evidence_missing`
  只能在当前确实处于 repair / continuation 且过程证据缺失时触发
- incremental-only contract 字段只能在当前 mode 为 `incrementalArtifact` 时视为 required；
  回落到 `executionGraph` 后必须清理，而不是当作缺失 protected field 继续告警

## Incremental Artifact Schema Responsibility

mode-specific schema 的正式维护责任应落在本层及其相邻实现中。

本层至少需要稳定承接：

- artifact plan response schema
- artifact chunk response schema
- artifact context graph schema
- artifact chunk repair / artifact replan schema
- mode 切换时的 schema 互斥校验

边界：

- 本层负责 provider-facing request/response schema 装配
- flow 层负责 mode 切换后的链路语义
- verifier 层负责断言 schema 互斥、mode 投影和 raw-content redaction

## Model Capability Table And Revision Cache

本层对 model capability table / revision cache 的稳定职责至少包括：

- provider/model capability 解析
- model pattern -> context window / capability profile 的匹配
- definitionRevision / request revision / cache-aware rendering 所需的版本化辅助状态
- runtime config loader 提供的 provider 配置到 provider request 组装的桥接

边界：

- model capability table 只表达 provider/model 能力，不表达 Goal / DoD 语义
- revision cache 只服务版本化装配与 request assembly，不替代正式 run/result lineage
- config loader / revision cache / request renderer 的实现拆分可以继续演进，但落点都应维持在本模块层

## Boundary

此层不负责：

- Goal / DoD 正式定义
- acceptance gate
- Result / Artifact 创建
- Runtime V2 drive / focus / authority / terminal 推进
- `StagePacket` / `NextAction` / `DispatchNextStep` 生产调度

它只负责把上游已准备好的 contract/context，映射成 provider-facing request 与 typed candidate。

补充：

- 本层不负责 accepted / rejected business closeout
- 本层不负责正式 Goal / DoD definition 版本裁决
- 本层不负责把 provider-specific cache 策略解释成业务规则
- 本层不负责决定 artifact 是否业务上满足 DoD；它只负责 mode-specific request assembly
- 本层不从历史 execution-context map 或 governance 文本重建 definition context
- attempt 预算、same-contract retry 与 failure disposition 属于 Orchestrator

## Source Documents

- `../00-core/runtime-request-cache-note.md`
- `../00-core/runtime-stage-contract.md`
- `kernel-orchestrator-governance.md`
- `../60-verification/core-runtime-verifier.md`
- `internal/modules/blocks/llm/readme.md`
