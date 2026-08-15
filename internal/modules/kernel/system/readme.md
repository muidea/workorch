# System Module

## 功能说明

`kernel/system` 在应用启动阶段协调 Soul、WorkspaceTemplate 和 MCP Server/Capability 默认资源的幂等初始化。它不保存这些资源的正式状态，只负责跨 owner 的初始化顺序和失败收敛。

## 组件交互及合同

- System 定义 `EnsureSoulDefaultsCommand`、`EnsureWorkspaceTemplateDefaultsCommand`、`EnsureMCPDefaultsCommand` 及对应 result。
- `blocks/soul`、`blocks/workspace` 和 `blocks/mcpregistry` 分别订阅对应 command，并只访问自己的 repository 完成幂等 ensure。
- System 在 owner 订阅启动后发送同步 EventHub command；缺少 response、返回类型错误或 owner 失败都必须使启动失败。
- System 只发送具体 typed ensure command；默认资源内容、模型校验、去重和持久化均由对应 owner 负责。
- System Biz 只保留接受 `Dependencies` 的一个构造入口；Module 配置不得再经无校验 constructor forwarding。

## 相关约束

- System 不得直接取得 Soul、Workspace 或 MCP repository/service。
- 默认初始化必须幂等，不能覆盖用户已存在的有效资源。
- System 只承担启动协调，不负责运行期业务治理、Run 调度或状态修复。
- 新增默认资源 owner 时必须增加具体 command/result、生命周期顺序和失败测试，不能增加 repository 直连。
