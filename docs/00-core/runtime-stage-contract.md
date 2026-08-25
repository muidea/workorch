# WorkOrch Unified Runtime Design Contract

Status: normative

Type: core-contract-index

Last Verified: 2026-08-25

## Purpose

本文是 WorkOrch Runtime 语义的唯一总入口。详细规则按单一专题拆分；下列子合同共同构成规范，不是可任选的参考资料。

## Contract Set

| 合同域 | 规范入口 | 回答的问题 |
| --- | --- | --- |
| Runtime semantics | [Runtime Topics](runtime/overview.md) | Owner、收敛、candidate transaction、四状态机与全局门禁 |
| Definition convergence | [Definition Convergence](definition-convergence/overview.md) | Goal/DoD all-of admission、语义图、调度、clarification 与上下文 |
| Request/cache | [Runtime Request Cache](runtime-request-cache-note.md) | C0～C5、provider ceiling、rendering 与 audit |

Runtime 行为变更必须先读取总入口，再从 Runtime Topics 进入受影响专题；不得只引用本页摘要替代子合同。

## Authority And Ownership

- SM-1 只属于 Run owner：Run lifecycle、continuation 与 administrative termination intent。`run.replacement` 是同一
  Run owner 在 SM-1 下的 subordinate transaction，不是第五个状态机或第二个 lifecycle owner。
- SM-2、SM-3、SM-4 只属于 Orchestrator：authority/focus convergence、candidate/sealed dispatch、drive/recovery/delivery；
  SM-4 的 delivery handoff record 不授予 Orchestrator 任何 SM-1 transition authority。
- Workspace、Result、Decision、Approval、Memory 与 capability owners 只修改自己的正式状态。
- Application 构造 typed bootstrap、提交 Start/Resume/Restart intent、提供协议与 projection；它不拥有 Runtime drive。
- 跨 owner I/O 只使用 EventHub 与接收 owner 定义的 concrete typed command/data/result；禁止 service/repository 直连。
- Runtime topic 标注某个 `StateMachine` 只表示参与该机器；`ReducerOwner + MachineRole` 必须分别固定唯一 reducer
  owner 与 participant/owner ingress/owner observation。EventHub envelope 从不直接调用 reducer，只有 owner-private
  internal event 可以选择 transition；Run 的 opaque aggregate CAS 也不获得 SM-2～SM-4 authority。

## Production Mainline

```text
Workspace owner seals admitted startup input
  -> Application builds typed RuntimeBootstrap
  -> Run owner seals SM-1 activation
  -> Orchestrator SM-4 drives persisted aggregate
  -> SM-2 seals authority, semantic contract, ready focus and evidence binding
  -> SM-3 requests/evaluates candidate and seals dispatch
  -> capability owner returns factual result/receipt
  -> SM-2 credits evidence, settles/reopens CWI or seals terminal
  -> SM-4 delivers Result -> Workspace -> Run
```

administrative cancellation/replacement 使用独立但同方向的交接链：

```text
Run SM-1 administrative termination intent
  -> Orchestrator SM-4 CAS-persists RuntimeTerminationRecord as delivery handoff
  -> SM-4 delivers Result -> Workspace -> Run
  -> Run private SM-1 terminal-delivery event applies cancelled/superseded
```

Recovery is idempotent re-drive from the Run-owned opaque aggregate. Projection、Eventstream、provider text、process-local cache
和历史 event sequence 都不能重建或替代 authority。

SM-4 convergence-terminal delivery 还必须通过 current semantic-authority fence：catalog retirement 后，只要当前 eligible
CWI 仍是 binding-free、semantic generation 尚未 seal，且 reducer 仅因该缺口形成
`target_grounding_unresolved`，SM-4 必须先 re-drive SM-3/SM-2 semantic transaction，不能把该临时 view 投递给
Result/Workspace/Run。current semantic operation authority ready 或 semantic transaction 自身 failed 后才可交付该终态；
其它已封存的 contract/budget/owner failure terminal 不得被重写为 semantic replan。administrative termination 使用独立的
Run-owner intent 与 SM-4 delivery handoff lane，不经过这一 convergence-only 例外；handoff record 本身不修改 Run state。

Runtime 必须区分 genuine governance wait、具备 typed recovery condition/trigger/deadline/budget 的 bounded recoverable
runtime wait 和 terminal failure。recoverable wait 是 SM-2/3/4 aggregate fact 与 SM-4 re-drive responsibility，不新增
SM-1 lifecycle state，也不能留下无后继的 `active + nonterminal` Run。完整记录、owner probe 映射与迁移见
[Recoverable Wait](runtime/recoverable-wait.md)。

provider、candidate、semantic proof、capability operation、repair 与 recovery 的消费按各自 persisted sub-ledger 管理；
产品级 execution-budget view 只确定性汇总，不成为第二份消费 authority。完整合同见
[Execution Budget](runtime/execution-budget.md)。

## Definition And LLM Boundary

- 正式 startup input 是 Goal、DoD、Workspace Template snapshot 与 resolved project workspace directory。
- canonical Goal/DoD aggregation、dependency、gate 与 terminal 只支持 `all_of`；不提供 `one_of`/`any_of` terminal escape。
- 显式 unsupported relation 只能存在于 Workspace owner 的 pre-seal admission candidate，并必须在 Run 前形成 blocking ticket。
- Definition admission 在 promotion 前分别无损编译 Template 与 Workspace 的 `EvidencePolicies`，拒绝空/重复 ID、
  invalid typed policy、normalize cardinality drift 及任何退役 `prospectiveTargetRef`，并把双方 exact policy
  ref/revision/snapshot hash 封存为 candidate 的 `AdmissionPolicyBindings`；final bootstrap 还 exact 比较 admitted
  Workspace policy projection。这些 binding 是 immutable policy authority，不是 exact prospective target authority。
