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
| `application/workorchd` | Application → Registry | Server/Capability create/get/list/update、`ReadSelectionSnapshotCommand/Result` | 按 topic 接受 exact context-registry、appservice 或 readmodel publisher identity |
| `blocks/mcp` | MCP execution → Registry | `GetServerCommand/Result`、`ReadRegistrySnapshotCommand/Result` | 仅对应 query 接受 exact `blocks/mcp` source |
| `kernel/system` | System → Registry | `EnsureDefaultsCommand/Result` | 仅接受 exact source `kernel/system` |
| repository backend | Registry → repository | `Load`、`Commit(expectedRevision, snapshot)` | owner-private aggregate CAS |

[`pkg/events`](pkg/events) 为 11 个 topic 定义 11 组 concrete command/result。Server 环境只保存受校验的 reference，
不保存明文 credential。selection snapshot 在一个 revision 内验证 enabled Server、Capability、binding 与 allowlist 关系。

## 不变量与非职责

- 所有 mutation 经过单个 `RegistrySnapshot` CAS；提交失败必须零部分写入，禁止拆成 Server/Capability 两套正式事务。
- 同一 catalog ID 的 production mutation 使用 owner contract 派生的 state lane 串行；不同 ID 的并发 mutation 由
  aggregate CAS conflict 后 reload/rebase 合并。最后成功提交的 replacement command 为当前值，exact replay 为 no-op。
- caller-supplied `ExpectedRevision` 不适用：Registry 是原子 catalog aggregate，不是 caller 驱动的 read-modify-write
  lifecycle/state machine；返回的 `RegistryRevision` 是一致快照 identity，不是 transition precondition。duplicate、invalid、
  CAS exhaustion 或 repository failure 均返回错误且不产生部分 aggregate。
- 所有 handler 都要求同步 `Result`；EventHub `Post` 不执行 query 或 mutation。
- Server/Capability list、selection、字符串、schema、environment reference 和聚合数量均受 owner 上限约束。
- update 保持 immutable identity；Server allowlist reconciliation 保留用户 metadata/disabled 状态并禁用失效 binding。
- defaults 只补齐缺失 ID，不覆盖已有资源；错误 System source 在 owner mutation 前拒绝。
- 所有 Application/MCP/System ingress 先按 topic 校验 exact publisher source，再执行 load、query 或 CAS；不存在 prefix 或
  trim 后授权。
- Registry 不推断 effect/risk/readiness，不选择 Runtime action；transport 与 reference resolution 归 `blocks/mcp`。

### 生命周期合同

- `Start` 逐成功订阅记账；部分失败保留 topic 和 service，未清理前拒绝再次注册。
- `BeginShutdown` 非等待式关闭 create/update/ensure；Get/List、ReadRegistrySnapshot 与 ReadSelectionSnapshot 保留原 typed/source 校验，作为既有事务的一致读取 drain，不授权新执行。
- 准入票追踪真实 handler 返回。`Quiesce(ctx)` 超时不清依赖，允许新预算重试；取消等待不等于工作退出。
- `Stop` 先排空、封最终准入再复核，逆序 checked 退订成功才移除账本；全部成功才释放 service/Module 引用。未知 panic 不吞掉，不遗留清理锁。
- 正式最终释放仍依赖共享任务与 Hub 屏障，本地 Quiesce 不能替代它。Configure/Start/Stop 由调用方串行执行，不覆盖未释放 owner；plugin guard 保留原 cause，SM-1～SM-4 业务权威不变。
- 回归入口：`biz/owner_lifecycle_test.go`、`module_lifecycle_test.go`，覆盖逐项失败、真实执行超时重试、fresh/drain 与 guard cause。

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
| duplicate / CAS conflict | `TestMCPRegistryRepositoryContract_Memory`、`TestMCPRegistryRepositoryContract_File`、`TestServiceRetriesCASConflictAndStopsAtBound` | 覆盖 repository fence，以及 service 在其他 writer 推进 revision/content 后 reload/rebase 并保留并发内容、八次 conflict exhaustion 与零部分写入 |
| freshness / committed order | `TestModuleMCPRegistryStateLaneSerializesConcurrentUpdatesLatestCommitWins`、`TestServiceRetriesCASConflictAndStopsAtBound`、`TestServiceServerReconciliationPreservesEditsAndDisablesStaleBindings` | 不适用 caller-supplied expected revision；覆盖同 ID state-lane 排队、latest committed wins、exact replay 零 commit，以及不同 writer 推进 aggregate 后 CAS reload/rebase；binding stale 仅指 allowlist reconciliation |
| restart/recovery | `TestModuleFileOwnerRestartReturnsConsistentEventHubSnapshot` | 覆盖 fresh owner、fresh Hub、fresh file repository handle 返回相同 revision 的一致 concrete snapshot |
| lifecycle | `TestModuleIdentityConfigureStartStopContract`、`TestModuleStopFencesSubsequentMCPRegistryDelivery` | 覆盖重复 Start/Stop、11-topic subscribe/unsubscribe 平衡，以及独立的 post-Stop delivery 零写入 fence |
| caller authorization | `TestMCPRegistryEnsureDefaultsRequiresExactSystemSource`、`TestMCPRegistryCommandSourceAuthorizationMatrix`、`TestModuleRejectsForgedMCPRegistryMutationWithoutOwnerIOEffect` | 覆盖 11 个 topic 的 Application/MCP/System exact source allowlist，以及 forged/noncanonical mutation 零写入 |
| provider / remote transport | 不适用：归 `blocks/mcp` | 不适用 |
