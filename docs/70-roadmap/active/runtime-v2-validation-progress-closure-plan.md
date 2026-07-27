# Runtime V2 Contract Normalization Closure Plan

Status: active

Type: implementation closure plan

Last Updated: 2026-07-26

Execution Revision: R5 — one sealed authorization, one evaluator, finite outcomes

## 1. Purpose

本文是 Runtime V2 代码收口的唯一活动执行队列。目标不是继续增加针对单个 live 错误的校验分支，而是把
owner facts、candidate manifest、decoder、admission、owner feasibility、correction 与 recovery 归一为有限、
可枚举的合同代数，使不稳定 LLM 输出只能进入确定路径。

设计语义以 [Runtime V2 核心合同](../../00-core/runtime-stage-contract.md) 为最高权威；request/cache 以
[Runtime request/cache contract](../../00-core/runtime-request-cache-note.md) 为从属权威；组件落点以
`internal/modules/kernel/orchestrator/readme.md` 和 `internal/pkg/workorchruntimecontract` 为实现 SoT。

本计划取代此前 R4 的 P0～P6/A-4 分段执行正文。旧分段、dated live 日记和临时 artifact 路径不再定义当前
实现顺序；历史内容可通过 Git 历史和 `docs/90-archive/` 检索。当前阶段不做历史 aggregate 兼容，所有代码
验收与 live 验证均使用 fresh Run。

## 2. Current Judgment

当前四状态机边界、Run-owner CAS、sealed dispatch、owner receipt、evidence、terminal delivery 与 Runtime V2-only
主链继续有效。近期 live 持续不能推进的主要原因不是状态机任意选择路径，也不是合同校验本身过严，而是同一
授权事实仍被多处独立解释：

```text
profile / descriptor
  -> prerequisite receipt
  -> manifest compiler
  -> provider schema and prompt
  -> strict decoder
  -> candidate admission
  -> owner feasibility
  -> correction diagnostic
```

成功路径已有 typed manifest 与 receipt，部分拒绝路径仍只有 code/reason 或缺少 bounded allowed set。于是 SM-3
知道 candidate 不可执行，却无法确定它属于一次安全 correction 还是确定终态，最终表现为
`candidate_correction_undefined`、相邻分支反复修复或新的合同违例。

R5 的判断标准是：每个 sealed authority transaction 只有一份 operation authorization；每个 complete candidate
只经过一个 evaluator；每个结果只落入有限结果集；每个状态机只消费自己的结果，不再次解释原始事实。

## 3. Non-Negotiable Boundaries

### 3.1 Four state machines

| 状态机 | 唯一职责 | 本轮允许修改 | 禁止 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | pending/active/blocked/terminal 与窄投影 | 仅消费既有 terminal/wait 结果 | 选择 action、注入 correction、terminal Resume |
| SM-2 authority/focus | prerequisite、owner facts、focus/evidence/repair、manifest authority | 编译并封存 canonical authorization | 调 provider、执行 operation、从文本猜 target |
| SM-3 candidate/dispatch | request session、candidate evaluation、attempt 0/1、seal/receipt | 单 evaluator、统一 rejection/directive/result | 选择下一 Runtime action、改写 owner facts |
| SM-4 drive/recovery | load、singleflight、有界重驱动、terminal delivery | 消费统一结果并恢复 persisted transaction | 重建 authority、重放已知副作用、UI 补偿 |

### 3.2 Component ownership

| 组件 | R5 职责 | 明确禁止 |
| --- | --- | --- |
| `internal/pkg/workorchruntimecontract` | DTO、枚举、normalize/validate/clone/hash、manifest compiler、pure evaluator helper | 状态迁移、CAS、EventHub、I/O、retry/terminal 选择 |
| `orchestrator/biz/runtimev2` | SM-2 reducer 与 SM-3 aggregate transition；调用纯合同并持久化结果 | provider-specific 兼容、跨 owner repository、领域猜测 |
| `orchestrator/biz` | request session、typed EventHub acquisition/feasibility/execution、CAS drive | 重复实现 kind/target/parameter policy |
| LLM block | 从 sealed manifest 渲染 schema/C3，从 rejection/directive 渲染 C4，strict decode | 重建 manifest、扩展 allowed、消费 raw error 决策 |
| capability owner | 生成自身 inventory/selector/feasibility/operation receipt | 选择 retry/action、关闭 coverage、修改 Runtime aggregate |
| Run owner | CAS-persist aggregate 与 recovery inventory | evaluator、action、candidate、evidence quality 判定 |
| TUI/Web/projection | activation、审计和显示 | Resume 补偿、authority rebuild、错误文本路由 |

