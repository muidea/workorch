# Skill Registry Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-10-22

## 功能范围与边界

`blocks/skill` 是 WorkOrch Registry Skill 的唯一 owner。它拥有 Skill identity、名称/版本、指令内容、metadata、
required tool/MCP/capability binding，以及 owner 从 canonical content 签发的 provenance。能力包括 create、get、
有界 cursor list 和显式准备的完整只读 Registry 观察；当前没有 update/delete command。

本组件不拥有 SkillHub catalog、workspace Skill 选择、activation budget、Run snapshot、Runtime authority 或 LLM prompt
投影。Registry 与 SkillHub 是两个独立 catalog；Skill owner 不解析 `skillhub:*`、不做跨 catalog fallback/merge/selection。
Application 的精确读取先查询 Registry，仅在 `ErrNotFound` 时回退 SkillHub；list/selection 的候选合并也由 Application
完成，Registry 的其他错误必须原样传播。`TriggerRules` 与 required binding 在这里仅是被校验、持久化的声明材料；候选
匹配、routing、activation 与实际调用不由 Skill owner 执行。

## 组件交互及合同

| 交互方 | 方向 | typed EventHub 合同 | 调用/授权语义 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Skill | `CreateSkillCommand/Result`、`GetSkillCommand/Result`、`ListSkillsCommand/Result` | create 只接受 exact `application/workorchd/context-registry`；get/list 接受该 source 与 exact `application/workorchd/appservice` |
| `application/workorchd` 显式准备子用例 | Application → Skill | `ObserveRegistryCatalogCommand/Result` | 仅 exact `application/workorchd`；携带 Workspace scope，但库仍为 global，不合并 SkillHub |
| repository backend | Skill → repository | `SkillRepository` create/get/list | owner-private 技术依赖，不是跨组件 facade |

topic、command 和 result 的唯一实现入口是 [`pkg/events`](pkg/events)。四个 topic 分别使用 concrete result，不共享可空
联合 envelope。list 使用稳定 ID cursor，默认 100、最大 200；输入字符串、内容与聚合 material 均受 owner 上限约束。

准备观察 topic 为 `/skill/startup-preparation/registry-catalog/query`。单次 repository List 完整读取并 detach，
不拼接多个时点的分页；memory/file List 在同一次读锁内取完整 map，键/记录 ID 不一致或 nil 记录拒绝，
不修复持久内容。观察逐项校验完整 canonical 原记录与 provenance，排序后拒绝重复 ID，最多 1024 项、
整个结果最多 8 MiB；超限返回错误和零完整结果，不把前缀标为完整。空库必须显式完整空列表。
每次返回 observation time、owner 重建即变化的随机 epoch 和完整内容 hash；hash 仅标识内容，不能替代
调用者完整记录复核、当前性或权限检查。没有 update/delete command 不等于观察永远 fresh。
该 topic 无写入、refresh、SkillHub 调用或选择行为，不证明完整合并目录、所选 Skill、资源 readiness 或 M3。

## 不变量与非职责

- create 是同步强写：没有 `Result` 的 EventHub `Post` 在持久化前返回，必须零写入。
- owner 对全部 scalar/list material 做 normalization、UTF-8/字节上限与 provenance 校验；读取到缺失或漂移的 owner provenance 时 fail-closed，
  不补写、不迁移。
- result/model 必须 detached clone；调用方不能通过返回值修改 owner state。
- repository、service 与 constructor 保持 Biz-private；跨 owner 只能使用 receiver-defined concrete EventHub 合同。
- `Start` 重复调用不重复注册；`Stop` 释放订阅，后续 delivery 不得读取或修改 Registry。Application publisher source 是
  owner ingress 合同，forged/noncanonical source 在 repository I/O 前拒绝。
- Skill 不解释 Goal/DoD，不选择 Soul/MCP，不修改 activation 或 Runtime aggregate。
- Skill 不根据 `TriggerRules` 路由请求；Application/profile 只能读取 owner 返回的 detached 声明后在自身用例中选择。

### 生命周期合同

