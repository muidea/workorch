# WorkOrch 22-Component Framework Boundary Remediation

Status: active

Type: remediation plan

Last Updated: 2026-07-19

## Implementation Progress

Completed in the current worktree (verification remains subject to the final
matrix):

- GMM-R1: Run recovery inventory is Setup-ready; Orchestrator fails startup on
  inventory acquisition failure and schedules idempotent SM-4 re-drives only
  after a successful synchronous inventory.
- GMM-R2: server lifecycle uses a bounded stopped notification and process-local
  wait state; EventHub no longer carries a channel.
- GMM-R3: all 11 owner `Module.Service()` facades and pure owner `service/`
  forwarding packages are removed; module handlers call their own `biz`.
- GMM-R4: Memory, Eventstream, Governance, and Workspace event contracts use
  owner DTOs; `pkg/events` has no repository import.
- GMM-R5: testkit runtime roots and Git receipts are explicit temporary roots;
  known source-tree residues and empty historical packages were removed.
- GMM-R8 topology migration: the shared `internal/modules/base/biz` now backs
  every framework Module/Block that uses EventHub. Module roots delegate only
  lifecycle to their component-local Biz; Orchestrator retains the same Runtime
  V2 authority inside `kernel/orchestrator/biz.Biz`.

GMM-R6 inventory and the pre-existing GMM-R7 documentation edits are present
in this worktree. A follow-up static audit on 2026-07-19 found that the former
module-root topology retained EventHub directly and there was no shared
`internal/modules/base/biz`. GMM-R8 source migration is now complete; this
plan remains active only for the externally supplied deployed restart-fixture
matrix described below.

Repository verification recorded on 2026-07-19:

- structural gates, `make fmt-check`, `go vet ./...`, and `git diff --check`;
- all 174 Go packages, executed serially as individual `go test -count=1`
  invocations (the ordinary multi-package command did not reliably expose
  complete package output in this environment);
- Run and Orchestrator targeted `-race` suites;
- `make verify-runtime-v2-positive-convergence-file`, `make build`, and the
  two-iteration `make verify-managed-service-smoke` lifecycle check.

The recorded commands above predate GMM-R8 and are only regression evidence
for the then-current topology. Fresh post-migration repository verification
was completed on 2026-07-19: `go test -p 1 ./... -count=1`, `make
release-check`, `make build`, `make verify-runtime-v2-positive-convergence-file`,
and the two-iteration `make verify-managed-service-smoke` all completed.
No deployed restart fixture manifest or target service is recorded in this
repository, so the external SM-4 restart matrix remains open.

## Purpose

本方案用于收口 2026-07-18 基于 `go-multi-module-dev` 对 WorkOrch 22 个
framework 运行组件进行只读核对后确认的真实遗留项。目标不是重新命名
`kernel` / `blocks`、拆分 `go.mod`，也不是按文件数量机械拆包，而是确保：

- framework lifecycle 不丢失 Runtime V2 restart recovery；
- EventHub command/data/result 只传有界具体数据，不传 channel 或 owner 实现；
- module/block 不再暴露可绕过 EventHub 的 owner service facade；
- owner event contract 不再把 persistence repository 类型作为跨组件 API；
- 测试运行数据全部落在显式临时目录，源码树保持可重复、可审计；
- 22 个组件的职责分类、入口加载、状态权威、交互合同和直接功能测试一致。

本方案是当前真实未关项。Runtime 核心语义仍以
[runtime-stage-contract.md](../../00-core/runtime-stage-contract.md) 为唯一设计
SoT，Orchestrator 实现仍以
`internal/modules/kernel/orchestrator/readme.md` 与
`internal/pkg/workorchruntimecontract` 为准。本方案只约束组件边界、生命周期
交付和内部合同收口，不创建第二份 Runtime V2 状态语义。

## Audit Baseline

### Component Inventory

当前入口显式加载 22 个运行组件：

| 分类 | 组件 |
| --- | --- |
| Initiator | `internal/initiators/workorchruntime` |
| Application module | `internal/modules/application/workorchd` |
| Kernel module / owner | `eventstream`、`governance`、`orchestrator`、`run`、`system` |
| Stateful block | `workspace`、`result`、`decision`、`approval`、`memory`、`skill`、`soul`、`mcpregistry` |
| Capability block | `file`、`git`、`llm`、`mcp`、`shell`、`ssh`、`web` |

当前分类继续保留：

- `workorchruntime` 已经是单一 repository provider initiator；不重新扩展为
  EventHub/runtime-policy/container initiator。
- `kernel/run` 虽然是正式状态 owner，仍保留在既有 kernel 分组；目录分组是
  WorkOrch 现有语义，不以此次整改为由迁移到 `blocks`。
- `eventstream` 聚合多个 owner notification 并维护审计协调状态，继续作为
  kernel module；它不是 owner recovery authority。
- 8 个 stateful block 与 7 个 capability block 的职责边界成立，不新增第 23
  个运行组件。

### Verified Positive Facts

只读核对已确认：

- `application/workorch/main.go` 显式 side-effect import 全部 22 个组件；
- 生产包未发现跨 owner 导入其他组件 `biz` / `service`；
- application、HTTP 与 process service 未直接获取 owner repository；
- 组件 EventHub 合同未发现 `map[string]any`、`[]any`、通用 Response 或
  JSON/reflect 分发桥；
- HTTP route 位于 `workorchd/service/server`，不存在全局 `/**` 业务兜底；
- 长期 recovery/governance/server task 主要使用 framework
  `task.BackgroundRoutine`；
- 22 个组件及 `internal/services/workorch` 的测试通过；
- `make verify-boundary-contract`、相关范围 `go vet` 通过；
- fresh deployed Runtime V2 bounded terminal 与 positive acceptance 已通过。

