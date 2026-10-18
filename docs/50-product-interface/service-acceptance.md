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

正式创建链路为 `POST /api/v1/workorch/workspace-authoring` → workspace-scoped `authoring` 查询／修订 → `authoring/compile` → `authoring/confirm`。创建 RequestID 与原 payload 在当前 Web 页面内存中保留以便精确重试，不保存到浏览器持久存储；服务端 owner 持久创建身份与业务状态。编译接受的 202 只表示 BackgroundRoutine 接受任务，不代表 owner 已完成规格或准入。查询始终以 Workspace 持久 ledger 为准，结果未决不得自动重发 provider。确认绑定原 input／permit／proposal／specification hashes；确认和准入不自动启动 Run。

STEP01 的生产 HTTP／真实进程重启验证入口为 `python3 scripts/verify-workspace-draft-step01.py`。
该入口必须自行在新隔离目录构建生产控制台及 Go 二进制，不接受调用者指定旧 `--binary`；保留实际构建命令／日志、
构建前后源码与内嵌控制台字节身份、二进制哈希，并核验运行进程 executable。dirty candidate 显式记录基准 SHA 与
源码树身份，不能冒称基准提交已通过；构建或验证期间源码漂移即失败。HTTP／进程证据不替代生产 Web 的保存、
刷新、关闭重开等交互证据；验收记录须引用同一候选产物及隔离数据根，不以组件测试数量充当本机验收通过。

浏览器交互验收必须证明：旧浏览器记录不再被读取或写入；新建向导重开为空，关闭有未保存输入或未决事务时明确提示；取消关闭保留页面内存原身份，确认关闭后迟到回执不能污染新会话。刷新后通过列表／详情读取 owner 已保存草稿，不自动创建、保存或确认。已开始的确认通过 owner 校验的 `confirmationRecovery` 参数由用户显式重放，不能从页面旧提案猜造；当前 revision 不改变既有 journal 的精确身份，首次准备仍受 CAS 约束。

STEP02 只验收到持久候选和六审。`scripts/verify-workspace-candidate-step02.py` 必须自行构建并绑定源码／生产控制台／二进制，
使用持久受限证据目录（非 `/tmp`），fixture 仅替换 provider HTTP 出站响应；live 需显式授权、配置及有界观察期限。
精确输入门禁覆盖缺字段、延迟任务、A→B→A、模板漂移，均在 reserve 前阻塞；合法纵向链须达到 `ready_to_confirm`，
完整源承诺与合法 DECLARED、恰好六审、完成重放零额外物理调用、独立文件仓及真实进程重启回读一致。
负例仍须覆盖独立审查缺项、FAIL／UNKNOWN、UNVERIFIED、provider／持久切点及预算；正确阻塞不替代成功链。
生产 Web 的正式入口、显式生成、完整候选展示、刷新／重开仍需独立交互证据；真实 provider 和人工源承诺语义复核另列，
不能以 fixture 或组件数量代替。本步所有路径均不得产生确认、正式 Goal/DoD、准入或 Run 副作用。

前置准备失败验收须使用本地 mock／file-backed owners，证明资源不存在／路径拒绝／大小边界、配置／请求构造及确定预算拒绝在 reserve 前被准确保存；原始 bytes、SHA256、非 UTF-8 和超限未保存说明往返一致。相同失败重复 compile 不增加诊断数或 revision，迟到新失败不得跨 input／template／目标写入；修复资源后同输入显式 compile 可以继续，已有 reserve 未决仍不重发 provider。独立诊断 GET 不调用 provider、不推进确认或创建 Run，包含当前和归档诊断的文件均为 0600；诊断保存失败和集合满必须明确报告，不能假称 owner 已有可查原文。Web 组件验收还须证明受理后 15 秒观察的截止不依赖 GET 成功：连续查询失败或挂起也会结束该观察，仅展示最后一次已知 owner 状态，不猜测后台完成／停止，不自动重发，迟到与跨会话回执仍被隔离；文本解码须保留 UTF-8 BOM（`ignoreBOM: true`），bytes／SHA256 与下载原字节不变；无效 UTF-8 不得替换字符伪装为文本，但已校验原始字节仍可下载。Authoring 失败诊断可分别携带`providerDocument`、`providerWire`与`providerCause`观测事实：providerCause表示transport leaf原始错误bytes，不是parseError安全分类；请求URL包装不保存、不清洗leaf，只能通过受限diagnostic原文读取。三个观测字段各自具有observed/complete/saved/observedBytes/sha256/reason事实；原始内容只在独立读取时带`contentBase64`。UI必须区分未观察、部分观察与未保存，不得把旧 payload（包括空 payload）视为 provider 输出；只对确实观察到并保存的 bytes 核对字节数、SHA-256，使用 fatal UTF-8 解码并保留 BOM，无效文本仍允许下载已校验原字节。parseError安全类别与provider document/wire/cause原始观测是分离数据，不得互相代替或合并hash。

