# Workorch 统一业务模型与交互关系

- 文档编号：WOC-DOMAIN-MODELS
- 版本：v0.8，模型合同冻结候选版（工作基线、M3创建完成、D1双模式激活及D2合同变更新Run路线已确认；其余产品分歧按需确认）
- 日期：2026-10-12
- 本轮范围：统一项目目标下的业务概念、身份、生命周期、权威归属和交互；作为后续逐项收口及 Web 新建 Workspace 合同的上位依据
- 当前阶段：只读设计评审；没有修改项目代码，没有执行测试、CI 或 live
- 代码参考：最新核验 `muidea/workorch@5360caf36e081717087557f34d2af3af1c16dc16`；旧 `5554296240074054db34b0c551f4d326cd34a361` 链接为历史基线。当前实现差距状态只在《逐项收口总纲》维护，本文及NC仅列模型/目标到源码的复用映射；本文不重复状态台账
- 强制组织规范：`muidea/skills-repo@6ba50452fccd33c912b07a0648fbb97acf15d756` 的 `go-multi-module-dev 2.6.0`

> 现有代码是可复用的参考。目标模型以项目目标、已确认产品要求和指定组织规范为依据。本文中的逻辑模型不要求各自新增 Go struct、数据库表或组件；模型统一也不意味着一次性重构全部 22 个运行单元。以下“目标规则”是工程设计合同；已确认产品要求、工程评审建议与待决产品差异分别登记，不将建议自动算作用户决议。

## 1. 已确认范围与本轮结论

### 1.1 已确认

1. 按项目目标和整体架构，从整体到细节逐项收口，每批都具备可集成验收的完整能力
2. 本阶段只评审并输出方案，不调整代码；收口方案统一用 Markdown 文档
3. Workspace 创建只保留 Web 入口；不需要保留 TUI 创建入口，也不新增 TUI 创建向导
4. 先统一业务模型及其关系，再继续讨论新建合同、Web 交互和创建完成产出
5. 现有实现按“直接复用、适配复用、替换、待核验”评估，不以保留现状约束目标，也不预设全部推倒重做
6. 用户已认可v0.2的完整性评估结论，将其作为逐项收口与渐进冻结的工作基线；此项确认不等于逐项批准所有技术选择、完成全部模型冻结或授权代码实施
7. 任何涉及修改已经设定的目标、验收标准或合同约束的动作，必须先发起ticket，并完整说明调整原因。此项适用于已设定承诺，不仅适用于已启动Run；ticket提交不是批准，批准不是已应用。不改变合同的正常任务分解、实现细化和再规划不因此自动要求变更ticket
8. 目标、验收标准或合同约束发生变化时，先完成候选修改，再对最终精确候选进行approve；通过后重新激活新Run执行。原Run不改写合同；新Run绑定新正式定义，保留真实前驱和历史。此项确认的是产品流程，不授权本轮修改代码或执行live

9. 新建完成点采用M3：合同已确认封存、必要资源准备和启动检查通过，但尚未创建Run。默认合同确认后由用户点击“激活执行”；同时保留明确授权下“一次提交、审查通过后自动启动”的模式。此项确认产品语义，不授权当前实际provider调用、代码变更或live执行

### 1.2 一页领域图

```text
Workspace 长期容器
├─ 定义：目标原文 → Requirement → Obligation → DoD / 验证策略
│                              ↓审查、精确确认、封存
│                         正式 Definition
├─ 配置：Template / Profile / Soul / Skill / 资源绑定
├─ 执行：启动准入 + 当前就绪 + 明确执行意图 → Run
│           Run → 规划/CWI → 工作步骤 → 操作/副作用 → Receipt
├─ 证明：Receipt / Artifact → Evidence → Verification → Closure
├─ 交付：正式 Closeout → Result / Artifact → Run 终态
└─ 支撑：Decision / Approval、Memory、Budget、Audit、恢复/删除
```

图是业务理解视图，不要求新增图数据库或另一套状态机。**22个技术运行单元不等于22个业务模型**；后面的40组逻辑模型可以对应现有聚合、值对象、从属记录或派生视图。Runtime是泛指运行时机制，当前由Orchestrator等明确owner分工实现，不另增名为Runtime的权威模块。

### 1.3 建议统一的主线

**用户目标与资源 → 可审阅的定义合同 → 精确确认与封存 → 独立启动准入及授权 → Run → 规划与实际执行 → 有来源的证据 → 验证与逐义务闭包 → 交付结果。**

这条主线有三个不能混淆的“完成”：

- **新建完成**：M3首次成立并记录完成回执；合同封存、必要准备和启动检查通过，此刻尚无Run
- **执行结束**：Run/执行尝试进入合法终态，可能成功、失败或取消，不等于目标验收通过；blocked是尚未结束的状态
- **目标完成**：所有本次合同 required 义务有有效证明并满足终态规则，由运行时权威裁决

Workspace新建以M1容器创建、M2合同确认、M3新建完成、M4激活执行区分事实。**M3完成点及D1双模式已经用户确认**：默认用户独立点击“激活执行”；明确授权的自动模式在M3成立后提交独立启动意图。组合交互可以不展示Run数量为0的瞬间，但审计必须证明M3先于Run创建；自动启动后展示真实Run状态，不能继续标为当前Run数量0。历史readiness不是永久启动许可证。

### 1.4 本次复核结论与文档适用顺序

v0.2根据《Workorch 业务模型完整性与充分性复核》补齐MC01–MC07及四条跨模型协议。保留40组DM、22个运行单元和R01–R55索引；69类模型族仅为历史覆盖索引，不是最新类型总数。**模型范围较全面不等于全部定义充分；本版给出可讨论、可逐批冻结的最小行为合同，不宣称全系统已可实施或验收。**新增规则是目标设计提案，不是本轮运行观察，也不证明现代码缺少全部对应机制。

建议采用以下文档适用顺序，并在每批冻结记录中确认：

1. 用户明确目标、最新范围决议及指定组织规范构成约束；安全底线和真实能力上限不得被任何配置覆盖
2. 已批准的具体决议优先于草稿建议；本文统一术语、权威和跨批不变量，逐项收口总纲保留G0–G4门禁及原有C/B编号
3. 本文唯一维护业务概念、关系、产品决议及跨域不变量；[Core](Workorch_核心AgentLoop与状态机合同基线.md)唯一维护完整循环、四机与恢复算法；[组件具体合同目录](contracts/组件具体合同目录.md)维护通用接口实例；[创建与定义变更合同](contracts/创建与定义变更合同.md)唯一维护NC00–NC05新建/授权/变更协议；[Web专项](Workorch_Web新建Workspace合同与交互方案.md)仅维护页面交互、错误展示与输入产出；[验收规格](validation/验收规格.md)维护用例、fixture与独立oracle；[总纲](Workorch_逐项收口总纲与首项方案.md)唯一维护G0–G4、当前差距和冻结/退出台账
4. 发现正文与专项合同不一致时记录冲突及受影响范围，由该批决议处理；不得按文件日期、加载顺序或“更详细”自动胜出。尚未批准的本文规则也不能覆盖已确认决议

产品差异按§11区分必须确认、可采用的兼容工程规则与后续能力条件项，不要求用户逐个批准所有实现细节。涉及本批可观察结果的未决项只阻塞相应范围；不依赖它的模型合同可继续收口。方案评审通过、用户决议、代码实施、集成测试通过和live通过须分开记录。

### 1.5 工作基线确认记录与冻结状态

- 确认日期：2026-10-08 18:33（UTC+08:00）
- 用户原话：“同意上述结论，以便实现冻结模型”
- 来源：本次对话消息 `Sentinel_642dd6823c9081918030b1137b034614`，针对v0.2整体模型可作为逐项评审基线、具体实施合同及产品决议仍需收口的结论
- 本次确认范围：认可上述评估结论，采用v0.2作为渐进冻结的工作基线。v0.2.1仅记录确认状态及其适用边界，不修改模型技术内容

| 对象 | 当前状态 | 约束 |
|---|---|---|
| 统一模型文档工作基线 | 已确认 | 后续逐项方案以其组织术语、关系及待决清单；发现冲突按§1.4处理 |
| 工程细化与产品决议 | 分开处理 | 工程确定性规则由评审形成冻结候选，不逐条要求用户审批；影响用户承诺、范围或执行语义的真实分歧按§11确认 |
| 各能力批次所涉模型与交互合同 | 分项待冻结 | 按§10及总纲记录适用范围、决议与验收合同，满足该批既有门禁后单独登记冻结 |
| 代码实施、集成验收与live | 未因本次确认而授权或通过 | 当前仍为只读评审；文档冻结与实现通过分别记录 |

合同变更规则确认来源：2026-10-08用户消息 `Sentinel_7bcf1d7e3a348191b475f4765077a753`：“如果涉及到需要对设定目标、验收标准或合同约束进行修改，需要通过发起ticket的方式，并完整的说明需要调整的原因”。这确认了必须经过ticket及原因说明的治理要求，不等于批准任何具体变更。随后用户消息 `Sentinel_246e6b8456c88191a9be66b525aaef8d` 明确：“如果目标、验收标准或合同约束发生变化，在修改完成通过approve后，重新激活新run执行”。因此D2的新定义/新Run承接路线已确认；approve绑定最终修改候选，激活仍需实际准入、安全交接及明确执行意图。具体协议见§5.9。

M3/D1确认来源：2026-10-08用户消息 `Sentinel_63772243c8808191b5f20338a0c954d4`：“1. 合同已确认封存、必要资源准备和启动检查通过，但尚未创建 Run ，2.默认在合同确认后，由用户点击‘激活执行’；同时保留明确授权下‘一次提交、审查通过后自动启动’的模式”。本版同步Web专项v0.4，产品边界见§6.1.2与§11.2；具体规则见NC01–NC03。

“基线已确认”表示有共同的评审起点；“分项已冻结”仅适用于有明确范围和批准记录的具体合同。不得将前者推导为40组模型、全部关系或所有批次已冻结。

## 2. 先统一四个层次

| 层次 | 内容 | 不能替代的东西 |
|---|---|---|
| 业务模型 | Workspace、Requirement、Obligation、Run、Evidence 等用户意义和状态 | 不是目录名称或接口 DTO 的同义词 |
| 权威与合同 | 谁可以写、哪些命令可转移状态、版本/幂等/事件语义 | 不是“谁拥有 repository 指针谁有业务解释权” |
| 运行单元 | 当前1 Initiator、5 Modules、16 Blocks；Workspace已位于kernel并承担Module协调职责 | 不要求每个业务模型对应一个运行单元 |
| 实现载体 | Go struct、聚合、文件、索引、日志、typed EventHub、HTTP、Web 页面 | 不是新的业务事实来源；展示缓存不能驱动权威状态 |

本文模型编号 `DMxx` 是文档追踪 ID，不是要求新增的运行时字段。生命周期名称用于描述合同阶段，不要求直接替换当前 enum。

## 3. 统一业务模型目录

“权威 owner”指有权解释并决定该事实的唯一领域主体；持久化提供者、应用编排者及 UI 不是当然的权威 owner。表中未给出现有精确类型名的逻辑模型，对应具体字段、接收合同及未落地项由[组件具体合同目录](contracts/组件具体合同目录.md)与[NC合同](contracts/创建与定义变更合同.md)逐项列明；文中的概念名不冒充现有Go类型。

### 3.1 定义、配置与资源域

