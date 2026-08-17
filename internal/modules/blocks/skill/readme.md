# Skill Registry Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-17

## 功能范围与边界

`blocks/skill` 管理 Skill registry，负责 Skill 的创建、查询、列表以及指令内容、metadata、binding 和 provenance 的正式状态。当前没有公开 update/delete command。

## 组件交互及合同

- Skill command/query 由 `blocks/skill/pkg/events` 定义，并返回 `SkillCommandResult`。
- `application/workorchd` 通过 EventHub 管理和读取 Skill，并在 fresh activation 中封存选择后的 Skill 指令、引用和 provenance。
- `kernel/orchestrator` 只消费 Run owner 已持久化的 typed activation snapshot，不直接查询 Skill owner；LLM 只接收 Orchestrator 组织后的有界 Skill 指令。
- Workspace/profile 只保存 Skill 引用或选择结果，不复制 Skill owner 状态。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Skill | create/get/list 与 `SkillCommandResult` | 完整 |
| Workspace activation | Application → Run bootstrap | detached Skill refs、instructions、metadata 与 provenance | 完整；不是 Skill owner 写合同 |
| `kernel/orchestrator` / `blocks/llm` | 无直接 Registry I/O | 只消费 sealed activation/request context | 完整；无跨 owner service/repository |

## 相关约束

- 本 block 只访问 Skill repository，不执行工具调用、不创建 Result、不修改 Run 状态。
- Skill 内容进入 prompt 前必须有 provenance、引用边界和 context budget 裁剪。
- core runtime 不得按 Skill ID、名称或说明文本写死业务分支。
- Skill guidance 不得覆盖 Goal、DoD、stage contract 或 requiredOutput。

## 待完善测试点

- [ ] 覆盖 create/get/list 返回值的深拷贝，修改调用方 slice/map 后不得污染 Skill owner 状态。
- [ ] 增加合同清单测试，冻结当前不存在 update/delete topic，防止无意扩大 Registry 写面。
- [ ] 覆盖 instruction、metadata、binding 和 provenance 的非法组合及大小上限。
- [ ] 增加 activation 边界测试，确认 Orchestrator/LLM 无 Skill repository/EventHub 直读且只消费 sealed snapshot。
