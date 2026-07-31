# Runtime V2 CWI Reconciliation 与单 Owner 收口计划

Status: active

Type: closure-plan

Last Updated: 2026-07-31

## 1. 目的与结论

本计划关闭最新 deployed live 发现的 false-completion 缺口：Runtime V2 已能把
`CWI -> sealed binding -> sealed operation -> owner receipt -> SM-2 coverage` 串成单一证据链，
但 activation 编译的 proof 仍可能过粗。结果是一个空的 `TODO` 测试和泛化的
`go test ./...` receipt 可以匹配多个异构 CWI，尽管 workspace 缺少声明的工具、配置、文档或真实行为。

收口目标不是让 Runtime core 解析 Goal 文本、源代码或特定技术栈，而是增加一个 **CWI
reconciliation** 前置收敛阶段：profile/template 为每个 CWI 声明可观察 proof，capability owner
返回当前 workspace 的 typed fact，只有 Orchestrator 的 SM-2 reducer 才能保留或重开 coverage。

`/run resume` 与 `/run restart` 都必须执行 reconciliation；差别只在 scope：

| 命令 | reconciliation scope | Run 语义 |
| --- | --- | --- |
| `/run resume` | current non-terminal Run 的 sealed Definition、全部 selected CWI、sealed binding | 复用同一 Run；只允许在该 Run 的 SM-2 中重开或保留 CWI |
| `/run restart` | workspace 当前选定 Goal/DoD/profile 编译出的完整 CWI/binding 集 | 创建 replacement Run；不继承 predecessor 的 coverage credit |

本计划是唯一的 active Runtime V2 收口队列。SMO-R2-C12 等 archive record 继续作为 receipt/binding
回归基线，但不能据此宣称 semantic acceptance 已完成。

## 当前收口状态（2026-07-31）

代码已完成并通过定向回归：activation 的 per-CWI reconciliation contract、`current_run`/`workspace_full`
scope、request generation/history、duplicate/stale/CAS 幂等、owner observation batch 一对一 receipt 约束，以及
File/Shell/Git/MCP 的不足证据分类。TUI/Web/application 现只投递 workspace intent，Workspace/Run/Orchestrator
通过各自 internal event 迁移状态。Application 与 owner adapter 的同步 EventHub 回执读取已统一使用独立的
`*cd.Error` 变量，避免把 `Result.Get()` 的 nil 指针装箱为非 nil `error`；workspace activation/restart 的
typed intent 回执和非法 JSON body 均有回归覆盖。

截至 2026-07-31，本轮已通过 `make fmt-check`、`make vet`、`make verify-boundary-contract`、
`go test -count=1 ./...`、`go test -race -count=1 ./...`，以及隔离 managed
`runtime-v2-managed`（含 `verify-llm`、`verify-mcp-stdio`、`verify-ci-mvp-file`）门禁。
本轮新增的隔离 `loadtest-governed`（2 个并发 workspace）也已通过：每个 Run 均完成
`acceptance.operations`、MCP owner receipt、validation evidence、SM-2 convergence 与 accepted Result
核对；验证器已改为读取当前 Runtime V2 acceptance/taskProgress 投影，不再依赖已删除的旧 step 字段。
本计划仍保持 `active`，剩余门禁不是再造状态机或兼容路径，而是：

1. fresh-process restart/reload 的完整 R3 矩阵；
2. managed deployed `resume`/`restart` scope、generation/history、删除 artifact 后 reopen 和并发 workspace live；
3. LSP Bridge profile 为十工具、五语言、未保存编辑、性能/并发、config/docs、build/race 等每个异构 CWI 提供
   独立真实 proof，并证明缺 proof 时不会 terminal complete。

在上述证据完成前不得将本计划移动到 `90-archive`，也不得把定向测试绿灯等同于 live semantic acceptance。

## 2. 适用约束

