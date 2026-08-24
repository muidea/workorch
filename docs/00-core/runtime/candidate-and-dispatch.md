# Runtime Candidate And Dispatch Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-24

## 规范化候选合同

Runtime 不要求穷举所有模型输出组合。它要求把动态 owner 事实先编译成一份有限、可验证的
`RuntimeCandidateContractManifest`，此后 renderer、decoder、candidate evaluator、owner feasibility 和
fixture 都只能消费这份封存合同，不能再次解释原始 profile、目录、错误文本或临时上下文。

每个 manifest 由零到多个 `SealedOperationAuthorization` 组成。该名称表示 manifest 内的一条纯值授权记录，
不是新的状态机或 owner；实现可在现有 manifest DTO 内表达。每条授权至少包含：

- canonical operation kind、owner module、parameter/variant contract；
- accepted/required effect scope、dependency 与封存的 `CoverageEvidenceBinding` 引用；每个可关闭
  CWI 的 binding 必须精确绑定 CWI、proof shape、target/selector、validation recipe/check（适用时的 metric/artifact）
  与来源 declaration/snapshot，不能用 candidate 自报 CWI 代替；
- 唯一 `TargetPolicy` 以及该策略所需的 exact refs、bounded owner scope 或 owner selector；
- 来源 prerequisite/owner receipt 的 snapshot/revision 引用；
- 仅包含本授权实际依赖的bounded owner revision/snapshot/fence set及其deterministic hash；无关owner revision不得进入
  freshness identity或使该authorization失效；
- manifest hash 与 operation-contract revision。

目标策略只允许三种，禁止为 live 症状增加第四种临时分支：

| TargetPolicy | 含义 | 必须拒绝 |
| --- | --- | --- |
| `exact_target` | operation 的全部 target 必须属于封存的精确 target set | 空 target、越界 target、运行时猜测 target |
| `existing_owner_scope` | operation 必须提供 concrete target，且每个 target 属于 owner 封存的既有资源 scope | 新建 target、截断/不完整 inventory、只命中部分 target |
| `prospective_owner_target` | candidate 必须提交 concrete target，并命中 owner 封存的 create selector/prospective constraint | 非 owner 允许的新建、无 target、绕过 owner feasibility |

`TargetPolicy` 是 owner-neutral 的纯合同词汇。File、Git、Shell、MCP 或业务 profile 的具体语义仍由对应
owner/profile 提供；Orchestrator 只编译和验证 typed receipt，不按文件名、命令或领域文本推断策略。

同一 candidate/dispatch 内，每个 durable operation 必须独占 canonical
`(OwnerModuleID, TargetRef)`。两个普通 operation 即使存在 `DependsOn`，也不能先后修改同一 owner resource，因为它们的
target snapshot/replay proof 都在第一个 effect 前封存；provider 必须把同一目标的完整变化合并为一个 operation 和一个
receipt。唯一例外是 owner fact 明确封存的 exact `RequiredOperationSequence`：其中每项必须绑定不同 requirement ref/variant
并按依赖执行。不同 owner/target 的 operation 仍可按 dependency graph 确定性执行，non-durable operation 不参与该约束。

owner inventory 与 provider context 是两个不同合同：

- owner inventory 必须是完整的有界授权事实，或是能被 owner 独立验证的封闭 selector；截断列表不得封存为
  scope；
- candidate context 可以从 manifest 生成有损展示，但不得删除执行所需的 kind、parameter、target policy、
  binding reference 或 effect 约束；
- `OwnerInventoryBudget`、`CandidateContextBudget`、`RequestRecoveryBudget` 必须使用不同 DTO/字段和校验入口，
  禁止因数值相同而互用；
- inventory 超限且 owner 无法提供封闭 selector 时，在 provider 前形成 typed prerequisite failure。提高固定
  常量不能替代该产品语义。

每个 required CWI 必须在任何 operation candidate/dispatch 前，由 SM-2 从 current Definition seed、受约束
semantic-plan document 与 matching owner feasibility 编译出 deterministic、hashable 的
`CoverageEvidenceBinding`，并封存其 binding set/hash。binding 是 DTO/合同，不是新的状态机：它至少含
`BindingID`、exact CWI ID、definition/focus identity、required evidence kind/effect scope、允许 operation
kind/variant、exact target 或 sealed selector、recipe/check、适用 metric/artifact 与 declaration/snapshot lineage。
当前 Runtime 对 validation proof 采用一 focus 一 CWI 的保守形状；共享 recipe 只有在每个 CWI 具有独立
check/target 并编译为独立 focus/binding 时才允许。focus 级泛化 recipe/check 不得由一个 owner receipt 扇出关闭
多个异构 CWI。缺 binding、binding 不能被 owner 独立验证或 binding set 与 current definition 不一致时，
SM-2 不得进入 operation provider/dispatch；它只能继续有界 semantic planning/owner prerequisite，或以完整 typed
diagnostic 终态化。该失败零 operation、Decision、Approval 或替代执行路径。Goal/DoD、TargetText、LLM semantic-plan
文本与历史 evidence 都只能是 proposal/context/diagnostic；只有 SM-2 seal 才能补全 canonical binding。

