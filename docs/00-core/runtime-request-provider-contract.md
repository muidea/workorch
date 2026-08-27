# Runtime Request Provider Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-27

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

## Mode-Specific Provider Response Guard

response guard 只从已封存的 effective output-token ceiling 与 selected provider context profile 派生；它不是第二份
output-token authority，也不能由 adapter、Runtime mode 或 retry 放宽。completion 的模型内容上限保持一致，但 wire
envelope 必须按协议模式分别限界：

| Mode | Required media type | Independent bounded read |
| --- | --- | --- |
| non-stream completion | `application/json` 或 `+json` | JSON body ceiling = escaped content allowance + fixed response envelope |
| streaming completion | `text/event-stream` | SSE wire ceiling；同时独立限制 accumulated delta content、data-event count 与 single-line bytes |
| embedding | `application/json` 或 `+json` | 从 selected embedding input/profile byte capacity 派生的独立 JSON response ceiling |

SSE 不能复用 non-stream body ceiling：合法流可能同时包含增量输出和 terminal response 中的完整输出，并含有每个 event
的 envelope 与非 data framing。SSE ceiling 必须对这些有界开销显式计数，但 accumulated model content 仍受同一
completion content ceiling 约束。non-stream 也必须对 provider decoded 的原始 output bytes（含最终交付前会被
`TrimSpace` 移除的首尾/分段空白）先执行同一 content ceiling，不能以规范化后的较短 payload 绕过 guard。所有加法、
乘法和 token-to-byte 转换都必须 overflow-safe；capacity 不完整或溢出在 HTTP submission 前 fail closed。

media type 在读取 body 前校验。bounded reader 只多探测一个 byte 以区分“恰好到上限”和超限；超限时
`ResponseBytes = limit + 1` 表示 guard 已命中，不代表已读取或允许记录完整 provider body。SSE 的 wire-limit error 必须
优先于该边界处 partial line/event 的 JSON decode error。所有 byte、content、event、line、media-type 与 decode violation
都形成 typed provider-protocol fact；LLM owner保留原始 guard code/response-byte audit，Orchestrator不得从错误文本改写
其 factual identity。

Responses/stream/embedding 对任意 redirect 都在第二次网络提交前 fail-closed：一个 `PhysicalCallID` 恰好只授权
一次 physical submission；任何后续 provider 调用都必须由 Orchestrator 以新的 physical identity 和预算 reservation
明确授权。注入 client 的 `CheckRedirect` 不能放宽该 owner at-most-once guard；跨 origin redirect 仍保留精确 typed
protocol identity。

## Whole-Document Transaction

semantic-plan 与 operations 均要求 wire normalization 后的 canonical document 是恰好一个完整、紧凑 JSON object：
无 Markdown fence、解释、前后缀或重复输入；object `additionalProperties=false`，renderer、schema、strict decoder 与
evaluator 共用同一 vocabulary、shape 和 sealed limit。

provider-facing instruction 仍严格要求裸 JSON。当前 wire normalization 最多只可移除一层完整、独占响应的精确 JSON code fence；
raw bytes/hash 保留原始 identity，任何 prose、其他语言标签、缺失 closing fence、多 fence 或
多 JSON value 都不得由该 normalization 放宽。

- incomplete/EOF、output limit、timeout、429/5xx 与允许恢复的 transport failure 只进入 request-session recovery；
- 401/403、configuration/protocol failure、submission-unknown 和 document recovery exhausted 形成 typed request fact；
- LLM EventHub result 必须是 `Stale | Feedback | DocumentOutcome` exactly-one。complete `DocumentOutcome` 才能携带
  与 framing hash/bytes 精确匹配的 opaque document content；incomplete/empty/invalid outcome 禁止携带 content，旧
  顶层 content fallback 不存在；
- provider 返回 coherent `completed` terminal 但没有 final output 时，LLM owner 必须发布
  `empty_document/provider_final_output_missing`，不得降格为 provider-protocol `Feedback`，也不得把 reasoning text
  当作 document content。该 submitted transaction 的 provider-native usage 与 completed audit 必须原样保留，供
  Orchestrator 精确结算；empty document 是 terminal document fact，不授权 whole-document recovery；
- 每个 result 必须携带 LLM-owner sealed audit，并逐项匹配 workspace/run、logical request、physical call、document kind、
  target、stable prefix 与 context budget。`providerSubmitted` 只由 protocol adapter 在调用 physical client 的边界写入；
  `requestBytes` 只是序列化计量，不能推断 submission。`DocumentOutcome` 必须 submitted，stale 必须 unsubmitted，
  transient/authentication 必须 submitted；unsubmitted fact 禁止 response bytes、usage/actual tokens，并固定 non-streaming；
