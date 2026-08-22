# Component Contract Inventory

Status: normative

Type: component-contract-index

Last Verified: 2026-08-21

## Purpose

本文是 registered framework units 与各自 component contract 的唯一规范目录。它陈述 1 Initiator、4 Modules、17
Blocks 的 owner 边界，不保存历史偏差、重构过程或 closure 记录。Runtime 语义仍以
[Runtime Core](../00-core/runtime-stage-contract.md)、[Orchestrator Contract](../../internal/modules/kernel/orchestrator/readme.md)
和 `internal/pkg/workorchruntimecontract` 为准。

## Initiator

| Unit | Current responsibility | Component contract |
| --- | --- | --- |
| `workorchruntime` | repository provider/factory and infrastructure lifecycle only | [contract](../../internal/initiators/workorchruntime/readme.md) |

## Modules

| Unit | Current responsibility | Component contract |
| --- | --- | --- |
| `application/workorchd` | typed bootstrap, HTTP/SSE, application use cases and projection; no Runtime drive | [contract](../../internal/modules/application/workorchd/readme.md) |
| `kernel/orchestrator` | sole SM-2/SM-3/SM-4 reducer, drive, evidence, governance resolution and delivery authority | [contract](../../internal/modules/kernel/orchestrator/readme.md) |
| `kernel/governance` | policy/risk evaluation and immutable governance facts | [contract](../../internal/modules/kernel/governance/readme.md) |
| `kernel/system` | startup coordination for owner-defined default-resource ensure commands | [contract](../../internal/modules/kernel/system/readme.md) |

## Resource Owner Blocks

| Unit | Owned state | Component contract |
| --- | --- | --- |
| `workspace` | Workspace, Goal, DoD, Template, Definition admission and execution intent | [contract](../../internal/modules/blocks/workspace/readme.md) |
| `run` | SM-1 lifecycle, lineage, replacement intent and opaque Runtime aggregate persistence | [contract](../../internal/modules/blocks/run/readme.md) |
| `result` | formal Result/Artifact and sealed terminal material | [contract](../../internal/modules/blocks/result/readme.md) |
| `decision` | DecisionTicket and Resolution | [contract](../../internal/modules/blocks/decision/readme.md) |
| `approval` | Approval lifecycle and decision | [contract](../../internal/modules/blocks/approval/readme.md) |
| `memory` | MemoryItem and RecallTrace | [contract](../../internal/modules/blocks/memory/readme.md) |
| `skill` | WorkOrch Registry Skill content、binding与typed provenance；不拥有SkillHub Catalog | [contract](../../internal/modules/blocks/skill/readme.md) |
| `soul` | Soul definition, revision and default resource | [contract](../../internal/modules/blocks/soul/readme.md) |
| `mcpregistry` | MCP server/capability catalog and selection metadata | [contract](../../internal/modules/blocks/mcpregistry/readme.md) |
| `eventstream` | bounded audit EventRecord storage and query | [contract](../../internal/modules/blocks/eventstream/readme.md) |

## Capability Blocks

| Unit | Current responsibility | Component contract |
| --- | --- | --- |
| `llm` | provider catalog/config snapshot, ceiling, LLM/embedding factual calls | [contract](../../internal/modules/blocks/llm/readme.md) |
| `file` | typed file create/read/edit/patch operations and target revision ownership | [contract](../../internal/modules/blocks/file/readme.md) |
| `git` | typed Git operations and factual results | [contract](../../internal/modules/blocks/git/readme.md) |
| `shell` | typed shell operations and managed-validation facts | [contract](../../internal/modules/blocks/shell/readme.md) |
| `ssh` | typed SSH connection/execution operations | [contract](../../internal/modules/blocks/ssh/readme.md) |
| `web` | typed web search/fetch operations | [contract](../../internal/modules/blocks/web/readme.md) |
| `mcp` | typed MCP operation execution; distinct from registry metadata ownership | [contract](../../internal/modules/blocks/mcp/readme.md) |

`internal/services/workorch` is the process shell for server/TUI/remote CLI modes, not a framework unit. Its contract is
[internal/services/workorch/readme.md](../../internal/services/workorch/readme.md).

## Shared Boundary

- each formal state has one owner and only that owner's private reducer chooses transitions;
- `scripts/state-transition-owners.manifest` is the sole verification-only inventory for formal machines and owner-private
  transition/effect gates; it must never become a production dispatcher or shared reducer registry;
- cross-owner I/O is EventHub plus receiver-defined concrete typed command/data/result;
- no owner exposes a sibling service or repository facade;
- Workorchd owns entry/use-case/projection concerns, Run owns SM-1, Orchestrator owns SM-2/3/4, capability blocks own effects,
  and Result persists already-judged outcomes;
- projection, Eventstream, HTTP DTO and process records are non-authoritative;
- adding, removing or reclassifying a registered unit requires updating this inventory and its component contract in the same change.