本计划覆盖当前注册的全部 22 个运行单元：
`internal/initiators/workorchruntime`、`internal/modules/application/workorchd`、
`internal/modules/kernel/governance`、`internal/modules/kernel/orchestrator`、
`internal/modules/kernel/system`、`internal/modules/blocks/approval`、
`internal/modules/blocks/decision`、`internal/modules/blocks/eventstream`、
`internal/modules/blocks/file`、`internal/modules/blocks/git`、`internal/modules/blocks/llm`、
`internal/modules/blocks/mcp`、`internal/modules/blocks/mcpregistry`、
`internal/modules/blocks/memory`、`internal/modules/blocks/result`、`internal/modules/blocks/run`、
`internal/modules/blocks/shell`、`internal/modules/blocks/skill`、`internal/modules/blocks/soul`、
`internal/modules/blocks/ssh`、`internal/modules/blocks/web`、`internal/modules/blocks/workspace`。

### 2.1 状态机与 owner

| 状态机 / 资源 | 唯一 owner | 允许的状态变更 |
| --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | 仅 Run Biz 将外部 intent/fact 归一为 Run internal event 后调用其 reducer |
| Workspace active-run / slot | `blocks/workspace` | 仅 Workspace Biz internal event 修改 `ActiveRunID` 或 workspace lifecycle |
| SM-2 authority、focus、coverage、reconciliation | `kernel/orchestrator` | 仅 Orchestrator internal event 经 `RuntimeController.ApplyInternalEvent` 与 transition coordinator CAS 修改 aggregate |
| SM-3 candidate / sealed dispatch | `kernel/orchestrator` | 同上；只接受 SM-2 已提交的 decision |
| SM-4 drive / recovery / delivery ledger | `kernel/orchestrator` | 同上；不得重建 SM-2 authority 或写其他 owner state |
| Result lifecycle | `blocks/result` | 仅 Result Biz internal event 消费 sealed terminal delivery fact |

跨 owner EventHub 合同只能传 immutable `Intent`、已持久化 `Fact`、`Observation`、`Receipt`
或 delivery acknowledgement。它们不得包含“把接收方状态设置为 X”的 target state。每个接收
Biz 必须先校验 workspace/run/definition/binding identity，再转为该 owner 的 internal event。

### 2.2 绝对禁止项

- TUI、Web、remote backend、SSE/projection、application scheduler、LLM、Decision/Approval 不得写
  Run、Workspace、Result、coverage、focus、dispatch 或 recovery state。
- Application 不得再通过 `SupersedeRun`、`StartRun`、`ContinueRun` 等 helper 选择生命周期迁移；它只做
  request adaptation、typed bootstrap/reconciliation contract 编译和 EventHub intent 投递。
- capability owner 只返回 observation/receipt；不得重开 CWI、选择 repair、请求 ticket 或终态化 Run。
- `internal/pkg/workorchruntimecontract` 只保存 DTO、enum、clone/normalize/validate/hash 等纯函数；不得含
  EventHub、CAS、I/O、retry、状态迁移或 policy decision。
- 不新增兼容 fallback、旧 aggregate migration、V1 路径、JSON/map/reflect bridge 或通过 error text 选择路径。
  缺少新 schema reconciliation contract 的 Run 必须走 `fresh_activation_required`，由显式 restart 创建新 Run。
- raw Goal/DoD、文件内容、provider body、credential 或 endpoint 不得作为 reconciliation 的持久化 evidence；
  只保存 binding identity、owner observation、hash/ref、结构化 check/metric/artifact fact 与安全 original diagnostic。

### 2.3 全局 scope 纪律

所有 CWI reconciliation command、observation、receipt、lease 和 read-model item 都必须携带
`WorkspaceID`；Run 范围还必须携带 `RunID`。入口先用 checked workspace context binder 绑定 scope；已有
scope 不匹配立即 conflict。不得在 Biz、Service、Module、全局变量、共享 cache 或后台任务保存未受 scope
约束的“当前 workspace”。singleflight、lease、idempotency key、observation generation 均使用
`workspaceID/runID` 复合 identity。

## 3. 目标运行流程

```text
TUI / Web / remote
  -> WorkspaceExecutionIntent{resume | restart, requestID, workspace scope}
  -> Workspace owner internal event
       validates active-run slot; emits immutable ActivationGrant / SelectedRun fact
  -> Run owner internal event
       resume: selects same non-terminal Run
       restart: creates replacement Run and owns predecessor supersede
       emits RunStateChanged + ReconciliationRequested fact
  -> Orchestrator SM-4 schedules the persisted reconciliation request
  -> capability owners return typed current-workspace observations
  -> Orchestrator internal event -> SM-2 reconciliation commit
       retain valid credit | reopen invalid credit | prerequisite/typed terminal
  -> ordinary SM-2 decision -> SM-3 candidate/dispatch -> receipts/evidence
  -> SM-2 terminal -> Result -> Workspace -> Run owner-local delivery
```

