# Orchestrator Module

## 功能定位

`kernel/orchestrator` 是 WorkOrch runtime convergence authority。它负责阶段状态、NextAction、StagePacket、coverage/evidence 解释、contract gate、repair/replan、artifact continuation 与 runtime block dispatch。

## 拥有边界

- 拥有 runtime 编排权威、EvidenceRecord、CoverageWorkItem、ToolCall、Patch、ValidationResult、ArtifactContinuation 等执行证据与收敛状态。
- 不拥有 Workspace/Goal/DoD、Run、Result、Decision、Approval、Memory 等 owner state。
- 不内置软件开发、语言栈、测试框架或具体业务领域策略。

## 主要交互

- 从 `kernel/workspace` 与 `kernel/run` 读取权威快照。
- 向 `blocks/llm` 请求 LLM candidate，向 `blocks/file`、`blocks/git`、`blocks/shell`、`blocks/ssh`、`blocks/mcp`、`blocks/web` 派发 runtime execution request。
- 与 `kernel/result` 创建 accepted/rejected result 与 artifact。
- 与 `kernel/decision`、`kernel/approval`、`kernel/governance` 协作处理治理阻塞与恢复。
- 向 `kernel/eventstream` 发布 runtime progress、evidence、result finalization 等事件。

## 组件交互合同

- Snapshot contract: 每次 dispatch 必须基于同一组 workspace/run/definition/evidence 权威快照。
- Block request contract: 派发给 blocks 的请求必须是 StagePacket/StepExecutionRequest 派生的结构化请求，并携带 workspace/run/step scope。
- Block response contract: blocks 只返回 normalized result/error/raw ref；orchestrator 负责 evidence quality、coverage binding 和 next action。
- Result contract: 只有 accepted evidence 与 DoD coverage 满足后才能调用 `kernel/result` 创建 accepted Result。
- Governance contract: decision/approval 只用于真实治理 gap；resolution/approval resume 必须继承原 stage contract。

## 已知缺口

- 当前盘点未发现 Orchestrator module 职责缺失；但该模块是 P0 合同集中点，修改时必须补 StagePacket、requiredOutput、evidence gate 和 output mode 回归测试。

## 边界约束

- LLM candidate 不能直接拥有状态转移权；必须经过 contract gate。
- output mode、stage、convergence state 与 RunState 必须保持分层，不能相互污染。
- rejected/invalid output 不得产生 accepted evidence 或 Result。
