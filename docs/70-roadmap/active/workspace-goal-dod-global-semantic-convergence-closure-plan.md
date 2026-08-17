# Workspace Goal/DoD Global Semantic Convergence Closure Plan

Status: active

Type: global-code-closure-plan

Last Verified: 2026-08-17

## 1. Purpose And Unique Scope

本文是 Workspace 从正式 Goal/DoD 到 CWI 持续收敛、最终 artifact 清单和完成报告的**全局唯一活动期代码收口计划**。
它统一覆盖：

- 大量 Goal/DoD 的稳定定义、依赖识别与完整性校验；
- 基于语义关系而非序号/ID 的 CWI 优先级与执行顺序；
- 有界 active focus、semantic proof batch、稳定 Definition 分区和 provider context；
- CWI settled/reopened 后的 owner-local 重投影；
- 单 Owner、四状态机、typed EventHub 和 terminal delivery 边界；
- 对当前全量 `coverageOrder`、Definition-order fallback 和每次请求重复完整 Goal/DoD 正文的替换与删除。

本范围内不得再创建第二份 active 设计、组件子计划或 live 热修计划。后续发现的新反例必须映射到本文的同一不变量、
工作项和验收矩阵；若需要补充方案，直接修改本文。已归档计划只能作为回归证据，不能重新成为实现约束。

本文是实施顺序，不覆盖以下更高权威：

1. [Runtime 核心合同](../../00-core/runtime-stage-contract.md)；
2. [Runtime request/cache note](../../00-core/runtime-request-cache-note.md)；
3. [Orchestrator 实现合同](../../../internal/modules/kernel/orchestrator/readme.md) 与
   `internal/pkg/workorchruntimecontract`。

当前 request/cache 合同仍要求 C1 携带完整且不压缩的 Goal/DoD；当前实现还保留 provider-authored
`coverageOrder`，但核心合同并未授予 LLM 全局排序或下一 action 权威。后者是需要删除的实现偏差，不是可继续扩展的
设计能力。

本文描述的是目标代码合同，不冒充当前生产事实。C1 分区、semantic scheduling 和移除 `coverageOrder` 只有在 WGSC-00
先完成权威文档对齐，并由后续工作项在同一 current-baseline 切片中完成代码、fixture 和验证后，才能成为生产行为；
在此之前不得用本文重解释已部署 aggregate，也不得提前把 implementation SoT 改写成尚未落地的事实。

### 1.1 Current/target boundary

| Topic | Current baseline | Target after this plan |
| --- | --- | --- |
| Definition cache prefix | 每个 semantic request 的 C1 携带完整 Goal/DoD | Workorchd 静态编译 immutable Definition partitions，Run owner 随 bootstrap 封存；同一 partition 内 C1 稳定，跨 partition 显式开启新 stable-prefix generation |
| Additional definition context | 依赖完整 C1 | manifest-authorized exact cross-partition text 只进入 C5 bounded expansion，不改写 C1 |
| CWI order | 当前实现仍有 provider `coverageOrder` 和 Definition-order fallback | SM-2 只从 sealed dependency graph、current facts 和 ID-free semantic policy 投影 ready focus |
| Runtime readiness | static Definition 与 dynamic owner readiness 尚未明确分层 | Workorchd 只做 static compile；Run 创建后由 SM-2 通过 typed owner facts 完成 dynamic readiness |

该表是迁移边界，不是双主线。任一 topic 完成生产切换时，必须在同一提交中删除对应 current-baseline surface，并更新
core note、implementation SoT、flow、verification 和 current-state；禁止长期保留 “current/target” 双写。

[Runtime Positive-Convergence Live Closure](runtime-single-owner-model-capability-output-closure-plan.md) 继续只承担部署态
正向链验证，不是本代码收口的第二份设计。本文 code-closed 后，相关 fresh-data/live 场景并入该唯一部署验证入口。

## 2. Product Outcome

目标 Workspace 流程固定为：

```text
1. user confirms Template + Goal/DoD; directory is optional and always resolves
   -> Workspace owner seals one definition revision and owner-owned semantic declarations

2. fresh activation preparation
   -> Workorchd compiles the complete neutral CWI dependency contract and immutable Definition partitions
   -> Run owner seals the typed bootstrap and performs its SM-1 owner-local activation
   -> SM-2 acquires dynamic owner prerequisites, validates and seals one scheduling contract
   -> SM-2 projects one bounded ready window
   -> SM-3 requests bounded per-CWI proof only
   -> capability owners return feasibility / operation receipts
   -> SM-2 seals binding, credits evidence and advances/reopens CWI
   -> repeat from the persisted dependency graph until all required CWI settle

3. terminal
   -> SM-2 seals artifact/coverage closeout
   -> SM-4 delivers Result -> Workspace -> Run
   -> application projects the completion report
```

排序不是一次性的全量线性计划。开始执行前封存的是：

- 完整 CWI 语义依赖 DAG；
- 可选的无状态 dependency gates；
- 可解释、确定性的语义排序策略；
- Definition revision、graph hash 和 scheduling-contract hash。

初始拓扑波次和每次 ready queue 都是上述合同对当前 persisted facts 的纯投影。CWI settled、blocked、retired 或 reopened 后，
SM-2 使用同一合同重新投影，不由 Application、LLM、projection 或 capability owner维护另一份队列。

Template 特定的 planning、execution、decision、approval、validation 和 artifact 等业务步骤，只能由
Template/Profile 编译为中立 relation、evidence obligation、governance dependency 或 artifact obligation；它们不是新的
kernel phase、状态机或固定全局流水线。SM-2 仍只按 current neutral graph/facts 收敛。

## 3. Current Baseline And Gaps

### 3.1 已有正确基础

- Workspace block 是 Workspace、Goal、DoD 和 Template 的唯一 owner。
- Workorchd activation compiler 生成 typed focus、acceptance slice 和 CWI。
- Profile 可以声明 `RequireSourceSettled` 与 evidence-triggered reopen dependency。
- `ValidateRuntimeCoverageItemDependencies` 已拒绝缺失引用、自依赖和 prerequisite/reopen cycle。
- `RuntimeCWIsEligibleForPlanning` 已要求所有 prerequisite 当前为 `settled + satisfied + current binding`。
- semantic proof batch 已受 `MaxCandidateContextItems` 约束，并包含 current focus 的必要 dependency closure。
- bound/effect-fenced current focus、duplicate、stale、restart 和 CAS 仍受现有 SM-2/SM-3/SM-4 不变量保护。

### 3.2 必须关闭的偏差

