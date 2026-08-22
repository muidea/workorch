# WorkOrch Core Runtime Verification

Status: normative

Type: verification

Last Verified: 2026-08-22

## Purpose

本文定义 Runtime 当前唯一的 core 验证合同。验证对象是四状态机、单 owner、typed EventHub 边界、
canonical decision、无损反馈、sealed dispatch、receipt 与 terminal delivery；禁止的 graph/step/checkpoint 与
一次性 workspace fixture 不能作为通过标准。

## Authority And Scope

core verifier 只验证：

- SM-1～SM-4 的 owner、合法/禁止迁移与跨状态机交接；
- Runtime EventHub contract 的 `ReducerOwner + MachineRole` 与 canonical owner 一致，participant/opaque persistence
  不能伪装成 reducer；正式 state mutation 只能进入 owner-private internal-event dispatcher；
- `ConvergenceState` 是唯一 canonical decision，action/output kind 仅可派生；
- request、candidate、feasibility、operation、drive、delivery 的 stable factual category 与 safe original
  diagnostic 全链路保真；
- candidate request session、attempt、sealed dispatch、operation execution、assessment、governance binding 与
  delivery acknowledgement 保持正交；
- capability receipt outcome、terminal outcome 与 owner lifecycle 各自只有一个权威字段；
- C0～C5、provider ceiling、stable prefix、audit-safe/provider-safe 投影和敏感信息边界；
- workspace/run scope、restart、duplicate/stale/CAS、exactly-once effect 与 projection 单向性；
- core 的领域中立性和 22 运行单元的 pkg / concrete EventHub 边界。

core verifier 不验证 software-dev 特定文件、语言、测试命令、目录或业务 DoD；这些属于 profile verifier。

## Four-State-Machine Mandatory Matrix

| 状态机 | 唯一 owner | 必须验证的合法迁移 | 必须拒绝的迁移 |
| --- | --- | --- | --- |
| SM-1 Run lifecycle | `blocks/run` | activation/continuation/typed terminal fact 先成为 owner-local event，再执行 pending→active、blocked continuation 或 terminal | 外部指定目标 state、读取 Orchestrator action/focus 选择 lifecycle、terminal→active |
| SM-2 authority/focus | `kernel/orchestrator` | prerequisite/feasibility/receipt/assessment/governance fact→internal event→canonical decision；focus/evidence/repair 原子推进 | capability/adapter 选 decision、错误文本选 target、直接写 Run/Workspace/Result |
| SM-3 candidate/dispatch | `kernel/orchestrator` | request session→complete document→single evaluator→persist diagnostic→final/唯一 correction；accepted→seal→execute | incomplete/transport 创建 attempt、attempt 2、adapter 选 correction/terminal、未 seal 执行 |
| SM-4 drive/recovery/delivery | `kernel/orchestrator` | Run fact→run-scoped singleflight→load/re-drive；typed conflict/recovery；Result→Workspace→Run acknowledgement | 重建 SM-2 authority、projection/application 反向驱动、重复 durable effect、写其他 owner state |

每次 Runtime 变更必须标出受影响 SM，并补：transition、invariant、restart/idempotency、stale/duplicate、
budget/guard 以及适用的跨状态机合同测试。

## Owner Resource Matrix

| Resource | Sole owner | Verification must reject |
| --- | --- | --- |
| Workspace candidate/Definition/startup/active slot | Workspace Block | projection/Run/Application direct mutation or unresolved activation |
| Run lifecycle/opaque record/replacement intent | Run Block | caller target state、Orchestrator lifecycle choice、duplicate successor |
| Result formal outcome | Result Block | Result recomputing acceptance or generic entity write bypass |
| Decision/Approval | corresponding Block | unbound ticket/resolution or cross-owner repository access |
| capability effect/session | corresponding capability Block | generic executor、unsealed operation、owner receipt rebuilding credit |
| audit/projection | Eventstream/workorchd | replay/reverse repair into owner authority |

