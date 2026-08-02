# Runtime 单 Owner、CWI Semantic Contract 自动编译与正向收敛收口计划

Status: active

Type: closure-plan

Last Updated: 2026-08-02

## 1. 范围与唯一目标

本文件是唯一 active Runtime 收口队列。设计 SoT 为
[`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)、
`internal/modules/kernel/orchestrator/readme.md` 与
`internal/pkg/workorchruntimecontract`。本计划只定义尚未完成的实现顺序、组件边界和验收门禁。

产品前提不可改变：用户选择 workspace template 并定义 Goal、DoD 与工作目录后，WorkOrch 必须自动完成任务拆解、
proof 规划、执行、修复与验收。普通用户不得被要求理解或填写 `profileContracts`、`evidencePolicies`、target、recipe、
check、metric、artifact 或 Runtime 内部 JSON。

本轮目标是建立一条唯一自动主线：

```text
Goal / DoD + selected template/profile + workspace facts
  -> minimal typed RuntimeBootstrap
  -> SM-2 requires semantic contract
  -> SM-3 requests and evaluates one typed semantic-plan document
  -> capability owners return bounded feasibility facts
  -> SM-2 seals per-CWI binding/reconciliation contract
  -> existing operation candidate / sealed dispatch / receipt / evidence mainline
```

该流程不新增第五个状态机，不允许 LLM、application、template、projection 或 capability owner 写 coverage，也不放宽
现有 binding/receipt evidence gate。

## 2. 最新事实与根因

### 2.1 已确认完成的基线

下列内容已经 code-closed，只作为回归基线：

1. Runtime 生产主链只有 `WorkspaceExecutionIntent -> Run owner fact -> DriveRuntime`。
2. application 不拥有 Run lifecycle；SM-1 只由 `blocks/run` 内部事件迁移。
3. SM-2、SM-3、SM-4 由 Orchestrator 拥有，并通过不同 internal-event kind 和 reducer entry 写各自字段。
4. presentation slice 与 semantic focus 已分离；四个 validation CWI 可形成四个独立 focus/binding。
5. bootstrap preflight 已能稳定排序并报告全部缺 proof CWI。
6. `resume=current_run`、`restart=workspace_full`、generation/history 与 owner observation 合同已存在。
7. activation failure 的 scheduler action 已能区分 repair/restart，不再把无 active Run 的失败推荐为 resume。
8. candidate raw CWI、assessment claim、`PassedChecks`、exit code、文件/Git 存在和 generic summary 均不能产生 credit。
9. 非 current schema 只产生 `fresh_activation_required`，不迁移、不推导、不进入第二路径。
10. 22-unit framework 边界和 `pkg` purity 已有回归基线。

### 2.2 本轮收口起因与当前状态

本轮起始 live 反例是 fresh LSP Bridge workspace 的 `/run restart` 在创建 Run 前返回
`coverage_binding_unavailable`。直接原因是
`software-dev` 只有 `dodItem/*` 通用质量门禁，没有当前 Goal/DoD 每项所需的 exact target/selector、recipe/check、
metric/artifact declaration。

当时实现用两种错误方式填补这一缺口：

- 要求 workspace template 管理员提前写死当前 workspace 的逐 CWI proof；
- 在普通 Workspace 创建/编辑界面暴露 `contextHints` 与 `evidencePolicies` 原始 JSON，让用户手工补合同。

上述入口现已删除，fresh semantic proposal → owner feasibility → SM-2 seal 与 run-scoped read model 也已完成代码收口。
旧 operation/recovery fixture、managed verifier，以及 empty/resume/restart/drift/crash/concurrency/CPU fresh deployed matrix
均已闭环。当前唯一阻塞是 R3-E3 fresh LSP Bridge 逐 CWI binding-matched live 验收；不得将起始反例继续描述成现行
生产设计，也不得把部署侧 provider 未 ready 误记为 Runtime 合同失败。

### 2.3 根因判定

真正缺失的是 **semantic contract compilation authority**：

- template/profile 只能声明能力词汇、约束、可用 selector/recipe family 和质量门禁，不能预知每个 workspace 的业务答案；
- Goal/DoD 提供业务语义，但 core 不得通过字符串、目录名或 workspace 名称硬编码推导 LSP/语言/文件；
- capability owner 能验证 target、selector、recipe 和工具能力，但不能解释 Goal/DoD 或修改 CWI；
- LLM 可以提出语义拆解，但其输出不是 authority，不能直接写 binding、coverage 或 action；
- 因此必须由 Orchestrator SM-2 在 owner facts 约束下验证并封存 semantic plan。

## 3. 目标合同模型

### 3.1 Bootstrap 两层输入

`RuntimeBootstrap` 必须区分：

1. **Definition seed**：Goal/DoD identity、正文、definition revision、selected template/profile、coverage item identity、
   presentation slice、capability/policy refs。
2. **Sealed semantic contract**：逐 CWI binding、target policy、operation/effect、recipe/check、metric/artifact、owner 与
   reconciliation binding set/hash。它只能在 Run 创建后由 SM-2 seal，不属于 bootstrap 输入。

fresh `restart` 只携带完整 Definition seed 创建 current-schema Run。所有 workspace 都经过同一 persisted semantic
planning、owner feasibility 与 SM-2 seal 主线；activation 不接受预制 sealed binding。

缺 exact proof 不再由 application 直接阻止 Run/LLM；它必须进入 persisted `semantic_contract_required` authority。
Definition seed 自身无效、template/profile 引用不存在、Goal/DoD 为空或 schema 非 current 仍在 activation 前严格拒绝。

### 3.2 单一 semantic-plan document

LLM provider-facing document 只增加一个必要类型：`semantic_plan`。统一 document envelope 只允许：

- `semantic_plan`：为 sealed Definition seed 的每个 required CWI 提出 proof declaration；不含 operation side effect；
- `operations`：消费已封存 semantic contract，进入现有 sealed dispatch 主链。

`semantic_plan` 至少逐 CWI 提供：

- CWI/focus/definition identity；
- owner module 与 owner-declared capability/operation kind；
- `exact_target | existing_owner_scope | prospective_owner_target` 中的一种 target policy；
- exact target 或 owner selector/prospective constraint；
- required evidence/effect；
- validation/behavior 的 recipe/check，适用时 metric/artifact；
- 使用的 template/profile declaration ref 与 capability catalog revision。owner catalog 必须在 provider request 前获取并封存；
  manifest 明确携带 snapshot/revision、operation kinds、target/selector/constraint refs 与 owner-confirmed recipe，禁止
  LLM 先猜 vocabulary、再通过 feasibility rejection 反向发现合同。

禁止字段包括 Runtime action、coverage state、credit、terminal、retry、governance ticket、owner result、receipt 或目标组件状态。

### 3.3 Authority 与封存

semantic-plan document 进入 SM-3 的统一 request-session、complete-document、evaluator 与 attempt-0/1 事务，但不会创建
`SealedDispatch`。SM-3 只提交 `semantic_plan_accepted | semantic_plan_rejected` internal fact；下一步由 SM-2 决定。

SM-2 只有在以下条件全部成立时才能 seal：

1. document 绑定 current workspace/run/definition/seed hash；
2. 每个 required CWI 恰好有一个独立 proposal；
3. kind、variant、parameter、target policy 均属于 template/profile 与 owner catalog 的中立交集；
4. File/Shell/MCP/Git/Web/SSH 等 canonical owner 对自己的 target/selector/recipe 返回匹配 feasibility observation；
5. binding 可稳定 normalize/hash，且完整覆盖 current definition；
6. 无跨 CWI receipt fan-out、wildcard credit、raw claim 或 owner 缺失。

seal 通过 Orchestrator private `semantic_contract_sealed` internal event 原子写入：

- `CoverageFocus.CoverageEvidenceBindings`；
- `RuntimeCWIReconciliationContract` 与 binding set/hash；
- capability/admissibility requirement refs；
- semantic-plan lineage 与 owner feasibility refs。

LLM、application、Run、Workspace、template、capability owner 和 projection 均不能直接写上述字段。

### 3.4 有界失败

semantic planning 必须复用既有 provider request session 与 attempt 预算，不创建无限 repair loop：

- incomplete transport/document：进入独立 request recovery ledger；
- complete contract rejection：最多一次 manifest-bounded correction；
- owner stale/prerequisite：作为独立 SM-2 factual event，不消耗 candidate correction；
- stale/prerequisite observation 必须先单独 CAS；随后以 persisted refresh generation 退休 matching catalog、保存完整
  gap lineage 并回到 `semantic_contract_required` 重获。duplicate generation no-op；新 catalog revision 产生新
  manifest/session，旧 complete session 只留 audit；共享 `prerequisite_refresh` budget 但不共享 candidate attempt；
- attempt 耗尽、无 owner capability或全部 proposal 不可验证：形成完整 typed terminal diagnostic；
- 不创建 Decision/Approval ticket 让用户替系统补技术合同；
- 不在同一 progress signature 下立即重调度，避免 CPU busy loop。

原始 diagnostic 必须保留每个 CWI、field path、actual、allowed/required/forbidden、owner observation、catalog/definition
revision 和安全原始 message；stable category 不得覆盖原始信息。

### 3.5 允许的 internal transition

semantic planning ledger 只是 SM-2/SM-3 aggregate 子域，不是新状态机。外部 EventHub result 必须先校验
workspace/run/definition/seed/request identity，再转换为下表中的 Orchestrator private internal event：

| 当前事实 | private internal event | 唯一可写 owner | 后继事实 | 禁止 |
| --- | --- | --- | --- | --- |
| Definition seed persisted、无 contract | `semantic_contract_required` | SM-2 | planning requirement + manifest identity | application/Run 直接写 binding |
| planning required、无 active request | `semantic_plan_request_started` | SM-3 | persisted request session | SM-2 直接调用 provider |
| complete semantic document | `semantic_plan_evaluated` | SM-3 | accepted proposal 或完整 rejection | document 直接写 coverage/dispatch |
| accepted proposal | `semantic_feasibility_required` | SM-2 | bounded owner probe set | SM-3 选择 owner outcome/action |
| complete matching owner batch | `semantic_feasibility_observed` | SM-2 | seal-ready 或 typed factual gap | owner 修改 proposal/binding |
| seal-ready | `semantic_contract_sealed` | SM-2 | binding set + reconciliation contract + lineage | 部分 seal、跨 CWI fan-out |
| correctable rejection | `semantic_plan_correction_required` | SM-3 | attempt 1 request | attempt 2、换 transaction/seed |
| final rejection/unsupported owner/budget exhausted | `semantic_contract_failed` | SM-2 | typed terminal + original diagnostic | Decision ticket、立即无 lease 重试 |
| sealed contract | existing SM-2 reduce | SM-2 | operation authority | 重新 semantic planning，除非新 restart |

SM-4 只能从 persisted ledger 调度未完成的 request/probe/seal/delivery；它不能创建 proposal、选择 owner、修改 binding
或把 failed 改回 planning。crash recovery 必须恢复 exact request/attempt/batch identity。

### 3.6 最小跨 owner 合同面

为避免继续增加平行合同，只允许以下增量：

1. 现有 Runtime candidate request/result 增加 `DocumentKind=semantic_plan|operations` 与互斥的 typed manifest/document；
   request session、provider transport、complete-document ledger、original diagnostic 和 attempt-0/1 复用同一实现。
2. 新增一组 concrete semantic-proof feasibility command/result，按 canonical owner 投递；command 只携带 sealed
   proposal、catalog/seed identity 和 workspace scope，result 只携带 factual feasibility、revision、bounded target/recipe
   observation 与 original diagnostic。
3. semantic contract seal 只使用 Orchestrator private internal event，不开放 EventHub command。
4. Run owner 继续只保存 opaque Runtime aggregate；不增加 semantic planning service/repository facade。
5. projection 只新增 read model 字段，不新增推进 command。

禁止为每个 CWI、owner、document kind 分别建立重复 request/evaluator/retry DTO；通过 typed union + discriminator 保持
一个 provider transaction 管道，同时保证 semantic document 与 operations document 字段互斥。

## 4. Resume 与 Restart 的唯一语义

### `/run resume`

- 只允许 current non-terminal、current-schema Run；
- 使用该 Run 已 sealed 的 Definition seed 与 semantic contract；
- 触发 `current_run` reconciliation，retain/reopen 当前 credit；
- 不重新解释最新 workspace Goal/DoD，不重新生成 semantic plan，不改变 template/profile；
- binding/definition drift 返回 `definition_changed_requires_restart`；
- duplicate request/generation 幂等，不增加 attempt 或重放 side effect。

### `/run restart`

- 使用 workspace 当前 Goal/DoD、selected template/profile 与当前 owner facts 创建 replacement Run；
- 创建新的 Definition seed，并在首轮 SM-2 执行 semantic contract compilation；
- scope 固定为 `workspace_full`；
- 不继承 predecessor credit、binding、dispatch、repair ledger、candidate attempt 或 governance；
- predecessor 只由 Run owner supersede，replacement 只激活一次；
- 空 workspace 可以通过 owner 允许的 `prospective_owner_target` 规划首个交付物，不要求用户预先创建文件或填写路径。

## 5. Template、Profile 与产品入口

### 5.1 Template/Profile 应声明什么

template/profile 负责声明中立、可复用的编译边界：

- 可用 capability owner、operation kind/variant 和 effect policy；
- target role、允许 discovery source、封闭 selector family 和 prospective-target 约束；
- validation/behavior recipe family、check/metric/artifact schema；
- 通用质量门禁、预算、Soul/Skill/MCP binding；
- provider-facing guidance，要求模型逐 CWI 输出 semantic plan。

template/profile 不得写入某个用户 workspace 的最终文件路径、LSP Bridge CWI matrix、语言列表、业务答案或历史 evidence。
`LSP-Bridge MVP` 仍不是 WorkOrch 内置默认模板。

### 5.2 普通用户入口

普通 Workspace 创建流程只要求：

- template；
- Goal；
- DoD；
- workspace/deliverable directory；
- 可选模型/Soul 等正常产品选择。

从普通 TUI/Web create/edit flow 删除 `contextHintsJSON`、`evidencePoliciesJSON` 和“编辑执行合同”原始 JSON 入口。
管理员的 template/profile CRUD 可以保留 typed contract 编辑能力，但必须提供 schema 校验和领域中立字段说明；它不是
workspace 执行的替代入口，也不能被 TUI/Web 自动填充为当前业务答案。

## 6. 单 Owner 与组件责任

| 组件 | 目标职责 | 禁止 |
| --- | --- | --- |
| `internal/initiators/workorchruntime` | 仅提供 repository provider/factory 与进程级基础设施句柄 | EventHub、Runtime policy、bootstrap export、semantic/状态机 authority |
| `application/workorchd/activationcore` | 构造 Definition seed、纯校验、提交 workspace intent | 调 LLM、查 capability repo、生成最终 target/recipe、迁移 Run/Runtime |
| `blocks/workspace` | Goal/DoD/template/profile 资源与 active slot owner | 解释 CWI、写 Runtime aggregate、选择 action |
| `blocks/run` | SM-1 activation/replacement/terminal owner；CAS 保存 opaque aggregate | 编译 semantic plan、写 SM-2/3/4 字段 |
| `kernel/orchestrator` SM-2 | semantic contract required/feasibility/seal、reconciliation、coverage/focus | 执行 operation、直读 owner repo、从 LLM claim 产生 credit |
| `kernel/orchestrator` SM-3 | semantic-plan/operations request session、document evaluator、operation dispatch | 选择下一 action、修改 coverage、让 semantic plan 产生 side effect |
| `kernel/orchestrator` SM-4 | drive lease、recovery、terminal delivery ack | 重建 contract、选择 CWI/target/candidate |
| `blocks/llm` | 返回绑定 transaction 的 typed document | 选择 retry/terminal/ticket、写 binding/coverage |
| `blocks/file/shell/mcp/git/web/ssh` | capability catalog、feasibility observation、sealed operation receipt | 解释 Goal/DoD、修改 CWI 或 Runtime 状态 |
| `blocks/result/decision/approval`、`kernel/governance` | 各自资源和治理事实 | 代替 semantic contract 修复、Resume/Drive Run |
| TUI/Web/client/projection/eventstream | typed intent、outcome、审计与展示 | 反向驱动 lifecycle、要求普通用户填写 Runtime JSON |
| `internal/pkg/workorchruntimecontract` | DTO、enum、normalize/validate/clone/hash、纯 evaluator helper | EventHub、I/O、CAS、timer、retry/action/terminal 决策 |

所有跨 owner I/O 继续使用 EventHub + concrete typed command/result；外部 fact 必须经 source/scope/identity 校验转换为
receiver-private internal event 后才能触发 reducer/CAS。

22-unit 边界矩阵继续作为本计划的结构门禁；未直接参与 semantic 主线的 owner 也必须保持自身资源边界，不能成为
fallback authority：

| 注册单元 | 本轮边界 |
| --- | --- |
| `internal/initiators/workorchruntime` | repository provider/factory only |
| `internal/modules/application/workorchd` | activation 与协议适配；不拥有 Runtime 状态 |
| `internal/modules/kernel/orchestrator` | SM-2/SM-3/SM-4 与 semantic compilation sole owner |
| `internal/modules/kernel/governance` | governance policy/fact owner；不修复 semantic contract |
| `internal/modules/kernel/system` | startup defaults owner；不推进 Runtime |
| `internal/modules/blocks/workspace` | Workspace/Goal/DoD/template 与 active slot owner |
| `internal/modules/blocks/run` | SM-1 与 opaque aggregate persistence owner |
| `internal/modules/blocks/result` | already-judged outcome owner |
| `internal/modules/blocks/decision` | decision ticket owner；不接 semantic failure |
| `internal/modules/blocks/approval` | approval ticket owner；不接 semantic failure |
| `internal/modules/blocks/eventstream` | audit event owner；projection 不反向推进 |
| `internal/modules/blocks/memory` | memory resource owner；memory 不产生 CWI credit |
| `internal/modules/blocks/skill` | skill resource owner；不解释 Runtime state |
| `internal/modules/blocks/soul` | soul resource owner；不解释 Runtime state |
| `internal/modules/blocks/mcpregistry` | MCP registry resource owner；不写 semantic ledger |
| `internal/modules/blocks/llm` | provider document transport owner；proposal 非 authority |
| `internal/modules/blocks/file` | File feasibility/operation receipt owner |
| `internal/modules/blocks/git` | Git feasibility/operation receipt owner |
| `internal/modules/blocks/shell` | Shell feasibility/operation receipt owner |
| `internal/modules/blocks/ssh` | SSH feasibility/operation receipt owner |
| `internal/modules/blocks/web` | Web feasibility/operation receipt owner |
| `internal/modules/blocks/mcp` | MCP feasibility/operation receipt owner |

## 7. 逐阶段代码收口

### Phase A — 合同与 aggregate 骨架

**组件：** `internal/pkg/workorchruntimecontract`、`kernel/orchestrator/biz/runtime`、Run model/storage。

1. 定义 Definition seed、semantic planning ledger、document kind、proposal、owner feasibility 和 sealed contract lineage DTO。
2. `RuntimeBootstrap.Validate` 只校验 Definition seed；删除 activation 编译/接收 sealed binding 和“缺 exact proof 必须零 Run/LLM”规则。
3. aggregate 增加唯一 semantic planning ledger；直接调整 current schema，不增加旧 schema 路径。
4. reducer 增加 `semantic_contract_required -> semantic_plan_requested -> feasibility_required -> sealed` owner-private transition。
5. progress signature 覆盖 seed、planning request/attempt、proposal hash、owner facts 和 sealed binding set，避免 bookkeeping 空转。

**门禁：** transition table、禁止迁移、hash determinism、invalid schema、duplicate/stale、CAS/restart、budget/guard 测试。

### Phase B — Provider document 与 owner feasibility

**组件：** Orchestrator candidate request/evaluator、`blocks/llm`、File/Shell/MCP/Git/Web/SSH owner。

1. renderer 根据 Definition seed、template policy 和 owner capability catalog 生成 bounded semantic-plan manifest。
2. decoder 只接受 manifest 允许字段；unknown/missing/forbidden 返回完整 path diagnostic。
3. semantic-plan evaluator 与 operations evaluator 共用 request session/rejection primitives，但按 document kind 使用单一入口。
4. canonical owner 提供无副作用 feasibility command/result；owner 不存在或 kind 不支持返回 typed capability failure。
5. SM-2 对完整 batch 做唯一 seal；任一 owner fact 不匹配则不生成半成品 binding。

**门禁：** LLM raw CWI 不产生 credit；unknown owner 不进入替代 owner 路径；stale feasibility 退休旧 proposal；attempt 1 后不发第三次请求。

### Phase C — Activation 与产品入口归一

**组件：** application activation、Workspace/Run intent、TUI/backend、Web、client、projection。

1. restart 用最小 bootstrap 创建 replacement Run；resume 不重编译 semantic contract。
2. 删除普通 workspace create/edit 的 raw contract JSON 字段和 UI。
3. projection 显示 `semantic_contract_required | planning | feasibility | sealed | rejected`，并绑定 request/workspace/run。
4. HTTP `202` 仅表示 transport accepted；最终展示 owner disposition 和完整 diagnostic。
5. recommendation 表只输出可执行动作：definition drift → restart；semantic planning terminal → 修订 Goal/DoD/template 后 restart；
   current resumable Run → resume。

**门禁：** 用户只填 Goal/DoD/template 即可启动；无 running 假状态；TUI/Web 不根据文本错误猜 action。

### Phase D — Reconciliation、execution 与恢复回归

**组件：** Orchestrator SM-2/3/4、Run、Workspace、Result、Decision/Approval/Governance、capability owners。

1. sealed semantic contract 后才允许 operation manifest/candidate。
2. `current_run` 只 retain/reopen；`workspace_full` 零 predecessor credit。
3. owner receipt 必须 exact binding/target/effect/recipe/check/metric/artifact match。
4. terminal delivery 固定 Result → Workspace → Run；单边失败只重试缺失 ack。
5. startup recovery 从 persisted semantic planning/dispatch/delivery ledger 恢复，不重复 provider complete document 或 side effect。
6. delayed successor 必须有 persisted lease；CAS conflict、pending planning 和 owner stale 均不得形成 CPU busy loop。

**门禁：** forged source、workspace mismatch、duplicate/stale、response loss、crash restart、双 workspace 并发和 CPU guard。

### Phase E — Fresh deployed 验收

| 场景 | 必须断言 |
| --- | --- |
| empty software-dev workspace + restart | 用户只给 Goal/DoD/template；自动生成 semantic contract 和 prospective target，形成首个 durable receipt |
| active Run + resume | 同一 Run、同一 sealed contract、`current_run` reconciliation、无 semantic replan、无 replacement |
| definition drift + resume | `definition_changed_requires_restart`，零新 plan/provider/operation/effect |
| terminal predecessor + restart | replacement Run、重新 semantic planning、`workspace_full`、零 predecessor credit |
| invalid semantic proposal | 完整 per-CWI diagnostic、bounded correction、零 operation/ticket/effect |
| missing capability/recipe | typed owner factual failure，不创建无信息 decision ticket |
| duplicate/stale/CAS/crash | request/generation/lease 幂等；complete document 和 durable operation不重放 |
| terminal delivery failure | 各 owner 独立 ack/retry；sealed terminal 不改写 |
| two workspaces concurrent | scope、request、planning ledger、binding、receipt、lease、projection 完全隔离 |
| CPU guard | repeated schedule/CAS/pending planning 无无 lease 循环或持续高 CPU |
| LSP Bridge MVP | 十工具、五语言、unsaved edit、bounded output、性能/并发、build/race、config/docs 各有独立 binding-matched proof |

执行顺序：相关 package tests → `make fmt-check` → `make vet` → `make verify-boundary-contract` →
`go test -count=1 ./...` → `go test -race -count=1 ./...` → `make verify-ci-mvp-file` → fresh deployed matrix。
任一 deployed 条件未完成，本计划保持 active。

## 8. 文件级实施追踪表

状态只允许 `pending | in_progress | code_closed | live_closed`。未通过 fresh deployed 的项不能标为 `live_closed`。

| ID | 状态 | 主要落点 | 交付物 | 最小验证 |
| --- | --- | --- | --- | --- |
| R3-A1 | code_closed | `internal/pkg/workorchruntimecontract` | Definition seed、semantic manifest/document/proposal/feasibility/ledger DTO；纯 normalize/validate/hash | `go test ./internal/pkg/workorchruntimecontract -count=1`；新增文件 import/purity scan |
| R3-A2 | code_closed | `application/.../activationcore` | 已删除生产 activation exact-binding/recipe/reconciliation hard reject、全部旧 proof-binding/preflight helper并生成 seed；bootstrap focus 仅保留 CWI identity、presentation/cursor 与高层验收要求，target/selector/recipe/operation/binding 等 provisional semantic authority 已清零且只能由 SM-2 `semantic_contract_sealed` 写回 | `go test ./internal/modules/application/workorchd/biz/appservice/activationcore -count=1`；`go test ./internal/modules/application/workorchd/biz/appservice/... -count=1`；`go test ./internal/modules/kernel/orchestrator/... -count=1` |
| R3-A3 | code_closed | Run model/repository + Orchestrator aggregate | current-schema semantic planning ledger 独立持久化、clone、CAS/reload；Run 仅 opaque 保存 | `go test ./internal/modules/kernel/orchestrator/... -count=1`；`go test ./internal/modules/blocks/run/... -count=1` |
| R3-A4 | code_closed | `kernel/orchestrator/biz/runtime` | 已加入八个 SM-2/SM-3 private event/reducer entry、attempt 0/1、request/document/owner-batch/seal lineage、幂等/陈旧门禁和 progress signature；统一 request session 已以强制 `DocumentKind` 区分 semantic/operations identity，semantic terminal 前会关闭未完成 provider session；semantic incomplete document 只封存一个 pending lease，duplicate Resume 与双 workspace singleflight/lease 隔离已覆盖 | transition/invariant/duplicate/stale/reload/budget tests；`TestSemanticDocumentPendingSchedulesOnePersistedLeaseWithoutBusyLoop`；`TestScheduleRuntimeDriveSeparatesSameRunIDAcrossWorkspaces` |
| R3-B1 | code_closed | candidate renderer/evaluator + `blocks/llm` | 已以强制 `DocumentKind` 和互斥 typed context 将 semantic document 接入唯一 provider/continuation/cache/audit pipeline；strict decoder、attempt-0/1 provider-safe correction 与 exact result echo 已覆盖 | `go test ./internal/pkg/workorchruntimecontract ./internal/modules/blocks/llm/... ./internal/modules/kernel/orchestrator/biz/... -count=1` |
| R3-B2 | code_closed | orchestrator events + File/Shell/MCP/Git/Web/SSH | 已加入单一 concrete typed semantic-proof feasibility command/result；六个 capability owner 只返回 `feasible/stale/prerequisite_required/infeasible` 事实。owner catalog 以通用 `ValidationRecipeRefs` 封存可执行验证身份，Shell 额外保留完整 command recipe，MCP 从当前 registry 封存 `mcp/tool_call/<server>/<tool>`；File 实查 existing/prospective workspace target，Shell 实查 sealed recipe，MCP/Web/Git/SSH 实查各自 catalog/policy/repository revision；非 File selector 只有同时匹配当前 binding 与 owner catalog snapshot 才满足 target grounding；Orchestrator adapter 严格校验 batch/manifest/seed/proposal/owner identity | `go test ./internal/modules/kernel/orchestrator/pkg/events ./internal/modules/blocks/{file,shell,mcp,git,web,ssh}/biz ./internal/modules/kernel/orchestrator/biz -count=1`；`make verify-mcp-stdio`；owner mismatch/catalog escape/forged catalog snapshot/SSH stale/original diagnostic tests |
| R3-B3 | code_closed | SM-2 seal/reconciliation | 已完成 complete feasible batch 的纯编译器与单次 SM-2 CAS 原子封存，并接入 production semantic drive；owner admissibility catalog 已前置到 provider request；stale/prerequisite 已按 factual-CAS → persisted generation catalog retirement → bounded reacquisition 收口，保留完整 gap/original diagnostic、新 revision 新 manifest/session 与 duplicate/budget/reload 门禁；单 observation CAS-conflict、CAS 后 response-loss/restart、multi-proposal partial-batch response-loss，以及 File/Git 跨 canonical owner 乱序、精确重复、冲突重复与重启 fan-out matrix 均已覆盖 | `go test ./internal/pkg/workorchruntimecontract ./internal/modules/kernel/orchestrator/biz/runtime -count=1`；`TestRuntimeSemanticCrossOwnerPartialBatchIsOrderIndependentAndRecoversAfterResponseLoss` |
| R3-C1 | code_closed | Workspace/Run intent + activation adapters | restart 仅携带由最新 Workspace Goal/DoD/template 编译的 minimal semantic seed bootstrap，并创建 `workspace_full` replacement Run；resume 不携带 bootstrap、不创建 replacement、不重做 semantic planning，固定复用 current Run sealed contract 并将每次 continuation 强制归一为 `current_run`，不得继承该 Run 历史 activation 的 `workspace_full` scope | Run/Workspace/activationcore tests；`TestRuntimeReconciliationRequestSeparatesRestartActivationFromResumeScope` |
| R3-C2 | code_closed | TUI/Web/client | 已从 Web 创建/详情页、HTTP create/update route、typed CLI/remote port/client 与 application create command 删除普通 raw `contextHints/evidencePolicies` 合同输入；template 管理仍是声明 owner；Web acceptance 现展示 run-scoped semantic planning phase、逐 CWI proposal/owner feasibility、完整 rejection/failure diagnostic 与 feasibility gap lineage | `make console-build`；workspace create unknown-field 与 unsupported route tests；readmodel port diagnostic 保真测试 |
| R3-C3 | code_closed | projection/eventstream | application 从 Run owner opaque aggregate 只读映射 semantic planning；投影绑定 workspace/run/definition/state revision，保留 request/manifest/document/batch/set/binding hash、完整 field violations 和有界详情元数据；RuntimeView 丢弃跨 workspace/run/revision 的 stale semantic projection，并在无 Runtime progress 时以 semantic state revision 建立 high-watermark | `go test ./internal/modules/application/workorchd/biz/appservice/readmodelprojection ./internal/modules/application/workorchd/biz/appservice/readmodelports -count=1`；run snapshot HTTP scope/diagnostic test |
| R3-D1 | code_closed | drive/recovery | semantic request session 先持久化，document/probe/seal 通过 private event/CAS 推进；duplicate、stale、CAS conflict、commit 后 response-loss、跨 File/Git 乱序 partial batch 与 fresh controller reload 均复用已提交 lineage，不重放已完成 owner fact/provider document | `go test ./internal/modules/kernel/orchestrator/biz/runtime -count=1`；semantic drive/session tests |
| R3-D2 | code_closed | operation/evidence/terminal delivery | production gate 已强制 sealed contract 后才进入原 operation 主链；旧 operation/recovery 测试夹具已统一通过 test-only Run-owner CAS helper 显式预置 sealed semantic authority，fresh semantic-drive 测试仍走真实 proposal/feasibility/seal；receipt、assessment、candidate retry、prerequisite、focus advance、governance、owner receipt 与 response-loss/crash restart 回归均未放宽 production gate | `go test ./internal/modules/kernel/orchestrator/... -count=1`；`go test ./internal/modules/blocks/run/... -count=1`；`go test ./internal/pkg/workorchruntimecontract -count=1` |
| R3-D3 | code_closed | scheduler/singleflight/lease | pending semantic document 只产生一个 persisted lease；duplicate Resume 不建立第二链；相同 RunID 的双 workspace 以 workspace/run key 隔离；stale/running lease 继续由既有 reducer/restart 回归覆盖 | `TestSemanticDocumentPendingSchedulesOnePersistedLeaseWithoutBusyLoop`；`TestScheduleRuntimeDriveSeparatesSameRunIDAcrossWorkspaces`；drive lease/startup recovery tests |
| R3-E1 | live_closed | managed verifier | mock-provider 已覆盖 File/Shell 与 MCP semantic-plan → owner feasibility → SM-2 seal → operation → owner receipt → terminal positive convergence；provider transport diagnostic 保持 audit-safe | `WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_ARTIFACT_DIR=.ci-artifacts/managed-workorch-semantic-r3 ./scripts/verify-workorch-managed-full.sh` |
| R3-E2 | live_closed | fresh deployed service | empty workspace semantic prospective target → File feasibility → first durable receipt、resume/restart/drift、superseded predecessor termination isolation、active/dispatch/delivery crash recovery、双 workspace 并发与 CPU idle guard 已闭环；empty lifecycle `/tmp/workorch-r3-e2-matrix-20260802i`，concurrent/crash `/tmp/workorch-r3-e2-c7-scripted-20260802e`，CPU `/tmp/workorch-r3-e2-idle-20260802c` | `python3 scripts/verify_runtime_c7_restart_harness.py --artifact-dir /tmp/workorch-r3-e2-matrix-20260802i --binary /tmp/workorch-r3-e2-current5`；`WORKORCH_MANAGED_SKIP_PREFLIGHT=1 WORKORCH_C7_SCRIPTED_ARTIFACT_DIR=/tmp/workorch-r3-e2-c7-scripted-20260802e make verify-runtime-c7-scripted`；`make fmt-check`；`make vet`；`make verify-boundary-contract`；`go test -count=1 ./...` |
| R3-E3 | pending | LSP Bridge workspace/template selection | 逐 CWI binding-matched semantic matrix | fresh LSP live |

每完成一项必须在本表更新状态和证据命令；不得通过删除测试、放宽 binding、写死 LSP 默认或要求用户手工 JSON 将其标记完成。

## 9. 删除与禁止清单

代码收口必须同步删除：

- 普通 workspace create/edit 的 raw `contextHints/evidencePolicies` JSON 输入；
- “缺 exact proof 时要求用户修复执行合同”的产品提示；
- application 在 Runtime 创建前要求 workspace-specific exact binding 的硬拒绝；
- 将 template wildcard 当 semantic credit 的任何路径；
- semantic-plan accepted 后直接创建 operation dispatch 的捷径；
- application/Workspace/Run/capability owner 写 SM-2 planning/binding/coverage 的入口；
- semantic planning failure 创建 Decision/Approval ticket 的替代处理；
- 非 current schema migration、alias、双读、旧字段或历史状态推导。

不得恢复 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch、自动 replan successor、map/JSON/reflect authority
bridge、application continuation queue、跨 owner service/repository 或 projection authority。

## 10. 完成定义

只有同时满足以下条件，本计划才可移入 `docs/90-archive/completed/`：

1. 普通用户只提供 template、Goal、DoD 和 workspace directory 即可启动 fresh Runtime；
2. SM-2 自动封存完整、逐 CWI、owner-confirmed semantic contract；
3. 任何 coverage credit 仍来自 sealed binding + sealed operation + matching owner receipt；
4. resume/restart scope、single-owner transition、restart recovery 和双 workspace 隔离全部通过；
5. TUI/Web 显示真实 planning/outcome/diagnostic，不暴露 Runtime JSON 必填项；
6. LSP Bridge fresh deployed semantic matrix 和 CPU guard 通过；
7. active overview、current-state、组件 readme、typed contract 与测试不存在冲突或失效字段。
