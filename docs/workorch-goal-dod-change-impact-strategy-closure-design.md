# WorkOrch Goal/DoD Change Impact and Repair Strategy Closure Design

本文定义 Goal / DoD 发生变化或累计时，WorkOrch runtime 如何在保持核心功能中立的前提下，先完成影响评估与策略选择，再进入状态变更、验证和验收。本文是 `workorch-runtime-stage-contract-neutrality-design.md` 在定义变化与修复收敛维度的补充设计，所有实现不得引入软件开发、测试框架、代码结构或具体业务领域规则。

## 背景

最新 workspace 执行核对暴露出一个共性问题：当 Goal / DoD 新增或调整后，LLM 容易直接沿用上一轮局部修复路径，或者只针对当前失败表面做变更，未先判断新目标与既有约束的关系。结果是：

- 新 Goal 被误当成替换约束，而不是累计约束。
- 旧 Goal / DoD 的有效合同在后续修复中被弱化。
- 修复多次落在失败表面，未重新判断根因目标。
- 验收证据、策略说明与实际变更之间缺少可追踪关系。

因此 runtime 需要提供中立的“定义变化影响包”和“修复策略重评估包”，让 LLM 在进入状态变更前明确：哪些约束仍然有效，哪些要求是新增的，是否存在冲突，应修改哪些中立 artifact / evidence / capability 输出，以及哪些修复方式被禁止。

## 核心约束

- Core runtime 只表达 Goal、DoD、constraint、artifact、evidence、capability、failure scope、repair target、strategy、blocker 等中立概念。
- Core runtime 不解释具体业务语义，不判断某个文件、接口、测试或产品行为应如何实现。
- 新 Goal / DoD 默认按 workspace 累计合同处理：latest refines previous，不替换仍然有效的历史约束。
- 若新旧约束冲突，LLM 必须输出冲突 / blocker / failed assessment，而不是绕过旧约束继续 patch。
- 修复不得通过削弱已有验收证据、删除仍有效覆盖、放宽约束或只改报告文本来伪造完成。
- Runtime 只能治理策略形态和证据关系；具体方案由 LLM 在当前 stage contract、profile、soul、skill、tool、mcp 约束内选择。

## Definition Change Impact 合同

当 activation 或 run context patch 发现 workspace 存在累计 Goal / DoD 时，runtime 必须生成 `definitionChangeImpact`：

```json
{
  "version": "workorch.definition_change_impact.v1",
  "changed": true,
  "policy": "workspace_goal_dod_cumulative_latest_refines_previous",
  "requiredPreExecutionAssessment": true,
  "goalChange": {
    "effectiveGoalId": "goal-latest",
    "latestGoalId": "goal-latest",
    "previousGoalIds": ["goal-old"],
    "allGoalsAreConstraints": true
  },
  "doDChange": {
    "effectiveDoDId": "dod-latest",
    "latestDoDId": "dod-latest",
    "previousDoDIds": ["dod-old"],
    "allDoDItemsAreAcceptanceGates": true
  },
  "strategyRequirements": [
    "preserve still-valid prior Goal and DoD constraints",
    "identify additive requirements and conflicts before state-changing work",
    "choose state-change, evidence, and verification targets before state-changing work",
    "do not weaken prior acceptance evidence or constraints"
  ]
}
```

字段语义：

- `changed`: 是否存在累计定义变化。只有多个 workspace Goal 或多个 workspace DoD 时为 `true`。
- `policy`: 只描述中立定义合并策略，不携带业务解释。
- `requiredPreExecutionAssessment`: 提醒 LLM 在状态变更前先做影响评估与策略选择。
- `goalChange` / `doDChange`: 只包含 ID、latest、previous、累计约束标志。
- `strategyRequirements`: 中立策略要求，供 prompt 和 repair package 使用。

`definitionChangeImpact` 属于 runtime 生成的保留上下文，activation 的外部 context patch 不得覆盖该字段。

## LLM Strategy Requirements

当 `definitionChangeImpact.changed=true` 时，LLM 的首个可执行决策必须在 `thoughtSummary`、`plan`、`executionGraph` 节点摘要或等价结构中体现以下判断：

- 影响范围：哪些当前 Goal / DoD 是新增约束，哪些历史约束仍需保留。
- 兼容关系：新约束是补充、细化、冲突还是阻塞。
- 执行策略：选择哪些中立 artifact / capability / evidence 目标进行状态变更或验证。
- 验收策略：如何映射所有有效 Goal / DoD coverage。
- 禁止路径：不得通过弱化旧约束、删除有效证据、降低验收要求来通过。

Core 不要求固定字段名，也不强制引入新 LLM schema 字段；但 prompt 必须明确这些要求，runtime package 必须稳定提供输入。

## Repair Strategy Reassessment 合同

当 repair 在同一 target scope 或同一 failure class 上反复失败时，runtime 必须在 repair 输入或 `repairActionPackage` 中加入 `strategyReassessment`：

```json
{
  "required": true,
  "reason": "same_target_scope_repeated_failure",
  "failedSurface": "targetRef=artifact",
  "repairAttempt": 4,
  "targetRepairAttempts": 2,
  "requiredDecisionFields": [
    "failedSurface",
    "rootCauseTarget",
    "repairTargetDecision",
    "forbiddenRepair",
    "verificationPlan"
  ],
  "requirements": [
    "distinguish failed surface from root cause target before patching again",
    "explain whether the next change targets state-change artifact, evidence, verification, or blocker handling",
    "do not weaken still-valid Goal or DoD constraints to make verification pass"
  ]
}
```

触发条件：

- `targetRepairAttempts >= 2`。
- 或 `runtimeRepairAttempt >= 3` 且当前失败仍指向同一 failure class / target scope。

该包只提示重评估，不直接指定领域修复方式。若重评估后发现当前路径不成立，LLM 应输出 failed assessment、blocker 或可执行的策略纠正变更。

## 收口清单

| 项 | 内容 | 状态 |
| --- | --- | --- |
| D1 | 输出本文档，明确中立合同与实现边界 | 完成 |
| I1 | activation / run context patch 生成 `definitionChangeImpact` | 完成 |
| I2 | 外部 executionContextPatch 禁止覆盖 `definitionChangeImpact` | 完成 |
| I3 | executionContextPackage 保留并压缩 `definitionChangeImpact` | 完成 |
| I4 | LLM prompt 暴露定义变化评估与策略选择要求 | 完成 |
| I5 | runtime repair 对重复失败加入 `strategyReassessment` | 完成 |
| I6 | repairActionPackage 携带 strategy reassessment 约束 | 完成 |
| T1 | activation/context package/prompt/repair 单测覆盖 | 完成 |

## 验收标准

- 多个 Goal / DoD 的 workspace activation 必须生成 `definitionChangeImpact.changed=true`。
- 该字段必须进入 `executionContextPackage`，并能在 LLM prompt 中被引用。
- 外部 context patch 不能覆盖 Goal、DoD、definition impact、rejected continuation 等 runtime 保留字段。
- 同 target 重复 repair 时，repair input 与 action package 必须要求 strategy reassessment。
- 所有新增规则保持中立，不包含软件开发、代码、测试框架、provider 业务逻辑等 core 特化规则。
