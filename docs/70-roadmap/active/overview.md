# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-07-31

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime 生产主链 | **Runtime-only**（`WorkspaceExecutionIntent → owner-local Run fact → DriveRuntime`）；application direct lifecycle、Run source guard、owner outcome forwarding 与 SM-4 producer trust 均为已关闭回归基线。 |
| Runtime receipt/binding 基线 | **已完成基线**；presentation slice 与 semantic focus 已分离，C12 保证 credit 只能从 sealed binding、operation 与 receipt 派生。 |
| Runtime 单 owner lifecycle、semantic contract 自动编译与 CWI reconciliation | **active（仅 R3-E3 live）**；Goal/DoD + selected template/profile + owner facts 已能自动形成 semantic proposal、owner feasibility 与 SM-2 sealed binding；普通 UI raw contract JSON 已删除，empty/resume/restart/drift/crash/concurrency/CPU deployed matrix 已闭环。剩余 fresh LSP Bridge 逐 CWI验收见 [active closure plan](runtime-cwi-reconciliation-single-owner-closure-plan.md)。 |
| 历史 prerequisite / candidate closure records | **archive baseline only**；旧 VPC、freshness、admission、positive-acceptance 计划不得独立定义当前状态或实施顺序 |
| 22-component framework boundary | **已完成并归档**；22 单元合同、deployed restart 与 workspace 删除后的迟到投影验证均已通过。 |
| 禁止的 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Mandatory Work

当前必须关闭的 Runtime 项只剩 **fresh LSP Bridge deployed semantic matrix**。CWI semantic contract 自动编译、
deployed reconciliation、空 workspace prospective target、resume/restart/drift/crash/concurrency 与 CPU guard 已闭环。
LSP live 必须继续由受约束 semantic-plan document + owner feasibility 形成 proposal，并仅由 SM-2 seal；不得要求普通用户
填写 target/recipe/check JSON，也不得以放宽 binding、HTTP 202、projection queued/accepted 或 generic validation 替代
逐 CWI owner-grounded proof。

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| R3 | single-owner semantic contract + CWI reconciliation | minimal bootstrap；SM-3 受控 semantic-plan document；owner feasibility；SM-2 seal；`resume=current_run`、`restart=workspace_full`；移除普通用户 raw contract JSON，并对 LSP Bridge 逐 CWI验收 | [active plan](runtime-cwi-reconciliation-single-owner-closure-plan.md) |
| OPT-1 | file-patch deployed live（可选） | 可补一次 `patch apply + validation + DoD closeout` deployed live；不阻塞 Runtime 收口 | [file-patch-side-effect-owner-migration-plan.md](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

延期产品候选（**不占 active**）：LLM context budget 高级项（failover / tokenizer / 原生 usage 精度 / context-too-large 差异化）→ [candidates/next-stage-demand-candidates.md](../candidates/next-stage-demand-candidates.md)。

统一清单：[current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)、[unfinished-todo-index.md](../../01-overview/unfinished-todo-index.md)。

## Active 文档清理规则

1. Runtime 的正式约束只来自 `00-core`、Orchestrator readme 与 typed contract；所有 completed record 都仅作
   回归基线，不得从历史记录另行推导状态迁移、lifecycle 或 live 修复队列。
2. current-schema completed Run 必须有 CWI→binding→sealed operation→owner receipt→evidence 链；并且 binding
   必须声明该 CWI 的独立语义 proof。机械执行绿灯、TODO test、泛化 command success、candidate raw CWI、assessment
   claim、`PassedChecks` 或 terminal 都不能替代该链。
3. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
4. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
5. active 不存放过程日记、历史文件清单或 live 原始产物；过程细节只在 archive 检索。
6. 清理失效内容：禁止的 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
7. 旧 `RUNTIME-SMO-R2` 与 `OPS-1` 均已归 archive；其中 OPS-1 只是假定存在 production maintenance command
   的历史记录，不能作为维护窗口任务。
8. C12、slice/focus 分离、activation recommendation、semantic contract 自动编译、普通用户 raw contract JSON 删除及
   R3-E2 deployed matrix 已收口；current R3 只剩 fresh LSP Bridge 逐 CWI semantic matrix。
   不得以 ticket、手工 overlay、budget/decoder 放宽或历史绿灯绕过 R3-E3。
