# Application Workorchd Architecture

Status: normative

Type: module-architecture

Last Verified: 2026-08-22

## Purpose

`internal/modules/application/workorchd` 是服务端 Application Module。它组合 HTTP/SSE、typed application use case、
Runtime bootstrap、execution admission 与 projection lifecycle，但不拥有 Workspace、Run、Runtime、Result、Decision、
Approval 或 capability 的正式状态。

组件内部结构、named ports 和 EventHub topics 以
[workorchd implementation contract](../../internal/modules/application/workorchd/readme.md) 为准；本文只定义跨层职责。

## Application Responsibilities

| Responsibility | Owns | Must not |
| --- | --- | --- |
| framework composition | one module lifecycle and dependency readiness | expose a sibling owner service/repository |
| execution admission | request normalization, synchronous Workspace intent submission and exact owner-result validation | own queue/dedupe/retry、SM-1 or successor creation |
| bootstrap composition | read activation-eligible owner snapshots and build typed `RuntimeBootstrap` | choose Runtime action or modify sealed input |
| runtime session | one process-lifetime session with scoped borrow/close | leak session to transport or clients |
| projection | derived manager/store lifecycle and rebuild | repair or rebuild owner authority |
| HTTP/SSE/CLI adapters | parse/render stable application DTO and invoke named use cases | retain EventHub or perform owner mutation directly |

The module fails closed before accepting work when required ports, owner contracts, runtime session or projection dependencies
are unavailable. Server-local execution queue、scheduler authority、projection 或 repository fallback 不属于生产路径。

## Definition And Execution Admission

Fresh execution follows one application sequence:

1. submit the exact setup input as a Workspace-owner `DefinitionAdmissionCandidate`;
2. run pure admission and promote only exact-current `admitted_all_of`;
3. when admission reports unsupported relation or ambiguity, open/reconcile the pre-Run blocking clarification ticket and stop;
4. read activation-eligible Workspace startup input plus referenced Template/Profile、provider、Soul、Skill、MCP and Memory facts;
5. build a typed `RuntimeBootstrap` from those owner snapshots and process defaults;
6. synchronously submit a workspace-scoped `resume|restart` intent and validate the exact Workspace-owner result; Workspace and Run owners perform their own transitions;
7. observe owner facts for projection while Orchestrator drive starts from Run-owner notification.

Normal TUI/Web input does not include raw context hints、evidence policy、target、recipe、check、artifact graph、target Run state
or per-run provider/capability override. A blank project directory is resolved before candidate persistence; no empty root enters
bootstrap or capability execution.

## Resume And Restart

| Intent | Application responsibility | Owner transaction |
| --- | --- | --- |
| `resume` | submit scope only; no bootstrap/context patch | Workspace/Run select the current recoverable Run and preserve its sealed selection |
| `restart` | compile a fresh bootstrap from current activation-eligible owner state | Run persists predecessor `ReplacementIntent` and creates the unique successor only after terminal delivery |

Application、Workspace and Orchestrator never pre-create or link the successor。For a non-current development baseline, Run owner
returns `data_reset_required` without provider/capability work or replacement intent。Decision/Approval handlers submit only the
corresponding owner resolution and refresh projection; they do not emit Resume。

Runtime-discovered Definition clarification不走普通Decision handler：workorchd只投影`revise_definition_and_restart`并接收
用户提交的新candidate与显式Restart intent。只有Run owner可以在matching `ReplacementIntent`已持久化后调用Decision owner的
dedicated replacement-resolution contract；application不关闭ticket、不代发该command，也不把修正后的Definition注入predecessor。

## Runtime, Projection And Lifecycle Boundary

The application owns one long-lived runtime session internally; named application ports borrow it for a bounded call and close the
borrow afterward。Transport、process service and clients never receive the session pointer。Managed observation of Eventstream
records is bounded read-side behavior, not Orchestrator Runtime or an authority rebuild surface。

Projection consumes immutable Eventstream/Orchestrator/owner/execution observations。Its cache/store can rebuild, but derived
writes and deletion remain application-owned and cannot mutate owner aggregates。

Execution admission has no independent lifecycle or queue. HTTP returns `202` only after synchronous Workspace-owner admission;
owner rejection is returned by that request, and duplicate identity is always delegated to the owner persistence contract. The
setup-draft response state and terminal acceptance presentation are pure projections of their source facts, never separately
persisted transition authority. Its only workspace-scoped admission observation is the latest immutable owner outcome in
`snapshot.runCommand`: `accepted` has only `acceptedAt`, `rejected` has only `rejectedAt`, and both retain exact request/scope
identity. Replacing this read-side observation when a later command completes does not create an application state machine。
Monotonic accepted/rejected/failure counters may summarize requests, but readyz、metrics and snapshots不得保存可迁移的
scheduler/admission lifecycle。Workorchd does not expose continuation state、scheduler/watchdog diagnostics、recovery
readback or a transition from any diagnostic into an owner command。

Lifecycle ordering is:

```text
start projection -> start transport
stop EventHub/CLI ingress -> stop transport -> clear ports -> close projection/tasks -> close runtime session
```

The module does not run inactive-Run recovery scans、stale-Step watchdogs、pending/started queues or orphan-step repair APIs;
the retired watchdog/continuation metrics are not part of the service contract and their reappearance fails closure observation。
Recovery remains SM-4 idempotent re-drive from the persisted aggregate。

## Cross-Owner Rules

- Cross-owner reads and writes use receiving-owner concrete `pkg/events` Command/Result through EventHub。
- HTTP、CLI and EventHub DTOs remain distinct contracts；no generic dispatcher、callback alias、Envelope、map/JSON/reflect bridge。
- `biz/execution` owns its focused submitter/projection ports and must never import protocol `service/server`; server types、metrics or
  handler state cannot become execution-admission authority。The `biz` package root remains the composition boundary that wires both sides。
- `module.go` only wires lifecycle；Hub subscription and application orchestration stay in `biz/`；protocol `service/` does not retain Hub。
- Production code does not construct sibling owner repository fixtures；test-only fixtures stay outside production APIs。
- Large/raw evidence is exposed by validated ref and never copied into EventHub、projection or Runtime authority。

## Related Documents

- [Architecture Overview](architecture-overview.md)
- [Storage And Read Model](storage-readmodel.md)
- [Kernel Module Boundaries](kernel-orchestrator-governance.md)
- [Service Command API](../50-product-interface/service-api.md)
- [Service Read API](../50-product-interface/service-read-api.md)
- [Service Acceptance](../50-product-interface/service-acceptance.md)
- [Definition Runtime Integration](../00-core/definition-convergence/runtime-integration.md)
