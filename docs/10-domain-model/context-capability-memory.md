# Context / Capability / Memory Domain Model

Status: active

Type: domain-model

Last Updated: 2026-07-14

## Purpose

本文件定义 WorkOrch 在运行时使用的上下文与能力对象：

- Soul
- Skill
- MCP Server / Capability
- Memory
- MemoryRecallTrace

**Runtime V2 边界**：Soul/Skill/MCP/Memory 在 activation 时封存为 `RuntimeActivationContext` 快照进入 bootstrap；candidate 上下文只读这些快照与 facts，不经 V1 execution-context map 重建。Memory 经 EventHub 由 owner 管理；recall **不得**替代 current-run accepted evidence 关闭 assessment。profile/capability 字段是 activation / C2 输入，kernel reducer 不做 profile ID 特判。

## Core Entities

### Soul

Soul 是治理和阶段行为边界的声明对象。

Soul 可以定义：

- 阶段约束
- allowed tool kinds
- capability allowlist
- 风险边界

### Skill

Skill 表示可复用的任务指导单元。

Skill 可来源于：

- WorkOrch 自身管理
- 外部 skill-hub / workspace-local 技能资产

### MCP Server / Capability

MCP Server 表示外部能力提供者。

Capability 是对工具、资源、提示词等能力的结构化描述。

它们的领域作用是：

- 表达能力边界
- 表达输入/输出 contract
- 表达启用、约束和使用策略

### Memory

MemoryItem 是可复用的知识压缩对象，不等于事件日志摘要。

当前稳定用途包括：

- project_context
- dod_feedback
- execution_lesson
- decision
- tool_feedback
- skill_feedback
- run_summary

稳定约束：

- Memory 只沉淀可复用的高价值输入、输出、约定、决策和经验。
- provider retry、schema invalid、瞬态 repair loop 和低价值日志不应默认固化为 Memory。
- Memory 不是过程证据本体，也不替代 step / event / artifact / validation evidence。

高价值 Memory 资产应优先围绕两类内容组织：

- accepted input memory：已被 runtime 接受并进入后续推理上下文的关键输入
- necessary output memory：围绕 Goal / DoD 输出、并被 runtime 接受、后续必须延续的结论

典型稳定内容包括：

- `project_context`：项目结构、入口、配置、验收边界等被接受的事实
- `dod_feedback`：DoD gap、coverage gap、failed/skipped item 的结构化结论
- `execution_lesson`：repair plan、verification plan、禁止重复的无效路径
- `tool_feedback` / `skill_feedback`：Skill、Tool、MCP、Soul、Prompt 的有效选择与约束
- `decision`：已确认的决策结论，而不是完整对话流水

### MemoryRecallTrace

MemoryRecallTrace 是过程证据对象。

它记录：

- 为什么 recall
- 何时 recall
- recall 到了哪些 memory
- 这些 recall 如何进入 activation / step input / recovery

它不替代 MemoryItem，而是 Memory 进入运行态上下文的过程证据。

稳定字段边界至少包括：

- `workspaceId`
- `runId`
- `stepId`
- `graphNodeId`
- `stage`
- `source`
- `policy`
- `status`
- `selectedMemoryIds`
- `candidateMemoryIds`
- `excludedMemoryIds`
- `retrievedMemoryHits`
- `reason`
- `occurredAt`
- `evidenceRef`

其中 `source` 应至少区分：

- `activation`
- `step_input`
- `recovery`

约束：

- `selectedMemoryIds` 表示最终进入当前运行态上下文的 memory
- `candidateMemoryIds` 表示被检索和评估过、但未必最终进入上下文的候选集
- `excludedMemoryIds` 表示被显式过滤或裁剪的 recall 对象
- `retrievedMemoryHits` 用于解释命中摘要、分值和选择原因，不等于 Memory 正文

#### Recall Ordering Policy

`MemoryRecallTrace` 进入运行态上下文时，当前排序优先级应稳定为：

