# LLM Capability Block

Status: active

Type: capability-block-contract

Last Verified: 2026-08-20

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
   sealed manifest、prompt 与 strict decoder 约束候选输出。
4. 能力不完整时 fail-fast，不使用 model pattern 或 conservative default。
5. `effectiveMaxOutputTokens` 直接采用 selected model 能力；Responses 的 input/output/wire/SSE 上限由 context window、
   输出保留量、利用率和 token/byte 估算派生。Embedding 不使用 chat output ceiling，但必须从 catalog
   `contextWindowTokens` 派生 input/response byte guard；容量缺失时 fail-closed。不存在进程级固定 output/request cap，
   non-stream JSON、stream content/event/line/wire 与 embedding JSON 都禁止无界读取。
6. 已封存 `ProviderCeiling` 的 Run 不因 catalog 刷新漂移。
7. ai-proxy client API key 是可选配置；catalog、Responses、stream、embedding 仅在 key 非空时发送 Bearer header，
   空 key 不得编码成无效 `Authorization: Bearer `。Responses 与 embedding 只允许同 origin redirect；scheme/host/port
   漂移必须在重发 prompt/input body 前以 typed protocol failure 拒绝。
8. Runtime candidate request 只消费 Orchestrator sealed canonical state、authority transaction、manifest、C0～C5
   与可选 sealed correction；不从 action、raw reason、projection 或 provider response 重建 authority。
9. incomplete/transient/authentication/configuration/protocol/submission-unknown 等只返回本边界 factual category 与
   safe original diagnostic；LLM Block 不创建 candidate attempt，也不选择 correction、retry、governance 或 terminal。
   非法 2xx JSON、非法 SSE framing/event、响应超限与跨 origin redirect 属于 `provider_protocol`，不得消费
   Orchestrator transport recovery budget；显式 protocol class 在 feedback 转换中不得改写为 internal。
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
13. profile `RequiredOwnerFacts` 作为 immutable declaration 进入 C2 和 stable prefix；它不是 current availability。
    current selectable identity 只能来自同 owner 的 C3 catalog。semantic instruction 明确禁止模型发明或宣称 owner fact；
    assessment credit/replan 仍只属于 Orchestrator SM-2。
14. Skill 只从 Run owner 的 sealed activation 进入 volatile C3。每项携带 WorkOrch Registry 或 SkillHub typed
    provenance、original/truncated instruction identity 与 activation-time byte budget；LLM 不读取 Skill repository/catalog、
    不重新选择或裁剪 Skill，也不把 Skill material 移入 C0～C2。缺失/漂移的 provenance 或 budget 在 Runtime
    invariant/request 前置校验中 fail-closed。
15. completion、embedding、catalog refresh 与 Runtime candidate 都是同步强调用。handler 必须先确认非空
    `magicevent.Result`，`Post` 不得触发 catalog refresh 或任何物理 provider 请求。
16. 每次物理 completion、embedding 与 Runtime candidate 调用都发布一条 bounded `LLMUsageRecordedData`；preflight
    同样发布 `ProviderSubmitted=false` audit。request/response bytes 来自 adapter 实际 wire 计量，失败时不得用 schema
    长度替代。setup draft 可为 process scope，activation embedding 可为 workspace scope，Runtime 为 workspace/run scope；
    RunID 不得脱离 WorkspaceID。

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
- [x] 2xx 非法 JSON、非法 SSE event、stream/content/wire/embedding response 超限稳定归类为 protocol；feedback 保留
  explicit `provider_protocol`，Orchestrator 对该类走 request-terminal 而非 transport recovery。
- [x] Responses/embedding 跨 origin 307/308 在重发 body 前拒绝，空 API key 不发送 Authorization。
- [x] model-derived request/output/response guard、sealed ProviderCeiling、ExpectedDocument token equality 与 catalog
  refresh 后 sealed revision 不漂移。
- [x] C0～C2 stable prefix、C3～C5 volatile tail、未声明 JSON Schema 能力时省略 provider format，以及 strict decoder
  拒绝非法 complete document。

## 待完善测试点

- [ ] 补齐所有 transport SDK/HTTP error variant 的 raw body、prompt、credential、endpoint 非泄漏组合矩阵；当前覆盖
  status body 脱敏、provider-safe correction、usage payload 与主要 protocol failure。
- [ ] 增加超大 model capacity、长 SSE 和 embedding vector 的 allocation/CPU benchmark；现有 benchmark 主要覆盖
  Runtime candidate rendering。
- [ ] 在 deployed real-provider 验收中覆盖 submission-unknown、native usage/cache usage 和 context-too-large 的供应商差异；
  LLM owner 仍只报告 factual outcome，不自行选择 retry/correction/terminal。
