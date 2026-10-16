# MIG07 Decision 组件迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | Decision唯一ticket/resolution/proposal及既有状态 |
| 本轮剩余适配/验收（已有实现见§0） | 完整变更提案schema、无Run作用域和最终候选精确批准绑定 |
| 移除或禁止继续依赖 | ChangeTicket平行owner；把澄清resolved自动转换为变更批准 |

- 批次归属：参与C05/C08/C12；按能力需要参与B06/B08，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：先开启兼容读取并保持新写入关闭；无Run变更先闭合后，再开放有Run安全后继
- 升级/回退重点：已应用变更不能靠撤回/快照抹去；回退不得吞掉强制候选字段。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-C](../validation/固定验收Fixtures.md#fx-c)、[FX-R](../validation/固定验收Fixtures.md#fx-r)；[VS-V28](../validation/验收规格.md#vs-v28), [VS-V29](../validation/验收规格.md#vs-v29), [VS-V05](../validation/验收规格.md#vs-v05), [VS-V06](../validation/验收规格.md#vs-v06)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：运行或定义协调→Decision | [CT-decision-OpenRuntimeGovernanceRequest](../contracts/decision.md#ct-decision-openruntimegovernancerequest)、[CT-decision-OpenWorkspaceDefinitionClarification](../contracts/decision.md#ct-decision-openworkspacedefinitionclarification)、[CT-decision-DecisionCommandEnsureResolved](../contracts/decision.md#ct-decision-decisioncommandensureresolved) | 现有普通澄清与治理请求可复用；合同变更登记及修订schema已存在，最终approve/adoption仍是剩余目标 |
| 出边：Decision→Orchestrator | [CT-orchestrator-OrchestratorCommandRecordDecisionEvidence](../contracts/orchestrator.md#ct-orchestrator-orchestratorcommandrecorddecisionevidence) | 事实证据不等同matching acceptance；变更物化由Workspace负责 |
| 出边：Decision通知/读取 | [CT-decision-TicketResolved](../contracts/decision.md#ct-decision-ticketresolved)、[CT-decision-DecisionCommandGetResolution](../contracts/decision.md#ct-decision-decisioncommandgetresolution) | 保留版本化决议身份，resolved不授新定义或Start权 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[decision具体合同页](../contracts/decision.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。



新增入边：[CT-decision-RegisterContractChangeProposal](../contracts/decision.md#ct-decision-registercontractchangeproposal)。复用问题材料及最终候选修订登记；当前有Run路径fail-closed，登记不是批准。

## 0. 2026-10-12固定SHA重核与剩余增量

新增RegisterContractChangeProposal及append-only修订/最终候选核验；实际source受控，revision/hash和重复request冲突检查已有实现。生命周期同样采用逐订阅和context等待。

先复用无Run提案→Workspace独立候选→审查材料回读链；最终approve/adoption/运行期matching与新Run仍按目标补齐，不能把登记回执或普通resolved当批准。

当前路径：`internal/modules/blocks/decision`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/module.go#L1-L81)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/pkg/common/common.go#L1-L74)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/decision/biz/owner.go#L1-L296)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 现状与目标

保留Decision唯一持久ticket/resolution及runtime governance proposal生命周期。Governance只计算政策和提交typed提案；Workspace独占正式定义物化；Orchestrator掌握matching runtime acceptance。不得另建ChangeTicket owner、Governance协调账本或应用层批准副本。

已实现的合同变更提案登记/修订与待补齐的完整语义应分开：设定目标、验收标准或约束变化必须先发ticket并完整说明原因；最终候选形成且完成审查后才approve，随后新定义封存及新Run激活。该路线用户已确认，无需重新问路线；具体变更仍须精确批准。无Run的定义变更允许真实定义作用域，不能伪造Run。

## 2. 代码影响与复用

[internal/modules/blocks/decision/biz/owner.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/decision/biz/owner.go)、[internal/modules/blocks/decision/module.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/decision/module.go)、[internal/modules/blocks/decision/plugin.go](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/decision/plugin.go)是已核对的owner及装配入口；全部提案字段、序列化、repository及事件别名须在G0逐项定位。

复用Decision存储和现有open/resolved/superseded语义；补齐精确提案版本及定义作用域映射。不宣称现有枚举已实现所有批准/拒绝/撤回语义；不能因存在JSON值表示便认定跨owner合同已违规，也不能以自由payload绕过领域schema校验。

## 3. 提案内容与typed合同

提案最小内容、用户精确选择与formal resolution的先后关系以[NC04](../contracts/创建与定义变更合同.md#nc04)为唯一完整定义。本组件迁移必须保留原因、版本、主体权限和影响集的可追溯性，不将用户选择、运行期采纳和定义物化合并为一个状态。

以下为拟收口交互语义，不声称当前topic或接口名：

| 接收者/动作 | 精确绑定 | 输出及边界 |
|---|---|---|
| Decision登记/修订提案 | request identity、Workspace、可选Run、base definition、proposal revision/hash | TicketID、提案版本；同key异内容冲突 |
| Decision记录用户精确选择 | 授权主体、expected proposal、最终candidate revision/hash、决定理由 | D2A选择及pending resolution proposal；不是formal resolved，不直接应用定义 |
| Orchestrator运行期采纳 | pending proposal、exact选择、Workspace D2B receipt及当前Run/generation/binding | D2R matching acceptance或拒绝；不授旧证据credit |
| Decision正式提交 | exact选择、D2B及runtime-bound时的D2R receipt | D2C formal resolution/ACK；此后才允许Workspace adoption |
| Workspace版本预检与采纳/物化 | D2B预检不要求formal resolution；D3采纳要求D2C ACK及精确新候选 | 原确认journal的物化回执；无Run不造matching Run，不另建Decision定义表 |
| Run后继激活 | 新定义、合法交接、执行意图、唯一请求身份 | 新Run及前驱关系；Decision不得自行创建 |

消息所有权遵循[唯一合同规范](../Workorch_组件固定合同与分层验收规范.md#generic-contract)，不共享repository。“已批准/未应用/受阻”由回执推导，非第二状态机。

## 4. 最小迁移顺序

1. 先落定提案schema与旧记录读取规则；Decision、Governance、Workspace、workorchd在同一兼容变更集适配，默认新能力不开启
2. 打通无Run的提案→最终候选审查→精确approve→Workspace确认journal，确保M3仍零Run
3. 再接运行期matching acceptance及D2新定义/新Run安全交接；批准与应用是可恢复阶段，不假装跨owner原子提交
4. 每步写入后丢响应查询原TicketID/版本/决议/物化身份；重试不得新造定义或后继。竞争提案由定义CAS和唯一后继规则裁决

草稿中已设定承诺变更也走ticket；忠实拆分/同合同replan无需额外ticket。紧急停止须留痕，不改变合同。

## 5. 存储兼容与回退

新提案内容与schema版本绑定；旧一般澄清ticket保持原类型和历史，不能批量转换成“合同变更已批准”。旧缺失候选绑定的决议不授新合同权限；实质原因、差异或影响改变生成新提案版本，旧approve不可继承。

先支持新旧读取，再启用新写入；若旧二进制无法保留强制字段/识别新类型，回退须禁用相关新写入并保留可读服务，不能直接部署旧版本吞字段。已应用提案不能靠撤回或快照回滚逆转，须另发变更提案并保留旧定义、Run、成本、unknown效果与迟到receipt；不能安全回退则阻塞并前向修复。

## 6. 逐项验收设计（未执行）

| 编号 | 场景 | 预期与证据 |
|---|---|---|
| MIG07-T01 | 完整提案+最终候选精确approve | resolution与candidate hash一致，物化可关联 |
| MIG07-T02 | 原因只有“实现困难”，影响未知却要求无条件应用 | 可保存open材料但拒绝应用；缺项诊断 |
| MIG07-T03 | approve后再修改候选 | 旧approve不匹配，须重审重批 |
| MIG07-T04 | 无Run的合同变更 | 使用定义作用域；Run总数仍0，后续合法Start才创建 |
| MIG07-T05 | 运行期迟到决议/旧generation | matching拒绝，不改当前合同或解锁操作 |
| MIG07-T06 | 物化成功丢响应，批准消息重复 | 同一物化及后继身份恢复，无重复Run |
| MIG07-T07 | 两张ticket并发修改同一基线 | 一个合法CAS生效，另一个重评，不覆盖 |
| MIG07-T08 | 已批准但资源/交接不满足 | 展示激活受阻，不声称新Run执行 |
| MIG07-T09 | 普通任务拆分不改合同 | 沿普通replan；不滥发合同变更ticket |

## 本组件集成证据补充

须分别覆盖无Run定义变更与有Run安全后继；模拟决议不能冒充真人已批准具体变更。

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
