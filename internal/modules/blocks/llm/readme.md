# LLM Capability Block

Status: active

v3 composition 的 versioned terminal-material rule/body/source selector 仍只按 exact subject 投影到 C3。
instruction 区分未来义务映射与真实 owner facts，明确普通 operation receipt 不证明最终 repository HEAD/tree/blob bytes。
`runtime_proof_composition_test.go` 固定完整声明／版本、输入漂移时 C0～C2 与 stable-prefix 不变及删声明拒绝。

Type: capability-block-contract

Last Verified: 2026-09-10

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
| `application/workorchd` | Application → LLM | `CompleteProviderCommand/CompleteProviderResult`、`CompleteProviderDocumentCommand/Result`、`GenerateEmbeddingCommand/GenerateEmbeddingResult`、active config/catalog/selection commands | generic completion 仅接受 exact `application/workorchd/appservice`；独立文档及其余 command 仅接受 exact `application/workorchd`，均在读取 payload/provider 前校验 |
| `kernel/orchestrator` | Orchestrator → LLM | capabilities、provider ceiling、candidate preflight、`RequestRuntimeCandidateCommand/Result` | 所有 Runtime topic 仅接受 exact `kernel/orchestrator` source |
| Eventstream | LLM → Eventstream | `LLMUsageRecordedData` | provider execution/preflight 的 bounded factual audit（包含未提交失败）；Eventstream 校验 exact LLM source |
| ai-proxy | LLM → HTTP provider | `/v1/models`、`/v1/responses`、`/v1/embeddings` | owner-private adapter；endpoint/credential 不进入跨 owner result |

[`pkg/events`](pkg/events) 为 completion 与 embedding 定义不同的 value result；不存在 nullable generic union。completion
以 `ValidateFor(command)` 绑定 exact target/model；embedding 绑定 exact target/provider/model/input，并要求 non-empty finite
vector 与真实 non-streaming physical response fact。owner 发布前和 Application 消费后使用同一校验。
`RequestRuntimeCandidateResult` 则是受校验的封闭事实联合，`Stale | Feedback | DocumentOutcome` 必须恰好一个，并绑定 exact
workspace/run、logical/physical request identity、document hash、audit 与 usage。

创建期独立文档调用保留 generic completion 的既有成功合同，复用同一 adapter 且不执行 fallback call。
`CompleteProviderDocumentResult` 的 `Document | Failure | Incomplete` 恰好一个，完整空文档仍由调用方严格解析；
exact opaque RequestIdentity、完整 accepted command hash、target、configuration revision、submitted 与 physical mode
由 `ValidateFor` 共同反证。LLM 不解释 Workspace phase 或 permit，不从审计重建调用结果。
文档、wire 与实际 transport leaf 分开记录 actual observed bytes／SHA256、complete、saved 和 reason；超过 256 KiB
不保存任何截断前缀，未读取不发明 bytes／hash。原始 leaf 只进入受限诊断，普通错误及 audit 保留安全分类，
不混入 request URL、配置或凭据。审计发布错误单独记录，不覆盖已经确定的 typed outcome；发布不等于持久投递。
Runtime 的原始 body／stream 禁止跨 owner 规则不变，此文档诊断合同只用于创建期 Workspace 受限查询。

### 生命周期合同

- 普通及 matched 订阅按每次成功获取登记；部分 Start 失败保留冻结配置、provider 和订阅账本，未清理前不能重复注册。退订成功才移除对应项。
- BeginShutdown 非等待式关闭新请求；冻结 active configuration 与独立 cache-only 准备观察允许只读 drain。普通 catalog、selection、ceiling 可能触发 refresh，不能因名字为查询就放行；candidate/preflight 和新 provider 请求同样拒绝。
- handler 票覆盖真实 provider/catalog 返回及同一 handler 的审计发布，不以调用方取消或 HTTP 关闭证明远端未执行。Quiesce 超时保留配置、physical submission guard 和依赖，可用新预算重试。
- 最终 Stop 封全部入站并再次排空，逆序 checked 退订全部成功后才清 owner/Module 引用。未知 panic 保留原因并释放清理 claim；调用方串行 Configure/Start/Stop，不能覆盖未释放 owner。
- 本地排空不替代共享任务与 Hub 屏障，不更改 SM-1～SM-4、提交身份、预算/unknown、原始诊断或 C0～C2/cache 合同。`biz/owner_lifecycle_test.go` 和 `module_lifecycle_test.go` 覆盖订阅切点、失败重试、真实 mock transport 返回及 framework guard。

