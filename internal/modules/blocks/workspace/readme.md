# Workspace Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-17

## 功能范围与边界

`blocks/workspace` 管理 Workspace、Goal、DoD 和 WorkspaceTemplate 组成的 workspace-definition 聚合，包括定义版本、激活绑定和模板查询。它只管理该聚合的正式状态，不承担 Run 调度或跨组件治理。

## 组件交互及合同

- Workspace、Goal、DoD 和 Template 的实际 CRUD command，以及 Template defaults ensure command，由
  `blocks/workspace/pkg/events` 分别定义具体类型；不存在覆盖四类资源的通用 CRUD envelope。
- Workspace 创建、状态变化、删除以及 Goal/DoD 定义变化由本 block 发布具体通知。
- `application/workorchd` 与需要 definition fact 的 owner 通过 EventHub 读取 Workspace、Goal、DoD、Template 和
  definition revision；其他 owner 不直接访问 Workspace repository。
- `application/workorchd` 通过 owner command/read port 管理和展示 workspace definition。
- `kernel/system` 发送 `EnsureWorkspaceTemplateDefaultsCommand`；本 block 在自己的 repository 内幂等初始化默认模板并返回具体 result。
- `WorkspaceTemplate` 是 workspace owner 的 DTO；EventHub result 只返回该 owner DTO，并在 module 内部完成与 repository entity 的双向 mapping、slice/map/payload clone。repository 类型不得泄漏给 application 或其他 owner。
- 外部定义命令保持唯一入口 `handleCommand -> executeCommand`；Owner 内部按 Workspace、Goal、DoD、Template
  四个命令族做穷尽且 fail-closed 的静态分派。各 handler 只是同一 Biz 内的职责切片，不是新的 Owner、状态机、
  repository/CAS 边界或 EventHub facade。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Workspace | Workspace/Goal/DoD/Template CRUD、definition read 与 execution intent ingress | 完整 |
| `kernel/system` | System → Workspace | `EnsureWorkspaceTemplateDefaultsCommand` | 完整 |
| `blocks/run` | 双向 | scoped execution intent；`RunStateChangedData`；sealed terminal delivery | 完整；状态转换分别留在各自 owner |
| `kernel/orchestrator` | consumer → Workspace | typed definition/activation fact 读取，不访问 repository | 完整 |
| `blocks/eventstream` | Workspace → Eventstream | create/state/delete、Goal/DoD definition 通知 | 完整 |

## 相关约束

- 只访问 Workspace、Goal、DoD 和 WorkspaceTemplate repository，不得持有 Run、Result 或其他 owner repository/service。
- 所有对象操作必须携带 WorkspaceID 或可唯一解析的对象 scope。
- definition revision 必须单调、可比较；激活绑定变化必须使旧定义选择可检测为 stale。
- Workspace block 不驱动 Runtime、不判定 Result、不执行治理恢复；execution-intent ingress 只校验当前
  Workspace/Goal/DoD、选择 active Run，并向 Run owner 发送 concrete typed intent。它消费 scoped sealed terminal outcome，按
  outcome identity 在自身 reducer 中释放 `ActiveRunID` 并维护 Workspace state projection。它不读取
  `ConvergenceState`、RuntimeAction、focus、repair 或 error message 重新裁决；delivery feedback 保留 stable
  category 与完整安全 original diagnostic。
- `fresh_activation_required` 只是 Run owner 的不可继续 disposition；普通 Run notification 不释放 Workspace slot。
  对不兼容 development data 的 restart，Run owner返回 `data_reset_required`，Workspace 不代替 Run 创建 successor、
  不读取旧 aggregate，也不通过旁路 fact 清除 active slot。

## 待完善测试点

- [ ] 覆盖 Workspace/Goal/DoD 每类定义变更对 definition revision 的单调推进和 stale activation 检测。
- [ ] 覆盖空工作目录的默认解析、相对/绝对路径规范化、目录逃逸和重复 workspace 隔离。
- [ ] 覆盖 execution intent 的缺失 Goal/DoD、跨 scope、重复投递与 active Run 冲突。
- [ ] 覆盖 sealed terminal outcome 重复/冲突投递，以及仅匹配 outcome identity 才释放 `ActiveRunID`。
- [ ] 覆盖普通 Run notification、`fresh_activation_required`、`data_reset_required` 均不能旁路释放 slot。
- [ ] 覆盖 Template defaults 幂等非覆盖和 Template DTO 中 slice/map/payload 的深拷贝。
