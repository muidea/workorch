# Orchestrator Module

## 功能定位

`kernel/orchestrator` 是 WorkOrch runtime convergence authority。它负责阶段状态、NextAction、StagePacket、coverage/evidence 解释、contract gate、repair/replan、artifact continuation 与 runtime block dispatch。

## 拥有边界

- 拥有 runtime 编排权威、EvidenceRecord、CoverageWorkItem、ToolCall、Patch、ValidationResult、ArtifactContinuation 等执行证据与收敛状态。
- Step 与 RuntimeCheckpoint 的正式模型归 `kernel/run`；Orchestrator 只在推进同一 Run 收敛状态机时写入执行步骤和恢复检查点，并由 `kernel/run` 通过 EventHub 提供统一读取合同。
- 不拥有 Workspace/Goal/DoD、Run、Result、Decision、Approval、Memory 等 owner state。
- 不内置软件开发、语言栈、测试框架或具体业务领域策略。

## 内部状态归属结论

| 状态 | 归属结论 | 不拆分原因 |
| --- | --- | --- |
| ToolCall | Orchestrator 内部执行证据 | 仅由 dispatch/finalize 流程创建和推进，生命周期从属于 Step。 |
| Patch | Orchestrator 内部收敛状态 | preview、approval、apply、revert 必须与 contract gate 和 evidence 绑定，不存在独立调用方状态机。 |
| ValidationResult | Orchestrator 内部执行证据 | 由 step result normalization 和 evidence quality gate 产生，只服务于同一 Run 的完成裁决。 |
| EvidenceRecord | Orchestrator 内部证据账本 | 汇聚 runtime、Decision、Approval 证据并负责 coverage/Result 裁决，拆分会分散 convergence transaction。 |
| CoverageWorkItem | Orchestrator 内部 coverage 状态 | 由 definition snapshot 派生，并只由 evidence reconciliation 更新。 |
| ArtifactContinuation | Orchestrator 内部 continuation 状态 | chunk、assemble、apply、retry 与 output mode/repair 状态机保持同一生命周期。 |

上述状态均没有独立 framework 生命周期、独立写入方或可单独启停的业务闭环；外部读取统一通过 `kernel/orchestrator/pkg/events` 的具体查询 command/result。因此不新增 stateful block。

## 主要交互

- 从 `blocks/workspace` 与 `kernel/run` 读取权威快照。
- 向 `blocks/llm` 请求 LLM candidate，向 `blocks/file`、`blocks/git`、`blocks/shell`、`blocks/ssh`、`blocks/mcp`、`blocks/web` 派发 runtime execution request。
- 与 `blocks/result` 创建 accepted/rejected result 与 artifact。
- 与 `blocks/decision`、`blocks/approval`、`kernel/governance` 协作处理治理阻塞与恢复。
- 作为 EvidenceRecord owner，订阅 Decision/Approval 投递的具体 evidence command，完成幂等持久化并同步返回具体 result。
- 向 `kernel/eventstream` 发布 runtime progress、evidence、result finalization 等事件。

## 组件交互合同

