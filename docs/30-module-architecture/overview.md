# WorkOrch Module Architecture Overview

Status: active

Type: module-architecture

Last Updated: 2026-06-09

## Purpose

本目录用于说明 WorkOrch 代码模块的职责、依赖关系、实现落点和跨模块边界。

本层只回答：

- 模块负责什么
- 模块依赖什么
- 模块暴露什么
- 模块参与哪些 runtime flow
- 模块拥有或投影哪些数据
- 主要代码路径落在哪里

## Reading Order

1. [architecture-overview.md](architecture-overview.md)
2. [repository-structure.md](repository-structure.md)
3. [application-workorchd.md](application-workorchd.md)
4. [mvvp-workspace-viewmodel-architecture.md](mvvp-workspace-viewmodel-architecture.md)
5. [kernel-workspace-run-result.md](kernel-workspace-run-result.md)
6. [kernel-orchestrator-governance.md](kernel-orchestrator-governance.md)
7. [kernel-context-capability.md](kernel-context-capability.md)
8. [blocks-runtime-adapters.md](blocks-runtime-adapters.md)
9. [llm-provider-assembly.md](llm-provider-assembly.md)
10. [storage-readmodel.md](storage-readmodel.md)
11. [projection-first-readmodel-io-closure-design.md](projection-first-readmodel-io-closure-design.md)

## Cross-Layer Boundary

- 业务对象定义属于 `10-domain-model/`
- 运行链路属于 `20-runtime-flow/`
- 核心 runtime contract 属于 `00-core/`
- software-dev / workspace / MCP 等特化能力属于 `40-profiles-capabilities/`
