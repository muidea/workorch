# Kernel Context / Capability Architecture

Status: active

Type: module-architecture

Last Verified: 2026-08-16

## Purpose

本文件说明 kernel 中围绕上下文、能力和记忆的模块职责。

## Runtime Boundary

| 模块 | Runtime 边界 |
| --- | --- |
| `blocks/soul` / `blocks/skill` / `blocks/mcpregistry` | 提供 capability allowlist、metadata、selection contract；workorchd 经 EventHub 读取并封存进 fresh activation，Orchestrator 只消费 sealed snapshot；不执行 side effect，不拥有 Run aggregate |
| `blocks/memory` | MemoryItem / MemoryRecallTrace 的正式 owner；**经 EventHub** 供 Orchestrator / Governance / workorchd 查询与写入；不直接写 Run/Result，不裁决 acceptance |
| LLM-facing 上下文 | Orchestrator 组织有界 `RuntimeCandidateContext` / `PromptContext`；本层 owner 不重建 禁止的 execution-context map |

显式非目标：

- Memory 作为过程证据本体或唯一 truth source
- capability block 自行驱动 `StagePacket` / `NextAction` / `DispatchNextStep`
- 绕过 EventHub 持有 sibling owner repository

## Main Modules

### `blocks/soul`

职责：

- Soul model
- capability allowlist / policy validation

### `blocks/skill`

职责：

- Skill metadata
- trigger / selection related business contract

### `blocks/mcp-registry`

职责：

- MCP server registry
- capability metadata
- policy / selection contract

### `blocks/memory`

职责：

- Memory CRUD
- workspace-scoped recall/search contract
- memory write policy
- MemoryRecallTrace 的正式持久化与查询输入
- EventHub command/notification：`MemoryChangedData` / `MemoryRecallRecordedData`

## Boundary

这些模块表达“有哪些能力、允许什么、可复用什么上下文”，
但不直接执行外部 side effect，也不拥有 Runtime drive 状态。

## Flow Participation

这些模块主要参与：

- activation context resolution（写入/绑定 `RuntimeBootstrap` Activation 面）
- selectedContext / candidate context 的有界输入
- memory recall（过程证据经 RecallTrace；正文经 Memory owner）
- capability policy enforcement（allowlist / selection，非 execution）

## Source Documents

- `architecture-overview.md`
- `kernel-orchestrator-governance.md`
- `../10-domain-model/context-capability-memory.md`
- `../20-runtime-flow/context-assembly-flow.md`
- `../40-profiles-capabilities/mcp/capability-management.md`
- `internal/modules/blocks/memory/readme.md`
- `internal/modules/blocks/soul/readme.md`