- startup input 封存后允许执行期 LLM 基于 exact revision/hash 处理 semantic proof、candidate 或 bounded gap diagnostic；LLM
  不得改写输入、选择 Runtime action、授予 credit 或裁决 terminal。
- owner-fact acquisition 是同一 CWI 的有界前置 transaction，不是 terminal proof：manifest 必须同时保留 Definition 的
  terminal evidence kinds 与当前 acquisition evidence kinds。acquisition 成功只允许 SM-2 通过 owner-private event 重开
  同一 CWI 并生成 terminal-proof successor；已有 sealed acquisition 时，LLM 的 `undeclared_prerequisite` 必须在 ticket 前
  由 pure manifest admission 拒绝。
- prospective acquisition 还必须用 pre-seal exact contract ref 组合 acquisition-owner hypothetical delta 与 required-owner
  dependency observation。pre-seal Definition 只封存 target policy、declaration selector/candidate refs 与 acquisition
  closure；由其编译的 generation-specific obligation/manifest 再封存 operation selector 与 current owner
  constraints，两者都不封存 exact prospective target。SM-3 semantic proposal 必须在两者交集内提出一个具体
  target，非空 candidate refs 还要求 exact member match；File/Shell
  只返回绑定该 proposal 的 factual feasibility/satisfaction，只有 SM-2 private event 封存的
  `CoverageEvidenceBinding` 首次使该 target 成为 authoritative；operation candidate 只能引用 current binding，
  dispatch 只从该 binding 派生并消费 exact target，不能重新选择。dependency catalog不进入LLM selectable surface。
  typed stale 只走 bounded fact refresh，不消耗 semantic correction。`would_not_satisfy` 必须先作为 exact-current
  File+Shell full composite fact 持久化，且只有 semantic attempt=0 可消耗既有的唯一 correction ledger。finite candidate
  catalog 才允许 pure evaluator 通过“排除 failed target、排除 current owner catalog 已存在 target、逐项重验 manifest”
  证明 exact remaining members；无剩余即 terminal。open selector 不提供可证明的 remaining member，只允许一次
  “不同的非空 target 且仍满足原 selector/constraints”的 relational target-only retry，Orchestrator 不得选择或合成 target。
  两者都保留同一 semantic batch/manifest/order lineage 和 attempt-0 proposal/document/hash 作为 immutable delta baseline，
  只清理 feasibility working set 并由 canonical correction transition 回到 `Requesting`。attempt=1 document 只能替换
  被拒 target；finite replacement 必须属于 exact remaining set，open replacement 必须满足 relational directive，并都要
  重新取得 fresh File feasibility/delta 与 fresh Shell satisfaction fact。attempt=0 时 fixed/singleton finite catalog、
  finite catalog 无剩余合法 target、invalid attempt-1 document，
  或 attempt-1 `would_not_satisfy` 均收口为 `semantic_contract_failed`；第二次 owner rejection 保留 typed original
  reason=`prospective_owner_fact_unsatisfied`，合法 attempt-1 `would_satisfy` 则可继续进入 SM-2 seal。File/Shell 只拥有 factual feasibility/satisfaction，不触发
  correction/terminal/SM-2 transition；该边不新增状态机、跨 owner transition 或可替代 terminal 关系。
- Definition 修订不能被 Resume 吸收；需要新输入时必须显式 Restart，并遵守 replacement/delivery transaction。

## Four Coordinated Machines

| ID | Sole owner | Owns | Must not |
| --- | --- | --- | --- |
| SM-1 | Run | activation、continuation、administrative lifecycle，以及同 owner subordinate replacement transaction | 选择 focus/action、解释 aggregate，或让 SM-4 代写 Run terminal |
| SM-2 | Orchestrator | authority、CWI、binding、credit、assessment、terminal | 调 provider 或执行 capability effect |
| SM-3 | Orchestrator | request session、attempt、candidate、sealed dispatch、receipt transaction | 选择下一 action 或绕过 seal |
| SM-4 | Orchestrator | bounded drive、singleflight、recovery、delivery ledger | 从 projection/history 重建 authority |

## Forbidden Production Surfaces

`StagePacket`、`NextAction`、`DispatchNextStep`、`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher、application
Start/Resume queue、自动 replan successor、checkpoint replay，以及 map/JSON/reflect authority bridge 均不得回到生产合同。

## Change Gate

每次 Runtime 变更必须：

1. 标明受影响的 SM、owner、persisted fact、合法及禁止迁移；
2. 为 owner-defined concrete command/result 覆盖 producer/consumer；
3. 覆盖 normal、duplicate、stale、CAS 前中断、CAS 后响应丢失与 fresh-process reload；
4. 覆盖 budget/guard 和零副作用断言；
5. 同步对应 core 子合同、组件 README、flow 与 verification；
6. 新增或重命名任何 `State | Status | Phase | Disposition | Outcome | Mode | Lifecycle | Stage` string type 时，必须在
   verification-only `scripts/state-model-owners.manifest` 中把它穷尽分类；只有 authoritative/owner-private state 可以绑定
   `scripts/state-transition-owners.manifest` transition。一个 state type 可绑定多个协调 transition，但所有 transition 必须
   回到该 type 的同一个唯一 owner；projection、outcome、disposition、technical mode 与 invariant 不得绑定 transition；
7. 通过 `make verify-boundary-contract` 及受影响测试。

实施进度、候选需求、dated run 和 closure diary 不属于本合同，也不进入 `docs/` 的规范树。
