# Runtime V2 单 Owner 状态机、统一执行入口与 CWI Reconciliation 收口计划

Status: active

Type: closure-plan

Last Updated: 2026-07-31

## 1. 本轮审计结论

Runtime V2 的核心 reducer、Run/Workspace/Result 的持久化 owner 边界，以及 CWI
reconciliation 的 `retain/reopen` 规则已经存在；但生产代码仍保留第二条 Run lifecycle
入口，并在 application/backend 层丢弃 owner 裁决。因而不能宣称整体执行过程已经符合
single-owner 设计，也不能把 `202 Accepted`、TUI 的“submitted”或 terminal `completed`
当作正向推进的充分证据。

本计划关闭的不是某个 candidate JSON 字段，而是下列共同根因：

1. `application/workorchd` 仍能按调用者指定的 Run 直接 Create/Start/Resume；
2. `blocks/run` 接受 `activate`、`continue` 时没有限制实际 EventHub source；
3. Workspace owner 的 `RunID`、`Disposition`、`Diagnostic` 被 application 降级为
   `Accepted:true`，EventHub backend 再降级为 `nil, nil`；
4. Orchestrator 的 Drive 与 termination request 仍是未限制 source 的可投递入口；
5. 现有绿灯以 reducer/unit 与协议适配测试为主，缺少 `restart -> run-002 ->
   workspace_full reconciliation -> terminal delivery` 的真实端到端证明；
6. LSP Bridge 异构 CWI 的 deployed semantic proof 仍未完成，历史空 TODO/generic
   validation 的 false-completion 反例尚未被 fresh live 矩阵关闭。

本文件是唯一 active Runtime V2 收口队列。归档的 C12、VPC、RUNTIME-SMO 等仅可作为
回归基线，不能恢复旧生命周期入口、兼容分支或另一套状态迁移规则。

## 2. 不可变设计规则

### 2.1 单 owner 与内部事件

外部组件只能发送 immutable intent 或已持久化 factual observation/receipt/delivery
acknowledgement。外部合同不得携带接收方目标状态，也不得以事件来源的文案、error text
或投影状态选择迁移。接收 owner 必须：

```text
external typed input
  -> identity/source/scope validation
  -> owner-private internal event
  -> owner reducer
  -> owner CAS
  -> immutable notification/fact
```

不得存在 `external command -> reducer/CAS` 的捷径。不存在兼容 fallback：旧 Run 若缺少
当前 schema 所需 bootstrap/reconciliation contract，必须返回
`fresh_activation_required`，由明确的 restart 创建 replacement Run。

### 2.2 状态机 owner 表

| 状态机 / 资源 | 唯一 owner | 允许的跨组件输入 | 禁止事项 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | Workspace execution-selection fact、Orchestrator sealed lifecycle/delivery fact | application、TUI、Web 或 governance 直接 Start/Continue/terminal Run |
| Workspace active-run slot | `blocks/workspace` | Run state fact、Run fresh-activation release fact、sealed terminal delivery fact | 创建/选择 Run、解释 CWI 或写 Runtime aggregate |
| SM-2 authority/focus/coverage/reconciliation | `kernel/orchestrator` | capability observation、operation receipt、definition/reconciliation fact | 直接执行 operation、让 observation 新增 credit、读取 owner repository |
| SM-3 candidate/sealed dispatch | `kernel/orchestrator` | LLM document、owner feasibility、operation receipt | LLM/capability owner 选择 repair、ticket 或 terminal |
| SM-4 drive/recovery/delivery | `kernel/orchestrator` | Run active/continuation fact、persisted lease timer fact、delivery ack | 选择 CWI/focus/candidate/repair、写 Run/Workspace/Result |
| Result lifecycle | `blocks/result` | sealed terminal delivery fact | projection/UI/Run 直接设置 accepted/rejected |

Orchestrator 可以拥有 SM-2、SM-3、SM-4，但三者必须使用分离的 internal-event kind、
reducer entry 和可写字段：SM-4 只能调度、lease、recovery、delivery acknowledgement；它不
能借“重驱动”重新选择 authority。共享 `RuntimeAggregate` 不是跨状态机写权限。

### 2.3 workspace scope

全部 command、fact、receipt、lease、idempotency key、singleflight key 与 projection item
必须带 `WorkspaceID`；Run 范围另带 `RunID`。入口先绑定 checked workspace context，已
有不匹配 scope 必须 conflict。任何 Biz、service、module、global cache 或 background task
不得保存未受该组合 identity 约束的“当前 workspace/run”。

