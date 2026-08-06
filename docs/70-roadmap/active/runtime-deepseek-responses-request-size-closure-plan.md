# Runtime DeepSeek Responses 请求规模与超时收口方案

Status: active

Type: implementation closure plan

Last Updated: 2026-08-06

## 1. 核对结论

live interaction 已确认：

```text
input              ≈ 19,322 tokens / ≈ 80 KB HTTP body
output             ≈ 29,413 tokens
total              ≈ 48,735 tokens
response headers   ≈ 257 ms
total duration     ≈ 230 s
body generation   ≈ 230 s
stream            false
```

当前请求已经不携带 `text.format`，因此远端 JSON Schema response formatting 已关闭。剩余瓶颈是一次
semantic-plan 同时覆盖 47 个 CWI，并允许模型按 `max_output_tokens=65,536` 生成完整文档。

本计划只收口请求规模、阶段预算、流式传输和 provider 错误边界，不改变四个状态机的 owner：

| 状态机 | Owner | 本计划允许的修改 |
| --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | 不修改状态迁移；只消费 typed outcome |
| SM-2 authority/focus | `kernel/orchestrator` | 仅由内部事件选择 focus/batch、记录 continuation/retry |
| SM-3 candidate transaction | `kernel/orchestrator` | 仅维护同一 request session 的 fragment/complete-document 状态 |
| SM-4 drive/recovery | `kernel/orchestrator` | 仅维护 provider timeout、body-idle、continuation 的恢复事实 |

## 2. 组件收口清单

### RDS-LLM：`internal/modules/blocks/llm`

Owner：LLM capability block。

必须完成：

1. ai-proxy 生产能力固定为 `Responses=true`、`JSONSchemaResponse=false`。
2. `/v1/responses` adapter 不发送 `text.format`、tools 或未声明的 provider 扩展字段。
3. 支持 typed 阶段输出预算；`max_output_tokens` 必须来自 sealed request ceiling 的阶段预算，不能直接等于模型目录最大值。
4. 对 `stream=true` 使用现有 `StreamComplete`，完整文档落盘前不得返回 candidate。
5. 非 2xx、body-idle timeout、transport timeout、空响应和协议解码失败必须分别形成 typed provider outcome。
6. 保留 `RequestBytes`、`ResponseBytes`、`ActualTokens`、`EffectiveMaxOutputTokens` 审计字段。

禁止：

- 在 LLM block 内选择 CWI、focus、retry、terminal 或 governance 状态；
- 在收到失败后自动切换 Chat Completions；
- 在 `pkg` 中实现 prompt 压缩或 Runtime 状态迁移。

验收：

- DeepSeek request 没有 `text.format`；
- `max_output_tokens` 不超过封存阶段预算；
- 流式半截 JSON 不会被当作 candidate；
- provider 502/timeout 不会被误记为成功或永久 transient。

### RDS-ORCH：`internal/modules/kernel/orchestrator/biz`

Owner：SM-2/SM-3/SM-4。

必须完成：

1. semantic-plan 按当前 sealed focus/CWI 批次请求，不再一次要求覆盖全 workspace 的全部 CWI。
2. batch identity、coverage item set、manifest hash 和 logical request ID 持久化在 Orchestrator aggregate。
3. 单次输出达到阶段预算时，通过同一 request session 的 continuation 获取后续片段；不得重新发送完整 C0-C5。
4. continuation 失败、断流、超时或 JSON 不完整时，记录 durable outcome，并由 SM-4 有界恢复。
5. attempt-1 只能携带原始 rejection 和已封存 CorrectionDirective；不得扩权或改变 operation shape。
6. batch 完成后才交给 SM-3 evaluator；SM-2 仍通过 private internal event 封存 binding。

禁止：

- LLM owner 直接推进 SM-2/SM-3/SM-4；
- 通过 map/JSON authority bridge 重建 focus 或 CWI；
- 用“减少 CWI 数量”绕过未覆盖项；未处理批次必须持久化为 pending。

验收：

- 47 个 CWI 不会出现在同一单次输出要求中；
- resume 从当前 batch 继续，restart 创建 workspace_full 新 batch；
- crash/restart 后能从 request session 和 fragment receipt 继续；
- batch 间 stable prefix hash 和 definition revision 可审计。

