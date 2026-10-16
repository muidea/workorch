# Runtime Result Closeout And Requirement Projection

Status: normative

Type: domain-model

Last Verified: 2026-08-29

## Purpose And Authority

本文定义 formal Result closeout、适用 owner revision set 及逐 requirement/CWI 三轴投影的稳定值合同。Orchestrator 从
SM-2/3/4 canonical ledger 裁决并 seal；Result owner 只验证 matching terminal delivery 后幂等保存；workorchd 只做一一对应的
detached projection。任何一层都不得重新评估 DoD、补造 evidence 或把投影状态写回 authority。

## Applicable Owner Revision Set

```text
RuntimeOwnerRevisionRef
  OwnerKind
  OwnerInstanceID
  ResourceKind
  ResourceID
  SnapshotID
  Revision
  Fence
  DependencyHash

RuntimeOwnerRevisionSet
  DefinitionRevision
  Entries[]
  SetHash
```

每个 entry 标识 evidence/binding/assessment 实际读取的一个 owner fact。`OwnerKind + OwnerInstanceID + ResourceKind +
ResourceID` 是唯一资源键；`SnapshotID`、`Revision`、`Fence` 至少有一个非零/非空，且只能按 receiving owner 合同解释。
`DependencyHash` 绑定实际消费的字段/selector/target，不是整个 owner repository hash。

`Entries[]` 按资源键、snapshot、revision、fence canonical 排序，禁止同一资源键出现冲突版本；`SetHash` 由
`DefinitionRevision + canonical entries` 计算。set 只包含该 closeout/requirement 实际依赖的 owner facts，不引入全局 world
counter。owner mutation 只使 dependency set 精确相交的 evidence/binding stale；无关 mutation 不改变 set 或 CWI 状态。

每个 evidence item保存自身 `RuntimeOwnerRevisionSet` 或其可解引用 immutable hash。Result closeout的set是所有 current
accepted evidence、accepted assessment和正式artifact dependency set的canonical union；union不能把未依赖owner加入其中。

`CompileRuntimeOwnerRevisionSetUnion` 是此 union 的纯值入口：每份输入必须保持原始 canonical 正文并通过完整
set/hash 校验，Definition 必须 exact-match；不同集合中完全相同的 entry 幂等合并，同资源的 snapshot、revision、fence
或 dependency hash 冲突必须拒绝，不能任取一个版本。资源键按四个独立字段比较，不通过分隔符拼接形成歧义。
空 union 不补造 owner fact。此 helper 不证明传入集合确实来自 producer；receiving owner 的实际读取、typed receipt、
aggregate 来源反证和 current applicability 仍须独立成立，不能用 self-hash 或 synthetic evidence revision 替代。

`RuntimeOwnerRevisionSetMatchesCurrent` 是 current applicability 的严格纯值比较入口：依赖与 current 集合分别保持
raw canonical 正文并通过完整 set/hash 校验，Definition exact-match；current 中每个被依赖的四字段资源键必须存在，
且完整 snapshot/revision/fence/dependency 字段一致。无关资源新增、移除或改变不影响比较；缺失或相交版本变化返回
不匹配，非法／冲突集合返回错误。它不证明 owner 观察的来源或 freshness，不发起查询，不选择 stale/reopen transition，
也不能使没有 actual producer 来源的集合获得 evidence credit。

## Historical Reads And Applicability Dependencies

The opt-in `evidence-applicability-dependencies/v1` value separates two immutable collections on the same operation
receipt and EvidenceFact. `OwnerRevisionSet` retains the complete original actual reads, including mutation inputs and
observed absence. `ApplicabilityDependencies` contains an exact `ProtocolRef` and a nonempty, raw-canonical
`OwnerRevisionSet` produced from separate actual owner observations of the postcondition. Both collections must bind the
same Definition; missing historical reads, unknown protocol, invalid raw identity, conflicting resources or wrong hashes
are rejected. Clone, normalization and reload preserve both original bodies; receipt and fact identities seal both bodies.

The second collection is the dependency snapshot to be tested, not a currentness certificate. It cannot be populated from
proposed content, `AfterHash`, current catalogs, receipt chronology or a later operation's reads. A source consumer must
independently reverse-prove the exact receipt, dispatch, execution key/fence and fact payload, even after coordinated
rehashing. Nil does not mean no dependencies, and an old receipt is never backfilled with a postcondition observation.

