# MCP And Capability Catalog Domain Model

Status: normative

Type: domain-model

Last Verified: 2026-08-18

## Objects And Owners

MCP Server 是外部能力提供者的受管登记对象，MCP registry 由 `blocks/mcpregistry` 唯一拥有。Capability 是工具、资源、
提示或其他可执行能力的结构化描述；file/git/shell/ssh/web/mcp/llm 等 concrete capability owner 只执行自身 typed operation。

Catalog declaration 表达：

- stable capability identity, kind and provider binding;
- typed input/output contract and schema revision;
- enablement, scope, risk and governance policy;
- current feasibility/readiness observation contract.

## Runtime Boundary

- Template/Profile 只声明 capability requirement/ref，activation 封存 exact catalog/policy snapshot；
- capability owner 返回 feasibility 或 concrete receipt，不选择 focus、CWI 或 next action；
- Orchestrator 只对 sealed candidate/dispatch 与 typed receipt 作中立校验，不理解 concrete tool/domain semantics；
- MCP-specific transport, credentials and discovery stay in adapter/owner layers and never enter Runtime authority maps.

## Invariants

- capability selection and snapshot are traceable and immutable within one activation;
- an unavailable required capability becomes a typed prerequisite fact；只有owner同时提供recovery condition、trigger/deadline
  和budget适用信息时，Orchestrator才可形成bounded recoverable runtime wait；真实外部选择/授权才进入governance，
  其它情况fail-closed，且均不得成为implicit alternate terminal branch；
- multiple operation variants for one fixed CWI are candidate feasibility choices, not Definition `one_of`;
- a capability receipt is factual input and does not itself grant accepted evidence or completion.

## Related Documents

- [Domain Model Overview](overview.md)
- [Runtime Capability Profiles](../40-profiles-capabilities/overview.md)
- [Block Boundaries](../30-module-architecture/block-boundaries.md)
