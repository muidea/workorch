# WorkOrch Module Architecture Overview

Status: active

Type: module-architecture

Last Updated: 2026-08-08

## Purpose

本目录说明 WorkOrch 代码模块的职责、依赖关系、实现落点与跨模块边界。

本层只回答：

- 模块负责什么
- 模块依赖什么
- 模块暴露什么
- 模块如何参与 Runtime 主链
- 模块拥有或投影哪些数据
- 主要代码路径落在哪里

## Production Mainline

生产主链仅为 **Runtime**：

- application 构造 typed `RuntimeBootstrap` 并提交 owner intent；Run owner 通过 SM-1 private event 创建/迁移 Run，
  restart successor 只能在 SM-4 已持久化 Workspace release acknowledgement 后创建
- Orchestrator `DriveRuntime` 作为唯一同步推进合同
- reducer → authority transaction → prerequisite | candidate → sealed dispatch → operation evidence → governance wait / terminal

**双层语义：**

| 层 | 含义 | 是否生产状态机 |
| --- | --- | --- |
| 层 A（语义 / profile） | 中立 stage 语言、output policy、profile 映射、LLM 意图与 evidence 严格度 | 否；不是 Orchestrator 状态名 |
| 层 B（实现） | Runtime facts / focus / requiredOutput / candidate / sealed dispatch | 是 |

已删除且不得作为生产约束的 禁止的 合同：`StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、自动 replan successor、通用 RuntimeAdapter registry 主链。

权威实现说明以模块 `readme.md` 与 `internal/pkg/workorchruntimecontract` 为准；核心设计合约见 `../00-core/runtime-stage-contract.md`。

## Reading Order

1. [architecture-overview.md](architecture-overview.md)
2. [repository-structure.md](repository-structure.md)
3. [application-workorchd.md](application-workorchd.md)
4. [kernel-orchestrator-governance.md](kernel-orchestrator-governance.md)
5. [blocks-workspace-run-result.md](blocks-workspace-run-result.md)
6. [blocks-runtime-adapters.md](blocks-runtime-adapters.md)
7. [mvvp-workspace-viewmodel-architecture.md](mvvp-workspace-viewmodel-architecture.md)
8. [llm-provider-assembly.md](llm-provider-assembly.md)
9. [storage-readmodel.md](storage-readmodel.md)
10. [projection-first-readmodel-io-closure-design.md](projection-first-readmodel-io-closure-design.md)

历史 禁止的 状态环 / resume-gap / StagePacket 设计文档若仍在本目录，仅作归档参考，不构成生产主链约束。

## Cross-Layer Boundary

- 业务对象定义属于 `10-domain-model/`
- 运行链路属于 `20-runtime-flow/`
- 核心 runtime contract 属于 `00-core/`
- software-dev / workspace / MCP 等特化能力属于 `40-profiles-capabilities/`
