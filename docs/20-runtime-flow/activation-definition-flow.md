# Activation And Definition Flow

Status: active

Type: runtime-flow

Last Updated: 2026-05-30

## Purpose

本文件定义从 workspace definition 到 execution activation，再到首个
`llm/workspace_execution` Step 的形成链路。

## Flow

```text
Workspace / Goal / DoD / Soul / Skill / MCP / Memory selection
-> execution activation request
-> appservice validation
-> Workspace / Goal / DoD / context resolution
-> Run creation
-> activation context assembly
-> first Step input assembly
-> orchestrator dispatches llm/workspace_execution
```

## Main Stages

### 1. Definition Resolution

activation 前必须明确：

- 当前 Workspace
- 当前有效 Goal / DoD
- 当前 Soul / Skill / MCP / Memory 选择
- 当前 deliverable / internal directory 边界

如果 Goal / DoD 缺失、无效或不完整，activation 不应进入实际运行。

#### Definition Change Impact

activation 在读取 Workspace / Goal / DoD 时，至少需要判断：

- 当前 run 是否仍基于最新 `definitionRevision`
- 旧 revision 的 active coverage / repair scope 是否还能沿用
- 当前 activation 是否需要重建 baseline、selectedContext 或 successor scope

稳定方向：

- activation 只消费当前 effective definition
- superseded / historical 的旧 Goal / DoD 只能作为 lineage reference
- definition change impact 是 activation 前置判断，不应等到 repair / acceptance 阶段才第一次发现

### 2. Execution Activation

activation 的职责是把“正式定义”和“当前上下文选择”变成一个新的 Run。

activation 不应直接承担：

- 最终业务结果判断
- graph 调度
- tool 执行
- patch 应用

#### Activation Vs Workspace Setup

activation 与 workspace setup / draft session 是不同入口：

| 入口 | 当前职责 |
| --- | --- |
| workspace setup / draft | 创建前澄清、模板建议、Soul 预选、Goal/DoD 草案 |
| execution activation | 基于已确认并已落库的正式 Workspace / Goal / DoD / bindings 创建新的 Run |

约束：

- setup draft 不创建 Run
- activation 不负责生成创建前草案
- template / draft / Soul guidance 只能影响 activation 的输入基线，不能替代正式定义

### 3. Initial Context Materialization

Run 创建后，需要把 activation 结果转化为首个 LLM Step 可消费的运行时输入。

该输入至少应包含：

- 当前 Goal / DoD
- 当前 stage / requested action
- 选中的 Skill / Soul / MCP / Memory 上下文
- 当前 workspace 已知资源摘要
- 当前 definition revision / lineage
- 当前 activation 形成的 coverage baseline / 初始 work item scope

稳定要求：

- activation 必须基于当前 effective definition 形成首轮 coverage baseline
- 若 definition revision 已变化，旧 revision 的 active coverage/work item 不得直接复用为新 run 的初始 scope
- 首个 step input 至少应能看到当前 definition 对应的初始收敛面，而不是等到 verification / repair 再补建 coverage ledger

#### Successor Activation Inheritance

当 activation 来源于 replan successor、governance continuation 或 definition change 后的新 run 时，
至少应稳定继承：

- predecessor failure context 的必要摘要
- 当前 effective Goal / DoD definition revision
- 仍有效的 selected Skill / Soul / MCP / Memory 选择
- superseded / historical run 只作为 lineage reference，不得直接污染新的 active contract

不应直接继承：

- 旧 revision 的 active coverage scope
- 已被 superseded 的 failure focus
- 已失效的 target snapshot / rejected output / wrapper failure 壳

## Key Flow Guarantees

- activation 必须围绕正式 Goal / DoD 定义工作，不能只依赖旧 run 历史
- activation 生成的是 Run，而不是直接生成最终输出
- 首个 step 必须是正式的 `llm/workspace_execution`
- Memory recall / selectedContext 应该在此时留下 activation 侧的过程证据
- definition change 必须先影响 activation 输入基线，而不是让旧 revision 默认继续执行
- successor activation 只能继承仍有效的 contract / context / lineage，而不能机械复制旧 run 活跃状态

## Output

activation 的正式输出是：

- 新的 Run
- 初始 execution context
- 首个 LLM Step 的 dispatch 前提

## Source Documents

- `planning-execution-flow.md`
- `../00-core/runtime-stage-contract.md`
- `../50-product-interface/service-api.md`
