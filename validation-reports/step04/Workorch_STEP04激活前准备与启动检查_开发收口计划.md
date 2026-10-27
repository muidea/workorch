# Workorch STEP04 激活前准备与启动检查开发收口计划

> 归档说明（2026-10-21）：本计划从 `docs/` 移至验证区域，正文保留原提案时点。
> 后续增量与批准以独立记录为准；用户已批准 `STEP04-实施前合同审阅稿.md` §3
> 精确 no-Run 协议 v1。下文“尚未批准”是原历史状态，不再是新的授权前置阻塞。
> 本文件不是现行设计权威，也不是整个 STEP04 验收通过记录。
>
> 2026-10-22 范围修订：用户明确“当前 skill-hub 的执行符合预期，按照当前 skill-hub 的运行效果进行对应内容收口”。
> SkillHub 显式 status 检查因此是下文“零写入/零联网/不启动进程”的窄例外：保留其项目状态保存及
> 配置服务端访问，不要求改造 SkillHub，不增加 install/sync/use/apply/repair。普通 GET/刷新仍不调用它，
> provider cache-only、零付费推理/embedding、零 Run 与其他 owner 边界不变。现行合同见
> `docs/10-domain-model/instruction-assets.md`，验证与未完成项见 `STEP04-SkillHub现行行为适配与验证记录.md`。

- 状态：待评审冻结的方案草案；新增合同尚未批准
- 固定基线：`d93463f37983575723393578c15faff51cf1d844`
- 架构约束：go-multi-module-dev v2.6，单一 owner 与 typed EventHub
- 本轮仅讨论和静态研究，未实施代码、运行项目测试/CI、调用 provider 或开展付费复验

## 1 结论和完成点

建议 STEP04 收窄为：**针对已经准备好资源、且尚无任何 Run 历史的 Workspace，显式检查精确封存合同的启动条件，执行纯编译，保存 Workspace owner 的检查记录和 M3 完成回执。** 缺少资源或托管配置时准确阻塞，提示人工准备后重查；本阶段不自动安装、生成配置、启动 MCP、联网刷新目录、调用推理或 embedding。

完整 M3 必须同时成立：

1. 精确合同已确认并封存，Definition、StartupInput、admission、Template/Profile/策略绑定一致
2. 合同要求的资源与创建准备确实就绪，有适当 owner 的实际事实
3. 全部 required 启动检查通过，身份、时点和有效性符合冻结合同
4. 成立时没有 Run，且 owner 协议能证明其与在途激活意图的先后关系

`eligible` 仅是启动输入准入，不是 M3；`bootstrap.Validate()` 仅证明结构有效，不是资源已准备或 required 检查已通过。准备记录不授予执行权限、dispatch authority 或目标验收 credit。[S01]、[S02]、[S03]

M3 是历史里程碑。后来过期或资源变化，应保留原 completionID、版本和时点，显示“新建已完成，当前启动受阻”；新定义不能继承旧 M3 徽章。真正激活、Run 创建、runtime、semantic plan 和执行派发不进入 STEP04。默认手动激活与明确有界自动模式保持既定方向，完整检查回执消费链留给 STEP05。[S01]

完整 M3 正例限定一个有界 profile：其所有 required 项须有真实且仍有效的适当 owner observations；本阶段不新发外部 probe。缺事实就阻塞，fixture 模拟 PASS 不能冒充生产 readiness。

## 2 基线事实与关键风险

STEP03 已确认的精确确认/封存链、一项 P3 及验证补强均按已关闭承接，无新增功能阻塞。无需重新索取旧原文或重复付费复验。以下差距属于 STEP04 的独立边界：

| 已核对现状 | 计划约束 |
|---|---|
| 当前 bootstrap 编译没有 Run transition，但内部仍有副作用 | 不把“无生命周期转换”当作纯函数；禁止整段接入 GET |
| `EnsureManagedConfigs` 创建目录并写 JSON，system 配置含 overwrite | 本阶段只观察已存在内容；缺失/冲突为阻塞，不自动调用该 helper |
| Memory 在非空且配置语义召回时可能 embedding | 保留原策略；不能传 nil loader 或改 list fallback 凑 PASS |
| `EnsureWorkspaceDefinitionAdmission` fallback 可提交 admission/打开 Decision ticket | 提取 exact-seal 只读查询，禁止检查过程自动补准入 |
| provider selection 校验可因缓存缺失/过期联网刷新 catalog | 新增 cache-only/no-refresh 观察；零推理不等于零外网 |
| SkillHub 读取可能吞错误；MCP 选择丢弃 registry revision | 保留完整性/错误/版本，不用空列表或声明替代实际可用事实 |
| 无 Run 投影只是展示；现有 execution-activations 是真实执行意图 | 新增小型 preparation 资源，不复用 Run 结果 DTO 或执行 endpoint |

