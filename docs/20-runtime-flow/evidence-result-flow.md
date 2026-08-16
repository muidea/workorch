# Evidence And Result Flow

Status: active

Type: runtime-flow

Last Verified: 2026-08-16

## Purpose

本文件定义 Runtime 过程 evidence 如何提交、如何与 authority 对齐、如何进入
assessment / formal Result，以及 aggregate 如何 CAS 持久化。

## State Machine Binding

本 flow 绑定核心规约中的 SM-2 evidence ledger、SM-3 dispatch/evidence transaction、SM-1 terminal lifecycle 与 SM-4 CAS/restart/projection delivery。stale/duplicate/conflict 只能按核心规约进入 diagnostic/idempotent 路径，不得推进 authority。

完整规约见 [Runtime 四状态机固化规约](../00-core/runtime-stage-contract.md#runtime-四状态机固化规约)。

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

## Commit Invariants

evidence commit 必须同时满足：

1. **Authority 一致**：workspace/run、definition revision、evidence revision、focus、
   authority snapshot 与当前 aggregate 决策一致
2. **Dispatch 一致**：存在对应 `SealedDispatch`；candidate hash / transaction 匹配
3. **Operation 一致**：evidence 来自 sealed operation（kind、id、coverage work item IDs、target refs）
4. **Coverage 绑定**：execution evidence 必须携带 sealed operation 的 coverage work item IDs；
   前序 focus evidence 不得关闭后续 focus
5. **幂等**：duplicate event id / 已处理 resolution 不得二次改变 ledger；重复调用只返回一致结果
6. **Definition 一致**：definition context revision 与 facts / transaction 一致，漂移 → conflict

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

## Result Path

```text
reducer Terminal (Complete | Fail)
  -> Orchestrator seals terminal result payload from:
       Terminal summary
       assessment ledger facts + evidence refs
       GoalID / DoDID from definition
  -> terminal delivery fact
  -> Result owner record -> Workspace owner release -> Run owner terminal
```

约束：

- Result **不判断** acceptance；只持久化正式 outcome
- accepted Result 引用 assessment ledger 中的 current-run evidence refs
- rejected Result 携带 terminal summary 与未满足 assessment / failure 解释
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
- [verification-acceptance-flow.md](verification-acceptance-flow.md)
- [planning-execution-flow.md](planning-execution-flow.md)
- `../00-core/runtime-stage-contract.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`（`runtime_state.go`、`evidence_record.go`、`canonical_runtime.go`）
