# Runtime V2 单 Owner 状态机收口方案

Status: active

Type: implementation closure plan

Last Updated: 2026-07-27

## 1. 目的、范围与权威关系

本方案将 Runtime V2 收紧为**单 owner、owner 内部事件迁移、跨组件只传不可变事实**，代号
`RUNTIME-SMO`。它是当前 Runtime lifecycle、projection 与跨 owner 状态边界的唯一活动实施队列。

它不替代以下权威：

1. [runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)：四个状态机及中立语义；
2. `internal/modules/kernel/orchestrator/readme.md`：Runtime V2 实现 SoT；
3. [runtime-v2-validation-progress-closure-plan.md](runtime-v2-validation-progress-closure-plan.md)：
   candidate manifest、evaluator、receipt 与 live observation 的收口记录。

`RUNTIME-VPC` 仅继续约束 candidate/receipt；状态机 owner、lifecycle projection 和跨 owner
状态方向由本方案唯一约束。旧文档中“Orchestrator 可以直接投影、更新或同步
Run/Workspace/Result 状态”的表述，在本方案 S0 完成后必须删除或迁入 archive；不保留兼容双路径。

### 1.1 2026-07-27 实现复核基线

本计划曾根据 topic/facade 删除和本地门禁将 S1～S6 标为已完成。2026-07-27 的实现级复核确认：
Run、Workspace、Result owner reducer 与部分 delivery acknowledgement 已落地，但仍存在可从生产入口
到达的第二状态写入路径、未封存的事实身份和非 CAS 持久化。因此，**不得以已有 reducer、全仓测试或
`verify-boundary-contract` 通过认定 RUNTIME-SMO 已关闭**。

本计划第 12 节是本次复核后的唯一剩余实现队列；它取代此前 S1～S6 的完成声明。除非该节每张卡的
退出条件和第 7 节的回归项均有证据，否则状态保持 `active`。

## 2. 不可变规则

### 2.1 单 owner

每个可变状态机只允许一个组件 owner。只有该 owner 的 Biz 内部事件 reducer 可以改变状态并持久化。

| 状态机 / 状态域 | 唯一 owner | 可改变的状态 |
| --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | Run lifecycle、blocker ref、Run lifecycle metadata |
| Workspace execution projection | `blocks/workspace` | `Workspace.State`、`ActiveRunID`、workspace projection metadata |
| SM-2 authority/focus | `kernel/orchestrator` | facts、focus、prerequisite、repair、governance、terminal decision |
| SM-3 candidate/dispatch | `kernel/orchestrator` | request session、attempt、dispatch、execution/evidence lineage |
| SM-4 drive/recovery/delivery | `kernel/orchestrator` | singleflight、recovery ledger、delivery acknowledgement；不写别的 owner 状态 |
| Decision lifecycle | `blocks/decision` | ticket/resolution state |
| Approval lifecycle | `blocks/approval` | approval state |
| Result lifecycle | `blocks/result` | Result recorded/accepted/rejected state |

Workspace、Decision、Approval、Result 是各自资源状态机，不是 SM-1～SM-4 的第二个实现；它们只依据
已持久化的跨 owner 事实收敛自身状态。

### 2.2 跨 owner 事件方向

跨组件消息仅可为：

1. **intent / fact input**：表达已发生或已持久化的事实，不携带接收 owner 的目标状态；
2. **owner notification**：owner 成功迁移后发布的不可变事实。

接收方统一遵循：

```text
typed event -> validate scope / causality -> enqueue owner-local event
  -> owner reducer -> CAS / persist -> owner notification
```

跨组件 handler 不得直接赋值、直接调用另一个 owner 的 transition facade，或把 `State`、`ActiveRunID`、
`ResultState` 作为调用方指定值。同步 `Send` 只用于受理、读取或 receipt；状态变化对外传播使用 owner
notification。重复 input 必须由 owner reducer 幂等处理。

### 2.3 明确禁止

- Orchestrator 持有或使用 `RunRepository`、`WorkspaceRepository`、Run/Workspace mutable model 写生命周期；
- Orchestrator 的 `AcceptRunStart`、`ActivateRuntimeV2Run`、`BlockOnDecision`、`BlockOnApproval`、
  `ProjectRuntimeV2Terminal`、`runStateSubsystem` 或等价 facade；
- Application、TUI、Web、Governance、Decision、Approval、Result、Eventstream 直接令 Run 进入指定状态；
- Orchestrator 直接指定 `Result.State` 或 Workspace target state；
- projection watcher、重启扫描、TUI/Web 反向重建 SM-2/SM-3 authority；
- 旧 Start/Resume/Block/terminal topic、alias、handler 与新事实合同双写或兼容共存。

## 3. 目标拓扑

