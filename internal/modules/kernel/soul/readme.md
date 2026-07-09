# Soul Module

## 功能定位

`kernel/soul` 是 Soul catalog、stage policy refs、capability/risk constraints 元数据的 owner module。它负责 Soul 定义、绑定所需的结构化策略元数据与查询。

## 拥有边界

- 拥有 Soul catalog 记录与 owner command/query。
- 维护 stage policy refs、capability constraints、risk constraints 等结构化元数据。
- 不直接调度 runtime block，也不决定 Run/Result 状态。

## 主要交互

- 被 `kernel/workspace` 或 workspace template/profile metadata 绑定到 workspace/run context。
- 被 `kernel/orchestrator` 作为中立 stage policy refs 读取。
- 被 `application/workorchd` context registry、Web/API 读写。
- 与 `kernel/skill` 一起参与 LLM context 的指导层。

## 组件交互合同

- Input contract: soul command 必须使用结构化 soulId、stage policy refs、constraints 和 metadata。
- Output contract: selected soul/stage policy 输出必须包含 refs、约束字段和 provenance。
- Workspace contract: workspace/template 只能绑定 soul refs，不复制 soul 内部实现为 owner state。
- Orchestrator contract: orchestrator 通过中立 constraints 消费 Soul，不按 Soul ID 或展示文本分支。
- LLM contract: Soul 指导进入 prompt 前必须 bounded，并保持与当前 stage contract 不冲突。

## 已知缺口

- 当前盘点未发现 Soul module 职责缺失；后续需要继续验证 stage policy refs 与 runtime requiredOutput 的优先级关系。

## 边界约束

- core runtime 不按 Soul ID 或显示文本分支。
- Soul 约束只能通过结构化中立字段消费。
- Soul guidance 不能覆盖当前 Goal/DoD 或 requiredOutput。
