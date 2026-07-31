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
| A1 | CWI reconciliation | **代码与 package/boundary/race 门禁已落地，待 deployed live**：`resume=current Run scope`、`restart=workspace full scope`；observation 作为 SM-2 owner-local input，逐项 retain/reopen CWI | [R3 plan](../70-roadmap/active/runtime-v2-cwi-reconciliation-single-owner-closure-plan.md) |
| A2 | 单 owner lifecycle | **代码与 package/boundary/race 门禁已落地，待 deployed live**：TUI/Web/application 只发 intent；Workspace/Run/Orchestrator/Result 仅由各自 internal event/reducer 迁移 | [R3 plan](../70-roadmap/active/runtime-v2-cwi-reconciliation-single-owner-closure-plan.md) |
| A3 | semantic profile proof | **activation 粒度 guard 已落地，profile/live 仍待验收**：validation proof 一 focus 一 CWI 且每 CWI 声明 artifact/recipe/check/metric；空 TODO、generic test、managed config 不得造成 terminal complete | [R3 plan](../70-roadmap/active/runtime-v2-cwi-reconciliation-single-owner-closure-plan.md) |

SMO-R2-C12 的 immutable binding、strict decoder、receipt matcher、replay/CAS 与 dual-workspace regressions
仍是 A1–A3 的基线，完成记录见
[runtime-v2-single-owner-state-machine-r2-c12-closure-record.md](../90-archive/completed/runtime-v2-single-owner-state-machine-r2-c12-closure-record.md)。

## B. 运行单元架构 / 生命周期

除 A2 外当前没有运行单元架构 / 生命周期收口项。ARCH-1 已完成并归档：workspace delete → late projection event 的
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