Reconciliation 是 SM-2 的前置 input，不是第五个状态机，也不是 Application/Run 的 repair loop。
SM-4 只负责使用既有 run-scoped lease/singleflight 投递 observation query 和重驱动；它不解释 observation
或选择 CWI outcome。

## 4. 统一合同

### 4.1 新增纯 DTO

`internal/pkg/workorchruntimecontract` 新增下列纯值合同（命名可在实现时微调，语义不得拆分）：

| 合同 | 必填字段与用途 |
| --- | --- |
| `RuntimeCWIReconciliationScope` | `current_run`、`workspace_full`；无第三种临时 scope |
| `RuntimeCWIReconciliationRequest` | workspace/run、request ID、scope、definition revision、binding-set hash、ordered CWI/binding refs、workspace baseline revision/ref |
| `RuntimeCWIReconciliationObservation` | request/binding/CWI identity、owner module、observed revision、`satisfied/unsatisfied/unverifiable/stale` factual category、target/check/metric/artifact facts、safe original diagnostic |
| `RuntimeCWIReconciliationBatch` | 完整 observation set/hash、owner/query receipts、generation；不含 coverage target state |
| `RuntimeCWIReconciliationResult` | Orchestrator-private SM-2 internal result：retain/reopen/prerequisite/terminal 的事实输入；不得作为跨 owner command |

`Request` 和 `Observation` 的 validation 必须拒绝缺 scope、definition/binding hash 漂移、未知 CWI/binding、
owner 不匹配、重复或跨 workspace ref。稳定 hash 只覆盖 normalized contractual字段，不覆盖时间、lease、
provider/model、projection revision 或原始文本。

### 4.2 coverage 的严格规则

1. reconciliation observation **不能直接新增 CWI credit**。它只能证明已存 credit 仍可保留，或要求 SM-2
   重开 CWI。
2. full restart 不继承 predecessor evidence。若 workspace 已有正确实现，必须由新 Run 执行 binding 明确允许的
   current-run read-only validation/inspection operation，并取得 matching receipt，才能关闭对应 CWI。
3. 若 binding 只接受 durable state-change proof，则已有文件/commit/manifest 不能作为新 Run credit；CWI 保持 open，
   除非 profile 明确提供现存状态的 validation proof。
4. 一个 operation 可引用多个 binding，但 owner receipt 必须逐个满足每个 binding 的 exact
   target/selector、effect、recipe、check、metric/artifact。泛化 command success、exit 0、`PassedChecks`、summary、
   assessment claim、文件/Git 存在都不得扇出关闭异构 CWI。
5. 当前 Runtime V2 收口采用保守的 activation 形状：validation proof 一个 focus 只能绑定一个 CWI。
   共享 recipe 允许复用，但每个 CWI 必须声明自己的 `RequiredCheckRef`/target，从而编译成独立 focus/binding；
   wildcard 或 focus 级泛化 recipe/check 在 activation 前失败为 `coverage_binding_unavailable`，不得让一个 receipt
   扇出关闭多个异构 CWI。后续若需要同一 dispatch 承载多个 CWI，必须先扩展 typed per-CWI proof 映射并补齐 owner
   receipt 回归，不能放宽当前校验。
6. 旧 evidence/receipt 不删除；SM-2 用 `reopened` lineage 和 exact reconciliation observation 记录为何撤销
   current credit。只有新的 binding-matched receipt 才能再次满足该 CWI。

### 4.3 profile/template 的职责

profile/template 是 Goal/DoD 语义到 proof 声明的唯一翻译层。每个 CWI 必须声明：

- precise artifact/target 或 owner-verifiable prospective selector；
- accepted operation kind/variant/effect；
- 可在当前 workspace 执行的 validation/inspection recipe；
- CWI-specific check refs，适用时的 metric/artifact constraints；
- 对已有交付物可用的 read-only validation proof，或明确要求新的 durable state change。

