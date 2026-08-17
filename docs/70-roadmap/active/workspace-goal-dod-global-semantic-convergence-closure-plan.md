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
- Goal/DoD 只支持 `all_of` 合取终态，以及不满足该合同时的 static pre-Run 或
  runtime-discovered Definition clarification 阻塞票据；
- Goal、DoD、Workspace Template snapshot 与解析后的项目工作目录作为不可变 startup input，执行期 LLM 只能基于其
  revision/hash 产生派生 proposal 或 clarification，不得改写输入；
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
| Definition aggregation | 尚未形成唯一的合取 admission 与 clarification ticket 合同 | Goal/DoD、Template/Profile 和 Runtime dependency/terminal schema 只接受 `all_of`；显式 one-of 在 Run 前建票，执行期 LLM 发现的隐含 one-of/semantic gap 进入绑定 current Run authority 的阻塞票据 |
| Startup inputs and LLM | setup authoring、activation input 与执行期 semantic processing 边界未完全分开 | Workspace owner 先封存 Goal、DoD、Template snapshot、resolved project directory；之后允许 Runtime 调用 LLM，但所有 request/result 必须绑定同一 startup-input hash 且不能修改四项输入 |

该表是迁移边界，不是双主线。任一 topic 完成生产切换时，必须在同一提交中删除对应 current-baseline surface，并更新
core note、implementation SoT、flow、verification 和 current-state；禁止长期保留 “current/target” 双写。

[Runtime Positive-Convergence Live Closure](runtime-single-owner-model-capability-output-closure-plan.md) 继续只承担部署态
正向链验证，不是本代码收口的第二份设计。本文 code-closed 后，相关 fresh-data/live 场景并入该唯一部署验证入口。

## 2. Product Outcome

目标 Workspace 流程固定为：

```text
1. user confirms Template + Goal/DoD; directory is optional and always resolves
   -> Workspace owner seals one startup-input contract: exact Goal/DoD, Template snapshot,
      resolved project workspace directory and semantic Definition revision

2. fresh activation preparation
   -> Workorchd compiles declared neutral CWI dependencies and immutable Definition partitions
   -> explicit structured one-of, unresolved declared prerequisite or static ambiguity opens one deterministic
      workspace-definition clarification ticket; no Run is created
   -> Run owner seals the typed bootstrap and performs its SM-1 owner-local activation
   -> SM-2 acquires dynamic owner prerequisites, validates and seals one scheduling contract
   -> SM-2 projects one bounded ready window
   -> SM-3 may call LLM with the sealed startup-input identity and requests bounded per-CWI proof only
   -> if LLM processing exposes an implicit one-of or undeclared semantic gap, SM-2 seals a
      run-scoped definition-clarification governed wait before the affected operation effect
   -> capability owners return feasibility / operation receipts
   -> SM-2 seals binding, credits evidence and advances/reopens CWI
   -> repeat from the persisted dependency graph until all required CWI settle

3. terminal
   -> SM-2 seals artifact/coverage closeout
   -> SM-4 delivers Result -> Workspace -> Run
   -> application projects the completion report

4. definition clarification during execution
   -> LLM diagnostic never rewrites startup input or chooses a branch
   -> user changes Goal/DoD/relation only through Workspace owner and creates a new Definition revision
   -> the current Run remains bound to the old startup-input contract and cannot Resume onto the new revision
   -> only an explicit Restart may create a fresh Run through the existing replacement/delivery lane
```

排序不是一次性的全量线性计划。开始执行前封存的是：

- 覆盖所有已声明 relation/ref/mapping 的完整 CWI 语义依赖 DAG；
- 仅表达 all-members-settled 的无状态 all-of dependency gates；
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
9. 排序只能处理已知依赖；static compiler 只能拒绝已声明但未满足的 prerequisite，执行期 LLM 发现的未声明语义缺口尚未
   形成不改写 startup input、阻塞 affected effect 并要求显式 Restart 的唯一票据合同。
10. Definition item 的增删改退役、Goal/DoD disposition 和 item→CWI cardinality 尚未形成唯一合同。
11. Definition/Profile/Runtime 仍允许或暗示 `one_of/any_of/any_settled`；这既没有唯一 terminal 合同，也没有
    “显式声明在 Run 前建 workspace-scope ticket，执行期隐含语义缺口在 current Run 内建 governed ticket”的
    owner/idempotency/recovery 边界。
12. 当前排序指标包含 transitive downstream count，却同时承诺 `O(N+E)`，性能合同不可同时成立。

## 4. Non-Negotiable Invariants

### 4.1 单 Owner 与四状态机

- Workspace owner 独占正式 Goal/DoD、item identity、用户确认关系和 definition revision。
- Run owner 独占 SM-1，并仅 CAS 持久化 Orchestrator opaque Runtime aggregate。
- Orchestrator 独占 SM-2、SM-3、SM-4；只有 SM-2 private internal event 可封存 scheduling contract、选择 focus、
  retain/reopen CWI 或授予 credit。
- Workorchd 只读取 owner snapshot、编译 typed bootstrap 和提供 UI/API 用例，不拥有 Runtime scheduling state。
- LLM 只提出 optional setup draft、per-CWI proof/candidate proposal 或 bounded semantic-gap diagnostic，不写
  startup input、dependency、priority、focus、binding、credit、ticket 或 terminal。
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

已声明但未满足的 prerequisite、未解析 provider、循环、歧义或超限必须在首次副作用前 typed fail-closed。这里的 static
completeness 只覆盖 Definition/Profile 已声明的 ref、relation 和 mapping；Workorchd 不从任意 Goal/DoD 自由文本推断一个
从未声明的业务前置项。执行期 LLM 后续发现的隐含 semantic gap 走第 5.6 节的 run-scoped clarification lane，不能通过执行
失败后 replan、扩图或改写 startup input 来补齐。

### 4.4 Partial Order，而非全量固定队列

- authority 是 dependency DAG + scheduling policy，不是 LLM 生成的完整线性序列。
- 只有当前 ready 节点可参与下一 focus 选择。
- bound、active dispatch、accepted effect 或 evidence lineage 的 current focus 必须保持 fenced。
- reopen 只影响其语义后继闭包；不重排无关 settled branch，不重新发送全部 Goal/DoD。
- Definition 关系变化必须使用新 definition revision 和 fresh activation/restart；active Run 不原地修改 graph。

### 4.5 仅允许 all-of terminal 语义

- canonical Definition aggregation、Template/Profile terminal declaration、assisted relation draft、RuntimeBootstrap dependency、
  scheduling contract 和 provider-facing Definition/relation schema 只允许 `all_of` 合取，不提供 `one_of`、`any_of`、
  `any_settled`、quorum、threshold 或 first-success terminal mode；
- 每个 required CWI 都必须进入唯一 derived terminal all-of root；只有全部 member 具有 current binding/evidence 且 settled，
  SM-2 才能形成 completed terminal；
- 多个 item 提供同一 required ref 不是“可任选的替代项”，而是 static ambiguity；Definition 必须改写为互不歧义的 required
  refs、明确唯一 provider，或把确实都必需的成员声明为 all-of aggregation；
- capability owner 在一个已固定 CWI 下提供多个 admissible operation kind/variant，只是 SM-3 candidate 可行性选择，不会减少
  required CWI 集合，也不构成 Definition one-of；
- 一个 concrete operation 的 tagged union、互斥 parameter 或 `RequireOneOfParameters` 可以作为 capability-owner input schema
  约束保留；它只要求 candidate 为同一个 sealed operation 提供唯一合法输入，不得被提升为 CWI/Goal/DoD 的可替代终态；