1. Workspace DoD 仍是 `[]string`，没有 item-level 稳定语义 identity、显式关系和 requires/provides 声明。
2. Workorchd 仍按数组位置生成 `dod-001`、`cwi-001`、`Order`；这些技术编号可能进入 fallback 执行顺序。
3. 依赖主要来自 Profile selector，compiler 明确不推断 Definition 的个性化语义关系。
4. 同一批 eligible CWI 默认按 Definition order 排序；presentation priority 与真正 execution priority 尚未形成完整语义模型。
5. semantic provider 仍需为 exact eligible closure 返回完整 `coverageOrder` permutation；CWI 越多，上下文和输出成本越高。
6. 当前 C1 每次包含完整 Goal/DoD；定义规模增大后，即使内容稳定，也会增加输入 token、噪声和 provider 输出偏离风险。
7. `MaxFocuses` 同时限制总定义规模和当前请求规模，尚未把“完整 CWI ledger”与“有界 active window”分离。
8. Profile 多 source × 多 dependent 关系会展开为大量直接边；大型 fan-in/fan-out 缺少无状态 gate 表达。
9. 排序只能处理已知依赖；缺失的小型前置项目前不能在首次 durable dispatch 前统一暴露。
10. Definition item 的增删改退役、Goal/DoD disposition 和 item→CWI cardinality 尚未形成唯一合同。
11. `any_settled` 只有满足谓词，没有绑定 current member/evidence 的 causal witness，重开后无法证明 focus 仍由当前事实支持。
12. 当前排序指标包含 transitive downstream count，却同时承诺 `O(N+E)`，性能合同不可同时成立。

## 4. Non-Negotiable Invariants

### 4.1 单 Owner 与四状态机

- Workspace owner 独占正式 Goal/DoD、item identity、用户确认关系和 definition revision。
- Run owner 独占 SM-1，并仅 CAS 持久化 Orchestrator opaque Runtime aggregate。
- Orchestrator 独占 SM-2、SM-3、SM-4；只有 SM-2 private internal event 可封存 scheduling contract、选择 focus、
  retain/reopen CWI 或授予 credit。
- Workorchd 只读取 owner snapshot、编译 typed bootstrap 和提供 UI/API 用例，不拥有 Runtime scheduling state。
- LLM 只提出 setup draft 或 per-CWI proof proposal，不写 dependency、priority、focus、binding、credit 或 terminal。
- capability owners 只发布 readiness/feasibility/receipt facts，不要求接收方进入某个状态。
- projection/Eventstream/TUI/Web 只显示已持久化事实。
- 不增加第五状态机、新 framework unit、跨 owner repository/service 或通用 EventHub envelope。

### 4.2 禁止按序号和 ID 推断业务顺序

以下值**不得进入 execution rank tuple，也不得用于推断 prerequisite、业务优先级、阶段或上下游关系**：

- Definition 数组位置；
- `RuntimeCoverageItem.Order`；
- acceptance slice 的 presentation ordinal；
- `goal-*`、`dod-*`、`cwi-*`、`focus-*`、`slice-*` 的数字后缀；
- ID 的字符串、字典或数值大小；
- repository/event 的创建顺序；
- projection 列表顺序。

ID 只允许用于 identity、引用、scope、hash 输入去重、stale/duplicate 校验和审计。业务排序必须来自已封存的语义关系、
用户/Profile 明确业务优先级、下游解锁度、关键路径和声明式成本等级。

完全相同的语义 rank 先比较 `SemanticTieBreakHash`；它只由规范化语义角色、正文 hash、requires/provides、关系类型、
业务优先级和成本等级计算，不读取 identity、数组位置或数字后缀。hash 只用于索引和快速比较，不能把碰撞当作语义相等：
hash 相同但 canonical semantic signature bytes 不同时，继续比较 signature bytes；signature 完全相同且不是显式
commutative group 时，activation 必须以 `ambiguous_semantic_duplicate` fail-closed，不得退化为按 ID 排序。
显式声明为 commutative group 的成员可以任意执行，但首次 SM-2 选择必须持久化，restart 复用该选择而不重新排序。

### 4.3 依赖先于排序

排序只能处理已知关系。任何实现都必须遵守：

```text
definition itemization
  -> dependency compilation
  -> completeness / cycle / ambiguity validation
  -> scheduling-contract seal
  -> bounded readiness preflight
  -> first durable dispatch
```

缺失 prerequisite、未解析 provider、循环、歧义或超限必须在首次副作用前 typed fail-closed。不能依靠执行失败后反复
replan 来补齐本可在 Definition 阶段发现的依赖。

### 4.4 Partial Order，而非全量固定队列

- authority 是 dependency DAG + scheduling policy，不是 LLM 生成的完整线性序列。
- 只有当前 ready 节点可参与下一 focus 选择。
- bound、active dispatch、accepted effect 或 evidence lineage 的 current focus 必须保持 fenced。
- reopen 只影响其语义后继闭包；不重排无关 settled branch，不重新发送全部 Goal/DoD。
- Definition 关系变化必须使用新 definition revision 和 fresh activation/restart；active Run 不原地修改 graph。

## 5. Canonical Definition And Dependency Model

### 5.1 Workspace-owned semantic items

目标 Workspace definition 使用 owner-owned typed item，而不是只有位置化字符串：

```text
WorkspaceDefinitionItem
  ItemRef                 stable owner-issued opaque identity
  Kind                    goal | dod
  Text                    exact user-confirmed text
  SemanticRole            profile/user-confirmed opaque role
  CoverageDisposition     required_coverage | constraint | context_only | aggregation
  BusinessPriority        critical | high | normal | low | deferred
  CostBand                tiny | small | medium | large | unknown
  RequiresRefs[]          opaque required input/result refs
  ProvidesRefs[]          opaque produced result refs
  DependsOnItemRefs[]     explicit hard prerequisites
  VerificationOfRefs[]    items whose stable result is verified here
  ContextItemRefs[]       prompt context only; never an execution gate
  InvalidationRefs[]      relations that may reopen this item
  CommutativeGroupRef     optional
  SupersedesItemRef       optional; only for an explicit semantic replacement
```

字段含义保持 owner-neutral。软件开发中的 scaffold、source、test、report 或 commit 等具体语义只能来自
software-dev Template/Profile；Orchestrator 不识别这些词。

普通 TUI/Web 用户仍只需填写 Template、Goal、DoD；workspace directory 是可选输入，空值按产品规则解析为
`.workorch-workspaces/<workspace-id>`。`BusinessPriority=normal`、`CostBand=unknown` 和 Template/Profile 的确定性
disposition/relation 默认值必须足以完成普通 setup。只有用户主动采用 assisted draft，或 deterministic compile 返回
`definition_clarification_required` 时，UI 才增加可读确认步骤；不得要求用户提交 Runtime JSON、target、recipe、check、
metric、artifact 或 capability readiness。

### 5.2 Item lifecycle and Definition→CWI mapping

`ItemRef` 不是正文 hash、数组位置或可复用编号。Workspace owner 按以下规则维护 identity：

- 同一语义义务的正文澄清、priority/cost 或 relation 调整保留 `ItemRef`，但产生新的 semantic Definition revision；
- 义务被实质替换时，旧 item 在新 revision 中进入 immutable retired history，新 item 取得新的 `ItemRef`，并可用
  `SupersedesItemRef` 建立审计 lineage；
- 删除等同 retire，retired identity 永不复用；新增永远取得新 identity；
- 仅 presentation 分组/排序变化写入 owner 的 projection preference，不改变 semantic Definition revision、graph 或 rank；
- active Run 永远绑定创建时的 revision。任何 add/edit/retire/relation change 只影响显式 fresh activation/restart，
  Resume 不迁移 graph、partition、CWI 或 credit。

Workorchd static compiler 必须生成完整映射：

```text
RuntimeDefinitionCoverageMapping
  DefinitionRevision
  ItemRef
  Disposition
  PrimaryCoverageRefs[]
  ConstraintConsumerCoverageRefs[]
  AggregationGateRefs[]
  ContextConsumerCoverageRefs[]
  MappingHash
```

