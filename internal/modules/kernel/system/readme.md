# System Module

Status: active

Type: kernel-module-contract

Last Verified: 2026-08-17

## 功能范围与边界

`kernel/system` 在应用启动阶段协调 Soul、WorkspaceTemplate 和 MCP Server/Capability 默认资源的幂等初始化。它不保存这些资源的正式状态，只负责跨 owner 的初始化顺序和失败收敛。

## 组件交互及合同

- `EnsureSoulDefaultsCommand`、`EnsureWorkspaceTemplateDefaultsCommand`、`EnsureMCPDefaultsCommand` 及对应 result
  分别由 Soul、Workspace 和 MCP Registry owner 在自己的 `pkg/events` 中定义；System 只导入并发送这些合同。
- `blocks/soul`、`blocks/workspace` 和 `blocks/mcpregistry` 分别订阅自己的 command，并只访问自己的 repository 完成幂等 ensure。
- System 在 owner 订阅启动后发送同步 EventHub command；缺少 response、返回类型错误或 owner 失败都必须使启动失败。
- System 只发送具体 typed ensure command；默认资源内容、模型校验、去重和持久化均由对应 owner 负责。
- System Biz 只保留接受 `Dependencies` 的一个构造入口；Module 配置不得再经无校验 constructor forwarding。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `blocks/soul` | System → Soul | `EnsureSoulDefaultsCommand` 及具体 result | 完整 |
| `blocks/workspace` | System → Workspace | `EnsureWorkspaceTemplateDefaultsCommand` 及具体 result | 完整 |
| `blocks/mcpregistry` | System → MCP Registry | `EnsureMCPDefaultsCommand` 及具体 result | 完整 |

三组同步合同均由资源 owner 定义。System 不定义通用 ensure envelope，也不读取任何 owner repository。

## 相关约束

- System 不得直接取得 Soul、Workspace 或 MCP repository/service。
- 默认初始化必须幂等，不能覆盖用户已存在的有效资源。
- System 只承担启动协调，不负责运行期业务治理、Run 调度或状态修复。
- 新增默认资源 owner 时必须增加具体 command/result、生命周期顺序和失败测试，不能增加 repository 直连。

## 待完善测试点

- [ ] 分别覆盖三类 owner 缺少 subscriber、返回 nil/wrong result 和返回 owner error，确认启动立即失败。
- [ ] 覆盖第一或第二组 ensure 成功、后续 ensure 失败的部分启动，确认重启后只靠 owner 幂等语义收敛。
- [ ] 覆盖重复启动与重复 ensure，确认已有用户资源不被默认值覆盖。
- [ ] 增加合同清单测试，确保新增默认资源必须显式增加 owner command/result，不能退化为通用 payload。
