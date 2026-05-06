# Workspace Web Search Capability Plan

## 背景

本文档用于跟踪 workorch 面向需要外部资料、实时信息和网页证据的任务补齐
Web Search 能力的需求、设计边界、任务状态和验收标准。

目标不是让 LLM 自由联网，也不是把 `curl` / shell 作为默认搜索路径，而是把
Web Search 纳入现有 `Run / Goal / DoD / Tool / Ticket / Approval /
Artifact / Memory` 治理闭环：

- 搜索和网页读取都作为受治理的 runtime block 执行。
- 每次搜索、读取、引用都必须产生可追溯证据。
- 默认只允许只读公网查询；内网、登录态、敏感域名、大批量抓取必须进入
  Approval。
- 最终 Result 和 Artifact 必须引用被采用的信息来源、读取时间和证据步骤。
- Memory 只写入最终采用的事实摘要和来源引用，不写入原始大网页正文。

## 目标

### 用户目标

用户只需要提交 `Goal` 和 `DoD`，workorch 能够在需要外部信息时自动：

- 判断是否需要 Web Search。
- 执行受控搜索并获得候选来源。
- 读取必要 URL 的正文或摘要。
- 基于来源证据完成分析、验证和最终回答。
- 在 Result / Artifact 中输出事实、来源、检索时间和证据引用。
- 将可复用结论按策略写入 Memory。

### 系统目标

- Web Search 能力必须通过统一 runtime block 暴露给 orchestrator 和 LLM。
- 第一版 provider 直接实现纯 HTTP client，对接一个标准 HTTP 搜索/读取网关。
- 后续 provider 扩展不得影响上层 step kind、read model、Artifact 和 UI 展示。
- 所有 HTTP 请求必须受 timeout、结果数、响应大小、域名策略和重定向策略限制。
- 搜索结果不能直接作为最终事实；最终事实必须引用搜索结果或 `web/fetch`
  产生的证据。
- Web/TUI/CLI 显示都必须复用 appservice read model，不复制搜索结果解析逻辑。
- Web Search 配置复用 workorch 整体配置框架，不引入独立配置文件、独立环境变量
  解析链路或独立热加载机制。

## 非目标

- 不在第一版直接接入多个商业搜索 provider。
- 不在第一版实现浏览器自动化、登录态网页操作或 JavaScript 渲染。
- 不在第一版支持任意网页爬虫、站点镜像或批量抓取。
- 不让 LLM 直接生成未受治理的 HTTP 请求。
- 不把 LLM provider 内置联网能力作为主路径。
- 不把原始网页全文无条件写入 Artifact 或 Memory。

## 当前基线

已存在或可复用能力：

- `Run / Goal / DoD` 编排和验收模型。
- `executionGraph` 工具调用模型。
- runtime registry 和 step evidence 记录。
- `Decision Ticket` 和 `Approval` 治理流程。
- `Result / Artifact` 交付物生成。
- `Memory` 写入、召回和 traceability。
- `MCPServer` / `MCPCapability` 能力管理模型。
- Web/TUI/CLI 对 Run、Artifact、Memory、Ticket、Approval 的基础展示。

待补齐能力：

- 正式 Web runtime block：`web/search`、`web/fetch`。
- HTTP provider 配置、调用、响应标准化和错误分类。
- 搜索/读取证据事件、read model 投影和 UI 展示。
- LLM runtimeCapabilities 注入和 soul / skill / workspaceTemplate 能力约束。
- Approval 策略：敏感域名、内网地址、登录态、大响应、大批量查询。
- Artifact / Memory 写入策略和 e2e 验收脚本。

## 架构边界

### 内置 block 边界

新增内置 block：

- `internal/modules/blocks/web`
  - `web/search`
  - `web/fetch`

要求：

- block 只负责 provider 调用、安全校验、响应标准化和结构化输出。
- block 不理解具体业务目标，不做最终事实判断。
- block 不直接写 Memory 或 Artifact。
- block 不绕过 orchestrator 创建 step evidence。
- block 不直接依赖 Web/TUI/CLI 展示格式。

### Provider 边界

