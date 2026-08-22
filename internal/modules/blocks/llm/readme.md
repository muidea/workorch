# LLM Capability Block

Status: active

Type: capability-block-contract

Last Verified: 2026-08-22

## 功能范围与边界

`blocks/llm` 是 WorkOrch 的 LLM 能力 block，独占：

- ai-proxy `/v1/models` catalog 访问、TTL/LKG cache 与并发 refresh merge
- `ProviderModelTarget` 精确解析与上下文预算推导
- provider client 解析（唯一的 `ai-proxy` OpenAI-compatible endpoint）
- Responses / embedding / Runtime candidate 请求
- Orchestrator provider ceiling prerequisite 封存
- provider/request factual result、audit-safe original diagnostic 与 provider-safe correction projection

application、Web 与 orchestrator 不直接访问 ai-proxy。

## 生产语义

1. 内置首选 provider ID 固定为 `ai-proxy`。
2. activation 与 Runtime 使用完整 `ProviderModelTarget{ProviderID, ModelID}`。
3. catalog 精确模型容量是能力权威；ai-proxy exact override 是 Responses 容量的唯一替代来源。
   OpenAI-compatible API surface 或 `/v1/models` 的 `/v1/responses` endpoint 声明不等同于
   `response_format` 支持；未显式声明 JSON Schema 能力时 adapter 必须省略该字段，并继续由
   sealed manifest、prompt 与 strict decoder 约束候选输出。catalog cache 只保存目录事实；`fresh | stale |
   unavailable` 是每次读取时纯派生的 readiness observation，不写回 cache、不提供 transition API，也不形成 LLM
   owner 内的第二个 lifecycle。
4. 能力不完整时 fail-fast，不使用 model pattern 或 conservative default。
5. `effectiveMaxOutputTokens` 直接采用 selected model 能力；Responses 的 input/output/wire/SSE 上限由 context window、
   输出保留量、利用率和 token/byte 估算派生。Embedding 不使用 chat output ceiling，但必须从 catalog
   `contextWindowTokens` 派生 input/response byte guard；容量缺失时 fail-closed。不存在进程级固定 output/request cap，
   non-stream JSON、stream content/event/line/wire 与 embedding JSON 都禁止无界读取。non-stream wire ceiling 只计算
   bounded JSON envelope 与最坏 JSON escaping；stream wire ceiling 独立计算累计 decoded content、bounded data-event
   count × event envelope、单行上限与 bounded non-data framing，并为 terminal response 可能携带的第二份 output 留出
   一条完整 line。non-stream decoded output 必须在 `TrimSpace` 等交付规范化之前按原始 bytes 检查同一 content ceiling，
   包括首尾与分段空白；所有乘加都必须安全防溢出，不能以整数回绕放宽 guard。
   SSE reader 必须先确认最后一行已完整结束，再解码该 data event；bounded reader 在 partial line
   后返回 wire-limit error 时，`response_byte_limit_exceeded` 必须先于 partial JSON protocol error。
6. 已封存 `ProviderCeiling` 的 Run 不因 catalog 刷新漂移。
7. ai-proxy client API key 是可选配置；catalog、Responses、stream、embedding 仅在 key 非空时发送 Bearer header，
   空 key 不得编码成无效 `Authorization: Bearer `。Responses 与 embedding 只允许同 origin redirect；scheme/host/port
   漂移必须在重发 prompt/input body 前以 typed protocol failure 拒绝，且最多 10 次 physical submission。注入 client 的
   redirect callback 只能收窄策略，不能绕过 owner origin/hop guard。2xx Responses/embedding 的实际 `Content-Type`
   必须与本次声明模式一致：non-stream/embedding 接受 JSON media type，stream 只接受 `text/event-stream`；mismatch
   只解析/处置当前物理响应并返回 typed protocol failure，禁止切换 parser 后发起第二次 provider call。
8. Runtime candidate request 只消费 Orchestrator sealed canonical state、authority transaction、manifest、C0～C5
   与可选 sealed correction；不从 action、raw reason、projection 或 provider response 重建 authority。
9. incomplete/transient/authentication/configuration/protocol/submission-unknown 等只返回本边界 factual category 与
   safe original diagnostic；LLM Block 不创建 candidate attempt，也不选择 correction、retry、governance 或 terminal。
   provider request 的 closed category/class vocabulary 与穷尽映射只在 LLM `pkg/events` 维护；Orchestrator 只验证
   sealed workspace/run/authority scope，不复制第二套 provider mapping。
   非法 2xx JSON、非法 SSE framing/event、响应超限与跨 origin redirect 属于 `provider_protocol`，不得消费
   Orchestrator transport recovery budget；显式 protocol class 在 feedback 转换中不得改写为 internal。`http.Client.Do`
   返回的 endpoint-bearing `url.Error` 或自定义 transport error 必须先收敛为稳定的 timeout/cancel/DNS/TLS/network code，
   原始 error 文本不得离开 provider adapter。adapter-owned typed error 必须通过 audit-safe identity 保留精确 code
   （例如 `response_byte_limit_exceeded`、transport code 或 safe HTTP status identity）；Runtime boundary 不得从
   `Error()` 文本重新猜测该 identity，也不得以 generic `provider_request_failed` 覆盖它。
