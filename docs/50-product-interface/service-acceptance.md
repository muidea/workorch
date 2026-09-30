# WorkOrch Service Acceptance

Status: normative

Type: verification-contract

Last Verified: 2026-09-05

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

## 持久规格创作 Live 验收

正式创建链路为 `POST /api/v1/workorch/workspace-authoring` → workspace-scoped `authoring` 查询／修订 → `authoring/compile` → `authoring/confirm`。创建 RequestID 与原 payload 必须保存并可精确重放；编译接受的 202 只表示 BackgroundRoutine 接受任务，不代表 owner 已完成规格或准入。查询始终以 Workspace 持久 ledger 为准，结果未决不得自动重发 provider。确认绑定原 input／permit／proposal／specification hashes；确认和准入不自动启动 Run。

确定性验收使用正式 HTTP ports、真实 file-backed Workspace／File／Decision owners 与 mock provider，覆盖六审查、exact confirmation replay、重启、FAIL／UNKNOWN 阻塞及资源漂移拒绝。它不替代真实 provider 或浏览器交互验收：

```bash
go test ./internal/modules/application/workorchd/biz \
  -run '^TestAuthoringHTTPFileBackedCompilationConfirmationAndRecovery$' -count=1
python3 scripts/test-verify-workspace-authoring-live.py
```

验收器必须拒绝不完整证据：`runs` 必须明确为列表且为空；六审查必须覆盖规范 kind、全部 PASS 并绑定当前 proposalHash；待确认规格必须有非空验收项、完整确认身份与一致的 derivationPermitID；确认必须同时返回 `admitted`、`eligible` 和非空 StartupInputHash。缺字段不得按成功或零 Run 处理。

对已启动、配置好 provider 的隔离实例，可显式执行：

```bash
python3 scripts/verify-workspace-authoring-live.py \
  --base-url http://127.0.0.1:8080 --template-id <template-id> \
  --goal '<目标>' --directory <项目目录> --resource <相对文件引用> --confirm
```

该验收会创建持久 Workspace 并调用 provider；不得作为无副作用 smoke 自动执行。脚本不读取 credential，不输出输入、provider 原文或配置，不删除测试 Workspace，也不启动 Run。默认只验收到六审查通过，`--confirm` 额外要求 eligible、精确确认重放与零自动 Run；模板缺可执行 proof 必须阻塞，不能为了通过修改验证声明。响应丢失可用打印的 `--request-id` 与原输入重放。此 live gate 不替代浏览器交互、进程重启和后续显式执行的独立验收。

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
- 每个 Profile-required acceptance/deliverable source 若不能映射 admitted exact CWI、不能由 activation compiler 生成
  canonical exact proof declaration，或缺 evidence kind/owner-receipt-capable operation，则返回
  `definition_profile_contract_invalid + select_or_fix_template_profile + BlockingTicketID`；wildcard只能作为已 admission exact CWI
  的声明模板，不能注入 current owner fact、recipe/native check、dispatch/credit authority，也不能降级为assessment-only focus；
- Profile required owner fact 缺 exact preexisting 或唯一 one-shot acquisition/reobserve closure、存在重复/悬空/环时返回
  `definition_profile_prerequisite_unresolved + select_or_fix_template_profile + BlockingTicketID`；Profile contract snapshot/hash
  permutation稳定；
- managed CWI 的 recipe-wide readiness 与逐 CWI readiness 分离；同一 surface 的 ready binding/missing preparation、native
  identity 跨 CWI 唯一性、File prospective all-of receipt、Shell fresh reobserve 和 restart 必须有真实 owner integration
  验收。preparation/文件写入/exit 0 均不产生 validation credit；Application 未经 Profile 声明不得自动生成该跨 owner
  convergence，MCP-only/无 Shell recipe 定义不得被强制进入 Shell preparation。稳定 ID `software-dev` 在产品界面必须保持
  语言与框架中立；Go-native 等具体 adapter identity 只出现在 capability-owner 合同与诊断中，不能进入模板名称/描述，也不能由
  目录扫描或 Application/Orchestrator 猜测；
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

验证适配器必须遵守同一份 owner admission 合同：成功响应统一由 `verify_execution_activation.py` 校验，
`replacement_pending / terminal_predecessor_successor_requested` 是合法的显式 Restart 准入事实，不等于 successor
已经创建。无效响应必须在继续轮询前返回非零，包括 Bash 命令替换与条件调用场景；不得由脚本改写 Run 状态。
MCP semantic mock 只从当前 `C3.semantic-authority` 的逐 CWI proof obligation 复制 `declarationRef`，拒绝缺失、重复及
跨 batch obligation；不得从 CWI ID、原始 Definition 或诊断文本重建引用。相关回归纳入 `verify-boundary-contract`。

Managed runner 的失败摘要保留步骤日志头尾，LLM/MCP verifier 不向终端倾倒完整 snapshot。JSON response 与 mock log
在失败时保留于 managed artifact root 下的独立 verifier 目录，随 CI artifact 一同收集；成功时由
`WORKORCH_VERIFY_KEEP_TMP=1` 控制保留。独立运行 verifier 的失败证据保留于其输出的临时目录。

`verify-ci-mvp-file`只证明上述基础组合，不自动等于PRD Runtime收敛发布gate。多drive、crash-window、wait、budget、
governance、revalidation、restart、三轴closeout和双Workspace隔离必须逐行满足
[Deployed Runtime Convergence Scenario Matrix](deployed-convergence-scenarios.md)。

## Deployed Convergence Evidence Gate

场景producer必须把每个独立执行的`*.dce.json`及其相对路径artifact保留在同一个evidence root。发布聚合器不把
`verify-ci-mvp-file`、unit test、shell退出码或普通`summary.json`转换成场景证据，只进行以下主动校验：

