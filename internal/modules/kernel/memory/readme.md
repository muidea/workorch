# Memory Module

## 功能定位

`kernel/memory` 是 MemoryItem 与 MemoryRecallTrace 的 owner module。它负责长期记忆、召回轨迹、向量准备元数据和 workspace/run 相关 memory 查询。

## 拥有边界

- 拥有 MemoryItem、MemoryRecallTrace 的 owner command/query。
- 记录 selected/candidate/excluded recall trace 与 provenance。
- 不覆盖当前 Goal/DoD、StagePacket 或 runtime contract。

## 主要交互

- 被 `kernel/orchestrator` 用于 runtime context assembly、recall trace 和 result memory 写入。
- 被 `blocks/llm` 间接消费为 LLM context 的一部分。
- 被 `application/workorchd` 用于 memory API、workspace snapshot 和 run context bundle。
- 与 `kernel/eventstream` 协作发布 memory/recall 事件。

## 组件交互合同

- Input contract: memory create/update/recall trace 命令必须包含 workspace/run/source/provenance 中的必要字段。
- Output contract: recall 输出必须区分 selected、candidate、excluded，并保留 reason 与 source refs。
- Orchestrator contract: orchestrator 可选择 memory context，但 memory 不覆盖 Goal/DoD 或 StagePacket。
- LLM contract: 进入 prompt 的 memory 必须 bounded、traceable，并可从 recall trace 反查。
- Projection contract: application 展示 memory 时必须保留 workspace/run scope 和召回状态。

## 已知缺口

- 当前盘点未发现 Memory module 职责缺失；需持续验证 rejected diagnostic 与 provider retry 噪声不会进入高价值 memory。

## 边界约束

- 只有高价值、可复用、带 provenance 的信息应进入长期 memory。
- rejected diagnostic、provider retry 噪声和 transient plumbing 信息不能成为高价值 memory。
- memory 不能改变 owner state 或 acceptance 结论。