- 对 static input 中显式 one-of，或已声明 relation/ref/mapping 无法确定性编译为完整 all-of 的定义，
  系统不得自动选分支、静默改写、调用 LLM 猜测或先创建 Run；必须按第 5.6.1 节生成
  static pre-Run Definition clarification 阻塞票据。

上述最后一条只约束**可由 static input 确定**的显式 one-of/ambiguity。任意自然语言中未结构化的隐含 one-of 不允许
Workorchd 通过关键词或领域猜测裁决；它可以在 startup input 封存后的 Runtime semantic processing 中由 LLM 作为 bounded
diagnostic 暴露，再由 SM-2 按第 5.6 节 fail-closed 建票。

### 4.6 Startup input immutability and execution-time LLM

Workspace owner 在任何执行期 LLM request 前封存唯一 startup-input contract：

```text
WorkspaceStartupInputContract
  WorkspaceID
  DefinitionRevision
  GoalDefinitionRef + exact text hash
  DoDDefinitionRefs[] + exact item-set hash
  WorkspaceTemplateRef + TemplateRevision + TemplateSnapshotHash
  ResolvedProjectWorkspaceDirectory
  StartupInputHash
```

- 用户输入的 directory 可以为空，但 owner/application 必须先将其解析为实际项目工作目录；默认值仍为
  `.workorch-workspaces/<workspace-id>`。Run/LLM/capability contract 使用已解析值，不把空字符串当作 root；
- 每个 Run 的 bootstrap、Definition partition、scheduling contract、LLM request/session、candidate、dispatch、evidence、
  clarification ticket 和 terminal 都必须绑定同一 `StartupInputHash`；
- startup input 一旦被当前 Run 封存，执行期 LLM 可以读取其授权的 exact partition/context，提出 semantic proof、gap
  diagnostic 或 operation candidate，但不能修改 Goal/DoD、切换 Template snapshot、改变项目 root、生成新的 Definition
  revision、扩张 required CWI、选择 Runtime action、授予 credit 或裁决 terminal；
- LLM 输出中的改写文本、替代 directory/template、未声明 requirement 或 global order 只可作为 rejected diagnostic；不能
  回写 Workspace owner、RuntimeBootstrap、C0～C2 或 scheduling contract；
- 用户采纳任何 Goal/DoD/关系澄清都必须通过 Workspace owner command 创建新 semantic Definition revision。当前 Run 保持
  原 startup input；Resume 不能吸收新 revision，只有显式 Restart 可在 predecessor delivery 后取得新的 startup input；
- “手工 authoring 可不调用 setup-draft LLM”只是产品输入能力，不是 Runtime 零 LLM 不变量。startup input 封存后，SM-3
  可以按现有 bounded request/session、cache、budget 和 recovery 合同调用 LLM。

## 5. Canonical Definition And Dependency Model

### 5.1 Workspace-owned semantic items

目标 Workspace definition 使用 owner-owned typed item，而不是只有位置化字符串：

```text
WorkspaceDefinitionItem
  ItemRef                 stable owner-issued opaque identity
  Kind                    goal | dod
  Text                    exact user-confirmed text
  SemanticRole            profile/user-confirmed opaque role
  CoverageDisposition     required_coverage | constraint | context_only | all_of_aggregation
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
disposition/relation 默认值必须足以完成普通 setup。只有用户主动采用 assisted draft 时，UI 才增加草案
确认页；deterministic compile 返回 `definition_clarification_required` 时，UI 提供的是阻塞纠正入口，不能通过
确认原定义放行。两条路径都不得要求用户提交 Runtime JSON、target、recipe、check、metric、artifact 或
capability readiness。

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
  StartupInputHash
  DefinitionRevision
  ItemRef
  Disposition
  PrimaryCoverageRefs[]
  ConstraintConsumerCoverageRefs[]
  AllOfGateRefs[]
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
- `all_of_aggregation` 必须映射到非 executable all-of gate/terminal coverage relation，不能伪装为 operation CWI；
- 每个 CWI、gate 和 constraint consumer 都必须可反向追溯到同一 Definition revision 的 item。

Template/Profile 必须提供中立默认规则。默认产品流将可确定关联的 DoD 编译为 `required_coverage`，将其 Goal 编译为
`all_of_aggregation`。用户确认、Profile 声明或 assisted draft 都不能使 `one_of/any_of` 成为合法 canonical relation；
一旦出现此类声明，必须在 Run 创建前返回 `definition_one_of_unsupported` 并进入第 5.6 节的阻塞票据流程。没有 DoD 关联的
Goal 只能由 Profile 明确编译为 `required_coverage`，或在 Run 创建前返回 `definition_clarification_required`。
Orchestrator 不按 `Kind=goal|dod` 自行猜 disposition。

### 5.3 Canonical relation classes

| Definition relation | 编译后的 Runtime 语义 | 是否阻塞执行 |
| --- | --- | --- |
| explicit `depends_on` | `requires_settled` direct edge | 是 |
| matching `requires/provides` | `requires_settled` direct edge 或 gate | 是 |
| `verification_of` | subject stable 后才可验证的 hard edge | 是 |
| `invalidated_by` | evidence-kind constrained reopen edge | 否；命中后重开 |
| `context_ref` | bounded request inclusion relation | 否 |
| `all_of` aggregation | derived all-members-settled dependency/terminal gate | 是 |

每条 canonical relation 保存 source、dependent、kind、origin 和 Definition revision。`origin` 只允许：

- `definition_confirmed`：用户确认，包括 LLM-assisted draft 被用户确认后的结果；
- `profile_declared`：Template/Profile 的领域声明；
- `input_output_compiled`：Workorchd 对 opaque requires/provides 的确定性匹配。

`one_of`、`any_of`、`any_settled` 及其 quorum/threshold/first-success 等价形式不是 canonical relation class，不能通过增加
origin 或用户确认获得合法性。LLM 未确认草案、自由文本相似度、ID/序号、文件名、operation 名和错误消息不能成为 relation origin。
其中只有 `definition_confirmed` relation 是 Workspace owner 可修改并持久化的 Definition 数据；
`profile_declared` 来自已选 Profile snapshot，`input_output_compiled` 是 Workorchd 对已封存声明的纯派生 bootstrap
fact。Workorchd 不成为 relation owner；Run owner 随 bootstrap 冻结来源，SM-2 只在 scheduling contract 中
封存经校验的 compiled relation/hash。

### 5.4 All-of dependency gates

大型 fan-in/fan-out 使用无状态 gate，避免把 selector 直接展开为 `N×M` 边：

```text
RuntimeCWIAllOfDependencyGate
  GateRef
  MemberCoverageRefs[]
  MemberSetHash
  DependentCoverageRefs[]
```

Gate 只有一种含义：`MemberCoverageRefs` 中**全部**成员均以 current binding/evidence settled+satisfied。它是 SM-2 的纯派生谓词：

- 没有 lifecycle、repository、CAS 或 EventHub topic；
- 不产生 candidate、operation、receipt、evidence 或 credit；
- member set 在 activation 时封存，运行中不能动态扩张；
- cycle validation 必须把 gate 当作图节点处理；
- member/edge 数量与 hash 必须进入 scheduling-contract identity。

当 gate 第一次支持 focus install 或 terminal completeness seal 时，SM-2 还必须把 causal witness 封入
focus/authority lineage：

```text
RuntimeCWIAllOfDependencyGateWitness
  GateRef
  SchedulingContractHash
  FactsRevision
  Entries[]               exactly one entry per member: coverage ref + current binding ref + satisfaction evidence refs/hash
  WitnessSetHash