第一版只实现纯 HTTP client provider：

- `provider`: `http`
- 由 workorch 调用外部 HTTP 搜索/读取网关。
- 外部网关负责真实搜索源、反爬处理、网页正文抽取、商业搜索 API 适配。
- workorch 只要求网关返回标准 JSON 结构。

后续可扩展 provider：

- `searxng`
- `brave`
- `bing`
- `google_custom_search`
- `mcp`
- `custom_http`

扩展要求：

- 新 provider 只能新增 adapter，不改变 `web/search` / `web/fetch` step kind。
- 上层 LLM 决策、orchestrator、read model、Artifact、TUI/CLI/Web 展示不感知
  provider 差异。

### Orchestrator 边界

- 继续负责编排 `executionGraph`。
- 不承载搜索 provider 细节。
- 工具结果必须回流给 LLM step，再由 LLM 决定继续 fetch、补充搜索、验证或
  完成 Result。
- 不允许 orchestrator 根据关键词硬编码触发搜索。

### AppService / Read Model 边界

- appservice 负责把搜索查询、读取 URL、来源摘要、证据引用投影到
  RunSnapshot / RunObservability / WorkspaceSnapshot。
- Web/TUI/CLI 只展示读模型。
- TUI 默认只展示最终来源摘要和关键引用；详细搜索过程留在 observability /
  Artifact。

## 能力模型

### web/search

输入建议：

- `query`: 搜索关键词，必填。
- `locale`: 可选，例如 `zh-CN`、`en-US`。
- `recency`: 可选，例如 `day`、`week`、`month`、`year`。
- `domains`: 可选，限定域名列表。
- `excludeDomains`: 可选，排除域名列表。
- `maxResults`: 可选，默认取 provider 配置。
- `safeSearch`: 可选，默认启用。
- `reason`: 可选，说明本次搜索用于哪个 Goal / DoD 判断。

输出建议：

- `status`: `completed` / `failed`。
- `stage`: `web_search`。
- `provider`
- `query`
- `searchedAt`
- `results`
  - `title`
  - `url`
  - `snippet`
  - `source`
  - `publishedAt`
  - `rank`
  - `contentHash`
- `resultCount`
- `truncated`
- `policy`
- `errorCode`
- `error`

规则：

- 搜索查询必须非空。
- 返回结果必须有数量上限。
- 每个结果必须有 URL，且 URL 必须通过安全校验。
- 搜索结果只表示候选来源，不等同于已验证事实。
- 若 Goal 明确需要最新信息，LLM 应优先使用 `web/search`，并在最终结果中标注
  检索时间。

### web/fetch

输入建议：

- `url`: 必填。
- `maxBytes`: 可选，默认取 provider 配置。
- `extractMode`: 可选，`text` / `metadata` / `summary`。
- `allowedContentTypes`: 可选。
- `reason`: 可选，说明读取该 URL 的目的。

输出建议：

- `status`: `completed` / `failed`。
- `stage`: `web_fetch`。
- `provider`
- `url`
- `finalUrl`
- `title`
- `content`
- `summary`
- `contentType`
- `contentHash`
- `retrievedAt`
- `publishedAt`
- `source`
- `size`
- `truncated`
- `redirects`
- `policy`
- `approvalRequired`
- `approvalAction`
- `approvalContext`
- `approvalId`
- `errorCode`
- `error`

规则：

- URL 必须是 `http` 或 `https`。
- 禁止默认访问 localhost、loopback、link-local、private CIDR、metadata endpoint。
- 默认限制重定向次数、响应大小和读取时间。
- 二进制内容默认只返回 metadata。
- 原始正文过大时必须截断或摘要化。

## HTTP Provider 设计

### 配置落点

Web Search 配置必须复用现有整体配置框架：

- 配置模型落在 `internal/pkg/workorchconfig.Config`。
- 新增顶层 `WebSearch WebSearchConfig`，与 `Server`、`Repository`、`Runtime`、
  `LLM` 同级。