1. 当前 run / 当前 step 的直接相关 recall
2. 当前 effective Goal / DoD / coverage gap 直接相关 recall
3. 最新 accepted input / necessary output memory
4. 当前 workspace 内仍有效的 project / decision / lesson memory
5. superseded / historical run 的低价值 summary 作为低优先级参考

约束：

- 只因为时间新而与当前失败面无关的 recall，不应压过当前目标直接相关 recall
- superseded / failed run 的 `run_summary` 默认不得优先于当前 run 的高价值 recall
- provider retry / wrapper failure 产生的弱信号 recall 不得占据高优先级槽位

#### Type Quota

recall 进入 `selectedContext` 时，至少应受中立类型配额约束。

推荐稳定方向：

| Memory 类型族 | 当前优先级 | 配额方向 |
| --- | --- | --- |
| `project_context` / accepted input | 高 | 保底保留 |
| `decision` / necessary output / active constraint | 高 | 保底保留 |
| `dod_feedback` / execution_lesson | 中高 | 受当前 coverage gap 驱动 |
| `tool_feedback` / `skill_feedback` | 中 | 只在当前 capability / stage 相关时进入 |
| `run_summary` | 低 | 仅在缺少更直接 recall 时补充 |

约束：

- 类型配额是 recall 裁剪规则，不是业务语义判断
- 同类型 recall 过多时，应优先保留当前 run / 当前 workspace / 当前目标直接相关的条目
- 低价值 `run_summary` 不得挤掉 `project_context`、`decision`、`dod_feedback` 这类高价值类型

#### Semantic Recall Rule

semantic recall 的对象层规则当前应至少保持：

- 只服务当前 Goal / DoD / failure surface / coverage gap / capability selection
- 只能提升“与当前目标语义相关”的高价值 memory，不得把所有相似文本都重新注入
- 结果必须通过 `MemoryRecallTrace` 记录：
  - candidate memory
  - selected memory
  - excluded memory
  - reason / policy / status

边界：

- semantic recall 是 context selection 策略，不是业务结论来源
- semantic 命中不等于 accepted evidence
- recall 结果若无法通过当前 stage / policy / type quota 过滤，必须进入 excluded / omitted，而不是静默消失

## Boundary

Context / Capability / Memory 层负责表达：

- 当前可用的能力边界
- 当前被选中的上下文资产
- 当前被召回的高价值记忆

它不直接表达：

- 正式业务结果
- 执行生命周期本体 / Runtime V2 aggregate
- 页面交互层表现
- assessment / Result 关闭条件

Memory 经 EventHub 由 `blocks/memory` owner 管理；Orchestrator / Governance 只发具体 command，不持有 Memory repository。

## Invariants

- Skill / Soul / MCP / Capability 的选择结果应可追溯
- Memory 只应沉淀可复用的高价值输入/输出，不应退化为事件日志摘要
- Memory recall 必须是正式过程证据，而不是仅靠前端解析 raw executionContext 或 V1 map
- provider retry / repair / replan 应能继承必要的上下文资产，但不能把无边界旧 Memory 默认重新注入
- `MemoryRecallTrace` 记录的是 recall 进入运行态的事实，不表达业务结果正确性
- recovery 继承已有 recall 时也应有可追踪 trace，而不是静默复用旧上下文
- 当前 run 的高价值 Memory 应优先于 superseded / failed run 的低价值 `run_summary`
- 只读流程摘要、provider retry 流水、泛化 `no test files` 之类弱信号不应占满 memory recall 槽位
- recall 顺序、类型配额与 semantic 策略只影响上下文选择，不直接表达 accepted business outcome
- recall **不得**替代 current-run accepted evidence 关闭 assessment 或 Result gate

## Projection Boundary

`selectedContext`、`ContextBundle`、`memoryRecallTraces`、Web/TUI context views
都属于上下文对象的运行态投影；不得反向写 Memory owner 或 V2 aggregate。

## Source Documents

- `../20-runtime-flow/context-assembly-flow.md`
- `../30-module-architecture/kernel-context-capability.md`
- `../40-profiles-capabilities/workspace/soul-routing.md`
- `../00-core/runtime-stage-contract.md`
- `run-step-result-artifact.md`
