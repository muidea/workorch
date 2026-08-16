# WorkOrch Current Unclosed Matrix

Status: active

Type: current-work-index

Last Verified: 2026-08-16

## Purpose

本文只列真实未关项。已完成的 CWI、single-owner、Orchestrator/Workorchd 内聚、22 组件复杂度和历史 schema
收口已迁入 archive 或 verification contract，不再以阶段编号占用 active 队列。

## Mandatory

| ID | 状态 | 范围 | Exit |
| --- | --- | --- | --- |
| RUNTIME-LIVE | open / deployed-only | fresh development data、真实 provider、Runtime persisted chain | semantic seal、CandidateAttempt、SealedDispatch、owner receipt、binding-matched evidence、existing-target revision-bound replacement、manifest-bound recipe applicability、后续 eligible CWI、authoritative closeout、sealed terminal 与 `Result -> Workspace -> Run` delivery 全部成立 |
| WRE-LIVE | open / deployed-only | Workspace model/reasoning selection | Web/TUI、Resume sealed selection、Restart fresh selection、active configuration restartRequired、双 Workspace 隔离全部成立 |

实施文档：

- [Runtime Positive-Convergence Live Closure](../70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md)
- [Workspace Reasoning Effort Deployed Verification](../70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md)

## Required Regression Baseline

以下均已 code/automated-closed，不是独立未关队列，但 live 验收前必须持续通过：

- SM-1 Run owner、SM-2/3/4 Orchestrator owner、Workspace/Result owner-local terminal transition；
- one `ApplyInternalEvent` mutation entry、one aggregate CAS coordinator、bounded drive/singleflight；
- Definition declaration/generation、eligible CWI closure、semantic order/batch/seal identity；
- single CandidateEvaluator、attempt 0/1、owner feasibility orthogonality；
- File create-only write 与 revision-bound existing-target replacement；
- File manifest-bound Shell recipe applicability、空 recipe semantic filtering 与 exact tree-revision invalidation；
- callable capability 与 validation surface readiness 分离；required-input MCP recipe 保留 typed prerequisite audit、零 endpoint
  side effect，且不进入 provider schema/C3/correction，ready/schema/binding 变化进入 catalog snapshot identity；
- Result closeout 仅保留 current unsatisfied canonical gap，当前 `Facts.Failure` 优先于历史 claimed gap；
- provider ceiling / ExpectedDocument equality、whole-document recovery、C0～C5 stable prefix；
- duplicate/stale/restart/partial-CAS/response-loss/budget/guard；
- Result/Workspace/Run terminal delivery order和 projection non-authority。

完整矩阵见 [Runtime Owner Convergence Verification Matrix](../60-verification/runtime-owner-convergence-matrix.md)。

## Optional And Deferred

| ID | 分类 | 内容 |
| --- | --- | --- |
| OPT-FILE-PATCH | optional | 额外执行一次 deployed `patch/apply + validation + DoD closeout`；不阻塞 RUNTIME-LIVE |
| CAND-LLM-BUDGET | candidate | provider failover、tokenizer 精度、native usage 与 context-too-large 差异化；见 `70-roadmap/candidates/` |

## Closed And Archived

- CWI-SO code closure：`90-archive/completed/runtime-cwi-reconciliation-single-owner-closure-plan.md`
- Orchestrator/Workorchd code runbook：`90-archive/completed/runtime-single-owner-code-closure-runbook.md`
- 22-unit complexity snapshot：`90-archive/records/runtime-22-component-function-complexity-inventory-2026-08-16.md`
- SO-MC code record：`90-archive/records/runtime-single-owner-model-capability-output-code-closure-record-2026-08-16.md`
- WRE code record：`90-archive/records/workspace-reasoning-effort-code-closure-record-2026-08-16.md`
- Runtime contract drift：`90-archive/completed/runtime-contract-drift-closure-plan.md` 与
  `90-archive/records/runtime-contract-drift-code-closure-record-2026-08-16.md`
