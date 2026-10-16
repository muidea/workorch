# MIG20：ssh 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | SSH target/session/credential引用、transport及receipt |
| 本轮剩余适配/验收（已有实现见§0） | 已实现逐订阅清理回归、session drain和远端效果对账 |
| 移除或禁止继续依赖 | 以断线/本地取消表示远端停止；换session/key重复副作用 |

- 批次归属：参与C04/C09/C12/C13；按能力需要参与B03/B07/B08/B10，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：只读远端观察先验收；获准可恢复副作用场景与真实权限到位后再开放写执行
- 升级/回退重点：冻结TargetRef/SessionRef/CredentialRef与凭据版本；旧Run未知效果及预算不能被后继或回退清除。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-L](../validation/固定验收Fixtures.md#fx-l)、[FX-O](../validation/固定验收Fixtures.md#fx-o)、[FX-P](../validation/固定验收Fixtures.md#fx-p)；[VS-V09](../validation/验收规格.md#vs-v09), [VS-V12](../validation/验收规格.md#vs-v12), [VS-V35](../validation/验收规格.md#vs-v35), [VS-V40](../validation/验收规格.md#vs-v40), [VS-V41](../validation/验收规格.md#vs-v41)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：Orchestrator→SSH | [CT-ssh-AcquireRuntimeOperationAdmissibility](../contracts/ssh.md#ct-ssh-acquireruntimeoperationadmissibility)、[CT-ssh-ExecuteOperation](../contracts/ssh.md#ct-ssh-executeoperation) | 精确TargetRef/SessionRef/CredentialRef及动态授权检查 |
| 返回边：ssh→执行调用方 | [CT-ssh-ExecuteOperation](../contracts/ssh.md#ct-ssh-executeoperation) 的typed Result | [CT-orchestrator-OrchestratorCommitRuntimeOperationReceipt](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommitruntimeoperationreceipt)；返回结果的后续receipt采纳沿既有Orchestrator消费路径，不新增能力owner直写运行状态的通道 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[ssh具体合同页](../contracts/ssh.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

逐项subscribedTopics、BeginShutdown/Quiesce和Teardown逆序成功出栈已经实现；原started清理缺口不能重复列为待实现。

复用精确target/session/credential授权，验证session drain与远端操作是否完成分别记录，unknown按原身份对账而不新建远端Run。

当前路径：`internal/modules/blocks/ssh`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/ssh/module.go#L1-L55)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/ssh/pkg/common/common.go#L1-L10)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/ssh/biz/owner.go#L1-L546)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 责任、证据和复用决策

DM13/24：远端目标、session、credential引用和实际执行事实。本组件拥有能力及真实操作事实，不拥有Workspace合同、Run终态或Orchestrator的义务闭包。receipt不是credit，能力ready不是行为验收通过。

**已知状态：** 逐项subscribedTopics、BeginShutdown/Quiesce和Teardown逆序成功出栈已经实现；原started清理缺口不能重复列为待实现。

**复用/适配/替换：** 复用SSH adapter、owner配置、session/transport、授权与receipt；适配远端scope、效果对账与动态失效；验证现有部分启动释放逻辑。远端资源仍由SSH owner管理，不新增远端Run权威。

## 2. 上下游合同与本组件规则

Orchestrator向SSH owner发送封存操作，RuntimeExecutionAuthorization绑定CatalogSnapshotID、OwnerRevision、CatalogObservation、Policy、BindingRef；BindingRef匹配TargetRef/SessionRef/CredentialRef。endpoint和秘密owner私有，事实/错误脱敏回传；Run不直接持有SSH连接。

远端命令边界须列目标主机、主体、工作目录、允许动作、路径/网络范围及凭据版本。连通性是readiness，不代表具备写权限。网络断开/本地context取消不证明远端命令停止；需远端操作身份、查询能力和可解释unknown。不能通过换session/key重做已可能生效的命令，也不能新Run清零旧效果/预算。

## 3. 小批迁移及兼容策略

1. 先回归已实现的每个订阅获取点并验证session drain，再配对Orchestrator上下文/receipt；先只读远端观察，再用获准可恢复副作用场景验收。真实远端权限或网络隔离缺失应记录blocked，不用本地transport fake宣称live完成。

## 4. 计划验收矩阵（全部未执行）

| 场景 | 必须观察到的结果 | 独立证据 |
|---|---|---|
| 匹配目标与授权执行 | 返回实际远端状态和脱敏receipt | target revision、命令结果 |
| target/session/credential漂移 | 拒绝，不连接错误目标 | 绑定差异、连接计数 |
| 每个订阅点失败 | 已获取资源释放/保留重试正确 | 订阅和session记录 |
| 远端执行后网络中断 | unknown并保留预算，按原身份对账 | 远端日志、operation账 |
| 同key不同命令 | 冲突，不再执行 | payload hash、命令计数 |
| 取消后远端未停 | 展示实际未知/运行状态，不伪造回滚 | 本地及远端状态 |
| 跨目录/越权限请求 | 执行前阻塞或权限拒绝可解释 | scope、拒绝结果 |
| 恢复/新Run遇到旧效果 | 不重放旧副作用，保留前驱责任 | 前后Run、receipt关联 |

## 6. 历史固定源码与规范（5554296）

- [ssh/pkg/events/operation.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/ssh/pkg/events/operation.go)
- [ssh/biz/owner.go#L44-L115](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/ssh/biz/owner.go#L44-L115)
- [ssh/biz/ssh_target_session.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/ssh/biz/ssh_target_session.go)
- [ssh/biz/ssh_transport.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/ssh/biz/ssh_transport.go)
- [ssh/biz/runtime_operation_receipt.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/ssh/biz/runtime_operation_receipt.go)

前轮读取operation合同；其余历史链接为已读关键控制流或固定树核对的具体深入审查入口，不声称全部源码已穷尽审核。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
