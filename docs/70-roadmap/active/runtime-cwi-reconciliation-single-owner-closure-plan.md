# Runtime 单 Owner、CWI Semantic Contract、合同归一与正向收敛收口计划

Status: active

Type: closure-plan

Last Updated: 2026-08-07

> 2026-08-06 live revision: 本文件以下新增的“Live P0 收口”章节优先于此前已完成项的描述。
> 最新 fresh deployed 验证发现 Semantic Planning 仍有一条旁路直接获取 owner operation admissibility，绕过
> `capability_state -> operation_admissibility` 的统一顺序，导致 `blocks/git` 在未提交 acquisition binding 时终止 Run。
> 在该章节全部完成前，不得将 Runtime 标记为 live_closed。

## 1. 范围与唯一目标

本文件是唯一 active Runtime 收口队列。设计 SoT 为
[`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md)、
`internal/modules/kernel/orchestrator/readme.md` 与
`internal/pkg/workorchruntimecontract`。本计划只定义尚未完成的实现顺序、组件边界和验收门禁。

产品前提不可改变：用户选择 workspace template 并定义 Goal、DoD 与工作目录后，WorkOrch 必须自动完成任务拆解、
proof 规划、执行、修复与验收。普通用户不得被要求理解或填写 `profileContracts`、`evidencePolicies`、target、recipe、
check、metric、artifact 或 Runtime 内部 JSON。

本轮目标是建立一条唯一自动主线，并将它使用的合同收敛为“owner EventHub 合同、Orchestrator
private internal event、owner-neutral pure value”三层：

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

合同收口遵循同一条硬规则：跨 owner 只能提交 typed intent、query、fact 或 receipt；接收方 owner 在自己的
`pkg/events` 定义 topic 与 concrete Command/Result；任何跨 owner Result 都不能携带目标状态、next action、retry、
terminal 或 reducer 指令。只有接收方 Biz 校验 source/scope/identity 后创建的 private internal event 可以触发其 reducer。

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
10. 22-unit framework 分类、Base Biz、lifecycle 与 Runtime-sensitive owner 合同已有回归基线；307 topic 的全量
    EventHub 定义/使用闭环已由
    [归档 EC 记录](../../90-archive/completed/component-event-contract-closure-plan.md) 固化。

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
均已闭环。最新合同审计确认并已关闭 capability execute、LLM provider 与八个 capability owner acquisition
topic/Command/Result 的错误归属；六个 operation admissibility acquisition、operation feasibility 与 semantic-proof feasibility，以及四个 CWI reconciliation query 也已回迁接收 owner。Run/Result/Workspace lifecycle 的接收 owner、行为路径和 canonical owner-neutral value 嵌入已完成，EventHub root 保持 owner-local concrete type；EC 全量定义包审计和验证已经关闭并归档。File target snapshot/selection、workspace manifest、repair-surface confirmation 与 Git
capability-state acquisition 也已完成 owner 回迁。canonical feedback/original diagnostic 已由 R3-F3 归一，
`RuntimeOrchestrator` 薄 wrapper 已删除，`RuntimeController` 也只公开唯一 internal-event mutation dispatcher 与两个 scoped
read。R3-F5 已通过全仓测试/race、console、boundary 与 fresh managed deployed 验收；验收期间暴露并关闭了
application 将 Workspace execution-intent 错绑到自身合同清单的问题。所有 lane caller 现在显式传入唯一接收 owner 的
`Contracts()`，生产代码不再聚合多个 owner 合同，AST 回归会拒绝 owner/topic 错配。当前 Runtime 唯一强制遗留为
R3-E3 fresh LSP Bridge live；全组件 EventHub 合同已由 EC 归档记录关闭。不得将部署侧 provider 未 ready 误记为
Runtime 合同失败，也不得用 R3-F 或 EC 历史绿灯替代 R3-E3 的真实语义验收。

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

### 3.7 合同三层模型

所有 Runtime 合同必须唯一归入下列一层，禁止同一类型同时承担两层职责：

| 层 | 正式落点 | 允许 | 禁止 |
| --- | --- | --- | --- |
| owner EventHub contract | 接收方 Module/Block 的 `pkg/events` | concrete topic、Command、Data、Result、纯 Validate/Clone | reducer、target state、next action、retry/terminal 选择、repository/handle、`any`/map envelope |
| Orchestrator private transition fact | `kernel/orchestrator/biz/runtime` | SM-2/3/4 internal fact、exhaustive dispatcher、reducer input | EventHub topic、跨 owner import/transport、projection DTO |
| owner-neutral pure value | `internal/pkg/workorchruntimecontract` | immutable DTO/enum/identity/schema、normalize/validate/clone/hash、纯 evaluator | EventHub、I/O、CAS/timer、owner adapter、状态迁移或 disposition policy |

共享 value 不取得状态 authority。`RuntimeScope`、authority/dispatch/operation identity 可以被多个 owner 合同嵌入，但
Run、Workspace、Result、Decision、Approval 和 Orchestrator 必须继续拥有彼此独立的 state、internal event 与 reducer。

### 3.8 EventHub 合同归属与迁移矩阵

topic 和 concrete Command/Result 由实际接收并维护资源/能力的 owner 定义；Orchestrator 只导入并发送：

| 当前合同 | 目标 owner/落点 | 目标形态 |
| --- | --- | --- |
| Orchestrator `ExecuteFile*`、patch execute | `blocks/file/pkg/events` | File owner execution command/result；按 typed variant 保留完整参数和 receipt |
| Orchestrator `ExecuteShellOperation` | `blocks/shell/pkg/events` | Shell/managed-validation execution command/result |
| Orchestrator `ExecuteGitOperation` | `blocks/git/pkg/events` | Git owner execution command/result |
| Orchestrator `ExecuteMCPOperation` | `blocks/mcp/pkg/events` | MCP owner execution command/result |
| Orchestrator `ExecuteSSHOperation` | `blocks/ssh/pkg/events` | SSH owner execution command/result |
| Orchestrator `ExecuteWebOperation` | `blocks/web/pkg/events` | Web owner execution command/result |
| provider request/result | `blocks/llm/pkg/events` | provider transport fact；不携带 candidate authority |
| opaque aggregate load/CAS、Run lifecycle apply | `blocks/run/pkg/events` | Run owner command/result；Run 不解释 aggregate |
| terminal outcome save/apply | `blocks/result/pkg/events`、`blocks/workspace/pkg/events`、`blocks/run/pkg/events` | 各 owner 独立 command/result 与 acknowledgement |

迁移直接切换 import、topic 与订阅，不保留 alias、dual subscribe、旧 topic forwarding 或 compatibility bridge。公共 sealed
identity 可以复用 `workorchruntimecontract` 值对象；owner-specific payload、topic 和 handler 不能下沉到共享 pkg。

### 3.9 Owner observation 与原始诊断归一

跨 owner 反馈只保留有限、正交的稳定 outcome：

```text
accepted | stale | infeasible | rejected | transient_failure |
permanent_failure | outcome_unknown | contract_defect
```

细粒度 producer class/code/message 不删除、不改写，统一保存在 `RuntimeFeedback.OriginalDiagnostic`：owner、class、code、
message、signature、operation、target refs、expected/observed revision、ordered field violations、details ref/hash/bytes、
truncated/redacted 必须原样保留。stable category 只供 receiver reducer 的 exhaustive table 使用，不能覆盖原始信息。

`FailureFact`、`RuntimeCandidateFailure`、`ValidationFailureDiagnostic` 可以作为产生端内部 typed fact 保留，但跨 owner、
aggregate audit、Result 和 projection 只传播一份 canonical `RuntimeFeedback`。禁止从 error string 反解析 category，也禁止
同时持久化多套互相可能冲突的 canonical failure。

owner feasibility 当前的 `Feasible + Feedback + Failure + Stale + PrerequisiteRequest` 平行表达应收敛为一个带明确
`Outcome` 的 `RuntimeOwnerObservation`。它可以携带 typed stale、prerequisite request 和 replay proof 细节，但不能携带
“reacquire/retry/terminate”等 Orchestrator action；这些选择只由 SM-2/3 private reducer 完成。

### 3.10 Orchestrator 内部合同面归一

`RuntimeInternalEvent` 继续是 owner-local fact，不迁入 `pkg`。现有 event kind 按职责形成四组可审计词汇：

- activation fact：创建 current aggregate 的已校验 Run fact；
- authority fact（SM-2）：semantic、reconciliation、prerequisite、feasibility、failure、coverage；
- transaction fact（SM-3）：provider session、candidate document/admission、dispatch、receipt、governance resolution；
- delivery fact（SM-4）：lease、recovery budget、terminal delivery acknowledgement、termination。

分组不允许使用 `Payload any` 或 `map[string]any`。每种 fact 仍必须有 concrete typed payload、workspace/run scope、必要的
authority/dispatch/causation identity 与 exhaustive validation。`ApplyInternalEvent` 保持唯一 mutation dispatcher，
`RuntimeTransitionCoordinator` 保持唯一 aggregate CAS boundary。

删除 `RuntimeOrchestrator` 薄 wrapper：`Biz` 直接持有 `RuntimeController` 和 evidence repositories；bootstrap/read/query
通过 Biz 的窄方法完成。将 controller 的历史导出 mutation method 收为 package-private，只公开 construction、唯一
internal-event apply、load 与必要的只读 receipt query。测试必须通过 internal event 或 package-local reducer fixture，
不能要求生产暴露第二 mutation API。

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

### Phase F — 合同归一与 owner-private 编译期固化

Phase F 是当前新增的强制代码收口，不得用 R3-E1/E2 的历史绿灯跳过。

#### F1 — 合同 inventory 与唯一分类

**组件：** 22 个运行单元、`internal/pkg/workorchruntimecontract`、boundary verifier。

1. 每个 Runtime topic/Command/Data/Result 在接收 owner 的 `Contracts()` 中声明 owner、接收 owner、command class、lane、
   side-effect、SM、source/scope/identity、canonical feedback 与迁移目标；Orchestrator internal fact 在 owner-private package
   以 exhaustive switch 分类。`workorchruntimecontract` 的全部生产源码由 package-level purity scan 自动覆盖，不维护导出
   type/value 的生产全局索引。
2. CI 拒绝 owner 不匹配、同 topic 多定义、Orchestrator 代 capability owner 定义 execute contract、共享 pkg 含 topic/I/O/
   transition/disposition、跨 owner `any`/map/JSON/reflect envelope，以及 `internal/pkg/events` 中的 topic/message/version/type
   registry。
3. 每个现存合同必须映射到第 3.7 节的一层；无法归类的合同先停止迁移并作为 architecture defect 处理。

**门禁：** owner-local contract 覆盖率 100%；private fact 常量/dispatcher/classification AST coverage；pure-value 全生产源码
purity scan；owner/topic/import scan；22-unit contract registration test。跨 owner 聚合只允许位于测试/CI，不得形成生产
lookup、dispatcher 或 fallback。

#### F2 — Owner EventHub 合同回迁

**组件：** Orchestrator、Decision、LLM、File、Shell、Git、MCP、SSH、Web、Run、Result、Workspace。

1. 先在接收 owner `pkg/events` 建立 concrete typed command/result 和 contract registration。
2. 同一提交内切换 Orchestrator adapter、owner subscription/handler、tests 与 boundary scan。
3. 直接删除 Orchestrator 旧 capability execute topic/type/mapper；不保留 alias、forwarder 或双订阅。
4. query、feasibility、execute 继续是三类不同权限合同；不得为减少类型将它们合成万能 command。
5. operation converter 若只服务某 owner wire contract，迁入该 owner `pkg/events` 的纯 mapper；authority admission/effect
   policy 仍留在 Runtime core/neutral contract 的正确层。

**门禁：** 每个 owner concrete result、source/scope mismatch、duplicate/stale、Post nil、command lane、receipt identity tests；
`make verify-boundary-contract`。

当前进度（2026-08-03）：File、Shell、Git、MCP、SSH、Web execute slice、LLM provider slice、八个 owner 的 Runtime capability acquisition、六个 owner 的 operation admissibility acquisition、operation feasibility 与 semantic-proof feasibility、四个 owner 的 CWI reconciliation query，以及 File target
snapshot/selection、workspace manifest、repair-surface query、Git capability-state，以及 Decision governance-request slice 已完成代码迁移。对应 execute topic、concrete
Command/Data/Result、operation mapper、provider ceiling/preflight/request 与 File query topic/concrete Command/Result 均由接收
owner 的 `blocks/*/pkg/events` 定义，各 owner Biz 和 Orchestrator/Git sender 已直接切换，Orchestrator 旧定义已删除；
未保留 alias、旧 topic 转发或双订阅。共享层只新增 owner-neutral sealed execution identity。
owner-local contract metadata 已登记 definition owner、实际 receiver、三层归属、SM、effect、source、scope、identity、feedback 与 lane identity；重复 topic/message shape 只在测试聚合校验。生产 `internal/pkg/events` 已删除跨组件 topic/message/version 全局索引，只接受调用方显式提供的 receiver-owner contract slice 计算 lane。boundary verifier 已固化 1 Initiator/4 Module/17 Block、owner-local `pkg/events` 覆盖，以及全部六个 capability execute、LLM provider、八个 capability acquisition、六个 operation admissibility acquisition、operation feasibility 与 semantic-proof feasibility、四个 CWI reconciliation query、上述 File/Git query 与 Decision governance-request owner 的归属。System startup defaults 的三组发送方合同也已分别回迁 Soul、Workspace 与 MCP Registry，`kernel/system/pkg/events` 已删除。Run/Result/Workspace lifecycle 的 receiver 与消费路径也已确认。Orchestrator private facts 已在 owner-private package 以非 registry 的 exhaustive classification 固化，AST 测试从常量源自动核对 dispatcher/classification 覆盖；`workorchruntimecontract` 由 CI 直接扫描全部生产源码的 import、EventHub、repository、external I/O、timer/async 与 transition impurity，不建立导出类型全局清单。因此 F1/F2 的 **Runtime-sensitive scope** 已完成；当时转入 EC 的非 Runtime topic、外部 type alias、生产使用闭环、Post/Send 与递归 payload 类型也已完成全量审计、验证并归档。

#### F3 — Feedback、observation 与 identity 归一

**组件：** `workorchruntimecontract`、capability owner adapter、Orchestrator reducer、Result/projection。

1. 建立 canonical scope/authority/dispatch/operation identity，替换重复字段组但不隐藏必需 scope。
2. 建立单一 `RuntimeOwnerObservation.Outcome`，删除互斥 bool/pointer 的多重 authority 表达。
3. 所有跨 owner failure 转换为 `RuntimeFeedback`；完整 producer diagnostic 原样嵌入，stable category 只用于 reducer。
4. reducer 对所有 stable outcome exhaustive；未知 category 作为 typed contract defect，不能 fallback 到 governance/retry。
5. Result、eventstream、TUI/Web 展示 original diagnostic；摘要是派生 read model，不能覆盖 canonical record。

**门禁：** original field order/multiplicity、details ref/hash、truncated/redacted 保真；unknown outcome、duplicate diagnostic、
projection round-trip、restart reload tests。

当前进度（2026-08-03）：operation feasibility 的六个 capability owner Result 与 owner-neutral observation 已删除
`Feasible` 平行 authority，统一使用 exhaustive `RuntimeOwnerFeasibilityOutcome`；owner-private persisted observation 同步使用
`Outcome`，candidate feasibility ledger 也只持久化该 observation，不再保留第二个 `Feasible bool`。六个 owner wire、
owner-neutral observation 与 persisted observation 均已删除 `FailureFact`；失败只跨 owner 传播一份
`RuntimeFeedback.OriginalDiagnostic`，并删除与 `RuntimeFeedback.StableCategory` 完全重复的 wire/neutral/persisted
`Category` 字段；reducer 只从 canonical feedback 纯读取稳定分类，stale/prerequisite/replay proof 仅作为
outcome-specific detail。reload 回归覆盖原始
target 顺序/重复、field violation 顺序/重复、revision、details ref/hash/bytes 与 truncated/redacted 保真；boundary verifier
已禁止上述 bool、Failure 和重复 diagnostic 字段回归。Run owner 的 opaque aggregate CAS Result 也已删除与
`RuntimeFeedback.StableCategory` 重复的 `Disposition`，SM-4 直接 exhaustive 消费 feedback category。operation receipt
也已删除 `FailureFact` 平行权威，只保留 `Outcome + RuntimeFeedback`；capability owner 在跨 EventHub 前绑定完整
workspace/run/authority/dispatch/operation identity，Orchestrator owner-private adapter/reducer 才重建内部 `FailureFact`。
raw validation stdout/stderr 在 diagnostic conversion 时清空并以 omitted/redacted 留痕，合同校验拒绝任何 raw output
回流。Result/audit、typed port/client、TUI 与 Web 已删除扁平 owner/code/message/revision/details 复制，统一投影一份嵌套
canonical `RuntimeOriginalDiagnostic`，覆盖 field violation 顺序/重复、details bytes、omitted/truncated/redacted 与 typed
target/validation observation round-trip。F3 仍需完成其它跨 owner feedback surface 与 canonical identity 的统一。
四个 capability owner 的 CWI reconciliation Result 已收敛为唯一 canonical observation；batch 删除与 observation
并行保存的 synthetic `OwnerReceiptRefs`，稳定 lineage ref 统一按需由 canonical observation 纯派生。该清理不涉及
operation execution 的真实 owner receipt identity。boundary verifier 已禁止恢复 reconciliation receipt 字段和
owner-local synthetic receipt helper。
八个 capability owner 的 acquisition Result 也已收敛为单一 `RuntimeCapabilityAcquisition`：显式携带
workspace/run/authority scope，freshness observation 改为必需 canonical fact，并删除外层重复 `PrerequisiteID`。
Orchestrator 在提交 SM-2 prerequisite fact 前穷举校验 scope、owner、prerequisite kind、observation 与所请求 capability
class/kinds，拒绝跨 workspace/run/authority replay、空 observation 和 class 外 capability。
六个 operation-admissibility owner acquisition Result 已同步归一为单一 scoped canonical acquisition；删除 Result 外层
重复 `PrerequisiteID`，freshness observation 改为必需值，并在 Orchestrator 边界同时校验 workspace/run/authority、
inventory owner/prerequisite、observation identity、capability-state binding 与 requested operation kinds。
其余五类 owner acquisition（File target snapshot、File target selection、File workspace manifest、LLM provider
ceiling、Git capability-state）也已完成 identity 收口：Result 显式回显 workspace/run/authority scope，freshness
observation 为必需 canonical fact，外层重复 `PrerequisiteID` 已删除；payload 自带 prerequisite identity 时与 observation
严格一致，不自带时由 observation 唯一承载。Orchestrator 在 SM-2 commit 前验证完整 scope、owner、kind、revision、
target/selector/provider/repository-state identity，boundary verifier 禁止旧平行字段恢复。operation execution 的真实
owner receipt 未受影响，也未引入万能 acquisition Result。
LLM provider request Result 也已删除跨 owner `RuntimeCandidateFailure`，只返回 scoped provider-request
`RuntimeFeedback`；authentication/configuration/transient/protocol stable category 与原始 class 穷举对应，workspace/run、
operations authority、LLM owner identity 以及 document/fragment/stale 互斥均在 Orchestrator adapter 校验，未知类别直接
作为合同缺陷拒绝。完整 provider document 的 schema/contract admission 仍只由 SM-3 evaluator 产生，不在 LLM wire
形成第二套 candidate authority；raw provider transport error 不进入 original diagnostic。provider preflight 的旧
`Ready + Reason` 也已删除，统一为 `provider_preflight` domain 的 `accepted | context_integrity` feedback；adapter 穷举校验
scope/authority/owner，且 context-integrity diagnostic 不泄漏 Goal/DoD 原文或 provider 私有内容。provider Result 中未被
Orchestrator 消费且已存在于 sealed audit 的 `RequestID/Status` 复制字段也已删除。
跨 Orchestrator → LLM → provider 的 physical request identity 已统一为 owner-neutral 纯值
`RuntimeCandidateRequestIdentity`；EventHub command/result 与 LLM request/response model 均只携带该 canonical identity，
不再平铺复制 logical request ID、sequence、physical call ID。Orchestrator private request session/internal event 仍按本
owner ledger 结构保存状态字段，不属于跨 owner identity duplication；boundary verifier 已禁止 wire/model 恢复平铺字段。
SM-4 terminal delivery 也已删除 Run owner command 中与 sealed `TerminalOutcome.Code/Summary` 平行的
`FailureReason`；Result → Workspace → Run 三个 owner 只接收同一 terminal identity，Run private reducer 再从该 outcome
派生持久化展示原因，不能由跨 owner 调用方覆盖。
Workspace → Run execution-intent admission 已删除两份 owner Result 中的自由文本 `Disposition + Diagnostic`，统一为
owner-neutral typed `RuntimeExecutionIntentOutcome + RuntimeExecutionIntentReason`；Run/Workspace Biz 仍各自拥有状态迁移，
合同只校验 outcome/reason 组合。application、HTTP/TUI backend 仅从 reason 纯派生展示文本，未知或不匹配组合直接作为
typed contract defect 拒绝，不能进入 SM-1。
Result/Workspace/Run lifecycle synchronous Result 中未被任何跨 owner reducer 消费的自由文本 `Diagnostic` 也已删除；
幂等/陈旧原因继续留在各 owner private transition 内部，wire 只返回当前 owner model、`Transitioned` 与适用的 sealed
delivery acknowledgement/runtime activation disposition，调用方不得从文本重建 owner state。
LLM owner-private `RuntimeCandidateResponse` 中与 sealed provider audit/typed error 重复的 `RequestID/Status` 也已删除；
usage audit 只从 canonical audit status 读取，provider error identity 只由 typed `CandidateRequestError` 保留，不再形成第二套
response authority。
六个 capability owner 的 execute Result 也已删除从 sealed command 直接复制的 `EventID/OperationID/EvidenceKind/SourceRef`；
owner wire 只返回 canonical persisted `RuntimeOperationReceipt`。Orchestrator 在 owner response 校验后，从自身 sealed
dispatch/operation/evidence identity 纯派生 evidence event/source ref，owner 不再回显第二套证据身份。

#### F4 — Orchestrator internal surface 收拢

**组件：** `kernel/orchestrator/biz`、`kernel/orchestrator/biz/runtime`。

1. 删除 `RuntimeOrchestrator` wrapper，`Biz` 直接装配 controller/evidence dependencies。
2. internal event 按 activation/SM-2/SM-3/SM-4 分组；保留 concrete payload 与 exhaustive dispatcher。
3. controller 除 `ApplyInternalEvent` 外的 mutation method 改为 package-private；删除父级一行式平行 mutation facade。
4. 所有外部 typed result 必须先校验 source、workspace/run、authority/dispatch/causation、revision，再构造 private fact。
5. 保持唯一 CAS coordinator；禁止 adapter、drive、projection 或 test-only production helper 直接 Save/Commit aggregate。

**门禁：** transition/invariant/idempotency/restart/stale/duplicate/budget/guard tests；production symbol/import scan 证明无第二
mutation entry；双 workspace concurrent scope test。

#### F5 — 全链验收与失效合同删除

1. 删除旧 topic/type/alias/converter、重复 failure authority、无调用导出 mutation API 和失效文档。
2. 更新 owner readme、`runtime-stage-contract.md`、current-state、contract inventory 与 TUI/Web diagnostic schema。
3. 执行 targeted tests、`make fmt-check`、`make vet`、`make verify-boundary-contract`、`go test -count=1 ./...`、
   `go test -race -count=1 ./...`、`make verify-ci-mvp-file`。
4. fresh deployed 重跑 empty/resume/restart/drift/crash/concurrency/CPU 与 LSP Bridge matrix；不得复用迁移前 artifact 判定
   Phase F live closed。

**门禁：** 所有状态转换只有对应 owner private internal event；所有跨 owner wire contract 由接收 owner 定义；完整原始
诊断端到端保真；没有兼容路径或第二 canonical contract。

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
| R3-E3 | pending | LSP Bridge workspace/template selection | 逐 CWI binding-matched semantic matrix；当前 fresh live 首个 Run 在 Git capability-state binding 提交前旁路请求 admissibility，见 P0-1 | fresh LSP live |
| R3-F1 | code_closed_runtime_scope | Runtime contracts + boundary verifier | Runtime-sensitive EventHub 合同已补 layer、SM、effect、source、scope、identity、feedback、lane identity metadata；Orchestrator private fact exhaustive classification、pure runtime values、唯一 owner lane inventory 已固化。全量 307 topic 使用闭环与递归 message type graph 不属于该绿灯，见 EC plan | `go test ./internal/pkg/eventcontract ./internal/pkg/events ./internal/modules/kernel/orchestrator/biz/runtime -run 'TestRuntimeInternalFactsHaveCompleteOwnerPrivateClassification|TestEventLaneCallersNameReceivingOwnerInventory' -count=1`；`make verify-boundary-contract` |
| R3-F2 | code_closed_runtime_scope | Runtime-sensitive receiver owners | File/Patch、Shell、Git、MCP、SSH、Web execute、LLM provider、Runtime acquisition/feasibility/reconciliation 与 Decision governance-request 已回迁接收 owner；Run/Result/Workspace lifecycle receiver/behavior 已确认，但外部 EventHub type alias 的定义包纯化转入 EC plan | `go test ./internal/modules/blocks/{decision,file,shell,git,llm,mcp,ssh,web,run,result,workspace}/... ./internal/modules/kernel/orchestrator/... -count=1`；`make verify-boundary-contract` |
| R3-F3 | code_closed | `workorchruntimecontract` + adapters/reducer/projection | operation feasibility 六 owner wire Result、neutral/persisted observation 与 candidate feasibility ledger 已统一为单一 typed `Outcome`，并删除与 canonical feedback stable category 重复的 `Category` 字段；operation receipt 已统一为 `Outcome + RuntimeFeedback` 且只在 Orchestrator private reducer 重建 `FailureFact`，六 owner execute Result 只返回 receipt、不再回显 derived evidence identity；LLM provider Result 已统一为 scoped provider-request `RuntimeFeedback`，candidate rejection 只在 SM-3 内产生，provider preflight 也已从 `Ready + Reason` 统一为 scoped feedback；provider physical request identity 已统一为 canonical `RuntimeCandidateRequestIdentity`；SM-4 terminal delivery、execution intent、lifecycle、Drive 与 governance resolution Result 已删除平行自由文本/可推导 disposition 并保留必要 typed CAS outcome；CWI reconciliation 删除 synthetic owner receipt refs；八个 capability、六个 operation-admissibility，以及 File target snapshot/selection/manifest、LLM provider ceiling、Git capability-state acquisition 均统一 scoped canonical fact、必需 freshness observation 与唯一 prerequisite identity；audit/port/client/TUI/Web 使用单一嵌套 original diagnostic 并保留完整安全 typed detail；Result shape 扫描未发现其它平行 free-form feedback authority | contract + scope replay + round-trip + restart + projection tests；`make verify-boundary-contract` |
| R3-F4 | code_closed | `kernel/orchestrator/biz{,/runtime}` | 已删除 `RuntimeOrchestrator` 薄 wrapper，`Biz` 直接装配 `RuntimeController` 与 evidence repositories；bootstrap 只提交 activation private fact；49 个历史 mutation method 已全部私有化，crash/restart 测试也只通过 typed internal-event fixture 写入。生产 controller 导出面严格等于 `ApplyInternalEvent/Load/QueryOperationReceipt`，唯一 CAS coordinator 与 symbol scan 门禁保持。fact 词汇已按 activation、SM-2 authority、SM-3 transaction、SM-4 delivery 分文件固化，完整性测试跨全部 fact 源穷举核对 dispatcher/classification | orchestrator transition/restart/concurrency tests + symbol scan |
| R3-F5 | live_closed | docs/tests/deployed service | 生产 `RuntimeOrchestrator`、第二 controller mutation API、旧测试命名与失效合同聚合已清零。全链验收先捕获 application execution-intent 使用错误 owner contract inventory 的 deployed panic，随后将 application/governance/orchestrator lane helper 收敛为调用点显式唯一 owner inventory，并以 `TestSubmitWorkspaceRunBindsWorkspaceOwnerLane` 和全生产源码 AST 扫描固化。全仓 test/race、console、vet、fmt、boundary、diff check 与 fresh managed File/Shell/MCP convergence 均通过；artifact `/tmp/workorch-r3-f5-managed-20260803c` | `go test ./... -count=1`；`go test -race ./... -count=1`；`make console-build`；`go vet ./...`；`make fmt-check`；`make verify-boundary-contract`；`git diff --check`；`WORKORCH_MANAGED_VERIFY_TARGET=verify-ci-mvp-file WORKORCH_MANAGED_VERIFY_ARTIFACT_DIR=/tmp/workorch-r3-f5-managed-20260803c WORKORCH_MANAGED_VERIFY_KEEP_ARTIFACTS=1 WORKORCH_VERIFY_KEEP_TMP=1 WORKORCH_MANAGED_SKIP_PREFLIGHT=1 ./scripts/verify-workorch-managed-full.sh` |

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
- Orchestrator 替 File/Shell/Git/MCP/SSH/Web/LLM 等接收 owner 定义的 execute/provider topic 与 Command/Result；
- owner 合同迁移期间的 topic alias、dual subscribe、forwarder、兼容 mapper 和 fallback；
- `FailureFact`、candidate failure、validation diagnostic 与 feedback 被同时当作 canonical 跨 owner failure 的路径；
- 跨 owner Result 中的 caller-selected target state、next action、retry 或可由 sealed state facts 推导的 drive disposition；governance resolution 的 `committed | stale_diagnostic | duplicate` 是不可由 convergence state 推导的正交 CAS commit fact，必须绑定 workspace/run/resolution scope 后保留；
- `RuntimeOrchestrator` 薄 wrapper 恢复、controller 对父级暴露的第二 mutation surface 和生产 test-helper mutation API；
- `RuntimeInternalEvent` 的 `any`/map payload 或按 error message 反解析 internal event kind/category 的路径。

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
7. 每个 EventHub topic/Command/Result 都由接收方 owner 定义，并通过 inventory/boundary scan；
8. Orchestrator 只有 private internal event dispatcher 和唯一 CAS boundary 可以改变 SM-2/3/4；其它 owner 同样只有自身
   internal event/reducer 可以改变自身状态；
9. 跨 owner feedback 使用唯一 stable outcome，同时端到端保留完整 original diagnostic；
10. 旧 topic/type/alias/forwarder、重复 canonical failure、兼容路径和第二 mutation API 清零；
11. active overview、current-state、组件 readme、typed contract 与测试不存在冲突或失效字段。

## 11. Live P0 收口：统一 prerequisite 调度与四个状态机唯一 owner（2026-08-06）

### 11.1 Fresh deployed 事实

最新实例已确认：

- WorkOrch `healthz`/`readyz` 正常，projection 为 `ready`；
- ai-proxy catalog 为 `fresh`；默认 `ai-proxy/gpt-5.5` 能力为 `ready`；
- workspace projection 为 `ready`，workspace 及 Run scope 正常；
- `run-001` 在启动后约 0.1 秒终止；
- `candidateAttemptCount=0`、`completedDispatchCount=0`、`acceptedEvidenceCount=0`、`stepDispatched=0`；
- 失败原因为：

```text
runtime_contract_defect:
runtime operation admissibility owner "blocks/git"
has no committed capability-state acquisition binding
```

因此本问题不是 LLM、API Key、catalog readiness 或 candidate JSON 问题，而是 SM-2 prerequisite 时序缺口。

### 11.2 已确认根因

生产路径存在两套 prerequisite 解析入口：

```text
runtime_mainline.go
  prepareRuntimePrerequisites
    runtimeOrderedPrerequisites

runtime_semantic_drive.go
  acquireRuntimeSemanticOwnerCatalog
    resolveRuntimePrerequisite(operation_admissibility)
```

普通主线会尝试排序 capability-state 与 operation-admissibility，但 Semantic Planning 主线发现 owner catalog 后直接解析
`operation_admissibility`，没有复用同一排序和依赖检查。对于 Git owner，admissibility 必须引用已经 committed 的
`RuntimeCapabilityStateSnapshot.AcquisitionBinding`；旁路路径在 snapshot 尚未提交时调用
`runtimeCapabilityStateBinding`，于是 Run 在 candidate 之前直接失败。

该问题属于 Orchestrator 内部调度路径分叉和 SM-2 owner 事实提交顺序不一致，不应通过放宽 binding、生成默认 binding、
跳过 Git acquisition 或让 Git 直接推进 Runtime 来修复。

### 11.3 四个状态机唯一 owner

| 状态机 | 唯一 owner | 允许改变状态的事件 | 其它组件职责 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | Run owner 内部事件 | 保存 Run aggregate、发出 lifecycle facts |
| SM-2 authority/focus/prerequisite convergence | `kernel/orchestrator` | Orchestrator private internal event | 编译/校验/提交 owner facts |
| SM-3 candidate/sealed-dispatch transaction | `kernel/orchestrator` | Orchestrator private candidate/dispatch/receipt events | 生成、封存、授权和记账 |
| SM-4 drive/recovery/projection delivery | `kernel/orchestrator` | Orchestrator private drive/recovery/delivery events | recovery、evidence、delivery 调度 |

`blocks/git`、`blocks/file`、`blocks/shell`、`blocks/llm`、`blocks/mcp`、`blocks/ssh`、`blocks/web`、`blocks/decision`、
`blocks/approval` 与 application/projection 不能改变 SM-1～SM-4。它们只能通过自身 `pkg/events` 中的 typed command/result
返回事实或 receipt。

### 11.4 目标唯一执行链

所有 fresh、resume、restart、recovery 和 semantic planning 路径必须收敛为同一条 Orchestrator 主线：

```text
Activation 编译 minimal RuntimeBootstrap
  ↓
Orchestrator 唯一 prerequisite dispatcher
  ↓
按依赖获取 capability_state
  ↓
Orchestrator private event CAS 提交 capability snapshot/binding
  ↓
按已提交 binding 获取 operation_admissibility
  ↓
Orchestrator private event CAS 提交 owner inventory
  ↓
SM-2 authority/focus convergence 完成
  ↓
SM-3 生成并封存 candidate
  ↓
sealed dispatch
  ↓
capability owner 执行并返回 receipt
  ↓
Orchestrator private event 提交 SM-4 evidence/recovery
  ↓
Orchestrator 发布 immutable lifecycle fact
  ↓
Run owner 内部事件完成 SM-1
```

Semantic Planning 只能消费已提交的 owner catalog、capability snapshot 和 admissibility facts，不能创建第二套 prerequisite
调度器。

### 11.5 代码收口顺序与落点

#### P0-1：统一 prerequisite dispatcher

**落点：** `internal/modules/kernel/orchestrator/biz/runtime_mainline.go`、`runtime_semantic_drive.go`、`runtime_drive.go`、
`biz/runtime/prerequisite.go`

要求：

1. 提取唯一 `dispatchPendingRuntimePrerequisites`（命名可按现有代码约定调整）；
2. `prepareRuntimePrerequisites` 和 Semantic Planning 不再各自解析 owner prerequisite；
3. dispatcher 每次从最新 aggregate 读取 pending，不能使用过期 pending 列表继续发送后续 owner command；
4. capability-state 必须先于同 owner operation-admissibility；
5. 每个 owner result 必须先提交 `RuntimeInternalEventPrerequisiteObserved`，再处理下一个 prerequisite；
6. 所有路径都必须复用同一个 workspace/run/authority scope 校验和 CAS 边界；
7. 未找到 capability binding 时保持 pending 或返回 typed prerequisite gap，不能直接把内部顺序缺陷变成 terminal
   `runtime_contract_defect`。

**门禁：**

- Semantic Planning 与普通主线的 prerequisite 顺序一致；
- Git capability snapshot 未提交时，不得发送 Git admissibility command；
- capability snapshot 提交后，admissibility request 的 binding 必须与 snapshot 完全一致；
- duplicate、stale、CAS conflict、response-loss、restart 均不能重复执行已提交 owner fact。

#### P0-2：Git capability-state owner 合同固化

**落点：** `internal/modules/blocks/git/pkg/events`、`internal/modules/blocks/git/biz`、`git/readme.md`

要求：

1. `AcquireCapabilityStateCommand/Result` 继续由 Git owner 定义；
2. Result 必须返回 canonical `RuntimeCapabilityStateSnapshot` 和 observation；
3. `WorkspaceID`、`RunID`、`AuthoritySnapshotID`、`PrerequisiteID`、`AcquisitionKey` 必须匹配 request；
4. 空仓库也必须返回合法 repository-state snapshot；
5. receipt store 必须幂等，冲突 acquisition key 必须显式失败；
6. Git handler 不得发送 Runtime transition event、修改 Run 或直接写 Orchestrator aggregate。

**门禁：** empty repository、initialized unstaged/staged、same-key replay、changed revision、cross-workspace scope、
response-loss/restart。

#### P0-3：SM-2 prerequisite result reducer 闸门

**落点：** `internal/modules/kernel/orchestrator/biz/runtime/prerequisite.go`、controller/reducer/internal-event dispatcher

要求：

1. result 必须存在于当前 `Pending`；
2. owner、kind、subject、scope、authority、binding 必须匹配；
3. 未知、乱序、过期、重复冲突 result 一律拒绝；
4. 只有 Orchestrator private internal event 可以写 `CapabilityStates`、`OperationAdmissibilities`、`Satisfied`；
5. reducer CAS 成功后才能让 dispatcher 继续处理下一个 prerequisite。

#### P0-4：SM-3 candidate/dispatch 前置闸门

**落点：** `internal/modules/kernel/orchestrator/biz/runtime_mainline.go`、candidate/dispatch reducer、LLM adapter

candidate 前必须满足：

```text
all sealed prerequisites satisfied
capability snapshots committed
operation admissibility committed
provider ceiling committed
focus/target grounding committed
```

否则不得请求 LLM、增加 candidate attempt 或创建 sealed dispatch。dispatch 必须重新校验 authority transaction、snapshot ID、
binding、owner 和 target set。

#### P0-5：SM-4 receipt/evidence/recovery 归一

**落点：** Orchestrator SM-4 reducer、各 capability owner execute Result、result/evidence adapter

能力 owner 只返回 receipt/evidence/failure fact；只有 Orchestrator private event 决定 evidence acceptance、focus advance、
recovery、validation、complete/failed。任何 owner result 不得包含或驱动 `nextAction`、terminal state、retry policy。

#### P0-6：Run lifecycle 隔离

**落点：** `internal/modules/blocks/run`、Orchestrator→Run typed command、application activation adapter

Run owner 只处理 Start/Block/Complete/Fail 等自身内部事件。Orchestrator 只能投递不可变 lifecycle fact；Run owner
校验 scope/identity 后转换为自己的 private internal event，再由 SM-1 reducer 迁移。Git、LLM、File、Shell 或 projection
不得直接发 Run state transition。

### 11.6 合同归一红线

禁止恢复以下实现：

- Semantic Planning 专用 prerequisite dispatcher；
- `operation_admissibility` 在 capability-state 之前执行；
- 默认/拼接/哈希生成的临时 capability binding；
- owner 直接修改 Runtime aggregate 或 Run state；
- application/service/repository 跨 owner 直连；
- `map[string]any`、JSON/reflect authority bridge、Envelope、StagePacket、NextAction、DispatchNextStep；
- 为了 live 通过而放宽 binding、target、receipt 或 evidence gate；
- 将内部顺序缺陷伪装为 Decision/Approval ticket；
- 通过 projection/read model 反向推进任何状态机。

### 11.7 状态机与事件验证矩阵

每个 P0 项必须补齐以下测试类型：

| 类型 | 必须验证 |
| --- | --- |
| transition | 只有 owner private event 能迁移对应状态 |
| invariant | 未 committed binding 不得请求 admissibility；SM-2 未完成不得进入 SM-3 |
| scope | workspace/run/authority/dispatch scope 不匹配即拒绝 |
| idempotency | 相同 acquisition key、receipt、internal event 重放为 no-op |
| stale/duplicate | 旧 revision、旧 binding、乱序 result 不推进状态 |
| crash/restart | CAS 前后、response-loss、重启恢复使用 persisted aggregate |
| budget/guard | 不重复调度、不 busy loop、不增加无界 retry |
| concurrency | 两个 workspace、同 RunID、并发 resume/restart 不互相污染 |
| live | fresh deployed empty/initialized/staged Git matrix 至少完成一次 durable state change |

### 11.8 文件级追踪新增项

| ID | 状态 | 落点 | 验收标准 |
| --- | --- | --- | --- |
| P0-1 | code_closed_pending_live | Orchestrator prerequisite dispatcher | Semantic/normal path 共用唯一 dispatcher，Git state 必先于 admissibility；Semantic Planning 旁路已删除 |
| P0-2 | code_closed_pending_live | `blocks/git` | typed acquisition binding、receipt、空仓库和 revision 变化均通过 fresh live |
| P0-3 | code_closed | Orchestrator reducer/internal events | 现有 reducer binding/owner/scope/pending 闸门与新增顺序回归测试覆盖；仍需 fresh live 验证 |
| P0-4 | pending | SM-3 candidate/dispatch | 未完成 SM-2 时 candidate/dispatch 计数保持 0 |
| P0-5 | pending | SM-4/evidence/recovery | owner receipt 只能经 Orchestrator internal event 推进 |
| P0-6 | pending | `blocks/run` + lifecycle command | Run state 只由 Run owner 内部事件迁移 |
| P0-7 | pending | cross-component contract scan | 无第二 dispatcher、无 owner 直写 aggregate、无跨 owner state transition |
| P0-8 | pending | fresh deployed live matrix | `run-001` 类 Git binding defect 消失，产生 candidate、dispatch、receipt 和有效 state change |

只有 P0-1～P0-8 全部达到 `live_closed`，并且 R3-E3 通过 fresh deployed LSP Bridge matrix，才允许本计划进入 archive。

**本轮代码验证记录（2026-08-06）：**

- `go test ./internal/modules/kernel/orchestrator/biz/... -count=1` 通过；
- `go test ./internal/modules/blocks/git/... -count=1` 通过；
- `go test ./internal/modules/application/workorchd/biz/appservice/activationcore/... -count=1` 通过；
- `go test ./... -count=1`、`go vet ./internal/modules/kernel/orchestrator/...`、`make fmt-check`、
  `make verify-boundary-contract` 通过；
- managed full 验证已完成构建、fmt、vet、全仓测试和 HTTP 检查，但被环境中的 `verify-mcp-stdio` 阶段阻断，
  因此 P0-8/R3-E3 仍不得标记为 `live_closed`；
- 当前没有新的 live positive-convergence 证据，不能以本轮定向测试替代 fresh deployed Git capability-state matrix。

### 11.9 完成判定

一次正向推进必须同时观察到：

```text
activation bootstrap sealed
→ capability-state acquisition event/result
→ operation-admissibility acquisition event/result
→ SM-2 committed revision
→ candidateAttemptCount > 0
→ sealed dispatch
→ capability receipt
→ accepted evidence
→ SM-4 progress
→ Run owner lifecycle event
```

任一环节缺失都只能标记为当前阶段 blocked/failed，并输出 owner、state revision、event revision、prerequisite ID、binding、
dispatch ID 和原始 diagnostic；不能以服务 ready、projection ready、catalog fresh 或单元测试通过替代完整 live 正向证据。

## 12. 单 Owner 状态机与跨组件合同最终收口（R4）

本节是对前述 P0 队列的边界归一补充。它不新增状态机，也不改变
[`runtime-stage-contract.md`](../../00-core/runtime-stage-contract.md) 的状态语义；当本节与旧的
`Apply...` 命名或“跨组件请求迁移”描述冲突时，以本节为准。

### 12.1 唯一 owner 矩阵

| 状态机/资源 | 唯一 owner | 允许迁移入口 | 其它组件只能提供 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `internal/modules/blocks/run` | Run Biz private lifecycle event | activation/continuation/lifecycle fact |
| SM-2 authority/focus convergence | `internal/modules/kernel/orchestrator` | Orchestrator private internal event | definition、owner observation、evidence fact |
| SM-3 candidate/sealed-dispatch | `internal/modules/kernel/orchestrator` | Orchestrator private candidate/dispatch event | LLM candidate、feasibility、operation receipt |
| SM-4 drive/recovery/delivery | `internal/modules/kernel/orchestrator` | Orchestrator private drive/recovery/delivery event | owner acknowledgement、process/recovery trigger |
| Workspace lifecycle/projection | `internal/modules/blocks/workspace` | Workspace Biz private lifecycle event | Run state fact、terminal delivery fact |
| Result lifecycle | `internal/modules/blocks/result` | Result Biz private lifecycle event | terminal material/fact |

SM-2、SM-3、SM-4 可以继续由同一个 Orchestrator owner 持有，但必须使用分离的 internal-event kind、reducer 和 CAS
边界。不能为了“拆组件”把三个状态机拆到不同 Module；那会重新制造跨 owner 状态迁移和共享可变状态。

### 12.2 唯一执行链

所有 activation、resume、restart、recovery 和 semantic planning 必须收敛到：

```text
Activation 编译 minimal RuntimeBootstrap
  -> Run owner 接收 activation/continuation intent
  -> SM-1 owner-local transition + RunStateChanged fact
  -> Orchestrator SM-4 drive
  -> 唯一 prerequisite dispatcher
  -> SM-2 private event 提交 capability/admissibility facts
  -> SM-3 semantic-plan / operations request
  -> SM-2 seal authority/binding
  -> sealed dispatch
  -> capability owner 执行并返回 typed receipt
  -> SM-4 private event 提交 evidence/recovery
  -> Orchestrator 发布 immutable terminal delivery fact
  -> Result -> Workspace -> Run 各自 owner-local transition
```

禁止任何组件根据 projection、LLM 文本、错误 message、旧 Run 状态或工作区猜测结果，直接触发另一个状态机迁移。

### 12.3 外部合同只能表达 intent、fact 或 acknowledgement

跨 owner EventHub 合同必须唯一属于以下三类：

| 类型 | 含义 | 禁止内容 |
| --- | --- | --- |
| Intent | 调用方请求做某事 | 目标状态、next action、retry、terminal |
| Fact | producer 已观察到某事实 | 接收方 reducer 指令、覆盖 credit、推导状态 |
| Acknowledgement | owner 已处理一个封存 identity | 新 authority、动作选择、跨 owner 写入 |

以下命名和形态已经直接迁移为最终合同，不再新增 `Apply`/通用 update 同类合同：

```text
RuntimeGovernanceWaitFact
RuntimeContinuationEligibleFact
RuntimeTerminalDeliveryFact
RunProjectionFact
StoreRuntimeStateRecordCommand（仅保存 opaque Runtime aggregate）
```

目标形态为 `...Observed`/`...Fact` typed contract。接收 owner 的处理顺序必须是：

```text
typed EventHub fact
  -> scope / identity / revision 校验
  -> owner private internal event
  -> owner reducer
  -> owner CAS
  -> owner notification / acknowledgement
```

跨组件合同不得携带 `TargetState`、`NextAction`、`ConvergenceState`、`FocusState`、`DispatchState`、`CoverageCredit`、
`TerminalDecision` 或完整可变 owner entity。Run 保存 Runtime aggregate 时是唯一例外，但该字段必须保持 opaque，Run
只能按 opaque ref/revision 做 CAS，不解释 SM-2/3/4 内容。

### 12.4 组件收口范围

#### 必须改造状态入口的核心组件

1. `internal/modules/kernel/orchestrator`
   - 保持唯一 `submitRuntimeInternalEvent` 和 single transition coordinator；
   - SM-2/3/4 不得互相直接调用 reducer；
   - SM-3 只提交 candidate/request/receipt fact，SM-2 决定 correction、focus、evidence 或 terminal；
   - SM-4 只恢复已持久化 ledger，不重建 authority，不修改其他 owner state。
2. `internal/modules/blocks/run`
   - 外部 lifecycle fact 先转换为 Run private event；
   - Runtime state write 使用专用 `StoreRuntimeStateRecordCommand`；
   - Run 是 SM-1 唯一迁移 owner，不能解析 Runtime aggregate。
3. `internal/modules/blocks/workspace`
   - Run projection、fresh activation slot release、terminal delivery 全部走 Workspace private event；
   - Workspace 自己推导 `Workspace.State` 和 `ActiveRunID`，不接受 Orchestrator target state。
4. `internal/modules/blocks/result`
   - terminal material 只作为事实；
   - Result owner 自己推导 accepted/rejected 并保存 delivery fence；
   - 不参与 SM-2/3/4 决策。

#### 必须做合同核对、不得拥有 Runtime 状态机的组件

```text
blocks/llm
blocks/file
blocks/git
blocks/shell
blocks/mcp
blocks/mcpregistry
blocks/web
blocks/ssh
blocks/decision
blocks/approval
application/workorchd
services/workorch
eventstream / projection / TUI / Web console
```

这些组件只能定义或消费自身 owner 的 concrete typed topic、Command、Result、receipt 和 read model，不得写 SM-1～SM-4，
不得根据 LLM 文本或 projection 选择 Runtime action。

`internal/pkg/workorchruntimecontract` 只保留 DTO、identity、normalize、validate、clone、hash 和纯 evaluator helper，
不得增加状态迁移、CAS、EventHub、timer、retry 或 owner adapter。

### 12.5 当前 live 证据与收口含义

最近 fresh deployed 的 `run-018` 已出现：

```text
semantic_plan_invalid
  -> semantic_plan_correction_required
  -> semantic_provider_transport_exhausted
  -> SM-2 terminal failure
  -> SM-4 terminal delivery
  -> Result -> Workspace -> Run owner delivery
```

其中 LLM 返回的非法 JSON（例如首字符不是 JSON 文档）必须保持为 `semantic_plan_invalid` 原始诊断；provider transport
失败必须作为独立 typed request outcome。两者不能由错误字符串互相覆盖，也不能由 LLM 直接设置 Run failed。当前 run
已能正确终止并完成 delivery，说明 owner-local terminal 路径工作；但尚未产生 candidate、dispatch、receipt 或 durable
state-change，不能标记为正向完成。

因此本节收口的 live 门禁必须同时验证：

1. semantic rejection 和 provider transport outcome 在 SM-3 ledger 中保持独立；
2. correction 只由 SM-3/SM-2 private event 产生，不能由 LLM 或 Run owner 产生；
3. terminal 只由 SM-2 seal，SM-4 只负责 delivery；
4. Result、Workspace、Run 依次以 owner-local event 完成各自迁移；
5. 一个 workspace 的 failure、retry 或 terminal 不会改变另一个 workspace 的状态。

### 12.6 R4 代码收口顺序

| 阶段 | 组件 | 收口内容 | 必须验证 |
| --- | --- | --- | --- |
| R4-1 | Orchestrator | 固化 SM-2/3/4 internal dispatcher、禁止跨 reducer 直调 | transition、scope、CAS、duplicate、restart |
| R4-2 | Run | lifecycle fact -> Run private event；opaque Runtime CAS 合同收窄 | active/blocked/terminal、stale、response-loss |
| R4-3 | Workspace | Run/terminal fact -> Workspace private event | active-run slot、fresh restart、旧 Run 不释放新 Run |
| R4-4 | Result | terminal fact -> Result private event | accepted/rejected、delivery fence、create race |
| R4-5 | LLM + capability blocks | 仅返回 typed feedback/receipt，拆分 semantic rejection 与 transport failure | invalid JSON、timeout、owner transient、receipt matching |
| R4-6 | application/TUI/Web/projection | 清除隐式推进、目标状态 patch 和 projection 驱动 | read-only、intent-only、SSE 重放不推进 |
| R4-7 | contracts | `Apply...` 迁移语义改为 `...Observed`/`...Fact`，清理 alias/dual subscribe | owner/topic/shape AST scan |
| R4-8 | live matrix | 双 workspace 并发、fresh/resume/restart、empty/initialized Git、semantic invalid/timeout/recovery | candidate -> dispatch -> receipt -> evidence -> state_change |

当前代码收口状态（2026-08-07）：

- R4-1 已完成：`RuntimeController.ApplyInternalEvent` 在 reducer/CAS 前强制校验 exhaustive owner-private metadata；
- R4-2 已完成：Run activation、governance wait、continuation、terminal、schema incompatible 全部先进入 Run private lifecycle event；
  pending resume 原有的 `applyLifecycleLocked` 直调旁路已清除；
- R4-3 已完成：Workspace 的 Run projection、fresh activation slot release、terminal delivery 全部先进入 Workspace private lifecycle event；
- R4-4 已完成：Result terminal delivery 先进入 Result private lifecycle event，再由 owner 构造状态并持久化；
- R4-5/R4-6 已完成代码审计并保留现有 typed feedback、transport/rejection 分离和 presentation read-only 回归；
- `verify-boundary-contract` 已增加 Run/Workspace/Result 生产 reducer 绕过扫描，测试代码可直接验证 reducer table，生产代码不允许绕过；
- R4-7 已完成：Run/Workspace/Result lifecycle transport 统一改为 immutable `...Fact` 与 `/fact/` topic；
  `UpdateRunCommand` 已直接切换为只保存 opaque aggregate 的 `StoreRuntimeStateRecordCommand`，旧类型、旧 topic、alias、
  dual subscribe 与 forwarding bridge 均已清零；boundary verifier 会拒绝其重新出现；
- R4-8 必须在包含本轮代码的 fresh deployed 实例上完成，不能由本地测试代替。

### 12.7 R4 完成判定

R4 只有在下列条件全部满足后才能关闭：

- 四个 Runtime 状态机各自只有一个 owner；
- Workspace、Result 的资源状态也只能由各自 owner-local event 迁移；
- EventHub 不再携带接收方目标状态或 reducer 指令；
- Orchestrator 不直接写 Run/Workspace/Result lifecycle；
- Run 不解析 Runtime aggregate；
- LLM/capability owner 不选择 action、retry、coverage 或 terminal；
- projection、TUI、Web、eventstream 不触发状态迁移；
- duplicate、stale、CAS conflict、restart、response-loss、budget guard 和双 workspace 并发测试通过；
- fresh deployed live 至少观察到：

```text
SM-2 committed revision
  -> candidateAttemptCount > 0
  -> sealed dispatch
  -> capability receipt
  -> accepted evidence
  -> SM-4 progress
  -> Run owner lifecycle fact/event
  -> durable state_change
```

在此之前，服务 ready、projection ready、catalog fresh、LLM 请求成功或 terminal delivery 成功都只能作为局部健康证据，
不能替代 Runtime 正向收敛证明。
