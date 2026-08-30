# MCP Registry Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

目录 `blocks/mcpregistry` 对应 ModuleID `blocks/mcp-registry`，是 MCP Server、Capability 及其 binding 的唯一 catalog owner。全部状态保存在一个带 revision 的
`RegistrySnapshot` 聚合中；本组件负责规范化、校验、原子 CAS 提交、一致快照、有界查询和内置默认资源 ensure。

命名映射固定为：目录 `mcpregistry`、Go package `mcp`、ModuleID `blocks/mcp-registry`、EventHub contract owner
`mcp-registry`、topic namespace `/mcp/...`。它与执行能力组件 `blocks/mcp` 不同；Registry 不打开 transport、不解析
secret reference、不执行 operation，也不修改 Runtime aggregate。

## 组件交互及合同

| 交互方 | 方向 | typed EventHub 合同 | 调用/授权语义 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Registry | Server/Capability create/get/list/update、`ReadSelectionSnapshotCommand/Result` | intended caller；当前无 exact source authorization |
| `blocks/mcp` | MCP execution → Registry | `GetServerCommand/Result`、`ReadRegistrySnapshotCommand/Result` | intended caller；当前无 exact source authorization |
| `kernel/system` | System → Registry | `EnsureDefaultsCommand/Result` | 仅接受 exact source `kernel/system` |
| repository backend | Registry → repository | `Load`、`Commit(expectedRevision, snapshot)` | owner-private aggregate CAS |

[`pkg/events`](pkg/events) 为 11 个 topic 定义 11 组 concrete command/result。Server 环境只保存受校验的 reference，
不保存明文 credential。selection snapshot 在一个 revision 内验证 enabled Server、Capability、binding 与 allowlist 关系。

## 不变量与非职责

- 所有 mutation 经过单个 `RegistrySnapshot` CAS；提交失败必须零部分写入，禁止拆成 Server/Capability 两套正式事务。
- 所有 handler 都要求同步 `Result`；EventHub `Post` 不执行 query 或 mutation。
- Server/Capability list、selection、字符串、schema、environment reference 和聚合数量均受 owner 上限约束。
- update 保持 immutable identity；Server allowlist reconciliation 保留用户 metadata/disabled 状态并禁用失效 binding。
- defaults 只补齐缺失 ID，不覆盖已有资源；错误 System source 在 owner mutation 前拒绝。
- Registry 不推断 effect/risk/readiness，不选择 Runtime action；transport 与 reference resolution 归 `blocks/mcp`。

## 能力验收

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| normal create/get/list/update/snapshot/selection | [`module_test.go`](module_test.go)、[`biz/mcp_service_test.go`](biz/mcp_service_test.go) | 覆盖核心路径 |
| invalid bounds、identity、relationship、reference/provenance | `TestServiceListsArePaginatedAndRejectInvalidBounds`、`TestServiceRejectsInvalidLookupAndUpdateCapabilityIDs`、`TestServiceSelectionSnapshotFailsClosedOnServerRelationship`、[`pkg/events/messages_test.go`](pkg/events/messages_test.go) | 覆盖 canonical/UTF-8/byte-bound lookup identity 与核心关系负面路径 |
| atomic failure / no partial write | `TestServiceCommitFailureLeavesNoPartialAggregate` | 覆盖通用 commit failure |
| defaults / idempotency | [`biz/defaults_test.go`](biz/defaults_test.go) | memory/file 覆盖 |
| System exact source | `TestMCPRegistryEnsureDefaultsRequiresExactSystemSource` | 覆盖 |
| Post effect guard | `TestModulePostCannotMutateRegistry` | 覆盖 |
| aggregate CAS / memory/file persistence | `TestMCPRegistryRepositoryContract_Memory`、`TestMCPRegistryRepositoryContract_File` in [`internal/pkg/repository/repository_contract_test.go`](../../../pkg/repository/repository_contract_test.go) | 覆盖 repository fence |
| duplicate / CAS conflict | `TestMCPRegistryRepositoryContract_Memory`、`TestMCPRegistryRepositoryContract_File` | 覆盖 repository expected-revision conflict；service CAS loop 的 conflict-then-success 与 conflict exhaustion 尚无注入验收 |
| stale / freshness | `TestServiceServerReconciliationPreservesEditsAndDisablesStaleBindings` | 这里的 stale 只指失效 binding reconciliation；公开 command 没有 caller-supplied expected revision 或独立 stale/freshness outcome，因此该语义当前未定义 |
| restart/recovery | file aggregate reload 由 repository contract 覆盖 | owner restart 后一致 snapshot 尚无独立 EventHub 验收 |
| lifecycle | `TestModuleIdentityConfigureStartStopContract` | 覆盖基本 Start/Stop；尚无 post-Stop delivery 断言 |
| caller authorization | 仅 defaults 有 exact System gate | Application/MCP caller source 未授权化 |
| provider / remote transport | 不适用：归 `blocks/mcp` | 不适用 |
