# Runtime V2 单 Owner 状态机重新收口计划

Status: active — C10/C11 completed; final C8 deployed fresh-instance acceptance remains open

Type: implementation closure plan

Plan ID: `RUNTIME-SMO-R2`

Last Updated: 2026-07-30

## 1. 目的、重开依据与文档权威

本计划重新打开 Runtime V2 的单 owner 收口。它不否定历史
[`RUNTIME-SMO`](../../90-archive/completed/runtime-v2-single-owner-state-machine-closure-plan.md)
已完成的 contract 归一工作；该记录继续只作为回归基线。本计划由新的 current-schema 反例重新打开，且只处理
下列仍可复现的 owner、恢复与投影偏差：

1. Run recovery inventory 读取 `RuntimeState.Decision`、`ActiveDispatch`、delivery 记账并输出
   `active_no_dispatch` 等 SM-2/3/4 分类，越过 Run 的 SM-1 owner 边界；
2. Orchestrator persistence adapter 将泛化 `common.ErrConflict` 解释为 CAS conflict，可能将语义
   freshness/contract 拒绝送入 SM-4 re-drive；
3. 进程内 drive gate 仅合并正在执行的 drive。drive 结束后才建立的 delay timer 没有与同一
   workspace/run/revision 的既有 timer 合并，可能形成重复 recovery 链；
4. schema 不支持的 active Run 被 inventory 当作可 drive 对象。Orchestrator 既不能加载 aggregate，也不能把
   failure 写回不可加载记录，导致 Run 保持 `active`、TUI/Web 继续提供 resume；
5. projection 使用历史 candidate attempt 推断 `running`，并将任意 nonterminal Run 推断为可 resume，混淆了
   owner authority 与展示状态。
6. Orchestrator 在 Run 的常规 command subscription 前同步恢复时，错误使用 `/run/command/get`；即使读取改为
   Setup topic，若立即启动 background drive，仍会在 `/run/command/update` 注册前产生第二个 lifecycle race。
7. assessment observation 的 `MissingEvidenceKinds` 被 reducer 与 validation-only ledger 直接消费，使 LLM 的
   claimed gap 间接选择 SM-2 action。有效 validation 未关闭 coverage 且已有 owner-grounded repair surface 时，
   Runtime 会错误地重复 validation 并耗尽预算，而非进入受封存 target 约束的 state-change repair。
8. `RuntimeOrchestrator` 仍公开一组直达 controller 的 Runtime V2 mutation facade。虽然当前生产 mainline
   没有调用者，但它绕开 `RuntimeInternalEvent → RuntimeController.ApplyInternalEvent →
   RuntimeTransitionCoordinator.Commit` 的唯一入口，且原 boundary scan 显式排除了该文件。

本计划的唯一设计权威按以下顺序解释：

