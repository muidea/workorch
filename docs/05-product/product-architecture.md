# WorkOrch Product Architecture

Status: active

Type: product-architecture

Last Verified: 2026-08-16

## Architecture View

```text
TUI / Web / HTTP client
        |
        v
workorchd application composition and projection
        |
        v  typed EventHub command/result
Workspace owner -> Run owner -> Orchestrator
                                |        |
                                |        +-> Decision / Approval owners
                                +----------> capability owners
                                |
                                v
                         Result -> Workspace -> Run
```

箭头表示 typed command/result 或终态 delivery，不表示跨 owner repository/service 调用。

## Layers And Responsibilities

| Layer | Code | Responsibility | Must not |
| --- | --- | --- | --- |
| Entry/process | `application/workorch`, `internal/services/workorch` | mode、CLI/TUI、local/remote backend、process lifecycle | 拥有业务状态 |
| Runtime initiator | `internal/initiators/workorchruntime` | repository provider/factory | 导出 EventHub、policy 或 bootstrap |
| Application module | `internal/modules/application/workorchd` | HTTP/SSE、typed bootstrap composition、projection | drive Runtime 或创建 successor |
| Stateful owners | Workspace、Run、Result、Decision、Approval、Memory、Skill、Soul、MCP Registry、Eventstream | owner-local model、repository 与 lifecycle | 接受跨 owner direct mutation |
| Kernel modules | Orchestrator、Governance、System | Runtime convergence、governance policy、startup defaults coordination | 接管其他 owner 状态机 |
| Capability blocks | LLM、File、Git、Shell、SSH、Web、MCP | concrete typed operation 与事实回执 | 选择下一步或接纳证据 |
| Shared contracts | `internal/pkg/*contract` | stable typed value/event contracts | 承载 owner service locator |

## Runtime Ownership

| Machine | Sole owner | Product meaning |
| --- | --- | --- |
| SM-1 | Run | pending/active/blocked/terminal lifecycle 与 restart replacement transaction |
| SM-2 | Orchestrator | authority/focus convergence |
| SM-3 | Orchestrator | candidate、feasibility、sealed dispatch transaction |
| SM-4 | Orchestrator | drive/recovery 和 terminal delivery sequencing |

Workspace owner 负责定义与 active Run 绑定；Result owner 负责 formal outcome；Decision/Approval owner 负责各自治理
lifecycle。它们通过 concrete event contract 协作，不共享状态机写权限。

## Primary Flows

### Fresh Execution

1. 用户保存 Workspace、Goal、DoD 和必要资源选择。
2. workorchd 构造 typed `RuntimeBootstrap`，提交 workspace execution `resume` intent。
3. Workspace owner 解析当前定义；Run owner 幂等创建或激活 Run。
4. Orchestrator 从 aggregate drive SM-2/3/4；capability owner 执行 sealed operation。
5. Orchestrator 接纳证据、推进 focus，并密封终态。
6. Result、Workspace、Run 按 owner acknowledgement 顺序完成投递。

### Resume

`resume` 只对当前 Workspace 的 active/recoverable Run 发出继续意图；recovery 从 aggregate 重新 drive，调用方不提供
目标 revision、next action 或 authority patch。

### Restart

`restart` 由 Workspace owner 转交 Run owner。Run owner持久化 predecessor 的 `ReplacementIntent`；待 predecessor
终态 delivery 完整确认后创建 successor。Application 不保存 continuation queue，也不在 projection 中推断 successor。

## Read And Write Boundaries

- write path：入口 → typed command → sole owner → owner-local event/state change。
- read path：owner facts → workorchd projection/SSE → TUI/Web/client。
- Eventstream 是审计日志；projection 是可重建读模型；两者都不是 authority source。
- repository 只由对应 owner 使用；同 owner 内部可以直接组合 Biz/Repo。

组件逐项核对见 [component documentation alignment](../01-overview/component-documentation-alignment.md)。