- 配置读取继续通过 `workorchconfig.Current()`。
- 配置保存继续通过 `workorchconfig.Save(next)` 写入 `application.toml`。
- 配置热加载继续通过 `workorchconfig.Reload()` 和 `workorchconfig.Revision()`。
- Web Search runtime adapter 使用 config loader + revision cache，模式对齐
  LLM runtime adapter。
- Web console 配置页复用现有 `/configuration` 读取路径，并新增受控更新接口，
  不绕过 appservice 直接写文件。
- API key 复用 LLM provider 的隐藏规则：读取时只返回 `apiKeyConfigured`，
  更新时空值表示保留已有密钥。

建议配置模型：

```go
type WebSearchConfig struct {
	DefaultProvider string                              `json:"defaultProvider" toml:"defaultProvider"`
	Providers       map[string]WebSearchProviderConfig `json:"providers" toml:"providers"`
}

type WebSearchProviderConfig struct {
	Provider       string            `json:"provider" toml:"provider"`
	SearchEndpoint string            `json:"searchEndpoint" toml:"searchEndpoint"`
	FetchEndpoint  string            `json:"fetchEndpoint" toml:"fetchEndpoint"`
	APIKey         string            `json:"apiKey" toml:"apiKey"`
	Headers        map[string]string `json:"headers" toml:"headers"`
	Timeout        string            `json:"timeout" toml:"timeout"`
	MaxResults     int               `json:"maxResults" toml:"maxResults"`
	MaxFetchBytes  int64             `json:"maxFetchBytes" toml:"maxFetchBytes"`
	MaxRedirects   int               `json:"maxRedirects" toml:"maxRedirects"`
	AllowDomains   []string          `json:"allowDomains" toml:"allowDomains"`
	DenyDomains    []string          `json:"denyDomains" toml:"denyDomains"`
	Disabled       *bool             `json:"disabled" toml:"disabled"`
}
```

`application.toml` 示例：

```toml
[webSearch]
defaultProvider = "default-http"

[webSearch.providers.default-http]
provider = "http"
searchEndpoint = "https://search-gateway.example.com/search"
fetchEndpoint = "https://search-gateway.example.com/fetch"
apiKey = ""
timeout = "20s"
maxResults = 8
maxFetchBytes = 262144
maxRedirects = 3
allowDomains = []
denyDomains = []
disabled = false

[webSearch.providers.default-http.headers]
X-WorkOrch-Client = "workorch"
```

### 配置

第一版配置字段建议：

```json
{
  "webSearch": {
    "defaultProviderId": "default-http",
    "providers": [
      {
        "id": "default-http",
        "provider": "http",
        "searchEndpoint": "https://search-gateway.example.com/search",
        "fetchEndpoint": "https://search-gateway.example.com/fetch",
        "apiKey": "",
        "headers": {
          "X-WorkOrch-Client": "workorch"
        },
        "timeout": "20s",
        "maxResults": 8,
        "maxFetchBytes": 262144,
        "maxRedirects": 3,
        "allowDomains": [],
        "denyDomains": [],
        "disabled": false
      }
    ]
  }
}
```

配置规则：

- `searchEndpoint` 必填。
- `fetchEndpoint` 可选；为空时 `web/fetch` 返回 provider unavailable。
- `apiKey` 按现有配置密钥规则隐藏和保存。
- `headers` 不允许覆盖 `Content-Type`、`Accept`、`User-Agent` 等系统保留头，
  除非后续明确设计。
- `timeout`、`maxResults`、`maxFetchBytes` 必须有系统默认上限。

### Search HTTP Contract

workorch 请求：

```json
{
  "workspaceId": "ws-xxx",
  "runId": "run-xxx",
  "stepId": "run-xxx/step-web-search-1",
  "query": "current golang stable version",
  "locale": "en-US",
  "recency": "month",
  "domains": [],
  "excludeDomains": [],
  "maxResults": 8,
  "safeSearch": true
}
```

网关响应：

```json
{
  "results": [
    {
      "title": "Go Downloads",
      "url": "https://go.dev/dl/",
      "snippet": "Download and install Go quickly...",
      "source": "go.dev",
      "publishedAt": ""
    }
  ]
}
```