## 3. 目标执行路径

```text
TUI / Web / remote
  -> WorkspaceExecutionIntent{resume | restart, workspaceID, requestID}
  -> application adapter: compile restart bootstrap only; return owner result unchanged
  -> Workspace owner internal event: validate current definition/slot; emit selection fact
  -> Run owner internal event:
       resume  = choose same active non-terminal Run
       restart = create replacement Run + predecessor supersede fact
  -> RunStateChanged / RunContinuationRequested fact
  -> Orchestrator SM-4 owner-private schedule/lease
  -> SM-2 reconciliation request + owner observations -> retain/reopen only
  -> SM-2 authority -> SM-3 candidate/sealed dispatch -> capability receipts
  -> SM-2 sealed terminal -> SM-4 delivery fact
  -> Result owner internal event -> Workspace owner internal event -> Run owner internal event
```

`resume` 与 `restart` 都必须 reconciliation：前者对同一非终态 Run 的 sealed selected CWI
执行 `current_run` scope；后者创建 replacement Run，对 Workspace 当前 Goal/DoD/profile 的
完整 binding 执行 `workspace_full` scope。restart 不继承 predecessor evidence、focus、dispatch、
repair ledger 或 coverage credit。

## 4. 已确认偏差与代码范围

| ID | 偏差 | 主要位置 | 影响的状态机 |
| --- | --- | --- | --- |
| SO-01 | 旧 application Run create/start/resume facade 和写 API 仍在 production tree | `application/workorchd/biz/appservice/run_service.go`、`runports/commands.go`、`service/server/http_run.go` | SM-1、Workspace slot |
| SO-02 | Run `activate`/`continue` command 无 actual source guard | `blocks/run/biz/lifecycle_handler.go` | SM-1 |
| SO-03 | Workspace execution owner result 被截断成 `Accepted:true` / `nil` | `application/workorchd/biz/owner.go`、`services/workorch/biz/backends/eventhub.go` | ingress feedback、SM-1 可观测性 |
| SO-04 | scheduler/projection 记录 queued/accepted，却不知道 owner 的真实 disposition | `application/workorchd/service/server/workorchd_scheduler_commands.go`、`workspaceviewcore` | projection / ingress |
| SO-05 | Drive 和 termination handler 未限定合法 producer | `kernel/orchestrator/biz/runtime_v2_drive.go`、`owner.go` | SM-4、terminal delivery |
| SO-06 | replacement/reconciliation 的端到端矩阵缺失 | TUI/backend/application/workspace/run/orchestrator tests | SM-1～SM-4 |
| SO-07 | LSP Bridge profile 的逐 CWI semantic live proof 未完成 | activation profile/template、File/Shell/MCP、managed deployed verifier | SM-2 evidence semantics |

## 5. 分组件收口清单

本计划覆盖以下全部已注册运行单元；路径是组件边界的唯一索引，表中职责不赋予跨 owner 状态写权限：

`internal/initiators/workorchruntime`、`internal/modules/application/workorchd`、
`internal/modules/blocks/approval`、`internal/modules/blocks/decision`、
`internal/modules/blocks/eventstream`、`internal/modules/blocks/file`、
`internal/modules/blocks/git`、`internal/modules/blocks/llm`、
`internal/modules/blocks/mcp`、`internal/modules/blocks/mcpregistry`、
`internal/modules/blocks/memory`、`internal/modules/blocks/result`、
`internal/modules/blocks/run`、`internal/modules/blocks/shell`、
`internal/modules/blocks/skill`、`internal/modules/blocks/soul`、
`internal/modules/blocks/ssh`、`internal/modules/blocks/web`、
`internal/modules/blocks/workspace`、`internal/modules/kernel/governance`、
`internal/modules/kernel/orchestrator`、`internal/modules/kernel/system`。

