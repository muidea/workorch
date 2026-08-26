# Block Boundary Contract

Status: normative

Type: module-architecture

Last Verified: 2026-08-26

## Purpose

本文只定义 17 个 Block 的分类、共同结构和跨 owner 交互边界。每个 Block 的正式状态、topic、command/result 与
私有 lifecycle 以对应 `internal/modules/blocks/<unit>/readme.md` 和 `pkg/events` 为实现 SoT；本文不复制组件内部流程。

## Block Classification

| Class | Units | Owner rule |
| --- | --- | --- |
| resource/state owner | `workspace`, `run`, `result`, `decision`, `approval`, `memory`, `skill`, `soul`, `mcpregistry`, `eventstream` | each owns one resource aggregate or audit resource |
| capability owner | `llm`, `file`, `git`, `shell`, `ssh`, `web`, `mcp` | each owns one technical capability, admissibility and factual execution result |

Block 是否有 repository、调用方数量或代码规模不改变分类。需要组合多个 owner command 完成业务闭环的是 Module；
只围绕一个资源或技术能力的是 Block。不得为了目录对称创建没有 owner/lifecycle 的空 Block。

## Common Unit Shape

使用 EventHub 的 Block 遵循：

```text
module.go
  -> biz.Biz embeds internal/modules/base/biz.Base
       -> own repository/state
       -> EventHub Send/Post/Subscribe
       -> owner-defined pkg/events
  -> optional service adapter
```

- `module.go` 只完成 framework lifecycle 接线，不保存 Hub、observer、业务 handler 或 owner state；
- `biz/` 内嵌共享 Base，承担 owner use-case、订阅、typed handler、后台任务和 Teardown；
- `service/` 只做本 Block 的协议适配，不接收 EventHub，不访问其它 owner repository/service；
- `pkg/events` 由接收 owner 定义 concrete topic、Command/Data/Result；调用方不得复制或建立共享 alias；
- owner-private entity/repository/mapper 留在 Block 内部，不通过 `pkg/models` 泄漏可变资源。
- resource owner 的 repository-facing mutation implementation、constructor 与非 EventHub use-case input 均留在 `biz/`
  私有范围；sibling production 与跨 owner 测试不得调用 `Service/NewService` 或导入 `biz/contracts`，只能经 owner `pkg/events` concrete
  command/result 交互，或构造 immutable fact fixture 验证 consumer。
- Eventstream 也不例外：workspace deletion 的 repository helper 为 owner-private，只接受
  `blocks/workspace` 发送的 concrete synchronous seal/purge command；module bridge 只持有标准
  `biz.Biz` 生命周期对象。

## Resource Owner Responsibilities

| Owner | Sole authority | Accepted cross-owner contribution |
| --- | --- | --- |
| Workspace | admission candidate、formal Definition/startup input、activation eligibility、active Run slot、Workspace deletion coordinator aggregate | application submits typed candidate/admission/delete intent; terminal delivery and owner deletion ack facts |
| Run | SM-1 lifecycle、opaque Runtime record CAS、same-owner replacement transaction/lineage、durable two-phase Workspace retirement fence | Workspace activation/continuation/deletion intent与 exact quiescence token；Orchestrator wait fact或 exact SM-4 terminal handoff，均先转成 Run-private event |
| Result | already-judged formal terminal outcome/artifact index | sealed Orchestrator terminal delivery |
| Decision | decision and Definition-clarification ticket lifecycle | concrete open/resolve request bound to persisted authority |
| Approval | approval lifecycle | concrete approval request/resolution |
| Memory | Memory and recall-trace resources | scoped typed remember/recall command |
| Skill / Soul | instruction assets and owner revisions | management/select/resolve command |
| MCP Registry | server/capability catalog resources | management/query command; MCP execution remains MCP capability owner |
| Eventstream | immutable bounded audit events | typed notification; never replay authority |

Resource owner只修改自己的 repository/aggregate。它可以验证 caller 提交的 intent/fact，但不能读取 sibling owner
repository 来补全跨 owner transaction。一个多 owner 流程若需要 persisted phase/retry/partial ack，必须由
一个业务 owner 持久化 coordinator aggregate，且只由该 owner 的 private internal event 推进。Application
可组合无状态的 query/use case，但不得持有这类流程的影子 saga。Workspace deletion 由 Workspace
持有唯一 durable coordinator，Application 只发送一个 intent，每个接收 owner 只返回自身事实。
跨 owner command adapter 只能校验并提交接收 owner 的 private requested event；任何物理清理或外部 effect 只能由该
owner 的 private dispatcher 调用，并在 effect 成功后通过 private completed event/CAS 推进 aggregate。adapter 直接调用
repository purge、先写 completed 再清理，或由 coordinator 代替资源 owner 清理，均属于第二条状态迁移入口。
owner-private physical sink 也不得混入普通 CRUD repository capability。Workspace configuration sidecar/index cleanup 通过
Workspace repository family 的独立 typed cleaner 注入，只能由 `directory_cleanup_requested` dispatcher 使用；Application、
Initiator adapter 与其他 owner 不能从 `WorkspaceRepository` 获得该 authority。
Decision/Approval 的 lifecycle transition 只能由各自 EventHub handler 归约为 owner-private event 后触发；
它们参与 Runtime SM-2 governance 与 SM-4 delivery，但不向 Orchestrator 暴露第二条 mutation ingress，也不拥有
SM-2/SM-4 authority。

