# Workorch 22组件组织与调整评审

- 文档编号：WOC-COMPONENT-ALIGNMENT
- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准
- 固定代码基线：`muidea/workorch@5360caf36e081717087557f34d2af3af1c16dc16`
- 组织规范：`muidea/skills-repo@6ba50452fccd33c912b07a0648fbb97acf15d756`，go-multi-module-dev v2.6.0
- 本轮仅更新Markdown设计；没有改项目代码、运行项目测试、触发CI或live。源码路径已与完整固定树核对，叶子行为未重新穷尽审计

<a id="organization-decision"></a>
## 1. 组织结论

当前22个运行单元已是1 Initiator + 5 Modules + 16 Blocks，角色分布已达到既定组织目标。Workspace已从blocks迁至kernel，其余21个保持角色；仍是同一owner，公共ModuleID保留blocks/workspace，不新增第23单元，也不要求22个go.mod。角色结构完成不等于产品闭环或验收全部完成。

全部22个单元进入最终适用验收范围，但不能据此推导全部重写或全部修改业务代码。按能力选择最小完整改动集，既有实现能满足目标时直接复用。Module拥有跨owner协调责任；Block可以拥有单一聚合的正式状态；Initiator只提供一种无业务裁决的基础设施能力。角色由责任闭环决定，不能以“有状态”或目录对称性升级。

业务对象/权威关系由[统一模型](Workorch_统一业务模型与交互关系.md)定义；状态转换由[核心AgentLoop](Workorch_核心AgentLoop与状态机合同基线.md)定义；接口规范由[固定合同规范](Workorch_组件固定合同与分层验收规范.md)定义；实际接口绑定见[具体合同目录](contracts/组件具体合同目录.md)。本文只维护组件分类和组织迁移决定，不复制状态机、通用合同或验收矩阵。

<a id="component-roster"></a>
## 2. 22个组件的分类和组织增量

