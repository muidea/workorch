# Canonical Definition And Dependency Model

Status: normative

Type: core-contract-part

Last Verified: 2026-08-25

## Workspace-owned semantic items

Workspace definition 使用 owner-owned typed item，而不是只有位置化字符串：

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
确认页；deterministic admission 返回 `definition_clarification_required` 时，UI 提供的是阻塞纠正入口，不能通过
确认原 candidate 放行或直接 promotion。两条路径都不得要求用户提交 Runtime JSON、target、recipe、check、metric、artifact
或 capability readiness。

## Item lifecycle and Definition→CWI mapping

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
一旦 candidate 出现此类显式输入，必须在 promotion/Run 创建前返回 `definition_one_of_unsupported` 并进入
[clarification flow](clarification.md)。没有 DoD 关联的
Goal 只能由 Profile 明确编译为 `required_coverage`，或在 promotion/Run 创建前返回 `definition_clarification_required`。
Orchestrator 不按 `Kind=goal|dod` 自行猜 disposition。

## Canonical relation classes

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
pre-seal candidate 为产生 typed rejection 而保留的 unsupported token 也不是 relation origin，不能被 promotion。
其中只有 `definition_confirmed` relation 是 Workspace owner 可修改并持久化的 Definition 数据；
`profile_declared` 来自已选 Profile snapshot，`input_output_compiled` 是 Workorchd 对已封存声明的纯派生 bootstrap
fact。Workorchd 不成为 relation owner；Run owner 随 bootstrap 冻结来源，SM-2 只在 scheduling contract 中
封存经校验的 compiled relation/hash。

## All-of dependency gates

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

当前 canonical Definition 不定义 `IndeterminatePolicy`、assessment threshold 或其它终态降级字段。每个
`TerminalAllOfGate` member 必须同时达到 current `covered`、存在适用 accepted evidence，并在要求 assessment 时为
`satisfied`；`unassessed`、`unsatisfied`、`indeterminate` 或 stale evidence 均不能贡献 terminal witness。candidate、Goal/DoD、
Template/Profile 或 assisted draft若声明 `indeterminate_as_success`、unknown-pass、best-effort-close 等等价策略，pre-seal
admission 必须以 `definition_terminal_policy_unsupported` 形成 blocking clarification，不能 promotion。执行期模型输出该策略
则按 contract rejection处理，不能修改 Definition或 terminal gate。

## Admission and runtime completeness validation

Workorchd 必须先对 Workspace owner-persisted immutable `DefinitionAdmissionCandidate` 完成 pure static admission，再由 Workspace owner
promotion；activation compiler 只能消费 promotion 后的正式 startup input。pre-seal admission 必须验证：

1. candidate 同时具有 exact Goal/DoD input、Template snapshot、resolved project directory、稳定 `CandidateHash` 和同一
   Workspace/Candidate revision scope；
2. 无空 identity、悬空引用、自依赖或重复冲突关系；
3. hard dependency 与 reopen graph 分别无 cycle；
4. 每个 `RequiresRef` 恰好由初始事实或一个唯一 provider item 满足；每个 required owner fact 也必须恰有一个
   sealed convergence declaration：preexisting，或 exact missing trigger + 唯一 acquisition owner/kind/selector +
   reobserve owner/fact + `MaxAcquisitionAttempts=1`；`prospective_owner_target` 还必须携带 nonempty exact
   `ProspectiveSatisfactionContractRef`，而 preexisting/其它 target policy 禁止携带；确实要求多个输入时必须拆成不同
   required ref 并由显式 all-of 汇合；
5. 多 provider、`one_of/any_of/any_settled`、quorum/threshold/first-success relation 或其他可替代分支语义一律拒绝；
6. 不存在 `indeterminate_as_success`、unknown-pass、assessment threshold 或其它降低 all-of terminal completeness 的策略；
7. 每个 disposition 与 Definition→CWI mapping 满足本文的 mapping cardinality；
8. required all-of aggregation/finalization gate 及唯一 terminal all-of root 的 member set 完整且 hash 稳定；
9. semantic duplicate 已合并、声明为 commutative，或 fail-closed；
10. immutable Definition partition 完整覆盖 active item，每个 CWI 绑定一个 home partition，partition 无超限或正文摘要；
11. Definition item、edge、gate、gate membership、partition 和总字节数符合彼此独立的容量预算；
12. 不因 context budget 较小而裁剪 authority inventory；
13. candidate 封存完整 normalized typed Profile snapshot，hash 与 candidate 中封存值一致；每个 Profile-required
    acceptance/deliverable source 必须映射到一个已 admission 的 exact CWI，并可由唯一 activation compiler 生成 canonical
    exact proof declaration，且具有非空受支持 evidence kinds 和至少一个可返回对应 typed owner receipt 的 accepted
    operation；wildcard 只能作为已 admission exact CWI 的声明模板，不能替代 exact CWI proof 或注入 current owner fact、recipe/native check、dispatch/credit authority；
14. Profile requirement/convergence declaration 无悬空 dependency 或 cycle，candidate bindings 与从 exact Profile snapshot
    独立派生的 manifest 逐项一致；
15. admission result 在输入 slice/map permutation 后保持同一 candidate/admission-result hash。

只有上述检查返回 `admitted_all_of` 后，Workspace owner 才可 promotion。随后 activation compiler 必须从正式
`WorkspaceStartupInputContract` 重新验证 candidate/admission-result lineage，编译 Definition→CWI mapping、graph、gate、partition
和 bootstrap；结果必须绑定 promotion 产生的 `DefinitionRevision + StartupInputHash`。任何 candidate/promoted snapshot 漂移均返回
stale/conflict，不得复用旧 admission result 或部分编译物。

Static completeness 只检查**已声明合同**。activation compiler 生成的 exact declaration 只命名已经 admission 的 CWI，
不从 Goal/DoD 文本推断新的业务要求。稳定错误码 `declared_prerequisite_unresolved` 表示 candidate-owned 显式
`RequiresRef`/`DependsOnItemRef` 没有唯一 source/provider closure；`definition_profile_contract_invalid` 表示 Profile-required
CWI 没有 exact executable proof closure；`definition_profile_prerequisite_unresolved` 表示 Profile-derived required owner fact
或其 candidate manifest 没有唯一 acquisition/reobserve closure。后两者要求 `select_or_fix_template_profile`，前者要求
`rewrite_definition_all_of`；三者都不宣称发现 Goal/DoD 自由文本中从未声明的业务前置项。
Workorchd 不使用关键词、文本相似度或领域规则补写 relation。Template/Profile 可以在自己的领域层确定性声明关系；可选
setup draft 只有用户采用后才成为 Workspace owner candidate；只有 admitted promotion 后才成为 definition-confirmed input。

Run 创建后、首次 semantic provider request 或 durable effect 前，SM-2 再通过 concrete typed owner command/result 完成
dynamic readiness validation：provider ceiling、capability/policy/catalog revision、validation surface、target selector 和
必要 governance contract 均必须 current。Workorchd 不读取 capability repository、不探测 owner，也不把“当前可用”写入
bootstrap。static ambiguity 返回 Definition clarification；dynamic unavailable/stale 先使用现有SM-2 typed
prerequisite/recovery合同：只有owner-defined condition、trigger/deadline和budget完整时可进入bounded recoverable runtime
wait，否则沿semantic-contract typed fail-closed主线。执行期 LLM 暴露的隐含 semantic gap 是第三类输入：它由 SM-3 先持久化为 scoped
observation，再由 SM-2 进入 [runtime-discovered clarification](runtime-clarification.md)
governed wait；不得伪装成 static compiler fact 或 owner readiness。
