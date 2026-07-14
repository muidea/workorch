# Product Interface Code Alignment Checklist

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件用于核对 `50-product-interface/` 当前产品接口设计是否已经被代码正确承接。

它不重新定义产品面，只把当前需要逐项核对的实现对齐项拆成可执行清单，
便于后续按 API / TUI / Web Console 逐项处理。

## Canonical Sources

核对时以以下文档为准：

- `../50-product-interface/overview.md`
- `../50-product-interface/service-api.md`
- `../50-product-interface/deployment.md`
- `../50-product-interface/tui/requirements.md`
- `../50-product-interface/tui/interaction-contract.md`
- `../50-product-interface/tui/display-refresh-consistency-design.md`
- `../50-product-interface/web-console/overview.md`
- `../30-module-architecture/application-workorchd.md`
- `../30-module-architecture/storage-readmodel.md`
- `../60-verification/service-acceptance.md`

## Review Goal

这一轮只回答三个问题：

1. 产品接口设计是否已经在代码中有明确落点。
2. API / TUI / Web 是否只消费正式读模型与后端 contract，而没有前端重演业务规则。
3. 哪些点属于“文档超前”，哪些点属于“实现偏差”。

## Suggested Review Scope

建议优先核对以下代码目录：

- `internal/modules/application/workorchd/service/server`
- `internal/modules/application/workorchd/biz/appservice`
- `internal/modules/application/workorchd/pkg/observed`
- `internal/modules/application/workorchd/pkg/presenter`
- `internal/services/workorch/service/tui`
- `web/workorch-console/src`

## Review Order

建议顺序：

1. service API
2. deployment / startup surface
3. TUI requirements / interaction / display
4. Web Console page-to-API mapping
5. raw/detail open boundary
6. form / interaction responsibility boundary

## 1. Service API

重点核对：

- response family 是否与文档一致。
- `workspace snapshot` / `run snapshot` / `run observability` 是否为主要聚合读模型入口。
- detail page 是否通过分页接口提供，而不是默认全量加载。
- `runtime/payloads` / `artifacts/content` 是否作为显式 raw/detail 打开路径。
- `artifactContinuations` 是否默认只返回 metadata，而不是正文。
- `selectedCapabilities` / `usedCapabilities`、`MemoryRecallTrace`、`coverageWorkItems`
  等字段是否从正式后端投影而来。
- handler 层是否只做 parse / validation / response，而不复制业务逻辑。

常见偏差：

- API 默认返回 raw payload / 大对象正文
- `artifactContinuations` 默认暴露 plan/chunk/assembled 内容
- server/handler 直接重演 appservice 业务逻辑

## 2. Deployment / Startup Surface

重点核对：

- container / local startup 入口是否与文档一致。
- file backend 挂载目录是否与文档中的 deliverable/internal boundary 一致。
- deployment 配置面是否只承接配置入口，不承接行为正确性证明。
- acceptance / verifier 是否与 deployment 边界分离，而没有混写在启动文档或实现路径中。

常见偏差：

- 启动脚本或入口混入验收断言
- file backend 路径和文档中的目录边界不一致

## 3. TUI Requirements / Interaction / Display

重点核对：

- 本地 TUI 是否不启动 `workorchd` 子进程。
- 本地 TUI 是否不通过 HTTP client 路径访问 WorkOrch。
- 本地/远程模式是否都通过共享 backend 抽象，而不是两套业务语义。
- TUI 是否使用 EventHub callback/subscription，而不是 fixed polling。
- 主视图是否是 result-first，而不是 event-stream-first。
- workspace switch / decision / approval / result/artifact review 是否与交互合同一致。
- `artifactContinuations` 是否只展示 metadata/progress，不默认展开正文。
- blocker / status / active run 是否都以 active workspace + active run 为准。

常见偏差：

- TUI 仍然依赖 HTTP polling
- TUI 主视图退化成 raw event/debug 面板
- decision/approval 输入逻辑和文档不一致

## 4. Web Console Page-To-API Mapping

重点核对：

- Business Board / Workspace Detail / Governance Queue / Runtime Control / Configuration /
  Service Status 是否与文档中的 API mapping 一致。
- Workspace Detail 是否优先消费 snapshot / observability / compact sections。
- GovernanceQueue 是否只在显式展开时读取 detail。
- SSE 是否只用于通知刷新，而不是唯一事实源。
- `artifactContinuations` 是否在 board/detail/observability 中保持 metadata-first。

常见偏差：

- 页面直接调用不在文档中的后门接口
- SSE 被当成唯一状态源
- Web 默认展开 artifact continuation 正文

## 5. Raw / Detail Open Boundary

重点核对：

- `runtime/payloads` 和 `artifacts/content` 是否是显式打开路径。
- raw/detail 打开是否不反向改变当前 summary state / active run / governance state。
- raw/detail 失败时，前端是否只展示接口错误，而不自行拼装替代内容。
- summary / snapshot / observability 是否不会默认嵌入 raw payload、artifact 正文、
  artifact continuation 正文。

常见偏差：

- raw 打开影响当前页面状态机
- 前端为失败的 raw/detail 请求自行构造展示内容
- snapshot/observability 默认混入 raw 内容

## 6. Form / Interaction Responsibility Boundary

重点核对：

- Web form 是否只做最小字段校验、组装请求体、调用 API、成功后回读。
- 前端是否没有重演：
  - DoD 质量规则
  - approval policy fallback
  - provider/runtime compatibility
  - run/decision/approval 状态机