这些正向事实是整改必须保持的回归基线，不得为了移除结构债务而破坏现有
Runtime V2 功能。

### Confirmed Gaps

| ID | 优先级 | 当前偏差 | 影响组件 |
| --- | --- | --- | --- |
| GMM-R1 | P1 | Orchestrator `Weight=30`、Run `Weight=40`；Orchestrator 在 Run owner `Run()` 订阅 recovery inventory 前可能启动一次性异步 recovery，失败后无重试 | `kernel/orchestrator`、`kernel/run` |
| GMM-R2 | P1 | `DoneCommandResult` 通过 EventHub 返回 `<-chan error`，泄漏进程生命周期资源句柄 | `application/workorchd`、`internal/services/workorch` |
| GMM-R3 | P2 | 11 个 module 暴露 `Module.Service()` owner facade；当前生产无调用，但公开表面允许未来绕过 EventHub | `eventstream`、`orchestrator`、`run`、8 个 stateful block |
| GMM-R4 | P2 | 4 个 owner event contract 直接使用 `internal/pkg/repository` query/entity 类型 | `memory`、`workspace`、`eventstream`、`governance` |
| GMM-R5 | P3 | 测试在源码 package cwd 下生成 `.workorch-data` / `.workorch-testkit-git-receipts`；存在空的历史 package 目录 | application/testkit/ports、Git block、process backend |
| GMM-R6 | P3 | `appservice` 根仍有较宽 facade；需要按职责核对，但不得以 61 个文件或 projection 文件数量本身作为拆分依据 | `application/workorchd` |
| GMM-R7 | P3 | 根 README/AGENTS 仍把 `workorchruntime` 描述为 EventHub/bootstrap helper，且 `service/` 描述仍容许 owner direct facade，与当前/目标边界不一致 | repository/docs/module readme |

## Target Architecture

```text
application/workorch main
  -> explicit initiator/module imports
  -> framework Setup(all components)
       -> repository initiator ready
       -> owner dependencies wired
       -> downstream-required typed subscriptions ready
  -> framework Run(weight order)
       -> owners/capabilities start
       -> Runtime V2 recovery inventory is already callable
       -> Orchestrator schedules idempotent re-drive
       -> application routes/listener become ready
  -> process service waits on local lifecycle signal

cross component runtime I/O
  = EventHub Send/Post + owner-defined concrete DTO only
  != owner Service()
  != repository DTO
  != channel/store/registry/recorder

framework component shape
  = module.go lifecycle bridge -> owner biz (embedded base/biz.Base)
  -> protocol adapter/service when needed
  != module root retaining Hub/observer/subscription/business handler
```

### Authority Rules

1. `Weight()` 只控制生命周期顺序，不能作为 recovery contract 的唯一依赖。
2. Run owner 是 Runtime V2 aggregate 与 recovery inventory 的正式 owner；
   Orchestrator 只能通过 typed inventory command 获取恢复对象。
3. Orchestrator 是 SM-4 recovery drive authority；application、process service、
   Eventstream 和 projection 不得补建 recovery queue。
4. process service 可以拥有本地 wait channel，但该 channel 不能进入 EventHub
   payload/result。
5. owner `biz` / repository 只能由 owner module 内部访问；跨 owner production
   调用只保留 EventHub-backed port。
6. owner event DTO 与 repository model 分离；持久化 mapping 留在 owner 内部。
7. testkit 可以直接组装 owner fixture，但必须与 production assembly 隔离，且
   所有文件写入必须进入 `t.TempDir()` 或显式 `/tmp` artifact root。

## Runtime V2 State-Machine Impact

| 状态机 | 本方案影响 | 必须保持 |
| --- | --- | --- |
| SM-1 Run lifecycle | 无状态迁移变更 | Run owner 只持久化 lifecycle/aggregate；terminal immutable |
| SM-2 authority/focus | 无 reducer/action 变更 | 不新增 profile、领域或恢复分支 |
| SM-3 candidate/dispatch | 无 candidate/seal/evidence 语义变更 | 不改变 attempt 0/1、receipt fence、dispatch seal |
| SM-4 drive/recovery/projection | GMM-R1 修复启动 inventory 可用性和一次性恢复丢失 | recovery 只从 persisted aggregate + typed Run inventory 幂等 re-drive；projection 不重建 authority |

GMM-R1 的代码说明和测试名称必须显式标注 `SM-4`。其余工作包属于 framework
边界整改，不得借机修改 Runtime V2 状态语义。

## Non-Goals

- 不迁移或改名 `kernel` / `blocks` 分组。
- 不拆分为多个 Go module。
- 不恢复 V1 `StagePacket`、`NextAction`、`DispatchNextStep`、
  `ExecutionContextPatch`、auto successor 或 application drive queue。
- 不新增通用 lifecycle container、Service registry、generic EventHub envelope。
- 不因为文件数量大而新建 module/block；只在 owner、生命周期或纯算法边界
  明确时调整 focused package。
- 不重写现有 owner repository schema；GMM-R4 只调整进程内 typed contract。
- 不把 startup recovery 改成 projection/watchdog/application polling。
- 不通过单纯调整 `Weight()` 宣称 GMM-R1 完成。

## GMM-R1: Close Runtime V2 Startup Recovery Lifecycle Race

### Problem

framework plugin manager 按 `Weight()` 升序依次执行 `Run()`。当前：

```text
orchestrator Weight=30
  -> Start()
  -> AsyncFunction(scheduleRuntimeV2StartupRecovery)

run Weight=40
  -> Start()
  -> subscribe TopicRunCommandRuntimeV2RecoveryInventory
```

