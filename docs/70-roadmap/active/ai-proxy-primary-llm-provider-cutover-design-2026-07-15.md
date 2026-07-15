# ai-proxy 首选 LLM Provider 与 Runtime V2 模型能力切换设计

Status: active

Note: 2026-07-15 最新复核确认 ai-proxy（`e40a632`）的 operation-aware catalog、WorkOrch 的 sealed
target/audit 主链和既有聚焦门禁仍可保留；但本文不能再标记为“仅待 live”。第 28 节规定的 typed
此前 LLM usage/context-budget read-model 的 `map[string]any` 投影已收敛为 typed `Payload`，仅保留
EventRecord 的单一受限解码边界；server package 的聚合测试合同也已同步到 concrete projection SSE，修正后
`make test` 已连续两次通过。最终跨进程 live verifier 已验证 catalog/chat/embedding、两个 sealed ceiling、
catalog change、WorkOrch restart 和 persisted audit/read-model 还原，见 §1.3 与
`docs/60-verification/ai-proxy-primary-llm-provider-cutover-live-verification-2026-07-15.md`。


Type: closure-design

Last Updated: 2026-07-15

Review State: code-closed

## 1. 文档目的

本文定义 WorkOrch 将 `/home/rangh/aispace/ai-proxy` 作为首选 LLM provider，并通过
`GET /v1/models` 自动枚举模型、解析模型上下文能力、生成确定性上下文预算、封存
Runtime V2 provider/model ceiling 的完整目标设计。

本文是后续代码收口的执行依据。实现期间不得同时保留旧 provider 配置模型、旧
activation 字段、旧 provider fallback 或其他兼容入口。代码完成后，应将最终事实同步到：

- `internal/modules/blocks/llm/readme.md`
- `internal/modules/kernel/orchestrator/readme.md`
- `docs/30-module-architecture/llm-provider-assembly.md`
- `docs/01-overview/current-state.md`
- 部署配置与 Web Console 使用文档

本文不改变 `docs/00-core/runtime-stage-contract.md` 定义的 Runtime V2 收敛主链，也不改变
`docs/00-core/runtime-request-cache-note.md` 定义的 C0-C5 provider-neutral cache 分层。

### 1.1 2026-07-15 首次复核结论（历史基线）

本节保留首次复核时的代码事实，用于解释 R01-R08 的来源；其中部分问题后来已完成。当前有效状态以
§1.2、§28.2、第 30 节勾选状态和第 31 节各批次状态为准。

当前不能将“类型、route 或 UI 已存在”视为对应批次完成。复核后的事实分为两组。

已确认成立：

1. canonical 配置已切换为 `AIProxy`、`DefaultChat`、可选 `DefaultEmbedding`、`Budget` 和
   `CustomProviders`。
2. `ProviderModelTarget`、`ResolvedProviderModelProfile` 主体和 Runtime V2 `ProviderCeiling` 已建立。
3. Runtime V2 candidate 已从 sealed ceiling 读取 target/budget，并通过
   `completeProviderWithSealedCeiling` 避免重新解析 catalog。
4. catalog cache 已具备 TTL、LKG、大小限制、同 key inflight 合并和不同 key 隔离。
5. application catalog HTTP route、EventHub use-case、ai-proxy-first Web 配置和 activation `llmTarget`
   主骨架已建立。
6. `defaultEmbedding` 为空或 semantic recall 失败时，workspace memory list 基线回退主体已实现。
7. ai-proxy 已具备 typed `/v1/models` DTO、规范化 operations、确定 RouteOwner、请求前 operation
   enforcement 和 endpoint capability 过滤；其全量 Go 测试与 vet 当前通过。

仍阻断代码收口：

1. **P1：Runtime V2 request audit 无生产闭环。** candidate 调用只返回 payload，provider usage、
   context budget、sealed target/revision 没有生产 `/run/llm_usage/recorded` 事件或等价 typed audit record；
   当前 read model 只有消费路径和测试 fixture。
2. **P1：ai-proxy exact override 绕过 operation authority。** `AIProxyModelOverrideConfig` 未声明
   operations；catalog 缺失或没有对应 model 时，override 仍可直接生成 ready profile。
3. **P1：catalog 错误体未使用当前完整 API Key 脱敏。** 当前仅清理控制字符和 `Bearer ` 文本，
   上游回显裸 secret 时可能进入 catalog summary、HTTP 或 Web。
4. **P1：配置 map key 未 canonicalize。** 校验使用 trim/case-fold，但没有重建规范化 map；readiness
   可通过 trimmed fallback 命中，provider client 和 revision 仍使用 exact lookup，可能出现 readiness 成功、
   request 构造失败或 revision 输入不完整。
5. **P1：旧 loadtest 与 README 合同残留。** `scripts/loadtest-governed-workspace-concurrency.py` 仍使用
   `defaultProviderId`、`defaultEmbeddingProviderId`、`providers` 和旧 context profile；README 仍描述旧
   `defaultEmbeddingProvider`。
6. **P2：readiness response 未携带 operation。** request 虽携带 operation，但
   `ResolvedProviderModelProfile` 没有 `Operation`，application 不能验证 response operation 与 request 一致。
7. **P2：Web 仍存在 operation 兼容回退。** operations 为空的 model 仍可能被展示；custom model
   operations 为空时会被自动补成 `chat_completions`。
8. **P2：API Key 没有独立显式清空字段。** 空值只能保留旧 secret，无法在保留 provider 的同时删除
   ai-proxy 或 custom provider API Key。
9. **P2：新主链 typed closure 未完成。** catalog application use-case 仍使用 `any` 选择 command，
   usage/context/memory evidence read model 仍依赖 `map[string]any`。
10. **P2：revision、错误分类和 catalog boundary 未严格实现。** configuration drift 仍返回
    `configuration_invalid`；`CatalogConnectionRevision` 包含 model override；catalog unknown/duplicate
    operation 被保留或静默去重，而不是按合同标记 unavailable。
11. **验证阻断：** WorkOrch appservice 根包可复现后台 drive/config manager cleanup 竞态；
    `make fmt-check` 和 `make vet` 未通过；全量 Go 与 live 联调未闭环。

因此，本文后续所有 `[x]` 只表示已经通过代码、测试和静态扫描共同确认的事实。上述阻断项对应的
第 30 节条目必须保持 `[ ]`，并按第 31 节顺序逐项收口。

### 1.2 2026-07-15 二次代码评审结论

在第 30 节大部分条目被勾选、顶部 Note 被更新为“live 前代码收口完成”后，再次按生产调用链而非单侧
fixture 进行核对。二次评审确认以下事实：

已确认成立：

1. ai-proxy `/v1/models` 已输出规范化 operations 和确定 RouteOwner，并在请求前执行 operation authority
   校验；ai-proxy 全量 Go 测试与 vet 通过。
2. WorkOrch target、activation persistence、ProviderCeiling、sealed configuration drift、catalog cache key/LKG、
   secret fingerprint/redaction 主体已建立。
3. WorkOrch config/runtimecontract/LLM/orchestrator focused tests、fmt、vet、boundary 和 Web build 当前通过。

重新打开的代码阻断：

| ID | 优先级 | 当前代码事实 | 影响 |
| --- | --- | --- | --- |
| RR01 | P0 | `GenerateCandidate` 仍返回 `[]byte`；provider completion 后对局部 `ProviderRequest.ContextBudget` 设置的 target/revision/ProviderSubmitted/ActualTokens 没有进入 `RequestRuntimeCandidateResult` 或持久化 owner command | 成功和失败 provider request 均不能从持久化 audit 还原 sealed target、预算、usage 和提交状态，R07 未完成 |
| RR02 | P0 | WorkOrch 全包测试运行 10 分钟后阻塞在 appservice session shutdown → workspace module unsubscribe；失败测试单独运行可通过 | 仍存在 suite-order/background lifecycle 竞态，`make test` 与 R05/R08 完成门禁不成立 |
| RR03 | P1 | `NormalizeLLMConfig` 先把 map key trim 后写入新 map，再执行 duplicate validation；`"custom-a"` 与 `" custom-a "` 会在校验前互相覆盖 | 配置结果依赖 Go map 遍历顺序，readiness/revision/client authority 可能选择非确定配置，R03 未完成 |
| RR04 | P1 | `configcore/llm_catalog.go` 仍使用 `var payload any` 和通用 result type switch；usage/context budget read model 仍大量依赖 `map[string]any` | 新主链 typed closure 与 R04/R07/R08 删除要求未完成 |
| RR05 | P1 | `scripts/loadtest-governed-workspace-concurrency.py` 仍发送 `defaultProviderId`、`defaultEmbeddingProviderId`、`providers` 和旧 `contextProfile` | 严格新 API 会拒绝 loadtest，live verifier/loadtest 清零条目不成立 |
| RR06 | P1 | Web 新建 custom model 自动填充 `chat_completions`；ConfigurationPage 没有 custom provider API Key clear UI/请求组装；后端 clear 与非空新值同时出现时采用 clear wins | Web 仍猜测 operation，管理员不能按设计显式清空 custom secret，冲突请求未 fail-fast，R06 未完成 |

因此第 30 节相关条目已重新取消勾选。后续不得仅以 focused tests 或字段存在作为完成依据；每个 RR 项必须
同时具备生产链证据、负向回归和对应静态扫描。

### 1.3 2026-07-15 最新 ai-proxy 联合复核结论（当前执行基线）

本节以最新 ai-proxy 提交 `e40a632 feat: close provider capability contract` 与当前 WorkOrch 工作区为准。
ai-proxy `go test ./... -count=1`、`go vet ./...` 已通过，并确认其 `GET /v1/models` 输出严格的
`chat_completions` / `embeddings` operations。WorkOrch 的既有 target、sealed ceiling、candidate audit
和配置/Web 代码不应回退。

但第 28 节的最终门禁尚未成立，新增以下阻断项：

| ID | 优先级 | 当前代码事实 | 必须收口的结果 |
| --- | --- | --- | --- |
| R11 | P1 | 已通过 `scripts/verify-ai-proxy-primary-llm-provider-live.sh` 完成跨进程验证；artifact `docs/60-verification/artifacts/ai-proxy-primary-llm-live-runtime-20260715T150944Z/` 记录真实 ai-proxy 与真实 WorkOrch 的 catalog/refresh、chat、embedding、embedding-only chat 拒绝、两个 Run 的不同 sealed ceiling、catalog change 后旧 Run audit 不漂移，以及 WorkOrch restart 后 persisted audit/read-model 还原 | 已完成；本地确定性上游仅替代外部模型凭据，不替代 ai-proxy 进程和协议 |

因此第 30 节中“仅待 live”的历史勾选不构成完成依据；R11 已通过可复核 artifact 关闭，本文状态为
`code-closed`。

## 2. 已确认前提

### 2.1 ai-proxy 当前合同

ai-proxy 已提供本地合成的 `GET /v1/models` 与 `POST /v1/models`：

- 数据来自 ai-proxy 的全局 `model_catalog`。
- 不实时查询上游 provider。
- 仅返回 catalog 中登记的具体模型，不展开 provider model pattern。
- `id` 是 WorkOrch 调用 ai-proxy 时应提交的模型 ID。
- `owned_by` 是 ai-proxy 内部路由归属提示，不是 WorkOrch provider identity。
- `contextWindowTokens` 与 `maxOutputTokens` 是 ai-proxy 扩展字段。
- ai-proxy 根据请求 body 中的 `model` 完成上游路由和同协议 fallback。

当前响应示例：

```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4o",
      "object": "model",
      "created": 0,
      "owned_by": "openai",
      "contextWindowTokens": 128000,
      "maxOutputTokens": 16384
    }
  ]
}
```

R02 收口后的目标响应必须增加 `operations`，例如：

