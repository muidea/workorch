# Skill Module

## 功能定位

`kernel/skill` 是 Skill registry 与 selected skill metadata 的 owner module。它负责 Skill 定义、查询、注册和运行上下文中的 skill 引用。

## 拥有边界

- 拥有 Skill 记录与 owner command/query。
- Skill 是指导/配置/能力引用，不是 runtime state transition。
- 不覆盖 Goal/DoD、StagePacket、execution contract 或 owner state。

## 主要交互

- 被 `kernel/orchestrator` 和 LLM context assembly 作为 selected skill refs 使用。
- 被 `application/workorchd` context registry、Web/API 读写。
- 与 `kernel/eventstream` 协作发布 skill 事件。
- 可与 `kernel/soul`、workspace template/profile metadata 一起参与 stage context。

## 组件交互合同

- Input contract: skill command 必须使用结构化 skillId、content/ref、metadata 和 workspace/run binding 信息。
- Output contract: selected skill 输出必须提供 bounded instruction metadata 与 provenance。
- Orchestrator contract: orchestrator 只消费 selected skill refs 和结构化 metadata，不按 skill 文本写死流程。
- LLM contract: skill 指令进入 prompt 前必须经过 context budget 与引用裁剪。
- Application contract: context registry 修改 Skill 只影响指导层，不直接改变 Run/Result owner state。

## 已知缺口

- 当前盘点未发现 Skill module 职责缺失；后续需要继续验证 skill guidance 不覆盖 requiredOutput。

## 边界约束

- skill 指令必须以可追溯 ref 进入 context。
- core runtime 不根据 skill ID 或说明文本写死业务分支。
- Skill module 不执行工具调用，也不创建 Result。
