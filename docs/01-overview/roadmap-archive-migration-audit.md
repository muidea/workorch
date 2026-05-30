# WorkOrch Roadmap And Archive Migration Audit

Status: active

Type: audit

Last Updated: 2026-05-29

## Scope

本审计记录将旧 `roadmap / completed` 类正文迁到 `70-roadmap/` 和 `90-archive/`
后的第一轮结果。

## Completed Migration

已迁入新落点：

- `../next-stage-demand-candidates.md`
- `../software-requirement-development-orchestration-plan.md`
- `../software-dev-dod-repair-closure-plan.md`
- `../workorch-unified-tui-console-migration-plan.md`
- `../next-mvp-acceptance.md`
- `../model-aware-llm-context-budget-todo.md`
- `../run-closure-quality-improvement-plan.md`
- `../mvp-backlog.md`
- `../next-stage-p1-development-plan.md`
- `../runtime-expansion-plan.md`
- `../advanced-runtime-scheduling-plan.md`
- `../next-stage-result-event-development-plan.md`
- `../software-dev-code-agent-closure-plan.md`
- `../workorch-tui-improvements-absorption-plan.md`
- `../llm-driven-development-plan.md`

## Current State

- 新路径正文已经建立
- 上述旧根目录文档已降级为过渡索引
- `../70-roadmap/active/overview.md`、`../70-roadmap/candidates/overview.md`、
  `../90-archive/completed/overview.md` 已补新路径入口

## Remaining Gaps

- active roadmap 文档之间仍存在旧路径互引
- completed archive 仍有其余历史正文未正式迁入
- completed archive 目前是“复制后的正文”，后续还需判断是否继续保留双份正文

## Follow-up

1. 继续清理旧路径互引。
2. 继续迁移剩余 completed / records / superseded 类正文。
3. 判断哪些旧 roadmap / completed 文件可以进一步降级为纯过渡索引。