映射 cardinality 固定为：

- `required_coverage` item 至少映射一个 CWI；一个 item 可拆成多个独立 CWI；
- `required_coverage` item 只有在其全部 `PrimaryCoverageRefs` 都以 current binding/evidence settled 后才满足；
- 每个 CWI 恰好有一个 `PrimarySourceItemRef`，并可附加多个 constraint/context source ref。一个 receipt 不得借
  many-to-many 映射扇出关闭多个不同 primary obligation；
- `constraint` 不直接取得 credit，但必须至少约束一个 CWI、binding 或 gate；
- `context_only` 只允许进入 manifest-authorized context，不创建 gate、CWI credit 或 terminal obligation；
- `aggregation` 必须映射到非 executable gate/terminal coverage relation，不能伪装为 operation CWI；
- 每个 CWI、gate 和 constraint consumer 都必须可反向追溯到同一 Definition revision 的 item。

Template/Profile 必须提供中立默认规则。默认产品流将可确定关联的 DoD 编译为 `required_coverage`，将其 Goal 编译为
`all_of aggregation`；`any_of` 必须是用户确认或 Profile 显式声明，不能由 LLM/Workorchd 猜测。没有 DoD 关联的 Goal
只能由 Profile 明确编译为 `required_coverage`，或在 Run 创建前返回
`definition_clarification_required`。Orchestrator 不按 `Kind=goal|dod` 自行猜 disposition。

### 5.3 Canonical relation classes

| Definition relation | 编译后的 Runtime 语义 | 是否阻塞执行 |
| --- | --- | --- |
| explicit `depends_on` | `requires_settled` direct edge | 是 |
| matching `requires/provides` | `requires_settled` direct edge 或 gate | 是 |
| `verification_of` | subject stable 后才可验证的 hard edge | 是 |
| `invalidated_by` | evidence-kind constrained reopen edge | 否；命中后重开 |
| `context_ref` | bounded request inclusion relation | 否 |
| `all_of/any_of` aggregation | derived dependency gate | 取决于 gate mode |

每条 canonical relation 保存 source、dependent、kind、origin 和 Definition revision。`origin` 只允许：

- `definition_confirmed`：用户确认，包括 LLM-assisted draft 被用户确认后的结果；
- `profile_declared`：Template/Profile 的领域声明；
- `input_output_compiled`：Workorchd 对 opaque requires/provides 的确定性匹配。

LLM 未确认草案、自由文本相似度、ID/序号、文件名、operation 名和错误消息不能成为 relation origin。
其中只有 `definition_confirmed` relation 是 Workspace owner 可修改并持久化的 Definition 数据；
`profile_declared` 来自已选 Profile snapshot，`input_output_compiled` 是 Workorchd 对已封存声明的纯派生 bootstrap
fact。Workorchd 不成为 relation owner；Run owner 随 bootstrap 冻结来源，SM-2 只在 scheduling contract 中
封存经校验的 compiled relation/hash。

### 5.4 Dependency gates

大型 fan-in/fan-out 使用无状态 gate，避免把 selector 直接展开为 `N×M` 边：

```text
RuntimeCWIDependencyGate
  GateRef
  Mode                    all_settled | any_settled
  MemberCoverageRefs[]
  MemberSetHash
  DependentCoverageRefs[]
```

Gate 是 SM-2 的纯派生谓词：

- 没有 lifecycle、repository、CAS 或 EventHub topic；
- 不产生 candidate、operation、receipt、evidence 或 credit；
- member set 在 activation 时封存，运行中不能动态扩张；
- cycle validation 必须把 gate 当作图节点处理；
- member/edge 数量与 hash 必须进入 scheduling-contract identity。

当 gate 第一次支持 focus install 时，SM-2 还必须把 causal witness 封入 focus/authority lineage：

```text
RuntimeCWIDependencyGateWitness
  GateRef
  SchedulingContractHash
  FactsRevision
  Entries[]               coverage ref + current binding ref + satisfaction evidence refs/hash
  SatisfiedMemberRefs[]
  WitnessSetHash
```

- `all_settled` witness 必须覆盖全部 member；`any_settled` witness 保存投影时全部 current satisfied member，而不是只记一个
  无来源布尔值；
- witness entry 必须逐项匹配 current binding 与 accepted evidence lineage；summary、LLM claim、ID 或旧 credit 不能成为
  witness；
- 新 member settled 不使已封存 `any_settled` witness stale；但 dispatch 前至少一个已封存 entry 必须仍 current；
- member reopened、binding/evidence stale 或 reconciliation 改变时，SM-2 重新计算 witness。`all_settled` 缺任一 member，
  或 `any_settled` 已封存 entry 全部失效时，dependent 退出 ready，未 effect-fenced focus 退休并重投影；
- effect-fenced focus 不得静默换 witness 或换 CWI：SM-3 先按原 fence 收敛 receipt/outcome，SM-2 再把失效 witness 记录为
  causal stale，拒绝从该 lineage 新增 credit，并按同一 graph reopen/reproject dependent；
- witness 变化、dependent reopen 和 evidence credit 使用同一 private internal event/CAS；duplicate witness hash 为零 revision；
- gate 本身仍没有 lifecycle。witness 是 SM-2 authority fact，不是第五状态机、独立 repository 或跨 owner DTO。

### 5.5 Two-phase completeness validation

Workorchd activation compiler 必须在创建 Run 前完成 static Definition validation：

1. 所有 item/relation/ref 均属于同一 Workspace 和 Definition revision；
2. 无空 identity、悬空引用、自依赖或重复冲突关系；
3. hard dependency 与 reopen graph 分别无 cycle；
4. 每个 `RequiresRef` 恰好由初始事实、一个 provider item 或显式 `any_of` gate 满足；
5. 多 provider 歧义没有显式 gate 时拒绝；
6. 每个 disposition 与 Definition→CWI mapping 满足第 5.2 节 cardinality；
7. required aggregation/finalization gate 的 member set 完整且 hash 稳定；
8. semantic duplicate 已合并、声明为 commutative，或 fail-closed；
9. immutable Definition partition 完整覆盖 active item，每个 CWI 绑定一个 home partition，partition 无超限或正文摘要；
10. Definition item、edge、gate、gate membership、partition 和总字节数符合彼此独立的容量预算；
11. 不因 context budget 较小而裁剪 authority inventory；
12. 编译结果在输入 slice/map permutation 后保持同一 semantic graph hash。

Run 创建后、首次 semantic provider request 或 durable effect 前，SM-2 再通过 concrete typed owner command/result 完成
dynamic readiness validation：provider ceiling、capability/policy/catalog revision、validation surface、target selector 和
必要 governance contract 均必须 current。Workorchd 不读取 capability repository、不探测 owner，也不把“当前可用”写入
bootstrap。static ambiguity 返回 Definition clarification；dynamic unavailable/stale 使用现有 SM-2 prerequisite/
semantic-contract typed fail-closed 主线，两者不得混成 setup LLM 猜测。

## 6. Semantic Scheduling Policy

### 6.1 Ready gate

一个 CWI 只有同时满足以下条件才进入 ready set：

- convergence state 为 `unbound | planning | reopened`；
- 所有 direct prerequisite 为 current `settled + satisfied + CurrentBindingRef`；
- 所有 `all_settled/any_settled` gate 已满足；
- Definition/scheduling contract、reconciliation 和 current catalog lineage 未 stale；
- 它不是另一个 bound/effect-fenced current focus 的非法替换。

