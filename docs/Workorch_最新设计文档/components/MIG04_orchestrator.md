# MIG04 orchestrator组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Module → Module；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | RuntimeAggregate、语义图、单compiler/seal/有序ACK与lineage guard |
| 本轮剩余适配/验收（已有实现见§0） | 源义务完整追踪、真实owner依赖适用性、实际producer剩余消费闭环和Stop传播回归 |
| 移除或禁止继续依赖 | 第二Plan/closure/compiler；用guard替代普通proof或绕过公开readiness |

- 批次归属：参与C04/C06/C08/C09/C10/C11/C12/C16；按能力需要参与B02/B03/B06/B07/B08，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：可先验证纯编译/持久材料子链；端到端执行开放前同时具备effect恢复与正确closeout，不先发布危险半链
- 升级/回退重点：新映射不能回填旧PASS；opaque schema及seal升级必须与Run/Result/Workspace同批兼容。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-G](../validation/固定验收Fixtures.md#fx-g)、[FX-R](../validation/固定验收Fixtures.md#fx-r)、[FX-O](../validation/固定验收Fixtures.md#fx-o)、[FX-T](../validation/固定验收Fixtures.md#fx-t)、[FX-L](../validation/固定验收Fixtures.md#fx-l)；[VS-V44](../validation/验收规格.md#vs-v44), [VS-V23](../validation/验收规格.md#vs-v23), [VS-V24](../validation/验收规格.md#vs-v24), [VS-V25](../validation/验收规格.md#vs-v25), [VS-V30](../validation/验收规格.md#vs-v30), [VS-V31](../validation/验收规格.md#vs-v31), [VS-V46](../validation/验收规格.md#vs-v46), [VS-V35](../validation/验收规格.md#vs-v35)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：驱动/receipt→Orchestrator | [CT-orchestrator-OrchestratorDriveRuntime](../contracts/orchestrator.md#ct-orchestrator-orchestratordriveruntime)、[CT-orchestrator-OrchestratorCommitRuntimeOperationReceipt](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommitruntimeoperationreceipt)、[CT-orchestrator-OrchestratorRequestRuntimeTermination](../contracts/orchestrator.md#ct-orchestrator-orchestratorrequestruntimetermination) | 运行权威仍在本owner；仅精确绑定事实可被采纳 |
| 出边：Orchestrator→Run opaque存储 | [CT-run-RunCommandRuntimeStateRecord](../contracts/run.md#ct-run-runcommandruntimestaterecord)、[CT-run-RunCommandStoreRuntimeStateRecord](../contracts/run.md#ct-run-runcommandstoreruntimestaterecord) | 聚合CAS与SM-1分离，Run不解释proof |
| 出边：Orchestrator→执行owner | [CT-file-ExecuteFileWriteOperation](../contracts/file.md#ct-file-executefilewriteoperation)、[CT-shell-ExecuteOperation](../contracts/shell.md#ct-shell-executeoperation)、[CT-mcp-ExecuteOperation](../contracts/mcp.md#ct-mcp-executeoperation) | 按所选能力扩展全目录；公开producer/readiness接线逐组合核验，不因topic存在声明可用 |
| 出边：Orchestrator→Result/Workspace/Run | [CT-result-ResultFactTerminalDelivery](../contracts/result.md#ct-result-resultfactterminaldelivery)、[CT-workspace-WorkspaceFactTerminalDelivery](../contracts/workspace.md#ct-workspace-workspacefactterminaldelivery)、[CT-run-RunFactTerminalDelivery](../contracts/run.md#ct-run-runfactterminaldelivery) | 复用既有有序ACK；当前证据适用性/真实owner依赖及handoff恢复同批核验 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[orchestrator具体合同页](../contracts/orchestrator.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

module Stop已传播Biz.Stop错误；owner_lifecycle区分fresh/drain并将异步任务计数保留至真实callback结束。source-obligation、terminal适用性及owner dependency相关增量必须按完整链验证，不能以旧Stop缺陷代替当前分析。

复用新的生命周期、源义务和seal/ACK；仍按总纲当前证据修正真实producer依赖及不足的公开消费边，精确核对current binding/lineage和proof适用性。

当前路径：`internal/modules/kernel/orchestrator`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/module.go#L1-L56)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/pkg/common/common.go#L1-L3)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner_lifecycle.go#L1-L126)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/owner.go#L1-L3764)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 当前与目标责任

保持kernel Module及现路径。作为DM19–25、DM31–35和SM-2/3/4唯一语义owner，负责分解、推进、副作用协议、证据适用性、逐义务闭包和正式closeout。Run只拥有SM-1并可存opaque aggregate；Result持久保存closeout但不重新裁决。

最新已实现（静态确认）：终态材料集中到[唯一共享纯compiler](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_result_compiler.go)，由SM-4在首次owner I/O前从独立持久前像生成并CAS封存[TerminalMaterialSeal](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime/terminal_material_seal.go)，随后重用原材料执行[Result→Workspace→Run](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_result_projection.go)。Result ACK含owner实际保存的ResultID及完整closeout；seal、ACK、重载采用raw clone/原值校验。此公共包只有纯合同/编译职责，不新增语义owner或第23运行组件。

当前EvidenceItems适用性已有实现：[accepted降级检查](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_result_compiler.go#L224-L254)调用[exact current binding与lineage/原dispatch反查](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_evidence_applicability.go#L3-L97)，不匹配转stale_or_superseded；旧“仅按CWI/ref展示历史Accepted”的子问题不得继续列作未修。独立剩余是[OwnerRevisionSet](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_result_compiler.go#L122-L136)仍以runtime_authority/accepted_evidence构造依赖，尚不能等价于真实File/Git等producer资源版本。此为来源依赖投影差距，不自动证明运行终态假通过。

新增[typed lineage assertions](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lineage_assertion_consumer.go)只作为普通proof后的AND guard：completed work rounds、真实owner receipt、repair后fresh validation；从封存source/declaration/CWI/完整文本hash重编，使用原evidence cutoff。post_terminal_delivery只由交付后的只读视图判断，不提前授credit、不新增SM。修复闭环[反查原拒绝、repair全部changed receipts、后续普通validation及exact input review](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_lineage_assertion_repair.go)，repair credit不能代替原完整目标证明。

生命周期Stop/插件已传播错误并具备fresh/drain接线，旧缺陷转为故障恢复回归项。源义务端到端完备性须与Workspace最新authoring映射和activation编译共同复核，不能因新增映射或guard便宣布全源义务闭包完成。公开implementation-work准入和真实live能力仍须逐组合验证，禁止删除failclosed开关求收口。

## 2. 复用与路径影响

- 直接复用RuntimeAggregate、现有semantic plan/CWI、适用性检查及Stop失败保留逻辑，不另建第二Plan图权威
- 适配 internal/modules/kernel/orchestrator/plugin.go、biz/owner.go：错误沿真实框架生命周期路径传播；不能新增框架不调用的TeardownChecked假装解决
- 复用 biz/runtime_result_projection.go的seal交付；残余修正定位 internal/pkg/workorchruntimecontract/runtime_terminal_result_compiler.go：保留已实现的当前适用性过滤，补真实producer依赖，不用runtime_authority合成取代来源
- 复用并深核 biz/internal/proof/evidence_lineage.go；与Workspace编译器、workorchd activationcore共同建立源义务映射
- 依赖 internal/pkg/workorchruntimecontract及默认模板策略，逐组合启用公开执行；不在本组件独自放宽定义政策

## 3. typed交互和核心不变量

| 方向 | 交接事实 |
|---|---|
| Workspace/workorchd启动输入→Run/Orchestrator | 不可变定义hash、源Obligation身份、策略、资源与scope；缺源义务拒绝初始化 |
| Orchestrator→Run | aggregate opaque保存/CAS及合法生命周期协调；Run不能重算证明 |
| Orchestrator→执行owner | sealed operation、ExecutionKey、payload hash、fence、预算/权限条件；owner返回真实receipt |
| Decision/Governance→Orchestrator | 精确决议及binding；由本owner决定acceptance/continuation，不以resolved直接授credit |
| Orchestrator→Result→Workspace→Run | 正式closeout持久确认、活动slot确认、SM-1合法终态；失败阶段可恢复 |

任务共享合法，但每个required源义务必须独立有闭包结果。Receipt成功不等于验收，Approval不等于验证，LLM意见不等于owner事实。旧Run可复用产物；新Run不得直接继承旧credit。真实已派发未知副作用保留原身份和预算预留，不无条件重试。

closeout顺序中的Workspace slot确认属于handoff屏障，不是立即允许新Start占用：须保留交接身份/fence，直至Run权威终态可核对才完成释放。若Result已保存、slot已确认而Run终态响应未知，恢复继续同一交接，禁止新旧双活动。

## 4. 按能力分批收口

1. 先与Workspace/workorchd固定义务→判据→CWI→证据/闭包追踪，构造A+B共享DoD仅证明A的独立反例
2. 收口Stop错误及owner quiescence，配合进程/全owner共享屏障；保留超时状态并可重试
3. 在确定支持的公开操作组合上完成一个真实工作闭环，与所选执行owner、Run和Result成组验证；同时准备独立oracle/证据生产者
4. 回归共享compiler已修当前适用性，修正残余真实依赖投影；复用已实现seal及有序ACK，验证Result持久→slot→SM-1每个断点恢复及slot屏障，不重建第二compiler
5. 增量扩展replan、ticket变更新Run、取消/unknown对账；不在前一批未通过时扩大工具面

上述步骤是关注点而非允许安全欠账的流水序号：步骤3若以完整交付/closeout作为退出，步骤4的当前证据适用性与P1交接必须前置或同批完成；首次dispatch前必须具备适用取消、unknown保留、effect身份/fence及预算对账基础。步骤5仅扩展更多治理/恢复场景，不能在第一条真实操作后才补这些底线。

工作量大不意味着一次实现所有DM；每批必须保留完整源义务和安全不变量，不能通过缩小已承诺目标使测试通过。

## 5. 版本、恢复与回退

aggregate schema、definition hash、semantic generation、binding revision和receipt身份各自记录，不能互相替代。新增映射采用明确迁移：旧记录无法证明完整源义务时保持未验证/阻塞，不回填假PASS。历史证据保留，用适用性区分stale/superseded。

崩溃恢复沿用原Run和effect身份；新定义批准后才走唯一后继新Run。回退前对账已派发/unknown effect与预算，保存最新ledger；禁止旧快照抹去副作用后重执行。若旧版本无法处理新aggregate/closeout，不启动旧写路径，优先前向修复或隔离只读恢复。

## 6. 验收用例（拟定，未运行）

1. 一个公开入口真实任务形成产物、receipt、逐义务证据、Result和合法SM-1终态
2. A+B共用工作但只证明A，B仍open，不能宣布全部完成
3. 原accepted证据依赖改变后失效：运行credit阻断且closeout不列为当前证明
4. 同一证据合法覆盖两义务时分别记录映射，不能强制重复执行浪费预算
5. Result保存后响应丢失，恢复读回同closeout；slot尚未确认时不提前终结Run
6. Stop超时错误传到真实进程生命周期，依赖/owner保留；重试完成后才释放
7. 外部成功本地超时保留unknown，查询/对账后续行，不换ExecutionKey盲重试
8. 最终候选approve后新Run使用新定义；旧Run合同及历史不改，旧credit不自动继承
9. 零测试命中、错误对象、unsupported grade或缺policy均failclosed
10. aggregate CAS冲突不覆盖新事实，累计回归验证旧已验收能力未倒退

补充故障例：closeout在slot确认后、Run终态前中断，同时发新Start，后者不得绕过handoff屏障；恢复后同closeout终态及slot释放可核对。

## 8. 历史固定版本证据（最新变化见第1节）

- [Stop失败保留](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/owner.go#L1085-L1129)
- [插件丢失错误](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/plugin.go#L55-L59)
- [closeout投影](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/runtime_result_projection.go#L318-L454)
- [真实lineage检查](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/orchestrator/biz/internal/proof/evidence_lineage.go#L65-L102)

新增拟验收反例（未运行）：seal与authority/recovery同CAS改写；初始化注入seal；有ACK无seal；缺失/漂移Result closeout；0/1/2 ACK；重试时recovery revision变化导致材料重编；guard通过但ordinary proof失败；future receipt补旧cutoff；repair后缺fresh普通验证，均不得通过相应门禁。


修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
