# Runtime V2 单 Owner 状态机统一收口方案

Status: active

Type: implementation closure plan

Plan ID: `RUNTIME-SMO`

Last Updated: 2026-07-29

## 1. 目的、范围与唯一性

本文是 Runtime V2 唯一活动代码收口队列。它以以下不可变规则为中心：

> 一个状态机只有一个语义 owner；跨组件只传 typed intent、immutable fact、owner receipt 或 delivery acknowledgement；任何状态转换只能由 owner Biz 先把外部输入转成 owner-local internal event，再通过 owner 的唯一 transition coordinator 持久化。

本轮不增加 Module/Block，不恢复 V1，不增加兼容双路径，不把 retry、CAS、reducer、filesystem/network I/O 下沉到 `internal/pkg/workorchruntimecontract`。

权威顺序：

1. `docs/00-core/runtime-stage-contract.md`：四状态机、候选结果分类和禁止迁移；
2. `internal/modules/kernel/orchestrator/readme.md` 与 `internal/pkg/workorchruntimecontract`：实现 SoT；
3. 本文：唯一未关闭实施队列；
4. `docs/01-overview/current-state.md`、`current-unclosed-matrix.md`：事实入口。

历史 C7、C8-B～C8-F 只保留为不可回退的测试基线。它们不证明新的 deployed aggregate 已收敛，也不能覆盖新的 persisted counterexample。

## 2. 最新 deployed 反例与根因

全新空 workspace `ws-d3f6c386d1ab82a5` 的单次 activation 证明：

1. SM-1 自动进入 `active` 并封存 ready provider/model；
2. SM-3 的 `go.mod`、`main.go` 两个 File operation 都形成真实 `workspace_durable` receipt；首个 receipt 改变 workspace tree 后，同 dispatch 后续 operation 继续执行；
3. File、Shell、LLM、Result、Workspace、Run 都没有越权写对方状态；terminal delivery 完成 `Result -> Workspace -> Run` acknowledgement；
4. 没有无信息 Decision/Approval，也没有人工 Resume 补偿。

但 Run 最终进入：

```text
runtime_contract_defect:
complete rejection code is not declared in the correction matrix
```

直接原因不是 Shell 错误。Shell 对没有 `*_test.go` 的 `go test` 正确返回 `validation_target_missing` feasibility observation。错误发生在 Orchestrator：该普通 owner feasibility 先被记为 `RuntimeCandidateFailureClassInfeasible`，下一次 drive 又把它当作同 manifest 的 candidate correction；correction matrix 未声明该 owner code，于是错误密封为 contract defect。

根因有两类：

| ID | 根因 | 违反点 |
| --- | --- | --- |
| SMO-FEASIBILITY | candidate contract rejection、owner feasibility、prerequisite stale 被压入同一 candidate-attempt 失败通道 | owner 事实没有唯一、可持久化的 SM-2 交接；新增 capability code 会重复暴露未知分支 |
| SMO-COVERAGE | 空 workspace 的 generic `source_change` 与 DoD executable validation 分成不同 focus | 首轮没有被封存为必须创建测试产物；后续 validation focus 又没有合法 state-change authority |
| SMO-EVENT | adapter/mainline 可直接调用 candidate attempt 关闭入口 | 外部 observation 未先转为 Orchestrator owner-local event，难以审计每个事实的唯一转换 |
| SMO-TEST | 单测直接调用首次 `FailCandidateAttempt`，未覆盖下一次 drive 的 attempt-1 gate | 测试绿不能证明 production handoff 闭环 |

因此不得用“把 `validation_target_missing` 加进 correction matrix”“提高 repair budget”“让 LLM 再试一次”“自动 Resume”作为修复。

## 3. 状态 owner 与允许交互