```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-4o",
      "object": "model",
      "created": 0,
      "owned_by": "openai",
      "operations": ["chat_completions"],
      "contextWindowTokens": 128000,
      "maxOutputTokens": 16384
    }
  ]
}
```

### 2.2 WorkOrch 切换前基线差距

以下是本设计启动时的旧主线差距，用于解释 C01-C07 的断代范围；当前实现状态以第 1.1 节评审结论为准：

1. 一个 `LLMProviderConfig` 固定绑定一个 `Model`，无法表达一个 ai-proxy endpoint 下的多个模型。
2. `LLM.ModelCapabilities` 依赖 provider/model pattern 匹配，不是 ai-proxy 精确目录权威。
3. `ContextProfileMode` 同时存在 auto/manual 与保守默认，可能掩盖模型能力缺失。
4. Runtime V2 `ProviderCeiling` 只封存 `ProviderID` 与 `MaxBytes`，无法确定具体模型。
5. workspace activation 只携带 `LLMProviderID`，model 仍由 provider config 隐式决定。
6. `LLMProviderID` 经进程内 `context.Context` 传给后台 drive，未成为持久化 Runtime V2 fact。
7. `selectProviderConfig` 在未指定 provider 时可能遍历 map 选择任意可用 provider，结果不确定。
8. catalog、endpoint、模型选择和某次 Run 的能力快照没有区分。
9. Web Console 以 provider 编辑器和 model capability pattern 为主要入口，不符合 ai-proxy 首选模式。

## 3. 目标与非目标

### 3.1 目标

- ai-proxy 是内置、首选、默认的 LLM provider。
- WorkOrch 自动读取 ai-proxy `/v1/models` 并展示可用模型。
- 每个模型使用精确的上下文窗口和最大输出信息推导上下文预算。
- provider endpoint、model catalog、model target、resolved profile、Runtime V2 ceiling 各自具有明确语义。
- workspace activation 在创建 Run 前解析出明确的 provider/model target。
- Runtime V2 prerequisite 封存确定的 provider/model capability，不依赖后续目录变化。
- 自定义 provider 只作为高级配置能力存在。
- 所有跨组件交互使用 EventHub 和具体 command/result 类型。
- provider-facing C0/C1/C2 稳定前缀不因 provider/model 切换发生漂移。
- 配置、API、Web、Runtime V2、测试和部署配置一次性切换到新合同。

### 3.2 非目标

- WorkOrch 不动态查询 ai-proxy 的每个上游 provider。
- WorkOrch 不复制 ai-proxy 的上游路由和 fallback 逻辑。
- WorkOrch 不基于模型名称猜测上下文窗口。
- WorkOrch 不自动选择“性能最好”或“价格最低”的模型。
- WorkOrch 不在 Runtime V2 reducer 中理解 OpenAI、Anthropic、ai-proxy 或具体模型名称。
- 本次不保留旧 LLM 配置、旧 HTTP payload 或旧 persisted active Run 的恢复兼容。
- 本次不引入 V1 StagePacket、NextAction、ExecutionContextPatch 或 map authority rebuild。

## 4. 不兼容切换决策

本次 LLM provider 调整不考虑兼容性，必须遵守以下规则：

1. 不读取或迁移旧 `[llm].defaultProvider`、`defaultEmbeddingProvider`、`modelCapabilities`、
   `providers` 配置。
2. 不保留旧 `LLMProviderConfig.Model` 固定绑定路径。
3. 不保留旧 `ContextProfileMode`、model pattern registry 或 conservative default 路径。
4. 不保留只携带 `LLMProviderID` 的 HTTP、application、context 或 EventHub 合同。
5. 不保留旧 provider 自动选择、map 遍历回退和 WorkOrch 侧 provider failover。
6. 不增加 legacy decoder、双写、旧字段 alias、未知字段回退或 compatibility adapter。
7. 旧配置应在启动配置校验阶段明确失败，不能被静默忽略。
8. 旧 pending/active/blocked Run 若未持久化完整 model target/ceiling，不允许继续 Resume；部署时应终止或清理后重新 activation。
9. completed/failed 历史 Run 可以只读展示，但不得进入新 Runtime V2 drive。

## 5. 核心架构决策

| 编号 | 决策 |
| --- | --- |
| ADR-01 | ai-proxy 使用保留 ProviderID `ai-proxy`，`owned_by` 不参与 WorkOrch provider 选择。 |
| ADR-02 | endpoint 与 model target 分离；不得为每个发现模型生成持久化 provider config。 |
| ADR-03 | `/v1/models` 是 ai-proxy 模型容量元数据权威；精确 override 只作为高级修正能力。 |
| ADR-04 | `blocks/llm` 独占 catalog HTTP 访问、缓存、能力解析和预算推导。 |
| ADR-05 | application、Web 和 orchestrator 不直接访问 ai-proxy。 |
| ADR-06 | activation 必须产生完整 `ProviderModelTarget`，默认值也在 application activation 阶段解析。 |
| ADR-07 | target identity 随 typed bootstrap 持久化，不能继续依赖 `context.Context` 传递。 |
| ADR-08 | Runtime V2 provider ceiling 同时封存 provider、model、token/byte ceiling 和能力 revision。 |
| ADR-09 | catalog 刷新只影响尚未封存 ceiling 的新 Run；已封存 Run 不漂移。 |
| ADR-10 | 模型能力不完整时 fail-fast，不使用模型名猜测或保守窗口掩盖缺失。 |
| ADR-11 | ai-proxy 上游 fallback 由 ai-proxy 负责，WorkOrch 不做二次 provider fallback。 |
| ADR-12 | 自定义 provider 通过显式 target 使用，不参与普通模式的隐式选择。 |
| ADR-13 | provider/model/baseURL 只进入动态 request tail 或 audit，不进入 C0/C1/C2 stable prefix。 |
| ADR-14 | catalog revision 使用规范化字段的确定性 hash，不依赖 JSON/reflect 内部桥接。 |
| ADR-15 | 所有直接 completion、embedding、catalog 和 Runtime prerequisite 合同均使用具体类型。 |
| ADR-16 | candidate 只能使用 sealed ceiling 推导请求预算；不得重新访问 catalog 或用当前 profile 替换 sealed profile。 |
| ADR-17 | provider client 可读取当前 canonical secret/endpoint，但必须先验证完整 target configuration revision 与 sealed revision 相等；不相等时 fail-fast，不允许静默漂移。 |
| ADR-18 | catalog cache revision 与 target configuration revision 分离；inflight refresh 必须按 catalog cache key 隔离。 |
| ADR-19 | 模型 readiness 必须绑定具体 operation；chat 与 embedding 不能共享无 operation 的模糊 ready 状态。 |
| ADR-20 | `defaultEmbedding` 是可选增强能力；未配置或调用失败不得清空 workspace memory 基线选择。 |
| ADR-21 | API Key 只以不可逆 fingerprint 参与内部 revision，不能以 configured/length 近似，也不能单独暴露 fingerprint。 |

## 6. 目标组件关系

```text
Web Console / CLI
        |
        | HTTP / application port
        v
application/workorchd
        |
        | typed EventHub command/result
        v
blocks/llm
  |-- ai-proxy catalog client/cache ---- GET /v1/models
  |-- custom provider definitions
  |-- target resolution
  |-- context budget resolution
  |-- provider request adapter
  `-- typed Runtime V2 candidate decoder
        ^
        | Orchestrator-owned typed prerequisite/candidate command
        |
kernel/orchestrator
        |
        `-- Run owner persisted Runtime V2 facts / ProviderCeiling
```

### 6.1 `blocks/llm` 职责

- 读取 canonical LLM 配置快照。
- 访问 ai-proxy `/v1/models`。
- 维护进程内 bounded catalog cache 与 last-known-good。
- 验证、规范化和排序模型目录。
- 解析 `ProviderModelTarget`。
- 推导 `ResolvedProviderModelProfile`。
- 响应 provider ceiling prerequisite。
- 使用封存 target 发起 completion/embedding 请求。
- 组装 C0-C5 provider-facing request。
- 严格解码 Runtime V2 candidate。

`blocks/llm` 不拥有 Run、Result、Evidence、Workspace 或正式业务状态。catalog cache 是该 block
内部的技术状态，不是业务 aggregate。

### 6.2 `application/workorchd` 职责

- 提供配置、catalog 查询/刷新、workspace activation HTTP/CLI 用例。
- 保存 ai-proxy endpoint、默认 target 和高级自定义 provider 配置。
- 通过 EventHub 查询 LLM block 的 catalog/readiness，不直接发 HTTP 到 ai-proxy。
- activation 时把缺省 target 解析为显式 target。
- 构造包含 target identity 的 typed `RuntimeBootstrap`。

### 6.3 `kernel/orchestrator` 职责

- 从 persisted bootstrap/facts 读取 target identity。
- 通过 Orchestrator 自有 prerequisite command 请求 LLM block 解析 ceiling。
- 校验并封存具体 `ProviderCeiling`。
- candidate 请求只使用 facts 中已封存 ceiling。
- 只解释通用的 identity/revision/ceiling 完整性，不解释模型品牌、路由或预算算法。

## 7. 统一术语和类型合同

### 7.1 Provider/model target

共享 Runtime V2 合同新增：

```go
type ProviderModelTarget struct {
    ProviderID string
    ModelID    string
}
```

约束：

- `ProviderID`、`ModelID` trim 后均不能为空。
- 内置 ai-proxy 的 `ProviderID` 固定为 `ai-proxy`。
- 自定义 provider ID 不得使用保留值 `ai-proxy`。
- target 是 identity，不包含 API Key、BaseURL 或动态 provider client。
- target 必须实现 `Normalized`、`Validate`、`Same` 和 clone 测试。

### 7.2 Catalog descriptor

`blocks/llm/pkg/models` 定义：

```go
type ProviderModelDescriptor struct {
    Target              runtimecontract.ProviderModelTarget
    RouteOwner          string
    Operations          []string
    ContextWindowTokens int
    MaxOutputTokens     int
    CapabilityReady     bool
    UnavailableReason   string
    CapabilityRevision  string
}

type ProviderModelCatalog struct {
    ProviderID string
    Models     []ProviderModelDescriptor
    Revision   string
    FetchedAt  time.Time
    ExpiresAt  time.Time
    Stale      bool
    Source     string
}
```

其中：

- `RouteOwner` 仅用于显示和审计。
- `CapabilityReady` 是目录能力是否足以进入 Runtime V2 的判定。
- `CapabilityRevision` 是单个模型能力 hash。
- `Revision` 是规范化完整目录 hash。
- model slice 必须按 case-fold 后的 model ID 稳定排序。

### 7.3 Resolved profile

```go
type ResolvedProviderModelProfile struct {
    Target                  runtimecontract.ProviderModelTarget
    Operation               string
    APISurface              string
    ContextWindowTokens     int
    ProviderMaxOutputTokens int
    EffectiveMaxOutputTokens int
    ReservedOutputTokens    int
    MaxInputTokens          int
    EstimatedTokensPerByte  float64
    MaxRequestBytes         int
    CapabilitySource        string
    CapabilityRevision      string
    ConfigurationRevision   string
}
```

`CapabilitySource` 只允许：

- `ai_proxy_catalog`
- `ai_proxy_exact_override`
- `custom_provider_explicit`

不得继续出现：

- `model_registry`
- `conservative_default`
- pattern match source

`Operation` 是本次解析实际请求的 operation，不是 descriptor 中全部 operation 的复制。readiness 调用方必须
校验返回 `Target` 和 `Operation` 均与请求一致。

### 7.4 Runtime V2 ProviderCeiling

替换现有合同：

