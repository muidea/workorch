# Runtime V2 System Component Closure Supplement

Status: completed

Type: implementation supplement

Parent plan: [runtime-v2-prerequisite-freshness-closure.md](runtime-v2-prerequisite-freshness-closure.md)

Component execution matrix: [runtime-v2-prerequisite-freshness-component-closure-matrix.md](runtime-v2-prerequisite-freshness-component-closure-matrix.md)

Last Updated: 2026-07-20

## Purpose

本补充将当前已暴露的运行期问题放入同一套组件合同：mutable
prerequisite freshness、Runtime governance decision、TUI `resume`、workspace
model、provider request timeout、终态投影和 framework boundary。它不是新的
Runtime 状态机，也不改变已有 RPF-1～RPF-5 的顺序；它只补足实现者在逐个收口时
必须同时核对的入口、错误语义和验收证据。

权威优先级不变：中立语义以
[runtime-stage-contract.md](../../00-core/runtime-stage-contract.md) 为准；实现权威为
`internal/modules/kernel/orchestrator/readme.md` 和
`internal/pkg/workorchruntimecontract`；本文件及父方案是 active execution
checklist。任何与前两者冲突的内容，必须先修改权威合同，不能在此处创建兼容分支。

### Reopened facts and intended outcome

| 已观察信号 | 不允许的局部修复 | 目标行为 |
| --- | --- | --- |
| File durable effect 后 Shell 使用旧 admissibility observation，最终显示 `candidate_retry_exhausted` | 在 Shell、TUI 或 provider 侧另加 retry | Orchestrator 精确失效、重取 prerequisite，并以新 transaction 的 attempt 0 继续 |
| runtime ticket 使用 `replan` 提交得到 `400 invalid argument` | UI 静默吞错，或 application 直接 Resume/重建 authority | Decision owner 返回可执行的 ticket action；成功 resolution 由 Orchestrator 幂等 continuation，失败返回可展示的 allowed actions 和原因 |
| TUI 修改 workspace model，已运行 Run 仍调用旧模型 | patch active Run、在 capability block 静默切模型 | 修改只影响下一次 activation；active Run 永远使用 sealed `ProviderModelTarget` |
| `/v1/chat/completions` 超时 | 把 timeout 伪装为 stale 或由 UI 盲重试 | LLM owner 返回 typed provider transport failure；独立预算/治理策略决定后续；read model 展示 phase、target 和可恢复动作 |
| boundary verifier 因环境缺少 `rg` 而失败 | 仅在 CI 安装工具、跳过结构校验 | verifier 使用仓库可携带依赖，并将 Base Biz / EventHub / typed DTO 边界作为可重复的本地门禁 |

## Fixed System Boundary

```text
TUI / Console / HTTP / CLI
      | typed user intent and read queries
      v
application/workorchd -------------------------> Workspace owner
  bootstrap for a new activation only                default model / definition
  Start or unified Resume only
      |
      v
Orchestrator (only SM-2/SM-3/SM-4 authority) <---- Run owner CAS aggregate
      |  acquire / probe / execute: EventHub Send       |
      v                                                   v
File Shell Git MCP SSH Web LLM                    Result / terminal projection
      | receipt: exact effect, ResourceChanges,          |
      |          PrerequisiteStale                        v
      +------------------------------------------> Eventstream/read model/UI
                                                   display only

Decision ticket resolution -- typed bound resolution --> Orchestrator
Approval resolution -- typed evidence only --> Orchestrator audit/read model
```

所有箭头都必须使用明确的 command/data/result。禁止跨 owner repository/service
调用、`map[string]any`、Envelope、JSON/reflect bridge 或 application runtime queue。
使用 EventHub 的 Module/Block 保持
`module.go -> biz (embed basebiz.Base) -> service/adapter`：`basebiz.Base` 仅提供
owner-neutral Hub/observer/background-task 基础能力，绝不承载 freshness、decision、
provider 或 UI 策略。

## Responsibility and Interaction Contract

| Component | Owns | May send / receive | Must never do | Independent regression |
| --- | --- | --- | --- | --- |
| `workorchruntimecontract` | value types, normalization, aggregate invariants, stable identity/budget | no EventHub business traffic | inspect files, choose an action, know profile semantics | `go test ./internal/pkg/workorchruntimecontract -count=1` |
| Run | Runtime V2 aggregate CAS, recovery inventory, Run lifecycle persistence | typed aggregate/projection command | reduce facts or classify stale | `go test ./internal/modules/kernel/run/... -count=1` |
| Orchestrator | reducer, freshness intersection, candidate/dispatch, governance continuation, terminal judgement | typed owner acquire/probe/execute/commit | access an owner repository or alter workspace directly | `go test ./internal/modules/kernel/orchestrator/... -count=1` |
| File/Shell/Git | their own observation, feasibility and exact effect receipt | concrete runtime owner command/result | choose next action or spend runtime budget | their own `go test ./internal/modules/blocks/<owner>/... -count=1` |
| MCP/SSH/Web | catalog/config/remote observation and capability receipt | concrete runtime owner command/result | treat ordinary network failure as freshness | their own block test command |
| LLM | sealed candidate request, selected-target request timeout/response | concrete candidate request/result | replace sealed target, classify stale, issue unsealed retry | `go test ./internal/modules/blocks/llm/... -count=1` |
| Decision | Runtime V2 governance ticket, allowed resolution action and resolution persistence | typed ticket-resolution commit | rebuild runtime authority or direct-Resume a Run | `go test ./internal/modules/blocks/decision/... -count=1` |
| Approval | approval fact, approval audit/evidence and its own idempotent state transition | typed approval evidence | masquerade as a Runtime V2 ticket, rebuild authority or direct-Resume a Run | `go test ./internal/modules/blocks/approval/... -count=1` |
| Result | durable already-judged outcome | typed terminal delivery | judge acceptance or reject a recoverable stale | `go test ./internal/modules/blocks/result/... -count=1` |
| workorchd/Eventstream/TUI/Console | inbound mapping and read projection | typed Start/Resume, ticket resolution, read DTO | choose/retry action, patch active target, rebuild aggregate | `go test ./internal/modules/application/workorchd/... ./internal/services/workorch/... -count=1` and console build |

The owner tests above are *direct* tests: a fake local dependency is allowed,
but another owner service/repository is not. Cross-owner behavior belongs only
to the orchestrator fake-owner tests and the file-backed acceptance fixtures.

## Cross-Cutting Contracts That Must Close Together

### 1. Unified user continuation: `resume` is the only runtime command

The user does not select a technical continuation path. TUI and Console expose
only `resume`; there is no separate runtime `restart` or `replan` command.
`resume` means the following, selected from persisted state rather than UI
guesswork:

| Persisted state | Application / owner action | Runtime consequence |
| --- | --- | --- |
| active with pending prerequisite, sealed dispatch, or pending terminal delivery | send scoped `ResumeRunCommand` | Orchestrator re-drives the same aggregate |
| blocked governance ticket already resolved | Decision owner commits its typed binding; duplicate commit re-enters the same singleflight drive | Orchestrator validates binding and continues; application does not send a second Resume |
| resolved generic Approval | persist the approval evidence; present the scoped `resume` affordance if the Run is otherwise resumable | no layer invents a Runtime V2 ticket binding or an automatic Resume |
| terminal, definition drift, or an intentional provider/model change for future work | application creates a **new** typed activation/Run after normal workspace/run commands | new aggregate; it is not a patch/restart of the old Run |
| unresolved decision/approval | return the persisted blocker and its allowed actions | no Resume and no speculative retry |

`replan` is therefore an **input intent**, not a Runtime V2 state-machine
transition. The Decision API must implement the following compatibility
contract before accepting it for a runtime ticket:

1. Load the ticket's persisted runtime-governance binding and its
   `AllowedRecoveryActions`; never infer permitted types from a global enum.