| 状态域 | 唯一 owner | 允许跨组件输入 | 禁止 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run/biz` | activation/resume intent、terminal delivery fact、governance resolution fact | Run 以外组件设置 lifecycle state 或选择 action |
| SM-2 authority/focus/repair | `kernel/orchestrator/biz/runtimev2` | prerequisite、owner feasibility、receipt、assessment、governance immutable fact | capability 执行操作或选择下一 action |
| SM-3 candidate/sealed dispatch | `kernel/orchestrator/biz/runtimev2` | complete document、candidate evaluator outcome、owner receipt | adapter/owner 关闭 attempt、seal dispatch 或选择 retry |
| SM-4 drive/recovery/delivery | `kernel/orchestrator/biz` 与 owner-local `runtimev2` | Run state fact、delivery acknowledgement | 重建 authority、写 Run/Workspace/Result 状态 |
| File/Git/Shell/MCP/SSH/Web 技术状态 | 各 capability Block Biz | sealed typed operation/probe command | 修改 Runtime aggregate、focus、coverage、terminal |
| Result/Workspace 资源状态 | 各自 Block Biz | terminal delivery fact | 裁决 evidence、改写 Orchestrator terminal |

SM-2、SM-3、SM-4 继续由同一个 Orchestrator Module 拥有同一 aggregate，但它们必须共享一个 transition coordinator；不得拆成跨组件 reducer，也不得让 `mainline`、adapter、projection 各自持久化局部 Runtime 状态。

### 3.1 唯一事件流

```text
external typed input / owner result
  -> owner Biz validates scope, causation and sealed identity
  -> owner-local internal event
  -> owner transition coordinator CAS persists owner state
  -> immutable fact / acknowledgement for downstream owner
