# MIG15 Result 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | ResultID、raw closeout、单compiler对应typed adapter与持久ACK |
| 本轮剩余适配/验收（已有实现见§0） | 上游语义修正的兼容读取及精确交接/历史读取验证 |
| 移除或禁止继续依赖 | Result重裁credit、重建seal或以文件存在标Run完成 |

- 批次归属：参与C10/C11/C12；按能力需要参与B07/B08/B11，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：保存子链可独立验收；slot/SM-1未闭合只显示交接中，不开放第二活动资格
- 升级/回退重点：保留已发ResultID及slot/Run回执；原artifact变化真实披露，不能删结果重来。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-T](../validation/固定验收Fixtures.md#fx-t)、[FX-O](../validation/固定验收Fixtures.md#fx-o)；[VS-V10](../validation/验收规格.md#vs-v10), [VS-V20](../validation/验收规格.md#vs-v20), [VS-V30](../validation/验收规格.md#vs-v30), [VS-V31](../validation/验收规格.md#vs-v31), [VS-V46](../validation/验收规格.md#vs-v46)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：Orchestrator→Result | [CT-result-ResultFactTerminalDelivery](../contracts/result.md#ct-result-resultfactterminaldelivery) | 原值材料经校验持久化，返回actual ResultID及closeout |
| 入边/返回边：应用交付读取 | [CT-result-ResultCommandListByRun](../contracts/result.md#ct-result-resultcommandlistbyrun)、[CT-result-ArtifactCommandReadContent](../contracts/result.md#ct-result-artifactcommandreadcontent) | 交付引用与真实内容核对；不存在时不伪造原版本 |
| 出边：Result通知/ACK | [CT-result-ResultCreated](../contracts/result.md#ct-result-resultcreated) | ACK是TerminalDelivery的typed Result，不能重裁closure或自行释放slot |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[result具体合同页](../contracts/result.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

Module仅管理Biz生命周期，新增owner_lifecycle保留终态收尾通道。raw seal/closeout保存和上游唯一compiler分工继续成立。

复用精确材料/有序ACK，核验上游SourceObligation结果及当前证据集合的完整保存、冲突、恢复和slot交接；不另写Result裁决器。

当前路径：`internal/modules/blocks/result`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/module.go#L1-L52)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/common/common.go#L1-L28)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner_lifecycle.go#L1-L68)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/owner.go#L1-L490)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

EvidenceItems的current-binding/lineage过滤复用[Orchestrator迁移说明](MIG04_orchestrator.md)所列共享compiler；Result只保存封存材料，不重新裁决。OwnerRevisionSet剩余来源依赖按[总纲X02](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)统一登记。

## 1. 现状与目标

Result继续保存正式结果、交付引用并提供读取。Orchestrator唯一裁定义务闭包、当前适用证据和closeout；Result不重新判定目标成功，不直接推进Run SM-1，不负责实际文件内容的所有权。

最新Result已采用owner-neutral终态材料的concrete typed适配，复用单一compiler；raw clone保持nil/empty、原始诊断及完整closeout。读取和保存校验原值，ACK携带owner实际持久化的ResultID及closeout，noop重放也返回保存材料。此为静态已实现，不能再列为从零新增任务。

残余当前证据适用性/真实producer依赖问题已迁到[共享compiler](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_result_compiler.go)，不是Result自判bug；当前coverage open时历史satisfied assessment已有降级保护。Result仍不重裁credit；修正生产端后验证精确保存、冲突、重载、交接及历史兼容。

## 2. 代码影响与复用

[internal/modules/blocks/result/biz/owner.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/result/biz/owner.go)、[internal/modules/blocks/result/module.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/result/module.go)、[internal/modules/blocks/result/plugin.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/result/plugin.go)：复用owner、存储与typed读取，G0定位实际保存/更新及序列化调用方。

[Result实际持久closeout ACK](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/biz/lifecycle_handler.go)、[shared material typed adapter](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/result/pkg/events/lifecycle_messages.go)、[raw seal validator](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_material_seal.go)：本轮重新读取最新关键路径。旧owner/module/plugin链接仅作历史入口，不代表这些文件本轮未改。

上游[终态投递](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_result_projection.go)已先封存后Result→Workspace→Run，有序ACK不等同已实测handoff屏障及所有故障恢复。

保留ResultID、Run关联、交付历史和原closeout；对新增proof/source版本信息进行显式兼容适配，不能把缺字段历史默认为当前证明齐全。

## 3. typed保存及终态合同

下述是目标合同及差距核对；最新已有terminal delivery typed事件、材料seal和有序ACK，不能以本表另造同义topic。

| 阶段/接收owner | 精确输入 | 回执及权限 |
|---|---|---|
| Result保存closeout | RunID、definition/hash、closeout identity/hash、固定结论、证据适用状态、artifact版本引用 | 幂等返回exact ResultID及实际持久closeout；与原seal比较，异内容拒绝；不重算closure |
| Workspace确认slot | 已存ResultID、closeout、当前Run及generation/fence | 持久slot交接ack并保持handoff屏障，不立即授新Start资格 |
| Run提交SM-1终态 | 前两者匹配ack、expected state revision、同一closeout/ResultID | 合法CAS终态；响应丢失可查询原结果 |
| Result读取/交付视图 | Workspace/Run/Result scope及访问主体 | 返回正式结果与真实交接进度；不凭结果存在称Run已结束 |

固定顺序：Result持久ack→Workspace slot ack→Run终态；随后同一交接身份核对终态事实并收尾准入屏障。Result不得绕过前置直接释放slot；屏障也不能等待自身前置形成循环。文件owner提供内容hash/位置事实，Result仅保存有归属的引用。

## 4. 小批迁移与断点恢复

1. 保留已实现identity/hash、单compiler、immutable seal及raw ACK；对残余证据语义字段变化制定显式兼容，Result与Orchestrator生产消费同批适配
2. 打通“保存→丢响应→原身份读回”的最小链；实际字节/字段与hash一致，不能重生成另一份结论以求重试成功
3. 在已有有序三owner ACK上，与Workspace、Run验证handoff fence及每个持久化后丢响应恢复；协议是一组阶段，不是假想跨owner事务
4. 最后接Web交付读取和历史结果展示；同Run派生视图可重建，但正式结论更正需要显式版本和来源，不覆盖原记录

前置为MIG04编排的正确closeout、MIG22活动slot以及MIG16合法生命周期；如果这些仍未收口，本组件只能验收保存子链，不能宣称端到端成功交付。

## 5. 存储、历史与回退

以schema version区分旧结果和新closeout绑定字段；保留历史accepted事实，同时明确当前适用/过时状态来源。只读适配不能给旧证据补造当前lineage。新写入启用前验证旧读者行为，必要时阻止旧版本读取为成功结论。

回退需保留已发ResultID、内容hash、已完成的slot/Run回执；不能删除结果再释放slot“从头来过”。数据库回滚前对账所有终态交接和现实副作用、unknown成本与预留；不安全时保留交接阻塞并前向修复。交付文件不存在或内容变更要真实披露，不能以旧路径存在性伪造原交付内容。

## 6. 逐项验收设计（未执行）

| 编号 | 场景 | 预期与证据 |
|---|---|---|
| MIG15-T01 | 同closeout同hash重复保存 | 一个ResultID，内容一致 |
| MIG15-T02 | 同closeout身份换结论/证据 | 冲突，原结论完整保留 |
| MIG15-T03 | Result提交成功后丢响应 | 查回原Result，继续同交接身份 |
| MIG15-T04 | slot已ack、Run未终态时新Start | handoff屏障拒绝第二活动资格 |
| MIG15-T05 | 旧Run迟到closeout/slot确认 | 不清理后继slot，不覆盖后继Result |
| MIG15-T06 | 上游包含过时accepted证据 | 端到端当前证明集合不采纳；历史仍可审计 |
| MIG15-T07 | Result存在但Run终态CAS失败 | 显示交接中/冲突，不把保存当Run完成 |
| MIG15-T08 | artifact同路径内容改变 | 精确hash差异可见，不冒用原交付版本 |
| MIG15-T09 | 历史记录缺新增绑定字段 | 标明历史/未核验，不补造PASS |

## 本组件集成证据补充

至少贯通保存、slot、SM-1三方及每个提交后丢响应切点；文件读取核验仅在获准环境执行。

新增未运行用例：Result ACK缺失closeout、body/hash漂移、nil/empty或重复集合被normalize隐藏、返回值别名修改、同Run旧版本材料重放，须拒绝漂移或保持原值；三owner ACK齐全才允许交付完成，seal存在本身不表示完成。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
