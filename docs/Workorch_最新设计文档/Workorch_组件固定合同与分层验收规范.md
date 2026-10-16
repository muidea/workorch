# Workorch 组件固定合同与分层验收规范

- 版本：v0.3；日期：2026-10-12；性质：工程冻结评审候选

- 状态：设计评审候选；尚未取得冻结批准，不是现有实现已经全部满足的证明
- 范围：先固定 Core Agent Loop 的四状态机与不变量，再沿固定 owner 合同接入 22 个运行单元
- 源码基线：`muidea/workorch@5360caf36e081717087557f34d2af3af1c16dc16`
- 组织规范：`muidea/skills-repo@6ba50452fccd33c912b07a0648fbb97acf15d756` 的 `go-multi-module-dev` v2.6.0 及 `references/EVENT_USAGE.md`
- 本次只读核对和方案撰写；未修改 WorkOrch 代码、未执行项目测试、CI、真实服务或 live
- 本文 CF、V、SG、U/P/W 编号只用于设计索引和未来验收条目，不是新 topic、Go 类型、状态机、既有测试函数或运行期注册表
- 配套主文：[Workorch 核心 Agent Loop 与状态机合同基线](Workorch_核心AgentLoop与状态机合同基线.md)；本篇重点展开组件合同和分层验收

## 1. 冻结的对象与前置条件

建议固定的是“业务输入/事实怎样被哪一个 owner 接受、产生什么权威写入、怎样恢复”，而不只是 Go struct 编译通过。冻结至少包含：

1. Core 四机的唯一 reducer owner、合法转换、guard、完整写集和同次 CAS 不变量
2. 每个跨 owner 交接的输入、身份、版本、权限、成功/拒绝/重复/过期/未知语义
3. 出站副作用前的持久承诺、丢失响应后的查询/对账和恢复规则
4. 原始目标与 DoD、Definition、Run、证据、预算、结果和历史的不可混淆边界
5. 组件单独通过、合同双端通过、公开 Web 全环通过各自能证明什么

顺序为：Core 评审基线确认 → owner 合同候选逐项确认 → provider/caller 单独符合性 → 按合同边接入真实对端 → 公开 Web 全环及故障恢复验证。不能用“22 个目录都已调整”代替任何一个关口。

冻结前必须解决：状态机未决分支、未知结果处置、终态交付顺序、预算耗尽后继、无可恢复触发器的等待、持久兼容策略。确需变更冻结项时更新合同版本和受影响验证范围，不能通过实现旁路悄悄放宽。

### 1.1 本轮产品边界

- 以Web为本轮唯一Workspace创建入口；必须移除TUI创建入口，不保留跳转或占位入口。其他TUI只读/运维能力是否保留不在本轮扩大决策。仓库当前仍有TUI/CLI，它们不能替代公开Web创建与全环验收
- M3 表示 Workspace 创建完成，必须仍然没有 Run；M4 才是单独的执行激活
- 默认手动激活；明确授权的、有界自动模式复用同一准入、权限、预算、防重和恢复合同。自动接受应记录真实政策/主体，不能伪造人工点击
- 已承诺Definition变更必须形成完整原因、影响和最终候选；严格按NC04：exact用户选择→Workspace definition-scope preflight→仅runtime-bound时Orchestrator matching→Decision formal commit/ACK→Workspace adoption→journal prepared→整组materialized seal→等待交接/激活；任何阶段成功不替代下阶段
- Decision拥有选择和formal resolution提交；Workspace拥有精确preflight、adoption和Definition应用。无Run或合法terminal prospective successor使用definition-scope receipt，不伪造Run或runtime continuation。Approval只负责高风险操作/运行治理许可，不替代最终候选批准
- 旧 Definition、旧 Run 输入、旧批准、证据与结果历史保持不可变。无 Run 的定义变更不创建虚构 Run；有前驱时由 Run owner 管理 replacement/lineage
- 批准变更、准许某次风险操作、授予目标验收 credit 是三个不同事实

## 2. 当前源码锚点：已有基础与不能直接推断的结论

以下为指定 SHA 的实际观察；后文“应/必须”是本候选的目标约束。

| 当前可核对事实 | 源码位置 | 对本次设计的含义 |
|---|---|---|
| 基线为 1 Initiator、5 Modules、16 Blocks；单一 go.mod；实际 magicCommon v1.5.18 | `AGENTS.md`、`README.md`、`go.mod` | 22 是运行单元数，不是 22 个 Go modules；规范 v1.5.16 的示例不要求降级 |
| Run 的 `StoreRuntimeStateRecordCommand` 携带 Workspace/Run、typed RuntimeStateRecord、LifecycleFence、expected runtime revision；查询可绑定持久ref/revision，结果 clone | `blocks/run/pkg/events/messages.go` | 已有物理 CAS/读回基础；不授予 Run 对 SM-2/3/4 的语义修改权 |
| Run lifecycle 的跨 owner 输入是 intent/fact，不携带任意目标 RunState；终态输入包含确切 ResultID | `blocks/run/pkg/events/lifecycle_messages.go` | caller 只能请求/提供事实，Run-private reducer 才能决定 SM-1 |
| 当前 `WorkspaceExecutionIntentCommand.Validate` 在该文件处理 Resume/Restart | 同上 | 不能凭这个类型断言它已经承载所有 Start 路径；冻结清单仍需把实际 Start 接线补全 |
| Workspace 保存 authoring 时绑定 request、expected input hash 和 workspace revision；查询有历史、candidate、admission、confirmation 关联校验 | `kernel/workspace/pkg/events/definition_authoring.go` | 可承接精确确认与恢复合同；字段存在不等于 M3 全流程已通过 |
| File 的具体 Execute 命令、target snapshot/selection、repair confirmation 与 reconciliation 合同已存在 | `blocks/file/pkg/events/messages.go`、`runtime_reconciliation.go` | capability 可以报告目标事实与执行事实，不能替 Core 选择 action 或判完成 |
| File `SealedRuntimeOperation` 验证 operation/target/CWI、execution authority 和 payload hash | `blocks/file/pkg/events/operation.go` | 应保留 sealed payload 与实际执行数据的单一来源，不能再给一个可漂移的 caller Data |
| LLM `CompleteProviderDocumentResult` 区分互斥 Document/Failure/Incomplete，绑定 request hash、配置版本，记录 submitted/usage | `blocks/llm/pkg/events/provider_document.go` | 文档回传成功、provider 成功、候选有效、目标满足必须分开 |
| Result 输入 `ResultMaterial` 排除 owner 生命周期字段；Result owner 验证 terminal fact 后派生自己的状态 | `blocks/result/pkg/events/lifecycle_messages.go` | Orchestrator 封存裁决材料，Result 保存材料；不能由 UI 给 Result 指定 accepted |
| transition manifest 将 SM-1 归 Run，SM-2/3/4 归 Orchestrator；Orchestrator 内部写集明确列出同次 CAS 的多机协调 | `scripts/state-transition-owners.manifest`、`biz/runtime/internal_event_write_set.go` | machine 分类不是共享 reducer；同一 owner 可以在一个私有事务内协调 sibling ledger |
| `RuntimeTransitionCoordinator.Commit` 校验完整 aggregate、预像、revision+1，然后委托 opaque CAS | `kernel/orchestrator/biz/runtime/transition_coordinator.go` | 核心收口点已经存在；需验证失败/丢回执/重载，不能只看接口签名 |

本篇“opaque CAS/持久化”只指Run不解释Orchestrator的SM-2/3/4语义；跨owner输入仍是具体typed RuntimeStateRecord，不是opaque bytes、任意JSON或共享可变store。

两处需在冻结评审明确处理，不能笼统说“现有接口全部合规”：

- Orchestrator 当前多个列表命令共用 `OrchestratorCommandResult`，含 ToolCalls/Patches/ValidationResults/CoverageWorkItems/ArtifactContinuations 多个字段。它是可见的有限形状，不能直接等同无约束 `any`，但仍需证明不同查询不会误接收别的结果臂；本版决定保留该有限结构，每个具体查询增加接收owner定义的单臂校验，禁止多臂/错臂；列表nil与空表示合法零项，其他臂必须为nil。具体臂绑定见Orchestrator合同卡；不新增通用Envelope，不保留“必要时”决策悬项
- File `SealRuntimeOperation` 当前通过 JSON round-trip 做 detached clone。确定性编码用于 hash/持久化与“用 JSON 做组件合同兼容传输”应分开评审；本版决定以显式typed深拷贝替换该JSON round-trip，逐字段复制slice/pointer/nested value；hash的规范编码独立保留原算法/版本，clone变更不得改hash。删除JSON clone后加入VS-V20反向别名检测；不得以JSON重编码作为schema迁移。typed clone/typed conversion保全边界，不能以此写法扩张为通用 JSON bridge。现有 Result typed conversion 与 raw clone 可作参考

新增必须保留的现状限制：

