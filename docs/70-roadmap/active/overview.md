# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-07-26

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime V2 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime V2 生产主链 | **代码已收口**（Start/Resume → `DriveRuntimeV2` only） |
| Runtime V2 正向收敛基础闭环 | **R5 remediation active**；将 candidate 处理归一为一份 sealed manifest、一个 evaluator 和五类结果。旧 R4/P0～P6/A-4 队列失效，N1～N8 代码/本地门禁已完成；real-provider 双 activation 与三态 restart 仍为外部 fixture residual。 |
| 历史 prerequisite / candidate closure records | **archive baseline only**；此前的 freshness、admission、positive-acceptance 证据不得独立定义当前状态或实施顺序，剩余链路均由 RUNTIME-VPC 统一收口 |
| 22-component framework boundary | **remediation active**；共享 Base Biz、全量本地 structural/race/file-backed 门禁已完成，待外部 deployed restart-fixture matrix |
| V1 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Open（仅此）

| ID | 项 | 说明 | 方案 / 历史过程稿 |
| --- | --- | --- | --- |
| RUNTIME-VPC | Runtime V2 合同归一与 live 正向收敛 | R5 保留严格业务合同，将合同的表达、评价和拒绝路由归一。N1～N8 代码/本地门禁已完成；real-provider 双 activation 与三态 restart 仍为外部 fixture residual。不得以 fallback、固定预算调整或 live 热修跳卡。 | [唯一收口方案](runtime-v2-validation-progress-closure-plan.md) |
| ARCH-1 | 22 组件 framework boundary / lifecycle 验证 | Base Biz / Module-root EventHub 拓扑及本地验证已收口；按 22 组件合同矩阵逐项保持独立回归，待有重启后的外部目标与三类 fixture manifest 后执行 deployed SM-4 matrix | [组件矩阵](component-contract-and-regression-closure-matrix.md) |
| OPS-1 | 生产目录 V1 retirement 正式 apply | 代码/live 已 complete；等待 clean build + 真实生产目录维护窗口执行 `runtime-v1-retirement --apply` | [deployed-workspace-progress-blocker-component-closure-plan.md](../../90-archive/completed/deployed-workspace-progress-blocker-component-closure-plan.md) |
| OPT-1 | file-patch deployed live（可选） | 代码迁移已完成；可选补一次 `patch apply + validation + DoD closeout` 的 deployed live | [file-patch-side-effect-owner-migration-plan.md](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

延期产品候选（**不占 active**）：LLM context budget 高级项（failover / tokenizer / 原生 usage 精度 / context-too-large 差异化）→ [candidates/next-stage-demand-candidates.md](../candidates/next-stage-demand-candidates.md)。

统一清单：[current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)、[unfinished-todo-index.md](../../01-overview/unfinished-todo-index.md)。

## Active 文档清理规则

1. Runtime V2 正向收敛只由 `RUNTIME-VPC` 主方案定义。旧正向收敛、freshness、candidate-admission
   completion record 仅是回归基线；与新 fresh aggregate 反证冲突时，不得再充当 active 约束。
2. 新增 active 项必须对应真实未完成工作；已归档 plan 只有在代码/证据复核给出明确反证并同步作废
   旧 completion record 时才允许重新打开。
3. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
4. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
5. active 不存放过程日记、历史文件清单或 live 原始产物；过程细节只在 archive 检索。
6. 清理失效内容：V1 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
7. 2026-07-26 复核结论：当前 `active/` 仅保留 RUNTIME-VPC 主方案、ARCH-1 组件矩阵与本索引；旧的
   22 组件整改过程已归档。以后若出现第二份 Runtime V2 实施顺序、状态迁移或 live
   修复计划，必须先删除冲突文本或迁入 archive，只能链接 RUNTIME-VPC 的工作卡。