2. If the ticket explicitly permits a replan-style continuation, normalize the
   user intent into a concrete, persisted runtime resolution defined by that
   binding (normally a continue resolution carrying the user's revised input).
   Record the original intent in the Decision audit payload.
3. Submit the typed ticket-resolution commit once. The Decision owner, not the
   HTTP/TUI layer, owns the durable commit. Its application dispatcher emits at
   most one scheduler `resume` continuation bound to that committed resolution;
   it never emits a Runtime V2 `replan` command or creates a successor. The
   scheduler/Orchestrator validate the binding and perform the idempotent
   continuation/re-drive.
4. If mapping is not permitted, return `409`/typed conflict with ticket ID,
   allowed actions and a human-readable remediation. A bare
   `invalid argument: resolution type "replan" is not allowed` is insufficient
   because it supplies no viable user action.
5. A duplicate click/retry uses the same resolution idempotency identity. It
   must return the original committed result or join the continuation; it must
   not consume candidate, repair, provider, recovery or refresh budget twice.

This preserves the ban on the former V1 auto-replan successor: no decision
endpoint may create an untyped successor or patch an aggregate. If a revised
Goal/DoD is required, it is a new explicit activation after the existing Run
has reached its governed outcome.

Required tests:

- runtime ticket accepts each action in its persisted allow-list and rejects
  unsupported action with the typed remediation payload;
- replan-style intent normalization is audited, commits once and causes one
  Orchestrator continuation;
- duplicate resolution, HTTP retry and process restart do not double-drive;
- normal non-runtime Decision semantics remain unchanged;
- application does not publish Resume after a governance resolution.

### 2. Model target lifecycle: persisted workspace preference versus sealed Run

There are two different values and they must not be conflated.

| Value | Owner / lifetime | Read/write contract |
| --- | --- | --- |
| `Workspace.DefaultLLMTarget` | Workspace owner; mutable preference | TUI `/model` updates it through the typed workspace command; it is visible immediately as the default for the **next** activation |
| `RuntimeActivationContext.ProviderTarget` / sealed provider ceiling | Run aggregate; immutable Run input | copied while creating the activation; all candidate attempts, freshness reacquisition and Resume use exactly this value |

Acceptance rules:

- Updating `/model` while a Run is active must leave that Run's target and
  sealed target revision unchanged; the workspace detail can show both
  "next-run target" and "current-run sealed target" when they differ.
- `resume` for the active/blocked Run cannot change provider/model. If the
  sealed target is unavailable, LLM returns a typed failure for that target;
  governance/terminal policy handles it without fallback substitution.
- Creating a new activation resolves explicit request target, then workspace
  default, then system default. The selected target and catalog/config revision
  are sealed before Start.
- Freshness only refreshes observation/configuration facts needed to use the
  sealed target. It cannot become a hidden model migration path.

Required tests: workspace update round-trip; active Run immutable after model
change; next activation uses the saved target; Resume retains original target;
unavailable sealed target does not silently select a fallback; read model/TUI
show the distinction.

### 3. Provider request timeout and failure taxonomy

`http://127.0.0.1:8090/v1/chat/completions` timeout is an LLM provider
transport outcome until the provider demonstrates an observed configuration or
catalog revision drift. It is not an admissibility stale by default.

| Failure class | Producer | Typed information required | May cause |
| --- | --- | --- | --- |
| request deadline / connection / HTTP 5xx | LLM adapter | sealed provider/model, endpoint class, phase, bounded cause, retryability | provider transport ledger or governance according to policy |
| provider response/schema invalid | LLM adapter | response class and contract diagnostic | candidate attempt handling for this sealed transaction |
| provider catalog/config revision changed | LLM prerequisite acquisition/probe | exact `provider_config`/`owner_catalog` observation and expected/observed revision | generic prerequisite invalidation/refresh |
| sealed target absent after fresh acquisition | LLM owner | target identity and availability fact | typed governance or terminal policy; never fallback model |
| client/proxy timeout while server status is unknown | HTTP/service adapter | request correlation id and timeout origin | user-visible uncertain transport state; no synthetic success or duplicate operation |

The request deadline is set once at the LLM adapter boundary from validated
configuration. The adapter must cancel the request context and preserve the
sealed target in diagnostics. TUI/Console display a concise provider failure
and the allowed `resume`/decision action, never a fake successful candidate or
an unbounded "retrying" spinner. Provider transport budget, candidate attempt,
operation recovery and prerequisite refresh budget remain independent.

Required tests: deadline cancellation; endpoint error mapping; no stale on a
plain timeout; exact stale when catalog revision actually changes; request ID
and target survive port/read-model mapping; repeated UI refresh does not issue
another provider request.

### 4. Freshness and durable effect boundary

This is implemented in detail by RPF P0–P5. At every operation boundary use
the following decision table; an owner must not improvise a retry.

| Owner result | Orchestrator transition | Forbidden consequence |
| --- | --- | --- |
| read-only probe returns `RuntimePrerequisiteStale` | invalidate unsealed proposal, pending prerequisite, new transaction attempt 0 | candidate failure/attempt 1/evidence |
| execute sees stale before effect | persist fenced stale receipt, invalidate dispatch and reacquire | operation replay/rejected evidence |
| accepted effect returns exact `ResourceChanges` | atomically retain accepted evidence and invalidate only intersecting observations | reuse remaining old dispatch operations |
| ordinary operation/provider failure | follow sealed evidence/governance path | classify it stale merely to bypass policy |
| duplicate or late receipt | use persisted identity/fence and retain diagnostic lineage only | second budget use or overwrite of fresh observation |

### 5. Projection, terminal delivery and structural verification

Read models, Eventstream, HTTP and TUI must expose facts, not reconstruct
authority. Minimum runtime display fields are run lifecycle, blocker/action,
sealed target, freshness source/owner/resource/revisions, refresh used/limit,
provider failure phase and terminal delivery state. A projection replay may
repair its own display but cannot send a decision resolution, Resume or owner
operation.

`make verify-boundary-contract` is a portability gate as well as an architecture
gate. It must run in a plain POSIX-like CI image without assuming `rg`; its
checks must reject a component without local `biz` + `basebiz.Base`, EventHub
traffic in module roots/services/initiators, and unbounded EventHub contracts.
When a new component is added, update the verifier only with a corresponding
component-local lifecycle and direct test; do not add a blanket exemption.

## Implementation Work Order

These slices extend, rather than replace, matrix P0–P5. A slice may be marked
complete only when its direct tests pass and its contract is reflected in its
owner readme or the implementation SoT.

| Slice | Depends on | Main files/packages to inspect | Deliverable | Acceptance evidence |
| --- | --- | --- | --- | --- |
| S0 contract baseline | none | `workorchruntimecontract`, orchestrator events | validate/normalize freshness, resolution and failure DTOs; no map/any | package tests + invalid payload tests |
| S1 decision continuation | S0 | `blocks/decision`, `blocks/approval`, orchestrator governance event adapter, workorchd decision route/client | ticket-specific allowed action response, replan-intent normalization or actionable conflict, idempotent commit | decision/orchestrator direct tests; HTTP mapping test |
| S2 model lifecycle | S0 | workspace command path, activation builder, LLM sealed ceiling, TUI/console workspace state | workspace preference/new activation/sealed active Run separation | workspace/activation/LLM/TUI regression |
| S3 provider failure UX | S0, S2 | LLM adapter, port DTO, read-model projection, remote backend, TUI/console | typed deadline/transport diagnosis without stale/fallback | LLM adapter + mapping + presentation tests |
| S4 freshness owners | S0 | RPF P1–P3: Orchestrator, File, Shell, Git | exact invalidation and re-acquisition | owner direct tests + fake-owner aggregate tests |
| S5 remaining owners/result | S4 | RPF P3/P4: MCP, SSH, Web, LLM, Result | exact observed revisions, stable cache prefix, no recoverable rejected result | per-owner tests |
| S6 delivery/presentation | S1–S5 | Run, Eventstream, workorchd, remote/TUI/Console | descriptive projection; unified resume only | package tests + console build |
| S7 durable acceptance | S1–S6 | file-backed drive fixtures and verifier scripts | File→Shell→Git positive chain and six crash boundaries | targeted, boundary, full and deployed evidence |

For every slice record: changed component/contract, affected SM IDs, legal and
forbidden transitions, direct test names, duplicate/restart/CAS proof, budget
proof, exact commands, and remaining dependency. Do not record an HTTP manual
test as a substitute for an owner direct regression.

### Executable component handoff ledger

The following ledger is the implementation handoff for the issues that led to
this supplement.  It intentionally names the *next code boundary* rather than
assigning a workaround to whichever UI first exposed the problem.  A row may
be completed independently only after its local gate passes; its integration
exit remains governed by S7/P5.

| ID | Code owner and bounded change | Input → output interaction contract | Explicitly out of scope | Direct regression and integration exit |
| --- | --- | --- | --- | --- |
| E1 | `workorchruntimecontract`, Run aggregate and orchestrator event DTOs: preserve a typed resolution remediation, provider failure diagnostic and freshness identity | typed value / persisted aggregate → normalized typed DTO; all identities are stable across retry/restart | HTTP wording, provider request execution, selecting a resolution | package validation/legacy-decode tests; invalid DTO and duplicate identity cases |
| E2 | Orchestrator reducer/controller/drive: normalize re-acquired prerequisite facts, advance a fully satisfied focus, and redrive only from the aggregate | owner observation/receipt/resolution → one CAS transition, invalidation or next reducer decision | inspecting files, calling an owner repository, choosing a provider fallback | legal/forbidden reducer + controller tests; duplicate/CAS/restart and drive-signature tests |
| E3 | File/Shell/Git owners: report only owner-observed dependency revisions and durable receipt changes | typed acquire/probe/execute command → fact/stale/receipt with bounded changes | refresh budget, candidate retry and focus advance | one direct package test per owner; File→Shell→Git file-backed chain |
| E4 | MCP/SSH/Web/LLM owners: distinguish observed configuration/catalog drift from ordinary transport failure | sealed request plus typed observation → stale **or** ordinary failure, never both | UI retry, unsealed retry, changing `ProviderModelTarget` | each owner package test; LLM stable-prefix and no-fallback regression |
| E5 | Decision/Approval plus orchestrator governance adapter: return ticket-specific remediation and commit a bound resolution once | ticket + user intent → allowed action/result or typed conflict including allowed actions | direct Resume, aggregate rebuild, V1 successor/replan creation | owner and adapter idempotency/restart tests; workorchd HTTP mapping regression |
| E6 | Workspace/activation builder and LLM target ceiling: separate mutable default from immutable Run input | workspace preference + explicit activation → sealed `ProviderModelTarget` for a new Run | patching an active Run/model or silent fallback | workspace/activation/LLM tests; active-vs-next-run presentation regression |
| E7 | workorchd, remote backend, Eventstream, TUI and Console: expose facts and one continuation verb | typed read DTO / user `resume` intent → display or typed Start/Resume request | decision resolution, retry scheduling, authority reconstruction | service/app/TUI tests and console build; projection replay remains read-only |
| E8 | boundary verifier: make the structural contract portable | repository sources → pass/fail structural result without external `rg` assumption | altering runtime behavior or adding blanket exemptions | `make verify-boundary-contract` in an environment without `rg` plus a negative fixture |
| E9 | file-backed/deployed acceptance: close the persisted sequence rather than a process-local happy path | real owner receipt/aggregate reload → one accepted terminal and complete delivery | hiding failures with extra owner/UI retry | P5 six-boundary fixture matrix, then fresh deployed trace |

#### Current implementation checkpoint (2026-07-20; local P5 gate restored)

The following is deliberately a progress record, not a completion claim:

- The M2 MCP capability handoff is locally proved: registry metadata is
  sealed through activation, a selected `none` / `safe_to_replay` tool is
  classified as validation-capable, and Orchestrator preserves that activation
  during its response-side candidate revalidation. The managed trace commits
  the `focus-002` MCP receipt before it fails in the mock assessment. The
  earliest follow-up is fixture-only M2-H1: select the exact final current
  Focus/RequiredOutput coverage slice rather than globally collecting historic
  `cwi-*` tokens. The strict decoder/reducer must continue to reject an
  out-of-slice item; no Decision/Resume or provider retry is permitted.

- E3 Shell now captures sealed-target before/after content revisions for a
  state-changing command and emits File-owned `workspace_target` changes; its
  exact execution-key replay test is present.  It still needs the full
  file-backed chain before E3 is complete.
- E3 Git now declares File workspace-tree dependency for repository state and
  admissibility observations. The reducer also filters required capability
  sequences by the current focus, avoiding Git sequence leakage into a
  File-only focus. The first file-backed trace showed that its default sealed
  refresh limit (3) was below the five durable effects exercised by the
  fixture; after the limit was hit, pending reacquisition incorrectly masked
  the typed exhaustion terminal and looked like a Git feasibility retry. The
  reducer now gives `prerequisite_refresh_exhausted` precedence, and the
  fixture seals its explicit test budget (8). A subsequent bounded rerun
  exposed that this fixture omitted Git's sealed
  `operation_admissibility` prerequisite although it generated `git/write`.
  Production activation already derives this prerequisite from the executable
  capability contract. After making the fixture obey that same bootstrap
  contract, the complete File→Shell→Git path passes with Git inventory
  invalidation/reacquisition and attempt 0 for every fresh transaction.
- E2 now advances focus after all re-acquired prerequisites are satisfied when
  no remaining freshness coverage blocks it.  A dedicated controller
  transition regression is still required; the existing integration fixture
  must not be treated as its replacement.
- E9 has a real File/Shell/Git file-backed fixture. Owner receipt/query,
  invalidation/reacquisition and candidate-seal restart boundaries have been
  migrated to the new per-effect transaction contract, and the positive-chain
  assertion now passes after the bootstrap-contract correction above. The two
  terminal delivery crash boundaries are covered by
  file-backed process-recreation tests: one restarts after terminal persistence
  before Result creation (including `prerequisite_refresh_exhausted` closeout),
  and the other restarts after Result persistence before Run/Workspace
  projection. Fresh deployed acceptance is still open; a passing local
  positive chain is not substituted for it.

When working a row, append the completion-record template from the component
matrix to the parent plan.  Do not promote a row merely because a related
presentation change builds or a manual HTTP request returns 2xx.

## Residual Code-Closure Execution Board (2026-07-20)

This section is the executable handoff for the work that remains after the
owner-local File/Shell/Git and portable-boundary checks. The combined
File→Shell→Git P5 local gate is restored, so it refines H2--H6 in the
[component matrix](runtime-v2-prerequisite-freshness-component-closure-matrix.md#code-closure-handoff-board);
it does not add a fifth state machine or an alternate runtime entrypoint.

### How to use this board

Complete the rows strictly in `R1 -> R2/R3 -> R4 -> R5`.  R2 and R3 may have
independent local changes after R1's contract is stable, but neither may be
called system-closed before R4 and R5.  One pull request/change batch owns one
row unless it only changes that row's explicitly named typed DTO.  A failure
must reopen the earliest row owning the violated assertion, rather than adding
a compensating retry in a later row.

| Order | Row | Current status | Merge precondition | System exit |
| --- | --- | --- | --- | --- |
| 1 | R1 capability and Result owner boundaries | in progress — LLM provider-ceiling stale contract locally proved; Result/read exit remains open | existing C0/C1 freshness and terminal contracts | R4 read/delivery integration |
| 2a | R2 governance continuation | in progress — runtime-ticket `replan` normalization, typed conflict and no-duplicate CLI continuation locally proved | R1 terminal/failure taxonomy | R4 one-command surface |
| 2b | R3 workspace target and activation sealing | partially characterized | R1 LLM failure taxonomy | R4 current-versus-next target display |
| 3 | R4 projection, remote and user surface | open | R1--R3 local gates | R5 process recreation/deployed trace |
| 4 | R5 persisted acceptance | local P5 gate passed; fresh deployed evidence open | R1--R4 and release gate | fresh deployed accepted terminal |

`locally closed` means the row's exact local gate passed in the current
worktree.  `system closed` additionally requires the named downstream exit.
No row may use a manually successful HTTP call, a Console build, or a passing
unrelated package as evidence for an untested state transition.

### Current implementation queue and stopping rules

The following cards are the next executable queue, not a second plan.  They
make the residual work small enough to land and regress independently.  A card
is deliberately stopped at its named owner boundary; its consumer cannot
"help" by adding a retry, direct repository call, or compensating projection.

| Next card | Status on 2026-07-20 | Sole writer / consumer boundary | Local completion assertion | Must remain open for |
| --- | --- | --- | --- | --- |
| C-R2b Decision-resolution process recreation | locally closed | Decision persists one resolution; Orchestrator consumes its persisted ticket/dispatch binding | file-backed rebuilt Hub/owners replay the same resolution identity with one evidence record, one drive lane and no extra candidate/operation/budget | R4 remote/TUI mapping and R5 deployed trace |
| C-R3b activation-precedence round trip | locally closed | Workspace writes only `DefaultLLMTarget`; activation builder seals a new Run | explicit -> workspace -> system precedence is captured by a newly created Run, while an active Run is byte-for-byte target-stable | R4 read-surface preservation and R5 |
| C-R4a projection replay guard | locally closed | projection/eventstream owner reads persisted authority only | replay counters for Drive, Decision commit, Result creation and owner execute are all zero; terminal delivery retries only its missing leg | R4 adapter mapping and R5 |
| C-R4b provider/read command mapping | in progress — R4b-1 and typed remote/TUI preservation proofs exist | remote/TUI/Console map typed read data and requests only | stale, provider transport failure, sealed target and next-run target stay distinct; only `resume` is executable; execute R4b-1--R4b-4 below in order | R5 |
| C-R5 deployed evidence bundle | open | production owners and Orchestrator retain normal writers | newly created deployed Run reaches accepted terminal with lineage, observations, invalidations, receipt fences and budgets recorded | final archival only |

The R1 Result guard and sealed-target no-fallback assertions are locally
proved, but R1 remains open until MCP/SSH/Web direct owner coverage and the
R4 delivery/read exit are complete.  No R1 outcome authorizes C-R2b or C-R4
to classify a timeout as stale or create a Result for a recoverable refresh.

### C-R4b executable micro-cards: one read surface, one `resume` verb

`C-R4b` is intentionally not one Console-only or TUI-only change. The same
typed fact crosses port, remote backend, read projection and two user
surfaces. The following micro-cards make that path independently reviewable
without moving any authority into an adapter. They execute in order. A later
micro-card may consume an already-proved DTO, but may not compensate for an
earlier missing fact by parsing an error string, inventing a retry, or issuing
another runtime command.

| Order | Micro-card and sole change boundary | Typed input -> output | Direct regression (including forbidden assertion) | Handoff / explicit stop |
| --- | --- | --- | --- | --- |
| R4b-1 | `workorchport` intent normalizer plus local/remote run backends | user run intent -> `resume`, or `ErrInvalidArgument` before EventHub/HTTP traffic | empty intent normalizes to `resume`; `restart` and `replan` are rejected; publish/request counters remain zero for both rejected intents | hands only a normalized scoped Resume request to the Run/Orchestrator entry; does not decide whether the Run is resumable or create a new Run |
| R4b-2 | workorchd read DTO/port/client/remote mapping | persisted freshness/provider/target facts -> value-preserving typed read DTO | fixture keeps `RuntimePrerequisiteStale` distinct from provider transport failure, and keeps `sealedLLMTarget` distinct from `defaultLLMTarget`; mapper does not alias a bootstrap pointer | hands immutable display facts to surfaces; it does not turn a failure into a command or infer a fallback target |
| R4b-3 | TUI parser/view and Console action/view mapper | typed read DTO plus supported user input -> descriptive display or typed `resume`/ticket-resolution/workspace-update request | `/run restart` and `/run replan` (and corresponding Console actions) are unavailable/rejected; stale, provider failure, current sealed target and next-run default render as four distinct facts | stops at presentation. Neither surface polls to retry a provider, resolves a ticket, or passes a model override with Resume |
| R4b-4 | workorchd projection/eventstream plus surface integration test | projection rebuild/read refresh -> derived updates only | observers/counters prove zero Drive, governance commit, Result create and owner execute calls while a remote/TUI/Console read refresh occurs | R5 alone validates process recreation and deployed progress; this card must not add a client-side recovery loop |

**R4b completion record.** Record the exact test names and commands below in
the parent-plan evidence ledger before changing the row from `open` to
`locally closed`. The required command set is deliberately split so an adapter
regression cannot be hidden by an end-to-end render:

```bash
go test ./internal/pkg/workorchport -count=1
go test ./internal/services/workorch/biz/backends -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/service/tui -count=1
cd web/workorch-console && npm run build
```

R4b remains **system-open** until R5 proves the same persisted facts survive
process recreation and a newly created deployed Run reaches an accepted
terminal. A successful HTTP response, a Console build, or a manually issued
second Resume is not R4b completion evidence.

### Single-component delivery protocol

The R1--R5 rows are intentionally integration-oriented.  To make them usable
as an implementation queue, every change inside a row must first be expressed
as one **component delivery card**.  A card has one formal-state writer and one
typed boundary; it may change an adjacent DTO only when that DTO is named in
the card.  This prevents a UI, application mapper or capability adapter from
becoming an implicit second authority while trying to repair a visible symptom.

| Card part | Mandatory content | Review rejection condition |
| --- | --- | --- |
| owner and state | package owning the persisted fact/receipt/ticket/read projection, plus the only legal mutation | a mapper, peer owner or test fixture writes the owner's state directly |
| typed boundary | concrete command/result/error/read DTO, producer, consumer, identity and idempotency/fence field | `map`/`any`, error-string parsing, a global enum or a cross-owner service/repository call is used as the contract |
| state-machine scope | affected SM IDs and the one legal before/after transition | a change claims to be display-only while it queues a drive, selects an action or mutates an aggregate |
| direct regression | owner-local fake peers, positive assertion and a counter-backed forbidden assertion | the proof needs a real peer repository/service, live provider or HTTP manual check |
| recovery and budget | duplicate/restart/CAS behavior and the exact ledger that may change | a duplicate spends budget, a restart rebuilds authority, or a different budget is used to hide failure |
| handoff | immutable output fact and the next consumer/card | the card declares itself system-closed without its named downstream proof |

The allowed implementation sequence for each card is:

```text
declare/validate typed contract
  -> write owner-local direct regression (including forbidden counter)
  -> implement the single owner mutation
  -> add mapper/adapter preservation test only
  -> run the row's integration handoff
  -> append the per-row implementation record
```

The following allocation is the normative first cut for the remaining code
work.  It complements the detailed R1--R5 sections below; it does not move
state ownership to the component named in the "consumer" column.

| Card | Sole formal-state writer | Typed input -> typed output | Component-local proof | Handoff / stop condition |
| --- | --- | --- | --- | --- |
| C-R1a owner observation | MCP, SSH, Web or LLM owner respectively | sealed acquire/probe/candidate request -> observation, `RuntimePrerequisiteStale`, or ordinary typed failure | confirmed revision drift yields exact stale; timeout/auth/transport yields no stale; duplicate request returns the bounded original identity | Orchestrator alone intersects/invalidate observations; stop if the owner needs to read Run state or choose a retry |
| C-R1b terminal delivery | Result owner for Result receipt; Orchestrator for terminal/delivery-lane state | already-judged terminal + terminal identity/fence -> idempotent receipt | duplicate delivery returns original receipt; unavailable Result storage changes no terminal judgement | Run/Workspace projection retry is SM-4 only; stop if Result evaluates evidence or stale |
| C-R2a ticket ingress | Decision or Approval owner | scoped ticket + requested intent + resolution identity -> normalized allowed resolution or actionable conflict | persisted allow-list controls permit/deny; duplicate/restart yields one durable resolution; continuation counter is at most one | Orchestrator validates bound ticket/dispatch and drives; stop if ingress sends `ResumeRunCommand` or creates successor |
| C-R3a activation target | Workspace owner for default; activation builder/Run owner for new sealed bootstrap | workspace preference + optional explicit target -> new immutable `ProviderModelTarget` | active Run remains unchanged after `/model`; new activation follows explicit -> workspace -> system | LLM consumes exactly sealed target; stop if Resume carries target override or a refresh changes target |
| C-R4a read projection | read-model/eventstream owner for derived view only | persisted aggregate/diagnostic -> value-preserving read DTO | replay side-effect counters for drive, ticket commit, Result creation and owner execution are all zero | remote/TUI/Console preserve fields; stop if projection derives an action or recovery policy |
| C-R4b user adapter | remote backend/TUI/Console mapper | read DTO or user intent -> display or typed Start/Resume/workspace/ticket request | distinct stale/provider/target fixtures remain distinct; parser rejects executable `restart`/`replan` | R5 checks persisted behavior; stop if adapter retries, appends a second continuation, or synthesizes target |
| C-R5 acceptance fixture | test fixture only; production owners retain their normal writers | durable process recreation boundary -> persisted evidence assertion | each crash boundary asserts identities/fences/budgets before and after recreation | fresh deployed trace; stop if a fixture injects retry behavior not available in production |

For a card touching an EventHub-owning Module or Block, retain the repository
shape `module.go -> biz (embed basebiz.Base) -> service/adapter`.  The card may
not add Hub subscriptions in a module root, `service/`, initiator or a read
mapper.  `basebiz.Base` supplies only owner-neutral Hub/observer/background
task mechanics; the typed contract and all runtime/governance/provider policy
remain in the named owner above.

### R1 — Capability observations, LLM failure taxonomy and Result delivery

**Bounded components.** `blocks/mcp`, `blocks/ssh`, `blocks/web`,
`blocks/llm`, `blocks/result`; their concrete EventHub DTOs; the existing
orchestrator adapters that validate their results.  The reducer, Run
repository, application route and UI are not editable as part of an owner
local fix unless a typed DTO must be extended.

| Producer | Accepted input | Required typed output | Consumer obligation | Must not do |
| --- | --- | --- | --- | --- |
| MCP / SSH / Web | sealed acquisition/probe command and owner-local catalog/config/remote observation | bounded observation, exact generic `RuntimePrerequisiteStale`, or ordinary owner failure | Orchestrator validates identity and performs all invalidation/refresh | infer a retry, mutate Run, or label connection/auth failure as stale |
| LLM | sealed candidate request with immutable `ProviderModelTarget` and one adapter deadline | response/evidence candidate result, ordinary provider-transport failure, or exact observed catalog/config stale | Orchestrator follows the sealed failure or freshness path; read model exposes target/phase | choose a fallback provider/model, issue an unsealed retry, or turn a timeout into stale |
| Result | an already-judged terminal delivery command with terminal identity/fence | idempotent persisted delivery receipt, including delivery-pending failure if storage is unavailable | delivery lane retries Result then Run/Workspace projection without changing judgement | judge acceptance/rejection or persist a Result for recoverable stale/refresh |

**Required code-level assertions.** The following are mandatory before marking
R1 locally closed.  New test names may differ, but each assertion must be
visible in the named owner package; it must not be hidden in an end-to-end
fixture.

1. MCP, SSH and Web each prove one confirmed owner revision drift produces
   generic stale with its exact owner/resource/revisions, and one transport or
   authentication failure remains an ordinary failure with no stale payload.
2. LLM proves adapter deadline cancellation, endpoint/error mapping, sealed
   target and request-correlation propagation, and that a plain
   `http://127.0.0.1:8090/v1/chat/completions` timeout is provider transport
   failure rather than prerequisite stale.
3. LLM proves a confirmed provider catalog/config revision change is stale,
   while an unavailable sealed target is reported for that same target and
   never selects a workspace/system fallback.
4. Candidate refresh proves C0--C2 and `stablePrefixHash` are unchanged while
   observation/failure context remains in C3--C5; a repeated read/poll creates
   no second provider request for the sealed transaction.
5. Result proves duplicate terminal delivery returns the original receipt, and
   a recoverable stale before refresh exhaustion creates neither rejected
   Result nor Run/Workspace terminal projection.

**Local commands.**

```bash
go test ./internal/modules/blocks/mcp/... -count=1
go test ./internal/modules/blocks/ssh/... -count=1
go test ./internal/modules/blocks/web/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/result/... -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
```

**Handoff.** R1 hands only typed stale/failure/delivery facts to R4.  It does
not authorize provider retry policy, a user-facing restart action, or a
terminal result produced outside the Orchestrator delivery lane.

### R2 — Decision ingress, Approval evidence and one durable continuation

**Bounded components.** `blocks/decision`, `blocks/approval`, their typed
EventHub contracts, the orchestrator governance adapter, and the workorchd
governance mappers. A Runtime V2 governance continuation is bound only to a
persisted Decision ticket. Approval remains a separately persisted owner fact:
without an explicit aggregate-level Approval binding it records evidence and
leaves the sole user continuation as scoped `resume`. The TUI/Console command
vocabulary does not gain a `replan` runtime command.

| Case | Request/contract | Required durable result | Forbidden shortcut |
| --- | --- | --- | --- |
| permitted normal resolution | ticket ID + allowed action + idempotency key | one ticket-bound resolution and one bound continuation request | application sends an additional `ResumeRunCommand` |
| permitted replan-style ingress | ticket ID + original `replan` intent, where the ticket binding permits normalization | audited original intent plus concrete persisted allowed resolution | emit Runtime V2 `intent=replan`, create successor, or patch active authority |
| unsupported replan-style ingress | ticket ID + intent outside persisted allow-list | typed conflict with ticket, allowed actions and remediation | raw `invalid argument` 400 with no actionable data |
| duplicate/restart submission | same resolution identity after commit or process recreation | original commit/result or joined singleflight continuation | second resolution, second drive, or another budget charge |
| generic Approval approve/reject | approval ID + scoped owner transition | one durable approval state plus one idempotent audit evidence record | application/UI sends scheduler Resume or presents approval as a Runtime V2 ticket |

**Required code-level assertions.** Direct Decision tests prove ticket-specific
allow-list lookup, audit normalization, conflict payload, duplicate idempotency
and restart after resolution persistence. Orchestrator tests prove a committed
resolution is bound to the persisted ticket/dispatch and causes exactly one
continuation. Approval tests prove its owner transition/evidence is idempotent
and the workorchd CLI/HTTP path publishes no scheduler continuation. Workorchd
tests prove HTTP maps the typed Decision conflict and never converts it to a
raw generic validation error.

**Local commands.**

```bash
go test ./internal/modules/blocks/decision/... -count=1
go test ./internal/modules/blocks/approval/... -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/... -count=1
```

**Handoff.** R2 supplies a durable blocker/resolution/read fact to R4.  The
application and UI may present allowed actions but never infer them or drive
the Run after a decision commit.

**Current local evidence (2026-07-20).** A Runtime V2 ticket whose persisted
allow-list includes `provide_missing_info` now accepts replan-style ingress as
an audited `requestedResolutionType=replan` and stores the concrete continue
resolution; it does not submit a Runtime V2 `replan` action. A ticket without
that continue permission returns `ResolutionActionConflictError`, which
unwraps to `ErrConflict` and is mapped by HTTP to `409` with ticket ID,
requested resolution type, allowed actions and remediation. The typed CLI
Decision paths no longer enqueue a scheduler `ResumeRun` after a successful
Decision commit: the Decision-owner ticket binding and Orchestrator's existing
singleflight governance continuation remain the only drive path. Direct proof:
`TestService_RuntimeGovernanceRejectsUnallowedActionBeforeProposal`,
`TestResolveDecisionViewNormalizesRuntimeV2ReplanToAllowedContinuation`,
`TestResolveDecisionViewReturnsActionableConflictForUnsupportedRuntimeV2Replan`,
`TestWriteBusinessErrorMapsTicketActionConflictToActionableHTTP409`, and
`TestCLIDecisionResolveDoesNotPublishSecondSchedulerContinuation`.

Approval is now explicitly excluded from the Runtime V2 ticket-continuation
path: `TestCLIApprovalResolveDoesNotPublishSchedulerContinuation` proves an
approve command stores its owner fact without adding an application scheduler
Resume. This is intentional, not a missing retry; an aggregate-level Approval
binding would need a new explicit typed Runtime V2 contract before it could
ever drive.

**C-R2b local evidence (2026-07-20).**
`TestRuntimeGovernanceResolutionFileBackedRestartReplaysExactlyOnce` persists
a governed aggregate, Decision ticket and resolution using file repositories,
then tears down the Hub, Run owner, Decision owner and Orchestrator Biz before
rebuilding all of them from the same files.  Replaying the exact typed Decision
resolution is accepted as the original durable result and reaches only
Orchestrator's duplicate continuation lane.  It leaves StateRevision,
governance evidence, candidate-attempt count, operation receipts and every
recovery budget unchanged; the rebuilt process schedules exactly one drive and
a second replay joins that singleflight instead of scheduling again.
`TestService_RuntimeGovernanceCommittedProposalReplaysExactResolution` is the
Decision-owner direct proof that only an identical committed proposal can take
this path; changed payload remains a typed conflict.  R2 now remains open only
for the R4 remote/TUI/Console conflict/remediation and Approval-to-`resume`
surface exit, followed by R5 deployed acceptance.

### R3 — Workspace preference, activation precedence and sealed model target

**Bounded components.** `blocks/workspace`, typed activation builder in
`application/workorchd`, the Run bootstrap/activation contract, LLM target
ceiling validation, and the read DTOs which show the two target values.  This
row is a data-lifecycle change, not a provider recovery feature.

| Value | Write authority | Read rule | Immutability boundary |
| --- | --- | --- | --- |
| `Workspace.DefaultLLMTarget` | Workspace typed update (`/model`) | next-run preference | may change while a Run is active |
| selected `ProviderModelTarget` | new activation builder, precedence explicit request -> workspace default -> system default | sealed target of exactly that Run | cannot change through Resume, decision, refresh or provider failure |
| selected catalog/config revision | same new activation / prerequisite contract | diagnostic and freshness expectation for that sealed target | a refresh may observe drift but cannot migrate target |

**Required code-level assertions.** Prove workspace update round-trip,
precedence of explicit/workspace/system values, active-Run target immutability,
Resume retaining the old target, and unavailable sealed target producing no
fallback.  The existing
`TestActivateWorkspaceExecutionWithBackendResumesSealedActiveRunWhenWorkspaceModelChanges`
is evidence for only the active-Run half; add/retain assertions for next new
activation and read DTO distinction before closing the row.

**Local commands.**

```bash
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/services/workorch/... -count=1
```

**Handoff.** R3 provides immutable current-run and mutable next-run target
fields to R4.  No R3 implementation may patch an active aggregate or hide a
provider failure by copying the newer workspace default.

**Current local evidence (2026-07-20).** `runtimecore.MapRun` now copies the
validated `RuntimeBootstrap.Activation.ProviderTarget` into the read-only
`sealedLLMTarget` Run field, and the app-model -> port -> HTTP-client -> remote
backend chain preserves it without pointer aliasing.  Remote Resume with an
explicit active Run ID now sends only Run scope and command metadata; it omits
`llmTarget` and all activation inputs.  TUI `/model show` reads the projected
Run target separately from `Workspace.DefaultLLMTarget`, rendering
`current-run sealed target` and `next-run workspace default` independently.
Direct proof: `TestMapRunProjectsSealedProviderTargetWithoutAliasingBootstrap`,
`TestPortRunInfoPreservesSealedTargetWithoutAliasing`,
`TestRemoteRunCommandResumeSendsRequestID`, and
`TestEventHubRunResumeForExistingRunOmitsActivationOverrides`, and
`TestModelShowDistinguishesNextRunDefaultFromCurrentRunSealedTarget`; the
existing `TestActivateWorkspaceExecutionWithBackendResumesSealedActiveRunWhenWorkspaceModelChanges`
continues to prove active-run immutability after a workspace update.

R3 remains in progress: the Console current-versus-next display is now
implemented in R4, and the LLM unavailable-sealed-target/no-fallback owner
regression is locally proved in R1.  The new-activation precedence round trip
is recorded below; R3 remains system-open until R4 read-surface preservation
and R5 deployed evidence are complete.

**C-R3b local evidence (2026-07-20).**
`TestCreateAndStartWorkspaceExecutionRunSealsProviderTargetPrecedence` resolves
each explicit -> workspace -> system case, creates and starts a new Run through
the typed Run-owner EventHub commands, then reloads the persisted bootstrap.
It mutates the source request/preference after creation and proves the Run
still holds its selected sealed target.  Together with
`TestActivateWorkspaceExecutionWithBackendResumesSealedActiveRunWhenWorkspaceModelChanges`,
this separates next-run preference selection from active-run immutability.

**C-R4a local evidence (2026-07-20).**
`TestManagerProjectionRebuildNeverIssuesRuntimeAuthorityCommands` rebuilds the
overview and detail projection with EventHub observers armed for Runtime V2
drive, governance-ticket commit, file operation execution and Result creation.
All four counters remain zero; the only emitted behavior is the two derived
projection-change notifications.  This is a direct SM-4 read-path proof and
does not treat a successful projection render as authority evidence.  C-R4b
still owns remote/TUI/Console preservation of the same facts and the sole
executable `resume` command.

**C-R4b partial local evidence (2026-07-20).** `NormalizeRunCommandIntent`
now accepts only empty/default and `resume` as executable Runtime V2 command
input. `restart` is historical display data and `replan` remains Decision
ingress/audit data; both are rejected with `ErrInvalidArgument` before either
the embedded EventHub backend or remote HTTP backend publishes a command.
`TestEventHubRunRestartIsRejectedBeforePublishing` and
`TestRemoteRunCommandRejectsHistoricalRuntimeIntentsBeforeRequest` prove the
zero-publication/zero-request forbidden transition. The companion scoped
resume tests prove that an existing Run sends no activation fields or model
override.

Read mapping now preserves a provider transport failure and a prerequisite
refresh as separate typed facts (`TestRuntimeV2ProgressFromClientKeepsProviderFailureSeparateFromFreshness`), and the TUI renders the two independently
without fabricating a candidate retry
(`TestRenderRuntimeProgressKeepsProviderTransportFailureSeparateFromFreshness`).
The existing current-versus-next model tests and Console build remain part of
the same card. This is only R4b-1--R4b-3 local evidence: C-R4b stays open for
the full read-only integration exit and R5 process/deployed evidence.

**R1 owner-observation partial local evidence (2026-07-20).** MCP and Web
now have EventHub-handler regressions which distinguish a confirmed
catalog/configuration revision change (typed `RuntimePrerequisiteStale`) from
an unavailable catalog/configuration source (ordinary typed infeasibility with
no stale payload):
`TestMCPOwnerProbeDistinguishesCatalogDriftFromCatalogUnavailable` and
`TestWebOwnerProbeDistinguishesConfigurationDriftFromUnavailableConfiguration`.
`TestSSHOwnerProbeReturnsTypedStaleBeforeAnyNetworkOperationWhenPolicyChanges`
proves SSH policy drift is detected before a network operation can be opened.
These tests are owner-local and do not grant any owner a refresh budget,
re-drive or Result authority. R1 remains open until its Result delivery and
all required LLM/read-path exits are complete.

### R4 — Authoritative delivery and descriptive user surfaces

**Bounded components.** `kernel/run`, `kernel/eventstream`,
`application/workorchd` projection and HTTP mapping, remote backend,
`services/workorch` TUI and `web/workorch-console`.  These are readers and
adapters: they consume persisted facts and issue only typed Start, unified
Resume, workspace update or ticket-resolution ingress.

| Persisted source fact | Required displayed/mapped data | Read-path invariant |
| --- | --- | --- |
| active/blocked/terminal Run aggregate | lifecycle, blocker/allowed action, terminal-delivery phase | projection loss/replay does not alter lifecycle or drive authority |
| prerequisite refresh | owner/domain/subject, expected/observed revision, source, used/limit and refreshing/exhausted state | never rendered as candidate retry or provider retry |
| provider transport failure | sealed target, endpoint class, phase, bounded remediation | never converted to stale or a fallback target |
| workspace/default and active/sealed target differ | next-run default plus current-run sealed target | Resume contains no target override |
| runtime command surface | `resume` as the sole continuation command | no executable `restart`/`replan`; replan-style text remains a Decision ingress only |

**Required code-level assertions.**

1. A projection replay after projection-store loss rebuilds only derived data;
   it does not call `DriveRuntimeV2`, resolve a ticket, create a Result, or
   call an owner operation.
2. A terminal persisted before Result/owner projection remains the Run
authority after replay; only terminal delivery retry is allowed.  A recoverable
stale remains non-terminal until the reducer persists refresh exhaustion.
3. Remote/read-model/TUI/Console distinguish prerequisite stale/refresh from
   provider transport failure and show the current sealed target separately
   from the next-run default.
4. TUI and Console expose only `resume`.  Historical persisted `restart`
   labels may be rendered as neutral history, but cannot be submitted as a
   command.  Existing TUI tests rejecting `/run restart` and `/decision
   replan` are necessary but do not replace the projection and target-display
   assertions.

**Local commands.**

```bash
go test ./internal/modules/kernel/run/... -count=1
go test ./internal/modules/kernel/eventstream/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
make console-build
```

**Handoff.** R4 is complete only when all inputs above remain read-only.  It
then hands the persisted aggregate and projection evidence to R5; it never
claims a deployed execution result.

### R5 — File-backed restart matrix and fresh deployed acceptance

**Bounded components.** File-backed test fixtures, verifier scripts and a
freshly deployed service/workspace.  R5 validates the same persisted contracts
from R1--R4; it is not a place to introduce behavior or test-only retries.

| Acceptance boundary | Required persisted evidence | Failure means |
| --- | --- | --- |
| owner effect -> receipt | exact execution key/fence and one owner effect | reopen owning capability contract (R1 or File/Shell/Git card) |
| receipt -> invalidation/evidence CAS | one receipt/evidence lineage and exact intersecting observations | reopen orchestrator freshness authority |
| invalidation -> reacquisition -> seal | new transaction at attempt 0; no old dispatch reuse | reopen orchestrator prerequisite/dispatch contract |
| terminal -> Result -> Run/Workspace projection | one immutable terminal, one Result and eventually complete owner projection | reopen R1 Result delivery or R4 projection boundary |
| fresh deployed run | run/workspace IDs, sealed target/revision, observations, invalidations, receipt fences, transaction/attempt lineage, budgets and accepted terminal | no deployment claim; record the blocking trace and reopen earliest owner |

The six local restart boundaries and the File->Shell->Git positive chain are
already necessary fixture evidence, but **not** a replacement for the last
row.  Fresh deployed acceptance must use a newly created workspace/run after
the newest service deployment; it must record the evidence listed above and
must fail on stale-as-`candidate_retry_exhausted`, side-effect replay,
unrelated refresh, active-target patch or unannounced provider fallback.

**Final command bundle before the deployed trace.**

```bash
make verify-boundary-contract
make verify-runtime-v2-positive-convergence-file
make verify-ci-mvp-file
make release-check
git diff --check
```

## Interface Annex: Contracts To Implement Without Guesswork

This annex is normative for R2--R4 implementation.  It gives the boundary
shape needed to make each component independently testable; it does **not**
introduce a second public Runtime V2 command or a generic transport envelope.
Concrete Go names may follow the existing owner packages, but the fields and
semantics below must be retained in the corresponding typed owner command,
result, error or read DTO.

### A. Runtime Decision governance resolution ingress

For a Runtime V2 governance continuation, the **Decision** owner is the sole
authority for whether an ingress intent is permitted.  HTTP, TUI and Console
only map the request to the typed Decision command and map its typed result
back to the caller.  Generic Approval is not an alternate runtime-ticket
owner: it may persist its own approval state/evidence, but without a distinct
aggregate-level Approval binding it cannot normalize a runtime intent or cause
an automatic continuation.

| Contract element | Produced by | Consumed by | Required semantics |
| --- | --- | --- | --- |
| `TicketID`, `WorkspaceID`, `RunID` | ingress mapper | Decision owner | The full persisted scope is supplied; ticket-only lookup may not silently bind a ticket from another workspace/run. |
| `RequestedIntent` | ingress mapper | Decision audit/normalizer | Keeps the user text/intent such as `replan`; it is audit data, never a Runtime V2 action. |
| `ResolutionID` / idempotency identity | caller or generated ingress identity | Decision persistence and Orchestrator continuation | Same identity returns or joins the prior durable result across duplicate submit and restart. |
| `AllowedRecoveryActions` + ticket binding | persisted governance ticket | Decision normalizer and Orchestrator validation | The allow-list is ticket-specific.  A global enum, UI menu or route parameter is not an authority. |
| `NormalizedResolution` | Decision owner | Orchestrator governance-resolution commit | Exists only when the ticket binding permits it.  Replan-style ingress maps to the concrete permitted resolution and records its original intent. |
| `RuntimeDecisionActionConflict` | Decision owner | HTTP/TUI/Console mapper | Contains `TicketID`, requested intent/action, stable list of allowed actions, remediation text and conflict code.  It maps to HTTP `409`, not `400`. |

The conflict is an expected domain result, not a bare formatter error.  The
presentation layer may format it, but must not parse an error string to infer
allowed actions.  Its minimum user-facing response is equivalent to:

```text
code: runtime_decision_action_unsupported
ticketId: <persisted ticket>
requestedIntent: replan
allowedActions: [continue, reject]
remediation: choose one allowed action or update the governing request
```

The only permitted **automatic Runtime V2** continuation sequence is:

```text
ingress -> Decision durable resolution commit
        -> ticket-bound Orchestrator governance-resolution commit
        -> Orchestrator singleflight re-drive/projection
```

No application watcher, HTTP handler, CLI dispatcher, TUI or Console step may
append a second `ResumeRunCommand` after the resolution commit.  A scheduler
notification is valid only when it carries the persisted resolution identity
and asks Orchestrator to join its own continuation lane; it is not a direct
Run resume and cannot carry a provider/model/definition override.

For generic Approval, the permitted sequence ends at its owner-local durable
state/evidence commit.  Where the Run is otherwise admissible, the user may
later issue the same scoped `resume` command used everywhere else; that is a
new explicit user action, not an Approval-derived scheduler continuation.

### B. Provider target lifecycle and read DTOs

The following fields must be kept separate in activation, persistence and all
read surfaces.  A display implementation may omit empty optional diagnostic
fields, but must never substitute one target for the other.

| Field / value | Writer | Immutable scope | Required reader behavior |
| --- | --- | --- | --- |
| `Workspace.DefaultLLMTarget` | Workspace owner `/model` typed update | mutable workspace preference | Display as `nextRunTarget`; changing it is immediately visible but does not alter active work. |
| `RuntimeBootstrap.Activation.ProviderModelTarget` | activation builder | new Run input | Copy the precedence result `explicit -> workspace -> system` before Start. |
| `ProviderCeiling` target + observed configuration/catalog revision | LLM owner prerequisite acquisition | sealed Run target, mutable observation revision | Show it as `currentRunSealedTarget` plus optional diagnostic revision; a drift may refresh the observation, never the target. |
| `RequestedTargetOverride` on Resume | no writer | forbidden | Validation returns typed conflict before a drive is queued. |

The workspace/read-model DTO must therefore carry both
`currentRunSealedTarget` and `nextRunDefaultTarget` when an active or blocked
Run exists.  Their equality is presentation information, not a reason to
elide the distinction.  A terminal Run may retain its sealed target as history;
the workspace default remains the source for the next activation.

### C. Freshness, provider transport and terminal-delivery read taxonomy

Each condition below has exactly one producer and one policy owner.  The
projection is a value-preserving mapper and may not change class or select a
recovery path.

| Condition | Typed producer result | Policy / state owner | Required read fields | Explicitly not |
| --- | --- | --- | --- | --- |
| owner-confirmed revision drift | `RuntimePrerequisiteStale` naming prerequisite/observation/owner/domain/subject and expected/observed revision | Orchestrator freshness reducer | `phase=prerequisite_refreshing|prerequisite_refresh_exhausted`, resource identity, source and refresh used/limit | candidate failure, provider retry or generic timeout |
| HTTP deadline, connection reset, 5xx or provider overload | typed provider transport/transient failure bound to sealed target and request correlation | LLM failure policy / Orchestrator ordinary candidate path | `phase=provider_failure`, sealed target, failure class, bounded remediation | prerequisite stale or target fallback |
| provider authentication/configuration rejected without owner-confirmed revision drift | typed provider configuration/auth failure bound to sealed target | governance/failure policy | target, failure class and actionable remediation | a hidden refresh or workspace-default substitution |
| terminal already decided but Result/Run/Workspace delivery incomplete | terminal delivery receipt/error with terminal identity/fence | Orchestrator SM-4 delivery lane | `phase=terminal_delivery_pending`, terminal code and completed delivery legs | a new Result judgement, a new candidate or a changed terminal |

### D. Component-local regression recipe

Every R2--R4 change must add a direct test at the owning boundary before an
integration fixture is changed.  The test is considered direct only when the
other owners are fake typed peers; importing or opening another owner's
repository/service invalidates the test and moves the assertion to R4/R5.

| Row | Owner package test arrangement | Trigger | Assertions that must all hold |
| --- | --- | --- | --- |
| R2 Decision/Approval | persisted ticket binding + fake typed continuation sink | permitted, unsupported and duplicate replan-style ingress | normalized audit or actionable conflict; one durable resolution identity; zero/one bound continuation as appropriate; no direct Run resume |
| R2 Orchestrator | persisted governed dispatch + committed resolution fake | duplicate/late/restart resolution delivery | ticket/dispatch/dependency binding checked; one singleflight re-drive; no second budget consumption |
| R3 Workspace/activation | mutable workspace fake + Run bootstrap capture | `/model`, then Resume, then a new activation | active sealed target unchanged; Resume rejects override; new activation uses exact precedence and seals revision |
| R3 LLM | sealed target fake provider catalog/config | unavailable target or confirmed config drift | same target in failure/stale; no fallback; stale and ordinary failure are mutually exclusive |
| R4 projection/eventstream | persisted aggregate + recording side-effect guards | replay after projection loss and terminal-delivery retry | reads only; no Drive, ticket resolution, Result create or owner execute; delivery retries only missing leg |
| R4 remote/TUI/Console | read DTO fixtures | stale, provider failure, distinct current/next target and command input | classes and targets stay distinct; only executable continuation is `resume` |

Required negative assertions should be concrete spies/counters rather than
absence-of-log checks.  For example, a projection replay test needs counters
for Drive, Decision resolution, Result creation and owner execution, all held
at zero; a duplicate resolution test needs a persisted resolution identity and
continuation counter held at one.

### Mandatory per-row implementation record

Append this record to the parent plan immediately after each row's local
gate.  Its purpose is to make component-local proof, system proof and deployed
proof impossible to confuse.

```text
Row: R<n> / H<n>
Status: open | locally closed | reopened | system closed
Changed paths and typed contract: <paths + command/result/value>
Affected SMs: <SM-1..SM-4>
Legal transition: <test + assertion>
Forbidden transition: <test + assertion>
Duplicate/restart/CAS proof: <test + persisted identity/fence>
Budget/cache proof: <test + exact ledger/hash, or N/A with reason>
Local commands: <command + exit code + date>
Downstream exit: <R<n>/P5/deployed trace>
```

Rows R1--R5 are complete only when this record is present and their stated
exits pass.  In particular, R5 stays open until a fresh deployed accepted
terminal trace is attached; it cannot be closed by local test success.

## State-Machine Review Gate

| Change | SM-1 | SM-2 | SM-3 | SM-4 | Required proof |
| --- | --- | --- | --- | --- | --- |
| model preference / new activation | admission only | new definition/facts only | initial transaction only | Start delivery | active sealed Run unchanged |
| unified Resume | active/blocked admission | none unless aggregate requires it | no client reseal | same singleflight re-drive | duplicate Resume joins, not double-drive |
| decision resolution | blocker lifecycle only | reducer receives accepted governance fact | bound dispatch/fence only | Orchestrator continuation | ticket binding/idempotency/restart |
| provider timeout | no synthetic terminal | no freshness unless observed revision drift | normal candidate failure policy only | projection of diagnostic | independent budgets |
| stale/change | recoverable active | pending/satisfied observation | invalidated proposal/dispatch | restart/re-drive | RPF F-01–F-17 matrix |
| terminal delivery | terminal remains immutable | no new facts | no candidate/dispatch rewrite | Result then Run/Workspace retry | six crash boundaries |

Any change that cannot fill its row is not ready for code merge. In particular,
adding a retry in a UI, Decision route, application service, owner adapter or
projection without a corresponding persisted Orchestrator transition is a
forbidden second state machine.

## Completion Evidence Bundle

Before moving the parent plan to archive, attach one record containing:

1. Run/workspace IDs for the fresh deployed acceptance and the sealed model
   target/revision.
2. Observation IDs, invalidation records, receipt fences, candidate transaction
   lineage and refresh/other budget values for the File→Shell→Git path.
3. One supported replan-style ticket submission, its normalized persisted
   action, duplicate submission result and resulting single continuation.
4. One timeout trace containing provider target, configured deadline, failure
   class and user-visible remediation, with evidence that it did not become
   stale or select another model.
5. Output from targeted package tests, `make verify-boundary-contract`,
   `git diff --check`, `go test -p 1 ./... -count=1`, relevant race tests and
   console build.

The complete RPF/ARCH closure still requires the parent plan's P0–P5 and the
separate external restart-fixture matrix. This supplement makes no completion
claim on their behalf.
