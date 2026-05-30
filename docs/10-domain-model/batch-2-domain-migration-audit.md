# Batch 2 Domain Migration Audit

Status: active

Type: audit

Last Updated: 2026-05-29

## Scope

本审计记录 `Batch 2` 建立领域模型层时识别出的三类问题：

- 冲突和不一致
- 内容不完整、需要补充
- 无法完整衔接的内容断点

## 1. 冲突和不一致

### 1.1 Workspace 领域边界与存储实现边界混写

`../workorch-workspace-deliverable-boundary-design.md` 同时包含：

- Workspace 领域边界
- 目录布局
- 工具执行规则
- 读模型边界
- 迁移策略

处理结论：

- 已在领域层只保留 Workspace 本体与边界
- 目录布局和实现落点后续继续迁到模块架构层

### 1.2 Memory 同时被描述为知识资产和过程证据来源

`../workspace-memory-recall-traceability-plan.md` 与
`../llm-decision-memory-improvement-plan.md` 对 Memory 的关注点不同：

- 前者强调 recall trace 和过程证据
- 后者强调高价值 Memory 的写入、排序、召回策略

处理结论：

- 领域层只保留 `MemoryItem` 与 `MemoryRecallTrace` 的对象边界
- 排序、refresh、repair 继承等动态规则后续迁到 flow 层
- `../llm-decision-memory-improvement-plan.md` 的高价值 memory 分类与 accepted input / necessary output 规则已开始迁入 `context-capability-memory.md`

### 1.3 Result / Artifact / Event 与旧状态文档存在重复定义

`../next-stage-result-event-development-plan.md`、`../workorch-runtime-storage-json-control-design.md`
和旧综合状态文档（现归档副本见 `../90-archive/superseded/workorch-status.md`）
都描述了 Result / Artifact / Event 的部分边界。

处理结论：

- 领域层先抽稳定对象定义
- 细节差异和 API/read-model 行为冲突后续在 flow/module 层继续校正

## 2. 内容不完整，需要补充

### 2.1 Goal / DoD definition revision 仍未独立建模

核心 contract 已把 definition revision、definition change impact 作为重要约束，
但当前领域层还未完整展开：

- definition revision
- previous definition hash
- definition change lineage

### 2.2 Run 过程证据对象还缺更细拆分

当前只在 `run-step-result-artifact.md` 中作为 evidence objects 概括了：

- ToolCall
- Patch
- ValidationResult
- RuntimeCheckpoint

但还缺：

- 每类对象更明确的最小字段边界
- 它们与 Step / Result / Artifact 的拥有关系

### 2.3 Governance 领域还缺 auto-governance 语义

Decision / Approval / Resolution 已抽出基础对象，
但以下内容还缺对象级说明：

- duplicate failure
- repair exhausted
- auto-governance ticket
- successor replan lineage

### 2.4 Event / Outcome / TaskProgress 仍偏概括

当前只建立了投影边界，仍缺：

- outcome 选择规则
- taskProgress 与 latest result/run 的约束
- event payload summary 与 raw payload ref 的对象层说明

### 2.5 MemoryRecallTrace 已迁入稳定对象边界，但动态排序规则仍缺

当前 `context-capability-memory.md` 已承接：

- `MemoryItem` 的稳定用途边界
- `MemoryRecallTrace` 的对象边界
- activation / step_input / recovery 的 source 区分

仍缺：

- recall 排序策略
- run 优先与类型配额
- semantic recall 规则细化

## 3. 无法完整衔接的内容断点

### 3.1 旧文档仍按实现/计划视角写作，不是纯对象文档

Batch 2 的输入文档普遍混有：

- 实施任务
- 验收脚本
- 历史进度
- API / Web / TUI 展示要求

这意味着领域层只能先抽出第一版稳定对象定义，无法直接机械迁移原文。

### 3.2 旧综合状态文档仍是当时部分对象边界的补充来源

例如：

- `Formal Event Boundary`
- `Task Progress And Recovery Projection`

这些内容在当时仍分散在旧综合状态文档中，
新领域文档暂时无法完全替代它。

### 3.3 Memory / Recall 的动态行为与静态对象边界仍缠绕

当前很多 Memory 规则本质上属于 runtime flow：

- activation recall
- step input refresh
- provider retry inheritance
- repair/replan 继承策略

这些内容在领域层无法完整收口，必须等 `20-runtime-flow/` 建立后再继续拆分。
