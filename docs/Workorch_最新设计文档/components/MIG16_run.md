# MIG16 Run 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | RunID、SM-1、replacement intent、opaque CAS及raw ACK |
| 本轮剩余适配/验收（已有实现见§0） | 已修owner/账本/等待回归、首次激活与唯一后继恢复核验 |
| 移除或禁止继续依赖 | Run解释SM-2/3/4；提前清topics或初始化失败丢owner |

- 批次归属：参与C04/C09/C11/C12；按能力需要参与B03/B05/B06/B07/B08/B09，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：Run持久记录与sealed bootstrap合法初始化先于SM-1激活；Orchestrator可在收到active事实后接纳/初始化自己的aggregate。在精确Runtime接纳前禁止drive/provider/effect，界面只显示派生的“启动中”；中断恢复原Run，不把active等同可执行
- 升级/回退重点：Run/ bootstrap/opaque版本分开；旧二进制不能降写不认识aggregate或丢handoff fence。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-C](../validation/固定验收Fixtures.md#fx-c)、[FX-R](../validation/固定验收Fixtures.md#fx-r)、[FX-T](../validation/固定验收Fixtures.md#fx-t)、[FX-L](../validation/固定验收Fixtures.md#fx-l)；[VS-V27](../validation/验收规格.md#vs-v27), [VS-V29](../validation/验收规格.md#vs-v29), [VS-V30](../validation/验收规格.md#vs-v30), [VS-V31](../validation/验收规格.md#vs-v31), [VS-V35](../validation/验收规格.md#vs-v35), [VS-V39](../validation/验收规格.md#vs-v39), [VS-V41](../validation/验收规格.md#vs-v41)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：Workspace→Run意图 | [CT-run-RunCommandWorkspaceExecutionIntent](../contracts/run.md#ct-run-runcommandworkspaceexecutionintent) | 复用既有意图入口；首激活跨手动/自动永久幂等是目标补齐 |
| 入边：Orchestrator→Run存储/终态 | [CT-run-RunCommandStoreRuntimeStateRecord](../contracts/run.md#ct-run-runcommandstoreruntimestaterecord)、[CT-run-RunFactTerminalDelivery](../contracts/run.md#ct-run-runfactterminaldelivery) | 只按外层合同存opaque；三方材料与SM-1 CAS版本分开 |
| 出边：Run→Workspace/运行消费者 | [CT-workspace-WorkspaceCommandGet](../contracts/workspace.md#ct-workspace-workspacecommandget)、[CT-run-RunRuntimeCommandsReady](../contracts/run.md#ct-run-runruntimecommandsready)、[CT-run-RunStateChanged](../contracts/run.md#ct-run-runstatechanged) | 查询正式Workspace事实并通报权威Run变化；不凭通知驱动重复初始化 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[run具体合同页](../contracts/run.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

Configure在订阅/recovery前保存owner；subscribeOwnerTopic逐项记账且不持commandMu调用Hub；Stop使用context两次Quiesce并成功取消后才移除topic。旧丢owner/无界Wait/提前清空问题已有实现修正。

保留RunID/CAS/opaque aggregate及首次激活身份，验证部分Configure、退出重试、启动初始化和唯一后继；沿同一seal/ACK恢复不重编proof。

当前路径：`internal/modules/blocks/run`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/module.go#L1-L69)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/common/common.go#L1-L3)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner_lifecycle.go#L1-L97)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/biz/owner.go#L1-L513)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 现状与目标

Run独占SM-1、冻结输入、恢复及唯一后继；存储Orchestrator的opaque aggregate/CAS，不解释SM-2/3/4或credit。恢复沿原Run；合同变更经ticket→最终候选approve→新定义→新Run，不热换合同。

M3创建完成时Run为0。默认用户点“激活执行”；明确授权自动模式也须先M3后独立M4请求。Run不得从合同确认、审查PASS或Approval记录自行推断执行授权。

## 2. 已查证代码与复用

Configure在订阅/recovery前保存owner；subscribeOwnerTopic逐项记账且不持commandMu调用Hub；Stop使用context两次Quiesce并成功取消后才移除topic。旧丢owner/无界Wait/提前清空问题已有实现修正。

复用原RunID、StateRevision CAS、replacement intent和opaque存储；继续验证数据原子性、SM-1合法转移与恢复，不新增臆造状态名。

### 2.1 最新终态持久化增量（已静态核对，未运行）

Run仍为SM-1唯一owner，并持久保存Orchestrator opaque aggregate。新aggregate含immutable TerminalMaterialSeal、GenerationStateRevision、完整材料/hash及当时recovery快照；不允许Run重编材料或解释proof。SM-4[先CAS封存再投递](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime/terminal_material_seal.go)，由[Result→Workspace→Run](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/orchestrator/biz/runtime_result_projection.go)携带真实ResultID；Run ACK通过[CloneRaw](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/run/pkg/events/lifecycle_messages.go)保持完整事实原值。Result body只由Result ACK提供，Workspace/Run ACK不得伪装材料producer。

[seal validator](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/pkg/workorchruntimecontract/runtime_terminal_material_seal.go)允许终态未seal且零ACK的待交付状态，拒绝有ACK无seal；生成revision与后续recovery/ACK revision分开。0/1/2 ACK未完成，完整ordered ACK只证明交付阶段，不替代前置义务验收。复用这些机制后验证恢复、格式拒绝及slot handoff，不重复设计第二封存系统。

<a id="first-activation-intent"></a>
## 3. typed边界与幂等身份

以下是拟收口动作语义，不声称当前API/topic具有这些名称。

| 交互 | 输入/检查 | 输出及权威 |
|---|---|---|
| 激活Run | Workspace、精确定义/启动hash、独立request、执行意图、slot generation/fence、当前readiness | 同首次activation-intent永久关联原Run；即便其已结束，手动/自动迟到重试仍不新建；同key异内容冲突 |
| 读写runtime | RunID、expected aggregate revision、opaque payload及约定完整性信息 | CAS回执；不解析内部证明或替Orchestrator改字段 |
| 显式后继 | 前驱、replacement身份、新定义及安全交接回执 | 唯一后继；无前驱则首个Run，不伪造历史 |
| 终态提交 | Result exact ack、Workspace slot handoff ack、closeout绑定、expected SM-1 revision | 合法终态CAS；记录同一ResultID |
| 崩溃恢复 | 原Run/recovery身份、冻结输入及持久aggregate | 恢复原Run；非审计重放，不创建新Run |

Workspace拥有slot，Orchestrator拥有运行语义；通过owner的pkg/events交互，不共享repository或可变句柄。

## 4. 最小迁移及多owner阶段

1. 先回归已修局部生命周期：初始化前保存owner；逐项订阅记账；可panic调用不持不可恢复锁；取消后context等待，未退出保留资源及清理清单
2. 与全局生命周期协议共同接线，owner真实退出、共享drain后才final release；不单改Stop签名便认定解决
3. 单独收口Start幂等/唯一活动与初始化恢复；Run创建后断线查原请求；Run侧bootstrap未合法初始化不得激活，Orchestrator尚未接纳时不得以SM-1 active伪称已可执行，更不得换request重建
4. 再接D2后继：先封存新定义、阻止不兼容新调度、对账在途效果并合法交接；旧Run无成功证明不得为腾slot伪造成功
5. 复用最新seal/有序ACK，与Result、Workspace、Orchestrator验证残余终态屏障和恢复：Result ack→slot ack（保留handoff fence）→Run终态→同身份收尾屏障。分阶段持久恢复，非跨owner原子事务

破坏性typed变更与所有调用方同批集成；每次验收一个完整闭环，禁止不安全双写。

## 5. 版本与安全回退

Run格式、bootstrap、opaque aggregate格式分别标记版本；Run只能检查已约定外层完整性，不把新版本aggregate重写成自身理解的旧对象。未知格式拒绝写入或交给支持版本处理；历史冻结输入不跟随模板/Profile更新。

保留前驱、新Run、replacement identity、unknown操作、预算预留及真实成本关联；新Run不得继承旧credit或清零未结责任。回退前核对已获slot、Run创建和外部效果，不能恢复旧数据库后重复执行。旧二进制不能读新格式或保持fence时禁止直接回退，选择保留读取/阻塞新启动并前向修复。

## 6. 逐项验收设计（未执行）

| 编号 | 场景 | 预期与证据 |
|---|---|---|
| MIG16-T01 | 双Start同key、不同key并发；首Run结束后自动/手动迟到 | 同activation-intent返回原Run，不因终态清掉去重；不同key不能双活动 |
| MIG16-T02 | M3达到但无激活意图 | Run数0；确认不触发Start |
| MIG16-T03 | Run创建后响应丢失/初始化崩溃 | 原身份恢复，无第二Run；真实阶段可查 |
| MIG16-T04 | 第N次订阅失败或panic | 已获订阅可清理、锁可重入、owner仍可达 |
| MIG16-T05 | 取消订阅失败后重试/worker不退出 | 保留未完成topics和依赖；ctx到期返回未完成 |
| MIG16-T06 | 两个aggregate writer使用同revision | 仅一个CAS成功；Run不合并内部语义 |
| MIG16-T07 | approve后候选被改或新定义未封存 | 不创建后继，返回精确绑定阻塞 |
| MIG16-T08 | 前驱unknown操作且后继请求到达 | 保留效果/预算责任，未经安全交接不双活动 |
| MIG16-T09 | slot已ack而终态提交失败 | fence仍有效；查询重入原交接，不放行新Start |
| MIG16-T10 | 旧Run迟到终态/恢复请求 | 不改后继slot，不复活终态Run |

## 本组件集成证据补充

真实三方Start/终态与至少一个恢复切点；SM-1、aggregate CAS、slot版本分别留证，不混称一个revision。

新增未运行用例：opaque记录raw clone不改原诊断/nil/empty；ACK缺body/材料漂移；seal生成后recovery revision推进仍复用原材料；首Run已终态的activation-intent迟到重试保持原Run，不借新终态机制重开执行。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
