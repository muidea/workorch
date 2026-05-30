# LLM Provider Assembly Architecture

Status: active

Type: module-architecture

Last Updated: 2026-05-30

## Purpose

本文件说明 LLM provider 相关配置解析、上下文预算、request 组装和 provider-facing 审计的实现落点。

## Main Responsibility

该层负责：

- provider/model capability resolution
- context profile resolution
- auto/manual budget calculation
- stage-aware budget application
- request audit projection
- provider request rendering

## Main Inputs

- provider config
- model capability table
- runtime stage
- context bundle
- context budget policy

## Provider Resolution

本层需要完成：

- provider type / model / endpoint 的解析
- model capability table 与 context profile 的匹配
- auto/manual budget resolution
- stage-aware budget application

但不决定：

- Goal / DoD 语义
- acceptance gate
- Result / Artifact 结论

## Main Outputs

- normalized provider context profile
- request messages / prompt sections
- contextBudget / request audit metadata

## Request Rendering Direction

provider-facing request 组装应至少负责：

- stable/runtime prefix 与 volatile execution tail 的分层渲染
- cache-aware section organization
- redaction-aware request rendering
- request audit projection
- raw request/response/stream 的 ref 策略

provider-specific 差异应尽量限制在 adapter 内，不扩散到上层 flow / product 文档。

## Provider-Specific Cache Mapping

本层对 provider-specific cache mapping 的稳定职责至少包括：

- 将 core-note 中的 `C0`~`C5` cache-aware 分层映射到具体 provider request 结构
- 消费 provider 私有 cache-control / cache-key / equivalent field
- 保证 provider/model/baseURL 等动态绑定不进入 provider-neutral stable prefix 语义

约束：

- cache mapping 只改变 provider-facing request 形态，不改变 core contract
- provider-specific 字段名不得上浮到 flow / product / domain 文档
- 不同 provider 的 cache 策略差异必须收敛在 adapter 或 provider assembly 层

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

这些字段用于证明“provider-facing request 是如何被组装的”，
而不是直接表达业务状态或 accepted/rejected 结论。

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

约束：

- 合同字段、进展字段、coverage 字段和 target snapshot 字段互不隐式代表
- `protected_repair_action_target_snapshot_missing_after_context_budget`
  只能在“原始输入已有 target snapshot，但预算后丢失”时触发
- `runtime_repair_process_evidence_missing`
  只能在当前确实处于 repair / continuation 且过程证据缺失时触发

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

它只负责把上游已准备好的 contract/context，映射成 provider-facing request。

补充：

- 本层不负责 accepted / rejected business closeout
- 本层不负责正式 Goal / DoD definition 版本裁决
- 本层不负责把 provider-specific cache 策略解释成业务规则

## Source Documents

- `../00-core/runtime-request-cache-note.md`
- `../00-core/runtime-stage-contract.md`
- `../60-verification/core-runtime-verifier.md`

## Open Migration Notes

- 当前很多 request rendering 规则仍写在 core contract 中，后续需要继续收束到本层。
