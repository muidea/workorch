# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-07-30

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime V2 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime V2 生产主链 | **V2-only**（Start/Resume → `DriveRuntimeV2` only）；状态 owner/recovery 仍按 RUNTIME-SMO-R2 收口 |
| Runtime V2 正向收敛、状态机 owner 与合同归一 | **重新打开**；current-schema recovery、CAS retry 和 projection 出现新的单 owner 反例，按 `RUNTIME-SMO-R2` 收口。 |
| 历史 prerequisite / candidate closure records | **archive baseline only**；旧 VPC、freshness、admission、positive-acceptance 计划不得独立定义当前状态或实施顺序 |
| 22-component framework boundary | **已完成并归档**；22 单元合同、deployed restart 与 workspace 删除后的迟到投影验证均已通过。 |
| V1 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Open

| ID | 项 | 说明 | 方案 / 历史过程稿 |
| --- | --- | --- | --- |
| RUNTIME-SMO-R2 | Runtime V2 单 owner 状态机重新收口 | Run inventory 越权分类、startup command-ready handoff、语义 conflict 误入 CAS retry、retry lease、schema 不可 drive、projection 误导 | [runtime-v2-single-owner-state-machine-closure-plan.md](runtime-v2-single-owner-state-machine-closure-plan.md) |
| OPT-1 | file-patch deployed live（可选） | 代码迁移已完成；可选补一次 `patch apply + validation + DoD closeout` 的 deployed live | [file-patch-side-effect-owner-migration-plan.md](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

延期产品候选（**不占 active**）：LLM context budget 高级项（failover / tokenizer / 原生 usage 精度 / context-too-large 差异化）→ [candidates/next-stage-demand-candidates.md](../candidates/next-stage-demand-candidates.md)。

统一清单：[current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)、[unfinished-todo-index.md](../../01-overview/unfinished-todo-index.md)。

## Active 文档清理规则

1. Runtime V2 的正式约束只来自 `00-core`、Orchestrator readme 与 typed contract；历史 RUNTIME-SMO、旧
   VPC、freshness、candidate-admission record 均为回归基线。`RUNTIME-SMO-R2` 是唯一活动 Runtime 收口队列，
   不得从历史记录另行推导状态迁移、lifecycle 或 live 修复队列。
2. 本次重新打开基于 current-schema recovery/CAS/projection 的明确反例；旧 completion record 保持 archive，
   不改写为当前规范。
3. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
4. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
5. active 不存放过程日记、历史文件清单或 live 原始产物；过程细节只在 archive 检索。
6. 清理失效内容：V1 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
7. 2026-07-30 后续复核发现 Run inventory 跨 owner 分类、泛化 conflict→CAS retry、无持久化 retry lease 与
   projection 反向误导等 current-schema 反例，故创建 `RUNTIME-SMO-R2`。此前的 `OPS-1` 仍只是假定存在
   production maintenance command 的历史记录，不能作为维护窗口任务。
