# MIG12：mcp 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | typed Registry解析、HTTP/stdio transport、授权与receipt |
| 本轮剩余适配/验收（已有实现见§0） | 已有逐订阅恢复回归、版本漂移核验和transport各自unknown对账 |
| 移除或禁止继续依赖 | started=false跳过清理；将目录存在当操作许可 |

- 批次归属：参与C04/C09/C12/C13；按能力需要参与B03/B07/B08/B10，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：可先闭合一种真实transport；另一种保持未支持，不用HTTP mock宣称stdio/live通过
- 升级/回退重点：保留RegistryRevision及封存授权，旧receipt缺绑定不升级为当前证据；session退出不推断远端effect回滚。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-L](../validation/固定验收Fixtures.md#fx-l)、[FX-P](../validation/固定验收Fixtures.md#fx-p)、[FX-O](../validation/固定验收Fixtures.md#fx-o)；[VS-V35](../validation/验收规格.md#vs-v35), [VS-V09](../validation/验收规格.md#vs-v09), [VS-V13](../validation/验收规格.md#vs-v13), [VS-V24](../validation/验收规格.md#vs-v24)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：Orchestrator→MCP | [CT-mcp-AcquireRuntimeOperationAdmissibility](../contracts/mcp.md#ct-mcp-acquireruntimeoperationadmissibility)、[CT-mcp-ExecuteOperation](../contracts/mcp.md#ct-mcp-executeoperation)、[CT-mcp-QueryRuntimeCWIReconciliation](../contracts/mcp.md#ct-mcp-queryruntimecwireconciliation) | 保留封存授权、效应和未知对账 |
| 出边：MCP→MCPRegistry | [CT-mcpregistry-MCPCommandGetServer](../contracts/mcpregistry.md#ct-mcpregistry-mcpcommandgetserver)、[CT-mcpregistry-MCPCommandReadRegistrySnapshot](../contracts/mcpregistry.md#ct-mcpregistry-mcpcommandreadregistrysnapshot) | 解析精确registry revision，transport/credential保持私有 |
| 返回边：mcp→执行调用方 | [CT-mcp-ExecuteOperation](../contracts/mcp.md#ct-mcp-executeoperation) 的typed Result | [CT-orchestrator-OrchestratorCommitRuntimeOperationReceipt](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommitruntimeoperationreceipt)；返回结果的后续receipt采纳沿既有Orchestrator消费路径，不新增能力owner直写运行状态的通道 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[mcp具体合同页](../contracts/mcp.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

Start/Teardown已使用逐项subscribedTopics账本、fresh/drain生命周期和可重试释放；不再以started位作为唯一清理依据。

复用registry typed解析、授权与receipt；按HTTP和stdio分别验证session停止/远端effect unknown以及Registry版本漂移。

当前路径：`internal/modules/blocks/mcp`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/module.go#L1-L62)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/pkg/common/common.go#L1-L14)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/mcp/biz/owner.go#L1-L1025)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 责任、证据和复用决策

DM24，关联DM39：远端工具执行事实；注册资产由MCPRegistry拥有。本组件拥有能力及真实操作事实，不拥有Workspace合同、Run终态或Orchestrator的义务闭包。receipt不是credit，能力ready不是行为验收通过。

**已知状态：** Start/Teardown已使用逐项subscribedTopics账本、fresh/drain生命周期和可重试释放；不再以started位作为唯一清理依据。

**复用/适配/替换：** 复用typed registry查询、HTTP/stdio transport、schema校验、RuntimeExecutionAuthorization和receipt；适配unknown对账与资源版本失效；验证已实现的部分启动清理。MCP继续Block，不把Registry迁进来，也不输出第二证明闭包。

## 2. 上下游合同与本组件规则

Orchestrator发能力/准入/feasibility/执行/reconciliation命令；MCP经MCPRegistry owner合同解析精确registry revision。现有授权封存CatalogSnapshotID、AcquisitionBinding、RegistryRevision、Capability、Policy及可选ValidationSurface；endpoint和credential保持owner私有，不放公共事件。

注册可发现不等于允许调用；schema接受不等于参数已授权。校验server/tool、输入hash、effect scope与真实能力版本，权限撤销要在执行时复核。远端不支持操作查询或幂等key时明确unknown及人工/策略对账，不承诺所有工具exactly-once。stdio session退出与远端effect终止分开。验证surface可作为方法能力，不凭工具自述就认定独立可信证明。

## 3. 小批迁移及兼容策略

1. 先验证已改订阅生命周期并保留adapter/session直到真实退出；再以Registry消费者和Orchestrator生产者配对验证漂移拒绝；随后选择一种实际transport闭合receipt对账，再扩展另一种，不以HTTP mock代表stdio/真实provider通过。

## 4. 计划验收矩阵（全部未执行）

| 场景 | 必须观察到的结果 | 独立证据 |
|---|---|---|
| 已授权精确tool调用 | 返回真实receipt，未自行闭合义务 | registry revision、请求/结果 |
| 任一订阅失败 | 清理已获取项且可重启 | 订阅状态与重试日志 |
| 注册版本或tool schema漂移 | 执行前拒绝/重新准入 | 旧新revision、拒绝结果 |
| 批准tool A后调用A+B | scope/payload冲突，不发transport | 授权hash、transport计数 |
| 远端成功本地超时 | 保留unknown和预算，按原身份对账 | 服务端操作记录、receipt |
| 重复请求与同key异payload | 同结果或安全对账；异payload冲突 | operation账、实际调用数 |
| session取消但远端仍运行 | 不报告effect已回滚 | session/远端状态分别记录 |
| 没有行为recipe仅连接ready | 保持unverifiable，不生成行为PASS | reconciliation、binding |

## 6. 历史固定源码与规范（5554296）

- [mcp/pkg/events/operation.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/mcp/pkg/events/operation.go)
- [mcp/biz/owner.go#L64-L115](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/mcp/biz/owner.go#L64-L115)
- [mcp/biz/mcp_http_transport.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/mcp/biz/mcp_http_transport.go)
- [mcp/biz/mcp_stdio_session_manager.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/mcp/biz/mcp_stdio_session_manager.go)
- [mcp/biz/runtime_operation_receipt.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/mcp/biz/runtime_operation_receipt.go)

前轮读取operation合同；其余历史链接为已读关键控制流或固定树核对的具体深入审查入口，不声称全部源码已穷尽审核。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