任何 priority 都不能越过 ready gate。

### 6.2 Semantic rank tuple

SM-2 使用 Kahn topological ready queue，并为每个 ready CWI计算可解释的词典序 rank：

```text
EffectiveBusinessPriority                 ASC
ImmediateHighPriorityReadyUnlockCount     DESC
CriticalPathDepth                         DESC
ImmediateReadyUnlockCount                 DESC
CostBand                                  ASC
SemanticTieBreakHash                      ASC
CanonicalSemanticSignature                ASC  (hash-collision only)
```

各字段定义如下：

- enum 比较顺序固定为 `critical < high < normal < low < deferred` 和
  `tiny < small < medium < large < unknown`；空值在 static compile 时规范化为 `normal/unknown`，不得读取 wire enum、
  ID 或数据库顺序；
- `EffectiveBusinessPriority`：自身与所有未完成后继中最高的声明式业务优先级。高优先级目标的低层 prerequisite 会继承该
  优先级；它通过 reverse-topological max propagation 计算，不物化 transitive closure。
- `ImmediateHighPriorityReadyUnlockCount`：当前项 settle 后，unsatisfied prerequisite/gate count 将归零且 effective priority
  为 critical/high 的**直接** dependent 数量。
- `CriticalPathDepth`：从本项到 required terminal gate 的最长 remaining hard-dependency path，通过 reverse-topological
  dynamic programming 计算。
- `ImmediateReadyUnlockCount`：当前项 settle 后会直接进入 ready 的 CWI/gate 数量；它不统计全部后继闭包。
- `CostBand`：只在解锁价值相同后使用，优先完成小型前置项；不得实现为全局 shortest-job-first。
- `SemanticTieBreakHash`：只加速 ID-free semantic signature 比较，不表达业务价值。
- `CanonicalSemanticSignature`：只在 hash collision 时比较规范化 bytes；它不含 identity、位置或 presentation 顺序。

禁止在 rank 中重新引入 transitive descendant count、reachability bitmap size 或全闭包 cardinality。priority propagation、
critical depth 和 direct unlock counter 都必须在 CWI + gate 扩展 DAG 上以 `O(N+E+G)` traversal 完成，其中 `G` 是 gate
membership 数；canonical serialization/sort 的复杂度单独按第 12.5 节核算。

禁止把 `RuntimeAcceptanceSlice.Priority` 直接复用为 execution priority。它继续只服务 presentation/request grouping；
若 Profile 需要业务优先级，必须显式编译为 `BusinessPriority`。

### 6.3 Dynamic ready projection

开始执行前，SM-2 封存：

```text
RuntimeCWISchedulingContract
  DefinitionRevision
  DefinitionSeedHash
  DefinitionCoverageMappingHash
  DefinitionPartitionSetHash
  SemanticGraphHash
  SchedulingPolicyHash
  DependencyGates[]
  StaticRankInputs[]
  ContractHash
```

每次 persisted transition 后，纯 evaluator 输出：

```text
RuntimeCWIReadyProjection
  ContractHash
  FactsRevision
  ReadyCoverageRefs[]
  BoundedActiveWindow[]
  GateWitnesses[]
  EffectiveRanks[]
  BlockedReasons[]
  FencedCurrentCoverageRef
  ProjectionHash
```

`BoundedActiveWindow` 不是第二份 persisted scheduler。只有既有 Orchestrator private SM-2 event 可把它安装到
`Focus/RemainingFocuses`，并同时封存该选择使用的 gate witness、effective rank 与 Definition partition ref；
duplicate projection 为零 revision，CAS conflict 必须 reload 后重新计算。

### 6.4 Stability barrier

会改变同一语义资源闭包的 CWI 必须通过 Profile/Definition hard edges 位于其 verification、aggregation 和 finalization
之前。这样可避免每完成一个小修改就过早验证，随后又因同一上游变化重开。

Orchestrator 不理解“源码”“测试”或“提交”；它只执行已封存的 hard edge、gate 和 evidence-triggered reopen relation。

## 7. Large Goal/DoD Context Strategy

### 7.1 Authority and immutable Definition partitions

完整 Workspace definition、Definition→CWI mapping、CWI inventory、dependency graph、partition set 和 scheduling contract
必须随 Run 持久化，不能因 provider context budget 而丢失。Workorchd 在 activation 前从同一 semantic Definition revision
确定性编译：

```text
RuntimeDefinitionPartitionSet
  DefinitionRevision
  DefinitionRootHash
  Partitions[]             immutable member refs + exact text + partition hash
  CWIHomePartitionRefs[]   exact CWI -> partition mapping
  AuthorizedExternalRefs[]
  PartitionSetHash
```

每个 active Definition item 恰好属于一个 home partition；每个 CWI 恰好绑定其 primary item 所在 partition。partition
内容不读取 current focus、evidence、failure、owner catalog 或 provider output，Run 内不能扩张、缩减或重写。

目标 C0～C5 保持：

| Section | 目标内容 |
| --- | --- |
| C0 | 中立 Runtime/semantic schema 和稳定规则 |
| C1 | `DefinitionPartitionEnvelope` + 当前 partition 全部 member 的 exact Goal/DoD 正文 |
| C2 | Template/Profile/capability/governance 的 immutable 声明 |
| C3 | current CWI/focus、required output、schedule/gate-witness/binding refs；不复制 Definition 正文 |
| C4 | provider-safe failure、evidence、attempt/correction diagnostic |
| C5 | manifest-authorized、content-addressed exact cross-partition context 与有界 trace |

“Goal/DoD 不压缩”和 stable-prefix 规则具体化为：

- canonical full Definition 在 Workspace owner 与 Run aggregate 中完整保留；
- C1 中 partition 的每个 member 都使用 exact、未摘要、未损失正文，不允许按 current focus 再切片；
- C1 只携带 Definition root identity、current partition envelope 和该 partition 正文，不线性复制其他 partition；
- current focus 在同一 partition 内变化时只改变 C3，C0/C1/C2 和 `StablePrefixHash` 必须保持不变；
- attempt 0/1、whole-document recovery 和同 semantic transaction 的所有 physical call 必须复用同一
  `DefinitionPartitionRef/Hash` 与 stable prefix；
- 从一个 home partition 切到另一个时，SM-2 先持久化新的 logical request 和
  `StablePrefixChangeDisposition=definition_partition_switch`。这是显式新 stable-prefix generation，不是 renderer drift；
- 同一 Run 内只有 sealed partition switch 可以预期改变 stable prefix；Definition revision 或 C2 immutable contract
  revision 变化必须使用 fresh activation，不能 patch 当前 Run。除此以外的 C0/C1/C2 hash 变化一律是
  `unexpected_stable_prefix_drift`。

### 7.2 Partition compilation and request selection

partition 先按 Template/Profile 的 semantic role、用户确认 group、hard relation 和 requires/provides closure 聚合，再在
`DefinitionPartitionBudget` 内确定性拆分。拆分与排序只能使用 canonical semantic signature，不能使用 ItemRef、数组位置
或 presentation order。一个不可再拆的 mandatory exact-context closure 超限时，activation 返回
`definition_partition_capacity_exceeded`，不得摘要正文或在 Runtime 临时重分区。

