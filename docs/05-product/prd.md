# WorkOrch Product Requirements

Status: active

Type: product-requirements

Last Verified: 2026-08-16

## Product Positioning

WorkOrch 是 local-first 的长期任务编排服务。用户为 Workspace 定义 Goal 和 DoD，系统在可审计、可治理、
可恢复的 Runtime 中调用 LLM 与能力，直到形成证据支持的结果、进入明确治理等待，或 fail-closed 终止。

## Required User Outcomes

### Goal And DoD Driven Delivery

- Goal 与 DoD 是 Workspace owner 管理的正式定义，不是临时 prompt 字段。
- 每个 Run 必须绑定不可变的 definition revision 与 typed `RuntimeBootstrap`。
- 完成必须由 accepted evidence、coverage 与 assessment 支持，不能依赖模型自述。
- 正常 Workspace 设置只要求模板、Goal、DoD 和目录；不要求用户编写 raw context/evidence JSON。

### Long-Running And Recoverable Execution

- 一个 Run 可以跨多次 drive、provider call 和 capability operation 持续推进。
- recovery 必须从 Run 中 CAS 持久化的 Runtime aggregate 幂等再 drive。
- stale、duplicate、crash-window 和 exhausted-budget 必须得到确定性处理。
- 多 Workspace 执行必须保留 workspace/run/request/revision scope。

### Deterministic Runtime Authority

- SM-1 只由 Run owner 转换。
- SM-2、SM-3、SM-4 只由 Orchestrator 内部事件转换。
- LLM 只提出 bounded semantic plan、candidate 或 assessment；不能直接改变 authority。
- capability owner 只校验并执行 concrete typed operation；回执不包含下一步决策。
- 未通过合同的模型输出或操作结果不能密封、执行或接纳为正式证据。

### Evidence-Based Result

- state change、validation、governance 与 artifact 必须可追溯到 operation、sealed dispatch 和 revision。
- Orchestrator 独占 evidence acceptance 与终态裁决。
- Result owner 保存 already-judged closeout，不重复判断 DoD。
- Eventstream 和 projection 只用于审计、展示与刷新，不回建 authority。

### Governed Human Interaction

- 缺少外部选择时使用 Decision；风险动作需要授权时使用 Approval。
- Decision/Approval owner 各自独占 ticket/request lifecycle。
- resolution 作为 typed fact 回到 Orchestrator；界面不直接改 Run 或 Runtime state。
- Runtime guard、重复合同失败或预算耗尽必须 fail-closed，不得伪造人工 ticket 来绕过终态。

### Explicit Continue And Restart

- `resume` 只请求继续当前可恢复 Run，不改变 definition，也不接受 execution-context patch。
- `restart` 显式请求按当前 Workspace 定义建立新的 execution lineage。
- restart transaction 必须先在 predecessor 上持久化 Run-owned `ReplacementIntent`，完成
  Orchestrator → Result → Workspace → Run 的终态确认后，才由 Run owner 创建并发布 successor。
- Application、Web 和 TUI 不选择 predecessor、successor 或目标 Run 状态。

### Context, Capability And Provider Governance

- Soul、Skill、MCP、Memory 与 capability catalog 通过 owner projection 和 typed contract 进入 bootstrap/request。
- capability availability 与 operation feasibility 必须在 dispatch 前确认。
- provider-facing cache prefix 保持 C0/C1/C2 稳定，C3-C5 承载易变 Runtime 事实。
- LLM/provider 的 retry、budget 和 reasoning effort 必须由对应 owner 的配置和已封存 activation 约束。

## Product Interfaces

- 默认二进制模式为 TUI；`--server` 启动 HTTP/Web。
- 当前执行写入口是 `POST /api/v1/workorch/workspaces/:workspaceId/execution-activations`，intent 为
  `resume` 或 `restart`。
- 资源管理、governance resolution 和查询使用各自 typed API。
- Web/TUI 必须以服务端 projection 为展示事实，不维护影子业务状态机。

## Acceptance Criteria

发布前至少满足：

- 单 Owner/typed EventHub 边界检查通过；
- Runtime transition、invariant、idempotency、restart、stale/duplicate 与 budget/guard 测试通过；
- current API/TUI 与 projection 合同测试通过；
- file-backed positive convergence 覆盖真实 owner 回执与 crash-window；
- active roadmap 只保留尚未用部署证据关闭的工作。

验证入口见 [verification overview](../60-verification/overview.md)。