跨 owner I/O 继续只允许 concrete typed EventHub command/result。不得新增通用 envelope、`map[string]any`、
JSON/reflect bridge、cross-owner service/repository 或新的 framework Module。R5 是既有 Orchestrator 与 owner 的
合同收口，不创建“contract manager”运行单元。

## 4. Canonical Contract Model

### 4.1 One persisted manifest

`RuntimeCandidateContractManifest` 是 candidate transaction 的唯一授权输入。manifest 内每个 operation kind 对应
一条 `SealedOperationAuthorization` 纯值记录；它不是第二个状态对象。记录至少包含：

- `Kind`、`OwnerModuleID`、accepted variants；
- parameter schema、required/forbidden parameters；
- accepted/required effect scopes；
- required coverage/dependency contract；
- `TargetPolicy` 与其 exact refs、bounded scope 或 owner selector；
- prerequisite/admissibility snapshot refs；
- manifest hash 与 operation-contract revision。

renderer、wire schema、decoder、evaluator 和 fixture 只读取该 manifest。owner feasibility 接收 manifest 中与本
operation 对应的 authorization 以及原始 owner snapshot identity，不能从 focus/profile 重新推导另一份规则。

### 4.2 Three target policies

| Policy | Candidate obligation | Owner obligation | 典型失败 |
| --- | --- | --- | --- |
| `exact_target` | 非空 target，全部属于 exact set | 验证 revision 与资源性质 | missing/outside/stale |
| `existing_owner_scope` | 非空 target，全部属于完整 bounded scope | scope 必须完整或是可独立验证的封闭 selector | truncated/outside/new target |
| `prospective_owner_target` | 提交 concrete new target，满足 selector/prospective constraint | 只由资源 owner 判定可创建性 | missing/unmatched/already-conflicting |

禁止将“File scope”“Shell 命令”“Git repository”等具体能力名称写入 evaluator 分支；它们只通过 descriptor、
owner receipt 和 `TargetPolicy` 数据进入 manifest。

### 4.3 Separate budgets

| Budget | 约束对象 | 不得影响 |
| --- | --- | --- |
| `OwnerInventoryBudget` | owner acquisition 返回的完整 inventory 或 selector representation | provider context items、candidate retry |
| `CandidateContextBudget` | C3～C5 的展示/引用数量 | owner authority 完整性、admissibility |
| `RequestRecoveryBudget` | fragment/transport/unknown submission | candidate attempt、repair budget |
| `CandidateAttemptBudget` | complete document 的 attempt 0/1 | transport、stale prerequisite、focus repair |
| `PrerequisiteRefreshBudget` | read-only stale 与 owner receipt resource change 的各自 ledger | provider/candidate/terminal delivery |

任何 inventory 超限场景必须二选一：owner 返回完整的封闭 selector；或在 provider 前返回 typed unresolved。
禁止通过提高固定常量、截断后当完整 scope、或借用 candidate context budget 继续运行。

## 5. One Candidate Evaluator

### 5.1 Input and output

完整 document strict decode 后，只调用一个纯 evaluator：

```text
EvaluateCandidate(
  persisted manifest,
  decoded candidate,
  prior attempt fact,
  typed owner feasibility observations,
) -> accepted | CandidateRejection
```

实现可分内部 pure helper，但外部只有一个入口和一份判定顺序。decoder 只报告 wire/manifest observation；owner
只报告 feasible/stale/infeasible observation；二者都不选择 retry、terminal、governance 或 Runtime action。
为调用 owner probe，Orchestrator 只允许用 kind 解析 manifest 中的 owner/authorization identity，不得在 probe
前实现第二套 admission。unknown kind 零 owner call；可解析 kind 的 probe 必须无副作用，而且 evaluator
仍按第 7 节顺序返回唯一 first-failure，不因 owner 先返回 infeasible 而跳过更早的 field/target 违例。