一个 Runtime semantic/proof request 默认包含：

1. current CWI 的完整 home partition C1；
2. C3 中 direct prerequisite、gate witness、verification 和 binding 的 current identity/evidence refs；
3. 当前 Profile/C2 声明；
4. manifest 已授权且确有需要的 cross-partition item，作为 C5 exact content-addressed entry；
5. 固定上限的同 ready window semantic signatures，仅用于局部对照，不携带未授权 Definition 正文。

`context_ref` 与 `requires_settled` 必须是两个字段和两个 evaluator。prompt 需要参考某项不能自动阻塞执行；执行依赖某项也
不能自动要求重复其正文。ready window、rank 或 focus 变化不得改变 C1 partition 内容。

一次 SM-3 semantic proof batch 必须 partition-homogeneous：SM-2 先从全局 ready rank 选择最高项及其 home partition，
再只允许同 partition 的其他 ready CWI 在预算内作为预取 proposal 加入 batch。partition affinity 不能进入 rank、越过更高
优先级 CWI 或强制下一 focus；每次 settle/reopen 后仍从全局 ready set 重算，必要时显式 switch partition。跨 partition
CWI 不得为了复用一次 provider request 而只发送摘要 Definition。

### 7.3 Bounded cross-partition context expansion

若当前 partition + C3 refs 仍不足，semantic document 只可从 manifest 已授权的 `ContextItemRefs` 枚举返回
`context_insufficient + requestedRefs`。SM-3 先持久化该完整 factual document；SM-2 校验 refs、content hash、预算和
lineage 后，通过 private event 封存下一份 C5 expansion 并创建新 logical request。

- expansion 不修改 C0/C1/C2、Definition partition 或 `StablePrefixHash`；
- 下一 logical request 的 C5、volatile-tail hash 和 context-expansion generation 必须变化；同 request 的 attempt 0/1
  继续复用完全相同的 C0～C3 与 C5 expansion；
- expansion 不消耗 candidate attempt 0/1、repair 或 prerequisite-refresh budget；
- 不能请求 manifest 外 item、改变 dependency graph、切换 home partition 或提升 priority；
- current primary item 不在 C1 home partition 是 compile/invariant defect，不能用 C5 补救；
- budget 耗尽以 typed context-integrity failure fail-closed；raw provider text 不能决定 expansion。

### 7.4 Optional hierarchical setup assistance

普通 setup 先应用 Template/Profile 的确定性 disposition、priority/cost default、requires/provides 和 relation rules；只要
static validation 完整，就不调用 LLM，也不增加额外确认页。大量 Goal/DoD 需要辅助时，用户可显式开启 assisted draft：

1. Template、用户分组、semantic role 和 requires/provides 先形成确定性局部分区；
2. 每个有界分区向 LLM 提供 exact item text，生成组内关系草案；
3. 跨组请求只携带组 manifest、semantic role、requires/provides 和 hash，筛选候选跨组关系；
4. 只对候选关系的两个端点再次加载 exact text 复核；
5. UI 展示依赖、priority/cost、disposition、假设、cycle/缺失警告；用户明确采用后才提交 Workspace owner；
6. 未采用草案可丢弃，进程重启可丢失，不得进入 RuntimeBootstrap。

LLM 草案中出现的 ID 顺序、输出数组顺序和自然语言“先做第一个”均无 authority。deterministic compile 仍有歧义时，只能返回
可读的 `definition_clarification_required`；不得要求用户补 raw Runtime JSON，也不得把 LLM 草案自动升级为 canonical
relation。

### 7.5 Independent budgets

下列预算必须使用不同字段、validator 和 audit，禁止因数值相同而互换：

- `DefinitionInventoryBudget`：完整 owner definition/CWI 数量与字节；
- `SchedulingGraphBudget`：edge、gate、gate membership、fan-in/fan-out 和图深度；
- `DefinitionPartitionBudget`：单个 immutable C1 partition 的 item/byte 上限；
- `DefinitionPartitionSetBudget`：partition 数量与 mapping bytes；
- `ActiveFocusWindowBudget`：一次 SM-2 ready window；
- `SemanticProofBatchBudget`：一次 per-CWI proof batch；
- `DefinitionExternalContextBudget`：一次 C5 exact external context；
- `DefinitionContextExpansionBudget`：同 semantic transaction 的 expansion generation 总数；
- 现有 Owner inventory、candidate、request recovery、repair、prerequisite refresh 预算。

总 Definition 超过安全上限必须在 activation 前返回 `definition_capacity_exceeded`，不得静默截断。正常大 Definition 只限制
partition、active window、provider batch 和 external context，不限制 canonical CWI ledger 的完整性。

## 8. State Machine Closure

| SM | 目标变化 | 不变边界 |
| --- | --- | --- |
| SM-1 | 无业务变化；Run seal fresh bootstrap/current-baseline schema | 不选择 dependency、priority、focus 或 next action |
| SM-2 | 验证并封存 scheduling contract/partition selection/gate witness；纯投影 ready window；settle/reopen 后重投影 | 不调 provider、不执行 operation、不解析领域词汇 |
| SM-3 | 删除全量 `coverage_order` document；只处理 bounded proof batch、context request 和 operations candidate | 不选择下一 CWI；不写 scheduling graph |
| SM-4 | drive 前要求 scheduling contract/ready projection 完整；Resume/load 从同一 aggregate 幂等恢复；Restart 只走既有 replacement/delivery lane | 不从 projection/LLM 重建 graph，不修改其他 owner state |

目标 owner-local 顺序：

```text
activation fact
  -> SM-2 cwi_scheduling_contract_sealed
  -> SM-2 eligible_focus_projected + partition/witness sealed
  -> SM-3 proof-batch request/evaluation
  -> owner feasibility
  -> SM-2 binding/reconciliation seal
  -> SM-3 operations request/sealed dispatch/receipt/evidence
  -> SM-2 settle or reopen + eligible_focus_projected
```

`cwi_scheduling_contract_sealed` 是建议的 Orchestrator private fact 名称；实现必须复用唯一
`ApplyInternalEvent -> RuntimeTransitionCoordinator.Commit` mutation/CAS 边界。它不是 EventHub topic，也不允许 Application
携带 caller-authored ranks、ready queue 或 target state。

### 8.1 Governance, Resume and Restart

- governance wait 封存 current scheduling contract、partition、focus、gate witness、dispatch fence 和 ready projection
  identity。resolution 只恢复同一 governed lineage；Decision/Approval、Application 或 UI 不得借 resolution 重排 CWI。
- governance resolution 后若 owner fact 已 stale，SM-2 使用同一 graph 做 prerequisite/reconciliation 和 targeted
  re-projection；它不能从 Workspace latest definition 或 LLM 重建 graph。
- Resume 只加载当前 Run 已封存的 Definition revision、partition set、scheduling contract、focus/witness、credit 和 recovery
  ledger。Resume 不重新运行 Workorchd compiler、不改 priority、不切换 Definition revision，也不重放 durable effect。
- Restart 使用 Workspace owner **当前** semantic Definition 创建 fresh bootstrap、新 partition set 和新 scheduling contract；
  successor 不继承 predecessor ready queue、binding、credit 或 context expansion。它仍必须等待 predecessor 的
  `Result -> Workspace -> Run` delivery 完成后由 Run owner 创建。
