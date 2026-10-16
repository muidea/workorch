# MIG22 workspace组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Module → Module（原Block已提升）；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | 同WorkspaceID、定义/确认journal、活动slot、唯一删除journal及mapping/decoder |
| 本轮剩余适配/验收（已有实现见§0） | 已迁角色恢复核验、审查语义与覆盖回归、M3回执与独立首次激活绑定 |
| 移除或禁止继续依赖 | 新旧路径双注册、平行Contract/Deletion owner、截断长Goal或回填旧批准 |

- 批次归属：参与C03/C05/C06/C09/C11/C12；按能力需要参与B01/B02/B04/B05/B06/B07/B09/B11，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：保留已完成角色/路径批的数据身份不变；草稿、审查、M3/M4各按完整能力批开放，删除fence随首次相关写入前置
- 升级/回退重点：kernel/workspace已是当前路径；包回退只改变装配不变journal，schema升级另行版本化且保留原hash/批准。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-G](../validation/固定验收Fixtures.md#fx-g)、[FX-C](../validation/固定验收Fixtures.md#fx-c)、[FX-D](../validation/固定验收Fixtures.md#fx-d)、[FX-T](../validation/固定验收Fixtures.md#fx-t)、[FX-L](../validation/固定验收Fixtures.md#fx-l)；[VS-V26](../validation/验收规格.md#vs-v26), [VS-V27](../validation/验收规格.md#vs-v27), [VS-V28](../validation/验收规格.md#vs-v28), [VS-V31](../validation/验收规格.md#vs-v31), [VS-V34](../validation/验收规格.md#vs-v34), [VS-V44](../validation/验收规格.md#vs-v44), [VS-V45](../validation/验收规格.md#vs-v45)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：workorchd→Workspace定义 | [CT-workspace-WorkspaceDefinitionAuthoringCreate](../contracts/workspace.md#ct-workspace-workspacedefinitionauthoringcreate)、[CT-workspace-WorkspaceSpecificationComplete](../contracts/workspace.md#ct-workspace-workspacespecificationcomplete)、[CT-workspace-WorkspaceDefinitionAdmissionIntent](../contracts/workspace.md#ct-workspace-workspacedefinitionadmissionintent) | 复用mapping/decoder与物化journal；required范围覆盖已有实现，审查语义充分性仍需验证 |
| 入边：应用/Orchestrator→Workspace执行与终态 | [CT-workspace-WorkspaceCommandExecutionIntent](../contracts/workspace.md#ct-workspace-workspacecommandexecutionintent)、[CT-workspace-WorkspaceFactTerminalDelivery](../contracts/workspace.md#ct-workspace-workspacefactterminaldelivery) | M3/M4首次激活与handoff屏障按目标适配；主题存在不代表全部语义已通过 |
| 出边：Workspace→Run/Orchestrator | [CT-run-RunCommandWorkspaceExecutionIntent](../contracts/run.md#ct-run-runcommandworkspaceexecutionintent)、[CT-orchestrator-OrchestratorCommandBeginWorkspaceRuntimeQuiescence](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommandbeginworkspaceruntimequiescence)、[CT-orchestrator-OrchestratorCommandConfirmWorkspaceRuntimeQuiescence](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommandconfirmworkspaceruntimequiescence) | 沿现owner协调执行/删除；Run状态及运行语义仍分别归属 |
| 出边：Workspace→资源删除阶段 | [CT-memory-MemoryCommandSealWorkspaceDeletion](../contracts/memory.md#ct-memory-memorycommandsealworkspacedeletion)、[CT-result-ResultCommandPurgeWorkspace](../contracts/result.md#ct-result-resultcommandpurgeworkspace) | 选例，完整删除参与者以本owner目录及唯一journal为准；提升Module不更换这些业务owner身份 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[workspace具体合同页](../contracts/workspace.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。



新增入边：[CT-workspace-WorkspaceDefinitionChangePrepare](../contracts/workspace.md#ct-workspace-workspacedefinitionchangeprepare)与[CT-workspace-WorkspaceDefinitionChangeQuery](../contracts/workspace.md#ct-workspace-workspacedefinitionchangequery)。准备独立待审候选且保留原seal、journal及预算，不等于adoption。

## 0. 2026-10-12固定SHA重核与剩余增量

目录与入口已经提升为kernel/workspace，当前Module→Module；common.ModuleID仍为blocks/workspace，原wire/owner身份保留。ValidateReviewCoverage已逐critic检查所有required要求；source-obligation编译、无Run变更Prepare/Query及候选审查已有增量。

复用同WorkspaceID、journal、slot及现有映射/审查覆盖；剩余是M3/M4全链证据、精确最终批准/物化与运行期安全变更，不能再次迁目录或重新造候选预算。

当前路径：`internal/modules/kernel/workspace`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/module.go#L1-L52)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/common/common.go#L1-L3)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner_lifecycle.go#L1-L106)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/owner.go#L1-L305)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

sealed-source mapping、严格decoder、owner重载重判、独立32KiB exact Goal及成功原文审计全部复用。[required逐critic覆盖](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/pkg/models/goal_specification_review_coverage.go#L1-L51)；[候选准备与预算继承](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/kernel/workspace/biz/definition_change_preparation.go#L14-L83)。全required覆盖是结构条件，不替代审查语义充分性或有理由N/A的产品约束。

## 1. 当前与目标责任

当前kernel Module；保留DM01–16定义/创建权威、活动slot及DM40唯一删除journal/reducer。现22运行单元已为1 Initiator+5 Modules+16 Blocks，不新增Definition/Contract/Deletion owner。

已查证：删除协调已组合Run、Orchestrator及多资源owner合同，符合Module角色。新基线已实现sealed-source mapping闭环及独立长Goal例外，不能重复列为从零建设；critic required覆盖与运行源义务编译已有实现，审查语义充分性及其端到端验收仍须核验。目录迁移不能替代业务合同验收，应分成受控小批。

## 2. 复用、适配、替换与路径

- 直接复用现有WorkspaceID、命名空间、定义/确认物化journal、slot、删除journal及typed协调结构
- 角色批已经迁移并更新main导入；复核plugin单次注册、旧journal读回及所有生产/测试引用，不再次移动路径
- 业务适配复用 pkg/definitionadmission/compiler.go、pkg/models/goal_specification.go、definition_authoring.go、definition_authoring_specification.go；新增复用definition_authoring_mapping*.go、definition_authoring_success_audit.go及mapping_outcomes.go，避免绕过owner重解析/重编译
- 保留 biz/deletion_coordinator.go的唯一协调权威；workorchd只接收请求/组合响应，不复制其journal
- 替换范围仅限不满足完整性合同的局部校验/映射逻辑；共享DoD合法，不强制一义务一CWI，不截断长Goal

kernel/workspace是已存在的当前路径。后续公开合同变化仍须清点所有进口，不用业务层兼容桥绕过typed边界。

## 3. owner合同与不变量

| 交互 | 合同语义及归属 |
|---|---|
| workorchd→Workspace | 草稿/输入、候选、审查、精确确认、创建回执；Workspace权威写入，应用无第二账本 |
| Workspace↔资源owner | 准备/检查精确ResourceBinding；实际能力owner返回事实，Workspace记录绑定和M3阶段 |
| Workspace→Run/Orchestrator | 精确定义与源义务/策略、slot/fence协议；Run拥有SM-1，Orchestrator拥有SM-2/3/4 |
| Decision→Workspace | ticket原基线、原因/影响、最终候选approve；校验后物化新定义，不能resolved即改旧Run |
| Workspace→各owner删除 | 唯一journal记录阶段；各owner只清理自身状态，unknown和在途任务须先安全处理 |

M3=精确合同已封存、必要资源已准备、当时启动检查通过、Run仍为0。默认随后用户激活；授权自动也必须先M3再独立M4。M3历史回执不保证未来readiness。每项源义务保留身份/来源/策略；critic按适用要求×维度覆盖，有理由N/A，不能六个PASS替代全量覆盖。

<a id="automatic-candidate-acceptance"></a>
授权自动模式的候选接纳须形成绑定有界政策版本、精确候选及授权范围的接纳回执，不能伪造人工点击确认；任何超出授权范围的候选变动均不得自动接纳。

Workspace将同一M3回执/精确定义绑定首次激活意图，slot空闲不能解除既有去重关联。须与Run永久关联同批核验，见[MIG16首次激活](MIG16_run.md#first-activation-intent)。

## 4. 小批迁移次序与依赖

1. 复核已经完成的角色迁移保持身份/存储/事件不变、单次注册，并验证在途journal恢复；不夹带证明规则重写
2. 与workorchd完成草稿可恢复和Web-only创建；资源准备采用原请求身份，失败不宣称M3
3. 复用已有sealed-source mapping、required审查覆盖和成功审计，与LLM/workorchd核验语义充分性及精确确认物化；同时回归已接入Orchestrator输入的逐义务映射
4. 与Run/Orchestrator完成M3→M4 slot/fence和唯一启动；动态readiness由真实owner重新提供
5. 与Decision/Governance完成ticket变更、最终候选approve、新定义与唯一后继新Run；不得修改旧Run绑定
6. 扩展删除/重启/取消恢复，验证tombstone与所有迟到事件；每批保留累计集成回归

删除隔离、tombstone/fence以及在途操作安全是首次创建/准备/启动相关操作的适用前置条件，不是一律延迟到步骤6。步骤6扩展完整恢复矩阵；前面任何切片会接触已删除/删除中Workspace，都必须先验证拒绝复活与不越权清理。

角色批通过不等于所有定义业务已通过；共享生命周期切换仍须全部参与者ready，Workspace自己的Quiesce不能代表全局屏障。

closeout的slot确认必须保留handoff屏障和身份/fence，直到Run最终SM-1状态可核对。不能在Result已保存、Run尚未终态时立即允许新Start，否则可能新旧双活动；此阶段中断按同一交接恢复。

## 5. 数据、兼容与回退

目录/包移动默认零数据迁移，禁止顺手更换owner键、ID前缀或journal位置。定义schema变化明确版本、hash算法及来源映射；不能重算旧hash后冒充旧批准。旧记录若不能证明审查齐全，保留历史并明确缺证据，不回填虚构review。

确认prepared后崩溃，应由原journal恢复materialized；重复确认同内容幂等、同身份不同内容冲突。活动slot与Run跨owner交接各阶段持久、CAS/fence可核对。删除已开始的Workspace不得因迟到创建/Start事件复活。

回退包路径须先保证只注册一个owner且旧程序能读当前schema；保留当前journal、slot、ticket批准及Run链接。已发生外部资源准备/执行时不能仅恢复旧Workspace快照，否则会遗忘真实effect；先对账并保留unknown预算/回执，必要时前向修复。不误删用户源目录，不以目录清空冒充全owner删除成功。

## 6. 验收用例（拟定，未运行）

1. 提升Module后同WorkspaceID/定义/删除journal可读可恢复，运行注册恰好一次
2. 第N资源准备失败时M3不成立；重试原请求仅补未完成阶段
3. 完整封存与资源/检查满足时M3回执成立且Run数量为0
4. 4097至32768字节合法exact Goal走独立原文Requirement，普通分组仍限4096字节；不截断、不借改写/混组获得长文本例外，并核对来源/全部适用要求
5. 六critic均PASS但漏一required要求/适用维度，候选不能封存；N/A需理由
6. A+B映射同DoD，运行输入仍保留A/B两项义务，B未证不得闭合
7. 候选A确认后变为B，旧确认/approve失效；物化中断沿原journal恢复
8. 双激活/响应丢失不产生非法双活动Run，旧readiness不绕过服务端重查
9. ticket批准新定义后只创建合法后继新Run；旧Run合同/历史不改
10. 删除中崩溃可按同journal继续；迟到Start被tombstone/fence拒绝，用户源目录不误删

slot迁移额外核对[VS-V27](../validation/验收规格.md#vs-v27)：首Run已终结时slot虽空，首次激活意图仍关联原Run；迟到自动请求不得新占slot。

新增回归设计（均未运行）：漏/重复candidate、混入Goal、跨source-kind混组、重复/大小写别名键、null、尾随JSON、孤立surrogate拒绝；成功原文与typed/canonical或失败原错误不一致时，owner重载拒绝而不修补；同permit异completion冲突；document完整且wire部分时保留两种独立事实；普通查询无原文，受限查询精确绑定Workspace/permit。旧记录缺mapping/原document时保留历史并按兼容规则阻塞，不伪造成功审计或重算旧批准。

## 8. 历史固定版本证据（5554296）

- [跨owner删除协调](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/biz/deletion_coordinator.go#L30-L402)
- [规格映射](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/definitionadmission/compiler.go#L113-L163)
- [review覆盖校验](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/goal_specification.go#L292-L370)
- [原Goal精确映射](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/workspace/pkg/models/definition_authoring_specification.go#L149-L156)

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