现状来源见 [S04]–[S09]。当前 bootstrap/资源 helper 只能按真实副作用拆分复用，不能换名字后宣称纯读取。

### 基线 CI 单独收口

继承基线现暴露 EventHub inventory **实际 311、期望 310** 的不一致；旧 10/3 问题已修复。这是集成出口前的独立基线门禁，不应标为 STEP03 新功能缺陷。后续先核对 topic、typed payload、owner、lane 和登记，再修正真实遗漏，不能直接改计数凑绿。基线未绿只能报告定向结果，不能宣称全量集成通过。本轮不启动修复、监控或重跑。

## 3 范围与职责

### 本阶段包含

- exact-seal 只读读取、Profile 所需检查清单、typed owner 观察
- 已准备资源的显式无业务副作用检查，纯 bootstrap/检查结论编译
- Workspace-owned 操作幂等、记录持久化、服务重启恢复、失效与脱敏
- 首次创建场景的 no-Run 并发门，见 §7 的待批准合同变更草案
- HTTP/Web 查询、检查、原操作恢复与阻塞展示，以及分层验收

### 明确排除

自动资源物化/安装/配置写入、仓库 clone、业务文件修改、付费推理/embedding、MCP 启动或外部 live probe、后台巡检、自动补授权、Run/runtime/semantic plan/执行派发、完整 Start/Restart 回执消费、TUI 新建、强制导出和新的通用工作流模块。

不自动修改已封存 Goal、DoD、Profile 义务、资源范围或预算。普通重查不需要 ticket；只有真实改变既有目标、验收、约束或合同承诺，才按既有 reasoned ticket → 修改审查 → 最终精确 approve → 采纳流程办理。§7 明确改变既有激活入口约束，因此单列 ticket 草案，不把它伪装成普通新 helper。

| 责任方 | 单一职责 |
|---|---|
| Workspace owner | exact seal、准备适用范围、操作身份/幂等、epoch/pending、检查记录与 M3；不替资源 owner 宣称可用 |
| File/目录/配置的实际 owner | 已有路径、内容、归属与权限的只读事实；不扩大 authoring read/runtime write 原合同 |
| LLM/provider owner | cache-only 配置/能力 revision、完整性、freshness；不发生成、embedding 或隐式 catalog HTTP |
| Soul/Skill/MCP/Memory owner | 精确选择、版本、完整性、模式与真实观察；声明不等于可用性 |
| Run owner | 完整且可排序的首次激活 inventory/请求结果；继续独占 Run 生命周期 |
| application | typed query/command 编排、纯编译、DTO；不直接访问其他 owner repository |
| HTTP/Web | 请求身份、版本 fence 和事实展示；不计算权威 M3、不产生自动启动 |

跨模块合同遵守 v2.6：具体 query/command/result、validator、topic/owner/lane 先登记；无跨 owner `biz`/存储访问、无动态 map 权威桥。小型记录放既有 Workspace 边界，详细资源证据留原 owner，以有界 ref/hash 关联，不增加无必要模块。

## 4 拟冻结的功能和接口

以下名称均为草案，不是现有 API。建议以 **8 项功能**收口：

