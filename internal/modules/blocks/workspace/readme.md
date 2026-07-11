# Workspace Stateful Block

## 功能说明

`blocks/workspace` 管理 Workspace、Goal、DoD 和 WorkspaceTemplate 组成的 workspace-definition 聚合，包括定义版本、激活绑定和模板查询。它只管理该聚合的正式状态，不承担 Run 调度或跨组件治理。

## 组件交互及合同

- Workspace、Goal、DoD 和 Template 的 create/ensure/get/list/update/delete 由 `blocks/workspace/pkg/events` 定义具体 command 和 `WorkspaceCommandResult`。
- Workspace 创建、状态变化、删除以及 Goal/DoD 定义变化由本 block 发布具体通知。
- `kernel/run` 和 `kernel/orchestrator` 通过 EventHub 读取 workspace definition、Goal、DoD、template 和 definition revision。
- `application/workorchd` 通过 owner command/read port 管理和展示 workspace definition。
- `kernel/system` 发送 `EnsureWorkspaceTemplateDefaultsCommand`；本 block 在自己的 repository 内幂等初始化默认模板并返回具体 result。

## 相关约束

- 只访问 Workspace、Goal、DoD 和 WorkspaceTemplate repository，不得持有 Run、Result 或其他 owner repository/service。
- 所有对象操作必须携带 WorkspaceID 或可唯一解析的对象 scope。
- definition revision 必须单调、可比较；激活绑定变化必须使旧定义选择可检测为 stale。
- Workspace block 不启动 Run、不判定 Result、不执行治理恢复。
