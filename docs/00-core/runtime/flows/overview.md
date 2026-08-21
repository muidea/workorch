# WorkOrch Runtime Flow

Status: normative

Type: runtime-flow-index

Last Verified: 2026-08-18

## Purpose

本目录按一次端到端数据流解释 Runtime 四状态机如何协作。它不重新定义 owner、transition、DTO 或模块目录；
发生冲突时以 [Runtime Core](../../overview.md) 和组件实现 SoT 为准。

## Flow Map

| Flow | Entry and exit | Focus |
| --- | --- | --- |
| [Activation And Definition](../../definition-convergence/activation-flow.md) | eligible Workspace input → Run activation/initial aggregate | candidate promotion、bootstrap、SM-1/SM-4 initialize |
| [Planning And Execution](planning-execution.md) | current authority → sealed operation receipt | prerequisite、candidate request、seal、capability execution |
| [Verification And Acceptance](verification-acceptance.md) | validation/assessment need → credit/focus/terminal | accepted evidence、assessment ledger、formal outcome |
| [Repair And Restart](repair-and-restart.md) | current gap、bounded recoverable wait or explicit restart → same-focus convergence or fresh successor | wait condition/trigger、repair budget、replacement intent、delivery ordering |
| [Evidence And Result](evidence-result.md) | owner receipt → aggregate evidence/terminal delivery | owner revision set、three-axis projection、closeout、Result→Workspace→Run |
| [Context Assembly](../../runtime-request-context-assembly.md) | sealed request identity → provider-safe C0～C5 | exact Definition partition、manifest、budget、audit |

## Reading Rule

按正在修改的数据流只读一个 focused flow；需要判断合法状态迁移时转到
[Four State Machines](../state-machines.md)，需要判断 candidate/dispatch 时转到
[Candidate Contract](../candidate-and-dispatch.md)，需要判断 request/cache 时转到
[Request And Cache](../../runtime-request-cache-note.md)，需要判断跨调用/操作消费时转到
[Execution Budget](../execution-budget.md)，需要判断无需人类选择的自动等待时转到
[Recoverable Wait](../recoverable-wait.md)。

## Layer Boundary

- `00-core/runtime/` owns normative Runtime semantics and invariants;
- `00-core/definition-convergence/` owns Definition admission、scheduling and clarification;
- this directory owns sequence and handoff explanation only;
- `10-domain-model/` owns business objects and owner-local invariants;
- `30-module-architecture/` owns code placement and EventHub boundaries;
- `40-profiles-capabilities/` owns domain/capability specialization;
- focused verification stays with the subject whose contract it proves。

Flow 文档可以引用 command/result 和 state，但不得建立第二份 transition table、owner authority、兼容执行路径或
实施进度说明。生产路径始终是 typed bootstrap、Run SM-1、Orchestrator SM-2/3/4、sealed dispatch 与 owner-ordered
terminal delivery。
