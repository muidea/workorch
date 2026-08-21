# Runtime Request Provider Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-18

## Typed Request Binding

Runtime 的唯一 provider transaction 管道使用显式 `DocumentKind`：

- `operations` 消费 typed `RuntimeCandidateContext`，其 authority 来自 persisted facts 与 current transaction；
- `semantic_plan` 消费 typed `RuntimeSemanticPlanRequestContext`，其 authority 来自 sealed Definition seed 与 semantic manifest；
- 两类字段互斥；空 kind 不解释为 operations，command/result/audit 必须回显同一 kind；
- LLM block 只消费 typed command，不从 map、prompt 文本或临时上下文重建 authority。

Orchestrator 在 request 前封存 logical request、authority/manifest identity、C0～C5 section hashes、
`StablePrefixHash` 与 `RuntimeExpectedDocumentContract`。transaction、definition、manifest 或 execution contract 漂移时不得 seal。

该persisted request session与canonical convergence state、`RequiredOutputContract`、`DocumentKind`、ExpectedDocument、
provider ceiling、reasoning effort、budget和stable-prefix identity共同构成purpose-bound semantic invocation contract。产品层
可以把这组约束称为semantic activation，但不得据此新增独立`SemanticActivation` repository/lifecycle、第五状态机或覆盖
`RuntimeBootstrap.Activation`。assessment/governance candidate仍使用`operations` document envelope，并由current canonical
state和RequiredOutput将允许variant收窄为唯一类型；它们不能借envelope执行operation或升级role。

## Provider Ceiling And Expected Document

唯一 output-token 权威是 LLM owner 对已封存 provider/model selection 与 configuration revision 返回的 typed
`ProviderCeiling`。下列值必须逐项相等：

```text
ProviderCeiling.EffectiveMaxOutputTokens
= RuntimeExpectedDocumentContract.MaxOutputTokens
= RequestRuntimeCandidateCommand.ExpectedDocument.MaxOutputTokens
= ProviderRequest.MaxOutputTokens
= provider-facing max_output_tokens
```

任何不等值、缺失 ceiling、selection/revision 漂移或 adapter override 都必须在 provider submission 前形成 typed
context-integrity failure，保持 `providerSubmitted=false` 和零 physical call。recovery 复用同一 ExpectedDocument
identity/token；不得重读配置、提高预算或按 model name 建第二套 ceiling。

provider ceiling 只约束模型可输出长度。manifest/schema 的数组、字符串、target、binding、content/effect 上限独立决定
Runtime 可接受的文档，二者不能互相替代。

## Whole-Document Transaction

semantic-plan 与 operations 均要求一次 response 返回恰好一个完整、紧凑 JSON object：无 Markdown fence、解释、
前后缀或重复输入；object `additionalProperties=false`，renderer、schema、strict decoder 与 evaluator 共用同一
vocabulary、shape 和 sealed limit。

- incomplete/EOF、output limit、timeout、429/5xx 与允许恢复的 transport failure 只进入 request-session recovery；
- 401/403、configuration/protocol failure、submission-unknown 和 recovery exhausted 形成 typed request fact；
- request failure 不创建 candidate attempt；不得跨 physical call 拼接 partial output 或把 raw suffix 注入下一次 prompt；
- 每个 retry 使用新的 `PhysicalCallID`，但保持 logical request、ExpectedDocument、C0～C3、transaction、batch 与 stable prefix；
- complete document 只进入一个 evaluator；candidate attempt 0/1 与 physical-call recovery 是独立预算。
- provider call allocation、usage、unknown consumption和duplicate accounting遵守
  [Execution Budget](runtime/execution-budget.md)；physical retry不能重置或借用candidate/repair budget。

## Request Audit

每次 logical request 至少持久化：

- `LogicalRequestID`、每次 `PhysicalCallID`、recovery generation；
- document kind、expected-document hash、response hash/bytes、document disposition、`providerSubmitted`；
- `StablePrefixHash`、各 section hash、`WorkspaceDefinitionHash`、`CapabilityContractHash`；
- current focus/required-output bundle hash、volatile-tail hash、Definition revision/change impact；
- included、normalized、compressed、omitted、redacted sections；
- protected fields、integrity warnings 与 bounded source refs。

历史字段名 `stageBundleHash` 可以作为存储兼容名存在，但其语义只能是 current focus/required-output bundle，
不能恢复 StagePacket/NextAction。raw request/response body、credential、endpoint 与未脱敏内容只记录允许的 hash、
bytes、disposition 和 redacted/omitted metadata。

attempt 1 只允许改变 C4 中 provider-safe `CandidateRejection` 与 Orchestrator-sealed `CorrectionDirective`。若修正需要
改变 operation authorization、target policy、allowed targets、manifest 或 C0～C3，则必须终止 current transaction，
等待 SM-2 以新 owner fact 建立新 authority。

## Context Priority And Integrity

| Priority | Protected content |
| --- | --- |
| P0 contract | Runtime/schema、workspace/run identity、exact current Definition partition、current required output |
| P1 current failure | stable failure class/signature、repair objective 与 bounded diagnostic |
| P2 coverage scope | current focus/CWI、coverage target 与 evidence requirement |
| P3 current evidence | latest accepted action/validation/artifact identity 与 progress digest |
| P4 current snapshot | sealed target/manifest/repair-target snapshot |
| P5+ | recent attempts、historical context 与 raw archives |

P0～P4 是当前决策必要上下文。budget、redaction 或 compaction 造成其关键字段缺失时，submission 必须 fail-closed；
不能为了 stable-prefix 命中压缩 exact current Definition partition、current failure、target snapshot 或 coverage target。
低优先级内容可以 bounded compress/ref-only，但必须在 audit 中记录 disposition。

## Owner Boundary

- Orchestrator 封存 request/session/context authority，并校验 result identity；它不解释 provider 私有 cache control。
- LLM owner解释 provider/model/config ceiling并执行 transaction；它不选择 Runtime action、candidate correction 或 evidence credit。
- provider adapter只能设置 provider 私有 cache key/control 和 wire mapping；动态 provider/model/baseURL 进入 audit/tail，不进入 stable prefix。
- request-log 布局、raw ref 命名、config loader 与 provider capability table 属于实现层，不是 core authority。
