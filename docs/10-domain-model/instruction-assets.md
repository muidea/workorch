# Soul And Skill Instruction Assets

Status: normative

Type: domain-model

Last Verified: 2026-08-21

## Soul

Soul 是治理与行为边界的声明对象，由 `blocks/soul` 唯一拥有。它可以声明阶段约束、allowed tool kinds、capability
allowlist 与风险边界，但不能选择 Runtime 下一动作、改写 Definition 或授予 evidence credit。

Workspace 可以声明默认 Soul 以及按语义位置绑定的 Soul。activation 只消费 owner 返回的 exact revision/snapshot，运行期
不能从 live resource 或 projection 静默替换。

## Skill

Skill来源必须保持分离：`blocks/skill`只拥有WorkOrch Registry资源；SkillHub是Application-private external catalog，
workspace-local资产也不能冒充Registry owner state。Registry create由owner签发canonical content-hash provenance，get/list
使用concrete bounded result；所有来源进入Runtime前都转换为detached typed provenance。

activation最多封存5项Skill；instruction单项上限8 KiB、总计24 KiB，metadata总计16 KiB。Skill只进入volatile C3；
LLM/Orchestrator不得在request或drive时直读Registry/Catalog、重新选择/裁剪，或把Skill提升到C0～C2。正文仅为受预算
约束的context，不成为operation authorization或业务结果。

### SkillHub preparation observation

Application 的显式准备检查沿用 SkillHub 现有 `status --json`／`status --json --global` 行为，
不承诺它是严格只读或零联网：SkillHub 可按自身配置先请求服务端，并保存项目检查状态。
该副作用合同为 `skillhub-status-effects/v1`；它不是安装、同步、use、apply 或修复许可，
WorkOrch 不改写 SkillHub 内部存储，也不自动追加这些命令。普通准备状态 GET、刷新、导航和重启读取
不执行 status；显式检查超时或断连后不能据此认定外部未发生效果，不隐式重发。

项目 status 原始 scope 为 `project`，与 WorkOrch 自身 `workspace` 来源域分开保留。global 清单按
`skill_id + agent` 唯一，`skill_count` 表示不同技能数而非 agent 条目数；保留所有 agent、orphaned、
原 status/reason/version/hash，不能靠过滤、去重或来源仓库 fallback 伪装完整目录。Codex 已安装指令
只从独立解析的 Codex 安装根读取，其他 agent 与 orphaned 仍留在完整诊断清单，不授选择 credit。

stdout 与 stderr 分开保留原 bytes、已读取字节数、所存前缀 SHA256 和完整性；每个流最多保存 1 MiB。
输出超限或未完整结束、状态失败、歧义或缺失均不得当作成功空清单。已安装 `SKILL.md` 的诊断观察
单项上限 64 KiB、每 scope 每轮总计 8 MiB；两轮实际读取原值分别保留，包括失败重读的已捕获原值与不完整标记。
拒绝越界路径、symlink、非普通文件和不完整读取，并真实重读比较。
该诊断上限不放宽上面的 activation 封存预算。清单完整、指令完整、seal 绑定、精确选择、freshness
及 readiness 是不同事实；status 成功不直接授予 PASS、M3 或执行权限，也不证明服务端实际使用了本机文件。
准备结果与原诊断只在受限内部边界返回，公开状态不得透传它们。

显式检查在安装指令两轮读取之间，按第一轮原 frontmatter 的 `referenceRefs` 观察辅助文件。
只允许规范、唯一的本地相对文件路径，相对于该条实际安装技能自己的目录；不接受 URL、绝对路径、
父目录跳转、query/fragment 或 repo/source/cwd fallback，不执行引用内容。独立固定安装根后，
再固定单技能目录，拒绝 symlink、非普通文件及越出该技能目录的访问；两轮实读辅助文件并比较原 bytes。
每 scope 最多 256 个引用、每文件 64 KiB、每轮合计 1 MiB，空文件和二进制原样保留；
缺失、超限、漂移和不完整读取明确记录，不能把前缀当完整文件。原诊断仍按受限 64 KiB 边界保留，
超界明确未保存及原大小/hash。辅助事实版本为 `workorch-skillhub-reference-observation/v1`，
与原 status/安装材料在同次原操作归档，正文及路径不进入公共状态或 Runtime context。

Workspace 从完整原选择和辅助归档独立编译 `workorch-preparation-skill-reference-compilation/v1`，
按 source/scope/sourceSkillID/ref 反查唯一来源；Registry 资源没有文件根合同的引用保持 source_unresolved，
不能借同名 SkillHub 技能或 Workspace 目录补造来源。公共 `skillReferenceState` 仅表示
observed_checks_pending、sources_incomplete、source_unresolved 或 unavailable，始终绑定原 blocked 操作和
完整选择来源；空引用声明不表示无需其他 Skill 检查。无辅助字段的旧历史不补读或重写，完成历史不能删除/改写。
这些事实只证明原时点读取，不证明持续 freshness、status 可接受、整体 Skill readiness 或 M3。