不允许把 managed internal config 当作 deliverable artifact。若 DoD 要求用户可交付配置/文档，profile 必须声明
deliverable-directory 中的具体 artifact binding；`workspace.InternalDirectory` 的 managed config 只能作为 capability
运行依赖。

## 5. 命令语义与状态边界

### 5.1 `/run resume`

1. TUI/Web 发送 `WorkspaceExecutionIntent{scope=current_run}`，不携带 state target、LLM override、definition patch
   或 CWI result。
2. Workspace owner 从自己的 active-run slot 选择唯一 Run，发出 immutable selected-run fact；没有 active
   non-terminal Run、Run terminal 或 `fresh_activation_required` 时返回 `restart_required`，零 drive/LLM/operation。
3. Run owner 校验 Run 的 sealed Definition 与 Workspace 当前选定 Goal/DoD 的 identity/revision；任一漂移返回
   `definition_changed_requires_restart`，不隐式创建 replacement Run。
4. Run owner接受后只产生 owner-local continuation/reconciliation request；Run lifecycle 不因普通 resume
   被外部直接改写。blocked Run 仍只能由匹配 governance resolution 进入 active。
5. Orchestrator 对当前 Run 的全部 selected CWI 执行 reconciliation，再依据 SM-2 结果继续已有 aggregate。

### 5.2 `/run restart`

1. TUI/Web 发送 `WorkspaceExecutionIntent{scope=workspace_full}`，不得携带 `RunID`、state target 或旧 coverage。
2. Application 只读取当前 owner snapshots 并无状态编译 latest Goal/DoD/profile 的 typed bootstrap 与 complete
   binding set；它不启动、continue、supersede 或写 slot。
3. Workspace owner internal event 串行授权同一 workspace 的 replacement activation；Run owner internal event
   决定 predecessor 的 supersede/new Run creation 并持久化新的 bootstrap。Workspace 只在消费 Run owner fact 后
   更新自己的 `ActiveRunID`。
4. 新 Run 的首个 Orchestrator drive 必须是 `workspace_full` reconciliation。它先验证所有 binding 所声明的
   当前 artifact/check，再选择 open CWI 的 prerequisite/repair/candidate；不得复制旧 Run focus、snapshot、
   dispatch、receipt、repair ledger 或 coverage credit。

## 6. 分组件收口清单

