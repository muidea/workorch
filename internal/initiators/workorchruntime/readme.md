# Repository Provider Initiator

Status: active

Type: initiator-contract

Last Verified: 2026-08-30

## 功能范围与边界

`internal/initiators/workorchruntime` 注册为 `workorch/repositories`。它根据进程配置创建 memory 或 file repository，并以
owner 分组的 `RepositoryProvider` 暴露仓储基础设施。

本 Initiator 只负责 repository 的构造、发布和生命周期撤销。它不拥有 EventHub topic、业务状态、Runtime 策略、默认业务
数据、跨 owner 编排或任何状态机转换。

## 组件交互及合同

| 交互方 | 方向 | 合同 |
| --- | --- | --- |
| `application/workorch` | 应用 → Initiator | framework 注册及 `Setup/Run/Teardown` 生命周期；只有应用入口启用组件 |
| `internal/pkg/workorchdeps` | owner → Initiator | typed `RepositoryConsumerID` 到 owner repository 分组的唯一生产解析入口 |
| Module / Block | consumer → `workorchdeps` | 只能取得自身职责对应的 repository 分组，不能取得内部 `repositorySet` 或 sibling owner 分组 |
| EventHub | 无 | 不定义、不订阅、不发送任何业务 Command/Result/Event |

`Workspace`、`Decision`、`Approval`、`EventStream`、`Governance`、`MCPRegistry`、`Memory`、`Orchestrator`、
`Result`、`Run`、`Skill` 与 `Soul` repository 分组均由这里构造；MCP Registry 只暴露单一 aggregate CAS repository。

## 不变量与非职责

- `Setup` 仅在未发布 provider 且不处于 closing 时准备；重复 Setup 返回冲突并保留当前 provider，不撤销仍被消费者使用的引用。首次配置读取或构造失败保持不可用。
- `RepositoryProvider` 在成功 `Setup` 前、首次失败 `Setup` 后及最终 `Teardown` 后返回 `nil`；BeginShutdown／Quiesce 和失败停止期间保留原 provider。
- `BeginShutdown(context.Context)` 只关闭重复准备，不等待、不释放；Initiator 无自有任务，`Quiesce` 检查调用预算但不冒充全体 owner／共享队列排空。最终 Teardown 必须由进程协调者在 owner、BackgroundRoutine 和 Hub 屏障成功后调用，取消预算通过 framework guard 返回原 cause 并保留引用。成功释放后允许建立新 provider。
- `Run` 不创建业务状态；`Teardown` 只撤销 provider 引用，并允许重复调用。
- Initiator 不保存 EventHub、BackgroundRoutine、Base Biz、runtime policy、owner service 或业务 facade。
- 生产代码不得绕过 `workorchdeps` 直接调用 `RepositoryProvider()`；consumer identity 与 repository family 必须精确匹配。
- file backend 的 `Setup` 只创建惰性 repository 句柄；存储路径 I/O 错误由执行该 I/O 的 repository 返回。

## 能力验收

| 验收维度 | 自动化证据 | 状态 |
| --- | --- | --- |
| normal | `TestRuntimeInitiatorMemoryRepositoriesCoverOwnerContracts`、`TestRuntimeInitiatorFileRepositoriesCoverOwnerContracts`、`TestRuntimeInitiatorSetupPublishesCompleteProviderOnlyOnSuccess` | 已覆盖 memory/file 当前 12 个 repository family 的全部公开子接口、owner aggregate `Validate` 与成功发布；不等价于各 repository 的 CRUD/CAS 行为合同 |
| invalid + scope | `TestRuntimeInitiatorSetupFailureLeavesRepositoriesEmpty`、`TestRuntimeInitiatorRejectsUnsupportedRepositoryBackend` | 已覆盖非法配置、未知 backend 与失败后 fail-closed |
| duplicate + idempotency | `TestRuntimeInitiatorRunIsIdempotent`、`TestRuntimeInitiatorTeardownClearsRuntimeResources` | 已覆盖重复 Run/Teardown；成功 Setup 本身不承诺复用同一 provider identity |
| stale + freshness | `TestRuntimeInitiatorSetupFailureLeavesRepositoriesEmpty` | 仅适用为重复 Setup 不泄漏旧 provider；无业务 freshness 语义 |
| restart / owner 重建 | `TestRuntimeInitiatorSetupAfterTeardownPublishesFreshProvider` | 已覆盖 Setup→Teardown→Setup 发布新 repository set |
| lifecycle | `TestRuntimeInitiatorIdentityAndZeroStateContract`、`TestRuntimeInitiatorSetupAfterTeardownPublishesFreshProvider`、`TestRuntimeInitiatorRunIsIdempotent`、`TestRuntimeInitiatorTeardownClearsRuntimeResources` | 已覆盖进程内 lifecycle；不验证 file repository 的进程重启数据恢复 |
| effect + receipt | 不适用 | Initiator 不执行外部业务 effect，也不产生 Runtime receipt |
| boundary | `TestRepositoryConsumersUseTheirOwnerRepositoryGroups`、`TestRepositoryProviderAccessIsCentralizedInWorkorchdeps`、`TestRepositoryInitiatorExportSurfaceStaysRepositoryOnly` | 已覆盖 consumer/family、调用入口、导出面及无 EventHub 操作 |

`assertRepositoryGroupsComplete` 显式枚举每个公开 repository group surface；若分组新增子接口，必须同步扩展该断言。具体
CRUD/CAS、错误、并发与 file reopen 语义仍由 repository contract 测试承担，不能从这些 Initiator 测试推导。
