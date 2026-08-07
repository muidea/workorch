# Runtime DeepSeek Responses 请求规模与超时收口方案

Status: active

Type: implementation closure plan

Last Updated: 2026-08-07

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

## 3.1 CWI 计划排序（由 LLM 提出、由 Orchestrator 校验）

CWI 的业务执行顺序不能由 `Order`、目录顺序、map 遍历或其它数值字段机械推导。排序本身是
semantic-plan 的一个显式步骤：LLM 在 `coverageOrder` 中提出完整的 CWI 顺序，说明依赖和交付优先级的
组合判断；它不是 authority，也不能改变 manifest 中的 CWI 集合、owner、operation、target 或 recipe。

Orchestrator 只做合同校验：

1. `coverageOrder` 必须是 full manifest CWI 的恰好一次排列，不得遗漏、重复或引入未知 CWI。
2. `ReopenDependencies` 必须满足依赖项先于被依赖项；循环依赖仍由 Definition seed 校验拒绝。
3. 只有通过上述校验的顺序才可由 SM-2 private event 封存并用于 batch 切分；校验失败只能形成
   semantic-plan correction，不能直接 dispatch 或由 Orchestrator 自行重排。
4. order 的 hash 绑定 semantic-plan document/batch lineage，用于 stale/duplicate/restart 检测；不写入
   activation manifest，也不把 source order 当成执行权威。

`internal/pkg/workorchruntimecontract` 只保存 `coverageOrder` DTO、排列校验和 hash helper；不选择顺序。
focus、batch 和 correction 仍由 Orchestrator SM-2 内部事件负责。

## 3.2 Semantic-plan Batch 合同

Full manifest 仍然包含 workspace 全部 CWI；batch 只是对 full manifest 的有序、持久化子集，不是新的 authority。

### Batch DTO

```text
RuntimeSemanticPlanBatch {
  batchId
  manifestHash
  definitionSeedHash
  sequence
  coverageWorkItemIds
  pendingCoverageWorkItemIds
  requestLogicalId
  documentHash
  dependencyBarrierRefs
  state
}
```

Batch 状态固定为：

```text
pending → requesting → document_complete → feasibility_pending
         → feasibility_complete → accepted
```

失败状态为 `failed`；失败 batch 不得被静默跳过。

### Document 规则

`RuntimeSemanticPlanDocument` 增加 `batchId`、`batchSequence` 和 batch coverage identity。Provider 返回的
document 只需覆盖当前 batch，但必须满足：

- `manifestHash` 和 `definitionSeedHash` 与 full manifest 一致；
- proposal 只能属于当前 batch CWI；
- 当前 batch 每个 CWI 恰好一个 proposal；
- batch 内 owner、operation、target、recipe 仍由 full manifest 校验；
- batch document 未完成前不能请求 owner feasibility。

全部 batch 完成后，Orchestrator 在 owner-local reducer 中合并 document：

```text
ordered batch documents
→ reject duplicate CWI
→ verify sequence/order/dependency barriers
→ verify full manifest coverage
→ compute merged document hash
→ request complete feasibility set
→ SM-2 seal binding/reconciliation
```

在 merged document 和完整 feasibility set 形成前，禁止 dispatch、evidence credit 和 terminal。

### Batch 选择

初始建议：`semanticPlanBatchMaxItems = 4..8`。选择顺序为：

1. 当前 active focus；
2. 同一 acceptance slice；
3. 依赖已满足的 CWI；
4. 依已校验并封存的 provider-authored `coverageOrder` 取连续项目；
5. 达到 batch item 或 output budget 即停止。

LLM 不得选择下一个 batch、修改 batch 顺序、增加 CWI 或改变 dependency。

### Ledger 字段

`RuntimeSemanticPlanningLedger` 增加：

```text
Batches
ActiveBatchID
NextBatchSequence
PendingCoverageIDs
CompletedBatchIDs
BatchDocuments
MergedDocument
MergedDocumentHash
```

所有字段只能由 Orchestrator private internal event + 单次 CAS 修改。`blocks/llm` 不得保存 batch 状态。

### Continuation 与 Batch 的区别

Continuation 只补齐同一 batch 的截断响应，必须保持相同：

```text
batchId / logicalRequestId / manifestHash / coverageWorkItemIds
```

下一个 batch 必须新建 `batchId`、logical request 和 sequence，不能用 continuation 扩大 coverage set。

### Resume/Restart/Crash

