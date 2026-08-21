# Runtime Result Closeout And Requirement Projection

Status: normative

Type: domain-model

Last Verified: 2026-08-18

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
`KnownLimitations[]` 只描述未被Definition声明为required的已知限制；不能豁免required CWI或把`indeterminate`变为成功。

`RequirementOutcomes[]`、refs、limitations和operation outcomes canonical排序后计算`CloseoutHash`。same `CloseoutID`的duplicate
delivery只有在`CloseoutHash`一致时幂等成功；hash不同必须fail-closed。Result owner不得补默认值使无效successful closeout通过。

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