| ID / 模型 | 业务含义与边界 | 身份、版本与生命周期 | 目标权威 / 当前参考 |
|---|---|---|---|
| <a id="dm01"></a>DM01 Workspace | 长期工作容器，关联目标定义、资源、执行历史；不是一次工具执行或 OS 沙箱 | WorkspaceID 稳定；元数据修订；创建中→可用→删除中→已删除，合同及运行状态另列 | Workspace owner；当前 Workspace.State=Active 不表示有运行中的 Run |
| <a id="dm02"></a>DM02 AuthoringInput / Source | 用户目标原文、资源引用、验收要求、范围及来源；保留原文与规范化表示 | WorkspaceID+输入修订；变更产生新修订，不能静默覆写已封存来源 | Workspace owner；应用负责输入适配和流程组织 |
| <a id="dm03"></a>DM03 Goal | 要达成的业务目的；既含原始表达，也可有可追踪的结构化目标项 | Goal身份/稳定 ItemRef+定义版本；草拟→纳入候选→封存 | Workspace owner；现有 Goal 的聚合规则不能代替逐义务证明 |
| <a id="dm04"></a>DM04 Requirement | 从目标、模板、条件或后续发现得到的原子要求，可包括约束和验收要求 | RequirementID 在对应规格修订内稳定；来源、原文位置/引用、必要性、适用性必须保留 | Workspace specification owner；现有五种 SourceKind 可复用 |
| <a id="dm05"></a>DM05 Obligation | 必须证明成立的独立义务；回答“还欠用户什么证明” | ObligationID+规格版本；定义阶段适用性、运行阶段闭包分别记录 | 定义由 Workspace 持有；运行期判定由 Orchestrator 负责，禁止两边共同改同一状态 |
| <a id="dm06"></a>DM06 DoD / AcceptanceCriterion | 可审查的完成标准与判断谓词；不是运行后的证据 | DoD/ItemRef+定义版本；每条标准明确判据、适用范围及验证策略 | Workspace owner；当前 DoD required_coverage 是参考；正式DoD的ready是定义状态，各Run的satisfied不得写回可复用DoD |
| <a id="dm07"></a>DM07 VerificationPolicy | 对判据采用什么方法、证据等级、独立性、freshness、失败/未知处理 | PolicyID或内容哈希+封存版本；运行中不得降级 | Workspace 定义合同；Orchestrator 执行既定规则，capability owner 提供事实 |
| <a id="dm08"></a>DM08 GoalSpecification / DefinitionContract | GoalSpecification是pre-seal规格提案；业务验收合同是来源、义务、正式Goal/DoD、策略的可读整体视图，二者不混成单一实体 | InputRevision、CandidateRevision与正式DefinitionRevision分别绑定；提案→审查→精确物化正式定义，各自hash可追溯 | Workspace owner；正式合同视图追溯到原有权威值，不新建平行可变Contract聚合 |
| <a id="dm09"></a>DM09 CompletenessReview | 对每项适用要求开展各维度审查，记录 PASS/FAIL/UNKNOWN/有理由的 N/A | 绑定候选修订/hash、审查维度、范围及证据；候选变化即过期 | Workspace 保存并判定是否齐备；LLM只产生审查建议，不能自行封存 |
| <a id="dm10"></a>DM10 Confirmation / SealReceipt | 精确合同的人工确认或显式授权政策下的接纳，以及该版正式内容已可靠物化的事实 | ConfirmationID或现有journal identity；精确版本/hash；prepared→materialized可恢复 | Workspace owner；确认文案、一次点击或HTTP成功本身不是物化回执 |
| <a id="dm11"></a>DM11 WorkspaceTemplate | 可复用的标准、能力要求、政策与知识配置；不是固定执行步骤 | TemplateID+revision/hash；继承DAG与来源；新版本只作用于新解析/定义 | Workspace owner；保留当前多父模板已有能力 |
| <a id="dm12"></a>DM12 RuntimeProfileContract / ResolvedTemplate / DefinitionSnapshot | 分别为领域验收声明、模板有效合并结果、精确定义快照，互有关联但非同义词 | 内容hash、来源版本链、解析规则版本；绑定定义/Run后不可热替换 | Workspace负责定义侧解析；Run保存绑定；Orchestrator使用冻结输入 |
| <a id="dm13"></a>DM13 ResourceBinding | 仓库、目录、数据源、文档、服务等资源的定位、用途与访问边界 | BindingID、资源版本策略、作用域；声明→验证→失效/替换；变化须评估影响 | Workspace记录绑定；实际资源owner验证和执行；选择资源不等于授权所有操作 |
| <a id="dm14"></a>DM14 CapabilityDescriptor / Readiness | 某owner可提供哪些操作及特定时点是否具备条件 | capability/operation版本、owner、上下文；readiness带时间和绑定，不永久有效 | 各capability owner发布事实；System/应用可汇总；Orchestrator不得凭LLM文本认定可用 |
| <a id="dm15"></a>DM15 Soul / Skill / MemoryBinding | 分别是行为边界、可复用技能资产、可召回知识；不混成一个“提示词包” | 各自ID、版本、来源与Workspace绑定；使用时形成可追踪快照或引用 | Soul、Skill、Memory各自owner；模板仅引用，不接管其源事实 |

#### 3.1.1 定义相关模型不是同义词

层级依次为：AuthoringInput（可修改输入）→GoalSpecification（pre-seal规格提案）→DefinitionAdmissionCandidate（封存候选）→DefinitionAdmissionResult（纯编译准入事实）→精确Confirmation及物化→正式Goal/DoD的DefinitionRevision→WorkspaceStartupInputContract与WorkspaceActivationAdmission。

InputRevision、CandidateRevision、DefinitionRevision和Run的StateRevision是不同版本轴，不能因为数字相同就互换。GoalSpecification不因获得PASS就成为运行期authority；正式Goal/DoD资源变更生成新定义修订，Run的验收状态不写回定义。

RuntimeProfileContract描述领域验收维度、交付期待、证据/依赖/recipe声明；ResolvedTemplate是模板解析结果；DefinitionSnapshot是某次封存输入视图。它们应分别保留来源和hash，并在启动快照中绑定。当前未查明独立Profile CRUD仓库，不虚构一个Profile owner或新配置数据库。

### 3.2 执行与治理域

| ID / 模型 | 业务含义与边界 | 身份、版本与生命周期 | 目标权威 / 当前参考 |
|---|---|---|---|
| <a id="dm16"></a>DM16 CandidateAdmission / ActivationEligibility | 候选编译准入与正式启动输入的activation eligibility分开；两者都不等于实时readiness或执行授权 | CandidateID/CandidateRevision/hash对应编译结果；正式StartupInputHash对应activation事实；旧绑定失效后不得复用 | Workspace持有事实，纯编译器提供候选判断，workorchd组织流程；没有独立可写准入副本 |
| <a id="dm17"></a>DM17 ExecutionIntent | 用户明确提出或在有效授权范围内代提交的 Start/Cancel/Resume/Restart 等动作意图 | RequestID、目标对象/修订、请求者、命令内容hash；accepted/rejected/pending；可幂等 | 各目标owner受理；应用负责鉴权、传递与结果组合，不在UI本地生效 |
| <a id="dm18"></a>DM18 Run | 一次绑定不可变定义/资源政策快照的执行实例；承载生命周期与执行历史 | RunID稳定；WorkspaceID、定义revision/hash；生命周期SM-1及CAS版本 | Run owner决定SM-1；不解释Orchestrator内部证明语义 |
| <a id="dm19"></a>DM19 RuntimeAggregate | 一次Run的规划、工作、治理、执行及证明权威状态的聚合 | RunID+aggregate revision/CAS、semantic generation；恢复沿用同一身份 | Orchestrator是语义owner；Run可持久化opaque载荷但不得重新解释或改写内容 |
| <a id="dm20"></a>DM20 CoverageWorkItem（CWI） | 把冻结定义中的覆盖责任转成运行期可跟踪单位；不是任意可执行任务 | CWI ID、稳定来源ItemRef、definition hash、focus/binding；canonical ledger为RuntimeCoverageItemFact，不另建同名DTO状态机 | Orchestrator；当前每个CWI有一个primary source item，保留与源义务关系的专门映射 |
| <a id="dm21"></a>DM21 Plan / WorkTask | 为满足未闭义务采取的策略、依赖和实际工作步骤；任务可以共享成果或并行 | Plan generation与步骤/操作引用；Task为逻辑工作分解概念，现有独立Task实体未证实，是否引入须另评审 | Orchestrator裁决；LLM提供建议；不要求新增一个与semantic plan平行的图权威 |
| <a id="dm22"></a>DM22 ExecutionAttempt | 对一个已封存并通过适用准入/授权检查的操作的某次实际尝试；重试不抹去旧尝试 | Attempt/ExecutionKey、Run/CWI/Task关联、attempt序号、fence；排队→执行→已知终态/unknown | Orchestrator管理调度边界；执行owner管理本次操作事实，各自写不同字段 |
| <a id="dm23"></a>DM23 EffectIntent / EffectLedger | 操作可能产生的外部影响及其提交/对账记录；不是普通日志 | Effect/operation identity、幂等键、sealed payload hash、fence、预算账；prepared/dispatched/known/unknown/reconciled | Orchestrator负责调度/副作用协议；capability owner负责本地操作回执，不能出现两份独立重试权威 |
| <a id="dm24"></a>DM24 ExecutionReceipt | 能力owner对精确操作实际发生了什么的回执 | ReceiptID、ExecutionKey、owner、payload/hash、输入输出/依赖/错误；不可冒认其他owner | File/Git/Shell/SSH/Web/MCP/LLM等对应owner；成功receipt不授予验收credit |
| <a id="dm25"></a>DM25 BudgetLedger | 预算上限、消耗、预留、未知调用成本与停止条件 | Run/operation scope+版本；账目单调可追踪；恢复不能重置已用预算 | 各调用事实来自执行owner；Orchestrator内部consumption sub-ledgers区分confirmed/unknown-reserved，BudgetView只读派生，不新增第二预算账本 |
| <a id="dm26"></a>DM26 DecisionTicket / Resolution | 需要澄清、选择、修订或控制动作的治理问题及明确决议；设定目标、验收标准、合同约束的变更必须使用其中的合同变更提案 | TicketID、Workspace/可选Run、当前definition或输入基线、提案版本/hash、原因及影响；当前open/resolved/superseded，不新增平行状态枚举；扩展语义见§5.9 | Decision owner持有ticket/resolution；governance协调规则，定义由Workspace owner验证及物化；运行期formal resolution仍需matching Orchestrator acceptance，决议不直接改合同或授credit |
| <a id="dm27"></a>DM27 Approval | Run-bound高风险操作许可，规定对象、动作、范围与有效条件；不承担无Run定义批准或自动激活授权 | ApprovalID、Run/operation/资源/请求者绑定；现有状态与目标有效性字段分开映射 | Approval owner持有许可；Governance编排；许可不等于操作已执行、定义获采纳或目标已通过 |
| <a id="dm28"></a>DM28 GovernanceProposal / Reconciliation | 持久治理提案、策略计算与运行期采纳是不同事实/动作 | proposal identity、问题/运行绑定、被授权范围及revision；阶段由对应owner推进，可恢复重入 | Decision独占durable runtime governance proposal生命周期；Governance计算策略并提交typed resolution；Orchestrator掌握binding/acceptance/continuation，无第二治理协调账本 |
| <a id="dm29"></a>DM29 Recovery / RestartLink | crash恢复当前Run，与新定义后显式创建后继Run的关系 | recovery使用原RunID；restart绑定前驱/后继及请求身份；唯一后继条件需执行合同 | Run管理SM-1与replacement intent；Orchestrator恢复SM-2/3/4；不是审计事件重放驱动状态 |

### 3.3 证明、结果及审计域