File mutations observe complete actual bytes after success under the owner effect lock, including recovery confirmation.
The applicability resource selector identifies the workspace, exact target and observed complete byte identity rather than
the execution ID. Equal observations can agree across executions without weakening strict version conflicts; a failed read,
missing target or drift from the sealed postcondition emits no applicability proof. Original mutation input observations
remain separate, and recovery records only the observations actually made during recovery.

SM-2 must compare applicability snapshots with authenticated, revision-fenced current owner observations before relying on
them for acceptance. An intersecting change invalidates only the affected evidence/binding/assessment and reopens the exact
CWI under its existing budget and effect responsibility; unrelated changes do not. Historical receipts and reads remain
immutable. SM-3 cannot dispatch from stale authority; SM-4 cannot bypass the SM-2 adjudication or choose a newest resource
version to make a conflicting union succeed. The terminal top-level current collection and historical per-evidence reads
remain distinct, and immutable terminal material is replayed without recertifying its old acceptance against mutable owners.

Managed Shell validation preserves the actual copied-input set independently as its applicability snapshot. Its concrete
query rereads only those original complete file selectors (bytes, size and mode), reverse-proves the persisted receipt and
execution source, and obtains the directory from Workspace. Missing is a factual observation; errors do not produce a set.
This does not enumerate newly added files or trace every subprocess read. For versioned proof, the terminal current union
uses the actual SM-2 observation rather than historical mutation inputs; per-evidence and per-assessment historical reads
remain intact. Equal current resources may merge; conflicting versions are rejected, never resolved by selecting newest.

## Three-Axis Requirement Projection

```text
RuntimeRequirementOutcome
  RequirementRef
  SourceItemRef
  CoverageWorkItemID
  DefinitionRevision
  CoverageState
  AssessmentState
  AssessmentReasonCode
  MissingEvidenceKinds[]
  EvidenceItems[]
  AcceptedEvidenceRefs[]
  OwnerRevisionSetHash

RuntimeRequirementEvidenceItem
  EvidenceRef
  BindingRef
  State
  ReasonCode
  OwnerRevisionSetHash
  OwnerRevisionSet           optional original evidence read snapshot
```

允许值固定为：

| Axis | Values |
| --- | --- |
| `CoverageState` | `uncovered | partially_covered | covered` |
| `AssessmentState` | `unassessed | satisfied | unsatisfied | indeterminate` |
| evidence item `State` | `candidate | accepted | rejected | stale_or_superseded` |

三轴分别派生，不能压缩成 overall enum、百分比或 score：

- coverage 只看当前 CWI 的 required binding/evidence completeness；部分 required evidence current 时为
  `partially_covered`，全部 current 且 accepted时为 `covered`。
- assessment 只看绑定该 CWI 和 current accepted evidence set 的最新 applicable accepted assessment fact；不存在为
  `unassessed`，明确通过为`satisfied`，明确不通过为`unsatisfied`，证据/owner fact不足以裁决为`indeterminate`。
- 每个 evidence ref 独立投影状态；同一 CWI 可以同时保留 accepted 与 rejected/stale历史项，不能用一个 summary state覆盖。
- `AcceptedEvidenceRefs[]` 必须恰好等于 `EvidenceItems[]` 中 current `accepted` 项的canonical ref set；其它状态不得贡献coverage
  credit。
- `indeterminate` 必须带 `AssessmentReasonCode`，并至少给出 `MissingEvidenceKinds[]` 或缺失 owner fact reference。当前 all-of
  基线下它永远不支持successful terminal。
- owner revision set/hash、Definition revision或binding mismatch的item只能是`stale_or_superseded`；不能回退为candidate或继续credit。

终态 evidence 投影必须解析 exact current CWI binding（Definition、ID、hash、focus），并验证当前 proof、完整
validation all-of、metric/artifact 和同 Workspace/Run 的来源。无 rollover 时反查原 completed dispatch binding；
同 binding ID 新 hash 不直接继承旧来源，只有唯一 exact equivalent resolution 与完整 proof identity 一致时适用。
历史 accepted 保留审计；旧 binding、弱证据、不完整 proof、跨 scope 或重复／冲突 resolution 投影为
`stale_or_superseded`，不进入 current `AcceptedEvidenceRefs`。此只读 compiler 不回写 coverage/assessment，
terminal seal 与 fresh-load 仍从独立持久前像反证材料；receipt/dispatch 完整来源门禁不因投影而省略。

