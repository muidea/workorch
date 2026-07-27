# WorkOrch Active Roadmap

Status: active

Type: roadmap

Last Updated: 2026-07-27

## Purpose

仅索引 **当前仍未关闭** 的工作项。已完成 / code-closed / 过程日记 / 被 Runtime V2 吸收的计划一律在 [90-archive/completed](../../90-archive/completed/overview.md) 检索，**不得**再从 archive 推导新实现约束。

## Runtime Code Status

| 主题 | 状态 |
| --- | --- |
| Runtime V2 生产主链 | **代码已收口**（Start/Resume → `DriveRuntimeV2` only） |
| Runtime V2 正向收敛基础闭环 | **R5 remediation active**；将 candidate 处理归一为一份 sealed manifest、一个 evaluator 和五类结果。旧 R4/P0～P6/A-4 队列失效，N1～N8 代码/本地门禁已完成；real-provider 双 activation 与三态 restart 仍为外部 fixture residual。 |
| Runtime V2 状态机 owner / lifecycle | **RUNTIME-SMO active**；第一阶段已移除旧 Orchestrator lifecycle facade 并增加 owner reducer，但实现复核已重新打开 R1～R8：generic 写入、sealed fact identity、administrative terminal、delivery CAS/fence、startup recovery 与门禁仍需收口。不能以 VPC 的 candidate 合同修复替代。 |
| 历史 prerequisite / candidate closure records | **archive baseline only**；此前的 freshness、admission、positive-acceptance 证据不得独立定义当前状态或实施顺序，剩余链路均由 RUNTIME-VPC 统一收口 |
| 22-component framework boundary | **remediation active**；共享 Base Biz、全量本地 structural/race/file-backed 门禁已完成，待外部 deployed restart-fixture matrix |
| V1 StagePacket / NextAction / graph dispatch 主链 | **已删除** |
| 设计结论上提 | 见 [00-core/runtime-stage-contract.md](../../00-core/runtime-stage-contract.md)、[current-state.md](../../01-overview/current-state.md) |

## Residual Open（仅此）

| ID | 项 | 说明 | 方案 / 历史过程稿 |
| --- | --- | --- | --- |
| RUNTIME-VPC | Runtime V2 合同归一与 live 正向收敛 | R5 保留严格业务合同，将合同的表达、评价和拒绝路由归一。N1～N8 代码/本地门禁已完成；real-provider 双 activation 与三态 restart 仍为外部 fixture residual。不得以 fallback、固定预算调整或 live 热修跳卡。 | [VPC candidate/receipt 方案](runtime-v2-validation-progress-closure-plan.md) |
| RUNTIME-SMO | Runtime V2 单 owner 状态机与 lifecycle | 旧 Orchestrator direct lifecycle facade 已删除，但 owner reducer仍有 parallel generic write、未封存的 fact identity、行政终态绕过、delivery CAS/fence 与 recovery 缺口。按 SMO 第 12 节 R1～R8 串行收口；不做兼容双写。 | [单 owner 收口方案](runtime-v2-single-owner-state-machine-closure-plan.md) |
| ARCH-1 | 22 组件 framework boundary / lifecycle 验证 | Base Biz / Module-root EventHub 拓扑及本地验证已收口；按 22 组件合同矩阵逐项保持独立回归，待有重启后的外部目标与三类 fixture manifest 后执行 deployed SM-4 matrix | [组件矩阵](component-contract-and-regression-closure-matrix.md) |
| OPS-1 | 生产目录 V1 retirement 正式 apply | 代码/live 已 complete；等待 clean build + 真实生产目录维护窗口执行 `runtime-v1-retirement --apply` | [deployed-workspace-progress-blocker-component-closure-plan.md](../../90-archive/completed/deployed-workspace-progress-blocker-component-closure-plan.md) |
| OPT-1 | file-patch deployed live（可选） | 代码迁移已完成；可选补一次 `patch apply + validation + DoD closeout` 的 deployed live | [file-patch-side-effect-owner-migration-plan.md](../../90-archive/completed/file-patch-side-effect-owner-migration-plan.md) |

延期产品候选（**不占 active**）：LLM context budget 高级项（failover / tokenizer / 原生 usage 精度 / context-too-large 差异化）→ [candidates/next-stage-demand-candidates.md](../candidates/next-stage-demand-candidates.md)。

统一清单：[current-unclosed-matrix.md](../../01-overview/current-unclosed-matrix.md)、[unfinished-todo-index.md](../../01-overview/unfinished-todo-index.md)。

## Active 文档清理规则

1. Runtime V2 的活动约束仅分为两个**不重叠**队列：`RUNTIME-VPC` 负责 candidate manifest、
   evaluator、receipt 与 live observation；`RUNTIME-SMO` 负责状态 owner、lifecycle projection 与跨 owner
   state fact。旧正向收敛、freshness、candidate-admission completion record 仅是回归基线；不得再产生第三套
   状态迁移、lifecycle 或 live 修复队列。
2. 新增 active 项必须对应真实未完成工作；已归档 plan 只有在代码/证据复核给出明确反证并同步作废
   旧 completion record 时才允许重新打开。
3. 实现约束以 `00-core` + 模块 readme + `workorchruntimecontract` 为准。
4. 某项 code-closed 且无运维/产品尾巴时，应 `git mv` 至 `90-archive/completed/` 并更新本 overview。
5. active 不存放过程日记、历史文件清单或 live 原始产物；过程细节只在 archive 检索。
6. 清理失效内容：V1 主链术语、重复 audit、假活跃 Status 不得继续占用 active。
7. 2026-07-27 复核结论：当前 `active/` 仅保留 RUNTIME-VPC、RUNTIME-SMO、ARCH-1 及运维尾项；旧的
   22 组件整改过程已归档。新增 Runtime V2 实施文本必须明确归入 VPC 或 SMO；若同时改变两者，必须拆成两张
   串行卡并删除冲突文本或迁入 archive。
