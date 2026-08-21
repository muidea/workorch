# Owner Persistence And Read-Model Boundary

Status: normative

Type: module-architecture

Last Verified: 2026-08-21

## Purpose

本文只定义 owner persistence、Eventstream audit 与 workorchd read model 的单向关系。repository layout/encoding 是
owner implementation detail；HTTP pagination、wait、raw-content 由 [Service Read API](../50-product-interface/service-read-api.md) 定义。

## Owner Repository Groups

| Data | Sole writer | Persistence meaning |
| --- | --- | --- |
| Workspace candidate/Definition/startup/active slot | Workspace Block | formal Workspace authority |
| Run lifecycle/lineage/replacement and opaque Runtime record | Run Block | SM-1 authority plus SM-2/3/4 opaque CAS storage |
| Runtime semantic facts/authority/candidate/evidence/delivery | Orchestrator | serialized inside opaque Runtime record; semantically Orchestrator-owned |
| Result/Artifact formal outcome | Result Block | already-judged terminal material |
| Decision/Approval/Memory/Soul/Skill/MCP Registry | corresponding Block | each owner resource |
| EventRecord | Eventstream Block | immutable audit record, never replay authority |

Initiator provides repository factories/groups only；it does not expose EventHub、bootstrap、Runtime policy or a cross-owner
repository container。Owner Biz receives only its repository group；sibling Module/Block cannot query it directly。

MCP Registry是包含Server、Capability与binding metadata的单一revisioned `RegistrySnapshot` aggregate。repository只提供
whole-snapshot `Load`与`Commit(expectedRevision)`；Server更新、Capability allowlist reconciliation和default ensure不能拆成
多个物理transaction，也不能由调用方拼接不同revision的list。

## Opaque Runtime CAS

Run owner stores Runtime aggregate as opaque `RuntimeStateRecord` and validates envelope scope、schema/current-baseline marker
and expected revision。Only Orchestrator decodes SM-2/3/4 facts and chooses their transitions。Run owner returns concrete
`accepted|aggregate_revision_mismatch|semantic_rejection|owner_transient` result；only actual revision mismatch enters CAS reload。

Process restart obtains Run inventory and opaque record through Run-owner typed query，then Orchestrator loads and idempotently
re-drives。Projection、EventRecord、request log、historical Step/checkpoint or process-local cache cannot rebuild aggregate。

## Eventstream Audit

Eventstream consumes bounded immutable notifications and owns query/replay order。It may persist owner IDs、scope、topic、
subject、safe summary/hash/ref and timestamps；it must not persist live repository/service objects or expose an owner mutation
port。Missing/delayed EventRecord does not roll back the producer owner transition。

## Workorchd Projection

```text
owner fact / owner read
  -> workorchd projection mapper
  -> detached bounded view model
  -> HTTP/SSE/TUI/Web
```

- projection can cache、rebuild、paginate and wait on typed notification；
- projection cache is process-local/derived and may be discarded；
- each lookup resolves Workspace/Run/resource scope before owner query；
- duplicate IDs across workspaces cannot leak；local-only identity crossing boundary must carry enough scope；
- terminal/status display is derived from owner facts and cannot submit reverse repair、Resume or state setter；
- mutable slices/maps and sensitive/raw content are detached、redacted or represented by allowlisted ref。

## Delete And Purge

Delete is an application-orchestrated sequence of owner-defined concrete commands。Each owner deletes only its own data；
partial failure remains a typed recoverable result and cannot be hidden by deleting projection/audit first。Purge never gives
Application direct repository access or lets one Block delete another owner aggregate。

## Verification

- repository tests cover owner isolation、CAS、restart and purge idempotency；
- server tests cover scope、pagination、SSE replay/cache invalidation and raw-ref validation；
- [Core Runtime Verification](../00-core/runtime/verification.md) covers opaque CAS/recovery；
- [Service Acceptance](../50-product-interface/service-acceptance.md) covers deployed read behavior。

## Related Contracts

- [Component Contract Inventory](../01-overview/component-documentation-alignment.md)
- [Block Boundaries](block-boundaries.md)
- [Service Read API](../50-product-interface/service-read-api.md)