Cross-owner regression必须证明 concrete receiving-owner Command/Result、workspace/run scope、missing/wrong result handling，
以及 Send/Post 的语义分离；不得使用 Envelope、map/any、JSON/reflect bridge 或可变资源句柄。

## Contract-Normalization Assertions

### Canonical Decision And Terminal

- aggregate、authority snapshot、transaction、request identity 不持久化独立 `RuntimeAction` 或
  `RequiredOutput.Kind`；
- provider-facing operation mode 只能从 sealed canonical state/output constraints 派生；
- Runtime aggregate 只保存 canonical state + terminal code/summary，不保存第二个 terminal kind；
- SM-4 sealed delivery 使用 `TerminalOutcome.Kind = completed | failed | cancelled | superseded`；
- completed/failed 只来自 sealed SM-2 terminal，cancelled/superseded 只来自 SM-1 typed administrative fact；
- terminal delivery identity/acknowledgement 不保留 action 或第二份 administrative kind；
- Run/Workspace/Result 不解释 Orchestrator decision，只执行各自 owner-local typed outcome transition。

### Feedback Preservation

- concrete Result 同时携带本 domain stable category 与 producer 的 audit-safe original diagnostic；
- original class/code/message/signature、field violations、target/revision、scope 与 details ref/hash 在
  normalize/clone/hash/persistence/EventHub/projection 中不被覆盖；
- producer 没有 diagnostic 的普通 accepted fact 才允许 original 为空；
- unknown code 保留 original 并 typed fail-fast，不能默认 retry、CAS、repair、governance 或 terminal；
- provider-safe correction projection 与 audit-safe original diagnostic 分开生成，互不覆盖；
- credential、endpoint、raw prompt/provider body/stream、未脱敏文件内容只允许记录 hash/bytes 与
  redacted/omitted metadata，不得进入 aggregate、eventstream、projection、memory 或下一次 candidate context。

### Candidate And Request

- incomplete document、fragment/EOF 与 transient transport 只使用 request-session recovery，不创建 attempt；
- authentication/configuration/protocol/submission-unknown/continuation-exhausted 形成 factual result，不创建
  candidate attempt；
- complete document 只进入一个 evaluator，first-failure 顺序稳定；
- evaluator 只返回 accepted 或完整 typed rejection，不返回 correction/retry/terminal disposition；
- SM-3 持久化 rejection 后才可形成 typed terminal 或唯一 `correctable_once` internal result；
- attempt 1 复用 transaction、manifest、C0～C3 与 stable prefix；第二次 complete rejection terminal，零第三次请求；
- owner feasibility 不进入 candidate correction 或 attempt ledger。
- request session + authority transaction + canonical state/RequiredOutput + DocumentKind/ExpectedDocument + provider
  ceiling/reasoning/budget/stable prefix构成唯一purpose-bound semantic invocation seal；不得出现独立SemanticActivation
  repository/lifecycle或assessment通过operations envelope升级role。

### Capability And Receipt

- File/Git/Shell/MCP/SSH/Web 各自拥有 emitted-code total mapping，共享 pkg 不维护全 owner code taxonomy；
- Web execute必须校验Orchestrator producer、完整sealed operation、current claimed receipt fence与exact grant；payload/execution/
  grant drift保持零network call。search redirect同origin，fetch DNS绑定actual dial并在redirect前重验；两者`at_most_once`且
  receipt loss不重复external request；
- feasibility 只返回 feasible/stale/prerequisite_required/infeasible factual result；
  `prerequisite_required` 仅在接收 owner 已定义对应 typed prerequisite acquisition command/result 时合法。
  当前没有该合同的 semantic proof owner 必须保留完整 original diagnostic 并 fail-closed（通常为
  `infeasible`/`unverifiable`），不得把它伪装成 revision stale、退休有效 catalog 或消耗 refresh budget；只有
  携带并精确匹配 expected/observed revision fence 的 `stale` 才能进入有界 catalog refresh。
