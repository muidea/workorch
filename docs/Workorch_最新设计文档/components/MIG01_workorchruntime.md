# MIG01 workorchruntime组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Initiator → Initiator；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | 单一provider、owner命名空间及原数据布局 |
| 本轮剩余适配/验收（已有实现见§0） | 按已取得资源记录准备/失败回收，参与最终释放屏障 |
| 移除或禁止继续依赖 | 提前关闭共享provider；没有另一个业务存储owner |

- 批次归属：参与C03/C04/C12；按能力需要参与B03/B07/B08，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：可先完成存储清单与失败传播适配；共享释放次序维持旧路径，直到全部消费者与真实框架桥同步切换
- 升级/回退重点：文件/内存repository的Result raw clone与MIG15成对核对；若格式升级，保留原journal和显式读写版本。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-L](../validation/固定验收Fixtures.md#fx-l)、[FX-T](../validation/固定验收Fixtures.md#fx-t)；[VS-V35](../validation/验收规格.md#vs-v35), [VS-V10](../validation/验收规格.md#vs-v10), [VS-V11](../validation/验收规格.md#vs-v11), [VS-V39](../validation/验收规格.md#vs-v39)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：注册owner Setup→provider | [CF00](../Workorch_组件固定合同与分层验收规范.md#cf00) | CF00基础设施装配；只注入owner-scoped repository能力，无业务EventHub topic |
| 出边：provider→Setup消费者 | [CF00](../Workorch_组件固定合同与分层验收规范.md#cf00) | 返回句柄或失败；最终释放由真实进程生命周期屏障允许。不得新造持久化业务Command |


<a id="storage-raw-clone"></a>
## 0. 2026-10-12固定SHA重核与剩余增量

内部Initiator角色不变，module.go已增加重复Setup拒绝、closing、BeginShutdown/Quiesce；provider仍是owner-scoped存储能力。全局进程已走ShutdownChecked，不能再列作尚未接线。

保留provider和数据布局，重点验证部分Setup、失败重试及最后释放顺序；存储格式与原子性按实际owner边逐项核验。

当前路径：`internal/initiators/workorchruntime`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/initiators/workorchruntime/module.go#L1-L257)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/initiators/workorchruntime/pkg/common/common.go#L1-L73)；[进程checked桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/services/workorch/framework_service.go#L1-L39)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

共享Result raw clone继续直接复用，文件/内存repository的原材料保真须与MIG15恢复链成对核验；不升级为跨owner原子性声明。

## 1. 当前与目标责任

保持Initiator及现路径 internal/initiators/workorchruntime。其唯一能力是owner-scoped持久化基础设施，不是DM业务状态owner；不得加入Hub路由、目标准入或恢复策略。现根实现提供repository provider、不持有Hub/Base，单能力形态可复用。多组repository不自动意味着多种基础设施能力。

目标：各owner只取得其授权存储能力，准备失败显式阻止依赖者启动；所有owner与共享任务/Hub真正排空后才能释放provider。全部存储实现的原子性、锁、文件格式和错误行为尚未穷尽核验，不能宣布无需修改。

## 2. 复用与路径影响

| 分类 | 处理 |
|---|---|
| 直接复用 | module.go中的单一provider装配与owner命名空间原则；保留存量Workspace/Run/Decision等身份 |
| 适配核验 | owner plugin Setup取得repo的失败传播、重复初始化、部分准备失败回收；盘点provider实际全部调用方 |
| 可能替换 | 只有证实不满足原子提交/恢复要求的局部存储算法才替换；本方案不要求更换存储引擎 |

已核路径：internal/initiators/workorchruntime/module.go、application/workorch/main.go、internal/services/workorch/framework_service.go，以及实际vendor application生命周期。具体repository文件清单在G0根据代码树逐一补齐，不能凭逻辑模型新建40张表。

## 3. 合同与相邻组件

此Initiator不强行虚构一套业务typed EventHub命令。provider仅在允许的基础设施装配边界注入；业务owner之间仍走各owner的pkg/events合同，不能借provider直读他方资源。

- 生产者：Initiator准备owner-scoped repository能力；消费者：显式注册单元的Setup
- 就绪合同：可用句柄或明确失败，不能返回“看似可用、首个写入再发现未初始化”的代理成功
- 释放合同：进程生命周期协调者确认owner quiescence、共享任务与Hub drain后方可调用最终释放；单owner退出不能代表全局可关库
- 业务原子性：由该业务owner定义CAS/幂等语义，基础设施负责可靠执行及错误/未知结果区分，不自行决定合同批准或Run终态

禁止暴露可变全局仓库集合给任意组件，禁止为了重试换命名空间生成第二份正式状态。

## 4. 最小迁移步骤及依赖

1. 与各owner共同列出存储命名空间、格式版本、提交原子性、关闭行为及未完成事务策略，先只补清单
2. 对受影响owner的准备失败路径加入可诊断错误和资源获取登记；与消费者同步适配，保留原数据布局
3. 配合进程service、Base和所有生命周期参与者准备分阶段停止协议；准备期间不得提前切换全局释放次序
4. 全参与者验证后再切换共享屏障；缺一owner排空证明时保留provider，不发布“进程正常退出”成功
5. 如需存储格式改变，单独小批双版本读取/显式写版本；完成独立迁移验证后才允许依赖新格式业务上线

依赖不是串行改完22个业务域：共享屏障需要全部参与者协议适配，但业务模型适配仍可按能力切片推进。

## 5. 兼容、故障恢复及回退

默认不改变ownerID、数据目录、WorkspaceID、RunID、journal键。持久化成功但响应丢失时，由业务请求身份读回确认，不猜测“未写入”。部分初始化仅释放已确认取得且无人使用的资源；关闭超时保留状态并允许用新预算重试。

回退优先退程序且保留兼容数据；若旧程序无法识别新schema，禁止直接启动旧版覆盖。先冻结写入、保存完整ledger与备份并验证可逆迁移；外部执行已发生时，恢复旧磁盘快照会遗忘副作用和预算，必须先对账，必要时采用前向修复。文件备份成功不构成业务一致性证明。

## 6. 验收用例（拟定，未运行）

1. 所有必需repo准备完成后owner可启动；缺一个必需repo必须fail-fast并指明owner
2. 第二个资源准备失败时，第一个已取得资源可安全回收，重试不会产生第二命名空间
3. Run写入成功但响应丢失，原请求读回唯一记录；同身份不同payload返回冲突
4. owner仍执行任务时请求provider释放必须被屏障阻止
5. Hub排空超时仍保留存储，后续重试成功后只释放一次
6. 同一数据集升级后读取原ID/journal保持一致；旧版本不支持格式时明确拒绝
7. 进程崩溃重启恢复未完成owner journal，既不重置预算也不重复外部effect
8. 装配/依赖检查证明业务owner没有借provider读取另一个owner仓库

## 8. 历史固定版本证据（5554296）

- [单能力Initiator](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/initiators/workorchruntime/module.go#L24-L97)
- [显式入口装配](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/application/workorch/main.go#L10-L34)
- [进程桥接](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/services/workorch/framework_service.go#L12-L35)
- [实际共享drain顺序](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/vendor/github.com/muidea/magicCommon/framework/application/application.go#L270-L313)

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
