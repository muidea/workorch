# MIG17：shell 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | sealed授权、recipe/ProofBinding、scratch保护及私有conjunction producer |
| 本轮剩余适配/验收（已有实现见§0） | 复用正式catalog/执行/receipt接线，补raw-output消费闭环并回归ordinary proof/版本 |
| 移除或禁止继续依赖 | 禁止退回started-only清理；把scratch当OS/网络/凭据隔离或未接线声明available |

- 批次归属：参与C04/C09/C10/C12/C13/C16；按能力需要参与B03/B07/B08/B10，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：保留已有sealed recipe生产执行接线；原文读取缺sender单独阻塞对应闭环，能力availability仍须匹配实际支持范围
- 升级/回退重点：保留旧recipe政策、原operation deadline和输出事实；不能回退为忽略零测试/截断/source漂移的读者。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-R](../validation/固定验收Fixtures.md#fx-r)、[FX-O](../validation/固定验收Fixtures.md#fx-o)、[FX-L](../validation/固定验收Fixtures.md#fx-l)；[VS-V23](../validation/验收规格.md#vs-v23), [VS-V25](../validation/验收规格.md#vs-v25), [VS-V35](../validation/验收规格.md#vs-v35), [VS-V40](../validation/验收规格.md#vs-v40), [VS-V47](../validation/验收规格.md#vs-v47)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：Orchestrator→Shell | [CT-shell-ExecuteOperation](../contracts/shell.md#ct-shell-executeoperation)、[CT-shell-ProbeRuntimeOwnerFactProspectiveSatisfaction](../contracts/shell.md#ct-shell-proberuntimeownerfactprospectivesatisfaction)、[CT-shell-QueryRuntimeCWIReconciliation](../contracts/shell.md#ct-shell-queryruntimecwireconciliation) | 复用授权、recipe与ProofBinding；private conjunction尚需正式生产接线 |
| 返回边：shell→执行调用方 | [CT-shell-ExecuteOperation](../contracts/shell.md#ct-shell-executeoperation) 的typed Result | [CT-orchestrator-OrchestratorCommitRuntimeOperationReceipt](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommitruntimeoperationreceipt)；返回结果的后续receipt采纳沿既有Orchestrator消费路径，不新增能力owner直写运行状态的通道 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[shell具体合同页](../contracts/shell.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

Start/Teardown的逐项账本已取代started清理；owner.go在执行和reconciliation分支调用executeShellProjectScope，不能继续用旧private文件注释断言整条合取链未接线。新增QueryRuntimeValidationRawOutput接收端，但最新固定SHA既有CI确认缺生产sender。

复用catalog/recipe/四阶段执行与原文存储，仅补原文查询生产消费闭环和inventory期望；持续验证固定all_of、raw完整性、实际CWI语义及强隔离适用条件，不降低fail-closed。

当前路径：`internal/modules/blocks/shell`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/module.go#L1-L61)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/pkg/common/common.go#L1-L47)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L1-L980)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

[实际执行分支](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L390-L425)；[原文查询订阅](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/shell/biz/owner.go#L73-L96)。原文消费缺口与普通执行recipe可用性分别记录，不把一个sender缺失扩大为全部执行不可用。

## 1. 责任、证据和复用决策

DM24，关联DM22/31–34：命令及validation recipe执行观察。本组件拥有能力及真实操作事实，不拥有Workspace合同、Run终态或Orchestrator的义务闭包。receipt不是credit，能力ready不是行为验收通过。

**已知状态：** Start/Teardown的逐项账本已取代started清理；owner.go在执行和reconciliation分支调用executeShellProjectScope，不能继续用旧private文件注释断言整条合取链未接线。新增QueryRuntimeValidationRawOutput接收端，但最新固定SHA既有CI确认缺生产sender。

**复用/适配/替换：** 复用sealed授权、recipe/ValidationProofBinding、原生检查和validation输入保护；适配实际执行覆盖、对象版本及超时事实；回归已替换的逐项清理，保留Shell单能力Block，验证裁决仍归Orchestrator。

### 1.1 合取执行已接线与原文消费剩余边界

Start/Teardown的逐项账本已取代started清理；owner.go在执行和reconciliation分支调用executeShellProjectScope，不能继续用旧private文件注释断言整条合取链未接线。新增QueryRuntimeValidationRawOutput接收端，但最新固定SHA既有CI确认缺生产sender。 四阶段all_of约束及原operation deadline仍保持；旧validation_project_conjunction.go的“尚未接线”注释不能覆盖owner.go的实际调用。

新增[CT-shell-QueryRuntimeValidationRawOutput](../contracts/shell.md#ct-shell-queryruntimevalidationrawoutput)；既有最新CI在此topic缺生产sender及inventory数量期望失配处失败，后续MVP跳过。精确运行证据及唯一gap见[总纲](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)，本轮未触发CI。

## 2. 上下游合同与本组件规则

Orchestrator提交精确执行上下文、封存命令及validation绑定；Shell返回进程状态、实际断言/测试数、日志、对象hash和receipt。现有RuntimeExecutionAuthorization已含CatalogObservation、Policy、ValidationRecipes、ValidationScopeTargetRefs及ValidationProofBinding，避免另加未绑定参数旁路。

定义封存时固定验证政策、对象选择规则和允许生成recipe的约束；运行时生成recipe只可细化，不降低门槛。对象生成后封存实际hash并记录验证环境与输入版本。退出码0、零测试、编译成功或错误目标均不能直接当作验收通过。scratch相对写可丢弃，不保证绝对路径、子进程、网络和共享缓存隔离；威胁模型要求更强能力时阻塞该recipe，不能自动放宽合同。

## 3. 小批迁移及兼容策略

1. 先覆盖启动故障和子进程退出/预算保留，复用但不重写已有private conjunction producer，获实施授权后补完整生产接线，再配对Orchestrator的policy、binding和Shell观察；用一个真实最小验证recipe证明正负例，然后扩展受支持方法。File负责实际产物内容，Result负责元数据；Shell不偷用其他owner的repo。

## 4. 计划验收矩阵（全部未执行）

| 场景 | 必须观察到的结果 | 独立证据 |
|---|---|---|
| 精确对象执行有效recipe | 报告命中断言与真实结果 | recipe/object hash、测试计数 |
| 退出码0但零测试 | 不产生合格验证观察 | 完整输出、解析计数 |
| 同名旧对象/错误目录 | 拒绝或结果不适用 | 对象hash、目标引用 |
| recipe降低required断言 | 封存校验拒绝 | 政策、recipe差异 |
| scratch中符号链接 | 拒绝复制；无宿主越界写 | 沙箱准备结果、文件差异 |
| 部分订阅失败 | 清理已成功订阅，可重复启动 | 订阅/退出记录 |
| 超时子进程尚存或效果未知 | 不当作已停止，保留对账责任 | 进程状态、receipt、预算 |
| 重复请求及迟到结果 | 原身份处理，旧generation不授credit | dispatch、binding、receipt |

## 6. 历史固定源码与规范（最新增量见1.1）

- [shell/pkg/events/operation.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/pkg/events/operation.go)
- [shell/biz/owner.go#L60-L92](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/biz/owner.go#L60-L92)
- [shell/biz/shell_validation_sandbox.go#L16-L115](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/biz/shell_validation_sandbox.go#L16-L115)
- [shell/biz/shell_runtime_execute.go#L135-L153](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/biz/shell_runtime_execute.go#L135-L153)
- [shell/biz/validation_observation.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/shell/biz/validation_observation.go)

本轮重新读取operation合同；其余链接为已读关键控制流或固定树核对的具体深入审查入口，不声称全部源码已穷尽审核。

新增未运行验收：未接线时catalog/readiness不得宣称project-conjunction可用；生产接线后固定四阶段缺一、任一skip/unfinished JSON/零测试、raw输出容量超限、source/member变动、总deadline到期或后代清理失败均不授proof；前三阶段成功但当前CWI行为失败仍失败。


修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
