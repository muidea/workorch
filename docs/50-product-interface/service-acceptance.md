# WorkOrch Service Acceptance

Status: normative

Type: verification-contract

Last Verified: 2026-08-26

## Purpose

本文定义 workorchd HTTP/Web surface、file-backed persistence 与已部署实例的验收边界。它验证 transport、projection
和 owner integration，不用 API 投影替代 Runtime 状态机验证。Runtime transition、CAS、restart 与 terminal delivery
断言见 [Core Runtime Verification](../00-core/runtime/verification.md)。

## Acceptance Layers

| Layer | Scope | Main evidence |
| --- | --- | --- |
| Static boundary | route/DTO/port、禁止依赖、owner-neutral transport | server package tests + `make verify-boundary-contract` |
| In-process HTTP | status code、scope、pagination、SSE、raw content、projection cache | `go test ./internal/modules/application/workorchd/service/server -count=1` |
| Service smoke | health/readiness/metrics/console/config/list endpoints | `make verify-service` |
| CI-safe business | template/Soul、mock LLM、MCP stdio、workspace activation | `make verify-ci-mvp-file` |
| Persistence/restart | owner repositories、EventRecord/SSE replay、Runtime re-drive | focused repository/server tests + restart scripts |
| Real-provider live | actual provider/MCP/weather/closure behavior | explicit `verify-real-*` / Runtime live targets |

Passing a higher layer does not excuse a lower-layer regression. Conversely, a unit test cannot close a deployed live
gate that explicitly requires provider, process restart or filesystem evidence.

Container、software-dev、governance-resolution 与 service-restart wrappers 必须执行 current typed contracts。Focused
wrapper 必须拒绝空的 `go test -run` selection；boundary verifier 必须拒绝 removed route 或 provider authority field。
静态边界还必须拒绝 `workorchd/biz/execution` 反向导入 `service/server`，并拒绝有效生产/验证脚本把
application scheduler/watchdog/continuation diagnostics、retired metrics 或 automatic-successor evidence恢复为通过条件。
退休指标名只允许出现在 closure observer 的显式“必须不存在”负向断言中。

## Current Mutation Contract

The only execution mutation endpoint is:

```http
POST /api/v1/workorch/workspaces/:workspaceId/execution-activations
```

Acceptance must prove:

- only `resume` and `restart` are admitted; empty intent normalizes to `resume`;
- body fields are limited to `requestId`, `reason`, `source`, `intent`; retired body/query/header Run priority is absent and rejected or ignored as non-authority according to protocol shape;
- `runId`, Goal/DoD content, provider selection and raw Runtime contract fields are rejected;
- the handler waits for a concrete, scope-matched Workspace-owner result and returns its exact workspace/run/request/intent/outcome/reason identity;
- only `reconciliation_requested|replacement_pending` map to `202 accepted`; the other five canonical outcomes map to typed
  `409 rejected`. A single pure classifier supplies HTTP status and `accepted|rejected` display state to HTTP and projection;
  client/Web/TUI consume that classification without reinterpreting outcomes;
- `202` means owner acceptance, never application-local queue admission; no `queued`/`queuedAt` projection is produced;
- a validated owner rejection outcome is returned synchronously and recorded only as a derived rejected observation, never as an owner state rewrite; context/transport/bootstrap/result-contract failures have no owner outcome and therefore return synchronously without writing `runCommand`;
- workspace snapshot只投影最新的immutable `runCommand` owner outcome：state恰为`accepted|rejected`，exact
  `outcome/admissionReason` 与 scope/request identity
  保持一致，且`acceptedAt`/`rejectedAt`互斥；readyz不得暴露另一份 admission/scheduler diagnostic state；
- server配置必须在active/persisted读取以及任一配置更新前拒绝unknown/retired execution scheduler与stale-dispatch watchdog字段，失败更新不得改写原文件；并发压测只观察客户端并发和owner Run事实，不能读取application run limit；
- the same workspace/request is submitted to the owner on every call; `requestId` is the carried identity, and Application不得拼装或投影第二份`idempotencyKey`; durable idempotency and payload-conflict detection remain with Workspace/Run;
- removed run create/start/resume and `execution-restarts` routes return `404` and create no projected command.

Do not reintroduce tests for `POST .../runs`, `POST .../runs/:runId/start`, `POST .../runs/:runId/resume` or
`ExecutionContextPatch` as successful behavior. Their rejection is the retired-surface guard.

## Workspace Deletion Contract

HTTP/owner integration acceptance must prove:

