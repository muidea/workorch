# Orchestrator Module

## 功能说明

`kernel/orchestrator` 是 runtime convergence authority，负责阶段状态、NextAction、StagePacket、contract gate、coverage/evidence、repair/replan、runtime block dispatch、Patch、ValidationResult、EvidenceRecord、CoverageWorkItem 与 ArtifactContinuation。它组合多个 owner 和技术 block 完成同一 Run 的收敛闭环。

## 组件交互及合同

- 通过 EventHub 读取 `blocks/workspace` 和 `kernel/run` 的权威快照，并通过 Run owner command 推进 Run/Step/Checkpoint。
- 通过 `internal/pkg/runtimeevent` 定义的 `ExecuteCommand` / `ExecuteCommandResult` 调用 File、Git、LLM、MCP、Shell、SSH 和 Web block；Accepted、Progress、Completed、Failed 通知也由该共享 runtime 协议定义。
- 通过 EventHub 调用 Result、Decision、Approval 和 Memory owner 的具体 command；不得直接持有这些 owner 的 service 或 repository。
- 与 `kernel/governance` 协作处理治理策略、decision/approval blocker 和恢复，但 Orchestrator 保持 runtime contract gate 权威。
- Decision 与 Approval 使用各自 `pkg/events` 定义 evidence command；Orchestrator 幂等写入自己的 EvidenceRecord 并返回具体 result。
- Orchestrator 自有查询、dispatch、Patch、ArtifactContinuation 和 ContractViolation 合同定义在 `kernel/orchestrator/pkg/events`。
- Eventstream 订阅 Orchestrator、Run、Result 及 runtime block 的具体通知形成审计记录。

## 相关约束

- 核心 runtime 必须保持领域中立，不得写死软件开发、语言栈、测试框架、MCP 或业务领域逻辑。
- 每次 dispatch 必须基于一致的 workspace/run/definition/evidence 快照。
- block 输出只是 candidate；evidence quality、coverage binding、next action 和 accepted Result 只能由 Orchestrator 判定。
- ToolCall、Patch、ValidationResult、EvidenceRecord、CoverageWorkItem 和 ArtifactContinuation 只属于 Orchestrator 的单一收敛状态机，不对外暴露写入 repository。
- 跨组件交互只能使用 EventHub 与具体 command/data/result；禁止跨 owner service/repository、Envelope、JSON/reflect 类型桥和通用投递包装。
- provider-facing LLM cache prefix 必须遵守 C0、C1、C2、C3-C5 稳定分层。