semantic planning 是 SM-2/SM-3 aggregate 子域，不是第五个状态机。唯一 private transition 序列为
`semantic_contract_required -> semantic_plan_request_started -> semantic_plan_evaluated ->
semantic_feasibility_required -> semantic_feasibility_observed -> semantic_contract_sealed`；完整 rejection 只能进入一次
`semantic_plan_correction_required`，再次 rejection 或 owner/budget 终局只能进入 `semantic_contract_failed`。
ledger 必须持久化 Definition seed、manifest、attempt 0/1、stable prefix、logical request、correction、document、
owner batch/observation set 与 sealed binding lineage hash；这些 identity 必须进入 convergence progress signature。
duplicate event 不增加 revision，scope/seed/request/batch 漂移必须拒绝，任何阶段都不得直接创建 dispatch、coverage、
Decision 或 Approval。

每次semantic-plan或operations provider call都必须使用SM-3 persisted request session形成purpose-bound invocation seal；
seal不是另一份activation authority。对应call、correction、semantic generation和operation consumption必须引用
[Execution Budget](execution-budget.md)中的exact sub-ledger identity。

`stale | prerequisite_required` 先以 `semantic_feasibility_observed` 单独持久化 owner factual observation。
只有带有 expected/observed revision 且精确匹配当前 catalog dependency 的 `stale` 事实，才能复用
`semantic_contract_required` private transition，按 persisted refresh generation 原子退休 matching catalog
observation、保存完整 gap/diagnostic lineage，并回到 bounded catalog acquisition。`prerequisite_required` 不得被
泛化为 stale、不得退休仍然有效的 catalog/observation，也不得消耗 prerequisite-refresh budget；当前 semantic
proof 没有 owner-defined typed prerequisite request，因此 SM-2 必须在保留该事实和完整 diagnostic 的前提下
fail-closed 为 typed terminal。未来若增加 prerequisite acquisition，必须先新增接收 owner 定义的 typed command/result，
再由同一 private transition 接入，不能复用 stale refresh。以上处理不是新的 phase 或第五状态机，不增加 semantic
candidate attempt。新 catalog revision 必须产生新 manifest/request identity，旧 complete session 只保留 audit；
generation history 的 identity 是封存的 `manifestHash + logicalRequestID`，所以同一 active generation 内的
correction/focus request 变化不会制造重复 history，而资源回滚重现旧 catalog hash 时可以用新的 logical request
建立独立 generation。该 pair 同时是 semantic focus/binding 的 generation identity；同 manifest 的新 logical request
必须生成不同 focus/binding ID，而同一 pair 的 restart/replay 必须稳定。duplicate generation 不写 CAS，refresh budget
耗尽才以原始 owner diagnostic 进入
`semantic_contract_failed`。

catalog acquisition 已标记的 validation-surface `prerequisite_required` 不进入上述 feasibility transition：正常
provider request 根本看不到该 identity，也不会为它创建 semantic attempt 或调用 endpoint。只有已经封存为 ready 的
surface 在后续发生 catalog 漂移，或接收了不符合 current catalog 的旧/伪造 proposal 时，owner feasibility 才按上述
规则 fail-closed；任何自动 input acquisition 仍必须先新增 owner-defined typed command/result。

### 单一 candidate evaluator

完整 candidate document 只允许进入一个纯 `CandidateEvaluator`。它的输入是 persisted manifest、decoded
candidate 和同一 authority contract 的 prior attempt fact；输出只能是 `accepted` 或一个完整的
`CandidateRejection`。owner feasibility 是 capability owner 的独立 typed factual observation，不是
`CandidateRejection` 的子类。decoder、admission 与 owner-feasibility adapter 可以提供 typed observation，但不能各自
选择 retry、terminal、governance 或下一 action。

为取得 feasibility observation，Orchestrator 只可先用 operation kind 解析 manifest 中的 owner 和
authorization identity；该步骤不做 candidate admission。无法解析的 kind 不调 owner，由 evaluator 在
kind 层拒绝；可解析的 operation 可调用无副作用 owner probe，owner 只返回
`feasible | stale | prerequisite_required | infeasible` observation。候选的结构性 evaluator 必须先按固定顺序
返回 wire/parameter/target 违例；只有结构性校验接受后，Orchestrator 才将 owner observation 作为独立后继处理。

`CandidateRejection` 至少包含：原始安全 class/code/message、operation ID（若适用）、field path、actual、
allowed/required/forbidden、manifest hash、rejected shape hash 与完整 scope/lineage。它是 evaluator 的事实输出，
不得携带 retry、terminal、governance、Runtime action 或 `CorrectionDisposition`。

SM-3 在收到 rejection 后，必须先把它转换为 owner-local internal event 并持久化；随后只能根据 current
manifest、attempt ledger、rejected shape 与 typed field violations 决定：

