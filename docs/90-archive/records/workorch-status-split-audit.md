# WorkOrch Status Split Audit

Status: active

Type: audit

Last Updated: 2026-05-30

## Scope

本审计记录第一次对旧综合状态文档做实质正文拆分迁移时识别出的三类问题：

- 冲突和不一致
- 内容不完整、需要补充
- 无法完整衔接的内容断点

## Historical Note

本文件记录的是第一次拆分迁移当时的判断，不等于当前实时未收口清单。

本文中的“旧综合状态文档”指当时的历史汇总入口，也就是后续降级为
（后续归档副本为 `../90-archive/superseded/workorch-status.md`）。除开头点名外，后文优先使用这一历史称呼。

后续已完成的主要收口包括：

- `../50-product-interface/service-api.md`
  - 已补 response family、projection/read-model、query/error/write contract
- `../60-verification/service-acceptance.md`
  - 已补 paginated detail、event/raw content、file-backend reload 与 deployed mapping
- `../90-archive/records/delivery-history.md`
  - 已继续吸收历史交付范围，并把多数可迁移历史主文档切换到当前有效路径

当前若需查看实时剩余项，应优先参考：

- `current-unclosed-matrix.md`
- `information-model-alignment-audit.md`
- `status-reference-cleanup-audit.md`

## 1. 冲突和不一致

### 1.1 旧综合状态文档的 API 清单已明显落后于实际路由

旧综合状态文档中缺少多类当前已存在接口，例如：

- `configuration/server`
- `configuration/web-search`
- `workspace-templates`
- `mcp/capabilities`
- `memory/:memoryId`
- `goal-dod-quality-check`
- `events/stream`
- `runtime/payloads`
- `artifacts/content`

处理结论：

- 新 `../50-product-interface/service-api.md` 已改为以 `internal/modules/application/workorchd/service/server/http.go` 为当前事实来源

### 1.2 旧综合状态文档把当前事实、验收、历史记录混在同一文件

主要混写了：

- active status / active TODO
- current repository state
- acceptance
- completed scope
- loadtest
- run closure quality
- framework alignment

处理结论：

- 已拆到 `current-state`、`service-api`、`service-acceptance`、`delivery-history`、`loadtest-baseline`
- 旧综合状态文档已降级为过渡索引

### 1.3 部署文档与服务验收文档仍有部分重叠

`../50-product-interface/deployment.md` 与新 `../60-verification/service-acceptance.md` 都还保留运行和验证命令。

处理结论：

- 当前允许阶段性重叠
- 后续需继续细化：deployment 负责“如何启动”，service acceptance 负责“如何证明行为”

## 2. 内容不完整，需要补充

### 2.1 `../50-product-interface/service-api.md` 仍缺 response schema 和模型映射

当前已迁出接口分组，但仍缺：

- 每个资源的响应结构
- snapshot / observability / payload ref 与读模型映射
- 重要查询参数的语义

### 2.2 `../60-verification/service-acceptance.md` 仍缺更细验收层次

当前已迁出启动与最小验收命令，但仍缺：

- file backend 持久化重载的更细断言
- paginated detail / payload / event replay 断言
- 与 deployed verification 目标的一一映射

### 2.3 `../90-archive/records/delivery-history.md` 仍缺部分旧状态正文的精细迁移

当前已迁出主要历史范围，但仍缺：

- 更细的阶段性记录粒度
- 与各 completed 文档之间的双向链接

## 3. 无法完整衔接的内容断点

### 3.1 旧综合状态文档之外仍有大量旧文档引用它

虽然该旧综合状态文档已被降级，但旧文档和历史记录中仍保留一批对它的显式
引用，后续工作重点已经从“继续拆正文”转为“清理旧引用和过渡说明”。

处理结论：

- 当前允许保留为过渡索引
- 后续需要继续逐项清理旧引用

### 3.2 一部分“当前仓库事实”还没有稳定承接文档

Batch 1 拆分时，以下内容当时还没有稳定承接文档，例如：

- 模块集合和边界
- framework baseline
- event/read-model 的当前实现事实

这些内容后续已部分进入：

- `current-state.md`
- `30-module-architecture/*`

但尚未完全形成单一来源。

### 3.3 历史交付与活跃 roadmap 的边界仍不完全清晰

旧综合状态文档中的一些章节既像完成记录，也像当前 follow-up 入口，例如：

- run closure quality
- task progress and recovery projection
- framework alignment

处理结论：

- 当前先迁到 `../90-archive/records/delivery-history.md` 或相应 overview
- 后续仍需逐项判断究竟属于 archive、roadmap 还是 verification
