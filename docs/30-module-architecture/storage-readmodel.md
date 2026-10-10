# Owner Persistence And Read-Model Boundary

Status: normative

Type: module-architecture

Last Verified: 2026-08-26

## Purpose

本文只定义 owner persistence、Eventstream audit 与 workorchd read model 的单向关系。repository layout/encoding 是
owner implementation detail；HTTP pagination、wait、raw-content 由 [Service Read API](../50-product-interface/service-read-api.md) 定义。

## Owner Repository Groups

| Data | Sole writer | Persistence meaning |
| --- | --- | --- |
| Workspace candidate/Definition/startup/active slot | Workspace Module | formal Workspace authority |
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
port。Missing/delayed EventRecord does not roll back the producer owner transition。Workspace deletion 完成后的
`/workspace/deleted` 是 post-seal transport-only invalidation receipt，不进入已永久封存的 Eventstream；reconnect
通过 owner snapshot 恢复 visibility。

## Workorchd Projection

```text
owner fact / owner read
  -> workorchd projection mapper
  -> detached bounded view model
  -> HTTP/SSE/TUI/Web
```

- projection can cache、rebuild、paginate and wait on typed notification；
- projection cache is process-local/derived and may be discarded；
- opaque `RuntimeStateRecord` 仅为单次 mapper 的瞬时输入，不进入 Store、HTTP cache 或 public DTO；
- Workspace/Run owner fields 原样投影；Workspace DTO 无 blocker refs，Run blocker refs 只来自 Run owner；
- `governanceDisplay`、`consistencyDiagnostic` 与 `displayOutcome` 是 bounded read-only DTO，不能反向形成 mutation intent；
- formal outcome 只认独立 Result owner projection，`displayOutcome` 不替代或重判 Result；
- each lookup resolves Workspace/Run/resource scope before owner query；
- duplicate IDs across workspaces cannot leak；local-only identity crossing boundary must carry enough scope；
- terminal/status display is derived from owner facts and cannot submit reverse repair、Resume or state setter；
- mutable slices/maps and sensitive/raw content are detached、redacted or represented by allowlisted ref。

Owner repository 的单文件物理发布不构成额外状态机，但必须共享同一 crash-safe durability contract：canonical path
只能通过 same-directory temp、完整写入、file `Sync`、close、atomic rename 和 parent-directory `Sync` 更新。rename 前失败
保留旧 generation；rename 后目录同步失败属于已发布但耐久确认不确定，任何 read cache 必须更新或失效。`.tmp` 文件、
Eventstream 和 projection 都不能在 canonical JSON 损坏时反向重建 owner authority。

## Delete And Purge

Workspace owner 持久化唯一 deletion aggregate 并编排 Workspace delete/retain/purge。Application 只发送
一个 concrete `DeleteWorkspaceCommand`，不保存 phase、owner ack、retry journal，也不访问任何
owner repository。已存在 active Run 的初始请求在写入 deletion intent 前直接 conflict；一旦 intent
持久化，Workspace 所有普通 setup/execution mutation 都被永久拒绝。

删除链的顺序是确定的：Run owner 先在 generation-head 原子封存 `draining` fence，拒绝所有新 Run和普通 SM-1/
sub-resource mutation，但仍允许唯一的 exact terminal-delivery opaque Runtime CAS drain lane。Orchestrator 先在该区间
排空 terminal-delivery/lease authority；只要 local activity、persisted lease或未确认 delivery仍存在，其 Begin 就零状态
变化返回 conflict。完全排空后才持久化 `quiescing` gate，gate后所有 Runtime mutation一律拒绝，并封存绑定逐 Run
revision与 Runtime ref/revision/hash 的 quiescence token；
Run owner精确复核该 token后才原子升级为永久 `retired`，随后 Orchestrator 才把自身 aggregate推进到 durable evidence
tombstone；之后 Result、Memory、Decision、Approval 各自封存 owner-local tombstone。`retain` 只隐藏并封禁资源，不做物理删除；
`purge` 由每个 owner 的 exact scope
private sink 幂等清除自身数据，Workspace 再清除自身 Definition/可交付目录与 ordinary configuration sidecar/index。
Workspace 的 `directory_cleanup_requested` 必须先用 exact current Workspace/deletion revision、intent 与 stable cleanup ID 调用
独立 owner-family cleaner；file backend 对 SoulBindings sidecar 做幂等空写，对 workspace index 做跨 repository-set 锁内 fresh-read、
仅目标 entry 清零、atomic replace 与复读空集证明，随后才允许 `directory_cleanup_observed` CAS。这样即使主 metadata 已推进而
普通 CAS 的 sidecar/index tail 遭遇崩溃或 I/O 错误，也不能恢复旧 configuration；其他 Workspace entry 保持不变。
Eventstream 总是
最后封存/清理，并保留 tombstone 与 global `StreamPosition` floor。

Run 与 Orchestrator evidence 的删除 proof 包含首次观察计数，因此各自在 owner-local durable technical state 中先写
`pending` receipt、再执行物理删除与空集复核、最后写 `completed`。journal 位于 owner system state，不得放进待删除
的 evidence/run surface；崩溃或响应丢失后的重驱必须返回原 receipt，不能把已删除后的零计数当作新 proof。

每个 owner 只从 `blocks/workspace` 接受自己定义的 concrete command，并且只能通过 owner-private
internal event 推进自身 fence/tombstone。响应丢失、部分失败和进程重启都从 Workspace aggregate
和各 owner 的 durable fact 幂等重驱；projection/audit 不是恢复 authority。

Run `draining`、Orchestrator `quiescing/quiesced` 与 Run `retired` 必须分别保存在各自 owner repository。Runtime CAS 与
Run finalize共享 generation-head 锁：CAS-first 使 token stale并要求重新观察，finalize-first使 CAS冲突。Orchestrator
Begin/token seal与 Runtime private mutation共享 owner barrier；进程重启先恢复 quiescing gate与 Run retirement fact：
已有 gate时不得恢复 Runtime mutation，只有 Run `draining` 时才恢复 exact terminal drain；不能先启动普通 Runtime recovery。

## Verification

- repository tests cover owner isolation、CAS、restart and purge idempotency；
- server tests cover scope、pagination、SSE replay/cache invalidation and raw-ref validation；
- [Core Runtime Verification](../00-core/runtime/verification.md) covers opaque CAS/recovery；
- [Service Acceptance](../50-product-interface/service-acceptance.md) covers deployed read behavior。

## Related Contracts

- [Component Contract Inventory](../01-overview/component-documentation-alignment.md)
- [Block Boundaries](block-boundaries.md)
- [Service Read API](../50-product-interface/service-read-api.md)