逐 evidence 的可选 `OwnerRevisionSet` 保全原 receipt/fact 的完整读取快照，包括 rejected/stale 项，
独立校验 raw canonical、hash 与 exact Definition，并纳入 closeout/material seal。它不是 top-level
`OwnerRevisionSetHash` 的另一份值，不证明当前性，也不参与 credit 重新裁决。不同 evidence 的历史读版本分别保存，
不能从 evidence chronology 任取最新版或把等价 proof lineage 当作 owner freshness。未采样时保持 nil；
没有 current owner observation 和完整 assessment/artifact 来源前，逐项保全不等于完整 terminal union 已实现。
反馈 v5 的 top-level set 由原 current-proof evidence reads 与仍适用的 accepted assessment adjudication reads 作 exact
canonical union，不再生成 `runtime_authority/accepted_evidence` 伪 owner 依赖。相同资源的冲突原读取必须失败，不能选
最新版；stale/rejected evidence 保留逐项原读取，但不进入当前 union。未采样的 nil 不补造空 observation；这份 observed
union 不冒未接线的 artifact 读取来源或全 owner currentness。历史 patch input 不是新的 postcondition observation。
旧版 aggregate 直接拒绝且保持原归档，不自动升级或填充依赖；新版本只进入新 activation。

## 逐源义务终态投影

Workspace 的 eligible exact candidate 编译不可变 `RuntimeSourceObligationInventory`，保留 Requirement、
Obligation、Verification、原判据、原声明及 candidate/specification/Definition/startup 身份。
多项义务可以共用一个 CWI；每项 verification 编译不同 check identity，进入该 CWI 的完整 all-of proof。
SM-2 仍以原 CWI ledger 裁决，不增加第二份可变义务闭包账本。

`RuntimeResultCloseout.SourceObligations` 保留完整封存 inventory；每个 requirement outcome 的
`SourceObligations[]` 分别携带 exact check、CoverageState、AssessmentState、reason、current binding 与
accepted evidence refs。closeout 校验从 inventory 重编完整检查集合，缺项或改判据不能靠重新计算 hash 通过。
Result 与 workorchd detached 透传已裁决材料；UI 不从共同 DoD 猜来源或自行判断完成。

当前源结论遵循共同 CWI 的 all-of 门禁：只有父 CWI covered+satisfied 且 current binding 的完整 proof
由 accepted evidence 满足，源检查才投影 covered+satisfied。仅 A、旧 binding、reopened CWI 或历史 assessment
不能恢复任一源义务的当前 credit；投影不提供 A 部分通过的独立 credit ledger。
原 Criterion/Declaration 文本不 trim、不摘要；review、creation、exit 0 与单项 native PASS 均不能替代整体证明。
显式跨 CWI 用途映射的逐源索引使用 `source-check/<原 CWI checkRef>`，保留历史来源与验收来源的独立条目；
未映射库存保持原索引身份。历史条目的原 Criterion/Declaration/check identity 不变，但普通验收 binding 与
evidence 必须来自确认的唯一 consumer。该索引只标识原来源，不增加或恢复独立 credit；行为与当前证明的
合取及 stale 撤销遵守 [Evidence Proof Purpose](../00-core/runtime/evidence-proof-purpose.md)。
requirement 与逐源 evidence 投影共用 exact current binding/proof 适用性检查：无 rollover 时反查同 Workspace/Run 的
原 completed dispatch binding；有来源 lineage 时匹配原 dispatch/operation、binding hash 及 current proof identity。
同 ID 新 hash 只能通过该 scope 内 exact equivalent resolution 重新获得适用性；incompatible、跨 Run/Workspace、
重复来源、缺失当前 binding、弱证据或不完整 all-of 均保留为历史审计，不进入 current accepted refs。
这些纯投影不重新裁决 coverage/assessment；terminal seal 与 fresh-load 仍从独立持久前像反证全部材料。
原始 provider/process 材料仍由诊断 owner 的受限读取边界保护，closeout 不携带该私有原始存储内容。

## Formal Result Value Contract

