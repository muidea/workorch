# Kernel Module Boundaries

Status: normative

Type: module-architecture

Last Verified: 2026-08-25

## Purpose

本文定义 `kernel/orchestrator`、`kernel/governance` 与 `kernel/system` 三个 Module 的闭环职责和跨 owner
边界。Runtime 状态语义由 [Runtime Stage Contract](../00-core/runtime-stage-contract.md) 定义；组件内部结构、topic、
Command/Result 与私有 mutation route 由各组件 `readme.md` 和源码定义。

| Module | Closed responsibility | Must not |
| --- | --- | --- |
| Orchestrator | SM-2/SM-3/SM-4 Runtime convergence | own SM-1 or another resource lifecycle |
| Governance | governance policy and auto-governance coordination | mutate Decision/Approval/Run/Result state |
| System | startup-default coordination across resource owners | become Template/Soul/MCP Registry owner |

三个 Module 使用同一 EventHub 与 owner-defined concrete contracts。使用 Hub 的逻辑留在各自 `biz/`；`module.go`
只做 framework lifecycle 接线，protocol adapter 不持有 Hub。Module 不取得 sibling owner repository、service、adapter
或可变资源句柄。

## Orchestrator Boundary

Orchestrator 是 SM-2、SM-3、SM-4 的唯一语义 owner，负责：

- authority/focus、semantic reconciliation、binding、evidence、assessment 与 terminal decision；
- request session、candidate evaluation、sealed dispatch 与 operation receipt transaction；
- bounded drive/recovery、governance wait 与 Result→Workspace→Run delivery ledger；其中
  `RuntimeTerminationRecord` 只是一项 SM-4 exact delivery handoff，不是 Run state。

所有外部 typed fact 必须先成为 Orchestrator 私有 internal event，再经唯一 transition coordinator 提交同一 aggregate
mutation。Run owner 只通过 concrete EventHub contract 提供 opaque Runtime record 的 expected-revision CAS；它不解释
Orchestrator aggregate。私有 internal event、reducer family 和 mutation route 不是公共 EventHub envelope。

Orchestrator 不拥有 SM-1、Workspace Definition/active slot、Result lifecycle、Decision/Approval 状态或 capability side
effect。SM-2 选择 canonical convergence state但不执行 operation；SM-3 只执行已经选择并密封的 transaction；SM-4
只从 persisted aggregate 再 drive 和投递，不从 projection/history 重建 authority。

prospective owner-fact probe 也不放宽该边界：SM-2 组合 File-authored hypothetical delta 与 Shell-authored nested
satisfaction observation，required-owner dependency catalog只进入 manifest freshness/CAS/seal，不进入 LLM selectable
surface。typed stale走bounded refresh；`would_not_satisfy`先持久化后由下一次drive形成统一`semantic_contract_failed`
terminal，并保留typed original reason=`prospective_owner_fact_unsatisfied`，禁止provider correction。所有状态变化仍只能由
Orchestrator private internal event提交。

## Governance Boundary

Governance Module 组合 Decision、Approval、Run、Workspace、Memory 与 Orchestrator 的 typed facts，执行治理策略评估、
自动解决、升级和阻塞，并只维护自身 `AutoGovernanceTrace` 协调状态。

- Decision owner 独占 DecisionTicket/Resolution；Approval owner 独占 authorization lifecycle。
- Governance 只能向相应 owner 提交具体 resolve command，不能直接修改 ticket、Approval、Run、Result 或 Runtime aggregate。
- Runtime continuation/terminal 仍由 Orchestrator 消费 owner resolution fact 后裁决；application 和 Governance 都不发送
  隐式 Resume。
- Runtime Definition clarification不接受Governance auto-resolution、普通continue/fail或replan policy；Orchestrator只负责
  dedicated open dispatch，ticket只能由Run owner已持久化的replacement intent经Decision owner专用合同收口。
- 无法满足策略或权限时必须升级或阻塞，不能用 process-local policy state 覆盖 persisted owner facts。

## System Boundary

System Module 在 framework dependencies 就绪后，按确定顺序向 Soul、Workspace Template 与 MCP Registry owner 发送各自
定义的同步 ensure command。它只协调启动，不拥有默认资源内容或 repository。

- 默认资源的 schema、校验、去重和持久化由接收 owner 负责。
- missing/wrong result 或任一 owner failure 必须使启动失败，不能静默跳过或建立 System-owned defaults store。
- ensure 必须 owner-idempotent；重放同一 concrete command 不能覆盖有效用户资源。
- System 不参与运行期治理、Run 调度、Runtime drive 或状态修复。

## Cross-Owner Handoffs

| Interaction | Contract boundary |
| --- | --- |
| Runtime aggregate read/CAS/inventory | Run owner `pkg/events`; payload opaque to Run |
| Workspace snapshot/activation/terminal release | Workspace owner concrete contracts |
| provider ceiling/request result | LLM owner factual Command/Result |
| File/Git/Shell/SSH/Web/MCP execution | corresponding capability-owner Command/Result |
| governance request/resolution | Decision or Approval owner contracts |
| Runtime Definition clarification | dedicated persisted Orchestrator dispatch and Decision/Run replacement-resolution contracts |
| formal terminal persistence | Result owner consumes sealed already-judged material |
| administrative terminal handoff | Orchestrator persists/retries exact `RuntimeTerminationRecord`; Result/Workspace/Run each acknowledge privately; only Run-private SM-1 event changes Run state |
| audit/projection observation | immutable notification only; never mutation authority |

Repeated notification、resolution、drive、ensure 或 delivery acknowledgement 必须保持 owner-local idempotency，不得重复
candidate attempt、side effect、evidence、Result、successor 或默认资源。

## Implementation Sources

- [Orchestrator implementation contract](../../internal/modules/kernel/orchestrator/readme.md)
- [Governance implementation contract](../../internal/modules/kernel/governance/readme.md)
- [System implementation contract](../../internal/modules/kernel/system/readme.md)
- [Block Boundaries](block-boundaries.md)
- [Core Runtime Verification](../00-core/runtime/verification.md)
