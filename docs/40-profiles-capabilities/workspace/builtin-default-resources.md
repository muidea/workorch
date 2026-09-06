# Built-In Workspace Default Resources

Status: normative

Type: profile-capability

Last Verified: 2026-08-21

## Purpose

本文定义内置 Workspace Template 与 Soul 资源的 inventory、embedded layout、owner ensure 和 activation boundary。
二者共同属于 startup default seeding contract，但正式状态仍分别由 Workspace/Template owner 与 Soul owner 管理。

## Exact Inventory

`internal/pkg/workorchdefaults/defaults` 当前包含：

- 6 templates：`generic-governed`, `weather-query`, `software-dev`, `code-review`, `doc-generator`, `data-analysis`；
- 每个 template 对应 `planning|execution|decision|approval|validation|artifact` 六个 Soul，共 36 个；
- template `defaultSoulId` 等于其 execution-role binding。

Inventory 是当前 exact baseline，不是最低数量。新增/删除资源必须同步 defaults tests、owner ensure 和本文。

## Embedded Layout

```text
internal/pkg/workorchdefaults/defaults/
  templates.json
  souls.json
  souls/<template>/<role>.md
  mcp_servers.json
  mcp_capabilities.json
```

`souls.json` 保存 metadata 与 required `soulMDRef`；default JSON 禁止 inline `soulMD`。Ref 必须是
`souls/<template>/<role>.md` 下的 relative path，无 traversal/empty segment，目标存在且内容非空。Loader 返回
owner-neutral management DTO，不写 repository。

## Owner Ensure

```text
embedded owner-neutral defaults
  -> kernel/system startup coordination
  -> Template owner concrete Ensure command
  -> Soul owner concrete EnsureDefaults command
  -> each owner validates and persists idempotently
```

System 不定义上述 command schema、不访问 owner repository，也不持有综合 defaults store。Partial startup failure
fail-fast；重启重放同一 owner command。MCP defaults 使用 MCP Registry owner 的独立 ensure contract，在 MCP 专题维护。

所有built-in都是seed-once、user-editable owner resource。Ensure只在built-in ID不存在时创建，绝不覆盖已有Soul、Template、
MCP Server或Capability；删除built-in后，下一次startup可按当时embedded declaration重建。embedded default变更不自动迁移
既有 resource，升级不得要求清空 Runtime 数据。

## Explicit Guarded Upgrade

Template/Soul 的资源库当前内容与新 binary 内嵌内容通过 `GET /api/v1/workorch/builtin-defaults` 独立展示：
`kind/id/present/currentContentHash/currentBuiltinRevision/builtinContentHash/upgradeAvailable`。current revision 为空表示
无法识别为受支持内置内容；已等于目标时 revision 为目标 hash。该视图不是任何已有 Run 的 sealed selection。

`POST /api/v1/workorch/builtin-defaults/upgrades` 只升级一个明确的 `kind + id`，并要求 exact
`expectedContentHash + targetContentHash`。Application 分别发送 Soul/Workspace 定义的 concrete command；比较、准入与写入
只由对应 owner-private internal event 触发，与普通资源编辑串行。无跨 owner upgrade saga、通用状态机或强制覆盖开关。

可升级基线是 `internal/pkg/workorchdefaults/revisions.go` 中经历史提交复核的 exact 内容指纹，不是 ID 前缀或宽松版本范围。
Soul 还必须保留 `builtin_seed` provenance；仅有 provenance 不能证明未被编辑。Template 比较完整内容，包括 policy 与
bindings。未知/定制内容、stale preview、目标 binary 已变化均返回 conflict 并保持原值。新增支持的基线必须来自复核后的
历史 owner-normalized 内容并配套回归；生产路径不读取 Git 或历史 Runtime JSON。该入口不升级 MCP registry。

写入成功但 response 丢失后，原请求在 fresh owner/file handle 重放；已达同一目标则返回 `upgraded=false`，不重复写入或
增加 revision。不存在的资源由 startup ensure 负责，upgrade 不承担隐式 create。

升级 catalog 不改写已存在 Workspace 的 template-bound policy、eligible admission 或 Run activation。需要采用新内容时，
显式刷新 Workspace template binding、重新完成 definition admission，再提交 Restart；Resume/recovery 始终使用原封存输入。
用户定制内容需要显式人工合并，不能把后续重启解释为允许覆盖。

## Template And Soul Contract

Template 可以声明 starter Goal/DoD、default Soul/role binding、context/evidence/capability policy 与 execution constraint。
这些字段是 admission/bootstrap declaration，不能选择 SM-2 state、seal dispatch 或写 evidence。

Soul exposes owner metadata and full `soulMd` guidance；它可以约束 Skill/MCP/capability reference 与模型行为，但不能
授予 owner catalog 中不存在的 operation、绕过 governance、选择 Runtime action 或接受 evidence。API/projection exposes
`soulMd`；`soulMDRef` 只是 embedded implementation detail，Template stores Soul ID/binding 而不是 path。

Fresh Start/Restart resolves exact Template/Soul revisions into immutable activation snapshot；Resume/recovery 不重读后来
编辑的 default file。Active guidance不得要求 StagePacket、NextAction、graph authority 或其它 forbidden provider schema。

## Verification

- `go test ./internal/pkg/workorchdefaults -count=1`；
- `go test ./internal/modules/blocks/soul/... ./internal/modules/blocks/workspace/... -count=1`；
- `make verify-workspace-template-soul`；
- `make verify-runtime-profile-software-dev`。

验证覆盖 exact inventory、ID/path/role 一致、non-empty body、path traversal、inline-body rejection、owner ensure idempotency、
binding validity 与 forbidden authority vocabulary。
显式升级还覆盖 current/target query、exact baseline、定制/未知/stale 零写入、并发普通编辑、写后响应丢失与 file reload replay；
Application/HTTP tests 验证 concrete owner result、错误 scope/hash、缺失/错误/重复结果和 `409` 冲突传播。

## Related Contracts

- [Soul Routing](soul-routing.md)
- [Workspace Setup Draft](setup-draft-session.md)
- [Profiles And Capabilities](../overview.md)
