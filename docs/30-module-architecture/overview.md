# WorkOrch Module Architecture

Status: normative

Type: module-architecture-index

Last Verified: 2026-08-18

## Purpose

本目录只说明 framework unit、process service、focused package、owner storage 与 protocol/read-model 的代码落点。
正式组件 topic、command/result、owned state 和内部交互以对应 unit `readme.md` 为实现 SoT。

## Topics

| Topic | Contract |
| --- | --- |
| stable layers, owner map and dependency direction | [Architecture Overview](architecture-overview.md) |
| physical 1+4+17 inventory and package placement | [Repository Structure](repository-structure.md) |
| application use-case composition | [Application Workorchd](application-workorchd.md) |
| Orchestrator/Governance/System module boundaries | [Kernel Modules](kernel-orchestrator-governance.md) |
| all resource and capability Block boundary pattern | [Block Boundaries](block-boundaries.md) |
| owner persistence and derived read-model separation | [Storage And Read Model](storage-readmodel.md) |
| exact registered unit and README mapping | [Component Contract Inventory](../01-overview/component-documentation-alignment.md) |

## Architecture Rules

- 1 Initiator、4 Modules、17 Blocks 是当前 framework unit baseline；`internal/services/workorch` 是 process shell，不是 Module。
- Module 组合多个 owner contract 完成业务闭环；Block 只拥有一个资源聚合或技术能力；Initiator 只提供一种无业务状态基础设施能力。
- 使用 EventHub 的 Module/Block 在自身 `biz/` 内嵌 Base；`module.go` 只接线，protocol `service/` 不持有 Hub。
- 跨 owner runtime I/O 使用接收 owner 定义的 concrete typed EventHub contract；禁止 service/repository 直连和通用 envelope。
- application composes use cases/projection but does not drive Runtime or write another owner's state。

产品语义见 `05-product`，对象见 `10-domain-model`，Runtime 顺序与验证见 `00-core`，组件实现边界见本目录，
profile/capability 验证见 `40-profiles-capabilities`，接口与部署验收见 `50-product-interface`。