```

Orchestrator 内部事件最少包括：

- `drive_requested`；
- `prerequisite_observed`；
- `candidate_document_observed`；
- `candidate_evaluation_observed`；
- `owner_feasibility_observed`；
- `operation_receipt_observed`；
- `assessment_observed`；
- `governance_resolution_observed`；
- `terminal_delivery_ack_observed`。

这些是 Orchestrator 内部事件，不是新的跨 owner EventHub 泛型 envelope。跨 owner 仍使用现有 owner-defined typed command/result；进入 Orchestrator 后才映射为内部 event。

## 4. 统一候选与 owner-feasibility 合同

### 4.1 结果类型必须正交

完整 candidate document 的处理不得再把所有失败称为 CandidateRejection。唯一分类如下：

| Outcome | 产生者 | 是否同 manifest attempt-1 | Orchestrator owner-local 后继 |
| --- | --- | --- | --- |
| `request_recoverable` | LLM request session | 否 | SM-4 request recovery ledger |
| `candidate_rejection.correctable_once` | CandidateEvaluator | 是，仅一次 | SM-3 correction attempt |
| `candidate_rejection.rejected_final` | CandidateEvaluator | 否 | SM-2 typed terminal |
| `owner_feasibility.stale` | capability owner | 否 | SM-2 precise prerequisite retirement/reacquisition |
| `owner_feasibility.prerequisite_required` | capability owner | 否 | SM-2 `acquire_prerequisite` |
| `owner_feasibility.infeasible` | capability owner | 否 | SM-2 记录 owner fact，形成新 authority、替代 validation 或 typed terminal |
| `accepted` | evaluator 与 feasibility 均可执行 | 不适用 | SM-3 seal dispatch |

`CorrectionDisposition` 只属于 `CandidateRejection`；它只能由纯 CandidateEvaluator 产生。owner feasibility 不携带 `RuntimeAction`、`Terminal`、`Retry`、`Governance` 或“写入哪个状态”的指令。

### 4.2 owner feasibility observation

`internal/pkg/workorchruntimecontract` 只定义纯 DTO/validator：

- workspace/run/transaction/manifest/operation/admissibility identity；
- `feasible | stale | prerequisite_required | infeasible`；
- owner-neutral factual category：`resource_absent`、`validation_recipe_unavailable`、`environment_unavailable`、`policy_disallowed`、`target_not_regular`、`observation_unknown`；
- owner code、signature、target refs、diagnostic、required prerequisite ref；
- clone/normalize/hash/validate。

DTO 不得定义下一 Runtime action。每个 owner code 必须映射到一个 factual category；未映射 code 在 capability owner 内 fail-fast 为 owner contract defect，不能越过分类后在 Orchestrator 变成未知字符串。
`observation_unknown` 只可用于映射表中已声明但暂不存在更精确 owner-neutral 语义的事实；它绝不是新增 code 的兼容 fallback。

### 4.3 Orchestrator 内部转换

`owner_feasibility_observed` 只能由 Orchestrator transition coordinator 消费：

1. `stale`：精确 invalidation，退休旧 attempt/document，SM-2 获取相交 prerequisite；
2. `prerequisite_required`：持久化 prerequisite request，SM-2 进入 `acquire_prerequisite`；
3. `infeasible`：持久化 owner feasibility fact 和 rejected operation shape，禁止开启 correction attempt；SM-2 按 sealed focus/evidence policy 形成新 authority；
4. 无合法替代 recipe/authority：形成稳定 typed terminal，如 `validation_recipe_exhausted`，不是 `runtime_contract_defect`；
5. 只有 CandidateEvaluator 产生的结构性 wire/parameter/target/effect/coverage violation 才可进入 `correctable_once`。

实施状态：`validation_recipe_unavailable -> validation_recipe_exhausted` 已由 SM-2 owner-local event
写入事实，并由 reducer 直接归约为同名 typed terminal；它不会回落到普通 repair 或再次请求同类 candidate。
一般 `infeasible` 的新 authority 以 `AuthoritySnapshotID` 参与 candidate retry contract identity；旧 invalidated
attempt 保留 observation lineage，新 authority 只能重新从 attempt-0 开始，绝不能复用或开启旧合同的 attempt-1。

同 focus 形成新 authority 必须改变 persisted feasibility/gap fact、manifest hash 或 prerequisite generation；仅 revision 增长而 convergence signature 不变必须 fail-fast。不得由 owner code 或 LLM 文本直接选择 `produce_state_change`。

## 5. 空 workspace 覆盖编译收口

Application 不拥有 Runtime 状态机，但它拥有 activation-time `RuntimeBootstrap` 编译。因此它必须在 Run 创建前完成声明式 coverage/blueprint 选择：

1. profile/template 根据已选择的 template、capability contract、workspace manifest 与 Goal/DoD 生成 sealed coverage；
2. 需要 state-changing 产物的 DoD 必须在可执行的 state-change focus 中具有 coverage binding；其后 validation evidence 通过 dependency reopen/close；
3. 空 workspace 的模块、实现、测试等具体布局只能由 profile/template 的 declarative bootstrap blueprint 提供，或由 owner selector 允许的 prospective target 提供；Runtime core 不按 Go、测试框架、文件名或自然语言猜路径；
4. generic `source_change` 不能独自关闭“工作区包含测试”“特定用户行为”等 DoD；
5. validation focus 发现缺失产物时，只能使用 activation 已封存的 coverage dependency/owner-grounded failure target。若不存在，明确 terminal，而不是随机写文件或把 validation entry 固化为 repair target。

这使“创建产物 -> 验证 -> assessment”成为同一有向收敛链，而不是事后由 Shell error 反推实现计划。

## 6. 组件级收口范围

| 组件 | 变更职责 | 禁止职责 | 独立回归 |
| --- | --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | feasibility/evaluation outcome DTO、纯 validate/hash | transition/CAS/retry/EventHub/I/O | outcome normalization、未知 category/code 拒绝 |
| `kernel/orchestrator/biz/runtimev2` | 唯一 internal-event transition coordinator；SM-2/3 fact handoff | capability I/O、领域/文件名特判 | evaluation→feasibility→new authority/terminal 全矩阵 |
| `kernel/orchestrator/biz/runtime_v2_mainline.go` | 发起 I/O 后投递内部 observation | 直接关闭 candidate attempt 或选择修复 action | production drive 的 attempt-0/next-drive 回归 |
| `application/workorchd/.../activationcore` | 编译 sealed coverage、blueprint、dependency | 迁移 Run/Runtime state、猜测 owner state | empty workspace bootstrap/DoD coverage matrix |
| `workorchdefaults/defaults/templates.json` | profile/template 声明 state-change + validation blueprint/policy | core 领域分支 | template/profile compilation tests |
| `blocks/shell/biz` | 真实 preflight/effect/validation observation 与 factual category | Runtime action/retry/terminal | all Shell code→category 与 exact receipt restart |
| `blocks/file/git/mcp/ssh/web/biz` | 同一 typed feasibility/receipt taxonomy | aggregate/focus/coverage 写入 | all emitted owner code category completeness |
| `blocks/llm/biz` | 渲染 sealed C0-C5，其中 C4 仅含 provider-safe feasibility fact | 补 action、重建 authority | stable-prefix / new-authority request audit |
| `blocks/run` | SM-1 owner-local activation/block/terminal | action selection | duplicate intent、terminal delivery |
| `blocks/result`、`blocks/workspace` | terminal delivery owner-local acknowledgement | acceptance/terminal 裁决 | Result→Workspace→Run idempotency |
| TUI/Web/eventstream | intent、审计、投影 | setter、自动 Resume、错误文本驱动 | no implicit state mutation scan |
| `scripts/verify-boundary-contract.sh` | 静态边界门禁 | 业务分流 | pkg purity、owner-result、UI/application scan |

## 7. 唯一串行实施队列

同一时刻只有一张卡可以 `in_progress`；完成实现、测试、文档和 static gate 后才能开始下一张。

| 顺序 | 卡片 | 状态 | 影响 SM | 退出条件 |
| --- | --- | --- | --- | --- |
| SMO-1 | 合同与事件词汇归一 | completed | SM-2/SM-3 | `RuntimeOwnerFeasibilityObservation` 已与 candidate rejection 分离；owner code 统一离开 correction matrix |
| SMO-2 | Orchestrator 单 transition coordinator | completed | SM-2/SM-3/SM-4 | `RuntimeInternalEvent` 是 Biz 唯一 mutation entry；mainline/drive/receipt/governance/delivery adapter 只提交 owner-local event，`RuntimeController.ApplyInternalEvent` 分派，`RuntimeTransitionCoordinator.Commit` 是唯一 CAS 边界，并有静态 gate 与单次 revision/duplicate 回归 |
| SMO-3 | Feasibility 到新 authority 收敛 | completed | SM-2/SM-3 | stale 精确退休并重取、prerequisite_required 进入 acquisition、infeasible 以新的 `AuthoritySnapshotID` retry contract 重开 attempt-0；重复/重启不重写，`validation_recipe_exhausted` 直接终态 |
| SMO-4 | activation coverage/blueprint 收口 | completed | activation→SM-2 | `activationcore` 在 Run 创建前编译 profile recipe artifact 拓扑、owner-grounded prospective target、bootstrap 前置 slices 与 source→validation reopen dependency；空 workspace 不靠 Runtime/core 猜测布局 |
| SMO-5 | capability taxonomy 全量收口 | completed | owner→SM-2/3 | File/Git/Shell/MCP/SSH/Web 所有当前 emitted code 都以 `owner + code` 映射到 typed factual category；能力 owner 的 EventHub result boundary 拒绝未映射或跨 owner code，total-matrix 与 capability suite 均已回归 |
| SMO-6 | request/rendering 与 boundary gate | completed | SM-3/SM-4 | C4 仅含专用 provider-safe failure/evidence/assessment/shape projection及 sealed correction/continuation suffix；raw reason/summary/payload、owner feasibility、repair/recovery/governance action 均被拒绝，静态扫描通过 |
| SMO-7 | restart/concurrency/fresh live | in_progress | SM-1～SM-4 | 第 9 节所有退出条件通过；不得人工 Resume/改 runtime data 补偿 |

C7-B～C7-E、C8-B～C8-F 是 regression baseline，不再作为可独立开始的卡。原 C8-G 被 SMO-1～SMO-7 取代，因为 live 已证明其前置合同未闭合。

## 8. 强制测试矩阵

### 8.1 合同与状态转换

- 每个 emitted owner code 到 factual category 的 total matrix；未知 code 拒绝在 capability owner；
- CandidateEvaluator 的 rejection matrix 只覆盖其自身可产生的结构性 code；
- candidate rejection、owner infeasible、stale、prerequisite request 互不转换；
- attempt 1 仅发生于完整 `correctable_once` directive；owner infeasible 后零 attempt-1；
- 每个外部结果都经 owner-local internal event 后才增加 `StateRevision`；
- 无 convergence signature 进展的连续 transition typed fail-fast。

### 8.2 空 workspace 与 evidence

- 通用空 workspace 的 state-change coverage 包含 profile 所声明的必需交付物；
- state-change 后 validation 被 reopen/required，validation 不足不得改写为随机 state change；
- 缺测试、缺入口、不可用命令、无替代 recipe 分别进入 sealed 后继；
- structured validation observation 充分时 assessment 关闭 coverage；不足时只形成合法 validation authority 或 typed terminal；
- 不使用 Goal/DoD、文件名、语言或测试框架文本在 Orchestrator reducer 中推断路径。

### 8.3 幂等与多 workspace

- owner feasibility 在 record 前/后 crash、next-drive、duplicate、乱序；
- receipt/evidence 前后 crash，exact `ExecutionKey + Fence` 不重放副作用/provider call；
- duplicate Resume 只合并 drive；
- 多 workspace 同时 activation、feasibility、dispatch、delivery，没有 context/identity 串扰；
- terminal delivery 保持 `Result -> Workspace -> Run`，projection 失败不改写 terminal。

## 9. Fresh deployed live 退出条件

重新 build/deploy 后创建无历史 workspace/run；不得复用 repair-loop aggregate，不得手工修改 runtime data、Decision、Approval 或以 Resume 修复。

必须连续证明：

1. activation 自动进入 active 并封存 ready provider/model；
2. 空 workspace 的 sealed blueprint/coverage 支持模块、实现、测试等必需交付物；
3. 同一 dispatch 首项 durable effect 后其余 sealed operation 继续；
4. 每个副作用都有 owner receipt，且真实 changed refs 不越出 sealed target；
5. owner feasibility stale、prerequisite、infeasible 分别进入唯一、可观察后继；
6. 缺测试或无 validation recipe 不触发 candidate correction matrix，也不产生随机写入或无信息 ticket；
7. validation 与 assessment 使用 structured observation；
8. restart、duplicate Resume、并发 workspace 不重放副作用/provider，也不串 scope；
9. workspace 独立验证满足 Goal/DoD，Runtime sealed completed terminal，并完成 Result→Workspace→Run acknowledgement；
10. 不出现只增长 revision、相邻合同逐轮暴露或 error-text 驱动的循环。

## 10. 静态门禁与完成定义

最低验证命令：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/blocks/{file,git,shell,mcp,ssh,web}/... -count=1
go test ./internal/modules/kernel/orchestrator/biz/runtimev2 -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/modules/blocks/{run,workspace,result,decision,approval,llm}/... -count=1
make verify-boundary-contract
go test -race ./internal/modules/kernel/orchestrator/... ./internal/modules/blocks/{llm,file,shell,run,workspace,result}/... -count=1
go test -p 1 ./... -count=1
```

`verify-boundary-contract.sh` 必须检查：

- `workorchruntimecontract` 无 transition/CAS/retry/EventHub/I/O；
- capability result 不含 Runtime action、terminal、retry 或 aggregate mutation；
- mainline/adapter 不直接调用 aggregate transition persistence；
- 只有 Orchestrator coordinator 写 SM-2/SM-3/SM-4 aggregate；
- 只有 Run/Workspace/Result owner 写各自 lifecycle；
- application/TUI/Web/eventstream 无自动 Resume、state setter、错误字符串分流；
- Orchestrator core 不出现 Go、文件名、测试框架或业务领域特判。

任一卡完成必须同步更新 core contract、Orchestrator readme、active overview、unclosed matrix 与直接测试。若发现合同与实现冲突，先更新权威设计再改代码；不得保留两种解释。
