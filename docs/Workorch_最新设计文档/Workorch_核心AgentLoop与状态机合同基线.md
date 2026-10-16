# Workorch 核心 Agent Loop 与状态机合同基线

版本：v0.3　日期：2026-10-12

状态：冻结候选，供设计评审。本文未获用户最终批准，不表示实现验收或 live 验证通过。

审查基线：Workorch `5360caf36e081717087557f34d2af3af1c16dc16`；架构方法参考 go-multi-module-dev `6ba50452fccd33c912b07a0648fbb97acf15d756`，v2.6.0。仅审查固定版本文档、源码及 ownership manifests；未改仓库、运行测试、启动服务或调用真实 provider。

本文独占核心循环顺序、状态算法、原子写集和恢复调度规则。业务对象及交互语义以[统一业务模型](Workorch_统一业务模型与交互关系.md)为准；接收 DTO 和字段合同以[具体合同目录](contracts/组件具体合同目录.md)为准；新建与变更目标协议以[NC01–NC05](contracts/创建与定义变更合同.md)为准，行政-only/迟到对账的目标schema以[运行期目标增量合同](contracts/运行期目标增量合同.md)为准；fixture/case 以[验收规格](validation/验收规格.md)为准；G0–G4、实施批次及未闭合差距只在[总纲](Workorch_逐项收口总纲与首项方案.md#gap-ledger)维护。

新增[状态迁移逐边索引](runtime/状态迁移逐边索引.md)为本文规范性附录：覆盖29个 transition families、SM-1全部20条不同状态边、SM-2/3/4全部76种私有event及25个支持family的97个event或明确子切点。每边列前态/事实、具体event、guard、完整写域与字段级源码、后态/output、拒绝/重复/恢复；不再靠概览箭头推断合法迁移。上述计数只覆盖当前基线；新增NC目标guard、私有事务及late-settlement拟新增event单列[TargetHooks](runtime/状态迁移逐边索引.md#TargetHooks)，不声称已被现存76种event实现。

## 1 先冻结什么

先冻结核心循环、状态所有权、跨边界合同和失败恢复规则，再让 22 个组件逐个接入和独立验证。核心循环固定为：

**SM-4 从持久状态驱动 → SM-2 确定当前义务及权限 → SM-3 请求并验收受限提案、封存执行 → capability owner 执行并返回事实 → SM-2 判定证据与 all-of 完成度 → 继续工作、等待或封存终态 → SM-4 有序交付。**

四个正式 Runtime 状态机是既有规范，不新增通用 workflow engine：

| 正式机器 | 唯一语义 owner | 冻结职责 | 不能越界 |
|---|---|---|---|
| SM-1 Run lifecycle | Run | activation、匹配治理继续、终态应用、Run lineage | 不选择工具、改变验收或替 SM-2 判成功 |
| SM-2 Authority and convergence | Orchestrator | facts → authority/decision；Definition/CWI、工作义务、证据及终验 | 不把模型声明、receipt 或 UI 状态直接当 credit |
| SM-3 Candidate and dispatch | Orchestrator | request session、candidate admission/correction、dispatch、operation receipt/evidence | 不自行放宽 authority、重置预算或授予恢复执行权 |
| SM-4 Durable drive recovery delivery | Orchestrator | wake/lease、恢复授权、有界等待、终态材料及交付 | 不发明业务 action、新 Run 或改写封存终态 |

SM-2/3/4 共享同一 Orchestrator aggregate 的语义所有权。Run 提供 opaque Runtime 持久化边界，不解释其业务内容。一次事实涉及多个 SM ledger 时，必须由唯一主事件分类、完整写集和**同一次 aggregate CAS**提交；不能先写一个机器、再补另一个机器。跨 owner 则是各自提交的单向 saga，不能假装有分布式原子事务。

依据：[Runtime 总合同][contract]、[四机规范][machines]、[状态分类清单][models]、[transition owner 清单][transitions]。

## 2 产品入口与 Runtime 入口

### 2.1 M3 是启动前就绪条件

产品语义由[统一业务模型](Workorch_统一业务模型与交互关系.md)与[创建及变更合同](contracts/创建与定义变更合同.md)唯一维护。Runtime接收的已决约束是：Web-only创建、默认手动启动；M3表示exact最终Definition已确认封存、资源就绪、startup check通过，且尚无Run；有界自动激活必须用户明确授权，不能从封存、readiness、重启或前Run终态推定授权。

NC01–NC03是目标方案，不声称现有字段/API已具备完整readiness证书或激活授权账本。Workspace以一次owner CAS提交授权消费、稳定activation intent和slot reservation作为接纳点；Run owner才物化RunID及pending状态。reservation不是ActiveRunID。撤销先赢则无permit，接纳先赢则已经受理；后续动态权限撤销仍限制未获授权effect。晚到first-activation重放即使原Run已终态也返回原Run，不能改读为Restart。详细原子边界、错误结果及恢复协议由NC02/NC03定义。

Activate/Start在本文是产品意图名称，不新增跨owner目标状态API；基线执行入口使用resume/restart kinds，无前驱的显式启动可由既有root Run创建路线承接。现有authoring creation不创建Run；现有admission eligible和Workspace active均不等于完整M3。[现有创建][creation]、[admission][admission]、[启动流程][activation]

### 2.2 全流程图

图中方框是职责或概念阶段，不声称每一项都有同名持久枚举。

```text
Web 创建 → 草拟与评审 → exact 最终确认 → Workspace 封存 Definition
                                          ↓
                         资源就绪 + startup check = M3，无 Run
                                          ↓ 显式启动或有界明确授权
Run SM-1 pending → active → Orchestrator SM-4 持久驱动
                               ↓
                    SM-2 facts / prerequisite / authority
                               ↓
                    SM-3 semantic 与 operation 提案
                               ↓ 严格验收、只读 feasibility、封存 dispatch
                    capability owner 执行 → exact receipt
                               ↓
             SM-2 工作进展或证据裁定 ← SM-3 evidence commit
                  ↙              ↓                  ↘
       有界工作 / repair     治理或技术等待        all-of 终验满足 / typed failure
                  └──────→ 原义务继续                 ↓
                                           sealed terminal decision
                                                       ↓
                                SM-4 immutable material seal
                                                       ↓
                                     Result ACK → Workspace ACK → Run ACK
                                                       ↓
                                              完整结束与可追溯结果

Goal / 验收 / 约束变化：reasoned ticket → exact 最终候选批准
                    → new Definition → 独立启动授权 → Run-owned 创建或 replacement → new Run
```

## 3 状态词汇与合法迁移

### 3.1 SM-1 的实际状态

真实 `RunState` 为 `pending / active / blocked_on_decision / blocked_on_approval / completed / failed / cancelled / superseded`。`ready`、`paused`、`M3`、`recovering` 均不是 Run enum。

| 当前状态 | 可到达的不同状态 | 触发与 guard | 写入和输出 |
|---|---|---|---|
| pending | active、两个 blocked、failed、cancelled、superseded | exact activation/治理/终态事实，由 Run reducer 检查 | Run revision、生命周期事实；不能直接 completed |
| active | 两个 blocked、completed、failed、cancelled、superseded | blocker 必须匹配治理依赖；completed/failed 仅取 SM-2 sealed terminal；行政终止取 Run intent | 只有 Run 私有事件应用 SM-1 |
| 任一 blocked | active、failed、cancelled、superseded | active 必须 exact persisted resolution 及 continuation fence；普通 Resume 不足 | 匹配继续 ACK 后才可执行；不能直接 completed |
| 任一 terminal | 无不同生命周期状态 | terminal decision、Definition、原结果不重开 | 可追加受控 replacement lineage，不改原终态 |

同状态在结构上可接受，并不许可任意字段改写；exact replay 仍须验证身份。非法输入、错 revision、错 blocker/fence 要拒绝，不能借 no-op 绕过 guard。[实际枚举][common]、[迁移表][run-transitions]、[Run intent][run-intent]

### 3.2 SM-2 的实际状态与裁定

`ConvergenceState` 恰为七值：`need_prerequisite / need_state_change / need_validation / need_assessment / need_governance / complete / failed`。这是确定性 reducer 对持久 facts 的结果，不是外部可设置的目标状态。

| 首个适用条件或已提交事实 | 合法裁定 | 同次状态变更必须保全 |
|---|---|---|
| 当前 prerequisite 不足或 stale | need_prerequisite，取得 typed owner facts 后重算 | 完整 pending graph、精确 dependency/代际，不能提前开 provider session |
| authority 许可且需产生资源变化 | need_state_change | 原 DefinitionFocus、当前 binding、预算及允许 effect 范围 |
| 需验证且 discovery/catalog 已就绪 | need_validation | exact check/recipe、owner scope、freshness 和 proof binding |
| 必要证据齐但尚未独立裁定 | need_assessment | exact pre-seal assessment authority 与 evidence cutoff |
| typed 信息或授权缺口 | need_governance | 唯一治理依赖及匹配 ticket/approval identity |
| 当前 proof 满足且还有未完成 CWI | 原子推进 focus，再确定下一 need_* | 当前 CWI settlement、coverage、下一 focus/authority 一致 |
| 全部必需 CWI 及附加 guards 满足 | complete，封存 terminal | 全量 all-of proof、结果和终态 authority |
| 明确不支持、不可恢复或对应预算/期限耗尽 | 按附录SM2-FailureMap的具体code和owner lane确定failed/need_governance/技术恢复 | 失败类型、原事实、预算及provenance；缺sealed治理dependency明确失败，不留任意治理兜底 |

完整优先级和typed失败映射见[SM2-Priority](runtime/状态迁移逐边索引.md#SM2-Priority)与[SM2-FailureMap](runtime/状态迁移逐边索引.md#SM2-FailureMap)。已封存 failure/budget disposition、repair confirmation/directive 等有确定优先级；不能以刷新事实、换 closure 或换 generation 掩盖它们。合法 repair 留在同一终验义务，修复后重新验证。`focus advance`、`repairable gap`、`recoverable runtime wait` 是迁移或事实分类，并非额外 ConvergenceState。[唯一 reducer][reducer]

### 3.3 SM-3 与 SM-4 的从属账本

以下都是既有类型，不新增第五个 Runtime 状态机。箭头只说明主路径；错误、退休与恢复边必须由对应 typed event 和 guard 允许，不能由 enum 名称推断任意互转。

| 账本及所属 | 实际值或主路径 | 关键 guard 和失败语义 |
|---|---|---|
| CWI binding，SM-2 | unbound / planning / bound / settled / reopened | binding 生命周期与 credit 分离；reopen 保留历史及跨代际预算 |
| Coverage，SM-2 | open / satisfied / unsatisfied | 只有确定性 all-of 裁定授予 satisfied |
| Candidate request，SM-3 | prepared / recovery_pending / transport_retry_pending / document_complete / terminal | 必须同时读 physical submission/settlement；prepared 不证明尚未发送 |
| Candidate attempt，SM-3 | pending / failed / accepted / invalidated | 仅 attempt 0、1；一次受限 correction，禁止 attempt 2 |
| Dispatch，SM-3 | sealed / committed / invalidated | accepted candidate 与 dispatch、适用 target/quota commitment 一起 CAS |
| Operation execution，SM-3 | sealed → claimed → owner_committed → evidence_committed；或 outcome_unknown | claim 前零 effect；普通 proof 分离 receipt/evidence 切点，working 协调结算见 §4 |
| Drive lease，SM-4 | scheduled / running | release 后无当前 lease；不存在虚构 done 值；generation/epoch/token fencing |
| Operation recovery，SM-4 | issued / in_flight / uncertain / settled / retired | 成功 claim 才给一次 physical handoff；无回执不等于可重做 |
| Recoverable wait | waiting / probe_claimed / condition_satisfied / expired / exhausted / retired | manifest 将其分类为 immutable_outcome；记录在 aggregate 中，但不是新 Run 状态 |
| Operation governance，SM-2 | suspended / ticket_bound / granted / rejected | grant 与 SM-3 continuation 同 CAS；绑定 exact operation 和递增 fence |

`RuntimeSemanticPlanningPhase` 是同 owner SM-2/3 协调账本：`semantic_contract_required`、`coverage_order_request_open`、`coverage_order_accepted`、`proof_batch_request_open`、`proof_batch_document_accepted`、`proof_batch_feasibility_pending`、`proof_batch_feasibility_complete`、`semantic_plan_requested`、`semantic_feasibility_required`、`semantic_plan_correction_required`、`semantic_contract_sealed`、`semantic_contract_failed`。它不是统一驱动所有组件的全局 Stage。

其余 Runtime 从属状态也沿同一 owner 注册：budget allocation 属 SM-3，budget reconciliation 与 delivery acknowledgement 属 SM-4，semantic-plan batch 属 SM-3，正式 closeout 中 requirement coverage/assessment/evidence 属 SM-2。它们不能成为额外驱动器或外部可写状态入口。

provider outcomes、feasibility dispositions、lease reasons、启动恢复分类、ToolCall/Patch/Validation/Artifact 的结果以及 Step/UI projections，须按 [model manifest][models] 保持原分类，不能升格为迁移 authority。

## 4 一次完整循环的固定合同

每一步都只接受当前持久身份，跨 owner I/O 在 reducer/CAS 外执行。收到响应后先验证当前 request、revision、authority、dispatch 及 fence，再提交事实。

1. **启动准入。** Application 提交 typed intent；Workspace 校验已封存且可启动的 Definition、资源及检查绑定；Run owner 创建唯一 generation、sealed bootstrap 和 pending Run，再由自身事件激活。重复 RequestID 同 payload 收敛到同一事务，变更 payload 则 conflict。NC03目标还要求pending Run的exact slot ACK后才activation；基线Run可先active再由Orchestrator接纳activation，active不等于Runtime已初始化/可执行，未接纳前零provider/effect，投影须明确启动未就绪
2. **恢复入口。** SM-4 从 Run typed inventory 和 opaque aggregate fresh-load；检查生命周期、retirement fence、exact persisted lease 和 continuation ACK。相同 Workspace/Run 的同步调用和后台 timer 共用 singleflight
3. **优先处理已封存交付。** terminal delivery ready 优先；行政终止独立 delivery lane。其次处理已存在 wait、Definition clarification、semantic authority、CWI reconciliation，再进入普通 convergence；不从旧错误或 UI 状态另造分支
4. **取得前置事实。** SM-2 经 receiving owner 的具体 typed query/acquisition 获取 catalog、target selection、snapshot、provider ceiling、validation discovery 等。缺项时不得创建 request session、调用模型或扣 candidate attempt。accepted effect 引起的 dependency invalidation 是正常前进，不偷扣技术恢复额度
5. **确定工作及证明义务。** 从 immutable Definition/CWI 编译允许的 semantic family、target、recipe 和 evidence policy。模型可以在封存范围内提议；只有 SM-2 接受 owner-grounded facts 并形成 authority。可行性不足不能让模型猜一个替代验收
6. **准备并请求模型。** 固定顺序为只读 preflight → fresh-load 校验当前 request intent → exact session CAS → physical request。session 绑定 logical request、ExpectedDocument、purpose、manifest、stable prefix、预算和允许的 provider；preflight 失败不创建 session、不分配该请求预算或调用 provider。只有 session 成功落地后，LLM owner 才发 physical request。返回一个完整当前 wire JSON document；禁止拼历史碎片或本地修补后接受
7. **受限候选验收。** 保存完整提案，执行 strict shape、manifest、身份和静态 conformance；再取得只读 owner feasibility。格式或约束错误按原 request/correction 合同有界处理；动态错误必须有 manifest-bound witness。document_complete 后重启只恢复本地验收，不能重问模型
8. **封存 dispatch。** accepted candidate、exact operations、authority/target commitments、适用 quota 和状态写集一次 CAS。主事件仅归属一个 SM，跨账本变化是同 owner 协调，不是共享状态所有权
9. **认领并执行。** 先持久 execution key、fence、idempotency/replay policy，再 claim。capability owner 只执行授权 effect、返回 exact receipt。File 实现通过 Orchestrator typed contract 查询/提交 receipt；“owner-authored receipt”不表示每个 capability 都另有 receipt repository
10. **提交证据。** 普通 proof 路径先持久 owner_committed，再由 evidence event 原子提交 evidence、processed-event identity 和 evidence_committed。ImplementationWork 的 OperationReceiptObserved 则必须在同一次 CAS 协调 SM-2 effect/round、SM-3 execution/evidence 与适用 SM-4 retirement，不能强行拆开其完整写集。两条路径的响应丢失都先查原 receipt；已知 effect 不重做。错 Run/focus/binding/target/native check 不获 credit
11. **工作推进或终验。** 合法工作进展、repair 和 owner-fact acquisition 不自动满足 DoD；SM-2 回到原终验链。普通 proof 要 current Run/focus + sealed binding + exact dispatch/receipt + quality/effect + recipe/check/metric/artifact + mandatory assessment 全部成立
12. **继续或等待。** 有执行义务则重算下一轮；genuine governance 必须 exact resolution；技术等待必须有 wait ID、条件 hash、probe/trigger owner、eligible time、deadline、budget。没有真实可恢复合同的错误不进入无限等待
13. **封存终态材料。** complete/failed 先有 SM-2 sealed decision；cancelled/superseded 先有 Run intent 和 SM-4 termination record。SM-4 由唯一纯 compiler 读取已验证 aggregate，独立 CAS 保存 immutable terminal material seal；首次 Result I/O 必须在其后
14. **有序关闭。** Result接收材料并ACK exact ResultID + persisted ResultCloseout；Workspace ACK完成自身handoff，基线实现此时清ActiveRunID，但逻辑handoff屏障仍保留，直到对应Run应用原ResultID进入exact终态并ACK。空slot不能创建或激活另一Run。0/1/2 ACK均未完整结束；原seal有效、ordered exact三ACK及适用post_terminal_delivery lineage guards全满足后，exact replay零新增工作。交付失败只补当前阶段，不重跑验收/effect。需要新Run时另走Run-owned intent/replacement；跨Run未知effect安全门禁仍须NC05证明，终态ACK本身不证明物理效果已消失

现有 drive 的 1024 次循环上限是 persisted-transition 技术 guard，不是 1024 次模型调用或产品工作预算。revision 无进展应报 typed invariant；仅 bookkeeping 进展须持久并挂载 successor 后让出执行，不能在内存空转。[驱动实现][drive]、[写集检查][write-set]、[CAS coordinator][coordinator]

### 4.1 必须包含真正做工作的链

目标不能退化成“拿已有文件反复证明是否完成”。需要在同一 Run 内支持有界工作轮次：

```text
原 DefinitionFocus 与 CWI 不变
→ 受政策约束的 ImplementationWork reservation / working binding
→ semantic proposal / owner feasibility / sealed work dispatch
→ capability effect / receipt / work-round settlement
→ 后续有界工作轮次，或回到原 Definition 的完整 validation + assessment
```

工作轮次只能推进 working ledger；不能生成代表终验的 CoverageEvidenceBinding、把 round completed 当 CWI satisfied、跳过终验或清零跨轮预算。与修复一样，真实 effect 必须可恢复、可追踪、可约束。

- 私有账本以 ScopePreview、Rounds、PurposeChoices 及 receipt lineage 表达事实，没有另一个工作生命周期 enum；用途仅 implementation_effect / terminal_proof
- 先只读 scope preview，再按 exact policy、owner、target 和轮次上限 reserve effect round；preview 不耗 effect round，重复用途选择不重复 reserve
- partial all-of receipt 留在原 round/dispatch；完整 receipt 集合和原 evidence 反向证明后才结算工作轮。working evidence 的 BindingRefs/CoverageWorkItemIDs 为空、Validation=nil，始终零终验 credit
- rejected/no-op/weak effect 不能借新轮续权；治理只挂起原 dispatch，unknown 不释放未知消费。每轮完成后取真实 fresh owner facts，不能从旧 catalog 直接续造权限

**当前关键缺口：私有 ImplementationWork 模型、transition 和 receipt 路径已存在，但 `ExplainRuntimeImplementationWorkAvailability` 对声明的 ImplementationWorkPolicy 返回 `Available=false` 与 unavailable 原因，admission gate 尚未开放。** 本文冻结的是必须验收的工作链，不能把这些私有实现存在写成生产链已经可用。[工作账本][implementation-state]、[开放门禁][implementation-policy]

### 4.2 all-of 完成不是模型的自我报告

最新source已实现结构化源义务链：eligible exact candidate的inventory进入冻结输入，每个CWI绑定逐obligation/verification的稳定check，Profile声明重新核对适用性，terminal输出逐源结论并拒绝删项。该结论只涵盖结构化图和确定性proof守卫；无Specification的旧输入仍可返回nil inventory，不能宣称任意自然语言承诺已被自动抽取或完整理解。依据：[sealed来源投影](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/definitionadmission/runtime_source_obligation.go#L13)、[逐义务check与seed反证](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_source_obligation_check.go#L65)、[Profile适用性校验](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_source_obligation_applicability.go#L7)、[terminal逐源结果与反漏项校验](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_source_obligation.go#L6)。

终态材料现在按exact current binding/proof及source/resolution lineage过滤accepted evidence；旧accepted审计事实若不适用被标为stale_or_superseded，而不恢复credit。[current proof适用性](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_evidence_applicability.go#L4)。但[当前OwnerRevisionSet compiler](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_result_compiler.go#L121)仍仅聚合accepted_evidence的runtime_authority refs，完整源owner版本闭包必须继续按总纲差距验收，不能将当前过滤实现扩大为所有owner版本已接齐。

首先以原始 Goal/验收/约束承诺清单核对 source obligation → CWI 映射和六类 review 的逐项适用 scope。canonical inventory 内 all-of 只是第二道门；遗漏承诺不在 inventory 内时，全部 CWI satisfied 仍不证明用户目标完整达成。对应当前缺口仅见[总纲现状与缺口台账](Workorch_逐项收口总纲与首项方案.md#gap-ledger)，本文不重复维护F1/F2状态。

- 同一 prospective multi-target dispatch 必须覆盖全部 required targets；每项都要 exact accepted receipt 和 evidence_committed。N−1 项仅为进展，不能跨 dispatch、repair 或 generation 拼成功集合
- assessment 的 satisfied 文本只是 observation。SM-2 依据 pre-seal authority 和原证据确定性重放裁定，并匹配相关 hash
- LineageAssertions 是普通 proof 的额外 AND guards；不能授予替代 credit，也不能用 future facts 补旧证据
- `complete`、Result accepted、Workspace slot 已空、界面显示完成，是不同事实；完整 closeout 必须原 seal 有效、ordered exact 三 owner ACK 及适用的 post_terminal_delivery lineage guards 全满足

## 5 支持状态机与从属事务

下列支持机器覆盖从创建到退出的周边生命周期，仍各有唯一 owner。22 组件不等于 22 个独立 Runtime 状态机。

| 对象及 owner | 状态或事务 | 合同与边界 |
|---|---|---|
| Workspace creation/specification | 私有 specification request reserved / completed | 保存 exact input/template/configuration/candidate identity；authoring 的 draft/reviewed 等是派生摘要 |
| Workspace confirmation | prepared → materialized 私有 journal | 绑定最终内容和 Goal/DoD after-image；崩溃恢复同一 journal；不是 activation 授权 |
| Workspace admission | eligible / pending_clarification_resolution | 纯 compiler admitted_all_of/clarification_required 是 disposition；由 Workspace promote/seal；匹配 Decision ACK 才释放 pending |
| Workspace lifecycle | active / blocked_on_approval / superseded | ActiveRunID、generation/revision cursor、terminal fence 管 slot；不能替 Run 判生命周期 |
| Decision lifecycle | open / resolved / superseded | resolution proposal pending / committed / rejected；runtime-bound先匹配Orchestrator acceptance；定义批准先Workspace definition-scope preflight，再Decision formal commit/ACK，最后Workspace adoption/journal；proposal不等于resolved/applied |
| Approval lifecycle | pending / approved / rejected / superseded | 创建 Context 是不可变授权对象；ResolutionContext 只审计；exact operation grant 才解除该操作 gate |
| Result lifecycle | draft / accepted / rejected / superseded | 对 terminal material owner-local 幂等应用；产出 exact ResultID/Closeout，不裁定 SM-2 authority |
| Run replacement | clarification_resolution / administrative_termination / successor_materialization 等私有阶段 | 一次 request 一个 successor；先前驱有序终态交付和 lineage CAS，再激活后继 |
| Run replacement recovery | scheduled / running / exhausted / completed | 当前按阶段最多八次失败；exhausted 持久保留，不能悄悄清预算或启动 pending 后继 |
| Workspace deletion | requested → 可选 waiting_for_idle → run_draining → runtime_quiescent → run_retired → owner_sealing → 完成分支 | retain 经 eventstream_finalizing → completed；purge 另经 owner_purging、directory_cleanup；每个 owner 私有 tombstone/ACK，禁止绕过 effect/交付安全 |
| Run retirement / Orchestrator deletion | draining → 精确 quiescence/token inventory → retired；owner-local seal/purge | generation-head fence 与 Runtime write barrier；不允许新 writer 或删除后复生 |
| Memory/Result/Decision/Approval/Eventstream 删除参与者 | 各 owner 的私有 deletion lifecycle | 独立 seal/purge ACK；Workspace coordinator 不跨 owner 直接删数据 |
| Governance trace / File effect gate | trace lifecycle / 私有效应准入 | trace 不替代批准；effect gate 不替代 Runtime authority |
| 内置 Template/Soul upgrade、candidate content | 各自 owner-private transaction | 新配置只进新启动快照，不热改旧 Run；candidate content 不能扩张执行权 |
| workorchd projection lifecycle | 派生投影生命周期 | 只重建显示；Eventstream、缓存、页面与历史 event 序列不能重建 authority |

表中 enum 存在不证明每个值都有公开迁移 API；例如 Decision/Approval 的 superseded 不能只凭声明就编造 public command。全量分类及 transition sink 以两个 pinned manifests 为准。[支持域状态][common]、[confirmation journal][confirmation]、[Decision proposal][decision-proposal]、[Approval reducer][approval]、[deletion][deletion]

### 5.1 Definition变更的Runtime衔接

已决业务链为reasoned ticket → 对最终exact candidate的approve → Workspace版本化confirmation/admission形成new Definition → 在明确启动授权和安全handoff下由Run owner形成new Run。Decision记录授权决议；Workspace负责内容物化/准入；Run-bound Approval仍负责运行治理及风险操作许可。具体字段、版本采纳、approval失效、post-seal journal与错误结果只在[NC04/NC05](contracts/创建与定义变更合同.md)定义。

Runtime算法只消费这些owner事实：不能热改旧Run Definition/bootstrap/dispatch/终态，不从approve自动推断activation，不把审批对象塞入dummy Run。普通显式Restart可复用同Definition，因而不自动要求Definition变更ticket。信息补充未改变Definition时可按exact resolution及continuation ACK继续同Run；目标/验收/约束变化必须新版本新Run。新Run从fresh workspace_full reconciliation开始，不能继承前驱coverage credit。

最新源码已新增Decision contract-change提案/revision登记和Workspace DefinitionChange候选准备、六类review及FinalCandidate反证，属于NC04前半准备子集。原confirmation仍拒绝已有StartupInput、ActivationAdmission或ActiveRunID，authoring successor仍仅覆盖unsealed clarification-blocked，现行Approval dispatcher要求非空RunID；不能从新准备路线推出exact final approval、adoption或new Run已经实施。新支持切点见[RS-DefinitionChange](runtime/状态迁移逐边索引.md#RS-workspace.definition_specification-DefinitionChange)与[RS-contract_change_registered](runtime/状态迁移逐边索引.md#RS-decision.lifecycle-contract_change_registered)；完整实现范围由NC04及总纲登记。[confirmation guards][confirmation-guards]、[successor guards][successor]、[Approval reducer][approval]

## 6 恢复与失败必须分清

| 动作或情况 | identity 与合法行为 | 禁止或停止条件 |
|---|---|---|
| Resume | 同一 current Run 和 immutable bootstrap；active 请求 reconciliation；pending 可激活 | 无 current/terminal 返回 restart_required；blocked 返回 governance_required；不能改 Definition/provider/model |
| 普通显式 Restart | Run-owned replacement，新 Run，可复用同一 Definition | 不自动要求 Definition 变更 ticket；也不从 terminal 自动创建 |
| Definition replacement | reasoned ticket + exact 最终批准 + new Definition + new Run | 禁止 Resume 吸收目标/验收/约束变化 |
| document/transport recovery | 同 logical request/session/purpose；按独立预算取完整 document | submitted 无结论为 provider_submission_unknown，零盲目 provider replay；配置/协议类 terminal 不包装重试 |
| corrective candidate retry | 同 transaction/manifest/stable prefix，attempt 0 → 1 | 只修封存 witness；第二次拒绝终止，不能放宽目标 |
| prerequisite refresh | 同义务，exact stale/dependency invalidation 后重新取 owner facts | read-only no-progress 用独立预算；accepted effect 正常失效不扣恢复额度 |
| semantic reopen / repair | 同 Run、原 Definition/CWI；合法新 binding/代际或同 focus repair | 保留反重放、失败 provenance 与总预算；repair 后必须 fresh validation |
| operation unknown | 先查 exact receipt；SM-4 发恢复授权、claim 后一次 handoff | in_flight 未终结不得二次执行；无 owner 支持的安全 replay/reconcile 合同则 fail closed |
| typed CAS conflict / owner_transient | scoped fresh-load/rebase/backoff，持久 successor | 普通 error 文本不获恢复权限；技术 reload guard 耗尽不等于业务 Run failed |
| terminal delivery failure | 读取原 immutable seal，补 exact 当前 ACK | 不重编 material、不重问模型、不改变终态 |
| 不兼容或损坏持久数据 | owner load 直接 fail closed，保留历史并进入明确运维修复流程 | 不伪造 failed/blocker/fresh activation，不自动迁移、清除或新建替代 Run |

已接纳permit后在pending或active但Runtime尚未初始化处收到合法取消，不能因正常ExecutableAdmission现已撤销而永久锁slot，也不能以不存在aggregate推定零effect。目标[pre-Runtime行政收尾](runtime/状态迁移逐边索引.md#TGT-NC03-AdministrativeCloseout)要求同Run写fence和owner未提交证明、不可转业务的administrative-only上下文及版本化RunAdministrativeIntent终态来源，再走原ordered closeout。普通启动资源失败只blocked；无SM2 sealed failure不造failed；无合法Cancel/policy不造cancelled。此目标新增event/codec/compiler分支未包含在当前76种event实现中。

SM-4 的合法 wake 来自已提交 activation/resume/continuation、授权同步 drive、exact persisted lease、startup inventory recovery 或匹配 owner resolution。wake 只是触发，不带行动权限。Stop 只撤销进程 context、timer/gate，保留持久 lease 和 operation authorization，不能当取消 Run。

每次退出必须留下可证明的下一步：仍持有同一 gate 继续、真实 terminal/governance/wait/lifecycle handoff，或已经持久且成功挂载的 exact successor。CAS/timer mount 失败不能宣称 timer live 或释放为 active orphan；旧 generation/epoch/token 的 timer 不能执行或删除新 successor。[SM-4 规范][machines]、[recoverable wait][wait]

**跨Run未知effect门禁。** 算法固定为：SM4协调持久handoff，capability owner提供exact receipt/reconciliation或可靠隔离事实，H1/H3的producer证据先在Workspace目标handoff journal保存refs；后继A6建立自身aggregate后，SM2才在fresh workspace_full reconciliation接受并gate每个冲突effect；无法判定冲突按可能冲突拒绝，lease过期不证明物理effect未发生。NC05唯一维护逐capability冲突scope、proof责任和解除条件；[附录HandoffBarrier](runtime/状态迁移逐边索引.md#HandoffBarrier)规定slot中间释放及Run terminal屏障。通用跨Run门禁当前未获实现证明，不可写成既成保障。

## 7 跨组件合同的固定格式

每条接入边界形成一张具体 contract card：

- 维护对应资源/状态的唯一能力owner及其具体ID、Command/Query/Data/Result类型与`pkg/events`位置；Command通常由接收能力owner定义，Data通知由源事实owner定义供fanout订阅，调用方不复制同能力topic/DTO
- schema 版本或现有 revision/hash 机制、Workspace/Run/Definition/request/operation 身份及因果关系
- 前置条件、authority/fence/freshness、允许读取和允许 side effect
- 输出事实、私有 internal event、primary SM、完整原子写集和 invariants
- 幂等键；exact duplicate、conflicting duplicate、stale 输入及 response loss 的确定行为
- 预算 owner、消耗与结算时点、错误分类、deadline、恢复权限及终止条件
- 可恢复持久 cut points、兼容/不兼容加载策略、独立测试 oracle 及 producer/consumer fixture

这是评审字段，不要求现有每个 DTO 都硬加同名 Version 或 envelope。禁止 generic command bus、ActionDispatcher、NextAction、StagePacket 或接受外部 target state 的通用接口；不能以“统一合同”绕过 concrete owner contracts。

最重要的同 CAS 写集包括：provider settlement + request disposition；candidate acceptance + dispatch/target/quota；receipt evidence commit + execution/processed identity；governance acceptance + exact continuation。operation recovery authorization 由 SM-4 私有事件授权，SM-3 不直接补写恢复账本。terminal material seal 必须独立 CAS，不能混入 authority、recovery、lease 或 ACK 变更。[写集定义][write-set]、[单一提交边界][coordinator]

## 8 不变量与验收衔接

### 8.1 不可破坏的统一不变量

1. 一个authoritative state一个owner；跨owner只交intent/fact；同owner完整协调一次CAS拒绝torn shape
2. 旧Run的Definition、bootstrap、封存authority/dispatch及terminal材料不可被新配置或UI修改
3. prerequisite未齐或preflight失败零session/该请求预算/provider；request/claim未持久零physical handoff
4. exact replay按完整身份校验复用既有结果；conflicting duplicate拒绝；不新增effect、credit或预算。支持owner私有marker是否返回conflict按附录具体guard，不以泛化幂等口号覆盖
5. receipt accepted、工作轮完成、repair、approval、assessment文本都不能单独授CWI credit
6. 恢复不清零总预算、不绕过fence；unknown不自动重做；未来事实不追溯补旧proof
7. 每个active非终态都有可执行持久义务或明确wait/governance/已挂载successor；无无限轮询和active orphan
8. terminal不重开；原seal、ordered exact三owner ACK及适用delivery lineage guards齐才closeout；中间空slot不解除Run handoff屏障，新Run不复用旧coverage

### 8.2 单一门禁与差距台账

设计冻结、schema接入、组件验证、跨owner集成和系统/live结论必须区分。G0–G4及R/C/B编号仅由[总纲门禁](Workorch_逐项收口总纲与首项方案.md#gates)和[总纲缺口台账](Workorch_逐项收口总纲与首项方案.md#gap-ledger)维护；本文不另编号或复制F1/F2/terminal compiler等剩余差距全文。ImplementationWork的当前admission恒false在§4.1保留为使用本算法的必要现状限制。

SG1为单组件、SG2为真实producer/consumer与跨owner切点、SG3为系统/适用live证据，其定义及用例落点引用[验收规格](validation/验收规格.md)。源码存在、文档审阅、局部测试或单次CI状态均不替代通过声明。

### 8.3 可执行验收入口

[附录逐边索引](runtime/状态迁移逐边索引.md)提供stable RT/R1/RS edge ID、guard、完整写域和源；[AtomicCutPoints](runtime/状态迁移逐边索引.md#AtomicCutPoints)提供必须注入的持久边界；[验收规格](validation/验收规格.md)提供唯一fixture/case/pass-fail oracle。覆盖必须包括normal、exact/conflicting duplicate、stale identity/revision、CAS前中断、CAS后response loss、fresh-process reload，断言StateRevision、真实owner调用、预算delta、effect、receipt/evidence/hash/lineage。

关键交汇点不得只以单模块fake通过代替：preflight期间request变更、ordinary receipt/evidence分切点、working receipt跨SM完整同CAS、all-of N−1、repair新鲜终验、governance先Run ACK、seal后0/1/2 ACK、slot清空但Run未终态、late activation终态重放、replacement exhausted、跨Run unknown冲突及不兼容/torn load。具体fixture字段和编号只在验收规格维护。

22组件接入和分层检验见[组件固定合同与分层验收规范](Workorch_组件固定合同与分层验收规范.md)。实现若要求改变目标/验收/已批准约束，回NC04请求reasoned change ticket，不以修复恢复逻辑为由偷偷放宽。

## 9 固定版本来源

产品目标以本批[统一业务模型](Workorch_统一业务模型与交互关系.md)及NC01–NC05为准；算法现状固定于下列commit。本文v0.3在v0.2逐边基线上核对新源码并迁移Workspace路径，没有借刷新改变目标、验收或合同约束。Workorch 源码链接均固定 commit，路径已对照该 commit tree；方法参考 [go-multi-module-dev v2.6.0 固定版本](https://github.com/muidea/skills-repo/blob/6ba50452fccd33c912b07a0648fbb97acf15d756/skills/go-multi-module-dev/SKILL.md)。源码事实、规范要求和本文提出的目标门禁已分别标明，未把设计要求当作已验收能力。

[contract]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/docs/00-core/runtime-stage-contract.md
[machines]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/docs/00-core/runtime/state-machines.md
[models]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/scripts/state-model-owners.manifest
[transitions]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/scripts/state-transition-owners.manifest
[creation]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_authoring_creation.go
[admission]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_admission.go
[activation]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/docs/00-core/definition-convergence/activation-flow.md
[common]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/common/states.go
[run-transitions]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/state_transitions.go
[run-intent]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/workspace_execution_intent.go
[reducer]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime/reducer.go
[drive]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_drive.go
[write-set]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime/internal_event_write_set.go
[coordinator]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime/transition_coordinator.go
[implementation-state]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_implementation_work_state.go
[implementation-policy]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_implementation_work_policy.go
[confirmation]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation_journal.go
[decision-proposal]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/models/runtime_governance_proposal.go
[approval]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/lifecycle_internal_event.go
[deletion]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/workspace_deletion.go
[confirmation-guards]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_authoring_confirmation_internal_event.go
[successor]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_authoring_successor.go
[wait]: https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/docs/00-core/runtime/recoverable-wait.md

## 10 版本记录（2026-10-12）

历史基线为`7993f63c4437dbf65c33c2e844683fd6389d63e0`（2026-10-10），本版固定`5360caf36e081717087557f34d2af3af1c16dc16`，前进120 commits。四机owner、76私有event、20条SM1边不变；Workspace移至kernel，29个family不变，支持切点由95增至97。源义务结构链和terminal current-proof过滤由旧缺口推进为有源码实现；ImplementationWork、administrative-only、两个late入口及完整NC04/NC05交接不得据此升级为已验收。新增/保留源码状态回写总纲唯一gap账本；本页只记录版本影响，不另起实施台账。所有测试结论均为本轮未运行。