`BackgroundRoutine.AsyncFunction` 会立即将任务交给活动 worker，并不等待剩余
module 完成 `Run()`。如果 inventory query 先执行，EventHub 没有 Run owner
subscriber；当前实现只记录 warning 并返回，active/sealed/terminal-delivery-pending
Run 可能一直不被 re-drive。

### Required Design

采用“Setup 阶段建立 recovery inventory 可用性 + Run 阶段执行恢复”的明确合同：

1. Run module 在 `Setup` 完成 repository/configure 后，建立
   `TopicRunCommandRuntimeV2RecoveryInventory` 的窄只读订阅。
2. Run module 的常规 CRUD/CAS subscription 仍由自身生命周期管理；不得因此
   将全部业务 Run handler 提前到 initiator 或 application。
3. Run `Start()` 必须识别已建立的 recovery subscription，不能重复订阅。
4. Run `Stop()` / `Teardown()` 逆序、幂等释放 recovery 与常规 topics。
5. Orchestrator `Run()` 同步完成全部分页 recovery inventory acquisition 后才返回
   success。不能把 missing subscriber、wrong result type 或 owner error 降级为
   warning 后继续 ready。
6. inventory acquisition 任一页失败时，Orchestrator `Run()` 返回明确错误，由
   framework 将 service startup 标记为 failed；本轮不引入另一套 startup retry
   状态机。
7. inventory 读取成功后，具体 per-run drive 交给 `BackgroundRoutine`，并继续
   使用 run-scoped singleflight、persisted aggregate 与现有 recovery budget。
8. 不允许 application 在服务 ready 后再次扫描或 Resume active Run 作为补偿。

### Expected Files

- `internal/modules/kernel/run/plugin.go`
- `internal/modules/kernel/run/module.go`
- `internal/modules/kernel/run/module_test.go`
- `internal/modules/kernel/orchestrator/plugin.go`
- `internal/modules/kernel/orchestrator/module.go`
- `internal/modules/kernel/orchestrator/module_test.go`
- `internal/modules/kernel/orchestrator/runtime_v2_drive_test.go`
- `internal/modules/kernel/run/readme.md`
- `internal/modules/kernel/orchestrator/readme.md`

### Required Tests

- all modules `Setup` 完成、仅 Orchestrator 执行 `Run` 时，recovery inventory 已
  可查询；测试不能先手工调用 Run `Start()` 掩盖真实顺序。
- Run recovery subscription 重复 Setup/Start 不重复 handler。
- Stop/Teardown 后 inventory 不再可用，重复 Teardown 不 panic。
- missing Run inventory result 使 Orchestrator startup 明确失败，不进入 ready。
- `active_no_dispatch` restart 只 schedule 一次 drive。
- `active_sealed_dispatch` restart 不重复 owner side effect。
- `terminal_delivery_pending` restart 只重试 Result/Run/Workspace delivery。
- blocked governance inventory 只校验 persisted binding，不自动 Resume。
- `go test -race` 下并发 startup/recovery 无 duplicate drive 或数据竞争。

### Completion Gate

GMM-R1 只有在“不依赖 Weight 先后也能证明 inventory ready”时完成。仅把 Run
权重从 40 改为 29、增加 sleep、延迟 goroutine 或增加 application startup scan
均判定为未完成。

## GMM-R2: Replace Channel-Carrying Done Contract

### Problem

`workorchd/pkg/events.DoneCommandResult` 当前返回 `<-chan error`。这把 application
module 内部 server lifecycle handle 通过 EventHub 泄漏给 process service，违反
事件合同只传数据的资源边界。

### Required Design

使用“typed stopped notification + process-local wait state”：

1. `workorchd/pkg/events` 定义只含数据的 `ServiceStoppedData`，至少包含：
   `ServiceName`、`State`、`ErrorCode`、有界 `ErrorMessage`、`StoppedAt`。
2. workorchd module/service 在 HTTP server 退出时通过 `EventHub.Post` 发布一次
   stopped notification；payload 不包含 error object、channel 或 server pointer。
3. `internal/services/workorch` 在 framework `Run()` 前建立 observer，并在自身
  结构体中维护本地 channel/once；channel 不离开 process service。
4. process service 收到 notification 后唤醒 `waitServer`，并将 typed error data
   映射为进程返回错误。
5. Startup 失败、server 立即退出、正常 context cancel、重复 notification 和
   Shutdown/Teardown 必须有明确行为。
6. Post handler/observer 必须支持 `result=nil`；不得调用 `result.Set`。
7. 删除 `DoneCommand`、`DoneCommandResult` 与 `queryServerDoneChannel`，不保留
   channel compatibility contract。

### Expected Files

- `internal/modules/application/workorchd/pkg/events/messages.go`
- `internal/modules/application/workorchd/pkg/events/topics.go`
- `internal/modules/application/workorchd/pkg/events/contracts.go`
- `internal/modules/application/workorchd/module.go`
- `internal/modules/application/workorchd/module_test.go`
- `internal/modules/application/workorchd/service/server/service.go`
- `internal/services/workorch/service.go`
- `internal/services/workorch/runtime.go`
- corresponding process/application lifecycle tests

### Required Tests

- Event contract reflection scan 不含 channel、writer、server、store 等资源句柄。
- server 在 process observer 建立后立即退出，notification 不丢失。
- normal stop、error stop、duplicate stop 只唤醒一次。
- observer 在 `result=nil` 下不 panic。
- process Shutdown 先 unsubscribe，再释放本地 wait channel。
- framework startup 未完成时不能等待一个永远不会到达的 notification。

## GMM-R3: Retire Direct Owner Service Facades

### Scope

以下组件当前公开 `type Service = ...` 与 `Module.Service()`：

