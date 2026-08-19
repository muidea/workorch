# System Module

Status: active

Type: kernel-module-contract

Last Verified: 2026-08-20

## 功能范围与边界

`kernel/system` 在应用启动阶段协调 Soul、WorkspaceTemplate 和 MCP Server/Capability 默认资源的幂等初始化。它不保存这些资源的正式状态，只负责跨 owner 的初始化顺序和失败收敛。内置资源是首次启动种子，不是由 System 在每次启动时强制刷新的系统托管对象。

## 组件交互及合同

- Soul 和 MCP Registry 各自在自己的 `pkg/events` 中定义 owner-local `EnsureDefaultsCommand` / `EnsureDefaultsResult`；
  Workspace 定义 `EnsureWorkspaceTemplateDefaultsCommand` / `EnsureWorkspaceTemplateDefaultsResult`。System 只导入并发送
  这三组具体合同，不定义通用 ensure 类型。
- `blocks/soul`、`blocks/workspace` 和 `blocks/mcpregistry` 分别订阅自己的 command，并只访问自己的 repository 完成幂等 ensure；MCP Server/Capability 在一个 Registry 聚合 CAS 中原子补齐。
- System 在 owner 订阅启动后发送同步 EventHub command；缺少 response、返回类型错误或 owner 失败都必须使启动失败。
- System 只发送具体 typed ensure command；默认资源内容、模型校验、去重和持久化均由对应 owner 负责。
- 三组 ensure 按 Soul → WorkspaceTemplate → MCP Registry 顺序执行并 fail-fast。部分成功不产生 System 状态；重启或重试从第一组重新发送，由各 owner 的 create-if-missing 语义收敛。
- System Biz 只保留接受 `Dependencies` 的一个构造入口；Module 配置不得再经无校验 constructor forwarding。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `blocks/soul` | System → Soul | `soul/pkg/events.EnsureDefaultsCommand` / `EnsureDefaultsResult` | 完整 |
| `blocks/workspace` | System → Workspace | `workspace/pkg/events.EnsureWorkspaceTemplateDefaultsCommand` / `EnsureWorkspaceTemplateDefaultsResult` | 完整 |
| `blocks/mcpregistry` | System → MCP Registry | `mcpregistry/pkg/events.EnsureDefaultsCommand` / `EnsureDefaultsResult` | 完整 |

三组同步合同均由资源 owner 定义。System 不定义通用 ensure envelope，也不读取任何 owner repository。

## 相关约束

- System 不得直接取得 Soul、Workspace 或 MCP repository/service。
- owner ensure 只对缺失的内置 ID 执行 Create。内置 ID 已存在时必须完整保留当前持久化对象；用户新增的非内置 ID 也不受影响。删除内置资源后，下次 ensure 会按当前嵌入默认值重新补建。
- 嵌入默认值的版本变化不得借普通 ensure 覆盖已有对象；若未来需要强制迁移，必须新增显式、可版本化且可审计的 owner migration 合同。
- System 只承担启动协调，不负责运行期业务治理、Run 调度或状态修复。
- 新增默认资源 owner 时必须增加具体 command/result、生命周期顺序和失败测试，不能增加 repository 直连。

## 已覆盖测试点

- [x] 三类 owner 的 missing subscriber、nil result、wrong result、`Ensured=false` 和 owner error 均使启动立即失败；System 只发送对应 owner 的具体 command/result。
- [x] Soul 与 WorkspaceTemplate ensure 成功、MCP Registry 缺席后的部分启动可在 owner 就绪后重试收敛。
- [x] owner 级 memory/file backend 的重复 ensure、部分默认值已存在及 System 重复 Start 均保留内置 ID 上的用户修改，同时补齐缺失默认资源。

## 待完善测试点

当前没有与本合同对应的可执行缺口。System 层只观察 EventHub command/result，因此部分启动与重试集成测试无需按 repository backend 重复；create-if-missing 与非覆盖语义已在各 owner 上同时覆盖 memory/file backend。同一 owner 的 state-lane command 由 EventHub 串行处理，repository Create 冲突仍要求二次读取到资源后才能收敛为成功。
