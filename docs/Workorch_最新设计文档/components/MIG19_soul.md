# MIG19_soul 行为边界与快照迁移方案

- 版本：v0.4；日期：2026-10-12；状态：设计冻结候选，未获用户批准，本轮未实施
- 角色：Block → Block；源码基线：`5360caf36e081717087557f34d2af3af1c16dc16`
- 本文只定义本组件的复用和迁移增量；[文档权威与共同前置](00_迁移总索引与集成顺序.md#authorities)、[当前缺口](../Workorch_逐项收口总纲与首项方案.md#gap-ledger)另有唯一维护处

<a id="migration-delta"></a>
## 迁移差分与稳定中间态

| 处理 | 本组件范围 |
|---|---|
| 保留 | Soul版本/hash/来源、capability边界及默认升级合同 |
| 本轮剩余适配/验收（已有实现见§0） | 类型化约束合成、用户编辑冲突保护与历史引用核验 |
| 移除或禁止继续依赖 | 按名称覆盖用户Soul；默认新版热替旧Run或扩大权限 |

- 批次归属：参与C05/C07/C13/C14；按能力需要参与B01/B05/B10，前置依赖与出口只采用[总纲能力批](../Workorch_逐项收口总纲与首项方案.md#batches)，本MIG不构成另一次序
- 可接受中间态：默认升级与冻结绑定分开；已编辑资产保持原版本直到明确合法选择
- 升级/回退重点：旧hash与source原样保留；回退不能恢复已撤销访问或用旧默认覆盖并发编辑。共同版本、故障和回退规则见[固定合同规范](../Workorch_组件固定合同与分层验收规范.md#compatibility)
- 验证入口：[FX-G](../validation/固定验收Fixtures.md#fx-g)、[FX-P](../validation/固定验收Fixtures.md#fx-p)；[VS-V13](../validation/验收规格.md#vs-v13), [VS-V37](../validation/验收规格.md#vs-v37), [VS-V40](../validation/验收规格.md#vs-v40), [VS-V43](../validation/验收规格.md#vs-v43)。下文保留组件特有断言，通用输入/幂等/CAS/故障矩阵不在此重定义；当前均未运行

<a id="contract-edges"></a>
## 精确合同入边与出边

CT状态与目标边的读法见[合同引用约定](00_迁移总索引与集成顺序.md#contract-reading)。

| 方向 | 精确合同引用 | 迁移关注点 |
|---|---|---|
| 入边：应用/Workspace→Soul | [CT-soul-SoulCommandGet](../contracts/soul.md#ct-soul-soulcommandget)、[CT-soul-SoulCommandValidateCapability](../contracts/soul.md#ct-soul-soulcommandvalidatecapability)、[CT-soul-SoulCommandUpgradeBuiltinDefault](../contracts/soul.md#ct-soul-soulcommandupgradebuiltindefault) | 版本/来源/约束边界与用户编辑保护分别核验 |
| 返回边：Soul→调用方 | [CT-soul-SoulCommandQueryBuiltinDefaults](../contracts/soul.md#ct-soul-soulcommandquerybuiltindefaults) | 固定版本typed Result；允许列表不是真实能力readiness或操作许可 |

该owner页的全topic构成完整接口范围；上表仅列迁移核心边。完整入边、通知、其他出边及guard/results/error/compatibility见[soul具体合同页](../contracts/soul.md)；本表只选本次迁移直接影响的边，不缩减目录覆盖。

## 0. 2026-10-12固定SHA重核与剩余增量

新增owner_lifecycle，Get/List/ValidateCapability/QueryBuiltinDefaults可用于合法收尾；版本化、用户来源、默认升级和原CRUD边界已重核。

复用Soul版本/hash和用户编辑保护；核验升级冲突、删除历史引用及实际约束合成，恢复不得覆盖用户修改。

当前路径：`internal/modules/blocks/soul`。证据：[当前Module/Initiator桥](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/module.go#L1-L49)；[pkg/common/common.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/pkg/common/common.go#L1-L16)；[biz/owner_lifecycle.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner_lifecycle.go#L1-L57)；[biz/owner.go](https://github.com/muidea/workorch/blob/5360caf36e081717087557f34d2af3af1c16dc16/internal/modules/blocks/soul/biz/owner.go#L1-L296)。本轮只静态阅读，不运行项目测试、CI或live；源码已有实现与最终验收通过分开记录。

未变的产品责任、资源归属和下文验收断言已重核保留；局部集成与恢复沿本篇步骤及[共同恢复规则](../Workorch_组件固定合同与分层验收规范.md#compatibility)，不复制公共合同。

## 1. 责任、现状与复用决策

目标保持Soul Block，拥有DM15行为描述、来源和版本资产。Soul可以表达强制边界或建议，但不拥有Goal、合同approve、Run状态或证明闭包；与用户约束冲突时不能通过提示词优先级暗改承诺。

已查证：Soul已有Revision、ContentHash、SourceProvenance（user_managed/builtin_seed）、SoulMD、AllowedSkill/MCP/Capability/Category/Tool列表；StableSoulContentHash有明确前缀和内容算法；CloneSoul复制切片。现有合同含Get/List/ValidateCapability/QueryBuiltinDefaults及Create/Update/Delete/EnsureDefaults/UpgradeBuiltinDefault。不能声称本组件缺版本化、缺默认升级或需新建SoulBinding owner。

复用现有资产及默认来源区分；适配目标中的冻结使用、冲突报告和撤销语义。默认升级是否完整保护用户编辑、全部删除引用行为及hash跨版本兼容仍需深核，测试文件存在不证明本轮已验收。

## 2. typed交互与边界

1. Workspace读取指定Soul身份/revision/hash/source，形成定义绑定；工作流可以选择多个角色Soul，但每个应用位置、强制或建议性质及来源需明确。LLM不能自行将系统默认新版替换进旧Run
2. ValidateCapability输出能力边界判断，不等价于实际capability owner的readiness或Approval的操作许可。列表中允许某工具不意味着预算充足、网络可达或用户授权已经存在
3. Soul约束遵循MC03：不突破系统安全底线、当前授权和能力上限；不弱化用户目标/禁止项与已批准合同。无类型化合成规则或允许集合为空时拒绝并列出冲突，不做后加载覆盖
4. builtin升级和user-managed更新由Soul owner维护来源与版本；System/workorchd发typed命令组织配置，不能直接写Soul仓库。新默认只供后续选择，冻结快照保持原版本
5. 行为资产更新不是合同修改的旁路。影响现有承诺需ticket列出原因、约束差异与影响，最终精确approve后建立新定义及新Run；普通未绑定资产编辑不自动创建ticket或Run

## 3. 迁移顺序与历史恢复

- 先建立Soul当前模型字段到定义绑定、Run上下文的逐字段映射，确认hash算法版本、source与role引用不丢失；不更换现有owner和ID
- 在合同合成切片中成对收口Soul/Workspace/workorchd冲突检查；在候选使用切片中成对核验LLM的实际输入和原冻结引用。只有这样才能证明资产与执行使用一致
- 默认升级以原revision/content hash和source为条件，对已用户编辑内容显式冲突/保留；不靠名称判断可以覆盖。更新后返回新版本及可查询身份，失联重试读回原操作结果
- 旧Soul已有hash和revision时直接保留，验证算法兼容；缺失字段仅可从原内容推导，不把今天builtin种子补成昨天的来源。无法确定的绑定保留不充分状态并阻塞依赖动作
- 删除资产时核对Workspace/Run引用保留政策，允许历史最小快照和撤销事实并存。不得为“清理”覆盖历史Run上下文；已撤销访问不会因恢复旧备份自动重获授权
- 回滚先检查新hash/来源/升级记录能否被旧代码理解；保留并发用户改动和新引用，禁止恢复老默认快照覆盖新内容。不能安全回退时只读隔离并前向修复

## 4. 计划验收矩阵（未执行）

| 用例 | 确定预期及独立证据 |
|---|---|
| 正常Soul绑定与运行恢复 | revision/hash/source及实际上下文保持一致 |
| Soul允许而合同禁止某工具 | 合同限制有效，实际执行拒绝；无权限扩大 |
| 强制Soul与用户Goal不可兼容 | 明确冲突阻止封存，不静默丢弃任一方 |
| builtin升级遇用户已编辑资产 | 保留用户版本并报告冲突/显式选择，不能覆盖 |
| 升级成功但响应丢失 | 原身份可查询，重试不重复增版或形成混合内容 |
| 封存后默认版本升级 | 新定义可用新版，原Run快照及验收约束不变 |
| Soul删除/撤销后旧Run恢复 | 按访问/保留政策明确阻塞或读历史快照，不重新赋权 |
| shutdown期间更新尚未落盘 | 未持久完成不回报成功，保留owner和原请求可恢复 |

## 6. 历史固定版本与影响路径（5554296）

- [Soul资产与默认升级合同](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/soul/pkg/events/contracts.go)：`internal/modules/blocks/soul/pkg/events/contracts.go`
- [版本、来源、hash与clone](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/soul/pkg/models/soul.go)：`internal/modules/blocks/soul/pkg/models/soul.go`
- [默认值测试参考（未运行）](https://github.com/muidea/workorch/blob/5554296240074054db34b0c551f4d326cd34a361/internal/modules/blocks/soul/biz/builtin_defaults_test.go)：`internal/modules/blocks/soul/biz/builtin_defaults_test.go`

修订记录：v0.4按2026-10-12固定SHA重新核对组织、Module/Biz生命周期及本组件实际合同增量；已实现部分改为复用/回归，未变产品目标与验收保留。