| ID / 模型 | 业务含义与边界 | 身份、版本与生命周期 | 目标权威 / 当前参考 |
|---|---|---|---|
| <a id="dm30"></a>DM30 Artifact | 可交付或验证的文件/对象；“文件存在”不是“满足目标” | ArtifactID/稳定引用、内容hash、位置/版本、producer、访问策略；新内容新版本 | Result管理交付元数据；文件内容归具体资源owner，不能把Result视为文件系统owner |
| <a id="dm31"></a>DM31 EvidenceFact | 可用于证明某义务的有来源事实，关联真实receipt、产物与观察上下文 | EvidenceID/content hash、producer、operation/Run、依赖revision、生成/观察时间 | Orchestrator管理证明使用和事实关联；原始执行事实仍归producer；历史记录不可改写成另一事实 |
| <a id="dm32"></a>DM32 EvidenceBinding / Applicability | 某条证据对某版义务/判据在当前上下文是否适用 | EvidenceID×Obligation/CWI/criterion×definition/binding/generation，含依赖范围 | Orchestrator裁决；historically accepted与currently applicable分开 |
| <a id="dm33"></a>DM33 Verification / Assessment | 对证据与判据的检查结果；确定性检验、语义意见和人工许可必须区分 | VerificationID/receipt关联、规则版本、输入集合hash；PASS/FAIL/UNKNOWN及诊断 | 检验owner产生检查事实；LLM给语义意见；Orchestrator根据冻结策略做最终接纳 |
| <a id="dm34"></a>DM34 ObligationClosure | 对单一原始义务的当前闭包：所需全部判据与有效证据是否齐备 | ObligationID+definition hash+closure revision/依赖；open→satisfied/blocked/failed；受影响后reopen/stale | Orchestrator唯一裁决；CWI完成不能跳过源义务完整性 |
| <a id="dm35"></a>DM35 RunCloseout | 汇总所有required义务、执行终态、限制、失败和交付的正式结论 | RunID+closeout identity/hash、定义与证据绑定；由权威状态确定后保存 | Orchestrator生成并协调交接；Result先持久确认，Workspace再确认活动slot，Run最后进入合法SM-1终态，不直接同步覆写 |
| <a id="dm36"></a>DM36 Result / DeliveryView | 持久化正式结果及面向用户的交付视图 | ResultID、RunID、closeout hash、artifact refs；修订不得篡改历史结论 | Result owner保存；Web/HTTP/TUI的读取投影不得成为第二验收引擎 |
| <a id="dm37"></a>DM37 AuditEvent / ReadProjection | 事实发生后的审计记录、时间线、SSE及查询投影 | EventID/owner sequence、对象/版本、因果/关联ID、受限原文引用及显示元数据；append-only或可重建投影 | Eventstream负责审计资源；业务owner负责原事实；日志不能用于自造authority |
| <a id="dm38"></a>DM38 MemoryItem / RecallTrace | 可复用知识与此次召回来源/筛选记录；不是合格证据的默认来源 | MemoryItemID+版本/来源；RecallTrace绑定调用与召回项；过期/删除有策略 | Memory owner；RecallTrace只证明上下文包含/来源，不得作为当前Run目标验收证据 |
| <a id="dm39"></a>DM39 RegistryAsset / ProviderBinding | MCP服务注册、Skill目录、LLM provider等可用资源的注册和绑定 | registry key+revision、能力版本、凭据引用及访问范围；启用/禁用/失效 | MCPRegistry、Skill、LLM等各自owner；发现入口不授予访问授权 |
| <a id="dm40"></a>DM40 WorkspaceDeletionJournal | 跨owner停止、封存、清理及最终删除的唯一协调账本 | WorkspaceID+deletion operation/generation、阶段、quiescence token、owner ack及retirement/tombstone；可重试且删除后保留必要身份 | 当前由kernel Workspace Module唯一持有；保留owner/journal连续性，不能复制到workorchd形成双账本 |

## 4. 核心术语边界及关系

### 4.1 不能互换的十个概念

- Requirement：用户或模板要求什么
- Obligation：系统必须独立证明什么
- DoD/Criterion：怎样判定它得到满足
- CWI：运行期怎样保持覆盖责任
- Task：为此准备做什么工作
- ExecutionAttempt：某次实际尝试了什么
- Effect/Receipt：产生什么影响，谁对实际发生的事实负责
- Evidence：哪些事实可以支持判据
- Assessment/Closure：检查意见是什么，权威最终是否接受其证明
- Result：把权威结论和交付物呈现给用户

例如用户要求“修改A且不破坏B”：可以一个Task完成修改，一个测试执行同时产生两类证据；但必须保留A与B各自的义务、适用判据及闭包。测试只验证A时，即使任务完成、命令exit 0、CWI局部通过，也不能宣布整体Goal完成。

### 4.2 关系、基数和强制不变量

| 关系 | 目标基数/语义 | 不变量 |
|---|---|---|
| Workspace→定义版本 | 1→0..N；草稿也可尚无封存版 | 一个版本一套精确内容，不能原地修改旧seal |
| Workspace→Run | 1→0..N历史；当前ActiveRun slot至多1 | slot归Workspace管理，Run生命周期仍归Run；多活动Run不是本轮已批准扩展 |
| Run→封存定义/有效配置 | 每Run恰绑定1套精确启动快照 | 之后模板、Goal、Soul变化不能热修改既有Run验收边界 |
| Requirement→Obligation | 本版工程选择一Requirement→多Obligation，每Obligation恰一个primary Requirement；兼容当前单RequirementID载体 | 多要求共享实现、判据或证据时保留各自独立义务；不新增多父义务，也不丢失原始语义 |
| Obligation↔DoD/Criterion | 可多对多；required obligation至少有适用判据/验证方式 | 一个判据可以覆盖多义务，但逐义务适用与充分性必须可判定 |
| DefinitionItem→CWI | 当前合同为每required item至少1CWI；每CWI恰1primary source item | 不因新增源规格映射把primary source变成歧义集合；完整义务关联用独立可审计映射表达 |
| Obligation↔CWI/Task | 可多对多的覆盖/实现映射 | 不要求一义务一任务；每未关闭required义务必须有推进路径或明确阻塞/失败 |
| Plan→Task→Attempt | 一计划多个工作步骤；一步可多次尝试 | 计划改变不删除义务；重试保留先前attempt、成本及unknown影响 |
| Attempt→Effect/Receipt | 按真实operation拆分，可能0..N；不可假定一个task恰一个外部副作用 | 未知回执先对账，不能直接新幂等键重复执行 |
| Evidence↔Criterion/Obligation | 可多对多；用显式binding记录使用 | 同一证据可复用，credit必须分别满足策略、独立性与freshness |
| Obligation→当前Closure | 每版义务最多一个当前权威判定，可保留多个历史版本 | 当前有效集合all-of；历史accepted不自动续期 |
| Run→Closeout→Result | Run权威终态对应正式closeout；结果可有多个展示投影 | 投影不自行补“成功”，不重新解释验收规则 |
| Artifact↔Evidence/Result | 产物可以同时作为交付和证据载体，也可只占一类 | 内容hash/来源/访问边界明确；路径同名不意味着内容相同 |
| Workspace→DeletionJournal | 同一删除过程一个权威账本 | 跨owner恢复只能继续同一账本，不由多个模块竞争清理 |

建议未来建立“源Requirement→源Obligation→定义项/CWI→实际工作/receipt→适用证据→闭包→Result”的可查询链。该链须保留逐源义务映射与运行证据；结构上有关系不证明全部承诺已无损闭包。当前实现状态仅见总纲。

### 4.3 要求类型、适用性与检查时点（MC01）

类型属于DM04–07的语义分类，不强制新增五个实体；一个原句可拆为多种原子要求，但全部保留DM02原文及来源位置。

| 类型 | 义务含义及检查时点 | 目标裁决规则 |
|---|---|---|
| 最终成果 | 交付对象在指定版本上满足规定结果；在相关产物变化后验证、closeout前复核 | 仅过程完成或文件存在不满足成果义务 |
| 全过程禁止/保持约束 | 例如不得越权写入、不得泄露秘密；适用于声明的整个执行区间 | 执行前准入与执行期强制控制、必要审计共同证明；已发生违规不能被最终正确产物冲销 |
| 执行前置条件 | 例如资源可写、能力就绪、预算或许可有效；每次受影响操作前检查 | 不成立或无法确认时阻塞相关操作；历史readiness不能永久复用 |
| 条件义务 | 条件成立时必须履行的义务；条件绑定明确对象、范围、版本与判定方法 | 条件TRUE纳入required闭包；FALSE且依据充分才可判N/A；UNKNOWN保持待判/阻塞，不当作FALSE |
| 假设/依赖条件 | 计划依赖但尚需验证的外部事实或用户声明 | 标明来源与有效期；未验证假设不得冒充证据。失效时重评影响，必要时澄清/修订而非删除义务 |

每项要求至少绑定：来源、语义类型、required属性、适用范围、检查时点、条件/假设来源、判据、失效触发器。类型可组合，但不得借“假设”标签降级用户明确要求。

定义阶段的适用性提案由Workspace保存并按冻结规则校验；需要用户澄清的范围选择由明确决议解决。运行期N/A仅由Orchestrator依已封存条件规则与可信事实裁决，记录条件、观察及版本、规则版本和原因。LLM或工具只能提供建议/观察；不能自行将难项设为N/A。未声明条件、证据不足、条件超期均不可免责。运行中新条件超出封存合同须进入修订决议，不能悄悄改变验收集合。

### 4.4 覆盖充分性与all-of闭包（MC02）

每项源义务保留独立身份及原始语义，形成可审阅记录：Requirement来源→Obligation及适用条件→Criterion集合→声明的验证合同→计划/CWI覆盖→实际Verification/EvidenceBinding→Closure。共享任务、判据或证据允许存在，但不允许共享后只剩一项合并状态。

当前目标先采用明确的all-of：一项适用required义务所声明的全部必需判据均通过，且所用证据适用、策略满足，才可关闭；Run需全部适用required义务关闭。条件判定与N/A按§4.3处理，未支持的任意OR、择支或复杂逻辑表达进入澄清或拒绝，不暗中选择较容易的一支。optional结果单独报告，不抵扣required缺项。

| 保证层 | 必须执行的检查 | 保证上限 |
|---|---|---|
| 结构检查 | 来源/ID完整、引用有效、每项required有判据与验证声明、无遗漏映射、审查适用范围齐备、版本一致 | 证明结构完整，不证明自然语言蕴含成立 |
| 语义充分性审查 | 对每项义务核对判据是否覆盖其全部承诺、边界和否定条件；共享判据逐项说明覆盖理由，记录审查范围及结论 | 形成可追责的审查意见，不伪称通用数学证明；UNKNOWN阻塞封存或对应批次 |
| 运行取证及裁决 | 检查实际对象/版本、断言、执行数、来源、独立性、freshness及全过程约束，再逐义务计算closure | 证明该次合同与观察范围内的结论，不外推到未测目标 |

反例：义务A与B映射同一测试，但只有A的断言，结构可能齐全，语义审查应识别B缺口；即使漏审，运行记录也不能凭“同一测试PASS”自动认定B已证明。每条共享binding必须明确所支持的判据和观察，不能把未观察部分补为PASS。

来源、判据、策略或资源变更时先算依赖影响集：使受影响审查、准入、验证绑定及closure失效/重开，保留历史；不把所有无关证据一概删除。候选版本变化不可沿用未重新绑定的审查。涉及设定目标、验收标准或合同约束变更必须先按§5.9发起完整ticket；修改完成并通过精确approve后走已确认D2新定义/新Run路线，不能直接编辑冻结Run。

### 4.5 配置合成、优先级与冲突（MC03）

有效配置由DM11/12/15/39提供来源链，不新增第二配置authority。解析必须输出“字段有效值、来源与版本、规则版本、强制/建议性质、冲突与裁决”记录，并随定义或Run冻结。

1. 系统安全底线、当前授权和能力上限为不可越过的限制；用户输入、模板、Soul、Skill均不能放宽它们
2. 用户明确目标/禁止项及已批准合同约束不能被默认值、知识召回或技能建议覆盖。强制模板/Profile约束与用户要求不兼容时暴露冲突，要求澄清或显式换模板/修订，不能静默舍弃任何一方
3. 多个限制可兼容时取共同允许范围，例如权限取交集、预算上限取较严界；但只有该字段已有明确类型化合成规则时才计算。没有规则、交集为空或产生不可实现目标时拒绝编译并说明冲突
4. 建议值只在未被明确要求及强制约束限定时补缺；Skill/Memory内容是非权威输入，不能改变权限、required集合或验证等级。标为强制的Soul边界也遵守上述兼容性检查
5. 多父模板保留DAG和逐字段来源；不采用最后写入者胜出。需要union/max/override等字段规则时须明确语义与安全单调性后批准，不由本版自动认定D6已解决
6. 已封存快照不随父模板/provider配置热变。运行时安全授权撤销、资源漂移或能力失效可阻塞执行，但不能悄悄重写既有验收合同

### 4.6 验证声明与接受条件（MC04）

DM07定义策略，DM14说明可用验证能力，DM24记录真实检查，DM33形成判断，DM34授予闭包。定义流程经覆盖审查和精确合同确认固定的是验证政策、必需断言、对象选择规则及允许的方法类别/生成约束，不要求尚未实现的代码、测试脚本或产物hash提前存在。运行期可按冻结政策提出proof proposal、生成具体recipe，经能力owner可行性/目标检查和Orchestrator封存后执行；只能细化实现，不得降低判据、独立性或证据门槛。超出政策允许范围才走合同变更路径。

