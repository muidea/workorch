# WorkOrch Docs 未完成待办项索引

Status: active

Type: index

Last Updated: 2026-07-31

## Purpose

Runtime V2-only 生产主链收口与 **2026-07-16 文档清理** 之后的 **真实未关项** 单点索引。
已完成切片、过程日记、假活跃 plan、dated live 记录、verification artifacts **不再罗列**（见 `90-archive/`）。

## 阅读导航

1. [A. Runtime V2 正向收敛](#a-runtime-v2-正向收敛)
2. [B. 运行单元架构 / 生命周期](#b-运行单元架构--生命周期)
3. [C. 可选验证](#c-可选验证)
4. [D. 延期候选](#d-延期候选)
5. [E. 已清理 / 不再跟踪](#e-已清理--不再跟踪)

## A. Runtime V2 正向收敛

| # | 主题 | 待办 | 计划 |
| --- | --- | --- | --- |
| A1 | CWI compiler 与 reconciliation | **当前 code gap**：presentation slice 的多 item 不得复用 validation proof；validation/behavior CWI 必须单 focus、单 binding，并由 `resume=current Run`、`restart=workspace full` reconciliation 逐项 retain/reopen | [R3 plan](../70-roadmap/active/runtime-v2-cwi-reconciliation-single-owner-closure-plan.md) |
| A2 | activation outcome | **当前 code gap**：bootstrap preflight 必须完整、稳定地报告所有缺 proof CWI；无 active Run 的 `coverage_binding_unavailable` 不能推荐 resume，也不能显示 execution started | [R3 plan](../70-roadmap/active/runtime-v2-cwi-reconciliation-single-owner-closure-plan.md) |
| A3 | semantic profile proof | **profile/live gap**：每个可关闭 CWI 声明 owner-grounded artifact/target、operation/effect、recipe/check/metric；空 TODO、generic test、managed config 不得造成 terminal complete | [R3 plan](../70-roadmap/active/runtime-v2-cwi-reconciliation-single-owner-closure-plan.md) |

SMO-R2-C12 的 immutable binding、strict decoder、receipt matcher、replay/CAS 与 dual-workspace regressions
仍是 A1–A3 的基线，完成记录见
[runtime-v2-single-owner-state-machine-r2-c12-closure-record.md](../90-archive/completed/runtime-v2-single-owner-state-machine-r2-c12-closure-record.md)。

## B. 运行单元架构 / 生命周期

当前没有单 owner lifecycle 边界收口项；其 source/scope/owner-private reducer 仅作 A1-A3 的回归门禁。ARCH-1 已完成并归档：workspace delete → late projection event 的
managed deployed matrix 已通过，见
[完成矩阵](../90-archive/completed/component-contract-and-regression-closure-matrix.md)。

## C. 可选验证

| # | 主题 | 待办 | 历史过程稿 |
| --- | --- | --- | --- |
| C1 | file-patch live | 可选补一次 deployed `patch apply + validation + DoD closeout`；不阻塞 Runtime V2 收口 | `90-archive/completed/file-patch-side-effect-owner-migration-plan.md` |

## D. 延期候选

| # | 主题 | 说明 |
| --- | --- | --- |
| D1 | LLM context budget 高级项 | failover / tokenizer / 原生 usage 精度 / context-too-large 差异化 → `70-roadmap/candidates/next-stage-demand-candidates.md`（不占 active） |

## E. 已清理 / 不再跟踪

以下类别 **已从 active / 当前验收导航移除**，不在本索引展开：

- 全部 `goal-dod-nextaction-s0`～`s11` 与 iterative plan（completed → archive）
- V1 `runtime-contract-retry-*` audit / batch / evidence index
- OPS-1：其假定的 Runtime V1 maintenance command 没有代码、typed contract 或 CLI 入口，已由 V2-only
  production boundary 取代；历史过程稿只留 archive
- 被 Runtime V2 reducer 吸收的 `convergence-authority-*`、`runtime-*-closure-plan`、`*-nextaction-*` hardening
- 2026-07-16 归档的产品/审计日记：ai-proxy cutover、operation-map、module-inventory、auto-gov、software-dev DoD/SRD、next-mvp、TUI 统一、run-quality、authority-live design、module-block contract validation、deployed blocker 长文、file-patch 长文、llm-context-budget todo、docs reorg plan
- dated live verification 记录与 `60-verification/artifacts/**`（出库；结论在 records markdown）
- dated prerequisite freshness、candidate admission、历史 RUNTIME-SMO、旧 VPC 与 production admissibility completion
  record（仅 archive regression baseline；不定义当前实施顺序）

事实入口：[current-state.md](current-state.md)  
未关矩阵：[current-unclosed-matrix.md](current-unclosed-matrix.md)  
Active 索引：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
