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

### 2026-07-29：receipt freshness 收口

已关闭一个会把正常执行误判为 recovery 的 SM-2 缺口：accepted durable receipt 的资源 revision 变化不再消耗
run-global recovery budget，也不会形成 `prerequisite_receipt_refresh_exhausted` terminal。Orchestrator 仍是唯一
invalidation、reacquisition、authority transition 与 CAS owner；它为每个受影响 prerequisite 持久化精确
invalidation record，并仅用该 `PrerequisiteID` 的 record generation 生成 capability-state acquisition key。

因此同一 execution receipt 的重放不会重复增长 generation 或重放 owner effect；无关 prerequisite 的变更不影响
该 key；read-only stale/no-progress 仍独占唯一有界的 `prerequisite_refresh` budget。File/Git/Shell 等 Block 只返回
typed receipt/fact，TUI/Web/read-model 仅投影 stale budget 与 invalidation 审计，均不得接管该状态迁移。

### 2026-07-29：Run persistence 与 SM-4 CAS liveness 收口

fresh live 的 `ws-93e2cb765f87dcca/run-003` 暴露了另一条非业务停滞路径：Run record 的无关模型写入会改变
完整 record revision；若该 revision 同时作为 Runtime aggregate 保存前置，Orchestrator 虽未发生 SM-2/SM-3
语义冲突，仍会收到 CAS conflict。CAS reload 记账达到上限后旧实现不再调度 drive，留下
`active + nonterminal + acquire_prerequisite + no scheduled drive`。

收口规则如下：

1. `blocks/run` 仍是 aggregate 的物理 persistence adapter 与 SM-1 lifecycle owner，但不拥有 SM-2/3/4
   语义；`UpdateRunCommand` 只接受 `ExpectedRuntimeStateRevision`，不得携带完整 Run model revision。
2. Run owner 发生物理 record CAS 时，若最新 record 的 RuntimeState revision 仍等于 expected revision，必须在
   最新 record 上 rebase 后重试，以保留并发的 lifecycle/model 字段；若 RuntimeState revision 已改变，才返回
   semantic conflict 给 Orchestrator。
3. Orchestrator 仍是唯一 aggregate reducer/CAS coordinator。CAS ledger 只限制持久化计数，不能许可静止：
   耗尽后 SM-4 保留 owner-local、退避的 reload/re-drive 后继；不得由 TUI/Web/Application/Run Block 发送补偿
   Resume，也不得把 CAS 误判为业务 terminal。

## 2. 最新 deployed 反例与根因

已完成的 SMO-1～SMO-6 是不可回退的代码与测试基线，但不构成 fresh deployed 正向收敛的证明。最新全新 workspace
`ws-bc03e2587d09de64/run-001` 仅 activation 一次、未手改 runtime data、Decision、Approval 或 Resume，得到以下事实：

1. SM-1 正常封存 ready 的 `ai-proxy/deepseek-v4-flash` 并进入 `active`；
2. activation blueprint 正确封存并形成 `go.mod`、`internal/lsp/manager.go`、`internal/lsp/manager_test.go` 的 durable File receipt；
3. 每个副作用均有 `ExecutionKey + Fence`、sealed target、owner receipt 和精确 prerequisite invalidation；没有无信息 Decision/Approval，也没有跨 workspace 串扰；
4. Shell 的 `go test ./...` 返回受封存 scope 约束的 structured failure：`undefined: os` 及 Manager API 缺失，diagnostic 已列出上述三个 target；
5. 随后 SM-2 连续选择 `produce_validation`，SM-3 连续封存 validation-only dispatch，没有新的 durable repair receipt；Run 在 `runtimeRepairAttempts=10` 后以 `no_progress_exhausted` 失败，并完成 `Result -> Workspace -> Run` delivery acknowledgement。

这不是 provider、上下文裁剪、activation coverage 或 CandidateEvaluator correction matrix 的问题。根因是
**可修复 implementation failure 的 repair authority 事实链不完整**：Shell 已返回 structured validation diagnostic，
但该 diagnostic 没有经过当前 File owner 的 target/revision/selector 确认而成为 `FailureTarget`。现有 SM-2 只在
`FailureTarget` 已持久化时才选择 state change，故将实际 implementation/effect gap 降格为 validation gap。

