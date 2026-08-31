# Runtime Invariants And Change Gates

Status: normative

Type: core-contract-part

Last Verified: 2026-08-31

## 跨状态机不变量

1. SM-1 不选择 action；SM-2 不执行 operation；SM-3 不选择下一 action；SM-4 不重建 authority，亦不修改别的 owner state。
2. 每次跨状态机交互都携带 workspace/run scope 以及适用的 definition/evidence revision、authority
   snapshot、dispatch/operation identity。
3. 正式事实必须先持久化，后产生外部可见 projection；projection 可最终一致，authority 不可反推。
4. candidate、dispatch、governance resolution、evidence commit、terminal delivery 都必须幂等。
5. provider target、definition 与 activation 在 Run 内封存；变化需要新的 typed activation，不能由 Resume
   patch。
6. contract/invariant violation 必须形成 typed diagnostic/failure 并沿以上方向收敛；不得旁路某个状态机
   修补正式状态。
7. recoverable runtime wait 必须携带 typed reason、recovery condition、trigger/deadline、budget 和 persisted successor；
   它不创建 SM-1 state、Decision/Approval 或 application scheduler authority。
8. execution-budget view 只能从 owner/Runtime sub-ledger 纯派生；allocation 先于 physical call/effect，unknown consumption
   不得被解释为 zero。
9. `target_grounding_unresolved` terminal delivery 必须同时满足 canonical terminal 与 current semantic-authority readiness；
   binding-free semantic generation pending 时只能请求 semantic plan，不能投递该临时终态或请求 operations candidate。
   其它 canonical terminal 不得因此被改写或重规划。

唯一 owner 约束的是 state authority，不是“每个 state type 只能有一个内部 transition ID”。同一 owner 可以用多个
private transition 协调自己的状态，但任何 transition 都不能跨 owner；`run.replacement` 因此只能作为 Run SM-1 的
subordinate transaction，不能升级为第五个状态机。administrative termination 必须保持
`Run SM-1 intent -> Orchestrator SM-4 handoff record/delivery -> Run-private SM-1 terminal`，handoff 不得代写 lifecycle。

## 中立 effect、evidence 与治理

| Effect scope | 可关闭 durable state change |
| --- | --- |
| `none`、`ephemeral_session`、`governance_only`、`unknown` | 否 |
| `workspace_durable`、`remote_durable` | 仅在 RequiredOutput 接受、owner receipt 证明且 sealed binding 逐字段匹配时 |

`need_state_change` 的 sealed candidate 至少含一个与 RequiredOutput 兼容的 durable operation；
`need_validation` 只能含 `none` 或显式允许的 `ephemeral_session` operation。assessment 只消费当前
Run 的 accepted evidence；assessment-only focus 不执行 executable operation。

每个 executable dispatch 必须恰好有一个 durable SM-3 admission 来源：普通 dispatch 对应 exact accepted
`RuntimeCandidateAttemptRecord`，governance continuation 对应 exact granted binding 的 `ResumeDispatchID`，两者互斥。
accepted attempt 必须在构造 dispatch 前，直接从同一 manifest + candidate 前像独立封存 operation target authorization
projection/hash；含 managed validation 时还必须独立封存完整 recipe body projection/hash。fresh-load 必须同时验证这两份
admission commitment 与 dispatch 完全一致，不能用协调改写 dispatch projection/self-seal 的方式回写 admission lineage；
非 managed candidate 不得携带 recipe commitment。governance continuation 必须从 immediate source 逐跳继承两份 commitment。
accepted operations attempt 还必须反证到更早持久化的 raw-canonical request-session `CandidateManifest` body；manifest 与 persisted
candidate 是 target/coverage/recipe projection 的共同 preimage，不能只依赖 attempt/dispatch 两份可协调改写副本及 self-hash。
typed `confirmed_validation_repair` manifest 使 `RuntimeDispatchRepairCommitment` 成为必填 admission projection；commitment
必须显式封存 sealed Profile/descriptor 确定的唯一 `RepairOwnerModuleID`，并绑定 origin dispatch、matching-owner
confirmation、rejected source receipt/evidence 与 closure，在 governance resume 逐 hop exact 继承。candidate 中所有
state-change operation descriptor owner、confirmation owner、resource changes 与 receipt producer 必须匹配该值；fresh-load 不能因
confirmation、directive 或 commitment 被删除而把该 transaction 归类为普通 dispatch，也不能按 capability 名称重选 repair owner。
dispatch seal 必须纳入 canonical candidate、validation recipe body、完整 owner catalog/observation preimage、精确 fallback
policy 与 per-operation execution authority；active execution、restart 和 governance resume 只能读取该 seal，不能用 current
owner catalog、current activation policy 或 current prerequisite observation 重新解释历史 authority。

