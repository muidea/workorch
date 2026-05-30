# WorkOrch Superseded Archive

Status: active

Type: archive

Last Updated: 2026-05-30

## Purpose

本目录承接已被新结构取代或只保留过渡价值的旧文档。

它不承接：

- 当前稳定设计定义
- 当前稳定产品/API/验收入口
- 已完成里程碑记录或专项历史复盘

## Candidate Inputs

- `refactor-archive.md`
- `workorch-status.md`
- `workspace-data-layout-plan.md`
- 迁移完成后的旧入口跳转文档

## Superseded Documents

- [refactor-archive.md](refactor-archive.md)
- [workorch-status.md](workorch-status.md)
- [workspace-data-layout-plan.md](workspace-data-layout-plan.md)

## Supersede Handling Rule

迁入本目录的文档应满足以下条件之一：

- 已被新结构中的稳定正文完全替代
- 只保留过渡、跳转或历史兼容价值
- 继续保留只会制造并行权威入口

当前处理方式：

- 若文档仍有“历史实现记录”价值，优先留在 `../completed/` 或 `../records/`
- 若文档只剩“已被替代”说明，则迁入 `superseded/`
- 若文档仍承担外部兼容别名角色，需先完成兼容入口退出，再正式只保留归档副本说明

## Cross-Archive Navigation

- 当前事实入口
  - `../../01-overview/current-state.md`
- 已完成主题
  - `../completed/overview.md`
- 历史记录
  - `../records/overview.md`
