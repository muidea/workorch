# WorkOrch Runtime Flow Overview

Status: active

Type: runtime-flow

Last Updated: 2026-07-14

## Purpose

本目录定义 WorkOrch **生产主线 Runtime V2** 的运行链路，而不是对象定义或模块目录树。

本层只回答：

- 一次 typed activation / bootstrap 如何形成 Run 与 initial facts
- drive / reducer 如何在 prerequisite、candidate、sealed dispatch、operation evidence 之间推进
- assessment ledger 与 Result projection 如何收口，而不由 Result 裁决
- repair 如何在同一 focus 上重新收敛；replan 语义如何只能通过新 activation/run 表达
- candidate request 的 `RuntimeCandidateContext` 与 C0–C5 stable prefix 如何组装

## Production Mainline

生产主线 = **Runtime V2 only**。

- Start / Resume 正式入口只进入 Runtime V2
- `DriveRuntimeV2Command/Result` 是唯一同步推进合同
- action 只来自 typed reducer；operation 只经 sealed dispatch 与 `ExecuteRuntimeOperationCommand/Result`
- 不存在生产主线中的 V1 `StagePacket`、`NextAction`、graph dispatch、`ExecutionContextPatch`、自动 replan successor

### Dual-Layer Language

| 层 | 语言 | 用途 |
| --- | --- | --- |
| 中立 stage / profile | semantic / profile 语言 | Goal/DoD、coverage focus、evidence role、stage 语义、capability 边界 |
| Runtime V2 drive | implementation 语言 | bootstrap、facts、reducer decision、candidate、`SealedDispatch`、operation evidence、CAS aggregate |

中立 stage 描述“当前需要什么”；V2 drive/reducer/sealed dispatch 描述“如何确定性收敛”。

### V2 Drive Loop

```text
Start / Resume
  -> InitializeRuntimeV2FromRun (typed bootstrap -> initial facts / load aggregate; 幂等)
  -> DriveRuntimeV2 (有界循环)
       |
       +-- AcquirePrerequisite
       |     resolve by owner module (manifest / target snapshot / capability / ...)
       |     CommitRuntimeV2Prerequisite -> reducer
       |
       +-- ProduceStateChange | ProduceValidation | ProduceAssessment | RequestGovernance
       |     BeginCandidateAttempt (attempt 0/1, same transaction + stable prefix)
       |     RequestRuntimeCandidate (RuntimeCandidateContext + DefinitionContext)
       |     Accept/Fail candidate attempt
       |     seal SealedDispatch (transaction + candidate hash + typed step input)
       |     |
       |     +-- Assessment  -> CommitRuntimeV2Assessment (assessment ledger)
       |     +-- Governance  -> OpenRuntimeGovernanceRequest -> governed_wait
       |     +-- Operations  -> ExecuteRuntimeOperation (File/Git/Shell/MCP/Web/SSH)
       |                       Commit execution evidence (coverage-bound) -> reducer
       |
       +-- Complete | Fail
             project Result (accepted/rejected) via Result owner
             project Run / Workspace terminal
```

Drive 非等待、非终态的每次 transition 必须增加 persisted `stateRevision`；无进展或超过 transition guard 必须 fail-fast。重复 `Drive` 与进程重启只从 Run owner aggregate 幂等恢复。

## Reading Order

1. [activation-definition-flow.md](activation-definition-flow.md) — bootstrap、definition/activation seal、Initialize 幂等
2. [planning-execution-flow.md](planning-execution-flow.md) — candidate → seal → ExecuteRuntimeOperation → evidence
3. [verification-acceptance-flow.md](verification-acceptance-flow.md) — produce_validation、assessment ledger、Result projection
4. [repair-replan-flow.md](repair-replan-flow.md) — same-focus reconverge vs new activation/run
5. [evidence-result-flow.md](evidence-result-flow.md) — commit invariants、stale→diagnostic、CAS aggregate
6. [context-assembly-flow.md](context-assembly-flow.md) — `RuntimeCandidateContext` 与 C0–C5 stable prefix

## Cross-Layer Boundary

| 层 | 职责 |
| --- | --- |
| `00-core/` | 中立 stage contract、context integrity、provider-neutral cache 规则 |
| `10-domain-model/` | 对象、边界、领域不变量 |
| `20-runtime-flow/`（本目录） | Runtime V2 运行链路与收口顺序 |
| `30-module-architecture/` | 模块落点、EventHub、实现文件 |
| `40-profiles-capabilities/` | profile / template / MCP capability 特化 |
| `internal/modules/kernel/orchestrator/readme.md` | Orchestrator 组件交互与合同权威说明 |
| `internal/pkg/workorchruntimecontract` | typed bootstrap / facts / candidate / dispatch / state 合同 |

## Non-Goals (Removed From Production Mainline)

以下内容**不是** Runtime V2 生产主线，文档与实现对齐时不得重新包装为正式路径：

- V1 `StagePacket` / `NextAction` / graph dispatch owner
- `ExecutionContextPatch` resume、application continuation queue
- 自动 replan successor lifecycle
- V1 runtime checkpoint replay、`NextActionForRun`、`DispatchNextStep`
- 以 `RuntimeBootstrap == nil` 或 `runtimeContractVersion` 做 V1/V2 路由

## Source Documents

- `../00-core/runtime-stage-contract.md`
- `../00-core/runtime-request-cache-note.md`
- `../../internal/modules/kernel/orchestrator/readme.md`
- `../../internal/pkg/workorchruntimecontract`
