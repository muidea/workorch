# Runtime 单 Owner 执行链路代码收口 Runbook

Status: active implementation appendix

Type: code-closure-runbook

Last Updated: 2026-08-14

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

当前剩余项是部署验收而非代码缺口：重新部署包含本 Runbook 修正的服务后，需要用清理后的数据重跑
`cwi-026`/`cwi-027`，确认 existing target 选择 revision-bound replacement、prospective target 完成
owner grounding，并观察 `cwi-027` 执行及最终 live closure。未完成该 live run 前，不宣称 SO-MC-10 已关闭。

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

## 4. `cwi-027` 专项闭环

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

## 5. 完整准入 evaluator 必须检查的维度

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

## 6. 测试与验证矩阵

### 6.1 定向 Go 测试

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
```

### 6.2 必测不变量

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

### 6.3 仓库门禁

```bash
make fmt-check
make vet
make test
make test-race
make verify-boundary-contract
make build
```

`make verify-ci-mvp-file` 只作为 mock/managed regression；不能替代真实 provider 的 SO-MC-10 live proof。

## 7. 部署与 live 验收顺序

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

## 8. 实施记录模板

每个工作包完成后在提交说明或验证记录中填写：

```text
Work package: Wn / C027-n
Commit:
Changed owners:
Changed state machines: SM-1/SM-2/SM-3/SM-4
New internal event(s):
Cross-owner typed contract(s):
Forbidden path removed:
Focused tests:
Boundary scan:
Restart/stale/duplicate evidence:
Deployment/live evidence:
Remaining risk:
```

未填写 owner、state machine、CAS、测试和 live evidence 的修改，不视为收口完成。

## 9. 明确禁止的“伪收口”

- 把 `cwi-027` 状态迁移给 File；
- 让 File 选择 `patch/apply`、retry、replan 或 terminal；
- 让 LLM 通过 JSON 创建 target/binding/evidence authority；
- 在 Application、TUI、Web 或 projection 中自动 Resume；
- 增加更大的 retry/authority budget 掩盖等价 authority；
- 只放宽 `file/write` 以绕过 existing target；
- 只修 prompt 不修 sealed manifest/evaluator；
- 只修 File policy 不修 semantic feasibility/admission 一致性；
- 拆出 SM-2、SM-3、SM-4 作为独立 framework owner；
- 用旧 Run/旧 schema 数据证明当前 baseline 已收敛。

## 10. 最终交付标准

代码收口只有在 W0-W8、C027-1 至 C027-4、全量自动化门禁和 fresh-data live proof 全部完成后才可标记完成。
SO-MC-10 在此之前保持 `open`。
