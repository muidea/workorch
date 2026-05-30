# Web Search Capability

Status: active

Type: profile-capability

Last Updated: 2026-05-30

## Purpose

本文件承接 Web search 相关能力的设计说明。

## Stable Rules

### Positioning

- Web Search 应作为受治理的 runtime capability，而不是让 LLM 自由联网或默认走 `curl` / shell
- 第一版 provider 采用纯 HTTP client，对接标准 HTTP 搜索/读取网关
- 上层只看 `web/search`、`web/fetch` 能力和结构化证据，不感知 provider 差异

### Capability Surface

- `web/search`
  - 执行受控搜索并返回候选来源
- `web/fetch`
  - 读取必要 URL 的正文、摘要或 metadata

搜索结果不是最终事实；最终事实必须引用 `web/search` / `web/fetch` 产生的证据。

### Safety Boundary

- 默认只允许只读公网查询
- 禁止默认访问 localhost、loopback、link-local、private CIDR、metadata endpoint
- 登录态、敏感域名、大批量抓取、大响应和异常重定向应进入 Approval 或直接拒绝
- 所有 HTTP 请求都应受 timeout、结果数、响应大小和重定向次数限制

### Evidence And Memory

- 每次搜索、读取、引用都必须产生可追溯证据
- 最终 Result / Artifact 必须引用采用的信息来源、检索/读取时间和证据步骤
- Memory 只写入最终采用的事实摘要与来源引用，不写入原始大网页正文

### Architecture Boundary

- web block 只负责 provider 调用、安全校验、响应标准化和结构化输出
- orchestrator 不根据关键词硬编码触发搜索
- appservice/read model 负责把 query、URL、来源摘要和证据引用投影到 Run / Workspace 视图

## Directional Contract

- `web/search` 输入至少包括 `query`，可附带 `domains`、`excludeDomains`、`maxResults`、`locale`、`recency`
- `web/search` 输出应包含 `provider`、`results`、`searchedAt`、`resultCount`、`truncated`
- `web/fetch` 输入至少包括 `url`，可附带 `maxBytes`、`extractMode`
- `web/fetch` 输出应包含 `finalUrl`、`title`、`summary/content`、`contentType`、`contentHash`、`retrievedAt`、`approvalRequired`

## Configuration Boundary

- Web Search 配置应复用统一配置框架，不引入独立配置文件或单独热加载链路
- API key 和 provider headers 的展示必须默认脱敏
- 新 provider 只能新增 adapter，不能改变 step kind、read model 和 UI 展示契约

## Step Kind / Adapter Mapping

当前稳定 step kind -> adapter 方向：

| Step Kind | Adapter Block | 说明 |
| --- | --- | --- |
| `web/search` | `blocks/web` | 受控搜索，返回候选来源 |
| `web/fetch` | `blocks/web` | 受控抓取，返回正文、摘要或 metadata |

profile 解释约束：

- `web/search` 只负责发现候选来源，不直接构成最终事实
- `web/fetch` 与 `web/search` 联合才能形成可引用的来源证据

## Evidence / Memory / Read-Model Mapping

Web capability 的稳定证据面包括：

- `ToolCall`
  - search/fetch 请求与结构化结果
- `RuntimeCheckpoint`
  - 安全拦截、重定向、大小限制、审批命中
- `Artifact`
  - 必要时承接来源摘要或导出的引用清单

Memory 写入约束：

- 只写入最终采用的事实摘要与来源引用
- 不写入大网页原文和中间候选噪音

产品层主要承接面：

- `tool-calls` detail page
- `run observability`
  - query、URL、引用链、closure trace
- `artifacts` / `artifacts/content`
  - 当最终需要导出来源清单时

## Verification Mapping

当前与 Web Search 直接相关的验证入口：

- `make verify-workspace-web-search`
- `make verify-workspace-web-search-e2e`
- `../../60-verification/service-acceptance.md`

这些入口分别回答：

- search/fetch contract 是否成立
- deployed service 是否保留来源可追溯性
- raw/detail/read-model 是否继续复用统一 API 面

## Approval Matrix

| 能力动作 | 默认审批方向 | 说明 |
| --- | --- | --- |
| `web/search` 普通公网查询 | 不需要 | 受 timeout / result count / domain policy 约束 |
| `web/search` 敏感域名 / 大批量查询 | 需要或拒绝 | 由 policy 决定 |
| `web/fetch` 普通公共 URL | 视大小/域名决定 | 可直接读取，也可要求审批 |
| `web/fetch` localhost/private CIDR/meta endpoint | 禁止 | 默认不开放 |
| `web/fetch` 大响应、异常重定向、登录态域名 | 需要或拒绝 | 必须进入治理边界 |

## Product Examples

典型产品展示方向：

- ToolCall detail
  - `query`、`provider`、`resultCount`、`finalUrl`、`contentHash`
- run observability
  - 来源列表、取用顺序、closure trace
- artifact/export
  - 当结果需要输出来源清单时，artifact summary 中列 `title/url/source`

更具体的跨产品示例：

- Console ToolCall detail
  - 区分 `web/search` 候选来源与 `web/fetch` 最终采用来源
- Run observability
  - 展示 query -> fetched sources -> accepted result 的引用链
- Result/Artifact 面板
  - 仅展示最终被采用的来源，而不是全部候选噪音

## Read-Model / UI Field Matrix

| 产品面 | 关键字段 | 当前方向 |
| --- | --- | --- |
| tool-calls detail | `query` / `provider` / `resultCount` / `finalUrl` / `contentHash` | search/fetch 主明细 |
| run observability | `webSources` / `timeline` / `closureTraces` | 来源使用链 |
| run snapshot | `artifacts` / `results` / source evidence refs | 最终交付聚合 |
| artifact/export | `title` / `url` / `source` / `retrievedAt` | 来源清单导出 |

## Cross-Layer Mapping

- block 分发：
  `../../30-module-architecture/blocks-runtime-adapters.md`
- 配置与 provider：
  `../../50-product-interface/service-api.md`
- 服务验收：
  `../../60-verification/service-acceptance.md`

## Source Documents

- `../../60-verification/service-acceptance.md`