- `kernel/eventstream`
- `kernel/orchestrator`
- `kernel/run`
- `blocks/workspace`
- `blocks/result`
- `blocks/decision`
- `blocks/approval`
- `blocks/memory`
- `blocks/skill`
- `blocks/soul`
- `blocks/mcpregistry`

生产代码扫描未发现调用者，因此可以进行无 compatibility layer 的内部收口。

### Required Design

1. 删除各 module root 的 `Service()` getter 与公开 `Service` alias。
2. 把现有 module command handler 直接连接到本 owner `biz`，或保留真正具有
   protocol adapter 职责的内部 focused package。
3. 仅做函数转调、没有独立 lifecycle/protocol/owner 语义的 `service/` wrapper
   应删除或并回 module/biz；不得为了目录对称强行保留。
4. 如果某个 service package 确有 owner 内部复用价值，应移入 owner 私有
   `internal/` 路径，且 module 外不可导入。
5. 测试必须从 `Module.Service()` 迁移到：
   - owner biz 直接单元测试；或
   - EventHub command/result 集成测试。
6. application/testkit 可以直接构造 owner fixture，但不得通过 production
   `Module.Service()` 获取可变 owner 实现。
7. 不新增 `ServiceExports`、`SetXService`、全局 registry 或 narrow callback
   作为替代旁路。

### Work Waves

#### GMM-R3a Stateful Blocks

按以下顺序收口，保持每一批可独立编译测试：

1. `skill`、`soul`、`mcpregistry`
2. `memory`、`result`
3. `decision`、`approval`
4. `workspace`

#### GMM-R3b Kernel Owners

1. `eventstream`
2. `run`
3. `orchestrator`

Orchestrator service facade 包含 Block/Cancel/Supersede/Fail 等状态方法，删除时
必须证明所有 production 写入已经由 Orchestrator typed command 与 Runtime V2
authority gate覆盖，不能把测试为了方便调用的 bypass 迁移到 application。

### Static Completion Gates

以下扫描必须为空：

```text
rg '^func \(m \*Module\) Service\(' internal/modules
rg '^type Service =' internal/modules
rg 'ServiceExports|exports\.Services|Set[A-Z].*Service' internal application
```

允许的 `service` 目录只包括真正的 route/HTTP/CLI/session/protocol adapter；纯
owner method wrapper 不再保留。

## GMM-R4: Decouple Owner Event DTOs From Repository Types

### Current Couplings

| Owner contract | 当前 persistence type |
| --- | --- |
| Memory | `repository.MemorySearchQuery`、`MemoryRecallTraceQuery` |
| Workspace | `*repository.WorkspaceTemplate` |
| Eventstream | `repository.EventRecordQuery` |
| Governance | `repository.AutoGovernanceTraceQuery` |

### Required Design

1. 在各 owner `pkg/models`、`pkg/contracts` 或 `pkg/events` 定义稳定、具体、
   有界的 command query/result DTO。
2. DTO 必须包含足够 scope、cursor/limit、filter 和排序字段；不能退化为
   `map[string]any`。
3. owner module/biz 在 EventHub boundary 内完成 DTO ↔ repository query/entity
   mapping。
4. persistence repository interface 与 file/memory model 不进入 owner event
   package公开字段。
5. application、process backend、kernel consumer 同步切换 owner DTO；不保留
   repository type alias compatibility。
6. REST/CLI DTO 不直接复用 EventHub DTO；映射仍留在 application/service layer。
7. 该变更不修改 persisted file schema，不迁移历史 runtime aggregate。

### Target DTOs

目标 DTO 使用现有字段语义直接切换，不增加兼容 alias：

```text
memoryevents.MemorySearchQuery
  WorkspaceID, Query, Tags, Types, Vector, MinScore, Limit

memoryevents.MemoryRecallTraceQuery
  WorkspaceID, RunID, StepID, Stage, Source, Status, Limit

eventstreamevents.EventRecordQuery
  WorkspaceID, RunID, Topic, ExcludeTopics, SubjectType, SubjectID,
  CursorAfterID, OccurredAfter, OccurredBefore, Limit, IncludePayload

governanceevents.AutoGovernanceTraceQuery
  WorkspaceID, RunID, StepID, ItemType, ItemID, Status, Limit

workspace model/contract WorkspaceTemplate
  ID, Name, Description, GoalText, DoDItems, DefaultSoulID,
  SoulBindings, ContextHints, EvidencePolicies
```

边界规则：

- query DTO 放 owner `pkg/events` 或 `pkg/contracts`；
- WorkspaceTemplate 正式 DTO 放 `blocks/workspace/pkg/models`，event result 引用
  owner model；
- Vector、Tags、Types、ExcludeTopics、DoDItems 等 slice 必须 clone 并执行现有
  上限；
- `SoulBindings` 等具体 map 必须 clone，不能携带可变 repository entity；
- repository mapping 函数保持 owner-private，不导出给 application。

### Required Tests

- query scope、limit、cursor、filter mapping 完整。
- zero/negative/oversized limit 有确定性处理。
- workspace/run duplicate local ID 仍按 explicit scope 解析。
- repository error/conflict 不被 mapping 吞掉。
- event contract registry 显示 owner definition package，不出现 repository package。

### Static Completion Gate

```text
rg 'internal/pkg/repository' internal/modules/*/*/pkg/events --glob '*.go'
```

结果必须为空。

## GMM-R5: Isolate Test Runtime Data From Source Packages

### Current Residue

测试运行后可在以下类别目录看到 ignored runtime data：

- `workorchd/biz/appservice/activationports/.workorch-data`
- `workorchd/biz/appservice/runports/.workorch-data`
- `workorchd/biz/testkit/.workorch-data`
- `blocks/git/.workorch-data`
- `internal/services/workorch/biz/backends/.workorch-data`
- corresponding `.workorch-testkit-git-receipts`