### 激活前 catalog cache-only 观察

`/llm/startup-preparation/catalog-cache/query` 使用 `ObserveProviderCatalogCacheCommand/Result`，
仅接受 exact `application/workorchd` source。它读取本 owner 已有缓存，不调用普通 catalog、
selection 或 ceiling，不等待 inflight refresh，不联网、不重试、不 fallback、不淘汰或改写缓存。
miss 返回 `unavailable/cache_miss`；精确过期点及超出 LKG 的已有条目返回
`stale/cache_expired`，保留完整模型声明、revision、原获取/过期时间。fresh 仅指观察时缓存未过期，
不是连通性、授权、provider ceiling 或 Workspace ready。

结果绑定 Workspace、观察时点和配置冻结生命周期内随机 `OwnerEpoch`；epoch 在 owner 释放后清除，
新 owner 不复用，也不由凭据派生。私有 endpoint、credential、cache key 和 credential-derived
configuration revision 不出此合同。完整结果最多 4096 个模型、1 MiB，字段与嵌套列表有界；
新 owner 结果另附可选的 `workorch-preparation-provider-configuration/v2`：本生命周期冻结的
DefaultChat 和完整排序的 exact model override 声明（仅模型 ID、容量与 operation，不含配置密钥、
endpoint 或其派生 hash）。override 最多 4096 项、每项最多 64 个 operation，且仍受完整结果 1 MiB 限制。
旧无该字段的历史仍可读；不会在 GET、恢复或重新解码时补出默认模型。结果 clone 深拷贝嵌套声明，
loader 后续变化和返回值修改不能改变 frozen source。该声明不证明预算、远端授权或连通性。
v2 的准备检查消费已冻结的本地配置，不要求 catalog 预热、独立网关授权回执或探测调用。
启动执行仍通过 LLM owner 校验精确模型选择并封存到 Runtime activation；后续 loop 消费该封存选择，
实际请求继续遵守原 ProviderCeiling、配置 revision、预算和失败记录合同。凭据与 endpoint 留在 LLM owner，
不复制到 Workspace 或运行上下文。v1 及无配置历史按原语义保留，不在读取时升级。
超限/损坏整体拒绝，不裁剪后声明完整。缓存里异常的 observation/诊断字段同样拒绝，不清洗后授信。
原 catalog/provider 诊断路径不变。embedding 的零 context/output 元数据原值保留为未知；精确 endpoint 与非负元数据
允许 `embeddingReady`，并消费 `workorch.embedding-transport/v1` 的独立 64 KiB 输入、512 KiB JSON 请求、4 MiB
响应限制，已知 context 估算只可收紧输入。该策略身份进入 capability revision 与 audit profile mode；它不证明模型
token 容量、远端授权或连通性。混合模型缺少 Responses 容量时全局 `capabilityReady` 仍为 false，不能借 Embedding
资格授予对话预算。原始输入不 trim、不截断，结果按原 command input 精确绑定。
Application 可把同一次完整观察传给 Workspace 原准备操作作为私有历史归档；该消费不改变
本 owner 的查询合同，也不增加 refresh/selection。归档后的 epoch 和 freshness 仅代表原观察时点，
不能在恢复、重启或 GET 时冒充当前 owner 事实，公开随机引用不授予 readiness。

