# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-07-30

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime V2 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime V2 生产主链 | **V2-only**（Start/Resume → `DriveRuntimeV2` only）；状态 owner/recovery 与 C12 semantic acceptance 均为已验证回归基线 |
| Runtime V2 正向收敛、状态机 owner 与合同归一 | **已完成**；每个 CWI 使用 sealed `CoverageEvidenceBinding`，SM-2 仅从 sealed operation 与 matching owner receipt 派生 coverage。raw CWI、`PassedChecks`、assessment claim 和 terminal projection 均无 coverage authority。 |
| 历史 prerequisite / candidate closure records | **archive baseline only**；旧 VPC、freshness、admission、positive-acceptance 计划不得独立定义当前状态或实施顺序 |
| 22-component framework boundary | **已完成并归档**；22 单元合同、deployed restart 与 workspace 删除后的迟到投影验证均已通过。 |
| V1 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Mandatory Work

当前没有必须关闭的 Runtime V2 代码项。SMO-R2-C12 已完成 T22–T30、replay/CAS、22-unit boundary 与 fresh
dual-workspace semantic live；完成记录见
[runtime-v2-single-owner-state-machine-r2-c12-closure-record.md](../../90-archive/completed/runtime-v2-single-owner-state-machine-r2-c12-closure-record.md)。

| ID | 项 | 说明 | 历史过程稿 |
| --- | --- | --- | --- |
| OPT-1 | file-patch deployed live（可选） | 可补一次 `patch apply + validation + DoD closeout` deployed live；不阻塞 Runtime V2 收口 | [file-patch-side-effect-owner-migration-plan.md](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

延期产品候选（**不占 active**）：LLM context budget 高级项（failover / tokenizer / 原生 usage 精度 / context-too-large 差异化）→ [candidates/next-stage-demand-candidates.md](../candidates/next-stage-demand-candidates.md)。

统一清单：[current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)、[unfinished-todo-index.md](../../01-overview/unfinished-todo-index.md)。

## Active 文档清理规则

1. Runtime V2 的正式约束只来自 `00-core`、Orchestrator readme 与 typed contract；所有 completed record 都仅作
   回归基线，不得从历史记录另行推导状态迁移、lifecycle 或 live 修复队列。
2. current-schema completed Run 必须有 CWI→binding→sealed operation→owner receipt→evidence 链；机械执行绿灯、
   candidate raw CWI、assessment claim、`PassedChecks` 或 terminal 都不能替代该链。
3. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
4. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
5. active 不存放过程日记、历史文件清单或 live 原始产物；过程细节只在 archive 检索。
6. 清理失效内容：V1 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
7. 2026-07-30 后续复核发现 Run inventory 跨 owner 分类、泛化 conflict→CAS retry、无持久化 retry lease 与
   projection 反向误导等 current-schema 反例，故创建 `RUNTIME-SMO-R2`。此前的 `OPS-1` 仍只是假定存在
   production maintenance command 的历史记录，不能作为维护窗口任务。
8. fresh false-completion 反例已由 C12 binding chain 收口；不得以 ticket、budget、decoder 放宽或历史 C8
   绿灯绕过 CWI proof。