| 组件编号 | 当前→目标角色 | 组织边界与最小结构动作 | 独特迁移方案 |
|---|---|---|---|
| C03.01 workorchruntime | Initiator→Initiator | 纯持久化基础设施；owner-scoped句柄不带业务裁决或Hub路由 | [MIG01](components/MIG01_workorchruntime.md#migration-delta) |
| C03.02 workorchd | Module→Module | 保留应用组合层；移除TUI创建入口，Web适配不拥有定义/Run副本 | [MIG02](components/MIG02_workorchd.md#migration-delta) |
| C03.03 governance | Module→Module | 保留跨owner治理协调；Decision仍唯一持久proposal owner | [MIG03](components/MIG03_governance.md#migration-delta) |
| C03.04 orchestrator | Module→Module | 保留运行编排与proof裁决；共享compiler是纯包，不能成为新owner | [MIG04](components/MIG04_orchestrator.md#migration-delta) |
| C03.05 system | Module→Module | 复用跨ownerEnsureDefaults启动用例；能力读投影按目标复用workorchd且不成为准入权威 | [MIG05](components/MIG05_system.md#migration-delta) |
| C03.06 approval | Block→Block | 单一风险许可聚合；有状态并不构成提升Module理由 | [MIG06](components/MIG06_approval.md#migration-delta) |
| C03.07 decision | Block→Block | 唯一ticket/resolution/proposal聚合；无需再拆ChangeTicket | [MIG07](components/MIG07_decision.md#migration-delta) |
| C03.08 eventstream | Block→Block | 审计与可重建读投影；不能驱动其他owner状态迁移 | [MIG08](components/MIG08_eventstream.md#migration-delta) |
| C03.09 file | Block→Block | 文件能力及真实内容；不与shared repository/file基础设施混为同一组件 | [MIG09](components/MIG09_file.md#migration-delta) |
| C03.10 git | Block→Block | 单一仓库能力；保留工作树/revision与receipt身份 | [MIG10](components/MIG10_git.md#migration-delta) |
| C03.11 llm | Block→Block | 模型provider能力与调用事实；规划/审查意见不构成业务协调owner | [MIG11](components/MIG11_llm.md#migration-delta) |
| C03.12 mcp | Block→Block | 远端工具执行能力；Registry资产留在MCPRegistry | [MIG12](components/MIG12_mcp.md#migration-delta) |
| C03.13 mcpregistry | Block→Block | 单一注册聚合；目录名mcpregistry不改变既有mcp-registry身份 | [MIG13](components/MIG13_mcpregistry.md#migration-delta) |
| C03.14 memory | Block→Block | 知识/召回资源及来源追踪；不是运行proof权威 | [MIG14](components/MIG14_memory.md#migration-delta) |
| C03.15 result | Block→Block | 正式closeout持久化/交付引用；不再编译或裁决目标结论 | [MIG15](components/MIG15_result.md#migration-delta) |
| C03.16 run | Block→Block | Run SM-1与opaque持久化；不解释Orchestrator的SM-2/3/4 | [MIG16](components/MIG16_run.md#migration-delta) |
| C03.17 shell | Block→Block | 命令及验证观察能力；私有producer接线不改变Block角色 | [MIG17](components/MIG17_shell.md#migration-delta) |
| C03.18 skill | Block→Block | 技能资产；无需求不补齐CRUD或新增技能执行引擎 | [MIG18](components/MIG18_skill.md#migration-delta) |
| C03.19 soul | Block→Block | 行为资产、版本与来源；不另建SoulBinding运行owner | [MIG19](components/MIG19_soul.md#migration-delta) |
| C03.20 ssh | Block→Block | 远端资源与执行事实；不新建远端Run权威 | [MIG20](components/MIG20_ssh.md#migration-delta) |
| C03.21 web | Block→Block | 网络能力，非用户Web UI；入口调整位于workorchd | [MIG21](components/MIG21_web.md#migration-delta) |
| C03.22 workspace | Module→Module（已提升） | kernel路径已落实；保留原ID、定义、活动slot及唯一删除journal，核验恢复 | [MIG22](components/MIG22_workspace.md#migration-delta) |

Base、进程service、HTTP/Web/TUI适配及framework vendor是横切载体；focused package中的DTO、纯compiler、mapper、读模型和算法没有独立生命周期，不另计运行单元。40组DM也不对应40个组件或40张表。owner私有entity/repository状态不能从公开模型包泄漏可变句柄。

21个使用Hub的单元已有Biz并内嵌Base，不重复“补Base”。workorchruntime不持有Hub/Base，继续纯单一持久化基础设施。实际magicCommon为v1.5.18，规范示例v1.5.16不是升级或降级指令；本候选不改变该依赖版本。

<a id="workspace-role-move"></a>
## 3. Workspace角色迁移范围

当前路径`internal/modules/kernel/workspace`已存在，旧blocks/workspace目录不在固定树；main已显式导入kernel路径，module/plugin/Biz分层已迁。`pkg/common/common.go`仍定义ModuleID="blocks/workspace"，这是保留的运行身份，不是遗漏的目录迁移。没有需求不新建Definition、Contract或WorkspaceDeletion运行组件。

已经完成的组织迁移继续保留并回归核验：

1. WorkspaceID、owner键、事件语义、持久化命名空间及历史关系
2. 正式定义/确认物化journal、活动slot及唯一删除journal/reducer
3. 显式入口单次注册；不能新旧路径各注册一个相同owner
4. 在途确认、删除及slot handoff的读回/恢复能力；目录移动默认不迁移数据
5. 公共typed合同与包导入成对更新；不能让兼容桥直接调用其他owner的service/repository

本轮不再实施目录移动；核验范围包含现Workspace目录、`application/workorch/main.go`、plugin注册和生产/测试导入方。相邻workorchd仍做请求鉴权、输入适配、用例组合与响应映射；Workspace维护自己的定义、创建和协调事实。两个Module都参与流程不产生共同写集或第二账本。

可独立验收的组织中间态是：同ID/同数据/同协议，唯一注册，旧journal仍可恢复。定义审查、M3/M4、证明映射不得混入目录批悄然变更；确需同时改API时以同一兼容变更集审查生产者/消费者。角色迁移不证明这些业务能力已完成。

回退包路径前核验当前schema与新引用是否可被旧程序读取、唯一注册是否保持。若发生新的持久业务事实或外部副作用，不能靠旧Workspace快照回滚；沿原journal对账或前向修复，详见[MIG22](components/MIG22_workspace.md)。

<a id="crosscutting-impact"></a>
## 4. 横切变更的组织影响

原9单元局部生命周期缺陷均须按新源码重新判断：workorchd/Run已先保存部分初始化owner；Governance/Approval已context等待及成功退订后出账；Git/MCP/Shell/SSH已逐项账本；Orchestrator Stop已传播错误。共享Base与框架checked桥、owner fresh/drain协议已实现，本轮移入复用与故障矩阵回归，不能再按旧状态要求从零建设。全部真实调用者仍在适用验收范围。

| 横切载体 | 必须成组适配的范围 | 不可接受的结构结果 |
|---|---|---|
| 进程service与实际vendor生命周期 | 复用checked框架桥与错误上达；逐owner验证取消/等待、最后provider释放 | 只添加框架不调用的TeardownChecked；只暴露Quiescer而owner等Teardown才取消 |
| Base任务API | 现有返回error的SyncTask/AsyncTask/Timer及其全部生产调用点 | 只改返回签名却漏接错误；把等待超时解释为任务已结束 |
| 资源取得记录 | 部分Setup/Configure、逐项订阅、worker/session/repo句柄 | started=false就跳过已取得资源；失败清空owner引用 |
| 锁与停止重试 | Run/Governance可panic的外部Hub动作与清理清单 | 持不可恢复锁调用Hub；实际取消前清空topics；无界Wait |
| 合同纯包 | 共享terminal compiler/seal、raw clone、各owner typed adapter | 重复compiler或共享可变业务权威；把实现包算第23组件 |

具体S1–S5状态和F1–F3等业务缺口仅在[总纲缺口台账](Workorch_逐项收口总纲与首项方案.md#gap-ledger)维护；各MIG保留组件专属定位、恢复义务和差分验收。全局生命周期协议见[固定合同规范](Workorch_组件固定合同与分层验收规范.md#lifecycle)，实际反例见[VS-V35](validation/验收规格.md#vs-v35)与[FX-L](validation/固定验收Fixtures.md#fx-l)。

现有生命周期接线须按真实参与者和框架桥回归；新适配仍可分提交准备，未经故障证据不能声明完整安全。保留未完成资源和依赖的中间态是恢复责任，不是成功退出。


### 4.1 当前已落地的合同与focused package分层

- owner的topic、Command/Data/Result仍归各自pkg/events；21个Hub单元沿Module/plugin→Biz(Base)→内部实现，Initiator不加Base。生命周期公共载体是internal/modules/base/biz/base.go与lifecycle.go，均不是第23运行单元
- Workspace公开定义版本纯合同位于internal/modules/kernel/workspace/pkg/contracts/definition_revision.go；定义compiler与来源义务投影在pkg/definitionadmission，eventmapping只做该owner的值映射。纯包无独立启动/存储权威，不再拆Module
- Approval/Decision的biz/contracts/commands.go是owner内部命令结构；不是跨owner对外API。对外仍引用各自pkg/events，不能因名字contracts就绕过EventHub
- internal/pkg/workorchruntimecontract、workorchprojectioncontract、workorchmanagementcontract等承担纯值、只读投影或compiler；引用者仍按实际owner读写，不能把共享DTO当共享可变状态
- workorchd的appservice/activationcore、activationports、ownerreadports与workspaceports继续承接应用组合；System现实现是跨ownerEnsureDefaults启动用例，读投影目标优先复用workorchd，不虚构System已存在的业务topic

这些分层的结构已存在；剩余工作是按能力批核对生产引用、真实授权/Source、生命周期和恢复，不是批量创建contract目录或为22个组件各建go.mod。

<a id="organization-acceptance"></a>
## 5. 评审及实现交付边界

每个组件须能从对应MIG反查当前角色、目标角色、具体文件、保留/新增/移除、合同ID入出边、能力批依赖、稳定中间态、回退及独特验收。具体合同作者维护实际topic/guard/results/errors/兼容，不以MIG中的目标用例名虚构现有Go API。

实施顺序、R/C/B编号、G0–G4、当前gap和验收状态只引用[收口总纲](Workorch_逐项收口总纲与首项方案.md#batches)。普通能力批与最终Web完整环所需SG层级由总纲判定，本文不建立第二套退出规则。详细fixture/oracle由[验收规格](validation/验收规格.md)维护。

M3、默认手动/明确授权自动、精确ticket变更路线沿用既有产品目标；无须重新选路线。本版是工程设计冻结候选，用户尚未批准；固定基线已有部分实现，但本轮未实施，不代表测试、集成或live已通过。若迁移必须改变已设定目标、验收标准或合同约束，先提交完整原因/差异/影响的ticket并批准精确候选；普通工程细化不能暗中改承诺。

强隔离、历史replay、独立人工最终验收等按模型中的触发条件判定适用性，不能因组织上没有新组件就删除义务。

<a id="organization-evidence"></a>
## 6. 固定版本组织证据

最新关键证据：[22单元显式入口](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/application/workorch/main.go#L12-L33)；[保留旧owner ID](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/common/common.go#L1-L3)；[Module注册及checked生命周期桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/plugin.go#L14-L65)；[owner-neutral fresh/drain与真实排空](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/base/biz/lifecycle.go#L17-L38)；[逐订阅及context停止](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L175-L395)。


- [组织规范v2.6.0](https://github.com/muidea/skills-repo/blob/6ba50452fccd33c912b07a0648fbb97acf15d756/skills/go-multi-module-dev/SKILL.md)
- [固定SHA入口装配](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/application/workorch/main.go)
- [Initiator单一provider](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/initiators/workorchruntime/module.go)
- [Workspace现角色目录与module](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/module.go)及[唯一删除协调](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/deletion_coordinator.go)
- [Base](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/base/biz/base.go)、[进程桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/framework_service.go)与[实际vendor生命周期](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/vendor/github.com/muidea/magicCommon/framework/application/application.go)

这些链接定位实际固定树中的文件，不将文件存在性当作动态行为通过。历史源码行号和组件专属控制流证据保留在对应MIG；最新增量与CI事实只引用[总纲固定证据](Workorch_逐项收口总纲与首项方案.md#current-gaps)，不在这里复制容易漂移的状态表。

修订记录：v0.3将组织分类与MIG职责保留为本文权威；通用合同、状态机、案例、批次和gap去重引用；补齐角色迁移稳定态与不安全回退边界。未变更产品目标，未执行迁移。

修订记录：v0.4（2026-10-12）按固定SHA重核全部22角色及当前路径；Workspace提升已落实，生命周期/合同纯包已有增量改为复用与回归，产品目标和既定验收不降级。