```text
FormalRuntimeResultRecord
  ResultID
  WorkspaceID
  RunID
  ExecutionLineageID
  GoalID
  DoDID
  State                    draft | accepted | rejected | superseded
  OwnerRevision
  RuntimeCloseout          RuntimeResultCloseout
  TerminalDelivery
  CreatedAt
  UpdatedAt

RuntimeResultCloseout
  SchemaVersion
  CloseoutID
  StartupInputHash
  DefinitionRevision
  RuntimeStateRevision
  TerminalOutcome
  RequirementOutcomes[]
  AcceptedEvidenceRefs[]
  DeliverableRefs[]
  ArtifactRefs[]
  OwnerRevisionSet
  KnownLimitations[]
  OperationOutcomes[]
  FailureClass
  FailureCode
  FailureSignature
  OriginalDiagnostic
  PredecessorRunID
  PredecessorResultID
  RecoveryPolicy
  RecoveryLedger
  CloseoutHash
```

`TerminalOutcome` 是 Orchestrator 已裁决的 typed completed/failed outcome。成功 closeout 必须满足：

1. 每个 terminal all-of member恰有一个matching `RequirementOutcome`；无缺失、重复或额外CWI；
2. 每项均为`covered`，要求assessment时均为`satisfied`，且至少一个current accepted evidence支持其required binding；
3. 不存在`unassessed`、`unsatisfied`、`indeterminate`或仅有stale/rejected evidence的required CWI；
4. top-level `AcceptedEvidenceRefs[]` 等于逐项accepted refs的canonical union；
5. `OwnerRevisionSet` 等于这些evidence、assessment、deliverable/artifact实际dependency set的canonical union；
6. terminal all-of witness、Runtime state revision与terminal delivery引用同一aggregate closeout transaction。

rejected closeout 仍必须逐项保存已满足、未满足、未判定和stale lineage，以及typed failure/gap；不能伪造成空结果。
具体 owner 原因来自已持久化 FailureFact，或 SM-2 在 owner-fact 有界耗尽时封存的 terminal `OriginalDiagnostic`；
无 FailureFact 不等于没有具体原因。终止 code 与原始 owner code 保持独立，原始 requirement/check/target/revision 信息
不得由 Result 或 Application 重建、归一成泛化错误或用于重新选择下一轮动作。原因参与 closeout hash，duplicate/restart
必须保持原始值。当前 Runtime 只允许 `ResultFeedbackVersionCurrent` 的反馈合同；缺失、旧版或未知版本在
record/aggregate/material 准入失败，不保留旧 renderer，也不升级、补写或删除旧归档。同版本重启及 ack 丢失的重投
仍必须产生完全相同的 Result 内容与 hash。
`KnownLimitations[]` 只描述未被Definition声明为required的已知限制；不能豁免required CWI或把`indeterminate`变为成功。

`RequirementOutcomes[]`、refs、limitations和operation outcomes canonical排序后计算`CloseoutHash`。same `CloseoutID`的duplicate
delivery只有在`CloseoutHash`一致时幂等成功；hash不同必须fail-closed。Result owner不得补默认值使无效successful closeout通过。

Result owner 成功持久化 terminal material 后，其 delivery acknowledgement 必须返回 exact owner-derived `ResultID`。SM-4 仅把
该 immutable fact 转交 Run owner，由后者随 terminal lifecycle封存为 `TerminalResultID`。显式 Restart 创建 successor 时，
`PredecessorRunID + PredecessorResultID` 必须来自 predecessor 的 Run-owned terminal record，且 predecessor lineage CAS先于
successor activation；任何层都不得从 canonical ID 规则重新猜测或从 projection 反向补齐 lineage。

## Projection Contract

workorchd公开的稳定读模型为：

```text
RuntimeCoverageView
  WorkspaceID
  RunID
  DefinitionRevision
  RuntimeStateRevision
  RequirementOutcomes[]
  OwnerRevisionSetHash
  ViewHash

RuntimeResultView
  ResultID
  State
  RuntimeCloseout
  OwnerRevision
```

active projection可以从current aggregate ledger生成`RuntimeCoverageView`；terminal Result view必须来自Result owner保存的exact
closeout。两者revision不一致时必须分别标示stale/omit，不能把active projection与旧Result拼接成新的成功结果。HTTP/Web/TUI
字段命名可以遵循各自DTO casing，但必须一一对应，不得删除axis、reason、revision或lineage。

## Verification

至少覆盖：canonical排序/hash、duplicate closeout、缺失/重复CWI、三轴互不覆盖、accepted ref union、indeterminate拒绝成功、
unrelated owner mutation不失效、intersecting mutation精确stale/reopen、terminal delivery response loss、Result owner reload和
projection revision mismatch。Runtime transition验证见 [Core Runtime Verification](../00-core/runtime/verification.md)。