```text
Application intent
  -> Run owner internal lifecycle event
  -> RunStateChanged(active)
  -> Orchestrator SM-4 internal drive event
  -> SM-2 / SM-3 owner-local transitions

Orchestrator persisted governance wait / continuation eligibility
  -> immutable runtime lifecycle fact
  -> Run owner internal lifecycle event
  -> RunStateChanged(blocked | active)

Orchestrator persisted terminal decision
  -> SM-4 terminal-delivery fact sequence
  -> Result owner internal record event -> ResultRecorded
  -> Workspace owner internal release event -> WorkspaceRunReleased
  -> Run owner internal terminal event -> RunStateChanged(terminal)
  -> Orchestrator delivery acknowledgement reducer
```

终态交付不再是跨 owner “同步写 Run + Workspace”的伪原子操作。SM-4 使用持久化 delivery
acknowledgement 在 `Result -> Workspace -> Run` 顺序收敛；每步仅由对应 owner 修改自身状态。单边失败只为
delivery pending，由 SM-4 幂等重试，绝不改写已 sealed 的 SM-2 terminal。

## 4. 当前违例与替换合同

| 当前入口 / 直接写入 | 违例 | 替换合同 |
| --- | --- | --- |
| `RuntimeOrchestrator.AcceptRunStart` | Orchestrator 写 Run/Workspace | Run owner 消费 activation intent，内部 reducer 激活并通知。 |
| `ActivateRuntimeV2Run` / `CanResumeRun` | Orchestrator 清 blocker、写 active | Orchestrator 持久化 governance evidence 后发布 `RuntimeContinuationEligible`；Run owner校验绑定后内部恢复。 |
| `BlockOnDecision` / `BlockOnApproval` | Orchestrator 令 Run blocked | Orchestrator 发布 `RuntimeGovernanceWaitRecorded`；Run owner内部进入 blocker 状态。 |
| `ProjectRuntimeV2Terminal` / `runStateSubsystem` | Orchestrator 同时更新 Run 与 Workspace | SM-4 依 terminal delivery facts 请求 Result、Workspace、Run owner 各自内部迁移。 |
| `projectRuntimeV2Result` 的预设 Result state | Orchestrator 指定 Result lifecycle | Result owner 消费 terminal fact，自行记录 accepted/rejected Result。 |
| Application Start/Resume → Orchestrator command | Application 间接管理 SM-1 | Application 仅提交 scoped activation/continue intent 给 Run owner；Run notification 才触发 drive。 |

## 5. 组件影响范围

### P0：必须修改

| 组件 | SM / 状态域 | 修改内容 |
| --- | --- | --- |
| `kernel/orchestrator` | SM-2/3/4 | 删除 Run/Workspace lifecycle facade、repository subsystem、直接投影；发布事实、记录 delivery acknowledgement、只调度自身 drive。 |
| `blocks/run` | SM-1 | 增加 owner-local lifecycle reducer；消费 activation/governance/terminal facts；唯一发布 Run state notification。 |
| `blocks/workspace` | workspace projection | 消费 Run/terminal facts；内部维护 `ActiveRunID`/state；不接受 Orchestrator model update。 |
| `blocks/result` | result lifecycle | 消费 terminal delivery fact；内部构造 Result 并发布 receipt/notification。 |
| `application/workorchd` | inbound adapter | Start/Resume 改为 intent；删除直达 Orchestrator lifecycle command 的 production path。 |

### P1：合同与事实 owner 同步

| 组件 | 修改内容 |
| --- | --- |
| `blocks/decision` | resolution committed 只提交治理事实，不要求 Run active。 |
| `blocks/approval` | approval committed 只提交治理事实，不要求 Run active。 |
| `kernel/governance` | 只产生自动 resolution/approval 与 trace，删除所有 lifecycle 假设和 fixture。 |
| `kernel/orchestrator/pkg/events` | 只保存 Orchestrator SM-2/3/4 facts；删除 Run lifecycle command alias。 |
| `blocks/run/pkg/events`、`blocks/workspace/pkg/events`、`blocks/result/pkg/events` | 定义各 owner 的 intent/fact/acknowledgement 合同，topic owner/destination 单一。 |

### P2：仅观察或回归同步

`eventstream`、WorkOrchd read model/SSE、TUI/Web、`system`、`soul`、`skill`、`memory`、`mcpregistry`、
`llm`、`file`、`git`、`shell`、`mcp`、`ssh`、`web`。

它们只能消费 owner notification 或返回 candidate、prerequisite、feasibility、receipt、配置；不得取得
Run/Workspace/Result 或四个 Runtime 状态机的迁移权限。

## 6. 串行收口卡

同一时刻只允许一张卡 `in_progress`。不得先增加新 topic 再保留旧 facade，也不得以 Web/TUI 兼容分支绕过顺序。

