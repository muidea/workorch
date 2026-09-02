# Evidence And Result Flow

Status: normative

Type: runtime-flow

Last Verified: 2026-09-02

## Purpose

本文件定义 Runtime 过程 evidence 如何提交、如何与 authority 对齐、如何进入
assessment / formal Result，以及 aggregate 如何 CAS 持久化。

## State Machine Binding

本 flow 绑定核心规约中的 SM-2 evidence ledger、SM-3 dispatch/evidence transaction、SM-1 terminal lifecycle 与 SM-4 CAS/restart/projection delivery。stale/duplicate/conflict 只能按核心规约进入 diagnostic/idempotent 路径，不得推进 authority。

完整规约见 [Runtime State Machines](../state-machines.md)。

## Flow

```text
sealed operation / assessment / governance resolution
  -> validate authority snapshot + dispatch + operation identity
  -> commit path:
       accepted evidence  -> EvidenceLedger + evidenceRevision++
       required evidence ready -> reducer / assessment
       focus advance only when assessment contract (if required) closes explicit work items
       reducer
  -> stale / duplicate / conflict path:
       DiagnosticRecord only (no ledger closeout)
  -> terminal:
       publish immutable terminal delivery fact
       Result owner -> Workspace owner -> Run owner local transitions
  -> persist RuntimeStateRecord via Run owner CAS (expected revision)
```

## Evidence Sources (Runtime)

生产主线 evidence 来自 typed 提交，而不是 禁止的 Step/ToolCall/Checkpoint 事件总线重放：

| 来源 | 提交形态 |
| --- | --- |
| sealed operation result | execution / validation evidence（coverage-bound） |
| AssessmentCandidate commit | `RuntimeAssessmentFact` → assessment ledger |
| governance resolution | accepted governance evidence 或 rejected terminal |
| prerequisite commit | resource facts（manifest、target snapshot、capabilities 等），并触发 reducer |

同步 typed operation result 是提交 execution evidence 的**唯一**执行回执。
Eventstream 通知可作审计，**不**驱动 Runtime 状态推进。

实时 owner response 与重启后读取到的已持久化 receipt 使用同一裁决入口，并先判断 receipt outcome：rejected 保留 owner
original diagnostic，不再用 unknown observed effect 覆盖；accepted/no-op 才继续执行 effect compatibility 与 quality gate。

## Commit Invariants

evidence commit 必须同时满足：

1. **Authority 一致**：workspace/run、definition revision、evidence revision、focus、
   authority snapshot 与当前 aggregate 决策一致
2. **Dispatch 一致**：存在对应 `SealedDispatch`；candidate hash / transaction 匹配
3. **Operation 一致**：evidence 来自 sealed operation（kind、id、binding refs、target refs）；owner-authored receipt payload
   必须 exact 投影到 `EvidenceFact`，SM-2 diagnostic 不得改写 `EvidenceReason`
4. **Coverage 绑定**：`BindingRefs` 只保存 sealed operation causation；`CoverageWorkItemIDs` 只由 SM-2 exact proof matcher
   产生，前序 focus evidence 不得关闭后续 focus。`confirmed_validation_repair` 可按 sealed commitment 保留一个不声明
   `state_change` proof 的 source binding，此时 accepted repair fact 必须为零 CWI credit、不得生成 binding mismatch，并在
   完成后固定进入 fresh validation；普通 binding/proof mismatch 仍 fail-closed
5. **幂等**：duplicate event id / 已处理 resolution 不得二次改变 ledger；重复调用只返回一致结果
6. **Definition 一致**：definition context revision 与 facts / transaction 一致，漂移 → conflict
7. **Owner revision set 一致**：evidence只绑定binding/operation实际依赖的owner revision/snapshot/fence；其set/hash必须与
   current observation逐项匹配，无关owner变化不得扩大stale范围
8. **Outcome-first 一致**：live/recovery 不得形成两套 receipt 判断；rejected owner diagnostic 不得被后续 effect/quality
   检查改写，accepted/no-op 则不能绕过这些检查
9. **Fact 双记账**：每个 `evidence_committed` execution 保存 exact `EvidenceFactID + EvidenceFactHash`，并反证完整 fact body、
   receipt、operation、binding/CWI 与 focus lineage；该 mutable commit projection 不进入 immutable dispatch authority hash
10. **全局序列一致**：ordinary accepted/rejected fact、governance resolution/fact 与 assessment fact 共同消费唯一连续
    `EvidenceSequence`/`evidence/<n>` revision；每个 typed entry 由 owning dispatch 恰消费一次，resume clone 不重复消费

Orchestrator 判定 evidence quality、coverage binding 与 next action；block 输出不能自封 accepted。

## Stale → Diagnostic

下列情况**不得**写入 accepted ledger closeout，只能进入 diagnostic lineage：

