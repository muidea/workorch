# Runtime V2 残余代码收口执行手册

Status: completed

Type: implementation runbook

Parent work: [runtime-v2-prerequisite-freshness-closure.md](runtime-v2-prerequisite-freshness-closure.md)

Companion documents: [component closure matrix](runtime-v2-prerequisite-freshness-component-closure-matrix.md) and [system component closure supplement](runtime-v2-system-component-closure-supplement.md)

Last Updated: 2026-07-20

## Purpose

本手册把当前仍未关闭的 Runtime V2 工作收敛为可逐卡实现、独立回归、再做集成验证的
代码队列。它不定义第五个状态机，也不复制核心语义：四状态机、禁止迁移及实现类型以
[`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)、
`internal/modules/kernel/orchestrator/readme.md` 和
`internal/pkg/workorchruntimecontract` 为唯一权威。

一次代码变更只能关闭一张卡的一个 owner 边界。先跑该卡局部回归，再运行指定的交接门；
局部绿灯只能标记为 `locally closed`，只有持久化重启和 fresh deployed trace 通过后才能
标记为 `system closed`。失败必须回到最早拥有该断言的卡，禁止在 UI、projection、
Application 或另一 capability owner 加补偿性 retry。

## Fixed Authority And Handoff

```text
user intent / read query
  -> workorchd, remote, TUI, Console       (map only)
  -> Start or scoped Resume admission      (SM-1)
  -> Run CAS aggregate + Orchestrator      (SM-4 -> SM-2 -> SM-3)
  -> typed EventHub acquire/probe/execute
  -> owner observation | stale | receipt
  -> Orchestrator CAS/reducer/terminal judgement
  -> Result, Run/Workspace projection, Eventstream/read DTO
  -> descriptive user surface
```

| Boundary | Sole formal-state writer | Typed handoff | Explicit prohibition |
| --- | --- | --- | --- |
| Run lifecycle/aggregate | Run owner CAS; Orchestrator makes narrow projection decision | scoped Start/Resume, aggregate update/read, recovery inventory | application queue, UI terminal write, cross-owner repository access |
| authority/focus | Orchestrator reducer | `RuntimeFacts`, prerequisite fact/stale, governance resolution, evidence commit | owner chooses next action; projection reconstructs facts |
| candidate/dispatch | Orchestrator | sealed transaction, candidate result, operation command, owner receipt | execute before seal; candidate attempt 2; owner direct Result/Run mutation |
| owner resource/capability | named File/Shell/Git/MCP/SSH/Web/LLM owner | concrete acquire/probe/execute result, observation, stale or receipt | owner refresh/candidate/repair budget decision |
| terminal storage/projection | Result owner stores judged closeout; Run/Workspace own projection state | terminal closeout/delivery receipt/read DTO | Result re-judges; eventstream/TUI triggers Drive |
| governance ingress | Decision owns ticket resolution; Approval owns approval evidence | ticket-bound normalized resolution or typed conflict | global resolution enum, automatic successor, application second Resume |

Every EventHub-using Module/Block remains `module.go -> biz(embed
basebiz.Base) -> service/adapter`. `basebiz.Base` supplies only unit identity,
Hub/observer and background-routine plumbing; it must never become a runtime
policy, typed-contract registry, repository facade or UI bridge.

## Transition And Budget Rules

| Observed input | Legal transition | Ledger allowed to change | Must not happen |
| --- | --- | --- | --- |
| owner-confirmed prerequisite revision drift before effect | SM-3 invalidated -> SM-2 pending prerequisite -> reacquire -> new transaction attempt 0 | prerequisite-refresh | candidate attempt 1, repair debt, provider fallback or rejected evidence |
| accepted durable receipt with intersecting resource change | retain receipt and atomically invalidate matching observations | no extra budget for invalidation | reuse affected old dispatch or broad string/path matching |
| HTTP deadline/connection/5xx on sealed LLM target | typed provider transport failure through normal candidate policy | provider transport only | `RuntimePrerequisiteStale`, model fallback, client blind retry |
| ticket resolution accepted | Decision durable commit -> bound Orchestrator continuation -> same singleflight drive | governance delivery only | HTTP/TUI/application sends second Resume or patches authority |
| terminal already sealed but Result/projection fails | SM-4 retries missing delivery leg only | terminal delivery | new candidate, Result re-judgement, terminal overwrite |
| duplicate/late command, receipt or resolution | load persisted identity/fence and return/join prior result | none | second effect/request/budget charge |

The user-visible verb is always `resume`. `restart` is historical display text
only. `replan` is Decision ingress/audit intent, not a Runtime V2 action: it
may normalize only through the ticket's persisted allow-list; otherwise return
typed conflict with ticket ID, allowed actions and remediation.

## Residual Component Cards

### RC-1 — Remaining capability owners and Result delivery

**Scope.** `blocks/mcp`, `blocks/ssh`, `blocks/web`, `blocks/llm`,
`blocks/result`, their own events/models, and owner-local tests. This card
implements no reducer or UI policy.

| Owner | Input -> output contract | Required local assertions | Handoff |
| --- | --- | --- | --- |
| MCP / SSH / Web | sealed acquire/probe/execute -> owner-observed catalog/config/policy observation, typed stale *or* ordinary failure/receipt | confirmed revision drift returns stale before effect; unavailable/auth/network failure is not stale; duplicate fence has no second operation | Orchestrator fake-owner and RC-4 read DTO |
| LLM | sealed target + deadline/config -> candidate or typed transport/config/schema failure bound to same target | request context cancels at deadline; timeout is not stale; C0-C2 prefix stable; no fallback; read refresh makes no provider request | Orchestrator candidate path and RC-4 provider display |
| Result | already-judged closeout -> idempotent delivery receipt | stale/recoverable state cannot create Result; duplicate closeout is one durable result; restart retries missing delivery only | RC-4 delivery projection |

SM impact: SM-3 inputs for capability results; SM-4 only for Result delivery.
Forbidden change: do not add provider retry, target substitution, refresh budget
or acceptance decision to an owner. Local tests use fake transport/store, never
a live provider or another owner repository.

```bash
go test ./internal/modules/blocks/mcp/... -count=1
go test ./internal/modules/blocks/ssh/... -count=1
go test ./internal/modules/blocks/web/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/result/... -count=1
```

### RC-2 — Decision/Approval and the single continuation lane

**Scope.** `blocks/decision`, `blocks/approval`, orchestrator governance event
adapter and workorchd ingress mapping. The persisted ticket binding, not route
or UI enum, determines allowed recovery actions.

| Case | Durable writer/result | Required assertion | Forbidden shortcut |
| --- | --- | --- | --- |
| supported replan-style intent | Decision writes one auditable normalized resolution using ticket binding | same `ResolutionID` across duplicate/HTTP retry/restart returns or joins one continuation | runtime `replan`, new successor, authority patch |
| unsupported intent | Decision returns `RuntimeDecisionActionConflict` (`409`) with ticket/actions/remediation | no scheduler/Drive/Resume traffic | bare invalid-argument and UI error parsing |
| generic Approval | Approval writes approval evidence only | scheduler continuation counter stays zero | fabricate ticket binding or automatic Resume |
| committed resolution | Orchestrator validates ticket/dispatch/dependency and joins singleflight drive | one governance evidence and no extra candidate/operation/budget | application resolved watcher sends second Resume |

SM impact: SM-1 blocker projection, SM-2 governance fact, SM-3 bound dispatch,
SM-4 continuation. Direct tests cover legal, unsupported, duplicate, late and
reconstructed-process delivery. A Decision test may fake continuation; the
cross-owner replay belongs to Orchestrator file-backed tests.

```bash
go test ./internal/modules/blocks/decision/... -count=1
go test ./internal/modules/blocks/approval/... -count=1
go test ./internal/modules/kernel/orchestrator/biz -count=1
go test ./internal/modules/application/workorchd/... -count=1
```

### RC-3 — Workspace preference, activation and immutable model target

**Scope.** Workspace typed update, activation builder, Run bootstrap and LLM
target preflight/read mapping.

| Value | Writer/lifetime | Required rule |
| --- | --- | --- |
| `Workspace.DefaultLLMTarget` | Workspace owner, mutable preference | `/model` updates this only; it is the default for the next activation |
| `RuntimeBootstrap.Activation.ProviderModelTarget` | activation builder/new Run | seal `explicit -> workspace -> system` before Start |
| provider ceiling target/revision | LLM prerequisite on sealed Run | refresh updates observation only; cannot migrate target |

Required sequence: update workspace model; prove active Run bootstrap and
sealed ceiling stay byte-for-byte unchanged; prove Resume rejects model
override and uses old target; create new activation and prove workspace target;
make sealed target unavailable and prove no fallback. SM impact is SM-1
admission and SM-4 Start only; it must not mutate SM-2/SM-3 of an active Run.

```bash
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/application/workorchd/biz/appservice/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
```

### RC-4 — Read projection, remote mapping and unified TUI/Console contract

**Scope.** Run/Workspace projection, Eventstream, workorchd read DTO/client,
remote backend, TUI and Console. This is a read/presentation card, not drive.

| Fact | Producer | Required presentation behavior |
| --- | --- | --- |
| prerequisite stale / refresh used-limit / owner-resource-revision | Orchestrator aggregate | show refresh state; never provider failure or retry |
| provider transport failure phase/correlation/sealed target | LLM/candidate diagnostic | concise failure and supported action; never stale/fallback |
| current Run sealed target | Run/bootstrap/LLM ceiling | show current execution target |
| workspace next-run default target | Workspace owner | show independently even if different |
| blocker actions / terminal delivery phase | Decision/SM-4 aggregate | show persisted fact only |

Direct regression is counter-backed: projection replay, remote refresh, TUI
redraw or Console refresh keeps Drive, governance commit, Result create and
owner execute counters at zero. Parser/action tests accept only scoped
`resume`, ticket-resolution intent or workspace update; `/run restart` and
`/run replan` are rejected before EventHub/HTTP traffic. No surface polls to
retry provider or attaches model override to Resume.

```bash
go test ./internal/pkg/workorchport -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/biz/backends -count=1
go test ./internal/services/workorch/service/tui -count=1
cd web/workorch-console && npm run build
```

### RC-5 — Orchestrator/Run recovery and file-backed acceptance

**Scope.** Run aggregate CAS, Orchestrator reducer/controller/executor/drive,
file-backed fixtures and verifier scripts. This card is the only integration
authority for owner observations and receipts.

For every fresh/stale/receipt prove exact owner resource identity; one aggregate
CAS containing fact/evidence/invalidation; new authority transaction with
candidate attempt 0 after refresh; no affected old dispatch reuse; independent
candidate/repair/provider/operation-recovery/refresh ledgers; duplicate/crash
recovery loads aggregate rather than rebuilding authority.

The File -> Shell -> Git chain starts from real bootstrap requirements,
including all canonical owner `operation_admissibility` prerequisites. A fixture
must not insert inventory after start just to pass. Review six crash boundaries:
before owner effect, after owner receipt, after evidence CAS, after
invalidation/reacquisition, after Result persistence and after Run/Workspace
projection. Each proves exactly-once effect/result and eventual projection.

```bash
go test ./internal/modules/kernel/orchestrator/... -count=1
make verify-runtime-v2-positive-convergence-file
make verify-ci-mvp-file
make verify-boundary-contract
git diff --check
```

### RC-6 — Structural and release evidence must be truthful

**Scope.** `scripts/verify-boundary-contract.sh`, release scripts and their
tests. The verifier works without `rg` and rejects a Module/Block lacking local
`biz` embedding `basebiz.Base`, Hub traffic outside Biz, or unbounded EventHub
DTOs. No blanket exemption is acceptable.

The release truthfulness defect was closed on 2026-07-20. Darwin compilation
previously failed on `syscall.TCGETS/TCSETS` and the release loop nevertheless
printed success after `tar` errors. The CLI now uses the portable `x/term`
boundary and the release loop is fail-fast. A deliberately invalid target
returns non-zero without a success message; a temporary-directory full
`make release-check` produces all four archives and `checksums.txt`. This is a
gate/reporting fix only and does not alter Runtime V2 behavior.

The deployed HTTP verifier now also applies bounded, configurable connect and
total curl timeouts to every request. An unreachable service is a failed P5
evidence attempt, not a hanging process or an implicit successful check.

```bash
make verify-boundary-contract
make release-check
```

## Execution Order And Stop Rules

```text
RC-1 contract/failure owners
  -> RC-2 governance and RC-3 activation (independent after shared DTOs settle)
  -> RC-4 read-only delivery/presentation
  -> RC-5 file-backed/restart integration
  -> RC-6 truthful structural/release gate
  -> fresh deployed accepted-terminal trace
```

RC-6 may be implemented independently because it owns no runtime state, but
its green result is required before final deployed claim. HTTP manual success,
Console build, second Resume, fixture-only mutation or release text output are
not substitutes for RC-5/deployed evidence.

## Per-Card Completion Record And Final Trace

Append this to the parent plan whenever a local card closes:

```text
Card: RC-<n>
Status: open | locally closed | reopened | system closed
Changed owner and typed boundary: <paths; concrete command/result/read DTO>
Affected SMs: <SM-1..SM-4>
Legal transition proof: <test and assertion>
Forbidden transition proof: <counter/assertion>
Duplicate/restart/CAS proof: <identity/fence + test>
Budget/cache proof: <ledger/stable-prefix test or N/A>
Local commands: <command, exit code, date>
Downstream handoff: <next RC card or deployed trace>
```

The final deployed step uses a newly created workspace and Run. Record
workspace/run IDs; sealed provider/model target and revision; prerequisite
observation IDs/revisions; invalidations; execution keys/fences and owner
receipts; authority transaction/attempts; all budgets; accepted Result; and
Run/Workspace projections. It must reach accepted terminal without manual
second Resume or fixture injection. On failure reopen the earliest owning card,
not an adapter-side retry.
