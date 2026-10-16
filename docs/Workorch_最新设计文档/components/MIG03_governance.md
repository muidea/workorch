# MIG03 governance组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Module → Module；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | 规则计算、Decision持久身份和typed提交 |
| 本轮剩余适配/验收（已有实现见§0） | 现有逐订阅/锁外取消/等待回归、定义作用域ticket剩余协调 |
| 移除或禁止继续依赖 | 2秒即释放与平行proposal/acceptance账本 |

- 批次归属：参与C04/C08/C12/C13；按能力需要参与B03/B06/B08，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：先完成局部退出可重试；变更协调只在Decision/Workspace两端支持精确候选时启用
- 升级/回退重点：旧proposal字段缺失不能默认扩大批准范围；回退不把新合同变更降级为普通replan。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-L](../validation/固定验收Fixtures.md#fx-l)、[FX-C](../validation/固定验收Fixtures.md#fx-c)、[FX-R](../validation/固定验收Fixtures.md#fx-r)；[VS-V35](../validation/验收规格.md#vs-v35), [VS-V28](../validation/验收规格.md#vs-v28), [VS-V29](../validation/验收规格.md#vs-v29), [VS-V13](../validation/验收规格.md#vs-v13)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：Decision/Run通知→Governance | [CT-decision-TicketCreated](../contracts/decision.md#ct-decision-ticketcreated)、[CT-run-RunStateChanged](../contracts/run.md#ct-run-runstatechanged) | 通知触发规则计算，不传递采纳权威 |
| 出边：Governance→Decision/Approval读取 | [CT-decision-DecisionCommandGetTicket](../contracts/decision.md#ct-decision-decisioncommandgetticket)、[CT-approval-ApprovalCommandGet](../contracts/approval.md#ct-approval-approvalcommandget) | 从真实owner取得版本与范围；决议写入路径按具体目录核对 |
| 出边：Governance通知/读取 | [CT-governance-GovernanceAutoResolved](../contracts/governance.md#ct-governance-governanceautoresolved)、[CT-governance-GovernanceAutoCommandListTraces](../contracts/governance.md#ct-governance-governanceautocommandlisttraces) | 保留治理事实与trace；目标exact ticket变更不伪装已实施 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[governance具体合同页](../contracts/governance.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。


## 0. 2026-10-12固定SHA重核与剩余增量

Start逐成功订阅登记；BeginShutdown锁外取消；Quiesce等待真实worker/reconcile完成；StopChecked锁外退订且成功后才出栈。旧固定2秒清空依赖问题不再作为当前未修项。ContractChangeDecisionKind被自动规则排除。

复用Service和Decision持久身份，局部集成验证失败退订可重试及ContractChange不得自动批准；精确变更以Decision/Workspace实际合同推进。

当前路径：`internal/modules/kernel/governance`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/module.go#L1-L81)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/pkg/common/common.go#L1-L3)；[biz/service.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/governance/biz/service.go#L1-L418)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 当前与目标责任

保持kernel Module，路径 internal/modules/kernel/governance。负责DM26–28的规则计算、typed决议提交及治理协调；Decision独占持久ticket/resolution/runtime governance proposal生命周期，Orchestrator独占运行语义采纳。Governance不能新增与二者平行的proposal或acceptance账本。

已查证：逐订阅登记、锁外退订及context真实等待已经实现，详见§0；生产typed交互保留，不据私有ports推定跨owner旁路。

## 2. 复用与路径影响

直接复用规则计算及生产typed交互适配，保留现有ticket和proposal身份。适配 internal/modules/kernel/governance/biz/service.go的Start/Stop、订阅登记、锁纪律及错误传播；module/plugin桥和全部Base任务调用方随合同检查。决议具体payload字段先对照Decision owner公开合同，不通过反射/JSON通用事件绕过类型。

不将全量Governance重写成工作流引擎；不因为目标变更新增独立ChangeTicket组件。需要持久化的新事实优先归实际owner，纯计算中间值不成为正式状态。

## 3. 生产者/消费者合同

| 交互 | owner关系 | 校验责任 |
|---|---|---|
| ticket/提案读取 | Governance→Decision | Workspace/可选Run、ticket/proposal版本与当前状态，禁止读取他方repo |
| 策略计算→决议提交 | Governance→Decision | 原基线、精确候选hash、动作范围、完整理由及权限来源；冲突/旧版本显式拒绝 |
| 风险操作许可 | Governance→Approval | 精确operation/payload/scope；合同approve不自动替代操作许可 |
| 运行期采纳 | Decision决议→Orchestrator | matching binding及acceptance由Orchestrator核对，resolved不等于已应用 |
| 定义变更物化 | 决议→Workspace既定协议 | 修改目标/验收/约束须ticket；最终候选批准后才可物化，再由新Run执行 |

以上为目标语义，不虚构现有事件常量。各owner自己的pkg/events拥有输入/输出及错误合同，Governance不在公共包发布别人的别名权威。

## 4. 分批迁移与依赖

1. 先整理现有规则输入、Decision durable proposal和Orchestrator采纳之间的责任，保留当前合法typed路径
2. 验证现有逐订阅获取记账；不要持生命周期锁执行可能panic的外部Hub动作；失败项保留到实际取消成功
3. 复核已替代固定2秒等待的可报告未完成停止协议，保留worker和仓库引用；与模块桥、Base及全局屏障协调
4. 与Decision、Workspace、workorchd成组补充无Run定义作用域的合同变更ticket；不伪造RunID
5. 再与Run/Orchestrator完成运行中变更：停止/对账在途副作用、精确物化新定义、唯一后继新Run。普通replan保持原Run和冻结目标

不得先把所有规则计算结果直接写成“批准”，再补权限；不得以本组件停止成功宣告共享任务/Hub已全局排空。

## 5. 数据兼容与恢复/回退

Governance不新增第二份持久治理日志。复用Decision身份及既有open/resolved/superseded状态，阶段进度从权威事实派生；新增语义字段采取显式schema版本和缺省拒绝，不能默认旧proposal批准新范围。

同一提案重复计算可重复提交同一意图，Decision保证幂等/冲突；提交后响应丢失先查询原提案。候选内容变化使原approve失效；runtime-bound分支须先取得matching acceptance，再由Decision提交formal resolution/ACK；该ACK仍不等于Workspace adoption/物化已完成。精确顺序引用[NC04](../contracts/创建与定义变更合同.md#nc04)。回退时保留所有已批准/已应用记录和在途effect；旧版无法理解新提案则暂停该类写入，不能将其降级成普通replan或删除后重建。

## 6. 验收用例（拟定，未运行）

1. 精确候选和有效权限下决议提交成功，读取Decision可见同一身份
2. 候选A批准后修改为B，B不可复用A批准，即使ticketID相同
3. 无Run的定义变更可发ticket，流程全程不创建虚假Run
4. runtime matching已成立且Decision formal ACK已提交，但Workspace adoption/物化尚未完成时，如实显示后续未完成阶段；不能激活旧定义冒充新结果
5. 高风险操作有合同approve但无operation Approval，操作仍不得执行
6. 第二订阅失败后第一订阅被准确登记和清理；重试无重复处理器
7. 取消订阅抛错/超时后锁可再次获取，待清理项保留，下一次Stop继续
8. worker超过2秒未退出时返回未完成且保留依赖，退出后才允许释放
9. 已发生远端effect但决议响应丢失，恢复不重置预算、不重新无条件执行
10. 正常不削弱承诺的replan不被误强制走合同变更；目标改变则必须走ticket及新Run

## 8. 历史固定版本证据（5554296）

- [生产typed ports](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/governance/biz/service.go#L105-L124)
- [订阅与停止](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/kernel/governance/biz/service.go#L169-L288)
- [共享任务错误API](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/base/biz/base.go#L119-L135)
- [进程释放顺序](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/vendor/github.com/muidea/magicCommon/framework/application/application.go#L270-L313)

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
