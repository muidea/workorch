# WorkOrch Unified Runtime Design Contract

Status: active

Type: core-contract

Last Updated: 2026-07-28

本文定义 WorkOrch 唯一的、领域中立的 Runtime V2 设计合同。生产主链仅为
`DriveRuntimeV2`、reducer 与 sealed dispatch；它治理事实、约束、证据、恢复与终态，
不理解软件开发、文件名、语言、测试框架或任何业务领域实现。

实现细节的权威顺序如下：

1. 本文的状态机、权威与禁止迁移；
2. [Runtime request/cache contract](runtime-request-cache-note.md)；
3. `internal/modules/kernel/orchestrator/readme.md` 与
   `internal/pkg/workorchruntimecontract`；
4. [RUNTIME-SMO 统一收口方案](../70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md)
   （candidate/receipt、状态 owner、跨状态机交接与 lifecycle 的唯一活动实施队列）。

已归档计划、历史 live 记录与过程日志只能帮助定位回归夹具，不能成为新的运行时约束。

## 核心中立与职责分层

Runtime core 只理解 workspace、run、definition、focus、requirement、prerequisite、candidate、
sealed dispatch、operation、effect、evidence、assessment、governance、result、recovery 和
projection。它不推断“应创建哪个文件”“何种实现正确”或“哪个工具最适合某一领域”。

| 层 | 职责 | 不得承担 |
| --- | --- | --- |
| Runtime core | facts、reducer、authority、sealed dispatch、evidence、governance wait、terminal | 领域策略、owner 资源猜测、provider 私有逻辑 |
| profile / capability | 交付物类型、可用能力、owner admission、validation/evidence policy | 修改 Runtime authority 或越权关闭 coverage |
| LLM | 在 sealed required output 内提出 candidate | 选择 Runtime action、扩展授权、裁决证据或终态 |
| TUI/Web/projection | activation、审计和展示 | 重建 authority、隐式续跑或补偿状态 |

`internal/pkg/workorchruntimecontract` 仅保存 DTO、合同定义以及无状态的 normalize、validate、
clone、hash helper。它不得拥有状态迁移、CAS、EventHub、filesystem/network I/O 或 retry 决策。

跨 owner I/O 只能通过 EventHub 的具体 typed command/result。禁止跨 owner service/repository、
`map[string]any`、无约束 `any`、JSON/reflect bridge 或通用 envelope。

Workspace scope 是每次事件执行上下文的一部分，而不是组件实例的“当前 workspace”状态。任何带
`WorkspaceID` 的入站 Runtime 命令必须先将该 ID 绑定到 EventHub context；已有 context 只能匹配，
不得被静默覆盖为另一 workspace。Biz、Service、Module、全局变量、共享 cache 和后台任务不得保存
未经该 scope 约束的当前 WorkspaceID。持久化模型、sealed contract 与 typed command 中的 ID 仅作为资源
归属和审计 identity；下游 repository、EventHub 和 background work 必须继承已绑定 context。
旧的 context 构造 helper 只能为未 scoped 的根 context 首次建立 scope，不能覆盖已有 scope；正式命令入口
必须使用会返回 conflict 的 checked binder，不能把该 helper 当成跨 workspace 切换机制。

## Runtime V2 生产主链

```text
typed RuntimeBootstrap
  -> Run owner receives activation / continuation intent
  -> SM-1 owner-local lifecycle transition + RunStateChanged fact
  -> SM-4 load/singleflight drive
  -> SM-2 reducer / authority transaction
  -> acquire prerequisite | request candidate
  -> SM-3 complete candidate / seal / execute / receipt / evidence
  -> SM-2 reduce / focus advance / governance / terminal
  -> SM-4 terminal delivery ledger
  -> Result owner -> Workspace owner -> Run owner owner-local delivery transitions
```

1. Application 只构造 typed `RuntimeBootstrap` 并向 Run owner 提交 scoped activation / continuation intent；
   不指定 Run、Workspace 或 Result 的目标状态。
2. Run owner 只持久化 Run lifecycle、Runtime V2 aggregate、definition revision 与 lineage。
3. Orchestrator 是 reducer、prerequisite/freshness、candidate/dispatch、evidence、governance
   resolution 与 terminal decision 的唯一权威。
