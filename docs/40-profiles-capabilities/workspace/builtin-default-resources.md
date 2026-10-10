# Built-In Workspace Default Resources

Status: normative

Type: profile-capability

Last Verified: 2026-09-10

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

所有 built-in 都是 user-editable owner resource。Ensure 创建缺失 ID；Template/Soul owner 还自动将精确匹配 reviewed
historical fingerprint 的未定制资源更新到当前内嵌配置，复用下述 owner-private guarded upgrade。未知、定制或缺少完整
来源/内容身份的资源保留，不做推测性覆盖。MCP Server/Capability 仍为 create-if-missing，不参与本合同的自动更新。
删除 built-in 后，下一次 startup 可按当时 embedded declaration 重建；不要求清空 Runtime 数据或重新创建 Workspace。

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

升级 catalog 本身不改写 Workspace、历史 candidate 或 Run activation，也不触发执行。用户定制的 catalog 资源需要显式
人工合并，不能把后续重启解释为允许覆盖。

## Automatic Configuration Loading

新 Start/Restart 读取当前 Template 与 Soul；配置加载不授权替换用户确认的规格：

- Goal-only authoring 在创建／修订 owner 事务中物化完整 Template binding。确认后 Restart 核对当前模板、目录与 Definition policy，复用 exact confirmed、eligible candidate；漂移时阻塞，不自动生成新候选或重新调用创作 provider。更新规格须走受支持的显式创作修订、六审查与确认；不支持的修订状态保持阻塞。catalog 升级与 template refresh 均不是用户确认。
- 以下自动物化与重新准入规则适用于其他受控 owner 定义路径，不能作为 Goal-only authoring 的旁路。

- Workspace 的 `TemplateBindingHash` 记录当前 authoring 字段所物化的 Template 版本；模板未变时保留 Workspace 显式
  配置。模板变化时，重新物化其 profile、default/stage Soul、context hints 与 evidence policies，删除已从模板移除的旧策略。
  这些字段跟随模板整体更新；需要长期维护独立策略时应绑定自己的 Template。旧 Workspace 没有该字段时用旧 startup 的
  Template hash 判断；连该来源也不存在时，在下一次新执行准入中采用当前模板，不回填历史记录。
- Goal/DoD 使用现有用户定义，保留 Workspace ID、目录、provider selection、生命周期和历史 Run。创建后、首次 Start 前
  的模板更新也遵守同一规则。
- Application 只编译 detached 当前输入；Workspace-private definition-admission event 在 Template CRUD 共用锁内重新核对
  Template hash、Workspace revision/configuration fence，再通过 CAS 追加新 candidate 并 promotion 新 startup input。
  同输入重放幂等；并发漂移拒绝，不封存混合版本。新配置不满足当前准入规则时阻塞，不回退使用旧 eligible seal。
- 已完成准入的历史 candidate 仅作不可变 identity/lineage 关联，不用当前 compiler/schema 重编译、规范化或重写。
  新 candidate 必须满足当前合同；缺失/冲突的归档身份及未解决的 clarification 仍 fail-closed，不能借配置更新绕过。
- Soul 在每次新 bootstrap 编译时按当前绑定从 Soul owner 读取，封存其 revision/content hash 和内容；跨请求不缓存旧内容。
  Resume、恢复和同一 Run 的后续 drive 始终使用该 Run 的原封存输入，不热替换运行中的配置。

影响边界是 Workspace pre-seal admission 与 SM-1 新执行输入；SM-1 lifecycle、SM-2/SM-3 authority/dispatch、SM-4 recovery
与 delivery 的 owner/transition 均不改变。配置加载不产生自动 successor，不跨组件管理状态机。

只读版本核对必须区分三层，不能把“已部署最新 binary”或“catalog 已升级”解释为“已有 Run 使用新配置”：

- catalog current/target 使用上述 builtin-defaults preview；
- Workspace DTO 的 `admittedTemplateRevision/admittedTemplateSnapshotHash/admittedStartupInputHash` 来自 Workspace owner
  已 promotion 的 startup input，未准入时为空；template snapshot hash 与 catalog content hash 是不同合同，不能直接等同；
- `acceptance.audit` 的 `startupInputHash/sealedProfileContractsHash/sealedSoulBindings` 只来自该 Run 封存 activation。
  Soul metadata 包含 default/stage、ID、revision、content hash、source provenance，不包含 instructions。

projection clone 必须 detached，display metadata 不得反向构造 admission。没有这些历史封存字段时展示未知，不回填当前
catalog、不推测升级成功，也不触发 owner mutation。