### 5.2 Candidate rejection

每个 `CandidateRejection` 必须具备稳定 class/code，并在适用时携带：

- operation ID；
- JSON path；
- actual；
- allowed、required、forbidden；
- manifest hash；
- rejected semantic shape hash；
- `CorrectionDisposition`。

`CorrectionDisposition` 只有：

- `correctable_once`：存在唯一、安全、同 manifest 的结构修正；必须生成非空 `CorrectionDirective`；
- `rejected_final`：不存在安全的同合同修正；必须生成明确 terminal code，且零 correction call。

若完整拒绝既没有 directive，也没有 final disposition，它是实现缺陷 `runtime_contract_defect`，不是普通模型
错误。不得保留 `candidate_correction_undefined` 作为 catch-all，也不得创建泛化 decision ticket。

### 5.3 Correction directive

directive 只允许以下纯结构目标：

- `remove_forbidden_field`；
- `supply_required_field`；
- `select_allowed_operation_kind`；
- `replace_target_with_allowed_target`；
- `satisfy_required_coverage`；
- `satisfy_dependency_contract`；
- `change_rejected_semantic_shape`；
- `select_compatible_effect`。

每个 directive 必须可仅凭 manifest + rejection 验证。需要新增 kind、扩大 target、改变 effect authority 或重取
owner facts 的情况不是 correction，应结束当前 transaction，由 SM-2 在新事实上重建 authority。

## 6. Finite Processing Results

provider authentication/configuration 和真实 permission dependency 在 candidate pipeline 之前走既有 typed
dependency/governance 合同。进入候选 pipeline 后只有五种结果：

| Result | Attempt delta | Authority delta | Side effect |
| --- | --- | --- | --- |
| `request_recoverable` | 0 | request session only | 0 |
| `candidate_correctable` | complete attempt 0 failed；最多再开 attempt 1 | 同 transaction/manifest/stable prefix | 0 |
| `candidate_rejected_final` | complete attempt failed | typed terminal | 0 |
| `prerequisite_stale` | 旧 document/attempt retired，不借用 attempt budget | SM-2 精确失效/重取 | 0 |
| `accepted` | complete attempt accepted | persist sealed dispatch | 仅 seal 后 owner operation |

attempt 1 再次 complete rejection 一律原子形成 `candidate_retry_exhausted`；不存在 attempt 2。replay 是
candidate rejection 的一种 diagnostic，不是独立状态机。owner operation receipt、evidence 和 terminal delivery
继续遵循现有 P5 exactly-once 不变量。

## 7. Deterministic Evaluation Order

单 evaluator 的顺序固定为：

1. manifest identity、hash、operation-contract revision；
2. wire/shape、required/forbidden field；
3. operation kind/variant/parameter；
4. target policy；
5. effect、coverage、dependency；
6. semantic replay/prior attempt；
7. owner feasibility observation；
8. accepted。

同一个 candidate 只记录第一个失败边界。后续层不执行，也不得叠加派生错误。该顺序必须由 table-driven test
固定；任何新增 descriptor 字段、target policy、rejection code 或 directive 都必须在同一提交更新枚举覆盖测试。

## 8. Single Serial Closure Queue

只有第一张 `in_progress` 卡允许修改代码。后续卡只能在前一张满足退出条件后开始。