此外存在没有 Go 文件的历史目录，如 `clicore`、`ownercommandports`、
`retirementcore`、`retirementports`、`runtimeports`、`blocks/pkg` 以及已清空的
field codec 目录。

### Required Design

1. 所有会创建 repository data、workspace internal data、Git receipt 或 E2E
   artifact 的测试必须显式注入 `t.TempDir()`。
2. 测试 helper 不得依赖 package working directory、repository 默认相对路径或
   当前进程 cwd。
3. Git capability receipt store 测试必须显式传入 workspace internal root。
4. E2E/live artifact 继续落在 `${TMPDIR:-/tmp}/workorch-e2e/...`，并拒绝 repo
   `.workorch-e2e`。
5. 不接受“新增 `.gitignore` 规则”作为整改；目标是文件不产生在源码树。
6. 空的历史 package 目录不作为新 layer 保留；如果没有明确职责和代码，直接
   清理。若未来需要，按真实职责重新创建。
7. 测试结束必须关闭 hub/background/client/session，避免目录因未释放句柄不能
   删除。

### Verification

在清洁源码树上运行目标测试与全量测试后执行：

```text
find application internal -type d \
  \( -name '.workorch-data' -o -name '.workorch-testkit-git-receipts' -o -name '.workorch-e2e' \)
```

结果必须为空；`git status --short --ignored` 不应显示这些测试产物。

## GMM-R6: Bound The Application Facade Without Mechanical Splitting

### Decision Rule

`workorchd/biz/appservice` 文件数量本身不是缺陷。是否移动代码只依据：

- 是否混合 owner 写入、projection、配置、协议或 runtime authority；
- 是否可以用一句话定义 focused package；
- 是否存在独立稳定 port/DTO/纯算法边界；
- 移动后是否减少反向依赖，而不是制造 wrapper。

### Allowed Root Responsibilities

appservice 根可保留：

- application use-case facade；
- RuntimeSession 构造与关闭；
- CLI command dispatcher；
- 面向 server/process adapter 的窄 service entrypoint；
- 只做参数投影和调用 focused port 的短 facade。

### Must Move Or Remove When Found

- projection 算法 → `readmodelprojection` / `workspaceviewcore`；
- owner EventHub command mapping → `ownerreadports` / focused owner port；
- configuration normalization → `configcore`；
- workspace definition/activation pure rules → corresponding `*core`；
- HTTP/SSE serialization → `service/server`；
- 空 package、单函数无语义 wrapper、双向依赖 package → 删除或并回真正 owner。

### Guardrails

- 不新增 application 子 module/block。
- 不把 application use case 下沉到 stateful block。
- 不把 projection store 变成正式状态 owner。
- 不移动 Runtime V2 reducer/drive/candidate/evidence authority。
- 不以“每个目录都有 biz/service/pkg”为目标。
- 每次移动必须有 import-cycle、public API 和 direct test 证明。

### Completion Evidence

完成一次 root facade inventory，逐文件标记 `facade / pure core / port / protocol /
obsolete`。只有明确误放项才移动。最终文档应记录保留理由；不能只报告文件数
下降。

### Root Facade Inventory (2026-07-18)

逐一核对后，没有需要为 R6 迁移的生产根文件。纯规则、owner command mapping、
configuration normalization、投影算法和 HTTP/SSE 协议已经分别位于 `*core`、
`*ports`、`ownerreadports`、`readmodelprojection` 和 `service/server`。下表按每个
根文件的命名族标记职责；`*_test.go` 覆盖所有根测试文件。

| 分类 | 根文件 | 保留理由 |
| --- | --- | --- |
| use-case facade | `approval_*service.go`、`decision_*service.go`、`artifact_content_service.go`、`configuration_service.go`、`goal_service.go`、`dod_service.go`、`goal_dod_quality_check_service.go`、`mcp_*service.go`、`memory_*service.go`、`run_*service.go`、`runtime_raw_content_service.go`、`skill_hub_service.go`、`skill_read_service.go`、`skill_service.go`、`soul_read_service.go`、`soul_service.go`、`workspace_*service.go`、`workspace_execution_defaults.go` | 面向 HTTP/CLI/process adapter 的窄用例入口；只编排 focused port，不持有 owner repository 或 Runtime V2 authority。 |
| CLI adapter | `cli_command_dispatcher.go`、`cli_governance_continuation.go`、全部 `cli_typed_*.go` | application 内部 CLI command mapping；稳定端侧 port 仍在 `internal/pkg/workorchport`。 |
| projection entrypoint | 全部 `read_model_*.go` | 只驱动/读取派生 read model；正式状态和恢复 authority 不在此处。 |
| session/lifecycle | `backend_session_pool.go`、`runtime_session.go`、`session_helpers.go` | 生命周期和 local/remote backend session 组装；不作为 framework module 或 owner facade。 |
| DTO | `app_models.go`、`context_registry_types.go` | application-local view/request DTO；没有 owner state。 |
| tests | 全部 `*_test.go`、`governance_evidence_test_support_test.go`、`owner_modules_fixture_test.go` | 仅测试 fixture/contract coverage，不构成 production boundary。 |

因此 R6 的收口是记录并维持当前结构，而不是机械移动文件。新增根文件必须先按本表
分类；projection algorithm、owner mapping、config normalization 和 HTTP serialization
不得回流到根目录。

## GMM-R7: Align Documentation With The Final Component Truth

### Current Drift

- `README.md` 与 `AGENTS.md` 仍把 `internal/initiators/workorchruntime` 描述为
  repository provider、EventHub access、bootstrap helper 的组合；当前代码和
  initiator readme 已明确它只提供 repository infrastructure。