- Snapshot contract: 每次 dispatch 必须基于同一组 workspace/run/definition/evidence 权威快照。
- Block request contract: 派发给 blocks 的请求必须是 StagePacket/StepExecutionRequest 派生的结构化请求，并携带 workspace/run/step scope。
- Block response contract: blocks 只返回 normalized result/error/raw ref；orchestrator 负责 evidence quality、coverage binding 和 next action。
- EventHub contract: runtime execute 使用 magicCommon `event.Hub` 直接交互；orchestrator 向 `/runtime/block` 发送 `runtimeevent.ExecuteCommand`，同步接收 `ExecuteCommandResult`，并在 `/runtime/orchestrator` 按需订阅 `ProgressData`、`CompletedData`、`FailedData`。
- Owner command Hub contract: module 只接收 framework `CommandHub`；run/workspace/decision/approval/result/memory repository adapter、runtime adapter 与 typed notification 共用该 Hub，禁止额外注入 projection Hub 或通过 `RawHub()` 反向拆包。
- Type ownership contract: `internal/pkg/runtimeevent` 是七个 runtime block 共用协议 owner；事件顶层禁止使用 `Envelope`、`map[string]any` 或 JSON marshal/unmarshal 转换。`StepExecutionRequest.Input`、result `Output` 与 progress `Data` 仅作为领域动态字段保留。
- Result contract: 只有 accepted evidence 与 DoD coverage 满足后才能调用 `blocks/result` 创建 accepted Result。
- Governance contract: decision/approval 只用于真实治理 gap；resolution/approval resume 必须继承原 stage contract。
- Governance evidence contract: Orchestrator 导入 Decision/Approval 的 `pkg/events`，分别订阅 `/decision/command/record-evidence` 与 `/approval/command/record-evidence`；消费端只访问自己的 EvidenceRecordRepository，不接收对方 repository/service 注入。
- Run notification contract: 生产 EventDrivenDispatch 装配下，Orchestrator 通过 Run 定义的 `RunStateChangedData` 投递 Run created/state-changed，不再构造 Run Envelope；Eventstream 保留事件实际 sourceModule。
- Patch notification contract: previewed/applied/failed/reverted 统一使用 Orchestrator 定义的 `PatchChangedData`；只携带 patch scope、摘要、hash、行数、approval/risk/error 等明确字段，不传输 patch 正文或 map payload。
- Artifact continuation notification contract: created/chunk-recorded/chunk-rejected/assembled/applied/failed 统一使用 `ArtifactContinuationChangedData`；sequence、coverage target 与 evidence refs 使用明确切片类型，投递到 `/orchestrator/consumers/#`。
- Contract violation notification contract: recorded/retry-requested/escalated 使用 `ContractViolationData`；开放诊断与 escalation context 使用 Orchestrator 自有递归 `StructuredPayload`，不使用 map/any、JSON 或 reflect 跨组件传输。

## 已知缺口

- 当前盘点未发现 Orchestrator module 职责缺失；但该模块是 P0 合同集中点，修改时必须补 StagePacket、requiredOutput、evidence gate 和 output mode 回归测试。
- 当前未发现 runtime block 交互合同缺失；后续新增执行通知必须继续由 `internal/pkg/runtimeevent` 定义具体类型，并由消费者显式订阅。
- Event-driven dispatch-step 已通过显式 `CommandHub` 发送具体 `DispatchStepCommand`，kernel/orchestrator 生产代码不再通过 `RawHub()` 反向拆包。
- Decision/Approval EvidenceRecord repository 直连已关闭；上下文采用具体递归值类型传输，不使用 Envelope、map/any、JSON 或 reflect 合同桥。
- Run、Step/LLM、Result/Artifact、Memory、Patch、ArtifactContinuation 与 ContractViolation 的生产 EventDrivenDispatch 通知已使用 typed EventHub。
- Orchestrator `eventSubsystem` 已删除 legacy `events.Hub` 与全部 Envelope fallback；相关测试直接观察 magicCommon EventHub 的具体 data，不再通过 `MemoryHub.Published()` 断言兼容投影。
- Approval created 通知只由 Approval owner 发布，Orchestrator 已删除重复 Envelope；MemoryRecall recorded 通知已迁回 Memory owner 的 upsert command handler 发布。
- Orchestrator module/plugin/testkit 与 biz 构造函数已删除 MagicHub bridge、legacy `events.Hub` dependency、无效非空校验和测试兼容参数；focused tests 不再为无效构造参数创建 `MemoryHub`。
- `stage_changed`、`graph_dispatched`、`repair_started`、`replan_triggered` 未发现生产发布点，其 topic、contract 与版本矩阵声明已删除；不得重新补建无业务来源的通知包装。
- C05 状态边界复核已完成；当前没有需要拆为独立 stateful block 的内部状态。

## 边界约束

- LLM candidate 不能直接拥有状态转移权；必须经过 contract gate。
- output mode、stage、convergence state 与 RunState 必须保持分层，不能相互污染。
- rejected/invalid output 不得产生 accepted evidence 或 Result。
