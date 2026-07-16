# ai-proxy 首选 LLM Provider live 验证记录

日期：2026-07-15

结论：通过。

验证脚本 [verify-ai-proxy-primary-llm-provider-live.sh](../../scripts/verify-ai-proxy-primary-llm-provider-live.sh)
在同一轮中启动真实 ai-proxy（提交 `e40a632`）、真实 WorkOrch 服务和本地确定性上游。确定性上游只模拟
模型响应；catalog、路由、operation enforcement 和 WorkOrch Runtime V2 主链均为真实进程。

artifact：`docs/60-verification/artifacts/ai-proxy-primary-llm-live-runtime-20260715T150944Z/`

已断言：

- ai-proxy `/v1/models` 及 WorkOrch catalog get/refresh 返回 chat 与 embedding 的 operation/capacity。
- 真实 ai-proxy 转发 chat 和 embedding；embedding-only 模型请求 chat 被拒绝。
- 两个 Runtime V2 Run 分别封存 `live-chat-a`（65,536 tokens）和 catalog 变更后的 `live-chat-b`（196,608 tokens）。
- Run A 的两个 candidate request 都有 `providerSubmitted`、target/capability/configuration revision、budget、
  provider-native usage 和稳定的 `stablePrefixHash`。
- ai-proxy catalog 变更并 refresh 后，Run A 的已持久化 audit ceiling 未变化；Run B 使用新 ceiling。
- WorkOrch 重启后，Run A 完成态、两个 `/run/llm_usage/recorded` 事件及 audit/read-model 字段保持一致。

同时通过：`make test`、`make fmt-check`、`make vet`、`make verify-boundary-contract`、
`npm --prefix web/workorch-console test --if-present`、`npm --prefix web/workorch-console run build`。