```go
type ProviderCeiling struct {
    Target                   ProviderModelTarget
    ContextWindowTokens      int
    ProviderMaxOutputTokens  int
    EffectiveMaxOutputTokens int
    MaxInputTokens           int
    MaxBytes                 int
    CapabilityRevision       string
    ConfigurationRevision    string
}
```

Runtime core 只要求：

- target 有效。
- token/byte ceiling 均为正数。
- `MaxInputTokens + EffectiveMaxOutputTokens <= ContextWindowTokens`。
- `MaxBytes` 满足 prerequisite `MinimumBytes`。
- capability/configuration revision 非空。

预算如何推导仍属于 LLM block，不进入 reducer。

## 8. 新配置合同

### 8.1 Go 配置模型

```go
type LLMConfig struct {
    AIProxy          AIProxyProviderConfig
    DefaultChat      ProviderModelTargetConfig
    DefaultEmbedding *ProviderModelTargetConfig
    Budget           LLMContextBudgetPolicyConfig
    CustomProviders  map[string]CustomLLMProviderConfig
}

type AIProxyProviderConfig struct {
    BaseURL                  string
    APIKey                   string
    Timeout                  string
    CatalogRequestTimeout    string
    CatalogTTL               string
    CatalogMaxStaleAge       string
    MaxCatalogResponseBytes  int
    MaxCatalogModels         int
    ModelOverrides           map[string]AIProxyModelOverrideConfig
}

type ProviderModelTargetConfig struct {
    ProviderID string
    ModelID    string
}

type LLMContextBudgetPolicyConfig struct {
    ContextUtilizationRatio float64
    EstimatedTokensPerByte  float64
    DefaultMaxOutputTokens  int
    MaxRequestBytes         int
}

type AIProxyModelOverrideConfig struct {
    ContextWindowTokens int
    MaxOutputTokens     int
    Operations          []string
}

type CustomLLMProviderConfig struct {
    Provider   string
    APISurface string
    BaseURL    string
    APIKey     string
    Timeout    string
    Models     map[string]CustomLLMModelConfig
}

type CustomLLMModelConfig struct {
    ContextWindowTokens int
    MaxOutputTokens     int
    Operations          []string
    Temperature         *float64
    ReasoningEffort     string
    ThinkingBudgetTokens int
    Cache               ProviderCacheConfig
}
```

### 8.2 推荐 TOML

```toml
[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = ""
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[llm.defaultEmbedding]
providerID = "ai-proxy"
modelID = "text-embedding-3-large"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
defaultMaxOutputTokens = 4096
maxRequestBytes = 4194304
```

高级自定义 provider：

```toml
[llm.customProviders.direct-anthropic]
provider = "anthropic"
apiSurface = "anthropic-messages"
baseURL = "https://api.anthropic.com"
apiKey = ""
timeout = "180s"

[llm.customProviders.direct-anthropic.models."claude-sonnet-4-20250514"]
contextWindowTokens = 200000
maxOutputTokens = 8192
operations = ["chat_completions"]
```

### 8.3 配置校验

- ai-proxy BaseURL 必须是合法 `http` 或 `https` URL。
- BaseURL 不允许 userinfo、query 或 fragment。
- timeout、TTL 和 max stale age 必须为正值。
- `CatalogMaxStaleAge >= CatalogTTL`。
- response bytes 和 model count 必须有正的有限上限。
- budget ratio 必须处于 `(0, 1]`。
- `EstimatedTokensPerByte > 0`。
- 每个 ai-proxy exact override 必须同时提供正的 context window、max output 和非空已知 operations；
  不允许只覆盖半个能力合同。
- default chat target 必须完整；embedding target 可以为空。
- custom provider ID trim 后唯一，且不能等于 `ai-proxy`。
- custom provider/model/override map key 必须先 canonical trim，再检测重复；不得保留两个规范化后相同的 key。
- custom model 必须明确配置 context window、max output 和 operations。
- operation 列表必须来自已知枚举。
- API Key 永远不出现在 read DTO、日志、EventHub result 或 request audit。

配置更新只做结构校验，不要求 ai-proxy 当时在线。连接和模型 readiness 由 catalog/resolve API 单独报告。

## 9. ai-proxy `/v1/models` 适配设计

### 9.1 外部 wire DTO

HTTP adapter 必须使用具体类型：

```go
type aiProxyModelsResponse struct {
    Object string               `json:"object"`
    Data   []aiProxyModelRecord `json:"data"`
}

type aiProxyModelRecord struct {
    ID                  string `json:"id"`
    Object              string `json:"object"`
    Created             int64  `json:"created"`
    OwnedBy             string `json:"owned_by"`
    Operations          []string `json:"operations"`
    ContextWindowTokens int    `json:"contextWindowTokens"`
    MaxOutputTokens     int    `json:"maxOutputTokens"`
}
```

ai-proxy 与 WorkOrch 本次均不要求兼容旧 catalog 扩展合同。`operations` 必须由 ai-proxy 的
规范化 `model_catalog` 明确给出，只允许 `chat_completions` 和 `embeddings`。WorkOrch 不根据
model ID 猜测 operation，也不因为 HTTP client 技术上实现了某个 endpoint 就宣称模型支持该 operation。

禁止在 WorkOrch adapter 中使用：

- `map[string]any`
- `[]any`
- `json.RawMessage` 作为 catalog 业务载荷
- reflect 类型分发
- JSON marshal/unmarshal 作为组件内合同转换

HTTP JSON decode 仅发生在外部协议边界。

### 9.2 请求规则

- 使用 `GET {BaseURL}/v1/models`。
- BaseURL 末尾 `/` 和已包含 `/v1` 的情况必须规范化，最终 path 只能出现一次 `/v1/models`。
- 使用与 ai-proxy completion 相同的 Bearer credential；API Key 为空时不发送 Authorization。
- `Accept: application/json`。
- 独立 catalog request timeout，默认 5 秒。
- 使用 `io.LimitReader` 或等价方式限制响应体。
- 仅接受 2xx；错误体只读取受限摘要并执行密钥/URL 脱敏。
- 不自动 retry 非幂等请求；GET 可以在同一次 refresh 内最多重试一次网络错误或 5xx，但总耗时不得超过 timeout。

### 9.3 响应校验

- `object` 必须为 `list`。
- model ID trim 后不能为空。
- model ID case-fold 后重复必须拒绝整个新目录，保留 LKG。
- `contextWindowTokens <= 0`：模型可展示但 `CapabilityReady=false`。
- `maxOutputTokens <= 0`：模型可展示但 `CapabilityReady=false`。
- `maxOutputTokens >= contextWindowTokens`：模型不可用。
- `operations` 为空、包含未知值或重复值：模型可展示但 `CapabilityReady=false`，并给出明确原因。
- operation 规范化后稳定排序；chat selector 只接受 `chat_completions`，embedding selector 只接受 `embeddings`。
- 模型数量超过上限：拒绝目录。
- 未知字段由 JSON decoder 忽略，不能进入内部 descriptor。
- `owned_by` 为空时规范化为 `ai-proxy`，但不影响 provider identity。

### 9.4 Revision 算法

catalog 和 capability revision 不通过 JSON round-trip 计算。应使用排序后的规范化字段进行长度前缀编码并计算 SHA-256：

```text
catalog revision input:
  providerID
  model count
  for each sorted model:
    modelID
    routeOwner
    operation count
    each sorted operation
    contextWindowTokens
    maxOutputTokens
    capabilityReady

capability revision input:
  providerID
  modelID
  operation count
  each sorted operation
  contextWindowTokens
  maxOutputTokens
  override revision
```

revision 格式使用稳定前缀，例如：

```text
catalog-sha256:<hex>
capability-sha256:<hex>
```

## 10. Catalog cache 与刷新状态机

### 10.1 Revision 分层与缓存 key

禁止继续用一个不完整的 `configurationRevision` 同时表达 catalog 连接状态与 Runtime target 执行状态。
必须拆成两个内部 revision：

```text
CatalogConnectionRevision:
  canonical ai-proxy baseURL
  catalog request timeout / TTL / max stale / response bytes / model count
  sha256("workorch.ai-proxy.catalog-auth.v1" + canonical API key)

TargetConfigurationRevision:
  normalized ProviderModelTarget
  provider kind / API surface / canonical baseURL / completion timeout
  sha256("workorch.llm.target-auth.v1" + canonical API key)
  complete budget policy
  exact model override or custom model declaration
  operation / inference / cache parameters
```

API Key fingerprint 只能作为最终 revision hash 的输入，不能单独进入日志、metrics、HTTP DTO、EventHub
result 或 request audit。API Key 的任意字节变化都必须改变对应 revision，同长度密钥轮换也不能例外。

内置 ai-proxy catalog cache key 为：

```text
providerID=ai-proxy + CatalogConnectionRevision
```

cache entry、LKG 和 inflight refresh 均必须携带同一个 cache key。禁止使用跨 key 的全局单一 inflight。

### 10.2 状态

```go
type ProviderCatalogState string

const (
    ProviderCatalogStateEmpty       ProviderCatalogState = "empty"
    ProviderCatalogStateFresh       ProviderCatalogState = "fresh"
    ProviderCatalogStateStale       ProviderCatalogState = "stale"
    ProviderCatalogStateRefreshing  ProviderCatalogState = "refreshing"
    ProviderCatalogStateUnavailable ProviderCatalogState = "unavailable"
)
```

### 10.3 读取规则

1. 没有缓存：同步刷新。
2. 当前时间早于 `ExpiresAt`：返回 fresh。
3. 已过 TTL 但未超过 max stale age：尝试刷新。
4. 刷新成功：替换 cache 和 LKG。
5. 刷新失败且 LKG 未超过 max stale age：返回 stale LKG，并附带受限错误摘要。
6. 刷新失败且没有可用 LKG：返回 unavailable。
7. 同一 key 的并发刷新必须合并，不能并发打满 `/v1/models`。
8. 不同 key 的刷新不得互相等待、复用结果或覆盖对方 LKG。
9. `CatalogConnectionRevision` 变化立即使旧 cache 失效。
10. refresh 完成时必须再次核对 call key；旧 key 结果不得写入新 key entry。

初版 LKG 仅为 `blocks/llm` 进程内技术缓存。服务重启后，新 Run 需要重新成功发现 catalog；已封存
Runtime V2 ceiling 的 Run 不依赖 catalog 恢复。

### 10.4 不使用后台轮询

catalog 采用按需 refresh：

- Configuration 页面读取模型列表。
- 用户显式刷新。
- activation target readiness 校验。
- provider ceiling prerequisite 在尚无可用 cache 时读取。

不新增常驻 timer；避免在没有使用 LLM 时持续请求 ai-proxy。

## 11. 模型能力解析规则

### 11.1 ai-proxy 优先级

只允许精确模型 ID 和明确 operation：

1. `aiProxy.modelOverrides[exactModelID]`。
2. ai-proxy catalog 精确 descriptor。
3. 能力不完整，模型不可用于新 Run。

target resolution 必须接收 `operation`：

- workspace activation/default chat 使用 `chat_completions`。
- memory semantic recall/default embedding 使用 `embeddings`。
- prerequisite/candidate 只解析 `chat_completions` ceiling。
- operation 不在 descriptor/override 中时返回 `operation_unsupported`，不得创建 Run 或发起 provider 请求。

禁止：

- model pattern。
- provider pattern。
- 根据 `gpt-*`、`claude-*` 等名称猜测。
- conservative 32K 等默认窗口。
- 未知模型继续提交请求。

override 是高级配置，只用于 ai-proxy catalog 暂时缺字段或已知元数据错误；必须显示
`CapabilitySource=ai_proxy_exact_override`。

### 11.2 自定义 provider

自定义 provider 不自动调用 `/v1/models`。其模型必须在高级配置中精确登记：

