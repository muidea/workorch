# Soul Registry Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-21

## 功能范围与边界

`blocks/soul` 是 Soul catalog 的唯一 owner。正式状态包括 Soul identity、monotonic revision、content hash、source
provenance、名称/描述、business domains、`SoulMD` guidance，以及 Skill、MCP server、MCP capability、capability
category、tool kind 五组 context-selection allowlist。

下列内容不属于 Soul owner 状态：

- stage binding 与 template stage-policy refs 由 Workspace/template/Application activation 组合；
- capability 是否存在、server/capability 归属、stage readiness 由 MCP Registry 或相应 capability owner 提供 typed fact；
- effect、replay、risk 与 approval authority 由 capability/governance 合同提供；

Soul allowlist 只约束 activation 选择和 provider-visible stage context，不产生 Runtime operation authority。SM-2/SM-3
仍只能从 template/profile 声明、capability owner fact 与 sealed manifest 获得授权。

## 组件交互及合同

- create/get/list/update/delete 与 allowlist validation 都由 `blocks/soul/pkg/events` 定义；每个 topic 使用独立的
  concrete command/result：`CreateSoulResult`、`GetSoulResult`、`ListSoulsResult`、`UpdateSoulResult`、
  `DeleteSoulResult`、`ValidateCapabilitySelectionResult`。
- 所有 topic 都是同步 command。若误用无 result 的 `Post`，handler 在任何读取或写入前直接返回；create、update、
  delete 和 ensure-defaults 不会静默改变 owner 状态。
- `ListSoulsCommand` 使用 `CursorAfterID + Limit`，默认 100、owner hard limit 200；结果回显稳定 `NextCursor`。
  Application 若需完整管理视图，必须逐页读取，并执行页数、条目数、单调 cursor 和 cycle guard。
- `application/workorchd` 通过 EventHub 管理/读取 Soul。fresh activation 使用 execution stage Soul（缺失时才回退
  effective default Soul）完成 Skill 自动选择和五组 allowlist admission，避免 default 先选中、execution 后拒绝。
- MCP selection 在 Application 组合层先用 Registry typed record 确认 capability 存在、绑定已选择 server 且具有
  category，再提交 Soul allowlist validation；Soul owner 不跨 owner 查询 Registry，也不猜 risk/effect/approval。
- activation 封存 detached default/stage Soul snapshot。LLM semantic-plan C3 消费 planning Soul；operation candidate
  C3 按 execution/validation/decision 当前 stage 选择 Soul，并按该 Soul 过滤共享 Skill/MCP context。approval/artifact
  当前没有 provider/capability 执行入口，其 binding 只是封存快照，不能宣称已有独立执行语义。
- `kernel/system` 发送 Soul owner-local `EnsureDefaultsCommand` / `EnsureDefaultsResult`；默认种子复用 create 的同一
  normalization/validation 路径，只创建缺失 ID，已存在对象保持不变。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Soul | concrete CRUD、bounded list、allowlist validation | 完整；只覆盖 Soul 自有五组 allowlist |
| `kernel/system` | System → Soul | `EnsureDefaultsCommand` / `EnsureDefaultsResult` | 完整；校验后 create-only、非覆盖 |
| Workspace activation | Application → Run bootstrap | detached default/stage Soul guidance 与 allowlist snapshot | 完整；选择使用 execution Soul |
| `blocks/llm` | Run snapshot → provider C3 | planning 及 execution/validation/decision 当前 stage guidance；Skill/MCP stage filter | 完整；不产生 operation authority |
| capability / governance owner | 无 Soul 直连 | existence、ownership、effect/risk/approval typed fact | 不属于 Soul 合同 |

## Owner 不变量

- Soul ID 必须是已 trim、非 `.`/`..`、不含路径分隔符的 UTF-8 值，最大 256 bytes；因此 memory/file backend 的
  正式 EventHub 寻址一致。
- name 必填且最大 256 bytes；description 最大 4 KiB；`SoulMD` 必填且最大 64 KiB；完整 Soul 最大 96 KiB。
- 每组 list 最多 64 项、单项最大 2 KiB；owner trim、去重并稳定排序。allowlist 中 `*` 必须单独出现；空 allowlist
  表示该类别不允许进入当前 stage context。
- create/update/default seed 写入规范化对象；get/list 对 repository 数据重新校验并 detached clone，非法存量数据
  fail-closed 为 conflict。
- create 从 revision 1 开始并签发稳定 content hash；相同内容 update 零写/零 revision，内容变化单调递增且保留 source
  provenance。builtin seed 标记 `builtin_seed`，用户创建标记 `user_managed`；完整 legacy record 读取时投影为
  revision 1/`legacy_owner_upgrade`，部分缺失的 revision metadata fail-closed。
- 本 block 只访问 Soul repository，不调度 Runtime block、不修改 Run 或 Result 状态。
- core Runtime 不得按 Soul ID、名称或显示文本写死业务分支；Soul guidance 不得覆盖 Goal、DoD、sealed manifest、
  stage contract、Runtime policy 或 requiredOutput。
- 默认 Soul 是可由用户编辑的首次启动种子；普通 ensure 不得覆盖内置 ID 上的当前对象。强制刷新只能通过另行定义的
  显式迁移合同完成。

## 已覆盖测试点

- [x] memory/file backend 上默认 Soul 的部分已存在、缺失补建、重复 ensure 与内置 ID 用户修改非覆盖语义。
- [x] 默认种子走 create 校验，非法 seed 在持久化前失败。
- [x] create/get/update/list concrete result 深拷贝；delete/validation 返回可校验 identity/disposition。
- [x] create/update/delete/ensure-defaults 误用 `Post` 时零写入。
- [x] ID、文本、list、wildcard 冲突、内容上限、规范化与稳定 cursor 分页。
- [x] Configure 缺 repository 或 EventHub 时 fail-fast；重复 Start/Stop 保持幂等。
- [x] execution Soul 优先于 default Soul；MCP capability/server/category 绑定校验；planning 与
  execution/validation/decision provider C3 使用对应 stage Soul，Skill/MCP context 不跨 stage 泄漏。
- [x] Orchestrator 不直接查询 Soul；既有 Run 只消费持久化 activation snapshot，Registry 后续变化不改写当前 Run。
- [x] Soul revision/content hash/source provenance 的 create、same-content noop、changed-content increment、builtin seed 与 legacy projection。
- [x] 接近 96 KiB aggregate 上限的 Soul normalization/get、总量越界拒绝，以及 250/450 条多页 Application 管理读取的
  顺序/cursor 正确性与 allocation/CPU benchmark。

## 扩展准入条件

- 当前没有已知的本地可执行合同或测试缺口。
- approval/artifact 当前没有 provider 或 capability 执行入口，因此不存在待补实现。未来若新增此类入口，必须先定义对应
  stage 的 typed 输入、allowlist projection 与消费回归，再将其从“仅封存 binding”提升为完整合同；该条件不能反向宣称
  当前已支持相应执行语义。