只读进度验收须证明 active 空 Workspace 和已准入无 Run 不显示正式结果完成；旧 accepted Result 不完成 successor，draft／rejected／superseded／accepted slice 或仅 Artifact 也不得完成。成功必须匹配当前适用 Run 的 exact terminal Result 与 startup seal；Runtime overlay、TaskProgress 和 compressed context 使用同一最终事实，不从 terminal 状态或结果数量补造完成。

结构化创作合同验收须覆盖 proposal/review builder 经 typed mapping 到实际 mock HTTP 请求：完整 schema 与 DTO 字段一致、所有嵌套对象闭合、原生能力开启与关闭均保留同一本地严格校验，非法显式 schema 在零 HTTP 调用下失败，旧 schema ID 调用保持兼容。用合成样例验证根及嵌套未知字段、错误类型／枚举、来源身份和自带审查拒绝，不复制 live 原始诊断到测试资产。仅改变当前输入不得改变稳定 C0 合同；provider 不支持 structured output 不得触发隐式 fallback 或第二次调用。

已知提案／审查失败的文件恢复验收须证明：重复 compile 及 owners 重开后仍返回原失败、原 permit、原 revision 和完整诊断元信息，不产生新的编译受理标记、provider 调用或 Run。恢复提示须区分此路径与未 reserve 的准备失败，不能暗示部署修复自动清除历史失败。

确定性验收使用正式 HTTP ports、真实 file-backed Workspace／File／Decision owners 与 mock provider，覆盖六审查、exact confirmation replay、重启、FAIL／UNKNOWN 阻塞及资源漂移拒绝。它不替代真实 provider 或浏览器交互验收：

```bash
make verify-workspace-authoring-contract
```

共享脚本 mock 的本地合同测试也在此目标中执行。它只解析正式 C3 创作输入，回显派生身份、保留候选来源与资源快照。proposal／critic 请求携带从持久模板快照编译的 typed Profile 声明；mock 仅在存在唯一、适用于全部候选项的 `dodItem/*` 声明时回显 DECLARED binding，verifier-feasibility 核对 exact binding，否则返回 UNKNOWN。声明不代表实际执行证据，不能替代 Workspace owner 准入或后续 capability-native proof。

LLM／MCP provider 分发已接入共享 mock；Template／Soul 使用仅支持 authoring 的本地 mock。三条脚本的创建统一通过 `create-authoring-verification-workspace.py` 完成持久创建、六审查、精确确认及零自动 Run 检查，再只读获取正式 Goal／DoD 身份，不调用退休写 REST。恢复 journal 默认位于用户 state 目录，不进入仓库或验收 artifacts，成功后仍保留。脚本语法与本地合同测试不等于 deployed managed 正例；部署验证需要单独授权并实际执行。

验收分别保留可控排队的四场景与真实 BackgroundRoutine 正例；后者不手动运行 queued task，验证 HTTP 202 后自动派生、六审查、确认及 owners 重开。owners 重开不是独立进程退出／启动验证，mock provider 也不替代真实 provider。

另有真实 worker 在派生 provider barrier 内的重复 compile 验收：原物理调用在途时重复提交不产生新派生调用，放行后共完成一次派生、一次 proposal 与六次审查，再验证确认重放和 owners 重开。该证据不替代所有阶段的并发提交、独立进程中断或预算耗尽测试。CI 独立执行本地合同目标，不代表 managed 验收脚本已在部署实例上通过。

另有可控排队竞争场景：四次 HTTP compile 均在草稿阶段接受，任务通过同一启动 barrier 并发运行，验证 owner permit 竞争后只产生八次物理调用，并完成确认及 owners 重开。该场景使用测试任务调度器，不等于真实 worker 的所有调度交错；启动 barrier 也不保证每次运行都同时到达 reserve。需与真实 worker 在途重放场景共同保留，不能替代修订交错、进程中断或全量 race 证据。

