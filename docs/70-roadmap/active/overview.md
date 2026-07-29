# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-07-29

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime V2 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime V2 生产主链 | **代码已收口**（Start/Resume → `DriveRuntimeV2` only） |
| Runtime V2 正向收敛与状态机 owner | **RUNTIME-SMO active，SMO-7 fresh live**；SMO-7A 的 Shell → File → Orchestrator repair-authority handoff 已完成代码、targeted regression 与静态门禁，待重新部署进行 restart/concurrency/fresh live。C7/C8 仅为回归基线。 |
| 历史 prerequisite / candidate closure records | **archive baseline only**；旧 VPC、freshness、admission、positive-acceptance 计划不得独立定义当前状态或实施顺序 |
| 22-component framework boundary | **remediation active**；共享 Base Biz、全量本地 structural/race/file-backed 门禁已完成，待外部 deployed restart-fixture matrix |
| V1 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Open（仅此）

| ID | 项 | 说明 | 方案 / 历史过程稿 |
| --- | --- | --- | --- |
| RUNTIME-SMO | Runtime V2 单 owner 状态机统一收口 | 唯一活动 Runtime 队列。SMO-1～SMO-7A 为已完成代码基线；当前执行 SMO-7 fresh live，验证可修复 validation failure 的 Shell fact → File confirmation → Orchestrator internal-event handoff 不重放且能形成 state-change attempt-0；不做兼容双写。 | [单 owner 统一收口方案](runtime-v2-single-owner-state-machine-closure-plan.md) |
| ARCH-1 | 22 组件 framework boundary / lifecycle 验证 | Base Biz / Module-root EventHub 拓扑及本地验证已收口；按 22 组件合同矩阵逐项保持独立回归，待有重启后的外部目标与三类 fixture manifest 后执行 deployed SM-4 matrix | [组件矩阵](component-contract-and-regression-closure-matrix.md) |
| OPS-1 | 生产目录 V1 retirement 正式 apply | 代码/live 已 complete；等待 clean build + 真实生产目录维护窗口执行 `runtime-v1-retirement --apply` | [deployed-workspace-progress-blocker-component-closure-plan.md](../../90-archive/completed/deployed-workspace-progress-blocker-component-closure-plan.md) |
| OPT-1 | file-patch deployed live（可选） | 代码迁移已完成；可选补一次 `patch apply + validation + DoD closeout` 的 deployed live | [file-patch-side-effect-owner-migration-plan.md](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

延期产品候选（**不占 active**）：LLM context budget 高级项（failover / tokenizer / 原生 usage 精度 / context-too-large 差异化）→ [candidates/next-stage-demand-candidates.md](../candidates/next-stage-demand-candidates.md)。

统一清单：[current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)、[unfinished-todo-index.md](../../01-overview/unfinished-todo-index.md)。

## Active 文档清理规则

1. Runtime V2 的活动约束只有 `RUNTIME-SMO`：它统一负责 candidate/receipt、SM-1～SM-4 owner、跨状态机
   原子交接、lifecycle projection 与 live。旧 VPC、freshness、candidate-admission completion record 仅是
   回归基线；不得再产生第二套状态迁移、lifecycle 或 live 修复队列。
2. 新增 active 项必须对应真实未完成工作；已归档 plan 只有在代码/证据复核给出明确反证并同步作废
   旧 completion record 时才允许重新打开。
3. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
4. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
5. active 不存放过程日记、历史文件清单或 live 原始产物；过程细节只在 archive 检索。
6. 清理失效内容：V1 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
7. 2026-07-28 复核结论：当前 `active/` 仅保留 RUNTIME-SMO、ARCH-1 及运维尾项；旧 VPC 与 22 组件整改
   过程已归档。新增 Runtime V2 实施文本必须归入 SMO 的一张串行卡，冲突文本直接删除或迁入 archive。