1. [`docs/00-core/runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)；
2. [`internal/modules/kernel/orchestrator/readme.md`](../../../internal/modules/kernel/orchestrator/readme.md)；
3. `internal/pkg/workorchruntimecontract` 的 typed DTO、enum 与纯 helper；
4. 本计划的实施顺序、组件清单、验收和删除项。

历史计划、live 日记、旧 schema fixture 不能推导新迁移，也不能以“过去已完成”为由绕过本计划的反例验证。

## 2. 目标与非目标

### 2.1 目标

将 Runtime 处理归一为一个固定模型：

```text
跨 owner immutable intent/fact/ack
  -> 接收 owner 校验 scope、causation、revision、sealed identity
  -> 接收 owner 的 private internal event
  -> 该 owner 的唯一 reducer / transition coordinator / CAS
  -> 先持久化，再发布下一条 immutable fact
```

正式状态只由唯一 owner 的 `biz` 内部 reducer 修改。外部 EventHub 输入是事实或意图，不是状态迁移命令；
internal event 不是新的跨 owner envelope。

### 2.2 非目标

- 不恢复 V1 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、自动 replan successor
  或 application continuation queue；
- 不增加通用 EventHub envelope、`map[string]any`、JSON/reflect bridge、跨 owner service/repository；
- 不做旧 Runtime aggregate 双读、双写、原地 migration 或按字段存在性 fallback；
- 不因历史 Run 不可读而自动创建 successor；只有用户明确 fresh activation/restart 才创建新 Run；
- 不将 software-development、某个模型、文件类型或 capability 专用规则写入 Orchestrator core；
- 不通过放宽 candidate/receipt 合同或增加无信息 Decision/Approval ticket 掩盖恢复问题。

## 3. 不可变规则

### 3.1 状态机与资源 owner

| 域 | 唯一 owner | 允许消费的跨 owner 输入 | 明确禁止 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | activation/continue intent、governance/terminal/incompatible fact | 外部指定 Run state；Run 选择 action 或 drive class |
| SM-2 authority/focus | `kernel/orchestrator` | prerequisite、feasibility、receipt、assessment、governance fact | capability 决定下一 action；SM-2 执行 operation |
| SM-3 candidate/sealed dispatch | `kernel/orchestrator` | provider outcome、candidate diagnostic、feasibility、operation receipt | LLM/adapter 选择 correction、retry、seal 或 terminal |
| SM-4 drive/recovery/delivery | `kernel/orchestrator` | Run opaque inventory、typed persistence/recovery fact、delivery ack | 重建 SM-2 authority；写 Run/Workspace/Result state |
| Workspace active-run/state | `blocks/workspace` | Run owner state fact、sealed terminal delivery fact | Workspace 判定 Runtime terminal 或自行 resume |
| Result lifecycle | `blocks/result` | sealed terminal delivery fact | Result 判定 evidence/coverage/terminal |
| Decision / Approval | 各自 Block | governance request、resolution intent | resolution 直接 Resume Run 或驱动 SM-4 |
| capability 技术状态 | 各 capability Block | sealed probe/operation command | 写 aggregate、focus、coverage、dispatch 或 terminal |

SM-2、SM-3、SM-4 可以在同一个 `kernel/orchestrator` 模块内，但必须共享一个 internal-event dispatcher、一个
transition coordinator 和一个 aggregate CAS 边界；它们不能通过 package 间 helper、timer 或 projection 间接
管理彼此状态。

### 3.2 合同字段分类

每个跨 owner DTO 字段必须在 code review 标明一种类别：

| 类别 | 含义 | 规则 |
| --- | --- | --- |
| canonical authority | owner reducer 真正消费的状态/identity | 持久化；仅 owner 写；不得有重复镜像 |
| derived projection | 从 canonical authority 纯派生的展示/操作文案 | 不参与 CAS/hash/transition；不得反向驱动 |
| original diagnostic | producer 提供的安全原始事实 | 完整保留，不决定 retry/terminal；只生成受控 provider-safe 投影 |

不能以压缩或覆盖 `OriginalDiagnostic` 的方式简化状态机。稳定 factual category 用于 owner reducer，原始诊断用于
审计、UI 和有界 correction；两者不可互相覆盖。

assessment 的 LLM payload 属于 original observation：其中的 disposition、summary、items、reason、claimed missing
evidence kinds、claimed target refs 与 evidence refs 必须保留，但不是 stable factual category，更不是 canonical
authority。`RuntimeRepairDirective.RequiredState` 是同一 focus 唯一可选择下一 repair action 的 persisted authority；
它只能由 Orchestrator 的 SM-2 private internal event 写入。不得为 assessment observation 新增一个并列的
action/state/repair-hint 字段，也不得用 observation 文本、文件名或 Goal/DoD 文本相似度分类。

### 3.3 scope 与幂等规则

所有跨 owner input 至少携带 `WorkspaceID`、`RunID`、causation/identity；适用时还必须携带 definition/evidence
revision、authority snapshot、dispatch、operation、execution key/fence 或 terminal delivery identity。缺少 scope 只能
拒绝，不能依赖进程“当前 workspace”补齐。

duplicate、stale、乱序、CAS 前中断、CAS 后响应丢失必须通过 persisted identity 幂等处理。进程内 map 只能是
优化 gate，不能成为恢复正确性的唯一依据。

## 4. 目标交互

### 4.1 正常 activation / resume

```text
TUI / Web / HTTP
  -> application: scoped activation or continuation intent
  -> Run owner: internal lifecycle event -> SM-1 CAS -> RunStateChanged fact
  -> Orchestrator: internal RunDriveEligible event -> SM-4 drive
  -> SM-2/SM-3: owner facts -> internal events -> aggregate CAS
  -> sealed terminal delivery fact
  -> Result owner ack -> Workspace owner ack -> Run owner ack
  -> Orchestrator: internal delivery-ack event -> SM-4 ledger CAS
```

Application、projection、TUI、Web 都不直接进入 reducer 或执行 drive。重复 resume 只由 Run owner 决定是否产生
continuation fact；Orchestrator 仅根据该事实排入自己的 owner-local drive。

### 4.2 schema 不支持的 Run

```text
Run inventory: SM-1 state + RuntimeState present + opaque ref/revision + actual schema
  -> Orchestrator loads typed record
  -> unsupported schema observation (no drive, no LLM, no operation)
  -> immutable RuntimeStateIncompatibleFact
  -> Run owner private lifecycle event
  -> fresh_activation_required administrative outcome
```

这只是明确拒绝不可读状态，不是兼容层：不得迁移旧字段、不得双读、不得将 schema 2 当作 schema 3、不得自动创建
新 Run。用户明确 restart 后，才由新的 activation 产生 current-schema bootstrap；Workspace owner 依据新的 Run
owner fact 切换 active run。

对已持久化 `fresh_activation_required` 的旧 Run，普通 `resume` 仍只返回 non-resumable fact，零 drive/LLM/operation。
显式 workspace-scoped `restart` 则采用唯一的 owner 链：application 读取 Run owner 的 typed fact 并同步转发给
Workspace `ReleaseFreshActivationRunSlotFactCommand`；Workspace reducer 仅在 `ActiveRunID == RunID` 时清空自身 slot，
空 slot 或迟到旧 fact 均 no-op；确认后 application 创建并启动新的 current-schema Run。此路径不得调用
`SupersedeRun`、Orchestrator termination、schema migration 或 successor link，因为这些操作都要求读取旧 aggregate。
旧 Run 保持 active + fresh-activation-required 的行政审计事实，不再占用 Workspace slot。

### 4.3 真实 CAS 与语义拒绝

```text
Run owner update result
  aggregate_revision_mismatch -> RuntimeCASConflictError -> SM-4 owner-local bounded retry
  semantic_rejection          -> SM-2/SM-3 typed factual path, never CAS retry
  owner_transient             -> SM-4 transient policy, never business terminal by default
```

`RuntimeCASConflictError` 必须带 expected/observed aggregate state revision。仅 repository physical CAS 失败但
aggregate revision 未变时，Run owner 应自己 reload/rebase 无关 SM-1/model 字段；它不能泄漏为 Orchestrator 的
CAS conflict。

## 5. 实施卡片

### SMO-R2-C0：先建立边界门禁与反例测试

**owner / 组件：** `kernel/orchestrator`、`blocks/run`、`workorchruntimecontract`；测试仅依赖相关 owner contract。

**实施：**

1. 为所有 Runtime V2 外部 input 建立“fact → private internal event → reducer/CAS”的测试入口；
2. 添加静态/结构测试：Run inventory 不得读取 aggregate decision/dispatch/delivery 语义，application/projection 不得
   调用 drive 或 owner repository，`pkg` 不得持有 Hub/I/O/retry/transition policy；
3. 所有新的 typed result 都标注 canonical、derived、diagnostic 分类，并保留 source owner 与 scope；
4. 将当前反例固化为最小 fixture，禁止仅凭过去 green test 将卡片关闭。

**禁止：** 新增 generic error-string router、全局 disposition、JSON bridge 或临时 “compatibility” 分支。

**完成条件：** 后续每张卡都有合法迁移、禁止迁移、duplicate/stale、restart 与 workspace 隔离测试。

### SMO-R2-C1：Run owner 只管理 SM-1，并关闭不可 drive 的 resume 入口

**组件：**

- `internal/modules/blocks/run/biz`
- `internal/modules/blocks/run/pkg/events`
- Run model/DTO、`blocks/run/readme.md`、Run lifecycle/recovery tests

**实施：**

1. 将 `QueryRuntimeV2RecoveryInventory` 收缩为 Run owner 可以权威描述的内容：SM-1 state、bootstrap/state
   presence、opaque state ref/revision、actual persisted schema、Run scope。删除其中对 Decision、ActiveDispatch、
   governance、delivery/recovery ledger 的读取和 `RuntimeV2RecoveryClass` 语义分类；
2. 引入 concrete `RuntimeStateIncompatibleFact`/结果，保留 actual/supported schema、opaque ref、scope 和安全原始
   diagnostic。Run owner 收到事实后先转换为私有 lifecycle event，再由 SM-1 reducer 形成
   `fresh_activation_required` 行政结果；
3. `ContinueRun` 对该结果返回 typed non-resumable disposition，既不发布 `RunContinuationRequested`，也不改变为
   active；
4. Run owner 保持 Runtime aggregate 不透明持久化：其 update 合同只以 aggregate state revision 作为给
   Orchestrator 的语义 fence，并对无关 Run model 写入自行 rebase；
5. terminal、governance continue、cancel/supersede 均继续由 Run owner reducer 处理。跨 owner input 只能是事实，
   不得包含目标 Run state。

**完成条件：** schema 不支持的 active Run 启动恢复和 `/run resume` 都得到 `fresh_activation_required`；零 drive、
零 LLM、零 operation、零 Decision/Approval ticket；显式 restart 才创建新 Run。

### SMO-R2-C2：Orchestrator 统一 SM-2/3/4 的输入与 mutation 边界

**组件：**

- `internal/modules/kernel/orchestrator/biz`
- `internal/modules/kernel/orchestrator/biz/runtimev2`
- `internal/modules/kernel/orchestrator/pkg/events`
- `internal/modules/kernel/orchestrator/readme.md`

**实施：**

1. Run notification、startup inventory、prerequisite、candidate request outcome、owner feasibility、operation receipt、
   governance resolution、delivery acknowledgement 和 drive failure 都先转为 `RuntimeInternalEvent`；
2. `RuntimeController.ApplyInternalEvent` 成为唯一 aggregate mutation dispatcher，
   `RuntimeTransitionCoordinator.Commit` 成为唯一 aggregate CAS 边界；
3. startup inventory 只做分页和 scope 过滤。Orchestrator load typed record 后才在 SM-4 内部区分
   `active_no_dispatch`、`active_sealed_dispatch`、`blocked_governance`、`terminal_delivery_pending`；
4. unsupported schema 是 non-drivable input。它发布 incompatibility fact 给 Run owner，不尝试提交 drive failure
   aggregate event；
5. SM-2 不执行 operation，SM-3 不选择下一个 focus/action，SM-4 不重建 authority、不写其他 owner state。所有
   action 都从 persisted canonical `ConvergenceState` 纯派生。
6. Run 在 Setup 提供 inventory、opaque `RuntimeStateRecord` query 和 commands-readiness query；Orchestrator 的
   startup inventory/load 必须只走这些窄 typed reads，禁止调用普通 `GetRunCommand`。盘点后仅保存
   owner-local、进程存活期的 recovery handoff；只有 Run 在全部普通 command 已订阅后发布
   `RuntimeV2CommandsReadyData`，才可启动 background drive、lease recovery 或发送 schema-incompatible fact。
   readiness fact 不迁移 Run state、不携带 workspace/run；handoff crash 后由下一进程重新 inventory，不能成为
   第二个 persisted state machine。

**完成条件：** 不存在从 adapter、timer、projection 或 error message 直达 reducer/CAS 的路径；restart 只从
persisted aggregate 再 drive，不重新 initialize 或增加 candidate attempt。

### SMO-R2-C3：精确 persistence 结果，消除语义冲突误入 CAS

**组件：**

- `blocks/run/biz`、`blocks/run/pkg/events`
- `kernel/orchestrator/biz/runtime_v2_state_repository.go`
- `kernel/orchestrator/biz/runtimev2/cas_conflict.go`、failure/recovery policy 与测试
- `internal/pkg/workorchruntimecontract`（仅 DTO/enum/纯 validation）

**实施：**

1. Run owner update result 明确返回 `aggregate_revision_mismatch`、`semantic_rejection`、`owner_transient`；
   semantic result 带完整安全 original diagnostic，不能退化成 `common.ErrConflict`；
2. state repository 仅在 expected/observed aggregate revision 不匹配时构造 `RuntimeCASConflictError`；删除
   `errors.Is(err, common.ErrConflict)` 的泛化包装；
3. freshness identity、definition drift、非法 transition、candidate request session conflict、owner feasibility stale
   等语义问题必须进入 SM-2/SM-3 factual reducer 路径，绝不消耗 CAS budget；
4. owner transient、delivery failure、outcome unknown 保持独立 policy，不把业务 Run 伪终态化。

**完成条件：** 语义 freshness conflict 产生零 SM-4 retry；真实 aggregate revision mismatch 才产生 CAS reload；
原始 diagnostic 在 audit/read model 中可追溯。

### SMO-R2-C4：SM-4 recovery 采用 owner-local persisted lease

**组件：** `kernel/orchestrator/biz/owner.go`、drive/recovery adapter、`biz/runtimev2` recovery ledger、相关 events/tests。

**实施：**

1. 用 aggregate 中由 Orchestrator owner 持久化的 retry schedule/lease 表达 `retry_due`；identity 至少包含
   workspace、run、aggregate revision、recovery generation、failure signature、due time；
2. 单个 workspace/run 同时只能存在一个 `scheduled | running` drive lease。进程内 singleflight 仅是该持久化
   identity 的优化，不可独立决定重试；
3. 同 revision + 同 failure signature 不得重复建立 timer；revision 未变化时 timer 触发后只验证 lease，不得 busy loop；
4. 只有 C3 定义的真实 CAS conflict 可安排 reload/re-drive。CAS ledger 耗尽停止计数增长，但保留一个退避、可观测的
   owner-local 后继；语义冲突不创建 lease；
5. `BackgroundRoutine` 只执行已获 lease 许可的 timer，不拥有 retry policy、Run lifecycle 或 authority。

**完成条件：** 两次并发 Resume、startup recovery、governance continuation 和 timer race 不会形成多条 retry 链；
同 revision 的失败不会持续耗 CPU；进程重启后 lease 能以 aggregate 身份恢复或安全失效。

**当前实现（2026-07-30）：** Runtime aggregate/Run opaque record 已升级为 current schema v4，并新增
`RuntimeDriveLease{workspace,run,aggregateRevision,recoveryGeneration,reason,failureSignature,dueAt,state}`。
Orchestrator 仅通过 owner-local internal event 将 lease 迁移为 `scheduled -> running -> released`；启动恢复将
遗留的 `running` lease 重新封存为新 generation 的 `scheduled` lease。进程内 singleflight/timer 只消费此 identity，
同 revision + reason + signature 的重复 schedule 不写 CAS、不新增 timer；旧 timer 只能安全失效。真实 CAS 仍是
唯一进入 CAS reload ledger 的来源，semantic rejection 不会构造 retry lease。C8 的全量 race、fresh deployed live 与
CPU/I/O 空闲基线已通过，见本计划的 C8 验证记录。

### SMO-R2-C5：terminal / governance 只通过 delivery fact 协作

**组件：**

- `blocks/result/biz`、`blocks/result/pkg/events`
- `blocks/workspace/biz`、`blocks/workspace/pkg/events`
- `blocks/run/biz`、`blocks/run/pkg/events`
- `blocks/decision`、`blocks/approval`、`kernel/governance`
- `kernel/orchestrator/biz`、`kernel/orchestrator/pkg/events`

**实施：**

1. Orchestrator 只密封 terminal/terminal-delivery fact 和记录 SM-4 ack；它不写 Result、Workspace、Run；
2. Result、Workspace、Run 依固定 `Result → Workspace → Run` 顺序各自将 sealed fact 转为私有事件并更新自己的
   资源；每次持久化后返回 typed acknowledgement；
3. Decision/Approval 只输出 ticket resolution fact。Orchestrator 消费已绑定 resolution 后写 governance evidence；
   Run owner 只消费 immutable continuation/governance fact，绝不接受“resume”状态命令；
4. 不兼容 schema、candidate 合同错误、projection 故障均不得降级为无信息 ticket；
5. 单边 delivery 成功只能记录 ack，不能重写 sealed terminal 或反向改变其他 owner state。

**完成条件：** 多次 delivery/restart 下 each owner 至多完成一次自身迁移；ticket 仅对应真实绑定 dependency；
Result 永不参与 acceptance 裁决。

### SMO-R2-C6：Application、TUI、Web、projection 降为意图与展示

**组件：**

- `internal/modules/application/workorchd/biz/appservice/activation*`
- `internal/modules/application/workorchd/biz/appservice/readmodel*`
- `internal/modules/application/workorchd/service/server`
- `internal/services/workorch/service/tui`
- `web/workorch-console`

**实施：**

1. Application 仅提交 scoped activation/continue intent，并读取 owner facts 返回 API disposition；不保存
   continuation queue、不执行 drive、不直接写 Run/Workspace/Result；
2. projection 只依据 owner canonical facts：`ActiveDispatchID == ""` 时不能因历史 candidate attempts 显示
   `running`；`fresh_activation_required` 显示为 restart/fresh activation，不显示 resume；
3. `RunStateActive` 不再自动等价于“可 drive”或“正在执行”。progress、recovery command、按钮状态分别使用
   explicit owner disposition；
4. TUI/Web 不得以 read-model health、SSE reconnect、历史 failure text 或本地 current workspace 反向驱动 runtime；
   操作请求一律带 workspace/run scope 和 causation。

**完成条件：** 无 active dispatch 的 Run 不显示 running；不支持 schema 的 Run 不显示为可 resume，而是给出
`/run restart`/fresh activation 指引；HTTP/TUI/Web 的同一 owner fact 显示一致。

### SMO-R2-C7：17 个 Block 与 4 个 Module 的完整边界复核

**必须改动或加入回归的组件矩阵：**

| 分类 | 组件 | 本轮动作 |
| --- | --- | --- |
| 核心 Module | `kernel/orchestrator` | C2–C5，唯一 SM-2/3/4 authority |
| 应用 Module | `application/workorchd` | C6，intent/read model only |
| 治理 Module | `kernel/governance` | C5 audit：只发布/消费治理事实，不迁移 Run |
| 系统 Module | `kernel/system` | audit：只提供启动默认值/配置，零 runtime transition |
| 资源 Block | `run` | C1、C3、C5，唯一 SM-1 owner |
| 资源 Block | `workspace`、`result` | C5，各自 owner-local terminal transition |
| 治理 Block | `decision`、`approval` | C5，resolution fact only |
| 观察 Block | `eventstream` | C6 audit：只审计/投影，零回写/drive |
| capability Block | `llm`、`file`、`git`、`shell`、`mcp`、`ssh`、`web` | 逐个 audit：只返回 concrete candidate/prerequisite/feasibility/receipt fact；零 action/retry/terminal/aggregate mutation |
| supporting Block | `memory`、`skill`、`soul`、`mcpregistry` | audit：只提供 scoped data/catalog fact；不得保存 current workspace/run 或状态迁移标记 |
| Initiator | `initiators/workorchruntime` | audit：factory/provider only，零 Hub、runtime policy、bootstrap/state owner |

每个事件驱动单元保持 `module.go → biz(Base) → service/adapter` 单向结构。`pkg` 仅保存该 owner 的 typed events、
DTO、常量和纯 helper；共享 `workorchruntimecontract` 仅保存 owner-neutral DTO、enum、clone/normalize/validate/hash
等纯函数。

#### R2 当前 22 组件范围

本表取代已归档的独立组件矩阵，避免 active 目录存在两份并列的实施标准。它只登记当前 R2 的审计范围；各卡的
具体改动和验收仍以上述 C1–C9 为准。

| 类型 | 注册组件 |
| --- | --- |
| Initiator | `internal/initiators/workorchruntime` |
| Application Module | `internal/modules/application/workorchd` |
| Kernel Module | `internal/modules/kernel/orchestrator`、`internal/modules/kernel/governance`、`internal/modules/kernel/system` |
| Resource / governance Block | `internal/modules/blocks/run`、`internal/modules/blocks/workspace`、`internal/modules/blocks/result`、`internal/modules/blocks/decision`、`internal/modules/blocks/approval`、`internal/modules/blocks/eventstream` |
| Capability Block | `internal/modules/blocks/llm`、`internal/modules/blocks/file`、`internal/modules/blocks/git`、`internal/modules/blocks/shell`、`internal/modules/blocks/mcp`、`internal/modules/blocks/ssh`、`internal/modules/blocks/web` |
| Supporting Block | `internal/modules/blocks/memory`、`internal/modules/blocks/skill`、`internal/modules/blocks/soul`、`internal/modules/blocks/mcpregistry` |

当前实施进度：C0 的 inventory boundary regression 已落地；C1 的 raw inventory、schema incompatibility
administrative outcome、non-resumable continuation，以及 explicit fresh restart 的 Run-fact → Workspace-slot-release →
current-schema activation 链已落地；C2 的 startup schema path 与 external input internal-event 化已落地；C3 的 typed
persistence result、真实 revision-mismatch CAS gate 与 semantic rejection 的完整 original diagnostic 审计链已落地；C4 的
schema-v4 persisted recovery lease、generation fence、restart recovery 与本地 timer/singleflight coalescing 已通过 race/live
验证。C5 已删除公开的
`RecordRuntimeDeliveryAcknowledgement` EventHub 命令和 Orchestrator facade；SM-4 ack 只能来自 Result、Workspace、Run
对 sealed terminal delivery command 的同步 typed response，随后经 Orchestrator private internal event 写入 ledger。
C6 已删除 Decision/Approval 成功后由 Agent CLI 自动发送第二个 `/run resume` 的路径，治理 resolution 只等待 owner
fact 链继续。C7 已为 Governance 的 ticket/approval notification 与 restart reconciliation 传递并验证 workspace
scope；无 scope 的本地 ticket/approval ID 不会发起跨 owner 读取。首次 activation/bootstrap 已改为
`activation_observed` private internal event；Run bootstrap fact 不再直接调用 controller 的 `Initialize` reducer。
2026-07-30 已补齐 Setup-phase `RuntimeStateRecord` query 与 explicit commands-ready handoff：启动恢复不再使用普通
`/run/command/get`，且在 Run 常规 command readiness 前只完成 inventory/load，不调度 drive、恢复 lease 或发送
incompatibility fact。该 process-local handoff 每次启动重建，启动/ready race 由 typed readiness query 与 immutable
ready fact 合并。C8 额外提供 Orchestrator-only `RuntimeV2DriveObservedData`，Application 仅将其投影为进程内指标；
它不持久化、不回流 drive，也不拥有另一套 recovery 状态。

**完成条件：** 所有 22 个单元均有 scope、typed result、owner mutation 与 no-cross-state-transition 的回归证据；
没有 global current workspace/run 或跨 owner repository/service 注入。

### SMO-R2-C9：assessment observation 与 repair authority 分离

**影响状态机与唯一 owner：** SM-2 是唯一修改对象，`kernel/orchestrator` 是唯一 owner。SM-3 只负责封存并交付
assessment candidate/dispatch；它不得选择下一 repair action。SM-1、SM-4、Run、Workspace、Result、Decision、Approval
以及任何 projection 不得参与本卡的状态转换。

**重新打开的可复现反例：** current-schema `run-005` 已有 current-focus 的有效、非 zero-work validation receipt，
assessment 也明确保留了未满足 coverage 的实现/测试信息和 owner-grounded target；但其 observation 写入
`MissingEvidenceKinds=[validation]`。现有 reducer 与 repair ledger 直接读取该字段，重复请求不收窄 coverage 的
validation，最终以 `validation-only repair budget exhausted` 终态。根因不是 LLM JSON、上下文裁剪或放宽候选合同，
而是将不可信 observation 提升为 action authority。

**直接修改组件：**

| 组件 | 责任与修改边界 |
| --- | --- |
| `internal/pkg/workorchruntimecontract` | 把 LLM assessment wire/observation 与 Orchestrator assessment fact 的 canonical repair 输入明确分层。可重命名/重构为 `RuntimeAssessmentObservation` 及其 claimed gap 字段；必须完整保留 summary、items、reason、evidence refs、claimed missing evidence kinds 与 target refs。pkg 仅保留 DTO、enum、normalize/validate/clone/hash 等纯函数；禁止引入 reducer、CAS、Hub、I/O、retry 或 state-action 选择。 |
| `kernel/orchestrator/biz/runtimev2` | 移除 `RuntimeAssessmentGap.MissingEvidenceKinds` 到 `need_validation` / `need_state_change` / validation-only ledger 的直接映射。assessment 只能通过 `RuntimeInternalEventAssessmentObserved` 进入 `RuntimeController.ApplyInternalEvent`；在同一 owner-local transition/CAS 内校验 observation、提交 coverage/assessment fact、基于 persisted facts 分类，并写入唯一 `RuntimeRepairDirective`。`CommitAssessment` 等具体 helper 不得成为对外可绕过 internal-event dispatcher 的 mutation 入口。 |
| `kernel/orchestrator/biz` 与 `pkg/events` | 保持外部输入为 scoped typed assessment/receipt/feasibility fact；不得增加“设置 repair state”的 EventHub command 或通用 envelope。若需要展示分类，只投影已持久化 directive 与 original observation，不能再传一个可驱动 reducer 的 action 字段。 |
| `blocks/llm` | candidate schema、decoder、renderer 与 fixture 将 assessment 明确为 observation：拒绝 `requiredState`、`runtimeAction`、retry、ticket、terminal 等 action 字段；允许的 claimed gap 字段只用于原始保真。LLM 不得通过 prompt、adapter 或 fallback 根据文本重建 repair authority。 |
| `application/workorchd/biz/appservice/activationcore` | focus compiler 只编译 evidence policy、coverage、target policy、selector 和可观测 validation recipe。它不能把 DoD/Goal 默认结果编码为 repair action，也不能写 aggregate；应提供分类所需的声明式、可封存输入。 |
| `blocks/file`、`blocks/shell` | 先作 contract audit，再按缺口补 immutable typed fact：File 只证明 target/selector、freshness、feasibility；Shell 只证明 validation 已执行、quality、zero-work 与 receipt。两者都不得返回 `need_*`、改变 repair ledger 或创建 ticket。 |

**只需保持边界并加入回归的组件：** `blocks/run`（SM-1）、`blocks/workspace`、`blocks/result`、`blocks/decision`、
`blocks/approval`、`kernel/governance`、`eventstream`、TUI、Web、`kernel/system`、`memory`、`skill`、`soul`、
`mcpregistry`、`git`、`mcp`、`ssh`、`web` 与 `initiators/workorchruntime`。它们不得因本卡获得 repair state、
current workspace/run、aggregate 写入或跨 owner lifecycle 权限。

**唯一 canonical 分类规则：** 分类器必须只读取当前持久化 aggregate 内、经 owner receipt 证明的事实；不得读取
LLM 自由文本、claimed gap 值、历史 candidate attempt、TUI/Web 状态或内存 cache。

| 条件（均为 current focus/current run） | SM-2 唯一结果 |
| --- | --- |
| 尚未存在有效、非 zero-work、quality 合格的 validation evidence | `need_validation`；此后才允许计入 validation-only ledger。 |
| 已有有效 validation，coverage 未收窄，并且封存 focus policy 要求 durable implementation/effect evidence，存在新鲜 owner-grounded repair surface | `need_state_change`；directive/closure 必须封存 coverage、baseline evidence revision、target policy 与 exact target 或 selector identity。 |
| 上述 state-change repair receipt 被接受 | 同一 repair closure 固定转换为 `need_validation`；不得从新的 assessment observation 回跳到无关 state change。 |
| repair surface 或 target snapshot 缺失、陈旧、与当前 definition/evidence revision 不匹配 | `need_prerequisite`；刷新 owner fact 后再由 SM-2 重新分类，零 provider/operation。 |
| 无合法 state-change target/selector，或封存 validation recipe 不可执行且没有可重获 prerequisite | 产生带 original diagnostic 的 typed terminal，例如 `repair_target_unavailable` 或 `validation_recipe_exhausted`；零 ticket。 |
| 存在 sealed 的真实 external/governance dependency | 仅按既有治理合同进入 `need_governance`；assessment gap、预算耗尽、候选或 evidence 合同违例本身均不构成 dependency。 |

“owner-grounded repair surface”只能是 exact focus target、仍新鲜的 owner failure target，或可被 owner 独立验证的
sealed selector/prospective constraint；File inventory 的截断展示、LLM target 文本、validation command entry 和路径名称
都不能自行成为 repair target。classification 产物不新增第二份 persisted state machine：唯一持久化 action 仍为
`Facts.RepairDirective.RequiredState`，其 closure identity 是 restart、duplicate 和 anti-replay 的唯一锚点。

**原子交接与恢复：**

```text
sealed assessment dispatch
  -> external typed observation
  -> Orchestrator RuntimeInternalEventAssessmentObserved
  -> SM-2 validates observation + receipt/coverage facts
  -> same owner-local transition commits AssessmentFact + RepairDirective
  -> persisted decision/manifest -> SM-3 only executes sealed candidate
```

该链不得发布“把状态设置为 X”的跨 owner event。CAS 前崩溃时没有 assessment/directive transition；CAS 后响应丢失时
重载同一 directive/closure 并幂等继续，不能根据 raw observation 重新挑选 action。SM-4 只能重新 drive 已持久化
directive，不能重建分类或消耗 validation-only budget。

**明确删除项：**

1. 删除 reducer 中按 `MissingEvidenceKinds` 直接返回 state-change/validation 的分支；
2. 删除 controller 中按同一字段增加 `ValidationOnlyRetries` 的分支；
3. 删除 repair closure 由 observation missing-kind 决定 `RequireValidationAfterStateChange` 的路径，改由 SM-2 的
   canonical classification/accepted repair receipt 决定；
4. 删除任何从 assessment reason、summary、target 文本、Goal/DoD 文本或 UI failure 文案推导 action 的 helper；
5. 不保留旧字段的兼容 action 读取、双读、fallback 或“未知 observation 默认 validation”分支。

**必须新增的回归：**

| 编号 | 场景 | 必须断言 |
| --- | --- | --- |
| T14 | 有效 validation 未关闭 coverage，存在新鲜 owner-grounded implementation repair surface | 即使 LLM claimed `MissingEvidenceKinds=[validation]`，SM-2 仍封存 `need_state_change`；candidate 受 repair closure/target policy 约束。 |
| T15 | LLM claimed `state_change`，但 persisted facts 仅缺可观测 validation | 只进入 `need_validation`；claimed 字段不改变 action 或预算。 |
| T16 | repair state change 成功 | 保留同一 closure，下一 action 恰为 `need_validation`；不得重新从 assessment 文本选 target。 |
| T17 | target 缺失或 stale | 只进入 `need_prerequisite`，零 LLM request、attempt、dispatch、operation、ticket；刷新后才分类。 |
| T18 | 无合法 target/selector 或 validation recipe | typed terminal 保留 original diagnostic；零 governance ticket、零 validation 循环。 |
| T19 | duplicate assessment、CAS 前崩溃、CAS 后响应丢失、fresh-process reload | directive/closure、repair ledger 与 action 完全幂等；不重复 provider/durable effect，不重新按 observation 分类。 |
| T20 | 两个并发 workspace 的同类 assessment repair | target、closure、ledger、singleflight、receipt 和 projection 不串 scope。 |
| T21 | LLM schema 与 UI/read model | action 字段严格拒绝；claimed 原始字段可追溯但只展示 canonical directive，UI 无反向 drive。 |

**完成条件：** 不存在 LLM assessment 字段到 `ConvergenceState`、repair budget、ticket、terminal 或 target 的直接数据流；
所有 repair action 都可从 `RuntimeRepairDirective` 及其 owner-grounded facts 重放；本卡通过后，live 形态必须覆盖
“有效 validation → state change repair → validation → coverage 收窄/terminal”的完整链，而不是仅证明一次候选通过。

**当前实现（2026-07-30）：** `RuntimeInternalEventAssessmentObserved` 已成为 assessment 的唯一 mutation 入口，
`CommitAssessment` 已私有化；`RuntimeAssessmentFact` 明确持久化 `ClaimedDisposition`（完整原始 observation）与
`Disposition`（仅由 SM-2 owner evidence/coverage 得出的 canonical classification）。candidate validation、commit 和
evidence-quality gate 都不再重写或以 observation 的 disposition、items、gap、refs、missing kinds、target 或 reason
拒绝/选择下一 action；SM-2 在同一 CAS transition 中更新 canonical coverage/repair ledger 并写入 directive。已删除
claimed missing-kind 对 state、validation-only 预算、target 与 closure 的直接路由；repair state-change 成功后强制使用
同一 closure 进入 fresh validation，且旧 validation shape 不得阻断该 follow-up，validation recipe 不可执行时以 typed
terminal 收敛。LLM schema 已拒绝 action 字段；read model、TUI、Web、port/client 显式展示 claimed/canonical
disposition、完整安全 observation/original diagnostic 与 persisted canonical directive，展示层没有 action 到 command/drive
的路径。T14–T21 的单元、重启、双 workspace 和投影回归已落地；C8 的 fresh two-workspace deployed live、全量
race/CPU/terminal-delivery 验收已通过。

### SMO-R2-C10：删除 Orchestrator production mutation facade

**影响状态机与唯一 owner：** 仅收紧 `kernel/orchestrator` 的 SM-2/SM-3/SM-4 mutation surface；不改变任何
reducer 规则、Run/Workspace/Result lifecycle 或 capability operation 行为。

**问题：** `RuntimeOrchestrator` 曾同时暴露初始化、prerequisite、candidate request/attempt、receipt、evidence、
governance、recovery budget 与 dispatch execution 的直接 controller wrapper。它能够让将来的 caller 不经
`RuntimeInternalEvent` 直接修改 aggregate；原 `verify-boundary-contract` 对该文件的排除又使该风险不可见。

**收口规则：**

1. production `RuntimeOrchestrator` 只保留 repository/controller 配置、scoped read（load/receipt query）、
   evidence repository read 与唯一 `ApplyRuntimeV2InternalEvent` mutation entry；bootstrap 继续构造
   `activation_observed` internal event，不能恢复直调 `Initialize`；
2. 所有测试 fixture 若需要建立中间 aggregate，必须在 `*_test.go` 中构造同一 typed internal event，不能重新
   调用 controller mutation helper；测试 helper 不参与 production build；
3. `verify-boundary-contract` 必须扫描 `runtime_orchestrator.go` 本身，并拒绝上述任意 mutation facade；
   caller scan 不得再排除该文件；
4. 不因 API 删除增加跨 owner EventHub command、generic envelope、兼容 facade 或第二个 state machine。

**合法链：**

```text
validated owner fact / bootstrap
  -> Orchestrator Biz private RuntimeInternalEvent
  -> RuntimeOrchestrator.ApplyRuntimeV2InternalEvent
  -> RuntimeController.ApplyInternalEvent
  -> RuntimeTransitionCoordinator.Commit
```

**禁止链：** `adapter/test-like facade → RuntimeController.Commit*/Record*/Seal*/Execute*`。

**完成记录（2026-07-30）：** 已删除 30 个 production mutation facade，并将仍需要的 focused owner tests 改为
test-only internal-event fixture helper。boundary verifier 已取消该文件排除并新增 facade declaration scan。
定向 Orchestrator tests 与 `make verify-boundary-contract` 已通过。此卡不宣称解决 LLM 反复选择 zero-work shell
command 的 live 收敛缺口；该缺口由 C11 单独处理。

### SMO-R2-C11：封存 owner-confirmed validation recipe，禁止 LLM 自由拼装验证命令

**影响状态机与唯一 owner：** SM-2 编译已封存 recipe contract，SM-3 只验证 recipe selection/dispatch，
Shell block 独占 discovery、preflight、execution 和 structured receipt。SM-1/SM-4、Run、Workspace、Result、
Decision、Approval、projection 均不获得 action 或 retry 权限。

**重开依据：** fresh live `run-006` 的五次 Shell `go test -run <LLM 猜测名称>` 都正确回报 `zero_work/inconclusive`；
SM-3 也正确阻止了重复 no-progress shape，最终以 `candidate_retry_exhausted` 终态。状态机没有越权，但把 command、
args 与测试名交给不可靠 provider 自由生成，使同一类无可执行检查的候选反复进入 dispatch。

**目标链：**

```text
profile/template declares declarative validation recipe
  -> activation seals recipe contract
  -> Shell owner discovers/preflights recipe against current workspace
  -> SM-2 compiles manifest from owner-confirmed recipe fact
  -> LLM selects recipeId + sealed target only
  -> Shell resolves and runs recipe; returns executed/passed/zero-work receipt
```

**不可变约束：** recipe 不可用只产生 Shell typed fact；SM-2 通过 internal event 决定
`need_prerequisite` 或 `validation_recipe_exhausted` typed terminal。LLM 不得提交 command、args、测试名、retry、
ticket 或 terminal。Shell 只报告 structured observation/original diagnostic，不能选择 Runtime action。原始 stdout
仍只按现有 redaction/bounded audit 合同保存。

**完成条件：** 同 recipe 的 zero-work receipt 至多执行一次并形成 canonical owner fact；后续 repair 只能基于
persisted directive/owner-grounded target，不能再次请求 provider 猜测测试名。无可执行 recipe 时零 ticket、零
candidate retry loop。需要 producer、decoder、manifest、Shell probe/execute、receipt、restart 和 two-workspace
scope 回归后才能关闭本卡。

**完成记录（2026-07-30）：** activation policy 现在通过显式 `recipeDeclared` 标记声明 recipe；该标记与
`recipeArgs` 分离，因此无参数可执行文件（例如 `true`）能形成明确、可验证的 recipe，而仅带
`commandName` 的观测 policy 不会被隐式提升为执行授权。未声明时携带 recipe 参数或 timeout 会在 activation
合同校验中 fail-fast；没有旧字段 fallback。Application payload mapper、默认 software-dev template 与所有
fixture 已同步该 current-schema 字段。

Shell 仅将 owner preflight-confirmed recipes 写入 admissibility snapshot；SM-2 只从该 snapshot 编译 manifest，
SM-3 只接受 `recipeId`，并在 sealed dispatch 中持久化候选实际选择的精确 recipe snapshot。Shell execute 与
feasibility 都只从该 sealed snapshot 解析 command/args；资源刷新、治理 resume 和 fresh-process reload 都不得
回退到 activation policy 或当前 inventory。无 recipe 在 provider 前以 `validation_recipe_exhausted` 收敛，零
candidate retry、ticket 或副作用。

回归已覆盖 explicit/no-arg declaration、undeclared recipe field rejection、payload mapping、default template、
strict decoder、manifest membership/hash、Shell preflight/feasibility/execute、resource refresh 后 restart 的 sealed
snapshot、file-backed persistence deep clone 与 no-recipe pre-provider terminal。C11 的 workspace/run scope 继续由
所有 prerequisite/probe/dispatch/receipt typed contracts 与 C8 two-workspace acceptance 共同验证；任何 recipe
缓存或“current workspace”状态均未引入。

### SMO-R2-C8：观测、验证与文档同步

**组件：** Orchestrator metrics/logging、Run/Workspace/Result notifications、Eventstream projection、验证脚本、
本计划及 current overview 文档。

**实施：**

1. 每个 drive/retry log/metric 必须带 workspace、run、aggregate revision、recovery generation、failure category、
   lease state 和 source；不得只记录自由文本 error；
2. 增加每 run 的 scheduled/running drive、retry timer、CAS reload、semantic reject、delivery pending 计数，便于
   CPU 异常时区分 busy loop、外部 I/O 和 projection；
3. 当前计划、`current-state`、unclosed matrix、todo index、active overview 与 module readme 对同一边界保持一致；
   历史 completed record 保持 archive，不修改为当前实施规范；
4. 任何 schema/contract 突破性修改都原子更新 producer、consumer、fixture、verifier 和文档；live 仅使用 fresh
   data，不人工修改 runtime data。
5. C9 的 observation、canonical directive、original diagnostic 三层字段必须在 TUI/Web/read model 中可区分：
   页面可完整呈现安全 observation，但只能以 persisted directive 表示下一 action，不能以 claimed gap 推导按钮、
   resume 或 drive。

**2026-07-30 验证记录：**

- `go test -race -count=1 ./...`；
- `make fmt-check`、`make console-build`、`make vet`、`go test -count=1 ./...` 与
  `make verify-boundary-contract`；
- `WORKORCH_MANAGED_SKIP_PREFLIGHT=1 WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file
  ./scripts/verify-workorch-managed-full.sh`；
- `make verify-runtime-v2-positive-convergence-file`，覆盖 CAS、lease、receipt、restart 与 terminal delivery；
- fresh managed C7 双 workspace mock-provider live，两个独立 Run 都完成，且 verifier 同时检查
  `claimedDisposition`（producer diagnostic）与 `canonicalDisposition`（SM-2 authority）；
- `make verify-runtime-idle-baseline`：两个无 active Run 的 workspace 在 30 秒基线内 CPU 0.07%、读 I/O 0、写 I/O
  4 KiB，零 projection queue 增长。该 target 不再把已删除的 event-record I/O 指标误设为 required；现存的可选指标只告警。

## 6. 测试与 live 验收矩阵

| 编号 | 场景 | 必须断言 |
| --- | --- | --- |
| T1 | current-schema fresh activation | Run owner intent → active fact → 单一 drive → 正向 evidence/terminal |
| T2 | unsupported schema active Run startup | Run owner 产生 fresh-activation-required；零 drive/LLM/operation/ticket |
| T3 | unsupported schema `/run resume` | typed non-resumable disposition；无 continuation fact/drive lease |
| T4 | explicit restart | 旧 Run 不原地迁移；新 current-schema Run 有独立 lineage；Workspace owner 切换 active run |
| T5 | semantic freshness/definition/transition conflict | 零 `RuntimeCASConflictError`、零 retry lease；original diagnostic 完整保留 |
| T6 | genuine aggregate revision race | 唯一 lease、退避 reload、revision 变化后继续；无并行 timer/CPU 空转 |
| T7 | CAS budget exhaustion | 计数有界；同 revision 无 busy loop；存在唯一可观测 owner-local 后继 |
| T8 | concurrent Start/Resume/governance/startup recovery | 每 workspace/run 最多一个 scheduled/running drive；两个 workspace 完全隔离 |
| T9 | crash at save/receipt/terminal delivery | 从 persisted identity 恢复；不重发 provider 或 durable side effect；delivery ack 幂等 |
| T10 | governance | 无真实 dependency 不建 ticket；resolution 不直接 Resume Run；正确进入 owner-local事实链 |
| T11 | TUI/Web/HTTP projection | 无 dispatch 不显示 running；fresh activation required 不提供 resume；同事实跨端一致 |
| T12 | 22-unit boundary scan | 无跨 owner repo/service、无 EventHub generic envelope/map/JSON bridge、pkg 无 Hub/I/O/state policy |
| T13 | Orchestrator 先于 Run Start 的 startup recovery | Setup 仅 inventory/opaque-record query；零普通 Get/Update/drive，Run commands-ready 后恰好释放同一 recovery action |
| T14–T21 | assessment observation / repair authority | 必须逐项满足 C9 的 direct-route、反向 claimed-gap、closure、prerequisite、terminal、restart、multi-workspace 和 UI/schema 断言；不得用单点 fixture 代替完整链。 |

最少执行顺序：先运行受影响 package 的 unit/transition/restart/race tests，再运行 `make fmt-check`、`make vet`、
`go test -count=1 ./...`、`make verify-boundary-contract`、`make verify-ci-mvp-file`。fresh live 验证至少使用两个
并发 workspace，并在 activation、semantic conflict、真实 CAS、restart、terminal delivery、TUI/Web 展示六类场景留下
可追溯证据。

## 7. 实施顺序与关闭门禁

1. **C0**：先锁定契约和反例；未通过不得修改 retry 或投影；
2. **C1 + C2**：先让 schema 不支持和恢复 inventory 走正确 owner 链，消除错误 drive 来源；
3. **C9**：先完成 assessment observation 与 canonical repair directive 分离；未证明原始诊断不选 action 前，
   不得以提高 validation budget、放宽 candidate 或创建 ticket 规避 live 反例；
4. **C3 + C4**：再拆 persistence conflict 和 retry lease，消除 livelock/CPU 风险；
5. **C5**：统一 terminal/governance fact saga；
6. **C6 + C7**：删除 UI/projection 和 capability 的隐式状态权威；
7. **C8**：全量 boundary、race、restart、fresh two-workspace live 验证及文档同步。

每次提交必须写明：影响的 SM、唯一 owner、外部 fact、对应 internal event、canonical/derived/diagnostic 字段、
合法与禁止迁移、重复/陈旧/重启行为、测试命令。禁止跨卡临时兼容、半更新 producer/consumer、以提高 budget、
重复 resume 或放宽 decoder 使 live 看似通过。

本计划只有在 T1–T21 全部通过、所有 22 单元复核完成、两个 fresh workspace 可持续正向推进到 complete、typed
business terminal 或真实 governed wait，且不存在 `active + nonterminal + 无 owner-local 后继`、合同违例循环或
无 revision 进展 CPU 空转时，才可移入 `90-archive/completed/`。