- 根 README 的通用结构说明把 `service/` 描述为 module-facing surface，容易被
  解读为 owner 可导出 direct service；目标边界中 `service/` 只保留
  HTTP/CLI/session/protocol adapter，跨 owner 仍只能走 EventHub。
- current-state/architecture overview 尚未记录本轮 lifecycle、channel、facade、
  repository DTO 和 test-data isolation 的真实 open 状态。

### Required Changes

1. `README.md`、`AGENTS.md`、`docs/30-module-architecture/*` 与各 module readme
   使用同一 22-component inventory 和职责分类。
2. initiator 描述只保留 repository provider/factory；不得再声明 EventHub、
   BackgroundRoutine、runtime policy 或 bootstrap export。
3. `service/` 定义为 protocol adapter；owner direct method wrapper 不作为推荐
   module surface。
4. Orchestrator/Run readme 明确 Setup-phase recovery inventory readiness 与 SM-4
   startup behavior。
5. workorchd/process-service readme 明确 stopped notification 与 process-local
   wait state，禁止 channel EventHub contract。
6. owner readme 明确 event DTO 与 repository model 分离。
7. 文档中的验证命令、文件路径和 active/completed 状态与最终代码一致。

文档更新必须与相应代码包同提交或紧邻提交，不能提前宣称 completed。

## GMM-R8: Migrate EventHub Ownership From Module Roots To Owner Biz

### Audit Finding

`go-multi-module-dev` requires every framework Module or Block that uses the
EventHub to own that interaction in a component-local `biz` type embedding
`internal/modules/base/biz.Base`. The initial audit found no shared Base Biz
package and found direct `event.Hub`, `Subscribe`, `Send`, or `Post` use in
the roots of the application module and the kernel/block owners. This is a
structural boundary defect even where the existing typed contracts and tests
otherwise pass.

`kernel/system` and capability `blocks/file` / `blocks/git` / `blocks/llm` /
`blocks/mcp` / `blocks/shell` / `blocks/ssh` / `blocks/web` are the first
migrated components:
their Module roots now only bridge lifecycle and delegate to Base-embedding
Biz types, which are their sole Hub entrypoints. Direct lifecycle and typed
File-operation tests remain green.

### Required Design

1. Add the owner-neutral `internal/modules/base/biz.Base` exactly once. It
   contains only owner ID, injected Hub, simple observer, injected Background
   Routine, owner-neutral EventHub helpers, and task wrappers. It has no
   plugin registration, business topic, repository, configuration, or concrete
   component import.
2. For every Hub-using Module/Block, move Hub fields, observer lifecycle,
   subscriptions, sends/posts, typed handlers, and owner use cases to its own
   `biz` package/type embedding Base.
3. `module.go`/`plugin.go` may receive Hub and BackgroundRoutine solely in
   framework `Setup` and pass them to `biz.New`; they must not retain either,
   create observers, subscribe, or issue events.
4. Protocol `service/` packages may depend on their own Biz narrow use cases,
   but must not receive or retain Hub. Cross-owner communication remains typed
   EventHub traffic from Biz.
5. Initiators remain excluded: `workorchruntime` stays a single repository
   infrastructure provider and must not embed Base.
6. Preserve Runtime V2 authority: this is a boundary move only. In particular,
   Orchestrator remains sole SM-2/SM-3/SM-4 authority and the migration must
   not introduce application drive, callback, Service registry, or direct
   repository path.

### Migration Waves

1. shared Base + `kernel/system` + `blocks/file` + `blocks/git` +
   `blocks/llm` + `blocks/mcp` + `blocks/shell` + `blocks/ssh` +
   `blocks/web` (completed);
2. remaining stateful block (`decision`); `blocks/workspace`, `blocks/result`,
   `blocks/approval`, `blocks/memory`, `blocks/skill`, `blocks/soul`, and
   `blocks/mcpregistry` completed;
3. kernel owners (`eventstream`, `governance`, and `run` completed;
   `orchestrator` completed);
4. `application/workorchd` framework root and CLI command dispatcher
   completed. The HTTP `serverState` now receives only a `RuntimeSession` and
   `BackgroundRoutine`; Hub construction, request-context injection, and
   server-side subscription/dispatch were removed. Process-facing
   protocol-adapter scans remain part of the final matrix;
5. static structural scan and all affected lifecycle/EventHub tests.

Each wave must preserve owner DTO contracts, idempotent teardown, nil-Result
Post handling, and BackgroundRoutine ownership. Do not retain a forwarding
root object merely to preserve a direct owner implementation surface.

### Current Structural Evidence (2026-07-19)

The final root-dependency pass moved the framework Hub and BackgroundRoutine
construction inputs into each component-local `biz.Dependencies`. In
particular, `approval`, `decision`, `git`, `llm`, `memory`, `mcpregistry`,
`result`, `shell`, `skill`, `soul`, `ssh`, and `workspace` no longer declare
Hub or observer types in `module.go`; their plugin `Setup` implementations
only assemble and pass the Biz-owned dependency DTO.

The following production scans meet the stated boundary (the first has the
single framework-signature exception described below):

```text
rg --glob 'module.go' 'event\.Hub|magicevent\.Hub|event\.SimpleObserver|magicevent\.SimpleObserver|\.Subscribe\(|\.Unsubscribe\(|\.Send\(|\.Post\(' internal/modules
rg --glob '**/service/**/*.go' --glob '!*_test.go' 'event\.Hub|magicevent\.Hub|event\.SimpleObserver|magicevent\.SimpleObserver|NewSimpleObserver|\.Subscribe\(|\.Unsubscribe\(|\.Send\(|\.Post\(' internal/modules
rg --glob 'plugin.go' '\.Subscribe\(|\.Unsubscribe\(|\.Send\(|\.Post\(|NewSimpleObserver' internal/modules
rg --glob '*.go' --glob '!*_test.go' --glob '!**/testkit/**' 'internal/modules/(blocks|kernel)/[^/]+/(biz|service|internal)' internal/modules/application internal/services
```

