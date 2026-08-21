# Semantic Scheduling Contract

Status: normative

Type: core-contract-part

Last Verified: 2026-08-18

## Non-Negotiable Invariants

### 单 Owner 与四状态机

- Workspace owner 独占 pre-seal `DefinitionAdmissionCandidate` lifecycle，以及正式 Goal/DoD、item identity、用户确认关系、
  definition revision 和 candidate→startup-input promotion。
- Run owner 独占 SM-1，并仅 CAS 持久化 Orchestrator opaque Runtime aggregate。
- Orchestrator 独占 SM-2、SM-3、SM-4；只有 SM-2 private internal event 可封存 scheduling contract、选择 focus、
  retain/reopen CWI 或授予 credit。
- Workorchd 只读取 owner snapshot、编译 typed bootstrap 和提供 UI/API 用例，不拥有 Runtime scheduling state。
- LLM 只提出 optional setup draft、per-CWI proof/candidate proposal 或 bounded semantic-gap diagnostic；只有用户明确采用的
  setup draft 才能通过 Workspace owner command 形成新 candidate。LLM 不写 canonical startup input、dependency、priority、
  focus、binding、credit、ticket 或 terminal。
- capability owners 只发布 readiness/feasibility/receipt facts，不要求接收方进入某个状态。
- projection/Eventstream/TUI/Web 只显示已持久化事实。
- 不增加第五状态机、新 framework unit、跨 owner repository/service 或通用 EventHub envelope。

### 禁止按序号和 ID 推断业务顺序

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

### 依赖先于排序

排序只能处理已知关系。任何实现都必须遵守：

```text
definition itemization
  -> dependency compilation
  -> completeness / cycle / ambiguity validation
  -> scheduling-contract seal
  -> bounded readiness preflight
  -> first durable dispatch
```

已声明但未满足的 prerequisite、未解析 provider、循环、歧义或超限必须在首次副作用前 typed fail-closed。这里的 static
completeness 只覆盖 Definition/Profile 已声明的 ref、relation 和 mapping；Workorchd 不从任意 Goal/DoD 自由文本推断一个
从未声明的业务前置项。执行期 LLM 后续发现的隐含 semantic gap 走
[run-scoped clarification lane](runtime-clarification.md)，不能通过执行
失败后 replan、扩图或改写 startup input 来补齐。

### Partial Order，而非全量固定队列

- authority 是 dependency DAG + scheduling policy，不是 LLM 生成的完整线性序列。
- 只有当前 ready 节点可参与下一 focus 选择。
- bound、active dispatch、accepted effect 或 evidence lineage 的 current focus 必须保持 fenced。
- reopen 只影响其语义后继闭包；不重排无关 settled branch，不重新发送全部 Goal/DoD。
- Definition 关系变化必须使用新 definition revision 和 fresh activation/restart；active Run 不原地修改 graph。

## Semantic Scheduling Policy

### Ready gate

一个 CWI 只有同时满足以下条件才进入 ready set：

- convergence state 为 `unbound | planning | reopened`；
- 所有 direct prerequisite 为 current `settled + satisfied + CurrentBindingRef`；
- 所有 all-of gate 的全部 member 已 current `settled + satisfied`，且 witness 完整；
- Definition/scheduling contract、reconciliation 和 current catalog lineage 未 stale；
- 它不是另一个 bound/effect-fenced current focus 的非法替换。

任何 priority 都不能越过 ready gate。

### Semantic rank tuple

SM-2 使用 Kahn topological ready queue，并为每个 ready CWI 计算可解释的词典序 rank：

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
- `EffectiveBusinessPriority`：自身与全部 required hard-dependency 后继中最高的声明式业务优先级。all-of 合同下不存在
  可被忽略的 optional branch；高优先级目标的低层 prerequisite 会继承该优先级。它通过一次 reverse-topological max
  propagation 计算，不物化 transitive closure。
