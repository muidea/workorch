# WorkOrch Product Design Overview

Status: normative

Type: product-design-index

Last Verified: 2026-08-21

## Purpose

本目录从产品视角说明 WorkOrch 要解决的问题、对用户暴露的行为，以及这些行为如何映射到现行 Runtime。
它不重新定义状态机、领域字段或 HTTP 合同。

## Current Product Baseline

- 用户在 Workspace 中维护 Goal、DoD、模板、Soul、模型选择和交付目录。
- Goal/DoD 先进入 Workspace-owner pre-seal candidate；只有 complete all-of admission 可以 promotion，歧义必须阻塞建票。
- 用户通过 workspace-scoped execution intent 请求 `resume` 或 `restart`，不直接创建、启动或指定某个 Run。
- Run owner 管理 SM-1 与 restart replacement transaction；Orchestrator 独占 SM-2、SM-3、SM-4。
- capability owner 只执行已经密封的 concrete typed operation，并返回事实回执。
- Result owner 保存 Orchestrator 已裁决并密封的终态结果；Workspace owner确认终态并释放 active slot。
- bounded recoverable runtime wait只在condition、trigger/deadline和budget完整时成立；它不创建治理ticket或新SM-1 state。
- coverage、assessment、evidence三轴和execution-budget view均为owner/aggregate事实的只读产品投影。
- Web/TUI 只提交意图、治理 resolution 和资源命令，并消费 projection。

生产主链仅为 `DriveRuntime`、reducer 与 sealed dispatch。中立 stage 名称可以用于 profile、Soul 和用户解释，
但不得成为第二套生产状态机。

## Reading Order

1. [PRD](prd.md)
2. [Product Architecture](product-architecture.md)

Product Architecture owns the user/resource interaction view. Formal objects and owner invariants continue in the Domain Model;
cross-owner sequences continue in Runtime Flow. This area does not maintain a third derived object/flow mapping.

## Requirement Expansion Map

本表只把 [PRD](prd.md) 的产品结果路由到唯一专题，不复制字段或状态机定义：

| PRD outcome | Primary design expansion |
| --- | --- |
| Goal/DoD、pre-seal admission、all-of 与两类 clarification | [Definition Convergence](../00-core/definition-convergence/overview.md) |
| CWI mapping、coverage/evidence/assessment 与 closeout | [Semantic Convergence](../00-core/runtime/semantic-convergence.md)；[Result Closeout](../10-domain-model/result-closeout.md) |
| bounded semantic planning、binding 与 same-focus repair | [Planning And Execution](../00-core/runtime/flows/planning-execution.md)；[Repair And Restart](../00-core/runtime/flows/repair-and-restart.md) |
| purpose-bound provider request、context 与 cache | [Runtime Request And Cache](../00-core/runtime-request-cache-note.md) |
| owner feasibility、candidate、sealed dispatch 与 receipt | [Candidate And Dispatch](../00-core/runtime/candidate-and-dispatch.md)；[Capability Catalog](../10-domain-model/capability-catalog.md) |
| evidence judgement、assessment 与 formal Result | [Verification And Acceptance](../00-core/runtime/flows/verification-acceptance.md)；[Evidence And Result](../00-core/runtime/flows/evidence-result.md) |
| owner revision set 与 freshness | [Result Closeout](../10-domain-model/result-closeout.md)；[Runtime Invariants](../00-core/runtime/invariants-and-change-gates.md) |
| governance wait、recoverable wait 与 failure | [Governance Domain](../10-domain-model/governance-decision-approval.md)；[Recoverable Wait](../00-core/runtime/recoverable-wait.md) |
| Resume、Restart、recovery 与 delivery | [Repair And Restart](../00-core/runtime/flows/repair-and-restart.md)；[Four State Machines](../00-core/runtime/state-machines.md) |
| context、cache、Memory 与 instruction assets | [Context Assembly](../00-core/runtime-request-context-assembly.md)；[Memory](../10-domain-model/memory.md)；[Instruction Assets](../10-domain-model/instruction-assets.md) |
| execution budget | [Execution Budget](../00-core/runtime/execution-budget.md) |

接口可观察行为统一从 [Product Interfaces](../50-product-interface/overview.md) 展开；发布级跨专题证明统一进入
[Deployed Convergence Scenarios](../50-product-interface/deployed-convergence-scenarios.md)。下层文档若无法映射回本表中的
PRD outcome，不得自行增加产品语义。

## Authority

- Runtime 设计合同：[runtime-stage-contract.md](../00-core/runtime-stage-contract.md)
- 领域对象：[domain model](../10-domain-model/overview.md)
- 执行流程：[runtime flow](../00-core/runtime/flows/overview.md)
- 组件边界：[module architecture](../30-module-architecture/overview.md)
- 接口合同：[product interface](../50-product-interface/overview.md)
- 验证入口：[Verification By Subject](../README.md#verification-by-subject)

发生冲突时，上述专项文档优先；本目录只保留产品级摘要。

## Documentation Rules

- 产品需求描述用户价值和可观察行为，不新增内部字段或迁移历史。
- “继续”映射到当前 Run 的 `resume`；“按新定义重新执行”映射到显式 `restart`。
- `restart` 由 Workspace owner 受理、Run owner 持久化 `ReplacementIntent` 并在终态投递完成后创建 successor。
- `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、应用层自动 successor 和 map authority
  rebuild 仅可作为已移除边界被提及。
- 软件开发、Web 搜索、MCP 等领域行为属于 profile/capability，不得写入 core Runtime。
