# LLM Capability Block

Status: active

Type: capability-block-contract

Last Verified: 2026-08-16

## 职责

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
3. catalog 精确模型容量是能力权威；exact override / custom explicit 是仅有的替代来源。
   OpenAI-compatible API surface 或 `/v1/models` 的 `/v1/responses` endpoint 声明不等同于
   `response_format` 支持；未显式声明 JSON Schema 能力时 adapter 必须省略该字段，并继续由
   sealed manifest、prompt 与 strict decoder 约束候选输出。
4. 能力不完整时 fail-fast，不使用 model pattern 或 conservative default。
5. `effectiveMaxOutputTokens` 直接采用 selected model 能力；`MaxInputTokens` 与 `MaxBytes` 由 context window、输出保留量、利用率和 token/byte 估算派生，不接受通用固定 output/request cap。
6. 已封存 `ProviderCeiling` 的 Run 不因 catalog 刷新漂移。
7. ai-proxy client API key 是可选配置；catalog、Responses、stream、embedding 仅在 key 非空时发送 Bearer header，空 key 不得编码成无效 `Authorization: Bearer `。
8. Runtime candidate request 只消费 Orchestrator sealed canonical state、authority transaction、manifest、C0～C5
   与可选 sealed correction；不从 action、raw reason、projection 或 provider response 重建 authority。
9. incomplete/transient/authentication/configuration/protocol/submission-unknown 等只返回本边界 factual category 与
   safe original diagnostic；LLM Block 不创建 candidate attempt，也不选择 correction、retry、governance 或 terminal。
10. complete document 只返回 strict decoded candidate/diagnostic。audit-safe original diagnostic 与 provider-safe
    field-violation projection 分开生成，任一转换都不得覆盖另一份；raw provider body/prompt/credential/endpoint
    不进入 Runtime aggregate、eventstream、projection、memory 或后续 request。
11. `Start()` 只读取一次 LLM configuration，并冻结为本 owner 的 active snapshot。配置文件 watcher 与 Web 保存
    只能改变下次进程启动使用的 persisted configuration；它们不得改变运行中的 catalog、Responses、embedding、
    provider ceiling 或 sealed-ceiling revision 校验。application 通过 typed active-configuration query 比较
    persisted revision，未匹配时投影 `restartRequired` 并拒绝 catalog refresh。

## EventHub 合同

- Provider/configuration：`/llm/command/complete-provider`、`/llm/command/generate-embedding`、
  `/llm/command/get-active-configuration`、`/llm/command/get-provider-model-catalog`、
  `/llm/command/refresh-provider-model-catalog`、`/llm/command/validate-inference-selection`。
- Runtime prerequisite/request：`/llm/runtime/acquire-provider-ceiling`、
  `/llm/runtime-prerequisite/acquire-capabilities`、`/llm/runtime-candidate/preflight`、
  `/llm/runtime-candidate/request`。
- 审计通知：`/run/llm_usage/recorded`。这些 topic、Command、Result/Data 都由 LLM owner 的
  `pkg/events` 定义；Orchestrator 只作为调用方消费 factual result。

## 非职责

- 不拥有 Run / Result / Evidence / Workspace 业务状态
- 不复制 ai-proxy 上游路由与 fallback
- 不在 Runtime reducer 中解释模型品牌或预算算法
- 不拥有 candidate evaluator、attempt/correction policy、Runtime action 或 terminal disposition
