# Soul Registry Stateful Block

## 功能定位

`blocks/soul` 是 Soul catalog、stage policy refs、capability/risk constraints 元数据的单一资源聚合 block。它管理结构化策略元数据与查询，但不承担 runtime 调度或跨组件策略编排。

## 拥有边界

- 拥有 Soul catalog 记录与 owner command/query。
- 只访问自身 Soul repository，不持有其他 module/block 的 service、repository 或 adapter。
- 维护 stage policy refs、capability constraints、risk constraints 等结构化元数据。
- 不直接调度 runtime block，也不决定 Run/Result 状态。

## 主要交互

- 被 `blocks/workspace` 或 workspace template/profile metadata 绑定到 workspace/run context。
- 被 `kernel/orchestrator` 作为中立 stage policy refs 读取。
- 被 `application/workorchd` context registry、Web/API 读写。
- 与 `blocks/skill` 一起参与 LLM context 的指导层。
- 订阅 System 定义的 `/system/command/ensure-soul-defaults`，在 Soul owner 内幂等加载内置 Soul。

## 组件交互合同

- Command contract：调用方导入 `blocks/soul/pkg/events`，通过 magicCommon EventHub 投递具体 Soul command/query，并接收 `SoulCommandResult`。
- Input contract：soul command 必须使用结构化 soulId、stage policy refs、constraints 和 metadata。
- Output contract：selected soul/stage policy result 必须包含 refs、约束字段和 provenance。
- Workspace contract：Workspace block/template 只能绑定 soul refs，不复制 Soul 内部实现为 owner state。
- Orchestrator contract：orchestrator 通过 EventHub 与中立 constraints 消费 Soul，不按 Soul ID 或展示文本分支。
- LLM contract：Soul 指导进入 prompt 前必须 bounded，并保持与当前 stage contract 不冲突；LLM block 不直接调用 Soul 实现。
- System defaults contract：只消费 `kernel/system/pkg/events` 定义的 `EnsureSoulDefaultsCommand` 并返回 `EnsureSoulDefaultsResult`；System 不取得 Soul repository。

## 已知缺口

- 当前盘点未发现 Soul block 职责、状态所有权或 EventHub 合同缺失；后续需要继续验证 stage policy refs 与 runtime requiredOutput 的优先级关系。

## 边界约束

- core runtime 不按 Soul ID 或显示文本分支。
- Soul 约束只能通过结构化中立字段消费。
- Soul guidance 不能覆盖当前 Goal/DoD 或 requiredOutput。