The first scan has the single framework-mandated `workorchd.Module.Setup` Hub
parameter; it does not retain or operate on the Hub. Targeted Block tests,
Run/Orchestrator race tests, application approval/session tests, the HTTP
server package, and the repository gates recorded above are fresh evidence for
this source state. Only deployed restart-fixture acceptance remains required
by the matrix below.

`scripts/verify-boundary-contract.sh` now makes the component audit
repeatable: it discovers all 21 `internal/modules/**/module.go` roots, requires
each local `biz/` to embed `basebiz.Base`, rejects EventHub operations in roots
and production `service/` adapters, keeps initiators Base-free, and rejects
unbounded/resource-carrying types in every owner `pkg/events` contract. The
20 standalone `plugin.go` registrations plus `workorchd/module.go` account for
all 21 module roots; `initiators/workorchruntime` is the sole non-Module
component and intentionally remains Base-free. The application Start/Resume
bridge has an explicit regression test proving the two outbound EventHub
commands retain their concrete types rather than using an `any` carrier.

## 22-Component Closure Matrix

The following historical per-component status is supplemented by GMM-R8. Every
Hub-using Module/Block in the table now has a component-local Base-embedding
Biz; the remaining entries are functional and final-verification evidence, not
an exemption from the Base Biz topology gate.

| 组件 | 当前结论 | 必须完成的整改 | 最终证据 |
| --- | --- | --- | --- |
| `initiators/workorchruntime` | 符合 | GMM-R7 文档 truth alignment；防回归、不扩大职责 | initiator contract test + no EventHub/policy export scan |
| `application/workorchd` | Base Biz topology 符合；其余既有边界结论待最终矩阵复核 | GMM-R2、R5、R6 防回归 | `module.go -> biz.Biz(Base)` scan、channel-free events、source-tree clean |
| `kernel/eventstream` | Base Biz topology 符合；其余既有边界结论待最终矩阵复核 | GMM-R3、R4 防回归 | `module.go -> biz.Service(Base)` scan、owner query DTO、targeted tests |
| `kernel/governance` | Base Biz topology 符合；其余既有边界结论待最终矩阵复核 | GMM-R4 防回归 | `module.go -> biz.Service(Base)` scan、owner query DTO、auto-governance tests |
| `kernel/orchestrator` | Base Biz topology 符合；其余既有边界结论待最终矩阵复核 | GMM-R1、R3 防回归 | `module.go -> biz.Biz(Base)` scan、lifecycle-order restart tests、no Service getter |
| `kernel/run` | Base Biz topology 符合；其余既有边界结论待最终矩阵复核 | GMM-R1、R3 防回归 | `module.go -> biz.Biz(Base)` scan、Setup inventory readiness、CAS/recovery tests |
| `kernel/system` | 符合 | 防回归 | typed default ensure + lifecycle test |
| `blocks/workspace` | 符合 | 防回归 | no Service getter、template DTO |
| `blocks/result` | 符合 | 防回归 | EventHub-only owner writes |
| `blocks/decision` | Base Biz topology 符合；其余既有边界结论待最终矩阵复核 | GMM-R3 防回归 | EventHub-only resolution/evidence |
| `blocks/approval` | 符合 | 防回归 | EventHub-only approval/evidence |
| `blocks/memory` | 符合 | 防回归 | no Service getter、memory query DTO |
| `blocks/skill` | Base Biz topology 符合；其余既有边界结论待最终矩阵复核 | GMM-R3 防回归 | EventHub-only registry contract |
| `blocks/soul` | Base Biz topology 符合；其余既有边界结论待最终矩阵复核 | GMM-R3 防回归 | EventHub-only registry/default contract |
| `blocks/mcpregistry` | Base Biz topology 符合；其余既有边界结论待最终矩阵复核 | GMM-R3 防回归 | EventHub-only registry/default contract |
| `blocks/file` | 符合 | 防回归 | path/governance/receipt tests |
| `blocks/git` | 符合 | GMM-R5 test path only | operation/receipt tests + no source artifact |
| `blocks/llm` | 符合 | 防回归 | catalog/provider/candidate/cache tests |
| `blocks/mcp` | 符合 | 防回归 | registry EventHub + bounded transport tests |
| `blocks/shell` | 符合 | 防回归 | bounded execution/validation tests |
| `blocks/ssh` | 符合 | 防回归 | structured auth/timeout/preflight tests |
| `blocks/web` | 符合 | 防回归 | approval/SSRF/config-revision tests |

## Implementation Order

严格按以下顺序执行，避免先清理表面结构再留下 restart correctness 缺口：

1. **GMM-R1 SM-4 lifecycle closure**
2. **GMM-R2 channel-free process/application lifecycle**
3. **GMM-R3 stateful block service facade retirement**
4. **GMM-R3 kernel service facade retirement**
5. **GMM-R4 owner DTO decoupling**
6. **GMM-R5 source-tree test isolation**
7. **GMM-R6 application facade evidence-based cleanup**
8. **GMM-R7 documentation truth alignment**
9. **GMM-R8 Base Biz topology migration**
10. **full structural, race, file-backed and deployed verification**

每个步骤独立提交、独立通过 targeted tests。不得把所有 facade/DTO/lifecycle 变更
压入一个不可审计的大提交。

## Verification Matrix

### Structural Gates