| SM-3 owner-local result | 语义 | 唯一行为 |
| --- | --- | --- |
| `correctable_once` | 同一 manifest 内存在唯一、有界的结构修正，且尚未使用 correction attempt | seal 一个 `CorrectionDirective`，只允许 attempt 1 |
| `rejected_final` | 没有安全同合同修正，或 correction attempt 已用尽 | 形成 typed candidate terminal，零额外 provider call |

`CorrectionDirective` 是 Orchestrator persisted state，不是 evaluator/LLM/capability feedback。它只描述 candidate
delta，例如删除 forbidden field、补 required field、选择 allowed kind、替换为 allowed target、改变 rejected
semantic shape；它不修改 authority，也不包含 raw provider/decoder/owner 错误文本。semantic coverage-order 的完整文档
结构错误必须保存 manifest 全量 CWI identity 与 field path，使用 typed `coverage_order_document` violation；proof-batch
保存 exact batch CWI identity 并使用 `proof_batch_document`。两者都只能由 SM-3 owner-private transition seal。

若 complete rejection 无法由 SM-3 根据 typed diagnostic 归类，属于 `runtime_contract_defect`，必须 fail-fast
且零额外 provider、ticket、dispatch 或 effect。`candidate_correction_undefined` 不得作为正常模型输出兜底。

### 正交候选与 owner 处理结果

provider dependency/governance 在进入 candidate pipeline 前单独处理。进入候选处理后，所有结果只允许归入：

| 结果 | 来源 | 状态机行为 |
| --- | --- | --- |
| `request_recoverable` | EOF、output limit、timeout、429/5xx、未完成 document | 仅接受 LLM owner 已确认 physical submission 的 fact；同一次 Orchestrator CAS 结算本次调用、推进 SM-3 session、消费适用的 SM-4 transport budget，并在耗尽时应用 SM-2 failure；随后才可用新 PhysicalCallID 做同一 sealed expected-document 的 whole-document 有界恢复；不创建 attempt、不拼接 raw suffix |
| `request_terminal` | provider authentication/configuration/protocol/submission unknown 或 document recovery exhausted | 归一为唯一 owner-private fact，在同一次 Orchestrator aggregate CAS 中关闭 SM-3 session 并应用 exact SM-2 failure；CAS-before fresh reload 只从 submitted marker 封存 unknown，CAS-after/duplicate 复用 exact terminal；不创建 attempt、不重调 provider |
| `candidate_rejection` | CandidateEvaluator 的 complete typed rejection | 先持久化 original diagnostic；由 SM-3 owner-local reducer 决定唯一 correction 或 typed terminal |
| `owner_feasibility` | capability owner 的 `stale`、`prerequisite_required` 或 `infeasible` factual observation | 先转为 Orchestrator internal event，再由SM-2退休prerequisite、取得新authority；只有独立typed recovery condition/trigger/deadline/budget完整时形成recoverable wait，否则typed terminal；零correction attempt。它不与 provider request terminal 共用结果或 transition |
| `accepted` | evaluator 接受且 owner feasibility 为 feasible | persist sealed dispatch 后才可执行 |

`requestBytes>0`、selected streaming mode、错误文本或 reservation 都不证明 physical submission。exact unsubmitted audit
必须在对应 session transition CAS 中 release reservations；缺失/篡改 audit 只能结算为 unknown 并终止当前 request，禁止
以 application retry、transport fallback 或 fresh reload 再次提交。

sealed execute adapter必须把完整operation payload、catalog/policy revision、exact owner subject、effect/replay class、
ExecutionKey/Fence和适用governance grant绑定到receiving-owner concrete command。owner在任何call/effect前重新验证producer、
current claimed receipt authority、current resource/schema/policy以及exact grant；独立mutable data copy、missing catalog、
stale revision或grant drift均保持零外部动作。

普通响应与 fresh-load receipt recovery 必须调用同一 outcome-first judgment。`Outcome=rejected` 先保留 owner 的 exact
original diagnostic，禁止再以 rejected receipt 的 `ObservedEffectScope=unknown` 覆盖成 Orchestrator effect mismatch；只有
`accepted | no_op` 才继续执行 sealed effect compatibility 与 evidence-quality gate。未知 outcome 直接 fail-closed。

claimed operation没有receipt时，只有sealed replay class为`safe_to_replay | owner_managed`或owner给出exact replay proof，
才可进入SM-4 recovery-budget handoff；`at_most_once | unknown`直接持久化`outcome_unknown`，零owner重放且不消费replay
budget。recovery不能因为operation被称为validation/read-only就放宽该规则。

correctable/final 只属于 SM-3 persisted transition result，不是 `CandidateRejection` 字段，也不得接收 owner
code。以上分类是候选处理的唯一分流表。错误字符串、TUI/Web、projection、repair hint、owner adapter 或
测试 fixture 不得定义额外分流。
