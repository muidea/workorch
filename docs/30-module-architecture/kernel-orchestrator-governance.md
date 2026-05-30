# Kernel Orchestrator / Governance Architecture

Status: active

Type: module-architecture

Last Updated: 2026-05-30

## Purpose

本文件说明 orchestrator 和 governance 相关 kernel 模块的代码责任和边界。

## Main Modules

### `kernel/orchestrator`

职责：

- Run lifecycle coordination
- Step dispatch
- graph follow-up / completion follow-up
- runtime repair / replan dispatch
- context gate recovery routing
- evidence mapping
- completion gate

### `kernel/decision`

职责：

- Decision Ticket lifecycle
- Resolution persistence and retrieval

更细实现边界：

- 只承接结构化 Decision Ticket 与 Resolution 本体
- 只负责恢复动作选择结果的持久化与读取
- 不直接决定 Web/TUI 如何交互展示

### `kernel/governance`

职责：

- governance-oriented integration logic
- auto-governance / ticket semantics 的承接点

更细实现边界：

- duplicate failure / repair exhausted / policy gate 的治理升级判断
- auto-governance 对结构化恢复动作的自动选择
- decision / approval / recovery path 的中立协同

它不替代：

- `kernel/run` 的生命周期本体
- `kernel/result` 的 accepted / rejected closeout
- product 层的 queue / drawer / form 交互

### Approval Boundary

approval 的 kernel 责任应保持为：

- pending / approved / rejected 的正式状态转换
- 与 run / step / action 的结构化关联
- 恢复链中的“允许 / 不允许继续”判断输入

approval 不应承担：

- Web/TUI 的按钮语义
- comment / drawer / batch form 的产品交互细节
- accepted business outcome 的最终裁决

## Boundary

这些模块负责“如何治理和推进状态”，但不直接负责：

- provider-side request rendering
- MCP 协议调用
- shell/file/git/web/ssh 外部动作

## Flow Participation

主要参与：

- planning / execution dispatch
- governance block / resume
- repair / replan
- completion gate / acceptance transition

## Responsibility Split Summary

| 关注点 | 主要 kernel 责任 |
| --- | --- |
| step dispatch / follow-up / completion follow-up | `kernel/orchestrator` |
| context gate fail-fast / read-required / blocker-required / replan-required routing | `kernel/orchestrator` |
| decision ticket / resolution object | `kernel/decision` |
| duplicate failure / repair exhausted / auto-governance 协同 | `kernel/governance` |
| approval formal state / recovery allow-deny input | governance-aligned kernel responsibility |

这些责任都服务恢复链，但不能混写成产品交互面。

## Source Documents

- `architecture-overview.md`
- `../20-runtime-flow/repair-replan-flow.md`
- `../60-verification/audits/full-chain-assessment-closure-plan.md`
- `../70-roadmap/active/run-closure-quality-improvement-plan.md`