| ID | 根因 | 违反点 |
| --- | --- | --- |
| SMO-REPAIR-AUTHORITY | Shell validation failure 与 File owner 的当前 target/revision 确认之间不存在 typed fact handoff | SM-2 没有 owner-grounded implementation/effect fact，无法合法生成 state-change authority |
| SMO-REPAIR-ROUTING | receipt/diagnostic 到下一 authority 的分类未区分 `implementation_effect_observed` 与 `validation_recipe_or_evidence_gap` | 违反“结构化 gap 类别是下一 action 的唯一依据” |
| SMO-NO-PROGRESS | 相同 failure signature、相同 target revision、零 durable repair receipt 仍重复驱动 validation | SM-4 的无进展 guard 未在错误 authority 路径上及早 fail-fast |
| SMO-TEST | 现有测试只证明普通 validation entry 不会凭文本变成写权限 | 未覆盖“sealed target + File freshness 确认后产生有限 state-change repair authority”的正向矩阵 |

不得以“允许 stderr 直接授予写权限”“把 Shell code 加入 candidate correction matrix”“提高 repair budget”“让 LLM 再试一次”或“自动 Resume”处理该反例。

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
- `validation_failure_observed`；
- `repair_surface_confirmation_observed`；
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

### 4.1.1 Provider candidate wire normalization

SM-3 的 complete document 先以原始 provider bytes 持久化到 C4，原文 hash、fragment 和 request-session
identity 不得因解析便利而改变。C5 在 strict decode 前仅允许调用
`NormalizeRuntimeCandidatePayloadDocument`：它只移除唯一、完整、外层为 `json` 语言标记的 Markdown 围栏，且仅允许围栏外存在空白。

- 裸 JSON 不变；围栏内文本仍由既有 strict JSON decoder 与 sealed manifest 校验。
- 前后 prose、其他语言标签、多个/嵌套围栏、缺失 closing fence、空 body、多个 JSON value、未知字段、参数/target/effect/coverage 违例均不得被兼容或修复。
- normalization 是 `internal/pkg/workorchruntimecontract` 的纯函数；LLM Block 继续透传原始 provider output，Orchestrator 仍是唯一 C5 decode/admission、attempt 与 dispatch owner。
- 已完成的 exact fenced candidate 是 wire 表达归一，不产生 correction attempt；不能安全归一的完整 document 才由 CandidateEvaluator 分类。

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

### 4.4 Validation failure 到 repair authority 的单 owner 交接

本节是 SMO-REPAIR-AUTHORITY 的唯一实施标准。它不放宽 candidate 合同，也不把状态迁移下放到 capability Block。

1. Shell 在执行 sealed validation operation 后只能返回 `RuntimeValidationFailureObservation`：完整 workspace/run/authority/dispatch/operation identity、failure category、signature、sealed validation scope 内的候选 target refs、evidence ref 和有界 diagnostic。该 DTO 是事实，**不得**含 Runtime action、retry、terminal、governance、coverage mutation 或 repair directive。
2. Orchestrator 接收 receipt 后先写入 `validation_failure_observed` 内部事件。它只保存 failure lineage；若类别可能是 implementation/effect，Orchestrator 以 typed File command 请求当前 repair-surface confirmation，不能从 stderr、Goal/DoD 或 LLM 文本猜 target。
3. File owner 只确认事实：目标是否属于 current sealed selector/manifest、是否为 regular target、当前 revision、可修复 target refs 和 observation binding。File 不写 Runtime aggregate、不变更 focus、不选择 action，也不调用 Shell/LLM。
4. Orchestrator 将 File result 映射为 `repair_surface_confirmation_observed`。仅当 signature、workspace/run/authority、sealed scope、target role 和 File revision 都匹配时，SM-2 才把事实归类为 `implementation_effect_observed`，持久化 repair closure 并生成新的 `produce_state_change` authority。
5. `validation_recipe_or_evidence_gap` 保持 `produce_validation`；recipe 不可用形成 `validation_recipe_exhausted`；stale confirmation 仅退休精确 prerequisite 后重取。任何 capability code、diagnostic 文本或 LLM 输出均不能直接选择 action。
6. 新 authority 必须具有新的 `AuthoritySnapshotID` 和新 candidate attempt-0。旧 validation dispatch/attempt 只保留审计 lineage，绝不能转为 attempt-1，也不能复用为 state-change dispatch。