### Fetch HTTP Contract

workorch 请求：

```json
{
  "workspaceId": "ws-xxx",
  "runId": "run-xxx",
  "stepId": "run-xxx/step-web-fetch-1",
  "url": "https://go.dev/dl/",
  "maxBytes": 262144,
  "extractMode": "text"
}
```

网关响应：

```json
{
  "url": "https://go.dev/dl/",
  "finalUrl": "https://go.dev/dl/",
  "title": "Downloads - The Go Programming Language",
  "content": "Download and install Go quickly...",
  "contentType": "text/html",
  "retrievedAt": "2026-05-05T00:00:00Z",
  "publishedAt": "",
  "source": "go.dev"
}
```

第一版强制外部网关返回标准结构，暂不实现通用 JSONPath 映射。这样可以减少
workorch 内部复杂度，并为后续 provider adapter 保持稳定边界。

## 错误模型

`web/search` 和 `web/fetch` 必须输出稳定 `errorCode`，禁止 Web/TUI/CLI、
LLM follow-up 或 e2e 解析自由文本错误。

第一版固定错误码：

| errorCode | 语义 | 典型场景 |
| --- | --- | --- |
| `provider_unavailable` | provider 不可用 | 未配置默认 provider、provider disabled、缺少 endpoint |
| `invalid_request` | 输入无效 | query 为空、URL 格式非法、maxResults 非法 |
| `policy_denied` | 策略拒绝 | 命中 denyDomains、禁止访问内网且不能审批 |
| `approval_required` | 需要审批 | 明文 http、内网、登录态、大批量查询等需要人工批准 |
| `timeout` | 调用超时 | provider 请求超时、DNS 解析超时、读取超时 |
| `bad_gateway_response` | 网关响应无效 | 非 2xx、JSON 结构不符合标准 contract |
| `content_too_large` | 内容过大 | fetch 响应超过 maxFetchBytes 或 provider 标记过大 |

输出规则：

- 成功时 `errorCode` 为空或省略。
- 失败或 blocked 时必须设置 `errorCode`。
- `error` 只作为面向人类的说明，不作为程序判断依据。
- `status=failed` 表示本 step 终止失败。
- `status=blocked` 且 `errorCode=approval_required` 表示需要 Approval 后重试。

## 安全与治理

### 默认允许

- 公网 `https` 搜索。
- 公网 `https` 网页读取。
- 小规模只读查询。
- 明确由 workspaceTemplate / soul / skill 允许的搜索阶段。

### 默认拒绝或需要 Approval

- `http` 明文请求：默认需要 Approval 或配置显式允许。
- localhost、loopback、private CIDR、link-local、metadata endpoint。
- 登录态、Cookie、自定义 Authorization 访问。
- 超过配置上限的大批量搜索或 fetch。
- 命中 `denyDomains` 的域名。
- 下载二进制文件或未知 content type。
- 用户未明确授权的内网域名。

### SSRF 防护细则

Web Search provider 必须在发起任何外部请求前完成 URL 和网络目标校验。

实现要求：

- 只允许 `http` / `https` scheme；其他 scheme 一律 `policy_denied`。
- 请求前解析 host，并对 DNS 解析得到的全部 IP 进行校验。
- redirect 后必须对 `Location` / final URL 重新执行完整校验。
- 拒绝 IPv4 / IPv6 loopback、private、link-local、unspecified、multicast。
- 拒绝 cloud metadata endpoint，包括但不限于：
  - `169.254.169.254`
  - `metadata.google.internal`
  - `100.100.100.200`
  - `fd00:ec2::254`
- 拒绝 userinfo URL，例如 `https://user:pass@example.com`，除非后续单独设计。
- DNS 解析失败返回 `bad_gateway_response` 或 `timeout`，不得退化为未校验访问。
- IP literal URL 也必须执行同样的 CIDR 策略。
- `allowDomains` 只能放宽域名白名单，不能绕过 metadata / loopback /
  link-local 等硬拒绝规则。

### 证据要求

每个 `web/search` / `web/fetch` step 至少记录：