| ID | owner / 组件 | 必须完成的收口 | 禁止遗留 |
| --- | --- | --- | --- |
| R3-01 | `docs/00-core`、flow、module readme、active 索引 | 将本计划的 owner map、两种 scope、reconciliation 不新增状态机、no-compat 规则写入权威文档 | 与 archive C12 record 并列为活动约束；“当前无 Runtime 缺口”的过期表述 |
| R3-02 | `internal/pkg/workorchruntimecontract` | 添加第 4.1 节 DTO/enum/hash/validation/clone；为 schema bump 提供纯 validation | reducer、CAS、EventHub、I/O、retry、profile-specific 分支 |
| R3-03 | `application/workorchd` activation compiler + defaults/templates | 编译 complete per-CWI reconciliation contract；LSP Bridge 模板为十工具、五语言、未保存编辑、性能、并发、config、docs、build/race 声明独立 proof/check | 仅以 focus 级泛化 `go test` 复制给多个异构 CWI；internal managed config 充当 deliverable |
| R3-04 | TUI、Web、remote/eventhub backend | 只发送 typed workspace intent、显示 owner projection 的 reconciliation phase/diagnostic；按 request ID 幂等 | 本地 run/CWI 状态推断、自动 restart、直接 Resume/Start state command |
| R3-05 | `application/workorchd/service/server` 与 appservice | 变为窄 ingress/bootstrap compiler；向 Workspace/Run owner 发布 typed intent/fact；同步 EventHub 回执必须保留具体 `*cd.Error` 语义，非法请求体统一归类为 `ErrInvalidArgument` | scheduler 选择 lifecycle path、调用 `SupersedeRun`/`StartRun`/`ContinueRun`、保存 reconciliation state；把 `Result.Get()` 的 nil error 装箱为失败 |
| R3-06 | `blocks/workspace` | owner-local activation slot selection/authorization；仅由 Run owner state fact 更新 ActiveRunID；workspace-scoped idempotency | 读取/修改 Run repository、解释 CWI、写 Runtime aggregate |
| R3-07 | `blocks/run` | owner-local resume/restart/activation dispatcher；SM-1 定义 resume/restart-required/definition-drift/replacement 结果并发布 immutable facts | 外部 EventHub handler 直接调用 reducer；Run 判断 coverage 或 drive action |
| R3-08 | `kernel/orchestrator` | 将 reconciliation request/owner results映射为 `RuntimeInternalEvent`；SM-2 原子 retain/reopen/prerequisite/terminal；SM-4 复用 bounded lease/singleflight | 新的跨组件 reconciliation state owner；SM-4/SM-3/LLM 决定 CWI outcome；observation 直接加 credit |
| R3-09 | `blocks/file` | 提供 binding-grounded artifact/tree/target/selector observation，含 observed revision 与安全诊断 | 从 Goal/DoD 推断业务语义、写 CWI 或 aggregate |
| R3-10 | `blocks/shell` | 执行 profile-declared inspection/validation recipe，回执 CWI-specific check/metric/artifact observation；泛化成功明确标为 insufficient | exit 0/summary 自动表示任何 CWI satisfied |
| R3-11 | `blocks/mcp` | 对声明的 tool surface/behavior 返回 structured observation；区分 endpoint readiness 与 behavior proof | 连接成功、描述文本或 LLM claim 作为功能覆盖 |
| R3-12 | `blocks/git`、`blocks/llm`、Decision/Approval | Git 仅 revision fact；LLM 仅 candidate；治理只处理真实 external dependency | 用 commit/LLM/decision 为 CWI 语义兜底 |
| R3-13 | `blocks/result`、eventstream、readmodelprojection、TUI/Web views | 投影 reconciliation request/result、reopened CWI、binding/check refs 与安全 original diagnostic；terminal 仍只消费 sealed fact | 投影反向重开/关闭 CWI、重新投递 drive 或 lifecycle command |

## 7. 实施顺序与验收门禁

### Phase A — 合同与入口（R3-01 至 R3-07）

1. 先落权威文档与 DTO/schema，删除或迁移冲突的 active 叙述。
2. 将 TUI/Web/backend/server 改成 intent-only；取消 resume 的隐式 replacement/new Run。
3. 将 Workspace 与 Run 的外部 handler 改为 `external command -> owner internal event -> owner reducer`。
4. 证明同 workspace 的 duplicate request、不同 workspace 的并发 request、terminal、blocked、fresh schema、
   definition drift 都没有跨 owner 写入和错误 scope 复用。

**门禁：** `go test` 覆盖 owner command producer/consumer、非法 target state 拒绝、workspace scope conflict、
restart 无 RunID、resume 不创建 replacement、schema old-run `fresh_activation_required`。通过前不得接入
reconciliation capability query。

### Phase B — profile 与 factual observation（R3-03、R3-09 至 R3-12）

1. 重写模板/profile proof 映射，强制 heterogeneous CWI 的独立 `RequiredCheckRef`；activation 拒绝共享
   validation proof 的多 CWI focus，确保 generic receipt 不会扇出。
2. File/Shell/MCP 实现 read-only observation；各 owner 做完整 binding identity、target/selector、recipe 与
workspace revision preflight。
3. 缺 recipe/check/owner、截断 inventory、unknown target、raw generic success 都返回 typed `unverifiable`/
`prerequisite_required`/`stale` fact，零 CWI credit。

**门禁：** 空 TODO 测试、缺 docs/config、缺十工具、缺五语言 validation、缺 race/metric proof 都不能关闭
对应 CWI；一条 generic receipt 不得关闭四条异构 CWI；真实拥有独立 checks/targets 的 CWI 必须编译为独立
focus/binding 后逐项通过。

### Phase C — Orchestrator convergence（R3-08）

1. 新 Run 初始化先封存 `workspace_full` reconciliation request；current Run resume 先封存
`current_run` request。
2. SM-4 只调度 owner query；所有结果进入 Orchestrator internal event。
3. SM-2 一次 CAS 同时提交 observation lineage、保留/重开 coverage、repair directive/next prerequisite；
   不允许 event handler、projection 或 capability callback 直接写 aggregate。
