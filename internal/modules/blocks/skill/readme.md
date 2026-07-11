# Skill Registry Stateful Block

## 功能说明

`blocks/skill` 管理 Skill registry，负责 Skill 的创建、查询、列表、更新以及指令内容、metadata、binding 和 provenance 的正式状态。

## 组件交互及合同

- Skill command/query 由 `blocks/skill/pkg/events` 定义，并返回 `SkillCommandResult`。
- `application/workorchd` 通过 EventHub 管理 Skill；`kernel/orchestrator` 通过 EventHub 获取选择后的 Skill refs 和有界 metadata。
- LLM 只接收 Orchestrator 组织后的 Skill 指令，不直接调用 Skill repository 或 service。
- Workspace/profile 只保存 Skill 引用或选择结果，不复制 Skill owner 状态。

## 相关约束

- 本 block 只访问 Skill repository，不执行工具调用、不创建 Result、不修改 Run 状态。
- Skill 内容进入 prompt 前必须有 provenance、引用边界和 context budget 裁剪。
- core runtime 不得按 Skill ID、名称或说明文本写死业务分支。
- Skill guidance 不得覆盖 Goal、DoD、stage contract 或 requiredOutput。
