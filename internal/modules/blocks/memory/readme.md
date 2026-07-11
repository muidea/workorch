# Memory Stateful Block

## 功能定位

`blocks/memory` 是 MemoryItem 与 MemoryRecallTrace 的单一记忆资源聚合 block。它管理长期记忆、召回轨迹和向量准备元数据，但不承担 runtime context assembly 或跨组件状态编排。

## 拥有边界

- 拥有 MemoryItem、MemoryRecallTrace 的 owner command/query。
- 记录 selected/candidate/excluded recall trace 与 provenance。
- 只访问自身 MemoryItem/MemoryRecallTrace repositories，不持有其他 module/block 的 service、repository 或 adapter。
- 不覆盖当前 Goal/DoD、StagePacket 或 runtime contract。

## 主要交互

- 被 `kernel/orchestrator` 用于 runtime context assembly、recall trace 和 result memory 写入。
- 被 `blocks/llm` 间接消费为 LLM context 的一部分。
- 被 `application/workorchd` 用于 memory API、workspace snapshot 和 run context bundle。
- 与 `kernel/eventstream` 协作发布 memory/recall 事件。

## 组件交互合同

- Command contract：调用方导入 `blocks/memory/pkg/events`，通过 magicCommon EventHub 投递具体 memory/recall command，并接收 `MemoryCommandResult`。
- Input contract：memory create/update/recall trace command 必须包含 workspace/run/source/provenance 中的必要字段。
- Output contract：recall result 必须区分 selected、candidate、excluded，并保留 reason 与 source refs。
- Orchestrator contract：orchestrator 可通过 EventHub 选择 memory context，但 Memory block 不覆盖 Goal/DoD 或 StagePacket。
- LLM contract：进入 prompt 的 memory 必须 bounded、traceable，并可从 recall trace 反查；LLM block 不直接调用 Memory 实现。
- Projection contract：application 通过 EventHub result/notification 展示 memory，并保留 workspace/run scope 和召回状态。
- Notification contract：Memory created/updated/deleted 使用 `MemoryChangedData`，recall recorded 使用 `MemoryRecallRecordedData`，统一投递到 `/memory/consumers/#`；标签、召回 ID 等字段使用明确切片类型，不通过 Envelope、map/any、JSON 或 reflect 传输。

## 已知缺口

- Memory API、recovery recall 与 EventDrivenDispatch Orchestrator 的生产通知链已完成 typed 收口；为通知额外传入的 legacy `events.Hub` 参数已删除。
- Recall owner 发布职责已收口：`MemoryRecallTrace` upsert command 持久化成功后由 Memory block 直接发布 `MemoryRecallRecordedData`，Orchestrator 不再重复发布 recall Envelope。
- MemoryItem owner 发布职责已收口：通用 create/update/delete command 持久化成功后分别发布 `MemoryChangedData`，delete command 显式携带 WorkspaceID 并在删除前校验 owner scope；application 与 Orchestrator 的外围重复发布路径已删除。
- 需持续验证 rejected diagnostic 与 provider retry 噪声不会进入高价值 memory。

## 边界约束

- 只有高价值、可复用、带 provenance 的信息应进入长期 memory。
- rejected diagnostic、provider retry 噪声和 transient plumbing 信息不能成为高价值 memory。
- memory 不能改变 owner state 或 acceptance 结论。