- operation 以 `RuntimeOperationReceipt.Outcome` 为唯一结果，不存在跨 EventHub `Status + Outcome` 双权威；
- durable no-op 使用 `Outcome=no_op`，删除重复 `NoOp bool`；read-only/validation 的正常零 durable effect 仍为
  accepted，`Changed` 只作 observed effect metadata；
- accepted receipt 不携带 failure，rejected/unknown outcome 保留完整 safe original diagnostic；
- durable effect 使用稳定 ExecutionKey/Fence/owner receipt，restart/duplicate 不重复副作用或改变 diagnostic hash；
- stale/resource revision 独立于 failure/candidate budget；accepted resource change 不消耗 recovery budget。

### Failure, Recovery And Governance

- 只有 repository expected/observed revision 不一致产生 typed concurrent-state conflict 并消费 CAS reload；
- semantic reject、stale identity、candidate session conflict、duplicate/no-op 不得借 `common.ErrConflict` 进入 CAS；
- provider transport、operation outcome recovery、delivery retry 使用独立 persisted ledger；
- recoverable runtime wait只有在typed owner recovery/reconciliation合同、condition、trigger/deadline和budget完整时合法；
  matching trigger幂等re-drive，stale/duplicate trigger零迁移，deadline/budget/guard耗尽typed terminal；
- wait不新增SM-1 state，不创建Decision/Approval，不由Web/TUI/projection/普通Resume驱动，且不存在
  `active + nonterminal + no persisted successor`；
- repair 只读取 typed gap/category，不解析 message、stderr/stdout 或 summary；
- retry/repair exhaustion 与 repeated failure 形成 typed terminal/no-progress fact，本身不能创建 ticket；
- governance 只来自独立封存的 `ExternalDependency/GovernanceRequirement`，不存在无信息 Decision/Approval；
- projection、TUI、Web、eventstream 不根据 error/message 自动 Resume、ticket、action 或 owner state change。

### Execution Budget

- 每个provider physical call、candidate correction、semantic generation、capability operation、repair/reopen和recovery
  consumption都绑定exact logical identity与唯一sub-ledger；
- allocation先于physical call/effect，allocation失败保持零外部动作；
- duplicate/stale/result loss/fresh-process reload不重复扣减；confirmed、unknown/reserved和remaining可确定重建；
- accepted resource invalidation、projection refresh、duplicate drive和CAS reload不消费业务预算；
- unknown consumption必须先走owner reconciliation或bounded wait，不能按zero重放；
- exhaustion只能进入合同定义的wait或typed terminal，不能产生治理ticket；
- product `RuntimeExecutionBudgetView`等于sub-ledger纯派生结果，任何UI/API写回尝试必须拒绝；
- Restart的新budget来自fresh policy，predecessor消费不复制为successor authority。

### Coverage, Freshness And Closeout

- canonical ledger到产品projection的mapping分别产生coverage、assessment、evidence三个维度，不能合并为overall enum/score；
- 内部`partially_satisfied`只关闭明确satisfied CWI；其余逐项仍投影为unassessed/unsatisfied/indeterminate；
- `indeterminate`在当前all-of基线下始终不通过terminal gate，且保留reason、missing evidence/owner fact；任何
  Definition/Profile/LLM/client声明的成功例外都必须在admission或candidate校验时拒绝；
- candidate/accepted/rejected/stale-or-superseded由receipt judgement、diagnostic和binding/freshness lineage纯派生；
- evidence只绑定实际依赖的owner revision/snapshot/fence set/hash；无关owner mutation不扩大stale范围；
- successful closeout包含Definition/Run lineage、逐CWI coverage、accepted evidence、assessment、deliverable/artifact、
  applicable owner revision set和known limitations；Result owner不重判或补字段形成成功。

## Workspace, Restart And Concurrency

