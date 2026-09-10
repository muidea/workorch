# Governance Decision / Approval Domain Model

Status: normative

Type: domain-model

Last Verified: 2026-08-21

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

`recoverable_runtime_wait`表示无需人类业务选择的typed恢复条件，不创建Decision/Approval，也不转换SM-1 blocker。
若所谓恢复实际缺少condition、trigger/deadline或budget，必须fail-closed，不能降级为人工ticket。

## Resolution

Decision 的通用模型支持 `provide_missing_info`、`reject_current_path` 和 `replan` resolution type，并保留 payload、
continuation action、resume/successor references。对现行 Runtime governance adapter：

- ticket只公开该次请求允许的 action；当前主链映射为提供信息后 `continue` 或拒绝当前路径后 `fail`；
- resolution 必须绑定 ticket、run、authority snapshot、governed dispatch 和 dependency；
- Decision owner先持久化 proposal，Orchestrator提交 matching resolution 后，Decision owner才形成正式 resolution；
- stale 或 duplicate resolution 不能改变新 authority。

通用 `replan` 字段不授权 application 创建 successor。重新按新 definition 执行必须走独立的 Workspace `restart`
intent 与 Run-owned `ReplacementIntent` transaction。

验证输入一致性评审的完整 `inconclusive` 报告使用 `runtime_governance` ticket（failure class
`validation_input_review`、requested action `provide_validation_input_facts`），不是 Definition clarification。
Orchestrator 绑定 immutable Definition/startup、原 required checks、exact 输入内容、report 和 request；Decision 独占票据状态。
`provide_missing_info` 仅提交非空且至多 2048 bytes 的完整 UTF-8 事实摘要；不能修改原准则、审批冲突或授予 credit。
matching resolution 接纳后，原报告 append-only 保留，新 semantic manifest/request 对同一内容重新评审。
同一 Run/exact subject 最多接受一次信息补充；仍无法判断明确失败，不因预算耗尽再开票。
只有完整业务不确定性可以进入这条路径，空/截断/格式/身份错误继续使用既有请求恢复和 correction budget。

Definition clarification使用受限的Decision ticket class，不继承上述通用resolution行为：

- pre-seal ticket只能由corrected all-of candidate的admission/promotion事实驱动
  `ResolveWorkspaceDefinitionClarificationByAdmissionCommand/Result`，再由Workspace owner acknowledgement释放activation；
- Run-scoped ticket的required action固定为`revise_definition_and_restart`，不接受人工/自动`continue|fail|replan`。只有Run
  owner已持久化matching replacement intent后，才能提交
  `ResolveRuntimeDefinitionClarificationByReplacementCommand/Result`；该resolution不产生Runtime continuation fact。

客户端与Governance module均不能代发这些owner-to-owner command。完整scope、binding与crash-window合同见
[Workspace Definition Clarification](../00-core/definition-convergence/clarification.md)和
[Runtime Definition Clarification](../00-core/definition-convergence/runtime-clarification.md)。

## Approval

Approval 表达 capability operation 的风险授权。主要字段为 identity/scope、reason/action、typed context、state、
auto-governance audit 与时间/actor。

States：`pending`、`approved`、`rejected`、`superseded`。

批准只解除 matching governed operation 的 gate；它不密封 dispatch、不执行副作用，也不接纳 operation evidence。
authorization target在创建后不可被resolution payload覆盖；Runtime Approval只接受Orchestrator producer创建和提交typed
principal resolution，workspace/run/operation/fence/grant任一漂移都不能释放当前gate。

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