| 编号 | 能力 | 固定输入输出与边界 |
|---|---|---|
| F01 | `QueryExactConfirmedStartupSnapshot` | 返回 exact 确认/封存链；缺失、不一致、待澄清失败；无 fallback admission/ticket |
| F02 | `CompilePreparationManifest` | 纯输入→稳定检查清单；required、适用条件、owner、证据类型、有效规则来源明确 |
| F03 | `ObservePreparedStartupResources` | typed owner 只读观察；保留完整性/版本/错误；不刷新 catalog、不启动进程、不写资源 |
| F04 | `CompilePreparedBootstrap` | detached typed bundle→纯校验结果/摘要；不收 Hub/client/repository/callback，不运行 semantic plan |
| F05 | `CheckWorkspaceStartupReadiness` | 显式命令接纳操作，收集 F01–F04，保存 exact 结果；允许写检查账本，不允许写资源或执行 |
| F06 | `GetWorkspacePreparationState` | 仅读已有记录及已知失效；不编译/probe/重试；缺记录显示待检查 |
| F07 | `RecordCreationCompletion` | 全 required PASS＋有效绑定＋no-Run fence 才记录历史 M3；不提交 Start |
| F08 | `RecoverPreparationAttempt` | 查询原操作后显式恢复允许的阶段；不因重启/超时自动重放未知效果 |

建议新增 GET/POST `/workspaces/:id/activation-preparation`，名称按最终合同目录冻结。GET 对应 F06；POST 对应显式检查/精确幂等重放。202 只表示操作已接纳，不代表 ready。必要时另设只读 attempt/diagnostic 子资源。

请求至少绑定 request/attempt ID、预期 Workspace revision、exact Definition/StartupInput、清单/config fingerprint、检查策略版本。服务端独立核验；可用 owner-issued opaque token 压缩字段，但必须绑定全组身份。客户端不能提交可信 PASS、任意文件路径、probe 命令、bootstrap、Run ID 或生命周期状态。

必须区分：接纳前拒绝、已接纳在途、已完成、被阻塞、已知失败、结果未知和 stale。HTTP 成功只代表响应成功；传输超时不证明“没有发生”。现有 authoring preparation-diagnostics、confirmation PASS 和 execution admission 不承载新启动检查语义。[S08]

## 5 检查清单和证据规则

每项包含稳定 checkID、要求来源、required/optional、适用理由、owner、对象 ref/revision/hash、所需事实类型、观察时点、有效规则与诊断。

清单至少覆盖：精确 seal；真实服务端目录与权限边界；已有托管配置内容；Soul/Skill 及目录完整性；MCP registry/连接事实；Memory 模式/库存/选择证据；provider 配置/能力/缓存 freshness；必要持久化完整性；首次激活 no-Run 事实。只检查本 profile 真正需要的项，不把所有资源都强制加入。

单项建议语义：PASS 为适当 owner 实际取得的合格事实；FAIL 为已检查且失败；BLOCKED 为缺授权/能力/资源/前置；UNKNOWN 为确实未能确认；NOT_APPLICABLE 必须由冻结 Profile/合同允许并给出理由。未做、未支持、未授权、缓存过期都不是 PASS，也不是随意 N/A。

编译产生的 prerequisite/DECLARED、capabilityReady、registry membership 或配置存在，只证明各自声明/结构。现有 MCP 测试对远端 /fail 仍可 Ready 且 endpointCalls=0，不能升格为当前可达证明。runtime dispatch preflight 依赖 Run/dispatch，不能造假 Run 调用。[S03]、[S06]、[S12]

检查有整体/单 owner hard deadline、有限清单/响应字节/并发上限，默认不隐式重试；超限/未完成准确阻塞，不截断后 PASS。默认零资源写入、零外部刷新/探测、零付费推理/embedding：

- 新增 cache-only owner 观察；cache miss/过期/LKG 不满足规定 freshness 时阻塞，GET 与显式检查都不能隐式刷新
- 真正配置为 list-only 或无 Memory 的 profile 可以按该事实验证；不得把 required semantic selection 改成 list-only
- 必需 MCP live/embedding/在线推理等事实无法在本阶段取得，保持具体阻塞，另提必要预算/权限；不能 mock 顶替生产事实
- 已存在配置缺失或内容不符为 `PREPARATION_INCOMPLETE`，给出准备对象与负责方，人工准备后再显式检查

未来若增加自动准备，须另冻结受控 typed File owner 合同：Workspace 批准的 internal scope、精确路径/清单/前后 hash、CAS、大小/次数、未知效果对账；不能扩权原 authoring deliverable-read 或需 Run 权限的 runtime-write。当前 system overwrite 事实不等于已有 create-only 保障。本扩展不是 STEP04 必做项。[S09]

## 6 记录身份 失效 恢复和 Web