10. complete document 只返回 strict decoded candidate/diagnostic。audit-safe original diagnostic 与 provider-safe
    field-violation projection 分开生成，任一转换都不得覆盖另一份；raw provider body/prompt/credential/endpoint
    不进入 Runtime aggregate、eventstream、projection、memory 或后续 request。
11. `Start()` 只读取一次 LLM configuration，并冻结为本 owner 的 active snapshot。配置文件 watcher 与 Web 保存
    只能改变下次进程启动使用的 persisted configuration；它们不得改变运行中的 catalog、Responses、embedding、
    provider ceiling 或 sealed-ceiling revision 校验。typed active-configuration result 同时返回 revision、DefaultChat
    与可选 DefaultEmbedding；application 的 setup draft、fresh activation 和 semantic recall 都只能消费该 snapshot。
    completion 的空 Target 精确表示 owner active DefaultChat，不允许 application 重读热配置补值。persisted revision
    未匹配时 application 投影 `restartRequired` 并拒绝 catalog refresh。
12. semantic C3 只渲染 owner catalog 的可选择 vocabulary：snapshot/revision、operation/variant、target/selector/
    constraint 和 `ready + direct` validation recipe identity。non-ready 或 auxiliary/not-declared validation surface、
    input schema/binding/prerequisite、完整 MCP capability effect metadata 与 Shell command/args 都不得进入 provider context；
    最终 renderer 必须再次按 readiness 与 proof applicability 取交集，不能把 audit fact 变成 model hint。
    coverage-order schema 要求 `proposals` 字段存在且恒为 `[]`，provider instruction 必须逐字保持同一合同，禁止用
    “不返回 proposals”制造 schema/prompt 冲突；结构错误只返回 factual diagnostic，由 Orchestrator SM-3 决定一次有界纠错。
13. profile `RequiredOwnerFacts` 作为 immutable declaration 进入 C2 和 stable prefix；它不是 current availability。
    current selectable identity 只能来自同 owner 的 C3 catalog。semantic instruction 明确禁止模型发明或宣称 owner fact；
    assessment credit/replan 仍只属于 Orchestrator SM-2。
14. Skill 只从 Run owner 的 sealed activation 进入 volatile C3。每项携带 WorkOrch Registry 或 SkillHub typed
    provenance、original/truncated instruction identity 与 activation-time byte budget；LLM 不读取 Skill repository/catalog、
    不重新选择或裁剪 Skill，也不把 Skill material 移入 C0～C2。缺失/漂移的 provenance 或 budget 在 Runtime
    invariant/request 前置校验中 fail-closed。
15. Soul guidance 同样只消费 Run owner 封存的 stage snapshot，并要求 owner-issued revision、content hash 与 source
    provenance 完整；LLM 不回查 Soul catalog，也不能用同 ID 的较新内容改写既有 Run 的 C3。planning 与
    execution/validation/decision 继续按 sealed stage binding 隔离。
16. completion、embedding、catalog refresh 与 Runtime candidate 都是同步强调用。handler 必须先确认非空
    `magicevent.Result`，`Post` 不得触发 catalog refresh 或任何物理 provider 请求。
17. 每次物理 completion、embedding 与 Runtime candidate 调用都发布一条 bounded `LLMUsageRecordedData`；preflight
    同样发布 `ProviderSubmitted=false` audit。request/response bytes 来自 adapter 实际 wire 计量，失败时不得用 schema
    长度替代。setup draft 可为 process scope，activation embedding 可为 workspace scope，Runtime 为 workspace/run scope；
    RunID 不得脱离 WorkspaceID。
18. `AcquireProviderCeilingCommand` 的timeout/temporary error只是一项owner factual failure；LLM不自行retry或创建wait。
    Orchestrator若封存bounded recoverable wait，后续probe仍必须使用相同workspace/run/authority、selection、minimum bytes与
    canonical request hash。matching result返回detached ceiling fact；stale selection/revision或非typed diagnostic不能解除wait。
19. Runtime provider result携带的typed usage只确认LLM owner实际观察到的physical call/token消费；无可靠usage时保持
    unknown，LLM、audit、billing或metrics均不能回写Orchestrator execution-budget allocation。provider返回的负数usage、
    无法安全派生的total或整数溢出属于typed provider protocol failure；不得向调用方暴露该usage或同一响应的payload/vector。

## EventHub 合同

- Provider/configuration：`/llm/command/complete-provider`、`/llm/command/generate-embedding`、
  `/llm/command/get-active-configuration`、`/llm/command/get-provider-model-catalog`、
  `/llm/command/refresh-provider-model-catalog`、`/llm/command/validate-inference-selection`。