### RDS-CONFIG：`internal/pkg/workorchconfig`

职责：声明式配置和规范化，不拥有 Runtime 状态。

必须完成：

1. 增加 semantic-plan、candidate、continuation 的阶段级输出预算配置，或提供等价的模型级 override。
2. 配置校验保证：
   - 阶段输出预算 `> 0`；
   - 不超过 sealed provider capability；
   - provider timeout 大于 ai-proxy body-idle timeout；
   - provider timeout 小于 Runtime 总体重试/恢复预算。
3. 默认配置不得使用 live 实例当前的 `1800s` provider timeout。

禁止：

- 通过固定全局 `MaxRequestBytes` 替代模型能力；
- 在配置层推导 CWI、target 或 operation；
- 为 embeddings 模型要求 chat token 能力。

### RDS-APP：`internal/modules/application/workorchd`

仅需完成配置边界：

- 配置读取、保存和 projection 必须展示实际 provider timeout 与阶段预算；
- API key 保存后必须明确要求重启服务才能影响 provider client；
- 不在 HTTP handler 中修改 Runtime aggregate 或选择下一步 action。

### RDS-WEB：`web/workorch-console`

仅在配置字段对用户可见时调整：

- 显示“保存 API key 后需重启 WorkOrch”；
- 显示 provider timeout、semantic-plan budget 和 continuation budget 的实际值；
- 不在前端构造 CWI、Schema 或 Runtime candidate。

### RDS-CAPABILITY：File/Git/Shell/MCP/Run/Result/Workspace/Eventstream

本次不修改业务逻辑。它们继续遵守：

- capability owner 只执行 typed operation/prerequisite 并返回 receipt；
- Run 只拥有 SM-1；
- Result/Workspace 只消费 terminal delivery fact；
- Eventstream/projection 只负责审计和展示。

## 3. 请求规模目标

目标不是让每次请求接近模型上限，而是让单次 semantic-plan 保持可预测：

```text
semantic-plan input：优先控制在 8K～16K tokens
semantic-plan output：优先控制在 8K～16K tokens
continuation fragment：4K～8K tokens
stream：优先 true
```

以上是阶段预算目标，不是 provider capability；最终值必须写入 sealed ceiling 并进入审计。

## 4. 超时顺序

必须满足：

```text
ai-proxy body-idle timeout
  < WorkOrch provider timeout
  < Runtime 总体 retry/recovery budget
```

推荐起始值：

```text
ai-proxy body-idle timeout：90s～120s
WorkOrch provider timeout：120s～180s
```

如果使用流式 Responses，body-idle timeout 只约束相邻响应块之间的空闲时间；不得把它当作完整请求总超时。

## 5. 验证矩阵

每项代码收口必须至少覆盖：

1. DeepSeek text-only non-stream request；
2. DeepSeek text-only stream request；
3. 大于阶段输出预算的 semantic-plan continuation；
4. 502 body-idle timeout；
5. 401/403 provider error；
6. malformed/incomplete JSON fragment；
7. crash before fragment persistence；
8. restart/resume 同一 request session；
9. restart 新建 workspace_full batch；
10. stable prefix hash 不漂移；
11. 全量测试、vet、fmt-check 和部署实例验证。

## 6. 完成判定

本计划完成必须同时满足：

- DeepSeek semantic-plan 不再一次生成 29K 以上输出；
- 单次请求在代理 body-idle timeout 内稳定获得首个响应块或明确错误；
- provider timeout 不再使用 1800s 这类无界等待配置；
- continuation、retry、batch 和 failure outcome 均可从持久化 aggregate 恢复；
- SM-1/SM-2/SM-3/SM-4 owner 边界没有被跨组件修改破坏；
- live run 至少完成一个 state-changing CWI，并产生 typed receipt/evidence 链。

## 7. 当前明确遗留项

1. LLM block 尚未执行阶段级 semantic-plan 输出预算；
2. Orchestrator 尚未将 47 个 CWI 拆为受控 batch；
3. 运行实例 provider timeout 为 `1800s`，与设计目标不符；
4. semantic-plan 仍在 prompt 中携带完整 schema 和 owner catalog；
5. semantic-plan 默认使用 `stream=false`；
6. live interaction 尚未形成 candidate/evidence，run-013 仍停留在 `semantic_plan_requested`。