- startup recovery 只能从 Run owner opaque aggregate 恢复现有 scheduling/partition facts。缺失 current-baseline contract
  返回 `fresh_activation_required`，不得以 projection、event history 或 default order 补建。

### 8.2 Static and dynamic prerequisite ownership

- Workorchd 只把 static Definition/Profile 输入、graph、mapping、partition 和声明式 capability policy refs 编译进 typed
  bootstrap，由 Run owner 随 activation 封存；不在 Run 前探测 live capability、target revision、provider ceiling 或
  governance readiness。
- Run 创建后，SM-2/SM-4 只通过 EventHub concrete typed acquisition/result 取得 dynamic owner facts。SM-2 是将这些 facts
  纳入 authority、binding 和 ready/fail-closed decision 的唯一 owner。
- capability owner 只返回自己的 readiness/feasibility/receipt；它不能修改 schedule、partition、focus 或 witness。
- static Definition failure 可阻止 Run creation；dynamic readiness failure 可以形成 prerequisite wait 或 typed terminal，
  但两者都必须发生在对应 provider request/dispatch/effect 前。

## 9. Component Landing Matrix

| Component | 必须收口 | 明确禁止 |
| --- | --- | --- |
| `blocks/workspace` | item lifecycle/disposition、definition-confirmed relations、revision/hash、owner-local typed mutation 和 retired lineage | Runtime focus/schedule、Profile-derived relation ownership、LLM draft authority、跨 owner write |
| `application/workorchd/biz/appservice/setupdraft` | **可选** bounded relation/priority/disposition draft，只有用户采用后才提交 owner command | 直接写 Workspace、强制普通 setup 调 LLM、创建 Run、封存 Runtime graph |
| `application/workorchd/.../activationcore` | 将 Definition/Profile 静态编译为完整 mapping/graph/gates/partitions；校验 completeness/capacity | 按数组/ID 定优先级、读取 live capability repository、选择 Runtime focus |
| `workorchruntimecontract` | semantic item/mapping/relation/schedule/gate-witness/partition/context DTO；纯 normalize/validate/hash/evaluator | EventHub、repository、CAS、retry/terminal/state transition |
| `kernel/orchestrator` SM-2 | dynamic prerequisite admission、scheduling seal、rank、ready/witness projection、partition selection、targeted reopen、唯一 focus install | 领域推断、第二 queue owner、ID/Order business sort |
| `kernel/orchestrator` SM-3 | bounded proof/C5-context/operations transaction；删除 coverage-order transaction | 全局排序、依赖写入、C1 mutation、第三 attempt |
| `kernel/orchestrator` SM-4 | persisted Resume/re-drive、guard、Restart delivery 和 terminal delivery不变 | graph/partition rebuild、projection authority |
| `blocks/llm` | immutable C1 partition + bounded C5 renderer、closed schema、audit；optional setup/proof factual result | 全量执行排序、dependency/priority authority、focus-driven C1、raw diagnostic action |
| Template/Profile/System defaults | 中立 disposition、关系、business priority、requires/provides、gate和成本等级声明 | 修改 Runtime state、把软件语义下沉 kernel |
| capability blocks | 当前 readiness/feasibility/receipt owner facts | 选择 CWI、修改 graph/rank/partition、请求 target state |
| `blocks/run` | opaque aggregate/current-baseline persistence | 解码 scheduling contract、选择 focus |
| projection/Web/TUI | definition关系确认、blocked reason、rank解释、context audit和progress展示 | 按显示顺序反向驱动 Runtime |
| Result/Workspace/Run delivery | 继续消费同一 sealed terminal | 重新排序 gap、重判 coverage |

不新增 framework component；各变化落入现有 owner 或 Orchestrator 内部 focused package。

## 10. Required Deletions And Replacements

新主线切换时必须在同一 current-baseline closure 中删除或失去生产可达性：

1. `RuntimeSemanticPlanStepCoverageOrder`；
2. semantic wire/schema/prompt 的 `coverageOrder`；
3. `CoverageOrderDocument`、`CoverageOrderHash` 和相应 pending lineage；
4. complete eligible-closure permutation evaluator/correction；
5. `ProjectRuntimeCWIEligibility(..., acceptedOrder)` 的 caller-authored order 输入；
6. `RuntimeCWIsEligibleForPlanning` 使用 Definition `Order` 或 ID 的 execution fallback；
7. `RuntimeCoverageItem.Order` 的 authority用途；若 UI 仍需展示顺序，改为 projection-only `PresentationOrdinal`；
8. 按 DoD 数组位置生成 `dod-%03d/cwi-%03d` identity 或把其当作关系依据的逻辑；只有 Workspace owner-issued、
   跨 presentation reorder 稳定且不承载顺序的 opaque ID 才可保留；
9. proof-batch 对全量 coverage-order document/hash 的依赖；
10. 每次 semantic/operations request 无条件复制完整 Goal/DoD；
11. 把总 CWI 数量与 active `MaxFocuses`/provider context 上限绑定的 gate；
12. Profile selector 的无界 source × dependent 笛卡尔积展开；
13. `HighPriorityDownstreamUnlockCount`、`TotalDownstreamUnlockCount` 或等价 transitive-closure rank；
14. current focus/context expansion 直接改写 C1 slice 的 renderer 与 hash 路径；
15. 只有 `any_settled=true`、没有 current binding/evidence witness 的 gate admission；
16. 普通 setup 强制调用 LLM 或要求用户确认机器草案的流程；
17. 用 hash equality 代替 canonical semantic signature collision check 的 comparator。

迁移期间不允许新旧 scheduler 同时写 focus。可以先增加纯 DTO/helper 和 comparison test，但生产切换提交必须同时：

- 启用 scheduling contract；
- 删除 provider `coverageOrder` authority；
- 将所有 focus 选择路由到同一 SM-2 projection；
- 删除 Definition-order fallback；
- 启用 immutable C1 partition/C5 expansion renderer，并拒绝 focus-driven C1 mutation；
- 更新 aggregate validator、hash、fixtures 和 fresh-data baseline。

当前 development baseline 不维护旧 aggregate migration。切换后旧数据返回 `fresh_activation_required`，部署验证使用清理后的
新数据。

## 11. Atomic Implementation Queue

所有状态初始为 `open`。只能按依赖顺序逐项关闭；完成一个工作项必须同步其测试和文档，不得以局部编译通过标记完成。

### WGSC-00 — Core contract alignment

Depends on: none

Affected machines: SM-2、SM-3、SM-4；SM-1 明确不变。

修改：

- `docs/00-core/runtime-stage-contract.md`；
- `docs/00-core/runtime-request-cache-note.md`；
- `internal/modules/kernel/orchestrator/readme.md`；
- activation、planning、context、verification flow docs。

Exit：更高权威明确 dependency DAG/scheduling contract、ID/order 禁令、immutable C1 Definition partition、bounded C5
expansion、gate causal witness、static/dynamic prerequisite 边界和无全量 provider `coverageOrder`；implementation SoT
同时把尚未落地内容标为 target 而非 current fact，且没有第五状态机或第二 Owner。

### WGSC-01 — Workspace semantic-definition owner contract

Depends on: WGSC-00

Affected machines: Runtime SM 无变化；Workspace owner-local definition reducer。

修改：

