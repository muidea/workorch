# Soul Registry Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-30

## 功能范围与边界

`blocks/soul` 是 Soul definition 的唯一 owner。它拥有 identity、monotonic revision、canonical content hash、source
provenance、名称/描述、business domains、`SoulMD`，以及 Skill、MCP server、MCP capability、capability category、tool
kind 五组 context-selection allowlist。公开能力为 CRUD、有界 list、allowlist validation 和内置默认资源 ensure。

Soul allowlist 只约束 Application 的 activation/context 选择，不产生 capability readiness、effect、governance 或 Runtime
operation authority。本组件不拥有 stage binding、workspace template policy、MCP Registry 关系、Run snapshot 或 SM-2/SM-3
transition。

## 组件交互及合同

| 交互方 | 方向 | typed EventHub 合同 | 调用/授权语义 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Soul | create/get/list/update/delete、`ValidateCapabilitySelectionCommand/Result` | intended caller；CRUD/validation 当前不执行 exact source authorization |
| `kernel/system` | System → Soul | `EnsureDefaultsCommand/Result` | 仅接受 exact source `kernel/system` |
| repository backend | Soul → repository | `SoulRepository` CRUD/list | owner-private 技术依赖 |

每个 topic 在 [`pkg/events`](pkg/events) 中有独立 concrete command/result。list 使用稳定 ID cursor，默认 100、最大 200。
默认资源 ensure 复用同一 normalization/validation 路径，只创建缺失 ID，不覆盖已有对象。

## 不变量与非职责

- 所有 topic 都是同步合同；`result == nil` 时在读取或写入前返回，`Post` 不得改变 Soul 或默认资源。
- create/update 规范化并校验 ID、内容、五组 allowlist、provenance 与聚合字节上限；revision 由 owner 单调签发。
- result 必须 detached clone；repository/service 不对其它组件开放。
- `EnsureDefaults` 的 caller identity 是合同的一部分，错误 source 即 fail-closed、零写入。
- Soul 不查询 Skill/MCP owner、不判断 selected capability 是否存在，也不把 guidance 转成 Runtime authority。
- `Start` 幂等注册，`Stop` 反向退订；Application source 仍是调用约定而非授权边界。

## 能力验收

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| normal CRUD/list/validation/revision | [`module_test.go`](module_test.go)、[`biz/soul_service_test.go`](biz/soul_service_test.go) | 覆盖 |
| invalid / allowlist / size / provenance | `TestServiceRejectsInvalidSoul`、`TestServiceCanonicalizesAndBoundsSoulState`、`TestServiceAcceptsNearLimitSoulAndRejectsAggregateOverflow` | 覆盖 |
| cursor / pagination bounds | `TestServiceListSoulsUsesBoundedStableCursor` | limit 分页与 noncanonical cursor 已覆盖；default 100 与 max 200 尚无独立断言 |
| defaults normal / idempotent | [`biz/defaults_test.go`](biz/defaults_test.go) | memory/file 均覆盖 |
| defaults invalid seed 零写入 | `TestEnsureSoulDefaultsRejectsInvalidSeedBeforePersistence` | memory 覆盖；file 尚无独立 invalid-seed 验收 |
| System exact source 与 Post 零写入 | `TestStrongSoulCommandsIgnoreFireAndForgetPost` | 覆盖 |
| detached result | [`pkg/events/messages_test.go`](pkg/events/messages_test.go) | 覆盖 |
| memory/file persistence | `TestSoulRepositoryContract_Memory`、`TestSoulRepositoryContract_File` in [`internal/pkg/repository/repository_contract_test.go`](../../../pkg/repository/repository_contract_test.go) | 覆盖 |
| duplicate / idempotency | delete missing 与 defaults replay 有局部覆盖 | duplicate create、duplicate update 的 EventHub 结果尚无矩阵验收 |
| stale/freshness | update command 没有 expected-revision/CAS 字段 | stale writer 语义未定义、未验收 |
| lifecycle | `TestModuleIdentityConfigureStartStopContract` | 覆盖基本 Start/Stop；尚无独立 post-Stop delivery 断言 |
| Application caller authorization | 无 exact source gate | 未覆盖，Application 仅为 intended caller |
| provider / external lifecycle | 不适用 | 不适用 |