- `resume`：恢复 active batch；优先继续已有 fragment，再继续 document/feasibility；完成后创建下一个 batch。
- `restart`：新 Run、新 full manifest、新 batch lineage；不继承 predecessor 的 semantic credit。
- provider 已提交但 outcome 未知：不得重复提交同一 logical request。
- fragment 已持久化：只允许继续同一 batch continuation。
- batch document 已完成：不得再次请求 LLM，直接恢复 feasibility 或下一个 batch。
- definition、catalog、provider ceiling 或 order hash 改变：当前 batch 失效，创建新 lineage。

## 3.3 Semantic planning 主流程迁移方案

本节是后续代码迁移的唯一执行顺序。迁移期间不得在单个阶段增加旁路 retry、旁路 feasibility、自动
replan 或跨 owner 状态写入。

### 3.3.1 新增并固化的 ledger phase

SM-3/SM-2 仍由 Orchestrator owner 管理，phase 只允许由 private internal event + 单次 CAS 迁移：

```text
semantic_contract_required
  → coverage_order_request_open
  → coverage_order_accepted
  → proof_batch_request_open
  → proof_batch_document_accepted
  → proof_batch_feasibility_pending
  → proof_batch_feasibility_complete
  → next proof_batch_request_open OR merged semantic_feasibility_required
  → semantic_feasibility_sealed
```

已有 `semantic_plan_requested`、`semantic_plan_correction_required` 只用于当前 step 的 provider
request session；不得把 order step 与 proof batch 混在同一个 logical request 中。

### 3.3.2 Order step

1. fresh activation 只创建 full manifest 和 order request session。
2. request context 的 `planStep=coverage_order`，provider 只能返回完整 `coverageOrder`，不得返回 proposal。
3. Orchestrator 解码并校验完整排列、依赖先后、manifest/seed identity，然后通过 private event 封存 order document。
4. provider 返回 malformed、遗漏、重复或反向依赖时，只进入既有 semantic correction budget；不得由 Orchestrator
   自行排序修正。
5. order document 的 `DocumentHash`、`CoverageOrderHash` 和 manifest hash 写入 ledger lineage。

### 3.3.3 Proof batch request

1. order accepted 后，Orchestrator 使用 `CompileRuntimeSemanticPlanBatch` 生成 batch；业务顺序完全来自
   provider-authored order。
2. 每个 batch 创建新的 `batchId`、`sequence`、`requestLogicalId`；continuation 只能复用同一 batch identity。
3. request context 携带 `planStep=proof_batch`、batch coverage IDs、batch identity；LLM 只能返回当前 batch
   proposals。
4. batch document 完成后通过 `SemanticPlanBatchDocumentObserved` 持久化，并迁移 batch 到
   `document_complete`；不重复请求同一个完整 document。
5. 当前 batch 未完成前禁止创建下一个 batch、请求 feasibility 或进入 seal。

### 3.3.4 Feasibility fan-out

1. `document_complete` 只由 Orchestrator 创建 owner feasibility 请求，capability owner 返回 typed facts。
2. 每个 batch 的 facts 通过 `SemanticPlanBatchFeasibilityObserved` 持久化；重复 exact result 幂等，冲突 result 拒绝。
3. batch facts 未完整覆盖当前 batch proposal 时保持 `feasibility_pending`，不得降级为普通 repair。
4. 当前 batch feasibility complete 后迁移 batch 为 `accepted`；若仍有 pending coverage，创建下一个 batch。

### 3.3.5 Full merge 与 seal

1. 所有 batch document 完成后调用 `MergeRuntimeSemanticPlanBatchDocuments`，通过 private merge event 保存
   `MergedDocument/MergedDocumentHash`。
2. 所有 batch feasibility result 完成后调用 `MergeRuntimeSemanticPlanBatchFeasibilityResults`，通过 private
   merge event 保存完整 `Feasibilities/FeasibilitySetHash`。
3. 只有 merged document 和完整 feasibility set 同时存在，才允许进入既有 `semantic_feasibility_required`
   和 SM-2 binding seal；任何单 batch result 都不能直接 seal。
4. seal 后才允许 operation candidate、dispatch、receipt/evidence；SM-1 lifecycle 不参与 semantic next-step 选择。

### 3.3.6 Request session 与恢复