```

- witness 必须与封存的 member set 一一对应并覆盖全部 member；空 member、缺失/重复 entry 或 partial witness 均为合同错误；
- witness entry 必须逐项匹配 current binding 与 accepted evidence lineage；summary、LLM claim、ID 或旧 credit 不能成为
  witness；
- 任一 member reopened、binding/evidence stale 或 reconciliation 改变时，原 witness 立即 stale；dependent 退出 ready，
  未 effect-fenced focus 退休，SM-2 只有在全部 member 再次 current 后才能封存新 witness 并重投影；
- effect-fenced focus 不得静默换 witness 或换 CWI：SM-3 先按原 fence 收敛 receipt/outcome，SM-2 再把失效 witness 记录为
  causal stale，拒绝从该 lineage 新增 credit，并按同一 graph reopen/reproject dependent；
- witness 变化、dependent reopen 和 evidence credit 使用同一 private internal event/CAS；duplicate witness hash 为零 revision；
- gate 本身仍没有 lifecycle。witness 是 SM-2 authority fact，不是第五状态机、独立 repository 或跨 owner DTO。

Workorchd 还必须为每个 Definition 确定性生成唯一 `TerminalAllOfGate`。其 member set 是所有 required CWI coverage ref 的
canonical set；Definition 中的 aggregation hierarchy 只贡献 hard edge/gate 约束，不作为另一种 terminal 选择模式。
terminal root 及其 membership 进入 semantic graph/scheduling-contract hash。它不是 Workspace 中的可编辑 item，不产生
credit；它只让 terminal completeness 与 `CriticalPathDepth` 拥有唯一、ID-free 的根。

### 5.5 Two-phase completeness validation

Workorchd activation compiler 必须在创建 Run 前完成 static Definition validation：

1. startup-input contract 同时具有 exact Goal/DoD identity、Template snapshot、resolved project directory 和稳定
   `StartupInputHash`；所有 item/relation/ref 均属于同一 Workspace 和 Definition revision；
2. 无空 identity、悬空引用、自依赖或重复冲突关系；
3. hard dependency 与 reopen graph 分别无 cycle；
4. 每个 `RequiresRef` 恰好由初始事实或一个唯一 provider item 满足；确实要求多个输入时必须拆成不同 required ref 并由
   显式 all-of aggregation 汇合；
5. 多 provider、`one_of/any_of/any_settled`、quorum/threshold/first-success relation 或其他可替代分支语义一律拒绝；
6. 每个 disposition 与 Definition→CWI mapping 满足第 5.2 节 cardinality；
7. required all-of aggregation/finalization gate 及唯一 terminal all-of root 的 member set 完整且 hash 稳定；
8. semantic duplicate 已合并、声明为 commutative，或 fail-closed；
9. immutable Definition partition 完整覆盖 active item，每个 CWI 绑定一个 home partition，partition 无超限或正文摘要；
10. Definition item、edge、gate、gate membership、partition 和总字节数符合彼此独立的容量预算；
11. 不因 context budget 较小而裁剪 authority inventory；
12. 编译结果在输入 slice/map permutation 后保持同一 semantic graph hash。

Static completeness 只检查**已声明合同**。稳定错误码 `declared_prerequisite_unresolved` 表示一个显式 `RequiresRef`、
`DependsOnItemRef` 或 Profile relation 没有合法 source/provider；它不宣称发现 Goal/DoD 自由文本中从未声明的业务前置项。
Workorchd 不使用关键词、文本相似度或领域规则补写 relation。Template/Profile 可以在自己的领域层确定性声明关系；可选
setup draft 只有用户采用后才成为 Workspace definition-confirmed input。

Run 创建后、首次 semantic provider request 或 durable effect 前，SM-2 再通过 concrete typed owner command/result 完成
dynamic readiness validation：provider ceiling、capability/policy/catalog revision、validation surface、target selector 和
必要 governance contract 均必须 current。Workorchd 不读取 capability repository、不探测 owner，也不把“当前可用”写入
bootstrap。static ambiguity 返回 Definition clarification；dynamic unavailable/stale 使用现有 SM-2 prerequisite/
semantic-contract typed fail-closed 主线。执行期 LLM 暴露的隐含 semantic gap 是第三类输入：它由 SM-3 先持久化为 scoped
observation，再由 SM-2 进入第 5.6.2 节的 clarification governed wait；不得伪装成 static compiler fact 或 owner readiness。

### 5.6 Definition clarification blocking tickets

Decision owner 独占两类 Definition clarification ticket 的 lifecycle，但二者的发现阶段、scope 和恢复动作不同，不能用一个
optional `RunID` 模糊处理，也不能互相伪装。

#### 5.6.1 Static pre-Run clarification

显式 one-of、已声明多 provider 可替代语义与 unresolved declared prerequisite 属于 Workspace Definition admission failure，
不属于 SM-2 Runtime governance。Workorchd activation use case 必须在 pure static compiler 返回 violation 后按以下顺序收口：

1. pure compiler 返回 `definition_one_of_unsupported | declared_prerequisite_unresolved |
   definition_clarification_required`；只报告显式 relation/ref/mapping，不能声称已理解任意自由文本；
2. 在创建 Workspace 对应 Run、Runtime aggregate 或发起 provider/capability work 前，通过同步 EventHub `Send` 向
   Decision owner 的 `pkg/events` 提交 concrete typed `OpenWorkspaceDefinitionClarificationRequestCommand`，并取得 concrete
   result；调用 context 必须已绑定 matching `WorkspaceID`；
3. ticket scope 固定为 `WorkspaceID + DefinitionRevision + StartupInputHash`，不含伪造的
   `RunID/DispatchID/AuthorityTransaction`。Run-scoped runtime-governance ticket/command 不得复用；
4. ticket 至少持久化 violation code、offending item/relation refs、canonical violation signature/hash、typed
   `RequiredActionKind=rewrite_definition_all_of` 和原始 Definition revision；不得使用 `map[string]any`、自由文本 reason 或
   UI payload 作为 admission authority，也不得只凭 hash equality 合并 canonical signature bytes 不同的 violation；
5. ticket identity 由 `WorkspaceID + DefinitionRevision + violation code + canonical violation hash` 确定性派生。同一无效
   revision 的重复 setup/activation 返回同一 `BlockingTicketID`，零重复写；
6. required action 只能要求用户通过 Workspace owner command 提交新的 semantic Definition revision，将可替代分支改写为
   完整 all-of、唯一 provider 和明确 required refs。Decision owner 不读取 Workspace repository，也不替用户修改 Definition；
7. Workorchd 对新 revision 重新执行同一 pure validation，并将 concrete validation outcome 交给 Decision owner。新
   revision 通过 all-of admission 时，旧 ticket 以 `definition_adjusted` resolution 收口；仍无效时，旧 revision ticket
   以 `definition_revision_superseded` 收口，并为新 invalid revision 返回其确定性 blocking ticket。两种情况都不得自动
   Start/Resume/Restart 或创建 successor；
8. HTTP/Web/TUI 的 setup result 固定为
   `definition_clarification_required + ViolationCode + BlockingTicketID` 并提供可读修改入口；显示文本不能反向决定
   violation、ticket identity 或 resolution。

若 Decision owner 建票失败，activation 整体 fail-closed，仍不得创建 Run。

#### 5.6.2 Runtime-discovered semantic clarification

startup input 与 Run 已封存后，SM-3 的 bounded semantic document 可以返回
`definition_clarification_required` factual result，指出 current item/ref 可能存在 `implicit_one_of |
undeclared_prerequisite | semantic_ambiguity`。这只是 provider original diagnostic，不是 relation、Runtime action 或 ticket
command；LLM 不携带或生成 WorkspaceID、RunID、authority、ticket、replacement intent 或 target state。

1. SM-3 先把 complete document 与 original diagnostic 持久化；Orchestrator 从 current aggregate 派生并绑定
   `WorkspaceID + RunID + DefinitionRevision + StartupInputHash + AuthoritySnapshotID + FocusRef + LogicalRequestID`，形成
   typed `RuntimeDefinitionGapObservation`；
2. SM-2 只接受 exact-current scope、manifest-authorized offending item refs 和完整 document hash。stale/mismatched/duplicate
   observation 不能建票、改 graph 或增加 revision；不要求 Orchestrator 解释领域文本，**不能证明 all-of 完整**本身就是
   fail-closed clarification 事实；
3. SM-2 进入 canonical `need_governance`，SM-3 通过现有 sealed governance transaction 生成 definition-clarification
   dispatch；随后使用现有 `OpenRuntimeGovernanceRequestCommand` 向 Decision owner 创建实际 Run-scoped ticket。LLM 不能
   直接提交 ticket，Workorchd/Application 也不能伪造 authority/dispatch；
4. ticket 绑定 current Run、Definition revision、StartupInputHash、authority snapshot、focus、semantic request/document 和
   deterministic gap signature；同一 lineage 重驱返回相同 `BlockingTicketID`，不得重复 provider request 或 ticket write；
5. 从 observation 被接受开始，不得为 affected focus 创建 operation candidate/dispatch/effect；较早已封存的其它 focus effect
   保留审计事实，但不能因此把 gap 视为满足。current Run 不扩图、不重编译 Definition，也不从 provider 文本自动 replan；
6. ticket 唯一恢复要求是 `revise_definition_and_restart`。用户通过 Workspace owner command 创建新 Definition revision 后，
   ticket仍不允许 `continue` 或 Resume；只有用户显式提交 Restart、且 Run owner 已持久化 matching `ReplacementIntent`，
   Decision owner 才能以绑定该 intent 的 typed resolution 收口 ticket；
7. Restart 继续遵守 predecessor `Result -> Workspace -> Run` delivery 后才创建 successor 的既有主线。successor 使用新
   startup-input contract，不继承 predecessor scheduling contract、binding、credit、LLM document 或 context expansion；
   predecessor 的既有 owner effects 只能由 successor reconciliation 重新观察，不能直接继承 credit；
8. ticket persistence、resolution delivery 或 replacement-intent binding 失败时保持 governed wait 并走现有有界 recovery；
   不得降级为 operation、自动 Resume、自动 Restart、静默 terminal success 或无票据继续执行。

两类流程都只复用 Decision owner、Workorchd setup 编排与现有 SM-2/SM-3/SM-4 governance/recovery，不增加 Runtime 状态机、
framework unit 或第二个 Definition owner。

## 6. Semantic Scheduling Policy

### 6.1 Ready gate

一个 CWI 只有同时满足以下条件才进入 ready set：

- convergence state 为 `unbound | planning | reopened`；
- 所有 direct prerequisite 为 current `settled + satisfied + CurrentBindingRef`；
- 所有 all-of gate 的全部 member 已 current `settled + satisfied`，且 witness 完整；
- Definition/scheduling contract、reconciliation 和 current catalog lineage 未 stale；
- 它不是另一个 bound/effect-fenced current focus 的非法替换。

任何 priority 都不能越过 ready gate。

### 6.2 Semantic rank tuple

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
membership 数；canonical serialization/sort 的复杂度单独按第 12.5 节核算。

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

### 6.3 Dynamic ready projection

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
  StartupInputHash
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

### 7.4 Execution-time LLM over sealed startup inputs

Goal、DoD、Workspace Template snapshot 和 resolved project workspace directory 是 Workspace 的启动输入，不是
LLM 在执行期可编辑的草稿。Workspace owner 封存第 4.6 节的 `WorkspaceStartupInputContract` 且 Run 绑定
`StartupInputHash` 后，Runtime 可以在已有 bounded request/session、cache、budget 和 recovery 合同内调用 LLM：

1. 每个 semantic/operations request 绑定 exact `WorkspaceID + RunID + DefinitionRevision + StartupInputHash`；
2. C0～C2 中的长期合同、exact Goal/DoD partition 和 Template/Profile policy 保持稳定，工作目录只作为
   sealed capability scope 使用，不由 provider 重定向；
3. LLM 只返回 schema-bounded proof、candidate、context request 或 semantic-gap diagnostic；SM-2/SM-3 按 current
   authority 验证、封存或拒绝；
4. provider 试图重写 Goal/DoD、Template、directory、Definition revision、required CWI 或 all-of terminal 时，该字段
   不进入 authority，并以 typed schema/integrity result fail-closed；
5. 若 bounded processing 暴露 implicit one-of、undeclared prerequisite 或 semantic ambiguity，按第 5.6.2 节建
   run-scoped governed ticket；不允许 LLM 选分支、补写 relation 或将新文本回写 startup input。

因此，“启动输入不可变”与“执行期可调用 LLM”同时成立；本计划不把普通 setup 规定为 Runtime
的零 LLM 路径。

### 7.5 Optional hierarchical authoring assistance

普通 setup 先应用 Template/Profile 的确定性 disposition、priority/cost default、requires/provides 和 relation rules。
用户可以完全手工完成四项 startup input 的 authoring 和封存，无需强制调用 setup-draft LLM 或增加机器
草案确认页；这只是 authoring 产品能力，不限制第 7.4 节的执行期 LLM。大量 Goal/DoD 需要辅助时，用户可
显式开启 assisted draft：

1. Template、用户分组、semantic role 和 requires/provides 先形成确定性局部分区；
2. 每个有界分区向 LLM 提供 exact item text，生成组内关系草案；
3. 跨组请求只携带组 manifest、semantic role、requires/provides 和 hash，筛选候选跨组关系；
4. 只对候选关系的两个端点再次加载 exact text 复核；
5. UI 展示依赖、priority/cost、disposition、假设、cycle/缺失警告；用户明确采用后才提交 Workspace owner；
6. 未采用草案可丢弃，进程重启可丢失，不得进入 RuntimeBootstrap。

LLM 草案中出现的 ID 顺序、输出数组顺序和自然语言“先做第一个”均无 authority。deterministic compile 仍有歧义时，只能返回
可读的 `definition_clarification_required`；不得要求用户补 raw Runtime JSON，也不得把 LLM 草案自动升级为 canonical
relation。草案提出的 `one_of/any_of` 即使被用户点击采用也不能绕过 all-of admission；UI 必须把它显示为需要改写的
Definition violation，而不是合法 relation 选项。

### 7.6 Independent budgets

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
| SM-1 | 无业务变化；Run seal fresh bootstrap/current-baseline schema 及 exact startup-input identity | 不选择 dependency、priority、focus 或 next action |
| SM-2 | 验证并封存 scheduling contract/partition selection/all-of gate witness；纯投影 ready window；接纳 exact-current `RuntimeDefinitionGapObservation` 并进入 `need_governance`；settle/reopen 后重投影 | 不调 provider、不执行 operation、不解析领域词汇、不改写 startup input |
| SM-3 | 删除全量 `coverage_order` document；只处理 bounded proof batch、context request、semantic-gap document、operations candidate 和 sealed governance dispatch | 不选择下一 CWI；不写 scheduling graph/startup input；不直接建票 |
| SM-4 | drive 前要求 startup-input/scheduling contract/ready projection 完整；Resume/load 从同一 aggregate 幂等恢复；Restart 只走既有 replacement/delivery lane | 不从 projection/LLM 重建 authority，不修改其他 owner state，不把新 revision 混入 Resume |

目标 owner-local 顺序：

```text
activation fact
  -> SM-2 cwi_scheduling_contract_sealed
  -> SM-2 eligible_focus_projected + partition/witness sealed
  -> SM-3 proof-batch request/evaluation
     -> [semantic gap] SM-3 persists diagnostic
        -> SM-2 accepts RuntimeDefinitionGapObservation + need_governance
        -> SM-3 sealed governance dispatch -> Decision owner run-scoped ticket
        -> governed wait until explicit definition revision + Restart
  -> owner feasibility
  -> SM-2 binding/reconciliation seal
  -> SM-3 operations request/sealed dispatch/receipt/evidence
  -> SM-2 settle or reopen + eligible_focus_projected
