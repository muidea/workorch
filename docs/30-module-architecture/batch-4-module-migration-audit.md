# Batch 4 Module Migration Audit

Status: active

Type: audit

Last Updated: 2026-05-29

## Scope

本审计记录 `Batch 4` 建立模块架构层时识别出的三类问题：

- 冲突和不一致
- 内容不完整、需要补充
- 无法完整衔接的内容断点

## 1. 冲突和不一致

### 1.1 旧架构文档与当前代码树存在漂移

`../workorch-architecture-design.md` 中仍存在：

- kernel 数量描述过时
- blocks 数量描述过时
- 依赖清单和 UI 依赖描述过时
- 已注册 step kind 不完整

处理结论：

- 本批只抽稳定模块边界
- 旧架构文档后续应继续收缩并最终退出主入口

### 1.2 结构文档与架构文档职责重叠

`../structure.md` 和 `../workorch-architecture-design.md` 都在描述：

- 目录树
- 模块落点
- 模块职责

处理结论：

- `repository-structure.md` 只保留目录/模块形状
- `architecture-overview.md` 只保留稳定层级和职责

### 1.3 LLM 预算/auto profile 文档兼具设计与实现双重角色

`../model-aware-llm-context-budget-design.md` 和
`../workorch-llm-auto-context-profile-design.md` 既像设计，又像实现说明。

处理结论：

- 本层统一将其视为 provider assembly 的实现说明
- 具体 core contract 仍由 `00-core/` 裁决
- `llm-provider-assembly.md` 已补入 provider resolution、request rendering direction 和 provider-facing budget/audit 责任

## 2. 内容不完整，需要补充

### 2.1 kernel 分组仍较粗

当前模块层只建立了：

- `kernel-workspace-run-result.md`
- `kernel-orchestrator-governance.md`
- `kernel-context-capability.md`

仍缺：

- 更细的 `eventstream` / `result` / `run evidence` 责任拆分
- governance 内 `decision` 与 `approval` 的实现边界细化

### 2.2 block adapter 仍缺注册矩阵

当前 `blocks-runtime-adapters.md` 只抽出 block 共同责任，
仍缺：

- step kind -> block kind 映射矩阵
- capability boundary
- output normalization 差异

补充说明：

- 当前 `blocks-runtime-adapters.md` 已迁入 high-level step kind mapping 与 output normalization 方向。
- 仍缺精确注册矩阵和更细的 step kind contract。

### 2.3 storage/readmodel 仍缺 snapshot/read endpoints 映射

当前 `storage-readmodel.md` 只抽稳定存储边界，
仍缺：

- snapshot projection 结构
- paginated detail endpoint 支撑关系
- payload ref 打开链路

补充说明：

- 当前 `storage-readmodel.md` 已迁入 itemized runtime evidence storage、raw payload ref strategy、projection-oriented read model，以及 file backend 原子写 / append-only 基本方向。
- `../workorch-runtime-storage-json-control-design.md` 的稳定内容已拆到 `../10-domain-model/event-observability.md` 和 `storage-readmodel.md`。
- 仍待后续收口的重点是 path-safe readable naming、segment overflow fallback，以及 full repository contract 与代码落点映射。

### 2.4 application-workorchd 仍缺 route 层与 session 层细化

当前 `application-workorchd.md` 已迁入：

- grouped exports
- request-scoped runtime session 方向
- observed / presenter / read-model 分层

仍缺：

- request-scoped runtime session
- bootstrap exports 分组
- observed/presenter/read-model 的更细边界

补充说明：

- `../workorch-magiccommon-context-first-upgrade.md` 的稳定内容已迁入 `application-workorchd.md`
- lifecycle/context-first 升级已明确归入应用层/框架接线边界，而非 runtime core contract

### 2.5 llm-provider-assembly 已迁入第一轮正文，但 provider 细化仍缺

当前 `llm-provider-assembly.md` 已迁入：

- provider resolution
- request rendering direction
- provider-facing budget / audit 责任

仍缺：

- provider-specific cache mapping
- request-log verifier
- model capability table / revision cache 的更细实现落点

补充说明：

- `../model-aware-llm-context-budget-design.md` 与 `../workorch-llm-auto-context-profile-design.md` 的稳定内容已继续迁入本文件

## 3. 无法完整衔接的内容断点

### 3.1 旧文档大量混入历史实现记录

`../structure.md`、`../workorch-magiccommon-context-first-upgrade.md`、
`../workorch-runtime-storage-json-control-design.md` 都混有：

- 历史收口记录
- 验证命令
- 实施步骤

这使模块层当前只能抽出“稳定职责”，不能直接完整迁移原文。

### 3.2 旧综合状态文档仍是当时部分模块落点事实来源

例如：

- route registration
- paginated detail endpoints
- read-model bounded projection

这些内容在当时仍分散在旧综合状态文档里，
模块层后续还要继续从中抽取稳定实现边界。

### 3.3 provider request rendering 规则仍深埋 core/flow 文档

虽然已建立 `llm-provider-assembly.md`，
但 request rendering、cachePlan、request audit 的细化规则仍大量散落在：

- `../00-core/runtime-stage-contract.md`
- `../20-runtime-flow/context-assembly-flow.md`
- `../workorch-llm-request-cache-aware-context-design.md`

后续必须继续从 core/flow 中抽取“实现落点”部分，才能让本层完整闭合。