### 最小持久记录

保存 schema/check-policy/compiler version；Workspace/creation epoch/attempt/request/payload hash；expected/current revision；exact confirmation/authoring identity（input、proposal、specification 不混用）、Candidate/Definition/StartupInput；Template/Profile/policy/manifest；各 owner observation/ref/revision；checkedAt/expiry/epoch；结果/原因；纯编译摘要及 no-Run 顺序证据。首次 M3 以 Workspace＋creation epoch 唯一，不按每版 Definition/证书重复签发；后续变化保留原 completionID。

配置身份仅绑定脱敏后的行为配置与 opaque auth/config revision/reference；不存 credential 明文或由 secret 值派生的 hash，轮换通过 owner revision 失效。只保存有界记录与引用，不存 provider client、Hub、大模型上下文或资源全文；日志/事件/API/Web/导出统一脱敏。

### 幂等和恢复

- 同 key 同规范化 payload 返回同操作；同 key 异 payload 冲突；重复点击/多标签仍由 owner 去重
- 超时先查原操作，不能换 ID 重做；新检查形成新 attempt，旧记录及历史 M3 保留
- 服务进程重启恢复原账本，不调用 execution Restart，不自动探测/联网/收费/写配置
- 进程/连接相关证据在 epoch 变化后失效；持久事实仍须符合原 revision/freshness
- 不明状态先对账；不能用超时、空列表或“未查到请求”证明旧消息不会迟到，详见 §7

### freshness 与界面

expiresAt 取所有 required 观察/相关授权的最早期限；无稳定 revision 的对象须有有界 scope/期限的 observation token，无此证明就不能长期 valid。TTL 不是无漂移保证；GET 只用已有时间和已知 revision/event 投影失效，不探测远端。显示检查时点/对象/等级/有效范围；未来激活仍须 use-time 核验。

Web 无需重新进入 authoring，可从已封存 Workspace 直接打开检查面板。只读刷新、显式检查、查询/恢复原操作与关闭分开；缺资源展示准备指引，不提供本阶段未实现的自动补齐按钮。读取/刷新/关页不启动任何后台动作。

封存、准备、检查、历史 M3、当前有效性、授权和真实 Run 是不同状态轴。页面不堆入一枚混合成功枚举；不新增“已被检查门禁保护”的绿色 Start 按钮。既有执行控制可保留并清楚分区，不能声称其已消费新回执。若真正有 Run，展示实际 Run，不继续显示当前无 Run。

刷新/重连由服务端恢复，沿既有规则不使用 localStorage/sessionStorage/IndexedDB 等保存业务输入或权威事务身份。session/query/owner-revision fence 应在应用迟到响应前生效，旧结果不能覆盖新版本。[S01]、[S08]

## 7 no-Run 并发门与合同变更 ticket 草案

### 为什么必须单列

当前 Workspace 入口只有 deletion RLock 与可能 Noop 的 provider CAS，没有持久在途激活 reservation；Run 普通列表未与 execution lock 排序，并可对 retired Workspace 返回空。一次空列表＋后续写 M3 存在 TOCTOU，不能证明提交时无 Run。[S10]、[S11]

下文只是文档中的 reasoned ticket 草案，**未创建外部 issue、未取得批准、没有虚构 ticket ID**。它改变既有 fresh intent 入口约束，需要在实施前精确评审/批准；新准备合同的评审不代替这项批准。

### 变更目的与范围

目的：让 M3 成立时的无 Run 事实可证明，同时保留 Run owner 的生命周期权。首次 M3 检查/签发只适用于**无任何 Run 历史**的 Workspace；历史 Run/替换事务返回 out-of-scope。此限制不改变既有 Start/Restart/Resume；对已登记 intent，先解析同键身份/结果，不能因首 Run 已创建就拒绝原意图回读。绝不补造 dummy Run。

拟议 before/after：现有 fresh intent 直接 forward；改为先在 Workspace 私有 reducer/CAS 中登记持久 preparationEpoch 与 pending reservation，再 forward。所有支持的 fresh ingress、精确重放路径都必须参加；不能只改 Web。

### 必须闭合的协议