- provider id。
- query 或 URL。
- 执行时间。
- status 和错误信息。
- 结果数量或内容 hash。
- 被截断标记。
- policy 决策摘要。

## Approval 协议

Web Search 必须复用现有 Approval 闭环，不引入独立审批状态机。

### 触发审批

当请求命中“可审批但默认不自动执行”的策略时，step 输出：

```json
{
  "status": "blocked",
  "stage": "web_fetch",
  "errorCode": "approval_required",
  "approvalRequired": true,
  "approvalAction": "web/fetch",
  "approvalContext": {
    "providerId": "default-http",
    "url": "http://intranet.example.local/status",
    "reason": "需要读取用户指定的内网状态页完成验收",
    "policyDecision": "approval_required",
    "policyReasons": ["private_network", "plain_http"]
  }
}
```

`web/search` 的审批 action 使用 `web/search`，context 至少包含：

- `providerId`
- `query`
- `maxResults`
- `domains`
- `policyDecision`
- `policyReasons`

`web/fetch` 的审批 action 使用 `web/fetch`，context 至少包含：

- `providerId`
- `url`
- `finalUrl`，如已知
- `maxBytes`
- `policyDecision`
- `policyReasons`

### 审批后恢复

重新执行时 input 必须携带：

```json
{
  "approvalId": "approval-xxx"
}
```

adapter 校验规则：

- `approvalId` 必须存在。
- Approval 必须为 approved。
- Approval 的 action 必须与当前 step kind 匹配。
- Approval context 必须与当前请求的 provider、query 或 URL、风险原因匹配。
- 已拒绝、过期或不匹配的 approval 必须返回 `policy_denied`。
- 审批通过只允许本次请求继续执行，不改变全局 provider 策略。

## LLM 使用策略

LLM runtimeCapabilities 需要新增：

- `web/search`: Search external web sources through a governed provider.
- `web/fetch`: Fetch one URL through a governed provider and return bounded
  content or metadata.

LLM 约束：

- 只有 runtimeCapabilities 声明后，LLM 才能生成 `web/search` / `web/fetch`
  executionGraph node。
- 涉及最新信息、政策法规、产品版本、外部事实核验、市场信息时，应优先搜索。
- 不能把 search snippet 当作唯一事实来源；重要结论必须 fetch 或引用可信来源。
- 最终输出必须包含来源和检索时间。
- 搜索失败时，若没有足够本地信息，必须 blocked_on_decision 或明确说明限制。

## Artifact 与 Memory 策略

### Artifact

建议新增或复用以下 Artifact 类型：

- `web-search-summary`
- `web-source-index`
- `web-fetch-content`

规则：

- Result summary 只引用被最终采用的来源。
- Source index 可记录所有候选结果。
- 原始 fetch 内容只有在对验收必要时才写 Artifact，且必须受大小限制。
- Artifact 必须引用 step evidence。

### Memory

规则：

- Memory 不写入原始网页全文。
- 只写入最终采用的事实摘要、来源 URL、retrievedAt 和 contentHash。
- 搜索过程中的未采用候选结果不写入 Memory。
- 若事实具有时效性，Memory 必须标记时间敏感或过期策略。

## Read Model 字段

AppService 需要提供统一 Web Source 投影，Web/TUI/CLI 不直接解析 step output。

最小 read model：

```json
{
  "query": "string",
  "providerId": "string",
  "searchedAt": "string",
  "sources": [
    {
      "title": "string",
      "url": "string",
      "snippet": "string",
      "source": "string",
      "publishedAt": "string",
      "rank": 1,
      "evidenceRef": "run-xxx/step-web-search-1"
    }
  ],
  "fetchedSources": [
    {
      "url": "string",
      "finalUrl": "string",
      "title": "string",
      "source": "string",
      "retrievedAt": "string",
      "contentHash": "string",
      "truncated": false,
      "evidenceRef": "run-xxx/step-web-fetch-1"
    }
  ],
  "evidenceRefs": ["run-xxx/step-web-search-1"],
  "approvalId": "approval-xxx",
  "policyDecision": "allowed"
}
```

字段规则：