- Application sends exactly one concrete Workspace-owner command and has no Run/resource enumeration, cross-owner purge list,
  directory cleanup, phase, retry journal or partial-ack state;
- an initially non-empty `activeRunId` returns `409` before any durable deletion intent or Run command is created;
- after admission, every ordinary Workspace setup/execution mutation conflicts, including while Run reports busy; an exact retry
  in the stable waiting phase does not consume another Workspace/deletion revision;
- Run's `draining -> retired` fence, Orchestrator's durable `quiescing/quiesced` token aggregate, Workspace's deletion aggregate,
  and every participant's local tombstone survive file-backed reopen;
  recovery re-drives the exact scope and response loss returns the same deterministic receipt; for Run and Orchestrator evidence,
  a crash after physical removal but before the owner aggregate records `purged` must reopen the owner-local pending journal and
  replay the original non-empty observations/proof rather than synthesize a zero-count receipt;
- retain hides Workspace, Run/Step/RuntimeCheckpoint, Result/Artifact, Memory/trace, Decision, Approval and EventRecord reads without physically deleting
  owner records; purge removes only each owner's own records through its exact private sink;
- ordinary create/update/CAS/delete/purge and delayed Runtime/notification writes cannot cross an owner fence, including races
  between independently constructed file repository sets; only the Run-owned opaque Runtime CAS drain lane remains legal during
  `draining`, and it closes atomically at `retired`;
- Run Setup-phase recovery inventory/readiness/exact-state topics reject every source except `kernel/orchestrator`; after `retired`,
  the inventory omits the Workspace and exact opaque state read returns NotFound, while `draining` exposes it only for terminal drain;
- purge ordering is Run draining/terminal drain, Orchestrator durable quiescing/token seal, Run retired, Orchestrator durable evidence seal,
  Result/Memory/Decision/Approval seal, owner-local purges, safe Workspace-owned directory cleanup, then Eventstream seal/purge
  last; Eventstream retains its tombstone
  and global `StreamPosition` floor;
- a terminal Run commit followed by a lost/pending Runtime delivery-ack CAS, a persisted scheduled/running terminal lease, and
  restart all converge through the exact private drain while Run remains `draining`; Orchestrator Begin has zero durable transition
  until that drain is empty, then the durable gate rejects every Runtime mutation. Generic drive/SM-2/SM-3/provider/capability/
  evidence work stays rejected throughout;
- quiescence tokens bind the exact ordered Run generation/revision and Runtime ref/revision/hash inventory; stale tokens and a
  concurrent opaque CAS produce zero retirement transition, while a completed retirement rejects the CAS;
- duplicate/stale/source-forged commands, payload drift, retain-to-purge escalation, unsafe directory targets and partial cleanup
  fail closed without a second state machine or cross-owner write.

## Definition Admission And Clarification

HTTP/owner integration acceptance必须证明：

- Workspace owner先持久化exact pre-seal candidate；只有pure `admitted_all_of` result可以promotion；
- one-of/any-of/threshold、未声明替代分支或无法确定唯一required relation返回稳定violation与`BlockingTicketID`，且零
  promotion/Run/provider/dispatch/effect；
- candidate 保存的完整 normalized typed Profile snapshot 与`RuntimeProfileContractsHash`精确匹配；hash-only、snapshot drift、
  candidate owner-fact manifest 与 snapshot 派生值不一致均在admission/Start/Restart前拒绝；
- 每个 Profile-required acceptance/deliverable source 缺 exact proof/evidence kind/owner-receipt-capable operation 时返回
  `definition_profile_contract_invalid + select_or_fix_template_profile + BlockingTicketID`；wildcard不能替代exact proof，
  也不能降级为assessment-only focus；
- Profile required owner fact 缺 exact preexisting 或唯一 one-shot acquisition/reobserve closure、存在重复/悬空/环时返回
  `definition_profile_prerequisite_unresolved + select_or_fix_template_profile + BlockingTicketID`；Profile contract snapshot/hash
  permutation稳定；
- corrected static candidate经过promotion、专用Decision resolution与Workspace acknowledgement后才eligible；response loss、
  duplicate或stale binding不能产生第二ticket/revision或运行窗口；
- Run-scoped runtime-definition clarification只投影`revise_definition_and_restart`；通用Decision single/batch resolve与Resume
  均拒绝继续该ticket；
- 修正后的Definition只通过普通candidate/admission产生新revision。显式Restart先持久化matching `ReplacementIntent`，再由
  Run owner与Decision owner的专用replacement command/result收口；HTTP/Web/TUI不提交该内部payload，也不预创建successor；
