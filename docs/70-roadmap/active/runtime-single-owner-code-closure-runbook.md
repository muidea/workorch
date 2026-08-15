# Runtime 单 Owner 执行链路代码收口 Runbook

Status: active implementation appendix

Type: code-closure-runbook

Last Updated: 2026-08-15

本文件是
[Runtime single-owner and model-capability output closure plan](runtime-single-owner-model-capability-output-closure-plan.md)
的逐项实施附录，不新增 Runtime 状态机、Owner 或生产主链。若本文与
[`docs/00-core/runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md) 或
`internal/modules/kernel/orchestrator/readme.md` 冲突，以核心合约和实现 SoT 为准。

## 1. 收口目标与当前证据

目标是把 Workspace 执行过程固定为一条可审计链路：每个状态机只有一个 Owner，状态转换只能由该 Owner 的
内部事件触发；跨组件只交换带 scope 的 typed command/result/fact/receipt；任何候选、目标、证据或终态都
不能越权成为另一个状态机的权威。

最近部署的 `run-002` 已证明以下边界有效：

- `cwi-026` 由 Orchestrator 完成 binding、dispatch、File receipt、evidence 和 assessment；
- File 对已有文件的 `file/write` 请求在产生第二次副作用前拒绝；
- `cwi-027` 没有 dispatch、receipt 或 workspace effect；
- Run 最终进入 `failed`，没有发现跨组件直接写 Run/Workspace/Result 状态。

代码侧的目标合同一致性已在本轮收口：`cwi-027` 的 prospective
`coverage-map.json` 现在会在 SM-2 进入候选前强制调度 File owner 的 typed target-selection；File 只验证
workspace-relative canonical path 与目标不存在性，已有目标返回 `Unsupported` 事实，不会授权
create-only `file/write`。因此，`target_grounding_unresolved` 仍作为 Orchestrator 的合法 fail-closed 终态，
但不会再因遗漏 prerequisite 而先进入 provider/操作链路。

当前功能闭环仍需使用清理后的数据重跑 `cwi-026`/`cwi-027`，确认 existing target 选择
revision-bound replacement、prospective target 完成 owner grounding，并观察 `cwi-027` 执行及最终 live closure。
与此同时，22 单元复杂度盘点已确认 Orchestrator 的外部 Owner 边界基本正确，但内部事件 union、状态事务模板、
semantic planning、candidate admission、grounding、evidence、governance 和 delivery 职责仍过度集中。本 Runbook
因此增加 O0-O8 Orchestrator 内部功能内聚专项和 WD0-WD8 Workorchd Application Module 内部功能内聚专项：
两者都不新增 Owner 或状态机，分别收窄 Runtime mutation core 与应用入口/用例/projection/transport 边界。
O0-O8 与 WD0-WD8 的代码/自动化收口现已完成；fresh-data live run 未完成前，仍不宣称 SO-MC-10 或本轮
deployed closure 已关闭。

## 2. 不可变规则

### 2.1 状态机 Owner

| 状态机/资源 | 唯一 Owner | 唯一合法 mutation entry | 其他组件只允许做什么 |
|---|---|---|---|
| SM-1 Run lifecycle、lineage、replacement intent | `internal/modules/blocks/run` | Run-private internal event + Run CAS | 发送 intent，消费 Run fact |
| SM-2 authority、focus、CWI、binding、evidence、terminal decision | `internal/modules/kernel/orchestrator` | Orchestrator-private SM-2 event + Runtime CAS | 提供 typed observation/receipt |
| SM-3 request session、candidate、attempt、sealed dispatch、receipt admission | `internal/modules/kernel/orchestrator` | Orchestrator-private SM-3 event + Runtime CAS | 提供 provider document 或 capability receipt |
| SM-4 drive、recovery、lease、reconciliation、delivery ledger | `internal/modules/kernel/orchestrator` | Orchestrator-private SM-4 event + Runtime CAS | 提供 lifecycle notification、delivery acknowledgement |
| Workspace definition、active Run slot | `internal/modules/blocks/workspace` | Workspace-private internal event | 读取定义、发送 active-run intent |
| terminal Result lifecycle | `internal/modules/blocks/result` | Result-private internal event | 接收 immutable terminal delivery |
| Decision ticket/resolution | `internal/modules/blocks/decision` | Decision-private internal event | 接收/返回治理事实 |
| Approval lifecycle | `internal/modules/blocks/approval` | Approval-private internal event | 接收/返回审批事实 |
| provider request/response lifecycle | `internal/modules/blocks/llm` | LLM owner request lifecycle | 返回 provider factual outcome |
| capability resource/receipt | 对应 `file/git/shell/mcp/web/ssh` Block | Capability-private internal event | 返回 factual observation/receipt |
| projection/read model | `workorchd`、`eventstream` | projection reducer | 只投影，不作 authority |

SM-2、SM-3、SM-4 都属于 Orchestrator，不能拆成三个 framework Module。`biz/runtime/` 下的文件可以按
职责拆包，但必须共享同一个 owner、private event dispatcher、Runtime aggregate 和 CAS 边界。

### 2.2 统一事件模式

每个有状态组件都必须满足：

```text
typed external command/fact
  -> owner handler: scope + identity + causation + freshness validation
  -> owner-private internal event
  -> owner-private reducer
  -> one owner repository/CAS write
  -> typed result / immutable fact / acknowledgement
```

EventHub command/result 不是状态转换。跨 owner payload 禁止携带：

- `targetState`、`nextAction`、`retry`、`focus advance`、`terminal decision`；
- 其他 owner 的 repository/service/adapter/callback/可变资源；
- `map[string]any`、`[]any`、JSON/反射兼容桥或通用 Envelope；
- 未带 workspace/run/authority/dispatch scope 的局部 ID。

### 2.3 Runtime 权威链

生产路径只能是：

```text
Run intent
 -> Run SM-1 internal event/CAS
 -> RunStateChanged fact
 -> Orchestrator SM-4 drive
 -> Orchestrator SM-2 authority/focus
 -> Orchestrator SM-3 semantic request/candidate/sealed dispatch
 -> capability owner operation + receipt
 -> Orchestrator SM-2 evidence/assessment/terminal
 -> SM-4 Result -> Workspace -> Run delivery
```

禁止恢复 `StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、自动 successor 或
projection-driven Resume。

## 3. 组件级实施队列

每个工作包完成后必须满足“代码、测试、边界扫描、文档”四项，不能只修改现场报错对应的一个函数。

### W0 — 建立 Owner 与入口基线

**Owner/代码落点**

- `internal/modules/kernel/orchestrator/biz/runtime_internal_events.go`
- `internal/modules/kernel/orchestrator/biz/runtime/transition_coordinator.go`
- `internal/modules/blocks/run/biz/lifecycle_internal_event.go`
- `internal/modules/blocks/workspace/biz/`
- `internal/modules/blocks/result/biz/`
- `internal/modules/blocks/decision/biz/`
- `internal/modules/blocks/approval/biz/`

**动作**

1. 列出每个状态字段的唯一写入函数和唯一 CAS repository。
2. 将外部 handler 收敛为“校验后提交 private internal event”。
3. 将 reducer/transition handler 保持 package-private；不得由 service、application 或测试生产适配器直接调用。
4. 清除跨 owner service/repository 注入和全局 runtime registry。

**验收**

- 静态扫描找不到非 Owner 写入 `Run.State`、`Workspace.ActiveRun`、`Result.State`、Runtime Facts 或 CWI ledger。
- duplicate/stale/CAS conflict 只经过同一个 owner reducer。
- owner module 的 Hub 订阅位于 `biz`，不在 `module.go` 或 `service`。

### W1 — Run SM-1 与 opaque Runtime persistence

**Owner：** `blocks/run`

**动作**

1. Start/Resume/Restart 只接受 scoped typed intent。
2. Run 内部决定 `pending -> active -> terminal/blocked`，Orchestrator 不传 lifecycle target state。
3. Runtime aggregate 仅作为 opaque record 通过 Run CAS 保存；Run 只校验 scope、schema、revision、hash。
4. Run 不解析 `Decision`、`SemanticPlanning`、candidate、dispatch、governance、evidence 或 delivery 内容。
5. 非当前 aggregate Resume 返回 `fresh_activation_required`；Restart 生成新 Run lineage，不迁移旧 CWI credit。

**重点测试**

- `internal/modules/blocks/run/biz/state_transitions_test.go`
- `replacement_intent_test.go`
- `recovery_inventory_boundary_test.go`
- 同 revision duplicate、CAS mismatch、restart isolation、terminal delivery。

### W2 — Orchestrator SM-2 focus/CWI/binding 收口

**Owner：** `kernel/orchestrator`，主要代码在 `biz/runtime/`。

**动作**

1. `Facts.Coverage` 是唯一 CWI ledger；不得新增第二份 CWI/focus 状态。
2. 当前 focus、eligible closure、semantic order、proof batch、feasibility set、binding set 必须来自同一
   persisted closure。
3. 只有 SM-2 private event 可以执行 focus advance、retain、reopen、binding retirement、evidence credit、terminal decision。
4. settled CWI 只可通过 typed reopen lineage 重新进入 planning，不能由 provider 或 capability owner 直接 reopen。
5. 每个 binding 必须包含 deterministic target 或 owner selector、operation kind/variant、target policy、revision
   （适用时）、recipe/check/effect/evidence requirement。

**重点测试**

- `cwi_ledger_test.go`
- `reconciliation_test.go`
- `coverage_evidence_binding_lifecycle_test.go`
- `cwi_binding_fence_test.go`
- eligible focus、blocked dependency、binding retirement、reopen lineage、no equivalent authority。

### W3 — Orchestrator SM-3 candidate/dispatch 单事务

**Owner：** `kernel/orchestrator`。

**动作**

1. `CandidateRequestSession`、attempt 0/1、candidate evaluator、sealed dispatch 和 operation receipt admission
   只能由 SM-3 private event 修改。
2. semantic-plan accepted 只产生 internal fact，不创建 dispatch，不写 evidence，不选择 Runtime action。
3. operations candidate 必须匹配已封存 manifest/binding；provider 不能创建 target、binding、check、metric、artifact、credit 或 retry authority。
4. candidate admission 必须在一次 Orchestrator CAS 中完成 candidate acceptance、binding match、dispatch seal。
5. owner feasibility 的 stale/infeasible/prerequisite 先转成 Orchestrator internal event，再由 SM-2 决定 refresh、replan 或 terminal；不得由 File/LLM 选择下一动作。
6. dispatch 执行前持久化 `ExecutionKey`、Fence、ExpectedEffectScope、BindingRef；receipt/evidence 不匹配时禁止 side effect credit。

**重点测试**

- `candidate_admission_test.go`
- `candidate_request_session_test.go`
- `dispatch_commit_test.go`
- `operation_receipt_test.go`
- `runtime_candidate_event_adapter_test.go`
- incomplete document、unknown field、stale response、duplicate candidate、same authority signature、dispatch fence、receipt replay。

### W4 — File target/feasibility/receipt 内聚

**Owner：** `internal/modules/blocks/file`

**动作**

1. File 维护自己的 target selection、workspace manifest、operation catalog、feasibility policy 和 receipt lifecycle。
2. File 只返回事实：target existence/type/revision/hash、selector result、operation feasibility、execution receipt。
3. `file/write` 只允许 create-only prospective target。
4. 已存在目标必须使用 revision-bound `patch/apply` 或 `text_replace`；revision 由 File 在 preflight/execute 验证。
5. `unified_diff` feasibility 必须 dry-run，无写入；hunk/context/revision 冲突在 dispatch seal 前返回 typed infeasible。
6. File 不得更新 CWI、focus、binding、evidence、retry、terminal 或 Run/Workspace 状态。

**重点测试**

- `file_owner_feasibility_policy.go`
- `file_runtime_adapter_test.go`
- `owner_reconciliation_test.go`
- `file_runtime_variant_test.go`
- absent/create、existing/write reject、existing/revision-bound patch accept、stale revision、ambiguous selector、no side effect。

### W5 — LLM provider 与 capability owner 边界

**LLM：** `internal/modules/blocks/llm`

- ProviderCeiling 是唯一模型输出上限来源。
- LLM 只拥有 provider request/response、usage、physical call 和 provider diagnostic。
- 不决定 Runtime retry、attempt、terminal、target、binding 或 evidence。
- incomplete document 只返回 provider factual outcome；whole-document recovery 由 Orchestrator SM-4/SM-3 负责。

**其他 capability：** `git`、`shell`、`mcp`、`web`、`ssh`

- 各自拥有 `pkg/events` 中的 concrete command/result。
- 只返回 scoped observation/receipt。
- 不解释 Goal/DoD，不从事实中选择 CWI，不提交 Runtime 状态转换。

**验收**

- 同一 operation kind/variant/target policy 的 semantic feasibility 与 operation feasibility 来自同一 owner policy revision。
- 无 capability owner 的 CWI-specific retry、terminal 或 binding retirement。

### W6 — Workspace/Result/Decision/Approval delivery

**动作**

1. Orchestrator 只发布 immutable terminal/delivery fact。
2. Result owner 先执行自己的内部事件并返回 acknowledgement。
3. Workspace owner 再释放 active Run slot。
4. Run owner 最后执行自身 terminal transition。
5. Decision/Approval 只在各自 owner 内部推进 ticket/resolution/approval。
6. projection failure 只能进入 delivery retry，不得改写 Orchestrator 已确定的 terminal authority。

**重点测试**

- `runtime/delivery_acknowledgement.go`
- `runtime_result_projection.go`
- `internal/modules/blocks/result/biz/`
- `internal/modules/blocks/workspace/biz/`
- `internal/modules/blocks/decision/biz/lifecycle_internal_event.go`
- `internal/modules/blocks/approval/biz/lifecycle_internal_event.go`
- Result→Workspace→Run 顺序、单边成功重试、duplicate acknowledgement、terminal 不被 projection 覆盖。

### W7 — Application/Service/Initiator/Projection 去权威化

**不得新增 Runtime 状态逻辑的组件**

- `internal/modules/application/workorchd`：HTTP/SSE、DTO、projection、只读 facade。
- `internal/services/workorch`：TUI/CLI intent、查询、展示。
- `internal/initiators/workorchruntime`：repository provider/factory，不提供 Runtime reducer 或 policy。
- `internal/modules/blocks/eventstream`：audit/display projection，不作 authority。
- `internal/pkg/workorchruntimecontract`：纯 DTO/enum/normalize/validate/hash/compiler/evaluator。

**扫描要求**

- Application/TUI/Web 不得直接 import kernel owner repository/service。
- Projection 不得调用 `DriveRuntime`、Resume 或任何 owner reducer。
- Initiator 不得内嵌 `base/biz.Base`，不得组合 repository、Hub、runtime policy、route registry 成综合容器。
- Runtime contract package 不得持有 Hub、执行 I/O、CAS 或选择 retry/terminal/action。

### W8 — Boundary contract 与文档同步

需要同步维护：

- `scripts/verify-boundary-contract.sh`
- `docs/00-core/runtime-stage-contract.md`
- `internal/modules/kernel/orchestrator/readme.md`
- 本文件及主计划
- `docs/01-overview/current-unclosed-matrix.md`

扫描必须覆盖：

- 跨 owner 直接 repository/service 写入；
- `targetState`、`NextAction`、自动 Resume/Restart；
- 非 Orchestrator CWI/binding/dispatch/evidence mutation；
- Run 解码 Runtime 内部 authority；
- File/LLM 选择 Runtime action；
- equivalent AuthoritySnapshot；
- map/JSON/reflect authority bridge；
- projection 驱动运行或终态；
- `RemainingFocuses[0]` 作为执行游标；
- stage token configuration authority。

## 4. Orchestrator 内部功能内聚专项

本专项负责降低 `kernel/orchestrator` 的内部复杂度。它只拆 Owner 内部业务组件，不改变 22 个 framework
运行单元基线，不新增 `module.go`、plugin lifecycle、EventHub owner、repository 或正式状态。

### 4.1 组件拆分判定

Orchestrator 内部采用两种组件形态：

1. **有状态私有组件**继续位于 `internal/modules/kernel/orchestrator/biz/runtime` 同一个 Go package。
   它们可以处理 Runtime aggregate 的某一职责，但只能由唯一 private dispatcher 调用，并且只能通过同一个
   `RuntimeTransitionCoordinator.Commit` 写入 aggregate。
2. **无状态 focused package**位于 `internal/modules/kernel/orchestrator/biz/internal/<function>`。
   它们只能执行确定性编译、规范化、校验、比较或评估，输入输出均为 concrete typed value；不得访问
   EventHub、repository、CAS 或可变 aggregate，也不得决定下一状态、重试或终态。

以下情况禁止拆成新 package 或运行单元：

- 两部分必须按固定顺序修改同一个 aggregate；
- 需要相互调用 reducer、共享可变 session 或暴露 repository；
- 拆分后需要把 private Runtime 字段、Hub、observer 或 transition callback 导出；
- 只有转调 wrapper，没有独立规则、测试价值或清晰的一句话职责；
- 为了减少文件长度而创建新的 Module、Block、Owner 或 lifecycle。

### 4.2 目标组件图

```text
typed owner command/fact/receipt
          |
          v
orchestrator/biz adapters
scope + identity + causation + freshness only
          |
          v
RuntimeController.ApplyInternalEvent        <- 唯一 mutation entry
          |
          +-- authorityComponent             <- SM-2
          +-- planningTransactionComponent   <- SM-3 planning transaction
          +-- candidateDispatchComponent     <- SM-3 candidate/dispatch/receipt
          +-- driveDeliveryComponent         <- SM-4
          |
          v
RuntimeTransitionCoordinator.Commit         <- 唯一 aggregate CAS boundary
          |
          v
Run owner opaque Runtime aggregate persistence

Pure focused components used by the handlers:
semanticplan | candidateadmission | grounding | proof | governancecheck | requestcontent
```

一次内部事件只能由一个私有组件处理。跨组件/跨 SM 的后果不能在 Commit 内自动调用后继 reducer；当前事件先
提交，随后由 `DriveRuntime` 读取已提交 aggregate，再显式提交下一 owner-private internal event。

### 4.3 有状态私有组件边界

| 私有组件 | 内聚职责 | 允许修改 | 禁止承担 |
|---|---|---|---|
| `runtimeMutationKernel` | event metadata/payload 校验、exhaustive dispatch、load、invariant、CAS、persistence outcome | 不拥有业务状态，只提交目标 handler 产生的合法变更 | 选择 focus、candidate、retry、terminal 或调用 provider/capability |
| `authorityComponent` | focus、CWI ledger、binding、evidence、assessment、terminal authority | SM-2 对应 aggregate 字段 | 创建/执行 candidate、管理 drive lease、向其他 Owner 写状态 |
| `planningTransactionComponent` | semantic manifest、request session、batch、document、correction lineage | SM-3 planning transaction 字段 | 修改 CWI credit、执行 operation、自动触发 replan successor |
| `candidateDispatchComponent` | candidate attempt、feasibility、admission、seal、dispatch、operation receipt | SM-3 candidate/dispatch transaction 字段 | 选择下一 CWI、修改 Run lifecycle、直接接纳 evidence credit |
| `driveDeliveryComponent` | drive lease、recovery budget、re-drive、reconciliation scheduling、terminal delivery ledger | SM-4 对应 aggregate 字段 | rebuild authority、选择 candidate、修改 Result/Workspace/Run 状态 |

这些组件不是独立状态 Owner，不得拥有第二份 aggregate、repository、revision、retry ledger 或进程内权威 cache。

### 4.4 无状态 focused package 边界

目标目录：

```text
internal/modules/kernel/orchestrator/biz/internal/
  semanticplan/
  candidateadmission/
  grounding/
  proof/
  governancecheck/
  requestcontent/
```

| focused package | 一句话职责 | 主要迁移来源 | 输出限制 |
|---|---|---|---|
| `semanticplan` | 编译并验证 semantic plan 文档及 correction facts | `runtime/semantic_planning.go` 的纯编译、batch merge、document validation 部分 | 返回 document validation、normalized proposal、diagnostic；不保存 session |
| `candidateadmission` | 对候选执行完整、确定性的准入检查 | `candidate_admission.go`、`candidate_rejection_policy.go`、candidate fingerprint helper | 返回 typed finding；不 seal dispatch、不消耗 attempt |
| `grounding` | 比较 prerequisite、owner feasibility、target/revision freshness | `prerequisite.go`、`prerequisite_freshness.go`、repair surface 的纯比较部分 | 返回 grounded/stale/infeasible facts；不 reopen CWI |
| `proof` | 校验 coverage proof、evidence lineage、binding/receipt 匹配 | `assessment_evidence_gate.go`、`coverage_proof_lineage.go`、`reconciliation.go` 的纯判断部分 | 返回 proof finding；不写 CWI ledger、不接纳 credit |
| `governancecheck` | 校验治理需求与 resolution 是否匹配 sealed authority | `governance.go` 的纯 policy/result validation 部分 | 返回 governance finding；不创建 ticket、不提交 resolution |
| `requestcontent` | 构造 provider request 分层内容、identity 和 stable prefix hash | `runtime_candidate_request_content*.go` | 返回 immutable content/hash；不读取 repository、不选择模型动作 |

focused package 可以导出包内调用所需的类型，但这些类型必须保持 owner-private 使用面：不能进入 EventHub 合同，
不能成为第二套 Runtime contract，也不能反向导入 `biz` 或 `biz/runtime`。跨 Owner 稳定 DTO 只有在确有共享合同时
才允许进入 `internal/pkg/workorchruntimecontract`，该包仍不得包含 transition、CAS、I/O 或 retry 决策。

### 4.5 目标目录职责

```text
internal/modules/kernel/orchestrator/biz/
  owner.go                         # lifecycle、Base、订阅与装配
  runtime_drive.go                 # 唯一同步 drive shell
  runtime_semantic_drive.go        # provider 调用 adapter，不持有状态
  *_event_adapter.go               # typed owner fact -> private event
  internal/                        # 无状态业务规则组件
  runtime/
    controller.go                  # 窄 facade
    internal_event*.go             # 唯一 private protocol/dispatcher
    transition_coordinator.go      # 唯一 CAS coordinator
    authority_*.go                 # SM-2 私有组件
    planning_transaction_*.go      # SM-3 planning 私有组件
    candidate_dispatch_*.go        # SM-3 execution transaction 私有组件
    drive_*.go                     # SM-4 drive/recovery
    delivery_*.go                  # SM-4 terminal delivery
```

EventHub adapters 保持在 `biz` package。不得为了目录对称，让 focused package 持有 `basebiz.Base`、Hub、observer，
或要求 `Biz` 导出其内部资源。

### 4.6 Private event 与 dispatcher 收窄

当前 `RuntimeInternalEvent` 的顶层业务字段必须按 machine payload 收口，目标形态为：

```go
type RuntimeInternalEvent struct {
    Kind  RuntimeInternalEventKind
    Scope RuntimeScope

    Activation  *ActivationEventPayload
    Authority   *AuthorityEventPayload
    Transaction *TransactionEventPayload
    Delivery    *DeliveryEventPayload
}
```

具体字段名可根据现有 contract 调整，但必须满足：

- 每个 Kind 在 exhaustive metadata 中唯一映射到一个 machine、一个 payload class 和 freshness policy；
- 每个事件恰好设置一个与 Kind 匹配的 concrete payload；
- payload 错配、缺失、多 payload、scope 不完整必须在 repository load/CAS 前失败；
- 禁止 `any`、`map[string]any`、JSON/反射 envelope 或通用 mutation callback；
- `RuntimeController` 对外继续只保留 `ApplyInternalEvent`、`Load`、`QueryOperationReceipt`；
- `applyActivationEvent`、`applyAuthorityEvent`、`applyTransactionEvent`、`applyDeliveryEvent` 只是唯一 dispatcher
  的私有分支，不形成第二 mutation entry。

57 个 event kind 可以因业务事实继续存在；目标不是减少合法事件种类，而是消除 61 个顶层字段形成的非法组合空间。

### 4.7 Existing-target semantic replan 闭环

本次 `cwi-026` 失败必须按中立能力语义修复，禁止在 Orchestrator 核心硬编码 `file/write -> patch/apply`：

1. capability owner 返回 stable typed failure class、current target identity/revision、policy/catalog revision、
   replacement constraint 和原 dispatch/receipt identity；不得返回 Runtime next action。
2. `candidateDispatchComponent` 通过 SM-3 internal event 接纳失败 receipt，关闭当前 dispatch transaction，并保存
   rejected candidate semantic fingerprint；它不选择替代 operation。
3. `authorityComponent` 通过独立 SM-2 internal event判断 CWI 是否回到 `need_state_change`。若 proof/binding 合同
   未变化，只退休失效 candidate/dispatch；不得无理由重建等价 binding/authority。
4. `driveDeliveryComponent` 只依据已提交 aggregate 安排有界 re-drive，不重建 authority，不直接调用 semantic
   successor。
5. `semanticplan`/`requestcontent` 在下一 request 的 volatile context 中必须包含 rejected candidate、stable failure
   class、最新 capability/catalog policy、target revision、replacement constraint 和剩余 budget。
6. `candidateadmission` 在派发前比较：workspace/run/CWI、authority revision、capability snapshot、target revision、
   failure class 与 candidate semantic fingerprint。在输入事实未变化时，相同候选属于 `duplicate_no_progress`，
   必须在第二次副作用前拒绝。
7. 合法的新候选仍需经过 owner feasibility、sealed binding 和 revision fence，再由 SM-3 seal/dispatch。

Runtime 核心测试使用 `create_only`、`target_already_exists`、`revision_bound_replacement_required` 等中立语义；
File Block 集成测试再验证这些语义与 `file/write`、`patch/apply`、`text_replace` 的具体映射。

### 4.8 分批工作包

| ID | 影响状态机 | 工作内容 | 完成门禁 |
|---|---|---|---|
| O0 | SM-2/3/4 | 冻结一个 dispatcher、一个 aggregate、一个 coordinator、三个 controller exported method 的结构基线 | 静态边界测试和现有 Orchestrator 测试全绿 |
| O1 | SM-2/3/4 | 将 `RuntimeInternalEvent` 收窄为 Scope + 四类 concrete payload；补 Kind/payload/metadata 穷举测试 | wrong/missing/multiple payload 在 load/CAS 前失败；行为快照不变 |
| O2 | SM-2/3/4 | 在同一 `biz/runtime` package 内建立 mutation kernel、authority、planning transaction、candidate dispatch、drive delivery 私有组件 | 私有组件不互调 reducer；所有写入仍走 coordinator |
| O3 | SM-3、SM-4 drive input | 提取 `semanticplan`、`requestcontent`，补 existing-target failure context 和 cache hash 回归 | 重规划输入包含最新 revision/constraint；稳定 C0-C2 hash 不漂移 |
| O4 | SM-3 | 提取 `candidateadmission`，统一 feasibility/admission/fingerprint，增加 duplicate-no-progress fence | 重复 create-only 候选不产生第二次 owner execute |
| O5 | SM-2、SM-3 | 提取 `grounding`、`proof`；统一 freshness、binding/receipt/evidence 判断 | stale/duplicate/restart 下无错误 credit、reopen 或 dispatch |
| O6 | SM-3 | 提取 `governancecheck`，收窄治理 adapter 为 typed fact mapping | governance 不写 SM-2/SM-4，不形成第二 retry authority |
| O7 | SM-2/3/4 | 清理 `controller.go`、`semantic_planning.go`、`runtime_mainline.go`、`dispatch_commit.go`、`prerequisite_freshness.go`、`reducer.go` 的混合职责；仅合并完全等价的事务模板 | 无万能 callback transaction；文件不再同时承担 request、transition、CAS 多类职责 |
| O8 | SM-2/3/4 | 更新实现 SoT、boundary script、复杂度盘点和 live 验证 | 22 单元不变；fresh process 完成 cwi-026 replacement → cwi-027 → terminal delivery |

O0-O8 必须按顺序实施。每个工作包单独提交，禁止同时进行事件 schema、状态语义、package 移动和 live 修复，
以便出现回归时能够按工作包回退和定位。

### 4.9 提取与依赖门禁

一个函数或类型只有同时满足以下条件才允许移入 focused package：

1. 对相同输入产生确定性输出；
2. 不读取或写入 repository、EventHub、clock、network、filesystem；需要时间时显式传入值；
3. 不修改 Runtime aggregate、request session、ledger、lease 或 budget；
4. 不返回 `nextAction`、目标状态、retry 指令或 terminal 决定；
5. 不导入 `orchestrator/biz`、`orchestrator/biz/runtime` 或其他 Owner 的实现包；
6. 能通过独立 table-driven tests 验证正常、边界、重复、stale 和 malformed 输入。

建议增加 boundary scan，确保：

- `biz/internal/**` 不导入 framework EventHub、owner repository 或 `biz/runtime`；
- 只有 `biz` adapter 使用 EventHub，只有 `runtime` mutation kernel 使用 Runtime repository port；
- focused package 的类型没有出现在跨 Owner topic payload 中；
- 新目录没有 `module.go`、plugin registration、`basebiz.Base` 或 background lifecycle。

### 4.10 复杂度与行为验收

- framework 运行单元仍为 1 Initiator、4 Modules、17 Blocks；内部 package 不计为新运行单元；
- `RuntimeInternalEvent` 顶层不再平铺业务字段，event kind/payload metadata 100% 穷举；
- aggregate、private dispatcher、transition coordinator、repository/CAS 各保持唯一；
- controller exported method 仍只有三个，不因拆包增加 owner facade；
- focused package 均有一句话职责、单向依赖和独立测试；
- 不新增无必要 EventHub topic、公共 DTO、runtime registry 或兼容桥；
- 最大文件热点下降，但不能把同一事务按行数机械切成多个相互穿透的 package；
- 纯结构重构前后 aggregate serialization、authority hash、request identity 和 stable prefix hash 保持一致；
- `cwi-026` 首次 create 成功、reopen 后接纳 owner rejection、重复 create 候选零副作用、合法 revision-bound
  replacement 成功，随后 `cwi-027` 获得执行焦点；
- fresh process、duplicate receipt、stale result、CAS-before、CAS-after-response-loss、budget exhaustion 均保持幂等和
  fail-closed；
- 终态仍按 Result -> Workspace -> Run 投递。

## 5. Workorchd Application Module 功能内聚专项

本专项负责降低 `internal/modules/application/workorchd` 的包图、公开面、入口编排和 projection 复杂度。
Workorchd 继续是一个 Application Module；本专项不创建新的 framework Module/Block、正式状态 Owner、repository
或 Runtime 状态机。

### 5.1 当前复杂度基线

以 2026-08-14 的
[22 单元功能与复杂度盘点](runtime-22-component-function-complexity-inventory.md)为历史基线（不把后续 working-tree
结构迁移误算为行为收益）：

- 888 个生产 Go 文件、约 55,149 生产 LOC；132 个测试文件、约 38,919 测试 LOC；
- `biz/appservice` 有 28 个一级子包和 9 组 `*core + *ports` 配对，根目录仍有 55 个生产文件；
- `readmodelprojection` 有 404 个生产文件、约 17,342 LOC 和 230 个 exported declaration；
- `service/server` 有 29 个生产文件、约 8,629 LOC，`http_types.go` 约 96 KB；
- production code 未直接导入其他 Owner 的 `biz` 或 repository，跨 Owner 访问主要保持在 concrete EventHub
  contract/model/read port 上；
- `service/server` 当前没有直接持有 EventHub observer，但仍接收 `runtimecore.Session`，并承担 scheduler、projection、
  diagnostics、cache、metrics 和 SSE 多类职责。

因此 Workorchd 的首要问题不是 Owner 越权，而是 application transport、use case、owner gateway、projection/read model
与 runtime session 之间的依赖和公开面过宽。

截至 2026-08-15 的 working tree，Workorchd 为 650 个生产 Go 文件/56,173 行、153 个测试文件/41,573 行；
`biz/appservice` 有 25 个一级子包、根目录 54 个生产文件；`readmodelprojection` 当前为 140 个生产文件/
17,087 行、8 个 exported declaration（六个 facade factory 与两个 typed input；数字 family 文件已清零），
`service/server` 为 41 个生产文件/8,142 行。文件数量变化来自
职责物理化和低风险薄配对归并，不代表新增 Module、Owner、状态机或 Runtime authority。生产 server 已无
`runtimecore`/EventHub/root appservice 依赖，`biz/runtimeComponent` 是唯一长生命周期 Session owner；生产
`serverState` 不再保存 Session、cache-authority callback、projection lifecycle/observer 或双 wiring flag。包内测试
基础设施可以构造 owner runtime，但不再扩展生产 `Service` API 或提供 exported Session-backed server constructor。
WD5-WD7 已完成代码/自动化收口，WD8 的文档、指标和仓库门禁已固化；deployed HTTP/SSE/CLI 与 fresh-data Runtime
proof 仍保持 open。

### 5.2 不可变边界与本地状态归属

| 状态/职责 | Workorchd 内唯一归属 | 允许入口 | 禁止行为 |
|---|---|---|---|
| framework lifecycle、EventHub subscription | `biz.Biz` | `module.go -> Setup/Start/Stop` | `module.go`、server 或 focused package 自建 observer/订阅 |
| application runtime session | runtime-session component | Biz 构造、borrow、close | HTTP 保存 Hub/Session、跨 Owner service registry |
| workspace execution activation coordination | execution/activation use case | HTTP、CLI、Workorchd typed command 调用同一 use case | transport 自建 queue/retry/Resume 语义或选择 Run/Runtime 状态 |
| derived workspace projection | 一个 projection manager + 一个 revision/high-watermark store | Biz 的 typed observation handler、显式只读 rebuild admin use case | 按事件族建立多个可变 Store、projection 发送 owner command |
| HTTP listener、wire DTO、SSE subscriber/replay | `service/server` 私有 transport component | server lifecycle 和 handler | 读取 owner repository、执行 application use case 编排 |
| process-local metrics/cache | server 内各自一个私有组件 | typed immutable telemetry/read result | 作为 Runtime、Run 或 projection authority |

Projection、metrics、cache 和 SSE registry 是可重建或进程内状态，不是正式业务状态机；但每类本地状态仍必须只有
一个写入组件、一个锁/revision 规则和明确生命周期，不能复制为多个互相回填的 cache/store。

### 5.3 目标依赖图

```text
module.go
   |
   v
biz.Biz(Base)                         <- 唯一 EventHub subscriptions/lifecycle
   |
   +-- runtime session                <- 唯一 application session
   +-- execution/activation use case  <- Resume/Restart intent coordination
   +-- owner command gateways         <- concrete typed Send only
   +-- owner read gateways            <- concrete typed query only
   +-- projection manager/store       <- derived state, one writer
   +-- CLI facade
   |
   +---------------- narrow use-case/query/notification ports ----------------+
                                                                            |
                                                                            v
service/server                                                    HTTP/SSE transport only
```

目标依赖方向为 `module.go -> biz -> use case/gateway/projection -> owner contracts`，以及
`biz -> service/server ports`。`service/server` 不得反向导入 `biz/appservice`、`runtimecore`、projection store 或
任何 Owner repository/service。

### 5.4 Server transport 收口

`service/server` 最终只承担：

- HTTP listener、route、request decode/validation、status/error mapping、response encode；
- SSE connection、filter、bounded replay、Last-Event-ID 和 wire DTO；
- transport-local metrics、subscriber registry、bounded run response cache；
- 调用按业务域划分的窄 application port。

必须迁出 server：

- `runtimecore.Session` 保存与 `withSharedSession`；
- owner EventHub command/result 解码；
- workspace Resume/Restart request identity、background submission 和 admission 编排；
- queued/accepted/rejected application projection mutation（仅由 Biz-owned projection component 接收；server 只记录 transport diagnostics）；
- workspace projection manager 的 Start/Rebuild/HandleEvent 生命周期；
- 将 owner telemetry 解释成 application action 的任何逻辑。

`http_types.go` 先在同一 `server` package 内按职责拆分，禁止为每个 endpoint 新建 package：

```text
http_types_workspace.go
http_types_run.go
http_types_governance.go
http_types_configuration.go
http_types_catalog_content.go
server_dependencies.go
server_state.go
server_config.go
server_metrics.go
server_run_cache.go
server_stream_registry.go
server_lifecycle.go
```

拆分后，任何单文件不得同时定义 HTTP DTO、application function/port、server mutable state、metrics/cache 和
lifecycle。HTTP JSON 可以在 transport 边界使用 map 进行最终 wire assembly，但不得把 map DTO 传入 EventHub 或
use case 权威合同。

### 5.5 Execution/activation 单一用例

建立一个 application-owned execution/activation use case，统一 HTTP、CLI 和 Workorchd typed scheduler command：

1. 校验 workspace scope、intent、request identity 和 causation；
2. restart 时编译 typed `RuntimeBootstrap`，resume 时不注入 execution-context patch；
3. 通过 Workspace Owner concrete intent 提交 resume/restart；
4. 校验具体 `WorkspaceExecutionIntentResult` 的 scope/request/kind；
5. 只记录 application-local queued/accepted/rejected projection 与 diagnostics；生产路径由 Biz-owned execution observation 同时写入 projection component 和 transport diagnostics，server 不再作为 projection writer；
6. background execution 只能使用 framework `BackgroundRoutine` 和显式 process context；
7. duplicate request identity 必须得到一致 application admission，不生成第二套 queue/retry authority。

该 use case 不得决定 Workspace/Run/Runtime target state，不得调用 Orchestrator reducer，不得因 HTTP 202、CLI 命令
或 projection event 自动 Resume。请求 ID、clock 和 scheduler 应显式注入，以便并发、duplicate 和超时测试。

### 5.6 Owner command/read gateway 收口

现有 `activationports`、`definitionports`、`governanceports`、`ownerreadports`、`readmodelports` 和 `ownerevent`
中的 EventHub 访问按两类收口：

- **owner command gateway**：按具体 Owner 文件封装 concrete command、lane scope、Send 和 concrete result validation；
- **owner read gateway**：按具体 Owner 文件封装 scoped query 和 concrete result validation。

门禁：

- gateway 可以保存 framework 注入的同一个 Hub 和固定 source，但不能保存 Owner service/repository/adapter；
- use case、HTTP、projection mapper 不接收裸 `event.Hub`，不通过 context 传播 Hub；
- 不建立 `topic + any`、反射、JSON、通用 Envelope 或跨 Owner result type switch registry；
- 每个 owner gateway 直接依赖该 Owner 的 `pkg/events`，不得复制 topic/DTO；
- `Send` 的 nil result、result error、Get error、type mismatch、scope mismatch 必须分别 fail-closed；
- subscription 仍只由 `biz.Biz(Base)` 管理；send-only gateway 不创建 observer。

### 5.7 Projection/read-model 收口

保留一个 `ProjectionManager`、一个 `ProjectionStore` 和一个统一 stream generation/revision/high-watermark 规则。
不得按事件族建立多个可变 Store。纯 mapper 按六个业务族归并：

1. workspace/definition；
2. run/step/checkpoint；
3. decision/approval/governance；
4. LLM/capability/context；
5. evidence/result/artifact/validation；
6. activity/audit/progress。

每个业务族对 `appservice` 只暴露一个窄 projector facade，其余 clone、normalization、field mapping、summary 和
selection helper 全部 package-private。404 个微型文件原则性收敛到约 40-80 个职责文件；若保留更多文件，必须在
实施记录中说明独立合同或测试价值，不能继续保持“一字段/一函数/一文件”的机械粒度。

Projection 事件处理只能更新可重建 Store 或发布 `WorkspaceProjectionChangedData`；rebuild、replay、SSE reconnect、
projection query、cache miss 都不能发送 owner command、调用 `DriveRuntime` 或触发 Resume/Restart。

### 5.8 Appservice package 与 facade 收口

第一批允许归并的低风险薄配对：

| 当前 package | 目标内聚 package | 前置条件 |
|---|---|---|
| `contentcore + contentports` | `content` | 保持 artifact/runtime raw path 安全和 owner read gateway 隔离 |
| `setupdraftcore + setupdraftports` | `setupdraft` | provider call 仍走 LLM owner typed gateway，prompt/parser 不进入 HTTP |
| `qualitycore + qualityports` | `definitionquality` | 只返回 quality finding，不修改 Workspace definition |

暂不机械合并：

- `activationcore + activationports`；
- `definitioncore + definitionports`；
- `governancecore + governanceports`；
- `readmodelcore + readmodelports + readmodelprojection`；
- `workspacecore + workspaceports`。

这些包先通过 facade 和依赖收窄消除 projection/ports 回流，再决定是否合并。出现反向依赖、共享可变 session 或
需要导出内部对象时，应保持同包 helper 或停止拆分，不能引入 callback bridge。

`biz/appservice` 根目录的 55 个生产文件逐一分类：

- 只有转调、无 validation/coordination 的 forwarding wrapper 并回对应 use case；
- HTTP-only mapping 返回 `service/server`；
- CLI-only mapping 归入 CLI facade；
- owner access 归入 command/read gateway；
- projection query 归入 read-model facade；
- 仅保留稳定 application ingress，禁止建立拥有数十业务域方法的 catch-all `ApplicationService`。

`biz/bootstrap` 与 `runtimecore` 只有在 server 已移除 Session 依赖、production/testkit 构造路径已分离后才评估归并。
生产 runtime session 不得演变为 owner module fixture、repository provider 或全局 service registry。

### 5.9 分批工作包

| ID | 工作内容 | 必须保持 | 完成门禁 |
|---|---|---|---|
| WD0 | 冻结 route、JSON、SSE、CLI、projection snapshot、package dependency 和性能基线 | 不改行为、不移动 package | Workorchd 定向测试、HTTP/SSE golden、race baseline 通过 |
| WD1 | 在同一 server package 内拆 `http_types.go`、state、config、metrics、cache、stream registry、lifecycle | route/wire schema/Last-Event-ID 不变 | 单文件职责扫描通过，server contract tests 全绿 |
| WD2 | 建立 workspace/run/governance/config/content 等窄 application dependency ports，先用 adapter 包装现有 appservice | server 不接触 Hub/owner repository | server 不再保存大量 function field；调用行为快照不变 |
| WD3 | 将 scheduler、request identity、background submission、queued/accepted/rejected projection 迁入 execution/activation use case | Workspace Owner 仍决定 lifecycle，Application 不自动 Resume | HTTP/CLI/typed command 共用同一用例；duplicate/timeout 测试通过 |
| WD4 | 将 projection lifecycle/store 从 server 移入 Biz-owned projection component，建立六个 projector facade | 一个 manager/store/revision/high-watermark | replay/rebuild/reconnect 零 owner command；并发/restart 测试通过 |
| WD5 | 按六个事件族归并 readmodelprojection 微型文件并降低 exported declaration | projection DTO 内容、排序、截断和 clone 语义不变 | projection golden/clone/high-watermark 测试通过，公开面清单收窄 |
| WD6 | 归并 `content`、`setupdraft`、`definitionquality` 薄配对并删除 appservice forwarding wrapper | 不引入环、不扩大公共 facade | `go list` 依赖单向；旧 import/path 扫描清零 |
| WD7 | 统一 concrete owner command/read gateway，移除 server/use case 的 Session/裸 Hub 传播；再评估 bootstrap/runtimecore 收口 | Hub subscription 仅 Biz；owner contracts 仍归各 Owner | server 不导入 appservice/runtimecore/owner event implementation；typed failure matrix 通过 |
| WD8 | 更新 Workorchd readme、repository structure、boundary script、复杂度盘点并完成 deployed regression | 22 运行单元、HTTP/CLI/SSE 兼容、Runtime 主链不变 | 全量门禁、race、fresh-data live run 与复杂度复盘通过 |

WD0-WD8 按顺序实施并单独提交。WD1/WD2 是行为等价结构迁移，WD3/WD4 才改变职责落点，WD5/WD6 做机械归并，
WD7 最后删除兼容入口；禁止在同一提交同时修改 HTTP schema、projection 算法、owner contract 和 runtime 行为。

当前 working-tree 的代码状态：WD0-WD8 均已完成代码与自动化收口。WD5 将公开面从 230 个基线 declaration
收窄到 8 个；WD6 删除 projection/setupdraft/cache-authority 纯转调 wrapper，保留的根入口均承担 session/backend
边界或稳定 application ingress；WD7 删除生产 Session/裸 Hub/root appservice、server-local projection lifecycle、
双 wiring 与兼容 execution projection。WD8 已记录指标并把这些边界写入 Go 结构测试和 repository shell gate。
这只关闭 WIC code/automated queue；deployed HTTP/SSE/CLI regression 与 SO-MC-10 fresh-data live proof 仍保持 open。

### 5.10 测试与性能门禁

Workorchd 每批至少覆盖：

- HTTP route/method/status/error/JSON field compatibility；
- SSE topic/filter/replay/Last-Event-ID/disconnect/backpressure；
- CLI typed command/result 和 `internal/pkg/workorchport.Backend` compatibility；
- execution use case 的 resume/restart、duplicate request、queue failure、context cancellation；
- owner gateway 的 nil/error/type/scope/stale result；
- projection mapper 的 deterministic output、deep clone、stable ordering、bounded payload；
- projection manager 的 rebuild coalescing、per-workspace concurrency、stream generation、revision floor、high-watermark；
- runtime session borrow/close、server shutdown、subscriber/cache cleanup；
- `go test -race` 下 projection store、SSE registry、run cache、metrics、session 无 data race。

性能必须保持或改善：

- HTTP read 不因拆包增加 owner query 次数；
- projection query 不触发全量 rebuild；
- 同 workspace rebuild 继续 coalesce，不形成重复后台任务；
- SSE replay/cache 保持有界；
- mapper 归并不增加不必要的深拷贝或反复序列化；
- WD0 记录的关键 benchmark/请求次数/分配量发生退化时停止归并并定位原因。

### 5.11 结构与行为验收

- framework 运行单元仍为 1 Initiator、4 Modules、17 Blocks；Workorchd 仍只有一个 Application Module；
- `module.go` 只做注册和 Setup/Run/Teardown 委托；
- `biz.Biz(Base)` 是唯一订阅/observer owner，Stop 幂等解除订阅并释放 session/projection/server；
- server 不导入 `runtimecore`、projection store、其他 Owner `pkg/events` 或任何 owner implementation；
- server 不持有 EventHub/Session，不执行 activation、owner result admission，也不拥有 projection lifecycle/store；
  显式 rebuild 只能调用 Biz 注入的 derived-write port；
- execution/activation use case 是 HTTP、CLI 和 typed scheduler command 的唯一编排入口；
- owner gateway 使用 concrete command/result，无 map/any/Envelope/reflect 合同；
- projection manager/store 唯一，所有 mapper 纯函数化，replay/reconnect 零副作用；
- `http_types.go` 的混合职责消失，appservice forwarding wrapper 和无环薄 package 显著减少；
- HTTP JSON/SSE、CLI/TUI backend、projection snapshot 和 process shutdown 保持兼容；
- Workorchd 不选择 SM-1/SM-2/SM-3/SM-4 transition，不因 projection 或 transport failure 改写 Owner state；
- 复杂度复盘必须重新记录生产文件/LOC、package 数、exported declaration、最大文件和跨包依赖边；只减少文件行数但
  增加 facade/topic/反向依赖不视为完成。

### 5.12 明确禁止的 Workorchd“简化”

- 把 HTTP、SSE、CLI 或 projection 拆成新的 framework Module/Block；
- 建立综合 `ApplicationService`、跨 Owner registry、topic map 或 callback bus；
- 让 server 直接持有 EventHub、runtime session、owner gateway 或 repository；
- 为六个 projection 事件族分别创建可变 Store/revision，破坏统一 snapshot/high-watermark；
- projection rebuild/replay/cache miss 发送 owner command 或自动 Resume；
- 为消除 package 而把 owner DTO 复制成 application DTO，再用 JSON/reflect 互转；
- 把 HTTP DTO 放入 EventHub contract，或把 raw owner event 直接作为 SSE wire body；
- 机械合并存在反向依赖的 `core + ports`，再用 interface callback 掩盖循环；
- 将 Hub 放入 context 继续向 use case/server 扩散；
- 用更大的 cache、queue 或 goroutine 数掩盖重复 projection/query 工作。

## 6. `cwi-027` 专项闭环

该专项必须按以下顺序实现，不允许只改 LLM prompt 或只改 File policy。

### C027-1：File owner target acquisition

File owner 对 `coverage-map.json` 返回带 scope 的 canonical target selection：

- owner module：`blocks/file`；
- selector owner：`blocks/file`；
- selector subject：`workspace_deliverable`；
- disposition：`create`、`selected`、`ambiguous` 或 `missing`；
- prospective target 或 exact target；
- candidate target refs（有界）；
- target revision/snapshot；
- observed revision。

如果 selector 不能唯一确定目标，必须返回 typed prerequisite/ambiguous fact，禁止让 LLM 猜路径。

### C027-2：SM-2 编译并封存 binding

Orchestrator 将 File observation 与 Definition seed 中的 `coverage_map` declaration 合并，封存唯一 binding。
封存前必须验证：

- `RequireDirectEvidence` 与 target policy 相容；
- `file/write` 仅对应 prospective create；
- existing target 只能对应 revision-bound replacement；
- `TargetRefs` 或 deterministic owner selector 至少有一项可由 File 验证；
- binding hash、catalog snapshot、owner policy revision 和 authority input 已持久化。

若不能满足，SM-2 只能形成 `owner_prerequisite_required`、`semantic_contract_failed` 或
`target_grounding_unresolved` typed terminal，不能创建 operations request。

### C027-3：operations candidate admission

候选必须匹配 sealed binding：

- operation kind/variant 正确；
- target refs 属于 sealed exact/owner-scope/prospective policy；
- 不得从 `coverageWorkItemIDs` 自报 credit；
- 不得添加新的 check、metric、artifact、binding 或 evidence requirement；
- `file/write` 对现有目标必须在纯 evaluator 阶段拒绝；
- candidate rejection 只能通过 SM-3 private event 形成 bounded correction 或 final typed failure。

### C027-4：dispatch、receipt、evidence

只有 C027-3 admitted 后才允许：

1. Orchestrator seal dispatch；
2. File claim `ExecutionKey + Fence`；
3. File 执行并持久化 owner receipt；
4. Orchestrator 校验 receipt 与 binding/target/revision/effect scope；
5. SM-2 提交 evidence CAS 并推进 CWI；
6. SM-4 继续下一个 eligible CWI。

任一步骤失败都必须证明：无未声明 workspace effect、无重复 receipt、无 evidence credit、无 focus 越权推进。

## 7. 完整准入 evaluator 必须检查的维度

一个纯 evaluator 在创建 sealed dispatch 前一次性检查：

| 维度 | 必须满足 |
|---|---|
| scope | workspace/run/definition/authority identity 全部匹配 |
| CWI | 当前只存在一个 eligible CWI closure |
| dependency | 所有 prerequisite 已 settled/satisfied/fresh |
| owner | operation kind 属于 canonical owner |
| kind/variant | 与 sealed declaration 和 binding 完全一致 |
| target | exact、owner-scope 或 prospective policy 可验证 |
| catalog | semantic/operation feasibility 使用同一 owner policy revision |
| proof | evidence/check/effect/metric/artifact requirements 来自 declaration |
| binding | 一个 proof-bearing operation 对应一个 canonical binding |
| graph | operation dependency 无环，辅助操作不得偷带 proof binding |
| retirement | active dispatch/effect/evidence 不得被隐式退休 |
| fence | dispatch/operation/binding/execution key/fence 为同一 identity |
| lineage | successor 必须有明确 reopen/retirement causation |
| request | active request session 与当前 manifest/binding set 一致 |

Evaluator 只能返回 typed outcome：`admitted`、`correctable_once`、`prerequisite_blocked`、`owner_stale`、
`owner_infeasible`、`duplicate_noop`、`stale_conflict` 或 `terminal_contract_failure`。它不得写状态或选择下一动作。

## 8. 测试与验证矩阵

### 8.1 定向 Go 测试

```bash
go test ./internal/pkg/workorchruntimecontract/... -count=1
go test ./internal/modules/blocks/run/... -count=1
go test ./internal/modules/blocks/file/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/blocks/result/... -count=1
go test ./internal/modules/blocks/decision/... -count=1
go test ./internal/modules/blocks/approval/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
```

### 8.2 必测不变量

- owner transition：每个 owner 的状态只能由自身 internal event 改变；
- transition：合法/非法边、duplicate、stale、CAS mismatch；
- restart：aggregate load、fresh activation、no old CWI credit；
- candidate：incomplete document 不创建 attempt/dispatch/effect；
- dispatch：ExecutionKey/Fence/receipt replay；
- target：create、existing replacement、ambiguous、missing、stale revision；
- evidence：binding/target/revision/effect/check 不匹配不得给 CWI credit；
- budget：request recovery、candidate attempt、prerequisite refresh、CAS recovery 互不借用；
- delivery：Result→Workspace→Run 顺序和幂等 acknowledgement；
- projection：投影失败不改写 Runtime authority；
- progress：相同 progress signature 不生成新 authority/request。
- application：HTTP/SSE/projection/CLI 不选择 owner transition，projection replay/reconnect 零 owner command。

### 8.3 仓库门禁

```bash
make fmt-check
make vet
make test
make test-race
make verify-boundary-contract
make build
```

`make verify-ci-mvp-file` 只作为 mock/managed regression；不能替代真实 provider 的 SO-MC-10 live proof。

## 9. 部署与 live 验收顺序

代码门禁通过后：

1. 清理 `.workorch-data` 与测试 workspace 数据，不迁移旧 Runtime schema/aggregate。
2. 部署当前 commit，记录 binary、commit、config revision、capability revision。
3. 创建全新 workspace/run，禁止手工 Resume/Restart 改变验证路径。
4. 读取 Run owner 的 SM-1 记录和 Orchestrator Runtime aggregate，确认 scope/revision 起始值。
5. 观察 `cwi-026`：candidate → dispatch → File receipt → evidence → assessment。
6. 观察资源变化后的 reopen：旧 binding 退休，新 authority input/manifest/binding 产生且原因明确。
7. 观察 `cwi-027`：必须先有 owner-grounded target，再选择合法 create/replacement operation。
8. 确认没有重复 `file/write`、无 revision 无限增长、无等价 AuthoritySnapshot。
9. 确认 focus 继续推进到后续 eligible CWI，而不是只以 fail-closed 作为“成功”。
10. 确认终态只按 Result→Workspace→Run delivery，并有三方 acknowledgement。

### Live 通过条件

必须同时满足：

- 至少一个真实 candidate、sealed dispatch、capability receipt、accepted evidence；
- reopened existing target 使用 revision-bound replacement 并继续后续 CWI；
- 没有 duplicate side effect、equivalent authority loop 或自动 Resume；
- Run、Workspace、Result、Orchestrator 各自只发生 owner-local transition；
- Run 最终完成或形成有明确原因的 typed terminal，不能只靠保护性失败结束。

## 10. 实施记录模板

每个工作包完成后在提交说明或验证记录中填写：

```text
Work package: Wn / On / WDn / C027-n
Commit:
Changed owners:
Changed state machines: SM-1/SM-2/SM-3/SM-4/none
Changed application-local components:
New internal event(s):
Cross-owner typed contract(s):
Forbidden path removed:
Focused tests:
Boundary scan:
Restart/stale/duplicate evidence:
Deployment/live evidence:
Remaining risk:
```

以下记录按时间追加；较早记录中的 `Remaining risk` 是当时切片状态，当前遗留项以第 1、5 节和最后一条实施记录为准。

未填写 owner、state machine、CAS、测试和 live evidence 的修改，不视为收口完成。

### 10.1 当前实施记录（2026-08-14）

```text
Work package: WD1 + WD2-A + WD3-A + WD4-A + WD4-B + WD4-C + WD4-D + WD4-E + WD4-F
Commit: working tree (未提交)
Changed owners: Workorchd Application Module 内部 transport/application boundary；未改变 Workspace/Run/Runtime owner
Changed state machines: none（仅结构与依赖注入边界迁移）
Changed application-local components: server DTO/dependencies/state/metrics/stream/lifecycle；七组 application operations；Biz runtime-session adapter；`biz/execution.UseCase`（异步 HTTP admission 与同步 typed scheduler admission）；`biz/projection.Component`（单 manager/store、background lifecycle、typed observation、workspace/governance/scheduler/execution derived writer）；Biz execution observation fan-out；`WorkspaceProjection` reader、observer、lifecycle 与 `WorkspaceProjectionWriter` 四端口分离
New internal event(s): none
Cross-owner typed contract(s): none（复用既有 concrete contracts）
Forbidden path removed: production Biz → server 的 runtime Session 直传；server 生产构造路径改用 typed ApplicationPorts；HTTP handler 不再直接调用 server-local scheduler queue；HTTP 与 typed EventHub scheduler 共用 Biz-owned execution admission；production projection 的构造/Start/Close/background ownership 与 EventStream/RuntimeDrive observation 已移入 Biz；workspace/definition/governance/scheduler/execution 派生 projection 更新与 full rebuild 通过 Biz-owned `WorkspaceProjectionWriter`；HTTP 持有的 `WorkspaceProjection` 读端口不再暴露任何 write/rebuild 方法；execution use case 或 Biz wiring 缺失时生产路径在 listener 启动前 fail-closed，不再退回第二套 server-local scheduler authority；execution 的 transport diagnostics 与 projection writer 已物理分离；application callback alias 已改为唯一 named contract
Focused tests: `go test ./internal/modules/application/workorchd/biz/execution -count=1`; `go test ./internal/modules/application/workorchd/biz/projection -count=1`（含 `TestExecutionObservationWritesOnlyBizOwnedProjection`）；`go test ./internal/modules/application/workorchd/service/server -run 'TestProductionProjectionHelpersPreferBizOwnedWriter|TestProductionServiceRunFailsClosedWhenApplicationAdmissionIsMissing|TestServiceRunRequiresBackgroundRoutine' -count=1`；server compile-only test；`go test ./internal/modules/application/workorchd/biz -count=1`; `go test ./internal/modules/application/workorchd/... -count=1`; execution/projection/Biz/server race tests
Boundary scan: `git diff --check` 通过；`make verify-boundary-contract` 通过
Restart/stale/duplicate evidence: execution use case 对 resume/restart 只做 intent/identity/scope 校验；owner rejection 不自动 retry；同一 workspace/request 的 HTTP admission 在 1024 条有界进程窗口内 coalesce，异义复用 fail-closed；持久化 duplicate authority 仍由 Workspace/Run/Orchestrator owner 处理
Deployment/live evidence: 未执行；需 WD3/WD4/WD7/WD8 完成后重新部署核对
Remaining risk: server 仍保留显式 legacy Session/default adapter 和 compatibility projection constructor 供 standalone/test 构造，但 typed production wiring 已在 Run 前校验 admission/scheduler/writer，legacy manager 也被显式注入为 writer；六类 projector facade、CLI 显式 execution port、WD5-WD7 package/gateway 收口尚未完成
```

### 10.2 O4/O5 owner feasibility 与 binding contradiction 分流收口（2026-08-14）

```text
Work package: O4 + O5 correctness slice（未宣告 O4/O5 整包完成）
Commit: working tree（未提交）
Changed owners: File capability owner 只补充自身 feasibility factual classification；Orchestrator 仍是 SM-2/SM-3 唯一 reducer；Run owner/persistence 未变
Changed state machines: SM-3 candidate feasibility invalidation；SM-2 exact binding contradiction correlation/retirement；SM-1、SM-4 未变
New internal event(s): 无新增 kind；既有 owner_feasibility_observed 与 semantic_binding_contradiction_observed 改为由显式 typed constraint impact 互斥分流
Cross-owner typed contract(s): RuntimeOperationFeasibilityObservation/RuntimeOwnerFeasibilityObservation 增加 owner-neutral ConstraintImpact；唯一当前值 sealed_target_authorization_contradicted；OwnerPolicy 保持纯来源 identity
Forbidden path removed: 以 OwnerPolicy.PolicyRevision 非空推断 binding contradiction；以最新 global semantic manifest 否定历史 generation 中仍被 current CWI binding 引用的 manifest；以 owner code/message 在 Orchestrator 核心选择 transition；operation_replay_proof_unavailable 被误报为 runtime_contract_defect
Focused tests: workorchruntimecontract、File Block 全包、Orchestrator 全包及 go test ./... 全量通过；包含 OwnerPolicy+observation_unknown 的 run-003 回归、wrong internal-event load/CAS 前拒绝、历史 semantic generation 精确关联、unknown generation 拒绝、duplicate/restart 幂等
Boundary scan: git diff --check、make fmt-check、make vet 与 make verify-boundary-contract 通过
Restart/stale/duplicate evidence: candidate-local owner observation 只形成 invalidated attempt 和 bounded equivalent-authority no-progress；显式 contradiction 仅在 persisted attempt/operation/manifest/catalog/authority/focus/binding/reconciliation/targets/revision 全匹配时退休一次；duplicate 与 restart 不增加 revision/lineage
Deployment/live evidence: 未执行；需提交、重新部署并以 fresh data 验证 cwi-026 replacement 后继续 cwi-027
Remaining risk: 当前仅关闭 run-003 暴露的错误分流与历史 generation 关联缺口；O4 candidateadmission package/fingerprint 全量提取、O5 grounding/proof package 全量提取、O0-O8 其余结构项及 fresh-data live closure 仍保持 open
```

### 10.3 O0 Runtime mutation owner 结构基线（2026-08-15）

```text
Work package: O0
Commit: working tree（未提交）
Changed owners: 无；仅冻结 Orchestrator owner 内部既有 mutation boundary
Changed state machines: SM-2/SM-3/SM-4 结构边界；未修改状态、迁移、预算或恢复语义；SM-1 未变
Changed owner-local components: RuntimeAggregate；RuntimeController；RuntimeTransitionCoordinator
New internal event(s): none
Cross-owner typed contract(s): none
Forbidden path removed: none（新增静态回归，阻止后续拆包引入第二 aggregate、第二 mutation dispatcher、额外 Controller 导出入口或绕过 coordinator 的 repository Save）
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/runtime -run 'TestRuntimeMutationOwnerStructureIsUnique|TestRuntimeInternalFactsHaveCompleteOwnerPrivateClassification|TestRuntimeTransitionCoordinator' -count=1`；`go test ./internal/modules/kernel/orchestrator/... -count=1`
Boundary scan: `TestRuntimeMutationOwnerStructureIsUnique` 从 production AST 核对一个 RuntimeAggregate、一个 ApplyInternalEvent、一个 RuntimeTransitionCoordinator、Controller 恰好三个导出方法及唯一 Save 调用点
Restart/stale/duplicate evidence: O0 不改变运行行为；既有 Orchestrator restart/stale/duplicate 回归全绿
Deployment/live evidence: 不适用（纯 owner-local 结构门禁，无部署行为变化）
Remaining risk: O0 自动化结构基线已完成但尚未形成独立提交；O1-O8 仍 open，O4/O5 仅完成 10.2 所列 correctness slice；fresh-data live closure 未执行
```

### 10.4 O1 Runtime internal event concrete payload 收口（2026-08-15）

```text
Work package: O1
Commit: working tree（未提交）
Changed owners: 无；Orchestrator 仍是 SM-2/SM-3/SM-4 唯一 owner，internal event 仍不跨 EventHub
Changed state machines: SM-2/SM-3/SM-4 的 owner-private 输入 envelope；未修改状态、合法/禁止迁移、预算、recovery、CAS 或 side-effect 语义；SM-1 未变
Changed owner-local components: RuntimeInternalEventScope；RuntimeInternalActivationPayload；RuntimeInternalAuthorityPayload；RuntimeInternalTransactionPayload；RuntimeInternalDeliveryPayload；唯一 ApplyInternalEvent dispatcher
New internal event(s): none；既有 kind 按 activation/authority/transaction/delivery source 精确绑定 concrete payload family
Cross-owner typed contract(s): none
Forbidden path removed: flat RuntimeInternalEvent 多用途字段联合体；payload 自带/复制 workspace-run identity；wrong/missing/multiple payload 穿透到 aggregate load/CAS
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/runtime -run 'TestRuntimeInternalFactsHaveCompleteOwnerPrivateClassification|TestRuntimeInternalEventEnvelope|TestRuntimeMutationOwnerStructureIsUnique' -count=1`；`go test ./internal/modules/kernel/orchestrator/biz/runtime -count=1`；`go test ./internal/modules/kernel/orchestrator/biz -count=1`
Boundary scan: 原有 production/test 223 个 RuntimeInternalEvent 构造点迁移为 Scope + 单 concrete payload；反射结构测试固定 envelope 仅六个字段；source AST 同时穷举 kind、payload class、metadata 与 dispatcher
Restart/stale/duplicate evidence: aggregate/reducer/ledger 未变；既有 Runtime 与 Orchestrator Biz 全包中的 restart、stale、duplicate、CAS、budget/guard 行为快照全绿
Deployment/live evidence: 不适用（owner-private 非持久化 envelope 的行为等价结构迁移）；全局 fresh-data live closure 仍需 O2-O8 后执行
Remaining risk: O1 自动化与代码迁移已完成但尚未形成独立提交；O2-O8 仍 open，O4/O5 仍仅完成 10.2 correctness slice
```

### 10.5 O2 私有组件与 mutation kernel 收口（2026-08-15）

```text
Work package: O2
Commit: working tree（未提交）
Changed owners: 无；Orchestrator 仍是 SM-2/SM-3/SM-4 唯一 owner，22 个 framework 运行单元不变
Changed state machines: SM-2 authority/reconciliation/governance/evidence、SM-3 semantic/request/candidate/receipt、SM-4 drive lease/recovery/budget/reconciliation/delivery；未改变状态语义、预算上限、side effect 或 SM-1
Changed owner-local components: runtimeMutationKernel；authorityComponent；planningTransactionComponent；candidateDispatchComponent；driveDeliveryComponent。semantic planning/request ledger、candidate admission/seal、authority facts/governance、operation receipt 与 drive/delivery handler 已按职责改为组件 receiver；Controller 保留的 package-private adapter 仅为既有测试 seam
New internal event(s): none；既有 dispatcher 的 SM-2、SM-3、SM-4 分支直接路由到对应私有组件
Cross-owner typed contract(s): none
Forbidden path removed: RuntimeController/DispatchCoordinator 各自持有 repository/coordinator 的隐式双依赖；私有组件不再保存 sibling、EventHub、aggregate 或第二 CAS 入口
Focused tests: `TestRuntimePrivateComponentsShareOneMutationKernel`、`TestRuntimePrivateComponentsUseTheControllerKernelInstance`、`TestRuntimeMutationOwnerStructureIsUnique`；结构测试固定五组件各自只嵌入同一 kernel、无 sibling/dispatcher 访问；drive lease、recovery budget、delivery acknowledgement、operation receipt、semantic planning/request、candidate admission 及 Orchestrator 全包回归
Boundary scan: runtime package `gofmt`、结构 AST 检查、`git diff --check`、`go test ./internal/modules/kernel/orchestrator/... -count=1`、全仓 `go test ./... -count=1`、`make fmt-check`、`make vet` 与 `make verify-boundary-contract` 均已通过
Restart/stale/duplicate evidence: 本切片保持原有 drive lease generation fence、budget idempotency、receipt fence/idempotency；新增组件只共享同一 kernel 指针，不改变 revision 或 replay identity
Deployment/live evidence: 不适用；未触发 Resume/Restart 或部署，fresh-data live closure 仍需 O3-O8 后执行
Remaining risk: O2 组件边界与 dispatcher 路由已完成；无状态 `semanticplan`/`candidateadmission`/`grounding`/`proof`/`governancecheck`/`requestcontent` focused package、executor/prerequisite 历史 helper 清理和 controller 窄化仍由 O3-O7 处理；O4/O5 correctness slice 之外的全量提取及 fresh-data live closure 保持 open
```

### 10.6 O3 semanticplan/requestcontent 与 existing-target 重规划输入收口（2026-08-15）

```text
Work package: O3
Commit: working tree（未提交）
Changed owners: Orchestrator owner 内部无状态规则与 LLM request adapter；File owner 的既有 create-only/replacement catalog 作为事实来源，未把 action 选择移入 File 或 LLM
Changed state machines: SM-3 semantic/request transaction、SM-4 drive input；未改变 SM-1 lifecycle、SM-2 authority transition、candidate attempt 上限、CAS 或副作用顺序
Changed owner-local components: `biz/internal/semanticplan`（manifest identity、batch/document lookup、existing-target typed projection）；`biz/internal/requestcontent`（bounded volatile request context、raw content hash/tail）；`runtime_semantic_drive.go`（provider I/O 前 detached-context guard）；LLM semantic provider 的 provider-safe C4 existing-target projection
New internal event(s): none；继续使用既有 semantic request/correction internal events
Cross-owner typed contract(s): none（复用已封存的 `RuntimeSemanticPlanManifest`、owner catalog、`RuntimeOriginalDiagnostic` 与 `RuntimeSemanticCorrectionDirective`；focused package 类型不进入 EventHub）
Forbidden path removed: semantic planning 中重复的 manifest identity/batch lookup helper；provider correction 只携带 raw owner message 或只依据 operation kind 猜测 replacement；request-content hash helper 与 repository carrier 混在同一层；focused package 访问 Runtime aggregate、repository、EventHub 或触发 transition
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/internal/... -count=1`；`TestExistingTargetContextCarriesRevisionAndOwnerAlternatives`；`TestSemanticPlanRequestContentCarriesLatestCatalogFactsWithoutRawMessage`；`TestRuntimeSemanticExistingTargetCorrectionCarriesOwnerRevisionAndReplacementFact`；`TestFocusedRulePackagesRemainStateless`；LLM semantic provider、Orchestrator runtime、File Block、`workorchruntimecontract` 全包回归
Boundary scan: focused package AST 门禁禁止 `biz/runtime`、repository、EventHub/task 依赖及 Save/Update/Create/Publish/Subscribe/ApplyInternalEvent 调用；`git diff --check` 通过；O3 focused packages 不声明 aggregate、controller、CAS 或 retry authority
Restart/stale/duplicate evidence: request content 仅由 detached manifest/diagnostic/correction 计算，稳定 C0/C1/C2 cache group 在 attempt-0/attempt-1 保持一致；attempt-1 仍严格绑定 source rejection hash、manifest、seed、stable prefix；无新增 provider call、attempt、revision 或 owner side effect
Deployment/live evidence: 未部署、未触发 Resume/Restart；历史旧 Run 仅作 counterexample，不作为当前 baseline 证明；需 O4-O8 完成后以清理数据 fresh process 验证 cwi-026 replacement -> cwi-027 -> terminal delivery
Remaining risk: `candidateadmission`/`grounding`/`proof`/`governancecheck` 尚未全量提取；controller/prerequisite/executor 混合 helper 仍由 O4-O7 收口；O3 只完成代码与自动化门禁，SO-MC-10 fresh-data live proof 仍 open
```

### 10.7 O4 candidateadmission 重放指纹切片（2026-08-15）

```text
Work package: O4 replay/fingerprint slice（未宣告 O4 整包完成）
Commit: working tree（未提交）
Changed owners: Orchestrator 内部 candidate admission；File/其他 capability owner 不变，仍只发布 feasibility/catalog fact
Changed state machines: SM-3 candidate admission/replay fence；未改变 SM-2 authority、SM-1 lifecycle、SM-4 delivery、attempt 上限或 CAS 顺序
Changed owner-local components: `biz/internal/candidateadmission`（state-change/validation operation 提取、rejected-shape、no-progress、complete-candidate semantic replay 的纯比较）；`runtime/candidate_admission.go` 仅保留 aggregate 事实投影、attempt/authority 过滤及 owner-local rejection seal
New internal event(s): none；纯比较包不接触 private event 或 EventHub
Cross-owner typed contract(s): none（复用 `RuntimeCandidate`、`RuntimeRejectedOperationShape` 与稳定 fingerprint contract）
Forbidden path removed: candidate replay/no-progress/complete semantic fingerprint 规则不再在 Runtime admission handler 内重复实现；focused package 不持有 aggregate、ledger、repository、CAS、retry 或 next-action
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/internal/candidateadmission -count=1`；`go test ./internal/modules/kernel/orchestrator/biz/runtime -run 'TestEvaluateCandidateAdmissionClassifiesFullAndPartialReplay|TestEvaluateCandidateAdmissionRejectsAcceptedNoProgressShape|TestAdmitCandidateProposalAtomicallyClosesReplayAttemptAndExhaustsSecondAttempt|TestAdmitCandidateProposalExhaustsSemanticReplayWithoutOperationIdentity' -count=1`；`TestFocusedRulePackagesRemainStateless`
Boundary scan: focused package AST 门禁纳入 `candidateadmission`；仅依赖 typed runtime contract，不导入 `biz/runtime`、repository、EventHub/task，不调用 Save/Update/Create/Publish/Subscribe/ApplyInternalEvent；`gofmt` 与 `git diff --check` 通过
Restart/stale/duplicate evidence: replay 比较仍由当前 aggregate 的 focus/authority/attempt 过滤后调用；既有 duplicate proposal、terminal duplicate、operation-ID 改写和参数顺序改写回归保持通过，第二次副作用前拒绝且不增加 revision/CAS
Deployment/live evidence: 未部署、未触发 Resume/Restart；需完成 O4 其余 feasibility/admission 统一、O5-O8 后，以 fresh data 验证 existing-target replacement 继续 cwi-027
Remaining risk: candidateadmission 尚未承载全部 manifest feasibility、grounding、repair-closure 和 proof/binding 检查；O4 remainder、O5-O8 与 SO-MC-10 fresh-data live proof 仍 open
```

### 10.8 O5 grounding/proof 纯比较切片（2026-08-15）

```text
Work package: O5 grounding/proof comparison slice（未宣告 O5 整包完成）
Commit: working tree（未提交）
Changed owners: Orchestrator 内部 prerequisite/evidence comparison；owner catalog、receipt 与 Run persistence owner 不变
Changed state machines: SM-2 freshness/proof attribution、SM-3 prerequisite admission input；未改变 refresh/reopen/credit/dispatch transition
Changed owner-local components: `biz/internal/grounding`（target membership、owner catalog policy scope、current catalog/observation 与 manifest freshness 比较）；`biz/internal/proof`（weak evidence、binding reference、focus operation、validation proof 与 equivalent-lineage resolution 比较）；Runtime wrappers 仅负责 aggregate/focus/lineage 事实投影和后续 mutation
New internal event(s): none；focused packages 不创建 invalidation、evidence 或 reconciliation event
Cross-owner typed contract(s): none（复用 Runtime typed catalog、observation、EvidenceFact、CoverageEvidenceBinding 与 proof-lineage contract）
Forbidden path removed: catalog freshness、target scope、evidence quality 和 proof predicates 不再在 Runtime handler 中各自复制；focused package 不导入 Runtime aggregate/repository/EventHub，不拥有 CWI credit 或 refresh budget
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/internal/grounding -count=1`；`go test ./internal/modules/kernel/orchestrator/biz/internal/proof -count=1`；Orchestrator Runtime 全包（含历史 semantic generation、stale observation、coverage lineage、duplicate receipt）；`TestFocusedRulePackagesRemainStateless`
Boundary scan: focused package AST 门禁纳入 `grounding`、`proof`；仅依赖 common/runtime contract，不调用 Save/Update/Create/Publish/Subscribe/ApplyInternalEvent；`gofmt` 与 `git diff --check` 通过
Restart/stale/duplicate evidence: freshness 比较仍要求 owner/prerequisite/snapshot/content hash 同时匹配；proof 比较仍要求当前 binding 与等价 lineage resolution，旧 generation 不重新获得 credit；既有 stale/duplicate/restart 回归保持通过
Deployment/live evidence: 未部署、未触发 Resume/Restart；需 O4 remainder、O6-O8 完成后，以 fresh data 验证 replacement 与 cwi-027 closure
Remaining risk: O5 尚未提取全部 reconciliation/repair-surface 纯判断；O4 feasibility/admission remainder、O6 governance、O7 controller/helper 清理及 SO-MC-10 fresh-data live proof 仍 open
```

### 10.9 O6 governancecheck 身份/结果校验切片（2026-08-15）

```text
Work package: O6 governancecheck validation slice（未宣告 O6 整包完成）
Commit: working tree（未提交）
Changed owners: Orchestrator authorityComponent 的治理 request/resolution 输入校验；Approval/Decision/外部 ticket owner 不变
Changed state machines: SM-3 governance dispatch transaction 的 typed identity fence；未改变 SM-2 authority、SM-4 delivery、ticket 创建或 resume dispatch 语义
Changed owner-local components: `biz/internal/governancecheck`（governed dispatch identity、dependency identity、ticket-bound outcome predicate）；`runtime/governance.go` 继续负责 aggregate load、binding/evidence mutation、resume dispatch 与 CAS
New internal event(s): none；治理 adapter 仍只接收既有 typed request/resolution
Cross-owner typed contract(s): none（复用既有 `RuntimeGovernanceRequestRecord`、`RuntimeGovernanceResolution` 与 binding enums）
Forbidden path removed: request/resolution 与 active governed dispatch 的身份、依赖和 outcome 判定不再散落于 mutation handler；focused package 不创建 ticket、不写 SM-2/SM-4、不调用 repository/EventHub
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/internal/governancecheck -count=1`；Orchestrator governance 全包与 Runtime 全包；`TestFocusedRulePackagesRemainStateless`；全仓 `go test ./... -count=1`；Runtime race test
Boundary scan: focused package AST 门禁纳入 `governancecheck`；仅依赖 typed contract，不调用 Save/Update/Create/Publish/Subscribe/ApplyInternalEvent；`git diff --check`、`make fmt-check`、`make vet` 与 `make verify-boundary-contract` 通过
Restart/stale/duplicate evidence: stale authority、duplicate resolution、unbound ticket 与 suspended/ticket-bound binding 的既有回归保持通过；outcome predicate 在 mutation 前 fail-closed
Deployment/live evidence: 未部署、未触发 Resume/Restart；需 O6 remainder、O7-O8 完成后 fresh-data live proof
Remaining risk: O6 尚未收窄全部 governance adapter/receipt mapping；O4/O5 remainder、O7 controller/helper 清理及 SO-MC-10 fresh-data live proof 仍 open
```

### 10.10 O4 单一 CandidateEvaluator 结构收口（2026-08-15）

```text
Work package: O4 structural evaluator remainder（自动化 code-closed；不代表 live-closed）
Commit: working tree（未提交）
Changed owners: 无；Orchestrator 仍是 SM-3 candidate transaction 唯一 owner，File/其他 capability owner 仍只返回 catalog/feasibility/receipt factual observation
Changed state machines: SM-3 candidate document admission；未改变 SM-1 lifecycle、SM-2 authority/focus、SM-4 drive/delivery、attempt 0/1 上限、correction policy、dispatch seal 或 CAS 顺序
Changed owner-local components: `biz/internal/candidateadmission.Evaluate` 成为 manifest identity、wire/field、kind/variant/parameter、target/binding/effect/dependency、repair closure、validation-only gap、grounding 与 replay 的唯一纯首错 evaluator；`biz/internal/grounding` 承载 current target selection、creation/scope recognition、repair-target precedence 与 exact membership；`runtime/candidate_admission.go` 只保留 aggregate authority/attempt/lineage 投影、一次 evaluator 调用及 owner-local rejection seal
New internal event(s): none；未新增 EventHub topic、Runtime state、attempt、budget 或 mutation entry
Cross-owner typed contract(s): none；复用 `RuntimeCandidateContractManifest`、`RuntimeCandidateFailure`、`RuntimeRejectedOperationShape`、target selection 与 repair closure concrete contract
Forbidden path removed: Runtime admission wrapper 内并列调用 manifest validator、activation validator、repair/grounding/replay helper 并自行选择 first failure；current-focus target selection、creation/scope 与 repair-target precedence 在 reducer/prerequisite/admission 各自复制；focused evaluator 探测 owner、seal correction/dispatch 或决定 retry/terminal
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/internal/... -count=1`；`TestEvaluateUsesOneStableStructuralThenReplayOrder`；`TestEvaluateRepairClosurePrecedesReplay`；`TestCurrentFocusTargetSelectionRequiresSatisfiedFocusPrerequisite`；`TestOwnerGroundedCreationAndWorkspaceScopeRemainBounded`；Runtime candidate admission 全包及 `TestOwnerBindingContradictionRetiresExactBindingWithoutEffectAndIsRestartIdempotent`
Boundary scan: `TestCandidateAdmissionUsesOnePureEvaluatorEntry` 固定 production 仅一个 `candidateadmission.Evaluate` 调用且 Runtime wrapper 不保留第二 manifest/activation validator；`TestFocusedRulePackagesRemainStateless` 继续禁止 focused package 导入 Runtime owner/repository/EventHub/task 或调用 Save/Update/Create/Publish/Subscribe/ApplyInternalEvent
Combined gates: `go test ./... -count=1`、`go test -race ./internal/modules/kernel/orchestrator/biz/runtime -count=1`、`make fmt-check`、`make vet`、`make verify-boundary-contract` 与 `git diff --check` 全部通过
Restart/stale/duplicate evidence: prior failed proposal、active rejected receipt 与 no-progress shape 仍先按当前 authority/focus/lineage 投影；manifest/repair/grounding 结构失败优先于 replay；duplicate binding contradiction 不写 revision/CAS，fresh replacement catalog 排除 create-only existing-target operation 并保留 revision-bound replacement；既有 File create receipt crash/restart 与 duplicate receipt 回归继续保证零第二次 owner effect
Deployment/live evidence: 未部署、未触发 Resume/Restart；历史运行不作为本切片证明，仍需 O5-O8 后使用清理数据验证 cwi-026 replacement -> cwi-027 -> terminal delivery
Remaining risk: O4 自动化代码收口完成，但 SO-MC-10 仍未 live-closed；O5 reconciliation/repair-surface、O6 governance adapter、O7 controller/helper、O8 文档/复杂度/live 验证仍 open
```

### 10.11 O5 reconciliation/repair-surface/proof 纯事实收口（2026-08-15）

```text
Work package: O5 reconciliation/repair-surface/proof comparison closure（自动化 code-closed；不代表 live-closed）
Commit: working tree（未提交）
Changed owners: 无；Orchestrator 仍是 SM-2/SM-3/SM-4 的唯一 Runtime owner，File/Shell/其他 capability owner 仍只返回 typed observation/receipt
Changed state machines: SM-2 reconciliation、CWI/binding/evidence attribution、repair-surface freshness；SM-3 receipt/binding admission；未改变 credit/reopen/dispatch/预算/CAS 语义
Changed owner-local components: `biz/internal/proof` 新增唯一的 reconciliation history/owner-observation 比较、repair-surface exact replay/conflict、sealed validation causation、current-CWI/binding lineage、coverage proof lineage attribution 与 owner receipt/binding finding；`biz/internal/grounding` 新增 validation target grounding 与 canonical target-set comparison；Runtime 仅负责 aggregate 私有事实投影、SM-2 disposition/credit/reopen 映射和现有 mutation/CAS
New internal event(s): none；没有新增 EventHub topic、Runtime state、owner command 或恢复预算
Cross-owner typed contract(s): none；复用 `RuntimeCWIReconciliation*`、`RuntimeRepairSurfaceConfirmation*`、`RuntimeCoverageProofLineage`、`EvidenceFact`、`RuntimeOperationReceipt` 与现有 target/freshness contract
Forbidden path removed: reconciliation duplicate/history、owner observation reason、repair confirmation replay/conflict、validation dispatch causation、旧 binding hash 迁移、receipt/binding proof 和 validation target freshness 不再由 Runtime 各自复制；focused package 不导入 Runtime aggregate/repository/EventHub，不写 CWI ledger，不选择 retain/reopen/prerequisite/credit/dispatch/terminal
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/internal/grounding -count=1`；`go test ./internal/modules/kernel/orchestrator/biz/internal/proof -count=1`；新增 reconciliation stale/duplicate、repair confirmation conflict/causation、old-binding/equivalent-resolution、exact receipt/multi-binding 与 target freshness 回归；`TestO5ProofAndGroundingComparisonsUseFocusedEntries`；`TestFocusedRulePackagesRemainStateless`
Boundary/performance scan: Runtime production 只通过 focused proof/grounding entry 投影纯判断；target set normalization/intersection 收敛到 `grounding`，lineage attribution 在一次 normalized facts 投影内完成，避免 handler 内重复构建比较规则；`gofmt`、`git diff --check` 通过
Restart/stale/duplicate evidence: exact reconciliation batch、repair confirmation 与 receipt replay 均零 revision/CAS；历史/陈旧 batch 不重新激活 generation；旧 binding evidence 只有当前 binding hash 或已持久化 equivalent resolution 才能归因；不完整 owner observation、过期 authority、multi-binding receipt 和未刷新 target observation fail-closed，不能产生错误 credit/reopen/dispatch
Combined gates: focused/internal、Runtime、全仓 `go test ./... -count=1`、Runtime race、`make fmt-check`、`make vet`、`make verify-boundary-contract` 与 `git diff --check` 全部通过
Deployment/live evidence: 未部署、未触发 Resume/Restart；历史运行不作为本切片证明，仍需 O7-O8 与 fresh-data 验证 `cwi-026 replacement -> cwi-027 -> terminal delivery`
Remaining risk: O5 自动化代码收口完成；O7 controller/executor/helper、O8 文档/复杂度/live 验证及 SO-MC-10 fresh-data live proof 仍 open
```

### 10.12 O6 governance adapter/receipt mapping 收口（2026-08-15）

```text
Work package: O6 governance identity/receipt/adapter closure（自动化 code-closed；不代表 live-closed）
Commit: working tree（未提交）
Changed owners: 无；Orchestrator 仍是 Runtime governance binding、resolution、evidence 与 resume dispatch 的唯一 owner，Decision/Approval 与 capability owner 仍只提交 concrete ticket/result/receipt
Changed state machines: SM-3 operation receipt admission、governance suspension binding、governed dispatch/resolution identity fence；SM-2 仍只由既有 reducer 把 persisted governance failure 映射为 `need_governance`；SM-4 drive/wait projection 只读取已绑定 ticket，未改变任何 transition、预算或投递顺序
Changed owner-local components: `biz/internal/governancecheck` 成为 request/dispatch、binding/execution/receipt、ticket/resolution、grant/resume-fence 与 governance dependency 的唯一纯比较/投影入口；`candidateDispatchComponent` 接管 operation governance receipt closeout；`authorityComponent` 继续独占 request/resolution mutation、evidence、resume dispatch 与 CAS；Biz adapter 只做 concrete EventHub contract 与 private event 映射
New internal event(s): none；capability receipt 继续先进入 `operation_receipt_observed`，其治理挂起收尾继续由既有 `dispatch_execution_requested` Owner 私有事件触发；没有增加 EventHub topic、Runtime state、ledger、retry 或 mutation entry
Cross-owner typed contract(s): none；复用 `RuntimeOperationReceipt`、`RuntimeGovernanceRequirement`、`RuntimeGovernanceRequestRecord`、`RuntimeGovernanceResolution`、operation binding/grant 与既有 Decision concrete command/result
Forbidden path removed: executor 不再自行拼接 suspended binding/dependency 或扫描 grant；Controller 不再承载 governance receipt closeout；receipt query 不再直接比较 grant；governance adapter/drive projection 不再手写 ticket/dispatch 扫描；Runtime aggregate validation 不再重复实现 request/dispatch、binding/execution/receipt/request 与 resolution identity 比较
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/internal/governancecheck -count=1`；新增 exact durable receipt、requirement drift、stale fence、detached grant、request lookup、dependency precedence/provider fallback 回归；Runtime governance 全包及 Biz governance adapter/restart 回归
Boundary/performance scan: `TestO6GovernanceComparisonsUseFocusedEntries` 固定 Runtime focused call sites与 `candidateDispatchComponent` receiver；`TestO6GovernanceAdaptersUseFocusedMappings` 禁止 adapter 重新扫描 GovernanceRequests 或绕过 `submitRuntimeInternalEvent`；grant 返回 detached evidence refs，receipt/requirement 各归一一次，没有额外 repository load、CAS 或 owner call
Restart/stale/duplicate evidence: exact ExecutionKey+Fence+receipt hash+requirement 才能形成 suspended binding；响应丢失仍从 persisted owner receipt 恢复；stale authority、conflicting receipt/requirement、unbound ticket、wrong resume fence、duplicate request/resolution 均在 mutation 前 fail-closed，既有 duplicate resolution 保持零第二次 governance evidence/resume dispatch
Combined gates: focused、Biz/Runtime targeted、全仓 `go test ./... -count=1`、`go test -race ./internal/modules/kernel/orchestrator/biz/runtime -count=1`、`make fmt-check`、`make vet`、`make verify-boundary-contract` 与 `git diff --check` 全部通过
Deployment/live evidence: 未部署、未触发 Resume/Restart；自动化证据不替代 fresh-data real-provider 验证
Remaining risk: O6 自动化代码收口完成；O7 controller/executor/helper、O8 文档/复杂度/live 验证及 SO-MC-10 `cwi-026 replacement -> cwi-027 -> terminal delivery` 仍 open
```

### 10.13 O7 controller/executor/helper 物理内聚与恢复交接收口（2026-08-15）

```text
Work package: O7（自动化 code-closed；不代表 O8/live-closed）
Commit: working tree（未提交）
Changed owners: 无；Orchestrator 仍是 Runtime aggregate、SM-2/SM-3/SM-4 reducer 与唯一 CAS owner；Run、File、LLM、Result、Workspace owner 未改变
Changed state machines: SM-2/SM-3/SM-4 的 owner-local handler 分组与 SM-3→SM-4 recovery handoff；未改变 SM-1 lifecycle、状态语义、attempt 上限、side-effect 顺序或终态投递顺序
Changed owner-local components: `controller.go` 收窄为初始化/Load；authority、assessment、candidate prerequisite/evidence/dispatch、planning prerequisite/semantic contract、drive delivery 与 execution coordinator 分文件；`runtime_mainline.go` 的 prerequisite drive 单独归入 Biz adapter；compatibility facade 仅转发到既有组件或同一 `ApplyInternalEvent`
New internal event(s): 无新增 kind；复用既有 `dispatch_execution_requested` 与 `recovery_budget_consumed`。前者在 claimed replay / sealed replay-proof 场景只返回 typed `recovery budget required` handoff，后者由 SM-4 `driveDeliveryComponent` 独立提交 `OperationRecoveryAttempts`，下一次 dispatch 以 operation ID + persisted attempt 作为授权 fence
Cross-owner typed contract(s): 无；新增恢复字段只位于 Orchestrator owner-private `RuntimeInternalTransactionPayload`，不进入 EventHub 或 Runtime public contract
Forbidden path removed: semantic planning 不再保留 callback/CAS transaction；CandidateRequestEnsured 不再由 planning handler 同时写 SM-2 failure，改为 `PendingAuthorityFailure` 后由独立 `failure_observed` event 接纳；candidate dispatch 不再直接终结 equivalent authority；SM-3 execution 不再直接消费 SM-4 recovery ledger；controller compatibility seam 不再跨组件直接写 budget/authority；旧 controller/executor 文件位置引用同步迁移到真实 owner transition 文件
Focused tests: `go test ./internal/modules/kernel/orchestrator/biz/runtime -count=1`；`go test ./internal/modules/kernel/orchestrator/biz -count=1`；O7 structure tests（semantic transaction callback、candidate request/execution facet、SM-3→SM-4 budget handoff、transition-file ownership）；owner receipt CAS-before/after、claimed safe replay、sealed replay-proof、MCP failure observation、claim/evidence/dispatch-close crash matrix
Boundary/performance scan: `make verify-boundary-contract` 通过；boundary script 已按物理拆分更新 `candidate_dispatch_transition.go` 与 `authority_transition.go` 的 sole-admission/active-dispatch fence；`git diff --check` 通过；未新增 EventHub、repository、aggregate、CAS 或 retry authority
Restart/stale/duplicate evidence: candidate request 的 SM-3→SM-2 failure handoff 在第二个 event 才产生 authority revision；恢复预算 event 与 dispatch event 各自最多一次 CAS；授权丢失或预算耗尽 fail-closed 为 outcome_unknown；owner receipt response-loss 仅按 ExecutionKey+Fence+ReplayProof 重用，重复 owner delivery 不增加副作用、evidence 或 revision
Combined gates: `go test ./... -count=1`、Orchestrator Runtime/Biz 全包、`go test -race ./internal/modules/kernel/orchestrator/biz/runtime -count=1`、`make fmt-check`、`make vet`、`make build`、`make verify-boundary-contract` 与 `git diff --check` 均已通过
Deployment/live evidence: 未部署、未触发 Resume/Restart；自动化 crash/restart 矩阵不替代 fresh-data real-provider 验证
Remaining risk: O7 自动化代码收口完成；O8 仍需完成实现 SoT/复杂度复盘、最终全量门禁与 fresh-data `cwi-026 replacement -> cwi-027 -> terminal delivery` live proof，SO-MC-10 在此之前保持 open
```

### 10.14 O8 实现 SoT、复杂度盘点与自动化门禁收口（2026-08-15）

```text
Work package: O8（文档/自动化 code-closed；不代表 deployed/live-closed）
Commit: working tree（未提交）
Changed owners: 无；22 个 framework 运行单元仍为 1 Initiator / 4 Modules / 17 Blocks
Changed state machines: none（只固化 O0-O7 已实现的 SM-2/SM-3/SM-4 owner 边界与恢复交接证据；SM-1 未变）
Changed owner-local components: Runtime 实现 SoT、当前事实入口、active roadmap、single-owner runbook、22-unit complexity inventory 与 boundary verifier 的物理文件路径/结构检查
New internal event(s): none
Cross-owner typed contract(s): none；recovery operation identity/attempt 仍仅存在于 Orchestrator owner-private transaction envelope
Forbidden path removed: 无新增行为迁移；补齐物理拆分后的 boundary script 路径，增加 recovery authorization envelope 的 malformed/partial/non-dispatch fail-fast 回归，避免旧文件引用或半截 SM-3→SM-4 token 重新形成第二入口
Focused tests: Runtime internal-event envelope、O0-O7 structure/restart tests；Orchestrator Runtime/Biz focused suites；Workorchd Biz/server suites
Complexity evidence: Orchestrator 当前 working tree 为 104 个生产 Go 文件、约 27,788 行与 91 个测试文件、约 45,871 行；controller.go 约 154 行；新增 transition 文件按 SM-2/SM-3/SM-4 facet 分组。文件数增加是职责物理化，不是新增 Owner、aggregate、CAS 或 EventHub topic
Boundary/performance gates: 当前 working tree 已通过 `make fmt-check`、`make vet`、`go test ./... -count=1`、`make test-race`（即 `go test -race ./...`）、`make build`、`make verify-boundary-contract` 与 `git diff --check`；构建仅生成被忽略的 `bin/`/console dist 产物
Restart/stale/duplicate evidence: recovery credentials 仅允许 dispatch event 的完整 operation ID + 正整数 persisted attempt；其他 transaction、半截字段、负 attempt 在 repository load 前拒绝；exact counter fence 防止 stale token 重用，预算耗尽保持 outcome_unknown/fail-closed
Deployment/live evidence: 未部署、未触发 Resume/Restart；fresh-data `cwi-026 replacement -> cwi-027 -> terminal delivery` 仍需在目标服务部署后执行
Remaining risk: 当时 WIC 的 WD5/WD7/WD8 尚未完成；这些 code-side 风险已由 10.21-10.23 关闭。deployed/live proof 与 SO-MC-10 仍 open
```

### 10.15 WD5-A readmodelprojection 机械归并（2026-08-15）

```text
Work package: WD5-A（机械结构切片；不宣告 WD5 整包完成）
Commit: working tree（未提交）
Changed owners: 无；Projection 仍是 Workorchd 内唯一可重建 derived-state writer，Eventstream/owner 状态未变
Changed state machines: none；不修改 SM-1/SM-2/SM-3/SM-4、事件合同、快照字段、排序、截断、clone 或 high-watermark 规则
Changed owner-local components: `readmodelprojection` 内的 scalar helper、read-model view、contract/section、event-payload、string/numeric 及 family 文件；保留每个原文件名的来源注释
Forbidden path removed: 一字段/一函数/一文件的机械微文件散布；没有新增 Store、revision、EventHub topic、owner command 或 application facade
Focused tests: `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1`；新增结构测试固定 production 文件不超过 150 且存在 consolidated `projection_` 文件
Boundary/performance scan: 多组无 import/精确 import helper 归并后 production 文件由 404 降至 134；mapper 算法与 DTO 内容未改，`git diff --check` 通过
Restart/stale/duplicate evidence: derived projection 仍可由 source event/high-watermark 重建；本切片不产生 authority、retry 或 owner side effect
Deployment/live evidence: 未部署、未触发 Resume/Restart；projection 机械归并不替代 fresh-data runtime live proof
Remaining risk: 当时的 WD5 facade/export 风险已由 10.18-10.21 关闭；deployed regression 仍 open
```

### 10.16 WD6-A/WD6-B appservice 薄配对归并（2026-08-15）

```text
Work package: WD6-A + WD6-B（低风险 package slice；不宣告 WD6/WD8 整包完成）
Commit: working tree（未提交）
Changed owners: 无；Goal/DoD quality、content read、workspace setup draft 仍是 application 纯编排/typed adapter，不拥有 Workspace/Run/Runtime 状态
Changed state machines: none；不修改 owner event、HTTP/SSE wire、LLM/File/Result 合同或 Runtime transition
Changed owner-local components: `qualitycore+qualityports -> definitionquality`；`contentcore+contentports -> content`；`setupdraftcore+setupdraftports -> setupdraft`
Forbidden path removed: 三组 `core/ports` 双包之间的重复 import/facade 边界；旧路径目录和 Go import 已清零；未新增 alias、callback bus 或通用 DTO bridge
Focused tests: `go test ./internal/modules/application/workorchd/biz/appservice/... ./internal/modules/application/workorchd/service/server -count=1`；`TestWD6ThinPairsHaveOneOwnerPackage` 固定新包存在且旧 split package 不得复生
Boundary/performance scan: `rg 'qualitycore|qualityports|contentcore|contentports|setupdraftcore|setupdraftports' --glob '*.go'` 无结果；合并仅改变包边界，不增加 owner query、序列化或 EventHub topic
Restart/stale/duplicate evidence: 本切片无持久化状态和重试 ledger 变化；现有 typed command/result failure matrix 保持通过
Deployment/live evidence: 未部署、未触发 Resume/Restart；需 WD5/WD7/WD8 完成后再做 deployed HTTP/CLI/SSE 核对
Remaining risk: 当时的 forwarding/transport/WD8 风险已由 10.22-10.23 关闭；deployed regression 仍 open
```

### 10.17 WD7-A production transport/runtime 边界（2026-08-15）

```text
Work package: WD7-A（production boundary slice；不宣告 WD7/WD8 整包完成）
Commit: working tree（未提交）
Changed owners: 无；Biz 仍是唯一 Workorchd EventHub/lifecycle owner，Workspace/Run/Runtime owner 未变
Changed state machines: none；server 只做 transport，不能选择或迁移 SM-1/SM-2/SM-3/SM-4
Changed owner-local components: `biz/runtimeComponent` 唯一持有长生命周期 `runtimecore.Session`；`runtimeApplicationPorts` 只保存 typed borrow callback，不复制 Session 指针；`workspaceCommandGateway` 只发送 concrete Workspace intent/result；生产 `service/server` 只接收 typed `ApplicationPorts`
Forbidden path removed: production server 对 `runtimecore`、magicCommon EventHub、owner operation adapter 和 Session constructor 的直接依赖；legacy session/operation constructors 已移至 `_test.go` 兼容夹具并 fail-closed 默认构造器
Focused tests: `go test ./internal/modules/application/workorchd/service/server -count=1`；`go test ./internal/modules/application/workorchd/... -count=1`；`production_transport_boundary_test.go` 扫描非 test Go 文件禁止 runtimecore/EventHub import
Boundary/performance scan: production server 的 runtimecore/EventHub/Subscribe/Send/Post 扫描为空；typed ApplicationPorts、projection writer、execution admission 仍由 Biz 注入，HTTP/SSE wire 未改
Restart/stale/duplicate evidence: gateway 只校验 scope/request/kind/result，未引入 queue/retry/recovery authority；ApplicationPorts 每次只借用并关闭短生命周期 Session，不持有第二份长生命周期指针；Session close、projection close、execution admission close 的 Stop 顺序保持幂等
Deployment/live evidence: 未部署、未触发 Resume/Restart；只证明 production boundary，不替代 deployed HTTP/SSE/CLI 与 fresh-data runtime proof
Remaining risk: 当时的 compatibility/WD8 code 风险已由 10.23 关闭；package-local test runtime 不是生产 API，deployed regression 仍 open
```

### 10.18 WD5-B governance projector facade（2026-08-15）

```text
Work package: WD5-B（治理投影语义 facade；不宣告 WD5 整包完成）
Commit: working tree（未提交）
Changed owners: 无；decision/approval/governance 仍为只读 projection，不拥有 Ticket、Approval 或 Runtime 状态
Changed state machines: none；不修改 SM-1/SM-2/SM-3/SM-4、事件合同、分页排序、DTO clone 或 blocker normalization 语义
Changed owner-local components: `readmodelprojection.GovernanceProjection()` 是治理族唯一 application-facing facade；mapper、分页窗口、view clone、open-blocker 与 workspace prompt/normalization helper 已降为包私有
Forbidden path removed: production Workorchd 不再直接调用治理族散落的 `Map*`/`Page*`/`Project*`/blocker/normalization helper；facade 不保存可变状态、不发送 owner command、不触发 Runtime
Focused tests: `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice/governanceports ./internal/modules/application/workorchd/biz/appservice/readmodelports ./internal/modules/application/workorchd/biz/appservice/workspaceviewcore ./internal/modules/application/workorchd/service/server -count=1`；结构测试固定 production 调用必须经 `GovernanceProjection()`
Boundary/performance scan: governance helper 的 production direct-call 扫描为空；本切片只改变 facade/可见性，不增加 Store、revision、EventHub topic、owner command 或序列化路径
Restart/stale/duplicate evidence: facade 无状态且纯函数，可由 source snapshot 重建；不产生 authority、retry、CAS 或跨 Owner side effect
Deployment/live evidence: 未部署、未触发 Resume/Restart；治理 facade 自动化证据不替代 fresh-data runtime live proof
Remaining risk: 当时的 facade/WD7-B/WD8 code 风险已由 10.19-10.23 关闭；deployed/live proof 仍 open
```

### 10.19 WD5-C remaining five projection facades（2026-08-15）

```text
Work package: WD5-C（workspace/run/context/evidence/activity production facade 路由；不宣告 WD5 整包完成）
Commit: working tree（未提交）
Changed owners: 无；六个 facade 均为无状态 typed function table，Projection manager/store/revision/high-watermark 仍只有一份
Changed state machines: none；不修改 SM-1/SM-2/SM-3/SM-4、owner event、projection DTO、排序、截断、clone 或 replay 语义
Changed owner-local components: 新增 `WorkspaceProjection()`、`RunProjection()`、`ContextProjection()`、`EvidenceProjection()`、`ActivityProjection()`；与 `GovernanceProjection()` 一起承接所有 Workorchd production read-model 调用
Forbidden path removed: production Workorchd 除两个 typed snapshot input 外不再直接访问散落的 readmodelprojection helper；facade 每次返回新的无状态函数表，调用方不能替换全局函数或建立第二 Store
Focused tests: `go test ./internal/modules/application/workorchd/... -count=1`；`TestWD5ProductionProjectionUsesSemanticFacades` 扫描全部非测试 Workorchd 源码并拒绝旧 direct helper call
Boundary/performance scan: production `readmodelprojection.<helper>` 调用只剩六个 facade 与 `RunSnapshotModelInput`/`WorkspaceSnapshotModelInput` typed input；该切片完成时 exported declaration 从 230 基线降至 223，production 文件为 140/17,066 LOC
Restart/stale/duplicate evidence: facade 无缓存、无 revision、无 observer、无 command；所有结果仍由相同纯 mapper 和唯一 projection store 重建
Deployment/live evidence: 未部署、未触发 Resume/Restart；projection facade 迁移不替代 HTTP/SSE/CLI deployed regression 或 SO-MC-10 live proof
Remaining risk: 当时的 helper/WD7-B/WD8 code 风险已由 10.20-10.23 关闭；deployed/live proof 仍 open
```

### 10.20 WD5-D semantic filenames and helper visibility（2026-08-15）

```text
Work package: WD5-D（投影语义命名 + evidence/activity helper 降可见性；不宣告 WD5 整包完成）
Commit: working tree（未提交）
Changed owners: 无；只调整同一 readmodelprojection 包的文件名与函数可见性
Changed state machines: none；不修改 SM-1/SM-2/SM-3/SM-4、projection 算法、DTO、Store、revision 或 event contract
Changed owner-local components: 33 个 `projection_family_NN.go` 全部按真实职责改名，例如 `evidence_page_projection.go`、`run_activity_page_projection.go`、`llm_usage_trace_projection.go`；evidence page 与 activity/event/memory mapper 只经 facade 暴露
Forbidden path removed: 数字序号文件不再掩盖业务职责；七个 evidence page helper 和五个 activity helper 从 package export surface 移除；结构测试禁止 `projection_family_*.go` 复生
Focused tests: `go test ./internal/modules/application/workorchd/... -count=1`；readmodelprojection 结构/facade 测试覆盖语义文件名、非空纯函数表和 production direct-call 扫描
Boundary/performance scan: production 文件仍为 140/17,066 LOC，数字 family 文件为 0，exported declaration 从 223 进一步降至 211；纯 rename/visibility change 不增加执行路径
Restart/stale/duplicate evidence: 无持久化、重试、CAS 或 projection revision 变化；相同输入仍调用相同 mapper
Deployment/live evidence: 未部署、未触发 Resume/Restart；本切片不替代 deployed/live proof
Remaining risk: 当时的 helper/WD7-B/WD8 code 风险已由 10.21-10.23 关闭；deployed/live proof 仍 open
```

### 10.21 WD5-E final projection export surface（2026-08-15）

```text
Work package: WD5-E（WD5 代码/自动化终结）
Commit: working tree（未提交）
Changed owners: 无；只收窄同一 readmodelprojection 包的 application-facing surface
Changed state machines: none；不修改 SM-1/SM-2/SM-3/SM-4、DTO、projection store、revision 或 high-watermark
Changed owner-local components: 六个无状态 facade factory（Activity/Context/Evidence/Governance/Run/Workspace）承载全部 production projection 调用；只保留 RunSnapshotModelInput/WorkspaceSnapshotModelInput 两个 typed input
Forbidden path removed: 其余 203 个 helper declaration 全部 package-private；删除 RunScopedResults/LatestRuntimeCheckpoint 等重复 wrapper；production direct helper 和数字 family filename 均由 AST 结构测试拒绝
Focused tests: `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection -count=1`；`go test ./internal/modules/application/workorchd/... -count=1`
Boundary/performance scan: exported declaration 从 230 历史基线/211 上一切片降至精确 8 个；facade 为无状态函数表，不增加缓存、clone 层或 owner query
Restart/stale/duplicate evidence: 无持久化、CAS、重试或 lifecycle 变化；相同输入仍进入原纯 mapper 与唯一 projection store
Deployment/live evidence: 未部署、未触发 Resume/Restart；WD5 code/automated closure 不替代 deployed regression
Remaining risk: 仅剩 deployed HTTP/SSE/CLI 与 fresh-data live proof，不再保留 WD5 helper 可见性待办
```

### 10.22 WD6-C forwarding/cache-authority facade 收口（2026-08-15）

```text
Work package: WD6-C（WD6 代码/自动化终结）
Commit: working tree（未提交）
Changed owners: 无；Application 内部 facade/薄包归并
Changed state machines: none；不修改任何 owner transition、Runtime candidate/evidence/dispatch 或 terminal delivery
Changed owner-local components: server 与 CLI 直接调用六族 typed projection facade；setupdraft 直接调用内聚包 Normalize/Merge/RevisionBaseline/ParseCompletion；content reader 只从 owner-backed artifact metadata 授权读取
Forbidden path removed: 删除根 `read_model_projection_service.go`、RunStatus/RunObservability 与 setupdraft 纯转调 wrapper；删除以 transport run-snapshot cache 充当 artifact metadata authority 的 application entry；缓存只优化 owner-backed run snapshot read
Focused tests: `go test ./internal/modules/application/workorchd/biz/appservice/... ./internal/modules/application/workorchd/service/server -count=1`
Boundary/performance scan: `biz/appservice` 保留的根入口承担 session/backend boundary 或稳定 application ingress；production server 不再导入根 appservice；artifact content cache shortcut 为零
Restart/stale/duplicate evidence: transport cache hit/miss 不改变 artifact authorization；不存在 owner metadata 时返回 not-found，不能由陈旧 snapshot 复活 content
Deployment/live evidence: 未部署、未触发 Resume/Restart
Remaining risk: deployed endpoint compatibility 与 fresh-data live proof 仍 open
```

### 10.23 WD7-B/WD8 transport compatibility 与自动化收口（2026-08-15）

```text
Work package: WD7-B + WD8（WIC code/automated closure）
Commit: working tree（未提交）
Changed owners: 无；Biz 仍是 application lifecycle/session/projection/execution 唯一 owner，server 仍是 transport adapter
Changed state machines: none；不修改 SM-1/SM-2/SM-3/SM-4 transition、authority hash、candidate、receipt、evidence 或 delivery
Changed owner-local components: Service.Run 只启动 HTTP；projection Start/Close 仅由 biz/projection.Component 执行；serverState 只保存 typed application/read/derived-write ports 与有界 transport state
Forbidden path removed: 删除生产 shared Session、runtimeSessionErr、compatRead cache callbacks、server-local projection observer/lifecycle/manager、productionWiring 双路径、compatibility execution projection、test-only Service.Startup 扩展和 exported Session-backed HTTP constructor；缺 execution/scheduler/writer port 一律在 listener 启动前 fail-closed
Focused tests: `go test ./internal/modules/application/workorchd/... -count=1`；结构测试扫描生产 imports、compatibility identifiers、facade export whitelist 和数字 filename
Boundary/performance gates: `make fmt-check`、`make vet`、`go test ./... -count=1`、`make test-race`、`make build`、`make verify-boundary-contract` 与 `git diff --check` 全部通过
Boundary/performance scan: Workorchd 650 个生产文件/56,173 LOC、153 个测试文件/41,573 LOC；server 41 个生产文件/8,142 LOC；readmodelprojection 140 个生产文件/17,087 LOC/8 exported declaration；22 个 framework 单元不变
Restart/stale/duplicate evidence: cache 不授权 artifact、projection lifecycle 不由 transport 重启、缺 port fail-closed、test projection/session teardown 不泄漏后台任务；生产 server 无第二套 queue/store/session owner
Deployment/live evidence: 未部署、未触发 Resume/Restart；deployed HTTP/SSE/CLI 与 `cwi-026 replacement -> cwi-027 -> terminal delivery` 仍待最新服务核对
Remaining risk: WIC code/automated queue 已关闭；SO-MC-10 与 deployed regression 保持 open
```

## 11. 明确禁止的“伪收口"

- 把 `cwi-027` 状态迁移给 File；
- 让 File 选择 `patch/apply`、retry、replan 或 terminal；
- 让 LLM 通过 JSON 创建 target/binding/evidence authority；
- 在 Application、TUI、Web 或 projection 中自动 Resume；
- 增加更大的 retry/authority budget 掩盖等价 authority；
- 只放宽 `file/write` 以绕过 existing target；
- 只修 prompt 不修 sealed manifest/evaluator；
- 只修 File policy 不修 semantic feasibility/admission 一致性；
- 拆出 SM-2、SM-3、SM-4 作为独立 framework owner；
- 把 focused package 变成持有 EventHub、repository、aggregate 或 lifecycle 的隐性 Owner；
- 为了减少文件行数而导出 Runtime 私有状态、建立通用 mutation callback 或跨 package reducer 调用；
- 把 Workorchd transport、projection 或 use case 拆成新的 framework Owner；
- 让 Workorchd server/projection 持有 runtime session、发送恢复命令或解释 owner state transition；
- 用旧 Run/旧 schema 数据证明当前 baseline 已收敛。

## 12. 最终交付标准

整体交付只有在 W0-W8、O0-O8、WD0-WD8、C027-1 至 C027-4、全量自动化门禁和 fresh-data live proof 全部
完成后才可标记完成。OIC/WIC 可以单独标记 code/automated-closed，但 SO-MC-10 在 fresh-data live proof 前保持
`open`。
