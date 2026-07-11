# Kernel Context / Capability Architecture

Status: active

Type: module-architecture

Last Updated: 2026-05-29

## Purpose

本文件说明 kernel 中围绕上下文、能力和记忆的模块职责。

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

## Boundary

这些模块表达“有哪些能力、允许什么、可复用什么上下文”，
但不直接执行外部 side effect。

## Flow Participation

这些模块主要参与：

- activation context resolution
- selectedContext assembly
- memory recall
- capability policy enforcement

## Source Documents

- `architecture-overview.md`
- `../10-domain-model/context-capability-memory.md`
- `../20-runtime-flow/context-assembly-flow.md`
- `../40-profiles-capabilities/mcp/capability-management.md`