- workspace-scoped command/result/feedback 携带 workspace/run，适用时携带 authority/dispatch/operation identity；
- component instance、Service、global/cache/background task 不保存未经 workspace scope 约束的“current workspace”；
- 至少两个 workspace 并发 candidate、receipt、recovery、delivery 时不串 identity、diagnostic 或 singleflight；
- CAS 前/后 crash、owner commit 前/后 crash、response 丢失、duplicate/stale/乱序均零重复副作用；
- Run inventory 只返回 SM-1 state + opaque record presence/revision/ref；Orchestrator load 后 owner-local 分类
  `active_no_dispatch`、`active_sealed_dispatch`、`blocked_governance`、`recoverable_runtime_wait`、
  `terminal_delivery_pending` 并自动恢复；
- 不出现 `active + nonterminal + no scheduled drive`，也不使用人工 Resume 作为恢复机制。
- current-baseline Restart 必须先 CAS predecessor `ReplacementIntent`，再完成 Result→Workspace→Run terminal delivery，
  最后由 Run-private reducer 幂等创建/激活唯一 successor；其他组件不能提前创建或链接；
- terminal delivery 已提交、successor 已创建但 lineage 未提交、lineage 已提交但 successor 尚未激活三个 restart cut point
  均必须在进程重启后收敛到同一 successor，重复 recovery 不得再次创建或激活；
- non-current predecessor 返回 `data_reset_required`，不写 replacement intent、不调用 provider/capability；
- 首次 `file/write` 创建成功后若同 CWI reopen，latest owner catalog 必须使 correction 选择 revision-bound
  `file/edit`/`patch/apply`；重复 create-only write 在副作用前拒绝且不能形成无限 revision/等价操作循环。

## Verification Entry

按变更范围先运行受影响 contract、owner 和 Orchestrator package tests，再执行仓库级 `make verify-boundary-contract`
以及 AGENTS/Makefile 规定的 aggregate test、race 和 vet gate。具体 package/命令清单以源码、Makefile 和 CI 为执行 SoT，
本合同不复制一份易漂移的 inventory。

Boundary verification 必须覆盖：共享 package transition policy、全 owner code 表、error-text routing、Action/Output.Kind
与 Status/Outcome 双权威、terminal action、Runtime reducer-owner/role 漂移、secondary owner 直接 state write、DoD
definition 的 draft/satisfied 影子状态、通用 Envelope/map/any/JSON/reflect bridge、scope 缺失及禁止 raw provider 内容。
测试选择为空或只命中 projection compatibility wrapper 不能算通过。

任何以 graph step、checkpoint 或 incremental artifact projection 为对象的 wrapper，只能验证非权威读侧，不能替代
上述 Runtime typed contract 测试或重新定义通过标准。

## Deployed Evidence Boundary

Core assertions进入 deployed environment时仍必须使用 fresh data、latest build 和至少两个隔离 workspace，且不得人工
修改 owner data/schema、重复 Resume、提高 budget、放宽 decoder 或启用第二执行路径。服务启停、HTTP、projection、
filesystem 和 provider evidence 的环境编排由 [Service Acceptance](../../50-product-interface/service-acceptance.md) 负责；profile-specific artifact、
validation 和 DoD proof 由对应 profile verifier负责。它们只能提供本合同断言的 deployed evidence，不能重新定义 Runtime
transition 或通过标准。

## Boundary

core verifier 不应验证：

- software-dev 特定测试命令、文件名、语言栈或项目结构；
- profile/Soul/Skill/MCP 的具体策略文本；
- 禁止的 StagePacket、NextAction、DispatchNextStep、ExecutionContextPatch、graph checkpoint 或自动 successor；
- 一次性 workspace/run 记录或 raw runtime/provider payload。

## Source Documents

- [Runtime Stage Contract](../runtime-stage-contract.md)
- [Runtime Request And Cache](../runtime-request-cache-note.md)
- [Recoverable Wait](recoverable-wait.md)
- [Execution Budget](execution-budget.md)
- [Result Closeout And Requirement Projection](../../10-domain-model/result-closeout.md)
- [Definition Verification](../definition-convergence/verification.md)
- `internal/modules/kernel/orchestrator/readme.md`
- [Service Acceptance](../../50-product-interface/service-acceptance.md)
- [Software-Dev Profile Verification](../../40-profiles-capabilities/software-dev/verification.md)