4. 收到 external workspace revision 变化、stale observation、duplicate observation、CAS crash/restart 时，从
   persisted request/batch/lease 恢复，不能重放 operation 或把 old credit 静默保留。

**门禁：** observation 无法新增 credit；reopened lineage 不丢失 old receipt；仅 matching new current-run
receipt 能再次关闭；每个非等待 transition 递增 StateRevision；无进展时有 persisted bounded successor 或 typed
failure，零 CPU busy loop。

### Phase D — 投影、回归与 live（R3-13）

1. 投影显示 scope、request generation、per-CWI observation、保留/重开原因、binding/check refs；不显示
误导性的“completed”直到 sealed terminal delivery。
2. 更新 TUI/Web 文案：resume 只继续 current Run，restart 才创建全 workspace activation；definition drift
明确提示 restart。
3. 执行 package、边界、race、deployed live 与 fresh-process restart 验证。

**必测矩阵：**

| 场景 | 断言 |
| --- | --- |
| current active Run + unchanged workspace + resume | 同 Run、同 sealed binding、只产生 reconciliation observation、无 replacement/LLM/side effect（若全部已满足） |
| current active Run + 删除 artifact/文档/配置 + resume | 同 Run，SM-2 将精确 CWI reopened；旧 receipt 仅保留 lineage |
| Goal/DoD/profile revision drift + resume | `definition_changed_requires_restart`，零 Run replacement/drive/LLM/effect |
| restart + 已有正确 workspace | 新 Run/full scope；仅新 Run matching validation receipt 才重新取得 credit |
| restart + 空 TODO test / generic go test | 对应 semantic CWI 不得关闭，零 false terminal |
| restart + true per-CWI LSP fixture | 新 Run 取得独立 proof 后完成；Result -> Workspace -> Run delivery 全部 ack |
| duplicate/stale/CAS crash/restart | query/commit 幂等；不重放 durable operation，不丢 reopened lineage |
| two workspaces concurrently resume/restart | 所有 request/lease/cache/projection 由 workspace/run scope 隔离 |

最终执行：相关 package tests、`make fmt-check`、`make verify-boundary-contract`、`make vet`、
`go test -count=1 ./...`、`go test -race -count=1 ./...`、隔离 managed `verify-ci-mvp-file`，以及至少一轮
fresh deployed LSP Bridge full-scope restart live。live 验收必须证明真实文件、配置、文档、独立 tests 和
binding-matched receipt；仅 Runtime terminal/绿灯不构成通过。

## 8. 完成定义

本计划仅在以下条件同时满足时关闭并归档：

1. resume/restart 的 scope、definition drift、replacement 语义与第 5 节一致；无隐式 Run replacement。
2. 所有正式状态迁移均可追踪到唯一 owner 的 internal event/reducer；扫描不存在跨 owner repository/service、
target-state command、application lifecycle decision 或 projection authority。
3. current-run/full-workspace reconciliation 在持久化、duplicate、stale、CAS/restart 和并发 workspace 下均
可恢复且无 side effect replay/CPU busy loop。
4. 每个 CWI 的 close/reopen 都能追溯到 binding、current owner observation、sealed operation 与 receipt；
generic validation、TODO test、summary/claim/exit code、文件/Git 存在不能越权。
5. LSP Bridge fresh live 同时证明：十工具、五语言、unsaved edit、bounded output、operability/performance/concurrency、
build/race、config、docs 和 artifact delivery 均有声明的逐项 proof；缺任一项时 Run 不得 terminal complete。

## 9. 文档同步

实施每个 Phase 时同步更新：

- `docs/00-core/runtime-stage-contract.md`（完成后的中立 owner/reconciliation 不变量）；
- `docs/20-runtime-flow/activation-definition-flow.md`、runtime flow overview；
- `internal/modules/kernel/orchestrator/readme.md` 与 affected block readme；
- `docs/01-overview/current-state.md`、`current-unclosed-matrix.md`、`unfinished-todo-index.md`；
- 本 active plan 的状态和验收链接。

完成后将本文件移至 `docs/90-archive/completed/`；archive 只保留回归证据，不继续定义生产实施顺序。