唯一允许的数据流为：

```text
Shell receipt/failure fact
  -> Orchestrator validation_failure_observed
  -> File repair-surface confirmation command/result
  -> Orchestrator repair_surface_confirmation_observed
  -> SM-2 reducer/CAS: new state-change authority
  -> SM-3 sealed dispatch -> File durable receipt
```

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
| `internal/pkg/workorchruntimecontract` | feasibility/evaluation、validation-failure、repair-surface DTO 与纯 validate/hash | transition/CAS/retry/EventHub/I/O/Runtime action | outcome normalization、未知 category/code 拒绝、repair observation identity/hash |
| `kernel/orchestrator/biz/runtimev2` | 唯一 internal-event transition coordinator；SM-2/3 fact handoff、repair authority 归约 | capability I/O、领域/文件名特判 | validation failure→File confirmation→new authority/terminal 全矩阵 |
| `kernel/orchestrator/biz/runtime_v2_mainline.go` 及 adapter | 发起 typed I/O 后投递 internal observation | 直接关闭 candidate attempt、直接写 aggregate 或选择修复 action | production drive 的 receipt/confirmation/next-drive 回归 |
| `application/workorchd/.../activationcore` | 编译 sealed coverage、blueprint、dependency | 迁移 Run/Runtime state、猜测 owner state 或补 repair action | empty workspace bootstrap/DoD coverage matrix |
| `workorchdefaults/defaults/templates.json` | profile/template 声明 state-change + validation blueprint/policy | core 领域分支 | template/profile compilation tests |
| `blocks/shell/biz` | 真实 preflight/effect/validation failure observation；候选 target 必须绑定 sealed validation scope | Runtime action/retry/terminal/repair target state | Shell failure category、receipt restart、无 action 字段 |
| `blocks/file/biz` | manifest/selector/current revision 的 repair-surface confirmation | aggregate/focus/coverage 写入、从 diagnostic 猜 target、选择 action | confirmation freshness、scope mismatch、stale/duplicate |
| `blocks/git/mcp/ssh/web/biz` | 同一 typed feasibility/receipt taxonomy；审计不携带 Runtime state | aggregate/focus/coverage/action 写入 | all emitted owner code category completeness |
| `blocks/llm/biz` | 渲染 sealed C0-C5，其中 C4 仅含 provider-safe repair fact/diagnostic projection | 补 action、重建 authority、从 raw diagnostic 推 target | stable-prefix / new-authority request audit |
| `blocks/run` | SM-1 owner-local activation/block/terminal；opaque aggregate physical CAS rebase | action selection、aggregate transition/recovery | duplicate intent、terminal delivery、model write 不阻断 aggregate save |
| `blocks/result`、`blocks/workspace` | terminal delivery owner-local acknowledgement | acceptance/terminal 裁决 | Result→Workspace→Run idempotency |
| TUI/Web/eventstream | intent、审计、投影 | setter、自动 Resume、错误文本驱动 | no implicit state mutation scan |
| `scripts/verify-boundary-contract.sh` | 静态边界门禁 | 业务分流 | pkg purity、owner-result、UI/application scan、repair fact 无 action、唯一 coordinator CAS |

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
| SMO-7A | validation failure repair-authority owner handoff | completed（code / targeted regression / static gate） | SM-2/SM-3/SM-4、Shell/File | `RuntimeValidationFailureObservation`、File read-only confirmation 与 Orchestrator two-event/CAS handoff 已实现；确认后形成 new authority attempt-0，拒绝 confirmation 在 provider 前 typed fail-fast。fresh deployed 证明移交给 SMO-7。 |
| SMO-7 | restart/concurrency/fresh live | in_progress | SM-1～SM-4 | 第 9 节所有退出条件通过；不得人工 Resume/改 runtime data 补偿 |

C7-B～C7-E、C8-B～C8-F 是 regression baseline，不再作为可独立开始的卡。原 C8-G 被 SMO-1～SMO-7A/SMO-7 取代，因为 live 已证明其前置合同未闭合。

## 8. 强制测试矩阵

### 8.1 合同与状态转换

