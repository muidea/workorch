# WorkOrch Module Architecture Overview

Status: active

Type: module-architecture

Last Verified: 2026-08-16

## Purpose

本目录说明当前代码单元的职责、依赖、owner 边界和 Runtime 参与方式。它不保存阶段性拆分计划或历史迁移
checklist。

## Production Mainline

```text
workorchd typed bootstrap + Workspace execution intent
  -> Workspace owner definition/scope admission
  -> Run owner SM-1 + opaque RuntimeStateRecord CAS
  -> Orchestrator SM-2/SM-3/SM-4 DriveRuntime
  -> concrete capability owner operation
  -> Orchestrator evidence/terminal seal
  -> Result -> Workspace -> Run owner-local delivery
```

Current-baseline Restart uses Run-owned `ReplacementIntent`; successor creation occurs only after predecessor terminal
delivery acknowledgment. Application, Workspace and Orchestrator cannot create or link it early.

## Reading Order

1. [Architecture Overview](architecture-overview.md)
2. [Repository Structure](repository-structure.md)
3. [Application Workorchd](application-workorchd.md)
4. [Kernel Orchestrator / Governance](kernel-orchestrator-governance.md)
5. [Workspace / Run / Result](blocks-workspace-run-result.md)
6. [Capability Adapters](blocks-runtime-adapters.md)
7. [Kernel Context / Capability](kernel-context-capability.md)
8. [LLM Provider Assembly](llm-provider-assembly.md)
9. [Storage And Read Model](storage-readmodel.md)

The reviewed per-unit definition is maintained in
[Component Documentation Alignment](../01-overview/component-documentation-alignment.md). Historical view-model and
projection closure designs are archived and no longer part of this reading order.

## Boundary Rules

- Owner state transition is selected only by that owner's private reducer/internal event.
- Cross-owner interaction uses EventHub plus a concrete receiving/producing owner command/result/data.
- Application composes use cases and projections; it is not a Runtime/Run/Workspace/Result owner.
- Capability blocks execute side effects and return facts; they do not judge evidence or terminal state.
- Eventstream and projection are read/audit surfaces, never Runtime replay or owner repair authority.
- Profile/template/Soul/MCP-specific policy stays outside the business-neutral kernel reducer.
- Removed StagePacket/NextAction/graph-dispatch/context-patch paths are not supported authority. Static boundary checks reject
  them from active provider resources, verifiers and authority packages; read-only compatibility projection is not a second
  runtime path.

## Documentation Layers

- business objects: `docs/10-domain-model`;
- execution sequence: `docs/20-runtime-flow`;
- code placement and dependency: this directory;
- feature/profile behavior: `docs/40-profiles-capabilities`;
- API/TUI/Web: `docs/50-product-interface`;
- acceptance: `docs/60-verification`;
- history: `docs/90-archive`.