1. 首次接纳 fresh intent 在 forward 前持久 reserve/increment epoch，绑定 RequestID、StartupInputHash 和 payload/bootstrap hash，失效当前 ready；同 ID 同 payload 返回原登记/结果，不重复递增或换意图；同 ID 异 payload 冲突
2. M3 完成是 Workspace state-lane 命令，要求 same expected epoch/revision、exact seal、无 unresolved pending；长检查不持锁，结果提交前重核关键版本
3. Run owner 新增完整、retirement-aware 的首次激活观察，在既有 `workspaceExecutionMu` 同一顺序边界下读取；不得把 retired/unknown/过滤后的空列表当 no-Run，持锁时不做跨 owner I/O
4. Workspace final CAS 只在 epoch 未变且仍无 pending 时记录 M3；其后到达的 fresh intent 先失效当前 readiness 再 forward，历史 M3 不被删除
5. forward 丢回复/超时保持 pending/unknown。**“查不到匹配请求”、空 inventory、计时器到期都不能清除 pending**，因为旧请求仍可能迟到
6. 只有确定的原请求结果，或 Run owner 持久终态拒绝/取消/fence token/tombstone 能证明旧消息以后不会被接纳，才可安全结算；若现有合同不支持此证明，就保持阻塞，先补齐最小协议，不能自动 forward 来“对账”
7. 旧版本遗留不明在途请求必须阻塞；优先支持新协议启用后创建、或可证明无旧在途请求的 Workspace，不能凭零历史推断没有迟到消息

并发门在 owner 合同/故障注入层验证顺序，最后的无 Run Web 验收不提交真实执行意图。本协议不要求 Start 已消费准备回执，也不新增 Start/auto-start 功能。它只约束既有 fresh intent 的登记顺序；不承诺跨 owner 原子锁住所有外部资源。Run owner 仍独占创建/激活，准备命令不提交执行意图。

### 替代方案 影响和回退

替代方案是保持入口不变，只交付 `noRunObservedAt` 时点报告；这可做准备子能力，但不能宣称 full M3 已收口。仅靠测试隔离、页面禁用或短 TTL 不能替代协议。

影响包括并发 fresh intent 的显式 conflict、pending 的恢复与旧版本迁移；不得静默排队/自动运行。最小验收包括提交与 reserve 的两种顺序、丢回复、进程重启、旧 forward 迟到、retired 查询、异 payload 重放和 final CAS 冲突。

回退时停止签发新的 M3，保留历史记录、pending 和 fence/tombstone，展示当前未知/受阻；不得删除协调状态后放行旧消息。此 ticket 批准只覆盖工程合同变更，不等于批准任何 Workspace Goal 变更，也不授予执行或付费权限。

## 8 未来精确激活消费合同

STEP04 冻结后续接口要求，不声称当前已全部实现。STEP05 应在所有 fresh Start/Restart owner 入口接入同一 gate：

- 绑定 exact Workspace/Definition/StartupInput、准备记录/M3 身份和独立手动授权或有界自动 grant/permit
- 重核 schema、scope、freshness、关键 owner 事实及 grant 状态；stale/越界/撤销/错身份 fail closed
- 不能静默用新配置重新编译，却声称消费旧记录；稳定 initialactivation 身份使手动/自动/迟到重试收敛为同一意图/Run
- Run owner 唯一创建/替换；Resume 继续复用原 Run 封存输入，不吸收新 Workspace 准备记录

确认合同、GET、保存回执和 M3 都不自动授权执行。明确有界自动模式以后可独立提交精确激活，无须对同一有效授权重复点击；其完整实现不在 STEP04。[S02]

## 9 三个渐进增量

### 增量一 固定合同与纯边界

完成 F01/F02/F04/F06 的小型 DTO、validator、清单和纯编译；隔离 admission fallback、managed write、embedding、catalog refresh；登记 v2.6 topic/owner/lane。冻结 §7 并发合同，明确其尚待批准。

出口：exact identity、required 覆盖、纯函数、GET 零隐式副作用的定向证据齐全；独立评审无功能阻塞。只能显示“待检查/受阻”，不称完整 M3。

### 增量二 owner 观察与 M3 记录

完成 F03/F05/F07/F08；资源已备场景的 owner 事实、cache-only 观察、幂等/失效/重启恢复；在获批范围内实现最小 epoch/pending/Run observation 协议。缺资源维持 BLOCKED，不扩展自动物化。