owner receipt ledger 与 dispatch execution ledger 必须按 `ExecutionKey + fence + OperationID + receipt ref/hash` 双向完全匹配；
`owner_committed|evidence_committed` execution 不得缺 receipt，receipt 也不得脱离 committed execution projection。governance
source 与 completed resume clone 可以投影同一份未漂移 receipt，但不能复制或改写 owner 事实。
`evidence_committed` execution 还必须保存 exact `EvidenceFactID + EvidenceFactHash`；hash 覆盖完整 persisted fact，但不进入
immutable dispatch authority hash。fresh-load/recovery 必须由该 commitment 精确反证 evidence 的 kind、revision、binding/CWI、
failure/reason、receipt 与 operation lineage，不能只凭 receipt hash 或 processed-event fence 猜测 evidence body。

`EvidenceSequence` 是 Orchestrator 唯一的全局 evidence chronology；ordinary accepted/rejected fact、governance resolution/fact
与 assessment fact 共享连续的 `evidence/<n>` revision，不允许各子账本自行计数。顺序只从 completed dispatch 的持久化顺序及
可选 active dispatch replay；每个 typed fact/resolution/assessment 必须被其 owning dispatch 恰消费一次，governance-resume
clone 只引用已由 source dispatch 消费的原 fact，不能再次推进 sequence。aggregate 的 `Facts.EvidenceRevision` 必须等于最终
sequence revision。orphan、duplicate、gap、跨 accepted/rejected 重复、普通 fact 冒充 assessment/governance、或协调改写
revision/sequence 均在 fresh-load fail closed。

assessment 另有 pre/post-seal 双门禁：`RuntimeAssessmentAuthorityInput` 必须 raw-canonical 且精确反证 Definition seed source、
purpose、Focus/bindings 与 owner admissibility catalog，其 hash 是 dispatch authority。purpose 必须先存在于 request-session
`CandidateManifest`，并由更早 persisted semantic provider/owner batch独立重放；同步改写 manifest/attempt/dispatch/self-hash
仍必须失败。commit-time
`RuntimeAssessmentAdjudication` 则由 persisted evidence/dispatch/coverage proof lineage 重放，并与
`RuntimeAssessmentFact.AdjudicationHash` 双记账。两者不能互相替代，post-seal adjudication 不进入 immutable dispatch hash。

所有 dispatch authority（assessment input、repair commitment、target/recipe/catalog、operation execution；governance continuation
还包括 source/grant 派生的 completed/claimed execution 与 fence/key）必须在一次 typed pre-seal builder 中完成，然后仅计算一次
最终 authority hash并 Validate。builder 返回后补写 authority 或为补写内容重算 hash均属 SM-3 边界违规。

operation-scoped governance必须保存恰好一个ticket/approval owner identity，以及resolution、dependency、subject、
receipt hash、ExecutionKey、fence、effect与sealed idempotency binding。continue只恢复suspended binding指向且未列入source
`CompletedOperationIDs`的exact `owner_committed` execution；保留source governance receipt/history，新resume execution
继承原ExecutionKey、`ExpectedEffectScope`、sealed idempotency、`AdmissibilitySnapshotID`、`AcquisitionBinding`与`ReplayProof`，以fence
`N+1`、空receipt ref/hash和`governance_granted` marker进入claimed。completed sibling必须为`evidence_committed`并完整
保留；pending sibling必须为`sealed`，只替换dispatch-scoped ExecutionKey，其candidate与sealed owner authority不变。
source execution/binding不匹配或execution/replay authority自身不一致必须fail-closed，禁止从current facts/catalog重建。
source dispatch必须为committed，且history严格满足`source operation dispatch < committed governance dispatch < resume dispatch`；
governance request必须是canonical raw record，并与治理dispatch的完整dependency kind/subject/action/risk/authorization hash一致。
owner call前以独立CAS把marker推进为`governance_effect_started`，只表示call可能已尝试；若exact receipt缺失，只有具备
sealed replay authority时，再次physical owner call才必须先由独立SM-4 event消费exact operation-recovery budget，否则
进入`outcome_unknown`。owner必须preflight exact grant；persisted grant与query/caller grant必须同时缺失或逐字段完全一致，
不能通过省略grant绕过治理。

validation recipe identity与check identity必须分离。required check只有在owner receipt中真实passed，且不同时出现在
failed/skipped时才能贡献credit；exit 0、recipe ID、非空PassedChecks或冲突outcome均不能替代matching check fact。