- model 的 `operations` 是执行合同，不是展示元数据。
- completion 前必须确认 `chat_completions`。
- embedding 前必须确认 `embeddings`。
- `ResolveProviderClient` 不得把“client 实现了某接口”当成“model 声明支持 operation”。
- trim 后重复的 provider ID/model ID 必须在配置校验阶段拒绝，不能依赖 map 遍历顺序选择。

- model ID。
- context window。
- max output。
- operations。
- provider-specific inference/cache 配置。

缺少任何必要容量字段时配置无效。

### 11.3 模型 operation 范围

当前 ai-proxy `/v1/models` 尚未返回 chat/embedding operation metadata，这是本次评审确认的上游合同缺口，
不是可以长期保留的 WorkOrch 推断空间。R02 必须同步切换 ai-proxy catalog：

- 每个模型必须明确声明至少一个已知 operation。
- catalog readiness 同时证明容量和 requested operation 完整。
- default chat/default embedding 只负责选择 target，不能替代模型 operation 声明。
- WorkOrch 不通过实际请求失败来补做 readiness，也不把 provider HTTP 4xx 当成正常 capability discovery。
- operation 变化必须改变 capability revision，但不改变 `ProviderModelTarget` identity。

## 12. 上下文预算算法

### 12.1 输入

- `ContextWindowTokens`：catalog/explicit capability。
- `ProviderMaxOutputTokens`：catalog/explicit capability。
- `DefaultMaxOutputTokens`：WorkOrch 全局请求策略。
- model-specific inference request max output（若高级配置明确声明）。
- `ContextUtilizationRatio`。
- `EstimatedTokensPerByte`。
- `MaxRequestBytes`。

### 12.2 有效输出额度

```text
requestedOutputTokens =
  model-specific configured max output, if present
  else global DefaultMaxOutputTokens

effectiveMaxOutputTokens = min(
  requestedOutputTokens,
  providerMaxOutputTokens,
  contextWindowTokens - 1
)
```

所有输入必须为正数。`effectiveMaxOutputTokens <= 0` 时 target 不可用。

### 12.3 输入 token 和 byte ceiling

```text
inputWindowTokens = contextWindowTokens - effectiveMaxOutputTokens

maxInputTokens = floor(
  inputWindowTokens * contextUtilizationRatio
)

estimatedInputBytes = floor(
  maxInputTokens / estimatedTokensPerByte
)

maxBytes = min(
  estimatedInputBytes,
  configured MaxRequestBytes
)
```

约束：

- 不再使用独立 `SafetyRatio` 和 `ContextUtilizationRatio` 双重折损。
- 不再保存用户可编辑的 `MaxInputTokens`、`ReservedOutputTokens`、`BytesHardLimit` 派生字段。
- 派生字段只作为 read model、request audit 和 `ProviderCeiling` 输出。
- `MaxRequestBytes` 是 WorkOrch 本地序列化/HTTP 安全上限，不代表 provider token window。
- provider request 在渲染后仍必须检查 actual bytes 与 estimated tokens。
- P0-P4 保护字段无法在 ceiling 内保留时返回 context integrity failure，不得静默压缩 Goal/DoD。

### 12.4 示例

```text
contextWindowTokens       = 128000
providerMaxOutputTokens   = 16384
defaultMaxOutputTokens    = 4096
contextUtilizationRatio   = 0.75
estimatedTokensPerByte    = 0.25
maxRequestBytes           = 4194304

effectiveMaxOutputTokens  = 4096
inputWindowTokens         = 123904
maxInputTokens            = 92928
estimatedInputBytes       = 371712
maxBytes                  = 371712
```

## 13. 默认模型选择

配置默认 target 的确定规则：

1. 配置已有完整 `defaultChat` target：按该 target 校验。
2. provider 是 ai-proxy，model ID 必须出现在当前 catalog 或 exact override 中。
3. 配置没有 default chat model 且 catalog 只有一个 ready model：Configuration UI 可以建议并显式保存该模型。
4. catalog 有多个模型而没有默认模型：配置状态为 `selection_required`。
5. 不允许服务端静默选择排序后的第一个模型。
6. default model 从 catalog 消失后：配置保留原 target，但 readiness 变为 `model_not_found`。
7. 不自动切换到其他模型。

workspace activation 可以显式覆盖 default target。未传 target 时，由 application 使用当前默认 target 补齐后再创建
bootstrap；Orchestrator 和 LLM block 不再解析全局默认值。

## 14. Workspace activation 与持久化

### 14.1 HTTP/Application 命令

替换：

```go
LLMProviderID string
```

为：

```go
LLMTarget *runtimecontract.ProviderModelTarget
```

HTTP JSON：

```json
{
  "goalId": "goal-1",
  "doDId": "dod-1",
  "llmTarget": {
    "providerId": "ai-proxy",
    "modelId": "gpt-4o"
  }
}
```

### 14.2 Activation 处理

1. application 读取请求 target；为空时读取 canonical default chat target。
2. application 通过 EventHub 请求 LLM block 校验 target readiness。
3. readiness 失败时不创建 Run，向 HTTP/CLI 返回具体配置错误。
4. application 将规范化 target 写入 `RuntimeActivationContext.ProviderTarget`。
5. `RuntimeBootstrap.Validate` 要求 provider target 有效。
6. bootstrap 随 Run 创建命令持久化到 Run owner。
7. Start/Resume 不再通过 `context.Context` 携带 provider ID。

### 14.3 RuntimeActivationContext

新增：

```go
type RuntimeActivationContext struct {
    ProviderTarget ProviderModelTarget
    // existing fields...
}
```

它是一次 Run 的 provider/model identity 快照，但不是 C0/C1/C2 stable prefix 内容。需要同步更新：

- `Normalized`
- `Validate`
- `Clone`
- `SameRuntimeActivationContext`
- persistence round-trip tests
- stable prefix tests，证明 target 变化不会改变 provider-neutral stable prefix

## 15. Runtime V2 prerequisite 与 candidate

### 15.1 AcquireProviderCeilingCommand

Orchestrator 自有合同调整为：

```go
type AcquireProviderCeilingCommand struct {
    WorkspaceID         common.WorkspaceID
    RunID               common.RunID
    AuthoritySnapshotID string
    PrerequisiteID      string
    Target              runtimecontract.ProviderModelTarget
    MinimumBytes        int
}
```

处理规则：

- target 必须等于 persisted `facts.Activation.ProviderTarget`。
- LLM block 解析 exact target，生成 `ResolvedProviderModelProfile`。
- 返回完整 `ProviderCeiling`。
- Orchestrator 校验 identity、revision、token ceiling 和 MinimumBytes。
- commit 后 `facts.Resources.ProviderCeiling` 成为本 Run 的唯一 provider capability authority。

不再使用 `RuntimeRequirement.SubjectRef` 存 provider ID。

### 15.2 Candidate 请求

- `GenerateCandidate` 从 `CandidateContext.Resources.ProviderCeiling.Target` 获取 target。
- candidate 从 sealed ceiling 构造 provider request budget/profile，不重新调用 target resolution。
- `completeProvider` 的 Runtime V2 入口接受完整 sealed ceiling，不只接受 target/provider ID。
- provider request 的 model 强制设置为 ceiling target model。
- 如果上层 `ProviderRequest.Model` 非空且与 target 不一致，返回 typed contract failure。
- candidate attempt 0/1 必须使用同一 target、capability revision 和 stable prefix。
- catalog/config 变化不能让 attempt 1 切换模型。
- candidate 不得调用 catalog get/refresh，也不得用当前 catalog descriptor 替换 ceiling 中的 token/byte 值。
- provider client 构造前只允许读取当前 canonical config 中的 endpoint/secret/client 参数，并计算
  `TargetConfigurationRevision` 与 `ceiling.ConfigurationRevision` 比较。
- revision 不一致时返回 `provider_configuration_drift` typed failure；不重算 ceiling、不切换 endpoint/model、
  不使用默认 provider、不自动刷新 catalog。
- request audit 必须直接记录 ceiling target、capability/configuration revision 和 ceiling budget。

### 15.3 Resume 与 restart

- Resume 只携带 workspace/run scope。
- target 和 ceiling 从 persisted aggregate 恢复。
- 已封存 ceiling 的 Run 不重新访问 catalog。
- restart 后若当前 target configuration revision 与 sealed revision 不一致，Run 明确失败并进入治理/重新 activation，
  不能静默按新配置继续。
- 尚未完成 provider prerequisite 的 Run 可以访问当前 catalog；若 target 已消失则明确失败。
- 不允许在 Resume payload 中替换 target。
- 更换模型等同于新的 activation/new Run。

## 16. LLM block EventHub 合同

### 16.1 Catalog 合同

由 `blocks/llm/pkg/events` 定义：

```go
type GetProviderModelCatalogCommand struct {
    ProviderID string
}

type GetProviderModelCatalogResult struct {
    Catalog models.ProviderModelCatalog
}

type RefreshProviderModelCatalogCommand struct {
    ProviderID string
}

type RefreshProviderModelCatalogResult struct {
    Catalog models.ProviderModelCatalog
}
```

`Get` 按 TTL 规则读取并按需刷新，`Refresh` 强制同步刷新。

### 16.2 Target readiness 合同

```go
type ResolveProviderModelTargetCommand struct {
    Target    runtimecontract.ProviderModelTarget
    Operation string
}

type ResolveProviderModelTargetResult struct {
    Profile models.ResolvedProviderModelProfile
}
```

调用方必须使用具体 operation。LLM block 返回的 `Profile.Target` 必须与请求 target 相同，且 profile
必须证明 requested operation 已声明；application readiness 不能只验证 target 格式。

application activation、configuration read model 和 prerequisite adapter 可以使用该能力，但正式 Runtime ceiling 仍由
Orchestrator 自有 prerequisite command/result 封存。

### 16.3 Completion/embedding 合同

替换为：

```go
type CompleteProviderCommand struct {
    Target  runtimecontract.ProviderModelTarget
    Request models.ProviderRequest
}

type GenerateEmbeddingCommand struct {
    Target runtimecontract.ProviderModelTarget
    Input  string
}

type LLMCommandResult struct {
    Target     runtimecontract.ProviderModelTarget
    Completion *ProviderCompletionData
    Embedding  *models.ProviderEmbeddingResult
}
```

`ProviderEmbeddingResult` 同样使用 target，不重复维护易漂移的 provider/model string。

### 16.4 EventHub 约束

- command/result 为具体 Go 类型。
- 不使用通用 Envelope。
- 不使用 `map[string]any` payload。
- 不使用 JSON/reflect bridge。
- `Send` 必须收到具体 response；nil response 视为装配或生命周期错误。
- catalog/resolve lane 使用 query policy；refresh 使用 execute policy；completion/embedding 使用 execute policy。
- LLM module 在 `Start` 订阅，在 `Stop` 逐项取消订阅。

## 17. Provider client 解析

### 17.1 ai-proxy

`ProviderID=ai-proxy` 时：

- endpoint、API Key 和 timeout 来自 `LLM.AIProxy`。
- APISurface 固定为 ai-proxy 当前兼容入口使用的 surface，不由普通用户选择。
- request model 来自 target。
- ai-proxy 内部根据 model 路由上游。
- WorkOrch 不读取或解释 `owned_by` 来选择 SDK adapter。

### 17.2 自定义 provider

`ProviderID!=ai-proxy` 时：

- 必须精确命中 `CustomProviders[ProviderID]`。
- ModelID 必须精确命中该 provider 的 `Models`。
- provider adapter 由显式 `Provider/APISurface` 创建。
- 不回退 ai-proxy，也不回退其他 custom provider。

### 17.3 删除 `selectProviderConfig`