```text
find application/workorch -maxdepth 3 -type f
rg '_ "github.com/muidea/workorch/internal/(initiators|modules)' application/workorch/main.go
rg '^func \(m \*Module\) Service\(' internal/modules
rg '^type Service =' internal/modules
rg 'ServiceExports|exports\.Services|Set[A-Z].*Service' internal application
rg 'internal/pkg/repository' internal/modules/*/*/pkg/events --glob '*.go'
rg 'map\[string\]any|\[\]any|<-chan|chan ' internal/modules/*/*/pkg/events internal/modules/application/workorchd/pkg/events --glob '*.go'
rg 'event\.Hub|event\.NewSimpleObserver|\.Subscribe(Func)?\(|\.Send\(|\.Post\(' internal/modules --glob 'module.go' --glob 'plugin.go'
rg 'internal/modules/.*/(biz|service)' internal/modules internal/services --glob '*.go'
find application internal -type d \( -name '.workorch-data' -o -name '.workorch-testkit-git-receipts' -o -name '.workorch-e2e' \)
```

结果解释必须区分同 owner 内部 import、test-only direct fixture 和真实 production
跨 owner import，不能只以 grep 命中数量判断。

### Targeted Tests

```text
go test -race ./internal/modules/kernel/run/... -count=1
go test -race ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/services/workorch/... -count=1
go test ./internal/modules/kernel/eventstream/... -count=1
go test ./internal/modules/kernel/governance/... -count=1
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/blocks/result/... -count=1
go test ./internal/modules/blocks/decision/... -count=1
go test ./internal/modules/blocks/approval/... -count=1
go test ./internal/modules/blocks/memory/... -count=1
go test ./internal/modules/blocks/skill/... -count=1
go test ./internal/modules/blocks/soul/... -count=1
go test ./internal/modules/blocks/mcpregistry/... -count=1
```

### Repository Gates

```text
make fmt-check
go test -p 1 ./... -count=1
go vet ./...
make verify-boundary-contract
make verify-runtime-v2-positive-convergence-file
git diff --check
make build
```

### Deployed Restart Acceptance

必须新增或扩展 deployed verifier，覆盖至少三种 fresh file-backed Runtime V2
restart fixture：

1. `active_no_dispatch`
2. `active_sealed_dispatch`（owner receipt 前后至少一个 crash point）
3. `terminal_delivery_pending`

验收流程：

```text
create fresh workspace/run
  -> persist target Runtime V2 phase
  -> stop service without terminal rewrite
  -> deploy/start latest binary
  -> do not call Resume/Activate
  -> observe automatic SM-4 recovery
  -> assert exactly-once/no duplicate effect
  -> assert accepted terminal or expected governed wait
  -> assert activeRun released at terminal
  -> assert projection source/projected revision aligned
```

随后再运行 fresh bounded terminal 与 standard positive acceptance，证明 lifecycle
整改没有破坏正常 activation 主链。所有 artifacts 必须位于 `/tmp/workorch-e2e` 或
显式临时 root，不得进入仓库。

部署后恢复核验使用 `make verify-runtime-v2-restart-recovery-live`。它要求调用方在
restart 前创建三个 fresh file-backed fixture，并通过
`WORKORCH_E2E_RESTART_FIXTURE_MANIFEST` 提供如下 JSON（每个 fixture 仅含
`class`、`workspaceId`、`runId`）：

```json
{"fixtures":[
  {"class":"active_no_dispatch","workspaceId":"...","runId":"..."},
  {"class":"active_sealed_dispatch","workspaceId":"...","runId":"..."},
  {"class":"terminal_delivery_pending","workspaceId":"...","runId":"..."}
]}
```

该 verifier 不发送 Activate/Resume，也不重启服务；它只在最新实例已启动后轮询
owner snapshot/observability，验证三类 fixture 自动到达 terminal 或 governed wait、
没有重复 executionKey/fence receipt、terminal activeRun 已释放且 projection revision
已经对齐。

## Completion Criteria

本方案只有在以下条件全部满足时才能归档：

1. Runtime V2 startup recovery 不依赖 Run/Orchestrator `Weight()` 偶然顺序，真实
   restart fixture 可无人工 Resume 自动推进。
2. EventHub 所有 command/data/result 不携带 channel、writer、server、repository、
   store、registry、recorder 或其他可变资源句柄。
3. 11 个 `Module.Service()` owner facade 与对应无协议 wrapper 已清零。
4. owner `pkg/events` 不再 import `internal/pkg/repository`。
5. production cross-owner I/O 仍只有 typed EventHub；无 application/repository
   直连、callback registry 或 generic bridge 回归。
6. 源码树在 targeted/full tests 后不产生 WorkOrch runtime/test data 目录。
7. appservice facade inventory 已完成；保留与移动均有职责证据，没有机械拆包。
8. 使用 Hub 的 21 个 Module/Block 都有 component-local Biz 嵌入 Base；Module
   root、plugin 和 protocol service 不再保留 Hub/observer/subscription/event calls。
9. 22 个组件 direct tests、race tests、全量 tests、vet、boundary、build 通过。
10. file-backed crash/restart matrix 与 fresh deployed positive acceptance 通过。
11. `current-state`、module readme、active matrix 和 verification 文档与最终实现一致。

## Closure Documentation

代码和 live 全部完成后：

1. 将本文件 `git mv` 到 `docs/90-archive/completed/`，文件名追加完成日期；
2. 更新 `docs/70-roadmap/active/overview.md` 删除 `ARCH-1`；
3. 更新 `docs/01-overview/current-state.md` 与
   `current-unclosed-matrix.md` 的完成事实；
4. 在 completed overview 登记最终 commit、验证命令和 deployed artifact root；
5. 不把 live JSON、runtime data 或临时目录提交到仓库。