- `make verify-deployed-convergence-manifest-tests`仅验证manifest verifier自身的schema、path、digest、scope、assertion和缺失
  scenario fail-closed回归，不属于deployed evidence；
- `make verify-deployed-convergence-ci-safe`要求`DC-01/03/05/06/07/08/09/10`全部存在且可验证；缺少`DC-06`等任一项时返回非零；
- `make verify-deployed-convergence-release`按`DEPLOYED_CONVERGENCE_PROFILE`验证`ci-safe`、`definition`、`real-provider`、
  `profile`或`full`集合；manifest root和aggregate report分别由`DEPLOYED_CONVERGENCE_MANIFEST_ROOT`与
  `DEPLOYED_CONVERGENCE_REPORT`指定；
- 每个manifest的`producerEntrypoint`还必须命中逐`scenarioId + scenarioVariant` exact producer registry；schema-valid手写文件、unit fixture或尚未接线的
  历史script summary均不能通过release gate；
- `make verify-deployed-full`最后执行`full`聚合，且分别要求`DC-01/ci-safe/non-real`与`DC-01/real-provider/real`；没有其中
  任一项、`DC-11/12`definition evidence、real-provider `DC-02`或具体profile `DC-04`时都不能形成发布通过结论。

manifest schema和逐场景mandatory assertion/ref要求见
[Deployed Runtime Convergence Scenario Matrix](deployed-convergence-scenarios.md#evidence-manifest)。服务验收脚本若未显式产出该
结构，就仍只是对应局部合同检查，不能被聚合器自动提升为发布证据。

## Persistence And Restart

File-backed acceptance must distinguish four data classes:

| Data | After process restart |
| --- | --- |
| Workspace/Goal/DoD/selection | owner repository reload, same identity/revision |
| Run lifecycle and opaque RuntimeStateRecord | Run owner reload; Orchestrator re-drive from aggregate only |
| EventRecord / SSE replay cursor | persisted query/replay remains ordered and workspace-scoped |
| application projection cache | may rebuild from owner snapshots/events; never becomes authority |

Required guards:

- every owner-backed single-file publication uses the repository-neutral crash-safe commit primitive: a same-directory
  temporary file is written completely, mode-set, file-synced and closed before atomic rename, then the parent directory is
  synced before durability is acknowledged; repositories must not expose an in-place truncate/write window on the canonical
  path;
- a failure before rename leaves the previous canonical value and cache authoritative. A failure after rename is
  `published but durability unacknowledged`: the caller returns the error, but any process cache must be advanced or evicted so
  that a retry fresh-reads the published generation instead of overwriting it from an older cached revision. Temporary files
  are never recovery authority and a corrupt canonical file continues to fail closed;
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
call evidence and filesystem/artifact evidence where applicable. A durable artifact without a matching validation/CWI/Result
acceptance delta is effect progress, not proof of positive acceptance convergence or complete deployed acceptance.

Closure observer 在观察窗口前后各采集 Workspace snapshot，以同一 `workspaceId/runId` 的 `acceptance.audit` 比较
owner-accepted evidence ID、credited validation ID、durable-effect execution key 和 satisfied CWI ID。独立反馈 activity、effect
与 acceptance delta；只有 validation/CWI/Result acceptance delta 或正式 accepted completion 可报告 `positiveProgress`。
仅有修复 effect 时报告 `effects_without_acceptance_progress`。`active` 是 SM-1 执行态；心跳、LLM usage、
state revision 变化仅说明 activity，不证明目标正向推进。没有可比 baseline 或窗口内零增量只能报告未观察到进展，不能
据此推断服务永久卡住；完整 completion 则要求当前 Run 已 completed、当前 Run accepted result 和 coverage 全闭合。
historical Run result、重复 receipt/fence、已有局部效果不能覆盖当前 failed/stalled/ineffectiveRetry。治理 blocker 单独分类，
`pass=true` 仅表示受治理等待可解释，`positiveProgress=false` 明确表示不是自动正向推进。Readiness 与这些判定相互独立。
`scripts/test_observe_runtime_progress.py` 随 boundary gate 执行，observer 不从上述视图生成 mutation。

Workspace heartbeat 的 `latestActivityAt` 与 `latestEffectiveProgressAt` 必须分开：前者是当前 Run 的活动时间，后者只来自
已接受且具有 CWI credit 的 exact owner receipt 及当前 Run accepted Result。零 CWI credit 的 changed durable repair 只更新
独立 `latestDurableEffectAt/durableEffectCount`。单次失败、无关检查的历史重复或上一份 projection 的 retry 标记不能生成当前
`ineffectiveRetry`；current failure 的实际重复（包括跨 repair closure 的同一 native-check failure）或正式预算耗尽可形成
该诊断。Profile 的 `validationRepairPolicyId/validationRepairMaxAttempts` 与全局 repair limit 分开显示，不从错误文本推断。
Web/TUI 与 heartbeat age metrics 共用这份只读投影，不生成新状态机或恢复动作。

Builtin-defaults HTTP 验收同时覆盖只读 current/target preview 与显式单资源 guarded upgrade，必须证明 missing/wrong/duplicate
owner result fail-closed、定制与 stale 请求 `409`、写后响应丢失/fresh-owner retry 幂等，以及普通启动只更新 exact reviewed
未定制版本、保留定制/未知内容。升级 catalog 不改变旧 Run 封存输入；新 Start/Restart 自动读取新配置并追加准入，
必须证明旧 candidate/Run 原样、无效新配置阻塞、Resume/recovery 不热替换。[Built-In Default Resources](../40-profiles-capabilities/workspace/builtin-default-resources.md)
是该专题合同。

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
