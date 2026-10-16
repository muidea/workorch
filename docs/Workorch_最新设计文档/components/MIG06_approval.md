# MIG06 Approval 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | 许可记录、原ApprovalID与typed入口 |
| 本轮剩余适配/验收（已有实现见§0） | 操作绑定有效性核验、reconciliation取消与可重试句柄 |
| 移除或禁止继续依赖 | 超时即清空cancel/done/repo；把批准当最终验收 |

- 批次归属：参与C04/C09/C12/C13；按能力需要参与B03/B07/B08，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：先回归已实现的worker退出保留，再启用要求新scope字段的调用；不能新许可写入后由旧执行端忽略限制
- 升级/回退重点：旧许可缺精确绑定保留历史只读，当前调用必须重新合法核验。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-L](../validation/固定验收Fixtures.md#fx-l)、[FX-O](../validation/固定验收Fixtures.md#fx-o)；[VS-V13](../validation/验收规格.md#vs-v13), [VS-V35](../validation/验收规格.md#vs-v35), [VS-V22](../validation/验收规格.md#vs-v22), [VS-V05](../validation/验收规格.md#vs-v05)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：运行/应用→Approval | [CT-approval-OpenRuntimeApprovalRequest](../contracts/approval.md#ct-approval-openruntimeapprovalrequest)、[CT-approval-ApprovalCommandEnsureApproved](../contracts/approval.md#ct-approval-approvalcommandensureapproved)、[CT-approval-ApprovalCommandEnsureRejected](../contracts/approval.md#ct-approval-approvalcommandensurerejected) | 绑定精确操作与当前条件，不等价于合同批准 |
| 出边：Approval→Orchestrator | [CT-orchestrator-OrchestratorCommitRuntimeGovernanceApproval](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommitruntimegovernanceapproval)、[CT-orchestrator-OrchestratorCommandRecordApprovalEvidence](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommandrecordapprovalevidence) | 保留决议提交/事实记录的区别；不得由此授目标credit |
| 出边：Approval事实通知 | [CT-approval-ApprovalResolved](../contracts/approval.md#ct-approval-approvalresolved) | 通知可审计，当前有效性仍通过owner核验 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[approval具体合同页](../contracts/approval.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

Biz现有逐成功订阅账本、BeginShutdown和context Quiesce；reconcileDone等待完成后才清引用，module仅Teardown成功后清Biz。旧2秒等待后释放不再是当前缺陷。

复用现许可聚合，验证超时后原cancel/done与repo仍可达、scope/撤销/过期与迟到receipt；不把合同变更approve当操作许可。

当前路径：`internal/modules/blocks/approval`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/module.go#L1-L81)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/pkg/common/common.go#L1-L9)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/approval/biz/owner.go#L1-L264)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 现状与目标

Approval继续拥有风险许可及其有效条件。Governance负责策略计算和协调，Orchestrator负责当前操作绑定与采纳，能力owner负责实际执行。approve不代表执行完成，不授予目标credit，也不能作为人工最终Verification或E5证明。

已核实生命周期修正见§0；许可范围、过期及撤销按既有目标继续核验，pending/approved/rejected/superseded沿用，不因迁移臆造枚举。

## 2. 代码影响与复用

[internal/modules/blocks/approval/biz/reconciliation.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/approval/biz/reconciliation.go#L66-L85)：改造取消、等待与可重试句柄保存。

[internal/modules/blocks/approval/biz/owner.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/approval/biz/owner.go#L107-L120)：依赖释放必须等待真实quiescence及共享drain。

[internal/modules/blocks/approval/module.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/approval/module.go) 与 [internal/modules/blocks/approval/plugin.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/approval/plugin.go)：核验错误上达和进程生命周期接线。

复用现有记录、repository、typed入口及Base；适配scope/版本/有效条件和结果回执；替换“超时即退出”的行为。所有许可创建、解析、执行前核验调用点在G0补全，不凭已检查根文件断言叶子审查完成。

## 3. typed合同与权威

以下是拟收口语义，不是已存在的topic/API名称。

| 交互 | 输入身份及接收owner判断 | 回执与禁止事项 |
|---|---|---|
| 请求许可 | Approval核对Workspace、可选Run、operation身份、scope、payload hash、主体和request identity | 返回原ApprovalID及精确版本；同key不同payload拒绝 |
| 提交许可决定 | Approval核对审批主体、expected revision、精确操作与决定 | 保存批准/拒绝事实；不得顺便修改合同或创建Run |
| 执行前核验 | Orchestrator与执行owner经Approval合同取得当前适用许可；核对operation/fence及动态条件 | 旧批准不可解锁新operation；到期等无法判断则阻塞 |
| 删除/退出 | Workspace请求本owner清理；进程请求取消和排空 | 清理只限自有记录；仍有在途核验时不销毁依赖 |

跨owner消息及返回值校验遵循[唯一合同规范](../Workorch_组件固定合同与分层验收规范.md#generic-contract)。本组件只处理精确操作许可，不把文档获准、LLM建议或投影当作业务授权。

## 4. 小批迁移及恢复阶段

1. 先固定许可身份、内容绑定和执行前复查判据；涉及合同的生产者/消费者在同一可编译提交中适配，不能只加字段不核验
2. 单独回归已实现的worker取消→实际退出→释放局部闭环：超时返回未完成，保留done、repo和重试入口；重复Stop沿原句柄等待
3. 与进程壳及其他owner一起核验现有全局quiescence协议；不能只暴露Quiescer却把取消留在Teardown
4. 以一次受控操作串起请求、批准、封存操作、执行前核验和receipt；批准入库与实际执行不承诺跨owner数据库原子性，丢响应先查询原身份

依赖MIG03治理、MIG04编排及实际能力owner；共同生命周期接线是完整协议，小批提交可以分步准备，但不把中间混合态宣称全局安全。

## 5. 持久化、旧记录与回退

记录新增绑定信息应带schema version并区分缺失与空值；历史许可缺少必要绑定时保留只读历史，不推导为全scope许可。迁移保留ApprovalID、原决定、主体、时间和关联操作。新消费者不能用旧读者忽略的新限制继续执行；无法安全降级时停止相关新请求，保持新版本读取或前向修复。

许可失效不等于现实操作被取消。回退前必须对账在途/unknown效果、迟到receipt和预算；不得删除许可或重置计费以宣称从未执行。存储快照不能覆盖已经发生的外部效果。

## 6. 逐项验收设计（未执行）

| 编号 | 输入/故障 | 唯一预期及可观测证据 |
|---|---|---|
| MIG06-T01 | 同请求、同hash重复请求 | 单一ApprovalID；读回原事实 |
| MIG06-T02 | 相同key改变资源或payload | 明确冲突；原许可不变 |
| MIG06-T03 | 批准A后请求A+B | 执行前拒绝；无B执行receipt |
| MIG06-T04 | 批准后条件变化/旧generation迟到 | 当前适用性不成立，不发新dispatch |
| MIG06-T05 | worker阻塞超过2秒 | Stop未完成、句柄及repo仍可用；无虚假退出 |
| MIG06-T06 | 解除阻塞并重试Stop | 原worker真实退出后释放一次，无重复worker |
| MIG06-T07 | 批准写入后响应丢失 | 原身份查询恢复，不补发无限许可 |
| MIG06-T08 | 把approved当human-verification输入 | 不产生义务满足或Goal成功 |
| MIG06-T09 | 旧格式记录无操作绑定 | 历史可读，当前执行拒绝或补合法核验 |

## 本组件集成证据补充

至少贯通一次允许操作与一次越scope拒绝，并捕获长worker取消/重试证据。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