Orchestrator 在 Workspace 删除中只拥有自己的 durable `quiescing -> quiesced -> evidence-fenced -> purged` aggregate。
它与 SM-4 terminal-delivery/lease drain 是同一 owner 内的协调，但不是新的 Runtime 状态机，也不授予 Workspace 或
其他 owner 修改 SM-2/3/4 的能力。Run `draining` 先把所有 Orchestrator ingress 限定为 exact terminal-delivery drain；
`quiescing` Begin 与这些入口共享 write barrier，只有 local/persisted drain 全空时才能落盘，落盘后所有 Runtime
mutation 都拒绝。`quiesced` token绑定完整有序 Run/Runtime inventory，不能只绑定 Workspace generation。
六类 Orchestrator evidence/content repository 必须通过一个 canonical owner repository set 共用该 tombstone；禁止将 raw
repository 与另一个 deletion fence 混装。

## Capability Owner Responsibilities

| Owner | Owns | Returns | Must not |
| --- | --- | --- | --- |
| LLM | provider/model/config ceiling、request transaction、provider result | typed complete/incomplete/transport/configuration fact | choose Runtime action or evidence credit |
| File | workspace tree/revision、file admissibility and mutation | revision/fence-bound factual receipt | interpret Goal/DoD |
| Git | repository observation and supported Git operation | typed revision/effect receipt | become File or Shell fallback |
| Shell | admitted command/managed validation session | bounded stdout/stderr refs and effect/check fact | bypass sealed recipe/target |
| SSH / Web | remote/network operation and bounded response | owner-scoped factual result | choose governance or terminal |
| MCP | selected MCP capability invocation | typed tool result/diagnostic | own MCP registry resources |

Capability owner publishes current catalog/admissibility/feasibility and executes only a concrete sealed operation. It does
not own SM-2/SM-3, candidate correction, CWI mapping, evidence acceptance, repair or terminal. Multiple variants for one CWI
are operation feasibility choices, never Definition `one_of`。

Prospective owner-fact feasibility follows the same boundary without cross-owner reads: File may publish only its immutable,
base-revision-bound hypothetical workspace-manifest delta; Shell may publish only a pure satisfaction observation against its own
exact dependency catalog/observation and the supplied delta. The Shell dependency catalog participates in Runtime freshness but is
not provider-selectable. Orchestrator joins both facts through its SM-2 private event; File, Shell and Orchestrator cannot mutate
one another's state machines.

## EventHub Interaction

- synchronization/admission/execute/query requiring a result uses `Send`; handler returns its concrete Result through `event.Result`;
- audit notification uses `Post`; Post handler must tolerate `result=nil` and never call `result.Set`;
- payload/result contains bounded data only—no repository、registry、recorder、writer、channel or service pointer;
- raw content只有在owner合同允许时才能以safe content-addressed opaque ref跨边界；credential、endpoint、unbounded body和
  raw transport diagnostic不得进入EventHub result；
- workspace-local identity crossing a Block boundary carries WorkspaceID and any Run/definition/authority/dispatch scope needed to remove ambiguity;
- missing result、wrong concrete type、scope mismatch、stale revision and duplicate identity are explicit typed outcomes, not success-by-default。

## Runtime Participation

```text
Orchestrator sealed operation
  -> capability owner concrete Execute Command
  -> owner preflight + exactly-once effect
  -> concrete factual Result/receipt
  -> Orchestrator private internal event
  -> SM-2 evidence/reducer decision
```

Orchestrator derives operation/evidence identity from sealed dispatch; capability owner returns its own factual receipt without
rebuilding coverage authority. Durable duplicate/restart reuses owner execution key/fence and cannot repeat side effects。

SSH command只携带opaque target/session/credential refs、exact approval与strict host identity，receipt返回exit/duration、
bounded full-output hash/ref及before/after remote revision；不能把host、credential material或sealed expectation当成observation。

## Related Contracts

- [Component Contract Inventory](../01-overview/component-documentation-alignment.md)
- [Architecture Overview](architecture-overview.md)
- [Capability Catalog](../10-domain-model/capability-catalog.md)
- [Runtime Candidate And Dispatch](../00-core/runtime/candidate-and-dispatch.md)