同一 cache-only command 可带可选 exact `Target` 意图；缺省使用冻结 DefaultChat，不猜 catalog 首项。
fresh 结果另附可选 `workorch-preparation-provider-budget-observation/v1`，由本 owner 复用现有
`ResolveProviderModelTarget`，仅输入本次已观察缓存及冻结配置，计算 Responses 容量预览。
保留精确 target、context/output/input/byte 容量、tokens-per-byte 与 capability revision；
不输出 credential-derived configuration revision，也不生成 Runtime ProviderCeiling、请求 allocation 或 client。
解析不可用只返回无容量值的 `capacity_unavailable`，miss/expired 不补预算；结果仍受完整 1 MiB 边界。
结果与 command target（或冻结默认项）严格相等，clone 隔离。它不是远端授权、可调用性、实际 request
装配预算或 Run 物理额度的证明，不联网、不新增资源写入。旧无预算历史不在读取时补算。

验证入口：`biz/catalog_cache_observation_test.go`、`pkg/events/catalog_cache_test.go`、`pkg/events/preparation_provider_configuration_test.go`；覆盖
miss/fresh/TTL/LKG、未完 refresh、配置轮换、完整性/隔离、生命周期及零 HTTP，不替代真实模型验收。

## 不变量与非职责

- immutable source obligation inventory 随 Definition 在 semantic／operations 的 C1 原样渲染；当前 proof batch 的 C3 保留全部 source checks、原 Criterion／Declaration 与完整 required check 集合。V2 按每项源判据评审，不以共同 CWI 或另一义务的文本替代。post-render integrity 独立比对 C1 inventory 和 C3 requirement，缺失／改写在物理 provider 调用前拒绝；保护预算超界显式失败，不截断正文。`biz/runtime_source_obligation_projection_test.go` 固定 exact 投影、删 B／改正文拒绝、same-contract correction 的 C0～C3 不变及 owner generation 变化时 C0～C2 稳定；这些渲染与来源约束不证明自然语言语义或授予 credit。
- semantic 请求必须携带 Orchestrator 的只读 coverage progress：完整 sealed inventory 的验收计数、当前 manifest/
  batch CWI 的当前证据引用数与重开数，以及 scope/revision/hash。接收前校验完整绑定，C4 渲染后逐值保护，
  缺失/裁剪/篡改在 provider 前拒绝；64 KiB 上限与 provider ceiling 同时生效。LLM 不把记录数量解释为 credit，
  不维护第二份进度状态；进度变化不改 C0～C3。详见 [context assembly](../../../../docs/00-core/runtime-request-context-assembly.md#semantic-请求的只读覆盖进度)。
- semantic 请求的 typed `ExecutionHistory` 观察同一 Run 已提交的最近 16 条回执及省略数，包含成功工作和失败事实。
  原 bound CWI 只证明历史归属；owner accepted、durable changed 不等于当前验收。接收端拒绝缺少 scope/hash、错序、
  重复、外来 CWI 或超限，C4 渲染后以 `P3.semantic_execution_history` 逐值校验；不展开 raw summary/参数/输出，
  不改变 C0～C3、稳定前缀、Runtime 状态或权限。它不能替代下述专用修复来源，也不包括无回执的在途/unknown 工作。
  历史保留封存的 requested target/expected effect 与实际观察的区别；历史及专用修复反馈均保留窄 typed owner
  diagnostic（code/revision/hash、truncated/redacted、可选 validation diagnostic）。不传原文/私有 detail ref/维护权限，
  自报退出码不证明进程执行或验收通过；新增字段参与原 C4 逐值完整性检查，不改变其他分区。
- semantic 请求必须保留 typed `ExecutionBudget` 的 pre-submission Run 物理余额与原 CWI proof-batch 用量；它只观察
  Orchestrator 已有账本，reserved/unknown 不退额，null 附加 cap 不表示无限执行，也不表示全部策略预算。接收端检查
  scope/revision/hash、完整成员及余额等式；C4 渲染后 `P3.semantic_execution_budget` 逐值保护，32 KiB 与 provider ceiling
  同时约束，不改变 C0～C3/stable prefix。LLM 不预留额度、不选择状态、不凭快照授予工作或验收权限。
- semantic repair proof-batch 必须经 typed context 接收本批每个修复 CWI 的 exact causal execution feedback；
  接收边界校验 workspace/run、Definition/startup、manifest、CWI、原 declaration/failure/owner/operation 和内容 hash。
  仅在 C4 渲染，post-render integrity 比对完整值；普通 proof/coverage order 的这个专用字段不接受无关历史失败。validation check
  projection 与 operations 请求共用纯合同，不含 raw output/错误原文；缺失或截断显式表达，整组上限 64 KiB 并受
  provider ceiling 限制。LLM 不核发 repair authority 或 credit，不变更状态；详见
  [context assembly](../../../../docs/00-core/runtime-request-context-assembly.md#semantic-repair-的执行事实反馈)。
- `Configure` 缺 EventHub 立即失败；`Start` 冻结配置且重复调用不重复订阅，`Stop` 清除订阅、配置和 physical guard。
- provider/model 必须由 exact catalog/config fact 解析；capacity 不完整时 fail-closed，不按模型名猜测、不静默换 target。
- `JSONSchemaResponse` 只有显式 capability fact 为 true 才发送；未声明时依赖同一 sealed manifest、prompt 和 local strict
  decoder，不发第二次无-format fallback call。
- 通用 ProviderRequest 的可选 `SchemaFormat=json_schema` 明确表示 `Schema` 是 JSON Schema 正文，而不是版本名称；显式格式必须在物理调用前校验为合法对象，不能因能力关闭而忽略畸形合同。空格式保留既有 schema ID／内联 schema 兼容语义，不为旧 Runtime 请求回填新字段或改变身份。格式标记不是 provider capability，不授予第二次调用或本地准入权限。创建期 proposal／review 使用各自完整 closed schema；原生 format 仍只在能力支持时映射。
- semantic Definition gap 仍是非权威 provider document。`undeclared_prerequisite` 必须按 schema 输出 CWI-bound
  `prerequisiteClaims` 的 exact owner/fact/operation tuple；prompt 明确禁止把 C2 已声明 requirement 描述为 undeclared，包含 C3
  已显示该 fact selectable 或已有 `ownerFactAcquisition` 的情况。LLM block 不判断 claim 真伪、不建票；同一 strict decoder 与 Orchestrator pure
  manifest admission 在 JSONSchemaResponse on/off 下统一 fail closed。
- operations renderer 只消费 sealed manifest 纯编译的 `RuntimeCandidateWireContract`，schema、C3 output contract、
  instruction 和 local strict compiler 共用其 exact hash/rules。required 与 auxiliary operation 都只要求模型提供真实
  选择；required kind、唯一 reachable variant、所有 reachable authorization 共有的 singleton binding、required
  cardinality-one singleton target 和 fixed parameter 由 Runtime 本地派生，provider 回显必须拒绝；unreachable variant
  authorization 不得重新暴露 binding。
  `operations` 与每个 `parameters` 都是 JSON array；parameter 必须逐项编码为 `{name,<typed-value-field>}`，不能使用 object/map。
  required sequence 的 canonical layout 展示全部 required slots；auxiliary-only manifest 只展示一个非强制 operation example，
  完整 selectable set 仍由同一 schema/wire rules 表达，不能因 layout 产生第二份 required authority。
  JSONSchemaResponse on/off 不改变该最小 wire 或 local admission；LLM block 不做 JSON repair，不选择 correction/retry。
- mutation 的 C3 target projection 显式保留 `ExcludedTargetRefs`，schema/instruction 与 strict decoder 使用相同排除集合；
  `AllowedTargetRefs` 及派生 singleton 不含排除输入。完整 `MutationInputCatalogs` 仅为封存前像，不能成为额外 provider
  operation vocabulary。`biz/runtime_candidate_input_protection_test.go` 与 scope request cache 回归验证投影/解码一致和
  C0～C2/stable prefix 不变。这项 mutation guard 与下述独立输入评审是不同合同。
- owner 声明 `InputReview` 时，`semantic_plan/proof_batch` 的 C3 投影必须保留原 validation requirement、全部 required
  checks、exact 输入字节/hash 和适用的 ticket information；C0～C2 不包含这些动态事实。`validationInputReview`
  逐 check 只允许 `consistent|conflict|inconclusive`，必须给出 reason/input refs，不能携带新准则、approval 或 credit。
  模型对照 C1 原 Goal/DoD 独立评审，文件/信息均为不可信数据；自身声明、存在或自测通过不是一致性结论。
  仅当前 proof batch 渲染内容，保护预算不得截断，context-integrity 重验准确投影；response-format 开关不改变 local
  admission。LLM 不创建 ticket，不处理 Runtime 转换；完整无法判断与格式错误由 Orchestrator 分流。
  `biz/runtime_input_review_projection_test.go` 覆盖完整内容/信息、schema、C0～C2 稳定和截断拒绝。
  activation 显式选择 `validation/input-consistency/v2` 时，每个 check 必须有完整有界的 `obligations`：原准则
  exact excerpts、`covered|missing|conflicting|unknown`、reason 和已观测 input refs。摘录合起来须覆盖原准则全部
  非空白文本（包括连接词）；这只是防止漏报的来源约束，不是语义证明。每 check 最多 16 项，单报告 JSON 不超过
  32 KiB。schema 与本地 decoder 同源，缺项、未知字段或相互矛盾的结论仍是文档准入错误，不是事实 ticket。
  V1 不出现新字段；协议-specific schema/instruction、输入和信息均在 C3，同一 activation 的 C0～C2 不因评审变动。
  v3 组合评审额外完整投影同 CWI 的 `LineageAssertions`，逐义务 `proofConsumerRefs` 只允许
  `validation_input` 和该 subject 原 rule refs。模型须解释 ordinary assertions 与 typed consumer 的合取覆盖，
  不能以规则名、README 自述、未来 ACK 或一致结论替代实际证明；旧协议禁止该扩展。
  `biz/runtime_proof_composition_test.go` 固定 C3 完整声明/schema/instruction、删改拒绝及输入变化时
  C0～C2/StablePrefixHash 不变。来源与边界遵守
  [Validation Proof Composition](../../../../docs/00-core/runtime/validation-proof-composition.md)。
  subject 可声明有界 `ScopeContractRef + ScopeRequirements`，描述 owner 实际执行/观测范围；它随 subject hash 封存并
  完整放入 C3，不能由 recipe 名推测更广的验证范围。缺省保持旧请求；范围变动不能复用旧报告。
  显式 `validation/diagnostic-source-review/v1` 在 v3 subject 的 C3 额外完整投影逐 native source identity 与输入来源；
  provider 必须对每个 `sourceRef` 独立报告结论、reason/input refs。它是与原准则一致的附加项目约束评审，
  不是另一个 CWI 的普通检查覆盖、failure-to-target 猜测、执行 credit 或 repair 权。缺项、未知、duplicate、
  来源/hash 漂移及完整输入/报告超界均 fail-closed；schema 和 local decoder 共同验收，不影响 C0-C2。
  具体有限 envelope、共享原 CWI 信息预算及 owner 边界见组合证明合同；旧 subject/wire 不引入该扩展。
- create-only validation preparation 的可选 `ContentContractRef + ContentRequirements` 由 capability owner 提供并进入
  catalog hash；semantic requirement/acquisition 与 operations 的 C3 必须随 exact target/native mapping 原样投影。
  LLM 不按文件名补规则，不把生成输入视为一致性结论或执行 credit；空扩展不改变历史 request JSON/hash。
  `biz/runtime_preparation_content_test.go` 验证三个投影入口一致、完整且保留旧格式。
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
- generic completion 与 Runtime I/O 按协议模式限制 token/byte/wire/SSE/event/line；embedding 独立限制 input/request/response
  bytes，未知 token 容量不虚构上限。成功 result 不能是同类型空值或漂移
  target。raw request、response、credential、
  endpoint 和未脱敏 diagnostic 不得进入 aggregate 或 audit notification。
- provider-facing cache 分区以 [Runtime request/cache contract](../../../../docs/00-core/runtime-request-cache-note.md) 为准：
  C0～C2 是连续稳定 prefix，C3～C5 是 current/volatile tail；意外 `stablePrefixHash` 漂移即 rendering defect。
- operations typed command保留完整 candidate context供owner-local integrity/freshness校验；provider-facing C3只投影sealed
  manifest命中的target/stage supporting context，不展开raw operation-admissibility、prerequisite-observation或无关owner/MCP
  catalog，operation vocabulary只来自同一个sealed manifest。manifest引用的supporting snapshot缺失、漂移或owner/kind
  不匹配时必须在physical submission前返回unsubmitted integrity fact，不能静默投影为空。
- repair authority 在 C3 使用显式字段投影，不序列化完整 persisted directive 中的诊断与 operation 参数。C4 独立保留 exact
  current validation failure 的 receipt/check/case/location/digest、环境/exit/timeout facts 和 missing/truncated 标记；该
  receipt 不受 recent-history 四项窗口裁剪。raw logs、自由诊断与 expected/actual 内容不进入 provider。comparison count
  是只读事实，不授权模型选择 retry、next focus 或 terminal；最终 rendered request 回归必须证明 C0～C2 bytes/hash 不漂移。
- C4 只接纳校验通过的可选 `validation/check-failure/v2`、`comparisonHash` 与封闭 `reportedAssertion` 形态；报告是检查
  自报，不是独立验收事实。v2 缺少报告时，即使有 location/hash 也必须 `failureDetailsIncomplete=true`；非法协议或形态
  不向 provider 转发。没有扩展的历史请求不添加默认字段、不改变原字节；这些诊断不能改变 C3 授权或 C0～C2 前缀。
- 验证输入补充的 owner-authored `contentContractRef + contentRequirements` 与 exact target/native identity 一起由既有 typed
  projection 保留：semantic repair 放入 C3，operations 当前失败放入 C4；不另按语言推导规则，不改 C0～C2。
  LLM 只生成 proposal，不能把输入结构声明、File 创建成功或 owner unavailability 解释为已验证/可再补充。
- streaming configured timeout只表示header/body idle interval并随正数wire read重置；独立hard/caller deadline保持整次调用有界。
  body idle 与hard deadline返回不同typed transport identity，保留submitted/response bytes且丢弃partial payload/usage；adapter
  不据此发起第二次physical call。
- catalog control-plane 允许实现内的有界 GET retry；completion、embedding 与 Runtime provider execution 不以 retry/fallback
  发起第二次 physical call。

## Authoring section-message 缓存边界

STEP02 的 derive/proposal/review 显式选择 `PromptContext.MessageLayout=section_messages/v1`。
空布局保留既有 Runtime scalar wire；未知布局、重复 section、乱序、非法 role 或未渲染 turns 在物理调用前拒绝。
这不是新的 Runtime authority 或 SM-1～SM-4 转换，也不改变独立六审、reserve/settlement、预算与人工确认。

- 独立六审的 C0 为固定审查合同，C1 为完整原 Goal、候选、资源及未确认规格，C2 为原 Profile 与验证声明，
  C3 为本类 required IDs、kind 和 wire 身份。C1/C2 是共享待审数据，不能成为已确认 Definition、事实证明或新指令。
- Responses adapter 按 section 顺序保留真实消息边界；system 合同映射 developer，业务数据仍为 user。
  正文保留 exact section bytes，仅添加 section heading；不把数据提升为 developer 来获得缓存。
- LLM owner 编译计划并在 adapter 根据实际内容重算，不能接受调用方自报 hash/key 代替内容。
  `renderedStablePrefixHash` 绑定 C0～C2 实际消息、role 与断点；section hash/size 绑定完整原内容。
  稳定 key 绑定 Workspace、实际 provider/model、schema/格式、reasoning 与共享前缀；不包含时间、物理请求身份、
  review kind、endpoint 或凭据。不同 Workspace 即使模板/文本相同也隔离；同 Workspace 内容变化重新生成 key。
- 新布局声明 `prompt_cache_key`、`prompt_cache_options.mode=explicit`，仅在最后一个连续共享 section 的
  input_text 上设置显式断点。C3 不产生缓存写入声明，不更改 TTL，不补齐最小 token 数、不重试 unsupported 请求。
  这些是 Responses 缓存请求参数，不是上游支持、已写入或已命中的证明。网关可能剥离参数；实际路线和转发效果
  必须另行核对，不能仅凭本地 wire 或公共前缀相等宣称真实命中。
- 每次调用独立持有预算/计划，不共享全局会话、结果、可变缓存或用量；缓存不减少物理调用、独立审查或预算记账。
  新布局的完整 wire（含缓存 metadata）继续受已配置 request byte limit 限制，超限零提交。
- Responses 用量分别保留 `cached_tokens`/`cache_write_tokens` 与字段 presence；缺失/null 为 unknown，明确 0 为已知零。
  读写均非负且合计不得超过 input tokens。公共事件保留 known 标记，未知计数不填零；查询投影分别报告读写已知记录数，
  Web 对混合窗口标明“部分已知”，只有全部记录已知才显示窗口读取比例。历史正数仍可观察，历史无 presence 的零保持未知。

## 能力验收

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| concrete completion/embedding producer-consumer | [`pkg/events/messages_test.go`](pkg/events/messages_test.go)、[`biz/owner_provider_command_test.go`](biz/owner_provider_command_test.go)、[`application setupdraft`](../../application/workorchd/biz/appservice/setupdraft/generator_test.go)、[`activation embedding`](../../application/workorchd/biz/appservice/activationports/embedding_test.go) | normal、跨-topic wrong-result、同类型空值、target/physical/vector drift 覆盖 |
| Runtime result exhaustive union / audit identity | [`pkg/events/runtime_provider_test.go`](pkg/events/runtime_provider_test.go) | 覆盖 normal、invalid、tamper |
| Runtime exact source / duplicate / concurrent | [`biz/runtime_owner_guard_test.go`](biz/runtime_owner_guard_test.go) | 覆盖 |
| provider protocol、output bound、stream/non-stream、embedding | [`biz/http_provider_test.go`](biz/http_provider_test.go)、[`biz/http_provider_security_matrix_test.go`](biz/http_provider_security_matrix_test.go) | 本地覆盖 wire success/negative edges、stream idle/hard deadline、持续进展与 reasoning-only terminal；不证明真实 ai-proxy、credential 或 upstream 模型兼容性 |
| candidate framing、schema、gap claim、correction、cache hash | `biz/runtime_candidate_*_test.go`（含 `TestRequestCandidateClassifiesNonObjectProviderOutputsAsNoDocument`、`TestRuntimeProviderRequestPreservesStablePrefixAndTypedContext`、`TestRuntimeCandidateAuxiliarySingletonWireIsMinimalWithJSONSchemaResponseOnOrOff`、`TestRuntimeCandidateOutputProjectsExecutableValueRulesIntoC3AndSchema`、`TestRuntimeCandidateRetryProjectsOwnerConstraintOnlyInC4WithoutStablePrefixDrift`）、`biz/runtime_semantic_prompt_test.go`、`pkg/models/runtime_candidate_correction_test.go`（含 `TestRuntimeCandidateCorrectionInputAcceptsExactNullRequiredContainer`、`TestRuntimeCandidateCorrectionInputValidatesOperationValueRuleAgainstManifest`、`TestRuntimeCandidateManifestCorrectionProofUsesExactWireRuleAcrossOperationKinds`、owner constraint/operation-set manifest-binding matrix）、`biz/provider_prompt_projection_test.go` | 覆盖 no-candidate framing 零 content、required/auxiliary 最小 wire、multi-operation exact rule identity、typed undeclared-prerequisite claim instruction、JSONSchemaResponse on/off、missing/empty/null/wrong-type schema witness 与 semantic witness 分离、value-rule/owner constraint/operation-set C3 declaration与C4 bounded witness、failure/correction exact binding、manifest-scoped C3 与 C0～C5 identity |
| authoring cache / concurrent Workspace isolation | `biz/provider_section_messages_test.go`、`biz/provider_section_owner_test.go`、Application `specifications/authoring_cache_test.go`、`readmodelprojection/llm_cache_presence_test.go`、Web `llmUsage` tests | 完整两版输入、六审稳定前缀/变化尾部、role/原文保真、scope/target/content 漂移、4 Workspace × 6 请求并发、stream/non-stream、真实 owner 审计与未知/零/读写用量；不证明 deployed cache hit |
| active configuration freeze | `TestBizStartFreezesLLMConfigurationForCatalogAndStatus` | 覆盖 |
| catalog cache/key/concurrent refresh | [`biz/catalog_cache_test.go`](biz/catalog_cache_test.go) | 覆盖 merge、key、auth rotation、capability mapping，以及本地 HTTP transport 的 network/5xx 有界 retry、4xx no-retry、oversize/malformed body、same/cross-origin redirect、fresh→stale→expired LKG |
| lifecycle / dependency fail-fast | [`module_test.go`](module_test.go)、`TestLLMOwnerStartIsIdempotentAndTeardownClearsLifecycle`、`TestProviderCommandsPublishAuditAndRejectPostBeforePhysicalCall` | 覆盖重复 Start、Stop 清理，以及 Stop 后 completion/embedding/catalog 零 physical call |
| generic caller authorization | `TestLLMGenericCommandMatrixRequiresExactApplicationSourceBeforePayload` | 覆盖 6 个 generic topic 的 exact Application source matrix，并证明拒绝发生在 payload 读取前 |
| deployed real provider | [Service Acceptance](../../../../docs/50-product-interface/service-acceptance.md)、[Deployed Convergence DC-01/DC-02](../../../../docs/50-product-interface/deployed-convergence-scenarios.md) | 组件级未覆盖；真实 ai-proxy endpoint、credential、模型 catalog 与 provider behavior 仅由显式 real-provider deployed gate 验收，不能计入组件单测完成度 |

## Validation preparation context

Runtime operations and semantic-plan use `section_messages/v1`. The actual Responses prefix is C0-C2; dynamic C3-C5
does not enter the explicit prefix key. Provider/model/schema-family/format/reasoning and Workspace remain isolated. Runtime's sealed
stable-prefix identity is not replaced by the rendered wire hash; neither a cache key nor a hit is execution authority.

`workorch.section-cache/v2` derives the operations or semantic-plan schema family only from the owner's exact known C0
bytes and matching prompt version/layout. Unknown contracts keep full schema isolation. Current manifest/step/batch schema
bytes remain exact in strict provider output and local response admission; they do not split an otherwise identical Runtime
prefix. Actual provider JSON-schema capability, raw-text mode and explicit schema format remain isolated. No cache scope is
persisted authority or a cross-Workspace result cache. `TestRuntimeCacheNamespacePreservesExactDynamicSchemaAndAdmission`
and the streaming/non-streaming concurrent section-message fixtures cover this boundary; deployed cache hits require usage
and actual gateway/upstream observations.

Candidate preparation hints name the actual `context.validationProofPreparations` array and its per-member owner,
CWI, requirement, semantic check and native check. There is no Go/Shell instruction in that generic rule. Owner-selected
`validationGenerationContexts` project complete exact source observations once per matching surface, with explicit
unavailable/over-capacity status, and only for current scope. Original source-check criterion/declaration accompanies
the native mapping when declared. Both request integrity gates reject loss or modification of protected observations.
Source observations never authorize writing another target, skipping review or claiming acceptance.