| 顺序 | 卡片 | owner / SM | 当前状态 | 退出条件 |
| --- | --- | --- | --- | --- |
| S0 | **规则和合同定稿** | docs、事件 owner | 已完成，随第 12 节增量维护 | 更新 core/readme/matrix；定义 intent/fact/ack DTO；逐字段标明 owner；旧 topic 删除清单明确。 |
| S1 | **SM-1 owner-local reducer** | Run / SM-1 | 重新打开；见 R1、R3、R4 | Run 内部事件覆盖 activate、wait、continue、terminal、cancel/supersede；外部 input 无 target state；CAS/duplicate/stale/terminal immutability 通过。 |
| S2 | **Workspace projection owner 化** | Workspace | 重新打开；见 R1、R2、R4 | Workspace 只从 facts 更新 active run/state；Orchestrator 不再直接更新；live run/stale slot/repeated release 通过。 |
| S3 | **治理事实闭环** | Orchestrator、Decision、Approval、Run | 重新打开；见 R3 | resolution/approval 先 owner-persist，再由 Orchestrator提交 SM-2 evidence，最后事实通知 Run；零 direct Resume/active facade。 |
| S4 | **终态 delivery saga** | SM-4、Result、Workspace、Run | 重新打开；见 R4、R5 | terminal -> Result -> Workspace -> Run acknowledgement 有界、可重启、exactly-once；单边失败仅 delivery pending。 |
| S5 | **入口与恢复归一** | WorkOrchd、Run、SM-4 | 重新打开；见 R5、R7 | Start/Resume 只提交 scoped intent；Run notification 触发 drive；inventory 只触发 Orchestrator internal drive；重复 intent 合并。 |
| S6 | **删除旧路径与门禁** | all touched | 重新打开；见 R1、R6、R8 | 删除 aliases/facades/direct repository subsystem/旧 helper；静态 gate 阻止回流。 |
| S7 | **回归与 live** | all touched | 未开始 | owner-local、跨 owner fact、restart matrix、fresh deployed activation 通过。 |

## 7. 强制验证

### S1 Run owner

- duplicate activation intent 只产生一次 transition/notification；
- 缺少 matching governance fact 的 continue 被拒绝；
- terminal 后任何 activation/continue/governance fact 均不能回 active；
- stale terminal、错误 workspace/run、错误 authority snapshot 均不改变 Run；
- `RunStateChangedData` 仅在 owner CAS 成功后发布。

### S2 Workspace owner

- Run active/blocked/terminal facts 只影响对应 workspace；
- 旧 terminal fact 不抢占或释放其他 live Run；
- release duplicate 与 fresh-process restart 幂等；
- Orchestrator production code 无 Workspace repository/model mutation path。

### S3 Governance

- Decision/Approval binding 不匹配时零 Run transition；
- duplicate committed fact 最多一次 SM-2 evidence commit 与一次 Run continue notification；
- Governance 自动处理零 `ResumeRun` / Run lifecycle command。

### S4 Terminal

- sealed terminal 前零 Result/Workspace/Run delivery；
- 任一 ack 丢失的 fresh-process retry 不重写已完成 owner state；
- delivery fence、terminal code/summary/authority identity 全量匹配；
- SM-4 delivery failure 不改变 SM-2 terminal，Run 不自行推导新 terminal。

### S5/S6 repository gates

```bash
rg -n --glob '*.go' --glob '!**/*_test.go' \
  'RunRepository|WorkspaceRepository|runStateSubsystem|AcceptRunStart|ActivateRuntimeV2Run|BlockOnDecision|BlockOnApproval|ProjectRuntimeV2Terminal' \
  internal/modules/kernel/orchestrator

rg -n --glob '*.go' --glob '!**/*_test.go' \
  'TopicOrchestratorCommand(StartRun|ResumeRun|BlockDecision|BlockApproval)|ResumeRunCommand|BlockOnDecisionCommand|BlockOnApprovalCommand' \
  internal/modules/application internal/modules/kernel/governance internal/modules/blocks/decision internal/modules/blocks/approval

make verify-boundary-contract
go test -race ./internal/modules/blocks/run/... ./internal/modules/blocks/workspace/... -count=1
go test -race ./internal/modules/kernel/orchestrator/... ./internal/modules/blocks/{decision,approval,result}/... -count=1
go test -p 1 ./... -count=1
```

以上扫描在 S0～S5 仅用于盘点；S6 后必须零命中并纳入 `verify-boundary-contract.sh`。

## 8. 跨状态机事实合同

每个跨 owner fact 至少包含：`workspaceID`、`runID`、source owner、causation ID、适用 definition/evidence
revision、authority snapshot、dispatch ID、terminal delivery ID 或 governance binding。scope、causation 或 sealed
identity 不匹配时，接收 owner 记为 diagnostic/no-op；禁止猜测、补齐或跨 owner 查询后直接改状态。

