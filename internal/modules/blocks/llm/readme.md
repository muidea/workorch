# blocks/llm

Status: active

Type: module-readme

Last Updated: 2026-07-17

## 职责

`blocks/llm` 是 WorkOrch 的 LLM 能力 block，独占：

- ai-proxy `/v1/models` catalog 访问、TTL/LKG cache 与并发 refresh merge
- `ProviderModelTarget` 精确解析与上下文预算推导
- provider client 解析（内置 `ai-proxy` + 高级 custom providers）
- completion / embedding / Runtime V2 candidate 请求
- Orchestrator provider ceiling prerequisite 封存

application、Web 与 orchestrator 不直接访问 ai-proxy。

## 生产语义

1. 内置首选 provider ID 固定为 `ai-proxy`。
2. activation 与 Runtime V2 使用完整 `ProviderModelTarget{ProviderID, ModelID}`。
3. catalog 精确模型容量是能力权威；exact override / custom explicit 是仅有的替代来源。
   OpenAI-compatible API surface 或 `/v1/models` 的 `chat_completions` 声明不等同于
   `response_format` 支持；未显式声明 JSON Schema 能力时 adapter 必须省略该字段，并继续由
   sealed manifest、prompt 与 strict decoder 约束候选输出。
4. 能力不完整时 fail-fast，不使用 model pattern 或 conservative default。
5. `effectiveMaxOutputTokens` 直接采用 selected model 能力；`MaxInputTokens` 与 `MaxBytes` 由 context window、输出保留量、利用率和 token/byte 估算派生，不接受通用固定 output/request cap。
6. 已封存 `ProviderCeiling` 的 Run 不因 catalog 刷新漂移。
7. ai-proxy client API key 是可选配置；catalog、chat、stream、embedding 仅在 key 非空时发送 Bearer header，空 key 不得编码成无效 `Authorization: Bearer `。

## EventHub 合同

- `/llm/command/complete-provider`
- `/llm/command/generate-embedding`
- `/llm/command/get-provider-model-catalog`
- `/llm/command/refresh-provider-model-catalog`
- `/llm/command/resolve-provider-model-target`
- Orchestrator prerequisite: `/orchestrator/prerequisite/acquire-provider-ceiling`

## 非职责

- 不拥有 Run / Result / Evidence / Workspace 业务状态
- 不复制 ai-proxy 上游路由与 fallback
- 不在 Runtime reducer 中解释模型品牌或预算算法
