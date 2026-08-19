# Skill Registry Stateful Block

Status: active

Type: stateful-block-contract

Last Verified: 2026-08-19

## 功能范围与边界

`blocks/skill` 只管理 WorkOrch Registry 中的 Skill 正式状态：Skill identity、指令内容、metadata、required
tool/MCP/capability binding，以及 owner 签发的 typed provenance。当前公开写面只有 create，不公开 update/delete
command。

SkillHub Catalog 不是本 Block 的 repository 或正式状态。它由 `application/workorchd` 的外部 catalog adapter 读取；
Application 可以在 activation 选择阶段合并两类候选，但必须保留各自的来源身份：

- WorkOrch Registry：`source=workorch-registry`、`scope=global`、空 `WorkspaceID`；
- SkillHub Catalog：`source=skill-hub`，scope 为 `global` 或绑定非空 `WorkspaceID` 的 `workspace`。

两类来源都使用 canonical Skill content 的 SHA-256 作为 `ContentHash`。SkillHub 自身的 source/applied/actual hash
仍属于 Application adapter metadata，不能替代 Runtime provenance 的 canonical content hash。

## 组件交互及合同

- `blocks/skill/pkg/events` 分别定义 `CreateSkillCommand/CreateSkillResult`、`GetSkillCommand/GetSkillResult` 和
  `ListSkillsCommand/ListSkillsResult`，不存在跨 topic 的可空字段联合 result。
- list 使用 ID cursor，默认每页 100 项、最大 200 项；owner 按 ID 稳定排序，Application 的多页读取另有
  1,000 页/100,000 项 hard limit，并拒绝乱序、空页 continuation、重复或异常 cursor。
- create 是同步 command：handler 在执行任何持久化之前要求非空 result。误用 EventHub `Post` 时必须零写入，
  不产生无回执的 fire-and-forget mutation。
- `application/workorchd` 通过 EventHub create/get/list WorkOrch Registry；SkillHub 只经 Application-private catalog
  adapter 读取。两者都先映射为 detached management DTO，再由 activation builder 封存。
- activation 最多封存 5 个 Skill；每条 instruction 最多 8 KiB、instruction 总计最多 24 KiB、metadata 总计最多
  16 KiB。instruction 采用确定性 UTF-8 安全裁剪并记录 original/included bytes 与 truncation；metadata 超限直接
  fail-closed。该 budget 与 typed provenance 一起持久化在 Run activation snapshot。
- `kernel/orchestrator` 和 `blocks/llm` 不导入 Skill owner model/events、不读取 Skill repository、不发送 Skill topic；
  它们只消费 Run owner 已持久化的 sealed activation。LLM 将该有界快照投影到 volatile C3，不在请求阶段再次选择、
  查询或裁剪 Skill。

## 交互合同完整性

| 交互组件 | 方向 | 具体合同 | 当前结论 |
| --- | --- | --- | --- |
| `application/workorchd` | Application → Skill Registry | concrete create/get/bounded-list command/result | 完整；写命令必须同步回执 |
| SkillHub Catalog | Catalog → Application adapter | external catalog DTO → typed Skill provenance | 完整；不是本 Block 状态或 EventHub 合同 |
| Workspace activation | Application → Run bootstrap | detached Skill snapshot、typed provenance 与 sealed byte budget | 完整；不复制 Registry repository authority |
| `kernel/orchestrator` / `blocks/llm` | 无直接 Registry I/O | 只消费 sealed activation/C3 projection | 完整；Registry/Catalog 后续更新不改变既有 Run |

## 相关约束

- 本 Block 只访问 Skill repository，不执行工具调用、不创建 Result、不修改 Run 或 SM-1～SM-4 状态。
- create 对 ID/name/version/description/instruction/list metadata 和总 content 执行 byte/count 上限；owner 规范化内容后
  生成 provenance，调用方不能提交或覆盖该身份。
- repository 返回的缺失、未知或与 content 不匹配的 provenance 必须 fail-closed；不得由 Application 猜测来源或
  继续构造 activation。
- `Configure` 必须同时取得 Skill repository 与 EventHub；任一依赖缺失时立即失败，不得延迟到 `Start` 或首个命令。
- core runtime 不得按 Skill ID、名称、来源或说明文本写死业务分支；Skill guidance 不得覆盖 Goal、DoD、stage
  contract 或 requiredOutput。

## 测试状态

已覆盖：

- [x] create/get/list concrete result 与 model slices 深拷贝，调用方修改不污染 owner state。
- [x] owner-issued provenance/content hash、非法 persisted provenance 和 instruction/content 大小上限。
- [x] bounded cursor pagination、Application 多页聚合及乱序、重复、空页、超大页异常。
- [x] create 经 `Post` 误投时零持久化；缺 repository/EventHub 的配置 fail-fast；重复 Start/Stop 幂等。
- [x] topic inventory 固定为 create/get/list，不存在公开 update/delete。
- [x] activation 对 Skill 数量、instruction/metadata byte budget、UTF-8 裁剪及 budget drift fail-closed。
- [x] 静态边界确认 Orchestrator/LLM 不导入 Skill owner、不引用 `SkillRepository` 或 Skill EventHub topic；LLM C3
  只渲染 sealed Skill snapshot/budget。

真正待完善：

- [ ] 增加 file backend 的旧版无 provenance 数据升级/拒绝策略端到端验证；当前 owner 对这类记录明确 fail-closed。
- [ ] 增加真实 Run restart 回归，确认 Registry/SkillHub 后续变更不改变已持久化的 activation snapshot。