```

`cwi_scheduling_contract_sealed` 是建议的 Orchestrator private fact 名称；实现必须复用唯一
`ApplyInternalEvent -> RuntimeTransitionCoordinator.Commit` mutation/CAS 边界。它不是 EventHub topic，也不允许 Application
携带 caller-authored ranks、ready queue 或 target state。

### 8.1 Governance, Resume and Restart

- 非 Definition-gap 的既有 governance wait 封存 current startup-input/scheduling contract、partition、focus、gate
  witness、dispatch fence 和 ready projection identity。resolution 只恢复同一 governed lineage；Decision/Approval、
  Application 或 UI 不得借 resolution 重排 CWI。
- 非 Definition-gap governance resolution 后若 owner fact 已 stale，SM-2 使用同一 graph 做 prerequisite/reconciliation
  和 targeted re-projection；它不能从 Workspace latest definition 或 LLM 重建 graph。
- runtime-discovered Definition-gap ticket 不存在 `continue` resolution；它在 current Run 中始终保持 governed wait。
  用户修改 startup input 产生新 Definition revision 后，仅 matching `ReplacementIntent` 和显式 Restart 可完成收口。
- Resume 只加载当前 Run 已封存的 `StartupInputHash`、Definition revision、partition set、scheduling contract、
  focus/witness、credit 和 recovery ledger。Resume 不重新运行 Workorchd compiler、不改 priority、不切换
  startup input/Definition revision，也不重放 durable effect。
- Restart 使用 Workspace owner **当前已封存**的四项 startup input 创建 fresh bootstrap、新 partition set 和新
  scheduling contract；successor 不继承 predecessor ready queue、binding、credit、LLM document 或 context expansion。
  它仍必须等待 predecessor 的 `Result -> Workspace -> Run` delivery 完成后由 Run owner 创建；predecessor
  已产生的 owner effect 仅由 successor reconciliation 重新观察，不直接继承 credit。
- startup recovery 只能从 Run owner opaque aggregate 恢复现有 scheduling/partition facts。缺失 current-baseline contract
  返回 `fresh_activation_required`，不得以 projection、event history 或 default order 补建。

### 8.2 Static and dynamic prerequisite ownership

- Workorchd 只把 owner-sealed startup-input identity、static Definition/Profile 已声明的 graph/mapping/partition 和声明式
  capability policy refs 编译进 typed bootstrap，由 Run owner 随 activation 封存；不从自由文本猜测未声明
  prerequisite，不在 Run 前探测 live capability、target revision、provider ceiling 或 governance readiness。
- Run 创建后，SM-2/SM-4 只通过 EventHub concrete typed acquisition/result 取得 dynamic owner facts。SM-2 是将这些 facts
  纳入 authority、binding 和 ready/fail-closed decision 的唯一 owner。
- capability owner 只返回自己的 readiness/feasibility/receipt；它不能修改 schedule、partition、focus 或 witness。
- static Definition failure 可阻止 Run creation；dynamic readiness failure 可以形成 prerequisite wait 或 typed terminal，
  但两者都必须发生在对应 provider request/dispatch/effect 前。
- 执行期 LLM 发现的 semantic gap 是第三条独立 lane：SM-3 只封存 diagnostic，SM-2 只按 current
  authority 接纳 observation 并进入 governance；它既不是 static compiler fact，也不是 capability readiness，
  不能触发 graph/startup-input mutation。

## 9. Component Landing Matrix

| Component | 必须收口 | 明确禁止 |
| --- | --- | --- |
| `blocks/workspace` | exact Goal/DoD、Template snapshot、resolved project directory、item lifecycle/disposition、all-of definition-confirmed relations、revision/`StartupInputHash`、owner-local typed mutation 和 retired lineage | Runtime focus/schedule、Profile-derived relation ownership、LLM draft authority、one-of persistence、跨 owner write |
| `blocks/decision` | static pre-Run workspace-scope 与 runtime-discovered run-scope 两类 Definition clarification ticket 的 concrete typed command/result、各自确定性 identity 和 owner-local resolution | 用 optional `RunID` 混合两类 scope、伪造 authority/replacement intent、读取/修改 Workspace Definition、启动或恢复 Run |
| `application/workorchd/biz/appservice/setupdraft` | **可选** bounded authoring draft，只有用户采用且通过 all-of validation 后才提交 owner command | 直接写 Workspace、强制 setup-draft LLM/机器草案确认、把手工 authoring 解释为 Runtime 零 LLM、把 one-of 作为选项、创建 Run、封存 Runtime graph |
| `application/workorchd/.../activationcore` | 将 owner-sealed startup input 与 Definition/Profile 已声明关系静态编译为完整 mapping/graph/all-of gates/partitions；返回 pure declared-completeness/capacity/admission violation | 猜测自由文本未声明 prerequisite、按数组/ID 定优先级、读取 live capability repository、创建 ticket、选择 Runtime focus |
| `application/workorchd` activation use case | 编排 pure validation、Decision owner pre-Run 建票和 typed setup result；validation 通过后才提交 Run activation intent | 持有 ticket/Definition 状态、把 static failure 送入 SM-2 governance、建票失败后继续创建 Run |
| `workorchruntimecontract` | startup-input identity envelope、semantic item/mapping/relation/schedule/all-of gate-witness/partition/context、typed gap observation DTO；纯 normalize/validate/hash/evaluator | EventHub、repository、CAS、one-of dependency/terminal evaluator、retry/terminal/state transition |
| `kernel/orchestrator` SM-2 | dynamic prerequisite admission、scheduling seal、rank、all-of ready/witness projection、partition selection、typed gap-observation admission/governance、targeted reopen、唯一 focus install | 领域推断、LLM call、startup-input mutation、第二 queue owner、ID/Order business sort、one-of terminal |
| `kernel/orchestrator` SM-3 | startup-input-bound bounded proof/C5-context/semantic-gap/operations transaction 与 sealed governance dispatch；删除 coverage-order transaction | 全局排序、依赖/startup-input 写入、C1 mutation、直接建票、第三 attempt |
| `kernel/orchestrator` SM-4 | persisted exact-input Resume/re-drive、guard、Restart replacement/delivery 和 terminal delivery不变 | graph/partition rebuild、将新 startup input 混入 Resume、projection authority |
| `blocks/llm` | immutable C1 partition + bounded C5 renderer、startup-input-bound request、closed proof/candidate/context/gap schema、audit；optional setup draft | 全量执行排序、dependency/priority/ticket authority、focus-driven C1、改写 Goal/DoD/Template/directory、raw diagnostic action |
| Template/Profile/System defaults | 中立 disposition、all-of 关系、business priority、requires/provides、gate和成本等级声明 | one-of/optional terminal 声明、修改 Runtime state、把软件语义下沉 kernel |
| capability blocks | 当前 readiness/feasibility/receipt owner facts | 选择 CWI、修改 graph/rank/partition、请求 target state |
| `blocks/run` | opaque aggregate/current-baseline persistence | 解码 scheduling contract、选择 focus |
| projection/Web/TUI | startup-input identity、static pre-Run 与 runtime-discovered clarification ticket、blocked reason、rank解释、context audit和progress展示 | 提供 one-of 选项、用显示文本决定 ticket/action、按显示顺序反向驱动 Runtime |
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
15. Definition aggregation、Profile terminal declaration、relation draft、Runtime dependency/terminal DTO/schema/prompt/evaluator
    中全部 `one_of`、`any_of`、`any_settled`、quorum、threshold、first-success surface，以及允许多 provider 歧义通过可替代
    gate 入场的 fallback；单个 concrete operation 的 owner-defined typed parameter union 不在删除范围；
16. 普通 setup 强制调用 setup-draft LLM 或要求用户确认机器草案的流程，以及因支持手工
    authoring 而将后续 Runtime 误定义为零 LLM 的禁令；
17. 未绑定 `StartupInputHash` 的执行期 LLM request/result，以及任何将 provider 改写文本回填 Goal/DoD、
    Template、project directory 或 RuntimeBootstrap 的路径；
18. 用 hash equality 代替 canonical semantic signature collision check 的 comparator。

迁移期间不允许新旧 scheduler 同时写 focus。可以先增加纯 DTO/helper 和 comparison test，但生产切换提交必须同时：

- 启用 scheduling contract；
- 删除 provider `coverageOrder` authority；
- 将所有 focus 选择路由到同一 SM-2 projection；
- 删除 Definition-order fallback；
- 启用 immutable C1 partition/C5 expansion renderer，并拒绝 focus-driven C1 mutation；
- 封存 Goal/DoD、Template snapshot、resolved project directory 的唯一 startup-input contract，并使全部执行期
  LLM request/result 绑定 `StartupInputHash`；
- 启用 all-of-only Definition admission、Decision owner static pre-Run workspace-scope clarification ticket 和
  runtime-discovered run-scope governed ticket，拒绝两类 scope 混用；
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

Exit：更高权威明确 dependency DAG/scheduling contract、ID/order 禁令、immutable startup input/C1 Definition
partition、execution-time LLM 只读边界、bounded C5 expansion、all-of-only terminal/gate causal witness、static pre-Run
与 runtime-discovered clarification 边界、static/dynamic/runtime-semantic-gap 三条 lane 和无全量 provider
`coverageOrder`；implementation SoT 同时把尚未落地内容标为 target 而非 current fact，且没有第五状态机或
第二 Owner。

### WGSC-01 — Workspace semantic-definition owner contract

Depends on: WGSC-00

Affected machines: Runtime SM 无变化；Workspace owner-local definition reducer。

修改：

- Workspace models、repository、owner commands/results/notifications；
- management/CLI/Web typed DTO；
- startup-input contract/snapshot/hash、Template snapshot、resolved project directory、item retire/supersede、
  disposition/mapping 与 clone/round-trip。

Exit：Goal、DoD、Template snapshot 和 resolved project directory 在 Run 前形成唯一 owner-sealed
`WorkspaceStartupInputContract`；item/disposition/lifecycle 和 definition-confirmed relation 由 Workspace owner 唯一持久化；Profile/compiled relation 保留
自己的 source ref/hash，不伪装为 Workspace mutation；用户关系或业务优先级变化必然改变 semantic definition revision；
required/constraint/context/all-of-aggregation mapping cardinality 完整；仅 presentation 重排不改变 semantic graph hash；
Workspace schema/command 不接受 one-of relation；duplicate/stale command 幂等；无 raw Runtime JSON。

### WGSC-02 — Startup-input authoring and optional assistance boundary

Depends on: WGSC-01

Affected machines: Runtime SM 无变化；Decision owner-local ticket reducer 与 Workspace owner-local definition reducer。

修改：Template/Profile deterministic defaults、setupdraft generator/schema/session、Decision owner static pre-Run clarification
ticket typed command/result 与 idempotency、Workorchd setup orchestration、HTTP/Web/TUI form 和 confirmation use case。

Exit：Template + Goal/DoD + optional directory 可通过纯手工 authoring 解析并封存为四项 startup input，不强制
setup-draft LLM 或机器草案确认，也不产生“执行期零 LLM”禁令；大 Definition 可选地分批提出组内/跨组关系；
未采用草案不写 owner；采用动作通过具体 Workspace owner command；LLM 输出顺序/ID 不产生 priority；歧义只返回可读
clarification，manual flow 完整可用；one-of 输入返回确定性 `BlockingTicketID`，同一 invalid revision 重试零重复写，且
ticket 不携带伪造 Run scope、不触发 Run creation/Start/Resume；mismatched workspace/revision/violation outcome 不得关闭
current ticket。

### WGSC-03 — Runtime neutral graph contracts

Depends on: WGSC-00、WGSC-01

Affected machines: 为 SM-2/3/4 提供纯合同，不拥有 transition。

修改：`internal/pkg/workorchruntimecontract` 的 startup-input identity envelope、semantic item/mapping、edge、all-of
gate/witness、唯一 terminal root、schedule、ready projection、Definition partition/C5 context、typed gap observation、预算和 hash。

Exit：startup-input identity 在 bootstrap/request/result/recovery DTO 中可精确传递且不可改写；
normalize/validate/clone/hash/permutation/collision tests 通过；graph traversal 为 `O(N+E+G)`，canonical serialization
最坏 `O((N+E+G) log(N+E+G))`；不存在 one-of dependency/terminal DTO/evaluator 或 transitive closure rank；rank static input 只计算一次，
comparator 不遍历 graph；包内无 EventHub/I/O/CAS/owner import；rank tuple 静态证明不读取 `Order`/ID 值。

### WGSC-04 — Workorchd activation compiler

Depends on: WGSC-01、WGSC-02、WGSC-03

Affected machines: 只构造 SM-2 输入，不迁移 Runtime state。

修改：activationcore work-unit identity、disposition/mapping、Profile relation compiler、requires/provides resolver、all-of
gate/terminal-root/partition compiler、capacity validation，以及 pure violation 到 Workorchd ticket use case 的 typed mapping。

Exit：相同语义 Definition 在输入顺序和 opaque ID 重命名后产生等价 graph/rank；已声明 prerequisite
未解析、cycle、ambiguity、duplicate 和 static capacity 超限均在 Run create 前失败；compiler 不声称发现自由文本中
未声明的 prerequisite；one-of/multi-provider ambiguity 先返回 typed violation，再由 use case 建立
Decision owner blocking ticket；compiler 自身无 EventHub/ticket side effect；每个 CWI 有 home partition；无 capability/LLM I/O。
dynamic owner readiness 不在本工作项伪装为 static fact。

### WGSC-05 — SM-2 scheduling seal and ready projection

Depends on: WGSC-03、WGSC-04

Affected machines: SM-2；SM-3/SM-4 只消费 persisted fact。

修改：typed dynamic prerequisite acquisition、`RuntimeDefinitionGapObservation` admission、Orchestrator private internal event、
authority component、CWI ledger/all-of gate-witness pure evaluator、partition selection、aggregate validator、progress signature。

Exit：one private event + one CAS 封存 scheduling contract；所有 focus 选择复用 one ready projector；duplicate 零 CAS；stale/CAS
conflict 重算；all-of gate witness 与 focus 同 CAS；任一 member stale 即撤销非 fenced dependent readiness；bound/effect-fenced
focus 不被替换；无 ID/Order/presentation sort/one-of branch；dynamic
readiness 在 provider/dispatch 前 fail-closed；exact-current gap observation 只能封存 `need_governance`，stale/duplicate
零票据/零 CAS，affected focus 零 candidate/dispatch/effect，不改 graph/startup input。

### WGSC-06 — Remove global coverage-order transaction

Depends on: WGSC-05

Affected machines: SM-3，及 SM-2/SM-3 lineage fence。

修改：semantic DTO/schema/prompt/evaluator/ledger/batch/correction/request session、bounded definition-gap diagnostic、sealed
governance dispatch 和全部 fixtures。

Exit：semantic provider 只处理当前 bounded proof batch，gap 只能返回 closed factual diagnostic 而不能生成
ticket/action/branch；不存在 `coverage_order` request/document/hash/correction；batch identity 绑定
startup-input + scheduling-contract + ready-projection + gate-witness + Definition-partition hash；attempt 0/1、
whole-document recovery 和 stable-prefix 规则保持。

### WGSC-07 — Stable Definition partition and bounded external context

Depends on: WGSC-03、WGSC-05、WGSC-06

Affected machines: SM-2/SM-3/SM-4 request recovery；不改变 SM-1。

修改：Definition partition envelope/compiler、C0～C5 renderer、LLM preflight/schema、startup-input identity binding、
stable-prefix generation audit、C5 context-expansion ledger。

Exit：每次 provider request/result 精确绑定当前 `StartupInputHash`，provider 对 Goal/DoD、Template 或 directory 的
改写不进入 authority；C1 partition 和 C5 exact context 分别受独立预算约束；C1 partition text 无损且不随同 partition
focus 变化；attempt 0/1/whole-document recovery 的 partition/stable prefix 不漂移；跨 partition focus switch 是显式新
generation；合法 expansion 只改变 C5 并新建 logical request；未选定义不丢 authority；无跨 partition 全量正文默认复制。

### WGSC-08 — Bounded focus window and large-ledger convergence

Depends on: WGSC-05、WGSC-06、WGSC-07

Affected machines: SM-2/SM-4。

修改：分离完整 CWI ledger容量与 active focus window；settle/reopen 后按 graph 增量投影；gate membership/witness 与 direct
unlock counter 高效计算。

Exit：超过现有 64 focus 的合法 Definition 可通过有界 window 持续推进；任何 provider batch 仍不超过 sealed limit；
Resume/startup 只从同一 aggregate 恢复 exact startup-input identity，Restart 使用 fresh bootstrap；无进程内 scheduler、transitive reachability
materialization 或 N×M edge materialization。

### WGSC-09 — Projection and explainability

Depends on: WGSC-01、WGSC-02、WGSC-05、WGSC-07、WGSC-08

Affected machines: none；read-only projection。

修改：projection contract/store、HTTP/SSE、Web/TUI progress。

Exit：用户可查看 startup-input revision/hash、static pre-Run workspace-scope 与 runtime-discovered run-scope
clarification ticket、semantic prerequisite、all-of gate witness、blocked reason、effective priority、immediate unlock count、
critical depth、active window、partition/prefix generation 和 context included/omitted refs；显示顺序
不能回写 authority；projection rebuild 不触发 drive。

### WGSC-10 — Combined verification and live handoff

Depends on: WGSC-00～WGSC-09

Affected machines: SM-1～SM-4 combined regression，其中 SM-1 语义不变。

修改：focused tests、boundary verifier、file-backed/current-baseline fixtures、startup-input immutability、static pre-Run 与
runtime-discovered clarification、governance/Resume/Restart regression、deployed live matrix 和所有 current docs。

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
| startup input seal | exact Goal/DoD、Template snapshot、resolved directory 与 Definition revision 共同产生稳定 `StartupInputHash` |
| change any startup input | 新 owner-sealed startup-input snapshot/hash；Goal/DoD/关系变化另产生新 Definition revision；当前 Run/bootstrap/graph 不变 |
| required item has no CWI / CWI has two primary items | static mapping validation fail-closed before Run creation |
| constraint has no consumer / all-of aggregation has no gate | static disposition validation fail-closed |
| declared prerequisite has no provider | Run 创建前 `declared_prerequisite_unresolved + BlockingTicketID`；零 Runtime/provider/dispatch/effect |
| free text may imply an undeclared prerequisite | static compiler 不做领域猜测或伪报完备；执行期只能走 typed semantic-gap lane |
| dangling/self dependency | fail-closed before Run creation |
| hard/reopen cycle | exact typed cycle path；零 Runtime work |
| explicit one-of in Goal/DoD/Profile/draft | `definition_one_of_unsupported + BlockingTicketID`；零 Run/provider/dispatch/effect |
| ambiguous providers | 不提供 any-of escape；返回 clarification blocking ticket并要求唯一 provider/required refs |
| duplicate invalid activation | 同一 revision/violation 返回同一 ticket；Decision owner零重复写，仍无 Run |
| corrected static-ticket Definition revision | 重新执行 pure all-of validation；通过后以 `definition_adjusted` 收口 ticket，但不自动 Start/Resume/Restart |
| replacement revision still one-of | 旧ticket以`definition_revision_superseded`收口；新revision取得确定性blocking ticket；仍无Run |
| stale/mismatched clarification outcome | workspace/revision/violation signature不匹配时拒绝关闭current ticket；不影响新revision |
| Decision ticket persistence unavailable | activation整体fail-closed；零 Run/provider/dispatch/effect |
| semantic duplicates | commutative、合并或 typed ambiguity；绝不按 ID兜底 |
| fan-in/fan-out | 使用 all-of gate，graph size接近 `O(N+E+G)` |
| all-of gate | focus witness逐项覆盖全部 member和 exact current evidence lineage |
| terminal root | 唯一 `TerminalAllOfGate` 覆盖全部 required terminal member；缺一项不能 complete |
| input permutation | Definition/scheduling hash稳定 |

### 12.2 Semantic priority

| Scenario | Required assertion |
| --- | --- |
| high-priority goal has low-level prerequisite | prerequisite继承 effective high priority |
| one item directly unlocks many high-priority dependents | 优先于无关小项 |
| deep fan-out with large transitive closure | 不计算 descendant cardinality/reachability materialization |
| ready-queue comparison | comparator不遍历graph；通常`O(1)`，signature hash collision时仅做`O(L)` canonical bytes比较 |
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
| manual startup authoring | Template + Goal/DoD + blank directory 无需强制 setup-draft LLM 即可封存，目录解析为默认路径；不限制后续 Runtime LLM |
| execution-time LLM identity | 每个 request/result 绑定 exact Workspace/Run/Definition/`StartupInputHash`；stale/mismatch 在 provider/authority mutation 前拒绝 |
| large Definition | provider只收到 bounded exact C1 partition；aggregate保留完整 authority |
| focus changes inside one partition | C1/StablePrefixHash不变，只改变C3 |
| focus changes across partitions | 先持久化partition-switch generation，再创建新logical request/prefix |
| proof batch contains multiple CWI | 全部home partition相同；cross-partition CWI必须拆为另一logical request |
| omitted unrelated partition | 不进入 prompt noise，但在 partition/context audit 可见 |
| required external context | 只可请求 manifest-authorized refs；C5 有界扩展且 C1 不变 |
| attempt 0/1 | DefinitionPartitionHash、C0～C3、C5 expansion和StablePrefixHash精确相同 |
| LLM returns global order | strict schema rejection；零 focus mutation |
| LLM orders by IDs | 输出无该字段/权限，不能影响 scheduler |
| LLM proposes rewritten Goal/DoD/Template/directory | schema/integrity fail-closed；零 Workspace/bootstrap/graph/startup-input mutation |
| LLM exposes implicit one-of | 只封存 bounded diagnostic；SM-2/SM-3 建 current Run scoped governed ticket，affected focus 零 candidate/dispatch/effect |
| LLM exposes undeclared prerequisite/semantic ambiguity | 不自动补 relation 或扩图；绑定 current authority 的 `revise_definition_and_restart` ticket |
| assisted draft returns one-of | adoption validation拒绝并进入clarification ticket；用户确认不能提升为合法relation |
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
| all-of gate witness member reopened | 原 witness 立即 stale且 dependent 退出ready；全部member再次current后才可重封存 |
| gate witness stale after effect fence | 不换focus/witness、不重放effect；原receipt收敛后拒绝新credit并targeted reopen |
| ordinary governance wait/resolution | startup input/graph/partition/focus/witness保持；resolution不触发重排 |
| duplicate runtime gap observation/redrive | 同一 lineage 返回同一 `BlockingTicketID`；零重复 provider request/ticket write/CAS |
| stale or mismatched runtime gap observation | 拒绝建票、改 graph 或增 revision；current authority 不变 |
| runtime definition-gap ticket | 保持 governed wait，无 `continue`/Resume resolution；affected focus 零 candidate/dispatch/effect |
| revised input while current Run waits | Workspace 产生新 revision/hash，current Run 仍绑定旧 startup input；不自动 Resume/Restart |
| Resume | 复用同一aggregate/`StartupInputHash`/schedule/partition/focus；不吸收新 revision、不重编译、不重放effect |
| Restart after definition gap | 只在 explicit Restart + matching `ReplacementIntent` + predecessor delivery 后fresh compile；不继承schedule/binding/credit/LLM document/context expansion |
| predecessor already has owner effects | successor reconciliation 重新观察；不直接继承 credit |
| dynamic capability unavailable | Run后由SM-2 typed fail-closed，且provider/dispatch/effect均为零 |
| all CWI settled | artifact/coverage closeout后 sealed terminal |
| partial delivery | 仅重试缺失的 Result/Workspace/Run acknowledgement |
| old aggregate | `fresh_activation_required`；零 provider/capability work |

### 12.5 Scale and performance

至少提供 1,000 CWI、线性/分支/聚合图的 deterministic fixture，并证明：

- graph traversal、cycle validation、priority propagation、critical depth、direct unlock 和 full ready fact evaluation 为
  `O(N+E+G)`；设 ready set 为 `R`、bounded window 为 `W`，ready heap/window selection 为
  `O(R + W log R + W log W)`；完整 ready-set serialization/hash 另计 `O(R log R)`；graph canonical
  normalization/hash 的最坏上界另计 `O((N+E+G) log(N+E+G))`；
- 不构造 transitive reachability matrix/bitmap/count；单项 settle/reopen 的增量路径为
  `O(A+E_A+G_A+Q log R)`，其中 `Q` 是受影响 ready-heap entry 数；最坏不超过一次 full fact evaluation + window selection；
- static rank input 只在 scheduling seal/reseal 时 reverse-topological 计算；ready comparator 不做 graph walk，通常 `O(1)`，
  hash collision fallback 为 `O(L)`；direct unlock counter 只读取 current facts + sealed contract 的纯派生
  unsatisfied-member counter，不形成第二份 authority；
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
go test ./internal/modules/blocks/decision/... -count=1
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

1. Goal、DoD、Workspace Template snapshot 和 resolved project directory 在 Run 前由 Workspace owner 封存为唯一
   startup-input contract；item、lifecycle/disposition 和 definition-confirmed relation 由 Workspace owner 唯一持久化；
   Profile/compiled relation 保留来源且 Definition→CWI mapping 完整；
2. Goal/DoD aggregation、Template/Profile terminal declaration、relation draft、Runtime dependency/terminal DTO/schema 只支持
   all-of；所有 required CWI 进入唯一 terminal all-of root，one-of/multi-provider ambiguity 没有合法 escape；单个 operation
   的 typed parameter union 不受影响；
3. Workorchd 只静态编译 owner-sealed input 中已声明的 neutral graph/all-of gate/partition，不猜测自由文本中
   未声明 prerequisite，不读取 live capability 或持有 Runtime scheduling state；
4. 显式 one-of 和已声明合同的 ambiguity/unresolved prerequisite 在 Run 创建前返回 deterministic
   Decision owner workspace-scope blocking ticket；同一 invalid revision 幂等，修订通过后只收口 ticket 而不
   自动 Start/Resume/Restart，且全程无伪造 Run scope；
5. 执行期 LLM 暴露 implicit one-of/undeclared prerequisite/semantic ambiguity 时，只能形成绑定 current
   authority 的 bounded diagnostic；SM-2/SM-3 通过既有 governance lane 创建 run-scope ticket，affected focus 在
   ticket/restart 收口前零 candidate/dispatch/effect；
6. 所有执行期 LLM request/result 绑定 exact `StartupInputHash`；provider 不能改写 Goal/DoD、Template
   snapshot、project directory、Definition revision、required CWI 或 all-of terminal；
7. SM-2 是 dynamic prerequisite admission、scheduling contract、all-of gate witness、ready projection、partition selection、
   runtime gap admission、focus 与 reopen 的唯一 owner；
8. execution priority没有任何 Definition position、`Order`、ID字典/数字或presentation priority输入；static rank input
   以 `O(N+E+G)` 预计算，ready comparator不遍历graph，且不存在transitive closure rank；
9. provider `coverageOrder`生产面和全部 fallback已删除；
10. 完整 CWI ledger与bounded active window、C1 partition、C5 external context已分离；
11. 同一 partition 内 C1/StablePrefixHash不随focus变化；cross-partition switch显式建新generation；expansion只改变C5；
12. static declared-prerequisite/cycle/ambiguity/capacity 在 Run 创建前失败；dynamic owner readiness 在
    provider/dispatch/effect 前处理；runtime semantic gap 不伪装成前两者；
13. 手工 Template + Goal/DoD + optional directory authoring 不强制 setup-draft LLM，assisted draft 只有用户采用后才
    进入 Workspace owner；该能力不构成执行期 Runtime 零 LLM 要求；
14. Definition-gap 修订不能被 Resume 吸收；只有 explicit Restart + matching `ReplacementIntent` 经 predecessor
    delivery 创建 fresh successor，不继承 predecessor credit/LLM document/context expansion；
15. 1,000-CWI自动化矩阵、duplicate/stale/governance/Resume/Restart/CAS/budget/guard全部通过；
16. fresh-data deployed run证明至少两个dependency wave正向推进并最终完成artifact closeout和
    `Result -> Workspace -> Run` delivery；
17. core contract、cache note、Orchestrator/Workspace/Decision/Workorchd/LLM README、current-state、flow、verification和UI文档一致；
18. active目录不存在同范围第二份代码收口计划。

局部 artifact、一次合法 dispatch、正确 fail-closed、revision增长、mock-only成功或单个CWI settled均不足以关闭本文。
