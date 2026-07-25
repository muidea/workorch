# WorkOrch Unified Runtime Design Contract

Status: active

Type: core-contract

Last Updated: 2026-07-25

本文定义 WorkOrch 唯一的、领域中立的 Runtime V2 设计合同。生产主链仅为
`DriveRuntimeV2`、reducer 与 sealed dispatch；它治理事实、约束、证据、恢复与终态，
不理解软件开发、文件名、语言、测试框架或任何业务领域实现。

实现细节的权威顺序如下：

1. 本文的状态机、权威与禁止迁移；
2. [Runtime request/cache contract](runtime-request-cache-note.md)；
3. `internal/modules/kernel/orchestrator/readme.md` 与
   `internal/pkg/workorchruntimecontract`；
4. [RUNTIME-VPC 收口方案](../70-roadmap/active/runtime-v2-validation-progress-closure-plan.md)。

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

## Runtime V2 生产主链

```text
typed RuntimeBootstrap
  -> SM-1 Start/Resume admission
  -> SM-4 load/singleflight drive
  -> SM-2 reducer / authority transaction
  -> acquire prerequisite | request candidate
  -> SM-3 complete candidate / seal / execute / receipt / evidence
  -> SM-2 reduce / focus advance / governance / terminal
  -> SM-1 narrow Run/Workspace projection
  -> SM-4 derived delivery retry
```

1. Application 只构造并持久化 typed `RuntimeBootstrap`，再发起 Start 或 Resume。
2. Run owner 只持久化 Run lifecycle、Runtime V2 aggregate、definition revision 与 lineage。
3. Orchestrator 是 reducer、prerequisite/freshness、candidate/dispatch、evidence、governance
   resolution 与 terminal decision 的唯一权威。
4. capability owner 只执行自己的 typed prerequisite/operation 并返回 receipt；Result owner只保存
   已裁决 outcome；projection/eventstream 仅为派生读模型。
5. 重启只从已持久化 aggregate 继续 drive；不得从进程内缓存、UI 状态或历史文本重建 authority。

## 四个协调状态机

### SM-1：Run lifecycle

```text
pending -> active <-> blocked_on_decision | blocked_on_approval
active -> completed | failed | cancelled | superseded
```

- `StartRunCommand` 仅允许 `pending -> active`；`ResumeRunCommand` 仅携带 workspace/run scope，
  仅进入同一 V2 drive。
- terminal 不可变。继续未完成工作只能由产品层显式创建新的 typed activation / Run，并保存
  predecessor lineage；不允许自动 successor。
- blocked 回 active 必须有匹配的 persisted governance resolution。
- complete/fail 只能来自 SM-2 sealed terminal decision；cancel/supersede 必须先持久化
  `RuntimeTerminationRecord`。
- Run/Workspace terminal projection 是单向派生动作；失败仅可重试交付，不能改写 authority。

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

- `DriveRuntimeV2Command/Result` 是唯一同步推进合同。Application 不保留 continuation queue。
- 每个非等待、非终态 transition 必须持久化递增 `StateRevision`；无进展或超过 guard 必须 typed
  fail-fast，不能在进程内空转。
- singleflight key 必须包含 workspace/run。重复 Start、Resume 和 governance continuation 只合并到
  同一 drive。
- startup 由 Run owner typed inventory 区分 `active_no_dispatch`、`active_sealed_dispatch`、
  `blocked_governance`、`terminal_delivery_pending`；盘点失败必须 fail-fast，不能由 application 猜测。
- reducer terminal 已 sealed 后，只允许 Result/Run/Workspace 的幂等 delivery retry。projection health
  不参与 reducer，也不是 runtime blocker。

禁止：重启重新 initialize 已有 aggregate、watcher/projection/TUI/Web 驱动 reducer、无 revision 进展的循环、
projection failure 覆盖既有 terminal、重放已知 provider/durable effect。

## 跨状态机不变量

1. SM-1 不选择 action；SM-2 不执行 operation；SM-3 不选择下一 action；SM-4 不重建 authority。
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
5. 同步更新 RUNTIME-VPC、当前事实/未关索引与对应 module readme；
6. 通过 `make verify-boundary-contract`，确认没有将状态迁移放入 `pkg`、没有 V1 回流、没有让
   projection/TUI/Web 成为 authority。

活动实施顺序仅由 RUNTIME-VPC 定义。若发现第二份 Runtime V2 状态机、graph/step continuation、
自动 successor 或 live 热修计划，必须先删除或迁入 `docs/90-archive/`，不能与本文并列。