验证合同分为“定义层政策声明”和“运行期封存验证实例”：前者规定必须证明什么及约束，后者在对象已产生或可解析时绑定具体方法、版本和精确输入。两层合计至少包含：

- 被证明的Obligation/Criterion；定义层对象选择规则，运行验证时解析出的精确对象及版本/hash、目标环境及资源依赖
- 定义层允许的方法类别、预期断言、检查范围、最低有效执行数或适用覆盖条件及三值解释；运行实例的具体recipe/版本和输入绑定
- 证据要求、采集者、验证者、所需独立性维度（身份、实现、环境、输入来源等），以及如何识别共享来源
- freshness窗口或依赖变化失效条件、输出观察与原始receipt绑定、不可用时的阻塞规则
- 谁可提出、确认和执行此方法，输出如何由Orchestrator按冻结规则接纳

动态生成的测试或recipe同样须验证其确实覆盖冻结判据，不能由其自身输出“我已充分覆盖”完成自证。产物尚未创建时可保留选择规则；实际验证前必须绑定精确版本，验证后对象变更按依赖使相关证据失效。

PASS至少同时满足：方法获准、对象/版本正确、必需断言实际执行且通过、执行数达到声明、证据来源可核对、独立性与freshness满足。exit 0、零测试、仅生成测试文件、两份改名的同源事实均不充分。缺能力或无法判断返回UNKNOWN/blocked，不自动降级。负例被正确拒绝可使“测试用例”PASS，但不能把被测业务对象的失败/阻塞状态改成成功。E5及人工最终验收按D7另决，Approval不替代Verification。

### 4.7 资源归属、保留与证据失效（MC05）

DM13记录资源绑定和管理权，实际资源owner执行操作；DM30/36记录产物和交付引用；DM40协调清理。归属、可访问、可修改、可清理是四个不同事实。

| 类别 | 默认管理边界 | 清理及交付规则 |
|---|---|---|
| 用户已有资源 | 绑定不是所有权转移；读/写/删除分别授权 | 删除Workspace默认仅解绑，不删除用户源仓库/数据；额外清理须精确授权 |
| 系统为Workspace创建 | 记录创建来源、用途及管理scope | 仅清理可证明由本Workspace管理的对象，保留恢复/审计所需身份 |
| 共享资源 | 由共享资源owner管理，共用不等于独占 | 不能因单一Workspace删除而整体清理；由owner确认仍有引用及保留义务 |
| 临时执行/验证资源 | 绑定Run/operation与lease，记录是否含未持久产物 | 在任务确已收敛、无未决恢复依赖且正式证据/交付已可靠保留后清理 |
| 正式交付与证据载体 | 记录内容hash、持久位置、访问方式和保留政策 | 创建/运行完成前验证可定位；不能只引用即将销毁的scratch路径 |

保留政策须声明适用对象、保留起点/期限或显式长期保留、责任owner、清理授权及例外；本版不捏造统一天数。未定政策若影响该批可恢复性/交付承诺则阻塞退出。凭据只存受控引用，导出不得包含秘密。

内容变更、权限撤销、失联或清理均由资源owner产生可核对事实，Orchestrator按依赖更新当前适用性；历史证据不改写为另一事实。已留存且满足策略的不可变证明可继续有效，要求实时可访问的证据则需重验或保持UNKNOWN。正式closeout保留当时版本结论，并追加后续失效说明，不静默覆盖历史成功。删除所需tombstone、unknown效果及合规保留记录不随UI条目消失而删除。

### 4.8 执行环境与授权引用（MC06）

长期Workspace是逻辑容器；每Run的工作环境、一次验证环境和远端操作目标是不同引用。DM13/14/17/22/27共同绑定环境身份、资源版本、运行主体、允许动作、网络/路径范围、隔离保证及有效条件。可以复用现有资源绑定和operation envelope，不强制新增ExecutionEnvironment组件、独立数据库或完整RBAC系统。

可信主体来自服务端鉴权或已批准执行上下文；RequestID、可发现注册项、目录路径或LLM建议不构成许可。执行前由拥有操作准入责任的owner核对精确对象/动作/scope及有效条件，发生撤销或漂移时按合同阻塞。可读不推导可写，一次批准不推导所有后续动作，授权A不覆盖A+B。Run授权快照用于追溯，不绕过执行时必须复核的动态撤销。

验证环境应满足声明的独立性和威胁模型；scratch复制仅说明文件准备，不能证明OS、网络、凭据或进程隔离。新建草稿只需明确引用和边界，可以呈现资源尚不可用；实际执行批次必须核实环境能力，不能用草稿成功代替执行准入。

### 4.9 核心模型的最小规范合同（v0.3新增）

本节将MC01–MC04落到可检查的字段语义，不规定Go字段拼写、独立表或新组件。后续实施方案须把每一项映射到现有字段、组合值或必须补充的内容；“已有类似字段”不能替代逐项映射。`Ref`均为带对象类型、owner作用域、稳定ID及所需修订/hash的引用。

| 模型 | 最小必备信息 | 约束及拒绝条件 |
|---|---|---|
| Source / AuthoringInput | 输入身份/修订；用户原文；原文内容hash；来源主体/时间；规范化规则版本及规范化内容；资源引用 | 原文不可因拆分而丢失。位置以明确的字符或字节单位定位原文；不得将短Requirement字段上限套在完整Goal上。规范化不改写原文证据 |
| Requirement | 定义/规格作用域内稳定ID；原子语义文本；SourceRef及原文区间或完整对象引用；来源种类；§4.3类型；required属性；适用对象/区间；条件引用；关联Obligation集合 | 每个原子required承诺至少产生一项独立可追踪义务；代码中用于保留全文的独立Goal来源锚点不需额外伪造重复义务，但其中全部承诺仍须提炼并覆盖，不能用锚点豁免证明。非用户来源标明模板/推导依据。推导要求不能冒充用户原话，拆分必须保留原要求的全部承诺 |
| Obligation | 稳定ID及规格修订；恰一个primary RequirementRef；义务文本及语义hash；required属性；适用条件；检查时点/区间；必需CriterionRef集合；覆盖审查记录引用 | 本版采用一Requirement对多Obligation的兼容路线，不引入多父义务。多项要求共享实现、Criterion或证据时各保留自己的义务；不得只保留合并义务状态 |
| Criterion / DoD item | 稳定ID及定义修订；可读判据；对象选择规则；适用范围；必需断言/观察及通过条件；VerificationPolicyRef；覆盖的Obligation引用 | 只有“测试通过”“功能完成”而无对象、断言及覆盖解释不足以封存。shared Criterion须明确其对每项义务的支持范围 |
| DefinitionCoverageMapping | 精确规格/正式定义hash；Requirement→Obligation→Criterion/定义项映射；运行lowering的CWI关联及映射规则版本；逐义务审查结论 | 映射是已有定义/运行合同中的可审计关系，不是新authority。封存前required义务无丢项；初始化后每项源义务可定位到覆盖责任/明确阻塞，CWI的primary source规则保持不变 |
| VerificationPolicy | 策略ID或规范化内容hash；允许的方法类别/生成约束；断言要求；最低执行覆盖；证据等级、独立性及来源规则；freshness/失效条件；UNKNOWN处置 | 生成recipe只能细化被冻结的政策，不能改变required集合或降低门槛。缺可信能力时保持不可执行/UNKNOWN，不伪造验证者 |
| ApplicabilityDecision | 被判条件及对象/区间；TRUE/FALSE/UNKNOWN；所用事实引用和精确版本；方法/规则版本；责任owner及时间；失效条件 | FALSE只能依已封存条件及足够事实得到N/A。UNKNOWN不免责；变化重新判断。未声明条件不能事后补成N/A |
| VerificationInstance / EvidenceBinding | 实例身份；精确Run/定义/义务/判据；recipe hash及版本；实际对象/资源版本；具体断言映射；输入证据/receipt集合；实际执行数；结果与诊断；binding/generation及依赖集合 | 身份不同不代表来源独立。断言未执行、错误对象、旧generation不适用、同名文件换内容均不得授予credit；引用必须可解析到真实owner事实 |
| ObligationClosure | Run+义务+定义hash；判定修订；适用性；每项必需判据的当前结果及binding；过程约束结果；聚合结论及解释；依赖/失效引用 | 由Orchestrator计算而非外部任意赋值。保留历史，但每一当前上下文只有一个权威判定；重开不抹去已发生的违规和历史结果 |

**版本轴不得合并：** Source/InputRevision用于输入并发；CandidateRevision/hash用于审查与确认；DefinitionRevision/hash用于正式合同；Run StateRevision用于SM-1；Runtime aggregate revision用于CAS；semantic generation及binding用于当前规划/证明适用性；recipe/资源revision用于实际验证对象。generation增加本身不改变定义，也不使所有旧事实物理删除；只允许依精确兼容及依赖规则重新判定使用资格。

**引用一致性规则：** 写入/封存时校验引用的owner、scope、对象存在性、精确版本及允许关系；不得仅凭字符串ID相同跨Workspace/Run连接。引用缺失或混代返回明确冲突/阻塞；历史外部对象已清理时依保留政策使用不可变凭证或标记不可再验证，不伪造对象。循环模板依赖、无法解析的判据/条件依赖阻塞编译；运行Plan可重规划但不能形成无合法推进路径的必需依赖死环。

#### 4.9.1 all-of的业务真值边界

以下是业务判定含义，不新增规则引擎或第二份循环算法；实际转换与原子写集统一见Core。对Run绑定的required义务o，A(o)表示其封存适用条件：

1. 无条件义务的A(o)=TRUE；有条件义务根据精确事实得到TRUE/FALSE/UNKNOWN
2. A(o)=FALSE且判定合法、完整、当前有效时，记为`EXEMPT_BY_CONDITION`（业务显示N/A）；保留条件证明，不把它写成已完成工作
3. A(o)=UNKNOWN时，该义务不能关闭。不得以尚未发现触发事实等同FALSE
4. A(o)=TRUE时，必需判据集合必须非空；集合内每一项都须有当前适用且满足政策的PASS验证。空集合的数学真值不能用于业务成功
5. 全过程义务另需覆盖完整声明区间的控制与观察；已证实违规产生失败事实，不能因最终状态修好或重规划而清除。观察存在影响判定的缺口时为UNKNOWN，不能从“无错误日志”推断全程合规
6. 一项适用义务的多个必需判据采用all-of，本基线不支持判据级N/A：每项均须PASS。确实只在某条件下适用的判据应在定义阶段拆成独立条件义务，保留来源及覆盖关系，由义务级条件判断；不能运行时临时豁免一个判据。没有封存的择支语义不得自动执行OR/任选一项
7. Goal成功要求每个required义务均为有效满足或有充分依据的条件豁免，同时满足终态协议、交付持久化及未决副作用政策。失败/UNKNOWN/缺项均阻断成功；optional未通过单独披露，不抵扣required缺口

PASS/FAIL/UNKNOWN是判断值；open/blocked/reopen是流程或闭包状态；N/A是适用性结论。这三类信息不得塞进一个无法解释的布尔值。规则保证已定义合同内的覆盖与判定一致，**不构成对任意自然语言目标已完整形式化的证明**；完整性仍依赖源文审查、逐义务语义评审及明确的适用范围。

### 4.10 DM01–40逐项字段与生命周期补全

本表补齐§3的每个模型，不以40个名称的存在代替定义充分性。owner、基数和跨对象关系分别继承§3和§4.2；表中新增信息均为目标业务语义，非现有Go字段声明。状态词描述业务阶段，具体合法private event、guard、CAS/写集只引用Core或具体合同，不在本文件定义第二套转换算法。

实体保留稳定ID和owner内版本；值对象无独立可变生命周期，以引用方快照版本演进；不可变事实只追加更正/失效关联；派生视图可重建但不可改authority。旧持久格式按组件合同逐版本读写/迁移，未知版本拒绝，缺失的授权/证明不得用默认值补成成功。不同载体可以复用同一aggregate；下表不要求40个数据库表。

