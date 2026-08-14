# WorkOrch Docs 未完成待办项索引

Status: active

Type: index

Last Updated: 2026-08-14

## Purpose

Runtime-only 生产主链收口与 **2026-07-16 文档清理** 之后的 **真实未关项** 索引。
已完成切片、过程日记、假活跃 plan、dated live 记录、verification artifacts **不再罗列**（见 `90-archive/`）。

## 阅读导航

1. [A. Runtime 正向收敛](#a-runtime-正向收敛)
2. [C. 可选验证](#c-可选验证)
3. [D. 延期候选](#d-延期候选)
4. [E. 已清理 / 不再跟踪](#e-已清理--不再跟踪)

## A. Runtime 正向收敛

| # | 主题 | 待办 | 计划 |
| --- | --- | --- | --- |
| A1 | Runtime single-owner and model-capability output closure (SO-MC) | SO-MC-12..18 code/automated closure is complete; deploy the target-bound File catalog fix, then execute SO-MC-10 fresh real-provider terminal proof with continued progress beyond an existing-target reopen. | [SO-MC plan](../70-roadmap/active/runtime-single-owner-model-capability-output-closure-plan.md) |
| A2 | Workspace reasoning effort deployed proof | 代码与正式合同已收口；剩余重新部署后的 Web Start/Restart、TUI `/model reasoning`、Resume sealed selection 和双 workspace 隔离验证 | [WRE plan](../70-roadmap/active/workspace-reasoning-effort-single-owner-closure-plan.md) |

The CWI-SO plan is now regression reference for A1 rather than an independent schema-v5 deployed queue.
Predecessor-owned restart、release-before-successor、complete-document recovery、strict decoder、receipt matcher、
owner-local terminal delivery、replay/CAS、CPU guard 与 dual-workspace matrix 继续作为 A1 的代码回归基线，但不能
代替统一 admission matrix。A1 必须组合验证 identity、dependency、target/catalog、kind/variant、proof authority、
binding/fence、retirement/effect、receipt/evidence、restart/resume 与 terminal delivery；本地绿灯不能据此标记为
live closed。

## C. 可选验证

| # | 主题 | 待办 | 历史过程稿 |
| --- | --- | --- | --- |
| C1 | file-patch live | 可选补一次 deployed `patch apply + validation + DoD closeout`；不阻塞 Runtime 收口 | `90-archive/completed/file-patch-side-effect-owner-migration-plan.md` |

## D. 延期候选

| # | 主题 | 说明 |
| --- | --- | --- |
| D1 | LLM context budget 高级项 | failover / tokenizer / 原生 usage 精度 / context-too-large 差异化 → `70-roadmap/candidates/next-stage-demand-candidates.md`（不占 active） |

## E. 已清理 / 不再跟踪

以下类别 **已从 active / 当前验收导航移除**，不在本索引展开：

- 全部 `goal-dod-nextaction-s0`～`s11` 与 iterative plan（completed → archive）
- 22 组件 EventHub 合同归一与全链验证（completed → [archive](../90-archive/completed/component-event-contract-closure-plan.md)）
- 禁止的 `runtime-contract-retry-*` audit / batch / evidence index
- OPS-1：其假定的 Runtime maintenance command 没有代码、typed contract 或 CLI 入口，已由 Runtime-only
  production boundary 取代；历史过程稿只留 archive
- 被 Runtime reducer 吸收的 `convergence-authority-*`、`runtime-*-closure-plan`、`*-nextaction-*` hardening
- 2026-07-16 归档的产品/审计日记：ai-proxy cutover、operation-map、module-inventory、auto-gov、software-dev DoD/SRD、next-mvp、TUI 统一、run-quality、authority-live design、module-block contract validation、deployed blocker 长文、file-patch 长文、llm-context-budget todo、docs reorg plan
- dated live verification 记录与 `60-verification/artifacts/**`（出库；结论在 records markdown）
- dated prerequisite freshness、candidate admission、历史 RUNTIME-SMO、旧 VPC 与 production admissibility completion
  record（仅 archive regression baseline；不定义当前实施顺序）

事实入口：[current-state.md](current-state.md)  
未关矩阵：[current-unclosed-matrix.md](current-unclosed-matrix.md)  
Active 索引：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