- runtime clarification的open/bind/ack任一response-loss或process restart复用exact identity，且predecessor不扩图、不接收新
  Definition、不产生affected-focus operation。

setup-draft LLM仍是可选authoring surface；manual Template + Goal + DoD + blank directory必须能够完成candidate/admission，且
不限制startup seal后的bounded Runtime LLM调用。

## In-Process HTTP Baseline

The workorchd server package test suite is the in-process HTTP baseline. It covers at least:

- all supported routes are registered and explicitly removed mutating routes are absent;
- production transport imports neither Runtime session/EventHub nor root appservice facade;
- Workspace/Goal/DoD scope validation and owner-generated identity;
- pre-seal candidate、all-of admission、两类Definition clarification的scope/action/revision isolation；
- workspace LLM selection completeness, CAS revision and scope isolation;
- seven-outcome synchronous execution admission, no pre-owner response/queued projection, exact request/outcome/reason projection,
  typed `409` owner rejection, local/remote parity and restart intent;
- Decision/Approval single and batch resolution;
- 两类Definition clarification ticket在通用Decision resolution中均被拒绝；static corrected admission与Runtime corrected
  Definition + Restart分别走各自专用owner binding；
- Memory/MCP/Soul/Skill/Template management;
- workspace-scoped Run reads and child-resource pagination;
- status wait, terminal snapshot cache and typed owner-notification invalidation;
- runtime-view三轴coverage/assessment/evidence、recoverable-wait reason/condition/deadline和execution-budget摘要；
- TaskProgress/runtime-view/context-bundle对exact canonical blocker输出同一`BlockingResourceRef`与owner-copied required/allowed
  action；same-run extra resource、GovernanceDisplay或字符串不能影响recovery；SM-2 budget与SM-4 recovery budget/audit分离；
- SSE filters, projection-only default, replay by `Last-Event-ID`, persisted replay after restart and bounded buffering;
- Artifact URI / Runtime raw-ref allowlisting and content response headers;
- file-backed persistence, long safe IDs and workspace isolation;
- graceful shutdown deadline and pprof/console/static asset registration.

## Deployed Smoke

`make verify-service` runs `scripts/verify-workorch-http.sh` against an already running service. It does not build, start,
stop or restart the process.

The script verifies:

- `/healthz`, `/readyz` and `/metrics`;
- embedded console HTML, CSS, entry bundle and lazy chunks;
- expected current navigation/interaction strings and absence of retired Web entries;
- configuration, workspace list, Run list and bounded EventRecord list wrappers.

Temporary evidence is removed on success. Set `WORKORCH_VERIFY_KEEP_ARTIFACTS=1` or
`WORKORCH_VERIFY_ARTIFACT_DIR=<dir>` to retain it. Connection and request timeouts are configurable through the script's
`WORKORCH_VERIFY_*` variables.

## CI-Safe Deployed Baseline

`make verify-ci-mvp-file` aggregates:

- `verify-service`;
- `verify-workspace-template-soul`;
- mock-provider `verify-llm`;
- deterministic MCP stdio `verify-mcp-stdio`.

It is the deployed CI-safe baseline: no real provider credentials, weather network dependency or implicit service process
management. The `-file` aliases target the same deployed-instance contract; backend identity is discovered from the service.

`verify-ci-mvp-file`只证明上述基础组合，不自动等于PRD Runtime收敛发布gate。多drive、crash-window、wait、budget、
governance、revalidation、restart、三轴closeout和双Workspace隔离必须逐行满足
[Deployed Runtime Convergence Scenario Matrix](deployed-convergence-scenarios.md)。

## Persistence And Restart

File-backed acceptance must distinguish four data classes:

| Data | After process restart |
| --- | --- |
| Workspace/Goal/DoD/selection | owner repository reload, same identity/revision |
| Run lifecycle and opaque RuntimeStateRecord | Run owner reload; Orchestrator re-drive from aggregate only |
| EventRecord / SSE replay cursor | persisted query/replay remains ordered and workspace-scoped |
| application projection cache | may rebuild from owner snapshots/events; never becomes authority |

Required guards:

- no startup scan mutates inactive/running Run state through workorchd;
- no stale Step/watchdog/checkpoint replay drives Runtime;
- duplicate inventory/notification/re-drive is idempotent;
- invalid/retired persisted data fails at load with no compatibility disposition, owner transition, or provider/capability call;
- current-baseline Restart preserves `ReplacementIntent` and terminal delivery ordering before successor creation;
- replacement owner-private persisted stage/generation lease survives every resolution/termination/materialization cut point；
  process-local task loss、Stop/fresh restart只恢复同一 successor；
