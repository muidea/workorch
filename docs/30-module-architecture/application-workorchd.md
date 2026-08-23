# Application Workorchd Architecture

Status: normative

Type: module-architecture

Last Verified: 2026-08-23

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
2. run pure admission over all-of relations plus required owner-fact acquisition/reobserve closure, and promote only exact-current `admitted_all_of`;
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
Decision read projection将owner binding复制为bounded typed `definitionClarification`并deep-clone `offendingRefs`；该DTO只供
展示与Workspace-scoped修订导航。workorchd不从Goal/DoD或display projection派生Restart freshness/eligibility，也不让Board
代发Definition Restart；详情页的用户显式Restart仍通过统一execution ingress交给Workspace/Run owner裁决。

## Runtime, Projection And Lifecycle Boundary

The application owns one long-lived runtime session internally; named application ports borrow it for a bounded call and close the
borrow afterward。Transport、process service and clients never receive the session pointer。Managed observation of Eventstream
records is bounded read-side behavior, not Orchestrator Runtime or an authority rebuild surface。

Projection consumes immutable Eventstream/Orchestrator/owner/execution observations。Its cache/store can rebuild, but derived
writes and deletion remain application-owned and cannot mutate owner aggregates。

`RuntimeStateRecord` 只在一次 projection mapping 调用期间作为瞬时输入；projection Store、application read model 与 HTTP
cache 不保存完整 aggregate。公开 Workspace/Run 的 canonical `state`、Workspace `activeRunId` 和 Run blocker refs 必须与 owner
snapshot 原样一致，不能由 Decision/Approval、TaskProgress 或 terminal display 合成覆盖。Workspace DTO 本身没有 blocker refs。
跨 owner 治理关联只写入 read-only `governanceDisplay`，偏差写入 `consistencyDiagnostic`；`displayOutcome` 只描述展示结果，
formal output 仍由独立 `result/results` 原样投影。这些字段不得进入或决定 Drive、Resume、Restart 或任何 owner mutation。

`TaskProgress.recovery`、ContextBundle 与 compressed context 复用同一个 factual `BlockingResourceRef`。该引用只在 canonical
active Run blocked state/ref 与同 scope owner resource 精确相交时存在，包含 owner kind/resource/scope/state 以及 owner 原样
提供的 `requiredAction/allowedUserActions`；`suggestedNavigation` 是独立 display hint。它们均不构造 execution 或 governance
mutation。SM-2 repair 只留在 `RepairLoop`/convergence budget；SM-4 transport/operation/CAS/terminal delivery 只留在
`recoveryBudget` 与 terminal audit，二者不能互相镜像。

Run lifecycle cache invalidation/metrics 只消费 `source=blocks/run` 且通过 `RunStateChangedData.Validate` 的 typed notification；
Eventstream replay 与 source-forged/invalid notification 只能被忽略。

Execution admission has no independent lifecycle or queue. HTTP returns `202` only for synchronous Workspace-owner
`reconciliation_requested|replacement_pending`; the other five canonical results return typed `409` with the same identity、
exact `outcome` and `admissionReason`.
owner rejection is returned by that request, and duplicate identity is always delegated to the owner persistence contract. The
setup-draft response state and terminal acceptance presentation are pure projections of their source facts, never separately
persisted transition authority. Its only workspace-scoped admission observation is the latest immutable owner outcome in
`snapshot.runCommand`: `accepted` has only `acceptedAt`, `rejected` has only `rejectedAt`, and both retain exact request/scope
identity、owner `outcome` 与 `admissionReason`。唯一写入口
`ApplyWorkspaceExecutionAdmission(command metadata, exact RuntimeExecutionIntentAdmission)` 先校验 exact owner fact，再经共享
`PresentAdmission` 纯派生 `state`、`diagnostic` 与互斥时间戳；caller 不提供 state 或 revision，Store 在同一锁事务中
把 `advanceEntryRevision` 结果写入 entry 与 runCommand。canonical 字段仅为
`workspaceId/runId/state/intent/requestId/requestReason/source/outcome/admissionReason/diagnostic/acceptedAt|rejectedAt/revision`，
不存在 `disposition`、歧义 `reason`、`error`、`definitionRevision` 或 `decisionId` shadow。transport/context failure 不生成
runCommand；caller priority 不存在。
Replacing this read-side observation when a later command completes does not create an application state machine。
Monotonic accepted/rejected/failure counters may summarize requests, but readyz、metrics and snapshots不得保存可迁移的
scheduler/admission lifecycle。Workorchd does not expose continuation state、scheduler/watchdog diagnostics、recovery
readback or a transition from any diagnostic into an owner command。
LLM usage read model 原样投影 Eventstream 中的 optional `logicalRequestId`/`physicalCallId`，并只消费 bounded
`physicalResponseMode` 与 `providerSubmitted` owner audit。新 Runtime usage 的两级身份必须完整且
`physicalCallId=requestId`；历史或 generic 记录允许两者都没有，但不得从 request bytes、streaming 或其他字段推断。
旧 `streaming` 是 mode 的兼容派生，不是第二份状态。非法或缺失 current physical audit 被拒绝，不能默认为
non-streaming/unsubmitted。

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