| 事实 | 发布 owner | 消费 owner | 接收 owner 可产生的内部迁移 |
| --- | --- | --- | --- |
| activation intent accepted | Run | Run（内部） | `pending -> active` |
| Run state changed | Run | Workspace、Orchestrator、projection | Workspace 更新自身投影；Orchestrator仅调度 SM-4 event |
| governance wait recorded | Orchestrator | Run | `active -> blocked_*` |
| continuation eligible | Orchestrator | Run | `blocked_* -> active` |
| terminal delivery request | Orchestrator SM-4 | Result / Workspace / Run | 各 owner 仅变更自身资源状态 |
| owner delivery acknowledgement | Result / Workspace / Run | Orchestrator SM-4 | 仅更新 SM-4 delivery ledger |

## 9. 完成定义与实施纪律

只有 S0～S7 全部完成，且每个状态字段可在代码、contract、测试中定位唯一 owner，才可关闭
`RUNTIME-SMO`。还必须满足：

1. 跨组件 handler 不直接赋值或持久化别的 owner state；
2. Orchestrator 不持有 Run/Workspace repository 或 lifecycle subsystem；
3. Start/Resume、governance、terminal、recovery 都经过 owner-local event；
4. duplicate、stale、CAS failure、response/ack loss、fresh-process restart、budget/guard、terminal
   immutability 均有回归；
5. 门禁、组件 race、全仓、restart matrix、fresh deployed fixture 通过；
6. active 文档不存在“Orchestrator 直接投影 Run/Workspace/Result”的冲突叙述。

实施时不做兼容，不将迁移逻辑放入 `pkg`/HTTP/TUI/service/Eventstream/generic helper，不以 callback、
repository 注入、全局 service、map/JSON envelope 连接 owner，也不扩大 SM-2 的领域语义或改变已收口的
candidate manifest/evaluator 合同。

## 10. 文档同步与清理清单

S0 必须将当前设计和实施说明同步到下表；这些文件共同构成唯一可阅读的标准。若旧文档仍声明
“Orchestrator 直接 Start/Resume、blocked/terminal 投影或写 Result/Workspace/Run”，必须在同一变更中改写；
无法保持当前语义的过程稿迁入 `docs/90-archive/`，不得留下兼容解释。

| 文档层 | 必须同步的文件 | 唯一表述 |
| --- | --- | --- |
| 核心合同 | `docs/00-core/runtime-stage-contract.md` | 单 owner、owner-local event、immutable fact、`Result -> Workspace -> Run` delivery saga；VPC/SMO 的职责不重叠。 |
| 实现 SoT | `internal/modules/kernel/orchestrator/readme.md`、`internal/modules/{blocks/run,blocks/workspace,blocks/result}/readme.md` | Orchestrator 只保留 SM-2/3/4；三个 Block 只变更自己的 state。 |
| 应用/治理 SoT | `internal/modules/application/workorchd/readme.md`、`internal/modules/kernel/governance/readme.md` | Application 只提交 intent；Governance 只提交 resolution/fact；均不恢复或改写 Run。 |
| 当前事实与活动索引 | `docs/01-overview/current-state.md`、`current-unclosed-matrix.md`、`docs/70-roadmap/active/overview.md`、`docs/README.md` | `RUNTIME-SMO` 是未关闭代码项；RUNTIME-VPC 仅负责 candidate/receipt。 |
| 运行流程 | `docs/20-runtime-flow/{overview,activation-definition-flow,repair-replan-flow,evidence-result-flow,verification-acceptance-flow}.md` | Start/Resume intent → Run owner notification → drive；治理/终态均走 immutable fact。 |
| 模块架构 | `docs/30-module-architecture/{architecture-overview,application-workorchd,kernel-orchestrator-governance,blocks-workspace-run-result}.md` | module/block boundary、owner-local reducer 与 SM-4 acknowledgement。 |
| 产品/领域/API | `docs/05-product/{prd,product-architecture,business-model-flow-contracts}.md`、`docs/10-domain-model/{governance-decision-approval,run-step-result-artifact}.md`、`docs/50-product-interface/{overview,service-api}.md` | 对用户可见的 Start/Resume、governance、Result 与 terminal 均不得暗示跨 owner state write。 |
| 验证与矩阵 | `docs/60-verification/{overview,core-runtime-verifier}.md`、`component-contract-and-regression-closure-matrix.md` | 将 owner-local / duplicate / stale / ack-loss / restart 作为必测门禁。 |

`docs/40-profiles-capabilities/**`、LLM request/cache、read-model/event observability 只描述 profile、请求或只读投影，
不得新增 lifecycle 规则；若后续发现其写入 owner state 的叙述，删除该段而不是补充例外。所有
`docs/90-archive/**` 仅保留历史索引，不回写为现行实现合同。