4. capability owner 只执行自己的 typed prerequisite/operation 并返回 receipt；Result、Workspace、Run
   owner 各自消费 terminal delivery fact 并只迁移自己的资源状态；projection/eventstream 仅为派生读模型。
5. 重启只从已持久化 aggregate 继续 drive；不得从进程内缓存、UI 状态或历史文本重建 authority。

### 跨 owner 生命周期规则

任何正式状态只由唯一 owner 的 Biz 内部 reducer 修改并持久化。跨 owner 消息只能携带 immutable intent、
已持久化 fact 或 delivery acknowledgement；不得携带“把接收方状态设为 X”的命令。接收 owner 必须先校验
workspace/run scope、causation、definition/evidence revision 与 sealed identity，再转为 owner-local event。

- SM-1 Run lifecycle 只由 `blocks/run` 迁移；Workspace active-run/state projection 只由
  `blocks/workspace` 迁移；Result lifecycle 只由 `blocks/result` 迁移。
- Orchestrator 只拥有 SM-2、SM-3、SM-4。SM-4 只记录 delivery request/acknowledgement 与重试，
  不直接写 Run、Workspace 或 Result。
- terminal delivery 的稳定顺序是 `Result -> Workspace -> Run`。任一步失败仅为 delivery pending；
  不得改写已 sealed 的 SM-2 terminal，亦不得以投影失败把 Run 反向置为 failed。

## 规范化候选合同

Runtime V2 不要求穷举所有模型输出组合。它要求把动态 owner 事实先编译成一份有限、可验证的
`RuntimeCandidateContractManifest`，此后 renderer、decoder、candidate evaluator、owner feasibility 和
fixture 都只能消费这份封存合同，不能再次解释原始 profile、目录、错误文本或临时上下文。

每个 manifest 由零到多个 `SealedOperationAuthorization` 组成。该名称表示 manifest 内的一条纯值授权记录，
不是新的状态机或 owner；实现可在现有 manifest DTO 内表达。每条授权至少包含：

- canonical operation kind、owner module、parameter/variant contract；
- accepted/required effect scope、coverage 与 dependency 约束；
- 唯一 `TargetPolicy` 以及该策略所需的 exact refs、bounded owner scope 或 owner selector；
- 来源 prerequisite/owner receipt 的 snapshot/revision 引用；
- manifest hash 与 operation-contract revision。

目标策略只允许三种，禁止为 live 症状增加第四种临时分支：

| TargetPolicy | 含义 | 必须拒绝 |
| --- | --- | --- |
| `exact_target` | operation 的全部 target 必须属于封存的精确 target set | 空 target、越界 target、运行时猜测 target |
| `existing_owner_scope` | operation 必须提供 concrete target，且每个 target 属于 owner 封存的既有资源 scope | 新建 target、截断/不完整 inventory、只命中部分 target |
| `prospective_owner_target` | candidate 必须提交 concrete target，并命中 owner 封存的 create selector/prospective constraint | 非 owner 允许的新建、无 target、绕过 owner feasibility |

`TargetPolicy` 是 owner-neutral 的纯合同词汇。File、Git、Shell、MCP 或业务 profile 的具体语义仍由对应
owner/profile 提供；Orchestrator 只编译和验证 typed receipt，不按文件名、命令或领域文本推断策略。

owner inventory 与 provider context 是两个不同合同：

- owner inventory 必须是完整的有界授权事实，或是能被 owner 独立验证的封闭 selector；截断列表不得封存为
  scope；
- candidate context 可以从 manifest 生成有损展示，但不得删除执行所需的 kind、parameter、target policy、
  coverage 或 effect 约束；
- `OwnerInventoryBudget`、`CandidateContextBudget`、`RequestRecoveryBudget` 必须使用不同 DTO/字段和校验入口，
  禁止因数值相同而互用；
- inventory 超限且 owner 无法提供封闭 selector 时，在 provider 前形成 typed prerequisite failure。提高固定
  常量不能替代该产品语义。

### 单一 candidate evaluator

完整 candidate document 只允许进入一个纯 `CandidateEvaluator`。它的输入是 persisted manifest、decoded
candidate、同一 authority contract 的 prior attempt fact 和 typed owner feasibility observations；输出只能是 `accepted` 或一个完整的
`CandidateRejection`。decoder、admission 与 owner-feasibility adapter 可以提供 typed observation，但不能各自
选择 retry、terminal、governance 或下一 action。

