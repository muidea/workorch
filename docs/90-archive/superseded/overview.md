# WorkOrch Superseded Archive

Status: active

Type: archive

Last Updated: 2026-07-28

## Purpose

本目录承接已被新结构取代或只保留过渡价值的旧文档。**不得**作为 Runtime V2 生产实现入口。

它不承接：

- 当前稳定设计定义
- 当前稳定产品/API/验收入口
- 已完成里程碑记录或专项历史复盘（见 `completed/` / `records/`）

## Supersede Handling Rule

迁入本目录的文档应满足：

- 已被新结构中的稳定正文完全替代
- 只保留过渡、跳转或历史兼容价值
- 继续保留只会制造并行权威入口

## Documents

### 既有

- [refactor-archive.md](refactor-archive.md)
- [workorch-status.md](workorch-status.md)
- [workspace-data-layout-plan.md](workspace-data-layout-plan.md)

### 2026-07-14：被 Runtime V2 正式文档替代的 module 设计

| 文档 | 替代入口 |
| --- | --- |
| goal-dod-nextaction-delivery-state-machine-design.md | `00-core` + `20-runtime-flow` + orchestrator readme |
| run-resume-gap-confirmation-design.md | activation/resume = typed bootstrap + DriveRuntimeV2 |
| eventhub-driven-workorchd-state-loop-design.md | application-workorchd + eventstream readme |
| runtime-execution-event-queue-state-loop-design.md | Runtime V2 drive（非 V1 queue） |
| contract-violation-recovery-loop-design.md | V2 evidence/governance path |
| projection-first-readmodel-io-closure-design.md | application projection + storage-readmodel |
| framework-eventhub-structure-migration-plan.md | 已完成的 EventHub 结构迁移 |
| eventhub-lane-policy-and-module-event-contract-plan.md | 模块 events 合同 + eventcontract |
| workorch-service-client-port-migration-plan.md | workorchport / client 现状 |
| llm-cache-improvement-design.md | runtime-request-cache-note + context-assembly-flow |
| mvvp-workspace-viewmodel-architecture.md | product-interface / console 现状 |

### 2026-07-28：并入单 Owner 状态机统一收口

| 文档 | 替代入口 |
| --- | --- |
| runtime-v2-validation-progress-closure-plan.md | `70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md`；旧文档只作 manifest/evaluator/receipt 回归基线 |

完整文件名以本目录 `ls` 为准。

## Navigation

- 当前事实：[../../01-overview/current-state.md](../../01-overview/current-state.md)
- 当前模块架构：[../../30-module-architecture/overview.md](../../30-module-architecture/overview.md)
- completed：[../completed/overview.md](../completed/overview.md)
