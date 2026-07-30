# LLM Provider Assembly

Status: active

Type: module-architecture

Last Updated: 2026-07-29

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

设计执行依据（历史收口稿，仅供归档检索）：
`docs/90-archive/completed/ai-proxy-primary-llm-provider-cutover-design-2026-07-15.md`。

## Runtime V2 边界

| 关注点 | 当前合同 |
| --- | --- |
| 入口 | Orchestrator sealed canonical `ConvergenceState`、`AuthorityTransaction`、manifest、definition context、request session 与可选 sealed correction projection |
| 输出 | typed candidate，或 request/candidate 的 stable factual category + audit-safe original diagnostic |
| provider correction | 只消费 C4 provider-safe field violations、shape hash 与 Orchestrator-sealed `CorrectionDirective` |
| 不输出 | correction/retry/terminal disposition、focus 重排、authority、repair objective、Result/Run/Workspace 写入指令 |
| 状态 | provider catalog/cache 与技术 request session adapter state；不写 Runtime aggregate、Run、Result、Memory 或 EvidenceRecord |

`blocks/llm` 只负责 provider-facing request 渲染、transport、stream/document 汇聚和 strict candidate decode。
complete candidate 的 contract evaluator 与 attempt/correction state 属于 Orchestrator SM-3；LLM Block 不得根据
provider code/message 选择 correction、retry、governance 或 terminal。

## Provider 与模型容量

- built-in `ai-proxy` 是唯一默认 provider；`customProviders` 只能由显式 target 使用，不能参与 map fallback。
- `/v1/models` 返回的 model identity、context window、max output、capability 与 revision 是 catalog SoT；不存在本地
  model pattern、conservative token default 或固定 4096/8192 output fallback。
- activation 必须把 ready 的完整 provider/model target 封入 `RuntimeBootstrap`；后续 request 只使用 sealed target。
- provider ceiling 必须按 selected model 自动推导并持久化；catalog 缺失、target 不 ready 或容量字段无效时返回
  typed configuration factual result，不能猜测容量或静默切换模型。
- TTL/LKG 只属于 LLM owner 的 catalog 可用性策略；LKG 必须携带 revision/observed-at，不能覆盖显式 not-ready、
  authentication 或 incompatible capability 事实。

## C0～C5 与 stable prefix

- C0：长期 runtime/schema contract；C1：Goal/DoD；C2：declarative capability/governance contract。
- C3～C5：current focus、manifest、volatile evidence、request/session 与 correction tail。
- `StablePrefixHash` 只覆盖稳定 section；同 authority/manifest 的 attempt 0/1 必须保持 C0～C3 与 stable prefix
  不变，attempt 1 只能增加 C4 的 provider-safe rejection + sealed correction。
- provider-specific cache marker 只能改变 request transport 形态，不能改变 section 内容、hash 或 Runtime authority。
- request 组装不得使用 `map[string]any`、V1 execution map、raw error/reason 或从 projection 重建 authority。

详细 section、预算、continuation 与 audit 规则以
[runtime-request-cache-note.md](../00-core/runtime-request-cache-note.md) 为准。

## 双层反馈与安全边界

LLM owner 必须分别产生：

1. persisted/audit-safe original diagnostic：保留允许跨边界的原 class/code/message、signature、field violations、
   request identity 与 truncation/redaction 元数据；
2. provider-safe correction projection：只包含 manifest-bound path、actual、allowed/required/forbidden、shape hash
   和 bounded objectives。

两者不得互相覆盖。credential、endpoint、raw prompt、raw provider body、raw stream 或未脱敏内容不得进入
Runtime aggregate、eventstream、projection、memory 或下一次 candidate context；只记录合同允许的 hash、bytes、
disposition 与 redacted/omitted 标记。

## Request/session 结果

provider/request session 与 candidate attempt 正交：

- incomplete document、fragment/EOF、429/5xx 等 transient result 进入有界 request-session recovery，不创建 attempt；
- authentication/configuration/protocol/submission-unknown/continuation-exhausted 先形成 typed factual result，不创建
  candidate attempt；
- complete document 才进入 Orchestrator 的 single candidate evaluator；
- candidate rejection 的 original diagnostic 必须先持久化，SM-3 才能决定 typed terminal 或唯一 correction；
- attempt 1 再次 complete rejection 形成 typed terminal，不存在第三次 provider request，也不创建无信息 ticket。

## 禁止边界

- 不选择 canonical `ConvergenceState`、candidate correction、retry、governance 或 terminal。
- 不扩展 manifest 的 operation kind、parameter、target、effect、coverage 或 dependency allowed set。
- 不裁决 evidence、assessment、Goal/DoD 或 Result。
- 不持有其他 owner repository/service，不经 JSON/reflect/Envelope 传递跨 owner 合同。
- 不恢复 `StagePacket`、`NextAction`、`DispatchNextStep`、execution graph 或 incremental-artifact V1 主链。
- 不把 audit/projection 或 provider-private cache metadata 回写成 Runtime state。

## Source Documents

- [runtime-stage-contract.md](../00-core/runtime-stage-contract.md)
- [runtime-request-cache-note.md](../00-core/runtime-request-cache-note.md)
- [kernel-orchestrator-governance.md](kernel-orchestrator-governance.md)
- `internal/modules/blocks/llm/readme.md`
- `internal/modules/kernel/orchestrator/readme.md`
