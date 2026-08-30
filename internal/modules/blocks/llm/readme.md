# LLM Capability Block

Status: active

Type: capability-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/llm` 拥有进程生命周期内冻结的 provider configuration snapshot、ai-proxy model catalog/cache、
`ProviderModelTarget` capability resolution、provider ceiling、Responses/embedding 调用、Runtime candidate provider transaction，
以及每次 physical request 的 factual audit/usage。当前内置 provider 只有 `ai-proxy` 的 OpenAI-compatible HTTP adapter。

本组件不选择 Workspace/Run/Runtime action，不修改 Goal/DoD 或 authority transaction，不决定 candidate admission、retry、
governance continuation 或 terminal state。Orchestrator 是 Runtime reducer；Application 只选择/传入 target 并消费 factual result；
Eventstream 只记录 audit notification。

## 组件交互及合同

| 交互方 | 方向 | typed EventHub 合同 | 调用/授权语义 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → LLM | `CompleteProviderCommand/CompleteProviderResult`、`GenerateEmbeddingCommand/GenerateEmbeddingResult`、active config/catalog/selection commands | completion 仅接受 exact `application/workorchd/appservice`；其余 generic command 仅接受 exact `application/workorchd`，均在读取 payload/provider 前校验 |
| `kernel/orchestrator` | Orchestrator → LLM | capabilities、provider ceiling、candidate preflight、`RequestRuntimeCandidateCommand/Result` | 所有 Runtime topic 仅接受 exact `kernel/orchestrator` source |
| Eventstream | LLM → Eventstream | `LLMUsageRecordedData` | provider execution/preflight 的 bounded factual audit（包含未提交失败）；Eventstream 校验 exact LLM source |
| ai-proxy | LLM → HTTP provider | `/v1/models`、`/v1/responses`、`/v1/embeddings` | owner-private adapter；endpoint/credential 不进入跨 owner result |

[`pkg/events`](pkg/events) 为 completion 与 embedding 定义不同的 value result；不存在 nullable generic union。completion
以 `ValidateFor(command)` 绑定 exact target/model；embedding 绑定 exact target/provider/model/input，并要求 non-empty finite
vector 与真实 non-streaming physical response fact。owner 发布前和 Application 消费后使用同一校验。
`RequestRuntimeCandidateResult` 则是受校验的封闭事实联合，`Stale | Feedback | DocumentOutcome` 必须恰好一个，并绑定 exact
workspace/run、logical/physical request identity、document hash、audit 与 usage。

## 不变量与非职责

- `Configure` 缺 EventHub 立即失败；`Start` 冻结配置且重复调用不重复订阅，`Stop` 清除订阅、配置和 physical guard。
- provider/model 必须由 exact catalog/config fact 解析；capacity 不完整时 fail-closed，不按模型名猜测、不静默换 target。
- `JSONSchemaResponse` 只有显式 capability fact 为 true 才发送；未声明时依赖同一 sealed manifest、prompt 和 local strict
  decoder，不发第二次无-format fallback call。
- Runtime logical/physical identity 在 owner guard 内执行 concurrent/duplicate at-most-once；provider outcome 不选择 caller retry。
- generic completion、embedding 与 Runtime I/O 均有 token/byte/wire/SSE/event/line 上限；成功 result 不能是同类型空值或漂移
  target。raw request、response、credential、
  endpoint 和未脱敏 diagnostic 不得进入 aggregate 或 audit notification。
- provider-facing cache 分区以 [Runtime request/cache contract](../../../../docs/00-core/runtime-request-cache-note.md) 为准：
  C0～C2 是连续稳定 prefix，C3～C5 是 current/volatile tail；意外 `stablePrefixHash` 漂移即 rendering defect。
- catalog control-plane 允许实现内的有界 GET retry；completion、embedding 与 Runtime provider execution 不以 retry/fallback
  发起第二次 physical call。

## 能力验收

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| concrete completion/embedding producer-consumer | [`pkg/events/messages_test.go`](pkg/events/messages_test.go)、[`biz/owner_provider_command_test.go`](biz/owner_provider_command_test.go)、[`application setupdraft`](../../application/workorchd/biz/appservice/setupdraft/generator_test.go)、[`activation embedding`](../../application/workorchd/biz/appservice/activationports/embedding_test.go) | normal、跨-topic wrong-result、同类型空值、target/physical/vector drift 覆盖 |
| Runtime result exhaustive union / audit identity | [`pkg/events/runtime_provider_test.go`](pkg/events/runtime_provider_test.go) | 覆盖 normal、invalid、tamper |
| Runtime exact source / duplicate / concurrent | [`biz/runtime_owner_guard_test.go`](biz/runtime_owner_guard_test.go) | 覆盖 |
| provider protocol、output bound、stream/non-stream、embedding | [`biz/http_provider_test.go`](biz/http_provider_test.go)、[`biz/http_provider_security_matrix_test.go`](biz/http_provider_security_matrix_test.go) | 仅以本地 `httptest.Server`/fake transport 覆盖主要 wire success/negative edges；不证明真实 ai-proxy、credential 或 upstream 模型兼容性 |
| candidate framing、schema、correction、cache hash | `biz/runtime_candidate_*_test.go`、`biz/provider_prompt_projection_test.go` | 覆盖 |
| active configuration freeze | `TestBizStartFreezesLLMConfigurationForCatalogAndStatus` | 覆盖 |
| catalog cache/key/concurrent refresh | [`biz/catalog_cache_test.go`](biz/catalog_cache_test.go) | 覆盖 merge、key、auth rotation、capability mapping，以及本地 HTTP transport 的 network/5xx 有界 retry、4xx no-retry、oversize/malformed body、same/cross-origin redirect、fresh→stale→expired LKG |
| lifecycle / dependency fail-fast | [`module_test.go`](module_test.go)、`TestLLMOwnerStartIsIdempotentAndTeardownClearsLifecycle`、`TestProviderCommandsPublishAuditAndRejectPostBeforePhysicalCall` | 覆盖重复 Start、Stop 清理，以及 Stop 后 completion/embedding/catalog 零 physical call |
| generic caller authorization | `TestLLMGenericCommandMatrixRequiresExactApplicationSourceBeforePayload` | 覆盖 6 个 generic topic 的 exact Application source matrix，并证明拒绝发生在 payload 读取前 |
| deployed real provider | [Service Acceptance](../../../../docs/50-product-interface/service-acceptance.md)、[Deployed Convergence DC-01/DC-02](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) | 组件级未覆盖；真实 ai-proxy endpoint、credential、模型 catalog 与 provider behavior 仅由显式 real-provider deployed gate 验收，不能计入组件单测完成度 |
