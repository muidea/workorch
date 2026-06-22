# Acceptance Record: Product Interface

Status: active

Type: verification

Last Updated: 2026-05-30

## Purpose

本文件记录对
`product-interface-code-alignment-checklist.md`
的第一轮正式验收结果。

判定口径：

- `pass`
  - 当前代码与产品接口设计一致，产品面只消费正式 contract / projection
- `doc-ahead`
  - 文档要求已明确，但当前证据不足以证明实现已经完全落地
- `impl-drift`
  - 当前实现与产品接口设计冲突

## Evidence Base

本轮主要检查以下实现与测试证据：

- `internal/modules/application/workorchd/service/server/http.go`
- `internal/modules/application/workorchd/service/server/http_test.go`
- `internal/modules/application/workorchd/biz/appservice/*`
- `internal/services/workorch/service/tui/*`
- `internal/services/workorch/biz/backends/*`
- `web/workorch-console/src/api.ts`
- `web/workorch-console/src/pages/*`
- `web/workorch-console/src/helpers.tsx`

## Acceptance Result

| Area | Evidence | Result | Notes |
| --- | --- | --- | --- |
| service API | `http.go`, `http_test.go`, appservice read-model mapping | pass | snapshot/observability/detail/raw endpoint 设计均有正式落点；handler 层继续保持 parse / validation / response 边界 |
| deployment / startup surface | `deployment.md` 责任边界 + application/server 入口代码 | pass | 当前实现未见 deployment/startup 面混入 acceptance 逻辑；backend 目录边界与产品文档一致 |
| TUI requirements / interaction | `service/tui/*`, local/remote backend, TUI tests | pass | 本地 TUI 通过 shared backend/EventHub 工作；不依赖本地 HTTP；decision/approval/result-first 主视图证据充分 |
| TUI display / refresh | `workspace_state.go`, `view.go`, model tests | pass | active workspace + active run 驱动展示；non-active workspace 只刷新 monitor；artifact continuation 仅展示 metadata/progress |
| Web Console mapping | `src/api.ts`, `WorkspaceDetailPage`, `DecisionQueuePage`, `ApprovalQueuePage`, `RuntimeControlPage` | pass | 页面与文档中的 API family 映射一致；SSE 只用于通知刷新；未见后门接口依赖 |
| raw/detail boundary | raw open services + web `artifactContentURL` / `runtimePayloadURL` | pass | `runtime/payloads` / `artifacts/content` 是显式打开路径；snapshot/observability 默认不嵌入 raw payload 或 continuation 正文 |
| form / interaction boundary | Web pages/forms, TUI command paths | pass | 表单只做最小校验和请求组装；未发现前端重演 DoD/governance/runtime 状态机 |
| cross-product consistency | API/TUI/Web shared fields and tests | pass | `artifactContinuations` metadata-first、cursor/detail、blocker/active run 选择规则在三层基本一致 |

## Requirement Notes

### 1. Service API

证据：

- `http.go`
- `http_test.go`
- appservice read-model queries

结论：

- 已证明 `workspace snapshot` / `run snapshot` / `run observability` 是正式聚合读模型入口
- 已证明 detail page 通过 `cursorAfterId` / `limit` 分页
- 已证明 `runtime/payloads` / `artifacts/content` 是显式 raw/detail 打开路径
- 已证明 `artifactContinuations` 通过 snapshot/read-model 投影默认只返回 metadata

### 2. Deployment / startup surface

证据：

- `application/workorch`
- `workorchd` server 入口
- deployment/service-acceptance 边界文档与实现路径

结论：

- 当前实现未把 acceptance 验证逻辑混入服务启动入口
- deliverable/internal data root 的运行路径与文档一致

### 3. TUI requirements / interaction

证据：

- `biz/backend/eventhub.go`
- `biz/backend/remote.go`
- `service/tui/model.go`, `view.go`, `workspace_state.go`
- 多组 TUI tests

结论：

- 已证明本地 TUI 使用 shared backend + EventHub callbacks
- 已证明远程模式通过 remote backend 抽象，不改业务语义
- 未见 fixed polling / 本地 HTTP listener 依赖
- decision/approval/result-first 主视图与产品文档一致

### 4. TUI display / refresh

证据：

- `workspace_state.go`
- `view.go`
- `TestModelRendersArtifactContinuationProgress`
- 多组 workspace switch / decision / approval / viewport tests

结论：

- 已证明 active workspace + active run 驱动主视图和 blocker
- 已证明 non-active workspace 更新只刷新 monitor
- 已证明 artifact continuation 默认仅展示 target/state/progress/failure summary

### 5. Web Console mapping

证据：

- `src/api.ts`
- `WorkspaceDetailPage.tsx`
- `DecisionQueuePage.tsx`
- `ApprovalQueuePage.tsx`
- `RuntimeControlPage.tsx`

结论：

- 已证明页面通过文档列出的 API families 获取正式读模型
- 已证明 Web 使用 `EventSource` 订阅 `/events/stream`，作为刷新通知而非唯一事实源
- 未见页面直接调用未文档化的状态构造后门

### 6. Raw/detail boundary

证据：

- `artifact_content_service.go`
- `runtime_raw_content_service.go`
- `api.ts` 中 `artifactContentURL` / `runtimePayloadURL`
- `WorkspaceDetailPage.tsx`

结论：

- 已证明 raw/detail 通过显式链接或 API 打开
- 已证明 snapshot/observability 默认不嵌入 raw payload 或 continuation 正文
- 当前未见 raw/detail 打开反向改写页面状态机

### 7. Form / interaction responsibility boundary

证据：

- Web form 页面实现
- TUI command/approval/decision tests

结论：

- 已证明前端主要负责字段收集、最小校验、调用 API、成功后回读
- 当前未见前端重演：
  - DoD 质量规则
  - approval fallback
  - provider/runtime compatibility
  - run/decision/approval 状态机

### 2026-06-22 Web State Display Helper Closure

本轮复核发现 Web Console 仍保留 `helpers.tsx` 与 `utils` 两套状态展示 label / tag 规则。已将 `helpers.tsx` 改为复用 `utils/constants`、`utils/format`、`utils/ui` 的 canonical 实现，并通过 re-export 保留旧 helper 导入兼容。页面现有 helper 引用不需要迁移，但 active run / blocker / terminal state 等状态显示规则只保留一份，避免后续 Web 页面重新形成第二套前端投影语义。

验证：

- `npm run build` in `web/workorch-console`
- `rg -n "export const stateLabels|export function stateTag|export function stateLabel|blocked_on_decision: '等待决策'|stalled: '停滞'" web/workorch-console/src/helpers.tsx web/workorch-console/src/utils -g '*.ts' -g '*.tsx'`
- `git diff --check -- web/workorch-console/src/helpers.tsx web/workorch-console/src/utils/constants.ts web/workorch-console/src/utils/format.ts web/workorch-console/src/utils/ui.tsx docs/60-verification/product-interface-code-alignment-checklist.md`

## Summary

本轮结论：

- 产品接口层设计与 API / TUI / Web 代码实现总体一致。
- 当前未发现明显 `impl-drift`。
- 也未发现必须保留为关键 `doc-ahead` 的核心缺口。

总体判定：

- 总体结果：`pass`
