# WorkOrch 22 组件合同与独立回归收口矩阵

Status: completed

Type: implementation closure matrix

Last Updated: 2026-07-29

## Purpose

本矩阵记录 22 组件收口的最终验收；原整改过程与已完成迁移记录已归档至
[framework boundary remediation record](../../90-archive/records/go-multi-module-22-component-boundary-remediation-2026-07-25.md)。
它不重述或替代 Runtime
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

`blocks/run` 只拥有 Run lifecycle，并提供 Orchestrator-owned RuntimeStateRecord 的 opaque persistence/CAS；
`blocks/eventstream` 只拥有 EventRecord 审计流。两者的目录与 Block 类型一致，module ID、EventHub source/destination
和 import path 均使用 `blocks/*`；不保留旧 `kernel/*` 兼容路径或身份。

## Current Baseline and Closure Rule

Base Biz 拓扑、channel-free stopped notification、owner facade 清除、owner DTO
去 repository 化、测试目录隔离，以及 workspace 删除后的迟到投影收敛已完成代码与本地回归。RUNTIME-SMO
的 SMO-C1～C6 已完成所有受影响组件的 feedback、outcome 与 pkg-purity 验证，SMO-C7 已完成 Runtime
restart/concurrency deployed fixture。最后的 `workspace_deleted_late_event` 已于 2026-07-29 在隔离的
managed file-backed 服务中通过：HTTP 删除先撤销 stale view，EventStream 随后持久化 `/workspace/deleted`，
WorkOrchd 的后到 rebuild 在双 owner `NotFound` 下保持 `/readyz` ready 且不增加失败计数。因此所有 `R`、`D`
标记均已消费；本记录不得再作为新的 Runtime authority 收口来源。

因此每个后续代码变更均按下表状态推进，而不是把已经完成的结构迁移重新当作
开放设计：

| 标记 | 含义 | 允许关闭的证据 |
| --- | --- | --- |
| `R` | RUNTIME-SMO 合同归一重新打开（历史） | 已由 SMO-C1～C6 producer/consumer、原始反馈保真、outcome 唯一化与静态门禁消费；当前无 `R` 卡片 |
| `L` | 本地实现与目标测试已落地 | 组件级测试、静态边界扫描、受影响的相邻合同测试 |
| `I` | 组件间合同已验证 | 至少一个真实 EventHub sender/receiver 的 typed 集成测试 |
| `D` | 外部部署验证待完成 | latest 服务上的 restart/projection fixture；不得用人工 Resume 替代 |
| `C` | 完成 | L + I + 所需 D 均成立，并同步更新当前事实文档 |

若修改触及 Runtime，变更说明必须标出 SM-1～SM-4 的影响。本矩阵只定义
组件边界与独立回归，**不定义** Runtime 语义收口的实施顺序或状态迁移。
已完成的 `RUNTIME-SMO` 记录不能再授权新行为；Runtime 变更必须从 `00-core`、Orchestrator readme 与新的
current-schema 反例出发。ARCH-1 自身的 framework 整改不得借机改写 authority、candidate、evidence 或 lifecycle
语义，也不得创建第二套 reducer/dispatch/lifecycle 任务。

## Common Interaction Contract