删除“requested -> default -> 任意可用 map item”的选择方式。新入口统一为：

```go
resolveProviderClient(target ProviderModelTarget) (ResolvedProviderClient, error)
```

任何空 target、未知 provider、未知 model 或能力不完整都直接返回具体错误。

## 18. HTTP API 设计

### 18.1 读取配置

保留：

```text
GET /api/v1/workorch/configuration
```

新的 LLM read model：

```go
type LLMConfiguration struct {
    AIProxy         AIProxyConfigurationView
    DefaultChat     runtimecontract.ProviderModelTarget
    DefaultEmbedding *runtimecontract.ProviderModelTarget
    Budget          LLMContextBudgetPolicyView
    CustomProviders []CustomProviderConfigurationView
    Catalog         ProviderCatalogSummary
}
```

`AIProxyConfigurationView` 只能返回 `APIKeyConfigured bool`，不能返回密钥。

### 18.2 更新配置

```text
PUT /api/v1/workorch/configuration/llm
```

请求结构与新配置模型一致；删除旧字段：

- `defaultProviderId`
- `defaultEmbeddingProviderId`
- `modelCapabilities`
- `providers`
- provider templates
- context profile mode 和派生字段
- failover provider 字段

HTTP decoder 保持严格 unknown-field rejection，使旧 payload 明确返回 400。

### 18.3 Catalog API

```text
GET  /api/v1/workorch/configuration/llm/models?providerId=ai-proxy
POST /api/v1/workorch/configuration/llm/models/refresh
```

刷新请求：

```json
{
  "providerId": "ai-proxy"
}
```

响应包含：

- provider ID。
- catalog state/revision/fetchedAt/expiresAt/stale。
- model descriptors。
- 受限、脱敏的 refresh error code/message。

HTTP service 必须实际注册上述两个 route，并通过 application -> typed EventHub -> LLM block 调用。
`GET /configuration` 应通过 `ReadServiceConfigurationWithCatalog` 或等价 use-case 返回当前 catalog，不能固定返回
`state=empty`。上游刷新失败但存在 stale/unavailable catalog summary 时仍返回结构化 summary；只有请求格式、
provider ID 或本地配置非法时才返回 4xx。handler 不直接访问 ai-proxy。

### 18.4 Workspace activation API

删除 `llmProviderId`，新增 `llmTarget`。旧字段因严格 JSON decode 返回 400。

## 19. Web Console 设计

### 19.1 普通配置区

只显示：

- ai-proxy BaseURL。
- API Key 是否已配置及更新输入。
- 连接状态、catalog 状态、最后刷新时间。
- “刷新模型”按钮。
- 默认对话模型下拉框。
- 默认 embedding 模型下拉框。
- 每个模型的 model ID、route owner、context window、max output、readiness。
- 当前自动推导的 max input/max bytes 只读信息。

### 19.2 高级配置区

折叠或独立页显示：

- ai-proxy exact model overrides。
- budget policy。
- custom providers。
- custom provider models 和 operation 声明。
- provider-specific inference/cache 参数。

高级配置更新必须使用 secret-preserving 语义：read DTO 只返回 `apiKeyConfigured`；update 中 API Key 为空表示
保留原值，显式清空必须使用单独 `clearApiKey`。custom provider read-edit-write 不得因为密钥不回显而清空旧密钥。

### 19.3 删除旧 UI

- provider 列表作为主入口。
- provider template 快速新增。
- model capability pattern 编辑器。
- auto/manual context mode。
- 手工编辑派生的 max input/reserved output/bytes hard limit。
- WorkOrch failover provider 编辑。

### 19.4 Workspace activation UI

- 默认显示全局 default chat target。
- 普通模式只显示 ai-proxy ready model。
- “高级”切换后才显示 custom provider/model target。
- 表单提交完整 `llmTarget`。
- target 不 ready 时禁止提交并显示具体原因。
- 普通 chat selector 只展示声明 `chat_completions` 的 ai-proxy ready model。
- embedding selector 只展示声明 `embeddings` 的 ready model，不与 chat selector 共用未过滤 options。

### 19.5 Memory semantic recall 降级合同

`defaultEmbedding` 为空表示未启用语义向量增强，不是 activation 错误。memory 选择顺序固定为：

1. 显式 memory IDs。
2. 配置了可用 embedding target 时执行语义检索。
3. 未配置 embedding，或 embedding/向量检索失败时，回退 workspace memory list 基线选择。

降级时必须保留 typed evidence，记录 `semantic_vector_search` 未执行或失败原因，但不能把“降级已处理”解释为
返回空 memory。语义检索是增强能力，不能使没有 embedding 配置的部署丢失原有 memory 上下文。

## 20. 错误分类

```go
type ProviderModelFailureCode string

const (
    ProviderModelFailureConfigurationInvalid ProviderModelFailureCode = "configuration_invalid"
    ProviderModelFailureCatalogUnavailable    ProviderModelFailureCode = "catalog_unavailable"
    ProviderModelFailureCatalogInvalid        ProviderModelFailureCode = "catalog_invalid"
    ProviderModelFailureSelectionRequired     ProviderModelFailureCode = "selection_required"
    ProviderModelFailureProviderNotFound      ProviderModelFailureCode = "provider_not_found"
    ProviderModelFailureModelNotFound         ProviderModelFailureCode = "model_not_found"
    ProviderModelFailureCapabilityIncomplete  ProviderModelFailureCode = "capability_incomplete"
    ProviderModelFailureOperationUnsupported  ProviderModelFailureCode = "operation_unsupported"
    ProviderModelFailureCeilingInsufficient   ProviderModelFailureCode = "ceiling_insufficient"
    ProviderModelFailureRequestOverBudget     ProviderModelFailureCode = "request_over_budget"
    ProviderModelFailureProviderUnavailable   ProviderModelFailureCode = "provider_unavailable"
    ProviderModelFailureConfigurationDrift    ProviderModelFailureCode = "provider_configuration_drift"
)
```

规则：

- 配置/catalog/selection 错误在 activation 前返回，不创建 Run。
- prerequisite 的 race/ceiling 错误返回 typed failure，由 Orchestrator 投影 Runtime V2 failure。
- provider HTTP/SDK 错误进入现有 candidate failure 分类，但必须携带 target 和 capability revision。
- 错误信息不包含 API Key、完整 Authorization、敏感响应体或带凭据 URL。

## 21. 可观测性与审计

### 21.1 Catalog 指标

建议新增：

```text
workorch_llm_catalog_refresh_total{provider,status}
workorch_llm_catalog_refresh_duration_seconds{provider}
workorch_llm_catalog_models{provider,state}
workorch_llm_catalog_stale{provider}
workorch_llm_catalog_last_success_timestamp_seconds{provider}
```

禁止把 model ID 作为无界 metrics label；模型明细进入 read model/log，不进入高基数指标。

### 21.2 Request audit

每次 provider request 至少记录：

- provider ID。
- model ID。
- capability revision。
- configuration revision。
- context window。
- provider/effective max output。
- max input/max bytes/actual bytes/estimated tokens。
- catalog stale 状态（仅对尚未封存前的解析审计）。
- stablePrefixHash、C0-C5 section audit。

Runtime V2 request audit 的 authority 来源只能是 sealed ceiling。禁止先记录 ceiling revision，再用重新解析的
current profile/client budget 发请求。completion result、usage trace 和 context budget trace 必须关联同一
`ProviderModelTarget`、capability revision 和 configuration revision。

provider/model/baseURL 不参与 stable prefix hash。

### 21.3 Usage/read model

usage trace 和 context budget trace 统一携带 `ProviderModelTarget` 或平铺后的稳定 provider/model 字段，不能只记录 provider。

## 22. 安全约束

- API Key 只存在于 canonical config 和 provider client 内存。
- EventHub command/result 不传 API Key。
- Web read DTO 只返回 `apiKeyConfigured`。
- 日志、metrics、errors、request audit 不记录 Authorization。
- catalog response body 有严格大小上限。
- catalog/provider 错误体脱敏必须使用当前请求的完整 secret 进行替换；只替换 `Bearer ` 前缀或只记录密钥长度
  不满足要求。上游即使原样回显 API Key，也不能进入 error、catalog summary 或日志。
- model ID 和 owned_by 作为外部输入，日志输出前必须限制长度并清理控制字符。
- BaseURL 只允许管理员配置；校验 scheme/userinfo/query/fragment。
- HTTP client 禁止自动把 Authorization 转发到跨 host redirect；建议禁用 redirect 或只允许同 host。

## 23. C0-C5 稳定前缀约束

本次 provider/model 动态绑定不得改变 provider-neutral稳定前缀：

- C0：runtime contract/schema。
- C1：Goal/DoD definition。
- C2：capability/governance/profile declarations。
- C3-C5：current focus、failure、evidence、large refs。

target、endpoint、catalog revision、capability revision 应进入 provider request binding/audit，不能写入 C0/C1/C2 section 内容。

必须新增回归测试：

1. 同一 definition/execution contract 切换 model，`stablePrefixHash` 不变。
2. 同一 authority attempt 0/1，target 和 capability revision 不变。
3. target 改变必须创建新 Run，不能修改旧 Run facts。
4. provider-specific cache marker 只作用于连续 C0/C1/C2。

## 24. 删除清单

### 24.1 配置层

- `LLMConfig.DefaultProvider`
- `LLMConfig.DefaultEmbeddingProvider`
- `LLMConfig.ModelCapabilities`
- `LLMConfig.Providers`
- `LLMModelCapabilityConfig`
- 旧 `LLMProviderConfig`
- `LLMProviderValueSets`
- `ResolveModelCapability`
- model pattern normalize/match helper
- context profile auto/manual parser
- conservative default capability source
- provider failover config

### 24.2 LLM block

- `ProviderConfig.Model` 固定模型语义
- `LoadProviderConfigs(defaultProviderID, valueSets)`
- `selectProviderConfig`
- `completeProvider(ctx, requestedProviderID, ...)`
- `generateEmbedding(ctx, providerID, ...)`
- 基于空 provider 的默认回退
- 基于 map iteration 的任意 provider 选择

保留并重构 provider HTTP/SDK adapter、PromptContext、cache plan、Runtime V2 candidate decoder。

### 24.3 Runtime/application

- `ContextWithRuntimeLLMProviderID`
- `RuntimeLLMProviderIDFromContext`
- `CreateAndStartWorkspaceExecutionRunRequest.LLMProviderID`
- `ActivateWorkspaceExecutionCommand.LLMProviderID`
- HTTP `llmProviderId`
- `AcquireProviderCeilingCommand.ProviderID`
- `ProviderCeiling.ProviderID`
- candidate 仅按 ProviderID 选择配置

### 24.4 API/Web

- 旧 `LLMConfiguration` provider/model capability DTO。
- 旧 `UpdateLLMConfigurationRequest`。
- ConfigurationPage provider 主编辑器和 capability pattern 表单。
- WorkspaceDetailPage 的 provider-only select。
- 旧 API fixture、测试 payload 和文案。

## 25. 代码影响矩阵

