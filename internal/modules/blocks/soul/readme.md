# Soul Registry Stateful Block

## 功能说明

`blocks/soul` 管理 Soul catalog 及其 stage policy refs、capability constraints、risk constraints 和 provenance，提供结构化策略元数据的 owner command/query。

## 组件交互及合同

- Soul 的 create/get/list/update/delete 和 capability validation 由 `blocks/soul/pkg/events` 定义，并返回 `SoulCommandResult`。
- `application/workorchd` 通过 EventHub 管理 Soul；Workspace/profile 只绑定 Soul ref。
- `kernel/orchestrator` 通过 EventHub 获取 Soul 的结构化中立约束，并将有界指导组织进 stage context。
- `kernel/system` 发送 `EnsureSoulDefaultsCommand`；本 block 在自己的 repository 内幂等初始化默认 Soul。

## 相关约束

- 本 block 只访问 Soul repository，不调度 runtime block、不修改 Run 或 Result 状态。
- core runtime 不得按 Soul ID、名称或显示文本写死业务分支。
- Soul 约束必须使用结构化中立字段，并保持 provenance。
- Soul guidance 不得覆盖当前 Goal、DoD、stage contract、runtime policy 或 requiredOutput。