| 场景 | 处理规则 |
| --- | --- |
| order request 已提交但 outcome 未知 | 不重放；恢复同一 logical request，等待 durable outcome |
| order document 已持久化 | 不重复请求 LLM，直接进入 order accepted |
| batch document 已持久化 | 不重复请求该 batch，直接进入 feasibility |
| batch feasibility 已部分持久化 | 只请求缺失 owner fact，保留已有结果 |
| batch 完成后 crash | 从 batch state 和 ledger 恢复下一个 batch 或 merge |
| definition/catalog/provider ceiling/order hash 漂移 | 旧 lineage 失效，创建新 request lineage |
| resume | 恢复当前 run 的 active batch/session |
| restart | 新 run、新 manifest、新 order request，不继承 semantic credit |

### 3.3.7 实施顺序

1. 先增加 phase/ledger identity 及迁移不变量测试。
2. 再接 order request context 和 order accepted private event。
3. 接 proof batch request session 和 batch state transitions。
4. 接 batch document/feasibility event 与恢复分支。
5. 接 full merge 与现有 feasibility seal gate。
6. 最后删除旧的“一次 full manifest 直接生成 proposals”主路径，并运行 live 验证。

### 3.3.8 禁止事项

- 不得用 `CoverageItem.Order`、目录顺序、map 遍历顺序替代 LLM `coverageOrder`。
- 不得让 LLM 选择下一个 batch、修改 batch sequence 或扩展 coverage set。
- 不得让 File/Git/Shell/MCP 等 capability owner 写 semantic ledger。
- 不得在 batch 未完成时调用全量 feasibility 或生成 sealed binding。
- 不得为恢复增加第二套 request session、第二套 document hash 或第二套 coverage authority。

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
11. dependency graph cycle/reverse-order rejection；
12. provider-authored `coverageOrder`/hash 在重复编译与 restart 后稳定；
13. batch CWI 重复、遗漏、跨 batch 越权 rejection；
14. batch document complete 后 crash，restart 不重复调用 LLM；
15. 全部 batch 未完成时禁止 SM-2 seal/dispatch；
16. 全量测试、vet、fmt-check 和部署实例验证。

## 6. 完成判定

本计划完成必须同时满足：

- DeepSeek semantic-plan 不再一次生成 29K 以上输出；
- 单次请求在代理 body-idle timeout 内稳定获得首个响应块或明确错误；
- provider timeout 不再使用 1800s 这类无界等待配置；
- continuation、retry、batch 和 failure outcome 均可从持久化 aggregate 恢复；
- provider-authored CWI order、dependency barriers 和 batch sequence 可重算且 hash 稳定；
- batch 合并后恰好覆盖 full manifest 全部 CWI，无重复、遗漏或越权；
- SM-1/SM-2/SM-3/SM-4 owner 边界没有被跨组件修改破坏；
- live run 至少完成一个 state-changing CWI，并产生 typed receipt/evidence 链。

## 7. 当前明确遗留项

1. phase vocabulary、request context `planStep`/batch identity、provider instruction、order-first request、proof-batch
   request-session、document → feasibility handoff、多 batch 循环和最终 seal gate 均已接入。
2. `RuntimeSemanticPlanningLedger` 已持久化 order/batch/document merge 基础字段及 batch documents；drive 会在 batch
   完成后归档 owner facts、创建下一 batch，或执行 document/feasibility merge 后进入既有 atomic seal。
3. batch DTO、状态校验、`planStep`、order/document identity、按已封存 order 编译 batch 的纯 helper、Orchestrator
   private batch-compilation/advance event、batch document 分片字段、full-manifest merge helper 及单 batch
   feasibility 完整性校验 helper、batch feasibility result DTO/identity、ledger 有序 slice 及全量 feasibility merge
   helper 已实现；feasibility 分批驱动已接入，owner facts 只在当前 batch 工作集内累积并按 batch result 归档。
4. batch document CAS 后 replay、merged document CAS 后 replay、最终多 batch feasibility merge CAS response-loss、batch correction
   保持 active lineage、跨部署 batch-size 变化时按已完成 coverage offset 恢复、accepted active batch feasibility identity/stale gate
   已补齐；batch document hash 与 batch state 的绑定、merge 前 incomplete-batch rejection、restart/new-run 与 fragment content
   隔离组合也已补齐。剩余仅为真实部署实例上的 restart/live 故障验证。
5. 运行实例仍需重新部署并确认 provider timeout、stream、8K 阶段预算和 typed body-idle outcome 生效。
6. fresh live 尚未证明 ordered batches 最终形成完整 binding→receipt→evidence 链。