| 组件 | 必须完成 | 禁止遗留 |
| --- | --- | --- |
| `application/workorchd/biz` | 将 scheduler port/result 改为透传 typed `WorkspaceExecutionIntentResult`；restart 只无状态编译 bootstrap；应用层不保存 Run/coverage/lease 状态 | `Accepted:true` 代替 disposition、lifecycle 决策、Run repository/service 直写 |
| `application/workorchd/appservice/runports` | 删除 production `CreateRun`、`StartRun`、`ResumeRun` path 与对应生产 facade；读模型保留只读 owner port | 用客户端 run ID/goal/DoD 创建可执行 Run、直投 activate/continue |
| `application/workorchd/service/server` | 删除 `POST /workspaces/:workspaceId/runs`；execution 仅走 `/execution-activations`；202 投影记录 request lifecycle 与真实 owner outcome | HTTP 选择 target Run state、伪成功、projection 反向驱动 |
| `services/workorch` TUI/EventHub/remote | backend 返回 `RunID/Disposition/Diagnostic`；TUI/Web 根据真实 disposition 展示，并以 requestID 刷新投影 | 空 `RunInfo`、把 transport accepted 显示成 execution started、自动 restart |
| `blocks/workspace` | 保持 external intent -> Workspace internal event；仅从 Run fact 修改 active slot；将 Run result 原样返回给 ingress | 读取/写 Run repo、创建 replacement、选择 coverage/drive |
| `blocks/run` | 外部只接收 Workspace selection fact；activate/continue 变为 Run-private internal events，或严格限制为 Run Biz 自发 event；Run owner 负责 replacement/predecessor 语义 | 任意 source 触发 SM-1、application direct Start/Resume、coverage 选择 |
| `kernel/orchestrator` | 收紧 Drive/retry/termination producer；SM-2/3/4 只经 internal event controller CAS；SM-4 只使用 persisted lease/singleflight/delivery ledger | 外部直接 drive、SM-4 改 authority、直写 Run/Workspace/Result |
| `blocks/result` | 仅消费 Orchestrator sealed terminal delivery 并转内部 event；维持 idempotent delivery fence | Result target state command、从 projection 接受完成结果 |
| `blocks/file`、`shell`、`mcp` | 只返回 binding-grounded observation/receipt；缺 target/check/recipe/behavior 时给 factual insufficient/unverifiable/stale | 直接 close/reopen CWI、generic success 扇出 credit |
| `blocks/git`、`llm`、`ssh`、`web` | Git 只 revision fact，LLM 只 candidate document，其余 capability 只 typed receipt | 以 commit/claim/readiness 替代语义 proof、写 aggregate 或 Run state |
| `decision`、`approval`、`kernel/governance` | 只持久化治理对象及 resolution fact；由 Orchestrator 消费后决定后续事实 | Resume/Drive Run、写 coverage/terminal |
| `eventstream`、readmodel projection、TUI/Web view | 展示 request/outcome/reconciliation/delivery lineages；只读 | 将 queued/accepted 投影作为 authority、反向投递 lifecycle/drive |
| `initiators/workorchruntime`、`system`、`memory`、`skill`、`soul`、`mcpregistry` | 保持基础设施、配置或资源 owner 职责，并使用 scoped typed event | runtime lifecycle facade、未 scoped 当前 workspace 状态 |
| `internal/pkg/workorchruntimecontract` | 仅 DTO、enum、clone、normalize、validate、hash、纯 evaluation | EventHub、CAS、I/O、timer/retry、reducer、profile/domain 分支 |

## 6. 实施阶段

### Phase A — 先封死第二条生命周期入口（SO-01、SO-02）

1. 删除 application `CreateRunWith*`、`StartRunWith*`、`ResumeRunWith*` 和 `runports` 的生产写命令。
2. 删除 HTTP `POST /workspaces/:workspaceId/runs` 及 serverState 的 `createRun` write dependency；保留 Run read API。
3. Run lifecycle handler 对 `activate`/`continue` 拒绝外部 source；将 replacement activation、startup
   successor activation、owner-local continuation 全部转为 Run private internal event。
4. 扫描整个 production tree：除 Workspace execution-selection fact 外，没有 producer 可投递 SM-1 transition。

**门禁：** 旧路径 404/不存在；伪造 application source 的 activate/continue 被拒绝；只有 Workspace
intent 可令 first/replacement Run 进入 active；所有 Run CAS 仍只在 Run Biz。

### Phase B — 统一 ingress 回执与投影（SO-03、SO-04）

1. application scheduler contract 返回完整 typed owner result，而不是 `error` + boolean。
2. Web asynchronous activation 将 `queued`、`owner_accepted`、`owner_rejected`、`replacement_pending`、
   `reconciliation_requested` 区分为不同 projection phase；每项绑定 requestID/workspaceID/runID。
3. EventHub/remote backend 与 TUI 将真实 result 映射为 `RunInfo` 或专用 execution outcome DTO。
4. `restart_required`、`definition_changed_requires_restart`、`fresh_activation_required`、
   `governance_required` 必须可见且不触发 LLM/drive/side effect。

