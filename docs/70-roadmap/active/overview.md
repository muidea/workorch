# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-07-31

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime V2 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime V2 生产主链 | **V2-only**（`WorkspaceExecutionIntent → owner-local Run fact → DriveRuntimeV2`）；application direct lifecycle、Run source guard、owner outcome forwarding 与 SM-4 producer trust 均为已关闭回归基线。 |
| Runtime V2 receipt/binding 基线 | **已完成基线**；C12 保证 credit 只能从 sealed binding、operation 与 receipt 派生。当前 live 缺口是 activation compiler 将 presentation slice 的多 CWI 错误地与 validation proof 复用。 |
| Runtime V2 单 owner lifecycle、统一 ingress 与 CWI reconciliation | **active**；状态 owner 边界已具备，但 CWI semantic proof 仍无法编译为可执行 bootstrap，且 activation 失败反馈存在错误的 resume 建议。按 [active closure plan](runtime-v2-cwi-reconciliation-single-owner-closure-plan.md) 先收口 CWI 编译/模板 proof，再验收全链路。 |
| 历史 prerequisite / candidate closure records | **archive baseline only**；旧 VPC、freshness、admission、positive-acceptance 计划不得独立定义当前状态或实施顺序 |
| 22-component framework boundary | **已完成并归档**；22 单元合同、deployed restart 与 workspace 删除后的迟到投影验证均已通过。 |
| V1 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Mandatory Work

当前必须关闭的 Runtime V2 项是 **CWI semantic proof 编译、activation outcome 归一与 deployed reconciliation**。
最新 deployed LSP Bridge live 已证明：`MaxWorkItems=4` 的 presentation slice 被错误用于 validation proof，导致
`coverage_binding_unavailable` 并在创建 Run 前停止。不得以放宽 binding、HTTP 202、projection queued/accepted 或
generic validation 替代逐 CWI owner-grounded semantic proof。

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| R3 | single-owner CWI contract + activation + reconciliation | validation/behavior CWI 逐项编译 binding/proof；`/run resume` 复核 current Run，`/run restart` 创建 full-workspace replacement；状态只由 owner internal event 迁移，并对 LSP Bridge 逐 CWI 验收 | [active plan](runtime-v2-cwi-reconciliation-single-owner-closure-plan.md) |
| OPT-1 | file-patch deployed live（可选） | 可补一次 `patch apply + validation + DoD closeout` deployed live；不阻塞 Runtime V2 收口 | [file-patch-side-effect-owner-migration-plan.md](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

延期产品候选（**不占 active**）：LLM context budget 高级项（failover / tokenizer / 原生 usage 精度 / context-too-large 差异化）→ [candidates/next-stage-demand-candidates.md](../candidates/next-stage-demand-candidates.md)。

统一清单：[current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)、[unfinished-todo-index.md](../../01-overview/unfinished-todo-index.md)。

## Active 文档清理规则

1. Runtime V2 的正式约束只来自 `00-core`、Orchestrator readme 与 typed contract；所有 completed record 都仅作
   回归基线，不得从历史记录另行推导状态迁移、lifecycle 或 live 修复队列。
2. current-schema completed Run 必须有 CWI→binding→sealed operation→owner receipt→evidence 链；并且 binding
   必须声明该 CWI 的独立语义 proof。机械执行绿灯、TODO test、泛化 command success、candidate raw CWI、assessment
   claim、`PassedChecks` 或 terminal 都不能替代该链。
3. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
4. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
5. active 不存放过程日记、历史文件清单或 live 原始产物；过程细节只在 archive 检索。
6. 清理失效内容：V1 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
7. 旧 `RUNTIME-SMO-R2` 与 `OPS-1` 均已归 archive；其中 OPS-1 只是假定存在 production maintenance command
   的历史记录，不能作为维护窗口任务。
8. C12 已收口 raw-credit 越权；current R3 未收口的是 profile proof 过粗、slice/binding 编译矛盾、activation feedback
   可执行性与 deployed semantic matrix。不得以 ticket、budget、decoder 放宽或历史绿灯绕过 R3。