- 每个 emitted owner code 到 factual category 的 total matrix；未知 code 拒绝在 capability owner；
- CandidateEvaluator 的 rejection matrix 只覆盖其自身可产生的结构性 code；
- candidate rejection、owner infeasible、stale、prerequisite request 互不转换；
- attempt 1 仅发生于完整 `correctable_once` directive；owner infeasible 后零 attempt-1；
- 每个外部结果都经 owner-local internal event 后才增加 `StateRevision`；
- 无 convergence signature 进展的连续 transition typed fail-fast。
- Shell failure observation、File confirmation、Orchestrator internal event 的 workspace/run/authority/dispatch/operation identity 必须完全一致；任一不一致零 aggregate mutation；
- Shell/other capability DTO 无 `RuntimeAction`、retry、terminal、governance、repair directive 或 aggregate 字段；
- `implementation_effect_observed + fresh File confirmation` 必须产生 new-authority attempt-0；同一事实不得进入 candidate correction；
- `validation_recipe_or_evidence_gap` 在无 repair-surface confirmation 时只能 validation 或 typed terminal，绝不因 diagnostic 文本获得 state change。

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
- 无关 Run model/lifecycle revision 与 aggregate save 并发时必须 rebase 并保留双方字段；RuntimeState revision
  变化仍必须返回 semantic CAS conflict；
- CAS reload ledger 耗尽后不得出现 `active + nonterminal + no scheduled drive`，且不得依赖 TUI/Web/Run intent
  补偿或错误地投影业务 terminal；
- terminal delivery 保持 `Result -> Workspace -> Run`，projection 失败不改写 terminal。
- validation receipt 后、File confirmation 前后 crash/restart、duplicate、乱序：最多一次确认命令，零副作用重放，且只由 Orchestrator 内部事件提交状态；
- 相同 failure signature、相同 confirmed revision 且零 durable repair receipt 时，SM-4 必须在再次请求 provider 前 typed fail-fast；不得耗尽普通 validation repair budget。

## 9. Fresh deployed live 退出条件

重新 build/deploy 后创建无历史 workspace/run；不得复用 repair-loop aggregate，不得手工修改 runtime data、Decision、Approval 或以 Resume 修复。

必须连续证明：

1. activation 自动进入 active 并封存 ready provider/model；
2. 空 workspace 的 sealed blueprint/coverage 支持模块、实现、测试等必需交付物；
3. 同一 dispatch 首项 durable effect 后其余 sealed operation 继续；
4. 每个副作用都有 owner receipt，且真实 changed refs 不越出 sealed target；
5. owner feasibility stale、prerequisite、infeasible 分别进入唯一、可观察后继；
6. 可修复的 implementation/effect validation failure 必须经历 Shell fact → File confirmation → Orchestrator internal event，并以新的 state-change authority/attempt-0 进行一次受封存 target 约束的 repair；
7. validation/evidence-quality gap 与 recipe-unavailable 分别保持 validation 或形成 typed terminal，均不产生随机写入、candidate correction 或无信息 ticket；
8. validation 与 assessment 使用 structured observation；
9. restart、duplicate Resume、confirmation 乱序和并发 workspace 不重放副作用/provider，也不串 scope；
10. workspace 独立验证满足 Goal/DoD，Runtime sealed completed terminal，并完成 Result→Workspace→Run acknowledgement；
11. 不出现只增长 revision、相邻合同逐轮暴露、error-text 驱动的循环，或同 signature 的十轮 validation-only 重试。

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
- validation failure observation 不得直接构成 repair authority；只有经当前 File owner confirmation 的 immutable fact 才可被 Orchestrator reducer 消费；
- mainline/adapter 不直接调用 aggregate transition persistence；
- 只有 Orchestrator coordinator 写 SM-2/SM-3/SM-4 aggregate；
- 只有 Run/Workspace/Result owner 写各自 lifecycle；
- application/TUI/Web/eventstream 无自动 Resume、state setter、错误字符串分流；
- Orchestrator core 不出现 Go、文件名、测试框架或业务领域特判。

任一卡完成必须同步更新 core contract、Orchestrator readme、active overview、unclosed matrix 与直接测试。若发现合同与实现冲突，先更新权威设计再改代码；不得保留两种解释。