- Workspace models、repository、owner commands/results/notifications；
- management/CLI/Web typed DTO；
- definition revision/hash、item retire/supersede、disposition/mapping 与 clone/round-trip。

Exit：item/disposition/lifecycle 和 definition-confirmed relation 由 Workspace owner 唯一持久化；Profile/compiled relation 保留
自己的 source ref/hash，不伪装为 Workspace mutation；用户关系或业务优先级变化必然改变 semantic definition revision；
required/constraint/context/aggregation mapping cardinality 完整；仅 presentation 重排不改变 semantic graph hash；
duplicate/stale command 幂等；无 raw Runtime JSON。

### WGSC-02 — Default setup and optional assistance boundary

Depends on: WGSC-01

Affected machines: none。

修改：Template/Profile deterministic defaults、setupdraft generator/schema/session、HTTP/Web/TUI form 和 confirmation use case。

Exit：Template + Goal/DoD + optional directory 的普通流程零 LLM 即可完成；大 Definition 可选地分批提出组内/跨组关系；
未采用草案不写 owner；采用动作通过具体 Workspace owner command；LLM 输出顺序/ID 不产生 priority；歧义只返回可读
clarification，manual flow 完整可用。

### WGSC-03 — Runtime neutral graph contracts

Depends on: WGSC-00、WGSC-01

Affected machines: 为 SM-2/3/4 提供纯合同，不拥有 transition。

修改：`internal/pkg/workorchruntimecontract` 的 semantic item/mapping、edge、gate/witness、schedule、ready projection、
Definition partition/C5 context、预算和 hash。

Exit：normalize/validate/clone/hash/permutation/collision tests 通过；graph traversal 为 `O(N+E+G)`，canonical serialization
最坏 `O((N+E+G) log(N+E+G))`；无 transitive closure rank；包内无 EventHub/I/O/CAS/owner import；rank tuple
静态证明不读取 `Order`/ID 值。

### WGSC-04 — Workorchd activation compiler

Depends on: WGSC-01、WGSC-03

Affected machines: 只构造 SM-2 输入，不迁移 Runtime state。

修改：activationcore work-unit identity、disposition/mapping、Profile relation compiler、requires/provides resolver、gate/partition
compiler、capacity validation。

Exit：相同语义 Definition 在输入顺序和 opaque ID 重命名后产生等价 graph/rank；missing prerequisite、cycle、ambiguity、duplicate
和 static capacity 超限均在 Run create 前失败；每个 CWI 有 home partition；无 capability/LLM I/O。dynamic owner readiness
不在本工作项伪装为 static fact。

### WGSC-05 — SM-2 scheduling seal and ready projection

Depends on: WGSC-03、WGSC-04

Affected machines: SM-2；SM-3/SM-4 只消费 persisted fact。

修改：typed dynamic prerequisite acquisition、Orchestrator private internal event、authority component、CWI ledger/gate-witness
pure evaluator、partition selection、aggregate validator、progress signature。

Exit：one private event + one CAS 封存 scheduling contract；所有 focus 选择复用 one ready projector；duplicate 零 CAS；stale/CAS
conflict 重算；gate witness 与 focus 同 CAS；bound/effect-fenced focus 不被替换；无 ID/Order/presentation sort；dynamic
readiness 在 provider/dispatch 前 fail-closed。

### WGSC-06 — Remove global coverage-order transaction

Depends on: WGSC-05

Affected machines: SM-3，及 SM-2/SM-3 lineage fence。

修改：semantic DTO/schema/prompt/evaluator/ledger/batch/correction/request session 和全部 fixtures。

Exit：semantic provider 只处理当前 bounded proof batch；不存在 `coverage_order` request/document/hash/correction；batch identity 绑定
scheduling-contract + ready-projection + gate-witness + Definition-partition hash；attempt 0/1、whole-document recovery 和
stable-prefix 规则保持。

### WGSC-07 — Stable Definition partition and bounded external context

Depends on: WGSC-03、WGSC-05、WGSC-06

Affected machines: SM-2/SM-3/SM-4 request recovery；不改变 SM-1。

修改：Definition partition envelope/compiler、C0～C5 renderer、LLM preflight/schema、stable-prefix generation audit、
C5 context-expansion ledger。

Exit：每次 provider request 的 C1 partition 和 C5 exact context 分别受独立预算约束；C1 partition text 无损且不随同 partition
focus 变化；attempt 0/1/whole-document recovery 的 partition/stable prefix 不漂移；跨 partition focus switch 是显式新
generation；合法 expansion 只改变 C5 并新建 logical request；未选定义不丢 authority；无跨 partition 全量正文默认复制。

### WGSC-08 — Bounded focus window and large-ledger convergence

Depends on: WGSC-05、WGSC-06、WGSC-07

Affected machines: SM-2/SM-4。

修改：分离完整 CWI ledger容量与 active focus window；settle/reopen 后按 graph 增量投影；gate membership/witness 与 direct
unlock counter 高效计算。

Exit：超过现有 64 focus 的合法 Definition 可通过有界 window 持续推进；任何 provider batch 仍不超过 sealed limit；
Resume/startup 只从同一 aggregate 恢复，Restart 使用 fresh bootstrap；无进程内 scheduler、transitive reachability
materialization 或 N×M edge materialization。

### WGSC-09 — Projection and explainability

Depends on: WGSC-01、WGSC-05、WGSC-07、WGSC-08

Affected machines: none；read-only projection。

修改：projection contract/store、HTTP/SSE、Web/TUI progress。

Exit：用户可查看 semantic prerequisite、gate witness、blocked reason、effective priority、immediate unlock count、critical
depth、active window、partition/prefix generation 和 context included/omitted refs；显示顺序不能回写 authority；
projection rebuild 不触发 drive。

### WGSC-10 — Combined verification and live handoff

Depends on: WGSC-00～WGSC-09

Affected machines: SM-1～SM-4 combined regression，其中 SM-1 语义不变。

修改：focused tests、boundary verifier、file-backed/current-baseline fixtures、governance/Resume/Restart regression、
deployed live matrix 和所有 current docs。

Exit：第 12 节全部自动化通过；fresh-data real-provider run 连续推进多个 dependency wave 并完成
`Result -> Workspace -> Run`；本文 code/live 状态与 active overview/current matrix一致。

## 12. Verification Matrix

### 12.1 Definition and graph

| Scenario | Required assertion |
| --- | --- |
| reorder Goal/DoD arrays only | semantic graph/rank 不变；presentation可独立变化 |
| rename all opaque IDs | 映射后的相对语义顺序不变；无 lexical/numeric ID influence |
| smaller numeric ID is dependent | prerequisite仍先执行 |
| clarify same semantic item | 保留 ItemRef、创建新 Definition revision；既有 Run 不漂移 |
| materially replace/delete item | old ItemRef retired且不复用；fresh activation使用新 mapping |
| required item has no CWI / CWI has two primary items | static mapping validation fail-closed before Run creation |
| constraint has no consumer / aggregation has no gate | static disposition validation fail-closed |
| missing tiny prerequisite | Run 创建前 `missing_prerequisite`；零 provider/dispatch/effect |
| dangling/self dependency | fail-closed before Run creation |
| hard/reopen cycle | exact typed cycle path；零 Runtime work |
| ambiguous providers | 无 explicit any-of gate时拒绝 |
| semantic duplicates | commutative、合并或 typed ambiguity；绝不按 ID兜底 |
| fan-in/fan-out | 使用 gate，graph size接近 `O(N+E+G)` |
| any-settled gate | focus封存全部 current witness entries和 exact evidence lineage |
| input permutation | Definition/scheduling hash稳定 |

