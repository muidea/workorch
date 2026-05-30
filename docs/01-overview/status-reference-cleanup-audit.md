# WorkOrch Status Reference Cleanup Audit

Status: active

Type: audit

Last Updated: 2026-05-30

## Scope

本审计记录第二轮“`../workorch-status.md` 引用清理迁移”的结果。

目标是清理新结构文档中仍把 `../workorch-status.md` 当作 source/fact 文档的情况，
将能直接迁移的引用改为新的承接文档。

## Completed Cleanup

本轮已完成以下直接引用清理：

- `../10-domain-model/workspace-goal-dod.md`
- `../10-domain-model/run-step-result-artifact.md`
- `../10-domain-model/governance-decision-approval.md`
- `../10-domain-model/event-observability.md`
- `../20-runtime-flow/activation-definition-flow.md`
- `../30-module-architecture/application-workorchd.md`
- `../30-module-architecture/kernel-workspace-run-result.md`
- `../30-module-architecture/blocks-runtime-adapters.md`
- `../30-module-architecture/storage-readmodel.md`
- `../90-archive/records/overview.md`
- `../90-archive/superseded/workorch-status.md`

补充：

- `../90-archive/records/overview.md`
  - candidate input 中的单工作区性能记录已切换到 archive 内当前有效路径

## Remaining Intentional References

以下位置仍保留 `../workorch-status.md`，原因如下：

- `batch-1-migration-audit.md`
  - 该审计文档记录的是 Batch 1 当时的事实，应保留原始审计上下文。
- `workorch-status-split-audit.md`
  - 该专项审计需要明确说明旧引用仍存在的事实。

当前核对后，active 设计正文中的剩余保留已基本收缩到：

- 无

其余主要残留已经集中在：

- `01-overview/*` 迁移/审计说明
- `90-archive/*` 历史追溯说明

## Remaining Non-Transition References Outside New Structure

当前未发现新的“仍把 `../workorch-status.md` 当正文事实来源”的 active 设计正文文档。

剩余引用主要属于：

- 过渡索引说明
- 历史 audit / archive 追溯

## Supersede Readiness

当前可用以下检查表判断根目录 `../workorch-status.md` 的内部迁移是否已经完成：

| 条件 | 当前状态 | 说明 |
| --- | --- | --- |
| active 设计正文不再把 `../workorch-status.md` 当事实来源 | satisfied | 当前 active 正文已无此类引用 |
| root-level 阅读导航不再把 `../workorch-status.md` 作为推荐过渡入口 | satisfied | `../README.md` 与 `current-state.md` 已移除对其的主动推荐 |
| archive/overview 历史说明不再需要保留旧路径追溯 | satisfied | 剩余保留已切换为“旧综合状态文档”历史语境或 superseded 副本路径 |
| 存在明确 superseded 落位 | satisfied | `../90-archive/superseded/workorch-status.md` 已建立正式归档副本 |

当前结论：

- 内部文档迁移已经完成；`../workorch-status.md` 不再是 active 阅读入口或内部事实来源
- 当前继续保留在根目录的唯一原因是外部旧引用兼容
- 后续是否物理移除根目录兼容别名，不再属于文档结构迁移阻塞项
