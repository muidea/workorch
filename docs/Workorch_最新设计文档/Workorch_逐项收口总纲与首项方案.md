# Workorch 逐项收口总纲与首项方案

- 文档编号：WOC-CLOSURE-PLAN
- 版本：v1.4（设计冻结候选，尚未形成实施/验证通过结论）
- 日期：2026-10-12
- 当前阶段：只读评审与方案输出；不修改项目代码，不运行测试、CI 或 live
- 组织原则：从项目目标与总体架构明确边界，按小批次能力闭环逐项集成验收；现有代码作为可复用参考；所有收口方案使用 Markdown

> 本文唯一维护 R01–R55 的原始要求、C01–C18 的评审归属、B 能力批依赖、G0–G4 退出规则，以及固定代码的事实/缺口状态。业务含义、逐边状态合同、具体 typed 接口、实际验收规格和组件迁移细节各有独立权威文件，入口见[设计文档总索引](00_设计文档总索引.md)。本版把原有问题清单推进为可追踪的分批设计；“合同已写明”“用户已确认产品语义”“工程已实现”“验证已执行”分别记录。本轮只更新 Markdown，不修改项目代码，不运行或监控测试、CI、服务或 live。

## 1. 基线与收口含义

### 1.1 固定评审基线

| 对象 | 固定版本或来源 |
|---|---|
| Workorch 当前增量复核 | `5360caf36e081717087557f34d2af3af1c16dc16`（2026-10-12固定；提交时间2026-10-12 05:28:53 UTC） |
| 上一版文档代码基线 | `7993f63c4437dbf65c33c2e844683fd6389d63e0`；[120提交差异](https://github.com/muidea/workorch/compare/7993f63c4437dbf65c33c2e844683fd6389d63e0...5360caf36e081717087557f34d2af3af1c16dc16)；完整tree比较454新增、210删除、567同路径修改 |
| Workorch 历史评审 | `5554296240074054db34b0c551f4d326cd34a361`；[13提交/266文件差异](https://github.com/muidea/workorch/compare/5554296240074054db34b0c551f4d326cd34a361...7993f63c4437dbf65c33c2e844683fd6389d63e0) |
| 强制组织规范 | [go-multi-module-dev v2.6.0](https://github.com/muidea/skills-repo/blob/6ba50452fccd33c912b07a0648fbb97acf15d756/skills/go-multi-module-dev/SKILL.md)，提交 `6ba50452fccd33c912b07a0648fbb97acf15d756` |
| 产品目标 | 《Workorch 多用途 Harness Agent 整体设计文档》v1.0（2026-09-28）及[设计讨论](https://chatgpt.com/share/6ac732aa-5eec-83e8-8bd1-16ff3b7cedb6) |
| 既有评审 | 《Workorch 全面架构实现评审与真实验收方案》，2026-10-08，R01–R55 和 22 个运行单元清单 |

后续进入每项代码评审时，先登记实际待评审提交并比较基线差异。旧结论必须复核适用性；不能将本文件里的已查证事实自动外推到新提交。

### 1.2 四类状态独立记录

| 状态轴 | 可用值 | 当前口径 |
|---|---|---|
| 方案评审 | 缺合同 / 已填合同 / 冻结候选 / 评审通过 / 需修订 | 详见总索引的权威路径和完整性复核；本包新增具体合同与验收规格，未经批准的工程设计仍是冻结候选，不能统称评审通过 |
| 用户决议 | 无需决议 / 待确认 / 已批准 / 已否决 / 已替代 | D1/D2按后续已明确设计口径；D4/D6/D8为工程评审项；D3/D5/D7按触发条件后续确认，见§3.3 |
| 代码实施 | 未开始 / 进行中 / 待代码评审 / 已合入 | 本轮未修改代码；新 SHA 已有上游实现增量，按§1.4分别记账，不等于整改验收完成 |
| 验证结果 | 未运行 / 阻塞 / FAIL / PASS / 不适用（附理由） | 本轮不运行测试/CI/live；沿用已核实的既有CI静态记录：新 SHA 为FAIL，静态新增实现不等于验证PASS |

“方案收口”要求目标、方案、接口、反例和退出条件完整且分歧处理清楚。“代码收口”还要求实际提交通过代码评审。“验证收口”要求适用门禁有真实证据。“最终收口”要求所有 required 目标完整覆盖并通过最终候选构建复验。不得把这些状态压成一个“完成”。

### 1.3 必须长期保持的原则

1. 每个原始要求保持稳定 R 编号，细分用 `Rxx.a`；不因重排目录、合并任务或分期交付而丢项
2. 一个工作项可实现多个要求，一个要求也可依赖多个工作项；每项要求仍需独立验收
3. 用途、强验证、强隔离等尚未具备的能力保留在最终目标清单。若分期，必须记录未交付项和后续门禁；一期通过不等于最终完整目标通过
4. 每项只有一个明确的权威 owner；禁止通过平行状态机、双 journal 或跨模块直连绕过 typed EventHub 合同
5. 先写独立判据和反例，再实施。执行方不能删除 required assertion、自签验收或用空测试命中换取 PASS
6. 新要求、语义变更及版本迁移必须影响到相应封存、恢复、投影和验收，不能仅改 JSON、目录或 expected value
7. 当前仅编写方案。下表“实施退出条件”和“验证门禁”均为将来必须达到的条件，不是已经执行的结果

<a id="current-gaps"></a>

### 1.4 最新固定事实与证据状态（5360caf）

相对上一版7993f63新增120次提交。完整recursive tree均未截断：4887→5131个blob；454新增、210删除、567同路径修改，4110同路径未变。目录移动会同时体现为新增/删除，不能把这些数字当独立业务改动数，也不能只依赖可能截断的compare文件列表。本轮重核源码与既有CI，未运行项目测试或live；以下状态只对上述固定SHA成立。

| 观察ID | 当前可证事实 | 证明上限和后续指向 |
|---|---|---|
| OBS01 | 创作mapping/原文audit继续保留；eligible specification现经CompileRuntimeSourceObligationInventory进入activation与DefinitionSeed.SourceObligations；终态新增逐源结论与防漏项校验 | F1由“缺逐源接线”转为已有静态链路待独立验证；不能据函数名证明自然语言语义充分或公开实施全链 |
| OBS02 | 六类review各自从全部Required IDs派生Expected scope；PASS经ValidateReviewCoverage拒任何Missing，optional不能替代required，独立Goal也须覆盖；未知review status/N/A不作为通过出口 | F2结构覆盖门禁已静态修复；required集合本身的自然语言语义充分性仍须独立审查，不能由ID集合完整自动证明 |
| OBS03 | 独立exact required USER_EXPLICIT Goal的32KiB例外与普通4KiB边界继续保留 | F3作为回归范围，不再重复立项；raw诊断bytes保真与规格合法规范化分开 |
| OBS04 | 单一terminal compiler、material seal、CAS后Result→Workspace→Run ACK已有；新增current-binding/proof适用过滤及完整lineage、sealed source obligation结论 | 复用现有compiler；不再把已实现current proof过滤当待从零补项，X02剩余owner依赖闭环仍需逐能力证明 |
| OBS05 | File读取/变更、Shell验证复制输入、Git commit读取增加真实依赖来源；新QueryRuntimeFileReadDependencies用于配对原receipt与当前owner观察 | 具体能力/operation/read-set范围必须逐项登记，不能由File正例推断全部owner均闭合 |
| OBS06 | Runtime hash/规范化/校验大量性能优化采用同次复用；新Run持久API使用typed RuntimeStateRecord且语义仍归Orchestrator | byte/hash/clone/alias、陈旧缓存及跨请求污染负例必须保留；不得把性能改动视为授权弱化raw校验或缓存权威 |
| OBS07 | Shell四阶段验证与原receipt/原文受限读取有新增接线；ImplementationWork非nil政策公开availability仍为false | X01公开执行门禁未解除；新的受控恢复/receipt测试不替代公开正例；原文查询另有X04接线失败 |
| OBS08 | Workspace已迁kernel并为Module，现22单元=1 Initiator+5 Modules+16 Blocks；Base checked任务/订阅、Lifecycle drain/generation、ShutdownChecked及Initiator Quiesce均有实现 | S1–S6不再照旧描述“仍是Block/void API完全未改”；目录与接口修正不等于所有停止切点已验证 |
| OBS09 | 创建binding/index部分持久化恢复、原创建request身份重放、页内存/服务端恢复及原文诊断有改进；改约有独立proposal/候选派生六审准备链 | M3完整证书、有界自动激活、exact最终approve/adoption/newRun、cross-Run unknown全链仍按P01–P04分别证明，不以改约准备子集冒充全流程 |
| OBS10 | 当前SHA既有CI run38215841512失败：owner topic audit报告Shell原文查询无生产sender；topic inventory307而expected306；随后managed MVP步骤跳过 | 这是当前失败证据，不能继续沿用旧15分钟超时为本SHA原因，也不能通过只改expected数字掩盖缺发送方 |

当前源码依据按主题唯一列在此处，接口和算法正文继续引用对应CT/RT/NC：

- [source obligation激活接线](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/activationports/workspace_execution_intent.go)
- [RuntimeBootstrap来源](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/activationcore/runtime_bootstrap.go)
- [当前Workspace规格与审查](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification.go)及[准入compiler](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/definitionadmission/compiler.go)
- [review required范围计算](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification_review_coverage.go)及[authoring请求/输出合同](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/specifications/authoring_request.go)
- [terminal compiler](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_result_compiler.go)及[material seal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime/terminal_material_seal.go)
- [ImplementationWork实际availability](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_implementation_work_policy.go)
- [当前full发布manifest注册门禁](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/scripts/verify_deployed_convergence_manifest.py#L289-L314)
- [新改约应用入口的范围限制](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/contract_change.go)
- [组织与数据保真约束](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/AGENTS.md)及[Workspace Module](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/module.go)

[当前CI run38215841512](https://github.com/muidea/workorch/actions/runs/38215841512)、[job114700248734](https://github.com/muidea/workorch/actions/runs/38215841512/job/114700248734)于2026-10-12 07:10 UTC结束为failure。失败步骤“Verify 持久规格创作合同（仅 mock）”中：TestOwnerEventCommandsHaveProductionSenderAndReceiver报告`/shell/validation/query-raw-output`的sender=false/receiver=true；TestOwnerEventContractsAreCompleteAndUnique报告inventory=307/want306。TestOwnerEventMessageGraphsAreConcrete在同job通过，但不能抵消两项失败。后续Managed verify-ci-mvp-file为skipped，不能记PASS或已执行。此处只是读取既有结果，没有启动、重跑或持续监控CI。

旧[run37916240152](https://github.com/muidea/workorch/actions/runs/37916240152)的Orchestrator Biz 15分钟超时保留为旧7993f63构建的历史证据；不据此推断当前根因已修或仍相同。最新权威路径见[总索引](00_设计文档总索引.md)，公共合同不在现状台账重复维护。

## 2. 从整体到细节的收口顺序

### 2.1 主线与依赖

C01 → C02 → C03 → … → C18 是评审主题的组织和阅读顺序，不是要求所有18项详细方案完成后才能规划第一批，也不是逐组件实施顺序。先冻结C01/C02中影响当前批次的目标、架构不变量及待决接口，再展开该批涉及的最小详细方案。无关的后期决议可保留待决，不能暗中裁掉最终目标。实施批次按第2.3节组织，同一时刻只推进一个实施批次；其他主题可做只读评审。C13的威胁模型与隔离约束须与C02一起冻结，在任何真实effect执行前落实；C13编号靠后不允许先执行再补安全。C16 的独立判据与证据采集设计必须在首次 live 之前就绪，可在 C01 冻结后提前准备；不能做完 live 再发现证据不可用。

| 项目 | 收口主题及预期文档 | 主要依赖 | 方案必须回答的问题 | 将来退出条件 |
|---|---|---|---|---|
| <a id="c01"></a>C01 | 项目目标与设计基线 | 无 | 最终目标、分期、D1–D8、55项原始要求如何不丢失 | 原始要求均有归属；所有阻塞当前阶段的决议有明确记录 |
| <a id="c02"></a>C02 | 总体架构、权威与状态机 | C01 | Goal→Contract→Plan→Execute→Evidence→Verify→Replan 的职责和唯一权威 | SM-1～4、owner、typed事件、读写权及禁止路径完整，无双权威 |
| <a id="c03"></a>C03 | 模块组织与22个运行单元 | C02 | Initiator/Module/Block分类、Base、包边界、装配和依赖 | 22项逐一有组织结论和逐叶审查清单，符合指定规范 |
| <a id="c04"></a>C04 | 生命周期与基础能力 | C02、C03 | 全局停止/drain/release、部分启动、超时重试、Base错误 | 全阶段失败保留依赖与资源账本；重试成功；drain后才能释放 |
| <a id="c05"></a>C05 | Goal规格、完整审查与封存 | C01、C02 | 原文/来源、长Goal、审查矩阵、需求/义务、不可变seal | 漏审、缺义务、长度边界、旧seal漂移均被确定检测 |
| <a id="c06"></a>C06 | 源义务到运行期证明映射 | C05 | 每项原始obligation如何无损映射且独立关闭 | A+B共享DoD但仅证明A不能完成；共享工作不丢独立义务 |
| <a id="c07"></a>C07 | 模板组合、Profile与快照 | C05、C06 | 多父组合、严格合并、policy、版本和旧Run一致性 | 真实组合与冲突矩阵满足D6；新模板不热改旧Run |
| <a id="c08"></a>C08 | 规划、分解与再规划 | C06、C07 | 所有未闭义务的工作路径、图变更、预算及blocked | 多步实施/修复不丢目标、不弱化政策、无无限重试 |
| <a id="c09"></a>C09 | 公开激活与真实执行链 | C04、C07、C08 | policy→准入→bootstrap→effect→receipt是否公开可达 | 原公开fullflow实际完成；unsupported仍拒绝；工作receipt不直接授验收credit |
| <a id="c10"></a>C10 | Evidence与独立验证 | C06、C08；执行验收另需C09 | 来源、等级、语义/确定性验证、独立性与UNKNOWN | 每条判据有实际断言；伪PASS、零tests、弱证据被拒绝 |
| <a id="c11"></a>C11 | Closure、freshness与交付投影 | C09、C10 | all-required关闭、陈旧证据、真实owner依赖、Result一致性 | 原规格逐项可反查；历史accepted与当前credit分开，缺一不完成 |
| <a id="c12"></a>C12 | 持久化、恢复、取消与迁移 | C04、C06、C09、C11 | CAS/unknown/预算、修订后继、取消、格式兼容及安全回滚 | 切点恢复无重复副作用或预算重置；唯一后继；旧格式明确处置 |
| <a id="c13"></a>C13 | 权限、隔离、网络与秘密 | C02、C03；落地验收需C09、C12 | D5威胁模型、不同capability边界、秘密全链防泄露 | 受信和不可信范围准确；对应越界/secret canary负例成立 |
| <a id="c14"></a>C14 | 上下文与可观测性 | C05、C10、C12、C13 | 有界上下文、来源、原文受限保真/公开投影安全、审计与authority分离 | 无原Goal静默截断；日志/投影不可驱动权威；重建可核对 |
| <a id="c15"></a>C15 | 多用途及公开产品面 | C07、C09–C14的合同与实现就绪 | coding/doc/data/ops/RAG/database逐用途实际承诺，入口及取消 | live前完成用途合同、实现与非live门禁；真实正反例在C17形成后回填本项；所有公开入口有覆盖 |
| <a id="c16"></a>C16 | 验收oracle、producer与证据身份 | C01/C02；随C05–C15的合同逐步接线，不依赖其live PASS | 真实观察器、构建/环境身份、13槽位、可信来源校准 | live前oracle冻结、producer接线及正负校准完成；全部实际live槽位由C17采集、C18聚合 |
| <a id="c17"></a>C17 | live与故障场景 | C04–C15适用合同/实现/非live门禁及C16采集准备就绪；另需执行授权 | 从最小目标到真实完整Goal，正反例、切点、预算/停止 | 同候选构建场景期望正确且证据完整；负例Run失败也可场景PASS |
| <a id="c18"></a>C18 | 全量回归、最终复核与发布判定 | C17；所有最终目标required项 | 全目标是否满足、遗留项是否影响required、可否复算 | 全部required有可信证据；full门禁全过，最终候选复验；部署另行授权 |

门禁分三层：方案批准 → 实现和确定性校准就绪 → 真实验证通过。表中的最终退出条件可能包含第三层；进入C17只要求其明确列出的前置层，不要求先有C17自身将产生的live证据。C17取证后回填C09–C15的验证状态，C18统一复核。C16校准可使用有明确身份的受控fixture，但不能将fixture记录当成真实场景证据，因此没有“先完成live才能准备producer”的循环。

C编号是稳定归属，不要求为每个C再复制一份全文。当前实际合同由总索引指向相应权威文件；一个文件可承接多个C，一项C也可由多个具体合同联合覆盖。仅确有新的独特方案时才新建Cxx文档，并登记与既有合同的关系。修订不得换编号绕过未通过项。

### 2.2 22个组件的承接与评审范围

全部22个运行单元均有审查与回归归属，但不能据此认定22个组件的全部业务代码都必须重写。当前/目标角色、A/B/C调整证据、22个 C03 子项、根/Biz/events/models/业务叶子清单及静态源码依据，统一维护在[22组件组织与调整评审](Workorch_22组件组织与调整评审.md)；具体保留、适配、替代、迁移与独特验收在[MIG01–MIG22及组件索引](components/00_迁移总索引与集成顺序.md)。本节不再复制同一张22行现状表。

C03.01–C03.22 与 MIG01–MIG22 顺序对应。Base、进程 shell、HTTP/Web/TUI适配与 vendor 生命周期桥是横切载体，不增列为第23个业务组件。已有21个 Hub 单元的 Biz/Base 不需要重复“补 Base”；当前 magicCommon v1.5.18 不因规范示例版本而降级。是否改代码按职责和叶子证据判定，未审部分保留“待审”，不能以根文件检查替代全模块审计。

<a id="batches"></a>

### 2.3 实施批次：每次形成一个可集成验收的能力闭环

所有B批共用设计前置：先冻结Core核心循环、四机写权、失败恢复和终态交接合同，再固定该批受影响的具体CT/NC以及SG1/SG2独立验收方案，最后才可进入获准的实施。这里要求设计基线先稳定，不要求先实现或验收整个Loop；B01也不能绕过此共同前置。

C编号是评审主题，B编号是未来实施批次，两者为多对多关系。例如一个“源义务在运行期仍可逐项核对”的批次，可以同时涉及C05/C06/C10/C11和Workspace、workorchd、Orchestrator、Result。跨模块本身不是范围失控；只要服务于同一个明确结果、权威唯一、接口变化完整且能单独验收，就可以构成一个小批次。不得为了逐模块推进而让接口两端长期不一致。

每批只承诺一个可观察结果。方案须列出要求编号、接口、文件/包、持久化格式、调用方、非目标及回归边界；不以固定文件数判断大小。若同时引入多个互不依赖能力、新状态机和大范围目录迁移，应拆批。必须原子一致的接口两端、读写兼容或生命周期阶段不得机械拆开；可采用版本化合同或受控适配，但过渡期也只能有一个权威写入者。

<a id="gates"></a>

| 批次门禁 | 必需内容 | 未通过时的处理 |
|---|---|---|
| <a id="g0"></a>G0 进入 | 上一批验收提交及其证据固定；当前实际代码SHA和已知失败登记；本批涉及的C01/C02决议、接口与安全边界明确；变更清单、复用分类、独立oracle及回归用例就绪 | 不开始实现；补方案、复核基线或缩小范围，不借用旧SHA的PASS |
| <a id="g1"></a>G1 实现可集成 | 仅在获准实施后修改；合同的生产者/消费者同步适配；静态组织检查、适用单测/合同测试通过；持久化变更有版本与恢复方案 | 停留本批修正；禁止连带塞入下一批功能 |
| <a id="g2"></a>G2 本批集成 | 通过真实模块装配及typed事件验证本批结果，记录实际运行用例数、正反例和故障路径；使用mock/fixture时明确替代边界 | 若失败、缺证据、零命中或环境阻塞，均不算本批通过；停止并修复或重新评审 |
| <a id="g3"></a>G3 累计回归 | 执行本批影响面回归及此前已验收能力的适用回归；对跨模块共享合同扩大回归；无新未解释回退 | 重开受影响旧项；完成修复和复验前不进入下一实施批 |
| <a id="g4"></a>G4 批次退出 | 普通能力批按预先冻结的适用 SG1/SG2 取得局部集成证据，并完成 G1–G3；最终 Web 集成批及产品收口必须 SG3。所有证据绑定同一候选构建，未交付/阻塞登记完整 | 不要求每批先有全产品 Web PASS；也不得把普通批局部退出写成产品完整或公开 live 通过。范围与层级不得在失败后下调 |

SG1/SG2/SG3 的具体符合性定义由[组件分层规范](Workorch_组件固定合同与分层验收规范.md)维护。G 是批次推进门禁，SG 是证明范围，两者不是同一套编号：SG1不单独代替G2/G3，普通能力批的必要集成仍须具备；SG3不前置到无关的内部合同批。

#### 能力批依赖登记

以下 B 编号是本包唯一实施依赖登记；均为未来设计分批，未开始实施、未批准运行、未取得G4。每批只选择表中一个可观察结果；可按接口保持原子性拆为 Bxx.a/Bxx.b，但不得新增第二套编号体系。B03/B04等不要求按数字死板串行，依赖满足后择一推进；同一时刻仅一个实施批次。

| 批次 | 本批唯一可观察结果与相关C | 必须满足的依赖 | 主要组件 | 预定证明范围与不承诺内容 |
|---|---|---|---|---|
| <a id="b01"></a>B01 | 定义准入不会遗漏任一适用 required review scope；C05/C06 | C01已定产品语义、C02唯一定义权威、目标合同与迁移方案；复用 OBS01–03 | MIG22/02/11，审计按需MIG08 | SG1+SG2；含实际authoring/确认链，F3作回归；不声称Runtime全闭包 |
| <a id="b02"></a>B02 | 每项源 obligation 在Runtime inventory、工作路径和逐项closeout中可追踪，遗漏必阻塞；C06/C08/C10/C11 | B01的身份/封存合同；SM-2/3/4逐边合同及独立oracle | MIG22/02/04/16/15 | SG1+SG2；受控真实owner链可先验证；不以fixture宣称公开执行 |
| <a id="b03"></a>B03 | 停止新准入后，owner、task、Hub和provider有序排空，失败可继续对账且资源不提前释放；C04 | C02威胁/生命周期合同、完整共享调用方清单；可与B01/B02前后调序 | MIG01及全部参与停止协议的owner | SG1+SG2；禁止不兼容生命周期混合态；所有真实effect相关最小保障须先具备 |
| <a id="b04"></a>B04 | 复核已完成的Workspace Module迁移保持唯一owner、原数据及在途journal可恢复；C03/C12 | 已冻结D8工程方案，生命周期与存储接口适用保障；迁移读写映射 | MIG22/01/02及真实删除参与方 | SG1+SG2；验证组织和单写切换，不夹带目标语义改变；当前目录/角色迁移已落地，不再次搬迁；剩余兼容/恢复按真实证据验收 |
| <a id="b05"></a>B05 | 创建操作可恢复地到达真实M3记录，旧检查漂移不会放行M4；C05/C07/C14/C15 | B01；创建资源操作所需B03保障；完整readiness/hash作用域和M3合同 | MIG22/02/05/09/11及绑定资源owner | SG1+SG2；正例限已接线的支持能力，未就绪能力必须阻塞；最终全Web由B11复验 |
| <a id="b06"></a>B06 | exact approved ticket经Workspace journal物化新Definition，Run owner建立唯一后继；C12 | B01/B02及B05相关激活/授权合同；前驱unknown屏障、Run lineage与恢复判据 | MIG07/03/22/02/04/16/08，许可按需MIG06 | SG1+SG2；批准与应用分开，响应丢失可对账；普通同Definition Restart不强制ticket |
| <a id="b07"></a>B07 | 已支持用途的公开ImplementationWork从准入到真实工作、逐义务证明及一致终态交付形成闭环；C09/C10/C11 | B02、B03、相关B05/B06接口；安全/预算/unknown最小保障；真实producer和独立oracle已接线；X01须解除 | MIG02/04/16/15和实际capability owners | SG1+SG2及本批公开入口正反例；若入口只能私有harness则本批未退出；最终Web完整范围由B11负责 |
| <a id="b08"></a>B08 | 一个指定中断/治理恢复切片保持原义务、预算与已发生effect，且不重复执行；C12/C13 | 前批对应能力已验收；其自身安全恢复前置早于真实effect，不能拖到本批才设计 | MIG03/04/06/07/16及本场景owner | 每次B08.a等选择cancel、同合同restart、unknown或恢复切点之一；SG1+SG2累计回归 |
| <a id="b09"></a>B09 | Workspace删除及迟到消息不会复活状态、误删用户资源或遗忘未对账effect；C03/C12 | B03/B04适用保障、资源归属、各owner tombstone/retirement合同 | MIG22及实际删除参与方 | SG1+SG2；每个owner唯一journal/事实责任；不承诺不可证明的任意回滚 |
| <a id="b10"></a>B10 | 一个明确用途或条件能力有真实合同、可信证明与公开正反例；C07/C10/C13/C14/C15 | B07及本用途安全、资源、provider/验证能力；D3/D5/D7等仅按真实触发需要补决议 | 按用途选择MIG，不机械覆盖全部22个 | coding/doc/data/ops/RAG/database及条件扩展逐个子批；只对声明用途授结论，原高阶目标不消失 |
| <a id="b11"></a>B11 | 最终候选在公开Web完成适用required全目标并可复算交付；C17/C18 | 所有最终required批及其证据、C16全槽位producer、授权/环境/费用与停止条件就绪 | 全部受影响MIG，22单元完成归属核对 | 必须SG3、适用full门禁和最终候选复验；任何required缺口仍阻塞最终收口 |

验收case、fixture和oracle的实际内容集中在[验收规格](validation/验收规格.md)，本表不复制每条case正文。B05/B06普通SG2退出只表示其所选合同范围经过实际双端验证；若其冻结scope本身承诺完整公开创建/后继激活，则该公开正例也是本批必需，不能以“最终B11再测”豁免。

当前没有经本轮运行验证的可用集成基线：旧 SHA 存在公开implementation-work fullflow失败；新 SHA 的现有CI运行37916240152同样FAIL，实际为完整go test的orchestrator/biz包15分钟超时（§1.4）；根因未定，不能从旧失败、单个在跑测试或提交标题推断。因此未来首批先建立并记录真实基线，不能将现有代码直接标为绿色。已知失败可纳入带理由的基线清单，必须证明未扩大失败范围；本批承诺解决的用例仍失败时绝不允许退出。涉及安全、权限、数据一致性、重复外部副作用、虚假完成或本批required判据的失败，不得以“历史问题”为由豁免。其他例外须记录范围、影响、理由、批准及后续必交付批次；不能借例外放行最终required门禁。

一批退出后，下一批从其已验收提交开始。同一时刻只有一个实施批次，避免多个未验收变更叠加使故障无法归因。需求变化或批内发现额外问题时先评估影响：属于本批结果必需的，修订方案并重新评审；不属于的，登记后续批次。不得无记录扩范围，也不能为了保持“小批”忽略安全前置。当前阶段只设计这些门禁，不执行代码调整、项目测试或live。

### 2.4 建议首批及公开live的边界

第一批继续规划为 **B01：源要求/义务完整性及准入边界**，以最新已实现的sealed-reference mapping、成功原文audit、独立exact Goal及32KiB例外为复用基线，不从头重做。先对已新增的review适用范围及逐源inventory接线进行反例核验，仅为仍有实际差距的部分制定修改；以遗漏、无效映射、长Goal边界、不完整审查反例复核既有实现。涉及C01/C02、C05及C06的接口定义；预计以Workspace和workorchd为主要影响面，最终范围由具体文件审查确定。若同批无法建立单一明确结果，优先拆出“准入不漏审”的最小闭环；已实现的长度合同纳入回归而非重复修复，不把三类修复强行捆绑。

B01的进入条件不是18项全部批准，而是影响它的目标语义、authoring授权边界、唯一规格owner、对外数据合同、旧格式处理及oracle已明确。C06运行期映射接口需要前瞻定义以避免B01数据模型返工；这不等于同批实现完整planner、executor及最终closure。B01集成验收应通过实际公开authoring/准入入口到权威持久状态的适用链路，并核对失败时不得形成可激活的有效seal。若测试只能从内部方法驱动，必须记为内部集成，公开入口集成仍待验证。

后续批次依§2.3的B登记逐项选择与细化，不另立第二实施顺序，也不把编号存在视为评审通过。共享生命周期整改必须有一体化阶段协议，可按支持兼容的子路径接入并验收；不兼容的阶段不得在同一进程混搭。任何真实effect前，必须满足其所需生命周期、恢复和安全前置。

公开implementation-work的policy/readiness阻塞经当前源码重核仍存在（Shell四阶段producer与receipt链已有新增接线，但公开ImplementationWork availability仍关闭）。早期可用受控测试harness证明本批真实模块交互、权威状态和反例，但必须标为“harness/受控集成”，不能通过绕过公开readiness来宣称公开端到端或live通过。公开链打通前，其live状态保持阻塞；公开链就绪并获执行授权后，再进行对应真实端到端验证。每个批次预先定义自己应达到的层级，禁止验收失败后临时降低层级换取PASS。最终仍需C17/C18完整候选构建验证。

### 2.5 现有代码的复用决策

设计依据的优先顺序为：用户确认的项目目标与授权边界 → 经评审的总体架构及强制组织规范 → 本批能力合同与验收判据 → 现有实现参考。已有函数、包名、目录、状态机和测试不是目标正确性的来源；已有良好设计也不因“新方案”而自动失效。

| 复用类别 | 判定条件 | 必须补充的证据 |
|---|---|---|
| 直接复用 | 语义、权威、接口、组织和安全约束均符合目标，无本批必改差异 | 固定版本、适用范围、代码审查及本批集成/回归证据；历史PASS不能免复验 |
| 适配复用 | 核心行为可保留，接口、职责、生命周期或证明语义需要调整 | 保留部分与修改部分清单、依赖方适配、迁移和反例；不让兼容层形成第二权威 |
| 重新实现 | 根本语义/权威不符，或适配复杂度与风险高于受控替换 | 替代理由、最小替换范围、新旧合同差异、切换及回归方案；不能据单点问题推导整体重写 |

分类按职责或代码单元而非整个组件一刀切，同一组件可同时包含三类。尚未深审的部分保持“待判定”，不能把22个组件全部预先判为需改或可原样保留。复用测试可作回归参考，但expected值和断言也须对照目标审查，不能用旧测试固化已有缺陷。

代码是否复用与历史数据是否保留是两项独立决策。重新实现不授权删除Run、receipt、审批、审计、删除journal或预算历史；直接复用也不免除schema/恢复兼容审查。owner ID或持久化布局可以在目标需要时经版本化迁移调整，必须保证单一权威、在途操作对账和可证明的历史关联。回滚不能遗忘已发生的外部副作用，不能通过重置证据与预算制造“干净成功”。

逐批验收可以减少定位成本，不能承诺后续无需重复验证。新证据、共享合同、规范、需求或构建身份变化都可能要求重开相关项；最终候选始终要复验适用required目标。

## 3. 首项详细方案：C01 项目目标与设计基线收口

### 3.1 本项目标与非目标

本项要得到一个不会在后续反复改变含义的“最终能力合同”：输入是什么、允许何种自动化、如何分解、什么才算完成、失败如何处理、哪些能力属于最终目标以及分期证据要求。

本项不调整代码、不裁掉原始要求、不批准实现或live操作，也不声称源码已经符合。后续架构和模块方案可提前研究，其中D1/D2应执行既定语义；D4/D6/D8由工程方案论证，不能把实现选择重新变成用户问卷；真正新增的能力或授权差异另行确认。

### 3.2 建议冻结的最终能力陈述

用户提供 Goal、资源、模板及授权边界后，Workorch 应形成覆盖完整目标的可验证 Contract；在不删除未完成要求、不降低验收政策的前提下自动分解、执行和受控再规划；每个必需义务由适用的独立证据与判据验收；只有全部必需义务成立时才声明目标完成。执行中对权限、成本、未知副作用、恢复和取消进行有界处理，最终交付可追溯到源要求、实际产物及运行环境的结果。

“多用途”“自动完成”“可恢复”“隔离”“独立验收”必须展开成逐项能力，不接受这些词单独作为完成标准。高阶能力可以有分期，但在最终目标中的状态始终可见。

<a id="decisions"></a>

### 3.3 决议分层与当前设计口径

D编号保持稳定，但不再将八项统一标成“待用户选择”。D1/D2已经明确的语义直接进入方案；D4/D6/D8属于工程评审与验收；D3/D5/D7只在相应后期场景触发时提出具体范围问题。设计口径不等于已实现或授权执行。

| 决议 | 当前口径与处理 | 未交付能力与影响 |
|---|---|---|
| <a id="d1"></a>D1 自动化入口 | 默认M3完成后由用户点击“激活执行”；仅在用户显式选择有边界自动推进策略时，在审查和准入通过后自动激活，不能从普通Goal提交或确认Contract推定执行授权 | 一次授权的易用性保留为可选模式；其授权范围、期限、预算、触发与撤销需有明确合同。影响R01、C05/C09/C15 |
| <a id="d2"></a>D2 规格修订 | 目标、验收标准或约束变化必须：提出ticket并完整说明原因 → 修改最终candidate → 用户批准该exact candidate → 生成新Definition → 创建新Run。批准“有此问题”或仅同意ticket不等于批准修改后的最终合同 | 旧Run/Definition不可热改；保留差异、原因、审批和唯一successor lineage；旧credit不自动继承，已有产物复用须重新取证。影响R08/R09、C05/C12 |
| <a id="d3"></a>D3 恢复与Replay | crash恢复、只读审计重现、历史分支执行分别定义；前两项按核心合同推进，历史分支执行在具体需求触发时确认，用新Run和新观察，不能重放既有副作用 | 不能用checkpoint等同完整Replay，也不静默删除后期执行Replay。影响R44/R45、C12/C14 |
| <a id="d4"></a>D4 动态任务分解 | 工程评审：以新增、分解、替换、合并、排序及多步Goal的反例选择复用或适配，实现始终维持唯一Graph/运行裁决权威 | 不要求用户选择内部图算法或类；保留R12–R18完整能力，C08/C09证明公开多步推进 |
| <a id="d5"></a>D5 运行隔离 | 先形成威胁模型；不可信代码或强隔离场景触发具体后端/权限范围确认。强隔离未具备前不执行不可信代码，不能用临时副本冒充OS/网络沙箱 | 后期范围待交付，不自动删去；任何受信范围限制须明确记录。影响R37–R42、C13 |
| <a id="d6"></a>D6 模板组合 | 工程评审：typed字段逐项定义单调合并、union/max/合法override及来源，未知冲突fail-closed；用base+language+domain反例验收 | 不以拒绝全部冲突冒充组合完成，不以JSON覆盖弱化父策略。影响R34–R36、C07 |
| <a id="d7"></a>D7 独立强验证 | grade5/人工最终验收作为后期条件能力保留；需求要求此等级时必须先建立可信来源、独立性、撤销和签署协议，缺失不得验收 | approval只是许可，不是credit；当前0–4不等于删掉强验证。影响R23–R28、C10/C15 |
| <a id="d8"></a>D8 Workspace组织 | 当前实现已完成：承担跨owner协调的Workspace位于kernel/workspace并为Module；保留既有owner wire身份。后续评审复核装配、数据兼容及恢复，不重复安排目录迁移或让用户重选内部分类 | 保持唯一删除journal/reducer和typed事件；在途恢复、单写切换与历史关联必验。影响R52、C02/C03/C12 |

### 3.3.1 M3完成边界与M4执行边界

M3/M4、Web-only、默认手动/显式有界自动以及seal后变更的完整目标合同由[Web创建专项](Workorch_Web新建Workspace合同与交互方案.md)及其目标合同链接维护。本文仅登记D1/D2的产品决定与批次影响；检查hash、证书有效性、activation race、ticket应用journal及cross-Run unknown等工程细节不在此另定一套规则。M3完成时尚无Run；普通Restart允许沿用同Definition，只有改变既定承诺才要求ticket→exact approve→新Definition→新Run。

### 3.4 全目标与分期交付登记规则

每个R项记录 `最终是否必需 / 适用条件 / 本阶段归属 / 决议 / 退出门禁 / 未交付后果`。原设计明确Roadmap的能力可分期，但其最终交付是否必需仍需产品决议；不能自动全部纳入当前发布，也不能自动全部排除。

需要确认的能力范围至少包含：支持的coding/doc/data/ops/RAG/database用途；真实模型/provider组合；不可信代码隔离；grade5/人工最终验收；历史分支执行；高级模板/marketplace/scale及质量指标。每项都应登记为“最终必须”“明确条件触发”“经批准不纳入最终目标”之一；未决时保持待决，不写成不适用。特定LSP-Bridge属于具体Goal的验收，不成为所有用途的通用前提。

阶段交付可以验收自己获批的范围，文件标题和结论必须写清“阶段范围通过”。最终目标收口时，不能还存在未交付的最终required项。

### 3.4.1 决议不能成为高阶能力的终点

下列是固定承接位，不表示已批准实施。C01负责决议；技术主项负责实现方案及验证。每项在最终范围确认后必须细化完成条件，不允许只挂在C01并等C18发现漏项。

| 范围/要求 | 稳定承接子项 | 细化要求 |
|---|---|---|
| R24/R28，grade5/人工最终验收 | C10.01 强验证与人工验收协议 | 来源身份、独立性、信任与撤销、事实签署、伪造/重放拒绝 |
| R38，隔离执行后端 | C13.01 不可信执行隔离；C13.02 执行后端 | 威胁模型；按批准范围选择容器/OS/远端/VM，验证真实边界，非机械绑定Docker |
| R45，历史分支执行Replay | C12.01 恢复与只读Replay；C12.02 历史分支执行 | 前者核心；后者是否最终必需待决。一旦纳入必须新Run、重新观察和幂等约束 |
| R50，specialist/multi-agent | C15.01 多用途基线；C15.02 specialist与多agent | 目标分工、权限/预算、交接证据、失败传播和最终单一验收权威 |
| R51，质量/效率指标 | C14.01 上下文与审计；C14.02 质量和效率指标 | false-completion、coverage、成本/延迟等定义、测量来源和评估集，不只数token |
| R54，高级模板/marketplace/scale | C07.01 高级组合；C15.03 marketplace；C15.04 scale与benchmark | 明确发布/信任/兼容、并发及隔离、可复现实验和容量目标；每项需判定最终范围 |

条件分支如果经批准不纳入最终目标，保留有理由的决议和原要求；若仅推迟，状态必须为“待交付”，不能“不适用”。C01完成冻结前，凡已纳入的能力必须有上述子项或同样稳定的新子项和可验收结果。

### 3.5 形成的合同和记录

C01详细方案确认后，维护以下逻辑记录，可先放在同一Markdown内，不要求立即引入新服务或新数据库：

1. 目标基线：原始文本、版本、来源、完整性hash的未来记录方式、最终与阶段范围
2. 需求登记：R01–R55、原始章节、适用性、主收口项、依赖项、独立验收判据
3. 决议登记：D1–D8及新增决议、分层、选择理由、既定语义来源/工程评审记录/适用用户确认、受影响R/C编号
4. 变更记录：旧/新语义、必要性、是否改变验收、需重开的收口项
5. 范围外记录：仅记录明确批准的排除，不接受“尚未实现”作为排除理由

### 3.6 本项评审反例与通过标准

| 检查编号 | 必须识别的反例 | 通过条件 |
|---|---|---|
| C01-G01 | 55项中任一没有主收口项或对应原始要求 | R01–R55唯一主归属齐全；跨项依赖可多对多 |
| C01-G02 | 将“当前做不到grade5”写成“最终无需grade5” | 缺能力与产品决议分开，未决保留 |
| C01-G03 | 将“新Run恢复”直接宣称“任意历史Replay已支持” | 三种语义明确，差异有决议或保持阻塞 |
| C01-G04 | 同一义务共享DoD后在清单消失 | 义务身份保留且逐项验收；共享工作不合并完成条件 |
| C01-G05 | 以目录copy、Web域名规则宣称全进程强隔离 | 威胁模型、适用capability及未覆盖边界明确 |
| C01-G06 | 架构升级生成第二个删除journal owner | 单一权威及在途迁移约束明确 |
| C01-G07 | 方案评审通过被写为已批准/已实现/live已通过 | 四轴状态逐项区分，无运行证据则不标验证PASS |
| C01-G08 | 旧SHA结论直接盖章新HEAD | 后续代码评审前有版本差异和结论适用性复核 |

这些是文档评审判据，本轮不运行项目验证。C01方案可以在明确标注未决项后交付；但C01“基线冻结完成”必须等到阻塞范围/语义的决议处理完毕，不能带着未决定义继续给最终实现盖章。

### 3.7 本项状态与下一步

- 方案：v1.4按120提交增量刷新实际实现、剩余差距和验收入口，保留明确权威分工、完整要求登记及能力批依赖；具体合同与验收规格按总索引定位。已填设计仍需评审，不等于全包已批准冻结
- 决议：D1/D2既定；D4/D6/D8工程评审；D3/D5/D7按后期触发条件确认，最终required范围不默默缩减
- 代码实施：本阶段不执行；已观察到上游实现增量，不据此宣称本批验收完成
- 验证：仅文档结构与引用核对；不启动项目测试/CI/live；读取的新SHA现有CI为FAIL
- 下一项：按已填的C02核心基线及逐边索引评审B01所需接口和案例；不重复起草已有模型/转换/组件合同，也不因尚未运行验证就要求用户重新决定M3/D1/D2

<a id="requirements"></a>

## 4. R01–R55 全量追踪清单

保留原编号和旧SHA原评审原文作为历史来源，新增当前SHA承接列；历史缺陷不得直接用于当前派工。当前实现事实唯一维护于§1.4/§5，下面仅逐R定位适用结论，独立case仍在验收规格维护。下表“主项”仅代表主要方案归属，不限制关联整改。全部R项都要纳入C16的判据/证据设计和C18的最终复核；需live的项进入C17。特别是C04生命周期、C09公开执行和C16发布证据还包含原55项之外的实现不变量，不能因没有独立R编号而遗漏。

| 原编号 | 原设计要求 | 主项 | 历史静态判断（5554296，非最新状态） | 旧SHA来源及原验收落点 | 当前5360caf承接 |
|---|---|---|---|---|---|
| <a id="r01"></a>R01 | §1–3 35 自动全链入口 | C09 | 部分实现；Workspace 创作到准入，确认后独立执行 | [E02](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/05-product/product-architecture.md#L35-L56)<br>D1；真实公开入口闭环 | NC00创建恢复新增，D1有界自动仍待；P01/P02/X01；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r02"></a>R02 | §3 7 结构化规格 | C05 | 模型已实现；语义完备仍受义务下沉影响 | [E04](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/goal_specification.go#L13-L94)<br>修复义务映射与长 Goal | 规格保留，F2全集门禁及F1逐源启动绑定新增；F3回归；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r03"></a>R03 | §7 约束与假设保留 | C05 | 部分支持；原文/source refs 不等于无损降低 | [E03](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/workspace-goal-dod.md#L29-L82) [E04](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/goal_specification.go#L13-L94)<br>复杂嵌套约束 mutation | source内容进入inventory；语义无损仍独立验收，F1；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r04"></a>R04 | §7 14 必需维度覆盖 | C05 | 声明集合校验与缺维度负例已读 | [E05](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/definitionadmission/goal_specification_test.go#L47-L123)<br>模板漏维度端到端拒绝 | 维度与适用验证门禁复用；不授全Goal语义PASS，F2；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r05"></a>R05 | §7 五类需求来源 | C05 | source kinds 已实现；运行发现走新定义 | [E04](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/goal_specification.go#L13-L94)<br>source identity 贯穿终态 | source身份随inventory/terminal传递新增；F1；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r06"></a>R06 | §7 14 每要求有义务 | C05 | admission 关联与负例已读；源义务闭包仍缺 | [E05](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/definitionadmission/goal_specification_test.go#L47-L123)<br>每项独立 runtime proof | 逐obligation编译及terminal防漏新增；F1；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r07"></a>R07 | §7 六维完备性审查 | C05 | 六角色及 UNKNOWN 阻塞存在；覆盖并集无门禁 | [E04](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/goal_specification.go#L13-L94) [E05](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/definitionadmission/goal_specification_test.go#L47-L123)<br>逐要求×适用维度覆盖 | 每review required全集已静态修复；F2不再按旧缺陷重做；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r08"></a>R08 | §8 14 规格冻结 | C05 | 代码支持 immutable seed；同 Run 不热更新 | [E11](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go#L154-L209) [E16](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/workorchruntimecontract/runtime_definition_verification_policy.go#L8-L48)<br>Resume/replan 哈希不变 | 封存不变；新proposal不热改旧Definition，P03；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r09"></a>R09 | §8 受控修订 | C12 | 部分满足；新 revision+restart 替代同 Run 修改 | [E03](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/workspace-goal-dod.md#L29-L82) [E20](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/definition-convergence/runtime-clarification.md#L45-L113)<br>D2；旧 credit 不继承 | 无Run改约准备子集新增；最终批准/应用/新Run仍P03；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r10"></a>R10 | §4 6 22 对象分层 | C06 | Requirement/Obligation/CWI/证据有区分；降低有缺口 | [E04](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/goal_specification.go#L13-L94) [E10](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go#L30-L90)<br>无损映射为统一合同 | 分层不变，SourceObligations启动/终态接线新增；F1；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r11"></a>R11 | 附录B 目标终态全映射 | C06 | 缺口；源 obligation 身份与语义未确定性保留 | [E18](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/result-closeout.md#L68-L159)<br>最高优先级闭包修复 | 逐源终态输出与反漏项新增；F1/X02仍需真实闭环验证；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r12"></a>R12 | §9 未满足项有路径 | C08 | 现 CWI 有规划/公平推进；不证明原义务完整 | [E10](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go#L30-L90) [E12](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/semantic_planning_test.go#L69-L230)<br>缺 owner 明确 blocked | 源义务inventory新增；规划/证明全闭合仍F1/X01；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r13"></a>R13 | §10 规划输入完整 | C08 | eligible CWI 与上下文结构支持 | [E10](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go#L30-L90) [E30](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime-request-context-assembly.md#L11-L102)<br>实查完整输入及预算 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r14"></a>R14 | §10 执行前约束 | C08 | 合同与准入负例支持；未实跑所有 operation | [E05](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/definitionadmission/goal_specification_test.go#L47-L123) [E33](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/invariants-and-change-gates.md#L11-L55)<br>资源、policy、DAG 负例 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r15"></a>R15 | §6 9 10 动态图编辑 | C08 | 有再规划；通用实施图全部编辑未证明等价 | [E11](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go#L154-L209) [E37](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/semantic-convergence.md#L31-L84)<br>D4；多文件修复正例 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r16"></a>R16 | §11 受控再规划 | C08 | 有 generation/repair/wait；保持历史与 seed | [E11](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go#L154-L209) [E36](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/flows/repair-and-restart.md#L11-L27)<br>耗尽有界失败，不删难项 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r17"></a>R17 | §11 不弱化目标证据 | C08 | runtime seed/profile 固定；仍依赖源义务完整性 | [E11](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go#L154-L209) [E16](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/workorchruntimecontract/runtime_definition_verification_policy.go#L8-L48)<br>required 与 policy 不变 | sealed source inventory加强原义务保全；F1/X01；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r18"></a>R18 | §9 有界独立执行 | C08 | sealed operation 事务存在；公开 implementation readiness 受阻 | [readiness 43–64](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/workorchruntimecontract/runtime_implementation_work_policy.go#L43-L64)<br>公共全流；不借 private driver | 公开ImplementationWork availability仍closed；X01；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r19"></a>R19 | §17 工具与领域解耦 | C03 | 七 capability owner 分工；跨 provider 实战未证 | [E21](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/30-module-architecture/architecture-overview.md#L11-L78) [E22](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/30-module-architecture/block-boundaries.md#L11-L131)<br>多 provider 与非 coding 例 | 22单元边界及Workspace Module已落地；MIG按实际调用核验；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r20"></a>R20 | §19 LLM 无状态权威 | C02 | 合同规定模型产事实候选，Runtime 校验裁决 | [E33](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/invariants-and-change-gates.md#L11-L55)<br>恶意/畸形 authority 注入 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r21"></a>R21 | §20 状态分离 | C02 | Run/Runtime/Result 分工与合同存在 | [E18](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/result-closeout.md#L68-L159) [E21](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/30-module-architecture/architecture-overview.md#L11-L78)<br>执行成功≠验收成功 | typed RuntimeStateRecord不改变Run物理/Orch语义分权；OBS06；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r22"></a>R22 | §12 证据一等对象 | C10 | typed receipt/hash/fence 链路支持 | [E14](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy.go#L15-L100) [E33](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/invariants-and-change-gates.md#L11-L55)<br>核对真实 producer/环境 | File/Shell/Git原读取依赖新增；X02只按已覆盖子集授结论；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r23"></a>R23 | §12 13 证据等级 | C10 | 0–4 owner receipt 校验及篡改负例已读 | [E14](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy.go#L15-L100) [E15](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy_test.go#L9-L99)<br>明确各 profile 最低等级 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r24"></a>R24 | §12 13 grade5 | C10 | 后续强验证缺口；当前无可信协议且拒绝 | [E14](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy.go#L15-L100) [E15](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy_test.go#L9-L99)<br>D7；不得伪装已支持 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r25"></a>R25 | §12 独立验证来源 | C10 | 按 owner 去重；不同模型/context 并非同一维度 | [E14](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy.go#L15-L100) [E25](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/readme.md#L132-L180)<br>定义适用风险与独立性 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r26"></a>R26 | §13 确定性检查 | C10 | managed recipe 与 native attestation 有协议 | [E14](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy.go#L15-L100) [E25](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/readme.md#L132-L180)<br>逐准则实际命中 assertion | Shell四阶段conjunction/receipt有新接线；原文查询缺sender为X04；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r27"></a>R27 | §13 语义验证 | C10 | assessment 与裁决合同支持；语义校准待证 | [E18](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/result-closeout.md#L68-L159) [E31](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/llm/biz/runtime_candidate_context.go#L1-L198)<br>正确/错误/弱测试对抗集 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r28"></a>R28 | §13 人工验收 | C10 | approval/governance 有许可事实；通用最终验收待定 | [E22](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/30-module-architecture/block-boundaries.md#L11-L131) [E33](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/invariants-and-change-gates.md#L11-L55)<br>D7；批准不直接给 credit | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r29"></a>R29 | §13 三值结论 | C10 | spec 三值、runtime 四态；indeterminate 拒绝完成 | [E04](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/goal_specification.go#L13-L94) [E19](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/workorchruntimecontract/runtime_result_closeout_formal_test.go#L35-L99)<br>缺环境与确定失败区分 | raw诊断未保存不使known failure退unknown；NC00/OBS09；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r30"></a>R30 | §14 all-required 关闭 | C11 | CWI 三轴合同严谨；源义务闭包仍不完整 | [E18](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/result-closeout.md#L68-L159) [E19](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/workorchruntimecontract/runtime_result_closeout_formal_test.go#L35-L99)<br>少一项不得 COMPLETE | 逐源all-of终态防漏新增；F1/X02/公开门禁仍各自证明；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r31"></a>R31 | §14 Executor 无终审权 | C02 | owner receipt 与 SM-2 裁决职责分离 | [E14](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy.go#L15-L100) [E33](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/invariants-and-change-gates.md#L11-L55)<br>claim/exit0/fileexists 负例 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r32"></a>R32 | §14 stale 与 reopen | C11 | 部分支持；freshness 合同存在，closeout stale 回填待整改 | [E17](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/result-closeout.md#L11-L66) [E11](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/semantic_planning.go#L154-L209)<br>相交失效、无关保留 | current proof/lineage过滤已修，真实owner依赖新子集；X02；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r33"></a>R33 | §35 最终逐项报告 | C11 | CWI closeout 完整；源义务与 stale 投影有缺口 | [E18](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/result-closeout.md#L68-L159) [E19](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/workorchruntimecontract/runtime_result_closeout_formal_test.go#L35-L99)<br>原规格 ID 贯穿真实终态 | 封存逐源结论Web只读投影新增；F1/X02；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r34"></a>R34 | §15 模板提供标准 | C07 | 合同与模板支持，不限定固定 workflow | [E38](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/workorchdefaults/defaults/templates.json#L1) [E39](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/40-profiles-capabilities/workspace/builtin-default-resources.md)<br>非 coding 推导任务闭环 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r35"></a>R35 | §15 继承来源 | C07 | 多父 DAG、hash、钻石/环/深度测试已读 | [E06](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/template_inheritance.go#L29-L109) [E09](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/template_inheritance_dag_test.go#L27-L109)<br>父更新不改变旧 Run seal | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r36"></a>R36 | §15 严格组合语义 | C07 | 部分；DoD 保护但冲突拒绝不同于 union/max | [E07](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/template_inheritance.go#L112-L217) [E16](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/workorchruntimecontract/runtime_definition_verification_policy.go#L8-L48)<br>D6；多模板组合 fixture | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r37"></a>R37 | §16 每 Run 隔离 | C13 | 部分；长期 Workspace/临时副本非 OS 沙箱 | [E23](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/biz/shell_validation_sandbox.go#L16-L57) [E25](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/readme.md#L132-L180)<br>D5；明确代码信任边界 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r38"></a>R38 | §16 32 多执行后端 | C13 | 后期/未验证；不能由部署 Dockerfile 推出 | [E21](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/30-module-architecture/architecture-overview.md#L11-L78)<br>按隔离目标单独交付 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r39"></a>R39 | §17 25 高风险策略 | C13 | owner policy/approval 支持；跨工具一致性待证 | [E22](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/30-module-architecture/block-boundaries.md#L11-L131) [E26](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/biz/shell_policy.go#L22-L63)<br>Shell/SSH/MCP 负例 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r40"></a>R40 | §25 文件及破坏边界 | C13 | 部分；File scope/命令过滤非任意进程安全证明 | [E23](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/biz/shell_validation_sandbox.go#L16-L57) [E26](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/biz/shell_policy.go#L22-L63)<br>可信域或强隔离验证 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r41"></a>R41 | §25 网络策略 | C13 | Web URL/domain 实现已读；非全进程网络沙箱 | [E27](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/web/biz/url_policy.go#L43-L90)<br>跨 Shell/SSH 边界待证 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r42"></a>R42 | §25 secret 脱敏 | C13 | 形状脱敏与 typed 排除支持；任意秘密未证明 | [E28](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/common/redaction.go#L13-L88) [E29](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/eventstream/pkg/recordpayload/audit_sanitize_test.go#L11-L54)<br>canary 全链与错误输出 | 原始诊断受限保真与公共面secret保护分开，不改写原文；OBS09/NC00；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r43"></a>R43 | §18 有界上下文 | C14 | C0–C5 分区与 typed projection 支持 | [E30](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime-request-context-assembly.md#L11-L102) [E31](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/llm/biz/runtime_candidate_context.go#L1-L198)<br>长中文/大资源不丢要求 | hash/规范化同次复用新增，不授权跨请求缓存authority；OBS06；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r44"></a>R44 | §21 crash/幂等恢复 | C12 | CAS/aggregate 合同及恢复测试部分支持 | [E33](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/invariants-and-change-gates.md#L11-L55) [E34](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime_recovery_inventory_test.go#L14-L119)<br>fresh process 提交窗口 | 真实receipt独立进程恢复测试新增；未运行本轮case且非公开X01证明；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r45"></a>R45 | §3 Replay | C12 | 部分；审计读取存在，历史 authority replay 禁止 | [E32](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/event-observability.md#L11-L33) [E36](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/flows/repair-and-restart.md#L11-L27)<br>D3；定义重放承诺 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r46"></a>R46 | §23 持久对象分离 | C12 | 合同已声明；未逐实现审计所有仓库原子性 | [E03](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/workspace-goal-dod.md#L29-L82) [E21](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/30-module-architecture/architecture-overview.md#L11-L78)<br>损坏、恢复、owner 原子性 | typed runtime record与checked持久生命周期复用；SG1/SG2仍需证据；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r47"></a>R47 | §24 审计 append-only | C12 | 合同与 sanitizer 测试支持；全路径未证 | [E29](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/eventstream/pkg/recordpayload/audit_sanitize_test.go#L11-L54) [E32](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/event-observability.md#L11-L33)<br>revision/丢响应/删除边界 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r48"></a>R48 | §3 取消恢复审批 | C12 | 恢复/治理合同有；Cancel 各公开阶段未逐项确认 | [E33](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/invariants-and-change-gates.md#L11-L55) [E36](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/flows/repair-and-restart.md#L11-L27)<br>authoring/执行/在途取消 | checked stop进展与行政-only目标分开；S1–S5/P04；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r49"></a>R49 | §3 26 多用途 | C15 | 六模板存在；各用途 proof/live 成熟度未证 | [E38](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/pkg/workorchdefaults/defaults/templates.json#L1)<br>每宣称用途一个正负例 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r50"></a>R50 | §27 specialist agents | C15 | 后期；结构化多角色符合 MVP 方向 | [E04](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/goal_specification.go#L13-L94) [E31](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/llm/biz/runtime_candidate_context.go#L1-L198)<br>不以同模型/无聊天判错 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r51"></a>R51 | §29 质量效率指标 | C14 | 后期/未验证；计数不证明 false-completion 指标 | [E18](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/10-domain-model/result-closeout.md#L68-L159) [E30](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime-request-context-assembly.md#L11-L102)<br>先统一义务级测量口径 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r52"></a>R52 | §30 31 模块职责 | C03 | 22 单元框架已查；生命周期及 Workspace 分类有偏差 | [E21](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/30-module-architecture/architecture-overview.md#L11-L78) [E22](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/30-module-architecture/block-boundaries.md#L11-L131)<br>按第7页范围补叶子审计 | Workspace已kernel Module，1I5M16B；S6目录迁移完成，S1–S5转新版回归；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r53"></a>R53 | §32 Phase1 全闭环 | C15 | 部分；公开实施门禁、完整义务与 live 未闭合 | [E37](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/semantic-convergence.md#L31-L84)<br>真实修改→验证→Result | 逐源/私有producer有进展，公开X01和当前CI X04仍阻塞；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r54"></a>R54 | §32 Phase2–5 扩展 | C15 | 继承/再规划部分已有；高等级/市场/规模后期 | [E06](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/template_inheritance.go#L29-L109) [E14](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy.go#L15-L100)<br>明确本期支持边界 | 既定能力/条件范围保留；未新增已通过结论，按本R独特case验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |
| <a id="r55"></a>R55 | §33–36 Harness 约束 | C02 | 机制丰富；语义降低与公开可用性仍有缺口 | [E14](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime/verification_policy.go#L15-L100) [E33](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/docs/00-core/runtime/invariants-and-change-gates.md#L11-L55)<br>反例与真实交付验收 | 四机/无LLM权威不变，新增scope/依赖/保真反例；未授整体验收；[当前证据](#current-gaps) / [验收追踪](validation/验收规格.md#r-case-matrix) |

<a id="gap-ledger"></a>

## 5. 当前差距与依赖台账

本表是当前实现差距的唯一状态登记。OBS指§1.4的固定源码事实；“设计已填”不把工程状态改成已实现，也不把未运行改成PASS。目标合同若仍有缺项必须在完整性复核中单列设计阻塞，不能一律改名为“工程待验”。

| 问题编号 | 当前状态及必须保持的原问题 | 设计/实施承接 | 关闭所需证据 |
|---|---|---|---|
| F1 | 已有实质源码进展，验证待完成。source obligation从eligible specification进入bootstrap/DefinitionSeed并贯穿逐源terminal输出；原“身份未下沉”不可再整体沿用。语义充分性与完整公开链仍未证明 | C06；B01/B02/B07；业务模型与Runtime合同 | 逐源义务身份、映射必要性/充分性、共享DoD只证明A而不证明B的独立反例及终态可反查 |
| F2 | 结构必审全集门禁已静态修复。owner从全部Required IDs生成各review范围并拒Missing，parser和confirmation重复反证；optional/N/A不能抵消required。自然语言语义充分性与适用case运行仍待验证 | C05/C16；B01 | 明确expected范围、coverage差集与N/A依据；漏任一适用项不能seal；真正未知必须阻塞 |
| F3 | 已静态修复、未回归。OBS03独立Goal 32KiB例外和普通4KiB边界存在 | C05/C14；B01回归 | 中文UTF-8字节边界、伪造来源/required/exact例外、重复/混合Goal来源和旧记录用例 |
| S1 | 已新增ShutdownChecked、分阶段drain和Initiator保留/Quiesce接线；需核对所有真实参与者、失败传播与释放顺序，不能仅看到方法名授PASS | C04；B03 | 真实框架调用链；失败不标released；在途typed通路保留到排空 |
| S2 | 已新增timeout保留句柄/资源账本及再次停止路径；保留超过期限、重试及唯一释放的适用回归 | C04；B03 | 超过2秒在途、再次停止、完成后唯一释放；不以超时冒充已停止 |
| S3 | 已新增部分启动登记与分阶段失败清理；剩余逐参与方多切点验收，不能继续把旧“未started完全不清理”当全量现状 | C04；B03 | 多订阅中途失败与未started时完整清理；每个已取得资源可核对 |
| S4 | 生命周期锁/取消/退订错误处理已有增量；逐资源重试、panic及依赖排空仍须按新代码切点取证 | C04；B03/B08 | 不持锁等待自己、故障可返回、依赖和重试身份不丢失 |
| S5 | Base任务/订阅checked error合同已实现；原void API缺口转为实际调用方传播与失败路径回归 | C04；B03 | 同步/异步/超时结果可传播，所有实际调用方适配，无被吞错误 |
| S6 | 目录/角色静态迁移已完成：Workspace为kernel Module；不再安排重复搬迁。旧持久数据/在途journal/单写责任的适用验证尚需完成 | C03/D8；B04/B09 | 单一Module owner、单journal/单写切换、旧数据与在途恢复；具体角色结论由22组件评审维护 |
| X01 | 公开执行未闭合。OBS07私有ImplementationWork链已存在，但 executable admission 仍为false，不能按私有测试或producer名称宣称公开可用 | C09；B07 | 真实readiness、registry、dispatch/receipt/proof同一可达公开链；支持正例成功、unsupported仍拒绝 |
| X02 | current binding/proof适用性、完整lineage与逐源终态输出已有修正；真实读依赖新增File/Shell/Git子集。terminal compiler的OwnerRevisionSet仍从accepted evidence构造runtime_authority引用；真实读取依赖新子集不自动完成全owner依赖采样/当前性与材料封存，剩余范围必须逐项证明 | C11；B02/B07/B08 | 最新exact binding和lineage membership、真实owner revision/hash、错scope/旧binding/tamper负例及同material seal投递 |
| X03 | 发布证据未就绪。当前重读scripts/verify_deployed_convergence_manifest.py，REGISTERED_PRODUCERS仍按每个requirement生成空frozenset，fail-closed正确；不能空注册骗过门禁 | C16/C17/C18；B11 | 真实producer先接线，13个scenario×variant槽位按full profile实际命中，wrong-build/scope/tamper校准 |
| X04 | 当前CI证实Shell原文读取接收已存在、缺生产发送方；topic审计expected306与实际307漂移 | C03/C14/C16；MIG17及实际consumer；B07/B11 | 接通明确受限sender/receiver与身份/原文读取oracle，再核对准确topic清单；不能仅更新expected数字 |
| P01 | 完整M3证书及实时检查待工程证明。OBS09 admission/hash/ready字段不自动等于创建完成 | C05/C09；B05 | M3 certificate绑定exact Definition、startup hash、资源/配置版本、检查合同版本和有效性；无Run；漂移会失效 |
| P02 | 有界自动激活与手动竞争待工程证明；既定D1无需重新决议 | C09/C15；B05/B07 | scope/hash作用域、期限/预算/次数/撤销、首次激活唯一性；进程恢复和旧请求不再起Run |
| P03 | 改约前半准备链已有：Decision版本化proposal、Workspace独立派生六审、exact final candidate反证。当前应用入口仍无批准/应用/HTTP全链且拒有历史Run范围；exact approve→adoption→新Definition→新Run未完成 | C05/C12；B06 | Decision精确approve→Workspace持久应用journal/after-image→新Definition→Run-owned新lineage；只批准问题不等于批准最终candidate |
| P04 | cross-Run unknown barrier及普通Restart边界待全链证明 | C12/C13；B06/B08 | 旧Run在途/unknown、已发生effect与预算不被新Run冲掉；普通Restart可sameDefinition且无需虚构合同变更ticket |

新增行政收尾和late receipt/usage的具体目标schema、写域、版本与兼容只在[运行期目标增量合同](contracts/运行期目标增量合同.md)维护；它们不计入当前76个RT事件或307个source topic，不能因写入文档而关闭P04/X02。

S1–S6已经按本次实际接口/目录变化重新分级；旧缺陷只保留为回归反例，不代表当前仍具有同一缺陷。F1/F2/X01–X04及P01–P04在当前候选仍不可据文件数量、测试函数存在或样例fixture授PASS。历史CI [37444269463](https://github.com/muidea/workorch/actions/runs/37444269463)属于旧SHA，当前既有CI失败见§1.4，二者不混用。

### 5.1 生命周期需要一体化方案

C04/B03以[组件具体合同目录](contracts/组件具体合同目录.md)的CF00及[组件迁移索引](components/00_迁移总索引与集成顺序.md)为实施依据。必须核对真实framework可调用的阶段；不能声明未被调用的TeardownChecked就宣称传播完整，不能在owner尚未取消时只添加Quiescer而制造等待死锁。此处仅登记横切依赖与停止条件，生命周期协议正文不重复维护。

### 5.2 迁移和回滚不是最后补文档

最新增量已改变authoring ledger（mapping/success audit/completion identity）、Runtime terminal material seal、Result raw持久化与ACK身份，以及lineage profile合同。C12应基于这些已有格式核对迁移、版本拒绝和恢复，不能仅把新增字段视为无风险兼容；旧记录缺少当前mapping/audit/seal时的实际拒绝与处置须逐项列明。

每个持久化合同变更同项提交读写、版本拒绝、旧Run处理、恢复和投影方案。缺少新证明映射的旧Run标legacy/unverified，不用新模板伪造旧历史。恢复快照前必须对账之后的外部副作用、receipt、unknown与预算，禁止“回滚账本→重复执行”。不能安全回滚时明确前向修复或隔离恢复。

### 5.3 验收证据须先设计后采集

C16的可执行case、fixture、独立oracle、producer校准、构建身份和实际槽位登记统一见[验收规格](validation/验收规格.md)与[固定验收Fixtures](validation/固定验收Fixtures.md)。本总纲只维护先后依赖：准备和校准先于对应live取证；C17采集后C18聚合。诊断failpoint与发布构建不可混身份，mock不可代替real-provider。未来执行另需环境、费用、秘密、副作用、受控重启和停止预算授权。

## 6. 新增或修订方案的最小记录

已有完整合同直接修订其权威文件，不再复制空模板。若确需新增C主题或B子批方案，必须实填下列记录；未填项保持设计阻塞，不能用“按模板补充”充当本轮交付。

1. 身份：文档版本、固定代码/规范SHA、R/C/B、适用目标/非目标、前批基线与已知失败
2. 业务边界：目标、不变量、用户结果、最终与阶段范围；引用唯一模型/状态/合同owner
3. 当前证据：已实现/静态风险/未审范围分别记录；最新gap仅链接§5，增量变化才更新台账
4. 设计差异：输入输出、前后状态、guard、CAS/fence/幂等、UNKNOWN、权限/预算、响应丢失与失败恢复
5. 双端影响：producer/consumer、文件/包/叶子、持久格式、装配及复用分类；接口两端不留不一致窗口
6. 迁移与回退：版本拒绝、旧Run/数据、在途effect与预算、单写切换；不适用必须说明原因
7. 独立验收：实际case ID、fixture、操作、唯一权威oracle、正反例/故障切点、证据身份和SG层级；用[验收规格](validation/验收规格.md)实填case而非另建同义case
8. 门禁：引用G0–G4，固定本批累计回归范围、required判据、失败停止和重开规则；不能事后降级
9. 授权与结论：四轴真实状态、用户已批产品决定和真正新决策、获准执行范围；方案不代替实施/测试/live授权
10. 修订记录：何种差异、理由、受影响引用及验证结果；若目标/验收/约束改变才遵守D2 ticket流程

## 7. 如何逐项使用

1. 先审C01：确认最终能力含义和分期，不要求同时批准任何代码调整或live执行
2. 明确C02总体权威及当前批次所需决议/接口，展开受影响的C03单元和最小详细方案；以B01等能力批次推进，不等待18项全部展开，也不按22个组件逐个重写
3. 每项先审目标和不变量，再审当前代码证据、方案、迁移、反例和退出条件；争议记录为决议，不循环改代码猜测需求
4. 同一变更影响多个项时，登记依赖并联动评审；不能因主项通过而忽略其他required项
5. 待用户另行授权实施后，才能进入代码修改；方案确认本身不等于执行授权。本阶段不会启动测试或live
6. 获准实施后每次只推进一批，完成本批集成和此前能力累计回归，证据固定后才进入下一批；失败先停止/修复/复验。通过小目标后继续约定完整Goal，最终由C18对全部required项目作一致性复核
7. 新证据推翻旧假设、规范版本改变、required目标变化或共享合同变更时，重开受影响项；保留历史结论和失败证据，不覆写为“从未失败”

## 8. 修订记录

| 版本 | 日期 | 内容 | 未完成边界 |
|---|---|---|---|
| v1.4 | 2026-10-12 | 固定5360caf与120提交完整tree差异；更新Workspace Module、checked lifecycle、source inventory、terminal依赖、改约准备及当前CI失败；保留目标/验收和稳定R/C/B编号 | 仅设计刷新；源码进展、静态CI结果和未运行验证分开 |
| v1.3 | 2026-10-10 | 建立唯一权威入口、当前事实/差距台账、B01–B11依赖；完整保留55R和18C，组件细节转权威引用；明确普通G4按SG1/SG2、最终Web/产品SG3；分开设计已填与工程/验证状态 | 仅Markdown；工程合同为冻结候选，不改代码、不运行或监控测试/CI/live；现有CI失败不变 |
| v1.2 | 2026-10-09 | 固定新SHA并区分13提交/266文件增量、已有实现、剩余缺口与现有CI失败；同步M3/D1/D2及决议分层；B01避免重做mapping/长度修复；保留55R、18C、22单元和G0–G4 | 只读与文档更新；不修改项目代码、不运行测试/CI/live；最新CI失败，不授整体PASS |
| v1.1 | 2026-10-08 | 区分18个评审主题与小批次实施；增加G0–G4集成/累计回归门禁、B01建议及公开live阻塞边界；确立现有代码参考地位与三类复用；补充迁移/例外/重开规则 | 仅方案修订；B01范围待细化评审及相关决议；无代码调整、无项目测试或live结果 |
| v1.0 | 2026-10-08 | 将既有全面评审组织为C01–C18；保留55项和22单元；建立统一模板并详细展开C01；已交叉核对依赖、全目标承接及22单元调整归因 | 产品决议待确认；C02–C18详细方案待逐项展开；无代码调整、无项目测试或live结果 |

**本阶段结论：** 可以先明确目标语义与架构权威，随后按最小能力批次逐一收口、集成和回归，不等待所有详细方案完成才规划第一批。现有代码按适配性复用，不作为强制保留的架构约束，也不默认全部重写。最终是否完全符合项目设计，必须由后续逐项实现证据和最终完整Goal验证决定。
