# WorkOrch Product Design Overview

Status: active

Type: product-design-index

Last Verified: 2026-08-16

## Purpose

本目录从产品视角说明 WorkOrch 要解决的问题、对用户暴露的行为，以及这些行为如何映射到现行 Runtime。
它不重新定义状态机、领域字段或 HTTP 合同。

## Current Product Baseline

- 用户在 Workspace 中维护 Goal、DoD、模板、Soul、模型选择和交付目录。
- 用户通过 workspace-scoped execution intent 请求 `resume` 或 `restart`，不直接创建、启动或指定某个 Run。
- Run owner 管理 SM-1 与 restart replacement transaction；Orchestrator 独占 SM-2、SM-3、SM-4。
- capability owner 只执行已经密封的 concrete typed operation，并返回事实回执。
- Result owner 保存 Orchestrator 已裁决并密封的终态结果；Workspace owner确认终态并释放 active slot。
- Web/TUI 只提交意图、治理 resolution 和资源命令，并消费 projection。

生产主链仅为 `DriveRuntime`、reducer 与 sealed dispatch。中立 stage 名称可以用于 profile、Soul 和用户解释，
但不得成为第二套生产状态机。

## Reading Order

1. [PRD](prd.md)
2. [Product Architecture](product-architecture.md)
3. [Business Model And Flow Contracts](business-model-flow-contracts.md)

## Authority

- Runtime 设计合同：[runtime-stage-contract.md](../00-core/runtime-stage-contract.md)
- 领域对象：[domain model](../10-domain-model/overview.md)
- 执行流程：[runtime flow](../20-runtime-flow/overview.md)
- 组件边界：[module architecture](../30-module-architecture/overview.md)
- 接口合同：[product interface](../50-product-interface/overview.md)
- 验证入口：[verification](../60-verification/overview.md)

发生冲突时，上述专项文档优先；本目录只保留产品级摘要。

## Documentation Rules

- 产品需求描述用户价值和可观察行为，不新增内部字段或迁移历史。
- “继续”映射到当前 Run 的 `resume`；“按新定义重新执行”映射到显式 `restart`。
- `restart` 由 Workspace owner 受理、Run owner 持久化 `ReplacementIntent` 并在终态投递完成后创建 successor。
- `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、应用层自动 successor 和 map authority
  rebuild 仅可作为已移除边界被提及。
- 软件开发、Web 搜索、MCP 等领域行为属于 profile/capability，不得写入 core Runtime。