- `sources` 来自 `web/search`。
- `fetchedSources` 来自 `web/fetch`。
- `evidenceRefs` 必须能定位原始 step。
- `policyDecision` 固定枚举：`allowed`、`denied`、`approval_required`。
- TUI/CLI 使用该模型显示最终来源摘要。
- Web 使用该模型展示完整来源索引和证据关系。

## Web / TUI / CLI 展示

### Web

Web 需要展示完整可追溯信息：

- Web Search 查询列表。
- 每次查询的 provider、时间、结果数量、状态。
- 被 fetch 的 URL、标题、来源、retrievedAt、contentHash。
- 来源与 Result / DoD / Artifact 的引用关系。
- Approval 关联和 policy 决策。

### TUI

TUI 默认简化展示：

- 运行中状态栏可显示 `Searching web...` 或 `Fetching source...`。
- Result 区显示最终来源列表。
- 不展示完整搜索流水和大段网页正文。

### CLI

CLI 支持：

- `workorch run` 输出最终来源摘要。
- `workorch events` / `workorch status` 可看到 web search/fetch step。
- 后续可增加 `workorch sources` 查看 Run 来源索引。

## 任务清单

状态说明：

- `pending`：已确认需求，尚未开始。
- `in_progress`：正在实现。
- `implemented`：代码已完成，等待验证。
- `verified`：代码和验收均已通过。
- `blocked`：存在外部依赖或设计问题需要先确认。

| ID | 任务 | 优先级 | 状态 | 验收 |
| --- | --- | --- | --- | --- |
| WWS-01 | 新增本文档并固化能力边界 | P0 | verified | 文档已创建 |
| WWS-02 | 新增 `internal/modules/blocks/web` 和 step kind 常量 | P0 | verified | 已新增 `web/search`、`web/fetch`，bootstrap/runtime 测试覆盖注册 |
| WWS-03 | 在 `workorchconfig` 中实现 Web Search 配置模型、保存、重载和 revision cache | P0 | verified | 配置读写测试通过，runtime adapter revision cache 测试通过，复用整体配置框架；已补齐 `/configuration/web-search` 更新接口和 Web 配置页 |
| WWS-04 | 实现 `web/search` HTTP client provider | P0 | verified | mock HTTP 网关测试覆盖标准 search output、坏响应稳定 errorCode |
| WWS-05 | 实现 `web/fetch` HTTP client provider | P0 | verified | mock HTTP 网关测试覆盖标准 fetch output、approval 后恢复执行 |
| WWS-06 | 补齐 URL / 域名 / CIDR / redirect / size / timeout 安全策略 | P0 | verified | 已实现并测试 scheme/userinfo/domain/metadata/DNS/IP/size/timeout/finalUrl 复核 |
| WWS-07 | 接入 runtime registry、bootstrap 和 plugin runtime | P0 | verified | 本地 runtime 已能 dispatch `web/search` / `web/fetch`，bootstrap 测试通过 |
| WWS-08 | 注入 LLM runtimeCapabilities 和输出校验 | P0 | verified | LLM capability catalog 已声明 `web/search` / `web/fetch`，测试覆盖 |
| WWS-09 | 接入 Approval 策略 | P0 | verified | 已覆盖 fetch 内网/loopback/finalUrl 和 search 大结果集 approvalRequired；approved approvalId 可恢复，不匹配返回 policy_denied |
| WWS-10 | 补齐事件、payload 和 EventStream 持久化 | P1 | verified | observability timeline 已可追溯持久化 web step event；测试覆盖 persisted `web/search` step event 投影 |
| WWS-11 | RunSnapshot / RunObservability / WorkspaceSnapshot 投影 | P1 | verified | read model 已返回统一 Web Source 来源索引，Run/Workspace/Observability 测试覆盖 |
| WWS-12 | Artifact 生成和 Result 引用策略 | P1 | verified | 完成 Result artifactRefs 接入 `web-source-index`，artifact 由 web step evidence 生成来源索引 |
| WWS-13 | Memory 写入策略 | P1 | verified | run summary memory 只写来源摘要引用、URL、contentHash 和 evidenceRefs；LLM prompt 禁止写入原始网页正文和未采用候选结果 |
| WWS-14 | Web 展示来源、fetch、policy、approval 关系 | P1 | verified | Web 通过统一 read model 展示 Web 来源、fetch、policy、approval、evidence/artifact 关系；前端构建通过 |
| WWS-15 | TUI/CLI 简化来源展示 | P1 | verified | TUI/CLI 只显示最终来源摘要和 evidenceRef，不展示网页正文；单测覆盖 TUI 来源渲染 |
| WWS-16 | 新增 `scripts/verify-workspace-web-search.sh` | P0 | verified | 验证脚本已覆盖 web block、config、appservice、bootstrap、orchestrator、LLM capability |
| WWS-17 | 部署实例回归验证 | P0 | verified | 新增 `scripts/verify-workspace-web-search-e2e.sh`，已使用 mock LLM + mock HTTP Web provider 完成真实 Workspace Run 并验证 snapshot/observability/EventStream/artifact 来源追溯 |

