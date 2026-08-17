# Soul Registry Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-17

## 功能范围与边界

`blocks/soul` 管理 Soul catalog 及其 stage policy refs、capability constraints、risk constraints 和 provenance，提供结构化策略元数据的 owner command/query。

## 组件交互及合同

- Soul 的 create/get/list/update/delete 和 capability validation 由 `blocks/soul/pkg/events` 定义，并返回 `SoulCommandResult`。
- `application/workorchd` 通过 EventHub 管理 Soul；Workspace/profile 只绑定 Soul ref。
- fresh activation 由 `application/workorchd` 读取并封存 Soul 的结构化中立约束；`kernel/orchestrator` 只消费该 typed snapshot，不直接查询 Soul owner。
- `kernel/system` 发送 `EnsureSoulDefaultsCommand`；本 block 在自己的 repository 内幂等初始化默认 Soul。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Soul | create/get/list/update/delete、capability validation | 完整 |
| `kernel/system` | System → Soul | `EnsureSoulDefaultsCommand` 及具体 result | 完整 |
| Workspace activation | Application → Run bootstrap | detached stage Soul 内容与结构化约束 | 完整；Orchestrator 无直接 Registry I/O |

## 相关约束

- 本 block 只访问 Soul repository，不调度 runtime block、不修改 Run 或 Result 状态。
- core runtime 不得按 Soul ID、名称或显示文本写死业务分支。
- Soul 约束必须使用结构化中立字段，并保持 provenance。
- Soul guidance 不得覆盖当前 Goal、DoD、stage contract、runtime policy 或 requiredOutput。

## 待完善测试点

- [ ] 覆盖默认 Soul 重复 ensure、用户已修改 Soul 和部分默认值已存在时的非覆盖幂等语义。
- [ ] 覆盖 capability validation 的未知 capability、冲突约束和空 stage binding。
- [ ] 覆盖 create/update/get/list 的深拷贝，尤其是 stage policy、capability/risk constraint 和 provenance slice。
- [ ] 增加 activation 边界测试，确认 Orchestrator 不直接查询 Soul，且 sealed Soul snapshot 不受后续 Registry 更新影响。