## 11. 旧 topic / facade 删除清单（S0 定稿，S6 执行）

S0 已定义下列替代合同。以下旧入口在 S6 前仅作盘点；S6 后必须零命中。

### 11.1 删除的 Orchestrator lifecycle command topic

| 旧 topic | 旧 DTO | 替代 |
| --- | --- | --- |
| `/orchestrator/command/start-run` | `StartRunCommand` | Run `TopicRunCommandActivate` |
| `/orchestrator/command/resume-run` | `ResumeRunCommand` | Orchestrator `RuntimeContinuationEligible` fact → Run `TopicRunCommandApplyContinuationEligible`；手动 continue 用 Run `TopicRunCommandContinue` |
| `/orchestrator/command/block-decision` | `BlockOnDecisionCommand` | Orchestrator `RuntimeGovernanceWaitRecorded` → Run `TopicRunCommandApplyGovernanceWait` |
| `/orchestrator/command/block-approval` | `BlockOnApprovalCommand` | 同上 |
| `/orchestrator/command/cancel-run` | `CancelRunCommand`（RunID-only） | Run `TopicRunCommandCancel`（scoped） |
| `/orchestrator/command/supersede-run` | `SupersedeRunCommand`（RunID-only） | Run `TopicRunCommandSupersede`（scoped） |
| `/orchestrator/command/fail-run` | `FailRunCommand` | SM-4 terminal delivery → Run `TopicRunCommandApplyTerminalDelivery` |

### 11.2 删除的 Orchestrator facade / subsystem

| 符号 | 替代 |
| --- | --- |
| `AcceptRunStart` | Run owner activate reducer |
| `ActivateRuntimeV2Run` / `CanResumeRun` | `RuntimeContinuationEligible` + Run continue reducer |
| `BlockOnDecision` / `BlockOnApproval` | `RuntimeGovernanceWaitRecorded` + Run wait reducer |
| `ProjectRuntimeV2Terminal` / `closeLoadedRun` lifecycle write | SM-4 terminal delivery saga |
| `runStateSubsystem` / direct `RunRepository`+`WorkspaceRepository` mutation | owner-local commands only |
| `projectRuntimeV2Result` 预设 `Result.State` create | Result `TopicResultCommandApplyTerminalDelivery` |

### 11.3 新增合同（S0 已落地代码）

| 合同 | owner 包 | topic |
| --- | --- | --- |
| `ActivateRunCommand` | `blocks/run/pkg/events` | `/run/command/activate` |
| `ContinueRunCommand` | `blocks/run/pkg/events` | `/run/command/continue` |
| `ApplyRuntimeGovernanceWaitCommand` | `blocks/run/pkg/events` | `/run/command/apply-governance-wait` |
| `ApplyRuntimeContinuationEligibleCommand` | `blocks/run/pkg/events` | `/run/command/apply-continuation-eligible` |
| `ApplyRuntimeTerminalDeliveryCommand` | `blocks/run/pkg/events` | `/run/command/apply-terminal-delivery` |
| `CancelRunCommand` / `SupersedeRunCommand`（scoped） | `blocks/run/pkg/events` | `/run/command/cancel` / `/run/command/supersede` |
| `ApplyRunProjectionFactCommand` | `blocks/workspace/pkg/events` | `/workspace/command/apply-run-projection-fact` |
| `ApplyRuntimeTerminalDeliveryCommand` | `blocks/workspace/pkg/events` | `/workspace/command/apply-terminal-delivery` |
| `ApplyRuntimeTerminalDeliveryCommand` | `blocks/result/pkg/events` | `/result/command/apply-terminal-delivery` |
| `RuntimeGovernanceWaitRecordedData` | `kernel/orchestrator/pkg/events` | `/orchestrator/runtime-lifecycle/governance-wait-recorded` |
| `RuntimeContinuationEligibleData` | `kernel/orchestrator/pkg/events` | `/orchestrator/runtime-lifecycle/continuation-eligible` |
| `RuntimeTerminalDeliveryRequestData` | `kernel/orchestrator/pkg/events` | `/orchestrator/runtime-lifecycle/terminal-delivery-requested` |
| `RecordRuntimeDeliveryAcknowledgementCommand` | `kernel/orchestrator/pkg/events` | `/orchestrator/runtime-lifecycle/record-delivery-acknowledgement` |
| `RuntimeTerminalDeliveryIdentity` / `RuntimeDeliveryAcknowledgement` | `workorchruntimecontract` | shared pure value |

### 11.4 S0 退出核对

