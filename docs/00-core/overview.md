# WorkOrch Core Contracts

Status: normative

Type: core-contract-index

Last Verified: 2026-08-18

## Purpose

本目录只保存跨产品入口和 profile 都必须成立的最高层合同。Core 保持业务中立，不写软件开发、具体工具、页面或部署策略。

## Reading Order

1. [Unified Runtime Contract](runtime-stage-contract.md)
2. 从 [Runtime Semantic Topics](runtime/overview.md) 进入对应单专题合同
3. Goal/DoD 或调度变更再读 [Definition Convergence](definition-convergence/overview.md)
4. LLM request/cache 变更再读 [Runtime Request Cache](runtime-request-cache-note.md)；跨 provider/capability 的长期执行预算读
   [Execution Budget](runtime/execution-budget.md)
5. 按数据流阅读 [Runtime Flow](runtime/flows/overview.md)，按变更范围进入对应 verification
6. 最后对照 Orchestrator README 与 `internal/pkg/workorchruntimecontract`

## Directory Boundary

- `runtime/`：Owner、收敛、candidate transaction、四状态机、execution budget、执行 flow、全局门禁与 core verification。
- `definition-convergence/`：Workspace Definition admission、all-of 图、semantic scheduling、clarification、activation flow 与验证。
- `runtime-request-cache-note.md` 及相邻 request 文档：provider-facing transaction、context assembly 与 cache partition。

产品需求、组件落点、UI、deployed service 编排、具体命令清单和实施计划不进入本目录。