| 范围 | 主要落点 | 收口内容 |
| --- | --- | --- |
| ai-proxy catalog | `/home/rangh/aispace/ai-proxy` | `/v1/models` 增加规范化 operations 并同步 revision/校验 |
| canonical config | `internal/pkg/workorchconfig` | 替换 LLM schema、严格校验、删除旧 value-set/pattern 能力 |
| shared runtime contract | `internal/pkg/workorchruntimecontract` | target、activation、ProviderCeiling、clone/normalize/validate |
| common context | `internal/pkg/common/runtime_context.go` | 删除 provider context，只保留 workspace scope |
| LLM models | `internal/modules/blocks/llm/pkg/models` | catalog、target profile、custom provider/client config |
| LLM events | `internal/modules/blocks/llm/pkg/events` | catalog/readiness/target completion/embedding typed 合同 |
| LLM biz | `internal/modules/blocks/llm/biz` | catalog client/cache、能力解析、预算算法、client resolver |
| LLM module | `internal/modules/blocks/llm/module.go` | 新订阅、target handling、prerequisite sealing |
| candidate path | `internal/modules/blocks/llm/runtime_candidate_provider.go` | 只从 sealed ceiling 构造请求并校验 target configuration revision |
| Orchestrator contracts | `internal/modules/kernel/orchestrator/pkg/events` | prerequisite target/ceiling 合同 |
| Orchestrator mainline | `internal/modules/kernel/orchestrator/runtime_v2_*` | 从 facts activation 取 target、完整 ceiling invariant |
| application config | `internal/modules/application/workorchd/biz/appservice/configcore` | 新配置 DTO、EventHub catalog/readiness port |
| activation | `internal/modules/application/workorchd/biz/appservice/activation*` | provider-only 改 target，bootstrap 持久化 |
| memory selection | `activationports/memory_*` | embedding operation 合同与 workspace list 基线降级 |
| HTTP | `internal/modules/application/workorchd/service/server` | 新配置/catalog/refresh/activation API |
| Web types/API | `web/workorch-console/src/types.ts`、`api.ts` | DTO 断代 |
| Web pages | `ConfigurationPage.tsx`、`WorkspaceDetailPage.tsx` | ai-proxy-first 与高级 custom UI |
| shipped config | `config/file`、`config/container-file`、README 示例 | 新 TOML |
| verifier/scripts | `scripts`、Makefile verify targets | 新 payload、catalog/target/live 验证 |
| module docs | LLM/Orchestrator/application readme | 最终合同回填 |

## 26. 分批代码收口顺序

每批都不得新增生产兼容路由。允许分批提交，但最终合并前必须形成单一新主线。

C01-C07 是最初切换顺序，用于说明完整目标范围；2026-07-15 评审后不得把“已有类型或文件”视为该批完成。
后续实际执行顺序以第 31 节 R01-R08 为准，R 批次用于修复已发现的行为缺口并完成最终验收。

### C01：共享 target 与新配置合同

- 新增 `ProviderModelTarget`。
- 替换 `workorchconfig.LLMConfig`。
- 更新 shipped TOML。
- 删除旧 config schema/value-set/pattern tests。
- 增加严格旧字段拒绝测试。

完成条件：仓库只能通过新 LLM TOML 启动。

### C02：ai-proxy catalog client/cache

- 新增 typed wire DTO。
- 新增 catalog descriptor、revision、validation。
- 实现 TTL/LKG/concurrent refresh merge。
- 新增 catalog EventHub command/result。
- 增加 httptest 覆盖大小、超时、重复模型、缺字段和 stale LKG。

完成条件：application 可通过 EventHub 得到稳定 typed catalog，不能直接访问 ai-proxy。

### C03：target resolution 与上下文预算

- 新增 exact override/custom provider resolution。
- 实现唯一预算算法。
- 删除 auto/manual、conservative default、model pattern。
- 新增边界和公式测试。

完成条件：任何可运行 target 都有完整 resolved profile 和 capability revision。

### C04：Runtime V2 target/ceiling 封存

- activation context 持久化 target。
- prerequisite command/result 和 ProviderCeiling 断代。
- candidate 强制使用 ceiling target。
- 删除 provider `context.Context` 传递。
- 更新 aggregate clone/normalize/invariant/restart tests。

完成条件：同一个 Run 的 model target 在 restart/attempt 1/catalog refresh 后均不漂移。

### C05：直接 LLM command 与 provider client 收口

- completion/embedding 改用 target。
- 删除 `selectProviderConfig` 和所有默认 map fallback。
- ai-proxy client 与 custom client 显式解析。
- usage/context audit 改用 target。

完成条件：所有 provider 请求都能追溯到显式 target，不存在空 target 生产调用。

### C06：Application HTTP 与 Web Console

- 新配置 read/update DTO。
- catalog get/refresh API。
- workspace activation `llmTarget`。
- 普通 ai-proxy 配置 UI 和高级 custom UI。
- 删除旧 provider/capability UI。

完成条件：Web 不再发送任何旧 LLM 字段。

### C07：旧路径清零与文档回填

- `rg` 清理旧字段、旧 helper、旧 topic 和旧文案。
- 更新 LLM/Orchestrator/application readme。
- 更新 module architecture、current state、deployment docs。
- 更新 live verifier。

完成条件：代码、配置、API、Web、测试和文档只有新合同。

## 27. 测试设计

### 27.1 Catalog 单元测试

- 正常解析多个模型并稳定排序。
- 缺 context/max output 的模型不可运行但仍可展示。
- case-fold 重复模型拒绝。
- 超过 response bytes/model count 拒绝。
- timeout、401、429、5xx 分类。
- stale LKG 回退。
- max stale 超期后 unavailable。
- 并发 refresh 只产生一个上游请求。
- 不同 cache key 的并发 refresh 不合并、不串结果。
- refresh 期间切换 endpoint/config，旧 call 结果不能写入新 entry。
- config revision 变化使 cache 失效。
- 同长度 API Key 轮换使 catalog connection revision 变化。
- API Key 不出现在错误和 read DTO。
- 上游错误体回显 API Key 时仍完成脱敏。
- `NormalizeLLMConfig` 对 trim 后重复 override/provider/model key 返回错误，不得在 canonicalize 阶段覆盖其中一个值。
- JSON/HTTP update 同时包含 `"custom-a"` 与 `" custom-a "` 时稳定返回 400，不依赖 map 遍历顺序。

### 27.2 能力与预算测试

- catalog 精确命中。
- exact override 覆盖 catalog。
- 未知模型失败。
- custom provider/model 精确命中。
- trim 后重复 provider/model key 拒绝。
- chat/embedding operation 精确校验。
- ai-proxy embedding target 可生成声明 embeddings 的 provider client。
- custom embedding-only model 被 completion 拒绝。
- output 上限取最小值。
- token/byte 公式边界。
- utilization ratio 非法配置失败。
- 不存在 conservative default。

### 27.3 Runtime V2 合同测试

- bootstrap 缺 target 失败。
- target clone/normalize/same。
- prerequisite 返回 target 不一致被拒绝。
- capability revision 为空被拒绝。
- insufficient ceiling 被拒绝。
- candidate 使用 ceiling model。
- request model 与 ceiling model 冲突被拒绝。
- attempt 1 target/revision/stable prefix 不变。
- restart 不重新发现 catalog。
- candidate/restart 不调用 catalog service。
- catalog 删除模型后，已封存 Run 仍使用原 ceiling；configuration revision 变化时明确返回 drift failure。
- current profile 与 sealed ceiling 不一致时不发送 provider request。
- Resume 无 provider/model patch。
- 成功 provider request 的 candidate result/audit record 携带 request/audit ID、sealed target、capability/configuration revision、budget、ProviderSubmitted 和 usage。
- provider 返回失败时仍产生 submitted/status/failure audit，不能只返回 failure 并丢弃 request metadata。
- event adapter 与 owner persistence round-trip 后仍能读取同一 audit ID、target/revision 和 budget。

### 27.4 Application/API 测试

- 配置 read 脱敏。
- PUT 新配置成功。
- 旧 payload unknown field 返回 400。
- catalog get/refresh 通过 EventHub。
- `/configuration` 返回 EventHub catalog，而不是固定 `empty`。
- refresh 失败返回受限 stale/unavailable summary。
- activation 默认 target 补齐。
- activation 显式 custom target。
- readiness 返回不同 target 时拒绝。
- readiness operation 不匹配时拒绝。
- defaultEmbedding 为空时回退 workspace memory list。
- embedding 调用失败时回退 workspace memory list 并保留 degraded evidence。
- target not ready 时不创建 Run。
- HTTP activation 不接受 `llmProviderId`。
- catalog get 与 refresh 使用各自具体 command/result 分支，测试中不允许通用 `any` payload/type switch。
- clearAIProxyApiKey 或 clearCustomProviderApiKeys 与同一 secret 的非空新值同时出现时返回 400。
- custom provider clear 只清除指定 provider，未知 provider ID 返回明确参数错误。

### 27.5 Web 测试

- ai-proxy 模型加载/刷新状态。
- 默认模型选择。
- chat/embedding options 按 operation 分离。
- capability incomplete 禁选。
- advanced custom provider 编辑。
- custom provider secret 留空时保留原密钥。
- activation 提交完整 target。
- API Key 不回显。
- 新建 custom model 默认 operations 为空，保存前必须由用户显式选择 operation。
- custom provider 提供独立“清空 API Key”操作，并提交 `clearCustomProviderApiKeys`。
- clear 与非空新 secret 冲突时前端阻止提交，并保留后端负向测试。
- Web helper 测试证明不会自动补 `chat_completions`。

## 28. 验证命令

实施阶段按风险从小到大执行：

```bash
go test ./internal/pkg/workorchconfig -count=1
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
make fmt-check
make vet
make test
make verify-boundary-contract
npm --prefix web/workorch-console test --if-present
npm --prefix web/workorch-console run build
rg -n "defaultProviderId|defaultEmbeddingProviderId|contextProfile|llmProviderId|\"providers\"" \
  scripts/loadtest-governed-workspace-concurrency.py
rg -n "var payload any|map\[string\]any" \
  internal/modules/application/workorchd/biz/appservice/configcore/llm_catalog.go \
  internal/modules/application/workorchd/biz/appservice/readmodelcore/llm_usage_types.go \
  internal/modules/application/workorchd/biz/appservice/readmodelcore/llm_trace_types.go \
  internal/modules/application/workorchd/biz/appservice/readmodelprojection/llm_context_budget_*.go
```

最后两组静态扫描的期望结果为无生产代码命中。历史 archive、明确的旧 payload 拒绝 fixture 和非 LLM 的
Web Search `defaultProviderId` 不属于本次命中范围。

live 验证至少覆盖：

1. WorkOrch 连接 ai-proxy 并列出 catalog。
2. UI 选择不同 context window 的两个模型。
3. 两次新 Run 分别封存不同 target/ceiling。
4. 第一个 Run 启动后刷新/修改 catalog，其 ceiling 不变化。
5. ai-proxy 暂时不可用时已有 Run 可恢复，新 activation 使用有效 LKG 或明确失败。
6. 模型从 catalog 移除后旧 Run 不切换，新 Run 不可使用该模型。
7. request audit 的 stablePrefixHash 不因 model 切换意外变化。
8. ai-proxy chat model 与 embedding model 分别通过 operation readiness 和真实请求。
9. 修改 API Key/endpoint/budget/custom model 后，新 Run 使用新 revision，旧 Run 返回明确 drift failure。
10. 未配置 defaultEmbedding 时，workspace activation 仍能回显基线 memory。

### 28.1 2026-07-15 评审验证基线

WorkOrch 已通过：

- `go test ./internal/pkg/workorchconfig -count=1`
- `go test ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/blocks/llm/... -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice/configcore -count=1`
- `go test ./internal/modules/application/workorchd/biz/appservice/activationports -count=1`
- `go test ./internal/modules/application/workorchd/service/server -count=1`
- `make verify-boundary-contract`
- `npm --prefix web/workorch-console test --if-present`
- `npm --prefix web/workorch-console run build`
- `git diff --check`

WorkOrch 未通过：

- `go test ./internal/modules/application/workorchd/... -count=1`：appservice 根包后台 Runtime V2 drive 在
  config manager/owner runtime 关闭后仍访问 EventHub 或临时目录，出现 `missing observer`、
  `config manager is closed` 和 `TempDir RemoveAll ... directory not empty`；单独重跑根包仍可复现。