为取得 feasibility observation，Orchestrator 只可先用 operation kind 解析 manifest 中的 owner 和
authorization identity；该步骤不做 candidate admission。无法解析的 kind 不调 owner，由 evaluator 在
kind 层拒绝；可解析的 operation 可调用无副作用 owner probe，owner 只返回
`feasible | stale | infeasible` observation。即使 owner 已返回 infeasible，evaluator 仍按固定顺序优先返回更早的
wire/parameter/target 违例，保证同一 document 只有一个 first-failure。

`CandidateRejection` 至少包含：class/code、operation ID（若适用）、field path、actual、allowed/required/
forbidden、manifest hash、rejected shape hash，以及显式 `CorrectionDisposition`：

| CorrectionDisposition | 语义 | SM-3 唯一行为 |
| --- | --- | --- |
| `correctable_once` | 同一 manifest 内存在唯一、有界的结构修正 | 生成一个 `CorrectionDirective`，只允许 attempt 1 |
| `rejected_final` | 没有安全的同合同修正空间 | 直接形成 typed candidate terminal，零额外 provider call |

`CorrectionDirective` 只描述 candidate delta，例如删除 forbidden field、补 required field、选择 allowed kind、
替换为 allowed target、改变 rejected semantic shape；它不选择 Runtime action、不修改 authority，也不包含 raw
provider/decoder/owner 错误文本。

若 complete rejection 无法归入上述两类，属于 `runtime_contract_defect`，必须 fail-fast 且零额外 provider、
ticket、dispatch 或 effect。`candidate_correction_undefined` 不得继续作为正常模型输出的兜底路径。

### 五类统一处理结果

provider dependency/governance 在进入 candidate pipeline 前单独处理。进入候选处理后，所有结果只允许归入：

| 结果 | 来源 | 状态机行为 |
| --- | --- | --- |
| `request_recoverable` | fragment、EOF、timeout、429/5xx、未完成 document | request-session 有界恢复；不创建 attempt |
| `candidate_correctable` | complete rejection + 完整 `CorrectionDirective` | 同 transaction/stable prefix 的唯一 attempt 1 |
| `candidate_rejected_final` | complete rejection + `rejected_final` | typed terminal；不请求 correction |
| `prerequisite_stale` | owner revision/receipt 与封存授权不一致 | SM-2 精确退休并重取；旧 document/attempt 不复活 |
| `accepted` | evaluator 与 owner feasibility 均接受 | persist sealed dispatch 后才可执行 |

以上分类是候选处理的唯一分流表。错误字符串、TUI/Web、projection、repair hint、owner adapter 或测试 fixture
不得定义额外分流。

## 四个协调状态机

### SM-1：Run lifecycle

```text
pending -> active <-> blocked_on_decision | blocked_on_approval
active -> completed | failed | cancelled | superseded
```

- activation / resume intent 仅携带 workspace/run scope 与必要 activation identity；Run owner 在自己的
  reducer 中决定 `pending -> active` 或匹配 blocker 的 `blocked -> active`，成功后才发布 Run state fact。
- terminal 不可变。继续未完成工作只能由产品层显式创建新的 typed activation / Run，并保存
  predecessor lineage；不允许自动 successor。
- blocked 回 active 必须有匹配的 persisted governance resolution。
- complete/fail 只能来自 SM-2 sealed terminal decision；cancel/supersede 必须先持久化
  `RuntimeTerminationRecord`。
- Run/Workspace/Result terminal delivery 是单向 owner-local saga；失败仅可重试对应 delivery，不能改写 authority。

禁止：SM-1 选择 action、HTTP/TUI/Web 直接写 terminal、Resume 注入 definition/provider/model/repair
patch、terminal 回到 active、projection 反向驱动 lifecycle。

### SM-2：Authority and focus convergence

SM-2 是 `RuntimeFacts -> RuntimeDecision + AuthoritySnapshot + AuthorityTransaction` 的唯一权威。

```text
load facts
  -> prerequisite missing: acquire + typed commit + reduce
  -> focus requires state change / validation / assessment / governance
  -> evidence and coverage gate
  -> focus advance | repairable gap | governance wait | sealed terminal
```

