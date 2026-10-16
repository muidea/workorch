# MIG10：git 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | Git adapter、精确工作树/revision与receipt |
| 本轮剩余适配/验收（已有实现见§0） | 已实现逐订阅账本回归、工作树变更依赖失效 |
| 移除或禁止继续依赖 | 单started位清理条件；为恢复干净擅自reset用户改动 |

- 批次归属：参与C04/C09/C10/C12/C13；按能力需要参与B03/B07/B08，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：先完成订阅故障回收，再开放相关操作；只读revision观察不授证明credit
- 升级/回退重点：保留未提交文件/index/子模块的实际依赖边界；commit响应丢失先核原提交，不重复提交。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-L](../validation/固定验收Fixtures.md#fx-l)、[FX-O](../validation/固定验收Fixtures.md#fx-o)；[VS-V35](../validation/验收规格.md#vs-v35), [VS-V09](../validation/验收规格.md#vs-v09), [VS-V24](../validation/验收规格.md#vs-v24), [VS-V12](../validation/验收规格.md#vs-v12)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：Orchestrator→Git | [CT-git-AcquireCapabilityState](../contracts/git.md#ct-git-acquirecapabilitystate)、[CT-git-ExecuteOperation](../contracts/git.md#ct-git-executeoperation)、[CT-git-QueryRuntimeCWIReconciliation](../contracts/git.md#ct-git-queryruntimecwireconciliation) | 仓库/工作树状态、执行与reconciliation分开 |
| 出边：Git→File | [CT-file-AcquireWorkspaceManifest](../contracts/file.md#ct-file-acquireworkspacemanifest) | 使用typed manifest取得必要文件事实，保持workspace-repository选择器 |
| 返回边：git→执行调用方 | [CT-git-ExecuteOperation](../contracts/git.md#ct-git-executeoperation) 的typed Result | [CT-orchestrator-OrchestratorCommitRuntimeOperationReceipt](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommitruntimeoperationreceipt)；返回结果的后续receipt采纳沿既有Orchestrator消费路径，不新增能力owner直写运行状态的通道 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[git具体合同页](../contracts/git.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

Start按subscribedTopics长度从未完成处续订，逐项成功后登记；Teardown按逆序成功退订后出栈，BeginShutdown/Quiesce按真实在途管理。旧started=false跳过清理结论已失效。

复用Git adapter和receipt；补工作树/未提交文件/子模块适用依赖、超时对账及File协作；失败恢复沿原操作，不reset用户工作树。

当前路径：`internal/modules/blocks/git`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/module.go#L1-L69)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/pkg/common/common.go#L1-L35)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/git/biz/owner.go#L1-L1084)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 责任、证据和复用决策

DM13/24：仓库资源、工作树/revision和操作事实。本组件拥有能力及真实操作事实，不拥有Workspace合同、Run终态或Orchestrator的义务闭包。receipt不是credit，能力ready不是行为验收通过。

**已知状态：** Start按subscribedTopics长度从未完成处续订，逐项成功后登记；Teardown按逆序成功退订后出栈，BeginShutdown/Quiesce按真实在途管理。旧started=false跳过清理结论已失效。

**复用/适配/替换：** 复用Git adapter、仓库检查、revision及capability receipt存储、sealed operation；适配精确工作树依赖和scope；保留逐成功订阅的取得/释放账本。保留Block角色及现存receipt身份。

## 2. 上下游合同与本组件规则

Orchestrator通过Git的能力、准入、feasibility、执行和reconciliation typed合同消费事实；Git通过File typed合同获取必要文件事实，不持有其repo。workspace-repository选择器继续表达仓库对象，不要求LLM发明路径。回传提交/工作树观察和receipt，不直接裁决CWI通过。

仓库身份须包含实际受管工作树与所需revision，而非仅目录或HEAD字符串。修改未提交文件、index、子模块等依赖是否参与必须由本批操作合同明确。既有用户仓库与临时worktree分开归属，禁止为恢复“干净”而擅自reset用户改动。commit成功只证明提交事实，不能证明测试或目标完成。

## 3. 小批迁移及兼容策略

1. 先验证逐成功订阅记账，覆盖全部失败点和取消订阅重试；同批核对生产模块桥和共享生命周期协议。再把版本观察配对Orchestrator依赖失效，最后验证本批真实Git操作及File协作，不能只测命令runner。

## 4. 计划验收矩阵（全部未执行）

| 场景 | 必须观察到的结果 | 独立证据 |
|---|---|---|
| 受管工作树只读观察 | 返回精确revision与scope，不赋验收credit | 仓库快照、typed结果 |
| 每个订阅点注入失败 | 已获取订阅可清理且重启无重复 | 订阅计数、清理日志 |
| 取消订阅失败后重试 | 保留未清理项，不丢owner | 重试前后清单 |
| 同身份payload改变 | 拒绝，无额外commit | receipt、git日志 |
| HEAD未变但相关文件改变 | 适用证据失效或重验 | 工作树摘要、依赖判断 |
| commit已发生响应丢失 | 查原操作与仓库，不盲目再commit | 原operation、提交记录 |
| 外部用户未提交文件 | 超范围操作拒绝或明确阻塞 | 前后diff、scope |
| 旧Run迟到写请求 | 过期fence拒绝，不改变后继工作树 | Run/fence、仓库差异 |

## 6. 历史固定源码与规范（5554296）

- [git/pkg/events/operation.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/git/pkg/events/operation.go)
- [git/biz/owner.go#L81-L115](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/git/biz/owner.go#L81-L115)
- [git/biz/git_repository_inspection.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/git/biz/git_repository_inspection.go)
- [git/biz/git_revision.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/git/biz/git_revision.go)
- [git/biz/capability_state_receipt_store.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/git/biz/capability_state_receipt_store.go)

前轮读取operation合同；其余历史链接为已读关键控制流或固定树核对的具体深入审查入口，不声称全部源码已穷尽审核。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
