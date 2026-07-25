# WorkOrch Records Archive

Status: active

Type: archive

Last Updated: 2026-07-25

## Purpose

本目录承接性能复盘、历史里程碑、交付记录、一次性经验总结，以及 **已退役的文档迁移 audit / tracker / dated live 验证记录**。

它不承接：

- 当前稳定 API 契约
- 当前稳定验收门禁
- 当前稳定 product / runtime 设计定义

## Current Records

### 交付与性能

- [delivery-history.md](delivery-history.md)
- [single-workspace-performance-optimization-record.md](single-workspace-performance-optimization-record.md)
- [workspace-concurrency-loadtest-record.md](workspace-concurrency-loadtest-record.md)
- [coverage-work-item-closure-history.md](coverage-work-item-closure-history.md)

### 2026-07-16 迁入：dated live / 过程验证记录

- [ai-proxy-primary-llm-provider-cutover-live-verification-2026-07-15.md](ai-proxy-primary-llm-provider-cutover-live-verification-2026-07-15.md)（结论：通过）
- [tui-workspace-runtime-refresh-history-closure-verification-2026-07-15.md](tui-workspace-runtime-refresh-history-closure-verification-2026-07-15.md)
- [authority-live-e2e-closure-verification.md](authority-live-e2e-closure-verification.md)（V1-era harness）
- [tui-display-closure-tracking.md](tui-display-closure-tracking.md)

说明：对应 `60-verification/artifacts/**` live 运行产物已从仓库移除；结论以本目录 markdown 为准。

### 2026-07-25 迁入：已完成 framework 整改过程

- [go-multi-module-22-component-boundary-remediation-2026-07-25.md](go-multi-module-22-component-boundary-remediation-2026-07-25.md)：22 组件 framework boundary 迁移过程与已完成证据；当前可执行的组件合同仅见 `70-roadmap/active/component-contract-and-regression-closure-matrix.md`。

### 2026-07-14 迁入：文档迁移 / 对齐 audit（历史过程，非当前入口）

- batch-1-migration-audit.md
- batch-2-domain-migration-audit.md
- batch-3-flow-migration-audit.md
- batch-4-module-migration-audit.md
- 50-product-batch-5-audit.md
- batch-6-verification-roadmap-archive-audit.md
- roadmap-archive-migration-audit.md
- status-reference-cleanup-audit.md
- workorch-status-split-audit.md
- information-model-alignment-audit.md
- incomplete-and-connection-gap-tracker.md
- architecture-optimization-plan.md

### 2026-07-14 迁入：历史验收 checklist / V1 live 复盘

- acceptance-record-*.md
- *-code-alignment-checklist.md / incremental-artifact-code-review-checklist.md
- framework-eventhub-module-boundary-checklist.md
- convergence-authority-dispatch-live-verification.md
- workorch-runtime-closure-modification-spec.md

完整列表以本目录文件名为准。

## Navigation

- 当前事实：[../../01-overview/current-state.md](../../01-overview/current-state.md)
- 已完成计划：[../completed/overview.md](../completed/overview.md)
- superseded：[../superseded/overview.md](../superseded/overview.md)
