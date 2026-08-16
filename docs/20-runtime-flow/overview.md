# WorkOrch Runtime Flow Overview

Status: active

Type: runtime-flow

Last Verified: 2026-08-16

## Purpose

本目录定义 WorkOrch **生产主线 Runtime** 的运行链路，而不是对象定义或模块目录树。

本层只回答：

- 一次 typed activation / bootstrap 如何形成 Run 与 initial facts
- drive / reducer 如何在 prerequisite、candidate、sealed dispatch、operation evidence 之间推进
- assessment ledger、formal Result 与派生 projection 如何依次收口，而不由 Result owner 裁决
- repair 如何在同一 focus 上重新收敛；显式 Restart 如何通过 Run-owned replacement transaction 形成 fresh Run
- candidate request 的 `RuntimeCandidateContext` 与 C0–C5 stable prefix 如何组装

## Production Mainline

生产主线 = **Runtime only**。

- Start / Resume 正式入口只向 Run owner 提交 intent；Run owner notification 才进入 Runtime
- `DriveRuntimeCommand/Result` 是唯一同步推进合同
- action 只来自 typed reducer；operation 只经 sealed dispatch 与接收 capability owner 的 concrete execute command/result
- 不存在生产主线中的 禁止的 `StagePacket`、`NextAction`、graph dispatch、`ExecutionContextPatch`、自动 replan successor

### Dual-Layer Language

| 层 | 语言 | 用途 |
| --- | --- | --- |
| 中立 stage / profile | semantic / profile 语言 | Goal/DoD、coverage focus、evidence role、stage 语义、capability 边界 |
| Runtime drive | implementation 语言 | bootstrap、facts、reducer decision、candidate、`SealedDispatch`、operation evidence、CAS aggregate |

中立 stage 描述“当前需要什么”；Runtime drive/reducer/sealed dispatch 描述“如何确定性收敛”。

### Four Coordinated State Machines

生产 Runtime 必须遵守核心合同中的四状态机规约：

1. SM-1 Run lifecycle；
2. SM-2 authority/focus convergence；
3. SM-3 candidate/sealed dispatch transaction；
4. SM-4 drive/recovery/projection delivery。

本目录仅按 flow 拆解四状态机，不重新定义 transition。完整状态、合法/禁止迁移、协调协议和变更门禁以 [Runtime 四状态机固化规约](../00-core/runtime-stage-contract.md#runtime-四状态机固化规约) 为准。

### Activation And Reconciliation

- Fresh Start：Run owner封存 application 编译的 Definition seed/activation，SM-2 在需要时通过有界 semantic-plan、
  capability-owner feasibility 与 private internal event 密封 proof binding/reconciliation contract；
- Resume：Run owner只恢复同一 current-baseline Run，Orchestrator加载原 aggregate 并幂等 re-drive，不重读或替换
  bootstrap/provider/Soul/capability selection；
- Restart：application 从 Workspace canonical state 编译 fresh bootstrap，Run owner先把 `ReplacementIntent` 持久化在
  predecessor；只有 `Result -> Workspace -> Run` terminal delivery 完成后，Run-private reducer 才创建并激活 successor；
- non-current predecessor：Run owner返回 `data_reset_required`，不创建 replacement intent 或 successor。

Application/projection/capability owner 都不能直接修改 coverage。File、Shell、Git、MCP 等 owner只返回
workspace/run/revision-scoped observation/receipt；SM-2 经唯一 internal-event/CAS retain credit、接纳新 evidence 或记录
`reopened` lineage。Duplicate/stale request/receipt 在进程重启后仍按 persisted identity 幂等处理。

### Runtime Drive Loop

```text
Start / Resume intent
  -> Run owner-local lifecycle transition -> RunStateChanged fact
  -> InitializeRuntimeFromRun (typed bootstrap -> initial facts / load aggregate; 幂等)
  -> DriveRuntime (有界循环)
       |
       +-- need_prerequisite
       |     resolve by owner module (manifest / target snapshot / capability / ...)
       |     CommitRuntimePrerequisite -> reducer
       |
       +-- need_state_change | need_validation | need_assessment | need_governance
       |     BeginCandidateAttempt (attempt 0/1, same transaction + stable prefix)
       |     RequestRuntimeCandidate (RuntimeCandidateContext + DefinitionContext)
       |     Accept/Fail candidate attempt
       |     seal SealedDispatch (transaction + candidate hash + typed step input)
       |     |
       |     +-- Assessment  -> CommitRuntimeAssessment (assessment ledger)
       |     +-- Governance  -> OpenRuntimeGovernanceRequest -> governed_wait
       |     +-- Operations  -> ExecuteRuntimeOperation (File/Git/Shell/MCP/Web/SSH)
       |                       Commit execution evidence (coverage-bound) -> reducer
       |
       +-- Complete | Fail
             terminal delivery fact
             Result owner -> Workspace owner -> Run owner local delivery
```

Drive 非等待、非终态的每次 transition 必须增加 persisted `stateRevision`；无进展或超过 transition guard 必须 fail-fast。重复 `Drive` 与进程重启只从 Run owner aggregate 幂等恢复。

## Reading Order

1. [activation-definition-flow.md](activation-definition-flow.md) — bootstrap、definition/activation seal、Initialize 幂等
2. [planning-execution-flow.md](planning-execution-flow.md) — candidate → seal → ExecuteRuntimeOperation → evidence
3. [verification-acceptance-flow.md](verification-acceptance-flow.md) — need_validation、assessment ledger、formal Result 与 projection
4. [repair-replan-flow.md](repair-replan-flow.md) — same-focus reconverge vs explicit replacement activation
5. [evidence-result-flow.md](evidence-result-flow.md) — commit invariants、stale→diagnostic、CAS aggregate
6. [context-assembly-flow.md](context-assembly-flow.md) — `RuntimeCandidateContext` 与 C0–C5 stable prefix

## Cross-Layer Boundary

| 层 | 职责 |
| --- | --- |
| `00-core/` | 中立 stage contract、context integrity、provider-neutral cache 规则 |
| `10-domain-model/` | 对象、边界、领域不变量 |
| `20-runtime-flow/`（本目录） | Runtime 运行链路与收口顺序 |
| `30-module-architecture/` | 模块落点、EventHub、实现文件 |
| `40-profiles-capabilities/` | profile / template / MCP capability 特化 |
| `internal/modules/kernel/orchestrator/readme.md` | Orchestrator 组件交互与合同权威说明 |
| `internal/pkg/workorchruntimecontract` | typed bootstrap / facts / candidate / dispatch / state 合同 |

## Non-Goals (Removed From Production Mainline)

以下内容**不是** Runtime 生产主线，文档与实现对齐时不得重新包装为正式路径：

- 禁止的 `StagePacket` / `NextAction` / graph dispatch owner
- `ExecutionContextPatch` resume、application continuation queue
- 自动 replan successor lifecycle
- 禁止的 runtime checkpoint replay、`NextActionForRun`、`DispatchNextStep`
- 以 `RuntimeBootstrap == nil` 选择另一条执行路径

## Source Documents

- `../00-core/runtime-stage-contract.md`
- `../00-core/runtime-request-cache-note.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`