## E2E 验收场景

### WWS-E2E-01: 基础搜索

- Given 配置了 HTTP search provider。
- When Run 需要查询外部资料。
- Then `web/search` step completed。
- And Run observability 展示 query、provider、searchedAt、results。

### WWS-E2E-02: 搜索后读取来源

- Given 搜索结果包含可访问 URL。
- When LLM 需要验证事实。
- Then 执行 `web/fetch`。
- And 输出 `contentHash`、`retrievedAt`、`source`。

### WWS-E2E-03: Result 引用来源

- Given Run 使用 web search/fetch 完成回答。
- When Run completed。
- Then Result 包含最终来源列表。
- And Artifact 包含 `web-source-index`。
- And DoD evidence refs 引用 web step。

### WWS-E2E-04: 安全策略拒绝内网 URL

- Given fetch URL 为 `http://127.0.0.1:8080` 或 private CIDR。
- When 未提供审批。
- Then step failed 或 blocked approval。
- And 不发起实际 HTTP 请求。

### WWS-E2E-05: Approval 后访问受限来源

- Given Workspace 策略允许审批后访问特定内网域名。
- When 用户批准。
- Then `web/fetch` 携带 approved `approvalId` 后继续执行。

### WWS-E2E-06: Memory 只写入采用事实

- Given 搜索返回多个候选结果。
- When 最终 Result 只采用其中一个来源。
- Then Memory 只包含采用事实摘要和来源引用。
- And 不包含未采用候选结果或原始网页全文。

## 验证命令

预期补齐后执行：

```bash
go test ./internal/modules/blocks/web/... -count=1
go test ./internal/modules/blocks/llm/... ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/biz/bootstrap ./internal/modules/application/workorchd/biz/appservice -count=1
go test ./internal/modules/application/workorch/service/tui ./internal/modules/application/workorch/service/cli -count=1
npm run build # web/workorch-console
./scripts/verify-workspace-web-search.sh
./scripts/verify-workspace-web-search-e2e.sh
go test ./... -count=1
```

当前验证记录：

- 2026-05-05：`./scripts/verify-workspace-web-search.sh` 已通过。
- 2026-05-05：`go test ./... -count=1` 已通过。
- 2026-05-06：`go test ./internal/modules/application/workorchd/biz/appservice ./internal/modules/application/workorchd/service/server -count=1` 已通过。
- 2026-05-06：`npm run build`（`web/workorch-console`）已通过，并刷新嵌入式 console assets。
- 2026-05-06：`./scripts/verify-workspace-web-search.sh` 已通过。
- 2026-05-06：本地部署实例执行 `WORKORCH_WEB_SEARCH_E2E_BASE_URL=http://127.0.0.1:18080 ./scripts/verify-workspace-web-search-e2e.sh` 已通过，Run `run-5d8f8ec03589000f` 完成并验证 Web 来源追溯。
- 2026-05-06：`go test ./... -count=1` 已通过。
- 部署级 e2e 会启动本地 mock LLM / Web HTTP provider；需要在允许本机监听端口的
  环境中执行。