- workspace create 是否不自动绑定后续 run，除非用户显式动作。
- governance resolution form 是否只组织 resolution payload，不推导 successor run 状态。
- TUI / Web 是否都不把 comment / note 等 UI 字段误表达成正式后端模型字段。

常见偏差：

- 前端本地判定 DoD 是否满足
- form 本地推导治理或运行状态
- UI 临时字段被当成正式模型字段

## 7. Cross-Product Consistency Checks

重点核对以下跨产品一致性：

- API、TUI、Web 是否对同一 read-model 字段保持相同语义。
- active workspace / active run / blocker / outcome 的选择规则是否一致。
- raw/detail 边界是否在 API / TUI / Web 三层保持一致。
- metadata-first projection 是否在 API、Web、TUI 三层都成立。
- deployment / service acceptance 是否没有重新混成一份文档或一条实现路径。

常见偏差：

- API 说 metadata-first，但 Web/TUI 默认读正文
- TUI 和 Web 对 blocker / active run 选择规则不一致
- deployment 文档和 acceptance 逻辑在实现里混层

## Classification Rules

核对结果建议分三类：

- `pass`
  - 设计与代码一致，产品面只消费正式 contract / projection
- `doc-ahead`
  - 文档定义已明确，但代码尚未正式落地
- `impl-drift`
  - 代码已有实现，但页面/API 行为、raw/detail 边界或交互责任与当前设计冲突

## High-Priority Checks

优先检查这些高风险点：

1. API 是否默认暴露 raw payload / continuation 正文
2. TUI 是否仍依赖 HTTP/polling 或退化为 event-stream-first
3. Web 是否把 SSE 当作唯一状态源
4. 前端是否重演 DoD / governance / runtime 业务规则
5. `artifactContinuations` 是否在 API / Web / TUI 三层都保持 metadata-first

## Suggested Output Template

后续逐项核对时，建议按以下格式记录：

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| service API | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| deployment / startup | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| TUI requirements / interaction | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| TUI display / refresh | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| Web Console mapping | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| raw/detail boundary | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| form / interaction boundary | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |
| cross-product consistency | `path/to/code` | pass / doc-ahead / impl-drift | 简述问题 |

## 2026-05-30 Code Alignment Result

| Area | Code Path | Result | Notes |
| --- | --- | --- | --- |
| service API | `internal/modules/application/workorchd/service/server/http.go` / `internal/modules/application/workorchd/biz/appservice/read_model_service.go` | pass | workspace/run snapshot、observability、detail page、raw content endpoint 均有正式落点；handler 继续保持 parse / validation / response 边界。 |
| deployment / startup | `docs/50-product-interface/deployment.md` / `internal/modules/application/workorchd` | pass | 本轮未发现 startup surface 混入 acceptance 断言；file backend 与 deliverable/internal boundary 继续由配置和 runtime session 承接。 |
| TUI requirements / interaction | `internal/services/workorch/service/tui` | pass | 本地 TUI 使用 shared backend 与 EventHub subscription；未发现 fixed polling 或本地 HTTP client 路径。slash command、decision、approval、result-first 主视图与当前交互合同一致。 |
| TUI display / refresh | `internal/services/workorch/service/tui/view.go` / `workspace_state.go` | pass | active workspace + active run 驱动主视图、状态栏和 blocker；非 active workspace 事件仅刷新 bounded monitor。artifact continuation 仅展示 target/state/progress/failure summary。 |
| Web Console mapping | `web/workorch-console/src/api.ts` / `web/workorch-console/src/pages` | pass | Business Board、Workspace Detail、Governance Queue、Runtime Control、Configuration、Service Status 均通过文档列出的 API families 获取正式读模型；SSE 只触发刷新。 |
| raw/detail boundary | `internal/modules/application/workorchd/biz/appservice/artifact_content_service.go` / `runtime_raw_content_service.go` / `web/workorch-console/src/api.ts` | pass | `artifacts/content` 与 `runtime/payloads` 仍是显式打开路径；snapshot/observability 默认不嵌入 runtime raw payload 或 artifact continuation 正文。 |
| form / interaction boundary | `web/workorch-console/src/pages` | pass | Web form 只做字段组织、最小校验和 API 调用；未发现前端重演 DoD 质量、approval fallback、provider compatibility 或 runtime 状态机。 |
| cross-product consistency | `web/workorch-console/src/utils/index.ts` / `web/workorch-console/src/helpers.tsx` | impl-drift fixed | 删除未使用但经 barrel 导出的 `utils/state.ts`，避免 Web 后续误用第二套 active/latest run、terminal state、DoD ready 投影规则；页面继续使用 `helpers.tsx` 的正式展示 helper。 |
| Web state display helper | `web/workorch-console/src/helpers.tsx` / `web/workorch-console/src/utils/{constants,format,ui}.ts*` | impl-drift fixed | `helpers.tsx` 不再维护第二套 state/progress/recovery label map 和 state tag color 规则；旧 helper 导出保留为 `utils/constants` 的 re-export，页面现有引用继续可用但状态展示规则只有一份。 |

### Follow-Up

- `helpers.tsx` 仍是 Web 当前主要展示 helper，但状态/进度/恢复 label map 与 state tag color 已收束到 `utils/constants`、`utils/format`、`utils/ui`；后续如继续拆分，应保持单一稳定模块，并同步替换页面引用，禁止重新出现多套 active run / blocker / terminal state 规则。