- prerequisite 未完成前不得请求 candidate。manifest、selection、snapshot、failure target、capability、
  validation scratch、provider ceiling 等必须通过 typed acquisition 获得并持久化。
- `RequiredOutputContract` 必须是 current focus、owner-confirmed capability、evidence policy 和
  side-effect compatibility 的中立交集。可行动性不足必须在 provider 前确定失败，而不是让 LLM 猜测。
- accepted owner receipt 的资源变更与 read-only stale 分别使用封存、独立且有界的 refresh ledger；
  只退休 dependency 精确相交的 observation。duplicate 不得重复 CAS、查询或扣减预算。
- evidence 必须同时通过 current-run、focus、coverage、quality、effect、receipt 和适用的 claim/
  executable-observation 门禁。历史 summary、memory、projection 和前一 focus evidence 不能关闭当前
  coverage。
- repairable gap 保持同一 focus，必须持久化结构化缺口。有效新 evidence 不清零同 focus 总 repair
  budget；只有原子 focus advance 才开启新的 repair ledger。
- governance 只用于真实权限、外部依赖、不可行 capability、预算升级或配置阻塞。可继续的候选/证据
  合同违规不能创建无信息 ticket。

禁止：SM-2 调 provider、执行 operation、直读其他 owner repository、将 profile/软件领域语义写进 reducer、
用数量而非质量关闭 evidence gate。

### SM-3：Candidate and sealed-dispatch transaction

```text
sealed authority + preflight ready
  -> persisted request session
  -> complete document -> candidate attempt 0 -> optional attempt 1
  -> accepted candidate -> persist sealed dispatch
  -> claimed operation -> owner receipt -> evidence commit -> close dispatch
```

- request session 必须先绑定 transaction、definition/evidence revision、required-output hash、
  stable-prefix hash 和 logical request identity。fragment、EOF、timeout、429/5xx、401/403 与未知提交
  走 session/provider recovery，不消耗 candidate 或 repair budget。
- 只有完整 document 才可创建 candidate attempt。attempt 0/1 必须使用同一 transaction 与 stable prefix；
  第二次完整 schema/contract rejection 形成 typed `candidate_retry_exhausted`，不存在 attempt 2。
- renderer、wire DTO、strict decoder、admission、owner feasibility 与 fixture 对同一
  `CandidateContractManifest` 给出一致解释；未知字段、参数、target、effect、coverage、dependency 或
  operation kind 不得由兼容 fallback 静默放宽。
- accepted candidate 必须在执行前持久化 `SealedDispatch`。operation 依 sealed dependency order 执行，
  每项使用稳定的 `ExecutionKey + Fence`；owner receipt 是 evidence 的唯一副作用回执。
- duplicate 结果幂等，stale 只进 diagnostic lineage。已存在 exact receipt 时重启只提交后续 evidence，
  不得重放副作用；未知、非安全重放的副作用进入 typed outcome-unknown/failure 路由。
- effect/evidence 类型由 sealed `RequiredOutput`、sealed expected effect 与 owner observed effect
  联合裁决。`none`、`ephemeral_session`、`unknown` 或 zero-work 不能满足 durable state change。

禁止：未持久化 request session 请求 provider、未完整 document 消耗 attempt、accepted candidate 不 seal
直接执行、executor/owner 选择下一 action、owner 直接终态化 Run/Result、依错误文本猜 target 或创建 ticket。

### SM-4：Drive, recovery and projection delivery

- `DriveRuntimeV2Command/Result` 是 Orchestrator 唯一同步推进合同。Application 不保留 continuation queue；
  Run owner state notification 才能投递 Orchestrator 的内部 drive event。
- 每个非等待、非终态 transition 必须持久化递增 `StateRevision`；无进展或超过 guard 必须 typed
  fail-fast，不能在进程内空转。
- singleflight key 必须包含 workspace/run。重复 Start、Resume 和 governance continuation 只合并到
  同一 drive。
- startup 由 Run owner typed inventory 区分 `active_no_dispatch`、`active_sealed_dispatch`、
  `blocked_governance`、`terminal_delivery_pending`；盘点失败必须 fail-fast，不能由 application 猜测。
- reducer terminal 已 sealed 后，SM-4 仅重试已持久化 terminal delivery request，等待 Result、Workspace、Run
  owner acknowledgement。projection health 不参与 reducer，也不是 runtime blocker。