- [x] core contract / readme / matrix 已声明单 owner 与 delivery saga（工作区 docs 同步）
- [x] intent/fact/ack DTO 与 topic 已定义，字段注释标明 owner
- [x] 旧 topic 删除清单明确（本节）
- [ ] S1～S6 代码收口：2026-07-27 复核已撤销此前完成判定；旧 Orchestrator facade 虽已删除，但第 12 节所列 generic 写入、identity、CAS、delivery 与恢复缺口仍可破坏单 owner 语义。
- [ ] S7 fresh deployed / live 验证：使用全新 workspace 覆盖 activation、governance continuation、terminal delivery restart。

## 12. 实现复核后的剩余收口卡（唯一执行队列）

本节按状态写入风险排序，而不是按文件目录排序。所有卡均禁止兼容双写；任何 `pkg` 只能保存 DTO、
合同与纯无状态 helper，状态迁移、CAS、重试和 EventHub I/O 必须留在对应 owner `biz`。

### R1：封死 generic lifecycle 写入口

**问题。** Workspace generic update 仍可携带并覆盖 `State`、`ActiveRunID`；Run generic update 仍可覆盖
blocker、failure、时间戳、lineage、priority 等 SM-1 元数据。这是与 owner reducer 平行的第二状态写路径。

**涉及组件。** `blocks/workspace`、`blocks/run`、`application/workorchd`；SM-1 与 Workspace execution
projection。

**收口。**

1. 将 Workspace generic update 合同拆为 definition/metadata allow-list，彻底移除 `State` 与 `ActiveRunID`；
   Lifecycle 字段只允许 Workspace lifecycle reducer 写入。
2. 将 Run generic update 限定为非 lifecycle 业务字段；blocker、failure、started/ended、lineage、priority 和
   state 一律只能由 Run reducer 派生。
3. Application 的 workspace metadata / LLM target 更新改为窄 owner command，禁止 read-modify-write 回传
   lifecycle 字段。
4. `verify-boundary-contract.sh` 增加静态检查，拒绝 generic command/model 映射写入上述字段。

**退出测试。** 并发执行 metadata update 与 Run/Workspace lifecycle projection；验证 lifecycle 字段不会回退。
错误字段必须在合同校验阶段拒绝，fresh-process 重放也不得恢复旧快照。

**2026-07-27 实施进度。** Workspace generic update 已只接受 definition/metadata allow-list，Run generic
update 已只接受 Runtime V2 aggregate 与双 revision fence。首次 `pending -> active` 发生在 SM-2 aggregate
初始化之前，现以显式 `RuntimeStateRevision=0` 的 Run owner pre-runtime projection fact 收敛 Workspace slot；
该例外只允许 `pending|active`、必须带 scope/causation/definition revision，且不得混入 authority/evidence/
dispatch identity。任何已有 Runtime state 的事实仍必须带 sealed authority/evidence identity。R1 的本地合同
测试已落地，fresh deployed activation 仍属 S7。

### R2：让 Workspace terminal projection 覆盖行政终态与 replacement

**问题。** `cancel/supersede` 通知不会触发 Workspace release；replacement 路径会先 supersede 旧 Run，再启动
新 Run，旧 `ActiveRunID` 因而可能拒绝新 Run projection。

**涉及组件。** `blocks/workspace`、`blocks/run`、`application/workorchd`；SM-1、Workspace projection、SM-4。

**收口。** 取消和 supersede 必须经 R5 的 terminal delivery saga；Workspace owner 只按经过 sealed identity
校验的 terminal fact 清空同一 Run slot，并持久化 owner-local release identity。replacement 只能在旧 Run
release acknowledgement 持久化后创建/激活新 Run，或由同一 owner-local CAS 交接 slot；不得依赖异步 notification
时序。

**退出测试。** cancel、supersede、definition stale replacement、restart replacement 四个场景均验证：旧 terminal
不能释放新 Run，新 Run不会与旧 slot 冲突，crash/retry 后 slot 唯一。

### R3：补全 governance 与 Run notification 的 sealed fact identity

**问题。** governance wait/continue 仅按 ticket 或 approval binding 匹配；Run state notification 未完整携带或
消费者未校验 source、causation、definition/evidence revision、authority/dispatch identity。

**涉及组件。** `kernel/orchestrator`、`blocks/run`、`blocks/workspace`、`blocks/decision`、`blocks/approval`；
SM-1、SM-2、SM-4。

**收口。** 所有 governance/continuation fact 和 Run notification 必须具备并验证：workspace/run scope、
source owner、causation ID、definition revision、evidence revision、authority snapshot、dispatch 或 governance
binding。Run owner从自身持久化 Runtime V2 aggregate/terminal binding 验证事实，而不是仅比较 ticket。消费者必须
验证 Event source；缺字段、旧 revision 或 identity 不匹配统一 diagnostic/no-op，不得迁移或调度 drive。

