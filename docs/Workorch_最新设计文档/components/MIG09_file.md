# MIG09：file 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | 路径guard、authoring读取、sealed操作及File内容owner |
| 本轮剩余适配/验收（已有实现见§0） | 资源归属、真实打开边界、版本/hash及可恢复交付引用 |
| 移除或禁止继续依赖 | 用户源目录误清理、临时副本冒充持久产物 |

- 批次归属：参与C05/C09/C12/C13；按能力需要参与B05/B07/B09，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：authoring只读链可先验收；执行与Result交付引用未闭合时不宣告产物已交付
- 升级/回退重点：路径/资源迁移保留归属清单；已写入后未知用原operation读回，不以旧快照或再写替代对账。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-G](../validation/固定验收Fixtures.md#fx-g)、[FX-O](../validation/固定验收Fixtures.md#fx-o)、[FX-D](../validation/固定验收Fixtures.md#fx-d)；[VS-V09](../validation/验收规格.md#vs-v09), [VS-V12](../validation/验收规格.md#vs-v12), [VS-V34](../validation/验收规格.md#vs-v34), [VS-V40](../validation/验收规格.md#vs-v40)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：Workspace/workorchd→File准备读取 | [CT-file-AcquireAuthoringResources](../contracts/file.md#ct-file-acquireauthoringresources)、[CT-file-VerifyAuthoringResource](../contracts/file.md#ct-file-verifyauthoringresource) | 定义scope无需Run；复用原authoring hash与资源事实 |
| 入边：Orchestrator→File执行 | [CT-file-ExecuteFileReadOperation](../contracts/file.md#ct-file-executefilereadoperation)、[CT-file-ExecuteFileWriteOperation](../contracts/file.md#ct-file-executefilewriteoperation)、[CT-file-ExecutePatchApplyOperation](../contracts/file.md#ct-file-executepatchapplyoperation) | 实际打开/路径guard、内容版本与receipt绑定；完整操作集见本owner目录 |
| 返回边：file→执行调用方 | [CT-file-ExecuteFileWriteOperation](../contracts/file.md#ct-file-executefilewriteoperation) 的typed Result | [CT-orchestrator-OrchestratorCommitRuntimeOperationReceipt](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommitruntimeoperationreceipt)；返回结果的后续receipt采纳沿既有Orchestrator消费路径，不新增能力owner直写运行状态的通道 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[file具体合同页](../contracts/file.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。



新增出入配对：[CT-file-QueryRuntimeFileReadDependencies](../contracts/file.md#ct-file-queryruntimefilereaddependencies)。Orchestrator读取实际File依赖快照；复用该查询而非继续只使用runtime_authority合成依赖。

## 0. 2026-10-12固定SHA重核与剩余增量

新增owner_lifecycle与QueryRuntimeFileReadDependencies公开合同；authoring资源与sealed操作的身份链保持分离。路径guard与共享repository/file仍是不同责任。

直接复用依赖查询与读写事实；与Orchestrator成对核验被读取对象的真实版本、路径替换/符号链接/TOCTOU、receipt恢复和Result交付引用。

当前路径：`internal/modules/blocks/file`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/module.go#L1-L72)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/pkg/common/common.go#L1-L20)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner_lifecycle.go#L1-L70)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/file/biz/owner.go#L1-L1254)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 责任、证据和复用决策

DM13/24/30：文件资源、执行回执及产物内容。本组件拥有能力及真实操作事实，不拥有Workspace合同、Run终态或Orchestrator的义务闭包。receipt不是credit，能力ready不是行为验收通过。

**已知状态：** 适配、深核；没有证据证明File存在Git/Shell同型started清理缺陷。已核实封存操作、authoring资源读取和路径检查实现，不能把目标适配全部登记为现存bug。

**复用/适配/替换：** 保留文件内容、路径guard、typed authoring查询和sealed operation；适配归属、资源版本、操作恢复及交付引用；仅替换被实际证明不合规的路径，不另建文件仓库或通用资源Module。

## 2. 上下游合同与本组件规则

创建阶段由Workspace/应用通过File合同准备或观察资源，现有AcquireAuthoringResourceCommand只绑定WorkspaceID、AuthoringInputHash和ResourceRef，无Run身份；返回WorkspaceRevision、ContentHash和Size。执行阶段由Orchestrator提交sealed operation，File返回真实操作receipt；Result仅保存交付元数据，不能越过File读取可变句柄。

资源分为用户既有源目录、Workspace管理目录、共享缓存、临时验证副本和正式产物。逐项记录创建者、owner、生命周期、允许删除范围；只读引用不授写权限。创建失败只清理本次已登记拥有的临时资源。删除Workspace由其唯一journal协调，File只处理自己的阶段。文件最终存在且稳定后再封存实际内容hash；未生成产物只能预声明选择规则，不编造hash。

## 3. 小批迁移及兼容策略

1. 先配对Workspace的authoring资源查询与File响应校验，完成草稿资源观察；再配对Orchestrator执行与Result产物引用，保留两条不同身份链。对符号链接、路径替换及并发内容变更做实际打开/读写边界核验；路径字符串规范化不自动证明无TOCTOU。

## 4. 计划验收矩阵（全部未执行）

| 场景 | 必须观察到的结果 | 独立证据 |
|---|---|---|
| 受限目录合法资源读取 | 返回精确内容/hash，M3前不创建Run | authoring请求及响应、Run列表 |
| 未获授权的绝对路径，或借../、符号链接逃逸允许边界 | 拒绝且目录外无读取/改写 | 拒绝原因、文件系统差异 |
| 准备中断后恢复 | 同Workspace复用已成功阶段，不删源目录 | 阶段回执、归属清单 |
| 同操作身份换payload | 冲突，不覆盖原内容 | 封存hash、前后快照 |
| 产物同名内容改变 | 旧证据不满足当前对象 | 两个内容hash、依赖失效记录 |
| 写入发生后响应丢失 | 按原身份查实际效果，unknown不盲写 | receipt、原子性/崩溃点记录 |
| 临时副本清理失败 | 保留清理责任，不冒充正式交付 | 资源状态及Result引用 |
| 取消/删除与写入竞争 | fence后无新dispatch，既有操作可对账 | fence与操作时间线 |

## 6. 历史固定源码与规范（5554296）

- [file/pkg/events/operation.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/file/pkg/events/operation.go)
- [file/pkg/events/authoring_resources.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/file/pkg/events/authoring_resources.go)
- [file/biz/file_path_guard.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/file/biz/file_path_guard.go)
- [file/biz/authoring_resources_linux.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/file/biz/authoring_resources_linux.go)
- [file/biz/runtime_operation_receipt.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/file/biz/runtime_operation_receipt.go)

前轮读取operation合同；其余历史链接为已读关键控制流或固定树核对的具体深入审查入口，不声称全部源码已穷尽审核。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