禁止：重启重新 initialize 已有 aggregate、watcher/projection/TUI/Web 驱动 reducer、无 revision 进展的循环、
projection failure 覆盖既有 terminal、重放已知 provider/durable effect。

## 跨状态机不变量

1. SM-1 不选择 action；SM-2 不执行 operation；SM-3 不选择下一 action；SM-4 不重建 authority，亦不修改别的 owner state。
2. 每次跨状态机交互都携带 workspace/run scope 以及适用的 definition/evidence revision、authority
   snapshot、dispatch/operation identity。
3. 正式事实必须先持久化，后产生外部可见 projection；projection 可最终一致，authority 不可反推。
4. candidate、dispatch、governance resolution、evidence commit、terminal delivery 都必须幂等。
5. provider target、definition 与 activation 在 Run 内封存；变化需要新的 typed activation，不能由 Resume
   patch。
6. contract/invariant violation 必须形成 typed diagnostic/failure 并沿以上方向收敛；不得旁路某个状态机
   修补正式状态。

## 中立 effect、evidence 与治理

| Effect scope | 可关闭 durable state change |
| --- | --- |
| `none`、`ephemeral_session`、`governance_only`、`unknown` | 否 |
| `workspace_durable`、`remote_durable` | 仅在 RequiredOutput 接受、receipt 证明且 coverage 对齐时 |

`produce_state_change` 的 sealed candidate 至少含一个与 RequiredOutput 兼容的 durable operation；
`produce_validation` 只能含 `none` 或显式允许的 `ephemeral_session` operation。assessment 只消费当前
Run 的 accepted evidence；assessment-only focus 不执行 executable operation。

operation-scoped governance 必须保存 ticket、resolution、dependency、subject、receipt hash、ExecutionKey
与 fence binding。continue 以同一 ExecutionKey 的下一 fence 执行，owner 必须 preflight grant；普通 receipt
不能绕过治理。

## LLM request、cache 与上下文

LLM 是不可靠的 protocol client，不是 authority。请求必须由 sealed typed context 渲染：

- C0：stable runtime/schema；C1：Goal/DoD；C2：declarative capability/governance contract；
  C3：current focus/required output；C4：failure/evidence/diagnostic；C5：有界 ref。
- P0–P4 的必要字段缺失时不得提交 provider。raw prompt、raw provider body、credential、endpoint、
  snapshot content 不得进入 aggregate、audit、projection 或 memory。
- unexpected stable-prefix drift 是 renderer defect。完整 request/cache 规则以
  [runtime-request-cache-note.md](runtime-request-cache-note.md) 为准。

## V1 retired surface

以下路径已从生产和活动实施约束中删除：`StagePacket`、`NextAction`、`DispatchNextStep`、
`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher、application Start/Resume queue、
自动 replan successor、V1 checkpoint replay，以及以 `runtimeContractVersion` 作为路由条件。

旧名称只能出现在 archive 的历史说明或明确的 retirement test 中；不得用于新的 DTO、EventHub 合同、
状态迁移、配置或 prompt 结构。

## 变更门禁与文档纪律

所有 Runtime V2 变更必须：

1. 标明影响的 SM-1～SM-4、owner、persisted fact、合法/禁止迁移；
2. 为每个 owner-defined typed command/result 添加 producer/consumer 回归；
3. 覆盖 normal、duplicate、stale、CAS 前中断、CAS 后响应丢失和 fresh-process reload；不可达语义必须
   由测试证明；
4. 覆盖预算/guard 与零副作用断言：在 prerequisite/authority 失败时不得产生 provider request、attempt、
   dispatch、ticket 或 effect；
5. 同步更新 RUNTIME-SMO、当前事实/未关索引与对应 module readme；
6. 通过 `make verify-boundary-contract`，确认没有将状态迁移放入 `pkg`、没有 V1 回流、没有让
   projection/TUI/Web 成为 authority。

活动实施顺序仅由 RUNTIME-SMO 定义；旧 candidate/receipt 计划只作为 superseded 回归基线。若发现第二份
Runtime V2 状态迁移、graph/step continuation、自动 successor 或 live 热修计划，
必须先删除或迁入 `docs/90-archive/`，不能与本文并列。
