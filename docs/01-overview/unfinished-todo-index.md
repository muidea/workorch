# WorkOrch Docs 未完成待办项索引

Status: active

Type: index

Last Updated: 2026-08-04

## Purpose

Runtime-only 生产主链收口与 **2026-07-16 文档清理** 之后的 **真实未关项** 单点索引。
已完成切片、过程日记、假活跃 plan、dated live 记录、verification artifacts **不再罗列**（见 `90-archive/`）。

## 阅读导航

1. [A. Runtime 正向收敛](#a-runtime-正向收敛)
2. [B. 运行单元架构 / EventHub 合同](#b-运行单元架构--eventhub-合同)
3. [C. 可选验证](#c-可选验证)
4. [D. 延期候选](#d-延期候选)
5. [E. 已清理 / 不再跟踪](#e-已清理--不再跟踪)

## A. Runtime 正向收敛

| # | 主题 | 待办 | 计划 |
| --- | --- | --- | --- |
| A1 | LSP Bridge deployed proof | **唯一 live gap**：在 ready provider 的 fresh deployment 中创建临时 LSP Bridge workspace/template selection，逐 CWI 验证 semantic proposal → owner feasibility → sealed binding → bindingRef operation → owner receipt → evidence/assessment → terminal；不得写入内置 defaults | [R3 plan](../70-roadmap/active/runtime-cwi-reconciliation-single-owner-closure-plan.md) |

Immutable binding、strict decoder、receipt matcher、replay/CAS、empty/resume/restart/drift/crash、CPU guard 与
dual-workspace deployed matrix 已闭环，继续作为 A1 的代码和 live 回归基线。

## B. 运行单元架构 / EventHub 合同

| # | 主题 | 待办 | 计划 |
| --- | --- | --- | --- |
| B1 | 22 组件 EventHub 合同归一 | 22/22 代码收口完成；307-topic 基线累计删除 41 个无闭环/错误 write topic，当前 266 个；MCP Registry owner identity 已与 MCP capability 分离，audit payload/version 已归还 Eventstream，shared Events 只保留 lane helper；其余 alias、跨 owner DTO、Post Result 依赖与递归 resource 图均由门禁清零，剩余 race、console、managed deployed 与 fresh 双 workspace live 验证 | [EC plan](../70-roadmap/active/component-event-contract-closure-plan.md) |

ARCH-1 的 1/4/17 分类、Base Biz 和 lifecycle 已完成并归档；B1 不重新打开组件分类，也不得修改 Runtime 状态机 owner。

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
