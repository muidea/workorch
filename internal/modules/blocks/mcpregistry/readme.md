# MCP Registry Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-20

## 功能范围与边界

`blocks/mcpregistry` 是 MCP Server 定义与 Capability catalog 的唯一正式状态 owner。Server、Capability 及两者的绑定关系保存在一个带 revision 的 `RegistrySnapshot` 聚合中；本 block 负责规范化、校验、原子 CAS 更新、一致快照和有界查询，但不打开 MCP transport、不执行 operation，也不修改 Runtime aggregate。

## 组件交互及合同

- `blocks/mcpregistry/pkg/events` 为每个 create/get/list/update、defaults、完整快照和选择快照 topic 定义独立 command/result；不存在通用 `MCPCommandResult` 联合 envelope。所有同步 handler 在 `result == nil` 时直接返回，因此 EventHub `Post` 不能执行查询或强写。
- Server command/result 只携带 `EnvironmentReference{Name, Reference}`。允许的引用为 `env://ENV_NAME`、固定 workspace/run/step/directory 占位符或经校验的 `{workspaceManagedConfig:relative/path}`；明文 secret、credential 和任意字符串在进入 owner 状态前即被拒绝。引用值只由 `blocks/mcp` 在执行边界解析。
- Server 与 Capability 通过单个 `MCPRegistryRepository.Load/Commit(expectedRevision)` 聚合合同持久化。一次 server update、allowlist reconciliation、Capability create/update 或 defaults ensure 要么整体提交，要么零提交；memory/file backend 均执行相同 CAS 语义。
- `ReadRegistrySnapshotCommand` 返回同一 revision 的完整 bounded Server/Capability catalog，供 `blocks/mcp` 编译 execution catalog；`ReadSelectionSnapshotCommand` 在同一 revision 内校验选中的 enabled Capability 必须属于选中的 enabled Server 且仍位于对应 allowlist，供 fresh activation 使用。
- Server/Capability list 使用 `cursorAfterID + limit`，owner 对 cursor/query 字节数、枚举值和 limit 执行上限校验。Application 跨页读取必须保持 `RegistryRevision` 不变并检查 cursor 前进；revision 漂移或不前进均 fail-closed。
- Capability create/update command 只携带字段化配置意图。owner 签发并维护 `Source`、`SourceRef` 和单调 `Revision`；update 不允许改变已存在 Capability 的 ServerID、kind 或 name。
- `Enabled` 是唯一可用性权威，不存在独立 `draft/active/deprecated Status`。Capability 只有在 Server enabled、Capability enabled 且 Server allowlist 含其精确 binding 时才可进入选择快照；非法或陈旧 binding 不可调用。
- Server allowlist 更新会为新增 binding 创建中立的 `server_allowlist` metadata，保留已有 binding 的用户 metadata/禁用状态，并禁用已删除或因 Server 禁用而失效的 Capability；不会把用户禁用项重新启用。
- `kernel/system` 发送 owner-local `EnsureDefaultsCommand`。Registry 在同一聚合事务内补齐缺失默认值，完整复用普通规范化/校验路径；内置 ID 或 binding 已存在时保持当前对象，不以启动 ensure 覆盖用户配置。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Registry | concrete Server/Capability CRUD、paged list、`ReadSelectionSnapshotCommand/Result` | 完整；管理读取检测跨页 revision，activation 使用单次一致选择快照 |
| `blocks/mcp` | MCP execution → Registry | `GetServerCommand/Result`、`ReadRegistrySnapshotCommand/Result` | 完整；只取得 detached catalog，不取得 repository，secret 引用在执行边界解析 |
| `kernel/system` | System → Registry | `EnsureDefaultsCommand` / `EnsureDefaultsResult` | 完整；原子、幂等且不覆盖已有资源 |
| `kernel/orchestrator` | 无直接 Registry I/O | 只消费 sealed activation 与 MCP execution owner facts | 完整 |

## 相关约束

- 本 block 只依赖一个 MCP Registry 聚合 repository；不得重新拆成独立 Server/Capability 写事务或向调用方暴露 repository/service。
- Registry 上限为 256 个 Server、4096 个 Capability；单页默认 100、最大 200，selection 最多 64 个 Server/256 个 Capability。ID、查询、schema、字符串列表和 environment reference 均有 owner 上限。
- `effectScope`、`idempotencyClass` 和 `direct | auxiliary | not_declared` validation proof applicability 必须是合法枚举；空值规范化为显式 `unknown`/`not_declared`，非法值不允许静默降级。Registry 不从名称、category、risk 或描述推断这些权威元数据。
- `ready | prerequisite_required | unavailable` 仍由 `blocks/mcp` 基于同一次 detached snapshot 与 sealed input 编译；Registry 不执行 probe、不生成 input binding、不选择 Runtime action。
- file backend 首次读取新聚合文件前可导入旧 Server/Capability 文件；旧 Capability 会取得 `legacy_import` provenance。旧 Server 若含明文 environment value，会在 owner 规范化阶段 fail-closed，必须迁移成引用后才能发布快照。
- 默认资源是可编辑的首次启动种子。强制刷新必须另行定义显式、版本化、可审计的 owner migration 合同。

## 测试状态

- [x] memory/file 聚合 CAS、深拷贝、file reload、旧文件导入及 revision conflict。
- [x] create/update/defaults 的原子提交；注入 Commit 失败后无部分 Server/Capability，重试可收敛。
- [x] plaintext environment value、非法/重复 reference、执行期缺失 env ref 均 fail-closed；EventHub DTO 与执行期解析有回归。
- [x] `Post` 不能修改 Registry；各 topic 使用 concrete result，一致完整/选择快照返回同一 revision。
- [x] Capability binding、immutable identity、Server/allowlist/enabled 关系、metadata 枚举、provenance/revision 与 stale reconciliation。
- [x] cursor/limit/query 上限、分页前进与 Application 跨页 revision 检查。
- [x] defaults 在 memory/file backend 原子幂等，保留内置 ID 上的用户 Server/Capability 配置。
- [x] Registry/MCP execution/Application/Orchestrator repository 与 transport 边界由静态合同扫描保护。

当前没有与本次评审合同对应的已知可执行缺口。