- **ImplementationWork 尚不能生产准入**：`runtime_implementation_work_policy.go` 中 `ExplainRuntimeImplementationWorkAvailability` 对声明了 ImplementationWorkPolicy 的要求报告 unavailable；`ErrRuntimeImplementationWorkUnavailable` 明确说明语法合法不等于可执行。私有 working/no-credit 状态/receipt代码存在，不能因此宣称公开链已可用。它是待关闭的实现缺口，不得借owner-fact acquisition或repair绕过
- **Approval 当前要求非空 RunID**：当前 lifecycle dispatcher 不能原样承载pre-Run最终定义确认。Decision最终候选授权决议与Workspace confirmation/admission需按上一节分工实现，不创建dummy Run
- **post-seal定义变更已有准备子集，完整链仍缺**：新Decision RegisterContractChangeProposal登记exact proposal/revision/hash；Workspace PrepareDefinitionChange构造独立候选并保留旧seal、历史permit/unknown及预算。当前application入口明确不包含HTTP、批准或应用，并拒绝Run历史；候选准备只支持原goal与criterion的exact差异。它不能替代NC04 exact approve、runtime matching、formal commit/ACK、adoption、整组seal与安全handoff。
- **receipt作者与持久owner不同**：capability提供typed执行事实；当前File通过Orchestrator的typed查询/commit接口保存receipt，持久receipt由Orchestrator负责。不能从“owner receipt”推断每个capability已有独立receipt仓库
- **跨Run unknown/in-flight屏障是目标约束**：现有生命周期和reconciliation合同不足以证明所有能力、所有新Run均已阻止与前驱未知效果冲突的副作用；需单列验证与缺口
- **证据适用性已有新增校验，闭环仍须独立证明**：最新terminal compiler已加入current proof适用性过滤与source obligation投影/反漏项检查；六类review也新增每required scope覆盖、差集检查、N/A拒绝及owner持久重读复验，不能继续把这些具体项写成完全未实现。完整目标仍包括原始承诺无损覆盖、证据真实性、terminal current binding/lineage以及真实依赖资源owner revisions；其中OwnerRevisionSet仍由accepted evidence聚合，不能将局部过滤升级为完整owner revision closure。实现证据、未实施目标和未执行验收三栏必须分开。

逐topic与接线证据以[具体合同覆盖台账](contracts/组件具体合同目录.md)为准；代表性锚点不再代替全量目录。台账区分已读取声明、已定位handler、已观察caller与未知项，不把静态读取当生产装配/运行验证。


### 2.1 2026-10-12 最新静态增量与边界

本轮以完整tree差异检查最新源码，不能用GitHub compare的300文件截断列表代替。组件消息声明为134个pkg/events生产Go文件、307个实际topic；相对旧基线保留302个稳定CT，新增5个，无topic删除或wire重命名。目录迁移不改变Workspace的owner身份与历史CT。