**门禁：** TUI/Web 可见真实 RunID 和 disposition；owner 返回拒绝时绝不显示“execution started”；
duplicate request 幂等返回同一 outcome。

### Phase C — Orchestrator 三个内部状态机隔离（SO-05）

1. 建立 producer allow-list：Drive 只接受 Orchestrator own schedule/lease path；termination 只接受
   Run owner supersede fact 或已定义的 Orchestrator-private fact。
2. SM-2 internal event 只写 authority/focus/coverage/reconciliation；SM-3 只写 candidate/dispatch；
   SM-4 只写 lease/recovery/delivery acknowledgement。
3. SM-4 需要重驱动时只提交同一 aggregate identity 的 schedule/lease；由 drive loop 读取 persisted
   SM-2/SM-3 结果，不得在 SM-4 helper 中选择 next action。
4. terminal delivery 保持 `Result -> Workspace -> Run`，每个 owner 校验 sealed identity 后再触发本地 reducer；
   delivery 失败仅形成 pending lease，不得改写 sealed terminal。

**门禁：** 伪造 drive/termination source 被拒绝；SM-4 mutation 单测证明不改变 focus/coverage/candidate；
重复/stale timer 不重放 operation；无进展只保留 bounded persisted successor，零 busy loop。

### Phase D — CWI semantic reconciliation（SO-07）

1. activation/profile 为每个 CWI 声明 precise target/selector、accepted operation/effect、独立
   recipe/check/metric/artifact proof；validation proof 一 focus 一 CWI。
2. File/Shell/MCP 仅输出 owner-attested current-workspace fact；generic exit 0、TODO test、summary、
   file/Git existence、readiness、LLM claim 均不能增加 credit。
3. `current_run` 只 retain/reopen existing credit；`workspace_full` 一律 reopen predecessor credit，
   新 Run 只能从新的 binding-matched receipt 获得 credit。

**门禁：** 删除 artifact/config/docs 后 resume 精确 reopen；generic validation 不能关闭多个异构 CWI；
已有正确 workspace 的 restart 也必须有 current-run matching validation receipt；缺任一 LSP proof 不能 complete。

### Phase E — 端到端与 deployed 验收（SO-06）

| 场景 | 断言 |
| --- | --- |
| active Run + resume | 同一 Run、`current_run` reconciliation、无 replacement；回执可见真实 disposition |
| completed old Run + restart | 创建并激活 `run-002`；Workspace slot 指向 `run-002`；首轮为 `workspace_full` |
| definition drift + resume | `definition_changed_requires_restart`，零 new Run/drive/LLM/effect |
| stale schema + restart | 旧 Run 仅释放匹配 slot；replacement 有 sealed current bootstrap |
| predecessor active + restart | successor pending、predecessor sealed supersede、terminal fact 后 successor 一次激活 |
| duplicate/stale/CAS/crash | request/generation/lease 幂等，不重放 durable operation，不丢 lineage |
| two workspaces concurrent | requestID、singleflight、lease、projection、receipt 完全按 workspace/run 隔离 |
| terminal delivery failure | Result/Workspace/Run 单边成功可重试；sealed terminal 不改写 |
| CPU guard | 反复 schedule/CAS/pending 不出现无 lease 的循环或持续高 CPU |
| LSP Bridge fresh live | 十工具、五语言、unsaved edit、bounded output、性能/并发、build/race、config/docs 各有独立 binding-matched proof |

执行顺序：先相关 package tests，再 `make fmt-check`、`make vet`、`make verify-boundary-contract`、
`go test -count=1 ./...`、`go test -race -count=1 ./...`、`make verify-ci-mvp-file`，最后执行 fresh
deployed restart/resume、双 workspace 与 LSP Bridge semantic live。任一 live 条件未完成，本计划保持 active。

## 7. 文档与清理规则

1. `docs/00-core/runtime-stage-contract.md` 和 Orchestrator readme 是运行时设计 SoT；本计划只定义
   尚未完成的收口顺序与验收。
2. 旧“R3 代码主链已完成，只剩 live proof”的表述已失效：SO-01 至 SO-05 为明确生产边界缺口，必须先关闭。
3. 不新增兼容路径、旧 aggregate migration、V1 StagePacket/NextAction、map/JSON authority bridge 或
   application continuation queue。
4. 每个 Phase 完成后同步 `active/overview.md`、`current-unclosed-matrix.md`、组件 readme 与对应测试；
   完成全部 Phase 后才可移至 `docs/90-archive/completed/`。
