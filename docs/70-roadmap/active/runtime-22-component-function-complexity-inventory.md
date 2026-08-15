# WorkOrch 22 个组件功能与复杂度盘点

Status: active architecture inventory; listed code/automated closure complete

Type: component-function-complexity-inventory

Last Updated: 2026-08-16

Snapshot: `c1b90039`

本文基于已提交且干净的 `c1b90039`，重新盘点当前 22 个 framework 运行单元。盘点只识别后续可维护性
收口点，不新增 Module、Block、Initiator、aggregate、状态机或 Runtime 生产入口，也不重新打开已经完成的
O0-O8、WD0-WD8 code/automated closure。所有后续动作仍以
[`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md) 和
[`kernel/orchestrator/readme.md`](../../../internal/modules/kernel/orchestrator/readme.md) 为状态机与实现边界。

部署后的 HTTP/SSE/CLI regression 与 SO-MC-10 fresh-data live proof 仍按 active roadmap 独立验收；它们不是
本次静态复杂度盘点可以替代的证据。

## 1. 结论

1. 当前分类仍是 **1 Initiator + 4 Modules + 17 Blocks = 22 个运行单元**，没有发现需要新增或合并
   framework unit 的充分理由。
2. 复杂度高度集中：`application/workorchd` 与 `kernel/orchestrator` 合计占 66.9% 的非测试 Go LOC，前者是
   应用功能面和文件碎片化，后者是 SM-2/SM-3/SM-4 语义及不变量密度；二者不能用同一种拆分方式处理。
3. 当前 42 个生产函数的圈复杂度大于 30，其中 Orchestrator 25 个、Workorchd 8 个；其余 9 个分布在
   Workspace、Run、Decision、Approval、Memory、LLM 和 File。状态 Owner 中最明确的低风险切片是把超宽
   command router 按资源/命令族拆成 owner-private handler，同时保留一个 Biz、一个 private reducer 和一个
   repository/CAS 边界。
4. `shell`、`llm`、`mcp`、`file`、`git` 的主要问题是单文件同时承载 policy、transport、preflight、effect
   observation、diagnostic 和 receipt mapping。即使单个函数圈复杂度不高，变更冲突面和外部 I/O 风险仍然高。
5. 后续简化必须是 **同一 Owner 内的职责物理收敛**。不应把 Orchestrator 拆成多个 Module，不应把
   Workspace/Goal/DoD/Template 拆成多个正式 Owner，也不应建立通用 capability executor。

## 2. 范围与方法

### 2.1 22 个运行单元

- Initiator：`internal/initiators/workorchruntime`。
- Modules：`internal/modules/application/workorchd`、`internal/modules/kernel/governance`、
  `internal/modules/kernel/orchestrator`、`internal/modules/kernel/system`。
- Blocks：`approval`、`decision`、`eventstream`、`file`、`git`、`llm`、`mcp`、`mcpregistry`、`memory`、
  `result`、`run`、`shell`、`skill`、`soul`、`ssh`、`web`、`workspace`。

`internal/modules/base/biz` 是共享 Biz 基座，不是运行单元；`application/workorch`、
`internal/services/workorch`、`internal/pkg` 和 Web 前端也不计入 22 个单元。

### 2.2 定量口径

- 文件/LOC：Git 已跟踪的 `*.go`；“非测试”指文件名不以 `_test.go` 结尾，因此也包含独立的 `biz/testkit`
  package，便于反映完整维护面。
- package 数：含非测试 Go 文件的唯一目录数。
- 最大文件：按 `wc -l` 统计的最大非测试 Go 文件。
- `CC>30`：`golangci-lint v2.10.1` 的 `gocyclo`，使用 `--tests=false`；表中记为“问题函数数/最大值”。
- `X 出/入`：通过 `go list` 统计 22 个单元之间的直接生产 import 边；Workorchd 的 `biz/testkit` 被排除。
  import 边包括 typed event/model contract，不等价于越权 mutation。
- 圈复杂度只衡量控制流分支；一个 1,900 行的规则文件可以没有 `CC>30`，但仍有物理内聚和变更冲突问题。

当前统一基线为：

- 非测试 Go：1,029 个文件、125,586 LOC；
- 测试 Go：360 个文件、112,921 LOC；
- 合计：1,389 个文件、238,507 LOC；
- 含非测试 Go 的 package 目录：148 个；
- 生产 `CC>30`：42 个函数，分布在 9 个运行单元；
- 大于等于 500 行的非测试 Go 文件：50 个。

### 2.3 复杂度评分

每个单元按 1-5 分评估，分数是结构风险，不是性能分数：

| 维度 | 含义 |
|---|---|
| F | 功能面：资源、协议、用例和领域规则的数量 |
| A | 状态面：正式聚合、状态转换、CAS、恢复或证据权威密度 |
| X | 交互面：typed contract、EventHub fan-in/fan-out 和 scope 数量 |
| O | 运行风险：外部 I/O、并发、超时、幂等、恢复和安全边界 |
| V | 维护体量：LOC、文件/包碎片化、热点文件和测试矩阵 |

总分分级：5-9 为低（L），10-14 为中（M），15-19 为高（H），20-25 为极高（VH）。

## 3. 当前定量基线

`CC>30` 为“数量/最大圈复杂度”，`X` 为“直接生产 fan-out/fan-in”。最大文件路径相对于组件根目录。

| 单元 | 类型 | 非测试文件/LOC | 测试文件/LOC | 包 | 最大文件（LOC） | CC>30 | X 出/入 |
|---|---|---:|---:|---:|---|---:|---:|
| `workorchruntime` | Initiator | 2 / 329 | 1 / 123 | 2 | `module.go`（241） | 0 / - | 0 / 0 |
| `application/workorchd` | Module | 650 / 56,173 | 153 / 41,574 | 38 | `biz/appservice/workspaceviewcore/store.go`（1,647） | 8 / 89 | 16 / 0 |
| `kernel/governance` | Module | 11 / 2,506 | 5 / 2,135 | 5 | `biz/service.go`（1,909） | 0 / - | 6 / 2 |
| `kernel/orchestrator` | Module | 104 / 27,830 | 91 / 45,925 | 12 | `biz/runtime/semantic_planning.go`（1,642） | 25 / 131 | 13 / 11 |
| `kernel/system` | Module | 4 / 212 | 1 / 120 | 3 | `biz/system.go`（119） | 0 / - | 4 / 0 |
| `blocks/approval` | Block | 13 / 985 | 3 / 1,165 | 6 | `biz/owner.go`（348） | 1 / 53 | 0 / 5 |
| `blocks/decision` | Block | 17 / 2,102 | 4 / 1,904 | 6 | `biz/owner.go`（651） | 1 / 68 | 1 / 4 |
| `blocks/eventstream` | Block | 11 / 1,855 | 5 / 1,002 | 6 | `pkg/recordpayload/payload.go`（842） | 0 / - | 8 / 1 |
| `blocks/file` | Block | 18 / 4,073 | 7 / 2,388 | 4 | `biz/file_runtime_adapter.go`（1,067） | 1 / 36 | 1 / 2 |
| `blocks/git` | Block | 16 / 2,450 | 8 / 1,345 | 4 | `biz/git_runtime_adapter.go`（1,026） | 0 / - | 2 / 2 |
| `blocks/llm` | Block | 24 / 7,155 | 15 / 3,631 | 7 | `biz/runtime_candidate_provider.go`（1,873） | 2 / 40 | 1 / 4 |
| `blocks/mcp` | Block | 16 / 2,698 | 8 / 2,136 | 4 | `biz/mcp_runtime_adapter.go`（1,389） | 0 / - | 2 / 4 |
| `blocks/mcpregistry` | Block | 11 / 1,118 | 3 / 511 | 5 | `biz/mcp_service.go`（382） | 0 / - | 1 / 3 |
| `blocks/memory` | Block | 11 / 1,200 | 4 / 588 | 5 | `pkg/events/messages.go`（361） | 1 / 46 | 0 / 5 |
| `blocks/result` | Block | 13 / 1,163 | 4 / 524 | 5 | `pkg/events/messages.go`（251） | 0 / - | 0 / 3 |
| `blocks/run` | Block | 21 / 3,765 | 15 / 2,508 | 5 | `biz/owner.go`（819） | 1 / 89 | 4 / 5 |
| `blocks/shell` | Block | 18 / 3,503 | 11 / 2,611 | 4 | `biz/shell_runtime_adapter.go`（1,977） | 0 / - | 1 / 1 |
| `blocks/skill` | Block | 9 / 447 | 3 / 222 | 5 | `biz/owner.go`（129） | 0 / - | 0 / 1 |
| `blocks/soul` | Block | 10 / 656 | 3 / 343 | 5 | `biz/owner.go`（186） | 0 / - | 0 / 2 |
| `blocks/ssh` | Block | 14 / 1,030 | 5 / 414 | 4 | `biz/ssh_runtime_adapter.go`（366） | 0 / - | 1 / 1 |
| `blocks/web` | Block | 16 / 1,928 | 5 / 739 | 6 | `biz/runtime_adapter.go`（863） | 0 / - | 2 / 2 |
| `blocks/workspace` | Block | 20 / 2,408 | 6 / 1,013 | 7 | `biz/owner.go`（624） | 2 / 118 | 1 / 6 |

### 3.1 体量和控制流集中点

1. `workorchd` 占非测试 LOC 的 44.7%，Orchestrator 占 22.2%；二者测试 LOC 合计 87,499，占 22 单元
   测试 LOC 的 77.5%。这说明变更风险集中，但也说明核心路径已经有较厚回归矩阵。
2. Workorchd 有 249 个不超过 40 行的非测试文件和 15 个至少 500 行的文件，仍同时存在碎片化和局部大文件。
   `readmodelprojection` 已从历史 431 个文件归并为 140 个文件、17,087 LOC、8 个 exported declaration；该成果
   不应通过重新增加公开 helper 或数字化临时文件名来逆转。
3. Orchestrator 有 11 个至少 500 行的文件。最大控制流热点是唯一
   `RuntimeController.ApplyInternalEvent`（CC 131），但它的 exhaustive dispatcher 本身是 Owner 边界要求；不能
   为追求 CC 数字改成动态 registry、reflect、callback transaction 或多个 mutation entry。
4. 状态 Owner 的 command router 是可直接收敛的热点：Workspace `executeCommand` CC 118、Run CC 89、Decision
   CC 68、Approval CC 53、Memory CC 46。它们可以按具体命令族委派给 private handler，而不改变 reducer/CAS。
5. 单文件维护热点前三位是 Shell adapter 1,977 行、Governance service 1,909 行、LLM candidate provider
   1,873 行。Shell 和 Governance 没有 `CC>30` 生产函数，说明问题主要是职责物理混放，而非单个深分支函数。
6. 22 单元当前没有 Go `Benchmark*`。因此后续“性能不退化”不能只靠 LOC/CC；需要先冻结 owner call、repository
   load/CAS、provider physical call、projection rebuild 和分配热点，再按需补 focused benchmark。

### 3.2 依赖扇出解释

- Workorchd 生产 fan-out 为 16，属于应用组合层的天然宽度；应通过七组 named `ApplicationPorts` 和 typed owner
  contract 管理，不能用综合 `RuntimeService` 隐藏。
- Orchestrator fan-out/fan-in 为 13/11，是 Runtime 收敛中心；依赖数量本身不能通过迁移状态到 capability owner
  来降低。
- Eventstream fan-out 为 8，是审计 payload 映射成本，不是业务编排权；它不能依据 EventRecord 修复 owner state。
- Workspace fan-in 为 6，Run/Approval/Memory fan-in 各为 5；这些公共 owner contract 的 DTO、topic 或 scope 变化
  具有较大爆炸半径，内部简化应优先保持外部合同不变。

## 4. 逐组件功能与复杂度判定

评分列为 `F/A/X/O/V = 总分`。

| 单元 | 单一职责/Owner 闭环 | 评分 | 当前判定与下一步内聚方向 |
|---|---|---:|---|
| `workorchruntime` | repository provider 创建、按 Owner 分组暴露和生命周期清理 | `1/1/2/2/2 = 8 L` | 边界稳定。保持一个 Initiator；`module.go` 只可按 backend builder/repository group mapping 拆 private helper，不引入 Hub、policy 或综合 runtime container。 |
| `application/workorchd` | HTTP/Web/CLI、execution use case、runtime session、projection/read model、SSE 和生命周期接线 | `5/2/5/4/5 = 21 VH` | 正式 Owner 边界已关闭，剩余是应用维护面。优先拆 `ApplicationPorts.Validate` 的七组校验、Workspace route registration、`workspaceviewcore.Store` 的 revision/update/query/clone 文件和纯 projection comparator；仍保持一个 Module、一个 projection manager/store。 |
| `kernel/governance` | 自动治理评估、resolve/escalate/block 与 AutoGovernanceTrace | `3/3/5/3/3 = 17 H` | `service.go` 1,909 行但无高 CC 函数，适合按 worker/reconciliation、ticket policy、approval policy、trace repository、payload parsing 分文件；Service、trace Owner 和 EventHub 生命周期不拆。 |
| `kernel/orchestrator` | SM-2 authority/focus、SM-3 candidate/dispatch、SM-4 drive/recovery/delivery | `5/5/5/5/5 = 25 VH` | O0-O8 已关闭 Owner/CAS/内部 facet 边界。下一步只处理 `RuntimeAggregate.Validate`、`commitEvidence`、prerequisite result、execution coordinator 等纯不变量/单事务热点；唯一 exhaustive dispatcher、aggregate、mutation kernel 和 coordinator 必须保留。 |
| `kernel/system` | 启动时幂等协调 Soul、WorkspaceTemplate、MCP defaults | `2/1/4/2/1 = 10 M` | 功能窄且独立生命周期明确，无拆分收益。defaults 继续使用 typed data/command，禁止 repository 直连。 |
| `blocks/approval` | Approval 请求、批准/拒绝、supersede、查询和通知 | `2/3/3/2/2 = 12 M` | `executeCommand` CC 53。把 decode/scope、query 和 approved/rejected/delete 命令委派拆文件；所有状态选择仍只进 Approval-private lifecycle event/reducer。 |
| `blocks/decision` | DecisionTicket 与 DecisionResolution 两阶段生命周期及 Runtime governance proposal 交接 | `3/4/4/2/3 = 16 H` | `executeCommand` CC 68。按 ticket query、resolution transition、runtime-governance adapter 分 owner-private handler；可恢复两阶段写和单 Owner 不变。 |
| `blocks/eventstream` | EventRecord append/order/high-watermark/query/purge | `3/3/5/3/3 = 17 H` | 复杂度集中在 842 行 `recordpayload` 映射。按 workspace/run、governance、result/memory、orchestrator 事件族分纯 codec 文件；保持一个 wire version/index 和一个 append authority。 |
| `blocks/file` | workspace-local target/snapshot/list/read/create/edit/patch/revert、feasibility 和 receipt | `4/3/4/5/4 = 20 VH` | 按 target selection、catalog/admissibility、feasibility/replay、path guard/content helper 拆 adapter 文件；create-only 和 revision-bound replacement 继续来自同一 File policy，副作用只经 File-private effect event。 |
| `blocks/git` | 受控 Git read/write、repository-state prerequisite 与 durable receipt | `3/3/4/5/3 = 18 H` | 将 1,026 行 adapter 按 repository inspection、read operation、write operation、capability-state receipt 分文件；不能借 Shell 执行来合并 Owner。 |
| `blocks/llm` | ai-proxy catalog/cache、ceiling、Responses/embedding、semantic/candidate request 和 provider fact | `5/3/5/5/4 = 22 VH` | 将 1,873 行 candidate provider 按 request session、context projection、closed schema、structured instruction、retry diagnostic 分文件；transport/catalog/cache 独立，C0-C2 和 physical call 语义不变。 |
| `blocks/mcp` | 已注册 MCP tool/resource/prompt 的 HTTP/stdio 调用和结果规范化 | `4/3/4/5/3 = 19 H` | 将 1,389 行 adapter 按 HTTP、短/长生命周期 stdio session、JSON-RPC codec、result normalization 分文件；Registry 仍是独立 Block，MCP 不解释 coverage。 |
| `blocks/mcpregistry` | MCPServer/MCPCapability registry 与运行时元数据 | `3/3/3/2/2 = 13 M` | 边界稳定。只整理 server/capability mapping、clone 和 defaults helper；不与 MCP execution 合并。 |
| `blocks/memory` | MemoryItem 与 MemoryRecallTrace owner | `3/3/3/2/2 = 13 M` | `executeCommand` CC 46。按 item CRUD/search 与 recall-trace handler 拆文件；同一 Memory Biz、repository group 和 notification authority 不变。 |
| `blocks/result` | sealed terminal Result/Artifact formal outcome 和 delivery acknowledgement | `3/3/3/2/2 = 13 M` | lifecycle 与 query 已分开，当前只需 clone/DTO 清理。Result 继续只存 Orchestrator 已判断 outcome，不做 evidence quality 判定。 |
| `blocks/run` | SM-1 Run lifecycle、lineage及 opaque RuntimeStateRecord CAS/inventory | `4/5/5/4/4 = 22 VH` | `executeCommand` CC 89。把 Run/Step/Checkpoint query、opaque Runtime CAS、purge/delete 分 owner-private handler；SM-1 lifecycle、inventory readiness 与 opaque persistence 不得互相解释。 |
| `blocks/shell` | 受控本地命令、policy、workspace effect、validation recipe 和 receipt | `4/3/4/5/4 = 20 VH` | 1,977 行 adapter 按 command policy、runner/output bounds、effect snapshot、validation evidence、diagnostic 分文件；零 `CC>30` 不表示无需拆，且不能新增共享可变 executor。 |
| `blocks/skill` | Skill registry、instruction、metadata、binding 和 provenance | `2/2/2/1/1 = 8 L` | 低复杂度，删除重复 clone/薄 forwarding 即可；不与 Soul 合并。 |
| `blocks/soul` | Soul catalog、stage/capability/risk policy 和 defaults | `3/2/3/2/2 = 12 M` | 保持一个 registry Block；集中 policy validation/defaults，不将 guidance 升格为 Runtime authority。 |
| `blocks/ssh` | 结构化 remote target/session/credential refs 与受控 command receipt | `3/2/4/5/2 = 16 H` | 当前体量可控；只在增长时拆 target/session validation、transport、diagnostic，不能复制 Shell 本地 policy 或泄漏 secret。 |
| `blocks/web` | 受控 search/fetch、source/raw ref、bounded content 与授权查询 | `4/2/4/5/3 = 18 H` | 将 provider transport、content bounder/result normalization、Approval query adapter 分文件；仍只返回外部事实，不写 Memory/Result。 |
| `blocks/workspace` | Workspace/Goal/DoD/Template definition 聚合、revision 和 active Run slot | `4/4/4/2/3 = 17 H` | `executeCommand` CC 118 为状态 Block 最大热点。按 Workspace、Goal、DoD、Template command family 拆 private handler，再单独保留 execution intent/lifecycle reducer；不能拆成四个 Owner。 |

## 5. 下一轮收口队列

### P0：状态 Owner 与核心入口热点

#### C22-01 Workspace command router（code/automated closed）

- 只移动 `blocks/workspace/biz/owner.go` 的具体 command 分支到
  `workspace_commands.go`、`goal_commands.go`、`dod_commands.go`、`template_commands.go` 等语义文件。
- `handleCommand -> executeCommand` 仍是唯一外部 command ingress；每个 mutation 仍提交 Workspace-private event。
- 保持 repository 次数、notification topic、scope 校验、definition revision 和 active slot reducer 不变。
- 目标：`executeCommand` 不再同时实现四类资源行为；不要求用 map registry 把圈复杂度“做成零”。

完成记录（2026-08-16）：

- 唯一 ingress 保持 `handleCommand -> executeCommand`；静态穷尽分类器只在同一 Workspace Biz 内委派到
  Workspace、Goal、DoD、Template 四个 private handler，未知 Topic fail-closed。没有新增 Module、Block、
  repository/CAS、EventHub round trip、mutation entry 或 forwarding package。
- 非测试 Go 从 `20 / 2,408 LOC` 变为 `25 / 2,535 LOC`，测试从 `6 / 1,013 LOC` 变为
  `7 / 1,067 LOC`；package 保持 7，最大生产文件从 `biz/owner.go` 624 行降为
  `biz/goal_dod_manager.go` 331 行。
- 生产 `CC>30` 从 `2 / 118` 降为 `1 / 32`；剩余项是本切片未修改的 `executeExecutionIntent`。exported
  declaration 保持 66，事件 Topic 保持 29，直接组件依赖与 Workspace Owner 权限边界不变。
- 资源状态机 affected：Workspace definition/resource owner；SM-1、SM-2、SM-3、SM-4 语义变化均为
  `none`。原有上下文绑定、repository 调用/CAS 顺序、notification 和 lifecycle reducer 保持不变。
- 已通过 `go test ./internal/modules/blocks/workspace/... -count=1`、Workorchd/Run/Orchestrator 消费方测试、
  `make fmt-check` 和 `make verify-boundary-contract`。

#### C22-02 Run command router（code/automated closed）

- 分离 Run query、Step/Checkpoint query、opaque RuntimeStateRecord CAS、purge/delete handler。
- 保持 SM-1 lifecycle handler、startup recovery inventory/readiness 和 Runtime opaque store 三条职责边界。
- Run 仍不得解码 SM-2/SM-3/SM-4 aggregate 来选择 lifecycle。

完成记录（2026-08-16）：

- 唯一 ingress 与锁边界保持 `handleCommand -> executeCommand` 和一次 `commandMu.RLock`；静态穷尽分类器在
  同一 Run Biz 内委派到 create、Run query、Step query、Checkpoint query、opaque Runtime state store、
  resource cleanup 六个 private handler，未知 Topic fail-closed。
- 非测试 Go 从 `21 / 3,765 LOC` 变为 `27 / 3,919 LOC`，测试从 `15 / 2,508 LOC` 变为
  `16 / 2,553 LOC`；package 保持 5，最大生产文件从 `biz/owner.go` 819 行降为
  `biz/workspace_execution_intent.go` 547 行。
- 生产 `CC>30` 从 `1 / 89` 降为 0；exported declaration 保持 61，事件 Topic 保持 31，直接组件依赖、
  owner mutation entry 与 EventHub 合同均未变化。
- affected：SM-1 Run resource owner 与 SM-4 的 opaque persistence transport；changed semantics：`none`。
  lifecycle private reducer、startup recovery inventory/readiness、source authentication、repository load/CAS、最多四次
  physical rebase、purge/delete scope fence 的顺序均保持不变；Run 没有新增任何 SM-2/SM-3/SM-4 解码。
- 已通过 `go test ./internal/modules/blocks/run/... -count=1`、Workorchd/Orchestrator 消费方测试、
  `make fmt-check` 和 `make verify-boundary-contract`。

#### C22-03 Orchestrator invariant/transaction hotspots（code/automated closed）

- `RuntimeAggregate.Validate` 按 activation、SM-2、SM-3、SM-4、cross-machine fence 调用 private validator；顶层
  仍是一次完整 aggregate validation。
- `commitEvidence` 分成 receipt identity admission、evidence derivation、CWI credit finding 和单次 transition commit；
  不得增加 repository load/CAS。
- `applyPrerequisiteResult` 按 prerequisite kind 拆纯 handler；`executeActiveDispatch` 只拆 preflight、owner call、
  receipt projection，execution claim/recovery budget handoff 保持原子顺序。
- `ApplyInternalEvent` 允许把 payload envelope validation 与四个 family 的 private route 分开，但必须继续是唯一
  exported mutation dispatcher 和穷举 kind gate，禁止动态 registry/callback transaction。

完成记录（2026-08-16）：

- `RuntimeAggregate.Validate` 仍是唯一完整 aggregate validation，并按既有 fail-fast 顺序调用 activation、SM-2、
  SM-3 active dispatch/activation fence/candidate transaction/governance、SM-4 delivery/drive lease、repair 与
  cross-machine fence 的 private validator；每个 phase 的非法状态及完整合法 aggregate 均有回归覆盖。
- evidence transaction 已拆为 persisted receipt admission、operation/scope matching、fact derivation、exact-CWI credit、
  accepted/rejected reduction 与 dispatch completion；所有 aggregate mutation 仍只经一次
  `commitEvidenceTransition`。性能断言冻结 committed 为 `1 load / 1 CAS`、duplicate 为 `1 / 0`、stale 为
  `1 / 1`，没有增加 repository round trip。
- prerequisite result 按 9 个 kind 委派 owner-private 纯 handler；active dispatch 使用同一 transaction-scoped private
  execution object 顺序执行 preflight、operation claim/recovery、capability owner call 与 receipt projection。单 operation
  路径继续是 `1 owner call / 4 repository loads / 3 CAS`，exact-fence recovery 与 budget handoff 次序未改变。
- `ApplyInternalEvent` 仍是唯一 exported mutation dispatcher，并在同一个静态 switch 中显式穷举 58 个 kind；信封校验
  后只进入 Activation、Authority、Transaction、Delivery 四个 package-private family route。AST 契约冻结每个 kind
  恰好进入一个 payload family，未知 kind fail-closed；没有 map registry、reflection、callback transaction、第二个
  mutation entry、repository/CAS 或 EventHub round trip。
- 非测试 Go 从 `104 / 27,830 LOC` 变为 `118 / 28,524 LOC`，测试从 `91 / 45,925 LOC` 变为
  `94 / 46,273 LOC`；package 保持 12，最大生产文件仍为 `biz/runtime/semantic_planning.go`（1,642 行），至少
  500 行的生产文件从 11 个降为 10 个。新增文件均是同 package 的 invariant/transaction phase 或 route 实现，
  没有 forwarding package。
- 生产 `CC>30` 从 `25 / 131` 降为 `20 / 60`；本工作包指定的 `Validate`、`commitEvidence`、
  `applyPrerequisiteResult`、`executeActiveDispatch`、`ApplyInternalEvent` 五个热点全部退出阈值。exported declaration、
  EventHub topic、owner mutation entry、直接跨单元 import 边与 persisted schema 均保持不变。
- affected：SM-2、SM-3、SM-4；SM-1 不受影响；changed semantics：`none`。已通过 Runtime focused transaction/
  invariant/dispatcher tests、`go test ./internal/modules/kernel/orchestrator/... -count=1`、`make fmt-check` 和
  `make verify-boundary-contract`。

#### C22-04 Workorchd pure/transport hotspots（code/automated closed）

- 将 `ApplicationPorts.Validate` 委派给七组 named port validator。
- 将 Workspace read/write route registration 按 endpoint family 拆语义函数，不改 route、DTO 或 status code。
- 将 `workspaceviewcore/store.go` 按 lifecycle/status、revision、mutation、query、metrics、clone 分文件；`Store`
  和锁/revision/high-watermark 仍唯一。
- 只对 projection 的 pure diff/equality/normalization helper 做物理收敛；保持 8-symbol export whitelist。

完成记录（2026-08-16）：

- `ApplicationPorts.Validate` 按 Workspace、Definition、Run、Governance、Registry、Memory、Configuration 七组
  named validator 顺序委派，保留原字段顺序与首个 missing-port diagnostic。reflection-based contract test 从完整端口集
  逐一清空每个 typed callback，确认全部字段继续 fail-closed；没有 callback registry、EventHub/repository handle 或
  新 application facade。
- Workspace read route 按 resource、projection、context、definition family 注册；write route 按
  create、inference、delete、execution、context、definition family 注册。顶层仍按原顺序调用，所有 path、method、
  handler body、DTO、metric、status code 与 projection refresh 次序不变，native route 集合回归通过。
- `workspaceviewcore.Store` 与唯一 `sync.RWMutex` 仍只在 `store.go` 声明，lifecycle/status、revision、mutation、query、
  metrics、clone 分为六个语义文件；`advanceEntryRevision`、`advanceGlobalRevision`、revision epoch/high-watermark、
  stream generation 和失败集合各保持单一实现。AST 契约冻结 `1 Store / 1 RWMutex / 1+1 revision authority`，既有
  restart、stale、duplicate/coalescing、generation recovery 与 source watermark 测试全部通过。
- `readmodelprojection` 将 workspace runtime view、typed section diff 与 equality 物理分开，并把 diff、LLM usage
  summary equality、convergence status normalization 分解为无状态 private phase。公开面仍精确为 8 个 declaration，
  没有新增 projection store、I/O、JSON/reflect authority 或数字化临时文件名。
- 非测试 Go 从 `650 / 56,173 LOC` 变为 `659 / 56,414 LOC`，测试从 `153 / 41,574 LOC` 变为
  `156 / 41,726 LOC`；package 保持 38，最大生产文件从 `workspaceviewcore/store.go`（1,647 行）降为
  `readmodelprojection/projection_readmodel_views.go`（1,102 行），至少 500 行的生产文件从 15 个降为 14 个。
  server 为 `42 / 8,230 LOC`，readmodelprojection 为 `142 / 17,187 LOC`。
- 生产 `CC>30` 从 `8 / 89` 降为 `2 / 45`；本工作包列出的 ports、两组 Workspace route、diff/equality/
  normalization 热点均退出阈值。剩余两个是未被本切片列入的 activation coverage compiler（45）与 run GET handler
  （32），不以扩大 C22-04 transport/projection 范围顺带修改。
- focused benchmark `BenchmarkStoreUpsertSnapshotAndRead` 在 `100x` 三次运行中稳定为 `58 allocs/op`、约
  `47.8 KB/op`；既有 manager 回归继续冻结 full rebuild 的 detail fan-out 为 0、相同 workspace 的 pending rebuild
  合并为一次且不同 workspace 独立。affected：application transport/derived projection；SM-1..SM-4 authority 不变；
  changed semantics：`none`。已通过 Workorchd 全量测试、projection export/structure tests、`make fmt-check`。

### P1：能力 Block 与协调/审计大文件

#### C22-05 Capability adapters（code/automated closed）

每次只处理一个 Owner，并保持 `typed preflight -> owner-private effect -> typed receipt`：

1. LLM（code/automated closed）：candidate request/context/schema/instruction/diagnostic；
2. File（code/automated closed）：target/catalog/feasibility/path guard；
3. Shell（code/automated closed）：policy/runner/effect/validation/diagnostic；
4. MCP（code/automated closed）：HTTP/stdio session/JSON-RPC/result；
5. Git（code/automated closed）：inspection/read/write/capability-state receipt；
6. Web/SSH（code/automated closed）：仅在前五项稳定后做 transport/normalization 清理。

LLM 完成记录（2026-08-16）：

- `runtime_candidate_provider.go` 已按 provider transaction、request、context、contract/instruction、closed schema
  拆分；semantic plan 同步按 admission/finalization、prompt、projection、instruction、diagnostic、integrity 拆分。
  `runtimeProviderRequest` 保持唯一静态 document-kind ingress，所有新增 helper 都是同一 LLM Biz 内的 private 纯函数；
  没有新增 owner、状态、EventHub topic、repository、provider client、mutation entry 或 forwarding package。
- C0～C5 的 section ID/顺序、stable/volatile/protected 属性、sealed stable prefix、provider ceiling、budget/context
  integrity 顺序和 audit-safe/provider-safe diagnostic 分界均由既有回归继续冻结；`GenerateCandidate` 仍只有一次
  `completeProviderWithSealedCeiling` 物理 provider call。affected：LLM capability request rendering；SM-1～SM-4
  changed semantics：`none`。
- 非测试 Go 从 `24 / 7,155 LOC` 变为 `34 / 7,323 LOC`，测试从 `15 / 3,631 LOC` 变为
  `16 / 3,653 LOC`；package 保持 7，最大生产文件从 `biz/runtime_candidate_provider.go` 1,873 行降为
  `biz/owner.go` 1,026 行，至少 500 行生产文件从 4 个降为 3 个。生产 `CC>30` 从 `2 / 40` 降为
  `1 / 39`；剩余 `readResponsesStream` 是本切片之外的 HTTP streaming protocol parser。exported declaration、
  11 个 EventHub topic和直接组件依赖均未改变。
- 新增 focused benchmark `BenchmarkRuntimeSemanticPlanProviderRequest`；100 次、3 轮样本为
  `618,084～628,553 ns/op`、`457,319～460,374 B/op`、`5,855～5,856 allocs/op`。已通过
  `go test ./internal/modules/blocks/llm/... -count=1` 与 production `gocyclo` 核对；全仓门禁在 C22-05 各 Owner
  结束后统一执行。

File 完成记录（2026-08-16）：

- 唯一 `RuntimeAdapter` 已按 snapshot/manifest、target selection、admissibility catalog、feasibility/replay proof、
  path guard、content helper 分成同 package 的 owner-private 文件。target selection 内部再按 request normalization、
  selector admission、prospective selector、profile candidates 与 manifest cardinality policy 分段；没有新增 Block、
  service facade、mutation/effect entry、EventHub topic、repository/CAS 或跨 capability 可变 helper。
- create-only 与 revision-bound replacement 仍来自同一 File catalog/feasibility policy：`file/write` 只允许
  `prospective_owner_target`，已有目标只能使用允许 exact/existing policy 的 `file/edit` 或 `patch/apply` variant，
  且 replacement 继续绑定 owner-observed revision/baseHash。focused benchmark 的完整 target/catalog/feasibility
  链路包含 3 次 bounded manifest observation、0 次 File-private effect、0 次 repository load/CAS，并在结束时核对
  文件内容未变化。affected：File capability read-only preflight；SM-1～SM-4 changed semantics：`none`。
- 非测试 Go 从 `18 / 4,073 LOC` 变为 `24 / 4,166 LOC`，测试从 `7 / 2,388 LOC` 变为
  `8 / 2,448 LOC`；package 保持 4，最大生产文件从 `biz/file_runtime_adapter.go` 1,067 行降为
  `biz/owner.go` 829 行，至少 500 行生产文件从 4 个降为 3 个，生产 `CC>30` 从 `1 / 36` 降为 0。
  exported declaration、15 个 EventHub topic和直接组件依赖均未改变。
- `BenchmarkRuntimeAdapterTargetCatalogFeasibility` 的 100 次、3 轮样本为
  `121,240～137,759 ns/op`、`44,929～45,016 B/op`、`766 allocs/op`。既有回归继续覆盖 selection
  cardinality、unsupported/noncanonical target、stale observation、create-only catalog、replacement variant、baseHash、
  replay proof 和无副作用 probe；已通过 `go test ./internal/modules/blocks/file/... -count=1` 与 production gocyclo。

Shell 完成记录（2026-08-16）：

- 唯一 `RuntimeAdapter` 已按 catalog/admissibility、validation preflight、feasibility、execution transaction、
  workspace effect observation、managed resource、runner/output bound、policy、diagnostic 分为同 package 的
  owner-private 文件。没有新增 Block、service facade、共享可变 executor/registry、mutation entry、EventHub topic、
  repository/CAS、retry 或 receipt owner。
- `ExecuteOperation` 的 policy/evidence/timeout/validation preflight、before observation、物理 process、after observation、
  typed diagnostic/receipt 顺序保持不变。AST 回归冻结唯一 execution transaction 中恰好一处
  `exec.CommandContext`、一处 `Start` 和一处 `Wait`，防止物理命令调用被拆成多条 effect path。affected：Shell
  capability execution；SM-1～SM-4 changed semantics：`none`。
- 非测试 Go 从 `18 / 3,503 LOC` 变为 `26 / 3,600 LOC`，测试从 `11 / 2,611 LOC` 变为
  `13 / 2,680 LOC`；package 保持 4，最大生产文件从 `biz/shell_runtime_adapter.go` 1,977 行降为
  `biz/owner.go` 598 行，至少 500 行生产文件从 2 个降为 1 个，生产 `CC>30` 继续为 0。exported declaration、
  6 个 EventHub topic和直接组件依赖均未改变。
- `BenchmarkRuntimeAdapterExecuteBoundedCommand` 的 20 次、3 轮样本为
  `531,749～538,546 ns/op`、`78,664～79,141 B/op`、`147～149 allocs/op`；每次迭代恰好执行一个前台
  process，repository load/CAS 与 EventHub 调用均为 0。既有测试继续覆盖危险命令、timeout/process group、
  output bounds、state-changing no-progress/effect mismatch、managed resource cleanup、validation preflight/evidence
  和安全 diagnostic；已通过 `go test ./internal/modules/blocks/shell/... -count=1` 与 production gocyclo。

MCP 完成记录（2026-08-16）：

- 唯一 `RuntimeAdapter` 已按 core ingress/catalog、HTTP transport、short/long-lived stdio operation、long-lived
  session manager、stdio process session、JSON-RPC codec、operation contract、result normalization 分为同 package
  的 owner-private 文件。Registry 继续是独立 Block；没有新增状态机、registry facade、EventHub topic/round trip、
  receipt owner、跨 capability executor 或第二 session authority。
- adapter 仍只有一个 `sessionMu` 与一个 workspace/server-keyed session map；短 session 的初始化/execute/close、
  长 session 的 create-or-reuse/idle eviction/failure eviction/adapter close 顺序保持不变。HTTP/stdio 继续共用
  typed operation contract、bounded codec、tool-error detection 和 result normalization，不解释 effect/coverage。
  affected：MCP capability transport；SM-1～SM-4 changed semantics：`none`。
- 非测试 Go 从 `16 / 2,698 LOC` 变为 `24 / 2,799 LOC`，测试从 `8 / 2,136 LOC` 变为
  `9 / 2,183 LOC`；package 保持 4，最大生产文件从 `biz/mcp_runtime_adapter.go` 1,389 行降为
  `biz/owner.go` 776 行，至少 500 行生产文件从 2 个降为 1 个，生产 `CC>30` 继续为 0。exported declaration、
  6 个 EventHub topic和直接组件依赖均未改变。
- `BenchmarkRuntimeAdapterHTTPToolCall` 的 50 次、3 轮样本为
  `317,444～349,474 ns/op`、`12,375～12,694 B/op`、`112～113 allocs/op`；独立计数器冻结每次
  operation 恰好一次 HTTP transport call，benchmark 不经过 EventHub 或 repository/CAS。既有回归继续覆盖
  HTTP/SSE、短/长 stdio、并发 session reuse、idle/failure eviction、JSON-RPC notification/response、output bound、
  policy、catalog 与 owner diagnostic；已通过 `go test ./internal/modules/blocks/mcp/... -count=1` 和 production gocyclo。

Git 完成记录（2026-08-16）：

- 唯一 `RuntimeAdapter` 已按 capability-state observation、operation ingress、read operations、write operations、
  repository inspection、Git CLI runner、pathspec guard、output codec 与 result normalization 分为同 package 的
  owner-private 文件。没有复用 Shell executor，也没有新增 Block、service facade、EventHub topic/round trip、
  repository、receipt owner、动态 command registry 或 forwarding package。
- `SupportedRuntimeOperationVariants` 继续与 typed descriptor 做穷尽比对；`ExecuteOperation` 仍静态分派 read/write。
  durable `CapabilityStateReceiptStore` 保持 Git owner 内唯一 capability-state receipt authority，其 workspace partition、
  same-key idempotency/conflict、并发 winner 与 restart reload 回归未变。affected：Git capability execution/state
  observation；SM-1～SM-4 changed semantics：`none`。
- 非测试 Go 从 `16 / 2,450 LOC` 变为 `24 / 2,538 LOC`，测试保持 `8 / 1,345 LOC`；package 保持 4，
  最大生产文件从 `biz/git_runtime_adapter.go` 1,026 行降为 `biz/owner.go` 700 行，至少 500 行生产文件从 2 个
  降为 1 个，生产 `CC>30` 继续为 0。exported declaration、7 个 EventHub topic和直接组件依赖均未改变。
- 既有测试继续覆盖 read/write variant exhaustive contract、pathspec escape、init/add/commit、status/diff/log/show/branch、
  timeout/output bound/error category、capability-state sequence，以及 durable receipt 的 restart/idempotency/conflict/
  cross-workspace fence；已通过 `go test ./internal/modules/blocks/git/... -count=1` 和 production gocyclo。

Web/SSH 完成记录（2026-08-16）：

- Web 唯一 `RuntimeAdapter` 已按 operation ingress/admissibility、search、fetch、provider transport、provider config
  cache、URL/SSRF policy、Approval typed query、result mapping、content byte bounder与纯 helper 分文件。Approval
  仍只经 scoped typed getter 查询，search/fetch 每次最多一个 provider request，final URL 重走同一 policy；没有新增
  owner、service/repository 注入、EventHub topic/round trip、retry 或 receipt authority。
- Web 非测试 Go 从 `16 / 1,928 LOC` 变为 `25 / 2,026 LOC`，测试从 `5 / 739 LOC` 变为
  `6 / 765 LOC`；package 保持 6，最大生产文件从 `biz/runtime_adapter.go` 863 行降为 `biz/owner.go` 364 行，
  至少 500 行生产文件从 1 个降为 0，生产 `CC>30` 继续为 0。exported declaration、5 个 EventHub topic和直接
  组件依赖均未改变；既有 HTTP/approval/SSRF/response-bound 回归与新增 content bounder 回归均通过。
- SSH 当前 366 行 adapter 存在明确的 target/session/credential、transport 与 output-bound 边界，已在不增加抽象层的
  前提下拆为四个同 package 职责文件；唯一 ingress 继续映射 timeout/command failure，因此没有制造独立 diagnostic
  facade。Probe 不开网络，execute 仍只启动一次 bounded SSH process，临时 key/askpass 仍由同一 cleanup 释放。
- SSH 非测试 Go 从 `14 / 1,030 LOC` 变为 `17 / 1,054 LOC`，测试保持 `5 / 414 LOC`；package 保持 4，
  最大生产文件从 `biz/ssh_runtime_adapter.go` 366 行降为 `biz/owner.go` 248 行，生产 `CC>30` 继续为 0。
  exported declaration、5 个 EventHub topic和直接组件依赖均未改变。Web/SSH affected：capability transport/
  normalization；SM-1～SM-4 changed semantics：`none`；已分别通过全包测试和 production gocyclo。

禁止为减少重复而建立跨 File/Git/Shell/MCP/Web/SSH 的通用可变 adapter、统一 receipt owner 或 capability Block。

#### C22-06 Governance 与 Eventstream（code/automated closed）

- Governance 只做文件级职责拆分：worker/reconciliation、ticket evaluation、approval evaluation、trace store、
  payload parser；AutoGovernanceTrace 仍是唯一协调状态。
- Eventstream 将 record payload codec 按事件族分文件，但 wire version、field key、append/revision/high-watermark
  只有一个权威。

完成记录（2026-08-16）：

- Governance 仍只有一个 `Service`、一条 `governanceWork` queue、一个 reconciliation lifecycle 和一个
  AutoGovernanceTrace repository；原 `service.go` 已按 lifecycle/command、worker/reconciliation、ticket policy、
  approval policy、typed payload parser、trace store/budget、共享纯 helper 分文件。Decision/Approval resolve 继续使用
  typed command，blocked-run reconciliation 不迁移 Run/Runtime。affected：Governance coordination state；SM-1～SM-4
  changed semantics：`none`。
- Governance 非测试 Go 从 `11 / 2,506 LOC` 变为 `17 / 2,576 LOC`，测试保持 `5 / 2,135 LOC`；
  package 保持 5，最大生产文件从 `biz/service.go` 1,909 行降为 `biz/payload_parser.go` 485 行，至少 500 行
  生产文件从 1 个降为 0，生产 `CC>30` 继续为 0。exported declaration、5 个 EventHub topic和直接组件依赖均未改变。
- Eventstream 的 `pkg/recordpayload` 已按 workspace/run/definition、decision/approval/governance、result/artifact/
  continuation、memory/recall、LLM usage 事件族分纯 codec 文件；field key 仍只在 `payload.go`，wire version 仍只在
  `version.go`。`event_stream_service.go`、唯一 append authority、workspace revision/high-watermark/index 和 append
  notification 顺序未修改。affected：audit codec；SM-1～SM-4 changed semantics：`none`。
- Eventstream 非测试 Go 从 `11 / 1,855 LOC` 变为 `16 / 1,884 LOC`，测试保持 `5 / 1,002 LOC`；
  package 保持 6，最大 codec 文件从 842 行降为 211 行，组件最大文件为未修改的 append owner 796 行，至少
  500 行生产文件从 2 个降为 1 个，生产 `CC>30` 继续为 0。exported declaration、4 个 EventHub topic和直接组件
  依赖均未改变；Governance/Eventstream 全包测试与 production gocyclo 均通过。

#### C22-07 Decision/Approval/Memory routers（code/automated closed）

- Decision：ticket query、resolution lifecycle、Runtime governance proposal adapter；
- Approval：query 与 approve/reject/delete；
- Memory：item CRUD/search 与 recall trace；
- 三者都保留现有具体 EventHub contract、scope fence、private reducer 和 repository group。

完成记录（2026-08-16）：

- Decision 保持 `handleCommand -> executeCommand` 唯一普通 command ingress，并由静态穷尽分类器委派到 ticket
  query、ticket mutation 和 resolution handler；Runtime governance request/proposal adapter、scope fence、notification
  分文件内聚。resolution/repository 两阶段写、proposal pending/committed/rejected private reducer、reconciliation
  重放和 Orchestrator typed commit/disposition 顺序未变。非测试 Go 从 `17 / 2,102 LOC` 变为
  `23 / 2,236 LOC`，测试从 `4 / 1,904 LOC` 变为 `5 / 1,945 LOC`；package 保持 6，最大生产文件从
  `biz/owner.go` 651 行降为 `biz/decision_service.go` 579 行，至少 500 行生产文件从 2 个降为 1 个，生产
  `CC>30` 从 `1 / 68` 降为 0。
- Approval 保持同一唯一 ingress，并静态穷尽地委派 query、approved/rejected resolution 和 delete；scope admission、
  private lifecycle reducer、repository update、evidence、resolved notification 与返回 scope 校验的先后顺序未变。
  非测试 Go 从 `13 / 985 LOC` 变为 `18 / 1,082 LOC`，测试从 `3 / 1,165 LOC` 变为
  `4 / 1,205 LOC`；package 保持 6，最大生产文件从 `biz/owner.go` 348 行降为
  `biz/approval_service.go` 164 行，生产 `CC>30` 从 `1 / 53` 降为 0。
- Memory 保持同一唯一 ingress，并静态穷尽地委派 item CRUD/search 与 recall-trace upsert/query/delete；workspace
  context、repository group、item mutation 后通知和 recall upsert 后通知顺序未变。非测试 Go 从
  `11 / 1,200 LOC` 变为 `15 / 1,294 LOC`，测试从 `4 / 588 LOC` 变为 `5 / 631 LOC`；package
  保持 5，最大生产文件仍为未修改的 `pkg/events/messages.go` 361 行，生产 `CC>30` 从 `1 / 46` 降为 0。
- 三个 router 都新增 Topic 唯一归族、无遗漏和未知 Topic fail-closed 回归。exported EventHub contract、Topic、
  组件 fan-out/fan-in 和 repository 类型均未变化；affected：Decision/Approval/Memory resource owners；SM-1～SM-4
  changed semantics：`none`。三个 Block 全包、Governance、Orchestrator 和 Workorchd 消费方测试以及 production
  gocyclo 均通过。

### P2：稳定单元只做清理（code/automated closed）

`workorchruntime`、`system`、`mcpregistry`、`result`、`skill`、`soul`、`ssh` 的当前结构没有新增运行单元的
收益。仅允许删除重复 clone、空 package、无校验 forwarding wrapper，或在同一 package 内按一句话职责分文件。

完成记录（2026-08-16）：

| 单元 | 收口结果 | 关闭依据 |
|---|---|---|
| `workorchruntime` | 无代码改动 | memory/file backend builder 与 12 组 Owner repository mapping 均被 typed provider 使用；2 个生产文件、329 LOC、最大 241 行、`CC>30=0`，没有空 package 或跨 Owner facade。 |
| `kernel/system` | 删除冗余转发 | 删除无校验 `New(eventHub, background)`，仅保留直接校验并构造的 `NewWithDependencies`；生产 LOC `212 -> 208`，唯一 Module、启动顺序和三个 typed ensure round trip 不变。 |
| `blocks/mcpregistry` | 无代码改动 | server DTO、capability canonical model、slice/map/payload clone 与 defaults helper 分别承担 EventHub、repository 和默认值隔离；11 个生产文件、1,118 LOC、最大 382 行、`CC>30=0`，删除会泄漏共享可变状态。 |
| `blocks/result` | 删除重复 clone | terminal material 仍统一经 `CloneResult` 深拷贝，只删除进入该边界前对 `EvidenceRefs`/`ArtifactRefs` 的重复复制；新增 detachment 回归与 benchmark，1000 次基准从 `2,176 B / 7 allocs` 降为 `1,664 B / 5 allocs`，owner lifecycle、delivery fence、repository call 与通知顺序不变。 |
| `blocks/skill` | 无代码改动 | command-to-model clone 与 EventHub result clone 位于不同信任边界，均不可删除；9 个生产文件、447 LOC、最大 129 行、`CC>30=0`。文档同步为实际的 create/get/list 合同，不虚构 update/delete surface。 |
| `blocks/soul` | 无代码改动 | command detach、owner model clone、defaults 与 capability selection validation 均有独立职责；10 个生产文件、656 LOC、最大 186 行、`CC>30=0`。 |
| `blocks/ssh` | 无额外改动 | C22-05 已完成同 package adapter 分文件与实际 process benchmark；P2 复核未发现重复 clone、空包或无校验 forwarding，维持 17 个生产文件、1,054 LOC、最大 248 行、`CC>30=0`。 |

这些单元下的 3 行 `pkg/common` 目录并非空 package：它们持有 contract/wiring 共用的稳定 Module ID，因此保留。
七个单元全包测试与 production gocyclo 均通过。affected：System startup coordination、Result owner-local material
copy；SM-1～SM-4 changed semantics：`none`。

## 6. 是否需要新增、合并或降级组件

当前结论均为 **否**：

- 不把 SM-2、SM-3、SM-4 拆成多个 Module；它们共享一个 Orchestrator aggregate/CAS。
- 不把 Workspace、Goal、DoD、Template 拆成四个 Block；它们共同形成 definition/revision/activation 聚合。
- 不把 Run 的 opaque Runtime persistence 移给 Orchestrator，也不让 Run 解释 aggregate。
- 不把 File、Git、Shell、MCP、Web、SSH 合并成通用执行 Block；安全、凭据、effect 和 receipt 边界不同。
- 不合并 Skill/Soul、MCP Registry/MCP execution、Result/Eventstream、Workspace/Run。
- 不把 projection、focused rule package、base Biz 或 application use case 计为新的 framework unit。

只有同时满足下列条件时才允许建立 focused subpackage：

1. 一句话职责稳定且被至少两个同 Owner 调用点复用，或能隔离纯算法/协议 codec；
2. 不持有 Hub、repository、BackgroundRoutine、aggregate、CAS、retry 或 lifecycle；
3. 不需要导出 owner-private reducer/实体来解决循环依赖；
4. 删除该 subpackage 不会改变 framework 的 Setup/Run/Teardown 数量。

否则只在原 package 内按职责拆文件。

## 7. 每个收口工作包的验收门禁

### 7.1 Owner 与行为

1. 一次只修改一个 Owner；提交说明标出涉及的 SM/资源状态机，纯结构变化也要明确 `changed semantics: none`。
2. 外部 command/result/topic、HTTP DTO、projection DTO、stable prefix 和 persisted schema 默认不变。
3. 不新增 mutation entry、repository/CAS、EventHub round trip、provider physical call、BackgroundRoutine 或 retry ledger。
4. stale、duplicate、restart、partial failure、budget/guard 和 terminal delivery 顺序继续由原 Owner 测试覆盖。

### 7.2 复杂度

每个工作包记录变更前后：

- 非测试文件/LOC/package 数；
- 最大文件及至少 500 行文件数；
- `gocyclo --tests=false` 的 `CC>30` 数量与最大值；
- exported declaration、EventHub topic、owner mutation entry 和直接跨单元 import 边；
- 是否新增只做 forwarding 的文件/package。

LOC 或文件数增加不自动判失败；如果一句话职责更清楚、公开面和 authority 不增加，可以接受。反之，仅把一个
大 switch 机械改成 map/reflect/callback、只转调 facade 或大量微型文件，不计为复杂度收益。

### 7.3 测试与性能

1. 先运行受影响 Owner 的 focused tests，再运行 `make fmt-check`、`make vet`、`make test`、
   `make test-race`、`make verify-boundary-contract` 和 `make build`。
2. 对 Orchestrator、Workorchd、LLM、File、Shell、MCP 的性能敏感切片，先补或冻结 focused benchmark；至少记录
   owner calls、repository load/CAS、provider calls、projection rebuild 次数及分配变化。
3. 部署后的 HTTP/SSE/CLI regression、fresh-data Runtime convergence 和 load test 单独执行；静态 CC 降低不能
   替代 live closure。

## 8. 盘点后的实施顺序

建议按下列顺序逐 Owner 提交：

```text
Workspace command router
 -> Run command router
 -> Orchestrator pure invariant/transaction hotspots
 -> Workorchd pure/transport hotspots
 -> LLM -> File -> Shell -> MCP -> Git
 -> Governance -> Eventstream
 -> Decision -> Approval -> Memory
 -> 其余稳定单元清理
```

这个顺序先处理状态 Owner 的高分支入口，再处理核心事务和应用维护面，最后处理外部能力与低风险清理；它不是
新的 Runtime 执行顺序。任何一步发现必须改变 Owner、persisted contract 或跨状态机 handoff 时，都应停止该复杂度
工作包，先按 Runtime 设计合同单独立项。

## 9. 本轮最终收口快照（2026-08-16）

本清单定义的 C22-01～C22-07 与 P2 已全部完成 code/automated closure。收口后仍保持
**1 Initiator + 4 Modules + 17 Blocks = 22 个运行单元**，含非测试 Go 的 package 目录仍为 148，没有新增
正式 Owner、aggregate、状态机、mutation entry、综合 runtime facade 或跨 capability 可变 executor。

| 指标 | `c1b90039` 基线 | 收口后 | 变化解释 |
|---|---:|---:|---|
| 非测试 Go | 1,029 文件 / 125,586 LOC | 1,141 文件 / 127,891 LOC | 增加 owner-private 语义文件与静态 contract helper；package 数不变。 |
| 测试 Go | 360 文件 / 112,921 LOC | 378 文件 / 113,917 LOC | 增加路由穷尽、事务/结构、detachment 与 benchmark 回归。 |
| 合计 | 1,389 文件 / 238,507 LOC | 1,519 文件 / 241,808 LOC | 净增 3,301 LOC，用于缩小职责文件和冻结边界/性能。 |
| ≥500 行生产文件 | 50 | 36 | 减少 14 个；最大文件仍是 Orchestrator `semantic_planning.go` 1,642 行。 |
| 生产 `CC>30` | 42，最大 131 | 24，最大 60 | Workspace/Run/Decision/Approval/Memory router 均关闭；目标事务热点关闭。 |

剩余 24 个 `CC>30` 分布为 Orchestrator 20、Workorchd 2、LLM HTTP stream parser 1、Workspace execution
intent 1。它们不属于本轮已定义的 C22 切片：Orchestrator 项涉及新的完整 transaction/invariant 选择，Workorchd
项涉及 activation compiler/HTTP use case，LLM 项涉及流协议 parser，Workspace 项涉及 execution-intent 聚合。
不得为清零数字而改成动态 registry、callback transaction 或拆出第二 Owner；如继续处理，必须分别建立新的
语义工作包、调用/分配基线和相应状态机影响说明。

最终自动化门禁：

- `make fmt-check`、`make vet`、`make verify-boundary-contract`、`make test`、`make test-race`、`make build` 全部通过；
- EventHub Post AST 审计按新的语义文件名更新，Post site 数量零增长，Memory recall 的匿名 Post 改为命名
  `publishMemoryRecallRecorded`；
- boundary verifier 的 scope、File shared feasibility policy、LLM effect policy 与 C4 projection 断言只更新到拆分后的
  实际文件，并未降低检查条件；
- focused benchmark 已记录 Workorchd projection store、LLM、File、Shell、MCP 与 Result material copy；Result
  material copy 从 `2,176 B / 7 allocs` 降至 `1,664 B / 5 allocs`。

部署后 HTTP/SSE/CLI、fresh-data Runtime convergence、SO-MC-10 与 load test 仍按独立 live closure 验收；本轮
没有部署、Resume/Restart 或真实外部 provider 操作，自动化通过不能替代该证据。