**退出测试。** 覆盖 ticket/approval 相同但 authority、dispatch、revision 或 causation 不同；覆盖伪造 source 和
过期 active notification。所有反例均零 Run/Workspace/Orchestrator 状态变化。

**2026-07-27 实施进度。** Run governance/terminal handler、Workspace lifecycle handler 与 Orchestrator
continuation scheduler 都校验实际 EventHub source，不能只信任 payload 的 `SourceOwner`。Workspace 的 Run
notification bridge 必须由 Workspace Biz 自身发布，且 payload 仍须声明 `blocks/run`；terminal command 必须
由 `kernel/orchestrator` 实际发出。伪造 source 的 reducer regression 已落地。

### R4：Result owner 自行构造终态并为三个 owner 建立 delivery fence

**问题。** Orchestrator 仍传入带 `Result.State` 的完整 model；Run/Workspace/Result 没有持久化 delivery
identity/fence，无法区分合法重放与不同 sealed terminal 的冲突投递。

**涉及组件。** `blocks/result`、`blocks/workspace`、`blocks/run`、`kernel/orchestrator`；SM-1、SM-4。

**收口。**

1. Result terminal command 改为只包含 sealed terminal fact 和必要 evidence/result payload；Result owner 自行从
   fact 派生 accepted/rejected/recorded 状态，禁止调用方提供 `Result.State` 或完整可变 Result model。
2. 三个 owner model 分别持久化 immutable `TerminalDeliveryID`、完整 sealed identity hash 和 owner-local
   acknowledgement/fence revision；同 identity 重放返回原 receipt，不同 identity 返回 conflict。
3. Run terminal reducer必须先校验 identity，再判断 terminal/no-op；Workspace slot 为空或归属其他 Run 时也必须
   校验 delivery 是否为已确认的同一 identity，不能将任意 terminal 当作成功。

**退出测试。** owner 级 duplicate、不同 delivery ID、不同 authority、不同 terminal code/summary/evidence、
ack 丢失和 fresh-process 重放矩阵；仅相同 identity 可以幂等成功。

**2026-07-27 实施进度。** shared `RuntimeTerminalDeliveryIdentity` 已强制 delivery/workspace/run/
authority/definition/evidence/dispatch/action/code/summary 的完整 identity；Result、Workspace、Run 和
Orchestrator delivery request 均委托同一 validator。caller 不能再以 partial identity 取得 owner delivery ack。

### R5：完成 SM-4 terminal saga、顺序和 CAS

**问题。** acknowledgement reducer 首次登记时未绑定 sealed aggregate terminal；delivery owner command 未统一
lane；Run/Workspace lifecycle 持久化未使用 revision/CAS；行政终态还可绕过 `RuntimeTerminationRecord`。

**涉及组件。** `kernel/orchestrator`、`blocks/result`、`blocks/workspace`、`blocks/run`；SM-1、SM-4。

**收口。**

1. cancel/supersede 作为 Application intent 进入 Orchestrator `RequestTermination`，CAS 持久化
   `RuntimeTerminationRecord` 后才发起统一 terminal delivery；删除 Run direct terminal path。
2. SM-4 acknowledgement reducer在首次 ack 前比较 aggregate sealed terminal identity，并强制
   `Result -> Workspace -> Run` 顺序；非法 owner、顺序、causation 或 terminal identity 一律 conflict。
3. owner terminal command 均绑定 owner lane key；Run/Workspace/Result lifecycle model/repository 增加 revision
   CAS。CAS conflict 仅重新读取并按同一 delivery identity 重放，不能重新推导 authority。

**退出测试。** 每一步 crash、ack loss、并发 duplicate、乱序 ack、CAS conflict、administrative termination 与
fresh-process restart；SM-2 terminal decision 在任何 delivery retry 中保持不变。

**2026-07-27 实施进度。** Run、Workspace、Result 已持久化 owner-local `Revision`；memory、file 与
workspace-scoped repository 均提供 compare-and-swap。Run/Workspace lifecycle reducer 在 conflict 后只重读并以
同一 immutable fact 重放，Result create conflict 则重读并比对 terminal delivery identity。已覆盖 normal 和
administrative supersede 在 Workspace/Run owner 持久化后 acknowledgement 丢失、fresh-process 重放与重复 delivery。
已补齐 Result persistence acknowledgement 丢失的 administrative supersede 场景，以及 Result create race、Workspace
release CAS conflict、Run terminal CAS conflict 的 owner-level replay matrix。replacement administrative intent 现在只在
同步 receipt 已证明 Workspace release acknowledgement 已持久化后才能创建新 Run；definition metadata 变更不再清空
`ActiveRunID` 或改写 Workspace lifecycle，必须经同一 replacement/terminal delivery 路径收敛。仍需完成 fresh
deployed/live 验证；在这些退出证据完成前 R5 保持进行中。