- nonterminal Runtime terminal-delivery ledger pollution fails current invariant validation at load；不得生成兼容分类或由 projection 修复；
- repeated restart/resume cannot create duplicate successor, attempt, evidence or terminal delivery.

Exact restart/recovery target names and their environment inputs are owned by the Makefile and scripts. This contract owns the
observable persistence and idempotency assertions only.

## Projection, Pagination And Raw Content

Acceptance should cover the following externally visible rules:

- workspace/run scope is resolved before repository query; duplicate IDs in other workspaces cannot leak;
- `limit` and `cursorAfterId` produce stable, non-duplicated pages;
- Run status `waitState` uses a bounded positive timeout and wakes on typed Run notification;
- HTTP `waitState`只控制long-poll，不解除或触发`recoverable_runtime_wait`；
- recoverable wait、governance wait与terminal failure projection互斥且绑定current Runtime revision；stale/duplicate trigger
  不导致第二次drive、effect或budget consumption；
- requirement/CWI coverage、assessment、evidence三轴逐项返回，`indeterminate`、`partially_covered`和
  `stale_or_superseded`不映射为successful closeout；
- Result projection包含Definition/Run lineage、coverage、accepted evidence、assessment、artifact、known limitations和
  applicable owner revision set/hash；
- execution-budget view等于aggregate sub-ledger的只读汇总，HTTP不存在clear/reset/top-up通用mutation；
- terminal cache invalidation/warmup never treats Eventstream payload as owner state;
- default SSE view emits projection changes; explicit owner/event views remain filtered by workspace/run;
- replay resumes strictly after the supplied event ID and tolerates process restart when records are persisted;
- list/detail payloads carry bounded summaries; large provider/artifact content is exposed only by validated ref/URI;
- path traversal, wrong root, missing scope and cross-workspace ref access fail closed.

## Error Contract

Tests must preserve the mapping defined by the server:

| Error family | HTTP |
| --- | --- |
| invalid argument/body/query | `400` |
| not found/removed route | `404` |
| blocked/conflict/stale revision | `409` |
| admission wait canceled | `408` |
| internal/dependency failure | `500` |

Unknown JSON fields are invalid. Runtime Decision action conflicts additionally return structured remediation fields; tests
must check stable code/action metadata without matching volatile diagnostic prose.

Definition clarification conflict还必须返回typed scope与required action。客户端不能从HTTP code或message推导`continue`；
runtime-definition ticket的唯一产品动作是`revise_definition_and_restart`。

## Real-Provider And Live Gates

Real-provider checks are explicit, credentialed and excluded from CI. Use `make help` and the corresponding verifier script as
the executable inventory for LLM、Runtime convergence、MCP、vector memory、weather and governance live gates.

They must record the deployed revision/config identity, workspace/run IDs, terminal or bounded-live disposition, provider
call evidence and filesystem/artifact evidence where applicable. A valid artifact without later convergence is positive
progress but not complete deployed acceptance.

`verify-authority-live-e2e-closure` 的权威 readback 只由 Workspace/Run/Decision owner 读模型与 workspace snapshot 的
immutable `runCommand` outcome构成。显式 execution activation提交后，matching request的`accepted|rejected` outcome就是
admission completion evidence；后续 Runtime 正向推进由 owner state/projection单独观察。该 verifier 和
`observe-workorch-runtime-closure` 不得读取 readyz/snapshot scheduler diagnostics，不得要求 continuation readback、
application auto-replan、自动 successor 或 watchdog compensation。`workorch_execution_admission_*` monotonic counters可作为
数量证据；退休 scheduler/watchdog metrics必须完全不暴露，observer只能把它们作为负向 marker。

## Change Rule

Any service interface change must update, in one change:

1. route registration and request/response DTO;
2. native route and behavior tests;
3. [Service Command API](service-api.md) and
   [Service Read API](service-read-api.md);
4. Web/TUI client contract when consumed there;
5. this acceptance matrix and the relevant deployed script.

涉及Runtime收敛、crash-window、wait/budget、governance、revalidation、restart或closeout的变更还必须更新并重新执行
[Deployed Runtime Convergence Scenario Matrix](deployed-convergence-scenarios.md)中的对应行；只更新HTTP smoke不构成闭环。

One-off acceptance records and raw run artifacts do not belong in `docs/`. Preserve reproducible assertions in this contract and
the executable verifier; use Git history for removed API or execution-path records.