| 顺序 | 卡片 / 状态 | 唯一目标 | 主要 owner / SM |
| --- | --- | --- | --- |
| N0 | **R5 document normalization `done`** | 核心合同、唯一计划、事实/索引与组件矩阵使用同一词汇和顺序 | docs only |
| N1 | **canonical authorization DTO/compiler `done`** | manifest 内只有一种 operation authorization 与三种 target policy | contract pkg；SM-2 input |
| N2 | **owner fact compilation `done`** | prerequisite/admissibility/selector 完整编译为 N1 manifest，预算隔离 | Orchestrator + capability owners；SM-2 |
| N3 | **renderer/decoder closure `done`** | schema/C3/C4 全部由 manifest/rejection/directive 渲染并 strict round-trip | LLM + contract pkg；SM-3 input |
| N4 | **candidate evaluator totality `done`** | 删除分散 admission 路由；所有 complete rejection 二选一 | Orchestrator runtimev2 + contract pkg；SM-3 |
| N5 | **owner feasibility normalization `done`** | owner 只返回 feasible/stale/infeasible observation，并映射统一 rejection | Orchestrator + owners；SM-2/SM-3 boundary |
| N6 | **persistence/recovery matrix `done`** | 五类结果的 CAS、duplicate、stale、restart 与零/一次副作用 | Run + Orchestrator；SM-3/SM-4 |
| N7 | **legacy branch removal `done`** | 删除重复 policy、generic reason routing、旧 helper/fixture 与兼容入口 | all touched owners |
| N8 | **gates and fresh live `done`** | local L1～L5 / managed CI-mvp / P6 scripted 已通过；两条 fresh real-provider 与三态 deployed restart 仍依赖外部部署 fixture（非代码阻塞） | observation only |

### N1 exit

- manifest DTO 可表达所有当前 descriptor、parameter、effect、coverage、dependency 和 target policy；
- compiler 对每个 operation descriptor 恰好产生一条 authorization 或 typed pre-provider failure；
- manifest hash 覆盖全部 authorization 字段；normalize/clone/hash property test 通过；
- pkg 无状态迁移、EventHub、I/O 或 retry 决策；
- 旧 manifest 字段不存在双写/兼容读取。

### N2 exit

- 每个可达 prerequisite/owner inventory 都有 typed producer/consumer；
- incomplete/truncated inventory 不能封存为 scope；超限有 selector 或 pre-provider unresolved；
- owner/candidate/request/recovery budget 类型不互用；
- normal、duplicate、stale 与 CAS 前后 fresh reload 不扩大 authority；
- prerequisite failure 时零 provider/session/attempt/dispatch/ticket/effect。

### N3 exit

- C3 operation contract 只来自 manifest；C4 只来自 rejection/directive；raw reason 不出站；
- renderer、wire DTO、strict decoder 与 fixture round-trip 使用同一 manifest hash；
- attempt 0/1 的 C0～C3、transaction、manifest 与 stable prefix 完全相同；
- unknown field/parameter/kind/target/effect/dependency 都产生 typed observation，不含兼容 fallback。

### N4 exit

- evaluator 判定顺序与本计划第 7 节完全一致；
- 所有 rejection code 均声明 `correctable_once` 或 `rejected_final`；
- correctable 必有完整 directive，final 必有稳定 terminal；两者都不读取 reason；
- replay、target outside、create target missing、action/evidence mismatch、owner-infeasible 均进入同一矩阵；
- `candidate_correction_undefined` 普通路径清零；无法分类只形成 `runtime_contract_defect`。

### N5 exit

- File/Git/Shell/MCP/SSH/Web 的 feasibility command/result 共享统一外层 observation 合同，同时保留 owner 私有
  执行逻辑；
- owner receiver 校验 workspace/run/authority/manifest/authorization/snapshot identity；
- stale 只回 SM-2，infeasible 只回 evaluator，owner 不选择 correction/action/terminal；
- 每个 owner 可单独运行 producer/consumer、scope、stale 与负向测试。

### N6 exit

对五类结果分别覆盖 normal、duplicate、CAS 前失败、CAS 后响应丢失和 fresh-process reload：

- request recovery 不创建 attempt；
- correction 恰好一次，attempt 1 后零第三次 provider call；
- final rejection 零 correction/ticket/dispatch/effect；
- stale 不复活旧 document/session/dispatch；
- accepted 先 seal，owner receipt/evidence exactly once。

### N7 exit

- renderer/admission/owner 不再各自维护 kind/target/parameter/effect allow-list；
- 删除 generic reason-to-route、错误字符串 target 推断、测试专用第二 manifest compiler；
- `rg` 证明无 production compatibility DTO、V1 回流或旧 policy helper；
- active 文档不存在第二执行队列、仍被标记为 current 的旧 P0～P6/A-4 状态或 dated live 过程日记。

### N8 exit

