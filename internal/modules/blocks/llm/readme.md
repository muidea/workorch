# LLM Capability Block

Status: active

Type: capability-block-contract

Last Verified: 2026-09-01

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
- semantic Definition gap 仍是非权威 provider document。`undeclared_prerequisite` 必须按 schema 输出 CWI-bound
  `prerequisiteClaims` 的 exact owner/fact/operation tuple；prompt 明确禁止把 C2 已声明 requirement 描述为 undeclared，包含 C3
  已显示该 fact selectable 或已有 `ownerFactAcquisition` 的情况。LLM block 不判断 claim 真伪、不建票；同一 strict decoder 与 Orchestrator pure
  manifest admission 在 JSONSchemaResponse on/off 下统一 fail closed。
- operations renderer 只消费 sealed manifest 纯编译的 `RuntimeCandidateWireContract`，schema、C3 output contract、
  instruction 和 local strict compiler 共用其 exact hash/rules。required 与 auxiliary operation 都只要求模型提供真实
  选择；required kind、唯一 reachable variant、所有 reachable authorization 共有的 singleton binding、required
  cardinality-one singleton target 和 fixed parameter 由 Runtime 本地派生，provider 回显必须拒绝；unreachable variant
  authorization 不得重新暴露 binding。
  JSONSchemaResponse on/off 不改变该最小 wire 或 local admission；LLM block 不做 JSON repair，不选择 correction/retry。
- LLM owner 在输出边界将 incomplete、empty、non-JSON、scalar/array root、multiple JSON roots 与 trailing bytes 归一为
  no-candidate `DocumentOutcome`，不携带 content。只有恰好一个完整 JSON object 能作为 opaque content 返回；framing outcome
  不创建 candidate correction，后续 whole-document recovery/terminal 只由 Orchestrator 的 sealed request session 决定。
- Runtime logical/physical identity 在 owner guard 内执行 concurrent/duplicate at-most-once；provider outcome 不选择 caller retry。
- attempt-1 correction 通过共享纯合同核对 observation 的 manifest/objectives、objective↔witness-dimension、path/actual/shape 与 sealed witness，再独立核对
  manifest field authorization。已选中 operation rule 后产生的 field-level schema observation 必须携带由 strict decoder 生成的 exact
  `RuntimeCandidateWireRuleIdentity`，并按 wire hash、operation index、required ref/kind 与 variant 对同一 manifest 重验；LLM
  不得按 `Actual`、operation kind 近似或单 operation manifest 猜测 field authority。LLM 不把 `missing_*`/`empty_*` 或 `null`/wrong-type required-field schema observation 重新解释为
  semantic target/binding value。对后者，C4 只投影 exact rejected JSON type 与 expected schema rule，要求重写完整 document；
  它不改变 C0～C3/stable prefix，也不授权 LLM block 或 decoder 本地修复 attempt-0 文档。
- `parameter_entry_encoding` correction 在 physical call 前 exact 核对 operation kind/index、container 与每个 fixed parameter 的
  name/type；C4 仅渲染由 contract type 推导的 `{name, wireValueField}` 结构提示，不携带 attempt-0 value，也不把合法 parameter
  错写为删除指令。该投影与 `JSONSchemaResponse` capability 无关，只改变 C4，不改变 C0～C3/stable prefix。
- `operation_value_rule` 的 C3 必须投影 manifest v7 中适用于 operation/selected variant 的 exact `valueRules`；unary rule 可收紧
  JSON Schema，但本地 strict evaluator 仍是字节范围及跨参数关系的权威。C4 只渲染 exact rule ref/path 和 closed
  `actual_shape -> required_shape`，不得携带 raw rejected/corrected value。LLM block 只消费 Orchestrator 封存的 detached witness，
  不选择规则、修改 candidate、触发 owner event 或取得 attempt transition authority。
- `owner_candidate_constraint` correction 必须在 C3 保留 manifest-declared `candidateConstraints`，在 C4 仅投影 exact
  operation/parameter path、constraint ref、封闭 actual shape/required shape 与 bounded target observation；不得投影 raw parameter、
  parser diagnostic 或 owner 私有实现，也不得改变 C0～C3/stable prefix。attempt 1 的 static shape 只由 capability owner 的 typed
  conformance result证明，dynamic target applicability 只由 owner 对 current target/revision 的 typed feasibility result证明，LLM
  block 不自行判定。
  declaration 投影同时保留 operation-level 与 variant-level constraints；`file/edit` 等无 variant operation 仍使用
  原有 correction 合同。相同规则可被不同 operation 引用，lookup 必须绑定 exact operation kind，不能按 ref 跨 kind 推断。
- `operation_set_cardinality` correction 只投影 manifest v7 封存的 exact owner/target、operation kind/index violation 与
  `one_operation_per_owner_target` objective；不得让模型通过修改 operation ID、顺序、依赖或 target 规避。attempt 1 不得改变
  原 owner/target 覆盖或非冲突 operation，必须合并为一个 ordinary durable operation，除非 manifest 的 exact required sequence
  明确授权例外。
- operation variant schema/instruction 必须按 kind 投影 manifest 已编译的 allowlist；exact required-sequence variant 与普通
  accepted variant 已由 Runtime 取并集。某 accepted kind 没有 variant restriction 时，不能因另一 kind 有 allowlist 而被删除。
- generic completion、embedding 与 Runtime I/O 均有 token/byte/wire/SSE/event/line 上限；成功 result 不能是同类型空值或漂移
  target。raw request、response、credential、
  endpoint 和未脱敏 diagnostic 不得进入 aggregate 或 audit notification。
