# WorkOrch Product Design Overview

Status: active

Type: product-design

Last Updated: 2026-07-01

## Purpose

本目录是 WorkOrch 面向产品需求、产品架构、业务模型、业务流程和跨模块交互合同的统一整理入口。

它只做三件事：

- 把分散在 `00-core` 到 `60-verification` 的稳定内容整理成产品可读的完整视图。
- 明确产品需求、功能设计、架构层级、业务模型、流程合同和模块交互之间的对应关系。
- 给出一致性规则，避免后续继续出现同一概念在多个文档中重复定义、互相冲突。

## Reading Order

1. [prd.md](prd.md)
2. [product-architecture.md](product-architecture.md)
3. [business-model-flow-contracts.md](business-model-flow-contracts.md)

## Authority Boundary

本目录是产品设计整理层，不替代以下权威来源：

- runtime core contract: `../00-core/runtime-stage-contract.md`
- domain model field boundary: `../10-domain-model/`
- runtime flow details: `../20-runtime-flow/`
- code module boundary: `../30-module-architecture/`
- profile/capability specialization: `../40-profiles-capabilities/`
- product interface/API details: `../50-product-interface/`
- acceptance/verifier/audit details: `../60-verification/`

若本目录与上述权威来源冲突：

- runtime 中立规则以 `00-core` 为准。
- 领域对象字段以 `10-domain-model` 为准。
- 运行流程顺序以 `20-runtime-flow` 为准。
- 模块职责和依赖以 `30-module-architecture` 为准。
- 具体 profile / Soul / template / MCP 策略以 `40-profiles-capabilities` 为准。
- HTTP/TUI/Web 产品接口以 `50-product-interface` 为准。

## Consistency Rules

- 一个概念只允许有一个权威定义，其余文档只能引用或摘要。
- Product requirement 只表达用户价值、场景和验收，不定义 runtime 内部状态机。
- Product architecture 只表达层级、职责和依赖方向，不新增对象字段。
- Business model / flow contract 只表达对象关系、流程输入输出和模块交互，不覆盖 profile 具体策略。
- 软件开发、Web 搜索、MCP 使用、Skill 选择等具体行为只能作为 profile/capability 策略存在，不能写成 core runtime 规则。

