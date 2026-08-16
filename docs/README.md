# WorkOrch Documentation Map

Status: active

Type: documentation-index

Last Verified: 2026-08-16

## Purpose

本文是 `docs/` 的唯一正式导航入口。当前文档按“核心合同 → 当前事实 → 产品/领域 → Runtime flow → 组件实现 →
profile/capability → 产品接口 → 验证 → 未关工作”组织；历史设计、完成记录和 superseded 路径统一位于
`90-archive/`，不得反向约束当前实现。

## Authority Order

1. [Runtime 核心合同](00-core/runtime-stage-contract.md)：四状态机、中立语义、Owner 与禁止迁移。
2. [Runtime request/cache note](00-core/runtime-request-cache-note.md)：C0～C5、ProviderCeiling 和请求审计。
3. `internal/modules/kernel/orchestrator/readme.md` 与 `internal/pkg/workorchruntimecontract`：Runtime 实现 SoT。
4. 各 Initiator/Module/Block `readme.md`：组件职责、Owned state、concrete EventHub 合同和非职责。
5. [Current State](01-overview/current-state.md)：当前仓库事实摘要。
6. `20-runtime-flow/`、`30-module-architecture/` 和其它专题文档：解释当前合同，不新增权威。
7. [Active Roadmap](70-roadmap/active/overview.md)：只包含真实未关工作。
8. `90-archive/`：历史证据，不是当前合同。

代码与文档不一致时，先按以上权威顺序确认是否为实现缺陷或文档漂移；不得用较低层文档覆盖较高层合同。

## Recommended Reading

1. [Current State](01-overview/current-state.md)
2. [Runtime 核心合同](00-core/runtime-stage-contract.md)
3. `internal/modules/kernel/orchestrator/readme.md`
4. [Runtime Flow Overview](20-runtime-flow/overview.md)
5. [Module Architecture Overview](30-module-architecture/overview.md)
6. [Component Documentation Alignment](01-overview/component-documentation-alignment.md)
7. [Runtime Owner Convergence Matrix](60-verification/runtime-owner-convergence-matrix.md)
8. [Active Roadmap](70-roadmap/active/overview.md)

## Directory Guide

| 目录 | 当前职责 |
| --- | --- |
| `00-core/` | 唯一 Runtime 核心合同与 request/cache 从属合同 |
| `01-overview/` | 当前事实、组件文档对齐和真实未关项 |
| `05-product/` | 产品定位、业务模型和产品架构 |
| `10-domain-model/` | Workspace、Run、Result、治理、Memory 与 observability 对象不变量 |
| `20-runtime-flow/` | activation、planning、execution、evidence、repair、acceptance 数据流 |
| `30-module-architecture/` | 22 个 framework 运行单元、process service、storage/read model 边界 |
| `40-profiles-capabilities/` | Template、Soul、Skill、MCP 与具体 capability/profile 语义 |
| `50-product-interface/` | HTTP、Web、TUI、部署接口 |
| `60-verification/` | 自动化、部署、竞态、负载与 Runtime owner 回归合同 |
| `70-roadmap/active/` | 当前未关的代码/内容收口与 deployed/live 验收 |
| `70-roadmap/candidates/` | 尚未晋级的需求候选，不是缺陷队列 |
| `90-archive/completed/` | 已完成设计/实施计划 |
| `90-archive/records/` | 审计、benchmark、live 和代码收口记录 |
| `90-archive/superseded/` | 已被当前合同替代的设计 |

## Read By Task

- Runtime 状态机与 Owner：
  [runtime-stage-contract.md](00-core/runtime-stage-contract.md)、
  [orchestrator README](../internal/modules/kernel/orchestrator/readme.md)。
- LLM 请求、缓存与预算：
  [runtime-request-cache-note.md](00-core/runtime-request-cache-note.md)、
  [context-assembly-flow.md](20-runtime-flow/context-assembly-flow.md)、
  [llm-provider-assembly.md](30-module-architecture/llm-provider-assembly.md)。
- 组件边界：
  [architecture-overview.md](30-module-architecture/architecture-overview.md)、
  [repository-structure.md](30-module-architecture/repository-structure.md)、
  [component-documentation-alignment.md](01-overview/component-documentation-alignment.md)。
- Workspace/Run/Result：
  [workspace-goal-dod.md](10-domain-model/workspace-goal-dod.md)、
  [run-step-result-artifact.md](10-domain-model/run-step-result-artifact.md)、
  [blocks-workspace-run-result.md](30-module-architecture/blocks-workspace-run-result.md)。
- Capability：
  [profiles-capabilities overview](40-profiles-capabilities/overview.md)、
  [blocks-runtime-adapters.md](30-module-architecture/blocks-runtime-adapters.md)。
- HTTP/TUI/Web/部署：
  [product-interface overview](50-product-interface/overview.md)、
  [service-api.md](50-product-interface/service-api.md)、
  [deployment.md](50-product-interface/deployment.md)。
- 验收：
  [verification overview](60-verification/overview.md)、
  [runtime-owner-convergence-matrix.md](60-verification/runtime-owner-convergence-matrix.md)、
  [service-acceptance.md](60-verification/service-acceptance.md)。
- 当前 live 工作：
  [Runtime positive convergence](70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md)、
  [Workspace reasoning effort](70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md)。

## Current Documentation Rules

- 当前 Runtime aggregate 只支持一个 development baseline；活动文档不使用历史 schema 号定义兼容链。
- 组件 README 不保留阶段编号、dated run、LOC/CC 快照、旧文件拆分过程或“待随提交归档”说明。
- 已删除的 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、通用 RuntimeAdapter registry 和
  自动 replan successor 只可作为明确的禁止项出现。
- Capability execute/prerequisite/feasibility command/result 由接收 capability owner 的 `pkg/events` 定义；
  Orchestrator 是调用方，不拥有通用 capability wire。
- `active/` 只保留未关计划；已完成代码收口和 dated live 记录必须迁入 archive。