- Runtime prerequisite/request：`/llm/runtime/acquire-provider-ceiling`、
  `/llm/runtime-prerequisite/acquire-capabilities`、`/llm/runtime-candidate/preflight`、
  `/llm/runtime-candidate/request`。
- 审计通知：`/run/llm_usage/recorded`。这些 topic、Command、Result/Data 都由 LLM owner 的
  `pkg/events` 定义；Orchestrator 只作为调用方消费 factual result。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → LLM | active configuration/default targets、catalog、refresh、selection validation、completion/embedding | 完整；setup/activation/embedding 均使用 owner 启动快照 |
| `kernel/orchestrator` | Orchestrator → LLM | provider ceiling、runtime capabilities/preflight/candidate request | 完整；LLM 不创建 attempt 或 authority |
| Skill Registry / SkillHub Catalog | 无直接 I/O | Run activation 中已封存的 bounded Skill C3 snapshot/budget | 完整；LLM 不查询或解释 Registry/Catalog |
| ai-proxy provider | LLM-private adapter | models、Responses、embedding | 完整；同 origin redirect、capacity-derived response guard |
| `blocks/eventstream` | LLM → Eventstream | process/workspace/run scoped typed LLM usage notification | 完整；不含 raw prompt/body/credential，audit 不参与 authority |

## 非职责

- 不拥有 Run / Result / Evidence / Workspace 业务状态
- 不复制 ai-proxy 上游路由与 fallback
- 不在 Runtime reducer 中解释模型品牌或预算算法
- 不拥有 candidate evaluator、attempt/correction policy、Runtime action 或 terminal disposition

## 已覆盖回归

- [x] owner `Start()` 配置冻结、persisted revision 漂移、`restartRequired`/refresh 拒绝，以及真实
  Setup Draft、fresh activation、DefaultEmbedding 不读取热配置。
- [x] completion/embedding/candidate 的 Post 零物理调用；setup completion、workspace embedding 与 Runtime/preflight
  audit 均覆盖 typed scope、submitted flag、usage 及实际 request/response bytes。
- [x] 2xx 非法 JSON、非法 SSE event、media type/mode mismatch、stream/content/wire/embedding response 超限稳定归类为 protocol；feedback 保留
  explicit `provider_protocol` 与 adapter 精确 code，Orchestrator 对该类走 request-terminal 而非 transport recovery，
  fresh reload 后仍可从 terminal failure fact 读取同一 original diagnostic。
- [x] non-stream 与 stream 的独立 wire ceiling 均覆盖 limit+1；合法高 event-overhead SSE 不再误用 non-stream ceiling；
  SSE partial final data line 与 wire byte limit 同时命中时，稳定保留 `response_byte_limit_exceeded`，不被提前 JSON decode 改写。
- [x] stream/non-stream `Content-Type` 与请求声明模式交叉 mismatch、两种 wire guard failure 都确认只有一次物理
  provider call，不存在 adapter fallback/replay。
- [x] Responses/stream/embedding 跨 origin 307/308 在重发 body 前拒绝；注入 redirect callback 的同源循环仍由 owner
  hop limit 有界终止，空 API key 不发送 Authorization。
- [x] model-derived request/output/response guard、sealed ProviderCeiling、ExpectedDocument token equality 与 catalog
  refresh 后 sealed revision 不漂移。
- [x] provider-ceiling transient failure的exact recoverable-wait probe、普通drive/伪造lease零额外调用与matching result恢复。
- [x] C0～C2 stable prefix、C3～C5 volatile tail、未声明 JSON Schema 能力时省略 provider format，以及 strict decoder
  拒绝非法 complete document。
- [x] completion、stream、embedding 的 deadline/cancel/DNS/TLS/generic transport，以及 400/401/429/503、malformed 2xx
  矩阵均确认不泄漏 raw body、prompt、credential、endpoint 或 transport detail；redirect protocol identity 保持 typed。
- [x] completion/stream/embedding usage 的负数与派生 total 溢出均 fail-closed 为 typed protocol fact，结果不携带
  rejected usage/payload/vector，且每个边界只发生一次 physical call。
- [x] 1M-token capacity limit arithmetic、4096-event SSE 与 8192-dimension embedding decode benchmark 已建立；基准机
  分别为 0 alloc、约 9.3ms/2.1MB、约 1.6ms/325KB，用于后续性能漂移比较，不改变模型派生 hard guard。

## Deployed verification

- 真实 provider 的 submission-unknown、native usage/cache usage 与 context-too-large 差异只能在部署验收中观察；仓库内
  HTTP adapter 已覆盖对应 typed factual contract 与模拟响应，真实凭证/endpoint 不进入 CI。该项是环境验收，不是当前
  code-closure 缺口；LLM owner 在任何供应商差异下仍不得自行选择 retry/correction/terminal。
