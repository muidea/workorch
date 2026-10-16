# MIG18_skill 技能资产与绑定迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | Skill Get/List/Create、ID/Version、Content和Provenance |
| 本轮剩余适配/验收（已有实现见§0） | 冻结引用解析、实际LLM采用内容、依赖/归属核验 |
| 移除或禁止继续依赖 | 无需求补Update/Delete；Skill自建执行状态机或越权修改required |

- 批次归属：参与C05/C07/C13/C14；按能力需要参与B01/B05/B10，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：仅启用现有来源类型的确定性引用；外部目录、local副本与缓存分别归属
- 升级/回退重点：缺hash只能从原内容补并标算法；旧Run在途引用不可删除或同版本覆盖。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-G](../validation/固定验收Fixtures.md#fx-g)、[FX-P](../validation/固定验收Fixtures.md#fx-p)；[VS-V20](../validation/验收规格.md#vs-v20), [VS-V24](../validation/验收规格.md#vs-v24), [VS-V37](../validation/验收规格.md#vs-v37), [VS-V40](../validation/验收规格.md#vs-v40)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：应用/Workspace→Skill | [CT-skill-SkillCommandCreate](../contracts/skill.md#ct-skill-skillcommandcreate)、[CT-skill-SkillCommandGet](../contracts/skill.md#ct-skill-skillcommandget)、[CT-skill-SkillCommandList](../contracts/skill.md#ct-skill-skillcommandlist) | 仅现有Get/List/Create；引用展开与冻结内容需调用方同步适配 |
| 返回边：Skill→调用方 | [CT-skill-SkillCommandGet](../contracts/skill.md#ct-skill-skillcommandget) | typed Result保留Content/Provenance并深复制；没有另造Update/Delete或执行命令 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[skill具体合同页](../contracts/skill.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

新增owner_lifecycle及逐订阅账本，Get/List/资产读取与fresh创建按真实路径处理；角色及Get/List/Create合同范围已重核保持。

复用内容/Provenance与冻结引用，检查真实LLM采用、引用依赖、收尾读取/停止恢复；无需求不添加Update/Delete或执行引擎。

当前路径：`internal/modules/blocks/skill`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/module.go#L1-L49)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/pkg/common/common.go#L1-L17)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner_lifecycle.go#L1-L56)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/skill/biz/owner.go#L1-L211)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 责任、现状与复用决策

目标保持Skill Block，拥有DM15/39技能资产、目录来源和版本引用；Skill提供可复用方法与依赖声明，不是合同、操作授权或验收裁决者。不能因为Skill描述了一个工作流就另建有独立状态机的执行引擎。

已查证：Skill包含ID/Version、Instruction、ReferenceRefs、RequiredToolKinds、RequiredMCPServerRefs、RequiredCapabilityRefs、Constraints和Provenance；Content投影与CloneSkill拷贝相关切片。当前typed合同公开Get/List/Create，并没有据此认定存在Update/Delete。新增生命周期操作必须先核验实际需要，不为接口对称填齐CRUD。

复用内容模型、来源记录和公开查询/创建；适配冻结引用及依赖检查，检视外部目录与Workspace-local副本的归属。当前根及生命周期形态可复用，未逐一审查引用展开、路径解析和全部资产边界，不能预先标为实现通过。

## 2. typed交互与约束合成

1. Workspace定义准备通过Skill owner取得指定资产版本及来源，形成可追踪绑定；模板只引用Skill，不接管资产源。LLM读取的是冻结内容/引用，不是每次按名称读取最新版
2. RequiredToolKinds/MCPServerRefs/CapabilityRefs只是依赖声明，由实际能力owner给出当前可用性，Workspace核对定义准入，启动时服务端重查；Skill可发现不等于工具已授权
3. Skill Constraints与用户目标、模板强制条件、Soul边界按MC03的已定义类型化规则合成；冲突要显式呈现，不能以后加载覆盖或修改required验收项。文本指令要求越权时按非权威输入处理
4. Skill版本/content hash、Provenance、外部引用解析版本和本地副本归属可追溯。Version相同但内容不同必须报告冲突/新的内容身份，不能把版本字符串当完整性证明
5. 变更资产可供新定义选择；已冻结Run不热换技能。安全下架可阻塞后续调用，但不能改写历史输入。只有影响既定承诺的替换才走ticket→精确approve→新定义→新Run

## 3. 定向迁移与回退

- 先枚举现有Get/List/Create生产消费者、Content与Provenance的投影路径，确认来源在跨owner交接后仍完整；目录组织保持blocks/skill
- 与Workspace/workorchd的合同准备成对固定版本、引用解析和依赖失败结果；再与LLM上下文使用方成对验证实际采用内容。M3只能证明必要技能/依赖准备就绪，不暗创建Run
- 先支持现有来源类型的确定性引用，不增加无需求的通用资产服务。外部目录读取、Workspace-local复制、缓存与最终交付产物分开，避免把用户源目录当临时副本清理
- 旧资产仅Version可知而hash缺失时，可对原持久内容计算并注明算法/迁移来源；无原始内容则标不充分，不能以目录当前最新版补历史。历史Run保存其当时binding，不全量覆盖
- 回滚保持原ID、来源与新旧版本关联，禁止将新版本覆盖回旧路径造成同版本异内容。存在在途Run使用新引用时，旧程序若不能解析须阻塞回退或只读兼容，不能删除被引用资产
- 全局Base/停机适配核验资产读取与复制任务的完成和资源释放；没有查明独立局部缺陷前仅登记核验影响，不照搬其他Block的bug结论

## 4. 计划验收矩阵（未执行）

| 用例 | 确定预期及独立证据 |
|---|---|
| 固定技能版本创建合同 | 来源、内容摘要、依赖检查与定义绑定相互一致 |
| 同版本字符串内容被替换 | 完整性冲突或新内容身份，不悄然沿用旧seal |
| 技能要求放宽禁止项/验收标准 | 编译/采纳拒绝并列冲突，required集合与授权不变 |
| 目录可发现但MCP依赖失效 | 当前readiness阻塞；目录列表成功不等于M3成功 |
| 封存后发布Skill新版 | 旧Run仍用原内容，新定义可显式选择新版 |
| 引用缺失/越出允许资源scope | 明确失败及来源定位，不用空内容或其他同名文件替代 |
| Workspace-local副本准备中断 | 原请求可恢复，外部源文件不被删除/覆盖 |
| 返回DTO切片被消费者修改 | owner资产不变，clone与Content投影各自行为可核对 |

## 6. 历史固定版本与影响路径（5554296）

- [现有Get/List/Create合同](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/skill/pkg/events/contracts.go)：`internal/modules/blocks/skill/pkg/events/contracts.go`
- [Skill、Content与Provenance](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/skill/pkg/models/skill.go)：`internal/modules/blocks/skill/pkg/models/skill.go`
- [现有组织边界测试参考（未运行）](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/skill/boundary_test.go)：`internal/modules/blocks/skill/boundary_test.go`

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