当前 `software-dev` 的新执行声明 `shell/project-native-validation/v4`，并显式封存
`recipeInputReviewProtocolRef=validation/input-consistency/v2`，使逐 CWI 输入必须经过完整原准则映射的独立一致性评审与
包范围验证；这是 Shell capability 合同，不改变模板中立身份、原 Goal/DoD 或验收集合。精确未定制的旧内置模板
通过 reviewed fingerprint 在 owner ensure 时升级；新 Start/Restart 核对当前配置，Goal-only authoring 的已确认模板漂移仍按上述规则阻塞，旧 Run 的 v1/v2/v3 不被回填。
终态 Run 不因升级重新运行；需要显式 Restart。未能识别的定制模板仍须用户合并，不能借升级绕过。
`b8447f75` 的完整旧模板来自冻结历史 fixture；memory/file owner 的重复 reopen 验证 exact upgrade 与定制内容保留，
不从当前模板删字段伪造旧基线。review 协议版本与 native adapter 版本分别封存，不能互相推断或在恢复时补默认值。

## Template And Soul Contract

Template 可以声明 starter Goal/DoD、default Soul/role binding、context/evidence/capability policy 与 execution constraint。
这些字段是 admission/bootstrap declaration，不能选择 SM-2 state、seal dispatch 或写 evidence。

`contextHints.profileContracts` 只允许当前 typed Profile decoder 消费的字段与精确类型，不保留看似政策却未被消费的
扩展。软件开发模板的隔离提示、受限前台执行、禁止 detached 验证与清理要求放在实际进入 activation 的
`contextHints.validationPolicies[].notes`；`validationScratchRequired=true` 与 bounded safety 声明保持不变。
不再声明未被 Runtime 消费的 `validationEnvironmentPolicy`。notes 是声明性 guidance，不创造隔离保证或执行权限；
实际进程、超时、effect、scratch 和清理仍由 sealed operation 与 capability owner 合同约束。原 Goal/DoD、required CWI、
proof requirements 和验收依赖不因该表示调整而改变；旧模板/Run 归档不回填，也不提供旧字段 fallback。

Soul exposes owner metadata and full `soulMd` guidance；它可以约束 Skill/MCP/capability reference 与模型行为，但不能
授予 owner catalog 中不存在的 operation、绕过 governance、选择 Runtime action 或接受 evidence。API/projection exposes
`soulMd`；`soulMDRef` 只是 embedded implementation detail，Template stores Soul ID/binding 而不是 path。

Fresh Start/Restart resolves exact Template/Soul revisions into immutable activation snapshot；Resume/recovery 不重读后来
编辑的 default file。Active guidance不得要求 StagePacket、NextAction、graph authority 或其它 forbidden provider schema。

## Verification

- `go test ./internal/pkg/workorchdefaults -count=1`；
- `go test ./internal/modules/blocks/soul/... ./internal/modules/kernel/workspace/... -count=1`；
- `make verify-workspace-template-soul`；
- `make verify-runtime-profile-software-dev`。

验证覆盖 exact inventory、ID/path/role 一致、non-empty body、path traversal、inline-body rejection、owner ensure idempotency、
binding validity 与 forbidden authority vocabulary。
显式升级还覆盖 current/target query、exact baseline、定制/未知/stale 零写入、并发普通编辑、写后响应丢失与 file reload replay；
Application/HTTP tests 验证 concrete owner result、错误 scope/hash、缺失/错误/重复结果和 `409` 冲突传播。
自动加载另外覆盖 memory/file 启动更新与定制保护、旧 schema 原样 read/CAS/reopen、新配置准入、首次 Start 前更新、
未变化配置幂等、Template/Workspace 并发漂移拒绝、无效新配置阻塞、Soul 重新读取及历史 Run 文件不变。
`TestIsolatedInputTemplateUpgradeReopensReviewedPreviousDefault` 使用冻结完整 `dfa5c1ad` 内容核对 memory/file 升级与
定制保护；`TestSoftwareDevRestartCompilesReviewedDefaultForFullSizeDefinition` 通过正常 HTTP authoring 和正式 restart
compiler，核对 24 条用户要求与 13 条模板必需项形成 37 条正式 DoD，完整生成 44 个 CWI／42 个 proof requirements，并保持幂等 startup；不手工替换 bootstrap adapter。独立显式 activation 与重放只创建一个 Run；该测试停在 SM-1 materialization，不替代 Runtime 执行。

## Related Contracts

- [Soul Routing](soul-routing.md)
- [Workspace Setup Draft](setup-draft-session.md)
- [Profiles And Capabilities](../overview.md)