```text
inbound protocol/UI
  -> workorchd Biz (facade DTO mapping)
  -> owner-defined EventHub Command/Data/Result
  -> owner Biz (state/capability execution)
  -> typed Result or notification
  -> workorchd projection / eventstream audit

Runtime only:
workorchd Start/Resume intent -> Run owner-local transition -> RunStateChanged -> Orchestrator DriveRuntime
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
- 每个跨 owner feedback 必须同时提供当前结果域的 stable factual category 与完整、安全 original
  diagnostic；状态机只消费前者，后者不得被通用 error/summary 覆盖。feedback 是 concrete Result 内的纯值，
  不是通用 Envelope。credential、raw prompt/provider body 等禁止内容只记录允许的 audit 元数据。
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
| `application/workorchd`（Module；C） | HTTP/CLI/SSE/read model、typed bootstrap 与 Start/Resume intent 发起；projection 是派生状态。 | HTTP/CLI facade、workspace-run request、owner notification。 | workspace/run/decision/approval 等 owner command；Run owner activation/continuation intent；`ServiceStoppedData`。 | drive queue、reducer、owner repository、EventHub channel。 | `go test ./internal/modules/application/workorchd/... -count=1`；request 映射、safe original diagnostic 投影、stopped 通知、删除后迟到投影收敛、Start/Resume concrete type；`verify-workspace-projection-switch` deployed `/readyz` 通过。 |
| `blocks/eventstream`（Block；C） | 审计 EventRecord 与投影 high-watermark；不拥有业务恢复。 | 各 owner notification、EventRecord query。 | typed audit/query result，供 projection/readiness 查询。 | action 选择、aggregate 重建、direct owner write、禁止 raw provider 内容持久化。 | `go test ./internal/modules/blocks/eventstream/... -count=1`；有界 original diagnostic、notification 持久化、cursor/filter DTO mapping、high-watermark 单调；`/workspace/deleted` persisted→projection deployed fixture 通过。 |
| `kernel/governance`（Module；L/I） | auto-governance trace/协调；不替代 Runtime contract gate。 | governance evaluate/trace query。 | decision/approval 的 typed request 或 trace result。 | 把 ticket resolution 直接改为 Run active、发送通用 Resume、默认 decision 替换 source dependency。 | `go test ./internal/modules/kernel/governance/... -count=1`；scope/filter mapping、重复 trace 幂等、无 repository DTO 外泄，且治理只提交证据、不续跑 Run。 |
| `blocks/run`（Block；C） | Run/Step 的 SM-1 owner；为 Orchestrator-owned RuntimeStateRecord 提供 opaque 物理 CAS、query 与 inventory。 | Run CRUD/CAS、activation/continuation/governance/typed terminal outcome、recovery inventory。 | 仅含 SM-1 state 与 opaque runtime presence/revision/ref 的 notification/inventory；typed record query/CAS result。 | 解释 Orchestrator action/focus/repair/dispatch/governance/delivery ledger、对 RuntimeStateRecord 做语义分类、operation 执行、repository 被其他 owner 直接访问。 | `go test -race ./internal/modules/blocks/run/... -count=1`；CAS stale/rebase、owner-local transition、runtime opaque projection/inventory、terminal outcome identity、原始 delivery diagnostic、重复 teardown；C7 restart fixture 已通过。 |
| `kernel/orchestrator`（Module；C） | Runtime 唯一 SM-2/3/4 reducer/drive/evidence/governance-resolution authority；SM-2 编译 sealed manifest，SM-3 持久化 evaluator factual diagnostic 后 owner-local 决定 correction/final，SM-4 只记录 delivery acknowledgement。 | Run state fact、internal Drive、typed owner facts、candidate diagnostic、feasibility observation、governance resolution、owner receipt query/commit。 | Run owner opaque RuntimeStateRecord load/CAS；prerequisite acquisition；LLM candidate；capability operation；Decision/Approval ticket；immutable governance/terminal fact。 | SM-1/Workspace/Result state write、profile/domain hard-code、provider adapter、共享 pkg disposition、raw reason routing、跨 owner repo、application continuation queue。 | `go test -race ./internal/modules/kernel/orchestrator/... -count=1`；manifest/evaluator first-failure、原始诊断保真、typed drive、sealed receipt/fence、stale/duplicate、CAS/semantic conflict 分离、recovery no duplicate effect、delivery acknowledgement；C7 三类 restart fixture 已通过。 |
| `kernel/system`（Module；L/I） | 启动默认 Soul/Template/MCP 的 ensure 协调；无 Runtime authority。 | framework startup/default ensure command。 | 对 Soul/Workspace/MCP Registry 的 typed ensure command。 | 默认值直接写对方 repository、业务 action/recovery。 | `go test ./internal/modules/kernel/system/... -count=1`；重复 ensure 幂等、owner error 可见、teardown 不关共享 runtime。 |
| `blocks/workspace`（C） | Workspace、Goal/DoD、Template、workspace default LLM target 的正式 owner。 | workspace/template CRUD、definition/snapshot、narrow active-run projection、typed terminal outcome。 | typed Workspace/Template DTO、owner notification 与 delivery acknowledgement。 | 解释 RuntimeAction、Run aggregate、candidate/evidence 裁决、repository entity 作为事件 DTO。 | `go test ./internal/modules/blocks/workspace/... -count=1`；scope、template clone、LLM target persistence、terminal outcome/diagnostic、伪造 cross-owner source、delete idempotency；真实 delete→`/workspace/deleted`→late projection rebuild deployed fixture 通过。 |
| `blocks/result`（C） | 已裁决 Result/Artifact 的正式存储；不决定 accepted/rejected。 | Result create/get/list、artifact query、typed terminal outcome。 | typed result DTO/notification 与 delivery acknowledgement。 | 解释 RuntimeAction、自行验收、推进 Run、直接改 Workspace。 | `go test ./internal/modules/blocks/result/... -count=1`；同一 terminal outcome write 幂等、owner original diagnostic 保留、EventHub-only write。 |
| `blocks/decision`（L/I） | Decision ticket 与 resolution 的正式 owner。 | create/list/get/resolve；runtime governance ticket resolution。 | typed resolution；将 bound runtime ticket resolution 提交给 Orchestrator。 | application watcher 再 Resume、无 ticket/漂移 binding 的 resolution、直接写 Run。 | `go test ./internal/modules/blocks/decision/... -count=1`；resolution type 校验、重复 resolution、ticket binding 与 Orchestrator command 类型。 |
| `blocks/approval`（L/I） | Approval ticket/approval evidence 的正式 owner。 | create/list/get/approve/reject/batch resolve。 | typed approval outcome/notification。 | 自行恢复 Run、绕过 governance dependency、跨 owner repo。 | `go test ./internal/modules/blocks/approval/... -count=1`；approve/reject 幂等、scope 校验、EventHub evidence 合同。 |
| `blocks/memory`（L/I） | Memory 与 recall trace 的正式 owner。 | CRUD/search、recall trace query。 | typed search/trace DTO/notification。 | repository query/entity 外泄、成为 Runtime evidence 裁决者。 | `go test ./internal/modules/blocks/memory/... -count=1`；limit/filter/scope mapping、slice clone、repository error 保留。 |
| `blocks/skill`（L/I） | Skill registry 的正式 owner。 | Skill CRUD/get/list/binding query。 | typed registry/default DTO。 | Hub facade、直接修改 workspace/run。 | `go test ./internal/modules/blocks/skill/... -count=1`；registry CRUD、default/binding scope、重复 lifecycle。 |
| `blocks/soul`（L/I） | Soul registry/default 的正式 owner。 | Soul CRUD/get/list/default query。 | typed Soul/default DTO。 | 直接写 Workspace default 或编排 runtime。 | `go test ./internal/modules/blocks/soul/... -count=1`；default ensure 幂等、owner DTO clone、teardown。 |
| `blocks/mcpregistry`（L/I） | MCP server/capability registry 的正式 owner。 | MCP server CRUD、capability/default query。 | typed registry/capability DTO。 | 执行 MCP 调用、直接写 workspace/run。 | `go test ./internal/modules/blocks/mcpregistry/... -count=1`；server/capability 查询、default ensure、DTO 不泄露 repo。 |
| `blocks/file`（C） | 有界 filesystem snapshot、list/read/write/edit/patch capability 与 owner receipt。 | Orchestrator typed prerequisite、sealed authorization feasibility、sealed file operation。 | typed bounded scope/selector、owner-local factual category + original diagnostic、唯一 operation receipt。 | 扩展 selector、选择 correction/action/terminal、裁决 evidence、直接更新 aggregate。 | `go test ./internal/modules/blocks/file/... -count=1`；emitted-code total mapping、original diagnostic round-trip、target policy/admissibility、receipt outcome、stale snapshot、workspace root containment。 |
| `blocks/git`（C） | Git init/add/commit 等 sealed capability 与 receipt。 | sealed authorization feasibility、sealed Git operation。 | owner-local factual category + original diagnostic、唯一 effect receipt（ExecutionKey/Fence/observed scope）。 | 选择 correction/action/terminal、合并/篡改 receipt、Status/Outcome 双权威、写 RuntimeState。 | `go test ./internal/modules/blocks/git/... -count=1`；emitted-code total mapping、original diagnostic、receipt exactly-once、init→add→commit accepted 顺序；restart fixture 覆盖 owner-commit crash point。 |
| `blocks/llm`（C） | provider/model catalog 与 Runtime candidate protocol adapter；只渲染 request 并返回 candidate/request factual observation。 | catalog/target query、sealed manifest candidate request、Orchestrator-sealed correction C4。 | typed candidate diagnostic、provider/session factual category、ceiling result。 | 从 map/V1/profile/raw reason 重建 authority、扩展 allowed set、选择 correction/terminal、覆盖原 provider-safe diagnostic。 | `go test ./internal/modules/blocks/llm/... -count=1`；manifest schema/strict round-trip、original/audit-safe 与 provider-safe 双投影、C4 directive-only、sealed ceiling、stable-prefix、provider timeout/schema failure。 |
| `blocks/mcp`（C） | 通过注册表配置执行有界 MCP capability operation。 | sealed authorization feasibility、sealed MCP operation、capability query。 | owner-local factual category + original diagnostic、唯一 operation receipt。 | 选择 correction/action/terminal、直接读取对方 service/repository、无界 transport payload、裁决运行状态。 | `go test ./internal/modules/blocks/mcp/... -count=1`；emitted-code total mapping、original diagnostic、authorization identity、registry lookup、freshness、bounded transport、receipt。 |
| `blocks/shell`（C） | 有界 Shell command/validation capability 与 receipt。 | sealed authorization feasibility、sealed shell operation。 | owner-local factual category + original diagnostic、validation observation 与唯一 receipt。 | 选择 correction/action/terminal、拼接未封存命令、直接终态化 Run、将 output 当 accepted evidence。 | `go test ./internal/modules/blocks/shell/... -count=1`；emitted-code total mapping、original diagnostic、freshness、timeout/exit、working-root containment、receipt outcome。 |
| `blocks/ssh`（C） | 有界 remote SSH capability 与 structured preflight/receipt。 | sealed authorization feasibility、sealed SSH operation。 | owner-local factual category + original diagnostic、唯一 preflight/operation receipt。 | 选择 correction/action/terminal、透传 credential、unbounded host/command、直接写 runtime。 | `go test ./internal/modules/blocks/ssh/... -count=1`；emitted-code total mapping、original diagnostic、freshness、auth/timeout、host scope、typed failure。 |
| `blocks/web`（C） | 有界 web capability，含 approval/SSRF/config revision 约束。 | sealed authorization feasibility、sealed web operation、capability configuration query。 | owner-local factual category + original diagnostic、唯一 response/operation receipt。 | 选择 correction/action/terminal、未审批网络访问、SSRF 绕过、直接做 governance 或 Result 裁决。 | `go test ./internal/modules/blocks/web/... -count=1`；emitted-code total mapping、original diagnostic、approval、SSRF、config revision、timeout/receipt。 |

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
| `orchestrator <-> run` | CAS revision/rebase、opaque recovery inventory + typed aggregate load、terminal delivery 幂等；Run 不分类 SM-2/3/4，Orchestrator 不直访 repository。 | orchestrator + run |
| `orchestrator -> workspace/llm/capability` | prerequisite owner facts 在 SM-2 编译为唯一 manifest；LLM evaluator 只回 factual diagnostic，SM-3 owner-local 决定 correction/final；capability feasibility/operation 都绑定同一 transaction/authorization/snapshot，并同时保留 stable category 与 original diagnostic；receipt outcome 唯一。 | orchestrator + workspace + llm + file/git/shell/mcp/ssh/web |
| `orchestrator <-> decision/approval/governance` | ticket/resolution/dependency binding 一次消费；Orchestrator 发布 continuation fact，Run owner-local continuation，不由 application Resume。 | orchestrator + decision + approval + governance + run |
| `orchestrator -> result -> workspace -> run` | reducer terminal 只发布 delivery fact；Result、Workspace、Run 依次 owner-local 幂等迁移并回执；delivery 失败不改写 authority terminal。 | orchestrator + result + run + workspace |
| `owner notification -> eventstream -> workorchd` | audit high-watermark 与 projection source revision 对齐；删除 owner 后的迟到事件删除 stale view 而不 degraded readiness。 | eventstream + workorchd + workspace |
| `workorchd -> process service` | stopped event 只传值；本地 wait channel 不经 EventHub；重复/立即 stopped 不丢信号。 | workorchd + internal/services/workorch |

推荐执行顺序为：先 producer/consumer 的组件级合同测试，再运行该表对应的边测试，
最后执行本矩阵的 repository gate。不得以端到端成功掩盖某一 owner 的 scope、
idempotency 或 lifecycle 单测缺失。

## ARCH-1 Deployed Completion Evidence

本节是 ARCH-1 的**完成验收索引**，不是独立 Runtime 实施计划。本矩阵只消费 Runtime 回归证据来关闭组件的 `D` 标记。`workspace_deleted_late_event`
由 EventStream/WorkOrchd 的组件合同单独验证，且不影响 Runtime authority。

部署验证使用最新构建重启服务后执行，不能调用 Activate/Resume 作为补偿，也不能
通过放宽 `/readyz` 掩盖投影问题：

| Fixture | 初始 persisted 状态 | 期望结果 | 关键否定断言 |
| --- | --- | --- | --- |
| `active_no_dispatch` | active aggregate，无 sealed dispatch | 已通过：自动转 `provider_submission_unknown` terminal | candidate/dispatch 不新增；不需要人工 Resume |
| `active_sealed_dispatch` | sealed dispatch，owner receipt 前停止 | 已通过：exact execution identity 转 `outcome_unknown` | 不重复 durable effect，不覆盖 owner-committed receipt |
| `terminal_delivery_pending` | reducer terminal，Result/Run/Workspace 尚未全投影 | 已通过：只补齐 delivery 并释放 activeRun | 不重算 authority，不把 terminal 改为 drive failure |
| `workspace_deleted_late_event` | workspace 已删除，投影收到迟到 owner notification | **已通过**：真实 `DELETE` 后 `/workspace/deleted` 已持久化，后到 rebuild、`/healthz=200`、`/readyz=200`、projection ready，stale view 已删除 | 双 `ErrNotFound` 不增加 failed rebuild，且不标记 projection degraded |

前三项已通过 `make verify-runtime-c7-scripted` 的 fresh managed manifest 执行。第四项已通过：

```text
WORKORCH_MANAGED_SKIP_PREFLIGHT=1 \
WORKORCH_MANAGED_VERIFY_TARGET=verify-workspace-projection-switch \
WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 \
./scripts/verify-workorch-managed-full.sh
```

执行日期为 2026-07-29，外部 artifact root 为 `/tmp/tmp.agyoONtjYb`；同时通过
`make verify-runtime-positive-convergence-file`，确认该收敛修复未破坏正常 activation。

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
make verify-runtime-positive-convergence-file
make release-check
```

执行 `rg` 的 CI/本地脚本必须以 `rg` 可用为前提；为最小构建环境保留
`grep -R` fallback，且 fallback 必须覆盖相同的 boundary patterns。不得因为工具
缺失跳过 `verify-boundary-contract`。

组件达到 `C` 后，更新：

1. 本文件对应卡片的状态与证据；
2. 主 remediation 方案的 Closure Matrix；
3. `current-state.md`、`current-unclosed-matrix.md` 与 active overview；
4. ARCH-1 已全部达到 C；本矩阵归档后只保留命令与外部 artifact root，不提交 runtime 数据。
