# Definition Context Strategy

Status: normative

Type: core-contract-part

Last Verified: 2026-08-25

## Large Goal/DoD Context Strategy

### Authority and immutable Definition partitions

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

semantic manifest 的 provider-selectable `OwnerCatalogs` 可以按 C3 bounded projection 暴露 exact allowed values；
prospective owner-fact composite probe 使用的 `OwnerFactDependencyCatalogs` 不属于 prompt context。它们只在 aggregate
中参与 manifest/hash/freshness/CAS/seal，不能进入 C3、schema enum、correction directive 或任何 LLM-visible catalog dump。
Template 与 Workspace 两侧的 evidence-policy snapshots 在 Definition admission 时分别形成 canonical
`AdmissionPolicyBindings`，作为稳定 C2 policy authority；任一含有退役 `prospectiveTargetRef` 的 snapshot 在 promotion
前即拒绝，binding 本身也不能向 C3/SM-3 注入 exact target。
pre-seal Definition/Profile authority 只声明 prospective target policy、declaration selector/candidate refs 与
satisfaction contract；generation-specific semantic obligation/manifest 再与 current owner catalog 编译 operation
selector/constraints，C3 只投影其 provider-visible bounded 部分。SM-3 semantic document 的 proposal 在两者交集内
提出 exact target；File/Shell fact 对该 proposal 进行绑定观测，只有 SM-2 sealed binding 才形成 target
authority。proposal、owner fact 或上一 generation target 均不得回写 C1/C2。

“Goal/DoD 不压缩”和 stable-prefix 规则具体化为：

- canonical full Definition 在 Workspace owner 与 Run aggregate 中完整保留；
- C1 中 partition 的每个 member 都使用 exact、未摘要、未损失正文，不允许按 current focus 再切片；
- C1 只携带 Definition root identity、current partition envelope 和该 partition 正文，不线性复制其他 partition；
- current focus 在同一 partition 内变化时只改变 C3，C0/C1/C2 和 `StablePrefixHash` 必须保持不变；
- attempt 0/1、whole-document recovery 和同 semantic transaction 的所有 physical call 必须复用同一
  `DefinitionPartitionRef/Hash` 与 stable prefix；
- prospective target 的 typed `all_of` mismatch 使用普通完整 document/schema attempt 1：只在 C4 增加
  manifest/CWI/field-path-bound violation 引用，复用同一 semantic batch、C0～C3 与 stable prefix，且不复制 C3
  constraint。exact-current full-composite `would_not_satisfy` 直接形成 owner-contract contradiction terminal，不生成 C4；
  typed stale 只刷新 fact；
- 从一个 home partition 切到另一个时，SM-2 先持久化新的 logical request 和
  `StablePrefixChangeDisposition=definition_partition_switch`。这是显式新 stable-prefix generation，不是 renderer drift；
- 同一 Run 内只有 sealed partition switch 可以预期改变 stable prefix；Definition revision 或 C2 immutable contract
  revision 变化必须使用 fresh activation，不能 patch 当前 Run。除此以外的 C0/C1/C2 hash 变化一律是
  `unexpected_stable_prefix_drift`。

### Partition compilation and request selection

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

### Bounded cross-partition context expansion

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

### Execution-time LLM over sealed startup inputs

Goal、DoD、Workspace Template snapshot 和 resolved project workspace directory 是 Workspace 的启动输入，不是
LLM 在执行期可编辑的草稿。Workspace owner 封存
[WorkspaceStartupInputContract](definition-admission.md#pre-seal-admission-startup-input-immutability-and-execution-time-llm) 且 Run 绑定
`StartupInputHash` 后，Runtime 可以在已有 bounded request/session、cache、budget 和 recovery 合同内调用 LLM：

1. 每个 semantic/operations request 绑定 exact `WorkspaceID + RunID + DefinitionRevision + StartupInputHash`；
2. C0～C2 中的长期合同、exact Goal/DoD partition 和 Template/Profile policy 保持稳定，工作目录只作为
   sealed capability scope 使用，不由 provider 重定向；
3. LLM 只返回 schema-bounded proof、candidate、context request 或 semantic-gap diagnostic；SM-2/SM-3 按 current
   authority 验证、封存或拒绝；
4. provider 试图重写 Goal/DoD、Template、directory、Definition revision、required CWI 或 all-of terminal 时，该字段
   不进入 authority，并以 typed schema/integrity result fail-closed；
5. 若 bounded processing 暴露 implicit one-of、undeclared prerequisite 或 semantic ambiguity，按
   [Runtime clarification](runtime-clarification.md) 建
   run-scoped governed ticket；不允许 LLM 选分支、补写 relation 或将新文本回写 startup input。

因此，“启动输入不可变”与“执行期可调用 LLM”同时成立；本合同不把普通 setup 规定为 Runtime
的零 LLM 路径。

### Independent budgets

下列预算必须使用不同字段、validator 和 audit，禁止因数值相同而互换：

- `DefinitionInventoryBudget`：完整 owner definition/CWI 数量与字节；
- `SchedulingGraphBudget`：edge、gate、gate membership、fan-in/fan-out 和图深度；
- `DefinitionPartitionBudget`：单个 immutable C1 partition 的 item/byte 上限；
- `DefinitionPartitionSetBudget`：partition 数量与 mapping bytes；
- `ActiveFocusWindowBudget`：一次 SM-2 ready window；
- `SemanticProofBatchBudget`：一次 per-CWI proof batch；
- `DefinitionExternalContextBudget`：一次 C5 exact external context；
- `DefinitionContextExpansionBudget`：同 semantic transaction 的 expansion generation 总数；
- `RuntimeDefinitionClarificationBudget`：单次 gap signature、offending refs、safe diagnostic 与 ticket binding 的数量/字节上限；
- 现有 Owner inventory、candidate、request recovery、repair、prerequisite refresh 预算。

总 Definition 超过安全上限必须在 promotion/Run 创建前返回 `definition_capacity_exceeded`，不得静默截断。正常大 Definition 只限制
partition、active window、provider batch 和 external context，不限制 canonical CWI ledger 的完整性。