### 12.2 Semantic priority

| Scenario | Required assertion |
| --- | --- |
| high-priority goal has low-level prerequisite | prerequisite继承 effective high priority |
| one item directly unlocks many high-priority dependents | 优先于无关小项 |
| deep fan-out with large transitive closure | 不计算 descendant cardinality/reachability materialization |
| lower-ranked CWI shares current partition | cache affinity不能越过更高rank的cross-partition CWI |
| equal unlock but different critical depth | longer critical path先行 |
| equal semantic value but different cost band | smaller declared prerequisite先行 |
| presentation priority changes | execution rank不变 |
| Definition `Order`/ID changes | execution rank不变 |
| semantic hash collision, different signatures | 比较 canonical bytes，不误判 duplicate、不读 ID |
| exact semantic tie | 明确 ambiguity/commutative rule，不读 ID |
| prerequisite reopened | dependent退出 ready；只重投影影响闭包 |
| current focus bound/effect-fenced | 任何新 rank都不能替换它 |

### 12.3 Context and LLM

| Scenario | Required assertion |
| --- | --- |
| normal setup | Template + Goal/DoD + blank directory零 LLM完成，目录解析为默认路径 |
| large Definition | provider只收到 bounded exact C1 partition；aggregate保留完整 authority |
| focus changes inside one partition | C1/StablePrefixHash不变，只改变C3 |
| focus changes across partitions | 先持久化partition-switch generation，再创建新logical request/prefix |
| proof batch contains multiple CWI | 全部home partition相同；cross-partition CWI必须拆为另一logical request |
| omitted unrelated partition | 不进入 prompt noise，但在 partition/context audit 可见 |
| required external context | 只可请求 manifest-authorized refs；C5 有界扩展且 C1 不变 |
| attempt 0/1 | DefinitionPartitionHash、C0～C3、C5 expansion和StablePrefixHash精确相同 |
| LLM returns global order | strict schema rejection；零 focus mutation |
| LLM orders by IDs | 输出无该字段/权限，不能影响 scheduler |
| malformed/timeout context request | whole-document recovery或typed terminal；不扩 dependency/priority |

### 12.4 State machine and recovery

| Scenario | Required assertion |
| --- | --- |
| scheduling contract missing | SM-3 provider request和dispatch均为零 |
| duplicate schedule/private event | zero revision/CAS |
| CAS conflict during seal | reload + pure recompute；无 process-local order复用 |
| crash after schedule before proof | exact contract/window恢复；一次 logical request |
| crash after dispatch/receipt | 不重放 durable effect；一次 evidence credit |
| settled CWI unlocks next wave | SM-2重投影并继续正向推进 |
| reopen after resource effect | exact successor closure重开；无全局重排/全量 prompt |
| any-gate witness member reopened | 至少一个sealed witness仍current才可继续，否则dependent退出ready |
| gate witness stale after effect fence | 不换focus/witness、不重放effect；原receipt收敛后拒绝新credit并targeted reopen |
| governance wait/resolution | graph/partition/focus/witness保持；resolution不触发重排 |
| Resume | 复用同一aggregate/schedule/partition/focus；不重编译、不重放effect |
| Restart | predecessor delivery后fresh compile；不继承schedule/binding/credit/context expansion |
| dynamic capability unavailable | Run后由SM-2 typed fail-closed，且provider/dispatch/effect均为零 |
| all CWI settled | artifact/coverage closeout后 sealed terminal |
| partial delivery | 仅重试缺失的 Result/Workspace/Run acknowledgement |
| old aggregate | `fresh_activation_required`；零 provider/capability work |

### 12.5 Scale and performance

至少提供 1,000 CWI、线性/分支/聚合图的 deterministic fixture，并证明：

- graph traversal、cycle validation、priority propagation、critical depth、direct unlock 和 full ready projection 为
  `O(N+E+G)`；canonical normalization/hash 的最坏上界单列为 `O((N+E+G) log(N+E+G))`；
- 不构造 transitive reachability matrix/bitmap/count；单项 settle/reopen 的增量路径为
  `O(A+E_A+G_A)`，最坏不超过一次 full projection；
- 无 source × dependent 的无界 `N×M` materialization；
- active focus、semantic proof batch、C1 partition和C5 external context始终不超过各自 sealed budget；
- duplicate projection近似零分配/零 CAS，单项 settle/reopen只重算必要闭包；
- stable graph/scheduling/request hash 在多次运行和输入 permutation后相同；
- provider请求大小不随全部已完成/无关 Definition正文线性增长；同 partition 多个 focus 复用同一 stable prefix。

## 13. Required Command Gates

每个代码切片先执行 focused tests，再执行：

```bash
go test ./internal/pkg/workorchruntimecontract -count=1
go test ./internal/modules/blocks/workspace/... -count=1
go test ./internal/modules/application/workorchd/... -count=1
go test ./internal/modules/blocks/llm/... -count=1
go test ./internal/modules/kernel/orchestrator/... -count=1
make verify-boundary-contract
make test
make build
```

涉及 Web 后额外执行 `make console-build`。代码总收口完成后运行 `make release-check` 与适用的 file-backed verifier；
real-provider/live 只在重新部署并清理 development data 后进行。

## 14. Exit Criteria

本文只有同时满足以下条件才能关闭并迁入 `docs/90-archive/completed/`：

1. Goal/DoD item、lifecycle/disposition 与 definition-confirmed relation 由 Workspace owner唯一持久化；Profile/compiled
   relation 保留来源且 Definition→CWI mapping完整；
2. Workorchd只静态编译完整 neutral graph/gate/partition，不读取 live capability或持有 Runtime scheduling state；
3. SM-2 是 dynamic prerequisite admission、scheduling contract、gate witness、ready projection、partition selection、focus与reopen的唯一 owner；
4. execution priority没有任何 Definition position、`Order`、ID字典/数字或presentation priority输入；
5. provider `coverageOrder`生产面和全部 fallback已删除；
6. 完整 CWI ledger与bounded active window、C1 partition、C5 external context已分离；
7. 同一 partition 内 C1/StablePrefixHash不随focus变化；cross-partition switch显式建新generation；expansion只改变C5；
8. static missing prerequisite/cycle/ambiguity/capacity 在Run创建前失败；dynamic owner readiness在provider/dispatch/effect前处理；
9. 普通 Template + Goal/DoD + optional directory setup零LLM可用，assisted draft只有用户采用后才进入Workspace owner；
10. 1,000-CWI自动化矩阵、duplicate/stale/governance/Resume/Restart/CAS/budget/guard全部通过；
11. fresh-data deployed run证明至少两个dependency wave正向推进并最终完成artifact closeout和
    `Result -> Workspace -> Run` delivery；
12. core contract、cache note、Orchestrator/Workspace/Workorchd/LLM README、current-state、flow、verification和UI文档一致；
13. active目录不存在同范围第二份代码收口计划。

局部 artifact、一次合法 dispatch、正确 fail-closed、revision增长、mock-only成功或单个CWI settled均不足以关闭本文。
