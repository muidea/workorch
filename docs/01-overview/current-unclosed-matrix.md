# WorkOrch Current Unclosed Matrix

Status: active

Type: current-work-index

Last Verified: 2026-08-18

## Purpose

本文只列真实未关项。已完成的 CWI、single-owner、Orchestrator/Workorchd 内聚、22 组件复杂度和历史 schema
收口已迁入 archive 或 verification contract，不再以阶段编号占用 active 队列。

## Mandatory

| ID | 状态 | 范围 | Exit |
| --- | --- | --- | --- |
| WGSC | open / code closure | 大量 Goal/DoD、semantic dependency、CWI scheduling、stable Definition partition | Workspace owner唯一持久化item lifecycle/disposition；Workorchd静态编译graph/gate/partition；SM-2按语义关系而非序号/ID封存schedule、gate witness和ready projection；删除provider全量coverage-order；C1 partition与C5/focus budget分离；1,000-CWI、Resume/Restart和fresh-data live矩阵通过 |
| RUNTIME-LIVE | open / deployed-only | fresh development data、真实 provider、Runtime persisted chain | semantic seal、CandidateAttempt、SealedDispatch、owner receipt、binding-matched evidence、existing-target revision-bound replacement、manifest-bound recipe applicability、后续 eligible CWI、authoritative closeout、sealed terminal 与 `Result -> Workspace -> Run` delivery 全部成立 |
| WRE-LIVE | open / deployed-only | Workspace model/reasoning selection | Web/TUI、Resume sealed selection、Restart fresh selection、active configuration restartRequired、双 Workspace 隔离全部成立 |

实施文档：

- [Workspace Goal/DoD Global Semantic Convergence](../70-roadmap/active/workspace-goal-dod-global-semantic-convergence-closure-plan.md)
- [Runtime Positive-Convergence Live Closure](../70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md)
- [Workspace Reasoning Effort Deployed Verification](../70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md)

## Required Regression Baseline

以下均已 code/automated-closed，不是独立未关队列，但 live 验收前必须持续通过：

- SM-1 Run owner、SM-2/3/4 Orchestrator owner、Workspace/Result owner-local terminal transition；
- one `ApplyInternalEvent` mutation entry、one aggregate CAS coordinator、bounded drive/singleflight；
- Definition declaration/generation、eligible CWI closure、semantic order/batch/seal identity；focus/binding identity 同时绑定
  `manifestHash + logicalRequestID`，历史 reopened/retired binding 不可重新成为 current；
- single CandidateEvaluator、attempt 0/1、owner feasibility orthogonality；
- effect/replay、prospective/existing target policy 与 capability resource prerequisite 只来自 current capability owner
  发布的 typed policy/catalog fact；Runtime core 只做通用 selector/requirement 编译，不按 MCP、Shell、tool/risk 或
  File create strategy 分支；
- File create-only write 与 revision-bound existing-target replacement；
- File manifest-bound Shell recipe applicability、完整 ready/prerequisite-required/unavailable surface、空 confirmed recipe
  semantic filtering 与 exact tree-revision invalidation；
- callable capability、validation surface readiness 与 proof applicability 分离；required-input MCP recipe 保留 typed
  prerequisite audit、零 endpoint side effect，ready auxiliary surface 也不得关闭 CWI；只有 `ready + direct` 进入 provider
  schema/C3/correction，readiness/applicability/schema/binding 变化进入 catalog snapshot identity；
- profile owner-fact postcondition 只由 current capability catalog 满足；Workorchd 只编译、capability owner 只发布事实、
  Orchestrator SM-2 只在其余 proof 完整时判定/有界重开，assessment claim 不得越权关闭；accepted no-progress operation
  shape 跨同一 CWI successor focus 保留，SM-3 在第二次副作用前拒绝重放；
- current-generation bound CWI 可保留在 effect-fenced focus queue，但不得重新进入 planning；duplicate/restart 零 CAS，
  stale manifest/binding/prerequisite 继续 fail-closed；
- Result closeout 仅保留 current unsatisfied canonical gap，当前 `Facts.Failure` 优先于历史 claimed gap；
- provider ceiling / ExpectedDocument equality、whole-document recovery、C0～C5 stable prefix；
- duplicate/stale/restart/partial-CAS/response-loss/budget/guard；
- SM-4 startup inventory 具备 cursor visited-set、page/item hard limit，覆盖 `A → B → A` cycle 与 commands-ready handoff；
- Run/Workspace lifecycle adapter 仅暴露 scoped snapshot `Read`，opaque Runtime aggregate read/CAS 保持独立 Run owner
  合同，Orchestrator 不保留跨 owner Create/Update/Delete/List 表面；
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
