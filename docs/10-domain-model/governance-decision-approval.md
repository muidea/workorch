# Governance Decision / Approval Domain Model

Status: active

Type: domain-model

Last Verified: 2026-08-16

## Ownership

- `blocks/decision` 独占 Ticket、Resolution 与 durable Runtime governance proposal lifecycle。
- `blocks/approval` 独占 Approval lifecycle。
- `kernel/governance` 计算自动治理策略，不写上述 owner 状态。
- `kernel/orchestrator` 独占 Runtime governance binding、resolution acceptance 与 continuation/failure 判断。
- Run owner只消费 Orchestrator发布的 immutable blocker/continuation/terminal fact 来转换 SM-1。

## Decision Ticket

Ticket 用于确实缺少外部选择或信息的 `need_governance`。主要字段为 identity/scope、reason/kind/question、typed
context、state、auto-governance audit、failure metadata、allowed recovery actions、Runtime proposal 与时间/actor。

States：`open`、`resolved`、`superseded`。

Runtime guard、重复合同失败、repair/provider budget 耗尽或 invariant violation 是系统终止原因，不应为了延长运行
而创建人工 Ticket。

## Resolution

Decision 的通用模型支持 `provide_missing_info`、`reject_current_path` 和 `replan` resolution type，并保留 payload、
continuation action、resume/successor references。对现行 Runtime governance adapter：

- ticket只公开该次请求允许的 action；当前主链映射为提供信息后 `continue` 或拒绝当前路径后 `fail`；
- resolution 必须绑定 ticket、run、authority snapshot、governed dispatch 和 dependency；
- Decision owner先持久化 proposal，Orchestrator提交 matching resolution 后，Decision owner才形成正式 resolution；
- stale 或 duplicate resolution 不能改变新 authority。

通用 `replan` 字段不授权 application 创建 successor。重新按新 definition 执行必须走独立的 Workspace `restart`
intent 与 Run-owned `ReplacementIntent` transaction。

## Approval

Approval 表达 capability operation 的风险授权。主要字段为 identity/scope、reason/action、typed context、state、
auto-governance audit 与时间/actor。

States：`pending`、`approved`、`rejected`、`superseded`。

批准只解除 matching governed operation 的 gate；它不密封 dispatch、不执行副作用，也不接纳 operation evidence。

## Runtime Flow

```text
Orchestrator seals governed dependency
  -> Decision/Approval owner opens owner-local object
  -> Run owner observes immutable blocker fact
  -> operator or governance policy submits resolution
  -> owner persists transition/proposal
  -> Orchestrator validates binding and reduces
  -> Run owner observes continuation or terminal fact
```

Web/TUI/workorchd 只提交 typed resolution 并展示 projection；resolve 后不得额外自动 Resume。

## Invariants

- ticket/approval identity、Workspace、Run、dispatch 与 authority snapshot 必须一致。
- resolution payload 不能携带 authority patch 或目标 Run state。
- auto-governance 与人工治理走相同 owner transition 和审计路径。
- application、projection、Eventstream 和 capability owner 均不能关闭或重开治理对象。