完整 software-dev 定义回归位于 Application Biz 的 HTTP 集成层：24 条用户要求与 13 条模板必需项全部进入正式 DoD，经六审查和 exact 确认得到 eligible、零 Run；Restart bootstrap 保留原 StartupInputHash、isolated adapter 与 v2 review，完整编译 42 个 proof／44 个 CWI。重复编译与 Goal／DoD／模板／目录／Definition policy／Profile 漂移拒绝不得增加 Workspace revision 或 provider 调用；typed authoring query 的错误 scope、旧 revision、缺 Record／Confirmation 或重复 Template Soul binding 均拒绝复用准入。未确认规格的显式启动必须零 Run。随后独立提交显式 activation，Run 必须封存同一 startup seal，首次保存默认 provider selection 后原请求重放仍只保留一个 Run、零额外创作调用。该测试抑制异步 drive 通知，验证到 SM-1 materialization，不替代 Runtime 收敛或真实执行。

持久 owner 门禁必须分别验证保存回执同 CAS 竞争只有一个赢家、128 条上限拒绝新请求但保留原重放、历史保存不能冒充当前修订；派生累计 16 次、规格累计 112 次和各阶段两次 attempt 不随输入 revision／RequestID／重开而重置。预算 fixture 不调用 provider，只证明 owner 授权额度，不能冒充物理调用计数。

独立子进程在派生 reserve、派生 completion、确认 prepare、正式 Goal 写入、正式 DoD 写入的持久切点退出，父进程重开后必须复用原 permit／确认身份且精确重放零额外写入。澄清 successor 还覆盖决议前、Decision resolution 已持久化但 Workspace 仍 pending、完整准入后三个进程退出边界；重开须复用原决议和 startup seal，不重复创建 ticket 或 Run。这些受控切点不等于随机 kill 或真实 provider 中断。

验收器必须拒绝不完整证据：`runs` 必须明确为列表且为空；六审查必须覆盖规范 kind、全部 PASS 并绑定当前 proposalHash；待确认规格必须有非空验收项、完整确认身份与一致的 derivationPermitID；确认必须同时返回 `admitted`、`eligible` 和非空 StartupInputHash。缺字段不得按成功或零 Run 处理。

对已启动、配置好 provider 的隔离实例，可显式执行：

```bash
python3 scripts/verify-workspace-authoring-live.py \
  --base-url http://127.0.0.1:8080 --template-id <template-id> \
  --goal '<目标>' --directory <项目目录> --resource <相对文件引用> \
  --recovery-file "$HOME/.local/state/workorch/authoring-live/<验证批次>.json" --confirm
```

该验收会创建持久 Workspace 并调用 provider；不得作为无副作用 smoke 自动执行。脚本不读取 credential，不输出输入、provider 原文或配置，不删除测试 Workspace，也不启动 Run。默认只验收到六审查通过，`--confirm` 额外要求 eligible、精确确认重放与零自动 Run；模板缺可执行 proof 必须阻塞，不能为了通过修改验证声明。

`--recovery-file` 必填，必须放在仓库外；文件以 `0600` 保存原创建输入、实例地址、Workspace ID 及发送前的精确确认请求，包含可能敏感的目标与资源引用，不得提交或公开。脚本持有本地排他锁并原子落盘，防止同一恢复文件被并发执行。响应丢失后使用同一文件、实例和原输入重试；输入或实例变化会拒绝，不覆盖原身份。已保存原确认时直接重放原 hashes 与 expected revision，不重新编译，不以当前提案替换原确认；即使 owner 已推进到 admitted 也可恢复。缺少 `--confirm` 则不恢复确认。该本地文件不是 owner authority，也不替代浏览器交互、进程重启和后续显式执行的独立验收。

恢复驱动区分持久阶段：`draft／derived` 可提交 compile，由 owner reserve 判定 fresh permit；`outcome_unresolved` 可能是正常 provider 在途或中断后未决，验收器在有界期限内只查询，允许原调用完成；超时保留原身份，不自动重发 provider。原确认返回 `confirming／admitting` 时，验收器查询并只重放 journal 中的原确认 hashes 与 expected revision，超时保留原身份；owner 已进入确认事务而本地没有原确认时 fail closed，不从当前提案构造替代确认。

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

普通 Web 创建只提交 Template、Goal、资源引用和可选用户验收要求，目录可留空。正式 DoD 由持久创作、六审查和用户精确确认事务物化；退休 setup-draft session 与手填 Goal／DoD CRUD 不能作为准入旁路。CLI／TUI 未提供完整创作向导时明确拒绝旧写入口。startup seal 后的 bounded Runtime LLM 调用仍遵守 Runtime 合同。

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