出口：一个真实可满足的有界 profile 形成 exact M3；所有未满足的 required 项准确阻塞。并发协议和未知 forward 不误解锁，记录无秘密，无 Run/runtime/业务副作用。

### 增量三 Web 与集成收口

实现直接入口、分轴展示、检查/原操作恢复、迟到响应 fence；校验 eligible/历史 M3/当前 readiness/真实 Run 区别。完成未来消费接口、交付索引与回归矩阵。

出口：下表 **20 项验收**的适用要求闭合；基线 CI 单独先收口，受影响累计回归与声明的 CI gate 通过；独立审查无 P0/P1/P2。不要求二十次独立 live 或全业务 live。必须如实说明尚未接入完整 receipt-consumption 的 Start/Restart，不能用局部门禁声称全链已保护。

## 10 分层验收矩阵

以下是待实施要求，本轮未执行。每项记录固定 SHA、profile、输入绑定、owner 结果、断言和实际验证层级。

| 编号 | 场景 | 核心通过条件 |
|---|---|---|
| A01 | exact seal | 错 Workspace/revision/hash、缺确认或待澄清失败；不补 admission/ticket |
| A02 | 清单与证据 | required 全覆盖；合法 N/A 有来源；声明、未做、未授权不算 PASS |
| A03 | 纯编译 | 无 Hub/I/O/副作用；Validate 不被解释为全 required 已通过 |
| A04 | GET 与导航 | GET/刷新/关页/轮询只读；资源写、外网、embedding、MCP启动、Run调用均为零 |
| A05 | 已有资源 | 真实 owner 事实；缺失/替换/同 inode 改写/跨 scope/symlink 失败，资源无写入 |
| A06 | 目录完整性 | SkillHub error 不伪装空列表；MCP/provider revision、完整性和 freshness 不丢失 |
| A07 | provider cache | cache miss/expired/LKG 按策略阻塞；即使显式检查也不隐式 HTTP 刷新 |
| A08 | Memory/外部依赖 | 不改 required semantic 模式；未授权 live/embedding 阻塞，默认无付费调用 |
| A09 | 幂等与并发 | 同 key 同 payload 同操作；异 payload 冲突；多标签不重复接纳 |
| A10 | 失效 | definition/config/resource/检查策略/TTL/epoch 改变使相关结果 stale；不冒称实时外部探测 |
| A11 | 崩溃与服务重启 | receipt/证书/M3 CAS 前后崩溃或丢回复后按原身份恢复，未提交不补造；无自动探测 |
| A12 | no-Run 顺序 | 所有 fresh ingress reserve 后才 forward；final CAS 与 reserve 竞态只有先完成后失效或阻塞 |
| A13 | unknown forward | 超时/空列表/缺请求不清 pending；迟到旧 forward 不能在无持久 fence 后误建 Run |
| A14 | 历史与迁移 | retired/有任何历史 Run/替换/旧未知消息不作首次 no-Run；不用 dummy Run |
| A15 | M3 事实 | exact 资源与 required 检查全部满足＋有效 no-Run 证据；后续失效保留历史 |
| A16 | 零运行 | 准备调用链无 Run创建/runtime初始化/semantic plan/dispatch/业务文件写，相关入口零调用 |
| A17 | HTTP/Web | 202≠ready；精确 DTO/错误身份；eligible/历史M3/当前检查/Run 分开 |
| A18 | UI 恢复 | 迟到响应/旧会话/低 owner revision 不覆盖新状态；从 owner 恢复，无业务持久缓存 |
| A19 | 脱敏与架构 | secret canary/异常返回不泄密；deadline/size cap 生效；单 owner、typed EventHub/inventory 一致 |
| A20 | 继承与交接 | STEP03 精确确认不回退；既有 execution/Resume 语义清晰；未来消费合同不冒称已实现 |

验证顺序为纯合同/单元 → typed owner 故障矩阵 → 一次有界全新 production build 的真实 HTTP/Web session，覆盖正例、一个 required 阻塞及同项恢复、同服务进程 stop/start。独立 owner readback、Run/runtime inventory、目录/hash、provider/embedding/网络调用计数核对结果；前序 authoring/fixture 准备先记基数，与 STEP04 增量分账。无 Run、零目标效果、零付费均须独立证明，截图不足。重启保留历史 M3，瞬态证据按 epoch 转 stale，不自动刷新；这里绝非 execution Restart。

