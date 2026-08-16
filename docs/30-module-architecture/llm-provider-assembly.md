# LLM Provider Assembly

Status: active

Type: module-architecture

Last Verified: 2026-08-16

## Production Configuration

WorkOrch 的 LLM surface 仅使用内置 `ai-proxy` provider identity。canonical config 为：

- `llm.aiProxy`：base URL、可选 API key、timeouts、catalog cache limits 与精确 model overrides；
- `llm.defaultChat`：默认 chat provider/model；
- 可选 `llm.defaultEmbedding`；
- `llm.budget`：context utilization 与 byte/token estimation。

旧的 `defaultProvider`、`providers`、pattern-based model capability 和 provider fallback 不属于当前配置合同。

## Owner Boundary

`blocks/llm` 是 provider catalog、target validation、provider ceiling、request transport、stream/document assembly 和
typed LLM operation result 的 owner。它通过自身 `pkg/events` 暴露 concrete command/result。

它不负责：

- 选择 canonical convergence state、repair、retry、governance 或 terminal；
- 修改 Run/Runtime/Result/Memory；
- 接纳 evidence 或评估 Goal/DoD；
- 从 raw map、projection 或 provider response 重建 authority。

complete candidate 的合同判断、attempt/correction 与 terminal disposition 属于 Orchestrator SM-3。

## AI Proxy Surfaces

| Purpose | Endpoint |
| --- | --- |
| Model catalog | `GET /v1/models` |
| Chat/candidate/semantic request | `POST /v1/responses` |
| Embedding | `POST /v1/embeddings` |

API key 非空时三个 surface 使用相同 Bearer 规则。credential、raw prompt/body/stream 和未脱敏 provider data 不得进入
Runtime aggregate、Eventstream、projection 或 Memory。

## Catalog And Selection

- `/v1/models` 或 exact `modelOverrides` 提供 context window、max output、supported operations 与 reasoning capability。
- catalog cache、TTL 与 last-known-good policy 完全属于 LLM owner；LKG 不得覆盖明确的 auth/not-ready/incompatible 事实。
- Workspace owner CAS 保存完整 `ProviderInferenceSelection`。
- fresh Start/Restart 将 selection 封入 `RuntimeBootstrap.Activation`；Resume/recovery 不重读 Workspace selection。
- Runtime prerequisite取得并密封 `ProviderCeiling`；缺失容量时 fail-closed，不猜测默认值或切换模型。
- 空 reasoning effort 表示 provider default；显式值必须在保存和运行时分别通过 capability/revision 校验。

## Active Versus Persisted Configuration

LLM owner 在进程 `Start()` 时冻结 active configuration，并提供不泄漏 endpoint/credential 的 typed revision query。
配置写接口更新 persisted configuration；若 revision 与 active snapshot 不同，application投影 `restartRequired`，新配置
在下次服务启动后生效。catalog refresh 不得把 persisted 值偷偷注入 active owner。

## Request And Cache Contract

provider-facing request 保持连续分层：

| Group | Content | Stability |
| --- | --- | --- |
| C0 | system/schema/Runtime contract | long-lived |
| C1 | Goal/DoD definition | definition-bound |
| C2 | declarative capability/governance contract | activation-bound |
| C3 | current authority/focus/required output | volatile |
| C4 | recent evidence/failure/correction | volatile |
| C5 | large refs/trace | ref-oriented |

`StablePrefixHash` 覆盖稳定 prefix，意外漂移是 request-rendering defect。provider-specific cache metadata 只能改变
transport hint，不能改变 section 内容或 Runtime authority。

## Request Session Boundary

- transport/incomplete document/transient provider结果由 bounded request-session 记录；
- authentication、configuration、protocol 或 submission-unknown 返回 typed factual outcome；
- 只有完整 document 才进入 Orchestrator candidate evaluator；
- candidate attempt 的 correction/terminal budget由 SM-3 持久化，LLM owner不私自重试形成新 candidate；
- `ArtifactContinuation` 可以作为受限产物传输/审计合同存在，但不是另一条 authority 或 dispatch 主链。

## References

- [Runtime request/cache note](../00-core/runtime-request-cache-note.md)
- [Runtime contract](../00-core/runtime-stage-contract.md)
- [LLM component contract](../../internal/modules/blocks/llm/readme.md)