- `Start` 逐成功订阅记账；部分失败保留已取得的 topic 和 service，未清理前拒绝再次注册。
- `BeginShutdown` 非等待式关闭 fresh create；Get/List/准备观察保留原 typed/source 校验，作为既有事务的只读 drain 通路。
- 所有订阅 handler 从准入到真实返回持有生命周期票，context 取消不等于完成。`Quiesce(ctx)` 超时保留资源，允许新预算重试。
- `Stop` 先排空，再关闭最终准入并复核；逆序退订仅成功才移除账本，全部成功才释放 service/Module 引用。未知 panic 不吞掉，也不遗留清理锁。
- 本地 Quiesce 不是全进程屏障；正式最终释放仍要求共享任务与 Hub 排空。调用方串行执行 Configure/Start/Stop；Configure 不覆盖未释放 owner。
- plugin void Teardown 通过 guard 传播包含原 cause 的失败；SM-1～SM-4 的业务转换、权限和预算不变。
- 回归入口：`biz/owner_lifecycle_test.go`、`module_lifecycle_test.go`，覆盖逐项订阅/退订失败、真实执行超时重试、fresh/drain 与 guard cause。

## 能力验收

准备目录的纯有界快照／原值验证位于 `pkg/models/preparation_catalog.go`，EventHub 仍使用本 owner 的
具体 command/result，显式转换为 detached snapshot，不引入 alias facade 或已验证凭据。
Application 可把两轮完整原值交 Workspace 归档为原准备请求的历史；Skill owner 不写 Workspace，
不将目录查询视为 SkillHub 组合选择、资源 readiness 或 M3。克隆保留 nil／empty 列表形状。

| 维度 | 可执行证据 | 覆盖结论 |
| --- | --- | --- |
| normal create/get/list 与分页 | [`module_test.go`](module_test.go)、[`biz/skill_service_test.go`](biz/skill_service_test.go) | 覆盖 |
| invalid、UTF-8、size | `TestServiceRejectsInvalidSkill`、`TestServiceRejectsInvalidUTF8SkillMaterial`、`TestServiceRejectsOversizedSkillMaterial`、`TestServiceGetSkillRejectsInvalidID` | 覆盖 scalar/list material 与 create/get ID 边界 |
| pagination/cursor/limit | `TestServiceListSkillsIsBoundedAndCursorPaged` | 以 205 条真实数据覆盖默认 100、最大 200 clamp、tail page，以及 cursor canonical/UTF-8/256-byte bound |
| provenance / detached result | `TestServiceRejectsPersistedSkillWithMissingOrDriftedOwnerProvenance`、[`pkg/events/messages_test.go`](pkg/events/messages_test.go) | 覆盖 |
| fire-and-forget effect guard | `TestCreateCommandWithoutSynchronousResultDoesNotMutateRegistry` | 覆盖 |
| memory/file persistence | `TestSkillRepositoryContract_Memory`、`TestSkillRepositoryContract_File` in [`internal/pkg/repository/repository_contract_test.go`](../../../pkg/repository/repository_contract_test.go) | 覆盖 |
| lifecycle | `TestModuleIdentityConfigureStartStopContract`、`TestModuleStopFencesSubsequentSkillDelivery` | 覆盖重复 Start/Stop、4-topic subscribe/unsubscribe 平衡，以及独立的 post-Stop delivery 零写入 fence |
| duplicate / idempotency | `TestModuleRejectsForgedSkillSourcesAndDuplicateCreateWithoutExtraWrite` | 覆盖 EventHub duplicate-create 返回 conflict 且 Registry 保持单项 |
| 准备完整性/漂移 | [`biz/preparation_catalog_test.go`](biz/preparation_catalog_test.go)、[`pkg/events/preparation_catalog_test.go`](pkg/events/preparation_catalog_test.go) | 覆盖重复、损坏、非 canonical、超限、取消、scope/source、drain 与 epoch 重建；Application 另外逐次重读并比较完整记录，不授长期 freshness |
| cross-catalog routing / activation | `TestReadSkillViewFallsBackToSkillHubWhenRepositorySkillMissing`、`TestListSkillsViewMergesRepositoryAndSkillHubSkills`、`TestSelectedSkillsCanResolveSkillHubReferences` | 这些是 Application 级边界证据；Skill owner 只保存声明，不执行 fallback、merge、匹配、选择或调用 |
| caller authorization | `TestSkillCommandSourceAuthorizationMatrix`、`TestModuleRejectsForgedSkillSourcesAndDuplicateCreateWithoutExtraWrite` | 覆盖全部 topic 的 exact source allowlist，以及 forged/noncanonical create 零写入 |
| provider / external lifecycle | 不适用：本组件不访问 provider 或 SkillHub | 不适用 |