证据包记录 final SHA、buildInput/consoleTree/binary hash、运行实例/config/backend 身份；复用 build_identity 只证明字节一致，不称可信 attestation。[S14] fixture 范围和生产义务分别报告：可隔离外部 provider 响应，但不能删 required；外部 live 未验不得 PASS。[S13]

## 11 独立评审与业务交付

先进行合同评审，再做 owner/架构及 no-Run 协议审查，随后检查实现、恢复/安全和集成证据。每个增量独立复核，不能用作者说明代替事实。P0/P1/P2 为零；P3 单列影响，不能掩盖身份、权限、M3 或恢复缺陷。若 no-Run 协议未闭合，只能报告准备子能力，不能称 full M3 收口。

最终用户产出为：

1. 精确准备清单：需要什么、已有什么、缺什么、哪个 owner/人员处理
2. 启动检查视图：证据等级、时点/有效范围、具体阻塞和允许的下一步
3. 历史 M3 记录：哪个版本何时完成、当时无 Run 的证据；与当前状态分开
4. 原操作恢复入口：准确已接纳/在途/未知/完成，不重复创建或隐式执行
5. 与 STEP03 合同和未来激活的 exact 关联；不把定义文件当业务目标成果

工程交付含固定合同、三个增量、20 项证据索引、独立评审、基线 gate 单独记录、已知限制与 STEP05 交接。可选导出不是本阶段 required 新承诺，也不是第二份合同数据库。

## 12 仅需确认的两项

1. **阶段范围**：是否采用“已准备资源＋显式只读检查＋纯编译＋准备/M3 记录”的有界 STEP04；自动物化与外部 live probe 延后，缺 required 条件持续阻塞
2. **合同变更**：是否批准 §7 最小 fresh intent fence 的设计方向，并在实施前审阅冻结后的精确合同；若不调整既有入口，只能交付时点观察，完整 M3 保持未收口

这两项是方案/合同确认，**不等于现在授权开发、运行测试、修复 CI 或调用 provider**；实施仍以用户后续明确指令为准。已定的 M3、默认手动/有界自动、Web-only 不重问。以后仅出现具体新增费用、数据/权限范围或已封存约束变化时，提出对应的最小许可或 reasoned ticket；不为普通工程拆分制造用户问卷。

## 固定来源

以下均固定到同一源码 SHA；源码证明现状，设计文档说明既定目标，本文新增要求仍属待批准草案。基线 CI 与 STEP03 关闭结论承接本轮已核对结果，不由测试文件存在推断 PASS。

[S01]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/docs/Workorch_最新设计文档/Workorch_Web新建Workspace合同与交互方案.md
[S02]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/docs/00-core/definition-convergence/activation-flow.md
[S03]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/application/workorchd/biz/appservice/activationcore/runtime_bootstrap.go
[S04]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/application/workorchd/biz/appservice/activationports/execution_context.go
[S05]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/application/workorchd/biz/appservice/activationports/definition_admission.go
[S06]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/blocks/llm/biz/catalog_service.go
[S07]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/application/workorchd/biz/appservice/activationports/memory_selection.go
[S08]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/application/workorchd/service/server/http_workspace.go
[S09]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/application/workorchd/biz/appservice/workspacecore/managed_configs.go
[S10]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/kernel/workspace/biz/execution_intent.go
[S11]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/blocks/run/biz/run_queries.go

[S12]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/internal/modules/blocks/mcp/biz/owner_semantic_probe_test.go#L174-L183
[S13]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/scripts/verify-workspace-confirmation-step03.py
[S14]: https://github.com/muidea/workorch/blob/d93463f37983575723393578c15faff51cf1d844/scripts/build_identity.py#L1-L25

组织规范来源：[go-multi-module-dev v2.6.0](https://github.com/muidea/skills-repo/blob/6ba50452fccd33c912b07a0648fbb97acf15d756/skills/go-multi-module-dev/SKILL.md)。基线 CI 结果：[d93463f 对应运行](https://github.com/muidea/workorch/actions/runs/39203208895/job/117900390075)，本轮不重启该运行。