同次原操作另保存 `workorch-preparation-skill-status-compilation/v1` 的选中来源诊断，
从完整原选择、原 status 和双读安装 bytes 独立重算，不修改原状态。只有精确 `synced` 值可以
进入 observed_freshness_pending；modified、conflict、空值、未知值及大小写／空白变体均为
status_blocked，不自动 repair。实际安装 frontmatter 版本与非空 status local_version 不同为
version_conflict；不得拿仓库 version 替换本地版本或因此改写原安装指令。缺少一侧版本不证明一致，
仍未获得版本完整性或 freshness credit。Registry 行只保留 owner 原观察，不伪造 SkillHub 状态。
完整来源缺失或不匹配为 sources_incomplete。该诊断及原 status 仅私有归档；公共
skillStatusState 只含上述四个有限分类，始终要求 v2 blocked 操作及完整选择来源。
空选择、synced、双读及版本未发现冲突都不授 PASS、持续有效性、daemon/local 一致性或 M3。
GET、完成重放、恢复和重开不重新执行 status，不回填或改写旧历史。

准备路径从真实已安装 bytes 独立解析 YAML frontmatter，不复用管理列表的宽松解析器。
重复键（包括嵌套键）、同一语义字段的多个别名、已知字段大小写变体、错误类型、未闭合头、
多个文档、alias／anchor／merge、自定义 tag、超过 16 层或 4096 节点均明确阻塞解析；不将它们
降级为空需求。名称和描述使用 string；版本可使用 string 或保持原标量拼写的数字版本。
内容投影优先使用真实安装指令的版本；缺失时可保留 status 的 local/version 观察，不能用 repo version
代替安装版本。全部原版本与不一致事实仍留在 status 诊断中，投影不证明它们一致。
需求／trigger／reference／constraint 使用 string list，兼容既有逗号分隔 scalar-list；
sequence 内的逗号保留在该元素内，不拆分。未知普通元数据仍保存在原文，不成为选择或授权输入。
无 frontmatter 的旧指令仍可保留为正文，缺失名称按原来源 ID 选择，不推断不存在的需求。

显式选择观察将正式 Registry、global Codex 安装项和本 Workspace 安装项合并，使用封存 Goal／DoD
原文及 execution 阶段 Soul 的 `allowedSkillRefs`，复用既有自动匹配的评分、Workspace 优先及 ID
排序，最多选择五项。全 ID 与 source skill ID 的 allowlist 语义保持不变；完整候选中的重复 ID、
无效 scope 或来源是冲突，不得静默覆盖。输出完整选中内容、typed 匹配理由和全部原来源诊断，
前后反查同 revision 的 Workspace seal；选中行与诊断清单独立复制。
原准备操作的 Workspace 私有结算从已核验完整来源纯编译并持久保存上述历史选择：封存
Goal/DoD 原文、来源身份、execution Soul allowlist、完整候选及选中原内容均保留在受限 archive。
独立重算全部材料，不以 hash 命中或调用者 bool 代替；缺源为 sources_incomplete、冲突为 conflict，
完整编译也只为 compiled_checks_pending。GET、原操作恢复和重开不补编译、不重读或覆盖历史。
同次 Workspace 结算可对上述完整选择复用既有 Runtime Skill 纯预算合同形成独立预览：
固定 5 项、正文单项 8 KiB／合计 24 KiB、元数据 16 KiB；允许的规范化／有界裁剪仅作用于预览，
完整安装 bytes 和选中 Content 不变。保存精确预算与裁剪记录或拒绝原诊断，独立重算原来源到全部
投影，不凭摘要或公开 summary 授信。公开预算轴仅表示 compiled_checks_pending、bounded_checks_pending
或 rejected；它不是 Run context 封存，也不替代辅助资源、状态／freshness 和来源一致性证明。
这只是选择材料，不是资源 PASS、当前有效性证明、Runtime context 封存或 M3：选中项的
`modified`／`conflict` 等原状态不得变为成功；仍需独立核验辅助文件、状态／freshness、
daemon／本机来源一致性及既有 context 预算。Workspace seal 的前后相等也不代表跨 owner 原子快照。

## Invariants

- Soul/Skill 的选择结果与 exact snapshot 必须可追溯；
- owner resource mutation 只影响后续 fresh activation，不改写 active Run；
- instruction content 可约束 candidate，但不能绕过 capability schema、sealed dispatch 或 governance；
- concrete software-development behavior stays in Profile/Soul/Skill, not neutral orchestrator core.

## Related Documents

- [Domain Model Overview](overview.md)
- [Context Assembly Flow](../00-core/runtime-request-context-assembly.md)
- [Soul Routing](../40-profiles-capabilities/workspace/soul-routing.md)