- `make fmt-check`：当前多个 Go 文件未通过 gofmt 检查。
- `make vet`：TUI 测试仍引用不存在的 `AppModel.runtimeEvents` 字段。

ai-proxy 已通过：

- `go test ./internal/config -count=1`
- `go test ./internal/proxy -count=1`
- `go test ./cmd/ai-proxy -count=1`
- `go test ./... -count=1`
- `go vet ./...`
- 全仓 Go gofmt 检查
- `git diff --check`

仍未闭环：

- WorkOrch request audit/usage 生产链路专项测试。
- exact override operation、完整 secret 脱敏、canonical map key、unknown/duplicate operation boundary 测试。
- Web operation 无回退和 API Key 独立清空测试。
- WorkOrch 与 ai-proxy catalog refresh、chat、embedding、negative operation 和 Runtime V2 restart live 联调。

该基线只用于确认收口起点，不得作为完成证据。每个 R 批次完成后必须记录新的聚焦测试结果。

### 28.2 2026-07-15 二次评审验证基线

二次评审实际执行结果：

已通过：

- `go test ./internal/pkg/workorchconfig -count=1`
- `go test ./internal/pkg/workorchruntimecontract -count=1`
- `go test ./internal/modules/blocks/llm/... -count=1`
- `go test ./internal/modules/kernel/orchestrator/... -count=1`
- ai-proxy `go test ./... -count=1` 与 `go vet ./...`
- WorkOrch `make fmt-check`、`make vet`、`make verify-boundary-contract`
- `npm --prefix web/workorch-console test --if-present`
- `npm --prefix web/workorch-console run build`
- WorkOrch/ai-proxy `git diff --check`

未通过：

- `go test ./internal/modules/application/workorchd/... -count=1` 在 10 分钟后 timeout；阻塞测试为
  `TestBatchResolveApprovalsWithBackendApprovesInRequestOrder`，调用栈停在 appservice session cleanup、
  `runtimeHub.Terminate`、workspace module `Stop` 和 EventHub `Unsubscribe`。
- 上述失败测试单独运行通过，说明不是稳定的业务断言失败，而是全套测试顺序、background routine 或 shutdown
  生命周期竞态；这仍属于必须修复的完成门禁。
- loadtest 旧 LLM 字段静态扫描有生产脚本命中。
- catalog application `any` 与 LLM usage/context budget `map[string]any` 静态扫描有生产命中。

由于 required package 已失败，本轮不再把 `make test` 视为通过，也不进入 live 验证。

## 29. 部署切换要求

由于不提供兼容性，部署必须作为维护窗口切换：

1. 停止创建新 Run。
2. 记录并处理所有 pending/active/blocked Run。
3. 旧 Run 不迁移；需要继续的 workspace 在新版本部署后重新 activation。
4. 备份旧 application TOML，但不由新版本读取。
5. 写入新 LLM TOML，至少配置 ai-proxy BaseURL 和 default chat target。
6. 部署 ai-proxy，确认 `/v1/models` 中目标模型容量字段和 `operations` 完整。
7. 部署 WorkOrch clean build。
8. 调用 catalog refresh API，确认 ready model 和 revision。
9. 创建全新 Runtime V2 Run 执行 live 验证。
10. 不通过恢复旧 provider 配置、inline decision payload 或 compatibility flag 绕过失败。

## 30. 完成验收清单

### 配置与目录

- [x] ai-proxy 是内置首选 provider。
- [x] 普通配置无需创建 provider item。
- [x] catalog HTTP get/refresh route 已注册且 Web 实际可调用。
- [x] `/configuration` 返回 EventHub catalog，不固定为 `empty`。
- [x] catalog 有 TTL、LKG、timeout、大小限制和按 cache key 的并发合并。
- [x] API Key 任意变化都使 catalog/target revision 变化。
- [x] descriptor、exact override 和 capability revision 均包含规范化 operation。
- [x] catalog unknown/duplicate operation 按合同标记 unavailable，不静默兼容或去重为 ready。
- [x] 缺容量或缺 requested operation 的模型在 catalog、override、custom 三条路径均不可进入新 Run。
- [x] provider/model/override 配置 map key 已 canonicalize，且 trim 后重复 key 在覆盖前被稳定拒绝；readiness、revision 和 client lookup 使用同一 key。（RR03）
- [x] `CatalogConnectionRevision` 只包含连接/catalog/auth，不包含 exact model override。

### Runtime V2

- [x] activation 持久化完整 provider/model target。
- [x] 不再使用 provider context value。
- [x] ProviderCeiling 封存 target 和 token/byte ceiling。
- [x] candidate 强制设置 ceiling model。
- [x] candidate 只使用 sealed ceiling，不重新解析 catalog/current profile。
- [x] attempt 1/restart 不访问 catalog。
- [x] configuration revision 漂移时返回 `provider_configuration_drift` typed failure，不静默使用新 endpoint/client profile。
- [x] restart/resume/catalog refresh 不改变已封存 target、capability revision 和 budget。
- [x] Runtime core 不包含 ai-proxy/provider-specific 策略。

### 组件合同

- [x] catalog/readiness/completion/embedding 使用具体 EventHub command/result。
- [x] application 不直接请求 ai-proxy。
- [x] readiness request/response 均携带 operation，并验证 response target、operation 与 request 一致。
- [x] completion/embedding 在 catalog、override、custom 三条路径均于 provider 请求前强制校验 operation。
- [x] catalog HTTP handler 只通过 application/EventHub 访问 LLM block。
- [x] cutover 新增主链使用 typed EventHub/target/ceiling/evidence；catalog application dispatch 已拆分具体 get/refresh send；usage 生产主链经 typed ProviderRequestAudit 与 `/run/llm_usage/recorded` 关联，且 read-model/port/client 不重新暴露 map authority。（R09）

### 不兼容清理

- [x] 旧 LLM TOML 字段清零。
- [x] 旧 HTTP DTO/JSON 字段清零。
- [x] production/test fixture 中旧 `[llm.providers.*]` 全部清零。
- [x] model pattern registry 生产代码与测试清零。
- [x] auto/manual context profile 生产代码与测试清零。
- [x] conservative default 生产代码与测试清零。
- [x] `selectProviderConfig` 与 map fallback 清零。
- [x] WorkOrch provider failover 清零。
- [x] provider-only activation 清零。
- [x] 所有 live verifier/loadtest 不再发送 `llmProviderId`、`defaultProviderId`、`defaultEmbeddingProviderId`、旧 `providers` 或旧 context profile。（RR05）
- [x] README、部署文档和示例不再描述 `defaultEmbeddingProvider`、旧 providers 或旧 context profile。

### 产品与验证

- [x] Web Console 默认展示 ai-proxy 模型选择。
- [x] custom provider 只在高级配置出现。
- [x] workspace activation 高级模式可选择显式 custom target。
- [x] chat/embedding 下拉严格按 operation 分离；operations 为空时不展示，新建 custom model 也不自动补默认 operation。（RR06）
- [x] custom provider API Key 留空更新时保留旧密钥。
- [x] ai-proxy/custom provider API Key 可通过 Web 独立 clear 操作显式清空；clear 与非空新值冲突时前后端均拒绝。（RR06）
- [x] defaultEmbedding 为空或 embedding 失败时保留 workspace memory 基线选择。
- [x] semantic degraded evidence 使用具体类型 MemorySelectionEvidence，不以 `map[string]any` 作为组件合同。
- [x] 每次 Runtime V2 provider request 生产并持久化 target/revision/budget/usage audit 字段；成功与失败请求均有 ProviderSubmitted/status/ActualTokens 或 failure audit。（RR01）
- [x] completion/usage/context budget 通过 request/audit ID 和 typed sealed target/revision 关联，不依赖局部 request mutation、测试 fixture 或 map payload 重建；历史 event payload 解码仅限单一投影适配边界。（RR01/RR04/R09）
- [x] catalog/provider 错误体使用当前完整 secret 脱敏，裸 secret 不进入 error/catalog summary/HTTP/Web。
- [x] C0/C1/C2 stable prefix 回归通过。
- [x] catalog/target resolution/restart/operation/security/audit 聚焦单元测试已补齐，并覆盖 RR01-RR06 负向场景。
- [x] `make fmt-check`、`make vet` 通过；appservice/server 全包连续两次无 suite-order/shutdown timeout。（R10：server SSE test contract 已同步）
- [x] 全量 Go（`make test`）、Web build、`make verify-boundary-contract` 通过。（R10：`make test` 已连续两次通过）
- [x] live 场景（catalog/chat/embedding/restart）实跑回填。（R11：见
  `scripts/verify-ai-proxy-primary-llm-provider-live.sh` 与 `docs/60-verification/artifacts/ai-proxy-primary-llm-live-runtime-20260715T150944Z/`）

## 31. 评审后逐项收口计划

必须按顺序处理；前一批未通过对应聚焦测试时，不进入后一批。每批只建立一条新主线，不新增兼容分支。

### R01：Runtime V2 sealed ceiling 执行主线

当前状态：**主体、drift typed failure 与 RR01 audit 生产链路已落地；仍须 live 联调确认端到端 audit 还原。**

涉及：

- `internal/modules/blocks/llm/runtime_candidate_provider.go`
- `internal/modules/blocks/llm/module.go`
- `internal/modules/blocks/llm/biz/target_resolution.go`
- `internal/pkg/workorchruntimecontract`

要求：

1. candidate Runtime V2 path 接收并使用完整 sealed ceiling。
2. 从 ceiling 构造请求预算和 profile，不调用 `resolveTarget`/catalog。
3. 当前 canonical config 只用于 endpoint/secret/client 构造。
4. 计算完整 `TargetConfigurationRevision` 并与 ceiling 比较；不一致返回 typed drift failure。
5. attempt 0/1/restart 保持 target、capability revision、configuration revision、stable prefix 不变。
6. 增加 `ProviderModelFailureConfigurationDrift = "provider_configuration_drift"`，禁止使用
   `configuration_invalid` 代替 sealed configuration drift。
7. provider 调用结果不能只返回 candidate payload；必须向 R07 提供本次 sealed request 与 completion/usage
   的具体 typed audit 输入。

完成条件：删除模型或刷新 catalog 不影响已封存 Run；修改 target config 后旧 Run 返回稳定 typed drift failure；
candidate 调用方可以获得并关联 sealed request audit，而不是丢弃 completion metadata。

### R02：operation-aware catalog 与 provider 执行

当前状态：**operation authority 主体已落地；Web 新建 custom model 不再自动补 operation。仍须 live 联调。**

涉及：

- `/home/rangh/aispace/ai-proxy` `/v1/models` DTO 与 model catalog
- `internal/modules/blocks/llm/pkg/models`
- `internal/modules/blocks/llm/pkg/events`
- `internal/modules/blocks/llm/biz/target_resolution.go`
- completion/embedding provider path

要求：

1. ai-proxy catalog 明确返回规范化 `operations`。
2. `AIProxyModelOverrideConfig` 增加必填 `Operations []string`；override 只能修正容量，不能绕过
   catalog/operation authority。
3. descriptor、override、capability revision 均包含规范化 operations 和 operation count。
4. `ResolvedProviderModelProfile` 增加 `Operation`；resolve/readiness command/result 均携带 requested
   operation，consumer 校验 response target、operation 和 revision。
5. WorkOrch catalog boundary 对 operations 执行严格校验：空、未知、重复均使 descriptor
   `CapabilityReady=false`，不得接受 alias 或静默去重为 ready。
6. Web 删除 operations 为空仍展示、custom operations 为空自动补 `chat_completions` 的回退。
7. completion 只接受 `chat_completions`，embedding 只接受 `embeddings`。
8. UI chat/embedding options 使用不同过滤结果，target 不 ready 时禁止提交。

