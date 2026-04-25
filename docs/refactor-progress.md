# WorkOrch 整理进度跟踪

> 最近更新：2026-04-24

> 说明：本文是历史归档记录，不作为当前活跃待办清单使用。较早阶段提到的 `phase1`、`application/workorchd/demo/scenarios.go`、`snapshot.go` 等中间态命名和文件已在后续阶段替换或删除；当前状态以 `docs/workorch-status.md` 为准。

## 归档内容

围绕 `go-multi-module-dev` 规则，记录 `workorch` 模块边界、应用层职责和代码落点的历史收敛过程。

## 已完成

### 1. 模块骨架补齐

- 已为以下模块补齐 `module.go`
  - `internal/modules/kernel/workspace`
  - `internal/modules/kernel/run`
  - `internal/modules/kernel/decision`
  - `internal/modules/kernel/orchestrator`
  - `internal/modules/blocks/approval`
  - `internal/modules/blocks/legacy`
- 已为以上模块补齐模块级 `pkg/common/ModuleID`
- 已为缺失模块补齐 `service/` 契约层文件

### 2. 模块名与入口收敛

- `application/workorchd/main.go` 已改为优先依赖模块根包，而不是直接拼接 `biz` 层
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go` 中的 `SourceModule` 已切换为模块常量
- `application/workorchd/demo/scenarios.go` 已切换为优先使用模块根包导出的 command 类型别名，不再直接依赖 `internal/pkg/contracts`

### 3. 应用层拆分

该阶段完成了应用层的首次拆分，后续已继续收敛为当前五层：

- `application/workorchd/main.go`
  - 只负责进程入口和输出循环
- `application/workorchd/bootstrap`
  - 负责 env、backend、repository 与 runtime 装配
- `application/workorchd/appservice`
  - 负责 application 级 orchestration / workflow 组装
- `application/workorchd/demo`
  - 当前仅保留兼容性导出壳层
- `application/workorchd/presenter`
  - 负责输出渲染

### 4. demo 进一步拆分

该阶段曾将 `application/workorchd/demo` 按职责拆成：

- `demo.go`
  - 场景注册与总调度
- `scenarios.go`
  - phase1 场景流转
- `snapshot.go`
  - 快照读取与组装

以上 `scenarios.go` / `snapshot.go` 已在第 16 节完成下沉并删除，当前对应职责已移动到 `application/workorchd/appservice`。

### 5. 测试补充

- 保留原有 `application/workorchd` 冒烟测试
- 新增 `application/workorchd/bootstrap/runtime_test.go`
  - 覆盖 `memory` backend 组装
  - 覆盖 `file` backend 最小持久化行为
- 新增 `application/workorchd/demo/demo_test.go`
  - 覆盖 `memory` backend 下 4 条 phase1 场景返回结果
  - 覆盖 `file` backend 下场景执行后的持久化文件落地
- 新增 `application/workorchd/demo/scenarios_test.go`
  - 覆盖 blocked run 上重复 start 的冲突路径
  - 覆盖 completed run 上重复 start 的幂等路径
  - 覆盖未阻塞 run 上 resume 的冲突路径
  - 覆盖 decision 未 resolve 时 resume 的 blocked 路径
  - 覆盖 failed run 上重复 resume 的终态幂等路径
  - 覆盖 run 与 workspace 绑定不一致时的冲突路径
  - 覆盖 ticket / approval 绑定不一致时的冲突路径
  - 覆盖 approval 已 reject 后再次 approve 的冲突路径
  - 覆盖 approval 已 approve 后再次 reject 的冲突路径

### 6. bootstrap 能力面收敛

- `application/workorchd/bootstrap.Runtime` 已开始从公开字段集合收敛为显式方法面
- `demo/scenarios.go` 与 `demo/snapshot.go` 已切换为通过 `Runtime` 方法访问 workspace/run/step/ticket/approval 与服务动作
- 应用层对底层 repository / service 字段的直接依赖已显著减少

### 7. bootstrap 职责继续拆分

- `application/workorchd/bootstrap/runtime.go` 已收敛为 `Runtime` 类型与方法面
- `application/workorchd/bootstrap/backend.go`
  - 负责 env 常量、repository backend 选择与 repository 装配
- `application/workorchd/bootstrap/assembly.go`
  - 负责 `Runtime` 组装、runtime adapter、event hub 与各模块 service 接线
- `bootstrap` 包内的 “后端选择 / 装配接线 / 运行时方法” 已不再堆在同一文件中

### 8. 模块内 command 契约收敛

- `internal/modules/kernel/workspace/service/service.go` 已收回 `CreateWorkspaceCommand` 与 `WorkspaceService`
- `internal/modules/kernel/run/service/service.go` 已收回 run 模块全部 command DTO 与 `RunService`
- `internal/modules/kernel/decision/service/service.go` 已收回 decision 模块 command DTO 与 `DecisionService`
- 对应 `module.go`、`biz/*.go` 和单测已切换为优先依赖模块本地 `service` 契约，不再直接依赖 `internal/pkg/contracts`

### 9. orchestrator 契约收尾

- `internal/modules/kernel/orchestrator/service/service.go` 已收回 `RuntimeOrchestrator` 接口定义
- `internal/modules/kernel/orchestrator/module.go` 已补齐模块级 `Service` 导出
- 当前 `application/` 与 `internal/modules/` 范围内，已不存在对 `internal/pkg/contracts` 的直接依赖

### 10. contracts 遗留包清理完成

- `internal/pkg/contracts` 已整体删除
- `README.md`、`docs/phase1-status.md`、`docs/structure.md` 已同步去除过渡期描述
- 共享内部能力当前仅保留 `internal/pkg/{common,events,repository}`

### 11. 应用层 command 感知继续压缩

- `application/workorchd/bootstrap.Runtime` 已进一步收敛为应用级参数面
- `application/workorchd/demo/scenarios.go` 已不再直接构造任何模块 command DTO
- `application/workorchd/demo/scenarios_test.go` 与 `application/workorchd/bootstrap/runtime_test.go` 已切换为通过 `Runtime` 应用级方法验证行为
- 当前 `application/workorchd` 范围内，模块 command DTO 的构造已集中在 `bootstrap` 包内部

### 12. 业务模块逐项收口启动

- 已将业务模块收口顺序明确为：`workspace -> approval -> decision -> run -> orchestrator -> legacy`
- `workspace` 已作为首个收口模块补齐重复创建冲突保护测试
- `approval` 已补齐创建参数校验、重复创建冲突、resolve 空 ID / not found 等边界测试
- `decision` 已补齐创建参数校验、重复创建冲突、resolve 空参数 / not found 等边界测试
- `run` 已补齐重复创建冲突、`StartRun` 参数校验、`CompleteRun/FailRun` 参数校验与无 orchestrator 本地终态落库测试
- `orchestrator` 已补齐 active-run 冲突、dispatch 参数校验、blocking 引用一致性、resume 缺失 blocking 引用、`CompleteRun/FailRun` 参数校验等测试
- `legacy` 已补齐 minimal runtime adapter 输出与 minimal tool adapter 构造的最小行为测试
- 后续模块继续按“能力面 / 边界行为 / 测试覆盖 / 应用接线”四个维度逐项收口

### 13. 业务集成收口首轮完成

- 已新增 `application/workorchd/demo/integration_test.go`
- 已基于 `application/workorchd/demo/runAll(...)` 与 `presenter.Snapshot` 建立 phase1 业务集成断言
- 业务集成断言当前已同时覆盖 `memory` 与 `file` 两种 repository backend
- 已覆盖 `workspace`
  - 校验 4 条 phase1 slice 的 workspace 最终均保持 `active`
  - 校验 `workspace.activeRunId` 已正确清空
  - 校验 workspace 与 run 的绑定关系一致
- 已覆盖 `approval`
  - 校验 approval-resume slice 中 approval 最终为 `approved`，run 最终为 `completed`
  - 校验 approval-rejected slice 中 approval 最终为 `rejected`，run 最终为 `failed`
- 已覆盖 `decision`
  - 校验 decision-resume slice 中 decision ticket 最终为 `resolved`
  - 校验对应 run 与 step 最终进入完成态
- 已覆盖 `run`
  - 校验 4 条 phase1 slice 的终态分别为 `completed/completed/completed/failed`
  - 校验 run 上 `blockingTicketId` 与 `blockingApprovalId` 均已在终态清空
- 已覆盖 `orchestrator + legacy`
  - 校验可执行 slice 中 runtime step 已按 `legacy/minimal` 成功分发并完成
  - 校验 step output 中 `runId`、`stepId`、`status` 与业务结果一致
  - 校验 approval-rejected 终态 slice 不会错误地产生 step 快照
- 已完成验证
  - `go test ./application/workorchd/demo/... -count=1`
  - `go test ./... -count=1`
- `file` backend 现已具备与 `memory` backend 等价的 phase1 最终快照与终态断言，不再只校验文件落地

### 14. 模块语义收口第一轮

- 已为 `workspace / run / decision / orchestrator / approval / legacy` 六个模块补齐真实 `Module` 入口
- 当前各模块 `module.go` 已不再只是 type alias / constructor 转发
- 当前各模块均已具备最小生命周期方法
  - `ID()`
  - `Setup(...)`
  - `Run()`
  - `Teardown(context.Context)`
- `application/workorchd/bootstrap/assembly.go` 已切换为通过模块入口统一装配
  - 先构造模块对象
  - 再执行 `Setup(...)`
  - 再执行 `Run()`
  - 不再直接在 bootstrap 中手工 new 各模块 `biz/service` 实现
- `application/workorchd/bootstrap.Runtime` 已补齐 `Shutdown(context.Context)`，具备按逆序 teardown 模块的最小能力面
- `approval` 模块 biz 已切换为统一依赖 `service/` 契约 command
  - 不再在 `biz` 内维护一套独立的 command DTO
  - 当前 `approval/module.go` 已可稳定暴露模块级 `Service()`
- 当前模块装配链路已收敛为
  - `legacy` 提供 runtime/tool adapter
  - `orchestrator` 依赖 runtime adapter 与 repository/event hub
  - `run` 依赖 orchestrator service
  - `workspace / decision / approval` 通过各自模块入口暴露稳定 service 面
- 已完成验证
  - `go test ./application/workorchd/bootstrap ./internal/modules/... -count=1`
  - `go test ./... -count=1`

### 15. service 语义收口第二轮

- 已为以下模块补齐显式 `service` adapter
  - `internal/modules/kernel/workspace/service`
  - `internal/modules/kernel/run/service`
  - `internal/modules/kernel/decision/service`
  - `internal/modules/kernel/orchestrator/service`
  - `internal/modules/blocks/approval/service`
- 当前以上 `service/` 包已不再只是 command DTO 与 interface 壳层
- 当前 `service/` 包已具备最小 adapter 语义
  - 通过显式 `Module` 结构承接模块对外 service 面
  - 通过函数依赖注入方式适配 `biz` 能力
  - 在未绑定 handler 时返回 `ErrInvalidArgument`，避免静默空实现
- 对应 `module.go` 已进一步收敛为 `biz + service` 双对象装配
  - `Setup(...)` 先构造 biz
  - 再构造 service adapter
  - `Service()` 返回模块 service adapter，而不是直接暴露 biz 实现
- `approval` 之外，`workspace / run / decision / orchestrator` 也已完成一轮 “biz 通过 service adapter 暴露” 收口
- 已完成验证
  - `go test ./application/workorchd/bootstrap ./internal/modules/... -count=1`
  - `go test ./... -count=1`

### 16. application 编排下沉第三轮

- 已新增 `application/workorchd/appservice`
  - 作为 application 级 orchestration / workflow 落点
  - 当前承接 phase1 demo 所需的业务推进与快照组装
- `application/workorchd/demo/demo.go` 当前已收敛为薄包装层
  - 顶层导出入口仅负责委托 `appservice.RunAllPhase1(...)`
- phase1 四条场景的业务推进逻辑已从 `demo` 下沉到 `application/workorchd/appservice/phase1.go`
  - `RunRunnableSlice`
  - `RunDecisionBlockedResumeSlice`
  - `RunApprovalBlockedResumeSlice`
  - `RunApprovalRejectedTerminalSlice`
- phase1 场景所需的快照组装逻辑也已一并下沉到 `appservice`
- 原 `application/workorchd/demo/snapshot.go` 已删除，避免 demo 包继续持有业务快照组装职责
- 原 `application/workorchd/demo/scenarios_test.go` 已迁移为 `application/workorchd/appservice/phase1_test.go`
- 原 `application/workorchd/demo/scenarios.go` 与 `demo` 包中的 `ensure*` 兼容性 wrapper 已删除
- `appservice` 当前已通过窄接口依赖运行时，而不再直接绑定 `*bootstrap.Runtime`
- `application/workorchd/demo/demo.go` 也已改为面向 `appservice.Runtime` 调用 `RunAllPhase1(...)`
- `README.md` 与 `docs/structure.md` 已同步新增 `appservice` 结构说明
- 已完成验证
  - `go test ./application/workorchd/appservice ./application/workorchd/demo -count=1`
  - `go test ./... -count=1`

### 17. env 入口继续下沉第四轮

- 已新增 `application/workorchd/appservice/entry.go`
  - 提供 `RunAllPhase1FromEnv(ctx)` 作为 phase1 的 env-backed 应用入口
  - 由 `appservice` 内部负责调用 `bootstrap.NewFromEnv()`
  - 由 `appservice` 内部负责在执行后调用 `Runtime.Shutdown(ctx)`
- 已新增 `application/workorchd/appservice/entry_test.go`
  - 覆盖 `file` backend 下的 env 入口执行与持久化文件落地
- `application/workorchd/main.go` 已直接委托 `appservice.RunAllPhase1FromEnv(...)`
  - `main` 不再通过 `demo` 间接触发 phase1 应用编排
- `application/workorchd/demo/demo.go` 已进一步收敛
  - 当前仅保留对 `appservice` 入口的兼容性转发
  - 已不再直接依赖 `bootstrap.NewFromEnv()`
- `README.md` 与 `docs/structure.md` 已同步修正
  - `appservice` 明确为 env-backed phase1 应用入口
  - `demo` 明确为 compatibility shim
- 本轮完成后，按环境构建 runtime 的职责已从 `demo` 继续下沉到 `appservice`

### 18. phase1 runtime adapter 下沉第五轮

- 已新增 `application/workorchd/appservice/runtime_adapter.go`
  - 在 `appservice` 内定义 phase1 专用 `RuntimeDependencies`
  - 在 `appservice` 内实现 `Runtime` 接口所需的读写桥接
  - 由 `appservice` 负责把 repository 读能力与模块 service 写能力拼成 phase1 应用运行时
- `application/workorchd/bootstrap.Runtime` 已继续收敛
  - 移除 phase1 专用 `Get/Create/Start/Resume/Resolve/...` 应用方法面
  - 当前仅保留装配后能力导出
    - `WorkspaceRepository()`
    - `RunRepository()`
    - `StepRepository()`
    - `DecisionTicketRepository()`
    - `ApprovalRepository()`
    - `WorkspaceService()`
    - `DecisionService()`
    - `ApprovalService()`
    - `RunService()`
    - `Shutdown(context.Context)`
- `application/workorchd/appservice/entry.go` 已改为
  - 先获取 `bootstrap.Runtime`
  - 再在 `appservice` 内构造 phase1 runtime adapter
  - 最后执行 `RunAllPhase1(...)`
- `application/workorchd/appservice/phase1_test.go` 与 `application/workorchd/demo/integration_test.go`
  - 已切换为通过 `appservice` runtime adapter 运行 phase1 流程
  - 不再要求 `bootstrap.Runtime` 直接充当 phase1 application facade
- `application/workorchd/bootstrap/runtime_test.go`
  - 已切换为通过 `WorkspaceService()` + `WorkspaceRepository()` 验证装配结果
- 本轮完成后，`bootstrap` 与 `appservice` 的边界进一步明确
  - `bootstrap` 负责 assemble
  - `appservice` 负责 application runtime 适配与业务 orchestration

### 19. assembled capability export 收口第六轮

- `application/workorchd/bootstrap.Runtime` 已继续从散列 getter 收口为单一聚合导出
  - 已新增 `bootstrap.Exports`
  - 已新增 `Runtime.Exports()`
  - `Exports()` 当前统一承接 repository/service 能力导出
- `bootstrap.Runtime` 已不再暴露以下散列 getter
  - `WorkspaceRepository()`
  - `RunRepository()`
  - `StepRepository()`
  - `DecisionTicketRepository()`
  - `ApprovalRepository()`
  - `WorkspaceService()`
  - `DecisionService()`
  - `ApprovalService()`
  - `RunService()`
- `application/workorchd/appservice/runtime_adapter.go` 已新增统一适配入口
  - `NewRuntimeFromBootstrap(*bootstrap.Runtime)`
  - `NewRuntimeFromExports(bootstrap.Exports)`
- `application/workorchd/appservice/entry.go`
  - 已切换为通过 `NewRuntimeFromBootstrap(...)` 构造 phase1 runtime
- `application/workorchd/appservice/phase1_test.go`
  - 已切换为复用统一 runtime 适配入口
- `application/workorchd/demo/demo_test.go` 与 `application/workorchd/demo/integration_test.go`
  - 已切换为复用统一 runtime 适配入口
  - 不再在测试中重复展开 repository/service 组装细节
- `application/workorchd/bootstrap/runtime_test.go`
  - 已切换为通过 `Exports()` 验证装配结果
- 本轮完成后，`bootstrap -> appservice` 的消费关系进一步收敛为
  - `bootstrap` 暴露单一 assembled capability export
  - `appservice` 负责把 export 适配到具体 application runtime

### 20. observed adapter 收口第七轮

- 已新增 `application/workorchd/observed`
  - 当前承接 phase1 acceptance snapshot 所需的 `observed` envelope 构造
  - 已新增 `observed/phase1_test.go` 作为独立单测落点
- `application/workorchd/presenter/presenter.go` 已继续收敛
  - `BuildSnapshot(...)` 不再手工构造 `events.Envelope`
  - `presenter` 当前只保留 snapshot view 映射与输出职责
  - 原 envelope 构造 helper 已从 `presenter` 删除
- `application/workorchd/appservice/phase1.go`
  - 已切换为通过 `observed.BuildPhase1Snapshot(...)` 组装 observed 输出
  - 再把 observed 结果传给 `presenter.BuildSnapshot(...)`
- 本轮完成后，`event / observed` 语义边界已开始收口为
  - `appservice` 负责 orchestration
  - `observed` 负责 acceptance observed adapter
  - `presenter` 负责渲染和 view model
- 当前阶段仍然只是 acceptance snapshot 语义
  - 尚未与真实 `event.Hub` 发布链路打通
  - 但已为后续把 observed adapter 替换为真实事件读取或订阅结果预留边界

### 21. observed 对运行时已发布事件接线第八轮

- `application/workorchd/bootstrap.Exports`
  - 已新增 `EventHub`
  - `bootstrap.Runtime` 当前除 repository/service 外，也会统一导出应用装配时使用的 `EventHub`
- `application/workorchd/appservice/runtime_adapter.go`
  - 已接入 `EventHub`
  - 已新增 `PublishedEvents()` 只读能力
  - 当前在 `eventHub` 为 `*events.MemoryHub` 时，可读取已发布 envelope 列表
- `application/workorchd/appservice/phase1.go`
  - 当前在 snapshot 组装阶段会优先探测 runtime 是否支持 `PublishedEvents()`
  - 如支持，则把已发布事件传给 `observed.BuildPhase1Snapshot(...)`
- `application/workorchd/observed/phase1.go`
  - 当前已改为“优先消费运行时已发布事件 + state fallback”的混合策略
  - `run` observed 当前优先从 `events.TopicRunStateChanged` 回放为 `/run/snapshot`
  - `step` observed 当前优先从 `events.TopicStepDispatched` / `events.TopicStepStateChanged` 回放为 `/step/snapshot`
  - `workspace / decision / approval` 当前仍使用 state-derived envelope
- `application/workorchd/observed/phase1_test.go`
  - 已新增 published runtime event 优先级验证
- 本轮完成后，phase1 observed 已开始真实消费 runtime 发布结果
  - 当前范围限定为 orchestrator 已发布的 `run/step`
  - 后续可继续把 `workspace / decision / approval` 也接入真实事件来源

### 22. workspace / decision / approval 事件接线第九轮

- `internal/modules/kernel/workspace/biz`
  - 已接入 `events.Hub`
  - `CreateWorkspace(...)` 当前会发布 `events.TopicWorkspaceCreated`
- `internal/modules/kernel/decision/biz`
  - 已接入 `events.Hub`
  - `CreateTicket(...)` 当前会发布 `events.TopicTicketCreated`
  - `ResolveTicket(...)` 当前会发布 `events.TopicTicketResolved`
- `internal/modules/blocks/approval/biz`
  - 已接入 `events.Hub`
  - `CreateApproval(...)` 当前会发布 `events.TopicApprovalCreated`
  - `Approve/Reject(...)` 当前会统一发布 `events.TopicApprovalResolved`
- `workspace / decision / approval` 三个模块的 `module.go`
  - 已把 `EventHub` 纳入模块依赖
  - `bootstrap/assembly.go` 已统一传入同一套应用级 `eventHub`
- `application/workorchd/observed/phase1.go`
  - 当前已继续扩大 published-event 优先范围
  - `workspace` 当前优先消费 `TopicWorkspaceCreated / TopicWorkspaceStateChanged`
  - `decision ticket` 当前优先消费 `TopicTicketCreated / TopicTicketResolved`
  - `approval` 当前优先消费 `TopicApprovalCreated / TopicApprovalResolved`
- `application/workorchd/observed/phase1_test.go`
  - 已新增 `workspace / decision / approval` published-event 优先级验证
- `internal/modules/kernel/workspace/biz/goal_dod_manager_test.go`
  - 已新增 workspace created 事件发布验证
- `internal/modules/kernel/decision/biz/decision_service_test.go`
  - 已新增 ticket created/resolved 事件发布验证
- `internal/modules/blocks/approval/biz/approval_service_test.go`
  - 已新增 approval created/resolved 事件发布验证
- 本轮完成后，phase1 acceptance observed 对五类对象中的四类已具备真实事件来源
  - `workspace`
  - `run`
  - `step`
  - `decision`
  - `approval`
  - 当前主要剩余问题已转为“workspace 的运行期 activeRun 变化是否也要由真实事件覆盖”

### 23. workspace 运行期状态事件覆盖第十轮

- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`
  - 已新增 `publishWorkspaceStateChanged(...)`
  - `StartRun(...)` 在占用 workspace 后会发布 `events.TopicWorkspaceStateChanged`
  - `CompleteRun(...)` 在清空 `activeRunId` 后会发布 `events.TopicWorkspaceStateChanged`
  - `FailRun(...)` 在清空 `activeRunId` 后也会发布 `events.TopicWorkspaceStateChanged`
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator_test.go`
  - 已补 `workspace` 运行期状态事件断言
  - 当前会校验 start/completed 路径上的 workspace 占用与释放事件
  - 当前也会校验 runtime failure 路径上的 workspace 释放事件
- `application/workorchd/observed/phase1_test.go`
  - 已新增 `workspace state changed` 优先级验证
  - 当前会优先取最新 `TopicWorkspaceStateChanged`，而不是早先 `TopicWorkspaceCreated`
- 本轮完成后，`workspace.activeRunId` 的运行期变化也已具备真实事件覆盖
  - phase1 observed 对 `workspace / run / step / decision / approval` 五类对象当前均可优先基于真实已发布事件生成 acceptance snapshot

### 24. observed 订阅式收集器第十一轮

- 已新增 `application/workorchd/observed/collector.go`
  - 当前通过 `EventHub.Subscribe(...)` 订阅 phase1 相关 topic
  - 当前会收集
    - `TopicWorkspaceCreated`
    - `TopicWorkspaceStateChanged`
    - `TopicRunStateChanged`
    - `TopicStepDispatched`
    - `TopicStepStateChanged`
    - `TopicTicketCreated`
    - `TopicTicketResolved`
    - `TopicApprovalCreated`
    - `TopicApprovalResolved`
  - 已提供 `Published()` 与 `Close()` 作为最小能力面
- 已新增 `application/workorchd/observed/collector_test.go`
  - 覆盖 collector 只收集订阅 topic
  - 覆盖 `Close()` 后停止继续收集
- `application/workorchd/appservice/runtime_adapter.go`
  - 已改为在构造时创建 `observed.Collector`
  - `PublishedEvents()` 当前返回 collector 中累积的 envelope
  - 当前不再直接依赖 `MemoryHub.Published()`
- 已新增 `application/workorchd/appservice/runtime_adapter_test.go`
  - 使用只实现 `Publish/Subscribe` 的自定义 `Hub`
  - 验证 `PublishedEvents()` 不依赖 `Published()` 扩展方法
- 本轮完成后，应用层 observed 已从 “读取特定 hub 实现内部 published 列表”
  - 收敛为 “通过 `Subscribe` 收集 phase1 相关事件”
  - 对具体 `Hub` 实现的耦合进一步降低
  - 也为后续把 collector 提升为真正的长期订阅组件预留了边界

### 25. collector 显式生命周期第十二轮

- `application/workorchd/appservice/runtime_adapter.go`
  - 已新增 `Close()`
  - 当前会显式关闭内部 `observed.Collector`
  - 关闭后会清空 collector 引用，避免重复继续收集
- `application/workorchd/appservice/entry.go`
  - `RunAllPhase1FromEnv(...)` 当前已在退出路径显式调用 `runtime.Close()`
  - 再调用 `bootstrapRuntime.Shutdown(ctx)`
- `application/workorchd/appservice/phase1_test.go`
  - 测试 helper 当前已通过 `t.Cleanup(runtime.Close)` 接入 collector 回收
- `application/workorchd/demo/demo_test.go` 与 `application/workorchd/demo/integration_test.go`
  - 已补 runtime `Close()` 调用
- `application/workorchd/appservice/runtime_adapter_test.go`
  - 已新增 `Close()` 后停止继续收集的验证
- 本轮完成后，collector 不再只是“存在关闭能力”
  - 已具备明确调用边界
  - 也使应用层 observed 生命周期比前一轮更完整

### 26. collector 生命周期外提第十三轮

- 已新增 `application/workorchd/appservice/runtime_managed.go`
  - 已引入 `ManagedRuntime`
  - 当前由 `ManagedRuntime` 显式持有
    - phase1 `Runtime`
    - `observed.Collector`
    - `PublishedEvents()`
    - `Close()`
- `application/workorchd/appservice/runtime_adapter.go`
  - 已去除 collector 持有与关闭职责
  - 当前只负责 repository/service 到 phase1 runtime 契约的业务适配
- `application/workorchd/appservice/entry.go`
  - `RunAllPhase1FromEnv(...)` 当前会先关闭 `ManagedRuntime`
  - 再执行 `bootstrapRuntime.Shutdown(ctx)`
- `application/workorchd/appservice/phase1_test.go`
  - helper 当前会显式回收 `ManagedRuntime`
- `application/workorchd/demo/demo_test.go` 与 `application/workorchd/demo/integration_test.go`
  - 已同步显式关闭 `ManagedRuntime`
- 本轮完成后，应用层 runtime 边界进一步明确为
  - `bootstrap.Runtime` 负责 assembled exports 与模块 shutdown
  - `ManagedRuntime` 负责 phase1 观察型运行时生命周期
  - `runtimeAdapter` 只保留业务读写桥接，不再混入订阅资源管理

### 27. phase1 观察契约映射收口第十四轮

- 已新增 `application/workorchd/observed/phase1_contract.go`
  - 当前显式定义 phase1 的 snapshot contract
    - `/workspace/snapshot`
    - `/run/snapshot`
    - `/step/snapshot`
    - `/decision/ticket/snapshot`
    - `/approval/snapshot`
  - 当前显式维护各 snapshot 对应的 published event topic 映射与 normalize 规则
- `application/workorchd/observed/phase1.go`
  - 已改为基于统一 contract 执行 published-event 优先选择与 fallback snapshot 组装
  - 不再在单文件中散落维护多套 `latest*SnapshotFromPublished(...)` helper
- `application/workorchd/observed/collector.go`
  - 已改为复用同一份 phase1 observed topic 清单
  - collector topic 订阅范围与 snapshot mapper contract 当前保持单源一致
- 本轮完成后，phase1 observed 当前进一步收敛为
  - contract 负责声明 “哪些 event topic 可回放成哪些 snapshot topic”
  - collector 负责按 contract 收集输入事件
  - `BuildPhase1Snapshot(...)` 负责对象态 fallback 与 contract 驱动的最终组装
  - 后续若继续扩 phase1 观察对象或迁移到更稳定的 event contract，改动面已集中

### 28. phase1 payload 契约统一第十五轮

- 已新增 `internal/pkg/events/phase1_payload.go`
  - 当前统一维护 phase1 事件 payload key 常量
  - 当前统一维护以下 payload builder
    - `WorkspacePayload(...)`
    - `RunPayload(...)`
    - `StepPayload(...)`
    - `DecisionTicketPayload(...)`
    - `DecisionResolutionPayload(...)`
    - `ApprovalPayload(...)`
- 已新增 `internal/pkg/events/phase1_payload_test.go`
  - 覆盖 workspace payload 字段输出
  - 覆盖 step payload 对 input/output 的浅拷贝保护
  - 覆盖 decision resolution payload 的扩展字段
- `internal/modules/kernel/workspace/biz/goal_dod_manager.go`
  - 已切换为通过 shared payload builder 发布 workspace created 事件
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`
  - 已切换为通过 shared payload builder 发布 run/workspace/step 状态事件
- `internal/modules/kernel/decision/biz/decision_service.go`
  - 已切换为通过 shared payload builder 发布 ticket created/resolved 事件
- `internal/modules/blocks/approval/biz/approval_service.go`
  - 已切换为通过 shared payload builder 发布 approval created/resolved 事件
- `application/workorchd/observed/phase1.go`
  - fallback snapshot payload 当前已复用同一套 shared builder
- 相关单测断言当前也已开始切换为 shared payload key 常量
- 本轮完成后，phase1 事件与 acceptance snapshot 之间除 topic contract 外，payload key 也已有单点定义
  - 后续若继续把 phase1 contract 往 typed event contract 提升，payload 变更面已进一步集中

### 29. phase1 payload decode/validate 第十六轮

- `internal/pkg/events/phase1_payload.go`
  - 已新增 phase1 payload typed decode 能力
    - `DecodeWorkspacePayload(...)`
    - `DecodeRunPayload(...)`
    - `DecodeStepPayload(...)`
    - `DecodeDecisionTicketPayload(...)`
    - `DecodeDecisionResolutionPayload(...)`
    - `DecodeApprovalPayload(...)`
  - 当前会对 phase1 payload 的关键字段执行最小类型校验
  - `step.input/output` 与 `decision resolution payload` 当前也会做 map 形态校验和拷贝保护
- `internal/pkg/events/phase1_payload_test.go`
  - 已补 decode round-trip 与非法 payload 拒绝测试
- `application/workorchd/observed/phase1_contract.go`
  - 当前在 published event -> snapshot normalize 过程中已接入 typed decode/validate
  - 对无法解码的 published payload，当前不会直接透传到 acceptance snapshot
  - 会继续回退到对象态 fallback snapshot
- `application/workorchd/observed/phase1_test.go`
  - 已补 published payload 非法时回退到 state-derived snapshot 的验证
- 本轮完成后，phase1 observed 当前已不再无条件信任 published event 的原始 `map[string]any`
  - topic contract、payload key、payload decode/validate 三层边界都已有集中化落点

### 30. phase1 typed event observer 第十七轮

- 已新增 `internal/pkg/events/phase1_event.go`
  - 当前显式定义 `Phase1Event` typed event contract
  - 已补 `WorkspaceEvent / RunEvent / StepEvent / DecisionTicketEvent / ApprovalEvent`
  - 已补 `DecodePhase1Event(...)`
  - 已补 `NormalizePhase1Envelope(...)`
  - 已补 `Phase1Topics()`
  - 已补 `SubscribePhase1(...)`
- 已新增 `internal/pkg/events/phase1_event_test.go`
  - 覆盖 typed event 解码
  - 覆盖 `SubscribePhase1(...)` 对非法 payload 的跳过策略
  - 覆盖 normalize 后 envelope 的标准化拷贝行为
- `application/workorchd/observed/collector.go`
  - 当前已切换为通过 `events.SubscribePhase1(...)` 收集 phase1 事件
  - collector 当前只接收已通过 typed decode/validate 的 phase1 event
- `application/workorchd/observed/collector_test.go`
  - 已补非法 phase1 payload 不进入 collector 的验证
- `application/workorchd/observed/phase1_contract.go`
  - snapshot normalize 当前也已切换为复用 `events.NormalizePhase1Envelope(...)`
  - `observed` 当前不再自行维护一套独立 payload normalize 逻辑
- 本轮完成后，phase1 event / observed 当前进一步收敛为
  - `internal/pkg/events`
    - topic 列表
    - payload builder
    - payload decode/validate
    - typed event envelope
    - observer subscription helper
  - `application/workorchd/observed`
    - 只负责 phase1 snapshot contract 与 acceptance 组装

### 31. phase1 typed snapshot contract 第十八轮

- 已新增 `application/workorchd/observed/snapshot_contract.go`
  - 当前显式定义 `SnapshotItem`
  - 当前显式定义 typed snapshot payload
    - `WorkspacePayload`
    - `RunPayload`
    - `StepPayload`
    - `DecisionTicketPayload`
    - `ApprovalPayload`
  - 已补 `FromEnvelope(...)`
  - 已补 `SnapshotItem.Envelope()` 作为兼容映射出口
- 已新增 `application/workorchd/observed/snapshot_contract_test.go`
  - 覆盖 decision ticket snapshot resolution 字段 round-trip
  - 覆盖非法 snapshot payload 拒绝
- `application/workorchd/observed/phase1.go`
  - `BuildPhase1Snapshot(...)` 当前已改为返回 typed `[]SnapshotItem`
  - fallback snapshot 当前也会先转成 typed snapshot item，而不是直接对外暴露裸 `events.Envelope`
- `application/workorchd/presenter/presenter.go`
  - 当前已改为消费 `[]observed.SnapshotItem`
  - `presenter.Snapshot.Observed` 当前已切换为自身的 `ObservedView`
  - 仍保持最终 JSON 输出字段形态与原 envelope 风格兼容
- 已新增 `application/workorchd/presenter/presenter_test.go`
  - 覆盖 typed observed item 到最终 JSON-compatible view 的映射
- `application/workorchd/appservice/phase1.go`
  - 当前继续由 `observed.BuildPhase1Snapshot(...)` 返回 typed snapshot item
  - 再由 `presenter` 负责最终渲染映射
- 本轮完成后，phase1 acceptance 输出边界进一步明确为
  - `events` 负责 typed event
  - `observed` 负责 typed snapshot
  - `presenter` 负责 JSON-compatible view/render

### 32. phase1 version compatibility 第十九轮

- 已新增 `internal/pkg/events/phase1_version.go`
  - 当前显式定义
    - `Phase1EventVersion`
    - `Phase1SnapshotVersion`
  - 当前显式定义
    - `IsCompatiblePhase1EventVersion(...)`
    - `IsCompatiblePhase1SnapshotVersion(...)`
- `internal/pkg/events/phase1_event.go`
  - typed phase1 event decode 当前已接入 event version compatibility 校验
  - 当前接受 legacy `v1` 并标准化到 `Phase1EventVersion`
  - 对不兼容版本当前会直接拒绝解码
- `application/workorchd/observed/snapshot_contract.go`
  - typed snapshot decode 当前已接入 snapshot version compatibility 校验
  - 当前会把兼容 snapshot 输入统一标准化到 `Phase1SnapshotVersion`
- `application/workorchd/observed/phase1_contract.go`
  - event -> snapshot normalize 当前会显式把 snapshot 版本收敛为 `Phase1SnapshotVersion`
- `application/workorchd/observed/phase1.go`
  - fallback snapshot 当前也已统一复用 `Phase1SnapshotVersion`
- `internal/modules/kernel/workspace/biz/goal_dod_manager.go`
  - 当前发布 workspace phase1 事件时已显式写入 `Phase1EventVersion`
- `internal/modules/kernel/decision/biz/decision_service.go`
  - 当前发布 decision phase1 事件时已显式写入 `Phase1EventVersion`
- `internal/modules/blocks/approval/biz/approval_service.go`
  - 当前发布 approval phase1 事件时已显式写入 `Phase1EventVersion`
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`
  - 当前发布 run/workspace/step phase1 事件时已显式写入 `Phase1EventVersion`
- 已补测试
  - `internal/pkg/events/phase1_event_test.go`
    - 覆盖 legacy version 标准化
    - 覆盖不兼容 version 拒绝
  - `application/workorchd/observed/snapshot_contract_test.go`
    - 覆盖 snapshot version 标准化与不兼容 version 拒绝
  - 各模块事件发布测试当前已开始断言 `Phase1EventVersion`
- 本轮完成后，phase1 event / snapshot 合同当前已具备显式 versioning / compatibility 校验
  - 不再依赖默认 `v1` 或隐式约定维持兼容

### 33. phase1 compatibility matrix 第二十轮

- `internal/pkg/events/phase1_version.go`
  - 当前已从全局 version 判断继续收敛为 topic-aware compatibility matrix
  - 已新增
    - `Phase1ContractSpec`
    - `Phase1EventContract(...)`
    - `Phase1SnapshotContract(...)`
    - `NormalizePhase1EventVersion(...)`
    - `NormalizePhase1SnapshotVersion(...)`
  - 当前 `IsCompatible...` 系列 helper 已转为复用 normalize + matrix，而不是单纯比较全局常量
- `internal/pkg/events/phase1_version_test.go`
  - 已补 event/snapshot topic matrix 标准化验证
  - 已补 unknown topic 拒绝验证
- `internal/pkg/events/phase1_event.go`
  - typed phase1 event decode 当前已改为按 topic 走显式 compatibility matrix
  - 后续若单个 topic 独立演进 version，不再需要改全局分支判断
- `application/workorchd/observed/snapshot_contract.go`
  - typed snapshot decode 当前也已按 snapshot topic 走显式 compatibility matrix
- 本轮完成后，phase1 versioning 当前进一步收敛为
  - 常量
  - compatibility rule
  - topic matrix
  - normalize helper
  - typed contract 消费方统一复用

### 34. phase1 最终契约收紧第二十一轮

- 按当前阶段“直接按最终实现目标处理”的原则，phase1 contract 当前已移除兼容分支
- `internal/pkg/events/phase1_version.go`
  - 已移除 legacy `v1` / 空版本兼容接受逻辑
  - 当前仅接受显式最终版本
    - `Phase1EventVersion`
    - `Phase1SnapshotVersion`
  - 当前 `Phase1EventContract(...)` / `Phase1SnapshotContract(...)` 仅承接 topic 到当前版本的显式映射
  - 当前 `ValidatePhase1EventVersion(...)` / `ValidatePhase1SnapshotVersion(...)` 只执行严格版本校验
- `internal/pkg/events/phase1_event.go`
  - typed phase1 event decode 当前已改为仅接受最终 event version
  - 不再对旧版本或缺省版本做标准化兼容
- `application/workorchd/observed/snapshot_contract.go`
  - typed snapshot decode 当前已改为仅接受最终 snapshot version
- `application/workorchd/observed/phase1_test.go`
  - published event 测试样例当前已统一显式携带最终 `Phase1EventVersion`
- `application/workorchd/observed/collector_test.go` 与 `application/workorchd/appservice/runtime_adapter_test.go`
  - collector/runtime 相关测试输入当前已统一切换到最终版本输入
- `application/workorchd/presenter/presenter.go`
  - `ObservedView` 的 JSON tag 当前已切换为最终 lower-camel-case 输出
  - 不再保留 envelope-style 大写字段 tag 兼容层
- 本轮完成后，phase1 event / snapshot / observed 输出当前已统一进入最终收口模式
  - 代码中不再继续为历史输入或历史输出形态保留兼容处理

### 35. bootstrap assembled runtime 壳层收口第二十二轮

- `application/workorchd/bootstrap/runtime.go`
  - `bootstrap.Runtime` 当前已进一步收敛为真正的 assembled runtime 壳层
  - 当前只显式持有
    - `Exports`
    - module lifecycle 列表
  - 已不再在 `Runtime` 内部分散维护一套 repo/service/eventHub 字段
- `application/workorchd/bootstrap/assembly.go`
  - 当前在装配完成后会一次性构造 `Exports`
  - 再把 `Exports + modules` 组装进 `bootstrap.Runtime`
- `application/workorchd/appservice/runtime_managed.go`
  - 继续通过 `bootstrap.Runtime.Exports()` 消费装配结果
  - 当前不受 `bootstrap` 内部字段收口方式影响
- 本轮完成后，`bootstrap` 当前边界进一步明确为
  - `assembly.go` 负责组装 `Exports`
  - `Runtime` 负责承载 `Exports()` 与 `Shutdown(...)`
  - `appservice` 负责消费 `Exports` 做 application runtime 适配

### 36. bootstrap capability group 分组收口第二十三轮

- `application/workorchd/bootstrap/runtime.go`
  - `bootstrap.Exports` 当前已从扁平字段集合进一步收敛为显式 capability group
  - 当前分为
    - `Repositories`
    - `Services`
    - `Infrastructure`
- `application/workorchd/bootstrap/assembly.go`
  - 当前在装配完成后按 capability group 一次性构造 `Exports`
  - repository / service / event hub 不再作为同级扁平字段暴露
- `application/workorchd/appservice/runtime_adapter.go`
  - `RuntimeDependencies` 当前也已对齐为分组依赖
  - phase1 runtime adapter 继续只消费应用层所需的 repository/service 能力，不直接感知装配细节
- `application/workorchd/appservice/runtime_managed.go`
  - 当前通过 `bootstrap.Exports.Repositories / Services / Infrastructure` 显式消费装配结果
  - collector 仍只从 `Infrastructure.EventHub` 接入，不再混在业务依赖字段中
- 本轮完成后，`bootstrap -> appservice` 的 application 边界进一步明确为
  - `bootstrap` 输出 capability group
  - `appservice` 按 group 做 phase1 runtime 适配
  - repository / service / event hub 的职责分界比上一轮更清晰

### 37. workspace 写路径回收第二十四轮

- `internal/modules/kernel/workspace/service/service.go`
  - `workspace` 模块当前已新增 `EnsureWorkspace` service 面
  - 当前由模块自身承接 “缺失时创建 / 已存在时恢复到 active 基线” 的写语义
- `internal/modules/kernel/workspace/biz/goal_dod_manager.go`
  - 当前已把 workspace 基线恢复逻辑收回模块 biz
  - 当 title/state/activeRunId 发生收敛时，会发布 `TopicWorkspaceStateChanged`
- `application/workorchd/appservice/phase1.go`
  - `Phase1Service.EnsureWorkspace(...)` 当前已直接委托 runtime `EnsureWorkspace(...)`
  - `appservice` 当前不再直接持有 workspace repository 的写路径
- `application/workorchd/appservice/runtime_adapter.go`
  - 当前也已移除 `UpdateWorkspace` / `CreateWorkspace` 这类 application 侧 workspace 写桥接
  - workspace 写动作现统一经 `workspace` 模块 service 进入
- `application/workorchd/appservice/phase1_test.go`
  - 已补 workspace stale state/title/activeRunId 被 service 路径收敛的回归测试
- 本轮完成后，phase1 runtime 当前进一步对齐为
  - repository 主要承担读侧
  - service 主要承接写侧
  - `appservice` 不再绕过 `workspace` 模块直接改 workspace 状态

### 38. appservice runtime 读写分面第二十五轮

- `application/workorchd/appservice/phase1.go`
  - `appservice` 当前已把 phase1 runtime 从单一扁平 `Runtime` 拆为
    - `ReadRuntime`
    - `ActionRuntime`
  - `Runtime` 当前仅作为两者的组合接口保留
- `application/workorchd/appservice/phase1.go`
  - `Phase1Service` 当前不再持有单一 `runtime` 字段
  - 已改为显式持有
    - `reads`
    - `actions`
- `application/workorchd/appservice/phase1.go`
  - snapshot / binding 校验等路径当前统一走 `reads`
  - 创建、启动、阻塞、恢复、审批、决策等动作当前统一走 `actions`
- 本轮完成后，`appservice` 当前边界进一步明确为
  - phase1 读模型与写动作在应用层也已显式分面
  - `runtimeAdapter` 虽仍承载同一实现对象，但应用编排代码已不再依赖“大而平”的 runtime 接口

### 39. appservice 写侧业务动作分组第二十六轮

- `application/workorchd/appservice/phase1.go`
  - `ActionRuntime` 当前已继续拆成按业务模块分组的动作接口
    - `WorkspaceActionRuntime`
    - `RunActionRuntime`
    - `DecisionActionRuntime`
    - `ApprovalActionRuntime`
- `application/workorchd/appservice/phase1.go`
  - `Phase1Service` 当前也已从单一 `actions` 字段改为显式持有各动作分组
  - workspace/run/decision/approval 的动作调用边界已可直接从字段面看清
- `application/workorchd/appservice/phase1.go`
  - `EnsureWorkspace` 当前只依赖 `workspaceActions`
  - run 启停/阻塞/恢复当前只依赖 `runActions`
  - ticket 创建/resolve 当前只依赖 `decisionActions`
  - approval 创建/approve/reject 当前只依赖 `approvalActions`
- 本轮完成后，`appservice` 的 phase1 写侧当前进一步收口为
  - 读侧按查询面表达
  - 写侧按业务动作组表达
  - application 编排代码与底层实现对象的耦合继续下降

### 40. appservice 读侧用途分组第二十七轮

- `application/workorchd/appservice/phase1.go`
  - `ReadRuntime` 当前已继续拆成按用途分组的读接口
    - `GuardReadRuntime`
    - `SnapshotReadRuntime`
- `application/workorchd/appservice/phase1.go`
  - `Phase1Service` 当前也已把读依赖从单一 `reads` 字段改为
    - `guardReads`
    - `snapshotReads`
- `application/workorchd/appservice/phase1.go`
  - run/ticket/approval 的前置校验、绑定检查、终态判断当前统一走 `guardReads`
  - workspace/run/step/ticket/approval 的最终快照组装与 observed 收集当前统一走 `snapshotReads`
- 本轮完成后，`appservice` 的 phase1 运行时当前进一步收口为
  - 读侧按用途表达
  - 写侧按业务动作组表达
  - application 编排层对运行时依赖的语义比上一轮更明确

### 41. runtime adapter 能力依赖收口第二十八轮

- `application/workorchd/appservice/runtime_adapter.go`
  - `RuntimeDependencies` 当前已不再使用扁平的 `Repositories / Services` 输入
  - 当前已改为
    - `Reads`
    - `Actions`
- `application/workorchd/appservice/runtime_adapter.go`
  - 读侧依赖当前已进一步分成
    - `RuntimeGuardDependencies`
    - `RuntimeSnapshotDependencies`
  - 写侧依赖当前已进一步分成
    - `RuntimeWorkspaceActionDependencies`
    - `RuntimeRunActionDependencies`
    - `RuntimeDecisionActionDependencies`
    - `RuntimeApprovalActionDependencies`
- `application/workorchd/appservice/runtime_adapter.go`
  - `runtimeAdapter` 当前内部也已不再保留 repo/service 平铺字段
  - 当前统一显式持有
    - `reads`
    - `actions`
- `application/workorchd/appservice/runtime_managed.go`
  - 当前也已按 `Reads / Actions` 结构装配 `runtimeAdapter`
- 本轮完成后，`appservice` 当前从 orchestration 到 adapter 的依赖表达已进一步对齐
  - phase1 service 用途分组
  - runtime adapter capability 分组
  - bootstrap exports group

### 42. runtime dependencies 组装辅助函数第二十九轮

- `application/workorchd/appservice/runtime_dependencies.go`
  - 当前已新增 `RuntimeDependenciesFromExports(...)`
  - `bootstrap.Exports -> appservice.RuntimeDependencies` 的 capability 映射已收拢到单点辅助函数
- `application/workorchd/appservice/runtime_managed.go`
  - `NewRuntimeFromExports(...)` 当前已不再手工展开整套 dependencies 映射
  - 当前只负责串联
    - `RuntimeDependenciesFromExports(...)`
    - `NewRuntimeAdapter(...)`
    - `NewManagedRuntime(...)`
- `application/workorchd/appservice/runtime_dependencies_test.go`
  - 已补聚焦测试，验证 grouped exports 会被正确映射到 grouped runtime dependencies
- 本轮完成后，`appservice` 当前入口组装进一步收口为
  - exports 映射单点维护
  - managed runtime 入口职责更薄
  - 后续 capability group 再调整时只需收敛一处映射代码

### 43. managed runtime assembly helper 第三十轮

- `application/workorchd/appservice/runtime_assembly.go`
  - 当前已新增 managed runtime assembly helper
  - 负责统一组装
    - runtime adapter
    - observed collector
    - managed runtime 壳层
- `application/workorchd/appservice/runtime_managed.go`
  - `NewRuntimeFromExports(...)` 当前已切换为通过 assembly helper 构造
  - `NewManagedRuntime(...)` 当前也已统一委托到 assembly helper
  - 入口文件当前只保留 managed runtime 壳层与显式生命周期方法
- `application/workorchd/appservice/runtime_assembly_test.go`
  - 已补聚焦测试，验证 assembly helper 会同时接好 runtime 与 collector
- 本轮完成后，`appservice` 当前 runtime 入口进一步收口为
  - exports -> dependencies helper
  - dependencies -> runtime/collector assembly helper
  - managed runtime entry 只保留最薄串联职责

### 44. runtime session 生命周期模式统一第三十一轮

- `application/workorchd/appservice/runtime_session.go`
  - 当前已新增 `RuntimeSession`
  - 统一承接
    - `bootstrap.Runtime`
    - `ManagedRuntime`
    - `Close(ctx)` 回收路径
- `application/workorchd/appservice/entry.go`
  - `RunAllPhase1FromEnv(...)` 当前已切换为直接使用 `RuntimeSession`
  - env 入口不再手工维护 `bootstrap + managed runtime` 的分散回收逻辑
- `application/workorchd/appservice/phase1_test.go`
  - `application/workorchd/demo/demo_test.go`
  - `application/workorchd/demo/integration_test.go`
  - `application/workorchd/appservice/runtime_*_test.go`
  - 上述同类测试当前也已统一切换到 `RuntimeSession` pattern
- `application/workorchd/appservice/runtime_session_test.go`
  - 已补 runtime session 自测，验证 close 后 managed/bootstrap runtime 均被释放，且可重复 close
- 本轮完成后，`appservice` 当前关于 runtime 生命周期的同类模式已进一步统一为
  - session 装配
  - session 回收
  - 测试与入口同一套处理方案

### 45. phase1 执行 helper 模式统一第三十二轮

- `application/workorchd/appservice/entry.go`
  - 当前已新增
    - `RunAllPhase1WithSession(...)`
    - `RunAllPhase1WithBackend(...)`
  - `RunAllPhase1FromEnv(...)` 当前也已改为复用统一执行 helper
- `application/workorchd/demo/demo.go`
  - 当前已新增 `RunAllWithBackend(...)` 作为 demo 侧的薄转发
  - 原先测试专用的 `runAll(runtime)` 模式已不再保留
- `application/workorchd/demo/demo_test.go`
  - `application/workorchd/demo/integration_test.go`
  - 当前已统一改为通过 `RunAllWithBackend(...)` 执行 phase1
  - 不再各自手工拼 `session + runtime + runAll`
- 本轮完成后，`appservice/demo` 当前同类执行模式已统一为
  - 入口：`RunAllPhase1FromEnv(...)`
  - 指定后端：`RunAllPhase1WithBackend(...)`
  - demo 薄转发：`RunAllWithBackend(...)`

### 46. 测试 helper 模式统一第三十三轮

- `application/workorchd/appservice/test_helpers_test.go`
  - 当前已新增 `newTestRuntimeSession(...)`
  - `appservice` 测试侧的 session setup/cleanup 当前已统一走单点 helper
- `application/workorchd/appservice/phase1_test.go`
  - `application/workorchd/appservice/runtime_dependencies_test.go`
  - `application/workorchd/appservice/runtime_assembly_test.go`
  - 上述测试当前已切换为复用 `newTestRuntimeSession(...)`
- `application/workorchd/demo/test_helpers_test.go`
  - 当前已新增 `mustRunAllWithBackend(...)`
  - `demo` 测试侧的 “指定 backend 执行 phase1 并断言” 当前已统一走单点 helper
- `application/workorchd/demo/demo_test.go`
  - `application/workorchd/demo/integration_test.go`
  - 当前已切换为复用 `mustRunAllWithBackend(...)`
- 本轮完成后，测试侧当前也已进一步收口为
  - appservice：统一 session helper
  - demo：统一 run helper
  - 同类 setup/execute pattern 不再在多个测试文件里重复散落

### 47. file backend 重载稳定性回归第三十四轮

- `application/workorchd/appservice/entry_test.go`
  - 已新增 `RunAllPhase1FromEnv(...)` 的 file backend 重载回归测试
  - 当前会连续两次基于同一 `WORKORCH_DATA_DIR` 执行 env 入口
  - 当前会校验 phase1 四条 slice 的标题、核心快照对象与 observed topic 集合在重载后保持稳定
- `application/workorchd/demo/integration_test.go`
  - 已新增 demo 薄转发层的 file backend 重复运行稳定性回归测试
  - 当前会在同一持久化目录上执行两轮 `RunAllWithBackend(...)`
  - 当前会校验 workspace/run/step/blocking 业务快照在重复运行后保持一致
  - 当前也会继续复用既有跨模块集成断言，确认第二轮执行后的业务终态仍满足 phase1 基线
- `application/workorchd/demo/integration_test.go`
  - 已去掉仅作一次转发的 `runAllSlices(...)` 测试包装
  - `integrationBackends(...)` 当前直接复用统一 `mustRunAllWithBackend(...)` helper
- 本轮完成后，`appservice/demo` 当前已补齐一轮 “持久化后重载 + 对同一数据目录重复运行” 的稳定性回归
  - 第 3 项遗留已从“重载稳定性”收窄为更细的异常分支补测

### 48. application action port 收口第三十五轮

- `application/workorchd/appservice/runtime_adapter.go`
  - 已移除对 `workspace/run/decision/approval` 模块 command DTO 的直接拼装
  - 当前 runtime adapter 写侧已只依赖 `appservice` 本地 action port
- 已新增 `application/workorchd/appservice/runtime_action_ports.go`
  - 当前集中承接 `module service -> appservice action port` 的命令适配
  - 模块 command DTO 的组装细节已从 application orchestration 主体中剥离到单点适配文件
- `application/workorchd/appservice/runtime_dependencies.go`
  - `RuntimeDependenciesFromExports(...)` 当前会直接把 `bootstrap.Exports.Services` 适配为本地 action port
  - `appservice` 与模块 `service` 契约的耦合已继续收束到 dependencies 装配路径
- `application/workorchd/appservice/runtime_dependencies_test.go`
  - 已新增 action port 驱动模块 service 的回归验证
  - 当前会校验通过 appservice action port 执行后，workspace/run/ticket/approval 均能正确落入底层 repository
- 本轮完成后，`appservice` 对模块层的依赖关系进一步明确为
  - 读侧：repository read model
  - 写侧：application local action port
  - module command 组装：单点 adapter
  - application 编排主体不再直接理解各模块 command DTO

### 49. 绑定异常分支补测第三十六轮

- `application/workorchd/appservice/phase1_test.go`
  - 已继续扩大 `EnsureDecisionTicket(...)` 的绑定异常覆盖
  - 当前除 `stepID` 不一致外，也已覆盖 `workspaceID` / `runID` 不一致冲突
- `application/workorchd/appservice/phase1_test.go`
  - 已继续扩大 `EnsureApproval(...)` 的绑定异常覆盖
  - 当前除 `stepID` 不一致外，也已覆盖 `workspaceID` / `runID` 不一致冲突
- 本轮完成后，第 3 项遗留中的 “更多 ticket/approval 绑定异常” 已继续收窄
  - 后续主要剩余重复恢复、终态幂等与其他异常组合分支

### 50. decision/approval ensure 语义下沉第三十七轮

- `internal/modules/kernel/decision/service`
  - 已新增 `EnsureTicket(...)` 与 `EnsureResolvedTicket(...)`
  - decision 模块当前已自行承接 ticket 创建幂等、绑定冲突与 resolved 幂等语义
- `internal/modules/blocks/approval/service`
  - 已新增 `EnsureApproval(...)` / `EnsureApproved(...)` / `EnsureRejected(...)`
  - approval 模块当前已自行承接 approval 创建幂等、绑定冲突与终态幂等/冲突语义
- `internal/modules/kernel/decision/biz/decision_service.go`
  - 已把 ticket 的 “存在即校验绑定，不存在则创建” 与 “resolved 即幂等” 能力补回模块 biz
- `internal/modules/blocks/approval/biz/approval_service.go`
  - 已把 approval 的 “存在即校验绑定，不存在则创建” 与 “approved/rejected ensure” 能力补回模块 biz
- `application/workorchd/appservice/phase1.go`
  - decision/approval 相关 `Ensure*` 当前已改为直接委托模块 action port
  - application 编排层已不再自己维护 ticket/approval 的 module-local guard 逻辑
- `application/workorchd/appservice/runtime_adapter.go`
  - guard read 面当前已继续缩窄到只保留 `Run`
  - `Ticket/Approval` 的 guard 查询已不再作为 application 编排前置依赖
- `application/workorchd/appservice/runtime_dependencies.go`
  - `RuntimeDependenciesFromExports(...)` 当前对 guard read 的消费已收缩，不再要求 ticket/approval guard 能力
- `internal/modules/kernel/decision/biz/decision_service_test.go`
  - 已新增 ensure ticket / ensure resolved 幂等与冲突回归
- `internal/modules/blocks/approval/biz/approval_service_test.go`
  - 已新增 ensure approval / ensure approved / ensure rejected 幂等与冲突回归
- 本轮完成后，第 1 项遗留已继续前移
  - decision / approval 的单模块幂等语义已从 `appservice` 下沉回模块 `service/biz`
  - `appservice` 更接近纯跨模块 orchestration

### 51. run ensure 语义下沉第三十八轮

- `internal/modules/kernel/run/service`
  - 已新增
    - `EnsureRun(...)`
    - `EnsureStarted(...)`
    - `EnsureBlockedOnDecision(...)`
    - `EnsureBlockedOnApproval(...)`
    - `EnsureResumed(...)`
  - run 模块当前已自行承接 run 创建、启动、阻塞、恢复的幂等与终态判断语义
- `internal/modules/kernel/run/biz/run_service.go`
  - 已把 run 的 workspace 绑定校验、completed/failed 终态幂等、blocked 状态判断收回模块 biz
  - application 层不再重复维护同一套 run guard 逻辑
- `internal/modules/kernel/run/biz/run_service_test.go`
  - 已新增 ensure run 幂等与 workspace 冲突回归
  - 已新增 ensure started / blocked / resumed 的终态与幂等回归
- `application/workorchd/appservice/phase1.go`
  - run 相关 `Ensure*` 当前已改为直接委托 run action port
  - `Phase1Service` 当前已不再持有独立 `guardReads`
- `application/workorchd/appservice/runtime_adapter.go`
  - runtime adapter 当前已不再区分 guard read repository
  - `GetRun(...)` 当前统一走 snapshot read
- `application/workorchd/appservice/runtime_dependencies.go`
  - `RuntimeDependencies` 当前已进一步收缩为
    - `Reads.Snapshot`
    - `Actions`
  - 原 `Reads.Guard` 已删除
- `application/workorchd/appservice/runtime_dependencies_test.go`
  - 已同步改为验证 snapshot read + action port 的收缩后依赖面
- 本轮完成后，第 1、2 项遗留继续同时前移
  - run 的单模块 ensure 语义已从 `appservice` 下沉回模块层
  - `appservice` 当前运行时读侧已从 `guard + snapshot` 收敛为纯 snapshot read

### 52. snapshot 依赖面压缩与重复 ensure 回归第三十九轮

- `application/workorchd/appservice/runtime_adapter.go`
  - `RuntimeDependencies` 当前已删除仅作转发的 `Reads` 包装层
  - `runtimeAdapter` 当前直接持有 `Snapshot + Actions`
- `application/workorchd/appservice/runtime_dependencies.go`
  - `RuntimeDependenciesFromExports(...)` 当前直接映射 `Snapshot` 读侧能力
  - appservice 运行时依赖表达已进一步扁平化
- `application/workorchd/appservice/runtime_dependencies_test.go`
  - 已同步校验收缩后的 `Snapshot + Actions` 依赖面
- `application/workorchd/appservice/phase1_test.go`
  - 已新增 `EnsureDecisionResolved(...)` 对已 resolved ticket 的幂等回归
  - 已新增 `EnsureApprovalApproved(...)` 对已 approved approval 的幂等回归
  - 已新增 `EnsureApprovalRejected(...)` 对已 rejected approval 的幂等回归
- 本轮完成后，第 2、3 项遗留继续收窄
  - `appservice` 内部运行时依赖形态更接近最终收口
  - 重复恢复 / 终态 ensure 的明显缺口继续减少

### 53. run 重复阻塞与终态幂等补测第四十轮

- `application/workorchd/appservice/phase1_test.go`
  - 已新增 `EnsureRunStarted(...)` 对 failed run 的冲突回归
  - 已新增 `EnsureRunResumed(...)` 对 completed run 的幂等回归
  - 已新增 `EnsureRunBlockedOnDecision(...)` 对 “同一 blocking 引用重复执行” 与 “completed 终态后再次 ensure” 的幂等回归
  - 已新增 `EnsureRunBlockedOnApproval(...)` 对 “同一 blocking 引用重复执行” 与 “completed 终态后再次 ensure” 的幂等回归
- 本轮完成后，第 3 项遗留中的 run 侧重复恢复 / 终态幂等缺口继续收窄
  - appservice 关于 run 主流程的重复执行边界当前已更接近 phase1 最终收口

### 54. bootstrap service 能力面评估第四十一轮

- 已复核 `bootstrap.Exports.Services` 的实际消费面
  - 当前只有 `appservice/runtime_dependencies.go` 负责把四个模块 service 适配为本地 action port
  - `bootstrap/runtime_test.go` 仅保留最小装配验证使用
- 当前评估结论为
  - 在不让 `bootstrap` 反向感知 `appservice` port 的前提下
  - `WorkspaceService / RunService / DecisionService / ApprovalService` 已是当前最小稳定模块 façade 集合
  - 继续压缩 `Services` 会把 application 专用概念反推回 bootstrap，边界更差
- 本轮完成后，第 2 项遗留已从“继续缩面”转为“保持当前稳定面，除非新增模块或真实应用入口变化”

### 55. phase1 typed event/observed 收口第四十二轮

- `application/workorchd/observed/collector.go`
  - collector 内部已从保存 `[]events.Envelope` 改为保存 `[]events.Phase1Event`
  - 对外已改为暴露 `PublishedPhase1Events()`
  - collector 当前不再在订阅入口把 typed event 立即降回原始 envelope
- `application/workorchd/appservice/runtime_managed.go`
  - `ManagedRuntime` 当前已对外暴露 `PublishedPhase1Events()`
- `application/workorchd/appservice/phase1.go`
  - phase1 snapshot 组装阶段当前已直接消费 typed phase1 event 列表
- `application/workorchd/observed/phase1_contract.go`
  - phase1 event-to-snapshot contract 当前已改为直接接受 `events.Phase1Event`
  - `workspace / run / step / decision / approval` 的 snapshot normalize 当前已直接基于 typed event data 生成 payload
  - observed 侧当前只在最终 snapshot 归一化时落回 `events.Envelope`
- `application/workorchd/observed/phase1.go`
  - `BuildPhase1Snapshot(...)` 当前已切换为 typed phase1 event 输入
- `application/workorchd/observed/collector_test.go`
  - 已同步验证 collector 暴露 typed event
- `application/workorchd/appservice/runtime_adapter_test.go`
  - 已同步验证 managed runtime 暴露 typed event
- `application/workorchd/appservice/runtime_assembly_test.go`
  - 已同步验证 collector 接线后的 typed event 收集
- `application/workorchd/observed/phase1_test.go`
  - 已同步切换为 typed phase1 event 构造与回归断言
- 本轮完成后，第 4 项遗留继续收窄
  - collector -> managed runtime -> observed phase1 snapshot 这条链路当前已不再以裸 `events.Envelope` 作为内部主语义
  - phase1 observed 当前更接近“typed event -> typed snapshot -> presenter view”的最终目标

### 56. typed snapshot 渲染链路收口第四十三轮

- `application/workorchd/observed/phase1_contract.go`
  - phase1 event normalize 当前已直接生成 `SnapshotItem`
  - `fallback(...)` 当前也已直接返回 typed snapshot item
- `application/workorchd/observed/phase1.go`
  - phase1 fallback snapshot 当前不再走 `Envelope -> FromEnvelope(...)` 的中转路径
  - 内部 `mustSnapshotItem(...)` 已删除
- `application/workorchd/observed/snapshot_contract.go`
  - 已新增 `SnapshotItem.PayloadMap()`
- `application/workorchd/presenter/presenter.go`
  - `MapObserved(...)` 当前已直接消费 `SnapshotItem` 字段与 payload map
  - presenter 渲染阶段已不再把 typed snapshot item 重新回转成 `Envelope`
- 本轮完成后，第 4 项遗留继续收窄
  - phase1 observed 主链路当前已进一步收口为 `typed event -> typed snapshot -> presenter view`
  - `Envelope` 当前主要保留在 contract 校验与边界转换，而不再作为内部渲染中转对象

### 57. run blocking 引用冲突保护第四十四轮

- `internal/modules/kernel/run/biz/run_service.go`
  - `EnsureBlockedOnDecision(...)` 当前已补 “不同 decision ticket” 与 “已被 approval 阻塞” 的冲突保护
  - `EnsureBlockedOnApproval(...)` 当前已补 “不同 approval” 与 “已被 decision 阻塞” 的冲突保护
  - run 模块当前不再允许通过 ensure 语义静默覆盖既有 blocking 引用
- `internal/modules/kernel/run/biz/run_service_test.go`
  - 已新增 decision blocking 引用冲突回归
  - 已新增 approval blocking 引用冲突回归
- `application/workorchd/appservice/phase1_test.go`
  - 已新增 appservice 层对 decision/approval blocking 引用冲突的回归
  - 当前会校验冲突返回 `ErrConflict`，且底层 run 状态与 blocking 引用不被改写
- 本轮完成后，第 1、3 项遗留继续收窄
  - run 模块关于 blocking ensure 的单对象语义进一步下沉并稳定
  - phase1 业务异常分支当前继续减少“静默覆盖状态”的风险

### 58. appservice 恢复与缺失对象异常补测第四十五轮

- `application/workorchd/appservice/phase1_test.go`
  - 已新增 `approval pending` 下 `EnsureRunResumed(...)` 返回 `ErrBlocked` 的回归
  - 已新增 `EnsureDecisionResolved(...)` 对缺失 ticket 返回 `ErrNotFound` 的回归
  - 已新增 `EnsureApprovalApproved(...)` / `EnsureApprovalRejected(...)` 对缺失 approval 返回 `ErrNotFound` 的回归
- 本轮完成后，第 3 项遗留继续收窄
  - 应用层当前已把一组关键异常分支显式锁定到测试基线
  - 模块层既有的 not-found / blocked 语义当前已在 `appservice` 层得到透传验证

### 59. observed reload 稳定性与 resolved decision 回放第四十六轮

- `application/workorchd/bootstrap/runtime.go`
  - `RepositoryExports` 已补出 `Resolutions repository.DecisionResolutionRepository`
- `application/workorchd/bootstrap/assembly.go`
  - bootstrap exports 当前已把 `repos.resolution` 暴露给 application runtime
- `application/workorchd/appservice/runtime_dependencies.go`
  - snapshot 依赖当前已补齐 `Resolutions`
- `application/workorchd/appservice/runtime_adapter.go`
  - 已新增 `GetResolutionByTicketID(...)`
- `application/workorchd/appservice/phase1.go`
  - phase1 snapshot 读取当前在 `ticket.State == resolved` 时显式加载 resolution
  - `observed.BuildPhase1Snapshot(...)` 已改为接收 `resolution` 对象态
- `application/workorchd/observed/phase1.go`
  - resolved ticket 在无 published event 的 fallback 路径下，当前也会输出 `resolutionId/type/payload`
  - workspace published snapshot normalize 当前统一归一到 `application/workorchd` sourceModule，避免 reload 后来源字段漂移
- `application/workorchd/observed/phase1_test.go`
  - 已新增 fallback resolved ticket resolution 字段回归
- `application/workorchd/appservice/runtime_dependencies_test.go`
  - 已新增 resolutions 仓储映射与读取回归
- `application/workorchd/demo/integration_test.go`
  - 前一轮已提升 reload 稳定性断言强度；本轮在不放松断言的前提下补齐了真实语义缺口
- 本轮完成后，第 3、4 项遗留继续收窄
  - file backend reload 下的 observed payload 当前已可稳定重建 resolved decision 细节
  - typed `event -> snapshot -> presenter` 主链路在 reload 场景下进一步闭合

### 60. decision/approval ensure 语义防漂移第四十七轮

- `internal/modules/kernel/decision/biz/decision_service.go`
  - `EnsureTicket(...)` 当前已补 `reason` 一致性校验
  - 同一 ticket ID 下若 `workspace/run/step` 相同但 `reason` 不同，当前会返回 `ErrConflict`
  - `EnsureResolvedTicket(...)` 当前在 ticket 已 resolved 时，会继续校验已落库 resolution 的 `id/type/payload`
  - 同一 ticket 的第二次 ensure resolve 若 resolution 语义不一致，当前不再静默返回成功，而是返回 `ErrConflict`
- `internal/modules/blocks/approval/biz/approval_service.go`
  - `EnsureApproval(...)` 当前已补 `reason` 一致性校验
  - 同一 approval ID 下若绑定一致但 `reason` 不同，当前会返回 `ErrConflict`
- `internal/modules/kernel/decision/biz/decision_service_test.go`
  - 已新增 ticket `reason` 不一致冲突回归
  - 已把 ensure resolved 回归扩展为 “相同 resolution 幂等、不同 resolution 冲突”
- `internal/modules/blocks/approval/biz/approval_service_test.go`
  - 已新增 approval `reason` 不一致冲突回归
- `application/workorchd/appservice/phase1_test.go`
  - 已新增 `EnsureDecisionResolved(...)` 对已 resolved 但 resolution 不一致时返回 `ErrConflict` 的回归
  - 已新增 `EnsureDecisionTicket(...)` / `EnsureApproval(...)` 对 `reason` 不一致时返回 `ErrConflict` 的回归
- 本轮完成后，第 1、3 项遗留继续收窄
  - decision / approval 的 ensure 语义当前进一步对齐为“同 ID 同语义才幂等”
  - phase1 应用层当前继续减少同类对象被静默错配或静默吞错的风险

### 61. run ensure 语义防漂移第四十八轮

- `internal/modules/kernel/run/biz/run_service.go`
  - `EnsureRun(...)` 当前已从仅校验 `workspaceID`
  - 继续收紧为同时校验 `workspaceID/goalID/doDID`
  - 同一 `runID` 下若 `goalID` 或 `doDID` 漂移，当前会返回 `ErrConflict`
- `internal/modules/kernel/run/biz/run_service_test.go`
  - 已把 ensure run 回归从 “workspace mismatch” 扩展为 “semantic mismatch”
  - 已新增 `goalID` 不一致冲突回归
  - 已新增 `doDID` 不一致冲突回归
- 本轮完成后，第 1 项遗留继续收窄
  - run 模块当前也进一步对齐为“同 ID 同语义才幂等”
  - `CreateRunCommand` 中已存在的领域字段当前不再在 ensure 路径被静默忽略

### 62. orchestrator 原始动作状态约束第四十九轮

- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`
  - `StartRun(...)` 当前已补状态约束
  - 当前只允许从 `created` 状态进入启动链路
  - 对 `running/blocked/completed/failed` 等非创建态再次直接 `StartRun(...)`，当前会返回 `ErrConflict`
  - `BlockOnDecision(...)` / `BlockOnApproval(...)` 当前也已补状态约束
  - 当前仅允许对 `created/running` run 施加 blocking
  - 对已 blocked 或 terminal run，不再允许通过原始动作面直接覆盖 blocking 状态
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator_test.go`
  - 已新增 `StartRun(...)` 对非 `created` run 返回 `ErrConflict` 的回归
  - 已把 block 命令测试从“清空旧 opposite refs”收口为“正常设置 refs + 非法状态冲突”
  - 已新增 blocked/terminal run 上直接 block 返回 `ErrConflict` 的回归
- 本轮完成后，第 1 项遗留继续收窄
  - orchestrator 底层动作面当前不再静默覆盖既有状态
  - 上层 `run service ensure` 与底层 orchestrator 原始动作的状态边界当前更加一致

### 63. appservice 入口空会话保护第五十轮

- `application/workorchd/appservice/entry.go`
  - `RunAllPhase1WithSession(...)` 当前已补 `nil session / nil managed runtime / nil inner runtime` 保护
  - 以上入口级非法输入当前统一返回 `ErrInvalidArgument`
  - 不再因空 session 直接 panic
- `application/workorchd/appservice/runtime_managed.go`
  - `NewRuntimeFromBootstrap(nil)` 当前返回 `nil`
  - `NewManagedRuntime(nil, ...)` 当前也返回 `nil`
  - runtime 构造入口当前不再因空依赖构造出“外层非 nil、内层不可用”的危险对象
- `application/workorchd/appservice/entry_test.go`
  - 已新增空 session/空 runtime 返回 `ErrInvalidArgument` 回归
- `application/workorchd/appservice/runtime_adapter_test.go`
  - 已新增 `NewManagedRuntime(nil, ...) == nil` 回归
- `application/workorchd/appservice/runtime_session_test.go`
  - 已新增 `NewRuntimeFromBootstrap(nil) == nil` 回归
- 本轮完成后，第 3 项遗留继续收窄
  - `appservice/demo` 入口链路当前已补齐一组直接的空输入保护
  - phase1 顶层执行入口当前对明显非法装配态已能稳定失败，而不是 panic

### 64. session/backend 非法配置收口第五十一轮

- `application/workorchd/appservice/runtime_session.go`
  - `NewRuntimeSessionWithBackend(...)` / `NewRuntimeSessionFromEnv(...)` 当前已改为返回 `(*RuntimeSession, error)`
  - 当前会在 session 构造链路中显式拦截 `unsupported repository backend` 这类配置型 panic
  - 对非法 backend 当前统一转换为 `ErrInvalidArgument`
  - 对其他非配置类 panic 仍保持继续抛出，不吞内部装配 bug
- `application/workorchd/appservice/entry.go`
  - `RunAllPhase1WithBackend(...)` / `RunAllPhase1FromEnv(...)` 当前已显式处理 session 构造错误
  - 非法 backend 当前不再通过 panic 终止顶层执行入口
- `application/workorchd/appservice/test_helpers_test.go`
  - 测试 helper 当前已对 session 构造错误显式失败，不再假设构造恒成功
- `application/workorchd/appservice/runtime_session_test.go`
  - 已新增 unsupported backend 返回 `ErrInvalidArgument` 回归
- `application/workorchd/appservice/entry_test.go`
  - 已新增 `RunAllPhase1WithBackend(...)` / `RunAllPhase1FromEnv(...)` 对 unsupported backend 返回 `ErrInvalidArgument` 回归
- `application/workorchd/demo/demo_test.go`
  - 已新增 demo 薄转发层对 unsupported backend 返回 `ErrInvalidArgument` 回归
- 本轮完成后，第 3 项遗留继续收窄
  - `appservice/demo` 顶层配置入口当前已补齐非法 backend 的稳定失败语义
  - phase1 执行入口当前对这类用户配置错误已不再以 panic 形式暴露

### 65. orchestrator 终态动作状态约束第五十二轮

- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator.go`
  - `CompleteRun(...)` 当前已补状态约束
  - 当前只允许从 `running` 进入 `completed`
  - 对 `created/blocked/completed/failed` run 直接 `CompleteRun(...)`，当前会返回 `ErrConflict`
  - `FailRun(...)` 当前也已补状态约束
  - 当前允许从 `created/running/blockedOnDecision/blockedOnApproval` 进入 `failed`
  - 对已 `completed/failed` run 直接 `FailRun(...)`，当前会返回 `ErrConflict`
- `internal/modules/kernel/orchestrator/biz/runtime_orchestrator_test.go`
  - 已新增 `CompleteRun(...)` 对非 `running` 状态返回 `ErrConflict` 的回归
  - 已新增 `FailRun(...)` 对 terminal run 返回 `ErrConflict` 的回归
  - 当前也会校验冲突时 run 对象不被改写
- 本轮完成后，第 1 项遗留继续收窄
  - orchestrator 原始终态动作当前不再静默覆写 terminal state
  - 底层原始动作与上层 run service 的状态边界当前进一步一致

### 66. demo/main 非法 backend 入口回归第五十三轮

- `application/workorchd/appservice/runtime_session_test.go`
  - 已新增 `NewRuntimeSessionFromEnv()` 对 unsupported backend 返回 `ErrInvalidArgument` 的回归
- `application/workorchd/demo/demo_test.go`
  - 已新增 `RunAllFromEnv(...)` 对 unsupported backend 返回 `ErrInvalidArgument` 的回归
- `application/workorchd/main_test.go`
  - 已新增 `go run ./application/workorchd` 在 unsupported backend 下失败的端到端回归
  - 当前会校验输出中包含
    - `invalid argument`
    - `unsupported repository backend`
- 本轮完成后，第 3 项遗留继续收窄
  - 非法 backend 的失败语义当前已从 `appservice` 一直锁到 `demo` 与 `main` 进程入口
  - 用户可见的顶层执行入口当前对该类配置错误具备稳定回归基线

### 67. file backend 默认 dataRoot 回归第五十四轮

- `application/workorchd/appservice/entry_test.go`
  - 已新增 `RunAllPhase1FromEnv(...)` 在 `WORKORCH_REPOSITORY_BACKEND=file` 且 `WORKORCH_DATA_DIR` 为空时的回归
  - 当前通过设置 `TMPDIR` 锁定默认落盘目录为 `${TMPDIR}/workorchd-data`
  - 当前会校验六个持久化文件都会落到默认目录
- `application/workorchd/demo/demo_test.go`
  - 已新增 `RunAllFromEnv(...)` 对同一默认 dataRoot 行为的回归
- `application/workorchd/main_test.go`
  - 已新增 `go run ./application/workorchd` 在 file backend 且省略 data dir 时的端到端回归
  - 当前会校验 demo 输出正常且默认目录下文件已生成
- 本轮完成后，第 3 项遗留继续收窄
  - file backend 当前不仅覆盖显式 dataRoot，也已覆盖默认 dataRoot 的顶层入口行为
  - `appservice/demo/main` 三层入口对该配置边界当前都具备稳定回归

### 68. appservice 顶层业务错误透传回归第五十五轮

- `application/workorchd/appservice/entry_test.go`
  - 已新增 `RunAllPhase1WithSession(...)` 顶层业务错误透传回归
  - 当前通过 fake runtime 显式验证 `ErrBlocked / ErrConflict / ErrNotFound` 会从入口直接返回
  - 当前也会校验首个业务错误发生后，不会继续进入后续 `EnsureRun(...)` 调用
- 本轮完成后，第 3 项遗留继续收窄
  - `appservice` 顶层执行入口当前已对关键业务错误类型具备显式透传基线
  - 顶层入口对这组错误当前不会吞错、改写错误类型或继续推进后续 slice

### 69. appservice 空依赖保护第五十六轮

- 已新增 `application/workorchd/appservice/guard.go`
  - 当前统一承接 `appservice` 内部对空依赖的最小保护
  - 对 `nil` runtime / service / repository / action port 当前统一返回 `ErrInvalidArgument`
- `application/workorchd/appservice/runtime_adapter.go`
  - 当前已为 snapshot read 与 action-side 全部入口补空依赖保护
  - `runtimeAdapter` 在依赖未装配完整时不再因空 repository 或空 action port 直接 panic
- `application/workorchd/appservice/phase1.go`
  - `RunAllPhase1(...)` 当前已对空 runtime 显式返回 `ErrInvalidArgument`
  - `Phase1Service` 当前也已为 `snapshotReads / workspaceActions / runActions / decisionActions / approvalActions` 补统一保护
  - 手工构造不完整 `Phase1Service` 时，当前会稳定失败而不是在 `snapshot` 或 action 调用中 panic
- `application/workorchd/appservice/runtime_adapter_test.go`
  - 已新增空依赖与 nil receiver 回归
  - 当前会校验 `runtimeAdapter` 各 read/action 入口统一返回 `ErrInvalidArgument`
- `application/workorchd/appservice/phase1_test.go`
  - 已新增 `RunAllPhase1(nil)`、空 action 依赖与缺失 snapshot read 的回归
  - 当前会校验 phase1 编排层对这类非法装配态稳定返回 `ErrInvalidArgument`
- 本轮完成后，第 3 项遗留继续收窄
  - `appservice` 入口之外，phase1 编排主体与 runtime adapter 当前也补齐了同类非法装配保护
  - phase1 链路对“部分构造对象被误调用”的失败语义当前已更加一致

### 70. file backend 默认 dataRoot 直连路径回归第五十七轮

- `application/workorchd/bootstrap/runtime_test.go`
  - 已新增 `NewWithBackend("file", "")` 的默认 dataRoot 回归
  - 当前会通过 `TMPDIR` 锁定默认目录为 `${TMPDIR}/workorchd-data`
  - bootstrap 直连 file backend 当前不再只依赖 env 链路间接覆盖
- `application/workorchd/appservice/entry_test.go`
  - 已新增 `RunAllPhase1WithBackend(ctx, "file", "")` 的默认 dataRoot 回归
  - 当前会校验 6 个持久化文件都落到默认目录
- `application/workorchd/demo/demo_test.go`
  - 已新增 `RunAllWithBackend(ctx, "file", "")` 的默认 dataRoot 回归
  - demo 薄包装层当前也已显式锁定与 `appservice` 一致的默认 file backend 语义
- 本轮完成后，第 3 项遗留继续收窄
  - file backend 的默认 dataRoot 当前不仅覆盖 `FromEnv(...)`，也覆盖了 `WithBackend(...)` 直连路径
  - `bootstrap -> appservice -> demo` 三层在该行为上当前具备统一回归基线

### 71. file backend 默认 dataRoot 重复执行回归第五十八轮

- `application/workorchd/appservice/runtime_session_test.go`
  - 已新增 `NewRuntimeSessionWithBackend("file", "")` 的默认 dataRoot 回归
  - session 直连 file backend 当前也已显式锁定默认目录语义
- `application/workorchd/appservice/entry_test.go`
  - 已新增 `RunAllPhase1WithBackend(ctx, "file", "")` 在默认 dataRoot 下的重载稳定性回归
  - 当前会校验同一默认目录上的两次执行返回稳定 phase1 snapshot
- `application/workorchd/demo/demo_test.go`
  - 已新增 `RunAllWithBackend(ctx, "file", "")` 在默认 dataRoot 下的重载稳定性回归
  - demo 薄包装层当前也已显式锁定默认 file backend 的重复执行语义
- `application/workorchd/main_test.go`
  - 已把默认 data dir 的 `go run ./application/workorchd` 回归扩展为两次执行
  - 进程入口当前不仅验证首轮可运行，也验证同一默认目录上重复执行仍稳定输出 4 条 slice
- 本轮完成后，第 3 项遗留继续收窄
  - file backend 的“默认 dataRoot + 重复执行”当前已从 session 直连路径一直锁到 main 进程入口
  - phase1 对默认持久化目录的幂等/重载基线当前进一步完整

### 72. appservice action port 契约回归第五十九轮

- 已新增 `application/workorchd/appservice/runtime_action_ports_test.go`
  - 当前独立覆盖 `workspace / run / decision / approval` 四组 action port
  - 已显式锁定 nil service 时统一返回 `ErrInvalidArgument`
- `runtime_action_ports`
  - 当前已显式锁定 appservice 参数到模块 command DTO 的映射
  - `run` action port 当前会继续保持 phase1 所需的最小 command 面
    - `EnsureRun(...)` 当前明确只写入 `runID/workspaceID`
    - `goalID/doDID` 在 phase1 adapter 中当前保持空值，不被意外漂移
  - `decision/approval` action port 当前也已显式锁定 ticket/approval 绑定字段与 resolve payload 透传行为
- 本轮完成后，第 1、3 项遗留继续收窄
  - `appservice` 到模块 `service` 的 command adapter 边界当前已具备独立回归
  - 后续若继续缩减 application 层或调整模块 command 结构，破坏面会更早在本层暴露

### 73. appservice session 生命周期与 panic 语义回归第六十轮

- `application/workorchd/appservice/entry_test.go`
  - 已新增 `RunAllPhase1WithSession(...)` 成功路径后的 session 资源释放回归
  - 已新增非法 runtime 路径下也会释放 `Bootstrap/Runtime` 的回归
- `application/workorchd/appservice/runtime_session_test.go`
  - 已新增 `buildBootstrapRuntime(...)` 对非配置型 panic 继续抛出的回归
  - 当前只对 `unsupported repository backend` 做 `ErrInvalidArgument` 转换，其他 panic 不会被静默吞掉
- 本轮完成后，第 3 项遗留继续收窄
  - `appservice` 顶层 session 生命周期当前不仅覆盖正常执行，也覆盖了非法装配态的释放语义
  - session 构造链路当前对“配置错误可转换、内部 bug 继续抛出”的边界也已有显式基线

### 74. managed runtime 空 exports 收口第六十一轮

- `application/workorchd/appservice/runtime_assembly.go`
  - `managedRuntimeAssemblyFromExports(...)` 当前已显式校验 phase1 必需的 repository/service exports
  - 缺关键 exports 时，当前不再继续构造半残的 runtime adapter 与 collector
  - `newManagedRuntimeFromAssembly(...)` 当前在 assembly.runtime 为空时直接返回 `nil`
- `application/workorchd/appservice/runtime_session_test.go`
  - 已新增 `NewRuntimeFromExports(bootstrap.Exports{}) == nil` 回归
- `application/workorchd/appservice/runtime_assembly_test.go`
  - 已新增空 exports 返回空 assembly 的回归
- 本轮完成后，第 1、3 项遗留继续收窄
  - `NewRuntimeFromBootstrap(nil)` / `NewManagedRuntime(nil, ...)` / `NewRuntimeFromExports(empty)` 当前已统一为“不构造危险对象”
  - managed runtime 构造层对非法装配态的失败语义当前进一步一致

### 75. observed collector payload 深克隆第六十二轮

- `application/workorchd/observed/collector.go`
  - `cloneEnvelope(...)` 当前已从顶层浅拷贝提升为递归克隆 `map[string]any` / `[]any`
  - collector 对外返回的 published phase1 event 当前不再把嵌套 payload 结构直接暴露给调用方
- `application/workorchd/observed/collector_test.go`
  - 已新增嵌套 `input/output/items` payload 的深克隆回归
  - 当前会校验调用方修改首次读取结果后，不会污染后续 `PublishedPhase1Events()` 返回值
- 本轮完成后，第 4 项遗留继续收窄
  - observed collector 当前对 phase1 typed event 的只读语义更稳定
  - 后续 `collector -> managed runtime -> observed snapshot` 链路当前更不容易被外部调用方的意外修改反向污染

### 76. observed/presenter 只读边界补强第六十三轮

- `application/workorchd/observed/snapshot_contract.go`
  - `SnapshotItem.PayloadMap()` 与 `Envelope()` 当前已改为递归深克隆 `map[string]any` / `[]any`
  - `StepPayload` 与 `DecisionTicketPayload` 的 payload encode/decode 当前不再把嵌套结构直接暴露给外部调用方
- `application/workorchd/observed/snapshot_contract_test.go`
  - 已新增嵌套 `input/output/items` payload 深克隆回归
  - 当前会校验对首次导出 payload 与 envelope 的修改不会污染后续读取结果
- `application/workorchd/presenter/presenter.go`
  - `MapStep(...)` 当前已对 `Input/Output` 做递归深克隆
  - `MapObserved(...)` 继续通过 `SnapshotItem.PayloadMap()` 维持只读输出边界
- `application/workorchd/presenter/presenter_test.go`
  - 已新增 `MapStep(...)` 与 `MapObserved(...)` 的嵌套 payload 深克隆回归
  - 当前会校验 presenter 输出被调用方修改后，不会反向污染 run model 与 observed snapshot 源对象
- 本轮完成后，第 4 项遗留继续收窄
  - `collector -> observed snapshot -> presenter` 主链路当前已统一为只读导出语义
  - phase1 观察输出更接近稳定快照，不会被调用方持有的 map/slice 嵌套引用反向污染

### 77. phase1 入口关闭错误合并回归第六十四轮

- `application/workorchd/appservice/runtime_session_test.go`
  - 已新增 `RuntimeSession.Close()` 在 `Bootstrap.Shutdown(...)` 失败时的回归
  - 当前会校验即使关闭失败，也会释放 `Runtime/Bootstrap` 持有引用，避免 session 进入半关闭态
- `application/workorchd/appservice/entry_test.go`
  - 已新增 phase1 成功完成后叠加 `session.Close(...)` 错误的回归
  - 已新增顶层业务错误与关闭错误通过 `errors.Join(...)` 同时保留的回归
  - 当前会校验 `RunAllPhase1WithSession(...)` 在异常组合路径下仍会释放 session 持有资源
- 本轮完成后，第 1、3 项遗留继续收窄
  - phase1 顶层入口当前对 “业务推进成功/失败 + 关闭失败” 的组合语义已具备显式测试约束
  - `appservice -> demo/main` 上游依赖的入口错误面当前更稳定，不会因关闭期异常丢失主错误或遗留脏 session 状态

## 当前结构判断

目前仓库已经达到以下状态：

- 模块目录结构基本符合 `go-multi-module-dev` 最小骨架
- 应用入口已经从“大文件拼装”演进到“入口 / bootstrap / appservice / demo / presenter”分层
- 应用层已不再直接依赖共享 contracts，也不再直接构造模块 command DTO
- `workspace/run/decision/orchestrator` 四个核心模块已具备自有契约层，旧 `contracts` 共享包已完成移除
- `workspace / approval / decision / run / orchestrator / legacy` 六个业务模块均已完成一轮能力面与边界面收口
- 六个业务模块已具备最小 `Module` 生命周期入口，bootstrap 也已开始通过模块入口而不是直接拼装底层实现
- `workspace / run / decision / orchestrator / approval` 五个模块的 `service/` 已具备最小 adapter 语义，不再只是 DTO/interface 集合
- `appservice` 已成为 application 级编排收口落点，`demo` 当前已不再承载 phase1 主业务推进逻辑
- `appservice` 也已成为 phase1 场景级测试的主要落点，`demo` 当前仅保留展示/场景入口职责
- `appservice` 与 `bootstrap` 之间当前已切换为窄接口依赖，application 编排层对具体 runtime 实现的耦合进一步降低
- `appservice` 现已同时承接 phase1 的 env-backed 顶层执行入口，并负责 runtime 生命周期回收
- `appservice` 当前也已把 collector 生命周期外提到 `ManagedRuntime`，`runtimeAdapter` 已收敛为纯业务适配层
- `appservice` 当前内部 phase1 runtime 也已显式拆成 `ReadRuntime / ActionRuntime`
- `appservice` 当前内部 phase1 写侧也已继续拆成 `workspace/run/decision/approval` 四组动作面
- `appservice` 当前内部 phase1 读侧已继续收口为 `snapshotReads`
- `appservice/runtime_adapter` 当前内部依赖也已继续拆成 `Reads / Actions` capability group
- `appservice` 当前写侧对模块能力的消费也已收口为本地 action port，不再在 application 编排主体直接拼装 module command DTO
- `appservice` 当前对 `decision/approval` 的 module-local ensure 语义也已下沉回模块层
- `appservice` 当前对 `run` 的 module-local ensure 语义也已下沉回模块层，application runtime 读侧已不再保留 guard capability
- `appservice` 当前对 run blocking 引用冲突的保护也已回收到模块 `service/biz`
- `appservice` 当前对 run 同 ID 下的 `goalID/doDID` 语义漂移也已由模块 `service/biz` 负责拦截
- `appservice` 下游依赖的 orchestrator 原始动作面当前也已补齐最小状态约束，不再静默覆盖非创建态/非可阻塞态 run
- `appservice` 下游依赖的 orchestrator 原始终态动作当前也已补齐状态约束，不再静默覆写 completed/failed run
- `appservice` 当前对 decision/approval 同 ID 下的 reason / resolution 语义漂移也已回收到模块 `service/biz`
- `appservice` 当前对 resume blocked 与缺失 decision/approval 对象的异常透传也已具备明确回归
- `appservice` 当前 runtime dependencies 也已去掉空的 `Reads` 包装层，收口为直接 `Snapshot + Actions`
- `appservice` 当前 exports -> runtime dependencies 的组装也已收拢到单点 helper
- `appservice` 当前 managed runtime 的 runtime/collector 接线也已收拢到单点 assembly helper
- `appservice` 当前同类 runtime 生命周期模式也已统一到 `RuntimeSession`
- `appservice/demo` 当前同类 phase1 执行模式也已统一到 session/backend helper
- `appservice/demo` 当前测试侧同类 setup/execute 模式也已统一到 test helper
- `appservice` 当前 phase1 执行入口对空 session / 空 runtime 也已具备显式非法输入保护
- `appservice` 当前 phase1 编排主体与 runtime adapter 也已对空 repository / 空 action port / 空 snapshot read 具备显式非法输入保护
- `appservice` 当前 phase1 执行入口对非法 repository backend 配置也已具备显式错误返回，不再 panic
- `appservice` 当前 phase1 顶层执行入口对关键业务错误类型也已具备显式透传回归
- `demo` 与 `main` 当前也已对非法 repository backend 的最终失败语义具备显式回归
- `demo` 与 `main` 当前也已对 file backend 默认 dataRoot 行为具备显式回归
- `main` 当前直接依赖 `appservice`，`demo` 已进一步退化为兼容性 shim
- phase1 所需的 runtime adapter 也已收回到 `appservice`，`bootstrap.Runtime` 不再直接承担 phase1 应用方法面
- `bootstrap.Runtime` 当前也已从散列 getter 收敛为单一 `Exports()` 聚合导出
- `bootstrap.Runtime` 当前内部也已收敛为单一 `Exports + lifecycle` 壳层，不再在实现内散落 repo/service 字段
- `bootstrap.Exports` 当前也已从扁平依赖集合继续收敛为 `Repositories / Services / Infrastructure` capability group
- `bootstrap.Exports.Services` 当前也已完成一轮稳定性评估，确认现阶段四个模块 façade 已是最小稳定集合
- `appservice` 当前已不再直接持有 workspace repository 写路径，workspace 基线恢复语义已回收到模块 service
- `observed` envelope 构造已从 `presenter` 下沉到独立 `observed` 适配层
- `observed` 当前已开始真实消费 `EventHub` 中已发布的 `workspace / run / step / decision / approval` 事件，而不是完全依赖 state 派生
- `workspace.activeRunId` 的运行期绑定与释放变化当前也已通过 orchestrator 真实事件覆盖
- `observed` 当前已通过订阅式 collector 收集 phase1 事件，不再要求应用层直接读取 `MemoryHub.Published()`
- collector 当前也已具备显式 `Close()` 生命周期，并已通过 `ManagedRuntime` 接入应用入口和主要测试路径
- phase1 observed 当前已具备集中化 event-to-snapshot contract，collector topic 与 snapshot normalize 规则已不再分散维护
- phase1 published event payload 当前也已具备集中化 key/builder 契约，不再在模块与应用层重复散落维护
- phase1 observed 当前也已具备 published payload 的 typed decode/validate 边界，不再直接透传未经校验的原始 payload
- phase1 collector 与 observed normalize 当前也已统一复用 shared typed event helper，不再各自维护独立的 phase1 event 解码路径
- phase1 collector -> appservice -> observed snapshot 当前也已统一直接传递 typed phase1 event，而不再在中途回落到裸 envelope
- phase1 observed 输出当前也已具备 typed snapshot contract，不再把裸 `events.Envelope` 直接暴露给 `presenter`
- phase1 observed fallback 组装与 presenter observed 渲染当前也已不再依赖 `Envelope` 作为内部中转
- phase1 event / snapshot 当前也已具备显式最终版本常量与严格校验，合同行为进一步稳定
- phase1 observed 输出当前也已改为最终 lower-camel-case 渲染形态，不再保留 envelope-style 输出兼容层
- phase1 observed 在 file backend reload 下的 resolved decision payload 当前也已具备稳定回放语义
- demo 逻辑现在已具备包级测试，不再只依赖应用级 smoke test
- demo 已具备首轮跨模块业务集成测试，可直接校验 phase1 四条 slice 在 `memory/file` 双后端下的最终业务结果与观察输出
- bootstrap 作为应用装配层已经开始暴露稳定方法面，而不是直接暴露 repo/service 字段
- bootstrap 已从单文件装配继续收敛为 `backend`、`assembly`、`runtime` 三个职责文件
- `approval` 模块已完成一轮 “biz 统一到 service 契约” 收口，并已扩展为多模块共用的 service adapter 模式模板

## 历史未完成项状态

本节原先记录的重构遗留项已经被后续 MVP 与 framework alignment 收口覆盖，不再作为活跃待办清单维护。当前有效状态以 `docs/workorch-status.md` 为准；如需继续扩展模块边界、运行时、事件观察或持久化能力，应重新拆成新的正式业务需求。

## 本轮基线

当前整理后的验证基线：

```bash
go test ./...
```

最近一次已通过。
