# WorkOrch Docs 未完成待办项索引

Status: active

Type: index

Last Updated: 2026-07-28

## Purpose

Runtime V2-only 生产主链收口与 **2026-07-16 文档清理** 之后的 **真实未关项** 单点索引。
已完成切片、过程日记、假活跃 plan、dated live 记录、verification artifacts **不再罗列**（见 `90-archive/`）。

## 阅读导航

1. [A. Runtime V2 正向收敛](#a-runtime-v2-正向收敛)
2. [B. 运行单元架构 / 生命周期](#b-运行单元架构--生命周期)
3. [C. 运维切换](#c-运维切换)
4. [D. 可选验证](#d-可选验证)
5. [E. 延期候选](#e-延期候选)
6. [F. 文档维护](#f-文档维护)
7. [G. 已清理 / 不再跟踪](#g-已清理--不再跟踪)

## A. Runtime V2 正向收敛

| # | 主题 | 待办 | 当前方案 |
| --- | --- | --- | --- |
| A1 | Runtime V2 单 owner convergence 收口 | 依 SMO-1～SMO-7 串行处理：先分离 candidate rejection 与 owner feasibility，再收敛唯一 transition coordinator、feasibility 后继、空 workspace blueprint、capability taxonomy、boundary gate，最后 fresh live。历史 C7/C8 仅作回归基线；不加入 compatibility fallback 或 live 热修跳卡。 | [唯一收口方案](../70-roadmap/active/runtime-v2-single-owner-state-machine-closure-plan.md) |

## B. 运行单元架构 / 生命周期

| # | 主题 | 待办 | 当前方案 |
| --- | --- | --- | --- |
| B1 | 22 组件 boundary | 按组件矩阵逐项收口状态权威、typed I/O、禁止边界和独立回归；最终执行 SM-4 deployed matrix | [组件矩阵](../70-roadmap/active/component-contract-and-regression-closure-matrix.md) |

## C. 运维切换

| # | 主题 | 待办 | 历史过程稿 |
| --- | --- | --- | --- |
| C1 | 生产 apply | clean build + 生产目录维护窗口正式 `runtime-v1-retirement --apply` | `90-archive/completed/deployed-workspace-progress-blocker-component-closure-plan.md` |

## D. 可选验证

| # | 主题 | 待办 | 历史过程稿 |
| --- | --- | --- | --- |
| D1 | file-patch live | 可选补一次 deployed `patch apply + validation + DoD closeout` | `90-archive/completed/file-patch-side-effect-owner-migration-plan.md` |

## E. 延期候选

| # | 主题 | 说明 |
| --- | --- | --- |
| E1 | LLM context budget 高级项 | failover / tokenizer / 原生 usage 精度 / context-too-large 差异化 → `70-roadmap/candidates/next-stage-demand-candidates.md`（不占 active） |

## F. 文档维护

| # | 主题 | 说明 |
| --- | --- | --- |
| F1 | 权威层术语抽查 | 确认 StagePacket/NextAction 等不再被写成生产主链 |
| F2 | block readme 措辞 | 可选；实现 SoT 仍是 `kernel/orchestrator/readme.md` |
| F3 | archive 索引 | 保持 `90-archive/*/overview.md` 可检索 |

## G. 已清理 / 不再跟踪

以下类别 **已从 active / 当前验收导航移除**，不在本索引展开：

- 全部 `goal-dod-nextaction-s0`～`s11` 与 iterative plan（completed → archive）
- V1 `runtime-contract-retry-*` audit / batch / evidence index
- 被 Runtime V2 reducer 吸收的 `convergence-authority-*`、`runtime-*-closure-plan`、`*-nextaction-*` hardening
- 2026-07-16 归档的产品/审计日记：ai-proxy cutover、operation-map、module-inventory、auto-gov、software-dev DoD/SRD、next-mvp、TUI 统一、run-quality、authority-live design、module-block contract validation、deployed blocker 长文、file-patch 长文、llm-context-budget todo、docs reorg plan
- dated live verification 记录与 `60-verification/artifacts/**`（出库；结论在 records markdown）
- dated prerequisite freshness、candidate admission、旧 VPC 与 production admissibility completion record（仅
archive regression baseline；当前实施顺序统一由 RUNTIME-SMO SMO-1～SMO-7 定义）

事实入口：[current-state.md](current-state.md)  
未关矩阵：[current-unclosed-matrix.md](current-unclosed-matrix.md)  
Active 索引：[../70-roadmap/active/overview.md](../70-roadmap/active/overview.md)
