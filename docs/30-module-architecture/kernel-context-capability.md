# Kernel Context / Capability Architecture

Status: active

Type: module-architecture

Last Updated: 2026-05-29

## Purpose

本文件说明 kernel 中围绕上下文、能力和记忆的模块职责。

## Main Modules

### `kernel/soul`

职责：

- Soul model
- capability allowlist / policy validation

### `kernel/skill`

职责：

- Skill metadata
- trigger / selection related business contract

### `kernel/mcp`

职责：

- MCP server registry
- capability metadata
- policy / selection contract

### `kernel/memory`

职责：

- Memory CRUD
- workspace-scoped recall/search contract
- memory write policy

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

## Open Migration Notes

- profile-specific 的 Skill / Soul / MCP 语义后续还要继续迁到 `40-profiles-capabilities/`
- selectedContext 的 request-side 装配细节后续迁到 `llm-provider-assembly.md`