- `ImmediateHighPriorityReadyUnlockCount`：当前项 settle 后，unsatisfied prerequisite/gate count 将归零且 effective priority
  为 critical/high 的**直接** dependent 数量。
- `CriticalPathDepth`：从本项到唯一 `TerminalAllOfGate` 的最长 hard-dependency path，通过一次 reverse-topological
  dynamic programming 计算。
- `ImmediateReadyUnlockCount`：当前项 settle 后会直接进入 ready 的 CWI/gate 数量；它不统计全部后继闭包。
- `CostBand`：只在解锁价值相同后使用，优先完成小型前置项；不得实现为全局 shortest-job-first。
- `SemanticTieBreakHash`：只加速 ID-free semantic signature 比较，不表达业务价值。
- `CanonicalSemanticSignature`：只在 hash collision 时比较规范化 bytes；它不含 identity、位置或 presentation 顺序。

禁止在 rank 中重新引入 transitive descendant count、reachability bitmap size 或全闭包 cardinality。priority propagation、
critical depth 和 direct unlock counter 都必须在 CWI + gate 扩展 DAG 上以 `O(N+E+G)` traversal 完成，其中 `G` 是 gate
membership 数；canonical serialization/sort 的复杂度单独按
[scale verification](verification.md#scale-and-performance) 核算。

复杂度边界进一步固定为：`EffectiveBusinessPriority`、`CriticalPathDepth`、semantic signature/hash 是 scheduling seal
时计算的 static rank input；两个 immediate unlock count 只读取 current facts + sealed contract 纯派生的 per-dependent/gate
unsatisfied-member counter，该 counter 不是独立 persisted authority。一次 full fact evaluation 为 `O(N+E+G)`；设 ready
set 大小为 `R`、bounded window 为 `W`，建 heap 为 `O(R)`，选择并排序 window 为 `O(W log R + W log W)`。若为 projection
hash/审计规范化完整 ready set，另计 `O(R log R)` canonical serialization。ready-queue comparator 不得遍历 graph：通常为
`O(1)`，只有 semantic hash collision 时才对已封存 signature bytes 做 `O(L)` 比较。单项 settle/reopen 只更新 outgoing
edges、受影响 gate 和对应 heap entries。因此“语义排序”不能通过在每次 comparison 中重新搜索后继来偷偷恢复
transitive-count 成本，也不能把排序/序列化成本冒充为线性 graph traversal。

禁止把 `RuntimeAcceptanceSlice.Priority` 直接复用为 execution priority。它继续只服务 presentation/request grouping；
若 Profile 需要业务优先级，必须显式编译为 `BusinessPriority`。

### Dynamic ready projection

开始执行前，SM-2 封存：

```text
RuntimeCWISchedulingContract
  StartupInputHash
  DefinitionRevision
  DefinitionSeedHash
  DefinitionCoverageMappingHash
  DefinitionPartitionSetHash
  SemanticGraphHash
  SchedulingPolicyHash
  AllOfDependencyGates[]
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
  AllOfGateWitnesses[]
  EffectiveRanks[]
  BlockedReasons[]
  FencedCurrentCoverageRef
  ProjectionHash
```

`BoundedActiveWindow` 不是第二份 persisted scheduler。只有既有 Orchestrator private SM-2 event 可把它安装到
`Focus/RemainingFocuses`，并同时封存该选择使用的 gate witness、effective rank 与 Definition partition ref；
duplicate projection 为零 revision，CAS conflict 必须 reload 后重新计算。

### Stability barrier

会改变同一语义资源闭包的 CWI 必须通过 Profile/Definition hard edges 位于其 verification、aggregation 和 finalization
之前。这样可避免每完成一个小修改就过早验证，随后又因同一上游变化重开。

Orchestrator 不理解“源码”“测试”或“提交”；它只执行已封存的 hard edge、gate 和 evidence-triggered reopen relation。