| 新增合同 | source已观察 | target未因此闭合 |
|---|---|---|
| [Decision proposal登记](contracts/decision.md#ct-decision-registercontractchangeproposal) | request、expected revision/hash、proposal持久身份与候选反查 | exact最终批准、runtime matching、formal commit/ACK和adoption |
| [Workspace独立改约候选准备](contracts/workspace.md#ct-workspace-workspacedefinitionchangeprepare)、[查询](contracts/workspace.md#ct-workspace-workspacedefinitionchangequery) | 原seal不覆盖、待结算permit拒绝、候选独立保存、reviewed identity精确回查 | 完整NC04与跨Run安全交接；不得放宽到任意constraint变更 |
| [File原read依赖重观察](contracts/file.md#ct-file-queryruntimefilereaddependencies) | original sealed file/read选择器、receipt hash、owner revision事实重新核对 | 不执行操作、不授credit，也不替Core决定证据有效性 |
| [Shell原始验证输出查询](contracts/shell.md#ct-shell-queryruntimevalidationrawoutput) | 原receipt/dispatch/operation/fence/stage/stream、owner保存身份、64KiB上界和SHA256校验 | 新源CI明确报告生产sender缺失；不能标接线完成或SG2通过 |

已读取的最新既有CI结果报告两项失败：`/shell/validation/query-raw-output sender=false receiver=true`，以及topic inventory实际307、测试expect仍306。此处是读取已有结果，不是本轮运行测试；需分别修复生产接线与维护清单，不得仅改数字掩盖缺失sender。[既有CI日志](https://github.com/muidea/workorch/actions/runs/38215841512/job/114700248734)

## 3. 不可转让的权威写集

### 3.1 四机唯一来源

四机owner、转换、guard、完整写集、同CAS约束及恢复后继只在[Core基线](Workorch_核心AgentLoop与状态机合同基线.md)和其链接的Runtime逐edge规格维护。组件卡引用对应边，不复制状态机大表。Run是SM-1语义owner与opaque持久提供方；Orchestrator是SM-2/3/4唯一语义owner。

### 3.2 其余正式写入

| owner | 自己可修改 | 只能观察/请求 |
|---|---|---|
| Workspace（目标 Module） | 草稿、候选、正式 Definition/启动输入、active-slot 及其协调记录、唯一删除 journal | Run lifecycle；其他 owner 的清理/退休结果 |
| Decision | ticket、完整原因与选项/最终候选绑定、approve授权决议及resolution、自身resolution提交历史（不含Workspace adoption权威） | Workspace的confirmation/admission写入、Run、操作执行与目标验收 |
| Approval | 风险请求/许可、principal、scope、有效性与 resolution | 目标语义、业务完成、任意未来操作权限 |
| Governance | 自己的政策执行 trace 与协调事务 | 通过 Decision/Approval 的合同取得正式决议，不直接改它们的存储 |
| Result | 从 sealed terminal material 构造 Result/Artifact 元数据、幂等 delivery 记录 | Orchestrator 的验收裁决；其他 owner 生命周期 |
| 能力 owner | 自身资源/adapter effects、typed receipt内容与查询/对账事实、自己的effect边界 | Core authority/coverage、Run state；当前Runtime receipt持久提交仍经Orchestrator合同 |
| LLM | provider 配置、模型事实、调用/输出/usage 事实 | semantic proposal 无权改正式定义、选择下一步或自授 PASS |
| Memory/Skill/Soul/MCPRegistry | 各自资产、版本、来源与有界查询 | 不将知识/技能/配置转成执行或验收权威 |
| Eventstream/workorchd projection | 审计记录/可重建读投影 | 不借丢通知、刷新页面、日志重放推进业务 |
| System | 多 owner 默认配置/能力就绪用例的协调结果 | 默认资源仍由原 owner 管理；health 不等于 execution admission |
| workorchruntime Initiator | 单一持久化基础能力的进程句柄及生命周期 | 不是 Hub 单元，不处理业务合同、不持有业务状态机 |

“事实”可能触发接收 owner 的合法私有转换，但不会把转换权限转给事实的生产者。出站 `SourceOwner` 字符串和 hash 只是校验材料，不能单独证明来源真实；还必须由受控装配、合法 sender/receiver 与已封存 identity 共同验证。

## 4. 有限合同族目录

以下 12 族覆盖业务交接；CF00 是基础设施例外。每族按能力/事实唯一owner分别定义concrete Command/Data/Result（Command接收能力方，Data归事实资源方），不能生成一个共同的 `ContractCommand{kind,payload}`。合同族目录是审查矩阵，不在运行期充当 command bus。

| 族 | 业务边界与主要 caller → receiver | 接收方可封存的权威/事实 | 禁止的捷径 |
|---|---|---|---|
| <a id="cf00"></a>CF00 基础设施生命周期 | 进程装配 → Initiator；framework → 各单元 | 已进入阶段、订阅/任务就绪、真实排空、资源释放事实 | 给 Initiator 加 Hub/Base/业务 saga；未排空先关持久资源 |
| <a id="cf01"></a>CF01 定义创作与准入 | workorchd → Workspace；Workspace/应用按各 owner 合同请求 LLM/File/Decision | exact input/candidate/review/confirmation、Definition revision、startup/admission；M3 完成事实 | 保存草稿即准入；确认即启动 Run；自动模式绕过最终候选 |
| <a id="cf02"></a>CF02 激活与执行意图 | Web → workorchd → Workspace → Run；Run/应用按实际接线向 Orchestrator提供已接受事实/typed bootstrap | Workspace 选择事实；Run SM-1/唯一 generation；Orchestrator activation aggregate | 绕过 Workspace/Run 私有准入，Web 直接 drive；首 Run 终结后迟到重试再造 Run |
| <a id="cf03"></a>CF03 不可变资产与能力发现 | System/Workspace/workorchd/Orchestrator → Skill/Soul/Memory/MCPRegistry/LLM/各能力 owner | 有界资产/配置/catalog/snapshot、版本与来源事实 | 返回 registry/store 指针；把 discoverable 当可执行；当前配置热换旧 Run |
| <a id="cf04"></a>CF04 owner 事实与适用性 | Orchestrator → File/Git/Shell/SSH/Web/MCP 等具体 owner；System 请求启动就绪事实 | scoped catalog、target/manifest、feasibility、freshness、repair/reconciliation observation | caller 自报环境满足；事实获取授予 terminal credit；超出声明 target 的 repair |
| <a id="cf05"></a>CF05 provider 文档 | Workspace/workorchd/Orchestrator 的明确阶段 → LLM | exact request/config 的输出/失败/不完整、submitted、usage、audit facts | provider 文本直接变 action；截断当完整；usage 缺失当零 |
| <a id="cf06"></a>CF06 sealed 操作与回执 | Orchestrator → 各 capability owner；owner → Orchestrator 的 receipt 接收合同 | capability effect/receipt；SM-3 execution/预算结算；SM-2 后续事实接受 | 未 seal 即 I/O；owner 写 credit；timeout 后换 key 重做 |
| <a id="cf07"></a>CF07 治理与精确变更 | workorchd/Governance/Orchestrator/Workspace → Decision；风险操作 → Approval；决议 → 对应接收owner | Decision最终候选approve授权决议；Workspace确认校验/新定义；Run replacement；Orchestrator exact采纳；Approval风险resolution | 用Run-bound Approval替pre-Run final approve；编辑后复用旧approve；无限自动授权；resolved当新Run完成 |
| <a id="cf08"></a>CF08 证据准入与验证 | capability/validation facts → Orchestrator；只读结果 → 应用投影 | exact current binding 下的 accepted/rejected evidence、all-of CWI/terminal | exit=0、文件存在、LLM PASS、风险批准直接折算 credit |
| <a id="cf09"></a>CF09 opaque 持久化与恢复 | Orchestrator → Run；各 coordinator → 自己依赖 owner 的 bounded inventory/query | Run opaque CAS receipt；Orchestrator 从 fresh aggregate re-drive | event sequence/projection/cache 恢复 authority；忽略 CAS 冲突继续旧计划 |
| <a id="cf10"></a>CF10 终态与结果交付 | Orchestrator → Result →（由 Orchestrator 继续）Workspace → Run | 有效immutable terminal seal；ResultID；exact有序ack及post-terminal lineage assertions；最终Run terminal | 三 owner 自由互调；Workspace ack 后未核实 Run terminal 就开放新 Start |
| <a id="cf11"></a>CF11 审计与投影 | owners → Eventstream；workorchd → owners 的只读合同；事实通知 → projection | 不可变审计和带源 revision 的派生 view | Post 成功当业务成功；SSE 顺序决定 authoritative state |
| <a id="cf12"></a>CF12 删除/退休与停机收束 | Workspace deletion coordinator → Run/Orchestrator/各资源 owner；进程 → 全体 lifecycle | Run retirement fence、Orchestrator quiescence、各 owner tombstone/purge proof、Workspace 完成 | 从 Application 另开删除 saga；跨 owner 直删；误删用户源数据；复生已删除对象 |

CF08 不是额外的“证据服务”或新的 owner；CF09 也不是共享状态管理器。涉及多个 owner 的业务用例由恰当 Module 协调，但每次实际状态写仍由接收 owner 私有入口完成。CF00/CF12 的进程静默与 Workspace 删除是不同 scope，不混用一套全局业务状态。

### 4.1 合同→转换→双方→oracle→分层案例追踪

下表将同一个承诺贯穿三层验证。U/P/W为设计案例组：U=单owner提供方/调用方符合性，P=真实双方及必要最小对端，W=公开Web流程；具体用例展开引用第9节V编号。转换栏只用已观察到的正式机器/资源事务族，尚待设计的业务扩展明确标注，不命名虚构internal event。

| 合同 | 关联转换/写集 | 调用方consumer → 提供方provider | 独立oracle | U 单owner | P 双端/最小切片 | W 公开Web全环 |
|---|---|---|---|---|---|---|
| CF00 | 进程Setup/Run/停止输入/排空/Teardown；无业务SM | 进程/framework → Initiator及各单元 | 真实存活handler/task数、释放次序、句柄仍可用性；不信Ready布尔值 | U00：V35/36，失败阶段及幂等释放 | P00：真实Hub/任务/持久provider，在途调用时停机 | W00：Web运行中重启，恢复原Run，不另开Run |
| CF01 | workspace.creation/definition_specification/definition_authoring_confirmation/definition_admission | workorchd → Workspace；生成/资源/澄清分别→LLM/File/Decision | 原Goal/资源/义务清单+exact candidate；Goal/DoD/journal前后像；Run count | U01：V01–06/18/26/28/43–45 | P01：真实应用/Workspace及资源、审查/确认合同 | W01：Web创建→review→exact confirmation→M3，Run=0；TUI创建入口不存在 |
| CF02 | workspace.execution_metadata；run.creation、SM-1；SM-2/3/4 activation初始化 | workorchd → Workspace → Run；实际typed activation/drive链→Orchestrator | durable request→唯一Run generation映射；slot/Run/aggregate三处一致 | U02：V03–12/26/27 | P02：真实Workspace/Run/Orchestrator，双击/并发/迟到 | W02：手动或已授权自动M4，首Run终结后重放原请求仍无新Run |
| CF03 | 各资产owner自己的资源版本/默认升级；不新增Runtime SM | System/Workspace/workorchd/Orchestrator → 具体资产owner | 独立保存的源内容/hash/provenance、分页完整性及原Run snapshot | U03：V02/05/20/37/40 | P03：真实caller+每类资产owner，current与sealed snapshot分开 | W03：变更默认配置后新Run用新快照、旧Run恢复不变 |
| CF04 | capability观察本身不推进Core；由Orchestrator私有SM-2 seal事实/绑定，必要时同CAS协调SM-3/4 | Orchestrator → 对应资源/能力owner | 独立资源inventory和目标范围；oracle不从LLM候选反推 | U04：V01/02/05/12/21/25/40 | P04：真实Core+owner，scope/版本漂移与feasibility stale | W04：WebRun实际资源变化后fresh acquisition，旧facts不授credit |
| CF05 | LLM调用/文档facts；Workspace私有request journal或Orchestrator SM-3 request/correction/预算 | authoring阶段/Orchestrator → LLM | 预固定request/config及受控provider原始返回；独立计数与usage事实 | U05：V03/04/09/15/18/19/47 | P05：真实LLM adapter+caller，submitted无outcome不replay | W05：真实provider调用与断流/失败，诊断可见且无假完成 |
| CF06 | effect owner私有准入；SM-3 sealed/claimed/owner_committed/evidence_committed/outcome_unknown；既有同CAS写集 | Orchestrator → capability；capability → Orchestrator receipt接收合同 | 独立effect sink/资源前后像；调用次数、execution key/fence和预算对账 | U06：V03–17/22/41/42 | P06：真实Core+一个effect owner+真实receipt持久边界，注入丢响应 | W06：实际工作→typed receipt→no-credit重观察/验证；ImplementationWork未准入则该分支不能PASS |
| CF07 | decision.lifecycle/approval.lifecycle；Workspace confirmation/admission；Run replacement；Orchestrator匹配采纳 | 应用/Governance/Core → Decision/Approval；决议→Workspace/Orchestrator/Run各自合同 | 冻结的旧/新候选差异+人工/授权主体与scope；旧版本不变；批准与实际应用分别核对 | U07：V05/13/28/29/43 | P07：真实Decision+Workspace；运行治理加Approval/Core；新版Definition应用扩展未完成须阻塞 | W07：完整原因ticket→修改→最终exact approve→新Definition→获准新Run；旧历史不可变 |
| CF08 | SM-2证据/CWI/all-of/terminal；SM-3 evidence结算同owner协调；无独立证据owner | capability验证facts → Orchestrator | 预先固定逐required obligation；错对象/漏项/旧binding/0测试反向控制 | U08：V21–25/44–46，错误必须被oracle抓住 | P08：真实validator owner+Core，receipt与evidence语义区分，CAS切点按实际原子写集 | W08：真实交付物逐义务验证；required缺一项不成功 |
| CF09 | Run opaque存储CAS；Orchestrator SM-2/3/4私有aggregate；SM-4恢复与lease | Orchestrator → Run；recovery查询→对应owner | durable preimage/revision+完整写集差分；新进程清空cache仍可读；独立effects计数 | U09：V06–12/15/16/39 | P09：真实Run+Orchestrator+拟支持的持久adapter，切点重启 | W09：Web创建同Run在进程重启后续驱；不支持schema明确阻塞而非重建 |
| CF10 | SM-4 immutable material/ack；result.lifecycle；workspace.lifecycle；Run SM-1最后终态 | Orchestrator依次→Result、Workspace、Run | 预封存material hash+各owner确切ack；Result原始ID；有序完成前缀与post_terminal_delivery lineage assertions | U10：V03/04/10/30/31 | P10：三owner真实交付，逐ack丢失/重复/错identity | W10：Web完整Result可读、有效seal+exact三ack+post-terminal lineage成立；slot空但Run未terminal不能新Start |
| CF11 | Eventstream事实/位置；workorchd.projection_lifecycle；不写业务SM | owners → Eventstream；应用query/订阅→owners | authoritative快照与projection源revision差分；断开SSE不改owner | U11：V19/20/34/38/40 | P11：真实Hub/Post与查询、脱敏和分页 | W11：Web断线/刷新恢复展示，通知乱序无业务重做 |
| CF12 | workspace.deletion；run.workspace_retirement；orchestrator.workspace_deletion；各资源owner deletion族；进程停机另属CF00 | Workspace → Run/Orchestrator/资源owners | 有序inventory/token、实际资源空集/保留集、首次计数journal/proof、tombstone | U12：V06/11/32–35/40/41 | P12：真实Workspace/Run/Core+选定资源owner，退休与CAS/物理purge切点 | W12：Web明确授权删除/保留流程；源资源不误删，fresh reload不复生 |

每个实际topic都要落到一条合同记录及至少一个U案例；有真实跨owner依赖的topic必须有对应P案例。核心用户承诺必须落到W案例；纯后台技术合同可由W案例的故障切点间接覆盖，不能编造一个多余UI按钮。U/P/W能复用输入，但关键oracle不能直接调用被测reducer/validator给自己判分。

### 4.2 固定合同的实现红线

- 所有跨Module/Block业务I/O通过同一应用EventHub。维护对应资源/状态的能力owner在自己的pkg/events唯一定义topic和concrete Command/Data/Result：同步命令由该能力接收owner提供合同；事实通知由事实资源owner定义并发布，合法订阅者导入同一Data，不按subscriber复制topic或DTO；调用方不得复制同能力DTO/topic或调用对方service、repository、adapter、reader callback
- 同步需要确认的业务使用Send和magicCommon event.Result；调用方检查结果存在、具体类型和error。异步Post仅通知，不依赖result.Set，不承担强一致写入回执
- 禁止新增通用Envelope、events.Response、任意kind/payload command bus、通用Send/Subscribe facade、reflect类型注册/分发和跨owner JSON兼容桥
- 禁止用map[string]any、[]any、无约束any或开放key/value袋承载合同语义。已有bounded typed字段表也必须有确定schema/允许集合，不能借“typed”之名退回万能payload
- 禁止传递repository/store/registry/recorder、数据库连接、channel、io.Writer、http.ResponseWriter或其他共享可变句柄；返回值中的slice/pointer必须有明确copy/ownership边界
- 共享纯值包可以保存真正owner-neutral的不可变ID/时间等值，不拥有业务topic/command/result或可变资源；owner-specific port生产实现只保存Hub/source/typed mapping
- private事件/transition descriptor不能导出成外部可选择的reducer入口；manifest仅作验证清单，不在运行期赋予调用权

<a id="generic-contract"></a>

## 5. 每项合同必须冻结的语义

<a id="compatibility"></a>

### 5.1 身份、版本与兼容

冻结记录应列明：合同文档版本、source SHA、receiving owner、实际 topic/Command/Result 路径、合法 caller、schema/validator/hash 版本、持久化读取版本、受影响消费者和测试向量。这里是文档和构建清单，不要求每个消息新增一个 version 字段。

- 合同兼容包括意义，不仅包括 JSON/Go 字段。改变目标作用域、默认权限、unknown 解释、幂等窗口、预算扣减、terminal 条件，都算语义变化
- 新 optional 字段只有在“缺省不扩权、不变验收、不改旧 hash、不丢历史事实”且双方验证通过时才可能兼容
- 新 required 字段、新 enum outcome、identity/hash 算法、ordering/CAS/fence 语义变化，不能默认为向后兼容
- 未识别状态/版本/结果臂：权威路径 fail-closed；展示可以说明不支持，不能默认为 success 或自动重算
- 同进程组件不必发明运行期版本协商总线。构建/装配时固定匹配版本；持久数据、外部协议、跨版本运行再按真实需要声明版本
- 只有当前构建明确支持且能按原identity/hash校验的旧Run，才可按其原schema读取与重驱；不支持或损坏的数据load fail-closed，保留历史并进入人工迁移/恢复评审。新配置只作用新activation，不以当前compiler重写旧Definition、归档和批准
- 升级要证明旧数据可读、在途操作可对账、恢复不会重复副作用；回滚要证明旧构建能读新数据。不能用数据库快照抹去已经发生的 operation/receipt/消费
- Workspace Block→Module 的角色变化不是自动改 owner ID/topic/存储 key 的理由；如必须改变，单独声明映射、唯一写入切换与旧记录读取规则

### 5.2 命令/结果语义维度（按需落在既有 concrete 类型）

每份 owner 合同说明下列维度的实际字段、来源和空值规则；不适用写明理由。不得把下表机械复制成所有消息的公共超大 DTO。

| 维度 | 必须回答 |
|---|---|
| identity/scope | 哪些 ID 只局部唯一；跨边界怎样绑定 Workspace、Run、generation、resource、operation、request；pre-Run 不能伪造 RunID |
| source/recipient | 谁可调用、怎样验证合法来源；owner 标识不是用户权限凭证 |
| semantic subject | 对哪一版 Definition/startup/manifest/candidate/binding/target 生效；哪些是事实引用、哪些是封存权威 |
| precondition/CAS | expected revision、preimage、原子成功条件、冲突返回 observed revision；读旧值不得自升 expected revision |
| effect authority | 允许的具体 operation/target/data destination；风险 grant、撤销/失效、fence 和真实 adapter 的执行前核验 |
| boundedness | 最大条数、字节、分页、deadline、尝试/资源消耗上限；完整性标记，截断不冒充全量 |
| request/idempotency | key 的作用域、payload/hash绑定、生命周期；同 key 同意图如何查询/重放，同 key 异意图怎样拒绝 |
| result truth | accepted、committed、duplicate、stale、rejected、not found、incomplete、unknown分别意味着什么；不强求各 owner 使用同名 enum |
| error | 传输错误与业务拒绝、永久不支持与暂态、权限不足与信息不足怎样分开；保留 bounded 原因和因果链 |
| cancellation | 未执行、已接受、进行中、已产生效果时的取消意义；取消请求不等于回滚或副作用不存在 |
| durability/recovery | 哪一步前必须 CAS/journal；丢响应从哪一权威记录读取；恢复下一动作由哪个 owner决定 |
| receipt/ack | 回执确认接收、执行、持久化还是终态；怎样绑定原 intent/material；missing ack不能当成功 |
| observability/privacy | 可审计 identity与已发生事实；输出脱敏；诊断不泄漏secret，不承担authority |

### 5.3 CAS、fence 与幂等的非等价性

- CAS 解决同一 owner 当前预像的竞争；fence 拒绝过期持有者；幂等 identity 控制同一逻辑请求重复效果。三者不能互相替代
- revision/generation由对应 owner 单调分配，不能用时间戳、消息到达顺序或 RunID 字符串猜测
- CAS 失败后禁止继续执行原计划；从 returned/fresh aggregate 重新读取 phase/generation/current binding，再决定是否重试
- 相同 key、相同 canonical intent：返回原 durable outcome 或当前明确状态；不能再扣预算、再建 Run、再写 artifact
- 相同 key、不同 payload/hash：冲突，零新增效果；不得当“用户的新要求”默默覆盖
- 幂等保留期必须覆盖可能迟到的重试、进程重启和 in-flight 生命周期。历史迁移/清理后也必须保持重试拒绝或可判别，不能让旧 key重新获得执行资格
- Fence 必须在实际效果 owner 的线性化边界验证。只在 Orchestrator 检查 fence，不足以阻止已过期 worker 的远端副作用
- 对不提供fence/幂等/查询的外部系统，不声称exactly-once。记录能证明的事实与unknown，禁止盲目补发；仅走合同明确支持的有界对账或失败。只有真正存在typed业务决策依赖时才进入治理，不能把技术unknown一律包装成ticket

### 5.4 中断与未知结果的持久化切点

| 切点 | 恢复后必须可证明 | 下一步 |
|---|---|---|
| admission/CAS 前中断 | 无已接受的 durable intent；不能仅以客户端断线推断这一点 | 查询确认未接受后，按同 request重试 |
| CAS成功、I/O尚未开始 | 封存intent/dispatch/预算reservation存在；还必须能证明尚未提交且该claim仍可首次执行 | fresh-load后仅在首次执行资格确证时按原identity/fence执行；缺receipt或“未证明发生”不够，否则转unknown/reconciliation |
| I/O 已开始、结果尚不可知 | intent、execution identity和预算承诺仍存在；不声称失败/零消费 | owner查询或授权对账；不换key再次执行 |
| owner效果已完成、caller丢响应 | owner可提供原 receipt或有界可信观察；无此能力则unknown | 查询/重放原事实，再经Core准入 |
| receipt已接受、后续CAS响应丢失 | fresh aggregate可判定是否已结算 | 返回原结果或续交付；不得重复credit/扣预算 |
| terminal material已CAS、Result ack丢失 | 原材料hash和delivery identity固定 | 查询/重放Result同一交付；不重算材料 |
| 任一delivery ack已CAS后重启 | 按已完成前缀推进下一owner | 不跳级，不因为投影显示完成而提前释放屏障 |

模型调用顺序还必须固定为preflight→fresh-load核对current intent→session CAS→provider物理调用；preflight可能耗时，不能沿用它之前的旧snapshot直接调用。CAS拒绝或current intent已退休时，零新增provider call。

provider已submitted但无outcome不得盲目重放；依现有Core合同走provider_submission_unknown的明确结果，不把它自动塞入通用恢复轮询。

这里的“CAS前/后”必须指明哪一个 owner/哪次事务；跨 owner 不存在一个虚构的全局原子 CAS。每次交接使用持久意图、幂等接收和确切 ack收敛。

### 5.5 超时、取消、权限与预算

- 区分 Hub尚未执行、handler已执行、外部系统是否提交以及 caller是否收到响应。规范规定同 Hub `Send` 已开始执行要等待实际结束；网络/provider 超时仍可能留下 unknown
- 同步子事件保留父 context，使 lane祖先链可检查；禁止丢 context绕过顺序或循环等待检测
- 取消请求停止后续尚未授权工作；进行中效果要有明确取消能力/回执。已完成效果不能被取消请求抹除；必要补偿另行取得权限
- 每个可能物理调用在效果前完成该类消费/预留的持久承诺；receipt/usage按相同 identity结算，unknown不得退回可花余额
- 产品预算视图只确定性汇总现有子账，不建立第二份可修改消费 authority。各维度物理次数、logical attempt、token/金额、proof/recovery预算不互相代替
- 并发最后一份预算只允许一条合法 reservation；late usage必须回原调用，不允许归到新Run/新attempt
- 风险 grant精确绑定主体、scope、operation/target、版本和有效性；approval不放宽Definition、不授credit、不覆盖无边界的未来操作
- recoverable wait必须有明确condition/trigger/deadline/预算/owner；耗尽或不支持时走规定后继。不能留下没有可达后继的active+nonterminal
- Run终态或空slot不能证明所有前驱effects已静默。目标要求：新Run冲突副作用前必须核对前驱unknown/in-flight的owner receipt和实际资源，无法证明安全时阻塞；不能继承旧credit或抹去旧unknown消费。此跨Run屏障尚未证明全覆盖
- 技术CAS冲突/安全恢复guard耗尽与业务预算耗尽分别处理；前者不随意制造业务failed/新ticket，按已冻结owner恢复合同保留可诊断事实

### 5.6 具体卡共同规则（唯一规范）

以下C规则适用于具体合同卡的引用；除卡列出的特化，不向所有Go消息机械新增相同字段。

<a id="cc01"></a>
**CC01 身份/空值/来源。** 同步合同必须取得正确concrete Command和合法调用上下文；按实际Validate、handler及卡列出的目标加强guard处理必填、可空和size。全局asset query可以没有Workspace/Run；pre-Run必须没有伪Run；Run期写入必须精确绑定Workspace/Run/generation/operation。零revision不是“任意版本”；仅显式create-if-absent合同可定义0。裸owner/source字符串不能替代受控装配与权限校验。未校验字段不是无限授权。

<a id="cc02"></a>
**CC02 输出臂与caller后继。** 同步Send先检查result非nil、transport/error、确切Result类型，再核验identity/hash/version及合法单臂；error+payload不得当成功。合法空列表是查询成功，找不到单对象按该卡的found/not-found语义。无稳定机器码的现有error沿用确切类型/错误链；不得发明全局code。invalid/unsupported停止并可展示；stale/conflict先fresh-query再重新评估，不重放旧决定；duplicate校验原identity后复用原outcome；unknown保留原身份并进入卡指定query/reconcile，不能宣告失败等同未发生。

<a id="cc03"></a>
**CC03 原子承诺/副作用。** 本地事实写以接收owner私有事务/CAS为线性化点；不承诺跨owner原子。sealed effect先持久准入、reservation及execution identity，再由能力owner检查exact seal/target/fence后I/O；effect与Runtime持久提交不是同事务，存在unknown切点。receipt作者是能力owner，当前Runtime receipt持久提交归Orchestrator；working receipt涉及的SM-2/SM-3协调必须一份aggregate单CAS，不能拆成两个承诺。query通常不写业务authority，但query receipt/cache有私有持久副作用时卡必须明列，不能把read lane当零物理写入。

<a id="cc04"></a>
**CC04 有界恢复/预算。** deadline/cancel仅限制本次等待，不能抹去已提交操作/已发生消费。query失败可在原scope有界重试；mutating/effect丢回复先query原identity，absence未经owner强语义证明不可换key执行。reservation、provider submitted/usage unknown及settlement归既有Runtime账，组件不能自授新额度；非Runtime资产管理仅服从请求deadline/size/pagination，不虚构运行预算账。recovery耗尽或owner不支持对账必须明确阻塞/治理/终止后继，不能无限active。前驱unknown/in-flight未对账时，同资源新Run不可借新key绕过。

<a id="cc05"></a>
**CC05 兼容/复制。** 仅固定SHA协同构建受支持；旧持久schema是否支持按实际validator与卡记录逐项声明，未知版本fail-closed。`nil`、空串、0、空slice、ContentPresent等presence flag的意义不得互换。返回及保存slice/pointer做typed detached clone；不跨owner传可变store/repo/adapter句柄。新required/enum/identity/hash/authority变化是破坏性，须变更票据与相关CT/VS/MIG影响分析。原业务义务的变更走Decision精确ticket和Workspace新Definition；本工程文档冻结不新造用户业务confirmation。

<a id="cc06"></a>
**CC06 通知/投影/生命周期。** Post仅通知；handler允许nil result，不靠Set提供强回执。调用方从owner typed query重建投影，乱序/丢失不改authority。Setup/Start登记订阅，停止输入→取消/等待task与handler真正排空→Teardown→最后释放Initiator持久资源；超时保留仍被依赖资源。实际没有接收topic的单元按CF00生命周期及出站边验收，不发明topic补数。

### 5.7 重复能力形状的共享特化

这些CS仅是文档特化引用，不新增共享Command、运行期kind、envelope或注册表；具体类型和guard仍由各唯一owner维护。

<a id="cs01"></a>

**CS01。** owner私有资源mutation；成功只证明该资源已接受，不能推出Run/Goal完成。source没有durable idempotency/revision字段时不许宣称exactly-once/CAS：响应丢失先按该资源ID查询，不能自动重发可能重复的Create；需新语义时单独变更CT版本。

<a id="cs02"></a>

**CS02。** sealed具体operation准入：检查owner/kind、scope、target、operation identity、payload hash、execution key/fence和grant；先query原receipt，再按准入执行并commit原identity回执。accepted/no-op/rejected/governance/unknown按真实Receipt区分，receipt不授terminal credit。

<a id="cs03"></a>

**CS03。** target change：未封存草稿只由Workspace合法创作路径修改；sealed Definition下拒绝直接CRUD，原义务改变必须NC04 reasoned ticket→exact candidate→新Definition，不能原位覆盖旧Goal/DoD历史。

<a id="cs04"></a>

**CS04。** ticket/approval/trace/resolution各由自己的owner持久；exact request、最终选择、scope/version与合法actor匹配。风险许可、正式decision ACK、Workspace adoption与目标验收四者不同；conflict/stale不改历史，后继引用NC04或Core治理边。

<a id="cs05"></a>

**CS05。** 以当前binding/CWI、generation、request、owner revision和target范围独立观察；satisfied/unsatisfied/unverifiable是事实臂，不能自行保留credit或重做effect。失败后Core fresh-load并按Core恢复边处理。

<a id="cs06"></a>

**CS06。** 删除/退休/静默事实只接受exact Workspace deletion identity、generation/请求hash/模式；与新写入/Start的竞争由owner fence裁决。保持最初journal/proof，不用purge后空集重算成功计数；恢复引用CC03/CC04及NC05，不能删除用户源资源。

<a id="cs07"></a>

**CS07。** 只验证该owner对候选或操作的范围/形状/适用性；通过不封存dispatch、不执行effect、不授CWI credit。catalog/config漂移需重新获取，不复用旧准入。

<a id="cs08"></a>

**CS08。** 应用facade只编排下一层owner的typed合同并映射结果；自身不直写资源/repository，不把transport成功当owner成功。底层owner的权限、幂等、scope和unknown规则必须完整传回。

<a id="cs09"></a>

**CS09。** 异步事实通知；Data由本资源owner唯一定义，subscriber只做projection/审计或在自己权限内fresh-query；无Command、无同步Result、Post不证明业务提交。重复/乱序/丢通知不能重做effect。

<a id="cs10"></a>

**CS10。** 执行intent或lifecycle fact由owner-private ingress核验；无可任意设置State的输入。稳定intent→唯一Run/generation，重复返回原binding；旧/终态后迟到请求不能再建Run。跨owner激活与终态屏障见NC03/NC05。

<a id="cs11"></a>

**CS11。** 有界能力/目标/manifest观察，只提供owner事实和原revision；caller对identity/current authority作接受决定。查询成功不写Core authority；缺失/过期后有界fresh-query。

<a id="cs12"></a>

**CS12。** 现有authoring/specification/confirmation/admission事务仅证明其实际journal/CAS局部阶段；expected revision/input/candidate hash、scope和presence guard见下方真实入口。目标NC00/NC01与NC04独立增量，不从单个成功返回推断M3、adoption或新Run。

<a id="cs13"></a>

**CS13。** 读/写typed RuntimeStateRecord（持久owner不接管SM-2/3/4语义）、drive及恢复依据owner持久记录，不能由通知/缓存重建authority。CAS与lease/fence分别检验；conflict fresh-load，unknown先查询原ref/revision。具体状态写集仅引用Core/Runtime逐edge。

<a id="cs14"></a>

**CS14。** 读取确切ID/scope；列表按Cursor/Limit/NextCursor有界遍历，合法空集合与单对象not-found不同。返回值detached，不给执行/验收权威；失败只可在原scope有界重查。该query自身不建立Runtime预算。

<a id="cs15"></a>

**CS15。** 返回当前可行性/未来目标可验证性事实，保存不可支持的原因与scope；不是实际效果或验收证据，不把unknown/unverifiable当PASS。

### 5.8 验收适用集的共享引用

这些CP只是已在唯一验收规格定义的case集合引用，不是新测试算法；每张CT的实际字段/guard/owner保持独立，不能因为共用CP就合并验收结论。

<a id="cp01"></a>

**CP01：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V03](validation/验收规格.md#vs-v03)、[VS-V04](validation/验收规格.md#vs-v04)、[VS-V05](validation/验收规格.md#vs-v05)、[VS-V26](validation/验收规格.md#vs-v26)、[VS-V28](validation/验收规格.md#vs-v28)、[VS-V43](validation/验收规格.md#vs-v43)、[VS-V44](validation/验收规格.md#vs-v44)、[VS-V45](validation/验收规格.md#vs-v45)

<a id="cp02"></a>

**CP02：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V03](validation/验收规格.md#vs-v03)、[VS-V04](validation/验收规格.md#vs-v04)、[VS-V07](validation/验收规格.md#vs-v07)、[VS-V08](validation/验收规格.md#vs-v08)、[VS-V09](validation/验收规格.md#vs-v09)、[VS-V10](validation/验收规格.md#vs-v10)、[VS-V11](validation/验收规格.md#vs-v11)、[VS-V12](validation/验收规格.md#vs-v12)、[VS-V13](validation/验收规格.md#vs-v13)、[VS-V14](validation/验收规格.md#vs-v14)、[VS-V16](validation/验收规格.md#vs-v16)、[VS-V17](validation/验收规格.md#vs-v17)、[VS-V22](validation/验收规格.md#vs-v22)、[VS-V41](validation/验收规格.md#vs-v41)、[VS-V42](validation/验收规格.md#vs-v42)

<a id="cp03"></a>

**CP03：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V03](validation/验收规格.md#vs-v03)、[VS-V04](validation/验收规格.md#vs-v04)、[VS-V09](validation/验收规格.md#vs-v09)、[VS-V15](validation/验收规格.md#vs-v15)、[VS-V18](validation/验收规格.md#vs-v18)、[VS-V47](validation/验收规格.md#vs-v47)

<a id="cp04"></a>

**CP04：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V03](validation/验收规格.md#vs-v03)、[VS-V04](validation/验收规格.md#vs-v04)、[VS-V30](validation/验收规格.md#vs-v30)、[VS-V31](validation/验收规格.md#vs-v31)、[VS-V46](validation/验收规格.md#vs-v46)

<a id="cp05"></a>

**CP05：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V03](validation/验收规格.md#vs-v03)、[VS-V05](validation/验收规格.md#vs-v05)、[VS-V13](validation/验收规格.md#vs-v13)、[VS-V28](validation/验收规格.md#vs-v28)、[VS-V29](validation/验收规格.md#vs-v29)、[VS-V43](validation/验收规格.md#vs-v43)

<a id="cp06"></a>

**CP06：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V03](validation/验收规格.md#vs-v03)、[VS-V06](validation/验收规格.md#vs-v06)、[VS-V27](validation/验收规格.md#vs-v27)、[VS-V31](validation/验收规格.md#vs-v31)

<a id="cp07"></a>

**CP07：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V05](validation/验收规格.md#vs-v05)、[VS-V21](validation/验收规格.md#vs-v21)、[VS-V25](validation/验收规格.md#vs-v25)、[VS-V40](validation/验收规格.md#vs-v40)

<a id="cp08"></a>

**CP08：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V05](validation/验收规格.md#vs-v05)、[VS-V39](validation/验收规格.md#vs-v39)、[VS-V40](validation/验收规格.md#vs-v40)

<a id="cp09"></a>

**CP09：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V05](validation/验收规格.md#vs-v05)、[VS-V40](validation/验收规格.md#vs-v40)

<a id="cp10"></a>

**CP10：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V05](validation/验收规格.md#vs-v05)、[VS-V46](validation/验收规格.md#vs-v46)

<a id="cp11"></a>

**CP11：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V06](validation/验收规格.md#vs-v06)、[VS-V10](validation/验收规格.md#vs-v10)、[VS-V11](validation/验收规格.md#vs-v11)、[VS-V12](validation/验收规格.md#vs-v12)、[VS-V39](validation/验收规格.md#vs-v39)

<a id="cp12"></a>

**CP12：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V06](validation/验收规格.md#vs-v06)、[VS-V11](validation/验收规格.md#vs-v11)、[VS-V32](validation/验收规格.md#vs-v32)、[VS-V33](validation/验收规格.md#vs-v33)、[VS-V34](validation/验收规格.md#vs-v34)、[VS-V41](validation/验收规格.md#vs-v41)

<a id="cp13"></a>

**CP13：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V37](validation/验收规格.md#vs-v37)、[VS-V39](validation/验收规格.md#vs-v39)、[VS-V40](validation/验收规格.md#vs-v40)

<a id="cp14"></a>

**CP14：** [VS-V01](validation/验收规格.md#vs-v01)、[VS-V02](validation/验收规格.md#vs-v02)、[VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V40](validation/验收规格.md#vs-v40)

<a id="cp15"></a>

**CP15：** [VS-V19](validation/验收规格.md#vs-v19)、[VS-V20](validation/验收规格.md#vs-v20)、[VS-V38](validation/验收规格.md#vs-v38)

<a id="cf00-common-behavior"></a>

### 5.9 CF00共同生命周期承诺

- 调用方：进程/framework；准入以实际Setup/Start配置为准，缺必需依赖则拒绝，不留下假ready
- 原子范围：仅本单元自己的阶段/资源；部分初始化失败逆序清理，已完成阶段不得重复关闭；未排空handler/task前不释放共享依赖
- 取消/unknown：停机超时必须保留仍被使用的资源与失败事实，下次继续未完成阶段；不能把timeout当drained
- 预算：不拥有Runtime execution预算；startup/shutdown deadline有界，不能借生命周期重置Run预算
- 支持：固定SHA协同装配；角色/依赖图改变须重验顺序，不允许跨version热换仍在途资源
- 验收：[VS-V35](validation/验收规格.md#vs-v35)、[VS-V36](validation/验收规格.md#vs-v36)；SG1生命周期+SG2真实依赖排空；产品最终SG3另验重启恢复

<a id="lifecycle"></a>

## 6. 22 个运行单元的合同与单独验收映射

当前角色来自指定SHA的结构基线；Workspace已迁移至kernel/workspace并成为Module，合计仍22：1 Initiator + 5 Modules + 16 Blocks。此项源码角色迁移已完成，不等于完整业务协调合同已经实现。以下“验收”全部为待实施验证设计。

| # | 单元：当前→目标 | 合同责任与接收owner | 单独验收最小证明/边界 |
|---|---|---|---|
| 01 | workorchruntime：I→I | CF00；单一repository provider/factory基础能力，不参加业务Hub合同 | 部分初始化失败、句柄唯一、重复释放、被依赖时不得提前释放；无Base/Hub/业务策略或综合runtime容器 |
| 02 | workorchd：M→M | 消费CF01/02/03/07/10/11；Web请求响应、typed bootstrap、projection | M3无Run；默认手动/有界自动同合同；双击/刷新/断线/旧页面不重做；projection不可写authority。单独通过不证明Core真实完成 |
| 03 | governance：M→M | CF07/11/00；用Decision/Approval合同组合政策决议，维护本owner trace | 许可范围、到期、旧proposal、重复通知、政策拒绝、人工/自动actor准确；无票据repo直连，不将policy命中当完成 |
| 04 | orchestrator：M→M | CF02–12的运行期协调；唯一SM-2/3/4；Runtime receipt持久owner | 四机写集、all-of、sealed dispatch、预算/unknown、恢复与终态delivery；同CAS防撕裂。ImplementationWork仍未准入，需先关闭该缺口；能力stub不能证明真实effects |
| 05 | system：M→M | CF03/04/00；组合各默认资产与就绪合同 | 幂等ensure，缺失必需依赖时fail-fast；部分ready不冒充全ready；不成为配置资产owner或runtime驱动者 |
| 06 | approval：B→B | CF07/11/12；风险scope及resolution owner | exact grant、旧revision、过期/拒绝/撤销、重复resolve、fresh-load与删除fence；approval不授目标credit |
| 07 | decision：B→B | CF01/07/11/12；ticket、完整原因、最终候选approve授权决议与resolution owner | 完整原因/差异可追溯；编辑后旧approve无效；resolution与实际采纳分开；pre-Run不造Run；重复/迟到/新旧candidate |
| 08 | eventstream：B→B | CF11/12/00；审计记录/查询/position owner | scoped分页、重复/缺口、脱敏、Post nil result、tombstone及position floor；丢审计不能重建/改写业务authority |
| 09 | file：B→B | CF01/04/06/08/12；目录、目标观察、具体文件effects | 路径/符号链接/归属范围；expected hash/fence、重复写、partial effect/unknown、真实内容hash、源目录不误删；文件存在不授credit |
| 10 | git：B→B | CF03/04/06/08/00；版本/工作树事实及授权Git操作 | repo/worktree身份、dirty/base revision、并发变更、部分启动清理、commit/push等分开权限与回执；不能以commit存在判Goal完成 |
| 11 | llm：B→B | CF03/05/11/00；provider调用与文档事实 | request/config精确匹配、wrong shape、截断/断流、重复response、submitted+usage unknown、secret脱敏；不能判semantic authority |
| 12 | mcp：B→B | CF03/04/06/08/00；按registry快照调用远端能力 | schema/版本漂移、timeout-after-effect、远端无幂等能力、取消/订阅真实退出；registry可发现不等于执行权限 |
| 13 | mcpregistry：B→B | CF03/11；server/capability注册资源owner | bounded catalog、revision/schema/provenance、重复注册/失效、无secret泄漏；不调用远端effects、不暴露mutable registry |
| 14 | memory：B→B | CF03/11/12；知识资产/recall trace owner | 来源/范围/分页/预算、selected与实际注入一致、fresh reload、删除/保留；旧知识不能成为当前证据或无限授权 |
| 15 | result：B→B | CF10/11/12；Result与Artifact元数据owner | exact material/hash、owner-derived ResultID、重复投递/错scope/丢ack、bounded内容读取/完整性；不重新验收或选择next action |
| 16 | run：B→B | CF02/09/10/12；SM-1、唯一generation、replacement、opaqueCAS | 单active资格、防迟到重复Start、private transitions、revision冲突、retirement与CAS竞争、predecessor/successor、精确ResultID |
| 17 | shell：B→B | CF03/04/06/08/00；命令/验证执行能力 | cwd/env/权限/recipe exact、timeout/子进程/真实kill与退出、输出截断、实际隔离、exit=0但零测试、预算/receipt |
| 18 | skill：B→B | CF03/11；技能资产owner | identity/version/hash/provenance、缺失或变更、snapshot不可变、有界内容；技能文本不能扩大操作授权 |
| 19 | soul：B→B | CF03/11；行为配置资产owner | defaults幂等与升级版本、当前值/Run snapshot分离、无旧Run热更新；配置不能覆盖Goal/DoD |
| 20 | ssh：B→B | CF03/04/06/08/00；远端连接/操作事实owner | 主机身份和scope、断线后效果unknown、远端重查/幂等能力、取消与连接排空、禁止盲重试 |
| 21 | web：B→B | CF03/04/06/08/00；HTTP/浏览能力，不是用户Web控制台 | URL/redirect/data destination/网络权限、响应不完整、写请求unknown、实际artifact；HTTP200不等业务验收 |
| 22 | workspace：M→M | CF01/02/07/10/12；Definition、活动资格与唯一删除协调 | M3无Run、候选精确确认、新Definition不可变历史、活动slot/handoff、唯一journal、每ownerack与恢复；不直接写Run/Result |

21个Hub单元保持 `module.go → biz（内嵌Base）→ owner内部service/adapter` 单向组织；Initiator除外。Base是共享基础包，不是第23个单元，也不承担合同注册与业务策略。

## 7. 已实例化合同与覆盖规则

[具体合同目录](contracts/组件具体合同目录.md)逐个列出固定SHA的实际topic、接收owner、消息类型、Data、handler/validator和source/target处置；owner卡已实例化，不能再以本篇模板代替交付。

- 稳定ID：`CT-{owner}-{源码Topic常量去Topic前缀}`。ID不随排序改变；topic重命名/删除留下迁移映射，不回收ID
- CF00–CF12是语义族；CT是实际接收能力。一个族含多个CT，不能用一个族的happy path证明全部CT
- 新业务NC00–NC05在[创建与定义变更合同](contracts/创建与定义变更合同.md)单点维护；与现有CT一对多映射，不声称目标新增名称已存在源码
- 支持状态分开记录：source existing/query/notification/declared-only；target retain/change/new/remove；usage observed/unverified/unused-by-explicit-retirement。未找到caller只能记unverified，不能等同unused
- 同shape能力可共用本规范C规则和owner局部规则，但每个真实topic仍有独立稳定ID、差异和验收适用项
- 任一未读取依赖、缺validator、未定位handler/调用端或未建立持久/恢复保证，都是证据缺项。source coverage 100%只指声明覆盖，不等于语义闭合、测试通过或生产可用

## 8. provider/caller 符合性验证方法

本文统一使用provider表示接收能力请求并提供结果承诺的owner，caller表示调用方，弃用易与消息发送方向混淆的producer/consumer简称。它们不是EventHub消息发送者/接收者的别名：命令由consumer发送，结果由provider产生，通知另按真实发送方向记录。

### 8.1 provider harness：验证真实接收 owner

- 使用真实owner的Biz、实际typed handler/validator/private reducer以及可检查的持久化边界；真实Hub跑订阅和Send/Post语义
- 其他owner可使用有界stub，基础存储可在局部阶段用fixture；但一旦宣称持久恢复通过，就必须使用拟支持的真实持久adapter并销毁进程内状态后重载
- 同时读取前后owner状态、合法写集、出站调用记录、effect sink、预算子账、receipt/ack；不能只assert err=nil
- 畸形输入、错误caller/scope/revision/fence、结果臂、边界大小和非法transition逐项fail-closed；拒绝允许必要审计，但必须零业务扩权/零未授权effect
- 对重复和并发输入检查持久记录、实际effect次数、reservation/settlement/credit次数，区分只处理通知与真正业务动作
- 直接调用Post handler时传nil result，避免Hub recover后只有日志而测试假绿

### 8.2 caller harness：验证真实调用方如何用合同

- 运行真实caller的typed mapping与结果分支，接收owner用遵守冻结合同的可控替身
- 替身必须注入missing result、wrong concrete type、transport error、valid rejected/stale/duplicate/unknown、错误scope/hash、旧revision、分页缺口、迟到结果
- caller必须先校验结果存在/类型/error与binding，再推进自己的私有转换；不得把request成功或非空payload当commit成功
- 使用实际生产EventHub-backed port接线；测试中direct adapter可以验证算法，但不能替代生产边界验收
- 不能让stub接受生产owner本应拒绝的输入，不能让stub直接写Orchestrator终态以证明caller正确

### 8.3 双端合同测试

真实provider与真实caller在同一真实Hub装配；按同一冻结版本验证命令、结果、lane/context、订阅生命周期、大小/分页和错误传播。只取相关owner，仍不需要一次启动全部22单元。

对于同进程新旧构建不支持混用的合同，明确只验证coordinated rollout；不要为了“兼容测试”新造反射bus。对于声明兼容的版本组合，明确验证新provider/旧caller、旧provider/新caller及持久数据两向矩阵；未声明组合必须拒绝。

### 8.4 独立 oracle

独立不是把实现里的expected函数再调用一次，也不是换一个文件/模型名。

- 判据先固定在验收输入中：原Goal/DoD、逐source obligation、禁止效果、目标scope和明确终态；不能从被测planner输出反推expected
- reducer验证采用人工评审转换表/不变量/写集及独立状态差分；不能用同一production reducer生成expected state
- capability效果用独立读取的文件内容/版本、远端查询或受控真实sink事实验证，不能仅信owner自己报告success
- completion按required obligation/CWI逐项核对binding、evidence freshness、检查对象和原始标准，禁止仅比较“completed”字符串
- 注入已知错误候选：漏一个required obligation、错target但测试通过、旧evidence、同源校验伪装独立、0测试exit0、unknown充PASS；oracle必须判失败
- 业务语义无法由确定性检查充分证明时，需要可追溯的独立审查和明确限制。LLM critic/schema合法不构成任意自然语言目标的充分证明
- harness复用纯ID/codec有时合理，但关键acceptance predicate不能与被测实现共享同一错误来源；记录oracle依赖并做反向控制验证

### 8.5 stub 的证明上限

| 替身/局部证据 | 能证明 | 不能证明 |
|---|---|---|
| repository fixture/in-memory store | 局部guard/reducer/identity算法 | crash durability、文件/数据库原子性、真实重载 |
| capability stub | Core对给定facts的接受/拒绝和状态推进 | 工具真实执行、网络权限、外部幂等、可交付产物 |
| scripted provider | 文档shape、边界、错误和预算分支 | 真实模型理解目标、真实provider usage/断流行为 |
| fake clock/scheduler | lease/deadline/budget分支 | 生产timer退出、进程停机真实排空 |
| direct port/调用private driver | 本地函数/某段内部路径 | 生产Hub/公开Web准入、权限、生产路由完整 |
| projection/SSE snapshot | 展示正确性及源revision映射 | authoritative commit、终态交付和恢复正确性 |

单组件报告要明确写出替身清单和未证明的外部性质。不能把“按理其余模块会这样返回”登记为真实全环证据。

## 9. 验收场景唯一来源

47个V场景及其可执行fixture规格、oracle、expect/error分类、故障切点、恢复终点和正反控制，只在[验收规格](validation/验收规格.md)维护。本篇不再保存第二套场景表。历史V01–V47分别映射VS-V01–VS-V47；具体子case以验收规格为准。

合同卡绑定适用VS，不做47×22机械乘积。每个同步CT至少覆盖合法输入、非法shape、错误结果/transport、mutable alias适用检查；纯通知按VS-V38及nil result验证；CF00按VS-V35/36。未声明需要稳定机器码的旧接口不伪造统一错误enum，oracle类别不作为运行期协议。

<a id="sg-levels"></a>

## 10. 三层通过结论与接入门禁

G0–G4进入/退出定义与唯一缺口台账引用[逐项收口总纲](Workorch_逐项收口总纲与首项方案.md)，本篇只定义SG验证层级。普通能力批G4要求适用SG1、SG2和该批局部集成；最终Web能力批及最终产品收口才要求SG3。不得把每个能力批都阻塞在尚未轮到的完整Web全环。

### SG1：单owner符合性通过（对应批次G1的重要证据）

进入：Core/相关合同版本已评审确认，scope、写集、oracle、stub限制齐备。

退出需同时满足：

- provider和caller相关检查通过；typed contract、schema/guard、合法写集、错误/重复/stale/unknown分支齐备
- 有关持久/副作用的关键切点验证，或明确标为尚未证明且不能进入依赖该性质的集成
- 生产结构扫描无跨owner service/repo/callback、generic bus、mutable handle、非法Base/Hub位置
- 结果标明具体SHA和合同版本、实际运行的检查、未运行项、替身范围；无silent skip

结论只能是“该owner在列明假设与边界内符合合同”。不等于真实对端兼容，更不等于完成用户Goal。

### SG2：合同双端/最小切片集成通过（对应G2/G3）

- 真实owner与真实caller经生产Hub/port接线，相关契约链全覆盖
- 接入一个owner时验证它的所有必要入边/出边，不能只跑单个happy path后把它标成“全接入”
- 公用contract修改必须重验受影响边与之前已通过切片；不得以“单模块开发”为由冻结危险混合态
- 真实持久/生命周期、receipt/permission/预算和故障恢复在该切片范围内可观测
- 能力真实效果尚未使用或provider是脚本时清晰保留限制

可按依赖先接Workspace/Run/Orchestrator/Result主脊，再接最小真实能力和治理/恢复，最后扩展其余能力。安全/预算/unknown与恢复前置不得留到最后补；22不是必须串行做22次孤立迁移。

### SG3：公开Web完整业务环通过（最终Web批及产品最终收口适用）

同一候选构建、配置、数据版本下，由公开Web完成：原目标输入→草稿/完整审查→精确确认→M3零Run→显式或已授权有界M4→实际owner工作→逐义务独立验证→immutable closeout→Result/Workspace/Run按序确认→用户可读交付。交付完整性还需有效terminal material seal和post_terminal_delivery lineage assertions；“收到三个ack”这个计数本身不充分。

还必须验证：阻塞/拒绝、budget exhaustion、unknown、取消、重启恢复、ticket精确变更→新Definition/newRun、错误/迟到输入、必要删除/停机边界。自动模式只在明确授权范围内证明，不把人工模式通过推广为自动模式通过。

不能用CLI/private driver绕开Web admission、seed completed aggregate、stub所有工具、删除失败断言或弱化DoD来取得该结论。全环成功需要业务结果与否定控制证据，不只要API 200、运行结束或测试exit0。

## 11. 建议冻结交付清单与评审决定

在批准进入开发前，建议形成一个可追踪的候选包：

1. Core四机转换表、private事件映射、完整写集和跨机不变量
2. CF目录下的真实owner合同清单；每项连接源码类型、caller、handler与validator，覆盖全部22单元责任
3. 现有字段与目标语义差距清单；明确保留/最小修正/新增必要项，不先创建通用抽象
4. 版本/持久兼容/唯一写入切换表；不会影响旧Run历史的证明义务
5. 每owner provider/caller案例、独立oracle和反例，含上述故障切点
6. 最小集成切片与公开Web全环的输入、验收依据、真实/替身边界和停止条件
7. 已批准冻结的具体条目、尚未决条目和变更程序；不得把本工程候选自动标为approved

本次交付构成冻结评审基线，尚不表示每一项具体API、兼容矩阵与验证证据均已冻结。已经确定的owner和产品边界作为后续设计约束；具体未决条目及实现缺口应逐项关闭，不能以一个总括“批准”掩盖缺项。新合同族、新owner、新状态机、共用mutable runtime容器、任意payload消息和全局command bus均不是这次“固定合同”的默认产物。

## 12. 固定来源

- [组织规范及EventHub红线](https://github.com/muidea/skills-repo/blob/6ba50452fccd33c912b07a0648fbb97acf15d756/skills/go-multi-module-dev/SKILL.md)
- [Event使用、Send/Post和生命周期语义](https://github.com/muidea/skills-repo/blob/6ba50452fccd33c912b07a0648fbb97acf15d756/skills/go-multi-module-dev/references/EVENT_USAGE.md)
- [WorkOrch仓库边界与22单元基线](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/AGENTS.md)
- [Runtime总合同](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/docs/00-core/runtime-stage-contract.md)
- [四状态机合同](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/docs/00-core/runtime/state-machines.md)
- [状态转换owner清单](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/scripts/state-transition-owners.manifest)
- [状态模型分类清单](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/scripts/state-model-owners.manifest)
- [Run当前typed消息与opaqueCAS](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/messages.go)
- [Run当前lifecycle事实](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go)
- [Workspace当前authoring合同](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/events/definition_authoring.go)
- [Orchestrator当前typed消息](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/events/messages.go)
- [Orchestrator私有完整写集](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime/internal_event_write_set.go)
- [Orchestrator aggregate CAS收口点](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime/transition_coordinator.go)
- [File具体能力与事实](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/messages.go)
- [File sealed payload校验](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/events/operation.go)
- [LLM文档与usage事实](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/llm/pkg/events/provider_document.go)
- [Result terminal material合同](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go)
- [源规格模型与review scope（最新源码复核入口）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go)
- [Definition admission编译（最新源码复核入口）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/definitionadmission/compiler.go)
- [Definition confirmation模型（最新源码复核入口）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_confirmation.go)
- [Terminal result compiler（最新源码复核入口）](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_result_compiler.go)
- [ImplementationWork生产准入限制](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_implementation_work_policy.go)
- [Approval当前Run-bound生命周期](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/lifecycle_internal_event.go)
- [Workspace确认现有guard](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_authoring_confirmation_internal_event.go)
- [Workspace successor目前限定unsealed clarification](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_authoring_successor.go)
- [File receipt经Orchestrator持久合同](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/runtime_operation_receipt.go)
- [终态完整确认含lineage条件](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime/delivery_acknowledgement.go)
- [Decision当前typed合同](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/events/messages.go)

- [六类review required scope覆盖与差集](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification_review_coverage.go#L8)
- [owner持久review重读校验](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/definition_authoring_specification_ledger.go#L323)
- [terminal current proof适用性](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_evidence_applicability.go#L4)
- [terminal source义务反漏项](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_source_obligation.go#L6)
