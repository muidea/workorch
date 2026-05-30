# Coverage Work Item Closure History

Status: active

Type: archive

Last Updated: 2026-05-30

## Purpose

本文件承接 `coverageWorkItems` / `coverageWorkItemContext` 的阶段性收口记录，
用于追溯实现与验证分批落地过程。

它不再作为当前稳定 core contract、acceptance flow 或 read-model 的权威入口。

当前稳定定义入口：

- `../../20-runtime-flow/verification-acceptance-flow.md`
- `../../20-runtime-flow/evidence-result-flow.md`
- `../../30-module-architecture/storage-readmodel.md`
- `../../60-verification/core-runtime-verifier.md`

## Closure History

- 2026-05-27
  - CWI-02 第一轮收口：runtime 新增 run-scoped `coverageWorkItems` /
    `coverageWorkItemContext` 账本投影，来源限定为当前 effective Goal leaf、DoD item、
    profile required acceptance dimension、profile required deliverable expectation 与可回链的 failed coverage item；
    该投影只写入 run execution context，不写回正式 Goal / DoD。
  - CWI-02 read model 收口：`RunSnapshot` 显式投影 `coverageWorkItems` 与
    `coverageWorkItemContext`，Web/TUI/API 只能读取该投影，不能反向覆盖 run ledger
    或 formal Goal / DoD。
  - CWI-03 第一轮收口：LLM provider context budget 将
    `coverageWorkItemContext` 作为动态上下文保护；存在 unresolved work item
    但缺少 actionable `currentFocus`，或 `currentFocus` 缺少最小字段时，触发
    context gate fail-fast；prompt 渲染将其放在 `C4.coverage-work-items`，
    不污染 C0/C1/C2 稳定缓存前缀。
  - CWI-04 第一轮收口：strict repair / verification / coverage repair 路径可以从
    work item 的 `missingEvidenceKinds` 机械映射到 `need_evidence`、
    `need_state_change`、`need_validation`、`need_assessment` 或
    `need_governance`。普通 activation / planning / replan continuation 初始阶段
    只投影上下文，不允许 work item 抢占原 stage contract。
  - CWI-05 第一轮收口：`coverageWorkItemContext` 会随 run execution context、
    step input、execution context package 与 repair action package 继承；
    wrapper failure / provider retry / schema repair 不应清空 unresolved work item scope。
    多 item 场景当前以稳定 `unresolvedItems` 摘要和单一 `currentFocus`
    驱动本轮收敛，共享 evidence 的逐项 alignment 仍由后续 evidence quality /
    assessment gate 执行。
  - CWI-06 首批回归：baseline 生成不改写 Goal/DoD、work item state-change
    缺口在缺少 target evidence 时进入 `need_evidence`、provider budget 对缺失
    current focus 的 unresolved ledger fail-fast；同时回归 kernel orchestrator、
    LLM budget/prompt 与 appservice read model，确认普通 activation/planning
    不被 CWI 改写为 validation。
  - CWI-03/CWI-05 第二轮收口：`coverageWorkItems` 的缺口计算改为按
    `requiredEvidenceKinds` 逐类判断，新增 `acceptedEvidenceByKind` /
    `rejectedEvidenceByKind` 投影，避免一个 evidence ref 批量关闭
    `state_change` 与 `validation` 等不同证据要求。
  - CWI-03 第二轮收口：`coverageWorkItemContext` 增加 `currentFocusItems`、
    `ledgerSummary`、`omittedCount`、omitted status/source-scope distribution
    与 compact evidence refs；provider context gate 校验 focus 的
    `definitionRevision`、source ref、evidence refs 与 next allowed convergence states。
  - CWI-02 第二轮收口：容器型 source 进入 `excludedSources` 并记录
    `excludedReason=container_source`；旧 definition revision 的 item 进入
    `historicalItems` / `historicalCount`，不再静默丢失，也不混入当前 `currentFocus`。
  - CWI-06 第二轮回归：补充 evidence-kind 独立缺口、旧 revision historical 隔离、
    top-k focus / ledgerSummary 投影、provider 对完整 `currentFocusItems` 的 gate 校验。
  - CWI-05 第三轮收口：execution / verification / acceptance 的最小 evidence 回写链路已接入。
    完成的 state-changing / validation step 只有在显式声明 `coverageTargets` 或
    `workItemIds` 时才写入对应 work item 的 accepted evidence；失败 step 写入
    rejected evidence 与 `currentGap`；validated `dodAssessment` satisfied item
    可把已满足全部 required evidence kind 的 work item 推进到 `satisfied`。
  - CWI-06 第三轮回归：补充 completed step 推进 `state_change`、assessment 推进
    `validation/satisfied`、failed step 写 rejected evidence、failed assessment item
    写 rejected validation evidence 的中立回归。
  - CWI-05 第四轮收口：accepted result 写入前新增 `coverageWorkItems`
    acceptance gate，只有所有 required work item 均 `satisfied` / `blocked` /
    `skipped` 且 satisfied item 带 accepted evidence refs 时才允许完成；
    `evidence_ready` 仍需 assessment 显式关闭。
  - CWI-05 第四轮收口：validated assessment 中的 satisfied claim 若无法回链到
    active work item，会作为当前 focus 的 rejected validation evidence 记录，
    并阻止 accepted result，避免自然语言 satisfied claim 绕过 coverage ledger。
  - CWI-06 第四轮回归：补充 acceptance gate 拒绝 unresolved work item、
    允许 satisfied evidence、拒绝未回链 satisfied assessment 的测试。
  - CWI-02/CWI-05 第五轮收口：replan successor 创建时会先按当前 effective definition
    baseline 重新归一化 `coverageWorkItems`，同 revision 且 source 仍存在的 item
    保留 accepted/rejected evidence lineage；已移除 source 或旧 revision item
    进入 `historicalItems`，不得继续成为 successor 的 `currentFocus`。
  - CWI-06 第五轮回归：补充 replan successor 保留当前 revision evidence lineage、
    隔离 stale source 并保留既有 historical ledger 的测试，确认 replan / successor
    路径不混入过期 coverage focus。

## Archive Boundary

本文件只承接：

- 分批收口时间线
- 阶段性实现与回归摘要
- 历史 closure 解释

本文件不承接：

- 当前稳定 coverage ledger 规则定义
- 当前稳定 acceptance gate
- 当前稳定 read-model / request audit 契约