- provider-facing cache 分区以 [Runtime request/cache contract](../../../../docs/00-core/runtime-request-cache-note.md) 为准：
  C0～C2 是连续稳定 prefix，C3～C5 是 current/volatile tail；意外 `stablePrefixHash` 漂移即 rendering defect。
- operations typed command保留完整 candidate context供owner-local integrity/freshness校验；provider-facing C3只投影sealed
  manifest命中的target/stage supporting context，不展开raw operation-admissibility、prerequisite-observation或无关owner/MCP
  catalog，operation vocabulary只来自同一个sealed manifest。manifest引用的supporting snapshot缺失、漂移或owner/kind
  不匹配时必须在physical submission前返回unsubmitted integrity fact，不能静默投影为空。
- streaming configured timeout只表示header/body idle interval并随正数wire read重置；独立hard/caller deadline保持整次调用有界。
  body idle 与hard deadline返回不同typed transport identity，保留submitted/response bytes且丢弃partial payload/usage；adapter
  不据此发起第二次physical call。
- catalog control-plane 允许实现内的有界 GET retry；completion、embedding 与 Runtime provider execution 不以 retry/fallback
  发起第二次 physical call。

## 能力验收

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| concrete completion/embedding producer-consumer | [`pkg/events/messages_test.go`](pkg/events/messages_test.go)、[`biz/owner_provider_command_test.go`](biz/owner_provider_command_test.go)、[`application setupdraft`](../../application/workorchd/biz/appservice/setupdraft/generator_test.go)、[`activation embedding`](../../application/workorchd/biz/appservice/activationports/embedding_test.go) | normal、跨-topic wrong-result、同类型空值、target/physical/vector drift 覆盖 |
| Runtime result exhaustive union / audit identity | [`pkg/events/runtime_provider_test.go`](pkg/events/runtime_provider_test.go) | 覆盖 normal、invalid、tamper |
| Runtime exact source / duplicate / concurrent | [`biz/runtime_owner_guard_test.go`](biz/runtime_owner_guard_test.go) | 覆盖 |
| provider protocol、output bound、stream/non-stream、embedding | [`biz/http_provider_test.go`](biz/http_provider_test.go)、[`biz/http_provider_security_matrix_test.go`](biz/http_provider_security_matrix_test.go) | 本地覆盖 wire success/negative edges、stream idle/hard deadline、持续进展与 reasoning-only terminal；不证明真实 ai-proxy、credential 或 upstream 模型兼容性 |
| candidate framing、schema、gap claim、correction、cache hash | `biz/runtime_candidate_*_test.go`（含 `TestRequestCandidateClassifiesNonObjectProviderOutputsAsNoDocument`、`TestRuntimeProviderRequestPreservesStablePrefixAndTypedContext`、`TestRuntimeCandidateAuxiliarySingletonWireIsMinimalWithJSONSchemaResponseOnOrOff`、`TestRuntimeCandidateOutputProjectsExecutableValueRulesIntoC3AndSchema`、`TestRuntimeCandidateRetryProjectsOwnerConstraintOnlyInC4WithoutStablePrefixDrift`）、`biz/runtime_semantic_prompt_test.go`、`pkg/models/runtime_candidate_correction_test.go`（含 `TestRuntimeCandidateCorrectionInputAcceptsExactNullRequiredContainer`、`TestRuntimeCandidateCorrectionInputValidatesOperationValueRuleAgainstManifest`、`TestRuntimeCandidateManifestCorrectionProofUsesExactWireRuleAcrossOperationKinds`、owner constraint/operation-set manifest-binding matrix）、`biz/provider_prompt_projection_test.go` | 覆盖 no-candidate framing 零 content、required/auxiliary 最小 wire、multi-operation exact rule identity、typed undeclared-prerequisite claim instruction、JSONSchemaResponse on/off、missing/empty/null/wrong-type schema witness 与 semantic witness 分离、value-rule/owner constraint/operation-set C3 declaration与C4 bounded witness、failure/correction exact binding、manifest-scoped C3 与 C0～C5 identity |
| active configuration freeze | `TestBizStartFreezesLLMConfigurationForCatalogAndStatus` | 覆盖 |
| catalog cache/key/concurrent refresh | [`biz/catalog_cache_test.go`](biz/catalog_cache_test.go) | 覆盖 merge、key、auth rotation、capability mapping，以及本地 HTTP transport 的 network/5xx 有界 retry、4xx no-retry、oversize/malformed body、same/cross-origin redirect、fresh→stale→expired LKG |
| lifecycle / dependency fail-fast | [`module_test.go`](module_test.go)、`TestLLMOwnerStartIsIdempotentAndTeardownClearsLifecycle`、`TestProviderCommandsPublishAuditAndRejectPostBeforePhysicalCall` | 覆盖重复 Start、Stop 清理，以及 Stop 后 completion/embedding/catalog 零 physical call |
| generic caller authorization | `TestLLMGenericCommandMatrixRequiresExactApplicationSourceBeforePayload` | 覆盖 6 个 generic topic 的 exact Application source matrix，并证明拒绝发生在 payload 读取前 |
| deployed real provider | [Service Acceptance](../../../../docs/50-product-interface/service-acceptance.md)、[Deployed Convergence DC-01/DC-02](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) | 组件级未覆盖；真实 ai-proxy endpoint、credential、模型 catalog 与 provider behavior 仅由显式 real-provider deployed gate 验收，不能计入组件单测完成度 |
