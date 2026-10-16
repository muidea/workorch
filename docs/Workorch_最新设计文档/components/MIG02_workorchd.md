# MIG02 workorchd组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Module → Module；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | 应用用例、owner decoder、activationcore与受限原文查询 |
| 本轮剩余适配/验收（已有实现见§0） | Web草稿恢复、源义务lowering、M3/M4与首次激活语义身份、可重试清理 |
| 移除或禁止继续依赖 | 全部TUI创建入口及别名；应用层平行权威或旧批准复用 |

- 批次归属：参与C03/C04/C05/C06/C09/C14/C15；按能力需要参与B01/B02/B03/B05/B06/B11，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：草稿链完成后可以只开放草稿/读取；完整M3与自动M4分别受owner回执及授权条件约束
- 升级/回退重点：旧UI回退前核对在途自动请求与物化；缺明确自动授权的旧数据只允许手动路径。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-C](../validation/固定验收Fixtures.md#fx-c)、[FX-G](../validation/固定验收Fixtures.md#fx-g)、[FX-P](../validation/固定验收Fixtures.md#fx-p)、[FX-L](../validation/固定验收Fixtures.md#fx-l)；[VS-V26](../validation/验收规格.md#vs-v26), [VS-V27](../validation/验收规格.md#vs-v27), [VS-V44](../validation/验收规格.md#vs-v44), [VS-V45](../validation/验收规格.md#vs-v45), [VS-V38](../validation/验收规格.md#vs-v38), [VS-V40](../validation/验收规格.md#vs-v40)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：适配层→workorchd | [CT-workorchd-WorkorchDCommandWorkspaceRunRequested](../contracts/workorchd.md#ct-workorchd-workorchdcommandworkspacerunrequested)、[CT-workorchd-WorkorchDCommandReadWorkspaceRuntimeView](../contracts/workorchd.md#ct-workorchd-workorchdcommandreadworkspaceruntimeview) | 保留已有入口/读取；Web-only创建和M3/M4语义是目标差分，不将旧RunRequested视为完整新激活合同 |
| 出边：workorchd→Workspace | [CT-workspace-WorkspaceDefinitionAuthoringCreate](../contracts/workspace.md#ct-workspace-workspacedefinitionauthoringcreate)、[CT-workspace-WorkspaceDefinitionAdmissionIntent](../contracts/workspace.md#ct-workspace-workspacedefinitionadmissionintent)、[CT-workspace-WorkspaceCommandExecutionIntent](../contracts/workspace.md#ct-workspace-workspacecommandexecutionintent) | 草稿、精确准入与执行意图分别适配；目标M3回执/首次activation-intent需在此链补齐 |
| 出边：workorchd→Decision/Approval/LLM | [CT-decision-DecisionCommandEnsureResolved](../contracts/decision.md#ct-decision-decisioncommandensureresolved)、[CT-approval-ApprovalCommandEnsureApproved](../contracts/approval.md#ct-approval-approvalcommandensureapproved)、[CT-llm-LLMCommandCompleteProviderDocument](../contracts/llm.md#ct-llm-llmcommandcompleteproviderdocument) | 决议、许可和审查分路；合同变更精确候选与授权自动策略属于目标补齐 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[workorchd具体合同页](../contracts/workorchd.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。


<a id="success-audit-access"></a>
## 0. 2026-10-12固定SHA重核与剩余增量

Setup在调用owner.Setup前保存Biz；BeginShutdown关闭执行准入/authoring任务/CLI/投影/HTTP，Quiesce按context等待，Stop错误不会清空module引用。源义务bootstrap和无Run合同变更材料登记已有生产实现。

复用现有生命周期与RuntimeBootstrapWithSourceObligations；补齐Web入口、精确批准/物化及运行期变更的剩余闭环，验证断线恢复和M3→独立M4，不重新造authoring账本。

当前路径：`internal/modules/application/workorchd`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/module.go#L1-L82)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/pkg/common/common.go#L1-L7)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/owner.go#L1-L434)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

成功审计仍要求完整已保存document；wire缺失/部分与document成功分开记录。普通查询只给metadata，原文查询除Workspace/permit绑定还须检查实际访问主体与导出脱敏。[无Run登记实际边界](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/contract_change.go#L1-L80)；[源义务bootstrap](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/application/workorchd/biz/appservice/activationcore/runtime_bootstrap.go#L17-L105)

## 1. 当前与目标责任

保持application Module，路径 internal/modules/application/workorchd。承担DM01–18相关入站适配、鉴权、Web交互与应用用例组合；不拥有第二份定义/合同、活动slot、Run或Decision账本。

已查证：模块在Setup前保存Biz并在Stop成功后才清空，原生命周期问题已有修正；源义务bootstrap已有实现，应配对Workspace与Orchestrator验证完整链。Web-only、M3/M4和完整合同变更仍按既定目标逐项取证。

## 2. 复用与具体路径

- 直接复用应用用例组织、typed owner交互、现有activationcore纯编译职能
- 适配 internal/modules/application/workorchd/module.go；先保留可清理owner及获取记录，再执行可失败Setup；停止失败保留可重试引用
- 适配 biz/appservice/activationcore/runtime_focus_compiler.go；保留每项源ObligationID、定义绑定及映射关系，不能仅用压缩后的DoD文字代替原承诺
- 使用现有 biz/definition_authoring_full_size_test.go作为回归参考，但补独立缺义务反例，不能把原测试PASS当新合同已满足
- Web已定位 `web/workorch-console/src/pages/WorkspaceDetailPage.tsx`，TUI已定位 `internal/services/workorch/service/tui/view.go`；本轮增量涉及用量展示，不构成创建入口已全部删除的证据。G0仍须枚举所有创建调用及别名。移除全部TUI“创建”入口及其绕路别名，保留未要求删除的观察/读取能力；不臆造不存在的前端文件

## 3. typed交互合同与不变量

以下为合同语义清单，不是声称现有Go符号名；实施时映射到对应owner pkg/events，必要新增须同步生产者消费者。

| 用例 | 生产者→消费者 | 必须绑定与结果 |
|---|---|---|
| 创建/恢复草稿 | Web→workorchd→Workspace | 请求身份、用户/Workspace scope、输入revision；返回权威创建阶段，不靠浏览器缓存 |
| 精确确认 | workorchd→Workspace | candidate revision/hash、确认内容、授权依据；返回物化回执 |
| readiness展示 | workorchd→资源owner/System | 精确定义与资源上下文、检查时点；只是事实汇总 |
| 激活 | workorchd→Workspace/Run既定启动协议 | 独立M4请求身份、StartupInputHash、用户或明确授权；slot/fence及唯一Run结果 |
| 合同变更 | workorchd→Decision→Workspace等 | ticket原因、最终候选hash、批准及物化关联；随后新Run |

M3成功必须同时满足封存、必要资源准备和当时启动检查，且尚未创建Run。默认展示“激活执行”由用户点击；授权自动模式也先形成M3回执，再用独立请求启动M4。精确接纳合同与执行授权分别记录，创建按钮不是隐含执行许可。网页断线不改变owner事实。

应用须原样传递明确授权政策及精确候选，展示真实接纳来源；回执约束由[Workspace迁移](MIG22_workspace.md#automatic-candidate-acceptance)统一记录，不伪造人工点击。

workorchd须将手动与自动传输请求归并为持久首次激活意图；页面重载/迟到请求不能再次执行。跨owner永久关联及首Run已终结反例见[MIG16](MIG16_run.md#first-activation-intent)，本组件与Run同批适配。

## 4. 小步迁移与成对依赖

1. 先与Workspace固定请求/返回语义及只读视图；不先制作脱离后端状态机的前端向导
2. 同批回归已修Setup/Stop失败保留，配合Base任务显式错误API；所有该批调用方不得漏接错误
3. 打通Web草稿、刷新恢复、幂等创建，移除TUI创建；此批只宣告草稿能力，不冒充M3完整
4. 复用已有mapping/独立长Goal/成功原文审计，与Workspace/LLM回归已实现的required审查覆盖和运行逐义务传递，核验语义充分性与精确封存，再展示完整M3回执
5. 与Run/Orchestrator/Workspace成组适配手动和授权自动激活；依赖全局生命周期屏障及真实独立oracle
6. 与Decision/Governance完成变更ticket精确approve路径；前端只展示采纳/物化/启动各阶段，不合并为一个“已完成”

## 5. 状态兼容、恢复和回退

沿用Workspace、输入、候选、确认journal等权威身份；前端会话/缓存可丢，不生成平行合同修订号。新增字段须声明缺省含义：缺少明确自动执行授权一律不自动启动。M3历史完成回执保留，当前readiness另行重查。

确认写入成功而HTTP丢失，按同请求读取owner回执；自动启动响应未知，查询原请求与Run，禁止换key重试。回退UI/应用前先检查在途自动启动和候选物化；不能恢复旧页面使新候选沿用旧approve。若旧程序不识别新协议，停写并保持可读诊断或前向修复，不删除新Run/请求记录。

## 6. 验收用例（拟定，未运行）

1. Web创建后刷新/断网恢复同一Workspace，不产生重复草稿
2. 全部M3条件满足返回完成回执，权威Run数量为0
3. 缺必要资源或封存未物化时不显示完成，不发送Start
4. 默认确认后不启动；点击激活且实时检查通过才产生唯一Run
5. 明确授权自动模式先M3后M4；撤销/失效授权或候选变化不得沿用
6. 双击激活、响应丢失重试保持一个请求结果与合法活动Run
7. 两项源义务共享DoD但仅完成一项，运行focus仍保留另一项，不被UI汇总隐藏
8. 旧候选确认、旧ticket批准均被服务端拒绝，前端不能越权覆盖
9. TUI全部创建入口及别名不可用，既有读取/观察功能仍按原合同回归
10. Setup中途失败可清理重试；Stop超时保留Biz和依赖，不显示全局已停

补充反例：授权自动请求延迟、用户手动启动且首Run已结束时，迟到自动请求不得再建Run；重载Web仍能读到同一首次激活关联。

补充观测反例（未运行）：Workspace有pre-Run LLM记录而Run窗口为空；触及记录上限、含不可解析事件、未加载payload及Run补入记录时，展示相应scope/partial/not_loaded，不伪称零费用或完整历史。普通列表/SSE不出现成功document/wire原文；受限审计按Workspace/permit返回并核对canonical hash，不授予封存/启动权。

## 8. 历史固定版本证据（5554296）

- [初始化与退出](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/application/workorchd/module.go#L36-L62)
- [focus生成](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/application/workorchd/biz/appservice/activationcore/runtime_focus_compiler.go#L85-L119)
- [现有大合同审查样例](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/application/workorchd/biz/definition_authoring_full_size_test.go#L139-L147)
- [共享Base任务API](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/base/biz/base.go#L119-L135)

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