- Orchestrator 把 audit 归一为 owner-private `confirmed | released | unknown` settlement，并在同一次 aggregate CAS 中与
  request-session transition 一起结算全部 physical-call allocations。unsubmitted exact fact 才 release；submitted fact
  confirm physical call，缺失/不可信 fact 只能 unknown 并封存 submission-unknown，不能重放；
- recoverable incomplete/transient observation 必须在一次 Orchestrator CAS 中同时完成 settlement、SM-3 session推进、
  必要的 SM-4 transport budget 消费与耗尽时的 SM-2 failure。body-idle 使用同一 transient path；只有 confirmed
  submission 可恢复，released/unknown 均不得进入 retry；
- authentication/configuration/protocol/submission-unknown/document-recovery-exhausted 先归一为一个 Orchestrator owner-private
  terminal fact，再在同一次 aggregate CAS 中同时关闭 SM-3 request session 并应用其确定性的 SM-2 failure。CAS 写入前
  中断只留下 submitted marker，fresh reload 必须封存 submission-unknown 且零 provider replay；CAS 已写入但响应丢失时
  fresh reload 复用 exact terminal/failure，duplicate 零写入；
- request failure 不创建 candidate attempt；不得跨 physical call 拼接 partial output 或把 raw suffix 注入下一次 prompt；
- 每个 retry 使用新的 `PhysicalCallID`，但保持 logical request、ExpectedDocument、C0～C3、transaction、batch 与 stable prefix；
- complete document 只进入一个 evaluator；candidate attempt 0/1 与 physical-call recovery 是独立预算。
- provider call allocation、usage、unknown consumption和duplicate accounting遵守
  [Execution Budget](runtime/execution-budget.md)；physical retry不能重置或借用candidate/repair budget。

## Request Audit

每次 logical request 至少持久化：

- `LogicalRequestID`、每次 `PhysicalCallID`、recovery generation；
- document kind、expected-document hash、response hash/bytes、document disposition、`providerSubmitted`、
  `PhysicalResponseMode=streaming|non_streaming`；公共 audit/projection 不保存并行 `streaming` 布尔字段；
- `StablePrefixHash`、各 section hash、`WorkspaceDefinitionHash`、`CapabilityContractHash`；
- current focus/required-output bundle hash、volatile-tail hash、Definition revision/change impact；
- included、normalized、compressed、omitted、redacted sections；
- protected fields、integrity warnings 与 bounded source refs。

当前 canonical 字段 `stageBundleHash` 只表示 current focus/required-output bundle，不能恢复 StagePacket/NextAction。
raw request/response body、credential、endpoint 与未脱敏内容只记录允许的 hash、
bytes、disposition 和 redacted/omitted metadata。

LLM usage Eventstream/read projection 同样必须原样保留 `logicalRequestId`/`physicalCallId`（Runtime request）、
`providerSubmitted` 与 `physicalResponseMode`；Runtime usage 的两级 identity 必须同时非空且
`physicalCallId=requestId`。generic usage 与引入双层 identity 前的 persisted trace 可以两者都没有，但任何 consumer
不得从 `RequestID`、request bytes 或 streaming 隐含/补造 logical identity。

attempt 1 只允许改变 C4 中 provider-safe `CandidateRejection` 与 Orchestrator persisted `CorrectionDirective` 的 detached
`correctionInput` 投影。该投影只有 manifest、objectives 与 concrete witness；persisted `Disposition`/`TerminalCode` 不进入
LLM command/model/C4，LLM 也不能反构 directive。若修正需要改变 operation authorization、target policy、allowed
targets、manifest 或 C0～C3，则必须终止 current transaction，等待 SM-2 以新 owner fact 建立新 authority。

prospective target 的 required-owner typed `all_of` constraint 只进入 C3 authority；document/schema mismatch 的普通
attempt-1 correction 保持 C0～C3、manifest、semantic batch/order 与 stable prefix 不变，C4 只引用
manifest/CWI/违规 field，不复制 constraint。exact-current File+Shell full-composite `would_not_satisfy` 不生成 C4 或
replacement target：它表示 owner contract contradiction，并直接收口为
`prospective_satisfaction_contract_violated -> semantic_contract_failed`。typed stale 只刷新 owner fact。

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
