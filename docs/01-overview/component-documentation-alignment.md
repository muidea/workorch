# Component Contract Inventory

Status: normative

Type: component-contract-index

Last Verified: 2026-08-30

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
| `kernel/governance` | policy/risk evaluation, immutable governance facts and owner-local AutoGovernanceTrace coordination/recovery | [contract](../../internal/modules/kernel/governance/readme.md) |
| `kernel/system` | startup coordination for owner-defined default-resource ensure commands | [contract](../../internal/modules/kernel/system/readme.md) |

## Resource Owner Blocks

| Unit | Owned state | Component contract |
| --- | --- | --- |
| `workspace` | Workspace/Goal/DoD/Template、pre-seal admission candidate、sealed startup input、activation admission、active-run projection、execution intent and deletion journal | [contract](../../internal/modules/blocks/workspace/readme.md) |
| `run` | SM-1 lifecycle、lineage/replacement、Step/Checkpoint、Workspace retirement fence and opaque Runtime aggregate persistence | [contract](../../internal/modules/blocks/run/readme.md) |
| `result` | formal Result/Artifact、sealed terminal material and deletion tombstone | [contract](../../internal/modules/blocks/result/readme.md) |
| `decision` | DecisionTicket/Resolution、Definition clarification binding、Runtime governance proposal/delivery checkpoint and deletion tombstone | [contract](../../internal/modules/blocks/decision/readme.md) |
| `approval` | Approval aggregate/authorization context/lifecycle、Runtime resolution proposal/checkpoints and deletion tombstone | [contract](../../internal/modules/blocks/approval/readme.md) |
| `memory` | MemoryItem、RecallTrace and deletion tombstone | [contract](../../internal/modules/blocks/memory/readme.md) |
| `skill` | WorkOrch Registry Skill content、binding与typed provenance；不拥有SkillHub Catalog | [contract](../../internal/modules/blocks/skill/readme.md) |
| `soul` | Soul definition/revision, five typed allowlists and default resources | [contract](../../internal/modules/blocks/soul/readme.md) |
| `mcpregistry` | MCP server/capability catalog and selection metadata | [contract](../../internal/modules/blocks/mcpregistry/readme.md) |
| `eventstream` | immutable audit records, global/workspace ordering, high-watermark query and deletion tombstone | [contract](../../internal/modules/blocks/eventstream/readme.md) |

## Capability Blocks

| Unit | Current responsibility | Component contract |
| --- | --- | --- |
| `llm` | provider catalog/config snapshot, ceiling, LLM/embedding factual calls | [contract](../../internal/modules/blocks/llm/readme.md) |
| `file` | typed file effects plus workspace manifest, target revision and factual feasibility observations | [contract](../../internal/modules/blocks/file/readme.md) |
| `git` | typed Git operations and factual results | [contract](../../internal/modules/blocks/git/readme.md) |
| `shell` | typed shell operations and managed-validation facts | [contract](../../internal/modules/blocks/shell/readme.md) |
| `ssh` | typed SSH connection/execution operations | [contract](../../internal/modules/blocks/ssh/readme.md) |
| `web` | typed web search/fetch operations | [contract](../../internal/modules/blocks/web/readme.md) |
| `mcp` | typed MCP operation execution; distinct from registry metadata ownership | [contract](../../internal/modules/blocks/mcp/readme.md) |

`internal/services/workorch` is the process shell for server/TUI/remote CLI modes, not a framework unit. Its contract is
[internal/services/workorch/readme.md](../../internal/services/workorch/readme.md).

## Component Contract Shape

每个 registered unit 的 `readme.md` 都使用同一最小合同骨架；组件可以在四个入口下用三级标题增加 owner-local 专题，但顶层
只能保留以下四个入口，且不能省略、改写或改变顺序：

1. `功能范围与边界`：只陈述本组件拥有的状态、能力和生命周期，并明确不拥有的相邻责任；
2. `组件交互及合同`：按方向列出接收 owner 定义的 concrete command/data/result 与外部技术适配，不复制其它 owner 的 reducer 规则；
3. `不变量与非职责`：列出 fail-closed、scope、identity、effect、资源释放和禁止依赖等稳定约束；
4. `能力验收`：把每项公开能力映射到可执行测试证据，并明确 normal、invalid/scope、duplicate/idempotency、
   stale/freshness、restart/recovery、lifecycle、effect/receipt 与 boundary 中适用的维度。

“完整”“已收口”“当前无缺口”不是合同证据。只有对应的 producer/consumer、合法与非法输入、owner identity、持久化
cut point 和资源生命周期均有可重复测试时，验收矩阵才能标为覆盖；不适用维度必须说明原因。真实 provider、远端 endpoint、
外部网络和主机差异属于部署验收，必须链接现有 service/deployed acceptance，不得用它们掩盖仓库内可执行合同缺口。

README 只保留当前有效事实和稳定验收入口，不保存历史迁移说明、closure diary、未来增强建议、checkbox 进度清单或逐实现
函数复述。Runtime 的共享语义链接 Core/Orchestrator SoT；能力 Block 只描述自身输入校验、effect 和 factual receipt。

## Shared Boundary

- each formal state has one owner and only that owner's private reducer chooses transitions;
- `scripts/state-transition-owners.manifest` is the sole verification-only inventory for formal machines and owner-private
  transition/effect gates; each privileged owner sink has exactly one owner-root call in the declared private dispatcher, while
  any other production reference must be an explicitly inventoried repository storage delegation; the inventory must never
  become a production dispatcher or shared reducer registry;
- cross-owner I/O is EventHub plus receiver-defined concrete typed command/data/result;
- no owner exposes a sibling service or repository facade;
- repository-family access is resolved only through `internal/pkg/workorchdeps`; other production packages cannot call the
  Initiator `RepositoryProvider()` directly;
- Workorchd owns entry/use-case/projection concerns, Run owns SM-1, Orchestrator owns SM-2/3/4, capability blocks own effects,
  and Result persists already-judged outcomes;
- projection, Eventstream, HTTP DTO and process records are non-authoritative;
- adding, removing or reclassifying a registered unit requires updating this inventory and its component contract in the same change.
