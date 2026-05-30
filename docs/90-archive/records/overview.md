# WorkOrch Records Archive

Status: active

Type: archive

Last Updated: 2026-05-30

## Purpose

本目录承接性能复盘、历史里程碑、交付记录和一次性经验总结。

它不承接：

- 当前稳定 API 契约
- 当前稳定验收门禁
- 当前稳定 product / runtime 设计定义

## Candidate Inputs

- `single-workspace-performance-optimization-record.md`
- `delivery-history.md`
- `workspace-concurrency-loadtest-record.md`
- `coverage-work-item-closure-history.md`
- 历史综合状态拆分后形成的稳定事实索引入口

## Current Records

- [delivery-history.md](delivery-history.md)
- [single-workspace-performance-optimization-record.md](single-workspace-performance-optimization-record.md)
- [workspace-concurrency-loadtest-record.md](workspace-concurrency-loadtest-record.md)
- [coverage-work-item-closure-history.md](coverage-work-item-closure-history.md)

## Record Layering

| 文档 | 当前职责 | 不负责 |
| --- | --- | --- |
| `delivery-history.md` | 已完成交付范围、阶段性完成记录、历史收口说明 | 当前稳定设计/验收定义 |
| `workspace-concurrency-loadtest-record.md` | 并发压测历史轮次、容量曲线、复盘说明 | 当前稳定 loadtest baseline 门禁 |
| `single-workspace-performance-optimization-record.md` | 单工作区 latency 优化复盘 | 并发容量基线或当前发布门禁 |
| `coverage-work-item-closure-history.md` | `coverageWorkItems` 分批实现与回归收口历史 | 当前稳定 coverage ledger / acceptance gate / read-model 契约 |

当前若需要查看稳定事实入口，应优先转到：

- `../../01-overview/current-state.md`
- `../../60-verification/loadtest-baseline.md`
- `../../60-verification/service-acceptance.md`