- authority snapshot 已漂移（stale snapshot）
- dispatch 已 committed 或不存在
- governance resolution 与 binding 不匹配或已消费
- 重复 event / 重复 resolution
- 与当前 focus / required output 无关的迟到结果

`RuntimeDiagnosticRecord` 至少记录 event id、authority snapshot id、current snapshot id、
dispatch id、reason。diagnostic 保留可审计性，不推进 coverage satisfied。

## CAS Aggregate

Runtime 正式状态是 `RuntimeStateRecord`，经 Run owner EventHub update 合同持久化：

- Orchestrator 使用 **expected revision** 执行 CAS
- **不**直接访问 Run repository
- **不**把 aggregate 编码进 `map[string]any`
- 每个非等待、非终态 drive transition 必须增加 `stateRevision`
- 无持久化进展或超过 transition guard → fail-fast，禁止进程内空转
- 重启只加载 persisted aggregate；不依赖进程内去重 map

主要持久化字段族：

- `Definition` / `Facts` / `Decision`
- `ActiveDispatch` / `CompletedDispatches`
- `CandidateAttempts`
- `GovernanceRequests` / `GovernanceResolutions`
- `ProcessedEventIDs` / `Diagnostics`
- `StateRevision` / `EvidenceSequence`
- execution-budget sub-ledgers、recoverable-wait record与applicable owner revision set/hash

fresh-load 以 completed dispatch persisted order、随后可选 active dispatch 为唯一 chronology replay 全局 evidence sequence；
projection、eventstream 时间、slice 当前排序或每类 ledger 的局部计数均不是顺序 authority。最终 replay count 必须等于
`EvidenceSequence`，最终 revision 必须等于 `Facts.EvidenceRevision`。orphan/duplicate/gap、跨 ledger 重复、assessment kind
混入 ordinary evidence，以及 governance-resume 对 source fact 的二次消费全部 fail closed。

assessment dispatch 还保存两种互斥时点的 proof：pre-seal raw-canonical `RuntimeAssessmentAuthorityInput` 精确绑定 Definition
seed source、purpose、Focus/bindings 与 owner admissibility catalog，并进入 dispatch authority hash；post-seal
`RuntimeAssessmentAdjudication` 只在 commit-time 从 accepted evidence、completed dispatch 与 coverage proof lineage 重放，和
`RuntimeAssessmentFact.AdjudicationHash` 在同一 CAS 双记账。后者不回写 immutable dispatch authority，restart 必须重放并
逐字段匹配，不能以模型 disposition 或两份协调 self-hash 替代 classifier proof。

## Result Path

```text
reducer Terminal (Complete | Fail)
  -> Orchestrator seals terminal result payload from:
       Terminal summary
       Definition revision + GoalID / DoDID
       requirement/CWI coverage + assessment ledger facts
       current accepted evidence refs
       deliverable/artifact refs + known limitations
       applicable owner revision set/hash + execution lineage
  -> terminal delivery fact
  -> Result owner record -> Workspace owner release -> Run owner terminal
```

约束：

- Result **不判断** acceptance；只持久化正式 outcome
- accepted Result 引用 assessment ledger 中的 current-run evidence refs
- rejected Result 携带 terminal summary 与未满足 assessment / failure 解释
- successful Result逐项携带coverage/assessment/evidence projection，`indeterminate`不得被默认转换为satisfied；
- Result引用的evidence在closeout时必须仍匹配其实际依赖的owner revision set；不存在global world counter或全量owner
  revision fan-in；
- 同一 terminal delivery identity 的重复事实由各 owner 幂等消费；状态冲突 → conflict
- SM-4 只记录 acknowledgement/retry；不得借 terminal delivery 创建 禁止的 Step、越权改变 owner state，
  或经过 禁止的 Result/DoD completion gate

## Ownership Boundary

仍属 Orchestrator 收敛状态机内部、不对外暴露写入 repository 的对象包括：

- ToolCall / Patch / ValidationResult 过程记录（若作为 internal 证据载体）
- EvidenceRecord / CoverageWorkItem 运行态
- ArtifactContinuation（若存在）仅在 Runtime 不变量内，不得形成第二套 dispatch owner

Result / Artifact 的产品展示分层（deliverable vs process evidence）可在 read-model / API 层细化；
flow 层只要求：**正式业务结果与过程 evidence 分层**，raw payload 默认 ref-only。

## Key Flow Guarantees

- snapshot、dispatch、operation 三者一致才能提交 accepted evidence
- stale / duplicate → diagnostic only
- aggregate CAS + stateRevision 进展是 drive 安全阀
- Result 是 owner 持久化的 formal outcome，不是 judge；由它派生的 UI outcome 才是 projection
- 正式状态只以 typed `RuntimeStateRecord` 为准

## Source Documents

- [overview.md](overview.md)
- [Verification And Acceptance](verification-acceptance.md)
- [Planning And Execution](planning-execution.md)
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`（`runtime_state.go`、`evidence_record.go`、`canonical_runtime.go`）
