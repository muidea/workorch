# Workspace Stateful Block

## 功能说明

`blocks/workspace` 管理 Workspace、Goal、DoD 和 WorkspaceTemplate 组成的 workspace-definition 聚合，包括定义版本、激活绑定和模板查询。它只管理该聚合的正式状态，不承担 Run 调度或跨组件治理。

## 组件交互及合同

- Workspace、Goal、DoD 和 Template 的 create/ensure/get/list/update/delete 由 `blocks/workspace/pkg/events` 定义具体 command 和 `WorkspaceCommandResult`。
- Workspace 创建、状态变化、删除以及 Goal/DoD 定义变化由本 block 发布具体通知。
- `blocks/run` 和 `kernel/orchestrator` 通过 EventHub 读取 workspace definition、Goal、DoD、template 和 definition revision。
- `application/workorchd` 通过 owner command/read port 管理和展示 workspace definition。
- `kernel/system` 发送 `EnsureWorkspaceTemplateDefaultsCommand`；本 block 在自己的 repository 内幂等初始化默认模板并返回具体 result。
- `WorkspaceTemplate` 是 workspace owner 的 DTO；EventHub result 只返回该 owner DTO，并在 module 内部完成与 repository entity 的双向 mapping、slice/map/payload clone。repository 类型不得泄漏给 application 或其他 owner。

## 相关约束

- 只访问 Workspace、Goal、DoD 和 WorkspaceTemplate repository，不得持有 Run、Result 或其他 owner repository/service。
- 所有对象操作必须携带 WorkspaceID 或可唯一解析的对象 scope。
- definition revision 必须单调、可比较；激活绑定变化必须使旧定义选择可检测为 stale。
- Workspace block 不启动 Run、不判定 Result、不执行治理恢复；它只消费 scoped sealed terminal outcome，按
  outcome identity 在自身 reducer 中释放 `ActiveRunID` 并维护 Workspace state projection。它不读取
  `ConvergenceState`、RuntimeAction、focus、repair 或 error message 重新裁决；delivery feedback 保留 stable
  category 与完整安全 original diagnostic。
- 对已经持久化为 `fresh_activation_required` 的 Run，Workspace 还可消费带 Run owner scope、opaque runtime ref/
  revision 与 actual/supported schema 的 `ReleaseFreshActivationRunSlotFactCommand`。该 reducer 只在
  `ActiveRunID == RunID` 时清空自身 slot 并保持 Workspace 可激活；空 slot 与迟到旧 fact 都是 no-op，绝不能清空
  新 Run。它不改变旧 Run state、不产生 terminal delivery，也不读取 Runtime aggregate。普通 Run notification
  绝不能自动释放该 slot；只有用户明确 restart 的 application forwarder 可以同步提交同一 typed fact。