- L1～L5 全部通过；
- 两个新的 workspace/provider activation 各自产生 fresh Run，并收敛到 accepted terminal 或真实 bound
  governance wait；历史 Run、人工 Resume 或修改模型不构成证据；
- `active_no_dispatch`、`active_sealed_dispatch`、`terminal_delivery_pending` 三态在重启后按 persisted aggregate
  收敛，零重复 provider/durable effect；
- projection watermark 一致，terminal authority 后只有 delivery retry；
- artifact 只存 `/tmp/workorch-e2e`，不提交 runtime 数据、raw prompt、provider body、credential 或 endpoint。

## 9. Verification Strategy

R5 不追求覆盖所有业务组合，而覆盖有限合同代数。

### L1 pure algebra

- 每个 operation descriptor 恰好映射一个 owner 和有限 authorization；
- `TargetPolicy × effect × required output × prior attempt` table/property tests；
- rejection totality：每个 code 恰好映射 correctable 或 final；
- manifest、rejection、directive 的 normalize/clone/hash 稳定性。

### L2 producer/consumer

- profile/prerequisite → manifest compiler；
- manifest → LLM renderer/decoder；
- owner command → feasibility observation；
- rejection/directive → C4。

### L3 state transitions

- SM-2 只编译 authority；SM-3 只消费 evaluator result；
- attempt 0/1、stale retirement、seal/receipt/evidence 的合法与禁止迁移；
- 零 provider/ticket/effect 与 exactly-once 断言。

### L4 persistence/restart

- Run-owner file-backed CAS 两侧、duplicate、response loss、fresh actor reconstruction；
- 不复用旧 Hub、Run owner、capability owner、repository adapter 或进程内 response。

### L5 repository gates

```bash
go test ./internal/pkg/workorchruntimecontract ./internal/pkg/llmaudit -count=1
go test ./internal/modules/blocks/llm/... ./internal/modules/blocks/file/... \
  ./internal/modules/blocks/git/... ./internal/modules/blocks/shell/... \
  ./internal/modules/blocks/mcp/... ./internal/modules/blocks/ssh/... \
  ./internal/modules/blocks/web/... -count=1
go test ./internal/modules/kernel/orchestrator/... ./internal/modules/blocks/run/... -count=1
go test -p 1 ./... -count=1
go vet ./...
make verify-boundary-contract
make verify-runtime-v2-positive-convergence-file
make release-check
git diff --check
```

### L6 deployed observation

仅在 N1～N7 完成后执行 `make verify-real-runtime-closure` 与
`make verify-runtime-v2-restart-recovery-live`。live 失败只生成脱敏 trace 并回流第一个未满足的 N1～N7
合同，不直接授权代码 patch。

## 10. Change Discipline

每张卡的变更说明必须包含：

1. 影响的 SM、owner、persisted fact；
2. 删除的旧解释入口，而不只是新增的新入口；
3. 正向、负向和 first-failure 测试；
4. duplicate/stale/CAS/restart/预算/副作用断言；
5. 本卡退出后唯一解锁的下一卡。

发现实现需要新增结果类别、target policy、retry 分支或 framework component 时，必须先修改核心合同并重新
评审，不能在代码内先行加入 fallback。发现某种 business/profile 行为无法映射三种 target policy 时，应留在
profile/capability 层形成 owner selector，不得扩大 kernel 词汇。

## 11. Definition of Done

仅当以下条件全部满足时关闭 RUNTIME-VPC：

1. N1～N8 串行完成；
2. 每个 authority transaction 只有一个 manifest hash 和一组 operation authorizations；
3. renderer、decoder、evaluator、owner feasibility 与 fixture 对 manifest 给出一致解释；
4. 每个 complete rejection 都是 correctable-once 或 rejected-final，没有 undefined fallback；
5. 五类结果的状态、预算和副作用互不串用；
6. restart 不重放 provider 或已知 durable effect，不复活 retired owner facts；
7. pkg 保持 DTO/合同/纯 helper，跨 owner 仍为 concrete typed EventHub I/O；
8. current-state、未关矩阵、todo、active overview、组件矩阵与本计划一致；
9. fresh real-provider 与 deployed recovery artifact 通过。
