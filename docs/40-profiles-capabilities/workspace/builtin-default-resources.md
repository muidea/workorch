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
MCP Server或Capability；删除built-in后，下一次startup可按当时embedded declaration重建。embedded default变更不迁移既有
resource；未来若需强制refresh，必须先定义versioned、auditable owner migration contract。

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
- `make verify-workspace-template-soul`；
- `make verify-runtime-profile-software-dev`。

验证覆盖 exact inventory、ID/path/role 一致、non-empty body、path traversal、inline-body rejection、owner ensure idempotency、
binding validity 与 forbidden authority vocabulary。

## Related Contracts

- [Soul Routing](soul-routing.md)
- [Workspace Setup Draft](setup-draft-session.md)
- [Profiles And Capabilities](../overview.md)