state-changing capability operation 的 receipt 必须由 owner 对真实 effect 做有界观察。对 workspace durable Shell
operation，真实 changed refs 必须非空且属于 sealed `RuntimeOperation.TargetRefs`；存在未声明 changed ref、只有
unknown effect 或 zero-work 时不能形成 accepted durable evidence。owner observation 只返回 typed receipt，不得
解释 Goal/DoD 或选择下一 Runtime action。

remote/network durable capability同样必须返回owner校验的changed/no-op、before/after revision与bounded resource changes，
不能把sealed expectation或catalog declaration回显为observed effect。没有end-to-end idempotency key的外部request必须
发布`at_most_once`；提交后receipt丢失进入`outcome_unknown`，不能以receipt query miss重复请求。

## LLM request、cache 与上下文

LLM 是不可靠的 protocol client，不是 authority。请求必须由 sealed typed context 渲染：

- C0：stable runtime/schema；C1：current immutable Definition partition exact text；C2：declarative capability/governance contract；
  C3：current focus/required output；C4：failure/evidence/diagnostic；C5：有界 ref。
- P0–P4 的必要字段缺失时不得提交 provider。raw prompt、raw provider body、credential、endpoint、
  snapshot content 不得进入 aggregate、audit、projection 或 memory。
- unexpected stable-prefix drift 是 renderer defect。完整 request/cache 规则以
  [runtime-request-cache-note.md](../runtime-request-cache-note.md) 为准。

## Execution budget

provider/capability consumption、candidate/semantic generation、repair/reopen 和 recovery 使用独立 logical identity 与
persisted ledger；duplicate、stale、CAS reload、projection refresh 和 accepted resource invalidation不得重复或错误扣减。
统一 allocation、unknown-consumption、wait/exhaustion、restart 和 projection合同见
[Execution Budget](execution-budget.md)。

## Current persisted-data baseline

当前阶段只维护一套持久化结构，不维护历史 aggregate、resource、event 或 repository layout 的 migration/reducer。
`RuntimeStateRecord`、`RuntimeBootstrap` 与 `Run` 没有用于选择历史解码分支的顶层版本字段；持久化 JSON 必须只包含
当前字段且只能包含一个值。未知/退役字段、旧目录、空文件、旧 payload 表示或当前不变量缺失都在 owner/repository
读取边界直接失败，不生成 Run fact、projection disposition、governance ticket 或 recovery lease，也不调用 provider/capability。

- `fresh_activation_required` 只表示一个可读的 current Run 缺少其 sealed `RuntimeBootstrap`，不得用于分类旧数据；
- 数据清理是部署者删除所配置 data root/workspace data 后重新创建 Workspace/Run 的外部操作，不是 SM-1 transition；
- fresh Run 只从 current bootstrap 开始，不继承既有 request、candidate、CWI credit、receipt 或 recovery budget；
- convergence、recovery 与 execution-budget policy 必须由 fresh activation 显式封存；normalize/clone/read 不补零值、
  不从其他 policy 派生缺失 policy，任一缺失都按无效 current data 失败；
- nested schema identity 只校验当前 typed document/ledger，不授权版本协商、默认值补齐或原地升级。

## Forbidden surface

以下路径已从生产与现行规范中删除：`StagePacket`、`NextAction`、`DispatchNextStep`、
`ExecutionContextPatch`、通用 RuntimeAdapter/step dispatcher、application Start/Resume queue、
自动 replan successor，以及 checkpoint replay。

这些结构不得用于 DTO、EventHub 合同、状态迁移、配置或 prompt。

## 变更门禁与文档纪律

所有 Runtime 变更必须：

1. 标明影响的 SM-1～SM-4、owner、persisted fact、合法/禁止迁移；
2. 为每个 owner-defined typed command/result 添加 producer/consumer 回归；
3. 覆盖 normal、duplicate、stale、CAS 前中断、CAS 后响应丢失和 fresh-process reload；不可达语义必须
   由测试证明；
4. 覆盖预算/guard 与零副作用断言：在 prerequisite/authority 失败时不得产生 provider request、attempt、
   dispatch、ticket 或 effect；
5. 同步更新当前事实、对应 module readme、flow 与稳定 verification contract；
6. 通过 `make verify-boundary-contract`，确认没有将状态迁移放入 `pkg`、没有 forbidden surface、没有让
   projection/TUI/Web 成为 authority。

`docs/` 不保存实施顺序、closure diary 或 live 热修计划。若需要跟踪工程工作，应使用外部 ticket/issue；最终合同只在本规范树
维护一份。发现第二份 Runtime 状态迁移、graph/step continuation 或自动 successor 设计时，必须直接作废并删除，而不是并列保留。