### R6：使静态门禁验证真正覆盖 owner 语义

**问题。** 现有 gate 仅能证明旧 facade 名称不存在或目录中出现 Base，不能阻止 generic lifecycle 回流、
Result 外部 state、缺 source/lane/identity 或 Biz 间接嵌入 Base。

**涉及组件。** `scripts/verify-boundary-contract.sh`、22 个 Module/Block readme 与相关测试。

**收口。** gate 必须至少检查：Biz 类型直接嵌入 `basebiz.Base`；generic Workspace/Run command 不含 lifecycle
写字段；Result terminal command 不携带调用方指定 state/model；production terminal command 均绑定 lane；
跨 owner lifecycle fact 的 required identity 字段不可省略；Orchestrator 构造不接受其他 owner repository/service。

**退出测试。** 为每条 gate 规则提供最小 fixture/negative regression，避免简单 `rg` 误报或漏报；历史 archive
显式排除。

**2026-07-27 实施进度。** 已删除 Run contract/module/Biz 中无生产消费者的 `StartRunCommand`、
`ResumeRunCommand`、`BlockOnDecisionCommand`、`BlockOnApprovalCommand`、`FailRunCommand` 旧表面；生产继续
只使用 scoped Activate/Continue 与 immutable governance/terminal facts。

### R7：SM-4 startup/recovery 失败必须可观察、可恢复

**问题。** inventory 之后单个 schedule 或 blocked-governance aggregate load 失败只记录 warning，调用方可能
收到 Start 成功而 active Run 没有任何后续 drive。

**涉及组件。** `kernel/orchestrator`、`application/workorchd`；SM-4。

**收口。** 对受理范围内的 schedule/load 失败返回明确失败，或持久化可枚举的 recovery retry record 并由
BackgroundRoutine 重试；不得吞掉。Run notification 触发 drive 前先验证 Run owner fact 的 state/identity。

**退出测试。** 注入单 workspace schedule failure、aggregate load failure、重启后的 retry；验证没有“已 active
但无人 drive”的静默状态，且 retry 不重复投递既有 dispatch。

**2026-07-27 实施进度。** 新增独立的 persisted `drive_schedule` recovery budget。Run active 或 continuation
notification 的 BackgroundRoutine 拒绝不再仅写 warn：Orchestrator CAS 记录该预算后发出自身 typed retry event；
retry 同样受 run-scoped singleflight 与预算约束。预算耗尽会以 error log 和 aggregate ledger 保留可枚举状态，
下次 startup inventory 仍可重新 schedule 同一 active Run，且不重建 authority 或重放 dispatch。

### R8：完成文档、组件结构与 live 判定

**问题。** Run Biz经 Service 间接持有 Base，与组件 readme/skill要求不一致；计划曾将 S1～S6 标记完成，
但 active 索引仍显示未实施，形成双重标准。

**涉及组件。** `blocks/run`、`docs/`、`scripts/verify-boundary-contract.sh`；S0、S6、S7。

**收口。** Run Biz直接嵌入 `basebiz.Base`，删除无独立协议价值的 forwarding Service facade；同步所有 active
文档状态。仅在 R1～R7 通过后执行 fresh deployed matrix：新 workspace activation、governance block/continue、
normal/administrative terminal、ack-loss restart、replacement、startup recovery。

**退出测试。** `make verify-boundary-contract`、受影响 owner race、`go test -p 1 ./... -count=1`、file-backed
restart matrix 和 fresh deployed live 全部通过；每次 live 记录附上 workspace/run、sealed identity、delivery
ledger 与 owner notification 证据。

### 12.1 当前合规判定

| 范围 | 判定 | 依据 |
| --- | --- | --- |
| SM-1 Run lifecycle | 本地合同符合；live 待验证 | generic 写入口已封死，治理/terminal source 与完整 identity 已收紧；仍需 S7 deployed matrix。 |
| Workspace projection | 本地合同符合；live 待验证 | 首次 activation pre-runtime projection、slot fence、terminal release 与 source 校验已覆盖；仍需 S7 deployed matrix。 |
| SM-2 authority/focus | 主体符合 | 本次未发现重新引入跨 owner action selection；仍依赖 R3/R5 的事实和终态边界。 |
| SM-3 candidate/dispatch | 主体符合 | 本次未发现新的跨 owner operation execution 或 next-action 写入。 |
| SM-4 delivery/recovery | 本地合同符合；live 待验证 | delivery identity/顺序/CAS 已 owner-local；schedule rejection 已写入独立 recovery ledger 并有有界 retry。 |
| `pkg` 边界 | 部分符合 | `workorchruntimecontract` 可保留纯值；状态迁移/I/O 必须按 R1～R7 留在 owner Biz。 |
| S7 live | 未验证 | 不得以本地测试替代 deployed/restart matrix。 |
