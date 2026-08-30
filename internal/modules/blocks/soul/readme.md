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
| `application/workorchd` | Application → Soul | create/get/list/update/delete、`ValidateCapabilitySelectionCommand/Result` | mutation/list/validation 只接受 exact `application/workorchd/context-registry`；get 另接受 exact `application/workorchd/appservice` |
| `kernel/system` | System → Soul | `EnsureDefaultsCommand/Result` | 仅接受 exact source `kernel/system` |
| repository backend | Soul → repository | `SoulRepository` CRUD/list | owner-private 技术依赖 |

每个 topic 在 [`pkg/events`](pkg/events) 中有独立 concrete command/result。list 使用稳定 ID cursor，默认 100、最大 200。
默认资源 ensure 复用同一 normalization/validation 路径，只创建缺失 ID，不覆盖已有对象。

## 不变量与非职责

- 所有 topic 都是同步合同；`result == nil` 时在读取或写入前返回，`Post` 不得改变 Soul 或默认资源。
- create/update 规范化并校验 ID、内容、五组 allowlist、provenance 与聚合字节上限；revision 由 owner 单调签发。
- 同一 Soul 的 production mutation 使用 owner contract 派生的 `state/soul/<soul-id>` lane 串行提交；update 是单条
  catalog replacement command，最后成功提交的 command 为当前值，exact content replay 为 no-op。
- `ExpectedRevision` 对该合同不适用：Soul 没有 read-modify-write lifecycle 或多阶段状态机；revision 是 owner 输出的
  变更序号而非 caller transition precondition。invalid/missing/duplicate/failure 分别返回错误且不产生部分写入。
- result 必须 detached clone；repository/service 不对其它组件开放。
- `EnsureDefaults` 的 caller identity 是合同的一部分，错误 source 即 fail-closed、零写入。
- Soul 不查询 Skill/MCP owner、不判断 selected capability 是否存在，也不把 guidance 转成 Runtime authority。
- `Start` 幂等注册，`Stop` 反向退订并 fence 后续 delivery；Application publisher source 是 owner ingress 合同，错误 source
  在 repository I/O 前拒绝。

## 能力验收

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| normal CRUD/list/validation/revision | [`module_test.go`](module_test.go)、[`biz/soul_service_test.go`](biz/soul_service_test.go) | 覆盖 |
| invalid / allowlist / size / provenance | `TestServiceRejectsInvalidSoul`、`TestServiceCanonicalizesAndBoundsSoulState`、`TestServiceAcceptsNearLimitSoulAndRejectsAggregateOverflow` | 覆盖 |
| cursor / pagination bounds | `TestServiceListSoulsUsesBoundedStableCursor`、`TestServiceListSoulsAppliesDefaultAndMaximumLimits` | 覆盖 stable cursor、noncanonical cursor、default 100 与 max 200 clamp |
| defaults normal / idempotent | [`biz/defaults_test.go`](biz/defaults_test.go) | memory/file 均覆盖 |
| defaults invalid seed 零写入 | `TestEnsureSoulDefaultsRejectsInvalidSeedBeforePersistence` | memory/file 均覆盖 validation-before-persistence |
| System exact source 与 Post 零写入 | `TestStrongSoulCommandsIgnoreFireAndForgetPost` | 覆盖 |
| detached result | [`pkg/events/messages_test.go`](pkg/events/messages_test.go) | 覆盖 |
| memory/file persistence | `TestSoulRepositoryContract_Memory`、`TestSoulRepositoryContract_File` in [`internal/pkg/repository/repository_contract_test.go`](../../../pkg/repository/repository_contract_test.go) | 覆盖 |
| duplicate / idempotency | `TestModuleSoulDuplicateCreateAndUpdateReplayAreOwnerConsistent`、`TestEnsureBuiltinSoulDefaultsSeedsMissingAndPreservesExisting` | duplicate create 返回 conflict；首次真实 update 将 revision 1→2，exact replay 保持 revision 2 与已更新内容不变 |
| stale/freshness | `TestModuleSoulStateLaneSerializesConcurrentUpdatesLatestCommitWins`、`TestModuleSoulDuplicateCreateAndUpdateReplayAreOwnerConsistent` | 不适用 caller-supplied expected revision；覆盖同 ID 并发 command 在 state lane 排队、revision 2→3 的提交顺序、latest committed wins，以及 exact replay revision/content 不漂移 |
| lifecycle | `TestModuleIdentityConfigureStartStopContract`、`TestModuleStopFencesSubsequentSoulDelivery` | 覆盖重复 Start/Stop、7-topic subscribe/unsubscribe 平衡，以及独立的 post-Stop delivery 零写入 fence |
| Application caller authorization | `TestSoulCommandSourceAuthorizationMatrix`、`TestModuleSoulDuplicateCreateAndUpdateReplayAreOwnerConsistent` | 覆盖全部 CRUD/validation topic 的 exact source allowlist，以及 forged/noncanonical mutation 零写入 |
| provider / external lifecycle | 不适用 | 不适用 |
