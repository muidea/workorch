# Application Workorchd Architecture

Status: active

Type: module-architecture

Last Verified: 2026-08-16

## Purpose

`internal/modules/application/workorchd` is the server application module. It composes HTTP/SSE, typed application use
cases, Runtime bootstrap construction, execution admission and read-model projection. It is not an aggregate owner and does
not drive Runtime convergence.

The component-level implementation contract is
[workorchd README](../../internal/modules/application/workorchd/readme.md).

## Internal Components

| Component | Cohesive responsibility | Must not own |
| --- | --- | --- |
| `biz.Biz` | framework lifecycle, one EventHub, component construction/order | sibling owner repository/service |
| `runtimeComponent` | sole long-lived workorchd `runtimecore.Session`; borrow/close lifecycle | Runtime/Run state or transport |
| `runtimeApplicationPorts` | seven groups of named application callbacks using short-lived session borrow | session pointer, generic dispatcher |
| `biz/execution.UseCase` | request normalization, admission, BackgroundRoutine scheduling, Workspace intent submission | durable retry/SM-1/successor |
| `biz/projection.Component` | sole projection manager/store lifecycle and derived writes | owner repair or authority rebuild |
| `service/server` | route registration, DTO parsing/rendering, SSE and transport diagnostics | EventHub/session/projection write lifecycle |
| CLI facade | translate stable `workorchport` requests into application use cases | direct owner `biz`/repository access |

Production transport receives only concrete named `ApplicationPorts`. A missing port or component fails closed before the
listener accepts work; server-local scheduler/projection fallbacks are not allowed.

## Runtime Activation

For a fresh execution, workorchd:

1. reads Workspace owner canonical Workspace/Goal/DoD/selection and referenced Soul/Skill/MCP/Memory resources through
   concrete EventHub commands;
2. validates the selected provider through the LLM owner;
3. compiles a typed `RuntimeBootstrap` containing Definition seed, stable focus/coverage input, frozen activation context
   and bounded convergence/recovery policy;
4. submits a workspace-scoped typed execution intent through Workspace owner; Run owner seals the bootstrap and owns SM-1;
5. observes accepted/rejected execution facts for projection while Orchestrator drive starts from Run owner notification.

The normal HTTP/TUI request contains only `intent`, priority and request metadata. It does not accept raw context hints,
evidence policy, target/recipe/check/artifact JSON, provider/Soul/capability overrides or target Run state.

## Resume And Restart

| Intent | Application behavior | Owner behavior |
| --- | --- | --- |
| `resume` | submit Workspace-scoped continuation intent; no bootstrap/context patch | Workspace/Run select current Run; Run-private reducer validates blocker/lifecycle and notifies drive |
| `restart` | compile fresh bootstrap from current Workspace state and submit intent with no Run ID | Run persists predecessor `ReplacementIntent`; terminal delivery completes before Run-private successor creation |

Application, Workspace and Orchestrator never create/link the replacement successor. For a non-current development baseline,
Run owner returns `data_reset_required` without persisting replacement intent.

Decision/Approval resolve handlers only submit the corresponding owner command and refresh projection. Runtime governance
resolution returns to Orchestrator through Decision owner; application does not emit Resume or choose a lifecycle state.

## Runtime Session Boundary

`runtimeComponent` owns exactly one process-lifetime `runtimecore.Session`. Application-port calls borrow a scoped handle and
close it after the callback; `runtimeApplicationPorts` stores only the borrow operation. The server and process service never
receive the session pointer.

`ManagedRuntime` provides bounded application observation of Eventstream records. It is not Orchestrator Runtime, does not
expose owner actions/snapshots and cannot be used to rebuild authority.

## Projection Boundary

`biz/projection.Component` owns the only `workspaceviewcore.Manager`/Store lifecycle. It consumes:

- Eventstream append notifications for audit/display input;
- immutable Orchestrator drive observations;
- owner snapshots;
- execution admission observations.

Server receives a read-only `WorkspaceProjection` plus explicit application callbacks. Derived writes, rebuild and deletion
remain in the Biz-owned projection component. Projection state may be rebuilt; owner state and Runtime aggregate may not.

## Lifecycle Order

Startup:

1. validate EventHub, runtime component, ports and projection dependencies;
2. start projection manager/subscriptions;
3. open execution admission;
4. start HTTP server.

Shutdown:

1. stop new execution admission;
2. gracefully stop transport;
3. close projection subscriptions/tasks;
4. wait for accepted background work;
5. close runtime component/session.

The module does not run inactive-Run recovery scans, stale-Step watchdogs, pending/started queues or orphan-step repair APIs.

## Cross-Owner Rules

- All owner reads/writes use the receiving owner's concrete `pkg/events` command/result.
- HTTP DTO, CLI DTO and EventHub DTO are distinct contracts.
- No generic envelope, callback alias, JSON/reflect bridge or owner service registry.
- No production repository builder/owner fixture; those are testkit-only.
- Configuration projection is management data; Runtime reads/seals canonical policy through its defined path.
- Large/raw evidence is exposed by validated ref, never copied through EventHub/projection.

## Verification

- `go test ./internal/modules/application/workorchd/... -count=1`;
- `make verify-boundary-contract` for transport/EventHub/repository/state-owner boundaries;
- [Service Acceptance](../60-verification/service-acceptance.md) for HTTP/deployed behavior;
- [Runtime Owner Convergence Matrix](../60-verification/runtime-owner-convergence-matrix.md) for SM interaction.

## Related Documents

- [Architecture Overview](architecture-overview.md)
- [Storage And Read Model](storage-readmodel.md)
- [Kernel Orchestrator / Governance](kernel-orchestrator-governance.md)
- [Service API](../50-product-interface/service-api.md)
- [Runtime Stage Contract](../00-core/runtime-stage-contract.md)
