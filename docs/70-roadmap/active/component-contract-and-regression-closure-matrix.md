# WorkOrch 22 组件合同与独立回归收口矩阵

Status: active

Type: implementation closure matrix

Last Updated: 2026-07-28

## Purpose

本矩阵是 22 组件当前唯一的 active 收口清单；原整改过程与已完成迁移记录已归档至
[framework boundary remediation record](../../90-archive/records/go-multi-module-22-component-boundary-remediation-2026-07-25.md)。
它不重述或替代 Runtime V2
状态机：状态转换仍以
[runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、
`kernel/orchestrator/readme.md` 和 `workorchruntimecontract` 为准。

每个组件的收口必须能回答六个问题：

1. 它唯一拥有或协调什么状态；
2. 它接收哪些有类型的 EventHub 命令/通知；
3. 它向哪些 owner 发出哪些有类型的命令/通知；
4. 它明确不能做什么；
5. 不启动完整服务时如何单独回归；
6. 哪一项跨组件或 deployed 证据才可以证明其与系统协同正确。

这里的“单独回归”是**组件级回归**：允许使用 `t.TempDir()`、in-memory/file
repository、测试 EventHub，以及相邻 owner 的 typed fake responder；不允许通过
`Module.Service()`、跨 owner repository 或全局 service registry 绕过生产合同。

## Component Type Baseline

组件类型由职责闭环决定，不由目录名、repository、文件规模或 framework 实现类型
名称决定。当前 22 个运行单元的唯一类型基线为：

| 类型 | 数量 | 组件 |
| --- | ---: | --- |
| Initiator | 1 | `initiators/workorchruntime` |
| Module | 4 | `application/workorchd`、`kernel/orchestrator`、`kernel/governance`、`kernel/system` |
| Block | 17 | `blocks/run`、`blocks/eventstream` 及全部 15 个 `blocks/*` owner |

`blocks/run` 与 `blocks/eventstream` 分别只拥有 Run aggregate 与 EventRecord
审计流，目录与 Block 类型一致。两者的 module ID、EventHub source/destination
和 import path 均使用 `blocks/*`；不保留旧 `kernel/*` 兼容路径或身份。

## Current Baseline and Closure Rule

截至本次记录，Base Biz 拓扑、channel-free stopped notification、owner facade
清除、owner DTO 去 repository 化、测试目录隔离，以及 workspace 删除后的迟到
投影收敛修复均已在当前工作树具备代码和本地回归。ARCH-1 仍为 active 的原因
仅是：最新二进制尚需在外部目标重启后执行 SM-4 三类 fixture；其中删除
workspace 后的迟到 EventStream 事件必须使 `/readyz` 恢复为 ready，而不是遗留
degraded projection。

因此每个后续代码变更均按下表状态推进，而不是把已经完成的结构迁移重新当作
开放设计：

| 标记 | 含义 | 允许关闭的证据 |
| --- | --- | --- |
| `L` | 本地实现与目标测试已落地 | 组件级测试、静态边界扫描、受影响的相邻合同测试 |
| `I` | 组件间合同已验证 | 至少一个真实 EventHub sender/receiver 的 typed 集成测试 |
| `D` | 外部部署验证待完成 | latest 服务上的 restart/projection fixture；不得用人工 Resume 替代 |
| `C` | 完成 | L + I + 所需 D 均成立，并同步更新当前事实文档 |

若修改触及 Runtime V2，变更说明必须标出 SM-1～SM-4 的影响。本矩阵只定义
组件边界与独立回归，**不定义** Runtime V2 语义收口的实施顺序或状态迁移。
`RUNTIME-SMO` 主方案中已明确授权的工作卡可以修改 SM-1～SM-4、Workspace/Result lifecycle 以及
candidate/receipt 合同。矩阵不得以“仅 SM-4”阻断任一收口，也不得创建第二套 reducer/dispatch/lifecycle
任务。ARCH-1 自身的 framework 整改不得借机改写 authority、candidate、evidence 或 lifecycle 语义。

## Common Interaction Contract

```text
inbound protocol/UI
  -> workorchd Biz (facade DTO mapping)
  -> owner-defined EventHub Command/Data/Result
  -> owner Biz (state/capability execution)
  -> typed Result or notification
  -> workorchd projection / eventstream audit

Runtime V2 only:
workorchd Start/Resume intent -> Run owner-local transition -> RunStateChanged -> Orchestrator DriveRuntimeV2
  -> SM-2 owner facts -> one persisted candidate manifest
  -> SM-3 request -> strict decode -> one evaluator -> sealed dispatch
  -> owner feasibility/operation receipt
  -> accepted evidence / governance resolution
  -> terminal delivery fact -> Result owner -> Workspace owner -> Run owner acknowledgement
```

所有行都必须满足下列通用不变量：

- Module root / plugin 只做 framework 生命周期桥接；使用 Hub 的 Module/Block
  只可由自己的 `biz`（嵌入 `basebiz.Base`）持有 Hub、observer、订阅和 task。
- 跨 owner I/O 只可使用 owner 定义的具体 Command/Data/Result。禁止直接导入
  对方 `biz`、`service`、repository、实体、channel、server、store、registry，
  以及 `map[string]any` / envelope / JSON 反射桥。
- `service/` 仅是 HTTP、CLI、session 或协议 adapter；它不保存 Hub，也不成为
  owner method 的转发 facade。
- 每个组件的 `pkg/` 只能保存 DTO、typed command/result、合同枚举与纯无状态
  normalize/validate/clone/hash/compiler helper。禁止在 `pkg/` 中持有 Hub/repository、执行
  I/O/CAS、调度 retry、选择 terminal/Runtime action 或推进任何状态机。
- EventHub DTO 是值合同：带 scope、ID、cursor/limit、bounded slice/map 和明确
  result/error code；它不得携带可变实现或 persistence type。
- 对 workspace-scoped 输入，组件先用 typed `WorkspaceID` 建立或核验 EventHub context；若 context
  已携带不同 workspace，必须返回 conflict。组件实例、Service、全局状态、共享 cache 和后台 task 不得
  保存“当前 workspace”标识；持久化记录/封存合同除外，其 ID 仅是资源归属 identity。
- 每个 Biz 的 `Teardown` 都先停止新输入、解除自身订阅、取消自身 task，再释放
  自身资源；重复调用不能 panic，也不能关闭共享 Hub/BackgroundRoutine。
- 任何测试产生的 workspace、receipt、repository 或 artifact 都进入 `t.TempDir()`
  或 `/tmp/workorch-e2e`；源码树不能生成 `.workorch-data`、
  `.workorch-testkit-git-receipts` 或 `.workorch-e2e`。

## Component Closure Cards

表中的“入站/出站”指 production EventHub 合同类别，而非穷举每个 topic 名；具体
topic、Command、Data、Result 的唯一代码 SoT 均是组件自己的 `pkg/events`。
“独立命令”必须在每次修改该组件后执行；涉及 runtime 的两项同时执行 `-race`。

| 组件（状态） | 功能职责与状态权威 | 入站合同 | 出站合同 | 禁止边界 | 独立命令与完成断言 |
| --- | --- | --- | --- | --- | --- |
| `initiators/workorchruntime`（Initiator；L） | 单一 repository provider/factory；不拥有业务状态。 | framework 初始化。 | 向本进程提供 repository infrastructure。 | EventHub、runtime policy、bootstrap 聚合、业务订阅。 | `go test ./internal/initiators/workorchruntime/... -count=1`；provider 配置失败 fail-fast，且无 Base/Hub export。 |
| `application/workorchd`（Module；L/I，D） | HTTP/CLI/SSE/read model、typed bootstrap 与 Start/Resume intent 发起；projection 是派生状态。 | HTTP/CLI facade、workspace-run request、owner notification。 | workspace/run/decision/approval 等 owner command；Run owner activation/continuation intent；`ServiceStoppedData`。 | drive queue、reducer、owner repository、EventHub channel。 | `go test ./internal/modules/application/workorchd/... -count=1`；request 映射、stopped 通知、删除后迟到投影收敛、Start/Resume concrete type。部署后 `/readyz` ready 是 D。 |
| `blocks/eventstream`（Block；L/I） | 审计 EventRecord 与投影 high-watermark；不拥有业务恢复。 | 各 owner notification、EventRecord query。 | typed audit/query result，供 projection/readiness 查询。 | action 选择、aggregate 重建、direct owner write。 | `go test ./internal/modules/blocks/eventstream/... -count=1`；notification 持久化、cursor/filter DTO mapping、high-watermark 单调。 |
| `kernel/governance`（Module；L/I） | auto-governance trace/协调；不替代 Runtime V2 contract gate。 | governance evaluate/trace query。 | decision/approval 的 typed request 或 trace result。 | 把 ticket resolution 直接改为 Run active、发送通用 Resume、默认 decision 替换 source dependency。 | `go test ./internal/modules/kernel/governance/... -count=1`；scope/filter mapping、重复 trace 幂等、无 repository DTO 外泄，且治理只提交证据、不续跑 Run。 |
| `blocks/run`（Block；L/I，D） | Run/Step/RuntimeStateRecord 的唯一持久化 owner、CAS、recovery inventory 与 SM-1 reducer；不选 action。 | Run CRUD/CAS、activation/continuation/governance/terminal fact、typed recovery inventory。 | Run state notification；向请求者返回 aggregate/inventory。 | reducer、operation 执行、repository 被其他 owner 直接访问。 | `go test -race ./internal/modules/blocks/run/... -count=1`；CAS stale、owner-local transition、首次 active pre-runtime fact、continuation source、terminal 完整 identity、重复 teardown、inventory 分类完整。restart fixture 是 D。 |
| `kernel/orchestrator`（Module；L/I，D） | Runtime V2 唯一 SM-2/3/4 reducer/drive/evidence/governance-resolution authority；SM-2 编译 sealed manifest，SM-3 只消费单 evaluator 结果，SM-4 只记录 delivery acknowledgement。 | Run state fact、internal Drive、typed owner facts、candidate/rejection、feasibility observation、governance resolution、own receipt query/commit。 | Run aggregate CAS/inventory；prerequisite acquisition；LLM candidate；capability operation；Decision/Approval ticket；immutable governance/terminal fact。 | SM-1/Workspace/Result state write、profile/domain hard-code、provider adapter、重复 kind/target/parameter policy、raw reason routing、跨 owner repo、application continuation queue。 | `go test -race ./internal/modules/kernel/orchestrator/... -count=1`；manifest/evaluator first-failure、typed drive、sealed receipt/fence、stale/duplicate、`drive_schedule` bounded retry、recovery no duplicate effect、delivery acknowledgement。三类 restart fixture 是 D。 |
| `kernel/system`（Module；L/I） | 启动默认 Soul/Template/MCP 的 ensure 协调；无 Runtime V2 authority。 | framework startup/default ensure command。 | 对 Soul/Workspace/MCP Registry 的 typed ensure command。 | 默认值直接写对方 repository、业务 action/recovery。 | `go test ./internal/modules/kernel/system/... -count=1`；重复 ensure 幂等、owner error 可见、teardown 不关共享 runtime。 |
| `blocks/workspace`（L/I，D） | Workspace、Goal/DoD、Template、workspace default LLM target 的正式 owner。 | workspace/template CRUD、definition/snapshot、narrow active-run projection。 | typed Workspace/Template DTO 与 owner notification。 | Run aggregate、candidate/evidence 裁决、repository entity 作为事件 DTO。 | `go test ./internal/modules/blocks/workspace/... -count=1`；scope、template clone、LLM target persistence、首次 activation pre-runtime projection、伪造 cross-owner source、delete idempotency。删除-迟到投影 deployed 验证为 D。 |
| `blocks/result`（L/I） | 已裁决 Result/Artifact 的正式存储；不决定 accepted/rejected。 | Result create/get/list、artifact query。 | typed result DTO/notification。 | 自行验收、推进 Run、直接改 Workspace。 | `go test ./internal/modules/blocks/result/... -count=1`；同一 terminal write 幂等、owner error 保留、EventHub-only write。 |
| `blocks/decision`（L/I） | Decision ticket 与 resolution 的正式 owner。 | create/list/get/resolve；runtime governance ticket resolution。 | typed resolution；将 bound runtime ticket resolution 提交给 Orchestrator。 | application watcher 再 Resume、无 ticket/漂移 binding 的 resolution、直接写 Run。 | `go test ./internal/modules/blocks/decision/... -count=1`；resolution type 校验、重复 resolution、ticket binding 与 Orchestrator command 类型。 |
| `blocks/approval`（L/I） | Approval ticket/approval evidence 的正式 owner。 | create/list/get/approve/reject/batch resolve。 | typed approval outcome/notification。 | 自行恢复 Run、绕过 governance dependency、跨 owner repo。 | `go test ./internal/modules/blocks/approval/... -count=1`；approve/reject 幂等、scope 校验、EventHub evidence 合同。 |
| `blocks/memory`（L/I） | Memory 与 recall trace 的正式 owner。 | CRUD/search、recall trace query。 | typed search/trace DTO/notification。 | repository query/entity 外泄、成为 Runtime evidence 裁决者。 | `go test ./internal/modules/blocks/memory/... -count=1`；limit/filter/scope mapping、slice clone、repository error 保留。 |
| `blocks/skill`（L/I） | Skill registry 的正式 owner。 | Skill CRUD/get/list/binding query。 | typed registry/default DTO。 | Hub facade、直接修改 workspace/run。 | `go test ./internal/modules/blocks/skill/... -count=1`；registry CRUD、default/binding scope、重复 lifecycle。 |
| `blocks/soul`（L/I） | Soul registry/default 的正式 owner。 | Soul CRUD/get/list/default query。 | typed Soul/default DTO。 | 直接写 Workspace default 或编排 runtime。 | `go test ./internal/modules/blocks/soul/... -count=1`；default ensure 幂等、owner DTO clone、teardown。 |
| `blocks/mcpregistry`（L/I） | MCP server/capability registry 的正式 owner。 | MCP server CRUD、capability/default query。 | typed registry/capability DTO。 | 执行 MCP 调用、直接写 workspace/run。 | `go test ./internal/modules/blocks/mcpregistry/... -count=1`；server/capability 查询、default ensure、DTO 不泄露 repo。 |
| `blocks/file`（L/I） | 有界 filesystem snapshot、list/read/write/edit/patch capability 与 owner receipt。 | Orchestrator typed prerequisite、sealed authorization feasibility、sealed file operation。 | typed bounded scope/selector、feasible/stale/infeasible observation、operation receipt。 | 扩展 selector、选择 correction/action/terminal、裁决 evidence、直接更新 aggregate。 | `go test ./internal/modules/blocks/file/... -count=1`；target policy/admissibility、feasibility identity、receipt、stale snapshot、workspace root containment。 |
| `blocks/git`（L/I，D） | Git init/add/commit 等 sealed capability 与 receipt。 | sealed authorization feasibility、sealed Git operation。 | typed feasible/stale/infeasible observation、effect receipt（ExecutionKey/Fence/observed scope）。 | 选择 correction/action/terminal、合并/篡改 receipt、以相同 state hash 代表多个 durable effect、写 RuntimeState。 | `go test ./internal/modules/blocks/git/... -count=1`；authorization identity、receipt exactly-once、临时 receipt root、init→add→commit accepted 顺序。restart fixture 覆盖 owner-commit crash point。 |
| `blocks/llm`（L/I） | provider/model catalog 与 Runtime V2 candidate protocol adapter；只渲染 request 并返回 decoded candidate/provider observation。 | catalog/target query、sealed manifest candidate request、typed rejection/directive C4。 | typed candidate、provider failure/ceiling 结果。 | 从 map/V1/profile/raw reason 重建 authority、扩展 allowed set、选择 correction/terminal、裁决 evidence。 | `go test ./internal/modules/blocks/llm/... -count=1`；manifest schema/strict round-trip、C4 directive-only、workspace target precedence、sealed ceiling、stable-prefix、provider timeout/schema failure。 |
| `blocks/mcp`（L/I） | 通过注册表配置执行有界 MCP capability operation。 | sealed authorization feasibility、sealed MCP operation、capability query。 | typed feasible/stale/infeasible observation、operation receipt/structured error。 | 选择 correction/action/terminal、直接读取对方 service/repository、无界 transport payload、裁决运行状态。 | `go test ./internal/modules/blocks/mcp/... -count=1`；authorization identity、registry lookup、freshness、bounded transport、receipt。 |
| `blocks/shell`（L/I） | 有界 Shell command/validation capability 与 receipt。 | sealed authorization feasibility、sealed shell operation。 | typed feasible/stale/infeasible observation、output/receipt/failure。 | 选择 correction/action/terminal、拼接未封存命令、直接终态化 Run、将 output 当 accepted evidence。 | `go test ./internal/modules/blocks/shell/... -count=1`；authorization identity、freshness、timeout/exit、working-root containment、receipt。 |
| `blocks/ssh`（L/I） | 有界 remote SSH capability 与 structured preflight/receipt。 | sealed authorization feasibility、sealed SSH operation。 | typed feasible/stale/infeasible observation、preflight/result/receipt。 | 选择 correction/action/terminal、透传 credential、unbounded host/command、直接写 runtime。 | `go test ./internal/modules/blocks/ssh/... -count=1`；authorization identity、freshness、auth/timeout、host scope、typed failure。 |
| `blocks/web`（L/I） | 有界 web capability，含 approval/SSRF/config revision 约束。 | sealed authorization feasibility、sealed web operation、capability configuration query。 | typed feasible/stale/infeasible observation、response summary/receipt/failure。 | 选择 correction/action/terminal、未审批网络访问、SSRF 绕过、直接做 governance 或 Result 裁决。 | `go test ./internal/modules/blocks/web/... -count=1`；authorization identity、approval、SSRF、config revision、timeout/receipt。 |

## Per-Component Regression Protocol

每张组件卡的独立命令之外，新增或修改组件时必须补齐以下五类测试；不满足其中
任一项，不得因全仓测试通过就标记为 `L`：

1. **Biz 行为测试**：直接构造本 owner Biz 与 owner-private repository fixture，
   验证唯一状态写入、错误传播、幂等和 input bound。测试名以
   `Test<Owner><UseCase>...` 命名。
2. **EventHub 合同测试**：以真实的 typed Command 发往 Biz，断言返回的具体
   Result 类型、scope 和 error code；`Post` 场景必须覆盖 `result=nil`。
3. **生命周期测试**：`Setup -> Run/Start -> Teardown/Stop -> Teardown/Stop`，
   断言只订阅一次、停止后不再响应、共享 Hub/BackgroundRoutine 仍可用。
4. **边界负例测试**：至少覆盖一个被拒绝的越界输入，例如错误 workspace/run
   scope、oversized limit、stale revision、wrong ticket/ExecutionKey/Fence，或
   未封存 operation。能力组件还必须证明不会写 Run aggregate。
5. **数据隔离测试**：文件型 fixture 使用 `t.TempDir()`，测试结束后断言无源码
   树运行目录；需要进程 artifact 时使用显式 `/tmp/workorch-e2e` 根。

对于发起跨 owner 请求的组件，fake responder 必须订阅**真实 owner topic**并返回
真实 result type；禁止用 callback、mock service 或 `map[string]any` 代替。这样
组件级测试同时成为 producer-side contract test，而 owner 的 Biz 测试成为
consumer-side contract test。

## Cross-Component Contract Tests

下列测试不能由任意单一组件替代；每次涉及所列边的改动均必须执行。它们是 `I`
证据的最小集合。

| 合同边 | 必须断言 | 责任方 |
| --- | --- | --- |
| `workorchd -> workspace/run/orchestrator` | activation 确定 workspace LLM target；Start/Resume 只携带 typed scope/bootstrap，且 `resume` 不要求用户选择 restart。 | workorchd + workspace + orchestrator |
| `orchestrator <-> run` | CAS revision、recovery inventory、terminal delivery 幂等；Run 不选 action，Orchestrator 不直访 repository。 | orchestrator + run |
| `orchestrator -> workspace/llm/capability` | prerequisite owner facts 在 SM-2 编译为唯一 manifest；LLM renderer/decoder 只消费 manifest/rejection/directive；SM-3 单 evaluator 与 capability feasibility/operation 都绑定同一 transaction/authorization/snapshot；owner 只回 observation/receipt。 | orchestrator + workspace + llm + file/git/shell/mcp/ssh/web |
| `orchestrator <-> decision/approval/governance` | ticket/resolution/dependency binding 一次消费；Orchestrator 发布 continuation fact，Run owner-local continuation，不由 application Resume。 | orchestrator + decision + approval + governance + run |
| `orchestrator -> result -> workspace -> run` | reducer terminal 只发布 delivery fact；Result、Workspace、Run 依次 owner-local 幂等迁移并回执；delivery 失败不改写 authority terminal。 | orchestrator + result + run + workspace |
| `owner notification -> eventstream -> workorchd` | audit high-watermark 与 projection source revision 对齐；删除 owner 后的迟到事件删除 stale view 而不 degraded readiness。 | eventstream + workorchd + workspace |
| `workorchd -> process service` | stopped event 只传值；本地 wait channel 不经 EventHub；重复/立即 stopped 不丢信号。 | workorchd + internal/services/workorch |

推荐执行顺序为：先 producer/consumer 的组件级合同测试，再运行该表对应的边测试，
最后执行本矩阵的 repository gate。不得以端到端成功掩盖某一 owner 的 scope、
idempotency 或 lifecycle 单测缺失。

## SM-4 Deployed Closure (ARCH-1 的 Remaining D Evidence)

本节是 ARCH-1 的**验收索引**，不是独立 Runtime V2 实施计划。Runtime fixture 的 candidate/receipt、
唯一状态 owner、terminal delivery、crash/restart 序列与退出条件均以
[RUNTIME-SMO 的 C7-B～C7-F](runtime-v2-single-owner-state-machine-closure-plan.md) 为准；本矩阵只消费其通过证据来关闭组件的 `D` 标记。`workspace_deleted_late_event`
仍由 EventStream/WorkOrchd 的组件合同单独验证，但不得影响 Runtime authority。

部署验证使用最新构建重启服务后执行，不能调用 Activate/Resume 作为补偿，也不能
通过放宽 `/readyz` 掩盖投影问题：

| Fixture | 初始 persisted 状态 | 期望结果 | 关键否定断言 |
| --- | --- | --- | --- |
| `active_no_dispatch` | active aggregate，无 sealed dispatch | 自动 re-drive 到 terminal 或 governed wait | 不新增第二个 drive/attempt；不需要人工 Resume |
| `active_sealed_dispatch` | sealed dispatch，分别在 owner receipt 前/后停止 | 恢复后 exactly once，或按 sealed idempotency 转 `outcome_unknown` | 不重复 durable effect，不覆盖 owner-committed receipt |
| `terminal_delivery_pending` | reducer terminal，Result/Run/Workspace 尚未全投影 | 只补齐 delivery 并释放 activeRun | 不重算 authority，不把 terminal 改为 drive failure |
| `workspace_deleted_late_event` | workspace 已删除，投影收到迟到 owner notification | `/healthz=200`、`/readyz=200`、projection ready，stale view 已删除 | 不将双 `ErrNotFound` 计为 projection degraded |

前三项通过 `make verify-runtime-v2-restart-recovery-live` 和 manifest 执行；第四项
部署后以真实 delete + late-event/rebuild 测试执行。全部 artifact 仅可置于
`/tmp/workorch-e2e`。完成后再执行 fresh bounded positive convergence，确认
recovery 修复没有破坏正常 activation。

## Code-Closure Checklist and Documentation Updates

一次组件改动的提交说明至少包含：组件卡名称、SM 影响（若有）、输入/输出合同、
新增负例、执行命令、是否需要 D。提交前依次执行：

```text
go test <changed-component-package>/... -count=1
# run/orchestrator changes additionally:
go test -race <changed-component-package>/... -count=1
make verify-boundary-contract
git diff --check
```

跨组件改动再执行相关的 Contract Tests；完成一波后执行：

```text
go test -p 1 ./... -count=1
go vet ./...
make verify-runtime-v2-positive-convergence-file
make release-check
```

执行 `rg` 的 CI/本地脚本必须以 `rg` 可用为前提；为最小构建环境保留
`grep -R` fallback，且 fallback 必须覆盖相同的 boundary patterns。不得因为工具
缺失跳过 `verify-boundary-contract`。

组件达到 `C` 后，更新：

1. 本文件对应卡片的状态与证据；
2. 主 remediation 方案的 Closure Matrix；
3. `current-state.md`、`current-unclosed-matrix.md` 与 active overview；
4. 若 ARCH-1 全部达到 C，则将两份 active 文档一同归档，并仅在 archive
   留下命令、commit 和外部 artifact root，不提交 runtime 数据。