| DM | 语义类型及最小持有信息 | 生命周期、有效性与迁移责任 | 独立验收追溯 |
|---|---|---|---|
| DM01 | 实体；WorkspaceID、归属、元数据修订、current DefinitionRef、创建操作、active/reservation/handoff generation、删除引用 | Workspace创建→可用→删除中→tombstone；Run终态不删除容器；slot变化精确绑定Run，空字段不抹除屏障 | [DM01](validation/验收规格.md#dm01) |
| DM02 | 原文版本记录；§4.9 Source字段、完整原字节与规范化来源、输入修订/创建身份 | 修改新增InputRevision；旧原文不可覆写。Web仅页内存暂存，关闭/刷新未保存内容丢失，持久恢复归服务端owner；迁移保bytes/hash和来源区间单位，旧候选不覆盖新输入 | [DM02](validation/验收规格.md#dm02) |
| DM03 | 定义项/值对象；Goal稳定ItemRef、原始表达、结构化目标项、来源/定义hash | 草拟→候选→封存后不可变；新目标生成新Definition，Run满足状态不写回Goal | [DM03](validation/验收规格.md#dm03) |
| DM04 | 定义项；§4.9 Requirement字段、原子语义/required/适用对象、关联义务 | Source拆分保留承诺；候选修订可新增/替换映射，封存后需变更协议；不存在由执行器把required改optional的迁移 | [DM04](validation/验收规格.md#dm04) |
| DM05 | 定义项；ObligationID、primary Requirement、独立语义hash、必需判据与适用条件 | 定义适用性与Run闭包分轴；共享实现不合并身份；删除/减义务需精确改约而非replan | [DM05](validation/验收规格.md#dm05) |
| DM06 | 定义项；Criterion/DoD ItemRef、对象选择规则、通过断言、VerificationPolicy和义务关联 | 候选→ready/sealed是定义阶段；Run satisfied独立。改门槛/断言创建新定义，不覆盖旧判据 | [DM06](validation/验收规格.md#dm06) |
| DM07 | 不可变策略值；方法类别、允许生成约束、证据等级/独立性、freshness、UNKNOWN处理及policy hash | 按Definition冻结；recipe生成只能细化允许范围；降级政策必须改约。无独立运行状态机 | [DM07](validation/验收规格.md#dm07) |
| DM08 | 候选聚合与正式合同视图；source/candidate/Definition各自revision/hash、覆盖映射、审查及manifest refs | 提案→review→exact接纳→物化；正式视图从Goal/DoD来源派生，不可另写一份Contract真相；局部物化无seal | [DM08](validation/验收规格.md#dm08) |
| DM09 | 审查事实；exact candidate、required适用全集、维度、结论、出处、证据/规则版本 | PASS/FAIL/UNKNOWN/有理由N/A保留原记录；candidate/适用范围变化使当前资格stale；不是运行验收事实 | [DM09](validation/验收规格.md#dm09) |
| DM10 | 接纳/物化事实及恢复记录；确认identity、主体/模式、exact hashes、after-image、journal阶段 | prepared→materialized由Workspace恢复；human与policy依据分开；确认响应丢失查原事实，不能重签不同内容 | [DM10](validation/验收规格.md#dm10) |
| DM11 | 可复用配置实体；TemplateID/revision、父DAG、字段来源/强制性质、解析规则hash | 发布新版本不热改旧解析；循环/冲突拒绝；迁移保留父链与旧快照，新版本只作用于新的显式解析 | [DM11](validation/验收规格.md#dm11) |
| DM12 | 三类独立值快照；Profile领域声明、ResolvedTemplate来源合并、DefinitionSnapshot精确内容各有hash/ref | 分别被定义/Run绑定，无独立CRUD假设；来源变化只生成新快照，不能凭名字相同替换 | [DM12](validation/验收规格.md#dm12) |
| DM13 | 资源绑定实体/引用；owner+BindingID、位置/版本策略、用途、访问/管理scope、验证receipt | 声明→验证→漂移/失效→重新绑定；权限撤销即阻断新操作；所有权变化不从绑定推导 | [DM13](validation/验收规格.md#dm13) |
| DM14 | 能力描述及有界检查事实；owner/operation版本、环境/依赖、check结果、checkedAt/expiry/revision | 描述可版本化；readiness PASS会过期/因依赖变化失效；Workspace证书聚合原事实，不能复制能力authority | [DM14](validation/验收规格.md#dm14) |
| DM15 | 三类资产绑定值；Soul/Skill/Memory各自ID/版本/来源、Workspace绑定与实际使用快照 | 源资产由各owner演进；绑定更新影响新快照，旧Run不热换。Skill/Memory不能改安全底线/验收策略 | [DM15](validation/验收规格.md#dm15) |
| DM16 | 纯编译准入事实与启动资格；exact candidate或正式startup input hash、policy版本、diagnostics | 输入改变旧admission不适用；编译PASS不授执行权，不证明动态资源就绪；无独立可写资格副本 | [DM16](validation/验收规格.md#dm16) |
| DM17 | 命令意图/授权记录；intent/request ID、主体、对象/version、payload hash、执行scope、接受状态 | submitted→accepted/rejected/unknown；首次activation稳定映射Run。重放不产生新意图；授权过期/撤销不改已发生事实 | [DM17](validation/验收规格.md#dm17) |
| DM18 | Run实体；RunID、Workspace、冻结Definition/startup、SM-1状态/CAS revision、Result与lineage | 合法迁移只由Run；terminal状态不可重开/热换合同；lineage metadata可append并增revision，见NC05 | [DM18](validation/验收规格.md#dm18) |
| DM19 | runtime语义聚合；RunID、authority revision、SM-2/3/4、generation、binding及各ledger refs | Orchestrator私有事件/CAS转换；同Run恢复原身份；Run存opaque载荷不能解释或修正业务状态 | [DM19](validation/验收规格.md#dm19) |
| DM20 | canonical覆盖事实；CWI ID、primary ItemRef、定义hash、coverage binding、源义务映射/闭包依赖 | 初始化/重规划保覆盖，重用须验证binding；不能因Task完成直接置源义务satisfied；不另建平行CWI状态机 | [DM20](validation/验收规格.md#dm20) |
| DM21 | 计划文档/逻辑工作分解；generation、目标未闭义务、依赖、步骤/operation refs、理由 | LLM提案→Orchestrator采纳/重规划；计划可替换但义务不可删；Task独立实体未经核验，不强制新增图仓库 | [DM21](validation/验收规格.md#dm21) |
| DM22 | 尝试记录；ExecutionKey、Run/CWI/操作、attempt序号、payload、fence及owner回执 | 待调度→claimed→已知结果/unknown；超时/lease过期不证明未执行；新attempt保旧成本与影响 | [DM22](validation/验收规格.md#dm22) |
| DM23 | 副作用意图/账本；operation identity、sealed payload/hash、冲突scope、dispatch/fence、reconciliation refs | prepared→dispatched→known/unknown→reconciled；未知先对账，不能换key重做；跨Run barrier仍引用原事实 | [DM23](validation/验收规格.md#dm23) |
| DM24 | owner不可变事实；ReceiptID、ExecutionKey、producer/version、输入/输出hash、实际目标与错误/成本 | 产生后不冒认或篡改；晚到回执与原attempt核对后接纳，replay不能为新generation自动授credit | [DM24](validation/验收规格.md#dm24) |
| DM25 | 消耗子账本及派生预算视图；budget policy scope、confirmed、unknown-reserved、预留/结算identity | 预留→确认/依法释放；未知不能当零或取消即释放；恢复/新Run沿原identity聚合，不双扣/不清零 | [DM25](validation/验收规格.md#dm25) |
| DM26 | 治理实体；Ticket/proposal revisions、原因/影响、主体scope、最终candidate、用户选择/正式resolution分别记录 | open提案→精确选择→适用binding验证/acceptance→Decision formal commit；不等于Workspace已应用；旧选择不可套新版 | [DM26](validation/验收规格.md#dm26) |
| DM27 | Run-bound风险许可；ApprovalID、Run/operation/payload、资源scope、主体及有效条件 | 请求→批准/拒绝/替代按具体合同；到期/撤销有效性不得凭现有enum缺项忽略；不能用于无Run定义/autoGrant | [DM27](validation/验收规格.md#dm27) |
| DM28 | durable proposal与策略/采纳事实；problem/binding、授权范围、policy trace、matching acceptance refs | Decision保proposal生命周期，Governance算策略，Orchestrator采纳；三者不共写一个状态，也不造第二协调账本 | [DM28](validation/验收规格.md#dm28) |
| DM29 | 恢复控制与lineage关系；原Run/lease、intent、真实前后Run/Result、definition和handoff refs | crash恢复同Run；明确Restart/newDefinition successor新Run；双端lineage一致后才激活，terminal只追加关系 | [DM29](validation/验收规格.md#dm29) |
| DM30 | 产物实体引用；ArtifactID、内容hash/位置/version、producer、访问/保留/管理scope | 内容变化新version；临时→持久交付需真实定位；清理不删必须证据/用户已有资源，Result不拥有文件系统 | [DM30](validation/验收规格.md#dm30) |
| DM31 | 不可变证明事实；EvidenceID/hash、真实producer/receipt、对象/环境/依赖版本、观察时间 | 原事实保留；后续只能追加关联/撤销使用资格，不能把历史失败改PASS；事实本身不自动有credit | [DM31](validation/验收规格.md#dm31) |
| DM32 | 证据适用关系；Evidence×义务/判据×定义/binding/generation、来源/独立性及dependency set | 历史accepted与当前applicable分轴；受影响依赖失效/重判，不无差别删事实；同名路径新内容不可沿旧binding | [DM32](validation/验收规格.md#dm32) |
| DM33 | 检查事实/评估；VerificationID、recipe/version、精确输入hash、断言与实际执行数、三值/diagnostics | owner检验、LLM意见、Orchestrator接纳分开；零测试、错对象、同源伪独立不PASS；复验形成新事实 | [DM33](validation/验收规格.md#dm33) |
| DM34 | 运行闭包事实；Run+Obligation+Definition、适用性、各required判据/binding、过程约束、判定revision | open→satisfied/blocked/failed，依赖变化reopen/stale；只有Orchestrator裁决；过程违规不能重规划抹除 | [DM34](validation/验收规格.md#dm34) |
| DM35 | 正式终态材料；closeout ID/hash、Run/Definition、all-of witness、证据/限制、Result ACK链 | 由权威快照编译/封存后交Result/Workspace/Run；partial ACK不算终态，材料不可随重试重编改结论 | [DM35](validation/验收规格.md#dm35) |
| DM36 | 结果实体与展示投影；ResultID、Run、closeout hash、artifacts、逐项结论/限制、后续delivery refs | Result先持久接纳exact closeout；用户视图可追加交付状态但不重写历史成功/失败；多投影一来源 | [DM36](validation/验收规格.md#dm36) |
| DM37 | 审计事实与投影；event ID、owner seq/revision、causation/correlation、对象/hash、原文bytes/byteCount/SHA256/保存完整性及受控读取ref | 事实append、投影可重建；重复乱序按owner身份处理；投影不驱动authority；原文不清洗/脱敏改写，受限读取保护，部分/未保存显式说明且不改已知失败 | [DM37](validation/验收规格.md#dm37) |
| DM38 | 知识实体及召回事实；MemoryID/version/provenance、适用/保留范围、RecallTrace和筛选理由 | 知识可更正/过期/删除，召回保使用来源；RecallTrace只证上下文包含，不替代当前Run验收 | [DM38](validation/验收规格.md#dm38) |
| DM39 | 注册实体/绑定值；owner registry key/version、enabled状态、能力/provider选择、凭据ref/scope | 注册/启用→禁用/失效；可发现不等于有权调用；版本改变使相应readiness/绑定重核，不暴露凭据 | [DM39](validation/验收规格.md#dm39) |
| DM40 | 删除协调journal；Workspace+operation/generation、阶段、quiescence token、各owner ACK、retirement/tombstone | 单owner顺序停止接入→收敛/清理→最终删除；失败续原journal；已迁Module角色继续保持唯一账本/身份，旧事件不得复活 | [DM40](validation/验收规格.md#dm40) |

## 5. 唯一权威与跨域不变量

### 5.1 写入、命令、事件、查询的边界

跨单元只经owner公开typed合同交互，不能通过repository、service或callback旁路写入。Command表达意图，owner持久提交后才有Fact；Query和审计/SSE只是观察。一个流程可跨多个owner，但不能假设跨目录、索引及外部效果有全局原子事务。ID、revision、hash、幂等及因果字段的逐接收合同由[组件目录](contracts/组件具体合同目录.md)维护，不再在本文复制通用大模板。

### 5.2 主体与责任的唯一归属

| 事实 | 唯一语义责任 | 跨域限制 |
|---|---|---|
| 定义、输入、覆盖审查、精确接纳、物化封存、M3历史记录 | Workspace | provider、Web、workorchd不能替owner封存；检查PASS不是Run证据 |
| 有界候选接纳/自动激活授权及撤销、定义版本采纳 | Workspace持久授权与定义记录；主体权限来自可信鉴权 | 不把无Run授权塞进Approval，不新增独立授权/变更服务或平行journal |
| 原因完整的变更问题及最终决议 | Decision保存ticket/resolution；Governance协调规则 | 决议记录授权主体、选择、范围和精确提案/候选版本；不能直接改定义、Run或credit |
| Run-bound风险许可 | Approval | 许可不等于执行、合同批准或人工最终验收 |
| 运行期问题匹配采纳、规划、效果、证明和闭包 | Orchestrator | runtime resolution必须经过matching acceptance；无Run改定义由Workspace精确采纳，不虚构Run来取得matching |
| Run生命周期及Run创建身份 | Run | 正式定义不可热换；slot归Workspace而SM-1归Run |
| 原始操作事实、能力和资源变化 | 对应能力/资源owner | 原始回执不自动授予验收credit |
| 正式结果存储、审计投影 | Result、Eventstream分别负责 | 不重算验收；投影延迟不撤销已提交事实 |

### 5.3 版本与事实边界

各版本轴保持§4.9所定义的区分。相同请求身份与同内容返回原事实，内容不同冲突；unknown先查原owner而非换幂等键。历史审查、readiness、授权、证据与完成回执不能自动适用于新版本。工作事实与最终验收事实分开，审计原始来源与业务authority分开。具体版本、错误和恢复映射见组件合同及NC合同。

### 5.4 P1 启动与正式终态交接保证

Workspace最多一个当前活动资格。空ActiveRunID并不足以获得新Start资格：尚存的reservation或handoff屏障也必须满足；旧Run的迟到ACK不得清空新slot。正式终态交接保持Result持久确认、Workspace精确交接确认、Run合法终态的因果关系，未核实对应Run终态不能释放新Start屏障。算法及字段写集仅见[Core](Workorch_核心AgentLoop与状态机合同基线.md)；首次激活及变更后继另见NC03/NC05。

### 5.5 P2 效果与预算保证

尝试、已发生效果、unknown和确认/预留成本均有原身份并可追溯。响应丢失不是可重做副作用的证明；取消、重规划、进程重启、定义升级或新Run都不得清零旧账。新Run只得到已授权的预算范围，不能据“新实例”重置Workspace/项目累计上限。外部系统不能证明幂等或安全隔离时阻塞冲突效果，不承诺通用exactly-once；具体能力冲突域、对账与屏障见Core和组件合同。

### 5.6 P3 决议与许可保证

Decision resolution、Workspace adoption、Orchestrator matching acceptance、Approval风险许可及实际执行是独立事实。旧问题、旧候选、旧Run或旧generation的回答不能解锁当前工作；授权A不覆盖A+B。审查建议不得伪装为用户批准，默认自动化政策不得批准削弱目标。决议与撤销/失效竞态见NC02/NC04；运行治理的私有转换见Core。

### 5.7 P4 恢复、取消与删除保证

恢复延续原Run、operation、预算及journal；Resume不更换合同，Restart是明确新意图并保留前驱。取消仅阻止之后允许停止的动作，不回滚现实世界。删除由Workspace唯一journal协调quiescence、drain、owner清理和tombstone；不因UI条目消失销毁unknown、审计或恢复身份。旧generation事件不得复活已退休对象。运行等待与人工blocked分开，升级/回滚须保留外部效果责任。具体顺序和失败切点只在Core及具体合同维护。

### 5.8 定义命令与审计保证

创建、修订、生成、审查、精确接纳、物化及查询分别表达意图/事实；生成晚到不覆盖新输入，部分Goal/DoD物化不可被读成有效seal。创建恢复与确认journal保留不同身份与责任，普通投影失败不伪造回滚。必需审计先决条件失败则阻塞；普通审计延迟可重投递且不得成为第二authority。NC00固定这条定义路径；通用审计/鉴权合同见组件目录。原始业务输入、provider输出及诊断按bytes/byteCount/SHA256保真；不能用清洗、替换、脱敏或摘要冒充原文。明确超界时保留未保存/未传递原因，不把partial标complete，也不把已知失败降为UNKNOWN。安全边界采用owner受限存储/读取；凭据、live配置及原始诊断不得提交Git或混入通用日志。诊断不是规格、准入或执行authority。固定源码约束见[数据保真与浏览器持久边界](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/AGENTS.md#L107-L114)。

### 5.9 已确认的合同变更产品规则

修改已设定目标、验收标准或合同约束，必须先发起原因完整的ticket。包括放宽或收紧承诺、required/optional、条件、证据门槛、独立性及权限/时间/成本约束；草稿阶段已设定承诺同样适用。忠实拆分、未决内容补齐、同合同下的任务分解/replan/允许recipe与安全重试不自动触发本规则；无法判清差异时不得静默改义。

固定产品顺序为：reasoned ticket → 候选修改及审查 → 有权主体approve最终精确候选 → Workspace精确采纳并可靠物化封存new Definition → 安全交接、当前准入和已获执行授权 → new Run。approve之后再次实质修改候选必须重新审查并approve；不能以ticket已resolved掩盖未应用或应用受阻。

原Run合同、SM-1终态及历史结果不可改写。terminal后可以新增带真实前驱/后继与原因的append-only lineage，不能借lineage重开旧Run或改写原终态。没有前驱Run时由Workspace核对定义版本采纳，以新定义创建首个Run，不虚构历史Run。新Run继承真实外部风险和累计预算责任；旧产物可复用但验收credit必须重新判断。紧急停止危险无需先改合同，但随后若调整承诺仍须ticket。

以上是已决产品，既不要求用户重新选择新Run路线，也不批准任何具体改约或当前代码/live执行。详细phase、intent/fact、input/output、幂等与崩溃恢复唯一见[NC04与NC05](contracts/创建与定义变更合同.md)。

## 6. 从新建到交付的业务关系

### 6.1 新建与定义

Web收集原始目标、模板、资源、约束与交付位置；Workspace形成可恢复输入与候选，保留Requirement→Obligation→Criterion映射、逐范围审查和精确接纳，随后可靠物化正式定义。M1容器、M2封存、M3新建完成、M4激活是四类事实，不能合并为一个ready布尔值。

#### 6.1.1 M3的新建完成含义

M3要求正式合同已确认封存，必需创建记录/索引/绑定/资源准备完整，精确启动准入与当前检查通过，并持久记录完成回执；该事实成立时尚无Run。回执是历史完成事实，readiness certificate是有版本和期限的就绪证明：Definition/startup input/config/资源版本及能力政策变更使相应当前资格失效，历史M3不删除。NC01是该证书唯一完整合同。

#### 6.1.2 默认手动与有界自动

默认在M3后由用户点击“激活执行”。明确自动模式可以一次提交完成候选政策接纳、封存、M3和独立激活，但不伪造对未知候选的人工签字。授权须有明确主体、来源权限、对象/动作/候选边界、预算/风险范围和期限，可撤销；Workspace持久归属及竞态规则见NC02。UNKNOWN、越界、到期或必须人决时暂停，不降低目标适配授权。

首次激活持久绑定稳定initialactivation identity，与传输requestId分离；手动/自动竞争归并同一Run，首个Run终态后迟到首次激活仍返回原Run。真正再次运行须新的明确执行意图。详见NC03。Web可以不展示瞬时“Run=0”，但必须能从审计证明M3先于Run；之后显示实际Run状态。

### 6.2 执行、证明与交付

Run绑定精确定义及启动快照；Orchestrator将源义务映射为覆盖责任，组织有界工作、真实能力操作和验证，逐义务裁决closure，正式closeout交给Result及Run。执行完成不等于目标完成，required缺口、UNKNOWN和未决风险不能被页面或任务成功覆盖。完整AgentLoop、四机转换和终态/恢复算法只在[Core](Workorch_核心AgentLoop与状态机合同基线.md)维护。

### 6.3 修订与历史

定义变更依§5.9及NC04/NC05；恢复、取消、Resume、Restart及删除依P1–P4。可执行历史分支/replay尚须D3触发决议，不能用只读审计重现替代。资源/权限及配置变化先判断对既有承诺的影响：只重做同合同下失效检查不自动改约；若改变承诺则必须ticket。

## 7. 概念与实现证据的关系

本文不是代码差距台账。当前固定源码参考为5360caf36e081717087557f34d2af3af1c16dc16；旧5554296证据及附录69模型族仅用于历史追溯。当前已实现、剩余缺口、未核验、CI和测试状态唯一见[总纲当前差距](Workorch_逐项收口总纲与首项方案.md#current-gaps)及[差距台账](Workorch_逐项收口总纲与首项方案.md#gap-ledger)，具体接收topic及Go映射见[组件具体合同目录](contracts/组件具体合同目录.md)。静态存在、设计完成、实现完成、测试通过与live通过须分开。

### 7.1 目标逻辑名不冒充现有Go类型

NC00–NC05中的目标记录/操作名是设计合同标识；尚无准确源码类型映射时明确标为target，不能因英文CamelCase就推断类型已存在。所有新记录复用既有owner和持久化边界；本文不要求新增40个struct/表/组件。

### 7.2 既有细化的模型归属

DefinitionAuthoringMapping是DM08提案wire，canonical GoalSpecification由owner编译，不是新权威；SuccessByteAudit属于DM37/DM08审计，不是运行证据。独立Goal requirement是完整来源锚点，不要求虚构重复DoD，但原子承诺仍须提炼为义务。RuntimeTerminalResultMaterialSeal属于DM35，RuntimeLineageAssertion及其评估属于DM07/32/33，post-terminal delivery属于DM36派生视图。这些不扩充40组概念或新增验收引擎。

2026-10-12增量仍归既有40DM：ContractChangeProposal/revisions归DM26，独立DefinitionChangeCandidate/Reviewed归DM08/09；新增不是第二Decision或第五Runtime。PreparationDiagnostic归DM37并引用DM02输入身份，保留原始失败/unknown与同一authoring预算责任（DM25）。详尽NC复用边界只见[NC源码映射](contracts/创建与定义变更合同.md#source-mapping)，当前缺口状态仍由总纲唯一登记。

最新源码已将Workspace/Orchestrator/Governance/System置于kernel，角色基线为1 Initiator、5 Modules、16 Blocks，见[固定组织说明](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/AGENTS.md#L15-L29)。源义务向Runtime传递已有具体封存校验：[SourceObligations编译及一致性反证](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/activationports/workspace_execution_intent.go#L47-L101)；这使DM04/05/20的可追踪输入有复用基础，但不证明逐源语义充分性或最终all-of闭包已验收。

## 8. 22个运行单元的模型覆盖

下表用于确保领域讨论没有遗漏组件，不表示各组件全部需要业务代码调整，也不表示各组件已验收。

| 运行单元 | 参考角色 | 模型职责/交互重点 | 本轮处理 |
|---|---|---|---|
| workorchruntime | Initiator | owner-scoped持久化基础设施；无独立业务终态 | 保持单一基础能力，参与资源生命周期核验 |
| workorchd | Module | DM01–18入口适配与跨owner应用流程 | 不另建领域权威；只Web创建，移除TUI创建入口由产品面批次处理 |
| governance | Module | DM26–28治理策略计算及typed决议提交 | 不接管Decision的持久proposal生命周期，不创造第二协调权威 |
| orchestrator | Module | DM19–25、31–35，运行裁决与证明 | 保持唯一SM-2/3/4语义owner |
| system | Module | 系统级配置/状态聚合、装配及管理 | 实际管理模型按owner核对；系统投影不夺取业务状态 |
| approval | Block | DM27 | 精确风险授权及有效期 |
| decision | Block | DM26/28，durable治理proposal生命周期 | 决议对象/版本及matching runtime acceptance归属 |
| eventstream | Block | DM37 | 受控审计元数据、SSE；原始诊断不改写，不作恢复权威 |
| file | Block | DM13/24/30的文件操作事实 | 路径边界、原子性、回执/产物版本 |
| git | Block | 仓库资源、revision与DM24 | 工作树/提交事实，不自行通过验收 |
| llm | Block | DM14/24/39、规划和审查建议 | provider/context/use事实，不发布终态命令 |
| mcp | Block | 远端工具操作与DM24 | transport与实际effect的未知状态 |
| mcpregistry | Block | DM39 | 注册配置与执行权限分开 |
| memory | Block | DM38/15 | 记忆项与RecallTrace，知识非默认证据 |
| result | Block | DM30/35/36元数据与结果存储 | 接收closeout，不重新裁决 |
| run | Block | DM18/29及opaque DM19存储 | 生命周期、CAS、唯一后继与恢复 |
| shell | Block | 操作/validation recipe与DM24 | 临时副本不是OS/网络强沙箱 |
| skill | Block | DM15/39 | skill资产、外部目录和Workspace-local资源分开 |
| soul | Block | DM15 | 行为边界与绑定快照，不覆盖用户约束 |
| ssh | Block | 远端资源/操作与DM24 | 远端权限、unknown和重复副作用 |
| web | Block | 网络资源/操作与DM24 | URL/DNS/重定向策略，不代表进程全局网络隔离 |
| workspace | Module（最新源码已位于kernel） | DM01–16、40的定义及删除协调 | 保留唯一owner/journal；目录/角色迁移事实不等于D8全部目标或集成验收通过 |

Base、process service、HTTP/Web、repository adapters等属于横切实现载体，不应为了凑齐模型数量另计入22个运行单元。共享生命周期修复可能影响很多单元，但每个模型仍保留独立责任，不能借共同适配夹带业务重写。

## 9. 55项设计要求的范围覆盖索引

本表回指前轮R01–R55需求编号，仅证明“本方案有归属”，不授予需求PASS，也不将Roadmap强行提升为本批交付项。

| 需求ID | 对应模型/章节 | 后续验证关注 |
|---|---|---|
| R01 | DM01–18，§6.1/6.2 | Goal+资源到启动的产品边界D1 |
| R02–R07 | DM02–09 | 原文保留、义务/判据关联、逐项审查与长Goal边界 |
| R08–R11 | DM08–12/20/34 | 封存、修订及源义务无损映射 |
| R12–R18 | DM19–25 | 每义务推进路径、动态规划、预算、fail-closed与实际多步任务 |
| R19–R21 | DM14/18–24/39 | provider解耦、有限决策及独立状态权威 |
| R22–R29 | DM24/30–33/27 | provenance、验证等级/独立性、三值结论；grade5单独决议 |
| R30–R33 | DM32–36 | 全required闭包、freshness及最终逐项追溯 |
| R34–R36 | DM11–15 | 模板继承/合并、policy快照及行为边界 |
| R37–R42 | DM13–15/23–27/37/39，§6.3 | 隔离威胁模型、权限、网络、secret全链边界 |
| R43 | DM02/12/15/38 | 有界上下文且不丢原Goal与关键来源 |
| R44–R48 | DM18–19/23–29/37/40 | 恢复、重放、持久化、审计、取消/恢复 |
| R49–R50 | DM11/14/21/39 | 逐用途能力，不因有模板或多角色就声称多用途达标 |
| R51 | DM25/34–37 | 指标来自真实权威事实，质量指标不以token数替代 |
| R52 | §2/5/8 | 指定组织规范、22单元及强类型事件边界 |
| R53 | §6全链、§10分批 | 最小公开端到端闭环与真实验收 |
| R54 | DM07/11/14/33/39 | 高级模板、强验证、marketplace/scale保留路线图归属，非当前全部必做 |
| R55 | §4/5/6 | 模型提供建议，Harness对权限、执行和证明强制约束 |

## 10. 范围追溯与分项冻结

本文件保留领域范围，冻结/实施批次及G0–G4唯一由[总纲](Workorch_逐项收口总纲与首项方案.md)管理。每项冻结须定位精确文档/合同版本、owner、影响的DM/R、具体决议、独立验收预期及证据状态；“工作基线已确认”不能替代带范围的冻结记录。真实改约先经§5.9，不能仅编辑冻结台账完成批准。

| 保留的原核对项 | 唯一语义位置 | 详细实施/验证位置 |
|---|---|---|
| MC01 要求类型 | §4.3、4.9 | 组件合同的定义接收能力、验收规格 |
| MC02 覆盖与闭包 | §4.4、4.9.1 | Core与验收规格 |
| MC03 配置 | §4.5 | 组件合同配置字段规则 |
| MC04 验证 | §4.6 | Core、能力合同及验收规格 |
| MC05 资源 | §4.7 | 资源合同、NC01/NC05及验收规格 |
| MC06 环境与授权 | §4.8 | 具体能力合同、NC02及验收规格 |
| MC07 创建完成 | §6.1、§11.2 | NC00–NC03、Web交互及WC-01–26 |
| P1–P4 | §5.4–5.7跨域保证 | Core算法及组件具体合同 |
| 合同变更 | §5.9、§11.1 | NC04/NC05及VS-NC04/VS-NC05 |
| 定义命令/审计 | §5.8 | NC00及组件具体合同 |

原MB00–MB06仅为讨论切片别名，保留映射、不另排实施顺序：MB00→C01/C02；MB01→C02/C03/C04/C12/C15及Web W范围；MB02→C05/C06/C07；MB03→C04/C07/C09；MB04→C06/C09/C10/C11；MB05→C08/C12/C13；MB06→C04/C12/C13/C14；C16–C18覆盖所有适用证据与集成。生命周期/持久化/独立oracle按实际依赖前置，不能等到“恢复批”才处理。逐批退出不豁免最终候选构建的累计受影响复验。

## 11. 仅在必要时请求确认

用户已要求继续完善，只有需要确认时给出清单。因此不把全部工程字段、目录组织或D1–D8逐项提交用户；本节保留真实产品分歧和触发时点。未确认不等于删去原目标，也不阻断无依赖部分。

### 11.1 已确认：D2，修改完成并approve后激活新Run

用户已明确选择：涉及设定目标、验收标准或合同约束变化，必须发起完整ticket；完成候选修改并通过approve后，重新激活新Run执行。该项无需再次确认，也不再保留同Run合同epoch替换为并列待决方案。

- 原Run合同及历史不可改写；新Run绑定新正式定义并关联实际前驱，旧产物可复用但验收credit需按新合同重新判断
- approve绑定最终修改候选和提案精确版本；修改后需重新approve。正式物化、匹配采纳、旧Run安全交接及新Run启动准入独立核验，产品规则见§5.9，详细协议见NC04/NC05
- 该决议解除DM18/19/29的产品路线分歧，但不表示接口映射、生命周期恢复或实际验收已经完成，也不授权当前代码实施/live
- 同一合同内任务分解、普通replan、获准recipe、安全重试及资源readiness重查不触发此合同变更流程，除非其实际改变承诺或另有治理政策

本版当前没有需要用户重复确认的D2问题。其余真正影响后续批次的产品问题在对应冻结节点提出，不将已确认事项反复列入待确认清单。

### 11.2 已确认：M3完成点及D1双模式激活

| 产品问题 | 已确认决定 | 冻结边界 |
|---|---|---|
| 新建“完成”点M | M3：合同已确认封存、必要资源准备和启动检查通过，尚未创建Run | 首次完成回执为历史事实；当前readiness须重查，M4另行创建Run |
| D1：手动与一次提交自动推进 | 默认确认合同后由用户点击“激活执行”；同时保留明确授权下审查通过自动启动 | 精确候选接纳及独立执行授权分别留痕，产品边界见§6.1.2，具体授权及UNKNOWN处理见NC02 |

本节两项不再列入待确认清单。MD方案格式已确认；产品合同MD导出保持可选，不是当前冻结阻塞项；若未来拟提升为新建required产出，才就新增承诺单独决议。D2已确认且适用于两种模式，自动推进不能改写已有承诺或省略变更ticket及最终候选approve。

### 11.3 由工程评审继续收口，无需逐项向用户确认

- Requirement→Obligation→Criterion及逐义务独立证明，是完整目标与真实验收要求的技术细化；本版选择每项义务一个primary Requirement、允许共享实现/证据，不增加多父义务复杂性
- 长期Workspace承载历史Run与每Run绑定精确定义，可作为基础组织；执行环境隔离是独立合同，不能以容器长期存在推导弱隔离即可满足全部目标。D2已确认通过新定义/新Run承接，不在原Run更换合同
- D4按动态分解、覆盖守恒、变更失效和有界再规划的能力验收，不要求用户选择是否新增名为Task/Graph的实体；具体载体优先复用现有canonical ledger
- D6采用强制限制兼容检查、明确字段规则及来源链。未有合成规则的冲突先拒绝并解释，随后按具体字段逐批补齐union/max/override等目标能力；这不是删去模板继承或把“全拒绝”当最终完整实现
- D8的目录/角色迁移已在5360caf参考代码中落地：**Workspace位于kernel并为Module，保留既有owner身份与唯一删除journal**。当前批次复核导入装配、typed合同、生命周期、存储兼容及所有消费者，不重复安排目录迁移；发现不兼容时重开相应工程评审。若将来另迁协调职责，仍必须证明唯一账本与恢复连续性，不能复制到workorchd。本轮只更新设计，不修改该实现

### 11.4 条件触发时才提出的新确认

D3可执行历史分支/副作用replay、D5不可信代码及强隔离威胁模型、D7外部独立/人工最终验收，在相关能力批次或实际资源风险触发时明确用户承诺、能力和授权。当前保留目标、边界与未验证标记，不因延后决定而从R01–R55中删去它们。只读审计重现不能替代可执行replay，Approval不能替代人类验收，scratch复制不能替代强隔离。

## 12. 参考与证据边界

- [固定代码参考5360caf](https://github.com/muidea/workorch/tree/5360caf36e081717087557f34d2af3af1c16dc16)
- [固定组织规范go-multi-module-dev 2.6.0](https://github.com/muidea/skills-repo/blob/6ba50452fccd33c912b07a0648fbb97acf15d756/skills/go-multi-module-dev/SKILL.md)
- 当前源码诊断、剩余差距与核验范围统一见[总纲](Workorch_逐项收口总纲与首项方案.md)；具体合同见[目录](contracts/组件具体合同目录.md)

本轮仅刷新Markdown设计文档，未修改项目代码、执行项目测试、CI或live。所有目标协议均可被审阅，不等同已有实现或全部冻结。冲突按§1.4处理，不能按文件时间或“写得更详细”自行覆盖已确认目标。

## 附录A 历史69模型族覆盖索引

本附录沿用前版5554296时期的69项覆盖索引，归入正文40组概念；只保留历史可追溯性，绝不作为5360caf的最新类型总数或现状清单。新增细化归属见§7.2，精确现行字段查组件合同目录。实体、值对象、事实、投影及内部恢复记录不可一律升级为聚合。

| 历史模型族（原参考实现/规范） | 正文归属及关系说明 |
|---|---|
| WorkspaceTemplate | DM11 |
| TemplateSource / effective template snapshot | DM11/12 |
| RuntimeProfileContract | DM12 |
| AcceptanceDimension / DeliverableExpectation | DM06、DM12：领域完成维度与交付声明 |
| EvidenceRequirement / VerificationPolicy | DM07：证据判据与验证策略 |
| BootstrapRecipe / BootstrapArtifact | DM12、DM13、DM30：初始化声明、目标资源与产物期待 |
| ProviderConfig / Model catalog / ProviderInferenceSelection | DM39、DM12、DM14：provider配置、运行选择快照、可用性 |
| MCP Server / Capability | DM39 |
| Runtime capability catalog / operation descriptor | DM14、DM23：能力描述与操作合同 |
| ResourceRef / ResourceSnapshot / OwnerFact | DM13 |
| TargetSelection / ValidationSurface / OperationAdmissibility / Feasibility | DM13、DM14、DM23、DM33：精确目标、可执行事实与验证面 |
| Directory boundary | DM13、DM01：资源目录边界及Workspace绑定 |
| Workspace | DM01 |
| DefinitionAuthoringCreation | DM01、DM02：创建身份、输入及可恢复阶段 |
| DefinitionAuthoringInput / Record | DM02 |
| DerivationRequest / SpecificationRequest / CompilationRef | DM02、DM08、DM09：输入派生、规格请求及审查/编译追踪 |
| GoalSpecification | DM08 |
| Requirement | DM04 |
| Obligation | DM05 |
| SpecificationVerification / CompletenessReview | DM07、DM09：证明方式声明与完整性审查 |
| DefinitionAdmissionCandidate | DM08/16 |
| DefinitionAdmissionResult | DM16 |
| DefinitionAuthoringConfirmation / confirmation journal / confirmed history | DM10：精确确认、物化journal与历史 |
| Goal / DoD | DM03/06 |
| WorkspaceDefinitionItem | DM03、DM06、DM08、DM20：Goal/DoD稳定定义项及CWI来源 |
| DefinitionRevision / WorkspaceStartupInputContract | DM08、DM12、DM16：正式定义版本、启动快照及准入绑定 |
| WorkspaceActivationAdmission | DM16 |
| WorkspaceDeletionRecord / tombstone | DM40 |
| Run / RunLineage | DM18/29 |
| RuntimeBootstrap / ActivationContext | DM12、DM18、DM19：Run封存启动输入及聚合初始化 |
| ReplacementIntent / recovery lease | DM17、DM18、DM29：显式重启意图与恢复/后继事务 |
| Runtime aggregate / RuntimeStateRecord | DM19 |
| DefinitionCoverageMapping / semantic dependency graph / partitions | DM05、DM08、DM20、DM21：源义务/定义项到覆盖与依赖的映射 |
| RuntimeCoverageItem / RuntimeCoverageItemFact | DM20 |
| DefinitionFocus / CoverageFocus / AcceptanceSlice | DM20、DM21、DM32：覆盖焦点、证明范围与绑定 |
| AllOfDependencyGate / TerminalAllOfGate / Witness | DM20、DM34、DM35：依赖all-of与终态闭包见证 |
| SemanticPlanDocument / ProofProposal / PlanningGeneration | DM21 |
| Candidate / RequestSession / RequiredOutputManifest | DM19、DM21、DM22：规划候选、逻辑请求与必须输出清单 |
| AuthoritySnapshot / AuthorityTransaction / ConvergenceState | DM19：Orchestrator内部运行权威状态/事务 |
| SealedDispatch / OperationExecutionEnvelope | DM22、DM23：封存dispatch及执行身份/影响边界 |
| Operation / Effect / Receipt / ReplayProof | DM22、DM23、DM24：操作尝试、副作用与owner回执/对账证明 |
| RecoverableWait / DriveLease / RecoveryAuthorization | DM19/29 |
| ExecutionBudgetPolicy / consumption sub-ledgers / BudgetView | DM25 |
| Step / Checkpoint / ToolCall / Patch / ValidationResult | DM21、DM22、DM24、DM33：工作/调用/修改过程及验证观测；checkpoint非恢复源 |
| CoverageEvidenceBinding / CoverageEvidenceProof | DM32 |
| EvidenceFact / EvidenceLedger / EvidenceRecord | DM31 |
| OwnerRevisionRef / OwnerRevisionSet | DM32 |
| Validation observation / assessment evidence / Assessment | DM24、DM31、DM33：真实检查回执、证明事实及评估 |
| RequirementOutcome / evidence item outcome | DM32、DM34、DM36：适用证据、义务判定及用户结果投影 |
| RuntimeResultCloseout / terminal all-of witness | DM35 |
| Result | DM36 |
| Artifact / deliverable reference | DM30 |
| TerminalDelivery / owner acknowledgements | DM35/36/01/18 |
| Decision Ticket / Resolution | DM26：治理问题及正式决议 |
| Approval / ResolutionProposal / principal | DM27、DM28：授权主体、许可与待采纳proposal |
| RuntimeGovernanceBinding / resolution acceptance | DM19、DM26、DM28：当前运行绑定及matching决议采纳 |
| DefinitionClarificationBinding | DM08、DM26、DM29：定义澄清、决议及修订后继关联 |
| AutoGovernancePolicy / Trace | DM28、DM37：治理策略计算及审计轨迹 |
| Soul / SoulBinding | DM15 |
| Skill / SkillProvenance | DM15/39 |
| MemoryItem | DM38 |
| MemoryRecallTrace | DM38 |
| RuntimeActivationContext / ContextManifest / ContextBundle | DM12、DM15、DM38、DM39：启动配置、指令/记忆及provider上下文引用 |
| EventRecord | DM37：不可变审计记录 |
| EventHub Command/Data/Result | §5：各接收owner的typed接口合同，不新增业务实体 |
| Workspace/Run/coverage/task/timeline/result projections | DM01、DM18、DM36、DM37：多领域只读展示；覆盖/任务来源见DM20/21 |
| ProviderRequestAudit / metrics / health / cache diagnostics | DM14、DM25、DM37、DM39：能力健康、用量投影、provider审计与诊断 |
| System defaults use case | DM11、DM12、DM39：默认模板/策略及provider配置的应用编排 |
| Runtime dependencies / framework lifecycle | §5、§8、附录A.1：基础设施依赖、quiescence/drain及释放合同，不新增业务owner |

### A.1 必须明确但不虚构已有实体的横切模型

- 执行环境/隔离边界：明确Run的逻辑工作目录、validation scratch、进程/远端执行环境和网络边界。当前临时副本不构成强沙箱；是否新建独立ExecutionEnvironment实体依威胁模型决议，本文不假定已经存在
- 身份与权限上下文：命令携带可信主体、对象归属、动作和作用域，凭据仅受控引用；不是新建用户/RBAC系统的既定方案。必须核对现有鉴权接入，不把RequestID当授权
- 生命周期：owner startup acquisition ledger、quiescence/drain回执、retirement/tombstone等是安全恢复必须表达的事实或内部机制，不是一组可以自由修改的用户业务实体
- 类型分类：实体拥有身份；值对象/快照按精确内容绑定；事实/回执只由来源owner产生；派生视图可重建；内部事务记录只服务所属owner恢复。不得因其在同一列表中就为每项建立repository或公开CRUD

### A.2 当前核验入口

当前差距与冻结台账统一见总纲；逐topic/Go字段、兼容及接收合同统一见组件目录，失败切点与独立expected统一见验收规格。本附录不复制当前状态。

### A.3 领域与权威规则的固定版本来源

以下是前版已读取的固定版本规范依据；它们描述现行合同，不替代本文件列出的目标差异决议，也不证明代码运行已通过。

- [docs/10-domain-model/overview.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/overview.md)
- [docs/10-domain-model/workspace-goal-dod.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/workspace-goal-dod.md)
- [docs/10-domain-model/run-result-artifact.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/run-result-artifact.md)
- [docs/10-domain-model/result-closeout.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/result-closeout.md)
- [docs/10-domain-model/governance-decision-approval.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/governance-decision-approval.md)
- [docs/10-domain-model/instruction-assets.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/instruction-assets.md)
- [docs/10-domain-model/capability-catalog.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/capability-catalog.md)
- [docs/10-domain-model/memory.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/memory.md)
- [docs/10-domain-model/event-observability.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/event-observability.md)
- [docs/00-core/runtime/ownership-and-language.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/ownership-and-language.md)
- [docs/00-core/runtime/state-machines.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/state-machines.md)
- [docs/00-core/definition-convergence/canonical-definition.md](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/definition-convergence/canonical-definition.md)


## 附录B 版本与旧章节去向

| v0.6原内容 | v0.7唯一位置 | 保留追踪身份 |
|---|---|---|
| §3–4实体、关系与MC语义 | 本文§3–4 | DM01–40、MC01–07全保留 |
| §5.3通用合同头、§10.2大模板 | 组件固定合同规范及组件目录；总纲冻结登记 | 不丢对象、版本、CAS、幂等、unknown、owner及审计要求 |
| §5.4–5.7、§6.2运行顺序 | Core；本文只保留P保证 | P1–P4 |
| §5.8、§5.9、§6.1完整新建/变更协议 | NC00–NC05 | M1–M4、D1/D2、reasoned ticket、精确approve |
| §7、§10.4当前差距/冻结台账 | 总纲 | 固定代码SHA与原风险状态 |
| 附录C.1–C.3贯通例与单变量反例 | 验收规格 | 原Goal、源义务和所有正反例 |
| 附录C.4–C.5、Web WC-01–26 | 验收规格NC/WC追溯 | 旧Run不换约、初次激活迟到重放等原义务 |

- v0.1：统一40组逻辑模型、69类实现模型族、22单元和55项要求归属，建立主要关系及待决项
- v0.2：依据《Workorch 业务模型完整性与充分性复核》修订§1、§4、§5、§6、§10及本附录；补齐最小语义与跨模型行为规则。原DM、R及运行单元身份不变，当时未新增业务owner或擅自批准D1–D8/M3
- v0.2.1：依据2026-10-08用户确认，将v0.2登记为逐项收口与渐进冻结的工作基线，补入§1.5确认记录及§10.2分项冻结记录要求；仅更新状态与流程说明，40组DM、既有关系、技术规则及M3/D1–D8待决状态均不变
- v0.3：依据用户“继续完善，只有在需要进行确认时给出待确认清单”的要求，补齐§4.9最小字段/版本/引用/all-of合同、§5.8定义命令及审计、§10.4冻结台账及附录C正反例；§11将当前必要确认收敛为D2，Web产品及高级能力决议按依赖延后，工程规则继续自主细化。40组DM、69类模型族、22单元、R01–R55索引保留；不将工程建议记为用户决议或全部冻结
- v0.4：依据用户明确要求，§1登记“修改设定目标、验收标准或合同约束必须发起ticket并完整说明原因”；扩展DM26及§5.9，明确提案/授权决议/技术校验/应用分离、版本CAS、原合同与副作用保留；依据用户随后明确决议，将D2登记为“修改完成并精确approve后，新定义封存并激活新Run”，补充安全交接/首个Run/幂等恢复，更新§6.3、§10冻结台账、§11并补充C.4。沿用原40组DM及既有owner，不创设平行ticket账本，不把已确认流程误记为具体变更或本轮实施获批
- v0.5：依据用户明确确认M3新建完成点及D1默认手动/明确授权自动模式，同步§1、DM10/17、§6、§10、§11及Web专项v0.2；区分政策接纳与人工确认、M3历史事实与M4当前执行、授权边界和幂等恢复。新增C.5验收场景；D2、40组DM、69类模型族、22单元和R01–R55索引保持不变，未修改代码或执行验证
- v0.6：以7993f63增量刷新Authoring mapping、独立精确Goal、六审/确认fence、成功原文审计、preRun及LLM查询完整性；同步终态材料与typed lineage的已实现边界及剩余风险。旧证据明确标为历史，M3/D1/D2及Web-only不变，未将静态修复或测试存在升级为验收PASS
- v0.7：按唯一文档职责去重；完整保留DM01–40、MC01–07、P1–P4、R01–55及原产品决议。NC00–NC05实例化M3证书、有界自动授权/撤销、首次激活、精确改约采纳和安全交接；历史类型索引不冒充最新计数。测试长矩阵迁至验收规格，当前差距/门禁迁至总纲，未改变目标或授权代码/live
- 本轮交付状态：文档修订与只读设计核对；未改项目代码、未执行项目测试/CI/live。不得把“已补入方案”记为“实现已通过验收”

- v0.8（2026-10-12）：以5360caf刷新kernel角色、源义务启动绑定、独立改约候选/Decision登记及原始诊断保真归属；40DM、M3/D1/D2、运行matching先于formal Decision和adoption、后继屏障不变。

## 附录C 验收义务入口

原贯通Goal及C.1–C.5正反例、Web WC-01–26均迁至[验收规格](validation/验收规格.md)，该文件维护实填fixture、输入、故障切点、独立expected、恢复终点与证据要求。本文件不再保存并行长测试矩阵。保留§4.1的A+B短例仅用于解释业务意义，不代替验收规格。