完成条件：catalog、exact override、custom provider 三条路径都不能在 operation 未声明时进入新 Run 或发起
provider request；ai-proxy embedding 可用；embedding-only model 不能用于 completion；错误统一为
`operation_unsupported`。

### R03：catalog cache 与 revision 隔离

当前状态：**RR03 已收口。canonicalize 在写入前检测 trim/case-fold 重复并 fail-fast；readiness/client 使用 exact map lookup。**

涉及：

- `internal/modules/blocks/llm/biz/catalog_service.go`
- `internal/pkg/workorchconfig`

要求：

1. 拆分 `CatalogConnectionRevision` 与 `TargetConfigurationRevision`。
2. API Key 使用域隔离 SHA-256 fingerprint 参与最终 revision。
3. cache entry/LKG/inflight 按 cache key 管理，不使用全局单一 inflight。
4. refresh 完成时校验 call key，禁止旧 endpoint 结果污染新 entry。
5. trim 后重复 provider/model/override key 在配置校验阶段拒绝，并在 normalize 阶段重建 canonical map；
   readiness、`TargetConfigurationRevision` 和 `ResolveProviderClient` 只能使用 canonical exact lookup。
6. `CatalogConnectionRevision` 删除 `ModelOverrides`，仅包含 canonical endpoint、catalog policy 和
   catalog auth fingerprint。
7. catalog/capability revision 使用明确的 model count、operation count 和长度前缀编码；不以拼接字符串代替
   operation 集合边界。

完成条件：同 key 并发只请求一次；不同 key 不串行、不串结果；同长度密钥轮换立即失效；带空格 key
在 readiness、revision、client 构造中不存在行为分叉；override 变化不使 catalog cache/LKG 无意义失效。

### R04：Application HTTP 与 catalog read model

当前状态：**RR04 catalog get/refresh 已拆成具体 typed send；生产 audit 主链 typed。read model 仍可从 event payload 投影。**

涉及：

- `internal/modules/application/workorchd/service/server/http_configuration.go`
- `internal/modules/application/workorchd/biz/appservice/configcore/llm_catalog.go`
- configuration service tests

要求：

1. 注册 catalog GET/refresh route。
2. handler 通过 application use-case 和 EventHub 调用 LLM block。
3. `/configuration` 使用 catalog-aware read path。
4. stale/unavailable 作为结构化 catalog summary 返回，错误信息受限且脱敏。
5. readiness 校验 response target、operation 和 profile revision 完整性。
6. `configcore/llm_catalog.go` 将 get/refresh 拆成两个具体 command send 分支，删除 `var payload any` 和
   通用 response type switch。
7. catalog client/error projection 必须持有当前请求 secret，并在错误体进入 `ProviderModelCatalog` 前按完整
   secret 替换；同时限制 model ID、route owner 和 URL 摘要长度、清理控制字符。
8. 增加上游错误体分别以裸 API Key、`Bearer <key>` 和结构化 JSON 回显 secret 的回归测试。

完成条件：Configuration 页面首次读取可看到目录；刷新按钮不再 404；错配 readiness 无法创建 Run；
新增 catalog application 主链无 `any` dispatch；任何上游错误体都不能把当前 API Key 投影到 HTTP/Web。

### R05：Memory 降级与 activation 正确性

当前状态：**RR02 主体已收口。session 关闭顺序为 module Stop → hub Terminate → background drain；appservice 全包连续两次通过。**

涉及：

- `activationports/memory_selection.go`
- `activationports/memory_semantic_selection.go`
- `workspace_execution_defaults.go`
- activation tests

要求：

1. defaultEmbedding 为空不返回配置错误。
2. 未启用或失败的语义检索回退 workspace list。
3. degraded evidence 与实际回退结果同时保留。
4. 将 semantic recall/degraded/fallback evidence 定义为具体类型，禁止在 activation/runtime 合同中使用
   `map[string]any` 拼装和读取。
5. 修复后台 Runtime V2 drive、config manager、EventHub observer 和临时 repository 的关闭顺序，确保测试
   cleanup 前所有 background routine 已完成或取消。
6. 稳定通过 activation 并发读取、system default soul 和 session cleanup 测试。

完成条件：没有 embedding 配置时 activation 仍加载基线 memory；degraded evidence 可由具体类型读取；
appservice 根包连续执行不再出现 `missing observer`、`config manager is closed` 或 TempDir cleanup 失败。

### R06：Web 高级配置与 secret-preserving update

当前状态：**RR06 已收口。custom model operations 默认空；custom clear UI/请求已加；clear 与非空新值前后端 fail-fast；activation 未 ready 禁用提交。**

涉及：

- `web/workorch-console/src/pages/ConfigurationPage.tsx`
- `web/workorch-console/src/pages/WorkspaceDetailPage.tsx`
- TypeScript request/read DTO
- `configcore/llm_update.go`

要求：

1. 增加 custom provider/model/operation/inference/cache 高级配置。
2. activation 高级模式可选择 custom target。
3. API Key 空值保留旧值；为 ai-proxy 增加 `clearApiKey`，为 custom provider 增加逐 provider 的独立
   `clearApiKey`，且 clear 与非空新值同时出现时返回参数错误。
4. 普通模式继续只展示 ai-proxy ready chat model；operations 为空或不匹配时不展示。
5. custom model operations 必须由用户显式声明；Web 不补默认 `chat_completions`。
6. activation selector 对 default/显式 target 均展示 readiness，不 ready 时在前端禁用提交，同时保留后端
   fail-fast 校验。

完成条件：Web 可完整管理高级配置；read-edit-write 不清空 secret；管理员可以显式清空 secret；Web 不再
猜测 model operation，也不能提交不 ready target。

### R07：Request audit、usage 与安全

当前状态：**RR01 主体已收口。`GenerateCandidate` 返回 typed result；成功/失败 audit 进入 `RequestRuntimeCandidateResult` 并经 `/run/llm_usage/recorded` 持久化。仍须 live 联调确认端到端 audit 还原。**

涉及：

- `internal/modules/blocks/llm/runtime_candidate_provider.go`
- `internal/modules/blocks/llm/pkg/models/decision.go`
- `internal/modules/blocks/llm/module.go`
- Runtime V2 candidate adapter/drive 调用链
- Run/eventstream owner 的 typed usage/audit command
- `readmodelcore`、`readmodelprojection`、`workorchport`、`workorchclient`

要求：

1. provider request audit 携带 target、capability/configuration revision 和 sealed budget。
2. usage/context trace 同时携带 provider/model/revision，不只记录 provider。
3. stablePrefixHash 不包含 provider/model/baseURL/revision。
4. catalog/provider 错误体使用完整 secret 脱敏，限制 model ID/route owner 日志长度并清理控制字符。
5. Runtime V2 provider 调用返回具体结果类型，至少包含 payload、usage、sealed request audit 和执行状态；
   不再只返回 `[]byte` 并丢弃 metadata。
6. 在 provider request 实际发送前/后设置可审计的 submitted 状态，禁止保留永远不赋值的
   `ProviderSubmitted` 字段。
7. 由明确 owner 生产并持久化 typed LLM usage/audit 记录；read model 不再依赖不存在的
   `/run/llm_usage/recorded` 外部假设或测试 fixture。
8. context budget、cache plan、cache usage/markers 使用具体 DTO；删除本次主链中的 `map[string]any` 投影和
   JSON round-trip 重建。
9. completion result、usage trace、context budget trace 使用同一 request/audit ID 与 target/revision 关联。

完成条件：一次真实 Runtime V2 provider request 可以从持久化 audit 还原其 sealed target、revision、预算、
stable prefix 和 usage；失败请求也有明确 submitted/status 记录；任何 audit、error、HTTP 或 Web DTO 均不能
获得 API Key、Authorization 或 fingerprint。

### R08：旧路径清零与验证闭环

当前状态：**loadtest 已切到新 LLM 合同；appservice 全包连续两次通过；session 关闭顺序已修复。仍须全量 `make test`、Web build、boundary 与 live 验证后改 code-closed。**

要求：

1. 删除旧 provider registry、`ContextProfileMode`、model registry、conservative default 代码和测试。
2. 重写 `scripts/loadtest-governed-workspace-concurrency.py` 的 provider 配置、选择、timeout 和 restore
   流程，统一使用 `aiProxy`、`defaultChat`、`defaultEmbedding`、`customProviders`、catalog 与
   activation `llmTarget`。
3. 清理 README、部署文档、示例和脚本中的 `defaultEmbeddingProvider`、`defaultProviderId`、旧
   `providers`、旧 context/inference profile；拒绝测试和 strip blacklist 可保留，但必须明确为负向合同。
4. 清理新增主链中的 `any`/map/JSON bridge；不得以外部协议兼容或未知 topic 回退为理由保留。
5. 补齐 catalog、target resolution、operation、restart、stable prefix、security、audit、HTTP/Web 测试。
6. 修复并通过 `make fmt-check`、`make vet`、application 全包和 `make test`。
7. 依次通过第 28 节命令和 live 场景。
8. 只有第 30 节全部勾选后，才将本文状态改为 `code-closed`。

完成条件：代码、配置、API、Web、测试、脚本和文档只剩唯一新合同；静态扫描、格式、vet、全量测试、
boundary 和 live 证据全部通过。

### 31.9 二次评审后的执行顺序

从当前代码状态继续收口时，按以下顺序执行：

1. **RR01 / R07 audit owner 主链**：把 candidate provider 调用结果改为具体类型，携带 request/audit ID、
   sealed target/revision/budget、usage 和 status；由明确 owner 持久化成功与失败 audit。
2. **RR03 / R03 config authority**：在 canonicalize 写入新 map 前检测 trim/case-fold 重复；删除 readiness/client
   的 trimmed fallback lookup，并补 HTTP 负向测试。
3. **RR04 / R04 typed closure**：拆分 catalog get/refresh 具体 send 分支；把本次 LLM usage/context budget 主链
   的 identity、profile、cache plan 和 usage 改为具体 DTO。
4. **RR06 / R02/R06 Web 合同**：新 custom model operations 默认空；增加 custom API Key clear UI/请求；
   clear 与非空新 secret 冲突前后端 fail-fast。
5. **RR05 / R08 脚本清零**：重写 loadtest 配置保存、mock/real target 选择、timeout 和 restore，统一使用
   `aiProxy/defaultChat/defaultEmbedding/customProviders/llmTarget`。
6. **RR02 / R05/R08 生命周期门禁**：修复全套测试中的 background routine、module Stop、EventHub unsubscribe
   和 session cleanup 顺序；至少连续两次通过 application 全包，再执行 `make test`。
7. **最终验证**：重新执行第 28 节全部命令和静态扫描；全部通过后才进入 catalog/chat/embedding/restart live
   场景，并回填可复核 artifact。

任何前置步骤未完成时，不得把顶部 Note 改回“仅待 live”，也不得把第 30 节对应条目重新勾选。

## 32. 最终架构结论

收口完成后的唯一生产语义是：

> WorkOrch 默认连接一个内置 ai-proxy provider endpoint，由 `blocks/llm` 通过
> `/v1/models` 发现精确模型容量和 operation；application 在 activation 阶段解析出显式
> provider/model target；Runtime V2 通过 typed prerequisite 封存该 target 的能力 ceiling；
> 后续 candidate、completion、restart 和 audit 只使用已封存 target/ceiling，并在发送请求前验证
> sealed configuration revision；embedding 使用显式 embedding target 和 operation 合同。任何路径都不再从
> 默认 provider、model pattern、当前 catalog、进程 context 或 map fallback 重建旧 Run authority。
> 自定义 provider 只作为显式高级能力存在。
